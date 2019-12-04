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


package vreq

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/clusterd/state"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/layout"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/swag"
)

// vra.AllocationHandlers methods

type placeSubState int

// placeSubState values (the ordering is meaningful)
// See https://goo.gl/bvYuX4 for background.
const (
	//
	// Un-provisioned Volumes
	//

	// Start planning by entering the critical section
	PlaceStartPlanning placeSubState = iota

	// Select Storage candidates (existing or new)
	PlaceSelectStorage

	// Update the storagePlan and save it.
	// This is a terminal state if only planning. It is also a cancel point.
	PlacePopulatePlan

	// Start executing the plan by entering the critical section if not already within it.
	PlaceStartExecuting

	// Fetch all claims made by this request
	PlaceFetchClaims

	// Handle completed StorageRequests
	// Save storage plan if any StorageRequest completed. This is a cancel point.
	PlaceHandleCompletedStorageRequests

	// Acquire storage from an existing storage request.
	// This is a cancel point if a replan is initiated.
	PlaceClaimFromExistingStorageRequests

	// Acquire storage by issuing a new storage request
	PlaceIssueNewStorageRequests

	// Allocate capacity in acquired Storage objects
	PlaceAllocateStorageCapacity

	// If there are pending StorageRequest objects then go to the STORAGE_WAIT state.
	PlaceWaitForStorageRequests

	// update the VolumeSeries object if not already done so
	PlaceUpdateVolumeSeries

	// Terminal state during placement. Must precede any undo state.
	PlaceDone

	//
	// Provisioned Volumes that need storage reattached
	//

	// Start by entering the critical section
	PlaceReattachStartPlanning

	// Construct the plan for storage movement and save it
	PlaceReattachPopulatePlan

	// Start executing the reattachment plan by entering the critical section if not already within it
	PlaceReattachExecute

	// Fetch all claims made by this request
	PlaceReattachFetchClaims

	// Handle completed StorageRequests
	// Update the storage plan if any StorageRequest completed. This is a cancel point.
	PlaceReattachHandleCompletedStorageRequests

	// Reattach storage as necessary by issuing a new storage request
	PlaceReattachIssueNewStorageRequests

	// If there are pending StorageRequest objects then go to the STORAGE_WAIT state.
	PlaceReattachWaitForStorageRequests

	PlaceReattachDone

	//
	// DELETE, UNDO (un-provisioned)
	//

	// Enter the critical section during DELETE operation processing
	PlaceDeleteStart

	// Tag Storage and VolumeSeries objects that will be affected by the DELETE operation
	PlaceTagForDeletion

	// Build a pseudo-StoragePlan so the undo path can be used for DELETE processing
	PlacePlanDeletion

	// Enter the critical section during undo
	PlaceUndoStart

	// Undo VolumeSeries changes if necessary, also resets the state to BOUND if the current state is provisioned or beyond
	PlaceUndoVolumeSeries

	// Recover Storage capacity if necessary
	PlaceUndoReleaseStorageCapacity

	// Remove all claims from cluster state object.
	PlaceUndoRemoveClaims

	// TBD Return Storage if necessary. Will return RetryLater if waiting for SRs.
	PlaceUndoReturnStorage

	// Undo completion
	PlaceUndoDone

	// Error
	PlaceError

	// LAST: No operation is performed in this state.
	PlaceNoOp
)

func (ss placeSubState) String() string {
	switch ss {
	case PlaceStartPlanning:
		return "PlaceStartPlanning"
	case PlaceSelectStorage:
		return "PlaceSelectStorage"
	case PlacePopulatePlan:
		return "PlacePopulatePlan"
	case PlaceStartExecuting:
		return "PlaceStartExecuting"
	case PlaceFetchClaims:
		return "PlaceFetchClaims"
	case PlaceHandleCompletedStorageRequests:
		return "PlaceHandleCompletedStorageRequests"
	case PlaceClaimFromExistingStorageRequests:
		return "PlaceClaimFromExistingStorageRequests"
	case PlaceIssueNewStorageRequests:
		return "PlaceIssueNewStorageRequests"
	case PlaceAllocateStorageCapacity:
		return "PlaceAllocateStorageCapacity"
	case PlaceWaitForStorageRequests:
		return "PlaceWaitForStorageRequests"
	case PlaceUpdateVolumeSeries:
		return "PlaceUpdateVolumeSeries"
	case PlaceDone:
		return "PlaceDone"
	case PlaceReattachStartPlanning:
		return "PlaceReattachStartPlanning"
	case PlaceReattachPopulatePlan:
		return "PlaceReattachPopulatePlan"
	case PlaceReattachExecute:
		return "PlaceReattachExecute"
	case PlaceReattachFetchClaims:
		return "PlaceReattachFetchClaims"
	case PlaceReattachHandleCompletedStorageRequests:
		return "PlaceReattachHandleCompletedStorageRequests"
	case PlaceReattachIssueNewStorageRequests:
		return "PlaceReattachIssueNewStorageRequests"
	case PlaceReattachWaitForStorageRequests:
		return "PlaceReattachWaitForStorageRequests"
	case PlaceReattachDone:
		return "PlaceReattachDone"
	case PlaceDeleteStart:
		return "PlaceDeleteStart"
	case PlaceTagForDeletion:
		return "PlaceTagForDeletion"
	case PlacePlanDeletion:
		return "PlacePlanDeletion"
	case PlaceUndoStart:
		return "PlaceUndoStart"
	case PlaceUndoVolumeSeries:
		return "PlaceUndoVolumeSeries"
	case PlaceUndoReleaseStorageCapacity:
		return "PlaceUndoReleaseStorageCapacity"
	case PlaceUndoRemoveClaims:
		return "PlaceUndoRemoveClaims"
	case PlaceUndoReturnStorage:
		return "PlaceUndoReturnStorage"
	case PlaceUndoDone:
		return "PlaceUndoDone"
	case PlaceError:
		return "PlaceError"
	}
	return fmt.Sprintf("placeSubState(%d)", ss)
}

