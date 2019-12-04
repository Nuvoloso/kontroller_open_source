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
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/op/go-logging"
)

// Animator animates VolumeSeriesRequests
type Animator struct {
	RetryInterval              time.Duration
	StopPeriod                 time.Duration
	Log                        *logging.Logger
	mux                        sync.Mutex
	cancelRun                  context.CancelFunc
	stoppedChan                chan struct{}
	notifyFlagged              bool
	notifyChan                 chan struct{}
	wakeCount                  int
	activeRequests             map[models.ObjID]*RequestHandlerState
	doneRequests               map[models.ObjID]models.ObjVersion
	allocateCapacityHandlers   AllocateCapacityHandlers
	allocationHandlers         AllocationHandlers
	attachFsHandlers           AttachFsHandlers
	bindHandlers               BindHandlers
	cgSnapshotCreateHandlers   CGSnapshotCreateHandlers
	changeCapacityHandlers     ChangeCapacityHandlers
	createHandlers             CreateHandlers
	createFromSnapshotHandlers CreateFromSnapshotHandlers
	mountHandlers              MountHandlers
	nodeDeleteHandlers         NodeDeleteHandlers
	publishHandlers            PublishHandlers
	publishServicePlanHandlers PublishServicePlanHandlers
	renameHandlers             RenameHandlers
	volDetachHandlers          VolumeDetachHandlers
	volSnapshotCreateHandlers  VolSnapshotCreateHandlers
	volSnapshotRestoreHandlers VolSnapshotRestoreHandlers
	ops                        Ops
	OCrud                      crud.Ops
	CrudeOps                   crude.Ops
	oItems                     *crud.Updates
	NotifyCount                int
}

// Ops specifies common operations that can be performed by the animator
type Ops interface {
	// RunBody is called once every RetryInterval to perform any instance-specific behavior, such as cleanup or caching,
	// and to obtain a list of candidate VolumeSeriesRequest objects to be dispatched
	RunBody(context.Context) []*models.VolumeSeriesRequest
	// ShouldDispatch is called from DispatchRequests to determine if the given request should be dispatched in this animator instance
	ShouldDispatch(context.Context, *RequestHandlerState) bool
}

// module constants
const (
	RetryIntervalDefault = time.Second * 30
	StopPeriodDefault    = time.Second * 30
)

// NewAnimator creates a new, initialized animator.
// By default, all of the request handlers simply set RetryLater.
// The Animator should be specialized through the use of the various WithXXXHandlers functions.
func NewAnimator(retryInterval time.Duration, log *logging.Logger, ops Ops) *Animator {
	a := &Animator{}
	a.SetRetryInterval(retryInterval)
	a.StopPeriod = StopPeriodDefault
	a.notifyFlagged = true // protect against Notify() until started
	a.Log = log
	a.ops = ops
	a.allocateCapacityHandlers = a
	a.allocationHandlers = a
	a.attachFsHandlers = a
	a.bindHandlers = a
	a.cgSnapshotCreateHandlers = a
	a.changeCapacityHandlers = a
	a.createHandlers = a
	a.createFromSnapshotHandlers = a
	a.mountHandlers = a
	a.nodeDeleteHandlers = a
	a.publishHandlers = a
	a.publishServicePlanHandlers = a
	a.renameHandlers = a
	a.volDetachHandlers = a
	a.volSnapshotCreateHandlers = a
	a.volSnapshotRestoreHandlers = a
	a.activeRequests = make(map[models.ObjID]*RequestHandlerState)
	a.doneRequests = make(map[models.ObjID]models.ObjVersion)
	a.oItems = &crud.Updates{Set: []string{"volumeSeriesRequestState", "requestMessages"}}
	return a
}

func (a *Animator) mergeSetItems(items []string) *Animator {
	for _, item := range items {
		if item == "syncPeers" {
			panic("syncPeers specified in update fields")
		}
		if !util.Contains(a.oItems.Set, item) {
			a.oItems.Set = append(a.oItems.Set, item)
		}
	}
	return a
}

// WithAllocateCapacityHandlers sets the AllocateCapacityHandlers interface of the animator
// and adds capacityReservationPlan, capacityReservationResult, storageFormula and systemTags to the set of items to update during database updates
func (a *Animator) WithAllocateCapacityHandlers(h AllocateCapacityHandlers) *Animator {
	a.allocateCapacityHandlers = h
	return a.mergeSetItems([]string{"capacityReservationPlan", "capacityReservationResult", "servicePlanAllocationId", "storageFormula", "systemTags"})
}

