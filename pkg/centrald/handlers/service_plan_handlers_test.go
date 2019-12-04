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
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/service_plan"
	vs "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	fal "github.com/Nuvoloso/kontroller/pkg/centrald/audit/fake"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestServicePlanFetchFilter(t *testing.T) {
	assert := assert.New(t)
	hc := newHandlerComp()
	obj := &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{Meta: &models.ObjMeta{ID: "spId1"}},
		ServicePlanMutable: models.ServicePlanMutable{
			Accounts: []models.ObjIDMutable{"aid1", "aid2"},
		},
	}
	rObj := &models.Role{}
	rObj.Capabilities = map[string]bool{}
	ai := &auth.Info{}

	t.Log("case: internal user")
	assert.NoError(hc.servicePlanFetchFilter(ai, obj))

	t.Log("case: SystemManagementCap capability")
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.SystemManagementCap: true}
	assert.NoError(hc.servicePlanFetchFilter(ai, obj))

	t.Log("case: CSPDomainManagementCap capability")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	assert.NoError(hc.servicePlanFetchFilter(ai, obj))

	t.Log("case: authorized account")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	ai.AccountID = "aid2"
	assert.NoError(hc.servicePlanFetchFilter(ai, obj))

	t.Log("case: not authorized account")
	ai.AccountID = "aid9"
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.servicePlanFetchFilter(ai, obj))

	t.Log("case: no authorized accounts")
	obj.Accounts = nil
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.servicePlanFetchFilter(ai, obj))
}

