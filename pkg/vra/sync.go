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
	"github.com/go-openapi/strfmt"
)

// DefaultSyncTickerPeriod is the period at which a blocked VSR will wake up to
// - check for cancelation if enabled (fetches the local VSR)
// - check for time out
const DefaultSyncTickerPeriod = time.Duration(30 * time.Second)

// SyncArgs contains the arguments to the SyncRequests method.
type SyncArgs struct {
	// The key of the local VSR in the coordinator VSR syncPeers map.
	LocalKey string
	// The state label to be set for the coordinating VSRs in the coordinator VSR syncPeers map.
	SyncState string
	// Optionally, a state label to set in the VolumeSeriesRequestState property of the coordinator VSR.
	CoordinatorStateOnSync string
	// The timeout limit.
	CompleteBy time.Time
	// If true ignore local and coordinator cancelation
	IgnoreCancel bool
	// Optional ticker period. The default is DefaultSyncTickerPeriod.
	TickerPeriod time.Duration
}

// Validate checks the receiver type for correctness
func (a *SyncArgs) Validate() error {
	if a.LocalKey == "" {
		return fmt.Errorf("missing LocalKey")
	}
	if !ValidateVolumeSeriesRequestState(a.SyncState) {
		return fmt.Errorf("invalid SyncState")
	}
	if a.CoordinatorStateOnSync != "" && !ValidateVolumeSeriesRequestState(a.CoordinatorStateOnSync) {
		return fmt.Errorf("invalid CoordinatorStateOnSync")
	}
	if a.CompleteBy.Before(time.Now()) {
		return fmt.Errorf("invalid CompleteBy")
	}
	return nil
}

// ErrSyncCanceled is returned if the local or coordinator VSRs are canceled during sync
var ErrSyncCanceled = fmt.Errorf("local or coordinator request canceled")

// SyncRequests synchronizes the state of this VSR with that of a set of peers via the syncPeers map of a coordination VSR.
// If the syncCoordinatorId of this VSR is not set then the method trivially returns with no error.
// See https://goo.gl/uM9i7U for more details.
// Normally the coordination VSR does not participate in the synchronization process; however this can be
// done by setting rhs.Request.SyncCoordinatorID to rhs.Request.Meta.ID in-memory.
// The call can fail if the local or coordinator VSRs are canceled unless IgnoreCancel is set.
func (rhs *RequestHandlerState) SyncRequests(ctx context.Context, args *SyncArgs) error {
	rs := &requestSync{rhs: rhs}
	rs.SyncArgs = *args
	rs.syncer = rs // self-reference
	return rs.Synchronize(ctx)
}

type syncer interface {
	syncWithPeers(ctx context.Context) (bool, error)
}

type requestSync struct {
	SyncArgs
	syncer        syncer
	rhs           *RequestHandlerState
	syncID        models.ObjIDMutable
	name          string
	done          bool
	mux           sync.Mutex
	notifyCount   int
	notifyFlagged bool
	notifyChan    chan struct{}
	watcherID     string
	origGen       int32
	isCanceled    bool
}

func (rs *requestSync) Synchronize(ctx context.Context) error {
	if err := rs.Validate(); err != nil {
		return err
	}
	rs.syncID = rs.rhs.Request.SyncCoordinatorID
	if string(rs.syncID) == "" {
		return nil
	}
	rs.origGen = -1 // unknown
	rs.name = fmt.Sprintf("sync:%s:%s:%s", rs.syncID, rs.rhs.Request.Meta.ID, rs.SyncState)
	rs.notifyReset(true)
	wid, _ := rs.rhs.A.CrudeOps.Watch(rs.getWatcherArgs(), rs)
	rs.rhs.A.Log.Debugf("watcher %s: [%s]", rs.name, wid)
	rs.watcherID = wid
	if rs.TickerPeriod <= 0 { // no error
		rs.TickerPeriod = DefaultSyncTickerPeriod
	}
	ticker := time.NewTicker(rs.TickerPeriod)
	defer ticker.Stop()
	var err error
	for err == nil {
		rs.notifyReset(false) // notifications while body is running will force another call
		rs.done, err = rs.syncer.syncWithPeers(ctx)
		if rs.done || err != nil {
			break
		}
		select {
		case <-rs.notifyChan: // channel closed on
			rs.rhs.A.Log.Debugf("%s: notify, cancelled:%v", rs.name, rs.isCanceled)
			if rs.isCanceled {
				err = ErrSyncCanceled
			}
		case <-ticker.C:
			if rs.CompleteBy.Before(time.Now()) {
				err = fmt.Errorf("timed out")
				rs.rhs.A.Log.Debugf("%s: timed out", rs.name)
				break
			}
			if !rs.IgnoreCancel {
				var vsr *models.VolumeSeriesRequest
				vsr, err = rs.rhs.A.OCrud.VolumeSeriesRequestFetch(ctx, string(rs.rhs.Request.Meta.ID))
				if err == nil {
					rs.rhs.Request = vsr
					if vsr.CancelRequested {
						err = ErrSyncCanceled
					}
				}
			}
		}
	}
	rs.rhs.A.CrudeOps.TerminateWatcher(rs.watcherID)
	rs.rhs.A.Log.Debugf("%s ⇒ [%v, %v]", rs.name, rs.done, err)
	return err
}

