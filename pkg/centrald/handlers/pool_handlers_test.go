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
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/pool"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/service_plan_allocation"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/storage"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
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

func TestPoolFetchFilter(t *testing.T) {
	assert := assert.New(t)
	hc := newHandlerComp()
	obj := &models.Pool{
		PoolAllOf0: models.PoolAllOf0{Meta: &models.ObjMeta{ID: "id1"}},
		PoolCreateOnce: models.PoolCreateOnce{
			AccountID:           "tid1",
			AuthorizedAccountID: "aid1",
		},
	}
	rObj := &models.Role{}
	rObj.Capabilities = map[string]bool{}
	ai := &auth.Info{}

	t.Log("case: internal user")
	assert.NoError(hc.poolFetchFilter(ai, obj))

	t.Log("case: SystemManagementCap capability fail")
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.SystemManagementCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.poolFetchFilter(ai, obj))

	t.Log("case: CSPDomainManagementCap capability correct tenant")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	ai.AccountID = "tid1"
	assert.NoError(hc.poolFetchFilter(ai, obj))

	t.Log("case: CSPDomainManagementCap capability wrong tenant")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	ai.AccountID = "tid2"
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.poolFetchFilter(ai, obj))

	t.Log("case: authorized account")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	ai.AccountID = "aid1"
	assert.NoError(hc.poolFetchFilter(ai, obj))

	t.Log("case: not authorized account")
	ai.AccountID = "aid9"
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.poolFetchFilter(ai, obj))
}

