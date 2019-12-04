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


package vra

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/strfmt"
)

// RequestHandlerState is the in-memory state of a volume series request.
// The animator will invoke the required handlers in order with this state structure.
// After the last handler returns the animator will set the request state to SUCCEEDED, FAILED or CANCELED depending
// on the values of InError, UndoInError and Canceling.
// If the RetryLater flag is set then remaining handlers are not called and the request updated and discarded.
// On entry, if the completion time has expired the request will be failed.
type RequestHandlerState struct {
	// A is the Animator
	A *Animator
	// VSUpdater is a helper to update the VolumeSeries.VolumeSeriesState
	VSUpdater VolumeUpdater
	// Request is the current request object.
	// A handler is responsible for saving the state in the database and updating the in-memory contents.
	Request *models.VolumeSeriesRequest
	// VolumeSeries is the associated volume series object.
	// The animator will load the VolumeSeries object if the VolumeSeriesID is set.
	VolumeSeries *models.VolumeSeries
	// If set the request has timed out.  This is set only on dispatch.
	// The concerned handlers will be called in reverse order to clean up.
	TimedOut bool
	// RetryLater should be set if a handler decides that further processing is not possible.
	// The request is not updated and will be re-loaded on the next iteration.
	RetryLater bool
	// InError should be set if the handler encounters an error.
	// The concerned handlers will be called in reverse order to clean up, from the
	// failing state handler backward. They should update this structure as they undo,
	// as not all earlier operations can be undone as objects/states change.
	// After undo completes, the animator will set the state of the request to FAILED if not already set.
	InError bool
	// AbortUndo should be set when an error occurs on the undo path and the
	// undo cannot continue. The animator will skip any remaining undo steps and transition
	// the request to FAILED. Setting this flag during normal processing is ignored.
	AbortUndo bool
	// Canceling should be set when the handler detects that cancel was requested.
	Canceling bool
	// The types of operations requested. They will be processed in a fixed order.
	HasAllocateCapacity, HasChangeCapacity, HasCreate, HasCreateFromSnapshot, HasBind, HasConfigure, HasPublish, HasMount, HasUnmount, HasUnpublish, HasDelete, HasDeleteSPA, HasRename, HasCGSnapshotCreate, HasVolSnapshotCreate, HasVolSnapshotRestore, HasAttachFs, HasDetachFs, HasUnbind, HasNodeDelete, HasVolDetach bool
	// originalCgID Tracks the ConsistencyGroupID across handler calls to determine if the update items should include it
	originalCgID models.ObjIDMutable
	// originalNodeID Tracks the NodeID across handler calls to determine if the update items should include it
	originalNodeID models.ObjIDMutable
	// originalSpaID Tracks the ServicePlanAllocationID across handler calls to determine if the update items should include it
	originalSpaID models.ObjIDMutable
	// originalVsID Tracks the VolumeSeriesID across handler calls to determine if the update items should include it
	originalVsID models.ObjIDMutable
	// Stash can be used by handlers that are invoked to handle multiple request states.
	Stash map[interface{}]interface{}
	// control whether the TimedOut message is issued
	suppressTimedOutMessage bool
	// order of current state
	currentStateOrder int
}

// AllocateCapacityHandlers is an interface to set capacity in a ServicePlanAllocation object or delete them
// and create or delete associated Pool objects
type AllocateCapacityHandlers interface {
	AllocateCapacity(context.Context, *RequestHandlerState)
	UndoAllocateCapacity(context.Context, *RequestHandlerState)
}

// AllocationHandlers is an interface for processing the allocation steps of mount operations.
type AllocationHandlers interface {
	ChooseNode(context.Context, *RequestHandlerState)
	Size(context.Context, *RequestHandlerState)
	ResizeCache(context.Context, *RequestHandlerState)
	Place(context.Context, *RequestHandlerState)
	UndoPlace(context.Context, *RequestHandlerState)
	UndoResizeCache(context.Context, *RequestHandlerState)
	UndoSize(context.Context, *RequestHandlerState)
}

// AttachFsHandlers is an interface for processing host filesystem mounts
type AttachFsHandlers interface {
	AttachFs(context.Context, *RequestHandlerState)
	UndoAttachFs(context.Context, *RequestHandlerState)
}

// BindHandlers is an interface for processing individual volume series bind operations.
type BindHandlers interface {
	Bind(context.Context, *RequestHandlerState)
	UndoBind(context.Context, *RequestHandlerState)
}

// CGSnapshotCreateHandlers is an interface for processing consistency group snapshot operations.
type CGSnapshotCreateHandlers interface {
	CGSnapshotCreate(context.Context, *RequestHandlerState)
	UndoCGSnapshotCreate(context.Context, *RequestHandlerState)
}

// ChangeCapacityHandlers is an interface for processing individual volume series change capacity operations.
type ChangeCapacityHandlers interface {
	ChangeCapacity(context.Context, *RequestHandlerState)
	UndoChangeCapacity(context.Context, *RequestHandlerState)
}

// CreateHandlers is an interface for processing individual volume series create operations.
type CreateHandlers interface {
	Create(context.Context, *RequestHandlerState)
	UndoCreate(context.Context, *RequestHandlerState)
}

// CreateFromSnapshotHandlers is an interface for volume series snapshot clone operations.
type CreateFromSnapshotHandlers interface {
	CreateFromSnapshot(context.Context, *RequestHandlerState)
	UndoCreateFromSnapshot(context.Context, *RequestHandlerState)
}

// MountHandlers is an interface for processing the volume configuration and export steps of mount operations.
type MountHandlers interface {
	Configure(context.Context, *RequestHandlerState)
	Export(context.Context, *RequestHandlerState)
	ReallocateCache(context.Context, *RequestHandlerState)
	UndoReallocateCache(context.Context, *RequestHandlerState)
	UndoExport(context.Context, *RequestHandlerState)
	UndoConfigure(context.Context, *RequestHandlerState)
}

// NodeDeleteHandlers is an interface to process the NODE_DELETE operation
type NodeDeleteHandlers interface {
	NodeDelete(context.Context, *RequestHandlerState)
}

// PublishHandlers is an interface for processing individual volume series publish operations.
type PublishHandlers interface {
	Publish(context.Context, *RequestHandlerState)
	UndoPublish(context.Context, *RequestHandlerState)
}

