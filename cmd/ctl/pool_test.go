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
	"os"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/cluster"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/pool"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestPoolMakeRecord(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	cmd := &poolCmd{}
	cmd.accounts = map[string]ctxIDName{
		"authAccountID": {"tenantID", "sub"},
		"tenantID":      {"", "tenant"},
	}
	cmd.cspDomains = map[string]string{
		"87956a70-4108-4a9a-8ca9-e391505db88c": "MyAWS",
	}
	o := &models.Pool{
		PoolAllOf0: models.PoolAllOf0{
			Meta: &models.ObjMeta{
				ID:           "id",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
		},
		PoolCreateOnce: models.PoolCreateOnce{
			AccountID:           "tenantID",
			AuthorizedAccountID: "authAccountID",
			CspDomainID:         "87956a70-4108-4a9a-8ca9-e391505db88c",
			CspStorageType:      "Amazon gp2",
			StorageAccessibility: &models.StorageAccessibilityMutable{
				AccessibilityScope:      "CSPDOMAIN",
				AccessibilityScopeObjID: "87956a70-4108-4a9a-8ca9-e391505db88c",
			},
		},
		PoolMutable: models.PoolMutable{
			PoolMutableAllOf0: models.PoolMutableAllOf0{
				ServicePlanReservations: map[string]models.StorageTypeReservation{},
			},
			PoolCreateMutable: models.PoolCreateMutable{
				SystemTags: []string{"tag1", "tag2"},
			},
		},
	}
	rec := cmd.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(poolHeaders))
	assert.Equal("id", rec[hID])
	assert.Equal("MyAWS", rec[hCspDomain])
	assert.Equal("Amazon gp2", rec[hCSPStorageType])
	assert.Equal("CSPDOMAIN", rec[hAccessibilityScope])
	assert.Equal("87956a70-4108-4a9a-8ca9-e391505db88c", rec[hAccessibilityScopeID])
	assert.Equal("tenant", rec[hAccount])
	assert.Equal("sub", rec[hAuthorizedAccount])

	// cacheCSPDomains is a singleton
	err := cmd.cacheCSPDomains()
	assert.Nil(err)

	// repeat without the maps
	cmd.accounts = map[string]ctxIDName{}
	cmd.cspDomains = map[string]string{}
	rec = cmd.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(poolHeaders))
	assert.Equal("87956a70-4108-4a9a-8ca9-e391505db88c", rec[hCspDomain])
	assert.Equal("tenantID", rec[hAccount])
	assert.Equal("authAccountID", rec[hAuthorizedAccount])
}

