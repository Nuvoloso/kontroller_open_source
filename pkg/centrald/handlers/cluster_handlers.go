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

	"github.com/Masterminds/semver"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/cluster"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/node"
	spa "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/service_plan_allocation"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
)

// register handlers for Cluster
func (c *HandlerComp) clusterRegisterHandlers() {
	c.app.API.ClusterClusterCreateHandler = ops.ClusterCreateHandlerFunc(c.clusterCreate)
	c.app.API.ClusterClusterDeleteHandler = ops.ClusterDeleteHandlerFunc(c.clusterDelete)
	c.app.API.ClusterClusterFetchHandler = ops.ClusterFetchHandlerFunc(c.clusterFetch)
	c.app.API.ClusterClusterListHandler = ops.ClusterListHandlerFunc(c.clusterList)
	c.app.API.ClusterClusterUpdateHandler = ops.ClusterUpdateHandlerFunc(c.clusterUpdate)
	c.app.API.ClusterClusterAccountSecretFetchHandler = ops.ClusterAccountSecretFetchHandlerFunc(c.clusterAccountSecretFetch)
	c.app.API.ClusterClusterOrchestratorGetDeploymentHandler = ops.ClusterOrchestratorGetDeploymentHandlerFunc(c.clusterOrchestratorGetDeployment)
}

var nmClusterMutable JSONToAttrNameMap

func (c *HandlerComp) clusterMutableNameMap() JSONToAttrNameMap {
	if nmClusterMutable == nil {
		c.cacheMux.Lock()
		defer c.cacheMux.Unlock()
		if nmClusterMutable == nil {
			nmClusterMutable = c.makeJSONToAttrNameMap(models.ClusterMutable{})
		}
	}
	return nmClusterMutable
}

// clusterFetchFilter verifies the caller has permission to fetch the object, returning an error if they do not
func (c *HandlerComp) clusterFetchFilter(ai *auth.Info, obj *models.Cluster) error {
	err := ai.CapOK(centrald.CSPDomainManagementCap, models.ObjIDMutable(obj.AccountID))
	if err != nil && len(obj.AuthorizedAccounts) > 0 {
		err = ai.CapOK(centrald.CSPDomainUsageCap, obj.AuthorizedAccounts...)
	}
	return err
}

// clusterApplyInheritedProperties silently inserts inherited properties
func (c *HandlerComp) clusterApplyInheritedProperties(ctx context.Context, ai *auth.Info, obj *models.Cluster, domObj *models.CSPDomain) error {
	inheritCUP := false
	if obj.ClusterUsagePolicy == nil || obj.ClusterUsagePolicy.Inherited {
		inheritCUP = true
	}
	if inheritCUP {
		if domObj == nil {
			var err error
			domObj, err = c.intCspDomainFetch(ctx, ai, string(obj.CspDomainID))
			if err != nil {
				return err
			}
		}
		obj.ClusterUsagePolicy = &models.ClusterUsagePolicy{}
		*obj.ClusterUsagePolicy = *domObj.ClusterUsagePolicy
		obj.ClusterUsagePolicy.Inherited = true
	}
	if !ai.Internal() { // only the internal user should be able to view the clusterIdentifier property of a Cluster object.
		obj.ClusterIdentifier = ""
	}
	return nil
}

// Handlers

