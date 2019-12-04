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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	fakeState "github.com/Nuvoloso/kontroller/pkg/clusterd/state/fake"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	mockcsp "github.com/Nuvoloso/kontroller/pkg/csp/mock"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestCGSnapshotCreate(t *testing.T) {
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
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	app.CSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	app.StateGuard = util.NewCriticalSectionGuard()
	assert.NotNil(app.StateGuard)
	fso := fakeState.NewFakeClusterState()
	app.StateOps = fso

	c := newComponent()
	c.Init(app)
	assert.NotNil(c.Log)
	assert.Equal(app.Log, c.Log)
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
			ClusterID: "cl-1",
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "CG_SNAPSHOT_VOLUMES",
			},
		},
	}

	newFakeCgSnapCreateOp := func() *fakeCgSnapCreateOp {
		op := &fakeCgSnapCreateOp{}
		op.c = c
		op.ops = op
		op.rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}
		return op
	}

	tl.Logger().Info("Case: init map, skip check, launch, skip count")
	tl.Flush()
	op := newFakeCgSnapCreateOp()
	op.rhs.Request.VolumeSeriesRequestState = "CG_SNAPSHOT_VOLUMES"
	op.retGIS = CgSCInitMapInDB
	op.retNC = 2
	op.reiDelayMultiplier = time.Microsecond
	c.rei.SetProperty("cg-snapshot-pre-wait-delay", &rei.Property{IntValue: 1})
	expCalled := []string{"GIS", "ISM", "LRS"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.RetryLater)
	stashVal := op.rhs.StashGet(cgSnapCreateStashKey{})
	assert.NotNil(stashVal)
	op2, ok := stashVal.(*cgSnapCreateOp)
	assert.True(ok)
	assert.Equal(&op.cgSnapCreateOp, op2)
	// re-enter same op, changed state
	op.rhs.Request.VolumeSeriesRequestState = "CG_SNAPSHOT_WAIT"
	op.retGIS = CgSCCheckForSubordinates
	expCalled = []string{"GIS", "ISM", "LRS", "GIS"} // cumulative
	tl.Logger().Infof("* enter: %s, NC", op.rhs.Request.VolumeSeriesRequestState)
	assert.Equal(1, tl.CountPattern("cg-snapshot-pre-wait-delay"))
	assert.NotEmpty(op.rhs.Request.RequestMessages)
	assert.Regexp("cg-snapshot-pre-wait-delay", op.rhs.Request.RequestMessages[0].Message)
	tl.Flush()
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.True(op.rhs.RetryLater)

	tl.Logger().Info("Case: no creation history, active")
	tl.Flush()
	op = newFakeCgSnapCreateOp()
	op.rhs.Request.VolumeSeriesRequestState = "CG_SNAPSHOT_WAIT"
	op.retActiveCount = 2
	op.retGIS = CgSCCheckForSubordinates
	expCalled = []string{"GIS", "CFS", "CAS"}
	tl.Logger().Infof("* enter: %s, AC", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.True(op.rhs.RetryLater)

	tl.Logger().Info("Case: no creation history, detect all terminated")
	tl.Flush()
	op = newFakeCgSnapCreateOp()
	op.rhs.Request.VolumeSeriesRequestState = "CG_SNAPSHOT_WAIT"
	op.retGIS = CgSCCheckForSubordinates
	expCalled = []string{"GIS", "CFS", "CAS"}
	tl.Logger().Infof("* enter: %s, AC", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.RetryLater)
	stashVal = op.rhs.StashGet(cgSnapCreateStashKey{})
	assert.NotNil(stashVal)
	op2, ok = stashVal.(*cgSnapCreateOp)
	assert.True(ok)
	assert.Equal(&op.cgSnapCreateOp, op2)
	// re-enter same op, changed state
	op.rhs.Request.VolumeSeriesRequestState = "CG_SNAPSHOT_FINALIZE"
	op.retGIS = CgSCDone
	expCalled = []string{"GIS", "CFS", "CAS", "GIS"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.RetryLater)

	tl.Logger().Info("Case: no creation history, externally finalized")
	tl.Flush()
	op = newFakeCgSnapCreateOp()
	op.rhs.Request.VolumeSeriesRequestState = "CG_SNAPSHOT_FINALIZE"
	op.retGIS = CgSCDone
	expCalled = []string{"GIS"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.RetryLater)

	tl.Logger().Info("Case: undo, 1 active")
	tl.Flush()
	op = newFakeCgSnapCreateOp()
	op.rhs.Request.VolumeSeriesRequestState = "UNDO_CG_SNAPSHOT_VOLUMES"
	op.inError = true
	op.rhs.TimedOut = true
	op.retActiveCount = 1
	op.retGIS = CgSCUndoCheckForSubordinates
	expCalled = []string{"GIS", "CAS", "CFS", "CAS"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.True(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.TimedOut)

	tl.Logger().Info("Case: undo, unknown")
	tl.Flush()
	op = newFakeCgSnapCreateOp()
	op.rhs.Request.VolumeSeriesRequestState = "UNDO_CG_SNAPSHOT_VOLUMES"
	op.inError = true
	op.rhs.TimedOut = true
	op.retUnknownCount = 1
	op.retGIS = CgSCUndoCheckForSubordinates
	expCalled = []string{"GIS", "CAS", "CFS", "CAS"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.TimedOut)

	tl.Logger().Info("Case: undo, failed")
	tl.Flush()
	op = newFakeCgSnapCreateOp()
	op.rhs.Request.VolumeSeriesRequestState = "UNDO_CG_SNAPSHOT_VOLUMES"
	op.inError = true
	op.rhs.TimedOut = true
	op.retFailedCount = 1
	op.retGIS = CgSCUndoCheckForSubordinates
	expCalled = []string{"GIS", "CAS", "CAS"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.True(op.rhs.TimedOut)

	// invoke the real handlers

	tl.Logger().Info("Case: real handler, no history")
	tl.Flush()
	fc.RetLsVOk = volume_series.NewVolumeSeriesListOK()
	fc.RetLsVOk.Payload = []*models.VolumeSeries{}
	rhs := &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}
	c.CGSnapshotCreate(nil, rhs)
	assert.True(rhs.InError)
	stashVal = rhs.StashGet(cgSnapCreateStashKey{})
	assert.NotNil(stashVal)
	op2, ok = stashVal.(*cgSnapCreateOp)
	assert.True(ok)
	assert.Equal(time.Second, op2.reiDelayMultiplier)

	tl.Logger().Info("Case: real handler, with history")
	tl.Flush()
	rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}
	rhs.StashSet(cgSnapCreateStashKey{}, &cgSnapCreateOp{})
	c.CGSnapshotCreate(nil, rhs)
	assert.True(rhs.InError)

	tl.Logger().Info("Case: real undo handler, no history")
	tl.Flush()
	rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}
	rhs.Request.PlanOnly = swag.Bool(true)
	c.UndoCGSnapshotCreate(nil, rhs)

	// check state strings exist up to CgScError
	var ss cgSnapCreateSubState
	for ss = CgSCInitMapInDB; ss < CgSCNoOp; ss++ {
		s := ss.String()
		tl.Logger().Debugf("Testing %d %s", ss, s)
		assert.Regexp("^CgSC", s)
	}
	assert.Regexp("^cgSnapCreateSubState", ss.String())
}

