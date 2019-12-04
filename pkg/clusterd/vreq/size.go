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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_formula"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/layout"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/swag"
)

// sizeSubState represents sizing sub-states.
// There is at most one database update in each sub-state.
type sizeSubState int

// sizeSubState values
const (
	// Populate the Request.storagePlan object
	SizeSetStoragePlan sizeSubState = iota
	// Return
	SizeDone
	// clear the Request.storagePlan object
	SizeCleanup
	// Return
	SizeUndoDone

	// LAST: No operation is performed in this state.
	SizeNoOp
)

func (ss sizeSubState) String() string {
	switch ss {
	case SizeSetStoragePlan:
		return "SizeSetStoragePlan"
	case SizeDone:
		return "SizeDone"
	case SizeCleanup:
		return "SizeCleanup"
	case SizeUndoDone:
		return "SizeUndoDone"
	}
	return fmt.Sprintf("sizeSubState(%d)", ss)
}

type sizeOperators interface {
	getInitialState(ctx context.Context) sizeSubState
	setStoragePlan(ctx context.Context)
	cleanup(ctx context.Context)
}

type sizeOp struct {
	c       *Component
	rhs     *vra.RequestHandlerState
	ops     sizeOperators
	inError bool
}

// vra.AllocationHandlers methods

// Size implements the SIZING state of a VolumeSeriesRequest. It uses the VolumeSeries capacityAllocations and ServicePlanAllocation StorageFormula.
// When planOnly with an accompanying BIND operation, the Request capacityReservationResult and storageFormula are used instead.
func (c *Component) Size(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &sizeOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// UndoSize reverses changes made by Size
func (c *Component) UndoSize(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &sizeOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// ResizeCache implements the RESIZING_CACHE state of a VolumeSeriesRequest based on the VolumeSeries spaAdditionalBytes.
// When planOnly the Request volumeSeriesCreateSpec.spaAdditionalBytes is used instead.
func (c *Component) ResizeCache(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &sizeOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// UndoResizeCache reverses changes made by ResizeCache
func (c *Component) UndoResizeCache(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &sizeOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// run executes the state machine
func (op *sizeOp) run(ctx context.Context) {
out:
	for ss := op.ops.getInitialState(ctx); !(op.rhs.InError || op.rhs.RetryLater); ss++ {
		op.c.Log.Debugf("VolumeSeriesRequest %s: %s %s", op.rhs.Request.Meta.ID, op.rhs.Request.VolumeSeriesRequestState, ss)
		switch ss {
		case SizeSetStoragePlan:
			op.ops.setStoragePlan(ctx)
		// case SizeDone takes default
		case SizeCleanup:
			op.ops.cleanup(ctx)
		// case SizeUndoDone takes default
		default:
			break out
		}
	}
	if op.inError {
		op.rhs.InError = true
	}
}

func (op *sizeOp) getInitialState(ctx context.Context) sizeSubState {
	if op.rhs.Request.VolumeSeriesRequestState == com.VolReqStateUndoSizing || op.rhs.Request.VolumeSeriesRequestState == com.VolReqStateUndoResizingCache {
		if swag.BoolValue(op.rhs.Request.PlanOnly) || vra.VolumeSeriesIsProvisioned(op.rhs.VolumeSeries.VolumeSeriesState) {
			// state is set to PROVISIONED in downstream configure step, so if we are provisioned, nothing to do in undo direction
			return SizeUndoDone
		}
		op.inError = op.rhs.InError
		op.rhs.InError = false // reset so state machine can run
		// StoragePlan is initialized in a single update, only needs to check 1 property
		if op.rhs.Request.StoragePlan != nil && len(op.rhs.Request.StoragePlan.StorageElements) > 0 {
			return SizeCleanup
		}
		return SizeUndoDone
	}
	if err := op.c.rei.ErrOnBool("size-block-on-start"); err != nil {
		op.rhs.SetAndUpdateRequestMessageDistinct(ctx, "Blocked on [%s]", err.Error())
		op.rhs.RetryLater = true
		return SizeDone
	}
	if op.rhs.TimedOut {
		return SizeCleanup
	}
	if op.rhs.Canceling || op.rhs.InError {
		return SizeDone
	}
	// StoragePlan is initialized in a single update, only needs to check 1 property
	if op.rhs.Request.StoragePlan != nil && len(op.rhs.Request.StoragePlan.StorageElements) > 0 {
		return SizeDone
	}
	return SizeSetStoragePlan
}

func (op *sizeOp) setStoragePlan(ctx context.Context) {
	planOnly := swag.BoolValue(op.rhs.Request.PlanOnly)
	formulaName := op.rhs.Request.StorageFormula
	if !planOnly || !(op.rhs.HasBind || op.rhs.HasChangeCapacity) {
		planOnly = false
		spa, err := op.c.oCrud.ServicePlanAllocationFetch(ctx, string(op.rhs.VolumeSeries.ServicePlanAllocationID))
		if err != nil {
			// assume transient error, update unlikely to be saved
			op.rhs.SetAndUpdateRequestMessage(ctx, "ServicePlanAllocation[%s] fetch failure: %s", op.rhs.VolumeSeries.ServicePlanAllocationID, err.Error())
			op.rhs.RetryLater = true
			return
		}
		formulaName = spa.StorageFormula
	}
	lparams := storage_formula.NewStorageFormulaListParams()
	lparams.Name = swag.String(string(formulaName))
	formulas, err := op.c.oCrud.StorageFormulaList(ctx, lparams)
	if err != nil {
		// assume transient error, update unlikely to be saved
		op.rhs.SetAndUpdateRequestMessage(ctx, "StorageFormula %s load failure: %s", formulaName, err.Error())
		op.rhs.RetryLater = true
		return
	} else if len(formulas.Payload) != 1 {
		op.rhs.SetRequestError("StorageFormula %s not found", formulaName)
		return
	}
	formula := formulas.Payload[0]
	plan := &models.StoragePlan{}
	hasCache := false
	for _, v := range formula.CacheComponent {
		if swag.Int32Value(v.Percentage) > 0 {
			hasCache = true
			break
		}
	}
	if hasCache && op.rhs.HasVolSnapshotRestore {
		op.rhs.SetRequestMessage("%s: ignored requested cache due to %s operation", com.VolReqStateSizing, com.VolReqOpVolRestoreSnapshot)
		hasCache = false
	}
	if vra.VolumeSeriesIsProvisioned(op.rhs.VolumeSeries.VolumeSeriesState) {
		if !hasCache {
			// state is set to PROVISIONED in downstream configure step, so if we are provisioned and no cache required, nothing to do
			return
		}
	} else {
		var layoutAlgorithm *layout.Algorithm
		// fetch the layout algorithm if recorded in the VS
		if op.rhs.VolumeSeries.LifecycleManagementData != nil && op.rhs.VolumeSeries.LifecycleManagementData.LayoutAlgorithm != "" {
			la, err := layout.FindAlgorithm(op.rhs.VolumeSeries.LifecycleManagementData.LayoutAlgorithm)
			if err != nil {
				op.rhs.SetRequestError("Invalid layout algorithm '%s' recorded in VolumeSeries", op.rhs.VolumeSeries.LifecycleManagementData.LayoutAlgorithm)
				return
			}
			layoutAlgorithm = la
			plan.StorageLayout = layoutAlgorithm.StorageLayout
		} else {
			plan.StorageLayout = models.StorageLayoutStandalone
			if formula.StorageLayout != models.StorageLayoutStandalone {
				// TBD use formula.StorageLayout or perhaps some other TBD formula property
				op.rhs.SetRequestMessage("%s: ignored requested storageLayout %s, using layout %s", com.VolReqStateSizing, string(formula.StorageLayout), plan.StorageLayout)
			}
			layoutAlgorithm = op.c.App.StateOps.FindLayoutAlgorithm(plan.StorageLayout)
			if layoutAlgorithm == nil {
				op.rhs.SetRequestError("No placement algorithm for layout '%s'", plan.StorageLayout)
				return
			}
		}
		plan.LayoutAlgorithm = layoutAlgorithm.Name
		// addStorageElementToPlan adds a storage element for the given pool in the specified amount if the amount is positive
		addStorageElementToPlan := func(poolID string, amount int64) {
			if amount > 0 {
				elem := &models.StoragePlanStorageElement{
					Intent:    com.VolReqStgElemIntentData,
					SizeBytes: swag.Int64(amount),
					StorageParcels: map[string]models.StorageParcelElement{
						com.VolReqStgPseudoParcelKey: {SizeBytes: swag.Int64(amount)},
					},
					PoolID: models.ObjIDMutable(poolID),
				}
				plan.StorageElements = append(plan.StorageElements, elem)
			}
		}
		if planOnly {
			for poolID, a := range op.rhs.Request.CapacityReservationResult.DesiredReservations {
				amount := swag.Int64Value(a.SizeBytes)
				addStorageElementToPlan(poolID, amount)
			}
		} else {
			// For now, allocate all reserved storage that is not already consumed
			for poolID, a := range op.rhs.VolumeSeries.CapacityAllocations {
				amount := swag.Int64Value(a.ReservedBytes) - swag.Int64Value(a.ConsumedBytes)
				addStorageElementToPlan(poolID, amount)
			}
		}
	}
	if hasCache {
		storageTypes := op.c.App.CSP.SupportedCspStorageTypes()
		totalAmount := swag.Int64Value(op.rhs.VolumeSeries.SizeBytes)
		if planOnly && op.rhs.HasChangeCapacity {
			totalAmount += swag.Int64Value(op.rhs.Request.VolumeSeriesCreateSpec.SpaAdditionalBytes)
		} else {
			totalAmount += swag.Int64Value(op.rhs.VolumeSeries.SpaAdditionalBytes)
		}
		for k, v := range formula.CacheComponent {
			if amount := (totalAmount*int64(swag.Int32Value(v.Percentage)) + 99) / 100; amount > 0 {
				key := com.VolReqStgSSDParcelKey
				for _, st := range storageTypes {
					if k == string(st.Name) {
						if value, ok := st.CspStorageTypeAttributes[csp.CSPEphemeralStorageType]; ok {
							key = value.Value
							break
						}
					}
				}
				elem := &models.StoragePlanStorageElement{
					Intent:    com.VolReqStgElemIntentCache,
					SizeBytes: swag.Int64(amount),
					StorageParcels: map[string]models.StorageParcelElement{
						key: {ProvMinSizeBytes: swag.Int64(0)}, // TBD use a value from the StorageFormula
					},
				}
				plan.StorageElements = append(plan.StorageElements, elem)
			}
		}
	}

	if !op.rhs.InError {
		op.rhs.Request.StoragePlan = plan
		if err = op.rhs.UpdateRequest(ctx); err != nil {
			// unlikely this would ever be saved
			op.rhs.SetRequestError("Failed to update VolumeSeriesRequest object: %s", err.Error())
		}
	}
}

func (op *sizeOp) cleanup(ctx context.Context) {
	// if any storage was actually allocated, it should have been released by the downstream step(s)
	op.rhs.Request.StoragePlan = &models.StoragePlan{
		PlacementHints:  make(map[string]models.ValueType, 0),
		StorageElements: make([]*models.StoragePlanStorageElement, 0),
	}
}
