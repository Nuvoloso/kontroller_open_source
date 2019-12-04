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
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	"github.com/Nuvoloso/kontroller/pkg/common"
	hk "github.com/Nuvoloso/kontroller/pkg/housekeeping"
	fhk "github.com/Nuvoloso/kontroller/pkg/housekeeping/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestVptRegisterAndValidate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	fts := &fhk.TaskScheduler{}
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log: tl.Logger(),
		},
		TaskScheduler: fts,
	}
	args := Args{
		RetryInterval: time.Second * 20,
	}
	c := ComponentInit(&args)
	c.App = app
	c.Log = app.Log

	vsrTask := vptRegisterAnimator(c)
	assert.NotNil(vsrTask)

	// failure, no task type
	ca := &models.TaskCreateOnce{}
	err := vsrTask.TaskValidate(ca)
	assert.Error(err)
	assert.Regexp("invalid animator", err)

	// success
	ca = &models.TaskCreateOnce{
		Operation: common.TaskVsrPurge,
	}
	assert.NoError(vsrTask.TaskValidate(ca))
}

func TestVptExec(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	fts := &fhk.TaskScheduler{}
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log: tl.Logger(),
		},
		TaskScheduler: fts,
	}
	args := Args{
		RetryInterval: time.Second * 20,
	}
	c := ComponentInit(&args)
	c.App = app
	c.Log = app.Log
	c.VSRRetentionPeriod = 7 * 24 * time.Hour // 7 days in sec

	vsrTask := vptRegisterAnimator(c)
	ctx := context.Background()

	now := time.Now()
	resVSR := []*models.VolumeSeriesRequest{
		&models.VolumeSeriesRequest{
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID:           "id1",
					Version:      1,
					TimeCreated:  strfmt.DateTime(now),
					TimeModified: strfmt.DateTime(now),
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				ClusterID:              "clusterID",
				CompleteByTime:         strfmt.DateTime(now.Add(time.Hour)),
				RequestedOperations:    []string{"CREATE", "BIND", "MOUNT"},
				VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{},
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					NodeID:         "nodeID",
					VolumeSeriesID: "VS-1",
				},
			},
		},
		&models.VolumeSeriesRequest{
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID:           "id2",
					Version:      1,
					TimeCreated:  strfmt.DateTime(now.Add(-time.Hour * 24 * 8)),
					TimeModified: strfmt.DateTime(now.Add(-time.Hour * 24 * 8)),
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				ClusterID:              "clusterID",
				CompleteByTime:         strfmt.DateTime(now.Add(time.Hour)),
				RequestedOperations:    []string{"DELETE"},
				VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{},
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					NodeID:         "nodeID",
					VolumeSeriesID: "VS-2",
				},
			},
		},
	}

	tO := &models.Task{
		TaskAllOf0: models.TaskAllOf0{
			Meta: &models.ObjMeta{ID: "TASK-1"},
		},
		TaskCreateOnce: models.TaskCreateOnce{
			Operation: common.TaskVsrPurge,
		},
	}
	// failure if app does not have a CrudHelpers
	fto := &fhk.TaskOps{
		RetO: tO,
	}
	tO.Messages = nil
	assert.Nil(app.CrudHelpers)
	assert.Panics(func() { vsrTask.TaskExec(ctx, fto) })
	assert.Equal(hk.TaskStateFailed, fto.InSs)
	assert.NotNil(tO.Messages)
	assert.Regexp("Not ready", tO.Messages[0].Message)

	lParams := ops.VolumeSeriesRequestListParams{
		IsTerminated: swag.Bool(true),
	}

	// success, failure to list VSRs
	tl.Logger().Info("case: success with failure to list VSRs")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mCrudHelpers := mock.NewMockAppCrudHelpers(mockCtrl)
	app.CrudHelpers = mCrudHelpers
	mVSROps := mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mVSROps.EXPECT().List(ctx, lParams).Return(nil, centrald.ErrorDbError)
	mDS := mock.NewMockDataStore(mockCtrl)
	mDS.EXPECT().OpsVolumeSeriesRequest().Return(mVSROps).Times(1)
	tO.Messages = nil
	app.DS = mDS
	assert.NotPanics(func() { vsrTask.TaskExec(ctx, fto) })
	assert.NotNil(tO.Messages)
	assert.Equal(int32(0), *fto.InSp.PercentComplete)
	assert.Regexp("Failure to list volume series requests", tO.Messages[0].Message)

	// success, VSRs to be deleted
	tl.Logger().Info("case: success")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mCrudHelpers = mock.NewMockAppCrudHelpers(mockCtrl)
	app.CrudHelpers = mCrudHelpers
	mVSROps = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mVSROps.EXPECT().List(ctx, lParams).Return(resVSR, nil)
	mVSROps.EXPECT().Delete(ctx, string(resVSR[1].Meta.ID)).Return(nil)
	mDS = mock.NewMockDataStore(mockCtrl)
	mDS.EXPECT().OpsVolumeSeriesRequest().Return(mVSROps).Times(2)
	tO.Messages = nil
	app.DS = mDS
	assert.NotPanics(func() { vsrTask.TaskExec(ctx, fto) })
	assert.NotNil(tO.Messages)
	assert.Equal(int32(100), *fto.InSp.PercentComplete)
	assert.Regexp("Deleted 1/1 volume series requests objects", tO.Messages[0].Message)

	// failure to delete VSR
	tl.Logger().Info("case: success with failure to delete VSR")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mCrudHelpers = mock.NewMockAppCrudHelpers(mockCtrl)
	app.CrudHelpers = mCrudHelpers
	mVSROps = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mVSROps.EXPECT().List(ctx, lParams).Return(resVSR, nil)
	mVSROps.EXPECT().Delete(ctx, string(resVSR[1].Meta.ID)).Return(centrald.ErrorDbError)
	mDS = mock.NewMockDataStore(mockCtrl)
	mDS.EXPECT().OpsVolumeSeriesRequest().Return(mVSROps).Times(2)
	tO.Messages = nil
	app.DS = mDS
	assert.NotPanics(func() { vsrTask.TaskExec(ctx, fto) })
	assert.NotNil(tO.Messages)
	assert.Equal(int32(100), *fto.InSp.PercentComplete)
	assert.Regexp("Deleted 0/1 volume series requests objects", tO.Messages[0].Message)
}
