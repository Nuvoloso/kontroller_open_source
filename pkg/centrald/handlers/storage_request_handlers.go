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
	"time"

	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
)

// Register handlers for StorageRequest
func (c *HandlerComp) storageRequestRegisterHandlers() {
	c.app.API.StorageRequestStorageRequestCreateHandler = ops.StorageRequestCreateHandlerFunc(c.storageRequestCreate)
	c.app.API.StorageRequestStorageRequestDeleteHandler = ops.StorageRequestDeleteHandlerFunc(c.storageRequestDelete)
	c.app.API.StorageRequestStorageRequestFetchHandler = ops.StorageRequestFetchHandlerFunc(c.storageRequestFetch)
	c.app.API.StorageRequestStorageRequestListHandler = ops.StorageRequestListHandlerFunc(c.storageRequestList)
	c.app.API.StorageRequestStorageRequestUpdateHandler = ops.StorageRequestUpdateHandlerFunc(c.storageRequestUpdate)
}

var nmStorageRequestMutable JSONToAttrNameMap

func (c *HandlerComp) storageRequestMutableNameMap() JSONToAttrNameMap {
	if nmStorageRequestMutable == nil {
		c.cacheMux.Lock()
		defer c.cacheMux.Unlock()
		if nmStorageRequestMutable == nil {
			nmStorageRequestMutable = c.makeJSONToAttrNameMap(M.StorageRequestMutable{})
		}
	}
	return nmStorageRequestMutable
}

// storageRequestFetchFilter verifies the caller has permission to fetch the object, returning an error if they do not.
// StorageRequest owner is typically a subordinate account, but the tenant and system admin can also view them.
func (c *HandlerComp) storageRequestFetchFilter(ai *auth.Info, obj *M.StorageRequest) error {
	err := ai.CapOK(centrald.CSPDomainUsageCap, M.ObjIDMutable(obj.AccountID))
	if err != nil {
		err = ai.CapOK(centrald.CSPDomainManagementCap, M.ObjIDMutable(obj.TenantAccountID))
		if err != nil {
			err = ai.CapOK(centrald.SystemManagementCap)
		}
	}
	return err
}

var storageRequestOperationSequences = map[string]int{
	com.StgReqOpProvision: 0,
	com.StgReqOpAttach:    0,
	com.StgReqOpFormat:    0,
	com.StgReqOpUse:       0,
	com.StgReqOpClose:     1,
	com.StgReqOpDetach:    1,
	com.StgReqOpRelease:   1,
	com.StgReqOpReattach:  2,
}

type storageOpsParsed struct {
	hasProvision, hasAttach, hasFormat, hasUse, hasClose, hasDetach, hasRelease, hasReattach bool
	canonicalOrder                                                                           []string
}

