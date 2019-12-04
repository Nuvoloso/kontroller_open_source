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
	"fmt"
	"net/http"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	fal "github.com/Nuvoloso/kontroller/pkg/centrald/audit/fake"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	mockcluster "github.com/Nuvoloso/kontroller/pkg/cluster/mock"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestVolumeSeriesFetchFilter(t *testing.T) {
	assert := assert.New(t)
	hc := newHandlerComp()
	obj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{Meta: &models.ObjMeta{ID: "id1"}},
		VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
			AccountID:       "aid1",
			TenantAccountID: "tid1",
		},
	}
	rObj := &models.Role{}
	rObj.Capabilities = map[string]bool{}
	ai := &auth.Info{}

	t.Log("case: internal user")
	assert.NoError(hc.volumeSeriesFetchFilter(ai, obj))

	t.Log("case: SystemManagementCap capability fails")
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.SystemManagementCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.volumeSeriesFetchFilter(ai, obj))

	t.Log("case: CSPDomainUsageCap capability fails")
	ai.RoleObj = &models.Role{}
	ai.AccountID = "aid1"
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.volumeSeriesFetchFilter(ai, obj))

	t.Log("case: VolumeSeriesFetchCap capability correct tenant")
	ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesFetchCap: true}
	ai.AccountID = "tid1"
	assert.NoError(hc.volumeSeriesFetchFilter(ai, obj))

	t.Log("case: VolumeSeriesFetchCap capability wrong tenant")
	ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesFetchCap: true}
	ai.AccountID = "tid2"
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.volumeSeriesFetchFilter(ai, obj))

	t.Log("case: owner account")
	ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: true}
	ai.AccountID = "aid1"
	assert.NoError(hc.volumeSeriesFetchFilter(ai, obj))

	t.Log("case: not owner account")
	ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: true}
	ai.AccountID = "aid9"
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.volumeSeriesFetchFilter(ai, obj))
}

