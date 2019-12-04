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
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/protection_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/snapshot"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
)

// Register handlers for ProtectionDomain
func (c *HandlerComp) protectionDomainRegisterHandlers() {
	c.app.API.ProtectionDomainProtectionDomainCreateHandler = ops.ProtectionDomainCreateHandlerFunc(c.protectionDomainCreate)
	c.app.API.ProtectionDomainProtectionDomainDeleteHandler = ops.ProtectionDomainDeleteHandlerFunc(c.protectionDomainDelete)
	c.app.API.ProtectionDomainProtectionDomainFetchHandler = ops.ProtectionDomainFetchHandlerFunc(c.protectionDomainFetch)
	c.app.API.ProtectionDomainProtectionDomainListHandler = ops.ProtectionDomainListHandlerFunc(c.protectionDomainList)
	c.app.API.ProtectionDomainProtectionDomainMetadataHandler = ops.ProtectionDomainMetadataHandlerFunc(c.protectionDomainMetadata)
	c.app.API.ProtectionDomainProtectionDomainUpdateHandler = ops.ProtectionDomainUpdateHandlerFunc(c.protectionDomainUpdate)
}

var nmProtectionDomainMutable JSONToAttrNameMap

func (c *HandlerComp) protectionDomainMutableNameMap() JSONToAttrNameMap {
	if nmProtectionDomainMutable == nil {
		c.cacheMux.Lock()
		defer c.cacheMux.Unlock()
		if nmProtectionDomainMutable == nil {
			nmProtectionDomainMutable = c.makeJSONToAttrNameMap(M.ProtectionDomainMutable{})
		}
	}
	return nmProtectionDomainMutable
}

// protectionDomainFetchFilter verifies the caller has permission to fetch the object, returning an error if they do not
func (c *HandlerComp) protectionDomainFetchFilter(ai *auth.Info, obj *M.ProtectionDomain) error {
	return ai.CapOK(centrald.ProtectionDomainManagementCap, obj.AccountID)
}

// Handlers

