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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/application_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/consistency_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/snapshot"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/swag"
)

// createSubState represents creating sub-states.
type createSubState int

// createSubState values
const (
	// Issue the ApplicationGroupCreate database operation if no AG exists
	CreateAG createSubState = iota
	// Issue the ConsistencyGroupCreate database operation if no CG exists
	CreateCG
	// Issue the VolumeSeriesCreate database operation
	CreateVS
	// Update the Request object with the VolumeSeriesID
	CreateSetID
	// Return
	CreateDone
	// Transition the VS to terminal DELETING state
	CreateSetDeleting
	// Delete all VS snapshots
	CreateDeleteSnapshots
	// Remove VS (about to be deleted) from CG, which might make it empty and deletable
	CreateRemoveVSFromCG
	// Delete empty ConsistencyGroup
	CreateDeleteCG
	// Delete empty ApplicationGroups
	CreateDeleteAGs
	// Delete VolumeSeries (terminal sub-state)
	CreateDeleteVS
	// Delete auto-created CG, then ...
	CreateDeleteCGAndAG
	// Delete auto-created AG
	CreateDeleteAGOnly

	// LAST: No operation is performed in this state
	CreateNoOp
)

func (ss createSubState) String() string {
	switch ss {
	case CreateCG:
		return "CreateCG"
	case CreateAG:
		return "CreateAG"
	case CreateVS:
		return "CreateVS"
	case CreateSetID:
		return "CreateSetID"
	case CreateDone:
		return "CreateDone"
	case CreateSetDeleting:
		return "CreateSetDeleting"
	case CreateDeleteSnapshots:
		return "CreateDeleteSnapshots"
	case CreateRemoveVSFromCG:
		return "CreateRemoveVSFromCG"
	case CreateDeleteCG:
		return "CreateDeleteCG"
	case CreateDeleteAGs:
		return "CreateDeleteAGs"
	case CreateDeleteVS:
		return "CreateDeleteVS"
	case CreateDeleteCGAndAG:
		return "CreateDeleteCGAndAG"
	case CreateDeleteAGOnly:
		return "CreateDeleteCGOnly"
	}
	return fmt.Sprintf("createSubState(%d)", ss)
}

type createOp struct {
	c          *Component
	rhs        *vra.RequestHandlerState
	ops        createOperators
	creatorTag string
	ags        []*models.ApplicationGroup
	cg         *models.ConsistencyGroup
	inError    bool
}

type createOperators interface {
	getInitialState(ctx context.Context) createSubState
	createApplicationGroup(ctx context.Context)
	createConsistencyGroup(ctx context.Context)
	createVolumeSeries(ctx context.Context)
	setID(ctx context.Context)
	setDeleting(ctx context.Context)
	removeVSFromCG(ctx context.Context)
	deleteCG(ctx context.Context)
	deleteAGs(ctx context.Context)
	deleteVS(ctx context.Context)
	deleteVSSnapshots(ctx context.Context)
}

// vra.CreateHandlers methods

