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
	"io/ioutil"
	"os"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/cluster"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/node"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan_allocation"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestServicePlanAllocationMakeRecord(t *testing.T) {
	assert := assert.New(t)

	cmd := &servicePlanAllocationCmd{}
	cmd.accounts = map[string]ctxIDName{
		"accountID": {"", "System"},
	}
	cmd.cspDomains = map[string]string{
		"cspDomainID": "MyAWS",
	}
	cmd.clusters = map[string]ctxIDName{
		"clusterID": ctxIDName{id: "cspDomainID", name: "MyCluster"},
	}
	cmd.servicePlans = map[string]string{
		"planID": "planet",
	}
	o := &models.ServicePlanAllocation{
		ServicePlanAllocationAllOf0: models.ServicePlanAllocationAllOf0{
			Meta: &models.ObjMeta{
				ID:      "id",
				Version: 1,
			},
			CspDomainID: "cspDomainID",
		},
		ServicePlanAllocationCreateOnce: models.ServicePlanAllocationCreateOnce{
			AuthorizedAccountID: "accountID",
			ClusterID:           "clusterID",
			ServicePlanID:       "planID",
		},
		ServicePlanAllocationMutable: models.ServicePlanAllocationMutable{
			ServicePlanAllocationMutableAllOf0: models.ServicePlanAllocationMutableAllOf0{
				ReservableCapacityBytes: swag.Int64(10000),
			},
			ServicePlanAllocationCreateMutable: models.ServicePlanAllocationCreateMutable{
				ReservationState:   "OK",
				TotalCapacityBytes: swag.Int64(20000),
				Tags:               []string{"tag1"},
			},
		},
	}

	rec := cmd.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(servicePlanAllocationHeaders))
	assert.Equal("System", rec[hAuthorizedAccount])
	assert.Equal("id", rec[hID])
	assert.Equal("MyCluster", rec[hCluster])
	assert.Equal("MyAWS", rec[hCspDomain])
	assert.Equal("20KB", rec[hTotalCapacityBytes])
	assert.Equal("10KB", rec[hReservableCapacityBytes])
	assert.Equal("OK", rec[hState])
	assert.Equal("planet", rec[hServicePlan])
	assert.Equal("tag1", rec[hTags])

	// repeat without maps
	cmd.accounts = map[string]ctxIDName{}
	cmd.cspDomains = map[string]string{}
	cmd.clusters = map[string]ctxIDName{}
	cmd.servicePlans = map[string]string{}

	rec = cmd.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(servicePlanAllocationHeaders))
	assert.Equal("accountID", rec[hAuthorizedAccount])
	assert.Equal("cspDomainID", rec[hCspDomain])
	assert.Equal("clusterID", rec[hCluster])
	assert.Equal("planID", rec[hServicePlan])
	assert.Equal("20KB", rec[hTotalCapacityBytes])
	assert.Equal("10KB", rec[hReservableCapacityBytes])
	assert.Equal("id", rec[hID])
	assert.Equal("OK", rec[hState])

}

