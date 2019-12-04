// Copyright 2019 Tad Lebeck
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.


package csi

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/application_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/consistency_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/snapshot"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/csi"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mountOperators interface {
	buildVarMap(ctx context.Context)
	checkForVSR(ctx context.Context) (bool, error)
	createMountVSR(ctx context.Context) error
	fetchLatestSnapshot(ctx context.Context, vsID string) (*models.Snapshot, error)
	fetchOrCreateCG(ctx context.Context) error
	fetchOrCreateAG(ctx context.Context, cgName string) error
	fetchTags(avdTags []string, podAnnotationFmt string) []string
	getVolumeState(ctx context.Context, vsID string) (vsMediaState, error)
	substituteVariables(s string) string
	varValue(avdValue string, annKey string, fallbackValue string) string
	waitForVSR(ctx context.Context) error
}

type mountOp struct {
	c             *csiComp
	ops           mountOperators
	app           *agentd.AppCtx
	args          *csi.MountArgs
	node          *models.Node
	cluster       *models.Cluster
	domain        *models.CSPDomain
	vs            *models.VolumeSeries
	vsr           *models.VolumeSeriesRequest
	vW            vra.RequestWaiter
	vsState       vsMediaState
	variableMap   map[string]string
	cg            *models.ConsistencyGroup
	ag            *models.ApplicationGroup
	annotationMap map[string]string
	snapshot      *models.Snapshot
}

// MountVolume is part of csi.NodeHandlerOps
func (c *csiComp) MountVolume(ctx context.Context, args *csi.MountArgs) error {
	cluster := c.app.AppObjects.GetCluster()
	node := c.app.AppObjects.GetNode()
	domain := c.app.AppObjects.GetCspDomain()
	if err := args.Validate(ctx); err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	op := &mountOp{
		c:       c,
		args:    args,
		app:     c.app,
		domain:  domain,
		cluster: cluster,
		node:    node,
	}
	op.ops = op // self-reference
	return op.run(ctx)
}

func (o *mountOp) run(ctx context.Context) error {
	pendingVSR, err := o.ops.checkForVSR(ctx)
	if err != nil {
		return err
	}
	if pendingVSR {
		o.app.Log.Debugf("CSI: VSR [%s] pending for volume-series [%s]", o.vsr.Meta.ID, o.args.VolumeID)
		return o.ops.waitForVSR(ctx)
	}
	o.vsState, err = o.ops.getVolumeState(ctx, o.args.VolumeID)
	o.app.Log.Debugf("CSI: vs state: [%s]", o.vsState.String())

	if o.vsState == NotCreatedDynamicState {
		o.ops.buildVarMap(ctx)
		err = o.ops.fetchOrCreateCG(ctx)
		if err != nil {
			return err
		}
	}

	switch o.vsState {
	case InvalidState:
		o.app.Log.Debugf("CSI: invalid-state err: %s", err.Error())
		return err
	case MountedState:
		return nil
	default:
		err = o.ops.createMountVSR(ctx)
	}
	if err == nil {
		return o.ops.waitForVSR(ctx)
	}
	return err
}

func (o *mountOp) checkForVSR(ctx context.Context) (bool, error) {
	o.app.Log.Debugf("CSI: check for pending VSR of volume-series [%s]", o.args.VolumeID)
	params := &volume_series_request.VolumeSeriesRequestListParams{
		IsTerminated: swag.Bool(false),
		NodeID:       swag.String(string(o.node.Meta.ID)),
		SystemTags:   []string{fmt.Sprintf("%s:%s", com.SystemTagVsrK8sNodePublish, o.args.VolumeID)},
	}
	vrl, err := o.app.OCrud.VolumeSeriesRequestList(ctx, params)
	if err != nil {
		o.app.Log.Errorf("CSI: checkForVSR: %s", err.Error())
		return false, status.Errorf(codes.Internal, "checkForVSR: %s", err.Error())
	}
	if len(vrl.Payload) >= 1 {
		o.vsr = vrl.Payload[0]
		return true, nil
	}
	return false, nil
}

