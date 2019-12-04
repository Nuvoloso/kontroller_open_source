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
	"github.com/go-openapi/swag"
)

// provSubState represents provisioning sub-states.
// There is at most one database update in each sub-state.
type provSubState int

// provSubState values
const (
	// Find a pool and allocate some capacity
	// If not found then set the request state to CAPACITY_WAIT and retry.
	ProvGetCapacity provSubState = iota
	// Create a storage object in the PROVISIONING state
	ProvCreateStorage
	// Set the poolId and the storageId atomically in the database
	ProvSetIds
	// Issue the CSP operation to create a Volume and then update the Storage object with the result
	ProvSetStorageProvisioned
	// Return
	ProvDone
	// Remove storage request tag from CSP volume
	ProvRemoveTag
	// Done removing tag
	ProvRemoveTagDone
	// Release provisioned resources
	ProvTimedOut
)

type provisionOp struct {
	c              *Component
	rhs            *requestHandlerState
	sizeBytes      int64
	pool           *models.Pool
	idsWerePresent bool
	ops            provisionOperators
}

type provisionOperators interface {
	getInitialState(ctx context.Context) provSubState
	getCapacity(ctx context.Context)
	createStorage(ctx context.Context)
	setIds(ctx context.Context)
	createCSPVolumeSetStorageProvisioned(ctx context.Context)
	removeCSPVolumeTag(ctx context.Context)
	timedOutCleanup(ctx context.Context)
}

// ProvisionStorage obtains storage
func (c *Component) ProvisionStorage(ctx context.Context, rhs *requestHandlerState) {
	op := &provisionOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// RemoveTag removes the CSP volume tag set during provisioning
func (c *Component) RemoveTag(ctx context.Context, rhs *requestHandlerState) {
	op := &provisionOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// run executes the provisioning state machine
func (op *provisionOp) run(ctx context.Context) {
out:
	for ss := op.ops.getInitialState(ctx); !(op.rhs.InError || op.rhs.RetryLater); ss++ {
		switch ss {
		case ProvGetCapacity:
			op.ops.getCapacity(ctx)
		case ProvCreateStorage:
			op.ops.createStorage(ctx)
		case ProvSetIds:
			op.ops.setIds(ctx)
		case ProvSetStorageProvisioned:
			op.ops.createCSPVolumeSetStorageProvisioned(ctx)
		case ProvDone:
			break out
		case ProvRemoveTag:
			op.ops.removeCSPVolumeTag(ctx)
		case ProvRemoveTagDone:
			break out
		case ProvTimedOut:
			op.ops.timedOutCleanup(ctx)
		default:
			break out
		}
	}
}

// getInitialState determines the initial provisioning substate.
// It will recover from a partially completed provisioning request.
func (op *provisionOp) getInitialState(ctx context.Context) provSubState {
	if op.rhs.TimedOut {
		return ProvTimedOut
	}
	if op.rhs.InError {
		return ProvDone
	}
	if op.rhs.Request.StorageRequestState == com.StgReqStateRemovingTag {
		return ProvRemoveTag
	}
	if op.rhs.Storage != nil && op.rhs.Pool != nil {
		s := op.rhs.Storage
		op.sizeBytes = swag.Int64Value(s.SizeBytes)
		if op.rhs.Request.PoolID == s.PoolID &&
			models.ObjID(op.rhs.Request.StorageID) == s.Meta.ID {
			op.idsWerePresent = true
			if s.StorageState != nil && s.StorageState.ProvisionedState == com.StgProvisionedStateProvisioned {
				op.c.Log.Debugf("StorageRequest %s: Storage %s already provisioned", op.rhs.Request.Meta.ID, s.StorageIdentifier)
				return ProvDone
			}
			return ProvSetStorageProvisioned
		}
		return ProvSetIds
	}
	return ProvGetCapacity
}

// getCapacity obtains capacity from a pool.
// The Pool object is updated in the database.
func (op *provisionOp) getCapacity(ctx context.Context) {
	var err error
	rhs := op.rhs
	sr := rhs.Request
	var dc csp.DomainClient
	if dc, err = op.c.App.AppCSP.GetDomainClient(rhs.CSPDomain); err != nil {
		rhs.setAndUpdateRequestMessage(ctx, "Domain client failure: %s", err.Error())
		rhs.RetryLater = true
		return
	}
	// get the actual size that will be provisioned
	if op.sizeBytes, err = dc.VolumeSize(ctx, models.CspStorageType(sr.CspStorageType), swag.Int64Value(sr.MinSizeBytes)); err != nil {
		op.rhs.setRequestError("VolumeSize: %s", err.Error())
		return
	}
	// fetch the pool object
	op.c.Log.Debugf("StorageRequest %s fetching Pool %s", sr.Meta.ID, sr.PoolID)
	op.pool, err = op.c.oCrud.PoolFetch(ctx, string(sr.PoolID))
	if err != nil {
		rhs.setRequestMessage("Error fetching pool: %s", err.Error())
		rhs.RetryLater = true
		return
	}
	op.rhs.setRequestMessage("Provisioned %d bytes from '%s'", op.sizeBytes, op.pool.Meta.ID)
}

// createStorage creates a Storage object in the database in the PROVISIONING state.
func (op *provisionOp) createStorage(ctx context.Context) {
	storage := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			SizeBytes:      swag.Int64(op.sizeBytes),
			CspStorageType: models.CspStorageType(op.rhs.Request.CspStorageType),
			PoolID:         models.ObjIDMutable(op.pool.Meta.ID),
		},
		StorageMutable: models.StorageMutable{
			AvailableBytes:   swag.Int64(op.sizeBytes),
			ShareableStorage: op.rhs.Request.ShareableStorage,
			StorageState: &models.StorageStateMutable{
				AttachmentState:  com.StgAttachmentStateDetached,
				ProvisionedState: com.StgProvisionedStateProvisioning,
			},
		},
	}
	var err error
	if err = op.c.rei.ErrOnBool("provision-fail-storage-create"); err != nil {
		op.rhs.setRequestError("Failed to create Storage object: %s", err.Error())
		return
	} else if err = op.c.rei.ErrOnBool("provision-block-storage-create"); err != nil {
		op.rhs.setAndUpdateRequestMessageDistinct(ctx, "CreateStorage: %s", err.Error())
		op.rhs.RetryLater = true // we can attempt again
		return
	}
	op.rhs.Storage, err = op.c.oCrud.StorageCreate(ctx, storage)
	if err != nil {
		op.rhs.setRequestError("Failed to create Storage object: %s", err.Error())
	}
}

