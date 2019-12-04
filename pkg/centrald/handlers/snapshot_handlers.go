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
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/snapshot"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/pstore"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
)

// register handlers for Snapshot
func (c *HandlerComp) snapshotRegisterHandlers() {
	c.app.API.SnapshotSnapshotCreateHandler = ops.SnapshotCreateHandlerFunc(c.snapshotCreate)
	c.app.API.SnapshotSnapshotDeleteHandler = ops.SnapshotDeleteHandlerFunc(c.snapshotDelete)
	c.app.API.SnapshotSnapshotFetchHandler = ops.SnapshotFetchHandlerFunc(c.snapshotFetch)
	c.app.API.SnapshotSnapshotListHandler = ops.SnapshotListHandlerFunc(c.snapshotList)
	c.app.API.SnapshotSnapshotUpdateHandler = ops.SnapshotUpdateHandlerFunc(c.snapshotUpdate)
}

var nmSnapshotMutable JSONToAttrNameMap

func (c *HandlerComp) snapshotMutableNameMap() JSONToAttrNameMap {
	if nmSnapshotMutable == nil {
		c.cacheMux.Lock()
		defer c.cacheMux.Unlock()
		if nmSnapshotMutable == nil {
			nmSnapshotMutable = c.makeJSONToAttrNameMap(M.Snapshot{})
		}
	}
	return nmSnapshotMutable
}

// snapshotFetchFilter verifies the caller has permission to fetch the object, returning an error if they do not.
// If they do, the properties of the object are filtered based on capabilities.
func (c *HandlerComp) snapshotFetchFilter(ai *auth.Info, obj *M.Snapshot) error {
	err := ai.CapOK(centrald.VolumeSeriesOwnerCap, M.ObjIDMutable(obj.AccountID))
	if err != nil {
		err = ai.CapOK(centrald.VolumeSeriesFetchCap, M.ObjIDMutable(obj.TenantAccountID))
	}
	return err
}

// check if original list is fully contained in the target list, e.g. all of the original elements
// are present in the target list
func (c *HandlerComp) containsLocations(originalLoc, targetLoc map[string]M.SnapshotLocation) bool {
	originalLocNum := len(originalLoc)
	var cnt int
	for cspDom := range originalLoc {
		if _, ok := targetLoc[cspDom]; ok {
			cnt++
		}
	}
	if cnt != originalLocNum {
		return false
	}
	return true
}

// validateLocations is a helper to validate all CspDomainIDs in case of set or append,
// also to make sure at least one location remains in case of remove.
// Caller must hold c.RLock
func (c *HandlerComp) validateLocations(ctx context.Context, ai *auth.Info, attr *centrald.UpdateAttr, cObj *M.Snapshot, uObj *M.SnapshotMutable) ([]*M.CSPDomain, error) {
	locations := cObj.Locations
	action := centrald.UpdateActionArgs{FromBody: true}
	if uObj != nil {
		if attr.Actions[centrald.UpdateRemove].FromBody {
			// It's OK if the REMOVE list contains elements that are not in the original list
			if c.containsLocations(cObj.Locations, uObj.Locations) {
				err := c.eUpdateInvalidMsg("on remove there should be at least one location left")
				return nil, err
			}
		}
		locations = uObj.Locations
		action = attr.Actions[centrald.UpdateAppend]
		if !action.FromBody {
			action = attr.Actions[centrald.UpdateSet]
		}
	}
	ret := []*M.CSPDomain{}
	for k, v := range locations {
		if _, exists := action.Fields[k]; action.FromBody || exists {
			if k != string(v.CspDomainID) { // key CspDomainID should match the one in SnapshotLocation obj
				err := c.eMissingMsg("invalid cspDomainId")
				return nil, err
			}
			cspObj, err := c.ops.intCspDomainFetch(ctx, ai, k)
			if err != nil {
				if err == centrald.ErrorNotFound {
					err = c.eMissingMsg("invalid cspDomainId")
				}
				return nil, err
			} else if !util.Contains(cspObj.AuthorizedAccounts, M.ObjIDMutable(cObj.AccountID)) {
				return nil, c.eUnauthorizedOrForbidden("CSP domain is not available to the account")
			}
			ret = append(ret, cspObj)
		}
	}
	return ret, nil
}