// PublishServicePlanHandlers is an interface for processing service plan publish operations.
type PublishServicePlanHandlers interface {
	PublishServicePlan(context.Context, *RequestHandlerState)
	// no undo
}

// RenameHandlers is an interface for processing individual volume series rename operations.
type RenameHandlers interface {
	Rename(context.Context, *RequestHandlerState)
	UndoRename(context.Context, *RequestHandlerState)
}

// VolumeDetachHandlers is an interface to process the VOL_DETACH operation
type VolumeDetachHandlers interface {
	VolDetach(context.Context, *RequestHandlerState)
}

// VolSnapshotCreateHandlers is an interface for processing volume series snapshot operations.
type VolSnapshotCreateHandlers interface {
	VolSnapshotCreate(context.Context, *RequestHandlerState)
	UndoVolSnapshotCreate(context.Context, *RequestHandlerState)
}

// VolSnapshotRestoreHandlers is an interface for processing volume series snapshot operations.
type VolSnapshotRestoreHandlers interface {
	VolSnapshotRestore(ctx context.Context, rhs *RequestHandlerState)
	UndoVolSnapshotRestore(ctx context.Context, rhs *RequestHandlerState)
}

// VolumeUpdater modifies the VolumeSeries for this request
type VolumeUpdater interface {
	SetVolumeState(context.Context, string)
	SetVolumeSystemTags(context.Context, ...string)
	RemoveVolumeSystemTags(context.Context, ...string)
}

// SetRequestError appends a message in the VolumeSeriesRequest object and logs it at Error level.
// It also sets the rhs.InError or rhs.UndoInError flag.
func (rhs *RequestHandlerState) SetRequestError(format string, args ...interface{}) string {
	msg := rhs._setRequestMessage(format, args...)
	rhs.A.Log.Errorf("VolumeSeriesRequest %s: %s", rhs.Request.Meta.ID, msg)
	rhs.InError = true
	return msg
}

// SetAndUpdateRequestError appends a message in the VolumeSeriesRequest object and logs it at Error level.
// It also sets the rhs.InError or rhs.UndoInError flag.
// The rhs.Canceling flag may set as a side-effect of updating the request object.
func (rhs *RequestHandlerState) SetAndUpdateRequestError(ctx context.Context, format string, args ...interface{}) (string, error) {
	msg := rhs.SetRequestError(format, args...)
	err := rhs.UpdateRequest(ctx)
	return msg, err
}

// SetAndUpdateRequestMessage will set a message and update the database with the request object (best effort, no guarantee).
// The rhs.Canceling flag may set as a side-effect of updating the request object.
func (rhs *RequestHandlerState) SetAndUpdateRequestMessage(ctx context.Context, format string, args ...interface{}) (string, error) {
	msg := rhs.SetRequestMessage(format, args...)
	err := rhs.UpdateRequest(ctx)
	return msg, err
}

// SetAndUpdateRequestMessageDistinct will set a message and update the database with the request object (best effort, no guarantee)
// if the message does not already exist.  This is useful for cases where the VSR is known to retry on the same condition
// multiple times, such as in persistent REI injection cases.
// The rhs.Canceling flag may set as a side-effect of updating the request object.
func (rhs *RequestHandlerState) SetAndUpdateRequestMessageDistinct(ctx context.Context, format string, args ...interface{}) (string, error) {
	if msg, exists := rhs.msgExists(format, args...); exists {
		return msg, nil
	}
	return rhs.SetAndUpdateRequestMessage(ctx, format, args...)
}

// SetAndUpdateRequestMessageRepeated will update existing message with the number of repetitions and update the database with the request object (best effort, no guarantee)
// if the message does already exist. This is useful for cases where the VSR is known to retry on the same condition multiple times.
// The rhs.Canceling flag may set as a side-effect of updating the request object.
func (rhs *RequestHandlerState) SetAndUpdateRequestMessageRepeated(ctx context.Context, format string, args ...interface{}) (string, error) {
	msg := fmt.Sprintf(format, args...)
	rhs.Request.RequestMessages = util.NewMsgList(rhs.Request.RequestMessages).WithTimestamp(time.Now()).InsertWithRepeatCount(msg, 0).ToModel()
	items := &crud.Updates{Set: []string{"requestMessages"}}
	err := rhs.UpdateRequestWithItems(ctx, items)
	return msg, err
}

// SetRequestMessage appends a message in the VolumeSeriesRequest object and logs it at the Info level.
func (rhs *RequestHandlerState) SetRequestMessage(format string, args ...interface{}) string {
	msg := rhs._setRequestMessage(format, args...)
	rhs.A.Log.Infof("VolumeSeriesRequest %s: %s", rhs.Request.Meta.ID, msg)
	return msg
}

// SetRequestMessageDistinct appends a message in the VolumeSeriesRequest object if it does not already exist.
// It logs it at the Info level.
func (rhs *RequestHandlerState) SetRequestMessageDistinct(format string, args ...interface{}) string {
	msg, exists := rhs.msgExists(format, args...)
	if !exists {
		msg = rhs._setRequestMessage(format, args...)
	}
	rhs.A.Log.Infof("VolumeSeriesRequest %s: %s", rhs.Request.Meta.ID, msg)
	return msg
}

func (rhs *RequestHandlerState) msgExists(format string, args ...interface{}) (string, bool) {
	msg := fmt.Sprintf(format, args...)
	ml := rhs.Request.RequestMessages
	for i := len(ml) - 1; i >= 0; i-- {
		if ml[i].Message == msg {
			return msg, true
		}
	}
	return msg, false
}

// _setRequestMessage provides common message append support
func (rhs *RequestHandlerState) _setRequestMessage(format string, args ...interface{}) string {
	msg := fmt.Sprintf(format, args...)
	ts := &models.TimestampedString{
		Message: msg,
		Time:    strfmt.DateTime(time.Now()),
	}
	rhs.Request.RequestMessages = append(rhs.Request.RequestMessages, ts)
	return msg
}

// SetRequestState changes the state in the VolumeSeriesRequest object and returns a boolean indicating if there was a change
func (rhs *RequestHandlerState) SetRequestState(newState string) bool {
	vsr := rhs.Request
	curState := vsr.VolumeSeriesRequestState
	if newState != curState {
		vsr.VolumeSeriesRequestState = newState
		rhs.SetRequestMessage("State change: %s ⇒ %s", curState, newState)
		return true
	}
	return false
}

