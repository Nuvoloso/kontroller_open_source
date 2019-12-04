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
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/strfmt"
)

// requestHandlerState is the in-memory state of a storage request.
// The animator will invoke the required handlers in order with this state structure.
// After the last handler returns the animator will set the request state to SUCCEEDED or FAILED depending
// on the value of InError.
// If the RetryLater flag is set then remaining handlers are not called and the request updated and discarded.
// On entry, if the completion time has expired the request will be failed.
type requestHandlerState struct {
	c *Component
	// Request is the current request object.
	// A handler is responsible for saving the state in the database and updating the in-memory contents.
	Request *models.StorageRequest
	// CSPDomain is the associated domain object.
	// The animator will load the CSPDomain object identified by CspDomainID (always present).
	CSPDomain *models.CSPDomain
	// Storage is the associated storage object.
	// The animator will load the Storage object if the StorageID is set.
	Storage *models.Storage
	// Pool is the associated pool of Storage.
	// The animator will load the Pool object if the StorageID is set.
	Pool *models.Pool
	// Cluster is the associated cluster object.
	// The animator will load the Cluster object if the ClusterID is set.
	Cluster *models.Cluster
	// If set the request has timed out.  This is set only on dispatch.
	// The concerned handlers will be called in reverse order to clean up.
	TimedOut bool
	// RetryLater should be set if a handler decides that further processing is not possible.
	// The request is not updated and will be re-loaded on the next iteration.
	RetryLater bool
	// InError should be set if the handler encounters an error. The animator will set the
	// state of the request to FAILED if not already set.
	// The concerned handlers will be called in reverse order to clean up, from the
	// failing state handler backward. They should update this structure as they undo,
	// as not all earlier operations can be undone as objects/states change. In particular,
	// Attach and Detach shouldn't try to undo/redo CSP operations.
	// For example:
	//  [PROV, ATT, DET,  REL (fails)] ⇒ cleanup [REL, DET, ATT, PROV]
	//  [PROV (fails), ATT] ⇒ cleanup [PROV]
	//  [PROV, ATT (fails)] ⇒ cleanup [ATT, PROV]
	InError bool
	// The types of operations requested. They will be processed in a fixed order.
	HasProvision, HasAttach, HasDetach, HasRelease, HasFormat, HasUse, HasClose, HasReattach bool
	// Done should be called when the animator completes processing (whether or not the request completes)
	// Handlers never call Done.
	Done func()
	// If true updates NodeID during state transition. Cleared automatically.
	SetNodeID bool
	// If true do not set StorageID during state transition.
	DoNotSetStorageID bool
}

// setRequestError appends a message in the StorageRequest object and logs it at Error level.
// It also sets the rhs.InError flag.
func (rhs *requestHandlerState) setRequestError(format string, args ...interface{}) string {
	msg := rhs._setRequestMessage(format, args...)
	rhs.c.Log.Errorf("StorageRequest %s: %s", rhs.Request.Meta.ID, msg)
	rhs.InError = true
	return msg
}

// setAndUpdateRequestMessage will set a message and update the database with the request object (best effort, no guarantee)
func (rhs *requestHandlerState) setAndUpdateRequestMessage(ctx context.Context, format string, args ...interface{}) (string, error) {
	msg := rhs.setRequestMessage(format, args...)
	items := &crud.Updates{}
	items.Set = []string{"storageRequestState", "requestMessages"}
	if !rhs.DoNotSetStorageID {
		items.Set = append(items.Set, "storageId")
	}
	modFn := func(o *models.StorageRequest) (*models.StorageRequest, error) {
		if o == nil {
			o = rhs.Request
		}
		sr := rhs.Request
		o.StorageRequestState = sr.StorageRequestState
		o.RequestMessages = sr.RequestMessages
		o.StorageID = sr.StorageID
		return o, nil
	}
	obj, err := rhs.c.oCrud.StorageRequestUpdater(ctx, string(rhs.Request.Meta.ID), modFn, items)
	if err == nil {
		rhs.Request = obj
	}
	return msg, err
}