func (c *HandlerComp) snapshotCreate(params ops.SnapshotCreateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewSnapshotCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil || params.Payload.PitIdentifier == "" || params.Payload.SnapIdentifier == "" || params.Payload.VolumeSeriesID == "" || len(params.Payload.Locations) == 0 || params.Payload.ProtectionDomainID == "" {
		err = c.eMissingMsg("PitIdentifier, SnapIdentifier, VolumeSeriesID, Locations and ProtectionDomainID are required")
		return ops.NewSnapshotCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	// validate DeleteAfterTime is in the future
	if !time.Time(params.Payload.DeleteAfterTime).IsZero() && !time.Time(params.Payload.DeleteAfterTime).After(time.Now()) {
		err = c.eMissingMsg("DeleteAfterTime is in the past")
		return ops.NewSnapshotCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.RLock()
	defer c.RUnlock()
	// validate VolumeSeries
	vsObj, err := c.DS.OpsVolumeSeries().Fetch(ctx, string(params.Payload.VolumeSeriesID))
	if err != nil {
		if err == centrald.ErrorNotFound {
			err = c.eMissingMsg("invalid VolumeSeriesID")
		}
		return ops.NewSnapshotCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err = ai.CapOK(centrald.VolumeSeriesOwnerCap, vsObj.AccountID); err != nil {
		return ops.NewSnapshotCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	snapPdObj, err := c.ops.intProtectionDomainFetch(ctx, ai, string(params.Payload.ProtectionDomainID))
	if err != nil {
		if err == centrald.ErrorNotFound {
			err = c.eMissingMsg("invalid protectionDomainId")
		}
		return ops.NewSnapshotCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	params.Payload.ConsistencyGroupID = M.ObjID(vsObj.ConsistencyGroupID)
	params.Payload.AccountID = M.ObjID(vsObj.AccountID)
	params.Payload.TenantAccountID = M.ObjID(vsObj.TenantAccountID)
	params.Payload.SizeBytes = swag.Int64Value(vsObj.SizeBytes)
	var locDomains []*M.CSPDomain
	if locDomains, err = c.validateLocations(ctx, ai, nil, params.Payload, nil); err != nil {
		return ops.NewSnapshotCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	// push the metadata to the snapshot catalog first, so fetch the cast of supporting objects ...
	var aObj, taObj *M.Account
	var cgObj *M.ConsistencyGroup
	var catPS *M.CSPDomain
	var catPD *M.ProtectionDomain
	srcObj := "account"
	if aObj, err = c.ops.intAccountFetch(ctx, ai, string(params.Payload.AccountID)); err == nil {
		if aObj.SnapshotCatalogPolicy == nil || aObj.SnapshotCatalogPolicy.CspDomainID == "" || aObj.SnapshotCatalogPolicy.ProtectionDomainID == "" {
			err = c.eMissingMsg("invalid snapshotCatalogPolicy")
			return ops.NewSnapshotCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
		} else if aObj.TenantAccountID != "" {
			srcObj = "tenant account"
			taObj, err = c.ops.intAccountFetch(ctx, ai, string(aObj.TenantAccountID))
		}
		if err == nil {
			srcObj = "consistency group"
			if cgObj, err = c.ops.intConsistencyGroupFetch(ctx, ai, string(params.Payload.ConsistencyGroupID), aObj); err == nil {
				aiInt := &auth.Info{} // internal call to get snapshot catalog protection store access
				srcObj = "catalog CSP domain"
				if catPS, err = c.ops.intCspDomainFetch(ctx, aiInt, string(aObj.SnapshotCatalogPolicy.CspDomainID)); err == nil {
					srcObj = "catalog protection domain"
					catPD, err = c.ops.intProtectionDomainFetch(ctx, aiInt, string(aObj.SnapshotCatalogPolicy.ProtectionDomainID))
				}
			}
		}
	}
	if err != nil {
		err = c.eInternalError("load %s object: %s", srcObj, err.Error())
		return ops.NewSnapshotCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	srcObjs := &pstore.SnapshotCatalogUpsertArgsSourceObjects{
		EntryObjects: pstore.SnapshotCatalogEntrySourceObjects{
			Snapshot:         params.Payload,
			Account:          aObj,
			TenantAccount:    taObj,
			VolumeSeries:     vsObj,
			ProtectionDomain: snapPdObj,
			ProtectionStores: locDomains,
			ConsistencyGroup: cgObj,
		},
		ProtectionStore:  catPS,
		ProtectionDomain: catPD,
	}
	scUA := &pstore.SnapshotCatalogUpsertArgs{}
	scUA.Initialize(srcObjs)
	c.Log.Debugf("PSO: SnapshotCatalogUpsert(%v, d:%s pd:%s)", scUA.Entry, catPS.Meta.ID, catPD.Meta.ID)
	if _, err = c.app.PSO.SnapshotCatalogUpsert(ctx, scUA); err != nil {
		c.Log.Errorf("PSO: SnapshotCatalogUpsert(%v, d:%s pd:%s): %s", scUA.Entry, catPS.Meta.ID, catPD.Meta.ID, err.Error())
		err = c.eInternalError("snapshot catalog upsert: %s", err.Error())
		return ops.NewSnapshotCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	// now create the snapshot
	obj, err := c.DS.OpsSnapshot().Create(ctx, params.Payload)
	if err != nil {
		return ops.NewSnapshotCreateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.Log.Infof("Snapshot %s for VS %s, CG %s and PitUUID %s created [id: %s]",
		obj.SnapIdentifier, obj.VolumeSeriesID, params.Payload.ConsistencyGroupID, obj.PitIdentifier, obj.Meta.ID)
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	return ops.NewSnapshotCreateCreated().WithPayload(obj)
}

func (c *HandlerComp) snapshotDelete(params ops.SnapshotDeleteParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewSnapshotDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var obj *M.Snapshot
	if obj, err = c.DS.OpsSnapshot().Fetch(ctx, params.ID); err != nil {
		return ops.NewSnapshotDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.Lock()
	defer c.Unlock()
	if err = ai.CapOK(centrald.VolumeSeriesOwnerCap, M.ObjIDMutable(obj.AccountID)); err != nil {
		return ops.NewSnapshotDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	// check if snapshot is still mounted -> delete is not allowed
	vsObj, err := c.DS.OpsVolumeSeries().Fetch(ctx, string(obj.VolumeSeriesID))
	if err != nil {
		if err == centrald.ErrorNotFound {
			err = c.eMissingMsg("invalid VolumeSeriesID")
		}
		return ops.NewSnapshotDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	for _, vsMount := range vsObj.Mounts {
		if vsMount.SnapIdentifier == obj.SnapIdentifier {
			err = &centrald.Error{M: "Snapshot is still mounted", C: centrald.ErrorUnauthorizedOrForbidden.C}
			return ops.NewSnapshotDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	// check that there is no active VSR referencing the snapshot
	var n int
	if n, err = c.DS.OpsVolumeSeriesRequest().Count(ctx, volume_series_request.VolumeSeriesRequestListParams{SnapshotID: swag.String(params.ID), IsTerminated: swag.Bool(false)}, 1); err == nil && n != 0 {
		err = c.eRequestInConflict("snapshot in use")
		return ops.NewSnapshotDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err = c.DS.OpsSnapshot().Delete(ctx, params.ID); err != nil {
		return ops.NewSnapshotDeleteDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	c.Log.Infof("Snapshot deleted [%s]", obj.Meta.ID)
	c.setDefaultObjectScope(params.HTTPRequest, obj)
	return ops.NewSnapshotDeleteNoContent()
}

func (c *HandlerComp) snapshotFetch(params ops.SnapshotFetchParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewSnapshotFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var obj *M.Snapshot
	obj, err = c.intSnapshotFetch(ctx, ai, params.ID)
	if err != nil {
		return ops.NewSnapshotFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewSnapshotFetchOK().WithPayload(obj)
}

// intSnapshotFetch is the internal shareable part of snapshotFetch; protect appropriately!
func (c *HandlerComp) intSnapshotFetch(ctx context.Context, ai *auth.Info, id string) (*M.Snapshot, error) {
	obj, err := c.DS.OpsSnapshot().Fetch(ctx, id)
	if err == nil {
		err = c.snapshotFetchFilter(ai, obj)
	}
	return obj, err
}

// intFetchSnapshotData fetches a Snapshot and returns its SnapshotData representation; protect appropriately!
func (c *HandlerComp) intSnapshotFetchData(ctx context.Context, ai *auth.Info, id string) (*M.SnapshotData, error) {
	snapObj, err := c.intSnapshotFetch(ctx, ai, id)
	if err != nil {
		return nil, err
	}
	snap := &M.SnapshotData{}
	snap.ConsistencyGroupID = M.ObjIDMutable(snapObj.ConsistencyGroupID)
	snap.DeleteAfterTime = snapObj.DeleteAfterTime
	snap.PitIdentifier = snapObj.PitIdentifier
	snap.ProtectionDomainID = snapObj.ProtectionDomainID
	snap.SizeBytes = swag.Int64(snapObj.SizeBytes)
	snap.SnapIdentifier = snapObj.SnapIdentifier
	snap.SnapTime = snapObj.SnapTime
	snap.VolumeSeriesID = snapObj.VolumeSeriesID
	snap.Locations = []*models.SnapshotLocation{}
	for _, loc := range snapObj.Locations {
		snap.Locations = append(snap.Locations, &loc)
	}
	return snap, nil
}

func (c *HandlerComp) snapshotList(params ops.SnapshotListParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewSnapshotListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var list []*M.Snapshot
	var count int
	// rather than querying for the entire list and performing RBAC filtering in the handler, constrain the query which should be more efficient
	params.AccountID, params.TenantAccountID, err = c.ops.constrainEitherOrQueryAccounts(ctx, ai, params.AccountID, centrald.VolumeSeriesOwnerCap, params.TenantAccountID, centrald.VolumeSeriesFetchCap)

	// if both AccountID and TenantAccountID are now nil and the caller is not internal, all accounts are filtered out, skip the query
	if err == nil && (params.AccountID != nil || params.TenantAccountID != nil || ai.Internal()) {
		if swag.BoolValue(params.CountOnly) {
			count, err = c.DS.OpsSnapshot().Count(ctx, params, 0)
		} else {
			err = c.verifySortKeys(c.snapshotMutableNameMap(), params.SortAsc, params.SortDesc)
			if err == nil {
				list, err = c.DS.OpsSnapshot().List(ctx, params)
				count = len(list)
			}
		}
	}
	if err != nil {
		return ops.NewSnapshotListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	// from doc: Suppresses the response body in a list operation. The number of items that would be returned can be determined from the Total-Count in the response header.
	if swag.BoolValue(params.CountOnly) {
		return ops.NewSnapshotListOK().WithTotalCount(int64(count))
	}
	return ops.NewSnapshotListOK().WithPayload(list).WithTotalCount(int64(count))
}

func (c *HandlerComp) snapshotUpdate(params ops.SnapshotUpdateParams) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewSnapshotUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err = centrald.ErrorUpdateInvalidRequest
		return ops.NewSnapshotUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var uP = [centrald.NumActionTypes][]string{
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    params.Set,
	}
	ua, err := c.makeStdUpdateArgs(c.snapshotMutableNameMap(), params.ID, params.Version, uP)
	if err != nil {
		return ops.NewSnapshotUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.IsModified("DeleteAfterTime") {
		if !time.Time(params.Payload.DeleteAfterTime).IsZero() && !time.Time(params.Payload.DeleteAfterTime).After(time.Now()) {
			err = c.eMissingMsg("DeleteAfterTime is in the past")
		} else {
			err = ai.InternalOK()
		}
		if err != nil {
			return ops.NewSnapshotUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	c.RLock()
	defer c.RUnlock()
	oObj, err := c.DS.OpsSnapshot().Fetch(ctx, params.ID)
	if err != nil {
		return ops.NewSnapshotUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	} else if ua.Version == 0 {
		ua.Version = int32(oObj.Meta.Version)
	} else if int32(oObj.Meta.Version) != ua.Version {
		err = centrald.ErrorIDVerNotFound
		return ops.NewSnapshotUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if ua.IsModified("Tags") {
		if err = ai.CapOK(centrald.VolumeSeriesOwnerCap, M.ObjIDMutable(oObj.AccountID)); err != nil {
			return ops.NewSnapshotUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	if ua.IsModified("Locations") {
		if err = ai.InternalOK(); err != nil {
			return ops.NewSnapshotUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
		if _, err = c.validateLocations(ctx, ai, ua.FindUpdateAttr("Locations"), oObj, params.Payload); err != nil {
			return ops.NewSnapshotUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
		}
	}
	obj, err := c.DS.OpsSnapshot().Update(ctx, ua, params.Payload)
	if err != nil {
		return ops.NewSnapshotUpdateDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewSnapshotUpdateOK().WithPayload(obj)
}
