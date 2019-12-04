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
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/pool"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan_allocation"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/swag"
)

// Originally just the BIND operation state machine but later extended to support CHANGE_CAPACITY.

// Bind satisfies the vra.BindHandlers interface
func (c *Component) Bind(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &bindOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// UndoBind satisfies the vra.BindHandlers interface
func (c *Component) UndoBind(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &bindOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// ChangeCapacity satisfies the vra.ChangeCapacityHandlers interface
func (c *Component) ChangeCapacity(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &bindOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// UndoChangeCapacity satisfies the vra.ChangeCapacityHandlers interface
func (c *Component) UndoChangeCapacity(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &bindOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

var bindRemoveTagRe = regexp.MustCompile("^" + strings.Replace(com.SystemTagVolClusterPrefix, ".", "\\.", -1) + ".+")

// bind sub-states
type bindSubState int

// bindSubState values (the ordering is meaningful)
const (
	//
	// BINDING or CHANGING_CAPACITY
	//

	// Load necessary objects (BIND only)
	BindLoadObjects bindSubState = iota
	// Reservation is serialized.
	// The reservation lock is released on return
	BindWaitForReservationLock
	// Search for the associated ServicePlanAllocation object
	BindFindSPA
	// Load the ServicePlanAllocation object
	BindLoadSPA
	// Check if the ServicePlanAllocation object is marked in-use by some other VSR
	BindCheckAvailabilityOfSPA
	// Determine how much storage is required of each pool
	BindPlanCapacityAllocation
	// Check if the ServicePlanAllocation object has reservable capacity
	BindCheckForCapacity
	// Load the Pool objects
	BindLoadPools
	// Set the CapacityReservationResult in the Request
	BindSetCapacityReservationResult
	// Update VSR state before reservation
	BindUpdateVSRBeforeReservation
	// Reserve capacity in the ServicePlanAllocation object
	BindReserveCapacityInSPA
	// Commit changes to the VolumeSeries.
	// If this succeeds there is no undoing the operation.
	BindCommitVS
	// Commit change in the SPA (terminal state)
	BindCommitSPA
	// Set the VSR state to CAPACITY_WAIT (terminal state)
	BindSetCapacityWait

	//
	// UNDO_BINDING or UNDO_CHANGING_CAPACITY
	//

	// Enter the critical section before returning capacity
	BindUndoWaitForReservationLock
	// Load the ServicePlanAllocation object
	BindUndoLoadSPA
	// Check if the SPA is owned by the VSR
	BindUndoCheckSPAOwnership
	// Marks the SPA in-use in the database
	BindUndoSetSPAInUse
	// Save off in the VSR any VS props that will be cleared, if necessary
	BindUndoSaveVSPropsInVSR
	// Change the state of the VolumeSeries to UNBOUND and clear all capacity, or revert changes made by CHANGE_CAPACITY.
	BindUndoRevertVS
	// Determine how much capacity to return to the SPA
	BindUndoComputeCapacityAllocation
	// Return earlier committed capacity reservations. (terminal state)
	BindUndoReturnCapacityToSPA
	// This is a terminal sub-state.
	BindUndoDone
	// error
	BindError
	// LAST: No operation is performed in this state.
	BindNoOp
)

func (ss bindSubState) String() string {
	switch ss {
	case BindLoadObjects:
		return "BindLoadObjects"
	case BindWaitForReservationLock:
		return "BindWaitForReservationLock"
	case BindFindSPA:
		return "BindFindSPA"
	case BindLoadSPA:
		return "BindLoadSPA"
	case BindCheckAvailabilityOfSPA:
		return "BindCheckAvailabilityOfSPA"
	case BindPlanCapacityAllocation:
		return "BindPlanCapacityAllocation"
	case BindCheckForCapacity:
		return "BindCheckForCapacity"
	case BindLoadPools:
		return "BindLoadPools"
	case BindSetCapacityReservationResult:
		return "BindSetCapacityReservationResult"
	case BindUpdateVSRBeforeReservation:
		return "BindUpdateVSRBeforeReservation"
	case BindReserveCapacityInSPA:
		return "BindReserveCapacityInSPA"
	case BindCommitVS:
		return "BindCommitVS"
	case BindCommitSPA:
		return "BindCommitSPA"
	case BindSetCapacityWait:
		return "BindSetCapacityWait"
	case BindUndoWaitForReservationLock:
		return "BindUndoWaitForReservationLock"
	case BindUndoLoadSPA:
		return "BindUndoLoadSPA"
	case BindUndoCheckSPAOwnership:
		return "BindUndoCheckSPAOwnership"
	case BindUndoSetSPAInUse:
		return "BindUndoSetSPAInUse"
	case BindUndoSaveVSPropsInVSR:
		return "BindUndoSaveVSPropsInVSR"
	case BindUndoRevertVS:
		return "BindUndoRevertVS"
	case BindUndoComputeCapacityAllocation:
		return "BindUndoComputeCapacityAllocation"
	case BindUndoReturnCapacityToSPA:
		return "BindUndoReturnCapacityToSPA"
	case BindUndoDone:
		return "BindUndoDone"
	case BindError:
		return "BindError"
	}
	return fmt.Sprintf("bindSubState(%d)", ss)
}

type bindOp struct {
	c                       *Component
	rhs                     *vra.RequestHandlerState
	ops                     bindOperators
	cluster                 *models.Cluster
	reservationCST          *util.CriticalSectionTicket
	inError                 bool
	planOnly                bool
	spaHasCapacity          bool
	spa                     *models.ServicePlanAllocation
	pools                   map[string]*models.Pool // id -> obj
	reservationPlanComputed bool
	reservationPlan         map[string]models.StorageTypeReservation
	totalReservationBytes   int64
	deltaReservationBytes   int64
	isCCOp                  bool
	isUndoOp                bool
	ccHasSizeBytes          bool
	ccHasSpaAdditionalBytes bool
	crrSetDesired           bool
	opName                  string
}

type bindOperators interface {
	getInitialState(ctx context.Context) bindSubState
	loadObjects(ctx context.Context)
	poolLoadAll(ctx context.Context)
	spaCanClaimOwnership(ctx context.Context)
	computeReservationPlan(ctx context.Context)
	spaCheckForCapacity(ctx context.Context)
	spaClearInUse(ctx context.Context)
	spaFetch(ctx context.Context)
	spaReserveCapacity(ctx context.Context)
	spaReturnCapacity(ctx context.Context)
	spaSearch(ctx context.Context)
	spaSetInUse(ctx context.Context)
	vsrSetCapacityReservationResult(ctx context.Context)
	vsSet(ctx context.Context)
	vsUndo(ctx context.Context)
	vsrSaveVSProps(ctx context.Context)
	vsrSetCapacityWait(ctx context.Context)
	vsrUpdate(ctx context.Context)
	waitForLock(ctx context.Context)
}

func (op *bindOp) run(ctx context.Context) {
	jumpToState := BindNoOp
out:
	for ss := op.ops.getInitialState(ctx); !(op.rhs.InError || op.rhs.RetryLater); ss++ {
		if jumpToState != BindNoOp {
			ss = jumpToState
			jumpToState = BindNoOp
		}
		op.c.Log.Debugf("VolumeSeriesRequest %s: %s %s", op.rhs.Request.Meta.ID, op.opName, ss)
		switch ss {
		case BindLoadObjects:
			op.ops.loadObjects(ctx)
		case BindWaitForReservationLock:
			op.ops.waitForLock(ctx)
			if op.rhs.Request.ServicePlanAllocationID != "" {
				jumpToState = BindLoadSPA
			} else if op.isCCOp {
				jumpToState = BindCommitVS // UNBOUND volume
			}
		case BindFindSPA:
			op.ops.spaSearch(ctx)
			if op.spa == nil {
				jumpToState = BindSetCapacityWait
			} else {
				jumpToState = BindPlanCapacityAllocation
			}
		case BindLoadSPA:
			op.ops.spaFetch(ctx)
			if op.spa == nil && !op.rhs.RetryLater {
				jumpToState = BindSetCapacityWait
			}
		case BindCheckAvailabilityOfSPA:
			op.ops.spaCanClaimOwnership(ctx)
		case BindPlanCapacityAllocation:
			op.ops.computeReservationPlan(ctx)
		case BindCheckForCapacity:
			op.ops.spaCheckForCapacity(ctx)
			if !op.spaHasCapacity {
				jumpToState = BindSetCapacityWait
			}
		case BindLoadPools:
			op.ops.poolLoadAll(ctx)
		case BindSetCapacityReservationResult:
			op.ops.vsrSetCapacityReservationResult(ctx)
		case BindUpdateVSRBeforeReservation:
			op.ops.vsrUpdate(ctx)
		case BindReserveCapacityInSPA:
			op.ops.spaReserveCapacity(ctx)
			if !(op.rhs.RetryLater || op.spaHasCapacity) {
				jumpToState = BindSetCapacityWait
			}
		case BindCommitVS:
			op.ops.vsSet(ctx)
		case BindCommitSPA:
			if op.spa != nil {
				op.ops.spaClearInUse(ctx)
			}
			break out
		case BindSetCapacityWait:
			op.ops.vsrSetCapacityWait(ctx)
			break out
		case BindUndoWaitForReservationLock:
			op.ops.waitForLock(ctx)
		case BindUndoLoadSPA:
			op.ops.spaFetch(ctx)
		case BindUndoCheckSPAOwnership:
			op.ops.spaCanClaimOwnership(ctx)
		case BindUndoSetSPAInUse:
			op.ops.spaSetInUse(ctx)
		case BindUndoSaveVSPropsInVSR:
			op.ops.vsrSaveVSProps(ctx)
		case BindUndoRevertVS:
			op.ops.vsUndo(ctx)
		case BindUndoComputeCapacityAllocation:
			op.ops.computeReservationPlan(ctx)
		case BindUndoReturnCapacityToSPA:
			op.ops.spaReturnCapacity(ctx)
			break out
		default:
			break out // any other state breaks the loop
		}
	}
	// release reservation lock if held
	if op.reservationCST != nil {
		op.c.Log.Debugf("VolumeSeriesRequest %s: releasing reservation lock", op.rhs.Request.Meta.ID)
		op.reservationCST.Leave()
	}
	if op.inError {
		op.rhs.InError = true
	}
}

//
// Operators
//

func (op *bindOp) classifyOp() {
	op.isUndoOp = false
	op.isCCOp = false
	op.opName = com.VolReqOpBind
	switch op.rhs.Request.VolumeSeriesRequestState {
	case com.VolReqStateUndoBinding:
		op.isUndoOp = true
	case com.VolReqStateUndoChangingCapacity:
		op.isUndoOp = true
		fallthrough
	case com.VolReqStateChangingCapacity:
		op.isCCOp = true
		op.opName = com.VolReqOpChangeCapacity
		if op.rhs.Request.VolumeSeriesCreateSpec.SizeBytes != nil {
			op.ccHasSizeBytes = true
		}
		if op.rhs.Request.VolumeSeriesCreateSpec.SpaAdditionalBytes != nil {
			op.ccHasSpaAdditionalBytes = true
		}
	}
}

func (op *bindOp) reiProp(suffix string) string {
	if op.isCCOp {
		return "chg-cap-" + suffix
	}
	return "bind-" + suffix
}

func (op *bindOp) getInitialState(ctx context.Context) bindSubState {
	op.classifyOp()
	op.planOnly = swag.BoolValue(op.rhs.Request.PlanOnly)
	if op.isUndoOp {
		if op.planOnly {
			return BindUndoDone
		}
		op.inError = op.rhs.InError
		op.rhs.InError = false // to enable cleanup
		sTag := util.NewTagList(op.rhs.Request.SystemTags)
		if _, found := sTag.Get(com.SystemTagVsrSpaCapacityWait); found {
			op.c.Log.Debugf("VolumeSeriesRequest %s: was in CAPACITY_WAIT - nothing to undo", op.rhs.Request.Meta.ID)
			return BindUndoDone
		}
		if op.rhs.Request.ServicePlanAllocationID == "" {
			if !(op.isCCOp || op.rhs.HasDelete || op.rhs.HasUnbind) {
				return BindUndoDone
			}
			op.rhs.Request.ServicePlanAllocationID = op.rhs.VolumeSeries.ServicePlanAllocationID
			if op.rhs.Request.ServicePlanAllocationID == "" {
				return BindUndoDone
			}
		}
		return BindUndoWaitForReservationLock
	}
	if op.rhs.TimedOut || op.rhs.InError {
		op.rhs.SetRequestError("Invalid invocation of BIND TimedOut=%v InError=%v", op.rhs.TimedOut, op.rhs.InError)
		return BindError // sanity check - should never happen
	}
	if op.isCCOp {
		op.rhs.Request.ServicePlanAllocationID = op.rhs.VolumeSeries.ServicePlanAllocationID // empty if UNBOUND
		return BindWaitForReservationLock
	}
	return BindLoadObjects
}

func (op *bindOp) loadObjects(ctx context.Context) {
	clID := string(op.rhs.Request.ClusterID)
	cl, err := op.c.oCrud.ClusterFetch(ctx, clID)
	if err != nil {
		op.rhs.SetRequestMessage("cluster fetch error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.cluster = cl
}

func (op *bindOp) poolLoadAll(ctx context.Context) {
	pools := make(map[string]*models.Pool)
	// prime with desired ids
	for poolID := range op.spa.StorageReservations {
		pools[poolID] = nil
	}
	lparams := pool.NewPoolListParams()
	lparams.ServicePlanAllocationID = swag.String(string(op.spa.Meta.ID))
	ret, err := op.c.oCrud.PoolList(ctx, lparams)
	if err != nil {
		op.rhs.SetAndUpdateRequestMessage(ctx, "Pool list error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	for _, poolObj := range ret.Payload {
		poolID := string(poolObj.Meta.ID)
		if _, present := pools[poolID]; !present {
			continue // not wanted
		}
		pools[poolID] = poolObj
	}
	// error if all pools not found
	for poolID, obj := range pools {
		if obj == nil {
			op.rhs.SetRequestError("pool [%s] no longer associated with the ServicePlanAllocation", poolID)
			return
		}
	}
	op.pools = pools
}

// spaCanClaimOwnership checks for potential ownership ability; sets RetryLater if in-use and not the owner
func (op *bindOp) spaCanClaimOwnership(ctx context.Context) {
	if op.vsrSpaInUse() && !op.vsrOwnsSpa() {
		op.rhs.RetryLater = true
	}
}

func (op *bindOp) computeReservationPlan(ctx context.Context) {
	f := op.c.App.SFC.FindStorageFormula(op.spa.StorageFormula)
	if f == nil {
		op.rhs.SetRequestError("Invalid storage formula '%s'", op.spa.StorageFormula)
		op.totalReservationBytes, op.deltaReservationBytes = 0, 0
		return
	}

	var curAdjB int64 // current adjustment
	if op.isUndoOp {
		// on the undo path the value is stashed in the VSR and the volume property cleared
		sTag := util.NewTagList(op.rhs.Request.SystemTags)
		if val, found := sTag.Get(com.SystemTagVsrSpaAdditionalBytes); found {
			curAdjB, _ = strconv.ParseInt(val, 10, 64)
		}
	} else {
		curAdjB = swag.Int64Value(op.rhs.VolumeSeries.SpaAdditionalBytes)
	}
	curSizeB := swag.Int64Value(op.rhs.VolumeSeries.SizeBytes) // current volume size
	curPlan := op.planFromFormula(f, curSizeB)                 // current plan
	op.reservationPlan = curPlan
	op.totalReservationBytes = curSizeB
	if op.isCCOp {
		var b strings.Builder
		fmt.Fprintf(&b, "VolumeSeriesRequest %s:", op.rhs.Request.Meta.ID)
		op.deltaReservationBytes = 0
		if op.ccHasSizeBytes { // VS state == BOUND
			newSizeB := swag.Int64Value(op.rhs.Request.VolumeSeriesCreateSpec.SizeBytes) // new volume size
			newPlan := op.planFromFormula(f, newSizeB)                                   // new plan
			deltaSizeB := newSizeB - curSizeB                                            // always positive
			fmt.Fprintf(&b, " sizeBytes:%s (Δ%s)", util.SizeBytes(newSizeB), util.SizeBytes(deltaSizeB))
			op.reservationPlan = newPlan
			op.deltaReservationBytes += deltaSizeB
			op.totalReservationBytes = newSizeB
		}
		if op.ccHasSpaAdditionalBytes { // VS state >= BOUND
			newAdjB := swag.Int64Value(op.rhs.Request.VolumeSeriesCreateSpec.SpaAdditionalBytes) // new adjustment
			deltaAdjB := newAdjB - curAdjB                                                       // can be negative
			fmt.Fprintf(&b, " spaAdditionalBytes:%s (Δ%s)", util.SizeBytes(newAdjB), util.SizeBytes(deltaAdjB))
			op.deltaReservationBytes += deltaAdjB
			op.totalReservationBytes += newAdjB
		}
		fmt.Fprintf(&b, " Δ=%s", util.SizeBytes(op.deltaReservationBytes))
		op.c.Log.Debug(b.String())
	} else {
		if op.isUndoOp {
			op.totalReservationBytes += curAdjB
		} else {
			curAdjB = 0 // ignore when BINDING
		}
		op.deltaReservationBytes = op.totalReservationBytes
		op.c.Log.Debugf("VolumeSeries %s: sizeBytes:%s spaAdditionalBytes:%s Δ=%s", op.rhs.Request.Meta.ID, util.SizeBytes(curSizeB), util.SizeBytes(curAdjB), util.SizeBytes(op.deltaReservationBytes))
	}
}

func (op *bindOp) planFromFormula(f *models.StorageFormula, sizeBytes int64) map[string]models.StorageTypeReservation {
	crpArgs := &centrald.CreateCapacityReservationPlanArgs{
		SizeBytes: sizeBytes,
		SF:        f,
	}
	crp := op.c.App.SFC.CreateCapacityReservationPlan(crpArgs)
	return crp.StorageTypeReservations
}

// spaCheckForCapacity sets spaHasCapacity based on in-use, available capacity and state
func (op *bindOp) spaCheckForCapacity(ctx context.Context) {
	op.spaHasCapacity = false
	if op.vsrSpaInUse() && !op.vsrOwnsSpa() {
		return
	}
	if op.deltaReservationBytes < 0 ||
		(op.spa.ReservationState == com.SPAReservationStateOk &&
			swag.Int64Value(op.spa.ReservableCapacityBytes)-op.deltaReservationBytes >= 0) {
		op.spaHasCapacity = true
	}
}

// spaClearInUse clears the SPA in-use mark in the database
func (op *bindOp) spaClearInUse(ctx context.Context) {
	op.spaSetOrClearInUse(ctx, false)
}

// spaFetch loads the SPA object into op.spa; it sets RetryLater only if the error is not-found.
func (op *bindOp) spaFetch(ctx context.Context) {
	spaID := string(op.rhs.Request.ServicePlanAllocationID)
	spa, err := op.c.oCrud.ServicePlanAllocationFetch(ctx, spaID)
	if err != nil {
		op.rhs.SetRequestMessage("ServicePlanAllocation fetch error: %s", err.Error())
		if err.Error() != com.ErrorNotFound {
			op.rhs.RetryLater = true
		}
		return
	}
	op.spa = spa
}

// spaReserveCapacity obtains Δ capacity from the SPA and marks it in-use; idempotent.
// Note: this is also used in the forward direction to return capacity to the SPA if Δ<0.
// If the SPA no longer can provide capacity (externally modified) then clear the spaHasCapacity flag and do not set RetryLater.
func (op *bindOp) spaReserveCapacity(ctx context.Context) {
	if op.vsrOwnsSpa() {
		return // already updated
	}
	if op.planOnly {
		op.rhs.SetRequestMessage("Plan: Reserve %s (Δ%s) in SPA", util.SizeBytes(op.totalReservationBytes), util.SizeBytes(op.deltaReservationBytes))
		return
	}
	systemTags := []string{op.sTagOp()}
	noCapError := fmt.Errorf("state or capacity changed externally")
	modifyFn := func(o *models.ServicePlanAllocation) (*models.ServicePlanAllocation, error) {
		if err := op.c.rei.ErrOnBool(op.reiProp("fail-in-reserve")); err != nil {
			return nil, err
		}
		if err := op.c.rei.ErrOnBool(op.reiProp("spa-change-in-reserve")); err != nil {
			op.rhs.SetRequestMessage(err.Error())
			return nil, noCapError
		}
		if o == nil {
			o = op.spa
		}
		rcb := swag.Int64Value(o.ReservableCapacityBytes)
		// TotalCapacityBytes and ReservationState can be externally modified, impacting reservable capacity; ignore if returning capacity.
		if op.deltaReservationBytes > 0 && (o.ReservationState != com.SPAReservationStateOk || rcb-op.deltaReservationBytes < 0) {
			return nil, noCapError
		}
		o.SystemTags = systemTags
		o.ReservableCapacityBytes = swag.Int64(rcb - op.deltaReservationBytes)
		return o, nil
	}
	items := &crud.Updates{
		Append: []string{"systemTags"},
		Set:    []string{"reservableCapacityBytes"},
	}
	o, err := op.c.oCrud.ServicePlanAllocationUpdater(ctx, string(op.spa.Meta.ID), modifyFn, items)
	if err != nil {
		op.rhs.SetAndUpdateRequestMessage(ctx, "ServicePlanAllocation update error: %s", err.Error())
		if err == noCapError {
			op.spaHasCapacity = false
		} else {
			op.rhs.RetryLater = true
		}
		return
	}
	op.spa = o
}

// spaReturnCapacity returns the Δ capacity to the SPA and clears the in-use indicator
func (op *bindOp) spaReturnCapacity(ctx context.Context) {
	systemTags := []string{op.sTagOp()}
	modifyFn := func(o *models.ServicePlanAllocation) (*models.ServicePlanAllocation, error) {
		if err := op.c.rei.ErrOnBool(op.reiProp("fail-in-return")); err != nil {
			return nil, err
		}
		if o == nil {
			o = op.spa
		}
		rcb := swag.Int64Value(o.ReservableCapacityBytes)
		o.SystemTags = systemTags
		o.ReservableCapacityBytes = swag.Int64(rcb + op.deltaReservationBytes)
		return o, nil
	}
	items := &crud.Updates{
		Remove: []string{"systemTags"},
		Set:    []string{"reservableCapacityBytes"},
	}
	o, err := op.c.oCrud.ServicePlanAllocationUpdater(ctx, string(op.spa.Meta.ID), modifyFn, items)
	if err != nil {
		op.rhs.SetAndUpdateRequestMessage(ctx, "ServicePlanAllocation update error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.spa = o
}

// spaSearch searches for a SPA object; sets op.spa
func (op *bindOp) spaSearch(ctx context.Context) {
	lparams := service_plan_allocation.NewServicePlanAllocationListParams()
	lparams.AuthorizedAccountID = swag.String(string(op.rhs.VolumeSeries.AccountID))
	lparams.ClusterID = swag.String(string(op.cluster.Meta.ID))
	lparams.ServicePlanID = swag.String(string(op.rhs.VolumeSeries.ServicePlanID))
	ret, err := op.c.oCrud.ServicePlanAllocationList(ctx, lparams)
	if err != nil {
		op.rhs.SetAndUpdateRequestMessage(ctx, "ServicePlanAllocation list error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	if len(ret.Payload) == 1 {
		op.spa = ret.Payload[0]
	}
}

// spaSetInUse sets the SPA in-use mark in the database
func (op *bindOp) spaSetInUse(ctx context.Context) {
	op.spaSetOrClearInUse(ctx, true)
}

// vsrSetCapacityReservationResult saves the current capacity allocation and plans the new capacity allocation.
// It updates the capacityReservationResult in the vsr.
func (op *bindOp) vsrSetCapacityReservationResult(ctx context.Context) {
	// save the current reservation
	crr := &models.CapacityReservationResult{
		CurrentReservations: map[string]models.PoolReservation{},
		DesiredReservations: map[string]models.PoolReservation{},
	}
	for poolID, ca := range op.rhs.VolumeSeries.CapacityAllocations {
		crr.CurrentReservations[poolID] = models.PoolReservation{SizeBytes: ca.ReservedBytes}
	}
	// find the pre-computed per-storageType allocation and set the corresponding per-pool capacity
	for poolID := range op.spa.StorageReservations {
		poolObj := op.pools[poolID]
		sz := int64(0)
		if el, ok := op.reservationPlan[poolObj.CspStorageType]; ok {
			sz = swag.Int64Value(el.SizeBytes) * int64(el.NumMirrors)
		}
		crr.DesiredReservations[poolID] = models.PoolReservation{SizeBytes: swag.Int64(sz)}
	}
	op.rhs.Request.CapacityReservationResult = crr
}

// vsSet sets VolumeSeries capacity and state properties in the database during a BIND operation
// and the appropriate capacity properties during a CHANGE_CAPACITY operation; idempotent
func (op *bindOp) vsSet(ctx context.Context) {
	state := com.VolStateBound
	if op.isCCOp {
		state = ""
	} else if op.rhs.VolumeSeries.VolumeSeriesState == com.VolStateBound {
		return
	}
	op.vsUpdate(ctx, state)
}

// vsUndo reverts changes made to the VolumeSeries; idempotent.
// ForUndoBinding or Deleting it transitions the VolumeSeries to the UNBOUND or DELETING states.
// For VolReqStateUndoChangingCapacity it reverts changes to the sizeBytes and spaAdditionalSizeBytes.
func (op *bindOp) vsUndo(ctx context.Context) {
	state := ""
	if !op.isCCOp {
		if op.rhs.VolumeSeries.VolumeSeriesState != com.VolStateBound {
			return
		}
		state = com.VolStateUnbound
	}
	if op.rhs.HasDelete {
		state = com.VolStateDeleting
	}
	if op.rhs.HasDelete || op.rhs.HasUnbind {
		// clear the "Current" values as the VolumeSeries will be unbound
		if op.rhs.Request.CapacityReservationResult == nil {
			op.rhs.Request.CapacityReservationResult = &models.CapacityReservationResult{}
		}
		op.rhs.Request.CapacityReservationResult.CurrentReservations = map[string]models.PoolReservation{}
	}
	op.vsUpdate(ctx, state)
}

// vsrSaveVSProps saves the volume series spaAdditionalBytes property in the vsr if not already present
func (op *bindOp) vsrSaveVSProps(ctx context.Context) {
	if op.planOnly {
		op.rhs.SetRequestMessage("Plan: Save VS spaAdditionalBytes in VSR")
		return
	}
	sTag := util.NewTagList(op.rhs.Request.SystemTags)
	if _, found := sTag.Get(com.SystemTagVsrSpaAdditionalBytes); !found {
		saB := swag.Int64Value(op.rhs.VolumeSeries.SpaAdditionalBytes)
		sTag.Set(com.SystemTagVsrSpaAdditionalBytes, fmt.Sprintf("%d", saB))
		op.rhs.Request.SystemTags = sTag.List()
		items := &crud.Updates{Set: []string{"systemTags"}}
		op.rhs.UpdateRequestWithItems(ctx, items)
	}
}

// vsrSetCapacityWait sets the state of the VSR to capacity wait in the database
// It sets an indication that CAPACITY_WAIT was entered in the VSR as it impacts
// UNDO logic in case of cancellation.
func (op *bindOp) vsrSetCapacityWait(ctx context.Context) {
	if op.planOnly {
		op.rhs.SetRequestMessage("Plan: Set VSR state to CAPACITY_WAIT")
		return
	}
	sTag := util.NewTagList(op.rhs.Request.SystemTags)
	sTag.Set(com.SystemTagVsrSpaCapacityWait, "")
	op.rhs.Request.SystemTags = sTag.List()
	op.rhs.SetAndUpdateRequestState(ctx, com.VolReqStateCapacityWait) // ignore errors
	op.rhs.RetryLater = true
}

// vsrUpdate updates the VSR in the database; sets the servicePlanAllocationId and storageFormula from op.spa
// It will save the original sizeBytes and spaAdditionalSizeBytes as needed so they can be restored during undo of CHANGE_CAPACITY.
// It clears the CAPACITY_WAIT indication if set.
func (op *bindOp) vsrUpdate(ctx context.Context) {
	if op.spa != nil {
		op.rhs.Request.ServicePlanAllocationID = models.ObjIDMutable(op.spa.Meta.ID)
		op.rhs.Request.StorageFormula = op.spa.StorageFormula // needed by SIZING if planOnly is true
	}
	if op.planOnly {
		op.rhs.SetRequestMessage("Plan: Update VSR (servicePlanAllocationId=[%s] storageFormula=[%s])", op.rhs.Request.ServicePlanAllocationID, op.rhs.Request.StorageFormula)
		return
	}
	sTag := util.NewTagList(op.rhs.Request.SystemTags)
	sTag.Delete(com.SystemTagVsrSpaCapacityWait)
	if op.ccHasSizeBytes {
		sb := swag.Int64Value(op.rhs.VolumeSeries.SizeBytes)
		sTag.Set(com.SystemTagVsrOldSizeBytes, strconv.FormatInt(sb, 10))
	}
	if op.ccHasSpaAdditionalBytes {
		sb := swag.Int64Value(op.rhs.VolumeSeries.SpaAdditionalBytes)
		sTag.Set(com.SystemTagVsrOldSpaAdditionalBytes, strconv.FormatInt(sb, 10))
	}
	op.rhs.Request.SystemTags = sTag.List()
	if err := op.rhs.UpdateRequest(ctx); err != nil { // could set Canceling
		op.rhs.SetRequestMessage("Update error: %s", err.Error()) // unlikely to be saved
		op.rhs.RetryLater = true
	}
}

// waitForLock is called in multiple sub-states
func (op *bindOp) waitForLock(ctx context.Context) {
	op.c.Log.Debugf("VolumeSeriesRequest %s: acquiring reservation lock", op.rhs.Request.Meta.ID)
	cst, err := op.c.reservationCS.Enter(string(op.rhs.Request.Meta.ID))
	if err != nil {
		op.rhs.SetRequestMessage("reservation lock error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.c.Log.Debugf("VolumeSeriesRequest %s: obtained reservation lock", op.rhs.Request.Meta.ID)
	op.reservationCST = cst
}

//
// Helpers
//

func (op *bindOp) sTagOp() string {
	return fmt.Sprintf("%s:%s", com.SystemTagVsrOperator, op.rhs.Request.Meta.ID)
}

func (op *bindOp) vsrOwnsSpa() bool {
	return util.Contains(op.spa.SystemTags, op.sTagOp())
}

func (op *bindOp) vsrClaimSpa() {
	if op.spa.SystemTags == nil {
		op.spa.SystemTags = make([]string, 0, 1)
	}
	op.spa.SystemTags = append(op.spa.SystemTags, op.sTagOp())
}

func (op *bindOp) vsrDisownSpa() {
	ownerTag := op.sTagOp()
	stags := make([]string, 0, len(op.spa.SystemTags))
	for _, st := range op.spa.SystemTags {
		if st != ownerTag {
			stags = append(stags, st)
		}
	}
	op.spa.SystemTags = stags
}

func (op *bindOp) vsrSpaInUse() bool {
	if op.spa.SystemTags != nil {
		prefix := com.SystemTagVsrOperator + ":"
		for _, st := range op.spa.SystemTags {
			if strings.HasPrefix(st, prefix) {
				return true
			}
		}
	}
	return false
}

func (op *bindOp) spaSetOrClearInUse(ctx context.Context, set bool) {
	systemTags := []string{op.sTagOp()}
	modProps := []string{"systemTags"}
	items := &crud.Updates{}
	injectProp := op.reiProp("fail-clear-in-use")
	var plan string
	if set {
		injectProp = op.reiProp("fail-set-in-use")
		items.Append = modProps
		plan = "in-use"
	} else {
		items.Remove = modProps
		plan = "not in-use"
	}
	if op.planOnly {
		op.rhs.SetRequestMessage("Plan: Mark ServicePlanAllocation object %s", plan)
		return
	}
	modifyFn := func(o *models.ServicePlanAllocation) (*models.ServicePlanAllocation, error) {
		if err := op.c.rei.ErrOnBool(injectProp); err != nil {
			return nil, err
		}
		if o == nil {
			o = op.spa
		}
		o.SystemTags = systemTags
		return o, nil
	}
	o, err := op.c.oCrud.ServicePlanAllocationUpdater(ctx, string(op.spa.Meta.ID), modifyFn, items)
	if err != nil {
		op.rhs.SetAndUpdateRequestMessage(ctx, "ServicePlanAllocation update error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.spa = o
}

func (op *bindOp) vsUpdate(ctx context.Context, state string) {
	if op.planOnly {
		op.rhs.SetRequestMessage("Plan: Commit changes to VolumeSeries")
		return
	}
	var bcID, spaID models.ObjIDMutable
	var injectProp string
	var setItems []string
	var sb, sab int64

	if op.isCCOp {
		injectProp = op.reiProp("fail-in-vs-update")
		setItems = []string{"messages"}
		sb = swag.Int64Value(op.rhs.Request.VolumeSeriesCreateSpec.SizeBytes)
		sab = swag.Int64Value(op.rhs.Request.VolumeSeriesCreateSpec.SpaAdditionalBytes)
		if op.isUndoOp {
			sTag := util.NewTagList(op.rhs.Request.SystemTags)
			hasTag := false
			if val, ok := sTag.Get(com.SystemTagVsrOldSizeBytes); ok {
				hasTag = true
				sb, _ = strconv.ParseInt(val, 10, 64)
			} else {
				sb = swag.Int64Value(op.rhs.VolumeSeries.SizeBytes)
			}
			if val, ok := sTag.Get(com.SystemTagVsrOldSpaAdditionalBytes); ok {
				hasTag = true
				sab, _ = strconv.ParseInt(val, 10, 64)
			} else {
				sab = swag.Int64Value(op.rhs.VolumeSeries.SpaAdditionalBytes)
			}
			if !hasTag || (sb == swag.Int64Value(op.rhs.VolumeSeries.SizeBytes) && sab == swag.Int64Value(op.rhs.VolumeSeries.SpaAdditionalBytes)) {
				// either the VS change was never committed, or it is already undone
				return
			}
		}
		if op.ccHasSizeBytes { // VS state is BOUND or UNBOUND
			if !op.isUndoOp && op.rhs.Request.CapacityReservationResult != nil && len(op.rhs.Request.CapacityReservationResult.DesiredReservations) > 0 { // BOUND
				op.crrSetDesired = true
				setItems = append(setItems, "sizeBytes", "capacityAllocations")
			} else { // UNBOUND or undo
				setItems = append(setItems, "sizeBytes")
			}
		}
		if op.ccHasSpaAdditionalBytes { // VS state is >= BOUND
			setItems = append(setItems, "spaAdditionalBytes")
		}
	} else {
		setItems = []string{"volumeSeriesState", "messages", "boundClusterId", "servicePlanAllocationId", "capacityAllocations", "systemTags", "spaAdditionalBytes", "lifecycleManagementData"}
		if op.isUndoOp {
			injectProp = op.reiProp("fail-in-vs-set-unbound")
		} else {
			bcID = models.ObjIDMutable(op.cluster.Meta.ID)
			spaID = models.ObjIDMutable(op.spa.Meta.ID)
			injectProp = op.reiProp("fail-in-vs-set-bound")
			op.crrSetDesired = true
		}
	}
	modifyFn := func(vs *models.VolumeSeries) (*models.VolumeSeries, error) {
		if err := op.c.rei.ErrOnBool(injectProp); err != nil {
			return nil, err
		}
		if vs == nil {
			vs = op.rhs.VolumeSeries
		}
		msgList := util.NewMsgList(vs.Messages)
		if !op.isCCOp {
			vs.BoundClusterID = bcID
			vs.ServicePlanAllocationID = spaID
			vs.SpaAdditionalBytes = swag.Int64(0) // reset
			msgList.Insert("State change %s ⇒ %s", vs.VolumeSeriesState, state)
			vs.VolumeSeriesState = state
			sTag := util.NewTagList(vs.SystemTags)
			for _, k := range sTag.Keys(bindRemoveTagRe) {
				sTag.Delete(k)
			}
			vs.SystemTags = sTag.List()
			vs.LifecycleManagementData = nil // BOUND or UNBOUND
		} else {
			if op.ccHasSizeBytes {
				msgList.Insert("sizeBytes change %s ⇒ %s", util.SizeBytes(swag.Int64Value(vs.SizeBytes)), util.SizeBytes(sb))
				vs.SizeBytes = swag.Int64(sb)
			}
			if op.ccHasSpaAdditionalBytes {
				msgList.Insert("spaAdditionalBytes change %s ⇒ %s", util.SizeBytes(swag.Int64Value(vs.SpaAdditionalBytes)), util.SizeBytes(sab))
				vs.SpaAdditionalBytes = swag.Int64(sab)
			}
		}
		cam := vs.CapacityAllocations
		if state == com.VolStateUnbound || state == com.VolStateDeleting {
			cam = nil // clear the map to unpin the pool objects
		} else {
			if cam == nil {
				cam = map[string]models.CapacityAllocation{}
			}
			if op.crrSetDesired {
				for spID, spr := range op.rhs.Request.CapacityReservationResult.DesiredReservations {
					if _, ok := cam[spID]; !ok {
						cam[spID] = models.CapacityAllocation{}
					}
					ca := cam[spID]
					ca.ReservedBytes = spr.SizeBytes
					cam[spID] = ca
				}
			}
		}
		vs.CapacityAllocations = cam
		vs.Messages = msgList.ToModel()
		return vs, nil
	}
	items := &crud.Updates{Set: setItems}
	obj, err := op.c.oCrud.VolumeSeriesUpdater(ctx, string(op.rhs.VolumeSeries.Meta.ID), modifyFn, items)
	if err != nil {
		op.rhs.SetAndUpdateRequestMessage(ctx, "VolumeSeries update error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.rhs.VolumeSeries = obj
}