// Create creates a new VolumeSeries object
func (c *Component) Create(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &createOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// UndoCreate reverses the changes made by Create
func (c *Component) UndoCreate(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &createOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// run executes the creating state machine
func (op *createOp) run(ctx context.Context) {
	jumpToState := CreateNoOp
out:
	for ss := op.ops.getInitialState(ctx); !(op.rhs.InError || op.rhs.RetryLater); ss++ {
		if jumpToState != CreateNoOp {
			ss = jumpToState
			jumpToState = CreateNoOp
		}
		op.c.Log.Debugf("VolumeSeriesRequest %s: CREATE/DELETE %s", op.rhs.Request.Meta.ID, ss)
		switch ss {
		case CreateAG:
			op.ops.createApplicationGroup(ctx)
		case CreateCG:
			op.ops.createConsistencyGroup(ctx)
		case CreateVS:
			op.ops.createVolumeSeries(ctx)
		case CreateSetID:
			op.ops.setID(ctx)
		case CreateDone:
			break out
		case CreateSetDeleting:
			op.ops.setDeleting(ctx)
		case CreateDeleteSnapshots:
			op.ops.deleteVSSnapshots(ctx)
			if op.cg == nil {
				jumpToState = CreateDeleteVS
			}
		case CreateRemoveVSFromCG:
			op.ops.removeVSFromCG(ctx)
		case CreateDeleteCG:
			op.ops.deleteCG(ctx)
		case CreateDeleteAGs:
			op.ops.deleteAGs(ctx)
		case CreateDeleteVS:
			op.ops.deleteVS(ctx)
			break out // terminal state
		case CreateDeleteCGAndAG:
			op.ops.deleteCG(ctx)
		case CreateDeleteAGOnly:
			op.ops.deleteAGs(ctx)
		default:
			break out
		}
	}
	if op.inError {
		op.rhs.InError = true
	}
}

func (op *createOp) getInitialState(ctx context.Context) createSubState {
	if swag.BoolValue(op.rhs.Request.PlanOnly) {
		// there is no use case that requires plan only on create; planOnly is for internal use
		op.rhs.SetRequestError("CREATE does not support planOnly")
		op.rhs.InError = true
		return CreateDone
	}
	op.creatorTag = fmt.Sprintf("%s:%s", com.SystemTagVsrCreator, op.rhs.Request.Meta.ID)
	if op.rhs.Request.VolumeSeriesRequestState == com.VolReqStateUndoCreating {
		op.inError = op.rhs.InError
		op.rhs.InError = false // switch off to enable cleanup
		op.loadObjects(ctx)
		if op.rhs.RetryLater || op.cg == nil && len(op.ags) == 0 && op.rhs.VolumeSeries == nil {
			return CreateDone
		} else if len(op.ags) == 0 && op.cg == nil {
			if op.rhs.VolumeSeries.VolumeSeriesState != com.VolStateDeleting {
				return CreateSetDeleting
			}
			return CreateDeleteSnapshots
		} else if op.rhs.VolumeSeries == nil {
			return CreateDeleteCGAndAG
		}
		if op.rhs.VolumeSeries.ConsistencyGroupID != "" {
			if op.rhs.VolumeSeries.VolumeSeriesState != com.VolStateDeleting {
				return CreateSetDeleting
			}
			return CreateDeleteSnapshots
		}
		if op.cg != nil {
			return CreateDeleteCG
		}
		return CreateDeleteAGs
	}
	if op.rhs.Canceling || op.rhs.InError {
		return CreateDone
	}
	op.loadObjects(ctx)
	if len(op.ags) == 0 || op.rhs.RetryLater {
		return CreateAG
	}
	if op.cg == nil {
		return CreateCG
	}
	if op.rhs.Request.VolumeSeriesID != "" {
		return CreateDone
	}
	if op.rhs.VolumeSeries != nil {
		return CreateSetID
	}
	return CreateVS
}

// loadObjects will attempt to load the ApplicationGroup, ConsistencyGroup and VolumeSeries objects
// It will set RetryLater if any query fails
func (op *createOp) loadObjects(ctx context.Context) {
	if op.rhs.VolumeSeries == nil {
		// VSR may have created the VS but has not yet updated the VSR with its ID
		qry := volume_series.NewVolumeSeriesListParams()
		qry.AccountID = swag.String(string(op.rhs.Request.VolumeSeriesCreateSpec.AccountID))
		qry.Name = swag.String(string(op.rhs.Request.VolumeSeriesCreateSpec.Name))
		qry.SystemTags = []string{op.creatorTag}
		vsl, err := op.c.oCrud.VolumeSeriesList(ctx, qry)
		if err != nil {
			op.rhs.SetRequestMessage("volume series query error: %s", err.Error())
			op.rhs.RetryLater = true
			return
		} else if len(vsl.Payload) == 1 {
			op.rhs.VolumeSeries = vsl.Payload[0]
		}
	}
	var err error
	var cgID string
	agIds := []string{}
	removedID := map[string]bool{}
	if op.rhs.VolumeSeries != nil {
		if op.rhs.VolumeSeries.ConsistencyGroupID != "" {
			cgID = string(op.rhs.VolumeSeries.ConsistencyGroupID)
		} else {
			// reset tags are all added in a single update
			agPrefix := fmt.Sprintf("%s:%s,", com.SystemTagVsrAGReset, op.rhs.Request.Meta.ID)
			cgPrefix := fmt.Sprintf("%s:%s,", com.SystemTagVsrCGReset, op.rhs.Request.Meta.ID)
			for _, tag := range op.rhs.VolumeSeries.SystemTags {
				if strings.HasPrefix(tag, agPrefix) {
					id := strings.TrimPrefix(tag, agPrefix)
					agIds = append(agIds, id)
					removedID[id] = true
				}
				if strings.HasPrefix(tag, cgPrefix) {
					cgID = strings.TrimPrefix(tag, cgPrefix)
					removedID[cgID] = true
				}
			}
		}
	}
	if op.rhs.HasCreate && cgID == "" {
		cgID = string(op.rhs.Request.VolumeSeriesCreateSpec.ConsistencyGroupID)
	}
	if cgID != "" {
		op.cg, err = op.c.oCrud.ConsistencyGroupFetch(ctx, cgID)
		if err != nil {
			// in the case cgID was already removed from the VS, it's possible for the CG to be deleted by another VSR
			if oErr, ok := err.(*crud.Error); ok && oErr.NotFound() && removedID[cgID] {
				op.rhs.SetRequestMessage("Consistency group has already been deleted")
			} else {
				op.rhs.SetRequestMessage("Consistency group query error: %s", err.Error())
				op.rhs.RetryLater = true
				return
			}
		}
	} else if op.rhs.HasCreate {
		qry := consistency_group.NewConsistencyGroupListParams()
		qry.AccountID = swag.String(string(op.rhs.Request.VolumeSeriesCreateSpec.AccountID))
		qry.Name = swag.String(string(op.rhs.Request.VolumeSeriesCreateSpec.Name))
		qry.SystemTags = []string{op.creatorTag}
		var cgl *consistency_group.ConsistencyGroupListOK
		cgl, err = op.c.oCrud.ConsistencyGroupList(ctx, qry)
		if err != nil {
			op.rhs.SetRequestMessage("Consistency group query error: %s", err.Error())
			op.rhs.RetryLater = true
			return
		} else if len(cgl.Payload) == 1 {
			op.cg = cgl.Payload[0]
		}
	}
	if op.cg != nil && len(agIds) == 0 {
		for _, id := range op.cg.ApplicationGroupIds { // cast precludes use of copy()
			agIds = append(agIds, string(id))
		}
	}
	if op.rhs.HasCreate && len(agIds) == 0 {
		for _, id := range op.rhs.Request.ApplicationGroupIds { // cast precludes use of copy()
			agIds = append(agIds, string(id))
		}
	}
	if len(agIds) > 0 {
		for _, id := range agIds {
			ag, err := op.c.oCrud.ApplicationGroupFetch(ctx, id)
			if err != nil {
				// in the case cgID was already removed from the VS, it's possible for the AG to also be deleted by another VSR
				if oErr, ok := err.(*crud.Error); ok && oErr.NotFound() && removedID[id] {
					op.rhs.SetRequestMessage("Application group has already been deleted")
				} else {
					op.rhs.SetRequestMessage("Application group query error: %s", err.Error())
					op.rhs.RetryLater = true
					return
				}
			} else {
				op.ags = append(op.ags, ag)
			}
		}
	} else if op.rhs.HasCreate {
		qry := application_group.NewApplicationGroupListParams()
		qry.AccountID = swag.String(string(op.rhs.Request.VolumeSeriesCreateSpec.AccountID))
		qry.Name = swag.String(string(op.rhs.Request.VolumeSeriesCreateSpec.Name))
		qry.SystemTags = []string{op.creatorTag}
		var agl *application_group.ApplicationGroupListOK
		agl, err = op.c.oCrud.ApplicationGroupList(ctx, qry)
		if err != nil {
			op.rhs.SetRequestMessage("Application group query error: %s", err.Error())
			op.rhs.RetryLater = true
			return
		} else if len(agl.Payload) > 0 {
			op.ags = agl.Payload
		}
	}
}

func (op *createOp) createConsistencyGroup(ctx context.Context) {
	agIDs := make([]models.ObjIDMutable, 0, len(op.ags))
	for _, ag := range op.ags {
		agIDs = append(agIDs, models.ObjIDMutable(ag.Meta.ID))
	}
	cg := &models.ConsistencyGroup{
		ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
			AccountID: op.rhs.Request.VolumeSeriesCreateSpec.AccountID,
		},
		ConsistencyGroupMutable: models.ConsistencyGroupMutable{
			ApplicationGroupIds: agIDs,
			Name:                op.rhs.Request.VolumeSeriesCreateSpec.Name,
			SystemTags:          []string{op.creatorTag},
		},
	}
	var err error
	op.cg, err = op.c.oCrud.ConsistencyGroupCreate(ctx, cg)
	if err != nil {
		op.rhs.SetRequestError("Failed to create ConsistencyGroup object: %s", err.Error())
	}
}

// createApplicationGroup auto-creates an AG if none specified for this new VS
func (op *createOp) createApplicationGroup(ctx context.Context) {
	ag := &models.ApplicationGroup{
		ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
			AccountID: op.rhs.Request.VolumeSeriesCreateSpec.AccountID,
		},
		ApplicationGroupMutable: models.ApplicationGroupMutable{
			Name:       op.rhs.Request.VolumeSeriesCreateSpec.Name,
			SystemTags: []string{op.creatorTag},
		},
	}
	var err error
	if ag, err = op.c.oCrud.ApplicationGroupCreate(ctx, ag); err != nil {
		op.rhs.SetRequestError("Failed to create ApplicationGroup object: %s", err.Error())
	} else {
		op.ags = append(op.ags, ag)
	}
}

