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
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	fm "github.com/Nuvoloso/kontroller/pkg/mount/fake"
	mockNuvo "github.com/Nuvoloso/kontroller/pkg/nuvoapi/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	fvra "github.com/Nuvoloso/kontroller/pkg/vra/fake"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestAttachFs(t *testing.T) {
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
	fM := &fm.Mounter{}
	c.mounter = fM
	app.NuvoVolDirPath = "/nuvo-vol-dir"

	now := time.Now()
	cbDur := 30 * time.Minute
	vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VR-1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			FsType:         "ext4",
			TargetPath:     "/var/lib/kubelet/mnt",
			ReadOnly:       false,
			CompleteByTime: strfmt.DateTime(now.Add(cbDur)),
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				VolumeSeriesID: "VS-1",
			},
		},
	}
	devName := "VS-1-HEAD"
	vsr := vsrClone(vsrObj)
	vsObj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: "VS-1",
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				Mounts: []*models.Mount{
					&models.Mount{
						SnapIdentifier:    "HEAD",
						MountState:        "MOUNTED",
						MountedNodeDevice: devName,
					},
				},
			},
		},
	}

	newOp := func() *fakeAttachFsOp {
		op := &fakeAttachFsOp{}
		op.c = c
		op.ops = op
		op.rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr), VolumeSeries: vsClone(vsObj)}
		return op
	}
	var expCalled []string
	var op *fakeAttachFsOp

	tl.Logger().Info("Case: MOUNT (not attached): AttachPreMountCheck")
	tl.Flush()
	op = newOp()
	op.retGIS = AttachFsPreMountCheck
	expCalled = []string{"GIS", "CIFM", "DM", "SVFT"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: MOUNT (attached): AttachPreMountCheck, isMounted")
	tl.Flush()
	op = newOp()
	op.retGIS = AttachFsPreMountCheck
	op.retCIFM = true
	expCalled = []string{"GIS", "CIFM", "SVFT"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: MOUNT (error): AttachFsError")
	tl.Flush()
	op = newOp()
	op.retGIS = AttachFsError
	expCalled = []string{"GIS"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: UNDO_MOUNT (attached): AttachPreUmountCheck, InError")
	tl.Flush()
	op = newOp()
	op.inError = true
	op.rhs.InError = false
	op.retGIS = AttachFsPreUmountCheck
	op.retCIFM = true
	expCalled = []string{"GIS", "CIFM", "PIO", "DU", "RIO", "CVFT"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)
	assert.True(op.rhs.InError)

	tl.Logger().Info("Case: UNMOUNT (attached): AttachPreUmountCheck")
	tl.Flush()
	op = newOp()
	op.retGIS = AttachFsPreUmountCheck
	op.retCIFM = true
	expCalled = []string{"GIS", "CIFM", "PIO", "DU", "RIO", "CVFT"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: UNMOUNT (not attached): AttachPreUmountCheck")
	tl.Flush()
	op = newOp()
	op.retGIS = AttachFsPreUmountCheck
	op.retCIFM = false
	expCalled = []string{"GIS", "CIFM", "RIO", "CVFT"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: UNMOUNT (head not mounted): AttachFsClearConditionInVolume")
	tl.Flush()
	op = newOp()
	op.retGIS = AttachFsClearConditionInVolume
	op.retCIFM = true
	expCalled = []string{"GIS", "CVFT"}
	op.run(ctx)
	assert.Equal(expCalled, op.called)

	// invoke the real handlers

	tl.Logger().Info("Case: AttachFs")
	tl.Flush()
	rhs := &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr), VolumeSeries: vsClone(vsObj)}
	rhs.VolumeSeries.Mounts = nil
	c.AttachFs(nil, rhs)
	assert.True(rhs.InError)

	tl.Logger().Info("Case: UndoAttachFs")
	tl.Flush()
	rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr), VolumeSeries: vsClone(vsObj)}
	rhs.VolumeSeries.Mounts = nil
	c.UndoAttachFs(nil, rhs) // Note: state is not UNDO_ATTACHING_FS to force error
	assert.True(rhs.InError)

	// check state strings exist up to VolSCError
	var ss attachFsSubState
	for ss = AttachFsPreMountCheck; ss < AttachFsNoOp; ss++ {
		s := ss.String()
		tl.Logger().Debugf("Testing %d %s", ss, s)
		assert.Regexp("^AttachFs", s)
	}
	assert.Regexp("^attachFsSubState", ss.String())
}

type fakeAttachFsOp struct {
	attachFsOp
	called  []string
	retGIS  attachFsSubState
	retCIFM bool
}

func (op *fakeAttachFsOp) getInitialState(ctx context.Context) attachFsSubState {
	op.called = append(op.called, "GIS")
	return op.retGIS
}

