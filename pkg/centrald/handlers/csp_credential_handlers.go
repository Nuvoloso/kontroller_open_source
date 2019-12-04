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

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/csp_credential"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
)

// register handlers for CspCredential
func (c *HandlerComp) cspCredentialRegisterHandlers() {
	c.app.API.CspCredentialCspCredentialCreateHandler = ops.CspCredentialCreateHandlerFunc(c.cspCredentialCreate)
	c.app.API.CspCredentialCspCredentialDeleteHandler = ops.CspCredentialDeleteHandlerFunc(c.cspCredentialDelete)
	c.app.API.CspCredentialCspCredentialFetchHandler = ops.CspCredentialFetchHandlerFunc(c.cspCredentialFetch)
	c.app.API.CspCredentialCspCredentialListHandler = ops.CspCredentialListHandlerFunc(c.cspCredentialList)
	c.app.API.CspCredentialCspCredentialUpdateHandler = ops.CspCredentialUpdateHandlerFunc(c.cspCredentialUpdate)
	c.app.API.CspCredentialCspCredentialMetadataHandler = ops.CspCredentialMetadataHandlerFunc(c.cspCredentialMetadata)
}

var nmCspCredentialMutable JSONToAttrNameMap

func (c *HandlerComp) cspCredentialMutableNameMap() JSONToAttrNameMap {
	if nmCspCredentialMutable == nil {
		c.cacheMux.Lock()
		defer c.cacheMux.Unlock()
		if nmCspCredentialMutable == nil {
			nmCspCredentialMutable = c.makeJSONToAttrNameMap(models.CSPCredentialMutable{})
		}
	}
	return nmCspCredentialMutable
}

// cspCredentialFetchFilter verifies the caller has permission to fetch the object, returning an error if they do not.
func (c *HandlerComp) cspCredentialFetchFilter(ai *auth.Info, obj *models.CSPCredential) error {
	return ai.CapOK(centrald.CSPDomainManagementCap, obj.AccountID)
}

// Handlers

