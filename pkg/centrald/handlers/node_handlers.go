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
	"fmt"
	"net/http"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/node"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/pool"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/storage"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
)

// Register handlers for Node
func (c *HandlerComp) nodeRegisterHandlers() {
	c.app.API.NodeNodeCreateHandler = ops.NodeCreateHandlerFunc(c.nodeCreate)
	c.app.API.NodeNodeDeleteHandler = ops.NodeDeleteHandlerFunc(c.nodeDelete)
	c.app.API.NodeNodeFetchHandler = ops.NodeFetchHandlerFunc(c.nodeFetch)
	c.app.API.NodeNodeListHandler = ops.NodeListHandlerFunc(c.nodeList)
	c.app.API.NodeNodeUpdateHandler = ops.NodeUpdateHandlerFunc(c.nodeUpdate)
}

var nmNodeMutable JSONToAttrNameMap

func (c *HandlerComp) nodeMutableNameMap() JSONToAttrNameMap {
	if nmNodeMutable == nil {
		c.cacheMux.Lock()
		defer c.cacheMux.Unlock()
		if nmNodeMutable == nil {
			nmNodeMutable = c.makeJSONToAttrNameMap(models.NodeMutable{})
		}
	}
	return nmNodeMutable
}

// NodeAndCluster is used to set the accessScope for node events
type NodeAndCluster struct {
	Node    *models.Node
	Cluster *models.Cluster
}

// nodeFetchFilter verifies the caller has permission to fetch the object, returning an error if they do not
func (c *HandlerComp) nodeFetchFilter(ai *auth.Info, obj *models.Node, cObj *models.Cluster) error {
	err := ai.CapOK(centrald.CSPDomainManagementCap, obj.AccountID)
	if err != nil && len(cObj.AuthorizedAccounts) > 0 {
		err = ai.CapOK(centrald.CSPDomainUsageCap, cObj.AuthorizedAccounts...)
	}
	return err
}

// Handlers