// setAndUpdateRequestMessage will set a message and update the database with the request object (best effort, no guarantee)
// if the message does not already exist.  This is useful for cases where the VSR is known to retry on the same condition
// multiple times, such as in persistent REI injection cases.
func (rhs *requestHandlerState) setAndUpdateRequestMessageDistinct(ctx context.Context, format string, args ...interface{}) (string, error) {
	msg := fmt.Sprintf(format, args...)
	ml := rhs.Request.RequestMessages
	for i := len(ml) - 1; i >= 0; i-- {
		if ml[i].Message == msg {
			return msg, nil
		}
	}
	return rhs.setAndUpdateRequestMessage(ctx, format, args...)
}

// setRequestMessage appends a message in the StorageRequest object and logs it at the Info level.
func (rhs *requestHandlerState) setRequestMessage(format string, args ...interface{}) string {
	msg := rhs._setRequestMessage(format, args...)
	rhs.c.Log.Infof("StorageRequest %s: %s", rhs.Request.Meta.ID, msg)
	return msg
}

// _setRequestMessage provides common message append support
func (rhs *requestHandlerState) _setRequestMessage(format string, args ...interface{}) string {
	msg := fmt.Sprintf(format, args...)
	ts := &models.TimestampedString{
		Message: msg,
		Time:    strfmt.DateTime(time.Now()),
	}
	rhs.Request.RequestMessages = append(rhs.Request.RequestMessages, ts)
	return msg
}

// setRequestState changes the state in the StorageRequest object and returns a boolean indicating if there was a change
func (rhs *requestHandlerState) setRequestState(newState string) bool {
	sr := rhs.Request
	curState := sr.StorageRequestState
	if newState != curState {
		sr.StorageRequestState = newState
		rhs.setRequestMessage("State change: %s ⇒ %s", curState, newState)
		return true
	}
	return false
}

// setAndUpdateRequestState sets the storage request state and updates the database
func (rhs *requestHandlerState) setAndUpdateRequestState(ctx context.Context, newState string) error {
	if rhs.setRequestState(newState) {
		items := &crud.Updates{}
		items.Set = []string{"storageRequestState", "requestMessages"}
		if rhs.SetNodeID {
			items.Set = append(items.Set, "nodeId")
			rhs.SetNodeID = false
		}
		if !rhs.DoNotSetStorageID {
			items.Set = append(items.Set, "storageId")
		}
		modFn := func(o *models.StorageRequest) (*models.StorageRequest, error) {
			if o == nil {
				o = rhs.Request
			}
			sr := rhs.Request
			o.StorageRequestState = sr.StorageRequestState
			o.RequestMessages = sr.RequestMessages
			o.StorageID = sr.StorageID
			return o, nil
		}
		obj, err := rhs.c.oCrud.StorageRequestUpdater(ctx, string(rhs.Request.Meta.ID), modFn, items)
		if err != nil {
			return err
		}
		rhs.Request = obj
	}
	return nil
}

// The stateOrder and attempt subs ensure that we do not repeat a state on restart.
var stateOrder = map[string]int{
	com.StgReqStateNew:          0,
	com.StgReqStateCapacityWait: 1,
	com.StgReqStateProvisioning: 1,
	com.StgReqStateAttaching:    2,
	com.StgReqStateFormatting:   3,
	com.StgReqStateUsing:        4,
	com.StgReqStateClosing:      5,
	com.StgReqStateDetaching:    6,
	com.StgReqStateReleasing:    7,
	com.StgReqStateRemovingTag:  7, // release and tag removal are mutually exclusive

	com.StgReqStateUndoDetaching:    8,
	com.StgReqStateUndoAttaching:    9,
	com.StgReqStateUndoProvisioning: 10,

	com.StgReqStateSucceeded: 11,
	com.StgReqStateFailed:    11,
}

