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


package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/snapshot"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

// Register handlers for VolumeSeriesRequest
func (c *HandlerComp) volumeSeriesRequestRegisterHandlers() {
	c.app.API.VolumeSeriesRequestVolumeSeriesRequestCancelHandler = ops.VolumeSeriesRequestCancelHandlerFunc(c.volumeSeriesRequestCancel)
	c.app.API.VolumeSeriesRequestVolumeSeriesRequestCreateHandler = ops.VolumeSeriesRequestCreateHandlerFunc(c.volumeSeriesRequestCreate)
	c.app.API.VolumeSeriesRequestVolumeSeriesRequestDeleteHandler = ops.VolumeSeriesRequestDeleteHandlerFunc(c.volumeSeriesRequestDelete)
	c.app.API.VolumeSeriesRequestVolumeSeriesRequestFetchHandler = ops.VolumeSeriesRequestFetchHandlerFunc(c.volumeSeriesRequestFetch)
	c.app.API.VolumeSeriesRequestVolumeSeriesRequestListHandler = ops.VolumeSeriesRequestListHandlerFunc(c.volumeSeriesRequestList)
	c.app.API.VolumeSeriesRequestVolumeSeriesRequestUpdateHandler = ops.VolumeSeriesRequestUpdateHandlerFunc(c.volumeSeriesRequestUpdate)
}

var nmVolumeSeriesRequestMutable JSONToAttrNameMap

func (c *HandlerComp) volumeSeriesRequestMutableNameMap() JSONToAttrNameMap {
	if nmVolumeSeriesRequestMutable == nil {
		c.cacheMux.Lock()
		defer c.cacheMux.Unlock()
		if nmVolumeSeriesRequestMutable == nil {
			nmVolumeSeriesRequestMutable = c.makeJSONToAttrNameMap(M.VolumeSeriesRequestMutable{})
		}
	}
	return nmVolumeSeriesRequestMutable
}

var volumeSeriesRequestOperationSequences = map[string]int{
	com.VolReqOpCreate:             0,
	com.VolReqOpBind:               0,
	com.VolReqOpMount:              0,
	com.VolReqOpPublish:            0,
	com.VolReqOpVolRestoreSnapshot: 0,
	com.VolReqOpAttachFs:           0,
	com.VolReqOpRename:             1,
	com.VolReqOpCreateFromSnapshot: 2,
	com.VolReqOpChangeCapacity:     3,
	com.VolReqOpDetachFs:           9,
	com.VolReqOpUnmount:            9,
	com.VolReqOpUnpublish:          9,
	com.VolReqOpDelete:             9,
	com.VolReqOpAllocateCapacity:   12,
	com.VolReqOpDeleteSPA:          13,
	com.VolReqOpCGCreateSnapshot:   17,
	com.VolReqOpConfigure:          19,
	com.VolReqOpVolCreateSnapshot:  19,
	com.VolReqOpUnbind:             20,
	com.VolReqOpNodeDelete:         30,
	com.VolReqOpVolDetach:          40,
}

// Table of permitted overlapping operations (in different VSRs) on the same object
var vsrOverlappingOps = map[string][]string{}

func opsCanOverlap(op1, op2 string) {
	permit := func(o1, o2 string) {
		v := vsrOverlappingOps[o1]
		v = append(v, o2)
		vsrOverlappingOps[o1] = v
	}
	permit(op1, op2)
	permit(op2, op1)
}

func opsOverlapOk(op1, op2 string) bool {
	return util.Contains(vsrOverlappingOps[op1], op2)
}

func init() {
	opsCanOverlap(com.VolReqOpCreateFromSnapshot, com.VolReqOpBind)
	opsCanOverlap(com.VolReqOpCreateFromSnapshot, com.VolReqOpCreateFromSnapshot)
	opsCanOverlap(com.VolReqOpCreateFromSnapshot, com.VolReqOpConfigure)
	opsCanOverlap(com.VolReqOpCreateFromSnapshot, com.VolReqOpMount)
	opsCanOverlap(com.VolReqOpCreateFromSnapshot, com.VolReqOpPublish)
	opsCanOverlap(com.VolReqOpCreateFromSnapshot, com.VolReqOpAttachFs)
	opsCanOverlap(com.VolReqOpCreateFromSnapshot, com.VolReqOpRename)
	opsCanOverlap(com.VolReqOpCreateFromSnapshot, com.VolReqOpDetachFs)
	opsCanOverlap(com.VolReqOpCreateFromSnapshot, com.VolReqOpUnmount)
	opsCanOverlap(com.VolReqOpCreateFromSnapshot, com.VolReqOpUnpublish)
	opsCanOverlap(com.VolReqOpCreateFromSnapshot, com.VolReqOpVolCreateSnapshot)
	opsCanOverlap(com.VolReqOpCreateFromSnapshot, com.VolReqOpVolRestoreSnapshot)
	opsCanOverlap(com.VolReqOpChangeCapacity, com.VolReqOpVolCreateSnapshot)
	// TBD: VOL_SNAPSHOT_CREATE, {GROW|PROVISION} can overlap
	// TBD: VOL_SNAPSHOT_RESTORE, SNAPSHOT_DELETE can overlap
}

type volumeSeriesOpsParsed struct {
	hasAllocateCapacity, hasDeleteSPA, hasChangeCapacity, hasUnbind                                                 bool
	hasCreate, hasCreateFromSnapshot, hasBind, hasMount, hasRename, hasUnmount, hasUnpublish, hasDelete, hasPublish bool
	hasConfigure, hasCGCreateSnapshot, hasVolCreateSnapshot, hasVolRestoreSnapshot, hasAttachFs, hasDetachFs        bool
	hasNodeDelete, hasVolDetach                                                                                     bool
	canonicalOrder                                                                                                  []string
}

