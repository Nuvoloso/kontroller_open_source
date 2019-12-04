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
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/pstore"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	logging "github.com/op/go-logging"
)

var errVSUpdateRetryAborted = fmt.Errorf("VolumeSeries update retry aborted")

// ErrRequestWaiterCtxExpired error returned when RequestWaiter context expires
var ErrRequestWaiterCtxExpired = fmt.Errorf("RequestWaiter context expired")

// GetFirstErrorMessage returns the first error message in the message list, if any.
// The message must start with the word "Error"
func GetFirstErrorMessage(ml []*models.TimestampedString) string {
	for _, m := range ml {
		if strings.HasPrefix(m.Message, "Error") {
			return m.Message
		}
	}
	return ""
}

// SetVolumeState is can be used by handlers to change the state of a VolumeSeries, retrying if there is a version mismatch.
// It does nothing if PlanOnly is set.
func (rhs *RequestHandlerState) SetVolumeState(ctx context.Context, newState string) {
	if swag.BoolValue(rhs.Request.PlanOnly) {
		return
	}
	var lastVS *models.VolumeSeries
	getVS := func(vs *models.VolumeSeries) (*models.VolumeSeries, error) {
		if vs == nil {
			vs = rhs.VolumeSeries
		}
		lastVS = vs
		if vs.VolumeSeriesState == newState {
			return nil, errVSUpdateRetryAborted
		}
		msg := fmt.Sprintf("State change %s ⇒ %s", vs.VolumeSeriesState, newState)
		vs.Messages = append(vs.Messages, &models.TimestampedString{
			Message: msg,
			Time:    strfmt.DateTime(time.Now()),
		})
		vs.VolumeSeriesState = newState
		return vs, nil
	}
	items := &crud.Updates{Set: []string{"volumeSeriesState", "messages"}}
	vs, err := rhs.A.OCrud.VolumeSeriesUpdater(ctx, string(rhs.VolumeSeries.Meta.ID), getVS, items)
	if err == errVSUpdateRetryAborted {
		vs = lastVS
	} else if err != nil {
		rhs.SetRequestMessage("failed to update VolumeSeries object: %s", err.Error())
		rhs.RetryLater = true
		return
	}
	rhs.VolumeSeries = vs
}

// SetVolumeSystemTags can be used by handlers to set one or more SystemTags in a VolumeSeries, retrying if there is a version mismatch.
// It does nothing if PlanOnly is set.
// The function understands the "key:value" syntax of tags and will preserve other tags.
func (rhs *RequestHandlerState) SetVolumeSystemTags(ctx context.Context, systemTags ...string) {
	if swag.BoolValue(rhs.Request.PlanOnly) || len(systemTags) == 0 {
		return
	}
	inTags := util.NewTagList(systemTags)
	rhs.updateVolumeSystemTags(ctx, inTags, true)
}

// RemoveVolumeSystemTags can be used by handlers to remove one or more SystemTags of a VolumeSeries, retrying if there is a version mismatch.
// It does nothing if PlanOnly is set.
// The function understands the "key:value" syntax of tags (removes tag matching the key portion of "k:v") and will preserve other tags.
func (rhs *RequestHandlerState) RemoveVolumeSystemTags(ctx context.Context, systemTags ...string) {
	if swag.BoolValue(rhs.Request.PlanOnly) || len(systemTags) == 0 {
		return
	}
	inTags := util.NewTagList(systemTags)
	rhs.updateVolumeSystemTags(ctx, inTags, false)
}

func (rhs *RequestHandlerState) updateVolumeSystemTags(ctx context.Context, inTags *util.TagList, setTags bool) {
	items := &crud.Updates{}
	items.Set = []string{"systemTags"}
	lastSystemTags := rhs.VolumeSeries.SystemTags
	modifyFn := func(vs *models.VolumeSeries) (*models.VolumeSeries, error) {
		if vs == nil {
			vs = rhs.VolumeSeries
		}
		oTags := util.NewTagList(vs.SystemTags)
		for _, k := range inTags.Keys(nil) {
			if setTags {
				v, _ := inTags.Get(k)
				oTags.Set(k, v)
			} else {
				oTags.Delete(k)
			}
		}
		vs.SystemTags = oTags.List()
		return vs, nil
	}
	vs, err := rhs.A.OCrud.VolumeSeriesUpdater(ctx, string(rhs.VolumeSeries.Meta.ID), modifyFn, items)
	if err != nil {
		rhs.SetRequestMessage("failed to update VolumeSeries object: %s", err.Error())
		rhs.VolumeSeries.SystemTags = lastSystemTags
		rhs.RetryLater = true
		return
	}
	rhs.VolumeSeries = vs
}

// StashSet sets a value in the stash
func (rhs *RequestHandlerState) StashSet(key interface{}, value interface{}) {
	if rhs.Stash == nil {
		rhs.Stash = make(map[interface{}]interface{})
	}
	rhs.Stash[key] = value
}

