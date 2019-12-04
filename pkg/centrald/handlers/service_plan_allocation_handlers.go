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
	"strconv"
	"strings"

	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/pool"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/service_plan_allocation"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
)

// Register handlers for ServicePlanAllocation
func (c *HandlerComp) servicePlanAllocationRegisterHandlers() {
	c.app.API.ServicePlanAllocationServicePlanAllocationCreateHandler = ops.ServicePlanAllocationCreateHandlerFunc(c.servicePlanAllocationCreate)
	c.app.API.ServicePlanAllocationServicePlanAllocationDeleteHandler = ops.ServicePlanAllocationDeleteHandlerFunc(c.servicePlanAllocationDelete)
	c.app.API.ServicePlanAllocationServicePlanAllocationCustomizeProvisioningHandler = ops.ServicePlanAllocationCustomizeProvisioningHandlerFunc(c.servicePlanAllocationCustomizeProvisioning)
	c.app.API.ServicePlanAllocationServicePlanAllocationFetchHandler = ops.ServicePlanAllocationFetchHandlerFunc(c.servicePlanAllocationFetch)
	c.app.API.ServicePlanAllocationServicePlanAllocationListHandler = ops.ServicePlanAllocationListHandlerFunc(c.servicePlanAllocationList)
	c.app.API.ServicePlanAllocationServicePlanAllocationUpdateHandler = ops.ServicePlanAllocationUpdateHandlerFunc(c.servicePlanAllocationUpdate)
}

var nmServicePlanAllocationMutable JSONToAttrNameMap

func (c *HandlerComp) servicePlanAllocationMutableNameMap() JSONToAttrNameMap {
	if nmServicePlanAllocationMutable == nil {
		c.cacheMux.Lock()
		defer c.cacheMux.Unlock()
		if nmServicePlanAllocationMutable == nil {
			nmServicePlanAllocationMutable = c.makeJSONToAttrNameMap(M.ServicePlanAllocationMutable{})
		}
	}
	return nmServicePlanAllocationMutable
}

// servicePlanAllocationFetchFilter verifies the caller has permission to fetch the object, returning an error if they do not
func (c *HandlerComp) servicePlanAllocationFetchFilter(ai *auth.Info, obj *M.ServicePlanAllocation) error {
	err := ai.CapOK(centrald.CSPDomainManagementCap, obj.AccountID)
	if err != nil {
		err = ai.CapOK(centrald.CSPDomainUsageCap, obj.AuthorizedAccountID)
	}
	return err
}

// servicePlanAllocationPseudoName generates a pseudo name for the SPA based on the current authorized account, service plan and cluster names
func (c *HandlerComp) servicePlanAllocationPseudoName(ctx context.Context, ai *auth.Info, obj *M.ServicePlanAllocation) (M.ObjName, error) {
	accountName := ai.AccountName
	if accountName == "" || ai.AccountID != string(obj.AuthorizedAccountID) {
		authAccountObj, err := c.DS.OpsAccount().Fetch(ctx, string(obj.AuthorizedAccountID))
		if err != nil {
			return "", err
		}
		accountName = string(authAccountObj.Name)
	}
	spObj, err := c.DS.OpsServicePlan().Fetch(ctx, string(obj.ServicePlanID))
	if err != nil {
		return "", err
	}
	clObj, err := c.DS.OpsCluster().Fetch(ctx, string(obj.ClusterID))
	if err != nil {
		return "", err
	}
	return M.ObjName(fmt.Sprintf("%s/%s/%s", accountName, spObj.Name, clObj.Name)), nil

}