func (c *HandlerComp) protectionDomainCreate(params ops.ProtectionDomainCreateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewProtectionDomainCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err = ai.CapOK(centrald.ProtectionDomainManagementCap); err != nil {
		return ops.NewProtectionDomainCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil || params.Payload.EncryptionAlgorithm == "" || params.Payload.EncryptionPassphrase == nil {
		err = centrald.ErrorMissing
		return ops.NewProtectionDomainCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if len(params.Payload.SystemTags) > 0 {
		err = c.eUnauthorizedOrForbidden("systemTags")
		return ops.NewProtectionDomainCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err = c.app.ValidateProtectionDomainEncryptionProperties(params.Payload.EncryptionAlgorithm, params.Payload.EncryptionPassphrase); err != nil {
		err = c.eMissingMsg("%s", err.Error())
		return ops.NewProtectionDomainCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.RLock()
	defer c.RUnlock()
	if err = c.app.AuditLog.Ready(); err != nil {
		return ops.NewProtectionDomainCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	_, err = c.DS.OpsAccount().Fetch(ctx, string(params.Payload.AccountID))
	if err != nil {
		if err == centrald.ErrorNotFound {
			err = c.eMissingMsg("invalid accountId")
		}
		return ops.NewProtectionDomainCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if string(params.Payload.AccountID) != ai.AccountID { // must be same owner
		err = centrald.ErrorUnauthorizedOrForbidden
		return ops.NewProtectionDomainCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.DS.OpsProtectionDomain().Create(ctx, params.Payload)
	if err != nil {
		return ops.NewProtectionDomainCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	msg := fmt.Sprintf("Encryption: %s", obj.EncryptionAlgorithm)
	c.app.AuditLog.Post(ctx, ai, centrald.ProtectionDomainCreateAction, obj.Meta.ID, obj.Name, M.ObjIDMutable(ai.AccountID), false, msg)
	c.Log.Infof("ProtectionDomain %s created in account [%s]", obj.Meta.ID, ai.AccountName)
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	return ops.NewProtectionDomainCreateCreated().WithPayload(obj)
}

func (c *HandlerComp) protectionDomainDelete(params ops.ProtectionDomainDeleteParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewProtectionDomainDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var obj *M.ProtectionDomain
	if err = c.app.AuditLog.Ready(); err == nil {
		// fetch also verifies the permissions necessary for deletion
		if obj, err = c.intProtectionDomainFetch(ctx, ai, params.ID); err == nil {
			c.Lock()
			defer c.Unlock()
			var aObj *M.Account
			aObj, err = c.ops.intAccountFetch(ctx, ai, string(obj.AccountID))
			if err == nil {
				if c.ops.accountReferencesProtectionDomain(aObj, string(obj.Meta.ID)) {
					err = c.eExists("referenced from Account object")
				} else {
					lpVSR := volume_series_request.NewVolumeSeriesRequestListParams()
					lpVSR.ProtectionDomainID = swag.String(params.ID)
					lpVSR.IsTerminated = swag.Bool(false)
					var n int
					if n, err = c.DS.OpsVolumeSeriesRequest().Count(ctx, lpVSR, 1); err == nil {
						if n != 0 {
							err = c.eExists("referenced from active VolumeSeriesRequest objects")
						} else {
							lpSnap := snapshot.NewSnapshotListParams()
							lpSnap.ProtectionDomainID = swag.String(params.ID)
							lpSnap.AccountID = swag.String(string(obj.AccountID))
							if n, err = c.DS.OpsSnapshot().Count(ctx, lpSnap, 1); err == nil {
								if n != 0 {
									err = c.eExists("referenced from Snapshot objects")
								} else {
									if err = c.DS.OpsProtectionDomain().Delete(ctx, params.ID); err == nil {
										c.app.AuditLog.Post(ctx, ai, centrald.ProtectionDomainDeleteAction, obj.Meta.ID, obj.Name, obj.AccountID, false, "Deleted")
										c.setDefaultObjectScope(params.HTTPRequest, obj)
										return ops.NewProtectionDomainDeleteNoContent()
									}
								}
							}
						}
					}
				}
			}
		}
	}
	if err == centrald.ErrorUnauthorizedOrForbidden { // not returned by AuditLog.Ready()
		var name M.ObjName
		var refID M.ObjIDMutable
		if obj != nil { // fetch can fail simply based on capabilities
			name = obj.Name
			refID = obj.AccountID
		}
		c.app.AuditLog.Post(ctx, ai, centrald.ProtectionDomainDeleteAction, M.ObjID(params.ID), name, refID, true, "Delete unauthorized")
	}
	return ops.NewProtectionDomainDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
}

func (c *HandlerComp) protectionDomainFetch(params ops.ProtectionDomainFetchParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewProtectionDomainFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.intProtectionDomainFetch(ctx, ai, params.ID)
	if err != nil {
		return ops.NewProtectionDomainFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewProtectionDomainFetchOK().WithPayload(obj)
}

func (c *HandlerComp) intProtectionDomainFetch(ctx context.Context, ai *auth.Info, id string) (*M.ProtectionDomain, error) {
	var obj *M.ProtectionDomain
	err := ai.CapOK(centrald.ProtectionDomainManagementCap) // preempt the fetch based on cap only
	if err == nil {
		obj, err = c.DS.OpsProtectionDomain().Fetch(ctx, id)
		if err == nil {
			err = c.protectionDomainFetchFilter(ai, obj)
		}
	}
	return obj, err
}

func (c *HandlerComp) protectionDomainList(params ops.ProtectionDomainListParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err == nil {
		err = ai.CapOK(centrald.ProtectionDomainManagementCap) // preempt the list based on cap only
	}
	if err != nil {
		return ops.NewProtectionDomainListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if !ai.Internal() && params.AccountID == nil {
		params.AccountID = swag.String(ai.AccountID) // reduce the query scope
	}
	list, err := c.DS.OpsProtectionDomain().List(ctx, params)
	if err != nil {
		return ops.NewProtectionDomainListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	ret := make([]*M.ProtectionDomain, 0, len(list))
	for _, obj := range list {
		if c.protectionDomainFetchFilter(ai, obj) == nil {
			ret = append(ret, obj)
		}
	}
	return ops.NewProtectionDomainListOK().WithPayload(ret)
}

func (c *HandlerComp) protectionDomainMetadata(params ops.ProtectionDomainMetadataParams) middleware.Responder {
	return ops.NewProtectionDomainMetadataOK().WithPayload(c.app.GetProtectionDomainMetadata())
}

func (c *HandlerComp) protectionDomainUpdate(params ops.ProtectionDomainUpdateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewProtectionDomainUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var uP = [centrald.NumActionTypes][]string{
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    params.Set,
	}
	nMap := c.protectionDomainMutableNameMap()
	ua, err := c.makeStdUpdateArgs(nMap, params.ID, params.Version, uP)
	if err != nil {
		return ops.NewProtectionDomainUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if !ai.Internal() && ua.IsModified("SystemTags") {
		err = c.eUnauthorizedOrForbidden("systemTags")
		return ops.NewProtectionDomainUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err = centrald.ErrorUpdateInvalidRequest
		return ops.NewProtectionDomainUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.RLock()
	defer c.RUnlock()
	isRenamed := ua.IsModified("Name")
	if isRenamed {
		if err = c.app.AuditLog.Ready(); err != nil {
			return ops.NewProtectionDomainUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	oObj, err := c.intProtectionDomainFetch(ctx, ai, params.ID)
	if err != nil {
		return ops.NewProtectionDomainUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.Version == 0 {
		ua.Version = int32(oObj.Meta.Version)
	} else if int32(oObj.Meta.Version) != ua.Version {
		err = centrald.ErrorIDVerNotFound
		return ops.NewProtectionDomainUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.DS.OpsProtectionDomain().Update(ctx, ua, params.Payload)
	if err != nil {
		return ops.NewProtectionDomainUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if isRenamed {
		msg := fmt.Sprintf("Renamed from '%s'", oObj.Name)
		c.app.AuditLog.Post(ctx, ai, centrald.ProtectionDomainUpdateAction, obj.Meta.ID, obj.Name, M.ObjIDMutable(obj.AccountID), false, msg)
	}
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	return ops.NewProtectionDomainUpdateOK().WithPayload(obj)
}