func (o *mountOp) opsForState() []string {
	switch o.vsState {
	case NotCreatedDynamicState:
		return []string{com.VolReqOpCreate, com.VolReqOpBind, com.VolReqOpMount, com.VolReqOpAttachFs}
	case NotBoundState:
		return []string{com.VolReqOpBind, com.VolReqOpMount, com.VolReqOpAttachFs}
	case NotMountedState:
		return []string{com.VolReqOpMount, com.VolReqOpAttachFs}
	case NoFsAttachedState:
		return []string{com.VolReqOpAttachFs}
	case SnapRestoreState:
		return []string{com.VolReqOpMount, com.VolReqOpVolRestoreSnapshot, com.VolReqOpAttachFs}
	}
	return []string{}
}

func (o *mountOp) createMountVSR(ctx context.Context) error {
	o.app.Log.Debugf("CSI: create VSR for volume-series [%s]", o.args.VolumeID)
	if err := o.c.rei.ErrOnBool("csi-mount-vsr"); err != nil {
		o.app.Log.Errorf("CSI: rei mount failure: %s", err.Error())
		return err
	}

	o.vsr = &models.VolumeSeriesRequest{
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			Creator: &models.Identity{
				AccountID: models.ObjIDMutable(o.args.Account.Meta.ID),
			},
			ClusterID:           models.ObjIDMutable(o.cluster.Meta.ID),
			RequestedOperations: o.opsForState(),
			CompleteByTime:      strfmt.DateTime(time.Now().Add(o.c.Args.VSRCompleteByPeriod)),
			TargetPath:          o.args.TargetPath,
			ReadOnly:            o.args.ReadOnly,
			FsType:              o.args.FsType,
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID:     models.ObjIDMutable(o.node.Meta.ID),
				SystemTags: models.ObjTags([]string{fmt.Sprintf("%s:%s", com.SystemTagVsrK8sNodePublish, o.args.VolumeID)}),
			},
		},
	}
	if o.vsState == NotCreatedDynamicState {
		o.vsr.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{
			VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
				AccountID: models.ObjIDMutable(o.args.Account.Meta.ID),
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				Name:               models.ObjName(o.args.Name),
				ServicePlanID:      models.ObjIDMutable(o.args.ServicePlan),
				SizeBytes:          swag.Int64(o.args.SizeBytes),
				SystemTags:         models.ObjTags([]string{fmt.Sprintf("%s:%s", com.SystemTagVolumeSeriesID, o.args.VolumeID)}),
				ConsistencyGroupID: models.ObjIDMutable(o.cg.Meta.ID),
				Tags:               models.ObjTags(o.fetchTags(o.args.AVD.VolumeTags, com.K8sPodAnnotationVolTagsFmt)),
				ClusterDescriptor:  models.ClusterDescriptor{com.ClusterDescriptorPVName: models.ValueType{Kind: "STRING", Value: o.args.Name}},
			},
		}
	} else {
		o.vsr.VolumeSeriesID = models.ObjIDMutable(o.args.VolumeID)
	}
	if o.vsState == SnapRestoreState {
		o.vsr.SnapshotID = models.ObjIDMutable(o.snapshot.Meta.ID)
	}
	vsr, err := o.app.OCrud.VolumeSeriesRequestCreate(ctx, o.vsr)
	if err != nil {
		o.app.Log.Errorf("CSI: createMountVSR: %s", err.Error())
		return status.Errorf(codes.Internal, "createMountVSR: %s", err.Error())
	}
	o.vsr = vsr
	return nil
}