// SetAndUpdateRequestState sets the volume series request state and updates the database
// The rhs.Canceling flag may set as a side-effect of updating the request object.
func (rhs *RequestHandlerState) SetAndUpdateRequestState(ctx context.Context, newState string) error {
	var err error
	if rhs.SetRequestState(newState) {
		err = rhs.UpdateRequest(ctx)
	}
	return err
}

// UpdateRequest updates the registered items in a request
// Sets rhs.Canceling if it detects ErrorIDVerNotFound occurred due to cancelRequested flag being set
func (rhs *RequestHandlerState) UpdateRequest(ctx context.Context) error {
	items := *rhs.A.oItems
	if rhs.HasCreate && rhs.Request.VolumeSeriesID != rhs.originalVsID && rhs.Request.ConsistencyGroupID != rhs.originalCgID {
		items.Set = append(items.Set, "volumeSeriesId", "consistencyGroupId")
	}
	if rhs.HasAllocateCapacity && rhs.Request.ServicePlanAllocationID != rhs.originalSpaID {
		items.Set = append(items.Set, "servicePlanAllocationId")
	}
	if (rhs.HasUnbind || rhs.HasDelete) && rhs.Request.NodeID != rhs.originalNodeID {
		items.Set = append(items.Set, "nodeId")
	}
	return rhs.UpdateRequestWithItems(ctx, &items)
}

// UpdateRequestWithItems updates the specified items in a request
func (rhs *RequestHandlerState) UpdateRequestWithItems(ctx context.Context, items *crud.Updates) error {
	reqVer := rhs.Request.Meta.Version
	rhs.A.Log.Debugf("VolumeSeriesRequest %s v%d: UpdateRequest %v", rhs.Request.Meta.ID, reqVer, items.Set)
	modifyFn := func(o *models.VolumeSeriesRequest) (*models.VolumeSeriesRequest, error) {
		if o != nil {
			// We must let the updater give us the latest version or else we enter an infinite loop but we must then apply all the changed items.
			// Since nothing else should modify the fields being updated here we just pick up the new metadata, syncPeers and cancelRequested values.
			// Note: cancelRequested and syncPeers will never be updated via UpdateRequest.
			if !rhs.Request.CancelRequested && o.CancelRequested {
				rhs.A.Log.Warningf("VolumeSeriesRequest %s: cancel requested", rhs.Request.Meta.ID)
				rhs.Canceling = true
				rhs.Request.CancelRequested = true
			}
			rhs.Request.Meta = o.Meta
			rhs.Request.SyncPeers = o.SyncPeers
		}
		return rhs.Request, nil
	}
	obj, err := rhs.A.OCrud.VolumeSeriesRequestUpdater(ctx, string(rhs.Request.Meta.ID), modifyFn, items)
	if err == nil {
		rhs.Request = obj
		rhs.originalCgID = obj.ConsistencyGroupID
		rhs.originalNodeID = obj.NodeID
		rhs.originalSpaID = obj.ServicePlanAllocationID
		rhs.originalVsID = obj.VolumeSeriesID
		rhs.A.Log.Debugf("VolumeSeriesRequest %s Updated v%d ⇒ v%d", rhs.Request.Meta.ID, reqVer, rhs.Request.Meta.Version)
	} else {
		rhs.A.Log.Errorf("VolumeSeriesRequest %s v%d: UpdateRequest %s", rhs.Request.Meta.ID, reqVer, err.Error())
	}
	return err
}

// done is called when the animator completes processing (whether or not the request completes)
func (rhs *RequestHandlerState) done() {
	reqID := rhs.Request.Meta.ID
	reqVer := rhs.Request.Meta.Version
	rhs.A.Log.Debugf("VolumeSeriesRequest %s v%d Done", reqID, reqVer)
	rhs.A.mux.Lock()
	defer rhs.A.mux.Unlock()
	rhs.A.doneRequests[reqID] = reqVer
	delete(rhs.A.activeRequests, reqID)
}

// failOnPanic is called by the requestAnimator panic RecoverFunc to attempt to fail the VSR that caused the panic, adding its stack to the VSR messages
func (rhs *RequestHandlerState) failOnPanic(ctx context.Context, panicArg interface{}, stack []byte) {
	// don't need logging of the panic message, PanicLogger already logged the stack
	rhs.InError = true
	rhs._setRequestMessage("Panic occurred: %v\n\n%s", panicArg, bytes.TrimSpace(stack))
	rhs.SetAndUpdateRequestState(ctx, com.VolReqStateFailed) // ignore failure
	rhs.done()
}

// State predicate support

func (rhs *RequestHandlerState) setCurrentStateOrder() {
	rhs.currentStateOrder = stateData.Order(rhs.Request.VolumeSeriesRequestState)
}

func (rhs *RequestHandlerState) attempt(state string) bool {
	return rhs.currentStateOrder <= stateData.Order(state) // assume state present
}

func (rhs *RequestHandlerState) attemptChooseNode() bool {
	return rhs.attempt(com.VolReqStateChoosingNode) && rhs.VolumeSeries.RootStorageID != ""
}

func (rhs *RequestHandlerState) attemptPlacementReattach() bool {
	return rhs.attempt(com.VolReqStatePlacementReattach) && rhs.VolumeSeries.RootStorageID != ""
}

func (rhs *RequestHandlerState) attemptUndoVolumeConfig() bool {
	return rhs.attempt(com.VolReqStateUndoVolumeConfig) && ((!rhs.HasDelete && !rhs.HasUnbind) || rhs.VolumeSeries.RootStorageID != "")
}

func (rhs *RequestHandlerState) attemptUndoPlacement() bool {
	return rhs.attempt(com.VolReqStateUndoPlacement) && ((!rhs.HasDelete && !rhs.HasUnbind) || len(rhs.VolumeSeries.StorageParcels) > 0)
}

func (rhs *RequestHandlerState) attemptUndoSizing() bool {
	return rhs.attempt(com.VolReqStateUndoSizing) && ((!rhs.HasDelete && !rhs.HasUnbind) || len(rhs.VolumeSeries.StorageParcels) > 0)
}