// WithAllocationHandlers sets the AllocationHandlers interface of the animator
// and adds storagePlan to the set of items to update during database updates
func (a *Animator) WithAllocationHandlers(h AllocationHandlers) *Animator {
	a.allocationHandlers = h
	return a.mergeSetItems([]string{"storagePlan"})
}

// WithAttachFsHandlers sets the AttachFsHandlers interface of the animator
func (a *Animator) WithAttachFsHandlers(h AttachFsHandlers) *Animator {
	a.attachFsHandlers = h
	return a
}

// WithBindHandlers sets the BindHandlers interface of the animator
// and adds capacityReservationResult, servicePlanAllocationId and systemTags to the set of items to update during database updates
func (a *Animator) WithBindHandlers(h BindHandlers) *Animator {
	a.bindHandlers = h
	return a.mergeSetItems([]string{"capacityReservationResult", "servicePlanAllocationId", "storageFormula", "systemTags"})
}

// WithCGSnapshotCreateHandlers sets the CGSnapshotCreateHandlers interface of the animator
func (a *Animator) WithCGSnapshotCreateHandlers(h CGSnapshotCreateHandlers) *Animator {
	a.cgSnapshotCreateHandlers = h
	return a
}

// WithChangeCapacityHandlers sets the ChangeCapacityHandlers interface of the animator
// and adds capacityReservationResult and systemTags to the set of items to update during database updates
func (a *Animator) WithChangeCapacityHandlers(h ChangeCapacityHandlers) *Animator {
	a.changeCapacityHandlers = h
	return a.mergeSetItems([]string{"capacityReservationResult", "systemTags"})
}

// WithCreateHandlers sets the CreateHandlers interface of the animator
func (a *Animator) WithCreateHandlers(h CreateHandlers) *Animator {
	a.createHandlers = h
	// the request update logic adds volumeSeriesId on a case-by-case basis
	return a
}

// WithCreateFromSnapshotHandlers sets the CreateFromSnapshotHandlers interface of the animator
func (a *Animator) WithCreateFromSnapshotHandlers(h CreateFromSnapshotHandlers) *Animator {
	a.createFromSnapshotHandlers = h
	return a.mergeSetItems([]string{"systemTags"})
}

// WithMountHandlers sets the MountHandlers interface of the animator
// and adds mountedNodeDevice to the set of items to update during database updates
func (a *Animator) WithMountHandlers(h MountHandlers) *Animator {
	a.mountHandlers = h
	return a.mergeSetItems([]string{"mountedNodeDevice", "systemTags"})
}

// WithNodeDeleteHandlers sets the NodeDeleteHandlers interface of the animator
func (a *Animator) WithNodeDeleteHandlers(h NodeDeleteHandlers) *Animator {
	a.nodeDeleteHandlers = h
	return a.mergeSetItems([]string{"systemTags"})
}

// WithPublishHandlers sets the PublishHandlers interface of the animator
func (a *Animator) WithPublishHandlers(h PublishHandlers) *Animator {
	a.publishHandlers = h
	return a
}

// WithPublishServicePlanHandlers sets the PublishServicePlanHandlers interface of the animator
func (a *Animator) WithPublishServicePlanHandlers(h PublishServicePlanHandlers) *Animator {
	a.publishServicePlanHandlers = h
	return a
}

// WithRenameHandlers sets the RenameHandlers interface of the animator
// and adds systemTags to the set of items to update during database updates
func (a *Animator) WithRenameHandlers(h RenameHandlers) *Animator {
	a.renameHandlers = h
	return a.mergeSetItems([]string{"systemTags"})
}

// WithVolumeDetachHandlers sets the VolumeDetachHandlers interface of the animator
func (a *Animator) WithVolumeDetachHandlers(h VolumeDetachHandlers) *Animator {
	a.volDetachHandlers = h
	return a.mergeSetItems([]string{"systemTags"})
}

// WithVolSnapshotCreateHandlers sets the VolSnapshotCreateHandlers interface of the animator
// and adds snapshot and lifecycleManagementData to the set of items to update during database updates
func (a *Animator) WithVolSnapshotCreateHandlers(h VolSnapshotCreateHandlers) *Animator {
	a.volSnapshotCreateHandlers = h
	return a.mergeSetItems([]string{"mountedNodeDevice", "snapshot", "lifecycleManagementData", "systemTags"})
}

