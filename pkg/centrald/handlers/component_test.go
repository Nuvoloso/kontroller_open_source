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

	"github.com/Nuvoloso/kontroller/pkg/auth"
	fa "github.com/Nuvoloso/kontroller/pkg/auth/fake"
	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/cluster"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	appAuth "github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	fhk "github.com/Nuvoloso/kontroller/pkg/housekeeping/fake"
	fmm "github.com/Nuvoloso/kontroller/pkg/metricmover/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
)

func TestComponentMethods(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	hc := newHandlerComp()
	assert.NotNil(hc.spaValidator)
	assert.NotNil(hc.ops)

	// Init
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mds := mock.NewMockDataStore(mockCtrl)
	app.DS = mds
	assert.NotPanics(func() { hc.Init(app) })
	assert.Equal(app.Log, hc.Log)
	assert.Equal(app, hc.app)
	assert.True(evM.CalledRH)
	assert.Equal(app.API, fMM.InRHApi)
	assert.Equal(hc, app.CrudHelpers)
	assert.NotNil(api.AccountAccountCreateHandler)
	assert.NotNil(api.AccountAccountDeleteHandler)
	assert.NotNil(api.AccountAccountFetchHandler)
	assert.NotNil(api.AccountAccountListHandler)
	assert.NotNil(api.AccountAccountProtectionDomainClearHandler)
	assert.NotNil(api.AccountAccountProtectionDomainSetHandler)
	assert.NotNil(api.AccountAccountSecretResetHandler)
	assert.NotNil(api.AccountAccountSecretRetrieveHandler)
	assert.NotNil(api.AccountAccountUpdateHandler)
	assert.NotNil(api.ApplicationGroupApplicationGroupCreateHandler)
	assert.NotNil(api.ApplicationGroupApplicationGroupDeleteHandler)
	assert.NotNil(api.ApplicationGroupApplicationGroupFetchHandler)
	assert.NotNil(api.ApplicationGroupApplicationGroupListHandler)
	assert.NotNil(api.ApplicationGroupApplicationGroupUpdateHandler)
	assert.NotNil(api.ClusterClusterAccountSecretFetchHandler)
	assert.NotNil(api.ClusterClusterCreateHandler)
	assert.NotNil(api.ClusterClusterDeleteHandler)
	assert.NotNil(api.ClusterClusterFetchHandler)
	assert.NotNil(api.ClusterClusterListHandler)
	assert.NotNil(api.ClusterClusterUpdateHandler)
	assert.NotNil(api.ConsistencyGroupConsistencyGroupCreateHandler)
	assert.NotNil(api.ConsistencyGroupConsistencyGroupDeleteHandler)
	assert.NotNil(api.ConsistencyGroupConsistencyGroupFetchHandler)
	assert.NotNil(api.ConsistencyGroupConsistencyGroupListHandler)
	assert.NotNil(api.ConsistencyGroupConsistencyGroupUpdateHandler)
	assert.NotNil(api.CspCredentialCspCredentialCreateHandler)
	assert.NotNil(api.CspCredentialCspCredentialDeleteHandler)
	assert.NotNil(api.CspCredentialCspCredentialFetchHandler)
	assert.NotNil(api.CspCredentialCspCredentialListHandler)
	assert.NotNil(api.CspCredentialCspCredentialUpdateHandler)
	assert.NotNil(api.CspDomainCspDomainCreateHandler)
	assert.NotNil(api.CspDomainCspDomainDeleteHandler)
	assert.NotNil(api.CspDomainCspDomainFetchHandler)
	assert.NotNil(api.CspDomainCspDomainListHandler)
	assert.NotNil(api.CspDomainCspDomainServicePlanCostHandler)
	assert.NotNil(api.CspDomainCspDomainUpdateHandler)
	assert.NotNil(api.CspStorageTypeCspStorageTypeFetchHandler)
	assert.NotNil(api.CspStorageTypeCspStorageTypeListHandler)
	assert.NotNil(api.NodeNodeCreateHandler)
	assert.NotNil(api.NodeNodeDeleteHandler)
	assert.NotNil(api.NodeNodeFetchHandler)
	assert.NotNil(api.NodeNodeListHandler)
	assert.NotNil(api.NodeNodeUpdateHandler)
	assert.NotNil(api.PoolPoolCreateHandler)
	assert.NotNil(api.PoolPoolDeleteHandler)
	assert.NotNil(api.PoolPoolFetchHandler)
	assert.NotNil(api.PoolPoolListHandler)
	assert.NotNil(api.PoolPoolUpdateHandler)
	assert.NotNil(api.ProtectionDomainProtectionDomainCreateHandler)
	assert.NotNil(api.ProtectionDomainProtectionDomainDeleteHandler)
	assert.NotNil(api.ProtectionDomainProtectionDomainFetchHandler)
	assert.NotNil(api.ProtectionDomainProtectionDomainListHandler)
	assert.NotNil(api.ProtectionDomainProtectionDomainMetadataHandler)
	assert.NotNil(api.ProtectionDomainProtectionDomainUpdateHandler)
	assert.NotNil(api.ServiceDebugDebugPostHandler)
	assert.NotNil(api.ServicePlanAllocationServicePlanAllocationCustomizeProvisioningHandler)
	assert.NotNil(api.ServicePlanServicePlanCloneHandler)
	assert.NotNil(api.ServicePlanServicePlanDeleteHandler)
	assert.NotNil(api.ServicePlanServicePlanFetchHandler)
	assert.NotNil(api.ServicePlanServicePlanListHandler)
	assert.NotNil(api.ServicePlanServicePlanPublishHandler)
	assert.NotNil(api.ServicePlanServicePlanRetireHandler)
	assert.NotNil(api.ServicePlanServicePlanUpdateHandler)
	assert.NotNil(api.SloSloListHandler)
	assert.NotNil(api.SnapshotSnapshotCreateHandler)
	assert.NotNil(api.SnapshotSnapshotDeleteHandler)
	assert.NotNil(api.SnapshotSnapshotFetchHandler)
	assert.NotNil(api.SnapshotSnapshotListHandler)
	assert.NotNil(api.SnapshotSnapshotUpdateHandler)
	assert.NotNil(api.StorageFormulaStorageFormulaListHandler)
	assert.NotNil(api.StorageRequestStorageRequestCreateHandler)
	assert.NotNil(api.StorageRequestStorageRequestDeleteHandler)
	assert.NotNil(api.StorageRequestStorageRequestFetchHandler)
	assert.NotNil(api.StorageRequestStorageRequestListHandler)
	assert.NotNil(api.StorageRequestStorageRequestUpdateHandler)
	assert.NotNil(api.StorageStorageCreateHandler)
	assert.NotNil(api.StorageStorageDeleteHandler)
	assert.NotNil(api.StorageStorageFetchHandler)
	assert.NotNil(api.StorageStorageListHandler)
	assert.NotNil(api.StorageStorageUpdateHandler)
	assert.NotNil(api.SystemSystemFetchHandler)
	assert.NotNil(api.SystemSystemUpdateHandler)
	assert.NotNil(api.SystemSystemHostnameFetchHandler)
	assert.NotNil(api.VolumeSeriesRequestVolumeSeriesRequestCancelHandler)
	assert.NotNil(api.VolumeSeriesRequestVolumeSeriesRequestCreateHandler)
	assert.NotNil(api.VolumeSeriesRequestVolumeSeriesRequestDeleteHandler)
	assert.NotNil(api.VolumeSeriesRequestVolumeSeriesRequestFetchHandler)
	assert.NotNil(api.VolumeSeriesRequestVolumeSeriesRequestListHandler)
	assert.NotNil(api.VolumeSeriesRequestVolumeSeriesRequestUpdateHandler)
	assert.NotNil(api.VolumeSeriesVolumeSeriesCreateHandler)
	assert.NotNil(api.VolumeSeriesVolumeSeriesDeleteHandler)
	assert.NotNil(api.VolumeSeriesVolumeSeriesFetchHandler)
	assert.NotNil(api.VolumeSeriesVolumeSeriesListHandler)
	assert.NotNil(api.VolumeSeriesVolumeSeriesNewIDHandler)
	assert.NotNil(api.VolumeSeriesVolumeSeriesPVSpecFetchHandler)
	assert.NotNil(api.VolumeSeriesVolumeSeriesUpdateHandler)

	// Task scheduler handlers registered
	fts, ok := app.TaskScheduler.(*fhk.TaskScheduler)
	assert.True(ok)
	assert.NotNil(fts)
	assert.NotNil(fts.InRhAPI)
	assert.Equal(hc, fts.InRhAM)

	assert.NotPanics(func() { hc.Start() })
	assert.NotPanics(func() { hc.Stop() })

	// StdUpdateArgs default behavior
	ua, err := hc.MakeStdUpdateArgs(nil, "", nil, [centrald.NumActionTypes][]string{})
	assert.Error(err)
	assert.Regexp("object not externalized", err)
	assert.Nil(ua)
}

