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
	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/application_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/consistency_group"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/go-openapi/runtime/middleware"
)

// Register handlers for ApplicationGroup
func (c *HandlerComp) applicationGroupRegisterHandlers() {
	c.app.API.ApplicationGroupApplicationGroupCreateHandler = ops.ApplicationGroupCreateHandlerFunc(c.applicationGroupCreate)
	c.app.API.ApplicationGroupApplicationGroupDeleteHandler = ops.ApplicationGroupDeleteHandlerFunc(c.applicationGroupDelete)
	c.app.API.ApplicationGroupApplicationGroupFetchHandler = ops.ApplicationGroupFetchHandlerFunc(c.applicationGroupFetch)
	c.app.API.ApplicationGroupApplicationGroupListHandler = ops.ApplicationGroupListHandlerFunc(c.applicationGroupList)
	c.app.API.ApplicationGroupApplicationGroupUpdateHandler = ops.ApplicationGroupUpdateHandlerFunc(c.applicationGroupUpdate)
}

var nmApplicationGroupMutable JSONToAttrNameMap

func (c *HandlerComp) applicationGroupMutableNameMap() JSONToAttrNameMap {
	if nmApplicationGroupMutable == nil {
		c.cacheMux.Lock()
		defer c.cacheMux.Unlock()
		if nmApplicationGroupMutable == nil {
			nmApplicationGroupMutable = c.makeJSONToAttrNameMap(M.ApplicationGroupMutable{})
		}
	}
	return nmApplicationGroupMutable
}

// applicationGroupFetchFilter verifies the caller has permission to fetch the object, returning an error if they do not.
// Owner is typically a subordinate account, but the tenant admin can also view them.
func (c *HandlerComp) applicationGroupFetchFilter(ai *auth.Info, obj *M.ApplicationGroup) error {
	err := ai.CapOK(centrald.VolumeSeriesOwnerCap, obj.AccountID)
	if err != nil {
		err = ai.CapOK(centrald.VolumeSeriesFetchCap, obj.TenantAccountID)
	}
	return err
}

// Handlers