func (c *HandlerComp) nodeCreate(params ops.NodeCreateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewNodeCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err := centrald.ErrorMissing
		return ops.NewNodeCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload.Name == "" || params.Payload.ClusterID == "" {
		err := c.eMissingMsg("clusterId and name required")
		return ops.NewNodeCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload.State == "" {
		params.Payload.State = common.NodeStateManaged
	} else {
		if !c.validateNodeState(params.Payload.State) {
			err := c.eMissingMsg("invalid state")
			return ops.NewNodeCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	c.RLock()
	defer c.RUnlock()
	if err = c.app.AuditLog.Ready(); err != nil {
		return ops.NewNodeCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var cObj *models.Cluster
	if cObj, err = c.DS.OpsCluster().Fetch(ctx, string(params.Payload.ClusterID)); err != nil {
		if err == centrald.ErrorNotFound {
			err = c.eMissingMsg("invalid clusterId")
		}
		return ops.NewNodeCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	params.Payload.AccountID = models.ObjIDMutable(cObj.AccountID)
	if err = ai.CapOK(centrald.CSPDomainManagementCap, params.Payload.AccountID); err != nil {
		c.app.AuditLog.Post(ctx, ai, centrald.NodeCreateAction, "", params.Payload.Name, "", true, "Create unauthorized")
		return ops.NewNodeCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ai.Internal() && ai.AccountID == "" {
		// Assume account is the cluster owner account because agentd normally creates the object as a side effect of the deployment YAML being applied
		ai.AccountID = string(params.Payload.AccountID)
		aObj, err := c.DS.OpsAccount().Fetch(ctx, ai.AccountID)
		if err != nil {
			return ops.NewNodeCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		ai.AccountName = string(aObj.Name)
	}
	if err = c.app.ValidateNodeAttributes(cObj.ClusterType, params.Payload.NodeAttributes); err != nil {
		err = c.eMissingMsg("nodeAttributes are invalid")
		return ops.NewNodeCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.DS.OpsNode().Create(ctx, params.Payload)
	if err != nil {
		return ops.NewNodeCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.app.AuditLog.Event(ctx, ai, centrald.ClusterUpdateAction, cObj.Meta.ID, cObj.Name, models.ObjIDMutable(obj.Meta.ID), false, fmt.Sprintf("Added node [%s]", obj.Name))
	// node object is created by a heartbeat and is never created in UNKNOWN state
	c.app.AuditLog.Event(ctx, ai, centrald.NodeUpdateAction, obj.Meta.ID, obj.Name, "", false, "Started heartbeat")
	c.Log.Infof("Node %s created [%s]", obj.Name, obj.Meta.ID)
	c.setScopeNode(params.HTTPRequest, obj, cObj, true, true)
	return ops.NewNodeCreateCreated().WithPayload(obj)
}

func (c *HandlerComp) nodeDelete(params ops.NodeDeleteParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewNodeDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var obj *models.Node
	if obj, err = c.DS.OpsNode().Fetch(ctx, params.ID); err == nil {
		if err = c.app.AuditLog.Ready(); err == nil {
			c.Lock()
			defer c.Unlock()
			var cObj *models.Cluster
			if cObj, err = c.DS.OpsCluster().Fetch(ctx, string(obj.ClusterID)); err != nil {
				return ops.NewNodeDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			if err = ai.CapOK(centrald.CSPDomainManagementCap, obj.AccountID); err == nil {
				srlParams := storage_request.StorageRequestListParams{NodeID: &params.ID, IsTerminated: swag.Bool(false)}
				saParams := storage.StorageListParams{AttachedNodeID: &params.ID}
				spParams := pool.PoolListParams{AccessibilityScope: swag.String("NODE"), AccessibilityScopeObjID: &params.ID}
				vrlParams := volume_series_request.VolumeSeriesRequestListParams{NodeID: &params.ID, IsTerminated: swag.Bool(false), RequestedOperationsNot: []string{common.VolReqOpNodeDelete}} // NODE_DELETE deletes the object
				var n int
				if n, err = c.DS.OpsStorageRequest().Count(ctx, srlParams, 1); err == nil {
					if n != 0 {
						err = &centrald.Error{M: "active storage requests are still associated with the node", C: centrald.ErrorExists.C}
					} else if n, err = c.DS.OpsVolumeSeries().Count(ctx, volume_series.VolumeSeriesListParams{ConfiguredNodeID: &params.ID}, 1); err == nil {
						if n != 0 {
							err = &centrald.Error{M: "volume series are still configured on the node", C: centrald.ErrorExists.C}
						} else if n, err = c.DS.OpsStorage().Count(ctx, saParams, 1); err == nil {
							if n != 0 {
								err = &centrald.Error{M: "storage must be detached from the node", C: centrald.ErrorExists.C}
							} else if n, err = c.DS.OpsPool().Count(ctx, spParams, 1); err == nil {
								if n != 0 {
									err = &centrald.Error{M: "pools are still associated with the node", C: centrald.ErrorExists.C}
								} else if n, err = c.DS.OpsVolumeSeriesRequest().Count(ctx, vrlParams, 1); err == nil {
									if n != 0 {
										err = &centrald.Error{M: "active volume series requests are still associated with the node", C: centrald.ErrorExists.C}
									} else if err = c.DS.OpsNode().Delete(ctx, params.ID); err == nil {
										c.app.AuditLog.Event(ctx, ai, centrald.ClusterUpdateAction, cObj.Meta.ID, cObj.Name, models.ObjIDMutable(obj.Meta.ID), false, fmt.Sprintf("Deleted node [%s]", obj.Name))
										c.Log.Infof("Node %s deleted [%s]", obj.Name, obj.Meta.ID)
										c.setScopeNode(params.HTTPRequest, obj, cObj, true, true)
										return ops.NewNodeDeleteNoContent()
									}
								}
							}
						}
					}
				}
			} else {
				c.app.AuditLog.Post(ctx, ai, centrald.NodeDeleteAction, obj.Meta.ID, obj.Name, "", true, "Delete unauthorized")
			}
		}
	}
	return ops.NewNodeDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
}

func (c *HandlerComp) nodeFetch(params ops.NodeFetchParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewNodeFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.DS.OpsNode().Fetch(ctx, params.ID)
	if err == nil {
		c.RLock()
		defer c.RUnlock()
		var cObj *models.Cluster
		if cObj, err = c.DS.OpsCluster().Fetch(ctx, string(obj.ClusterID)); err == nil {
			err = c.nodeFetchFilter(ai, obj, cObj)
		}
	}
	if err != nil {
		return ops.NewNodeFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewNodeFetchOK().WithPayload(obj)
}

func (c *HandlerComp) nodeList(params ops.NodeListParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewNodeListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	list, err := c.DS.OpsNode().List(ctx, params)
	if err != nil {
		return ops.NewNodeListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	ret := make([]*models.Node, 0, len(list))
	c.RLock()
	defer c.RUnlock()
	var cObj *models.Cluster
	for _, obj := range list {
		if cObj == nil || cObj.Meta.ID != models.ObjID(obj.ClusterID) {
			if cObj, err = c.DS.OpsCluster().Fetch(ctx, string(obj.ClusterID)); err != nil {
				return ops.NewNodeListDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
		}
		if c.nodeFetchFilter(ai, obj, cObj) == nil {
			ret = append(ret, obj)
		}
	}
	return ops.NewNodeListOK().WithPayload(ret)
}

var emptyNode = &models.Node{}

func (c *HandlerComp) nodeUpdate(params ops.NodeUpdateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewNodeUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if util.Contains(params.Set, "totalCacheBytes") {
		if util.Contains(params.Set, "availableCacheBytes") {
			err = c.eUpdateInvalidMsg("availableCacheBytes cannot be updated together with totalCacheBytes")
			return ops.NewNodeUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		params.Set = append(params.Set, "availableCacheBytes") // auto-updated below after old object is fetched
	}
	var uP = [centrald.NumActionTypes][]string{
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    params.Set,
	}
	ua, err := c.MakeStdUpdateArgs(emptyNode, params.ID, params.Version, uP)
	if err != nil {
		return ops.NewNodeUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err = centrald.ErrorUpdateInvalidRequest
		return ops.NewNodeUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.IsModified("Name") && params.Payload.Name == "" {
		err := c.eUpdateInvalidMsg("non-empty name is required")
		return ops.NewNodeUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.RLock()
	defer c.RUnlock()
	c.NodeLock()
	defer c.NodeUnlock()
	oObj, err := c.DS.OpsNode().Fetch(ctx, params.ID)
	if err != nil {
		return ops.NewNodeUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	} else if ua.Version == 0 {
		ua.Version = int32(oObj.Meta.Version)
	} else if int32(oObj.Meta.Version) != ua.Version {
		err = centrald.ErrorIDVerNotFound
		return ops.NewNodeUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err = c.app.AuditLog.Ready(); err != nil {
		return ops.NewNodeUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.IsModified("AvailableCacheBytes") ||
		ua.IsModified("CacheUnitSizeBytes") ||
		ua.IsModified("LocalStorage") ||
		ua.IsModified("NodeAttributes") ||
		ua.IsModified("Service") ||
		ua.IsModified("State") ||
		ua.IsModified("TotalCacheBytes") {
		if err = ai.InternalOK(); err != nil {
			c.app.AuditLog.Post(ctx, ai, centrald.NodeUpdateAction, models.ObjID(params.ID), models.ObjName(oObj.Name), "", true, "Update unauthorized")
			return ops.NewNodeUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if ua.IsModified("State") {
			if c.validateNodeState(params.Payload.State) {
				if oObj.State == common.NodeStateTearDown && params.Payload.State != oObj.State {
					err = c.eInvalidState("invalid node state transition")
				}
			} else {
				err = c.eUpdateInvalidMsg("invalid node state")
			}
			if err != nil {
				return ops.NewNodeUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
		}
	} else {
		if err = ai.CapOK(centrald.CSPDomainManagementCap, oObj.AccountID); err != nil {
			c.app.AuditLog.Post(ctx, ai, centrald.NodeUpdateAction, models.ObjID(params.ID), models.ObjName(oObj.Name), "", true, "Update unauthorized")
			return ops.NewNodeUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	totalCacheBytes := swag.Int64Value(oObj.TotalCacheBytes)
	if ua.IsModified("TotalCacheBytes") {
		difference := swag.Int64Value(params.Payload.TotalCacheBytes) - swag.Int64Value(oObj.TotalCacheBytes)
		availableCacheBytes := swag.Int64Value(oObj.AvailableCacheBytes) + difference
		if availableCacheBytes < 0 {
			// can happen if cache fails while allocated, assume agentd will clean up
			availableCacheBytes = 0
		}
		params.Payload.AvailableCacheBytes = swag.Int64(availableCacheBytes)
		totalCacheBytes = swag.Int64Value(params.Payload.TotalCacheBytes)
	} else if ua.IsModified("AvailableCacheBytes") && swag.Int64Value(params.Payload.AvailableCacheBytes) > swag.Int64Value(oObj.TotalCacheBytes) {
		err = c.eUpdateInvalidMsg("availableCacheBytes must not be greater than totalCacheBytes")
		return ops.NewNodeUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.IsModified("CacheUnitSizeBytes") {
		if swag.Int64Value(params.Payload.CacheUnitSizeBytes) > totalCacheBytes {
			err = c.eUpdateInvalidMsg("cacheUnitSizeBytes must not be greater than totalCacheBytes")
			return ops.NewNodeUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	cObj, err := c.DS.OpsCluster().Fetch(ctx, string(oObj.ClusterID))
	if err != nil {
		return ops.NewNodeUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	// TBD: validate node attributes
	obj, err := c.DS.OpsNode().Update(ctx, ua, params.Payload)
	if err != nil {
		return ops.NewNodeUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	// sensitive updates require internal rights, no audit logging required
	serviceStateChanged := false
	beforeService := oObj.Service
	ns := obj.Service
	var pns *models.NuvoService
	if params.Payload != nil {
		pns = params.Payload.Service
	}
	if ua.IsModified("Service") && (ns == nil || (pns != nil && ns.State != pns.State)) {
		serviceStateChanged = true
	}
	stateChanged := false
	if ua.IsModified("State") && oObj.State != params.Payload.State {
		stateChanged = true
	}
	if ua.IsModified("Service") && ((beforeService == nil && ns != nil) || (beforeService != nil && ns.State != beforeService.State)) {
		var msg string
		if beforeService.State == common.ServiceStateUnknown || beforeService == nil {
			msg = "Started heartbeat"
		}
		if ns.State == common.ServiceStateUnknown {
			msg = "Stopped heartbeat"
		}
		if msg != "" {
			c.app.AuditLog.Event(ctx, ai, centrald.NodeUpdateAction, obj.Meta.ID, obj.Name, "", false, msg)
		}
	}
	c.setScopeNode(params.HTTPRequest, obj, cObj, stateChanged, serviceStateChanged)
	return ops.NewNodeUpdateOK().WithPayload(obj)
}

func (c *HandlerComp) setScopeNode(r *http.Request, obj *models.Node, cObj *models.Cluster, stateChanged bool, serviceStateChanged bool) {
	m := crude.ScopeMap{
		"clusterId":           string(obj.ClusterID),
		"state":               obj.State,
		"stateChanged":        fmt.Sprintf("%v", stateChanged),
		"serviceStateChanged": fmt.Sprintf("%v", serviceStateChanged),
	}
	if obj.Service != nil {
		m["serviceState"] = obj.Service.State
	}
	c.addNewObjIDToScopeMap(obj, m)
	c.app.CrudeOps.SetScope(r, m, &NodeAndCluster{Node: obj, Cluster: cObj})
}