func TestPoolList(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	resCspDomains := &csp_domain.CspDomainListOK{
		Payload: []*models.CSPDomain{
			&models.CSPDomain{
				CSPDomainAllOf0: models.CSPDomainAllOf0{
					Meta: &models.ObjMeta{
						ID: "87956a70-4108-4a9a-8ca9-e391505db88c",
					},
					CspDomainType: models.CspDomainTypeMutable("AWS"),
				},
				CSPDomainMutable: models.CSPDomainMutable{
					Name: "domainName",
				},
			},
		},
	}
	res := &pool.PoolListOK{
		Payload: []*models.Pool{
			&models.Pool{
				PoolAllOf0: models.PoolAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				PoolCreateOnce: models.PoolCreateOnce{
					AuthorizedAccountID: "SystemAccountID",
					ClusterID:           "CLUSTER-1",
					CspDomainID:         "87956a70-4108-4a9a-8ca9-e391505db88c",
					CspStorageType:      "Amazon gp2",
					StorageAccessibility: &models.StorageAccessibilityMutable{
						AccessibilityScope:      "CSPDOMAIN",
						AccessibilityScopeObjID: "87956a70-4108-4a9a-8ca9-e391505db88c",
					},
				},
				PoolMutable: models.PoolMutable{
					PoolMutableAllOf0: models.PoolMutableAllOf0{
						ServicePlanReservations: map[string]models.StorageTypeReservation{},
					},
					PoolCreateMutable: models.PoolCreateMutable{
						SystemTags: []string{"tag1", "tag2"},
					},
				},
			},
			&models.Pool{
				PoolAllOf0: models.PoolAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				PoolCreateOnce: models.PoolCreateOnce{
					AuthorizedAccountID: "SystemAccountID",
					ClusterID:           "CLUSTER-1",
					CspDomainID:         "87956a70-4108-4a9a-8ca9-e391505db88c",
					CspStorageType:      "Amazon S3",
					StorageAccessibility: &models.StorageAccessibilityMutable{
						AccessibilityScope: "CSPDOMAIN",
					},
				},
			},
		},
	}
	clM := mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clRes := &cluster.ClusterListOK{
		Payload: []*models.Cluster{
			&models.Cluster{
				ClusterAllOf0: models.ClusterAllOf0{
					Meta: &models.ObjMeta{
						ID:           "CLUSTER-1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				ClusterCreateOnce: models.ClusterCreateOnce{
					ClusterType: "kubernetes",
					CspDomainID: models.ObjIDMutable("87956a70-4108-4a9a-8ca9-e391505db88c"),
				},
				ClusterMutable: models.ClusterMutable{
					ClusterMutableAllOf0: models.ClusterMutableAllOf0{
						ClusterVersion: "cluster-version-1",
						Service: &models.NuvoService{
							ServiceState: models.ServiceState{
								State: "RUNNING",
							},
						},
					},
					ClusterCreateMutable: models.ClusterCreateMutable{
						Name:        "cluster1",
						Description: "cluster1 object",
						Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
						ClusterAttributes: map[string]models.ValueType{
							"something": models.ValueType{Kind: "STRING", Value: "good"},
							"smells":    models.ValueType{Kind: "STRING", Value: "bad"},
							"awful":     models.ValueType{Kind: "STRING", Value: "ugly"},
						},
						ClusterIdentifier: "CLUSTER-UUID-1",
					},
				},
			},
		},
	}
	aM := mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	resAccounts := &account.AccountListOK{
		Payload: []*models.Account{
			&models.Account{
				AccountAllOf0: models.AccountAllOf0{
					Meta: &models.ObjMeta{
						ID: "SystemAccountID",
					},
				},
				AccountMutable: models.AccountMutable{
					Name: "System",
				},
			},
		},
	}
	svpM := mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	resSvcP := &service_plan.ServicePlanListOK{
		Payload: []*models.ServicePlan{
			&models.ServicePlan{
				ServicePlanAllOf0: models.ServicePlanAllOf0{
					Meta: &models.ObjMeta{
						ID: "PLAN-1",
					},
				},
				ServicePlanMutable: models.ServicePlanMutable{
					Name: "ServicePlan",
				},
			},
		},
	}

	// list, all defaults
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	dm := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	dOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	dOps.EXPECT().CspDomainList(dm).Return(resCspDomains, nil)
	params := pool.NewPoolListParams()
	m := mockmgmtclient.NewPoolMatcher(t, params)
	cOps := mockmgmtclient.NewMockPoolClient(mockCtrl)
	mAPI.EXPECT().Pool().Return(cOps)
	cOps.EXPECT().PoolList(m).Return(res, nil)
	clOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clM).Return(clRes, nil)
	aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	svcPOps := mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(svcPOps).MinTimes(1)
	svcPOps.EXPECT().ServicePlanList(svpM).Return(resSvcP, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initPool()
	err := parseAndRun([]string{"pool", "list"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(poolDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list, json, most options
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	dm = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	dOps.EXPECT().CspDomainList(dm).Return(resCspDomains, nil)
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clM).Return(clRes, nil)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	params = pool.NewPoolListParams()
	params.CspDomainID = swag.String("87956a70-4108-4a9a-8ca9-e391505db88c")
	params.CspStorageType = swag.String("storageType")
	params.AccessibilityScope = swag.String("CSPDOMAIN")
	params.AccessibilityScopeObjID = swag.String("scopeObjID")
	params.ClusterID = swag.String("CLUSTER-1")
	params.AuthorizedAccountID = swag.String("SystemAccountID")
	m = mockmgmtclient.NewPoolMatcher(t, params)
	cOps = mockmgmtclient.NewMockPoolClient(mockCtrl)
	mAPI.EXPECT().Pool().Return(cOps)
	cOps.EXPECT().PoolList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initPool()
	err = parseAndRun([]string{"pool", "list", "-D", "domainName", "-T", "storageType",
		"--scope", "CSPDOMAIN", "--scope-object-id", "scopeObjID", "-C", "cluster1", "-Z", "System",
		"-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(res.Payload, te.jsonData)

	// list default, yaml
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	dm = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	dOps.EXPECT().CspDomainList(dm).Return(resCspDomains, nil)
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clM).Return(clRes, nil)
	params = pool.NewPoolListParams()
	m = mockmgmtclient.NewPoolMatcher(t, params)
	cOps = mockmgmtclient.NewMockPoolClient(mockCtrl)
	mAPI.EXPECT().Pool().Return(cOps)
	cOps.EXPECT().PoolList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initPool()
	err = parseAndRun([]string{"pool", "list", "-o", "yaml"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(res.Payload, te.yamlData)

	// list follow
	fw := &fakeWatcher{t: t}
	appCtx.watcher = fw
	fw.CallCBCount = 1
	fos := &fakeOSExecutor{}
	appCtx.osExec = fos
	poolListCmdRunCacheThreshold = 0 // force a cache refresh
	defer func() { poolListCmdRunCacheThreshold = 1 }()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dm = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	dOps.EXPECT().CspDomainList(dm).Return(resCspDomains, nil).MinTimes(1)
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clM).Return(clRes, nil)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps).MinTimes(1)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil).MinTimes(1)
	svcPOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(svcPOps).MinTimes(1)
	svcPOps.EXPECT().ServicePlanList(svpM).Return(resSvcP, nil).MinTimes(1)
	params = pool.NewPoolListParams()
	params.CspDomainID = swag.String("87956a70-4108-4a9a-8ca9-e391505db88c")
	params.ClusterID = swag.String("CLUSTER-1")
	params.AuthorizedAccountID = swag.String("SystemAccountID")
	m = mockmgmtclient.NewPoolMatcher(t, params)
	cOps = mockmgmtclient.NewMockPoolClient(mockCtrl)
	mAPI.EXPECT().Pool().Return(cOps)
	cOps.EXPECT().PoolList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initPool()
	err = parseAndRun([]string{"pool", "list", "-f", "-D", "domainName", "-C", "cluster1", "-Z", "System"})
	assert.Nil(err)
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))
	expWArgs := &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{
				URIPattern:   "/pools/?",
				ScopePattern: ".*authorizedAccountId:SystemAccountID.*clusterId:CLUSTER-1.*cspDomainId:87956a70-4108-4a9a-8ca9-e391505db88c",
			},
		},
	}
	assert.NotNil(fw.InArgs)
	assert.Equal(expWArgs, fw.InArgs)
	assert.NotNil(fw.InCB)
	assert.Equal(1, fw.NumCalls)
	assert.NoError(fw.CBErr)

	// list with columns, valid account and owner authorized account
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "System").Return("SystemAccountID", nil)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	dm = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	dOps.EXPECT().CspDomainList(dm).Return(resCspDomains, nil)
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clM).Return(clRes, nil)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	params = pool.NewPoolListParams()
	params.AuthorizedAccountID = swag.String("SystemAccountID")
	m = mockmgmtclient.NewPoolMatcher(t, params)
	cOps = mockmgmtclient.NewMockPoolClient(mockCtrl)
	mAPI.EXPECT().Pool().Return(cOps)
	cOps.EXPECT().PoolList(m).Return(res, nil)
	svcPOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(svcPOps).MinTimes(1)
	svcPOps.EXPECT().ServicePlanList(svpM).Return(resSvcP, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initPool()
	err = parseAndRun([]string{"pools", "list", "-c", "CSPDomain", "-A", "System", "--owner-auth"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, 1)
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))
	params.AuthorizedAccountID = nil
	appCtx.Account, appCtx.AccountID = "", ""

	// list with owner-auth and authorized
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "Nuvoloso").Return("tid1", nil)
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initPool()
	err = parseAndRun([]string{"pool", "list", "-A", "Nuvoloso", "--owner-auth", "-Z", "dave"})
	assert.NotNil(err)
	assert.Regexp("do not specify --authorized-account and --owner-auth together", err)
	appCtx.Account, appCtx.AccountID = "", ""

	// list with owner-auth and no account
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initPool()
	err = parseAndRun([]string{"pool", "list", "--owner-auth"})
	assert.NotNil(err)
	assert.Regexp("--owner-auth requires --account", err)
	appCtx.Account, appCtx.AccountID = "", ""

	// list with invalid columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initPool()
	err = parseAndRun([]string{"pool", "list", "--columns", "Name,Foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// list with invalid names
	for _, tc := range []string{"domain", "cluster", "authAccount"} {
		cmd := []string{"pool", "list"}
		var eMsg string
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
		switch tc {
		case "domain":
			cmd = append(cmd, "-D", "WrongDomain")
			eMsg = "CSPDomain.*not found"
		case "cluster":
			cmd = append(cmd, "-D", "domainName", "-C", "WrongCluster")
			eMsg = "cluster.*WrongCluster.*not found in domain"
		case "authAccount":
			cmd = append(cmd, "-Z", "WrongAuthAccount")
			eMsg = "account.*WrongAuthAccount.*not found"
			aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
			mAPI.EXPECT().Account().Return(aOps)
			aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
		}
		t.Log("list with invalid", tc)
		dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
		mAPI.EXPECT().CspDomain().Return(dOps)
		dm = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
		dOps.EXPECT().CspDomainList(dm).Return(resCspDomains, nil)
		clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
		mAPI.EXPECT().Cluster().Return(clOps)
		clOps.EXPECT().ClusterList(clM).Return(clRes, nil)
		appCtx.API = mAPI
		te = &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initPool()
		err = parseAndRun(cmd)
		assert.Error(err)
		t.Log("error is", err.Error())
		assert.Regexp(eMsg, err.Error())
	}

	// list, API failure model error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	dm = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	dOps.EXPECT().CspDomainList(dm).Return(resCspDomains, nil)
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clM).Return(clRes, nil)
	params = pool.NewPoolListParams()
	m = mockmgmtclient.NewPoolMatcher(t, params)
	cOps = mockmgmtclient.NewMockPoolClient(mockCtrl)
	mAPI.EXPECT().Pool().Return(cOps)
	apiErr := &pool.PoolListDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	cOps.EXPECT().PoolList(m).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initPool()
	err = parseAndRun([]string{"pools", "list"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// list, API failure arbitrary error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	dm = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	dOps.EXPECT().CspDomainList(dm).Return(resCspDomains, nil)
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clM).Return(clRes, nil)
	params = pool.NewPoolListParams()
	m = mockmgmtclient.NewPoolMatcher(t, params)
	cOps = mockmgmtclient.NewMockPoolClient(mockCtrl)
	mAPI.EXPECT().Pool().Return(cOps)
	otherErr := fmt.Errorf("OTHER ERROR")
	cOps.EXPECT().PoolList(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initPool()
	err = parseAndRun([]string{"pool", "list"})
	assert.NotNil(err)
	assert.Equal(otherErr.Error(), err.Error())

	// cspdomain list error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	dm = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	cspDomainListErr := fmt.Errorf("cspDomainListError")
	dOps.EXPECT().CspDomainList(dm).Return(nil, cspDomainListErr)
	params = pool.NewPoolListParams()
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initPool()
	err = parseAndRun([]string{"pool", "list"})
	assert.NotNil(err)
	assert.Equal(cspDomainListErr.Error(), err.Error())

	// cluster without domain
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initPool()
	err = parseAndRun([]string{"pool", "list", "-C", "Name"})
	assert.NotNil(err)
	assert.Regexp("cluster-name requires domain", err)

	// cache cluster fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	dm = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	dOps.EXPECT().CspDomainList(dm).Return(resCspDomains, nil)
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clM).Return(nil, fmt.Errorf("cluster-cache-err"))
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initPool()
	err = parseAndRun([]string{"pool", "list"})
	assert.NotNil(err)
	assert.Regexp("cluster-cache-err", err.Error())

	mockCtrl.Finish()
	t.Log("init context failure")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "System").Return("", fmt.Errorf("ctx error"))
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initPool()
	err = parseAndRun([]string{"pool", "list", "-A", "System"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initPool()
	err = parseAndRun([]string{"pool", "list", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestPoolGet(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	reqToFetch := "SP-1"
	fParams := pool.NewPoolFetchParams()
	fParams.ID = reqToFetch
	now := time.Now()
	fRet := &pool.PoolFetchOK{
		Payload: &models.Pool{
			PoolAllOf0: models.PoolAllOf0{
				Meta: &models.ObjMeta{
					ID:           "id",
					Version:      1,
					TimeCreated:  strfmt.DateTime(now),
					TimeModified: strfmt.DateTime(now),
				},
			},
			PoolCreateOnce: models.PoolCreateOnce{
				AuthorizedAccountID: "SystemAccountID",
				ClusterID:           "CLUSTER-1",
				CspDomainID:         "87956a70-4108-4a9a-8ca9-e391505db88c",
				CspStorageType:      "Amazon gp2",
				StorageAccessibility: &models.StorageAccessibilityMutable{
					AccessibilityScope:      "CSPDOMAIN",
					AccessibilityScopeObjID: "87956a70-4108-4a9a-8ca9-e391505db88c",
				},
			},
			PoolMutable: models.PoolMutable{
				PoolMutableAllOf0: models.PoolMutableAllOf0{
					ServicePlanReservations: map[string]models.StorageTypeReservation{},
				},
				PoolCreateMutable: models.PoolCreateMutable{
					SystemTags: []string{"tag1", "tag2"},
				},
			},
		},
	}
	dmM := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	resCspDomains := &csp_domain.CspDomainListOK{
		Payload: []*models.CSPDomain{
			&models.CSPDomain{
				CSPDomainAllOf0: models.CSPDomainAllOf0{
					Meta: &models.ObjMeta{
						ID: "87956a70-4108-4a9a-8ca9-e391505db88c",
					},
					CspDomainType: models.CspDomainTypeMutable("AWS"),
				},
				CSPDomainMutable: models.CSPDomainMutable{
					Name: "domainName",
				},
			},
		},
	}
	aM := mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	resAccounts := &account.AccountListOK{
		Payload: []*models.Account{
			&models.Account{
				AccountAllOf0: models.AccountAllOf0{
					Meta: &models.ObjMeta{
						ID: "SystemAccountID",
					},
				},
				AccountMutable: models.AccountMutable{
					Name: "System",
				},
			},
		},
	}
	svpM := mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	resSvcP := &service_plan.ServicePlanListOK{
		Payload: []*models.ServicePlan{
			&models.ServicePlan{
				ServicePlanAllOf0: models.ServicePlanAllOf0{
					Meta: &models.ObjMeta{
						ID: "PLAN-1",
					},
				},
				ServicePlanMutable: models.ServicePlanMutable{
					Name: "ServicePlan",
				},
			},
		},
	}

	// fetch
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	spOps := mockmgmtclient.NewMockPoolClient(mockCtrl)
	mAPI.EXPECT().Pool().Return(spOps).MinTimes(1)
	mF := mockmgmtclient.NewPoolMatcher(t, fParams)
	spOps.EXPECT().PoolFetch(mF).Return(fRet, nil)
	domOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(dmM).Return(resCspDomains, nil)
	aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	svOps := mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(svOps)
	svOps.EXPECT().ServicePlanList(svpM).Return(resSvcP, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initPool()
	err := parseAndRun([]string{"pool", "get", reqToFetch})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(poolDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, 1)

	// apiError
	apiErr := &pool.PoolFetchDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	spOps = mockmgmtclient.NewMockPoolClient(mockCtrl)
	mAPI.EXPECT().Pool().Return(spOps).MinTimes(1)
	mF = mockmgmtclient.NewPoolMatcher(t, fParams)
	spOps.EXPECT().PoolFetch(mF).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initPool()
	err = parseAndRun([]string{"pool", "get", "--id", reqToFetch})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	mockCtrl.Finish()
	t.Log("init context failure")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "System").Return("", fmt.Errorf("ctx error"))
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initPool()
	err = parseAndRun([]string{"pool", "get", "-A", "System", "--id", reqToFetch})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// invalid columns
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initPool()
	err = parseAndRun([]string{"pool", "get", "--id", reqToFetch, "--columns", "foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	t.Log("expected id")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initPool()
	err = parseAndRun([]string{"pool", "get"})
	assert.NotNil(err)
	assert.Regexp("expected --id flag or a single identifier argument", err)
	appCtx.Account, appCtx.AccountID = "", ""
}
