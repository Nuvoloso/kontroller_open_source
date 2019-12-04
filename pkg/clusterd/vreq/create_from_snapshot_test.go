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


package vreq

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/snapshot"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestCreateFromSnapshot(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	ctx := context.Background()
	c := &Component{}
	c.Init(app)
	assert.NotNil(c.Log)
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc

	vsr := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VR-1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			ClusterID: "CLUSTER-1",
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "CREATING_FROM_SNAPSHOT",
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID:         "NODE-1",
				VolumeSeriesID: "VS-1",
				Snapshot: &models.SnapshotData{
					VolumeSeriesID: "VS-1",
					SnapIdentifier: "snap-1",
				},
			},
		},
	}

	newFakeCreateFromSnapshotOp := func() *fakeCreateFromSnapshotOp {
		op := &fakeCreateFromSnapshotOp{}
		op.c = c
		op.ops = op
		op.rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}
		return op
	}

	tl.Logger().Info("Case: init map, skip check, launch")
	tl.Flush()
	op := newFakeCreateFromSnapshotOp()
	op.retGIS = CfsInitMapInDB
	op.retNC = 1
	expCalled := []string{"GIS", "ISM", "LS"}
	tl.Flush()
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	tl.Logger().Info("Case: check, launch")
	tl.Flush()
	op = newFakeCreateFromSnapshotOp()
	op.retGIS = CfsCheckForSubordinate
	op.retNC = 1
	expCalled = []string{"GIS", "CFS", "LS"}
	tl.Flush()
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	tl.Logger().Info("Case: check, analyze, wait")
	tl.Flush()
	op = newFakeCreateFromSnapshotOp()
	op.retGIS = CfsCheckForSubordinate
	op.retCFS = true
	op.retActiveCount = 1
	expCalled = []string{"GIS", "CFS", "AS"}
	tl.Flush()
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	tl.Logger().Info("Case: check, analyze, fail")
	tl.Flush()
	op = newFakeCreateFromSnapshotOp()
	op.retGIS = CfsCheckForSubordinate
	op.retCFS = true
	op.retFailedCount = 1
	expCalled = []string{"GIS", "CFS", "AS"}
	tl.Flush()
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)

	tl.Logger().Info("Case: check, analyze, fetch, set")
	tl.Flush()
	op = newFakeCreateFromSnapshotOp()
	op.retGIS = CfsCheckForSubordinate
	op.retCFS = true
	expCalled = []string{"GIS", "CFS", "AS", "FS", "PUB", "CPD", "SR"}
	tl.Flush()
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	tl.Logger().Info("Case: fetch, set")
	tl.Flush()
	op = newFakeCreateFromSnapshotOp()
	op.retGIS = CfsFetchSubordinate
	op.retCFS = true
	expCalled = []string{"GIS", "FS", "PUB", "CPD", "SR"}
	tl.Flush()
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	tl.Logger().Info("Case: undo, subordinate running")
	tl.Flush()
	op = newFakeCreateFromSnapshotOp()
	op.retGIS = CfsUndoCheckForSubordinate
	op.retCFS = true
	op.retActiveCount = 1
	op.retFailedCount = 0
	op.rhs.TimedOut = true
	expCalled = []string{"GIS", "CFS", "AS"}
	tl.Flush()
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.TimedOut)

	tl.Logger().Info("Case: undo, subordinate succeeded")
	tl.Flush()
	op = newFakeCreateFromSnapshotOp()
	op.retGIS = CfsUndoCheckForSubordinate
	op.retCFS = true
	op.retActiveCount = 0
	op.retFailedCount = 0
	op.rhs.TimedOut = true
	expCalled = []string{"GIS", "CFS", "AS", "FS", "PUB", "CPD", "SR"}
	tl.Flush()
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.TimedOut)
	assert.Equal(0, len(op.rhs.Request.RequestMessages))

	tl.Logger().Info("Case: undo, subordinate failed")
	tl.Flush()
	op = newFakeCreateFromSnapshotOp()
	op.retGIS = CfsUndoCheckForSubordinate
	op.retCFS = true
	op.retActiveCount = 0
	op.retFailedCount = 1
	op.rhs.TimedOut = true
	expCalled = []string{"GIS", "CFS", "AS"}
	tl.Flush()
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.True(op.rhs.TimedOut)
	assert.Regexp("Timed out", op.rhs.Request.RequestMessages[0])

	tl.Logger().Info("Case: undo, subordinate not found")
	tl.Flush()
	op = newFakeCreateFromSnapshotOp()
	op.retGIS = CfsUndoCheckForSubordinate
	op.retCFS = false
	op.retActiveCount = 0
	op.retFailedCount = 0
	op.rhs.TimedOut = true
	expCalled = []string{"GIS", "CFS"}
	tl.Flush()
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.TimedOut)
	assert.Regexp("Timed out", op.rhs.Request.RequestMessages[0])

	// invoke the real handlers
	tl.Logger().Info("Case: real handler")
	tl.Flush()
	rhs := &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}
	rhs.InError = true
	c.CreateFromSnapshot(ctx, rhs)

	tl.Logger().Info("Case: real undo handler")
	tl.Flush()
	rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}
	rhs.Request.VolumeSeriesRequestState = "UNDO_CREATING_FROM_SNAPSHOT"
	fc.RetLsVRErr = fmt.Errorf("vsr-list")
	c.UndoCreateFromSnapshot(ctx, rhs)

	// check state strings exist up to CfsNoOp
	var ss cfsSubState
	for ss = CfsInitMapInDB; ss < CfsNoOp; ss++ {
		s := ss.String()
		tl.Logger().Debugf("Testing %d %s", ss, s)
		assert.Regexp("^Cfs", s)
	}
	assert.Regexp("^cfsSubState", ss.String())
}

