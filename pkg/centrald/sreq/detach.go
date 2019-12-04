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
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/strfmt"
)

// detachSubState represents detaching sub-states.
// There is at most one database update in each sub-state.
type detachSubState int

// detachSubState values
const (
	// Put the storage object into the DETACHING state
	DetachStorageStart detachSubState = iota
	// Issue the CSP operation to detach a Volume and then update the Storage object with the result
	DetachCSPVolume
	// Update the Storage object
	DetachSetDetached
	// Return
	DetachDone
	// Put storage into ERROR state
	DetachCleanup
)

type detachOp struct {
	c          *Component
	rhs        *requestHandlerState
	ops        detachOperators
	wasInError bool
}

type detachOperators interface {
	getInitialState(ctx context.Context) detachSubState
	setDetached(ctx context.Context)
	setDetaching(ctx context.Context)
	detachStorage(ctx context.Context)
	cleanup(ctx context.Context)
}

// DetachStorage removes storage from a node
// This handler is called in both DETACHING and UNDO_ATTACHING storage request states (see getInitialState)
func (c *Component) DetachStorage(ctx context.Context, rhs *requestHandlerState) {
	op := &detachOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// UndoDetachStorage transitions the storage to ERROR
func (c *Component) UndoDetachStorage(ctx context.Context, rhs *requestHandlerState) {
	op := &detachOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.ops.cleanup(ctx)
}

// run executes the detaching state machine
func (op *detachOp) run(ctx context.Context) {
out:
	for ss := op.ops.getInitialState(ctx); !(op.rhs.InError || op.rhs.RetryLater); ss++ {
		switch ss {
		case DetachStorageStart:
			op.ops.setDetaching(ctx)
		case DetachCSPVolume:
			op.ops.detachStorage(ctx)
		case DetachSetDetached:
			op.ops.setDetached(ctx)
		case DetachDone:
			break out
		case DetachCleanup:
			op.ops.cleanup(ctx)
		default:
			break out
		}
	}
	op.rhs.InError = op.rhs.InError || op.wasInError
}

func (op *detachOp) getInitialState(ctx context.Context) detachSubState {
	if op.rhs.Request.StorageRequestState == com.StgReqStateUndoAttaching {
		op.wasInError = op.rhs.InError
		op.rhs.InError = false // temporarily reset so sub-state machine can run
	} else if op.rhs.TimedOut {
		return DetachCleanup
	}
	if op.rhs.InError {
		return DetachDone
	}
	op.c.Log.Debugf("StorageRequest %s: Storage [%s] StorageIdentifier=%s AttachmentState=%s", op.rhs.Request.Meta.ID, op.rhs.Storage.Meta.ID, op.rhs.Storage.StorageIdentifier, op.rhs.Storage.StorageState.AttachmentState)
	if op.rhs.Storage.StorageState.AttachmentState == com.StgAttachmentStateDetaching {
		return DetachCSPVolume
	} else if op.rhs.Storage.StorageState.AttachmentState == com.StgAttachmentStateDetached {
		return DetachDone
	} else if op.rhs.Storage.StorageState.AttachmentState == com.StgAttachmentStateAttaching {
		return DetachCSPVolume
	}
	return DetachStorageStart
}

func (op *detachOp) setDetaching(ctx context.Context) {
	rhs := op.rhs
	ts := &models.TimestampedString{
		Message: fmt.Sprintf("AttachmentState change: %s ⇒ %s", rhs.Storage.StorageState.AttachmentState, com.StgAttachmentStateDetaching),
		Time:    strfmt.DateTime(time.Now()),
	}
	rhs.Storage.StorageState.AttachmentState = com.StgAttachmentStateDetaching
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

func (op *detachOp) detachStorage(ctx context.Context) {
	rhs := op.rhs
	var dc csp.DomainClient
	var err error
	if err = op.c.rei.ErrOnBool("detach-storage-fail"); err != nil {
		op.rhs.setAndUpdateRequestMessage(ctx, "rei error:%s", err.Error())
		rhs.RetryLater = true
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
	if dc, err = op.c.App.AppCSP.GetDomainClient(rhs.CSPDomain); err != nil {
		rhs.setAndUpdateRequestMessage(ctx, "Domain client failure: %s", err.Error())
		rhs.RetryLater = true
		return
	}
	vda := &csp.VolumeDetachArgs{
		VolumeIdentifier:       rhs.Storage.StorageIdentifier,
		NodeIdentifier:         node.NodeIdentifier,
		ProvisioningAttributes: node.NodeAttributes,
	}
	forceMsg := ""
	if val, found := util.NewTagList(rhs.Request.SystemTags).Get(com.SystemTagForceDetachNodeID); found && val == string(rhs.Storage.StorageState.AttachedNodeID) {
		vda.Force = true
		forceMsg = " (forced)"
	}
	if _, err = dc.VolumeDetach(ctx, vda); err != nil {
		if err == csp.ErrorVolumeNotAttached {
			op.c.Log.Debugf("StorageRequest %s: CSP volume %s already detached", rhs.Request.Meta.ID, rhs.Storage.StorageIdentifier)
		} else {
			rhs.setAndUpdateRequestMessage(ctx, "Failed to detach CSP volume: %s", err.Error())
			rhs.RetryLater = true
			return
		}
	}
	rhs.setRequestMessage("Detached volume %s%s", rhs.Storage.StorageIdentifier, forceMsg)
}

func (op *detachOp) setDetached(ctx context.Context) {
	rhs := op.rhs
	forceMsg := ""
	if val, found := util.NewTagList(rhs.Request.SystemTags).Get(com.SystemTagForceDetachNodeID); found && val == string(rhs.Storage.StorageState.AttachedNodeID) {
		forceMsg = " (forced)"
	}
	ts := &models.TimestampedString{
		Message: fmt.Sprintf("AttachmentState change: %s ⇒ %s%s", rhs.Storage.StorageState.AttachmentState, com.StgAttachmentStateDetached, forceMsg),
		Time:    strfmt.DateTime(time.Now()),
	}
	rhs.Storage.StorageState.AttachmentState = com.StgAttachmentStateDetached
	rhs.Storage.StorageState.AttachedNodeDevice = ""
	rhs.Storage.StorageState.AttachedNodeID = ""
	if forceMsg != "" {
		rhs.Storage.StorageState.DeviceState = com.StgDeviceStateUnused
	}
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

// cleanup should transition the Storage to ERROR
func (op *detachOp) cleanup(ctx context.Context) {
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