// storageRequestValidateRequestedOperations will parse, validate and canonicalize the requested operations
func (c *HandlerComp) storageRequestValidateRequestedOperations(operations []string) (*storageOpsParsed, error) {
	lastSeq := -1
	for _, op := range operations {
		if seq, found := storageRequestOperationSequences[op]; found {
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
	sop := &storageOpsParsed{
		hasAttach:    util.Contains(operations, com.StgReqOpAttach),
		hasClose:     util.Contains(operations, com.StgReqOpClose),
		hasDetach:    util.Contains(operations, com.StgReqOpDetach),
		hasFormat:    util.Contains(operations, com.StgReqOpFormat),
		hasProvision: util.Contains(operations, com.StgReqOpProvision),
		hasReattach:  util.Contains(operations, com.StgReqOpReattach),
		hasRelease:   util.Contains(operations, com.StgReqOpRelease),
		hasUse:       util.Contains(operations, com.StgReqOpUse),
	}
	// normalize the requested operation order
	reqOps := make([]string, 0, len(operations))
	if sop.hasProvision {
		reqOps = append(reqOps, com.StgReqOpProvision)
	}
	if sop.hasAttach {
		reqOps = append(reqOps, com.StgReqOpAttach)
	}
	if sop.hasFormat {
		reqOps = append(reqOps, com.StgReqOpFormat)
	}
	if sop.hasUse {
		reqOps = append(reqOps, com.StgReqOpUse)
	}
	if sop.hasClose {
		reqOps = append(reqOps, com.StgReqOpClose)
	}
	if sop.hasDetach {
		reqOps = append(reqOps, com.StgReqOpDetach)
	}
	if sop.hasRelease {
		reqOps = append(reqOps, com.StgReqOpRelease)
	}
	if sop.hasReattach {
		reqOps = append(reqOps, com.StgReqOpReattach)
		sop.hasClose = true
		sop.hasDetach = true
		sop.hasAttach = true
		sop.hasUse = true
	}
	sop.canonicalOrder = reqOps
	return sop, nil
}

// Handlers

func (c *HandlerComp) storageRequestCreate(params ops.StorageRequestCreateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err := centrald.ErrorMissing
		return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	// NB: autogen swagger code ensures RequestedOperations are present, valid, unique and at least 1 item.
	// It also ensures MinSizeBytes is non-negative
	sop, err := c.storageRequestValidateRequestedOperations(params.Payload.RequestedOperations)
	if err != nil {
		err = c.eMissingMsg("%s", err.Error())
		return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	params.Payload.RequestedOperations = sop.canonicalOrder // rewrite
	if sop.hasFormat && params.Payload.ParcelSizeBytes == nil {
		err = c.eMissingMsg("FORMAT requires parcelSizeBytes")
		return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
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
	c.Lock() // write lock required due to query for other storage requests
	defer c.Unlock()
	sID := string(params.Payload.StorageID)
	var nObj *M.Node
	var sObj *M.Storage
	var pObj *M.Pool
	if sop.hasProvision {
		if pObj, err = c.DS.OpsPool().Fetch(ctx, string(params.Payload.PoolID)); err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eMissingMsg("invalid poolId")
			}
			return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if err = ai.CapOK(centrald.CSPDomainUsageCap, pObj.AuthorizedAccountID); err != nil {
			return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		// initialize several ID properties from the pool object
		params.Payload.AccountID = M.ObjID(pObj.AuthorizedAccountID)
		params.Payload.TenantAccountID = M.ObjID(pObj.AccountID)
		params.Payload.ClusterID = M.ObjID(pObj.ClusterID)
		params.Payload.CspStorageType = pObj.CspStorageType
		params.Payload.CspDomainID = M.ObjID(pObj.CspDomainID)
		cspStorageType := c.app.GetCspStorageType(M.CspStorageType(pObj.CspStorageType))
		if swag.Int64Value(params.Payload.MinSizeBytes) > swag.Int64Value(cspStorageType.MaxAllocationSizeBytes) {
			err = c.eMissingMsg("minSizeBytes is greater than cspStorageType maxAllocationSizeBytes")
			return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		params.Payload.StorageID = ""
	} else {
		params.Payload.PoolID = ""
		sObj, err = c.DS.OpsStorage().Fetch(ctx, sID)
		if err == nil {
			err = ai.CapOK(centrald.CSPDomainUsageCap, M.ObjIDMutable(sObj.AccountID))
		}
		if err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eMissingMsg("invalid storageId")
			}
			return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if sObj.StorageState.AttachmentState != "DETACHED" && sObj.StorageState.AttachedNodeID != "" {
			if !sop.hasReattach { // REATTACH provides a valid Payload.NodeID and storage is attached
				if sop.hasAttach {
					err = c.eMissingMsg("storageId %s is not DETACHED", sID)
					return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
				}
				// is DETACH, RELEASE, FORMAT, CLOSE (standalone only), USE
				params.Payload.NodeID = sObj.StorageState.AttachedNodeID
			}
		}
		// FORMAT and USE require attached storage or a preceding ATTACH operation
		if (sop.hasFormat || sop.hasUse) && sObj.StorageState.AttachmentState != com.StgAttachmentStateAttached {
			if !sop.hasAttach {
				err = c.eMissingMsg("FORMAT/USE will fail because storageId %s is not ATTACHED and no ATTACH operation specified", sID)
				return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
		}
		list, err := c.DS.OpsStorageRequest().List(ctx, ops.StorageRequestListParams{StorageID: &sID, IsTerminated: swag.Bool(false)})
		if err != nil {
			return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if len(list) != 0 {
			err = c.eMissingMsg("storageId %s is in use by storageRequest %s", sID, string(list[0].Meta.ID))
			return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		params.Payload.AccountID = sObj.AccountID
		params.Payload.TenantAccountID = sObj.TenantAccountID
		params.Payload.ClusterID = sObj.ClusterID
		params.Payload.CspDomainID = sObj.CspDomainID
	}
	// FORMAT and USE require a storageID or preceding PROVISION+ATTACH
	if (sop.hasFormat || sop.hasUse) && params.Payload.StorageID == "" && !(sop.hasProvision && sop.hasAttach) {
		err := c.eMissingMsg("FORMAT/USE will fail because storageId is not specified and no PROVISION+ATTACH operations specified")
		return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if sop.hasUse && !sop.hasFormat && (sObj == nil || sObj.StorageState == nil || sObj.StorageState.MediaState != com.StgMediaStateFormatted) {
		err := c.eMissingMsg("USE requires formatted Storage")
		return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if !time.Time(params.Payload.CompleteByTime).IsZero() && !time.Time(params.Payload.CompleteByTime).After(time.Now()) {
		err = c.eMissingMsg("completeByTime is in the past")
		return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if sop.hasAttach && !sop.hasReattach {
		if nObj, err = loadNodeObject(params.Payload.NodeID); err != nil {
			return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		params.Payload.ClusterID = M.ObjID(nObj.ClusterID)
		clObj, err := c.DS.OpsCluster().Fetch(ctx, string(params.Payload.ClusterID))
		if err == nil {
			if len(clObj.AuthorizedAccounts) > 0 {
				err = ai.CapOK(centrald.CSPDomainUsageCap, clObj.AuthorizedAccounts...)
			} else {
				err = ai.InternalOK()
			}
		}
		if err != nil {
			return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if pObj != nil && pObj.CspDomainID != clObj.CspDomainID {
			err = c.eMissingMsg("node %s and pool %s are in different CSP domains", nObj.Name, string(params.Payload.PoolID))
			return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		} else if params.Payload.CspDomainID != M.ObjID(clObj.CspDomainID) {
			err = c.eMissingMsg("node %s and storage object %s are in different CSP domains", nObj.Name, string(params.Payload.StorageID))
			return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if sop.hasClose && !sop.hasReattach {
		if sObj.StorageState.AttachmentState != com.StgAttachmentStateAttached {
			err = c.eMissingMsg("storageId is not ATTACHED")
			return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		// ensure no references to the Storage object in VolumeSeries
		vslParams := volume_series.VolumeSeriesListParams{StorageID: &sID}
		var n int
		if n, err = c.DS.OpsVolumeSeries().Count(ctx, vslParams, 1); err == nil && n != 0 {
			err = c.eRequestInConflict("volume series are still using the storage")
		}
		if err != nil {
			return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if sop.hasReattach {
		var rnObj *M.Node
		if rnObj, err = loadNodeObject(params.Payload.ReattachNodeID); err != nil {
			return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if err = c.nodeIsActive(rnObj); err != nil {
			return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if M.ObjID(rnObj.ClusterID) != sObj.ClusterID {
			err = c.eMissingMsg("reattachNodeId not in the same cluster")
			return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if sObj.StorageState.AttachedNodeID == params.Payload.ReattachNodeID && sObj.StorageState.DeviceState == com.StgDeviceStateOpen {
			err = c.eMissingMsg("storage already attached and in use on reattachNodeId")
			return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		params.Payload.NodeID = params.Payload.ReattachNodeID
		if sObj.StorageState.AttachedNodeID != "" {
			params.Payload.NodeID = sObj.StorageState.AttachedNodeID // effective node follows Storage if set
		}
	}
	if params.Payload.NodeID != "" && (sop.hasAttach || sop.hasReattach || sop.hasUse || sop.hasFormat || sop.hasClose) {
		if nObj == nil {
			nObj, err = loadNodeObject(params.Payload.NodeID)
			if err != nil {
				return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
		}
		if err = c.nodeIsActive(nObj); err != nil {
			return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	obj, err := c.DS.OpsStorageRequest().Create(ctx, params.Payload)
	if err != nil {
		return ops.NewStorageRequestCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.Log.Infof("StorageRequest created [%s]", obj.Meta.ID)
	c.setScopeStorageRequest(params.HTTPRequest, obj)
	return ops.NewStorageRequestCreateCreated().WithPayload(obj)
}

func (c *HandlerComp) storageRequestDelete(params ops.StorageRequestDeleteParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewStorageRequestDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var obj *M.StorageRequest
	if obj, err = c.DS.OpsStorageRequest().Fetch(ctx, params.ID); err != nil {
		return ops.NewStorageRequestDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.Lock()
	defer c.Unlock()
	if err = ai.CapOK(centrald.CSPDomainUsageCap, M.ObjIDMutable(obj.AccountID)); err != nil {
		if err = ai.CapOK(centrald.CSPDomainManagementCap, M.ObjIDMutable(obj.TenantAccountID)); err != nil {
			return ops.NewStorageRequestDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if !util.Contains([]string{"SUCCEEDED", "FAILED"}, obj.StorageRequestState) {
		err = c.eExists("Storage request is still active")
		return ops.NewStorageRequestDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if obj.VolumeSeriesRequestClaims != nil && len(obj.VolumeSeriesRequestClaims.Claims) > 0 {
		for vsrID := range obj.VolumeSeriesRequestClaims.Claims {
			vsrObj, err := c.DS.OpsVolumeSeriesRequest().Fetch(ctx, vsrID)
			if err != nil && err != centrald.ErrorNotFound {
				return ops.NewStorageRequestDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			if err == nil && !vra.VolumeSeriesRequestStateIsTerminated(vsrObj.VolumeSeriesRequestState) {
				err = c.eExists("Active VolumeSeriesRequest %s has a claim", vsrID)
				return ops.NewStorageRequestDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
		}
	}
	if err = c.DS.OpsStorageRequest().Delete(ctx, params.ID); err != nil {
		return ops.NewStorageRequestDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.Log.Infof("StorageRequest deleted [%s]", obj.Meta.ID)
	c.setScopeStorageRequest(params.HTTPRequest, obj)
	return ops.NewStorageRequestDeleteNoContent()
}

func (c *HandlerComp) storageRequestFetch(params ops.StorageRequestFetchParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewStorageRequestFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.DS.OpsStorageRequest().Fetch(ctx, params.ID)
	if err == nil {
		err = c.storageRequestFetchFilter(ai, obj)
	}
	if err != nil {
		return ops.NewStorageRequestFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewStorageRequestFetchOK().WithPayload(obj)
}

func (c *HandlerComp) storageRequestList(params ops.StorageRequestListParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewStorageRequestListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	list, err := c.DS.OpsStorageRequest().List(ctx, params)
	if err != nil {
		return ops.NewStorageRequestListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	ret := make([]*M.StorageRequest, 0, len(list))
	for _, obj := range list {
		if c.storageRequestFetchFilter(ai, obj) == nil {
			ret = append(ret, obj)
		}
	}
	return ops.NewStorageRequestListOK().WithPayload(ret)
}

func (c *HandlerComp) storageRequestUpdate(params ops.StorageRequestUpdateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err == nil {
		// all modifications are performed by trusted clients only
		err = ai.InternalOK()
	}
	if err != nil {
		return ops.NewStorageRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var uP = [centrald.NumActionTypes][]string{
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    params.Set,
	}
	ua, err := c.makeStdUpdateArgs(c.storageRequestMutableNameMap(), params.ID, &params.Version, uP)
	if err != nil {
		return ops.NewStorageRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err = centrald.ErrorUpdateInvalidRequest
		return ops.NewStorageRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.IsModified("StorageRequestState") && !c.app.ValidateStorageRequestState(params.Payload.StorageRequestState) {
		err = c.eUpdateInvalidMsg("invalid storageRequestState")
		return ops.NewStorageRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	checkIfNodeIsActive := func(ctx context.Context, nodeID M.ObjIDMutable) error {
		nObj, err := c.DS.OpsNode().Fetch(ctx, string(nodeID))
		if err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eMissingMsg("invalid nodeId")
			}
			return err
		}
		if err = c.nodeIsActive(nObj); err != nil {
			return err
		}
		return nil
	}
	c.RLock()
	defer c.RUnlock()
	if ua.IsModified("StorageID") && params.Payload.StorageID != "" {
		if _, err = c.DS.OpsStorage().Fetch(ctx, string(params.Payload.StorageID)); err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eUpdateInvalidMsg("invalid storageId")
			}
			return ops.NewStorageRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if ua.IsModified("NodeID") && params.Payload.NodeID != "" {
		if mErr := checkIfNodeIsActive(ctx, params.Payload.NodeID); mErr != nil {
			return ops.NewStorageRequestUpdateDefault(c.eCode(mErr)).WithPayload(c.eError(mErr))
		}
	}
	if ua.IsModified("StorageRequestState") {
		srObj, err := c.DS.OpsStorageRequest().Fetch(ctx, params.ID)
		if err != nil {
			return ops.NewStorageRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if params.Version != int32(srObj.Meta.Version) {
			err = centrald.ErrorIDVerNotFound
			return ops.NewStorageRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if params.Payload.StorageRequestState != srObj.StorageRequestState {
			if util.Contains(c.app.TerminalStorageRequestStates(), srObj.StorageRequestState) {
				err = c.eInvalidState(fmt.Sprintf("cannot modify state of terminated request (%s)", srObj.StorageRequestState))
				return ops.NewStorageRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			if vra.GetSRProcess(params.Payload.StorageRequestState, "") == vra.ApAgentd {
				if mErr := checkIfNodeIsActive(ctx, srObj.NodeID); mErr != nil {
					return ops.NewStorageRequestUpdateDefault(c.eCode(mErr)).WithPayload(c.eError(mErr))
				}
			}
		}
	}
	if a := ua.FindUpdateAttr("VolumeSeriesRequestClaims"); a != nil && a.IsModified() {
		if params.Payload.VolumeSeriesRequestClaims == nil {
			err = c.eUpdateInvalidMsg("missing volumeSeriesRequestClaims")
			return ops.NewStorageRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		action := a.Actions[centrald.UpdateSet] // is a struct
		if _, exists := action.Fields["Claims"]; action.FromBody || exists {
			for k := range params.Payload.VolumeSeriesRequestClaims.Claims {
				if _, err = c.DS.OpsVolumeSeriesRequest().Fetch(ctx, k); err != nil {
					if err == centrald.ErrorNotFound {
						err = c.eUpdateInvalidMsg("invalid volumeSeriesRequestId: %s", k)
					}
					return ops.NewStorageRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
				}
			}
		}
	}
	obj, err := c.DS.OpsStorageRequest().Update(ctx, ua, params.Payload)
	if err != nil {
		return ops.NewStorageRequestUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.setScopeStorageRequest(params.HTTPRequest, obj)
	return ops.NewStorageRequestUpdateOK().WithPayload(obj)
}

func (c *HandlerComp) setScopeStorageRequest(r *http.Request, obj *M.StorageRequest) {
	m := crude.ScopeMap{
		"clusterId":           string(obj.ClusterID),
		"nodeId":              string(obj.NodeID),
		"storageRequestState": string(obj.StorageRequestState),
		"poolId":              string(obj.PoolID),
	}
	c.addNewObjIDToScopeMap(obj, m)
	c.app.CrudeOps.SetScope(r, m, obj)
}