func (rhs *RequestHandlerState) attemptUndoPublish() bool {
	return rhs.attempt(com.VolReqStateUndoPublishing) && ((!rhs.HasDelete && !rhs.HasUnbind) || len(rhs.VolumeSeries.ClusterDescriptor) > 0)
}

func (rhs *RequestHandlerState) undoOnly() bool {
	return !rhs.HasUnbind && !rhs.HasDelete && !rhs.HasDeleteSPA && !rhs.HasUnmount && (rhs.currentStateOrder >= soMinUndoStateOrder && rhs.currentStateOrder < soTerminationStatesOrder)
}

// requestAnimator processes volume series requests using a set of handlers.
// It is intended to be run in its own goroutine. It should not be holding a.mux when invoking rhs.Done.
// TBD: Create a derived context with the complete by deadline for use by the handlers (only).
//      It should not be used for the final update of state!
//      Issue with this is how to determine that the request failed because the deadline was reached?
func (rhs *RequestHandlerState) requestAnimator(ctx context.Context) {
	rhs.A.Log.Debugf("Processing VolumeSeriesRequest %s v%d", rhs.Request.Meta.ID, rhs.Request.Meta.Version)
	rhs.setCurrentStateOrder()
	if time.Now().After(time.Time(rhs.Request.CompleteByTime)) {
		rhs.TimedOut = true
	} else if rhs.undoOnly() { // RequestState is an UNDO state recovering from previous InError
		rhs.InError = true
	}
	// Assemble the ordered state transitions
	// Note: the handler has guaranteed correct operation sequences.
	states := []string{}
	undoStates := []string{}
	if rhs.HasCreate {
		if rhs.attempt(com.VolReqStateCreating) {
			states = append(states, com.VolReqStateCreating)
		} else if rhs.attempt(com.VolReqStateUndoCreating) {
			undoStates = append(undoStates, com.VolReqStateUndoCreating)
		}
	}
	if rhs.HasCreateFromSnapshot {
		if rhs.attempt(com.VolReqStateCreatingFromSnapshot) {
			states = append(states, com.VolReqStateCreatingFromSnapshot)
		} else if rhs.attempt(com.VolReqStateUndoCreatingFromSnapshot) {
			undoStates = append(undoStates, com.VolReqStateUndoCreatingFromSnapshot)
		}
		if rhs.attempt(com.VolReqStateSnapshotRestoreDone) {
			states = append(states, com.VolReqStateSnapshotRestoreDone)
		}
		if rhs.TimedOut {
			rhs.suppressTimedOutMessage = true
			// set this in the request so the logic doesn't actually write it to the object
			rhs.Request.VolumeSeriesRequestState = com.VolReqStateUndoCreatingFromSnapshot
		}
	}
	if rhs.HasBind {
		if rhs.attempt(com.VolReqStateBinding) {
			states = append(states, com.VolReqStateBinding)
			if !rhs.TimedOut && rhs.Request.VolumeSeriesRequestState == com.VolReqStateCapacityWait {
				// Change the in-memory state but don't update the db so that if
				// capacity is still not present no actual state change is recorded.
				rhs.SetRequestState(com.VolReqStateBinding)
			}
		} else if rhs.attempt(com.VolReqStateUndoBinding) {
			undoStates = append(undoStates, com.VolReqStateUndoBinding)
		}
	}
	if rhs.HasPublish {
		if rhs.attempt(com.VolReqStatePublishing) {
			states = append(states, com.VolReqStatePublishing)
		} else if rhs.attemptUndoPublish() {
			undoStates = append(undoStates, com.VolReqStateUndoPublishing)
		}
	}
	if rhs.HasMount || rhs.HasConfigure {
		if rhs.attempt(com.VolReqStateSizing) {
			states = append(states, com.VolReqStateSizing)
		} else if !rhs.HasConfigure && rhs.attemptUndoSizing() {
			undoStates = append(undoStates, com.VolReqStateUndoSizing)
		}
		if rhs.attempt(com.VolReqStatePlacement) {
			states = append(states, com.VolReqStatePlacement)
			if !rhs.TimedOut && rhs.Request.VolumeSeriesRequestState == com.VolReqStateStorageWait {
				// Change the in-memory state but don't update the db so that if
				// storage is still not present no actual state change is recorded.
				rhs.SetRequestState(com.VolReqStatePlacement)
			}
		} else if !rhs.HasConfigure && rhs.attemptUndoPlacement() {
			undoStates = append(undoStates, com.VolReqStateUndoPlacement)
		}
	}
	if rhs.HasMount {
		if rhs.attempt(com.VolReqStateVolumeConfig) {
			states = append(states, com.VolReqStateVolumeConfig)
		} else if rhs.attemptUndoVolumeConfig() {
			undoStates = append(undoStates, com.VolReqStateUndoVolumeConfig)
		}
		if rhs.attempt(com.VolReqStateVolumeExport) {
			states = append(states, com.VolReqStateVolumeExport)
		} else if rhs.attempt(com.VolReqStateUndoVolumeExport) {
			undoStates = append(undoStates, com.VolReqStateUndoVolumeExport)
		}
	}
	if rhs.HasVolSnapshotRestore {
		if rhs.attempt(com.VolReqStateSnapshotRestore) {
			states = append(states, com.VolReqStateSnapshotRestore)
		} else if rhs.attempt(com.VolReqStateUndoSnapshotRestore) {
			undoStates = append(undoStates, com.VolReqStateUndoSnapshotRestore)
		}
		if rhs.attempt(com.VolReqStateSnapshotRestoreFinalize) {
			states = append(states, com.VolReqStateSnapshotRestoreFinalize)
		}
	}
	if rhs.HasAttachFs {
		if rhs.attempt(com.VolReqStateAttachingFs) {
			states = append(states, com.VolReqStateAttachingFs)
		} else if rhs.attempt(com.VolReqStateUndoAttachingFs) {
			undoStates = append(undoStates, com.VolReqStateUndoAttachingFs)
		}
	}
	if rhs.HasDetachFs {
		// DETACH_FS has no undo path
		if rhs.attempt(com.VolReqStateUndoAttachingFs) {
			states = append(undoStates, com.VolReqStateUndoAttachingFs)
		}
	}
	if rhs.HasUnmount {
		// UNMOUNT has no undo path
		if rhs.attempt(com.VolReqStateUndoVolumeExport) {
			states = append(states, com.VolReqStateUndoVolumeExport)
		}
	}
	if rhs.HasDelete || rhs.HasUnbind {
		// DELETE and UNBIND have no undo path
		if rhs.attemptChooseNode() {
			states = append(states, com.VolReqStateChoosingNode)
		}
		if rhs.attemptPlacementReattach() {
			states = append(states, com.VolReqStatePlacementReattach)
		}
		if rhs.attemptUndoVolumeConfig() {
			states = append(states, com.VolReqStateUndoVolumeConfig)
		}
		if rhs.attemptUndoPlacement() {
			states = append(states, com.VolReqStateUndoPlacement)
		}
		if rhs.attemptUndoSizing() {
			states = append(states, com.VolReqStateUndoSizing)
		}
	}
	if rhs.HasUnpublish || rhs.HasDelete || rhs.HasUnbind {
		// UNPUBLISH and DELETE have no undo path
		if rhs.attemptUndoPublish() {
			states = append(states, com.VolReqStateUndoPublishing)
		}
	}
	if rhs.HasDelete || rhs.HasUnbind {
		if rhs.attempt(com.VolReqStateUndoBinding) {
			states = append(states, com.VolReqStateUndoBinding)
		}
	}
	if rhs.HasDelete {
		if rhs.attempt(com.VolReqStateUndoCreating) {
			states = append(states, com.VolReqStateUndoCreating)
		}
	}
	if rhs.HasRename {
		if rhs.attempt(com.VolReqStateRenaming) {
			states = append(states, com.VolReqStateRenaming)
		} else if rhs.attempt(com.VolReqStateUndoRenaming) {
			undoStates = append(undoStates, com.VolReqStateUndoRenaming)
		}
	}
	if rhs.HasAllocateCapacity {
		if rhs.attempt(com.VolReqStateAllocatingCapacity) {
			states = append(states, com.VolReqStateAllocatingCapacity)
		} else if rhs.attempt(com.VolReqStateUndoAllocatingCapacity) {
			undoStates = append(undoStates, com.VolReqStateUndoAllocatingCapacity)
		}
		if rhs.attempt(com.VolReqStatePublishingServicePlan) {
			states = append(states, com.VolReqStatePublishingServicePlan)
		}
	}
	if rhs.HasDeleteSPA {
		// DELETE_SPA is a pseudo-UNDO operation has no undo path
		if rhs.attempt(com.VolReqStateDeletingSPA) {
			states = append(states, com.VolReqStateDeletingSPA)
		}
	}
	if rhs.HasCGSnapshotCreate {
		if rhs.attempt(com.VolReqStateCGSnapshotVolumes) {
			states = append(states, com.VolReqStateCGSnapshotVolumes)
		}
		if rhs.attempt(com.VolReqStateCGSnapshotWait) {
			states = append(states, com.VolReqStateCGSnapshotWait)
		}
		if rhs.attempt(com.VolReqStateCGSnapshotFinalize) {
			states = append(states, com.VolReqStateCGSnapshotFinalize)
		}
		if rhs.attempt(com.VolReqStateUndoCGSnapshotVolumes) {
			undoStates = append(undoStates, com.VolReqStateUndoCGSnapshotVolumes)
		}
	}
	if rhs.HasConfigure {
		if rhs.attempt(com.VolReqStateVolumeConfig) {
			states = append(states, com.VolReqStateVolumeConfig)
		} else if rhs.attemptUndoVolumeConfig() {
			undoStates = append(undoStates, com.VolReqStateUndoVolumeConfig)
		}
	}
	if rhs.HasVolSnapshotCreate {
		if rhs.attempt(com.VolReqStatePausingIO) {
			states = append(states, com.VolReqStatePausingIO)
		} else if rhs.attempt(com.VolReqStateUndoPausingIO) {
			undoStates = append(undoStates, com.VolReqStateUndoPausingIO)
		}
		if rhs.attempt(com.VolReqStatePausedIO) {
			states = append(states, com.VolReqStatePausedIO)
		} else if rhs.attempt(com.VolReqStateUndoPausedIO) {
			undoStates = append(undoStates, com.VolReqStateUndoPausedIO)
		}
		if rhs.attempt(com.VolReqStateCreatingPiT) {
			states = append(states, com.VolReqStateCreatingPiT)
		} else if rhs.attempt(com.VolReqStateUndoCreatingPiT) {
			undoStates = append(undoStates, com.VolReqStateUndoCreatingPiT)
		}
		if rhs.attempt(com.VolReqStateCreatedPiT) {
			states = append(states, com.VolReqStateCreatedPiT)
		} else if rhs.attempt(com.VolReqStateUndoCreatedPiT) {
			undoStates = append(undoStates, com.VolReqStateUndoCreatedPiT)
		}
		if rhs.attempt(com.VolReqStateSnapshotUploading) {
			states = append(states, com.VolReqStateSnapshotUploading)
		} else if rhs.attempt(com.VolReqStateUndoSnapshotUploading) {
			undoStates = append(undoStates, com.VolReqStateUndoSnapshotUploading)
		}
		if rhs.attempt(com.VolReqStateSnapshotUploadDone) {
			states = append(states, com.VolReqStateSnapshotUploadDone)
		} else if rhs.attempt(com.VolReqStateUndoSnapshotUploadDone) {
			undoStates = append(undoStates, com.VolReqStateUndoSnapshotUploadDone)
		}
		if rhs.attempt(com.VolReqStateFinalizingSnapshot) {
			states = append(states, com.VolReqStateFinalizingSnapshot)
		} // no undo for VolReqStateFinalizingSnapshot
	}
	if rhs.HasChangeCapacity {
		if rhs.attempt(com.VolReqStateChangingCapacity) {
			states = append(states, com.VolReqStateChangingCapacity)
			if !rhs.TimedOut && rhs.Request.VolumeSeriesRequestState == com.VolReqStateCapacityWait {
				// Change the in-memory state but don't update the db so that if
				// capacity is still not present no actual state change is recorded.
				rhs.SetRequestState(com.VolReqStateChangingCapacity)
			}
		} else if rhs.attempt(com.VolReqStateUndoChangingCapacity) {
			undoStates = append(undoStates, com.VolReqStateUndoChangingCapacity)
		}
		if VolumeSeriesCacheIsRequested(rhs.VolumeSeries) {
			if rhs.attempt(com.VolReqStateResizingCache) {
				states = append(states, com.VolReqStateResizingCache)
			} else if rhs.attempt(com.VolReqStateUndoResizingCache) {
				undoStates = append(undoStates, com.VolReqStateUndoResizingCache)
			}
			if rhs.attempt(com.VolReqStateReallocatingCache) {
				states = append(states, com.VolReqStateReallocatingCache)
			} else if rhs.attempt(com.VolReqStateUndoReallocatingCache) {
				undoStates = append(undoStates, com.VolReqStateUndoReallocatingCache)
			}
		}
	}
	if rhs.HasNodeDelete {
		if rhs.attempt(com.VolReqStateDrainingRequests) {
			states = append(states, com.VolReqStateDrainingRequests)
		}
		if rhs.attempt(com.VolReqStateCancelingRequests) {
			states = append(states, com.VolReqStateCancelingRequests)
		}
		if rhs.attempt(com.VolReqStateDetachingVolumes) {
			states = append(states, com.VolReqStateDetachingVolumes)
		}
		if rhs.attempt(com.VolReqStateDetachingStorage) {
			states = append(states, com.VolReqStateDetachingStorage)
		}
		if rhs.attempt(com.VolReqStateVolumeDetachWait) {
			states = append(states, com.VolReqStateVolumeDetachWait)
		}
		if rhs.attempt(com.VolReqStateVolumeDetached) {
			states = append(states, com.VolReqStateVolumeDetached)
		}
		if rhs.attempt(com.VolReqStateDeletingNode) {
			states = append(states, com.VolReqStateDeletingNode)
		}
	}
	if rhs.HasVolDetach {
		if rhs.attempt(com.VolReqStateVolumeDetachWait) {
			states = append(states, com.VolReqStateVolumeDetachWait)
		}
		if rhs.attempt(com.VolReqStateVolumeDetaching) {
			states = append(states, com.VolReqStateVolumeDetaching)
		}
		if rhs.attempt(com.VolReqStateVolumeDetached) {
			states = append(states, com.VolReqStateVolumeDetached)
		}
	}
	if rhs.TimedOut && !rhs.suppressTimedOutMessage {
		rhs.SetRequestMessage("Timed out")
	}
	// Execute the state machine
	rhs.A.Log.Debugf("VolumeSeriesRequest %s v%d: TimedOut:%v InError:%v Canceling:%v states: %v", rhs.Request.Meta.ID, rhs.Request.Meta.Version, rhs.TimedOut, rhs.InError, rhs.Canceling, states)
	if !rhs.InError && !rhs.TimedOut && !rhs.Canceling {
		for _, state := range states {
			if !rhs.InError && !rhs.Canceling && !rhs.RetryLater {
				if rhs.SetAndUpdateRequestState(ctx, state) != nil {
					rhs.RetryLater = true
					break
				} else if rhs.Canceling {
					break
				}
				switch state {
				case com.VolReqStateCreating:
					rhs.A.createHandlers.Create(ctx, rhs)
				case com.VolReqStateBinding:
					rhs.A.bindHandlers.Bind(ctx, rhs)
				case com.VolReqStatePublishing:
					rhs.A.publishHandlers.Publish(ctx, rhs)
				case com.VolReqStateChoosingNode:
					rhs.A.allocationHandlers.ChooseNode(ctx, rhs)
				case com.VolReqStateSizing:
					rhs.A.allocationHandlers.Size(ctx, rhs)
				case com.VolReqStatePlacement:
					fallthrough
				case com.VolReqStatePlacementReattach:
					rhs.A.allocationHandlers.Place(ctx, rhs)
				case com.VolReqStateVolumeConfig:
					rhs.A.mountHandlers.Configure(ctx, rhs)
				case com.VolReqStateVolumeExport:
					rhs.A.mountHandlers.Export(ctx, rhs)
				case com.VolReqStateUndoVolumeExport:
					rhs.A.mountHandlers.UndoExport(ctx, rhs)
				case com.VolReqStateUndoVolumeConfig:
					rhs.A.mountHandlers.UndoConfigure(ctx, rhs)
				case com.VolReqStateUndoPlacement:
					rhs.A.allocationHandlers.UndoPlace(ctx, rhs)
				case com.VolReqStateUndoSizing:
					rhs.A.allocationHandlers.UndoSize(ctx, rhs)
				case com.VolReqStateUndoPublishing:
					rhs.A.publishHandlers.UndoPublish(ctx, rhs)
				case com.VolReqStateUndoBinding:
					rhs.A.bindHandlers.UndoBind(ctx, rhs)
				case com.VolReqStateUndoCreating:
					rhs.A.createHandlers.UndoCreate(ctx, rhs)
				case com.VolReqStateRenaming:
					rhs.A.renameHandlers.Rename(ctx, rhs)
				case com.VolReqStateAllocatingCapacity:
					rhs.A.allocateCapacityHandlers.AllocateCapacity(ctx, rhs)
				case com.VolReqStatePublishingServicePlan:
					rhs.A.publishServicePlanHandlers.PublishServicePlan(ctx, rhs)
				case com.VolReqStateDeletingSPA:
					rhs.A.allocateCapacityHandlers.UndoAllocateCapacity(ctx, rhs)
				case com.VolReqStateChangingCapacity:
					rhs.A.changeCapacityHandlers.ChangeCapacity(ctx, rhs)
				case com.VolReqStateResizingCache:
					rhs.A.allocationHandlers.ResizeCache(ctx, rhs)
				case com.VolReqStateReallocatingCache:
					rhs.A.mountHandlers.ReallocateCache(ctx, rhs)
				case com.VolReqStateCGSnapshotVolumes:
					fallthrough
				case com.VolReqStateCGSnapshotWait:
					fallthrough
				case com.VolReqStateCGSnapshotFinalize:
					rhs.A.cgSnapshotCreateHandlers.CGSnapshotCreate(ctx, rhs)
				case com.VolReqStatePausingIO:
					fallthrough
				case com.VolReqStatePausedIO:
					fallthrough
				case com.VolReqStateCreatingPiT:
					fallthrough
				case com.VolReqStateCreatedPiT:
					fallthrough
				case com.VolReqStateSnapshotUploading:
					fallthrough
				case com.VolReqStateSnapshotUploadDone:
					fallthrough
				case com.VolReqStateFinalizingSnapshot:
					rhs.A.volSnapshotCreateHandlers.VolSnapshotCreate(ctx, rhs)
				case com.VolReqStateSnapshotRestore:
					fallthrough
				case com.VolReqStateSnapshotRestoreFinalize:
					rhs.A.volSnapshotRestoreHandlers.VolSnapshotRestore(ctx, rhs)
				case com.VolReqStateCreatingFromSnapshot:
					fallthrough
				case com.VolReqStateSnapshotRestoreDone:
					rhs.A.createFromSnapshotHandlers.CreateFromSnapshot(ctx, rhs)
				case com.VolReqStateAttachingFs:
					rhs.A.attachFsHandlers.AttachFs(ctx, rhs)
				case com.VolReqStateUndoAttachingFs:
					rhs.A.attachFsHandlers.UndoAttachFs(ctx, rhs)
				case com.VolReqStateDrainingRequests:
					rhs.A.nodeDeleteHandlers.NodeDelete(ctx, rhs)
				case com.VolReqStateCancelingRequests:
					rhs.A.nodeDeleteHandlers.NodeDelete(ctx, rhs)
				case com.VolReqStateDetachingVolumes:
					rhs.A.nodeDeleteHandlers.NodeDelete(ctx, rhs)
				case com.VolReqStateDetachingStorage:
					rhs.A.nodeDeleteHandlers.NodeDelete(ctx, rhs)
				case com.VolReqStateVolumeDetachWait: // atypical: multiple interfaces
					if rhs.HasNodeDelete {
						rhs.A.nodeDeleteHandlers.NodeDelete(ctx, rhs)
					} else {
						rhs.A.volDetachHandlers.VolDetach(ctx, rhs)
					}
				case com.VolReqStateVolumeDetaching:
					rhs.A.volDetachHandlers.VolDetach(ctx, rhs)
				case com.VolReqStateVolumeDetached: // atypical: multiple interfaces
					if rhs.HasNodeDelete {
						rhs.A.nodeDeleteHandlers.NodeDelete(ctx, rhs)
					} else {
						rhs.A.volDetachHandlers.VolDetach(ctx, rhs)
					}
				case com.VolReqStateDeletingNode:
					rhs.A.nodeDeleteHandlers.NodeDelete(ctx, rhs)
				}
				// if there is an undo state for the current state, add it to the list
				if undoState := stateData.Undo(state); undoState != "" && !util.Contains(undoStates, undoState) {
					undoStates = append(undoStates, undoState)
				}
			}
		}
	} else if len(states) > 0 {
		// current or initial state may need to be undone
		if undoState := stateData.Undo(states[0]); undoState != "" {
			undoStates = append(undoStates, undoState)
		}
	}
	rhs.AbortUndo = false
	rhs.A.Log.Debugf("VolumeSeriesRequest %s v%d: TimedOut:%v InError:%v Canceling:%v undoStates:%v", rhs.Request.Meta.ID, rhs.Request.Meta.Version, rhs.TimedOut, rhs.InError, rhs.Canceling, undoStates)
	// cleanup on error, cancel or timed out
	if rhs.TimedOut || rhs.InError || rhs.Canceling {
		// reverse the undo states
		for left, right := 0, len(undoStates)-1; left < right; left, right = left+1, right-1 {
			undoStates[left], undoStates[right] = undoStates[right], undoStates[left]
		}
		rhs.A.Log.Debugf("VolumeSeriesRequest %s v%d: undoStates: %v", rhs.Request.Meta.ID, rhs.Request.Meta.Version, undoStates)
		// call the handlers to clean up
		for _, state := range undoStates {
			if !rhs.RetryLater && !rhs.AbortUndo {
				if rhs.SetAndUpdateRequestState(ctx, state) != nil {
					rhs.RetryLater = true
					break
				}
				switch state {
				case com.VolReqStateUndoVolumeExport:
					rhs.A.mountHandlers.UndoExport(ctx, rhs)
				case com.VolReqStateUndoVolumeConfig:
					rhs.A.mountHandlers.UndoConfigure(ctx, rhs)
				case com.VolReqStateUndoPlacement:
					rhs.A.allocationHandlers.UndoPlace(ctx, rhs)
				case com.VolReqStateUndoSizing:
					rhs.A.allocationHandlers.UndoSize(ctx, rhs)
				case com.VolReqStateUndoPublishing:
					rhs.A.publishHandlers.UndoPublish(ctx, rhs)
				case com.VolReqStateUndoBinding:
					rhs.A.bindHandlers.UndoBind(ctx, rhs)
				case com.VolReqStateUndoCreating:
					rhs.A.createHandlers.UndoCreate(ctx, rhs)
				case com.VolReqStateUndoCreatingFromSnapshot:
					rhs.A.createFromSnapshotHandlers.UndoCreateFromSnapshot(ctx, rhs)
				case com.VolReqStateUndoRenaming:
					rhs.A.renameHandlers.UndoRename(ctx, rhs)
				case com.VolReqStateUndoAllocatingCapacity:
					rhs.A.allocateCapacityHandlers.UndoAllocateCapacity(ctx, rhs)
				case com.VolReqStateUndoCGSnapshotVolumes:
					rhs.A.cgSnapshotCreateHandlers.UndoCGSnapshotCreate(ctx, rhs)
				case com.VolReqStateUndoReallocatingCache:
					rhs.A.mountHandlers.UndoReallocateCache(ctx, rhs)
				case com.VolReqStateUndoResizingCache:
					rhs.A.allocationHandlers.UndoResizeCache(ctx, rhs)
				case com.VolReqStateUndoChangingCapacity:
					rhs.A.changeCapacityHandlers.UndoChangeCapacity(ctx, rhs)
				case com.VolReqStateUndoSnapshotUploadDone:
					fallthrough
				case com.VolReqStateUndoSnapshotUploading:
					fallthrough
				case com.VolReqStateUndoCreatedPiT:
					fallthrough
				case com.VolReqStateUndoCreatingPiT:
					fallthrough
				case com.VolReqStateUndoPausedIO:
					fallthrough
				case com.VolReqStateUndoPausingIO:
					rhs.A.volSnapshotCreateHandlers.UndoVolSnapshotCreate(ctx, rhs)
				case com.VolReqStateUndoSnapshotRestore:
					rhs.A.volSnapshotRestoreHandlers.UndoVolSnapshotRestore(ctx, rhs)
				case com.VolReqStateUndoAttachingFs:
					rhs.A.attachFsHandlers.UndoAttachFs(ctx, rhs)
				}
				if rhs.AbortUndo {
					rhs.A.Log.Debugf("VolumeSeriesRequest %s v%d: AbortUndo state:%s", rhs.Request.Meta.ID, rhs.Request.Meta.Version, state)
				}
			}
		}
	}
	// enter terminal states
	if !rhs.RetryLater {
		finalState := com.VolReqStateSucceeded
		if rhs.InError || rhs.AbortUndo || rhs.TimedOut {
			finalState = com.VolReqStateFailed
		} else if rhs.Canceling {
			finalState = com.VolReqStateCanceled
		}
		rhs.SetAndUpdateRequestState(ctx, finalState) // ignore failure
	}
	rhs.done()
}