func (rs *requestSync) getWatcherArgs() *models.CrudWatcherCreateArgs {
	reqScopePat := fmt.Sprintf("syncCoordinatorId:%s", rs.syncID)
	cancelURIPat := fmt.Sprintf("^/volume-series-requests/(%s|%s)/cancel", rs.rhs.Request.Meta.ID, rs.syncID)
	return &models.CrudWatcherCreateArgs{
		Name: rs.name,
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{
				MethodPattern: "PATCH|POST",
				URIPattern:    "^/volume-series-requests/",
				ScopePattern:  reqScopePat,
			},
			&models.CrudMatcher{
				MethodPattern: "POST",
				URIPattern:    cancelURIPat, // either local or the sync coordinator is canceled
			},
		},
	}
}

func (rs *requestSync) CrudeNotify(cbt crude.WatcherCallbackType, ce *crude.CrudEvent) error {
	if cbt == crude.WatcherEvent {
		if strings.Contains(ce.TrimmedURI, "/cancel") {
			if rs.IgnoreCancel {
				return nil
			}
			rs.isCanceled = true
		}
		rs.notify()
	}
	return nil
}

func (rs *requestSync) notify() {
	rs.mux.Lock()
	defer rs.mux.Unlock()
	rs.notifyCount++
	if !rs.notifyFlagged {
		rs.notifyFlagged = true
		close(rs.notifyChan)
	}
}

func (rs *requestSync) notifyReset(force bool) {
	rs.mux.Lock()
	defer rs.mux.Unlock()
	if rs.notifyFlagged || force {
		rs.notifyFlagged = false
		rs.notifyChan = make(chan struct{})
	}
}

var errSyncBreakOut = fmt.Errorf("sync-not-an-error")

func (rs *requestSync) syncWithPeers(ctx context.Context) (bool, error) {
	var allInSync bool
	var version models.ObjVersion
	modifyFn := func(o *models.VolumeSeriesRequest) (*models.VolumeSeriesRequest, error) {
		if o == nil {
			return nil, nil // always fetch
		}
		var err error
		allInSync, _, err = rs.modifyCoordinator(o)
		version = o.Meta.Version
		return o, err
	}
	items := &crud.Updates{Set: []string{fmt.Sprintf("syncPeers.%s", rs.LocalKey)}} // only update local record
	if rs.CoordinatorStateOnSync != "" {
		items.Set = append(items.Set, "volumeSeriesRequestState", "requestMessages")
	}
	_, err := rs.rhs.A.OCrud.VolumeSeriesRequestUpdater(ctx, string(rs.syncID), modifyFn, items)
	if err != nil && err != errSyncBreakOut {
		return false, err
	}
	rs.rhs.A.Log.Debugf("%s ⇒ [%v, %d]", rs.name, allInSync, version)
	return allInSync, nil
}

