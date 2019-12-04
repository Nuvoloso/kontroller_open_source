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


package main

import (
	"fmt"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/application_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/cluster"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/consistency_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_credential"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/node"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/pool"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/protection_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// most paths are tested many times in the subcommand UTs, just test remaining error paths here
func TestCacheErrorPaths(t *testing.T) {
	assert := assert.New(t)
	ch := &cacheHelper{}

	// validateAccount, tenant account succeeds but subtenant fails
	ch.accounts = map[string]ctxIDName{
		"id1": {"", "nuvoloso"},
		"id2": {"id1", "titus"},
	}
	id, err := ch.validateAccount("nuvoloso/tad", "authorized")
	assert.Empty(id)
	if assert.Error(err) {
		assert.Equal("authorized account 'nuvoloso/tad' not found", err.Error())
	}

	// again, no usage specified
	id, err = ch.validateAccount("nuvoloso/tad", "")
	assert.Empty(id)
	if assert.Error(err) {
		assert.Equal("account 'nuvoloso/tad' not found", err.Error())
	}

	// validateAccount, tenant account lookup fails
	id, err = ch.validateAccount("nova/tad", "")
	assert.Empty(id)
	if assert.Error(err) {
		assert.Equal("account 'nova/tad' not found", err.Error())
	}

	// subordinate fails with appCtx
	appCtx.Account, appCtx.AccountID = "nova", "tid1"
	id, err = ch.validateAccount("nuvoloso", "")
	assert.Empty(id)
	if assert.Error(err) {
		assert.Equal("account 'nuvoloso' not found", err.Error())
	}
	ch.accounts = nil
	appCtx.Account, appCtx.AccountID = "", ""

	// validateDomainClusterNodeNames ignores lookup error if nothing to validate
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	cspDomainListErr := fmt.Errorf("cspDomainListError")
	dm := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	dOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	dOps.EXPECT().CspDomainList(dm).Return(nil, cspDomainListErr)
	dom, cluster, node, err := ch.validateDomainClusterNodeNames("", "", "")
	assert.Zero(dom)
	assert.Zero(cluster)
	assert.Zero(node)
	assert.NoError(err)

	// validateApplicationConsistencyGroupNames performs no lookups if nothing to validate
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	ag, cg, err := ch.validateApplicationConsistencyGroupNames([]string{}, "")
	assert.Empty(ag)
	assert.Zero(cg)
	assert.NoError(err)
}

func TestVolumeSeriesCache(t *testing.T) {
	assert := assert.New(t)
	ch := &cacheHelper{}

	var resVS = &volume_series.VolumeSeriesListOK{
		Payload: []*models.VolumeSeries{
			&models.VolumeSeries{
				VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
					Meta: &models.ObjMeta{
						ID:      "vs1",
						Version: 1,
					},
					RootParcelUUID: "uuid1",
				},
				VolumeSeriesMutable: models.VolumeSeriesMutable{
					VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
						Name: "MyVolSeries",
					},
				},
			},
			&models.VolumeSeries{
				VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
					Meta: &models.ObjMeta{
						ID:      "vs2",
						Version: 1,
					},
					RootParcelUUID: "uuid2",
				},
				VolumeSeriesMutable: models.VolumeSeriesMutable{
					VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
						Name: "OtherVolSeries",
					},
				},
			},
		},
	}

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()

	// list all existing volume series and build cache
	assert.Equal(len(ch.volumeSeries), 0)
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	vsm := mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesListParams())
	vsOps := mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vsOps)
	vsOps.EXPECT().VolumeSeriesList(vsm).Return(resVS, nil).MinTimes(1)
	err := ch.cacheVolumeSeries("", "", "")
	assert.Nil(err)
	// VS cache contains all existing VSs now
	assert.Equal(len(ch.volumeSeries), len(resVS.Payload))

	// failure to list VS
	expectedErr := fmt.Errorf("SOME ERROR")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	vsm = mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesListParams())
	vsOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vsOps)
	vsOps.EXPECT().VolumeSeriesList(vsm).Return(nil, expectedErr)
	err = ch.cacheVolumeSeries("", "", "")
	assert.NotNil(err)
	assert.Equal(expectedErr, err)

	// failure to fetch VS, api error
	ch.volumeSeries = nil
	apiErr := &volume_series.VolumeSeriesFetchDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	vsm = mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesFetchParams().WithID("vs1"))
	vsOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vsOps)
	vsOps.EXPECT().VolumeSeriesFetch(vsm).Return(nil, apiErr)
	err = ch.cacheVolumeSeries("vs1", "", "")
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// failure to fetch VS, arbitrary error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	vsm = mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesFetchParams().WithID("vs1"))
	vsOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vsOps)
	vsOps.EXPECT().VolumeSeriesFetch(vsm).Return(nil, expectedErr)
	err = ch.cacheVolumeSeries("vs1", "", "")
	assert.NotNil(err)
	assert.Equal(expectedErr, err)
}