func TestUUIDGenerator(t *testing.T) {
	assert := assert.New(t)

	uuidS := uuidGenerator()
	assert.NotEmpty(uuidS)
	uuidV, err := uuid.FromString(uuidS)
	assert.NoError(err)
	assert.Equal(uuidS, uuidV.String())
}

func TestGetAuthInfo(t *testing.T) {
	assert := assert.New(t)
	hc := newHandlerComp()

	ret, err := hc.GetAuthInfo(&http.Request{})
	assert.Nil(ret)
	assert.Equal(centrald.ErrorInternalError, err)

	ai := &appAuth.Info{}
	ret, err = hc.GetAuthInfo(requestWithAuthContext(ai))
	assert.True(ai == ret)
	assert.NoError(err)
}

func TestConstrainBothQueryAccounts(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()

	aRole := &M.Role{
		RoleMutable: M.RoleMutable{
			Capabilities: map[string]bool{centrald.CSPDomainUsageCap: true},
		},
	}
	aAI := &appAuth.Info{AccountID: "a1", TenantAccountID: "t1", RoleObj: aRole}
	tRole := &M.Role{
		RoleMutable: M.RoleMutable{
			Capabilities: map[string]bool{centrald.CSPDomainManagementCap: true, centrald.CSPDomainUsageCap: true},
		},
	}
	tAI := &appAuth.Info{AccountID: "t1", RoleObj: tRole}

	t.Log("success: internal caller")
	ai := &appAuth.Info{}
	aID, tID := swag.String("a1"), swag.String("t1")
	raID, rtID, skip := hc.constrainBothQueryAccounts(ai, aID, "", tID, "")
	assert.False(skip)
	assert.Equal(aID, raID)
	assert.Equal(tID, rtID)

	t.Log("success: skip for no capabilities")
	ai = &appAuth.Info{AccountID: "t1", RoleObj: &M.Role{}}
	raID, rtID, skip = hc.constrainBothQueryAccounts(ai, aID, "", tID, "")
	assert.True(skip)
	assert.Nil(raID)
	assert.Nil(rtID)

	t.Log("case: no accounts and caller is tenant admin")
	raID, rtID, skip = hc.constrainBothQueryAccounts(tAI, nil, centrald.CSPDomainUsageCap, nil, centrald.CSPDomainManagementCap)
	assert.False(skip)
	assert.Nil(raID)
	assert.Equal(tID, rtID)

	t.Log("case: no accounts and caller is normal account")
	raID, rtID, skip = hc.constrainBothQueryAccounts(aAI, nil, centrald.CSPDomainUsageCap, nil, centrald.CSPDomainManagementCap)
	assert.False(skip)
	assert.Equal(aID, raID)
	assert.Nil(rtID)

	t.Log("case: tenant account and caller is tenant admin")
	raID, rtID, skip = hc.constrainBothQueryAccounts(tAI, nil, centrald.CSPDomainUsageCap, tID, centrald.CSPDomainManagementCap)
	assert.False(skip)
	assert.Nil(raID)
	assert.Equal(tID, rtID)

	t.Log("case: wrong tenant account and caller is tenant admin")
	raID, rtID, skip = hc.constrainBothQueryAccounts(tAI, nil, centrald.CSPDomainUsageCap, swag.String("t2"), centrald.CSPDomainManagementCap)
	assert.True(skip)
	assert.Nil(raID)
	assert.Nil(rtID)

	t.Log("case: tenant auth account and caller is tenant admin")
	raID, rtID, skip = hc.constrainBothQueryAccounts(tAI, tID, centrald.CSPDomainUsageCap, nil, centrald.CSPDomainManagementCap)
	assert.False(skip)
	assert.Equal(tID, raID)
	assert.Nil(rtID)

	t.Log("case: both accounts and caller is tenant admin")
	raID, rtID, skip = hc.constrainBothQueryAccounts(tAI, tID, centrald.CSPDomainUsageCap, tID, centrald.CSPDomainManagementCap)
	assert.False(skip)
	assert.Equal(tID, raID)
	assert.Equal(tID, rtID)

	t.Log("case: subordinate auth account and caller is tenant admin")
	raID, rtID, skip = hc.constrainBothQueryAccounts(tAI, aID, centrald.CSPDomainUsageCap, nil, centrald.CSPDomainManagementCap)
	assert.False(skip)
	assert.Equal(aID, raID)
	assert.Equal(tID, rtID)

	t.Log("case: auth account and caller is normal account")
	raID, rtID, skip = hc.constrainBothQueryAccounts(aAI, aID, centrald.CSPDomainUsageCap, nil, centrald.CSPDomainManagementCap)
	assert.False(skip)
	assert.Equal(aID, raID)
	assert.Nil(rtID)

	t.Log("case: wrong auth account and caller is normal account")
	raID, rtID, skip = hc.constrainBothQueryAccounts(aAI, swag.String("a2"), centrald.CSPDomainUsageCap, nil, centrald.CSPDomainManagementCap)
	assert.True(skip)
	assert.Nil(raID)
	assert.Nil(rtID)

	t.Log("case: both accounts and caller is normal account")
	raID, rtID, skip = hc.constrainBothQueryAccounts(aAI, aID, centrald.CSPDomainUsageCap, tID, centrald.CSPDomainManagementCap)
	assert.False(skip)
	assert.Equal(aID, raID)
	assert.Equal(tID, rtID)

	t.Log("case: tenant account and caller is normal account")
	raID, rtID, skip = hc.constrainBothQueryAccounts(aAI, nil, centrald.CSPDomainUsageCap, tID, centrald.CSPDomainManagementCap)
	assert.False(skip)
	assert.Equal(aID, raID)
	assert.Equal(tID, rtID)
}