func TestVolumeSeriesCreate(t *testing.T) {
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
	params := ops.VolumeSeriesCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
		Payload:     &models.VolumeSeriesCreateArgs{},
	}
	params.Payload.AccountID = "account2"
	params.Payload.Name = "vs1"
	params.Payload.Description = "a volume series"
	params.Payload.ConsistencyGroupID = "con"
	params.Payload.ServicePlanID = "awesome"
	params.Payload.SizeBytes = swag.Int64(1)
	params.Payload.Tags = []string{"prod"}
	ctx := params.HTTPRequest.Context()
	obj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{ID: models.ObjID("objectId"), Version: 1},
		},
		VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
			AccountID: params.Payload.AccountID,
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				ConsistencyGroupID: params.Payload.ConsistencyGroupID,
				ServicePlanID:      params.Payload.ServicePlanID,
				Name:               params.Payload.Name,
			},
		},
	}
	aObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta:            &models.ObjMeta{ID: "account2"},
			TenantAccountID: "tid1",
		},
	}
	cgObj := &models.ConsistencyGroup{
		ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
			Meta: &models.ObjMeta{ID: models.ObjID("cg1")},
		},
		ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
			AccountID: "account2",
		},
		ConsistencyGroupMutable: models.ConsistencyGroupMutable{
			Name: "conG",
		},
	}
	spObj := &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			Meta:  &models.ObjMeta{ID: models.ObjID("awesome")},
			State: centrald.PublishedState,
		},
		ServicePlanMutable: models.ServicePlanMutable{
			Name:     "servicePlan",
			Accounts: []models.ObjIDMutable{"account1", "account2", "account3"},
		},
	}

	// success (all defaults)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	t.Log("case: success (all defaults)")
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(params.Payload.AccountID)).Return(aObj, nil)
	oSP := mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Fetch(ctx, string(params.Payload.ServicePlanID)).Return(spObj, nil)
	oCG := mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, string(params.Payload.ConsistencyGroupID)).Return(cgObj, nil)
	oV := mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Create(ctx, params.Payload).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	hc.DS = mds
	var ret middleware.Responder
	cntRLock := hc.cntRLock
	cntRUnlock := hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesCreate(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.VolumeSeriesCreateCreated)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Len(evM.InSSProps, 4)
	assert.EqualValues("account2", evM.InSSProps["accountId"])
	assert.EqualValues("con", evM.InSSProps["consistencyGroupId"])
	assert.EqualValues("awesome", evM.InSSProps["servicePlanId"])
	assert.EqualValues("objectId", evM.InSSProps["meta.id"])
	assert.EqualValues("tid1", params.Payload.TenantAccountID)
	assert.Equal(obj, evM.InACScope)
	exp := &fal.Args{AI: ai, Action: centrald.VolumeSeriesCreateAction, ObjID: "objectId", Name: params.Payload.Name, RefID: "con", Message: "Created with service plan[servicePlan] in consistency group[conG]"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	tl.Flush()

	// success with id specified (internal)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: success (id specified, internal)")
	assert.NoError(ai.InternalOK())
	params.Payload.SystemTags = []string{fmt.Sprintf("%s:some-id", com.SystemTagVolumeSeriesID)}
	fa.Posts = []*fal.Args{}
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(params.Payload.AccountID)).Return(aObj, nil)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Fetch(ctx, string(params.Payload.ServicePlanID)).Return(spObj, nil)
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, string(params.Payload.ConsistencyGroupID)).Return(cgObj, nil)
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Create(ctx, params.Payload).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesCreate(params) })
	assert.NotNil(ret)
	_, ok = ret.(*ops.VolumeSeriesCreateCreated)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Len(evM.InSSProps, 4)
	assert.EqualValues("account2", evM.InSSProps["accountId"])
	assert.EqualValues("con", evM.InSSProps["consistencyGroupId"])
	assert.EqualValues("awesome", evM.InSSProps["servicePlanId"])
	assert.EqualValues("objectId", evM.InSSProps["meta.id"])
	assert.EqualValues("tid1", params.Payload.TenantAccountID)
	assert.Equal(obj, evM.InACScope)
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	params.Payload.SystemTags = nil // reset
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.volumeSeriesCreate(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.VolumeSeriesCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	t.Log("case: not authorized")
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	fa.Posts = []*fal.Args{}
	assert.NotPanics(func() { ret = hc.volumeSeriesCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	exp = &fal.Args{AI: ai, Action: centrald.VolumeSeriesCreateAction, Name: params.Payload.Name, Err: true, Message: "Create unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	ai.AccountID, ai.RoleObj = "", nil
	tl.Flush()

	t.Log("case: auditLog not ready")
	fa.ReadyRet = centrald.ErrorDbError
	assert.NotPanics(func() { ret = hc.volumeSeriesCreate(params) })
	mE, ok = ret.(*ops.VolumeSeriesCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	// all the error cases
	var payload models.VolumeSeriesCreateArgs
	for tc := 0; tc <= 16; tc++ {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		payload = *params.Payload
		mds = mock.NewMockDataStore(mockCtrl)
		oA = mock.NewMockAccountOps(mockCtrl)
		oSP = mock.NewMockServicePlanOps(mockCtrl)
		oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
		oV = mock.NewMockVolumeSeriesOps(mockCtrl)
		cntRLock = hc.cntRLock
		cntRUnlock = hc.cntRUnlock
		spObj.State = centrald.PublishedState
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
			t.Log("case: ServicePlanID missing")
			payload.ServicePlanID = ""
		case 4:
			t.Log("case: ServicePlanID fetch fails")
			expectedErr = centrald.ErrorDbError
			// cover path that sets accounts from the auth.Info, ignoring the payload account
			ai.AccountID = "aid1"
			ai.TenantAccountID = "tid2"
			ai.RoleObj = &models.Role{}
			ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: true}
			oSP.EXPECT().Fetch(ctx, string(params.Payload.ServicePlanID)).Return(nil, expectedErr)
			mds.EXPECT().OpsServicePlan().Return(oSP)
		case 5:
			t.Log("case: ServicePlanID not found")
			oSP.EXPECT().Fetch(ctx, string(params.Payload.ServicePlanID)).Return(nil, centrald.ErrorNotFound)
			mds.EXPECT().OpsServicePlan().Return(oSP)
		case 6:
			t.Log("case: ServicePlanID", centrald.RetiredState)
			spObj.State = centrald.RetiredState
			oSP.EXPECT().Fetch(ctx, string(params.Payload.ServicePlanID)).Return(spObj, nil)
			mds.EXPECT().OpsServicePlan().Return(oSP)
		case 7:
			t.Log("case: ServicePlan not available to Account")
			ai.AccountID = "another"
			expectedErr = hc.eUnauthorizedOrForbidden("servicePlan is not available to the account").(*centrald.Error)
			oSP.EXPECT().Fetch(ctx, string(params.Payload.ServicePlanID)).Return(spObj, nil)
			mds.EXPECT().OpsServicePlan().Return(oSP)
		case 8:
			t.Log("case: ConsistencyGroupID fetch fails")
			expectedErr = centrald.ErrorDbError
			ai.AccountID = string(aObj.Meta.ID)
			payload.ConsistencyGroupID = "cg1"
			oSP.EXPECT().Fetch(ctx, string(params.Payload.ServicePlanID)).Return(spObj, nil)
			mds.EXPECT().OpsServicePlan().Return(oSP)
			oCG.EXPECT().Fetch(ctx, string(payload.ConsistencyGroupID)).Return(nil, expectedErr)
			mds.EXPECT().OpsConsistencyGroup().Return(oCG)
		case 9:
			t.Log("case: ConsistencyGroupID not found")
			payload.ConsistencyGroupID = "cg1"
			oSP.EXPECT().Fetch(ctx, string(params.Payload.ServicePlanID)).Return(spObj, nil)
			mds.EXPECT().OpsServicePlan().Return(oSP)
			oCG.EXPECT().Fetch(ctx, string(payload.ConsistencyGroupID)).Return(nil, centrald.ErrorNotFound)
			mds.EXPECT().OpsConsistencyGroup().Return(oCG)
		case 10:
			t.Log("case: ConsistencyGroupID not available to Account")
			cgObj.AccountID = "another"
			payload.ConsistencyGroupID = "cg1"
			expectedErr = hc.eUnauthorizedOrForbidden("consistencyGroup is not available to the account").(*centrald.Error)
			oSP.EXPECT().Fetch(ctx, string(params.Payload.ServicePlanID)).Return(spObj, nil)
			mds.EXPECT().OpsServicePlan().Return(oSP)
			oCG.EXPECT().Fetch(ctx, string(payload.ConsistencyGroupID)).Return(cgObj, nil)
			mds.EXPECT().OpsConsistencyGroup().Return(oCG)
		case 11:
			t.Log("case: Create fails because object exists")
			expectedErr = centrald.ErrorExists
			cgObj.AccountID = models.ObjIDMutable(ai.AccountID)
			oSP.EXPECT().Fetch(ctx, string(params.Payload.ServicePlanID)).Return(spObj, nil)
			mds.EXPECT().OpsServicePlan().Return(oSP)
			oCG.EXPECT().Fetch(ctx, string(payload.ConsistencyGroupID)).Return(cgObj, nil)
			mds.EXPECT().OpsConsistencyGroup().Return(oCG)
			oV.EXPECT().Create(ctx, gomock.Any()).Return(nil, expectedErr)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
		case 12:
			t.Log("case: ConsistencyGroupID missing")
			payload.ConsistencyGroupID = ""
		case 13:
			t.Log("case: sizeBytes missing")
			payload.SizeBytes = nil
		case 14:
			t.Log("case: sizeBytes 0")
			payload.SizeBytes = swag.Int64(0)
		case 15:
			t.Log("case: attempt to set identifier")
			assert.Error(ai.InternalOK())
			expectedErr = hc.eUnauthorizedOrForbidden("attempt to set object identifier").(*centrald.Error)
			payload.SystemTags = []string{fmt.Sprintf("%s:some-id", com.SystemTagVolumeSeriesID)}
		case 16:
			t.Log("case: attemp to set clusterDescriptor")
			assert.Error(ai.InternalOK())
			expectedErr = hc.eUnauthorizedOrForbidden("attempt to set clusterDescriptor").(*centrald.Error)
			payload.ClusterDescriptor = models.ClusterDescriptor{com.ClusterDescriptorPVName: models.ValueType{Kind: "STRING", Value: "pvname"}}
		default:
			assert.True(false)
		}
		hc.DS = mds
		assert.NotPanics(func() {
			ret = hc.volumeSeriesCreate(ops.VolumeSeriesCreateParams{HTTPRequest: requestWithAuthContext(ai), Payload: &payload})
		})
		assert.NotNil(ret)
		mE, ok = ret.(*ops.VolumeSeriesCreateDefault)
		assert.True(ok)
		assert.Equal(expectedErr.C, int(mE.Payload.Code))
		assert.Regexp("^"+expectedErr.M, *mE.Payload.Message)
		if payload.ConsistencyGroupID != "" {
			assert.Equal(cntRLock+1, hc.cntRLock)
			assert.Equal(cntRUnlock+1, hc.cntRUnlock)
		}
		if tc == 4 {
			assert.EqualValues("aid1", payload.AccountID)
			assert.EqualValues("tid2", payload.TenantAccountID)
		}
		tl.Flush()
	}

	// no payload
	params.Payload = nil
	t.Log("case: nil payload")
	assert.NotPanics(func() { ret = hc.volumeSeriesCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp(centrald.ErrorMissing.M, *mE.Payload.Message)
}

func TestVolumeSeriesDelete(t *testing.T) {
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
	params := ops.VolumeSeriesDeleteParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("objectID"),
			},
		},
		VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
			AccountID: "account2",
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				ServicePlanID: "awesome",
			},
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				VolumeSeriesState: "DELETING",
			},
		},
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oV := mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Delete(ctx, params.ID).Return(nil)
	oV.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oV).MinTimes(1)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	cntLock := hc.cntLock
	cntUnlock := hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesDelete(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.VolumeSeriesDeleteNoContent)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Len(evM.InSSProps, 2)
	assert.EqualValues("account2", evM.InSSProps["accountId"])
	assert.EqualValues("awesome", evM.InSSProps["servicePlanId"])
	assert.Equal(obj, evM.InACScope)
	exp := &fal.Args{AI: ai, Action: centrald.VolumeSeriesDeleteAction, ObjID: models.ObjID(params.ID), Name: obj.Name, Message: "Deleted"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	tl.Flush()

	// delete failure case, also covers case of allowing deletion with 0-sized parcels and deleting DELETING volumes
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: delete failure")
	obj.StorageParcels = make(map[string]models.ParcelAllocation, 0)
	obj.StorageParcels["storageId"] = models.ParcelAllocation{SizeBytes: swag.Int64(0)}
	obj.VolumeSeriesState = "DELETING"
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Delete(ctx, params.ID).Return(centrald.ErrorDbError)
	oV.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oV).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesDelete(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.VolumeSeriesDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.volumeSeriesDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized")
	fa.Posts = []*fal.Args{}
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	assert.NotPanics(func() { ret = hc.volumeSeriesDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	exp = &fal.Args{AI: ai, Action: centrald.VolumeSeriesDeleteAction, ObjID: models.ObjID(params.ID), Name: obj.Name, Err: true, Message: "Delete unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	ai.RoleObj = nil
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: auditLog not ready")
	fa.ReadyRet = centrald.ErrorDbError
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesDelete(params) })
	mE, ok = ret.(*ops.VolumeSeriesDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	fa.ReadyRet = nil
	tl.Flush()

	// wrong volumeSeriesState
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: wrong volumeSeriesState")
	obj.VolumeSeriesState = com.VolStateUnbound
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Regexp("not in DELETING state", *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	// non-zero mounts exist
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: non-zero mounts exist")
	obj.VolumeSeriesState = com.VolStateDeleting
	obj.Mounts = []*models.Mount{
		{MountMode: "READ_WRITE", MountState: "MOUNTED", MountedNodeDevice: "/dev/a", MountedNodeID: "node1", SnapIdentifier: "1"},
	}
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oV).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Regexp("still mounted", *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	obj.Mounts = []*models.Mount{}
	tl.Flush()

	// non-zero storage parcels exist
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: non-zero storage parcels exist")
	obj.StorageParcels["storageId"] = models.ParcelAllocation{SizeBytes: swag.Int64(1)}
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oV).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Regexp("Storage parcels", *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	// fetch failure case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oV).MinTimes(1)
	t.Log("case: fetch failure")
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
	assert.Equal(cntLock, hc.cntLock)
	assert.Equal(cntUnlock, hc.cntUnlock)
}

func TestVolumeSeriesFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.VolumeSeriesFetchParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("objectID"),
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{},
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oV := mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oV).MinTimes(1)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.volumeSeriesFetch(params) })
	assert.NotNil(ret)
	mR, ok := ret.(*ops.VolumeSeriesFetchOK)
	assert.True(ok)
	assert.Equal(obj, mR.Payload)
	tl.Flush()

	// fetch failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oV).MinTimes(1)
	t.Log("case: fetch failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesFetch(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.VolumeSeriesFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.volumeSeriesFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized")
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	hc.DS = mds
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	assert.NotPanics(func() { ret = hc.volumeSeriesFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	ai.RoleObj = nil
}

func TestVolumeSeriesList(t *testing.T) {
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
	params := ops.VolumeSeriesListParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	ctx := params.HTTPRequest.Context()
	objects := []*models.VolumeSeries{
		&models.VolumeSeries{
			VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
				Meta: &models.ObjMeta{
					ID: models.ObjID("objectID"),
				},
			},
			VolumeSeriesMutable: models.VolumeSeriesMutable{},
		},
	}
	aggregates := []*centrald.Aggregation{
		&centrald.Aggregation{FieldPath: "sizeBytes", Type: "sum", Value: 42},
	}

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oV := mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().List(ctx, params).Return(objects, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oV).MinTimes(1)
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.volumeSeriesList(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.VolumeSeriesListOK)
	assert.True(ok)
	assert.EqualValues(objects, mO.Payload)
	assert.Empty(mO.Aggregations)
	assert.Len(objects, int(mO.TotalCount))
	assert.Equal(1, fops.CntConstrainEOQueryAccounts)
	assert.Nil(fops.InConstrainEOQueryAccountsAID)
	assert.Nil(fops.InConstrainEOQueryAccountsTID)
	tl.Flush()

	// list fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().List(ctx, params).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oV).MinTimes(1)
	t.Log("case: List failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesList(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.VolumeSeriesListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	assert.Equal(2, fops.CntConstrainEOQueryAccounts)

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.volumeSeriesList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.VolumeSeriesListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	assert.Equal(2, fops.CntConstrainEOQueryAccounts)
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
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().List(ctx, cParams).Return(objects, nil)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.VolumeSeriesListOK)
	assert.True(ok)
	assert.Equal(3, fops.CntConstrainEOQueryAccounts)
	assert.Equal(params.AccountID, fops.InConstrainEOQueryAccountsAID)
	assert.Equal(params.TenantAccountID, fops.InConstrainEOQueryAccountsTID)
	tl.Flush()

	t.Log("case: constrainEitherOrQueryAccounts nil accounts")
	ai.RoleObj = &models.Role{}
	params.AccountID, params.TenantAccountID = swag.String("a1"), swag.String("t1")
	fops.RetConstrainEOQueryAccountsAID, fops.RetConstrainEOQueryAccountsTID = nil, nil
	assert.NotPanics(func() { ret = hc.volumeSeriesList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.VolumeSeriesListOK)
	assert.True(ok)
	assert.Equal(4, fops.CntConstrainEOQueryAccounts)
	assert.Equal(params.AccountID, fops.InConstrainEOQueryAccountsAID)
	assert.Equal(params.TenantAccountID, fops.InConstrainEOQueryAccountsTID)
	tl.Flush()

	t.Log("case: constrainEitherOrQueryAccounts error")
	ai.RoleObj = &models.Role{}
	params.AccountID, params.TenantAccountID = swag.String("a1"), swag.String("t1")
	fops.RetConstrainEOQueryAccountsErr = centrald.ErrorDbError
	assert.NotPanics(func() { ret = hc.volumeSeriesList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.VolumeSeriesListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	assert.Equal(5, fops.CntConstrainEOQueryAccounts)
	fops.RetConstrainEOQueryAccountsAID, fops.RetConstrainEOQueryAccountsTID = swag.String("a1"), swag.String("t1")
	fops.RetConstrainEOQueryAccountsErr = nil
	ai.RoleObj = nil
	tl.Flush()

	// success with aggregation
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: aggregation success")
	params.Sum = []string{"sizeBytes"}
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Aggregate(ctx, params).Return(aggregates, 5, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oV).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.VolumeSeriesListOK)
	assert.True(ok)
	assert.Len(mO.Payload, 0)
	assert.Equal([]string{"sizeBytes:sum:42"}, mO.Aggregations)
	assert.Equal(5, int(mO.TotalCount))
	assert.Equal(6, fops.CntConstrainEOQueryAccounts)
	tl.Flush()

	// aggregation fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: aggregation failure")
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Aggregate(ctx, params).Return(nil, 0, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oV).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.VolumeSeriesListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	assert.Equal(7, fops.CntConstrainEOQueryAccounts)
}

func TestVolumeSeriesNewID(t *testing.T) {
	assert := assert.New(t)
	hc := newHandlerComp()

	ai := &auth.Info{}
	params := ops.NewVolumeSeriesNewIDParams()

	t.Log("case: no auth info")
	params.HTTPRequest = &http.Request{}
	ret := hc.volumeSeriesNewID(params)
	assert.NotNil(ret)
	mE, ok := ret.(*ops.VolumeSeriesNewIDDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)

	t.Log("case: internal user")
	params.HTTPRequest = requestWithAuthContext(ai)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mds := mock.NewMockDataStore(mockCtrl)
	oV := mock.NewMockVolumeSeriesOps(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oV).MinTimes(1)
	oV.EXPECT().NewID().Return("new-id").MinTimes(1)
	hc.DS = mds
	ret = hc.volumeSeriesNewID(params)
	assert.NotNil(ret)
	mO, ok := ret.(*ops.VolumeSeriesNewIDOK)
	assert.True(ok)
	assert.Equal(com.ValueTypeString, mO.Payload.Kind)
	assert.Equal("new-id", mO.Payload.Value)

	t.Log("case: not internal user")
	ai.RoleObj = &models.Role{}
	ret = hc.volumeSeriesNewID(params)
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesNewIDDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
}

func TestVolumeSeriesUpdate(t *testing.T) {
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
	assert.NotPanics(func() { nMap = hc.volumeSeriesMutableNameMap() })
	// validate some embedded properties
	assert.Equal("name", nMap.jName("Name"))
	assert.Equal("servicePlanId", nMap.jName("ServicePlanID"))
	assert.Equal("messages", nMap.jName("Messages"))

	// parse params
	objM := &models.VolumeSeriesMutable{}
	objM.Name = "vs1"
	params := ops.VolumeSeriesUpdateParams{
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
	uaM := updateArgsMatcher(t, ua).Matcher()

	aObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{ID: "account1"},
		},
		AccountMutable: models.AccountMutable{
			Name: "aName",
		},
	}
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{ID: models.ObjID("cl1")},
		},
	}
	clObj.Name = "bunch"
	clObj.CspDomainID = "csp1"
	cgObj := &models.ConsistencyGroup{
		ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
			Meta: &models.ObjMeta{ID: models.ObjID("cg1")},
		},
		ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
			AccountID: "account1",
		},
		ConsistencyGroupMutable: models.ConsistencyGroupMutable{
			Name: "con",
		},
	}
	nObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta:      &models.ObjMeta{ID: models.ObjID("node1")},
			ClusterID: "cl1",
		},
	}
	pObj := &models.Pool{
		PoolAllOf0: models.PoolAllOf0{
			Meta: &models.ObjMeta{ID: models.ObjID("prov1")},
		},
		PoolCreateOnce: models.PoolCreateOnce{
			AccountID:           "tid1",
			AuthorizedAccountID: "account1",
		},
	}
	spObj := &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			Meta:  &models.ObjMeta{ID: models.ObjID("planId")},
			State: centrald.PublishedState,
		},
		ServicePlanMutable: models.ServicePlanMutable{
			Name:     "servicePlan",
			Accounts: []models.ObjIDMutable{"account1", "account2", "account3"},
		},
	}
	stObj := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta:      &models.ObjMeta{ID: models.ObjID("st1")},
			AccountID: "account1",
		},
	}

	t.Log("case: success")
	obj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{ID: models.ObjID("objectID"), Version: models.ObjVersion(*params.Version)},
		},
	}
	obj.AccountID = "account1"
	obj.BoundClusterID = "cl1"
	obj.ConsistencyGroupID = "cg1"
	obj.ServicePlanID = "planId"
	obj.SizeBytes = swag.Int64(1024)
	obj.VolumeSeriesState = com.VolStateBound
	vsrLParam := volume_series_request.VolumeSeriesRequestListParams{
		VolumeSeriesID: swag.String(string(obj.Meta.ID)),
		IsTerminated:   swag.Bool(false),
	}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mds := mock.NewMockDataStore(mockCtrl)
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(obj.AccountID)).Return(aObj, nil)
	mds.EXPECT().OpsAccount().Return(oA)
	oV := mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oV.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	mds.EXPECT().OpsVolumeSeries().Return(oV).MinTimes(1)
	hc.DS = mds
	var ret middleware.Responder
	cntRLock := hc.cntRLock
	cntRUnlock := hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesUpdate(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.VolumeSeriesUpdateOK)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Equal(obj, mO.Payload)
	assert.Equal("account1", ai.AccountID)
	assert.Equal("aName", ai.AccountName)
	assert.Len(evM.InSSProps, 4)
	assert.EqualValues("account1", evM.InSSProps["accountId"])
	assert.EqualValues("cl1", evM.InSSProps["clusterId"])
	assert.EqualValues("cg1", evM.InSSProps["consistencyGroupId"])
	assert.EqualValues("planId", evM.InSSProps["servicePlanId"])
	assert.Equal(obj, evM.InACScope)
	tl.Flush()

	// success with volumeSeries query, no version, validation adds version
	var oN *mock.MockNodeOps
	aiVO := &auth.Info{
		AccountID:       "account1",
		TenantAccountID: "tid1",
		RoleObj: &models.Role{
			RoleMutable: models.RoleMutable{
				Capabilities: map[string]bool{centrald.VolumeSeriesOwnerCap: true},
			},
		},
	}
	var oObj *models.VolumeSeries
	for tc := 1; tc <= 7; tc++ {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		mds = mock.NewMockDataStore(mockCtrl)
		fa.Events = []*fal.Args{}
		fa.Posts = []*fal.Args{}
		obj.Meta.Version = 3
		obj.VolumeSeriesState = com.VolStateUnbound
		obj.BoundClusterID = ""
		obj.ConfiguredNodeID = ""
		obj.ConsistencyGroupID = "cg1"
		obj.Mounts = nil
		testutils.Clone(obj, &oObj)
		params.Payload.ConsistencyGroupID = "cg1"
		params.Payload.Mounts = []*models.Mount{
			{SnapIdentifier: "1", MountedNodeID: "node1", MountMode: "READ_WRITE", MountState: "MOUNTING"},
			{SnapIdentifier: "HEAD", MountedNodeID: "node1", MountedNodeDevice: "dev", MountMode: "READ_WRITE", MountState: "MOUNTED"},
		}
		params.Version = nil
		uaAddBoundCspDomainID := false
		params.Payload.BoundClusterID = ""
		params.Payload.BoundCspDomainID = ""
		switch tc {
		case 1:
			t.Log("case: success, no version with configuredNodeId, mounts, DELETING, empty cgID")
			params.Append = []string{}
			params.Set = []string{"mounts", "name", "configuredNodeId", "consistencyGroupId", "sizeBytes", "spaAdditionalBytes", "volumeSeriesState"}
			params.Payload.ConfiguredNodeID = "node1"
			params.Payload.ConsistencyGroupID = ""
			params.Payload.VolumeSeriesState = com.VolStateDeleting
			params.Payload.SizeBytes = obj.SizeBytes                   // no actual change
			params.Payload.SpaAdditionalBytes = obj.SpaAdditionalBytes // no actual change
			oObj.BoundClusterID = "cl1"
			oObj.ConsistencyGroupID = "cg1"
			oObj.Mounts = params.Payload.Mounts // HEAD already MOUNTED on the node, fetch skipped
			obj.BoundClusterID = "cl1"
			obj.ConsistencyGroupID = ""
			obj.ConfiguredNodeID = "node1"
			obj.Mounts = oObj.Mounts
			oN = mock.NewMockNodeOps(mockCtrl)
			oN.EXPECT().Fetch(ctx, string(nObj.Meta.ID)).Return(nObj, nil)
			mds.EXPECT().OpsNode().Return(oN)
		case 2:
			// success, no version, validation adds version
			t.Log("case: success, no version with empty boundClusterId")
			params.Payload.ConsistencyGroupID = obj.ConsistencyGroupID // no actual change
			params.Payload.ServicePlanID = obj.ServicePlanID           // no actual change
			params.Set = []string{"name", "boundClusterId", "consistencyGroupId", "servicePlanId"}
		case 3:
			t.Log("case: success with boundClusterId and consistencyGroupId")
			params.Payload.BoundClusterID = "cl1"
			uaAddBoundCspDomainID = true
			params.Payload.ConsistencyGroupID = obj.ConsistencyGroupID
			params.Set = []string{"name", "boundClusterId", "consistencyGroupId"}
			oObj.ConsistencyGroupID = "cgOld"
			obj.BoundClusterID = "cl1"
			oObj.VolumeSeriesState = com.VolStateBound
			oC := mock.NewMockClusterOps(mockCtrl)
			oC.EXPECT().Fetch(ctx, string(params.Payload.BoundClusterID)).Return(clObj, nil)
			mds.EXPECT().OpsCluster().Return(oC)
			oCG := mock.NewMockConsistencyGroupOps(mockCtrl)
			oCG.EXPECT().Fetch(ctx, string(params.Payload.ConsistencyGroupID)).Return(cgObj, nil)
			mds.EXPECT().OpsConsistencyGroup().Return(oCG)
			oVR := mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			oVR.EXPECT().List(ctx, vsrLParam).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
		case 4:
			t.Log("case: success, no version with all owner auth fields")
			oObj.ConsistencyGroupID = "cgOld"
			params.HTTPRequest = requestWithAuthContext(aiVO)
			ctx = params.HTTPRequest.Context()
			params.Append = []string{}
			params.Set = []string{"name", "description", "tags", "consistencyGroupId"} // all that are allowed with VolumeSeriesOwnerCap
			oCG := mock.NewMockConsistencyGroupOps(mockCtrl)
			oCG.EXPECT().Fetch(ctx, string(params.Payload.ConsistencyGroupID)).Return(cgObj, nil)
			mds.EXPECT().OpsConsistencyGroup().Return(oCG)
			oVR := mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			oVR.EXPECT().List(ctx, vsrLParam).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
		case 5:
			t.Log("case: success, boundClusterId unchanged, unmount ignored, event generated, IOPS increase")
			params.Set = []string{"boundClusterId", "mounts", "servicePlanId", "sizeBytes", "spaAdditionalBytes"}
			params.Payload.BoundClusterID = oObj.BoundClusterID
			params.Payload.Mounts = params.Payload.Mounts[:1]
			oObj.Mounts = params.Payload.Mounts
			oObj.ServicePlanID = "oldSP"
			obj.Mounts = params.Payload.Mounts
			params.Payload.SizeBytes = swag.Int64(10240)
			params.Payload.SpaAdditionalBytes = swag.Int64(1024)
			oSP := mock.NewMockServicePlanOps(mockCtrl)
			oSP.EXPECT().Fetch(ctx, string(params.Payload.ServicePlanID)).Return(spObj, nil)
			mds.EXPECT().OpsServicePlan().Return(oSP)
		case 6:
			t.Log("case: success, mounts removes HEAD")
			params.Set = []string{"spaAdditionalBytes"}
			params.Remove = []string{"mounts"}
			params.Payload.SpaAdditionalBytes = swag.Int64(1024)
			oObj.Mounts = params.Payload.Mounts
			oObj.SpaAdditionalBytes = swag.Int64(2048)
			obj.Mounts = nil
		case 7:
			t.Log("case: clear boundClusterId")
			assert.Equal("UNBOUND", obj.VolumeSeriesState)
			oObj.BoundClusterID = "cl1"
			params.Set = []string{"boundClusterId"}
			params.Payload.BoundClusterID = ""
			uaAddBoundCspDomainID = true
		default:
			assert.False(true)
		}
		uP[centrald.UpdateAppend] = params.Append
		testutils.Clone(params.Set, &uP[centrald.UpdateSet])
		if uaAddBoundCspDomainID {
			uP[centrald.UpdateSet] = append(uP[centrald.UpdateSet], "boundCspDomainId")
		}
		uP[centrald.UpdateRemove] = params.Remove
		assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
		assert.Nil(err)
		assert.NotNil(ua)
		uaM = updateArgsMatcher(t, ua).AddsVersion(3).Matcher()
		oV = mock.NewMockVolumeSeriesOps(mockCtrl)
		oV.EXPECT().Fetch(ctx, params.ID).Return(oObj, nil)
		oV.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
		mds.EXPECT().OpsVolumeSeries().Return(oV).MinTimes(1)
		hc.DS = mds
		cntRLock = hc.cntRLock
		cntRUnlock = hc.cntRUnlock
		assert.NotPanics(func() { ret = hc.volumeSeriesUpdate(params) })
		assert.NotNil(ret)
		mO, ok = ret.(*ops.VolumeSeriesUpdateOK)
		assert.True(ok)
		assert.Equal(cntRLock+1, hc.cntRLock)
		assert.Equal(cntRUnlock+1, hc.cntRUnlock)
		assert.Equal(obj, mO.Payload)
		if tc == 1 {
			assert.Len(evM.InSSProps, 4)
			assert.EqualValues("cl1", evM.InSSProps["clusterId"])
			assert.EqualValues("node1", evM.InSSProps["nodeId"])
			assert.Empty(fa.Events)
			assert.Empty(fa.Posts)
		} else if tc == 3 {
			assert.Len(evM.InSSProps, 4)
			assert.EqualValues("cl1", evM.InSSProps["clusterId"])
			assert.EqualValues("cg1", evM.InSSProps["consistencyGroupId"])
			exp1 := &fal.Args{AI: ai, Action: centrald.VolumeSeriesUpdateAction, ObjID: models.ObjID(params.ID), Name: obj.Name, RefID: "cl1", Message: "Bound to cluster[bunch]"}
			exp2 := &fal.Args{AI: ai, Action: centrald.VolumeSeriesUpdateAction, ObjID: models.ObjID(params.ID), Name: obj.Name, RefID: "cg1", Message: "Updated consistency group [con]"}
			assert.Equal([]*fal.Args{exp1, exp2}, fa.Posts)
			assert.Empty(fa.Events)
		} else {
			assert.Len(evM.InSSProps, 3)
			assert.EqualValues("cg1", evM.InSSProps["consistencyGroupId"])
			if tc == 4 {
				exp := &fal.Args{AI: aiVO, Action: centrald.VolumeSeriesUpdateAction, ObjID: models.ObjID(params.ID), Name: obj.Name, RefID: "cg1", Message: "Updated consistency group [con]"}
				assert.Equal([]*fal.Args{exp}, fa.Posts)
				assert.Empty(fa.Events)
			} else if tc == 5 {
				exp := &fal.Args{AI: ai, Action: centrald.VolumeSeriesUpdateAction, ObjID: models.ObjID(params.ID), Name: obj.Name, Message: "Updated service plan [servicePlan], sizeBytes, max IOPS increase"}
				assert.Equal([]*fal.Args{exp}, fa.Events)
				assert.Empty(fa.Posts)
			} else if tc == 6 {
				exp := &fal.Args{AI: ai, Action: centrald.VolumeSeriesUpdateAction, ObjID: models.ObjID(params.ID), Name: obj.Name, Message: "Updated unmounted HEAD"}
				assert.Equal([]*fal.Args{exp}, fa.Posts)
				exp.Message = "Updated max IOPS decrease"
				assert.Equal([]*fal.Args{exp}, fa.Events)
			} else {
				assert.Empty(fa.Events)
				assert.Empty(fa.Posts)
			}
		}
		if uaAddBoundCspDomainID {
			if params.Payload.BoundClusterID != "" {
				assert.Equal(clObj.CspDomainID, params.Payload.BoundCspDomainID)
			} else {
				assert.EqualValues("", params.Payload.BoundCspDomainID)
			}
		} else {
			assert.EqualValues("", params.Payload.BoundCspDomainID)
		}
		assert.EqualValues("account1", evM.InSSProps["accountId"])
		assert.EqualValues("planId", evM.InSSProps["servicePlanId"])
		assert.Equal(obj, evM.InACScope)
		params.HTTPRequest = requestWithAuthContext(ai)
		ctx = params.HTTPRequest.Context()
	}

	// Update failed after successful verification paths
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: Update failed")
	params.Payload.BoundClusterID = "cl1"
	params.Payload.ConsistencyGroupID = "cg1"
	params.Payload.RootStorageID = "st1"
	params.Payload.ServicePlanID = "planId"
	params.Payload.Mounts = []*models.Mount{
		{SnapIdentifier: "HEAD", MountedNodeID: "node1", MountedNodeDevice: "/dev/abc", MountMode: "READ_WRITE", MountState: "MOUNTED"},
	}
	params.Payload.CapacityAllocations = map[string]models.CapacityAllocation{
		"prov1": {ConsumedBytes: swag.Int64(1), ReservedBytes: swag.Int64(101010)},
	}
	params.Payload.StorageParcels = map[string]models.ParcelAllocation{
		"st1": {SizeBytes: swag.Int64(22)},
	}
	params.Payload.VolumeSeriesState = com.VolStateInUse
	params.Append = []string{"mounts", "capacityAllocations", "storageParcels"}
	params.Set = []string{"boundClusterId", "consistencyGroupId", "rootStorageId", "servicePlanId", "volumeSeriesState"}
	params.Remove = []string{}
	params.Version = swag.Int32(8)
	obj.Meta.Version = 8
	testutils.Clone(obj, &oObj)
	oObj.ServicePlanID = "oldPlan"
	oObj.ConsistencyGroupID = "cgOld"
	uP[centrald.UpdateAppend] = params.Append
	testutils.Clone(params.Set, &uP[centrald.UpdateSet])
	uP[centrald.UpdateSet] = append(uP[centrald.UpdateSet], "boundCspDomainId")
	uP[centrald.UpdateRemove] = params.Remove
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).Matcher()
	oC := mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, string(params.Payload.BoundClusterID)).Return(clObj, nil)
	oS := mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Fetch(ctx, string(params.Payload.RootStorageID)).Return(stObj, nil).Times(2)
	oSP := mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Fetch(ctx, string(params.Payload.ServicePlanID)).Return(spObj, nil)
	oCG := mock.NewMockConsistencyGroupOps(mockCtrl)
	oCG.EXPECT().Fetch(ctx, string(params.Payload.ConsistencyGroupID)).Return(cgObj, nil)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, string(nObj.Meta.ID)).Return(nObj, nil)
	oP := mock.NewMockPoolOps(mockCtrl)
	oP.EXPECT().Fetch(ctx, string(pObj.Meta.ID)).Return(pObj, nil)
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Fetch(ctx, params.ID).Return(oObj, nil)
	oV.EXPECT().Update(ctx, uaM, params.Payload).Return(nil, centrald.ErrorDbError)
	oVR := mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().List(ctx, vsrLParam).Return([]*models.VolumeSeriesRequest{}, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oC)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	mds.EXPECT().OpsNode().Return(oN)
	mds.EXPECT().OpsStorage().Return(oS).Times(2)
	mds.EXPECT().OpsPool().Return(oP)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	mds.EXPECT().OpsVolumeSeries().Return(oV).Times(2)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesUpdate(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.VolumeSeriesUpdateDefault)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: auditLog not ready")
	params.Append = []string{}
	params.Set = []string{"name"}
	fa.ReadyRet = centrald.ErrorDbError
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesUpdate(params) })
	mD, ok = ret.(*ops.VolumeSeriesUpdateDefault)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.volumeSeriesUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.VolumeSeriesUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	// auth failures
	ai.RoleObj = &models.Role{}
	ai.AccountID = string(obj.AccountID)
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	fa.Events = []*fal.Args{}
	for k := range nMap {
		if k == "boundCspDomainId" {
			continue
		}
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		t.Log("case: auth failure", k)
		fa.Posts = []*fal.Args{}
		params.Set = []string{k}
		mds = mock.NewMockDataStore(mockCtrl)
		exp := &fal.Args{AI: ai, Action: centrald.VolumeSeriesUpdateAction, ObjID: models.ObjID(params.ID), Name: obj.Name, Err: true, Message: "Update unauthorized"}
		if util.Contains([]string{"consistencyGroupId", "description", "name", "tags"}, k) {
			params.HTTPRequest = requestWithAuthContext(ai)
		} else {
			params.HTTPRequest = requestWithAuthContext(aiVO)
			exp.AI = aiVO
		}
		ctx = params.HTTPRequest.Context()
		oV = mock.NewMockVolumeSeriesOps(mockCtrl)
		oV.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
		mds.EXPECT().OpsVolumeSeries().Return(oV)
		hc.DS = mds
		assert.NotPanics(func() { ret = hc.volumeSeriesUpdate(params) })
		assert.NotNil(ret)
		mD, ok = ret.(*ops.VolumeSeriesUpdateDefault)
		assert.True(ok)
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)
		assert.Equal([]*fal.Args{exp}, fa.Posts)
	}
	ai.RoleObj = nil
	fa.Posts = []*fal.Args{}

	// no changes requested
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: no change")
	params.Append = []string{}
	params.Set = []string{}
	mds = mock.NewMockDataStore(mockCtrl)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.VolumeSeriesUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)

	// missing invalid updates
	obj.AccountID = "account1"
	obj.RootStorageID = "st1"
	obj.StorageParcels = make(map[string]models.ParcelAllocation, 0)
	obj.StorageParcels["st1"] = models.ParcelAllocation{SizeBytes: swag.Int64(0)}
	var verP *int32
	for tc := 1; tc <= 41; tc++ {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		mds = mock.NewMockDataStore(mockCtrl)
		oA = mock.NewMockAccountOps(mockCtrl)
		oC = mock.NewMockClusterOps(mockCtrl)
		oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
		oN = mock.NewMockNodeOps(mockCtrl)
		oP = mock.NewMockPoolOps(mockCtrl)
		oS = mock.NewMockStorageOps(mockCtrl)
		oSP = mock.NewMockServicePlanOps(mockCtrl)
		oV = mock.NewMockVolumeSeriesOps(mockCtrl)
		oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
		ai.AccountID = "account1"
		ai.RoleObj = nil
		var uObj *models.VolumeSeries
		testutils.Clone(obj, &uObj)
		spObj.State = centrald.PublishedState
		var payload *models.VolumeSeriesMutable
		testutils.Clone(params.Payload, &payload)
		append := []string{}
		set := []string{"name"}
		remove := []string{}
		verP = nil
		expectedErr := centrald.ErrorUpdateInvalidRequest
		switch tc {
		case 1:
			t.Log("case: empty name")
			payload.Name = ""
		case 2:
			t.Log("case: servicePlanId fetch fails")
			set[0] = "servicePlanId"
			payload.ServicePlanID = "plan"
			expectedErr = centrald.ErrorDbError
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oSP.EXPECT().Fetch(ctx, string(payload.ServicePlanID)).Return(nil, expectedErr)
			mds.EXPECT().OpsServicePlan().Return(oSP)
		case 3:
			t.Log("case: servicePlanId not found")
			set = []string{"servicePlanId"}
			payload.ServicePlanID = "plan"
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oSP.EXPECT().Fetch(ctx, string(payload.ServicePlanID)).Return(nil, centrald.ErrorNotFound)
			mds.EXPECT().OpsServicePlan().Return(oSP)
		case 4:
			t.Log("case: servicePlanId", centrald.RetiredState)
			set[0] = "servicePlanId"
			payload.ServicePlanID = "plan"
			spObj.State = centrald.RetiredState
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oSP.EXPECT().Fetch(ctx, string(payload.ServicePlanID)).Return(spObj, nil)
			mds.EXPECT().OpsServicePlan().Return(oSP)
		case 5:
			t.Log("case: ServicePlan is not accessible to Account")
			set[0] = "servicePlanId"
			payload.ServicePlanID = "plan"
			uObj.AccountID = "another"
			expectedErr = &centrald.Error{M: "servicePlan", C: 403}
			oSP.EXPECT().Fetch(ctx, string(payload.ServicePlanID)).Return(spObj, nil)
			mds.EXPECT().OpsServicePlan().Return(oSP)
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
		case 6:
			t.Log("case: BoundClusterID fetch fails")
			set = []string{"boundClusterId", "volumeSeriesState"}
			payload.BoundClusterID = "cl1"
			payload.VolumeSeriesState = com.VolStateBound
			expectedErr = centrald.ErrorDbError
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oC.EXPECT().Fetch(ctx, string(payload.BoundClusterID)).Return(nil, expectedErr)
			mds.EXPECT().OpsCluster().Return(oC)
		case 7:
			t.Log("case: BoundClusterID not found")
			set = []string{"boundClusterId", "volumeSeriesState"}
			payload.BoundClusterID = "cl1"
			payload.VolumeSeriesState = com.VolStateBound
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oC.EXPECT().Fetch(ctx, string(payload.BoundClusterID)).Return(nil, centrald.ErrorNotFound)
			mds.EXPECT().OpsCluster().Return(oC)
		case 8:
			t.Log("case: BoundClusterID and unbound")
			set[0] = "boundClusterId"
			payload.BoundClusterID = "cl1"
			uObj.VolumeSeriesState = com.VolStateUnbound
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
		case 9:
			t.Log("case: empty BoundClusterID and bound")
			set[0] = "boundClusterId"
			payload.BoundClusterID = ""
			uObj.BoundClusterID = "cl1"
			uObj.VolumeSeriesState = com.VolStateBound
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
		case 10:
			t.Log("case: ConfiguredNodeID fetch fails")
			set[0] = "configuredNodeId"
			payload.ConfiguredNodeID = "node1"
			expectedErr = centrald.ErrorDbError
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oN.EXPECT().Fetch(ctx, "node1").Return(nil, expectedErr)
			mds.EXPECT().OpsNode().Return(oN)
		case 11:
			t.Log("case: ConfiguredNodeID not found")
			set[0] = "configuredNodeId"
			payload.ConfiguredNodeID = "node1"
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oN.EXPECT().Fetch(ctx, "node1").Return(nil, centrald.ErrorNotFound)
			mds.EXPECT().OpsNode().Return(oN)
		case 12:
			t.Log("case: ConfiguredNodeID wrong for BoundClusterID")
			set[0] = "configuredNodeId"
			payload.ConfiguredNodeID = "node1"
			uObj.BoundClusterID = "other"
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oN.EXPECT().Fetch(ctx, "node1").Return(nObj, nil)
			mds.EXPECT().OpsNode().Return(oN)
		case 13:
			t.Log("case: RootStorageID fetch fails")
			set[0] = "rootStorageId"
			payload.RootStorageID = "st1"
			expectedErr = centrald.ErrorDbError
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oS.EXPECT().Fetch(ctx, string(payload.RootStorageID)).Return(nil, expectedErr)
			mds.EXPECT().OpsStorage().Return(oS)
		case 14:
			t.Log("case: RootStorageID not found")
			set[0] = "rootStorageId"
			payload.RootStorageID = "st1"
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oS.EXPECT().Fetch(ctx, string(payload.RootStorageID)).Return(nil, centrald.ErrorNotFound)
			mds.EXPECT().OpsStorage().Return(oS)
		case 15:
			t.Log("case: RootStorageID is not accessible to Account")
			set[0] = "rootStorageId"
			payload.RootStorageID = "st1"
			uObj.AccountID = "another"
			expectedErr = &centrald.Error{M: "storage.*not available", C: 403}
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oS.EXPECT().Fetch(ctx, string(payload.RootStorageID)).Return(stObj, nil)
			mds.EXPECT().OpsStorage().Return(oS)
		case 16:
			t.Log("case: CapacityAllocations fetch fails")
			set[0] = "capacityAllocations.prov1"
			payload.CapacityAllocations = map[string]models.CapacityAllocation{
				"prov1": {ConsumedBytes: swag.Int64(1), ReservedBytes: swag.Int64(1)},
			}
			expectedErr = centrald.ErrorDbError
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oP.EXPECT().Fetch(ctx, "prov1").Return(nil, expectedErr)
			mds.EXPECT().OpsPool().Return(oP)
		case 17:
			t.Log("case: CapacityAllocations not found")
			set[0] = "capacityAllocations"
			payload.CapacityAllocations = map[string]models.CapacityAllocation{
				"prov1": {ConsumedBytes: swag.Int64(1), ReservedBytes: swag.Int64(1)},
			}
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oP.EXPECT().Fetch(ctx, "prov1").Return(nil, centrald.ErrorNotFound)
			mds.EXPECT().OpsPool().Return(oP)
		case 18:
			t.Log("case: CapacityAllocations is not accessible to Account")
			set[0] = "capacityAllocations"
			payload.CapacityAllocations = map[string]models.CapacityAllocation{
				"prov1": {ConsumedBytes: swag.Int64(1), ReservedBytes: swag.Int64(1)},
			}
			uObj.AccountID = "another"
			expectedErr = &centrald.Error{M: "pool.*not available", C: 403}
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oP.EXPECT().Fetch(ctx, "prov1").Return(pObj, nil)
			mds.EXPECT().OpsPool().Return(oP)
		case 19:
			t.Log("case: StorageParcels fetch fails")
			set[0] = "storageParcels.st1"
			payload.StorageParcels = map[string]models.ParcelAllocation{"st1": {SizeBytes: swag.Int64(1)}}
			expectedErr = centrald.ErrorDbError
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oS.EXPECT().Fetch(ctx, "st1").Return(nil, expectedErr)
			mds.EXPECT().OpsStorage().Return(oS)
		case 20:
			t.Log("case: StorageParcels not found")
			set[0] = "storageParcels"
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			payload.StorageParcels = map[string]models.ParcelAllocation{"st1": {SizeBytes: swag.Int64(1)}}
			oS.EXPECT().Fetch(ctx, "st1").Return(nil, centrald.ErrorNotFound)
			mds.EXPECT().OpsStorage().Return(oS)
		case 21:
			t.Log("case: StorageParcels is not accessible to Account")
			set[0] = "storageParcels"
			payload.StorageParcels = map[string]models.ParcelAllocation{"st1": {SizeBytes: swag.Int64(1)}}
			uObj.AccountID = "another"
			expectedErr = &centrald.Error{M: "storage.*not available", C: 403}
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oS.EXPECT().Fetch(ctx, "st1").Return(stObj, nil)
			mds.EXPECT().OpsStorage().Return(oS)
		case 22:
			t.Log("case: fetch failure")
			set[0] = "servicePlanId"
			payload.ServicePlanID = "plan"
			expectedErr = centrald.ErrorNotFound
			oV.EXPECT().Fetch(ctx, params.ID).Return(nil, expectedErr)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
		case 23:
			t.Log("case: invalid mounts.snapIdentifier")
			set[0] = "mounts"
			payload.Mounts = []*models.Mount{{SnapIdentifier: "2"}}
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
		case 24:
			t.Log("case: invalid mountMode")
			set[0] = "mounts"
			payload.Mounts = []*models.Mount{{SnapIdentifier: "1", MountMode: "quantum"}}
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
		case 25:
			t.Log("case: invalid mountState")
			set[0] = "mounts"
			payload.Mounts = []*models.Mount{{SnapIdentifier: "1", MountMode: "READ_ONLY", MountState: "Mountain"}}
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
		case 26:
			t.Log("case: invalid mountedNodeDevice")
			set[0] = "mounts"
			payload.Mounts = []*models.Mount{{SnapIdentifier: "1", MountMode: "READ_ONLY", MountState: "MOUNTED"}}
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
		case 27:
			t.Log("case: mountedNodeId fetch fails")
			set[0] = "mounts"
			payload.Mounts = []*models.Mount{{SnapIdentifier: "1", MountMode: "READ_ONLY", MountState: "MOUNTING", MountedNodeID: "wrong"}}
			expectedErr = centrald.ErrorDbError
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oN.EXPECT().Fetch(ctx, "wrong").Return(nil, expectedErr)
			mds.EXPECT().OpsNode().Return(oN)
		case 28:
			t.Log("case: invalid mountedNodeId")
			set[0] = "mounts.0"
			payload.Mounts = []*models.Mount{{SnapIdentifier: "1", MountMode: "READ_ONLY", MountState: "MOUNTING", MountedNodeID: "wrong"}}
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oN.EXPECT().Fetch(ctx, "wrong").Return(nil, centrald.ErrorNotFound)
			mds.EXPECT().OpsNode().Return(oN)
		case 29:
			t.Log("case: wrong mountedNodeId for cluster")
			set[0] = "mounts.0"
			uObj.BoundClusterID = "other1"
			payload.Mounts = []*models.Mount{{SnapIdentifier: "1", MountMode: "READ_ONLY", MountState: "MOUNTING", MountedNodeID: "node1"}}
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oN.EXPECT().Fetch(ctx, "node1").Return(nObj, nil)
			mds.EXPECT().OpsNode().Return(oN)
		case 30:
			t.Log("case: remove rootStorageId from storageParcels")
			payload.StorageParcels = map[string]models.ParcelAllocation{"st1": {SizeBytes: swag.Int64(1)}}
			remove = []string{"storageParcels"}
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
		case 31:
			t.Log("case: rootStorageId not in storageParcels")
			payload.RootStorageID = "st2"
			set[0] = "rootStorageId"
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oS.EXPECT().Fetch(ctx, "st2").Return(stObj, nil)
			mds.EXPECT().OpsStorage().Return(oS)
		case 32:
			t.Log("case: invalid volumeSeriesState")
			payload.VolumeSeriesState = "foo"
			set[0] = "volumeSeriesState"
		case 33:
			t.Log("case: consistencyGroupId fetch fails")
			set = []string{"consistencyGroupId"}
			payload.ConsistencyGroupID = "cg1"
			uObj.ConsistencyGroupID = "cgOld"
			expectedErr = centrald.ErrorDbError
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oCG.EXPECT().Fetch(ctx, string(payload.ConsistencyGroupID)).Return(nil, expectedErr)
			mds.EXPECT().OpsConsistencyGroup().Return(oCG)
		case 34:
			t.Log("case: consistencyGroupId not found")
			set = []string{"consistencyGroupId"}
			payload.ConsistencyGroupID = "cg1"
			uObj.ConsistencyGroupID = "cgOld"
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oCG.EXPECT().Fetch(ctx, string(payload.ConsistencyGroupID)).Return(nil, centrald.ErrorNotFound)
			mds.EXPECT().OpsConsistencyGroup().Return(oCG)
		case 35:
			t.Log("case: consistencyGroup is not accessible to Account")
			set = []string{"consistencyGroupId"}
			payload.ConsistencyGroupID = "cg1"
			uObj.AccountID = "another"
			uObj.ConsistencyGroupID = "cgOld"
			expectedErr = &centrald.Error{M: "consistencyGroup", C: 403}
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oCG.EXPECT().Fetch(ctx, string(payload.ConsistencyGroupID)).Return(cgObj, nil)
			mds.EXPECT().OpsConsistencyGroup().Return(oCG)
		case 36:
			t.Log("case: empty consistencyGroup")
			set = []string{"consistencyGroupId"}
			payload.ConsistencyGroupID = ""
			ai.RoleObj = aiVO.RoleObj
			expectedErr = &centrald.Error{M: ".*non-empty consistencyGroup", C: 400}
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
		case 37:
			t.Log("case: consistencyGroup check: fail VSR list")
			set = []string{"consistencyGroupId"}
			payload.ConsistencyGroupID = "cg1"
			uObj.ConsistencyGroupID = "cgOld"
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oCG.EXPECT().Fetch(ctx, string(payload.ConsistencyGroupID)).Return(cgObj, nil)
			mds.EXPECT().OpsConsistencyGroup().Return(oCG)
			vsrLParam = volume_series_request.VolumeSeriesRequestListParams{
				VolumeSeriesID: swag.String(string(uObj.Meta.ID)),
				IsTerminated:   swag.Bool(false),
			}
			oVR.EXPECT().List(ctx, vsrLParam).Return(nil, centrald.ErrorDbError)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			expectedErr = centrald.ErrorDbError
		case 38:
			t.Log("case: consistencyGroup check: conflicting VOL_SNAPSHOT_CREATE")
			set = []string{"consistencyGroupId"}
			payload.ConsistencyGroupID = "cg1"
			uObj.ConsistencyGroupID = "cgOld"
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oCG.EXPECT().Fetch(ctx, string(payload.ConsistencyGroupID)).Return(cgObj, nil)
			mds.EXPECT().OpsConsistencyGroup().Return(oCG)
			vsrLParam = volume_series_request.VolumeSeriesRequestListParams{
				VolumeSeriesID: swag.String(string(uObj.Meta.ID)),
				IsTerminated:   swag.Bool(false),
			}
			VSRl := []*models.VolumeSeriesRequest{&models.VolumeSeriesRequest{}}
			VSRl[0].Meta = &models.ObjMeta{ID: "vol-snap-create"}
			VSRl[0].RequestedOperations = []string{"VOL_SNAPSHOT_CREATE"}
			oVR.EXPECT().List(ctx, vsrLParam).Return(VSRl, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			expectedErr = centrald.ErrorRequestInConflict
		case 39:
			t.Log("case: invalid version")
			verP = swag.Int32(1)
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			expectedErr = centrald.ErrorIDVerNotFound
		case 40:
			t.Log("case: Account fetch failure")
			ai.AccountID = ""
			expectedErr = centrald.ErrorDbError
			oV.EXPECT().Fetch(ctx, params.ID).Return(uObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oA.EXPECT().Fetch(ctx, "account1").Return(nil, expectedErr)
			mds.EXPECT().OpsAccount().Return(oA)
		case 41:
			t.Log("case: Explicitly set boundCspDomainId")
			set = []string{"boundCspDomainId"}
		default:
			assert.False(true)
		}
		hc.DS = mds
		assert.NotPanics(func() {
			req := requestWithAuthContext(ai)
			ctx = req.Context()
			ret = hc.volumeSeriesUpdate(ops.VolumeSeriesUpdateParams{HTTPRequest: req, ID: params.ID, Payload: payload, Set: set, Append: append, Remove: remove, Version: verP})
		})
		assert.NotNil(ret)
		mD, ok = ret.(*ops.VolumeSeriesUpdateDefault)
		assert.True(ok)
		assert.Equal(expectedErr.C, int(mD.Payload.Code))
		assert.Regexp("^"+expectedErr.M, *mD.Payload.Message)
		tl.Flush()
	}

	// no payload
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: no payload")
	mds = mock.NewMockDataStore(mockCtrl)
	params.Set = []string{"name"}
	params.Payload = nil
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.VolumeSeriesUpdateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
		assert.Equal(centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	}
	assert.Empty(fa.Events)
	assert.Empty(fa.Posts)
}

func TestVolumeSeriesPVSpecFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}

	params := ops.VolumeSeriesPVSpecFetchParams{
		HTTPRequest:    requestWithAuthContext(ai),
		ID:             "objectID",
		FsType:         swag.String("ext4"),
		ClusterVersion: swag.String("v1"),
	}
	ctx := params.HTTPRequest.Context()
	vsObj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{Meta: &models.ObjMeta{ID: "id1"}},
		VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
			AccountID:       "aid1",
			TenantAccountID: "tid1",
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				BoundClusterID:    "clusterID",
				VolumeSeriesState: "BOUND",
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				SizeBytes: swag.Int64(10737418240), //10GiB
			},
		},
	}
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "clusterID",
			},
		},
		ClusterCreateOnce: models.ClusterCreateOnce{
			ClusterType: "kubernetes",
		},
		ClusterMutable: models.ClusterMutable{
			ClusterMutableAllOf0: models.ClusterMutableAllOf0{
				ClusterVersion: "1.9",
			},
		},
	}
	sysObj := &models.System{
		SystemAllOf0: models.SystemAllOf0{
			Meta: &models.ObjMeta{ID: "systemID"},
		},
	}
	args := &cluster.PVSpecArgs{
		AccountID:      string(vsObj.AccountID),
		ClusterVersion: swag.StringValue(params.ClusterVersion),
		VolumeID:       string(params.ID),
		SystemID:       string(sysObj.Meta.ID),
		FsType:         swag.StringValue(params.FsType),
		Capacity:       fmt.Sprintf("10Gi"),
	}
	pv := &cluster.PVSpec{
		Format: "yaml",
		PvSpec: "data",
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oVS := mock.NewMockVolumeSeriesOps(mockCtrl)
	oVS.EXPECT().Fetch(ctx, params.ID).Return(vsObj, nil)
	oC := mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, string(vsObj.BoundClusterID)).Return(clObj, nil)
	oSys := mock.NewMockSystemOps(mockCtrl)
	oSys.EXPECT().Fetch().Return(sysObj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	mds.EXPECT().OpsCluster().Return(oC)
	mds.EXPECT().OpsSystem().Return(oSys)
	cl := mockcluster.NewMockClient(mockCtrl)
	hc.clusterClientMap = make(map[string]cluster.Client)
	hc.clusterClientMap[clObj.ClusterType] = cl
	cl.EXPECT().GetPVSpec(params.HTTPRequest.Context(), args).Return(pv, nil)
	t.Log("case: pvSpec fetch success")
	tl.Flush()
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.volumeSeriesPVSpecFetch(params) })
	assert.NotNil(ret)
	mR, ok := ret.(*ops.VolumeSeriesPVSpecFetchOK)
	assert.True(ok)
	assert.Equal(pv.Format, mR.Payload.Format)
	assert.Equal(pv.PvSpec, mR.Payload.PvSpec)

	// states  vsObj.VolumeSeriesState = "BOUND"
	goodStates := []string{"BOUND", "IN_USE", "CONFIGURED", "PROVISIONED"}
	for _, state := range goodStates {
		vsObj.VolumeSeriesState = state
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		oVS := mock.NewMockVolumeSeriesOps(mockCtrl)
		oVS.EXPECT().Fetch(ctx, params.ID).Return(vsObj, nil)
		oC := mock.NewMockClusterOps(mockCtrl)
		oC.EXPECT().Fetch(ctx, string(vsObj.BoundClusterID)).Return(clObj, nil)
		oSys := mock.NewMockSystemOps(mockCtrl)
		oSys.EXPECT().Fetch().Return(sysObj, nil)
		mds := mock.NewMockDataStore(mockCtrl)
		mds.EXPECT().OpsVolumeSeries().Return(oVS)
		mds.EXPECT().OpsCluster().Return(oC)
		mds.EXPECT().OpsSystem().Return(oSys)
		cl := mockcluster.NewMockClient(mockCtrl)
		hc.clusterClientMap = make(map[string]cluster.Client)
		hc.clusterClientMap[clObj.ClusterType] = cl
		cl.EXPECT().GetPVSpec(params.HTTPRequest.Context(), args).Return(pv, nil)
		t.Logf("case: pv spec fetch success with state %s", state)
		tl.Flush()
		hc.DS = mds
		var ret middleware.Responder
		assert.NotPanics(func() { ret = hc.volumeSeriesPVSpecFetch(params) })
		assert.NotNil(ret)
		mR, ok := ret.(*ops.VolumeSeriesPVSpecFetchOK)
		assert.True(ok)
		assert.Equal(pv.Format, mR.Payload.Format)
		assert.Equal(pv.PvSpec, mR.Payload.PvSpec)
	}

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	params.Capacity = swag.Int64(10737418240)
	params.ClusterVersion = nil
	args.Capacity = fmt.Sprintf("10Gi")
	args.ClusterVersion = "1.9"
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVS.EXPECT().Fetch(ctx, params.ID).Return(vsObj, nil)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, string(vsObj.BoundClusterID)).Return(clObj, nil)
	oSys = mock.NewMockSystemOps(mockCtrl)
	oSys.EXPECT().Fetch().Return(sysObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	mds.EXPECT().OpsCluster().Return(oC)
	mds.EXPECT().OpsSystem().Return(oSys)
	cl = mockcluster.NewMockClient(mockCtrl)
	hc.clusterClientMap = make(map[string]cluster.Client)
	hc.clusterClientMap[clObj.ClusterType] = cl
	cl.EXPECT().GetPVSpec(params.HTTPRequest.Context(), args).Return(nil, fmt.Errorf("GetPVSpec error"))
	t.Log("case: pvSpec fetch failure")
	tl.Flush()
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesPVSpecFetch(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.VolumeSeriesPVSpecFetchDefault)
	assert.True(ok)
	assert.Regexp(centrald.ErrorInvalidData.M, *mE.Payload.Message)
	assert.Equal(centrald.ErrorInvalidData.C, int(mE.Payload.Code))

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVS.EXPECT().Fetch(ctx, params.ID).Return(vsObj, nil)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, string(vsObj.BoundClusterID)).Return(clObj, nil)
	oSys = mock.NewMockSystemOps(mockCtrl)
	oSys.EXPECT().Fetch().Return(sysObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	mds.EXPECT().OpsCluster().Return(oC)
	mds.EXPECT().OpsSystem().Return(oSys)
	hc.clusterClientMap = make(map[string]cluster.Client)
	hc.clusterClientMap[clObj.ClusterType] = nil
	t.Log("case: clusterClient lookup failure")
	tl.Flush()
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesPVSpecFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesPVSpecFetchDefault)
	assert.True(ok)
	assert.Regexp("Cluster type .* not found", *mE.Payload.Message)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVS.EXPECT().Fetch(ctx, params.ID).Return(vsObj, nil)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, string(vsObj.BoundClusterID)).Return(clObj, nil)
	oSys = mock.NewMockSystemOps(mockCtrl)
	oSys.EXPECT().Fetch().Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	mds.EXPECT().OpsCluster().Return(oC)
	mds.EXPECT().OpsSystem().Return(oSys)
	t.Log("case: system fetch failure")
	tl.Flush()
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesPVSpecFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesPVSpecFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Regexp(centrald.ErrorDbError.M, *mE.Payload.Message)

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVS.EXPECT().Fetch(ctx, params.ID).Return(vsObj, nil)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, string(vsObj.BoundClusterID)).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	mds.EXPECT().OpsCluster().Return(oC)
	t.Log("case: Cluster fetch failure")
	tl.Flush()
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesPVSpecFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesPVSpecFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Regexp(centrald.ErrorDbError.M, *mE.Payload.Message)

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	vsObj.VolumeSeriesState = "BADSTATE"
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVS.EXPECT().Fetch(ctx, params.ID).Return(vsObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	t.Log("case: VS bad state failure")
	tl.Flush()
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesPVSpecFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesPVSpecFetchDefault)
	assert.True(ok)
	assert.Regexp("Volume Series is not bound to the cluster", *mE.Payload.Message)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mE.Payload.Code))
	vsObj.VolumeSeriesState = "BOUND" // reset

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVS.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	t.Log("case: VS fetch failure")
	tl.Flush()
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesPVSpecFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesPVSpecFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Regexp(centrald.ErrorDbError.M, *mE.Payload.Message)

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	params.FsType = swag.String("badFS")
	t.Log("case: FSType incorrect failure")
	tl.Flush()
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesPVSpecFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesPVSpecFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mE.Payload.Code))
	assert.Regexp("Unsupported file system type .*", *mE.Payload.Message)
	params.FsType = swag.String("ext4") // reset

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.volumeSeriesPVSpecFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesPVSpecFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: unauthorized for volume")
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVS.EXPECT().Fetch(ctx, params.ID).Return(vsObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	hc.DS = mds
	ai.AccountID = "aid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: false}
	assert.NotPanics(func() { ret = hc.volumeSeriesPVSpecFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesPVSpecFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	ai.RoleObj = nil
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: unauthorized for cluster")
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVS.EXPECT().Fetch(ctx, params.ID).Return(vsObj, nil)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, string(vsObj.BoundClusterID)).Return(clObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	mds.EXPECT().OpsCluster().Return(oC)
	hc.DS = mds
	ai.AccountID = "aid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: false, centrald.VolumeSeriesOwnerCap: true}
	assert.NotPanics(func() { ret = hc.volumeSeriesPVSpecFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesPVSpecFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	ai.RoleObj = nil
	tl.Flush()
}