// StashGet retrieves a value from the stash
func (rhs *RequestHandlerState) StashGet(key interface{}) interface{} {
	if rhs.Stash != nil {
		if value, exists := rhs.Stash[key]; exists {
			return value
		}
	}
	return nil
}

// CopyProgress satisfies the pstore.CopyProgressReporter interface.
// It updates the snapshotProgress property of the request and updates the database.
// It is not necessary to specify the snapshotProgress property when registering the animator handler.
func (rhs *RequestHandlerState) CopyProgress(ctx context.Context, cpr pstore.CopyProgressReport) {
	rhs.A.Log.Debugf("VolumeSeriesRequest %s: PSO CopyProgress %#v", rhs.Request.Meta.ID, cpr)
	// constrain percentage complete to 99%
	pc := int32(cpr.PercentComplete)
	if pc >= 100 {
		pc = 99
	} else if pc < 0 {
		pc = 0
	}
	rhs.Request.Progress = &models.Progress{
		TotalBytes:       cpr.TotalBytes,
		OffsetBytes:      cpr.OffsetBytes,
		TransferredBytes: cpr.TransferredBytes,
		PercentComplete:  swag.Int32(pc),
		Timestamp:        strfmt.DateTime(time.Now()),
	}
	items := &crud.Updates{Set: []string{"progress"}}
	rhs.UpdateRequestWithItems(ctx, items) // eat the error
}

// RequestWaiter provides an abstraction for RWaiter
type RequestWaiter interface {
	RW() *RWaiter
	WaitForVSR(ctx context.Context) (*models.VolumeSeriesRequest, error)
	WaitForSR(ctx context.Context) (*models.StorageRequest, error)
	CrudeNotify(cbt crude.WatcherCallbackType, ce *crude.CrudEvent) error
	HastenContextDeadline(ctx context.Context, t time.Duration) (newContext context.Context, cancel context.CancelFunc)
}

// RequestWaiterVSRInspector defines an inspector to determine if the wait should fail based on
// some property of the VSR request other than entering a terminal state.
type RequestWaiterVSRInspector interface {
	CanWaitOnVSR(*models.VolumeSeriesRequest) error
}

// RequestWaiterSRInspector defines an inspector to determine if the wait should fail based on
// some property of the SR request other than entering a terminal state.
type RequestWaiterSRInspector interface {
	CanWaitOnSR(*models.StorageRequest) error
}

// RWaiter implements a blocking call that waits for the completion of a request (VSR or SR)
type RWaiter struct {
	waitInterval  time.Duration
	vsr           *models.VolumeSeriesRequest
	sr            *models.StorageRequest
	crudeOps      crude.Ops
	clientOps     crud.Ops
	notifyCount   int
	notifyFlagged bool
	notifyChan    chan struct{}
	mux           sync.Mutex
	watcherID     string
	log           *logging.Logger
	name          string
	vsrInspector  RequestWaiterVSRInspector
	srInspector   RequestWaiterSRInspector
}

var _ = RequestWaiter(&RWaiter{})

// RequestWaiterArgs contains the properties needed to construct a RequestWaiter.
type RequestWaiterArgs struct {
	VSR          *models.VolumeSeriesRequest
	SR           *models.StorageRequest
	CrudeOps     crude.Ops
	ClientOps    crud.Ops
	Log          *logging.Logger
	VsrInspector RequestWaiterVSRInspector // optional
	SrInspector  RequestWaiterSRInspector  // optional
}

// Validate validate the input args for RequestWaiter
func (args *RequestWaiterArgs) Validate() error {
	if (args.VSR == nil && args.SR == nil) || (args.VSR != nil && args.SR != nil) || // precisely one of VSR or SR should be set
		args.CrudeOps == nil ||
		args.ClientOps == nil ||
		args.Log == nil {
		return fmt.Errorf("invalid arguments")
	}
	return nil
}

// NewRequestWaiter takes the request waiter arguments, validates and returns a new RWaiter
func NewRequestWaiter(args *RequestWaiterArgs) RequestWaiter {
	if err := args.Validate(); err != nil {
		panic(fmt.Sprintf("Attempt to create invalid request waiter: %v", args))
	}
	rW := &RWaiter{
		vsr:          args.VSR,
		sr:           args.SR,
		crudeOps:     args.CrudeOps,
		clientOps:    args.ClientOps,
		log:          args.Log,
		waitInterval: time.Duration(time.Second * 5), // low wait interval for first loop
		vsrInspector: args.VsrInspector,
		srInspector:  args.SrInspector,
	}
	return rW
}

// RW is an identity method
func (rw *RWaiter) RW() *RWaiter {
	return rw
}

// WaitForVSR waits for a pending vsr to finish and handles other cancel conditions appropriately
// It takes a ctx and a VSR object as input and returns the updated VSR or an error
// ctx will be used to timeout
func (rw *RWaiter) WaitForVSR(ctx context.Context) (*models.VolumeSeriesRequest, error) {
	if rw.vsr == nil {
		panic("WaitForVSR requires a VolumeSeriesRequest")
	}
	rw.name = fmt.Sprintf("RequestWaiter:%s", string(rw.vsr.Meta.ID))
	err := rw.waitForRequest(ctx)
	return rw.vsr, err
}

