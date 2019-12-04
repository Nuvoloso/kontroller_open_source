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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	fhk "github.com/Nuvoloso/kontroller/pkg/housekeeping/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	fVra "github.com/Nuvoloso/kontroller/pkg/vra/fake"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestNodeDelete(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	sfc := &fakeSFC{}
	app.SFC = sfc
	fts := &fhk.TaskScheduler{}
	app.TaskScheduler = fts
	ctx := context.Background()
	c := &Component{}
	c.Init(app)
	assert.NotNil(c.Log)
	assert.NotNil(c.reservationCS)
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc

	maxTime := util.DateTimeMaxUpperBound()
	vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VR-1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			RequestedOperations: []string{"NODE_DELETE"},
			ClusterID:           "CLUSTER-1",
			CompleteByTime:      strfmt.DateTime(maxTime),
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID: "NODE-1",
			},
		},
	}
	nObj := &models.Node{}
	nObj.Meta = &models.ObjMeta{ID: "NODE-1"}
	nObj.State = common.NodeStateTearDown

	activeSR := []*models.StorageRequest{
		&models.StorageRequest{},
		&models.StorageRequest{},
	}
	attachedStg := []*models.Storage{
		&models.Storage{},
		&models.Storage{},
	}

	newOp := func(state string) *fakeNodeDeleteOp {
		op := &fakeNodeDeleteOp{}
		op.c = c
		op.ops = op
		op.rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsrObj)}
		op.rhs.Request.VolumeSeriesRequestState = state
		return op
	}
	var expCalled []string
	var op *fakeNodeDeleteOp

	tl.Logger().Info("Case: DRAINING_REQUESTS (node state not TEAR_DOWN, vsr/sr wait success)")
	tl.Flush()
	op = newOp("DRAINING_REQUESTS")
	op.retGIS = NdLoadNode
	testutils.Clone(nObj, &op.retFN)
	op.retFN.State = common.NodeStateTimedOut
	op.srObjs = []*models.StorageRequest{&models.StorageRequest{}}
	op.srsToWaitOn = []*models.StorageRequest{&models.StorageRequest{}}
	op.vsrsToWaitOn = []*models.VolumeSeriesRequest{&models.VolumeSeriesRequest{}}
	expCalled = []string{"GIS", "FN", "TdN", "LASR", "WFSR", "LVsr", "WVsr"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	stashVal := op.rhs.StashGet(nodeDeleteStashKey{})
	assert.NotNil(stashVal)
	sOp, ok := stashVal.(*nodeDeleteOp)
	assert.True(ok)

	tl.Logger().Info("Case: DRAINING_REQUESTS (node state TEAR_DOWN, vsr wait failure)")
	tl.Flush()
	op = newOp("DRAINING_REQUESTS")
	op.retGIS = NdLoadNode
	op.retFN = nObj
	op.vsrsToWaitOn = []*models.VolumeSeriesRequest{&models.VolumeSeriesRequest{}}
	op.retWVsrRetryLater = true
	expCalled = []string{"GIS", "FN", "LASR", "LVsr", "WVsr"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)

	tl.Logger().Info("Case: CANCELING_REQUESTS (no srs/vsrs)")
	tl.Flush()
	op = newOp("CANCELING_REQUESTS")
	op.retGIS = NdFindActiveSRs
	expCalled = []string{"GIS", "LASR", "LVsr"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	tl.Logger().Info("Case: CANCELING_REQUESTS (cancelable srs/vsrs, remaining vsrs)")
	tl.Flush()
	op = newOp("CANCELING_REQUESTS")
	op.retGIS = NdFindActiveSRs
	op.vsrsToCancel = []*models.VolumeSeriesRequest{&models.VolumeSeriesRequest{}}
	op.vsrObjs = []*models.VolumeSeriesRequest{&models.VolumeSeriesRequest{}}
	op.srObjs = []*models.StorageRequest{&models.StorageRequest{}}
	expCalled = []string{"GIS", "LASR", "FSR", "LVsr", "CVsr", "WVsr", "LVsr", "FC"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	tl.Logger().Info("Case: CANCELING_REQUESTS (cancelable vsrs, error)")
	tl.Flush()
	op = newOp("CANCELING_REQUESTS")
	op.retGIS = NdSearchForCancelableVSRs
	op.vsrsToCancel = []*models.VolumeSeriesRequest{&models.VolumeSeriesRequest{}}
	op.retCVsrRetryLater = true
	expCalled = []string{"GIS", "LVsr", "CVsr"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)

	tl.Logger().Info("Case: CANCELING_REQUESTS (no cancelable vsrs, force cancel remaining vsrs)")
	tl.Flush()
	op = newOp("CANCELING_REQUESTS")
	op.retGIS = NdSearchForCancelableVSRs
	op.vsrObjs = []*models.VolumeSeriesRequest{&models.VolumeSeriesRequest{}}
	expCalled = []string{"GIS", "LVsr", "FC"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	tl.Logger().Info("Case: CANCELING_REQUESTS (no cancelable vsrs, force cancel remaining vsrs, error)")
	tl.Flush()
	op = newOp("CANCELING_REQUESTS")
	op.retGIS = NdSearchForCancelableVSRs
	op.vsrObjs = []*models.VolumeSeriesRequest{&models.VolumeSeriesRequest{}}
	op.retFCRetryLater = true
	expCalled = []string{"GIS", "LVsr", "FC"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)

	tl.Logger().Info("Case: DETACHING_VOLUMES (not primed)")
	tl.Flush()
	op = newOp("DETACHING_VOLUMES")
	op.retGIS = NdFindConfiguredVolumes
	op.valSkipSubordinateCheck = true
	op.valNumCreated = 1
	expCalled = []string{"GIS", "LCV", "PPM", "LRS"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: DETACHING_VOLUMES (primed, response pending)")
	tl.Flush()
	op = newOp("DETACHING_VOLUMES")
	op.rhs.Request.SyncPeers = map[string]models.SyncPeer{}
	op.retGIS = NdCheckForSubordinates
	expCalled = []string{"GIS", "CFS", "LRS", "ASR"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)
	assert.True(op.rhs.RetryLater)

	tl.Logger().Info("Case: DETACHING_VOLUMES (primed, all responded)")
	tl.Flush()
	op = newOp("DETACHING_VOLUMES")
	op.rhs.Request.SyncPeers = map[string]models.SyncPeer{}
	op.retGIS = NdCheckForSubordinates
	op.retASR = true
	expCalled = []string{"GIS", "CFS", "LRS", "ASR"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.RetryLater)

	tl.Logger().Info("Case: DETACHING_STORAGE (no attached storage)")
	tl.Flush()
	op = newOp("DETACHING_STORAGE")
	op.retGIS = NdFindAttachedStorage
	expCalled = []string{"GIS", "FAS", "FDSR", "CDSR"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: DETACHING_STORAGE (no new DETACH SRs)")
	tl.Flush()
	op = newOp("DETACHING_STORAGE")
	op.stgObjs = attachedStg
	op.detachSrObjs = activeSR
	op.retGIS = NdFindAttachedStorage
	expCalled = []string{"GIS", "FAS", "FDSR", "CDSR", "WFSR"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: DETACHING_STORAGE (new DETACH SRs)")
	tl.Flush()
	op = newOp("DETACHING_STORAGE")
	op.stgObjs = attachedStg
	op.detachSrObjs = activeSR
	op.retGIS = NdFindAttachedStorage
	expCalled = []string{"GIS", "FAS", "FDSR", "CDSR", "WFSR"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: DETACHING_STORAGE")
	tl.Flush()
	op = newOp("DETACHING_STORAGE")
	op.retGIS = NdDetachingStorageBreakout
	expCalled = []string{"GIS"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: VOLUME_DETACH_WAIT (no subordinates)")
	tl.Flush()
	op = newOp("VOLUME_DETACH_WAIT")
	op.retGIS = NdVolumeDetachWaitSync
	expCalled = []string{"GIS"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: VOLUME_DETACH_WAIT (has subordinates)")
	tl.Flush()
	op = newOp("VOLUME_DETACH_WAIT")
	op.retGIS = NdVolumeDetachWaitSync
	op.rhs.Request.SyncPeers = map[string]models.SyncPeer{"something": models.SyncPeer{}}
	expCalled = []string{"GIS", "SS-VOLUME_DETACH_WAIT"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: VOLUME_DETACH_WAIT (has subordinates, sync failure)")
	tl.Flush()
	op = newOp("VOLUME_DETACH_WAIT")
	op.retGIS = NdVolumeDetachWaitSync
	op.rhs.Request.SyncPeers = map[string]models.SyncPeer{"something": models.SyncPeer{}}
	op.syncInError = true
	expCalled = []string{"GIS", "SS-VOLUME_DETACH_WAIT", "SF-VOLUME_DETACH_WAIT"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	stashVal = op.rhs.StashGet(nodeDeleteStashKey{})
	assert.Nil(stashVal)

	tl.Logger().Info("Case: VOLUME_DETACHED (no subordinates)")
	tl.Flush()
	op = newOp("VOLUME_DETACHED")
	op.retGIS = NdVolumeDetachedWaitSync
	expCalled = []string{"GIS"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: VOLUME_DETACHED (has subordinates)")
	tl.Flush()
	op = newOp("VOLUME_DETACHED")
	op.rhs.Request.SyncPeers = map[string]models.SyncPeer{"something": models.SyncPeer{}}
	op.retGIS = NdVolumeDetachedWaitSync
	expCalled = []string{"GIS", "SS-VOLUME_DETACHED"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: VOLUME_DETACHED (has subordinates, sync failure)")
	tl.Flush()
	op = newOp("VOLUME_DETACHED")
	op.retGIS = NdVolumeDetachedWaitSync
	op.rhs.Request.SyncPeers = map[string]models.SyncPeer{"something": models.SyncPeer{}}
	op.syncInError = true
	expCalled = []string{"GIS", "SS-VOLUME_DETACHED", "SF-VOLUME_DETACHED"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: DELETING_NODE")
	tl.Flush()
	op = newOp("DELETING_NODE")
	op.retGIS = NdDeletingNode
	expCalled = []string{"GIS", "DN"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	// invoke the real handlers
	tl.Logger().Info("Invoke real handler")
	tl.Flush()
	rhs := &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsrObj)}
	rhs.Request.VolumeSeriesRequestState = "DRAINING_REQUESTS"
	rhs.Request.PlanOnly = swag.Bool(true) // no real changes
	fc.RetNObj = nObj
	fc.PassThroughNodeUpdateObj = true
	fc.RetLsVRErr = nil
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{Payload: []*models.VolumeSeriesRequest{}}
	fc.RetLsSRObj = &storage_request.StorageRequestListOK{Payload: []*models.StorageRequest{}}
	c.NodeDelete(nil, rhs)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	stashVal = rhs.StashGet(nodeDeleteStashKey{})
	assert.NotNil(stashVal)
	sOp, ok = stashVal.(*nodeDeleteOp)
	assert.True(ok)

	tl.Logger().Info("Invoke real handler with stashed op")
	rhs.Request.VolumeSeriesRequestState = "DELETING_NODE"
	assert.Equal(nObj, sOp.nObj)
	c.NodeDelete(nil, rhs)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	stashVal2 := rhs.StashGet(nodeDeleteStashKey{})
	assert.NotNil(stashVal2)
	sOp2, ok := stashVal2.(*nodeDeleteOp)
	assert.True(ok)
	assert.Equal(sOp, sOp2)

	// check state strings exist
	var ss nodeDeleteSubState
	for ss = NdLoadNode; ss < NdNoOp; ss++ {
		s := ss.String()
		tl.Logger().Debugf("Testing %d %s", ss, s)
		assert.Regexp("^Nd", s)
	}
	assert.Regexp("^ndSubState", ss.String())
}

type fakeNodeDeleteOp struct {
	nodeDeleteOp
	called                  []string
	retCVsrRetryLater       bool
	retGIS                  nodeDeleteSubState
	retFCRetryLater         bool
	retFN                   *models.Node
	retASR                  bool
	retWFSRErr              error
	retWVsrRetryLater       bool
	valNumCreated           int
	valSkipSubordinateCheck bool
	syncInError             bool
	retWaitSRs              []*models.StorageRequest
	retDetachSRs            []*models.StorageRequest
	retFailSRs              []*models.StorageRequest
}

func (op *fakeNodeDeleteOp) getInitialState(ctx context.Context) nodeDeleteSubState {
	op.called = append(op.called, "GIS")
	return op.retGIS
}

func (op *fakeNodeDeleteOp) allSubordinatesResponded() bool {
	op.called = append(op.called, "ASR")
	return op.retASR
}

func (op *fakeNodeDeleteOp) cancelVSRs(ctx context.Context) {
	op.called = append(op.called, "CVsr")
	op.rhs.RetryLater = op.retCVsrRetryLater
}

func (op *fakeNodeDeleteOp) checkForSubordinates(ctx context.Context) {
	op.called = append(op.called, "CFS")
}

func (op *fakeNodeDeleteOp) deleteNode(ctx context.Context) {
	op.called = append(op.called, "DN")
}

func (op *fakeNodeDeleteOp) failSRs(ctx context.Context, srs []*models.StorageRequest) []*models.StorageRequest {
	op.called = append(op.called, "FSR")
	return op.retFailSRs
}

func (op *fakeNodeDeleteOp) fetchNode(ctx context.Context) {
	op.called = append(op.called, "FN")
	op.nObj = op.retFN
}

func (op *fakeNodeDeleteOp) filterActiveSRs(ctx context.Context) {
}

func (op *fakeNodeDeleteOp) findAttachedStorage(ctx context.Context) {
	op.called = append(op.called, "FAS")
}

func (op *fakeNodeDeleteOp) findDetachSRs(ctx context.Context) {
	op.called = append(op.called, "FDSR")
}

func (op *fakeNodeDeleteOp) forceCancelRemainingVSRs(ctx context.Context) {
	op.called = append(op.called, "FC")
	op.rhs.RetryLater = op.retFCRetryLater
}

func (op *fakeNodeDeleteOp) launchDetachSRs(ctx context.Context) []*models.StorageRequest {
	op.called = append(op.called, "CDSR")
	return op.retDetachSRs
}

func (op *fakeNodeDeleteOp) launchRemainingSubordinates(ctx context.Context) {
	op.called = append(op.called, "LRS")
	op.numCreated = op.valNumCreated
}

func (op *fakeNodeDeleteOp) listActiveSRs(ctx context.Context) {
	op.called = append(op.called, "LASR")
}

func (op *fakeNodeDeleteOp) listAndClassifyNodeVSRs(ctx context.Context) {
	op.called = append(op.called, "LVsr")
}

func (op *fakeNodeDeleteOp) listConfiguredVS(ctx context.Context) {
	op.called = append(op.called, "LCV")
}

func (op *fakeNodeDeleteOp) primePeerMap(ctx context.Context) {
	op.called = append(op.called, "PPM")
	op.skipSubordinateCheck = op.valSkipSubordinateCheck
}

func (op *fakeNodeDeleteOp) syncFail(ctx context.Context) {
	label := fmt.Sprintf("SF-%s", op.rhs.Request.VolumeSeriesRequestState)
	op.called = append(op.called, label)
}

func (op *fakeNodeDeleteOp) syncState(ctx context.Context) {
	label := fmt.Sprintf("SS-%s", op.rhs.Request.VolumeSeriesRequestState)
	op.called = append(op.called, label)
	op.rhs.InError = op.syncInError
}

func (op *fakeNodeDeleteOp) teardownNode(ctx context.Context) {
	op.called = append(op.called, "TdN")
}

func (op *fakeNodeDeleteOp) waitForSRs(ctx context.Context, waitSRs []*models.StorageRequest) []*models.StorageRequest {
	op.called = append(op.called, "WFSR")
	return op.retWaitSRs
}

func (op *fakeNodeDeleteOp) waitForVSRs(ctx context.Context, vsrs []*models.VolumeSeriesRequest) []*models.VolumeSeriesRequest {
	op.called = append(op.called, "WVsr")
	op.rhs.RetryLater = op.retWVsrRetryLater
	return nil
}

func TestNodeDeleteSteps(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	sfc := &fakeSFC{}
	app.SFC = sfc
	fts := &fhk.TaskScheduler{}
	app.TaskScheduler = fts
	ctx := context.Background()
	c := &Component{}
	c.Init(app)
	assert.NotNil(c.Log)
	assert.NotNil(c.reservationCS)
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc

	// objects and helpers
	idN := "NODE-1"
	idVS0 := "VS-0"
	idVS1 := "VS-1"
	idSR0 := "SR-0"
	idSR1 := "SR-1"
	idS0 := "S-0"
	idS1 := "S-1"
	idNdVSR := "VSR-ND"

	maxTime := util.DateTimeMaxUpperBound()
	vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID(idNdVSR),
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			RequestedOperations: []string{"NODE_DELETE"},
			ClusterID:           "CLUSTER-1",
			CompleteByTime:      strfmt.DateTime(maxTime),
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID: models.ObjIDMutable(idN),
			},
		},
	}
	nObj := &models.Node{}
	nObj.Meta = &models.ObjMeta{ID: models.ObjID(idN)}
	nObj.State = common.NodeStateTimedOut

	configuredVS := []*models.VolumeSeries{
		&models.VolumeSeries{},
		&models.VolumeSeries{},
	}
	configuredVS[0].Meta = &models.ObjMeta{ID: models.ObjID(idVS0)}
	configuredVS[1].Meta = &models.ObjMeta{ID: models.ObjID(idVS1)}

	activeSRs := []*models.StorageRequest{
		&models.StorageRequest{},
		&models.StorageRequest{},
	}
	activeSRs[0].Meta = &models.ObjMeta{ID: models.ObjID(idSR0)}
	activeSRs[1].Meta = &models.ObjMeta{ID: models.ObjID(idSR1)}

	makeSyncPeerMap := func() map[string]models.SyncPeer {
		spm := make(map[string]models.SyncPeer)
		for _, vs := range configuredVS {
			spm[string(vs.Meta.ID)] = models.SyncPeer{}
		}
		spm[string(nObj.Meta.ID)] = models.SyncPeer{}
		return spm
	}

	attachedStg := []*models.Storage{
		&models.Storage{},
		&models.Storage{},
	}
	attachedStg[0].Meta = &models.ObjMeta{ID: models.ObjID(idS0)}
	attachedStg[0].StorageState = &models.StorageStateMutable{AttachedNodeID: models.ObjIDMutable(idN)}
	attachedStg[1].Meta = &models.ObjMeta{ID: models.ObjID(idS1)}
	attachedStg[1].StorageState = &models.StorageStateMutable{AttachedNodeID: models.ObjIDMutable(idN)}

	newOp := func(state string) *nodeDeleteOp {
		c.oCrud = fc
		c.Animator.OCrud = fc
		op := &nodeDeleteOp{}
		op.c = c
		op.ops = op
		op.rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsrObj)}
		op.rhs.Request.VolumeSeriesRequestState = state
		return op
	}
	var op *nodeDeleteOp

	// ***************************** getInitialState

	t.Log("case: getInitialState DRAINING_REQUESTS")
	op = newOp(common.VolReqStateDrainingRequests)
	assert.Equal(NdLoadNode, op.getInitialState(ctx))
	assert.False(op.planOnly)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.EqualValues(op.rhs.Request.Meta.ID, op.syncID)

	t.Log("case: getInitialState DRAINING_REQUESTS (planOnly)")
	op = newOp(common.VolReqStateDrainingRequests)
	op.rhs.Request.PlanOnly = swag.Bool(true)
	assert.Equal(NdLoadNode, op.getInitialState(ctx))
	assert.True(op.planOnly)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Empty(op.syncID)

	t.Log("case: getInitialState CANCELING_REQUESTS")
	op = newOp(common.VolReqStateCancelingRequests)
	assert.Equal(NdFindActiveSRs, op.getInitialState(ctx))
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	t.Log("case: getInitialState DETACHING_VOLUMES (not primed)")
	op = newOp(common.VolReqStateDetachingVolumes)
	assert.Equal(NdFindConfiguredVolumes, op.getInitialState(ctx))
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	t.Log("case: getInitialState DETACHING_VOLUMES (primed)")
	op = newOp(common.VolReqStateDetachingVolumes)
	op.rhs.Request.SyncPeers = makeSyncPeerMap()
	assert.Equal(NdCheckForSubordinates, op.getInitialState(ctx))
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	t.Log("case: getInitialState DETACHING_STORAGE")
	op = newOp(common.VolReqStateDetachingStorage)
	assert.Equal(NdFindAttachedStorage, op.getInitialState(ctx))
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	t.Log("case: getInitialState VOLUME_DETACH_WAIT")
	op = newOp(common.VolReqStateVolumeDetachWait)
	assert.Equal(NdVolumeDetachWaitSync, op.getInitialState(ctx))
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	t.Log("case: getInitialState VOLUME_DETACHED")
	op = newOp(common.VolReqStateVolumeDetached)
	assert.Equal(NdVolumeDetachedWaitSync, op.getInitialState(ctx))
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	t.Log("case: getInitialState DELETING_NODE")
	op = newOp(common.VolReqStateDeletingNode)
	assert.Equal(NdDeletingNode, op.getInitialState(ctx))
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	t.Log("case: getInitialState *other*")
	op = newOp("foo")
	assert.Equal(NdError, op.getInitialState(ctx))
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)

	// ***************************** allSubordinatesResponded

	t.Log("case: allSubordinatesResponded (planOnly)")
	op = newOp(common.VolReqStateDetachingVolumes)
	op.planOnly = true
	assert.True(op.allSubordinatesResponded())

	t.Log("case: allSubordinatesResponded (empty SyncPeers)")
	op = newOp(common.VolReqStateDetachingVolumes)
	assert.True(op.allSubordinatesResponded())

	t.Log("case: allSubordinatesResponded (SyncPeers has partial responses)")
	op = newOp(common.VolReqStateDetachingVolumes)
	op.rhs.Request.SyncPeers = makeSyncPeerMap()
	op.rhs.Request.SyncPeers[idVS0] = models.SyncPeer{ID: "vsr-vs0"}
	assert.False(op.allSubordinatesResponded())
	assert.Equal(models.SyncPeer{}, op.rhs.Request.SyncPeers[idN])

	t.Log("case: allSubordinatesResponded (SyncPeers has all responses)")
	op.rhs.Request.SyncPeers[idVS1] = models.SyncPeer{ID: "vsr-vs1"}
	assert.True(op.allSubordinatesResponded())
	assert.Equal(models.SyncPeer{}, op.rhs.Request.SyncPeers[idN])

	// ***************************** cancelVSRs

	fc = &fake.Client{}

	expCanceledVsrs := func(crVal bool) []*models.VolumeSeriesRequest {
		l := []*models.VolumeSeriesRequest{
			&models.VolumeSeriesRequest{},
		}
		l[0].Meta = &models.ObjMeta{ID: "VSR-TO-CANCEL"}
		l[0].CancelRequested = crVal
		return l
	}

	t.Log("case: cancelVSRs (cancel requested)")
	op = newOp(common.VolReqStateCancelingRequests)
	expectedCanceledVsrs := expCanceledVsrs(false)
	op.vsrsToCancel = expectedCanceledVsrs
	fc.RetVsrCancelErr = nil
	fc.InVsrCancelID = ""
	testutils.Clone(op.vsrsToCancel[0], &fc.RetVsrCancelObj)
	fc.RetVsrCancelObj.CancelRequested = true
	op.cancelVSRs(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(ctx, fc.InUVRCtx)
	assert.Equal(&crud.Updates{Append: []string{"requestMessages", "systemTags"}}, fc.InUVRItems)
	assert.EqualValues(expectedCanceledVsrs[0], fc.InUVRObj)
	assert.Equal(ctx, fc.InVsrCancelCtx)
	assert.NotEmpty(op.vsrsToCancel)
	assert.NotEmpty(fc.InVsrCancelID)
	assert.EqualValues(op.vsrsToCancel[0].Meta.ID, fc.InVsrCancelID)
	assert.Equal(expCanceledVsrs(true), op.vsrsToCancel)

	t.Log("case: cancelVSRs (cancel already requested)")
	fc.RetVsrCancelErr = fmt.Errorf("vsr-cancel-error")
	fc.InVsrCancelID = ""
	op = newOp(common.VolReqStateCancelingRequests)
	op.vsrsToCancel = expCanceledVsrs(true)
	op.cancelVSRs(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Empty(fc.InVsrCancelID)
	assert.NotEmpty(op.vsrsToCancel)
	assert.Equal(expCanceledVsrs(true), op.vsrsToCancel)

	t.Log("case: cancelVSRs (cancel request error)")
	op = newOp(common.VolReqStateCancelingRequests)
	expectedCanceledVsrs = expCanceledVsrs(false)
	op.vsrsToCancel = expectedCanceledVsrs
	op.cancelVSRs(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(ctx, fc.InVsrCancelCtx)
	assert.NotEmpty(op.vsrsToCancel)
	assert.NotEmpty(fc.InVsrCancelID)
	assert.EqualValues(op.vsrsToCancel[0].Meta.ID, fc.InVsrCancelID)
	assert.Equal(expectedCanceledVsrs, op.vsrsToCancel)

	t.Log("case: cancelVSRs (update request error, proceed to cancel)")
	fc.InVsrCancelID = "" // reset
	op = newOp(common.VolReqStateCancelingRequests)
	expectedCanceledVsrs = expCanceledVsrs(false)
	op.vsrsToCancel = expectedCanceledVsrs
	op.cancelVSRs(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(ctx, fc.InVsrCancelCtx)
	assert.NotEmpty(op.vsrsToCancel)
	assert.NotEmpty(fc.InVsrCancelID)
	assert.EqualValues(op.vsrsToCancel[0], fc.InUVRObj)
	assert.EqualValues(op.vsrsToCancel[0].Meta.ID, fc.InVsrCancelID)
	assert.Equal(expectedCanceledVsrs, op.vsrsToCancel)

	t.Log("case: cancelVSRs (planOnly)")
	op = newOp(common.VolReqStateCancelingRequests)
	op.planOnly = true
	op.vsrsToCancel = expCanceledVsrs(false)
	op.cancelVSRs(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	// ***************************** checkForSubordinates

	fc = &fake.Client{}

	t.Log("case: checkForSubordinates (success)")
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{
		Payload: []*models.VolumeSeriesRequest{&models.VolumeSeriesRequest{}},
	}
	fc.RetLsVRObj.Payload[0].Meta = &models.ObjMeta{ID: "vsr-vs0"}
	fc.RetLsVRObj.Payload[0].VolumeSeriesRequestState = "VOLUME_DETACH_WAIT"
	fc.RetLsVRObj.Payload[0].VolumeSeriesID = models.ObjIDMutable(idVS0)
	op = newOp(common.VolReqStateDetachingVolumes)
	op.rhs.Request.SyncPeers = makeSyncPeerMap()
	op.checkForSubordinates(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(ctx, fc.InLsVRCtx)
	vsrLP := volume_series_request.NewVolumeSeriesRequestListParams()
	vsrLP.SyncCoordinatorID = swag.String(idNdVSR)
	vsrLP.IsTerminated = swag.Bool(false)
	assert.Equal(vsrLP, fc.InLsVRObj)
	assert.Equal(models.ObjIDMutable("vsr-vs0"), op.rhs.Request.SyncPeers[idVS0].ID)
	assert.Equal("VOLUME_DETACH_WAIT", op.rhs.Request.SyncPeers[idVS0].State)
	assert.Equal(models.SyncPeer{}, op.rhs.Request.SyncPeers[idN])
	assert.Equal(models.SyncPeer{}, op.rhs.Request.SyncPeers[idVS1])

	t.Log("case: checkForSubordinates (list failure)")
	fc.RetLsVRObj = nil
	fc.RetLsVRErr = fmt.Errorf("vsr-list")
	op = newOp(common.VolReqStateDetachingVolumes)
	op.rhs.Request.SyncPeers = makeSyncPeerMap()
	op.checkForSubordinates(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	t.Log("case: checkForSubordinates (no SyncPeers)")
	op = newOp(common.VolReqStateDetachingVolumes)
	op.checkForSubordinates(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	// ***************************** deleteNode

	fc = &fake.Client{}

	t.Log("case: deleteNode (success)")
	op = newOp(common.VolReqStateDrainingRequests)
	op.deleteNode(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(ctx, fc.InNodeDeleteCtx)
	assert.EqualValues(op.rhs.Request.NodeID, fc.InNodeDeleteObj)

	t.Log("case: deleteNode (failure)")
	fc.RetNodeDeleteErr = fmt.Errorf("node-delete")
	op = newOp(common.VolReqStateDrainingRequests)
	op.deleteNode(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	t.Log("case: deleteNode (planOnly)")
	op = newOp(common.VolReqStateDrainingRequests)
	op.planOnly = true
	op.deleteNode(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	// ***************************** failSRs

	fc = &fake.Client{}
	activeSRs[0].RequestedOperations = []string{common.StgReqOpUse}
	activeSRs[0].StorageRequestState = common.StgReqStateUsing
	activeSRs[1].RequestedOperations = []string{common.StgReqOpFormat}
	activeSRs[1].StorageRequestState = common.StgReqStateFormatting
	activeSRs2 := []*models.StorageRequest{
		&models.StorageRequest{},
		&models.StorageRequest{},
	}
	testutils.Clone(activeSRs[0], &activeSRs2[0])
	testutils.Clone(activeSRs[1], &activeSRs2[1])

	t.Log("case: failSRs (success)")
	op = newOp(common.VolReqStateCancelingRequests)
	remainingSRs := op.failSRs(ctx, activeSRs2)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(&crud.Updates{Set: []string{"storageRequestState"}, Append: []string{"requestMessages", "systemTags"}}, fc.InUSRItems)
	assert.Equal(0, len(remainingSRs))
	for _, sr := range op.srsToFail {
		assert.Equal(common.StgReqStateFailed, sr.StorageRequestState)
	}

	t.Log("case: failSRs (failure)")
	fc.RetUSRErr = fmt.Errorf("sr-update-error")
	op = newOp(common.VolReqStateCancelingRequests)
	remainingSRs = op.failSRs(ctx, activeSRs2)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(2, len(remainingSRs))

	// ***************************** fetchNode

	fc = &fake.Client{}

	t.Log("case: fetchNode (success)")
	fc.RetNObj = nObj
	op = newOp(common.VolReqStateDrainingRequests)
	op.fetchNode(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(nObj, op.nObj)
	assert.Equal(ctx, fc.InNCtx)
	assert.EqualValues(op.rhs.Request.NodeID, fc.InNId)

	t.Log("case: fetchNode (already loaded)")
	fc.RetNErr = fmt.Errorf("node-failure")
	fc.RetNObj = nil
	op.fetchNode(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(nObj, op.nObj)

	t.Log("case: fetchNode (failure)")
	op = newOp(common.VolReqStateDrainingRequests)
	op.fetchNode(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Nil(op.nObj)

	// ***************************** filterActiveSRs

	t.Log("case: filterActiveSRs")
	activeSRs = []*models.StorageRequest{
		&models.StorageRequest{},
		&models.StorageRequest{},
		&models.StorageRequest{},
		&models.StorageRequest{},
		&models.StorageRequest{},
	}
	activeSRs[0].Meta = &models.ObjMeta{ID: models.ObjID("SR-WITH_USE_USING-AG-F")}
	activeSRs[0].RequestedOperations = []string{common.StgReqOpProvision, common.StgReqOpAttach, common.StgReqOpFormat, common.StgReqOpUse}
	activeSRs[0].StorageRequestState = common.StgReqStateUsing
	activeSRs[1].Meta = &models.ObjMeta{ID: models.ObjID("SR-WITH_FORMAT_FORMATTING-AG-F")}
	activeSRs[1].RequestedOperations = []string{common.StgReqOpFormat}
	activeSRs[1].StorageRequestState = common.StgReqStateFormatting
	activeSRs[2].Meta = &models.ObjMeta{ID: models.ObjID("SR-WITH_CLOSE_CLOSING-AG-F")}
	activeSRs[2].RequestedOperations = []string{common.StgReqOpClose, common.StgReqOpDetach, common.StgReqOpRelease}
	activeSRs[2].StorageRequestState = common.StgReqStateClosing
	activeSRs[3].Meta = &models.ObjMeta{ID: models.ObjID("SR-WITH_CLOSE_NEW-AG-F")}
	activeSRs[3].RequestedOperations = []string{common.StgReqOpClose}
	activeSRs[3].StorageRequestState = common.StgReqStateNew
	activeSRs[4].Meta = &models.ObjMeta{ID: models.ObjID("SR-WITH_PROVISION_NEW-NOT_AG-W")}
	activeSRs[4].RequestedOperations = []string{common.StgReqOpProvision}
	activeSRs[4].StorageRequestState = common.StgReqStateNew

	op = newOp(common.VolReqStateCancelingRequests)
	op.srObjs = activeSRs
	op.filterActiveSRs(ctx)
	assert.Equal(4, len(op.srsToFail))
	assert.NotNil(op.srsToFail)
	for _, sr := range op.srsToFail {
		assert.Regexp("SR-.*-AG-F$", string(sr.Meta.ID))
		assert.Error(op.CanWaitOnSR(sr), "ID:%s Op:%s St:%s", sr.Meta.ID, sr.RequestedOperations[0], sr.StorageRequestState)
	}
	assert.Equal(1, len(op.srsToWaitOn))
	assert.NotNil(op.srsToWaitOn)
	for _, sr := range op.srsToWaitOn {
		assert.Regexp("SR-.*-NOT_AG-W$", string(sr.Meta.ID))
		assert.NoError(op.CanWaitOnSR(sr), "ID:%s Op:%s St:%s", sr.Meta.ID, sr.RequestedOperations[0], sr.StorageRequestState)
	}

	// ***************************** findAttachedStorage

	fc = &fake.Client{}

	t.Log("case: findAttachedStorage (success)")
	fc.RetLsSOk = &storage.StorageListOK{Payload: attachedStg}
	op = newOp(common.VolReqStateDetachingStorage)
	op.findAttachedStorage(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(attachedStg, op.stgObjs)

	t.Log("case: findAttachedStorage (failure to list storage)")
	fc.RetLsSErr = fmt.Errorf("storage-list-failure")
	fc.RetLsSOk = nil
	op.findAttachedStorage(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(attachedStg, op.stgObjs)

	t.Log("case: findAttachedStorage (failure)")
	op = newOp(common.VolReqStateCancelingRequests)
	op.findAttachedStorage(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Nil(op.stgObjs)

	// ***************************** findDetachSRs

	fc = &fake.Client{}

	vsrCreatorTag := fmt.Sprintf("%s:%s", common.SystemTagVsrCreator, op.rhs.Request.Meta.ID)
	fdTag := fmt.Sprintf("%s:%s", common.SystemTagForceDetachNodeID, op.rhs.Request.NodeID)
	detachSR := []*models.StorageRequest{
		&models.StorageRequest{},
		&models.StorageRequest{},
	}
	detachSR[0].Meta = &models.ObjMeta{ID: models.ObjID(idSR0)}
	detachSR[0].RequestedOperations = []string{common.StgReqOpDetach}
	detachSR[0].NodeID = op.rhs.Request.NodeID
	detachSR[0].StorageID = models.ObjIDMutable(idS0)
	detachSR[0].SystemTags = []string{vsrCreatorTag, fdTag}
	detachSR[1].Meta = &models.ObjMeta{ID: models.ObjID(idSR1)}
	detachSR[1].RequestedOperations = []string{common.StgReqOpDetach}
	detachSR[1].NodeID = op.rhs.Request.NodeID
	detachSR[1].StorageID = models.ObjIDMutable(idS1)
	detachSR[1].SystemTags = []string{vsrCreatorTag, fdTag}

	t.Log("case: findDetachSRs (no new DETACH SRs to create)")
	fc.RetLsSRObj = &storage_request.StorageRequestListOK{Payload: detachSR}
	op = newOp(common.VolReqStateDetachingStorage)
	op.stgObjs = attachedStg
	op.findDetachSRs(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(len(op.stgIDsWithSR), len(op.detachSrObjs))

	t.Log("case: findDetachSRs (no active DETACH SRs, new SRs to be created)")
	fc.RetLsSRObj = &storage_request.StorageRequestListOK{}
	op = newOp(common.VolReqStateDetachingStorage)
	op.stgObjs = attachedStg
	op.findDetachSRs(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(0, len(op.stgIDsWithSR))
	assert.Equal(0, len(op.detachSrObjs))

	t.Log("case: findDetachSRs (failure to list SRs)")
	op = newOp(common.VolReqStateCancelingRequests)
	op.stgObjs = attachedStg
	fc.RetLsSRErr = fmt.Errorf("storage-request-list-failure")
	op.findDetachSRs(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(0, len(op.stgIDsWithSR))
	assert.Equal(0, len(op.detachSrObjs))

	// ***************************** forceCancelRemainingVSRs

	fc = &fake.Client{}

	remainingVsrs := func() []*models.VolumeSeriesRequest {
		l := []*models.VolumeSeriesRequest{
			&models.VolumeSeriesRequest{},
		}
		l[0].Meta = &models.ObjMeta{ID: "VSR-TO-FORCE-CANCEL"}
		return l
	}

	t.Log("case: forceCancelRemainingVSRs (success)")
	op = newOp(common.VolReqStateCancelingRequests)
	op.vsrObjs = remainingVsrs()
	op.forceCancelRemainingVSRs(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(ctx, fc.InUVRCtx)
	assert.Equal(&crud.Updates{Set: []string{"volumeSeriesRequestState"}, Append: []string{"requestMessages", "systemTags"}}, fc.InUVRItems)
	assert.EqualValues(op.vsrObjs[0], fc.InUVRObj)
	assert.Equal(common.VolReqStateCanceled, op.vsrObjs[0].VolumeSeriesRequestState)
	assert.EqualValues([]string{fmt.Sprintf("%s:%s", common.SystemTagVsrNodeDeleted, idN)}, op.vsrObjs[0].SystemTags)
	assert.NotEmpty(op.vsrObjs[0].RequestMessages)
	assert.Regexp("Canceled.*"+idN+".*deleted", op.vsrObjs[0].RequestMessages[0].Message)

	t.Log("case: forceCancelRemainingVSRs (failure)")
	fc.RetUVRErr = fmt.Errorf("vsr-update")
	op = newOp(common.VolReqStateCancelingRequests)
	op.vsrObjs = remainingVsrs()
	op.forceCancelRemainingVSRs(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	t.Log("case: forceCancelRemainingVSRs (planOnly)")
	op = newOp(common.VolReqStateCancelingRequests)
	op.planOnly = true
	op.vsrObjs = remainingVsrs()
	op.forceCancelRemainingVSRs(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	// ***************************** launchDetachSRs

	fc = &fake.Client{}

	t.Log("case: launchDetachSRs (success")
	detachSR[0].SystemTags = []string{}
	fc.RetLsSRObj = &storage_request.StorageRequestListOK{Payload: detachSR}
	fc.RetSRCObj = detachSR[0]
	op = newOp(common.VolReqStateDetachingStorage)
	op.stgObjs = attachedStg
	detachSRs := op.launchDetachSRs(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(len(attachedStg), len(detachSRs))

	t.Log("case: launchDetachSRs (failure to create new DETACH SRs)")
	detachSR[0].SystemTags = []string{}
	fc.RetLsSRObj = &storage_request.StorageRequestListOK{Payload: detachSR}
	fc.RetSRCErr = fmt.Errorf("storage-request-create-failure")
	op = newOp(common.VolReqStateDetachingStorage)
	op.stgObjs = attachedStg
	detachSRs = op.launchDetachSRs(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(0, len(detachSRs))

	// ***************************** launchRemainingSubordinates

	fc = &fake.Client{}

	t.Log("case: launchRemainingSubordinates (success)")
	fc.RetVRCObj = &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "vsrID",
			},
		},
	}
	op = newOp(common.VolReqStateDetachingVolumes)
	op.rhs.Request.SyncPeers = makeSyncPeerMap()
	op.rhs.Request.SyncPeers[idVS0] = models.SyncPeer{ID: "vsr-vs0"}
	op.launchRemainingSubordinates(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(1, op.numCreated)
	assert.Equal(ctx, fc.InVRCCtx)
	vsr := &models.VolumeSeriesRequest{}
	vsr.RequestedOperations = []string{common.VolReqOpVolDetach}
	vsr.NodeID = models.ObjIDMutable(idN)
	vsr.VolumeSeriesID = models.ObjIDMutable(idVS1)
	vsr.SyncCoordinatorID = models.ObjIDMutable(idNdVSR)
	vsr.CompleteByTime = vsrObj.CompleteByTime
	assert.Equal(vsr, fc.InVRCArgs)

	t.Log("case: launchRemainingSubordinates (failure)")
	fc.RetVRCErr = fmt.Errorf("vsr-create")
	op = newOp(common.VolReqStateDetachingVolumes)
	op.rhs.Request.SyncPeers = makeSyncPeerMap()
	op.rhs.Request.SyncPeers[idVS0] = models.SyncPeer{ID: "vsr-vs0"}
	op.launchRemainingSubordinates(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(0, op.numCreated)

	t.Log("case: launchRemainingSubordinates (planOnly)")
	fc.RetVRCErr = fmt.Errorf("vsr-create")
	op = newOp(common.VolReqStateDetachingVolumes)
	op.rhs.Request.SyncPeers = makeSyncPeerMap()
	op.rhs.Request.SyncPeers[idVS0] = models.SyncPeer{ID: "vsr-vs0"}
	op.planOnly = true
	op.launchRemainingSubordinates(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(0, op.numCreated)

	// ***************************** listActiveSRs

	fc = &fake.Client{}

	t.Log("case: listActiveSRs (success)")
	fc.RetLsSRObj = &storage_request.StorageRequestListOK{Payload: activeSRs}
	op = newOp(common.VolReqStateCancelingRequests)
	op.listActiveSRs(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(activeSRs, op.srObjs)

	t.Log("case: listActiveSRs (already loaded)")
	fc.RetLsSRErr = fmt.Errorf("storage-request-list-failure")
	fc.RetLsSRObj = nil
	op.listActiveSRs(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(activeSRs, op.srObjs)

	t.Log("case: listActiveSRs (failure)")
	op = newOp(common.VolReqStateCancelingRequests)
	op.listActiveSRs(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Nil(op.srObjs)

	// ***************************** listAndClassifyNodeVSRs

	nodeVsrData := []struct {
		firstOp   string
		state     string
		expAction string
	}{
		// in agentd
		{"UNMOUNT", "NEW", "F"},
		{"MOUNT", "VOLUME_CONFIG", "F"},
		{"DELETE", "UNDO_VOLUME_CONFIG", "F"},

		// not in agentd
		{"MOUNT", "NEW", "C"},
		{"MOUNT", "PLACEMENT", "C"},
		{"DELETE", "UNDO_PLACEMENT", "W"},
		{"UNBIND", "NEW", "W"},
		{"UNBIND", "UNDO_SIZING", "W"},
		{"DELETE", "NEW", "C"}, // starts in centrald
	}
	makeNodeVsrs := func() ([]*models.VolumeSeriesRequest, int, int, int) {
		ret := []*models.VolumeSeriesRequest{}
		cntA := 0
		cntN := 0
		numU := 0
		numC := 0
		for _, d := range nodeVsrData {
			si := vra.GetStateInfo(d.state)
			var id string
			if si.GetProcess(d.firstOp) == vra.ApAgentd {
				id = fmt.Sprintf("VSR-IN-AG-%s-%s-%d-%s", d.firstOp, d.state, cntA, d.expAction)
				cntA++
			} else {
				cntN++
				if si.IsUndo() || util.Contains([]string{"UNBIND"}, d.firstOp) {
					numU++
				} else {
					numC++
				}
				id = fmt.Sprintf("VSR-NOT-IN-AG-%s-%s-%d-%s", d.firstOp, d.state, cntN, d.expAction)
			}
			vsr := &models.VolumeSeriesRequest{}
			vsr.Meta = &models.ObjMeta{ID: models.ObjID(id)}
			vsr.RequestedOperations = []string{d.firstOp}
			vsr.VolumeSeriesRequestState = d.state
			vsr.NodeID = models.ObjIDMutable(idN)
			ret = append(ret, vsr)
		}
		return ret, cntA, numU, numC
	}

	fc = &fake.Client{}

	t.Log("case: listAndClassifyNodeVSRs (success)")
	nodeVsrs, numFail, numWait, numCancel := makeNodeVsrs()
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{Payload: nodeVsrs}
	expLsVR := volume_series_request.NewVolumeSeriesRequestListParams()
	expLsVR.NodeID = swag.String(idN)
	expLsVR.IsTerminated = swag.Bool(false)
	expLsVR.RequestedOperationsNot = []string{common.VolReqOpNodeDelete}
	op = newOp(common.VolReqStateDrainingRequests)
	op.listAndClassifyNodeVSRs(ctx)
	tl.Flush()
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(expLsVR, fc.InLsVRObj)
	assert.NotNil(op.vsrObjs)
	assert.Len(op.vsrObjs, len(nodeVsrs))
	assert.NotNil(op.vsrsToFail)
	assert.Len(op.vsrsToFail, numFail)
	for _, vsr := range op.vsrsToFail {
		assert.Regexp("VSR-IN-AG-.*-F$", string(vsr.Meta.ID))
		assert.Error(op.CanWaitOnVSR(vsr), "ID:%s Op:%s St:%s", vsr.Meta.ID, vsr.RequestedOperations[0], vsr.VolumeSeriesRequestState)
	}
	assert.NotNil(op.vsrsToWaitOn)
	assert.Len(op.vsrsToWaitOn, numWait)
	for _, vsr := range op.vsrsToWaitOn {
		assert.Regexp("VSR-NOT-IN-AG-.*-W$", string(vsr.Meta.ID))
		assert.NoError(op.CanWaitOnVSR(vsr), "ID:%s Op:%s St:%s", vsr.Meta.ID, vsr.RequestedOperations[0], vsr.VolumeSeriesRequestState)
	}
	assert.NotNil(op.vsrsToCancel)
	assert.Len(op.vsrsToCancel, numCancel)
	for _, vsr := range op.vsrsToCancel {
		assert.Regexp("VSR-NOT-IN-AG-.*-C$", string(vsr.Meta.ID))
		assert.NoError(op.CanWaitOnVSR(vsr), "ID:%s Op:%s St:%s", vsr.Meta.ID, vsr.RequestedOperations[0], vsr.VolumeSeriesRequestState)
	}

	t.Log("case: listAndClassifyNodeVSRs (failure)")
	fc.RetLsVRObj = nil
	fc.RetLsVRErr = fmt.Errorf("list-vsr-error")
	op = newOp(common.VolReqStateDrainingRequests)
	op.listAndClassifyNodeVSRs(ctx)
	tl.Flush()
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Nil(op.vsrObjs)
	assert.Nil(op.vsrsToFail)
	assert.Nil(op.vsrsToWaitOn)
	assert.Nil(op.vsrsToCancel)

	// ***************************** listConfiguredVS

	fc = &fake.Client{}

	t.Log("case: listConfiguredVS (success)")
	vsLP := volume_series.NewVolumeSeriesListParams()
	vsLP.ConfiguredNodeID = swag.String(string(op.rhs.Request.NodeID))
	fc.RetLsVOk = &volume_series.VolumeSeriesListOK{Payload: configuredVS}
	op = newOp(common.VolReqStateDetachingVolumes)
	op.listConfiguredVS(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(configuredVS, op.vsObjs)
	assert.Equal(ctx, fc.InLsVCtx)
	assert.Equal(vsLP, fc.InLsVObj)

	t.Log("case: listConfiguredVS (already loaded)")
	fc.RetLsVErr = fmt.Errorf("volume-list-failure")
	fc.RetLsVOk = nil
	op.listConfiguredVS(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(configuredVS, op.vsObjs)

	t.Log("case: listConfiguredVS (failure)")
	op = newOp(common.VolReqStateDetachingVolumes)
	op.listConfiguredVS(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Nil(op.vsObjs)

	// ***************************** primePeerMap

	fc = &fake.Client{}
	initialSyncPeerMap := makeSyncPeerMap()

	t.Log("case: primePeerMap (success)")
	op = newOp(common.VolReqStateDetachingVolumes)
	op.vsObjs = configuredVS
	op.primePeerMap(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.EqualValues(initialSyncPeerMap, op.rhs.Request.SyncPeers)
	assert.True(op.skipSubordinateCheck)
	assert.Equal(ctx, fc.InVSRUpdaterCtx)
	assert.EqualValues(op.rhs.Request.Meta.ID, fc.InVSRUpdaterID)
	assert.Equal(&crud.Updates{Set: []string{"syncPeers", "requestMessages"}}, fc.InVSRUpdaterItems)

	t.Log("case: primePeerMap (already loaded)")
	fc.RetVSRUpdaterErr = fmt.Errorf("vsr-updater-error")
	op.primePeerMap(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.EqualValues(initialSyncPeerMap, op.rhs.Request.SyncPeers)
	assert.False(op.skipSubordinateCheck)

	t.Log("case: primePeerMap (failure)")
	op = newOp(common.VolReqStateDetachingVolumes)
	op.vsObjs = configuredVS
	op.primePeerMap(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	// ***************************** syncFail

	t.Log("case: syncFail")
	op = newOp(common.VolReqStateVolumeDetachWait)
	assert.Empty(op.syncID) // to force return in op.rhs.SyncAbort
	op.syncFail(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.True(op.advertisedFailure)
	expSAA := &vra.SyncAbortArgs{
		LocalKey:   idN,
		LocalState: common.VolReqStateFailed,
	}
	assert.NoError(expSAA.Validate())
	assert.Equal(expSAA, op.saa)

	t.Log("case: syncFail (reentrant)")
	op.saa = nil
	op.syncFail(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Empty(op.saa)

	// ***************************** syncState

	t.Log("case: syncState (success)")
	op = newOp(common.VolReqStateVolumeDetachWait)
	assert.Empty(op.syncID) // to force return in op.rhs.SyncRequests with valid args
	op.syncState(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	expSA := &vra.SyncArgs{
		LocalKey:   idN,
		SyncState:  common.VolReqStateVolumeDetachWait,
		CompleteBy: maxTime,
	}
	assert.NoError(expSA.Validate())
	assert.Equal(expSA, op.sa)

	t.Log("case: syncState (failure)")
	op = newOp(common.VolReqStateVolumeDetachWait)
	assert.Empty(op.syncID)                                      // to force return in op.rhs.SyncRequests with valid args
	op.rhs.Request.CompleteByTime = strfmt.DateTime(time.Time{}) // invalid time
	op.syncState(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	expSA = &vra.SyncArgs{
		LocalKey:   idN,
		SyncState:  common.VolReqStateVolumeDetachWait,
		CompleteBy: time.Time{},
	}
	assert.Error(expSA.Validate())
	assert.Equal(expSA, op.sa)

	t.Log("case: syncState (planOnly)")
	op.planOnly = true
	op.sa = nil
	op.rhs.InError = false
	op.syncState(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Nil(op.sa)

	t.Log("case: syncState (block on rei)")
	c.rei.SetProperty("nd-block-sync-VOLUME_DETACH_WAIT", &rei.Property{BoolValue: true})
	op = newOp(common.VolReqStateVolumeDetachWait)
	op.syncState(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Nil(op.sa)

	// ***************************** teardownNode

	fc = &fake.Client{}

	t.Log("case: teardownNode (success)")
	fc.PassThroughNodeUpdateObj = true
	assert.Equal(common.NodeStateTimedOut, nObj.State)
	op = newOp(common.VolReqStateDrainingRequests)
	testutils.Clone(nObj, &op.nObj)
	op.teardownNode(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(ctx, fc.InNodeUpdateCtx)
	assert.EqualValues(op.nObj, fc.InNodeUpdateObj)
	assert.Equal(&crud.Updates{Set: []string{"state"}}, fc.InNodeUpdateItems)
	assert.NotEqual(nObj, op.nObj)
	assert.Equal(common.NodeStateTearDown, op.nObj.State)
	op.nObj.State = nObj.State
	assert.Equal(nObj, op.nObj)

	t.Log("case: teardownNode (failure)")
	fc.PassThroughNodeUpdateObj = false
	fc.RetNodeUpdateErr = fmt.Errorf("node-update-error")
	assert.Equal(common.NodeStateTimedOut, nObj.State)
	op = newOp(common.VolReqStateDrainingRequests)
	testutils.Clone(nObj, &op.nObj)
	op.teardownNode(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	t.Log("case: teardownNode (planOnly)")
	assert.Equal(common.NodeStateTimedOut, nObj.State)
	op = newOp(common.VolReqStateDrainingRequests)
	op.planOnly = true
	testutils.Clone(nObj, &op.nObj)
	op.teardownNode(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	t.Log("case: teardownNode (rei)")
	fc.PassThroughNodeUpdateObj = false
	fc.RetNodeUpdateErr = nil
	c.rei.SetProperty("teardown-fatal-error", &rei.Property{BoolValue: true})
	op = newOp(common.VolReqStateDrainingRequests)
	testutils.Clone(nObj, &op.nObj)
	op.teardownNode(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)

	// ***************************** waitForSRs
	fw := fVra.NewFakeRequestWaiter()
	rwfOrig := requestWaiterFactory
	defer func() {
		requestWaiterFactory = rwfOrig
	}()
	var srWA *vra.RequestWaiterArgs
	rwFakeFactory := func(args *vra.RequestWaiterArgs) vra.RequestWaiter {
		srWA = args
		return fw
	}
	requestWaiterFactory = rwFakeFactory

	t.Log("case: waitForSRs (success)")
	op = newOp(common.VolReqStateCancelingRequests)
	remainingSRs = op.waitForSRs(nil, activeSRs)
	assert.NotNil(srWA)
	assert.NoError(srWA.Validate())
	assert.Equal(op.c.App.CrudeOps, srWA.CrudeOps)
	assert.Equal(op.c.oCrud, srWA.ClientOps)
	assert.Equal(op.c.Log, srWA.Log)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(0, len(remainingSRs))

	t.Log("case: waitForSRs (failure)")
	fw.Error = fmt.Errorf("wait-failure")
	op = newOp(common.VolReqStateCancelingRequests)
	remainingSRs = op.waitForSRs(ctx, activeSRs)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(len(activeSRs), len(remainingSRs))

	// ***************************** waitForVSRs

	fc = &fake.Client{}
	fw = fVra.NewFakeRequestWaiter()
	defer func() {
		requestWaiterFactory = rwfOrig
	}()
	var vsrWA *vra.RequestWaiterArgs
	rwFakeFactory = func(args *vra.RequestWaiterArgs) vra.RequestWaiter {
		vsrWA = args
		return fw
	}
	requestWaiterFactory = rwFakeFactory

	t.Log("case: waitForVSRs (success)")
	op = newOp(common.VolReqStateDrainingRequests)
	activeVSRs := []*models.VolumeSeriesRequest{
		&models.VolumeSeriesRequest{},
	}
	activeVSRs[0].Meta = &models.ObjMeta{ID: "ACTIVE-VSR"}
	remVSRs := op.waitForVSRs(ctx, activeVSRs)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Empty(remVSRs)
	assert.NotNil(vsrWA)
	assert.NoError(vsrWA.Validate())
	assert.Equal(activeVSRs[0], vsrWA.VSR)
	assert.Equal(op.c.App.CrudeOps, vsrWA.CrudeOps)
	assert.Equal(op.c.oCrud, vsrWA.ClientOps)
	assert.Equal(op.c.Log, vsrWA.Log)
	assert.NotNil(vsrWA.VsrInspector)

	t.Log("case: waitForVSRs (fail in wait)")
	op = newOp(common.VolReqStateDrainingRequests)
	fw.Error = fmt.Errorf("wait-failure")
	remVSRs = op.waitForVSRs(ctx, activeVSRs)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.NotEmpty(remVSRs)
	assert.Equal(activeVSRs, remVSRs)
}