func (o *mountOp) waitForVSR(ctx context.Context) error {
	o.app.Log.Debugf("CSI: wait for VSR [%s] for volume-series [%s]", o.vsr.Meta.ID, o.args.VolumeID)
	var err error
	wArgs := &vra.RequestWaiterArgs{
		VSR:       o.vsr,
		CrudeOps:  o.app.CrudeOps,
		ClientOps: o.app.OCrud,
		Log:       o.app.Log,
	}
	if o.vW == nil { // to aid in UT
		o.vW = vra.NewRequestWaiter(wArgs)
	}
	if err == nil {
		ctx, cancel := o.vW.HastenContextDeadline(ctx, time.Second)
		defer cancel()
		o.vsr, err = o.vW.WaitForVSR(ctx)
	}
	if err != nil {
		if err == vra.ErrRequestWaiterCtxExpired {
			o.app.Log.Errorf("CSI: waitForVSR: %s: volume-series-request [%s] %s", err.Error(), o.vsr.Meta.ID, o.vsr.VolumeSeriesRequestState)
			return status.Errorf(codes.DeadlineExceeded, "request [%s] in progress [%s]", o.vsr.Meta.ID, o.vsr.VolumeSeriesRequestState)
		}
		o.app.Log.Errorf("CSI: waitForVSR: %s", err.Error())
		return status.Errorf(codes.Internal, "waitForVSR: %s", err.Error())
	}
	if o.vsr.VolumeSeriesRequestState != com.VolReqStateSucceeded {
		sep, detail := "", ""
		if detail = vra.GetFirstErrorMessage(o.vsr.RequestMessages); detail != "" {
			sep = ": "
		}
		o.app.Log.Errorf("CSI: waitForVSR: volume-series-request [%s] %s%s%s", o.vsr.Meta.ID, o.vsr.VolumeSeriesRequestState, sep, detail)
		return status.Errorf(codes.Internal, "request [%s] %s%s%s", o.vsr.Meta.ID, o.vsr.VolumeSeriesRequestState, sep, detail)
	}
	_, err = o.ops.getVolumeState(ctx, o.args.VolumeID)
	return err
}

func (o *mountOp) getVolumeState(ctx context.Context, vsID string) (vsMediaState, error) {
	var err error
	o.vs, err = o.app.OCrud.VolumeSeriesFetch(ctx, o.args.VolumeID)
	if err != nil {
		if e, ok := err.(*crud.Error); ok {
			if e.NotFound() {
				if o.args.Dynamic {
					return NotCreatedDynamicState, status.Errorf(codes.Internal, "volume-series [%s] not yet created", o.args.VolumeID)
				}
				return InvalidState, status.Errorf(codes.NotFound, "volume-series [%s] not found", vsID)
			}
		} else {
			return InvalidState, status.Errorf(codes.Internal, "volume-series [%s] fetch failed: %s", vsID, err.Error())
		}
	}
	if string(o.vs.BoundClusterID) != string(o.cluster.Meta.ID) {
		return InvalidState, status.Errorf(codes.Internal, "volume-series [%s] not bound to cluster [%s]", vsID, string(o.cluster.Meta.ID))
	}
	if o.vs.VolumeSeriesState == com.VolStateInUse && vra.VolumeSeriesHeadIsMounted(o.vs) {
		if !vra.VolumeSeriesFsIsAttached(o.vs) {
			return NoFsAttachedState, status.Errorf(codes.Internal, "volume-series [%s] fs is not attached", o.args.VolumeID)
		}
		if o.vs.ConfiguredNodeID != models.ObjIDMutable(o.node.Meta.ID) {
			return InvalidState, status.Errorf(codes.Internal, "volume-series [%s] is mounted on node [%s]", o.args.VolumeID, o.vs.ConfiguredNodeID)
		}
		o.app.Log.Debugf("CSI: volume-series [%s] is mounted", o.vs.Meta.ID)
		return MountedState, nil
	}
	if !vra.VolumeSeriesIsBound(o.vs.VolumeSeriesState) && !vra.VolumeSeriesIsProvisioned(o.vs.VolumeSeriesState) {
		return InvalidState, status.Errorf(codes.Internal, "volume-series [%s] state is not mountable: %s", o.args.VolumeID, o.vs.VolumeSeriesState)
	}
	if !vra.VolumeSeriesIsProvisioned(o.vs.VolumeSeriesState) {
		if o.snapshot, err = o.fetchLatestSnapshot(ctx, o.args.VolumeID); err != nil {
			return InvalidState, status.Errorf(codes.Internal, "error fetching snapshot for volume-series [%s]: %s", o.args.VolumeID, err.Error())
		}
		if o.snapshot != nil {
			return SnapRestoreState, status.Errorf(codes.Internal, "snapshot [%s] for volume-series [%s] being restored", string(o.snapshot.Meta.ID), o.args.VolumeID)
		}
	}
	return NotMountedState, status.Errorf(codes.Internal, "volume-series [%s] is not mounted", o.args.VolumeID)
}

