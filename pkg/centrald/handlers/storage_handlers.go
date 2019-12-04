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

	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/storage"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
)

// Register handlers for Storage
func (c *HandlerComp) storageRegisterHandlers() {
	c.app.API.StorageStorageCreateHandler = ops.StorageCreateHandlerFunc(c.storageCreate)
	c.app.API.StorageStorageDeleteHandler = ops.StorageDeleteHandlerFunc(c.storageDelete)
	c.app.API.StorageStorageFetchHandler = ops.StorageFetchHandlerFunc(c.storageFetch)
	c.app.API.StorageStorageListHandler = ops.StorageListHandlerFunc(c.storageList)
	c.app.API.StorageStorageUpdateHandler = ops.StorageUpdateHandlerFunc(c.storageUpdate)
}

var nmStorageMutable JSONToAttrNameMap

func (c *HandlerComp) storageMutableNameMap() JSONToAttrNameMap {
	if nmStorageMutable == nil {
		c.cacheMux.Lock()
		defer c.cacheMux.Unlock()
		if nmStorageMutable == nil {
			nmStorageMutable = c.makeJSONToAttrNameMap(M.StorageMutable{})
		}
	}
	return nmStorageMutable
}

// storageFetchFilter verifies the caller has permission to fetch the object, returning an error if they do not.
// Storage owner is typically a subordinate account, but the tenant and system admin can also view them.
func (c *HandlerComp) storageFetchFilter(ai *auth.Info, obj *M.Storage) error {
	err := ai.CapOK(centrald.CSPDomainUsageCap, M.ObjIDMutable(obj.AccountID))
	if err != nil {
		err = ai.CapOK(centrald.CSPDomainManagementCap, M.ObjIDMutable(obj.TenantAccountID))
		if err != nil {
			err = ai.CapOK(centrald.SystemManagementCap)
		}
	}
	return err
}

// validateStorageSizes is a helper to validate *Bytes constraints on create or update
// Note: negative values are disallowed by the swagger autogen code
// TBD validate that availableBytes <= (sizeBytes - Sum(ParcelAllocation.sizeBytes))
func (c *HandlerComp) validateStorageSizes(ua *centrald.UpdateArgs, obj *M.Storage, uObj *M.StorageMutable) error {
	var availableBytes int64
	if ua != nil && ua.IsModified("AvailableBytes") {
		availableBytes = swag.Int64Value(uObj.AvailableBytes)
	} else {
		availableBytes = swag.Int64Value(obj.AvailableBytes)
	}
	sizeBytes := swag.Int64Value(obj.SizeBytes)
	if availableBytes > sizeBytes {
		return fmt.Errorf("availableBytes must not be greater than sizeBytes")
	}
	return nil
}

// validateStorageState is a helper to validate constraints regarding fields of StorageState on create or update
// Note: this helper may query the mongods for Node; caller must hold c.RLock
func (c *HandlerComp) validateStorageState(ctx context.Context, ua *centrald.UpdateArgs, oObj *M.Storage, uObj *M.StorageMutable) error {
	if oObj == nil {
		oObj = &M.Storage{}
	}
	var attachmentState string
	var deviceState string
	var mediaState string
	var provisionedState string
	var device string
	var nodeID M.ObjIDMutable
	oState := oObj.StorageState
	if oState != nil {
		attachmentState = oState.AttachmentState
		deviceState = oState.DeviceState
		mediaState = oState.MediaState
		provisionedState = oState.ProvisionedState
		device = oState.AttachedNodeDevice
		nodeID = oState.AttachedNodeID
	}
	var uState *M.StorageStateMutable
	if uObj != nil {
		uState = uObj.StorageState
	}
	// on creation, the states can be defaulted
	if attachmentState == "" {
		attachmentState = centrald.DefaultStorageAttachmentState
	}
	if deviceState == "" {
		deviceState = centrald.DefaultStorageDeviceState
	}
	if mediaState == "" {
		mediaState = centrald.DefaultStorageMediaState
	}
	if provisionedState == "" {
		provisionedState = centrald.DefaultStorageProvisionedState
	}
	storageIdentifier := oObj.StorageIdentifier
	if ua != nil && ua.IsModified("StorageIdentifier") {
		storageIdentifier = uObj.StorageIdentifier
	}
	if uState != nil && ua != nil && ua.IsModified("StorageState") {
		// find updated fields. Because this is a struct, only "set" needs to be considered
		if a := ua.FindUpdateAttr("StorageState"); a != nil {
			action := a.Actions[centrald.UpdateSet]
			if _, exists := action.Fields["AttachmentState"]; action.FromBody || exists {
				attachmentState = uState.AttachmentState
			}
			if _, exists := action.Fields["DeviceState"]; action.FromBody || exists {
				deviceState = uState.DeviceState
			}
			if _, exists := action.Fields["MediaState"]; action.FromBody || exists {
				mediaState = uState.MediaState
			}
			if _, exists := action.Fields["ProvisionedState"]; action.FromBody || exists {
				provisionedState = uState.ProvisionedState
			}
			if _, exists := action.Fields["AttachedNodeID"]; action.FromBody || exists {
				nodeID = uState.AttachedNodeID
			}
			if _, exists := action.Fields["AttachedNodeDevice"]; action.FromBody || exists {
				device = uState.AttachedNodeDevice
			}
		}
	}
	if !c.app.ValidateStorageAttachmentState(attachmentState) {
		return fmt.Errorf("attachmentState must be one of %s", c.app.SupportedStorageAttachmentStates())
	}
	if !c.app.ValidateStorageDeviceState(deviceState) {
		return fmt.Errorf("deviceState must be one of %s", c.app.SupportedStorageDeviceStates())
	}
	if !c.app.ValidateStorageMediaState(mediaState) {
		return fmt.Errorf("mediaState must be one of %s", c.app.SupportedStorageMediaStates())
	}
	if !c.app.ValidateStorageProvisionedState(provisionedState) {
		return fmt.Errorf("provisionedState must be one of %s", c.app.SupportedStorageProvisionedStates())
	}
	if (nodeID == "" || device == "") && attachmentState == "ATTACHED" {
		return fmt.Errorf("attachedNodeId and attachedNodeDevice are required for attachmentState %s", attachmentState)
	} else if (nodeID == "") && util.Contains([]string{"ATTACHING", "DETACHING"}, attachmentState) {
		return fmt.Errorf("attachedNodeId is required for attachmentState %s", attachmentState)
	}
	if nodeID != "" {
		if nObj, err := c.DS.OpsNode().Fetch(ctx, string(nodeID)); err != nil {
			if err == centrald.ErrorNotFound {
				err = fmt.Errorf("invalid attachedNodeId")
			}
			return err
		} else if oObj != nil && M.ObjID(nObj.ClusterID) != oObj.ClusterID {
			return fmt.Errorf("nodeId not in cluster")
		}
	}
	// StorageIdentifier required in PROVISIONED, also if StorageIdentifier was ever set, do not let it be reset when UNPROVISIONING
	if storageIdentifier == "" && (provisionedState == com.StgProvisionedStateProvisioned || oObj.StorageIdentifier != "" && provisionedState == com.StgProvisionedStateUnprovisioning) {
		return fmt.Errorf("storageIdentifier required for provisionedState %s", provisionedState)
	}
	return nil
}

