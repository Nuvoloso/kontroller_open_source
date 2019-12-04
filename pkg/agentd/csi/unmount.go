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
	"time"

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/csi"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type unmountOperators interface {
	checkForVSRs(ctx context.Context) bool
	createUnmountVSR(ctx context.Context, ops []string) error
	waitForVSR(ctx context.Context) error
	volUnpublished(ctx context.Context, vsID string) (vsMediaState, error)
}

type unmountOp struct {
	c                 *csiComp
	ops               unmountOperators
	app               *agentd.AppCtx
	args              *csi.UnmountArgs
	nodeIdentifier    string
	clusterIdentifier string
	vs                *models.VolumeSeries
	vsr               *models.VolumeSeriesRequest
	vW                vra.RequestWaiter
}

// UnmountVolume is part of csi.NodeHandlerOps
func (c *csiComp) UnmountVolume(ctx context.Context, args *csi.UnmountArgs) error {
	cluster := c.app.AppObjects.GetCluster()
	node := c.app.AppObjects.GetNode()
	if err := args.Validate(ctx); err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	op := &unmountOp{
		c:                 c,
		args:              args,
		app:               c.app,
		nodeIdentifier:    string(node.Meta.ID),
		clusterIdentifier: string(cluster.Meta.ID),
	}
	op.ops = op // self-reference
	return op.run(ctx)
}

func (o *unmountOp) run(ctx context.Context) error {
	vsState, err := o.ops.volUnpublished(ctx, o.args.VolumeID)
	switch vsState {
	case InvalidState:
		return err
	case NotMountedState:
		return nil
	case MountedState:
		o.app.Log.Debugf("CSI: creating VSR to unmount volume-series [%s]", o.args.VolumeID)
		err = o.ops.createUnmountVSR(ctx, []string{com.VolReqOpUnmount})
	case FsAttachedState:
		o.app.Log.Debugf("CSI: creating VSR to unmount and detach fs for volume-series [%s]", o.args.VolumeID)
		err = o.ops.createUnmountVSR(ctx, []string{com.VolReqOpDetachFs, com.VolReqOpUnmount})
	}

	if err == nil {
		return o.ops.waitForVSR(ctx)
	}
	if e, ok := err.(*crud.Error); ok {
		if e.InConflict() {
			if o.ops.checkForVSRs(ctx) {
				o.app.Log.Debugf("CSI: VSR [%s] pending for volume-series [%s]", o.vsr.Meta.ID, o.args.VolumeID)
				return o.ops.waitForVSR(ctx)
			}
		}
	}
	o.app.Log.Debugf("CSI: Ignored VSR create error for volume-series [%s]: %s", o.args.VolumeID, err.Error())
	_, err = o.ops.volUnpublished(ctx, o.args.VolumeID)
	return err
}

func (o *unmountOp) checkForVSRs(ctx context.Context) bool {
	o.app.Log.Debugf("CSI: check for pending VSR of volume-series [%s]", o.args.VolumeID)
	params := &volume_series_request.VolumeSeriesRequestListParams{
		IsTerminated:   swag.Bool(false),
		VolumeSeriesID: &o.args.VolumeID,
		NodeID:         &o.nodeIdentifier,
	}
	vrl, err := o.app.OCrud.VolumeSeriesRequestList(ctx, params)
	if err != nil {
		o.app.Log.Errorf("CSI: volume-series-request list failed: %s", err.Error())
		return false
	}
	for _, vsr := range vrl.Payload {
		if util.Contains(vsr.RequestedOperations, com.VolReqOpUnmount) || util.Contains(vsr.RequestedOperations, com.VolReqOpDetachFs) {
			o.vsr = vsr
			return true
		}
	}
	return false
}

func (o *unmountOp) createUnmountVSR(ctx context.Context, ops []string) error {
	if err := o.c.rei.ErrOnBool("csi-unmount-vsr"); err != nil {
		o.app.Log.Errorf("CSI: rei unmount failure: %s", err.Error())
		return err
	}
	o.vsr = &models.VolumeSeriesRequest{}
	o.vsr.VolumeSeriesID = models.ObjIDMutable(o.vs.Meta.ID)
	o.vsr.NodeID = models.ObjIDMutable(o.nodeIdentifier)
	o.vsr.SnapIdentifier = ""
	o.vsr.RequestedOperations = ops
	o.vsr.CompleteByTime = strfmt.DateTime(time.Now().Add(5 * time.Minute))
	o.vsr.TargetPath = o.args.TargetPath
	vsr, err := o.app.OCrud.VolumeSeriesRequestCreate(ctx, o.vsr)
	if err != nil {
		return err
	}
	o.vsr = vsr
	return nil
}

func (o *unmountOp) waitForVSR(ctx context.Context) error {
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
		o.vsr, err = o.vW.WaitForVSR(ctx)
	}
	if err != nil {
		if err == vra.ErrRequestWaiterCtxExpired {
			o.app.Log.Errorf("CSI: waitForVSR: %s: volume-series-request [%s] %s", err.Error(), o.vsr.Meta.ID, o.vsr.VolumeSeriesRequestState)
			return status.Errorf(codes.DeadlineExceeded, "request [%s] in progress [%s]", o.vsr.Meta.ID, o.vsr.VolumeSeriesRequestState)
		}
		return status.Errorf(codes.Internal, "volume-series-request [%s] wait failed: %s", o.vsr.Meta.ID, err.Error())
	}
	if o.vsr.VolumeSeriesRequestState != com.VolReqStateSucceeded {
		return status.Errorf(codes.Internal, "volume-series-request [%s] %s", o.vsr.Meta.ID, o.vsr.VolumeSeriesRequestState)
	}
	_, err = o.ops.volUnpublished(ctx, o.args.VolumeID)
	return err
}

func (o *unmountOp) volUnpublished(ctx context.Context, vsID string) (vsMediaState, error) {
	var err error
	o.vs, err = o.app.OCrud.VolumeSeriesFetch(ctx, o.args.VolumeID)
	if err != nil {
		if e, ok := err.(*crud.Error); ok {
			if e.NotFound() {
				return InvalidState, status.Errorf(codes.NotFound, "volume-series [%s] not found", vsID)
			}
		} else {
			return InvalidState, status.Errorf(codes.Internal, "volume-series [%s] fetch failed: %s", vsID, err.Error())
		}
	}
	if string(o.vs.BoundClusterID) != o.clusterIdentifier {
		return InvalidState, status.Errorf(codes.Internal, "volume-series [%s] not bound to cluster [%s]", vsID, o.clusterIdentifier)
	}
	if o.vs.VolumeSeriesState == com.VolStateInUse && vra.VolumeSeriesHeadIsMounted(o.vs) {
		if vra.VolumeSeriesFsIsAttached(o.vs) {
			return FsAttachedState, status.Errorf(codes.Internal, "volume-series [%s] fs is still attached", o.args.VolumeID)
		}
		o.app.Log.Debugf("CSI: volume-series [%s] is mounted", o.vs.Meta.ID)
		return MountedState, status.Errorf(codes.Internal, "volume-series [%s] is still mounted", o.args.VolumeID)
	}
	return NotMountedState, nil
}