func (o *mountOp) fetchOrCreateCG(ctx context.Context) error {
	o.app.Log.Debug("CSI: deriving consistency group")
	cgName := o.varValue(o.args.AVD.ConsistencyGroupName, com.K8sPodAnnotationCGName, com.ConsistencyGroupNameUnspecified) // TBD: use cluster usage policy for default
	cgDescription := o.varValue(o.args.AVD.ConsistencyGroupDescription, com.K8sPodAnnotationCGDesc, "")
	cgTags := o.fetchTags(o.args.AVD.ConsistencyGroupTags, com.K8sPodAnnotationCGTagsFmt)

	lParams := &consistency_group.ConsistencyGroupListParams{
		Name:      swag.String(cgName),
		AccountID: swag.String(string(o.args.Account.Meta.ID)),
	}
	cgList, err := o.app.OCrud.ConsistencyGroupList(ctx, lParams)
	if err != nil {
		o.app.Log.Debugf("CSI: failed to query consistency group: %s", err.Error())
		return err
	}
	if len(cgList.Payload) == 1 {
		o.cg = cgList.Payload[0]
	} else {
		err = o.ops.fetchOrCreateAG(ctx, cgName)
		if err != nil {
			return err
		}
		o.app.Log.Debugf("CSI: creating consistency group: %s", cgName)
		cg := &models.ConsistencyGroup{
			ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
				AccountID: models.ObjIDMutable(o.args.Account.Meta.ID),
			},
			ConsistencyGroupMutable: models.ConsistencyGroupMutable{
				Name: models.ObjName(cgName),
				ApplicationGroupIds: []models.ObjIDMutable{
					models.ObjIDMutable(o.ag.Meta.ID),
				},
				Tags:        models.ObjTags(cgTags),
				Description: models.ObjDescription(cgDescription),
				SystemTags:  models.ObjTags([]string{com.SystemTagMustDeleteOnUndoCreate}),
			},
		}
		o.cg, err = o.app.OCrud.ConsistencyGroupCreate(ctx, cg)
		if err != nil {
			o.app.Log.Debugf("CSI: create consistency group failed: %s", err.Error())
			return err
		}
	}
	return nil
}

func (o *mountOp) fetchOrCreateAG(ctx context.Context, cgName string) error {
	fallbackName := cgName
	if val, ok := o.variableMap[com.TemplateVarK8sPodControllerName]; ok && val != "" {
		fallbackName = val
	}
	agName := o.varValue(o.args.AVD.ApplicationGroupName, com.K8sPodAnnotationAGName, fallbackName) // TBD: use cluster usage policy for default
	agDescription := o.varValue(o.args.AVD.ApplicationGroupDescription, com.K8sPodAnnotationAGDesc, "")
	agTags := o.fetchTags(o.args.AVD.ApplicationGroupTags, com.K8sPodAnnotationAGTagsFmt)

	lParams := &application_group.ApplicationGroupListParams{
		Name:      swag.String(agName),
		AccountID: swag.String(string(o.args.Account.Meta.ID)),
	}
	agList, err := o.app.OCrud.ApplicationGroupList(ctx, lParams)
	if err != nil {
		o.app.Log.Debugf("CSI: failed to query application group: %s", err.Error())
		return err
	}
	if len(agList.Payload) == 1 {
		o.ag = agList.Payload[0]
	} else {
		o.app.Log.Debugf("CSI: creating application group: %s", agName)
		ag := &models.ApplicationGroup{
			ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
				AccountID: models.ObjIDMutable(o.args.Account.Meta.ID),
			},
			ApplicationGroupMutable: models.ApplicationGroupMutable{
				Name:        models.ObjName(agName),
				Description: models.ObjDescription(agDescription),
				Tags:        models.ObjTags(agTags),
				SystemTags:  models.ObjTags([]string{com.SystemTagMustDeleteOnUndoCreate}),
			},
		}
		o.ag, err = o.app.OCrud.ApplicationGroupCreate(ctx, ag)
		if err != nil {
			o.app.Log.Debugf("CSI: create application group failed: %s", err.Error())
			return err
		}
	}
	return nil
}

