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


package sreq

import (
	"context"
	"fmt"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/go-openapi/strfmt"
)

// attachSubState represents attaching sub-states.
// There is at most one database update in each sub-state.
type attachSubState int

// attachSubState values
const (
	// Put the storage object into the ATTACHING state
	AttachStorageStart attachSubState = iota
	// Issue the CSP operation to attach a Volume and then update the Storage object with the result
	AttachSetStorageAttached
	// Return
	AttachDone
	// Put storage into ERROR state
	AttachTimedOut
)

type attachOp struct {
	c   *Component
	rhs *requestHandlerState
	ops attachOperators
}

type attachOperators interface {
	getInitialState(ctx context.Context) attachSubState
	setAttaching(ctx context.Context)
	attachStorage(ctx context.Context)
	timedOutCleanup(ctx context.Context)
}

// AttachStorage makes storage available on a node
func (c *Component) AttachStorage(ctx context.Context, rhs *requestHandlerState) {
	op := &attachOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// run executes the attaching state machine
func (op *attachOp) run(ctx context.Context) {
out:
	for ss := op.ops.getInitialState(ctx); !(op.rhs.InError || op.rhs.RetryLater); ss++ {
		switch ss {
		case AttachStorageStart:
			op.ops.setAttaching(ctx)
		case AttachSetStorageAttached:
			op.ops.attachStorage(ctx)
		case AttachDone:
			break out
		case AttachTimedOut:
			op.ops.timedOutCleanup(ctx)
		default:
			break out
		}
	}
}

func (op *attachOp) getInitialState(ctx context.Context) attachSubState {
	if op.rhs.TimedOut {
		return AttachTimedOut
	}
	if op.rhs.InError {
		return AttachDone
	}
	if op.rhs.Storage.StorageState.AttachmentState == com.StgAttachmentStateAttaching {
		return AttachSetStorageAttached
	} else if op.rhs.Storage.StorageState.AttachmentState == com.StgAttachmentStateAttached {
		op.c.Log.Debugf("StorageRequest %s: Storage %s already attached", op.rhs.Request.Meta.ID, op.rhs.Storage.StorageIdentifier)
		return AttachDone
	}
	return AttachStorageStart
}

func (op *attachOp) setAttaching(ctx context.Context) {
	if err := op.c.rei.ErrOnBool("block-before-attach"); err != nil {
		op.rhs.setAndUpdateRequestMessageDistinct(ctx, "Blocked on %s", err.Error())
		op.rhs.RetryLater = true // we can attempt again
		return
	}
	rhs := op.rhs
	ts := &models.TimestampedString{
		Message: fmt.Sprintf("AttachmentState change: %s ⇒ %s", rhs.Storage.StorageState.AttachmentState, com.StgAttachmentStateAttaching),
		Time:    strfmt.DateTime(time.Now()),
	}
	rhs.Storage.StorageState.AttachmentState = com.StgAttachmentStateAttaching
	rhs.Storage.StorageState.AttachedNodeID = rhs.Request.NodeID
	rhs.Storage.StorageState.AttachedNodeDevice = ""
	rhs.Storage.StorageState.Messages = append(rhs.Storage.StorageState.Messages, ts)
	items := &crud.Updates{}
	items.Set = []string{"storageState"}
	sObj, err := op.c.oCrud.StorageUpdate(ctx, rhs.Storage, items)
	if err == nil { // succeed
		rhs.Storage = sObj
	} else {
		// assume db error
		rhs.setAndUpdateRequestMessage(ctx, "Storage update failure: %s", err.Error())
		rhs.RetryLater = true
	}
}

func (op *attachOp) attachStorage(ctx context.Context) {
	rhs := op.rhs
	if err := op.c.rei.ErrOnBool("block-in-attach"); err != nil {
		op.rhs.setAndUpdateRequestMessageDistinct(ctx, "Blocked on %s", err.Error())
		op.rhs.RetryLater = true // we can attempt again
		return
	}
	// only have nodeID, need CSP node identifier
	node, err := op.c.oCrud.NodeFetch(ctx, string(rhs.Request.NodeID))
	if err != nil {
		// assume db error
		rhs.setAndUpdateRequestMessage(ctx, "Node %s load failure: %s", rhs.Request.NodeID, err.Error())
		rhs.RetryLater = true
		return
	}
	var dc csp.DomainClient
	if dc, err = op.c.App.AppCSP.GetDomainClient(rhs.CSPDomain); err != nil {
		rhs.setAndUpdateRequestMessage(ctx, "Domain client failure: %s", err.Error())
		rhs.RetryLater = true
		return
	}
	vaa := &csp.VolumeAttachArgs{
		VolumeIdentifier:       rhs.Storage.StorageIdentifier,
		NodeIdentifier:         node.NodeIdentifier,
		ProvisioningAttributes: node.NodeAttributes,
	}
	var vol *csp.Volume
	// VolumeAttach handles the case where the volume is already attached and just returns the updated vol objects
	if vol, err = dc.VolumeAttach(ctx, vaa); err != nil {
		rhs.setRequestError("Failed to attach CSP volume: %s", err.Error())
		return
	}
	rhs.setRequestMessage("Attached storage %s", rhs.Storage.Meta.ID)
	// update the Storage object state
	ts := &models.TimestampedString{
		Message: fmt.Sprintf("AttachmentState change: %s ⇒ %s", rhs.Storage.StorageState.AttachmentState, com.StgAttachmentStateAttached),
		Time:    strfmt.DateTime(time.Now()),
	}
	rhs.Storage.StorageState.AttachmentState = com.StgAttachmentStateAttached
	rhs.Storage.StorageState.AttachedNodeDevice = vol.Attachments[0].Device
	rhs.Storage.StorageState.Messages = append(rhs.Storage.StorageState.Messages, ts)
	items := &crud.Updates{}
	items.Set = []string{"storageState"}
	sObj, err := op.c.oCrud.StorageUpdate(ctx, rhs.Storage, items)
	if err != nil { // assume db error
		rhs.setAndUpdateRequestMessage(ctx, "Failed to update Storage state: %s", err.Error())
		rhs.RetryLater = true
		return
	}
	rhs.Storage = sObj
}

// timedOutCleanup should transition the Storage to ERROR
func (op *attachOp) timedOutCleanup(ctx context.Context) {
	rhs := op.rhs
	// update the Storage object state
	ts := &models.TimestampedString{
		Message: fmt.Sprintf("AttachmentState change: %s ⇒ %s", rhs.Storage.StorageState.AttachmentState, com.StgAttachmentStateError),
		Time:    strfmt.DateTime(time.Now()),
	}
	rhs.Storage.StorageState.AttachmentState = com.StgAttachmentStateError
	rhs.Storage.StorageState.Messages = append(rhs.Storage.StorageState.Messages, ts)
	items := &crud.Updates{}
	items.Set = []string{"storageState"}
	sObj, err := op.c.oCrud.StorageUpdate(ctx, rhs.Storage, items)
	if err != nil {
		op.c.Log.Warningf("Failed to update Storage state of %s", rhs.Storage.Meta.ID)
	} else {
		rhs.Storage = sObj
	}
}
