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
	"strings"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/cluster"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	clPkg "github.com/Nuvoloso/kontroller/pkg/cluster"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
)

// register handlers for CspDomain
func (c *HandlerComp) cspDomainRegisterHandlers() {
	c.app.API.CspDomainCspDomainCreateHandler = ops.CspDomainCreateHandlerFunc(c.cspDomainCreate)
	c.app.API.CspDomainCspDomainDeleteHandler = ops.CspDomainDeleteHandlerFunc(c.cspDomainDelete)
	c.app.API.CspDomainCspDomainDeploymentFetchHandler = ops.CspDomainDeploymentFetchHandlerFunc(c.cspDomainDeploymentFetch)
	c.app.API.CspDomainCspDomainFetchHandler = ops.CspDomainFetchHandlerFunc(c.cspDomainFetch)
	c.app.API.CspDomainCspDomainListHandler = ops.CspDomainListHandlerFunc(c.cspDomainList)
	c.app.API.CspDomainCspDomainMetadataHandler = ops.CspDomainMetadataHandlerFunc(c.cspDomainMetadata)
	c.app.API.CspDomainCspDomainServicePlanCostHandler = ops.CspDomainServicePlanCostHandlerFunc(c.cspDomainServicePlanCost)
	c.app.API.CspDomainCspDomainUpdateHandler = ops.CspDomainUpdateHandlerFunc(c.cspDomainUpdate)
}

var nmCspDomainMutable JSONToAttrNameMap

func (c *HandlerComp) cspDomainMutableNameMap() JSONToAttrNameMap {
	if nmCspDomainMutable == nil {
		c.cacheMux.Lock()
		defer c.cacheMux.Unlock()
		if nmCspDomainMutable == nil {
			nmCspDomainMutable = c.makeJSONToAttrNameMap(models.CSPDomainMutable{})
		}
	}
	return nmCspDomainMutable
}

// cspDomainFetchFilter verifies the caller has permission to fetch the object, returning an error if they do not.
// On success, SECRET CspDomainAttributes have their value obscured except for the object owner
func (c *HandlerComp) cspDomainFetchFilter(ai *auth.Info, obj *models.CSPDomain) error {
	err := ai.CapOK(centrald.CSPDomainManagementCap, obj.AccountID)
	if err != nil && len(obj.AuthorizedAccounts) > 0 {
		if err = ai.CapOK(centrald.CSPDomainUsageCap, obj.AuthorizedAccounts...); err == nil {
			for k, v := range obj.CspDomainAttributes {
				if v.Kind == com.ValueTypeSecret { // note: CSPCredential no longer in object
					obj.CspDomainAttributes[k] = models.ValueType{Kind: com.ValueTypeSecret, Value: "********"}
				}
			}
		}
	}
	return err
}

// cspDomainApplyInheritedProperties silently inserts inherited properties, ClusterUsagePolicy and CspCredential
func (c *HandlerComp) cspDomainApplyInheritedProperties(ctx context.Context, ai *auth.Info, obj *models.CSPDomain, sysObj *models.System, cspCredObj *models.CSPCredential) error {
	inheritCUP := false
	if obj.ClusterUsagePolicy == nil || obj.ClusterUsagePolicy.Inherited {
		inheritCUP = true
	}
	var err error
	if inheritCUP {
		if sysObj == nil {
			sysObj, err = c.DS.OpsSystem().Fetch()
			if err != nil {
				return err
			}
		}
		obj.ClusterUsagePolicy = &models.ClusterUsagePolicy{}
		*obj.ClusterUsagePolicy = *sysObj.ClusterUsagePolicy
		obj.ClusterUsagePolicy.Inherited = true
	}
	if obj.CspCredentialID != "" && ai.Internal() { // merge credential if internal but no error otherwise
		if cspCredObj == nil {
			if cspCredObj, err = c.ops.intCspCredentialFetch(ctx, ai, string(obj.CspCredentialID)); err != nil {
				return err
			}
		}
		if obj.CspDomainAttributes == nil {
			obj.CspDomainAttributes = map[string]models.ValueType{}
		}
		for ca, vt := range cspCredObj.CredentialAttributes {
			obj.CspDomainAttributes[ca] = vt
		}
	}
	return nil
}

// Handlers

