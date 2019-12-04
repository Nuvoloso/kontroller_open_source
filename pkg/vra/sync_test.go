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
	"regexp"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestSyncInvocation(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	past := now.Add(-10 * time.Minute)
	future := now.Add(10 * time.Minute)

	sa := &SyncArgs{
		LocalKey:               "lkey",
		SyncState:              com.VolReqStateUndoPausedIO,
		CoordinatorStateOnSync: com.VolReqStateUndoSnapshotUploadDone,
		CompleteBy:             future,
	}
	assert.NoError(sa.Validate())
	sa.CoordinatorStateOnSync = ""
	assert.NoError(sa.Validate())

	// failure cases
	tcs := []SyncArgs{
		SyncArgs{LocalKey: "", SyncState: com.VolReqStateCreatedPiT, CoordinatorStateOnSync: "", CompleteBy: future},
		SyncArgs{LocalKey: "lkey", SyncState: "foo", CoordinatorStateOnSync: "", CompleteBy: future},
		SyncArgs{LocalKey: "lkey", SyncState: com.VolReqStateCreatedPiT, CoordinatorStateOnSync: "foo", CompleteBy: future},
		SyncArgs{LocalKey: "lkey", SyncState: com.VolReqStateCreatedPiT, CoordinatorStateOnSync: "", CompleteBy: past},
	}
	for i, tc := range tcs {
		assert.Errorf(tc.Validate(), "tc%d", i)
	}

	rhs := RequestHandlerState{
		Request: &models.VolumeSeriesRequest{},
	}
	assert.NoError(rhs.SyncRequests(nil, sa))
	sa.LocalKey = ""
	assert.Error(rhs.SyncRequests(nil, sa))
}