// servicePlanAllocationValidateSizes is a helper to validate *Bytes constraints
// On Update (non-nil ua and uObj)
// Note: negative values are disallowed by the swagger autogen code
func (c *HandlerComp) servicePlanAllocationValidateSizes(ctx context.Context, ua *centrald.UpdateArgs, obj *M.ServicePlanAllocation, uObj *M.ServicePlanAllocationMutable) error {
	reservableCapacityBytes := swag.Int64Value(obj.ReservableCapacityBytes)
	totalCapacityBytes := swag.Int64Value(obj.TotalCapacityBytes)
	if ua != nil && ua.IsModified("ReservableCapacityBytes") {
		reservableCapacityBytes = swag.Int64Value(uObj.ReservableCapacityBytes)
	}
	if ua != nil && ua.IsModified("TotalCapacityBytes") {
		totalCapacityBytes = swag.Int64Value(uObj.TotalCapacityBytes)
		// reservableCapacityBytes is auto-updated: make sure it does not go negative (normally swagger does non-negative the check)
		if reservableCapacityBytes < 0 {
			return fmt.Errorf("change to totalCapacityBytes would cause reservableCapacityBytes to be negative")
		}
	}
	if reservableCapacityBytes > totalCapacityBytes {
		return fmt.Errorf("reservableCapacityBytes must not be greater than totalCapacityBytes")
	}
	if ua != nil {
		vParams := volume_series.VolumeSeriesListParams{
			ServicePlanAllocationID: swag.String(string(obj.Meta.ID)),
			VolumeSeriesStateNot:    []string{common.VolStateDeleting},
			Sum:                     []string{fmt.Sprintf("sizeBytes")},
		}
		sums, _, err := c.DS.OpsVolumeSeries().Aggregate(ctx, vParams)
		if err != nil {
			return err
		}
		amountBytes := sums[0].Value
		if reservableCapacityBytes+amountBytes > totalCapacityBytes {
			return fmt.Errorf("reservableCapacityBytes plus the size(s) of reserved storage exceeds totalCapacityBytes")
		}
	}
	return nil
}

// SpaValidator interface
type SpaValidator interface {
	servicePlanAllocationCreateValidate(context.Context, *auth.Info, *M.ServicePlanAllocation) (*M.Account, *M.ServicePlan, *M.Cluster, error)
}

// servicePlanAllocationCreateValidate validates the object in the context of a create operation.
// It can also be called to validate the VSR creation arguments.
// Lock must be held across this method.
// Returns the following objects that were fetched during validation (Account, ServicePlan, Cluster, error).
// On ErrorUnauthorizedOrForbidden error, the objects are also returned for use in audit logging.
func (c *HandlerComp) servicePlanAllocationCreateValidate(ctx context.Context, ai *auth.Info, obj *M.ServicePlanAllocation) (*M.Account, *M.ServicePlan, *M.Cluster, error) {
	if obj == nil {
		return nil, nil, nil, centrald.ErrorMissing
	}
	var err error
	if obj.AccountID == "" || obj.AuthorizedAccountID == "" || obj.ClusterID == "" || obj.ServicePlanID == "" {
		err = c.eMissingMsg("accountId, authorizedAccountId, clusterId and servicePlanId are required")
		return nil, nil, nil, err
	}
	err = c.servicePlanAllocationValidateSizes(ctx, nil, obj, nil)
	if err != nil {
		err = c.eMissingMsg("%s", err.Error())
		return nil, nil, nil, err
	}
	allowedReservationStates := []string{common.SPAReservationStateDisabled, common.SPAReservationStateUnknown}
	if obj.ReservationState != "" && !util.Contains(allowedReservationStates, obj.ReservationState) {
		err = c.eMissingMsg("creation reservationState must be one of %s", allowedReservationStates)
		return nil, nil, nil, err
	}
	if obj.StorageFormula != "" && !c.app.ValidateStorageFormula(obj.StorageFormula) {
		err = c.eMissingMsg("invalid storageFormula")
		return nil, nil, nil, err
	}
	accountObj, err := c.DS.OpsAccount().Fetch(ctx, string(obj.AccountID))
	if err != nil {
		if err == centrald.ErrorNotFound {
			err = c.eMissingMsg("invalid accountId")
		}
		return nil, nil, nil, err
	}
	authAccountObj := accountObj
	if obj.AccountID != obj.AuthorizedAccountID {
		if authAccountObj, err = c.DS.OpsAccount().Fetch(ctx, string(obj.AuthorizedAccountID)); err != nil {
			if err == centrald.ErrorNotFound {
				err = c.eMissingMsg("invalid authorizedAccountId")
			}
			return nil, nil, nil, err
		}
	}
	clObj, err := c.DS.OpsCluster().Fetch(ctx, string(obj.ClusterID))
	if err != nil {
		if err == centrald.ErrorNotFound {
			err = c.eMissingMsg("invalid clusterId")
		}
		return nil, nil, nil, err
	}
	// any operations are permitted only for the cluster in the MANAGED state
	if clObj.State != common.ClusterStateManaged {
		err = c.eMissingMsg("invalid Cluster state")
		return nil, nil, nil, err
	}
	spObj, err := c.DS.OpsServicePlan().Fetch(ctx, string(obj.ServicePlanID))
	if err != nil {
		if err == centrald.ErrorNotFound {
			err = c.eMissingMsg("invalid servicePlanId")
		}
		return nil, nil, nil, err
	}
	// now that all objects have been retrieved check authorizations
	if err = ai.CapOK(centrald.CSPDomainManagementCap, obj.AccountID); err != nil {
		return authAccountObj, spObj, clObj, err
	}
	// tenant admin can authorize their own account or any subordinate account
	if err = ai.CapOK(centrald.CSPDomainManagementCap, obj.AuthorizedAccountID, authAccountObj.TenantAccountID); err != nil {
		return authAccountObj, spObj, clObj, err
	}
	// no need for auth check on Service Plan, already know CSPDomainManagementCap is present and the SPA is authorizing a new account
	if obj.AccountID != M.ObjIDMutable(clObj.AccountID) { // must be same owner
		return authAccountObj, spObj, clObj, centrald.ErrorUnauthorizedOrForbidden
	}
	if obj.StorageReservations != nil {
		for poolID := range obj.StorageReservations {
			poolObj, err := c.DS.OpsPool().Fetch(ctx, string(poolID))
			if err != nil {
				if err == centrald.ErrorNotFound {
					err = c.eMissingMsg("invalid poolId %s", poolID)
				}
				return nil, nil, nil, err
			}
			if obj.AccountID != poolObj.AccountID { // must be same owner
				return authAccountObj, spObj, clObj, centrald.ErrorUnauthorizedOrForbidden
			}
			// TBD: check any additional constraints
		}
	}
	return authAccountObj, spObj, clObj, nil
}

