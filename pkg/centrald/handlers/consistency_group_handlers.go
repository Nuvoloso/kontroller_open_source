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
	"sort"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/consistency_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/snapshot"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/go-openapi/runtime/middleware"
)

// Register handlers for ConsistencyGroup
func (c *HandlerComp) consistencyGroupRegisterHandlers() {
	c.app.API.ConsistencyGroupConsistencyGroupCreateHandler = ops.ConsistencyGroupCreateHandlerFunc(c.consistencyGroupCreate)
	c.app.API.ConsistencyGroupConsistencyGroupDeleteHandler = ops.ConsistencyGroupDeleteHandlerFunc(c.consistencyGroupDelete)
	c.app.API.ConsistencyGroupConsistencyGroupFetchHandler = ops.ConsistencyGroupFetchHandlerFunc(c.consistencyGroupFetch)
	c.app.API.ConsistencyGroupConsistencyGroupListHandler = ops.ConsistencyGroupListHandlerFunc(c.consistencyGroupList)
	c.app.API.ConsistencyGroupConsistencyGroupUpdateHandler = ops.ConsistencyGroupUpdateHandlerFunc(c.consistencyGroupUpdate)
}

var nmConsistencyGroupMutable JSONToAttrNameMap

func (c *HandlerComp) consistencyGroupMutableNameMap() JSONToAttrNameMap {
	if nmConsistencyGroupMutable == nil {
		c.cacheMux.Lock()
		defer c.cacheMux.Unlock()
		if nmConsistencyGroupMutable == nil {
			nmConsistencyGroupMutable = c.makeJSONToAttrNameMap(M.ConsistencyGroupMutable{})
		}
	}
	return nmConsistencyGroupMutable
}

// consistencyGroupFetchFilter verifies the caller has permission to fetch the object, returning an error if they do not.
// Owner is typically a subordinate account, but the tenant admin can also view them.
func (c *HandlerComp) consistencyGroupFetchFilter(ai *auth.Info, obj *M.ConsistencyGroup) error {
	err := ai.CapOK(centrald.VolumeSeriesOwnerCap, obj.AccountID)
	if err != nil {
		err = ai.CapOK(centrald.VolumeSeriesFetchCap, obj.TenantAccountID)
	}
	return err
}

// consistencyGroupApplyInheritedProperties silently inserts the default snapshot management policy values from the owner Account object.
func (c *HandlerComp) consistencyGroupApplyInheritedProperties(ctx context.Context, ai *auth.Info, obj *M.ConsistencyGroup, accountObj *M.Account) error {
	if obj.SnapshotManagementPolicy == nil || obj.SnapshotManagementPolicy.Inherited {
		aObj := accountObj
		if accountObj == nil {
			faObj, err := c.intAccountFetch(ctx, ai, string(obj.AccountID))
			if err != nil {
				return err
			}
			aObj = faObj
		}
		obj.SnapshotManagementPolicy = &M.SnapshotManagementPolicy{}
		*obj.SnapshotManagementPolicy = *aObj.SnapshotManagementPolicy
		obj.SnapshotManagementPolicy.Inherited = true
	}
	return nil
}

// Handlers