func newVSOP(operations []string) *volumeSeriesOpsParsed {
	sop := &volumeSeriesOpsParsed{
		hasAllocateCapacity:   util.Contains(operations, com.VolReqOpAllocateCapacity),
		hasAttachFs:           util.Contains(operations, com.VolReqOpAttachFs),
		hasBind:               util.Contains(operations, com.VolReqOpBind),
		hasCGCreateSnapshot:   util.Contains(operations, com.VolReqOpCGCreateSnapshot),
		hasChangeCapacity:     util.Contains(operations, com.VolReqOpChangeCapacity),
		hasConfigure:          util.Contains(operations, com.VolReqOpConfigure),
		hasCreate:             util.Contains(operations, com.VolReqOpCreate),
		hasCreateFromSnapshot: util.Contains(operations, com.VolReqOpCreateFromSnapshot),
		hasDelete:             util.Contains(operations, com.VolReqOpDelete),
		hasDeleteSPA:          util.Contains(operations, com.VolReqOpDeleteSPA),
		hasDetachFs:           util.Contains(operations, com.VolReqOpDetachFs),
		hasMount:              util.Contains(operations, com.VolReqOpMount),
		hasNodeDelete:         util.Contains(operations, com.VolReqOpNodeDelete),
		hasPublish:            util.Contains(operations, com.VolReqOpPublish),
		hasRename:             util.Contains(operations, com.VolReqOpRename),
		hasUnbind:             util.Contains(operations, com.VolReqOpUnbind),
		hasUnmount:            util.Contains(operations, com.VolReqOpUnmount),
		hasUnpublish:          util.Contains(operations, com.VolReqOpUnpublish),
		hasVolCreateSnapshot:  util.Contains(operations, com.VolReqOpVolCreateSnapshot),
		hasVolDetach:          util.Contains(operations, com.VolReqOpVolDetach),
		hasVolRestoreSnapshot: util.Contains(operations, com.VolReqOpVolRestoreSnapshot),
	}
	// normalize the requested operation order
	reqOps := make([]string, 0, len(operations))
	if sop.hasAllocateCapacity {
		reqOps = append(reqOps, com.VolReqOpAllocateCapacity)
	}
	if sop.hasDeleteSPA {
		reqOps = append(reqOps, com.VolReqOpDeleteSPA)
	}
	if sop.hasCreate {
		reqOps = append(reqOps, com.VolReqOpCreate)
	}
	if sop.hasBind {
		reqOps = append(reqOps, com.VolReqOpBind)
	}
	if sop.hasPublish {
		reqOps = append(reqOps, com.VolReqOpPublish)
	}
	if sop.hasMount {
		reqOps = append(reqOps, com.VolReqOpMount)
	}
	if sop.hasVolRestoreSnapshot {
		reqOps = append(reqOps, com.VolReqOpVolRestoreSnapshot)
	}
	if sop.hasAttachFs {
		reqOps = append(reqOps, com.VolReqOpAttachFs)
	}
	if sop.hasRename {
		reqOps = append(reqOps, com.VolReqOpRename)
	}
	if sop.hasCreateFromSnapshot {
		reqOps = append(reqOps, com.VolReqOpCreateFromSnapshot)
	}
	if sop.hasChangeCapacity {
		reqOps = append(reqOps, com.VolReqOpChangeCapacity)
	}
	if sop.hasDetachFs {
		reqOps = append(reqOps, com.VolReqOpDetachFs)
	}
	if sop.hasUnmount {
		reqOps = append(reqOps, com.VolReqOpUnmount)
	}
	if sop.hasUnpublish {
		reqOps = append(reqOps, com.VolReqOpUnpublish)
	}
	if sop.hasDelete {
		reqOps = append(reqOps, com.VolReqOpDelete)
	}
	if sop.hasCGCreateSnapshot {
		reqOps = append(reqOps, com.VolReqOpCGCreateSnapshot)
	}
	if sop.hasConfigure {
		reqOps = append(reqOps, com.VolReqOpConfigure)
	}
	if sop.hasVolCreateSnapshot {
		reqOps = append(reqOps, com.VolReqOpVolCreateSnapshot)
	}
	if sop.hasUnbind {
		reqOps = append(reqOps, com.VolReqOpUnbind)
	}
	if sop.hasNodeDelete {
		reqOps = append(reqOps, com.VolReqOpNodeDelete)
	}
	if sop.hasVolDetach {
		reqOps = append(reqOps, com.VolReqOpVolDetach)
	}
	sop.canonicalOrder = reqOps
	return sop
}

// volumeSeriesRequestValidateCreator validates the Creator, setting its values from the auth.Info if present
// payload.VolumeSeriesCreateSpec must be set and validated by caller
func (c *HandlerComp) volumeSeriesRequestValidateCreator(ctx context.Context, ai *auth.Info, payload *M.VolumeSeriesRequestCreateArgs) error {
	if payload.Creator == nil {
		payload.Creator = &M.Identity{}
	}
	// always use auth identity as the creator when specified
	if ai.AccountID != "" {
		payload.Creator.AccountID = M.ObjIDMutable(ai.AccountID)
		payload.Creator.TenantAccountID = M.ObjID(ai.TenantAccountID)
	} else if payload.Creator.AccountID != "" {
		// assume access control checks were already performed, just ensure creator is consistent (must be tenant admin or subordinate)
		if payload.Creator.AccountID == payload.VolumeSeriesCreateSpec.AccountID {
			payload.Creator.TenantAccountID = M.ObjID(payload.VolumeSeriesCreateSpec.TenantAccountID)
		} else if payload.Creator.AccountID == payload.VolumeSeriesCreateSpec.TenantAccountID {
			payload.Creator.TenantAccountID = ""
		} else {
			return c.eMissingMsg("creator.accountId")
		}
	}
	var err error
	if ai.UserID != "" {
		payload.Creator.UserID = M.ObjIDMutable(ai.UserID)
	} else if payload.Creator.UserID != "" {
		if payload.Creator.AccountID == "" {
			err = c.eMissingMsg("creator.userId specified without creator.accountId")
		} else {
			var aObj *M.Account
			aObj, err = c.DS.OpsAccount().Fetch(ctx, string(payload.Creator.AccountID))
			if err == nil {
				if _, ok := aObj.UserRoles[string(payload.Creator.UserID)]; !ok {
					err = c.eMissingMsg("creator.userId has no role in the specified account")
				}
			}
		}
	}
	return err
}

// volumeSeriesRequestValidateRequestedOperations will parse, validate and canonicalize the requested operations
func (c *HandlerComp) volumeSeriesRequestValidateRequestedOperations(operations []string) (*volumeSeriesOpsParsed, error) {
	lastSeq := -1
	for _, op := range operations {
		if seq, found := volumeSeriesRequestOperationSequences[op]; found {
			if lastSeq < 0 {
				lastSeq = seq
			}
			if seq != lastSeq {
				return nil, fmt.Errorf("invalid operation combination")
			}
		} else {
			return nil, fmt.Errorf("unsupported operation %s", op)
		}
	}
	return newVSOP(operations), nil
}

// checks for permissible concurrent volume operations
func (c *HandlerComp) volumeSeriesRequestCreateCheckConcurrentVolOps(vsID string, sop *volumeSeriesOpsParsed, list []*M.VolumeSeriesRequest) error {
	if len(list) == 0 {
		return nil
	}
	conflictingOps := ""
out:
	for _, vsr := range list {
		for _, op1 := range sop.canonicalOrder {
			for _, op2 := range vsr.RequestedOperations {
				if !opsOverlapOk(op1, op2) {
					conflictingOps = fmt.Sprintf("object %s request [%s] conflicts with [%s] in VolumeSeriesRequest %s", vsID, op1, op2, vsr.Meta.ID)
					break out
				}
			}
		}
	}
	if conflictingOps == "" {
		return nil
	}
	return c.eRequestInConflict(conflictingOps)
}

// volumeSeriesRequestFetchFilter verifies the caller has permission to fetch the object, returning an error if they do not.
// If they do, the properties of the object are filtered based on capabilities.
// The account IDs stored in the VolumeSeriesCreateSpec are used for this check rather than the Creator since the Creator
// could be the tenant or a subordinate but the visibility rules are independent of which account created the VSR.
func (c *HandlerComp) volumeSeriesRequestFetchFilter(ai *auth.Info, obj *M.VolumeSeriesRequest) error {
	err := ai.CapOK(centrald.VolumeSeriesOwnerCap, obj.VolumeSeriesCreateSpec.AccountID)
	if err != nil {
		err = ai.CapOK(centrald.CSPDomainManagementCap, obj.VolumeSeriesCreateSpec.TenantAccountID)
	}
	return err
}

// Handlers