// recomputeSPAState calculates the reservation state of the object. Should only be called after validating sizes.
func (c *HandlerComp) recomputeSPAState(newState string, reservableCapacityBytes int64) string {
	if newState != common.SPAReservationStateDisabled {
		if reservableCapacityBytes > 0 {
			return common.SPAReservationStateOk
		} else if reservableCapacityBytes == 0 {
			return common.SPAReservationStateNoCapacity
		}
	}
	return common.SPAReservationStateDisabled
}

// Handlers

func (c *HandlerComp) servicePlanAllocationCreate(params ops.ServicePlanAllocationCreateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewServicePlanAllocationCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err := centrald.ErrorMissing
		return ops.NewServicePlanAllocationCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ai.AccountID != "" {
		// always use auth AccountID as the owner when specified
		params.Payload.AccountID = M.ObjIDMutable(ai.AccountID)
	} else if ai.Internal() {
		// Assume actual account is as specified in the params, TBD tweak client
		ai.AccountID = string(params.Payload.AccountID)
	}
	obj := &M.ServicePlanAllocation{
		ServicePlanAllocationCreateOnce: params.Payload.ServicePlanAllocationCreateOnce,
		ServicePlanAllocationMutable: M.ServicePlanAllocationMutable{
			ServicePlanAllocationCreateMutable: params.Payload.ServicePlanAllocationCreateMutable,
		},
	}
	obj.TotalCapacityBytes = swag.Int64(swag.Int64Value(obj.TotalCapacityBytes)) // turns nil pointer into zero value
	obj.ReservableCapacityBytes = obj.TotalCapacityBytes
	if err = c.app.AuditLog.Ready(); err != nil {
		return ops.NewServicePlanAllocationCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.RLock()
	defer c.RUnlock()
	authAccountObj, spObj, clObj, err := c.spaValidator.servicePlanAllocationCreateValidate(ctx, ai, obj)
	var pseudoName M.ObjName
	if authAccountObj != nil && spObj != nil && clObj != nil {
		pseudoName = M.ObjName(fmt.Sprintf("%s/%s/%s", authAccountObj.Name, spObj.Name, clObj.Name))
	}
	if err != nil {
		if err == centrald.ErrorUnauthorizedOrForbidden {
			c.app.AuditLog.Post(ctx, ai, centrald.ServicePlanAllocationCreateAction, "", pseudoName, "", true, "Create unauthorized")
		}
		return ops.NewServicePlanAllocationCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj.ReservationState = c.recomputeSPAState(obj.ReservationState, swag.Int64Value(obj.ReservableCapacityBytes))
	obj.CspDomainID = clObj.CspDomainID
	if obj, err = c.DS.OpsServicePlanAllocation().Create(ctx, obj); err != nil {
		return ops.NewServicePlanAllocationCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.app.AuditLog.Post(ctx, ai, centrald.ServicePlanAllocationCreateAction, obj.Meta.ID, pseudoName, "", false, fmt.Sprintf("Created with capacity %s", util.SizeBytesToString(swag.Int64Value(obj.TotalCapacityBytes))))
	c.Log.Infof("ServicePlanAllocation created [%s]", obj.Meta.ID)
	c.setScopeServicePlanAllocation(params.HTTPRequest, obj)
	return ops.NewServicePlanAllocationCreateCreated().WithPayload(obj)
}

func (c *HandlerComp) servicePlanAllocationDelete(params ops.ServicePlanAllocationDeleteParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewServicePlanAllocationDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var obj *M.ServicePlanAllocation
	if obj, err = c.DS.OpsServicePlanAllocation().Fetch(ctx, params.ID); err == nil {
		if ai.AccountID == "" && ai.Internal() {
			// Assume actual account is as specified in the object, TBD tweak client
			ai.AccountID = string(obj.AccountID)
		}
		if err = c.app.AuditLog.Ready(); err == nil {
			c.Lock()
			defer c.Unlock()
			var pseudoName M.ObjName
			if pseudoName, err = c.servicePlanAllocationPseudoName(ctx, ai, obj); err != nil {
				return ops.NewServicePlanAllocationDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			if err = ai.CapOK(centrald.CSPDomainManagementCap, obj.AccountID); err == nil {
				vslParams := volume_series.VolumeSeriesListParams{ServicePlanAllocationID: &params.ID}
				plParams := pool.PoolListParams{ServicePlanAllocationID: &params.ID}
				vrlParams := volume_series_request.VolumeSeriesRequestListParams{ServicePlanAllocationID: &params.ID, IsTerminated: swag.Bool(false)}
				var n int
				if n, err = c.DS.OpsVolumeSeries().Count(ctx, vslParams, 1); err == nil {
					if n != 0 {
						err = &centrald.Error{M: "volume series are still associated with the service plan allocation", C: centrald.ErrorExists.C}
					} else if n, err = c.DS.OpsPool().Count(ctx, plParams, 1); err == nil {
						if n != 0 {
							err = &centrald.Error{M: "pools are still associated with the service plan allocation", C: centrald.ErrorExists.C}
						} else {
							deleteInProgress := false
							for _, tag := range obj.SystemTags {
								if strings.HasPrefix(tag, common.SystemTagVsrDeleting) {
									deleteInProgress = true
									break
								}
							}
							if !deleteInProgress {
								n, err = c.DS.OpsVolumeSeriesRequest().Count(ctx, vrlParams, 1)
							}
							if err == nil {
								if n != 0 {
									err = &centrald.Error{M: "active volume series requests are still associated with the service plan allocation", C: centrald.ErrorExists.C}
								} else if err = c.DS.OpsServicePlanAllocation().Delete(ctx, params.ID); err == nil {
									c.app.AuditLog.Post(ctx, ai, centrald.ServicePlanAllocationDeleteAction, obj.Meta.ID, pseudoName, "", false, "Deleted")
									c.Log.Infof("ServicePlanAllocation deleted [%s]", obj.Meta.ID)
									c.setScopeServicePlanAllocation(params.HTTPRequest, obj)
									return ops.NewServicePlanAllocationDeleteNoContent()
								}
							}
						}
					}
				}
			} else {
				c.app.AuditLog.Post(ctx, ai, centrald.ServicePlanAllocationDeleteAction, obj.Meta.ID, pseudoName, "", true, "Delete unauthorized")
			}
		}
	}
	return ops.NewServicePlanAllocationDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
}

func (c *HandlerComp) servicePlanAllocationFetch(params ops.ServicePlanAllocationFetchParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewServicePlanAllocationFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.intServicePlanAllocationFetch(ctx, ai, params.ID)
	if err != nil {
		return ops.NewServicePlanAllocationFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewServicePlanAllocationFetchOK().WithPayload(obj)
}

func (c *HandlerComp) intServicePlanAllocationFetch(ctx context.Context, ai *auth.Info, id string) (*M.ServicePlanAllocation, error) {
	obj, err := c.DS.OpsServicePlanAllocation().Fetch(ctx, id)
	if err == nil {
		err = c.servicePlanAllocationFetchFilter(ai, obj)
	}
	return obj, err
}

func (c *HandlerComp) servicePlanAllocationList(params ops.ServicePlanAllocationListParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewServicePlanAllocationListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var skip bool
	params.AuthorizedAccountID, params.AccountID, skip = c.ops.constrainBothQueryAccounts(ai, params.AuthorizedAccountID, centrald.CSPDomainUsageCap, params.AccountID, centrald.CSPDomainManagementCap)
	if skip {
		return ops.NewServicePlanAllocationListOK()
	}

	list, err := c.DS.OpsServicePlanAllocation().List(ctx, params)
	if err != nil {
		return ops.NewServicePlanAllocationListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewServicePlanAllocationListOK().WithPayload(list)
}

func (c *HandlerComp) servicePlanAllocationUpdate(params ops.ServicePlanAllocationUpdateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewServicePlanAllocationUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if util.Contains(params.Set, "totalCapacityBytes") {
		if util.Contains(params.Set, "reservableCapacityBytes") {
			err := c.eUpdateInvalidMsg("reservableCapacityBytes cannot be updated together with totalCapacityBytes")
			return ops.NewServicePlanAllocationUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		params.Set = append(params.Set, "reservableCapacityBytes") // auto-updated below after old object is fetched
	}
	if util.Contains(params.Set, "reservableCapacityBytes") && !util.Contains(params.Set, "reservationState") {
		params.Set = append(params.Set, "reservationState") // recompute state if reservable capacity bytes is changed
	}
	var uP = [centrald.NumActionTypes][]string{
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    params.Set,
	}
	ua, err := c.makeStdUpdateArgs(c.servicePlanAllocationMutableNameMap(), params.ID, params.Version, uP)
	if err != nil {
		return ops.NewServicePlanAllocationUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err = centrald.ErrorUpdateInvalidRequest
		return ops.NewServicePlanAllocationUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.IsModified("StorageFormula") && params.Payload.StorageFormula != "" {
		if !c.app.ValidateStorageFormula(params.Payload.StorageFormula) {
			err = c.eUpdateInvalidMsg("invalid storageFormula")
			return ops.NewServicePlanAllocationUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	c.RLock()
	defer c.RUnlock()
	oObj, err := c.DS.OpsServicePlanAllocation().Fetch(ctx, params.ID)
	if err != nil {
		return ops.NewServicePlanAllocationUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.Version == 0 {
		ua.Version = int32(oObj.Meta.Version)
	} else if int32(oObj.Meta.Version) != ua.Version {
		err = centrald.ErrorIDVerNotFound
		return ops.NewServicePlanAllocationUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err := c.app.AuditLog.Ready(); err != nil {
		return ops.NewServicePlanAllocationUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ai.AccountID == "" && ai.Internal() {
		// Assume actual account is as specified in the object, TBD tweak client
		ai.AccountID = string(oObj.AccountID)
	}
	pseudoName, err := c.servicePlanAllocationPseudoName(ctx, ai, oObj)
	if err != nil {
		return ops.NewServicePlanAllocationUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	// most fields can only be updated via VSR which has internal rights
	if ua.OthersModified("Tags") { // TBD: ProvisioningHints?
		if err = ai.InternalOK(); err != nil {
			c.app.AuditLog.Post(ctx, ai, centrald.ServicePlanAllocationUpdateAction, M.ObjID(params.ID), pseudoName, "", true, "Update unauthorized")
			return ops.NewServicePlanAllocationUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	} else if err = ai.CapOK(centrald.CSPDomainManagementCap, oObj.AccountID); err != nil {
		c.app.AuditLog.Post(ctx, ai, centrald.ServicePlanAllocationUpdateAction, M.ObjID(params.ID), pseudoName, "", true, "Update unauthorized")
		return ops.NewServicePlanAllocationUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.IsModified("ReservableCapacityBytes") || ua.IsModified("TotalCapacityBytes") || ua.IsModified("StorageReservations") || ua.IsModified("ReservationState") {
		if params.Payload.TotalCapacityBytes == nil {
			params.Payload.TotalCapacityBytes = oObj.TotalCapacityBytes
		}
		if params.Payload.ReservableCapacityBytes == nil {
			params.Payload.ReservableCapacityBytes = oObj.ReservableCapacityBytes
		}
		if params.Payload.ReservationState == "" {
			params.Payload.ReservationState = oObj.ReservationState
		}
	}
	if ua.IsModified("ReservationState") { // Check after loading params with previous state. Auto-update will cause this to be true.
		if !c.app.ValidateServicePlanAllocationReservationState(params.Payload.ReservationState) {
			err = c.eUpdateInvalidMsg("servicePlanAllocation.reservationState must be one of %s", c.app.SupportedServicePlanAllocationReservationStates())
			return ops.NewServicePlanAllocationUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	var diffCap int64
	if ua.IsModified("ReservableCapacityBytes") || ua.IsModified("TotalCapacityBytes") || ua.IsModified("ReservationState") {
		// given the pre-check at the top of the function, auto-update totalCapacityBytes changes
		if ua.IsModified("TotalCapacityBytes") {
			diffCap = swag.Int64Value(params.Payload.TotalCapacityBytes) - swag.Int64Value(oObj.TotalCapacityBytes)
			params.Payload.ReservableCapacityBytes = swag.Int64(swag.Int64Value(oObj.ReservableCapacityBytes) + diffCap)
		}
		if err = c.servicePlanAllocationValidateSizes(ctx, ua, oObj, params.Payload); err != nil {
			if _, ok := err.(*centrald.Error); !ok {
				err = c.eUpdateInvalidMsg("%s", err.Error())
			}
			return ops.NewServicePlanAllocationUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		params.Payload.ReservationState = c.recomputeSPAState(params.Payload.ReservationState, swag.Int64Value(params.Payload.ReservableCapacityBytes))
	}
	if ua.IsModified("StorageReservations") {
		for poolID := range params.Payload.StorageReservations {
			poolObj, err := c.DS.OpsPool().Fetch(ctx, string(poolID))
			if err == nil && oObj.AccountID != M.ObjIDMutable(poolObj.AccountID) { // must be same owner
				err = centrald.ErrorUnauthorizedOrForbidden
				c.app.AuditLog.Post(ctx, ai, centrald.ServicePlanAllocationUpdateAction, M.ObjID(params.ID), pseudoName, "", true, "Update unauthorized")
			}
			if err != nil {
				if err == centrald.ErrorNotFound {
					err = c.eUpdateInvalidMsg("invalid poolID %s", poolID)
				}
				return ops.NewServicePlanAllocationUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
			}
			// TBD: check any additional constraints
		}
	}
	obj, err := c.DS.OpsServicePlanAllocation().Update(ctx, ua, params.Payload)
	if err != nil {
		return ops.NewServicePlanAllocationUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if diffCap != 0 {
		c.app.AuditLog.Event(ctx, ai, centrald.ServicePlanAllocationUpdateAction, obj.Meta.ID, pseudoName, "", false, fmt.Sprintf("Updated with new capacity %s", util.SizeBytesToString(swag.Int64Value(obj.TotalCapacityBytes))))
	}
	if ua.IsModified("ChargedCostPerGiB") && obj.ChargedCostPerGiB != oObj.ChargedCostPerGiB {
		c.app.AuditLog.Post(ctx, ai, centrald.ServicePlanAllocationUpdateAction, obj.Meta.ID, pseudoName, "", false, fmt.Sprintf("Charged cost changed from [%g] to [%g]", oObj.ChargedCostPerGiB, obj.ChargedCostPerGiB))
	}
	c.setScopeServicePlanAllocation(params.HTTPRequest, obj)
	return ops.NewServicePlanAllocationUpdateOK().WithPayload(obj)
}

func (c *HandlerComp) servicePlanAllocationCustomizeProvisioning(params ops.ServicePlanAllocationCustomizeProvisioningParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewServicePlanAllocationCustomizeProvisioningDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ai.Internal() {
		err = c.eUnauthorizedOrForbidden("not for internal use")
		return ops.NewServicePlanAllocationCustomizeProvisioningDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.ApplicationGroupName == nil && params.ApplicationGroupDescription == nil && len(params.ApplicationGroupTag) == 0 && params.ConsistencyGroupName == nil && params.ConsistencyGroupDescription == nil && len(params.ConsistencyGroupTag) == 0 && len(params.VolumeSeriesTag) == 0 {
		err = c.eMissingMsg("applicationGroupDescription, applicationGroupName, applicationGroupTag, consistencyGroupDescription, consistencyGroupName, consistencyGroupTag or volumeSeriesTag required")
		return ops.NewServicePlanAllocationCustomizeProvisioningDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	obj, err := c.ops.intServicePlanAllocationFetch(ctx, ai, params.ID)
	if err != nil {
		return ops.NewServicePlanAllocationCustomizeProvisioningDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ai.GetAccountID() != string(obj.AuthorizedAccountID) {
		err = c.eUnauthorizedOrForbidden("")
		return ops.NewServicePlanAllocationCustomizeProvisioningDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	clObj, err := c.ops.intClusterFetch(ctx, ai, string(obj.ClusterID))
	if err != nil {
		return ops.NewServicePlanAllocationCustomizeProvisioningDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	cc := c.lookupClusterClient(clObj.ClusterType)
	if cc == nil {
		err = c.eInvalidData("support for clusterType '%s' missing", clObj.ClusterType)
		return ops.NewServicePlanAllocationCustomizeProvisioningDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	vt, err := c.ops.intAccountSecretRetrieve(ctx, ai, string(obj.AuthorizedAccountID), string(obj.ClusterID), clObj)
	if err != nil {
		return ops.NewServicePlanAllocationCustomizeProvisioningDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	name := common.CustomizedSecretNameDefault
	ns := ""
	if clObj.ClusterType == cluster.K8sClusterType {
		if params.K8sName != nil {
			name = swag.StringValue(params.K8sName)
		}
		if params.K8sNamespace != nil {
			ns = swag.StringValue(params.K8sNamespace)
		}
	}
	sca := &cluster.SecretCreateArgsMV{
		Intent:    cluster.SecretIntentDynamicVolumeCustomization,
		Name:      name,
		Namespace: ns,
	}
	sca.CustomizationData.AccountSecret = vt.Value
	sca.CustomizationData.ApplicationGroupName = swag.StringValue(params.ApplicationGroupName)
	sca.CustomizationData.ApplicationGroupDescription = swag.StringValue(params.ApplicationGroupDescription)
	sca.CustomizationData.ApplicationGroupTags = params.ApplicationGroupTag
	sca.CustomizationData.ConsistencyGroupName = swag.StringValue(params.ConsistencyGroupName)
	sca.CustomizationData.ConsistencyGroupDescription = swag.StringValue(params.ConsistencyGroupDescription)
	sca.CustomizationData.ConsistencyGroupTags = params.ConsistencyGroupTag
	sca.CustomizationData.VolumeTags = params.VolumeSeriesTag
	ss, err := cc.SecretFormatMV(ctx, sca)
	if err != nil {
		err = c.eInvalidData("%s", err.Error())
		return ops.NewServicePlanAllocationCustomizeProvisioningDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewServicePlanAllocationCustomizeProvisioningOK().WithPayload(&M.ValueType{Kind: common.ValueTypeSecret, Value: ss})
}

func (c *HandlerComp) setScopeServicePlanAllocation(r *http.Request, obj *M.ServicePlanAllocation) {
	m := crude.ScopeMap{
		"authorizedAccountID":     string(obj.AuthorizedAccountID),
		"clusterID":               string(obj.ClusterID),
		"cspDomainID":             string(obj.CspDomainID),
		"reservableCapacityBytes": strconv.FormatInt(swag.Int64Value(obj.ReservableCapacityBytes), 10),
		"servicePlanID":           string(obj.ServicePlanID),
		"totalCapacityBytes":      strconv.FormatInt(swag.Int64Value(obj.TotalCapacityBytes), 10),
	}
	c.addNewObjIDToScopeMap(obj, m)
	c.app.CrudeOps.SetScope(r, m, obj)
}