func TestServicePlanClone(t *testing.T) {
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
	args := &models.ServicePlanCloneArgs{
		Name: "newServicePlan1",
	}
	params := ops.ServicePlanCloneParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "oldServicePlanID",
		Payload:     args,
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			Meta: &models.ObjMeta{
				ID:      models.ObjID("objectID"),
				Version: 1,
			},
		},
	}
	obj.Name = params.Payload.Name

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oSP := mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Clone(ctx, params).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.servicePlanClone(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.ServicePlanCloneCreated)
	assert.True(ok)
	assert.Len(evM.InSSProps, 1)
	assert.EqualValues("objectID", evM.InSSProps["meta.id"])
	assert.Equal(obj, evM.InACScope)
	exp := &fal.Args{AI: ai, Action: centrald.ServicePlanCloneAction, ObjID: "objectID", Name: models.ObjName(params.Payload.Name), Message: "Created"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	tl.Flush()

	// Clone failed - object name exists
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Clone(ctx, params).Return(nil, centrald.ErrorExists)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	t.Log("case: Clone fails because object exists")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.servicePlanClone(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.ServicePlanCloneDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorExists.M, *mE.Payload.Message)
	tl.Flush()

	// Clone failed - source object not found
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Clone(ctx, params).Return(nil, centrald.ErrorIDVerNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	t.Log("case: Clone fails because sourceID not found")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.servicePlanClone(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ServicePlanCloneDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorIDVerNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorIDVerNotFound.M, *mE.Payload.Message)
	tl.Flush()

	// missing property failure
	params.Payload.Name = ""
	t.Log("case: empty name")
	assert.NotPanics(func() { ret = hc.servicePlanClone(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ServicePlanCloneDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
	tl.Flush()

	// no payload
	params.Payload = nil
	t.Log("case: nil payload")
	assert.NotPanics(func() { ret = hc.servicePlanClone(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ServicePlanCloneDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorMissing.M, *mE.Payload.Message)
	params.Payload = args

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: auditLog not ready")
	fa.ReadyRet = centrald.ErrorDbError
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.servicePlanClone(params) })
	mE, ok = ret.(*ops.ServicePlanCloneDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.servicePlanClone(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ServicePlanCloneDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	t.Log("case: unauthorized")
	ai.RoleObj = &models.Role{}
	fa.Posts = []*fal.Args{}
	assert.NotPanics(func() { ret = hc.servicePlanClone(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ServicePlanCloneDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	exp = &fal.Args{AI: ai, Action: centrald.ServicePlanCloneAction, ObjID: models.ObjID(params.ID), Name: models.ObjName(params.Payload.Name), Err: true, Message: "Clone unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
}

func TestServicePlanDelete(t *testing.T) {
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
	params := ops.ServicePlanDeleteParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("objectID"),
			},
		},
	}
	obj.Name = "servicePlan"
	ssParams := ops.ServicePlanListParams{SourceServicePlanID: &params.ID}
	vsParams := vs.VolumeSeriesListParams{ServicePlanID: &params.ID}
	vrlParams := volume_series_request.VolumeSeriesRequestListParams{ServicePlanID: &params.ID, IsTerminated: swag.Bool(false)}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	t.Log("case: success")
	oSP := mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oSP.EXPECT().BuiltInPlan(params.ID).Return("", false)
	oSP.EXPECT().Count(ctx, ssParams, uint(1)).Return(0, nil)
	oSP.EXPECT().Delete(ctx, params.ID).Return(nil)
	oV := mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Count(ctx, vsParams, uint(1)).Return(0, nil)
	oVR := mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Count(ctx, vrlParams, uint(1)).Return(0, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP).MinTimes(1)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	hc.DS = mds
	var ret middleware.Responder
	cntLock := hc.cntLock
	cntUnlock := hc.cntUnlock
	assert.NotPanics(func() { ret = hc.servicePlanDelete(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.ServicePlanDeleteNoContent)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Empty(evM.InSSProps)
	assert.Equal(obj, evM.InACScope)
	exp := &fal.Args{AI: ai, Action: centrald.ServicePlanDeleteAction, ObjID: "objectID", Name: models.ObjName(obj.Name), Message: "Deleted"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	tl.Flush()

	// delete failure case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: delete failure")
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oSP.EXPECT().BuiltInPlan(params.ID).Return("", false)
	oSP.EXPECT().Count(ctx, ssParams, uint(1)).Return(0, nil)
	oSP.EXPECT().Delete(ctx, params.ID).Return(centrald.ErrorDbError)
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Count(ctx, vsParams, uint(1)).Return(0, nil)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Count(ctx, vrlParams, uint(1)).Return(0, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP).MinTimes(1)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.servicePlanDelete(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.ServicePlanDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: auditLog not ready")
	mds = mock.NewMockDataStore(mockCtrl)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oSP.EXPECT().BuiltInPlan(params.ID).Return("", false)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP).Times(2)
	fa.ReadyRet = centrald.ErrorDbError
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.servicePlanDelete(params) })
	mE, ok = ret.(*ops.ServicePlanDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.servicePlanDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ServicePlanDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	// unauthorized
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: unauthorized")
	fa.Posts = []*fal.Args{}
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{}
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oSP.EXPECT().BuiltInPlan(params.ID).Return("", false)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP).Times(2)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.servicePlanDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ServicePlanDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	exp = &fal.Args{AI: ai, Action: centrald.ServicePlanDeleteAction, ObjID: models.ObjID(params.ID), Name: models.ObjName(obj.Name), Err: true, Message: "Delete unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	ai.RoleObj = nil
	tl.Flush()

	// cascading validation failure cases
	tObj := []string{"service plan", "volume series", "volume series request"}
	for tc := 0; tc < len(tObj)*2; tc++ {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		t.Log("case: " + tObj[tc/2] + []string{" count fails", " exists"}[tc%2])
		mds = mock.NewMockDataStore(mockCtrl)
		oSP = mock.NewMockServicePlanOps(mockCtrl)
		oSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
		oSP.EXPECT().BuiltInPlan(params.ID).Return("", false)
		mds.EXPECT().OpsServicePlan().Return(oSP).MinTimes(2)
		count, err := tc%2, []error{centrald.ErrorDbError, nil}[tc%2]
		switch tc / 2 {
		case 2:
			oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			oVR.EXPECT().Count(ctx, vrlParams, uint(1)).Return(count, err)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			count, err = 0, nil
			fallthrough
		case 1:
			oV = mock.NewMockVolumeSeriesOps(mockCtrl)
			oV.EXPECT().Count(ctx, vsParams, uint(1)).Return(count, err)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			count, err = 0, nil
			fallthrough
		case 0:
			oSP.EXPECT().Count(ctx, ssParams, uint(1)).Return(count, err)
		default:
			assert.True(false)
		}
		hc.DS = mds
		cntLock = hc.cntLock
		cntUnlock = hc.cntUnlock
		assert.NotPanics(func() { ret = hc.servicePlanDelete(params) })
		assert.NotNil(ret)
		mE, ok = ret.(*ops.ServicePlanDeleteDefault)
		if assert.True(ok) {
			if tc%2 == 0 {
				assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
				assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
			} else {
				assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
				assert.Regexp("^service plan .* "+tObj[tc/2], *mE.Payload.Message)
			}
		}
		assert.Equal(cntLock+1, hc.cntLock)
		assert.Equal(cntUnlock+1, hc.cntUnlock)
		tl.Flush()
	}

	// builtin failure case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oSP.EXPECT().BuiltInPlan(params.ID).Return(string(obj.Name), true)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP).Times(2)
	t.Log("case: builtin failure")
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.servicePlanDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ServicePlanDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	assert.Equal(cntLock, hc.cntLock)
	assert.Equal(cntUnlock, hc.cntUnlock)
	tl.Flush()

	// fetch failure case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	t.Log("case: fetch failure")
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.servicePlanDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ServicePlanDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
	assert.Equal(cntLock, hc.cntLock)
	assert.Equal(cntUnlock, hc.cntUnlock)
}

func TestServicePlanFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.ServicePlanFetchParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("objectID"),
			},
		},
	}
	obj.Name = "servicePlan"

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oSP := mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	t.Log("case: ServicePlanFetch success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.servicePlanFetch(params) })
	assert.NotNil(ret)
	mR, ok := ret.(*ops.ServicePlanFetchOK)
	assert.True(ok)
	assert.Equal(obj, mR.Payload)
	tl.Flush()

	// fetch failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	t.Log("case: ServicePlanDelete fetch failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.servicePlanFetch(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.ServicePlanFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.servicePlanFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ServicePlanFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	t.Log("case: unauthorized")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	hc.DS = mds
	ai.AccountID = "aid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	assert.NotPanics(func() { ret = hc.servicePlanFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ServicePlanFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
}

func TestServicePlanList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.ServicePlanListParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	ctx := params.HTTPRequest.Context()
	objects := []*models.ServicePlan{
		&models.ServicePlan{
			ServicePlanAllOf0: models.ServicePlanAllOf0{
				Meta: &models.ObjMeta{
					ID: models.ObjID("objectID"),
				},
			},
		},
		&models.ServicePlan{
			ServicePlanAllOf0: models.ServicePlanAllOf0{
				Meta: &models.ObjMeta{
					ID: models.ObjID("objectID2"),
				},
			},
			ServicePlanMutable: models.ServicePlanMutable{
				Accounts: []models.ObjIDMutable{"aid1", "aid2"},
			},
		},
	}
	objects[0].Name = "servicePlan"

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oSP := mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().List(ctx, params).Return(objects, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.servicePlanList(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.ServicePlanListOK)
	assert.True(ok)
	assert.Equal(objects, mO.Payload)
	tl.Flush()

	// list fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().List(ctx, params).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	t.Log("case: List failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.servicePlanList(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.ServicePlanListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.servicePlanList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ServicePlanListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: unauthorized filtered")
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().List(ctx, params).Return(objects, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	hc.DS = mds
	ai.AccountID = "aid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	assert.NotPanics(func() { ret = hc.servicePlanList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.ServicePlanListOK)
	assert.True(ok)
	if assert.Len(mO.Payload, 1) {
		assert.EqualValues("objectID2", mO.Payload[0].Meta.ID)
	}
}

func TestServicePlanPublish(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ctx := context.Background()
	req := &http.Request{}
	req.WithContext(ctx)
	params := ops.ServicePlanPublishParams{HTTPRequest: req, ID: "objectID"}
	obj := &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("objectID"),
			},
		},
	}
	obj.Name = "servicePlan"
	obj.State = centrald.PublishedState

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oSP := mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Publish(ctx, params).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	t.Log("case: ServicePlanPublish success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.servicePlanPublish(params) })
	assert.NotNil(ret)
	mR, ok := ret.(*ops.ServicePlanPublishOK)
	assert.True(ok)
	assert.Equal(obj, mR.Payload)
	assert.Empty(evM.InSSProps)
	assert.Equal(obj, evM.InACScope)
	tl.Flush()

	// publish failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Publish(ctx, params).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	t.Log("case: ServicePlanDelete publish failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.servicePlanPublish(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.ServicePlanPublishDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
}

func TestServicePlanRetire(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ctx := context.Background()
	req := &http.Request{}
	req.WithContext(ctx)
	params := ops.ServicePlanRetireParams{HTTPRequest: req, ID: "objectID"}
	obj := &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("objectID"),
			},
		},
	}
	obj.Name = "servicePlan"

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oSP := mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Retire(ctx, params).Return(obj, nil)
	oSP.EXPECT().BuiltInPlan(params.ID).Return("", false)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP).Times(2)
	t.Log("case: ServicePlanRetire success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.servicePlanRetire(params) })
	assert.NotNil(ret)
	mR, ok := ret.(*ops.ServicePlanRetireOK)
	assert.True(ok)
	assert.Equal(obj, mR.Payload)
	assert.Empty(evM.InSSProps)
	assert.Equal(obj, evM.InACScope)
	tl.Flush()

	// retire failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Retire(ctx, params).Return(nil, centrald.ErrorNotFound)
	oSP.EXPECT().BuiltInPlan(params.ID).Return("", false)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP).Times(2)
	t.Log("case: ServicePlanRetire retire failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.servicePlanRetire(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.ServicePlanRetireDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)

	// builtin failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().BuiltInPlan(params.ID).Return(string(obj.Name), true)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	t.Log("case: ServicePlanRetire builtin failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.servicePlanRetire(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ServicePlanRetireDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
}

func TestServicePlanUpdate(t *testing.T) {
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

	aObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta:            &models.ObjMeta{ID: "aid1"},
			TenantAccountID: "tid1",
		},
	}
	aObj.Name = "aName"

	// make the name map
	var nMap JSONToAttrNameMap
	assert.NotPanics(func() { nMap = hc.servicePlanMutableNameMap() })
	// validate some embedded properties
	assert.Equal("name", nMap.jName("Name"))
	assert.Equal("slos", nMap.jName("Slos"))
	assert.Equal("tags", nMap.jName("Tags"))

	// parse params
	objM := &models.ServicePlanMutable{}
	objM.Name = "servicePlan"
	objM.Description = "new description"
	objM.Accounts = []models.ObjIDMutable{"aid1"}
	allSlos := models.SloListMutable{
		"slo1": {ValueType: models.ValueType{Kind: "INT", Value: "5"}},
		"slo2": {ValueType: models.ValueType{Kind: "STRING", Value: "value"}},
	}
	objM.Slos = allSlos
	objM.Tags = models.ObjTags{"tag1", "tag2", "tag3"}
	ai := &auth.Info{}
	params := ops.ServicePlanUpdateParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
		Version:     swag.Int32(8),
		Remove:      []string{},
		Append:      []string{nMap.jName("Accounts")},
		Set:         []string{nMap.jName("Name"), nMap.jName("Slos")},
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

	// success cases, cover all ways to successfully update Slos
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	obj := &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			State: centrald.UnpublishedState,
		},
	}
	obj.Meta = &models.ObjMeta{
		ID:      models.ObjID(params.ID),
		Version: 8,
	}
	obj.Name = "old name"
	obj.Slos = objM.Slos
	exp := &fal.Args{AI: ai, Action: centrald.ServicePlanUpdateAction, ObjID: models.ObjID(params.ID), Name: "old name", Message: "Updated accounts aa added"}
	tcs := []string{"", "append slos", "slos.slo1", "accounts builtIn ok", "tags builtIn ok"}
	for _, tc := range tcs {
		cp := params //copy
		switch tc {
		case "":
			// nothing special
		case "append slos":
			cp.Append = []string{nMap.jName("Slos"), nMap.jName("Accounts")}
			cp.Set = []string{nMap.jName("Name")}
		case "slos.slo1":
			cp.Append = []string{nMap.jName("Accounts")}
			cp.Set = []string{nMap.jName("Name"), tc}
		case "accounts builtIn ok":
			cp.Set = []string{}
		case "tags builtIn ok":
			cp.Append = []string{}
			cp.Set = []string{"tags"}
		default:
			assert.False(true)
		}
		var ua *centrald.UpdateArgs
		var err error
		var uP = [centrald.NumActionTypes][]string{
			centrald.UpdateRemove: cp.Remove,
			centrald.UpdateAppend: cp.Append,
			centrald.UpdateSet:    cp.Set,
		}
		assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, cp.ID, cp.Version, uP) })
		assert.Nil(err)
		assert.NotNil(ua)
		uaM := updateArgsMatcher(t, ua).Matcher()
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		fv := &fakeAuthAccountValidator{}
		hc.authAccountValidator = fv
		mds := mock.NewMockDataStore(mockCtrl)
		oS := mock.NewMockServicePlanOps(mockCtrl)
		mds.EXPECT().OpsServicePlan().Return(oS).MinTimes(2)
		oS.EXPECT().Update(ctx, uaM, cp.Payload).Return(obj, nil)
		oS.EXPECT().Fetch(ctx, cp.ID).Return(obj, nil)
		if tc == "accounts builtIn ok" || tc == "tags builtIn ok" {
			oS.EXPECT().BuiltInPlan(cp.ID).Return(string(obj.Name), true)
		} else {
			oS.EXPECT().BuiltInPlan(cp.ID).Return("", false)
		}
		if util.Contains(cp.Append, "accounts") {
			fv.retString = "aa added"
		}
		t.Log("case: success", tc)
		hc.DS = mds
		fa.Posts = []*fal.Args{}
		var ret middleware.Responder
		assert.NotPanics(func() { ret = hc.servicePlanUpdate(cp) })
		assert.NotNil(ret)
		mO, ok := ret.(*ops.ServicePlanUpdateOK)
		assert.True(ok)
		assert.Equal(obj, mO.Payload)
		if util.Contains(cp.Append, "accounts") {
			assert.Equal([]*fal.Args{exp}, fa.Posts)
			assert.Equal(centrald.ServicePlanUpdateAction, fv.inAction)
			assert.Equal(params.ID, fv.inObjID)
			assert.EqualValues(obj.Name, fv.inObjName)
			assert.Empty(fv.inOldIDList)
			assert.Equal(params.Payload.Accounts, fv.inUpdateIDList)
		} else {
			assert.Empty(fa.Posts)
		}
		tl.Flush()
	}
	objM.Slos = allSlos

	// failure fetching existing ServicePlan
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: fetch failed")
	oS := mock.NewMockServicePlanOps(mockCtrl)
	oS.EXPECT().BuiltInPlan(params.ID).Return("", false)
	oS.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oS).Times(2)
	hc.DS = mds
	ret := hc.servicePlanUpdate(params)
	assert.NotNil(ret)
	mD, ok := ret.(*ops.ServicePlanUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mD.Payload.Message)
	tl.Flush()

	// validateAuthorizedAccountsUpdate failure, error is converted correctly
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: account not exist")
	oS = mock.NewMockServicePlanOps(mockCtrl)
	oS.EXPECT().BuiltInPlan(params.ID).Return("", false)
	oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oS).Times(2)
	hc.DS = mds
	fv := &fakeAuthAccountValidator{retError: fmt.Errorf("aa error")}
	hc.authAccountValidator = fv
	assert.NotPanics(func() { ret = hc.servicePlanUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ServicePlanUpdateDefault)
	assert.True(ok)
	assert.Equal(http.StatusInternalServerError, int(mD.Payload.Code))
	assert.Equal("aa error", *mD.Payload.Message)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: auditLog not ready")
	mds = mock.NewMockDataStore(mockCtrl)
	oS = mock.NewMockServicePlanOps(mockCtrl)
	oS.EXPECT().BuiltInPlan(params.ID).Return("", false)
	oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsServicePlan().Return(oS).Times(2)
	fa.ReadyRet = centrald.ErrorDbError
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.servicePlanUpdate(params) })
	mD, ok = ret.(*ops.ServicePlanUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.servicePlanUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ServicePlanUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	// Update failed, also tests no version param path
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: update failed")
	params.Append = []string{}
	params.Remove = []string{}
	params.Version = nil
	uP[centrald.UpdateAppend] = params.Append
	uP[centrald.UpdateRemove] = params.Remove
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).AddsVersion(int32(obj.Meta.Version)).Matcher()
	oS = mock.NewMockServicePlanOps(mockCtrl)
	oS.EXPECT().BuiltInPlan(params.ID).Return("", false)
	oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oS.EXPECT().Update(ctx, uaM, params.Payload).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oS).Times(3)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.servicePlanUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ServicePlanUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	params.Version = swag.Int32(8)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: version mismatch")
	params.Version = swag.Int32(7)
	mds = mock.NewMockDataStore(mockCtrl)
	oS = mock.NewMockServicePlanOps(mockCtrl)
	oS.EXPECT().BuiltInPlan(params.ID).Return("", false)
	oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsServicePlan().Return(oS).Times(2)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.servicePlanUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ServicePlanUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorIDVerNotFound.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorIDVerNotFound.M, *mD.Payload.Message)
	params.Version = swag.Int32(8)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: SLO update unauthorized")
	params = ops.ServicePlanUpdateParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
		Version:     swag.Int32(8),
		Remove:      []string{},
		Append:      []string{},
		Set:         []string{"slos"},
		Payload:     objM,
	}
	fa.Posts = []*fal.Args{}
	ai.RoleObj = &models.Role{}
	ai.AccountID = "tid2"
	uP[centrald.UpdateRemove] = params.Remove
	uP[centrald.UpdateSet] = params.Set
	uP[centrald.UpdateAppend] = params.Append
	oS = mock.NewMockServicePlanOps(mockCtrl)
	oS.EXPECT().BuiltInPlan(params.ID).Return("", false)
	oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oS).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.servicePlanUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ServicePlanUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)
	exp = &fal.Args{AI: ai, Action: centrald.ServicePlanUpdateAction, ObjID: models.ObjID(params.ID), Name: models.ObjName(obj.Name), Err: true, Message: "Update unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	ai.RoleObj = nil
	tl.Flush()

	// missing invalid updates
	tcs = []string{
		"no change", "name", "- slos", "+ slos", "wrong #slo", "wrong slos", "slos.slo2", "immutable slo",
		"slo.kind", "slo.immutable", "published", "name builtIn", "description builtIn",
	}
	for _, tc := range tcs {
		objM.Slos = allSlos
		obj.State = centrald.UnpublishedState
		obj.Slos = allSlos
		params.Set = []string{tc}
		params.Append = []string{}
		params.Remove = []string{}
		checksBuiltIn := true
		isBuiltIn := false
		doesFetch := false
		switch tc {
		case "no change":
			params.Set = []string{}
			checksBuiltIn = false
		case "name":
			params.Payload.Name = ""
			checksBuiltIn = false
		case "- slos":
			params.Set = []string{}
			params.Remove = []string{"slos"}
			doesFetch = true
		case "+ slos":
			params.Set = []string{}
			params.Append = []string{"slos"}
			obj.Slos = models.SloListMutable{
				"slo2": {ValueType: models.ValueType{Kind: "STRING", Value: "value"}},
			}
			doesFetch = true
		case "wrong #slo":
			params.Set = []string{"slos"}
			obj.Slos = models.SloListMutable{
				"slo1": {ValueType: models.ValueType{Kind: "INT", Value: "5"}},
				"slo2": {ValueType: models.ValueType{Kind: "STRING", Value: "value"}},
				"slo3": {ValueType: models.ValueType{Kind: "STRING", Value: "value"}},
			}
			doesFetch = true
		case "wrong slos":
			params.Set = []string{"slos"}
			obj.Slos = models.SloListMutable{
				"slo1": {ValueType: models.ValueType{Kind: "INT", Value: "5"}},
				"slo3": {ValueType: models.ValueType{Kind: "STRING", Value: "value"}},
			}
			doesFetch = true
		case "slos.slo2":
			obj.Slos = models.SloListMutable{
				"slo1": {ValueType: models.ValueType{Kind: "INT", Value: "5"}},
				"slo3": {ValueType: models.ValueType{Kind: "STRING", Value: "value"}},
			}
			doesFetch = true
		case "immutable slo":
			params.Set = []string{"slos"}
			obj.Slos = models.SloListMutable{
				"slo1": {
					ValueType:                 models.ValueType{Kind: "INT", Value: "5"},
					RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: true},
				},
				"slo2": {ValueType: models.ValueType{Kind: "STRING", Value: "value"}},
			}
			doesFetch = true
		case "slo.kind":
			params.Set = []string{"slos"}
			obj.Slos = models.SloListMutable{
				"slo1": {ValueType: models.ValueType{Kind: "STRING", Value: "5"}},
				"slo2": {ValueType: models.ValueType{Kind: "STRING", Value: "value"}},
			}
			doesFetch = true
		case "slo.immutable":
			params.Set = []string{"slos"}
			objM.Slos = models.SloListMutable{
				"slo1": {ValueType: models.ValueType{Kind: "INT", Value: "5"}},
				"slo2": {
					ValueType:                 models.ValueType{Kind: "STRING", Value: "value"},
					RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: true},
				},
			}
			obj.Slos = models.SloListMutable{
				"slo1": {ValueType: models.ValueType{Kind: "INT", Value: "5"}},
				"slo2": {ValueType: models.ValueType{Kind: "STRING", Value: "value"}},
			}
			doesFetch = true
		case "published":
			params.Set = []string{"slos"}
			obj.State = centrald.PublishedState
			doesFetch = true
		case "name builtIn":
			params.Set = []string{"name"}
			params.Payload.Name = "new plan name"
			isBuiltIn = true
		case "description builtIn":
			params.Set = []string{"description"}
			isBuiltIn = true
		default:
			assert.False(true)
		}
		if doesFetch || checksBuiltIn {
			mockCtrl.Finish()
			mockCtrl = gomock.NewController(t)
			oS = mock.NewMockServicePlanOps(mockCtrl)
			if isBuiltIn {
				oS.EXPECT().BuiltInPlan(params.ID).Return(string(objM.Name), true)
			} else if checksBuiltIn {
				oS.EXPECT().BuiltInPlan(params.ID).Return("", false)
			}
			if doesFetch {
				oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			}
			mds = mock.NewMockDataStore(mockCtrl)
			mds.EXPECT().OpsServicePlan().Return(oS).MinTimes(1)
			hc.DS = mds
		}
		t.Log("case: error case " + tc)
		assert.NotPanics(func() { ret = hc.servicePlanUpdate(params) })
		assert.NotNil(ret)
		mD, ok = ret.(*ops.ServicePlanUpdateDefault)
		assert.True(ok)
		assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
		assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
		t.Log("full error: " + *mD.Payload.Message)
		tl.Flush()
	}

	// no payload
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: no payload")
	mds = mock.NewMockDataStore(mockCtrl)
	params.Payload = nil
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.servicePlanUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ServicePlanUpdateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
		assert.Equal(centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	}
}
