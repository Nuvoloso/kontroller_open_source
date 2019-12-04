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
	c *SRComp
	// Request is the current request object.
	// A handler is responsible for saving the state in the database and updating the in-memory contents.
	Request *models.StorageRequest
	// Storage is the associated storage object.
	// The animator will load the Storage object if the StorageID is set.
	Storage *models.Storage
	// If set the request has timed out.  This is set only on entry into the animator.
	TimedOut bool
	// RetryLater should be set if a handler decides that further processing is not possible.
	// The request is not updated and will be re-loaded on the next iteration.
	RetryLater bool
	// InError should be set if the handler encounters an error. The animator will set the
	// state of the request to FAILED if not already set.
	InError bool
	// The types of operations requested. They will be processed in a fixed order.
	HasProvision, HasAttach, HasFormat, HasUse, HasClose, HasDetach, HasRelease, HasReattach bool
	// Done should be called when the animator completes processing (whether or not the request completes)
	// Handlers never call Done.
	Done func()
}

// requestHandlers is an interface for processing individual storage request operations.
type requestHandlers interface {
	FormatStorage(context.Context, *requestHandlerState)
	UseStorage(context.Context, *requestHandlerState)
	CloseStorage(context.Context, *requestHandlerState)
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
	modFn := func(o *models.StorageRequest) (*models.StorageRequest, error) {
		if o == nil {
			o = rhs.Request
		}
		sr := rhs.Request
		o.StorageRequestState = sr.StorageRequestState
		o.RequestMessages = sr.RequestMessages
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
		modFn := func(o *models.StorageRequest) (*models.StorageRequest, error) {
			if o == nil {
				o = rhs.Request
			}
			sr := rhs.Request
			o.StorageRequestState = sr.StorageRequestState
			o.RequestMessages = sr.RequestMessages
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
	com.StgReqStateNew:         0,
	com.StgReqStateFormatting:  1,
	com.StgReqStateUsing:       2,
	com.StgReqStateClosing:     3,
	com.StgReqStateRemovingTag: 4,
	com.StgReqStateSucceeded:   5,
	com.StgReqStateFailed:      5,
}

func (rhs *requestHandlerState) attemptFormat() bool {
	ordinal := stateOrder[rhs.Request.StorageRequestState] // assume state present
	return ordinal <= 1
}

func (rhs *requestHandlerState) attemptUse() bool {
	ordinal := stateOrder[rhs.Request.StorageRequestState] // assume state present
	return ordinal <= 2
}

func (rhs *requestHandlerState) attemptClose() bool {
	ordinal := stateOrder[rhs.Request.StorageRequestState] // assume state present
	return ordinal <= 3
}

func (rhs *requestHandlerState) attemptRemoveTag() bool {
	ordinal := stateOrder[rhs.Request.StorageRequestState] // assume state present
	return ordinal <= 4
}

// requestAnimator processes storage requests using a set of handlers.
// It is intended to be run in its own goroutine. It should not be holding c.mux when invoking rhs.Done.
// TBD: Create a derived context with the complete by deadline for use by the handlers (only).
//      It should not be used for the final update of state!
//      Issue with this is how to determine that the request failed because the deadline was reached?
func (c *SRComp) requestAnimator(ctx context.Context, rhs *requestHandlerState) {
	c.Log.Debugf("Processing StorageRequest %s", rhs.Request.Meta.ID)
	if time.Now().After(time.Time(rhs.Request.CompleteByTime)) {
		rhs.setRequestMessage("Timed out")
		rhs.TimedOut = true
	}
	var states []string
	if rhs.HasFormat && rhs.attemptFormat() {
		states = append(states, com.StgReqStateFormatting)
	}
	if rhs.HasUse && rhs.attemptUse() {
		states = append(states, com.StgReqStateUsing)
	}
	if rhs.HasClose && rhs.attemptClose() {
		states = append(states, com.StgReqStateClosing)
		if rhs.HasDetach {
			states = append(states, com.StgReqStateDetaching)
		}
	}
	if rhs.HasReattach {
		states = c.reattachGetStates(rhs) // may set RetryLater or InError
	}
	if rhs.HasProvision && rhs.attemptRemoveTag() {
		states = append(states, com.StgReqStateRemovingTag)
	}
	// Execute the state machine
	c.Log.Debugf("StorageRequest %s: TimedOut:%v RetryLater:%v InError:%v states: %v", rhs.Request.Meta.ID, rhs.TimedOut, rhs.RetryLater, rhs.InError, states)
	if !rhs.TimedOut {
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
				case com.StgReqStateFormatting:
					c.requestHandlers.FormatStorage(ctx, rhs)
				case com.StgReqStateUsing:
					c.requestHandlers.UseStorage(ctx, rhs)
				case com.StgReqStateClosing:
					c.requestHandlers.CloseStorage(ctx, rhs)
				case com.StgReqStateDetaching:
					rhs.RetryLater = true
				case com.StgReqStateRemovingTag:
					rhs.RetryLater = true
				}
			}
		}
	}
	// cleanup on error or timedout
	if rhs.TimedOut || rhs.InError {
		// no cleanup in agentd, however if HasAttach is true must transition to UNDO_ATTACHING for centrald cleanup
		if rhs.HasAttach {
			// setting RetryLater keeps Request alive for centrald
			rhs.RetryLater = true
			if err := rhs.setAndUpdateRequestState(ctx, com.StgReqStateUndoAttaching); err != nil {
				if e, ok := err.(*crud.Error); ok && !e.IsTransient() {
					rhs.setRequestError("Error: %s", err.Error())
					rhs.RetryLater = false
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
		// If not reattach mark the Storage object as UNUSED/OPEN (depending on the requested operation) if the request failed (best effort, as needed)
		if finalState != com.StgReqStateSucceeded && rhs.Storage != nil && !rhs.HasReattach {
			ss := rhs.Storage.StorageState
			finalDeviceState := com.StgDeviceStateUnused
			if rhs.HasClose {
				finalDeviceState = com.StgDeviceStateOpen
			}
			if ss != nil && ss.DeviceState != finalDeviceState {
				ss.Messages = append(ss.Messages, &models.TimestampedString{
					Message: fmt.Sprintf("State change: deviceState %s ⇒ %s", ss.DeviceState, finalDeviceState),
					Time:    strfmt.DateTime(time.Now()),
				})
				ss.DeviceState = finalDeviceState
				items := &crud.Updates{Set: []string{"storageState"}}
				rhs.c.oCrud.StorageUpdate(ctx, rhs.Storage, items) // ignore failure
			}
		}
		rhs.setAndUpdateRequestState(ctx, finalState) // ignore failure
	}
	rhs.Done()
}

// dispatchRequests processes active agentd storage requests and returns a count of the new requests added
func (c *SRComp) dispatchRequests(ctx context.Context, srs []*models.StorageRequest) int {
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
			HasProvision: util.Contains(sr.RequestedOperations, com.StgReqOpProvision), // centrald
			HasAttach:    util.Contains(sr.RequestedOperations, com.StgReqOpAttach),    // centrald
			HasFormat:    util.Contains(sr.RequestedOperations, com.StgReqOpFormat),
			HasUse:       util.Contains(sr.RequestedOperations, com.StgReqOpUse),
			HasClose:     util.Contains(sr.RequestedOperations, com.StgReqOpClose),
			HasDetach:    util.Contains(sr.RequestedOperations, com.StgReqOpDetach),   // centrald
			HasRelease:   util.Contains(sr.RequestedOperations, com.StgReqOpRelease),  // centrald
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
		if !(rhs.HasFormat || rhs.HasUse || rhs.HasClose || rhs.HasReattach) ||
			((rhs.HasProvision || rhs.HasAttach) &&
				!(sr.StorageRequestState == com.StgReqStateFormatting || sr.StorageRequestState == com.StgReqStateUsing)) ||
			(rhs.HasClose && !util.Contains([]string{com.StgReqStateNew, com.StgReqStateClosing}, sr.StorageRequestState)) ||
			(rhs.HasReattach && !util.Contains([]string{com.StgReqStateNew, com.StgReqStateClosing, com.StgReqStateUsing}, sr.StorageRequestState)) {
			c.Log.Debugf("Skipping StorageRequest %s processed by centrald", reqID)
			continue
		}
		if sr.StorageID != "" {
			if rhs.Storage, err = c.oCrud.StorageFetch(ctx, string(sr.StorageID)); err == nil {
				ss := rhs.Storage.StorageState
				if ss == nil || ss.AttachmentState != com.StgAttachmentStateAttached || ss.AttachedNodeID != c.thisNodeID {
					err = fmt.Errorf("storage %s storageState invalid", sr.StorageID)
				}
			}
		} else {
			err = fmt.Errorf("storageId not set in StorageRequest %s", reqID)
		}
		if err != nil {
			if e, ok := err.(*crud.Error); ok && e.IsTransient() {
				c.Log.Warningf("Skipping StorageRequest %s because of a transient error", reqID)
				continue
			}
			c.Log.Errorf("Failing StorageRequest %s: %s", reqID, err.Error())
			rhs.setRequestError("Error: %s", err.Error())
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