// setIds updates the StorageRequest with the IDs of the Pool and Storage objects.
func (op *provisionOp) setIds(ctx context.Context) {
	op.rhs.Request.StorageID = models.ObjIDMutable(op.rhs.Storage.Meta.ID)
	items := &crud.Updates{}
	items.Set = []string{"storageRequestState", "requestMessages", "storageId"}
	modFn := func(o *models.StorageRequest) (*models.StorageRequest, error) {
		if o == nil {
			o = op.rhs.Request
		}
		sr := op.rhs.Request
		o.StorageRequestState = sr.StorageRequestState
		o.RequestMessages = sr.RequestMessages
		o.StorageID = sr.StorageID
		return o, nil
	}
	obj, err := op.c.oCrud.StorageRequestUpdater(ctx, string(op.rhs.Request.Meta.ID), modFn, items)
	if err == nil {
		op.rhs.Request = obj  // replace only if no error
		op.rhs.Pool = op.pool // expose for later operations
		op.pool = nil         // clear internally for consistency
	} else {
		// unlikely this would ever be saved
		op.rhs.setRequestError("Failed to update StorageRequest object: %s", err.Error())
	}
}

// createCSPVolumeSetStorageProvisioned issues the CSP operation to create a volume.
// It then updates the Storage object state to PROVISIONED.
func (op *provisionOp) createCSPVolumeSetStorageProvisioned(ctx context.Context) {
	rhs := op.rhs
	sr := rhs.Request
	var err error
	var dc csp.DomainClient
	if dc, err = op.c.App.AppCSP.GetDomainClient(rhs.CSPDomain); err != nil {
		op.rhs.setAndUpdateRequestMessage(ctx, "Domain client failure: %s", err.Error())
		rhs.RetryLater = true
		return
	}
	tags := []string{
		com.VolTagSystem + ":" + op.c.systemID,
		com.VolTagStorageID + ":" + string(rhs.Storage.Meta.ID),
		com.VolTagPoolID + ":" + string(op.rhs.Pool.Meta.ID),
		com.VolTagStorageRequestID + ":" + string(sr.Meta.ID),
	}
	var vol *csp.Volume
	// idsWerePresent && !Storage.StorageIdentifier ⇒ Did we previously issue a VolumeCreate?
	if op.idsWerePresent {
		vla := &csp.VolumeListArgs{
			StorageTypeName:        models.CspStorageType(sr.CspStorageType),
			Tags:                   tags,
			ProvisioningAttributes: rhs.Cluster.ClusterAttributes,
		}
		op.c.Log.Debugf("Checking if CSP Volume exists: tags=%v", tags)
		vols, err := dc.VolumeList(ctx, vla)
		if err == nil && len(vols) > 0 {
			op.c.Log.Warningf("StorageRequest %s: found %d CSP Volumes with tags=%v", sr.Meta.ID, len(vols), tags)
			for _, v := range vols {
				op.c.Log.Debugf("Found volume %s: %s %d", v.Identifier, v.ProvisioningState, v.SizeBytes)
				// select the first that is provisioned
				if vol == nil && v.ProvisioningState == csp.VolumeProvisioningProvisioned {
					vol = v
					op.c.Log.Infof("StorageRequest %s: recovered CSP Volume %s", sr.Meta.ID, vol.Identifier)
				}
			}
		} else if err != nil {
			op.c.Log.Debugf("Ignoring error in VolumeList: %s", err.Error())
		}
	}
	if vol == nil {
		vca := &csp.VolumeCreateArgs{
			StorageTypeName:        models.CspStorageType(sr.CspStorageType),
			SizeBytes:              op.sizeBytes,
			Tags:                   tags,
			ProvisioningAttributes: rhs.Cluster.ClusterAttributes,
		}
		err = op.c.rei.ErrOnBool("provision-fail-csp-volume-create")
		if err == nil {
			vol, err = dc.VolumeCreate(ctx, vca)
		}
		if err != nil {
			op.rhs.setRequestError("Error: Failed to create CSP volume: %s", err.Error())
			return
		}
	}
	rhs.setRequestMessage("Created volume %s", vol.Identifier)
	// update the Storage object state
	ts := &models.TimestampedString{
		Message: fmt.Sprintf("ProvisionedState change: %s ⇒ %s", rhs.Storage.StorageState.ProvisionedState, com.StgProvisionedStateProvisioned),
		Time:    strfmt.DateTime(time.Now()),
	}
	rhs.Storage.StorageState.ProvisionedState = com.StgProvisionedStateProvisioned
	rhs.Storage.StorageState.Messages = append(rhs.Storage.StorageState.Messages, ts)
	rhs.Storage.StorageIdentifier = vol.Identifier
	items := &crud.Updates{}
	items.Set = []string{"storageState", "storageIdentifier"}
	sObj, err := op.c.oCrud.StorageUpdate(ctx, rhs.Storage, items)
	if err == nil { // succeed
		rhs.Storage = sObj
	} else { // fail
		rhs.setRequestError("Failed to update Storage state: %s", err.Error())
		op.c.Log.Debugf("TBD: Volume and Storage to be cleaned up elsewhere") // TBD
	}
}