func TestConstrainEitherOrQueryAccounts(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()

	aObj := &M.Account{
		AccountAllOf0: M.AccountAllOf0{
			Meta:            &M.ObjMeta{ID: "a1"},
			TenantAccountID: "t1",
		},
	}
	aRole := &M.Role{
		RoleMutable: M.RoleMutable{
			Capabilities: map[string]bool{centrald.CSPDomainUsageCap: true},
		},
	}
	aAI := &appAuth.Info{AccountID: "a1", TenantAccountID: "t1", RoleObj: aRole}
	tRole := &M.Role{
		RoleMutable: M.RoleMutable{
			Capabilities: map[string]bool{centrald.CSPDomainManagementCap: true, centrald.CSPDomainUsageCap: true},
		},
	}
	tAI := &appAuth.Info{AccountID: "t1", RoleObj: tRole}

	t.Log("success: internal caller")
	ctx := context.Background()
	ai := &appAuth.Info{}
	aID, tID := swag.String("a1"), swag.String("t1")
	raID, rtID, err := hc.constrainEitherOrQueryAccounts(ctx, ai, aID, "", tID, "")
	assert.NoError(err)
	assert.Equal(aID, raID)
	assert.Equal(tID, rtID)

	t.Log("case: no accounts and caller is tenant admin")
	raID, rtID, err = hc.constrainEitherOrQueryAccounts(ctx, tAI, nil, centrald.CSPDomainUsageCap, nil, centrald.CSPDomainManagementCap)
	assert.NoError(err)
	assert.Equal(tID, raID)
	assert.Equal(tID, rtID)

	t.Log("case: tenant account and caller is tenant admin")
	raID, rtID, err = hc.constrainEitherOrQueryAccounts(ctx, tAI, nil, centrald.CSPDomainUsageCap, tID, centrald.CSPDomainManagementCap)
	assert.NoError(err)
	assert.Nil(raID)
	assert.Equal(tID, rtID)

	t.Log("case: owner account and caller is tenant admin")
	raID, rtID, err = hc.constrainEitherOrQueryAccounts(ctx, tAI, tID, centrald.CSPDomainUsageCap, nil, centrald.CSPDomainManagementCap)
	assert.NoError(err)
	assert.Equal(tID, raID)
	assert.Nil(rtID)

	t.Log("case: both accounts and caller is tenant admin")
	raID, rtID, err = hc.constrainEitherOrQueryAccounts(ctx, tAI, tID, centrald.CSPDomainUsageCap, tID, centrald.CSPDomainManagementCap)
	assert.NoError(err)
	assert.Equal(tID, raID)
	assert.Equal(tID, rtID)

	t.Log("case: subordinate account and caller is tenant admin")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mds := mock.NewMockDataStore(mockCtrl)
	oA := mock.NewMockAccountOps(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	oA.EXPECT().Fetch(ctx, *aID).Return(aObj, nil)
	hc.DS = mds
	raID, rtID, err = hc.constrainEitherOrQueryAccounts(ctx, tAI, aID, centrald.CSPDomainUsageCap, nil, centrald.CSPDomainManagementCap)
	assert.NoError(err)
	assert.Equal(aID, raID)
	assert.Nil(rtID)

	t.Log("case: owner account is not subordinate")
	aObj.TenantAccountID = ""
	oA.EXPECT().Fetch(ctx, *aID).Return(aObj, nil)
	raID, rtID, err = hc.constrainEitherOrQueryAccounts(ctx, tAI, aID, centrald.CSPDomainUsageCap, nil, centrald.CSPDomainManagementCap)
	assert.NoError(err)
	assert.Nil(raID)
	assert.Nil(rtID)

	t.Log("case: owner account is wrong subordinate")
	aObj.TenantAccountID = "t2"
	oA.EXPECT().Fetch(ctx, *aID).Return(aObj, nil)
	raID, rtID, err = hc.constrainEitherOrQueryAccounts(ctx, tAI, aID, centrald.CSPDomainUsageCap, nil, centrald.CSPDomainManagementCap)
	assert.NoError(err)
	assert.Nil(raID)
	assert.Nil(rtID)

	t.Log("case: owner account is not found")
	oA.EXPECT().Fetch(ctx, *aID).Return(nil, centrald.ErrorNotFound)
	raID, rtID, err = hc.constrainEitherOrQueryAccounts(ctx, tAI, aID, centrald.CSPDomainUsageCap, nil, centrald.CSPDomainManagementCap)
	assert.NoError(err)
	assert.Nil(raID)
	assert.Nil(rtID)

	t.Log("case: owner account lookup error")
	oA.EXPECT().Fetch(ctx, *aID).Return(nil, centrald.ErrorDbError)
	raID, rtID, err = hc.constrainEitherOrQueryAccounts(ctx, tAI, aID, centrald.CSPDomainUsageCap, nil, centrald.CSPDomainManagementCap)
	assert.Equal(centrald.ErrorDbError, err)
	assert.Nil(raID)
	assert.Nil(rtID)

	t.Log("case: no accounts and caller is normal account")
	raID, rtID, err = hc.constrainEitherOrQueryAccounts(ctx, aAI, nil, centrald.CSPDomainUsageCap, nil, centrald.CSPDomainManagementCap)
	assert.NoError(err)
	assert.Equal(aID, raID)
	assert.Nil(rtID)

	t.Log("case: owner account and caller is normal account")
	raID, rtID, err = hc.constrainEitherOrQueryAccounts(ctx, aAI, aID, centrald.CSPDomainUsageCap, nil, centrald.CSPDomainManagementCap)
	assert.NoError(err)
	assert.Equal(aID, raID)
	assert.Nil(rtID)

	t.Log("case: both accounts and caller is normal account")
	raID, rtID, err = hc.constrainEitherOrQueryAccounts(ctx, aAI, aID, centrald.CSPDomainUsageCap, tID, centrald.CSPDomainManagementCap)
	assert.NoError(err)
	assert.Equal(aID, raID)
	assert.Nil(rtID)

	t.Log("case: tenant account and caller is normal account")
	raID, rtID, err = hc.constrainEitherOrQueryAccounts(ctx, aAI, nil, centrald.CSPDomainUsageCap, tID, centrald.CSPDomainManagementCap)
	assert.NoError(err)
	assert.Nil(raID)
	assert.Nil(rtID)
}

func TestLookupClusterClient(t *testing.T) {
	assert := assert.New(t)
	hc := newHandlerComp()
	// empty map
	hc.clusterClientMap = nil
	ret := hc.lookupClusterClient("kubernetes")
	assert.NotNil(ret)
	assert.NotNil(hc.clusterClientMap)

	// already exists in map
	ret = hc.lookupClusterClient("kubernetes")
	assert.NotNil(ret)
	assert.NotNil(hc.clusterClientMap)

	// bad clustertype
	ret = hc.lookupClusterClient("badcluster")
	assert.Nil(ret)
}

func TestGetAuth(t *testing.T) {
	assert := assert.New(t)
	hc := newHandlerComp()

	ret, mErr := hc.GetAuth(&http.Request{})
	assert.Nil(ret)
	if assert.NotNil(mErr) && assert.NotNil(mErr.Message) {
		assert.EqualValues(centrald.ErrorInternalError.C, mErr.Code)
		assert.Equal(centrald.ErrorInternalError.M, *mErr.Message)
	}

	ai := &appAuth.Info{}
	ret, mErr = hc.GetAuth(requestWithAuthContext(ai))
	assert.True(ai == ret)
	assert.Nil(mErr)
}

func TestEventAllowed(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	ev := &crude.CrudEvent{}

	// objects that have filters, cover allowed and disallowed
	tcs := []interface{}{
		&M.Account{AccountAllOf0: M.AccountAllOf0{Meta: &M.ObjMeta{ID: "id1"}}},
		&M.ApplicationGroup{ApplicationGroupAllOf0: M.ApplicationGroupAllOf0{Meta: &M.ObjMeta{ID: "id1"}}},
		&M.Cluster{ClusterAllOf0: M.ClusterAllOf0{Meta: &M.ObjMeta{ID: "id1"}}},
		&M.ConsistencyGroup{ConsistencyGroupAllOf0: M.ConsistencyGroupAllOf0{Meta: &M.ObjMeta{ID: "id1"}}},
		&M.CSPCredential{CSPCredentialAllOf0: M.CSPCredentialAllOf0{Meta: &M.ObjMeta{ID: "id1"}}},
		&M.CSPDomain{CSPDomainAllOf0: M.CSPDomainAllOf0{Meta: &M.ObjMeta{ID: "id1"}}},
		&NodeAndCluster{
			// Node events use this custom object because access checks require both Node and Cluster
			Node:    &M.Node{NodeAllOf0: M.NodeAllOf0{Meta: &M.ObjMeta{ID: "id1"}}},
			Cluster: &M.Cluster{ClusterAllOf0: M.ClusterAllOf0{Meta: &M.ObjMeta{ID: "id1"}}},
		},
		&M.Pool{PoolAllOf0: M.PoolAllOf0{Meta: &M.ObjMeta{ID: "id1"}}},
		&M.ProtectionDomain{ProtectionDomainAllOf0: M.ProtectionDomainAllOf0{Meta: &M.ObjMeta{ID: "id1"}}},
		&M.ServicePlanAllocation{ServicePlanAllocationAllOf0: M.ServicePlanAllocationAllOf0{Meta: &M.ObjMeta{ID: "id1"}}},
		&M.ServicePlan{ServicePlanAllOf0: M.ServicePlanAllOf0{Meta: &M.ObjMeta{ID: "id1"}}},
		&M.Snapshot{SnapshotAllOf0: M.SnapshotAllOf0{Meta: &M.ObjMeta{ID: "id1"}}},
		&M.Storage{StorageAllOf0: M.StorageAllOf0{Meta: &M.ObjMeta{ID: "id1"}}},
		&M.StorageRequest{StorageRequestAllOf0: M.StorageRequestAllOf0{Meta: &M.ObjMeta{ID: "id1"}}},
		&M.User{UserAllOf0: M.UserAllOf0{Meta: &M.ObjMeta{ID: "id1"}}},
		&M.VolumeSeries{VolumeSeriesAllOf0: M.VolumeSeriesAllOf0{Meta: &M.ObjMeta{ID: "id1"}}},
		&M.VolumeSeriesRequest{
			VolumeSeriesRequestAllOf0: M.VolumeSeriesRequestAllOf0{Meta: &M.ObjMeta{ID: "id1"}},
			VolumeSeriesRequestCreateOnce: M.VolumeSeriesRequestCreateOnce{
				VolumeSeriesCreateSpec: &M.VolumeSeriesCreateArgs{},
			},
		},
	}
	for _, tc := range tcs {
		ev.AccessControlScope = tc
		ai := &appAuth.Info{}
		assert.True(hc.EventAllowed(ai, ev))
		ai.RoleObj = &M.Role{}
		assert.False(hc.EventAllowed(ai, ev))
	}

	// objects without filters are always allowed
	tcs = []interface{}{
		&M.System{},
	}
	for _, tc := range tcs {
		ev.AccessControlScope = tc
		ai := &appAuth.Info{RoleObj: &M.Role{}}
		assert.True(hc.EventAllowed(ai, ev))
	}

	// objects that should not generate events
	tcs = []interface{}{
		&M.CSPStorageType{}, &M.Node{}, &M.Role{}, &M.SLO{}, &M.StorageFormula{},
	}
	for _, tc := range tcs {
		ev.AccessControlScope = tc
		assert.False(hc.EventAllowed(&appAuth.Info{}, ev))
		assert.Equal(1, tl.CountPattern("unsupported AccessControlScope"))
		tl.Flush()
	}

	// nil auth treated like internal auth
	ev.AccessControlScope = &M.User{}
	assert.True(hc.EventAllowed(nil, ev))
	ev.AccessControlScope = &M.Role{}
	assert.False(hc.EventAllowed(nil, ev))
	tl.Flush()

	// nil scope with POST metrics and audit-log-records URI is ignored
	assert.False(hc.EventAllowed(&appAuth.Info{}, &crude.CrudEvent{Method: "POST", TrimmedURI: "/metrics/storage"}))
	assert.False(hc.EventAllowed(&appAuth.Info{}, &crude.CrudEvent{Method: "POST", TrimmedURI: "/audit-log-records"}))
	assert.Zero(tl.CountPattern("unsupported AccessControlScope"))

	// task events allowed
	assert.True(hc.EventAllowed(nil, &crude.CrudEvent{TrimmedURI: "/tasks"}))
	// debug events allowed
	assert.True(hc.EventAllowed(nil, &crude.CrudEvent{TrimmedURI: "/debug"}))

	// invalid invocations
	assert.False(hc.EventAllowed(&auth.Info{}, nil))
	assert.False(hc.EventAllowed(nil, nil))
	assert.Equal(1, tl.CountPattern("wrong Subject.*auth.Info"))
	assert.False(hc.EventAllowed(&appAuth.Info{}, nil))
	assert.False(hc.EventAllowed(&appAuth.Info{}, &crude.CrudEvent{}))
	assert.Equal(1, tl.CountPattern("unsupported AccessControlScope.*nil"))
	tl.Flush()
	assert.False(hc.EventAllowed(&appAuth.Info{}, &crude.CrudEvent{Method: "GET", TrimmedURI: "/metrics/storage"}))
	assert.Equal(1, tl.CountPattern("unsupported AccessControlScope"))
}

func TestTaskAuth(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()

	subject := &appAuth.Info{}

	// success, internal
	assert.True(subject.Internal())
	res := hc.TaskViewOK(subject, "OP")
	assert.True(res)

	// success, system management cap
	subject.RoleObj = &M.Role{
		RoleMutable: M.RoleMutable{
			Capabilities: map[string]bool{
				centrald.SystemManagementCap: true,
			},
		},
	}
	assert.False(subject.Internal())
	assert.NoError(subject.CapOK(centrald.SystemManagementCap))
	res = hc.TaskViewOK(subject, "OP")
	assert.True(res)

	// failure, wrong cap
	subject.RoleObj = &M.Role{}
	assert.False(subject.Internal())
	assert.Error(subject.CapOK(centrald.SystemManagementCap))
	res = hc.TaskCreateOK(subject, "OP")
	assert.False(res)

	// failure, invalid subject
	s := &fa.Subject{}
	res = hc.TaskCancelOK(s, "OP")
	assert.False(res)
}

type fakeOps struct {
	InAccountRefsPDaObj *M.Account
	InAccountRefsPDpdID string
	RetAccountRefsPD    bool

	AccountSecretDriverCB     func(accountSecretDriverOps)
	CntAccountSecretDriver    int
	InAccountSecretDriverAi   *appAuth.Info
	InAccountSecretDriverCtx  context.Context
	InAccountSecretDriverID   string
	RetAccountSecretDriverErr error

	InConstrainBothQueryAccountsAi    *appAuth.Info
	InConstrainBothQueryAccountsAID   *string
	InConstrainBothQueryAccountsACap  string
	InConstrainBothQueryAccountsTID   *string
	InConstrainBothQueryAccountsTCap  string
	CntConstrainBothQueryAccounts     int
	RetConstrainBothQueryAccountsAID  *string
	RetConstrainBothQueryAccountsTID  *string
	RetConstrainBothQueryAccountsSkip bool

	InConstrainEOQueryAccountsAi   *appAuth.Info
	InConstrainEOQueryAccountsAID  *string
	InConstrainEOQueryAccountsACap string
	InConstrainEOQueryAccountsTID  *string
	InConstrainEOQueryAccountsTCap string
	CntConstrainEOQueryAccounts    int
	RetConstrainEOQueryAccountsAID *string
	RetConstrainEOQueryAccountsTID *string
	RetConstrainEOQueryAccountsErr error

	CntAccountFetch       int
	InAccountFetchAi      *appAuth.Info
	InAccountFetchID      string
	RetAccountFetchErr    error
	RetAccountFetchObj    *M.Account
	InAccountFetchAiMulti []*appAuth.Info
	InAccountFetchMulti   []string
	RetAccountFetchMulti  []*M.Account

	CntAccountSecretRetrieve     int
	InAccountSecretRetrieveAi    *appAuth.Info
	InAccountSecretRetrieveCtx   context.Context
	InAccountSecretRetrieveAID   string
	InAccountSecretRetrieveClID  string
	InAccountSecretRetrieveClObj *M.Cluster
	RetAccountSecretRetrieveVT   *M.ValueType
	RetAccountSecretRetrieveErr  error

	CntAccountUpdate      int
	InAccountUpdateAi     *appAuth.Info
	InAccountUpdateObj    *M.Account
	InAccountUpdateParams *account.AccountUpdateParams
	RetAccountUpdateErr   error
	RetAccountUpdateObj   *M.Account

	CntClusterFetch    int
	InClusterFetchAi   *appAuth.Info
	InClusterFetchID   string
	RetClusterFetchErr error
	RetClusterFetchObj *M.Cluster

	CntClusterUpdate      int
	InClusterUpdateAi     *appAuth.Info
	InClusterUpdateObj    *M.Cluster
	InClusterUpdateParams *cluster.ClusterUpdateParams
	RetClusterUpdateErr   error
	RetClusterUpdateObj   *M.Cluster

	CntConsistencyGroupFetch     int
	InConsistencyGroupFetchAi    *appAuth.Info
	InConsistencyGroupFetchID    string
	InConsistencyGroupAccountObj *M.Account
	RetConsistencyGroupFetchErr  error
	RetConsistencyGroupFetchObj  *M.ConsistencyGroup

	CntCspDomainFetch       int
	InCspDomainFetchAi      *appAuth.Info
	InCspDomainFetchID      string
	RetCspDomainFetchErr    error
	RetCspDomainFetchObj    *M.CSPDomain
	InCspDomainFetchAiMulti []*appAuth.Info
	InCspDomainFetchMulti   []string
	RetCspDomainFetchMulti  []*M.CSPDomain

	CntCspCredentialFetch    int
	InCspCredentialFetchAi   *appAuth.Info
	InCspCredentialFetchID   string
	RetCspCredentialFetchErr error
	RetCspCredentialFetchObj *M.CSPCredential

	CntProtectionDomainFetch       int
	InProtectionDomainFetchAi      *appAuth.Info
	InProtectionDomainFetchID      string
	RetProtectionDomainFetchObj    *M.ProtectionDomain
	RetProtectionDomainFetchErr    error
	InProtectionDomainFetchAiMulti []*appAuth.Info
	InProtectionDomainFetchMulti   []string
	RetProtectionDomainFetchMulti  []*M.ProtectionDomain

	CntServicePlanAllocationFetch    int
	InServicePlanAllocationFetchAi   *appAuth.Info
	InServicePlanAllocationFetchID   string
	RetServicePlanAllocationFetchErr error
	RetServicePlanAllocationFetchObj *M.ServicePlanAllocation

	CntServicePlanFetch    int
	InServicePlanFetchAi   *appAuth.Info
	InServicePlanFetchID   string
	RetServicePlanFetchErr error
	RetServicePlanFetchObj *M.ServicePlan

	CntSnapshotFetch    int
	InSnapshotFetchAi   *appAuth.Info
	InSnapshotFetchID   string
	RetSnapshotFetchErr error
	RetSnapshotFetchObj *M.Snapshot

	CntSnapshotFetchData    int
	InSnapshotFetchDataAi   *appAuth.Info
	InSnapshotFetchDataID   string
	RetSnapshotFetchDataErr error
	RetSnapshotFetchDataObj *M.SnapshotData

	CntSystemHostnameFetch    int
	InSystemHostnameFetchCTX  context.Context
	RetSystemHostnameFetchHn  string
	RetSystemHostnameFetchErr error

	CntSystemFetch    int
	RetSystemFetchObj *M.System
	RetSystemFetchErr error
}

var _ = internalOps(&fakeOps{})

func (op *fakeOps) accountReferencesProtectionDomain(aObj *M.Account, pdID string) bool {
	op.InAccountRefsPDaObj = aObj
	op.InAccountRefsPDpdID = pdID
	return op.RetAccountRefsPD
}

func (op *fakeOps) accountSecretDriver(ctx context.Context, ai *appAuth.Info, id string, ops accountSecretDriverOps) error {
	op.InAccountSecretDriverCtx = ctx
	op.InAccountSecretDriverAi = ai
	op.InAccountSecretDriverID = id
	op.CntAccountSecretDriver++
	if op.AccountSecretDriverCB != nil {
		op.AccountSecretDriverCB(ops)
	}
	return op.RetAccountSecretDriverErr
}

func (op *fakeOps) constrainBothQueryAccounts(ai *appAuth.Info, accountID *string, accountCap string, tenantAccountID *string, tenantAccountCap string) (*string, *string, bool) {
	op.InConstrainBothQueryAccountsAi = ai
	op.InConstrainBothQueryAccountsAID = accountID
	op.InConstrainBothQueryAccountsACap = accountCap
	op.InConstrainBothQueryAccountsTID = tenantAccountID
	op.InConstrainBothQueryAccountsTCap = tenantAccountCap
	op.CntConstrainBothQueryAccounts++
	return op.RetConstrainBothQueryAccountsAID, op.RetConstrainBothQueryAccountsTID, op.RetConstrainBothQueryAccountsSkip
}

func (op *fakeOps) constrainEitherOrQueryAccounts(ctx context.Context, ai *appAuth.Info, accountID *string, accountCap string, tenantAccountID *string, tenantAccountCap string) (*string, *string, error) {
	op.InConstrainEOQueryAccountsAi = ai
	op.InConstrainEOQueryAccountsAID = accountID
	op.InConstrainEOQueryAccountsACap = accountCap
	op.InConstrainEOQueryAccountsTID = tenantAccountID
	op.InConstrainEOQueryAccountsTCap = tenantAccountCap
	op.CntConstrainEOQueryAccounts++
	return op.RetConstrainEOQueryAccountsAID, op.RetConstrainEOQueryAccountsTID, op.RetConstrainEOQueryAccountsErr
}

func (op *fakeOps) intAccountFetch(ctx context.Context, ai *appAuth.Info, id string) (*M.Account, error) {
	op.InAccountFetchAi = ai
	op.InAccountFetchID = id
	op.CntAccountFetch++
	if op.InAccountFetchMulti != nil {
		op.InAccountFetchMulti = append(op.InAccountFetchMulti, id)
	}
	if op.InAccountFetchAiMulti != nil {
		op.InAccountFetchAiMulti = append(op.InAccountFetchAiMulti, ai)
	}
	if op.RetAccountFetchErr == nil && len(op.RetAccountFetchMulti) > 0 {
		op.RetAccountFetchObj, op.RetAccountFetchMulti = op.RetAccountFetchMulti[0], op.RetAccountFetchMulti[1:]
		if op.RetAccountFetchObj == nil {
			op.RetAccountFetchErr = fmt.Errorf("fake-account-missing")
		}
	}
	return op.RetAccountFetchObj, op.RetAccountFetchErr
}

func (op *fakeOps) intAccountSecretRetrieve(ctx context.Context, ai *appAuth.Info, aID string, clID string, clObj *M.Cluster) (*M.ValueType, error) {
	op.InAccountSecretRetrieveCtx = ctx
	op.InAccountSecretRetrieveAi = ai
	op.InAccountSecretRetrieveAID = aID
	op.InAccountSecretRetrieveClID = clID
	op.InAccountSecretRetrieveClObj = clObj
	op.CntAccountSecretRetrieve++
	return op.RetAccountSecretRetrieveVT, op.RetAccountSecretRetrieveErr
}

func (op *fakeOps) intAccountUpdate(params account.AccountUpdateParams, ai *appAuth.Info, oObj *M.Account) (*M.Account, error) {
	op.InAccountUpdateParams = &params
	op.InAccountUpdateAi = ai
	op.InAccountUpdateObj = oObj
	op.CntAccountUpdate++
	return op.RetAccountUpdateObj, op.RetAccountUpdateErr
}

func (op *fakeOps) intClusterFetch(ctx context.Context, ai *appAuth.Info, id string) (*M.Cluster, error) {
	op.InClusterFetchAi = ai
	op.InClusterFetchID = id
	op.CntClusterFetch++
	return op.RetClusterFetchObj, op.RetClusterFetchErr
}

func (op *fakeOps) intClusterUpdate(params cluster.ClusterUpdateParams, ai *appAuth.Info, oObj *M.Cluster) (*M.Cluster, error) {
	op.InClusterUpdateParams = &params
	op.InClusterUpdateAi = ai
	op.InClusterUpdateObj = oObj
	op.CntClusterUpdate++
	return op.RetClusterUpdateObj, op.RetClusterUpdateErr
}

func (op *fakeOps) intCspDomainFetch(ctx context.Context, ai *appAuth.Info, id string) (*M.CSPDomain, error) {
	op.InCspDomainFetchAi = ai
	op.InCspDomainFetchID = id
	op.CntCspDomainFetch++
	if op.InCspDomainFetchMulti != nil {
		op.InCspDomainFetchMulti = append(op.InCspDomainFetchMulti, id)
	}
	if op.InCspDomainFetchAiMulti != nil {
		op.InCspDomainFetchAiMulti = append(op.InCspDomainFetchAiMulti, ai)
	}
	if op.RetCspDomainFetchErr == nil && len(op.RetCspDomainFetchMulti) > 0 {
		op.RetCspDomainFetchObj, op.RetCspDomainFetchMulti = op.RetCspDomainFetchMulti[0], op.RetCspDomainFetchMulti[1:]
		if op.RetCspDomainFetchObj == nil {
			op.RetCspDomainFetchErr = fmt.Errorf("fake-dom-missing")
		}
	}
	return op.RetCspDomainFetchObj, op.RetCspDomainFetchErr
}

func (op *fakeOps) intConsistencyGroupFetch(ctx context.Context, ai *appAuth.Info, id string, aObj *M.Account) (*M.ConsistencyGroup, error) {
	op.InConsistencyGroupFetchAi = ai
	op.InConsistencyGroupFetchID = id
	op.InConsistencyGroupAccountObj = aObj
	op.CntConsistencyGroupFetch++
	return op.RetConsistencyGroupFetchObj, op.RetConsistencyGroupFetchErr
}

func (op *fakeOps) intCspCredentialFetch(ctx context.Context, ai *appAuth.Info, id string) (*M.CSPCredential, error) {
	op.InCspCredentialFetchAi = ai
	op.InCspCredentialFetchID = id
	op.CntCspCredentialFetch++
	return op.RetCspCredentialFetchObj, op.RetCspCredentialFetchErr
}

func (op *fakeOps) intProtectionDomainFetch(ctx context.Context, ai *appAuth.Info, id string) (*M.ProtectionDomain, error) {
	op.InProtectionDomainFetchAi = ai
	op.InProtectionDomainFetchID = id
	op.CntProtectionDomainFetch++
	if op.InProtectionDomainFetchMulti != nil {
		op.InProtectionDomainFetchMulti = append(op.InProtectionDomainFetchMulti, id)
	}
	if op.InProtectionDomainFetchAiMulti != nil {
		op.InProtectionDomainFetchAiMulti = append(op.InProtectionDomainFetchAiMulti, ai)
	}
	if op.RetProtectionDomainFetchErr == nil && len(op.RetProtectionDomainFetchMulti) > 0 {
		op.RetProtectionDomainFetchObj, op.RetProtectionDomainFetchMulti = op.RetProtectionDomainFetchMulti[0], op.RetProtectionDomainFetchMulti[1:]
		if op.RetProtectionDomainFetchObj == nil {
			op.RetProtectionDomainFetchErr = fmt.Errorf("fake-pd-missing")
		}
	}
	return op.RetProtectionDomainFetchObj, op.RetProtectionDomainFetchErr
}

func (op *fakeOps) intServicePlanAllocationFetch(ctx context.Context, ai *appAuth.Info, id string) (*M.ServicePlanAllocation, error) {
	op.InServicePlanAllocationFetchAi = ai
	op.InServicePlanAllocationFetchID = id
	op.CntServicePlanAllocationFetch++
	return op.RetServicePlanAllocationFetchObj, op.RetServicePlanAllocationFetchErr
}

func (op *fakeOps) intServicePlanFetch(ctx context.Context, ai *appAuth.Info, id string) (*M.ServicePlan, error) {
	op.InServicePlanFetchAi = ai
	op.InServicePlanFetchID = id
	op.CntServicePlanFetch++
	return op.RetServicePlanFetchObj, op.RetServicePlanFetchErr
}

func (op *fakeOps) intSnapshotFetch(ctx context.Context, ai *appAuth.Info, id string) (*M.Snapshot, error) {
	op.InSnapshotFetchAi = ai
	op.InSnapshotFetchID = id
	op.CntSnapshotFetch++
	return op.RetSnapshotFetchObj, op.RetSnapshotFetchErr
}

func (op *fakeOps) intSnapshotFetchData(ctx context.Context, ai *appAuth.Info, id string) (*M.SnapshotData, error) {
	op.InSnapshotFetchDataAi = ai
	op.InSnapshotFetchDataID = id
	op.CntSnapshotFetchData++
	return op.RetSnapshotFetchDataObj, op.RetSnapshotFetchDataErr
}

func (op *fakeOps) intSystemHostnameFetch(ctx context.Context) (string, error) {
	op.InSystemHostnameFetchCTX = ctx
	op.CntSystemHostnameFetch++
	return op.RetSystemHostnameFetchHn, op.RetSystemHostnameFetchErr
}

func (op *fakeOps) intSystemFetch() (*M.System, error) {
	op.CntSystemFetch++
	return op.RetSystemFetchObj, op.RetSystemFetchErr
}
