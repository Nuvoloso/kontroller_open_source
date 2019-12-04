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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/cluster"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	fal "github.com/Nuvoloso/kontroller/pkg/centrald/audit/fake"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	clPkg "github.com/Nuvoloso/kontroller/pkg/cluster"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestCspDomainFetchFilter(t *testing.T) {
	assert := assert.New(t)
	hc := newHandlerComp()
	obj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta:      &models.ObjMeta{ID: "id1"},
			AccountID: "tid1",
			CspDomainAttributes: map[string]models.ValueType{
				"k1": models.ValueType{Kind: "STRING", Value: "v1"},
				"k2": models.ValueType{Kind: "SECRET", Value: "v1"},
			},
		},
		CSPDomainMutable: models.CSPDomainMutable{
			AuthorizedAccounts: []models.ObjIDMutable{"aid1", "aid2"},
		},
	}
	rObj := &models.Role{}
	rObj.Capabilities = map[string]bool{}
	ai := &auth.Info{}

	t.Log("case: internal user")
	assert.NoError(hc.cspDomainFetchFilter(ai, obj))
	assert.Len(obj.CspDomainAttributes, 2)
	assert.Equal("v1", obj.CspDomainAttributes["k1"].Value)
	assert.Equal("v1", obj.CspDomainAttributes["k2"].Value)

	t.Log("case: SystemManagementCap capability fails")
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.SystemManagementCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.cspDomainFetchFilter(ai, obj))

	t.Log("case: CSPDomainManagementCap capability correct tenant")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	ai.AccountID = "tid1"
	assert.NoError(hc.cspDomainFetchFilter(ai, obj))
	assert.Len(obj.CspDomainAttributes, 2)
	assert.Equal("v1", obj.CspDomainAttributes["k1"].Value)
	assert.Equal("v1", obj.CspDomainAttributes["k2"].Value)

	t.Log("case: CSPDomainManagementCap capability wrong tenant")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	ai.AccountID = "tid2"
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.cspDomainFetchFilter(ai, obj))

	t.Log("case: authorized account")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	ai.AccountID = "aid2"
	assert.NoError(hc.cspDomainFetchFilter(ai, obj))
	assert.Len(obj.CspDomainAttributes, 2)
	assert.Equal("v1", obj.CspDomainAttributes["k1"].Value)
	assert.Equal("********", obj.CspDomainAttributes["k2"].Value)

	t.Log("case: not authorized account")
	ai.AccountID = "aid9"
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.cspDomainFetchFilter(ai, obj))

	t.Log("case: no authorized accounts")
	obj.AuthorizedAccounts = nil
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.cspDomainFetchFilter(ai, obj))
}

func TestCspDomainApplyApplyInheritedProperties(t *testing.T) {
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
	ctx := context.Background()
	cspDomainTypes := app.SupportedCspDomainTypes()
	assert.NotNil(cspDomainTypes)
	assert.NotEmpty(cspDomainTypes)
	dt := cspDomainTypes[0]
	obj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID:      "objectID",
				Version: 1,
			},
			AccountID:     "aid1",
			CspDomainType: dt,
			CspDomainAttributes: map[string]models.ValueType{
				"cda1": models.ValueType{Kind: "STRING", Value: "string1"},
				"cda2": models.ValueType{Kind: "INT", Value: "4"},
				"cda3": models.ValueType{Kind: "SECRET", Value: "secret string"},
			},
		},
		CSPDomainMutable: models.CSPDomainMutable{
			Name:            "cspDomain1",
			ManagementHost:  "f.q.d.n",
			CspCredentialID: "credID",
			ClusterUsagePolicy: &models.ClusterUsagePolicy{
				Inherited: false,
			},
		},
	}
	credObj := &models.CSPCredential{
		CSPCredentialAllOf0: models.CSPCredentialAllOf0{
			Meta:          &models.ObjMeta{ID: "credID"},
			AccountID:     "tid1",
			CspDomainType: dt,
		},
		CSPCredentialMutable: models.CSPCredentialMutable{
			CredentialAttributes: map[string]models.ValueType{
				"ca1": models.ValueType{Kind: "STRING", Value: "v1"},
				"ca2": models.ValueType{Kind: "SECRET", Value: "v2"},
			},
		},
	}
	ai := &auth.Info{AccountID: "aid1"}
	assert.True(ai.Internal())

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()

	t.Log("case: success, no policy override, cred merged")
	oCred := mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, "credID").Return(credObj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	hc.DS = mds
	err := hc.cspDomainApplyInheritedProperties(ctx, ai, obj, nil, nil)
	assert.Nil(err)
	expAttrs := map[string]models.ValueType{
		"ca1":  models.ValueType{Kind: "STRING", Value: "v1"},
		"ca2":  models.ValueType{Kind: "SECRET", Value: "v2"},
		"cda1": models.ValueType{Kind: "STRING", Value: "string1"},
		"cda2": models.ValueType{Kind: "INT", Value: "4"},
		"cda3": models.ValueType{Kind: "SECRET", Value: "secret string"},
	}
	assert.Equal(util.SortedStringKeys(expAttrs), util.SortedStringKeys(obj.CspDomainAttributes))
	tl.Flush()

	t.Log("case: success, credential attributes formerly present in CspDomain object get overridden")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	obj.CspDomainAttributes = map[string]models.ValueType{
		"ca1":  models.ValueType{Kind: "STRING", Value: "old_v1"},
		"ca2":  models.ValueType{Kind: "SECRET", Value: "old_v2"},
		"cda1": models.ValueType{Kind: "STRING", Value: "string1"},
		"cda2": models.ValueType{Kind: "INT", Value: "4"},
		"cda3": models.ValueType{Kind: "SECRET", Value: "secret string"},
	}
	err = hc.cspDomainApplyInheritedProperties(ctx, ai, obj, nil, credObj)
	assert.Nil(err)
	assert.Equal(util.SortedStringKeys(expAttrs), util.SortedStringKeys(obj.CspDomainAttributes))
	tl.Flush()

	t.Log("case: error fetching CspCredential object")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, "credID").Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	hc.DS = mds
	err = hc.cspDomainApplyInheritedProperties(ctx, ai, obj, nil, nil)
	assert.NotNil(err)
	tl.Flush()

	t.Log("case: no CspCredentialId present -> no need to use")
	obj.CspCredentialID = ""
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	err = hc.cspDomainApplyInheritedProperties(ctx, ai, obj, nil, nil)
	assert.Nil(err)
	tl.Flush()

	// Test as an external user
	ai.RoleObj = &models.Role{}
	assert.False(ai.Internal())

	t.Log("case: success, not internal user, credential ignored")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	obj.CspCredentialID = "credID"
	obj.CspDomainAttributes = map[string]models.ValueType{
		"cda1": models.ValueType{Kind: "STRING", Value: "string1"},
		"cda2": models.ValueType{Kind: "INT", Value: "4"},
		"cda3": models.ValueType{Kind: "SECRET", Value: "secret string"},
	}
	err = hc.cspDomainApplyInheritedProperties(ctx, ai, obj, nil, credObj)
	assert.Nil(err)
	for k := range credObj.CredentialAttributes {
		_, found := obj.CspDomainAttributes[k]
		assert.False(found)
	}
	tl.Flush()
}

