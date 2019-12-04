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
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	appServant "github.com/Nuvoloso/kontroller/pkg/agentd/fake"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/snapshot"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/csp/aws"
	mockcsp "github.com/Nuvoloso/kontroller/pkg/csp/mock"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	fakeMount "github.com/Nuvoloso/kontroller/pkg/mount/fake"
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	mockNuvo "github.com/Nuvoloso/kontroller/pkg/nuvoapi/mock"
	"github.com/Nuvoloso/kontroller/pkg/pstore"
	fps "github.com/Nuvoloso/kontroller/pkg/pstore/fake"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestVolSnapshotCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM

	c := newComponent()
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc
	ctx := context.Background()

	pitUUID := "8e0e60e2-063f-4a29-ba79-e380ab08e75d"
	vsr := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VR-1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			ClusterID:         "CL-1",
			SyncCoordinatorID: "sync-1",
			SnapIdentifier:    "Snap-1",
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				VolumeSeriesID: "VS-1",
				Snapshot: &models.SnapshotData{
					PitIdentifier: pitUUID,
					SizeBytes:     swag.Int64(100000),
				},
			},
		},
	}
	vsObj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: "VS-1",
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				VolumeSeriesState: "CONFIGURED",
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				ConsistencyGroupID: "CG-1",
			},
		},
	}

	newFakeVolSnapCreateOp := func(state string) *fakeVolSCOps {
		op := &fakeVolSCOps{}
		op.c = c
		op.ops = op
		op.rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr), VolumeSeries: vsClone(vsObj)}
		op.rhs.Request.VolumeSeriesRequestState = state
		return op
	}

	var op *fakeVolSCOps
	var expCalled []string

	tl.Logger().Info("Case: CG race startup")
	tl.Flush()
	op = newFakeVolSnapCreateOp("PAUSING_IO")
	op.retCgRACEInError = true
	expCalled = []string{"CgRACE"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: Creating PiT (not present)")
	tl.Flush()
	op = newFakeVolSnapCreateOp("CREATING_PIT")
	op.retGIS = VolSCTimestampPiT
	expCalled = []string{"GIS", "TSP", "CP", "GWS"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: Creating PiT (not present, error)")
	tl.Flush()
	op = newFakeVolSnapCreateOp("CREATING_PIT")
	op.retGIS = VolSCCreatePiT
	op.retCPInError = true
	expCalled = []string{"GIS", "CP", "sFAIL"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: Creating PiT (present, not recorded)")
	tl.Flush()
	op = newFakeVolSnapCreateOp("CREATING_PIT")
	op.retGIS = VolSCGetWriteStats
	op.retLP = []string{pitUUID}
	expCalled = []string{"GIS", "GWS"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: Created PiT")
	tl.Flush()
	op = newFakeVolSnapCreateOp("CREATED_PIT")
	op.retGIS = VolSCCreatedPitSync
	expCalled = []string{"GIS", "CSYNC-F", "RIO", "UFZ"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: Pausing IO")
	tl.Flush()
	op = newFakeVolSnapCreateOp("PAUSING_IO")
	op.retGIS = VolSCListPiTs
	expCalled = []string{"CgRACE", "GIS", "LP", "LVS", "PIO", "FZ"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: Paused IO")
	tl.Flush()
	op = newFakeVolSnapCreateOp("PAUSED_IO")
	op.retGIS = VolSCPausedIOSync
	expCalled = []string{"GIS", "PSYNC-F"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: Snapshot uploading")
	tl.Flush()
	op = newFakeVolSnapCreateOp("SNAPSHOT_UPLOADING")
	op.retGIS = VolSCSnapshotUploadPlan
	op.inPCBasePiT = "basePiT"
	op.inPCBaseSnapID = "baseSnapID"
	expCalled = []string{"GIS", "LP", "LVS", "PC", "ET", "PsUP"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: Snapshot uploading (exported)")
	tl.Flush()
	op = newFakeVolSnapCreateOp("SNAPSHOT_UPLOADING")
	op.retGIS = VolSCSnapshotUploadPlan
	op.inPCTipExported = true
	op.inPCBasePiT = "basePiT"
	op.inPCBaseSnapID = "baseSnapID"
	expCalled = []string{"GIS", "LP", "LVS", "PC", "PsUP"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: Snapshot uploading (no base)")
	tl.Flush()
	op = newFakeVolSnapCreateOp("SNAPSHOT_UPLOADING")
	op.retGIS = VolSCSnapshotUploadPlan
	op.inPCBaseSnapID = "baseSnapID"
	expCalled = []string{"GIS", "LP", "LVS", "PC", "ET", "PsUP"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: Snapshot uploading (no base, fail)")
	tl.Flush()
	op = newFakeVolSnapCreateOp("SNAPSHOT_UPLOADING")
	op.retGIS = VolSCSnapshotUploadPlan
	op.inPCBaseSnapID = "baseSnapID"
	op.retPsUPInError = true
	expCalled = []string{"GIS", "LP", "LVS", "PC", "ET", "PsUP", "sFAIL"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: Snapshot uploaded")
	tl.Flush()
	op = newFakeVolSnapCreateOp("SNAPSHOT_UPLOAD_DONE")
	op.retGIS = VolSCSnapshotUploadSync
	expCalled = []string{"GIS", "USYNC-T"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: Finalize (reload pits, plan)")
	tl.Flush()
	op = newFakeVolSnapCreateOp("FINALIZING_SNAPSHOT")
	op.retGIS = VolSCFinalizeStart
	op.retDPInError = true
	expCalled = []string{"GIS", "LP", "LVS", "FSP", "RmOP", "UT", "CSO", "SGL", "FSiV", "SGU", "FSYNC-T"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.InError)

	tl.Logger().Info("Case: Finalize (pits, plan present)")
	tl.Flush()
	op = newFakeVolSnapCreateOp("FINALIZING_SNAPSHOT")
	op.retGIS = VolSCFinalizeStart
	op.pits = []string{pitUUID}
	op.tipSnapID = op.rhs.Request.SnapIdentifier
	expCalled = []string{"GIS", "LVS", "FSP", "RmOP", "UT", "CSO", "SGL", "FSiV", "SGU", "FSYNC-T"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: Finalize (retry on FSiV releases guard on the way out)")
	tl.Flush()
	op = newFakeVolSnapCreateOp("FINALIZING_SNAPSHOT")
	op.retGIS = VolSCFinalizeStart
	op.pits = []string{pitUUID}
	op.tipSnapID = op.rhs.Request.SnapIdentifier
	op.retFSiVRetryLater = true
	op.lmdCST = &util.CriticalSectionTicket{}
	expCalled = []string{"GIS", "LVS", "FSP", "RmOP", "UT", "CSO", "SGL", "FSiV", "SGU"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: Undo snapshot uploading (pits not loaded)")
	tl.Flush()
	op = newFakeVolSnapCreateOp("UNDO_SNAPSHOT_UPLOADING")
	op.retGIS = VolSCUndoSnapshotUploadStart
	op.retLP = []string{pitUUID}
	op.inPCTipExported = true
	expCalled = []string{"GIS", "LP", "PC", "PsDEL", "UT"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: Undo snapshot uploading (pits loaded, snap not exported)")
	tl.Flush()
	op = newFakeVolSnapCreateOp("UNDO_SNAPSHOT_UPLOADING")
	op.retGIS = VolSCUndoSnapshotUploadStart
	op.pits = []string{pitUUID}
	expCalled = []string{"GIS", "PC", "PsDEL"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: Undo creating PiT (pit present, recorded)")
	tl.Flush()
	op = newFakeVolSnapCreateOp("UNDO_CREATING_PIT")
	op.retGIS = VolSCUndoCreatingPitStart
	op.retLP = []string{pitUUID}
	expCalled = []string{"GIS", "LP", "DP"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: Undo creating PiT (no pit, recorded)")
	tl.Flush()
	op = newFakeVolSnapCreateOp("UNDO_CREATING_PIT")
	op.retGIS = VolSCUndoCreatingPitStart
	expCalled = []string{"GIS", "LP"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: Undo creating PiT (no pit, not recorded)")
	tl.Flush()
	op = newFakeVolSnapCreateOp("UNDO_CREATING_PIT")
	op.retGIS = VolSCUndoCreatingPitStart
	expCalled = []string{"GIS", "LP"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: Undo pausing IO")
	tl.Flush()
	op = newFakeVolSnapCreateOp("UNDO_PAUSING_IO")
	op.retGIS = VolSCUndoPausingIO
	expCalled = []string{"GIS", "RIO", "UFZ"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	// invoke the real handlers

	tl.Logger().Info("Case: real handler, no history")
	tl.Flush()
	fc.PassThroughUVRObj = true
	rhs := &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr), VolumeSeries: vsClone(vsObj)}
	c.VolSnapshotCreate(nil, rhs)
	assert.False(rhs.InError)

	tl.Logger().Info("Case: real handler, with history")
	tl.Flush()
	rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr), VolumeSeries: vsClone(vsObj)}
	rhs.StashSet(volSnapCreateStashKey{}, &volSnapCreateOp{})
	c.VolSnapshotCreate(nil, rhs)
	assert.False(rhs.InError)

	tl.Logger().Info("Case: real undo handler, no history")
	tl.Flush()
	rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}
	rhs.Request.PlanOnly = swag.Bool(true)
	rhs.Request.VolumeSeriesRequestState = "UNDO_SNAPSHOT_UPLOAD_DONE"
	rhs.InError = true
	c.UndoVolSnapshotCreate(nil, rhs)
	assert.True(rhs.InError)

	// check state strings exist up to VolSCError
	var ss volSnapCreateSubState
	for ss = VolSCPausingIO; ss < VolSCNoOp; ss++ {
		s := ss.String()
		tl.Logger().Debugf("Testing %d %s", ss, s)
		assert.Regexp("^VolSC", s)
	}
	assert.Regexp("^volSnapCreateSubState", ss.String())
}

type fakeVolSCOps struct {
	volSnapCreateOp
	called            []string
	inDPPiTs          []string
	inPCBasePiT       string
	inPCBaseSnapID    string
	inPCTipExported   bool
	retAC             int
	retCgRACEInError  bool
	retDPInError      bool
	retGIS            volSnapCreateSubState
	retLP             []string
	retNC             int
	retCPInError      bool
	retPsUPInError    bool
	retFSiVRetryLater bool
	retVSSnapshots    []*models.Snapshot
}

func (op *fakeVolSCOps) exportTip(ctx context.Context) {
	op.called = append(op.called, "ET")
}
func (op *fakeVolSCOps) fetchServicePlan(ctx context.Context) {
	op.called = append(op.called, "FSP")
}
func (op *fakeVolSCOps) finalizeSnapshotDataInVolume(ctx context.Context) {
	op.called = append(op.called, "FSiV")
	op.rhs.RetryLater = op.retFSiVRetryLater
}
func (op *fakeVolSCOps) fsFreeze(ctx context.Context) {
	op.called = append(op.called, "FZ")
}
func (op *fakeVolSCOps) fsUnfreeze(ctx context.Context) {
	op.called = append(op.called, "UFZ")
}
func (op *fakeVolSCOps) getInitialState(ctx context.Context) volSnapCreateSubState {
	op.called = append(op.called, "GIS")
	return op.retGIS
}
func (op *fakeVolSCOps) handleCGStartupRace(ctx context.Context) {
	op.called = append(op.called, "CgRACE")
	op.rhs.InError = op.retCgRACEInError
	op.inCgRace = op.retCgRACEInError
}
func (op *fakeVolSCOps) listVolumeSnapshots(ctx context.Context) {
	op.called = append(op.called, "LVS")
	op.vsSnapshots = op.retVSSnapshots
}
func (op *fakeVolSCOps) nuvoCreatePiT(ctx context.Context) {
	op.called = append(op.called, "CP")
	op.rhs.InError = op.retCPInError
}
func (op *fakeVolSCOps) nuvoDeletePiT(ctx context.Context, pitUUID string) {
	op.called = append(op.called, "DP")
	op.inDPPiTs = append(op.inDPPiTs, pitUUID)
	op.rhs.InError = op.retDPInError
}
func (op *fakeVolSCOps) nuvoGetWriteStat(ctx context.Context) {
	op.called = append(op.called, "GWS")
}
func (op *fakeVolSCOps) nuvoListPiTs(ctx context.Context) {
	op.called = append(op.called, "LP")
	op.pits = op.retLP
}
func (op *fakeVolSCOps) nuvoPauseIO(ctx context.Context) {
	op.called = append(op.called, "PIO")
}
func (op *fakeVolSCOps) nuvoResumeIO(ctx context.Context) {
	op.called = append(op.called, "RIO")
}
func (op *fakeVolSCOps) planCopy(ctx context.Context) {
	op.called = append(op.called, "PC")
	op.tipPiT = op.rhs.Request.Snapshot.PitIdentifier
	op.tipSnapID = op.rhs.Request.SnapIdentifier
	op.tipExported = op.inPCTipExported
	op.basePiT = op.inPCBasePiT
	op.baseSnapIdentifier = op.inPCBaseSnapID
}
func (op *fakeVolSCOps) psDelete(ctx context.Context) {
	op.called = append(op.called, "PsDEL")
}
func (op *fakeVolSCOps) psUpload(ctx context.Context) {
	op.called = append(op.called, "PsUP")
	op.rhs.InError = op.retPsUPInError
}
func (op *fakeVolSCOps) recordSnapshotInVolume(ctx context.Context) {
	op.called = append(op.called, "RSiV")
}
func (op *fakeVolSCOps) createSnapshotObject(ctx context.Context) {
	op.called = append(op.called, "CSO")
}
func (op *fakeVolSCOps) removeOlderPiTs(ctx context.Context) {
	op.called = append(op.called, "RmOP")
}
func (op *fakeVolSCOps) lmdGuardLock(ctx context.Context) {
	op.called = append(op.called, "SGL")
}
func (op *fakeVolSCOps) lmdGuardUnlock(ctx context.Context) {
	op.called = append(op.called, "SGU")
}
func (op *fakeVolSCOps) syncFail(ctx context.Context) {
	op.called = append(op.called, "sFAIL")
}
func (op *fakeVolSCOps) syncState(ctx context.Context, adjustCB bool) {
	labelMap := map[string]string{
		"PAUSED_IO":            "PSYNC",
		"CREATED_PIT":          "CSYNC",
		"SNAPSHOT_UPLOAD_DONE": "USYNC",
		"FINALIZING_SNAPSHOT":  "FSYNC",
	}
	label := "UNKNOWN"
	if l, ok := labelMap[op.rhs.Request.VolumeSeriesRequestState]; ok {
		label = l
	}
	if adjustCB {
		label += "-T"
	} else {
		label += "-F"
	}
	op.called = append(op.called, label)
}
func (op *fakeVolSCOps) timestampPiT(ctx context.Context) {
	op.called = append(op.called, "TSP")
}
func (op *fakeVolSCOps) unexportTip(ctx context.Context) {
	op.called = append(op.called, "UT")
}

func TestVolSnapshotCreateSteps(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log: tl.Logger(),
		},
		LMDGuard: util.NewCriticalSectionGuard(),
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cspOp := mockcsp.NewMockCloudServiceProvider(mockCtrl)
	app.CSP = cspOp
	appS := &appServant.AppServant{}
	app.AppServant = appS
	fsu := &fakeStateUpdater{}
	app.StateUpdater = fsu
	fPSO := &fps.Controller{}
	app.PSO = fPSO
	app.NuvoVolDirPath = "/nuvo-vol-dir"

	ctx := context.Background()

	c := newComponent()
	c.Init(app)
	assert.NotNil(c.Log)
	assert.Equal(app.Log, c.Log)
	fc := &fake.Client{}
	fc.PassThroughUVRObj = true
	c.oCrud = fc
	c.Animator.OCrud = fc

	fcResetVSRUpdater := func() {
		fc.InVSRUpdaterID = ""
		fc.InVSRUpdaterItems = nil
		fc.FetchVSRUpdaterObj = nil
		fc.RetVSRUpdaterObj = nil
		fc.RetVSRUpdaterErr = nil
		fc.RetVSRUpdaterUpdateErr = nil
		fc.ForceFetchVSRUpdater = false
		fc.ModVSRUpdaterObj = nil
		fc.ModVSRUpdaterErr = nil
		fc.ModVSRUpdaterObj2 = nil
	}

	fm := &vscFakeMounter{}
	fmReset := func() {
		fm.eVS = nil
		fm.eVSR = nil
		fm.retEInError = false
		fm.ueVS = nil
		fm.ueVSR = nil
		fm.retUEInError = false
	}

	fM := &fakeMount.Mounter{}
	c.mounter = fM

	pitUUID := "8e0e60e2-063f-4a29-ba79-e380ab08e75d"
	snapID := "SNAP-ID"
	now := time.Now()
	vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:          "VR-1",
				TimeCreated: strfmt.DateTime(now.Add(-20 * time.Second)),
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			ClusterID:          "CL-1",
			ProtectionDomainID: "PD-1",
			SyncCoordinatorID:  "PARENT-VSR-1",
			SnapIdentifier:     snapID,
			CompleteByTime:     strfmt.DateTime(now),
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				VolumeSeriesID: "VS-1",
				Snapshot: &models.SnapshotData{
					PitIdentifier:      pitUUID,
					ProtectionDomainID: "PD-1",
					SizeBytes:          swag.Int64(100000),
				},
			},
		},
	}
	vsr := vsrClone(vsrObj)
	nuvoVolIdentifier := "nuvo-vol-id"
	vsObj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: "VS-1",
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				VolumeSeriesState:    "IN_USE",
				NuvoVolumeIdentifier: nuvoVolIdentifier,
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				ConsistencyGroupID: "CG-1",
				ServicePlanID:      "SP-1",
				SizeBytes:          swag.Int64(100000),
			},
		},
	}
	parentVSRObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "PARENT-VSR-1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			ClusterID: "CL-1",
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				ConsistencyGroupID: "CG-1",
			},
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				SyncPeers: models.SyncPeerMap{
					"VS-1": models.SyncPeer{},
				},
			},
		},
	}
	parentVSR := vsrClone(parentVSRObj)

	domObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID: "DOM-1",
			},
		},
	}
	domObj.CspDomainType = "AWS"
	domObj.CspDomainAttributes = map[string]models.ValueType{ // required for pstore validators
		aws.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: "bn"},
		aws.AttrRegion:           models.ValueType{Kind: "STRING", Value: "rg"},
		aws.AttrAccessKeyID:      models.ValueType{Kind: "STRING", Value: "ak"},
		aws.AttrSecretAccessKey:  models.ValueType{Kind: "SECRET", Value: "sk"},
	}

	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "CL-1",
			},
		},
		ClusterCreateOnce: models.ClusterCreateOnce{
			CspDomainID: "DOM-1",
		},
	}

	pdObj := &models.ProtectionDomain{
		ProtectionDomainAllOf0: models.ProtectionDomainAllOf0{
			Meta: &models.ObjMeta{ID: "PD-1"},
		},
		ProtectionDomainCreateOnce: models.ProtectionDomainCreateOnce{
			EncryptionAlgorithm: "AES-256",
			EncryptionPassphrase: &models.ValueType{
				Kind: "SECRET", Value: "the passphrase",
			},
		},
	}

	spObj := &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			Meta: &models.ObjMeta{ID: "SP-1"},
		},
		ServicePlanMutable: models.ServicePlanMutable{
			Slos: map[string]models.RestrictedValueType{
				"RPO": models.RestrictedValueType{ValueType: models.ValueType{Kind: "DURATION", Value: "4h"}},
			},
		},
	}

	sRec := &models.SnapshotData{
		PitIdentifier:      pitUUID,
		SnapIdentifier:     snapID,
		ConsistencyGroupID: vsObj.ConsistencyGroupID,
		SnapTime:           strfmt.DateTime(now),
		SizeBytes:          swag.Int64(100000),
	}

	newOp := func() *volSnapCreateOp {
		op := &volSnapCreateOp{}
		op.c = c
		op.ops = op
		op.mops = fm
		op.rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr), VolumeSeries: vsClone(vsObj)}
		return op
	}
	var op *volSnapCreateOp
	var err error

	//  ***************************** checkIfCgStartupRace
	vsr.VolumeSeriesRequestState = "PAUSING_IO"
	tl.Logger().Infof("case: checkIfCgStartupRace %s no race", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	op.checkIfCgStartupRace(parentVSR)
	assert.False(op.inCgRace)
	assert.False(op.rhs.InError)
	t.Logf("syncPeers: %v", parentVSR.SyncPeers)
	_, present := parentVSR.SyncPeers[string(op.rhs.VolumeSeries.Meta.ID)]
	assert.True(present)

	tl.Logger().Infof("case: checkIfCgStartupRace %s in race", vsr.VolumeSeriesRequestState)
	tl.Flush()
	parentVSR = vsrClone(parentVSRObj)
	op = newOp()
	op.rhs.VolumeSeries.ConsistencyGroupID = parentVSR.ConsistencyGroupID + "foo"
	op.checkIfCgStartupRace(parentVSR)
	assert.True(op.inCgRace)
	assert.True(op.rhs.InError)
	assert.Regexp("consistency group.*changed", op.rhs.Request.RequestMessages[0])
	_, present = parentVSR.SyncPeers[string(op.rhs.VolumeSeries.Meta.ID)]
	assert.False(present)

	tl.Logger().Infof("case: checkIfCgStartupRace %s injecting error", vsr.VolumeSeriesRequestState)
	tl.Flush()
	c.rei.SetProperty("vsc-cg-race", &rei.Property{BoolValue: true})
	op = newOp()
	op.checkIfCgStartupRace(parentVSR)
	assert.True(op.inCgRace)
	assert.True(op.rhs.InError)
	assert.Regexp("injecting error", op.rhs.Request.RequestMessages[0])
	_, present = parentVSR.SyncPeers[string(op.rhs.VolumeSeries.Meta.ID)]
	assert.False(present)

	vsr.VolumeSeriesRequestState = "UNDO_PAUSING_IO"
	tl.Logger().Infof("case: checkIfCgStartupRace %s already in race", vsr.VolumeSeriesRequestState)
	tl.Flush()
	parentVSR = vsrClone(parentVSRObj)
	op = newOp()
	op.inCgRace = true
	op.checkIfCgStartupRace(parentVSR)
	assert.True(op.inCgRace)
	assert.False(op.rhs.InError)                                          // block not entered again
	_, present = parentVSR.SyncPeers[string(op.rhs.VolumeSeries.Meta.ID)] // block not entered again
	assert.True(present)                                                  // block not entered again

	//  ***************************** handleCGStartupRace
	vsr.VolumeSeriesRequestState = "PAUSING_IO"
	tl.Logger().Infof("case: handleCGStartupRace %s no race", vsr.VolumeSeriesRequestState)
	tl.Flush()
	parentVSR = vsrClone(parentVSRObj)
	fcResetVSRUpdater()
	fc.FetchVSRUpdaterObj = parentVSR
	op = newOp()
	op.handleCGStartupRace(ctx)
	assert.False(op.inCgRace)
	assert.False(op.rhs.InError)

	tl.Logger().Infof("case: handleCGStartupRace %s in race", vsr.VolumeSeriesRequestState)
	tl.Flush()
	parentVSR = vsrClone(parentVSRObj)
	fcResetVSRUpdater()
	fc.FetchVSRUpdaterObj = parentVSR
	op = newOp()
	op.rhs.VolumeSeries.ConsistencyGroupID = parentVSR.ConsistencyGroupID + "foo"
	op.handleCGStartupRace(ctx)
	assert.True(op.inCgRace)
	assert.True(op.rhs.InError)
	assert.Regexp("consistency group.*changed", op.rhs.Request.RequestMessages[0])
	_, present = parentVSR.SyncPeers[string(op.rhs.VolumeSeries.Meta.ID)]
	assert.False(present)

	tl.Logger().Infof("case: handleCGStartupRace %s updater error", vsr.VolumeSeriesRequestState)
	tl.Flush()
	parentVSR = vsrClone(parentVSRObj)
	fcResetVSRUpdater()
	fc.RetVSRUpdaterErr = fmt.Errorf("updater-error")
	op = newOp()
	op.handleCGStartupRace(ctx)
	assert.False(op.inCgRace)
	assert.True(op.rhs.RetryLater)

	//  ***************************** getInitialState

	entryStates := []struct {
		label string
		state volSnapCreateSubState
	}{
		{"PAUSING_IO", VolSCListPiTs},
		{"PAUSED_IO", VolSCPausedIOSync},
		{"CREATING_PIT", VolSCTimestampPiT},
		{"CREATED_PIT", VolSCCreatedPitSync},
		{"SNAPSHOT_UPLOADING", VolSCSnapshotUploadPlan},
		{"SNAPSHOT_UPLOAD_DONE", VolSCSnapshotUploadSync},
		{"FINALIZING_SNAPSHOT", VolSCFinalizeStart},
		{"UNDO_PAUSING_IO", VolSCUndoPausingIO},
		{"UNDO_PAUSED_IO", VolSCDone},
		{"UNDO_CREATING_PIT", VolSCUndoCreatingPitStart},
		{"UNDO_CREATED_PIT", VolSCDone},
		{"UNDO_SNAPSHOT_UPLOADING", VolSCUndoSnapshotUploadStart},
		{"UNDO_SNAPSHOT_UPLOAD_DONE", VolSCDone},
	}
	for _, tc := range entryStates {
		vsr.VolumeSeriesRequestState = tc.label
		tl.Logger().Infof("case: getInitialState %s", vsr.VolumeSeriesRequestState)
		tl.Flush()
		op = newOp()
		assert.Equal(tc.state, op.getInitialState(ctx), "%s", tc.label)
		assert.False(op.rhs.RetryLater)
		assert.False(op.rhs.InError)
		if tc.label == "PAUSING_IO" {
			c.rei.SetProperty("vsc-block-on-start", &rei.Property{BoolValue: true})
			op = newOp()
			assert.Equal(VolSCDone, op.getInitialState(ctx), "%s", tc.label)
			assert.True(op.rhs.RetryLater)
			assert.False(op.rhs.InError)
		}
	}

	tl.Logger().Infof("case: getInitialState but no longer CONFIGURED")
	op = newOp()
	op.rhs.Request.VolumeSeriesRequestState = "SNAPSHOT_UPLOADING"
	op.rhs.VolumeSeries.VolumeSeriesState = "PROVISIONED"
	assert.Equal(VolSCDone, op.getInitialState(ctx))
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Regexp("no longer CONFIGURED", op.rhs.Request.RequestMessages[0])

	for _, st := range volSnapCreateUndoStates {
		tl.Logger().Infof("case: getInitialState (rei vsc-no-undo) %s", st)
		tl.Flush()
		c.rei.SetProperty("vsc-no-undo", &rei.Property{BoolValue: true})
		op = newOp()
		op.rhs.Request.VolumeSeriesRequestState = st
		assert.Equal(VolSCDone, op.getInitialState(ctx))
		assert.False(op.rhs.RetryLater)
		assert.False(op.rhs.InError)
		assert.Regexp("Undo logic suppressed", op.rhs.Request.RequestMessages[0])
	}

	//  ***************************** exportTip
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "SNAPSHOT_UPLOADING"

	tl.Logger().Infof("case: exportTip %s", vsr.VolumeSeriesRequestState)
	tl.Flush()
	fmReset()
	op = newOp()
	op.tipSnapID = "tipSnapID"
	op.tipPiT = "tipPiT"
	op.exportTip(ctx)
	assert.False(op.rhs.InError)
	assert.Equal(com.VolReqStateVolumeExport, fm.eVSR.VolumeSeriesRequestState)
	assert.NotNil(fm.stash)
	sv := op.rhs.StashGet(exportInternalCallStashKey{})
	assert.NotNil(sv)
	eOp, ok := sv.(*exportInternalArgs)
	assert.True(ok)
	assert.Equal(eOp.snapID, op.tipSnapID)
	assert.Equal(eOp.pitUUID, op.tipPiT)
	fm.eVSR.SnapIdentifier = op.rhs.Request.SnapIdentifier
	fm.eVSR.VolumeSeriesRequestState = op.rhs.Request.VolumeSeriesRequestState
	assert.Equal(op.rhs.Request, fm.eVSR)
	assert.Equal("SNAPSHOT_UPLOADING", op.rhs.Request.VolumeSeriesRequestState)

	tl.Logger().Infof("case: exportTip (in error) %s", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op.rhs.StashSet(exportInternalCallStashKey{}, nil)
	fmReset()
	fm.retEInError = true
	op = newOp()
	op.tipSnapID = "tipSnapID"
	op.tipPiT = "tipPiT"
	op.exportTip(ctx)
	assert.True(op.rhs.InError)
	assert.NotNil(fm.stash)
	sv = op.rhs.StashGet(exportInternalCallStashKey{})
	assert.NotNil(sv)
	eOp, ok = sv.(*exportInternalArgs)
	assert.True(ok)
	assert.Equal(eOp.snapID, op.tipSnapID)
	assert.Equal(eOp.pitUUID, op.tipPiT)
	assert.Equal("SNAPSHOT_UPLOADING", op.rhs.Request.VolumeSeriesRequestState)

	tl.Logger().Infof("case: exportTip (planOnly) %s", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.tipSnapID = "tipSnapID"
	op.exportTip(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("Export LUN for Snapshot", op.rhs.Request.RequestMessages[0])

	//  ***************************** fetchCluster / fetchDomain

	tl.Logger().Infof("case: fetch cluster and domain")
	tl.Flush()
	fc.RetLClObj = clObj
	fc.RetLClErr = nil
	fc.RetLDObj = domObj
	fc.RetLDErr = nil
	op = newOp()
	err = op.fetchDomain(ctx)
	assert.NoError(err)
	assert.Equal(clObj, op.clObj)
	assert.Equal(domObj, op.domObj)

	// repeat should return cached objects
	tl.Logger().Infof("case: fetch cluster and domain (cached)")
	tl.Flush()
	fc.RetLDObj = nil
	fc.RetLDErr = fmt.Errorf("dom-fetch")
	fc.RetLClObj = nil
	fc.RetLClErr = fmt.Errorf("cl-fetch")
	op.clObj = clObj
	op.domObj = domObj
	err = op.fetchCluster(ctx)
	assert.NoError(err)
	err = op.fetchDomain(ctx)
	assert.NoError(err)
	assert.Equal(clObj, op.clObj)
	assert.Equal(domObj, op.domObj)

	tl.Logger().Infof("case: fetchDomain (error)")
	tl.Flush()
	fc.RetLDObj = nil
	fc.RetLDErr = fmt.Errorf("dom-fetch")
	op = newOp()
	op.clObj = clObj
	err = op.fetchDomain(ctx)
	assert.Error(err)
	assert.Regexp("dom-fetch", err)
	assert.Nil(op.domObj)

	tl.Logger().Infof("case: fetchDomain (cluster error)")
	tl.Flush()
	fc.RetLClObj = nil
	fc.RetLClErr = fmt.Errorf("cl-fetch")
	op = newOp()
	err = op.fetchDomain(ctx)
	assert.Error(err)
	assert.Regexp("cl-fetch", err)
	assert.Nil(op.clObj)
	assert.Nil(op.domObj)

	//  ***************************** fetchProtectionDomain

	tl.Logger().Infof("case: fetch protectionDomain")
	tl.Flush()
	fc.InPDFetchID = ""
	fc.RetPDFetchObj = pdObj
	fc.RetPDFetchErr = nil
	op = newOp()
	assert.NotEmpty(op.rhs.Request.ProtectionDomainID)
	err = op.fetchProtectionDomain(ctx)
	assert.Equal(pdObj, op.pdObj)
	assert.Equal(string(op.rhs.Request.ProtectionDomainID), fc.InPDFetchID)

	tl.Logger().Infof("case: fetch protectionDomain (cached)")
	tl.Flush()
	fc.InPDFetchID = ""
	fc.RetPDFetchObj = nil
	fc.RetPDFetchErr = fmt.Errorf("pd-fetch")
	assert.NotEmpty(op.rhs.Request.ProtectionDomainID)
	err = op.fetchProtectionDomain(ctx)
	assert.Equal(pdObj, op.pdObj)
	assert.Empty(fc.InPDFetchID)

	tl.Logger().Infof("case: fetch protectionDomain (error)")
	tl.Flush()
	fc.InPDFetchID = ""
	fc.RetPDFetchObj = nil
	fc.RetPDFetchErr = fmt.Errorf("pd-fetch")
	op = newOp()
	assert.NotEmpty(op.rhs.Request.ProtectionDomainID)
	err = op.fetchProtectionDomain(ctx)
	assert.Error(err)
	assert.Regexp("pd-fetch", err)
	assert.Nil(op.pdObj)
	assert.Equal(string(op.rhs.Request.ProtectionDomainID), fc.InPDFetchID)

	//  ***************************** fetchServicePlan

	tl.Logger().Infof("case: fetchServicePlan")
	tl.Flush()
	appS.RetGSPObj = spObj
	appS.RetGSPErr = nil
	op = newOp()
	op.fetchServicePlan(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.EqualValues(op.rhs.VolumeSeries.ServicePlanID, appS.InGSPid)
	assert.Equal(spObj, op.spObj)

	tl.Logger().Infof("case: fetchServicePlan (error)")
	tl.Flush()
	appS.RetGSPObj = nil
	appS.RetGSPErr = fmt.Errorf("sp-fetch-error")
	op = newOp()
	op.fetchServicePlan(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Nil(op.spObj)

	tl.Logger().Infof("case: fetchServicePlan (plan only)")
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.fetchServicePlan(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Regexp("Fetch ServicePlan", op.rhs.Request.RequestMessages[0])
	assert.Nil(op.spObj)

	//  ***************************** finalizeSnapshotDataInVolume / computeVsSchedulerDataAfterSnapshot
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "FINALIZING_SNAPSHOT"

	rpoDur := 4 * time.Hour
	rpoDurSec := int64(rpoDur / time.Second)
	safetyBuffer := int64(float64(rpoDurSec) * agentd.SnapshotSafetyBufferMultiplier)

	volLMD := &models.LifecycleManagementData{
		EstimatedSizeBytes: 0,
		SizeEstimateRatio:  1.0,
	}
	vsrSRec := &models.SnapshotData{
		PitIdentifier:      pitUUID,
		SnapIdentifier:     snapID,
		ConsistencyGroupID: vsObj.ConsistencyGroupID,
		SnapTime:           strfmt.DateTime(now),
		Locations: []*models.SnapshotLocation{
			&models.SnapshotLocation{CreationTime: strfmt.DateTime(now), CspDomainID: "DOM-1"},
		},
	}
	oneMi := int64(1048576)
	thirtyMi := 30 * oneMi
	invocationLMD := &models.LifecycleManagementData{ // computeVsSchedulerDataAfterSnapshot tests
		// from volume
		EstimatedSizeBytes: volLMD.EstimatedSizeBytes, // can vary
		SizeEstimateRatio:  volLMD.SizeEstimateRatio,
		// from execution
		LastSnapTime:              vsrSRec.SnapTime,
		LastUploadTime:            strfmt.DateTime(now),
		LastUploadSizeBytes:       thirtyMi,
		LastUploadTransferRateBPS: int32(oneMi),
		GenUUID:                   "uuid",
		WriteIOCount:              100,
		// unmodified
		FinalSnapshotNeeded: true,
		LayoutAlgorithm:     "layoutAlgorithm",
	}
	lastWriteStat := &nuvoapi.StatsIO{
		SizeTotal:  uint64(20 * oneMi),
		SeriesUUID: "uuid",
		Count:      uint64(200),
	}
	lun := &agentd.LUN{
		LastWriteStat: lastWriteStat,
	}
	vsrLMDObj := &models.LifecycleManagementData{ // finalizeSnapshotDataInVolume tests
		EstimatedSizeBytes:        10 * oneMi,
		GenUUID:                   "uuid",
		WriteIOCount:              150,
		LastUploadTransferRateBPS: int32(oneMi),
	}
	interimBytes := int64(lastWriteStat.SizeTotal) - vsrLMDObj.EstimatedSizeBytes

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cspOp = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cspOp.EXPECT().ProtectionStoreUploadTransferRate().Return(int32(oneMi))
	app.CSP = cspOp

	var buf bytes.Buffer
	op = newOp()
	tl.Logger().Infof("case: computeVsSchedulerDataAfterSnapshot (!EstimatedSizeBytes, LastUploadTransferRateBPS=0, SafetyBuffer)")
	tl.Flush()
	inLMD := lmdClone(invocationLMD)
	inLMD.LastUploadTransferRateBPS = 0
	inLMD.LastUploadSizeBytes = (rpoDurSec - 2*safetyBuffer + 1) * oneMi // force fallback to safety buffer
	inLMD.EstimatedSizeBytes = 0
	lmd, nextOffsetSec := op.computeVsSchedulerDataAfterSnapshot(&buf, inLMD, rpoDur, interimBytes)
	assert.Equal(safetyBuffer, nextOffsetSec)
	nextSnapTime := time.Time(inLMD.LastSnapTime).Add(time.Duration(nextOffsetSec) * time.Second)
	finalLMD := lmdClone(inLMD)
	finalLMD.EstimatedSizeBytes = interimBytes
	finalLMD.NextSnapshotTime = strfmt.DateTime(nextSnapTime)
	finalLMD.SizeEstimateRatio = volLMD.SizeEstimateRatio // unchanged
	assert.Equal(finalLMD, lmd)
	tl.Logger().Debugf("%s", buf.String())
	buf.Reset()

	tl.Logger().Infof("case: computeVsSchedulerDataAfterSnapshot (!EstimatedSizeBytes, LastUploadTransferRateBPS=0)")
	tl.Flush()
	inLMD = lmdClone(invocationLMD)
	inLMD.LastUploadSizeBytes = 0
	inLMD.LastUploadTransferRateBPS = int32(oneMi)
	inLMD.EstimatedSizeBytes = 0
	lmd, nextOffsetSec = op.computeVsSchedulerDataAfterSnapshot(&buf, inLMD, rpoDur, interimBytes)
	expNextOffsetSec := rpoDurSec - safetyBuffer
	assert.Equal(expNextOffsetSec, nextOffsetSec)
	nextSnapTime = time.Time(inLMD.LastSnapTime).Add(time.Duration(nextOffsetSec) * time.Second)
	finalLMD = lmdClone(inLMD)
	finalLMD.EstimatedSizeBytes = interimBytes
	finalLMD.NextSnapshotTime = strfmt.DateTime(nextSnapTime)
	finalLMD.SizeEstimateRatio = volLMD.SizeEstimateRatio // unchanged
	assert.Equal(finalLMD, lmd)
	tl.Logger().Debugf("%s", buf.String())
	buf.Reset()

	tl.Logger().Infof("case: computeVsSchedulerDataAfterSnapshot (EstimatedSizeBytes, LastUploadTransferRateBPS)")
	tl.Flush()
	inLMD = lmdClone(invocationLMD)
	inLMD.LastUploadSizeBytes = thirtyMi
	inLMD.LastUploadTransferRateBPS = int32(oneMi)
	inLMD.EstimatedSizeBytes = 20 * oneMi
	lmd, nextOffsetSec = op.computeVsSchedulerDataAfterSnapshot(&buf, inLMD, rpoDur, interimBytes)
	expNextOffsetSec = rpoDurSec - inLMD.LastUploadSizeBytes/oneMi - safetyBuffer
	assert.Equal(expNextOffsetSec, nextOffsetSec)
	nextSnapTime = time.Time(inLMD.LastSnapTime).Add(time.Duration(nextOffsetSec) * time.Second)
	finalLMD = lmdClone(inLMD)
	finalLMD.EstimatedSizeBytes = interimBytes
	finalLMD.NextSnapshotTime = strfmt.DateTime(nextSnapTime)
	finalLMD.SizeEstimateRatio = 30.0 / 20.0
	assert.Equal(finalLMD, lmd)
	tl.Logger().Debugf("%s", buf.String())
	buf.Reset()

	tl.Logger().Infof("case: finalizeSnapshotDataInVolume (updater error)")
	tl.Flush()
	appS.InFLvsID = ""
	appS.InFLSnapID = ""
	appS.RetFLObj = nil
	fc.RetVSUpdaterErr = fmt.Errorf("vs-updater-error")
	op = newOp()
	op.spObj = spObj
	op.rhs.Request.Snapshot = vsrSRec
	op.rhs.Request.LifecycleManagementData = lmdClone(vsrLMDObj)
	op.rhs.VolumeSeries.LifecycleManagementData = volLMD
	op.finalizeSnapshotDataInVolume(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Regexp("vs-updater-error", op.rhs.Request.RequestMessages[0])
	assert.EqualValues(op.rhs.VolumeSeries.Meta.ID, appS.InFLvsID)
	assert.Equal("", appS.InFLSnapID)

	// in the next 4 cases set finalSnapshotNeeded to true to check when it gets unset
	lhuVal := "VALUE1"
	lhu := fmt.Sprintf("%s:%s", com.SystemTagVolumeLastHeadUnexport, lhuVal)

	tl.Logger().Infof("case: finalizeSnapshotDataInVolume (not previously un-exported)")
	tl.Flush()
	appS.InFLvsID = ""
	appS.InFLSnapID = ""
	appS.RetFLObj = lun
	fc.RetVSUpdaterErr = nil
	op = newOp()
	op.spObj = spObj
	op.rhs.Request.Snapshot = vsrSRec
	assert.Equal(snapID, op.rhs.Request.SnapIdentifier)
	op.rhs.Request.LifecycleManagementData = lmdClone(vsrLMDObj)
	volLMD.FinalSnapshotNeeded = true
	op.rhs.VolumeSeries.LifecycleManagementData = volLMD
	op.finalizeSnapshotDataInVolume(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.EqualValues(op.rhs.VolumeSeries.Meta.ID, fc.InVSUpdaterID)
	assert.Equal([]string{"lifecycleManagementData", "systemTags"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Remove)
	assert.EqualValues(op.rhs.VolumeSeries.Meta.ID, appS.InFLvsID)
	assert.Equal("", appS.InFLSnapID)
	assert.NotNil(op.rhs.VolumeSeries.LifecycleManagementData)
	assert.Equal(interimBytes, op.rhs.VolumeSeries.LifecycleManagementData.EstimatedSizeBytes)
	assert.False(op.rhs.VolumeSeries.LifecycleManagementData.FinalSnapshotNeeded)

	tl.Logger().Infof("case: finalizeSnapshotDataInVolume (un-exported during vsr)")
	tl.Flush()
	appS.InFLvsID = ""
	appS.InFLSnapID = ""
	appS.RetFLObj = lun
	fc.RetVSUpdaterErr = nil
	op = newOp()
	stags := util.NewTagList(nil)
	stags.Set(com.SystemTagVolumeLastHeadUnexport, lhuVal)
	stags.Set(com.SystemTagVolumeHeadStatSeries, vsrLMDObj.GenUUID+"foo")
	stags.Set(com.SystemTagVolumeHeadStatCount, fmt.Sprintf("%d", vsrLMDObj.WriteIOCount+10))
	op.rhs.VolumeSeries.SystemTags = stags.List()
	op.spObj = spObj
	op.rhs.Request.Snapshot = vsrSRec
	assert.Equal(snapID, op.rhs.Request.SnapIdentifier)
	op.rhs.Request.LifecycleManagementData = lmdClone(vsrLMDObj)
	volLMD.FinalSnapshotNeeded = true
	op.rhs.VolumeSeries.LifecycleManagementData = volLMD
	op.finalizeSnapshotDataInVolume(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.EqualValues(op.rhs.VolumeSeries.Meta.ID, fc.InVSUpdaterID)
	assert.Equal([]string{"lifecycleManagementData", "systemTags"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Remove)
	assert.EqualValues(op.rhs.VolumeSeries.Meta.ID, appS.InFLvsID)
	assert.Equal("", appS.InFLSnapID)
	assert.NotNil(op.rhs.VolumeSeries.LifecycleManagementData)
	assert.Equal(interimBytes, op.rhs.VolumeSeries.LifecycleManagementData.EstimatedSizeBytes)
	assert.True(op.rhs.VolumeSeries.LifecycleManagementData.FinalSnapshotNeeded)
	stags = util.NewTagList(op.rhs.VolumeSeries.SystemTags)
	val, ok := stags.Get(com.SystemTagVolumeLastHeadUnexport)
	assert.True(ok)
	assert.Equal(lhuVal, val)
	val, ok = stags.Get(com.SystemTagVolumeHeadStatSeries)
	assert.True(ok)
	assert.Equal(vsrLMDObj.GenUUID, val)
	val, ok = stags.Get(com.SystemTagVolumeHeadStatCount)
	assert.True(ok)
	assert.Equal(fmt.Sprintf("%d", vsrLMDObj.WriteIOCount), val)

	tl.Logger().Infof("case: finalizeSnapshotDataInVolume (un-exported before VSR)")
	tl.Flush()
	appS.InFLvsID = ""
	appS.InFLSnapID = ""
	appS.RetFLObj = lun
	fc.RetVSUpdaterErr = nil
	op = newOp()
	op.rhs.VolumeSeries.SystemTags = []string{lhu}
	op.rhs.Request.SystemTags = []string{lhu}
	op.spObj = spObj
	op.rhs.Request.Snapshot = vsrSRec
	assert.Equal(snapID, op.rhs.Request.SnapIdentifier)
	op.rhs.Request.LifecycleManagementData = lmdClone(vsrLMDObj)
	volLMD.FinalSnapshotNeeded = true
	op.rhs.VolumeSeries.LifecycleManagementData = volLMD
	op.finalizeSnapshotDataInVolume(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.EqualValues(op.rhs.VolumeSeries.Meta.ID, fc.InVSUpdaterID)
	assert.Equal([]string{"lifecycleManagementData", "systemTags"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Remove)
	assert.EqualValues(op.rhs.VolumeSeries.Meta.ID, appS.InFLvsID)
	assert.Equal("", appS.InFLSnapID)
	assert.NotNil(op.rhs.VolumeSeries.LifecycleManagementData)
	assert.Equal(interimBytes, op.rhs.VolumeSeries.LifecycleManagementData.EstimatedSizeBytes)
	assert.False(op.rhs.VolumeSeries.LifecycleManagementData.FinalSnapshotNeeded)

	tl.Logger().Infof("case: finalizeSnapshotDataInVolume (re-un-exported during VSR)")
	tl.Flush()
	appS.InFLvsID = ""
	appS.InFLSnapID = ""
	appS.RetFLObj = lun
	fc.RetVSUpdaterErr = nil
	op = newOp()
	op.rhs.VolumeSeries.SystemTags = []string{lhu + "xxx"}
	op.rhs.Request.SystemTags = []string{lhu}
	op.spObj = spObj
	op.rhs.Request.Snapshot = vsrSRec
	assert.Equal(snapID, op.rhs.Request.SnapIdentifier)
	op.rhs.Request.LifecycleManagementData = lmdClone(vsrLMDObj)
	volLMD.FinalSnapshotNeeded = true
	op.rhs.VolumeSeries.LifecycleManagementData = volLMD
	op.finalizeSnapshotDataInVolume(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.EqualValues(op.rhs.VolumeSeries.Meta.ID, fc.InVSUpdaterID)
	assert.Equal([]string{"lifecycleManagementData", "systemTags"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Remove)
	assert.EqualValues(op.rhs.VolumeSeries.Meta.ID, appS.InFLvsID)
	assert.Equal("", appS.InFLSnapID)
	assert.NotNil(op.rhs.VolumeSeries.LifecycleManagementData)
	assert.Equal(interimBytes, op.rhs.VolumeSeries.LifecycleManagementData.EstimatedSizeBytes)
	assert.True(op.rhs.VolumeSeries.LifecycleManagementData.FinalSnapshotNeeded)

	tl.Logger().Infof("case: finalizeSnapshotDataInVolume (no previous vol lmd, lastWriteStat UUID changed)")
	tl.Flush()
	appS.InFLvsID = ""
	appS.InFLSnapID = ""
	appS.RetFLObj = lun
	fc.RetVSUpdaterErr = nil
	op = newOp()
	op.spObj = spObj
	op.rhs.Request.Snapshot = vsrSRec
	assert.Equal(snapID, op.rhs.Request.SnapIdentifier)
	op.rhs.Request.LifecycleManagementData = lmdClone(vsrLMDObj)
	op.rhs.Request.LifecycleManagementData.GenUUID = "different uuid" // pretend previous was different
	op.finalizeSnapshotDataInVolume(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.EqualValues(op.rhs.VolumeSeries.Meta.ID, fc.InVSUpdaterID)
	assert.Equal([]string{"lifecycleManagementData", "systemTags"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Remove)
	assert.EqualValues(op.rhs.VolumeSeries.Meta.ID, appS.InFLvsID)
	assert.Equal("", appS.InFLSnapID)
	assert.NotNil(op.rhs.VolumeSeries.LifecycleManagementData)
	assert.EqualValues(0, op.rhs.VolumeSeries.LifecycleManagementData.EstimatedSizeBytes)

	tl.Logger().Infof("case: finalizeSnapshotDataInVolume (plan only)")
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.rhs.Request.Snapshot = vsrSRec
	assert.Equal(snapID, op.rhs.Request.SnapIdentifier)
	op.finalizeSnapshotDataInVolume(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Regexp("Finalize snapshot in volume", op.rhs.Request.RequestMessages[0])

	//  ***************************** fsFreeze / fsUnfreeze via doFreezeUnfreeze helper
	tl.Logger().Infof("case: fsFreeze no filesystem attached")
	tl.Flush()
	op = newOp()
	op.rhs.VolumeSeries.SystemTags = nil
	op.fsFreeze(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Freeze: No filesystem attached", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: fsUnfreeze no filesystem attached")
	tl.Flush()
	op = newOp()
	op.rhs.VolumeSeries.SystemTags = nil
	op.fsUnfreeze(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Unfreeze: No filesystem attached", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: fsFreeze planOnly")
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.rhs.VolumeSeries.SystemTags = []string{fmt.Sprintf("%s:target", com.SystemTagVolumeFsAttached)}
	fM.InFsFArgs = nil
	fM.RetFsFfCmds = bytes.NewBufferString("fsfreeze\n")
	op.fsFreeze(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(fM.InFsFArgs)
	assert.True(fM.InFsFArgs.PlanOnly)
	assert.Equal("target", fM.InFsFArgs.Target)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Equal("fsfreeze", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Infof("case: fsFreeze success")
	tl.Flush()
	op = newOp()
	op.rhs.VolumeSeries.SystemTags = []string{fmt.Sprintf("%s:target", com.SystemTagVolumeFsAttached)}
	fM.InFsFArgs = nil
	fM.RetFsFfCmds = bytes.NewBufferString("fsfreeze\n")
	op.fsFreeze(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(fM.InFsFArgs)
	assert.False(fM.InFsFArgs.PlanOnly)
	assert.Equal("target", fM.InFsFArgs.Target)
	assert.Equal(time.Time(op.rhs.Request.CompleteByTime), fM.InFsFArgs.Deadline)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Equal("fsfreeze", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Infof("case: fsFreeze error ignored")
	tl.Flush()
	op = newOp()
	op.rhs.VolumeSeries.SystemTags = []string{fmt.Sprintf("%s:target", com.SystemTagVolumeFsAttached)}
	fM.InFsFArgs = nil
	fM.RetFsFfCmds = bytes.NewBufferString("fsfreeze error\n")
	fM.RetFsFErr = fmt.Errorf("freeze-error")
	op.fsFreeze(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(fM.InFsFArgs)
	assert.False(fM.InFsFArgs.PlanOnly)
	assert.Equal("target", fM.InFsFArgs.Target)
	assert.Equal(time.Time(op.rhs.Request.CompleteByTime), fM.InFsFArgs.Deadline)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Equal("fsfreeze error", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Infof("case: fsUnfreeze planOnly")
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.rhs.VolumeSeries.SystemTags = []string{fmt.Sprintf("%s:target", com.SystemTagVolumeFsAttached)}
	fM.InUFsFArgs = nil
	fM.RetUFsFCmds = bytes.NewBufferString("fsfreeze\n")
	op.fsUnfreeze(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(fM.InUFsFArgs)
	assert.True(fM.InUFsFArgs.PlanOnly)
	assert.Equal("target", fM.InUFsFArgs.Target)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Equal("fsfreeze", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Infof("case: fsUnfreeze success")
	tl.Flush()
	op = newOp()
	op.rhs.VolumeSeries.SystemTags = []string{fmt.Sprintf("%s:target", com.SystemTagVolumeFsAttached)}
	fM.InUFsFArgs = nil
	fM.RetUFsFCmds = bytes.NewBufferString("fsfreeze\n")
	op.fsUnfreeze(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(fM.InUFsFArgs)
	assert.False(fM.InUFsFArgs.PlanOnly)
	assert.Equal("target", fM.InUFsFArgs.Target)
	assert.Equal(time.Time(op.rhs.Request.CompleteByTime), fM.InUFsFArgs.Deadline)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Equal("fsfreeze", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Infof("case: fsUnfreeze error ignored")
	tl.Flush()
	op = newOp()
	op.rhs.VolumeSeries.SystemTags = []string{fmt.Sprintf("%s:target", com.SystemTagVolumeFsAttached)}
	fM.InUFsFArgs = nil
	fM.RetUFsFCmds = bytes.NewBufferString("fsfreeze error\n")
	fM.RetUfErr = fmt.Errorf("unfreeze-error")
	op.fsUnfreeze(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(fM.InUFsFArgs)
	assert.False(fM.InUFsFArgs.PlanOnly)
	assert.Equal("target", fM.InUFsFArgs.Target)
	assert.Equal(time.Time(op.rhs.Request.CompleteByTime), fM.InUFsFArgs.Deadline)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Equal("fsfreeze error", op.rhs.Request.RequestMessages[0].Message)

	//  ***************************** nuvoCreatePiT
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "CREATING_PIT"

	tl.Logger().Infof("case: nuvoCreatePiT %s", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI := mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().CreatePit(nuvoVolIdentifier, pitUUID).Return(nil)
	c.App.NuvoAPI = nvAPI
	op.nuvoCreatePiT(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.True(op.pitCreated())

	tl.Logger().Infof("case: nuvoCreatePiT %s (error)", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().CreatePit(nuvoVolIdentifier, pitUUID).Return(fmt.Errorf("create-pit-error"))
	c.App.NuvoAPI = nvAPI
	op.nuvoCreatePiT(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Regexp("CreatePit.*create-pit-error", op.rhs.Request.RequestMessages[0])
	assert.False(op.pitCreated())

	tl.Logger().Infof("case: nuvoCreatePiT %s (temp error)", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nErr := nuvoapi.NewNuvoAPIError("tempErr", true)
	nvAPI.EXPECT().CreatePit(nuvoVolIdentifier, pitUUID).Return(nErr)
	c.App.NuvoAPI = nvAPI
	op.nuvoCreatePiT(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("tempErr", op.rhs.Request.RequestMessages[0])
	assert.False(op.pitCreated())

	tl.Logger().Infof("case: nuvoCreatePiT %s (planOnly)", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.nuvoCreatePiT(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("Create PiT", op.rhs.Request.RequestMessages[0])
	assert.False(op.pitCreated())

	//  ***************************** nuvoDeletePiT
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "UNDO_CREATING_PIT"

	tl.Logger().Infof("case: nuvoDeletePiT %s", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	op.pits = []string{pitUUID, "otherPiT"}
	assert.True(op.pitCreated())
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().DeletePit(nuvoVolIdentifier, pitUUID).Return(nil)
	c.App.NuvoAPI = nvAPI
	op.nuvoDeletePiT(ctx, pitUUID)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.False(op.pitCreated())
	assert.Equal([]string{"otherPiT"}, op.pits)

	tl.Logger().Infof("case: nuvoDeletePiT %s (error)", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	op.pits = []string{pitUUID}
	assert.True(op.pitCreated())
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().DeletePit(nuvoVolIdentifier, pitUUID).Return(fmt.Errorf("create-pit-error"))
	c.App.NuvoAPI = nvAPI
	op.nuvoDeletePiT(ctx, pitUUID)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Regexp("DeletePit.*create-pit-error", op.rhs.Request.RequestMessages[0])
	assert.True(op.pitCreated())

	tl.Logger().Infof("case: nuvoDeletePiT %s (temp error)", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	op.pits = []string{pitUUID}
	assert.True(op.pitCreated())
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nErr = nuvoapi.NewNuvoAPIError("tempErr", true)
	nvAPI.EXPECT().DeletePit(nuvoVolIdentifier, pitUUID).Return(nErr)
	c.App.NuvoAPI = nvAPI
	op.nuvoDeletePiT(ctx, pitUUID)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("tempErr", op.rhs.Request.RequestMessages[0])
	assert.True(op.pitCreated())

	tl.Logger().Infof("case: nuvoDeletePiT %s (planOnly)", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	op.pits = []string{pitUUID}
	assert.True(op.pitCreated())
	op.planOnly = true
	op.nuvoDeletePiT(ctx, pitUUID)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("Delete PiT", op.rhs.Request.RequestMessages[0])
	assert.True(op.pitCreated())

	//  ***************************** nuvoGetWriteStat
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "CREATING_PIT"

	tl.Logger().Infof("case: nuvoGetWriteStat %s", vsr.VolumeSeriesRequestState)
	tl.Flush()
	writeStats := &nuvoapi.StatsIO{
		SizeTotal:  1000,
		SeriesUUID: "UUID",
		Count:      32,
	}
	userStats := &nuvoapi.StatsCache{}
	metaStats := &nuvoapi.StatsCache{}
	comboStats := &nuvoapi.StatsCombinedVolume{
		IOWrites:      *writeStats,
		CacheUser:     *userStats,
		CacheMetadata: *metaStats,
	}
	op = newOp()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().GetVolumeStats(false, nuvoVolIdentifier).Return(comboStats, nil)
	c.App.NuvoAPI = nvAPI
	op.nuvoGetWriteStat(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	expLMD := &models.LifecycleManagementData{
		EstimatedSizeBytes: 1000,
		GenUUID:            "UUID",
		WriteIOCount:       32,
	}
	assert.Equal(expLMD, op.rhs.Request.LifecycleManagementData)

	tl.Logger().Infof("case: nuvoGetWriteStat %s (error)", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().GetVolumeStats(false, nuvoVolIdentifier).Return(nil, fmt.Errorf("get-stats"))
	c.App.NuvoAPI = nvAPI
	op.nuvoGetWriteStat(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Nil(op.rhs.Request.LifecycleManagementData)

	tl.Logger().Infof("case: nuvoGetWriteStat %s (plan only)", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.nuvoGetWriteStat(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Nil(op.rhs.Request.LifecycleManagementData)

	//  ***************************** nuvoListPiTs, pitCreated
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "CREATING_PIT"

	retListPits := []string{
		pitUUID,
	}
	tl.Logger().Infof("case: nuvoListPiTs %s", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().ListPits(nuvoVolIdentifier).Return(retListPits, nil)
	c.App.NuvoAPI = nvAPI
	op.nuvoListPiTs(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(retListPits, op.pits)
	assert.True(op.pitCreated())
	op.rhs.Request.Snapshot.PitIdentifier = pitUUID + "foo"
	assert.False(op.pitCreated())

	tl.Logger().Infof("case: nuvoListPiTs %s (error)", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().ListPits(nuvoVolIdentifier).Return(nil, fmt.Errorf("list-pits-error"))
	c.App.NuvoAPI = nvAPI
	op.nuvoListPiTs(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Regexp("ListPits.*list-pits-error", op.rhs.Request.RequestMessages[0])
	assert.Nil(op.pits)
	assert.False(op.pitCreated())

	tl.Logger().Infof("case: nuvoListPiTs %s (temp error)", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nErr = nuvoapi.NewNuvoAPIError("tempErr", true)
	nvAPI.EXPECT().ListPits(nuvoVolIdentifier).Return(nil, nErr)
	c.App.NuvoAPI = nvAPI
	op.nuvoListPiTs(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("tempErr", op.rhs.Request.RequestMessages[0])
	assert.Nil(op.pits)
	assert.False(op.pitCreated())

	//  ***************************** listVolumeSnapshots
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "PAUSING_IO"

	vsSnapshots := []*models.Snapshot{
		&models.Snapshot{
			SnapshotAllOf0: models.SnapshotAllOf0{
				Meta: &models.ObjMeta{
					ID: "id0",
				},
				SnapTime:           strfmt.DateTime(now.Add(-1 * time.Hour)),
				SnapIdentifier:     "baseSnap",
				PitIdentifier:      "basePiT",
				ProtectionDomainID: "pd-1",
			},
			SnapshotMutable: models.SnapshotMutable{},
		},
		&models.Snapshot{
			SnapshotAllOf0: models.SnapshotAllOf0{
				Meta: &models.ObjMeta{
					ID: "id1",
				},
				SnapTime:           strfmt.DateTime(now.Add(-2 * time.Hour)),
				SnapIdentifier:     "Snap-1",
				PitIdentifier:      "PiT-1",
				ProtectionDomainID: "pd-1",
			},
			SnapshotMutable: models.SnapshotMutable{},
		},
		&models.Snapshot{
			SnapshotAllOf0: models.SnapshotAllOf0{
				Meta: &models.ObjMeta{
					ID: "id2",
				},
				SnapTime:           strfmt.DateTime(now.Add(-3 * time.Hour)),
				SnapIdentifier:     "Snap-2",
				PitIdentifier:      "PiT-2",
				ProtectionDomainID: "pd-1",
			},
			SnapshotMutable: models.SnapshotMutable{},
		},
	}
	opSnapshots := []*models.Snapshot{
		&models.Snapshot{
			SnapshotAllOf0: models.SnapshotAllOf0{
				Meta: &models.ObjMeta{
					ID: "id0",
				},
				SnapTime:           strfmt.DateTime(now.Add(-1 * time.Hour)),
				SnapIdentifier:     "baseSnap",
				PitIdentifier:      "basePiT",
				ProtectionDomainID: "pd-1",
			},
			SnapshotMutable: models.SnapshotMutable{},
		},
		&models.Snapshot{
			SnapshotAllOf0: models.SnapshotAllOf0{
				Meta: &models.ObjMeta{
					ID: "id1",
				},
				SnapTime:           strfmt.DateTime(now.Add(-2 * time.Hour)),
				SnapIdentifier:     "Snap-1",
				PitIdentifier:      "PiT-1",
				ProtectionDomainID: "pd-1",
			},
			SnapshotMutable: models.SnapshotMutable{},
		},
	}

	tl.Logger().Infof("case: listVolumeSnapshots (planOnly, no pits)")
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.listVolumeSnapshots(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("No PiTs present - snapshots not fetched", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Infof("case: listVolumeSnapshots (planOnly, with pits)")
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.pits = []string{"pit"}
	op.listVolumeSnapshots(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("Fetching snapshots for volume.*PiTs", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Infof("case: listVolumeSnapshots (no pits)")
	tl.Flush()
	op = newOp()
	fc.InLsSnapshotObj = nil
	op.listVolumeSnapshots(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Nil(fc.InLsSnapshotObj)
	assert.Nil(op.vsSnapshots)

	tl.Logger().Infof("case: listVolumeSnapshots (with pits)")
	tl.Flush()
	op = newOp()
	op.pits = []string{pitUUID, "otherPiT", "basePiT", "PiT-1"}
	fc.InLsSnapshotObj = nil
	fc.RetLsSnapshotOk = &snapshot.SnapshotListOK{Payload: []*models.Snapshot{}}
	for _, snap := range vsSnapshots {
		if util.Contains(op.pits, snap.PitIdentifier) {
			fc.RetLsSnapshotOk.Payload = append(fc.RetLsSnapshotOk.Payload, snap)
		}
	}
	op.listVolumeSnapshots(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.EqualValues(op.rhs.VolumeSeries.Meta.ID, *fc.InLsSnapshotObj.VolumeSeriesID)
	assert.Equal(op.pits, fc.InLsSnapshotObj.PitIdentifiers)
	assert.Equal(op.vsSnapshots, opSnapshots)

	tl.Logger().Infof("case: listVolumeSnapshots (list error)")
	tl.Flush()
	op = newOp()
	op.pits = []string{pitUUID, "otherPiT", "basePiT", "PiT-1"}
	fc.RetLsSnapshotErr = fmt.Errorf("OTHER ERROR")
	op.listVolumeSnapshots(ctx)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	//  ***************************** planCopy
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "SNAPSHOT_UPLOADING"

	vsrMounts := []*models.Mount{
		&models.Mount{
			SnapIdentifier: "HEAD",
		},
		&models.Mount{
			SnapIdentifier: "baseSnap",
			MountState:     "MOUNTED",
		},
	}

	tl.Logger().Infof("case: planCopy")
	tl.Flush()
	op = newOp()
	op.rhs.Request.ProtectionDomainID = "pd-1"
	op.rhs.Request.Snapshot = sRec
	op.vsSnapshots = opSnapshots
	op.rhs.VolumeSeries.Mounts = vsrMounts
	op.pits = []string{pitUUID, "basePiT"}
	op.planCopy(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal("baseSnap", op.baseSnapIdentifier)
	assert.Equal("basePiT", op.basePiT)
	assert.False(op.tipExported)
	assert.Equal(pitUUID, op.tipPiT)
	assert.Equal(snapID, op.tipSnapID)

	tl.Logger().Infof("case: planCopy pd changed")
	tl.Flush()
	op = newOp()
	op.rhs.Request.ProtectionDomainID = "pd-12"
	op.rhs.Request.Snapshot = sRec
	op.vsSnapshots = opSnapshots
	op.rhs.VolumeSeries.Mounts = vsrMounts
	op.pits = []string{pitUUID, "basePiT"}
	op.planCopy(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Empty(op.baseSnapIdentifier)
	assert.Empty(op.basePiT)
	assert.False(op.tipExported)
	assert.Equal(pitUUID, op.tipPiT)
	assert.Equal(snapID, op.tipSnapID)
	assert.Regexp("Forcing baseline snapshot because protection domain changed", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: planCopy pd changed")
	tl.Flush()
	op = newOp()
	op.rhs.Request.Snapshot = sRec
	op.rhs.VolumeSeries.Mounts = vsrMounts
	op.planCopy(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Empty(op.baseSnapIdentifier)
	assert.Empty(op.basePiT)
	assert.False(op.tipExported)
	assert.Equal(pitUUID, op.tipPiT)
	assert.Equal(snapID, op.tipSnapID)
	assert.Regexp("Baseline snapshot", op.rhs.Request.RequestMessages[0])

	//  ***************************** psDelete
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "UNDO_SNAPSHOT_UPLOADING"

	tl.Logger().Infof("case: psDelete")
	tl.Flush()
	fPSO.InSDArg = nil
	fPSO.RetSDErr = nil
	fsu.rCalled = 0
	op = newOp()
	op.clObj = clObj
	op.domObj = domObj
	op.psDelete(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(fPSO.InSDArg)
	assert.True(fPSO.InSDArg.Validate(), "SDArg: %#v", fPSO.InSDArg)
	assert.Equal(op.rhs.Request.SnapIdentifier, fPSO.InSDArg.SnapIdentifier)
	assert.EqualValues(op.rhs.Request.Meta.ID, fPSO.InSDArg.ID)
	assert.EqualValues(domObj.CspDomainType, fPSO.InSDArg.PStore.CspDomainType)
	assert.EqualValues(domObj.CspDomainAttributes, fPSO.InSDArg.PStore.CspDomainAttributes)
	assert.EqualValues(pitUUID, fPSO.InSDArg.SourceSnapshot)
	assert.Equal(0, fsu.rCalled)

	tl.Logger().Infof("case: psDelete (error)")
	tl.Flush()
	fPSO.InSDArg = nil
	fPSO.RetSDErr = fmt.Errorf("snapshot-delete")
	fPSO.RetSDObj = nil
	fsu.rCalled = 0
	op = newOp()
	op.clObj = clObj
	op.domObj = domObj
	op.psDelete(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.NotNil(fPSO.InSDArg)
	assert.Equal(1, fsu.rCalled)

	tl.Logger().Infof("case: psDelete (domain fetch error)")
	tl.Flush()
	op = newOp()
	op.clObj = clObj
	fc.RetLDObj = nil
	fc.RetLDErr = fmt.Errorf("dom-fetch")
	op.psDelete(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)

	//  ***************************** psUpload
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "SNAPSHOT_UPLOADING"

	tl.Logger().Infof("case: psUpload (req lmd, no vol lmd, no base PiT or SnapIdentifier)")
	tl.Flush()
	fPSO.InSBArg = nil
	fPSO.RetSBErr = nil
	fPSO.RetSBObj = &pstore.SnapshotBackupResult{
		Stats: pstore.CopyStats{
			BytesTransferred: 1000,
		},
	}
	fsu.rCalled = 0
	op = newOp()
	op.clObj = clObj
	op.domObj = domObj
	op.pdObj = pdObj
	op.tipPiT = pitUUID
	op.rhs.Request.MountedNodeDevice = "VsUuid-PitUuid"
	op.rhs.Request.LifecycleManagementData = &models.LifecycleManagementData{
		EstimatedSizeBytes: 999,
		GenUUID:            "uuid",
		WriteIOCount:       10488,
	}
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().LunPath(string(op.c.App.NuvoVolDirPath), "VsUuid-PitUuid").Return("/dir_path/VsUuid-PitUuid").Times(1)
	op.c.App.NuvoAPI = nvAPI
	tB := time.Now()
	op.psUpload(ctx)
	tA := time.Now()
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(clObj, op.clObj)
	assert.Equal(domObj, op.domObj)
	assert.NotNil(fPSO.InSBArg)
	assert.True(fPSO.InSBArg.Validate())
	assert.Equal(op.rhs.Request.SnapIdentifier, fPSO.InSBArg.SnapIdentifier)
	assert.EqualValues(op.rhs.Request.Meta.ID, fPSO.InSBArg.ID)
	assert.EqualValues(domObj.CspDomainType, fPSO.InSBArg.PStore.CspDomainType)
	assert.EqualValues(domObj.CspDomainAttributes, fPSO.InSBArg.PStore.CspDomainAttributes)
	assert.Equal("/dir_path/VsUuid-PitUuid", fPSO.InSBArg.SourceFile)
	assert.EqualValues(op.rhs.VolumeSeries.Meta.ID, fPSO.InSBArg.VsID)
	assert.Equal(op.rhs.VolumeSeries.NuvoVolumeIdentifier, fPSO.InSBArg.NuvoVolumeIdentifier)
	assert.Equal("", fPSO.InSBArg.BasePiT)
	assert.Equal("", fPSO.InSBArg.BaseSnapIdentifier)
	assert.Equal(pitUUID, fPSO.InSBArg.IncrPiT)
	assert.Equal(op.rhs, fPSO.InSBArg.Reporter)
	assert.Equal(1, len(op.rhs.Request.Snapshot.Locations))
	assert.EqualValues(domObj.Meta.ID, op.rhs.Request.Snapshot.Locations[0].CspDomainID)
	assert.True(tB.Before(time.Time(op.rhs.Request.Snapshot.Locations[0].CreationTime)))
	assert.True(tA.After(time.Time(op.rhs.Request.Snapshot.Locations[0].CreationTime)))
	assert.NotNil(op.rhs.Request.LifecycleManagementData)
	assert.Equal(op.rhs.Request.Snapshot.Locations[0].CreationTime, op.rhs.Request.LifecycleManagementData.LastUploadTime)
	assert.Equal(fPSO.RetSBObj.Stats.BytesTransferred, op.rhs.Request.LifecycleManagementData.LastUploadSizeBytes)
	assert.NotZero(op.rhs.Request.LifecycleManagementData.LastUploadTransferRateBPS)
	assert.EqualValues(999, op.rhs.Request.LifecycleManagementData.EstimatedSizeBytes)
	assert.EqualValues("uuid", op.rhs.Request.LifecycleManagementData.GenUUID)
	assert.EqualValues(10488, op.rhs.Request.LifecycleManagementData.WriteIOCount)
	assert.Equal(0, fsu.rCalled)
	assert.Equal(1, tl.CountPattern("VolumeSeriesRequest .* VolumeSeries \\[.*\\] PSO SnapshotBackup\\(sn:.* bsn: p:.* d:DOM-1 s:.* b: pd:.*\\)"))

	tl.Logger().Infof("case: psUpload (vol has lmd, with base PiT, 0 bytes transferred)")
	tl.Flush()
	fPSO.InSBArg = nil
	fPSO.RetSBErr = nil
	fPSO.RetSBObj = &pstore.SnapshotBackupResult{
		Stats: pstore.CopyStats{
			BytesTransferred: 0,
		},
	}
	fsu.rCalled = 0
	op = newOp()
	assert.NotEmpty(op.rhs.Request.ProtectionDomainID)
	op.clObj = clObj
	op.domObj = domObj
	op.pdObj = pdObj
	op.tipPiT = pitUUID
	op.basePiT = "basePiT"
	op.baseSnapIdentifier = "baseSnapIdentifier"
	op.rhs.Request.MountedNodeDevice = "VsUuid-PitUuid"
	op.rhs.VolumeSeries.LifecycleManagementData = &models.LifecycleManagementData{
		LastUploadTransferRateBPS: 1000,
		SizeEstimateRatio:         3.142,
		EstimatedSizeBytes:        2000,
	}
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().LunPath(string(op.c.App.NuvoVolDirPath), "VsUuid-PitUuid").Return("/dir_path/VsUuid-PitUuid").Times(1)
	op.c.App.NuvoAPI = nvAPI
	tB = time.Now()
	op.psUpload(ctx)
	tA = time.Now()
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(clObj, op.clObj)
	assert.Equal(domObj, op.domObj)
	assert.NotNil(fPSO.InSBArg)
	assert.True(fPSO.InSBArg.Validate())
	assert.Equal(op.rhs.Request.SnapIdentifier, fPSO.InSBArg.SnapIdentifier)
	assert.EqualValues(op.rhs.Request.Meta.ID, fPSO.InSBArg.ID)
	assert.EqualValues(domObj.CspDomainType, fPSO.InSBArg.PStore.CspDomainType)
	assert.EqualValues(domObj.CspDomainAttributes, fPSO.InSBArg.PStore.CspDomainAttributes)
	assert.Equal("/dir_path/VsUuid-PitUuid", fPSO.InSBArg.SourceFile)
	assert.Equal("basePiT", fPSO.InSBArg.BasePiT)
	assert.Equal("baseSnapIdentifier", fPSO.InSBArg.BaseSnapIdentifier)
	assert.Equal(pitUUID, fPSO.InSBArg.IncrPiT)
	assert.Equal(pdObj.EncryptionAlgorithm, fPSO.InSBArg.EncryptionAlgorithm)
	assert.Equal(pdObj.EncryptionPassphrase.Value, fPSO.InSBArg.Passphrase)
	assert.Equal(string(pdObj.Meta.ID), fPSO.InSBArg.ProtectionDomainID)
	assert.Equal(1, len(op.rhs.Request.Snapshot.Locations))
	assert.EqualValues(domObj.Meta.ID, op.rhs.Request.Snapshot.Locations[0].CspDomainID)
	assert.True(tB.Before(time.Time(op.rhs.Request.Snapshot.Locations[0].CreationTime)))
	assert.True(tA.After(time.Time(op.rhs.Request.Snapshot.Locations[0].CreationTime)))
	assert.NotNil(op.rhs.Request.LifecycleManagementData)
	assert.Equal(op.rhs.Request.Snapshot.Locations[0].CreationTime, op.rhs.Request.LifecycleManagementData.LastUploadTime)
	assert.Equal(fPSO.RetSBObj.Stats.BytesTransferred, op.rhs.Request.LifecycleManagementData.LastUploadSizeBytes)
	assert.EqualValues(1000, op.rhs.Request.LifecycleManagementData.LastUploadTransferRateBPS)
	assert.EqualValues(3.142, op.rhs.Request.LifecycleManagementData.SizeEstimateRatio)
	assert.EqualValues(0, op.rhs.Request.LifecycleManagementData.EstimatedSizeBytes)
	assert.Equal(0, fsu.rCalled)
	assert.Equal(1, tl.CountPattern("VolumeSeriesRequest .* VolumeSeries \\[.*\\] PSO SnapshotBackup\\(sn:.* bsn:baseSnapIdentifier p:.* d:DOM-1 s:.* b:basePiT pd:.*\\)"))

	tl.Logger().Infof("case: psUpload (comm error)")
	tl.Flush()
	fPSO.InSBArg = nil
	fPSO.RetSBErr = pstore.NewRetryableError("retryable-error")
	fPSO.RetSBObj = nil
	fsu.rCalled = 0
	op = newOp()
	op.clObj = clObj
	op.domObj = domObj
	op.pdObj = pdObj
	op.tipPiT = pitUUID
	op.rhs.Request.MountedNodeDevice = "VsUuid-PitUuid"
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().LunPath(string(op.c.App.NuvoVolDirPath), "VsUuid-PitUuid").Return("/dir_path/VsUuid-PitUuid").Times(1)
	op.c.App.NuvoAPI = nvAPI
	op.psUpload(ctx)
	tA = time.Now()
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Equal(1, fsu.rCalled)

	tl.Logger().Infof("case: psUpload (upload error)")
	tl.Flush()
	fPSO.InSBArg = nil
	fPSO.RetSBErr = fmt.Errorf("snapshot-backup")
	fPSO.RetSBObj = &pstore.SnapshotBackupResult{Error: pstore.CopyError{Description: "snapshot-backup", Fatal: true}}
	fsu.rCalled = 0
	op = newOp()
	op.clObj = clObj
	op.domObj = domObj
	op.pdObj = pdObj
	op.tipPiT = pitUUID
	op.rhs.Request.MountedNodeDevice = "VsUuid-PitUuid"
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().LunPath(string(op.c.App.NuvoVolDirPath), "VsUuid-PitUuid").Return("/dir_path/VsUuid-PitUuid").Times(1)
	op.c.App.NuvoAPI = nvAPI
	op.psUpload(ctx)
	tA = time.Now()
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Regexp("snapshot-backup", op.rhs.Request.RequestMessages[0])
	assert.Equal(1, fsu.rCalled)

	tl.Logger().Infof("case: psUpload (pd fetch error)")
	tl.Flush()
	op = newOp()
	op.clObj = clObj
	op.domObj = domObj
	assert.NotEmpty(op.rhs.Request.ProtectionDomainID)
	fc.RetPDFetchObj = nil
	fc.RetPDFetchErr = fmt.Errorf("pd-fetch")
	op.psUpload(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)

	tl.Logger().Infof("case: psUpload (domain fetch error)")
	tl.Flush()
	op = newOp()
	op.clObj = clObj
	fc.RetLDObj = nil
	fc.RetLDErr = fmt.Errorf("dom-fetch")
	op.psUpload(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)

	tl.Logger().Infof("case: psUpload (plan only)")
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.psUpload(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Regexp("Copy snapshot to protection store", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: psUpload (injected error)")
	tl.Flush()
	op = newOp()
	c.rei.SetProperty("vsc-upload-error", &rei.Property{BoolValue: true})
	op.psUpload(ctx)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Regexp("vsc-upload-error", op.rhs.Request.RequestMessages[0])

	//  ***************************** removeOlderPiTs
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "FINALIZING_SNAPSHOT"
	vsSnapshots = []*models.Snapshot{
		&models.Snapshot{
			SnapshotAllOf0: models.SnapshotAllOf0{
				Meta: &models.ObjMeta{
					ID: "id1",
				},
				SnapTime:           strfmt.DateTime(now.Add(-1 * time.Hour)),
				SnapIdentifier:     "baseSnap",
				PitIdentifier:      "pitBull",
				ProtectionDomainID: "pd-1",
			},
			SnapshotMutable: models.SnapshotMutable{},
		},
		&models.Snapshot{
			SnapshotAllOf0: models.SnapshotAllOf0{
				Meta: &models.ObjMeta{
					ID: "id2",
				},
				SnapTime:           strfmt.DateTime(now.Add(-2 * time.Hour)),
				SnapIdentifier:     "baseSnap",
				PitIdentifier:      "pitBase-1",
				ProtectionDomainID: "pd-1",
			},
			SnapshotMutable: models.SnapshotMutable{},
		},
		&models.Snapshot{
			SnapshotAllOf0: models.SnapshotAllOf0{
				Meta: &models.ObjMeta{
					ID: "id3",
				},
				SnapTime:           strfmt.DateTime(now.Add(-3 * time.Hour)),
				SnapIdentifier:     "baseSnap",
				PitIdentifier:      "pitBase-2",
				ProtectionDomainID: "pd-1",
			},
			SnapshotMutable: models.SnapshotMutable{},
		},
		&models.Snapshot{
			SnapshotAllOf0: models.SnapshotAllOf0{
				Meta: &models.ObjMeta{
					ID: "id4",
				},
				SnapTime:           strfmt.DateTime(now.Add(-4 * time.Hour)),
				SnapIdentifier:     "baseSnap",
				PitIdentifier:      "pitBase-3",
				ProtectionDomainID: "pd-1",
			},
			SnapshotMutable: models.SnapshotMutable{},
		},
	}

	tl.Logger().Infof("case: removeOlderPiTs")
	tl.Flush()
	op = newOp()
	op.vsSnapshots = vsSnapshots
	fOp := &fakeVolSCOps{} // to intercept deletion calls
	fOp.rhs = op.rhs
	op.ops = fOp
	op.pits = []string{pitUUID, "pitBull", "pitBase-1", "pitBase-2"}
	op.removeOlderPiTs(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal([]string{"pitBase-2", "pitBase-1", "pitBull"}, fOp.inDPPiTs)

	tl.Logger().Infof("case: removeOlderPiTs (error)")
	tl.Flush()
	op = newOp()
	op.vsSnapshots = vsSnapshots
	fOp = &fakeVolSCOps{} // to intercept deletion calls
	fOp.rhs = op.rhs
	fOp.retDPInError = true
	op.ops = fOp
	op.pits = []string{pitUUID, "pitBull", "pitBase-1", "pitBase-2"}
	op.removeOlderPiTs(ctx)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal([]string{"pitBase-2"}, fOp.inDPPiTs)

	tl.Logger().Infof("case: removeOlderPiTs (planOnly)")
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.vsSnapshots = vsSnapshots
	op.pits = []string{pitUUID, "pitBull", "pitBase-1", "pitBase-2"}
	op.removeOlderPiTs(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(3, len(op.rhs.Request.RequestMessages))
	for i, pit := range []string{"pitBase-2", "pitBase-1", "pitBull"} {
		assert.Regexp(pit, op.rhs.Request.RequestMessages[i], "%d %s", i, pit)
	}

	//  ***************************** snapIsMounted
	vsr = vsrClone(vsrObj)

	tl.Logger().Infof("case: snapIsMounted")
	tl.Flush()
	op = newOp()
	assert.False(op.snapIsMounted("snap"))
	op.rhs.VolumeSeries.Mounts = []*models.Mount{{SnapIdentifier: "snap"}}
	assert.False(op.snapIsMounted("snap"))
	op.rhs.VolumeSeries.Mounts = []*models.Mount{{SnapIdentifier: "snap", MountState: "MOUNTED"}}
	assert.True(op.snapIsMounted("snap"))

	//  ***************************** lmdGuardLock / lmdGuardUnlock

	tl.Logger().Infof("case: lmdGuardLock")
	tl.Flush()
	op = newOp()
	op.lmdGuardLock(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(op.lmdCST)

	tl.Logger().Infof("case: lmdGuardUnlock")
	tl.Flush()
	op.lmdGuardUnlock(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Nil(op.lmdCST)

	tl.Logger().Infof("case: lmdGuardUnlock (no-op)")
	tl.Flush()
	op.lmdCST = nil
	op.lmdGuardUnlock(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	tl.Logger().Infof("case: lmdGuardLock (error)")
	tl.Flush()
	app.LMDGuard.Drain()
	op = newOp()
	op.lmdGuardLock(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Nil(op.lmdCST)
	assert.Regexp("lock error", op.rhs.Request.RequestMessages[0])
	app.LMDGuard = util.NewCriticalSectionGuard() // restore

	tl.Logger().Infof("case: lmdGuardLock (plan only)")
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.lmdGuardLock(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Nil(op.lmdCST)

	//  ***************************** syncFail
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "CREATING_PIT"

	tl.Logger().Infof("case: syncFail: cancelled")
	tl.Flush()
	op = newOp()
	op.rhs.Request.SyncCoordinatorID = "" // no need to actually do anything
	op.rhs.Canceling = true
	assert.False(op.advertisedFailure)
	op.syncFail(ctx)
	assert.NotNil(op.saa)
	assert.EqualValues(op.rhs.Request.VolumeSeriesID, op.saa.LocalKey)
	assert.Equal("CANCELED", op.saa.LocalState)
	assert.True(op.advertisedFailure)

	tl.Logger().Infof("case: syncFail: failed")
	tl.Flush()
	op = newOp()
	op.rhs.Request.SyncCoordinatorID = "" // no need to actually do anything
	assert.False(op.advertisedFailure)
	op.syncFail(ctx)
	assert.NotNil(op.saa)
	assert.EqualValues(op.rhs.Request.VolumeSeriesID, op.saa.LocalKey)
	assert.Equal("FAILED", op.saa.LocalState)
	assert.True(op.advertisedFailure)

	tl.Logger().Infof("case: syncFail: already called")
	tl.Flush()
	op = newOp()
	op.advertisedFailure = true
	op.syncFail(ctx)
	assert.Nil(op.saa)

	//  ***************************** syncState
	syncStates := map[string]bool{
		"PAUSED_IO":            false,
		"CREATED_PIT":          false,
		"SNAPSHOT_UPLOAD_DONE": true,
		"FINALIZING_SNAPSHOT":  true,
	}
	for state, adjustCB := range syncStates {
		vsr = vsrClone(vsrObj)
		vsr.VolumeSeriesRequestState = state
		tB := time.Now()
		vsr.CompleteByTime = strfmt.DateTime(tB.Add(-25 * time.Second)) // in the past to force an error
		tl.Logger().Infof("case: syncState %s", state)
		tl.Flush()
		op = newOp()
		op.syncState(ctx, adjustCB)
		tA := time.Now()
		assert.False(op.rhs.RetryLater)
		assert.True(op.rhs.InError)
		assert.Regexp("Sync error invalid CompleteBy", op.rhs.Request.RequestMessages[0])
		assert.NotNil(op.sa)
		expSA := &vra.SyncArgs{
			LocalKey:   string(vsr.VolumeSeriesID),
			SyncState:  state,
			CompleteBy: time.Time(op.rhs.Request.CompleteByTime), // use timestamp from clone
		}
		if state == "FINALIZING_SNAPSHOT" {
			expSA.CoordinatorStateOnSync = "CG_SNAPSHOT_FINALIZE"
		}
		if adjustCB {
			origDur := time.Time(op.rhs.Request.CompleteByTime).Sub(time.Time(op.rhs.Request.Meta.TimeCreated))
			assert.WithinDuration(tB.Add(origDur), op.sa.CompleteBy, tA.Sub(tB), state)
			op.sa.CompleteBy = expSA.CompleteBy // to facilitate the match
		}
		assert.Equal(expSA, op.sa, state)

		tl.Logger().Infof("case: syncCurrentState %s (planOnly)", state)
		tl.Flush()
		op = newOp()
		op.planOnly = true
		op.syncState(ctx, adjustCB)
		assert.False(op.rhs.RetryLater)
		assert.False(op.rhs.InError)
		assert.Regexp(state+" sync", op.rhs.Request.RequestMessages[0])
	}

	//  ***************************** timestampPiT
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "CREATING_PIT"

	tl.Logger().Infof("case: timestampPiT %s  (lastHeadUnexportTime not set)", vsr.VolumeSeriesRequestState)
	tl.Flush()
	fcResetVSRUpdater()
	t0 := time.Now()
	op = newOp()
	op.rhs.VolumeSeries.SystemTags = nil
	op.rhs.Request.SystemTags = nil
	assert.NotZero(swag.Int64Value(op.rhs.VolumeSeries.SizeBytes))
	op.timestampPiT(ctx)
	t1 := time.Now()
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Contains(fc.InVSRUpdaterItems.Set, "snapshot")
	sRec = op.rhs.Request.Snapshot
	assert.Equal(pitUUID, sRec.PitIdentifier)
	assert.Equal(snapID, sRec.SnapIdentifier)
	assert.Equal(swag.Int64Value(op.rhs.VolumeSeries.SizeBytes), swag.Int64Value(sRec.SizeBytes))
	assert.True(t0.Before(time.Time(sRec.SnapTime)))
	assert.True(t1.After(time.Time(sRec.SnapTime)))
	assert.Equal(vsObj.ConsistencyGroupID, sRec.ConsistencyGroupID)
	assert.Empty(op.rhs.Request.SystemTags)

	tl.Logger().Infof("case: timestampPiT %s (lastHeadUnexportTime set)", vsr.VolumeSeriesRequestState)
	tl.Flush()
	fcResetVSRUpdater()
	t0 = time.Now()
	op = newOp()
	lhu = fmt.Sprintf("%s:VALUE", com.SystemTagVolumeLastHeadUnexport)
	op.rhs.VolumeSeries.SystemTags = []string{lhu}
	assert.NotZero(swag.Int64Value(op.rhs.VolumeSeries.SizeBytes))
	op.timestampPiT(ctx)
	t1 = time.Now()
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Contains(fc.InVSRUpdaterItems.Set, "snapshot")
	sRec = op.rhs.Request.Snapshot
	assert.Equal(pitUUID, sRec.PitIdentifier)
	assert.Equal(snapID, sRec.SnapIdentifier)
	assert.Equal(swag.Int64Value(op.rhs.VolumeSeries.SizeBytes), swag.Int64Value(sRec.SizeBytes))
	assert.True(t0.Before(time.Time(sRec.SnapTime)))
	assert.True(t1.After(time.Time(sRec.SnapTime)))
	assert.Equal(vsObj.ConsistencyGroupID, sRec.ConsistencyGroupID)
	assert.NotEmpty(op.rhs.Request.SystemTags)
	assert.EqualValues([]string{lhu}, op.rhs.Request.SystemTags)

	tl.Logger().Infof("case: timestampPiT %s (error)", vsr.VolumeSeriesRequestState)
	tl.Flush()
	fcResetVSRUpdater()
	fc.RetVSRUpdaterErr = fmt.Errorf("updater-error")
	t0 = time.Now()
	op = newOp()
	op.timestampPiT(ctx)
	t1 = time.Now()
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Contains(fc.InVSRUpdaterItems.Set, "snapshot")
	assert.True(t0.Before(time.Time(op.rhs.Request.Snapshot.SnapTime)))
	assert.True(t1.After(time.Time(op.rhs.Request.Snapshot.SnapTime)))

	tl.Logger().Infof("case: timestampPiT %s (plan only)", vsr.VolumeSeriesRequestState)
	tl.Flush()
	fcResetVSRUpdater()
	fc.RetVSRUpdaterErr = fmt.Errorf("updater-error")
	op = newOp()
	op.planOnly = true
	op.timestampPiT(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("Record PiT timestamp", op.rhs.Request.RequestMessages[0])

	//  ***************************** unexportTip
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "UNDO_SNAPSHOT_UPLOADING"

	tl.Logger().Infof("case: unexportTip %s", vsr.VolumeSeriesRequestState)
	tl.Flush()
	fmReset()
	op = newOp()
	op.tipSnapID = "tipSnapID"
	op.rhs.Request.Snapshot = &models.SnapshotData{
		PitIdentifier: "pitID",
	}
	op.unexportTip(ctx)
	assert.False(op.rhs.InError)
	assert.Equal(com.VolReqStateUndoVolumeExport, fm.ueVSR.VolumeSeriesRequestState)
	assert.NotNil(fm.stash)
	sv = op.rhs.StashGet(exportInternalCallStashKey{})
	assert.NotNil(sv)
	eOp, ok = sv.(*exportInternalArgs)
	assert.True(ok)
	assert.Equal(eOp.snapID, op.tipSnapID)
	assert.Equal(eOp.pitUUID, op.rhs.Request.Snapshot.PitIdentifier)
	fm.ueVSR.SnapIdentifier = op.rhs.Request.SnapIdentifier
	fm.ueVSR.VolumeSeriesRequestState = op.rhs.Request.VolumeSeriesRequestState
	assert.Equal(op.rhs.Request, fm.ueVSR)
	assert.Equal("UNDO_SNAPSHOT_UPLOADING", op.rhs.Request.VolumeSeriesRequestState)

	tl.Logger().Infof("case: unexportTip (in error) %s", vsr.VolumeSeriesRequestState)
	tl.Flush()
	fmReset()
	fm.retUEInError = true
	op = newOp()
	op.tipSnapID = "tipSnapID"
	op.rhs.Request.Snapshot = &models.SnapshotData{
		PitIdentifier: "pitID",
	}
	op.unexportTip(ctx)
	assert.True(op.rhs.InError)
	assert.Equal(com.VolReqStateUndoVolumeExport, fm.ueVSR.VolumeSeriesRequestState)
	assert.NotNil(fm.stash)
	sv = op.rhs.StashGet(exportInternalCallStashKey{})
	assert.NotNil(sv)
	eOp, ok = sv.(*exportInternalArgs)
	assert.True(ok)
	assert.Equal(eOp.snapID, op.tipSnapID)
	assert.Equal(eOp.pitUUID, op.rhs.Request.Snapshot.PitIdentifier)
	assert.Equal("UNDO_SNAPSHOT_UPLOADING", op.rhs.Request.VolumeSeriesRequestState)

	tl.Logger().Infof("case: unexportTip (planOnly) %s", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.tipSnapID = "tipSnapID"
	op.unexportTip(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("Unexport LUN for Snapshot", op.rhs.Request.RequestMessages[0])

	//  ***************************** createSnapshotObject
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "FINALIZING_SNAPSHOT"

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)

	tl.Logger().Infof("case: createSnapshotObject (success)")
	tl.Flush()
	op = newOp()
	op.spObj = spObj
	op.rhs.Request.Snapshot = vsrSRec
	op.rhs.Request.Snapshot.SizeBytes = swag.Int64(100000)
	assert.NotEmpty(op.rhs.Request.ProtectionDomainID)
	op.createSnapshotObject(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(vsrSRec.PitIdentifier, fc.InCSSnapshotObj.PitIdentifier)
	assert.Equal(vsrSRec.SnapIdentifier, fc.InCSSnapshotObj.SnapIdentifier)
	assert.Equal(vsrSRec.SnapTime, fc.InCSSnapshotObj.SnapTime)
	assert.Equal(vsrSRec.DeleteAfterTime, fc.InCSSnapshotObj.DeleteAfterTime)
	for _, loc := range vsrSRec.Locations {
		assert.Contains(fc.InCSSnapshotObj.Locations, string(loc.CspDomainID))
		l := fc.InCSSnapshotObj.Locations[string(loc.CspDomainID)]
		assert.Equal(loc.CspDomainID, l.CspDomainID)
		assert.Equal(loc.CreationTime, l.CreationTime)
	}
	assert.EqualValues(op.rhs.VolumeSeries.Meta.ID, fc.InCSSnapshotObj.VolumeSeriesID)
	assert.Equal(op.rhs.Request.ProtectionDomainID, fc.InCSSnapshotObj.ProtectionDomainID)
	stags = util.NewTagList(fc.InCSSnapshotObj.SystemTags)
	val, ok = stags.Get(com.SystemTagVsrCreator)
	assert.True(ok)
	assert.Equal(string(vsr.SyncCoordinatorID), val)
	assert.Regexp("Snapshot object created", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: createSnapshotObject (object already exists -> ignore error)")
	tl.Flush()
	op = newOp()
	op.spObj = spObj
	op.rhs.Request.Snapshot = vsrSRec
	op.rhs.Request.Snapshot.SizeBytes = swag.Int64(100000)
	fc.RetCSSnapshotCreateObjErr = fake.ExistsErr
	op.createSnapshotObject(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(1, tl.CountPattern("VolumeSeriesRequest .*: snapshot object \\(PitUUID .*, SnapID .* and VS .*\\) already exists"))

	tl.Logger().Infof("case: createSnapshotObject (failure to create snapshot object)")
	tl.Flush()
	op = newOp()
	op.spObj = spObj
	op.rhs.Request.Snapshot = vsrSRec
	op.rhs.Request.Snapshot.SizeBytes = swag.Int64(100000)
	fc.RetCSSnapshotCreateObjErr = fmt.Errorf("snapshot-create-error")
	op.createSnapshotObject(ctx)
	assert.True(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Regexp("Error creating snapshot object: snapshot-create-error", op.rhs.Request.RequestMessages[0])
	assert.Equal(1, len(op.rhs.Request.RequestMessages))
}
