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


package crud

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/application_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/cluster"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/consistency_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/metrics"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/node"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/pool"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/protection_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan_allocation"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/snapshot"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_formula"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/system"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/task"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/docker/go-units"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestError(t *testing.T) {
	assert := assert.New(t)

	err := NewError(nil)
	assert.NotNil(err)
	e, ok := err.(*Error)
	if assert.True(ok, "err is an *Error") && assert.NotNil(e.Payload) {
		assert.Equal(*serverError, e.Payload)
		assert.False(e.NotFound())
	}

	err = NewError(&models.Error{Code: 400})
	assert.NotNil(err)
	e, ok = err.(*Error)
	if assert.True(ok, "err is an *Error") && assert.NotNil(e.Payload) {
		assert.Equal(*serverError, e.Payload)
		assert.True(e.IsTransient())
		assert.False(e.NotFound())
	}

	inError := &models.Error{Code: 409, Message: swag.String(com.ErrorExists)}
	err = NewError(inError)
	assert.NotNil(err)
	e, ok = err.(*Error)
	if assert.True(ok, "err is an *Error") && assert.NotNil(e.Payload) {
		assert.Equal(*inError, e.Payload)
		assert.Equal(*inError.Message, e.Error())
		assert.False(e.IsTransient())
		assert.False(e.NotFound())
		assert.False(e.InConflict())
		assert.True(e.Exists())
	}

	notFoundError := &models.Error{Code: 404, Message: swag.String(com.ErrorNotFound + " trailer")}
	err = NewError(notFoundError)
	assert.NotNil(err)
	e, ok = err.(*Error)
	if assert.True(ok, "err is an *Error") && assert.NotNil(e.Payload) {
		assert.Equal(*notFoundError, e.Payload)
		assert.Equal(*notFoundError.Message, e.Error())
		assert.False(e.IsTransient())
		assert.True(e.NotFound())
		assert.False(e.InConflict())
	}

	inConflictError := &models.Error{Code: 409, Message: swag.String(com.ErrorRequestInConflict + " trailer")}
	err = NewError(inConflictError)
	assert.NotNil(err)
	e, ok = err.(*Error)
	if assert.True(ok, "err is an *Error") && assert.NotNil(e.Payload) {
		assert.Equal(*inConflictError, e.Payload)
		assert.Equal(*inConflictError.Message, e.Error())
		assert.False(e.IsTransient())
		assert.False(e.NotFound())
		assert.True(e.InConflict())
	}
}

func TestNewClient(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	logger := tl.Logger()
	c := NewClient(mAPI, logger)
	if assert.NotNil(c) {
		assert.Equal(mAPI, c.ClientAPI)
		assert.Equal(logger, c.Log)
	}
	c = NewClient(mAPI, logger)
	if assert.NotNil(c) {
		assert.Equal(mAPI, c.ClientAPI)
		assert.Equal(logger, c.Log)
	}
}

func TestFetchSystemObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	resSys := &system.SystemFetchOK{
		Payload: &models.System{
			SystemAllOf0: models.SystemAllOf0{
				Meta: &models.ObjMeta{
					ID:      "SYSTEM",
					Version: 1,
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	sysOps := mockmgmtclient.NewMockSystemClient(mockCtrl)
	mAPI.EXPECT().System().Return(sysOps).MinTimes(1)
	sysOps.EXPECT().SystemFetch(gomock.Not(gomock.Nil())).Return(resSys, nil).MinTimes(1)
	s, err := c.SystemFetch(ctx)
	assert.Nil(err)
	assert.NotNil(s)
	assert.Equal(resSys.Payload, s)

	// api Error
	apiErr := system.NewSystemFetchDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("system fetch error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	sysOps = mockmgmtclient.NewMockSystemClient(mockCtrl)
	mAPI.EXPECT().System().Return(sysOps).MinTimes(1)
	sysOps.EXPECT().SystemFetch(gomock.Not(gomock.Nil())).Return(nil, apiErr).MinTimes(1)
	s, err = c.SystemFetch(ctx)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(s)
}

func TestFetchSystemHostname(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	resSys := &system.SystemHostnameFetchOK{
		Payload: "hostname",
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	sysOps := mockmgmtclient.NewMockSystemClient(mockCtrl)
	mAPI.EXPECT().System().Return(sysOps).MinTimes(1)
	sysOps.EXPECT().SystemHostnameFetch(gomock.Not(gomock.Nil())).Return(resSys, nil).MinTimes(1)
	s, err := c.SystemHostnameFetch(ctx)
	assert.Nil(err)
	assert.NotNil(s)
	assert.Equal(resSys.Payload, s)

	// api Error
	apiErr := system.NewSystemHostnameFetchDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("system hostname fetch error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	sysOps = mockmgmtclient.NewMockSystemClient(mockCtrl)
	mAPI.EXPECT().System().Return(sysOps).MinTimes(1)
	sysOps.EXPECT().SystemHostnameFetch(gomock.Not(gomock.Nil())).Return(nil, apiErr).MinTimes(1)
	s, err = c.SystemHostnameFetch(ctx)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Empty(s)
}

func TestFetchAccountObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &account.AccountFetchOK{
		Payload: &models.Account{
			AccountAllOf0: models.AccountAllOf0{
				Meta: &models.ObjMeta{
					ID:      "Account-1",
					Version: 1,
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps).MinTimes(1)
	mA := mockmgmtclient.NewAccountMatcher(t, account.NewAccountFetchParams().WithID(string(res.Payload.Meta.ID)))
	aOps.EXPECT().AccountFetch(mA).Return(res, nil)
	aOut, err := c.AccountFetch(ctx, string(res.Payload.Meta.ID))
	assert.Equal(ctx, mA.FetchParam.Context)
	assert.Nil(err)
	assert.NotNil(aOut)
	assert.Equal(res.Payload, aOut)

	// api Error
	apiErr := account.NewAccountFetchDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("account fetch error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps).MinTimes(1)
	aOps.EXPECT().AccountFetch(mA).Return(nil, apiErr)
	aOut, err = c.AccountFetch(nil, string(res.Payload.Meta.ID))
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(aOut)
}

func TestListAccountObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &account.AccountListOK{
		Payload: []*models.Account{
			&models.Account{
				AccountAllOf0: models.AccountAllOf0{
					Meta: &models.ObjMeta{
						ID:      "Account-1",
						Version: 1,
					},
				},
				AccountMutable: models.AccountMutable{
					Name: "MyAccount",
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	acOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(acOps).MinTimes(1)
	mAc := mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	acOps.EXPECT().AccountList(mAc).Return(res, nil)
	acl, err := c.AccountList(ctx, mAc.ListParam)
	assert.Equal(ctx, mAc.ListParam.Context)
	assert.Nil(err)
	assert.NotNil(acl)
	assert.EqualValues(res, acl)

	// api failure
	apiErr := account.NewAccountListDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("account list error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	acOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(acOps).MinTimes(1)
	acOps.EXPECT().AccountList(mAc).Return(nil, apiErr)
	acl, err = c.AccountList(ctx, mAc.ListParam)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(acl)
}

func TestCreateApplicationGroupObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &application_group.ApplicationGroupCreateCreated{
		Payload: &models.ApplicationGroup{
			ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
				Meta: &models.ObjMeta{
					ID:      "AG-1",
					Version: 1,
				},
			},
			ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
				AccountID: "A-1",
			},
			ApplicationGroupMutable: models.ApplicationGroupMutable{
				Name: "AG",
			},
		},
	}
	agIn := &models.ApplicationGroup{
		ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
			AccountID: "A-1",
		},
		ApplicationGroupMutable: models.ApplicationGroupMutable{
			Name: "AG",
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	agOps := mockmgmtclient.NewMockApplicationGroupClient(mockCtrl)
	mAPI.EXPECT().ApplicationGroup().Return(agOps).MinTimes(1)
	mAG := mockmgmtclient.NewApplicationGroupMatcher(t, application_group.NewApplicationGroupCreateParams().WithPayload(agIn))
	agOps.EXPECT().ApplicationGroupCreate(mAG).Return(res, nil)
	agOut, err := c.ApplicationGroupCreate(ctx, agIn)
	assert.Equal(ctx, mAG.CreateParam.Context)
	assert.Nil(err)
	assert.NotNil(agOut)
	assert.Equal(res.Payload, agOut)

	// error case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	agOps = mockmgmtclient.NewMockApplicationGroupClient(mockCtrl)
	mAPI.EXPECT().ApplicationGroup().Return(agOps).MinTimes(1)
	mAG.CreateParam.Context = nil // will be set from call
	apiErr := application_group.NewApplicationGroupCreateDefault(400)
	apiErr.Payload = &models.Error{Message: swag.String("create error"), Code: 400}
	agOps.EXPECT().ApplicationGroupCreate(mAG).Return(nil, apiErr)
	agOut, err = c.ApplicationGroupCreate(ctx, agIn)
	assert.Equal(ctx, mAG.CreateParam.Context)
	assert.Error(err)
	assert.Regexp("create error", err.Error())
	assert.Nil(agOut)
}

func TestDeleteApplicationGroupObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	agID := "AG-1"

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	agOps := mockmgmtclient.NewMockApplicationGroupClient(mockCtrl)
	mAPI.EXPECT().ApplicationGroup().Return(agOps).MinTimes(1)
	mAG := mockmgmtclient.NewApplicationGroupMatcher(t, application_group.NewApplicationGroupDeleteParams().WithID(agID))
	agOps.EXPECT().ApplicationGroupDelete(mAG).Return(nil, nil)
	err := c.ApplicationGroupDelete(ctx, agID)
	assert.Equal(ctx, mAG.DeleteParam.Context)
	assert.Nil(err)

	// api Error
	apiErr := application_group.NewApplicationGroupDeleteDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("storage delete error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	agOps = mockmgmtclient.NewMockApplicationGroupClient(mockCtrl)
	mAPI.EXPECT().ApplicationGroup().Return(agOps).MinTimes(1)
	agOps.EXPECT().ApplicationGroupDelete(mAG).Return(nil, apiErr)
	err = c.ApplicationGroupDelete(ctx, agID)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
}

func TestFetchApplicationGroupObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &application_group.ApplicationGroupFetchOK{
		Payload: &models.ApplicationGroup{
			ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
				Meta: &models.ObjMeta{
					ID:      "AG-1",
					Version: 1,
				},
			},
			ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
				AccountID: "A-1",
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	agOps := mockmgmtclient.NewMockApplicationGroupClient(mockCtrl)
	mAPI.EXPECT().ApplicationGroup().Return(agOps).MinTimes(1)
	mAG := mockmgmtclient.NewApplicationGroupMatcher(t, application_group.NewApplicationGroupFetchParams().WithID(string(res.Payload.Meta.ID)))
	agOps.EXPECT().ApplicationGroupFetch(mAG).Return(res, nil)
	agOut, err := c.ApplicationGroupFetch(ctx, string(res.Payload.Meta.ID))
	assert.Equal(ctx, mAG.FetchParam.Context)
	assert.Nil(err)
	assert.NotNil(agOut)
	assert.Equal(res.Payload, agOut)

	// api Error
	apiErr := application_group.NewApplicationGroupFetchDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("application group fetch error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	agOps = mockmgmtclient.NewMockApplicationGroupClient(mockCtrl)
	mAPI.EXPECT().ApplicationGroup().Return(agOps).MinTimes(1)
	agOps.EXPECT().ApplicationGroupFetch(mAG).Return(nil, apiErr)
	agOut, err = c.ApplicationGroupFetch(nil, string(res.Payload.Meta.ID))
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(agOut)
}

func TestListApplicationGroupObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &application_group.ApplicationGroupListOK{
		Payload: []*models.ApplicationGroup{
			&models.ApplicationGroup{
				ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
					Meta: &models.ObjMeta{
						ID:      "AG-1",
						Version: 1,
					},
				},
				ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
					AccountID: "A-1",
				},
				ApplicationGroupMutable: models.ApplicationGroupMutable{
					Name: "MyAG",
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	agOps := mockmgmtclient.NewMockApplicationGroupClient(mockCtrl)
	mAPI.EXPECT().ApplicationGroup().Return(agOps).MinTimes(1)
	mAG := mockmgmtclient.NewApplicationGroupMatcher(t, application_group.NewApplicationGroupListParams())
	agOps.EXPECT().ApplicationGroupList(mAG).Return(res, nil)
	agl, err := c.ApplicationGroupList(ctx, mAG.ListParam)
	assert.Equal(ctx, mAG.ListParam.Context)
	assert.Nil(err)
	assert.NotNil(agl)
	assert.EqualValues(res, agl)

	// api failure
	apiErr := application_group.NewApplicationGroupListDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("ag list error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	agOps = mockmgmtclient.NewMockApplicationGroupClient(mockCtrl)
	mAPI.EXPECT().ApplicationGroup().Return(agOps).MinTimes(1)
	agOps.EXPECT().ApplicationGroupList(mAG).Return(nil, apiErr)
	agl, err = c.ApplicationGroupList(ctx, mAG.ListParam)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(agl)
}

func TestUpdateApplicationGroupObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &application_group.ApplicationGroupUpdateOK{
		Payload: &models.ApplicationGroup{
			ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
				Meta: &models.ObjMeta{
					ID:      "AG-1",
					Version: 1,
				},
			},
			ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
				AccountID: "A-1",
			},
			ApplicationGroupMutable: models.ApplicationGroupMutable{
				Name: "AG",
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	agOps := mockmgmtclient.NewMockApplicationGroupClient(mockCtrl)
	mAPI.EXPECT().ApplicationGroup().Return(agOps).MinTimes(1)
	mAG := mockmgmtclient.NewApplicationGroupMatcher(t, application_group.NewApplicationGroupUpdateParams().WithID(string(res.Payload.Meta.ID)))
	mAG.UpdateParam.Payload = &res.Payload.ApplicationGroupMutable
	mAG.UpdateParam.Set = []string{"description", "name"}
	mAG.UpdateParam.Append = []string{"appendArg"}
	mAG.UpdateParam.Remove = []string{"removeArg"}
	mAG.UpdateParam.Context = nil // will be set from call
	agOps.EXPECT().ApplicationGroupUpdate(mAG).Return(res, nil).MinTimes(1)
	items := &Updates{}
	items.Set = mAG.UpdateParam.Set
	items.Append = mAG.UpdateParam.Append
	items.Remove = mAG.UpdateParam.Remove
	agOut, err := c.ApplicationGroupUpdate(ctx, res.Payload, items)
	assert.Equal(ctx, mAG.UpdateParam.Context)
	assert.Nil(err)
	assert.NotNil(agOut)
	assert.Equal(res.Payload, agOut)

	// API error
	apiErr := application_group.NewApplicationGroupUpdateDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("application group request update error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	agOps = mockmgmtclient.NewMockApplicationGroupClient(mockCtrl)
	mAPI.EXPECT().ApplicationGroup().Return(agOps).MinTimes(1)
	mAG.UpdateParam.Version = swag.Int32(int32(res.Payload.Meta.Version))
	agOps.EXPECT().ApplicationGroupUpdate(mAG).Return(nil, apiErr).MinTimes(1)
	mAG.UpdateParam.Context = nil // will be set from call
	items.Version = 1
	agOut, err = c.ApplicationGroupUpdate(ctx, res.Payload, items)
	assert.Equal(ctx, mAG.UpdateParam.Context)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(agOut)
}

func TestApplicationGroupUpdater(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	// A fake updater is used to test - this test focuses on updaterArg function calls.
	// Updater is fully tested in TestVolumeSeriesObjUpdater.
	fu := &fakeUpdater{}
	c.updater = fu
	ctx := context.Background()

	ag := &models.ApplicationGroup{}
	ag.Meta = &models.ObjMeta{ID: "AG-1", Version: 2}

	var mfO *models.ApplicationGroup
	modifyFn := func(o *models.ApplicationGroup) (*models.ApplicationGroup, error) {
		mfO = o
		return nil, nil
	}

	fu.ouRetO = ag
	items := &Updates{Set: []string{"systemTags"}}
	c.ApplicationGroupUpdater(ctx, "AG-1", modifyFn, items)
	assert.Equal(ctx, fu.ouInCtx)
	assert.Equal("AG-1", fu.ouInOid)
	assert.Equal(items, fu.ouInItems)
	assert.NotNil(fu.ouInUA)
	ua := fu.ouInUA
	assert.Equal("ApplicationGroup", ua.typeName)
	assert.NotNil(ua.modifyFn)
	assert.NotNil(ua.fetchFn)
	assert.NotNil(ua.updateFn)
	assert.NotNil(ua.metaFn)

	// test modify gets called
	ua.modifyFn(ag)
	assert.Equal(ag, mfO)
	ua.modifyFn(nil)
	assert.Nil(mfO)

	// test meta functionality
	assert.Equal(ag.Meta, ua.metaFn(ag))

	// test fetch invocation
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	agOps := mockmgmtclient.NewMockApplicationGroupClient(mockCtrl)
	mAPI.EXPECT().ApplicationGroup().Return(agOps).MinTimes(1)
	mFAG := mockmgmtclient.NewApplicationGroupMatcher(t, application_group.NewApplicationGroupFetchParams().WithID("AG-1"))
	agOps.EXPECT().ApplicationGroupFetch(mFAG).Return(nil, fmt.Errorf("fetch-error"))
	retF, err := ua.fetchFn(ctx, "AG-1")
	assert.Equal(ctx, mFAG.FetchParam.Context)
	assert.Error(err)
	assert.Regexp("fetch-error", err)
	assert.Nil(retF)

	// test update invocation
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	agOps = mockmgmtclient.NewMockApplicationGroupClient(mockCtrl)
	items.Version = int32(ag.Meta.Version)
	mAPI.EXPECT().ApplicationGroup().Return(agOps)
	mUAG := mockmgmtclient.NewApplicationGroupMatcher(t, application_group.NewApplicationGroupUpdateParams().WithID(string(ag.Meta.ID)))
	mUAG.UpdateParam.Payload = &ag.ApplicationGroupMutable
	mUAG.UpdateParam.Set = []string{"systemTags"}
	mUAG.UpdateParam.Version = swag.Int32(int32(ag.Meta.Version))
	mUAG.UpdateParam.Context = nil // will be set from call
	agOps.EXPECT().ApplicationGroupUpdate(mUAG).Return(nil, fmt.Errorf("update-error"))
	retU, err := ua.updateFn(ctx, ag, items)
	assert.Equal(ctx, mUAG.UpdateParam.Context)
	assert.Error(err)
	assert.Regexp("update-error", err)
	assert.Nil(retU)

	// error path
	fu.ouRetErr = fmt.Errorf("updater-error")
	fu.ouRetO = nil
	_, err = c.ApplicationGroupUpdater(ctx, "AG-1", modifyFn, items)
	assert.Error(err)
	assert.Regexp("updater-error", err)
}

func TestFetchClusterObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	resD := &cluster.ClusterFetchOK{
		Payload: &models.Cluster{
			ClusterAllOf0: models.ClusterAllOf0{
				Meta: &models.ObjMeta{
					ID:      "CLUSTER-1",
					Version: 1,
				},
			},
			ClusterCreateOnce: models.ClusterCreateOnce{
				ClusterType: "kubernetes",
			},
			ClusterMutable: models.ClusterMutable{
				ClusterCreateMutable: models.ClusterCreateMutable{
					Name: "MyCluster",
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	clOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps).MinTimes(1)
	mCl := mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterFetchParams().WithID(string(resD.Payload.Meta.ID)))
	clOps.EXPECT().ClusterFetch(mCl).Return(resD, nil).MinTimes(1)
	s, err := c.ClusterFetch(ctx, string(resD.Payload.Meta.ID))
	assert.Equal(ctx, mCl.FetchParam.Context)
	assert.Nil(err)
	assert.NotNil(s)
	assert.Equal(resD.Payload, s)

	// api Error
	apiErr := cluster.NewClusterFetchDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("cluster fetch error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps).MinTimes(1)
	clOps.EXPECT().ClusterFetch(mCl).Return(nil, apiErr).MinTimes(1)
	s, err = c.ClusterFetch(nil, string(resD.Payload.Meta.ID))
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(s)
}

func TestUpdateClusterObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()
	res := &cluster.ClusterUpdateOK{
		Payload: &models.Cluster{
			ClusterAllOf0: models.ClusterAllOf0{
				Meta: &models.ObjMeta{
					ID:      "Cluster1",
					Version: 1,
				},
			},
			ClusterCreateOnce: models.ClusterCreateOnce{
				AccountID:   "ownerAccountId",
				ClusterType: "kubernetes",
				CspDomainID: "cspDomainID",
			},
			ClusterMutable: models.ClusterMutable{
				ClusterCreateMutable: models.ClusterCreateMutable{
					AuthorizedAccounts: []models.ObjIDMutable{"a-aid1"},
					Name:               "ClusterName",
					Tags:               []string{"tag1", "tag2"},
					ClusterIdentifier:  "clusterID",
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	ops := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(ops).MinTimes(1)
	mC := mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterUpdateParams().WithID(string(res.Payload.Meta.ID)))
	mC.UpdateParam.Version = swag.Int32(2)
	mC.UpdateParam.Payload = &res.Payload.ClusterMutable
	mC.UpdateParam.Set = []string{"name"}
	mC.UpdateParam.Append = []string{"tags"}
	mC.UpdateParam.Remove = []string{"authorizedAccounts"}
	mC.UpdateParam.Context = nil // will be set from call
	ops.EXPECT().ClusterUpdate(mC).Return(res, nil).MinTimes(1)
	items := &Updates{}
	items.Set = mC.UpdateParam.Set
	items.Append = mC.UpdateParam.Append
	items.Remove = mC.UpdateParam.Remove
	items.Version = 2
	v, err := c.ClusterUpdate(ctx, res.Payload, items)
	assert.Equal(ctx, mC.UpdateParam.Context)
	assert.Nil(err)
	assert.NotNil(v)
	assert.Equal(res.Payload, v)

	// API error
	apiErr := cluster.NewClusterUpdateDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("cluster request update error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	ops = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(ops).MinTimes(1)
	ops.EXPECT().ClusterUpdate(mC).Return(nil, apiErr).MinTimes(1)
	mC.UpdateParam.Context = nil // will be set from call
	v, err = c.ClusterUpdate(ctx, res.Payload, items)
	assert.Equal(ctx, mC.UpdateParam.Context)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(v)
}

func TestUpdaterClusterObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	fu := &fakeUpdater{}
	c.updater = fu
	ctx := context.Background()

	cl := &models.Cluster{}
	cl.Meta = &models.ObjMeta{ID: "Cluster1", Version: 2}

	var mfO *models.Cluster
	modifyFn := func(o *models.Cluster) (*models.Cluster, error) {
		mfO = o
		return nil, nil
	}

	fu.ouRetO = cl
	items := &Updates{Set: []string{"name"}, Version: 2}
	c.ClusterUpdater(ctx, "Cluster1", modifyFn, items)
	assert.Equal(ctx, fu.ouInCtx)
	assert.Equal("Cluster1", fu.ouInOid)
	assert.Equal(items, fu.ouInItems)
	assert.NotNil(fu.ouInUA)
	ua := fu.ouInUA
	assert.Equal("Cluster", ua.typeName)
	assert.NotNil(ua.modifyFn)
	assert.NotNil(ua.fetchFn)
	assert.NotNil(ua.updateFn)
	assert.NotNil(ua.metaFn)

	// test modify gets called
	ua.modifyFn(cl)
	assert.Equal(cl, mfO)
	ua.modifyFn(nil)
	assert.Nil(mfO)

	// test meta functionality
	assert.Equal(cl.Meta, ua.metaFn(cl))

	// test fetch invocation
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	ops := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(ops).MinTimes(1)
	mFCP := mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterFetchParams().WithID("Cluster1"))
	ops.EXPECT().ClusterFetch(mFCP).Return(nil, fmt.Errorf("fetch-error"))
	retF, err := ua.fetchFn(ctx, "Cluster1")
	assert.Equal(ctx, mFCP.FetchParam.Context)
	assert.Error(err)
	assert.Regexp("fetch-error", err)
	assert.Nil(retF)

	// test update invocation
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	ops = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(ops)
	mUCP := mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterUpdateParams().WithID(string(cl.Meta.ID)))
	mUCP.UpdateParam.Payload = &cl.ClusterMutable
	mUCP.UpdateParam.Set = []string{"name"}
	mUCP.UpdateParam.Version = swag.Int32(int32(cl.Meta.Version))
	mUCP.UpdateParam.Context = nil // will be set from call
	ops.EXPECT().ClusterUpdate(mUCP).Return(nil, fmt.Errorf("update-error"))
	retU, err := ua.updateFn(ctx, cl, items)
	assert.Equal(ctx, mUCP.UpdateParam.Context)
	assert.Error(err)
	assert.Regexp("update-error", err)
	assert.Nil(retU)

	// error path
	fu.ouRetErr = fmt.Errorf("updater-error")
	fu.ouRetO = nil
	_, err = c.ClusterUpdater(ctx, "Cluster1", modifyFn, items)
	assert.Error(err)
	assert.Regexp("updater-error", err)
}
func TestCreateConsistencyGroupObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &consistency_group.ConsistencyGroupCreateCreated{
		Payload: &models.ConsistencyGroup{
			ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
				Meta: &models.ObjMeta{
					ID:      "CG-1",
					Version: 1,
				},
			},
			ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
				AccountID: "A-1",
			},
			ConsistencyGroupMutable: models.ConsistencyGroupMutable{
				Name: "CG",
			},
		},
	}
	cgIn := &models.ConsistencyGroup{
		ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
			AccountID: "A-1",
		},
		ConsistencyGroupMutable: models.ConsistencyGroupMutable{
			Name: "CG",
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	cgOps := mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cgOps).MinTimes(1)
	mCG := mockmgmtclient.NewConsistencyGroupMatcher(t, consistency_group.NewConsistencyGroupCreateParams().WithPayload(cgIn))
	cgOps.EXPECT().ConsistencyGroupCreate(mCG).Return(res, nil)
	cgOut, err := c.ConsistencyGroupCreate(ctx, cgIn)
	assert.Equal(ctx, mCG.CreateParam.Context)
	assert.Nil(err)
	assert.NotNil(cgOut)
	assert.Equal(res.Payload, cgOut)

	// error case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	cgOps = mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cgOps).MinTimes(1)
	mCG.CreateParam.Context = nil // will be set from call
	apiErr := consistency_group.NewConsistencyGroupCreateDefault(400)
	apiErr.Payload = &models.Error{Message: swag.String("create error"), Code: 400}
	cgOps.EXPECT().ConsistencyGroupCreate(mCG).Return(nil, apiErr)
	cgOut, err = c.ConsistencyGroupCreate(ctx, cgIn)
	assert.Equal(ctx, mCG.CreateParam.Context)
	assert.Error(err)
	assert.Regexp("create error", err.Error())
	assert.Nil(cgOut)
}

func TestDeleteConsistencyGroupObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	cgID := "CG-1"

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	cgOps := mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cgOps).MinTimes(1)
	mCG := mockmgmtclient.NewConsistencyGroupMatcher(t, consistency_group.NewConsistencyGroupDeleteParams().WithID(cgID))
	cgOps.EXPECT().ConsistencyGroupDelete(mCG).Return(nil, nil)
	err := c.ConsistencyGroupDelete(ctx, cgID)
	assert.Equal(ctx, mCG.DeleteParam.Context)
	assert.Nil(err)

	// api Error
	apiErr := consistency_group.NewConsistencyGroupDeleteDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("storage delete error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	cgOps = mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cgOps).MinTimes(1)
	cgOps.EXPECT().ConsistencyGroupDelete(mCG).Return(nil, apiErr)
	err = c.ConsistencyGroupDelete(ctx, cgID)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
}

func TestFetchConsistencyGroupObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &consistency_group.ConsistencyGroupFetchOK{
		Payload: &models.ConsistencyGroup{
			ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
				Meta: &models.ObjMeta{
					ID:      "CG-1",
					Version: 1,
				},
			},
			ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
				AccountID: "A-1",
			},
			ConsistencyGroupMutable: models.ConsistencyGroupMutable{
				Name:                "CG",
				ApplicationGroupIds: []models.ObjIDMutable{"app"},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	cgOps := mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cgOps).MinTimes(1)
	mCG := mockmgmtclient.NewConsistencyGroupMatcher(t, consistency_group.NewConsistencyGroupFetchParams().WithID(string(res.Payload.Meta.ID)))
	cgOps.EXPECT().ConsistencyGroupFetch(mCG).Return(res, nil)
	cgOut, err := c.ConsistencyGroupFetch(ctx, string(res.Payload.Meta.ID))
	assert.Equal(ctx, mCG.FetchParam.Context)
	assert.Nil(err)
	assert.NotNil(cgOut)
	assert.Equal(res.Payload, cgOut)

	// api Error
	apiErr := consistency_group.NewConsistencyGroupFetchDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("consistency group fetch error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	cgOps = mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cgOps).MinTimes(1)
	cgOps.EXPECT().ConsistencyGroupFetch(mCG).Return(nil, apiErr)
	cgOut, err = c.ConsistencyGroupFetch(nil, string(res.Payload.Meta.ID))
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(cgOut)
}

func TestListConsistencyGroupObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &consistency_group.ConsistencyGroupListOK{
		Payload: []*models.ConsistencyGroup{
			&models.ConsistencyGroup{
				ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
					Meta: &models.ObjMeta{
						ID:      "CG-1",
						Version: 1,
					},
				},
				ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
					AccountID: "A-1",
				},
				ConsistencyGroupMutable: models.ConsistencyGroupMutable{
					Name:                "MyCG",
					ApplicationGroupIds: []models.ObjIDMutable{"app"},
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	cgOps := mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cgOps).MinTimes(1)
	mCG := mockmgmtclient.NewConsistencyGroupMatcher(t, consistency_group.NewConsistencyGroupListParams())
	cgOps.EXPECT().ConsistencyGroupList(mCG).Return(res, nil)
	cgl, err := c.ConsistencyGroupList(ctx, mCG.ListParam)
	assert.Equal(ctx, mCG.ListParam.Context)
	assert.Nil(err)
	assert.NotNil(cgl)
	assert.EqualValues(res, cgl)

	// api failure
	apiErr := consistency_group.NewConsistencyGroupListDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("cg list error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	cgOps = mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cgOps).MinTimes(1)
	cgOps.EXPECT().ConsistencyGroupList(mCG).Return(nil, apiErr)
	cgl, err = c.ConsistencyGroupList(ctx, mCG.ListParam)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(cgl)
}

func TestUpdateConsistencyGroupObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &consistency_group.ConsistencyGroupUpdateOK{
		Payload: &models.ConsistencyGroup{
			ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
				Meta: &models.ObjMeta{
					ID:      "CG-1",
					Version: 1,
				},
			},
			ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
				AccountID: "A-1",
			},
			ConsistencyGroupMutable: models.ConsistencyGroupMutable{
				Name:                "CG",
				ApplicationGroupIds: []models.ObjIDMutable{"app"},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	cgOps := mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cgOps).MinTimes(1)
	mCG := mockmgmtclient.NewConsistencyGroupMatcher(t, consistency_group.NewConsistencyGroupUpdateParams().WithID(string(res.Payload.Meta.ID)))
	mCG.UpdateParam.Payload = &res.Payload.ConsistencyGroupMutable
	mCG.UpdateParam.Set = []string{"description", "name"}
	mCG.UpdateParam.Append = []string{"appendArg"}
	mCG.UpdateParam.Remove = []string{"removeArg"}
	mCG.UpdateParam.Context = nil // will be set from call
	cgOps.EXPECT().ConsistencyGroupUpdate(mCG).Return(res, nil).MinTimes(1)
	items := &Updates{}
	items.Set = mCG.UpdateParam.Set
	items.Append = mCG.UpdateParam.Append
	items.Remove = mCG.UpdateParam.Remove
	cgOut, err := c.ConsistencyGroupUpdate(ctx, res.Payload, items)
	assert.Equal(ctx, mCG.UpdateParam.Context)
	assert.Nil(err)
	assert.NotNil(cgOut)
	assert.Equal(res.Payload, cgOut)

	// API error
	apiErr := consistency_group.NewConsistencyGroupUpdateDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("consistency group request update error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	cgOps = mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cgOps).MinTimes(1)
	mCG.UpdateParam.Version = swag.Int32(int32(res.Payload.Meta.Version))
	cgOps.EXPECT().ConsistencyGroupUpdate(mCG).Return(nil, apiErr).MinTimes(1)
	mCG.UpdateParam.Context = nil // will be set from call
	items.Version = 1
	cgOut, err = c.ConsistencyGroupUpdate(ctx, res.Payload, items)
	assert.Equal(ctx, mCG.UpdateParam.Context)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(cgOut)
}

func TestConsistencyGroupUpdater(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	// A fake updater is used to test - this test focuses on updaterArg function calls.
	// Updater is fully tested in TestVolumeSeriesObjUpdater.
	fu := &fakeUpdater{}
	c.updater = fu
	ctx := context.Background()

	cg := &models.ConsistencyGroup{}
	cg.Meta = &models.ObjMeta{ID: "CG-1", Version: 2}

	var mfO *models.ConsistencyGroup
	modifyFn := func(o *models.ConsistencyGroup) (*models.ConsistencyGroup, error) {
		mfO = o
		return nil, nil
	}

	fu.ouRetO = cg
	items := &Updates{Set: []string{"systemTags"}}
	c.ConsistencyGroupUpdater(ctx, "CG-1", modifyFn, items)
	assert.Equal(ctx, fu.ouInCtx)
	assert.Equal("CG-1", fu.ouInOid)
	assert.Equal(items, fu.ouInItems)
	assert.NotNil(fu.ouInUA)
	ua := fu.ouInUA
	assert.Equal("ConsistencyGroup", ua.typeName)
	assert.NotNil(ua.modifyFn)
	assert.NotNil(ua.fetchFn)
	assert.NotNil(ua.updateFn)
	assert.NotNil(ua.metaFn)

	// test modify gets called
	ua.modifyFn(cg)
	assert.Equal(cg, mfO)
	ua.modifyFn(nil)
	assert.Nil(mfO)

	// test meta functionality
	assert.Equal(cg.Meta, ua.metaFn(cg))

	// test fetch invocation
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	cgOps := mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cgOps).MinTimes(1)
	mFCG := mockmgmtclient.NewConsistencyGroupMatcher(t, consistency_group.NewConsistencyGroupFetchParams().WithID("CG-1"))
	cgOps.EXPECT().ConsistencyGroupFetch(mFCG).Return(nil, fmt.Errorf("fetch-error"))
	retF, err := ua.fetchFn(ctx, "CG-1")
	assert.Equal(ctx, mFCG.FetchParam.Context)
	assert.Error(err)
	assert.Regexp("fetch-error", err)
	assert.Nil(retF)

	// test update invocation
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	cgOps = mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	items.Version = int32(cg.Meta.Version)
	mAPI.EXPECT().ConsistencyGroup().Return(cgOps)
	mUCG := mockmgmtclient.NewConsistencyGroupMatcher(t, consistency_group.NewConsistencyGroupUpdateParams().WithID(string(cg.Meta.ID)))
	mUCG.UpdateParam.Payload = &cg.ConsistencyGroupMutable
	mUCG.UpdateParam.Set = []string{"systemTags"}
	mUCG.UpdateParam.Version = swag.Int32(int32(cg.Meta.Version))
	mUCG.UpdateParam.Context = nil // will be set from call
	cgOps.EXPECT().ConsistencyGroupUpdate(mUCG).Return(nil, fmt.Errorf("update-error"))
	retU, err := ua.updateFn(ctx, cg, items)
	assert.Equal(ctx, mUCG.UpdateParam.Context)
	assert.Error(err)
	assert.Regexp("update-error", err)
	assert.Nil(retU)

	// error path
	fu.ouRetErr = fmt.Errorf("updater-error")
	fu.ouRetO = nil
	_, err = c.ConsistencyGroupUpdater(ctx, "CG-1", modifyFn, items)
	assert.Error(err)
	assert.Regexp("updater-error", err)
}

func TestFetchCSPDomainObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	resD := &csp_domain.CspDomainFetchOK{
		Payload: &models.CSPDomain{
			CSPDomainAllOf0: models.CSPDomainAllOf0{
				Meta: &models.ObjMeta{
					ID:      "CSP-DOMAIN-1",
					Version: 1,
				},
				CspDomainType: "AWS",
			},
			CSPDomainMutable: models.CSPDomainMutable{
				Name: "MyCSPDomain",
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	dOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	mD := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainFetchParams().WithID(string(resD.Payload.Meta.ID)))
	dOps.EXPECT().CspDomainFetch(mD).Return(resD, nil).MinTimes(1)
	s, err := c.CSPDomainFetch(ctx, string(resD.Payload.Meta.ID))
	assert.Equal(ctx, mD.FetchParam.Context)
	assert.Nil(err)
	assert.NotNil(s)
	assert.Equal(resD.Payload, s)

	// api Error
	apiErr := csp_domain.NewCspDomainFetchDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("csp domain fetch error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainFetch(mD).Return(nil, apiErr).MinTimes(1)
	s, err = c.CSPDomainFetch(nil, string(resD.Payload.Meta.ID))
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(s)
}

func TestListCSPDomainObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	resD := &csp_domain.CspDomainListOK{
		Payload: []*models.CSPDomain{
			&models.CSPDomain{
				CSPDomainAllOf0: models.CSPDomainAllOf0{
					Meta: &models.ObjMeta{
						ID:      "CSP-DOMAIN-1",
						Version: 1,
					},
					CspDomainType: "AWS",
				},
				CSPDomainMutable: models.CSPDomainMutable{
					Name: "MyCSPDomain",
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	dOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	mD := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	dOps.EXPECT().CspDomainList(mD).Return(resD, nil).MinTimes(1)
	doms, err := c.CSPDomainList(ctx, mD.ListParam)
	assert.Equal(ctx, mD.ListParam.Context)
	assert.Nil(err)
	assert.NotNil(doms)
	assert.EqualValues(resD, doms)

	// api failure
	apiErr := csp_domain.NewCspDomainListDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("csp domain list error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(mD).Return(nil, apiErr).MinTimes(1)
	doms, err = c.CSPDomainList(ctx, mD.ListParam)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(doms)
}

func TestUpdateCspDomainObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()
	res := &csp_domain.CspDomainUpdateOK{
		Payload: &models.CSPDomain{
			CSPDomainAllOf0: models.CSPDomainAllOf0{
				Meta: &models.ObjMeta{
					ID:      "CSP-1",
					Version: 1,
				},
				AccountID:     "account1",
				CspDomainType: "AWS",
			},
			CSPDomainMutable: models.CSPDomainMutable{
				Name:               "MyCSPDomain",
				AuthorizedAccounts: []models.ObjIDMutable{"a-aid1"},
				Tags:               []string{"tag1", "tag2"},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	ops := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(ops).MinTimes(1)
	mC := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainUpdateParams().WithID(string(res.Payload.Meta.ID)))
	mC.UpdateParam.Version = swag.Int32(2)
	mC.UpdateParam.Payload = &res.Payload.CSPDomainMutable
	mC.UpdateParam.Set = []string{"name"}
	mC.UpdateParam.Append = []string{"tags"}
	mC.UpdateParam.Remove = []string{"authorizedAccounts"}
	mC.UpdateParam.Context = nil // will be set from call
	ops.EXPECT().CspDomainUpdate(mC).Return(res, nil).MinTimes(1)
	items := &Updates{}
	items.Set = mC.UpdateParam.Set
	items.Append = mC.UpdateParam.Append
	items.Remove = mC.UpdateParam.Remove
	items.Version = 2
	v, err := c.CSPDomainUpdate(ctx, res.Payload, items)
	assert.Equal(ctx, mC.UpdateParam.Context)
	assert.Nil(err)
	assert.NotNil(v)
	assert.Equal(res.Payload, v)

	// API error
	apiErr := csp_domain.NewCspDomainUpdateDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("domain request update error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	ops = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(ops).MinTimes(1)
	ops.EXPECT().CspDomainUpdate(mC).Return(nil, apiErr).MinTimes(1)
	mC.UpdateParam.Context = nil // will be set from call
	v, err = c.CSPDomainUpdate(ctx, res.Payload, items)
	assert.Equal(ctx, mC.UpdateParam.Context)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(v)
}

func TestUpdaterCspDomainObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	fu := &fakeUpdater{}
	c.updater = fu
	ctx := context.Background()

	cd := &models.CSPDomain{}
	cd.Meta = &models.ObjMeta{ID: "CSP-1", Version: 2}

	var mfO *models.CSPDomain
	modifyFn := func(o *models.CSPDomain) (*models.CSPDomain, error) {
		mfO = o
		return nil, nil
	}

	fu.ouRetO = cd
	items := &Updates{Set: []string{"name"}, Version: 2}
	c.CSPDomainUpdater(ctx, "CSP-1", modifyFn, items)
	assert.Equal(ctx, fu.ouInCtx)
	assert.Equal("CSP-1", fu.ouInOid)
	assert.Equal(items, fu.ouInItems)
	assert.NotNil(fu.ouInUA)
	ua := fu.ouInUA
	assert.Equal("CspDomain", ua.typeName)
	assert.NotNil(ua.modifyFn)
	assert.NotNil(ua.fetchFn)
	assert.NotNil(ua.updateFn)
	assert.NotNil(ua.metaFn)

	// test modify gets called
	ua.modifyFn(cd)
	assert.Equal(cd, mfO)
	ua.modifyFn(nil)
	assert.Nil(mfO)

	// test meta functionality
	assert.Equal(cd.Meta, ua.metaFn(cd))

	// test fetch invocation
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	ops := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(ops).MinTimes(1)
	mFCP := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainFetchParams().WithID("CSP-1"))
	ops.EXPECT().CspDomainFetch(mFCP).Return(nil, fmt.Errorf("fetch-error"))
	retF, err := ua.fetchFn(ctx, "CSP-1")
	assert.Equal(ctx, mFCP.FetchParam.Context)
	assert.Error(err)
	assert.Regexp("fetch-error", err)
	assert.Nil(retF)

	// test update invocation
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	ops = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(ops)
	mUCP := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainUpdateParams().WithID(string(cd.Meta.ID)))
	mUCP.UpdateParam.Payload = &cd.CSPDomainMutable
	mUCP.UpdateParam.Set = []string{"name"}
	mUCP.UpdateParam.Version = swag.Int32(int32(cd.Meta.Version))
	mUCP.UpdateParam.Context = nil // will be set from call
	ops.EXPECT().CspDomainUpdate(mUCP).Return(nil, fmt.Errorf("update-error"))
	retU, err := ua.updateFn(ctx, cd, items)
	assert.Equal(ctx, mUCP.UpdateParam.Context)
	assert.Error(err)
	assert.Regexp("update-error", err)
	assert.Nil(retU)

	// error path
	fu.ouRetErr = fmt.Errorf("updater-error")
	fu.ouRetO = nil
	_, err = c.CSPDomainUpdater(ctx, "CSP-1", modifyFn, items)
	assert.Error(err)
	assert.Regexp("updater-error", err)
}

func TestCreateNodeObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &node.NodeCreateCreated{
		Payload: &models.Node{
			NodeAllOf0: models.NodeAllOf0{
				Meta: &models.ObjMeta{
					ID:      "NODE-1",
					Version: 1,
				},
				NodeIdentifier: "nodeIdentifier",
				ClusterID:      "CL-1",
			},
			NodeMutable: models.NodeMutable{
				Name: "MyNode",
			},
		},
	}
	nIn := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			NodeIdentifier: "nodeIdentifier",
			ClusterID:      "CL-1",
		},
		NodeMutable: models.NodeMutable{
			Name: "MyNode",
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	nOps := mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps).MinTimes(1)
	mN := mockmgmtclient.NewNodeMatcher(t, node.NewNodeCreateParams())
	mN.CreateParam.Payload = nIn
	mN.CreateParam.Context = nil // will be set from call
	nOps.EXPECT().NodeCreate(mN).Return(res, nil).MinTimes(1)
	nOut, err := c.NodeCreate(ctx, nIn)
	assert.Equal(ctx, mN.CreateParam.Context)
	assert.Nil(err)
	assert.NotNil(nOut)
	assert.Equal(res.Payload, nOut)

	// error case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps).MinTimes(1)
	mN.CreateParam.Context = nil // will be set from call
	apiErr := node.NewNodeCreateDefault(400)
	apiErr.Payload = &models.Error{Message: swag.String("create error"), Code: 400}
	nOps.EXPECT().NodeCreate(mN).Return(nil, apiErr).MinTimes(1)
	nOut, err = c.NodeCreate(ctx, nIn)
	assert.Equal(ctx, mN.CreateParam.Context)
	assert.Error(err)
	assert.Regexp("create error", err.Error())
	assert.Nil(nOut)
}

func TestDeleteNodeObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	nodeID := "NODE-1"

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	nOps := mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps).MinTimes(1)
	mN := mockmgmtclient.NewNodeMatcher(t, node.NewNodeDeleteParams().WithID(nodeID))
	nOps.EXPECT().NodeDelete(mN).Return(nil, nil).MinTimes(1)
	err := c.NodeDelete(ctx, nodeID)
	assert.Equal(ctx, mN.DeleteParam.Context)
	assert.Nil(err)

	// api Error
	apiErr := node.NewNodeDeleteDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("node delete error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps).MinTimes(1)
	nOps.EXPECT().NodeDelete(mN).Return(nil, apiErr).MinTimes(1)
	err = c.NodeDelete(ctx, nodeID)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
}

func TestFetchNodeObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	resN := &node.NodeFetchOK{
		Payload: &models.Node{
			NodeAllOf0: models.NodeAllOf0{
				Meta: &models.ObjMeta{
					ID:      "node-1",
					Version: 1,
				},
			},
			NodeMutable: models.NodeMutable{
				Name: "MyNode",
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	nOps := mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps).MinTimes(1)
	mN := mockmgmtclient.NewNodeMatcher(t, node.NewNodeFetchParams().WithID(string(resN.Payload.Meta.ID)))
	nOps.EXPECT().NodeFetch(mN).Return(resN, nil).MinTimes(1)
	s, err := c.NodeFetch(ctx, string(resN.Payload.Meta.ID))
	assert.Equal(ctx, mN.FetchParam.Context)
	assert.Nil(err)
	assert.NotNil(s)
	assert.Equal(resN.Payload, s)

	// api Error
	apiErr := node.NewNodeFetchDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("node fetch error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps).MinTimes(1)
	nOps.EXPECT().NodeFetch(mN).Return(nil, apiErr).MinTimes(1)
	s, err = c.NodeFetch(nil, string(resN.Payload.Meta.ID))
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(s)
}

func TestListNodeObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	resN := &node.NodeListOK{
		Payload: []*models.Node{
			&models.Node{
				NodeAllOf0: models.NodeAllOf0{
					Meta: &models.ObjMeta{
						ID:      "node-1",
						Version: 1,
					},
				},
				NodeMutable: models.NodeMutable{
					Name: "MyNode",
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	nOps := mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps).MinTimes(1)
	mN := mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps.EXPECT().NodeList(mN).Return(resN, nil).MinTimes(1)
	nodes, err := c.NodeList(ctx, mN.ListParam)
	assert.Equal(ctx, mN.ListParam.Context)
	assert.Nil(err)
	assert.NotNil(nodes)
	assert.EqualValues(resN, nodes)

	// api Error
	apiErr := node.NewNodeListDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("node list error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps).MinTimes(1)
	nOps.EXPECT().NodeList(mN).Return(nil, apiErr).MinTimes(1)
	nodes, err = c.NodeList(ctx, mN.ListParam)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(nodes)
}

func TestUpdateNodeObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	resN := &node.NodeUpdateOK{
		Payload: &models.Node{
			NodeAllOf0: models.NodeAllOf0{
				Meta: &models.ObjMeta{
					ID:      "node-1",
					Version: 1,
				},
			},
			NodeMutable: models.NodeMutable{
				Name: "MyNode",
			},
		},
	}

	// success case, no version enforcement, with storageState
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	nOps := mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps).MinTimes(1)
	mN := mockmgmtclient.NewNodeMatcher(t, node.NewNodeUpdateParams().WithID(string(resN.Payload.Meta.ID)))
	mN.UpdateParam.Payload = &resN.Payload.NodeMutable
	mN.UpdateParam.Set = []string{"setArg"}
	mN.UpdateParam.Append = []string{"appendArg"}
	mN.UpdateParam.Remove = []string{"removeArg"}
	mN.UpdateParam.Context = nil // will be set from call
	nOps.EXPECT().NodeUpdate(mN).Return(resN, nil).MinTimes(1)
	items := &Updates{}
	items.Set = mN.UpdateParam.Set
	items.Append = mN.UpdateParam.Append
	items.Remove = mN.UpdateParam.Remove
	n, err := c.NodeUpdate(ctx, resN.Payload, items)
	assert.Equal(ctx, mN.UpdateParam.Context)
	assert.Nil(err)
	assert.NotNil(n)
	assert.Equal(resN.Payload, n)

	// success, with version enforcement, no storage state
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps).MinTimes(1)
	mN.UpdateParam.Version = swag.Int32(int32(resN.Payload.Meta.Version))
	nOps.EXPECT().NodeUpdate(mN).Return(resN, nil).MinTimes(1)
	mN.UpdateParam.Context = nil // will be set from call
	items.Version = int32(resN.Payload.Meta.Version)
	n, err = c.NodeUpdate(ctx, resN.Payload, items)
	assert.Equal(ctx, mN.UpdateParam.Context)
	assert.Nil(err)
	assert.NotNil(n)
	assert.Equal(resN.Payload, n)

	// API error
	apiErr := node.NewNodeUpdateDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("node update error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps).MinTimes(1)
	nOps.EXPECT().NodeUpdate(mN).Return(nil, apiErr).MinTimes(1)
	mN.UpdateParam.Context = nil // will be set from call
	n, err = c.NodeUpdate(ctx, resN.Payload, items)
	assert.Equal(ctx, mN.UpdateParam.Context)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(n)
}

func TestNodeUpdater(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	// A fake updater is used to test - this test focuses on updaterArg function calls.
	// Updater is fully tested in TestVolumeSeriesObjUpdater.
	fu := &fakeUpdater{}
	c.updater = fu
	ctx := context.Background()

	nObj := &models.Node{}
	nObj.Meta = &models.ObjMeta{ID: "NODE-1", Version: 2}

	var mfO *models.Node
	modifyFn := func(o *models.Node) (*models.Node, error) {
		mfO = o
		return nil, nil
	}

	fu.ouRetO = nObj
	items := &Updates{Set: []string{"tags"}, Version: 2}
	c.NodeUpdater(ctx, "NODE-1", modifyFn, items)
	assert.Equal(ctx, fu.ouInCtx)
	assert.Equal("NODE-1", fu.ouInOid)
	assert.Equal(items, fu.ouInItems)
	assert.NotNil(fu.ouInUA)
	ua := fu.ouInUA
	assert.Equal("Node", ua.typeName)
	assert.NotNil(ua.modifyFn)
	assert.NotNil(ua.fetchFn)
	assert.NotNil(ua.updateFn)
	assert.NotNil(ua.metaFn)

	// test modify gets called
	ua.modifyFn(nObj)
	assert.Equal(nObj, mfO)
	ua.modifyFn(nil)
	assert.Nil(mfO)

	// test meta functionality
	assert.Equal(nObj.Meta, ua.metaFn(nObj))

	// test fetch invocation
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	nOps := mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps).MinTimes(1)
	mfN := mockmgmtclient.NewNodeMatcher(t, node.NewNodeFetchParams().WithID("NODE-1"))
	nOps.EXPECT().NodeFetch(mfN).Return(nil, fmt.Errorf("fetch-error"))
	retF, err := ua.fetchFn(ctx, "NODE-1")
	assert.Equal(ctx, mfN.FetchParam.Context)
	assert.Error(err)
	assert.Regexp("fetch-error", err)
	assert.Nil(retF)

	// test update invocation
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	muN := mockmgmtclient.NewNodeMatcher(t, node.NewNodeUpdateParams().WithID(string(nObj.Meta.ID)))
	muN.UpdateParam.Payload = &nObj.NodeMutable
	muN.UpdateParam.Set = []string{"tags"}
	muN.UpdateParam.Version = swag.Int32(int32(nObj.Meta.Version))
	muN.UpdateParam.Context = nil // will be set from call
	nOps.EXPECT().NodeUpdate(muN).Return(nil, fmt.Errorf("update-error"))
	retU, err := ua.updateFn(ctx, nObj, items)
	assert.Equal(ctx, muN.UpdateParam.Context)
	assert.Error(err)
	assert.Regexp("update-error", err)
	assert.Nil(retU)

	// error path
	fu.ouRetErr = fmt.Errorf("updater-error")
	fu.ouRetO = nil
	_, err = c.NodeUpdater(ctx, "NODE-1", modifyFn, items)
	assert.Error(err)
	assert.Regexp("updater-error", err)
}

func TestCreatePoolObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &pool.PoolCreateCreated{
		Payload: &models.Pool{
			PoolAllOf0: models.PoolAllOf0{
				Meta: &models.ObjMeta{
					ID:      "SP-1",
					Version: 1,
				},
			},
			PoolCreateOnce: models.PoolCreateOnce{
				AccountID:           "ownerAccountId",
				AuthorizedAccountID: "authorizedAccountId",
				ClusterID:           "clusterId",
				CspDomainID:         "csp-domain-1",
				CspStorageType:      "Amazon gp2",
				StorageAccessibility: &models.StorageAccessibilityMutable{
					AccessibilityScope:      "CSPDOMAIN",
					AccessibilityScopeObjID: "csp-domain-1",
				},
			},
			PoolMutable: models.PoolMutable{
				PoolMutableAllOf0: models.PoolMutableAllOf0{
					ServicePlanReservations: map[string]models.StorageTypeReservation{},
				},
				PoolCreateMutable: models.PoolCreateMutable{
					SystemTags: []string{"stag1", "stag2"},
				},
			},
		},
	}
	vIn := &models.Pool{
		PoolCreateOnce: res.Payload.PoolCreateOnce,
		PoolMutable:    res.Payload.PoolMutable,
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spaOps := mockmgmtclient.NewMockPoolClient(mockCtrl)
	mAPI.EXPECT().Pool().Return(spaOps).MinTimes(1)
	mSPA := mockmgmtclient.NewPoolMatcher(t, pool.NewPoolCreateParams())
	mSPA.CreateParam.Payload = &models.PoolCreateArgs{
		PoolCreateOnce:    vIn.PoolCreateOnce,
		PoolCreateMutable: vIn.PoolCreateMutable,
	}
	mSPA.CreateParam.Context = nil // will be set from call
	spaOps.EXPECT().PoolCreate(mSPA).Return(res, nil).MinTimes(1)
	vOut, err := c.PoolCreate(ctx, vIn)
	assert.Equal(ctx, mSPA.CreateParam.Context)
	assert.Nil(err)
	assert.NotNil(vOut)
	assert.Equal(res.Payload, vOut)

	// error case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spaOps = mockmgmtclient.NewMockPoolClient(mockCtrl)
	mAPI.EXPECT().Pool().Return(spaOps).MinTimes(1)
	mSPA.CreateParam.Context = nil // will be set from call
	apiErr := pool.NewPoolCreateDefault(400)
	apiErr.Payload = &models.Error{Message: swag.String("create error"), Code: 400}
	spaOps.EXPECT().PoolCreate(mSPA).Return(nil, apiErr).MinTimes(1)
	vOut, err = c.PoolCreate(ctx, vIn)
	assert.Equal(ctx, mSPA.CreateParam.Context)
	assert.Error(err)
	assert.Regexp("create error", err.Error())
	assert.Nil(vOut)
}

func TestFetchPoolObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	resSP := &pool.PoolFetchOK{
		Payload: &models.Pool{
			PoolAllOf0: models.PoolAllOf0{
				Meta: &models.ObjMeta{
					ID:      "SP-1",
					Version: 1,
				},
			},
			PoolCreateOnce: models.PoolCreateOnce{
				CspStorageType: "Amazon gp2",
			},
			PoolMutable: models.PoolMutable{
				PoolMutableAllOf0: models.PoolMutableAllOf0{
					ServicePlanReservations: map[string]models.StorageTypeReservation{},
				},
				PoolCreateMutable: models.PoolCreateMutable{
					SystemTags: []string{"stag1", "stag2"},
				},
			},
		},
	}

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps := mockmgmtclient.NewMockPoolClient(mockCtrl)
	mAPI.EXPECT().Pool().Return(spOps).MinTimes(1)
	mSP := mockmgmtclient.NewPoolMatcher(t, pool.NewPoolFetchParams().WithID(string(resSP.Payload.Meta.ID)))
	spOps.EXPECT().PoolFetch(mSP).Return(resSP, nil).MinTimes(1)
	sp, err := c.PoolFetch(ctx, string(resSP.Payload.Meta.ID))
	assert.Equal(ctx, mSP.FetchParam.Context)
	assert.Nil(err)
	assert.NotNil(sp)
	assert.Equal(resSP.Payload, sp)

	// api error
	apiErr := pool.NewPoolFetchDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("pool fetch error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps = mockmgmtclient.NewMockPoolClient(mockCtrl)
	mAPI.EXPECT().Pool().Return(spOps).MinTimes(1)
	spOps.EXPECT().PoolFetch(mSP).Return(nil, apiErr).MinTimes(1)
	sp, err = c.PoolFetch(ctx, string(resSP.Payload.Meta.ID))
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(sp)
}

func TestListPoolObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	resSP := &pool.PoolListOK{
		Payload: []*models.Pool{
			&models.Pool{
				PoolAllOf0: models.PoolAllOf0{
					Meta: &models.ObjMeta{
						ID:      "PROV-1",
						Version: 1,
					},
				},
				PoolMutable: models.PoolMutable{
					PoolMutableAllOf0: models.PoolMutableAllOf0{
						ServicePlanReservations: map[string]models.StorageTypeReservation{},
					},
					PoolCreateMutable: models.PoolCreateMutable{
						SystemTags: []string{"stag1", "stag2"},
					},
				},
			},
			&models.Pool{
				PoolAllOf0: models.PoolAllOf0{
					Meta: &models.ObjMeta{
						ID:      "PROV-2",
						Version: 3,
					},
				},
				PoolMutable: models.PoolMutable{
					PoolMutableAllOf0: models.PoolMutableAllOf0{
						ServicePlanReservations: map[string]models.StorageTypeReservation{},
					},
					PoolCreateMutable: models.PoolCreateMutable{
						SystemTags: []string{"stag1", "stag2"},
					},
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps := mockmgmtclient.NewMockPoolClient(mockCtrl)
	mAPI.EXPECT().Pool().Return(spOps).MinTimes(1)
	mSP := mockmgmtclient.NewPoolMatcher(t, pool.NewPoolListParams())
	spOps.EXPECT().PoolList(mSP).Return(resSP, nil).MinTimes(1)
	sps, err := c.PoolList(ctx, mSP.ListParam)
	assert.Equal(ctx, mSP.ListParam.Context)
	assert.Nil(err)
	assert.NotNil(sps)
	assert.Equal(resSP, sps)

	// api failure
	apiErr := pool.NewPoolListDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("pool list error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps = mockmgmtclient.NewMockPoolClient(mockCtrl)
	mAPI.EXPECT().Pool().Return(spOps).MinTimes(1)
	mSP.ListParam.Context = nil // will be set from call
	spOps.EXPECT().PoolList(mSP).Return(nil, apiErr).MinTimes(1)
	sps, err = c.PoolList(ctx, mSP.ListParam)
	assert.Equal(ctx, mSP.ListParam.Context)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(sps)
}

func TestDeletePoolObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	servicePlanAllocationID := "SPA-1"

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spaOps := mockmgmtclient.NewMockPoolClient(mockCtrl)
	mAPI.EXPECT().Pool().Return(spaOps).MinTimes(1)
	mSPA := mockmgmtclient.NewPoolMatcher(t, pool.NewPoolDeleteParams().WithID(servicePlanAllocationID))
	spaOps.EXPECT().PoolDelete(mSPA).Return(nil, nil).MinTimes(1)
	err := c.PoolDelete(ctx, servicePlanAllocationID)
	assert.Equal(ctx, mSPA.DeleteParam.Context)
	assert.Nil(err)

	// api Error
	apiErr := pool.NewPoolDeleteDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("servicePlanAllocation delete error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spaOps = mockmgmtclient.NewMockPoolClient(mockCtrl)
	mAPI.EXPECT().Pool().Return(spaOps).MinTimes(1)
	spaOps.EXPECT().PoolDelete(mSPA).Return(nil, apiErr).MinTimes(1)
	err = c.PoolDelete(ctx, servicePlanAllocationID)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
}

func TestUpdatePoolObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	resSP := &pool.PoolUpdateOK{
		Payload: &models.Pool{
			PoolAllOf0: models.PoolAllOf0{
				Meta: &models.ObjMeta{
					ID:      "SP-1",
					Version: 1,
				},
			},
			PoolCreateOnce: models.PoolCreateOnce{
				CspStorageType: "Amazon gp2",
			},
			PoolMutable: models.PoolMutable{
				PoolMutableAllOf0: models.PoolMutableAllOf0{
					ServicePlanReservations: map[string]models.StorageTypeReservation{},
				},
				PoolCreateMutable: models.PoolCreateMutable{
					SystemTags: []string{"stag1", "stag2"},
				},
			},
		},
	}

	// success case, no version enforcement, with storageState
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps := mockmgmtclient.NewMockPoolClient(mockCtrl)
	mAPI.EXPECT().Pool().Return(spOps).MinTimes(1)
	mSP := mockmgmtclient.NewPoolMatcher(t, pool.NewPoolUpdateParams().WithID(string(resSP.Payload.Meta.ID)))
	mSP.UpdateParam.Payload = &resSP.Payload.PoolMutable
	mSP.UpdateParam.Set = []string{"availableCapacityBytes", "poolState"}
	mSP.UpdateParam.Append = []string{"appendArg"}
	mSP.UpdateParam.Remove = []string{"removeArg"}
	mSP.UpdateParam.Context = nil // will be set from call
	spOps.EXPECT().PoolUpdate(mSP).Return(resSP, nil).MinTimes(1)
	items := &Updates{}
	items.Set = mSP.UpdateParam.Set
	items.Append = mSP.UpdateParam.Append
	items.Remove = mSP.UpdateParam.Remove
	sp, err := c.PoolUpdate(ctx, resSP.Payload, items)
	assert.Equal(ctx, mSP.UpdateParam.Context)
	assert.Nil(err)
	assert.NotNil(sp)
	assert.Equal(resSP.Payload, sp)

	// success, with version enforcement, no storage state
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps = mockmgmtclient.NewMockPoolClient(mockCtrl)
	mAPI.EXPECT().Pool().Return(spOps).MinTimes(1)
	mSP.UpdateParam.Version = swag.Int32(int32(resSP.Payload.Meta.Version))
	spOps.EXPECT().PoolUpdate(mSP).Return(resSP, nil).MinTimes(1)
	mSP.UpdateParam.Context = nil // will be set from call
	items.Version = int32(resSP.Payload.Meta.Version)
	sp, err = c.PoolUpdate(ctx, resSP.Payload, items)
	assert.Equal(ctx, mSP.UpdateParam.Context)
	assert.Nil(err)
	assert.NotNil(sp)
	assert.Equal(resSP.Payload, sp)

	// API error
	apiErr := pool.NewPoolUpdateDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("pool update error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps = mockmgmtclient.NewMockPoolClient(mockCtrl)
	mAPI.EXPECT().Pool().Return(spOps).MinTimes(1)
	spOps.EXPECT().PoolUpdate(mSP).Return(nil, apiErr).MinTimes(1)
	mSP.UpdateParam.Context = nil // will be set from call
	sp, err = c.PoolUpdate(ctx, resSP.Payload, items)
	assert.Equal(ctx, mSP.UpdateParam.Context)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(sp)
}

func TestPoolUpdater(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	// A fake updater is used to test - this test focuses on updaterArg function calls.
	// Updater is fully tested in TestVolumeSeriesObjUpdater.
	fu := &fakeUpdater{}
	c.updater = fu
	ctx := context.Background()

	spa := &models.Pool{}
	spa.Meta = &models.ObjMeta{ID: "SP-1", Version: 2}

	var mfO *models.Pool
	modifyFn := func(o *models.Pool) (*models.Pool, error) {
		mfO = o
		return nil, nil
	}

	fu.ouRetO = spa
	items := &Updates{Set: []string{"tags"}, Version: 2}
	c.PoolUpdater(ctx, "SP-1", modifyFn, items)
	assert.Equal(ctx, fu.ouInCtx)
	assert.Equal("SP-1", fu.ouInOid)
	assert.Equal(items, fu.ouInItems)
	assert.NotNil(fu.ouInUA)
	ua := fu.ouInUA
	assert.Equal("Pool", ua.typeName)
	assert.NotNil(ua.modifyFn)
	assert.NotNil(ua.fetchFn)
	assert.NotNil(ua.updateFn)
	assert.NotNil(ua.metaFn)

	// test modify gets called
	ua.modifyFn(spa)
	assert.Equal(spa, mfO)
	ua.modifyFn(nil)
	assert.Nil(mfO)

	// test meta functionality
	assert.Equal(spa.Meta, ua.metaFn(spa))

	// test fetch invocation
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps := mockmgmtclient.NewMockPoolClient(mockCtrl)
	mAPI.EXPECT().Pool().Return(spOps).MinTimes(1)
	mFSP := mockmgmtclient.NewPoolMatcher(t, pool.NewPoolFetchParams().WithID("SP-1"))
	spOps.EXPECT().PoolFetch(mFSP).Return(nil, fmt.Errorf("fetch-error"))
	retF, err := ua.fetchFn(ctx, "SP-1")
	assert.Equal(ctx, mFSP.FetchParam.Context)
	assert.Error(err)
	assert.Regexp("fetch-error", err)
	assert.Nil(retF)

	// test update invocation
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps = mockmgmtclient.NewMockPoolClient(mockCtrl)
	mAPI.EXPECT().Pool().Return(spOps)
	mUSP := mockmgmtclient.NewPoolMatcher(t, pool.NewPoolUpdateParams().WithID(string(spa.Meta.ID)))
	mUSP.UpdateParam.Payload = &spa.PoolMutable
	mUSP.UpdateParam.Set = []string{"tags"}
	mUSP.UpdateParam.Version = swag.Int32(int32(spa.Meta.Version))
	mUSP.UpdateParam.Context = nil // will be set from call
	spOps.EXPECT().PoolUpdate(mUSP).Return(nil, fmt.Errorf("update-error"))
	retU, err := ua.updateFn(ctx, spa, items)
	assert.Equal(ctx, mUSP.UpdateParam.Context)
	assert.Error(err)
	assert.Regexp("update-error", err)
	assert.Nil(retU)

	// error path
	fu.ouRetErr = fmt.Errorf("updater-error")
	fu.ouRetO = nil
	_, err = c.PoolUpdater(ctx, "SP-1", modifyFn, items)
	assert.Error(err)
	assert.Regexp("updater-error", err)
}

func TestCreateProtectionDomainObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &protection_domain.ProtectionDomainCreateCreated{
		Payload: &models.ProtectionDomain{
			ProtectionDomainAllOf0: models.ProtectionDomainAllOf0{
				Meta: &models.ObjMeta{
					ID:      "SP-1",
					Version: 1,
				},
			},
			ProtectionDomainCreateOnce: models.ProtectionDomainCreateOnce{
				AccountID:            "ownerAccountId",
				EncryptionAlgorithm:  "ALG",
				EncryptionPassphrase: &models.ValueType{Kind: "SECRET", Value: "mySecret"},
			},
			ProtectionDomainMutable: models.ProtectionDomainMutable{
				Name:        "MyPD",
				Description: "A PD",
				SystemTags:  []string{"stag1", "stag2"},
				Tags:        []string{"tag1"},
			},
		},
	}
	vIn := &models.ProtectionDomain{
		ProtectionDomainCreateOnce: res.Payload.ProtectionDomainCreateOnce,
		ProtectionDomainMutable:    res.Payload.ProtectionDomainMutable,
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spaOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
	mAPI.EXPECT().ProtectionDomain().Return(spaOps).MinTimes(1)
	mSPA := mockmgmtclient.NewProtectionDomainMatcher(t, protection_domain.NewProtectionDomainCreateParams())
	mSPA.CreateParam.Payload = &models.ProtectionDomainCreateArgs{
		ProtectionDomainCreateOnce: vIn.ProtectionDomainCreateOnce,
		ProtectionDomainMutable:    vIn.ProtectionDomainMutable,
	}
	mSPA.CreateParam.Context = nil // will be set from call
	spaOps.EXPECT().ProtectionDomainCreate(mSPA).Return(res, nil).MinTimes(1)
	vOut, err := c.ProtectionDomainCreate(ctx, vIn)
	assert.Equal(ctx, mSPA.CreateParam.Context)
	assert.Nil(err)
	assert.NotNil(vOut)
	assert.Equal(res.Payload, vOut)

	// error case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spaOps = mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
	mAPI.EXPECT().ProtectionDomain().Return(spaOps).MinTimes(1)
	mSPA.CreateParam.Context = nil // will be set from call
	apiErr := protection_domain.NewProtectionDomainCreateDefault(400)
	apiErr.Payload = &models.Error{Message: swag.String("create error"), Code: 400}
	spaOps.EXPECT().ProtectionDomainCreate(mSPA).Return(nil, apiErr).MinTimes(1)
	vOut, err = c.ProtectionDomainCreate(ctx, vIn)
	assert.Equal(ctx, mSPA.CreateParam.Context)
	assert.Error(err)
	assert.Regexp("create error", err.Error())
	assert.Nil(vOut)
}

func TestFetchProtectionDomainObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	resSP := &protection_domain.ProtectionDomainFetchOK{
		Payload: &models.ProtectionDomain{
			ProtectionDomainAllOf0: models.ProtectionDomainAllOf0{
				Meta: &models.ObjMeta{
					ID:      "SP-1",
					Version: 1,
				},
			},
			ProtectionDomainCreateOnce: models.ProtectionDomainCreateOnce{
				AccountID:            "ownerAccountId",
				EncryptionAlgorithm:  "ALG",
				EncryptionPassphrase: &models.ValueType{Kind: "SECRET", Value: "mySecret"},
			},
			ProtectionDomainMutable: models.ProtectionDomainMutable{
				Name:        "MyPD",
				Description: "A PD",
				SystemTags:  []string{"stag1", "stag2"},
				Tags:        []string{"tag1"},
			},
		},
	}

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
	mAPI.EXPECT().ProtectionDomain().Return(spOps).MinTimes(1)
	mSP := mockmgmtclient.NewProtectionDomainMatcher(t, protection_domain.NewProtectionDomainFetchParams().WithID(string(resSP.Payload.Meta.ID)))
	spOps.EXPECT().ProtectionDomainFetch(mSP).Return(resSP, nil).MinTimes(1)
	sp, err := c.ProtectionDomainFetch(ctx, string(resSP.Payload.Meta.ID))
	assert.Equal(ctx, mSP.FetchParam.Context)
	assert.Nil(err)
	assert.NotNil(sp)
	assert.Equal(resSP.Payload, sp)

	// api error
	apiErr := protection_domain.NewProtectionDomainFetchDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("protection_domain fetch error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps = mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
	mAPI.EXPECT().ProtectionDomain().Return(spOps).MinTimes(1)
	spOps.EXPECT().ProtectionDomainFetch(mSP).Return(nil, apiErr).MinTimes(1)
	sp, err = c.ProtectionDomainFetch(ctx, string(resSP.Payload.Meta.ID))
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(sp)
}

func TestListProtectionDomainObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	resSP := &protection_domain.ProtectionDomainListOK{
		Payload: []*models.ProtectionDomain{
			&models.ProtectionDomain{
				ProtectionDomainAllOf0: models.ProtectionDomainAllOf0{
					Meta: &models.ObjMeta{
						ID:      "PD-1",
						Version: 1,
					},
				},
				ProtectionDomainCreateOnce: models.ProtectionDomainCreateOnce{
					AccountID:            "ownerAccountId",
					EncryptionAlgorithm:  "ALG",
					EncryptionPassphrase: &models.ValueType{Kind: "SECRET", Value: "mySecret"},
				},
				ProtectionDomainMutable: models.ProtectionDomainMutable{
					Name:        "MyPD1",
					Description: "A PD",
					SystemTags:  []string{"stag1", "stag2"},
					Tags:        []string{"tag1"},
				},
			},
			&models.ProtectionDomain{
				ProtectionDomainAllOf0: models.ProtectionDomainAllOf0{
					Meta: &models.ObjMeta{
						ID:      "PD-2",
						Version: 1,
					},
				},
				ProtectionDomainCreateOnce: models.ProtectionDomainCreateOnce{
					AccountID:            "ownerAccountId",
					EncryptionAlgorithm:  "ALG",
					EncryptionPassphrase: &models.ValueType{Kind: "SECRET", Value: "mySecret"},
				},
				ProtectionDomainMutable: models.ProtectionDomainMutable{
					Name:        "MyPD2",
					Description: "A PD",
					SystemTags:  []string{"stag1", "stag2"},
					Tags:        []string{"tag1"},
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
	mAPI.EXPECT().ProtectionDomain().Return(spOps).MinTimes(1)
	mSP := mockmgmtclient.NewProtectionDomainMatcher(t, protection_domain.NewProtectionDomainListParams())
	spOps.EXPECT().ProtectionDomainList(mSP).Return(resSP, nil).MinTimes(1)
	sps, err := c.ProtectionDomainList(ctx, mSP.ListParam)
	assert.Equal(ctx, mSP.ListParam.Context)
	assert.Nil(err)
	assert.NotNil(sps)
	assert.Equal(resSP, sps)

	// api failure
	apiErr := protection_domain.NewProtectionDomainListDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("protection_domain list error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps = mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
	mAPI.EXPECT().ProtectionDomain().Return(spOps).MinTimes(1)
	mSP.ListParam.Context = nil // will be set from call
	spOps.EXPECT().ProtectionDomainList(mSP).Return(nil, apiErr).MinTimes(1)
	sps, err = c.ProtectionDomainList(ctx, mSP.ListParam)
	assert.Equal(ctx, mSP.ListParam.Context)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(sps)
}

func TestDeleteProtectionDomainObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	objID := "PD-1"

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spaOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
	mAPI.EXPECT().ProtectionDomain().Return(spaOps).MinTimes(1)
	mSPA := mockmgmtclient.NewProtectionDomainMatcher(t, protection_domain.NewProtectionDomainDeleteParams().WithID(objID))
	spaOps.EXPECT().ProtectionDomainDelete(mSPA).Return(nil, nil).MinTimes(1)
	err := c.ProtectionDomainDelete(ctx, objID)
	assert.Equal(ctx, mSPA.DeleteParam.Context)
	assert.Nil(err)

	// api Error
	apiErr := protection_domain.NewProtectionDomainDeleteDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("pd delete error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spaOps = mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
	mAPI.EXPECT().ProtectionDomain().Return(spaOps).MinTimes(1)
	spaOps.EXPECT().ProtectionDomainDelete(mSPA).Return(nil, apiErr).MinTimes(1)
	err = c.ProtectionDomainDelete(ctx, objID)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
}

func TestUpdateProtectionDomainObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	resSP := &protection_domain.ProtectionDomainUpdateOK{
		Payload: &models.ProtectionDomain{
			ProtectionDomainAllOf0: models.ProtectionDomainAllOf0{
				Meta: &models.ObjMeta{
					ID:      "PD-1",
					Version: 1,
				},
			},
			ProtectionDomainCreateOnce: models.ProtectionDomainCreateOnce{
				AccountID:            "ownerAccountId",
				EncryptionAlgorithm:  "ALG",
				EncryptionPassphrase: &models.ValueType{Kind: "SECRET", Value: "mySecret"},
			},
			ProtectionDomainMutable: models.ProtectionDomainMutable{
				Name:        "MyPD1",
				Description: "A PD",
				SystemTags:  []string{"stag1", "stag2"},
				Tags:        []string{"tag1"},
			},
		},
	}

	// success case, no version enforcement
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
	mAPI.EXPECT().ProtectionDomain().Return(spOps).MinTimes(1)
	mSP := mockmgmtclient.NewProtectionDomainMatcher(t, protection_domain.NewProtectionDomainUpdateParams().WithID(string(resSP.Payload.Meta.ID)))
	mSP.UpdateParam.Payload = &resSP.Payload.ProtectionDomainMutable
	mSP.UpdateParam.Set = []string{"name", "description"}
	mSP.UpdateParam.Append = []string{"systemTags"}
	mSP.UpdateParam.Remove = []string{"tags"}
	mSP.UpdateParam.Context = nil // will be set from call
	spOps.EXPECT().ProtectionDomainUpdate(mSP).Return(resSP, nil).MinTimes(1)
	items := &Updates{}
	items.Set = mSP.UpdateParam.Set
	items.Append = mSP.UpdateParam.Append
	items.Remove = mSP.UpdateParam.Remove
	sp, err := c.ProtectionDomainUpdate(ctx, resSP.Payload, items)
	assert.Equal(ctx, mSP.UpdateParam.Context)
	assert.Nil(err)
	assert.NotNil(sp)
	assert.Equal(resSP.Payload, sp)

	// success, with version enforcement
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps = mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
	mAPI.EXPECT().ProtectionDomain().Return(spOps).MinTimes(1)
	mSP.UpdateParam.Version = swag.Int32(int32(resSP.Payload.Meta.Version))
	spOps.EXPECT().ProtectionDomainUpdate(mSP).Return(resSP, nil).MinTimes(1)
	mSP.UpdateParam.Context = nil // will be set from call
	items.Version = int32(resSP.Payload.Meta.Version)
	sp, err = c.ProtectionDomainUpdate(ctx, resSP.Payload, items)
	assert.Equal(ctx, mSP.UpdateParam.Context)
	assert.Nil(err)
	assert.NotNil(sp)
	assert.Equal(resSP.Payload, sp)

	// API error
	apiErr := protection_domain.NewProtectionDomainUpdateDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("protection_domain update error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps = mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
	mAPI.EXPECT().ProtectionDomain().Return(spOps).MinTimes(1)
	spOps.EXPECT().ProtectionDomainUpdate(mSP).Return(nil, apiErr).MinTimes(1)
	mSP.UpdateParam.Context = nil // will be set from call
	sp, err = c.ProtectionDomainUpdate(ctx, resSP.Payload, items)
	assert.Equal(ctx, mSP.UpdateParam.Context)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(sp)
}

func TestProtectionDomainUpdater(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	// A fake updater is used to test - this test focuses on updaterArg function calls.
	// Updater is fully tested in TestVolumeSeriesObjUpdater.
	fu := &fakeUpdater{}
	c.updater = fu
	ctx := context.Background()

	spa := &models.ProtectionDomain{}
	spa.Meta = &models.ObjMeta{ID: "SP-1", Version: 2}

	var mfO *models.ProtectionDomain
	modifyFn := func(o *models.ProtectionDomain) (*models.ProtectionDomain, error) {
		mfO = o
		return nil, nil
	}

	fu.ouRetO = spa
	items := &Updates{Set: []string{"tags"}, Version: 2}
	c.ProtectionDomainUpdater(ctx, "SP-1", modifyFn, items)
	assert.Equal(ctx, fu.ouInCtx)
	assert.Equal("SP-1", fu.ouInOid)
	assert.Equal(items, fu.ouInItems)
	assert.NotNil(fu.ouInUA)
	ua := fu.ouInUA
	assert.Equal("ProtectionDomain", ua.typeName)
	assert.NotNil(ua.modifyFn)
	assert.NotNil(ua.fetchFn)
	assert.NotNil(ua.updateFn)
	assert.NotNil(ua.metaFn)

	// test modify gets called
	ua.modifyFn(spa)
	assert.Equal(spa, mfO)
	ua.modifyFn(nil)
	assert.Nil(mfO)

	// test meta functionality
	assert.Equal(spa.Meta, ua.metaFn(spa))

	// test fetch invocation
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
	mAPI.EXPECT().ProtectionDomain().Return(spOps).MinTimes(1)
	mFSP := mockmgmtclient.NewProtectionDomainMatcher(t, protection_domain.NewProtectionDomainFetchParams().WithID("SP-1"))
	spOps.EXPECT().ProtectionDomainFetch(mFSP).Return(nil, fmt.Errorf("fetch-error"))
	retF, err := ua.fetchFn(ctx, "SP-1")
	assert.Equal(ctx, mFSP.FetchParam.Context)
	assert.Error(err)
	assert.Regexp("fetch-error", err)
	assert.Nil(retF)

	// test update invocation
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps = mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
	mAPI.EXPECT().ProtectionDomain().Return(spOps)
	mUSP := mockmgmtclient.NewProtectionDomainMatcher(t, protection_domain.NewProtectionDomainUpdateParams().WithID(string(spa.Meta.ID)))
	mUSP.UpdateParam.Payload = &spa.ProtectionDomainMutable
	mUSP.UpdateParam.Set = []string{"tags"}
	mUSP.UpdateParam.Version = swag.Int32(int32(spa.Meta.Version))
	mUSP.UpdateParam.Context = nil // will be set from call
	spOps.EXPECT().ProtectionDomainUpdate(mUSP).Return(nil, fmt.Errorf("update-error"))
	retU, err := ua.updateFn(ctx, spa, items)
	assert.Equal(ctx, mUSP.UpdateParam.Context)
	assert.Error(err)
	assert.Regexp("update-error", err)
	assert.Nil(retU)

	// error path
	fu.ouRetErr = fmt.Errorf("updater-error")
	fu.ouRetO = nil
	_, err = c.ProtectionDomainUpdater(ctx, "SP-1", modifyFn, items)
	assert.Error(err)
	assert.Regexp("updater-error", err)
}

func TestFetchServicePlanObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &service_plan.ServicePlanFetchOK{
		Payload: &models.ServicePlan{
			ServicePlanAllOf0: models.ServicePlanAllOf0{
				Meta: &models.ObjMeta{
					ID: "SERVICE-PLAN-1",
				},
				IoProfile: &models.IoProfile{
					IoPattern:    &models.IoPattern{Name: "sequential", MinSizeBytesAvg: swag.Int32(16384), MaxSizeBytesAvg: swag.Int32(262144)},
					ReadWriteMix: &models.ReadWriteMix{Name: "read-write", MinReadPercent: swag.Int32(30), MaxReadPercent: swag.Int32(70)},
				},
				ProvisioningUnit: &models.ProvisioningUnit{
					IOPS:       swag.Int64(22),
					Throughput: swag.Int64(0),
				},
				VolumeSeriesMinMaxSize: &models.VolumeSeriesMinMaxSize{
					MinSizeBytes: swag.Int64(1 * int64(units.GiB)),
					MaxSizeBytes: swag.Int64(64 * int64(units.TiB)),
				},
				State:               "PUBLISHED",
				SourceServicePlanID: "SERVICE-PLAN-0",
			},
			ServicePlanMutable: models.ServicePlanMutable{
				Name:        "Service Plan Name",
				Description: "Service Plan Description",
				Accounts:    []models.ObjIDMutable{"SystemID"},
				Slos: models.SloListMutable{
					"Availability": models.RestrictedValueType{
						ValueType: models.ValueType{
							Kind:  "STRING",
							Value: "99.999%",
						},
						RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{
							Immutable: true,
						},
					},
				},
				Tags: []string{"tag1", "tag2"},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps := mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps).MinTimes(1)
	mSP := mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanFetchParams().WithID(string(res.Payload.Meta.ID)))
	spOps.EXPECT().ServicePlanFetch(mSP).Return(res, nil).MinTimes(1)
	v, err := c.ServicePlanFetch(ctx, string(res.Payload.Meta.ID))
	assert.Equal(ctx, mSP.FetchParam.Context)
	assert.Nil(err)
	assert.NotNil(v)
	assert.Equal(res.Payload, v)

	// api Error
	apiErr := service_plan.NewServicePlanFetchDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("servicePlan fetch error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps).MinTimes(1)
	spOps.EXPECT().ServicePlanFetch(mSP).Return(nil, apiErr).MinTimes(1)
	v, err = c.ServicePlanFetch(nil, string(res.Payload.Meta.ID))
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(v)
}

func TestListServicePlanObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	resSP := &service_plan.ServicePlanListOK{
		Payload: []*models.ServicePlan{
			&models.ServicePlan{
				ServicePlanAllOf0: models.ServicePlanAllOf0{
					Meta: &models.ObjMeta{
						ID:      "ServicePlan-1",
						Version: 1,
					},
				},
				ServicePlanMutable: models.ServicePlanMutable{
					Name: "MyServicePlan",
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps := mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps).MinTimes(1)
	mN := mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	spOps.EXPECT().ServicePlanList(mN).Return(resSP, nil).MinTimes(1)
	plans, err := c.ServicePlanList(ctx, mN.ListParam)
	assert.Equal(ctx, mN.ListParam.Context)
	assert.Nil(err)
	assert.NotNil(plans)
	assert.EqualValues(resSP, plans)

	// api Error
	apiErr := service_plan.NewServicePlanListDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("node list error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps).MinTimes(1)
	spOps.EXPECT().ServicePlanList(mN).Return(nil, apiErr).MinTimes(1)
	plans, err = c.ServicePlanList(ctx, mN.ListParam)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(plans)
}

func TestUpdateServicePlanObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &service_plan.ServicePlanUpdateOK{
		Payload: &models.ServicePlan{
			ServicePlanAllOf0: models.ServicePlanAllOf0{
				Meta: &models.ObjMeta{
					ID: "SERVICE-PLAN-1",
				},
				IoProfile: &models.IoProfile{
					IoPattern:    &models.IoPattern{Name: "sequential", MinSizeBytesAvg: swag.Int32(16384), MaxSizeBytesAvg: swag.Int32(262144)},
					ReadWriteMix: &models.ReadWriteMix{Name: "read-write", MinReadPercent: swag.Int32(30), MaxReadPercent: swag.Int32(70)},
				},
				ProvisioningUnit: &models.ProvisioningUnit{
					IOPS:       swag.Int64(22),
					Throughput: swag.Int64(0),
				},
				VolumeSeriesMinMaxSize: &models.VolumeSeriesMinMaxSize{
					MinSizeBytes: swag.Int64(1 * int64(units.GiB)),
					MaxSizeBytes: swag.Int64(64 * int64(units.TiB)),
				},
				State:               "PUBLISHED",
				SourceServicePlanID: "SERVICE-PLAN-0",
			},
			ServicePlanMutable: models.ServicePlanMutable{
				Name:        "Service Plan Name",
				Description: "Service Plan Description",
				Accounts:    []models.ObjIDMutable{"SystemID"},
				Slos: models.SloListMutable{
					"Availability": models.RestrictedValueType{
						ValueType: models.ValueType{
							Kind:  "STRING",
							Value: "99.999%",
						},
						RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{
							Immutable: true,
						},
					},
				},
				Tags: []string{"tag1", "tag2"},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps := mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps).MinTimes(1)
	mSP := mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanUpdateParams().WithID(string(res.Payload.Meta.ID)))
	mSP.UpdateParam.Version = swag.Int32(2)
	mSP.UpdateParam.Payload = &res.Payload.ServicePlanMutable
	mSP.UpdateParam.Set = []string{"name"}
	mSP.UpdateParam.Append = []string{"tags"}
	mSP.UpdateParam.Remove = []string{"accounts"}
	mSP.UpdateParam.Context = nil // will be set from call
	spOps.EXPECT().ServicePlanUpdate(mSP).Return(res, nil).MinTimes(1)
	items := &Updates{}
	items.Set = mSP.UpdateParam.Set
	items.Append = mSP.UpdateParam.Append
	items.Remove = mSP.UpdateParam.Remove
	items.Version = 2
	v, err := c.ServicePlanUpdate(ctx, res.Payload, items)
	assert.Equal(ctx, mSP.UpdateParam.Context)
	assert.Nil(err)
	assert.NotNil(v)
	assert.Equal(res.Payload, v)

	// API error
	apiErr := service_plan.NewServicePlanUpdateDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("servicePlan request update error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps).MinTimes(1)
	spOps.EXPECT().ServicePlanUpdate(mSP).Return(nil, apiErr).MinTimes(1)
	mSP.UpdateParam.Context = nil // will be set from call
	v, err = c.ServicePlanUpdate(ctx, res.Payload, items)
	assert.Equal(ctx, mSP.UpdateParam.Context)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(v)
}

func TestUpdaterServicePlanAllocationObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	fu := &fakeUpdater{}
	c.updater = fu
	ctx := context.Background()

	sp := &models.ServicePlan{}
	sp.Meta = &models.ObjMeta{ID: "SP-1", Version: 2}

	var mfO *models.ServicePlan
	modifyFn := func(o *models.ServicePlan) (*models.ServicePlan, error) {
		mfO = o
		return nil, nil
	}

	fu.ouRetO = sp
	items := &Updates{Set: []string{"name"}, Version: 2}
	c.ServicePlanUpdater(ctx, "SP-1", modifyFn, items)
	assert.Equal(ctx, fu.ouInCtx)
	assert.Equal("SP-1", fu.ouInOid)
	assert.Equal(items, fu.ouInItems)
	assert.NotNil(fu.ouInUA)
	ua := fu.ouInUA
	assert.Equal("ServicePlan", ua.typeName)
	assert.NotNil(ua.modifyFn)
	assert.NotNil(ua.fetchFn)
	assert.NotNil(ua.updateFn)
	assert.NotNil(ua.metaFn)

	// test modify gets called
	ua.modifyFn(sp)
	assert.Equal(sp, mfO)
	ua.modifyFn(nil)
	assert.Nil(mfO)

	// test meta functionality
	assert.Equal(sp.Meta, ua.metaFn(sp))

	// test fetch invocation
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps := mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps).MinTimes(1)
	mFSP := mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanFetchParams().WithID("SP-1"))
	spOps.EXPECT().ServicePlanFetch(mFSP).Return(nil, fmt.Errorf("fetch-error"))
	retF, err := ua.fetchFn(ctx, "SP-1")
	assert.Equal(ctx, mFSP.FetchParam.Context)
	assert.Error(err)
	assert.Regexp("fetch-error", err)
	assert.Nil(retF)

	// test update invocation
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps)
	mUSP := mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanUpdateParams().WithID(string(sp.Meta.ID)))
	mUSP.UpdateParam.Payload = &sp.ServicePlanMutable
	mUSP.UpdateParam.Set = []string{"name"}
	mUSP.UpdateParam.Version = swag.Int32(int32(sp.Meta.Version))
	mUSP.UpdateParam.Context = nil // will be set from call
	spOps.EXPECT().ServicePlanUpdate(mUSP).Return(nil, fmt.Errorf("update-error"))
	retU, err := ua.updateFn(ctx, sp, items)
	assert.Equal(ctx, mUSP.UpdateParam.Context)
	assert.Error(err)
	assert.Regexp("update-error", err)
	assert.Nil(retU)

	// error path
	fu.ouRetErr = fmt.Errorf("updater-error")
	fu.ouRetO = nil
	_, err = c.ServicePlanUpdater(ctx, "SP-1", modifyFn, items)
	assert.Error(err)
	assert.Regexp("updater-error", err)
}
func TestCreateServicePlanAllocationObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &service_plan_allocation.ServicePlanAllocationCreateCreated{
		Payload: &models.ServicePlanAllocation{
			ServicePlanAllocationAllOf0: models.ServicePlanAllocationAllOf0{
				Meta: &models.ObjMeta{
					ID:      "spa1",
					Version: 1,
				},
				CspDomainID: "csp-domain-1",
			},
			ServicePlanAllocationCreateOnce: models.ServicePlanAllocationCreateOnce{
				AccountID:           "ownerAccountId",
				AuthorizedAccountID: "authorizedAccountId",
				ClusterID:           "clusterId",
				ServicePlanID:       "servicePlanId",
			},
			ServicePlanAllocationMutable: models.ServicePlanAllocationMutable{
				ServicePlanAllocationMutableAllOf0: models.ServicePlanAllocationMutableAllOf0{
					ReservableCapacityBytes: swag.Int64(200000000000),
				},
				ServicePlanAllocationCreateMutable: models.ServicePlanAllocationCreateMutable{
					Messages:            []*models.TimestampedString{},
					ProvisioningHints:   map[string]models.ValueType{},
					StorageFormula:      "Formula1",
					StorageReservations: map[string]models.StorageTypeReservation{},
					SystemTags:          []string{"stag1", "stag2"},
					Tags:                models.ObjTags{"tag1", "tag2", "tag3"},
					TotalCapacityBytes:  swag.Int64(200000000000),
				},
			},
		},
	}
	vIn := &models.ServicePlanAllocation{
		ServicePlanAllocationCreateOnce: res.Payload.ServicePlanAllocationCreateOnce,
		ServicePlanAllocationMutable:    res.Payload.ServicePlanAllocationMutable,
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spaOps := mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(spaOps).MinTimes(1)
	mSPA := mockmgmtclient.NewServicePlanAllocationMatcher(t, service_plan_allocation.NewServicePlanAllocationCreateParams())
	mSPA.CreateParam.Payload = &models.ServicePlanAllocationCreateArgs{
		ServicePlanAllocationCreateOnce:    vIn.ServicePlanAllocationCreateOnce,
		ServicePlanAllocationCreateMutable: vIn.ServicePlanAllocationCreateMutable,
	}
	mSPA.CreateParam.Context = nil // will be set from call
	spaOps.EXPECT().ServicePlanAllocationCreate(mSPA).Return(res, nil).MinTimes(1)
	vOut, err := c.ServicePlanAllocationCreate(ctx, vIn)
	assert.Equal(ctx, mSPA.CreateParam.Context)
	assert.Nil(err)
	assert.NotNil(vOut)
	assert.Equal(res.Payload, vOut)

	// error case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spaOps = mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(spaOps).MinTimes(1)
	mSPA.CreateParam.Context = nil // will be set from call
	apiErr := service_plan_allocation.NewServicePlanAllocationCreateDefault(400)
	apiErr.Payload = &models.Error{Message: swag.String("create error"), Code: 400}
	spaOps.EXPECT().ServicePlanAllocationCreate(mSPA).Return(nil, apiErr).MinTimes(1)
	vOut, err = c.ServicePlanAllocationCreate(ctx, vIn)
	assert.Equal(ctx, mSPA.CreateParam.Context)
	assert.Error(err)
	assert.Regexp("create error", err.Error())
	assert.Nil(vOut)
}

func TestDeleteServicePlanAllocationObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	servicePlanAllocationID := "SPA-1"

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spaOps := mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(spaOps).MinTimes(1)
	mSPA := mockmgmtclient.NewServicePlanAllocationMatcher(t, service_plan_allocation.NewServicePlanAllocationDeleteParams().WithID(servicePlanAllocationID))
	spaOps.EXPECT().ServicePlanAllocationDelete(mSPA).Return(nil, nil).MinTimes(1)
	err := c.ServicePlanAllocationDelete(ctx, servicePlanAllocationID)
	assert.Equal(ctx, mSPA.DeleteParam.Context)
	assert.Nil(err)

	// api Error
	apiErr := service_plan_allocation.NewServicePlanAllocationDeleteDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("servicePlanAllocation delete error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spaOps = mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(spaOps).MinTimes(1)
	spaOps.EXPECT().ServicePlanAllocationDelete(mSPA).Return(nil, apiErr).MinTimes(1)
	err = c.ServicePlanAllocationDelete(ctx, servicePlanAllocationID)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
}

func TestFetchServicePlanAllocationObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &service_plan_allocation.ServicePlanAllocationFetchOK{
		Payload: &models.ServicePlanAllocation{
			ServicePlanAllocationAllOf0: models.ServicePlanAllocationAllOf0{
				Meta: &models.ObjMeta{
					ID:      "SPA-1",
					Version: 1,
				},
				CspDomainID: "CSP-DOMAIN-1",
			},
			ServicePlanAllocationCreateOnce: models.ServicePlanAllocationCreateOnce{
				AccountID:           "ownerAccountId",
				AuthorizedAccountID: "authorizedAccountId",
				ClusterID:           "clusterId",
				ServicePlanID:       "servicePlanId",
			},
			ServicePlanAllocationMutable: models.ServicePlanAllocationMutable{
				ServicePlanAllocationMutableAllOf0: models.ServicePlanAllocationMutableAllOf0{
					ReservableCapacityBytes: swag.Int64(200000000000),
				},
				ServicePlanAllocationCreateMutable: models.ServicePlanAllocationCreateMutable{
					Messages:            []*models.TimestampedString{},
					ProvisioningHints:   map[string]models.ValueType{},
					StorageFormula:      "Formula1",
					StorageReservations: map[string]models.StorageTypeReservation{},
					SystemTags:          []string{"stag1", "stag2"},
					Tags:                models.ObjTags{"tag1", "tag2", "tag3"},
					TotalCapacityBytes:  swag.Int64(200000000000),
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spaOps := mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(spaOps).MinTimes(1)
	mSPA := mockmgmtclient.NewServicePlanAllocationMatcher(t, service_plan_allocation.NewServicePlanAllocationFetchParams().WithID(string(res.Payload.Meta.ID)))
	spaOps.EXPECT().ServicePlanAllocationFetch(mSPA).Return(res, nil).MinTimes(1)
	v, err := c.ServicePlanAllocationFetch(ctx, string(res.Payload.Meta.ID))
	assert.Equal(ctx, mSPA.FetchParam.Context)
	assert.Nil(err)
	assert.NotNil(v)
	assert.Equal(res.Payload, v)

	// api Error
	apiErr := service_plan_allocation.NewServicePlanAllocationFetchDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("servicePlanAllocation fetch error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spaOps = mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(spaOps).MinTimes(1)
	spaOps.EXPECT().ServicePlanAllocationFetch(mSPA).Return(nil, apiErr).MinTimes(1)
	v, err = c.ServicePlanAllocationFetch(nil, string(res.Payload.Meta.ID))
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(v)
}

func TestListServicePlanAllocationObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &service_plan_allocation.ServicePlanAllocationListOK{
		Payload: []*models.ServicePlanAllocation{
			&models.ServicePlanAllocation{
				ServicePlanAllocationAllOf0: models.ServicePlanAllocationAllOf0{
					Meta: &models.ObjMeta{
						ID:      "SPA-1",
						Version: 1,
					},
					CspDomainID: "CSP-DOMAIN-1",
				},
				ServicePlanAllocationCreateOnce: models.ServicePlanAllocationCreateOnce{
					AccountID:           "ownerAccountId",
					AuthorizedAccountID: "authorizedAccountId",
					ClusterID:           "clusterId",
					ServicePlanID:       "servicePlanId",
				},
				ServicePlanAllocationMutable: models.ServicePlanAllocationMutable{
					ServicePlanAllocationMutableAllOf0: models.ServicePlanAllocationMutableAllOf0{
						ReservableCapacityBytes: swag.Int64(200000000000),
					},
					ServicePlanAllocationCreateMutable: models.ServicePlanAllocationCreateMutable{
						Messages:            []*models.TimestampedString{},
						ProvisioningHints:   map[string]models.ValueType{},
						StorageFormula:      "Formula1",
						StorageReservations: map[string]models.StorageTypeReservation{},
						SystemTags:          []string{"stag1", "stag2"},
						Tags:                models.ObjTags{"tag1", "tag2", "tag3"},
						TotalCapacityBytes:  swag.Int64(200000000000),
					},
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spaOps := mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(spaOps).MinTimes(1)
	mSPA := mockmgmtclient.NewServicePlanAllocationMatcher(t, service_plan_allocation.NewServicePlanAllocationListParams())
	spaOps.EXPECT().ServicePlanAllocationList(mSPA).Return(res, nil).MinTimes(1)
	v, err := c.ServicePlanAllocationList(ctx, mSPA.ListParam)
	assert.Equal(ctx, mSPA.ListParam.Context)
	assert.Nil(err)
	assert.NotNil(v)
	assert.EqualValues(res, v)

	// api failure
	apiErr := service_plan_allocation.NewServicePlanAllocationListDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("servicePlanAllocation list error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spaOps = mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(spaOps).MinTimes(1)
	mSPA.ListParam.Context = nil // will be set from call
	spaOps.EXPECT().ServicePlanAllocationList(mSPA).Return(nil, apiErr).MinTimes(1)
	v, err = c.ServicePlanAllocationList(ctx, mSPA.ListParam)
	assert.Equal(ctx, mSPA.ListParam.Context)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(v)
}

func TestUpdateServicePlanAllocationObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &service_plan_allocation.ServicePlanAllocationUpdateOK{
		Payload: &models.ServicePlanAllocation{
			ServicePlanAllocationAllOf0: models.ServicePlanAllocationAllOf0{
				Meta: &models.ObjMeta{
					ID:      "SPA-1",
					Version: 1,
				},
				CspDomainID: "CSP-DOMAIN-1",
			},
			ServicePlanAllocationCreateOnce: models.ServicePlanAllocationCreateOnce{
				AccountID:           "ownerAccountId",
				AuthorizedAccountID: "authorizedAccountId",
				ClusterID:           "clusterId",
				ServicePlanID:       "servicePlanId",
			},
			ServicePlanAllocationMutable: models.ServicePlanAllocationMutable{
				ServicePlanAllocationMutableAllOf0: models.ServicePlanAllocationMutableAllOf0{
					ReservableCapacityBytes: swag.Int64(200000000000),
				},
				ServicePlanAllocationCreateMutable: models.ServicePlanAllocationCreateMutable{
					Messages:            []*models.TimestampedString{},
					ProvisioningHints:   map[string]models.ValueType{},
					StorageFormula:      "Formula1",
					StorageReservations: map[string]models.StorageTypeReservation{},
					SystemTags:          []string{"stag1", "stag2"},
					Tags:                models.ObjTags{"tag1", "tag2", "tag3"},
					TotalCapacityBytes:  swag.Int64(200000000000),
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spaOps := mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(spaOps).MinTimes(1)
	mSPA := mockmgmtclient.NewServicePlanAllocationMatcher(t, service_plan_allocation.NewServicePlanAllocationUpdateParams().WithID(string(res.Payload.Meta.ID)))
	mSPA.UpdateParam.Version = swag.Int32(2)
	mSPA.UpdateParam.Payload = &res.Payload.ServicePlanAllocationMutable
	mSPA.UpdateParam.Set = []string{"totalCapacityBytes", "reservableCapacityBytes", "reservationState"}
	mSPA.UpdateParam.Append = []string{"systemTags"}
	mSPA.UpdateParam.Remove = []string{"storageReservations"}
	mSPA.UpdateParam.Context = nil // will be set from call
	spaOps.EXPECT().ServicePlanAllocationUpdate(mSPA).Return(res, nil).MinTimes(1)
	items := &Updates{}
	items.Set = mSPA.UpdateParam.Set
	items.Append = mSPA.UpdateParam.Append
	items.Remove = mSPA.UpdateParam.Remove
	items.Version = 2
	v, err := c.ServicePlanAllocationUpdate(ctx, res.Payload, items)
	assert.Equal(ctx, mSPA.UpdateParam.Context)
	assert.Nil(err)
	assert.NotNil(v)
	assert.Equal(res.Payload, v)

	// API error
	apiErr := service_plan_allocation.NewServicePlanAllocationUpdateDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("servicePlanAllocation request update error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spaOps = mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(spaOps).MinTimes(1)
	spaOps.EXPECT().ServicePlanAllocationUpdate(mSPA).Return(nil, apiErr).MinTimes(1)
	mSPA.UpdateParam.Context = nil // will be set from call
	v, err = c.ServicePlanAllocationUpdate(ctx, res.Payload, items)
	assert.Equal(ctx, mSPA.UpdateParam.Context)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(v)
}

func TestServicePlanAllocationUpdater(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	// A fake updater is used to test - this test focuses on updaterArg function calls.
	// Updater is fully tested in TestVolumeSeriesObjUpdater.
	fu := &fakeUpdater{}
	c.updater = fu
	ctx := context.Background()

	spa := &models.ServicePlanAllocation{}
	spa.Meta = &models.ObjMeta{ID: "SPA-1", Version: 2}

	var mfO *models.ServicePlanAllocation
	modifyFn := func(o *models.ServicePlanAllocation) (*models.ServicePlanAllocation, error) {
		mfO = o
		return nil, nil
	}

	fu.ouRetO = spa
	items := &Updates{Set: []string{"reservationState"}, Version: 2}
	c.ServicePlanAllocationUpdater(ctx, "SPA-1", modifyFn, items)
	assert.Equal(ctx, fu.ouInCtx)
	assert.Equal("SPA-1", fu.ouInOid)
	assert.Equal(items, fu.ouInItems)
	assert.NotNil(fu.ouInUA)
	ua := fu.ouInUA
	assert.Equal("ServicePlanAllocation", ua.typeName)
	assert.NotNil(ua.modifyFn)
	assert.NotNil(ua.fetchFn)
	assert.NotNil(ua.updateFn)
	assert.NotNil(ua.metaFn)

	// test modify gets called
	ua.modifyFn(spa)
	assert.Equal(spa, mfO)
	ua.modifyFn(nil)
	assert.Nil(mfO)

	// test meta functionality
	assert.Equal(spa.Meta, ua.metaFn(spa))

	// test fetch invocation
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spaOps := mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(spaOps).MinTimes(1)
	mFetchSPA := mockmgmtclient.NewServicePlanAllocationMatcher(t, service_plan_allocation.NewServicePlanAllocationFetchParams().WithID("SPA-1"))
	spaOps.EXPECT().ServicePlanAllocationFetch(mFetchSPA).Return(nil, fmt.Errorf("fetch-error"))
	retF, err := ua.fetchFn(ctx, "SPA-1")
	assert.Equal(ctx, mFetchSPA.FetchParam.Context)
	assert.Error(err)
	assert.Regexp("fetch-error", err)
	assert.Nil(retF)

	// test update invocation
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	spaOps = mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(spaOps)
	mUpdateSPA := mockmgmtclient.NewServicePlanAllocationMatcher(t, service_plan_allocation.NewServicePlanAllocationUpdateParams().WithID(string(spa.Meta.ID)))
	mUpdateSPA.UpdateParam.Payload = &spa.ServicePlanAllocationMutable
	mUpdateSPA.UpdateParam.Set = []string{"reservationState"}
	mUpdateSPA.UpdateParam.Version = swag.Int32(int32(spa.Meta.Version))
	mUpdateSPA.UpdateParam.Context = nil // will be set from call
	spaOps.EXPECT().ServicePlanAllocationUpdate(mUpdateSPA).Return(nil, fmt.Errorf("update-error"))
	retU, err := ua.updateFn(ctx, spa, items)
	assert.Equal(ctx, mUpdateSPA.UpdateParam.Context)
	assert.Error(err)
	assert.Regexp("update-error", err)
	assert.Nil(retU)

	// error path
	fu.ouRetErr = fmt.Errorf("updater-error")
	fu.ouRetO = nil
	_, err = c.ServicePlanAllocationUpdater(ctx, "SPA-1", modifyFn, items)
	assert.Error(err)
	assert.Regexp("updater-error", err)
}

func TestCreateStorageRequestObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	now := time.Now()
	res := &storage_request.StorageRequestCreateCreated{
		Payload: &models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID:      "SR-1",
					Version: 1,
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				ClusterID:           "CLUSTER-1",
				CompleteByTime:      strfmt.DateTime(now.Add(time.Hour)),
				CspDomainID:         "CSP-DOMAIN-1",
				CspStorageType:      "Amazon gp2",
				MinSizeBytes:        swag.Int64(10737418240),
				PoolID:              "SP-1",
				RequestedOperations: []string{"ATTACH", "FORMAT", "USE"},
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "NEW",
				},
				StorageRequestCreateMutable: models.StorageRequestCreateMutable{
					NodeID:     "NODE-1",
					StorageID:  "S-1",
					SystemTags: []string{"stag"},
				},
			},
		},
	}

	srIn := &models.StorageRequest{
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			ClusterID:           "CLUSTER-1",
			CompleteByTime:      strfmt.DateTime(now.Add(time.Hour)),
			CspDomainID:         "CSP-DOMAIN-1",
			CspStorageType:      "Amazon gp2",
			MinSizeBytes:        swag.Int64(10737418240),
			PoolID:              "SP-1",
			RequestedOperations: []string{"PROVISION", "ATTACH"},
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestCreateMutable: models.StorageRequestCreateMutable{
				NodeID:     "NODE-1",
				StorageID:  "S-1",
				SystemTags: []string{"stag"},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	srOps := mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).MinTimes(1)
	mSR := mockmgmtclient.NewStorageRequestMatcher(t, storage_request.NewStorageRequestCreateParams())
	mSR.CreateParam.Payload = &models.StorageRequestCreateArgs{
		StorageRequestCreateOnce:    srIn.StorageRequestCreateOnce,
		StorageRequestCreateMutable: srIn.StorageRequestCreateMutable,
	}
	mSR.D = time.Hour
	mSR.CreateParam.Context = nil // will be set from call
	srOps.EXPECT().StorageRequestCreate(mSR).Return(res, nil).MinTimes(1)
	srOut, err := c.StorageRequestCreate(ctx, srIn)
	assert.Equal(ctx, mSR.CreateParam.Context)
	assert.Nil(err)
	assert.NotNil(srOut)
	assert.Equal(res.Payload, srOut)

	// error case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).MinTimes(1)
	apiErr := storage_request.NewStorageRequestCreateDefault(400)
	apiErr.Payload = &models.Error{Message: swag.String("create error"), Code: 400}
	srIn.MinSizeBytes = nil
	mSR = mockmgmtclient.NewStorageRequestMatcher(t, storage_request.NewStorageRequestCreateParams())
	mSR.CreateParam.Payload = &models.StorageRequestCreateArgs{
		StorageRequestCreateOnce:    srIn.StorageRequestCreateOnce,
		StorageRequestCreateMutable: srIn.StorageRequestCreateMutable,
	}
	mSR.D = time.Hour
	mSR.CreateParam.Context = nil // will be set from call
	srOps.EXPECT().StorageRequestCreate(mSR).Return(nil, apiErr).MinTimes(1)
	srOut, err = c.StorageRequestCreate(ctx, srIn)
	assert.Equal(ctx, mSR.CreateParam.Context)
	assert.Error(err)
	assert.Regexp("create error", err.Error())
	assert.Nil(srOut)
}

func TestListStorageRequests(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	now := time.Now()
	resSR := &storage_request.StorageRequestListOK{
		Payload: []*models.StorageRequest{
			&models.StorageRequest{
				StorageRequestAllOf0: models.StorageRequestAllOf0{
					Meta: &models.ObjMeta{
						ID:           "fb213ad9-4b9b-44ca-adf0-968babf7bb2c",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				StorageRequestCreateOnce: models.StorageRequestCreateOnce{
					ClusterID:           "CLUSTER-1",
					CompleteByTime:      strfmt.DateTime(now.Add(time.Hour)),
					CspDomainID:         "CSP-DOMAIN-1",
					CspStorageType:      "Amazon gp2",
					MinSizeBytes:        swag.Int64(10737418240),
					PoolID:              "POOL-1",
					RequestedOperations: []string{"PROVISION", "ATTACH"},
				},
				StorageRequestMutable: models.StorageRequestMutable{
					StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
						StorageRequestState: "NEW",
					},
					StorageRequestCreateMutable: models.StorageRequestCreateMutable{
						NodeID: "NODE-1",
					},
				},
			},
			&models.StorageRequest{
				StorageRequestAllOf0: models.StorageRequestAllOf0{
					Meta: &models.ObjMeta{
						ID:           "94e860ec-ac86-428c-95ff-3f8a38bbf2f1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				StorageRequestCreateOnce: models.StorageRequestCreateOnce{
					CompleteByTime:      strfmt.DateTime(now.Add(time.Hour)),
					CspDomainID:         "CSP-DOMAIN-1",
					PoolID:              "POOL-1",
					RequestedOperations: []string{"DETACH", "RELEASE"},
				},
				StorageRequestMutable: models.StorageRequestMutable{
					StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
						StorageRequestState: "NEW",
					},
					StorageRequestCreateMutable: models.StorageRequestCreateMutable{
						StorageID: "STORAGE-1",
					},
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	srOps := mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).MinTimes(1)
	mSR := mockmgmtclient.NewStorageRequestMatcher(t, storage_request.NewStorageRequestListParams().WithIsTerminated(swag.Bool(false)))
	srOps.EXPECT().StorageRequestList(mSR).Return(resSR, nil).MinTimes(1)
	srs, err := c.StorageRequestList(ctx, mSR.ListParam)
	assert.Equal(ctx, mSR.ListParam.Context)
	assert.Nil(err)
	assert.NotNil(srs)
	assert.Equal(resSR, srs)

	// api failure
	apiErr := storage_request.NewStorageRequestListDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("storage request list error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).MinTimes(1)
	mSR = mockmgmtclient.NewStorageRequestMatcher(t, storage_request.NewStorageRequestListParams().WithIsTerminated(swag.Bool(false)))
	mSR.ListParam.StorageID = swag.String("SOME-STORAGE-ID")
	srOps.EXPECT().StorageRequestList(mSR).Return(nil, apiErr).MinTimes(1)
	srs, err = c.StorageRequestList(nil, mSR.ListParam)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(srs)
}

func TestFetchStorageRequest(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	resSR := &storage_request.StorageRequestFetchOK{
		Payload: &models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "SR-1",
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	srOps := mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).MinTimes(1)
	mSR := mockmgmtclient.NewStorageRequestMatcher(t, storage_request.NewStorageRequestFetchParams().WithID(string(resSR.Payload.Meta.ID)))
	srOps.EXPECT().StorageRequestFetch(mSR).Return(resSR, nil).MinTimes(1)
	sr, err := c.StorageRequestFetch(ctx, string(resSR.Payload.Meta.ID))
	assert.Equal(ctx, mSR.FetchParam.Context)
	assert.Nil(err)
	assert.NotNil(sr)
	assert.Equal(resSR.Payload, sr)

	// api Error
	apiErr := storage_request.NewStorageRequestFetchDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("storage request fetch error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).MinTimes(1)
	srOps.EXPECT().StorageRequestFetch(mSR).Return(nil, apiErr).MinTimes(1)
	sr, err = c.StorageRequestFetch(ctx, string(resSR.Payload.Meta.ID))
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(sr)
}

func TestUpdateStorageRequestObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	resSR := &storage_request.StorageRequestUpdateOK{
		Payload: &models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID:      "fb213ad9-4b9b-44ca-adf0-968babf7bb2c",
					Version: 1,
				},
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "NEW",
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	srOps := mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).MinTimes(1)
	mSR := mockmgmtclient.NewStorageRequestMatcher(t, storage_request.NewStorageRequestUpdateParams().WithID(string(resSR.Payload.Meta.ID)))
	mSR.UpdateParam.ID = string(resSR.Payload.Meta.ID)
	mSR.UpdateParam.Payload = &resSR.Payload.StorageRequestMutable
	mSR.UpdateParam.Set = []string{"storageRequestState", "requestMessages", "poolId", "storageId"}
	mSR.UpdateParam.Append = []string{"appendArg"}
	mSR.UpdateParam.Remove = []string{"removeArg"}
	mSR.UpdateParam.Version = int32(resSR.Payload.Meta.Version)
	mSR.UpdateParam.Context = nil // will be set from call
	srOps.EXPECT().StorageRequestUpdate(mSR).Return(resSR, nil).MinTimes(1)
	items := &Updates{}
	items.Set = mSR.UpdateParam.Set
	items.Append = mSR.UpdateParam.Append
	items.Remove = mSR.UpdateParam.Remove
	sr, err := c.StorageRequestUpdate(ctx, resSR.Payload, items)
	assert.Equal(ctx, mSR.UpdateParam.Context)
	assert.Nil(err)
	assert.NotNil(sr)
	assert.Equal(resSR.Payload, sr)

	// API error + additional properties set
	resSR.Payload.RequestMessages = make([]*models.TimestampedString, 0)
	resSR.Payload.PoolID = models.ObjIDMutable("SP-1")
	resSR.Payload.StorageID = models.ObjIDMutable("STORAGE-1")
	apiErr := storage_request.NewStorageRequestUpdateDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("storage request update error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).MinTimes(1)
	mSR.UpdateParam.Set = []string{"storageRequestState", "requestMessages", "poolId", "storageId"}
	srOps.EXPECT().StorageRequestUpdate(mSR).Return(nil, apiErr).MinTimes(1)
	sr, err = c.StorageRequestUpdate(nil, resSR.Payload, items)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(sr)
}

func TestStorageRequestUpdater(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	// A fake updater is used to test - this test focuses on updaterArg function calls.
	// Updater is fully tested in TestVolumeSeriesObjUpdater.
	fu := &fakeUpdater{}
	c.updater = fu
	ctx := context.Background()

	sr := &models.StorageRequest{}
	sr.Meta = &models.ObjMeta{ID: "SR-1", Version: 2}

	var mfO *models.StorageRequest
	modifyFn := func(o *models.StorageRequest) (*models.StorageRequest, error) {
		mfO = o
		return nil, nil
	}

	fu.ouRetO = sr
	items := &Updates{Set: []string{"volumeSeriesRequestClaims"}}
	c.StorageRequestUpdater(ctx, "SR-1", modifyFn, items)
	assert.Equal(ctx, fu.ouInCtx)
	assert.Equal("SR-1", fu.ouInOid)
	assert.Equal(items, fu.ouInItems)
	assert.NotNil(fu.ouInUA)
	ua := fu.ouInUA
	assert.Equal("StorageRequest", ua.typeName)
	assert.NotNil(ua.modifyFn)
	assert.NotNil(ua.fetchFn)
	assert.NotNil(ua.updateFn)
	assert.NotNil(ua.metaFn)

	// test modify gets called
	ua.modifyFn(sr)
	assert.Equal(sr, mfO)
	ua.modifyFn(nil)
	assert.Nil(mfO)

	// test meta functionality
	assert.Equal(sr.Meta, ua.metaFn(sr))

	// test fetch invocation
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	srOps := mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).MinTimes(1)
	mFSR := mockmgmtclient.NewStorageRequestMatcher(t, storage_request.NewStorageRequestFetchParams().WithID("SR-1"))
	srOps.EXPECT().StorageRequestFetch(mFSR).Return(nil, fmt.Errorf("fetch-error"))
	retF, err := ua.fetchFn(ctx, "SR-1")
	assert.Equal(ctx, mFSR.FetchParam.Context)
	assert.Error(err)
	assert.Regexp("fetch-error", err)
	assert.Nil(retF)

	// test update invocation
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps)
	mUSR := mockmgmtclient.NewStorageRequestMatcher(t, storage_request.NewStorageRequestUpdateParams().WithID(string(sr.Meta.ID)))
	mUSR.UpdateParam.Payload = &sr.StorageRequestMutable
	mUSR.UpdateParam.Set = []string{"volumeSeriesRequestClaims"}
	mUSR.UpdateParam.Version = int32(sr.Meta.Version)
	mUSR.UpdateParam.Context = nil // will be set from call
	srOps.EXPECT().StorageRequestUpdate(mUSR).Return(nil, fmt.Errorf("update-error"))
	retU, err := ua.updateFn(ctx, sr, items)
	assert.Equal(ctx, mUSR.UpdateParam.Context)
	assert.Error(err)
	assert.Regexp("update-error", err)
	assert.Nil(retU)

	// error path
	fu.ouRetErr = fmt.Errorf("updater-error")
	fu.ouRetO = nil
	_, err = c.StorageRequestUpdater(ctx, "SR-1", modifyFn, items)
	assert.Error(err)
	assert.Regexp("updater-error", err)
}

func TestCreateSnapshotObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	now := time.Now()
	resS := &snapshot.SnapshotCreateCreated{
		Payload: &models.Snapshot{
			SnapshotAllOf0: models.SnapshotAllOf0{
				Meta: &models.ObjMeta{
					ID:      models.ObjID("objectID"),
					Version: 1,
				},
				AccountID:          "account1",
				ConsistencyGroupID: "cg1",
				PitIdentifier:      "pit1",
				SizeBytes:          12345,
				SnapIdentifier:     "HEAD",
				SnapTime:           strfmt.DateTime(now),
				TenantAccountID:    "tenAcc1",
				VolumeSeriesID:     "vs1",
			},
			SnapshotMutable: models.SnapshotMutable{
				DeleteAfterTime: strfmt.DateTime(now.Add(90 * 24 * time.Hour)),
				Locations: map[string]models.SnapshotLocation{
					"csp-1": {CreationTime: strfmt.DateTime(now), CspDomainID: "csp-1"},
				},
				Messages:   []*models.TimestampedString{&models.TimestampedString{}},
				SystemTags: models.ObjTags{},
				Tags:       models.ObjTags{},
			},
		},
	}
	sIn := &models.Snapshot{
		SnapshotAllOf0: models.SnapshotAllOf0{
			PitIdentifier:  resS.Payload.PitIdentifier,
			SnapIdentifier: resS.Payload.SnapIdentifier,
			SnapTime:       resS.Payload.SnapTime,
			VolumeSeriesID: resS.Payload.VolumeSeriesID,
		},
		SnapshotMutable: models.SnapshotMutable{
			DeleteAfterTime: resS.Payload.DeleteAfterTime,
			Locations:       resS.Payload.Locations,
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	sOps := mockmgmtclient.NewMockSnapshotClient(mockCtrl)
	mAPI.EXPECT().Snapshot().Return(sOps).MinTimes(1)
	mS := mockmgmtclient.NewSnapshotMatcher(t, snapshot.NewSnapshotCreateParams().WithPayload(sIn))
	sOps.EXPECT().SnapshotCreate(mS).Return(resS, nil).MinTimes(1)
	sOut, err := c.SnapshotCreate(ctx, sIn)
	assert.Equal(ctx, mS.CreateParam.Context)
	assert.Nil(err)
	assert.NotNil(sOut)
	assert.Equal(resS.Payload, sOut)

	// error case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	sOps = mockmgmtclient.NewMockSnapshotClient(mockCtrl)
	mAPI.EXPECT().Snapshot().Return(sOps).MinTimes(1)
	mS.CreateParam.Context = nil // will be set from call
	apiErr := snapshot.NewSnapshotCreateDefault(400)
	apiErr.Payload = &models.Error{Message: swag.String("create error"), Code: 400}
	sOps.EXPECT().SnapshotCreate(mS).Return(nil, apiErr).MinTimes(1)
	sOut, err = c.SnapshotCreate(ctx, sIn)
	assert.Equal(ctx, mS.CreateParam.Context)
	assert.Error(err)
	assert.Regexp("create error", err.Error())
	assert.Nil(sOut)
}

func TestListSnapshotObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	now := time.Now()
	resS := &snapshot.SnapshotListOK{
		Payload: []*models.Snapshot{
			&models.Snapshot{
				SnapshotAllOf0: models.SnapshotAllOf0{
					Meta: &models.ObjMeta{
						ID:      "id1",
						Version: 1,
					},
					AccountID:          "accountID",
					ConsistencyGroupID: "c-g-1",
					PitIdentifier:      "pit1",
					SizeBytes:          12345,
					SnapIdentifier:     "HEAD",
					SnapTime:           strfmt.DateTime(now),
					TenantAccountID:    "id2",
					VolumeSeriesID:     "vs1",
				},
				SnapshotMutable: models.SnapshotMutable{
					DeleteAfterTime: strfmt.DateTime(now.Add(90 * 24 * time.Hour)),
					Locations: map[string]models.SnapshotLocation{
						"CSP-DOMAIN-1": {CreationTime: strfmt.DateTime(now), CspDomainID: "CSP-DOMAIN-1"},
					},
					Messages:   []*models.TimestampedString{&models.TimestampedString{Message: "msg1"}},
					SystemTags: models.ObjTags{"stag13", "stag32"},
					Tags:       models.ObjTags{"tag13", "tag32"},
				},
			},
			&models.Snapshot{
				SnapshotAllOf0: models.SnapshotAllOf0{
					Meta: &models.ObjMeta{
						ID:      "id2",
						Version: 1,
					},
					AccountID:          "accountID",
					ConsistencyGroupID: "c-g-1",
					PitIdentifier:      "pit1",
					SizeBytes:          12345,
					SnapIdentifier:     "HEAD",
					SnapTime:           strfmt.DateTime(now.Add(-1 * time.Hour)),
					TenantAccountID:    "id2",
					VolumeSeriesID:     "vs1",
				},
				SnapshotMutable: models.SnapshotMutable{
					DeleteAfterTime: strfmt.DateTime(now.Add(120 * 24 * time.Hour)),
					Locations: map[string]models.SnapshotLocation{
						"CSP-DOMAIN-1": {CreationTime: strfmt.DateTime(now.Add(-2 * time.Hour)), CspDomainID: "CSP-DOMAIN-1"},
					},
					Messages:   []*models.TimestampedString{&models.TimestampedString{Message: "msg2"}},
					SystemTags: models.ObjTags{"stag21", "stag22"},
					Tags:       models.ObjTags{"tag21", "tag22"},
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	sOps := mockmgmtclient.NewMockSnapshotClient(mockCtrl)
	mAPI.EXPECT().Snapshot().Return(sOps).MinTimes(1)
	mS := mockmgmtclient.NewSnapshotMatcher(t, snapshot.NewSnapshotListParams())
	sOps.EXPECT().SnapshotList(mS).Return(resS, nil).MinTimes(1)
	s, err := c.SnapshotList(ctx, mS.ListParam)
	assert.Equal(ctx, mS.ListParam.Context)
	assert.Nil(err)
	assert.NotNil(s)
	assert.EqualValues(resS, s)

	// api failure
	apiErr := snapshot.NewSnapshotListDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("snapshot list error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	sOps = mockmgmtclient.NewMockSnapshotClient(mockCtrl)
	mAPI.EXPECT().Snapshot().Return(sOps).MinTimes(1)
	mS.ListParam.Context = nil // will be set from call
	sOps.EXPECT().SnapshotList(mS).Return(nil, apiErr).MinTimes(1)
	s, err = c.SnapshotList(ctx, mS.ListParam)
	assert.Equal(ctx, mS.ListParam.Context)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(s)
}

func TestDeleteSnapshotObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	snapshotID := "SNAP-1"

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	sOps := mockmgmtclient.NewMockSnapshotClient(mockCtrl)
	mAPI.EXPECT().Snapshot().Return(sOps).MinTimes(1)
	mS := mockmgmtclient.NewSnapshotMatcher(t, snapshot.NewSnapshotDeleteParams().WithID(snapshotID))
	sOps.EXPECT().SnapshotDelete(mS).Return(nil, nil).MinTimes(1)
	err := c.SnapshotDelete(ctx, snapshotID)
	assert.Equal(ctx, mS.DeleteParam.Context)
	assert.Nil(err)

	// api Error
	apiErr := snapshot.NewSnapshotDeleteDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("snapshot delete error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	sOps = mockmgmtclient.NewMockSnapshotClient(mockCtrl)
	mAPI.EXPECT().Snapshot().Return(sOps).MinTimes(1)
	sOps.EXPECT().SnapshotDelete(mS).Return(nil, apiErr).MinTimes(1)
	err = c.SnapshotDelete(ctx, snapshotID)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
}
func TestCreateStorageObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	resS := &storage.StorageCreateCreated{
		Payload: &models.Storage{
			StorageAllOf0: models.StorageAllOf0{
				Meta: &models.ObjMeta{
					ID:      "STORAGE-1",
					Version: 1,
				},
				CspDomainID:    "CSP-DOMAIN-1",
				CspStorageType: "Amazon gp2",
				SizeBytes:      swag.Int64(107374182400),
				StorageAccessibility: &models.StorageAccessibility{
					StorageAccessibilityMutable: models.StorageAccessibilityMutable{
						AccessibilityScope:      "CSPDOMAIN",
						AccessibilityScopeObjID: "CSP-DOMAIN-1",
					},
				},
				PoolID: "SP-1",
			},
			StorageMutable: models.StorageMutable{
				AvailableBytes:    swag.Int64(10737418240),
				StorageIdentifier: "vol:volume-1",
				StorageState: &models.StorageStateMutable{
					AttachmentState:  "DETACHED",
					ProvisionedState: "PROVISIONING",
				},
			},
		},
	}
	sIn := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			SizeBytes:      resS.Payload.SizeBytes,
			CspStorageType: resS.Payload.CspStorageType,
			PoolID:         resS.Payload.PoolID,
		},
		StorageMutable: models.StorageMutable{
			AvailableBytes: resS.Payload.AvailableBytes,
			StorageState:   resS.Payload.StorageState,
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	sOps := mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(sOps).MinTimes(1)
	mS := mockmgmtclient.NewStorageMatcher(t, storage.NewStorageCreateParams().WithPayload(sIn))
	sOps.EXPECT().StorageCreate(mS).Return(resS, nil).MinTimes(1)
	sOut, err := c.StorageCreate(ctx, sIn)
	assert.Equal(ctx, mS.CreateParam.Context)
	assert.Nil(err)
	assert.NotNil(sOut)
	assert.Equal(resS.Payload, sOut)

	// error case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	sOps = mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(sOps).MinTimes(1)
	mS.CreateParam.Context = nil // will be set from call
	apiErr := storage.NewStorageCreateDefault(400)
	apiErr.Payload = &models.Error{Message: swag.String("create error"), Code: 400}
	sOps.EXPECT().StorageCreate(mS).Return(nil, apiErr).MinTimes(1)
	sOut, err = c.StorageCreate(ctx, sIn)
	assert.Equal(ctx, mS.CreateParam.Context)
	assert.Error(err)
	assert.Regexp("create error", err.Error())
	assert.Nil(sOut)
}

func TestFetchStorageObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	resS := &storage.StorageFetchOK{
		Payload: &models.Storage{
			StorageAllOf0: models.StorageAllOf0{
				Meta: &models.ObjMeta{
					ID:      "STORAGE-1",
					Version: 1,
				},
				CspDomainID:    "CSP-DOMAIN-1",
				CspStorageType: "Amazon gp2",
				SizeBytes:      swag.Int64(107374182400),
				StorageAccessibility: &models.StorageAccessibility{
					StorageAccessibilityMutable: models.StorageAccessibilityMutable{
						AccessibilityScope:      "CSPDOMAIN",
						AccessibilityScopeObjID: "CSP-DOMAIN-1",
					},
				},
				PoolID: "SP-1",
			},
			StorageMutable: models.StorageMutable{
				AvailableBytes:    swag.Int64(10737418240),
				StorageIdentifier: "vol:volume-1",
				StorageState: &models.StorageStateMutable{
					AttachedNodeDevice: "/dev/xvb0",
					AttachedNodeID:     "NODE-1",
					AttachmentState:    "DETACHED",
					ProvisionedState:   "PROVISIONED",
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	sOps := mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(sOps).MinTimes(1)
	mS := mockmgmtclient.NewStorageMatcher(t, storage.NewStorageFetchParams().WithID(string(resS.Payload.Meta.ID)))
	sOps.EXPECT().StorageFetch(mS).Return(resS, nil).MinTimes(1)
	s, err := c.StorageFetch(ctx, string(resS.Payload.Meta.ID))
	assert.Equal(ctx, mS.FetchParam.Context)
	assert.Nil(err)
	assert.NotNil(s)
	assert.Equal(resS.Payload, s)

	// api Error
	apiErr := storage.NewStorageFetchDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("storage fetch error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	sOps = mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(sOps).MinTimes(1)
	sOps.EXPECT().StorageFetch(mS).Return(nil, apiErr).MinTimes(1)
	s, err = c.StorageFetch(nil, string(resS.Payload.Meta.ID))
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(s)
}

func TestListStorageObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	resS := &storage.StorageListOK{
		Payload: []*models.Storage{
			&models.Storage{
				StorageAllOf0: models.StorageAllOf0{
					Meta: &models.ObjMeta{
						ID:      "STORAGE-1",
						Version: 1,
					},
					CspDomainID:    "CSP-DOMAIN-1",
					CspStorageType: "Amazon gp2",
					SizeBytes:      swag.Int64(107374182400),
					StorageAccessibility: &models.StorageAccessibility{
						StorageAccessibilityMutable: models.StorageAccessibilityMutable{
							AccessibilityScope:      "CSPDOMAIN",
							AccessibilityScopeObjID: "CSP-DOMAIN-1",
						},
					},
					PoolID: "SP-1",
				},
				StorageMutable: models.StorageMutable{
					AvailableBytes:    swag.Int64(10737418240),
					StorageIdentifier: "vol:volume-1",
					StorageState: &models.StorageStateMutable{
						AttachedNodeDevice: "/dev/xvb0",
						AttachedNodeID:     "NODE-1",
						AttachmentState:    "DETACHED",
						ProvisionedState:   "PROVISIONED",
					},
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	sOps := mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(sOps).MinTimes(1)
	mS := mockmgmtclient.NewStorageMatcher(t, storage.NewStorageListParams())
	sOps.EXPECT().StorageList(mS).Return(resS, nil).MinTimes(1)
	s, err := c.StorageList(ctx, mS.ListParam)
	assert.Equal(ctx, mS.ListParam.Context)
	assert.Nil(err)
	assert.NotNil(s)
	assert.EqualValues(resS, s)

	// api failure
	apiErr := storage.NewStorageListDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("storage list error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	sOps = mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(sOps).MinTimes(1)
	mS.ListParam.Context = nil // will be set from call
	sOps.EXPECT().StorageList(mS).Return(nil, apiErr).MinTimes(1)
	s, err = c.StorageList(ctx, mS.ListParam)
	assert.Equal(ctx, mS.ListParam.Context)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(s)
}

func TestDeleteStorageObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	storageID := "STORAGE-1"

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	sOps := mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(sOps).MinTimes(1)
	mS := mockmgmtclient.NewStorageMatcher(t, storage.NewStorageDeleteParams().WithID(storageID))
	sOps.EXPECT().StorageDelete(mS).Return(nil, nil).MinTimes(1)
	err := c.StorageDelete(ctx, storageID)
	assert.Equal(ctx, mS.DeleteParam.Context)
	assert.Nil(err)

	// api Error
	apiErr := storage.NewStorageDeleteDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("storage delete error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	sOps = mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(sOps).MinTimes(1)
	sOps.EXPECT().StorageDelete(mS).Return(nil, apiErr).MinTimes(1)
	err = c.StorageDelete(ctx, storageID)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
}

func TestUpdateStorageObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	resS := &storage.StorageUpdateOK{
		Payload: &models.Storage{
			StorageAllOf0: models.StorageAllOf0{
				Meta: &models.ObjMeta{
					ID:      "STORAGE-1",
					Version: 1,
				},
			},
			StorageMutable: models.StorageMutable{
				AvailableBytes:    swag.Int64(10737418240),
				StorageIdentifier: "vol:volume-1",
				StorageState: &models.StorageStateMutable{
					AttachedNodeDevice: "/dev/xvb0",
					AttachedNodeID:     "NODE-1",
					AttachmentState:    "DETACHED",
					ProvisionedState:   "PROVISIONED",
				},
			},
		},
	}

	// success case, with storageState
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	sOps := mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(sOps).MinTimes(1)
	mS := mockmgmtclient.NewStorageMatcher(t, storage.NewStorageUpdateParams().WithID(string(resS.Payload.Meta.ID)))
	mS.UpdateParam.Version = int32(resS.Payload.Meta.Version)
	mS.UpdateParam.Payload = &resS.Payload.StorageMutable
	mS.UpdateParam.Set = []string{"availableBytes", "storageIdentifier", "storageState"}
	mS.UpdateParam.Append = []string{"appendArg"}
	mS.UpdateParam.Remove = []string{"removeArg"}
	mS.UpdateParam.Context = nil // will be set from call
	sOps.EXPECT().StorageUpdate(mS).Return(resS, nil).MinTimes(1)
	items := &Updates{}
	items.Set = mS.UpdateParam.Set
	items.Append = mS.UpdateParam.Append
	items.Remove = mS.UpdateParam.Remove
	s, err := c.StorageUpdate(ctx, resS.Payload, items)
	assert.Equal(ctx, mS.UpdateParam.Context)
	assert.Nil(err)
	assert.NotNil(s)
	assert.Equal(resS.Payload, s)

	// success, no storage state
	resS.Payload.StorageMutable.StorageState = nil
	mS.UpdateParam.Set = []string{"availableBytes", "storageIdentifier"}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	sOps = mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(sOps).MinTimes(1)
	sOps.EXPECT().StorageUpdate(mS).Return(resS, nil).MinTimes(1)
	mS.UpdateParam.Context = nil // will be set from call
	items.Set = mS.UpdateParam.Set
	s, err = c.StorageUpdate(ctx, resS.Payload, items)
	assert.Equal(ctx, mS.UpdateParam.Context)
	assert.Nil(err)
	assert.NotNil(s)
	assert.Equal(resS.Payload, s)

	// API error
	apiErr := storage.NewStorageUpdateDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("storage request update error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	sOps = mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(sOps).MinTimes(1)
	mS.UpdateParam.Version = int32(resS.Payload.Meta.Version)
	sOps.EXPECT().StorageUpdate(mS).Return(nil, apiErr).MinTimes(1)
	mS.UpdateParam.Context = nil // will be set from call
	s, err = c.StorageUpdate(ctx, resS.Payload, items)
	assert.Equal(ctx, mS.UpdateParam.Context)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(s)
}

func TestListStorageFormulaObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &storage_formula.StorageFormulaListOK{
		Payload: []*models.StorageFormula{
			&models.StorageFormula{
				CacheComponent: map[string]models.StorageFormulaTypeElement{
					"Amazon SSD": {Percentage: swag.Int32(20)},
				},
				Description: "formula",
				IoProfile: &models.IoProfile{
					IoPattern:    &models.IoPattern{Name: "random", MinSizeBytesAvg: swag.Int32(0), MaxSizeBytesAvg: swag.Int32(16384)},
					ReadWriteMix: &models.ReadWriteMix{Name: "read-write", MinReadPercent: swag.Int32(30), MaxReadPercent: swag.Int32(70)},
				},
				Name: "formula-e",
				SscList: map[string]models.ValueType{
					"Availability": {
						Kind:  "PERCENTAGE",
						Value: "99.999%",
					},
					"Response Time Average": {
						Kind:  "DURATION",
						Value: "8ms",
					},
				},
				StorageComponent: map[string]models.StorageFormulaTypeElement{
					"Amazon gp2": {Percentage: swag.Int32(100)},
				},
				StorageLayout: "mirrored",
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	fOps := mockmgmtclient.NewMockStorageFormulaClient(mockCtrl)
	mAPI.EXPECT().StorageFormula().Return(fOps).MinTimes(1)
	mF := mockmgmtclient.NewStorageFormulaMatcher(t, storage_formula.NewStorageFormulaListParams())
	fOps.EXPECT().StorageFormulaList(mF).Return(res, nil).MinTimes(1)
	l, err := c.StorageFormulaList(ctx, mF.ListParam)
	assert.Equal(ctx, mF.ListParam.Context)
	assert.Nil(err)
	assert.NotNil(l)
	assert.EqualValues(res, l)

	// api failure
	apiErr := storage_formula.NewStorageFormulaListDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("storage formula list error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	fOps = mockmgmtclient.NewMockStorageFormulaClient(mockCtrl)
	mAPI.EXPECT().StorageFormula().Return(fOps).MinTimes(1)
	mF.ListParam.Context = nil // will be set from call
	fOps.EXPECT().StorageFormulaList(mF).Return(nil, apiErr).MinTimes(1)
	l, err = c.StorageFormulaList(ctx, mF.ListParam)
	assert.Equal(ctx, mF.ListParam.Context)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(l)
}

func TestCreateTaskObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	tIn := &models.Task{
		TaskCreateOnce: models.TaskCreateOnce{
			Operation: "SOME-OP",
			ObjectID:  "OBJ-1",
		},
	}
	resT := &task.TaskCreateCreated{
		Payload: &models.Task{
			TaskAllOf0: models.TaskAllOf0{
				Meta: &models.ObjMeta{
					ID: "TASK-1",
				},
			},
			TaskCreateOnce: tIn.TaskCreateOnce,
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	tOps := mockmgmtclient.NewMockTaskClient(mockCtrl)
	mAPI.EXPECT().Task().Return(tOps).MinTimes(1)
	mT := mockmgmtclient.NewTaskMatcher(t, task.NewTaskCreateParams().WithPayload(&tIn.TaskCreateOnce))
	tOps.EXPECT().TaskCreate(mT).Return(resT, nil).MinTimes(1)
	tOut, err := c.TaskCreate(ctx, tIn)
	assert.Equal(ctx, mT.CreateParam.Context)
	assert.Nil(err)
	assert.NotNil(tOut)
	assert.Equal(resT.Payload, tOut)

	// error case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	tOps = mockmgmtclient.NewMockTaskClient(mockCtrl)
	mAPI.EXPECT().Task().Return(tOps).MinTimes(1)
	mT.CreateParam.Context = nil // will be set from call
	apiErr := task.NewTaskCreateDefault(400)
	apiErr.Payload = &models.Error{Message: swag.String("create error"), Code: 400}
	tOps.EXPECT().TaskCreate(mT).Return(nil, apiErr).MinTimes(1)
	tOut, err = c.TaskCreate(ctx, tIn)
	assert.Equal(ctx, mT.CreateParam.Context)
	assert.Error(err)
	assert.Regexp("create error", err.Error())
	assert.Nil(tOut)
}

func TestCreateVolumeSeriesObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &volume_series.VolumeSeriesCreateCreated{
		Payload: &models.VolumeSeries{
			VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
				Meta: &models.ObjMeta{
					ID:      "vs1",
					Version: 1,
				},
				RootParcelUUID: "rootUUID",
			},
			VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
				AccountID: "systemID",
			},
			VolumeSeriesMutable: models.VolumeSeriesMutable{
				VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
					BoundClusterID:      "",
					CapacityAllocations: map[string]models.CapacityAllocation{},
					Messages:            []*models.TimestampedString{},
					Mounts:              []*models.Mount{},
					RootStorageID:       "",
					StorageParcels:      map[string]models.ParcelAllocation{},
					VolumeSeriesState:   "UNBOUND",
				},
				VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
					ConsistencyGroupID: "same",
					Description:        "description",
					Name:               "name",
					ServicePlanID:      "plan-A",
					SizeBytes:          swag.Int64(107374182400),
					Tags:               []string{},
				},
			},
		},
	}
	vIn := &models.VolumeSeries{
		VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
			AccountID: "systemID",
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				ConsistencyGroupID: res.Payload.ConsistencyGroupID,
				Description:        res.Payload.Description,
				Name:               res.Payload.Name,
				ServicePlanID:      res.Payload.ServicePlanID,
				SizeBytes:          swag.Int64(107374182400),
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vOps := mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vOps).MinTimes(1)
	mV := mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesCreateParams())
	mV.CreateParam.Payload = &models.VolumeSeriesCreateArgs{
		VolumeSeriesCreateOnce:    vIn.VolumeSeriesCreateOnce,
		VolumeSeriesCreateMutable: vIn.VolumeSeriesCreateMutable,
	}
	mV.CreateParam.Context = nil // will be set from call
	vOps.EXPECT().VolumeSeriesCreate(mV).Return(res, nil).MinTimes(1)
	vOut, err := c.VolumeSeriesCreate(ctx, vIn)
	assert.Equal(ctx, mV.CreateParam.Context)
	assert.Nil(err)
	assert.NotNil(vOut)
	assert.Equal(res.Payload, vOut)

	// error case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vOps).MinTimes(1)
	mV.CreateParam.Context = nil // will be set from call
	apiErr := volume_series.NewVolumeSeriesCreateDefault(400)
	apiErr.Payload = &models.Error{Message: swag.String("create error"), Code: 400}
	vOps.EXPECT().VolumeSeriesCreate(mV).Return(nil, apiErr).MinTimes(1)
	vOut, err = c.VolumeSeriesCreate(ctx, vIn)
	assert.Equal(ctx, mV.CreateParam.Context)
	assert.Error(err)
	assert.Regexp("create error", err.Error())
	assert.Nil(vOut)
}

func TestDeleteVolumeSeriesObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	volumeSeriesID := "vs1"

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vOps := mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vOps).MinTimes(1)
	mV := mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesDeleteParams().WithID(volumeSeriesID))
	vOps.EXPECT().VolumeSeriesDelete(mV).Return(nil, nil).MinTimes(1)
	err := c.VolumeSeriesDelete(ctx, volumeSeriesID)
	assert.Equal(ctx, mV.DeleteParam.Context)
	assert.Nil(err)

	// api Error
	apiErr := volume_series.NewVolumeSeriesDeleteDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("volumeSeries delete error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vOps).MinTimes(1)
	vOps.EXPECT().VolumeSeriesDelete(mV).Return(nil, apiErr).MinTimes(1)
	err = c.VolumeSeriesDelete(ctx, volumeSeriesID)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
}

func TestFetchVolumeSeriesObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &volume_series.VolumeSeriesFetchOK{
		Payload: &models.VolumeSeries{
			VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
				Meta: &models.ObjMeta{
					ID:      "vs1",
					Version: 2,
				},
				RootParcelUUID: "rootUUID",
			},
			VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
				AccountID: "systemID",
			},
			VolumeSeriesMutable: models.VolumeSeriesMutable{
				VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
					BoundClusterID:      "",
					CapacityAllocations: map[string]models.CapacityAllocation{},
					Messages:            []*models.TimestampedString{},
					Mounts:              []*models.Mount{},
					RootStorageID:       "",
					StorageParcels:      map[string]models.ParcelAllocation{},
					VolumeSeriesState:   "UNBOUND",
				},
				VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
					ConsistencyGroupID: "same",
					Description:        "description",
					Name:               "name",
					ServicePlanID:      "plan-A",
					SizeBytes:          swag.Int64(107374182400),
					Tags:               []string{},
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vOps := mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vOps).MinTimes(1)
	mV := mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesFetchParams().WithID(string(res.Payload.Meta.ID)))
	vOps.EXPECT().VolumeSeriesFetch(mV).Return(res, nil).MinTimes(1)
	v, err := c.VolumeSeriesFetch(ctx, string(res.Payload.Meta.ID))
	assert.Equal(ctx, mV.FetchParam.Context)
	assert.Nil(err)
	assert.NotNil(v)
	assert.Equal(res.Payload, v)

	// api Error
	apiErr := volume_series.NewVolumeSeriesFetchDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("volumeSeries fetch error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vOps).MinTimes(1)
	vOps.EXPECT().VolumeSeriesFetch(mV).Return(nil, apiErr).MinTimes(1)
	v, err = c.VolumeSeriesFetch(nil, string(res.Payload.Meta.ID))
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(v)
}

func TestVolumeSeriesNewID(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &volume_series.VolumeSeriesNewIDOK{
		Payload: &models.ValueType{
			Kind:  "STRING",
			Value: "VOLUME-ID",
		},
	}

	params := volume_series.NewVolumeSeriesNewIDParams()
	params.SetContext(ctx)

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vOps := mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vOps).MinTimes(1)
	vOps.EXPECT().VolumeSeriesNewID(params).Return(res, nil)
	v, err := c.VolumeSeriesNewID(ctx)
	assert.Nil(err)
	assert.NotNil(v)
	assert.EqualValues(res.Payload, v)

	// api error
	apiErr := volume_series.NewVolumeSeriesNewIDDefault(403)
	apiErr.Payload = &models.Error{
		Code:    403,
		Message: swag.String("unauthorized"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vOps).MinTimes(1)
	vOps.EXPECT().VolumeSeriesNewID(params).Return(nil, apiErr)
	v, err = c.VolumeSeriesNewID(ctx)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(v)
}

func TestListVolumeSeriesObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &volume_series.VolumeSeriesListOK{
		Payload: []*models.VolumeSeries{
			&models.VolumeSeries{
				VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
					Meta: &models.ObjMeta{
						ID:      "vs1",
						Version: 2,
					},
					RootParcelUUID: "rootUUID",
				},
				VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
					AccountID: "systemID",
				},
				VolumeSeriesMutable: models.VolumeSeriesMutable{
					VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
						BoundClusterID:      "",
						CapacityAllocations: map[string]models.CapacityAllocation{},
						Messages:            []*models.TimestampedString{},
						Mounts:              []*models.Mount{},
						RootStorageID:       "",
						StorageParcels:      map[string]models.ParcelAllocation{},
						VolumeSeriesState:   "UNBOUND",
					},
					VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
						ConsistencyGroupID: "same",
						Description:        "description",
						Name:               "name",
						ServicePlanID:      "plan-A",
						SizeBytes:          swag.Int64(107374182400),
						Tags:               []string{},
					},
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vOps := mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vOps).MinTimes(1)
	mV := mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesListParams())
	vOps.EXPECT().VolumeSeriesList(mV).Return(res, nil).MinTimes(1)
	v, err := c.VolumeSeriesList(ctx, mV.ListParam)
	assert.Equal(ctx, mV.ListParam.Context)
	assert.Nil(err)
	assert.NotNil(v)
	assert.EqualValues(res, v)

	// api failure
	apiErr := volume_series.NewVolumeSeriesListDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("volumeSeries list error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vOps).MinTimes(1)
	mV.ListParam.Context = nil // will be set from call
	vOps.EXPECT().VolumeSeriesList(mV).Return(nil, apiErr).MinTimes(1)
	v, err = c.VolumeSeriesList(ctx, mV.ListParam)
	assert.Equal(ctx, mV.ListParam.Context)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(v)
}

func TestUpdateVolumeSeriesObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &volume_series.VolumeSeriesUpdateOK{
		Payload: &models.VolumeSeries{
			VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
				Meta: &models.ObjMeta{
					ID:      "vs1",
					Version: 3,
				},
				RootParcelUUID: "rootUUID",
			},
			VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
				AccountID: "systemID",
			},
			VolumeSeriesMutable: models.VolumeSeriesMutable{
				VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
					BoundClusterID: "cl1",
					CapacityAllocations: map[string]models.CapacityAllocation{
						"prov1": {ConsumedBytes: swag.Int64(0), ReservedBytes: swag.Int64(1111107374182400)},
					},
					Messages:          []*models.TimestampedString{},
					Mounts:            []*models.Mount{},
					RootStorageID:     "",
					StorageParcels:    map[string]models.ParcelAllocation{},
					VolumeSeriesState: "BOUND",
				},
				VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
					ConsistencyGroupID: "same",
					Description:        "description",
					Name:               "name",
					ServicePlanID:      "plan-A",
					SizeBytes:          swag.Int64(107374182400),
					Tags:               []string{},
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vOps := mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vOps).MinTimes(1)
	mV := mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesUpdateParams().WithID(string(res.Payload.Meta.ID)))
	mV.UpdateParam.Version = swag.Int32(2)
	mV.UpdateParam.Payload = &res.Payload.VolumeSeriesMutable
	mV.UpdateParam.Set = []string{"sizeBytes", "boundClusterId", "volumeSeriesState"}
	mV.UpdateParam.Append = []string{"appendArg"}
	mV.UpdateParam.Remove = []string{"removeArg"}
	mV.UpdateParam.Context = nil // will be set from call
	vOps.EXPECT().VolumeSeriesUpdate(mV).Return(res, nil).MinTimes(1)
	items := &Updates{}
	items.Set = mV.UpdateParam.Set
	items.Append = mV.UpdateParam.Append
	items.Remove = mV.UpdateParam.Remove
	items.Version = 2
	v, err := c.VolumeSeriesUpdate(ctx, res.Payload, items)
	assert.Equal(ctx, mV.UpdateParam.Context)
	assert.Nil(err)
	assert.NotNil(v)
	assert.Equal(res.Payload, v)

	// API error
	apiErr := volume_series.NewVolumeSeriesUpdateDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("volumeSeries request update error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vOps).MinTimes(1)
	vOps.EXPECT().VolumeSeriesUpdate(mV).Return(nil, apiErr).MinTimes(1)
	mV.UpdateParam.Context = nil // will be set from call
	v, err = c.VolumeSeriesUpdate(ctx, res.Payload, items)
	assert.Equal(ctx, mV.UpdateParam.Context)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(v)
}

func vsClone(vs *models.VolumeSeries) *models.VolumeSeries {
	var copy *models.VolumeSeries
	testutils.Clone(vs, &copy)
	return copy
}

func TestVolumeSeriesObjUpdater(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ctx := context.Background()
	c := &Client{
		Log: tl.Logger(),
	}
	// This UT tests the real updater
	c.updater = c

	vsObj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID:      "VS-1",
				Version: 1,
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				VolumeSeriesState: "NEW",
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				SizeBytes: swag.Int64(int64(1 * units.GiB)),
			},
		},
	}
	resF := &volume_series.VolumeSeriesFetchOK{Payload: vsClone(vsObj)}
	resV := &volume_series.VolumeSeriesUpdateOK{Payload: vsClone(vsObj)}

	items := &Updates{Set: []string{"volumeSeriesState", "attr-will-be-removed-on-retry"}}
	initCall := 0
	otherCall := 0
	var returnNilOnInit bool
	var abortErr error
	verNum := int32(0)
	var vsLast *models.VolumeSeries
	modVS := func(vs *models.VolumeSeries) (*models.VolumeSeries, error) {
		if vs == nil {
			initCall++
			if returnNilOnInit {
				return nil, nil
			}
		} else {
			otherCall++
		}
		if abortErr != nil {
			return nil, abortErr
		}
		items.Set = []string{"volumeSeriesState"} // adjust items in modify callback
		verNum++
		vsLast = vsClone(vsObj)
		vsLast.Meta.Version = models.ObjVersion(verNum)
		return vsLast, nil
	}

	// retry on ErrorIDVerNotFound error
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vOps := mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vOps).MinTimes(1)
	mF := mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesFetchParams().WithID(string(vsObj.Meta.ID)))
	vOps.EXPECT().VolumeSeriesFetch(mF).Return(resF, nil).MinTimes(1)
	mU := mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesUpdateParams().WithID(string(vsObj.Meta.ID)))
	mU.UpdateParam.Version = &verNum
	mU.UpdateParam.Payload = &vsObj.VolumeSeriesMutable
	mU.UpdateParam.Set = []string{"volumeSeriesState"}
	mU.UpdateParam.Context = nil // will be set from call
	op1 := vOps.EXPECT().VolumeSeriesUpdate(mU).Return(nil, fmt.Errorf("%s", com.ErrorIDVerNotFound)).MaxTimes(1)
	vOps.EXPECT().VolumeSeriesUpdate(mU).Return(nil, fmt.Errorf("%s", "other error")).MinTimes(1).After(op1)
	_, err := c.VolumeSeriesUpdater(ctx, string(vsObj.Meta.ID), modVS, items)
	assert.Error(err)
	assert.Equal("other error", err.Error())
	assert.Equal(1, initCall)
	tl.Flush()

	// no retry on other update error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vOps).MinTimes(1)
	vOps.EXPECT().VolumeSeriesUpdate(mU).Return(nil, fmt.Errorf("other-error"))
	initCall = 0
	otherCall = 0
	_, err = c.VolumeSeriesUpdater(ctx, string(vsObj.Meta.ID), modVS, items)
	assert.Error(err)
	assert.Regexp("other-error", err)
	assert.Equal(1, initCall)
	assert.Equal(0, otherCall)
	tl.Flush()

	// fail on a fetch error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vOps).MinTimes(1)
	vOps.EXPECT().VolumeSeriesFetch(mF).Return(nil, fmt.Errorf("vs-fetch-error"))
	returnNilOnInit = true
	initCall = 0
	otherCall = 0
	_, err = c.VolumeSeriesUpdater(ctx, string(vsObj.Meta.ID), modVS, items)
	assert.Error(err)
	assert.Regexp("vs-fetch-error", err)
	assert.Equal(1, initCall)
	assert.Equal(0, otherCall)
	tl.Flush()

	// success no retry
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vOps).MinTimes(1)
	vOps.EXPECT().VolumeSeriesUpdate(mU).Return(resV, nil)
	returnNilOnInit = false
	initCall = 0
	otherCall = 0
	obj, err := c.VolumeSeriesUpdater(ctx, string(vsObj.Meta.ID), modVS, items)
	assert.Equal(vsObj, obj)
	assert.NoError(err)
	assert.Equal(1, initCall)
	assert.Equal(0, otherCall)
	tl.Flush()

	// wrong vsID
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	initCall = 0
	otherCall = 0
	_, err = c.VolumeSeriesUpdater(ctx, "wrong", modVS, items)
	if assert.Error(err) {
		assert.Regexp("invalid object", err)
	}
	assert.Equal(1, initCall)
	assert.Equal(0, otherCall)
	tl.Flush()

	// abort operation on initial CB error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	initCall = 0
	otherCall = 0
	abortErr = fmt.Errorf("abort")
	obj, err = c.VolumeSeriesUpdater(ctx, string(vsObj.Meta.ID), modVS, items)
	if assert.Error(err) {
		assert.Equal("abort", err.Error())
	}
	assert.Equal(1, initCall)
	assert.Equal(0, otherCall)

	// abort operation on CB error in loop
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vOps).MinTimes(1)
	vOps.EXPECT().VolumeSeriesFetch(mF).Return(resF, nil).MinTimes(1)
	returnNilOnInit = true
	initCall = 0
	otherCall = 0
	abortErr = fmt.Errorf("abort")
	obj, err = c.VolumeSeriesUpdater(ctx, string(vsObj.Meta.ID), modVS, items)
	if assert.Error(err) {
		assert.Equal("abort", err.Error())
	}
	assert.Equal(1, initCall)
	assert.Equal(1, otherCall)
}

func TestCancelVolumeSeriesRequestObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	cbTime := time.Now().Add(time.Hour)
	res := &volume_series_request.VolumeSeriesRequestCancelOK{
		Payload: &models.VolumeSeriesRequest{
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "vr1",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"BIND", "MOUNT"},
				CompleteByTime:      strfmt.DateTime(cbTime),
				ClusterID:           "cluster1",
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs1",
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vrOps := mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(vrOps).MinTimes(1)
	mVR := mockmgmtclient.NewVolumeSeriesRequestMatcher(t, volume_series_request.NewVolumeSeriesRequestCancelParams().WithID(string(res.Payload.Meta.ID)))
	vrOps.EXPECT().VolumeSeriesRequestCancel(mVR).Return(res, nil).MinTimes(1)
	r, err := c.VolumeSeriesRequestCancel(ctx, string(res.Payload.Meta.ID))
	assert.Equal(ctx, mVR.CancelParam.Context)
	assert.Nil(err)
	assert.NotNil(r)
	assert.Equal(res.Payload, r)

	// api Error
	apiErr := volume_series_request.NewVolumeSeriesRequestCancelDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("vsr cancel error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vrOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(vrOps).MinTimes(1)
	vrOps.EXPECT().VolumeSeriesRequestCancel(mVR).Return(nil, apiErr).MinTimes(1)
	r, err = c.VolumeSeriesRequestCancel(nil, string(res.Payload.Meta.ID))
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(r)
}

func TestCreateVolumeSeriesRequestObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	now := time.Now()
	res := &volume_series_request.VolumeSeriesRequestCreateCreated{
		Payload: &models.VolumeSeriesRequest{
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID:      "VR-1",
					Version: 1,
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				ClusterID:           "CLUSTER-1",
				CompleteByTime:      strfmt.DateTime(now.Add(time.Hour)),
				RequestedOperations: []string{"CREATE", "BIND"},
				VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{
					VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
						AccountID: "A-1",
					},
					VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
						Name:          "VS-1",
						SizeBytes:     swag.Int64(1099511627776),
						ServicePlanID: "S-1",
					},
				},
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "V-1",
					SystemTags:     []string{"stag"},
				},
			},
		},
	}

	vrIn := &models.VolumeSeriesRequest{
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			ClusterID:           "CLUSTER-1",
			CompleteByTime:      strfmt.DateTime(now.Add(time.Hour)),
			RequestedOperations: []string{"CREATE", "BIND"},
			VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{
				VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
					AccountID: "A-1",
				},
				VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
					Name:          "VS-1",
					SizeBytes:     swag.Int64(1099511627776),
					ServicePlanID: "S-1",
				},
			},
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				VolumeSeriesID: "V-1",
				SystemTags:     []string{"stag"},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vrOps := mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(vrOps).MinTimes(1)
	mVR := mockmgmtclient.NewVolumeSeriesRequestMatcher(t, volume_series_request.NewVolumeSeriesRequestCreateParams())
	mVR.CreateParam.Payload = &models.VolumeSeriesRequestCreateArgs{
		VolumeSeriesRequestCreateOnce:    vrIn.VolumeSeriesRequestCreateOnce,
		VolumeSeriesRequestCreateMutable: vrIn.VolumeSeriesRequestCreateMutable,
	}
	mVR.D = time.Hour
	mVR.CreateParam.Context = nil // will be set from call
	vrOps.EXPECT().VolumeSeriesRequestCreate(mVR).Return(res, nil).MinTimes(1)
	vrOut, err := c.VolumeSeriesRequestCreate(ctx, vrIn)
	assert.Equal(ctx, mVR.CreateParam.Context)
	assert.Nil(err)
	assert.NotNil(vrOut)
	assert.Equal(res.Payload, vrOut)

	// error case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vrOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(vrOps).MinTimes(1)
	apiErr := volume_series_request.NewVolumeSeriesRequestCreateDefault(400)
	apiErr.Payload = &models.Error{Message: swag.String("create error"), Code: 400}
	vrOps.EXPECT().VolumeSeriesRequestCreate(mVR).Return(nil, apiErr).MinTimes(1)
	vrOut, err = c.VolumeSeriesRequestCreate(ctx, vrIn)
	assert.Equal(ctx, mVR.CreateParam.Context)
	assert.Error(err)
	assert.Regexp("create error", err.Error())
	assert.Nil(vrOut)
}

func TestFetchVolumeSeriesRequestObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	cbTime := time.Now().Add(time.Hour)
	res := &volume_series_request.VolumeSeriesRequestFetchOK{
		Payload: &models.VolumeSeriesRequest{
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "vr1",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"BIND", "MOUNT"},
				CompleteByTime:      strfmt.DateTime(cbTime),
				ClusterID:           "cluster1",
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs1",
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vrOps := mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(vrOps).MinTimes(1)
	mVR := mockmgmtclient.NewVolumeSeriesRequestMatcher(t, volume_series_request.NewVolumeSeriesRequestFetchParams().WithID(string(res.Payload.Meta.ID)))
	vrOps.EXPECT().VolumeSeriesRequestFetch(mVR).Return(res, nil).MinTimes(1)
	r, err := c.VolumeSeriesRequestFetch(ctx, string(res.Payload.Meta.ID))
	assert.Equal(ctx, mVR.FetchParam.Context)
	assert.Nil(err)
	assert.NotNil(r)
	assert.Equal(res.Payload, r)

	// api Error
	apiErr := volume_series_request.NewVolumeSeriesRequestFetchDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("storage fetch error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vrOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(vrOps).MinTimes(1)
	vrOps.EXPECT().VolumeSeriesRequestFetch(mVR).Return(nil, apiErr).MinTimes(1)
	r, err = c.VolumeSeriesRequestFetch(nil, string(res.Payload.Meta.ID))
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(r)
}

func TestListVolumeSeriesRequests(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	now := time.Now()
	res := &volume_series_request.VolumeSeriesRequestListOK{
		Payload: []*models.VolumeSeriesRequest{
			&models.VolumeSeriesRequest{
				VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
					Meta: &models.ObjMeta{
						ID:           "vr1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
					ClusterID:           "CLUSTER-1",
					CompleteByTime:      strfmt.DateTime(now.Add(time.Hour)),
					PlanOnly:            swag.Bool(false),
					RequestedOperations: []string{"CREATE", "BIND"},
				},
				VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
					VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
						VolumeSeriesRequestState: "NEW",
					},
				},
			},
			&models.VolumeSeriesRequest{
				VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
					Meta: &models.ObjMeta{
						ID:           "vr2",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
					ClusterID:           "CLUSTER-1",
					CompleteByTime:      strfmt.DateTime(now.Add(time.Hour)),
					PlanOnly:            swag.Bool(false),
					RequestedOperations: []string{"MOUNT"},
				},
				VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
					VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
						VolumeSeriesRequestState: "NEW",
					},
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vrOps := mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(vrOps).MinTimes(1)
	mVR := mockmgmtclient.NewVolumeSeriesRequestMatcher(t, volume_series_request.NewVolumeSeriesRequestListParams().WithIsTerminated(swag.Bool(false)))
	vrOps.EXPECT().VolumeSeriesRequestList(mVR).Return(res, nil).MinTimes(1)
	srs, err := c.VolumeSeriesRequestList(ctx, mVR.ListParam)
	assert.Equal(ctx, mVR.ListParam.Context)
	assert.Nil(err)
	assert.NotNil(srs)
	assert.Equal(res, srs)

	// api failure
	apiErr := volume_series_request.NewVolumeSeriesRequestListDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("storage request list error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vrOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(vrOps).MinTimes(1)
	mVR = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, volume_series_request.NewVolumeSeriesRequestListParams().WithIsTerminated(swag.Bool(false)))
	mVR.ListParam.VolumeSeriesID = swag.String("SOME-STORAGE-ID")
	vrOps.EXPECT().VolumeSeriesRequestList(mVR).Return(nil, apiErr).MinTimes(1)
	srs, err = c.VolumeSeriesRequestList(nil, mVR.ListParam)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(srs)
}

func TestUpdateVolumeSeriesRequestObj(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	res := &volume_series_request.VolumeSeriesRequestUpdateOK{
		Payload: &models.VolumeSeriesRequest{
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID:      "vr1",
					Version: 1,
				},
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					CapacityReservationPlan: &models.CapacityReservationPlan{
						StorageTypeReservations: map[string]models.StorageTypeReservation{
							"Amazon gp2": {NumMirrors: 1, SizeBytes: swag.Int64(107374182400)},
						},
					},
					CapacityReservationResult: &models.CapacityReservationResult{
						DesiredReservations: map[string]models.PoolReservation{
							"SP-1": {SizeBytes: swag.Int64(107374182400)},
						},
					},
					VolumeSeriesRequestState: "NEW",
				},
			},
		},
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vrOps := mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(vrOps).MinTimes(1)
	mVR := mockmgmtclient.NewVolumeSeriesRequestMatcher(t, volume_series_request.NewVolumeSeriesRequestUpdateParams().WithID(string(res.Payload.Meta.ID)))
	mVR.UpdateParam.Payload = &res.Payload.VolumeSeriesRequestMutable
	mVR.UpdateParam.Set = []string{"volumeSeriesRequestState", "requestMessages", "capacityReservationPlan", "capacityReservationResult", "volumeSeriesId"}
	mVR.UpdateParam.Append = []string{"appendArg"}
	mVR.UpdateParam.Remove = []string{"removeArg"}
	mVR.UpdateParam.Version = int32(res.Payload.Meta.Version)
	mVR.UpdateParam.Context = nil // will be set from call
	vrOps.EXPECT().VolumeSeriesRequestUpdate(mVR).Return(res, nil).MinTimes(1)
	items := &Updates{}
	items.Set = mVR.UpdateParam.Set
	items.Append = mVR.UpdateParam.Append
	items.Remove = mVR.UpdateParam.Remove
	vr, err := c.VolumeSeriesRequestUpdate(ctx, res.Payload, items)
	assert.Equal(ctx, mVR.UpdateParam.Context)
	assert.Nil(err)
	assert.NotNil(vr)
	assert.Equal(res.Payload, vr)

	// API error + additional properties set
	res.Payload.RequestMessages = make([]*models.TimestampedString, 0)
	res.Payload.VolumeSeriesID = models.ObjIDMutable("vr1")
	apiErr := volume_series_request.NewVolumeSeriesRequestUpdateDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("volume series request update error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vrOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(vrOps).MinTimes(1)
	mVR.UpdateParam.Context = nil // will be set from call
	vrOps.EXPECT().VolumeSeriesRequestUpdate(mVR).Return(nil, apiErr).MinTimes(1)
	vr, err = c.VolumeSeriesRequestUpdate(nil, res.Payload, items)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
	assert.Nil(vr)
}

func TestVolumeSeriesRequestUpdater(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	// A fake updater is used to test - this test focuses on updaterArg function calls.
	// Updater is fully tested in TestVolumeSeriesObjUpdater.
	fu := &fakeUpdater{}
	c.updater = fu
	ctx := context.Background()

	vsr := &models.VolumeSeriesRequest{}
	vsr.Meta = &models.ObjMeta{ID: "VSR-1", Version: 2}

	var mfO *models.VolumeSeriesRequest
	modifyFn := func(o *models.VolumeSeriesRequest) (*models.VolumeSeriesRequest, error) {
		mfO = o
		return nil, nil
	}

	fu.ouRetO = vsr
	items := &Updates{Set: []string{"reservationState"}, Version: 2}
	c.VolumeSeriesRequestUpdater(ctx, "VSR-1", modifyFn, items)
	assert.Equal(ctx, fu.ouInCtx)
	assert.Equal("VSR-1", fu.ouInOid)
	assert.Equal(items, fu.ouInItems)
	assert.NotNil(fu.ouInUA)
	ua := fu.ouInUA
	assert.Equal("VolumeSeriesRequest", ua.typeName)
	assert.NotNil(ua.modifyFn)
	assert.NotNil(ua.fetchFn)
	assert.NotNil(ua.updateFn)
	assert.NotNil(ua.metaFn)

	// test modify gets called
	ua.modifyFn(vsr)
	assert.Equal(vsr, mfO)
	ua.modifyFn(nil)
	assert.Nil(mfO)

	// test meta functionality
	assert.Equal(vsr.Meta, ua.metaFn(vsr))

	// test fetch invocation
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vsrOps := mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(vsrOps).MinTimes(1)
	mFetchSPA := mockmgmtclient.NewVolumeSeriesRequestMatcher(t, volume_series_request.NewVolumeSeriesRequestFetchParams().WithID("VSR-1"))
	vsrOps.EXPECT().VolumeSeriesRequestFetch(mFetchSPA).Return(nil, fmt.Errorf("fetch-error"))
	retF, err := ua.fetchFn(ctx, "VSR-1")
	assert.Equal(ctx, mFetchSPA.FetchParam.Context)
	assert.Error(err)
	assert.Regexp("fetch-error", err)
	assert.Nil(retF)

	// test update invocation
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	vsrOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(vsrOps)
	mVR := mockmgmtclient.NewVolumeSeriesRequestMatcher(t, volume_series_request.NewVolumeSeriesRequestUpdateParams().WithID(string(vsr.Meta.ID)))
	mVR.UpdateParam.Payload = &vsr.VolumeSeriesRequestMutable
	mVR.UpdateParam.Set = []string{"reservationState"}
	mVR.UpdateParam.Version = int32(vsr.Meta.Version)
	mVR.UpdateParam.Context = nil // will be set from call
	vsrOps.EXPECT().VolumeSeriesRequestUpdate(mVR).Return(nil, fmt.Errorf("update-error"))
	retU, err := ua.updateFn(ctx, vsr, items)
	assert.Equal(ctx, mVR.UpdateParam.Context)
	assert.Error(err)
	assert.Regexp("update-error", err)
	assert.Nil(retU)

	// error path
	fu.ouRetErr = fmt.Errorf("updater-error")
	fu.ouRetO = nil
	_, err = c.VolumeSeriesRequestUpdater(ctx, "VSR-1", modifyFn, items)
	assert.Error(err)
	assert.Regexp("updater-error", err)
}

func TestMetricVolumeSeriesIO(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	data := []*models.IoMetricDatum{}
	mData := &models.IoMetricData{
		Data: data,
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	mOps := mockmgmtclient.NewMockMetricsClient(mockCtrl)
	mAPI.EXPECT().Metrics().Return(mOps).MinTimes(1)
	mMM := mockmgmtclient.NewMetricMatcher(t, metrics.NewVolumeSeriesIOMetricUploadParams().WithPayload(mData))
	mOps.EXPECT().VolumeSeriesIOMetricUpload(mMM).Return(nil, nil).MinTimes(1)
	err := c.VolumeSeriesIOMetricUpload(ctx, data)
	assert.Equal(ctx, mMM.VolumeSeriesIOUpload.Context)
	assert.Nil(err)

	// api Error
	apiErr := metrics.NewVolumeSeriesIOMetricUploadDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("VolumeSeriesIOMetricUpload error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	mOps = mockmgmtclient.NewMockMetricsClient(mockCtrl)
	mAPI.EXPECT().Metrics().Return(mOps).MinTimes(1)
	mOps.EXPECT().VolumeSeriesIOMetricUpload(mMM).Return(nil, apiErr).MinTimes(1)
	err = c.VolumeSeriesIOMetricUpload(ctx, data)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
}

func TestMetricStorageSeriesIO(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &Client{
		Log: tl.Logger(),
	}
	ctx := context.Background()

	data := []*models.IoMetricDatum{}
	mData := &models.IoMetricData{
		Data: data,
	}

	// success case
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	mOps := mockmgmtclient.NewMockMetricsClient(mockCtrl)
	mAPI.EXPECT().Metrics().Return(mOps).MinTimes(1)
	mMM := mockmgmtclient.NewMetricMatcher(t, metrics.NewStorageIOMetricUploadParams().WithPayload(mData))
	mOps.EXPECT().StorageIOMetricUpload(mMM).Return(nil, nil).MinTimes(1)
	err := c.StorageIOMetricUpload(ctx, data)
	assert.Equal(ctx, mMM.StorageIOUpload.Context)
	assert.Nil(err)

	// api Error
	apiErr := metrics.NewStorageIOMetricUploadDefault(400)
	apiErr.Payload = &models.Error{
		Code:    400,
		Message: swag.String("StorageIOMetricUpload error"),
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.ClientAPI = mAPI
	mOps = mockmgmtclient.NewMockMetricsClient(mockCtrl)
	mAPI.EXPECT().Metrics().Return(mOps).MinTimes(1)
	mOps.EXPECT().StorageIOMetricUpload(mMM).Return(nil, apiErr).MinTimes(1)
	err = c.StorageIOMetricUpload(ctx, data)
	assert.NotNil(err)
	assert.Regexp(*apiErr.Payload.Message, err.Error())
}

type fakeUpdater struct {
	ouInCtx   context.Context
	ouInOid   string
	ouInItems *Updates
	ouInUA    *updaterArgs
	ouRetO    interface{}
	ouRetErr  error
}

func (u *fakeUpdater) objUpdate(ctx context.Context, oID string, items *Updates, ua *updaterArgs) (interface{}, error) {
	u.ouInCtx = ctx
	u.ouInOid = oID
	u.ouInItems = items
	u.ouInUA = ua
	return u.ouRetO, u.ouRetErr
}
