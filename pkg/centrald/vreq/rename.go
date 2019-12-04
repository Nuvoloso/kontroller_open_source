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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

// renameSubState represents renaming sub-states.
// There is at most one database update in each sub-state.
type renameSubState int

// renameSubState values
const (
	// Tag VSR with the old VS name
	RenameStart renameSubState = iota
	// Rename the VS
	RenameVS
	// Rename the CG if the same name
	RenameCG
	// Rename a AG if the same name
	RenameAG
	// Return
	RenameDone
	// Revert rename of the AG
	RenameRevertAG
	// Revert rename of the CG
	RenameRevertCG
	// Revert rename of the VS
	RenameRevertVS
)

type renameOp struct {
	c       *Component
	rhs     *vra.RequestHandlerState
	ops     renameOperators
	oldName string
	newName string
	ag      *models.ApplicationGroup
	cg      *models.ConsistencyGroup
	inError bool
}

type renameOperators interface {
	getInitialState(ctx context.Context) renameSubState
	saveInitialState(ctx context.Context)
	renameConsistencyGroup(ctx context.Context, oldName, newName string)
	renameApplicationGroup(ctx context.Context, oldName, newName string)
	renameVolumeSeries(ctx context.Context, oldName, newName string)
}

// vra.RenameHandlers methods