func (op *fakeAttachFsOp) checkIfFsMounted(ctx context.Context, ignoreError bool) {
	op.called = append(op.called, "CIFM")
	op.isMounted = op.retCIFM
}

func (op *fakeAttachFsOp) clearVolumeFsTag(ctx context.Context) {
	op.called = append(op.called, "CVFT")
}

func (op *fakeAttachFsOp) doMount(ctx context.Context) {
	op.called = append(op.called, "DM")
}

func (op *fakeAttachFsOp) doUnmount(ctx context.Context) {
	op.called = append(op.called, "DU")
}

func (op *fakeAttachFsOp) nuvoPauseIO(ctx context.Context) {
	op.called = append(op.called, "PIO")
}

func (op *fakeAttachFsOp) nuvoResumeIO(ctx context.Context) {
	op.called = append(op.called, "RIO")
}

func (op *fakeAttachFsOp) setVolumeFsTag(ctx context.Context) {
	op.called = append(op.called, "SVFT")
}

func TestAttachFsSteps(t *testing.T) {
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
	fM := &fm.Mounter{}
	c.mounter = fM
	app.NuvoVolDirPath = "/nuvo-vol-dir"

	now := time.Now()
	tCB := now.Add(time.Minute).Round(time.Millisecond) // rounded as strfmt loses precision
	vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VR-1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			FsType:         "ext4",
			TargetPath:     "/var/lib/kubelet/mnt",
			ReadOnly:       false,
			CompleteByTime: strfmt.DateTime(tCB),
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				VolumeSeriesID: "VS-1",
			},
		},
	}
	devName := "VS-1-HEAD"
	devPath := "/nuvo-vol-dir/VS-1-HEAD"
	vsr := vsrClone(vsrObj)
	vsObj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: "VS-1",
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				Mounts: []*models.Mount{
					&models.Mount{
						SnapIdentifier:    "HEAD",
						MountState:        "MOUNTED",
						MountedNodeDevice: devName,
					},
				},
			},
		},
	}

	newOp := func() *attachFsOp {
		op := &attachFsOp{}
		op.c = c
		op.ops = op
		op.rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr), VolumeSeries: vsClone(vsObj)}
		return op
	}
	var op *attachFsOp

	// ***************************** checkIfFsMounted
	tl.Logger().Info("case: checkIfFsMounted error ignored")
	mockCtrl := gomock.NewController(t)
	op = newOp()
	op.headDevice = devName
	op.isMounted = true
	nvAPI := mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().LunPath(string(op.c.App.NuvoVolDirPath), op.headDevice).Return(devPath).Times(1)
	op.c.App.NuvoAPI = nvAPI
	fM.RetIfmErr = fmt.Errorf("check-error")
	fM.RetIfmCmds = bytes.NewBufferString("mount\n")
	op.checkIfFsMounted(ctx, true)
	assert.False(op.isMounted)
	assert.False(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Equal("mount", op.rhs.Request.RequestMessages[0].Message)
	tl.Flush()

	tl.Logger().Info("case: checkIfFsMounted error recorded")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	op = newOp()
	op.headDevice = devName
	op.isMounted = true
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().LunPath(string(op.c.App.NuvoVolDirPath), op.headDevice).Return(devPath).Times(1)
	op.c.App.NuvoAPI = nvAPI
	fM.RetIfmErr = fmt.Errorf("check-error")
	op.checkIfFsMounted(ctx, false)
	assert.False(op.isMounted)
	assert.True(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 2)
	assert.Equal("mount", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal("check-error", op.rhs.Request.RequestMessages[1].Message)
	tl.Flush()

	tl.Logger().Info("case: checkIfFsMounted not mounted")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	op = newOp()
	op.headDevice = devName
	op.isMounted = true
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().LunPath(string(op.c.App.NuvoVolDirPath), op.headDevice).Return(devPath).Times(1)
	op.c.App.NuvoAPI = nvAPI
	fM.RetIfmErr = nil
	fM.RetIfmBool = false
	op.checkIfFsMounted(ctx, false)
	assert.False(op.isMounted)
	assert.False(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Equal("mount", op.rhs.Request.RequestMessages[0].Message)
	tl.Flush()

	tl.Logger().Info("case: checkIfFsMounted mounted")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	op = newOp()
	op.headDevice = devName
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().LunPath(string(op.c.App.NuvoVolDirPath), op.headDevice).Return(devPath).Times(1)
	op.c.App.NuvoAPI = nvAPI
	fM.RetIfmErr = nil
	fM.RetIfmBool = true
	op.checkIfFsMounted(ctx, false)
	assert.True(op.isMounted)
	assert.False(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Equal("mount", op.rhs.Request.RequestMessages[0].Message)
	tl.Flush()
	mockCtrl.Finish()

	// ***************************** clearVolumeFsTag
	tl.Logger().Info("case: clearVolumeFsTag")
	op = newOp()
	fU := &fvra.VolumeUpdater{}
	op.rhs.VSUpdater = fU
	op.clearVolumeFsTag(ctx)
	assert.NotNil(fU.InRVSTags)
	assert.Len(fU.InRVSTags, 1)
	assert.Equal(fU.InRVSTags[0], fmt.Sprintf("%s:%s", com.SystemTagVolumeFsAttached, op.rhs.Request.TargetPath))

	tl.Logger().Info("case: clearVolumeFsTag PLAN-ONLY")
	op = newOp()
	op.planOnly = true
	fU = &fvra.VolumeUpdater{}
	op.rhs.VSUpdater = fU
	op.clearVolumeFsTag(ctx)
	assert.Nil(fU.InRVSTags)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Equal("Clear volume system tag", op.rhs.Request.RequestMessages[0].Message)

	// ***************************** doMount
	tl.Logger().Info("case: doMount")
	mockCtrl = gomock.NewController(t)
	op = newOp()
	op.headDevice = devName
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().LunPath(string(op.c.App.NuvoVolDirPath), op.headDevice).Return(devPath).Times(1)
	op.c.App.NuvoAPI = nvAPI
	fM.RetMfErr = nil
	fM.RetMfCmds = bytes.NewBufferString("fsck\nmount\n")
	op.doMount(ctx)
	assert.False(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 2)
	assert.Equal("fsck", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal("mount", op.rhs.Request.RequestMessages[1].Message)
	tl.Flush()
	mockCtrl.Finish()

	tl.Logger().Info("case: doMount error")
	mockCtrl = gomock.NewController(t)
	op = newOp()
	op.headDevice = devName
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().LunPath(string(op.c.App.NuvoVolDirPath), op.headDevice).Return(devPath).Times(1)
	op.c.App.NuvoAPI = nvAPI
	fM.RetMfErr = fmt.Errorf("mount-error")
	fM.RetMfCmds = bytes.NewBufferString("fsck\nmount\n")
	op.doMount(ctx)
	assert.True(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 3)
	assert.Equal("fsck", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal("mount", op.rhs.Request.RequestMessages[1].Message)
	assert.Equal("mount-error", op.rhs.Request.RequestMessages[2].Message)
	tl.Flush()
	mockCtrl.Finish()

	// ***************************** doUnmount
	tl.Logger().Info("case: doUnmount")
	op = newOp()
	op.headDevice = devName
	fM.RetUfErr = nil
	fM.RetUfCmds = bytes.NewBufferString("umount\n")
	op.doUnmount(ctx)
	assert.False(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Equal("umount", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Info("case: doUnmount error")
	op = newOp()
	op.headDevice = devName
	fM.RetUfErr = fmt.Errorf("umount-error")
	fM.RetUfCmds = bytes.NewBufferString("umount\n")
	op.doUnmount(ctx)
	assert.True(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 2)
	assert.Equal("umount", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal("umount-error", op.rhs.Request.RequestMessages[1].Message)

	// ***************************** getInitialState
	tl.Logger().Info("case: getInitialState UNDO, head mounted")
	op = newOp()
	op.rhs.Request.VolumeSeriesRequestState = com.VolReqStateUndoAttachingFs
	assert.Equal(AttachFsPreUmountCheck, op.getInitialState(ctx))
	assert.Equal("VS-1-HEAD", op.headDevice)
	assert.False(op.planOnly)

	tl.Logger().Info("case: getInitialState UNDO, head not mounted")
	op = newOp()
	op.rhs.Request.VolumeSeriesRequestState = com.VolReqStateUndoAttachingFs
	op.rhs.VolumeSeries.Mounts = nil
	assert.Equal(AttachFsClearConditionInVolume, op.getInitialState(ctx))
	assert.Equal("", op.headDevice)
	assert.False(op.planOnly)

	tl.Logger().Info("case: getInitialState UNDO, head mounted, PLAN-ONLY")
	op = newOp()
	op.rhs.Request.VolumeSeriesRequestState = com.VolReqStateUndoAttachingFs
	op.rhs.Request.PlanOnly = swag.Bool(true)
	assert.Equal(AttachFsPreUmountCheck, op.getInitialState(ctx))
	assert.Equal("VS-1-HEAD", op.headDevice)
	assert.True(op.planOnly)

	tl.Logger().Info("case: getInitialState, head not mounted")
	op = newOp()
	op.rhs.Request.VolumeSeriesRequestState = com.VolReqStateAttachingFs
	op.rhs.VolumeSeries.Mounts = nil
	assert.Equal(AttachFsError, op.getInitialState(ctx))
	assert.Equal("", op.headDevice)
	assert.False(op.planOnly)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Equal("HEAD not mounted", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Info("case: getInitialState, head mounted")
	op = newOp()
	op.rhs.Request.VolumeSeriesRequestState = com.VolReqStateAttachingFs
	assert.Equal(AttachFsPreMountCheck, op.getInitialState(ctx))
	assert.Equal("VS-1-HEAD", op.headDevice)
	assert.False(op.planOnly)

	tl.Logger().Info("case: getInitialState, head mounted, PLAN-ONLY")
	op = newOp()
	op.rhs.Request.VolumeSeriesRequestState = com.VolReqStateAttachingFs
	op.rhs.Request.PlanOnly = swag.Bool(true)
	assert.Equal(AttachFsPreMountCheck, op.getInitialState(ctx))
	assert.Equal("VS-1-HEAD", op.headDevice)
	assert.True(op.planOnly)

	// ***************************** getMountArgs
	tl.Logger().Info("case: getMountArgs rw")
	mockCtrl = gomock.NewController(t)
	op = newOp()
	op.rhs.Request.ReadOnly = false
	op.headDevice = devName
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().LunPath(string(op.c.App.NuvoVolDirPath), op.headDevice).Return(devPath).Times(1)
	op.c.App.NuvoAPI = nvAPI
	fma := op.getMountArgs()
	assert.NotNil(fma)
	assert.NotEmpty(fma.LogPrefix)
	assert.Equal(devPath, fma.Source)
	assert.Equal(op.rhs.Request.TargetPath, fma.Target)
	assert.Equal(op.rhs.Request.FsType, fma.FsType)
	assert.Equal([]string{}, fma.Options)
	assert.True(fma.Deadline.Equal(tCB))
	assert.False(fma.PlanOnly)
	assert.NotNil(fma.Commands)
	tl.Flush()
	mockCtrl.Finish()

	tl.Logger().Info("case: getMountArgs ro")
	mockCtrl = gomock.NewController(t)
	op = newOp()
	op.headDevice = devName
	op.rhs.Request.ReadOnly = true
	op.rhs.Request.RequestedOperations = []string{com.VolReqOpVolRestoreSnapshot}
	op.planOnly = true
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().LunPath(string(op.c.App.NuvoVolDirPath), op.headDevice).Return(devPath).Times(1)
	op.c.App.NuvoAPI = nvAPI
	timeBefore := time.Now()
	fma = op.getMountArgs()
	timeAfter := time.Now()
	assert.NotNil(fma)
	assert.NotEmpty(fma.LogPrefix)
	assert.Equal(devPath, fma.Source)
	assert.Equal(op.rhs.Request.TargetPath, fma.Target)
	assert.Equal(op.rhs.Request.FsType, fma.FsType)
	assert.Equal([]string{"ro"}, fma.Options)
	assert.True(timeBefore.Add(time.Minute * 5).Before(fma.Deadline))
	assert.True(timeAfter.Add(time.Minute * 5).After(fma.Deadline))
	assert.True(fma.PlanOnly)
	assert.NotNil(fma.Commands)
	tl.Flush()
	mockCtrl.Finish()

	// ***************************** getUnmountArgs
	tl.Logger().Info("case: getUnmountArgs")
	op = newOp()
	op.rhs.Request.ReadOnly = false
	op.headDevice = devName
	fua := op.getUnmountArgs()
	assert.NotNil(fua)
	assert.NotEmpty(fua.LogPrefix)
	assert.Equal(op.rhs.Request.TargetPath, fua.Target)
	assert.True(fua.Deadline.Equal(tCB))
	assert.False(fua.PlanOnly)
	assert.NotNil(fua.Commands)

	// ***************************** setVolumeFsTag
	tl.Logger().Info("case: setVolumeFsTag")
	op = newOp()
	fU = &fvra.VolumeUpdater{}
	op.rhs.VSUpdater = fU
	op.setVolumeFsTag(ctx)
	assert.NotNil(fU.InSVSTags)
	assert.Len(fU.InSVSTags, 1)
	assert.Equal(fU.InSVSTags[0], fmt.Sprintf("%s:%s", com.SystemTagVolumeFsAttached, op.rhs.Request.TargetPath))

	tl.Logger().Info("case: setVolumeFsTag PLAN-ONLY")
	op = newOp()
	op.planOnly = true
	fU = &fvra.VolumeUpdater{}
	op.rhs.VSUpdater = fU
	op.setVolumeFsTag(ctx)
	assert.Nil(fU.InSVSTags)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Equal("Set volume system tag", op.rhs.Request.RequestMessages[0].Message)
}
