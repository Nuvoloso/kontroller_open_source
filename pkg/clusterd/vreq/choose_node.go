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

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/swag"
)

// chooseNodeSubState represents sizing sub-states.
// There is at most one database update in each sub-state.
type chooseNodeSubState int

// chooseNodeSubState values. Note that because NodeID cannot be changed once set, there is no undo path
const (
	// Populate the Request.storagePlan object
	ChooseNodeSelectHealthyNode chooseNodeSubState = iota
	// Return
	ChooseNodeDone

	// LAST: No operation is performed in this state.
	ChooseNodeNoOp
)

func (ss chooseNodeSubState) String() string {
	switch ss {
	case ChooseNodeSelectHealthyNode:
		return "ChooseNodeSelectHealthyNode"
	case ChooseNodeDone:
		return "ChooseNodeDone"
	}
	return fmt.Sprintf("chooseNodeSubState(%d)", ss)
}

type chooseNodeOperators interface {
	getInitialState(ctx context.Context) chooseNodeSubState
	selectHealthyNode(ctx context.Context)
}

type chooseNodeOp struct {
	c   *Component
	rhs *vra.RequestHandlerState
	ops chooseNodeOperators
}

// vra.AllocationHandlers methods

// ChooseNode implements the CHOOSING_NODE state of a VolumeSeriesRequest.
// It attempts to pick a healthy node when the request NodeID is empty.
func (c *Component) ChooseNode(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &chooseNodeOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// run executes the state machine
func (op *chooseNodeOp) run(ctx context.Context) {
out:
	for ss := op.ops.getInitialState(ctx); !(op.rhs.InError || op.rhs.RetryLater); ss++ {
		op.c.Log.Debugf("VolumeSeriesRequest %s: %s %s", op.rhs.Request.Meta.ID, op.rhs.Request.VolumeSeriesRequestState, ss)
		switch ss {
		case ChooseNodeSelectHealthyNode:
			op.ops.selectHealthyNode(ctx)
		// case SizeDone takes default
		default:
			break out
		}
	}
}

func (op *chooseNodeOp) getInitialState(ctx context.Context) chooseNodeSubState {
	if op.rhs.Request.NodeID != "" || op.rhs.Canceling || op.rhs.InError || op.rhs.TimedOut {
		return ChooseNodeDone
	}
	return ChooseNodeSelectHealthyNode
}

func (op *chooseNodeOp) selectHealthyNode(ctx context.Context) {
	if swag.BoolValue(op.rhs.Request.PlanOnly) {
		op.rhs.SetRequestMessage("Plan: choose a healthy node and set nodeId in VSR")
		return
	}
	if err := op.c.rei.ErrOnBool("choose-node-block-on-select"); err != nil {
		op.rhs.SetAndUpdateRequestMessageDistinct(ctx, "Blocked on [%s]", err.Error())
		op.rhs.RetryLater = true
		return
	}
	node := op.c.App.StateOps.GetHealthyNode("")
	if node == nil {
		op.rhs.SetAndUpdateRequestMessageDistinct(ctx, "waiting for a healthy node to become available")
		op.rhs.RetryLater = true
		return
	}
	op.rhs.Request.NodeID = models.ObjIDMutable(node.Meta.ID)
}
