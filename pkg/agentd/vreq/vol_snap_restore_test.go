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

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	appServant "github.com/Nuvoloso/kontroller/pkg/agentd/fake"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/csp/aws"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	mockNuvo "github.com/Nuvoloso/kontroller/pkg/nuvoapi/mock"
	"github.com/Nuvoloso/kontroller/pkg/pstore"
	fps "github.com/Nuvoloso/kontroller/pkg/pstore/fake"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestVolSnapshotRestore(t *testing.T) {
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
	ctx := context.Background()
	c := newComponent()
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc

	snapIdentifier := "SrcSnapIdentifier"
	pitUUID := "8e0e60e2-063f-4a29-ba79-e380ab08e75d"
	snapDomID := models.ObjIDMutable("srcSnapDomain")
	sd := models.SnapshotData{
		VolumeSeriesID: "SRC-VS",
		SnapIdentifier: snapIdentifier,
		PitIdentifier:  pitUUID,
		Locations: []*models.SnapshotLocation{
			&models.SnapshotLocation{CspDomainID: snapDomID},
		},
	}
	vsr := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VR-1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			ClusterID:         "CL-1",
			SyncCoordinatorID: "sync-1",
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID:         "NODE-1",
				VolumeSeriesID: "VS-1",
				Snapshot:       &sd,
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
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				ConsistencyGroupID: "CG-1",
			},
		},
	}

	newFakeVolSnapRestoreOp := func(state string) *fakeVolSROps {
		op := &fakeVolSROps{}
		op.c = c
		op.ops = op
		op.rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr), VolumeSeries: vsClone(vsObj)}
		op.rhs.Request.VolumeSeriesRequestState = state
		return op
	}

	var op *fakeVolSROps
	var expCalled []string

	tl.Logger().Info("Case: SNAPSHOT_RESTORE ok")
	tl.Flush()
	op = newFakeVolSnapRestoreOp("SNAPSHOT_RESTORE")
	op.retGIS = VolSRDisableMetrics
	expCalled = []string{"GIS", "DM", "PsD"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: SNAPSHOT_RESTORE fail")
	tl.Flush()
	op = newFakeVolSnapRestoreOp("SNAPSHOT_RESTORE")
	op.retGIS = VolSRDisableMetrics
	op.retPSInError = true
	expCalled = []string{"GIS", "DM", "PsD", "sFAIL"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: SNAPSHOT_RESTORE_FINALIZE ok (!HasAttachFs)")
	tl.Flush()
	op = newFakeVolSnapRestoreOp("SNAPSHOT_RESTORE_FINALIZE")
	op.retGIS = VolSRRecordOriginInVolume
	expCalled = []string{"GIS", "ROiV", "FSYNC", "UH"}
	tl.Logger().Infof("* enter: %s (no ATTACH_FS)", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: SNAPSHOT_RESTORE_FINALIZE ok (HasAttachFs")
	tl.Flush()
	op = newFakeVolSnapRestoreOp("SNAPSHOT_RESTORE_FINALIZE")
	op.retGIS = VolSRRecordOriginInVolume
	op.rhs.HasAttachFs = true
	expCalled = []string{"GIS", "ROiV", "FSYNC", "EM"}
	tl.Logger().Infof("* enter: %s (with ATTACH_FS)", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: UNDO_SNAPSHOT_RESTORE")
	tl.Flush()
	op = newFakeVolSnapRestoreOp("UNDO_SNAPSHOT_RESTORE")
	op.inError = true
	op.retGIS = VolSRUndoDisableMetrics
	expCalled = []string{"GIS", "EM"}
	tl.Logger().Infof("* enter: %s", op.rhs.Request.VolumeSeriesRequestState)
	tl.Flush()
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	// invoke the real handlers

	tl.Logger().Info("Case: real handler, no history")
	tl.Flush()
	fc.PassThroughUVRObj = true
	rhs := &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}
	c.VolSnapshotRestore(nil, rhs)
	assert.False(rhs.InError)

	tl.Logger().Info("Case: real handler, with history")
	tl.Flush()
	rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}
	rhs.StashSet(volSnapRestoreStashKey{}, &volSnapRestoreOp{})
	c.VolSnapshotRestore(nil, rhs)
	assert.False(rhs.InError)

	tl.Logger().Info("Case: real undo handler, no history")
	tl.Flush()
	rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}
	rhs.Request.VolumeSeriesRequestState = "UNDO_SNAPSHOT_RESTORE"
	rhs.Request.PlanOnly = swag.Bool(true)
	rhs.InError = true
	c.UndoVolSnapshotRestore(nil, rhs)
	assert.True(rhs.InError)

	// check state strings exist up to VolSCError
	var ss volSnapRestoreSubState
	for ss = VolSRDownloadSnapshot; ss < VolSRNoOp; ss++ {
		s := ss.String()
		tl.Logger().Debugf("Testing %d %s", ss, s)
		assert.Regexp("^VolSR", s)
	}
	assert.Regexp("^volSnapRestoreSubState", ss.String())
}

type fakeVolSROps struct {
	volSnapRestoreOp
	called       []string
	retGIS       volSnapRestoreSubState
	retPSInError bool
}

func (op *fakeVolSROps) getInitialState(ctx context.Context) volSnapRestoreSubState {
	op.called = append(op.called, "GIS")
	return op.retGIS
}
func (op *fakeVolSROps) psDownload(ctx context.Context) {
	op.called = append(op.called, "PsD")
	op.rhs.InError = op.retPSInError
}
func (op *fakeVolSROps) recordOriginInVolume(ctx context.Context) {
	op.called = append(op.called, "ROiV")
}
func (op *fakeVolSROps) syncFail(ctx context.Context) {
	op.called = append(op.called, "sFAIL")
}
func (op *fakeVolSROps) syncState(ctx context.Context) {
	op.called = append(op.called, "FSYNC")
}
func (op *fakeVolSROps) unexportHeadNoSnapshot(ctx context.Context) {
	op.called = append(op.called, "UH")
}

func (op *fakeVolSROps) disableMetrics(ctx context.Context, disable bool) {
	if disable {
		op.called = append(op.called, "DM")
	} else {
		op.called = append(op.called, "EM")
	}
}

func TestVolSnapshotRestoreSteps(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log: tl.Logger(),
		},
	}
	appS := &appServant.AppServant{}
	app.AppServant = appS
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	ctx := context.Background()
	c := newComponent()
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc
	fsu := &fakeStateUpdater{}
	app.StateUpdater = fsu
	fPSO := &fps.Controller{}
	app.PSO = fPSO
	app.NuvoVolDirPath = "/nuvo-vol-dir"

	fm := &vscFakeMounter{}
	fmReset := func() {
		fm.eVS = nil
		fm.eVSR = nil
		fm.retEInError = false
		fm.ueVS = nil
		fm.ueVSR = nil
		fm.retUEInError = false
	}

	now := time.Now()
	cbDur := 30 * time.Minute
	snapVol := models.ObjIDMutable("SrcVol")
	snapIdentifier := "SrcSnapIdentifier"
	pitUUID := "8e0e60e2-063f-4a29-ba79-e380ab08e75d"
	snapDomID := models.ObjIDMutable("srcSnapDomain")
	sd := models.SnapshotData{
		VolumeSeriesID:     snapVol,
		SnapIdentifier:     snapIdentifier,
		PitIdentifier:      pitUUID,
		ProtectionDomainID: "PD-1",
		Locations: []*models.SnapshotLocation{
			&models.SnapshotLocation{CspDomainID: snapDomID},
		},
	}
	vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:          "VR-1",
				TimeCreated: strfmt.DateTime(now),
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			ClusterID:          "CL-1",
			ProtectionDomainID: "PD-1",
			SyncCoordinatorID:  "sync-1",
			CompleteByTime:     strfmt.DateTime(now.Add(cbDur)),
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID:         "NODE-1",
				VolumeSeriesID: "VS-1",
				Snapshot:       &sd,
			},
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				MountedNodeDevice: "mnd",
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
	assert.NotNil(parentVSR)
	domObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta:          &models.ObjMeta{ID: models.ObjID(snapDomID)},
			CspDomainType: "AWS",
		},
		CSPDomainMutable: models.CSPDomainMutable{},
	}
	domObj.CspDomainAttributes = map[string]models.ValueType{ // required for pstore validators
		aws.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: "bn"},
		aws.AttrRegion:           models.ValueType{Kind: "STRING", Value: "rg"},
		aws.AttrAccessKeyID:      models.ValueType{Kind: "STRING", Value: "ak"},
		aws.AttrSecretAccessKey:  models.ValueType{Kind: "SECRET", Value: "sk"},
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

	newOp := func() *volSnapRestoreOp {
		op := &volSnapRestoreOp{}
		op.c = c
		op.mops = fm
		op.ops = op
		op.rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr), VolumeSeries: vsClone(vsObj)}
		return op
	}
	var op *volSnapRestoreOp
	var err error

	//  ***************************** getInitialState
	entryStates := []struct {
		label string
		state volSnapRestoreSubState
	}{
		{"SNAPSHOT_RESTORE", VolSRDisableMetrics},
		{"SNAPSHOT_RESTORE_FINALIZE", VolSRRecordOriginInVolume},
		{"UNDO_SNAPSHOT_RESTORE", VolSRUndoDisableMetrics},
	}
	for _, tc := range entryStates {
		vsr.VolumeSeriesRequestState = tc.label
		tl.Logger().Infof("case: getInitialState %s", vsr.VolumeSeriesRequestState)
		tl.Flush()
		op = newOp()
		op.rhs.VolumeSeries.VolumeSeriesState = com.VolStateInUse
		op.rhs.VolumeSeries.Mounts = []*models.Mount{
			&models.Mount{SnapIdentifier: "HEAD"},
		}
		assert.Equal(tc.state, op.getInitialState(ctx), "%s", tc.label)
		assert.False(op.rhs.RetryLater)
		assert.False(op.rhs.InError)
	}

	tl.Logger().Infof("case: getInitialState %s - invalid vol state", com.VolReqStateSnapshotRestore)
	tl.Flush()
	op = newOp()
	op.rhs.VolumeSeries.VolumeSeriesState = com.VolStateConfigured
	op.rhs.Request.VolumeSeriesRequestState = com.VolReqStateSnapshotRestore
	assert.Equal(VolSRDone, op.getInitialState(ctx))
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Regexp("Invalid volume state.*CONFIGURED", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: getInitialState %s - HEAD not mounted", com.VolReqStateSnapshotRestore)
	tl.Flush()
	op = newOp()
	op.rhs.VolumeSeries.VolumeSeriesState = com.VolStateInUse
	op.rhs.VolumeSeries.Mounts = []*models.Mount{
		&models.Mount{SnapIdentifier: "nothead"},
	}
	op.rhs.Request.VolumeSeriesRequestState = com.VolReqStateSnapshotRestore
	assert.Equal(VolSRDone, op.getInitialState(ctx))
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Regexp("HEAD not mounted", op.rhs.Request.RequestMessages[0])

	//  ***************************** psDownload / psDownloadFromPStore

	tl.Logger().Infof("case: psDownloadFromPStore: rei-error")
	tl.Flush()
	op = newOp()
	c.rei.SetProperty("vsc-download-error", &rei.Property{BoolValue: true})
	res, err := op.psDownloadFromPStore(ctx, snapDomID)
	assert.Error(err)
	assert.Regexp("vsc-download-error", err)
	assert.Nil(res)

	tl.Logger().Infof("case: psDownloadFromPStore: dom fetch error")
	tl.Flush()
	fc.RetLDErr = fmt.Errorf("csp-domain-fetch")
	op = newOp()
	res, err = op.psDownloadFromPStore(ctx, snapDomID)
	assert.Error(err)
	assert.Regexp("csp-domain-fetch", err)
	assert.Nil(res)
	assert.True(op.rhs.RetryLater)

	tl.Logger().Infof("case: psDownloadFromPStore: restore error")
	tl.Flush()
	mockCtrl := gomock.NewController(t)
	fc.RetLDErr = nil
	fc.RetLDObj = domObj
	fPSO.RetSRErr = fmt.Errorf("snap-restore")
	fPSO.RetSRObj = nil
	fsu.rCalled = 0
	op = newOp()
	op.pdObj = pdObj
	nvAPI := mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().LunPath(string(op.c.App.NuvoVolDirPath), op.rhs.Request.MountedNodeDevice).Return("destPath").Times(1)
	op.c.App.NuvoAPI = nvAPI
	res, err = op.psDownloadFromPStore(ctx, snapDomID)
	assert.Error(err)
	assert.Regexp("snap-restore", err)
	assert.Equal(fPSO.RetSRObj, res)
	assert.Equal(1, fsu.rCalled)
	mockCtrl.Finish()

	tl.Logger().Infof("case: psDownload: PSO retryable error")
	tl.Flush()
	fc.InPDFetchID = ""
	fc.RetPDFetchObj = pdObj
	fc.RetPDFetchErr = nil
	mockCtrl = gomock.NewController(t)
	fPSO.RetSRErr = pstore.NewRetryableError("retryable-error")
	fPSO.RetSRObj = nil
	fsu.rCalled = 0
	op = newOp()
	assert.NotEmpty(op.rhs.Request.ProtectionDomainID)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().LunPath(string(op.c.App.NuvoVolDirPath), op.rhs.Request.MountedNodeDevice).Return("destPath").Times(1)
	op.c.App.NuvoAPI = nvAPI
	op.snapIdentifier = snapIdentifier
	op.psDownload(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(1, fsu.rCalled)
	assert.Equal(pdObj, op.pdObj)
	mockCtrl.Finish()

	tl.Logger().Infof("case: psDownload: PSO other error")
	tl.Flush()
	fc.InPDFetchID = ""
	fc.RetPDFetchObj = pdObj
	fc.RetPDFetchErr = nil
	mockCtrl = gomock.NewController(t)
	fPSO.RetSRErr = fmt.Errorf("pso-error")
	fPSO.RetSRObj = nil
	fsu.rCalled = 0
	op = newOp()
	assert.NotEmpty(op.rhs.Request.ProtectionDomainID)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().LunPath(string(op.c.App.NuvoVolDirPath), op.rhs.Request.MountedNodeDevice).Return("destPath").Times(1)
	op.c.App.NuvoAPI = nvAPI
	op.snapIdentifier = snapIdentifier
	op.psDownload(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Equal(1, fsu.rCalled)
	assert.Equal(pdObj, op.pdObj)
	mockCtrl.Finish()

	tl.Logger().Infof("case: psDownload: dom fetch error")
	tl.Flush()
	fc.InPDFetchID = ""
	fc.RetPDFetchObj = pdObj
	fc.RetPDFetchErr = nil
	fc.RetLDObj = nil
	fc.RetLDErr = fmt.Errorf("csp-domain-fetch")
	op = newOp()
	assert.NotEmpty(op.rhs.Request.ProtectionDomainID)
	op.snapIdentifier = snapIdentifier
	op.psDownload(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(pdObj, op.pdObj)

	tl.Logger().Infof("case: psDownload: pd fetch error")
	tl.Flush()
	fc.InPDFetchID = ""
	fc.RetPDFetchObj = nil
	fc.RetPDFetchErr = fmt.Errorf("pd-fetch")
	op = newOp()
	assert.NotEmpty(op.rhs.Request.ProtectionDomainID)
	op.psDownload(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Nil(op.pdObj)

	tl.Logger().Infof("case: psDownload: planOnly")
	tl.Flush()
	op = newOp()
	op.snapIdentifier = snapIdentifier
	op.planOnly = true
	op.psDownload(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp(snapIdentifier, op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: psDownload: ok")
	tl.Flush()
	fc.InPDFetchID = ""
	fc.RetPDFetchObj = pdObj
	fc.RetPDFetchErr = nil
	mockCtrl = gomock.NewController(t)
	fc.RetLDErr = nil
	fc.RetLDObj = domObj
	fPSO.RetSRErr = nil
	fPSO.RetSRObj = &pstore.SnapshotRestoreResult{}
	fsu.rCalled = 0
	op = newOp()
	op.snapIdentifier = snapIdentifier
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().LunPath(string(op.c.App.NuvoVolDirPath), op.rhs.Request.MountedNodeDevice).Return("destPath").Times(1)
	op.c.App.NuvoAPI = nvAPI
	op.psDownload(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	sra := fPSO.InSRArg
	assert.True(sra.Validate())
	assert.Equal(op.rhs.Request.Snapshot.SnapIdentifier, sra.SnapIdentifier)
	assert.EqualValues(op.rhs.Request.Meta.ID, sra.ID)
	assert.EqualValues(domObj.CspDomainType, sra.PStore.CspDomainType)
	assert.EqualValues(domObj.CspDomainAttributes, sra.PStore.CspDomainAttributes)
	assert.Equal(pitUUID, sra.SourceSnapshot)
	assert.Equal("destPath", sra.DestFile)
	assert.Equal(pdObj.EncryptionAlgorithm, sra.EncryptionAlgorithm)
	assert.Equal(pdObj.EncryptionPassphrase.Value, sra.Passphrase)
	assert.Equal(string(pdObj.Meta.ID), sra.ProtectionDomainID)
	assert.Equal(op.rhs, sra.Reporter)
	assert.Regexp("Downloaded snapshot", op.rhs.Request.RequestMessages[0])
	assert.Regexp(snapIdentifier, op.rhs.Request.RequestMessages[0])
	assert.Equal(0, fsu.rCalled)
	assert.Equal(pdObj, op.pdObj)
	mockCtrl.Finish()

	//  ***************************** recordOriginInVolume

	tl.Logger().Infof("case: recordOriginInVolume: planOnly")
	tl.Flush()
	fc.RetVSUpdaterErr = fmt.Errorf("updater-err")
	op = newOp()
	op.snapIdentifier = snapIdentifier
	op.planOnly = true
	op.recordOriginInVolume(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("Insert origin message in volume", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: recordOriginInVolume: updater error")
	tl.Flush()
	fc.RetVSUpdaterErr = fmt.Errorf("updater-err")
	op = newOp()
	op.snapIdentifier = snapIdentifier
	op.recordOriginInVolume(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("updater-err", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: recordOriginInVolume: ok")
	tl.Flush()
	fc.RetVSUpdaterErr = nil
	fc.InVSUpdaterID = ""
	fc.InVSUpdaterItems = nil
	op = newOp()
	op.rhs.VolumeSeries.SystemTags = []string{fmt.Sprintf("%s:XXX", com.SystemTagVsrRestoring)}
	op.snapIdentifier = snapIdentifier
	op.recordOriginInVolume(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(&crud.Updates{Set: []string{"messages", "systemTags"}}, fc.InVSUpdaterItems)
	assert.EqualValues(op.rhs.VolumeSeries.Meta.ID, fc.InVSUpdaterID)
	assert.Equal(op.rhs.VolumeSeries, fc.ModVSUpdaterObj)
	assert.Regexp("Initialized from Volume", op.rhs.VolumeSeries.Messages[0])
	assert.Equal(2, len(op.rhs.VolumeSeries.SystemTags))
	st := fmt.Sprintf("%s:%s", com.SystemTagVolumeRestoredSnap, op.snapIdentifier)
	assert.Equal(st, op.rhs.VolumeSeries.SystemTags[0])
	st = fmt.Sprintf("%s:%s", com.SystemTagVsrRestored, op.rhs.Request.Meta.ID)
	assert.Equal(st, op.rhs.VolumeSeries.SystemTags[1])

	//  ***************************** syncFail
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "SNAPSHOT_RESTORE"

	tl.Logger().Infof("case: syncFail: cancelled")
	tl.Flush()
	op = newOp()
	op.snapIdentifier = snapIdentifier
	op.rhs.Request.SyncCoordinatorID = "" // no need to actually do anything
	op.rhs.Canceling = true
	assert.False(op.advertisedFailure)
	op.syncFail(ctx)
	assert.NotNil(op.saa)
	assert.EqualValues(snapIdentifier, op.saa.LocalKey)
	assert.Equal("CANCELED", op.saa.LocalState)
	assert.True(op.advertisedFailure)

	tl.Logger().Infof("case: syncFail: failed")
	tl.Flush()
	op = newOp()
	op.snapIdentifier = snapIdentifier
	op.rhs.Request.SyncCoordinatorID = "" // no need to actually do anything
	assert.False(op.advertisedFailure)
	op.syncFail(ctx)
	assert.NotNil(op.saa)
	assert.EqualValues(snapIdentifier, op.saa.LocalKey)
	assert.Equal("FAILED", op.saa.LocalState)
	assert.True(op.advertisedFailure)

	tl.Logger().Infof("case: syncFail: already called")
	tl.Flush()
	op = newOp()
	op.advertisedFailure = true
	op.syncFail(ctx)
	assert.Nil(op.saa)

	//  ***************************** syncState
	syncStates := []string{
		"SUCCEEDED",
	}
	for _, state := range syncStates {
		vsr = vsrClone(vsrObj)
		vsr.VolumeSeriesRequestState = state
		vsr.SyncCoordinatorID = "" // make the Sync a no-op
		tl.Logger().Infof("case: syncState %s", state)
		tl.Flush()
		op = newOp()
		op.snapIdentifier = snapIdentifier
		tBefore := time.Now()
		op.syncState(ctx)
		tAfter := time.Now()
		assert.False(op.rhs.RetryLater)
		assert.False(op.rhs.InError)
		assert.NotNil(op.sa)
		assert.True(tBefore.Add(cbDur).Before(op.sa.CompleteBy)) // bracket
		assert.True(tAfter.Add(cbDur).After(op.sa.CompleteBy))   // bracket
		expSA := &vra.SyncArgs{
			LocalKey:               snapIdentifier,
			SyncState:              state,
			CoordinatorStateOnSync: "SNAPSHOT_RESTORE_DONE",
			CompleteBy:             op.sa.CompleteBy,
		}
		assert.Equal(expSA, op.sa)
		assert.True(vra.VolumeSeriesRequestStateIsTerminated(expSA.SyncState))

		tl.Logger().Infof("case: syncCurrentState %s (planOnly)", state)
		tl.Flush()
		op = newOp()
		op.snapIdentifier = snapIdentifier
		op.planOnly = true
		op.syncState(ctx)
		assert.False(op.rhs.RetryLater)
		assert.False(op.rhs.InError)
		assert.Regexp("sync", op.rhs.Request.RequestMessages[0])

		tl.Logger().Infof("case: syncCurrentState %s (syncError)", state)
		tl.Flush()
		op = newOp()
		vsr.VolumeSeriesRequestState = state
		vsr.CompleteByTime = strfmt.DateTime(time.Now().Add(-5 * time.Second)) // in the past to force an error
		op.syncState(ctx)
		assert.False(op.rhs.RetryLater)
		assert.True(op.rhs.InError)
	}

	//  ***************************** unexportHeadNoSnapshot
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "SNAPSHOT_RESTORE_FINALIZE"

	tl.Logger().Infof("case: unexportHeadNoSnapshot")
	tl.Flush()
	fmReset()
	op = newOp()
	op.unexportHeadNoSnapshot(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(com.VolReqStateUndoVolumeExport, fm.ueVSR.VolumeSeriesRequestState)
	assert.NotNil(fm.stash)
	sv := op.rhs.StashGet(exportInternalCallStashKey{})
	assert.NotNil(sv)
	eOp, ok := sv.(*exportInternalArgs)
	assert.True(ok)
	assert.Equal("HEAD", eOp.snapID)
	fm.ueVSR.SnapIdentifier = op.rhs.Request.SnapIdentifier
	fm.ueVSR.VolumeSeriesRequestState = op.rhs.Request.VolumeSeriesRequestState
	assert.Equal(op.rhs.Request, fm.ueVSR)
	assert.Equal("SNAPSHOT_RESTORE_FINALIZE", op.rhs.Request.VolumeSeriesRequestState)

	tl.Logger().Infof("case: unexportHeadNoSnapshot (error ignored)")
	tl.Flush()
	op.rhs.StashSet(exportInternalCallStashKey{}, nil)
	fmReset()
	fm.retUEInError = true
	op = newOp()
	op.unexportHeadNoSnapshot(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(com.VolReqStateUndoVolumeExport, fm.ueVSR.VolumeSeriesRequestState)
	assert.NotNil(fm.stash)
	sv = op.rhs.StashGet(exportInternalCallStashKey{})
	assert.NotNil(sv)
	eOp, ok = sv.(*exportInternalArgs)
	assert.True(ok)
	assert.Equal("HEAD", eOp.snapID)
	assert.Equal("SNAPSHOT_RESTORE_FINALIZE", op.rhs.Request.VolumeSeriesRequestState)

	tl.Logger().Infof("case: unexportHeadNoSnapshot (planOnly)")
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.unexportHeadNoSnapshot(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("Unexport HEAD no snapshot", op.rhs.Request.RequestMessages[0])

	// **************************** disableMetrics
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "SNAPSHOT_RESTORE"
	lun := &agentd.LUN{
		DisableMetrics: false,
	}
	tl.Logger().Infof("case: disableMetrics (all ops)")
	tl.Flush()
	fmReset()
	appS.InFLvsID = ""
	appS.InFLSnapID = ""
	appS.RetFLObj = lun
	app.AppServant = appS
	op = newOp()
	op.c.App = app
	op.rhs.HasMount = true
	op.rhs.HasAttachFs = true
	op.disableMetrics(ctx, true)
	assert.EqualValues(vsrObj.VolumeSeriesID, appS.InFLvsID)
	assert.EqualValues("HEAD", appS.InFLSnapID)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.True(lun.DisableMetrics)

	tl.Logger().Infof("case: Enable metrics (no mount, no attach_fs)")
	tl.Flush()
	lun.DisableMetrics = true
	appS.InFLvsID = ""
	appS.InFLSnapID = ""
	appS.RetFLObj = lun
	app.AppServant = appS
	op = newOp()
	op.c.App = app
	op.disableMetrics(ctx, false)
	assert.False(lun.DisableMetrics)

	tl.Logger().Infof("case: Enable metrics ignored when RHS has standalone mount")
	tl.Flush()
	lun.DisableMetrics = false
	appS.InFLvsID = ""
	appS.InFLSnapID = ""
	appS.RetFLObj = lun
	app.AppServant = appS
	op = newOp()
	op.c.App = app
	op.rhs.HasMount = true
	op.disableMetrics(ctx, false)
	assert.True(lun.DisableMetrics)

	tl.Logger().Infof("case: Enable metrics when RHS has attach_fs (even with mount)")
	tl.Flush()
	lun.DisableMetrics = true
	appS.InFLvsID = ""
	appS.InFLSnapID = ""
	appS.RetFLObj = lun
	app.AppServant = appS
	op = newOp()
	op.c.App = app
	op.rhs.HasMount = true
	op.rhs.HasAttachFs = true
	op.disableMetrics(ctx, false)
	assert.False(lun.DisableMetrics)

	tl.Logger().Infof("case: disableMetrics (lun object not found)")
	tl.Flush()
	lun = nil
	appS.InFLvsID = ""
	appS.InFLSnapID = ""
	appS.RetFLObj = lun
	app.AppServant = appS
	op = newOp()
	op.c.App = app
	op.disableMetrics(ctx, false)
	assert.Equal(1, tl.CountPattern("Failed to set disable metric collection to false"))

	tl.Logger().Infof("case: disableMetrics- enable (lun object not found)")
	tl.Flush()
	lun = nil
	appS.InFLvsID = ""
	appS.InFLSnapID = ""
	appS.RetFLObj = lun
	app.AppServant = appS
	op = newOp()
	op.c.App = app
	op.disableMetrics(ctx, true)
	assert.Equal(1, tl.CountPattern("Failed to set disable metric collection to true"))

	tl.Logger().Infof("case: disableMetrics (planOnly)")
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.disableMetrics(ctx, true)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("Disable Metric collection", op.rhs.Request.RequestMessages[0])
}
