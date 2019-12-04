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
	"strings"

	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/alecthomas/units"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
)

// Register handlers for VolumeSeries
func (c *HandlerComp) volumeSeriesRegisterHandlers() {
	c.app.API.VolumeSeriesVolumeSeriesCreateHandler = ops.VolumeSeriesCreateHandlerFunc(c.volumeSeriesCreate)
	c.app.API.VolumeSeriesVolumeSeriesDeleteHandler = ops.VolumeSeriesDeleteHandlerFunc(c.volumeSeriesDelete)
	c.app.API.VolumeSeriesVolumeSeriesFetchHandler = ops.VolumeSeriesFetchHandlerFunc(c.volumeSeriesFetch)
	c.app.API.VolumeSeriesVolumeSeriesListHandler = ops.VolumeSeriesListHandlerFunc(c.volumeSeriesList)
	c.app.API.VolumeSeriesVolumeSeriesNewIDHandler = ops.VolumeSeriesNewIDHandlerFunc(c.volumeSeriesNewID)
	c.app.API.VolumeSeriesVolumeSeriesPVSpecFetchHandler = ops.VolumeSeriesPVSpecFetchHandlerFunc(c.volumeSeriesPVSpecFetch)
	c.app.API.VolumeSeriesVolumeSeriesUpdateHandler = ops.VolumeSeriesUpdateHandlerFunc(c.volumeSeriesUpdate)
}

var nmVolumeSeriesMutable JSONToAttrNameMap

func (c *HandlerComp) volumeSeriesMutableNameMap() JSONToAttrNameMap {
	if nmVolumeSeriesMutable == nil {
		c.cacheMux.Lock()
		defer c.cacheMux.Unlock()
		if nmVolumeSeriesMutable == nil {
			nmVolumeSeriesMutable = c.makeJSONToAttrNameMap(M.VolumeSeriesMutable{})
		}
	}
	return nmVolumeSeriesMutable
}

// volumeSeriesFetchFilter verifies the caller has permission to fetch the object, returning an error if they do not.
// Volume Series owner is typically a subordinate account, but the tenant admin can also view them.
func (c *HandlerComp) volumeSeriesFetchFilter(ai *auth.Info, obj *M.VolumeSeries) error {
	err := ai.CapOK(centrald.VolumeSeriesOwnerCap, obj.AccountID)
	if err != nil {
		err = ai.CapOK(centrald.VolumeSeriesFetchCap, obj.TenantAccountID)
	}
	return err
}

// volumeSeriesValidateCreateArgs validates the createParams, the caller is expected to hold the handlerComp lock or read-lock
// The names of the service plan and consistency group are returned on success
func (c *HandlerComp) volumeSeriesValidateCreateArgs(ctx context.Context, ai *auth.Info, payload *M.VolumeSeriesCreateArgs) (spName string, cgName string, err error) {
	if payload == nil || payload.Name == "" || payload.ServicePlanID == "" || swag.Int64Value(payload.SizeBytes) == 0 {
		return "", "", c.eMissingMsg("name, servicePlanId and sizeBytes are required")
	}
	if err = ai.CapOK(centrald.VolumeSeriesOwnerCap); err != nil {
		c.app.AuditLog.Post(ctx, ai, centrald.VolumeSeriesCreateAction, "", payload.Name, "", true, "Create unauthorized")
		return "", "", err
	}
	if !ai.Internal() {
		sTags := util.NewTagList(payload.SystemTags)
		if _, ok := sTags.Get(com.SystemTagVolumeSeriesID); ok {
			return "", "", c.eUnauthorizedOrForbidden("attempt to set object identifier")
		}
		if len(payload.ClusterDescriptor) != 0 {
			return "", "", c.eUnauthorizedOrForbidden("attempt to set clusterDescriptor")
		}
	}
	if ai.AccountID != "" {
		// use auth info when specified (otherwise caller has internal role)
		payload.AccountID = M.ObjIDMutable(ai.AccountID)
		payload.TenantAccountID = M.ObjIDMutable(ai.TenantAccountID)
	} else {
		var aObj *M.Account
		if aObj, err = c.DS.OpsAccount().Fetch(ctx, string(payload.AccountID)); err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eMissingMsg("invalid accountId")
			}
			return "", "", err
		}
		payload.TenantAccountID = aObj.TenantAccountID
	}
	payload.SizeBytes = swag.Int64(util.RoundUpBytes(swag.Int64Value(payload.SizeBytes), util.BytesInGiB))
	var spObj *M.ServicePlan
	if spObj, err = c.DS.OpsServicePlan().Fetch(ctx, string(payload.ServicePlanID)); err != nil {
		if err == centrald.ErrorNotFound {
			err = c.eMissingMsg("invalid servicePlanId")
		}
	} else if spObj.State == centrald.RetiredState {
		err = c.eMissingMsg("servicePlan %s is %s", spObj.Name, centrald.RetiredState)
	} else if !util.Contains(spObj.Accounts, payload.AccountID) {
		err = c.eUnauthorizedOrForbidden("servicePlan is not available to the account")
	} else {
		spName = string(spObj.Name)
	}
	// TBD enforce access to unpublished ServicePlans when they are supported
	if err == nil && payload.ConsistencyGroupID != "" {
		var cgObj *M.ConsistencyGroup
		if cgObj, err = c.DS.OpsConsistencyGroup().Fetch(ctx, string(payload.ConsistencyGroupID)); err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eMissingMsg("invalid consistencyGroupId")
			}
		} else if cgObj.AccountID != payload.AccountID {
			err = c.eUnauthorizedOrForbidden("consistencyGroup is not available to the account")
		} else {
			cgName = string(cgObj.Name)
		}
	}
	return spName, cgName, err
}