func (op *createOp) createVolumeSeries(ctx context.Context) {
	vs := &models.VolumeSeries{
		VolumeSeriesCreateOnce: op.rhs.Request.VolumeSeriesCreateSpec.VolumeSeriesCreateOnce,
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesCreateMutable: op.rhs.Request.VolumeSeriesCreateSpec.VolumeSeriesCreateMutable,
		},
	}
	vs.ConsistencyGroupID = models.ObjIDMutable(op.cg.Meta.ID)
	vs.SystemTags = append(vs.SystemTags, op.creatorTag)
	var err error
	if op.rhs.VolumeSeries, err = op.c.oCrud.VolumeSeriesCreate(ctx, vs); err != nil {
		op.rhs.SetRequestError("Failed to create VolumeSeries object: %s", err.Error())
	}
}

// setID updates the VolumeSeriesRequest with the ID of the VolumeSeries and ConsistencyGroup objects.
func (op *createOp) setID(ctx context.Context) {
	op.rhs.Request.VolumeSeriesID = models.ObjIDMutable(op.rhs.VolumeSeries.Meta.ID)
	op.rhs.Request.ConsistencyGroupID = models.ObjIDMutable(op.cg.Meta.ID)
	op.rhs.SetRequestMessage("Created volume series %s[%s]", op.rhs.VolumeSeries.Name, op.rhs.VolumeSeries.Meta.ID)
	err := op.c.rei.ErrOnBool("create-fail-in-setID")
	if err == nil {
		err = op.rhs.UpdateRequest(ctx)
	}
	if err != nil {
		// unlikely this would ever be saved
		op.rhs.SetRequestError("Failed to update VolumeSeriesRequest object: %s", err.Error())
	}
}

