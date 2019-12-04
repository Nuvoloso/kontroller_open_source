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


package handlers

import (
	"net/http"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/application_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/consistency_group"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	fal "github.com/Nuvoloso/kontroller/pkg/centrald/audit/fake"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestApplicationGroupFetchFilter(t *testing.T) {
	assert := assert.New(t)
	hc := newHandlerComp()
	obj := &models.ApplicationGroup{
		ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{Meta: &models.ObjMeta{ID: "id1"}},
		ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
			AccountID:       "aid1",
			TenantAccountID: "tid1",
		},
	}
	rObj := &models.Role{}
	rObj.Capabilities = map[string]bool{}
	ai := &auth.Info{}

	t.Log("case: internal user")
	assert.NoError(hc.applicationGroupFetchFilter(ai, obj))

	t.Log("case: SystemManagementCap capability fails")
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.SystemManagementCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.applicationGroupFetchFilter(ai, obj))

	t.Log("case: CSPDomainManagementCap capability fails")
	ai.RoleObj = &models.Role{}
	ai.AccountID = "aid1"
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.applicationGroupFetchFilter(ai, obj))

	t.Log("case: CSPDomainUsageCap capability fails")
	ai.RoleObj = &models.Role{}
	ai.AccountID = "aid1"
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.applicationGroupFetchFilter(ai, obj))

	t.Log("case: VolumeSeriesFetchCap capability correct tenant")
	ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesFetchCap: true}
	ai.AccountID = "tid1"
	assert.NoError(hc.applicationGroupFetchFilter(ai, obj))

	t.Log("case: VolumeSeriesFetchCap capability wrong tenant")
	ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesFetchCap: true}
	ai.AccountID = "tid2"
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.applicationGroupFetchFilter(ai, obj))

	t.Log("case: owner account")
	ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: true}
	ai.AccountID = "aid1"
	assert.NoError(hc.applicationGroupFetchFilter(ai, obj))

	t.Log("case: not owner account")
	ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: true}
	ai.AccountID = "aid9"
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.applicationGroupFetchFilter(ai, obj))
}

func TestApplicationGroupCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	fa := &fal.AuditLog{}
	app.AuditLog = fa
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.ApplicationGroupCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
		Payload:     &models.ApplicationGroup{},
	}
	params.Payload.AccountID = "account2"
	params.Payload.Name = "ag1"
	params.Payload.Description = "a application group"
	params.Payload.SystemTags = []string{"prod"}
	ctx := params.HTTPRequest.Context()
	obj := &models.ApplicationGroup{
		ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
			Meta: &models.ObjMeta{ID: models.ObjID("objectId"), Version: 1},
		},
		ApplicationGroupMutable: models.ApplicationGroupMutable{
			Name: params.Payload.Name,
		},
	}
	aObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta:            &models.ObjMeta{ID: "account2"},
			TenantAccountID: "tid1",
		},
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	t.Log("case: success (all defaults)")
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(params.Payload.AccountID)).Return(aObj, nil)
	oAG := mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Create(ctx, params.Payload).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	mds.EXPECT().OpsApplicationGroup().Return(oAG)
	hc.DS = mds
	var ret middleware.Responder
	cntRLock := hc.cntRLock
	cntRUnlock := hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.applicationGroupCreate(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.ApplicationGroupCreateCreated)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Len(evM.InSSProps, 1)
	assert.EqualValues("objectId", evM.InSSProps["meta.id"])
	assert.EqualValues("tid1", params.Payload.TenantAccountID)
	assert.Equal(obj, evM.InACScope)
	exp := &fal.Args{AI: ai, Action: centrald.ApplicationGroupCreateAction, ObjID: "objectId", Name: params.Payload.Name, Message: "Created"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	tl.Flush()

	t.Log("case: auditLog not ready")
	*fa = fal.AuditLog{}
	fa.ReadyRet = centrald.ErrorDbError
	assert.NotPanics(func() { ret = hc.applicationGroupCreate(params) })
	mE, ok := ret.(*ops.ApplicationGroupCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.applicationGroupCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ApplicationGroupCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	t.Log("case: not authorized")
	fa.Posts = []*fal.Args{}
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	assert.NotPanics(func() { ret = hc.applicationGroupCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ApplicationGroupCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	exp = &fal.Args{AI: ai, Action: centrald.ApplicationGroupCreateAction, Name: params.Payload.Name, Err: true, Message: "Create unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	ai.AccountID, ai.RoleObj = "", nil
	tl.Flush()

	// all the remaining error cases
	var payload models.ApplicationGroup
	for tc := 0; tc <= 3; tc++ {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		payload = *params.Payload
		mds = mock.NewMockDataStore(mockCtrl)
		oA = mock.NewMockAccountOps(mockCtrl)
		oAG = mock.NewMockApplicationGroupOps(mockCtrl)
		cntRLock = hc.cntRLock
		cntRUnlock = hc.cntRUnlock
		expectedErr := centrald.ErrorMissing
		switch tc {
		case 0:
			t.Log("case: Name missing")
			payload.Name = ""
		case 1:
			t.Log("case: AccountID fetch fails")
			expectedErr = centrald.ErrorDbError
			oA.EXPECT().Fetch(ctx, string(params.Payload.AccountID)).Return(nil, expectedErr)
			mds.EXPECT().OpsAccount().Return(oA)
		case 2:
			t.Log("case: AccountID not found")
			oA.EXPECT().Fetch(ctx, string(params.Payload.AccountID)).Return(nil, centrald.ErrorNotFound)
			mds.EXPECT().OpsAccount().Return(oA)
		case 3:
			t.Log("case: Create fails because object exists")
			expectedErr = centrald.ErrorExists
			// cover path that sets accounts from the auth.Info, ignoring the payload account
			ai.AccountID, ai.TenantAccountID = "aid1", "tid2"
			p2 := payload
			p2.AccountID, p2.TenantAccountID = "aid1", "tid2"
			oAG.EXPECT().Create(ctx, &p2).Return(nil, expectedErr)
			mds.EXPECT().OpsApplicationGroup().Return(oAG)
		default:
			assert.True(false)
		}
		hc.DS = mds
		assert.NotPanics(func() {
			ret = hc.applicationGroupCreate(ops.ApplicationGroupCreateParams{HTTPRequest: requestWithAuthContext(ai), Payload: &payload})
		})
		assert.NotNil(ret)
		mE, ok = ret.(*ops.ApplicationGroupCreateDefault)
		assert.True(ok)
		assert.Equal(expectedErr.C, int(mE.Payload.Code))
		assert.Regexp("^"+expectedErr.M, *mE.Payload.Message)
		if tc > 0 {
			assert.Equal(cntRLock+1, hc.cntRLock)
			assert.Equal(cntRUnlock+1, hc.cntRUnlock)
		}
		tl.Flush()
	}

	// no payload
	params.Payload = nil
	t.Log("case: nil payload")
	assert.NotPanics(func() { ret = hc.applicationGroupCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ApplicationGroupCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
}

func TestApplicationGroupDelete(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	fa := &fal.AuditLog{}
	app.AuditLog = fa
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.ApplicationGroupDeleteParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.ApplicationGroup{
		ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
			Meta: &models.ObjMeta{ID: models.ObjID("objectID")},
		},
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oAG := mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oAG.EXPECT().Delete(ctx, params.ID).Return(nil)
	oCG := mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Count(ctx, consistency_group.ConsistencyGroupListParams{ApplicationGroupID: &params.ID}, uint(1)).Return(0, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsApplicationGroup().Return(oAG).MinTimes(1)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	cntLock := hc.cntLock
	cntUnlock := hc.cntUnlock
	assert.NotPanics(func() { ret = hc.applicationGroupDelete(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.ApplicationGroupDeleteNoContent)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Empty(evM.InSSProps)
	assert.Equal(obj, evM.InACScope)
	exp := &fal.Args{AI: ai, Action: centrald.ApplicationGroupDeleteAction, ObjID: models.ObjID(params.ID), Name: obj.Name, Message: "Deleted"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	tl.Flush()

	// delete failure case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: delete failure")
	oAG = mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oAG.EXPECT().Delete(ctx, params.ID).Return(centrald.ErrorDbError)
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Count(ctx, consistency_group.ConsistencyGroupListParams{ApplicationGroupID: &params.ID}, uint(1)).Return(0, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsApplicationGroup().Return(oAG).MinTimes(1)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.applicationGroupDelete(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.ApplicationGroupDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: auditLog not ready")
	oAG = mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsApplicationGroup().Return(oAG).MinTimes(1)
	hc.DS = mds
	*fa = fal.AuditLog{}
	fa.ReadyRet = centrald.ErrorDbError
	assert.NotPanics(func() { ret = hc.applicationGroupDelete(params) })
	mE, ok = ret.(*ops.ApplicationGroupDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.applicationGroupDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ApplicationGroupDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized")
	fa.Posts = []*fal.Args{}
	oAG = mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsApplicationGroup().Return(oAG)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	assert.NotPanics(func() { ret = hc.applicationGroupDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ApplicationGroupDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	exp = &fal.Args{AI: ai, Action: centrald.ApplicationGroupDeleteAction, ObjID: models.ObjID(params.ID), Name: obj.Name, Err: true, Message: "Delete unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	ai.RoleObj = nil
	tl.Flush()

	// ConsistencyGroup exist
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: ConsistencyGroup exist")
	oAG = mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Count(ctx, consistency_group.ConsistencyGroupListParams{ApplicationGroupID: &params.ID}, uint(1)).Return(1, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsApplicationGroup().Return(oAG)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.applicationGroupDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ApplicationGroupDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Regexp("contains one or more consistency groups", *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	// ConsistencyGroup count fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: ConsistencyGroup count fails")
	oAG = mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Count(ctx, consistency_group.ConsistencyGroupListParams{ApplicationGroupID: &params.ID}, uint(1)).Return(0, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsApplicationGroup().Return(oAG)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.applicationGroupDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ApplicationGroupDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	// fetch failure case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oAG = mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsApplicationGroup().Return(oAG)
	t.Log("case: fetch failure")
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.applicationGroupDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ApplicationGroupDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
	assert.Equal(cntLock, hc.cntLock)
	assert.Equal(cntUnlock, hc.cntUnlock)
}

func TestApplicationGroupFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.ApplicationGroupFetchParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.ApplicationGroup{
		ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("objectID"),
			},
		},
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oAG := mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsApplicationGroup().Return(oAG)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.applicationGroupFetch(params) })
	assert.NotNil(ret)
	mR, ok := ret.(*ops.ApplicationGroupFetchOK)
	assert.True(ok)
	assert.Equal(obj, mR.Payload)
	tl.Flush()

	// fetch failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oAG = mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsApplicationGroup().Return(oAG)
	t.Log("case: fetch failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.applicationGroupFetch(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.ApplicationGroupFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.applicationGroupFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ApplicationGroupFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized")
	oAG = mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsApplicationGroup().Return(oAG)
	hc.DS = mds
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	assert.NotPanics(func() { ret = hc.applicationGroupFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ApplicationGroupFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	ai.RoleObj = nil
}

func TestApplicationGroupList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	fops := &fakeOps{}
	hc.ops = fops
	ai := &auth.Info{}
	params := ops.ApplicationGroupListParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	ctx := params.HTTPRequest.Context()
	objects := []*models.ApplicationGroup{
		&models.ApplicationGroup{
			ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
				Meta: &models.ObjMeta{
					ID: models.ObjID("objectID1"),
				},
			},
			ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
				AccountID:       "aid1",
				TenantAccountID: "tid1",
			},
		},
		&models.ApplicationGroup{
			ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
				Meta: &models.ObjMeta{
					ID: models.ObjID("objectID2"),
				},
			},
			ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
				AccountID:       "aid2",
				TenantAccountID: "tid1",
			},
		},
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oAG := mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().List(ctx, params).Return(objects, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsApplicationGroup().Return(oAG)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.applicationGroupList(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.ApplicationGroupListOK)
	assert.True(ok)
	assert.EqualValues(objects, mO.Payload)
	tl.Flush()

	// list fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oAG = mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().List(ctx, params).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsApplicationGroup().Return(oAG)
	t.Log("case: List failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.applicationGroupList(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.ApplicationGroupListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.applicationGroupList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ApplicationGroupListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	t.Log("case: constrainEitherOrQueryAccounts changes accounts")
	ai.RoleObj = &models.Role{}
	params.AccountID, params.TenantAccountID = swag.String("a1"), swag.String("t1")
	fops.RetConstrainEOQueryAccountsAID, fops.RetConstrainEOQueryAccountsTID = swag.String("aid1"), swag.String("tid1")
	cParams := params
	cParams.AccountID, cParams.TenantAccountID = fops.RetConstrainEOQueryAccountsAID, fops.RetConstrainEOQueryAccountsTID
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oAG = mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().List(ctx, cParams).Return(objects, nil)
	mds.EXPECT().OpsApplicationGroup().Return(oAG)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.applicationGroupList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.ApplicationGroupListOK)
	assert.True(ok)
	assert.Equal(3, fops.CntConstrainEOQueryAccounts)
	assert.Equal(params.AccountID, fops.InConstrainEOQueryAccountsAID)
	assert.Equal(params.TenantAccountID, fops.InConstrainEOQueryAccountsTID)
	tl.Flush()

	t.Log("case: constrainEitherOrQueryAccounts nil accounts")
	ai.RoleObj = &models.Role{}
	params.AccountID, params.TenantAccountID = swag.String("a1"), swag.String("t1")
	fops.RetConstrainEOQueryAccountsAID, fops.RetConstrainEOQueryAccountsTID = nil, nil
	assert.NotPanics(func() { ret = hc.applicationGroupList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.ApplicationGroupListOK)
	assert.True(ok)
	assert.Equal(4, fops.CntConstrainEOQueryAccounts)
	assert.Equal(params.AccountID, fops.InConstrainEOQueryAccountsAID)
	assert.Equal(params.TenantAccountID, fops.InConstrainEOQueryAccountsTID)
	tl.Flush()

	t.Log("case: constrainEitherOrQueryAccounts error")
	ai.RoleObj = &models.Role{}
	params.AccountID, params.TenantAccountID = swag.String("a1"), swag.String("t1")
	fops.RetConstrainEOQueryAccountsErr = centrald.ErrorDbError
	assert.NotPanics(func() { ret = hc.applicationGroupList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ApplicationGroupListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	assert.Equal(5, fops.CntConstrainEOQueryAccounts)
	fops.RetConstrainEOQueryAccountsAID, fops.RetConstrainEOQueryAccountsTID = swag.String("a1"), swag.String("t1")
	fops.RetConstrainEOQueryAccountsErr = nil
	ai.RoleObj = nil
}

func TestApplicationGroupUpdate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	fa := &fal.AuditLog{}
	app.AuditLog = fa
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}

	// make the name map
	var nMap JSONToAttrNameMap
	assert.NotPanics(func() { nMap = hc.applicationGroupMutableNameMap() })
	// validate some embedded properties
	assert.Equal("name", nMap.jName("Name"))
	assert.Equal("systemTags", nMap.jName("SystemTags"))

	// parse params
	objM := &models.ApplicationGroupMutable{}
	objM.Name = "ag1"
	params := ops.ApplicationGroupUpdateParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
		Version:     swag.Int32(8),
		Remove:      []string{},
		Append:      []string{},
		Set:         []string{nMap.jName("Name")},
		Payload:     objM,
	}
	ctx := params.HTTPRequest.Context()
	var ua *centrald.UpdateArgs
	var err error
	var uP = [centrald.NumActionTypes][]string{
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    params.Set,
	}
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM := updateArgsMatcher(t, ua)

	// success
	obj := &models.ApplicationGroup{
		ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
			Meta: &models.ObjMeta{ID: models.ObjID("objectID"), Version: models.ObjVersion(*params.Version)},
		},
	}
	obj.AccountID = "account1"
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oAG := mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oAG.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsApplicationGroup().Return(oAG).MinTimes(1)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.applicationGroupUpdate(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.ApplicationGroupUpdateOK)
	assert.True(ok)
	assert.Equal(obj, mO.Payload)
	assert.Empty(evM.InSSProps)
	assert.Equal(obj, evM.InACScope)
	tl.Flush()

	mockCtrl.Finish()
	t.Log("case: success, no version, validation adds version, valid auth")
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	obj.Meta.Version = 3
	aiVO := &auth.Info{
		AccountID:       "account1",
		TenantAccountID: "tid1",
		RoleObj: &models.Role{
			RoleMutable: models.RoleMutable{
				Capabilities: map[string]bool{centrald.VolumeSeriesOwnerCap: true},
			},
		},
	}
	params.HTTPRequest = requestWithAuthContext(aiVO)
	ctx = params.HTTPRequest.Context()
	params.Version = nil
	params.Append = []string{"tags"}
	params.Set = []string{"description", "name"}
	params.Payload.Tags = []string{"new:tag"}
	uP[centrald.UpdateAppend] = params.Append
	uP[centrald.UpdateSet] = params.Set
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).AddsVersion(3)
	oAG = mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oAG.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	mds.EXPECT().OpsApplicationGroup().Return(oAG).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.applicationGroupUpdate(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.ApplicationGroupUpdateOK)
	assert.True(ok)
	assert.Equal(obj, mO.Payload)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	// Update failed after successful verification paths
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: update failed")
	params.Append = []string{"systemTags"}
	params.Set = []string{"description"}
	params.Payload.SystemTags = []string{"system1:tag", "system2:tag"}
	params.Version = swag.Int32(8)
	obj.Meta.Version = 8
	uP[centrald.UpdateAppend] = params.Append
	uP[centrald.UpdateSet] = params.Set
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua)
	oAG = mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oAG.EXPECT().Update(ctx, uaM, params.Payload).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsApplicationGroup().Return(oAG).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.applicationGroupUpdate(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.ApplicationGroupUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: fetch failure")
	oAG = mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsApplicationGroup().Return(oAG)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.applicationGroupUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ApplicationGroupUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mD.Payload.Message)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: auditLog not ready")
	oAG = mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsApplicationGroup().Return(oAG)
	fa.ReadyRet = centrald.ErrorDbError
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.applicationGroupUpdate(params) })
	mD, ok = ret.(*ops.ApplicationGroupUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.applicationGroupUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ApplicationGroupUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: unauthorized for systemTags")
	params.HTTPRequest = requestWithAuthContext(aiVO)
	ctx = params.HTTPRequest.Context()
	params.Append = []string{"systemTags"}
	params.Set = []string{"description"}
	oAG = mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsApplicationGroup().Return(oAG)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.applicationGroupUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ApplicationGroupUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)
	ai.RoleObj = nil
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: unauthorized")
	ai.RoleObj = &models.Role{}
	ai.AccountID = string(obj.AccountID)
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	params.Append = []string{}
	params.Set = []string{"description"}
	oAG = mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsApplicationGroup().Return(oAG)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.applicationGroupUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ApplicationGroupUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)
	ai.RoleObj = nil
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	// missing/invalid updates
	var verP *int32
	for tc := 1; tc <= 4; tc++ {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		mds = mock.NewMockDataStore(mockCtrl)
		oAG = mock.NewMockApplicationGroupOps(mockCtrl)
		obj.AccountID = "account1"
		params.Payload.Name = "new"
		payload := params.Payload
		set := []string{"name"}
		append := []string{}
		verP = nil
		expectedErr := centrald.ErrorUpdateInvalidRequest
		switch tc {
		case 1:
			t.Log("case: no change")
			set = []string{}
		case 2:
			t.Log("case: empty name")
			payload.Name = ""
		case 3:
			t.Log("case: no payload")
			payload = nil
		case 4:
			t.Log("case: invalid version")
			verP = swag.Int32(1)
			oAG = mock.NewMockApplicationGroupOps(mockCtrl)
			oAG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			mds.EXPECT().OpsApplicationGroup().Return(oAG)
			expectedErr = centrald.ErrorIDVerNotFound
		default:
			assert.False(true)
		}
		hc.DS = mds
		assert.NotPanics(func() {
			ret = hc.applicationGroupUpdate(ops.ApplicationGroupUpdateParams{HTTPRequest: requestWithAuthContext(ai), ID: params.ID, Payload: payload, Set: set, Append: append, Version: verP})
		})
		assert.NotNil(ret)
		mD, ok = ret.(*ops.ApplicationGroupUpdateDefault)
		assert.True(ok)
		assert.Equal(expectedErr.C, int(mD.Payload.Code))
		assert.Regexp("^"+expectedErr.M, *mD.Payload.Message)
		tl.Flush()
	}
}