func (c *HandlerComp) consistencyGroupCreate(params ops.ConsistencyGroupCreateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewConsistencyGroupCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil || params.Payload.Name == "" || len(params.Payload.ApplicationGroupIds) == 0 {
		err := c.eMissingMsg("name and applicationGroupIds are required")
		return ops.NewConsistencyGroupCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.RLock()
	defer c.RUnlock()
	if err = c.app.AuditLog.Ready(); err != nil {
		return ops.NewConsistencyGroupCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err = ai.CapOK(centrald.VolumeSeriesOwnerCap); err != nil {
		c.app.AuditLog.Post(ctx, ai, centrald.ConsistencyGroupCreateAction, "", params.Payload.Name, "", true, "Create unauthorized")
		return ops.NewConsistencyGroupCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var aObj *M.Account
	if ai.AccountID != "" {
		// use auth info when specified (otherwise caller has internal role)
		params.Payload.AccountID = M.ObjIDMutable(ai.AccountID)
		params.Payload.TenantAccountID = M.ObjIDMutable(ai.TenantAccountID)
	} else {
		if aObj, err = c.intAccountFetch(ctx, ai, string(params.Payload.AccountID)); err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eMissingMsg("invalid accountId")
			}
			return ops.NewConsistencyGroupCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		params.Payload.TenantAccountID = aObj.TenantAccountID
	}
	agNames, err := c.validateApplicationGroupIds(ctx, params.Payload.AccountID, params.Payload.ApplicationGroupIds)
	if err != nil {
		if c.eError(err).Code == int32(centrald.ErrorUnauthorizedOrForbidden.C) {
			c.app.AuditLog.Post(ctx, ai, centrald.ConsistencyGroupCreateAction, "", params.Payload.Name, "", true, "Create unauthorized")
		}
		return ops.NewConsistencyGroupCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.DS.OpsConsistencyGroup().Create(ctx, params.Payload)
	if err != nil {
		return ops.NewConsistencyGroupCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.app.AuditLog.Post(ctx, ai, centrald.ConsistencyGroupCreateAction, obj.Meta.ID, obj.Name, obj.ApplicationGroupIds[0], false, fmt.Sprintf("Created in application group [%s]", agNames[0]))
	for i, id := range obj.ApplicationGroupIds[1:] {
		c.app.AuditLog.Post(ctx, ai, centrald.ConsistencyGroupUpdateAction, obj.Meta.ID, obj.Name, id, false, fmt.Sprintf("Added to application group [%s]", agNames[i+1]))
	}
	c.Log.Infof("ConsistencyGroup %s created [%s]", obj.Name, obj.Meta.ID)
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	c.consistencyGroupApplyInheritedProperties(ctx, ai, obj, aObj) // error impossible as the account is already fetched
	return ops.NewConsistencyGroupCreateCreated().WithPayload(obj)
}

func (c *HandlerComp) consistencyGroupDelete(params ops.ConsistencyGroupDeleteParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewConsistencyGroupDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var obj *M.ConsistencyGroup
	if obj, err = c.DS.OpsConsistencyGroup().Fetch(ctx, params.ID); err == nil {
		c.Lock()
		defer c.Unlock()
		if err = c.app.AuditLog.Ready(); err == nil {
			if err = ai.CapOK(centrald.VolumeSeriesOwnerCap, obj.AccountID); err != nil {
				c.app.AuditLog.Post(ctx, ai, centrald.ConsistencyGroupDeleteAction, obj.Meta.ID, obj.Name, "", true, "Delete unauthorized")
				return ops.NewConsistencyGroupDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			var n int
			if n, err = c.DS.OpsVolumeSeries().Count(ctx, volume_series.VolumeSeriesListParams{ConsistencyGroupID: &params.ID}, 1); err == nil {
				if n != 0 {
					err = &centrald.Error{M: "consistency group contains one or more volume series", C: centrald.ErrorExists.C}
					return ops.NewConsistencyGroupDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
				}
			} else {
				return ops.NewConsistencyGroupDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			if n, err = c.DS.OpsSnapshot().Count(ctx, snapshot.SnapshotListParams{ConsistencyGroupID: &params.ID}, 1); err == nil {
				if n != 0 {
					err = &centrald.Error{M: "consistency group is referenced by one or more volume series snapshots", C: centrald.ErrorExists.C}
					return ops.NewConsistencyGroupDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
				}
			} else {
				return ops.NewConsistencyGroupDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			if err = c.DS.OpsConsistencyGroup().Delete(ctx, params.ID); err == nil {
				c.app.AuditLog.Post(ctx, ai, centrald.ConsistencyGroupDeleteAction, obj.Meta.ID, obj.Name, "", false, "Deleted")
				c.Log.Infof("ConsistencyGroup %s deleted [%s]", obj.Name, obj.Meta.ID)
				c.setDefaultObjectScope(params.HTTPRequest, obj)
				return ops.NewConsistencyGroupDeleteNoContent()
			}
		}
	}
	return ops.NewConsistencyGroupDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
}

func (c *HandlerComp) consistencyGroupFetch(params ops.ConsistencyGroupFetchParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewConsistencyGroupFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.intConsistencyGroupFetch(ctx, ai, params.ID, nil)
	if err != nil {
		return ops.NewConsistencyGroupFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewConsistencyGroupFetchOK().WithPayload(obj)
}

func (c *HandlerComp) intConsistencyGroupFetch(ctx context.Context, ai *auth.Info, id string, aObj *M.Account) (*M.ConsistencyGroup, error) {
	obj, err := c.DS.OpsConsistencyGroup().Fetch(ctx, id)
	if err == nil {
		err = c.consistencyGroupFetchFilter(ai, obj)
		if err == nil {
			err = c.consistencyGroupApplyInheritedProperties(ctx, ai, obj, aObj)
		}
	}
	return obj, err
}

func (c *HandlerComp) consistencyGroupList(params ops.ConsistencyGroupListParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewConsistencyGroupListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var list []*M.ConsistencyGroup
	// rather than querying for the entire list and performing RBAC filtering in the handler, constrain the query which should be more efficient
	params.AccountID, params.TenantAccountID, err = c.ops.constrainEitherOrQueryAccounts(ctx, ai, params.AccountID, centrald.VolumeSeriesOwnerCap, params.TenantAccountID, centrald.VolumeSeriesFetchCap)

	// if both AccountID and TenantAccountID are now nil and the caller is not internal, all accounts are filtered out, skip the query
	if err == nil && (params.AccountID != nil || params.TenantAccountID != nil || ai.Internal()) {
		list, err = c.DS.OpsConsistencyGroup().List(ctx, params)
	}
	if err != nil {
		return ops.NewConsistencyGroupListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	ret := make([]*M.ConsistencyGroup, 0, len(list))
	for _, obj := range list {
		if err = c.consistencyGroupApplyInheritedProperties(ctx, ai, obj, nil); err == nil {
			ret = append(ret, obj)
		}
	}
	return ops.NewConsistencyGroupListOK().WithPayload(ret)
}

func (c *HandlerComp) consistencyGroupUpdate(params ops.ConsistencyGroupUpdateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewConsistencyGroupUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var uP = [centrald.NumActionTypes][]string{
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    params.Set,
	}
	ua, err := c.makeStdUpdateArgs(c.consistencyGroupMutableNameMap(), params.ID, params.Version, uP)
	if err != nil {
		return ops.NewConsistencyGroupUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err = centrald.ErrorUpdateInvalidRequest
		return ops.NewConsistencyGroupUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.IsModified("Name") && params.Payload.Name == "" {
		err := c.eUpdateInvalidMsg("non-empty name is required")
		return ops.NewConsistencyGroupUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.RLock()
	defer c.RUnlock()
	oObj, err := c.DS.OpsConsistencyGroup().Fetch(ctx, params.ID)
	if err != nil {
		return ops.NewConsistencyGroupUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	} else if ua.Version == 0 {
		ua.Version = int32(oObj.Meta.Version)
	} else if int32(oObj.Meta.Version) != ua.Version {
		err = centrald.ErrorIDVerNotFound
		return ops.NewConsistencyGroupUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err = c.app.AuditLog.Ready(); err != nil {
		return ops.NewConsistencyGroupUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.IsModified("SystemTags") {
		if err = ai.InternalOK(); err != nil {
			return ops.NewConsistencyGroupUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	} else {
		if err = ai.CapOK(centrald.VolumeSeriesOwnerCap, M.ObjIDMutable(oObj.AccountID)); err != nil {
			c.app.AuditLog.Post(ctx, ai, centrald.ConsistencyGroupUpdateAction, oObj.Meta.ID, oObj.Name, "", true, "Update unauthorized")
			return ops.NewConsistencyGroupUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	var aObj *M.Account
	if ua.IsModified("SnapshotManagementPolicy") {
		if err = c.validateSnapshotManagementPolicy(params.Payload.SnapshotManagementPolicy); err != nil {
			return ops.NewConsistencyGroupUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if aObj, err = c.intAccountFetch(ctx, ai, string(oObj.AccountID)); err != nil {
			return ops.NewConsistencyGroupUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	var addedAGs, removedAGs []*models.ApplicationGroup
	if a := ua.FindUpdateAttr("ApplicationGroupIds"); a != nil && a.IsModified() {
		addedAGs, removedAGs, err = c.validateApplicationGroupUpdate(ctx, ai, centrald.ConsistencyGroupUpdateAction, params.ID, models.ObjName(oObj.Name), oObj.AccountID, a, oObj.ApplicationGroupIds, params.Payload.ApplicationGroupIds)
		if err != nil {
			return ops.NewConsistencyGroupUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	obj, err := c.DS.OpsConsistencyGroup().Update(ctx, ua, params.Payload)
	if err != nil {
		return ops.NewConsistencyGroupUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.consistencyGroupApplyInheritedProperties(ctx, ai, obj, aObj) // ignore errors
	for _, ag := range addedAGs {
		c.app.AuditLog.Post(ctx, ai, centrald.ConsistencyGroupUpdateAction, models.ObjID(params.ID), models.ObjName(oObj.Name), models.ObjIDMutable(ag.Meta.ID), false, fmt.Sprintf("Added to application group [%s]", ag.Name))
	}
	for _, ag := range removedAGs {
		c.app.AuditLog.Post(ctx, ai, centrald.ConsistencyGroupUpdateAction, models.ObjID(params.ID), models.ObjName(oObj.Name), models.ObjIDMutable(ag.Meta.ID), false, fmt.Sprintf("Removed from application group [%s]", ag.Name))
	}
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	return ops.NewConsistencyGroupUpdateOK().WithPayload(obj)
}

// validateApplicationGroupUpdate determines if an update to ApplicationGroupIds is allowed.
// Lists of objects sorted by name corresponding to the added and removed IDs are returned on success.
// Caller must hold c.RLock
func (c *HandlerComp) validateApplicationGroupUpdate(ctx context.Context, ai *auth.Info, action centrald.AuditAction, objID string, objName models.ObjName, objAccountID models.ObjIDMutable, a *centrald.UpdateAttr, old, upd []models.ObjIDMutable) (added []*models.ApplicationGroup, removed []*models.ApplicationGroup, err error) {
	toAdd := map[models.ObjIDMutable]struct{}{}
	toRemove := map[models.ObjIDMutable]struct{}{}
	inOld := map[models.ObjIDMutable]struct{}{}
	for _, id := range old {
		inOld[id] = struct{}{}
	}
	if a.Actions[centrald.UpdateAppend].FromBody {
		for _, id := range upd {
			if _, exists := inOld[id]; !exists {
				toAdd[id] = struct{}{}
			}
		}
	} else if a.Actions[centrald.UpdateRemove].FromBody {
		for _, id := range upd {
			if _, exists := inOld[id]; exists {
				toRemove[id] = struct{}{}
			}
		}
		if len(toRemove) == len(inOld) {
			return nil, nil, c.eUpdateInvalidMsg("cannot remove all applicationGroupIds")
		}
	} else {
		// 2 passes to determine which IDs are added and removed from the original list.
		// Because the replacement list can be sparse, build up the full new list during the 1st pass
		action := a.Actions[centrald.UpdateSet]
		inNew := map[models.ObjIDMutable]struct{}{}
		if action.FromBody && len(upd) == 0 {
			return nil, nil, c.eUpdateInvalidMsg("non-empty applicationGroupIds required")
		}
		for i, id := range upd {
			if _, exists := action.Indexes[i]; action.FromBody || exists {
				inNew[id] = struct{}{}
				if _, exists = toAdd[id]; exists {
					return nil, nil, c.eMissingMsg("applicationGroupIds must be unique: %s", id)
				}
				if _, exists = inOld[id]; !exists {
					toAdd[id] = struct{}{}
				}
			}
		}
		for _, id := range old {
			if _, exists := inNew[id]; !exists {
				toRemove[id] = struct{}{}
			}
		}
	}
	added = make([]*models.ApplicationGroup, 0, len(toAdd))
	for id := range toAdd {
		agObj, err := c.DS.OpsApplicationGroup().Fetch(ctx, string(id))
		if err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eUpdateInvalidMsg("invalid applicationGroupId: %s", id)
			}
		} else if agObj.AccountID != objAccountID {
			err = &centrald.Error{M: fmt.Sprintf("applicationGroup %s is not available to the account", id), C: centrald.ErrorUnauthorizedOrForbidden.C}
			c.app.AuditLog.Post(ctx, ai, centrald.ConsistencyGroupUpdateAction, models.ObjID(objID), objName, "", true, "Update unauthorized")
		}
		if err != nil {
			return nil, nil, err
		}
		added = append(added, agObj)
	}
	sort.SliceStable(added, func(i, j int) bool { return added[i].Name < added[j].Name })
	removed = make([]*models.ApplicationGroup, 0, len(toRemove))
	for id := range toRemove {
		agObj, err := c.DS.OpsApplicationGroup().Fetch(ctx, string(id))
		if err != nil {
			return nil, nil, err
		}
		removed = append(removed, agObj)
	}
	sort.SliceStable(removed, func(i, j int) bool { return removed[i].Name < removed[j].Name })
	return added, removed, nil
}
