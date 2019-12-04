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
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/consistency_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/snapshot"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
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

func TestConsistencyGroupFetchFilter(t *testing.T) {
	assert := assert.New(t)
	hc := newHandlerComp()
	obj := &models.ConsistencyGroup{
		ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{Meta: &models.ObjMeta{ID: "id1"}},
		ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
			AccountID:       "aid1",
			TenantAccountID: "tid1",
		},
	}
	rObj := &models.Role{}
	rObj.Capabilities = map[string]bool{}
	ai := &auth.Info{}

	t.Log("case: internal user")
	assert.NoError(hc.consistencyGroupFetchFilter(ai, obj))

	t.Log("case: SystemManagementCap capability fails")
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.SystemManagementCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.consistencyGroupFetchFilter(ai, obj))

	t.Log("case: CSPDomainManagementCap capability fails")
	ai.RoleObj = &models.Role{}
	ai.AccountID = "aid1"
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.consistencyGroupFetchFilter(ai, obj))

	t.Log("case: CSPDomainUsageCap capability fails")
	ai.RoleObj = &models.Role{}
	ai.AccountID = "aid1"
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.consistencyGroupFetchFilter(ai, obj))

	t.Log("case: VolumeSeriesFetchCap capability correct tenant")
	ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesFetchCap: true}
	ai.AccountID = "tid1"
	assert.NoError(hc.consistencyGroupFetchFilter(ai, obj))

	t.Log("case: VolumeSeriesFetchCap capability wrong tenant")
	ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesFetchCap: true}
	ai.AccountID = "tid2"
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.consistencyGroupFetchFilter(ai, obj))

	t.Log("case: owner account")
	ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: true}
	ai.AccountID = "aid1"
	assert.NoError(hc.consistencyGroupFetchFilter(ai, obj))

	t.Log("case: not owner account")
	ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: true}
	ai.AccountID = "aid9"
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.consistencyGroupFetchFilter(ai, obj))
}

func TestConsistencyGroupApplyInheritedProperties(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	ctx := context.Background()

	ai := &auth.Info{}
	obj := &models.ConsistencyGroup{
		ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
			Meta: &models.ObjMeta{ID: "id1"},
		},
		ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
			AccountID:       "aid1",
			TenantAccountID: "tid1",
		},
		ConsistencyGroupMutable: models.ConsistencyGroupMutable{},
	}
	aObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("aid1"),
			},
		},
		AccountMutable: models.AccountMutable{
			Name: "account",
			SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
				RetentionDurationSeconds: swag.Int32(99),
				Inherited:                false,
			},
			SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
				CspDomainID:        "domID",
				ProtectionDomainID: "pdID",
				Inherited:          false,
			},
			VsrManagementPolicy: &models.VsrManagementPolicy{
				RetentionDurationSeconds: swag.Int32(100),
				NoDelete:                 false,
				Inherited:                false,
			},
		},
	}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()

	t.Log("case: policies not inherited")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	obj.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{
		RetentionDurationSeconds: swag.Int32(99),
		Inherited:                false,
	}
	err := hc.consistencyGroupApplyInheritedProperties(ctx, ai, obj, nil)
	assert.Nil(err)
	tl.Flush()

	t.Log("case: nil policy")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	obj.SnapshotManagementPolicy = nil
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(aObj.Meta.ID)).Return(aObj, nil).MinTimes(1)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	err = hc.consistencyGroupApplyInheritedProperties(ctx, ai, obj, nil)
	assert.Nil(err)
	assert.NotEqual(obj.SnapshotManagementPolicy, aObj.SnapshotManagementPolicy)
	tl.Flush()

	t.Log("case: policy inherited")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	obj.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{
		RetentionDurationSeconds: swag.Int32(99),
		Inherited:                true,
	}
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(aObj.Meta.ID)).Return(aObj, nil).MinTimes(1)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	err = hc.consistencyGroupApplyInheritedProperties(ctx, ai, obj, nil)
	assert.Nil(err)
	assert.NotEqual(obj.SnapshotManagementPolicy, aObj.SnapshotManagementPolicy)
	tl.Flush()

	t.Log("case: error fetching account object")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	obj.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{
		RetentionDurationSeconds: swag.Int32(99),
		Inherited:                true,
	}
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(aObj.Meta.ID)).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	err = hc.consistencyGroupApplyInheritedProperties(ctx, ai, obj, nil)
	assert.NotNil(err)
	tl.Flush()
}

