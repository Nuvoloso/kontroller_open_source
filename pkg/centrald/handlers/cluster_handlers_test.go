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
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/cluster"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/node"
	spa "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/service_plan_allocation"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	fal "github.com/Nuvoloso/kontroller/pkg/centrald/audit/fake"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	mockcluster "github.com/Nuvoloso/kontroller/pkg/cluster/mock"
	"github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestClusterFetchFilter(t *testing.T) {
	assert := assert.New(t)
	hc := newHandlerComp()
	obj := &models.Cluster{
		ClusterAllOf0:     models.ClusterAllOf0{Meta: &models.ObjMeta{ID: "id1"}},
		ClusterCreateOnce: models.ClusterCreateOnce{AccountID: "tid1"},
		ClusterMutable: models.ClusterMutable{
			ClusterCreateMutable: models.ClusterCreateMutable{
				AuthorizedAccounts: []models.ObjIDMutable{"aid1", "aid2"},
			},
		},
	}
	rObj := &models.Role{}
	rObj.Capabilities = map[string]bool{}
	ai := &auth.Info{}

	t.Log("case: internal user")
	assert.NoError(hc.clusterFetchFilter(ai, obj))

	t.Log("case: SystemManagementCap capability fails")
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.SystemManagementCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.clusterFetchFilter(ai, obj))

	t.Log("case: CSPDomainManagementCap capability correct tenant")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	ai.AccountID = "tid1"
	assert.NoError(hc.clusterFetchFilter(ai, obj))

	t.Log("case: CSPDomainUsageCap capability wrong tenant")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	ai.AccountID = "tid2"
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.clusterFetchFilter(ai, obj))

	t.Log("case: authorized account")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	ai.AccountID = "aid2"
	assert.NoError(hc.clusterFetchFilter(ai, obj))

	t.Log("case: not authorized account")
	ai.AccountID = "aid9"
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.clusterFetchFilter(ai, obj))

	t.Log("case: no authorized accounts")
	obj.AuthorizedAccounts = nil
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.clusterFetchFilter(ai, obj))
}