// Handlers

func (c *HandlerComp) storageCreate(params ops.StorageCreateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewStorageCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err := centrald.ErrorMissing
		return ops.NewStorageCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload.PoolID == "" {
		err := c.eMissingMsg("poolId required")
		return ops.NewStorageCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err = c.validateStorageSizes(nil, params.Payload, nil); err != nil {
		err = c.eMissingMsg("%s", err.Error())
		return ops.NewStorageCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.RLock()
	defer c.RUnlock()
	err = c.validateStorageState(ctx, nil, params.Payload, nil)
	if err != nil {
		err = c.eMissingMsg("%s", err.Error())
		return ops.NewStorageCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var pObj *M.Pool
	if pObj, err = c.DS.OpsPool().Fetch(ctx, string(params.Payload.PoolID)); err != nil {
		if err == centrald.ErrorNotFound {
			err = c.eMissingMsg("invalid poolID")
		}
		return ops.NewStorageCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	params.Payload.ClusterID = M.ObjID(pObj.ClusterID) // set cluster from pool
	if err = ai.CapOK(centrald.CSPDomainUsageCap, pObj.AuthorizedAccountID); err != nil {
		return ops.NewStorageCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	// stash some pool properties in the storage object
	params.Payload.AccountID = M.ObjID(pObj.AuthorizedAccountID)
	if pObj.AuthorizedAccountID != pObj.AccountID {
		params.Payload.TenantAccountID = M.ObjID(pObj.AccountID)
	} else {
		// if IDs are the same in the pool, the AuthorizedAccount is a tenant account
		params.Payload.TenantAccountID = ""
	}
	params.Payload.CspDomainID = M.ObjID(pObj.CspDomainID)
	params.Payload.CspStorageType = M.CspStorageType(pObj.CspStorageType)
	params.Payload.StorageAccessibility = &M.StorageAccessibility{}
	params.Payload.StorageAccessibility.StorageAccessibilityMutable = *pObj.StorageAccessibility
	obj, err := c.DS.OpsStorage().Create(ctx, params.Payload)
	if err != nil {
		return ops.NewStorageCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.Log.Infof("Storage %s created [%s]", obj.StorageIdentifier, obj.Meta.ID)
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	return ops.NewStorageCreateCreated().WithPayload(obj)
}

func (c *HandlerComp) storageDelete(params ops.StorageDeleteParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewStorageDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var obj *M.Storage
	if obj, err = c.DS.OpsStorage().Fetch(ctx, params.ID); err == nil {
		c.Lock()
		defer c.Unlock()
		if err = ai.CapOK(centrald.CSPDomainUsageCap, M.ObjIDMutable(obj.AccountID)); err != nil {
			return ops.NewStorageDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		vslParams := volume_series.VolumeSeriesListParams{StorageID: &params.ID}
		vrlParams := volume_series_request.VolumeSeriesRequestListParams{StorageID: &params.ID, IsTerminated: swag.Bool(false)}
		var n int
		if !util.Contains([]string{"UNPROVISIONED", "UNPROVISIONING"}, obj.StorageState.ProvisionedState) {
			err = &centrald.Error{M: "storage is not in UNPROVISIONED or UNPROVISIONING state", C: centrald.ErrorExists.C}
		} else if n, err = c.DS.OpsVolumeSeries().Count(ctx, vslParams, 1); err == nil {
			if n != 0 {
				err = &centrald.Error{M: "volume series are still using the storage", C: centrald.ErrorExists.C}
			} else if n, err = c.DS.OpsVolumeSeriesRequest().Count(ctx, vrlParams, 1); err == nil {
				if n != 0 {
					err = &centrald.Error{M: "active volume series requests are still associated with the storage", C: centrald.ErrorExists.C}
				} else if err = c.DS.OpsStorage().Delete(ctx, params.ID); err == nil {
					c.Log.Infof("Storage %s deleted [%s]", obj.StorageIdentifier, obj.Meta.ID)
					c.setDefaultObjectScope(params.HTTPRequest, obj)
					return ops.NewStorageDeleteNoContent()
				}
			}
		}
	}
	return ops.NewStorageDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
}

func (c *HandlerComp) storageFetch(params ops.StorageFetchParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewStorageFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.DS.OpsStorage().Fetch(ctx, params.ID)
	if err == nil {
		err = c.storageFetchFilter(ai, obj)
	}
	if err != nil {
		return ops.NewStorageFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewStorageFetchOK().WithPayload(obj)
}

func (c *HandlerComp) storageList(params ops.StorageListParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewStorageListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var aggregations []*centrald.Aggregation
	var list []*M.Storage
	var count int
	// Aggregate() is implemented in the datastore, so must modify params to enforce RBAC by adding additional query filters instead of using storageFetchFilter.
	params.AccountID, params.TenantAccountID, err = c.ops.constrainEitherOrQueryAccounts(ctx, ai, params.AccountID, centrald.CSPDomainUsageCap, params.TenantAccountID, centrald.CSPDomainManagementCap)

	// if both AccountID and TenantAccountID are now nil and the caller is not system admin (or internal), all accounts are filtered out, skip the query
	if err == nil && (params.AccountID != nil || params.TenantAccountID != nil || ai.CapOK(centrald.SystemManagementCap) == nil) {
		if len(params.Sum) > 0 {
			aggregations, count, err = c.DS.OpsStorage().Aggregate(ctx, params)
		} else {
			list, err = c.DS.OpsStorage().List(ctx, params)
			count = len(list)
		}
	}
	if err != nil {
		return ops.NewStorageListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewStorageListOK().WithPayload(list).WithTotalCount(int64(count)).WithAggregations(c.makeAggregationResult(aggregations))
}

func (c *HandlerComp) storageUpdate(params ops.StorageUpdateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err == nil {
		// all modifications are performed by trusted clients only
		err = ai.InternalOK()
	}
	if err != nil {
		return ops.NewStorageUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var uP = [centrald.NumActionTypes][]string{
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    params.Set,
	}
	ua, err := c.makeStdUpdateArgs(c.storageMutableNameMap(), params.ID, &params.Version, uP)
	if err != nil {
		return ops.NewStorageUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err = centrald.ErrorUpdateInvalidRequest
		return ops.NewStorageUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.RLock()
	defer c.RUnlock()
	// fetch the current object to perform validations; both fields require validation
	// NB: version query parameter is required for storageUpdate so no need to set it
	oObj, err := c.DS.OpsStorage().Fetch(ctx, params.ID)
	if err != nil {
		return ops.NewStorageUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	} else if int32(oObj.Meta.Version) != params.Version {
		// there was concern that leaving the version check for mongo to do later could produce misleading error messages
		err = centrald.ErrorIDVerNotFound
		return ops.NewStorageUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	// TBD query for ParcelAllocation objects for this pool and use in size validation
	if ua.IsModified("AvailableBytes") {
		if err = c.validateStorageSizes(ua, oObj, params.Payload); err != nil {
			err = c.eUpdateInvalidMsg("%s", err.Error())
			return ops.NewStorageUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if ua.IsModified("StorageState") || ua.IsModified("StorageIdentifier") {
		if err = c.validateStorageState(ctx, ua, oObj, params.Payload); err != nil {
			err = c.eUpdateInvalidMsg("%s", err.Error())
			return ops.NewStorageUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	// The shareableStorage property may be changed only if currently not providing capacity (it may be attached)
	if ua.IsModified("ShareableStorage") && oObj.ShareableStorage != params.Payload.ShareableStorage && (swag.Int64Value(oObj.AvailableBytes) != (swag.Int64Value(oObj.TotalParcelCount) * swag.Int64Value(oObj.ParcelSizeBytes))) {
		err = c.eUpdateInvalidMsg("shareableStorage may be changed only when no capacity is provisioned")
		return ops.NewStorageUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.DS.OpsStorage().Update(ctx, ua, params.Payload)
	if err != nil {
		return ops.NewStorageUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	return ops.NewStorageUpdateOK().WithPayload(obj)
}