func (c *HandlerComp) cspDomainCreate(params ops.CspDomainCreateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewCspDomainCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err := centrald.ErrorMissing
		return ops.NewCspDomainCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload.Name == "" || params.Payload.CspDomainType == "" || params.Payload.CspDomainAttributes == nil || params.Payload.CspCredentialID == "" {
		err := c.eMissingMsg("name, cspDomainType, cspCredentialID and cspDomainAttributes required")
		return ops.NewCspDomainCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if len(params.Payload.StorageCosts) > 0 {
		if err = c.validateStorageCosts(params.Payload.StorageCosts, string(params.Payload.CspDomainType)); err != nil {
			err = c.eMissingMsg("%s", err.Error())
			return ops.NewCspDomainCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if ai.AccountID != "" {
		// always use auth AccountID as the owner when specified
		params.Payload.AccountID = models.ObjIDMutable(ai.AccountID)
	}
	c.RLock()
	defer c.RUnlock()
	if err = c.app.AuditLog.Ready(); err != nil {
		return ops.NewCspDomainCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if _, err = c.DS.OpsAccount().Fetch(ctx, string(params.Payload.AccountID)); err != nil {
		if err == centrald.ErrorNotFound {
			err = c.eMissingMsg("invalid accountId")
		}
		return ops.NewCspDomainCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err = ai.CapOK(centrald.CSPDomainManagementCap, params.Payload.AccountID); err != nil {
		c.app.AuditLog.Post(ctx, ai, centrald.CspDomainCreateAction, "", params.Payload.Name, "", true, "Create unauthorized")
		return ops.NewCspDomainCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	for _, aid := range params.Payload.AuthorizedAccounts {
		aObj, err := c.DS.OpsAccount().Fetch(ctx, string(aid))
		if err == nil {
			// tenant admin can authorize their own account or any subordinate account
			if err = ai.CapOK(centrald.CSPDomainManagementCap, aid, aObj.TenantAccountID); err != nil {
				c.app.AuditLog.Post(ctx, ai, centrald.CspDomainCreateAction, "", params.Payload.Name, "", true, fmt.Sprintf("Create attempted with incorrect authorizedAccount %s[%s]", aObj.Name, aObj.Meta.ID))
			}
		}
		if err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eMissingMsg("invalid authorizedAccounts: %s", aid)
			}
			return ops.NewCspDomainCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	cspCredObj, err := c.ops.intCspCredentialFetch(ctx, ai, string(params.Payload.CspCredentialID))
	if err != nil {
		return ops.NewCspDomainCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err := c.app.AppCSP.ValidateAndInitCspDomain(ctx, params.Payload, cspCredObj.CredentialAttributes); err != nil {
		err := c.eMissingMsg("%s", err.Error())
		return ops.NewCspDomainCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	sysObj, err := c.DS.OpsSystem().Fetch()
	if err != nil {
		return ops.NewCspDomainCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	params.Payload.ManagementHost = strings.TrimSpace(params.Payload.ManagementHost)
	obj, err := c.DS.OpsCspDomain().Create(ctx, params.Payload)
	if err != nil {
		return ops.NewCspDomainCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.cspDomainApplyInheritedProperties(ctx, ai, obj, sysObj, cspCredObj) // no error possible
	msg := "Created"
	if obj.ManagementHost != "" {
		msg = fmt.Sprintf("%s with management host[%s]", msg, obj.ManagementHost)
	}
	c.app.AuditLog.Post(ctx, ai, centrald.CspDomainCreateAction, obj.Meta.ID, obj.Name, obj.CspCredentialID, false, msg)
	for st, sc := range obj.StorageCosts {
		c.app.AuditLog.Post(ctx, ai, centrald.CspDomainUpdateAction, obj.Meta.ID, obj.Name, "", false, fmt.Sprintf(stgCostSetFmt, st, sc.CostPerGiB))
	}
	c.Log.Infof("CspDomain %s created [%s]", obj.Name, obj.Meta.ID)
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	return ops.NewCspDomainCreateCreated().WithPayload(obj)
}

func (c *HandlerComp) cspDomainDelete(params ops.CspDomainDeleteParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewCspDomainDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var obj *models.CSPDomain
	if obj, err = c.DS.OpsCspDomain().Fetch(ctx, params.ID); err == nil {
		if err = c.app.AuditLog.Ready(); err == nil {
			c.Lock()
			defer c.Unlock()
			if err = ai.CapOK(centrald.CSPDomainManagementCap, obj.AccountID); err == nil {
				// verify domain is not referenced in any account SnapshotCatalogPolicy
				var n int
				if n, err = c.DS.OpsAccount().Count(ctx, account.AccountListParams{CspDomainID: &params.ID}, 1); err == nil {
					if n != 0 {
						err = &centrald.Error{M: fmt.Sprintf("CSP domain is referenced in account SnapshotCatalogPolicy"), C: centrald.ErrorExists.C}
					}
				}
				if err != nil {
					return ops.NewCspDomainDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
				}
				if n, err = c.DS.OpsCluster().Count(ctx, cluster.ClusterListParams{CspDomainID: &params.ID}, 1); err == nil {
					// cluster check covers all objects that require a cluster: node, pool, SPA, storage, SR
					if n != 0 {
						err = &centrald.Error{M: "clusters still exist in the CSP domain", C: centrald.ErrorExists.C}
					} else if err = c.DS.OpsCspDomain().Delete(ctx, params.ID); err == nil {
						c.app.AuditLog.Post(ctx, ai, centrald.CspDomainDeleteAction, obj.Meta.ID, obj.Name, obj.CspCredentialID, false, "Deleted")
						c.Log.Infof("CspDomain %s deleted [%s]", obj.Name, obj.Meta.ID)
						c.setDefaultObjectScope(params.HTTPRequest, obj)
						return ops.NewCspDomainDeleteNoContent()
					}
				}
			} else {
				c.app.AuditLog.Post(ctx, ai, centrald.CspDomainDeleteAction, obj.Meta.ID, obj.Name, "", true, "Delete unauthorized")
			}
		}
	}
	return ops.NewCspDomainDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
}

func (c *HandlerComp) cspDomainFetch(params ops.CspDomainFetchParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewCspDomainFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.intCspDomainFetch(ctx, ai, params.ID)
	if err != nil {
		return ops.NewCspDomainFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewCspDomainFetchOK().WithPayload(obj)
}

// intCspDomainFetch is the internal shareable part of cspDomainFetch; protect appropriately!
func (c *HandlerComp) intCspDomainFetch(ctx context.Context, ai *auth.Info, id string) (*models.CSPDomain, error) {
	obj, err := c.DS.OpsCspDomain().Fetch(ctx, id)
	if err == nil {
		if err = c.cspDomainFetchFilter(ai, obj); err == nil {
			err = c.cspDomainApplyInheritedProperties(ctx, ai, obj, nil, nil)
		}
	}
	return obj, err
}

func (c *HandlerComp) cspDomainList(params ops.CspDomainListParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewCspDomainListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var skip bool
	params.AuthorizedAccountID, params.AccountID, skip = c.ops.constrainBothQueryAccounts(ai, params.AuthorizedAccountID, centrald.CSPDomainUsageCap, params.AccountID, centrald.CSPDomainManagementCap)
	if skip {
		return ops.NewCspDomainListOK()
	}

	sysObj, err := c.DS.OpsSystem().Fetch()
	if err != nil {
		return ops.NewCspDomainListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	list, err := c.DS.OpsCspDomain().List(ctx, params)
	if err != nil {
		return ops.NewCspDomainListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	credCache := map[string]*models.CSPCredential{}
	var credObj *models.CSPCredential
	var ok bool
	ret := make([]*models.CSPDomain, 0, len(list))
	for _, obj := range list {
		if c.cspDomainFetchFilter(ai, obj) == nil { // Filter call still needed to obscure secrets
			if ai.Internal() && obj.CspCredentialID != "" { // credential merging only for internal user
				if credObj, ok = credCache[string(obj.CspCredentialID)]; !ok {
					cred, err := c.ops.intCspCredentialFetch(ctx, ai, string(obj.CspCredentialID))
					if err != nil {
						c.Log.Errorf("CSPDomain[%s]: error looking up CSPCredential[%s]: %s", obj.Meta.ID, obj.CspCredentialID, err.Error())
						continue // skip the domain
					}
					credObj = cred
					credCache[string(obj.CspCredentialID)] = cred
				}
			}
			c.cspDomainApplyInheritedProperties(ctx, ai, obj, sysObj, credObj) // no error possible
			ret = append(ret, obj)
		}
	}
	return ops.NewCspDomainListOK().WithPayload(ret)
}

func (c *HandlerComp) cspDomainUpdate(params ops.CspDomainUpdateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewCspDomainUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var uP = [centrald.NumActionTypes][]string{
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    params.Set,
	}
	nMap := c.cspDomainMutableNameMap()
	ua, err := c.makeStdUpdateArgs(nMap, params.ID, params.Version, uP)
	if err != nil {
		return ops.NewCspDomainUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err = c.eUpdateInvalidMsg("missing payload")
		return ops.NewCspDomainUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.IsModified("Name") && params.Payload.Name == "" {
		err = c.eUpdateInvalidMsg("non-empty name is required")
		return ops.NewCspDomainUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.IsModified("ClusterUsagePolicy") {
		if err := c.validateClusterUsagePolicy(params.Payload.ClusterUsagePolicy, com.AccountSecretScopeCspDomain); err != nil {
			return ops.NewCspDomainUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	credUpdated := false
	var cspCredObj *models.CSPCredential
	if ua.IsModified("CspCredentialID") {
		if cspCredObj, err = c.ops.intCspCredentialFetch(ctx, ai, string(params.Payload.CspCredentialID)); err != nil {
			err = c.eUpdateInvalidMsg("invalid cspCredentialId")
			return ops.NewCspDomainUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		credUpdated = true
	}
	obj, err := c.DS.OpsCspDomain().Fetch(ctx, params.ID)
	if err != nil {
		return ops.NewCspDomainUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	origCspCredentialID := obj.CspCredentialID
	if credUpdated && origCspCredentialID == params.Payload.CspCredentialID {
		cspCredObj = nil
	} else if err = ai.InternalOK(); err == nil && obj.CspCredentialID != "" {
		if cspCredObj == nil {
			if cspCredObj, err = c.ops.intCspCredentialFetch(ctx, ai, string(obj.CspCredentialID)); err != nil {
				return ops.NewCspDomainUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
		}
	}
	if ua.Version == 0 {
		ua.Version = int32(obj.Meta.Version)
	} else if int32(obj.Meta.Version) != ua.Version {
		err = centrald.ErrorIDVerNotFound
		return ops.NewCspDomainUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err = c.app.AuditLog.Ready(); err != nil {
		return ops.NewCspDomainUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err = ai.CapOK(centrald.CSPDomainManagementCap, obj.AccountID); err != nil {
		c.app.AuditLog.Post(ctx, ai, centrald.CspDomainUpdateAction, models.ObjID(params.ID), obj.Name, "", true, "Update unauthorized")
		return ops.NewCspDomainUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	costDetail := []string{}
	if ua.IsModified("StorageCosts") {
		costs, msgs, err := c.mergeStorageCosts(ua.FindUpdateAttr("StorageCosts"), nMap.jName("StorageCosts"), obj.StorageCosts, params.Payload.StorageCosts)
		if err == nil {
			err = c.validateStorageCosts(costs, string(obj.CspDomainType))
		}
		if err != nil {
			err = c.eUpdateInvalidMsg("%s", err.Error())
			return ops.NewCspDomainUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		obj.StorageCosts = costs
		costDetail = msgs
	}
	authDetail := ""
	if a := ua.FindUpdateAttr("AuthorizedAccounts"); a != nil && a.IsModified() {
		c.RLock()
		defer c.RUnlock()
		if authDetail, err = c.authAccountValidator.validateAuthorizedAccountsUpdate(ctx, ai, centrald.CspDomainUpdateAction, params.ID, obj.Name, a, obj.AuthorizedAccounts, params.Payload.AuthorizedAccounts); err != nil {
			return ops.NewCspDomainUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	mgmtHostDetail := ""
	params.Payload.ManagementHost = strings.TrimSpace(params.Payload.ManagementHost)
	if ua.IsModified("ManagementHost") && obj.ManagementHost != params.Payload.ManagementHost {
		mgmtHostDetail = fmt.Sprintf("management host[%s]", params.Payload.ManagementHost)
	}
	sysObj, err := c.DS.OpsSystem().Fetch()
	if err != nil {
		return ops.NewCspDomainUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if obj, err = c.DS.OpsCspDomain().Update(ctx, ua, params.Payload); err != nil {
		return ops.NewCspDomainUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.cspDomainApplyInheritedProperties(ctx, ai, obj, sysObj, cspCredObj)
	if len(authDetail) > 0 {
		c.app.AuditLog.Post(ctx, ai, centrald.CspDomainUpdateAction, models.ObjID(params.ID), obj.Name, "", false, fmt.Sprintf("Updated authorizedAccounts %s", authDetail))
	}
	if len(mgmtHostDetail) > 0 {
		c.app.AuditLog.Event(ctx, ai, centrald.CspDomainUpdateAction, models.ObjID(params.ID), obj.Name, "", false, fmt.Sprintf("Updated %s", mgmtHostDetail))
	}
	if credUpdated && origCspCredentialID != params.Payload.CspCredentialID {
		c.app.AuditLog.Post(ctx, ai, centrald.CspDomainUpdateAction, models.ObjID(params.ID), obj.Name, obj.CspCredentialID, false, fmt.Sprintf("Updated CspCredentialID %s", params.Payload.CspCredentialID))
	}
	for _, msg := range costDetail {
		c.app.AuditLog.Post(ctx, ai, centrald.CspDomainUpdateAction, models.ObjID(params.ID), obj.Name, "", false, msg)
	}
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	return ops.NewCspDomainUpdateOK().WithPayload(obj)
}

func (c *HandlerComp) cspDomainDeploymentFetch(params ops.CspDomainDeploymentFetchParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewCspDomainDeploymentFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.DS.OpsCspDomain().Fetch(ctx, params.ID)
	if err == nil {
		// because clusters are owned by the domain account, only consider the domain owner, not the authorized accounts
		err = ai.CapOK(centrald.CSPDomainManagementCap, obj.AccountID)
	}
	if err != nil {
		return ops.NewCspDomainDeploymentFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if obj.ManagementHost == "" {
		err = c.eInvalidData("managementHost is not set")
		return ops.NewCspDomainDeploymentFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if swag.StringValue(params.Name) != "" && swag.BoolValue(params.Force) == false {
		count, err := c.DS.OpsCluster().Count(ctx, cluster.ClusterListParams{CspDomainID: &params.ID, Name: params.Name}, 1)
		if count > 0 {
			err = centrald.ErrorExists
		}
		if err != nil {
			return ops.NewCspDomainDeploymentFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	sysObj, err := c.DS.OpsSystem().Fetch()
	if err != nil {
		return ops.NewCspDomainDeploymentFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	args := &clPkg.MCDeploymentArgs{
		SystemID:       string(sysObj.Meta.ID),
		CSPDomainID:    string(obj.Meta.ID),
		CSPDomainType:  string(obj.CspDomainType),
		ClusterType:    swag.StringValue(params.ClusterType),
		ClusterName:    swag.StringValue(params.Name),
		ManagementHost: obj.ManagementHost,
		DriverType:     c.app.AppArgs.DriverType,
	}
	deploy, err := c.app.AppCSP.GetMCDeployment(args)
	if err != nil {
		err = c.eInvalidData("%s", err.Error())
		return ops.NewCspDomainDeploymentFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	ret := ops.NewCspDomainDeploymentFetchOK()
	ret.Payload = &models.CspDomainDeploymentFetchOKBody{}
	ret.Payload.Deployment = deploy.Deployment
	ret.Payload.Format = deploy.Format
	return ret
}

func (c *HandlerComp) cspDomainMetadata(params ops.CspDomainMetadataParams) middleware.Responder {
	dt := models.CspDomainTypeMutable(params.CspDomainType)
	if !c.app.ValidateCspDomainType(dt) {
		err := c.eInvalidData("invalid cspDomainType '%s'", dt)
		return ops.NewCspDomainMetadataDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	md := &models.CSPDomainMetadata{}
	md.AttributeMetadata = c.app.DomainAttrDescForType(dt)
	ret := ops.NewCspDomainMetadataOK()
	ret.Payload = md
	return ret
}

func (c *HandlerComp) cspDomainServicePlanCost(params ops.CspDomainServicePlanCostParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewCspDomainServicePlanCostDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var spObj *models.ServicePlan
	obj, err := c.ops.intCspDomainFetch(ctx, ai, params.ID)
	if err == nil {
		spObj, err = c.ops.intServicePlanFetch(ctx, ai, params.ServicePlanID)
	}
	if err != nil {
		return ops.NewCspDomainServicePlanCostDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	spCost, err := c.app.AppCSP.DomainServicePlanCost(ctx, obj, spObj)
	if err != nil {
		err = c.eErrorNotFound("%s", err.Error())
		return ops.NewCspDomainServicePlanCostDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewCspDomainServicePlanCostOK().WithPayload(spCost)
}