type placeOp struct {
	c                     *Component
	rhs                   *vra.RequestHandlerState
	ops                   placeOperators
	inError               bool
	stateCST              *util.CriticalSectionTicket
	selStg                *state.SelectStorageResponse
	claims                state.ClaimList
	activeSRs             bool
	rID                   string
	vsrTag                string
	mustRestart           bool
	mustUpdateStoragePlan bool
	mustUpdateRHS         bool
	capByP                map[string]int64 // capacity by Pool in this VSR
	capByS                map[string]int64 // capacity by Storage in this VSR
	la                    *layout.Algorithm
}

type placeOperators interface {
	allocateStorageCapacity(ctx context.Context)
	assignStorageInVolumeSeries(ctx context.Context)
	claimFromExistingStorageRequests(ctx context.Context)
	fetchClaims(ctx context.Context)
	getInitialState(ctx context.Context) placeSubState
	handleCompletedReattachStorageRequests(ctx context.Context)
	handleCompletedStorageRequests(ctx context.Context)
	issueNewReattachStorageRequests(ctx context.Context)
	issueNewStorageRequests(ctx context.Context)
	populateDeletionPlan(ctx context.Context)
	populatePlan(ctx context.Context)
	populateReattachPlan(ctx context.Context)
	recomputeStoragePlan(ctx context.Context)
	releaseStorageCapacity(ctx context.Context)
	removeClaims(ctx context.Context)
	removeStorageFromVolumeSeries(ctx context.Context)
	selectStorage(ctx context.Context)
	tagObjects(ctx context.Context)
	updatePlan(ctx context.Context)
	waitForLock(ctx context.Context)
	waitForStorageRequests(ctx context.Context)
}