func TestCspCredentialsCache(t *testing.T) {
	assert := assert.New(t)
	ch := &cacheHelper{}

	credListRes := &csp_credential.CspCredentialListOK{
		Payload: []*models.CSPCredential{
			&models.CSPCredential{
				CSPCredentialAllOf0: models.CSPCredentialAllOf0{
					Meta: &models.ObjMeta{
						ID: "objectID1",
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
		},
	}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()

	// list all existing csp credentials and build cache
	assert.Equal(len(ch.cspCredentials), 0)
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	cm := mockmgmtclient.NewCspCredentialMatcher(t, csp_credential.NewCspCredentialListParams())
	cOps := mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps)
	cOps.EXPECT().CspCredentialList(cm).Return(credListRes, nil).MinTimes(1)
	err := ch.cacheCspCredentials("", "")
	assert.Nil(err)
	// CspCredentials cache contains all existing CspCredentials now
	assert.Equal(len(ch.cspCredentials), len(credListRes.Payload))

	// failure to list csp credentials
	expectedErr := fmt.Errorf("SOME ERROR")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	ch.cspCredentials = nil
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	cm = mockmgmtclient.NewCspCredentialMatcher(t, csp_credential.NewCspCredentialListParams())
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps)
	cOps.EXPECT().CspCredentialList(cm).Return(nil, expectedErr).MinTimes(1)
	err = ch.cacheCspCredentials("", "")
	assert.NotNil(err)
	assert.Equal(expectedErr, err)
	assert.Equal(len(ch.cspCredentials), 0)
}

func TestValidateProtectionDomainName(t *testing.T) {
	assert := assert.New(t)

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()

	// cacheProtectionDomains error (via validateProtectionDomainName)
	c := &cacheHelper{}
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	pdm := mockmgmtclient.NewProtectionDomainMatcher(t, protection_domain.NewProtectionDomainListParams())
	pdOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
	mAPI.EXPECT().ProtectionDomain().Return(pdOps).MinTimes(1)
	pdOps.EXPECT().ProtectionDomainList(pdm).Return(nil, fmt.Errorf("pd-error")).MinTimes(1)
	appCtx.API = mAPI
	pdID, err := c.validateProtectionDomainName("PD1")
	assert.Error(err)
	assert.Empty(pdID)
	assert.Empty(c.protectionDomains)
	mockCtrl.Finish()

	// cacheProtectionDomains success (via validateProtectionDomainName)
	c = &cacheHelper{}
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	pdm = mockmgmtclient.NewProtectionDomainMatcher(t, protection_domain.NewProtectionDomainListParams())
	pdOps = mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
	mAPI.EXPECT().ProtectionDomain().Return(pdOps).MinTimes(1)
	pdOps.EXPECT().ProtectionDomainList(pdm).Return(resProtectionDomains, nil).MinTimes(1)
	appCtx.API = mAPI
	pdID, err = c.validateProtectionDomainName("PD1")
	assert.NoError(err)
	assert.NotEmpty(c.protectionDomains)
	assert.Equal("PD-1", pdID)
	pdObj, err := c.lookupProtectionDomainByID(pdID)
	assert.NoError(err)
	assert.NotNil(pdObj)
	assert.Equal(resProtectionDomains.Payload[0], pdObj)

	// fail to find a name in the cache
	assert.NotEmpty(c.protectionDomains)
	pdID, err = c.validateProtectionDomainName("not-in-cache")
	assert.Error(err)
	assert.Empty(pdID)
	pdObj, err = c.lookupProtectionDomainByID(pdID + "foo")
	assert.Error(err)
	assert.Regexp("protection domain with ID.*not found", err)
	assert.Nil(pdObj)
}

var resAccounts = &account.AccountListOK{
	Payload: []*models.Account{
		&models.Account{
			AccountAllOf0: models.AccountAllOf0{
				Meta: &models.ObjMeta{
					ID: "accountID",
				},
			},
			AccountMutable: models.AccountMutable{
				Name: "System",
			},
		},
		&models.Account{
			AccountAllOf0: models.AccountAllOf0{
				Meta: &models.ObjMeta{
					ID: "authAccountID",
				},
			},
			AccountMutable: models.AccountMutable{
				Name: "AuthAccount",
			},
		},
	},
}
var resAGs = &application_group.ApplicationGroupListOK{
	Payload: []*models.ApplicationGroup{
		&models.ApplicationGroup{
			ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
				Meta: &models.ObjMeta{
					ID: "a-g-1",
				},
			},
			ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
				AccountID: "accountID",
			},
			ApplicationGroupMutable: models.ApplicationGroupMutable{
				Name: "AG",
			},
		},
	},
}
var resCspDomains = &csp_domain.CspDomainListOK{
	Payload: []*models.CSPDomain{
		&models.CSPDomain{
			CSPDomainAllOf0: models.CSPDomainAllOf0{
				Meta: &models.ObjMeta{
					ID: "CSP-DOMAIN-1",
				},
			},
			CSPDomainMutable: models.CSPDomainMutable{
				Name: "domainName",
			},
		},
	},
}
var resClusters = &cluster.ClusterListOK{
	Payload: []*models.Cluster{
		&models.Cluster{
			ClusterAllOf0: models.ClusterAllOf0{
				Meta: &models.ObjMeta{
					ID: "clusterID",
				},
			},
			ClusterCreateOnce: models.ClusterCreateOnce{
				CspDomainID: models.ObjIDMutable("CSP-DOMAIN-1"),
			},
			ClusterMutable: models.ClusterMutable{
				ClusterCreateMutable: models.ClusterCreateMutable{
					Name: "clusterName",
				},
			},
		},
	},
}
var resCGs = &consistency_group.ConsistencyGroupListOK{
	Payload: []*models.ConsistencyGroup{
		&models.ConsistencyGroup{
			ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
				Meta: &models.ObjMeta{
					ID: "c-g-1",
				},
			},
			ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
				AccountID: "accountID",
			},
			ConsistencyGroupMutable: models.ConsistencyGroupMutable{
				Name:                "CG",
				ApplicationGroupIds: []models.ObjIDMutable{"a-g-1"},
			},
		},
	},
}
var resNodes = &node.NodeListOK{
	Payload: []*models.Node{
		&models.Node{
			NodeAllOf0: models.NodeAllOf0{
				Meta: &models.ObjMeta{
					ID: "nodeID-1",
				},
				ClusterID: models.ObjIDMutable("clusterID"),
			},
			NodeMutable: models.NodeMutable{
				Name: "nodeName",
			},
		},
	},
}
var resPlans = &service_plan.ServicePlanListOK{
	Payload: []*models.ServicePlan{
		&models.ServicePlan{
			ServicePlanAllOf0: models.ServicePlanAllOf0{
				Meta: &models.ObjMeta{
					ID: "plan-1",
				},
			},
			ServicePlanMutable: models.ServicePlanMutable{
				Name: "plan B",
			},
		},
	},
}
var resPools = &pool.PoolListOK{
	Payload: []*models.Pool{
		&models.Pool{
			PoolAllOf0: models.PoolAllOf0{
				Meta: &models.ObjMeta{
					ID: "poolID",
				},
			},
			PoolCreateOnce: models.PoolCreateOnce{
				CspDomainID: "CSP-DOMAIN-1",
			},
			PoolMutable: models.PoolMutable{
				PoolCreateMutable: models.PoolCreateMutable{},
			},
		},
	},
}
var resProtectionDomains = &protection_domain.ProtectionDomainListOK{
	Payload: []*models.ProtectionDomain{
		&models.ProtectionDomain{
			ProtectionDomainAllOf0: models.ProtectionDomainAllOf0{
				Meta: &models.ObjMeta{
					ID: "PD-1",
				},
			},
			ProtectionDomainCreateOnce: models.ProtectionDomainCreateOnce{
				AccountID:            "accountID",
				EncryptionAlgorithm:  "AES-256",
				EncryptionPassphrase: &models.ValueType{Kind: "SECRET", Value: "PD1 password"},
			},
			ProtectionDomainMutable: models.ProtectionDomainMutable{
				Name:        "PD1",
				Description: "PD key",
				SystemTags:  models.ObjTags{"stag1", "stag2"},
				Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
			},
		},
	},
}
var resListVS = &volume_series.VolumeSeriesListOK{
	Payload: []*models.VolumeSeries{
		&models.VolumeSeries{
			VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
				Meta: &models.ObjMeta{
					ID:      "vs1",
					Version: 1,
				},
				RootParcelUUID: "uuid1",
			},
			VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
				AccountID: "accountID",
			},
			VolumeSeriesMutable: models.VolumeSeriesMutable{
				VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
					Name: "MyVolSeries",
				},
			},
		},
	},
}
var resFetchVS = &volume_series.VolumeSeriesFetchOK{
	Payload: &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID:      "vs1",
				Version: 1,
			},
			RootParcelUUID: "uuid1",
		},
		VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
			AccountID: "accountID",
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				Name: "MyVolSeries",
			},
		},
	},
}

