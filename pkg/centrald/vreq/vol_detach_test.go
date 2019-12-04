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
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestVolDetach(t *testing.T) {
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
			Meta: &models.ObjMeta{ID: "VD-1"},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			RequestedOperations: []string{"VOL_DETACH"},
			CompleteByTime:      strfmt.DateTime(maxTime),
			SyncCoordinatorID:   "VSR-ND-1",
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID:         "NODE-1",
				VolumeSeriesID: "VS-1",
			},
		},
	}

	newOp := func(state string) *fakeVolDetachOp {
		op := &fakeVolDetachOp{}
		op.c = c
		op.ops = op
		op.rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsrObj)}
		op.rhs.Request.VolumeSeriesRequestState = state
		return op
	}
	var expCalled []string
	var op *fakeVolDetachOp

	tl.Logger().Info("Case: VOLUME_DETACH_WAIT")
	tl.Flush()
	op = newOp("VOLUME_DETACH_WAIT")
	op.retGIS = VdStartSync
	expCalled = []string{"GIS", "SS-VOLUME_DETACH_WAIT"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	stashVal := op.rhs.StashGet(volDetachStashKey{})
	assert.NotNil(stashVal)
	sOp, ok := stashVal.(*volDetachOp)
	assert.True(ok)

	tl.Logger().Info("Case: VOLUME_DETACH_WAIT (sync error)")
	tl.Flush()
	op = newOp("VOLUME_DETACH_WAIT")
	op.retGIS = VdStartSync
	op.syncInError = true
	expCalled = []string{"GIS", "SS-VOLUME_DETACH_WAIT", "SF-VOLUME_DETACH_WAIT"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	tl.Logger().Info("Case: VOLUME_DETACHING")
	tl.Flush()
	op = newOp("VOLUME_DETACHING")
	op.retGIS = VdDetachVolume
	expCalled = []string{"GIS", "DV"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	tl.Logger().Info("Case: VOLUME_DETACHED")
	tl.Flush()
	op = newOp("VOLUME_DETACHED")
	op.retGIS = VdEndSync
	expCalled = []string{"GIS", "SS-VOLUME_DETACHED"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	tl.Logger().Info("Case: VOLUME_DETACHED (sync error)")
	tl.Flush()
	op = newOp("VOLUME_DETACHED")
	op.retGIS = VdEndSync
	op.syncInError = true
	expCalled = []string{"GIS", "SS-VOLUME_DETACHED", "SF-VOLUME_DETACHED"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	tl.Logger().Info("Invoke real handler")
	tl.Flush()
	rhs := &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsrObj)}
	rhs.Request.VolumeSeriesRequestState = "VOLUME_DETACH_WAIT"
	rhs.Request.PlanOnly = swag.Bool(true) // no real changes
	c.VolDetach(nil, rhs)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	stashVal = rhs.StashGet(volDetachStashKey{})
	assert.NotNil(stashVal)
	sOp, ok = stashVal.(*volDetachOp)
	assert.True(ok)

	tl.Logger().Info("Invoke real handler with stashed op")
	rhs.Request.VolumeSeriesRequestState = "VOLUME_DETACHING"
	c.VolDetach(nil, rhs)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	stashVal2 := rhs.StashGet(volDetachStashKey{})
	assert.NotNil(stashVal2)
	sOp2, ok := stashVal2.(*volDetachOp)
	assert.True(ok)
	assert.Equal(sOp, sOp2)

	// check state strings exist
	var ss volDetachSubState
	for ss = VdStartSync; ss < VdNoOp; ss++ {
		s := ss.String()
		tl.Logger().Debugf("Testing %d %s", ss, s)
		assert.Regexp("^Vd", s)
	}
	assert.Regexp("^vdSubState", ss.String())
}

type fakeVolDetachOp struct {
	volDetachOp
	called      []string
	retGIS      volDetachSubState
	syncInError bool
}

func (op *fakeVolDetachOp) getInitialState(ctx context.Context) volDetachSubState {
	op.called = append(op.called, "GIS")
	return op.retGIS
}

func (op *fakeVolDetachOp) detachVolume(ctx context.Context) {
	op.called = append(op.called, "DV")
	return
}

func (op *fakeVolDetachOp) syncFail(ctx context.Context) {
	label := fmt.Sprintf("SF-%s", op.rhs.Request.VolumeSeriesRequestState)
	op.called = append(op.called, label)
}

func (op *fakeVolDetachOp) syncState(ctx context.Context) {
	label := fmt.Sprintf("SS-%s", op.rhs.Request.VolumeSeriesRequestState)
	op.called = append(op.called, label)
	op.rhs.InError = op.syncInError
}

func TestVolDetachSteps(t *testing.T) {
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

	idVS := "VS-1"
	idN := "NODE-1"

	maxTime := util.DateTimeMaxUpperBound()
	vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{ID: "VD-1"},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			RequestedOperations: []string{"VOL_DETACH"},
			CompleteByTime:      strfmt.DateTime(maxTime),
			SyncCoordinatorID:   "VSR-ND-1",
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID:         models.ObjIDMutable(idN),
				VolumeSeriesID: "VS-1",
			},
		},
	}
	otherMount := &models.Mount{MountedNodeID: "otherNode", SnapIdentifier: "snap-2"}
	vsObj := &models.VolumeSeries{}
	vsObj.Meta = &models.ObjMeta{ID: models.ObjID(idVS)}
	vsObj.VolumeSeriesState = common.VolStateInUse
	vsObj.ConfiguredNodeID = models.ObjIDMutable(idN)
	vsObj.Mounts = []*models.Mount{
		&models.Mount{MountedNodeID: models.ObjIDMutable(idN), SnapIdentifier: common.VolMountHeadIdentifier},
		&models.Mount{MountedNodeID: models.ObjIDMutable(idN), SnapIdentifier: "snap-1"},
		otherMount,
	}
	vsObj.CacheAllocations = map[string]models.CacheAllocation{
		idN: models.CacheAllocation{},
	}
	vsObj.SystemTags = []string{
		"other:value",
		fmt.Sprintf("%s:X", common.SystemTagVolumeHeadStatSeries),
		fmt.Sprintf("%s:Y", common.SystemTagVolumeHeadStatCount),
		fmt.Sprintf("%s:Z", common.SystemTagVolumeFsAttached),
	}

	newOp := func(state string) *volDetachOp {
		c.oCrud = fc
		c.Animator.OCrud = fc
		op := &volDetachOp{}
		op.c = c
		op.ops = op
		op.rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsrObj), VolumeSeries: vsClone(vsObj)}
		op.rhs.Request.VolumeSeriesRequestState = state
		return op
	}
	var op *volDetachOp

	// ***************************** getInitialState

	t.Log("case: getInitialState VOLUME_DETACH_WAIT")
	op = newOp(common.VolReqStateVolumeDetachWait)
	assert.Equal(VdStartSync, op.getInitialState(ctx))
	assert.False(op.planOnly)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	t.Log("case: getInitialState VOLUME_DETACH_WAIT (planOnly)")
	op = newOp(common.VolReqStateVolumeDetachWait)
	op.rhs.Request.PlanOnly = swag.Bool(true)
	assert.Equal(VdStartSync, op.getInitialState(ctx))
	assert.True(op.planOnly)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	t.Log("case: getInitialState VOLUME_DETACHING")
	op = newOp(common.VolReqStateVolumeDetaching)
	assert.Equal(VdDetachVolume, op.getInitialState(ctx))
	assert.False(op.planOnly)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	t.Log("case: getInitialState VOLUME_DETACHED")
	op = newOp(common.VolReqStateVolumeDetached)
	assert.Equal(VdEndSync, op.getInitialState(ctx))
	assert.False(op.planOnly)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	t.Log("case: getInitialState *other*")
	op = newOp("foo")
	assert.Equal(VdError, op.getInitialState(ctx))
	assert.False(op.planOnly)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)

	// ***************************** detachVolume

	t.Log("case: detachVolume (success)")
	fc = &fake.Client{}
	op = newOp(common.VolReqStateVolumeDetaching)
	op.detachVolume(ctx)
	tl.Flush()
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(ctx, fc.InVSUpdaterCtx)
	assert.EqualValues(idVS, fc.InVSUpdaterID)
	assert.Equal(&crud.Updates{Set: []string{"cacheAllocations", "configuredNodeId", "messages", "mounts", "systemTags", "volumeSeriesState"}}, fc.InVSUpdaterItems)
	assert.NotNil(op.rhs.VolumeSeries)
	assert.NotEqual(vsObj, op.rhs.VolumeSeries)
	vs := op.rhs.VolumeSeries
	assert.Equal(common.VolStateProvisioned, vs.VolumeSeriesState)
	assert.EqualValues("", vs.ConfiguredNodeID)
	assert.NotNil(vs.Mounts)
	assert.Len(vs.Mounts, 1)
	assert.Equal(otherMount, vs.Mounts[0])
	assert.NotNil(vs.CacheAllocations)
	assert.Empty(vs.CacheAllocations)
	sTags := util.NewTagList(vs.SystemTags)
	_, found := sTags.Get(common.SystemTagVolumeLastHeadUnexport)
	_, found = sTags.Get("other")
	assert.True(found)
	for _, tag := range []string{common.SystemTagVolumeHeadStatSeries, common.SystemTagVolumeHeadStatCount, common.SystemTagVolumeFsAttached} {
		_, found = sTags.Get(tag)
		assert.False(found)
	}
	assert.Len(vs.Messages, 1)
	assert.Regexp("State change.*PROVISIONED", vs.Messages[0].Message)

	t.Log("case: detachVolume (no change needed)")
	op = newOp(common.VolReqStateVolumeDetaching)
	vs.CacheAllocations = nil
	vs.Messages = nil
	op.rhs.VolumeSeries = vs // from previous operation
	op.detachVolume(ctx)
	tl.Flush()
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	vs = op.rhs.VolumeSeries
	assert.Len(vs.Messages, 0)

	t.Log("case: detachVolume (error)")
	fc.RetVSUpdaterErr = fmt.Errorf("update-error")
	op = newOp(common.VolReqStateVolumeDetaching)
	op.detachVolume(ctx)
	tl.Flush()
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	t.Log("case: detachVolume (planOnly)")
	fc.RetVSUpdaterErr = fmt.Errorf("update-error")
	op = newOp(common.VolReqStateVolumeDetaching)
	op.planOnly = true
	op.detachVolume(ctx)
	tl.Flush()
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	// ***************************** syncFail

	t.Log("case: syncFail")
	op = newOp(common.VolReqStateVolumeDetachWait)
	op.rhs.Request.SyncCoordinatorID = "" // force failure
	op.syncFail(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.True(op.advertisedFailure)
	expSAA := &vra.SyncAbortArgs{
		LocalKey:   idVS,
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
	op = newOp(common.VolReqStateVolumeDetached)
	op.rhs.Request.SyncCoordinatorID = "" // force failure
	op.syncState(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	expSA := &vra.SyncArgs{
		LocalKey:   idVS,
		SyncState:  common.VolReqStateVolumeDetached,
		CompleteBy: maxTime,
	}
	assert.NoError(expSA.Validate())
	assert.Equal(expSA, op.sa)

	t.Log("case: syncState (failure)")
	op = newOp(common.VolReqStateVolumeDetached)
	op.rhs.Request.SyncCoordinatorID = ""                        // force failure
	op.rhs.Request.CompleteByTime = strfmt.DateTime(time.Time{}) // invalid time
	op.syncState(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	expSA = &vra.SyncArgs{
		LocalKey:   idVS,
		SyncState:  common.VolReqStateVolumeDetached,
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
	c.rei.SetProperty("vd-block-sync-VOLUME_DETACHED", &rei.Property{BoolValue: true})
	op = newOp(common.VolReqStateVolumeDetached)
	op.syncState(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Nil(op.sa)
}