type fakeCgSnapCreateOp struct {
	cgSnapCreateOp
	called          []string
	retGIS          cgSnapCreateSubState
	retNC           int
	retActiveCount  int
	retFailedCount  int
	retUnknownCount int
}

func (op *fakeCgSnapCreateOp) getInitialState(ctx context.Context) cgSnapCreateSubState {
	op.called = append(op.called, "GIS")
	return op.retGIS
}

func (op *fakeCgSnapCreateOp) initSubordinateMapInDB(ctx context.Context) {
	op.called = append(op.called, "ISM")
	op.skipSubordinateCheck = true
}

func (op *fakeCgSnapCreateOp) checkForSubordinates(ctx context.Context) {
	op.called = append(op.called, "CFS")
}

func (op *fakeCgSnapCreateOp) launchRemainingSubordinates(ctx context.Context) {
	op.called = append(op.called, "LRS")
	op.numCreated = op.retNC
}

func (op *fakeCgSnapCreateOp) countActiveSubordinateVSRs(ctx context.Context) {
	op.called = append(op.called, "CAS")
	op.activeCount = op.retActiveCount
	op.failedCount = op.retFailedCount
	op.unknownCount = op.retUnknownCount
	if op.failedCount > 0 {
		op.rhs.InError = true
	} else if op.activeCount > 0 {
		op.rhs.RetryLater = true
	}
}