func (rs *requestSync) modifyCoordinator(o *models.VolumeSeriesRequest) (bool, bool, error) {
	var allInSync bool
	var err error
	chg := false
	if o.CancelRequested && !rs.IgnoreCancel {
		err = ErrSyncCanceled
		rs.rhs.A.Log.Debugf("%s: coordinator CANCELED", rs.name)
	} else if o.SyncPeers == nil {
		err = fmt.Errorf("syncPeers not initialized")
	} else if localRecord, exists := o.SyncPeers[rs.LocalKey]; exists {
		if rs.origGen == -1 {
			// find peer max
			maxGen := int32(0)
			for pk, peer := range o.SyncPeers {
				if VolumeSeriesRequestStateIsTerminated(peer.State) {
					return false, chg, fmt.Errorf("peer [%s] has terminated", pk)
				}
				if peer.GenCounter > maxGen {
					maxGen = peer.GenCounter
				}
			}
			if localRecord.GenCounter != maxGen { // local not ready
				localRecord.GenCounter = maxGen
				o.SyncPeers[rs.LocalKey] = localRecord // update genCounter only
				chg = true
			}
			// check that all peers are in the same session
			minGen := maxGen
			for _, peer := range o.SyncPeers {
				if peer.GenCounter < minGen {
					minGen = peer.GenCounter
				}
			}
			if minGen != localRecord.GenCounter {
				if chg {
					return false, chg, nil // make local ready first
				}
				rs.rhs.A.Log.Debugf("%s: peers not ready", rs.name)
				return false, chg, errSyncBreakOut
			}
			rs.origGen = localRecord.GenCounter
		}
		if localRecord.State != rs.SyncState || localRecord.ID == "" {
			localRecord.State = rs.SyncState
			localRecord.ID = models.ObjIDMutable(rs.rhs.Request.Meta.ID)
			o.SyncPeers[rs.LocalKey] = localRecord
			chg = true
		}
		allInSync = true
		for pk, peer := range o.SyncPeers {
			if peer.State != rs.SyncState {
				allInSync = false
				if VolumeSeriesRequestStateIsTerminated(peer.State) {
					err = fmt.Errorf("peer [%s] has terminated", pk)
				}
				break
			}
		}
		// the last to synchronize updates the coordinator and sets the next genCounter
		if allInSync && chg && err == nil {
			localRecord.GenCounter++
			o.SyncPeers[rs.LocalKey] = localRecord
			if rs.CoordinatorStateOnSync != "" && o.VolumeSeriesRequestState != rs.CoordinatorStateOnSync {
				msg := fmt.Sprintf("State change: %s ⇒ %s", o.VolumeSeriesRequestState, rs.CoordinatorStateOnSync)
				o.VolumeSeriesRequestState = rs.CoordinatorStateOnSync
				ts := &models.TimestampedString{
					Message: msg,
					Time:    strfmt.DateTime(time.Now()),
				}
				o.RequestMessages = append(o.RequestMessages, ts)
			}
		}
		if !chg && err == nil {
			err = errSyncBreakOut
		}
	} else {
		err = fmt.Errorf("record [%s] not found in syncPeers", rs.LocalKey)
	}
	return allInSync, chg, err
}

// SyncAbortArgs contains arguments to the SyncAbort method
type SyncAbortArgs struct {
	// The key of the local VSR in the coordinator VSR syncPeers map.
	LocalKey string
	// The failure state to advertise - must be a terminal state
	LocalState string
}

// Validate checks the receiver type for correctness
func (a *SyncAbortArgs) Validate() error {
	if a.LocalKey == "" {
		return fmt.Errorf("missing LocalKey")
	}
	if !VolumeSeriesRequestStateIsTerminated(a.LocalState) {
		return fmt.Errorf("invalid terminal state")
	}
	return nil
}

// SyncAbort advertises failure of the local participant
func (rhs *RequestHandlerState) SyncAbort(ctx context.Context, args *SyncAbortArgs) error {
	if err := args.Validate(); err != nil {
		return err
	}
	syncID := rhs.Request.SyncCoordinatorID
	if string(syncID) == "" {
		return nil
	}
	modifyFn := func(o *models.VolumeSeriesRequest) (*models.VolumeSeriesRequest, error) {
		if o == nil {
			return nil, nil // always fetch
		}
		var err error
		if o.SyncPeers == nil {
			err = fmt.Errorf("syncPeers not initialized")
		} else if localRecord, exists := o.SyncPeers[args.LocalKey]; exists {
			if localRecord.State == args.LocalState {
				err = errSyncBreakOut
			} else {
				localRecord.State = args.LocalState
				o.SyncPeers[args.LocalKey] = localRecord
			}
		} else {
			err = fmt.Errorf("record [%s] not found in syncPeers", args.LocalKey)
		}
		return o, err
	}
	items := &crud.Updates{Set: []string{fmt.Sprintf("syncPeers.%s", args.LocalKey)}} // only update local record
	_, err := rhs.A.OCrud.VolumeSeriesRequestUpdater(ctx, string(syncID), modifyFn, items)
	if err != nil && err != errSyncBreakOut {
		rhs.A.Log.Errorf("abort: VolumeSeriesRequest %s Coordinator %s ⇒ %s", rhs.Request.Meta.ID, syncID, err.Error())
		return err
	}
	rhs.A.Log.Debugf("abort: VolumeSeriesRequest %s Coordinator %s ⇒ [%s %s]", rhs.Request.Meta.ID, syncID, args.LocalKey, args.LocalState)
	return nil
}