// loadCache tells cacheLoadHelper how much of the cache to mock
type loadCache int

// cacheAmount values
const (
	loadContext loadCache = iota
	loadDomains
	loadClusters
	loadNodes
	loadPools
	loadAccounts
	loadCGs
	loadAGs
	loadPlans
	loadVSsWithList
	loadVSsWithFetch
	loadProtectionDomains
)

// cacheLoadHelper supplies all of the mock expectations to load the caches in the normal case
func cacheLoadHelper(t *testing.T, mockCtrl *gomock.Controller, cacheIt ...loadCache) *mockmgmtclient.MockAPI {
	if len(cacheIt) == 0 {
		cacheIt = []loadCache{loadContext, loadDomains, loadClusters, loadNodes, loadPools, loadAccounts, loadCGs, loadAGs, loadPlans}
	}
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	if util.Contains(cacheIt, loadPlans) {
		ppm := mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
		pOps := mockmgmtclient.NewMockServicePlanClient(mockCtrl)
		mAPI.EXPECT().ServicePlan().Return(pOps)
		pOps.EXPECT().ServicePlanList(ppm).Return(resPlans, nil)
	}
	if util.Contains(cacheIt, loadAGs) {
		agm := mockmgmtclient.NewApplicationGroupMatcher(t, application_group.NewApplicationGroupListParams())
		agOps := mockmgmtclient.NewMockApplicationGroupClient(mockCtrl)
		mAPI.EXPECT().ApplicationGroup().Return(agOps).MinTimes(1)
		agOps.EXPECT().ApplicationGroupList(agm).Return(resAGs, nil).MinTimes(1)
	}
	if util.Contains(cacheIt, loadCGs) {
		cgm := mockmgmtclient.NewConsistencyGroupMatcher(t, consistency_group.NewConsistencyGroupListParams())
		cgOps := mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
		mAPI.EXPECT().ConsistencyGroup().Return(cgOps).MinTimes(1)
		cgOps.EXPECT().ConsistencyGroupList(cgm).Return(resCGs, nil).MinTimes(1)
	}
	if util.Contains(cacheIt, loadAccounts) {
		apm := mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
		aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
		mAPI.EXPECT().Account().Return(aOps).MinTimes(1)
		aOps.EXPECT().AccountList(apm).Return(resAccounts, nil).MinTimes(1)
	}
	if util.Contains(cacheIt, loadNodes) {
		nm := mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
		nOps := mockmgmtclient.NewMockNodeClient(mockCtrl)
		mAPI.EXPECT().Node().Return(nOps).MinTimes(1)
		nOps.EXPECT().NodeList(nm).Return(resNodes, nil).MinTimes(1)
	}
	if util.Contains(cacheIt, loadClusters) {
		cm := mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
		clOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
		mAPI.EXPECT().Cluster().Return(clOps).MinTimes(1)
		clOps.EXPECT().ClusterList(cm).Return(resClusters, nil).MinTimes(1)
	}
	if util.Contains(cacheIt, loadDomains) {
		dm := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
		dOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
		mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
		dOps.EXPECT().CspDomainList(dm).Return(resCspDomains, nil).MinTimes(1)
	}
	if util.Contains(cacheIt, loadContext) {
		authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
		mAPI.EXPECT().Authentication().Return(authOps)
		authOps.EXPECT().SetContextAccount("", "System").Return("accountID", nil)
	}
	if util.Contains(cacheIt, loadVSsWithList) {
		mvs := mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesListParams())
		mvs.ListParam.Name = swag.String(string(resListVS.Payload[0].Name))
		mvs.ListParam.AccountID = swag.String(string(resAccounts.Payload[0].Meta.ID))
		vsOps := mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
		mAPI.EXPECT().VolumeSeries().Return(vsOps).MinTimes(1)
		vsOps.EXPECT().VolumeSeriesList(mvs).Return(resListVS, nil).MinTimes(1)
	}
	if util.Contains(cacheIt, loadVSsWithFetch) {
		mvs := mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesFetchParams().WithID("vs1"))
		vsOps := mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
		mAPI.EXPECT().VolumeSeries().Return(vsOps).MinTimes(1)
		vsOps.EXPECT().VolumeSeriesFetch(mvs).Return(resFetchVS, nil).MinTimes(1)
	}
	if util.Contains(cacheIt, loadProtectionDomains) {
		pdm := mockmgmtclient.NewProtectionDomainMatcher(t, protection_domain.NewProtectionDomainListParams())
		pdOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
		mAPI.EXPECT().ProtectionDomain().Return(pdOps).MinTimes(1)
		pdOps.EXPECT().ProtectionDomainList(pdm).Return(resProtectionDomains, nil).MinTimes(1)
	}
	return mAPI
}
