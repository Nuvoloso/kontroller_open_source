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

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockcluster "github.com/Nuvoloso/kontroller/pkg/cluster/mock"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestPublishServicePlan(t *testing.T) {
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
				ID: "VSR-1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			RequestedOperations: []string{"ALLOCATE_CAPACITY"},
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "PUBLISHING_SERVICE_PLAN",
			},
		},
	}

	newFakePspOp := func() *fakePspOp {
		op := &fakePspOp{}
		op.c = c
		op.ops = op
		op.rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}
		return op
	}

	tl.Logger().Info("Case: success")
	tl.Flush()
	op := newFakePspOp()
	op.retGIS = PspLoadSpa
	expCalled := []string{"GIS", "LSPA", "LSP", "PTC", "USPA"}
	tl.Flush()
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	tl.Logger().Info("Case: spa ID not set")
	tl.Flush()
	op = newFakePspOp()
	op.retGIS = PspDone
	expCalled = []string{"GIS"}
	tl.Flush()
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	// invoke the real handlers
	tl.Logger().Info("Case: real handler")
	tl.Flush()
	rhs := &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}
	rhs.Request.PlanOnly = swag.Bool(true)
	c.PublishServicePlan(ctx, rhs)

	// check state strings exist up to CfsNoOp
	var ss pspSubState
	for ss = PspLoadSpa; ss < PspNoOp; ss++ {
		s := ss.String()
		tl.Logger().Debugf("Testing %d %s", ss, s)
		assert.Regexp("^Psp", s)
	}
	assert.Regexp("^pspSubState", ss.String())
}

type fakePspOp struct {
	pspOp
	called []string
	retGIS pspSubState
}

func (op *fakePspOp) getInitialState(ctx context.Context) pspSubState {
	op.called = append(op.called, "GIS")
	return op.retGIS
}

func (op *fakePspOp) loadSP(ctx context.Context) {
	op.called = append(op.called, "LSP")
}

func (op *fakePspOp) loadSPA(ctx context.Context) {
	op.called = append(op.called, "LSPA")
}

func (op *fakePspOp) publishToCluster(ctx context.Context) {
	op.called = append(op.called, "PTC")
}

func (op *fakePspOp) updateSPA(ctx context.Context) {
	op.called = append(op.called, "USPA")
}