func TestSyncProcess(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	fc := &fake.Client{}
	evM := fev.NewFakeEventManager()
	ops := &fakeOps{}
	a := NewAnimator(0, tl.Logger(), ops)
	a.OCrud = fc
	a.CrudeOps = evM

	rhs := &RequestHandlerState{
		A:       a,
		Request: &models.VolumeSeriesRequest{},
	}
	rhs.Request.Meta = &models.ObjMeta{ID: "vsr-1"}
	rhs.Request.SyncCoordinatorID = "sync-1"

	rs := &requestSync{
		SyncArgs: SyncArgs{
			LocalKey:               "lkey",
			SyncState:              com.VolReqStateUndoPausedIO,
			CoordinatorStateOnSync: com.VolReqStateUndoSnapshotUploadDone,
		},
		rhs: rhs,
	}
	fs := &fakeSyncer{}
	rs.syncer = fs

	tl.Logger().Info("case: watcher args")
	wa := rs.getWatcherArgs()
	assert.NotNil(wa)
	assert.Len(wa.Matchers, 2)
	m := wa.Matchers[0]
	assert.NotEmpty(m.MethodPattern)
	tl.Logger().Debugf("0.MethodPattern: %s", m.MethodPattern)
	re := regexp.MustCompile(m.MethodPattern)
	assert.True(re.MatchString("PATCH"))
	assert.NotEmpty(m.URIPattern)
	tl.Logger().Debugf("0.URIPattern: %s", m.URIPattern)
	re = regexp.MustCompile(m.URIPattern)
	assert.True(re.MatchString("/volume-series-requests/id"))
	assert.NotEmpty(m.ScopePattern)
	tl.Logger().Debugf("0.ScopePattern: %s", m.ScopePattern)
	re = regexp.MustCompile(m.ScopePattern)
	assert.True(re.MatchString("abc:def clusterId:cluster nodeId:node syncCoordinatorId:sync-1"))

	m = wa.Matchers[1]
	assert.NotEmpty(m.MethodPattern)
	tl.Logger().Debugf("1.MethodPattern: %s", m.MethodPattern)
	re = regexp.MustCompile(m.MethodPattern)
	assert.True(re.MatchString("POST"))
	assert.NotEmpty(m.URIPattern)
	tl.Logger().Debugf("1.URIPattern: %s", m.URIPattern)
	re = regexp.MustCompile(m.URIPattern)
	assert.True(re.MatchString(fmt.Sprintf("/volume-series-requests/%s/cancel", rs.rhs.Request.Meta.ID)))
	assert.True(re.MatchString(fmt.Sprintf("/volume-series-requests/%s/cancel", rs.syncID)))
	assert.Empty(m.ScopePattern)

	tl.Logger().Info("case: time out")
	tl.Flush()
	rs.CompleteBy = time.Now().Add(10 * time.Millisecond)
	rs.TickerPeriod = time.Millisecond * 25
	err := rs.Synchronize(nil)
	assert.Error(err)
	assert.Regexp("timed out", err)
	assert.EqualValues(-1, rs.origGen)

	tl.Logger().Info("case: syncer error")
	tl.Flush()
	rs.CompleteBy = time.Now().Add(10 * time.Minute)
	rs.TickerPeriod = 0
	fs.retErr = fmt.Errorf("syncer-error")
	err = rs.Synchronize(nil)
	assert.Error(err)
	assert.Regexp("syncer-error", err)

	tl.Logger().Info("case: syncer success after notify, ignoring cancelation")
	tl.Flush()
	fs.retErr = nil
	fs.retDone = false
	fs.called = 0
	done := false
	rs.IgnoreCancel = true
	rs.isCanceled = false
	go func() {
		err = rs.Synchronize(nil)
		done = true
	}()

	for fs.called == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	// multiple notifications
	fs.blocked = true
	cancelationEvent := &crude.CrudEvent{
		TrimmedURI: "/volume-series-requests/id/cancel",
	}
	assert.NoError(rs.CrudeNotify(crude.WatcherEvent, &crude.CrudEvent{})) // indirectly calls notify
	assert.NoError(rs.CrudeNotify(crude.WatcherEvent, cancelationEvent))   // indirectly calls notify
	for n := fs.called; n == fs.called && fs.blocked; {
		fs.blocked = false
		time.Sleep(1 * time.Millisecond)
	}
	fs.retDone = true
	assert.NoError(rs.CrudeNotify(crude.WatcherEvent, &crude.CrudEvent{})) // indirectly calls notify
	for !done {
		time.Sleep(1 * time.Millisecond)
	}
	assert.NoError(err)

	tl.Logger().Info("case: syncer detects cancelation event")
	tl.Flush()
	fs.retErr = nil
	fs.retDone = false
	fs.called = 0
	done = false
	rs.isCanceled = false
	rs.IgnoreCancel = false
	go func() {
		err = rs.Synchronize(nil)
		done = true
	}()
	for fs.called == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	assert.NoError(rs.CrudeNotify(crude.WatcherEvent, cancelationEvent)) // indirectly calls notify
	for !done {
		time.Sleep(1 * time.Millisecond)
	}
	assert.Error(err)
	assert.Equal(ErrSyncCanceled, err)

	tl.Logger().Info("case: syncer detects local cancellation")
	tl.Flush()
	var canceledRequest *models.VolumeSeriesRequest
	testutils.Clone(rhs.Request, &canceledRequest)
	canceledRequest.CancelRequested = true
	fc.RetVRObj, fc.RetVRErr = canceledRequest, nil
	rs.isCanceled = false
	rs.IgnoreCancel = false
	rs.CompleteBy = time.Now().Add(10 * time.Minute)
	rs.TickerPeriod = time.Millisecond * 10
	fs = &fakeSyncer{}
	rs.syncer = fs
	err = rs.Synchronize(nil)
	assert.Equal(ErrSyncCanceled, err)

	tl.Logger().Info("case: syncer fails to fetch local")
	tl.Flush()
	errFetch := fmt.Errorf("FETCH-FAILED")
	fc.RetVRObj, fc.RetVRErr = nil, errFetch
	rs.isCanceled = false
	rs.IgnoreCancel = false
	rs.CompleteBy = time.Now().Add(10 * time.Minute)
	rs.TickerPeriod = time.Millisecond * 10
	fs = &fakeSyncer{}
	rs.syncer = fs
	err = rs.Synchronize(nil)
	assert.Equal(errFetch, err)
}

type fakeSyncer struct {
	called  int
	blocked bool
	retDone bool
	retErr  error
}