func TestServicePlanAllocationGet(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	reqToFetch := "SPA-1"
	fParams := service_plan_allocation.NewServicePlanAllocationFetchParams()
	fParams.ID = reqToFetch

	fRet := &service_plan_allocation.ServicePlanAllocationFetchOK{
		Payload: &models.ServicePlanAllocation{
			ServicePlanAllocationAllOf0: models.ServicePlanAllocationAllOf0{
				Meta: &models.ObjMeta{
					ID:      "id",
					Version: 1,
				},
				CspDomainID: "CSP-DOMAIN-1",
			},
			ServicePlanAllocationCreateOnce: models.ServicePlanAllocationCreateOnce{
				AuthorizedAccountID: "authAccountID",
				ClusterID:           "clusterID",
				ServicePlanID:       "planID",
			},
			ServicePlanAllocationMutable: models.ServicePlanAllocationMutable{
				ServicePlanAllocationMutableAllOf0: models.ServicePlanAllocationMutableAllOf0{
					ReservableCapacityBytes: swag.Int64(10000),
					ClusterDescriptor: map[string]models.ValueType{
						"cdKey": models.ValueType{Kind: "STRING", Value: "Key value\n"},
					},
				},
				ServicePlanAllocationCreateMutable: models.ServicePlanAllocationCreateMutable{
					ReservationState:   "OK",
					TotalCapacityBytes: swag.Int64(20000),
				},
			},
		},
	}

	resAccounts := &account.AccountListOK{
		Payload: []*models.Account{
			&models.Account{
				AccountAllOf0: models.AccountAllOf0{
					Meta: &models.ObjMeta{
						ID: "authAccountID",
					},
				},
				AccountMutable: models.AccountMutable{
					Name: "System",
				},
			},
		},
	}

	resPlans := &service_plan.ServicePlanListOK{
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

	outFile := "./spa-descriptor"
	defer func() { os.Remove(outFile) }()

	// fetch, cover cache list failures ignored in Emit
	dbError := fmt.Errorf("db error")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(cOps).MinTimes(1)
	mF := mockmgmtclient.NewServicePlanAllocationMatcher(t, fParams)
	cOps.EXPECT().ServicePlanAllocationFetch(mF).Return(fRet, nil)
	accountMatcher := mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	accountOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(accountOps)
	accountOps.EXPECT().AccountList(accountMatcher).Return(resAccounts, nil)
	spMatcher := mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	spOps := mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps)
	spOps.EXPECT().ServicePlanList(spMatcher).Return(resPlans, nil)
	domMatcher := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(nil, dbError)
	clMatcher := mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(nil, dbError)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err := parseAndRun([]string{"service-plan-allocation", "get", reqToFetch})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(servicePlanAllocationDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, 1)

	// emit cluster descriptor key
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(cOps).MinTimes(1)
	mF = mockmgmtclient.NewServicePlanAllocationMatcher(t, fParams)
	cOps.EXPECT().ServicePlanAllocationFetch(mF).Return(fRet, nil)
	appCtx.API = mAPI
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"service-plan-allocation", "get", "--id", reqToFetch, "-K", "cdKey"})
	assert.Nil(err)
	resBytes, err := ioutil.ReadFile(outFile)
	assert.Nil(err)
	assert.Equal(fRet.Payload.ClusterDescriptor["cdKey"].Value, string(resBytes))

	// fail on invalid cluster descriptor key, use stdout
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(cOps).MinTimes(1)
	mF = mockmgmtclient.NewServicePlanAllocationMatcher(t, fParams)
	cOps.EXPECT().ServicePlanAllocationFetch(mF).Return(fRet, nil)
	appCtx.API = mAPI
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"service-plan-allocation", "get", "--id", reqToFetch, "-K", "fooKey", "-O", "-"})
	assert.NotNil(err)
	assert.Regexp("fooKey", err)

	// fail on bad output file
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"service-plan-allocation", "get", "--id", reqToFetch, "-K", "cdKey", "-O", "./bad/outputFile"})
	assert.NotNil(err)
	assert.Regexp("no such file or directory", err)

	// apiError
	apiErr := &service_plan_allocation.ServicePlanAllocationFetchDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(cOps).MinTimes(1)
	mF = mockmgmtclient.NewServicePlanAllocationMatcher(t, fParams)
	cOps.EXPECT().ServicePlanAllocationFetch(mF).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"service-plan-allocation", "get", "--id", reqToFetch})
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
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "get", "--id", reqToFetch, "-A", "System"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// invalid columns
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"service-plan-allocation", "get", "--id", reqToFetch, "--columns", "foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	t.Log("expected id")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"service-plan-allocation", "get"})
	assert.NotNil(err)
	assert.Regexp("expected --id flag or a single identifier argument", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestServicePlanAllocationList(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	resAccounts := &account.AccountListOK{
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

	resCspDomains := &csp_domain.CspDomainListOK{
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

	resClusters := &cluster.ClusterListOK{
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

	resPlans := &service_plan.ServicePlanListOK{
		Payload: []*models.ServicePlan{
			&models.ServicePlan{
				ServicePlanAllOf0: models.ServicePlanAllOf0{
					Meta: &models.ObjMeta{
						ID: "planID",
					},
				},
				ServicePlanMutable: models.ServicePlanMutable{
					Name: "planet",
				},
			},
		},
	}

	resNodes := &node.NodeListOK{
		Payload: []*models.Node{},
	}

	res := &service_plan_allocation.ServicePlanAllocationListOK{
		Payload: []*models.ServicePlanAllocation{
			&models.ServicePlanAllocation{
				ServicePlanAllocationAllOf0: models.ServicePlanAllocationAllOf0{
					Meta: &models.ObjMeta{
						ID:      "id",
						Version: 1,
					},
					CspDomainID: "CSP-DOMAIN-1",
				},
				ServicePlanAllocationCreateOnce: models.ServicePlanAllocationCreateOnce{
					AccountID:           "accountID",
					AuthorizedAccountID: "authAccountID",
					ClusterID:           "clusterID",
					ServicePlanID:       "planID",
				},
				ServicePlanAllocationMutable: models.ServicePlanAllocationMutable{
					ServicePlanAllocationMutableAllOf0: models.ServicePlanAllocationMutableAllOf0{
						ReservableCapacityBytes: swag.Int64(10000),
					},
					ServicePlanAllocationCreateMutable: models.ServicePlanAllocationCreateMutable{
						ReservationState:   "OK",
						TotalCapacityBytes: swag.Int64(20000),
					},
				},
			},
		},
	}

	// list, all defaults
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	params := service_plan_allocation.NewServicePlanAllocationListParams()
	spaOps := mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(spaOps).MinTimes(1)
	mF := mockmgmtclient.NewServicePlanAllocationMatcher(t, params)
	spaOps.EXPECT().ServicePlanAllocationList(mF).Return(res, nil)
	nm := mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps := mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil).MinTimes(1)
	accountMatcher := mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	accountOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(accountOps)
	accountOps.EXPECT().AccountList(accountMatcher).Return(resAccounts, nil).MinTimes(1)
	spMatcher := mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	spOps := mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps)
	spOps.EXPECT().ServicePlanList(spMatcher).Return(resPlans, nil).MinTimes(1)
	domMatcher := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil).MinTimes(1)
	clMatcher := mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(resClusters, nil).MinTimes(1)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err := parseAndRun([]string{"service-plan-allocation", "list"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(servicePlanAllocationDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list, json, options
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "System").Return("accountID", nil)
	nm = mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil).MinTimes(1)
	accountMatcher = mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	accountOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(accountOps)
	accountOps.EXPECT().AccountList(accountMatcher).Return(resAccounts, nil).MinTimes(1)
	spMatcher = mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	spOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps)
	spOps.EXPECT().ServicePlanList(spMatcher).Return(resPlans, nil).MinTimes(1)
	domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil).MinTimes(1)
	clMatcher = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(resClusters, nil).MinTimes(1)
	params = service_plan_allocation.NewServicePlanAllocationListParams()
	mF = mockmgmtclient.NewServicePlanAllocationMatcher(t, params)
	params.ClusterID = swag.String("clusterID")
	params.CspDomainID = swag.String("CSP-DOMAIN-1")
	params.ReservationState = []string{"OK"}
	params.ReservationStateNot = []string{"NOCAPACITY", "DISABLED"}
	params.AuthorizedAccountID = swag.String("authAccountID")
	params.ServicePlanID = swag.String("planID")
	params.Tags = []string{"tag1"}
	params.StorageFormulaName = swag.String("storageFName")
	spaOps = mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(spaOps).MinTimes(1)
	spaOps.EXPECT().ServicePlanAllocationList(mF).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"service-plan-allocation", "list", "-D", "domainName", "-C", "clusterName",
		"--state=OK", "--state=!NOCAPACITY", "--state=!DISABLED", "-A", "System", "-Z", "AuthAccount",
		"-t", "tag1", "-F", "storageFName", "-P", "planet", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(res.Payload, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// list, yaml
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	nm = mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil).MinTimes(1)
	domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil).MinTimes(1)
	clMatcher = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(resClusters, nil).MinTimes(1)
	params = service_plan_allocation.NewServicePlanAllocationListParams()
	params.PoolID = swag.String("poolId")
	spaOps = mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(spaOps).MinTimes(1)
	mF = mockmgmtclient.NewServicePlanAllocationMatcher(t, params)
	spaOps.EXPECT().ServicePlanAllocationList(mF).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"service-plan-allocation", "list", "-o", "yaml", "--pool-id", "poolId"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(res.Payload, te.yamlData)

	// list with follow
	fw := &fakeWatcher{t: t}
	appCtx.watcher = fw
	fw.CallCBCount = 1
	fos := &fakeOSExecutor{}
	appCtx.osExec = fos
	servicePlanAllocationListCmdRunCacheThreshold = 0 // force a cache refresh
	defer func() { servicePlanAllocationListCmdRunCacheThreshold = 1 }()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "AuthAccount").Return("authAccountID", nil)
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil).MinTimes(1)
	accountMatcher = mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	accountOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(accountOps).MinTimes(1)
	accountOps.EXPECT().AccountList(accountMatcher).Return(resAccounts, nil).MinTimes(1)
	spMatcher = mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	spOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps)
	spOps.EXPECT().ServicePlanList(spMatcher).Return(resPlans, nil).MinTimes(1)
	domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps).MinTimes(1)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil).MinTimes(1)
	clMatcher = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps).MinTimes(1)
	clOps.EXPECT().ClusterList(clMatcher).Return(resClusters, nil).MinTimes(1)
	params = service_plan_allocation.NewServicePlanAllocationListParams()
	params.ClusterID = swag.String("clusterID")
	params.CspDomainID = swag.String("CSP-DOMAIN-1")
	params.AuthorizedAccountID = swag.String("authAccountID")
	params.ServicePlanID = swag.String("planID")
	mF = mockmgmtclient.NewServicePlanAllocationMatcher(t, params)
	spaOps = mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(spaOps)
	spaOps.EXPECT().ServicePlanAllocationList(mF).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"service-plan-allocation", "list", "-f", "-D", "domainName", "-C", "clusterName", "-A", "AuthAccount", "--owner-auth", "-P", "planet"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, 7)
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))
	expWArgs := &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{
				URIPattern:   "/service-plan-allocations/?",
				ScopePattern: ".*clusterID:clusterID.*cspDomainID:CSP-DOMAIN-1.*authorizedAccountID:authAccountID.*servicePlanID:planID",
			},
		},
	}
	assert.NotNil(fw.InArgs)
	assert.Equal(expWArgs, fw.InArgs)
	assert.NotNil(fw.InCB)
	assert.Equal(1, fw.NumCalls)
	assert.NoError(fw.CBErr)
	appCtx.Account, appCtx.AccountID = "", ""

	// list with columns, cache list failures ignored
	dbError := fmt.Errorf("db error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	nm = mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil).MinTimes(1)
	accountMatcher = mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	accountOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(accountOps)
	accountOps.EXPECT().AccountList(accountMatcher).Return(nil, dbError)
	spMatcher = mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	spOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps)
	spOps.EXPECT().ServicePlanList(spMatcher).Return(nil, dbError)
	domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil).MinTimes(1)
	clMatcher = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(resClusters, nil).MinTimes(1)
	params = service_plan_allocation.NewServicePlanAllocationListParams()
	spaOps = mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(spaOps).MinTimes(1)
	mF = mockmgmtclient.NewServicePlanAllocationMatcher(t, params)
	spaOps.EXPECT().ServicePlanAllocationList(mF).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"service-plan-allocation", "list", "--columns", "ID,TotalCap"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, 2)
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list with invalid columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"service-plan-allocation", "list", "--columns", "Name,Foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// list, API failure model error
	apiErr := &service_plan_allocation.ServicePlanAllocationListDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	nm = mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil)
	domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil)
	clMatcher = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(resClusters, nil)
	params = service_plan_allocation.NewServicePlanAllocationListParams()
	spaOps = mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(spaOps)
	mF = mockmgmtclient.NewServicePlanAllocationMatcher(t, params)
	spaOps.EXPECT().ServicePlanAllocationList(mF).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"service-plan-allocation", "list"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// list, API failure arbitrary error
	otherErr := fmt.Errorf("OTHER ERROR")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	nm = mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil)
	domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil)
	clMatcher = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(resClusters, nil)
	params = service_plan_allocation.NewServicePlanAllocationListParams()
	spaOps = mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(spaOps)
	mF = mockmgmtclient.NewServicePlanAllocationMatcher(t, params)
	spaOps.EXPECT().ServicePlanAllocationList(mF).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"service-plan-allocation", "list"})
	assert.NotNil(err)
	assert.Equal(otherErr.Error(), err.Error())

	// cspdomain list error
	cspDomainListErr := fmt.Errorf("cspDomainListError")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(nil, cspDomainListErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"service-plan-allocation", "list", "-D", "domain"})
	assert.NotNil(err)
	assert.Equal(cspDomainListErr.Error(), err.Error())

	// cluster list error
	clusterListErr := fmt.Errorf("clusterListErr")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil)
	clMatcher = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(nil, clusterListErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"service-plan-allocation", "list", "-D", "domain"})
	assert.NotNil(err)
	assert.Equal(clusterListErr.Error(), err.Error())

	// service plan list error
	servicePlanListError := fmt.Errorf("servicePlanListError")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil)
	clMatcher = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(resClusters, nil)
	nm = mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil)
	spMatcher = mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	spOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps)
	spOps.EXPECT().ServicePlanList(spMatcher).Return(nil, servicePlanListError)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"service-plan-allocation", "list", "-P", "serviceplan"})
	assert.NotNil(err)
	assert.Equal(servicePlanListError.Error(), err.Error())

	// account list error
	accountListError := fmt.Errorf("accountListError")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil)
	clMatcher = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(resClusters, nil)
	nm = mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil)
	accountMatcher = mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	accountOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(accountOps)
	accountOps.EXPECT().AccountList(accountMatcher).Return(nil, accountListError)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"service-plan-allocation", "list", "-Z", "accountname"})
	assert.NotNil(err)
	assert.Equal(accountListError.Error(), err.Error())

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
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "list", "-A", "System"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// not found cases
	notFoundTCs := []struct {
		args            []string
		pat             string
		needAccount     bool
		needServicePlan bool
		accountErr      error
	}{
		{
			args: []string{"spa", "list", "-D", "foo"},
			pat:  "csp domain.*not found",
		},
		{
			args: []string{"spa", "list", "-D", "domainName", "-C", "foo"},
			pat:  "cluster.*not found",
		},
		{
			args:        []string{"spa", "list", "-D", "domainName", "-C", "clusterName", "-Z", "foo"},
			pat:         "authorized account.*not found",
			needAccount: true,
		},
		{
			args:            []string{"spa", "list", "-D", "domainName", "-C", "clusterName", "-P", "foo"},
			pat:             "service plan.*not found",
			needServicePlan: true,
		},
	}
	for _, tc := range notFoundTCs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
		domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
		domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
		mAPI.EXPECT().CspDomain().Return(domOps)
		domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil)
		clMatcher = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
		clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
		mAPI.EXPECT().Cluster().Return(clOps)
		clOps.EXPECT().ClusterList(clMatcher).Return(resClusters, nil)
		nm = mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
		nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
		mAPI.EXPECT().Node().Return(nOps)
		nOps.EXPECT().NodeList(nm).Return(resNodes, nil)
		if tc.needServicePlan {
			spMatcher = mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
			spOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
			mAPI.EXPECT().ServicePlan().Return(spOps)
			spOps.EXPECT().ServicePlanList(spMatcher).Return(resPlans, nil)
		}
		if tc.needAccount {
			accountMatcher = mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
			accountOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
			mAPI.EXPECT().Account().Return(accountOps)
			accountOps.EXPECT().AccountList(accountMatcher).Return(resAccounts, nil)
		}
		appCtx.API = mAPI
		te = &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initServicePlanAllocation()
		err = parseAndRun(tc.args)
		assert.NotNil(err)
		assert.Regexp(tc.pat, err.Error())
	}

	// invalid flag combinations
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "list", "-C", "clusterName"})
	assert.Error(err)
	assert.Regexp("cluster-name requires domain", err)

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "list", "-Z", "auth", "--owner-auth"})
	assert.Error(err)
	assert.Regexp("do not specify --auth-account-name and --owner-auth together", err)

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "list", "--owner-auth"})
	assert.Error(err)
	assert.Regexp("owner-auth requires --account", err)

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"service-plan-allocation", "list", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestServicePlanAllocationModify(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	resAccounts := &account.AccountListOK{
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

	resCspDomains := &csp_domain.CspDomainListOK{
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

	resClusters := &cluster.ClusterListOK{
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

	resPlans := &service_plan.ServicePlanListOK{
		Payload: []*models.ServicePlan{
			&models.ServicePlan{
				ServicePlanAllOf0: models.ServicePlanAllOf0{
					Meta: &models.ObjMeta{
						ID: "planID",
					},
				},
				ServicePlanMutable: models.ServicePlanMutable{
					Name: "planet",
				},
			},
		},
	}

	resNodes := &node.NodeListOK{
		Payload: []*models.Node{},
	}

	lRes := &service_plan_allocation.ServicePlanAllocationListOK{
		Payload: []*models.ServicePlanAllocation{
			&models.ServicePlanAllocation{
				ServicePlanAllocationAllOf0: models.ServicePlanAllocationAllOf0{
					Meta: &models.ObjMeta{
						ID:      "id",
						Version: 1,
					},
					CspDomainID: "CSP-DOMAIN-1",
				},
				ServicePlanAllocationCreateOnce: models.ServicePlanAllocationCreateOnce{
					AccountID:           "accountID",
					AuthorizedAccountID: "authAccountID",
					ClusterID:           "clusterID",
					ServicePlanID:       "planID",
				},
				ServicePlanAllocationMutable: models.ServicePlanAllocationMutable{
					ServicePlanAllocationMutableAllOf0: models.ServicePlanAllocationMutableAllOf0{
						ReservableCapacityBytes: swag.Int64(10000),
					},
					ServicePlanAllocationCreateMutable: models.ServicePlanAllocationCreateMutable{
						ReservationState:   "OK",
						TotalCapacityBytes: swag.Int64(20000),
					},
				},
			},
		},
	}
	lParams := service_plan_allocation.NewServicePlanAllocationListParams()
	lParams.CspDomainID = swag.String(string(lRes.Payload[0].CspDomainID))
	lParams.ClusterID = swag.String(string(lRes.Payload[0].ClusterID))
	lParams.AuthorizedAccountID = swag.String(string(lRes.Payload[0].AuthorizedAccountID))
	lParams.ServicePlanID = swag.String(string(lRes.Payload[0].ServicePlanID))
	lm := mockmgmtclient.NewServicePlanAllocationMatcher(t, lParams)

	// update (APPEND)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(cOps).MinTimes(1)
	cOps.EXPECT().ServicePlanAllocationList(lm).Return(lRes, nil)
	domMatcher := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil)
	clMatcher := mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(resClusters, nil)
	nm := mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps := mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil)
	spMatcher := mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	spOps := mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps)
	spOps.EXPECT().ServicePlanList(spMatcher).Return(resPlans, nil)
	accountMatcher := mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	accountOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(accountOps)
	accountOps.EXPECT().AccountList(accountMatcher).Return(resAccounts, nil)
	uParams := service_plan_allocation.NewServicePlanAllocationUpdateParams()
	uParams.Payload = &models.ServicePlanAllocationMutable{
		ServicePlanAllocationCreateMutable: models.ServicePlanAllocationCreateMutable{
			ReservationState: "DISABLED",
			Tags:             []string{"tag1", "tag2"},
		},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"reservationState"}
	uParams.Append = []string{"tags"}
	um := mockmgmtclient.NewServicePlanAllocationMatcher(t, uParams)
	uRet := service_plan_allocation.NewServicePlanAllocationUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().ServicePlanAllocationUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err := parseAndRun([]string{"spa", "modify", "-Z", "AuthAccount", "-D", "domainName",
		"-C", "clusterName", "-P", "planet", "--disable-reservation",
		"-t", "tag1", "-t", "tag2", "-V", "1", "-o", "json",
	})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.ServicePlanAllocation{uRet.Payload}, te.jsonData)

	// similar but with SET, also cover case of enabling DISABLED SPA
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(cOps).MinTimes(1)
	cOps.EXPECT().ServicePlanAllocationList(lm).Return(lRes, nil)
	lRes.Payload[0].ReservationState = "DISABLED"
	domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil)
	clMatcher = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(resClusters, nil)
	nm = mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil)
	spMatcher = mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	spOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps)
	spOps.EXPECT().ServicePlanList(spMatcher).Return(resPlans, nil)
	accountMatcher = mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	accountOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(accountOps)
	accountOps.EXPECT().AccountList(accountMatcher).Return(resAccounts, nil)
	uParams = service_plan_allocation.NewServicePlanAllocationUpdateParams()
	uParams.Payload = &models.ServicePlanAllocationMutable{
		ServicePlanAllocationCreateMutable: models.ServicePlanAllocationCreateMutable{
			ReservationState: "UNKNOWN",
			Tags:             []string{"tag1", "tag2"},
		},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"reservationState", "tags"}
	um = mockmgmtclient.NewServicePlanAllocationMatcher(t, uParams)
	uRet = service_plan_allocation.NewServicePlanAllocationUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().ServicePlanAllocationUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "modify", "-Z", "AuthAccount", "-D", "domainName",
		"-C", "clusterName", "-P", "planet", "--enable-reservation",
		"-t", "tag1", "-t", "tag2", "--tag-action", "SET", "-V", "1", "-o", "json",
	})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.ServicePlanAllocation{uRet.Payload}, te.jsonData)
	lRes.Payload[0].ReservationState = "OK" // undo

	// similar but with remove
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(cOps).MinTimes(1)
	cOps.EXPECT().ServicePlanAllocationList(lm).Return(lRes, nil)
	domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil)
	clMatcher = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(resClusters, nil)
	nm = mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil)
	spMatcher = mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	spOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps)
	spOps.EXPECT().ServicePlanList(spMatcher).Return(resPlans, nil)
	accountMatcher = mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	accountOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(accountOps)
	accountOps.EXPECT().AccountList(accountMatcher).Return(resAccounts, nil)
	uParams = service_plan_allocation.NewServicePlanAllocationUpdateParams()
	uParams.Payload = &models.ServicePlanAllocationMutable{
		ServicePlanAllocationCreateMutable: models.ServicePlanAllocationCreateMutable{
			ReservationState: "DISABLED",
			Tags:             []string{"tag1", "tag2"},
		},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"reservationState"}
	uParams.Remove = []string{"tags"}
	um = mockmgmtclient.NewServicePlanAllocationMatcher(t, uParams)
	uRet = service_plan_allocation.NewServicePlanAllocationUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().ServicePlanAllocationUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "modify", "-Z", "AuthAccount", "-D", "domainName", "--disable-reservation",
		"-C", "clusterName", "-P", "planet", "-t", "tag1", "-t", "tag2", "--tag-action", "REMOVE", "-V", "1", "-o", "json",
	})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.ServicePlanAllocation{uRet.Payload}, te.jsonData)

	// Enabling reservation when already in enabled state
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(cOps).MinTimes(1)
	cOps.EXPECT().ServicePlanAllocationList(lm).Return(lRes, nil)
	domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil)
	clMatcher = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(resClusters, nil)
	nm = mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil)
	spMatcher = mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	spOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps)
	spOps.EXPECT().ServicePlanList(spMatcher).Return(resPlans, nil)
	accountMatcher = mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	accountOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(accountOps)
	accountOps.EXPECT().AccountList(accountMatcher).Return(resAccounts, nil)
	uParams = service_plan_allocation.NewServicePlanAllocationUpdateParams()
	uParams.Payload = &models.ServicePlanAllocationMutable{
		ServicePlanAllocationCreateMutable: models.ServicePlanAllocationCreateMutable{
			Tags: []string{"tag1", "tag2"},
		},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Remove = []string{"tags"}
	um = mockmgmtclient.NewServicePlanAllocationMatcher(t, uParams)
	uRet = service_plan_allocation.NewServicePlanAllocationUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().ServicePlanAllocationUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "modify", "-Z", "AuthAccount", "-D", "domainName", "--enable-reservation",
		"-C", "clusterName", "-P", "planet", "-t", "tag1", "-t", "tag2", "--tag-action", "REMOVE", "-V", "1", "-o", "json",
	})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.ServicePlanAllocation{uRet.Payload}, te.jsonData)

	// no changes
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(cOps).MinTimes(1)
	cOps.EXPECT().ServicePlanAllocationList(lm).Return(lRes, nil)
	domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil)
	clMatcher = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(resClusters, nil)
	nm = mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil)
	spMatcher = mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	spOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps)
	spOps.EXPECT().ServicePlanList(spMatcher).Return(resPlans, nil)
	accountMatcher = mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	accountOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(accountOps)
	accountOps.EXPECT().AccountList(accountMatcher).Return(resAccounts, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "modify", "-Z", "AuthAccount", "-D", "domainName",
		"-C", "clusterName", "-P", "planet", "-V", "1", "-o", "json",
	})
	assert.NotNil(err)
	assert.Nil(te.jsonData)
	assert.Regexp("No modifications", err.Error())

	// update error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	updateErr := service_plan_allocation.NewServicePlanAllocationUpdateDefault(400)
	updateErr.Payload = &models.Error{Code: 400, Message: swag.String("update error")}
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(cOps).MinTimes(1)
	cOps.EXPECT().ServicePlanAllocationList(lm).Return(lRes, nil)
	domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil)
	clMatcher = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(resClusters, nil)
	nm = mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil)
	spMatcher = mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	spOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps)
	spOps.EXPECT().ServicePlanList(spMatcher).Return(resPlans, nil)
	accountMatcher = mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	accountOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(accountOps)
	accountOps.EXPECT().AccountList(accountMatcher).Return(resAccounts, nil)
	uParams = service_plan_allocation.NewServicePlanAllocationUpdateParams()
	uParams.Payload = &models.ServicePlanAllocationMutable{
		ServicePlanAllocationCreateMutable: models.ServicePlanAllocationCreateMutable{
			ReservationState: "DISABLED",
			Tags:             []string{"tag1", "tag2"},
		},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"reservationState"}
	uParams.Append = []string{"tags"}
	um = mockmgmtclient.NewServicePlanAllocationMatcher(t, uParams)
	uRet = service_plan_allocation.NewServicePlanAllocationUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().ServicePlanAllocationUpdate(um).Return(nil, updateErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "modify", "-Z", "AuthAccount", "-D", "domainName",
		"-C", "clusterName", "-P", "planet", "--disable-reservation",
		"-t", "tag1", "-t", "tag2", "-V", "1", "-o", "json",
	})
	assert.NotNil(err)
	assert.Regexp("update error", err.Error())

	// other error on update
	otherErr := fmt.Errorf("other error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(cOps).MinTimes(1)
	cOps.EXPECT().ServicePlanAllocationList(lm).Return(lRes, nil)
	domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil)
	clMatcher = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(resClusters, nil)
	nm = mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil)
	spMatcher = mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	spOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps)
	spOps.EXPECT().ServicePlanList(spMatcher).Return(resPlans, nil)
	accountMatcher = mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	accountOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(accountOps)
	accountOps.EXPECT().AccountList(accountMatcher).Return(resAccounts, nil)
	uParams = service_plan_allocation.NewServicePlanAllocationUpdateParams()
	uParams.Payload = &models.ServicePlanAllocationMutable{
		ServicePlanAllocationCreateMutable: models.ServicePlanAllocationCreateMutable{
			ReservationState: "DISABLED",
			Tags:             []string{"tag1", "tag2"},
		},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"reservationState"}
	uParams.Append = []string{"tags"}
	um = mockmgmtclient.NewServicePlanAllocationMatcher(t, uParams)
	uRet = service_plan_allocation.NewServicePlanAllocationUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().ServicePlanAllocationUpdate(um).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "modify", "-Z", "AuthAccount", "-D", "domainName",
		"-C", "clusterName", "-P", "planet", "--disable-reservation",
		"-t", "tag1", "-t", "tag2", "-V", "1", "-o", "json",
	})
	assert.NotNil(err)
	assert.Regexp(otherErr.Error(), err.Error())

	// invalid columns
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "modify", "-Zz", "-Dd", "-Cc", "-Pp", "--columns", "ID,foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err.Error())

	// not found
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "AuthAccount").Return("authAccountID", nil)
	domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil)
	clMatcher = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(resClusters, nil)
	nm = mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil)
	spMatcher = mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	spOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps)
	spOps.EXPECT().ServicePlanList(spMatcher).Return(resPlans, nil)
	emptyRes := &service_plan_allocation.ServicePlanAllocationListOK{
		Payload: []*models.ServicePlanAllocation{},
	}
	cOps = mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(cOps).MinTimes(1)
	cOps.EXPECT().ServicePlanAllocationList(lm).Return(emptyRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "modify", "-A", "AuthAccount", "-D", "domainName",
		"-C", "clusterName", "-P", "planet", "--disable-reservation",
		"-t", "tag1", "-t", "tag2", "-V", "1", "-o", "json",
	})
	assert.NotNil(err)
	assert.Regexp("not found", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// list error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil)
	clMatcher = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(resClusters, nil)
	nm = mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil)
	spMatcher = mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	spOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps)
	spOps.EXPECT().ServicePlanList(spMatcher).Return(resPlans, nil)
	accountMatcher = mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	accountOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(accountOps)
	accountOps.EXPECT().AccountList(accountMatcher).Return(resAccounts, nil)
	cOps = mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(cOps).MinTimes(1)
	cOps.EXPECT().ServicePlanAllocationList(lm).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "modify", "-Z", "AuthAccount", "-D", "domainName",
		"-C", "clusterName", "-P", "planet", "--disable-reservation",
		"-t", "tag1", "-t", "tag2", "-V", "1", "-o", "json",
	})
	assert.NotNil(err)
	assert.Regexp(otherErr.Error(), err.Error())

	// disable and enable allocation
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "modify", "-Z", "AuthAccount", "-D", "domainName",
		"-C", "clusterName", "-P", "planet", "--disable-reservation", "--enable-reservation"})
	assert.NotNil(err)
	assert.Regexp("enable.*disable.*together", err.Error())

	// missing required fields
	missingTCs := [][]string{
		[]string{"spa", "modify", "-Z", "AuthAccount", "-C", "clusterName", "-P", "planet"},
		[]string{"spa", "modify", "-Z", "AuthAccount", "-D", "domainName", "-P", "planet"},
		[]string{"spa", "modify", "-Z", "AuthAccount", "-D", "domainName", "-C", "clusterName"},
	}
	for _, tc := range missingTCs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
		appCtx.API = mAPI
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initServicePlanAllocation()
		err = parseAndRun(tc)
		assert.NotNil(err)
		assert.Regexp("the required flag.*was not specified", err.Error())
	}

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "modify", "-D", "domainName", "-C", "clusterName", "-P", "planet"})
	assert.Error(err)
	assert.Regexp("Neither .*account.*authorized-account.*specified", err)

	// service plan not found
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	emptyPlanRes := &service_plan.ServicePlanListOK{
		Payload: []*models.ServicePlan{},
	}
	domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil)
	clMatcher = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(resClusters, nil)
	nm = mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil)
	accountMatcher = mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	accountOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(accountOps)
	accountOps.EXPECT().AccountList(accountMatcher).Return(resAccounts, nil)
	spMatcher = mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	spOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps)
	spOps.EXPECT().ServicePlanList(spMatcher).Return(emptyPlanRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "modify", "-Z", "AuthAccount", "-D", "domainName",
		"-C", "clusterName", "-P", "planet", "--disable-reservation",
		"-t", "tag1", "-t", "tag2", "-V", "1", "-o", "json",
	})
	assert.NotNil(err)
	assert.Regexp("not found", err.Error())

	// service plan list error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil)
	clMatcher = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(resClusters, nil)
	nm = mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil)
	accountMatcher = mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	accountOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(accountOps)
	accountOps.EXPECT().AccountList(accountMatcher).Return(resAccounts, nil)
	spMatcher = mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	spOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(spOps)
	spOps.EXPECT().ServicePlanList(spMatcher).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "modify", "-Z", "AuthAccount", "-D", "domainName",
		"-C", "clusterName", "-P", "planet", "--disable-reservation",
		"-t", "tag1", "-t", "tag2", "-V", "1", "-o", "json",
	})
	assert.NotNil(err)
	assert.Regexp(otherErr.Error(), err.Error())

	// authorized account not found
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	emptyAccountRes := &account.AccountListOK{
		Payload: []*models.Account{},
	}
	domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil)
	clMatcher = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(resClusters, nil)
	nm = mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil)
	accountMatcher = mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	accountOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(accountOps)
	accountOps.EXPECT().AccountList(accountMatcher).Return(emptyAccountRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "modify", "-Z", "AuthAccount", "-D", "domainName",
		"-C", "clusterName", "-P", "planet", "--disable-reservation",
		"-t", "tag1", "-t", "tag2", "-V", "1", "-o", "json",
	})
	assert.NotNil(err)
	assert.Regexp("not found", err.Error())

	// authorized account list error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil)
	clMatcher = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(resClusters, nil)
	nm = mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil)
	accountMatcher = mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	accountOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(accountOps)
	accountOps.EXPECT().AccountList(accountMatcher).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "modify", "-Z", "AuthAccount", "-D", "domainName",
		"-C", "clusterName", "-P", "planet", "--disable-reservation",
		"-t", "tag1", "-t", "tag2", "-V", "1", "-o", "json",
	})
	assert.NotNil(err)
	assert.Regexp(otherErr.Error(), err.Error())

	// cluster list error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil)
	clMatcher = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "modify", "-Z", "AuthAccount", "-D", "domainName",
		"-C", "clusterName", "-P", "planet", "--disable-reservation",
		"-t", "tag1", "-t", "tag2", "-V", "1", "-o", "json",
	})
	assert.NotNil(err)
	assert.Regexp(otherErr.Error(), err.Error())

	// domain list error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	domMatcher = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "modify", "-Z", "AuthAccount", "-D", "domainName",
		"-C", "clusterName", "-P", "planet", "--disable-reservation",
		"-t", "tag1", "-t", "tag2", "-V", "1", "-o", "json",
	})
	assert.NotNil(err)
	assert.Regexp(otherErr.Error(), err.Error())

	mockCtrl.Finish()
	t.Log("init context failure")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "Tenant").Return("", fmt.Errorf("ctx error"))
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "modify", "-A", "Tenant", "-Z", "sub", "-D", "d", "-C", "c", "-P", "p"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "modify", "-Z", "AuthAccount", "-D", "domainName",
		"-C", "clusterName", "-P", "planet", "--disable-reservation",
		"-t", "tag1", "-t", "tag2", "-V", "1", "o", "json",
	})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestServicePlanAllocationCustomizeProvisioning(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	outFile := "./spa-secret"
	defer func() { os.Remove(outFile) }()

	t.Log("success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(cOps).MinTimes(1)
	cpParams := service_plan_allocation.NewServicePlanAllocationCustomizeProvisioningParams()
	cpParams.ID = "spaID"
	cpParams.ApplicationGroupName = swag.String("agName")
	cpParams.ApplicationGroupDescription = swag.String("agDesc")
	cpParams.ApplicationGroupTag = []string{"agT1", "agT2"}
	cpParams.ConsistencyGroupName = swag.String("cgName")
	cpParams.ConsistencyGroupDescription = swag.String("cgDesc")
	cpParams.ConsistencyGroupTag = []string{"cgT1", "cgT2"}
	cpParams.VolumeSeriesTag = []string{"vsT1", "vsT2"}
	cpParams.K8sName = swag.String("k8sName")
	cpParams.K8sNamespace = swag.String("k8sNamespace")
	mCP := mockmgmtclient.NewServicePlanAllocationMatcher(t, cpParams)
	cpRet := service_plan_allocation.NewServicePlanAllocationCustomizeProvisioningOK()
	cpRet.Payload = &models.ValueType{Kind: "SECRET", Value: "secret data"}
	cOps.EXPECT().ServicePlanAllocationCustomizeProvisioning(mCP).Return(cpRet, nil)
	appCtx.API = mAPI
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err := parseAndRun([]string{"spa", "customize-provisioning", "spaID",
		"--application-group-name", "agName",
		"--application-group-description", "agDesc",
		"--application-group-tag", "agT1", "--application-group-tag", "agT2",
		"--consistency-group-name", "cgName",
		"--consistency-group-description", "cgDesc",
		"--consistency-group-tag", "cgT1", "--consistency-group-tag", "cgT2",
		"--volume-series-tag", "vsT1", "--volume-series-tag", "vsT2",
		"--k8s-name", "k8sName", "--k8s-namespace", "k8sNamespace",
		"-O", outFile})
	assert.NoError(err)
	mockCtrl.Finish()

	t.Log("case: api failure, stdout")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(cOps).MinTimes(1)
	cpErr := service_plan_allocation.NewServicePlanAllocationCustomizeProvisioningDefault(400)
	cpErr.Payload = &models.Error{Code: 400, Message: swag.String("cpErr")}
	cOps.EXPECT().ServicePlanAllocationCustomizeProvisioning(mCP).Return(nil, cpErr)
	appCtx.API = mAPI
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "customize-provisioning", "--id", "spaID",
		"--application-group-name", "agName",
		"--application-group-description", "agDesc",
		"--application-group-tag", "agT1", "--application-group-tag", "agT2",
		"--consistency-group-name", "cgName",
		"--consistency-group-description", "cgDesc",
		"--consistency-group-tag", "cgT1", "--consistency-group-tag", "cgT2",
		"--volume-series-tag", "vsT1", "--volume-series-tag", "vsT2",
		"--k8s-name", "k8sName", "--k8s-namespace", "k8sNamespace",
		"-O", "-"})
	assert.Error(err)
	assert.Regexp("cpErr", err)

	t.Log("case: invalid output file")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "customize", "--id", "spaID", "-O", "./bad/file"})
	assert.NotNil(err)
	assert.Regexp("no such file or directory", err.Error())

	mockCtrl.Finish()
	t.Log("case: init context failure")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "System").Return("", fmt.Errorf("ctx error"))
	appCtx.API = mAPI
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "customize", "--id", "spaID", "-A", "System"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("expected id")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlanAllocation()
	err = parseAndRun([]string{"spa", "customize-provisioning",
		"--application-group-name", "agName",
		"--application-group-description", "agDesc",
		"--application-group-tag", "agT1", "--application-group-tag", "agT2",
		"--consistency-group-name", "cgName",
		"--consistency-group-description", "cgDesc",
		"--consistency-group-tag", "cgT1", "--consistency-group-tag", "cgT2",
		"--volume-series-tag", "vsT1", "--volume-series-tag", "vsT2",
		"--k8s-name", "k8sName", "--k8s-namespace", "k8sNamespace",
		"-O", outFile})
	assert.NotNil(err)
	assert.Regexp("expected --id flag or a single identifier argument", err)
	appCtx.Account, appCtx.AccountID = "", ""
}