// DispatchRequests processes active volume series requests and returns a count of the new requests added
func (a *Animator) DispatchRequests(ctx context.Context, vrl []*models.VolumeSeriesRequest) int {
	var err error
	cnt := 0
	for _, vsr := range vrl {
		err = nil
		reqID := vsr.Meta.ID
		reqVer := vsr.Meta.Version
		if a.skipProcessing(reqID, reqVer) {
			continue // being processed
		}
		a.Log.Debugf("Loading %s v%d", reqID, reqVer)
		rhs := &RequestHandlerState{
			A:                     a,
			Request:               vsr,
			HasAllocateCapacity:   util.Contains(vsr.RequestedOperations, com.VolReqOpAllocateCapacity),
			HasAttachFs:           util.Contains(vsr.RequestedOperations, com.VolReqOpAttachFs),
			HasBind:               util.Contains(vsr.RequestedOperations, com.VolReqOpBind),
			HasCGSnapshotCreate:   util.Contains(vsr.RequestedOperations, com.VolReqOpCGCreateSnapshot),
			HasChangeCapacity:     util.Contains(vsr.RequestedOperations, com.VolReqOpChangeCapacity),
			HasConfigure:          util.Contains(vsr.RequestedOperations, com.VolReqOpConfigure),
			HasCreate:             util.Contains(vsr.RequestedOperations, com.VolReqOpCreate),
			HasCreateFromSnapshot: util.Contains(vsr.RequestedOperations, com.VolReqOpCreateFromSnapshot),
			HasDelete:             util.Contains(vsr.RequestedOperations, com.VolReqOpDelete),
			HasDeleteSPA:          util.Contains(vsr.RequestedOperations, com.VolReqOpDeleteSPA),
			HasDetachFs:           util.Contains(vsr.RequestedOperations, com.VolReqOpDetachFs),
			HasMount:              util.Contains(vsr.RequestedOperations, com.VolReqOpMount),
			HasNodeDelete:         util.Contains(vsr.RequestedOperations, com.VolReqOpNodeDelete),
			HasPublish:            util.Contains(vsr.RequestedOperations, com.VolReqOpPublish),
			HasRename:             util.Contains(vsr.RequestedOperations, com.VolReqOpRename),
			HasUnbind:             util.Contains(vsr.RequestedOperations, com.VolReqOpUnbind),
			HasUnmount:            util.Contains(vsr.RequestedOperations, com.VolReqOpUnmount),
			HasUnpublish:          util.Contains(vsr.RequestedOperations, com.VolReqOpUnpublish),
			HasVolDetach:          util.Contains(vsr.RequestedOperations, com.VolReqOpVolDetach),
			HasVolSnapshotCreate:  util.Contains(vsr.RequestedOperations, com.VolReqOpVolCreateSnapshot),
			HasVolSnapshotRestore: util.Contains(vsr.RequestedOperations, com.VolReqOpVolRestoreSnapshot),
			Canceling:             vsr.CancelRequested,
			originalCgID:          vsr.ConsistencyGroupID,
			originalNodeID:        vsr.NodeID,
			originalSpaID:         vsr.ServicePlanAllocationID,
			originalVsID:          vsr.VolumeSeriesID,
		}
		rhs.VSUpdater = rhs // self-reference
		if !a.ops.ShouldDispatch(ctx, rhs) {
			a.Log.Debugf("Skipping VolumeSeriesRequest %s v%d because ShouldDispatch returned false", reqID, reqVer)
			continue
		}
		if err == nil && vsr.VolumeSeriesID != "" { // the request references a VolumeSeries object
			rhs.VolumeSeries, err = a.OCrud.VolumeSeriesFetch(ctx, string(vsr.VolumeSeriesID))
		}
		if err != nil {
			if e, ok := err.(*crud.Error); ok && e.IsTransient() {
				a.Log.Warningf("Skipping VolumeSeriesRequest %s v%d because of a transient error", reqID, reqVer)
				continue
			}
			a.Log.Errorf("Failing VolumeSeriesRequest %s v%d: %s", reqID, reqVer, err.Error())
			rhs.SetRequestError("Error: %s", err.Error()) // dispatch with error
		}
		requestCtx := ctx
		if vsr.Creator != nil && vsr.Creator.AccountID != "" {
			requestCtx = context.WithValue(ctx, mgmtclient.AuthIdentityKey{}, vsr.Creator)
		}
		a.mux.Lock()
		// unlikely that there are overlapping dispatches but check once more...
		if _, ok := a.activeRequests[reqID]; !ok {
			a.Log.Debugf("Dispatching %s v%d %v", reqID, reqVer, rhs.Request.RequestedOperations)
			a.activeRequests[reqID] = rhs
			go util.PanicLogger(a.Log, func() { rhs.requestAnimator(requestCtx) }, func(panicArg interface{}, stack []byte) { rhs.failOnPanic(requestCtx, panicArg, stack) })
			cnt++
		}
		a.mux.Unlock()
	}
	return cnt
}