func TestCspDomainCreate(t *testing.T) {
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
	var dt models.CspDomainTypeMutable
	var dtAttrs map[string]models.ValueType
	var storageTypes []*models.CSPStorageType
	for _, dt = range cspDomainTypes {
		dad := app.DomainAttrDescForType(dt)
		assert.NotEmpty(dad)
		dtAttrs = make(map[string]models.ValueType, len(dad))
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
		storageTypes = []*models.CSPStorageType{}
		for _, st := range app.SupportedCspStorageTypes() {
			if string(st.CspDomainType) == string(dt) {
				storageTypes = append(storageTypes, st)
			}
		}
		if len(storageTypes) > 0 {
			break
		}
	}
	assert.True(len(storageTypes) > 0)
	scm := map[string]models.StorageCost{
		string(storageTypes[0].Name): {CostPerGiB: 1.0},
	}

	ai := &auth.Info{AccountID: "aid1"}
	params := ops.CspDomainCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
		Payload: &models.CSPDomain{
			CSPDomainAllOf0: models.CSPDomainAllOf0{
				CspDomainType:       dt,
				CspDomainAttributes: dtAttrs,
			},
			CSPDomainMutable: models.CSPDomainMutable{
				Name:            "cspDomain1",
				ManagementHost:  "f.q.d.n",
				CspCredentialID: "credID",
				StorageCosts:    scm,
			},
		},
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID:      "objectID",
				Version: 1,
			},
			AccountID:     "aid1",
			CspDomainType: params.Payload.CspDomainType,
			CspDomainAttributes: map[string]models.ValueType{
				"property1": {Kind: "STRING", Value: "string1"},
				"property2": {Kind: "INT", Value: "4"},
				"property3": {Kind: "SECRET", Value: "secret string"},
			},
		},
		CSPDomainMutable: models.CSPDomainMutable{
			Name:            params.Payload.Name,
			ManagementHost:  params.Payload.ManagementHost,
			CspCredentialID: "credID",
			StorageCosts:    scm,
		},
	}
	credAttrs := map[string]models.ValueType{
		"k1": models.ValueType{Kind: "STRING", Value: "v1"},
		"k2": models.ValueType{Kind: "SECRET", Value: "v2"},
	}
	credObj := &models.CSPCredential{
		CSPCredentialAllOf0: models.CSPCredentialAllOf0{
			Meta:      &models.ObjMeta{ID: "credID"},
			AccountID: "tid1",
		},
		CSPCredentialMutable: models.CSPCredentialMutable{
			CredentialAttributes: credAttrs,
		},
	}
	aObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta:            &models.ObjMeta{ID: "aid2"},
			TenantAccountID: "tid1",
		},
	}
	sysObj := &models.System{
		SystemAllOf0: models.SystemAllOf0{
			Meta: &models.ObjMeta{
				ID: "sys_id1",
			},
		},
		SystemMutable: models.SystemMutable{
			ClusterUsagePolicy: &models.ClusterUsagePolicy{
				AccountSecretScope: "GLOBAL",
			},
		},
	}
	gCtx := params.HTTPRequest.Context()
	// success, accountID from auth.Info, inherit
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	t.Log("case: success")
	assert.Nil(obj.ClusterUsagePolicy)
	mds := mock.NewMockDataStore(mockCtrl)
	oA := mock.NewMockAccountOps(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	oA.EXPECT().Fetch(ctx, "aid1").Return(aObj, nil)
	oCred := mock.NewMockCspCredentialOps(mockCtrl)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	oCred.EXPECT().Fetch(ctx, "credID").Return(credObj, nil)
	appCSP := mock.NewMockAppCloudServiceProvider(mockCtrl)
	app.AppCSP = appCSP
	appCSP.EXPECT().ValidateAndInitCspDomain(gCtx, params.Payload, credAttrs).Return(nil)
	appCSP.EXPECT().GetCspStorageType(models.CspStorageType(string(storageTypes[0].Name))).Return(storageTypes[0])
	oCSP := mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Create(ctx, params.Payload).Return(obj, nil)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	oSYS := mock.NewMockSystemOps(mockCtrl)
	oSYS.EXPECT().Fetch().Return(sysObj, nil)
	mds.EXPECT().OpsSystem().Return(oSYS).MinTimes(1)
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.cspDomainCreate(params) })
	assert.EqualValues("aid1", params.Payload.AccountID)
	assert.NotNil(ret)
	_, ok := ret.(*ops.CspDomainCreateCreated)
	assert.True(ok)
	assert.Len(evM.InSSProps, 1)
	assert.EqualValues("objectID", evM.InSSProps["meta.id"])
	assert.Equal(obj, evM.InACScope)
	assert.NotNil(obj.ClusterUsagePolicy)
	exp := &fal.Args{AI: ai, Action: centrald.CspDomainCreateAction, ObjID: "objectID", Name: params.Payload.Name, Message: "Created with management host[f.q.d.n]", RefID: "credID"}
	amSC := &fal.Args{AI: ai, Action: centrald.CspDomainUpdateAction, ObjID: "objectID", Name: params.Payload.Name, Message: fmt.Sprintf(stgCostSetFmt, string(storageTypes[0].Name), 1.0)}
	assert.Equal([]*fal.Args{exp, amSC}, fa.Posts)
	tl.Flush()

	obj.ClusterUsagePolicy.Inherited = false // disable inheritance for the remaining tests

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: success with no audit detail")
	fa.Posts = []*fal.Args{}
	params.Payload.ManagementHost = "   "
	obj.ManagementHost = ""
	appCSP = mock.NewMockAppCloudServiceProvider(mockCtrl)
	app.AppCSP = appCSP
	appCSP.EXPECT().ValidateAndInitCspDomain(gCtx, params.Payload, credAttrs).Return(nil)
	appCSP.EXPECT().GetCspStorageType(models.CspStorageType(string(storageTypes[0].Name))).Return(storageTypes[0])
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "aid1").Return(&models.Account{}, nil)
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, "credID").Return(credObj, nil)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Create(ctx, params.Payload).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	oSYS = mock.NewMockSystemOps(mockCtrl)
	oSYS.EXPECT().Fetch().Return(sysObj, nil)
	mds.EXPECT().OpsSystem().Return(oSYS).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainCreate(params) })
	assert.NotNil(ret)
	_, ok = ret.(*ops.CspDomainCreateCreated)
	exp.Message = "Created"
	assert.Equal([]*fal.Args{exp, amSC}, fa.Posts)
	assert.Empty(params.Payload.ManagementHost)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: StorageCost validation fails")
	mockCtrl = gomock.NewController(t)
	scm[string(storageTypes[0].Name)] = models.StorageCost{CostPerGiB: -1.0}
	appCSP = mock.NewMockAppCloudServiceProvider(mockCtrl)
	app.AppCSP = appCSP
	appCSP.EXPECT().GetCspStorageType(models.CspStorageType(string(storageTypes[0].Name))).Return(storageTypes[0])
	assert.NotPanics(func() { ret = hc.cspDomainCreate(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.CspDomainCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M+": storageCosts", *mE.Payload.Message)
	tl.Flush()
	mockCtrl.Finish()

	params.Payload.StorageCosts = nil // ignore cost for the rest of the test
	obj.StorageCosts = nil

	// Create failed, cover accountId present case
	t.Log("case: Create fails")
	mockCtrl = gomock.NewController(t)
	ai.AccountID = ""
	params.Payload.AccountID = "aid1"
	params.Payload.ManagementHost = "  f.q.d.n  "
	appCSP = mock.NewMockAppCloudServiceProvider(mockCtrl)
	app.AppCSP = appCSP
	appCSP.EXPECT().ValidateAndInitCspDomain(gCtx, params.Payload, credAttrs).Return(nil)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "aid1").Return(&models.Account{}, nil)
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, "credID").Return(credObj, nil)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Create(ctx, params.Payload).Return(nil, centrald.ErrorExists)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	oSYS = mock.NewMockSystemOps(mockCtrl)
	oSYS.EXPECT().Fetch().Return(sysObj, nil)
	mds.EXPECT().OpsSystem().Return(oSYS).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorExists.M, *mE.Payload.Message)
	assert.Equal("f.q.d.n", params.Payload.ManagementHost)
	tl.Flush()

	mockCtrl.Finish()
	t.Log("case: auditLog not ready")
	fa.ReadyRet = centrald.ErrorDbError
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainCreate(params) })
	mE, ok = ret.(*ops.CspDomainCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.cspDomainCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainCreateDefault)
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
	assert.NotPanics(func() { ret = hc.cspDomainCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainCreateDefault)
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
	assert.NotPanics(func() { ret = hc.cspDomainCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainCreateDefault)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M+": invalid accountId", *mE.Payload.Message)
	params.Payload.AccountID = ""
	tl.Flush()

	// ValidateAndInitCspDomain fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: ValidateAndInitCspDomain fails")
	ai.AccountID = "aid1"
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "aid1").Return(&models.Account{}, nil)
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, "credID").Return(credObj, nil)
	mds.EXPECT().OpsAccount().Return(oA)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	appCSP = mock.NewMockAppCloudServiceProvider(mockCtrl)
	app.AppCSP = appCSP
	appCSP.EXPECT().ValidateAndInitCspDomain(gCtx, params.Payload, credAttrs).Return(fmt.Errorf("validation failure"))
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainCreate(params) })
	assert.NotNil(ret)
	_, ok = ret.(*ops.CspDomainCreateCreated)
	assert.False(ok)
	mE, ok = ret.(*ops.CspDomainCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
	assert.Regexp("validation failure", *mE.Payload.Message)
	tl.Flush()

	// CspCredential not found
	mockCtrl.Finish()
	t.Log("case: cspCredential not found")
	mockCtrl = gomock.NewController(t)
	params.Payload.AccountID = "aid1"
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "aid1").Return(aObj, nil)
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, "credID").Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.cspDomainCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainCreateDefault)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorNotFound.M, *mE.Payload.Message)
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
	assert.NotPanics(func() { ret = hc.cspDomainCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainCreateDefault)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	exp = &fal.Args{AI: ai, Action: centrald.CspDomainCreateAction, Name: params.Payload.Name, Err: true, Message: "Create unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	params.Payload.AccountID = ""
	ai.RoleObj = nil
	tl.Flush()

	// System fetch fails, cover authorizedAccountId present case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: System fetch fails")
	params.Payload.AccountID = "aid1"
	params.Payload.AuthorizedAccounts = []models.ObjIDMutable{"aAid2"}
	appCSP = mock.NewMockAppCloudServiceProvider(mockCtrl)
	app.AppCSP = appCSP
	appCSP.EXPECT().ValidateAndInitCspDomain(gCtx, params.Payload, credAttrs).Return(nil)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "aid1").Return(&models.Account{}, nil)
	oA.EXPECT().Fetch(ctx, "aAid2").Return(&models.Account{}, nil)
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, "credID").Return(credObj, nil)
	oSYS = mock.NewMockSystemOps(mockCtrl)
	oSYS.EXPECT().Fetch().Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(2)
	mds.EXPECT().OpsSystem().Return(oSYS).MinTimes(1)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	tl.Flush()

	// wrong authorizedAccountId
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: wrong authorizedAccountId")
	fa.Posts = []*fal.Args{}
	ai.AccountID = "aid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	params.Payload.AccountID = "aid1"
	params.Payload.AuthorizedAccounts = []models.ObjIDMutable{"aAid2"}
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "aid1").Return(&models.Account{}, nil)
	aObj2 := &models.Account{}
	aObj2.Meta = &models.ObjMeta{ID: "aAid2"}
	aObj2.TenantAccountID = "tid1"
	oA.EXPECT().Fetch(ctx, "aAid2").Return(aObj2, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(2)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	exp.Message = "Create attempted with incorrect authorizedAccount [aAid2]"
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	tl.Flush()

	// authorized account fetch db error
	mockCtrl.Finish()
	t.Log("case: authorized account fetch db error")
	mockCtrl = gomock.NewController(t)
	params.Payload.AccountID = "aid1"
	params.Payload.AuthorizedAccounts = []models.ObjIDMutable{"aAid2"}
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "aid1").Return(&models.Account{}, nil)
	oA.EXPECT().Fetch(ctx, "aAid2").Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(2)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.cspDomainCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainCreateDefault)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	tl.Flush()

	// authorized account not found
	mockCtrl.Finish()
	t.Log("case: authorized account not found")
	mockCtrl = gomock.NewController(t)
	params.Payload.AccountID = "aid1"
	params.Payload.AuthorizedAccounts = []models.ObjIDMutable{"aAid2"}
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "aid1").Return(&models.Account{}, nil)
	oA.EXPECT().Fetch(ctx, "aAid2").Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(2)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.cspDomainCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainCreateDefault)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
	params.Payload.AccountID = ""
	params.Payload.AuthorizedAccounts = []models.ObjIDMutable{}
	tl.Flush()

	// missing property failure cases
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	tcs := []string{"name", "cspDomainType", "cspDomainAttributes"}
	for _, tc := range tcs {
		cl := *params.Payload // copy
		switch tc {
		case "name":
			cl.Name = ""
		case "cspDomainType":
			cl.CspDomainType = ""
		case "cspDomainAttributes":
			cl.CspDomainAttributes = nil
		default:
			assert.False(true)
		}
		t.Log("case: CSPDomainCreate missing error case: " + tc)
		assert.NotPanics(func() {
			ret = hc.cspDomainCreate(ops.CspDomainCreateParams{HTTPRequest: requestWithAuthContext(ai), Payload: &cl})
		})
		assert.NotNil(ret)
		_, ok = ret.(*ops.CspDomainCreateCreated)
		assert.False(ok)
		mE, ok := ret.(*ops.CspDomainCreateDefault)
		assert.True(ok)
		assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
		assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
		tl.Flush()
	}

	// no payload
	params.Payload = nil
	t.Log("case: nil payload")
	assert.NotPanics(func() { ret = hc.cspDomainCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorMissing.M, *mE.Payload.Message)
	assert.True(ok)
}