// removeCSPVolumeTag removes storage request tag from the CSP volume, best effort
func (op *provisionOp) removeCSPVolumeTag(ctx context.Context) {
	dc, err := op.c.App.AppCSP.GetDomainClient(op.rhs.CSPDomain)
	if err != nil {
		op.rhs.setAndUpdateRequestMessage(ctx, "Domain client failure: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	vta := &csp.VolumeTagArgs{
		VolumeIdentifier: op.rhs.Storage.StorageIdentifier,
		Tags: []string{
			com.VolTagStorageRequestID + ":" + string(op.rhs.Request.Meta.ID),
		},
	}
	op.rhs.c.Log.Debugf("VolumeTagsDelete %s: %v", vta.VolumeIdentifier, vta.Tags)
	if _, err = dc.VolumeTagsDelete(ctx, vta); err != nil {
		op.rhs.c.Log.Debugf("Ignoring VolumeTagsDelete error: %s", err.Error())
	}
}

// timedOutCleanup releases provisioned resources
func (op *provisionOp) timedOutCleanup(ctx context.Context) {
	if op.rhs.Storage != nil {
		op.sizeBytes = swag.Int64Value(op.rhs.Storage.SizeBytes)
		// delete the Storage object before we return capacity to the pool
		if err := op.c.oCrud.StorageDelete(ctx, string(op.rhs.Storage.Meta.ID)); err != nil {
			op.c.Log.Warningf("Failed to delete Storage object %s", op.rhs.Storage.Meta.ID)
		} else {
			op.rhs.Storage = nil
		}
	}
	op.sizeBytes = 0
}