type fakeCreateFromSnapshotOp struct {
	createFromSnapshotOp
	called         []string
	retGIS         cfsSubState
	retNC          int
	retFS          models.ObjIDMutable
	retCFS         bool
	retActiveCount int
	retFailedCount int
}

func (op *fakeCreateFromSnapshotOp) getInitialState(ctx context.Context) cfsSubState {
	op.called = append(op.called, "GIS")
	return op.retGIS
}

func (op *fakeCreateFromSnapshotOp) analyzeSubordinate(ctx context.Context) {
	op.called = append(op.called, "AS")
	op.activeCount = op.retActiveCount
	op.failedCount = op.retFailedCount
	if op.failedCount > 0 {
		op.rhs.InError = true
	} else if op.activeCount > 0 {
		op.rhs.RetryLater = true
	}
}

func (op *fakeCreateFromSnapshotOp) checkForSubordinate(ctx context.Context) {
	op.called = append(op.called, "CFS")
	op.foundSubordinate = op.retCFS
}

func (op *fakeCreateFromSnapshotOp) fetchSubordinate(ctx context.Context) {
	op.called = append(op.called, "FS")
	op.newVsID = op.retFS
}

func (op *fakeCreateFromSnapshotOp) initSubordinateMapInDB(ctx context.Context) {
	op.called = append(op.called, "ISM")
	op.skipSubordinateCheck = true
}

func (op *fakeCreateFromSnapshotOp) launchSubordinate(ctx context.Context) {
	op.called = append(op.called, "LS")
	op.numCreated = op.retNC
	if op.numCreated > 0 {
		op.rhs.RetryLater = true
	}
}

func (op *fakeCreateFromSnapshotOp) publish(ctx context.Context) {
	op.called = append(op.called, "PUB")
}

func (op *fakeCreateFromSnapshotOp) checkPublishDone(ctx context.Context) {
	op.called = append(op.called, "CPD")
}

func (op *fakeCreateFromSnapshotOp) setResult(ctx context.Context) {
	op.called = append(op.called, "SR")
}