// undoStateMap provides a mapping from individual normal states to their corresponding undo state, if any
var undoStateMap = map[string]string{
	com.StgReqStateDetaching:    com.StgReqStateUndoDetaching,
	com.StgReqStateAttaching:    com.StgReqStateUndoAttaching,
	com.StgReqStateProvisioning: com.StgReqStateUndoProvisioning,
}

func (rhs *requestHandlerState) attemptProvision() bool {
	ordinal := stateOrder[rhs.Request.StorageRequestState] // assume state present
	return ordinal <= 1
}

func (rhs *requestHandlerState) attemptAttach() bool {
	ordinal := stateOrder[rhs.Request.StorageRequestState] // assume state present
	return ordinal <= 2
}

func (rhs *requestHandlerState) attemptDetach() bool {
	ordinal := stateOrder[rhs.Request.StorageRequestState] // assume state present
	return ordinal <= 6
}

func (rhs *requestHandlerState) attemptRelease() bool {
	ordinal := stateOrder[rhs.Request.StorageRequestState] // assume state present
	return ordinal <= 7
}

func (rhs *requestHandlerState) attemptRemoveTag() bool {
	ordinal := stateOrder[rhs.Request.StorageRequestState] // assume state present
	return ordinal <= 7
}

// agentd operations are seen here only if preceded by attach, and their processing only involves a state change
func (rhs *requestHandlerState) attemptFormat() bool {
	ordinal := stateOrder[rhs.Request.StorageRequestState] // assume state present
	return ordinal <= 3
}

func (rhs *requestHandlerState) attemptUse() bool {
	if rhs.HasFormat {
		return false
	}
	ordinal := stateOrder[rhs.Request.StorageRequestState] // assume state present
	return ordinal <= 4
}

func (rhs *requestHandlerState) attemptClose() bool {
	ordinal := stateOrder[rhs.Request.StorageRequestState] // assume state present
	return ordinal <= 5
}

func (rhs *requestHandlerState) attemptUndoDetach() bool {
	ordinal := stateOrder[rhs.Request.StorageRequestState] // assume state present
	return ordinal <= 8
}

func (rhs *requestHandlerState) attemptUndoAttach() bool {
	ordinal := stateOrder[rhs.Request.StorageRequestState] // assume state present
	return ordinal <= 9
}

func (rhs *requestHandlerState) attemptUndoProvision() bool {
	ordinal := stateOrder[rhs.Request.StorageRequestState] // assume state present
	return ordinal <= 10
}

func (rhs *requestHandlerState) undoOnly() bool {
	ordinal := stateOrder[rhs.Request.StorageRequestState] // assume state present
	return ordinal >= 8 && ordinal <= 10
}

// requestHandlers is an interface for processing individual storage request operations.
type requestHandlers interface {
	ProvisionStorage(context.Context, *requestHandlerState)
	AttachStorage(context.Context, *requestHandlerState)
	DetachStorage(context.Context, *requestHandlerState)
	UndoDetachStorage(context.Context, *requestHandlerState)
	ReleaseStorage(context.Context, *requestHandlerState)
	ReattachSwitchNodes(context.Context, *requestHandlerState)
	RemoveTag(context.Context, *requestHandlerState)
}