func TestPublishServicePlanSteps(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log:       tl.Logger(),
			CSISocket: "csi-sock",
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
				ID: "VSR-1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			RequestedOperations: []string{"ALLOCATE_CAPACITY"},
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "PUBLISHING_SERVICE_PLAN",
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				ServicePlanAllocationID: "SPA-1",
			},
		},
	}
	spaObj := &models.ServicePlanAllocation{
		ServicePlanAllocationAllOf0: models.ServicePlanAllocationAllOf0{
			Meta: &models.ObjMeta{ID: "SPA-1"},
		},
		ServicePlanAllocationCreateOnce: models.ServicePlanAllocationCreateOnce{
			ServicePlanID: "SERVICE-PLAN-1",
		},
	}
	spObj := &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			Meta: &models.ObjMeta{ID: "SERVICE-PLAN-1"},
		},
	}

	newOp := func() *pspOp {
		op := &pspOp{}
		op.c = c
		op.ops = op
		op.rhs = &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}
		return op
	}
	var op *pspOp

	//  ***************************** getInitialState
	tl.Logger().Infof("case: getInitialState: CSI not configured")
	tl.Flush()
	op = newOp()
	csiSock := app.CSISocket
	app.CSISocket = ""
	op.rhs.Request.ServicePlanAllocationID = ""
	st := op.getInitialState(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(PspDone, st)
	app.CSISocket = csiSock

	tl.Logger().Infof("case: getInitialState: rei-error")
	tl.Flush()
	c.rei.SetProperty("publish-sp-fail", &rei.Property{BoolValue: true})
	op = newOp()
	op.rhs.Request.ServicePlanAllocationID = ""
	st = op.getInitialState(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Equal(PspDone, st)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("publish-sp-fail", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: getInitialState: spaId not set")
	tl.Flush()
	op = newOp()
	op.rhs.Request.ServicePlanAllocationID = ""
	st = op.getInitialState(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Equal(PspDone, st)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Missing spaAllocationId", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: getInitialState: planOnly")
	tl.Flush()
	op = newOp()
	op.rhs.Request.PlanOnly = swag.Bool(true)
	st = op.getInitialState(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(PspLoadSpa, st)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.True(op.planOnly)

	tl.Logger().Infof("case: getInitialState: normal")
	tl.Flush()
	op = newOp()
	st = op.getInitialState(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(PspLoadSpa, st)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.False(op.planOnly)

	//  ***************************** loadSP
	tl.Logger().Infof("case: loadSP: error")
	tl.Flush()
	fc.InSvPFetchID = ""
	fc.RetSvPFetchObj = nil
	fc.RetSvPFetchErr = fmt.Errorf("sp-fetch")
	op = newOp()
	testutils.Clone(spaObj, &op.spa)
	op.loadSP(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.EqualValues(op.spa.ServicePlanID, fc.InSvPFetchID)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("sp-fetch", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: loadSP success")
	tl.Flush()
	fc.InSvPFetchID = ""
	fc.RetSvPFetchObj = spObj
	fc.RetSvPFetchErr = nil
	op = newOp()
	testutils.Clone(spaObj, &op.spa)
	op.loadSP(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.EqualValues(op.spa.ServicePlanID, fc.InSvPFetchID)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.Equal(spObj, op.sp)

	//  ***************************** loadSPA
	tl.Logger().Infof("case: loadSPA: error")
	tl.Flush()
	fc.InSPAFetchID = ""
	fc.RetSPAFetchObj = nil
	fc.RetSPAFetchErr = fmt.Errorf("spa-fetch")
	op = newOp()
	testutils.Clone(spaObj, &op.spa)
	op.loadSPA(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.EqualValues(op.rhs.Request.ServicePlanAllocationID, fc.InSPAFetchID)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("spa-fetch", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: loadSPA success")
	tl.Flush()
	fc.InSPAFetchID = ""
	fc.RetSPAFetchObj = spaObj
	fc.RetSPAFetchErr = nil
	op = newOp()
	testutils.Clone(spaObj, &op.spa)
	op.loadSPA(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.EqualValues(op.rhs.Request.ServicePlanAllocationID, fc.InSPAFetchID)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.Equal(spaObj, op.spa)

	//  ***************************** publishToCluster
	tl.Logger().Infof("case: publishToCluster planOnly")
	tl.Flush()
	op = newOp()
	op.planOnly = true
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cc := mockcluster.NewMockClient(mockCtrl)
	op.c.App.ClusterClient = cc
	op.publishToCluster(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	mockCtrl.Finish()

	tl.Logger().Infof("case: publishToCluster error")
	tl.Flush()
	op = newOp()
	testutils.Clone(spObj, &op.sp)
	mockCtrl = gomock.NewController(t)
	cc = mockcluster.NewMockClient(mockCtrl)
	cc.EXPECT().ServicePlanPublish(gomock.Any(), op.sp).Return(nil, fmt.Errorf("spp-err"))
	op.c.App.ClusterClient = cc
	op.publishToCluster(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("spp-err", op.rhs.Request.RequestMessages[0])
	assert.Nil(op.cd)
	mockCtrl.Finish()

	tl.Logger().Infof("case: publishToCluster ok")
	tl.Flush()
	op = newOp()
	testutils.Clone(spObj, &op.sp)
	mockCtrl = gomock.NewController(t)
	cc = mockcluster.NewMockClient(mockCtrl)
	expCd := models.ClusterDescriptor{}
	cc.EXPECT().ServicePlanPublish(gomock.Any(), op.sp).Return(expCd, nil)
	op.c.App.ClusterClient = cc
	op.publishToCluster(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.NotNil(op.cd)
	assert.Equal(expCd, op.cd)

	//  ***************************** updateSPA
	tl.Logger().Infof("case: updateSPA planOnly")
	tl.Flush()
	op = newOp()
	op.planOnly = true
	testutils.Clone(spaObj, &op.spa)
	op.updateSPA(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Update clusterDescriptor in SPA", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: updateSPA: error")
	tl.Flush()
	fc.InSPAUpdaterID = ""
	fc.InSPAUpdaterItems = nil
	fc.InSPAUpdaterCtx = nil
	fc.RetSPAUpdaterUpdateErr = fmt.Errorf("updater-err")
	op = newOp()
	op.cd = models.ClusterDescriptor{}
	testutils.Clone(spaObj, &op.spa)
	op.updateSPA(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(op.cd, fc.ModSPAUpdaterObj.ClusterDescriptor)
	assert.Equal(op.spa, fc.ModSPAUpdaterObj)
	assert.EqualValues(op.rhs.Request.ServicePlanAllocationID, fc.InSPAUpdaterID)
	assert.Equal([]string{"clusterDescriptor"}, fc.InSPAUpdaterItems.Append)
	assert.Empty(fc.InSPAUpdaterItems.Set)
	assert.Empty(fc.InSPAUpdaterItems.Remove)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("updater-err", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: updateSPA: ok")
	tl.Flush()
	fc.InSPAUpdaterID = ""
	fc.InSPAUpdaterItems = nil
	fc.InSPAUpdaterCtx = nil
	fc.RetSPAUpdaterUpdateErr = nil
	op = newOp()
	op.cd = models.ClusterDescriptor{}
	testutils.Clone(spaObj, &op.spa)
	op.updateSPA(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(op.cd, fc.ModSPAUpdaterObj.ClusterDescriptor)
	assert.Equal(op.spa, fc.ModSPAUpdaterObj)
	assert.NoError(fc.ModSPAUpdaterErr)
	assert.EqualValues(op.rhs.Request.ServicePlanAllocationID, fc.InSPAUpdaterID)
	assert.Equal([]string{"clusterDescriptor"}, fc.InSPAUpdaterItems.Append)
	assert.Empty(fc.InSPAUpdaterItems.Set)
	assert.Empty(fc.InSPAUpdaterItems.Remove)
	assert.Len(op.rhs.Request.RequestMessages, 0)
}
