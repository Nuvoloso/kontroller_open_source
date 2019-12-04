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
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/csp_credential"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/csp_domain"
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

func TestCspCredentialFetchFilter(t *testing.T) {
	assert := assert.New(t)
	hc := newHandlerComp()
	obj := &models.CSPCredential{
		CSPCredentialAllOf0: models.CSPCredentialAllOf0{
			Meta:      &models.ObjMeta{ID: "id1"},
			AccountID: "tid1",
		},
		CSPCredentialMutable: models.CSPCredentialMutable{
			CredentialAttributes: map[string]models.ValueType{
				"k1": models.ValueType{Kind: "STRING", Value: "v1"},
				"k2": models.ValueType{Kind: "SECRET", Value: "v2"},
			},
		},
	}
	rObj := &models.Role{}
	rObj.Capabilities = map[string]bool{}
	ai := &auth.Info{}

	t.Log("case: internal user")
	assert.NoError(hc.cspCredentialFetchFilter(ai, obj))
	assert.Len(obj.CredentialAttributes, 2)
	assert.Equal("v1", obj.CredentialAttributes["k1"].Value)
	assert.Equal("v2", obj.CredentialAttributes["k2"].Value)

	t.Log("case: SystemManagementCap capability fails")
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.SystemManagementCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.cspCredentialFetchFilter(ai, obj))

	t.Log("case: CSPDomainManagementCap capability correct tenant")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	ai.AccountID = "tid1"
	assert.NoError(hc.cspCredentialFetchFilter(ai, obj))
	assert.Len(obj.CredentialAttributes, 2)
	assert.Equal("v1", obj.CredentialAttributes["k1"].Value)
	assert.Equal("v2", obj.CredentialAttributes["k2"].Value)

	t.Log("case: CSPDomainManagementCap capability wrong tenant")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	ai.AccountID = "tid2"
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.cspCredentialFetchFilter(ai, obj))

	t.Log("case: not authorized account")
	ai.AccountID = "aid9"
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.cspCredentialFetchFilter(ai, obj))
}

