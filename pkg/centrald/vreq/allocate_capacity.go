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
	"strings"

	client_pool "github.com/Nuvoloso/kontroller/pkg/autogen/client/pool"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan_allocation"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/swag"
)

// acSubState tracks Allocate Capacity sub-states
type acSubState int

// acSubState values (the ordering is meaningful)
const (
	// Load the ancillary objects
	ACLoadObjects acSubState = iota
	// Enter the critical section.
	// The reservation lock is released on return from the handler.
	ACWaitForReservationLock
	// Find the SPA object.
	ACFindSPA
	// Fail if the SPA in use by some other VSR, or the input request is invalid.
	ACValidateVSRAgainstSPA
	// Determine the appropriate StorageFormula to use and make the capacityReservationPlan (CRP) if not already done so
	ACMakePlan
	// Update the formula and capacityReservationPlan (CRP) in the VSR
	ACUpdatePlanInVSR
	// Create the SPA - if creating mark it in use
	ACCreateSPA
	// Mark the SPA object in use by this VSR if not already set.
	ACMarkSPAInUse
	// Find Pool objects for the authorized account and cluster
	ACFindPools
	// Create remaining Pool objects for the authorized account, cluster and storage type
	ACCreatePools
	// Initialize the capacityReservationResult (CRR) in the VSR and update
	ACInitializeCRRInVSR
	// Add capacity in the Pool objects
	ACAddCapacityToPools
	// Update the ServicePlan with the authorized account
	ACUpdateServicePlan
	// Finalize SPA object
	ACFinalizeSpa
	// Terminal state
	ACDone

	//
	// UNDO path
	//
	// Search for the SPA object
	ACUndoFindSPA
	// Load Objects for UNDO path
	ACUndoLoadObjects
	// Enter the critical section.
	// The reservation lock is released on return from the handler.
	ACUndoWaitForLock
	// Find Pool objects for the authorized account and cluster
	ACUndoFindPools
	// Remove capacity from Pool objects
	ACUndoRemoveCapacityFromPools
	// Remove StorageReservations from the SPA (so pools can be deleted), delete SPA path
	ACRemoveStorageReservations
	// Remove references to empty Pool objects from the Request
	ACUndoRemoveEmptyPoolReferencesInRequest
	// Delete empty Pool objects
	ACUndoDeletePools
	// Clear the reference to the SPA in the Request
	ACUndoClearSpaInRequest
	// Release the SPA object; delete if created by this VSR.
	ACUndoReleaseSPA
	// Remove account from service plan
	ACUndoRemoveAccountFromSDC
	// Terminal state
	ACUndoDone

	// error
	ACError
	// LAST: No operation is performed in this state.
	ACNoOp
)

func (ss acSubState) String() string {
	switch ss {
	case ACLoadObjects:
		return "ACLoadObjects"
	case ACWaitForReservationLock:
		return "ACWaitForReservationLock"
	case ACFindSPA:
		return "ACFindSPA"
	case ACValidateVSRAgainstSPA:
		return "ACValidateVSRAgainstSPA"
	case ACMakePlan:
		return "ACMakePlan"
	case ACUpdatePlanInVSR:
		return "ACUpdatePlanInVSR"
	case ACCreateSPA:
		return "ACCreateSPA"
	case ACMarkSPAInUse:
		return "ACMarkSPAInUse"
	case ACFindPools:
		return "ACFindPools"
	case ACCreatePools:
		return "ACCreatePools"
	case ACInitializeCRRInVSR:
		return "ACInitializeCRRInVSR"
	case ACAddCapacityToPools:
		return "ACAddCapacityToPools"
	case ACUpdateServicePlan:
		return "ACUpdateServicePlan"
	case ACFinalizeSpa:
		return "ACFinalizeSpa"
	case ACDone:
		return "ACDone"
	case ACUndoFindSPA:
		return "ACUndoFindSPA"
	case ACUndoLoadObjects:
		return "ACUndoLoadObjects"
	case ACUndoWaitForLock:
		return "ACUndoWaitForLock"
	case ACUndoFindPools:
		return "ACUndoFindPools"
	case ACUndoRemoveCapacityFromPools:
		return "ACUndoRemoveCapacityFromPools"
	case ACRemoveStorageReservations:
		return "ACRemoveStorageReservations"
	case ACUndoRemoveEmptyPoolReferencesInRequest:
		return "ACUndoRemoveEmptyPoolReferencesInRequest"
	case ACUndoDeletePools:
		return "ACUndoDeletePools"
	case ACUndoClearSpaInRequest:
		return "ACUndoClearSpaInRequest"
	case ACUndoReleaseSPA:
		return "ACUndoReleaseSPA"
	case ACUndoRemoveAccountFromSDC:
		return "ACUndoRemoveAccountFromSDC"
	case ACUndoDone:
		return "ACUndoDone"
	case ACError:
		return "ACError"
	}
	return fmt.Sprintf("acSubState(%d)", ss)
}