// requestAnimator processes storage requests using a set of handlers.
// It is intended to be run in its own goroutine. It should not be holding c.mux when invoking rhs.Done.
// TBD: Create a derived context with the complete by deadline for use by the handlers (only).
//      It should not be used for the final update of state!
//      Issue with this is how to determine that the request failed because the deadline was reached?
func (c *Component) requestAnimator(ctx context.Context, rhs *requestHandlerState) {
	c.Log.Debugf("Processing StorageRequest %s", rhs.Request.Meta.ID)
	if time.Now().After(time.Time(rhs.Request.CompleteByTime)) {
		rhs.setRequestMessage("Timed out")
		rhs.TimedOut = true
	} else if rhs.undoOnly() { // RequestState is an UNDO state recovering from previous InError possibly in agentd
		rhs.InError = true
	}
	// Assemble the ordered state transitions
	// Note: the handler has guaranteed correct operation sequences.
	var states, undoStates []string
	if rhs.HasReattach { // standalone special operation, no undo
		states = c.reattachGetStates(rhs)
	}
	if rhs.HasProvision {
		if rhs.attemptProvision() {
			states = append(states, com.StgReqStateProvisioning)
			if !rhs.TimedOut && rhs.Request.StorageRequestState == com.StgReqStateCapacityWait {
				// Change the in-memory state but don't update the db so that if
				// capacity is still not present no actual state change is recorded.
				rhs.setRequestState(com.StgReqStateProvisioning)
			}
		} else if rhs.attemptUndoProvision() {
			undoStates = append(undoStates, com.StgReqStateUndoProvisioning)
		}
	}
	if rhs.HasAttach {
		if rhs.attemptAttach() {
			states = append(states, com.StgReqStateAttaching)
		} else if rhs.attemptUndoAttach() { // used when agentd transitions state to ATTACHING_UNDO
			undoStates = append(undoStates, com.StgReqStateUndoAttaching)
		}
	}
	// no undoStates for format or use: dispatchRequests ensures requestAnimator is not called during undo until back in a centrald state
	if rhs.HasFormat && rhs.attemptFormat() {
		states = append(states, com.StgReqStateFormatting)
	}
	if rhs.HasUse && rhs.attemptUse() {
		states = append(states, com.StgReqStateUsing)
	}
	if rhs.HasClose && rhs.attemptClose() {
		states = append(states, com.StgReqStateClosing)
	}
	if rhs.HasDetach {
		if rhs.attemptDetach() {
			states = append(states, com.StgReqStateDetaching)
		} else if rhs.attemptUndoDetach() {
			undoStates = append(undoStates, com.StgReqStateUndoDetaching)
		}
	}
	// RELEASE cannot be undone
	if rhs.HasRelease && rhs.attemptRelease() {
		states = append(states, com.StgReqStateReleasing)
	} else if rhs.HasProvision && rhs.attemptRemoveTag() {
		states = append(states, com.StgReqStateRemovingTag)
	}
	// Execute the state machine
	c.Log.Debugf("StorageRequest %s: TimedOut:%v states: %v", rhs.Request.Meta.ID, rhs.TimedOut, states)
	if !rhs.InError && !rhs.TimedOut {
		for _, state := range states {
			if !rhs.InError && !rhs.RetryLater {
				if err := rhs.setAndUpdateRequestState(ctx, state); err != nil {
					if e, ok := err.(*crud.Error); ok && !e.IsTransient() {
						rhs.setRequestError("Error: %s", err.Error())
					} else {
						rhs.RetryLater = true
					}
					break
				}
				switch state {
				case com.StgReqStateProvisioning:
					c.requestHandlers.ProvisionStorage(ctx, rhs)
				case com.StgReqStateAttaching:
					c.requestHandlers.AttachStorage(ctx, rhs)
				case com.StgReqStateDetaching:
					c.requestHandlers.DetachStorage(ctx, rhs)
				case com.StgReqStateReleasing:
					c.requestHandlers.ReleaseStorage(ctx, rhs)
				case com.StgReqStateReattaching:
					c.requestHandlers.ReattachSwitchNodes(ctx, rhs)
				case com.StgReqStateRemovingTag:
					c.requestHandlers.RemoveTag(ctx, rhs)
				// agentd operations
				case com.StgReqStateClosing:
					rhs.RetryLater = true
				case com.StgReqStateFormatting:
					rhs.RetryLater = true
				case com.StgReqStateUsing:
					rhs.RetryLater = true
				}
				if !rhs.HasReattach {
					// if there is an undo state for the current state, add it to the list
					if undoState, ok := undoStateMap[state]; ok {
						undoStates = append(undoStates, undoState)
					}
				}
			}
		}
	} else if len(states) > 0 && !rhs.HasReattach {
		// current or initial state may need to be undone
		if undoState, ok := undoStateMap[states[0]]; ok {
			undoStates = append(undoStates, undoState)
		}
	}
	// cleanup on error or timedout
	if rhs.TimedOut || rhs.InError {
		// reverse the undo states
		for left, right := 0, len(undoStates)-1; left < right; left, right = left+1, right-1 {
			undoStates[left], undoStates[right] = undoStates[right], undoStates[left]
		}
		c.Log.Debugf("StorageRequest %s: TimedOut:%v undostates:%v", rhs.Request.Meta.ID, rhs.TimedOut, undoStates)
		// call the handlers to clean up
		for _, state := range undoStates {
			if !rhs.RetryLater {
				if err := rhs.setAndUpdateRequestState(ctx, state); err != nil {
					if e, ok := err.(*crud.Error); ok && !e.IsTransient() {
						rhs.setRequestError("Error: %s", err.Error())
					} else {
						rhs.RetryLater = true
					}
					break
				}
				switch state {
				case com.StgReqStateUndoProvisioning:
					// ReleaseStorage handles the Undo of ProvisionStorage
					c.requestHandlers.ReleaseStorage(ctx, rhs)
				case com.StgReqStateUndoAttaching:
					// DetachStorage handles the Undo of AttachStorage
					c.requestHandlers.DetachStorage(ctx, rhs)
				case com.StgReqStateUndoDetaching:
					c.requestHandlers.UndoDetachStorage(ctx, rhs)
				}
			}
		}
	}
	// enter terminal states
	if !rhs.RetryLater {
		finalState := com.StgReqStateSucceeded
		if rhs.InError || rhs.TimedOut {
			finalState = com.StgReqStateFailed
		}
		rhs.setAndUpdateRequestState(ctx, finalState) // ignore failure
	}
	rhs.Done()
}

