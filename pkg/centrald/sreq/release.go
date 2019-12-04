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

// releaseSubState represents sub-states in the release state machine.
// There is at most one database update in each sub-state.
type releaseSubState int

// releaseSubState values
const (
	// Tag the CSP Volume with the SR id and change the state of the Storage object to UNPROVISIONING
	RelSetStorageState releaseSubState = iota
	// Delete the CSP Volume and then delete the Storage object.
	RelDeleteStorage
	// Done
	RelDone
)

type releaseOp struct {
	c          *Component
	rhs        *requestHandlerState
	ops        releaseOperators
	wasInError bool
}

type releaseOperators interface {
	getInitialState(ctx context.Context) releaseSubState
	setStorageState(ctx context.Context)
	deleteStorage(ctx context.Context)
}

// ReleaseStorage returns storage
// This handler is called in both RELEASING and UNDO_PROVISIONING storage request states (see getInitialState)
func (c *Component) ReleaseStorage(ctx context.Context, rhs *requestHandlerState) {
	op := &releaseOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// run executes the provisioning state machine
func (op *releaseOp) run(ctx context.Context) {
out:
	for ss := op.ops.getInitialState(ctx); !(op.rhs.InError || op.rhs.RetryLater); ss++ {
		switch ss {
		case RelSetStorageState:
			op.ops.setStorageState(ctx)
		case RelDeleteStorage:
			op.ops.deleteStorage(ctx)
		case RelDone:
			break out
		}
	}
	op.rhs.InError = op.rhs.InError || op.wasInError
	op.rhs.Pool = nil               // do not attempt to clear the CSP volume SR tag
	op.rhs.DoNotSetStorageID = true // do not update the storageID property
}

// getInitialState determines the initial substate.
// It will recover from a partially completed release request.
func (op *releaseOp) getInitialState(ctx context.Context) releaseSubState {
	if op.rhs.Request.StorageRequestState == com.StgReqStateUndoProvisioning {
		op.wasInError = op.rhs.InError
		op.rhs.InError = false // temporarily reset so sub-state machine can run
	} else if op.rhs.TimedOut || op.rhs.InError {
		return RelDone
	}
	if op.rhs.Storage == nil {
		// cannot assume capacity was not returned
		return RelDone
	}
	if op.rhs.Storage.StorageState.ProvisionedState == com.StgProvisionedStateUnprovisioning {
		return RelDeleteStorage
	}
	return RelSetStorageState
}

// setStorageState will change the state of the Storage object to UNPROVISIONING
func (op *releaseOp) setStorageState(ctx context.Context) {
	rhs := op.rhs
	// update the Storage object state
	ss := rhs.Storage.StorageState
	if ss.ProvisionedState != com.StgProvisionedStateUnprovisioning {
		ts := &models.TimestampedString{
			Message: fmt.Sprintf("ProvisionedState change: %s ⇒ %s", ss.ProvisionedState, com.StgProvisionedStateUnprovisioning),
			Time:    strfmt.DateTime(time.Now()),
		}
		ss.ProvisionedState = com.StgProvisionedStateUnprovisioning
		ss.Messages = append(ss.Messages, ts)
		op.c.Log.Debugf("StorageRequest %s: Updating Storage %s: %s", rhs.Request.Meta.ID, rhs.Storage.Meta.ID, ts.Message)
		items := &crud.Updates{}
		items.Set = []string{"storageState"}
		if obj, err := op.c.oCrud.StorageUpdate(ctx, rhs.Storage, items); err == nil {
			rhs.Storage = obj
		} else {
			rhs.setAndUpdateRequestMessage(ctx, "Failed to update Storage state: %s", err.Error())
			rhs.RetryLater = true
		}
	}
}

// deleteStorage will delete the CSP Volume and then delete the Storage object if present
func (op *releaseOp) deleteStorage(ctx context.Context) {
	rhs := op.rhs
	var err error
	// StorageIdentifier can be empty if a StorageRequest fails during PROVISIONING
	if rhs.Storage.StorageIdentifier != "" {
		var dc csp.DomainClient
		if dc, err = op.c.App.AppCSP.GetDomainClient(rhs.CSPDomain); err != nil {
			rhs.setAndUpdateRequestMessage(ctx, "Domain client failure deleting volume: %s", err.Error())
			rhs.RetryLater = true
			return
		}
		vda := &csp.VolumeDeleteArgs{
			VolumeIdentifier:       rhs.Storage.StorageIdentifier,
			ProvisioningAttributes: rhs.Cluster.ClusterAttributes,
		}
		op.c.Log.Debugf("StorageRequest %s: Deleting CSP volume %s", rhs.Request.Meta.ID, vda.VolumeIdentifier)
		if err = dc.VolumeDelete(ctx, vda); err != nil {
			if err == csp.ErrorVolumeNotFound {
				op.c.Log.Debugf("StorageRequest %s: CSP volume %s already deleted", rhs.Request.Meta.ID, vda.VolumeIdentifier)
			} else {
				rhs.setAndUpdateRequestMessage(ctx, "Failed to delete CSP volume: %s", err.Error())
				rhs.RetryLater = true
				return
			}
		}
		op.c.Log.Infof("StorageRequest %s: Deleted CSP volume %s", rhs.Request.Meta.ID, vda.VolumeIdentifier)
	} else {
		op.c.Log.Debugf("StorageRequest %s: no CSP volume to delete", rhs.Request.Meta.ID)
	}
	// delete the Storage object
	if err = op.c.oCrud.StorageDelete(ctx, string(rhs.Storage.Meta.ID)); err == nil {
		rhs.setRequestMessage("Deleted Storage %s", rhs.Storage.Meta.ID)
	} else {
		rhs.setAndUpdateRequestMessage(ctx, "Failed to delete Storage: %s", err.Error())
		rhs.RetryLater = true
	}
}