func (fs *fakeSyncer) syncWithPeers(ctx context.Context) (bool, error) {
	fs.called++
	for fs.blocked {
		time.Sleep(5 * time.Millisecond)
	}
	return fs.retDone, fs.retErr
}

func TestSyncerImplementation(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	fc := &fake.Client{}
	evM := fev.NewFakeEventManager()
	ops := &fakeOps{}
	a := NewAnimator(0, tl.Logger(), ops)
	a.OCrud = fc
	a.CrudeOps = evM

	rhs := &RequestHandlerState{
		A:       a,
		Request: &models.VolumeSeriesRequest{},
	}
	rhs.Request.Meta = &models.ObjMeta{ID: "vsr-1"}
	rhs.Request.SyncCoordinatorID = "sync-1"

	rs := &requestSync{
		SyncArgs: SyncArgs{
			LocalKey:   "lkey",
			SyncState:  com.VolReqStateUndoPausedIO,
			CompleteBy: time.Now().Add(1 * time.Minute),
		},
		rhs: rhs,
	}
	rs.syncer = rs // self-ref to use real implementation

	tl.Logger().Info("case: syncer invocation, items:[peer]")
	fc.RetVSRUpdaterErr = fmt.Errorf("updater-error")
	fc.InVSRUpdaterItems = nil
	err := rs.Synchronize(nil) // api call
	assert.Error(err)
	assert.Regexp("updater-error", err)
	assert.NotNil(fc.InVSRUpdaterItems)
	assert.Equal(&crud.Updates{Set: []string{"syncPeers.lkey"}}, fc.InVSRUpdaterItems)
	assert.EqualValues(-1, rs.origGen)

	tl.Logger().Info("case: syncPeers not initialized, items:[peer,state]")
	rs.SyncArgs.CoordinatorStateOnSync = com.VolReqStateUndoSnapshotUploadDone
	fc.RetVSRUpdaterErr = nil
	fc.InVSRUpdaterItems = nil
	fc.FetchVSRUpdaterObj = &models.VolumeSeriesRequest{}
	fc.FetchVSRUpdaterObj.Meta = &models.ObjMeta{ID: "coordVSR", Version: 8}
	err = rs.Synchronize(nil) // api call
	assert.Error(err)
	assert.Regexp("syncPeers not initialized", err)
	assert.NotNil(fc.InVSRUpdaterItems)
	assert.Equal(&crud.Updates{Set: []string{"syncPeers.lkey", "volumeSeriesRequestState", "requestMessages"}}, fc.InVSRUpdaterItems)
	assert.EqualValues(-1, rs.origGen)

	tl.Logger().Info("case: no local record")
	fc.RetVSRUpdaterErr = nil
	fc.FetchVSRUpdaterObj.SyncPeers = make(map[string]models.SyncPeer) // empty
	err = rs.Synchronize(nil)
	assert.Error(err)
	assert.Regexp("record.* not found in syncPeers", err)
	assert.EqualValues(-1, rs.origGen)

	tl.Logger().Info("case: peer terminated (at gen sync)")
	fc.RetVSRUpdaterErr = nil
	fc.FetchVSRUpdaterObj.SyncPeers = map[string]models.SyncPeer{
		"peer": models.SyncPeer{State: "FAILED"},
		"lkey": models.SyncPeer{},
	}
	err = rs.Synchronize(nil) // api call
	assert.Error(err)
	assert.Regexp("peer.* has terminated", err)
	assert.EqualValues(-1, rs.origGen)

	tl.Logger().Info("case: peer terminated (after gen sync, chg)")
	vsrObj := &models.VolumeSeriesRequest{}
	vsrObj.Meta = &models.ObjMeta{ID: "coordVSR", Version: 8}
	vsrObj.SyncPeers = map[string]models.SyncPeer{
		"peer": models.SyncPeer{State: "FAILED", GenCounter: 1},
		"lkey": models.SyncPeer{GenCounter: 1},
	}
	rs.origGen = 1
	allInSync, chg, err := rs.modifyCoordinator(vsrObj) // updater modify
	assert.False(allInSync)
	assert.True(chg)
	assert.Error(err)
	assert.Regexp("peer.* has terminated", err)
	assert.EqualValues(1, rs.origGen)

	tl.Logger().Info("case: peer terminated (after gen sync, !chg)")
	vsrObj = &models.VolumeSeriesRequest{}
	vsrObj.Meta = &models.ObjMeta{ID: "coordVSR", Version: 8}
	vsrObj.SyncPeers = map[string]models.SyncPeer{
		"peer": models.SyncPeer{State: "FAILED", GenCounter: 1},
		"lkey": models.SyncPeer{State: rs.SyncState, GenCounter: 1, ID: models.ObjIDMutable(rs.rhs.Request.Meta.ID)},
	}
	rs.origGen = 1
	allInSync, chg, err = rs.modifyCoordinator(vsrObj) // updater modify
	assert.False(allInSync)
	assert.False(chg)
	assert.Error(err)
	assert.Regexp("peer.* has terminated", err)
	assert.EqualValues(1, rs.origGen)

	tl.Logger().Info("case: updated, peers not in sync")
	vsrObj = &models.VolumeSeriesRequest{}
	vsrObj.Meta = &models.ObjMeta{ID: "coordVSR", Version: 8}
	vsrObj.SyncPeers = map[string]models.SyncPeer{
		"peer1": models.SyncPeer{State: "P1"},
		"peer2": models.SyncPeer{State: "P2"},
		"lkey":  models.SyncPeer{},
	}
	rs.origGen = -1
	allInSync, chg, err = rs.modifyCoordinator(vsrObj) // updater modify
	assert.NoError(err)
	assert.False(allInSync)
	lr, ok := vsrObj.SyncPeers["lkey"]
	assert.True(ok)
	assert.Equal(rs.SyncArgs.SyncState, lr.State)
	assert.EqualValues(rs.rhs.Request.Meta.ID, lr.ID)
	assert.EqualValues(0, rs.origGen)

	tl.Logger().Info("case: no update, peers not in sync")
	vsrObj = &models.VolumeSeriesRequest{}
	vsrObj.Meta = &models.ObjMeta{ID: "coordVSR", Version: 8}
	vsrObj.SyncPeers = map[string]models.SyncPeer{
		"peer1": models.SyncPeer{State: "P1"},
		"peer2": models.SyncPeer{State: "P2"},
		"lkey":  models.SyncPeer{ID: models.ObjIDMutable(rs.rhs.Request.Meta.ID), State: rs.SyncArgs.SyncState},
	}
	fc.FetchVSRUpdaterObj = vsrObj
	rs.origGen = -1
	allInSync, err = rs.syncWithPeers(nil) // syncer call
	assert.NoError(err)
	assert.False(allInSync)
	assert.EqualValues(0, rs.origGen)

	tl.Logger().Info("case: last to sync, coordinator update")
	rs.SyncArgs.CoordinatorStateOnSync = com.VolReqStateSnapshotUploadDone
	vsrObj = &models.VolumeSeriesRequest{}
	vsrObj.Meta = &models.ObjMeta{ID: "coordVSR", Version: 8}
	vsrObj.VolumeSeriesRequestState = "FOO"
	vsrObj.SyncPeers = map[string]models.SyncPeer{
		"peer1": models.SyncPeer{State: rs.SyncArgs.SyncState, GenCounter: 2}, // not completed
		"peer2": models.SyncPeer{State: rs.SyncArgs.SyncState, GenCounter: 3}, // completed
		"lkey":  models.SyncPeer{ID: models.ObjIDMutable(rs.rhs.Request.Meta.ID), State: "FOO", GenCounter: 2},
	}
	rs.origGen = 2                                     // will not trigger initial gen check
	allInSync, chg, err = rs.modifyCoordinator(vsrObj) // updater modify
	assert.NoError(err)
	assert.True(allInSync)
	assert.Equal(rs.SyncArgs.CoordinatorStateOnSync, vsrObj.VolumeSeriesRequestState)
	assert.Len(vsrObj.RequestMessages, 1)
	assert.Regexp("State change: FOO .* SNAPSHOT_UPLOAD_DONE", vsrObj.RequestMessages[0].Message)
	assert.EqualValues(3, vsrObj.SyncPeers["lkey"].GenCounter)
	assert.EqualValues(2, rs.origGen)

	tl.Logger().Info("case: last to sync, no coordinator update")
	rs.SyncArgs.CoordinatorStateOnSync = ""
	vsrObj = &models.VolumeSeriesRequest{}
	vsrObj.Meta = &models.ObjMeta{ID: "coordVSR", Version: 8}
	vsrObj.SyncPeers = map[string]models.SyncPeer{
		"peer1": models.SyncPeer{State: rs.SyncArgs.SyncState},
		"peer2": models.SyncPeer{State: rs.SyncArgs.SyncState},
		"lkey":  models.SyncPeer{ID: models.ObjIDMutable(rs.rhs.Request.Meta.ID), State: "FOO"},
	}
	rs.origGen = -1
	allInSync, chg, err = rs.modifyCoordinator(vsrObj) // updater modify
	assert.NoError(err)
	assert.True(allInSync)
	assert.EqualValues(0, rs.origGen)
	assert.EqualValues(1, vsrObj.SyncPeers["lkey"].GenCounter)

	tl.Logger().Info("case: peers not ready")
	vsrObj = &models.VolumeSeriesRequest{}
	vsrObj.Meta = &models.ObjMeta{ID: "coordVSR", Version: 8}
	vsrObj.SyncPeers = map[string]models.SyncPeer{
		"peer1": models.SyncPeer{State: "S1", GenCounter: 2},
		"peer2": models.SyncPeer{State: "S1", GenCounter: 1},
		"lkey":  models.SyncPeer{ID: models.ObjIDMutable(rs.rhs.Request.Meta.ID), State: "S1", GenCounter: 2},
	}
	rs.SyncState = "S2"
	rs.origGen = -1
	allInSync, chg, err = rs.modifyCoordinator(vsrObj)
	assert.Error(err)
	assert.Equal(errSyncBreakOut, err)
	assert.False(allInSync)
	assert.EqualValues(2, vsrObj.SyncPeers["lkey"].GenCounter)
	assert.Equal("S1", vsrObj.SyncPeers["lkey"].State)
	assert.EqualValues(-1, rs.origGen)

	tl.Logger().Info("case: local + peers not ready")
	vsrObj = &models.VolumeSeriesRequest{}
	vsrObj.Meta = &models.ObjMeta{ID: "coordVSR", Version: 8}
	vsrObj.SyncPeers = map[string]models.SyncPeer{
		"peer1": models.SyncPeer{State: "S1", GenCounter: 2},
		"peer2": models.SyncPeer{State: "S1", GenCounter: 1},
		"lkey":  models.SyncPeer{ID: models.ObjIDMutable(rs.rhs.Request.Meta.ID), State: "S1", GenCounter: 1},
	}
	rs.SyncState = "S2"
	rs.origGen = -1
	allInSync, chg, err = rs.modifyCoordinator(vsrObj)
	assert.NoError(err)
	assert.False(allInSync)
	assert.EqualValues(2, vsrObj.SyncPeers["lkey"].GenCounter) // local now ready
	assert.Equal("S1", vsrObj.SyncPeers["lkey"].State)         // no change in state
	assert.EqualValues(-1, rs.origGen)

	tl.Logger().Info("case: local only not ready, ignore cancel")
	vsrObj = &models.VolumeSeriesRequest{}
	vsrObj.Meta = &models.ObjMeta{ID: "coordVSR", Version: 8}
	vsrObj.SyncPeers = map[string]models.SyncPeer{
		"peer1": models.SyncPeer{State: "S1", GenCounter: 2},
		"peer2": models.SyncPeer{State: "S1", GenCounter: 2},
		"lkey":  models.SyncPeer{ID: models.ObjIDMutable(rs.rhs.Request.Meta.ID), State: "S1", GenCounter: 1},
	}
	vsrObj.CancelRequested = true
	rs.IgnoreCancel = true
	rs.SyncState = "S2"
	rs.origGen = -1
	allInSync, chg, err = rs.modifyCoordinator(vsrObj)
	assert.NoError(err)
	assert.False(allInSync)
	assert.EqualValues(2, vsrObj.SyncPeers["lkey"].GenCounter) // local now ready
	assert.Equal("S2", vsrObj.SyncPeers["lkey"].State)         // state changed
	assert.EqualValues(2, rs.origGen)

	tl.Logger().Info("case: cancel requested")
	vsrObj = &models.VolumeSeriesRequest{}
	vsrObj.Meta = &models.ObjMeta{ID: "coordVSR", Version: 8}
	vsrObj.CancelRequested = true
	rs.IgnoreCancel = false
	allInSync, chg, err = rs.modifyCoordinator(vsrObj)
	assert.Error(err)
	assert.Equal(ErrSyncCanceled, err)
	assert.False(chg)
	assert.False(allInSync)
}