func TestCspDomainDelete(t *testing.T) {
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

	domID := "domID"
	usedDomID := "usedDomID"
	params := ops.CspDomainDeleteParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          domID,
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID(domID),
			},
			AccountID:     "aid1",
			CspDomainType: "AWS",
		},
		CSPDomainMutable: models.CSPDomainMutable{
			Name: "cspDomain",
		},
	}
	clusterParams := cluster.ClusterListParams{CspDomainID: &params.ID}
	aParams := account.AccountListParams{CspDomainID: &domID}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	t.Log("case: success")
	oCSP := mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Delete(ctx, params.ID).Return(nil)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oC := mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Count(ctx, clusterParams, uint(1)).Return(0, nil)
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Count(ctx, aParams, uint(1)).Return(0, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oC)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	var ret middleware.Responder
	cntLock := hc.cntLock
	cntUnlock := hc.cntUnlock
	assert.NotPanics(func() { ret = hc.cspDomainDelete(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.CspDomainDeleteNoContent)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Empty(evM.InSSProps)
	assert.Equal(obj, evM.InACScope)
	exp := &fal.Args{AI: ai, Action: centrald.CspDomainDeleteAction, ObjID: models.ObjID(params.ID), RefID: obj.CspCredentialID, Name: obj.Name, Message: "Deleted"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	tl.Flush()

	// delete failure case, cover valid tenant account
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: delete failure")
	ai.AccountID = "aid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Delete(ctx, params.ID).Return(centrald.ErrorDbError)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Count(ctx, clusterParams, uint(1)).Return(0, nil)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Count(ctx, aParams, uint(1)).Return(0, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oC)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.cspDomainDelete(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.CspDomainDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	ai.RoleObj = nil
	tl.Flush()

	// domain is referenced in Account SnapshotCatalogPolicy
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: domain is referenced in Account SnapshotCatalogPolicy")
	obj.Meta.ID = models.ObjID(usedDomID)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Count(ctx, aParams, uint(1)).Return(1, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.cspDomainDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Regexp("CSP domain is referenced in account SnapshotCatalogPolicy", *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	obj.Meta.ID = models.ObjID(domID)
	tl.Flush()

	// account count failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: account list failure")
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Count(ctx, aParams, uint(1)).Return(0, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: auditLog not ready")
	mds = mock.NewMockDataStore(mockCtrl)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsCspDomain().Return(oCSP)
	fa.ReadyRet = centrald.ErrorDbError
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainDelete(params) })
	mE, ok = ret.(*ops.CspDomainDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.cspDomainDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	// not authorized tenant admin
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized tenant admin")
	fa.Posts = []*fal.Args{}
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	mds = mock.NewMockDataStore(mockCtrl)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsCspDomain().Return(oCSP)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.cspDomainDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Regexp(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	exp = &fal.Args{AI: ai, Action: centrald.CspDomainDeleteAction, ObjID: models.ObjID(params.ID), Name: obj.Name, Err: true, Message: "Delete unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	ai.RoleObj = nil
	tl.Flush()

	// validation failures
	for tc := 0; tc < 2; tc++ {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		t.Log("case: cluster " + []string{"count fails", "exists"}[tc%2])
		mds = mock.NewMockDataStore(mockCtrl)
		oCSP = mock.NewMockCspDomainOps(mockCtrl)
		oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
		mds.EXPECT().OpsCspDomain().Return(oCSP)
		count, err := tc%2, []error{centrald.ErrorDbError, nil}[tc%2]
		oC = mock.NewMockClusterOps(mockCtrl)
		oC.EXPECT().Count(ctx, clusterParams, uint(1)).Return(count, err)
		mds.EXPECT().OpsCluster().Return(oC)
		oA = mock.NewMockAccountOps(mockCtrl)
		oA.EXPECT().Count(ctx, aParams, uint(1)).Return(0, nil)
		mds.EXPECT().OpsAccount().Return(oA)
		hc.DS = mds
		cntLock = hc.cntLock
		cntUnlock = hc.cntUnlock
		assert.NotPanics(func() { ret = hc.cspDomainDelete(params) })
		assert.NotNil(ret)
		mE, ok = ret.(*ops.CspDomainDeleteDefault)
		if assert.True(ok) {
			if tc%2 == 0 {
				assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
				assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
			} else {
				assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
				assert.Regexp("clusters .* the CSP domain", *mE.Payload.Message)
			}
		}
		assert.Equal(cntLock+1, hc.cntLock)
		assert.Equal(cntUnlock+1, hc.cntUnlock)
		tl.Flush()
	}

	// fetch failure case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: fetch failure")
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.cspDomainDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
	assert.Equal(cntLock, hc.cntLock)
	assert.Equal(cntUnlock, hc.cntUnlock)
}

func TestCspDomainFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.CspDomainFetchParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID: "objectID",
			},
			AccountID:     "aid1",
			CspDomainType: "AWS",
		},
		CSPDomainMutable: models.CSPDomainMutable{
			Name:            "cspDomain",
			CspCredentialID: "credID",
		},
	}
	sysObj := &models.System{
		SystemAllOf0: models.SystemAllOf0{
			Meta: &models.ObjMeta{
				ID: "sys_id1",
			},
		},
		SystemMutable: models.SystemMutable{
			ClusterUsagePolicy: &models.ClusterUsagePolicy{
				AccountSecretScope: "GLOBAL",
			},
		},
	}
	credObj := &models.CSPCredential{
		CSPCredentialAllOf0: models.CSPCredentialAllOf0{
			Meta:          &models.ObjMeta{ID: "credID"},
			AccountID:     "tid1",
			CspDomainType: "AWS",
		},
		CSPCredentialMutable: models.CSPCredentialMutable{
			CredentialAttributes: map[string]models.ValueType{
				"ca1": models.ValueType{Kind: "STRING", Value: "v1"},
				"ca2": models.ValueType{Kind: "SECRET", Value: "v2"},
			},
		},
	}

	// success (with inheritance)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	t.Log("case: success (with inheritance)")
	assert.Nil(obj.ClusterUsagePolicy)
	oCSP := mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCred := mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, "credID").Return(credObj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	oSYS := mock.NewMockSystemOps(mockCtrl)
	oSYS.EXPECT().Fetch().Return(sysObj, nil)
	mds.EXPECT().OpsSystem().Return(oSYS).MinTimes(1)
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.cspDomainFetch(params) })
	assert.NotNil(ret)
	mR, ok := ret.(*ops.CspDomainFetchOK)
	assert.True(ok)
	assert.Equal(obj, mR.Payload)
	assert.NotNil(obj.ClusterUsagePolicy)
	assert.True(obj.ClusterUsagePolicy.Inherited)
	obj.ClusterUsagePolicy.Inherited = false
	assert.Equal(sysObj.ClusterUsagePolicy, obj.ClusterUsagePolicy)
	tl.Flush()

	// success (without inheritance)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: success (no inheritance)")
	obj.ClusterUsagePolicy = &models.ClusterUsagePolicy{
		AccountSecretScope: "CSPDOMAIN",
	}
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, "credID").Return(credObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainFetch(params) })
	assert.NotNil(ret)
	mR, ok = ret.(*ops.CspDomainFetchOK)
	assert.True(ok)
	assert.Equal(obj, mR.Payload)
	assert.NotEqual(sysObj.ClusterUsagePolicy, obj.ClusterUsagePolicy)
	tl.Flush()

	// fetch failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: fetch failure")
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainFetch(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.CspDomainFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)

	// inheritance failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: inheritance failure")
	obj.ClusterUsagePolicy = &models.ClusterUsagePolicy{
		Inherited: true,
	}
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	oSYS = mock.NewMockSystemOps(mockCtrl)
	oSYS.EXPECT().Fetch().Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsSystem().Return(oSYS).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.cspDomainFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: unauthorized")
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP)
	hc.DS = mds
	ai.AccountID = "aid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: false}
	assert.NotPanics(func() { ret = hc.cspDomainFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
}

func TestCspDomainList(t *testing.T) {
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
	params := ops.CspDomainListParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	ctx := params.HTTPRequest.Context()
	objects := []*models.CSPDomain{
		&models.CSPDomain{
			CSPDomainAllOf0: models.CSPDomainAllOf0{
				Meta: &models.ObjMeta{
					ID: "objectID",
				},
				AccountID:     "tid1",
				CspDomainType: "AWS",
			},
			CSPDomainMutable: models.CSPDomainMutable{
				AuthorizedAccounts: []models.ObjIDMutable{"aid1"},
				Name:               "cspDomain",
				CspCredentialID:    "credID",
			},
		},
		&models.CSPDomain{
			CSPDomainAllOf0: models.CSPDomainAllOf0{
				Meta: &models.ObjMeta{
					ID: "objectID2",
				},
				AccountID:     "tid1",
				CspDomainType: "AWS",
			},
			CSPDomainMutable: models.CSPDomainMutable{
				AuthorizedAccounts: []models.ObjIDMutable{"aid2"},
				Name:               "cspDomain2",
				CspCredentialID:    "credID",
			},
		},
	}
	sysObj := &models.System{
		SystemAllOf0: models.SystemAllOf0{
			Meta: &models.ObjMeta{
				ID: "sys_id1",
			},
		},
		SystemMutable: models.SystemMutable{
			ClusterUsagePolicy: &models.ClusterUsagePolicy{
				AccountSecretScope: "GLOBAL",
			},
		},
	}
	credObj := &models.CSPCredential{
		CSPCredentialAllOf0: models.CSPCredentialAllOf0{
			Meta:          &models.ObjMeta{ID: "credID"},
			AccountID:     "tid1",
			CspDomainType: "AWS",
		},
		CSPCredentialMutable: models.CSPCredentialMutable{
			CredentialAttributes: map[string]models.ValueType{
				"ca1": models.ValueType{Kind: "STRING", Value: "v1"},
				"ca2": models.ValueType{Kind: "SECRET", Value: "v2"},
			},
		},
	}

	cloneObjects := func() []*models.CSPDomain {
		l := make([]*models.CSPDomain, len(objects))
		for i, o := range objects {
			testutils.Clone(o, &l[i])
		}
		return l
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	t.Log("case: success, internal user, credentials merged")
	fops.RetCspCredentialFetchObj = credObj
	oCSP := mock.NewMockCspDomainOps(mockCtrl)
	retObjects := cloneObjects()
	oCSP.EXPECT().List(ctx, params).Return(retObjects, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	oSYS := mock.NewMockSystemOps(mockCtrl)
	oSYS.EXPECT().Fetch().Return(sysObj, nil)
	mds.EXPECT().OpsSystem().Return(oSYS).MinTimes(1)
	hc.DS = mds
	assert.True(ai.Internal())
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.cspDomainList(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.CspDomainListOK)
	assert.True(ok)
	assert.Len(mO.Payload, len(objects))
	assert.Equal(retObjects, mO.Payload)
	for _, o := range mO.Payload {
		assert.NotNil(o.ClusterUsagePolicy)
		for k := range credObj.CredentialAttributes {
			assert.Containsf(o.CspDomainAttributes, k, "case cred[%s]", k)
		}
	}
	tl.Flush()
	mockCtrl.Finish()

	// credential lookup fails
	t.Log("case: success, internal user, cred lookup fails")
	fops.RetCspCredentialFetchErr = centrald.ErrorDbError
	mockCtrl = gomock.NewController(t)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	retObjects = cloneObjects()
	oCSP.EXPECT().List(ctx, params).Return(retObjects, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	oSYS = mock.NewMockSystemOps(mockCtrl)
	oSYS.EXPECT().Fetch().Return(sysObj, nil)
	mds.EXPECT().OpsSystem().Return(oSYS).MinTimes(1)
	hc.DS = mds
	assert.True(ai.Internal())
	assert.NotPanics(func() { ret = hc.cspDomainList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.CspDomainListOK)
	assert.True(ok)
	assert.Len(mO.Payload, 0)
	fops.RetCspCredentialFetchErr = nil
	tl.Flush()
	mockCtrl.Finish()

	// list fails
	t.Log("case: List failure")
	mockCtrl = gomock.NewController(t)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().List(ctx, params).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	oSYS = mock.NewMockSystemOps(mockCtrl)
	oSYS.EXPECT().Fetch().Return(sysObj, nil)
	mds.EXPECT().OpsSystem().Return(oSYS).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainList(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.CspDomainListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()
	mockCtrl.Finish()

	// system fails
	t.Log("case: System failure")
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oSYS = mock.NewMockSystemOps(mockCtrl)
	oSYS.EXPECT().Fetch().Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsSystem().Return(oSYS).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspDomainListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.cspDomainList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspDomainListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: aid2 account, credentials not merged")
	mockCtrl = gomock.NewController(t)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	retObjects = cloneObjects()
	oCSP.EXPECT().List(ctx, params).Return(retObjects, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP)
	oSYS = mock.NewMockSystemOps(mockCtrl)
	oSYS.EXPECT().Fetch().Return(sysObj, nil)
	mds.EXPECT().OpsSystem().Return(oSYS).MinTimes(1)
	hc.DS = mds
	ai.AccountID = "aid2"
	ai.TenantAccountID = "tid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	assert.NotPanics(func() { ret = hc.cspDomainList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.CspDomainListOK)
	assert.True(ok)
	assert.Len(mO.Payload, 1)
	for _, o := range mO.Payload {
		assert.Contains(o.AuthorizedAccounts, models.ObjIDMutable(ai.AccountID))
		for k := range credObj.CredentialAttributes {
			assert.NotContains(o.CspDomainAttributes, k)
		}
	}
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: internal user, no CspCredential - skip lookup")
	retObjects = cloneObjects()
	retObjects[0].CspCredentialID = ""
	retObjects[1].CspCredentialID = ""
	cnt := fops.CntCspCredentialFetch
	mockCtrl = gomock.NewController(t)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().List(ctx, params).Return(retObjects, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP)
	oSYS = mock.NewMockSystemOps(mockCtrl)
	oSYS.EXPECT().Fetch().Return(sysObj, nil)
	mds.EXPECT().OpsSystem().Return(oSYS).MinTimes(1)
	hc.DS = mds
	ai.AccountID = ""
	ai.RoleObj = nil
	assert.True(ai.Internal())
	assert.NotPanics(func() { ret = hc.cspDomainList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.CspDomainListOK)
	assert.True(ok)
	assert.Len(mO.Payload, len(objects))
	for _, o := range mO.Payload {
		for k := range credObj.CredentialAttributes {
			assert.NotContains(o.CspDomainAttributes, k)
		}
	}
	assert.Equal(cnt, fops.CntCspCredentialFetch)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: constrainBothQueryAccounts changes accounts")
	ai.AccountID = "tid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	params.AuthorizedAccountID, params.AccountID = swag.String("a1"), swag.String("t1")
	fops.RetConstrainBothQueryAccountsAID, fops.RetConstrainBothQueryAccountsTID = swag.String("aid1"), swag.String("tid1")
	cParams := params
	cParams.AuthorizedAccountID, cParams.AccountID = fops.RetConstrainBothQueryAccountsAID, fops.RetConstrainBothQueryAccountsTID
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().List(ctx, cParams).Return(retObjects, nil)
	mds.EXPECT().OpsCspDomain().Return(oCSP)
	oSYS = mock.NewMockSystemOps(mockCtrl)
	oSYS.EXPECT().Fetch().Return(sysObj, nil)
	mds.EXPECT().OpsSystem().Return(oSYS)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.CspDomainListOK)
	assert.True(ok)
	assert.Len(mO.Payload, len(retObjects))
	assert.Equal(7, fops.CntConstrainBothQueryAccounts)
	assert.Equal(params.AuthorizedAccountID, fops.InConstrainBothQueryAccountsAID)
	assert.Equal(params.AccountID, fops.InConstrainBothQueryAccountsTID)
	tl.Flush()

	t.Log("case: constrainBothQueryAccounts skip")
	fops.RetConstrainBothQueryAccountsSkip = true
	assert.NotPanics(func() { ret = hc.cspDomainList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.CspDomainListOK)
	assert.True(ok)
	assert.Empty(mO.Payload)
	assert.Equal(8, fops.CntConstrainBothQueryAccounts)
}

func TestCspDomainUpdate(t *testing.T) {
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
	assert.NotPanics(func() { nMap = hc.cspDomainMutableNameMap() })
	assert.Equal("authorizedAccounts", nMap.jName("AuthorizedAccounts"))
	assert.Equal("cspCredentialId", nMap.jName("CspCredentialID"))

	// build attrs from the domain type description
	cspDomainTypes := app.SupportedCspDomainTypes()
	assert.NotNil(cspDomainTypes)
	assert.NotEmpty(cspDomainTypes)
	dt := models.CspDomainTypeMutable("AWS")
	assert.Contains(cspDomainTypes, dt)
	dad := app.DomainAttrDescForType(dt)
	assert.NotEmpty(dad)
	dtAttrs := make(map[string]models.ValueType, len(dad))
	for da, desc := range dad {
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
	storageTypes := []*models.CSPStorageType{}
	for _, st := range app.SupportedCspStorageTypes() {
		if string(st.CspDomainType) == string(dt) {
			storageTypes = append(storageTypes, st)
		}
	}
	assert.True(len(storageTypes) > 0)

	// parse params
	objM := &models.CSPDomainMutable{
		Name:           "cspDomain",
		ManagementHost: "newHost",
		StorageCosts: map[string]models.StorageCost{
			string(storageTypes[0].Name): {CostPerGiB: 1.0},
		},
	}
	ai := &auth.Info{}
	params := ops.CspDomainUpdateParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
		Version:     swag.Int32(8),
		Remove:      []string{},
		Append:      []string{},
		Set:         []string{nMap.jName("Name"), nMap.jName("ManagementHost"), nMap.jName("StorageCosts")},
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
	obj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta:                meta,
			AccountID:           "aid1",
			CspDomainType:       dt,
			CspDomainAttributes: dtAttrs,
		},
		CSPDomainMutable: models.CSPDomainMutable{
			CspCredentialID: "credID",
			StorageCosts: map[string]models.StorageCost{
				string(storageTypes[0].Name): {CostPerGiB: 1.5},
			},
		},
	}
	aObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta:            &models.ObjMeta{ID: "aid1"},
			TenantAccountID: "tid1",
		},
	}
	aObj.Name = "aName"
	sysObj := &models.System{
		SystemAllOf0: models.SystemAllOf0{
			Meta: &models.ObjMeta{
				ID: "sys_id1",
			},
		},
		SystemMutable: models.SystemMutable{
			ClusterUsagePolicy: &models.ClusterUsagePolicy{
				AccountSecretScope: "GLOBAL",
			},
		},
	}
	credAttrs := map[string]models.ValueType{
		"ca1": models.ValueType{Kind: "STRING", Value: "v1"},
		"ca2": models.ValueType{Kind: "SECRET", Value: "v2"},
	}
	credObj := &models.CSPCredential{
		CSPCredentialAllOf0: models.CSPCredentialAllOf0{
			Meta:          &models.ObjMeta{ID: "credID"},
			AccountID:     "tid1",
			CspDomainType: "AWS",
		},
		CSPCredentialMutable: models.CSPCredentialMutable{
			CredentialAttributes: credAttrs,
		},
	}

	// success
	var fetchObj *models.CSPDomain
	testutils.Clone(obj, &fetchObj)
	fetchObj.CspDomainAttributes["aws_access_key_id"] = models.ValueType{Kind: "STRING", Value: "key_id"}
	fetchObj.CspDomainAttributes["aws_secret_access_key"] = models.ValueType{Kind: "SECRET", Value: "secret_access_key"}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	t.Log("case: success")
	oCSP := mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(fetchObj, nil)
	oCSP.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	oCred := mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, "credID").Return(credObj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	appCSP := mock.NewMockAppCloudServiceProvider(mockCtrl)
	app.AppCSP = appCSP
	appCSP.EXPECT().GetCspStorageType(models.CspStorageType(string(storageTypes[0].Name))).Return(storageTypes[0])
	oSYS := mock.NewMockSystemOps(mockCtrl)
	oSYS.EXPECT().Fetch().Return(sysObj, nil)
	mds.EXPECT().OpsSystem().Return(oSYS).MinTimes(1)
	hc.DS = mds
	var ret middleware.Responder
	cntRLock := hc.cntRLock
	cntRUnlock := hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.cspDomainUpdate(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.CspDomainUpdateOK)
	assert.True(ok)
	assert.Equal(cntRLock, hc.cntRLock)
	assert.Equal(cntRUnlock, hc.cntRUnlock)
	assert.Equal(obj, mO.Payload)
	assert.Empty(evM.InSSProps)
	assert.Equal(obj, evM.InACScope)
	exp := &fal.Args{AI: ai, Action: centrald.CspDomainUpdateAction, ObjID: models.ObjID(params.ID), Name: obj.Name, Message: "Updated management host[newHost]"}
	assert.Equal([]*fal.Args{exp}, fa.Events)
	exp = &fal.Args{AI: ai, Action: centrald.CspDomainUpdateAction, ObjID: models.ObjID(params.ID), Name: obj.Name, Message: fmt.Sprintf(stgCostChgFmt, string(storageTypes[0].Name), 1.5, 1.0)}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	obj.CspDomainAttributes = nil
	obj.Meta = meta
	tl.Flush()

	mockCtrl.Finish()
	testutils.Clone(obj, &fetchObj)
	fa.Events = []*fal.Args{}
	fa.Posts = []*fal.Args{}
	// updating CspCredentialID
	t.Log("case: updating CspCredentialID")
	mockCtrl = gomock.NewController(t)
	obj.CspCredentialID = "NEW_credID"
	params.Set = []string{"cspCredentialId"}
	params.Payload.CspCredentialID = "NEW_credID"
	uP[centrald.UpdateSet] = params.Set
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).Matcher()
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(fetchObj, nil)
	oCSP.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, "NEW_credID").Return(credObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	oSYS = mock.NewMockSystemOps(mockCtrl)
	oSYS.EXPECT().Fetch().Return(sysObj, nil)
	mds.EXPECT().OpsSystem().Return(oSYS).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainUpdate(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.CspDomainUpdateOK)
	assert.True(ok)
	assert.Equal(cntRLock, hc.cntRLock)
	assert.Equal(cntRUnlock, hc.cntRUnlock)
	assert.Equal(obj, mO.Payload)
	assert.Empty(evM.InSSProps)
	assert.Equal(obj, evM.InACScope)
	exp = &fal.Args{AI: ai, Action: centrald.CspDomainUpdateAction, ObjID: models.ObjID(params.ID), Name: obj.Name, Message: "Updated CspCredentialID NEW_credID", RefID: "NEW_credID"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	obj.CspCredentialID = "credID"
	tl.Flush()

	mockCtrl.Finish()
	testutils.Clone(obj, &fetchObj)
	fa.Events = []*fal.Args{}
	fa.Posts = []*fal.Args{}
	// updating CspCredentialID with the same CspCredentialID
	t.Log("case: updating CspCredentialID with the same CspCredentialID")
	mockCtrl = gomock.NewController(t)
	obj.CspCredentialID = "credID"
	params.Set = []string{"cspCredentialId"}
	params.Payload.CspCredentialID = "credID"
	uP[centrald.UpdateSet] = params.Set
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).Matcher()
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(fetchObj, nil)
	oCSP.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, "credID").Return(credObj, nil).MinTimes(2)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	mds.EXPECT().OpsCspCredential().Return(oCred).MinTimes(2)
	oSYS = mock.NewMockSystemOps(mockCtrl)
	oSYS.EXPECT().Fetch().Return(sysObj, nil)
	mds.EXPECT().OpsSystem().Return(oSYS).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainUpdate(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.CspDomainUpdateOK)
	assert.True(ok)
	assert.Equal(cntRLock, hc.cntRLock)
	assert.Equal(cntRUnlock, hc.cntRUnlock)
	assert.Equal(obj, mO.Payload)
	assert.Empty(evM.InSSProps)
	assert.Equal(obj, evM.InACScope)
	assert.Empty(fa.Posts)
	assert.Empty(fa.Events)
	obj.CspCredentialID = "credID"
	tl.Flush()

	mockCtrl.Finish()
	fa.Posts = []*fal.Args{}
	t.Log("case: auditLog not ready")
	params.Set = []string{nMap.jName("Name"), nMap.jName("ManagementHost")}
	uP[centrald.UpdateSet] = params.Set
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).Matcher()
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, "credID").Return(credObj, nil)
	mds.EXPECT().OpsCspDomain().Return(oCSP)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	fa.ReadyRet = centrald.ErrorDbError
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainUpdate(params) })
	mD, ok := ret.(*ops.CspDomainUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.cspDomainUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspDomainUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized tenant admin")
	fa.Events = []*fal.Args{}
	fa.Posts = []*fal.Args{}
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	mds = mock.NewMockDataStore(mockCtrl)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsCspDomain().Return(oCSP)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspDomainUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)
	exp = &fal.Args{AI: ai, Action: centrald.CspDomainUpdateAction, ObjID: models.ObjID(params.ID), Name: obj.Name, Err: true, Message: "Update unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	ai.RoleObj = nil
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: fetch failure")
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspDomainUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()

	// Update failed, also tests no version param paths
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: failed on Update")
	params.Version = nil
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).AddsVersion(int32(obj.Meta.Version)).Matcher()
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCSP.EXPECT().Update(ctx, uaM, params.Payload).Return(nil, centrald.ErrorDbError)
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, "credID").Return(credObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	oSYS = mock.NewMockSystemOps(mockCtrl)
	oSYS.EXPECT().Fetch().Return(sysObj, nil)
	mds.EXPECT().OpsSystem().Return(oSYS).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspDomainUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	obj.Meta = meta
	tl.Flush()

	mockCtrl.Finish()
	t.Log("case: invalid storage costs")
	params.Append = nil
	params.Remove = nil
	params.Set = []string{nMap.jName("StorageCosts")}
	params.Payload.StorageCosts = map[string]models.StorageCost{"foo": {}}
	mockCtrl = gomock.NewController(t)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, "credID").Return(credObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	appCSP = mock.NewMockAppCloudServiceProvider(mockCtrl)
	app.AppCSP = appCSP
	appCSP.EXPECT().GetCspStorageType(models.CspStorageType("foo")).Return(nil)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspDomainUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	assert.Regexp("storageCosts.*invalid storage type", *mD.Payload.Message)

	mockCtrl.Finish()
	tl.Flush()
	t.Log("case: missing storage costs")
	params.Append = nil
	params.Remove = nil
	params.Set = []string{nMap.jName("StorageCosts") + ".notThere"}
	mockCtrl = gomock.NewController(t)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, "credID").Return(credObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspDomainUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	assert.Regexp("set='storageCosts.notThere'", *mD.Payload.Message)

	// version pre-check fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: version pre-check fails")
	params.Set = []string{nMap.jName("Name")}
	params.Version = swag.Int32(8)
	oVersion := obj.Meta.Version
	obj.Meta.Version++
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, "credID").Return(credObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspDomainUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorIDVerNotFound.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorIDVerNotFound.M, *mD.Payload.Message)
	obj.Meta.Version = oVersion
	tl.Flush()

	// System fetch failed, cover AuthorizedAccounts remove case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: System fetch failed, cover AuthorizedAccounts remove case")
	params.Set = []string{nMap.jName("Name")}
	params.Append = []string{}
	params.Remove = []string{"authorizedAccounts"}
	params.Payload.AuthorizedAccounts = []models.ObjIDMutable{"aid1"}
	uP[centrald.UpdateSet] = params.Set
	uP[centrald.UpdateAppend] = params.Append
	uP[centrald.UpdateRemove] = params.Remove
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, "credID").Return(credObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	oSYS = mock.NewMockSystemOps(mockCtrl)
	oSYS.EXPECT().Fetch().Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsSystem().Return(oSYS).MinTimes(1)
	hc.DS = mds
	fv := &fakeAuthAccountValidator{}
	hc.authAccountValidator = fv
	assert.NotPanics(func() { ret = hc.cspDomainUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspDomainUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	assert.Equal(centrald.CspDomainUpdateAction, fv.inAction)
	assert.Equal(params.ID, fv.inObjID)
	assert.Equal(obj.Name, fv.inObjName)
	assert.Empty(fv.inOldIDList)
	assert.Equal(params.Payload.AuthorizedAccounts, fv.inUpdateIDList)
	tl.Flush()

	// CspCredential fetch failed
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: CspCredential fetch failed")
	params.Set = []string{nMap.jName("Name")}
	params.Append = []string{}
	params.Remove = []string{}
	params.Payload.AuthorizedAccounts = []models.ObjIDMutable{"aid1"}
	uP[centrald.UpdateSet] = params.Set
	uP[centrald.UpdateAppend] = params.Append
	uP[centrald.UpdateRemove] = params.Remove
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, "credID").Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspDomainUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()

	// append authorizedAccounts with fake validator
	mockCtrl.Finish()
	t.Log("case: append authorizedAccounts")
	mockCtrl = gomock.NewController(t)
	fa.Posts = []*fal.Args{}
	ai.AccountID = "aid1" // cover valid tenant admin case
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	params.Set = []string{"managementHost"}
	params.Append = []string{"authorizedAccounts"}
	params.Remove = []string{}
	params.Payload.AuthorizedAccounts = []models.ObjIDMutable{"aid1"}
	obj.ManagementHost = params.Payload.ManagementHost
	params.Payload.ManagementHost = "  " + params.Payload.ManagementHost + "  "
	uP[centrald.UpdateSet] = params.Set
	uP[centrald.UpdateAppend] = params.Append
	uP[centrald.UpdateRemove] = params.Remove
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCSP.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	oSYS = mock.NewMockSystemOps(mockCtrl)
	oSYS.EXPECT().Fetch().Return(sysObj, nil)
	mds.EXPECT().OpsSystem().Return(oSYS).MinTimes(1)
	hc.DS = mds
	fv = &fakeAuthAccountValidator{retString: "aa added"}
	hc.authAccountValidator = fv
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.cspDomainUpdate(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.CspDomainUpdateOK)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Equal(obj, mO.Payload)
	exp = &fal.Args{AI: ai, Action: centrald.CspDomainUpdateAction, ObjID: models.ObjID(params.ID), Name: obj.Name, Message: "Updated authorizedAccounts aa added"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	assert.Empty(fa.Events)
	assert.Equal(centrald.CspDomainUpdateAction, fv.inAction)
	assert.Equal(params.ID, fv.inObjID)
	assert.Equal(obj.Name, fv.inObjName)
	assert.Empty(fv.inOldIDList)
	assert.Equal(params.Payload.AuthorizedAccounts, fv.inUpdateIDList)
	tl.Flush()

	// validateAuthorizedAccountsUpdate error (cover authorizedAccounts set case)
	mockCtrl.Finish()
	t.Log("case: append authorizedAccounts, validateAuthorizedAccountsUpdate error")
	mockCtrl = gomock.NewController(t)
	params.Append = []string{}
	params.Remove = []string{}
	params.Set = []string{"authorizedAccounts"}
	params.Payload.AuthorizedAccounts = []models.ObjIDMutable{"aid1"}
	uP[centrald.UpdateAppend] = params.Append
	uP[centrald.UpdateRemove] = params.Remove
	uP[centrald.UpdateSet] = params.Set
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP)
	hc.DS = mds
	fv = &fakeAuthAccountValidator{retError: fmt.Errorf("aa error")}
	hc.authAccountValidator = fv
	assert.NotPanics(func() { ret = hc.cspDomainUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspDomainUpdateDefault)
	assert.True(ok)
	assert.Equal(http.StatusInternalServerError, int(mD.Payload.Code))
	assert.Equal("aa error", *mD.Payload.Message)
	tl.Flush()

	// non-existing CspCredential
	mockCtrl.Finish()
	t.Log("case: cspCredential not found")
	mockCtrl = gomock.NewController(t)
	params.Set = []string{"cspCredentialId"}
	params.Payload.CspCredentialID = "BAD_ID"
	oCred = mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, "BAD_ID").Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspDomainUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M+": invalid cspCredentialId", *mD.Payload.Message)
	tl.Flush()

	// no changes requested
	params.Set = []string{}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	t.Log("case: CspDomainUpdate no change")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspDomainUpdateDefault)
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
	assert.NotPanics(func() { ret = hc.cspDomainUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspDomainUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	tl.Flush()

	// invalid CUP
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: invalid clusterUsagePolicy")
	params.Set = []string{"clusterUsagePolicy"}
	params.Payload.ClusterUsagePolicy = nil
	mds = mock.NewMockDataStore(mockCtrl)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspDomainUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	tl.Flush()

	// no payload
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: no payload")
	mds = mock.NewMockDataStore(mockCtrl)
	params.Set = []string{"clusterUsagePolicy"}
	params.Payload = nil
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.CspDomainUpdateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
		assert.Regexp(centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
		assert.Regexp("missing payload", *mD.Payload.Message)
	}
}

func TestCspDomainDeploymentFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.CspDomainDeploymentFetchParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID: "objectID",
			},
			CspDomainType: "AWS",
		},
		CSPDomainMutable: models.CSPDomainMutable{
			ManagementHost: "managementHost",
		},
	}
	sysObj := &models.System{
		SystemAllOf0: models.SystemAllOf0{
			Meta: &models.ObjMeta{
				ID: "systemID",
			},
		},
		SystemMutable: models.SystemMutable{
			Name: "system",
		},
	}
	args := &clPkg.MCDeploymentArgs{
		SystemID:       string(sysObj.Meta.ID),
		CSPDomainID:    string(obj.Meta.ID),
		CSPDomainType:  string(obj.CspDomainType),
		ManagementHost: obj.ManagementHost,
	}
	dep := &clPkg.Deployment{
		Format:     "yaml",
		Deployment: "data",
	}

	// success (default no clusterType)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oCSP := mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oSYS := mock.NewMockSystemOps(mockCtrl)
	oSYS.EXPECT().Fetch().Return(sysObj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	mds.EXPECT().OpsSystem().Return(oSYS)
	appCSP := mock.NewMockAppCloudServiceProvider(mockCtrl)
	appCSP.EXPECT().GetMCDeployment(args).Return(dep, nil).MinTimes(1)
	t.Log("case: cspDomainDeploymentFetch success")
	tl.Flush()
	hc.DS = mds
	app.AppCSP = appCSP
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.cspDomainDeploymentFetch(params) })
	assert.NotNil(ret)
	mR, ok := ret.(*ops.CspDomainDeploymentFetchOK)
	assert.True(ok)
	assert.Equal(dep.Format, mR.Payload.Format)
	assert.Equal(dep.Deployment, mR.Payload.Deployment)

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.cspDomainDeploymentFetch(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.CspDomainDeploymentFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: unauthorized")
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP)
	hc.DS = mds
	ai.AccountID = "aid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: false}
	assert.NotPanics(func() { ret = hc.cspDomainDeploymentFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainDeploymentFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	ai.RoleObj = nil
	tl.Flush()

	// deployment failure (specify clusterType and name)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	params.ClusterType = swag.String("fooClusterType")
	params.Name = swag.String("customName")
	args.ClusterType = *params.ClusterType
	args.ClusterName = *params.Name
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oSYS = mock.NewMockSystemOps(mockCtrl)
	oSYS.EXPECT().Fetch().Return(sysObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	mds.EXPECT().OpsSystem().Return(oSYS)
	appCSP = mock.NewMockAppCloudServiceProvider(mockCtrl)
	appCSP.EXPECT().GetMCDeployment(args).Return(nil, fmt.Errorf("fake error")).MinTimes(1)
	oCluster := mock.NewMockClusterOps(mockCtrl)
	oCluster.EXPECT().Count(ctx, cluster.ClusterListParams{CspDomainID: &params.ID, Name: params.Name}, uint(1)).Return(0, nil)
	mds.EXPECT().OpsCluster().Return(oCluster)
	t.Log("case: cspDomainDeploymentFetch deployment failure")
	tl.Flush()
	hc.DS = mds
	app.AppCSP = appCSP
	assert.NotPanics(func() { ret = hc.cspDomainDeploymentFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainDeploymentFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInvalidData.C, int(mE.Payload.Code))
	assert.Regexp(centrald.ErrorInvalidData.M, *mE.Payload.Message)

	// deployment success (specify clusterType and name); clustername exists
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	params.ClusterType = swag.String("fooClusterType")
	params.Name = swag.String("customName")
	params.Force = swag.Bool(true)
	args.ClusterType = *params.ClusterType
	args.ClusterName = *params.Name
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oSYS = mock.NewMockSystemOps(mockCtrl)
	oSYS.EXPECT().Fetch().Return(sysObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	mds.EXPECT().OpsSystem().Return(oSYS)
	appCSP = mock.NewMockAppCloudServiceProvider(mockCtrl)
	appCSP.EXPECT().GetMCDeployment(args).Return(dep, nil).MinTimes(1)
	t.Log("case: cspDomainDeploymentFetch deployment success while clustername exists")
	tl.Flush()
	hc.DS = mds
	app.AppCSP = appCSP
	assert.NotPanics(func() { ret = hc.cspDomainDeploymentFetch(params) })
	assert.NotNil(ret)
	mR, ok = ret.(*ops.CspDomainDeploymentFetchOK)
	assert.True(ok)
	assert.Equal(dep.Format, mR.Payload.Format)
	assert.Equal(dep.Deployment, mR.Payload.Deployment)

	// ClusterName exists
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: cluster name exists")
	tl.Flush()
	params.Force = swag.Bool(false)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	appCSP = mock.NewMockAppCloudServiceProvider(mockCtrl)
	oCluster = mock.NewMockClusterOps(mockCtrl)
	oCluster.EXPECT().Count(ctx, cluster.ClusterListParams{CspDomainID: &params.ID, Name: params.Name}, uint(1)).Return(1, nil)
	mds.EXPECT().OpsCluster().Return(oCluster)
	hc.DS = mds
	app.AppCSP = appCSP
	assert.NotPanics(func() { ret = hc.cspDomainDeploymentFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainDeploymentFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorExists.M, *mE.Payload.Message)

	// deployment failure (system fetch fails)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	params.Name = nil
	params.ClusterType = nil
	args.ClusterType = ""
	args.ClusterName = ""
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oSYS = mock.NewMockSystemOps(mockCtrl)
	oSYS.EXPECT().Fetch().Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	mds.EXPECT().OpsSystem().Return(oSYS)
	t.Log("case: system fetch error")
	tl.Flush()
	hc.DS = mds
	app.AppCSP = appCSP
	assert.NotPanics(func() { ret = hc.cspDomainDeploymentFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainDeploymentFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Regexp(centrald.ErrorDbError.M, *mE.Payload.Message)

	// managementHost not set
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: managementHost not set")
	tl.Flush()
	obj.ManagementHost = ""
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP)
	appCSP = mock.NewMockAppCloudServiceProvider(mockCtrl)
	hc.DS = mds
	app.AppCSP = appCSP
	assert.NotPanics(func() { ret = hc.cspDomainDeploymentFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainDeploymentFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInvalidData.C, int(mE.Payload.Code))
	assert.Regexp(centrald.ErrorInvalidData.M+".*managementHost", *mE.Payload.Message)

	// fetch failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	t.Log("case: cspDomainDeploymentFetch fetch failure")
	tl.Flush()
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.cspDomainDeploymentFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.CspDomainDeploymentFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
}

func TestCspMetadata(t *testing.T) {
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
	md := &models.CSPDomainMetadata{}
	md.AttributeMetadata = app.DomainAttrDescForType(dt)
	assert.NotEmpty(md.AttributeMetadata)
	retCDM := &ops.CspDomainMetadataOK{
		Payload: md,
	}
	params := ops.CspDomainMetadataParams{
		CspDomainType: string(dt),
	}

	// success case
	rc := hc.cspDomainMetadata(params)
	assert.NotNil(rc)
	ret, ok := rc.(*ops.CspDomainMetadataOK)
	assert.True(ok)
	assert.Equal(retCDM, ret)

	// failure case
	params.CspDomainType += "foo"
	rc = hc.cspDomainMetadata(params)
	assert.NotNil(rc)
	retErr, ok := rc.(*ops.CspDomainMetadataDefault)
	assert.True(ok)
	assert.EqualValues(400, retErr.Payload.Code)
	assert.Regexp("invalid cspDomainType", *retErr.Payload.Message)
}

func TestCspDomainServicePlanCost(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ctx := context.Background()

	fOps := &fakeOps{}
	hc.ops = fOps

	ai := &auth.Info{}
	domObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID: "domID",
			},
			CspDomainType: "AWS",
		},
		CSPDomainMutable: models.CSPDomainMutable{
			ManagementHost: "managementHost",
		},
	}
	spObj := &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("spID"),
			},
		},
	}
	params := ops.CspDomainServicePlanCostParams{
		ID:            "domID",
		ServicePlanID: "spID",
	}

	t.Log("case: missing auth")
	params.HTTPRequest = &http.Request{}
	ret := hc.cspDomainServicePlanCost(params)
	mD, ok := ret.(*ops.CspDomainServicePlanCostDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)

	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()

	t.Log("case: dom fetch failure")
	fOps.RetCspDomainFetchObj = nil
	fOps.RetCspDomainFetchErr = centrald.ErrorNotFound
	ret = hc.cspDomainServicePlanCost(params)
	mD, ok = ret.(*ops.CspDomainServicePlanCostDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorNotFound.M, *mD.Payload.Message)

	fOps.RetCspDomainFetchObj = domObj
	fOps.RetCspDomainFetchErr = nil

	t.Log("case: sp fetch failure")
	fOps.RetServicePlanFetchObj = nil
	fOps.RetServicePlanFetchErr = centrald.ErrorNotFound
	ret = hc.cspDomainServicePlanCost(params)
	mD, ok = ret.(*ops.CspDomainServicePlanCostDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorNotFound.M, *mD.Payload.Message)

	fOps.RetServicePlanFetchObj = spObj
	fOps.RetServicePlanFetchErr = nil

	t.Log("case: DomainServicePlanCost failure")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	appCSP := mock.NewMockAppCloudServiceProvider(mockCtrl)
	app.AppCSP = appCSP
	appCSP.EXPECT().DomainServicePlanCost(ctx, domObj, spObj).Return(nil, fmt.Errorf("spc-error"))
	ret = hc.cspDomainServicePlanCost(params)
	mD, ok = ret.(*ops.CspDomainServicePlanCostDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorNotFound.M, *mD.Payload.Message)
	assert.Regexp("spc-error", *mD.Payload.Message)
	mockCtrl.Finish()
	tl.Flush()

	t.Log("case: DomainServicePlanCost success")
	mockCtrl = gomock.NewController(t)
	appCSP = mock.NewMockAppCloudServiceProvider(mockCtrl)
	app.AppCSP = appCSP
	spCost := &models.ServicePlanCost{}
	appCSP.EXPECT().DomainServicePlanCost(ctx, domObj, spObj).Return(spCost, nil)
	ret = hc.cspDomainServicePlanCost(params)
	mOK, ok := ret.(*ops.CspDomainServicePlanCostOK)
	assert.True(ok)
	assert.Equal(spCost, mOK.Payload)
}
