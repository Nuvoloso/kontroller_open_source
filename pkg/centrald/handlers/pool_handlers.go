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
	"net/http"

	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/pool"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/service_plan_allocation"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/storage"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
)

// Register handlers for Pool
func (c *HandlerComp) poolRegisterHandlers() {
	c.app.API.PoolPoolCreateHandler = ops.PoolCreateHandlerFunc(c.poolCreate)
	c.app.API.PoolPoolDeleteHandler = ops.PoolDeleteHandlerFunc(c.poolDelete)
	c.app.API.PoolPoolFetchHandler = ops.PoolFetchHandlerFunc(c.poolFetch)
	c.app.API.PoolPoolListHandler = ops.PoolListHandlerFunc(c.poolList)
	c.app.API.PoolPoolUpdateHandler = ops.PoolUpdateHandlerFunc(c.poolUpdate)
}

var nmPoolMutable JSONToAttrNameMap

func (c *HandlerComp) poolMutableNameMap() JSONToAttrNameMap {
	if nmPoolMutable == nil {
		c.cacheMux.Lock()
		defer c.cacheMux.Unlock()
		if nmPoolMutable == nil {
			nmPoolMutable = c.makeJSONToAttrNameMap(M.PoolMutable{})
		}
	}
	return nmPoolMutable
}

// poolFetchFilter verifies the caller has permission to fetch the object, returning an error if they do not
func (c *HandlerComp) poolFetchFilter(ai *auth.Info, obj *M.Pool) error {
	err := ai.CapOK(centrald.CSPDomainManagementCap, obj.AccountID)
	if err != nil {
		err = ai.CapOK(centrald.CSPDomainUsageCap, obj.AuthorizedAccountID)
	}
	return err
}

// Handlers