func TestClusterCreate(t *testing.T) {
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
	fOps := &fakeOps{}
	hc.ops = fOps
	ai := &auth.Info{}
	params := ops.ClusterCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
		Payload:     &models.ClusterCreateArgs{},
	}
	params.Payload.AuthorizedAccounts = []models.ObjIDMutable{"aid1", "aid2"}
	params.Payload.ClusterType = "kubernetes"
	params.Payload.ClusterIdentifier = "myCluster"
	params.Payload.CspDomainID = "cspObjectID"
	params.Payload.State = common.ClusterStateDeployable
	params.Payload.Name = "cluster1"
	params.Payload.Description = "a cluster"
	params.Payload.ClusterAttributes = map[string]models.ValueType{
		"attr": models.ValueType{Kind: "STRING", Value: "foo"},
	}
	ctx := params.HTTPRequest.Context()
	var createArg *models.ClusterCreateArgs
	testutils.Clone(params.Payload, &createArg)
	createArg.AccountID = "aid1"
	obj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID:      "objectID",
				Version: 1,
			},
		},
	}
	obj.Name = params.Payload.Name
	cspObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID(params.Payload.CspDomainID),
			},
			AccountID:     "aid1",
			CspDomainType: "AWS",
		},
		CSPDomainMutable: models.CSPDomainMutable{
			Name: "cspDomain",
			ClusterUsagePolicy: &models.ClusterUsagePolicy{
				AccountSecretScope: "CLUSTER",
			},
		},
	}
	aObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{ID: "aid1"},
		},
	}
	aObj.Name = "aName"
	aObj2 := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta:            &models.ObjMeta{ID: "aid2"},
			TenantAccountID: "aid1",
		},
	}
	aObj2.Name = "a2Name"

	// success (internal user)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	t.Log("case: success, internal role")
	assert.Nil(obj.ClusterUsagePolicy)
	oC := mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Create(ctx, createArg).Return(obj, nil)
	fOps.RetCspDomainFetchObj = cspObj
	fOps.RetCspDomainFetchErr = nil
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "aid1").Return(aObj, nil).Times(2)
	oA.EXPECT().Fetch(ctx, "aid2").Return(aObj2, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	var ret middleware.Responder
	cntRLock := hc.cntRLock
	cntRUnlock := hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.clusterCreate(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.ClusterCreateCreated)
	assert.True(ok)
	assert.Len(evM.InSSProps, 1)
	assert.EqualValues("objectID", evM.InSSProps["meta.id"])
	assert.Equal(obj, evM.InACScope)
	assert.NotNil(obj.ClusterUsagePolicy)
	assert.Equal(ai, fOps.InCspDomainFetchAi)
	assert.EqualValues(params.Payload.CspDomainID, fOps.InCspDomainFetchID)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	exp := &fal.Args{AI: ai, Action: centrald.ClusterCreateAction, ObjID: "objectID", Name: params.Payload.Name, Message: "Created"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	assert.Equal("aid1", ai.AccountID)
	assert.Equal("aName", ai.AccountName)
	*fa = fal.AuditLog{}
	tl.Flush()

	// Create failed - object exists
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: Create fails because object exists")
	params.Payload.AuthorizedAccounts = nil
	createArg.AuthorizedAccounts = nil
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Create(ctx, createArg).Return(nil, centrald.ErrorExists)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.clusterCreate(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.ClusterCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorExists.M, *mE.Payload.Message)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	tl.Flush()

	mockCtrl.Finish()
	t.Log("case: auditLog not ready")
	fa.ReadyRet = centrald.ErrorDbError
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.clusterCreate(params) })
	mE, ok = ret.(*ops.ClusterCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.clusterCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	mockCtrl = gomock.NewController(t)
	t.Log("case: Create fails because cspDomainID db error")
	fOps.RetCspDomainFetchObj = nil
	fOps.RetCspDomainFetchErr = centrald.ErrorDbError
	mds = mock.NewMockDataStore(mockCtrl)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.clusterCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	tl.Flush()

	// Create failed - csp domain object not found
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: Create fails because cspDomainID not found")
	fOps.RetCspDomainFetchObj = nil
	fOps.RetCspDomainFetchErr = centrald.ErrorNotFound
	mds = mock.NewMockDataStore(mockCtrl)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.clusterCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
	assert.Regexp("cspDomainId$", *mE.Payload.Message)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	tl.Flush()

	// Invalid state
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: invalid state")
	params.Payload.State = "INVALID STATE"
	assert.NotPanics(func() { ret = hc.clusterCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
	assert.Regexp("invalid state", *mE.Payload.Message)
	tl.Flush()

	// not authorized tenant admin
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized tenant admin")
	params.Payload.State = ""
	params.Payload.ClusterIdentifier = ""
	fa.Posts = []*fal.Args{}
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	fOps.RetCspDomainFetchObj = cspObj
	fOps.RetCspDomainFetchErr = nil
	mds = mock.NewMockDataStore(mockCtrl)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.clusterCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterCreateDefault)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	exp = &fal.Args{AI: ai, Action: centrald.ClusterCreateAction, Name: params.Payload.Name, Err: true, Message: "Create unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	ai.RoleObj = nil
	tl.Flush()

	mockCtrl.Finish()
	t.Log("case: Create fails because account fetch db error")
	params.Payload.State = ""
	params.Payload.ClusterIdentifier = ""
	mockCtrl = gomock.NewController(t)
	ai.AccountID = ""
	fOps.RetCspDomainFetchObj = cspObj
	fOps.RetCspDomainFetchErr = nil
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "aid1").Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.clusterCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	ai.AccountID = "aid1"
	tl.Flush()

	mockCtrl.Finish()
	t.Log("case: Create fails because authorized account fetch db error")
	params.Payload.State = ""
	params.Payload.ClusterIdentifier = ""
	mockCtrl = gomock.NewController(t)
	ai.AccountID = "aid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	params.Payload.AuthorizedAccounts = []models.ObjIDMutable{"aid1"}
	fOps.RetCspDomainFetchObj = cspObj
	fOps.RetCspDomainFetchErr = nil
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "aid1").Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.clusterCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	ai.RoleObj = nil
	tl.Flush()

	mockCtrl.Finish()
	t.Log("case: Create fails because authorized account not found")
	mockCtrl = gomock.NewController(t)
	fOps.RetCspDomainFetchObj = cspObj
	fOps.RetCspDomainFetchErr = nil
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "aid1").Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.clusterCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	tl.Flush()

	// Create failed - not subordinate account
	mockCtrl.Finish()
	t.Log("case: not subordinate account")
	prevTID := aObj2.TenantAccountID
	aObj2.TenantAccountID = "foo"
	params.Payload.State = ""
	params.Payload.ClusterIdentifier = ""
	mockCtrl = gomock.NewController(t)
	fa.Posts = []*fal.Args{}
	ai.AccountID = string(cspObj.AccountID)
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	params.Payload.AuthorizedAccounts = []models.ObjIDMutable{"aid2"}
	fOps.RetCspDomainFetchObj = cspObj
	fOps.RetCspDomainFetchErr = nil
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "aid2").Return(aObj2, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.clusterCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	exp.Message = "Create attempted with incorrect authorizedAccount a2Name[aid2]"
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	params.Payload.AuthorizedAccounts = nil
	aObj2.TenantAccountID = prevTID
	tl.Flush()
	mockCtrl.Finish()

	// missing property failure cases
	tcs := []string{"name", "clusterType", "cspDomainId", "badCT"}
	for _, tc := range tcs {
		cl := *params.Payload // copy
		switch tc {
		case "name":
			cl.Name = ""
		case "clusterType":
			cl.ClusterType = ""
		case "cspDomainId":
			cl.CspDomainID = ""
		case "badCT":
			cl.ClusterType = "INVALID-CLUSTER-TYPE"
		default:
			assert.False(true)
		}
		t.Log("case: error case: " + tc)
		cntRLock = hc.cntRLock
		cntRUnlock = hc.cntRUnlock
		assert.NotPanics(func() {
			ret = hc.clusterCreate(ops.ClusterCreateParams{HTTPRequest: params.HTTPRequest, Payload: &cl})
		})
		assert.NotNil(ret)
		_, ok = ret.(*ops.ClusterCreateCreated)
		assert.False(ok)
		mE, ok := ret.(*ops.ClusterCreateDefault)
		assert.True(ok)
		assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
		assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
		assert.Equal(cntRLock, hc.cntRLock)
		assert.Equal(cntRUnlock, hc.cntRUnlock)
		tl.Flush()
	}

	// no payload
	params.Payload = nil
	t.Log("case: nil payload")
	assert.NotPanics(func() { ret = hc.clusterCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorMissing.M, *mE.Payload.Message)

	// success (authorized user)
	t.Log("case: success, external user")
	fa.Posts = []*fal.Args{}
	ai = &auth.Info{AccountID: string(cspObj.AccountID)}
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	assert.Error(ai.InternalOK()) // not internal
	params = ops.ClusterCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
		Payload:     &models.ClusterCreateArgs{},
	}
	params.Payload.AuthorizedAccounts = []models.ObjIDMutable{"aid1", "aid2"}
	params.Payload.ClusterType = "kubernetes"
	params.Payload.CspDomainID = "cspObjectID"
	params.Payload.Name = "cluster1"
	params.Payload.Description = "a cluster"
	ctx = params.HTTPRequest.Context()
	testutils.Clone(params.Payload, &createArg)
	createArg.AccountID = "aid1"
	createArg.State = common.ClusterStateDeployable
	obj = &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID:      "objectID",
				Version: 1,
			},
		},
	}
	obj.Name = params.Payload.Name
	mockCtrl = gomock.NewController(t)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Create(ctx, createArg).Return(obj, nil)
	fOps.RetCspDomainFetchObj = cspObj
	fOps.RetCspDomainFetchErr = nil
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "aid1").Return(aObj, nil).Times(1)
	oA.EXPECT().Fetch(ctx, "aid2").Return(aObj2, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.clusterCreate(params) })
	assert.NotNil(ret)
	_, ok = ret.(*ops.ClusterCreateCreated)
	assert.True(ok)
	assert.Len(evM.InSSProps, 1)
	assert.EqualValues("objectID", evM.InSSProps["meta.id"])
	assert.Equal(obj, evM.InACScope)
	assert.NotNil(obj.ClusterUsagePolicy)
	assert.Equal(ai, fOps.InCspDomainFetchAi)
	assert.EqualValues(params.Payload.CspDomainID, fOps.InCspDomainFetchID)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	exp = &fal.Args{AI: ai, Action: centrald.ClusterCreateAction, ObjID: "objectID", Name: params.Payload.Name, Message: "Created"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	assert.Equal("aid1", ai.AccountID)
	*fa = fal.AuditLog{}

	t.Log("case: failure, external user sets clusterIdentifier")
	tl.Flush()
	params.Payload.State = ""
	params.Payload.ClusterIdentifier = "clusterIdentifier"
	assert.NotPanics(func() { ret = hc.clusterCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	assert.Regexp("set clusterIdentifier, messages or state", *mE.Payload.Message)

	t.Log("case: failure, external user sets state")
	tl.Flush()
	params.Payload.State = "state"
	params.Payload.ClusterIdentifier = ""
	assert.NotPanics(func() { ret = hc.clusterCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	assert.Regexp("set clusterIdentifier, messages or state", *mE.Payload.Message)

	t.Log("case: failure, external user sets messages")
	tl.Flush()
	params.Payload.Messages = []*models.TimestampedString{&models.TimestampedString{}}
	params.Payload.State = ""
	assert.NotPanics(func() { ret = hc.clusterCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	assert.Regexp("set clusterIdentifier, messages or state", *mE.Payload.Message)
}

func TestClusterDelete(t *testing.T) {
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
	params := ops.ClusterDeleteParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "objectID",
			},
		},
	}
	obj.Name = "cluster"
	obj.AccountID = "aid1"
	srlParams := storage_request.StorageRequestListParams{ClusterID: &params.ID, IsTerminated: swag.Bool(false)}
	nodeParams := node.NodeListParams{ClusterID: &params.ID}
	vsParams := volume_series.VolumeSeriesListParams{BoundClusterID: &params.ID}
	vrlParams := volume_series_request.VolumeSeriesRequestListParams{ClusterID: &params.ID, IsTerminated: swag.Bool(false)}
	spaParams := spa.ServicePlanAllocationListParams{ClusterID: &params.ID}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	t.Log("case: success")
	oC := mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Delete(ctx, params.ID).Return(nil)
	oC.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oSR := mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().Count(ctx, srlParams, uint(1)).Return(0, nil)
	oV := mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Count(ctx, vsParams, uint(1)).Return(0, nil)
	oN := mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Count(ctx, nodeParams, uint(1)).Return(0, nil)
	oVR := mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Count(ctx, vrlParams, uint(1)).Return(0, nil)
	oSPA := mock.NewMockServicePlanAllocationOps(mockCtrl)
	oSPA.EXPECT().Count(ctx, spaParams, uint(1)).Return(0, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	mds.EXPECT().OpsStorageRequest().Return(oSR)
	mds.EXPECT().OpsNode().Return(oN)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	hc.DS = mds
	cntLock := hc.cntLock
	cntUnlock := hc.cntUnlock
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.clusterDelete(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.ClusterDeleteNoContent)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Empty(evM.InSSProps)
	assert.Equal(obj, evM.InACScope)
	exp := &fal.Args{AI: ai, Action: centrald.ClusterDeleteAction, ObjID: models.ObjID(params.ID), Name: obj.Name, Message: "Deleted"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	tl.Flush()

	// delete failure case, cover valid tenant account
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: delete failure")
	ai.AccountID = "aid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Delete(ctx, params.ID).Return(centrald.ErrorDbError)
	oC.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().Count(ctx, srlParams, uint(1)).Return(0, nil)
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Count(ctx, vsParams, uint(1)).Return(0, nil)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Count(ctx, nodeParams, uint(1)).Return(0, nil)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Count(ctx, vrlParams, uint(1)).Return(0, nil)
	oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
	oSPA.EXPECT().Count(ctx, spaParams, uint(1)).Return(0, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	mds.EXPECT().OpsStorageRequest().Return(oSR)
	mds.EXPECT().OpsNode().Return(oN)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.clusterDelete(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.ClusterDeleteDefault)
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
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsCluster().Return(oC)
	fa.ReadyRet = centrald.ErrorDbError
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.clusterDelete(params) })
	mE, ok = ret.(*ops.ClusterDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.clusterDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	// not authorized tenant admin
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized tenant admin")
	fa.Posts = []*fal.Args{}
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	mds = mock.NewMockDataStore(mockCtrl)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsCluster().Return(oC)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.clusterDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Regexp(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	exp = &fal.Args{AI: ai, Action: centrald.ClusterDeleteAction, ObjID: models.ObjID(params.ID), Name: obj.Name, Err: true, Message: "Delete unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	ai.RoleObj = nil
	tl.Flush()

	// cascading validation failure cases
	tObj := []string{"storage request", "volume series", "node", "volume series request", "service plan allocation"}
	for tc := 0; tc < len(tObj)*2; tc++ {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		t.Log("case: " + tObj[tc/2] + []string{" count fails", " exists"}[tc%2])
		mds = mock.NewMockDataStore(mockCtrl)
		oC = mock.NewMockClusterOps(mockCtrl)
		oC.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
		mds.EXPECT().OpsCluster().Return(oC)
		count, err := tc%2, []error{centrald.ErrorDbError, nil}[tc%2]
		switch tc / 2 {
		case 4:
			oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
			oSPA.EXPECT().Count(ctx, spaParams, uint(1)).Return(count, err)
			mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
			count, err = 0, nil
			fallthrough
		case 3:
			oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			oVR.EXPECT().Count(ctx, vrlParams, uint(1)).Return(count, err)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			count, err = 0, nil
			fallthrough
		case 2:
			oN = mock.NewMockNodeOps(mockCtrl)
			oN.EXPECT().Count(ctx, nodeParams, uint(1)).Return(count, err)
			mds.EXPECT().OpsNode().Return(oN)
			count, err = 0, nil
			fallthrough
		case 1:
			oV = mock.NewMockVolumeSeriesOps(mockCtrl)
			oV.EXPECT().Count(ctx, vsParams, uint(1)).Return(count, err)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			count, err = 0, nil
			fallthrough
		case 0:
			oSR = mock.NewMockStorageRequestOps(mockCtrl)
			oSR.EXPECT().Count(ctx, srlParams, uint(1)).Return(count, err)
			mds.EXPECT().OpsStorageRequest().Return(oSR)
		default:
			assert.True(false)
		}
		hc.DS = mds
		cntLock = hc.cntLock
		cntUnlock = hc.cntUnlock
		assert.NotPanics(func() { ret = hc.clusterDelete(params) })
		assert.NotNil(ret)
		mE, ok = ret.(*ops.ClusterDeleteDefault)
		if assert.True(ok) {
			if tc%2 == 0 {
				assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
				assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
			} else {
				assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
				assert.Regexp(tObj[tc/2]+".* the cluster", *mE.Payload.Message)
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
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.clusterDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
	assert.Equal(cntLock, hc.cntLock)
	assert.Equal(cntUnlock, hc.cntUnlock)
}

func TestClusterFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.ClusterFetchParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "objectID",
			},
		},
		ClusterCreateOnce: models.ClusterCreateOnce{
			CspDomainID: "domID",
		},
	}
	obj.Name = "cluster"
	obj.AccountID = "aid1"
	domObj := &models.CSPDomain{
		CSPDomainMutable: models.CSPDomainMutable{
			ClusterUsagePolicy: &models.ClusterUsagePolicy{
				AccountSecretScope: "CLUSTER",
			},
			CspCredentialID: "credID",
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

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	assert.Nil(obj.ClusterUsagePolicy)
	oC := mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	oCSP := mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, string(obj.CspDomainID)).Return(domObj, nil)
	oCred := mock.NewMockCspCredentialOps(mockCtrl)
	oCred.EXPECT().Fetch(ctx, "credID").Return(credObj, nil)
	mds.EXPECT().OpsCspCredential().Return(oCred)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.clusterFetch(params) })
	assert.NotNil(ret)
	mR, ok := ret.(*ops.ClusterFetchOK)
	assert.True(ok)
	assert.Equal(obj, mR.Payload)
	assert.NotNil(obj.ClusterUsagePolicy)
	tl.Flush()

	// fetch failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: fetch failure")
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.clusterFetch(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.ClusterFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)

	// dom fetch failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: fetch failure")
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, string(obj.CspDomainID)).Return(nil, centrald.ErrorNotFound)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.clusterFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.clusterFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: unauthorized")
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	hc.DS = mds
	ai.AccountID = "aid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: false}
	assert.NotPanics(func() { ret = hc.clusterFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	tl.Flush()

	t.Log("case: external user, no ClusterIdentifier returned")
	obj.ClusterUsagePolicy = &models.ClusterUsagePolicy{
		Inherited: false,
	}
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	assert.Error(ai.InternalOK()) // not internal
	mockCtrl = gomock.NewController(t)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.clusterFetch(params) })
	assert.NotNil(ret)
	mR, ok = ret.(*ops.ClusterFetchOK)
	assert.True(ok)
	assert.Equal(obj, mR.Payload)
	assert.Equal("", obj.ClusterIdentifier)
}

func TestClusterList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	fOps := &fakeOps{}
	hc.ops = fOps
	ai := &auth.Info{}
	params := ops.ClusterListParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	ctx := params.HTTPRequest.Context()
	objects := []*models.Cluster{
		&models.Cluster{
			ClusterAllOf0: models.ClusterAllOf0{
				Meta: &models.ObjMeta{
					ID: "objectID",
				},
			},
			ClusterCreateOnce: models.ClusterCreateOnce{
				AccountID:   "aid1",
				CspDomainID: "cspDomainID", // all in same dom
			},
			ClusterMutable: models.ClusterMutable{
				ClusterCreateMutable: models.ClusterCreateMutable{
					Name: "cluster",
				},
			},
		},
		&models.Cluster{
			ClusterAllOf0: models.ClusterAllOf0{
				Meta: &models.ObjMeta{
					ID: "objectID2",
				},
			},
			ClusterCreateOnce: models.ClusterCreateOnce{
				AccountID:   "aid2",
				CspDomainID: "cspDomainID", // all in same dom
			},
			ClusterMutable: models.ClusterMutable{
				ClusterCreateMutable: models.ClusterCreateMutable{
					Name: "cluster2",
				},
			},
		},
	}
	cspObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID: "cspDomainID",
			},
			AccountID:     "aid1",
			CspDomainType: "AWS",
		},
		CSPDomainMutable: models.CSPDomainMutable{
			Name: "cspDomain",
			ClusterUsagePolicy: &models.ClusterUsagePolicy{
				AccountSecretScope: "CLUSTER",
			},
		},
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	fOps.RetCspDomainFetchObj = cspObj
	fOps.RetCspDomainFetchErr = nil
	fOps.CntCspDomainFetch = 0
	oC := mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().List(ctx, params).Return(objects, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.clusterList(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.ClusterListOK)
	assert.True(ok)
	assert.Equal(objects, mO.Payload)
	for _, obj := range mO.Payload {
		assert.NotNil(obj.ClusterUsagePolicy)
	}
	assert.True(len(mO.Payload) > 1)         // multiple clusters
	assert.True(fOps.CntCspDomainFetch == 1) // but only one domain call
	tl.Flush()

	// list fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().List(ctx, params).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	t.Log("case: List failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.clusterList(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.ClusterListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)

	// cache fails
	fOps.RetCspDomainFetchObj = nil
	fOps.RetCspDomainFetchErr = centrald.ErrorInternalError
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().List(ctx, params).Return(objects, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	t.Log("case: Dom lookup failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.clusterList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.ClusterListOK)
	assert.True(ok)
	assert.Len(mO.Payload, 0)

	// dom lookup succeeds from now on
	fOps.RetCspDomainFetchObj = cspObj
	fOps.RetCspDomainFetchErr = nil

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.clusterList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ClusterListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	mockCtrl.Finish()
	t.Log("case: constrainBothQueryAccounts changes accounts")
	ai.RoleObj = &models.Role{}
	params.AuthorizedAccountID, params.AccountID = swag.String("a1"), swag.String("t1")
	fOps.RetConstrainBothQueryAccountsAID, fOps.RetConstrainBothQueryAccountsTID = swag.String("aid1"), swag.String("tid1")
	cParams := params
	cParams.AuthorizedAccountID, cParams.AccountID = fOps.RetConstrainBothQueryAccountsAID, fOps.RetConstrainBothQueryAccountsTID
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().List(ctx, cParams).Return(objects, nil)
	mds.EXPECT().OpsCluster().Return(oC)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.clusterList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.ClusterListOK)
	assert.True(ok)
	assert.Len(mO.Payload, len(objects))
	assert.Equal(4, fOps.CntConstrainBothQueryAccounts)
	assert.Equal(params.AuthorizedAccountID, fOps.InConstrainBothQueryAccountsAID)
	assert.Equal(params.AccountID, fOps.InConstrainBothQueryAccountsTID)
	tl.Flush()

	t.Log("case: constrainBothQueryAccounts skip")
	fOps.RetConstrainBothQueryAccountsSkip = true
	assert.NotPanics(func() { ret = hc.clusterList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.ClusterListOK)
	assert.True(ok)
	assert.Empty(mO.Payload)
	assert.Equal(5, fOps.CntConstrainBothQueryAccounts)
}

func TestClusterUpdate(t *testing.T) {
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
	fOps := &fakeOps{}
	hc.ops = fOps

	// make the name map
	var nMap JSONToAttrNameMap
	assert.NotPanics(func() { nMap = hc.clusterMutableNameMap() })
	// validate some embedded properties
	assert.Equal("name", nMap.jName("Name"))
	assert.Equal("authorizedAccounts", nMap.jName("AuthorizedAccounts"))
	assert.Equal("clusterAttributes", nMap.jName("ClusterAttributes"))

	// parse params
	objM := &models.ClusterMutable{}
	objM.Name = "cluster"
	ai := &auth.Info{}
	params := ops.ClusterUpdateParams{
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

	obj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID:      "id1",
				Version: 8,
			},
		},
		ClusterCreateOnce: models.ClusterCreateOnce{
			AccountID:   "aid1",
			CspDomainID: "cspDomainID",
		},
	}
	obj.Name = "clusterName"
	cspObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID: "cspDomainID",
			},
			AccountID:     "aid1",
			CspDomainType: "AWS",
		},
		CSPDomainMutable: models.CSPDomainMutable{
			Name: "cspDomain",
			ClusterUsagePolicy: &models.ClusterUsagePolicy{
				AccountSecretScope: "CLUSTER",
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

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	fOps.RetCspDomainFetchObj = cspObj
	fOps.RetCspDomainFetchErr = nil
	oC := mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oC.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	cntRLock := hc.cntRLock
	cntRUnlock := hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.clusterUpdate(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.ClusterUpdateOK)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Equal(obj, mO.Payload)
	assert.NotNil(obj.ClusterUsagePolicy)
	assert.Empty(evM.InSSProps)
	assert.Equal(obj, evM.InACScope)
	assert.Empty(fa.Posts)
	tl.Flush()

	// failures related to state transitions
	for tc := 0; tc <= 3; tc++ {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		cntRLock = hc.cntRLock
		cntRUnlock = hc.cntRUnlock
		expErr := centrald.ErrorRequestInConflict
		expErrPat := ""
		switch tc {
		case 0:
			t.Log("case: failure when updating ClusterIdentifier in a wrong state")
			obj.State = common.ClusterStateDeployable
			params.Set = []string{"clusterIdentifier", "state"}
			params.Payload.State = common.ClusterStateResetting
			params.Payload.ClusterIdentifier = "some_id"
			expErr = centrald.ErrorInvalidState
			expErrPat = "invalid state transition"
		case 1:
			t.Log("case: failure when updating ClusterIdentifier without state change")
			params.Set = []string{"clusterIdentifier"}
			params.Payload.ClusterIdentifier = "some_id"
			expErr = centrald.ErrorMissing
			expErrPat = "state must be set with clusterIdentifier"
		case 2:
			t.Log("case: failure when transitioning from DEPLOYABLE state to MANAGED and no ClusterIdentifier specified")
			obj.State = common.ClusterStateDeployable
			params.Set = []string{"state"}
			params.Payload.State = common.ClusterStateManaged
			params.Payload.ClusterIdentifier = ""
			expErr = centrald.ErrorMissing
			expErrPat = "clusterIdentifier must be set when transitioning to MANAGED"
		case 3:
			t.Log("case: failure when transitioning to DEPLOYABLE state with non-empty ClusterIdentifier specified")
			obj.State = common.ClusterStateManaged
			params.Set = []string{"clusterIdentifier", "state"}
			params.Payload.State = common.ClusterStateDeployable
			params.Payload.ClusterIdentifier = "some_id"
			expErr = centrald.ErrorMissing
			expErrPat = "clusterIdentifier must be cleared when transitioning to DEPLOYABLE"
		default:
			assert.False(true)
		}
		oC = mock.NewMockClusterOps(mockCtrl)
		oC.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
		mds = mock.NewMockDataStore(mockCtrl)
		mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
		hc.DS = mds
		assert.NotPanics(func() { ret = hc.clusterUpdate(params) })
		assert.NotNil(ret)
		mD, ok := ret.(*ops.ClusterUpdateDefault)
		assert.True(ok)
		assert.Equal(cntRLock+1, hc.cntRLock)
		assert.Equal(cntRUnlock+1, hc.cntRUnlock)
		assert.Equal(expErr.C, int(mD.Payload.Code))
		assert.Regexp("^"+expErr.M, *mD.Payload.Message)
		assert.Regexp(expErrPat, *mD.Payload.Message)
		tl.Flush()
	}

	params.Payload = objM                     // reset
	params.Set = []string{nMap.jName("Name")} // reset

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: domain fetch fails")
	fOps.RetCspDomainFetchObj = nil
	fOps.RetCspDomainFetchErr = centrald.ErrorInternalError
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.clusterUpdate(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.ClusterUpdateDefault)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	tl.Flush()

	// dom lookup succeeds for the rest of the tests
	fOps.RetCspDomainFetchObj = cspObj
	fOps.RetCspDomainFetchErr = nil

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: update fails, cover authorizedAccounts remove case")
	params.Remove = []string{"authorizedAccounts"}
	params.Set = []string{"clusterVersion", "service", "clusterAttributes"} // internalOK allowed
	params.Payload.AuthorizedAccounts = []models.ObjIDMutable{"aid1"}
	uP[centrald.UpdateSet] = params.Set
	uP[centrald.UpdateRemove] = params.Remove
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oC.EXPECT().Update(ctx, uaM, params.Payload).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	hc.DS = mds
	fv := &fakeAuthAccountValidator{}
	hc.authAccountValidator = fv
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.clusterUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ClusterUpdateDefault)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	assert.Equal(centrald.ClusterUpdateAction, fv.inAction)
	assert.Equal(params.ID, fv.inObjID)
	assert.Equal(obj.Name, fv.inObjName)
	assert.Empty(fv.inOldIDList)
	assert.Equal(params.Payload.AuthorizedAccounts, fv.inUpdateIDList)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: invalid state")
	params.Append = []string{}
	params.Remove = []string{}
	params.Set = []string{"state"}
	params.Payload.State = common.ClusterStateManaged + "-INVALID"
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.clusterUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ClusterUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	assert.Regexp("invalid cluster state", *mD.Payload.Message)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: fetch fails")
	mds = mock.NewMockDataStore(mockCtrl)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds.EXPECT().OpsCluster().Return(oC)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.clusterUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ClusterUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mD.Payload.Message)
	tl.Flush()

	// append authorizedAccounts, fully tested in TestValidateAuthorizedAccountsUpdate
	mockCtrl.Finish()
	t.Log("case: append authorizedAccounts")
	mockCtrl = gomock.NewController(t)
	ai.AccountID = "aid1" // cover valid tenant admin case
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	params.Append = []string{"authorizedAccounts"}
	params.Remove = []string{}
	params.Set = []string{"name"}
	params.Payload.AuthorizedAccounts = []models.ObjIDMutable{"aid1"}
	uP[centrald.UpdateAppend] = params.Append
	uP[centrald.UpdateRemove] = params.Remove
	uP[centrald.UpdateSet] = params.Set
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oC.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	hc.DS = mds
	fv = &fakeAuthAccountValidator{retString: "aa added"}
	hc.authAccountValidator = fv
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.clusterUpdate(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.ClusterUpdateOK)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Equal(obj, mO.Payload)
	exp := &fal.Args{AI: ai, Action: centrald.ClusterUpdateAction, ObjID: "objectID", Name: obj.Name, Message: "Updated authorizedAccounts aa added"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	assert.Equal(centrald.ClusterUpdateAction, fv.inAction)
	assert.Equal(params.ID, fv.inObjID)
	assert.Equal(obj.Name, fv.inObjName)
	assert.Empty(fv.inOldIDList)
	assert.Equal(params.Payload.AuthorizedAccounts, fv.inUpdateIDList)
	tl.Flush()

	for _, tc := range []string{"clusterVersion", "service", "clusterAttributes", "clusterIdentifier", "state", "messages"} {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		t.Log("case: not internal:", tc)
		params.Set = []string{tc}
		fa.Posts = []*fal.Args{}
		mds = mock.NewMockDataStore(mockCtrl)
		oC = mock.NewMockClusterOps(mockCtrl)
		oC.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
		mds.EXPECT().OpsCluster().Return(oC)
		hc.DS = mds
		assert.NotPanics(func() { ret = hc.clusterUpdate(params) })
		assert.NotNil(ret)
		mD, ok = ret.(*ops.ClusterUpdateDefault)
		assert.True(ok)
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)
		exp = &fal.Args{AI: ai, Action: centrald.ClusterUpdateAction, ObjID: models.ObjID(params.ID), Name: obj.Name, Err: true, Message: "Update unauthorized"}
		assert.Equal([]*fal.Args{exp}, fa.Posts)
		tl.Flush()
	}

	mockCtrl.Finish()
	t.Log("case: set authorizedAccounts, validateAuthorizedAccountsUpdate error, no version")
	mockCtrl = gomock.NewController(t)
	params.Append = []string{}
	params.Remove = []string{}
	params.Set = []string{"authorizedAccounts"}
	params.Payload.AuthorizedAccounts = []models.ObjIDMutable{"aid1"}
	params.Version = nil
	uP[centrald.UpdateAppend] = params.Append
	uP[centrald.UpdateRemove] = params.Remove
	uP[centrald.UpdateSet] = params.Set
	mds = mock.NewMockDataStore(mockCtrl)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsCluster().Return(oC)
	hc.DS = mds
	fv = &fakeAuthAccountValidator{retError: fmt.Errorf("aa error")}
	hc.authAccountValidator = fv
	assert.NotPanics(func() { ret = hc.clusterUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ClusterUpdateDefault)
	assert.True(ok)
	assert.Equal(http.StatusInternalServerError, int(mD.Payload.Code))
	assert.Equal("aa error", *mD.Payload.Message)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: auditLog not ready")
	mds = mock.NewMockDataStore(mockCtrl)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsCluster().Return(oC)
	fa.ReadyRet = centrald.ErrorDbError
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.clusterUpdate(params) })
	mD, ok = ret.(*ops.ClusterUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.clusterUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ClusterUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized tenant admin")
	fa.Posts = []*fal.Args{}
	mds = mock.NewMockDataStore(mockCtrl)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsCluster().Return(oC)
	hc.DS = mds
	ai.AccountID = "otherID"
	assert.NotPanics(func() { ret = hc.clusterUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ClusterUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	ai.RoleObj = nil
	tl.Flush()

	// no changes requested
	params.Set = []string{}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	t.Log("case: no change")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.clusterUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ClusterUpdateDefault)
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
	assert.NotPanics(func() { ret = hc.clusterUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ClusterUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: wrong version")
	params.Payload.Name = "newName"
	params.Version = swag.Int32(7)
	mds = mock.NewMockDataStore(mockCtrl)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsCluster().Return(oC)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.clusterUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ClusterUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorIDVerNotFound.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorIDVerNotFound.M, *mD.Payload.Message)
	tl.Flush()

	// updating CUP in ACTIVE state
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: invalid update of CUP in ACTIVE state")
	params.Set = []string{"clusterUsagePolicy"}
	params.Payload.ClusterUsagePolicy = &models.ClusterUsagePolicy{
		Inherited: true,
	}
	obj.State = common.ClusterStateManaged
	mds = mock.NewMockDataStore(mockCtrl)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsCluster().Return(oC)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.clusterUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ClusterUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	assert.Regexp("invalid state", *mD.Payload.Message)
	tl.Flush()

	// invalid CUP
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: invalid clusterUsagePolicy")
	params.Set = []string{"clusterUsagePolicy"}
	params.Payload.ClusterUsagePolicy = nil
	obj.State = common.ClusterStateDeployable
	mds = mock.NewMockDataStore(mockCtrl)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsCluster().Return(oC)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.clusterUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ClusterUpdateDefault)
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
	assert.NotPanics(func() { ret = hc.clusterUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ClusterUpdateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
		assert.Regexp(centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
		assert.Regexp("missing payload", *mD.Payload.Message)
	}
}

func TestClusterAccountSecretFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	fOps := &fakeOps{}
	hc.ops = fOps

	ai := &auth.Info{}
	params := ops.ClusterAccountSecretFetchParams{
		ID: "clusterID",
	}
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "clusterID",
			},
		},
	}
	clObj.Name = "cluster"
	clObj.AccountID = "ownerAccountID"
	clObj.AuthorizedAccounts = []models.ObjIDMutable{"account0", "account1", "account2"}

	t.Log("case: missing auth")
	params.HTTPRequest = &http.Request{}
	ret := hc.clusterAccountSecretFetch(params)
	mD, ok := ret.(*ops.ClusterAccountSecretFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)

	params.HTTPRequest = requestWithAuthContext(ai)

	t.Log("case: missing authorizedAccountID")
	ret = hc.clusterAccountSecretFetch(params)
	mD, ok = ret.(*ops.ClusterAccountSecretFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorMissing.M, *mD.Payload.Message)
	assert.Regexp("authorizedAccountId", *mD.Payload.Message)

	params.AuthorizedAccountID = "unauthorizedAccount"

	t.Log("case: fetch failure")
	fOps.RetClusterFetchObj = nil
	fOps.RetClusterFetchErr = centrald.ErrorNotFound
	ret = hc.clusterAccountSecretFetch(params)
	mD, ok = ret.(*ops.ClusterAccountSecretFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorNotFound.M, *mD.Payload.Message)

	fOps.RetClusterFetchObj = clObj
	fOps.RetClusterFetchErr = nil

	t.Log("case: unauthorized account")
	assert.NotNil(params.AuthorizedAccountID)
	ret = hc.clusterAccountSecretFetch(params)
	mD, ok = ret.(*ops.ClusterAccountSecretFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorMissing.M, *mD.Payload.Message)
	assert.Regexp("authorizedAccountId", *mD.Payload.Message)

	params.AuthorizedAccountID = "account1"

	t.Log("case: invalid cluster state")
	ret = hc.clusterAccountSecretFetch(params)
	mD, ok = ret.(*ops.ClusterAccountSecretFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInvalidState.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorInvalidState.M, *mD.Payload.Message)
	assert.Regexp("cluster state", *mD.Payload.Message)

	clObj.State = common.ClusterStateManaged

	t.Log("case: cluster client failure")
	clObj.ClusterType = "foo"
	ret = hc.clusterAccountSecretFetch(params)
	mD, ok = ret.(*ops.ClusterAccountSecretFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInvalidData.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorInvalidData.M, *mD.Payload.Message)

	clObj.ClusterType = cluster.K8sClusterType

	t.Log("case: secret retrieval error")
	fOps.RetAccountSecretRetrieveVT = nil
	fOps.RetAccountSecretRetrieveErr = centrald.ErrorInternalError
	ret = hc.clusterAccountSecretFetch(params)
	mD, ok = ret.(*ops.ClusterAccountSecretFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorInternalError.M, *mD.Payload.Message)

	fOps.RetAccountSecretRetrieveErr = nil
	fOps.RetAccountSecretRetrieveVT = &models.ValueType{Kind: common.ValueTypeSecret, Value: "the secret"}

	// mock the cluster client to validate the arguments passed
	t.Log("case: secret format ok (defaults)")
	mockCtrl := gomock.NewController(t)
	cc := mockcluster.NewMockClient(mockCtrl)
	hc.clusterClientMap = map[string]cluster.Client{}
	hc.clusterClientMap[clObj.ClusterType] = cc
	expSCA := &cluster.SecretCreateArgsMV{
		Intent:            cluster.SecretIntentAccountIdentity,
		Name:              common.AccountSecretClusterObjectName,
		CustomizationData: cluster.AccountVolumeData{AccountSecret: fOps.RetAccountSecretRetrieveVT.Value},
	}
	cc.EXPECT().SecretFormatMV(gomock.Any(), expSCA).Return("encoded secret", nil)
	ret = hc.clusterAccountSecretFetch(params)
	mO, ok := ret.(*ops.ClusterAccountSecretFetchOK)
	assert.True(ok)
	assert.Equal(common.ValueTypeSecret, mO.Payload.Kind)
	assert.Regexp("encoded secret", mO.Payload.Value)
	mockCtrl.Finish()

	t.Log("case: secret format ok (k8sNamespace)")
	params.K8sNamespace = swag.String("my-namespace")
	mockCtrl = gomock.NewController(t)
	cc = mockcluster.NewMockClient(mockCtrl)
	hc.clusterClientMap = map[string]cluster.Client{}
	hc.clusterClientMap[clObj.ClusterType] = cc
	expSCA = &cluster.SecretCreateArgsMV{
		Intent:            cluster.SecretIntentAccountIdentity,
		Name:              common.AccountSecretClusterObjectName,
		Namespace:         "my-namespace",
		CustomizationData: cluster.AccountVolumeData{AccountSecret: fOps.RetAccountSecretRetrieveVT.Value},
	}
	cc.EXPECT().SecretFormatMV(gomock.Any(), expSCA).Return("encoded secret", nil)
	ret = hc.clusterAccountSecretFetch(params)
	mO, ok = ret.(*ops.ClusterAccountSecretFetchOK)
	assert.True(ok)
	assert.Equal(common.ValueTypeSecret, mO.Payload.Kind)
	assert.Regexp("encoded secret", mO.Payload.Value)
	mockCtrl.Finish()

	t.Log("case: secret format error")
	mockCtrl = gomock.NewController(t)
	cc = mockcluster.NewMockClient(mockCtrl)
	hc.clusterClientMap[clObj.ClusterType] = cc
	cc.EXPECT().SecretFormatMV(gomock.Any(), expSCA).Return("", fmt.Errorf("secret-format"))
	ret = hc.clusterAccountSecretFetch(params)
	mD, ok = ret.(*ops.ClusterAccountSecretFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInvalidData.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorInvalidData.M, *mD.Payload.Message)
	assert.Regexp("secret-format", *mD.Payload.Message)
}