// dispatchRequests processes active storage requests and returns a count of the new requests added
func (c *Component) dispatchRequests(ctx context.Context, srs []*models.StorageRequest) int {
	dMap := map[models.ObjID]*models.CSPDomain{}
	spMap := map[models.ObjIDMutable]*models.Pool{}
	clMap := map[models.ObjID]*models.Cluster{}
	var err error
	cnt := 0
	for _, sr := range srs {
		err = nil // reset each pass!
		c.mux.Lock()
		reqID := sr.Meta.ID
		if _, ok := c.activeRequests[reqID]; ok {
			c.Log.Debugf("StorageRequest %s already being processed", reqID)
			c.mux.Unlock()
			continue // being processed
		}
		if doneVersion, ok := c.doneRequests[reqID]; ok {
			// An active request may become Done in another thread while dispatch progresses, such outdated requests should be ignored
			if doneVersion > sr.Meta.Version {
				c.Log.Debugf("StorageRequest %s processing is already done", reqID)
				c.mux.Unlock()
				continue // being processed
			}
		}
		c.mux.Unlock()
		c.Log.Debugf("Loading %s", reqID)
		rhs := &requestHandlerState{
			c:            c,
			Request:      sr,
			HasProvision: util.Contains(sr.RequestedOperations, com.StgReqOpProvision),
			HasAttach:    util.Contains(sr.RequestedOperations, com.StgReqOpAttach),
			HasDetach:    util.Contains(sr.RequestedOperations, com.StgReqOpDetach),
			HasRelease:   util.Contains(sr.RequestedOperations, com.StgReqOpRelease),
			HasFormat:    util.Contains(sr.RequestedOperations, com.StgReqOpFormat),   // agentd
			HasUse:       util.Contains(sr.RequestedOperations, com.StgReqOpUse),      // agentd
			HasClose:     util.Contains(sr.RequestedOperations, com.StgReqOpClose),    // agentd
			HasReattach:  util.Contains(sr.RequestedOperations, com.StgReqOpReattach), // centrald/agentd based on Storage state
			Done: func() {
				c.Log.Debugf("StorageRequest %s Done", reqID)
				c.mux.Lock()
				defer c.mux.Unlock()
				c.doneRequests[reqID] = c.activeRequests[reqID].Request.Meta.Version
				delete(c.activeRequests, reqID)
			},
		}
		// the following check assumes the operation sequence was validated
		if ((rhs.HasFormat || rhs.HasUse || rhs.HasClose) &&
			((sr.StorageRequestState == com.StgReqStateNew && !(rhs.HasProvision || rhs.HasAttach || rhs.HasReattach)) ||
				sr.StorageRequestState == com.StgReqStateFormatting || sr.StorageRequestState == com.StgReqStateUsing || sr.StorageRequestState == com.StgReqStateClosing)) ||
			(rhs.HasReattach && !util.Contains([]string{com.StgReqStateDetaching, com.StgReqStateReattaching, com.StgReqStateAttaching, com.StgReqStateNew}, sr.StorageRequestState)) {
			c.Log.Debugf("Skipping StorageRequest %s processed by agentd", reqID)
			continue
		}
		// load the CSPDomain object via a cache
		if dObj, ok := dMap[sr.CspDomainID]; ok {
			rhs.CSPDomain = dObj
		} else {
			rhs.CSPDomain, err = c.oCrud.CSPDomainFetch(ctx, string(sr.CspDomainID))
			if err == nil {
				dMap[sr.CspDomainID] = rhs.CSPDomain
			}
		}
		if err == nil && sr.StorageID != "" { // the request references a Storage object and indirectly a Pool
			rhs.Storage, err = c.oCrud.StorageFetch(ctx, string(sr.StorageID))
			if err == nil {
				// load the Pool indirectly via a cache
				spID := rhs.Storage.PoolID
				if spObj, ok := spMap[spID]; ok {
					rhs.Pool = spObj
				} else {
					rhs.Pool, err = c.oCrud.PoolFetch(ctx, string(spID))
					if err == nil {
						spMap[spID] = rhs.Pool
					}
				}
			}
			if err != nil && sr.StorageRequestState == com.StgReqStateReleasing {
				err = nil
			}
		}
		if err == nil && sr.ClusterID != "" { // load the Cluster indirectly via a cache
			if clObj, ok := clMap[sr.ClusterID]; ok {
				rhs.Cluster = clObj
			} else {
				rhs.Cluster, err = c.oCrud.ClusterFetch(ctx, string(sr.ClusterID))
				if err == nil {
					clMap[sr.ClusterID] = rhs.Cluster
				}
			}
		}
		if err != nil {
			if e, ok := err.(*crud.Error); ok && e.IsTransient() {
				c.Log.Warningf("Skipping StorageRequest %s because of a transient error", reqID)
				continue
			}
			c.Log.Errorf("Failing StorageRequest %s: %s", reqID, err.Error())
			rhs.setRequestError("Error: %s", err.Error())
		}
		// evaluate special case of a NEW re-attach with partial progress as determined by the storage state
		if rhs.HasReattach && sr.StorageRequestState == com.StgReqStateNew && !c.dispatchNewReattachSR(rhs) {
			c.Log.Debugf("Skipping StorageRequest %s processed by agentd", reqID)
			continue
		}
		c.mux.Lock()
		// unlikely that there are overlapping dispatches but check once more...
		if _, ok := c.activeRequests[reqID]; !ok {
			c.Log.Debugf("Dispatching %s %v", reqID, rhs.Request.RequestedOperations)
			c.activeRequests[reqID] = rhs
			go util.PanicLogger(c.Log, func() { c.requestAnimator(ctx, rhs) })
			cnt++
		}
		c.mux.Unlock()
	}
	return cnt
}