func (c *HandlerComp) volumeSeriesFindMount(obj *M.VolumeSeries, snapID string) *M.Mount {
	for _, m := range obj.Mounts {
		if m.SnapIdentifier == snapID {
			return m
		}
	}
	return nil
}

// Handlers

func (c *HandlerComp) volumeSeriesCreate(params ops.VolumeSeriesCreateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewVolumeSeriesCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil || params.Payload.ConsistencyGroupID == "" {
		// volumeSeriesValidateCreateArgs allows empty ConsistencyGroupID for the VSR, but it is disallowed on actual create
		err = c.eMissingMsg("consistencyGroupId")
		return ops.NewVolumeSeriesCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.RLock()
	defer c.RUnlock()
	if err = c.app.AuditLog.Ready(); err != nil {
		return ops.NewVolumeSeriesCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	spName, cgName, err := c.volumeSeriesValidateCreateArgs(ctx, ai, params.Payload)
	if err != nil {
		return ops.NewVolumeSeriesCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.DS.OpsVolumeSeries().Create(ctx, params.Payload)
	if err != nil {
		return ops.NewVolumeSeriesCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	// service plans are immutable so no need to post a reference to the ID
	msg := fmt.Sprintf("Created with service plan[%s] in consistency group[%s]", spName, cgName)
	c.app.AuditLog.Post(ctx, ai, centrald.VolumeSeriesCreateAction, obj.Meta.ID, obj.Name, obj.ConsistencyGroupID, false, msg)
	c.Log.Infof("VolumeSeries %s created [%s]", obj.Name, obj.Meta.ID)
	c.setScopeVolumeSeries(params.HTTPRequest, obj)
	return ops.NewVolumeSeriesCreateCreated().WithPayload(obj)
}

func (c *HandlerComp) volumeSeriesDelete(params ops.VolumeSeriesDeleteParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewVolumeSeriesDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var obj *M.VolumeSeries
	if obj, err = c.DS.OpsVolumeSeries().Fetch(ctx, params.ID); err == nil {
		c.Lock()
		defer c.Unlock()
		if err = c.app.AuditLog.Ready(); err != nil {
			return ops.NewVolumeSeriesDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if err = ai.CapOK(centrald.VolumeSeriesOwnerCap, obj.AccountID); err != nil {
			c.app.AuditLog.Post(ctx, ai, centrald.VolumeSeriesDeleteAction, obj.Meta.ID, obj.Name, "", true, "Delete unauthorized")
			return ops.NewVolumeSeriesDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if obj.VolumeSeriesState != com.VolStateDeleting {
			err = &centrald.Error{M: "VolumeSeries is not in DELETING state", C: centrald.ErrorExists.C}
			return ops.NewVolumeSeriesDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if len(obj.Mounts) > 0 {
			err = &centrald.Error{M: "VolumeSeries HEAD or snapshots are still mounted", C: centrald.ErrorExists.C}
			return ops.NewVolumeSeriesDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		var parcelsExist bool
		for _, v := range obj.StorageParcels {
			if swag.Int64Value(v.SizeBytes) > 0 {
				parcelsExist = true
				break
			}
		}
		if parcelsExist {
			err = &centrald.Error{M: "Storage parcels are still associated with this volume series", C: centrald.ErrorExists.C}
		} else if err = c.DS.OpsVolumeSeries().Delete(ctx, params.ID); err == nil {
			c.app.AuditLog.Post(ctx, ai, centrald.VolumeSeriesDeleteAction, obj.Meta.ID, obj.Name, "", false, "Deleted")
			c.Log.Infof("VolumeSeries %s deleted [%s]", obj.Name, obj.Meta.ID)
			c.setScopeVolumeSeries(params.HTTPRequest, obj)
			return ops.NewVolumeSeriesDeleteNoContent()
		}
	}
	return ops.NewVolumeSeriesDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
}

func (c *HandlerComp) volumeSeriesFetch(params ops.VolumeSeriesFetchParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewVolumeSeriesFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.DS.OpsVolumeSeries().Fetch(ctx, params.ID)
	if err == nil {
		err = c.volumeSeriesFetchFilter(ai, obj)
	}
	if err != nil {
		return ops.NewVolumeSeriesFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewVolumeSeriesFetchOK().WithPayload(obj)
}

func (c *HandlerComp) volumeSeriesList(params ops.VolumeSeriesListParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewVolumeSeriesListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var aggregations []*centrald.Aggregation
	var list []*M.VolumeSeries
	var count int
	// Aggregate() is implemented in the datastore, so must modify params to enforce RBAC by adding additional query filters instead of using volumeSeriesFetchFilter.
	params.AccountID, params.TenantAccountID, err = c.ops.constrainEitherOrQueryAccounts(ctx, ai, params.AccountID, centrald.VolumeSeriesOwnerCap, params.TenantAccountID, centrald.VolumeSeriesFetchCap)

	// if both AccountID and TenantAccountID are now nil and the caller is not internal, all accounts are filtered out, skip the query
	if err == nil && (params.AccountID != nil || params.TenantAccountID != nil || ai.Internal()) {
		if len(params.Sum) > 0 {
			aggregations, count, err = c.DS.OpsVolumeSeries().Aggregate(ctx, params)
		} else {
			list, err = c.DS.OpsVolumeSeries().List(ctx, params)
			count = len(list)
		}
	}
	if err != nil {
		return ops.NewVolumeSeriesListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewVolumeSeriesListOK().WithPayload(list).WithTotalCount(int64(count)).WithAggregations(c.makeAggregationResult(aggregations))
}

func (c *HandlerComp) volumeSeriesNewID(params ops.VolumeSeriesNewIDParams) middleware.Responder {
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewVolumeSeriesNewIDDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err = ai.InternalOK(); err != nil {
		return ops.NewVolumeSeriesNewIDDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewVolumeSeriesNewIDOK().WithPayload(&M.ValueType{Kind: com.ValueTypeString, Value: c.DS.OpsVolumeSeries().NewID()})
}

func (c *HandlerComp) volumeSeriesUpdate(params ops.VolumeSeriesUpdateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var uP = [centrald.NumActionTypes][]string{
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    params.Set,
	}
	ua, err := c.makeStdUpdateArgs(c.volumeSeriesMutableNameMap(), params.ID, params.Version, uP)
	if err != nil {
		return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err = centrald.ErrorUpdateInvalidRequest
		return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.IsModified("Name") && params.Payload.Name == "" {
		err := c.eUpdateInvalidMsg("non-empty name is required")
		return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	vsState := ""
	if ua.IsModified("VolumeSeriesState") {
		if !c.app.ValidateVolumeSeriesState(params.Payload.VolumeSeriesState) {
			err := c.eUpdateInvalidMsg("invalid volumeSeriesState")
			return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		vsState = params.Payload.VolumeSeriesState
	}
	if ua.IsModified("BoundCspDomainID") {
		err := c.eUpdateInvalidMsg("boundCspDomainId may not be set explicitly")
		return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.RLock()
	defer c.RUnlock()
	obj, err := c.DS.OpsVolumeSeries().Fetch(ctx, params.ID)
	if err != nil {
		return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	} else if ua.Version == 0 {
		ua.Version = int32(obj.Meta.Version)
	} else if int32(obj.Meta.Version) != ua.Version {
		err = centrald.ErrorIDVerNotFound
		return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err = c.app.AuditLog.Ready(); err != nil {
		return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.OthersModified("ConsistencyGroupID", "Description", "Name", "Tags") {
		if err = ai.InternalOK(); err != nil {
			c.app.AuditLog.Post(ctx, ai, centrald.VolumeSeriesUpdateAction, obj.Meta.ID, obj.Name, "", true, "Update unauthorized")
			return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	} else {
		if err = ai.CapOK(centrald.VolumeSeriesOwnerCap, M.ObjIDMutable(obj.AccountID)); err != nil {
			c.app.AuditLog.Post(ctx, ai, centrald.VolumeSeriesUpdateAction, obj.Meta.ID, obj.Name, "", true, "Update unauthorized")
			return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if vsState == "" {
		vsState = obj.VolumeSeriesState
	}
	if ai.Internal() && ai.AccountID == "" {
		// Assume caller is performing the operation on behalf of the owner account, TBD clients should pass the X-Account header
		ai.AccountID = string(obj.AccountID)
		aObj, err := c.DS.OpsAccount().Fetch(ctx, ai.AccountID)
		if err != nil {
			return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		ai.AccountName = string(aObj.Name)
		ai.TenantAccountID = string(obj.TenantAccountID)
	}

	var boundMsg, cgMsg string // these messages must be logged with their corresponding RefID
	auditAttrs := []string{}
	eventAttrs := []string{}
	clusterID := obj.BoundClusterID
	if ua.IsModified("BoundClusterID") && clusterID != params.Payload.BoundClusterID {
		params.Payload.BoundCspDomainID = ""
		if params.Payload.BoundClusterID != "" {
			if vsState == com.VolStateUnbound {
				err = c.eUpdateInvalidMsg("invalid boundClusterId in volumeSeriesState %s", vsState)
				return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			clObj, err := c.DS.OpsCluster().Fetch(ctx, string(params.Payload.BoundClusterID))
			if err == nil {
				err = c.clusterFetchFilter(ai, clObj)
			}
			if err != nil {
				if err == centrald.ErrorNotFound {
					err = c.eUpdateInvalidMsg("invalid boundClusterId")
				}
				return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			boundMsg = fmt.Sprintf("Bound to cluster[%s]", clObj.Name)
			params.Payload.BoundCspDomainID = clObj.CspDomainID
		} else if vsState != com.VolStateDeleting && vsState != com.VolStateUnbound {
			err = c.eUpdateInvalidMsg("empty boundClusterId in volumeSeriesState %s", vsState)
			return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		clusterID = params.Payload.BoundClusterID
		bda := ua.FindUpdateAttr("BoundCspDomainID")
		bda.Actions[centrald.UpdateSet].FromBody = true
	}
	if ua.IsModified("ConfiguredNodeID") && params.Payload.ConfiguredNodeID != "" {
		nObj, err := c.DS.OpsNode().Fetch(ctx, string(params.Payload.ConfiguredNodeID))
		if err == centrald.ErrorNotFound || (err == nil && nObj.ClusterID != clusterID) {
			err = c.eUpdateInvalidMsg("invalid configuredNodeId: %s", params.Payload.ConfiguredNodeID)
		}
		if err != nil {
			return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	rootStorageID := string(obj.RootStorageID)
	if ua.IsModified("RootStorageID") {
		if params.Payload.RootStorageID != "" {
			sObj, err := c.DS.OpsStorage().Fetch(ctx, string(params.Payload.RootStorageID))
			if err == nil && sObj.AccountID != M.ObjID(obj.AccountID) {
				err = &centrald.Error{M: fmt.Sprintf("storage %s is not available to the account", params.Payload.RootStorageID), C: centrald.ErrorUnauthorizedOrForbidden.C}
			}
			if err != nil {
				if err == centrald.ErrorNotFound {
					err = c.eUpdateInvalidMsg("invalid rootStorageId")
				}
				return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
		}
		rootStorageID = string(params.Payload.RootStorageID)
	}
	if ua.IsModified("ServicePlanID") && params.Payload.ServicePlanID != obj.ServicePlanID {
		spObj, err := c.DS.OpsServicePlan().Fetch(ctx, string(params.Payload.ServicePlanID))
		if err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eUpdateInvalidMsg("invalid servicePlanId")
			}
			return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if spObj.State == centrald.RetiredState {
			err = c.eUpdateInvalidMsg("servicePlan %s is %s", spObj.Name, centrald.RetiredState)
			return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if !util.Contains(spObj.Accounts, obj.AccountID) {
			err = &centrald.Error{M: "servicePlan is not available to the account", C: centrald.ErrorUnauthorizedOrForbidden.C}
			return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		eventAttrs = append(eventAttrs, fmt.Sprintf("service plan [%s]", spObj.Name))
	}
	if ua.IsModified("ConsistencyGroupID") && params.Payload.ConsistencyGroupID != obj.ConsistencyGroupID {
		if params.Payload.ConsistencyGroupID == "" {
			if vsState != com.VolStateDeleting {
				err := c.eUpdateInvalidMsg("non-empty consistencyGroupId is required")
				return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
		} else {
			cgObj, err := c.DS.OpsConsistencyGroup().Fetch(ctx, string(params.Payload.ConsistencyGroupID))
			if err != nil {
				if err == centrald.ErrorNotFound {
					err = c.eUpdateInvalidMsg("invalid consistencyGroupId")
				}
				return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
			} else if cgObj.AccountID != obj.AccountID {
				err = &centrald.Error{M: "consistencyGroup is not available to the account", C: centrald.ErrorUnauthorizedOrForbidden.C}
				return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			// Must ensure that the volume series is not currently involved in a VOL_SNAPSHOT_CREATE operation
			lParams := volume_series_request.VolumeSeriesRequestListParams{
				VolumeSeriesID: swag.String(string(obj.Meta.ID)),
				IsTerminated:   swag.Bool(false),
			}
			list, err := c.DS.OpsVolumeSeriesRequest().List(ctx, lParams)
			if err != nil {
				return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			for _, vsr := range list {
				if vsr.RequestedOperations[0] == com.VolReqOpVolCreateSnapshot {
					err = c.eRequestInConflict("modification of consistencyGroupId blocked by VOL_SNAPSHOT_CREATE operation %s", vsr.Meta.ID)
					return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
				}
			}
			cgMsg = fmt.Sprintf("Updated consistency group [%s]", cgObj.Name)
		}
	}
	checkHeadRemoved := false
	if a := ua.FindUpdateAttr("Mounts"); a != nil && a.IsModified() {
		action := a.Actions[centrald.UpdateAppend]
		isRemove := false
		if !action.FromBody {
			action = a.Actions[centrald.UpdateRemove]
			isRemove = action.FromBody
		}
		if !action.FromBody {
			action = a.Actions[centrald.UpdateSet]
			checkHeadRemoved = true
		}
		for i, m := range params.Payload.Mounts {
			if _, exists := action.Indexes[i]; action.FromBody || exists {
				var nObj *M.Node
				if isRemove {
					if m.SnapIdentifier == com.VolMountHeadIdentifier {
						checkHeadRemoved = true
					}
					continue
				}
				if !c.app.ValidateMountMode(m.MountMode) {
					err = c.eUpdateInvalidMsg("invalid mountMode: %s", m.MountMode)
				} else if !c.app.ValidateMountState(m.MountState) {
					err = c.eUpdateInvalidMsg("invalid mountState: %s", m.MountState)
				} else if m.MountState == com.VolMountStateMounted && m.MountedNodeDevice == "" {
					err = c.eUpdateInvalidMsg("mountedNodeDevice is required when mountState is %s", m.MountState)
				} else {
					prevMount := c.volumeSeriesFindMount(obj, m.SnapIdentifier)
					if prevMount == nil || prevMount.MountedNodeID != m.MountedNodeID {
						nObj, err = c.DS.OpsNode().Fetch(ctx, string(m.MountedNodeID))
						if err == centrald.ErrorNotFound || (err == nil && nObj.ClusterID != clusterID) {
							err = c.eUpdateInvalidMsg("invalid mountedNodeId: %s", m.MountedNodeID)
						}
					}
					if m.MountState == com.VolMountStateMounted && m.SnapIdentifier == com.VolMountHeadIdentifier && (prevMount == nil || prevMount.MountState != com.VolMountStateMounted) {
						auditAttrs = append(auditAttrs, "mounted "+com.VolMountHeadIdentifier)
					}
				}
				if err != nil {
					return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
				}
			}
		}
		if checkHeadRemoved {
			// for audit log: only report unmounted if the HEAD was actually previously mounted
			if m := c.volumeSeriesFindMount(obj, com.VolMountHeadIdentifier); m == nil || !util.Contains([]string{com.VolMountStateMounted, com.VolMountStateUnmounting}, m.MountState) {
				checkHeadRemoved = false
			}
		}
	}
	if ua.IsModified("SizeBytes") && swag.Int64Value(params.Payload.SizeBytes) != swag.Int64Value(obj.SizeBytes) {
		eventAttrs = append(eventAttrs, "sizeBytes")
	}
	if ua.IsModified("SpaAdditionalBytes") {
		oldValue, newValue := swag.Int64Value(obj.SpaAdditionalBytes), swag.Int64Value(params.Payload.SpaAdditionalBytes)
		if oldValue < newValue {
			eventAttrs = append(eventAttrs, "max IOPS increase")
		} else if oldValue > newValue {
			eventAttrs = append(eventAttrs, "max IOPS decrease")
		}
	}
	if a := ua.FindUpdateAttr("CapacityAllocations"); a != nil && a.IsModified() && !a.Actions[centrald.UpdateRemove].FromBody {
		action := a.Actions[centrald.UpdateAppend]
		if !action.FromBody {
			action = a.Actions[centrald.UpdateSet]
		}
		for k := range params.Payload.CapacityAllocations {
			if _, exists := action.Fields[k]; action.FromBody || exists {
				pObj, err := c.DS.OpsPool().Fetch(ctx, k)
				if err == nil && pObj.AuthorizedAccountID != obj.AccountID {
					err = &centrald.Error{M: fmt.Sprintf("pool %s is not available to the account", k), C: centrald.ErrorUnauthorizedOrForbidden.C}
				}
				if err != nil {
					if err == centrald.ErrorNotFound {
						err = c.eUpdateInvalidMsg("invalid poolId: %s", k)
					}
					return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
				}
			}
		}
	}
	foundRootParcel := false
	if a := ua.FindUpdateAttr("StorageParcels"); a != nil && a.IsModified() {
		if !a.Actions[centrald.UpdateRemove].FromBody {
			action := a.Actions[centrald.UpdateAppend]
			if !action.FromBody {
				action = a.Actions[centrald.UpdateSet]
			}
			for k := range params.Payload.StorageParcels {
				if _, exists := action.Fields[k]; action.FromBody || exists {
					sObj, err := c.DS.OpsStorage().Fetch(ctx, k)
					if err == nil && sObj.AccountID != M.ObjID(obj.AccountID) {
						err = &centrald.Error{M: fmt.Sprintf("storage %s is not available to the account", k), C: centrald.ErrorUnauthorizedOrForbidden.C}
					}
					if err != nil {
						if err == centrald.ErrorNotFound {
							err = c.eUpdateInvalidMsg("invalid storageId: %s", k)
						}
						return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
					}
					if k == rootStorageID {
						foundRootParcel = true
					}
				}
			}
		} else if _, exists := params.Payload.StorageParcels[rootStorageID]; exists {
			err = c.eUpdateInvalidMsg("remove storageParcels removes rootStorageId")
			return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	// if rootStorageID is not found in the payload.storageParcels, check the obj
	if !foundRootParcel && rootStorageID != "" && obj != nil {
		if _, foundRootParcel = obj.StorageParcels[rootStorageID]; !foundRootParcel {
			err = c.eUpdateInvalidMsg("rootStorageId not found in storageParcels")
			return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if obj, err = c.DS.OpsVolumeSeries().Update(ctx, ua, params.Payload); err != nil {
		return ops.NewVolumeSeriesUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if checkHeadRemoved {
		if c.volumeSeriesFindMount(obj, com.VolMountHeadIdentifier) == nil {
			auditAttrs = append(auditAttrs, "unmounted "+com.VolMountHeadIdentifier)
		}
	}
	if boundMsg != "" {
		c.app.AuditLog.Post(ctx, ai, centrald.VolumeSeriesUpdateAction, M.ObjID(params.ID), obj.Name, obj.BoundClusterID, false, boundMsg)
	}
	if cgMsg != "" {
		c.app.AuditLog.Post(ctx, ai, centrald.VolumeSeriesUpdateAction, M.ObjID(params.ID), obj.Name, obj.ConsistencyGroupID, false, cgMsg)
	}
	if len(auditAttrs) > 0 {
		msg := fmt.Sprintf("Updated %s", strings.Join(auditAttrs, ", "))
		c.app.AuditLog.Post(ctx, ai, centrald.VolumeSeriesUpdateAction, M.ObjID(params.ID), obj.Name, "", false, msg)
	}
	if len(eventAttrs) > 0 {
		msg := fmt.Sprintf("Updated %s", strings.Join(eventAttrs, ", "))
		c.app.AuditLog.Event(ctx, ai, centrald.VolumeSeriesUpdateAction, M.ObjID(params.ID), obj.Name, "", false, msg)
	}
	c.setScopeVolumeSeries(params.HTTPRequest, obj)
	return ops.NewVolumeSeriesUpdateOK().WithPayload(obj)
}

func (c *HandlerComp) volumeSeriesPVSpecFetch(params ops.VolumeSeriesPVSpecFetchParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewVolumeSeriesPVSpecFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if swag.StringValue(params.FsType) != com.FSTypeXfs && swag.StringValue(params.FsType) != com.FSTypeExt4 {
		err = c.eUpdateInvalidMsg("Unsupported file system type %s", swag.StringValue(params.FsType))
		return ops.NewVolumeSeriesPVSpecFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.DS.OpsVolumeSeries().Fetch(ctx, params.ID)
	if err == nil {
		err = c.volumeSeriesFetchFilter(ai, obj)
	}
	if err != nil {
		return ops.NewVolumeSeriesPVSpecFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	// check if volume is in bound, in-use, provisioned or configured state.
	if !(obj.VolumeSeriesState == com.VolStateBound ||
		obj.VolumeSeriesState == com.VolStateInUse ||
		obj.VolumeSeriesState == com.VolStateConfigured ||
		obj.VolumeSeriesState == com.VolStateProvisioned) {
		err = c.eUpdateInvalidMsg("Volume Series is not bound to the cluster")
		return ops.NewVolumeSeriesPVSpecFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	clObj, err := c.DS.OpsCluster().Fetch(ctx, string(obj.BoundClusterID))
	if err == nil {
		err = c.clusterFetchFilter(ai, clObj)
	}
	if err != nil {
		return ops.NewVolumeSeriesPVSpecFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	sysObj, err := c.DS.OpsSystem().Fetch()
	if err != nil {
		return ops.NewVolumeSeriesPVSpecFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	clVersion := swag.StringValue(params.ClusterVersion)
	if clVersion == "" {
		clVersion = clObj.ClusterVersion
	}
	clusterClient := c.lookupClusterClient(clObj.ClusterType)
	if clusterClient == nil {
		err = &centrald.Error{M: fmt.Sprintf("Cluster type '%s' not found", clObj.ClusterType), C: centrald.ErrorNotFound.C}
		return ops.NewVolumeSeriesPVSpecFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	size := (swag.Int64Value(obj.SizeBytes)) / int64(units.GiB)
	if params.Capacity != nil {
		size = swag.Int64Value(params.Capacity) / int64(units.GiB)
	}

	args := &cluster.PVSpecArgs{
		AccountID:      string(obj.AccountID),
		ClusterVersion: clVersion,
		VolumeID:       string(params.ID),
		SystemID:       string(sysObj.Meta.ID),
		FsType:         swag.StringValue(params.FsType),
		Capacity:       fmt.Sprintf("%dGi", size),
	}
	pvSpec, err := clusterClient.GetPVSpec(params.HTTPRequest.Context(), args)

	if err != nil {
		err = c.eInvalidData("%s", err.Error())
		return ops.NewVolumeSeriesPVSpecFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}

	ret := ops.NewVolumeSeriesPVSpecFetchOK()
	ret.Payload = &M.VolumeSeriesPVSpecFetchOKBody{}
	ret.Payload.PvSpec = pvSpec.PvSpec
	ret.Payload.Format = pvSpec.Format
	return ret
}

func (c *HandlerComp) setScopeVolumeSeries(r *http.Request, obj *M.VolumeSeries) {
	m := crude.ScopeMap{
		"accountId":     string(obj.AccountID),
		"servicePlanId": string(obj.ServicePlanID),
	}
	if obj.BoundClusterID != "" {
		m["clusterId"] = string(obj.BoundClusterID) // not "boundClusterId", clusterd uses clusterId in its scope
	}
	if obj.ConfiguredNodeID != "" {
		m["nodeId"] = string(obj.ConfiguredNodeID)
	}
	if obj.ConsistencyGroupID != "" {
		m["consistencyGroupId"] = string(obj.ConsistencyGroupID)
	}
	c.addNewObjIDToScopeMap(obj, m)
	c.app.CrudeOps.SetScope(r, m, obj)
}