func (c *HandlerComp) poolCreate(params ops.PoolCreateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewPoolCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err = ai.CapOK(centrald.CSPDomainManagementCap); err != nil {
		return ops.NewPoolCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err := centrald.ErrorMissing
		return ops.NewPoolCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload.CspDomainID == "" || params.Payload.CspStorageType == "" {
		err := c.eMissingMsg("cspDomainId and cspStorageType required")
		return ops.NewPoolCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err = c.app.ValidateStorageAccessibility(params.Payload.CspDomainID, M.CspStorageType(params.Payload.CspStorageType), params.Payload.StorageAccessibility); err != nil {
		err = c.eMissingMsg("%s", err.Error())
		return ops.NewPoolCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.RLock()
	defer c.RUnlock()
	cObj, err := c.DS.OpsCspDomain().Fetch(ctx, string(params.Payload.CspDomainID))
	if err != nil {
		if err == centrald.ErrorNotFound {
			err = c.eMissingMsg("invalid cspDomainId")
		}
		return ops.NewPoolCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ai.AccountID != "" {
		// use auth info when specified (otherwise caller has internal role)
		params.Payload.AccountID = M.ObjIDMutable(ai.AccountID)
	} else {
		_, err := c.DS.OpsAccount().Fetch(ctx, string(params.Payload.AccountID))
		if err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eMissingMsg("invalid accountId")
			}
			return ops.NewPoolCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if params.Payload.AccountID != cObj.AccountID { // must be same owner
		err = centrald.ErrorUnauthorizedOrForbidden
		return ops.NewPoolCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload.AuthorizedAccountID != params.Payload.AccountID { // skip fetch if authorized is the tenant
		aObj, err := c.DS.OpsAccount().Fetch(ctx, string(params.Payload.AuthorizedAccountID))
		if err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eMissingMsg("invalid authorizedAccountId")
			}
			return ops.NewPoolCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if params.Payload.AccountID != aObj.TenantAccountID { // must be subordinate
			err = c.eMissingMsg("invalid authorizedAccountId")
			return ops.NewPoolCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	clObj, err := c.DS.OpsCluster().Fetch(ctx, string(params.Payload.ClusterID))
	if err != nil {
		if err == centrald.ErrorNotFound {
			err = c.eMissingMsg("invalid clusterId")
		}
		return ops.NewPoolCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if clObj.CspDomainID != params.Payload.CspDomainID {
		err = c.eMissingMsg("cluster not in specified CSP domain")
		return ops.NewPoolCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	err = c.app.ValidateCspDomainAndStorageTypes(cObj.CspDomainType,
		M.CspStorageType(params.Payload.CspStorageType))
	if err != nil {
		err = c.eMissingMsg("%s", err.Error())
		return ops.NewPoolCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload.StorageAccessibility.AccessibilityScope == "NODE" {
		if _, err = c.DS.OpsNode().Fetch(ctx, string(params.Payload.StorageAccessibility.AccessibilityScopeObjID)); err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eMissingMsg("invalid accessibilityScopeObjId for accessibilityScope NODE")
			}
			return ops.NewPoolCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	obj, err := c.DS.OpsPool().Create(ctx, params.Payload)
	if err != nil {
		return ops.NewPoolCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.Log.Infof("Pool %s created", obj.Meta.ID)
	c.setScopePool(params.HTTPRequest, obj)
	return ops.NewPoolCreateCreated().WithPayload(obj)
}

func (c *HandlerComp) poolDelete(params ops.PoolDeleteParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewPoolDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var obj *M.Pool
	if obj, err = c.DS.OpsPool().Fetch(ctx, params.ID); err == nil {
		c.Lock()
		defer c.Unlock()
		if err = ai.CapOK(centrald.CSPDomainManagementCap, obj.AccountID); err == nil {
			srlParams := storage_request.StorageRequestListParams{PoolID: &params.ID, IsTerminated: swag.Bool(false)}
			vrlParams := volume_series_request.VolumeSeriesRequestListParams{PoolID: &params.ID, IsTerminated: swag.Bool(false)}
			var n int
			if n, err = c.DS.OpsStorageRequest().Count(ctx, srlParams, 1); err == nil {
				if n != 0 {
					err = &centrald.Error{M: "active storage requests are still associated with the pool", C: centrald.ErrorExists.C}
				} else if n, err = c.DS.OpsStorage().Count(ctx, storage.StorageListParams{PoolID: &params.ID}, 1); err == nil {
					if n != 0 {
						err = &centrald.Error{M: "storage objects are still associated with the pool", C: centrald.ErrorExists.C}
					} else if n, err = c.DS.OpsVolumeSeries().Count(ctx, volume_series.VolumeSeriesListParams{PoolID: &params.ID}, 1); err == nil {
						if n != 0 {
							err = &centrald.Error{M: "volume series are still associated with the pool", C: centrald.ErrorExists.C}
						} else if n, err = c.DS.OpsVolumeSeriesRequest().Count(ctx, vrlParams, 1); err == nil {
							if n != 0 {
								err = &centrald.Error{M: "active volume series requests are still associated with the pool", C: centrald.ErrorExists.C}
							} else if n, err = c.DS.OpsServicePlanAllocation().Count(ctx, service_plan_allocation.ServicePlanAllocationListParams{PoolID: &params.ID}, 1); err == nil {
								if n != 0 {
									err = &centrald.Error{M: "service plan allocation objects are still associated with the pool", C: centrald.ErrorExists.C}
								} else if err = c.DS.OpsPool().Delete(ctx, params.ID); err == nil {
									c.Log.Infof("Pool %s deleted", obj.Meta.ID)
									c.setScopePool(params.HTTPRequest, obj)
									return ops.NewPoolDeleteNoContent()
								}
							}
						}
					}
				}
			}
		}
	}
	return ops.NewPoolDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
}

func (c *HandlerComp) poolFetch(params ops.PoolFetchParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewPoolFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.DS.OpsPool().Fetch(ctx, params.ID)
	if err == nil {
		err = c.poolFetchFilter(ai, obj)
	}
	if err != nil {
		return ops.NewPoolFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewPoolFetchOK().WithPayload(obj)
}

func (c *HandlerComp) poolList(params ops.PoolListParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewPoolListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	list, err := c.DS.OpsPool().List(ctx, params)
	if err != nil {
		return ops.NewPoolListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	ret := make([]*M.Pool, 0, len(list))
	for _, obj := range list {
		if c.poolFetchFilter(ai, obj) == nil {
			ret = append(ret, obj)
		}
	}
	return ops.NewPoolListOK().WithPayload(ret)
}

func (c *HandlerComp) poolUpdate(params ops.PoolUpdateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewPoolUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var uP = [centrald.NumActionTypes][]string{
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    params.Set,
	}
	nMap := c.poolMutableNameMap()
	ua, err := c.makeStdUpdateArgs(nMap, params.ID, params.Version, uP)
	if err != nil {
		return ops.NewPoolUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err = centrald.ErrorUpdateInvalidRequest
		return ops.NewPoolUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.RLock()
	defer c.RUnlock()
	oObj, err := c.DS.OpsPool().Fetch(ctx, params.ID)
	if err != nil {
		return ops.NewPoolUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.Version == 0 {
		ua.Version = int32(oObj.Meta.Version)
	} else if int32(oObj.Meta.Version) != ua.Version {
		err = centrald.ErrorIDVerNotFound
		return ops.NewPoolUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	// attributes are only modified internally
	if err = ai.InternalOK(); err != nil {
		return ops.NewPoolUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.DS.OpsPool().Update(ctx, ua, params.Payload)
	if err != nil {
		return ops.NewPoolUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.setScopePool(params.HTTPRequest, obj)
	return ops.NewPoolUpdateOK().WithPayload(obj)
}

func (c *HandlerComp) setScopePool(r *http.Request, obj *M.Pool) {
	m := crude.ScopeMap{
		"cspDomainID":         string(obj.CspDomainID),
		"clusterId":           string(obj.ClusterID),
		"authorizedAccountId": string(obj.AuthorizedAccountID),
	}
	c.addNewObjIDToScopeMap(obj, m)
	c.app.CrudeOps.SetScope(r, m, obj)
}