func TestPoolCreate(t *testing.T) {
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
	cspDomainTypes := app.SupportedCspDomainTypes()
	assert.NotNil(cspDomainTypes)
	assert.NotEmpty(cspDomainTypes)
	dt := models.CspDomainTypeMutable("AWS")
	assert.Contains(cspDomainTypes, dt)
	cspStorageTypes := app.SupportedCspStorageTypes()
	var cstN, nstN models.CspStorageType
	// pool attributes must match what is expected
	minSize := int64(0)
	for _, st := range cspStorageTypes {
		if s := swag.Int64Value(st.MinAllocationSizeBytes); s > minSize {
			minSize = s
		}
		if st.CspDomainType == models.CspDomainType(dt) && st.AccessibilityScope == "CSPDOMAIN" {
			cstN = st.Name
		} else if st.CspDomainType == models.CspDomainType(dt) && st.AccessibilityScope == "NODE" {
			nstN = st.Name
		}
	}
	assert.NotZero(cstN)
	assert.NotZero(nstN)
	ai := &auth.Info{}
	params := ops.PoolCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
		Payload:     &models.PoolCreateArgs{},
	}
	params.Payload.CspDomainID = "cspObjectID"
	params.Payload.ClusterID = "clusterID"
	params.Payload.AccountID = "accountID"
	params.Payload.AuthorizedAccountID = "authorizedAccountID"
	params.Payload.CspStorageType = string(cstN)
	params.Payload.StorageAccessibility = &models.StorageAccessibilityMutable{
		AccessibilityScope:      "CSPDOMAIN",
		AccessibilityScopeObjID: "cspObjectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.Pool{
		PoolAllOf0: models.PoolAllOf0{
			Meta: &models.ObjMeta{
				ID:      "objectID",
				Version: 1,
			},
		},
	}
	cspObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID(params.Payload.CspDomainID),
			},
			AccountID:     "accountID",
			CspDomainType: dt,
		},
		CSPDomainMutable: models.CSPDomainMutable{
			AuthorizedAccounts: []models.ObjIDMutable{"authorizedAccountID"},
			Name:               "cspDomain",
		},
	}
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID(params.Payload.ClusterID),
			},
		},
		ClusterCreateOnce: models.ClusterCreateOnce{
			AccountID:   "accountID",
			CspDomainID: models.ObjIDMutable(params.Payload.CspDomainID),
		},
		ClusterMutable: models.ClusterMutable{
			ClusterCreateMutable: models.ClusterCreateMutable{
				AuthorizedAccounts: []models.ObjIDMutable{"authorizedAccountID"},
			},
		},
	}
	aObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{Meta: &models.ObjMeta{ID: "accountID"}},
	}
	aaObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta:            &models.ObjMeta{ID: "authorizedAccountID"},
			TenantAccountID: "accountID",
		},
	}
	nodeObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "nodeId",
			},
		},
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { tl.Logger().Info("/////////// ALL DONE"); tl.Flush(); mockCtrl.Finish() }()
	oP := mock.NewMockPoolOps(mockCtrl)
	var retObj *models.Pool
	testutils.Clone(obj, &retObj)
	retObj.CspDomainID = "cspDomainId"
	retObj.ClusterID = "clusterId"
	retObj.AccountID = "accountID"
	retObj.AuthorizedAccountID = "authorizedAccountID"
	oP.EXPECT().Create(ctx, params.Payload).Return(retObj, nil)
	oCSP := mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, string(params.Payload.CspDomainID)).Return(cspObj, nil)
	oCL := mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(params.Payload.ClusterID)).Return(clObj, nil)
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(params.Payload.AccountID)).Return(aObj, nil)
	oA.EXPECT().Fetch(ctx, string(params.Payload.AuthorizedAccountID)).Return(aaObj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	mds.EXPECT().OpsCluster().Return(oCL).MinTimes(1)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(2)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	cntRLock := hc.cntRLock
	cntRUnlock := hc.cntRUnlock
	assert.Nil(evM.InSSProps)
	assert.NotPanics(func() { ret = hc.poolCreate(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.PoolCreateCreated)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Len(evM.InSSProps, 4)
	assert.EqualValues("cspDomainId", evM.InSSProps["cspDomainID"])
	assert.EqualValues("clusterId", evM.InSSProps["clusterId"])
	assert.EqualValues("authorizedAccountID", evM.InSSProps["authorizedAccountId"])
	assert.EqualValues("objectID", evM.InSSProps["meta.id"])
	assert.Equal(retObj, evM.InACScope)
	tl.Flush()

	// success with node scope, valid tenant account
	ai.AccountID = "accountID"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	params.Payload.AccountID = "ignored"
	params.Payload.AuthorizedAccountID = "accountID"
	params.Payload.CspStorageType = string(nstN)
	params.Payload.StorageAccessibility.AccessibilityScope = "NODE"
	params.Payload.StorageAccessibility.AccessibilityScopeObjID = "nodeId"
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oP = mock.NewMockPoolOps(mockCtrl)
	oP.EXPECT().Create(ctx, params.Payload).Return(obj, nil)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, string(params.Payload.CspDomainID)).Return(cspObj, nil)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(params.Payload.ClusterID)).Return(clObj, nil)
	oNode := mock.NewMockNodeOps(mockCtrl)
	oNode.EXPECT().Fetch(ctx, "nodeId").Return(nodeObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	mds.EXPECT().OpsCluster().Return(oCL).MinTimes(1)
	mds.EXPECT().OpsNode().Return(oNode).MinTimes(1)
	t.Log("case: success")
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.poolCreate(params) })
	assert.NotNil(ret)
	_, ok = ret.(*ops.PoolCreateCreated)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	tl.Flush()

	// Create failed, also test correct CSPDOMAIN accessibility, authorized subordinate
	params.Payload.AuthorizedAccountID = "authorizedAccountID"
	params.Payload.CspStorageType = string(cstN)
	params.Payload.StorageAccessibility.AccessibilityScope = "CSPDOMAIN"
	params.Payload.StorageAccessibility.AccessibilityScopeObjID = params.Payload.CspDomainID
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oP = mock.NewMockPoolOps(mockCtrl)
	oP.EXPECT().Create(ctx, params.Payload).Return(nil, centrald.ErrorExists)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, string(params.Payload.CspDomainID)).Return(cspObj, nil)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(params.Payload.AuthorizedAccountID)).Return(aaObj, nil)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(params.Payload.ClusterID)).Return(clObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	mds.EXPECT().OpsCluster().Return(oCL).MinTimes(1)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	t.Log("case: Create fails")
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.poolCreate(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.PoolCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorExists.M, *mE.Payload.Message)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	tl.Flush()

	// wrong tenant for csp domain
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: wrong tenant for csp domain")
	ai.AccountID = "other"
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, string(params.Payload.CspDomainID)).Return(cspObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.poolCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.PoolCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	tl.Flush()

	// Create failed due to NODE scope fetch failure
	ai.AccountID, ai.RoleObj = "", nil
	params.Payload.AccountID = "accountID"
	params.Payload.CspStorageType = string(nstN)
	params.Payload.StorageAccessibility.AccessibilityScope = "NODE"
	params.Payload.StorageAccessibility.AccessibilityScopeObjID = "nodeId"
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, string(params.Payload.CspDomainID)).Return(cspObj, nil)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(params.Payload.ClusterID)).Return(clObj, nil)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(params.Payload.AccountID)).Return(aObj, nil)
	oA.EXPECT().Fetch(ctx, string(params.Payload.AuthorizedAccountID)).Return(aaObj, nil)
	oNode = mock.NewMockNodeOps(mockCtrl)
	oNode.EXPECT().Fetch(ctx, "nodeId").Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	mds.EXPECT().OpsCluster().Return(oCL).MinTimes(1)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(2)
	mds.EXPECT().OpsNode().Return(oNode).MinTimes(1)
	hc.DS = mds
	t.Log("case: NODE scope db error")
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.poolCreate(params) })
	assert.NotNil(ret)
	_, ok = ret.(*ops.PoolCreateCreated)
	assert.False(ok)
	mE, ok = ret.(*ops.PoolCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, swag.StringValue(mE.Payload.Message))
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	tl.Flush()

	// Create failed due to NODE scope node not found
	params.Payload.CspStorageType = string(nstN)
	params.Payload.StorageAccessibility.AccessibilityScope = "NODE"
	params.Payload.StorageAccessibility.AccessibilityScopeObjID = "nodeId"
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, string(params.Payload.CspDomainID)).Return(cspObj, nil)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(params.Payload.ClusterID)).Return(clObj, nil)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(params.Payload.AccountID)).Return(aObj, nil)
	oA.EXPECT().Fetch(ctx, string(params.Payload.AuthorizedAccountID)).Return(aaObj, nil)
	oNode = mock.NewMockNodeOps(mockCtrl)
	oNode.EXPECT().Fetch(ctx, "nodeId").Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	mds.EXPECT().OpsCluster().Return(oCL).MinTimes(1)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(2)
	mds.EXPECT().OpsNode().Return(oNode).MinTimes(1)
	hc.DS = mds
	t.Log("case: NODE scope node not found")
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.poolCreate(params) })
	assert.NotNil(ret)
	_, ok = ret.(*ops.PoolCreateCreated)
	assert.False(ok)
	mE, ok = ret.(*ops.PoolCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, swag.StringValue(mE.Payload.Message))
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	tl.Flush()

	// missing/invalid property failure cases
	tcs := []string{"cspDomainId", "empty cspStorageType", "invalid cspStorageType", "storageAccessibility",
		"cspDomainId lookup", "bad scope", "bad CSPDOMAIN scope", "bad domain type",
		"badAccountID", "badAuthorizedAccountID", "wrongAuthorizedAccountID", "badClusterID", "wrongCluster"}
	for _, tc := range tcs {
		cl := *params.Payload // copy
		switch tc {
		case "cspDomainId":
			cl.CspDomainID = ""
		case "empty cspStorageType":
			cl.CspStorageType = ""
		case "invalid cspStorageType":
			cl.CspStorageType += "xxx"
		case "storageAccessibility":
			cl.StorageAccessibility = nil
		case "cspDomainId lookup":
			cl.CspStorageType = string(cstN)
			cl.StorageAccessibility = &models.StorageAccessibilityMutable{
				AccessibilityScope:      "CSPDOMAIN",
				AccessibilityScopeObjID: params.Payload.CspDomainID,
			}
			mockCtrl.Finish()
			mockCtrl = gomock.NewController(t)
			oCSP = mock.NewMockCspDomainOps(mockCtrl)
			oCSP.EXPECT().Fetch(ctx, string(params.Payload.CspDomainID)).Return(nil, centrald.ErrorNotFound)
			mds = mock.NewMockDataStore(mockCtrl)
			mds.EXPECT().OpsCspDomain().Return(oCSP)
			hc.DS = mds
		case "bad scope":
			cl.StorageAccessibility = &models.StorageAccessibilityMutable{
				AccessibilityScope:      "INVALID",
				AccessibilityScopeObjID: "not used",
			}
		case "bad CSPDOMAIN scope":
			cl.StorageAccessibility = &models.StorageAccessibilityMutable{
				AccessibilityScope:      "CSPDOMAIN",
				AccessibilityScopeObjID: "invalid",
			}
		case "bad domain type":
			badCspObj := &models.CSPDomain{}
			testutils.Clone(cspObj, badCspObj)
			badCspObj.CspDomainType = "xxx"
			mockCtrl.Finish()
			mockCtrl = gomock.NewController(t)
			oCSP = mock.NewMockCspDomainOps(mockCtrl)
			oCSP.EXPECT().Fetch(ctx, string(params.Payload.CspDomainID)).Return(badCspObj, nil)
			oCL = mock.NewMockClusterOps(mockCtrl)
			oCL.EXPECT().Fetch(ctx, string(params.Payload.ClusterID)).Return(clObj, nil)
			oA = mock.NewMockAccountOps(mockCtrl)
			oA.EXPECT().Fetch(ctx, string(params.Payload.AccountID)).Return(aObj, nil)
			oA.EXPECT().Fetch(ctx, string(params.Payload.AuthorizedAccountID)).Return(aaObj, nil)
			mds = mock.NewMockDataStore(mockCtrl)
			mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
			mds.EXPECT().OpsCluster().Return(oCL).MinTimes(1)
			mds.EXPECT().OpsAccount().Return(oA).MinTimes(2)
			hc.DS = mds
		case "badAccountID":
			mockCtrl.Finish()
			mockCtrl = gomock.NewController(t)
			oCSP = mock.NewMockCspDomainOps(mockCtrl)
			oCSP.EXPECT().Fetch(ctx, string(params.Payload.CspDomainID)).Return(cspObj, nil)
			oA = mock.NewMockAccountOps(mockCtrl)
			oA.EXPECT().Fetch(ctx, string(params.Payload.AccountID)).Return(nil, centrald.ErrorNotFound)
			mds = mock.NewMockDataStore(mockCtrl)
			mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
			mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
			hc.DS = mds
		case "badAuthorizedAccountID":
			mockCtrl.Finish()
			mockCtrl = gomock.NewController(t)
			oCSP = mock.NewMockCspDomainOps(mockCtrl)
			oCSP.EXPECT().Fetch(ctx, string(params.Payload.CspDomainID)).Return(cspObj, nil)
			oA = mock.NewMockAccountOps(mockCtrl)
			oA.EXPECT().Fetch(ctx, string(params.Payload.AccountID)).Return(aObj, nil)
			oA.EXPECT().Fetch(ctx, string(params.Payload.AuthorizedAccountID)).Return(nil, centrald.ErrorNotFound)
			mds = mock.NewMockDataStore(mockCtrl)
			mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
			mds.EXPECT().OpsAccount().Return(oA).MinTimes(2)
			hc.DS = mds
		case "wrongAuthorizedAccountID":
			wrongAA := &models.Account{}
			testutils.Clone(aaObj, wrongAA)
			wrongAA.TenantAccountID = "other"
			mockCtrl.Finish()
			mockCtrl = gomock.NewController(t)
			oCSP = mock.NewMockCspDomainOps(mockCtrl)
			oCSP.EXPECT().Fetch(ctx, string(params.Payload.CspDomainID)).Return(cspObj, nil)
			oA = mock.NewMockAccountOps(mockCtrl)
			oA.EXPECT().Fetch(ctx, string(params.Payload.AccountID)).Return(aObj, nil)
			oA.EXPECT().Fetch(ctx, string(params.Payload.AuthorizedAccountID)).Return(wrongAA, nil)
			mds = mock.NewMockDataStore(mockCtrl)
			mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
			mds.EXPECT().OpsAccount().Return(oA).MinTimes(2)
			hc.DS = mds
		case "badClusterID":
			mockCtrl.Finish()
			mockCtrl = gomock.NewController(t)
			oCSP = mock.NewMockCspDomainOps(mockCtrl)
			oCSP.EXPECT().Fetch(ctx, string(params.Payload.CspDomainID)).Return(cspObj, nil)
			oA = mock.NewMockAccountOps(mockCtrl)
			oA.EXPECT().Fetch(ctx, string(params.Payload.AccountID)).Return(aObj, nil)
			oA.EXPECT().Fetch(ctx, string(params.Payload.AuthorizedAccountID)).Return(aaObj, nil)
			oCL = mock.NewMockClusterOps(mockCtrl)
			oCL.EXPECT().Fetch(ctx, string(params.Payload.ClusterID)).Return(nil, centrald.ErrorNotFound)
			mds = mock.NewMockDataStore(mockCtrl)
			mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
			mds.EXPECT().OpsAccount().Return(oA).MinTimes(2)
			mds.EXPECT().OpsCluster().Return(oCL).MinTimes(1)
			hc.DS = mds
		case "wrongCluster":
			mockCtrl.Finish()
			mockCtrl = gomock.NewController(t)
			oCSP = mock.NewMockCspDomainOps(mockCtrl)
			oCSP.EXPECT().Fetch(ctx, string(params.Payload.CspDomainID)).Return(cspObj, nil)
			oA = mock.NewMockAccountOps(mockCtrl)
			oA.EXPECT().Fetch(ctx, string(params.Payload.AccountID)).Return(aObj, nil)
			oA.EXPECT().Fetch(ctx, string(params.Payload.AuthorizedAccountID)).Return(aaObj, nil)
			oCL = mock.NewMockClusterOps(mockCtrl)
			clObj2 := models.Cluster{}
			testutils.Clone(clObj, &clObj2)
			clObj2.CspDomainID = params.Payload.CspDomainID + "foo"
			oCL.EXPECT().Fetch(ctx, string(params.Payload.ClusterID)).Return(&clObj2, nil)
			mds = mock.NewMockDataStore(mockCtrl)
			mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
			mds.EXPECT().OpsAccount().Return(oA).MinTimes(2)
			mds.EXPECT().OpsCluster().Return(oCL).MinTimes(1)
			hc.DS = mds
		default:
			assert.False(true)
		}
		tl.Logger().Info("---------------------------------------------------")
		tl.Logger().Info("case: PoolCreate error case: " + tc)
		tl.Flush()
		cntRLock = hc.cntRLock
		cntRUnlock = hc.cntRUnlock
		assert.NotPanics(func() {
			ret = hc.poolCreate(ops.PoolCreateParams{HTTPRequest: requestWithAuthContext(ai), Payload: &cl})
		})
		assert.NotNil(ret)
		_, ok = ret.(*ops.PoolCreateCreated)
		assert.False(ok)
		mE, ok := ret.(*ops.PoolCreateDefault)
		assert.True(ok)
		assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
		assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
		if util.Contains([]string{"cspDomainId lookup", "bad domain type", "badAccountID", "badAuthorizedAccountID", "badClusterID", "wrongAuthorizedAccountID", "wrongCluster"}, tc) {
			cntRLock++
			cntRUnlock++
		}
		assert.Equal(cntRLock, hc.cntRLock, tc)
		assert.Equal(cntRUnlock, hc.cntRUnlock, tc)
	}

	t.Log("case: not authorized tenant")
	ai2 := &auth.Info{
		AccountID: "other",
		RoleObj:   &models.Role{},
	}
	params.HTTPRequest = requestWithAuthContext(ai2)
	assert.NotPanics(func() { ret = hc.poolCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.PoolCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.poolCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.PoolCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	// no payload
	params.Payload = nil
	t.Log("case: nil payload")
	assert.NotPanics(func() { ret = hc.poolCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.PoolCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorMissing.M, *mE.Payload.Message)
}

func TestPoolDelete(t *testing.T) {
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
	ai := &auth.Info{}
	params := ops.PoolDeleteParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.Pool{
		PoolAllOf0: models.PoolAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("objectID"),
			},
		},
		PoolMutable: models.PoolMutable{},
	}
	obj.CspDomainID = "cspDomainId"
	obj.ClusterID = "clusterId"
	obj.AccountID = "aid1"
	obj.AuthorizedAccountID = "authorizedAccountId"
	srlParams := storage_request.StorageRequestListParams{PoolID: &params.ID, IsTerminated: swag.Bool(false)}
	vsParams := volume_series.VolumeSeriesListParams{PoolID: &params.ID}
	vrlParams := volume_series_request.VolumeSeriesRequestListParams{PoolID: &params.ID, IsTerminated: swag.Bool(false)}
	spaLParams := service_plan_allocation.ServicePlanAllocationListParams{PoolID: &params.ID}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oP := mock.NewMockPoolOps(mockCtrl)
	oP.EXPECT().Delete(ctx, params.ID).Return(nil)
	oP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oSPA := mock.NewMockServicePlanAllocationOps(mockCtrl)
	oSPA.EXPECT().Count(ctx, spaLParams, uint(1)).Return(0, nil)
	oSR := mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().Count(ctx, srlParams, uint(1)).Return(0, nil)
	oS := mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Count(ctx, storage.StorageListParams{PoolID: &params.ID}, uint(1)).Return(0, nil)
	oV := mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Count(ctx, vsParams, uint(1)).Return(0, nil)
	oVR := mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Count(ctx, vrlParams, uint(1)).Return(0, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
	mds.EXPECT().OpsStorageRequest().Return(oSR)
	mds.EXPECT().OpsStorage().Return(oS)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	cntLock := hc.cntLock
	cntUnlock := hc.cntUnlock
	assert.Nil(evM.InSSProps)
	assert.NotPanics(func() { ret = hc.poolDelete(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.PoolDeleteNoContent)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Len(evM.InSSProps, 3)
	assert.EqualValues("cspDomainId", evM.InSSProps["cspDomainID"])
	assert.EqualValues("clusterId", evM.InSSProps["clusterId"])
	assert.EqualValues("authorizedAccountId", evM.InSSProps["authorizedAccountId"])
	assert.Equal(obj, evM.InACScope)
	tl.Flush()

	// delete failure case, cover valid tenant account
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: delete failure")
	ai.AccountID = "aid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	oP = mock.NewMockPoolOps(mockCtrl)
	oP.EXPECT().Delete(ctx, params.ID).Return(centrald.ErrorDbError)
	oP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
	oSPA.EXPECT().Count(ctx, spaLParams, uint(1)).Return(0, nil)
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().Count(ctx, srlParams, uint(1)).Return(0, nil)
	oS = mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Count(ctx, storage.StorageListParams{PoolID: &params.ID}, uint(1)).Return(0, nil)
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Count(ctx, vsParams, uint(1)).Return(0, nil)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Count(ctx, vrlParams, uint(1)).Return(0, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
	mds.EXPECT().OpsStorageRequest().Return(oSR)
	mds.EXPECT().OpsStorage().Return(oS)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.poolDelete(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.PoolDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	// not authorized tenant admin
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized tenant admin")
	ai.AccountID = "otherID"
	mds = mock.NewMockDataStore(mockCtrl)
	oP = mock.NewMockPoolOps(mockCtrl)
	oP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsPool().Return(oP)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.poolDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.PoolDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Regexp(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	ai.RoleObj = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.poolDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.PoolDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	// cascading validation failure cases
	tObj := []string{"storage request", "storage", "volume series", "volume series request", "service plan allocation"}
	for tc := 0; tc < len(tObj)*2; tc++ {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		t.Log("case: " + tObj[tc/2] + []string{" count fails", " exists"}[tc%2])
		mds = mock.NewMockDataStore(mockCtrl)
		oP = mock.NewMockPoolOps(mockCtrl)
		oP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
		mds.EXPECT().OpsPool().Return(oP)
		count, err := tc%2, []error{centrald.ErrorDbError, nil}[tc%2]
		switch tc / 2 {
		case 4:
			oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
			oSPA.EXPECT().Count(ctx, spaLParams, uint(1)).Return(count, err)
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
			oV = mock.NewMockVolumeSeriesOps(mockCtrl)
			oV.EXPECT().Count(ctx, vsParams, uint(1)).Return(count, err)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			count, err = 0, nil
			fallthrough
		case 1:
			oS = mock.NewMockStorageOps(mockCtrl)
			oS.EXPECT().Count(ctx, storage.StorageListParams{PoolID: &params.ID}, uint(1)).Return(count, err)
			mds.EXPECT().OpsStorage().Return(oS)
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
		assert.NotPanics(func() { ret = hc.poolDelete(params) })
		assert.NotNil(ret)
		mE, ok = ret.(*ops.PoolDeleteDefault)
		if assert.True(ok) {
			if tc%2 == 0 {
				assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
				assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
			} else {
				assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
				assert.Regexp(tObj[tc/2]+".* the pool", *mE.Payload.Message)
			}
		}
		assert.Equal(cntLock+1, hc.cntLock)
		assert.Equal(cntUnlock+1, hc.cntUnlock)
		tl.Flush()
	}

	// fetch failure case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oP = mock.NewMockPoolOps(mockCtrl)
	oP.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
	t.Log("case: fetch failure")
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.poolDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.PoolDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
	assert.Equal(cntLock, hc.cntLock)
	assert.Equal(cntUnlock, hc.cntUnlock)
}

func TestPoolFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.PoolFetchParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.Pool{
		PoolAllOf0: models.PoolAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("objectID"),
			},
		},
		PoolMutable: models.PoolMutable{},
	}
	obj.AccountID = "tid1"
	obj.AuthorizedAccountID = "aid1"

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oP := mock.NewMockPoolOps(mockCtrl)
	oP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.poolFetch(params) })
	assert.NotNil(ret)
	mR, ok := ret.(*ops.PoolFetchOK)
	assert.True(ok)
	assert.Equal(obj, mR.Payload)
	tl.Flush()

	// fetch failure, cover authorized account
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: fetch failure")
	ai.AccountID = "aid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	oP = mock.NewMockPoolOps(mockCtrl)
	oP.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.poolFetch(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.PoolFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: unauthorized")
	ai.AccountID = "aid2"
	oP = mock.NewMockPoolOps(mockCtrl)
	oP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.poolFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.PoolFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.poolFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.PoolFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
}

func TestPoolList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.PoolListParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	ctx := params.HTTPRequest.Context()
	objects := []*models.Pool{
		&models.Pool{
			PoolAllOf0: models.PoolAllOf0{
				Meta: &models.ObjMeta{
					ID: models.ObjID("objectID"),
				},
			},
			PoolCreateOnce: models.PoolCreateOnce{
				AccountID:           "aid1",
				AuthorizedAccountID: "aid2",
			},
		},
		&models.Pool{
			PoolAllOf0: models.PoolAllOf0{
				Meta: &models.ObjMeta{
					ID: models.ObjID("objectID2"),
				},
			},
			PoolCreateOnce: models.PoolCreateOnce{
				AccountID:           "aid1",
				AuthorizedAccountID: "aid1",
			},
		},
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	t.Log("case: success")
	oP := mock.NewMockPoolOps(mockCtrl)
	oP.EXPECT().List(ctx, params).Return(objects, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.poolList(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.PoolListOK)
	assert.True(ok)
	assert.Equal(objects, mO.Payload)
	tl.Flush()

	// list fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: List failure")
	oP = mock.NewMockPoolOps(mockCtrl)
	oP.EXPECT().List(ctx, params).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.poolList(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.PoolListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.poolList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.PoolListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: unauthorized filtered")
	oP = mock.NewMockPoolOps(mockCtrl)
	oP.EXPECT().List(ctx, params).Return(objects, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP)
	hc.DS = mds
	ai.AccountID = "aid2"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	assert.NotPanics(func() { ret = hc.poolList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.PoolListOK)
	assert.True(ok)
	if assert.Len(mO.Payload, 1) {
		assert.EqualValues("objectID", mO.Payload[0].Meta.ID)
	}
}

func TestPoolUpdate(t *testing.T) {
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
	cspDomainTypes := app.SupportedCspDomainTypes()
	assert.NotNil(cspDomainTypes)
	assert.NotEmpty(cspDomainTypes)
	dt := models.CspDomainTypeMutable("AWS")
	assert.Contains(cspDomainTypes, dt)
	cspStorageTypes := app.SupportedCspStorageTypes()
	var stN models.CspStorageType
	minSize := int64(0)
	for _, st := range cspStorageTypes {
		if s := swag.Int64Value(st.MinAllocationSizeBytes); s > minSize {
			minSize = s
		}
		if st.CspDomainType == models.CspDomainType(dt) {
			stN = st.Name
		}
	}
	assert.NotZero(stN)

	// make the name map
	var nMap JSONToAttrNameMap
	assert.NotPanics(func() { nMap = hc.poolMutableNameMap() })

	// parse params
	objM := &models.PoolMutable{}
	ai := &auth.Info{}
	params := ops.PoolUpdateParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
		Version:     swag.Int32(8),
		Remove:      []string{},
		Append:      []string{},
		Set:         []string{nMap.jName("SystemTags"), nMap.jName("ServicePlanReservations")},
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

	// success, several cases using set
	obj := &models.Pool{}
	obj.Meta = &models.ObjMeta{
		ID:      models.ObjID(params.ID),
		Version: 8,
	}
	obj.CspDomainID = "cspDomainId"
	obj.ClusterID = "clusterId"
	obj.AccountID = "tid1"
	obj.AuthorizedAccountID = "authorizedAccountId"
	obj.CspStorageType = string(stN)
	assert.Equal(params.Set[0], "systemTags")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	var ret middleware.Responder

	mockCtrl = gomock.NewController(t)
	oP := mock.NewMockPoolOps(mockCtrl)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
	params.Set[0] = "systemTags"
	params.Version = swag.Int32(int32(obj.Meta.Version))
	oP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oP.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	t.Log("case: update success")
	hc.DS = mds
	evM.InSSProps = nil
	assert.NotPanics(func() { ret = hc.poolUpdate(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.PoolUpdateOK)
	assert.True(ok)
	assert.Equal(obj, mO.Payload)
	assert.Len(evM.InSSProps, 3)
	assert.EqualValues("cspDomainId", evM.InSSProps["cspDomainID"])
	assert.EqualValues("clusterId", evM.InSSProps["clusterId"])
	assert.EqualValues("authorizedAccountId", evM.InSSProps["authorizedAccountId"])
	assert.Equal(obj, evM.InACScope)
	tl.Flush()

	// Fetch failed
	mockCtrl.Finish()
	params.Set[0] = "systemTags"
	uP[centrald.UpdateSet] = params.Set
	params.Version = swag.Int32(int32(obj.Meta.Version))
	ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP)
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).Matcher()
	mockCtrl = gomock.NewController(t)
	oP = mock.NewMockPoolOps(mockCtrl)
	oP.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
	t.Log("case: update fail on Fetch")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.poolUpdate(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.PoolUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()

	// Update failed, also tests the no-version in query codepath
	mockCtrl.Finish()
	params.Version = nil
	ua2, err := hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP)
	assert.Nil(err)
	assert.NotNil(ua2)
	uaM2 := updateArgsMatcher(t, ua2).AddsVersion(int32(obj.Meta.Version)).Matcher()
	mockCtrl = gomock.NewController(t)
	oP = mock.NewMockPoolOps(mockCtrl)
	oP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oP.EXPECT().Update(ctx, uaM2, params.Payload).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
	t.Log("case: fail on Update")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.poolUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.PoolUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()

	// auth failures
	aiN := &auth.Info{
		AccountID: "authorizedAccountId",
		RoleObj:   &models.Role{},
	}
	aiN.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	for k := range nMap {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		t.Log("case: auth failure", k)
		params.Set = []string{k}
		mds = mock.NewMockDataStore(mockCtrl)
		params.HTTPRequest = requestWithAuthContext(aiN)
		ctx = params.HTTPRequest.Context()
		oP = mock.NewMockPoolOps(mockCtrl)
		oP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
		mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
		hc.DS = mds
		assert.NotPanics(func() { ret = hc.poolUpdate(params) })
		assert.NotNil(ret)
		mD, ok = ret.(*ops.PoolUpdateDefault)
		assert.True(ok)
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)
	}
	ai.RoleObj = nil

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.poolUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.PoolUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	// validation failures
	oVersion := obj.Meta.Version
	tcs := []string{"no change", "bad-version"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		mds = mock.NewMockDataStore(mockCtrl)
		oP = mock.NewMockPoolOps(mockCtrl)
		expErr := centrald.ErrorUpdateInvalidRequest
		switch tc {
		case "no change":
			params.Set = []string{}
		case "bad-version":
			params.Set = []string{"systemTags"}
			params.Version = swag.Int32(int32(obj.Meta.Version))
			obj.Meta.Version++
			mds.EXPECT().OpsPool().Return(oP)
			oP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			expErr = centrald.ErrorIDVerNotFound
		default:
			assert.False(true)
		}
		t.Log("case: update error:", tc)
		hc.DS = mds
		assert.NotPanics(func() { ret = hc.poolUpdate(params) })
		assert.NotNil(ret)
		mD, ok = ret.(*ops.PoolUpdateDefault)
		assert.True(ok)
		assert.Equal(expErr.C, int(mD.Payload.Code))
		assert.Regexp("^"+expErr.M, *mD.Payload.Message)
		obj.Meta.Version = oVersion
		tl.Flush()
	}

	// existing object fetch fails in this validation code path
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oP = mock.NewMockPoolOps(mockCtrl)
	oP.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP)
	t.Log("case: update attributes object fetch fails")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.poolUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.PoolUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()

	// no payload
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: no payload")
	mds = mock.NewMockDataStore(mockCtrl)
	params.Payload = nil
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.poolUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.PoolUpdateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
		assert.Equal(centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	}
}