func (c *HandlerComp) cspCredentialCreate(params ops.CspCredentialCreateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewCspCredentialCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err := centrald.ErrorMissing
		return ops.NewCspCredentialCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload.Name == "" || params.Payload.CspDomainType == "" || params.Payload.CredentialAttributes == nil {
		err := c.eMissingMsg("name, cspDomainType and credentialAttributes required")
		return ops.NewCspCredentialCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ai.AccountID != "" {
		// always use auth AccountID as the owner when specified
		params.Payload.AccountID = models.ObjIDMutable(ai.AccountID)
	}
	c.RLock()
	defer c.RUnlock()
	if err = c.app.AuditLog.Ready(); err != nil {
		return ops.NewCspCredentialCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if _, err = c.DS.OpsAccount().Fetch(ctx, string(params.Payload.AccountID)); err != nil {
		if err == centrald.ErrorNotFound {
			err = c.eMissingMsg("invalid accountId")
		}
		return ops.NewCspCredentialCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err = ai.CapOK(centrald.CSPDomainManagementCap, params.Payload.AccountID); err != nil {
		c.app.AuditLog.Post(ctx, ai, centrald.CspCredentialCreateAction, "", params.Payload.Name, "", true, "Create unauthorized")
		return ops.NewCspCredentialCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err := c.app.AppCSP.ValidateCspCredential(params.Payload.CspDomainType, nil, params.Payload.CredentialAttributes); err != nil {
		err := c.eMissingMsg("%s", err.Error())
		return ops.NewCspCredentialCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.DS.OpsCspCredential().Create(ctx, params.Payload)
	if err != nil {
		return ops.NewCspCredentialCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.app.AuditLog.Post(ctx, ai, centrald.CspCredentialCreateAction, obj.Meta.ID, obj.Name, "", false, "Created")
	c.Log.Debugf("CspCredential %s created [%s]", obj.Name, obj.Meta.ID)
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	return ops.NewCspCredentialCreateCreated().WithPayload(obj)
}

func (c *HandlerComp) cspCredentialDelete(params ops.CspCredentialDeleteParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewCspCredentialDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var obj *models.CSPCredential
	if obj, err = c.DS.OpsCspCredential().Fetch(ctx, params.ID); err == nil {
		if err = c.app.AuditLog.Ready(); err == nil {
			c.Lock()
			defer c.Unlock()
			if err = ai.CapOK(centrald.CSPDomainManagementCap, obj.AccountID); err == nil {
				// cannot be deleted if referenced from a CSPDomain object
				lParams := csp_domain.CspDomainListParams{
					AccountID:       swag.String(string(obj.AccountID)),
					CspDomainType:   swag.String(string(obj.CspDomainType)),
					CspCredentialID: swag.String(params.ID),
				}
				var n int
				if n, err = c.DS.OpsCspDomain().Count(ctx, lParams, 1); err == nil {
					if n != 0 {
						err = &centrald.Error{M: "CSP domains still refer to the credential", C: centrald.ErrorExists.C}
					} else if err = c.DS.OpsCspCredential().Delete(ctx, params.ID); err == nil {
						c.app.AuditLog.Post(ctx, ai, centrald.CspCredentialDeleteAction, obj.Meta.ID, obj.Name, "", false, "Deleted")
						c.Log.Infof("CspDomain %s deleted [%s]", obj.Name, obj.Meta.ID)
						c.setDefaultObjectScope(params.HTTPRequest, obj)
						return ops.NewCspCredentialDeleteNoContent()
					}
				}
			} else {
				c.app.AuditLog.Post(ctx, ai, centrald.CspCredentialDeleteAction, obj.Meta.ID, obj.Name, "", true, "Delete unauthorized")
			}
		}
	}
	return ops.NewCspCredentialDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
}

func (c *HandlerComp) cspCredentialFetch(params ops.CspCredentialFetchParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewCspCredentialFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.intCspCredentialFetch(ctx, ai, params.ID)
	if err != nil {
		return ops.NewCspCredentialFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewCspCredentialFetchOK().WithPayload(obj)
}

// intCspCredentialFetch is the internal shareable part of cspCredentialFetch; protect appropriately!
func (c *HandlerComp) intCspCredentialFetch(ctx context.Context, ai *auth.Info, id string) (*models.CSPCredential, error) {
	obj, err := c.DS.OpsCspCredential().Fetch(ctx, id)
	if err == nil {
		err = c.cspCredentialFetchFilter(ai, obj)
	}
	return obj, err
}

func (c *HandlerComp) cspCredentialList(params ops.CspCredentialListParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewCspCredentialListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if !ai.Internal() {
		// apply access control to the query parameters, skip the query if the caller lacks the capability or if the account is a mismatch
		if swag.StringValue(params.AccountID) == "" {
			params.AccountID = swag.String(ai.AccountID)
		}
		if ai.CapOK(centrald.CSPDomainManagementCap) != nil || swag.StringValue(params.AccountID) != ai.AccountID {
			return ops.NewCspCredentialListOK()
		}
	}
	list, err := c.DS.OpsCspCredential().List(ctx, params)
	if err != nil {
		return ops.NewCspCredentialListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewCspCredentialListOK().WithPayload(list)
}

func (c *HandlerComp) cspCredentialUpdate(params ops.CspCredentialUpdateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewCspCredentialUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var uP = [centrald.NumActionTypes][]string{
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    params.Set,
	}
	nMap := c.cspCredentialMutableNameMap()
	ua, err := c.makeStdUpdateArgs(nMap, params.ID, params.Version, uP)
	if err != nil {
		return ops.NewCspCredentialUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err = c.eUpdateInvalidMsg("missing payload")
		return ops.NewCspCredentialUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.IsModified("Name") && params.Payload.Name == "" {
		err = c.eUpdateInvalidMsg("non-empty name is required")
		return ops.NewCspCredentialUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.DS.OpsCspCredential().Fetch(ctx, params.ID)
	if err != nil {
		return ops.NewCspCredentialUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.Version == 0 {
		ua.Version = int32(obj.Meta.Version)
	} else if int32(obj.Meta.Version) != ua.Version {
		err = centrald.ErrorIDVerNotFound
		return ops.NewCspCredentialUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err = c.app.AuditLog.Ready(); err != nil {
		return ops.NewCspCredentialUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err = ai.CapOK(centrald.CSPDomainManagementCap, obj.AccountID); err != nil {
		c.app.AuditLog.Post(ctx, ai, centrald.CspCredentialUpdateAction, models.ObjID(params.ID), obj.Name, "", true, "Update unauthorized")
		return ops.NewCspCredentialUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	credsUpdated := false
	if ua.IsModified("CredentialAttributes") {
		attrs, err := c.mergeAttributes(ua.FindUpdateAttr("CredentialAttributes"), nMap.jName("CredentialAttributes"),
			obj.CredentialAttributes, params.Payload.CredentialAttributes)
		if err != nil {
			return ops.NewCspCredentialUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		obj.Meta = nil // force no use of cache, obj.Meta.Version has not changed yet (and will not change if a failure occurs)
		oldAttrs := make(map[string]models.ValueType)
		for key, value := range obj.CredentialAttributes {
			oldAttrs[key] = value
		}
		obj.CredentialAttributes = attrs
		if err := c.app.AppCSP.ValidateCspCredential(obj.CspDomainType, oldAttrs, obj.CredentialAttributes); err != nil {
			err = c.eUpdateInvalidMsg("%s", err.Error())
			return ops.NewCspCredentialUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		credsUpdated = true
	}
	if obj, err = c.DS.OpsCspCredential().Update(ctx, ua, params.Payload); err != nil {
		return ops.NewCspCredentialUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if credsUpdated {
		lParams := csp_domain.CspDomainListParams{
			AccountID:       swag.String(string(obj.AccountID)),
			CspDomainType:   swag.String(string(obj.CspDomainType)),
			CspCredentialID: swag.String(params.ID),
		}
		if res, err := c.DS.OpsCspDomain().List(ctx, lParams); err == nil && len(res) == 1 {
			c.app.AuditLog.Event(ctx, ai, centrald.CspDomainUpdateAction, res[0].Meta.ID, res[0].Name, models.ObjIDMutable(obj.Meta.ID), false, "Updated CspCredential attributes")
		}
	}
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	return ops.NewCspCredentialUpdateOK().WithPayload(obj)
}

func (c *HandlerComp) cspCredentialMetadata(params ops.CspCredentialMetadataParams) middleware.Responder {
	dt := models.CspDomainTypeMutable(params.CspDomainType)
	if !c.app.ValidateCspDomainType(dt) {
		err := c.eInvalidData("invalid cspDomainType '%s'", dt)
		return ops.NewCspCredentialMetadataDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	md := &models.CSPCredentialMetadata{}
	md.AttributeMetadata = c.app.CredentialAttrDescForType(dt)
	ret := ops.NewCspCredentialMetadataOK()
	ret.Payload = md
	return ret
}
