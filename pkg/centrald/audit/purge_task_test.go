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


package audit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	fal "github.com/Nuvoloso/kontroller/pkg/centrald/audit/fake"
	"github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	"github.com/Nuvoloso/kontroller/pkg/common"
	hk "github.com/Nuvoloso/kontroller/pkg/housekeeping"
	fhk "github.com/Nuvoloso/kontroller/pkg/housekeeping/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestAuditPurgeTaskRegisterAndValidate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			API:    &operations.NuvolosoAPI{},
			Log:    tl.Logger(),
			Server: &restapi.Server{},
		},
		TaskScheduler: &fhk.TaskScheduler{},
	}
	args := Args{
		RetryInterval: time.Second * 20,
	}
	c := Register(&args)
	c.Init(app)
	c.App = app
	c.Log = app.Log

	purgeTask := purgeTaskRegisterAnimator(c)
	assert.NotNil(purgeTask)

	// failure, no task type
	ca := &models.TaskCreateOnce{}
	err := purgeTask.TaskValidate(ca)
	assert.Error(err)
	assert.Regexp("invalid animator", err)

	// success
	ca = &models.TaskCreateOnce{
		Operation: common.TaskAuditRecordsPurge,
	}
	assert.NoError(purgeTask.TaskValidate(ca))
}

func TestTestAuditPurgeTaskExec(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log:    tl.Logger(),
			API:    &operations.NuvolosoAPI{},
			Server: &restapi.Server{},
		},
		TaskScheduler: &fhk.TaskScheduler{},
	}
	args := Args{
		RetryInterval: time.Second * 20,
	}
	c := Register(&args)
	c.Init(app)
	c.App = app
	c.Log = app.Log

	fa := &fal.AuditLog{}
	app.AuditLog = fa

	ctx := context.Background()

	purgeTask := purgeTaskRegisterAnimator(c)
	assert.NotNil(purgeTask)

	tO := &models.Task{
		TaskAllOf0: models.TaskAllOf0{
			Meta: &models.ObjMeta{ID: "TASK-1"},
		},
		TaskCreateOnce: models.TaskCreateOnce{
			Operation: common.TaskAuditRecordsPurge,
		},
	}
	// failure if app does not have CrudHelpers
	fto := &fhk.TaskOps{
		RetO: tO,
	}
	tO.Messages = nil
	assert.Nil(app.CrudHelpers)
	assert.Panics(func() { purgeTask.TaskExec(ctx, fto) })
	assert.Equal(hk.TaskStateFailed, fto.InSs)
	assert.NotNil(tO.Messages)
	assert.Regexp("Not ready", tO.Messages[0].Message)

	// success
	tl.Logger().Info("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mCrudHelpers := mock.NewMockAppCrudHelpers(mockCtrl)
	mCrudHelpers.EXPECT().Lock()
	mCrudHelpers.EXPECT().Unlock()
	app.CrudHelpers = mCrudHelpers
	fa.ExpireRet = nil
	tO.Messages = nil
	assert.NotPanics(func() { purgeTask.TaskExec(ctx, fto) })
	assert.Nil(tO.Messages)

	// failure
	tl.Logger().Info("case: failure to delete audit log records")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mCrudHelpers = mock.NewMockAppCrudHelpers(mockCtrl)
	mCrudHelpers.EXPECT().Lock()
	mCrudHelpers.EXPECT().Unlock()
	app.CrudHelpers = mCrudHelpers
	fa.ExpireRet = errors.New("expire-error")
	tO.Messages = nil
	assert.NotPanics(func() { purgeTask.TaskExec(ctx, fto) })
	assert.NotNil(tO.Messages)
	assert.Regexp("Failure to expire", tO.Messages[0].Message)
}