// Place performs the PLACEMENT or PLACEMENT_REATTACH operation
func (c *Component) Place(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &placeOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// UndoPlace undoes the PLACEMENT operation
func (c *Component) UndoPlace(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &placeOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

func (op *placeOp) run(ctx context.Context) {
out:
	for ss := op.ops.getInitialState(ctx); !(op.rhs.InError || op.rhs.RetryLater || op.mustRestart); ss++ {
		op.c.Log.Debugf("VolumeSeriesRequest %s: PLACE %s", op.rhs.Request.Meta.ID, ss)
		switch ss {
		// Un-provisioned volumes
		case PlaceStartPlanning:
			op.ops.waitForLock(ctx)
		case PlaceSelectStorage:
			op.ops.selectStorage(ctx)
		case PlacePopulatePlan:
			op.ops.populatePlan(ctx)
			op.ops.updatePlan(ctx)
			if swag.BoolValue(op.rhs.Request.PlanOnly) || op.rhs.Canceling {
				break out
			}
		case PlaceStartExecuting:
			op.ops.waitForLock(ctx) // reentrant
		case PlaceFetchClaims:
			op.ops.fetchClaims(ctx)
		case PlaceHandleCompletedStorageRequests:
			op.ops.handleCompletedStorageRequests(ctx)
			op.ops.updatePlan(ctx)
			if op.rhs.Canceling {
				break out
			}
		case PlaceClaimFromExistingStorageRequests:
			op.ops.claimFromExistingStorageRequests(ctx)
		case PlaceIssueNewStorageRequests:
			op.ops.issueNewStorageRequests(ctx)
		case PlaceAllocateStorageCapacity:
			op.ops.allocateStorageCapacity(ctx)
		case PlaceWaitForStorageRequests:
			op.ops.waitForStorageRequests(ctx)
		case PlaceUpdateVolumeSeries:
			op.ops.assignStorageInVolumeSeries(ctx)

		// Provisioned volumes with local unshared storage
		case PlaceReattachStartPlanning:
			op.ops.waitForLock(ctx)
		case PlaceReattachPopulatePlan:
			op.ops.populateReattachPlan(ctx)
			op.ops.updatePlan(ctx)
			if swag.BoolValue(op.rhs.Request.PlanOnly) || op.rhs.Canceling {
				break out
			}
		case PlaceReattachExecute:
			op.ops.waitForLock(ctx) // reentrant
		case PlaceReattachFetchClaims:
			op.ops.fetchClaims(ctx)
		case PlaceReattachHandleCompletedStorageRequests:
			op.ops.handleCompletedReattachStorageRequests(ctx)
			op.ops.updatePlan(ctx)
			if op.rhs.Canceling {
				break out
			}
		case PlaceReattachIssueNewStorageRequests:
			op.ops.issueNewReattachStorageRequests(ctx)
		case PlaceReattachWaitForStorageRequests:
			op.ops.waitForStorageRequests(ctx)

		// undo/delete
		case PlaceDeleteStart:
			op.ops.waitForLock(ctx)
		case PlaceTagForDeletion:
			op.ops.tagObjects(ctx)
		case PlacePlanDeletion:
			op.ops.populateDeletionPlan(ctx)
			op.ops.updatePlan(ctx)
			if swag.BoolValue(op.rhs.Request.PlanOnly) {
				break out
			}
		case PlaceUndoStart:
			op.ops.waitForLock(ctx) // reentrant
		case PlaceUndoVolumeSeries:
			op.ops.removeStorageFromVolumeSeries(ctx)
		case PlaceUndoReleaseStorageCapacity:
			op.ops.releaseStorageCapacity(ctx)
		case PlaceUndoRemoveClaims:
			op.ops.removeClaims(ctx)
		default:
			break out
		}
	}
	// release reservation lock if held
	if op.stateCST != nil {
		op.c.Log.Debugf("VolumeSeriesRequest %s: releasing state lock", op.rID)
		op.stateCST.Leave()
	}
	if op.mustRestart {
		op.ops.recomputeStoragePlan(ctx)
	}
	if op.inError {
		op.rhs.InError = true
	}
}

func (op *placeOp) getInitialState(ctx context.Context) placeSubState {
	op.rID = string(op.rhs.Request.Meta.ID)
	op.vsrTag = fmt.Sprintf("%s:%s", com.SystemTagVsrPlacement, op.rID)
	vs := op.rhs.VolumeSeries
	if op.rhs.Request.VolumeSeriesRequestState == com.VolReqStateUndoPlacement {
		op.inError = op.rhs.InError
		op.rhs.InError = false // switch off to enable cleanup
		if swag.BoolValue(op.rhs.Request.PlanOnly) {
			return PlaceUndoDone
		}
		deletePlanTag := fmt.Sprintf("%s:%s", com.SystemTagVsrPlacement, com.VolStateDeleting)
		if (op.rhs.HasDelete || op.rhs.HasUnbind) && (op.rhs.Request.StoragePlan == nil || len(op.rhs.Request.StoragePlan.StorageElements) == 0 || !util.Contains(op.rhs.Request.SystemTags, deletePlanTag)) {
			return PlaceDeleteStart
		}
		return PlaceUndoStart
	}
	if vra.VolumeSeriesIsProvisioned(vs.VolumeSeriesState) {
		// recover the layout algorithm of the provisioned volume
		var err error
		var laName string
		if op.rhs.VolumeSeries.LifecycleManagementData != nil {
			laName = op.rhs.VolumeSeries.LifecycleManagementData.LayoutAlgorithm
		}
		op.la, err = layout.FindAlgorithm(laName)
		if err != nil {
			op.rhs.SetRequestError("Invalid layout algorithm '%s' recorded in VolumeSeries", laName)
			return PlaceError
		}
		// If the layout algorithm uses unshared local storage then we need to reattach the storage to the local node
		if op.la.SharedStorageOk || op.la.RemoteStorageOk {
			return PlaceDone // nothing to do
		}
		for _, el := range op.rhs.Request.StoragePlan.StorageElements {
			if el.Intent == com.VolReqStgElemIntentData {
				return PlaceReattachExecute
			}
		}
		return PlaceReattachStartPlanning
	}
	if op.rhs.Request.StoragePlan == nil || len(op.rhs.Request.StoragePlan.StorageElements) < 1 {
		op.rhs.SetRequestError("Invalid StoragePlan")
		return PlaceDone
	}
	for _, el := range op.rhs.Request.StoragePlan.StorageElements {
		if el.Intent != com.VolReqStgElemIntentCache {
			if _, has := el.StorageParcels[com.VolReqStgPseudoParcelKey]; has {
				return PlaceStartPlanning
			}
			break
		}
	}
	return PlaceStartExecuting
}

// waitForLock is called in multiple sub-states; it is re-entrant
func (op *placeOp) waitForLock(ctx context.Context) {
	if op.stateCST != nil {
		return
	}
	op.c.Log.Debugf("VolumeSeriesRequest %s: acquiring state lock", op.rID)
	cst, err := op.c.App.StateGuard.Enter(string(op.rID))
	if err != nil {
		op.rhs.SetRequestMessage("state lock error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.c.Log.Debugf("VolumeSeriesRequest %s: obtained state lock", op.rID)
	op.stateCST = cst
}

func (op *placeOp) selectStorage(ctx context.Context) {
	ssa := &state.SelectStorageArgs{
		VSR: op.rhs.Request,
		VS:  op.rhs.VolumeSeries,
	}
	la, err := layout.FindAlgorithm(op.rhs.Request.StoragePlan.LayoutAlgorithm) // set in sizing
	if err != nil {
		op.rhs.SetRequestError("Layout algorithm %s not found", op.rhs.Request.StoragePlan.LayoutAlgorithm)
		return
	}
	ssa.LA = la
	res, err := op.c.App.StateOps.SelectStorage(ctx, ssa)
	if err != nil {
		op.rhs.SetRequestMessage("Select storage error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.selStg = res
}

// Tag Storage and VolumeSeries objects with vsrTag for DELETE operation
func (op *placeOp) tagObjects(ctx context.Context) {
	if swag.BoolValue(op.rhs.Request.PlanOnly) {
		return
	}

	cs := op.c.App.StateOps.CS()
	for k := range op.rhs.VolumeSeries.StorageParcels {
		sObj, isAttached := op.c.App.StateOps.LookupStorage(k)
		if sObj == nil {
			op.rhs.SetRequestMessage("Storage %s not present", k)
			continue
		}
		if util.Contains(sObj.SystemTags, op.vsrTag) {
			op.c.Log.Debugf("VolumeSeriesRequest %s: already tagged Storage %s", op.rID, k)
			continue
		}
		sObj.SystemTags = []string{op.vsrTag}
		items := &crud.Updates{}
		items.Append = []string{"systemTags"}
		op.c.Log.Debugf("VolumeSeriesRequest %s: tagging Storage[%s]", op.rID, k)
		obj, err := op.c.oCrud.StorageUpdate(ctx, sObj, items)
		if err != nil {
			op.rhs.SetRequestMessage("Storage %s update error: %s", k, err.Error())
			op.rhs.RetryLater = true
			return
		}
		if isAttached {
			cs.Storage[k] = obj // refresh
		} else {
			cs.DetachedStorage[k] = obj // refresh
		}
		op.rhs.SetRequestMessage("Tagged Storage[%s]", k)
	}

	changedVsState := ""
	vsID := string(op.rhs.VolumeSeries.Meta.ID)
	getVS := func(vs *models.VolumeSeries) (*models.VolumeSeries, error) {
		if vs == nil {
			vs = op.rhs.VolumeSeries
		}
		if op.rhs.HasDelete && vs.VolumeSeriesState != com.VolStateDeleting {
			// DELETE can start in UNDO_PLACEMENT for a provisioned volume that has never been configured, so the DELETING state is not set
			vs.Messages = util.NewMsgList(vs.Messages).Insert("State change %s ⇒ %s", vs.VolumeSeriesState, com.VolStateDeleting).ToModel()
			vs.VolumeSeriesState = com.VolStateDeleting
			changedVsState = " and set state to " + com.VolStateDeleting
		}
		vs.SystemTags = []string{op.vsrTag}
		return vs, nil
	}
	items := &crud.Updates{
		Set:    []string{"messages", "volumeSeriesState"},
		Append: []string{"systemTags"},
	}
	obj, err := op.c.oCrud.VolumeSeriesUpdater(ctx, vsID, getVS, items)
	if err != nil {
		op.rhs.SetRequestMessage("Failed to update VolumeSeries object: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.rhs.SetRequestMessage("Tagged VolumeSeries[%s]%s", vsID, changedVsState)
	op.rhs.VolumeSeries = obj
}

func (op *placeOp) populatePlan(ctx context.Context) {
	plan := op.rhs.Request.StoragePlan
	for i, el := range op.selStg.Elements {
		// StorageElements with CACHE intent are no-ops: they have no VolReqStgPseudoParcelKey pseudo-key and an empty Items list
		delete(plan.StorageElements[i].StorageParcels, com.VolReqStgPseudoParcelKey)
		for j, item := range el.Items {
			spe := models.StorageParcelElement{
				SizeBytes:        swag.Int64(item.SizeBytes),
				ShareableStorage: item.ShareableStorage,
			}
			var key string
			if item.Storage != nil {
				key = string(item.Storage.Meta.ID)
				op.c.Log.Debugf("VolumeSeriesRequest %s: StoragePlan(%d,%d) %s: Sz:%s", op.rID, i, j, key, util.SizeBytes(item.SizeBytes))
			} else {
				key = fmt.Sprintf("%s-%d-%d", com.VolReqStgPseudoParcelKey, i, j)
				spe.ProvMinSizeBytes = swag.Int64(item.MinSizeBytes)
				spe.ProvParcelSizeBytes = swag.Int64(el.ParcelSizeBytes)
				spe.ProvRemainingSizeBytes = swag.Int64(item.RemainingSizeBytes)
				spe.ProvNodeID = item.NodeID
				spe.ProvStorageRequestID = item.StorageRequestID
				op.c.Log.Debugf("VolumeSeriesRequest %s: StoragePlan(%d,%d) %s: Sz:%s MSz:%s PSz:%s RB:%s N:%s SR:%s", op.rID, i, j, key,
					util.SizeBytes(item.SizeBytes), util.SizeBytes(item.MinSizeBytes), util.SizeBytes(el.ParcelSizeBytes),
					util.SizeBytes(item.RemainingSizeBytes), item.NodeID, item.StorageRequestID)
			}
			plan.StorageElements[i].StorageParcels[key] = spe
		}
	}
	op.selStg = nil
	op.mustUpdateStoragePlan = true
}

// populateDeletionPlan populates the StoragePlan from the VolumeSeries so the undo path be used for a DELETE operation
func (op *placeOp) populateDeletionPlan(ctx context.Context) {
	vs := op.rhs.VolumeSeries
	plan := &models.StoragePlan{}
	plan.StorageLayout = models.StorageLayoutStandalone
	for spID := range vs.CapacityAllocations {
		sf := func(sObj *models.Storage) bool {
			_, match := vs.StorageParcels[string(sObj.Meta.ID)]
			if match {
				match = string(sObj.PoolID) == spID
			}
			return match
		}
		sList := op.c.App.StateOps.FindAllStorage(sf) // attachment and claim states irrelevant when deleting VS
		elem := &models.StoragePlanStorageElement{
			Intent:         com.VolReqStgElemIntentData,
			StorageParcels: map[string]models.StorageParcelElement{},
			PoolID:         models.ObjIDMutable(spID),
		}
		var totalBytes int64
		for _, sObj := range sList {
			sID := string(sObj.Meta.ID)
			parcel := vs.StorageParcels[sID]
			totalBytes += swag.Int64Value(parcel.SizeBytes)
			elem.StorageParcels[sID] = models.StorageParcelElement{SizeBytes: parcel.SizeBytes}
		}
		elem.SizeBytes = swag.Int64(totalBytes)
		plan.StorageElements = append(plan.StorageElements, elem)
	}

	op.rhs.Request.StoragePlan = plan
	sTags := util.NewTagList(op.rhs.Request.SystemTags)
	sTags.Set(com.SystemTagVsrPlacement, com.VolStateDeleting)
	op.rhs.Request.SystemTags = sTags.List() // tag differentiates re-attach storage plan from deletion storage plan
	op.mustUpdateStoragePlan = true
}

// populateReattachPlan extends the StoragePlan to track reattachment progress
// Note: "CACHE" elements may exist in the plan from the sizing step
func (op *placeOp) populateReattachPlan(ctx context.Context) {
	plan := op.rhs.Request.StoragePlan
	// Add "DATA" StoragePlanStorageElement for each Storage object used by the Volume
	// Added elements are sparse as we only use them to track reattachment progress
	for storageID, pa := range op.rhs.VolumeSeries.StorageParcels {
		sObj, isAttached := op.c.App.StateOps.LookupStorage(storageID)
		storageNodeID := ""
		if isAttached {
			storageNodeID = string(sObj.StorageState.AttachedNodeID)
		}
		if sObj == nil {
			op.rhs.SetRequestMessage("Storage [%s] not accessible", storageID)
			op.rhs.RetryLater = true
			return
		}
		se := &models.StoragePlanStorageElement{
			Intent:         com.VolReqStgElemIntentData,
			PoolID:         sObj.PoolID,
			StorageParcels: map[string]models.StorageParcelElement{},
		}
		plan.StorageElements = append(plan.StorageElements, se)
		se.StorageParcels[string(sObj.Meta.ID)] = models.StorageParcelElement{
			ProvNodeID: storageNodeID, // empty string is ok - this is the case if the storage is not attached
			SizeBytes:  pa.SizeBytes,
		}
		op.c.Log.Debugf("VolumeSeriesRequest %s: StoragePlan(%d,0) StorageID[%s] NodeID[%s]", op.rID, len(plan.StorageElements)-1, sObj.Meta.ID, storageNodeID)
	}
	op.mustUpdateStoragePlan = true
}

// updatePlan is invoked from multiple sub-states and provides a cancellation point
func (op *placeOp) updatePlan(ctx context.Context) {
	if op.mustUpdateStoragePlan {
		_, err := op.rhs.SetAndUpdateRequestMessage(ctx, "Storage plan updated") // could detect cancel
		if err != nil {
			op.rhs.RetryLater = true
			return
		}
		op.mustUpdateStoragePlan = false
	}
}

func (op *placeOp) fetchClaims(ctx context.Context) {
	op.claims = op.c.App.StateOps.FindClaimsByRequest(op.rID)
}

func (op *placeOp) handleCompletedStorageRequests(ctx context.Context) {
	for _, el := range op.rhs.Request.StoragePlan.StorageElements {
		keys := util.StringKeys(el.StorageParcels) // need to modify the map while traversing
		for _, k := range keys {
			if strings.HasPrefix(k, com.VolReqStgPseudoParcelKey) {
				if vsrC, clD := op.claims.FindVSRClaimByAnnotation(op.rID, k); vsrC != nil {
					if err := op.c.rei.ErrOnBool("sr-race"); err != nil {
						op.rhs.RetryLater = true
						return
					}
					sID := string(clD.StorageRequest.StorageID)
					srID := string(clD.StorageRequest.Meta.ID)
					srState := clD.StorageRequest.StorageRequestState
					op.c.Log.Debugf("VolumeSeriesRequest %s: %s ⇒ StorageRequest[%s] %s", op.rID, k, srID, srState)
					if srState == com.StgReqStateSucceeded {
						op.c.Log.Debugf("VolumeSeriesRequest %s: %s ⇒ Storage[%s]", op.rID, k, sID)
						el.StorageParcels[sID] = el.StorageParcels[k] // replace StorageRequest
						delete(el.StorageParcels, k)                  // with Storage
						op.mustUpdateStoragePlan = true
						clD.RemoveClaim(op.rID) // in-mem only
					} else if srState == com.StgReqStateFailed {
						detail := vra.GetFirstErrorMessage(clD.StorageRequest.RequestMessages)
						if detail == "" {
							detail = "failed"
						}
						op.rhs.SetAndUpdateRequestError(ctx, "Error: %s StorageRequest[%s] %s", k, srID, detail)
						clD.RemoveClaim(op.rID) // in-mem only
						return
					} else {
						op.activeSRs = true
					}
				}
			}
		}
	}
}

func (op *placeOp) handleCompletedReattachStorageRequests(ctx context.Context) {
	nodeID := string(op.rhs.Request.NodeID)
	for _, el := range op.rhs.Request.StoragePlan.StorageElements {
		if el.Intent == com.VolReqStgElemIntentData {
			for storageID, spe := range el.StorageParcels {
				if vsrC, clD := op.claims.FindVSRClaimByAnnotation(op.rID, storageID); vsrC != nil {
					srID := string(clD.StorageRequest.Meta.ID)
					srState := clD.StorageRequest.StorageRequestState
					if srState == com.StgReqStateSucceeded {
						op.rhs.SetRequestMessage("Reattached Storage [%s] Node:[%s ⇒ %s]", storageID, spe.ProvNodeID, nodeID)
						spe.ProvNodeID = nodeID
						el.StorageParcels[storageID] = spe
						op.mustUpdateStoragePlan = true
						clD.RemoveClaim(op.rID) // in-mem only
					} else if srState == com.StgReqStateFailed {
						op.rhs.SetRequestMessage("REATTACH [%s] StorageRequest[%s] failed", storageID, srID)
						clD.RemoveClaim(op.rID) // in-mem only; removing claim results in a re-attach attempt
					} else {
						op.c.Log.Debugf("VolumeSeriesRequest %s: REATTACH [%s] StorageRequest[%s] %s", op.rID, storageID, srID, srState)
						op.activeSRs = true
					}
				}
			}
		}
	}
}

func (op *placeOp) claimFromExistingStorageRequests(ctx context.Context) {
	numClaims := 0
	for _, el := range op.rhs.Request.StoragePlan.StorageElements {
		for k, spe := range el.StorageParcels {
			if strings.HasPrefix(k, com.VolReqStgPseudoParcelKey) && spe.ProvStorageRequestID != "" {
				if vsrC, _ := op.claims.FindVSRClaimByAnnotation(op.rID, k); vsrC == nil {
					vc := &state.VSRClaim{
						RequestID:  op.rID,
						SizeBytes:  swag.Int64Value(spe.SizeBytes),
						Annotation: k,
					}
					cd := op.c.App.StateOps.FindClaimByStorageRequest(spe.ProvStorageRequestID)
					if cd == nil {
						op.rhs.SetRequestMessage("%s references missing SR %s", k, spe.ProvStorageRequestID)
						op.mustRestart = true
						return
					}
					if cd.StorageRequest.StorageRequestState == com.StgReqStateFailed {
						op.c.Log.Debugf("VolumeSeriesRequest %s: Skipping failed SR[%s]", op.rID, spe.ProvStorageRequestID)
						continue
					}
					if _, err := op.c.App.StateOps.AddClaim(ctx, cd.StorageRequest, vc); err != nil {
						op.rhs.SetRequestMessage("%s: AddClaim error %s", k, err.Error())
						op.rhs.RetryLater = true
						return
					}
					op.c.Log.Debugf("VolumeSeriesRequest %s: Added claim for %s to SR[%s]", op.rID, util.SizeBytesToString(vc.SizeBytes), spe.ProvStorageRequestID)
					numClaims++
				}
			}
		}
	}
	if numClaims > 0 {
		op.fetchClaims(ctx) // refresh
		op.activeSRs = true
		op.mustUpdateRHS = true
	}
}

func (op *placeOp) recomputeStoragePlan(ctx context.Context) {
	op.c.App.StateOps.RemoveClaims(op.rID)
	op.rhs.SetRequestMessage("Recomputing storagePlan")
	op.rhs.Request.StoragePlan = &models.StoragePlan{}
	// need to repeat SIZING
	op.rhs.SetAndUpdateRequestState(ctx, com.VolReqStateSizing) // ignore error as we're bailing out anyway
	op.rhs.RetryLater = true
	op.c.Animator.Notify() // nudge, nudge
}

func (op *placeOp) issueNewStorageRequests(ctx context.Context) {
	for _, el := range op.rhs.Request.StoragePlan.StorageElements {
		for k, spe := range el.StorageParcels {
			if strings.HasPrefix(k, com.VolReqStgPseudoParcelKey) && spe.ProvStorageRequestID == "" {
				if vsrC, _ := op.claims.FindVSRClaimByAnnotation(op.rID, k); vsrC == nil {
					sr := &models.StorageRequest{
						StorageRequestCreateOnce: models.StorageRequestCreateOnce{
							CompleteByTime:      op.rhs.Request.CompleteByTime,
							PoolID:              el.PoolID,
							MinSizeBytes:        spe.ProvMinSizeBytes,
							ParcelSizeBytes:     spe.ProvParcelSizeBytes,
							ShareableStorage:    spe.ShareableStorage,
							RequestedOperations: []string{com.StgReqOpProvision, com.StgReqOpAttach, com.StgReqOpFormat, com.StgReqOpUse},
						},
						StorageRequestMutable: models.StorageRequestMutable{
							StorageRequestCreateMutable: models.StorageRequestCreateMutable{
								NodeID:     models.ObjIDMutable(spe.ProvNodeID),
								SystemTags: []string{op.vsrTag},
								VolumeSeriesRequestClaims: &models.VsrClaim{
									RemainingBytes: spe.ProvRemainingSizeBytes,
									Claims: map[string]models.VsrClaimElement{
										op.rID: models.VsrClaimElement{
											SizeBytes:  spe.SizeBytes,
											Annotation: k,
										},
									},
								},
							},
						},
					}
					op.c.Log.Debugf("VolumeSeriesRequest %s: Create StorageRequest %s SP:%s Node:%s Sz:%s remB:%s", op.rID, k, el.PoolID, spe.ProvNodeID, util.SizeBytesToString(swag.Int64Value(spe.ProvMinSizeBytes)), util.SizeBytesToString(swag.Int64Value(spe.ProvRemainingSizeBytes)))
					srObj, err := op.c.oCrud.StorageRequestCreate(ctx, sr)
					if err != nil {
						op.rhs.SetRequestMessage("%s: StorageRequest error %s", k, err.Error())
						op.rhs.RetryLater = true
						return
					}
					op.activeSRs = true
					op.mustUpdateRHS = true
					op.c.App.StateOps.TrackStorageRequest(srObj)
				}
			}
		}
	}
}

func (op *placeOp) issueNewReattachStorageRequests(ctx context.Context) {
	nodeID := string(op.rhs.Request.NodeID)
	for _, el := range op.rhs.Request.StoragePlan.StorageElements {
		for storageID, spe := range el.StorageParcels {
			if el.Intent == com.VolReqStgElemIntentData && spe.ProvNodeID != nodeID {
				if vsrC, _ := op.claims.FindVSRClaimByAnnotation(op.rID, storageID); vsrC == nil {
					sr := &models.StorageRequest{
						StorageRequestCreateOnce: models.StorageRequestCreateOnce{
							CompleteByTime:      op.rhs.Request.CompleteByTime,
							RequestedOperations: []string{com.StgReqOpReattach},
							ReattachNodeID:      models.ObjIDMutable(nodeID),
						},
						StorageRequestMutable: models.StorageRequestMutable{
							StorageRequestCreateMutable: models.StorageRequestCreateMutable{
								StorageID:  models.ObjIDMutable(storageID),
								SystemTags: []string{op.vsrTag},
								VolumeSeriesRequestClaims: &models.VsrClaim{
									Claims: map[string]models.VsrClaimElement{
										op.rID: models.VsrClaimElement{
											SizeBytes:  spe.SizeBytes,
											Annotation: storageID,
										},
									},
								},
							},
						},
					}
					op.rhs.SetRequestMessage("Reattaching Storage [%s] Node:[%s ⇒ %s]", storageID, spe.ProvNodeID, nodeID)
					srObj, err := op.c.oCrud.StorageRequestCreate(ctx, sr)
					if err != nil {
						op.rhs.SetRequestMessage("Storage [%s]: StorageRequest error %s", storageID, err.Error())
						op.rhs.RetryLater = true
						return
					}
					op.activeSRs = true
					op.mustUpdateRHS = true
					op.c.App.StateOps.TrackStorageRequest(srObj)
				}
			}
		}
	}
}

func (op *placeOp) removeClaims(ctx context.Context) {
	op.c.Log.Debugf("VolumeSeriesRequest %s: removing all claims from cluster state", op.rID)
	op.c.App.StateOps.RemoveClaims(op.rID)
}

func (op *placeOp) allocateStorageCapacity(ctx context.Context) {
	cs := op.c.App.StateOps.CS()
	for _, el := range op.rhs.Request.StoragePlan.StorageElements {
		if el.Intent == com.VolReqStgElemIntentCache {
			continue
		}
		for k, spe := range el.StorageParcels {
			if !strings.HasPrefix(k, com.VolReqStgPseudoParcelKey) { // Storage ID
				sObj, has := cs.Storage[k]
				if !has {
					op.rhs.SetRequestError("Storage %s not present", k)
					return
				}
				if util.Contains(sObj.SystemTags, op.vsrTag) {
					op.c.Log.Debugf("VolumeSeriesRequest %s: already processed Storage %s", op.rID, k)
					continue
				}
				avail := swag.Int64Value(sObj.AvailableBytes)
				sz := swag.Int64Value(spe.SizeBytes)
				if avail < sz {
					op.rhs.SetRequestError("Storage %s: Insufficient capacity: avail=%s need=%s", k, util.SizeBytes(avail), util.SizeBytes(sz))
					return
				}
				sObj.AvailableBytes = swag.Int64(avail - sz)
				sObj.SystemTags = []string{op.vsrTag}
				sObj.StorageState.Messages = util.NewMsgList(sObj.StorageState.Messages).Insert("Allocated %s to VSR %s", util.SizeBytes(sz), op.rID).ToModel()
				items := &crud.Updates{}
				items.Set = []string{"availableBytes", "storageState"}
				items.Append = []string{"systemTags"}
				op.c.Log.Debugf("VolumeSeriesRequest %s: Allocating %s from Storage[%s,%v,%v]", op.rID, util.SizeBytes(sz), k, sObj.ShareableStorage, spe.ShareableStorage)
				if sObj.ShareableStorage != spe.ShareableStorage {
					items.Set = append(items.Set, "shareableStorage")
					sObj.ShareableStorage = spe.ShareableStorage
				}
				obj, err := op.c.oCrud.StorageUpdate(ctx, sObj, items)
				if err != nil {
					op.rhs.SetRequestMessage("Storage %s update error: %s", k, err.Error())
					op.rhs.RetryLater = true
					return
				}
				cs.Storage[k] = obj // refresh
				op.rhs.SetRequestMessage("Allocated %s from Storage[%s]", util.SizeBytes(sz), k)
				op.mustUpdateRHS = true
			}
		}
	}
}

func (op *placeOp) releaseStorageCapacity(ctx context.Context) {
	cs := op.c.App.StateOps.CS()
	for _, el := range op.rhs.Request.StoragePlan.StorageElements {
		if el.Intent == com.VolReqStgElemIntentCache {
			continue
		}
		for k, spe := range el.StorageParcels {
			if !strings.HasPrefix(k, com.VolReqStgPseudoParcelKey) { // Storage ID
				sObj, isAttached := op.c.App.StateOps.LookupStorage(k)
				if sObj == nil {
					op.c.Log.Debugf("VolumeSeriesRequest %s: Storage %s not present", op.rID, k)
					continue
				}
				if !util.Contains(sObj.SystemTags, op.vsrTag) {
					op.c.Log.Debugf("VolumeSeriesRequest %s: already processed Storage %s", op.rID, k)
					continue
				}
				avail := swag.Int64Value(sObj.AvailableBytes)
				sz := swag.Int64Value(spe.SizeBytes)
				if sObj.ShareableStorage {
					sObj.AvailableBytes = swag.Int64(avail + sz)
				} else { // restore all parcels
					sObj.AvailableBytes = swag.Int64(swag.Int64Value(sObj.TotalParcelCount) * swag.Int64Value(sObj.ParcelSizeBytes))
				}
				sObj.SystemTags = []string{op.vsrTag}
				sObj.StorageState.Messages = util.NewMsgList(sObj.StorageState.Messages).Insert("Returned %s", util.SizeBytes(sz)).ToModel()
				items := &crud.Updates{}
				items.Set = []string{"availableBytes", "storageState"}
				items.Remove = []string{"systemTags"}
				op.c.Log.Debugf("VolumeSeriesRequest %s: Releasing %s from Storage[%s]", op.rID, util.SizeBytes(sz), k)
				obj, err := op.c.oCrud.StorageUpdate(ctx, sObj, items)
				if err != nil {
					op.rhs.SetRequestMessage("Storage %s update error: %s", k, err.Error())
					op.rhs.RetryLater = true
					return
				}
				if isAttached {
					cs.Storage[k] = obj // refresh
				} else {
					cs.DetachedStorage[k] = obj // refresh
				}
				op.rhs.SetRequestMessage("Released %s from Storage[%s]", util.SizeBytes(sz), k)
				op.mustUpdateRHS = true
			}
		}
	}
}

func (op *placeOp) waitForStorageRequests(ctx context.Context) {
	if op.activeSRs {
		// Normally STORAGE_WAIT ⇒ PLACEMENT ⇒ STORAGE_WAIT will not be updated in the database.
		// However, if we've created SRs in this transition, or if we've made changes to the VSR then force an update.
		// Likewise, UNDO_PLACEMENT has no official WAIT state but we may have to retry.
		if op.mustUpdateRHS {
			if err := op.rhs.SetAndUpdateRequestState(ctx, com.VolReqStateStorageWait); err != nil {
				op.rhs.SetRequestError("Unable to update state: %s", err.Error()) // may not update
				return
			}
		} else {
			op.rhs.SetRequestState(com.VolReqStateStorageWait)
		}
		op.rhs.RetryLater = true
	}
}

func (op *placeOp) makePlanMaps() {
	op.capByP = make(map[string]int64) // capacity by Pool in this VSR
	op.capByS = make(map[string]int64) // capacity by Storage in this VSR
	for _, el := range op.rhs.Request.StoragePlan.StorageElements {
		if el.Intent == com.VolReqStgElemIntentCache {
			continue
		}
		spID := string(el.PoolID)
		szTotal := int64(0)
		for sID, spe := range el.StorageParcels {
			sz := swag.Int64Value(spe.SizeBytes)
			szTotal += sz
			op.capByS[sID] = sz // storage disjoint across storage elements
		}
		spCap, has := op.capByP[spID]
		if has {
			spCap += szTotal
		} else {
			spCap = szTotal
		}
		op.capByP[spID] = spCap
	}
}

func (op *placeOp) assignStorageInVolumeSeries(ctx context.Context) {
	vsID := string(op.rhs.VolumeSeries.Meta.ID)
	if str := op.c.rei.GetString("placement-block-on-vs-update"); str == vsID {
		op.rhs.SetAndUpdateRequestMessageDistinct(ctx, "Blocked on REI [vreq/placement-block-on-vs-update]")
		op.rhs.RetryLater = true
		return
	}
	if util.Contains(op.rhs.VolumeSeries.SystemTags, op.vsrTag) {
		op.c.Log.Debugf("VolumeSeriesRequest %s: already updated VolumeSeries[%s]", op.rID, vsID)
		return
	}
	op.makePlanMaps()
	cs := op.c.App.StateOps.CS()
	var buf bytes.Buffer
	// update callback
	getVS := func(vs *models.VolumeSeries) (*models.VolumeSeries, error) {
		if vs == nil {
			vs = op.rhs.VolumeSeries
		}
		buf.Reset()
		fmt.Fprintf(&buf, "\n capacityAllocations:")
		for _, spID := range util.SortedStringKeys(op.capByP) {
			consumed := op.capByP[spID]
			ca, has := vs.CapacityAllocations[spID]
			if has { // otherwise ignore it!
				cb := swag.Int64Value(ca.ConsumedBytes)
				cb += consumed
				ca.ConsumedBytes = swag.Int64(cb)
				vs.CapacityAllocations[spID] = ca
				fmt.Fprintf(&buf, "\n  - SP[%s] consumedBytes=%s (+%s)", spID, util.SizeBytes(cb), util.SizeBytes(consumed))
			}
		}
		if vs.StorageParcels == nil {
			vs.StorageParcels = map[string]models.ParcelAllocation{}
		}
		fmt.Fprintf(&buf, "\n storageParcels:")
		for _, sID := range util.SortedStringKeys(op.capByS) {
			cap := op.capByS[sID]
			sb := cap
			pa, has := vs.StorageParcels[sID]
			if has {
				sb = swag.Int64Value(pa.SizeBytes) + cap
			} else {
				pa = models.ParcelAllocation{}
			}
			pa.SizeBytes = swag.Int64(sb)
			vs.StorageParcels[sID] = pa
			spID := string(cs.Storage[sID].PoolID)
			fmt.Fprintf(&buf, "\n  - S[%s] sizeBytes=%s (+%s) SP[%s]", sID, util.SizeBytes(sb), util.SizeBytes(cap), spID)
		}
		msgList := util.NewMsgList(vs.Messages)
		msgList.Insert("VolumeSeries capacity added:%s", buf.String())
		if !vra.VolumeSeriesIsProvisioned(vs.VolumeSeriesState) {
			msgList.Insert("State change %s ⇒ %s", vs.VolumeSeriesState, com.VolStateProvisioned)
			vs.VolumeSeriesState = com.VolStateProvisioned
		}
		vs.Messages = msgList.ToModel()
		vs.SystemTags = []string{op.vsrTag}
		if vs.LifecycleManagementData == nil {
			vs.LifecycleManagementData = &models.LifecycleManagementData{}
		}
		vs.LifecycleManagementData.LayoutAlgorithm = op.rhs.Request.StoragePlan.LayoutAlgorithm
		return vs, nil
	}
	items := &crud.Updates{
		Set:    []string{"capacityAllocations", "storageParcels", "messages", "lifecycleManagementData", "volumeSeriesState"},
		Append: []string{"systemTags"},
	}
	obj, err := op.c.oCrud.VolumeSeriesUpdater(ctx, vsID, getVS, items)
	if err != nil {
		op.rhs.SetRequestMessage("Failed to update VolumeSeries object: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.rhs.SetRequestMessage("VolumeSeries capacity added:%s", buf.String())
	op.rhs.VolumeSeries = obj
}

func (op *placeOp) removeStorageFromVolumeSeries(ctx context.Context) {
	vsID := string(op.rhs.VolumeSeries.Meta.ID)
	if !util.Contains(op.rhs.VolumeSeries.SystemTags, op.vsrTag) {
		op.c.Log.Debugf("VolumeSeriesRequest %s: already undid change to VolumeSeries[%s]", op.rID, vsID)
		return
	}
	op.makePlanMaps()
	var buf bytes.Buffer
	// update callback
	getVS := func(vs *models.VolumeSeries) (*models.VolumeSeries, error) {
		if vs == nil {
			vs = op.rhs.VolumeSeries
		}
		buf.Reset()
		fmt.Fprintf(&buf, "\n capacityAllocations:")
		for _, spID := range util.SortedStringKeys(op.capByP) {
			consumed := op.capByP[spID]
			ca, has := vs.CapacityAllocations[spID]
			if has { // otherwise ignore it!
				cb := swag.Int64Value(ca.ConsumedBytes)
				cb -= consumed
				if cb != 0 {
					ca.ConsumedBytes = swag.Int64(cb)
				} else {
					ca.ConsumedBytes = nil
				}
				vs.CapacityAllocations[spID] = ca
				fmt.Fprintf(&buf, "\n  - SP[%s] consumedBytes=%s (-%s)", spID, util.SizeBytes(cb), util.SizeBytes(consumed))
			}
		}
		fmt.Fprintf(&buf, "\n storageParcels:")
		for _, sID := range util.SortedStringKeys(op.capByS) {
			cap := op.capByS[sID]
			pa := vs.StorageParcels[sID]
			sb := swag.Int64Value(pa.SizeBytes) - cap
			if sb == 0 {
				delete(vs.StorageParcels, sID)
			} else {
				pa.SizeBytes = swag.Int64(sb)
				vs.StorageParcels[sID] = pa
			}
			sObj, _ := op.c.App.StateOps.LookupStorage(sID)
			fmt.Fprintf(&buf, "\n  - S[%s] sizeBytes=%s (-%s) SP[%s]", sID, util.SizeBytes(sb), util.SizeBytes(cap), sObj.PoolID)
		}
		msgList := util.NewMsgList(vs.Messages).Insert("VolumeSeries capacity removed:%s", buf.String())
		if vra.VolumeSeriesIsProvisioned(vs.VolumeSeriesState) {
			msgList = msgList.Insert("State change %s ⇒ %s", vs.VolumeSeriesState, com.VolStateBound)
			vs.VolumeSeriesState = com.VolStateBound
		}
		vs.Messages = msgList.ToModel()
		vs.SystemTags = []string{op.vsrTag}
		if vs.LifecycleManagementData != nil {
			vs.LifecycleManagementData.LayoutAlgorithm = ""
		}
		return vs, nil
	}
	items := &crud.Updates{
		Set:    []string{"capacityAllocations", "storageParcels", "messages", "volumeSeriesState", "lifecycleManagementData"},
		Remove: []string{"systemTags"},
	}
	obj, err := op.c.oCrud.VolumeSeriesUpdater(ctx, vsID, getVS, items)
	if err != nil {
		op.rhs.SetRequestMessage("Failed to update VolumeSeries object: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.rhs.SetRequestMessage("VolumeSeries capacity removed:%s", buf.String())
	op.rhs.VolumeSeries = obj
}
