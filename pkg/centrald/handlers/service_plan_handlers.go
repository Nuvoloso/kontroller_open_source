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

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/service_plan"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
)

// Register handlers for ServicePlan
func (c *HandlerComp) servicePlanRegisterHandlers() {
	c.app.API.ServicePlanServicePlanCloneHandler = ops.ServicePlanCloneHandlerFunc(c.servicePlanClone)
	c.app.API.ServicePlanServicePlanDeleteHandler = ops.ServicePlanDeleteHandlerFunc(c.servicePlanDelete)
	c.app.API.ServicePlanServicePlanFetchHandler = ops.ServicePlanFetchHandlerFunc(c.servicePlanFetch)
	c.app.API.ServicePlanServicePlanListHandler = ops.ServicePlanListHandlerFunc(c.servicePlanList)
	c.app.API.ServicePlanServicePlanPublishHandler = ops.ServicePlanPublishHandlerFunc(c.servicePlanPublish)
	c.app.API.ServicePlanServicePlanRetireHandler = ops.ServicePlanRetireHandlerFunc(c.servicePlanRetire)
	c.app.API.ServicePlanServicePlanUpdateHandler = ops.ServicePlanUpdateHandlerFunc(c.servicePlanUpdate)
}

var nmServicePlanMutable JSONToAttrNameMap

func (c *HandlerComp) servicePlanMutableNameMap() JSONToAttrNameMap {
	if nmServicePlanMutable == nil {
		c.cacheMux.Lock()
		defer c.cacheMux.Unlock()
		if nmServicePlanMutable == nil {
			nmServicePlanMutable = c.makeJSONToAttrNameMap(models.ServicePlanMutable{})
		}
	}
	return nmServicePlanMutable
}

// servicePlanFetchFilter verifies the caller has permission to fetch the object, returning an error if they do not
func (c *HandlerComp) servicePlanFetchFilter(ai *auth.Info, obj *models.ServicePlan) error {
	err := ai.CapOK(centrald.SystemManagementCap)
	if err != nil {
		err = ai.CapOK(centrald.CSPDomainManagementCap)
	}
	if err != nil && len(obj.Accounts) > 0 {
		err = ai.CapOK(centrald.CSPDomainUsageCap, obj.Accounts...)
	}
	return err
}

// Handlers