// WaitForSR waits for a pending storage request to finish and handles other cancel conditions appropriately
// It takes a ctx and a SR object as input and returns the updated SR or an error
// ctx will be used to timeout
func (rw *RWaiter) WaitForSR(ctx context.Context) (*models.StorageRequest, error) {
	if rw.sr == nil {
		panic("WaitForSR requires a StorageRequest")
	}
	rw.name = fmt.Sprintf("RequestWaiter:%s", string(rw.sr.Meta.ID))
	err := rw.waitForRequest(ctx)
	return rw.sr, err
}

// RequestStateIsTerminated verifies if a request (VSR or SR) state is a terminal state
func (rw *RWaiter) RequestStateIsTerminated() bool {
	if rw.vsr != nil {
		return VolumeSeriesRequestStateIsTerminated(rw.vsr.VolumeSeriesRequestState)
	}
	return StorageRequestStateIsTerminated(rw.sr.StorageRequestState)
}

func (rw *RWaiter) waitForRequest(ctx context.Context) error {
	var err error
	rw.notifyReset(true)
	wid, _ := rw.crudeOps.Watch(rw.getWatcherArgs(), rw)
	rw.watcherID = wid
	for !rw.RequestStateIsTerminated() {
		if rw.vsr != nil && rw.vsrInspector != nil {
			err = rw.vsrInspector.CanWaitOnVSR(rw.vsr)
		} else if rw.sr != nil && rw.srInspector != nil {
			err = rw.srInspector.CanWaitOnSR(rw.sr)
		}
		if err != nil {
			break
		}
		rw.notifyReset(false) // notifications while body is running will force another call
		select {
		case <-rw.notifyChan: // channel closed on
			rw.log.Debug("RequestWaiter notified")
		case <-ctx.Done():
			rw.log.Debugf("%s", ErrRequestWaiterCtxExpired.Error())
			err = ErrRequestWaiterCtxExpired
			break
		case <-time.After(rw.waitInterval):
			// wait
			rw.log.Debug("RequestWaiter new interval")
			rw.waitInterval = time.Duration(time.Second * 30)
		}
		if err != nil {
			break
		}
		if rw.vsr != nil {
			var vsr *models.VolumeSeriesRequest
			vsr, err = rw.clientOps.VolumeSeriesRequestFetch(ctx, string(rw.vsr.Meta.ID))
			if err != nil {
				err = fmt.Errorf("volume-series-request [%s] lookup failed: %s", rw.vsr.Meta.ID, err.Error())
				break
			}
			rw.vsr = vsr
		} else { // SR
			var sr *models.StorageRequest
			sr, err = rw.clientOps.StorageRequestFetch(ctx, string(rw.sr.Meta.ID))
			if err != nil {
				err = fmt.Errorf("storage-request [%s] lookup failed: %s", rw.sr.Meta.ID, err.Error())
				break
			}
			rw.sr = sr
		}
	}
	rw.crudeOps.TerminateWatcher(rw.watcherID)
	return err
}

func (rw *RWaiter) getWatcherArgs() *models.CrudWatcherCreateArgs {
	var uriPattern string // pattern to match a change of state
	if rw.vsr != nil {
		uriPattern = fmt.Sprintf("^/volume-series-requests/%s?.*=volumeSeriesRequestState", string(rw.vsr.Meta.ID))
	} else {
		uriPattern = fmt.Sprintf("^/storage-requests/%s?.*=storageRequestState", string(rw.sr.Meta.ID))
	}
	return &models.CrudWatcherCreateArgs{
		Name: rw.name,
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{
				MethodPattern: "PATCH",
				URIPattern:    uriPattern,
			},
		},
	}
}

// CrudeNotify handles CRUD events
func (rw *RWaiter) CrudeNotify(cbt crude.WatcherCallbackType, ce *crude.CrudEvent) error {
	if cbt == crude.WatcherEvent {
		rw.notify()
	}
	return nil
}

func (rw *RWaiter) notify() {
	rw.mux.Lock()
	defer rw.mux.Unlock()
	rw.notifyCount++
	if !rw.notifyFlagged {
		rw.notifyFlagged = true
		close(rw.notifyChan)
	}
}

func (rw *RWaiter) notifyReset(force bool) {
	rw.mux.Lock()
	defer rw.mux.Unlock()
	if rw.notifyFlagged || force {
		rw.notifyFlagged = false
		rw.notifyChan = make(chan struct{})
	}
}

// HastenContextDeadline takes the provided context and reduces the deadline by the requested duration
func (rw *RWaiter) HastenContextDeadline(ctx context.Context, t time.Duration) (newContext context.Context, cancel context.CancelFunc) {
	var newDeadline time.Time
	if deadline, ok := ctx.Deadline(); ok {
		newDeadline = deadline.Add(-t)
	}
	if newDeadline.After(time.Now()) {
		return context.WithDeadline(ctx, newDeadline)
	}
	return context.WithCancel(ctx)
}
