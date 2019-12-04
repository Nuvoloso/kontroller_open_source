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

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	mockNuvo "github.com/Nuvoloso/kontroller/pkg/nuvoapi/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestBaseOpNuvoPauseResume(t *testing.T) {
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

	app.NuvoVolDirPath = "/nuvo-vol-dir"

	ctx := context.Background()

	c := newComponent()
	c.Init(app)
	assert.NotNil(c.Log)
	assert.Equal(app.Log, c.Log)

	vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VR-1",
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
				NuvoVolumeIdentifier: nuvoVolIdentifier,
			},
		},
	}

	newOp := func() *baseOp {
		op := &baseOp{}
		op.c = c
		op.rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr), VolumeSeries: vsClone(vsObj)}
		return op
	}
	var op *baseOp

	//  ***************************** nuvoPauseIO

	tl.Logger().Infof("case: nuvoPauseIO %s", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI := mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().PauseIo(nuvoVolIdentifier).Return(nil)
	c.App.NuvoAPI = nvAPI
	op.nuvoPauseIO(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	tl.Logger().Infof("case: nuvoPauseIO %s (error)", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().PauseIo(nuvoVolIdentifier).Return(fmt.Errorf("pause-io-error"))
	c.App.NuvoAPI = nvAPI
	op.nuvoPauseIO(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Regexp("PauseIo.*pause-io-error", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: nuvoPauseIO %s (temp error)", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nErr := nuvoapi.NewNuvoAPIError("tempErr", true)
	nvAPI.EXPECT().PauseIo(nuvoVolIdentifier).Return(nErr)
	c.App.NuvoAPI = nvAPI
	op.nuvoPauseIO(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("tempErr", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: nuvoPauseIO %s (planOnly)", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.nuvoPauseIO(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("Pause IO", op.rhs.Request.RequestMessages[0])

	//  ***************************** nuvoResumeIO

	tl.Logger().Infof("case: nuvoResumeIO %s", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().ResumeIo(nuvoVolIdentifier).Return(nil)
	c.App.NuvoAPI = nvAPI
	op.nuvoResumeIO(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	tl.Logger().Infof("case: nuvoResumeIO %s (error)", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().ResumeIo(nuvoVolIdentifier).Return(fmt.Errorf("resume-io-error"))
	c.App.NuvoAPI = nvAPI
	op.nuvoResumeIO(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Regexp("ResumeIo.*resume-io-error", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: nuvoResumeIO %s (temp error)", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nErr = nuvoapi.NewNuvoAPIError("tempErr", true)
	nvAPI.EXPECT().ResumeIo(nuvoVolIdentifier).Return(nErr)
	c.App.NuvoAPI = nvAPI
	op.nuvoResumeIO(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("tempErr", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: nuvoResumeIO %s (planOnly)", vsr.VolumeSeriesRequestState)
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.nuvoResumeIO(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("Resume IO", op.rhs.Request.RequestMessages[0])
}