func TestSyncAbort(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	fc := &fake.Client{}
	evM := fev.NewFakeEventManager()
	ops := &fakeOps{}
	a := NewAnimator(0, tl.Logger(), ops)
	a.OCrud = fc
	a.CrudeOps = evM

	rhs := &RequestHandlerState{
		A:       a,
		Request: &models.VolumeSeriesRequest{},
	}
	rhs.Request.Meta = &models.ObjMeta{ID: "vsr-1"}
	rhs.Request.SyncCoordinatorID = "sync-1"

	var err error

	// failure cases
	tcs := []SyncAbortArgs{
		SyncAbortArgs{LocalKey: "", LocalState: com.VolReqStateCreatedPiT},
		SyncAbortArgs{LocalKey: "lkey", LocalState: com.VolReqStateCreatedPiT},
	}
	for _, tc := range tcs {
		tl.Logger().Info("case: invalid arguments %v", tc)
		err = rhs.SyncAbort(nil, &tc)
		assert.Error(err)
	}

	saa := &SyncAbortArgs{LocalKey: "lkey", LocalState: com.VolReqStateFailed}
	fc.FetchVSRUpdaterObj = &models.VolumeSeriesRequest{}

	tl.Logger().Info("case: syncPeers not initialized")
	fc.RetVSRUpdaterErr = nil
	fc.FetchVSRUpdaterObj.SyncPeers = nil
	err = rhs.SyncAbort(nil, saa)
	assert.Error(err)
	assert.Regexp("syncPeers not initialized", err)

	tl.Logger().Info("case: no local record")
	fc.RetVSRUpdaterErr = nil
	fc.FetchVSRUpdaterObj.SyncPeers = make(map[string]models.SyncPeer) // empty
	err = rhs.SyncAbort(nil, saa)
	assert.Error(err)
	assert.Regexp("record.* not found in syncPeers", err)

	tl.Logger().Info("case: already updated")
	fc.RetVSRUpdaterErr = nil
	fc.ModVSRUpdaterErr2 = nil
	fc.FetchVSRUpdaterObj.SyncPeers = map[string]models.SyncPeer{
		"lkey": models.SyncPeer{State: "FAILED"},
		"peer": models.SyncPeer{},
	}
	err = rhs.SyncAbort(nil, saa)
	assert.NoError(err)
	assert.Equal(errSyncBreakOut, fc.ModVSRUpdaterErr2)

	tl.Logger().Info("case: updating")
	fc.FetchVSRUpdaterObj.SyncPeers = map[string]models.SyncPeer{
		"lkey": models.SyncPeer{State: "NOT-FAILED"},
		"peer": models.SyncPeer{},
	}
	err = rhs.SyncAbort(nil, saa)
	assert.NoError(err)
	assert.Nil(fc.ModVSRUpdaterErr2)
	assert.Equal("FAILED", fc.FetchVSRUpdaterObj.SyncPeers["lkey"].State)
	assert.Equal([]string{"syncPeers.lkey"}, fc.InVSRUpdaterItems.Set)
	assert.Nil(fc.InVSRUpdaterItems.Append)
	assert.Nil(fc.InVSRUpdaterItems.Remove)

	tl.Logger().Info("case: no coordinator")
	rhs.Request.SyncCoordinatorID = ""
	fc.RetVSRUpdaterErr = fmt.Errorf("updater-error")
	err = rhs.SyncAbort(nil, saa)
	assert.NoError(err)
}