func (o *mountOp) fetchTags(avdTags []string, podAnnotationFmt string) []string {
	tags := []string{}
	for _, tag := range avdTags {
		tags = append(tags, o.substituteVariables(tag))
	}
	if len(tags) == 0 {
		ok := true
		val := ""
		for i := 1; ok; i++ {
			key := fmt.Sprintf(podAnnotationFmt, i)
			if val, ok = o.annotationMap[key]; ok {
				tags = append(tags, o.substituteVariables(val))
			}
		}
	}
	return tags
}

func (o *mountOp) buildVarMap(ctx context.Context) {
	o.variableMap = map[string]string{
		com.TemplateVarAccountID:        string(o.args.Account.Meta.ID),
		com.TemplateVarAccountName:      string(o.args.Account.Name),
		com.TemplateVarClusterID:        string(o.cluster.Meta.ID),
		com.TemplateVarClusterName:      string(o.cluster.Name),
		com.TemplateVarCspDomainID:      string(o.domain.Meta.ID),
		com.TemplateVarCspDomainName:    string(o.domain.Name),
		com.TemplateVarVolumeSeriesID:   string(o.args.VolumeID),
		com.TemplateVarVolumeSeriesName: string(o.args.Name),
	}
	pfa := &cluster.PodFetchArgs{
		Name:      o.args.PodName,
		Namespace: o.args.PodNamespace,
	}
	o.variableMap[com.TemplateVarK8sPodName] = o.args.PodName
	o.variableMap[com.TemplateVarK8sPodNamespace] = o.args.PodNamespace
	podObj, _ := o.c.app.ClusterClient.PodFetch(ctx, pfa)
	if podObj != nil {
		o.variableMap[com.TemplateVarK8sPodControllerName] = podObj.ControllerName
		o.variableMap[com.TemplateVarK8sPodControllerUID] = podObj.ControllerID
		for k, v := range podObj.Labels {
			key := fmt.Sprintf(com.TemplateVarK8sPodLabelsFmt, k)
			o.variableMap[key] = v
		}
		o.annotationMap = podObj.Annotations
	} else {
		o.annotationMap = map[string]string{}
	}
}

func (o *mountOp) substituteVariables(s string) string {
	return os.Expand(s, func(varName string) string {
		return o.variableMap[varName]
	})
}

func (o *mountOp) varValue(avdValue string, annKey string, fallbackValue string) string {
	if avdValue != "" {
		return o.substituteVariables(avdValue)
	} else if mapValue, ok := o.annotationMap[annKey]; ok {
		return o.substituteVariables(mapValue)
	} else if fallbackValue != "" {
		return o.substituteVariables(fallbackValue)
	}
	return ""
}

func (o *mountOp) fetchLatestSnapshot(ctx context.Context, vsID string) (*models.Snapshot, error) {
	lParams := &snapshot.SnapshotListParams{
		VolumeSeriesID: &vsID,
		SortDesc:       []string{"snapTime"},
		Limit:          swag.Int32(1),
	}
	snapList, err := o.app.OCrud.SnapshotList(ctx, lParams)
	if err != nil {
		return nil, err
	}
	if len(snapList.Payload) == 1 {
		return snapList.Payload[0], nil
	}
	return nil, nil
}