func TestCreateFromSnapshotSteps(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	ctx := context.Background()
	c := &Component{}
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc

	snapIdentifier := "snap-1"
	vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VR-1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			ClusterID:      "CLUSTER-1",
			CompleteByTime: strfmt.DateTime(time.Now().Add(30 * time.Minute)),
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "CREATING_FROM_SNAPSHOT",
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID:         "NODE-1",
				VolumeSeriesID: "VS-1",
				Snapshot: &models.SnapshotData{
					VolumeSeriesID: "VS-1",
					SnapIdentifier: snapIdentifier,
				},
			},
		},
	}
	vsr := vsrClone(vsrObj)

	vsObj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: "VS-1",
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				ConsistencyGroupID: "CG-1",
				ServicePlanID:      "SP-1",
				SizeBytes:          swag.Int64(100000),
			},
		},
	}

	now := time.Now()
	snapList := &snapshot.SnapshotListOK{
		Payload: []*models.Snapshot{
			&models.Snapshot{
				SnapshotAllOf0: models.SnapshotAllOf0{
					Meta: &models.ObjMeta{
						ID:      "id1",
						Version: 1,
					},
					ConsistencyGroupID: models.ObjID(vsObj.ConsistencyGroupID),
					PitIdentifier:      "pitid",
					ProtectionDomainID: "pdId",
					SizeBytes:          swag.Int64Value(vsObj.SizeBytes),
					SnapIdentifier:     snapIdentifier,
					SnapTime:           strfmt.DateTime(now),
					VolumeSeriesID:     vsrObj.VolumeSeriesID,
				},
				SnapshotMutable: models.SnapshotMutable{
					DeleteAfterTime: strfmt.DateTime(now.Add(90 * 24 * time.Hour)),
					Locations: map[string]models.SnapshotLocation{
						"ps1": {CreationTime: strfmt.DateTime(now), CspDomainID: "ps1"},
					},
				},
			},
		},
	}

	newOp := func() *createFromSnapshotOp {
		op := &createFromSnapshotOp{}
		op.c = c
		op.ops = op
		op.rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr), VolumeSeries: vsClone(vsObj)}
		return op
	}
	var op *createFromSnapshotOp

	//  ***************************** analyzeSubordinate
	syncPeer := make(map[string]models.SyncPeer)

	tl.Logger().Infof("case: analyzeSubordinate: active")
	tl.Flush()
	op = newOp()
	syncPeer[snapIdentifier] = models.SyncPeer{State: "ACTIVE"}
	op.rhs.Request.SyncPeers = syncPeer
	op.foundSubordinate = true
	op.analyzeSubordinate(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(1, op.activeCount)
	assert.Zero(op.failedCount)

	tl.Logger().Infof("case: analyzeSubordinate: failed")
	tl.Flush()
	op = newOp()
	syncPeer[snapIdentifier] = models.SyncPeer{State: "FAILED"}
	op.rhs.Request.SyncPeers = syncPeer
	op.foundSubordinate = true
	op.analyzeSubordinate(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Equal(1, op.failedCount)
	assert.Zero(op.activeCount)
	assert.Regexp("snapshot restore failed", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: analyzeSubordinate: succeeded")
	tl.Flush()
	op = newOp()
	syncPeer[snapIdentifier] = models.SyncPeer{State: "SUCCEEDED"}
	op.rhs.Request.SyncPeers = syncPeer
	op.foundSubordinate = true
	op.analyzeSubordinate(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Zero(op.failedCount)
	assert.Zero(op.activeCount)

	tl.Logger().Infof("case: analyzeSubordinate: planOnly")
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.analyzeSubordinate(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Zero(op.failedCount)
	assert.Zero(op.activeCount)

	tl.Logger().Infof("case: analyzeSubordinate: invalid invocation")
	tl.Flush()
	op = newOp()
	assert.Panics(func() { op.analyzeSubordinate(ctx) })

	//  ***************************** checkForSubordinate

	tl.Logger().Infof("case: checkForSubordinate: planOnly")
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.checkForSubordinate(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	tl.Logger().Infof("case: checkForSubordinate: list error")
	tl.Flush()
	fc.RetLsVRErr = fmt.Errorf("vsr-list")
	op = newOp()
	op.checkForSubordinate(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("list error.*vsr-list", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: checkForSubordinate: list empty")
	tl.Flush()
	fc.RetLsVRErr = nil
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{}
	fc.InLsVRObj = nil
	op = newOp()
	op.checkForSubordinate(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Nil(op.rhs.Request.SyncPeers)
	assert.Empty(op.rhs.Request.SyncPeers)
	assert.NotNil(fc.InLsVRObj)
	assert.EqualValues(vsr.Meta.ID, swag.StringValue(fc.InLsVRObj.SyncCoordinatorID))
	assert.False(op.foundSubordinate)

	tl.Logger().Infof("case: checkForSubordinate: list not empty")
	tl.Flush()
	fc.RetLsVRErr = nil
	subVsr := &models.VolumeSeriesRequest{}
	subVsr.Meta = &models.ObjMeta{ID: "sub-1"}
	subVsr.VolumeSeriesRequestState = "STATE"
	subVsr.VolumeSeriesID = "new-vs"
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{
		Payload: []*models.VolumeSeriesRequest{subVsr},
	}
	fc.InLsVRObj = nil
	op = newOp()
	op.snapIdentifier = snapIdentifier
	syncPeer[snapIdentifier] = models.SyncPeer{}
	op.rhs.Request.SyncPeers = syncPeer
	op.checkForSubordinate(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.NotNil(op.rhs.Request.SyncPeers)
	assert.NotEmpty(op.rhs.Request.SyncPeers)
	sub, ok := op.rhs.Request.SyncPeers[snapIdentifier]
	assert.True(ok)
	assert.Equal(subVsr.VolumeSeriesRequestState, sub.State)
	assert.EqualValues(subVsr.Meta.ID, sub.ID)
	assert.EqualValues(subVsr.VolumeSeriesID, op.newVsID)
	assert.True(op.foundSubordinate)

	//  ***************************** getInitialState

	tl.Logger().Infof("case: getInitialState: syncPeers nil")
	tl.Flush()
	op = newOp()
	st := op.getInitialState(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(CfsInitMapInDB, st)
	assert.Equal(snapIdentifier, op.snapIdentifier)

	tl.Logger().Infof("case: getInitialState: syncPeers empty")
	tl.Flush()
	op = newOp()
	op.rhs.Request.SyncPeers = make(map[string]models.SyncPeer)
	st = op.getInitialState(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(CfsInitMapInDB, st)
	assert.Equal(snapIdentifier, op.snapIdentifier)

	tl.Logger().Infof("case: getInitialState: SNAPSHOT_RESTORE_DONE")
	tl.Flush()
	op = newOp()
	syncPeer[snapIdentifier] = models.SyncPeer{State: "SUCCEEDED"}
	op.rhs.Request.SyncPeers = syncPeer
	op.rhs.Request.VolumeSeriesRequestState = "SNAPSHOT_RESTORE_DONE"
	st = op.getInitialState(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(CfsFetchSubordinate, st)
	assert.Equal(snapIdentifier, op.snapIdentifier)

	tl.Logger().Infof("case: getInitialState: not SNAPSHOT_RESTORE_DONE, planOnly")
	tl.Flush()
	op = newOp()
	syncPeer[snapIdentifier] = models.SyncPeer{State: "ACTIVE"}
	op.rhs.Request.SyncPeers = syncPeer
	op.rhs.Request.VolumeSeriesRequestState = "notSNAPSHOT_RESTORE_DONE"
	op.rhs.Request.PlanOnly = swag.Bool(true)
	st = op.getInitialState(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(CfsCheckForSubordinate, st)
	assert.True(op.planOnly)
	assert.Equal(snapIdentifier, op.snapIdentifier)

	tl.Logger().Infof("case: getInitialState: UNDO_CREATING_FROM_SNAPSHOT")
	tl.Flush()
	op = newOp()
	op.rhs.Request.VolumeSeriesRequestState = "UNDO_CREATING_FROM_SNAPSHOT"
	st = op.getInitialState(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(CfsUndoCheckForSubordinate, st)
	assert.Equal(com.VolReqStateCreatingFromSnapshot, op.rhs.Request.VolumeSeriesRequestState)

	//  ***************************** initSubordinateMapInDB

	tl.Logger().Infof("case: initSubordinateMapInDB: updater error")
	tl.Flush()
	fc.RetVSRUpdaterErr = fmt.Errorf("vsr-updater-err")
	fc.InVSRUpdaterID = ""
	fc.InVSRUpdaterItems = nil
	op = newOp()
	op.snapIdentifier = snapIdentifier
	assert.Nil(op.rhs.Request.SyncPeers)
	op.initSubordinateMapInDB(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	expItems := &crud.Updates{Set: []string{"syncPeers"}}
	assert.Equal(expItems, fc.InVSRUpdaterItems)
	assert.EqualValues(op.rhs.Request.Meta.ID, fc.InVSRUpdaterID)

	tl.Logger().Infof("case: initSubordinateMapInDB: updater ok")
	tl.Flush()
	fc.RetVSRUpdaterErr = nil
	fc.InVSRUpdaterID = ""
	fc.InVSRUpdaterItems = nil
	fc.RetVSRUpdaterObj = nil
	op = newOp()
	op.snapIdentifier = snapIdentifier
	assert.Nil(op.rhs.Request.SyncPeers)
	op.initSubordinateMapInDB(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.NotNil(op.rhs.Request.SyncPeers)
	assert.NotEmpty(op.rhs.Request.SyncPeers)
	_, ok = op.rhs.Request.SyncPeers[snapIdentifier]
	assert.True(ok)
	assert.NotNil(fc.ModVSRUpdaterObj)
	assert.Equal(fc.ModVSRUpdaterObj, op.rhs.Request)
	assert.True(op.skipSubordinateCheck)

	//  ***************************** fetchSubordinate

	tl.Logger().Infof("case: fetchSubordinate: planOnly")
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.fetchSubordinate(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("Fetch subordinate VSR", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: fetchSubordinate: fetch error")
	tl.Flush()
	fc.InVRFetchID = ""
	fc.RetVRErr = fmt.Errorf("vsr-fetch")
	op = newOp()
	op.snapIdentifier = snapIdentifier
	op.fetchSubordinate(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	tl.Logger().Infof("case: fetchSubordinate: fetch ok")
	tl.Flush()
	fc.InVRFetchID = ""
	fc.RetVRErr = nil
	fc.RetVRObj = &models.VolumeSeriesRequest{}
	fc.RetVRObj.VolumeSeriesID = "new-vs"
	op = newOp()
	op.snapIdentifier = snapIdentifier
	op.fetchSubordinate(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(fc.RetVRObj.VolumeSeriesID, op.newVsID)

	//  ***************************** launchSubordinate

	tl.Logger().Infof("case: launchSubordinate: no ID, planOnly")
	tl.Flush()
	op = newOp()
	op.snapIdentifier = snapIdentifier
	op.planOnly = true
	op.launchSubordinate(ctx)
	assert.Nil(fc.InVRCCtx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("Launching CREATE, BIND, MOUNT, VOL_SNAPSHOT_RESTORE", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: launchSubordinate: no ID, failure to list snapshots")
	tl.Flush()
	op = newOp()
	op.snapIdentifier = snapIdentifier
	fc.RetLsSnapshotErr = fmt.Errorf("snap-list-error")
	op.launchSubordinate(ctx)
	assert.Nil(fc.InVRCCtx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("Launching CREATE, BIND, MOUNT, VOL_SNAPSHOT_RESTORE", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: launchSubordinate: no ID, more than 1 snapshot listed")
	tl.Flush()
	op = newOp()
	op.snapIdentifier = snapIdentifier
	fc.RetLsSnapshotErr = nil
	fc.RetLsSnapshotOk = &snapshot.SnapshotListOK{
		Payload: []*models.Snapshot{
			&models.Snapshot{},
			&models.Snapshot{},
		},
	}
	op.launchSubordinate(ctx)
	assert.Nil(fc.InVRCCtx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Regexp("Launching CREATE, BIND, MOUNT, VOL_SNAPSHOT_RESTORE", op.rhs.Request.RequestMessages[0])
	assert.Regexp("Snapshot .* invalid", op.rhs.Request.RequestMessages[1])

	tl.Logger().Infof("case: launchSubordinate: no ID, create error")
	tl.Flush()
	fc.PassThroughUVRObj = true
	fc.RetVRCErr = fmt.Errorf("vsr-create")
	op = newOp()
	op.snapIdentifier = snapIdentifier
	fc.RetLsSnapshotErr = nil
	fc.RetLsSnapshotOk = snapList
	op.rhs.Request.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{}
	op.launchSubordinate(ctx)
	assert.Equal(ctx, fc.InVRCCtx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("Launching CREATE, BIND, MOUNT, VOL_SNAPSHOT_RESTORE", op.rhs.Request.RequestMessages[0])
	assert.Regexp("vsr-create", op.rhs.Request.RequestMessages[1])
	assert.Equal(0, op.numCreated)

	tl.Logger().Infof("case: launchSubordinate: no ID, create ok")
	tl.Flush()
	fc.RetVRCErr = nil
	fc.InVRCArgs = nil
	op = newOp()
	op.snapIdentifier = snapIdentifier
	op.rhs.Request.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{}
	syncPeer[snapIdentifier] = models.SyncPeer{}
	op.rhs.Request.SyncPeers = syncPeer
	op.launchSubordinate(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(1, op.numCreated)
	assert.Equal(ctx, fc.InVRCCtx)
	assert.NotNil(fc.InVRCArgs)
	cA := &models.VolumeSeriesRequestCreateArgs{}
	cA.RequestedOperations = []string{com.VolReqOpCreate, com.VolReqOpBind, com.VolReqOpMount, com.VolReqOpVolRestoreSnapshot}
	cA.SyncCoordinatorID = models.ObjIDMutable(op.rhs.Request.Meta.ID)
	cA.CompleteByTime = op.rhs.Request.CompleteByTime
	cA.VolumeSeriesCreateSpec = op.rhs.Request.VolumeSeriesCreateSpec
	cA.VolumeSeriesCreateSpec.SystemTags = []string{fmt.Sprintf("%s:%s", com.SystemTagVsrRestoring, op.rhs.Request.Meta.ID)}
	cA.NodeID = op.rhs.Request.NodeID
	cA.ClusterID = op.rhs.Request.ClusterID
	sd := models.SnapshotData{
		ConsistencyGroupID: models.ObjIDMutable(snapList.Payload[0].ConsistencyGroupID),
		DeleteAfterTime:    snapList.Payload[0].DeleteAfterTime,
		Locations: []*models.SnapshotLocation{
			&models.SnapshotLocation{
				CreationTime: strfmt.DateTime(now),
				CspDomainID:  "ps1"},
		},
		PitIdentifier:      snapList.Payload[0].PitIdentifier,
		ProtectionDomainID: snapList.Payload[0].ProtectionDomainID,
		SizeBytes:          &snapList.Payload[0].SizeBytes,
		SnapTime:           snapList.Payload[0].SnapTime,
		VolumeSeriesID:     models.ObjIDMutable(op.rhs.VolumeSeries.Meta.ID),
		SnapIdentifier:     snapIdentifier,
	}
	cA.Snapshot = &sd
	cA.ProtectionDomainID = sd.ProtectionDomainID
	expVS := &models.VolumeSeriesRequest{}
	expVS.VolumeSeriesRequestCreateOnce = cA.VolumeSeriesRequestCreateOnce
	expVS.VolumeSeriesRequestCreateMutable = cA.VolumeSeriesRequestCreateMutable
	assert.Equal(expVS, fc.InVRCArgs)

	tl.Logger().Infof("case: launchSubordinate: ID present")
	tl.Flush()
	fc.RetVRCErr = fmt.Errorf("vsr-create")
	op = newOp()
	op.snapIdentifier = snapIdentifier
	syncPeer[snapIdentifier] = models.SyncPeer{ID: "subVSR"}
	op.rhs.Request.SyncPeers = syncPeer
	op.launchSubordinate(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(0, op.numCreated)

	//  ***************************** setResult

	tl.Logger().Infof("case: setResult: planOnly")
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.setResult(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("Set the new volumeSeries ID in systemTags", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: setResult: ok")
	tl.Flush()
	op = newOp()
	op.newVsID = "newVS"
	op.rhs.Request.VolumeSeriesRequestState = com.VolReqStateUndoCreatingFromSnapshot
	op.setResult(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.NotNil(op.rhs.Request.SystemTags)
	stVal := fmt.Sprintf("%s:%s", com.SystemTagVsrNewVS, op.newVsID)
	assert.Equal(stVal, op.rhs.Request.SystemTags[0])
	assert.Regexp(op.newVsID, op.rhs.Request.RequestMessages[0])
	assert.Equal(com.VolReqStateSnapshotRestoreDone, op.rhs.Request.VolumeSeriesRequestState)

	// if tag exists don't set it again
	assert.Equal(1, len(op.rhs.Request.RequestMessages))
	op.setResult(ctx)
	assert.Equal(1, len(op.rhs.Request.RequestMessages))
	assert.Equal(stVal, op.rhs.Request.SystemTags[0])

	// ***************************** publish
	tl.Logger().Infof("case: publish: planOnly")
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.publish(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	tl.Logger().Infof("case: publish: vsr list failure")
	tl.Flush()
	fc.RetLsVRErr = fmt.Errorf("vsr-list")
	op = newOp()
	op.publish(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("vsr-list", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: publish: vsr list not empty failure")
	tl.Flush()
	fc.RetLsVRErr = nil
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{
		Payload: []*models.VolumeSeriesRequest{vsrObj},
	}
	op = newOp()
	op.publish(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	tl.Logger().Infof("case: publish: create failure")
	tl.Flush()
	fc.RetLsVRErr = nil
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{}
	fc.PassThroughUVRObj = true
	fc.RetVRCErr = fmt.Errorf("vsr-create")
	op = newOp()
	timeBefore := time.Now()
	op.rhs.Request.CompleteByTime = strfmt.DateTime(timeBefore)
	op.publish(ctx)
	assert.Equal(ctx, fc.InVRCCtx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("Launching PUBLISH", op.rhs.Request.RequestMessages[0])
	assert.Regexp("vsr-create", op.rhs.Request.RequestMessages[1])
	assert.True(timeBefore.Before(time.Time(fc.InVRCArgs.CompleteByTime)))

	tl.Logger().Infof("case: publish: create ok")
	tl.Flush()
	fc.RetVRCErr = nil
	fc.InVRCArgs = nil
	fc.RetVRCObj = &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "testID",
			},
		},
	}
	systag := fmt.Sprintf("%s:%s", com.SystemTagVsrPublishVsr, "testID")
	op = newOp()
	op.publish(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(ctx, fc.InVRCCtx)
	assert.NotNil(fc.InVRCArgs)
	cA = &models.VolumeSeriesRequestCreateArgs{}
	cA.SyncCoordinatorID = models.ObjIDMutable(op.rhs.Request.Meta.ID)
	cA.RequestedOperations = []string{com.VolReqOpPublish}
	cA.CompleteByTime = op.rhs.Request.CompleteByTime
	cA.ClusterID = op.rhs.Request.ClusterID
	expVS = &models.VolumeSeriesRequest{}
	expVS.VolumeSeriesRequestCreateOnce = cA.VolumeSeriesRequestCreateOnce
	expVS.VolumeSeriesRequestCreateMutable = cA.VolumeSeriesRequestCreateMutable
	assert.Equal(expVS, fc.InVRCArgs)
	assert.Contains(op.rhs.Request.SystemTags, systag)
	assert.Equal("testID", op.publishVsrID)
	assert.Regexp("Launching PUBLISH", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: publish: has system tag")
	tl.Flush()
	op = newOp()
	systag = fmt.Sprintf("%s:%s", com.SystemTagVsrPublishVsr, "testID")
	op.rhs.Request.SystemTags = []string{systag}
	op.publish(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	// ***************************** hasPublishSystemTag
	tl.Logger().Infof("case: hasPublishSystemTag: empty systemtags")
	tl.Flush()
	op = newOp()
	ret := op.hasPublishSystemTag(ctx)
	assert.False(ret)

	tl.Logger().Infof("case: hasPublishSystemTag: no vsrPublish systemtag")
	tl.Flush()
	op = newOp()
	op.rhs.Request.SystemTags = []string{}
	ret = op.hasPublishSystemTag(ctx)
	assert.False(ret)

	tl.Logger().Infof("case: hasPublishSystemTag: systemTag exists")
	tl.Flush()
	op = newOp()
	systag = fmt.Sprintf("%s:%s", com.SystemTagVsrPublishVsr, "testID")
	op.rhs.Request.SystemTags = []string{systag}
	ret = op.hasPublishSystemTag(ctx)
	assert.True(ret)
	assert.Equal("testID", op.publishVsrID)

	// ***************************** checkPublishDone
	tl.Logger().Infof("case: checkPublishDone: vsr fetch failure")
	tl.Flush()
	op = newOp()
	fc.RetVRErr = fmt.Errorf("vsr-fetch")
	op.checkPublishDone(ctx)
	assert.True(op.rhs.RetryLater)

	tl.Logger().Infof("case: checkPublishDone: vsr not done yet")
	tl.Flush()
	op = newOp()
	fc.RetVRObj = &models.VolumeSeriesRequest{
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "notDone",
			},
		},
	}
	fc.RetVRErr = nil
	op.checkPublishDone(ctx)
	assert.True(op.rhs.RetryLater)

	tl.Logger().Infof("case: checkPublishDone: vsr not successful")
	tl.Flush()
	op = newOp()
	fc.RetVRObj.VolumeSeriesRequestState = com.VolReqStateFailed
	fc.RetVRErr = nil
	op.checkPublishDone(ctx)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	tl.Logger().Infof("case: checkPublishDone: vsr successful")
	tl.Flush()
	op = newOp()
	fc.RetVRObj.VolumeSeriesRequestState = com.VolReqStateSucceeded
	fc.RetVRErr = nil
	op.checkPublishDone(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
}