func (c *HandlerComp) servicePlanClone(params ops.ServicePlanCloneParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	if err := c.app.AuditLog.Ready(); err != nil {
		return ops.NewServicePlanCloneDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	// clone is not currently supported, restrict to internal users only
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err == nil {
		err = ai.InternalOK()
		if err != nil {
			c.app.AuditLog.Post(ctx, ai, centrald.ServicePlanCloneAction, models.ObjID(params.ID), models.ObjName(params.Payload.Name), "", true, "Clone unauthorized")
		}
	}
	if err != nil {
		return ops.NewServicePlanCloneDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err := centrald.ErrorMissing
		return ops.NewServicePlanCloneDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload.Name == "" {
		err := c.eMissingMsg("non-empty name is required")
		return ops.NewServicePlanCloneDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.DS.OpsServicePlan().Clone(ctx, params)
	if err != nil {
		return ops.NewServicePlanCloneDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.app.AuditLog.Post(ctx, ai, centrald.ServicePlanCloneAction, obj.Meta.ID, models.ObjName(obj.Name), "", false, "Created")
	c.Log.Infof("ServicePlan %s created [%s]", obj.Name, obj.Meta.ID)
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	return ops.NewServicePlanCloneCreated().WithPayload(obj)
}

func (c *HandlerComp) servicePlanDelete(params ops.ServicePlanDeleteParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewServicePlanDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var obj *models.ServicePlan
	if obj, err = c.DS.OpsServicePlan().Fetch(ctx, params.ID); err == nil {
		if name, builtIn := c.DS.OpsServicePlan().BuiltInPlan(params.ID); builtIn {
			c.Log.Warningf("Attempt to delete built-in service plan '%s'", name)
			err = centrald.ErrorUnauthorizedOrForbidden
		} else if err = c.app.AuditLog.Ready(); err == nil {
			c.Lock()
			defer c.Unlock()
			// Clone and Delete are not currently supported, restrict to internal users only
			if err = ai.InternalOK(); err == nil {
				vrlParams := volume_series_request.VolumeSeriesRequestListParams{ServicePlanID: &params.ID, IsTerminated: swag.Bool(false)}
				var n int
				if n, err = c.DS.OpsServicePlan().Count(ctx, ops.ServicePlanListParams{SourceServicePlanID: &params.ID}, 1); err == nil {
					if n != 0 {
						err = &centrald.Error{M: "service plan is the source of one or more service plans", C: centrald.ErrorExists.C}
					} else if n, err = c.DS.OpsVolumeSeries().Count(ctx, volume_series.VolumeSeriesListParams{ServicePlanID: &params.ID}, 1); err == nil {
						if n != 0 {
							err = &centrald.Error{M: "service plan is in use by one or more volume series", C: centrald.ErrorExists.C}
						} else if n, err = c.DS.OpsVolumeSeriesRequest().Count(ctx, vrlParams, 1); err == nil {
							if n != 0 {
								err = &centrald.Error{M: "service plan is associated with active volume series requests", C: centrald.ErrorExists.C}
							} else if err = c.DS.OpsServicePlan().Delete(ctx, params.ID); err == nil {
								c.app.AuditLog.Post(ctx, ai, centrald.ServicePlanDeleteAction, obj.Meta.ID, models.ObjName(obj.Name), "", false, "Deleted")
								c.Log.Infof("ServicePlan %s deleted [%s]", obj.Name, obj.Meta.ID)
								c.setDefaultObjectScope(params.HTTPRequest, obj)
								return ops.NewServicePlanDeleteNoContent()
							}
						}
					}
				}
			} else {
				c.app.AuditLog.Post(ctx, ai, centrald.ServicePlanDeleteAction, obj.Meta.ID, models.ObjName(obj.Name), "", true, "Delete unauthorized")
			}
		}
	}
	return ops.NewServicePlanDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
}

func (c *HandlerComp) servicePlanFetch(params ops.ServicePlanFetchParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewServicePlanFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.intServicePlanFetch(ctx, ai, params.ID)
	if err != nil {
		return ops.NewServicePlanFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewServicePlanFetchOK().WithPayload(obj)
}

func (c *HandlerComp) intServicePlanFetch(ctx context.Context, ai *auth.Info, id string) (*models.ServicePlan, error) {
	obj, err := c.DS.OpsServicePlan().Fetch(ctx, id)
	if err == nil {
		err = c.servicePlanFetchFilter(ai, obj)
	}
	return obj, err
}

func (c *HandlerComp) servicePlanList(params ops.ServicePlanListParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewServicePlanListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	list, err := c.DS.OpsServicePlan().List(ctx, params)
	if err != nil {
		return ops.NewServicePlanListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	ret := make([]*models.ServicePlan, 0, len(list))
	for _, obj := range list {
		if c.servicePlanFetchFilter(ai, obj) == nil {
			ret = append(ret, obj)
		}
	}
	return ops.NewServicePlanListOK().WithPayload(ret)
}

func (c *HandlerComp) servicePlanPublish(params ops.ServicePlanPublishParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	// All service plans are currently published, additional RBAC not currently needed, TBD
	obj, err := c.DS.OpsServicePlan().Publish(ctx, params)
	if err != nil {
		return ops.NewServicePlanPublishDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	return ops.NewServicePlanPublishOK().WithPayload(obj)
}

func (c *HandlerComp) servicePlanRetire(params ops.ServicePlanRetireParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	// All service plans are currently built-in, additional RBAC not currently needed, TBD
	var obj *models.ServicePlan
	var err error
	if name, builtIn := c.DS.OpsServicePlan().BuiltInPlan(params.ID); builtIn {
		c.Log.Warning("Attempt to retire built-in service plan '%s'", name)
		err = centrald.ErrorUnauthorizedOrForbidden
	} else {
		obj, err = c.DS.OpsServicePlan().Retire(ctx, params)
	}
	if err != nil {
		return ops.NewServicePlanRetireDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	return ops.NewServicePlanRetireOK().WithPayload(obj)
}

func (c *HandlerComp) servicePlanUpdate(params ops.ServicePlanUpdateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewServicePlanUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var uP = [centrald.NumActionTypes][]string{
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    params.Set,
	}
	ua, err := c.makeStdUpdateArgs(c.servicePlanMutableNameMap(), params.ID, params.Version, uP)
	if err != nil {
		return ops.NewServicePlanUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err = centrald.ErrorUpdateInvalidRequest
		return ops.NewServicePlanUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.IsModified("Name") && params.Payload.Name == "" {
		err = c.eUpdateInvalidMsg("non-empty name is required")
		return ops.NewServicePlanUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	_, builtIn := c.DS.OpsServicePlan().BuiltInPlan(params.ID)
	if builtIn && (ua.IsModified("Name") || ua.IsModified("Description")) {
		// TBD disallow modifying tags after "cost" tag is moved to SPA
		err = c.eUpdateInvalidMsg("only accounts or tags of built-in service plans can be modified")
		return ops.NewServicePlanUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	// fetch the object to determine the current state and for logging
	obj, err := c.DS.OpsServicePlan().Fetch(ctx, params.ID)
	if err != nil {
		return ops.NewServicePlanUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	} else if ua.Version == 0 {
		ua.Version = int32(obj.Meta.Version)
	} else if int32(obj.Meta.Version) != ua.Version {
		err = centrald.ErrorIDVerNotFound
		return ops.NewServicePlanUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err = c.app.AuditLog.Ready(); err != nil {
		return ops.NewServicePlanUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if a := ua.FindUpdateAttr("Slos"); a != nil && a.IsModified() {
		if util.Contains(params.Remove, "slos") {
			err = c.eUpdateInvalidMsg("cannot remove slos")
			return ops.NewServicePlanUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if obj.State != centrald.UnpublishedState {
			err = c.eUpdateInvalidMsg("slos can only be modified in %s state", centrald.UnpublishedState)
			return ops.NewServicePlanUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		// Until Clone is supported, restrict to internal user only
		if err = ai.InternalOK(); err != nil {
			c.app.AuditLog.Post(ctx, ai, centrald.ServicePlanUpdateAction, models.ObjID(params.ID), models.ObjName(obj.Name), "", true, "Update unauthorized")
			return ops.NewServicePlanUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		// verify that all slos names are valid, and that kind and immutable are not being modified
		if a.Actions[centrald.UpdateSet].FromBody && len(params.Payload.Slos) != len(obj.Slos) {
			err = c.eUpdateInvalidMsg("cannot add or remove slos")
			return ops.NewServicePlanUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		action := centrald.UpdateSet
		if a.Actions[centrald.UpdateAppend].FromBody {
			action = centrald.UpdateAppend
		}
		for k, v := range params.Payload.Slos {
			needCheck := a.Actions[action].FromBody
			if !needCheck {
				_, needCheck = a.Actions[action].Fields[k]
			}
			if needCheck {
				if _, ok := obj.Slos[k]; !ok {
					err = c.eUpdateInvalidMsg("invalid slo name '%s'", k)
					return ops.NewServicePlanUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
				}
				if obj.Slos[k].Immutable {
					err = c.eUpdateInvalidMsg("slo '%s' is immutable in this service plan", k)
					return ops.NewServicePlanUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
				}
				if v.Immutable || obj.Slos[k].Kind != v.Kind {
					err = c.eUpdateInvalidMsg("only value of slo '%s' can be updated", k)
					return ops.NewServicePlanUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
				}
			}
		}
		// TBD validate SLO value modifications
	}
	detail := ""
	if a := ua.FindUpdateAttr("Accounts"); a != nil && a.IsModified() {
		c.RLock()
		defer c.RUnlock()
		detail, err = c.authAccountValidator.validateAuthorizedAccountsUpdate(ctx, ai, centrald.ServicePlanUpdateAction, params.ID, models.ObjName(obj.Name), a, obj.Accounts, params.Payload.Accounts)
		if err != nil {
			return ops.NewServicePlanUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if obj, err = c.DS.OpsServicePlan().Update(ctx, ua, params.Payload); err != nil {
		return ops.NewServicePlanUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if len(detail) > 0 {
		c.app.AuditLog.Post(ctx, ai, centrald.ServicePlanUpdateAction, models.ObjID(params.ID), models.ObjName(obj.Name), "", false, fmt.Sprintf("Updated accounts %s", detail))
	}
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	return ops.NewServicePlanUpdateOK().WithPayload(obj)
}