// WithVolSnapshotRestoreHandlers sets the VolSnapshotRestoreHandlers interface of the animator
func (a *Animator) WithVolSnapshotRestoreHandlers(h VolSnapshotRestoreHandlers) *Animator {
	a.volSnapshotRestoreHandlers = h
	return a.mergeSetItems([]string{"systemTags"})
}

// WithStopPeriod overrides the StopPeriodDefault
func (a *Animator) WithStopPeriod(p time.Duration) *Animator {
	a.StopPeriod = p
	return a
}

// SetRetryInterval changes the retry interval
func (a *Animator) SetRetryInterval(retryInterval time.Duration) {
	a.RetryInterval = RetryIntervalDefault
	if retryInterval > 0 {
		a.RetryInterval = retryInterval
	}
}

// Start starts the animator
func (a *Animator) Start(clientOps crud.Ops, crudeOps crude.Ops) {
	a.Log.Info("Starting VolumeSeriesRequest Animator")
	a.OCrud = clientOps
	a.CrudeOps = crudeOps
	ctx, cancel := context.WithCancel(context.Background())
	a.cancelRun = cancel
	a.wakeCount = 0
	a.notifyReset(true)
	go util.PanicLogger(a.Log, func() { a.run(ctx) })
}

// Stop terminates the animator
func (a *Animator) Stop() {
	if a.cancelRun != nil {
		a.Log.Info("Stopping VolumeSeriesRequest Animator")
		a.stoppedChan = make(chan struct{})
		a.cancelRun()
		select {
		case <-a.stoppedChan: // response received
		case <-time.After(a.StopPeriod):
			a.Log.Warning("Timed out waiting for termination")
		}
	}
	a.Log.Info("Stopped VolumeSeriesRequest Animator")
}

// Notify is used to awaken the run loop
func (a *Animator) Notify() {
	a.mux.Lock()
	defer a.mux.Unlock()
	a.NotifyCount++
	if !a.notifyFlagged {
		a.notifyFlagged = true
		close(a.notifyChan)
	}
}

// CrudeNotify satisfies the crude.Watcher interface
func (a *Animator) CrudeNotify(cbt crude.WatcherCallbackType, ce *crude.CrudEvent) error {
	if cbt == crude.WatcherEvent {
		reqID := models.ObjID(ce.ID())
		reqVer := models.ObjVersion(math.MaxInt32)
		if verStr, ok := ce.Scope[crude.ScopeMetaVersion]; ok {
			if tmpVer, err := strconv.Atoi(verStr); err != nil {
				a.Log.Debugf("CrudeNotify invalid version %s", verStr)
			} else {
				reqVer = models.ObjVersion(tmpVer)
			}
		}
		if !a.skipProcessing(reqID, reqVer) {
			a.Notify()
		}
	}
	return nil
}

func (a *Animator) notifyReset(force bool) {
	a.mux.Lock()
	defer a.mux.Unlock()
	if a.notifyFlagged || force {
		a.notifyFlagged = false
		a.notifyChan = make(chan struct{})
	}
}

func (a *Animator) skipProcessing(reqID models.ObjID, reqVer models.ObjVersion) bool {
	a.mux.Lock()
	defer a.mux.Unlock()
	if _, ok := a.activeRequests[reqID]; ok {
		a.Log.Debugf("VolumeSeriesRequest %s already being processed", reqID)
		return true
	}
	if doneVersion, ok := a.doneRequests[reqID]; ok {
		// An active request may become Done in another thread. Such outdated requests should be ignored
		if doneVersion > reqVer {
			a.Log.Debugf("VolumeSeriesRequest %s is already done in this animator (v%d < v%d)", reqID, reqVer, doneVersion)
			return true
		}
	}
	return false
}

func (a *Animator) run(ctx context.Context) {
	for {
		a.notifyReset(false) // notifications while body is running will force another call
		a.mux.Lock()
		// clean out old doneRequests before the new list of VSRs is queried
		a.doneRequests = make(map[models.ObjID]models.ObjVersion)
		a.mux.Unlock()
		a.DispatchRequests(ctx, a.ops.RunBody(ctx))
		select {
		case <-a.notifyChan: // channel closed on
			a.Log.Debug("VolumeSeriesRequest Animator awakened")
			a.wakeCount++
		case <-ctx.Done():
			close(a.stoppedChan) // notify waiter by closing the channel
			return
		case <-time.After(a.RetryInterval):
			// wait
			a.Log.Debug("VolumeSeriesRequest Animator new interval")
		}
	}
}