func (c *HandlerComp) volumeSeriesRequestCancel(params ops.VolumeSeriesRequestCancelParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewVolumeSeriesRequestCancelDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.Lock() // take write lock to avoid version mismatch on update
	defer c.Unlock()
	obj, err := c.DS.OpsVolumeSeriesRequest().Fetch(ctx, params.ID)
	if err == nil {
		err = ai.CapOK(centrald.VolumeSeriesOwnerCap, obj.VolumeSeriesCreateSpec.AccountID)
		if err != nil {
			err = ai.CapOK(centrald.CSPDomainManagementCap, obj.VolumeSeriesCreateSpec.TenantAccountID)
		}
	}
	if err == nil && vra.VolumeSeriesRequestStateIsTerminated(obj.VolumeSeriesRequestState) {
		err = &centrald.Error{M: "VolumeSeriesRequest is already terminated", C: centrald.ErrorNotFound.C}
	}
	if err != nil {
		return ops.NewVolumeSeriesRequestCancelDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if !obj.CancelRequested {
		// skip update if the request is already canceled
		obj, err = c.DS.OpsVolumeSeriesRequest().Cancel(ctx, params.ID, int32(obj.Meta.Version))
		if err != nil {
			return ops.NewVolumeSeriesRequestCancelDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	c.Log.Infof("VolumeSeriesRequest canceled [%s]", obj.Meta.ID)
	c.setScopeVolumeSeriesRequest(params.HTTPRequest, obj)
	return ops.NewVolumeSeriesRequestCancelOK().WithPayload(obj)
}

func (c *HandlerComp) volumeSeriesRequestCreate(params ops.VolumeSeriesRequestCreateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err := centrald.ErrorMissing
		return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	// NB: autogen swagger code ensures requestedOperations are present, unique and at least 1 item.
	sop, err := c.volumeSeriesRequestValidateRequestedOperations(params.Payload.RequestedOperations)
	if err != nil {
		err = c.eMissingMsg("%s", err.Error())
		return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	params.Payload.RequestedOperations = sop.canonicalOrder // rewrite
	if !time.Time(params.Payload.CompleteByTime).IsZero() && !time.Time(params.Payload.CompleteByTime).After(time.Now()) {
		err = c.eMissingMsg("completeByTime is in the past")
		return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if !(sop.hasCreateFromSnapshot || sop.hasVolRestoreSnapshot) {
		params.Payload.Snapshot = nil // discard input
	}
	params.Payload.ProtectionDomainID = "" // discard input
	// helper to determine a mounted node with a preference for the head
	mountedNodePreferHead := func(vsObj *M.VolumeSeries) (M.ObjIDMutable, bool) {
		var nodeID M.ObjIDMutable
		for _, m := range vsObj.Mounts {
			if m.MountState == com.VolMountStateMounted { // IN_USE does not guarantee MOUNTED
				if m.SnapIdentifier == com.VolMountHeadIdentifier {
					return m.MountedNodeID, true // head node
				}
				nodeID = m.MountedNodeID // a PiT node
			}
		}
		return nodeID, false
	}
	// helper to determine if the head is mounted
	headIsMounted := func(vsObj *M.VolumeSeries) bool {
		return vra.VolumeSeriesHeadIsMounted(vsObj)
	}
	// helper to determine if syncCoordinatorId is valid
	syncCoordinatorIsValid := func(validOps []string) (*M.VolumeSeriesRequest, error) {
		vsr, err := c.DS.OpsVolumeSeriesRequest().Fetch(ctx, string(params.Payload.SyncCoordinatorID))
		if err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eMissingMsg("invalid syncCoordinatorId")
			}
			return nil, err
		}
		if !util.Contains(validOps, vsr.RequestedOperations[0]) {
			err = c.eMissingMsg("syncCoordinatorId not in %v", validOps)
			return nil, err
		}
		return vsr, nil
	}
	// helper to load cluster object
	loadClusterObject := func(clusterID M.ObjIDMutable) (*M.Cluster, error) {
		clObj, err := c.DS.OpsCluster().Fetch(ctx, string(clusterID))
		if err == nil {
			err = c.clusterFetchFilter(ai, clObj)
		}
		if err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eMissingMsg("invalid clusterId")
			}
			return nil, err
		}
		return clObj, nil
	}
	// helper to load node object
	loadNodeObject := func(nodeID M.ObjIDMutable) (*M.Node, error) {
		nObj, err := c.DS.OpsNode().Fetch(ctx, string(nodeID))
		if err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eMissingMsg("invalid nodeId")
			}
			return nil, err
		}
		return nObj, nil
	}
	// enter critical section: write lock required due to query for other volume series requests
	c.Lock()
	defer c.Unlock()
	if err = c.app.AuditLog.Ready(); err != nil {
		return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var vsObj *M.VolumeSeries
	var nObj *M.Node
	if !sop.hasDeleteSPA {
		params.Payload.ServicePlanAllocationID = "" // TBD: Zap servicePlanAllocationId on create for now.
	}
	mustLoadVolume := false
	mustLoadVolumeAccount := false
	if sop.hasCreate {
		// access control also checked in volumeSeriesValidateCreateArgs
		if _, _, err = c.volumeSeriesValidateCreateArgs(ctx, ai, params.Payload.VolumeSeriesCreateSpec); err != nil {
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if len(params.Payload.ApplicationGroupIds) > 0 {
			if params.Payload.VolumeSeriesCreateSpec.ConsistencyGroupID != "" {
				err = c.eMissingMsg("applicationGroupIds must not be specified with volumeSeriesCreateSpec.consistencyGroupID")
				return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			if _, err = c.validateApplicationGroupIds(ctx, params.Payload.VolumeSeriesCreateSpec.AccountID, params.Payload.ApplicationGroupIds); err != nil {
				return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
		}
		vlParams := volume_series.VolumeSeriesListParams{
			Name:      swag.String(string(params.Payload.VolumeSeriesCreateSpec.Name)),
			AccountID: swag.String(string(params.Payload.VolumeSeriesCreateSpec.AccountID)),
		}
		vsCount, err := c.DS.OpsVolumeSeries().Count(ctx, vlParams, 1)
		if err != nil {
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if vsCount != 0 {
			err = c.eExists("'%s' already exists", params.Payload.VolumeSeriesCreateSpec.Name)
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		vrlParams := ops.VolumeSeriesRequestListParams{
			IsTerminated: swag.Bool(false),
			AccountID:    swag.String(string(params.Payload.VolumeSeriesCreateSpec.AccountID)),
		}
		vsrList, err := c.DS.OpsVolumeSeriesRequest().List(ctx, vrlParams)
		if err != nil {
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		for _, vsr := range vsrList {
			if util.Contains(vsr.RequestedOperations, com.VolReqOpCreate) &&
				vsr.VolumeSeriesCreateSpec.Name == params.Payload.VolumeSeriesCreateSpec.Name {
				err = c.eRequestInConflict("pending CREATE for '%s'", params.Payload.VolumeSeriesCreateSpec.Name)
				return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
		}
	} else if sop.hasAllocateCapacity {
		if params.Payload.ServicePlanAllocationCreateSpec == nil {
			err = c.eMissingMsg("missing servicePlanAllocationCreateSpec")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		spaSpec := params.Payload.ServicePlanAllocationCreateSpec
		spaObj := &M.ServicePlanAllocation{
			ServicePlanAllocationCreateOnce: spaSpec.ServicePlanAllocationCreateOnce,
			ServicePlanAllocationMutable: M.ServicePlanAllocationMutable{
				ServicePlanAllocationCreateMutable: spaSpec.ServicePlanAllocationCreateMutable,
			},
		}
		// access control also checked in servicePlanAllocationCreateValidate
		if _, _, _, err := c.spaValidator.servicePlanAllocationCreateValidate(ctx, ai, spaObj); err != nil {
			if err != centrald.ErrorUnauthorizedOrForbidden && c.eCode(err) < 500 {
				err = c.eMissingMsg("invalid servicePlanAllocationCreateSpec: %s", err.Error())
			}
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		// VolumeSeriesCreateSpec not expected in this case. Set TenantAccountID and AccountID from the spaSpec where
		// the AccountID corresponds to the tenant, the AuthorizedAccountID is either the tenant or a subordinate.
		// In VolumeSeriesCreateSpec, TenantAccountID is only set when the AccountID is that of a subordinate.
		params.Payload.VolumeSeriesCreateSpec = &M.VolumeSeriesCreateArgs{}
		params.Payload.VolumeSeriesCreateSpec.AccountID = spaSpec.AuthorizedAccountID
		if spaSpec.AccountID != spaSpec.AuthorizedAccountID {
			params.Payload.VolumeSeriesCreateSpec.TenantAccountID = spaSpec.AccountID
		}
		params.Payload.ClusterID = spaSpec.ClusterID
	} else if sop.hasCGCreateSnapshot {
		if params.Payload.ConsistencyGroupID == "" || params.Payload.ClusterID == "" {
			err = c.eMissingMsg("require consistencyGroupId and clusterId")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		cgObj, err := c.DS.OpsConsistencyGroup().Fetch(ctx, string(params.Payload.ConsistencyGroupID))
		if err == nil {
			err = ai.CapOK(centrald.VolumeSeriesOwnerCap, cgObj.AccountID)
		}
		if err != nil {
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		params.Payload.VolumeSeriesID = "" // discard input
		params.Payload.VolumeSeriesCreateSpec = &M.VolumeSeriesCreateArgs{}
		params.Payload.VolumeSeriesCreateSpec.AccountID = cgObj.AccountID
		params.Payload.VolumeSeriesCreateSpec.TenantAccountID = cgObj.TenantAccountID
	} else if sop.hasCreateFromSnapshot {
		snap := params.Payload.Snapshot
		if params.Payload.SnapshotID != "" {
			snap, err = c.ops.intSnapshotFetchData(ctx, ai, string(params.Payload.SnapshotID))
			if err != nil {
				err = c.eMissingMsg("invalid snapshotID: %s", err.Error())
				return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			params.Payload.Snapshot = snap
			params.Payload.ProtectionDomainID = snap.ProtectionDomainID
		} else if snap == nil || snap.VolumeSeriesID == "" || snap.SnapIdentifier == "" || params.Payload.VolumeSeriesCreateSpec == nil || params.Payload.NodeID == "" {
			err = c.eMissingMsg("require snapshot.volumeSeriesId, snapshot.snapIdentifier, volumeSeriesCreateSpec and nodeId")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		params.Payload.VolumeSeriesID = snap.VolumeSeriesID
		mustLoadVolume = true
	} else if sop.hasDeleteSPA {
		spa, err := c.DS.OpsServicePlanAllocation().Fetch(ctx, string(params.Payload.ServicePlanAllocationID))
		if err == nil {
			err = ai.CapOK(centrald.CSPDomainManagementCap, spa.AccountID)
		}
		if err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eMissingMsg("invalid servicePlanAllocationId")
			}
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		// To simplify the VSR handler, set fields found in the SPA into the spaSpec, but with 0 TotalCapacityBytes
		// The ServicePlanAllocationID is not required once spaSpec is set, so reset it
		spaSpec := &M.ServicePlanAllocationCreateArgs{}
		spaSpec.ServicePlanAllocationCreateOnce = spa.ServicePlanAllocationCreateOnce
		spaSpec.ServicePlanAllocationCreateMutable = spa.ServicePlanAllocationCreateMutable
		spaSpec.TotalCapacityBytes = swag.Int64(0)
		params.Payload.ServicePlanAllocationCreateSpec = spaSpec
		params.Payload.ServicePlanAllocationID = ""
		// VolumeSeriesCreateSpec not expected in this case. Set TenantAccountID and AccountID from the SPA where
		// the AccountID corresponds to the tenant, the AuthorizedAccountID is either the tenant or a subordinate.
		// In VolumeSeriesCreateSpec, TenantAccountID is only set when the AccountID is that of a subordinate.
		params.Payload.VolumeSeriesCreateSpec = &M.VolumeSeriesCreateArgs{}
		params.Payload.VolumeSeriesCreateSpec.AccountID = spa.AuthorizedAccountID
		if spa.AccountID != spa.AuthorizedAccountID {
			params.Payload.VolumeSeriesCreateSpec.TenantAccountID = spa.AccountID
		}
	} else if sop.hasChangeCapacity {
		vcs := params.Payload.VolumeSeriesCreateSpec
		if params.Payload.VolumeSeriesID == "" || vcs == nil || (vcs.SizeBytes == nil && vcs.SpaAdditionalBytes == nil) {
			err = c.eMissingMsg("require volumeSeriesId and volumeSeriesCreateSpec.{sizeBytes and/or spaAdditionalBytes}")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		mustLoadVolume = true
	} else if sop.hasVolCreateSnapshot || sop.hasMount {
		mustLoadVolume = true
		mustLoadVolumeAccount = true
	} else if sop.hasNodeDelete || sop.hasVolDetach {
		if err = ai.InternalOK(); err != nil {
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if params.Payload.NodeID == "" {
			err = c.eMissingMsg("nodeId is required")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		nObj, err = loadNodeObject(params.Payload.NodeID)
		if err != nil {
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if (sop.hasNodeDelete && !util.Contains(vra.ValidNodeStatesForNodeDelete(), nObj.State)) || (sop.hasVolDetach && !util.Contains(vra.ValidNodeStatesForVolDetach(), nObj.State)) {
			err = c.eMissingMsg("invalid node state: %s", nObj.State)
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if sop.hasNodeDelete {
			// check for duplicate NODE_DELETE VSRs
			vrlParams := ops.VolumeSeriesRequestListParams{
				IsTerminated: swag.Bool(false),
				NodeID:       swag.String(string(params.Payload.NodeID)),
			}
			vsrList, err := c.DS.OpsVolumeSeriesRequest().List(ctx, vrlParams)
			if err != nil {
				return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			for _, vsr := range vsrList {
				if util.Contains(vsr.RequestedOperations, com.VolReqOpNodeDelete) {
					err = c.eRequestInConflict("pending NODE_DELETE VSR for node %s", params.Payload.NodeID)
					return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
				}
			}
			// VolumeSeriesCreateSpec not expected in this case.
			// Set AccountID from the nObj where the AccountID corresponds to the tenant
			params.Payload.VolumeSeriesCreateSpec = &M.VolumeSeriesCreateArgs{}
			params.Payload.VolumeSeriesCreateSpec.AccountID = nObj.AccountID
		}
		if sop.hasVolDetach {
			if params.Payload.SyncCoordinatorID == "" || params.Payload.VolumeSeriesID == "" {
				err = c.eMissingMsg("syncCoordinatorID and volumeSeriesID must be specified")
				return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			// sync coordinator must be a NODE_DELETE VSR
			if _, err := syncCoordinatorIsValid([]string{com.VolReqOpNodeDelete}); err != nil {
				return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			mustLoadVolume = true
		}
		params.Payload.CompleteByTime = strfmt.DateTime(util.DateTimeMaxUpperBound()) // force
	} else {
		mustLoadVolume = true
	}
	if mustLoadVolume {
		vsID := string(params.Payload.VolumeSeriesID)
		vsObj, err = c.DS.OpsVolumeSeries().Fetch(ctx, vsID)
		if err == nil {
			err = ai.CapOK(centrald.VolumeSeriesOwnerCap, vsObj.AccountID)
		}
		if err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eMissingMsg("invalid volumeSeriesId")
			}
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		list, err := c.DS.OpsVolumeSeriesRequest().List(ctx, ops.VolumeSeriesRequestListParams{VolumeSeriesID: &vsID, IsTerminated: swag.Bool(false)})
		if err != nil {
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if err = c.volumeSeriesRequestCreateCheckConcurrentVolOps(vsID, sop, list); err != nil {
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if params.Payload.VolumeSeriesCreateSpec == nil {
			params.Payload.VolumeSeriesCreateSpec = &M.VolumeSeriesCreateArgs{}
		}
		params.Payload.VolumeSeriesCreateSpec.AccountID = vsObj.AccountID
		params.Payload.VolumeSeriesCreateSpec.TenantAccountID = vsObj.TenantAccountID
	}
	var vsAccountObj *M.Account
	if mustLoadVolumeAccount || (sop.hasCreate && sop.hasMount) {
		accountID := string(params.Payload.VolumeSeriesCreateSpec.AccountID)
		vsAccountObj, err = c.ops.intAccountFetch(ctx, ai, accountID)
		if err != nil {
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	var clObj *M.Cluster
	if sop.hasBind {
		if !sop.hasCreate && vsObj.VolumeSeriesState != com.VolStateUnbound {
			err = c.eMissingMsg("BIND requires UNBOUND volumeSeriesState")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		clObj, err = loadClusterObject(params.Payload.ClusterID)
		if err != nil {
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if sop.hasPublish {
		if !sop.hasBind {
			if !vra.VolumeSeriesIsBound(vsObj.VolumeSeriesState) {
				err = c.eMissingMsg("PUBLISH requires BOUND volumeSeriesState")
				return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			// cache clusterId in the request
			params.Payload.ClusterID = vsObj.BoundClusterID
			clObj, err = loadClusterObject(params.Payload.ClusterID)
			if err != nil {
				return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
		}
		cl := c.lookupClusterClient(clObj.ClusterType)
		if cl == nil {
			err = c.eMissingMsg("Cluster type '%s' not found", clObj.ClusterType)
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if (params.Payload.DriverType != "" && !util.Contains(cl.GetDriverTypes(), params.Payload.DriverType)) ||
			(params.Payload.FsType != "" && !util.Contains(cl.GetFileSystemTypes(), params.Payload.FsType)) {
			err = c.eMissingMsg("invalid fsType or driverType")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if sop.hasMount || sop.hasVolCreateSnapshot {
		var cspID M.ObjIDMutable
		if clObj != nil {
			cspID = clObj.CspDomainID
		} else if vsObj != nil {
			cspID = vsObj.BoundCspDomainID
		}
		params.Payload.ProtectionDomainID = c.accountProtectionDomainForProtectionStore(vsAccountObj, cspID)
		if params.Payload.ProtectionDomainID == "" {
			err = c.eMissingMsg("VOL_SNAPSHOT_CREATE and MOUNT require protectionDomainId")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if vsAccountObj.SnapshotCatalogPolicy == nil || vsAccountObj.SnapshotCatalogPolicy.CspDomainID == "" || vsAccountObj.SnapshotCatalogPolicy.ProtectionDomainID == "" {
			err = c.eMissingMsg("invalid snapshotCatalogPolicy")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if sop.hasMount { // no support currently for 2nd mount of with different snapIdentifier - would impact VOL_SNAPSHOT_RESTORE
		if !sop.hasBind {
			if vsObj == nil || !util.Contains([]string{com.VolStateBound, com.VolStateProvisioned, com.VolStateConfigured}, vsObj.VolumeSeriesState) {
				err = c.eMissingMsg("MOUNT requires BOUND, PROVISIONED or CONFIGURED volumeSeriesState")
				return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			// cache clusterId in the request
			params.Payload.ClusterID = vsObj.BoundClusterID
		}
		if params.Payload.SnapIdentifier != "" && params.Payload.SnapIdentifier != com.VolMountHeadIdentifier {
			err = c.eMissingMsg("invalid snapIdentifier")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if sop.hasConfigure {
		if vsObj == nil || vsObj.VolumeSeriesState != com.VolStateProvisioned {
			err = c.eMissingMsg("CONFIGURE requires a PROVISIONED volumeSeries")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		// cache clusterId in the request
		params.Payload.ClusterID = vsObj.BoundClusterID
	}
	if sop.hasMount || sop.hasConfigure {
		nObj, err = loadNodeObject(params.Payload.NodeID)
		if err != nil {
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if nObj.ClusterID != params.Payload.ClusterID {
			err = c.eMissingMsg("node %s is not a member of the bound cluster of the volumeSeries", nObj.Name)
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if sop.hasRename {
		if params.Payload.VolumeSeriesCreateSpec == nil || params.Payload.VolumeSeriesCreateSpec.Name == "" {
			err = c.eMissingMsg("volumeSeriesCreateSpec.name is required")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if params.Payload.VolumeSeriesCreateSpec.Name == vsObj.Name {
			err = c.eMissingMsg("name is the same as existing volumeSeries name")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if sop.hasCreateFromSnapshot {
		cSpec := params.Payload.VolumeSeriesCreateSpec
		if params.Payload.SnapshotID != "" { // SnapshotData fetched
			cSpec.SizeBytes = params.Payload.Snapshot.SizeBytes
		} else {
			vsID := string(params.Payload.Snapshot.VolumeSeriesID)
			// fetch snapshot object using snapshot data from payload
			lParams := snapshot.NewSnapshotListParams()
			lParams.SnapIdentifier = &params.Payload.Snapshot.SnapIdentifier
			lParams.VolumeSeriesID = &vsID
			sd, err := c.DS.OpsSnapshot().List(ctx, lParams)
			if err != nil {
				return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			if len(sd) != 1 || len(sd[0].Locations) == 0 || sd[0].SizeBytes == 0 {
				// there should not really be more than one snapshot with given snapID and vsID as their combination should be unique
				err = c.eMissingMsg("invalid snapshot.snapIdentifier")
				return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			cSpec.SizeBytes = swag.Int64(sd[0].SizeBytes)
			params.Payload.ProtectionDomainID = sd[0].ProtectionDomainID
		}
		cSpec.AccountID = vsObj.AccountID
		if cSpec.ServicePlanID == "" {
			cSpec.ServicePlanID = vsObj.ServicePlanID
		}
		if _, _, err = c.volumeSeriesValidateCreateArgs(ctx, ai, params.Payload.VolumeSeriesCreateSpec); err != nil {
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		nObj, err = loadNodeObject(params.Payload.NodeID)
		if err != nil {
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		params.Payload.ClusterID = nObj.ClusterID
	}
	if sop.hasAttachFs {
		if params.Payload.TargetPath == "" {
			err = c.eMissingMsg("ATTACH_FS requires targetPath")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if sop.hasCreate && !sop.hasMount {
			err = c.eMissingMsg("ATTACH_FS requires a mounted volume")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if vsObj != nil && !sop.hasMount {
			nodeID, isHead := mountedNodePreferHead(vsObj)
			if vsObj.VolumeSeriesState != com.VolStateInUse || !isHead {
				err = c.eMissingMsg("ATTACH_FS requires the HEAD to be mounted")
				return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			if vra.VolumeSeriesFsIsAttached(vsObj) {
				err = c.eMissingMsg("ATTACH_FS requires filesystem to not be attached")
				return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			// add these IDs to the request so it will be routed correctly
			params.Payload.ClusterID = vsObj.BoundClusterID
			params.Payload.NodeID = nodeID
		}
	}
	if sop.hasDetachFs {
		if params.Payload.TargetPath == "" {
			err = c.eMissingMsg("DETACH_FS requires targetPath")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		nodeID, isHead := mountedNodePreferHead(vsObj)
		if vsObj.VolumeSeriesState != com.VolStateInUse || !isHead {
			err = c.eMissingMsg("DETACH_FS requires HEAD mounted")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if !vra.VolumeSeriesFsIsAttached(vsObj) {
			err = c.eMissingMsg("DETACH_FS requires filesystem to be attached")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		// add these IDs to the request so it will be routed correctly
		params.Payload.ClusterID = vsObj.BoundClusterID
		params.Payload.NodeID = nodeID
	}
	if sop.hasUnmount {
		if vsObj.VolumeSeriesState != com.VolStateInUse {
			err = c.eMissingMsg("UNMOUNT requires IN_USE volumeSeriesState")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if params.Payload.SnapIdentifier != "" && params.Payload.SnapIdentifier != com.VolMountHeadIdentifier {
			err = c.eMissingMsg("invalid snapIdentifier")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if !sop.hasDetachFs && vra.VolumeSeriesFsIsAttached(vsObj) {
			err = c.eMissingMsg("UNMOUNT requires filesystem to not be attached")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		nObj, err = loadNodeObject(params.Payload.NodeID)
		if err != nil {
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		snap := params.Payload.SnapIdentifier
		if snap == "" {
			snap = com.VolMountHeadIdentifier
		}
		found := false
		for _, m := range vsObj.Mounts {
			if m.MountedNodeID == params.Payload.NodeID && m.SnapIdentifier == snap {
				found = true
				break
			}
		}
		if !found {
			err = c.eMissingMsg("specified snapIdentifier is not mounted on specified node")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		// add clusterId to the request, if present
		params.Payload.ClusterID = vsObj.BoundClusterID
	}
	if sop.hasUnpublish {
		if !vra.VolumeSeriesIsPublished(vsObj) {
			err = c.eMissingMsg("specified volumeSeries is not published")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if !sop.hasUnmount && headIsMounted(vsObj) {
			err = c.eMissingMsg("specified volumeSeries is still in use")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		params.Payload.ClusterID = vsObj.BoundClusterID
	}
	if sop.hasDelete {
		if !sop.hasUnmount && vsObj.VolumeSeriesState == com.VolStateInUse {
			err = c.eMissingMsg("specified volumeSeries is still in use")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if sop.hasUnbind {
		if vsObj.VolumeSeriesState == com.VolStateProvisioned {
			if vsObj.LifecycleManagementData != nil && vsObj.LifecycleManagementData.FinalSnapshotNeeded {
				err = c.eMissingMsg(com.ErrorFinalSnapNeeded)
				return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
		} else if vsObj.VolumeSeriesState != com.VolStateBound {
			err = c.eMissingMsg("invalid volumeSeriesState")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if sop.hasUnbind || sop.hasDelete {
		// add clusterId to the request
		params.Payload.ClusterID = vsObj.BoundClusterID
		if !sop.hasUnmount && vsObj.RootStorageID != "" { // hasUnmount not possible with hasUnbind
			// Find the node where the RootStorageID is attached, if any, and set that node in the request.
			// The node UUID is required so Storelandia can be told to destroy the volume
			stObj, err := c.DS.OpsStorage().Fetch(ctx, string(vsObj.RootStorageID))
			if err != nil {
				return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			if stObj.StorageState.AttachmentState == com.StgAttachmentStateAttached {
				params.Payload.NodeID = stObj.StorageState.AttachedNodeID
			}
		}
	}
	if sop.hasCGCreateSnapshot {
		clObj, err := c.DS.OpsCluster().Fetch(ctx, string(params.Payload.ClusterID))
		if err == nil {
			err = c.clusterFetchFilter(ai, clObj)
		}
		if err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eMissingMsg("invalid clusterId")
			}
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		lParams := ops.VolumeSeriesRequestListParams{
			ConsistencyGroupID: swag.String(string(params.Payload.ConsistencyGroupID)),
			ClusterID:          swag.String(string(params.Payload.ClusterID)),
			IsTerminated:       swag.Bool(false),
		}
		// check for outstanding snapshot operations directly or indirectly on the consistency group
		num, err := c.DS.OpsVolumeSeriesRequest().Count(ctx, lParams, 1)
		if err != nil {
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if num != 0 {
			err = c.eRequestInConflict("consistency group %s has active operations", params.Payload.ConsistencyGroupID)
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if sop.hasVolCreateSnapshot {
		if !util.Contains([]string{com.VolStateInUse, com.VolStateConfigured, com.VolStateProvisioned}, vsObj.VolumeSeriesState) {
			err = c.eMissingMsg("volume state expected to be IN_USE, CONFIGURED or PROVISIONED")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		// Creation time race conditions:
		// - if not in a CG snapshot context must ensure that no conflicting snapshot creation active for its CG
		// - if launched in a CG snapshot context but the Vol CG changes - this must be handled by the VSR animator
		if params.Payload.SyncCoordinatorID == "" {
			lParams := ops.VolumeSeriesRequestListParams{
				ConsistencyGroupID: swag.String(string(vsObj.ConsistencyGroupID)),
				IsTerminated:       swag.Bool(false),
			}
			list, err := c.DS.OpsVolumeSeriesRequest().List(ctx, lParams)
			if err != nil {
				return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			for _, vsr := range list {
				if vsr.RequestedOperations[0] == com.VolReqOpCGCreateSnapshot &&
					(vsr.ClusterID == "" || vsr.ClusterID == vsObj.BoundClusterID) { // any cluster or this volume's cluster
					err = c.eRequestInConflict("volumeSeriesRequest %s %s", com.VolReqOpCGCreateSnapshot, string(vsr.Meta.ID))
					return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
				}
			}
			params.Payload.SnapIdentifier = uuidGenerator() // random, distinct
		} else { // sync coordinator must be a CG_SNAPSHOT_CREATE VSR
			if _, err := syncCoordinatorIsValid([]string{com.VolReqOpCGCreateSnapshot}); err != nil {
				return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			// if vsObj.ConsistencyGroupID != vsr.ConsistencyGroupID {
			// cg change startup race - must be handled by the animator
			// }
			params.Payload.SnapIdentifier = string(params.Payload.SyncCoordinatorID) // pre-determined
		}
		// pick the appropriate node for this action (prefer HEAD but settle for a PiT) in the VSR
		var nodeID M.ObjIDMutable
		switch vsObj.VolumeSeriesState {
		case com.VolStateInUse:
			nodeID, _ = mountedNodePreferHead(vsObj)
			if nodeID == "" {
				err = c.eMissingMsg("volume node missing or not yet mounted")
				return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
		case com.VolStateConfigured:
			nodeID = vsObj.ConfiguredNodeID
		case com.VolStateProvisioned:
			nodeID = params.Payload.NodeID
			if !sop.hasConfigure {
				if nodeID == "" {
					err = c.eMissingMsg("PROVISIONED state requires nodeId")
					return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
				}
				nObj, err = loadNodeObject(params.Payload.NodeID)
				if err != nil {
					return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
				}
				if nObj.ClusterID != vsObj.BoundClusterID {
					err = c.eMissingMsg("volume not bound to cluster containing nodeId")
					return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
				}
			}
		}
		params.Payload.NodeID = nodeID                               // for crudE
		params.Payload.ClusterID = vsObj.BoundClusterID              // for crudE
		params.Payload.ConsistencyGroupID = vsObj.ConsistencyGroupID // to detect CG snapshot conflicts
		params.Payload.Snapshot = &M.SnapshotData{
			PitIdentifier: uuidGenerator(),
		}
	}
	if sop.hasVolRestoreSnapshot {
		if !sop.hasMount {
			if vsObj == nil || vsObj.VolumeSeriesState != com.VolStateInUse || !headIsMounted(vsObj) {
				err = c.eMissingMsg("VOL_SNAPSHOT_RESTORE requires IN_USE volumeSeriesState with the HEAD mounted")
				return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
		}
		snap := params.Payload.Snapshot
		if params.Payload.SnapshotID != "" {
			snap, err = c.ops.intSnapshotFetchData(ctx, ai, string(params.Payload.SnapshotID))
			if err != nil {
				err = c.eMissingMsg("invalid snapshotID: %s", err.Error())
				return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			params.Payload.Snapshot = snap
		} else if snap == nil || swag.Int64Value(snap.SizeBytes) == 0 || snap.VolumeSeriesID == "" || snap.SnapIdentifier == "" || len(snap.Locations) == 0 || snap.PitIdentifier == "" { // TBD: internal only?
			err = c.eMissingMsg("invalid snapshot")
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if !sop.hasCreate {
			if swag.Int64Value(vsObj.SizeBytes) != swag.Int64Value(snap.SizeBytes) {
				err = c.eMissingMsg("volume/snapshot size mismatch")
				return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
		}
		if params.Payload.SyncCoordinatorID != "" { // must be a CREATE_FROM_SNAPSHOT VSR
			if _, err := syncCoordinatorIsValid([]string{com.VolReqOpCreateFromSnapshot}); err != nil {
				return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
		}
		params.Payload.ProtectionDomainID = snap.ProtectionDomainID
	}
	if sop.hasChangeCapacity {
		err = nil
		vcs := params.Payload.VolumeSeriesCreateSpec
		chgSB := false
		chgSAB := false
		if vcs.SizeBytes != nil {
			chgSB = true
			sb := swag.Int64Value(vsObj.SizeBytes)
			nsb := swag.Int64Value(vcs.SizeBytes)
			if nsb <= sb {
				err = c.eMissingMsg("new sizeBytes must be greater than current sizeBytes")
			} else if vra.VolumeSeriesIsProvisioned(vsObj.VolumeSeriesState) { // TBD: relax this later
				err = c.eMissingMsg("volume cannot be provisioned when changing sizeBytes")
			}
		}
		if err == nil && vcs.SpaAdditionalBytes != nil {
			chgSAB = true
			if !vra.VolumeSeriesIsBound(vsObj.VolumeSeriesState) {
				err = c.eMissingMsg("volume must be bound when changing spaAdditionalBytes")
			} else {
				saB := swag.Int64Value(vsObj.SpaAdditionalBytes)
				nsaB := swag.Int64Value(vcs.SpaAdditionalBytes)
				if nsaB == saB {
					chgSAB = false
					vcs.SpaAdditionalBytes = nil // in case sizeBytes ok
				} else if vra.VolumeSeriesCacheIsRequested(vsObj) {
					// IDs needed for cache reallocation steps
					params.Payload.ClusterID = vsObj.BoundClusterID
					params.Payload.NodeID = vsObj.ConfiguredNodeID
				}
			}
		}
		if err == nil && !(chgSB || chgSAB) {
			err = c.eMissingMsg("no change requested in sizeBytes or spaAdditionalBytes")
		}
		if err != nil {
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if params.Payload.ConsistencyGroupID == "" && vsObj != nil {
		params.Payload.ConsistencyGroupID = vsObj.ConsistencyGroupID
	}
	if params.Payload.NodeID != "" && !sop.hasNodeDelete && !sop.hasVolDetach {
		if nObj == nil {
			nObj, err = loadNodeObject(params.Payload.NodeID)
			if err != nil {
				return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
		}
		if err = c.nodeIsActive(nObj); err != nil {
			return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if err = c.volumeSeriesRequestValidateCreator(ctx, ai, params.Payload); err != nil {
		return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.DS.OpsVolumeSeriesRequest().Create(ctx, params.Payload)
	if err != nil {
		return ops.NewVolumeSeriesRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.Log.Infof("VolumeSeriesRequest created [%s]", obj.Meta.ID)
	c.setScopeVolumeSeriesRequest(params.HTTPRequest, obj)
	return ops.NewVolumeSeriesRequestCreateCreated().WithPayload(obj)
}

func (c *HandlerComp) volumeSeriesRequestDelete(params ops.VolumeSeriesRequestDeleteParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewVolumeSeriesRequestDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var obj *M.VolumeSeriesRequest
	if obj, err = c.DS.OpsVolumeSeriesRequest().Fetch(ctx, params.ID); err != nil {
		return ops.NewVolumeSeriesRequestDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.Lock()
	defer c.Unlock()
	err = ai.CapOK(centrald.VolumeSeriesOwnerCap, obj.VolumeSeriesCreateSpec.AccountID)
	if err != nil {
		err = ai.CapOK(centrald.CSPDomainManagementCap, obj.VolumeSeriesCreateSpec.TenantAccountID)
	}
	if err != nil {
		return ops.NewVolumeSeriesRequestDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if !vra.VolumeSeriesRequestStateIsTerminated(obj.VolumeSeriesRequestState) {
		err = c.eExists("VolumeSeries is still active")
		return ops.NewVolumeSeriesRequestDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	srLParams := storage_request.NewStorageRequestListParams()
	srLParams.IsTerminated = swag.Bool(false)
	srLParams.VolumeSeriesRequestID = swag.String(params.ID)
	if srL, err := c.DS.OpsStorageRequest().List(ctx, srLParams); err == nil && len(srL) != 0 {
		err = c.eExists("VolumeSeriesRequest has claims in active StorageRequest objects")
		return ops.NewVolumeSeriesRequestDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	} else if err != nil {
		return ops.NewVolumeSeriesRequestDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err = c.DS.OpsVolumeSeriesRequest().Delete(ctx, params.ID); err != nil {
		return ops.NewVolumeSeriesRequestDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.Log.Infof("VolumeSeriesRequest deleted [%s]", obj.Meta.ID)
	c.setScopeVolumeSeriesRequest(params.HTTPRequest, obj)
	return ops.NewVolumeSeriesRequestDeleteNoContent()
}

func (c *HandlerComp) volumeSeriesRequestFetch(params ops.VolumeSeriesRequestFetchParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewVolumeSeriesRequestFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.DS.OpsVolumeSeriesRequest().Fetch(ctx, params.ID)
	if err == nil {
		err = c.volumeSeriesRequestFetchFilter(ai, obj)
	}
	if err != nil {
		return ops.NewVolumeSeriesRequestFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewVolumeSeriesRequestFetchOK().WithPayload(obj)
}

func (c *HandlerComp) volumeSeriesRequestList(params ops.VolumeSeriesRequestListParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewVolumeSeriesRequestListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var list []*M.VolumeSeriesRequest
	// rather than querying for the entire list and performing RBAC filtering in the handler, constrain the query which should be more efficient
	params.AccountID, params.TenantAccountID, err = c.ops.constrainEitherOrQueryAccounts(ctx, ai, params.AccountID, centrald.VolumeSeriesOwnerCap, params.TenantAccountID, centrald.CSPDomainManagementCap)

	// if both AccountID and TenantAccountID are now nil and the caller is not internal, all accounts are filtered out, skip the query
	if err == nil && (params.AccountID != nil || params.TenantAccountID != nil || ai.Internal()) {
		list, err = c.DS.OpsVolumeSeriesRequest().List(ctx, params)
	}
	if err != nil {
		return ops.NewVolumeSeriesRequestListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewVolumeSeriesRequestListOK().WithPayload(list)
}

func (c *HandlerComp) volumeSeriesRequestUpdate(params ops.VolumeSeriesRequestUpdateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err == nil {
		// all modifications are performed by trusted clients only
		err = ai.InternalOK()
	}
	if err != nil {
		return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var uP = [centrald.NumActionTypes][]string{
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    params.Set,
	}
	ua, err := c.makeStdUpdateArgs(c.volumeSeriesRequestMutableNameMap(), params.ID, &params.Version, uP)
	if err != nil {
		return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err = centrald.ErrorUpdateInvalidRequest
		return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.IsModified("VolumeSeriesRequestState") && !vra.ValidateVolumeSeriesRequestState(params.Payload.VolumeSeriesRequestState) {
		err = c.eUpdateInvalidMsg("invalid volumeSeriesRequestState")
		return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.RLock()
	defer c.RUnlock()
	var vsrObj *M.VolumeSeriesRequest
	if ua.AnyModified("VolumeSeriesID", "VolumeSeriesRequestState", "NodeID", "ConsistencyGroupID") {
		if vsrObj, err = c.DS.OpsVolumeSeriesRequest().Fetch(ctx, params.ID); err != nil {
			return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		} else if params.Version != int32(vsrObj.Meta.Version) {
			err = centrald.ErrorIDVerNotFound
			return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if ua.IsModified("VolumeSeriesID") && vsrObj.VolumeSeriesID != params.Payload.VolumeSeriesID {
		if vsrObj.VolumeSeriesID != "" {
			err = c.eUpdateInvalidMsg("volumeSeriesId cannot be modified once set")
			return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if params.Payload.VolumeSeriesID != "" {
			if _, err = c.DS.OpsVolumeSeries().Fetch(ctx, string(params.Payload.VolumeSeriesID)); err != nil {
				if err == centrald.ErrorNotFound {
					err = c.eUpdateInvalidMsg("invalid volumeSeriesId")
				}
				return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
		}
	}
	if ua.IsModified("VolumeSeriesRequestState") {
		if util.Contains(vra.TerminalVolumeSeriesRequestStates(), vsrObj.VolumeSeriesRequestState) {
			err = c.eInvalidState(fmt.Sprintf("cannot modify state of terminated request (%s)", vsrObj.VolumeSeriesRequestState))
			return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if ua.IsModified("NodeID") && vsrObj.NodeID != params.Payload.NodeID {
		if vsrObj.NodeID != "" {
			err = c.eUpdateInvalidMsg("nodeId cannot be modified once set")
			return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if params.Payload.NodeID != "" {
			if _, err = c.DS.OpsNode().Fetch(ctx, string(params.Payload.NodeID)); err != nil {
				if err == centrald.ErrorNotFound {
					err = c.eUpdateInvalidMsg("invalid nodeId")
				}
				return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
		}
	}
	if ua.IsModified("ServicePlanAllocationID") && params.Payload.ServicePlanAllocationID != "" {
		if _, err := c.DS.OpsServicePlanAllocation().Fetch(ctx, string(params.Payload.ServicePlanAllocationID)); err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eUpdateInvalidMsg("invalid servicePlanAllocationId")
			}
			return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if ua.IsModified("StorageFormula") && params.Payload.StorageFormula != "" {
		if !c.app.ValidateStorageFormula(params.Payload.StorageFormula) {
			err := c.eUpdateInvalidMsg("invalid storageFormula")
			return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if a := ua.FindUpdateAttr("CapacityReservationPlan"); a != nil && a.IsModified() {
		if params.Payload.CapacityReservationPlan == nil {
			err = c.eUpdateInvalidMsg("invalid capacityReservationPlan")
			return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		// CapacityReservationPlan is a struct, so only UpdateSet applies
		action := a.Actions[centrald.UpdateSet]
		if _, exists := action.Fields["StorageTypeReservations"]; action.FromBody || exists {
			for k := range params.Payload.CapacityReservationPlan.StorageTypeReservations {
				if st := c.app.GetCspStorageType(M.CspStorageType(k)); st == nil {
					err = c.eUpdateInvalidMsg("invalid storageType %s in capacityReservationPlan.storageTypeReservations", k)
					return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
				}
			}
		}
	}
	if a := ua.FindUpdateAttr("CapacityReservationResult"); a != nil && a.IsModified() {
		if params.Payload.CapacityReservationResult == nil {
			err = c.eUpdateInvalidMsg("invalid capacityReservationResult")
			return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		// CapacityReservationResult is a struct, so only UpdateSet applies
		action := a.Actions[centrald.UpdateSet]
		pCache := map[string]struct{}{}
		if _, exists := action.Fields["DesiredReservations"]; action.FromBody || exists {
			for k := range params.Payload.CapacityReservationResult.DesiredReservations {
				if _, err = c.DS.OpsPool().Fetch(ctx, k); err != nil {
					if err == centrald.ErrorNotFound {
						err = c.eUpdateInvalidMsg("invalid poolId %s in capacityReservationResult.desiredReservations", k)
					}
					return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
				}
				pCache[k] = struct{}{}
			}
		}
		if _, exists := action.Fields["CurrentReservations"]; action.FromBody || exists {
			for k := range params.Payload.CapacityReservationResult.CurrentReservations {
				if _, ok := pCache[k]; ok {
					continue
				}
				if _, err = c.DS.OpsPool().Fetch(ctx, k); err != nil {
					if err == centrald.ErrorNotFound {
						err = c.eUpdateInvalidMsg("invalid poolId %s in capacityReservationResult.currentReservations", k)
					}
					return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
				}
			}
		}
	}
	if a := ua.FindUpdateAttr("StoragePlan"); a != nil && a.IsModified() {
		if params.Payload.StoragePlan == nil {
			err = c.eUpdateInvalidMsg("invalid storagePlan")
			return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		// TBD validate layoutAlgorithm, placementHints
		// StoragePlan is a struct, so only UpdateSet applies
		action := a.Actions[centrald.UpdateSet]
		if _, exists := action.Fields["StorageElements"]; action.FromBody || exists {
			for _, se := range params.Payload.StoragePlan.StorageElements {
				if !util.Contains([]string{com.VolReqStgElemIntentCache, com.VolReqStgElemIntentData}, se.Intent) {
					err = c.eUpdateInvalidMsg("unsupported storagePlan.storageElements.intent %s", se.Intent)
					return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
				}
				if se.Intent == com.VolReqStgElemIntentCache {
					// cache has no pool or storage objects
					continue
				}
				if _, err = c.DS.OpsPool().Fetch(ctx, string(se.PoolID)); err != nil {
					if err == centrald.ErrorNotFound {
						err = c.eUpdateInvalidMsg("invalid poolId %s in storagePlan.storageElements", se.PoolID)
					}
					return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
				}
				for k := range se.StorageParcels {
					if !strings.HasPrefix(k, com.VolReqStgPseudoParcelKey) {
						if _, err = c.DS.OpsStorage().Fetch(ctx, k); err != nil {
							if err == centrald.ErrorNotFound {
								err = c.eUpdateInvalidMsg("invalid storageId %s in storagePlan.storageElements.storageParcels", k)
							}
							return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
						}
					}
				}
			}
		}
	}
	if ua.IsModified("ConsistencyGroupID") && vsrObj.ConsistencyGroupID != params.Payload.ConsistencyGroupID {
		if vsrObj.ConsistencyGroupID != "" {
			err = c.eUpdateInvalidMsg("consistencyGroupId cannot be modified once set")
			return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if params.Payload.ConsistencyGroupID != "" {
			cg, err := c.DS.OpsConsistencyGroup().Fetch(ctx, string(params.Payload.ConsistencyGroupID))
			if err != nil {
				if err == centrald.ErrorNotFound {
					err = c.eUpdateInvalidMsg("invalid consistencyGroupId")
				}
				return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			if cg.AccountID != vsrObj.VolumeSeriesCreateSpec.AccountID {
				err = c.eUpdateInvalidMsg("consistencyGroup and volumeSeries account mismatch")
				return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
		}
	}
	obj, err := c.DS.OpsVolumeSeriesRequest().Update(ctx, ua, params.Payload)
	if err != nil {
		return ops.NewVolumeSeriesRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.setScopeVolumeSeriesRequest(params.HTTPRequest, obj)
	return ops.NewVolumeSeriesRequestUpdateOK().WithPayload(obj)
}

func (c *HandlerComp) setScopeVolumeSeriesRequest(r *http.Request, obj *M.VolumeSeriesRequest) {
	m := crude.ScopeMap{
		"clusterId":                string(obj.ClusterID),
		crude.ScopeMetaVersion:     strconv.FormatInt(int64(obj.Meta.Version), 10),
		"nodeId":                   string(obj.NodeID),
		"volumeSeriesRequestState": string(obj.VolumeSeriesRequestState),
		"volumeSeriesId":           string(obj.VolumeSeriesID),
	}
	if obj.ConsistencyGroupID != "" {
		m["consistencyGroupId"] = string(obj.ConsistencyGroupID)
	}
	if obj.SyncPeers != nil && len(obj.SyncPeers) > 0 {
		m["syncCoordinatorId"] = string(obj.Meta.ID)
	}
	if obj.SyncCoordinatorID != "" {
		m["parentId"] = string(obj.SyncCoordinatorID)
	}
	c.addNewObjIDToScopeMap(obj, m)
	if _, ok := m[crude.ScopeMetaID]; ok {
		m["requestedOperations"] = strings.Join(obj.RequestedOperations, ",")
	}
	c.app.CrudeOps.SetScope(r, m, obj)
}