// setDeleting puts the VS into the terminal DELETING state
func (op *createOp) setDeleting(ctx context.Context) {
	op.rhs.VSUpdater.SetVolumeState(ctx, com.VolStateDeleting)
}

// removeVSFromCG resets the ConsistencyGroupID on the VS so it might be empty, allowing it to be deleted.
// Tags are added to the VS systemTags to record the CG and its AGs to allow the VSR to be restarted.
func (op *createOp) removeVSFromCG(ctx context.Context) {
	resetTag := fmt.Sprintf("%s:%s,%s", com.SystemTagVsrCGReset, op.rhs.Request.Meta.ID, op.rhs.VolumeSeries.ConsistencyGroupID)
	op.rhs.VolumeSeries.ConsistencyGroupID = ""
	op.rhs.VolumeSeries.SystemTags = []string{resetTag}
	for _, id := range op.cg.ApplicationGroupIds {
		resetTag = fmt.Sprintf("%s:%s,%s", com.SystemTagVsrAGReset, op.rhs.Request.Meta.ID, id)
		op.rhs.VolumeSeries.SystemTags = append(op.rhs.VolumeSeries.SystemTags, resetTag)
	}
	items := &crud.Updates{Set: []string{"consistencyGroupId"}, Append: []string{"systemTags"}}
	vs, err := op.c.oCrud.VolumeSeriesUpdate(ctx, op.rhs.VolumeSeries, items)
	if err != nil {
		op.rhs.SetRequestMessage("Failed to update VolumeSeries object: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.rhs.VolumeSeries = vs
}

func (op *createOp) deleteAGs(ctx context.Context) {
	// if undoing (not explicit DELETE op), only delete auto-created AG
	if len(op.ags) == 0 || (!op.rhs.HasDelete &&
		!util.Contains(op.ags[0].SystemTags, op.creatorTag) &&
		!util.Contains(op.ags[0].SystemTags, com.SystemTagMustDeleteOnUndoCreate)) {
		return
	}
	for _, ag := range op.ags {
		if err := op.c.oCrud.ApplicationGroupDelete(ctx, string(ag.Meta.ID)); err != nil {
			if oErr, ok := err.(*crud.Error); ok && (oErr.Payload.Code == int32(centrald.ErrorExists.C) || oErr.NotFound()) {
				op.c.Log.Debugf("Ignoring failure to delete ApplicationGroup object %s", ag.Meta.ID)
			} else {
				op.c.Log.Warningf("Failed to delete ApplicationGroup object %s", ag.Meta.ID)
			}
		} else {
			op.rhs.SetRequestMessage("Deleted application group %s[%s]", ag.Name, ag.Meta.ID)
		}
	}
}

func (op *createOp) deleteCG(ctx context.Context) {
	// if undoing (not explicit DELETE op), only delete auto-created CG
	if op.cg == nil || (!op.rhs.HasDelete &&
		!util.Contains(op.cg.SystemTags, op.creatorTag) &&
		!util.Contains(op.cg.SystemTags, com.SystemTagMustDeleteOnUndoCreate)) {
		return
	}
	if err := op.c.oCrud.ConsistencyGroupDelete(ctx, string(op.cg.Meta.ID)); err != nil {
		if oErr, ok := err.(*crud.Error); ok && (oErr.Payload.Code == int32(centrald.ErrorExists.C) || oErr.NotFound()) {
			op.c.Log.Debugf("Ignoring failure to delete ConsistencyGroup object %s", op.cg.Meta.ID)
		} else {
			op.c.Log.Warningf("Failed to delete ConsistencyGroup object %s", op.cg.Meta.ID)
		}
	} else {
		op.rhs.SetRequestMessage("Deleted consistency group %s[%s]", op.cg.Name, op.cg.Meta.ID)
	}
}

// deleteVSSnapshots should delete all VolumeSeries related snapshot objects if any exist
func (op *createOp) deleteVSSnapshots(ctx context.Context) {
	vsID := string(op.rhs.VolumeSeries.Meta.ID)
	lParams := snapshot.NewSnapshotListParams()
	lParams.VolumeSeriesID = &vsID
	snapshots, err := op.c.oCrud.SnapshotList(ctx, lParams)
	if err != nil {
		op.rhs.RetryLater = true
		return
	}
	for _, snapshot := range snapshots.Payload {
		err = op.c.oCrud.SnapshotDelete(ctx, string(snapshot.Meta.ID))
		if err != nil {
			op.rhs.RetryLater = true
			return
		}
	}
}

// deleteVS should delete the VolumeSeries if it exists
func (op *createOp) deleteVS(ctx context.Context) {
	err := op.c.rei.ErrOnBool("create-fail-in-delete")
	if err == nil {
		err = op.c.oCrud.VolumeSeriesDelete(ctx, string(op.rhs.VolumeSeries.Meta.ID))
	}
	if err != nil {
		if op.rhs.HasDelete {
			op.rhs.SetRequestError("Failed to delete VolumeSeries [%s]: %s", op.rhs.VolumeSeries.Meta.ID, err.Error())
		} else {
			op.rhs.SetRequestMessage("Failed to delete VolumeSeries [%s]: %s", op.rhs.VolumeSeries.Meta.ID, err.Error())
		}
	} else {
		op.rhs.SetRequestMessage("Deleted volume series %s[%s]", op.rhs.VolumeSeries.Name, op.rhs.VolumeSeries.Meta.ID)
		op.rhs.VolumeSeries = nil
		// Request.VolumeSeriesID cannot be reset
	}
}