func TestCGSnapshotCreateSteps(t *testing.T) {
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
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	app.CSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	app.StateGuard = util.NewCriticalSectionGuard()
	assert.NotNil(app.StateGuard)
	fso := fakeState.NewFakeClusterState()
	app.StateOps = fso
	ctx := context.Background()

	c := newComponent()
	c.Init(app)
	assert.NotNil(c.Log)
	assert.Equal(app.Log, c.Log)
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc

	vol1Obj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID:      "VS-1",
				Version: 1,
			},
		},
	}
	v1vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VSR-VS-1",
			},
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "PAUSING_IO",
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				VolumeSeriesID: "VS-1",
			},
		},
	}
	vol2Obj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID:      "VS-2",
				Version: 2,
			},
		},
	}
	v2vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VSR-VS-2",
			},
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "SNAPSHOT_UPLOADING",
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				VolumeSeriesID: "VS-2",
			},
		},
	}
	vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VR-1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			ClusterID:      "cl-1",
			CompleteByTime: strfmt.DateTime(time.Now().Add(time.Hour)),
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				ConsistencyGroupID: "cg-1",
			},
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "CG_SNAPSHOT_VOLUMES",
			},
		},
	}
	vsr := vsrClone(vsrObj)

	newOp := func() *cgSnapCreateOp {
		op := &cgSnapCreateOp{}
		op.c = c
		op.ops = op
		op.rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}
		return op
	}

	//  ***************************** getInitialState

	// UNDO_CG_SNAPSHOT_VOLUMES cases
	vsr.VolumeSeriesRequestState = "UNDO_CG_SNAPSHOT_VOLUMES"

	tl.Logger().Infof("case: getInitialState %s, planOnly", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op := newOp()
	op.rhs.Request.PlanOnly = swag.Bool(true)
	op.rhs.InError = true
	assert.Equal(CgSCUndoDone, op.getInitialState(ctx))
	assert.True(op.inError)
	assert.False(op.rhs.InError) // cleared
	assert.True(op.planOnly)

	tl.Logger().Infof("case: getInitialState %s", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	op.rhs.InError = true
	assert.Equal(CgSCUndoCheckForSubordinates, op.getInitialState(ctx))
	assert.True(op.inError)
	assert.False(op.rhs.InError) // cleared
	assert.False(op.planOnly)

	// CG_SNAPSHOT_VOLUMES cases
	vsr.VolumeSeriesRequestState = "CG_SNAPSHOT_VOLUMES"

	tl.Logger().Infof("case: getInitialState %s, nil map", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	assert.Equal(CgSCInitMapInDB, op.getInitialState(ctx))

	tl.Logger().Infof("case: getInitialState %s, empty map", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	op.rhs.Request.SyncPeers = make(map[string]models.SyncPeer)
	assert.Equal(CgSCInitMapInDB, op.getInitialState(ctx))

	tl.Logger().Infof("case: getInitialState %s, initialized map", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	op.rhs.Request.SyncPeers = map[string]models.SyncPeer{"vs1": models.SyncPeer{}}
	assert.Equal(CgSCCheckForSubordinates, op.getInitialState(ctx))

	// CG_SNAPSHOT_WAIT cases
	vsr.VolumeSeriesRequestState = "CG_SNAPSHOT_WAIT"
	tl.Logger().Infof("case: getInitialState %s", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	assert.Equal(CgSCCheckForSubordinates, op.getInitialState(ctx))

	// CG_SNAPSHOT_FINALIZE cases
	vsr.VolumeSeriesRequestState = "CG_SNAPSHOT_FINALIZE"
	tl.Logger().Infof("case: getInitialState %s", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	assert.Equal(CgSCDone, op.getInitialState(ctx))

	//  ***************************** initSubordinateMapInDB
	vsr.VolumeSeriesRequestState = "CG_SNAPSHOT_VOLUMES"

	tl.Logger().Infof("case: initSubordinateMapInDB %s, list error", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	fc.RetLsVErr = fmt.Errorf("vs-list")
	fc.InLsVObj = nil
	op = newOp()
	op.initSubordinateMapInDB(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.NotNil(fc.InLsVObj)
	assert.EqualValues(vsr.ConsistencyGroupID, swag.StringValue(fc.InLsVObj.ConsistencyGroupID))
	assert.EqualValues(vsr.ClusterID, swag.StringValue(fc.InLsVObj.BoundClusterID))
	assert.Equal([]string{com.VolStateInUse, com.VolStateConfigured, com.VolStateProvisioned}, fc.InLsVObj.VolumeSeriesState)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("vs-list", op.rhs.Request.RequestMessages[0].Message)
	assert.False(op.skipSubordinateCheck)

	tl.Logger().Infof("case: initSubordinateMapInDB %s, no volumes", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	fc.RetLsVErr = nil
	fc.RetLsVOk = &volume_series.VolumeSeriesListOK{}
	op = newOp()
	op.initSubordinateMapInDB(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("No.*volumes", op.rhs.Request.RequestMessages[0].Message)
	assert.False(op.skipSubordinateCheck)

	tl.Logger().Infof("case: initSubordinateMapInDB %s, no applicable volumes", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	fc.RetLsVErr = nil
	vsListOk := &volume_series.VolumeSeriesListOK{Payload: []*models.VolumeSeries{vsClone(vol1Obj), vsClone(vol2Obj)}}
	vsListOk.Payload[0].VolumeSeriesState = com.VolStateProvisioned
	vsListOk.Payload[0].LifecycleManagementData = &models.LifecycleManagementData{}
	vsListOk.Payload[1].VolumeSeriesState = com.VolStateProvisioned
	fc.RetLsVOk = vsListOk
	op = newOp()
	op.initSubordinateMapInDB(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("No.*volumes", op.rhs.Request.RequestMessages[0].Message)
	assert.False(op.skipSubordinateCheck)

	tl.Logger().Infof("case: initSubordinateMapInDB %s, found volumes, update error (fatal)", vsr.VolumeSeriesRequestState)
	tl.Flush()
	vsListOk = &volume_series.VolumeSeriesListOK{Payload: []*models.VolumeSeries{vsClone(vol1Obj), vsClone(vol2Obj)}}
	vsListOk.Payload[0].VolumeSeriesState = com.VolStateProvisioned
	vsListOk.Payload[0].LifecycleManagementData = &models.LifecycleManagementData{}
	vsListOk.Payload[0].SystemTags = []string{fmt.Sprintf("%s:node1", com.SystemTagVolumeLastConfiguredNode)}
	expMap := map[string]models.SyncPeer{
		string(vol1Obj.Meta.ID): models.SyncPeer{Annotation: "node1"},
		string(vol2Obj.Meta.ID): models.SyncPeer{},
	}
	op = newOp()
	fc.RetLsVErr = nil
	fc.RetLsVOk = vsListOk
	fc.RetVSRUpdaterUpdateErr = fmt.Errorf("vsr-update")
	fc.ModVSRUpdaterObj = nil
	fc.InVSRUpdaterItems = nil
	op = newOp()
	op.initSubordinateMapInDB(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.NotNil(fc.InVSRUpdaterItems)
	assert.Equal(&crud.Updates{Set: []string{"syncPeers", "requestMessages"}}, fc.InVSRUpdaterItems)
	assert.NotNil(fc.ModVSRUpdaterObj)
	assert.Equal(fc.ModVSRUpdaterObj, op.rhs.Request)
	assert.EqualValues(expMap, fc.ModVSRUpdaterObj.SyncPeers)
	assert.False(op.skipSubordinateCheck)

	tl.Logger().Infof("case: initSubordinateMapInDB %s, found volumes, update error (transient)", vsr.VolumeSeriesRequestState)
	tl.Flush()
	vsListOk = &volume_series.VolumeSeriesListOK{Payload: []*models.VolumeSeries{vsClone(vol1Obj), vsClone(vol2Obj)}}
	expMap = map[string]models.SyncPeer{
		string(vol1Obj.Meta.ID): models.SyncPeer{},
		string(vol2Obj.Meta.ID): models.SyncPeer{},
	}
	op = newOp()
	fc.RetLsVErr = nil
	fc.RetLsVOk = vsListOk
	err := crud.NewError(nil)
	fc.RetVSRUpdaterUpdateErr = err
	fc.ModVSRUpdaterObj = nil
	fc.InVSRUpdaterItems = nil
	op = newOp()
	op.initSubordinateMapInDB(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.NotNil(fc.InVSRUpdaterItems)
	assert.Equal(&crud.Updates{Set: []string{"syncPeers", "requestMessages"}}, fc.InVSRUpdaterItems)
	assert.NotNil(fc.ModVSRUpdaterObj)
	assert.Equal(fc.ModVSRUpdaterObj, op.rhs.Request)
	assert.EqualValues(expMap, fc.ModVSRUpdaterObj.SyncPeers)
	assert.False(op.skipSubordinateCheck)

	tl.Logger().Infof("case: initSubordinateMapInDB %s, created map", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	fc.RetLsVErr = nil
	fc.RetLsVOk = vsListOk
	fc.RetVSRUpdaterUpdateErr = nil
	op = newOp()
	op.initSubordinateMapInDB(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.EqualValues(expMap, op.rhs.Request.SyncPeers)
	assert.True(op.skipSubordinateCheck)

	//  ***************************** checkForSubordinates

	tl.Logger().Infof("case: checkForSubordinates, list error")
	tl.Flush()
	op = newOp()
	fc.RetLsVRErr = fmt.Errorf("vsr-list")
	fc.InLsVRObj = nil
	op = newOp()
	op.checkForSubordinates(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.NotNil(fc.InLsVObj)
	assert.EqualValues(vsr.Meta.ID, swag.StringValue(fc.InLsVRObj.SyncCoordinatorID))
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("vsr-list", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Infof("case: checkForSubordinates, empty map")
	tl.Flush()
	vsrListOk := &volume_series_request.VolumeSeriesRequestListOK{Payload: []*models.VolumeSeriesRequest{vsrClone(v1vsrObj), vsrClone(v2vsrObj)}}
	fc.RetLsVRErr = nil
	fc.RetLsVRObj = vsrListOk
	op = newOp()
	op.checkForSubordinates(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.NotNil(op.rhs.Request.SyncPeers)
	assert.Empty(op.rhs.Request.SyncPeers)

	tl.Logger().Infof("case: checkForSubordinates, updated map")
	tl.Flush()
	expMap = map[string]models.SyncPeer{
		string(vol1Obj.Meta.ID): models.SyncPeer{ID: models.ObjIDMutable(v1vsrObj.Meta.ID), State: v1vsrObj.VolumeSeriesRequestState},
	}
	fc.RetLsVRErr = nil
	fc.RetLsVRObj = vsrListOk
	op = newOp()
	op.rhs.Request.SyncPeers = map[string]models.SyncPeer{
		string(vol1Obj.Meta.ID): models.SyncPeer{},
	}
	op.checkForSubordinates(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.EqualValues(expMap, op.rhs.Request.SyncPeers)

	//  ***************************** launchRemainingSubordinates

	tl.Logger().Infof("case: launchRemainingSubordinates, create error (fatal)")
	tl.Flush()
	fc.RetVRCErr = fmt.Errorf("vsr-create")
	fc.InVRCArgs = nil
	op = newOp()
	op.rhs.Request.SyncPeers = map[string]models.SyncPeer{
		string(vol1Obj.Meta.ID): models.SyncPeer{},
	}
	op.launchRemainingSubordinates(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Equal(ctx, fc.InVRCCtx)
	assert.NotNil(fc.InVRCArgs)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("vsr-create", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Infof("case: launchRemainingSubordinates, create error (transient)")
	tl.Flush()
	err = crud.NewError(nil)
	fc.RetVRCErr = err
	fc.InVRCArgs = nil
	op = newOp()
	op.rhs.Request.SyncPeers = map[string]models.SyncPeer{
		string(vol1Obj.Meta.ID): models.SyncPeer{},
	}
	op.launchRemainingSubordinates(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(ctx, fc.InVRCCtx)
	assert.NotNil(fc.InVRCArgs)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("create error", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Infof("case: launchRemainingSubordinates, plan only")
	tl.Flush()
	fc.RetVRCErr = nil
	fc.InVRCArgs = nil
	op = newOp()
	op.planOnly = true
	op.rhs.Request.SyncPeers = map[string]models.SyncPeer{
		string(vol1Obj.Meta.ID): models.SyncPeer{ID: models.ObjIDMutable(v1vsrObj.Meta.ID), State: v1vsrObj.VolumeSeriesRequestState},
		string(vol2Obj.Meta.ID): models.SyncPeer{},
	}
	op.launchRemainingSubordinates(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Nil(fc.InVRCArgs)
	assert.Equal(0, op.numCreated)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Launch", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Infof("case: launchRemainingSubordinates, launch 1, skip 1")
	tl.Flush()
	fc.RetVRCErr = nil
	fc.InVRCArgs = nil
	op = newOp()
	op.rhs.Request.SyncPeers = map[string]models.SyncPeer{
		string(vol1Obj.Meta.ID): models.SyncPeer{ID: models.ObjIDMutable(v1vsrObj.Meta.ID), State: v1vsrObj.VolumeSeriesRequestState},
		string(vol2Obj.Meta.ID): models.SyncPeer{},
	}
	op.launchRemainingSubordinates(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(ctx, fc.InVRCCtx)
	assert.NotNil(fc.InVRCArgs)
	assert.EqualValues([]string{com.VolReqOpVolCreateSnapshot}, fc.InVRCArgs.RequestedOperations)
	assert.EqualValues(vsr.Meta.ID, fc.InVRCArgs.SyncCoordinatorID)
	assert.EqualValues(string(vol2Obj.Meta.ID), fc.InVRCArgs.VolumeSeriesID)
	assert.EqualValues(op.rhs.Request.CompleteByTime, fc.InVRCArgs.CompleteByTime)
	assert.Equal(1, op.numCreated)

	tl.Logger().Infof("case: launchRemainingSubordinates, configure first, node ok")
	tl.Flush()
	fc.RetVRCErr = nil
	fc.InVRCArgs = nil
	fso.InGHNid = ""
	fso.RetGHNobj = &models.Node{}
	fso.RetGHNobj.Meta = &models.ObjMeta{ID: "node1"}
	op = newOp()
	op.rhs.Request.SyncPeers = map[string]models.SyncPeer{
		string(vol2Obj.Meta.ID): models.SyncPeer{Annotation: "node1"},
	}
	op.launchRemainingSubordinates(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(ctx, fc.InVRCCtx)
	assert.NotNil(fc.InVRCArgs)
	assert.EqualValues([]string{com.VolReqOpConfigure, com.VolReqOpVolCreateSnapshot}, fc.InVRCArgs.RequestedOperations)
	assert.Equal("node1", fso.InGHNid)
	assert.EqualValues("node1", fc.InVRCArgs.NodeID)
	assert.EqualValues(vsr.Meta.ID, fc.InVRCArgs.SyncCoordinatorID)
	assert.EqualValues(string(vol2Obj.Meta.ID), fc.InVRCArgs.VolumeSeriesID)
	assert.EqualValues(op.rhs.Request.CompleteByTime, fc.InVRCArgs.CompleteByTime)
	assert.Equal(1, op.numCreated)

	tl.Logger().Infof("case: launchRemainingSubordinates, configure first, no node available")
	tl.Flush()
	fc.RetVRCErr = nil
	fc.InVRCArgs = nil
	fso.InGHNid = ""
	fso.RetGHNobj = nil
	op = newOp()
	op.rhs.Request.SyncPeers = map[string]models.SyncPeer{
		string(vol2Obj.Meta.ID): models.SyncPeer{Annotation: "node1"},
	}
	op.launchRemainingSubordinates(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Equal(ctx, fc.InVRCCtx)
	assert.Equal("node1", fso.InGHNid)
	assert.Equal(0, op.numCreated)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("no alternative.*node1", op.rhs.Request.RequestMessages[0].Message)

	//  ***************************** countActiveSubordinateVSRs

	tl.Logger().Infof("case: countActiveSubordinateVSRs, plan only")
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.rhs.Request.SyncPeers = map[string]models.SyncPeer{
		"v1": models.SyncPeer{ID: "vsrV1", State: "SUCCEEDED"},
		"v2": models.SyncPeer{ID: "vsrV2", State: "FAILED"},
		"v3": models.SyncPeer{ID: "vsrV3", State: "CANCELED"},
		"v4": models.SyncPeer{ID: "vsrV4", State: "NOT_TERMINATED"},
		"v5": models.SyncPeer{},
	}
	op.countActiveSubordinateVSRs(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(0, op.activeCount)
	assert.Equal(0, op.failedCount)

	tl.Logger().Infof("case: countActiveSubordinateVSRs (all completed)")
	tl.Flush()
	op = newOp()
	op.rhs.Request.SyncPeers = map[string]models.SyncPeer{
		"v1": models.SyncPeer{ID: "vsrV1", State: "SUCCEEDED"},
		"v2": models.SyncPeer{ID: "vsrV2", State: "SUCCEEDED"},
		"v3": models.SyncPeer{ID: "vsrV3", State: "SUCCEEDED"},
	}
	op.countActiveSubordinateVSRs(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(0, op.activeCount)
	assert.Equal(0, op.failedCount)

	tl.Logger().Infof("case: countActiveSubordinateVSRs (active, no errors)")
	tl.Flush()
	op = newOp()
	op.rhs.Request.SyncPeers = map[string]models.SyncPeer{
		"v1": models.SyncPeer{ID: "vsrV1", State: "SUCCEEDED"},
		"v2": models.SyncPeer{ID: "vsrV2", State: "SUCCEEDED"},
		"v3": models.SyncPeer{ID: "vsrV3", State: "SUCCEEDED"},
		"v4": models.SyncPeer{ID: "vsrV4", State: "NOT_TERMINATED"},
		"v5": models.SyncPeer{},
	}
	op.countActiveSubordinateVSRs(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(1, op.activeCount)
	assert.Equal(0, op.failedCount)
	assert.Equal(1, op.unknownCount)

	tl.Logger().Infof("case: countActiveSubordinateVSRs (errors present)")
	tl.Flush()
	op = newOp()
	op.rhs.Request.SyncPeers = map[string]models.SyncPeer{
		"v1": models.SyncPeer{ID: "vsrV1", State: "SUCCEEDED"},
		"v2": models.SyncPeer{ID: "vsrV2", State: "FAILED"},
		"v3": models.SyncPeer{ID: "vsrV3", State: "CANCELED"},
		"v4": models.SyncPeer{ID: "vsrV4", State: "NOT_TERMINATED"},
		"v5": models.SyncPeer{},
	}
	op.countActiveSubordinateVSRs(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Equal(1, op.activeCount)
	assert.Equal(2, op.failedCount)
	assert.Equal(1, op.unknownCount)
	assert.Regexp("Volume snapshots failed", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Infof("case: countActiveSubordinateVSRs (only unknown)")
	tl.Flush()
	op = newOp()
	op.rhs.Request.SyncPeers = map[string]models.SyncPeer{
		"v1": models.SyncPeer{ID: "vsrV1"},
		"v2": models.SyncPeer{ID: "vsrV2"},
		"v3": models.SyncPeer{ID: "vsrV3"},
		"v4": models.SyncPeer{ID: "vsrV4"},
		"v5": models.SyncPeer{},
	}
	op.countActiveSubordinateVSRs(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(0, op.activeCount)
	assert.Equal(0, op.failedCount)
	assert.Equal(5, op.unknownCount)

}