func TestCspCredentialCreate(t *testing.T) {
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
	// build attrs from the domain type description
	cspDomainTypes := app.SupportedCspDomainTypes()
	assert.NotNil(cspDomainTypes)
	assert.NotEmpty(cspDomainTypes)
	dt := cspDomainTypes[0]
	dad := app.DomainAttrDescForType(dt)
	assert.NotEmpty(dad)
	dtAttrs := make(map[string]models.ValueType, len(dad))
	for da, desc := range dad {
		vt := models.ValueType{Kind: desc.Kind}
		switch desc.Kind {
		case "INT":
			vt.Value = "5"
		case "SECRET":
			vt.Value = "credentials"
		case "STRING":
			vt.Value = "string"
		default:
			assert.True(false)
		}
		dtAttrs[da] = vt
	}
	ai := &auth.Info{AccountID: "aid1"}
	params := ops.CspCredentialCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
		Payload: &models.CSPCredential{
			CSPCredentialAllOf0: models.CSPCredentialAllOf0{
				CspDomainType: dt,
			},
			CSPCredentialMutable: models.CSPCredentialMutable{
				Name:                 "cspCredential1",
				CredentialAttributes: dtAttrs,
			},
		},
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.CSPCredential{
		CSPCredentialAllOf0: models.CSPCredentialAllOf0{
			Meta: &models.ObjMeta{
				ID:      "objectID",
				Version: 1,
			},
			AccountID:     "aid1",
			CspDomainType: params.Payload.CspDomainType,
		},
		CSPCredentialMutable: models.CSPCredentialMutable{
			Name: params.Payload.Name,
			CredentialAttributes: map[string]models.ValueType{
				"property1": {Kind: "STRING", Value: "string1"},
				"property2": {Kind: "INT", Value: "4"},
				"property3": {Kind: "SECRET", Value: "secret string"},
			},
		},
	}
	aObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta:            &models.ObjMeta{ID: "aid2"},
			TenantAccountID: "tid1",
		},
	}

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()

	// success, accountID from auth.Info
	t.Log("case: success")
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "aid1").Return(aObj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	appCSP := mock.NewMockAppCloudServiceProvider(mockCtrl)
	app.AppCSP = appCSP
	appCSP.EXPECT().ValidateCspCredential(obj.CspDomainType, nil, dtAttrs).Return(nil)
	oCred := mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Create(ctx, params.Payload).Return(obj, nil)
	mds.EXPECT().OpsCspCredential().Return(oCred).MinTimes(1)
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.cspCredentialCreate(params) })
	assert.EqualValues("aid1", params.Payload.AccountID)
	assert.NotNil(ret)
	_, ok := ret.(*ops.CspCredentialCreateCreated)
	assert.True(ok)
	assert.Len(evM.InSSProps, 1)
	assert.EqualValues("objectID", evM.InSSProps["meta.id"])
	assert.Equal(obj, evM.InACScope)
	exp := &fal.Args{AI: ai, Action: centrald.CspCredentialCreateAction, ObjID: "objectID", Name: params.Payload.Name, Message: "Created"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: success with no audit detail")
	fa.Posts = []*fal.Args{}
	appCSP = mock.NewMockAppCloudServiceProvider(mockCtrl)
	app.AppCSP = appCSP
	appCSP.EXPECT().ValidateCspCredential(obj.CspDomainType, nil, dtAttrs).Return(nil)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "aid1").Return(&models.Account{}, nil)
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Create(ctx, params.Payload).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	mds.EXPECT().OpsCspCredential().Return(oCred).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspCredentialCreate(params) })
	assert.NotNil(ret)
	_, ok = ret.(*ops.CspCredentialCreateCreated)
	exp.Message = "Created"
	assert.Equal([]*fal.Args{exp}, fa.Posts)

	// Create failed, cover accountId present case
	t.Log("case: Create fails")
	ai.AccountID = ""
	params.Payload.AccountID = "aid1"
	appCSP = mock.NewMockAppCloudServiceProvider(mockCtrl)
	app.AppCSP = appCSP
	appCSP.EXPECT().ValidateCspCredential(obj.CspDomainType, nil, dtAttrs).Return(nil)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "aid1").Return(&models.Account{}, nil)
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Create(ctx, params.Payload).Return(nil, centrald.ErrorExists)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	mds.EXPECT().OpsCspCredential().Return(oCred).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspCredentialCreate(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.CspCredentialCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorExists.M, *mE.Payload.Message)
	tl.Flush()

	mockCtrl.Finish()
	t.Log("case: auditLog not ready")
	fa.ReadyRet = centrald.ErrorDbError
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspCredentialCreate(params) })
	mE, ok = ret.(*ops.CspCredentialCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.cspCredentialCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspCredentialCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	// account fetch db error
	t.Log("case: account fetch fails")
	mockCtrl = gomock.NewController(t)
	params.Payload.AccountID = "aid1"
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "aid1").Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	cntRLock := hc.cntRLock
	cntRUnlock := hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.cspCredentialCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspCredentialCreateDefault)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	tl.Flush()

	// account not found
	mockCtrl.Finish()
	t.Log("case: account not found")
	mockCtrl = gomock.NewController(t)
	params.Payload.AccountID = "aid1"
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "aid1").Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.cspCredentialCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspCredentialCreateDefault)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
	params.Payload.AccountID = ""
	tl.Flush()

	// account not authorized
	mockCtrl.Finish()
	t.Log("case: account not authorized")
	mockCtrl = gomock.NewController(t)
	fa.Posts = []*fal.Args{}
	ai.AccountID = "aid1"
	ai.RoleObj = &models.Role{}
	params.Payload.AccountID = "aid1"
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "aid1").Return(aObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.cspCredentialCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspCredentialCreateDefault)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	exp = &fal.Args{AI: ai, Action: centrald.CspCredentialCreateAction, Name: params.Payload.Name, Err: true, Message: "Create unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	params.Payload.AccountID = ""
	ai.RoleObj = nil
	tl.Flush()

	// missing property failure cases
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	tcs := []string{"name", "cspDomainType", "credentialAttributes"}
	for _, tc := range tcs {
		cl := *params.Payload // copy
		switch tc {
		case "name":
			cl.Name = ""
		case "cspDomainType":
			cl.CspDomainType = ""
		case "credentialAttributes":
			cl.CredentialAttributes = nil
		default:
			assert.False(true)
		}
		t.Log("case: CSPCredentialCreate missing error case: " + tc)
		assert.NotPanics(func() {
			ret = hc.cspCredentialCreate(ops.CspCredentialCreateParams{HTTPRequest: requestWithAuthContext(ai), Payload: &cl})
		})
		assert.NotNil(ret)
		_, ok = ret.(*ops.CspCredentialCreateCreated)
		assert.False(ok)
		mE, ok := ret.(*ops.CspCredentialCreateDefault)
		assert.True(ok)
		assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
		assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
		tl.Flush()
	}

	// ValidateCspCredential fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: ValidateCspCredential fails")
	ai.AccountID = "aid1"
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "aid1").Return(&models.Account{}, nil)
	mds.EXPECT().OpsAccount().Return(oA)
	appCSP = mock.NewMockAppCloudServiceProvider(mockCtrl)
	app.AppCSP = appCSP
	appCSP.EXPECT().ValidateCspCredential(obj.CspDomainType, nil, dtAttrs).Return(fmt.Errorf("validation failure"))
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspCredentialCreate(params) })
	assert.NotNil(ret)
	_, ok = ret.(*ops.CspCredentialCreateCreated)
	assert.False(ok)
	mE, ok = ret.(*ops.CspCredentialCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
	assert.Regexp("validation failure", *mE.Payload.Message)
	tl.Flush()

	// no payload
	params.Payload = nil
	t.Log("case: nil payload")
	assert.NotPanics(func() { ret = hc.cspCredentialCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspCredentialCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorMissing.M, *mE.Payload.Message)
	assert.True(ok)
}

func TestCspCredentialDelete(t *testing.T) {
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
	params := ops.CspCredentialDeleteParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "credID",
	}
	ctx := params.HTTPRequest.Context()
	cspCredObj := &models.CSPCredential{
		CSPCredentialAllOf0: models.CSPCredentialAllOf0{
			Meta: &models.ObjMeta{
				ID: "credID",
			},
			AccountID:     "aid1",
			CspDomainType: "AWS",
		},
		CSPCredentialMutable: models.CSPCredentialMutable{
			Name: "cspCredential",
		},
	}
	domListParams := csp_domain.CspDomainListParams{
		AccountID:       swag.String(string(cspCredObj.AccountID)),
		CspDomainType:   swag.String(string(cspCredObj.CspDomainType)),
		CspCredentialID: swag.String(params.ID),
	}

	var ret middleware.Responder

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	t.Log("case: success")
	oCred := mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Delete(ctx, params.ID).Return(nil)
	oCred.EXPECT().Fetch(ctx, params.ID).Return(cspCredObj, nil)
	oCSP := mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Count(ctx, domListParams, uint(1)).Return(0, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspCredential().Return(oCred).MinTimes(1)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	hc.DS = mds
	cntLock := hc.cntLock
	cntUnlock := hc.cntUnlock
	assert.NotPanics(func() { ret = hc.cspCredentialDelete(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.CspCredentialDeleteNoContent)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Empty(evM.InSSProps)
	assert.Equal(cspCredObj, evM.InACScope)
	exp := &fal.Args{AI: ai, Action: centrald.CspCredentialDeleteAction, ObjID: models.ObjID(params.ID), Name: cspCredObj.Name, Message: "Deleted"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	fa.Posts = []*fal.Args{}
	tl.Flush()

	// failure: credential is referenced by CSPDomain
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: failure: credential is referenced by CSPDomain")
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, params.ID).Return(cspCredObj, nil)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Count(ctx, domListParams, uint(1)).Return(1, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspCredential().Return(oCred).MinTimes(1)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.cspCredentialDelete(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.CspCredentialDeleteDefault)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Empty(evM.InSSProps)
	assert.Equal(cspCredObj, evM.InACScope)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Regexp("CSP domains still refer to the credential", *mE.Payload.Message)
	tl.Flush()
	params.ID = "credID"
	cspCredObj.Meta.ID = "credID"

	// delete failure case, cover valid tenant account
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: delete failure")
	ai.AccountID = "aid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Delete(ctx, params.ID).Return(centrald.ErrorDbError)
	oCred.EXPECT().Fetch(ctx, params.ID).Return(cspCredObj, nil)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Count(ctx, domListParams, uint(1)).Return(0, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspCredential().Return(oCred).MinTimes(1)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.cspCredentialDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspCredentialDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	ai.RoleObj = nil
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: auditLog not ready")
	mds = mock.NewMockDataStore(mockCtrl)
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, params.ID).Return(cspCredObj, nil)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	fa.ReadyRet = centrald.ErrorDbError
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspCredentialDelete(params) })
	mE, ok = ret.(*ops.CspCredentialDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.cspCredentialDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspCredentialDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	// not authorized tenant admin
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized tenant admin")
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	mds = mock.NewMockDataStore(mockCtrl)
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, params.ID).Return(cspCredObj, nil)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.cspCredentialDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspCredentialDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Regexp(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	exp = &fal.Args{AI: ai, Action: centrald.CspCredentialDeleteAction, ObjID: models.ObjID(params.ID), Name: cspCredObj.Name, Err: true, Message: "Delete unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	ai.RoleObj = nil
	tl.Flush()

	// fetch failure case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: fetch failure")
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspCredential().Return(oCred).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.cspCredentialDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspCredentialDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
	assert.Equal(cntLock, hc.cntLock)
	assert.Equal(cntUnlock, hc.cntUnlock)
}

func TestCspCredentialFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.CspCredentialFetchParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.CSPCredential{
		CSPCredentialAllOf0: models.CSPCredentialAllOf0{
			Meta: &models.ObjMeta{
				ID: "objectID",
			},
			AccountID:     "aid1",
			CspDomainType: "AWS",
		},
		CSPCredentialMutable: models.CSPCredentialMutable{
			Name: "cspCredential",
		},
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	t.Log("case: success")
	oCred := mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspCredential().Return(oCred).MinTimes(1)
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.cspCredentialFetch(params) })
	assert.NotNil(ret)
	mR, ok := ret.(*ops.CspCredentialFetchOK)
	assert.True(ok)
	assert.Equal(obj, mR.Payload)
	tl.Flush()

	// fetch failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: fetch failure")
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspCredential().Return(oCred).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspCredentialFetch(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.CspCredentialFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.cspCredentialFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspCredentialFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: unauthorized")
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	hc.DS = mds
	ai.AccountID = "aid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: false}
	assert.NotPanics(func() { ret = hc.cspCredentialFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspCredentialFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
}

func TestCspCredentialList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.CspCredentialListParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	ctx := params.HTTPRequest.Context()
	objects := []*models.CSPCredential{
		&models.CSPCredential{
			CSPCredentialAllOf0: models.CSPCredentialAllOf0{
				Meta: &models.ObjMeta{
					ID: "objectID",
				},
				AccountID:     "aid1",
				CspDomainType: "AWS",
			},
			CSPCredentialMutable: models.CSPCredentialMutable{
				Name: "cspCredential",
			},
		},
		&models.CSPCredential{
			CSPCredentialAllOf0: models.CSPCredentialAllOf0{
				Meta: &models.ObjMeta{
					ID: "objectID2",
				},
				AccountID:     "aid2",
				CspDomainType: "AWS",
			},
			CSPCredentialMutable: models.CSPCredentialMutable{
				Name: "cspCredential2",
			},
		},
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	t.Log("case: success")
	oCred := mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().List(ctx, params).Return(objects, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspCredential().Return(oCred).MinTimes(1)
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.cspCredentialList(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.CspCredentialListOK)
	assert.True(ok)
	assert.Equal(objects, mO.Payload)
	tl.Flush()

	// list fails, account added to params
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: List failure, account added to params")
	ai.AccountID = "aid2"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	cParams := params
	cParams.AccountID = swag.String("aid2")
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().List(ctx, cParams).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspCredential().Return(oCred).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspCredentialList(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.CspCredentialListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.cspCredentialList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspCredentialListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	t.Log("case: mismatched account")
	params.AccountID = swag.String("aid1")
	ai.AccountID = "aid2"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	assert.NotPanics(func() { ret = hc.cspCredentialList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.CspCredentialListOK)
	assert.True(ok)
	assert.Empty(mO.Payload)

	t.Log("case: missing capability")
	ai.AccountID = "aid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	assert.NotPanics(func() { ret = hc.cspCredentialList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.CspCredentialListOK)
	assert.True(ok)
	assert.Empty(mO.Payload)
}

func TestCspCredentialUpdate(t *testing.T) {
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

	// make the name map
	var nMap JSONToAttrNameMap
	assert.NotPanics(func() { nMap = hc.cspCredentialMutableNameMap() })
	assert.Equal("credentialAttributes", nMap.jName("CredentialAttributes"))

	// build attrs from the domain type description
	cspDomainTypes := app.SupportedCspDomainTypes()
	assert.NotNil(cspDomainTypes)
	assert.NotEmpty(cspDomainTypes)
	dt := cspDomainTypes[0]
	cad := app.CredentialAttrDescForType(dt)
	assert.NotEmpty(cad)
	dtAttrs := make(map[string]models.ValueType, len(cad))
	for da, desc := range cad {
		vt := models.ValueType{Kind: desc.Kind}
		switch desc.Kind {
		case "INT":
			vt.Value = "5"
		case "STRING":
			vt.Value = "string"
		case "SECRET":
			vt.Value = "credentials"
		default:
			assert.True(false)
		}
		dtAttrs[da] = vt
	}

	// parse params
	objM := &models.CSPCredentialMutable{
		Name:                 "cspCredential",
		CredentialAttributes: dtAttrs,
	}
	ai := &auth.Info{}
	params := ops.CspCredentialUpdateParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
		Version:     swag.Int32(8),
		Remove:      []string{},
		Append:      []string{},
		Set:         []string{nMap.jName("Name"), nMap.jName("CredentialAttributes")},
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

	meta := &models.ObjMeta{ID: "id1", Version: models.ObjVersion(*params.Version)}
	cspCredObj := &models.CSPCredential{
		CSPCredentialAllOf0: models.CSPCredentialAllOf0{
			Meta:          meta,
			AccountID:     "aid1",
			CspDomainType: dt,
		},
		CSPCredentialMutable: models.CSPCredentialMutable{
			Name:                 "cred1",
			CredentialAttributes: dtAttrs,
		},
	}
	domListParams := csp_domain.CspDomainListParams{
		AccountID:       swag.String(string(cspCredObj.AccountID)),
		CspDomainType:   swag.String(string(cspCredObj.CspDomainType)),
		CspCredentialID: swag.String(params.ID),
	}
	domObj := []*models.CSPDomain{
		&models.CSPDomain{
			CSPDomainAllOf0: models.CSPDomainAllOf0{
				Meta: &models.ObjMeta{
					ID: "domObjectID",
				},
				AccountID:     "aid1",
				CspDomainType: "AWS",
			},
			CSPDomainMutable: models.CSPDomainMutable{
				Name: "cspDomain",
			},
		},
	}

	oldAttrs := make(map[string]models.ValueType)
	for key, value := range cspCredObj.CredentialAttributes {
		oldAttrs[key] = value
	}
	var ret middleware.Responder

	// success
	var fetchObj *models.CSPCredential
	testutils.Clone(cspCredObj, &fetchObj)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	t.Log("case: success")
	oCred := mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, params.ID).Return(fetchObj, nil)
	oCred.EXPECT().Update(ctx, uaM, params.Payload).Return(cspCredObj, nil)
	oCSP := mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().List(ctx, domListParams).Return(domObj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspCredential().Return(oCred).MinTimes(1)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	appCSP := mock.NewMockAppCloudServiceProvider(mockCtrl)
	app.AppCSP = appCSP
	appCSP.EXPECT().ValidateCspCredential(fetchObj.CspDomainType, oldAttrs, dtAttrs).Return(nil)
	hc.DS = mds
	cntRLock := hc.cntRLock
	cntRUnlock := hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.cspCredentialUpdate(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.CspCredentialUpdateOK)
	assert.True(ok)
	assert.Equal(cntRLock, hc.cntRLock)
	assert.Equal(cntRUnlock, hc.cntRUnlock)
	assert.Equal(cspCredObj, mO.Payload)
	assert.Empty(evM.InSSProps)
	assert.Equal(cspCredObj, evM.InACScope)
	exp := &fal.Args{
		AI:      ai,
		Action:  centrald.CspDomainUpdateAction,
		ObjID:   domObj[0].Meta.ID,
		Name:    domObj[0].Name,
		Message: "Updated CspCredential attributes",
		RefID:   models.ObjIDMutable(cspCredObj.Meta.ID),
	}
	assert.Equal([]*fal.Args{exp}, fa.Events)
	cspCredObj.CredentialAttributes = nil
	fa.Posts = []*fal.Args{}
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: auditLog not ready")
	mds = mock.NewMockDataStore(mockCtrl)
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, params.ID).Return(cspCredObj, nil)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	fa.ReadyRet = centrald.ErrorDbError
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspCredentialUpdate(params) })
	mD, ok := ret.(*ops.CspCredentialUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.cspCredentialUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspCredentialUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized tenant admin")
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	mds = mock.NewMockDataStore(mockCtrl)
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, params.ID).Return(cspCredObj, nil)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspCredentialUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspCredentialUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)
	exp = &fal.Args{AI: ai, Action: centrald.CspCredentialUpdateAction, ObjID: models.ObjID(params.ID), Name: cspCredObj.Name, Err: true, Message: "Update unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	ai.RoleObj = nil
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: fetch failure")
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspCredential().Return(oCred).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspCredentialUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspCredentialUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()

	// Update failed, also tests no version param paths
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: failed on Update")
	oldAttrs = map[string]models.ValueType{}
	params.Version = nil
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).AddsVersion(int32(cspCredObj.Meta.Version)).Matcher()
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, params.ID).Return(cspCredObj, nil)
	oCred.EXPECT().Update(ctx, uaM, params.Payload).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspCredential().Return(oCred).MinTimes(1)
	appCSP = mock.NewMockAppCloudServiceProvider(mockCtrl)
	app.AppCSP = appCSP
	hc.DS = mds
	appCSP.EXPECT().ValidateCspCredential(fetchObj.CspDomainType, oldAttrs, dtAttrs).Return(nil)
	assert.NotPanics(func() { ret = hc.cspCredentialUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspCredentialUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	assert.Equal(dtAttrs, cspCredObj.CredentialAttributes)
	cspCredObj.CredentialAttributes = nil
	cspCredObj.Meta = meta
	tl.Flush()

	// invalid update attributes for Credential
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: invalid update attributes for Credential")
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, params.ID).Return(cspCredObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	appCSP = mock.NewMockAppCloudServiceProvider(mockCtrl)
	app.AppCSP = appCSP
	appCSP.EXPECT().ValidateCspCredential(fetchObj.CspDomainType, oldAttrs, dtAttrs).Return(fmt.Errorf("invalid attribute"))
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspCredentialUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspCredentialUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	assert.Regexp("invalid attribute", *mD.Payload.Message)
	cspCredObj.CredentialAttributes = nil
	cspCredObj.Meta = meta
	tl.Flush()

	// invalid attribute update set missing attribute (codepath fully tested in TestMergeAttributes)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: missing attribute in payload")
	params.Set = []string{nMap.jName("CredentialAttributes") + ".notThere"}
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, params.ID).Return(cspCredObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspCredentialUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspCredentialUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	assert.Regexp("set='credentialAttributes.notThere'", *mD.Payload.Message)
	tl.Flush()

	// version pre-check fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: version pre-check fails")
	params.Set = []string{nMap.jName("CredentialAttributes") + ".property1"}
	params.Version = swag.Int32(8)
	oVersion := cspCredObj.Meta.Version
	cspCredObj.Meta.Version++
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, params.ID).Return(cspCredObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspCredentialUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspCredentialUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorIDVerNotFound.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorIDVerNotFound.M, *mD.Payload.Message)
	cspCredObj.Meta.Version = oVersion
	tl.Flush()

	// no changes requested
	params.Set = []string{}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	t.Log("case: CspCredentialUpdate no change")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspCredentialUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspCredentialUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	tl.Flush()

	// empty name
	params.Set = []string{"name"}
	params.Payload.Name = ""
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	t.Log("case: empty name")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspCredentialUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspCredentialUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	tl.Flush()

	// no payload
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: no payload")
	mds = mock.NewMockDataStore(mockCtrl)
	params.Set = []string{"name"}
	params.Payload = nil
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspCredentialUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspCredentialUpdateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
		assert.Regexp(centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
		assert.Regexp("missing payload", *mD.Payload.Message)
	}
}

func TestCspCredentialMetadata(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app

	// build attrs from the domain type description
	cspDomainTypes := app.SupportedCspDomainTypes()
	assert.NotNil(cspDomainTypes)
	assert.NotEmpty(cspDomainTypes)
	dt := cspDomainTypes[0]
	md := &models.CSPCredentialMetadata{}
	md.AttributeMetadata = app.CredentialAttrDescForType(dt)
	assert.NotEmpty(md.AttributeMetadata)
	retCDM := &ops.CspCredentialMetadataOK{
		Payload: md,
	}
	params := ops.CspCredentialMetadataParams{
		CspDomainType: string(dt),
	}

	// success case
	rc := hc.cspCredentialMetadata(params)
	assert.NotNil(rc)
	ret, ok := rc.(*ops.CspCredentialMetadataOK)
	assert.True(ok)
	assert.Equal(retCDM, ret)

	// failure case
	params.CspDomainType += "foo"
	rc = hc.cspCredentialMetadata(params)
	assert.NotNil(rc)
	retErr, ok := rc.(*ops.CspCredentialMetadataDefault)
	assert.True(ok)
	assert.EqualValues(400, retErr.Payload.Code)
	assert.Regexp("invalid cspDomainType", *retErr.Payload.Message)
}