// Rename renames a new VolumeSeries object
func (c *Component) Rename(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &renameOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// UndoRename reverses the changes made by Rename
func (c *Component) UndoRename(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &renameOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// run executes the renaming state machine
func (op *renameOp) run(ctx context.Context) {
out:
	for ss := op.ops.getInitialState(ctx); !(op.rhs.InError || op.rhs.RetryLater); ss++ {
		switch ss {
		case RenameStart:
			op.ops.saveInitialState(ctx)
		case RenameVS:
			op.ops.renameVolumeSeries(ctx, op.oldName, op.newName)
		case RenameCG:
			op.ops.renameConsistencyGroup(ctx, op.oldName, op.newName)
		case RenameAG:
			op.ops.renameApplicationGroup(ctx, op.oldName, op.newName)
		case RenameDone:
			break out
		case RenameRevertAG:
			op.ops.renameApplicationGroup(ctx, op.newName, op.oldName)
		case RenameRevertCG:
			op.ops.renameConsistencyGroup(ctx, op.newName, op.oldName)
		case RenameRevertVS:
			op.ops.renameVolumeSeries(ctx, op.newName, op.oldName)
		default:
			break out
		}
	}
	if op.inError {
		op.rhs.InError = true
	}
}

func (op *renameOp) getInitialState(ctx context.Context) renameSubState {
	op.newName = string(op.rhs.Request.VolumeSeriesCreateSpec.Name)
	op.oldName = string(op.rhs.VolumeSeries.Name)
	prefix := com.SystemTagVsrOldName + ":"
	tagPresent := false
	for _, tag := range op.rhs.Request.SystemTags {
		if strings.HasPrefix(tag, prefix) {
			tagPresent = true
			op.oldName = strings.TrimPrefix(tag, prefix)
		}
	}
	if op.rhs.Request.VolumeSeriesRequestState == com.VolReqStateUndoRenaming {
		op.inError = op.rhs.InError
		op.rhs.InError = false // switch off to enable cleanup
		if tagPresent {
			op.loadGroups(ctx, op.newName)
			if op.rhs.RetryLater {
				return RenameDone
			}
			if op.ag != nil {
				return RenameRevertAG
			}
			if op.cg != nil {
				return RenameRevertCG
			}
			if string(op.rhs.VolumeSeries.Name) == op.newName {
				return RenameRevertVS
			}
		}
		return RenameDone
	}
	if op.rhs.Canceling || op.rhs.InError {
		return RenameDone
	}
	op.loadGroups(ctx, op.oldName)
	if op.rhs.RetryLater {
		return RenameDone
	}
	if !tagPresent {
		return RenameStart
	}
	if string(op.rhs.VolumeSeries.Name) == op.oldName {
		return RenameVS
	}
	if op.cg != nil {
		return RenameCG
	}
	if op.ag != nil {
		return RenameAG
	}
	return RenameDone
}

// loadGroups will attempt to load the ConsistencyGroup and ApplicationGroup objects,
// setting op.cg and op.ag respectively if their names match
// It will set RetryLater if the query fails
func (op *renameOp) loadGroups(ctx context.Context, matchName string) {
	if op.rhs.VolumeSeries.ConsistencyGroupID != "" {
		cg, err := op.c.oCrud.ConsistencyGroupFetch(ctx, string(op.rhs.VolumeSeries.ConsistencyGroupID))
		if err != nil {
			op.rhs.SetRequestMessage("Consistency group query error: %s", err.Error())
			op.rhs.RetryLater = true
			return
		} else if string(cg.Name) == matchName {
			op.cg = cg
		}
		for _, id := range cg.ApplicationGroupIds {
			ag, err := op.c.oCrud.ApplicationGroupFetch(ctx, string(id))
			if err != nil {
				op.rhs.SetRequestMessage("Application group query error: %s", err.Error())
				op.rhs.RetryLater = true
				return
			} else if string(ag.Name) == matchName {
				op.ag = ag
				break
			}
		}
	}
}

// save initial volume series name in the VSR to support restart
func (op *renameOp) saveInitialState(ctx context.Context) {
	tag := fmt.Sprintf("%s:%s", com.SystemTagVsrOldName, op.oldName)
	op.rhs.Request.SystemTags = append(op.rhs.Request.SystemTags, tag)
	if err := op.rhs.UpdateRequest(ctx); err != nil {
		// unlikely this would ever be saved
		op.rhs.SetRequestError("Failed to update VolumeSeriesRequest object: %s", err.Error())
	}
}

var errAlreadyRenamed = errors.New("already renamed")

func (op *renameOp) renameApplicationGroup(ctx context.Context, oldName, newName string) {
	if op.ag == nil {
		return
	}
	if swag.BoolValue(op.rhs.Request.PlanOnly) {
		op.rhs.SetRequestMessage("Would rename ApplicationGroup from %s to %s", oldName, newName)
		return
	}
	var lastAG *models.ApplicationGroup
	modAG := func(ag *models.ApplicationGroup) (*models.ApplicationGroup, error) {
		if ag == nil {
			ag = op.ag
		}
		lastAG = ag
		if ag.Name == models.ObjName(newName) {
			return nil, errAlreadyRenamed
		}
		ag.Name = models.ObjName(newName)
		return ag, nil
	}
	items := &crud.Updates{Set: []string{"name"}}
	ag, err := op.c.oCrud.ApplicationGroupUpdater(ctx, string(op.ag.Meta.ID), modAG, items)
	if err == errAlreadyRenamed {
		ag = lastAG
	} else if err != nil {
		// assume transient error, update unlikely to be saved
		op.rhs.SetRequestError("Failed to rename ApplicationGroup[%s]: %s", op.ag.Meta.ID, err.Error())
		return
	}
	op.ag = ag
	op.rhs.SetRequestMessage("Renamed ApplicationGroup[%s]: %s", op.ag.Meta.ID, newName)
}

func (op *renameOp) renameConsistencyGroup(ctx context.Context, oldName, newName string) {
	if op.cg == nil {
		return
	}
	if swag.BoolValue(op.rhs.Request.PlanOnly) {
		op.rhs.SetRequestMessage("Would rename ConsistencyGroup from %s to %s", oldName, newName)
		return
	}
	var lastCG *models.ConsistencyGroup
	modCG := func(cg *models.ConsistencyGroup) (*models.ConsistencyGroup, error) {
		if cg == nil {
			cg = op.cg
		}
		lastCG = cg
		if cg.Name == models.ObjName(newName) {
			return nil, errAlreadyRenamed
		}
		cg.Name = models.ObjName(newName)
		return cg, nil
	}
	items := &crud.Updates{Set: []string{"name"}}
	cg, err := op.c.oCrud.ConsistencyGroupUpdater(ctx, string(op.cg.Meta.ID), modCG, items)
	if err == errAlreadyRenamed {
		cg = lastCG
	} else if err != nil {
		// assume transient error, update unlikely to be saved
		op.rhs.SetRequestError("Failed to rename ConsistencyGroup[%s]: %s", op.cg.Meta.ID, err.Error())
		return
	}
	op.cg = cg
	op.rhs.SetRequestMessage("Renamed ConsistencyGroup[%s]: %s", op.cg.Meta.ID, newName)
}

func (op *renameOp) renameVolumeSeries(ctx context.Context, oldName, newName string) {
	if swag.BoolValue(op.rhs.Request.PlanOnly) {
		op.rhs.SetRequestMessage("Would rename VolumeSeries from %s to %s", oldName, newName)
		return
	}
	var lastVS *models.VolumeSeries
	modVS := func(vs *models.VolumeSeries) (*models.VolumeSeries, error) {
		if vs == nil {
			vs = op.rhs.VolumeSeries
		}
		lastVS = vs
		if vs.Name == models.ObjName(newName) {
			return nil, errAlreadyRenamed
		}
		vs.Name = models.ObjName(newName)
		vs.Messages = []*models.TimestampedString{
			&models.TimestampedString{
				Message: fmt.Sprintf("Renamed %s ⇒ %s", oldName, newName),
				Time:    strfmt.DateTime(time.Now()),
			},
		}
		return vs, nil
	}
	items := &crud.Updates{Set: []string{"name"}, Append: []string{"messages"}}
	vs, err := op.c.oCrud.VolumeSeriesUpdater(ctx, string(op.rhs.VolumeSeries.Meta.ID), modVS, items)
	if err == errAlreadyRenamed {
		vs = lastVS
	} else if err != nil {
		// assume transient error, update unlikely to be saved
		op.rhs.SetRequestError("Failed to rename VolumeSeries[%s]: %s", op.rhs.VolumeSeries.Meta.ID, err.Error())
		return
	}
	op.rhs.VolumeSeries = vs
	op.rhs.SetRequestMessage("Renamed VolumeSeries[%s]: %s", op.rhs.VolumeSeries.Meta.ID, newName)
}