func (c *HandlerComp) applicationGroupCreate(params ops.ApplicationGroupCreateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewApplicationGroupCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil || params.Payload.Name == "" {
		err := c.eMissingMsg("name")
		return ops.NewApplicationGroupCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.RLock()
	defer c.RUnlock()
	if err = c.app.AuditLog.Ready(); err != nil {
		return ops.NewApplicationGroupCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err = ai.CapOK(centrald.VolumeSeriesOwnerCap); err != nil {
		c.app.AuditLog.Post(ctx, ai, centrald.ApplicationGroupCreateAction, "", params.Payload.Name, "", true, "Create unauthorized")
		return ops.NewApplicationGroupCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ai.AccountID != "" {
		// use auth info when specified (otherwise caller has internal role)
		params.Payload.AccountID = M.ObjIDMutable(ai.AccountID)
		params.Payload.TenantAccountID = M.ObjIDMutable(ai.TenantAccountID)
	} else {
		aObj, err := c.DS.OpsAccount().Fetch(ctx, string(params.Payload.AccountID))
		if err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eMissingMsg("invalid accountId")
			}
			return ops.NewApplicationGroupCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		params.Payload.TenantAccountID = aObj.TenantAccountID
	}
	obj, err := c.DS.OpsApplicationGroup().Create(ctx, params.Payload)
	if err != nil {
		return ops.NewApplicationGroupCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.app.AuditLog.Post(ctx, ai, centrald.ApplicationGroupCreateAction, obj.Meta.ID, obj.Name, "", false, "Created")
	c.Log.Infof("ApplicationGroup %s created [%s]", obj.Name, obj.Meta.ID)
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	return ops.NewApplicationGroupCreateCreated().WithPayload(obj)
}

func (c *HandlerComp) applicationGroupDelete(params ops.ApplicationGroupDeleteParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewApplicationGroupDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var obj *M.ApplicationGroup
	if obj, err = c.DS.OpsApplicationGroup().Fetch(ctx, params.ID); err == nil {
		c.Lock()
		defer c.Unlock()
		if err = c.app.AuditLog.Ready(); err == nil {
			if err = ai.CapOK(centrald.VolumeSeriesOwnerCap, obj.AccountID); err != nil {
				c.app.AuditLog.Post(ctx, ai, centrald.ApplicationGroupDeleteAction, obj.Meta.ID, obj.Name, "", true, "Delete unauthorized")
				return ops.NewApplicationGroupDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			var n int
			if n, err = c.DS.OpsConsistencyGroup().Count(ctx, consistency_group.ConsistencyGroupListParams{ApplicationGroupID: &params.ID}, 1); err == nil {
				if n != 0 {
					err = &centrald.Error{M: "application group contains one or more consistency groups", C: centrald.ErrorExists.C}
				} else if err = c.DS.OpsApplicationGroup().Delete(ctx, params.ID); err == nil {
					c.app.AuditLog.Post(ctx, ai, centrald.ApplicationGroupDeleteAction, obj.Meta.ID, obj.Name, "", false, "Deleted")
					c.Log.Infof("ApplicationGroup %s deleted [%s]", obj.Name, obj.Meta.ID)
					c.setDefaultObjectScope(params.HTTPRequest, obj)
					return ops.NewApplicationGroupDeleteNoContent()
				}
			}
		}
	}
	return ops.NewApplicationGroupDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
}

func (c *HandlerComp) applicationGroupFetch(params ops.ApplicationGroupFetchParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewApplicationGroupFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.DS.OpsApplicationGroup().Fetch(ctx, params.ID)
	if err == nil {
		err = c.applicationGroupFetchFilter(ai, obj)
	}
	if err != nil {
		return ops.NewApplicationGroupFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewApplicationGroupFetchOK().WithPayload(obj)
}

func (c *HandlerComp) applicationGroupList(params ops.ApplicationGroupListParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewApplicationGroupListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var list []*M.ApplicationGroup
	// rather than querying for the entire list and performing RBAC filtering in the handler, constrain the query which should be more efficient
	params.AccountID, params.TenantAccountID, err = c.ops.constrainEitherOrQueryAccounts(ctx, ai, params.AccountID, centrald.VolumeSeriesOwnerCap, params.TenantAccountID, centrald.VolumeSeriesFetchCap)

	// if both AccountID and TenantAccountID are now nil and the caller is not internal, all accounts are filtered out, skip the query
	if err == nil && (params.AccountID != nil || params.TenantAccountID != nil || ai.Internal()) {
		list, err = c.DS.OpsApplicationGroup().List(ctx, params)
	}
	if err != nil {
		return ops.NewApplicationGroupListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewApplicationGroupListOK().WithPayload(list)
}

func (c *HandlerComp) applicationGroupUpdate(params ops.ApplicationGroupUpdateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewApplicationGroupUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var uP = [centrald.NumActionTypes][]string{
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    params.Set,
	}
	ua, err := c.makeStdUpdateArgs(c.applicationGroupMutableNameMap(), params.ID, params.Version, uP)
	if err != nil {
		return ops.NewApplicationGroupUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err = centrald.ErrorUpdateInvalidRequest
		return ops.NewApplicationGroupUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.IsModified("Name") && params.Payload.Name == "" {
		err := c.eUpdateInvalidMsg("non-empty name is required")
		return ops.NewApplicationGroupUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	oObj, err := c.DS.OpsApplicationGroup().Fetch(ctx, params.ID)
	if err != nil {
		return ops.NewApplicationGroupUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	} else if ua.Version == 0 {
		ua.Version = int32(oObj.Meta.Version)
	} else if int32(oObj.Meta.Version) != ua.Version {
		err = centrald.ErrorIDVerNotFound
		return ops.NewApplicationGroupUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err = c.app.AuditLog.Ready(); err != nil {
		return ops.NewApplicationGroupUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.IsModified("SystemTags") {
		if err = ai.InternalOK(); err != nil {
			return ops.NewApplicationGroupUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	} else {
		if err = ai.CapOK(centrald.VolumeSeriesOwnerCap, M.ObjIDMutable(oObj.AccountID)); err != nil {
			c.app.AuditLog.Post(ctx, ai, centrald.ApplicationGroupUpdateAction, oObj.Meta.ID, oObj.Name, "", true, "Update unauthorized")
			return ops.NewApplicationGroupUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}

	obj, err := c.DS.OpsApplicationGroup().Update(ctx, ua, params.Payload)
	if err != nil {
		return ops.NewApplicationGroupUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	return ops.NewApplicationGroupUpdateOK().WithPayload(obj)
}
