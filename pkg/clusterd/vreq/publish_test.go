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
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	mockcluster "github.com/Nuvoloso/kontroller/pkg/cluster/mock"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	fakeState "github.com/Nuvoloso/kontroller/pkg/clusterd/state/fake"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	fvra "github.com/Nuvoloso/kontroller/pkg/vra/fake"
	units "github.com/docker/go-units"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestPublishSteps(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log:      tl.Logger(),
			SystemID: "system_id",
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()

	c := newComponent()
	c.Init(app)
	assert.NotNil(c.Log)
	assert.Equal(app.Log, c.Log)
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc
	ctx := context.Background()

	vsObj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID:      "VS-1",
				Version: 1,
			},
		},
		VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
			AccountID: "Account-1",
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				VolumeSeriesState: "BOUND",
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				SizeBytes:     swag.Int64(int64(1 * units.GiB)),
				SystemTags:    []string{},
				ServicePlanID: "someID",
			},
		},
	}
	vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VSR-1",
			},
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "PUBLISHING",
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				VolumeSeriesID: "VS-1",
			},
		},
	}

	spObj := &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			Meta: &models.ObjMeta{
				ID: "SP-1",
			},
		},
		ServicePlanMutable: models.ServicePlanMutable{
			Name: models.ServicePlanName("spName"),
		},
	}

	vs := vsClone(vsObj)
	vsr := vsrClone(vsrObj)
	newOp := func() *publishOp {
		op := &publishOp{}
		op.c = c
		op.ops = op
		op.rhs = &vra.RequestHandlerState{A: c.Animator, VolumeSeries: vsClone(vs), Request: vsrClone(vsr)}
		return op
	}
	// ***************************** getInitialState

	// UNDO_PUBLISH cases
	op := newOp()
	op.published = true
	op.cd = models.ClusterDescriptor{
		"somekey": models.ValueType{Kind: "STRING", Value: "someString"},
	}
	op.rhs.Request.VolumeSeriesRequestState = com.VolReqStateUndoPublishing
	assert.Equal(PublishUndoPublish, op.getInitialState(ctx))

	op = newOp()
	op.rhs.Request.VolumeSeriesRequestState = com.VolReqStateUndoPublishing
	assert.Equal(PublishUndoDone, op.getInitialState(ctx))

	// PUBLISH cases
	op = newOp()
	op.rhs.Request.VolumeSeriesRequestState = com.VolReqStatePublishing
	assert.Equal(LoadServicePlan, op.getInitialState(ctx))

	op.rhs.Canceling = true
	assert.Equal(PublishDone, op.getInitialState(ctx))
	op.rhs.Canceling = false // reset

	op.rhs.InError = true
	assert.Equal(PublishDone, op.getInitialState(ctx))
	op.rhs.InError = false // reset

	// ***************************** loadServicePlan

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	fc.InSvPFetchID = ""
	fc.RetSvPFetchObj = nil
	fc.RetSvPFetchErr = fmt.Errorf("sp-fetch")
	op = newOp()
	op.loadServicePlan(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.EqualValues(op.rhs.VolumeSeries.ServicePlanID, fc.InSvPFetchID)
	assert.Regexp("sp-fetch", op.rhs.Request.RequestMessages[0])

	fc.InSvPFetchID = ""
	fc.RetSvPFetchObj = spObj
	fc.RetSvPFetchErr = nil
	op = newOp()
	op.loadServicePlan(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.EqualValues(op.rhs.VolumeSeries.ServicePlanID, fc.InSvPFetchID)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.Equal(spObj, op.sp)

	// ***************************** publish

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	op = newOp()
	mCluster := mockcluster.NewMockClient(mockCtrl)
	op.c.App.ClusterClient = mCluster
	op.c.App.CSISocket = "somesocket"
	op.sp = spObj
	pvca := &cluster.PersistentVolumeCreateArgs{
		VolumeID:        string(vsObj.Meta.ID),
		SizeBytes:       swag.Int64Value(vsObj.SizeBytes),
		AccountID:       string(vsObj.AccountID),
		SystemID:        op.c.App.AppArgs.SystemID,
		DriverType:      com.K8sDriverTypeCSI,
		ServicePlanName: string(op.sp.Name),
	}
	mCluster.EXPECT().PersistentVolumePublish(ctx, pvca).Return(nil, nil)
	op.publish(ctx)
	assert.False(op.rhs.InError)

	// error in PVCreate
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	op = newOp()
	mCluster = mockcluster.NewMockClient(mockCtrl)
	op.c.App.ClusterClient = mCluster
	op.sp = spObj
	pvErr := fmt.Errorf("pv-create-error")
	mCluster.EXPECT().PersistentVolumePublish(ctx, gomock.Not(gomock.Nil())).Return(nil, pvErr)
	op.publish(ctx)
	assert.True(op.rhs.InError)
	assert.Regexp("pv-create-error", op.rhs.Request.RequestMessages[0].Message)

	// ***************************** unPublish

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	op = newOp()
	mCluster = mockcluster.NewMockClient(mockCtrl)
	op.c.App.ClusterClient = mCluster
	mCluster.EXPECT().PersistentVolumeDelete(ctx, gomock.Not(gomock.Nil())).Return(nil, nil)
	op.unPublish(ctx)
	assert.False(op.rhs.RetryLater)

	// error in PVDelete
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	op = newOp()
	mCluster = mockcluster.NewMockClient(mockCtrl)
	op.c.App.ClusterClient = mCluster
	pvErr = fmt.Errorf("pv-delete-error")
	mCluster.EXPECT().PersistentVolumeDelete(ctx, gomock.Not(gomock.Nil())).Return(nil, pvErr)
	op.unPublish(ctx)
	assert.True(op.rhs.RetryLater)

	// error in PVDelete, PV is in a bound state
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	op = newOp()
	mCluster = mockcluster.NewMockClient(mockCtrl)
	op.c.App.ClusterClient = mCluster
	pvErr = cluster.NewK8sError(cluster.ErrPvIsBoundOnDelete.Message, cluster.StatusReasonUnknown)
	mCluster.EXPECT().PersistentVolumeDelete(ctx, gomock.Not(gomock.Nil())).Return(nil, pvErr)
	op.unPublish(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)

	// error in PVDelete, succeeds because PV is not found
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	op = newOp()
	mCluster = mockcluster.NewMockClient(mockCtrl)
	op.c.App.ClusterClient = mCluster
	pvErr = cluster.NewK8sError("", cluster.StatusReasonNotFound)
	mCluster.EXPECT().PersistentVolumeDelete(ctx, gomock.Not(gomock.Nil())).Return(nil, pvErr)
	op.unPublish(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	// ***************************** vsPublish
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	op = newOp()
	fc.RetUVErr = nil
	fc.PassThroughUVObj = true
	fc.InUVItems = nil
	fc.RetVSUpdaterErr = fmt.Errorf("updater-error")
	fc.InVSUpdaterItems = nil
	op.vsPublish(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Failed to update VolumeSeries object:*", op.rhs.Request.RequestMessages[0].Message)
	fc.RetVSUpdaterErr = nil

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	op = newOp()
	op.cd = models.ClusterDescriptor{
		"somekey": models.ValueType{Kind: "STRING", Value: "someString"},
	}
	fc.PassThroughUVObj = true
	fc.RetUVErr = nil
	op.vsPublish(ctx)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(op.rhs.VolumeSeries.ClusterDescriptor)
	assert.Contains(op.rhs.VolumeSeries.ClusterDescriptor, "somekey")

	// ***************************** vsUnpublish
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	op = newOp()
	op.rhs.VolumeSeries.ClusterDescriptor = map[string]models.ValueType{
		com.K8sPvcYaml: models.ValueType{Kind: "STRING", Value: "Some string"},
	}
	fc.RetUVErr = nil
	fc.PassThroughUVObj = true
	fc.InUVItems = nil
	fc.RetVSUpdaterErr = fmt.Errorf("updater-error")
	fc.InVSUpdaterItems = nil
	op.vsUnpublish(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Failed to update VolumeSeries object:*", op.rhs.Request.RequestMessages[0].Message)
	assert.NotNil(op.rhs.VolumeSeries.ClusterDescriptor)
	fc.RetVSUpdaterErr = nil

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	op = newOp()
	op.rhs.VolumeSeries.ClusterDescriptor = map[string]models.ValueType{
		com.K8sPvcYaml: models.ValueType{Kind: "STRING", Value: "Some string"},
	}
	fc.PassThroughUSObj = true
	fc.RetUSErr = nil
	op.vsUnpublish(ctx)
	assert.False(op.rhs.InError)
	assert.Empty(op.rhs.VolumeSeries.ClusterDescriptor)
}

func TestPublish(t *testing.T) {
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
	c := newComponent()
	c.Init(app)
	fso := fakeState.NewFakeClusterState()
	app.StateOps = fso

	vr := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VSR-1",
			},
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "PUBLISHING",
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				VolumeSeriesID: "VS-1",
			},
		},
	}

	op := &fakePublishOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = LoadServicePlan
	expCalled := []string{"GIS", "LSP", "PUB", "VPUB"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakePublishOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = PublishUndoPublish
	expCalled = []string{"GIS", "UPUB", "VUPUB"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakePublishOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = PublishVsPublish
	expCalled = []string{"GIS", "VPUB"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakePublishOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = PublishDone
	expCalled = []string{"GIS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakePublishOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = PublishUndoVsUnpublish
	expCalled = []string{"GIS", "VUPUB"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakePublishOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = PublishNoOp
	expCalled = []string{"GIS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakePublishOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = PublishNoOp
	op.inError = true
	expCalled = []string{"GIS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.True(op.rhs.InError)

	// call the real handler with an error

	v := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: "VS-1",
			},
			RootParcelUUID: "uuid",
		},
	}
	vr = &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VR-1",
			},
		},
	}
	vr.VolumeSeriesID = models.ObjIDMutable(v.Meta.ID)
	fc := &fake.Client{}
	fvu := &fvra.VolumeUpdater{}
	c.oCrud = fc
	c.Animator.OCrud = fc
	rhs := &vra.RequestHandlerState{
		A:            c.Animator,
		InError:      true,
		Request:      vr,
		VolumeSeries: v,
	}
	rhs.VSUpdater = fvu
	c.Publish(nil, rhs)
	assert.Empty(fc.InLsSFObj)

	// call real handler in undo
	vr.VolumeSeriesRequestState = com.VolReqStateUndoSizing
	rhs.InError = true
	c.UndoPublish(nil, rhs)
	assert.True(rhs.InError)
	assert.Empty(fvu.InSVSState)

	// check place state strings exist up to SizeNoOp
	var ss sizeSubState
	for ss = SizeSetStoragePlan; ss < SizeNoOp; ss++ {
		s := ss.String()
		tl.Logger().Debugf("Testing %d %s", ss, s)
		assert.Regexp("^Size", s)
	}
	assert.Regexp("^sizeSubState", ss.String())
}

type fakePublishOps struct {
	publishOp
	called []string
	retGIS publishSubState
}

func (op *fakePublishOps) getInitialState(ctx context.Context) publishSubState {
	op.called = append(op.called, "GIS")
	return op.retGIS
}

func (op *fakePublishOps) loadServicePlan(ctx context.Context) {
	op.called = append(op.called, "LSP")
}

func (op *fakePublishOps) publish(ctx context.Context) {
	op.called = append(op.called, "PUB")
}

func (op *fakePublishOps) unPublish(ctx context.Context) {
	op.called = append(op.called, "UPUB")
}

func (op *fakePublishOps) vsPublish(ctx context.Context) {
	op.called = append(op.called, "VPUB")
}

func (op *fakePublishOps) vsUnpublish(ctx context.Context) {
	op.called = append(op.called, "VUPUB")
}