func (c *HandlerComp) clusterCreate(params ops.ClusterCreateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewClusterCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err := centrald.ErrorMissing
		return ops.NewClusterCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload.Name == "" || params.Payload.ClusterType == "" || params.Payload.CspDomainID == "" {
		err := c.eMissingMsg("name, clusterType and cspDomainId required")
		return ops.NewClusterCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if !c.app.ValidateClusterType(params.Payload.ClusterType) {
		err := c.eMissingMsg("clusterType must be one of: %s", c.app.SupportedClusterTypes())
		return ops.NewClusterCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	// TBD: validate clusterAttributes by clusterType
	if params.Payload.ClusterIdentifier != "" || params.Payload.State != "" || len(params.Payload.Messages) != 0 {
		if err = ai.InternalOK(); err != nil {
			if err == centrald.ErrorUnauthorizedOrForbidden {
				err = c.eUnauthorizedOrForbidden("set clusterIdentifier, messages or state")
			}
			return ops.NewClusterCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if params.Payload.State == "" {
		params.Payload.State = common.ClusterStateDeployable
	} else {
		if !c.validateClusterState(params.Payload.State) {
			err := c.eMissingMsg("invalid state")
			return ops.NewClusterCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	c.RLock()
	defer c.RUnlock()
	if err = c.app.AuditLog.Ready(); err != nil {
		return ops.NewClusterCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	dom, err := c.ops.intCspDomainFetch(ctx, ai, string(params.Payload.CspDomainID))
	if err != nil {
		if err == centrald.ErrorNotFound {
			err = c.eMissingMsg("invalid cspDomainId")
		}
		return ops.NewClusterCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err = ai.CapOK(centrald.CSPDomainManagementCap, dom.AccountID); err != nil {
		c.app.AuditLog.Post(ctx, ai, centrald.ClusterCreateAction, "", params.Payload.Name, "", true, "Create unauthorized")
		return ops.NewClusterCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	params.Payload.AccountID = models.ObjID(dom.AccountID)
	if ai.Internal() && ai.AccountID == "" {
		// Assume account is the domain owner account because clusterd normally creates the object as a side effect of the deployment YAML being applied
		ai.AccountID = string(dom.AccountID)
		aObj, err := c.DS.OpsAccount().Fetch(ctx, ai.AccountID)
		if err != nil {
			return ops.NewClusterCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		ai.AccountName = string(aObj.Name)
	}
	for _, aid := range params.Payload.AuthorizedAccounts {
		aObj, err := c.DS.OpsAccount().Fetch(ctx, string(aid))
		if err == nil {
			// tenant admin can authorize their own account or any subordinate account
			if err = ai.CapOK(centrald.CSPDomainManagementCap, aid, aObj.TenantAccountID); err != nil {
				c.app.AuditLog.Post(ctx, ai, centrald.ClusterCreateAction, "", params.Payload.Name, "", true, fmt.Sprintf("Create attempted with incorrect authorizedAccount %s[%s]", aObj.Name, aObj.Meta.ID))
			}
		}
		if err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eMissingMsg("invalid authorizedAccounts: %s", aid)
			}
			return ops.NewClusterCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	obj, err := c.DS.OpsCluster().Create(ctx, params.Payload)
	if err != nil {
		return ops.NewClusterCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.clusterApplyInheritedProperties(ctx, ai, obj, dom) // no error possible
	c.app.AuditLog.Post(ctx, ai, centrald.ClusterCreateAction, obj.Meta.ID, obj.Name, obj.CspDomainID, false, "Created")
	c.Log.Infof("Cluster %s created [%s] in domain %s [%s]", obj.Name, obj.Meta.ID, dom.Name, dom.Meta.ID)
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	return ops.NewClusterCreateCreated().WithPayload(obj)
}

func (c *HandlerComp) clusterDelete(params ops.ClusterDeleteParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewClusterDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var obj *models.Cluster
	if obj, err = c.DS.OpsCluster().Fetch(ctx, params.ID); err == nil {
		if err = c.app.AuditLog.Ready(); err == nil {
			c.Lock()
			defer c.Unlock()
			if err = ai.CapOK(centrald.CSPDomainManagementCap, models.ObjIDMutable(obj.AccountID)); err == nil {
				srlParams := storage_request.StorageRequestListParams{ClusterID: &params.ID, IsTerminated: swag.Bool(false)}
				vrlParams := volume_series_request.VolumeSeriesRequestListParams{ClusterID: &params.ID, IsTerminated: swag.Bool(false)}
				var n int
				if n, err = c.DS.OpsStorageRequest().Count(ctx, srlParams, 1); err == nil {
					if n != 0 {
						err = &centrald.Error{M: "active storage requests are still associated with the cluster", C: centrald.ErrorExists.C}
					} else if n, err = c.DS.OpsVolumeSeries().Count(ctx, volume_series.VolumeSeriesListParams{BoundClusterID: &params.ID}, 1); err == nil {
						if n != 0 {
							err = &centrald.Error{M: "volume series are still bound to the cluster", C: centrald.ErrorExists.C}
						} else if n, err = c.DS.OpsNode().Count(ctx, node.NodeListParams{ClusterID: &params.ID}, 1); err == nil {
							if n != 0 {
								err = &centrald.Error{M: "nodes still exist in the cluster", C: centrald.ErrorExists.C}
							} else if n, err = c.DS.OpsVolumeSeriesRequest().Count(ctx, vrlParams, 1); err == nil {
								if n != 0 {
									err = &centrald.Error{M: "active volume series requests are still associated with the cluster", C: centrald.ErrorExists.C}
								} else if n, err = c.DS.OpsServicePlanAllocation().Count(ctx, spa.ServicePlanAllocationListParams{ClusterID: &params.ID}, 1); err == nil {
									if n != 0 {
										err = &centrald.Error{M: "service plan allocation objects are still associated with the cluster", C: centrald.ErrorExists.C}
									} else if err = c.DS.OpsCluster().Delete(ctx, params.ID); err == nil {
										c.app.AuditLog.Post(ctx, ai, centrald.ClusterDeleteAction, obj.Meta.ID, obj.Name, "", false, "Deleted")
										c.Log.Infof("Cluster %s deleted [%s]", obj.Name, obj.Meta.ID)
										c.setDefaultObjectScope(params.HTTPRequest, obj)
										return ops.NewClusterDeleteNoContent()
									}
								}
							}
						}
					}
				}
			} else {
				c.app.AuditLog.Post(ctx, ai, centrald.ClusterDeleteAction, obj.Meta.ID, obj.Name, "", true, "Delete unauthorized")
			}
		}
	}
	return ops.NewClusterDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
}

func (c *HandlerComp) clusterFetch(params ops.ClusterFetchParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewClusterFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.intClusterFetch(ctx, ai, params.ID)
	if err != nil {
		return ops.NewClusterFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewClusterFetchOK().WithPayload(obj)
}

// intClusterFetch is the internal shareable part of clusterFetch; protect appropriately!
func (c *HandlerComp) intClusterFetch(ctx context.Context, ai *auth.Info, id string) (*models.Cluster, error) {
	obj, err := c.DS.OpsCluster().Fetch(ctx, id)
	if err == nil {
		if err = c.clusterFetchFilter(ai, obj); err == nil {
			err = c.clusterApplyInheritedProperties(ctx, ai, obj, nil)
		}
	}
	return obj, err
}

func (c *HandlerComp) clusterList(params ops.ClusterListParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewClusterListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var skip bool
	params.AuthorizedAccountID, params.AccountID, skip = c.ops.constrainBothQueryAccounts(ai, params.AuthorizedAccountID, centrald.CSPDomainUsageCap, params.AccountID, centrald.CSPDomainManagementCap)
	if skip {
		return ops.NewClusterListOK()
	}

	c.RLock()
	defer c.RUnlock()
	list, err := c.DS.OpsCluster().List(ctx, params)
	if err != nil {
		return ops.NewClusterListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	domCache := map[string]*models.CSPDomain{}
	ret := make([]*models.Cluster, 0, len(list))
	for _, obj := range list {
		domObj, ok := domCache[string(obj.CspDomainID)]
		if !ok {
			if domObj, err = c.ops.intCspDomainFetch(ctx, ai, string(obj.CspDomainID)); err != nil {
				c.Log.Errorf("Cluster[%s]: error looking up CSPDomain[%s]: %s", obj.Meta.ID, obj.CspDomainID, err.Error())
				continue // skip the cluster
			}
			domCache[string(obj.CspDomainID)] = domObj
		}
		c.clusterApplyInheritedProperties(ctx, ai, obj, domObj) // no error possible
		ret = append(ret, obj)
	}
	return ops.NewClusterListOK().WithPayload(ret)
}

var emptyCluster = &models.Cluster{}

func (c *HandlerComp) clusterUpdate(params ops.ClusterUpdateParams) middleware.Responder {
	obj, err := c.intClusterUpdate(params, nil, nil)
	if err != nil {
		return ops.NewClusterUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewClusterUpdateOK().WithPayload(obj)
}

// intClusterUpdate is the body of the update handler that can be used in a different context.
// It will obtain the needed locks (mux.RLock, clusterMux.Lock) if oObj is nil
func (c *HandlerComp) intClusterUpdate(params ops.ClusterUpdateParams, ai *auth.Info, oObj *models.Cluster) (*models.Cluster, error) {
	ctx := params.HTTPRequest.Context()
	var err error
	if ai == nil {
		ai, err = c.GetAuthInfo(params.HTTPRequest)
		if err != nil {
			return nil, err
		}
	}
	var uP = [centrald.NumActionTypes][]string{
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    params.Set,
	}
	ua, err := c.MakeStdUpdateArgs(emptyCluster, params.ID, params.Version, uP)
	if err != nil {
		return nil, err
	}
	if params.Payload == nil {
		err = c.eUpdateInvalidMsg("missing payload")
		return nil, err
	}
	if ua.IsModified("Name") && params.Payload.Name == "" {
		err := c.eUpdateInvalidMsg("non-empty name is required")
		return nil, err
	}
	if oObj == nil {
		c.RLock()
		defer c.RUnlock()
		c.ClusterLock()
		defer c.ClusterUnlock()
		oObj, err = c.DS.OpsCluster().Fetch(ctx, params.ID)
		if err != nil {
			return nil, err
		}
	}
	if ua.IsModified("ClusterUsagePolicy") {
		if oObj.State != common.ClusterStateDeployable {
			err := c.eUpdateInvalidMsg("invalid state")
			return nil, err
		}
		if err := c.validateClusterUsagePolicy(params.Payload.ClusterUsagePolicy, common.AccountSecretScopeCluster); err != nil {
			return nil, err
		}
	}
	if ua.Version == 0 {
		ua.Version = int32(oObj.Meta.Version)
	} else if int32(oObj.Meta.Version) != ua.Version {
		err = centrald.ErrorIDVerNotFound
		return nil, err
	}
	if err = c.app.AuditLog.Ready(); err != nil {
		return nil, err
	}
	if ua.IsModified("ClusterVersion") || ua.IsModified("Service") || ua.IsModified("ClusterAttributes") || ua.IsModified("ClusterIdentifier") || ua.IsModified("State") || ua.IsModified("Messages") {
		if err = ai.InternalOK(); err != nil {
			c.app.AuditLog.Post(ctx, ai, centrald.ClusterUpdateAction, models.ObjID(params.ID), models.ObjName(oObj.Name), "", true, "Update unauthorized")
			return nil, err
		}
	} else {
		if err = ai.CapOK(centrald.CSPDomainManagementCap, models.ObjIDMutable(oObj.AccountID)); err != nil {
			c.app.AuditLog.Post(ctx, ai, centrald.ClusterUpdateAction, models.ObjID(params.ID), models.ObjName(oObj.Name), "", true, "Update unauthorized")
			return nil, err
		}
	}
	if ua.IsModified("ClusterIdentifier") {
		if !ua.IsModified("State") {
			err := c.eMissingMsg("state must be set with clusterIdentifier")
			return nil, err
		}
		// when transitioning to DEPLOYABLE state ClusterIdentifier must be reset, e.g. set to empty string
		if params.Payload.State == common.ClusterStateDeployable && params.Payload.ClusterIdentifier != "" {
			err := c.eMissingMsg("clusterIdentifier must be cleared when transitioning to %s", common.ClusterStateDeployable)
			return nil, err
		}
		// ClusterIdentifier may be modified (set to non-empty value) only when changing state from DEPLOYABLE to MANAGED
		if !(oObj.State == common.ClusterStateDeployable && params.Payload.State == common.ClusterStateManaged) {
			err := c.eInvalidState("invalid state transition (%s ⇒ %s)", oObj.State, params.Payload.State)
			return nil, err
		}
	}
	if ua.IsModified("State") {
		if !c.validateClusterState(params.Payload.State) {
			err := c.eUpdateInvalidMsg("invalid cluster state")
			return nil, err
		}
		// when transitioning from DEPLOYABLE state to MANAGED ClusterIdentifier is required
		if oObj.State == common.ClusterStateDeployable && params.Payload.State == common.ClusterStateManaged && (!ua.IsModified("ClusterIdentifier") || params.Payload.ClusterIdentifier == "") {
			err := c.eMissingMsg("clusterIdentifier must be set when transitioning to %s", common.ClusterStateManaged)
			return nil, err
		}
	}
	dom, err := c.ops.intCspDomainFetch(ctx, ai, string(oObj.CspDomainID))
	if err != nil {
		c.Log.Errorf("Cluster[%s]: error looking up CSPDomain[%s]: %s", oObj.Meta.ID, oObj.CspDomainID, err.Error())
		return nil, err
	}
	detail := ""
	if a := ua.FindUpdateAttr("AuthorizedAccounts"); a != nil && a.IsModified() {
		detail, err = c.authAccountValidator.validateAuthorizedAccountsUpdate(ctx, ai, centrald.ClusterUpdateAction, params.ID, models.ObjName(oObj.Name), a, oObj.AuthorizedAccounts, params.Payload.AuthorizedAccounts)
		if err != nil {
			return nil, err
		}
	}
	// TBD: validate clusterAttributes by clusterType
	obj, err := c.DS.OpsCluster().Update(ctx, ua, params.Payload)
	if err != nil {
		return nil, err
	}
	c.clusterApplyInheritedProperties(ctx, ai, obj, dom) // no error possible
	if len(detail) > 0 {
		c.app.AuditLog.Post(ctx, ai, centrald.ClusterUpdateAction, models.ObjID(params.ID), models.ObjName(oObj.Name), "", false, fmt.Sprintf("Updated authorizedAccounts %s", detail))
	}
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	return obj, nil
}

func (c *HandlerComp) clusterAccountSecretFetch(params ops.ClusterAccountSecretFetchParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewClusterAccountSecretFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.AuthorizedAccountID == "" {
		err = c.eMissingMsg("authorizedAccountId")
		return ops.NewClusterAccountSecretFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.RLock()
	defer c.RUnlock()
	oObj, err := c.ops.intClusterFetch(ctx, ai, params.ID) // does appropriate auth check too
	if err != nil {
		return ops.NewClusterAccountSecretFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if !util.Contains(oObj.AuthorizedAccounts, models.ObjIDMutable(params.AuthorizedAccountID)) {
		err = c.eMissingMsg("authorizedAccountId")
		return ops.NewClusterAccountSecretFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if oObj.State != common.ClusterStateManaged {
		err = c.eInvalidState("cluster state not %s", common.ClusterStateManaged)
		return ops.NewClusterAccountSecretFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	cc := c.lookupClusterClient(oObj.ClusterType)
	if cc == nil {
		err = c.eInvalidData("support for clusterType '%s' missing", oObj.ClusterType)
		return ops.NewClusterAccountSecretFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	ns := ""
	if oObj.ClusterType == cluster.K8sClusterType {
		ns = swag.StringValue(params.K8sNamespace)
	}
	vt, err := c.ops.intAccountSecretRetrieve(ctx, ai, params.AuthorizedAccountID, params.ID, oObj)
	if err != nil {
		return ops.NewClusterAccountSecretFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	sca := &cluster.SecretCreateArgsMV{
		Intent:            cluster.SecretIntentAccountIdentity,
		Name:              common.AccountSecretClusterObjectName,
		Namespace:         ns,
		CustomizationData: cluster.AccountVolumeData{AccountSecret: vt.Value},
	}
	ss, err := cc.SecretFormatMV(ctx, sca)
	if err != nil {
		err = c.eInvalidData("%s", err.Error())
		return ops.NewClusterAccountSecretFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewClusterAccountSecretFetchOK().WithPayload(&models.ValueType{Kind: common.ValueTypeSecret, Value: ss})
}

func (c *HandlerComp) clusterOrchestratorGetDeployment(params ops.ClusterOrchestratorGetDeploymentParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewClusterOrchestratorGetDeploymentDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	// existential lock and c.clusterMux acquired to serialize against any Cluster object modification
	c.RLock()
	defer c.RUnlock()
	c.ClusterLock()
	defer c.ClusterUnlock()
	oObj, err := c.ops.intClusterFetch(ctx, ai, params.ID) // does appropriate auth check too
	if err != nil {
		return ops.NewClusterOrchestratorGetDeploymentDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	// verify state to be DEPLOYABLE to allow operation
	// NOTE: One can also use this operation in the MANAGED state in case the cluster software deployment is required for any other purpose.
	if !(oObj.State == common.ClusterStateDeployable || oObj.State == common.ClusterStateManaged) {
		err := c.eInvalidState("permitted only in the %s or %s states", common.ClusterStateDeployable, common.ClusterStateManaged)
		return ops.NewClusterOrchestratorGetDeploymentDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	dom, err := c.ops.intCspDomainFetch(ctx, ai, string(oObj.CspDomainID))
	if err != nil {
		c.Log.Errorf("Cluster[%s]: error looking up CSPDomain[%s]: %s", oObj.Meta.ID, oObj.CspDomainID, err.Error())
		return ops.NewClusterOrchestratorGetDeploymentDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if dom.ManagementHost == "" {
		err := c.eInvalidData("Cluster[%s]: managementHost property must be present in associated CSPDomain[%s]", oObj.Meta.ID, oObj.CspDomainID)
		return ops.NewClusterOrchestratorGetDeploymentDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	sysObj, err := c.ops.intSystemFetch()
	if err != nil {
		return ops.NewClusterOrchestratorGetDeploymentDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	clusterOrchestratorType := swag.StringValue(params.OrchestratorType)
	if clusterOrchestratorType == "" {
		clusterOrchestratorType = oObj.ClusterType
	} else {
		if swag.StringValue(params.OrchestratorVersion) == "" {
			err = c.eInvalidData("Must set orchestrator version if type is specified")
			return ops.NewClusterOrchestratorGetDeploymentDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if swag.StringValue(params.OrchestratorVersion) != "" {
		if _, err = semver.NewVersion(swag.StringValue(params.OrchestratorVersion)); err != nil {
			err := c.eInvalidData("Invalid semantic version: %s", swag.StringValue(params.OrchestratorVersion))
			return ops.NewClusterOrchestratorGetDeploymentDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	args := &cluster.MCDeploymentArgs{
		SystemID:       string(sysObj.Meta.ID),
		CSPDomainID:    string(oObj.CspDomainID),
		ClusterID:      params.ID,
		CSPDomainType:  string(dom.CspDomainType),
		ClusterType:    clusterOrchestratorType,
		ClusterVersion: swag.StringValue(params.OrchestratorVersion),
		ClusterName:    string(oObj.Name),
		ManagementHost: dom.ManagementHost,
		DriverType:     c.app.AppArgs.DriverType,
	}
	deploy, err := c.app.AppCSP.GetMCDeployment(args)
	if err != nil {
		err = c.eInvalidData("%s", err.Error())
		return ops.NewClusterOrchestratorGetDeploymentDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	ret := ops.NewClusterOrchestratorGetDeploymentOK()
	ret.Payload = &models.ClusterOrchestratorGetDeploymentOKBody{}
	ret.Payload.Deployment = deploy.Deployment
	ret.Payload.Format = deploy.Format
	return ret
}