func TestClusterOrchestratorGetDeployment(t *testing.T) {
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
	fOps := &fakeOps{}
	hc.ops = fOps
	ai := &auth.Info{}

	gdParams := ops.ClusterOrchestratorGetDeploymentParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	obj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "objectID",
			},
		},
		ClusterCreateOnce: models.ClusterCreateOnce{
			AccountID:   "aid1",
			CspDomainID: "domID",
			ClusterType: cluster.K8sClusterType,
		},
		ClusterMutable: models.ClusterMutable{
			ClusterCreateMutable: models.ClusterCreateMutable{
				State: common.ClusterStateDeployable,
				Name:  "cluster",
			},
		},
	}
	domObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			CspDomainType: "AWS",
		},
		CSPDomainMutable: models.CSPDomainMutable{
			ClusterUsagePolicy: &models.ClusterUsagePolicy{
				AccountSecretScope: "CLUSTER",
			},
			CspCredentialID: "credID",
			ManagementHost:  "f.q.d.n",
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
	args := &cluster.MCDeploymentArgs{
		SystemID:       string(sysObj.Meta.ID),
		CSPDomainID:    string(obj.CspDomainID),
		ClusterID:      string(obj.Meta.ID),
		CSPDomainType:  string(domObj.CspDomainType),
		ClusterType:    obj.ClusterType,
		ClusterName:    string(obj.Name),
		ManagementHost: domObj.ManagementHost,
		DriverType:     app.DriverType,
	}
	dep := &cluster.Deployment{
		Format:     "yaml",
		Deployment: "data",
	}
	var ret middleware.Responder

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	t.Log("case: success")
	args.ClusterVersion = "v1.14.6"
	gdParams.OrchestratorVersion = swag.String("v1.14.6")
	fOps.RetClusterFetchObj = obj
	fOps.RetCspDomainFetchObj = domObj
	fOps.RetSystemFetchObj = sysObj
	appCSP := mock.NewMockAppCloudServiceProvider(mockCtrl)
	appCSP.EXPECT().GetMCDeployment(args).Return(dep, nil).MinTimes(1)
	app.AppCSP = appCSP
	assert.NotPanics(func() { ret = hc.clusterOrchestratorGetDeployment(gdParams) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.ClusterOrchestratorGetDeploymentOK)
	assert.True(ok)
	tl.Flush()

	t.Log("case: version panic")
	gdParams.OrchestratorVersion = swag.String("xyz,abc")
	assert.NotPanics(func() { ret = hc.clusterOrchestratorGetDeployment(gdParams) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.ClusterOrchestratorGetDeploymentDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInvalidData.C, int(mE.Payload.Code))
	assert.Regexp(centrald.ErrorInvalidData.M, *mE.Payload.Message)
	gdParams.OrchestratorVersion = nil // reset
	args.ClusterVersion = ""           // reset

	t.Log("case: cluster fetch failure")
	fOps.RetClusterFetchObj = nil
	fOps.RetClusterFetchErr = centrald.ErrorInternalError
	assert.NotPanics(func() { ret = hc.clusterOrchestratorGetDeployment(gdParams) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterOrchestratorGetDeploymentDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	fOps.RetClusterFetchObj = obj // restore
	fOps.RetClusterFetchErr = nil // restore
	tl.Flush()

	t.Log("case: domain fetch failure")
	fOps.RetCspDomainFetchObj = nil
	fOps.RetCspDomainFetchErr = centrald.ErrorInternalError
	assert.NotPanics(func() { ret = hc.clusterOrchestratorGetDeployment(gdParams) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterOrchestratorGetDeploymentDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	fOps.RetCspDomainFetchObj = domObj // restore
	fOps.RetCspDomainFetchErr = nil    // restore
	tl.Flush()

	t.Log("case: domain missing managementHost property")
	domObj.ManagementHost = ""
	fOps.RetCspDomainFetchObj = domObj
	assert.NotPanics(func() { ret = hc.clusterOrchestratorGetDeployment(gdParams) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterOrchestratorGetDeploymentDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInvalidData.C, int(mE.Payload.Code))
	assert.Regexp(centrald.ErrorInvalidData.M, *mE.Payload.Message)
	assert.Regexp("invalid or insufficient data: Cluster\\[objectID\\]: managementHost property must be present in associated CSPDomain\\[domID\\]", *mE.Payload.Message)
	domObj.ManagementHost = "f.q.d.n" // restore
	tl.Flush()

	t.Log("case: wrong cluster state")
	obj.State = "bad_state"
	fOps.RetClusterFetchObj = obj
	fOps.RetClusterFetchErr = nil
	assert.NotPanics(func() { ret = hc.clusterOrchestratorGetDeployment(gdParams) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterOrchestratorGetDeploymentDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInvalidState.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorInvalidState.M, *mE.Payload.Message)
	assert.Regexp("permitted only in the DEPLOYABLE or MANAGED states", *mE.Payload.Message)
	obj.State = common.ClusterStateDeployable // restore
	tl.Flush()

	t.Log("case: system fetch failure")
	fOps.RetSystemFetchObj = nil
	fOps.RetSystemFetchErr = centrald.ErrorInternalError
	assert.NotPanics(func() { ret = hc.clusterOrchestratorGetDeployment(gdParams) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterOrchestratorGetDeploymentDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	fOps.RetSystemFetchObj = sysObj // restore
	fOps.RetSystemFetchErr = nil    // restore
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: GetMCDeployment failure")
	gdParams.OrchestratorType = swag.String("mesos")
	gdParams.OrchestratorVersion = swag.String("1.2.3")
	args.ClusterType = "mesos"
	args.ClusterVersion = "1.2.3"
	appCSP = mock.NewMockAppCloudServiceProvider(mockCtrl)
	appCSP.EXPECT().GetMCDeployment(args).Return(nil, fmt.Errorf("fake error")).MinTimes(1)
	app.AppCSP = appCSP
	assert.NotPanics(func() { ret = hc.clusterOrchestratorGetDeployment(gdParams) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterOrchestratorGetDeploymentDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInvalidData.C, int(mE.Payload.Code))
	assert.Regexp(centrald.ErrorInvalidData.M, *mE.Payload.Message)
	assert.Regexp("fake error", *mE.Payload.Message)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: Orchestrator type specified, without ochestrator version")
	gdParams.OrchestratorType = swag.String("mesos")
	gdParams.OrchestratorVersion = swag.String("")
	assert.NotPanics(func() { ret = hc.clusterOrchestratorGetDeployment(gdParams) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterOrchestratorGetDeploymentDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInvalidData.C, int(mE.Payload.Code))
	assert.Regexp(centrald.ErrorInvalidData.M, *mE.Payload.Message)
	assert.Regexp("Must set orchestrator version if type is specified", *mE.Payload.Message)
	tl.Flush()

	t.Log("case: auth internal error")
	gdParams.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.clusterOrchestratorGetDeployment(gdParams) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ClusterOrchestratorGetDeploymentDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	gdParams.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()
}