type acOperators interface {
	addAccountToSDC(ctx context.Context)
	addCapacityToPools(ctx context.Context)
	clearSpaIDInRequest(ctx context.Context)
	createPools(ctx context.Context)
	createSPA(ctx context.Context)
	deletePools(ctx context.Context)
	deleteSPA(ctx context.Context)
	finalizeSPA(ctx context.Context)
	findPools(ctx context.Context)
	findSPA(ctx context.Context)
	getInitialState(ctx context.Context) acSubState
	initializeCRR(ctx context.Context)
	loadObjects(ctx context.Context)
	markSPAInUse(ctx context.Context)
	releaseSPA(ctx context.Context)
	removeAccountFromSDC(ctx context.Context)
	removeCapacityFromPools(ctx context.Context)
	removeEmptyPoolReferences(ctx context.Context)
	removeStorageReservations(ctx context.Context)
	selectStorageFormula(ctx context.Context)
	setPoolCapacityForSPA(ctx context.Context, args *SetCapacityArgs) (*models.Pool, error)
	updateRequest(ctx context.Context)
	validateVSRAgainstSPA(ctx context.Context)
	waitForLock(ctx context.Context)
}

// AllocateCapacity satisfies the vra.AllocateCapacityHandlers interface
func (c *Component) AllocateCapacity(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &acOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// UndoAllocateCapacity satisfies the vra.AllocateCapacityHandlers interface
func (c *Component) UndoAllocateCapacity(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &acOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

type acOp struct {
	c              *Component
	rhs            *vra.RequestHandlerState
	ops            acOperators
	inError        bool
	planOnly       bool
	planningDone   bool
	cluster        *models.Cluster
	domain         *models.CSPDomain
	sp             *models.ServicePlan
	spa            *models.ServicePlanAllocation
	reservationCST *util.CriticalSectionTicket
	pools          map[string]*models.Pool // cst -> pool
}

func (op *acOp) sTagCreator() string {
	return fmt.Sprintf("%s:%s", com.SystemTagVsrCreator, op.rhs.Request.Meta.ID)
}

func (op *acOp) sTagOp() string {
	return fmt.Sprintf("%s:%s", com.SystemTagVsrOperator, op.rhs.Request.Meta.ID)
}

func (op *acOp) vsrCreateSpaSysTag() string {
	return fmt.Sprintf("%s:%s", com.SystemTagVsrCreateSPA, op.rhs.Request.Meta.ID)
}

func (op *acOp) vsrOwnsSpa() bool {
	return util.Contains(op.spa.SystemTags, op.sTagOp())
}

func (op *acOp) vsrCreatedSpa() bool {
	return util.Contains(op.spa.SystemTags, op.sTagCreator())
}

func (op *acOp) vsrCreateSpaTagInRequest() bool {
	return util.Contains(op.rhs.Request.SystemTags, op.vsrCreateSpaSysTag())
}

func (op *acOp) run(ctx context.Context) {
	jumpToState := ACNoOp
out:
	for ss := op.ops.getInitialState(ctx); !(op.rhs.InError || op.rhs.RetryLater); ss++ {
		if jumpToState != ACNoOp {
			ss = jumpToState
			jumpToState = ACNoOp
		}
		op.c.Log.Debugf("VolumeSeriesRequest %s: %s %s", op.rhs.Request.Meta.ID, op.rhs.Request.VolumeSeriesRequestState, ss)
		switch ss {
		case ACLoadObjects:
			op.ops.loadObjects(ctx)
		case ACWaitForReservationLock:
			op.ops.waitForLock(ctx)
		case ACFindSPA:
			op.ops.findSPA(ctx)
			if op.spa == nil {
				if op.rhs.HasDeleteSPA {
					jumpToState = ACUndoRemoveAccountFromSDC
				} else if !op.planningDone {
					jumpToState = ACMakePlan
				} else {
					jumpToState = ACCreateSPA
				}
			}
		case ACValidateVSRAgainstSPA:
			op.ops.validateVSRAgainstSPA(ctx)
			if op.planningDone {
				jumpToState = ACMarkSPAInUse
			}
		case ACMakePlan:
			op.ops.selectStorageFormula(ctx)
		case ACUpdatePlanInVSR:
			op.ops.updateRequest(ctx)
			if op.spa != nil {
				jumpToState = ACMarkSPAInUse
			}
		case ACCreateSPA:
			op.ops.createSPA(ctx)
			jumpToState = ACFindPools
		case ACMarkSPAInUse:
			op.ops.markSPAInUse(ctx)
		case ACFindPools:
			op.ops.findPools(ctx)
			if op.planOnly && op.rhs.HasDeleteSPA {
				jumpToState = ACUndoRemoveAccountFromSDC
			}
		case ACCreatePools:
			op.ops.createPools(ctx)
		case ACInitializeCRRInVSR:
			if op.planOnly {
				break out
			}
			if op.rhs.Request.CapacityReservationResult == nil || len(op.rhs.Request.CapacityReservationResult.DesiredReservations) == 0 {
				op.ops.initializeCRR(ctx)
			}
			op.ops.updateRequest(ctx)
		case ACAddCapacityToPools:
			op.ops.addCapacityToPools(ctx)
			if op.rhs.HasDeleteSPA {
				jumpToState = ACRemoveStorageReservations
			}
		case ACUpdateServicePlan:
			op.ops.addAccountToSDC(ctx)
		case ACFinalizeSpa:
			op.ops.finalizeSPA(ctx)
		case ACUndoFindSPA:
			op.ops.findSPA(ctx)
			if op.spa == nil || !(op.vsrOwnsSpa() || op.vsrCreatedSpa()) { // must undo if either owner or creator
				jumpToState = ACUndoDone
			}
		case ACUndoLoadObjects:
			op.ops.loadObjects(ctx)
		case ACUndoWaitForLock:
			op.ops.waitForLock(ctx)
		case ACUndoFindPools:
			op.ops.findPools(ctx)
		case ACUndoRemoveCapacityFromPools:
			op.ops.removeCapacityFromPools(ctx)
			jumpToState = ACUndoRemoveEmptyPoolReferencesInRequest
		case ACRemoveStorageReservations:
			op.ops.removeStorageReservations(ctx)
		case ACUndoRemoveEmptyPoolReferencesInRequest:
			op.ops.removeEmptyPoolReferences(ctx)
		case ACUndoDeletePools:
			op.ops.deletePools(ctx)
		case ACUndoClearSpaInRequest:
			op.ops.clearSpaIDInRequest(ctx)
		case ACUndoReleaseSPA:
			if op.vsrCreatedSpa() || op.rhs.HasDeleteSPA {
				op.ops.deleteSPA(ctx)
			} else {
				op.ops.releaseSPA(ctx)
			}
		case ACUndoRemoveAccountFromSDC:
			if op.vsrCreateSpaTagInRequest() || op.rhs.HasDeleteSPA {
				op.ops.removeAccountFromSDC(ctx)
			}
		default:
			break out
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

func (op *acOp) getInitialState(ctx context.Context) acSubState {
	op.planOnly = swag.BoolValue(op.rhs.Request.PlanOnly)
	if op.rhs.Request.VolumeSeriesRequestState == com.VolReqStateUndoAllocatingCapacity {
		if op.planOnly {
			return ACUndoDone
		}
		op.inError = op.rhs.InError
		op.rhs.InError = false // clear to process
		return ACUndoFindSPA
	}
	if op.rhs.TimedOut || op.rhs.InError {
		op.rhs.SetRequestError("Invalid invocation of %s TimedOut=%v InError=%v", op.rhs.Request.RequestedOperations[0], op.rhs.TimedOut, op.rhs.InError)
		return ACError // sanity check - should never happen
	}
	if op.rhs.Request.CapacityReservationPlan != nil &&
		len(op.rhs.Request.CapacityReservationPlan.StorageTypeReservations) > 0 &&
		op.rhs.Request.ServicePlanAllocationCreateSpec.StorageFormula != "" {
		op.planningDone = true
	}
	return ACLoadObjects
}

func (op *acOp) loadObjects(ctx context.Context) {
	clID := string(op.rhs.Request.ServicePlanAllocationCreateSpec.ClusterID)
	cl, err := op.c.oCrud.ClusterFetch(ctx, clID)
	if err != nil {
		op.rhs.SetRequestMessage("cluster fetch error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	domID := string(cl.CspDomainID)
	dom, err := op.c.oCrud.CSPDomainFetch(ctx, domID)
	if err != nil {
		op.rhs.SetRequestMessage("domain fetch error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	spID := string(op.rhs.Request.ServicePlanAllocationCreateSpec.ServicePlanID)
	sp, err := op.c.oCrud.ServicePlanFetch(ctx, spID)
	if err != nil {
		op.rhs.SetRequestMessage("service plan fetch error: %s", err.Error())
		op.rhs.RetryLater = true
	}
	op.cluster = cl
	op.domain = dom
	op.sp = sp
}

// updateRequest is called in multiple sub-states
func (op *acOp) updateRequest(ctx context.Context) {
	if err := op.rhs.UpdateRequest(ctx); err != nil { // could set Canceling
		op.rhs.SetRequestMessage("Update error: %s", err.Error()) // unlikely to be saved
		op.rhs.RetryLater = true
	}
}

// waitForLock is called in multiple sub-states
func (op *acOp) waitForLock(ctx context.Context) {
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

func (op *acOp) selectStorageFormula(ctx context.Context) {
	sfs, err := op.c.App.SFC.StorageFormulasForServicePlan(ctx, op.rhs.Request.ServicePlanAllocationCreateSpec.ServicePlanID, nil)
	if err != nil {
		op.rhs.SetRequestMessage("storage formula error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	if len(sfs) == 0 {
		op.rhs.SetRequestError("no applicable storage formula found")
		return
	}
	choices := []string{}
	for _, sf := range sfs {
		choices = append(choices, string(sf.Name))
	}
	sf, err := op.c.App.SFC.SelectFormulaForDomain(op.domain, sfs)
	if err != nil {
		op.rhs.SetRequestError("unable to select storage formula: %s", err.Error())
		return
	}
	op.rhs.SetRequestMessage("Selected storageFormula %s from %v", sf.Name, choices)
	crpArgs := &centrald.CreateCapacityReservationPlanArgs{
		SizeBytes: swag.Int64Value(op.rhs.Request.ServicePlanAllocationCreateSpec.TotalCapacityBytes),
		SF:        sf,
	}
	op.rhs.Request.CapacityReservationPlan = op.c.App.SFC.CreateCapacityReservationPlan(crpArgs)
	op.rhs.Request.StorageFormula = sf.Name
}

func (op *acOp) findSPA(ctx context.Context) {
	cs := op.rhs.Request.ServicePlanAllocationCreateSpec
	lparams := service_plan_allocation.NewServicePlanAllocationListParams()
	lparams.AuthorizedAccountID = swag.String(string(cs.AuthorizedAccountID))
	lparams.ClusterID = swag.String(string(cs.ClusterID))
	lparams.ServicePlanID = swag.String(string(cs.ServicePlanID))
	ret, err := op.c.oCrud.ServicePlanAllocationList(ctx, lparams)
	if err != nil {
		op.rhs.SetAndUpdateRequestMessage(ctx, "ServicePlanAllocation list error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	if len(ret.Payload) == 1 {
		op.spa = ret.Payload[0]
		op.rhs.Request.ServicePlanAllocationID = models.ObjIDMutable(op.spa.Meta.ID)
	}
}

func (op *acOp) validateVSRAgainstSPA(ctx context.Context) {
	if op.vsrOwnsSpa() {
		return
	}
	for _, stag := range op.spa.SystemTags {
		if strings.HasPrefix(stag, com.SystemTagVsrOperator) {
			vsrID := strings.TrimPrefix(stag, com.SystemTagVsrOperator+":")
			if op.vsrIsActive(ctx, vsrID) {
				op.rhs.SetRequestError("ServicePlanAllocation being processed by request [%s]", vsrID)
				return
			}
			op.rhs.SetRequestMessageDistinct("Ignoring stale lock by terminated VSR[%s]", vsrID)
		}
	}
	alreadyReserved := swag.Int64Value(op.spa.TotalCapacityBytes) - swag.Int64Value(op.spa.ReservableCapacityBytes)
	if alreadyReserved > swag.Int64Value(op.rhs.Request.ServicePlanAllocationCreateSpec.TotalCapacityBytes) {
		op.rhs.SetRequestError("Invalid totalCapacityBytes: %s already reserved", util.SizeBytesToString(alreadyReserved))
	}
}

func (op *acOp) markSPAInUse(ctx context.Context) {
	if op.vsrOwnsSpa() {
		return
	}
	if op.planOnly {
		op.rhs.SetRequestMessage("Plan: Mark ServicePlanAllocation object in use")
		return
	}
	modifyFn := func(o *models.ServicePlanAllocation) (*models.ServicePlanAllocation, error) {
		if o == nil {
			o = op.spa
		}
		stag := util.NewTagList(o.SystemTags)
		stag.Set(com.SystemTagVsrOperator, string(op.rhs.Request.Meta.ID))
		if op.rhs.HasDeleteSPA {
			stag.Set(com.SystemTagVsrDeleting, string(op.rhs.Request.Meta.ID))
		}
		o.SystemTags = stag.List()
		return o, nil
	}
	items := &crud.Updates{Set: []string{"systemTags"}}
	o, err := op.c.oCrud.ServicePlanAllocationUpdater(ctx, string(op.spa.Meta.ID), modifyFn, items)
	if err != nil {
		op.rhs.SetAndUpdateRequestMessage(ctx, "ServicePlanAllocation update error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.spa = o
}

func (op *acOp) createSPA(ctx context.Context) {
	cs := op.rhs.Request.ServicePlanAllocationCreateSpec
	spaObj := &models.ServicePlanAllocation{
		ServicePlanAllocationCreateOnce: cs.ServicePlanAllocationCreateOnce,
		ServicePlanAllocationMutable: models.ServicePlanAllocationMutable{
			ServicePlanAllocationCreateMutable: cs.ServicePlanAllocationCreateMutable, // note: contains formula
		},
	}
	spaObj.Messages = nil
	spaObj.ReservationState = ""
	spaObj.StorageReservations = nil
	spaObj.StorageFormula = op.rhs.Request.StorageFormula
	spaObj.SystemTags = []string{op.sTagCreator(), op.sTagOp()}
	spaObj.TotalCapacityBytes = swag.Int64(0) // create empty
	if op.planOnly {
		op.rhs.SetRequestMessage("Plan: Create ServicePlanAllocation object")
		op.spa = spaObj
		return
	}
	var o *models.ServicePlanAllocation
	err := op.c.rei.ErrOnBool("ac-fail-spa-create")
	if err == nil {
		o, err = op.c.oCrud.ServicePlanAllocationCreate(ctx, spaObj)
	}
	if err == nil {
		err = op.c.rei.ErrOnBool("ac-fail-after-spa-create")
	}
	if err != nil {
		op.rhs.SetAndUpdateRequestMessage(ctx, "ServicePlanAllocation create error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.rhs.Request.SystemTags = append(op.rhs.Request.SystemTags, op.vsrCreateSpaSysTag())
	op.spa = o
	op.rhs.Request.ServicePlanAllocationID = models.ObjIDMutable(op.spa.Meta.ID) // note: needs clearing before delete on UNDO path
	op.rhs.SetRequestMessage("Created ServicePlanAllocation [%s]", o.Meta.ID)
}

func (op *acOp) finalizeSPA(ctx context.Context) {
	systemTags := []string{op.sTagOp()}
	// leave creator indicator in case of downstream error on the creating VSR
	sr := map[string]models.StorageTypeReservation{}
	for spID, spr := range op.rhs.Request.CapacityReservationResult.DesiredReservations {
		sr[spID] = models.StorageTypeReservation{
			NumMirrors: spr.NumMirrors,
			SizeBytes:  spr.SizeBytes,
		}
	}
	modifyFn := func(o *models.ServicePlanAllocation) (*models.ServicePlanAllocation, error) {
		if err := op.c.rei.ErrOnBool("ac-fail-in-spa-finalize"); err != nil {
			return nil, err
		}
		if o == nil {
			o = op.spa
		}
		o.SystemTags = systemTags
		o.StorageReservations = sr
		o.TotalCapacityBytes = op.rhs.Request.ServicePlanAllocationCreateSpec.TotalCapacityBytes
		return o, nil
	}
	items := &crud.Updates{
		Remove: []string{"systemTags"},
		Set:    []string{"storageReservations", "totalCapacityBytes"},
	}
	o, err := op.c.oCrud.ServicePlanAllocationUpdater(ctx, string(op.spa.Meta.ID), modifyFn, items)
	if err != nil {
		op.rhs.SetAndUpdateRequestMessage(ctx, "ServicePlanAllocation update error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.rhs.Request.ServicePlanAllocationID = models.ObjIDMutable(o.Meta.ID)
	op.spa = o
}

func (op *acOp) clearSpaIDInRequest(ctx context.Context) {
	op.rhs.Request.ServicePlanAllocationID = ""
	op.updateRequest(ctx)
}

func (op *acOp) releaseSPA(ctx context.Context) {
	systemTags := []string{op.sTagOp()}
	modifyFn := func(o *models.ServicePlanAllocation) (*models.ServicePlanAllocation, error) {
		if o == nil {
			o = op.spa
		}
		o.SystemTags = systemTags
		return o, nil
	}
	items := &crud.Updates{
		Remove: []string{"systemTags"},
	}
	o, err := op.c.oCrud.ServicePlanAllocationUpdater(ctx, string(op.spa.Meta.ID), modifyFn, items)
	if err != nil {
		op.rhs.SetAndUpdateRequestMessage(ctx, "ServicePlanAllocation update error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.spa = o
}

func (op *acOp) deleteSPA(ctx context.Context) {
	op.rhs.SetRequestMessage("Deleting ServicePlanAllocation [%s]", op.spa.Meta.ID)
	var err error
	err = op.c.rei.ErrOnBool("ac-fail-spa-delete")
	if err == nil {
		err = op.c.oCrud.ServicePlanAllocationDelete(ctx, string(op.spa.Meta.ID))
	}
	if err != nil {
		op.rhs.SetAndUpdateRequestMessage(ctx, "ServicePlanAllocation delete error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.spa = nil
}

func (op *acOp) findPools(ctx context.Context) {
	if op.pools == nil {
		op.pools = make(map[string]*models.Pool)
	}
	// prime with desired types
	for cst := range op.rhs.Request.CapacityReservationPlan.StorageTypeReservations {
		op.pools[cst] = nil
	}
	cs := op.rhs.Request.ServicePlanAllocationCreateSpec
	lparams := client_pool.NewPoolListParams()
	lparams.AuthorizedAccountID = swag.String(string(cs.AuthorizedAccountID))
	lparams.ClusterID = swag.String(string(cs.ClusterID))
	ret, err := op.c.oCrud.PoolList(ctx, lparams)
	if err != nil {
		op.rhs.SetAndUpdateRequestMessage(ctx, "Pool list error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	// pools previously referenced by the spa whose types are still wanted must be picked up
	for _, poolObj := range ret.Payload {
		if _, present := op.pools[poolObj.CspStorageType]; !present {
			continue // type not wanted
		}
		if _, present := op.spa.StorageReservations[string(poolObj.Meta.ID)]; !present {
			continue
		}
		op.pools[poolObj.CspStorageType] = poolObj
		op.rhs.SetRequestMessage("Found previously used pool for %s: [%s]", poolObj.CspStorageType, poolObj.Meta.ID)
	}
	// pools referenced in the capacityReservationResult plan must be picked up
	if op.rhs.Request.CapacityReservationResult != nil {
		crr := op.rhs.Request.CapacityReservationResult
		for _, poolObj := range ret.Payload {
			if o, present := op.pools[poolObj.CspStorageType]; !present || o != nil {
				continue // type not wanted or already picked up the pool
			}
			if _, present := crr.DesiredReservations[string(poolObj.Meta.ID)]; !present {
				continue
			}
			op.pools[poolObj.CspStorageType] = poolObj
			op.rhs.SetRequestMessage("Found planned pool for %s: [%s]", poolObj.CspStorageType, poolObj.Meta.ID)
		}
	}
	// now fill in the remaining gaps
	for _, poolObj := range ret.Payload {
		if o, present := op.pools[poolObj.CspStorageType]; !present || o != nil {
			continue // type not wanted or already picked up the pool
		}
		op.pools[poolObj.CspStorageType] = poolObj
		op.rhs.SetRequestMessage("Found existing pool for %s: [%s]", poolObj.CspStorageType, poolObj.Meta.ID)
	}
}

func (op *acOp) createPools(ctx context.Context) {
	cs := op.rhs.Request.ServicePlanAllocationCreateSpec
	sCST := []string{}
	for cst, o := range op.pools {
		if o != nil {
			continue
		}
		sCST = append(sCST, cst)
	}
	for _, cst := range sCST {
		cstObj := op.c.App.GetCspStorageType(models.CspStorageType(cst))
		sa := &models.StorageAccessibilityMutable{
			AccessibilityScope: cstObj.AccessibilityScope,
		}
		if sa.AccessibilityScope == com.AccScopeCspDomain {
			sa.AccessibilityScopeObjID = op.cluster.CspDomainID
		}
		poolObj := &models.Pool{
			PoolCreateOnce: models.PoolCreateOnce{
				AccountID:            cs.AccountID,
				AuthorizedAccountID:  cs.AuthorizedAccountID,
				ClusterID:            cs.ClusterID,
				CspDomainID:          op.cluster.CspDomainID,
				CspStorageType:       cst,
				StorageAccessibility: sa,
			},
		}
		if op.planOnly {
			op.rhs.SetRequestMessage("Plan: Create new pool for %s", cst)
			continue
		}
		var o *models.Pool
		err := op.c.rei.ErrOnBool("ac-fail-pool-create")
		if err == nil {
			o, err = op.c.oCrud.PoolCreate(ctx, poolObj)
		}
		if err == nil {
			err = op.c.rei.ErrOnBool("ac-fail-after-pool-create")
		}
		if err != nil {
			op.rhs.SetAndUpdateRequestMessage(ctx, "Pool create error: %s", err.Error())
			op.rhs.RetryLater = true
			return
		}
		op.pools[cst] = o
		op.rhs.SetRequestMessage("Created pool [%s] for %s", o.Meta.ID, cst)
	}
}

func (op *acOp) removeEmptyPoolReferences(ctx context.Context) {
	pools := []*models.Pool{}
	for _, o := range op.pools {
		if o != nil && len(o.ServicePlanReservations) == 0 {
			pools = append(pools, o) // candidates for deletion
		}
	}
	needToUpdate := false
	for _, o := range pools {
		poolID := string(o.Meta.ID)
		// remove references to the Pool or else we won't be able to update the VSR or delete the Pool
		crr := op.rhs.Request.CapacityReservationResult
		if crr != nil {
			poolRemoved := false
			if _, present := crr.CurrentReservations[poolID]; present {
				poolRemoved = true
				delete(crr.CurrentReservations, poolID)
			}
			if _, present := crr.DesiredReservations[poolID]; present {
				poolRemoved = true
				delete(crr.DesiredReservations, poolID)
			}
			if poolRemoved {
				needToUpdate = true
				op.rhs.SetRequestMessage("Removed references to pool [%s]", poolID)
			}
		}
	}
	if needToUpdate && op.rhs.UpdateRequest(ctx) != nil {
		op.rhs.RetryLater = true
	}
}

func (op *acOp) deletePools(ctx context.Context) {
	pools := []*models.Pool{}
	for _, o := range op.pools {
		if o != nil && len(o.ServicePlanReservations) == 0 {
			pools = append(pools, o) // candidates for deletion
		}
	}
	for _, o := range pools {
		poolID := string(o.Meta.ID)
		op.rhs.SetRequestMessage("Deleting pool [%s]", poolID)
		if err := op.c.oCrud.PoolDelete(ctx, poolID); err != nil {
			op.rhs.RetryLater = true
			break
		}
		op.pools[o.CspStorageType] = nil
	}
}

func (op *acOp) initializeCRR(ctx context.Context) {
	crr := &models.CapacityReservationResult{
		CurrentReservations: map[string]models.PoolReservation{},
		DesiredReservations: map[string]models.PoolReservation{},
	}
	spaID := string(op.spa.Meta.ID)
	for cst, str := range op.rhs.Request.CapacityReservationPlan.StorageTypeReservations {
		poolObj := op.pools[cst]
		poolID := string(poolObj.Meta.ID)
		// save current capacity value (including none)
		spr := models.PoolReservation{NumMirrors: 1}
		if curStr, found := poolObj.ServicePlanReservations[spaID]; found {
			spr.NumMirrors = curStr.NumMirrors
			spr.SizeBytes = curStr.SizeBytes
		}
		crr.CurrentReservations[poolID] = spr
		// save desired values
		crr.DesiredReservations[poolID] = models.PoolReservation{
			NumMirrors: str.NumMirrors,
			SizeBytes:  str.SizeBytes,
		}
	}
	op.rhs.Request.CapacityReservationResult = crr
}

func (op *acOp) addCapacityToPools(ctx context.Context) {
	op.setPoolCapacity(ctx, op.rhs.Request.CapacityReservationResult.DesiredReservations)
}

func (op *acOp) removeCapacityFromPools(ctx context.Context) {
	if op.rhs.Request.CapacityReservationResult != nil && len(op.rhs.Request.CapacityReservationResult.CurrentReservations) > 0 {
		op.setPoolCapacity(ctx, op.rhs.Request.CapacityReservationResult.CurrentReservations) // roll back
	}
}

func (op *acOp) setPoolCapacity(ctx context.Context, sprMap map[string]models.PoolReservation) {
	pools := []*models.Pool{}
	for _, o := range op.pools {
		pools = append(pools, o)
	}
	for poolID, spr := range sprMap {
		for _, p := range pools {
			if string(p.Meta.ID) == poolID {
				sca := &SetCapacityArgs{
					ServicePlanAllocationID: string(op.spa.Meta.ID),
					Capacity:                &spr,
					Pool:                    p,
				}
				op.c.Log.Debugf("VolumeSeriesRequest %s: setting pool [%s] capacity (%d,%d)", op.rhs.Request.Meta.ID, poolID, spr.NumMirrors, swag.Int64Value(spr.SizeBytes))
				var o *models.Pool
				var err error
				err = op.c.rei.ErrOnBool("ac-fail-pool-set-capacity")
				if err == nil {
					o, err = op.ops.setPoolCapacityForSPA(ctx, sca)
				}
				if err != nil {
					op.rhs.SetAndUpdateRequestMessage(ctx, "Error updating pool [%s]: %s", poolID, err.Error())
					op.rhs.RetryLater = true
					return
				}
				op.pools[string(p.CspStorageType)] = o
				op.rhs.SetRequestMessage("Set capacity in pool [%s] to (%d, %s)", poolID, spr.NumMirrors, util.SizeBytes(swag.Int64Value(spr.SizeBytes)))
				break // pool loop
			}
		}
	}
}

// SetCapacityArgs contains the arguments for SetPoolCapacityForSPA
type SetCapacityArgs struct {
	ServicePlanAllocationID string
	Capacity                *models.PoolReservation
	Pool                    *models.Pool
}

// setPoolCapacityForSPA updates the Pool object with the new or changed needs of a ServicePlanAllocation.
// The function does nothing if the specified SPA values are already set in the Pool.
// Specifying 0 mirrors or size results in the SPA being removed from the servicePlanReservations map.
func (op *acOp) setPoolCapacityForSPA(ctx context.Context, args *SetCapacityArgs) (*models.Pool, error) {
	pool := args.Pool
	nM := args.Capacity.NumMirrors
	sB := swag.Int64Value(args.Capacity.SizeBytes)
	updateReservationInMap := false
	deleteReservationInMap := false
	if str, ok := pool.ServicePlanReservations[args.ServicePlanAllocationID]; ok {
		if nM == 0 || sB == 0 {
			deleteReservationInMap = true
		} else if str.NumMirrors != nM || swag.Int64Value(str.SizeBytes) != sB {
			updateReservationInMap = true
		}
	} else if nM != 0 && sB != 0 {
		updateReservationInMap = true
	}
	if !(updateReservationInMap || deleteReservationInMap) {
		return pool, nil
	}
	modifyFn := func(o *models.Pool) (*models.Pool, error) {
		if o == nil {
			o = pool
		}
		if deleteReservationInMap && o.ServicePlanReservations != nil {
			delete(o.ServicePlanReservations, args.ServicePlanAllocationID)
		}
		if updateReservationInMap {
			if o.ServicePlanReservations == nil {
				o.ServicePlanReservations = make(map[string]models.StorageTypeReservation)
			}
			o.ServicePlanReservations[args.ServicePlanAllocationID] = models.StorageTypeReservation{
				NumMirrors: args.Capacity.NumMirrors,
				SizeBytes:  args.Capacity.SizeBytes,
			}
		}
		return o, nil
	}
	items := &crud.Updates{}
	items.Set = []string{"servicePlanReservations"}
	return op.c.oCrud.PoolUpdater(ctx, string(pool.Meta.ID), modifyFn, items)
}

func (op *acOp) removeStorageReservations(ctx context.Context) {
	if len(op.spa.StorageReservations) == 0 && swag.Int64Value(op.spa.TotalCapacityBytes) == 0 {
		return
	}
	modifyFn := func(o *models.ServicePlanAllocation) (*models.ServicePlanAllocation, error) {
		if err := op.c.rei.ErrOnBool("ac-fail-in-spa-remove-res"); err != nil {
			return nil, err
		}
		if o == nil {
			o = op.spa
		}
		o.StorageReservations = nil
		o.TotalCapacityBytes = swag.Int64(0)
		return o, nil
	}
	items := &crud.Updates{
		Set: []string{"storageReservations", "totalCapacityBytes"},
	}
	o, err := op.c.oCrud.ServicePlanAllocationUpdater(ctx, string(op.spa.Meta.ID), modifyFn, items)
	if err != nil {
		op.rhs.SetAndUpdateRequestMessage(ctx, "ServicePlanAllocation update error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.spa = o
}

func (op *acOp) addAccountToSDC(ctx context.Context) {
	cs := op.rhs.Request.ServicePlanAllocationCreateSpec
	if op.planOnly {
		op.rhs.SetRequestMessage("Plan: Add account [%s] to service plan, Domain and Cluster", cs.AuthorizedAccountID)
		return
	}
	if util.Contains(op.sp.Accounts, cs.AuthorizedAccountID) {
		op.rhs.SetRequestMessage("Account [%s] already authorized for service plan", cs.AuthorizedAccountID)
	} else {
		op.updateServicePlan(ctx, true)
	}
	if util.Contains(op.domain.AuthorizedAccounts, cs.AuthorizedAccountID) {
		op.rhs.SetRequestMessage("Account [%s] already authorized for Domain", cs.AuthorizedAccountID)
	} else {
		op.updateDomain(ctx, true)
	}
	if util.Contains(op.cluster.AuthorizedAccounts, cs.AuthorizedAccountID) {
		op.rhs.SetRequestMessage("Account [%s] already authorized for Cluster", cs.AuthorizedAccountID)
	} else {
		op.updateCluster(ctx, true)
	}
	return
}

func (op *acOp) removeAccountFromSDC(ctx context.Context) {
	authorizedAccountID := op.rhs.Request.ServicePlanAllocationCreateSpec.AuthorizedAccountID
	if op.planOnly {
		op.rhs.SetRequestMessage("Plan: Remove account [%s] from service plan, Domain and Cluster", authorizedAccountID)
		return
	}
	if util.Contains(op.sp.Accounts, authorizedAccountID) {
		op.updateServicePlan(ctx, false)
	} else {
		op.rhs.SetRequestMessage("Account [%s] already removed from ServicePlan", string(authorizedAccountID))
	}
	if util.Contains(op.domain.AuthorizedAccounts, authorizedAccountID) {
		op.updateDomain(ctx, false)
	} else {
		op.rhs.SetRequestMessage("Account [%s] already removed from Domain", string(authorizedAccountID))
	}
	if util.Contains(op.cluster.AuthorizedAccounts, authorizedAccountID) {
		op.updateCluster(ctx, false)
	} else {
		op.rhs.SetRequestMessage("Account [%s] already removed from Cluster", string(authorizedAccountID))
	}
}

func (op *acOp) updateServicePlan(ctx context.Context, setAccount bool) {
	cs := op.rhs.Request.ServicePlanAllocationCreateSpec
	authAccounts := []models.ObjIDMutable{cs.AuthorizedAccountID}
	modifyFn := func(o *models.ServicePlan) (*models.ServicePlan, error) {
		if o == nil {
			o = op.sp
		}
		o.Accounts = authAccounts
		return o, nil
	}
	items := &crud.Updates{}
	retMessage := ""
	if setAccount {
		items.Append = []string{"accounts"}
		retMessage = "Account [%s] authorized for ServicePlan [%s]"
	} else {
		items.Remove = []string{"accounts"}
		retMessage = "Account [%s] removed from ServicePlan [%s]"
	}
	o, err := op.c.oCrud.ServicePlanUpdater(ctx, string(op.sp.Meta.ID), modifyFn, items)
	if err != nil {
		op.rhs.SetAndUpdateRequestMessage(ctx, "ServicePlan update error: %s", err.Error())
		if !strings.HasPrefix(err.Error(), com.ErrorUpdateInvalidRequest) {
			// if error other than invalid request - retry later
			op.rhs.RetryLater = true
		}
		return
	}
	op.rhs.SetAndUpdateRequestMessage(ctx, retMessage, cs.AuthorizedAccountID, o.Meta.ID)
	op.sp = o
}

func (op *acOp) updateDomain(ctx context.Context, setAccount bool) {
	cs := op.rhs.Request.ServicePlanAllocationCreateSpec
	authAccounts := []models.ObjIDMutable{cs.AuthorizedAccountID}
	modifyFn := func(o *models.CSPDomain) (*models.CSPDomain, error) {
		if o == nil {
			o = op.domain
		}
		o.AuthorizedAccounts = authAccounts
		return o, nil
	}
	items := &crud.Updates{}
	retMessage := ""
	if setAccount {
		items.Append = []string{"authorizedAccounts"}
		retMessage = "Account [%s] authorized for Domain [%s]"
	} else {
		items.Remove = []string{"authorizedAccounts"}
		retMessage = "Account [%s] removed from Domain [%s]"
	}
	o, err := op.c.oCrud.CSPDomainUpdater(ctx, string(op.domain.Meta.ID), modifyFn, items)
	if err != nil {
		op.rhs.SetAndUpdateRequestMessage(ctx, "Domain update error: %s", err.Error())
		if !strings.HasPrefix(err.Error(), com.ErrorUpdateInvalidRequest) {
			// if error other than invalid request - retry later
			op.rhs.RetryLater = true
		}
		return
	}
	op.rhs.SetAndUpdateRequestMessage(ctx, retMessage, cs.AuthorizedAccountID, o.Meta.ID)
	op.domain = o
}

func (op *acOp) updateCluster(ctx context.Context, setAccount bool) {
	cs := op.rhs.Request.ServicePlanAllocationCreateSpec
	authAccounts := []models.ObjIDMutable{cs.AuthorizedAccountID}
	modifyFn := func(o *models.Cluster) (*models.Cluster, error) {
		if o == nil {
			o = op.cluster
		}
		o.AuthorizedAccounts = authAccounts
		return o, nil
	}
	items := &crud.Updates{}
	retMessage := ""
	if setAccount {
		items.Append = []string{"authorizedAccounts"}
		retMessage = "Account [%s] authorized for Cluster [%s]"
	} else {
		items.Remove = []string{"authorizedAccounts"}
		retMessage = "Account [%s] removed from Cluster [%s]"
	}
	o, err := op.c.oCrud.ClusterUpdater(ctx, string(op.cluster.Meta.ID), modifyFn, items)
	if err != nil {
		op.rhs.SetAndUpdateRequestMessage(ctx, "Cluster update error: %s", err.Error())
		if !strings.HasPrefix(err.Error(), com.ErrorUpdateInvalidRequest) {
			// if error other than invalid request - retry later
			op.rhs.RetryLater = true
		}
		return
	}
	op.rhs.SetAndUpdateRequestMessage(ctx, retMessage, cs.AuthorizedAccountID, o.Meta.ID)
	op.cluster = o
}

func (op *acOp) vsrIsActive(ctx context.Context, vsrID string) bool {
	vsr, err := op.c.oCrud.VolumeSeriesRequestFetch(ctx, vsrID)
	if err != nil {
		if e, ok := err.(*crud.Error); ok && e.NotFound() {
			return false
		}
		return true
	}
	return !vra.VolumeSeriesRequestStateIsTerminated(vsr.VolumeSeriesRequestState)
}