func TestConsistencyGroupCreate(t *testing.T) {
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
	params := ops.ConsistencyGroupCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
		Payload:     &models.ConsistencyGroup{},
	}
	params.Payload.AccountID = "account2"
	params.Payload.ApplicationGroupIds = []models.ObjIDMutable{"ag1"}
	params.Payload.Name = "cg1"
	params.Payload.Description = "a consistency group"
	params.Payload.SystemTags = []string{"prod"}
	params.Payload.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{
		RetentionDurationSeconds: swag.Int32(99),
		Inherited:                false,
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.ConsistencyGroup{
		ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
			Meta: &models.ObjMeta{ID: "objectId", Version: 1},
		},
		ConsistencyGroupMutable: models.ConsistencyGroupMutable{
			ApplicationGroupIds: params.Payload.ApplicationGroupIds,
			Name:                params.Payload.Name,
			SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
				RetentionDurationSeconds: swag.Int32(99),
				Inherited:                false,
			},
		},
	}
	aObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta:            &models.ObjMeta{ID: "account2"},
			TenantAccountID: "tid1",
		},
	}
	agObj := &models.ApplicationGroup{
		ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
			Meta: &models.ObjMeta{ID: "ag1"},
		},
		ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
			AccountID: "account2",
		},
		ApplicationGroupMutable: models.ApplicationGroupMutable{
			Name: "app",
		},
	}
	sysObj := &models.System{
		SystemAllOf0: models.SystemAllOf0{
			Meta: &models.ObjMeta{
				ID:      "sys_id1",
				Version: 1,
			},
		},
		SystemMutable: models.SystemMutable{
			SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
				RetentionDurationSeconds: swag.Int32(99),
			},
			SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
				CspDomainID:        "domID",
				ProtectionDomainID: "pdID",
			},
			VsrManagementPolicy: &models.VsrManagementPolicy{
				RetentionDurationSeconds: swag.Int32(100),
				NoDelete:                 false,
			},
		},
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	t.Log("case: success (all defaults)")
	mds := mock.NewMockDataStore(mockCtrl)
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(params.Payload.AccountID)).Return(aObj, nil)
	oA.EXPECT().Fetch(ctx, string(aObj.TenantAccountID)).Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsAccount().Return(oA).MaxTimes(2)
	oSys := mock.NewMockSystemOps(mockCtrl)
	oSys.EXPECT().Fetch().Return(sysObj, nil)
	mds.EXPECT().OpsSystem().Return(oSys)
	oAG := mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Fetch(ctx, string(params.Payload.ApplicationGroupIds[0])).Return(agObj, nil)
	mds.EXPECT().OpsApplicationGroup().Return(oAG)
	oCG := mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Create(ctx, params.Payload).Return(obj, nil)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	hc.DS = mds
	var ret middleware.Responder
	cntRLock := hc.cntRLock
	cntRUnlock := hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.consistencyGroupCreate(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.ConsistencyGroupCreateCreated)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Len(evM.InSSProps, 1)
	assert.EqualValues("objectId", evM.InSSProps["meta.id"])
	assert.EqualValues("tid1", params.Payload.TenantAccountID)
	assert.Equal(obj, evM.InACScope)
	exp := &fal.Args{AI: ai, Action: centrald.ConsistencyGroupCreateAction, ObjID: "objectId", Name: params.Payload.Name, RefID: "ag1", Message: "Created in application group [app]"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	assert.Empty(fa.Events)
	tl.Flush()

	t.Log("case: success with multiple AGs")
	params.Payload.ApplicationGroupIds = append(params.Payload.ApplicationGroupIds, "ag2")
	obj.ApplicationGroupIds = params.Payload.ApplicationGroupIds
	var agObj2 *models.ApplicationGroup
	testutils.Clone(agObj, &agObj2)
	agObj2.Name = "rap"
	oA.EXPECT().Fetch(ctx, string(params.Payload.AccountID)).Return(aObj, nil)
	oA.EXPECT().Fetch(ctx, string(aObj.TenantAccountID)).Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsAccount().Return(oA).MaxTimes(2)
	oSys = mock.NewMockSystemOps(mockCtrl)
	oSys.EXPECT().Fetch().Return(sysObj, nil)
	mds.EXPECT().OpsSystem().Return(oSys)
	oAG.EXPECT().Fetch(ctx, string(params.Payload.ApplicationGroupIds[0])).Return(agObj, nil)
	oAG.EXPECT().Fetch(ctx, string(params.Payload.ApplicationGroupIds[1])).Return(agObj2, nil)
	mds.EXPECT().OpsApplicationGroup().Return(oAG).Times(2)
	oCG.EXPECT().Create(ctx, params.Payload).Return(obj, nil)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	fa.Posts = []*fal.Args{}
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.consistencyGroupCreate(params) })
	assert.NotNil(ret)
	_, ok = ret.(*ops.ConsistencyGroupCreateCreated)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Len(evM.InSSProps, 1)
	assert.EqualValues("objectId", evM.InSSProps["meta.id"])
	assert.EqualValues("tid1", params.Payload.TenantAccountID)
	assert.Equal(obj, evM.InACScope)
	exp = &fal.Args{AI: ai, Action: centrald.ConsistencyGroupCreateAction, ObjID: "objectId", Name: params.Payload.Name, RefID: "ag1", Message: "Created in application group [app]"}
	exp2 := &fal.Args{AI: ai, Action: centrald.ConsistencyGroupUpdateAction, ObjID: "objectId", Name: params.Payload.Name, RefID: "ag2", Message: "Added to application group [rap]"}
	assert.Equal([]*fal.Args{exp, exp2}, fa.Posts)
	assert.Empty(fa.Events)
	params.Payload.ApplicationGroupIds = params.Payload.ApplicationGroupIds[0:1] // reset
	obj.ApplicationGroupIds = params.Payload.ApplicationGroupIds
	tl.Flush()

	t.Log("case: auditLog not ready")
	fa.Posts = []*fal.Args{}
	fa.ReadyRet = centrald.ErrorDbError
	assert.NotPanics(func() { ret = hc.consistencyGroupCreate(params) })
	mE, ok := ret.(*ops.ConsistencyGroupCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.consistencyGroupCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ConsistencyGroupCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	t.Log("case: not authorized")
	fa.Posts = []*fal.Args{}
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	assert.NotPanics(func() { ret = hc.consistencyGroupCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ConsistencyGroupCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	exp = &fal.Args{AI: ai, Action: centrald.ConsistencyGroupCreateAction, Name: params.Payload.Name, Err: true, Message: "Create unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	ai.AccountID, ai.RoleObj = "", nil

	// all the remaining error cases
	var payload models.ConsistencyGroup
	for tc := 0; tc <= 5; tc++ {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		payload = *params.Payload
		fa.Posts = []*fal.Args{}
		mds = mock.NewMockDataStore(mockCtrl)
		oA = mock.NewMockAccountOps(mockCtrl)
		oAG = mock.NewMockApplicationGroupOps(mockCtrl)
		oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
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
			agObj.AccountID = "aid1"
			oAG.EXPECT().Fetch(ctx, string(payload.ApplicationGroupIds[0])).Return(agObj, nil)
			mds.EXPECT().OpsApplicationGroup().Return(oAG)
			oCG.EXPECT().Create(ctx, &p2).Return(nil, expectedErr)
			mds.EXPECT().OpsConsistencyGroup().Return(oCG)
		case 4:
			t.Log("case: validateApplicationGroupIds fails due to non-found AG")
			agObj.AccountID = payload.AccountID
			oA.EXPECT().Fetch(ctx, string(payload.AccountID)).Return(aObj, nil)
			oA.EXPECT().Fetch(ctx, string(aObj.TenantAccountID)).Return(nil, centrald.ErrorDbError)
			mds.EXPECT().OpsAccount().Return(oA).MaxTimes(2)
			oSys = mock.NewMockSystemOps(mockCtrl)
			oSys.EXPECT().Fetch().Return(sysObj, nil)
			mds.EXPECT().OpsSystem().Return(oSys)
			oAG.EXPECT().Fetch(ctx, string(payload.ApplicationGroupIds[0])).Return(nil, centrald.ErrorNotFound)
			mds.EXPECT().OpsApplicationGroup().Return(oAG)
		case 5:
			t.Log("case: validateApplicationGroupIds fails due to different accounts in AG and create op")
			expectedErr = &centrald.Error{
				M: fmt.Sprintf("applicationGroup %s is not available to the account", payload.ApplicationGroupIds[0]),
				C: centrald.ErrorUnauthorizedOrForbidden.C,
			}
			payload.AccountID = "account222"
			oA.EXPECT().Fetch(ctx, string(payload.AccountID)).Return(aObj, nil)
			oA.EXPECT().Fetch(ctx, string(aObj.TenantAccountID)).Return(nil, centrald.ErrorDbError)
			mds.EXPECT().OpsAccount().Return(oA).MaxTimes(2)
			oSys = mock.NewMockSystemOps(mockCtrl)
			oSys.EXPECT().Fetch().Return(sysObj, nil)
			mds.EXPECT().OpsSystem().Return(oSys)
			oAG.EXPECT().Fetch(ctx, string(payload.ApplicationGroupIds[0])).Return(agObj, nil)
			mds.EXPECT().OpsApplicationGroup().Return(oAG)
		default:
			assert.True(false)
		}
		hc.DS = mds
		assert.NotPanics(func() {
			ret = hc.consistencyGroupCreate(ops.ConsistencyGroupCreateParams{HTTPRequest: requestWithAuthContext(ai), Payload: &payload})
		})
		assert.NotNil(ret)
		mE, ok = ret.(*ops.ConsistencyGroupCreateDefault)
		assert.True(ok)
		assert.Equal(expectedErr.C, int(mE.Payload.Code))
		assert.Regexp("^"+expectedErr.M, *mE.Payload.Message)
		if tc > 0 {
			assert.Equal(cntRLock+1, hc.cntRLock)
			assert.Equal(cntRUnlock+1, hc.cntRUnlock)
		}
		if tc == 5 {
			assert.Equal([]*fal.Args{exp}, fa.Posts)
		} else {
			assert.Empty(fa.Posts)
		}
		ai.AccountID, ai.TenantAccountID = "", ""
		tl.Flush()
	}

	// no payload
	params.Payload = nil
	t.Log("case: nil payload")
	assert.NotPanics(func() { ret = hc.consistencyGroupCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ConsistencyGroupCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
}

func TestConsistencyGroupDelete(t *testing.T) {
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
	params := ops.ConsistencyGroupDeleteParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.ConsistencyGroup{
		ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
			Meta: &models.ObjMeta{ID: "objectID"},
		},
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oCG := mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCG.EXPECT().Delete(ctx, params.ID).Return(nil)
	oV := mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Count(ctx, volume_series.VolumeSeriesListParams{ConsistencyGroupID: &params.ID}, uint(1)).Return(0, nil)
	oS := mock.NewMockSnapshotOps(mockCtrl)
	oS.EXPECT().Count(ctx, snapshot.SnapshotListParams{ConsistencyGroupID: &params.ID}, uint(1)).Return(0, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG).MinTimes(1)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	mds.EXPECT().OpsSnapshot().Return(oS)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	cntLock := hc.cntLock
	cntUnlock := hc.cntUnlock
	assert.NotPanics(func() { ret = hc.consistencyGroupDelete(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.ConsistencyGroupDeleteNoContent)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Empty(evM.InSSProps)
	assert.Equal(obj, evM.InACScope)
	exp := &fal.Args{AI: ai, Action: centrald.ConsistencyGroupDeleteAction, ObjID: models.ObjID(params.ID), Name: obj.Name, Message: "Deleted"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	tl.Flush()

	// delete failure case (related VS)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: delete failure (related VS)")
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCG.EXPECT().Delete(ctx, params.ID).Return(centrald.ErrorDbError)
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Count(ctx, volume_series.VolumeSeriesListParams{ConsistencyGroupID: &params.ID}, uint(1)).Return(0, nil)
	oS = mock.NewMockSnapshotOps(mockCtrl)
	oS.EXPECT().Count(ctx, snapshot.SnapshotListParams{ConsistencyGroupID: &params.ID}, uint(1)).Return(0, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG).MinTimes(1)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	mds.EXPECT().OpsSnapshot().Return(oS)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.consistencyGroupDelete(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.ConsistencyGroupDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	// delete failure case (related snapshots)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: delete failure (related snapshots)")
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCG.EXPECT().Delete(ctx, params.ID).Return(centrald.ErrorDbError)
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Count(ctx, volume_series.VolumeSeriesListParams{ConsistencyGroupID: &params.ID}, uint(1)).Return(0, nil)
	oS = mock.NewMockSnapshotOps(mockCtrl)
	oS.EXPECT().Count(ctx, snapshot.SnapshotListParams{ConsistencyGroupID: &params.ID}, uint(1)).Return(0, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG).MinTimes(1)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	mds.EXPECT().OpsSnapshot().Return(oS)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.consistencyGroupDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ConsistencyGroupDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: auditLog not ready")
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG).MinTimes(1)
	hc.DS = mds
	fa.ReadyRet = centrald.ErrorDbError
	assert.NotPanics(func() { ret = hc.consistencyGroupDelete(params) })
	mE, ok = ret.(*ops.ConsistencyGroupDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.consistencyGroupDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ConsistencyGroupDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized")
	fa.Posts = []*fal.Args{}
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	assert.NotPanics(func() { ret = hc.consistencyGroupDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ConsistencyGroupDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	exp = &fal.Args{AI: ai, Action: centrald.ConsistencyGroupDeleteAction, ObjID: models.ObjID(params.ID), Name: obj.Name, Err: true, Message: "Delete unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	ai.RoleObj = nil
	tl.Flush()

	// VolumeSeries exist
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: VolumeSeries exist")
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Count(ctx, volume_series.VolumeSeriesListParams{ConsistencyGroupID: &params.ID}, uint(1)).Return(1, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.consistencyGroupDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ConsistencyGroupDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Regexp("contains one or more volume series", *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	// snapshots exist
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: snapshots exist")
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Count(ctx, volume_series.VolumeSeriesListParams{ConsistencyGroupID: &params.ID}, uint(1)).Return(0, nil)
	oS = mock.NewMockSnapshotOps(mockCtrl)
	oS.EXPECT().Count(ctx, snapshot.SnapshotListParams{ConsistencyGroupID: &params.ID}, uint(1)).Return(2, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG).MinTimes(1)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	mds.EXPECT().OpsSnapshot().Return(oS)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.consistencyGroupDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ConsistencyGroupDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Regexp("is referenced by one or more volume series snapshots", *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	// VolumeSeries count fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: VolumeSeries count fails")
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Count(ctx, volume_series.VolumeSeriesListParams{ConsistencyGroupID: &params.ID}, uint(1)).Return(0, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.consistencyGroupDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ConsistencyGroupDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	// Snapshots count fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: snapshots count fails")
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Count(ctx, volume_series.VolumeSeriesListParams{ConsistencyGroupID: &params.ID}, uint(1)).Return(0, nil)
	oS = mock.NewMockSnapshotOps(mockCtrl)
	oS.EXPECT().Count(ctx, snapshot.SnapshotListParams{ConsistencyGroupID: &params.ID}, uint(1)).Return(0, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	mds.EXPECT().OpsSnapshot().Return(oS)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.consistencyGroupDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ConsistencyGroupDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	// fetch failure case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	t.Log("case: fetch failure")
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.consistencyGroupDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ConsistencyGroupDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
	assert.Equal(cntLock, hc.cntLock)
	assert.Equal(cntUnlock, hc.cntUnlock)
}

func TestConsistencyGroupFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.ConsistencyGroupFetchParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.ConsistencyGroup{
		ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
			Meta: &models.ObjMeta{
				ID: "objectID",
			},
		},
		ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
			AccountID:       "aid1",
			TenantAccountID: "tid1",
		},
		ConsistencyGroupMutable: models.ConsistencyGroupMutable{
			SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
				RetentionDurationSeconds: swag.Int32(99),
				Inherited:                false,
			},
		},
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oCG := mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.consistencyGroupFetch(params) })
	assert.NotNil(ret)
	mR, ok := ret.(*ops.ConsistencyGroupFetchOK)
	assert.True(ok)
	assert.Equal(obj, mR.Payload)
	tl.Flush()

	// fetch failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	t.Log("case: fetch failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.consistencyGroupFetch(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.ConsistencyGroupFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
	tl.Flush()

	// consistencyGroupApplyInheritedProperties fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	obj.SnapshotManagementPolicy.Inherited = true
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "aid1").Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsAccount().Return(oA)
	t.Log("case: consistencyGroupApplyInheritedProperties fails")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.consistencyGroupFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ConsistencyGroupFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.consistencyGroupFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ConsistencyGroupFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized")
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	hc.DS = mds
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	assert.NotPanics(func() { ret = hc.consistencyGroupFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ConsistencyGroupFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	ai.RoleObj = nil
}

func TestConsistencyGroupList(t *testing.T) {
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
	params := ops.ConsistencyGroupListParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	ctx := params.HTTPRequest.Context()
	objects := []*models.ConsistencyGroup{
		&models.ConsistencyGroup{
			ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
				Meta: &models.ObjMeta{
					ID: "objectID1",
				},
			},
			ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
				AccountID:       "aid1",
				TenantAccountID: "tid1",
			},
			ConsistencyGroupMutable: models.ConsistencyGroupMutable{
				SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
					RetentionDurationSeconds: swag.Int32(99),
					Inherited:                false,
				},
			},
		},
		&models.ConsistencyGroup{
			ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
				Meta: &models.ObjMeta{
					ID: "objectID2",
				},
			},
			ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
				AccountID:       "aid2",
				TenantAccountID: "tid1",
			},
			ConsistencyGroupMutable: models.ConsistencyGroupMutable{
				SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
					RetentionDurationSeconds: swag.Int32(99),
					Inherited:                false,
				},
			},
		},
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oCG := mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().List(ctx, params).Return(objects, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.consistencyGroupList(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.ConsistencyGroupListOK)
	assert.True(ok)
	assert.EqualValues(objects, mO.Payload)
	tl.Flush()

	// list fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().List(ctx, params).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	t.Log("case: List failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.consistencyGroupList(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.ConsistencyGroupListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()

	// consistencyGroupApplyInheritedProperties fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().List(ctx, params).Return(objects, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "aid1").Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsAccount().Return(oA)
	objects[0].SnapshotManagementPolicy.Inherited = true
	t.Log("case: consistencyGroupApplyInheritedProperties fails")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.consistencyGroupList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.ConsistencyGroupListOK)
	assert.True(ok)
	assert.Len(mO.Payload, 1) // only one obj needs to be updated
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.consistencyGroupList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ConsistencyGroupListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	mockCtrl.Finish()
	t.Log("case: constrainEitherOrQueryAccounts changes accounts")
	ai.RoleObj = &models.Role{}
	params.AccountID, params.TenantAccountID = swag.String("a1"), swag.String("t1")
	fops.RetConstrainEOQueryAccountsAID, fops.RetConstrainEOQueryAccountsTID = swag.String("aid1"), swag.String("tid1")
	cParams := params
	cParams.AccountID, cParams.TenantAccountID = fops.RetConstrainEOQueryAccountsAID, fops.RetConstrainEOQueryAccountsTID
	objects[0].SnapshotManagementPolicy.Inherited = false
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().List(ctx, cParams).Return(objects, nil)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.consistencyGroupList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.ConsistencyGroupListOK)
	assert.True(ok)
	assert.Equal(4, fops.CntConstrainEOQueryAccounts)
	assert.Equal(params.AccountID, fops.InConstrainEOQueryAccountsAID)
	assert.Equal(params.TenantAccountID, fops.InConstrainEOQueryAccountsTID)
	tl.Flush()

	t.Log("case: constrainEitherOrQueryAccounts nil accounts")
	ai.RoleObj = &models.Role{}
	params.AccountID, params.TenantAccountID = swag.String("a1"), swag.String("t1")
	fops.RetConstrainEOQueryAccountsAID, fops.RetConstrainEOQueryAccountsTID = nil, nil
	assert.NotPanics(func() { ret = hc.consistencyGroupList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.ConsistencyGroupListOK)
	assert.True(ok)
	assert.Equal(5, fops.CntConstrainEOQueryAccounts)
	assert.Equal(params.AccountID, fops.InConstrainEOQueryAccountsAID)
	assert.Equal(params.TenantAccountID, fops.InConstrainEOQueryAccountsTID)
	tl.Flush()

	t.Log("case: constrainEitherOrQueryAccounts error")
	ai.RoleObj = &models.Role{}
	params.AccountID, params.TenantAccountID = swag.String("a1"), swag.String("t1")
	fops.RetConstrainEOQueryAccountsErr = centrald.ErrorDbError
	assert.NotPanics(func() { ret = hc.consistencyGroupList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ConsistencyGroupListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	assert.Equal(6, fops.CntConstrainEOQueryAccounts)
}

func TestConsistencyGroupUpdate(t *testing.T) {
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
	assert.NotPanics(func() { nMap = hc.consistencyGroupMutableNameMap() })
	// validate some embedded properties
	assert.Equal("name", nMap.jName("Name"))
	assert.Equal("systemTags", nMap.jName("SystemTags"))

	// parse params
	objM := &models.ConsistencyGroupMutable{}
	objM.Name = "cg1"
	objM.ApplicationGroupIds = []models.ObjIDMutable{"ag2"}
	params := ops.ConsistencyGroupUpdateParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
		Version:     swag.Int32(8),
		Remove:      []string{},
		Append:      []string{},
		Set:         []string{nMap.jName("Name"), nMap.jName("ApplicationGroupIds")},
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

	obj := &models.ConsistencyGroup{
		ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
			Meta: &models.ObjMeta{ID: "objectID", Version: models.ObjVersion(*params.Version)},
		},
		ConsistencyGroupMutable: models.ConsistencyGroupMutable{
			SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
				RetentionDurationSeconds: swag.Int32(99),
				Inherited:                false,
			},
			ApplicationGroupIds: []models.ObjIDMutable{"ag0", "ag00"},
		},
	}
	obj.AccountID = "account1"
	agObj0 := &models.ApplicationGroup{
		ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
			Meta: &models.ObjMeta{ID: "ag0"},
		},
		ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
			AccountID: "account1",
		},
		ApplicationGroupMutable: models.ApplicationGroupMutable{
			Name: "app0",
		},
	}
	agObj00 := &models.ApplicationGroup{
		ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
			Meta: &models.ObjMeta{ID: "ag00"},
		},
		ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
			AccountID: "account1",
		},
		ApplicationGroupMutable: models.ApplicationGroupMutable{
			Name: "app00",
		},
	}
	agObj1 := &models.ApplicationGroup{
		ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
			Meta: &models.ObjMeta{ID: "ag1"},
		},
		ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
			AccountID: "account1",
		},
		ApplicationGroupMutable: models.ApplicationGroupMutable{
			Name: "app",
		},
	}
	agObj2 := &models.ApplicationGroup{
		ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
			Meta: &models.ObjMeta{ID: "ag2"},
		},
		ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
			AccountID: "account1",
		},
		ApplicationGroupMutable: models.ApplicationGroupMutable{
			Name: "app2",
		},
	}
	aObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("account1"),
			},
		},
		AccountMutable: models.AccountMutable{
			Name: "account",
			SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
				RetentionDurationSeconds: swag.Int32(99),
				Inherited:                false,
			},
			SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
				CspDomainID:        "domID",
				ProtectionDomainID: "pdID",
				Inherited:          false,
			},
			VsrManagementPolicy: &models.VsrManagementPolicy{
				RetentionDurationSeconds: swag.Int32(100),
				NoDelete:                 false,
				Inherited:                false,
			},
		},
	}
	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mds := mock.NewMockDataStore(mockCtrl)
	oAG := mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Fetch(ctx, string(params.Payload.ApplicationGroupIds[0])).Return(agObj1, nil)
	oAG.EXPECT().Fetch(ctx, string(obj.ApplicationGroupIds[0])).Return(agObj0, nil)
	oAG.EXPECT().Fetch(ctx, string(obj.ApplicationGroupIds[1])).Return(agObj00, nil)
	mds.EXPECT().OpsApplicationGroup().Return(oAG).Times(3)
	oCG := mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCG.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG).MinTimes(1)
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.consistencyGroupUpdate(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.ConsistencyGroupUpdateOK)
	assert.True(ok)
	assert.Equal(obj, mO.Payload)
	assert.Empty(evM.InSSProps)
	assert.Equal(obj, evM.InACScope)
	exp := []*fal.Args{
		{AI: ai, Action: centrald.ConsistencyGroupUpdateAction, ObjID: models.ObjID(params.ID), Name: obj.Name, RefID: "ag1", Message: "Added to application group [app]"},
		{AI: ai, Action: centrald.ConsistencyGroupUpdateAction, ObjID: models.ObjID(params.ID), Name: obj.Name, RefID: "ag0", Message: "Removed from application group [app0]"},
		{AI: ai, Action: centrald.ConsistencyGroupUpdateAction, ObjID: models.ObjID(params.ID), Name: obj.Name, RefID: "ag00", Message: "Removed from application group [app00]"},
	}
	assert.Equal(exp, fa.Posts)
	tl.Flush()

	mockCtrl.Finish()
	t.Log("case: success, no version, validation adds version, valid auth")
	mockCtrl = gomock.NewController(t)
	fa.Posts = []*fal.Args{}
	mds = mock.NewMockDataStore(mockCtrl)
	obj.Meta.Version = 3
	aiVO := &auth.Info{
		AccountID:       "account1",
		TenantAccountID: "tid1",
		RoleObj: &models.Role{
			RoleMutable: models.RoleMutable{
				Capabilities: map[string]bool{centrald.AccountFetchAllRolesCap: true, centrald.VolumeSeriesOwnerCap: true},
			},
		},
	}
	params.HTTPRequest = requestWithAuthContext(aiVO)
	ctx = params.HTTPRequest.Context()
	params.Version = nil
	params.Append = []string{"applicationGroupIds", "tags"}
	params.Set = []string{"description", "name", "snapshotManagementPolicy"}
	params.Payload.ApplicationGroupIds = []models.ObjIDMutable{"ag0", "ag2", "ag1"} // append, overlap OK
	params.Payload.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{NoDelete: true}
	params.Payload.Tags = []string{"new:tag"}
	uP[centrald.UpdateAppend] = params.Append
	uP[centrald.UpdateSet] = params.Set
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).AddsVersion(3)
	oAG = mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Fetch(ctx, string(params.Payload.ApplicationGroupIds[1])).Return(agObj2, nil)
	oAG.EXPECT().Fetch(ctx, string(params.Payload.ApplicationGroupIds[2])).Return(agObj1, nil)
	mds.EXPECT().OpsApplicationGroup().Return(oAG).Times(2)
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCG.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG).MinTimes(1)
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(obj.AccountID)).Return(aObj, nil).MinTimes(1)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.consistencyGroupUpdate(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.ConsistencyGroupUpdateOK)
	assert.True(ok)
	if !ok {
		mD, ok := ret.(*ops.ConsistencyGroupUpdateDefault)
		assert.True(ok)
		assert.Fail("total failure", *mD.Payload.Message)
	}
	assert.Equal(obj, mO.Payload)
	exp = []*fal.Args{
		{AI: aiVO, Action: centrald.ConsistencyGroupUpdateAction, ObjID: models.ObjID(params.ID), Name: obj.Name, RefID: "ag1", Message: "Added to application group [app]"},
		{AI: aiVO, Action: centrald.ConsistencyGroupUpdateAction, ObjID: models.ObjID(params.ID), Name: obj.Name, RefID: "ag2", Message: "Added to application group [app2]"},
	}
	assert.Equal(exp, fa.Posts)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	mockCtrl.Finish()
	t.Log("case: success, modify non-inherited snapshot management policy, remove AG")
	mockCtrl = gomock.NewController(t)
	fa.Posts = []*fal.Args{}
	mds = mock.NewMockDataStore(mockCtrl)
	obj.Meta.Version = 3
	obj.SnapshotManagementPolicy.Inherited = true
	params.HTTPRequest = requestWithAuthContext(aiVO)
	ctx = params.HTTPRequest.Context()
	params.Append = []string{}
	params.Set = []string{"snapshotManagementPolicy"}
	params.Remove = []string{"applicationGroupIds"}
	params.Payload.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{NoDelete: true}
	params.Payload.ApplicationGroupIds = []models.ObjIDMutable{"ag00"}
	uP[centrald.UpdateAppend] = params.Append
	uP[centrald.UpdateSet] = params.Set
	uP[centrald.UpdateRemove] = params.Remove
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).AddsVersion(3)
	oAG = mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Fetch(ctx, string(params.Payload.ApplicationGroupIds[0])).Return(agObj00, nil)
	mds.EXPECT().OpsApplicationGroup().Return(oAG)
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCG.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG).MinTimes(1)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(obj.AccountID)).Return(aObj, nil).MinTimes(1)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.consistencyGroupUpdate(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.ConsistencyGroupUpdateOK)
	assert.True(ok)
	assert.Equal(obj, mO.Payload)
	exp = []*fal.Args{{AI: aiVO, Action: centrald.ConsistencyGroupUpdateAction, ObjID: models.ObjID(params.ID), Name: obj.Name, RefID: "ag00", Message: "Removed from application group [app00]"}}
	assert.Equal(exp, fa.Posts)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	mockCtrl.Finish()
	t.Log("case: failure, modify non-inherited snapshot management policy")
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	obj.Meta.Version = 3
	obj.SnapshotManagementPolicy.Inherited = true
	params.HTTPRequest = requestWithAuthContext(aiVO)
	ctx = params.HTTPRequest.Context()
	params.Append = []string{}
	params.Set = []string{"snapshotManagementPolicy"}
	params.Remove = []string{"applicationGroupIds"}
	params.Payload.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{NoDelete: true}
	params.Payload.ApplicationGroupIds = []models.ObjIDMutable{"ag00"}
	uP[centrald.UpdateAppend] = params.Append
	uP[centrald.UpdateSet] = params.Set
	uP[centrald.UpdateRemove] = params.Remove
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).AddsVersion(3)
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG).MinTimes(1)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(obj.AccountID)).Return(nil, centrald.ErrorNotFound).MinTimes(1)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.consistencyGroupUpdate(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.ConsistencyGroupUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	mockCtrl.Finish()
	t.Log("case: success, setting inherited snapshot management policy flag overwrites all other possible options")
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	obj.SnapshotManagementPolicy.Inherited = false
	params.HTTPRequest = requestWithAuthContext(aiVO)
	ctx = params.HTTPRequest.Context()
	params.Append = []string{}
	params.Set = []string{"snapshotManagementPolicy"}
	params.Payload.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{
		NoDelete:  true,
		Inherited: true,
	}
	uP[centrald.UpdateAppend] = params.Append
	uP[centrald.UpdateSet] = params.Set
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).AddsVersion(3)
	oAG = mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Fetch(ctx, string(params.Payload.ApplicationGroupIds[0])).Return(agObj00, nil)
	mds.EXPECT().OpsApplicationGroup().Return(oAG)
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCG.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG).MinTimes(1)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(obj.AccountID)).Return(aObj, nil).MinTimes(1)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.consistencyGroupUpdate(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.ConsistencyGroupUpdateOK)
	assert.True(ok)
	assert.Equal(obj, mO.Payload)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	obj.SnapshotManagementPolicy.Inherited = false
	tl.Flush()

	mockCtrl.Finish()
	t.Log("case: failure, attempt to modify inherited snapshot management policy with NoDelete false and no retention period specified")
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	obj.SnapshotManagementPolicy.Inherited = true
	params.HTTPRequest = requestWithAuthContext(aiVO)
	ctx = params.HTTPRequest.Context()
	params.Append = []string{}
	params.Set = []string{"snapshotManagementPolicy"}
	params.Payload.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{
		NoDelete:                 false,
		Inherited:                false,
		RetentionDurationSeconds: swag.Int32(0),
	}
	uP[centrald.UpdateAppend] = params.Append
	uP[centrald.UpdateSet] = params.Set
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).AddsVersion(3)
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.consistencyGroupUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ConsistencyGroupUpdateDefault)
	assert.True(ok)
	expectedErr := &centrald.Error{M: "proper RetentionDurationSeconds value should be provided if 'NoDelete' flag is false", C: 400}
	assert.Equal(expectedErr.C, int(mD.Payload.Code))
	assert.Regexp(".*"+expectedErr.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	obj.SnapshotManagementPolicy.Inherited = false
	tl.Flush()

	// Update failed after successful verification paths
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: update failed, valid internal auth")
	params.Append = []string{"systemTags"}
	params.Set = []string{"description"}
	params.Remove = []string{}
	params.Payload.ApplicationGroupIds = []models.ObjIDMutable{"ag1"}
	params.Payload.SystemTags = []string{"system1:tag", "system2:tag"}
	params.Version = swag.Int32(8)
	obj.Meta.Version = 8
	obj.ApplicationGroupIds = []models.ObjIDMutable{"ag1", "ag2"}
	uP[centrald.UpdateAppend] = params.Append
	uP[centrald.UpdateSet] = params.Set
	uP[centrald.UpdateRemove] = params.Remove
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua)
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCG.EXPECT().Update(ctx, uaM, params.Payload).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.consistencyGroupUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ConsistencyGroupUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: fetch failure")
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.consistencyGroupUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ConsistencyGroupUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mD.Payload.Message)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: auditLog not ready")
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	fa.ReadyRet = centrald.ErrorDbError
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.consistencyGroupUpdate(params) })
	mD, ok = ret.(*ops.ConsistencyGroupUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.consistencyGroupUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ConsistencyGroupUpdateDefault)
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
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.consistencyGroupUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ConsistencyGroupUpdateDefault)
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
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.consistencyGroupUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ConsistencyGroupUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)
	ai.RoleObj = nil
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	// missing/invalid updates
	var verP *int32
	for tc := 1; tc <= 11; tc++ {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		mds = mock.NewMockDataStore(mockCtrl)
		oAG = mock.NewMockApplicationGroupOps(mockCtrl)
		oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
		obj.AccountID = "account1"
		payloadCopy := *params.Payload
		payload := &payloadCopy
		set := []string{"name"}
		append := []string{}
		remove := []string{}
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
			expectedErr = centrald.ErrorIDVerNotFound
		case 5:
			t.Log("case: applicationGroupId fetch fails")
			set = []string{"applicationGroupIds.0"}
			payload.ApplicationGroupIds = []models.ObjIDMutable{"ag3"}
			expectedErr = centrald.ErrorDbError
			oAG.EXPECT().Fetch(ctx, string(payload.ApplicationGroupIds[0])).Return(nil, expectedErr)
			mds.EXPECT().OpsApplicationGroup().Return(oAG)
		case 6:
			t.Log("case: applicationGroupId not found")
			set = []string{"applicationGroupIds"}
			payload.ApplicationGroupIds = []models.ObjIDMutable{"ag3"}
			oAG.EXPECT().Fetch(ctx, string(payload.ApplicationGroupIds[0])).Return(nil, centrald.ErrorNotFound)
			mds.EXPECT().OpsApplicationGroup().Return(oAG)
		case 7:
			t.Log("case: applicationGroup is not accessible to Account")
			set = []string{"applicationGroupIds"}
			payload.ApplicationGroupIds = []models.ObjIDMutable{"ag3"}
			obj.AccountID = "another"
			expectedErr = &centrald.Error{M: "applicationGroup", C: 403}
			oAG.EXPECT().Fetch(ctx, string(payload.ApplicationGroupIds[0])).Return(agObj0, nil)
			mds.EXPECT().OpsApplicationGroup().Return(oAG)
		case 8:
			t.Log("case: applicationGroup set not unique")
			set = []string{"applicationGroupIds"}
			payload.ApplicationGroupIds = []models.ObjIDMutable{"ag3", "ag4", "ag4"}
			expectedErr = &centrald.Error{M: ".*applicationGroupIds.* unique: ag4", C: 400}
		case 9:
			t.Log("case: remove all applicationGroups")
			set = []string{}
			remove = []string{"applicationGroupIds"}
			payload.ApplicationGroupIds = []models.ObjIDMutable{"ag1", "ag2"}
			expectedErr = &centrald.Error{M: ".*remove all applicationGroupIds", C: 400}
		case 10:
			t.Log("case: remove applicationGroup fetch fails")
			set = []string{}
			remove = []string{"applicationGroupIds"}
			payload.ApplicationGroupIds = []models.ObjIDMutable{"ag1"}
			expectedErr = centrald.ErrorDbError
			oAG.EXPECT().Fetch(ctx, string(payload.ApplicationGroupIds[0])).Return(nil, expectedErr)
			mds.EXPECT().OpsApplicationGroup().Return(oAG)
		case 11:
			t.Log("case: SET non-empty required")
			set = []string{"applicationGroupIds"}
			payload.ApplicationGroupIds = []models.ObjIDMutable{}
			expectedErr = &centrald.Error{M: ".*non-empty applicationGroupIds required", C: 400}
		default:
			assert.False(true)
		}
		if tc > 3 {
			oCG.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			mds.EXPECT().OpsConsistencyGroup().Return(oCG)
		}
		hc.DS = mds
		assert.NotPanics(func() {
			ret = hc.consistencyGroupUpdate(ops.ConsistencyGroupUpdateParams{HTTPRequest: requestWithAuthContext(ai), ID: params.ID, Payload: payload, Set: set, Append: append, Remove: remove, Version: verP})
		})
		assert.NotNil(ret)
		mD, ok = ret.(*ops.ConsistencyGroupUpdateDefault)
		assert.True(ok)
		assert.Equal(expectedErr.C, int(mD.Payload.Code))
		assert.Regexp("^"+expectedErr.M, *mD.Payload.Message)
		tl.Flush()
	}
}
