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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/cluster"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/node"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestStorageRequestMakeRecord(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	cmd := &storageRequestCmd{}
	cmd.accounts = map[string]ctxIDName{
		"authAccountID": ctxIDName{id: "", name: "AuthAccount"},
	}
	cmd.cspDomains = map[string]string{
		"cspDomainID": "MyAWS",
	}
	cmd.clusters = map[string]ctxIDName{
		"clusterID": ctxIDName{id: "cspDomainID", name: "MyCluster"},
	}
	cmd.nodes = map[string]ctxIDName{
		"nodeID": ctxIDName{id: "clusterID", name: "node1"},
	}
	o := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:           "id",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
		},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			AccountID:           "authAccountID",
			TenantAccountID:     "tID",
			ClusterID:           "clusterID",
			CompleteByTime:      strfmt.DateTime(now),
			CspDomainID:         "cspDomainID",
			CspStorageType:      "Amazon gp2",
			MinSizeBytes:        swag.Int64(1073741824),
			ParcelSizeBytes:     swag.Int64(1048576),
			PoolID:              "poolID",
			RequestedOperations: []string{"PROVISION", "ATTACH", "FORMAT", "USE"},
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
				StorageRequestState: "PROVISIONING",
			},
			StorageRequestCreateMutable: models.StorageRequestCreateMutable{
				NodeID:    "nodeID",
				StorageID: "storageID",
			},
		},
	}
	rec := cmd.makeRecord(o, now)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(storageRequestHeaders))
	assert.Equal("id", rec[hID])
	assert.Equal(strfmt.DateTime(now).String(), rec[hCompleteBy])
	assert.Equal("1GiB", rec[hMinSizeBytes])
	assert.Equal("PROVISION, ATTACH, FORMAT, USE", rec[hRequestedOperations])
	assert.Equal("AuthAccount", rec[hAccount])
	assert.Equal("Amazon gp2", rec[hCSPStorageType])
	assert.Equal("PROVISIONING", rec[hRequestState])
	assert.Equal("MyAWS", rec[hCspDomain])
	assert.Equal("MyCluster", rec[hCluster])
	assert.Equal("node1", rec[hNode])
	assert.Equal("poolID", rec[hPool])
	assert.Equal("1MiB", rec[hParcelSizeBytes])

	// cache routines are singletons
	assert.NoError(cmd.loadDCNCaches())
	assert.NoError(cmd.cacheAccounts())

	// repeat without the maps
	cmd.accounts = map[string]ctxIDName{}
	cmd.cspDomains = map[string]string{}
	cmd.clusters = map[string]ctxIDName{}
	cmd.nodes = map[string]ctxIDName{}
	rec = cmd.makeRecord(o, now)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(storageRequestHeaders))
	assert.Equal("authAccountID", rec[hAccount])
	assert.Equal("cspDomainID", rec[hCspDomain])
	assert.Equal("clusterID", rec[hCluster])
	assert.Equal("nodeID", rec[hNode])
	assert.Equal("poolID", rec[hPool])
}

func TestStorageRequestList(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	res := &storage_request.StorageRequestListOK{
		Payload: []*models.StorageRequest{
			&models.StorageRequest{
				StorageRequestAllOf0: models.StorageRequestAllOf0{
					Meta: &models.ObjMeta{
						ID:           "fb213ad9-4b9b-44ca-adf0-968babf7bb2c",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				StorageRequestCreateOnce: models.StorageRequestCreateOnce{
					ClusterID:           "clusterID",
					CompleteByTime:      strfmt.DateTime(now.Add(time.Hour)),
					CspDomainID:         "CSP-DOMAIN-1",
					CspStorageType:      "Amazon gp2",
					MinSizeBytes:        swag.Int64(10737418240),
					PoolID:              "POOL-1",
					RequestedOperations: []string{"PROVISION", "ATTACH"},
				},
				StorageRequestMutable: models.StorageRequestMutable{
					StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
						StorageRequestState: "NEW",
					},
				},
			},
			&models.StorageRequest{
				StorageRequestAllOf0: models.StorageRequestAllOf0{
					Meta: &models.ObjMeta{
						ID:           "94e860ec-ac86-428c-95ff-3f8a38bbf2f1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				StorageRequestCreateOnce: models.StorageRequestCreateOnce{
					CompleteByTime:      strfmt.DateTime(now.Add(time.Hour)),
					CspDomainID:         "CSP-DOMAIN-1",
					PoolID:              "POOL-1",
					RequestedOperations: []string{"DETACH", "RELEASE"},
				},
				StorageRequestMutable: models.StorageRequestMutable{
					StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
						StorageRequestState: "NEW",
					},
					StorageRequestCreateMutable: models.StorageRequestCreateMutable{
						NodeID:    "nodeID-1",
						StorageID: "STORAGE-1",
					},
				},
			},
		},
	}

	// list, all defaults
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes, loadAccounts)
	params := storage_request.NewStorageRequestListParams()
	m := mockmgmtclient.NewStorageRequestMatcher(t, params)
	cOps := mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(cOps)
	cOps.EXPECT().StorageRequestList(m).Return(res, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err := parseAndRun([]string{"storage-request", "list"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(storageRequestDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list, json, most options
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
	params = storage_request.NewStorageRequestListParams()
	m = mockmgmtclient.NewStorageRequestMatcher(t, params)
	params.CspDomainID = swag.String("CSP-DOMAIN-1")
	params.ClusterID = swag.String("clusterID")
	params.NodeID = swag.String("nodeID-1")
	params.PoolID = swag.String("POOL-1")
	params.StorageRequestState = swag.String("NEW")
	params.IsTerminated = swag.Bool(false)
	params.StorageID = swag.String("STORAGE-1")
	params.VolumeSeriesRequestID = swag.String("VSR-1")
	cOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(cOps)
	cOps.EXPECT().StorageRequestList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-requests", "list", "-D", "domainName", "-C", "clusterName",
		"-N", "nodeName", "-P", "POOL-1", "--state", "NEW", "-S", "STORAGE-1", "--active",
		"--volume-series-request-id", "VSR-1",
		"-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(res.Payload, te.jsonData)

	// list, yaml, remaining args
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
	params = storage_request.NewStorageRequestListParams()
	m = mockmgmtclient.NewStorageRequestMatcher(t, params)
	params.CspDomainID = swag.String("CSP-DOMAIN-1")
	params.ClusterID = swag.String("clusterID")
	params.NodeID = swag.String("nodeID-1")
	params.PoolID = swag.String("POOL-1")
	params.StorageRequestState = swag.String("NEW")
	params.IsTerminated = swag.Bool(true)
	params.StorageID = swag.String("STORAGE-1")
	aTmGe := strfmt.DateTime(time.Now().Add(-10 * time.Minute))
	params.ActiveOrTimeModifiedGE = &aTmGe
	m.DActiveOrTimeModifiedGE = 10 * time.Minute
	cOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(cOps)
	cOps.EXPECT().StorageRequestList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "list", "-D", "domainName", "-C", "clusterName",
		"-N", "nodeName", "-P", "POOL-1", "--state", "NEW", "-S", "STORAGE-1", "--terminated", "-R", "10m",
		"-o", "yaml"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(res.Payload, te.yamlData)

	// list with follow
	fw := &fakeWatcher{t: t}
	appCtx.watcher = fw
	fw.CallCBCount = 1
	fos := &fakeOSExecutor{}
	appCtx.osExec = fos
	storageRequestListCmdRunCacheThreshold = 0 // force a cache refresh
	defer func() { storageRequestListCmdRunCacheThreshold = 1 }()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes, loadAccounts)
	params = storage_request.NewStorageRequestListParams()
	m = mockmgmtclient.NewStorageRequestMatcher(t, params)
	params.CspDomainID = swag.String("CSP-DOMAIN-1")
	params.ClusterID = swag.String("clusterID")
	params.NodeID = swag.String("nodeID-1")
	params.PoolID = swag.String("POOL-1")
	params.StorageRequestState = swag.String("NEW")
	params.IsTerminated = swag.Bool(true)
	params.StorageID = swag.String("STORAGE-1")
	cOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(cOps)
	cOps.EXPECT().StorageRequestList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "list", "-f", "-D", "domainName", "-C", "clusterName",
		"-N", "nodeName", "-P", "POOL-1", "--state", "NEW", "-S", "STORAGE-1", "--terminated"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(storageRequestDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))
	expWArgs := &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{
				URIPattern:   "/storage-requests/?",
				ScopePattern: ".*clusterId:clusterID.*nodeId:nodeID-1.*poolId:POOL-1.*storageRequestState:NEW",
			},
		},
	}
	assert.NotNil(fw.InArgs)
	assert.Equal(expWArgs, fw.InArgs)
	assert.NotNil(fw.InCB)
	assert.Equal(1, fw.NumCalls)
	assert.NoError(fw.CBErr)
	// test the idle callback (still active)
	cmd := &storageRequestListCmd{}
	cmd.tableCols = storageRequestDefaultHeaders
	testutils.Clone(res.Payload, &cmd.res)
	assert.Len(cmd.res, 2)
	cmd.res[0].StorageRequestState = "FAILED"
	cmd.res[1].StorageRequestState = "SUCCEEDED"
	t.Log("** idle callback 1")
	t.Log(cmd.res[0])
	t.Log(cmd.res[1])
	cmd.activeReq = true
	cmd.clusters = make(map[string]ctxIDName)
	cmd.nodes = make(map[string]ctxIDName)
	cmd.cspDomains = make(map[string]string)
	te = &TestEmitter{}
	appCtx.Emitter = te
	err = cmd.NoChangeTick()
	assert.NoError(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(storageRequestDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))
	// test the idle callback (completed)
	cmd.activeReq = false
	t.Log("** idle callback 2")
	t.Log(cmd.res[0])
	t.Log(cmd.res[1])
	te = &TestEmitter{}
	appCtx.Emitter = te
	err = cmd.NoChangeTick()
	assert.NoError(err)
	assert.Len(te.tableHeaders, 0)
	assert.Len(te.tableData, 0)
	assert.False(cmd.activeReq)

	// list with columns, --node-id success case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes, loadAccounts)
	params = storage_request.NewStorageRequestListParams()
	params.NodeID = swag.String("nodeId")
	m = mockmgmtclient.NewStorageRequestMatcher(t, params)
	cOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(cOps)
	cOps.EXPECT().StorageRequestList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "list", "--columns", "ID,Operations", "--node-id", "nodeId"})
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
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "list", "--columns", "Name,Foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// list, API failure model error
	apiErr := &storage_request.StorageRequestListDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
	params = storage_request.NewStorageRequestListParams()
	m = mockmgmtclient.NewStorageRequestMatcher(t, params)
	cOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(cOps)
	cOps.EXPECT().StorageRequestList(m).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "list"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// list, API failure arbitrary error
	otherErr := fmt.Errorf("OTHER ERROR")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
	params = storage_request.NewStorageRequestListParams()
	m = mockmgmtclient.NewStorageRequestMatcher(t, params)
	cOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(cOps)
	cOps.EXPECT().StorageRequestList(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "list"})
	assert.NotNil(err)
	assert.Equal(otherErr.Error(), err.Error())

	// cspdomain list error
	cspDomainListErr := fmt.Errorf("cspDomainListError")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dm := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	dOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	dOps.EXPECT().CspDomainList(dm).Return(nil, cspDomainListErr).MinTimes(1)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "list", "-D", "domain"})
	assert.NotNil(err)
	assert.Equal(cspDomainListErr.Error(), err.Error())

	// cluster list error
	clusterListErr := fmt.Errorf("clusterListErr")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains)
	cm := mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(cm).Return(nil, clusterListErr).MinTimes(1)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "list", "-D", "domain"})
	assert.NotNil(err)
	assert.Equal(clusterListErr.Error(), err.Error())

	// node list error
	nodeListErr := fmt.Errorf("nodeListErr")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters)
	nm := mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps := mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(nil, nodeListErr).MinTimes(1)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "list", "-D", "domain"})
	assert.NotNil(err)
	assert.Equal(nodeListErr.Error(), err.Error())

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
	initStorageRequest()
	err = parseAndRun([]string{"sr", "list", "-A", "System"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// not found cases
	notFoundTCs := []struct {
		args []string
		pat  string
	}{
		{
			args: []string{"sr", "list", "-D", "foo"},
			pat:  "csp domain.*not found",
		},
		{
			args: []string{"sr", "list", "-D", "domainName", "-C", "foo"},
			pat:  "cluster.*not found",
		},
		{
			args: []string{"sr", "list", "-D", "domainName", "-C", "clusterName", "-N", "foo"},
			pat:  "node.*not found",
		},
	}
	for _, tc := range notFoundTCs {
		t.Logf("tc: %s", tc.pat)
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
		nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
		mAPI.EXPECT().Node().Return(nOps)
		nOps.EXPECT().NodeList(nm).Return(resNodes, nil).MinTimes(1)
		cm = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
		clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
		mAPI.EXPECT().Cluster().Return(clOps)
		clOps.EXPECT().ClusterList(cm).Return(resClusters, nil).MinTimes(1)
		dm = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
		dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
		mAPI.EXPECT().CspDomain().Return(dOps)
		dOps.EXPECT().CspDomainList(dm).Return(resCspDomains, nil).MinTimes(1)
		appCtx.API = mAPI
		te = &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initStorageRequest()
		err = parseAndRun(tc.args)
		assert.NotNil(err)
		assert.Regexp(tc.pat, err.Error())
	}

	// invalid flag combinations
	invalidFlagTCs := []struct {
		args []string
		pat  string
	}{
		{
			args: []string{"sr", "list", "--terminated", "--active"},
			pat:  "terminated.*active",
		},
		{
			args: []string{"sr", "list", "-N", "nodeName"},
			pat:  "node-name requires cluster-name and domain",
		},
		{
			args: []string{"sr", "list", "-N", "nodeName", "-C", "clusterName"},
			pat:  "cluster-name requires domain",
		},
		{
			args: []string{"sr", "list", "-N", "nodeName", "--node-id", "nodeId"},
			pat:  "node-name.*node-id",
		},
	}
	for _, tc := range invalidFlagTCs {
		t.Logf("tc: %s", tc.pat)
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
		appCtx.API = mAPI
		te = &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initStorageRequest()
		err = parseAndRun(tc.args)
		assert.NotNil(err)
		assert.Regexp(tc.pat, err.Error())
	}

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "list", "columns", "ID,Operations"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestStorageRequestCreate(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	params := storage_request.NewStorageRequestCreateParams()
	params.Payload = &models.StorageRequestCreateArgs{
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			CompleteByTime:      strfmt.DateTime(now),
			MinSizeBytes:        swag.Int64(21474836480),
			ParcelSizeBytes:     swag.Int64(1048576),
			PoolID:              "POOL-1",
			RequestedOperations: []string{"PROVISION", "ATTACH", "FORMAT", "USE"},
		},
		StorageRequestCreateMutable: models.StorageRequestCreateMutable{
			NodeID:    "nodeID-1",
			StorageID: "STORAGE-1",
		},
	}
	res := &storage_request.StorageRequestCreateCreated{
		Payload: &models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID:           "id",
					Version:      1,
					TimeCreated:  strfmt.DateTime(now),
					TimeModified: strfmt.DateTime(now),
				},
			},
			StorageRequestCreateOnce: params.Payload.StorageRequestCreateOnce,
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "NEW",
				},
				StorageRequestCreateMutable: params.Payload.StorageRequestCreateMutable,
			},
		},
	}

	// success, all operations
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	nm := mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps := mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil).MinTimes(1)
	cm := mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(cm).Return(resClusters, nil).MinTimes(1)
	dm := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	dOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	dOps.EXPECT().CspDomainList(dm).Return(resCspDomains, nil).MinTimes(1)
	m := mockmgmtclient.NewStorageRequestMatcher(t, params)
	m.D = time.Hour
	cOps := mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(cOps)
	cOps.EXPECT().StorageRequestCreate(m).Return(res, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err := parseAndRun([]string{"storage-request", "create", "-O", "PROVISION", "-O", "ATTACH", "-O", "FORMAT", "-O", "USE",
		"-D", "domainName", "-b", "20GiB", "-p", "1MiB", "-P", "POOL-1",
		"-C", "clusterName", "-N", "nodeName", "-S", "STORAGE-1", "-x", "1h",
		"-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.StorageRequest{res.Payload}, te.jsonData)

	// create, invalid columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "create", "-O", "PROVISION", "-O", "ATTACH",
		"-D", "domainName", "-P", "POOL-1", "-b", "20GiB",
		"-C", "clusterName", "-N", "nodeName", "-S", "STORAGE-1",
		"-o", "json", "--columns", "Name,Foo"})

	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// REATTACH create, API model error
	apiErr := &storage_request.StorageRequestCreateDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil).MinTimes(1)
	cm = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(cm).Return(resClusters, nil).MinTimes(1)
	dm = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	dOps.EXPECT().CspDomainList(dm).Return(resCspDomains, nil).MinTimes(1)
	params.Payload.RequestedOperations = []string{"REATTACH"}
	params.Payload.ParcelSizeBytes = nil
	params.Payload.NodeID = ""
	params.Payload.ReattachNodeID = models.ObjIDMutable(resNodes.Payload[0].Meta.ID)
	cOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(cOps)
	cOps.EXPECT().StorageRequestCreate(m).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "create", "-O", "REATTACH",
		"-D", "domainName", "-P", "POOL-1", "-b", "20GiB",
		"-C", "clusterName", "-N", "nodeName", "-S", "STORAGE-1",
		"-o", "json"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// create, add system tags, API failure arbitrary error
	otherErr := fmt.Errorf("OTHER ERROR")
	params.Payload.SystemTags = []string{"tag:val", "tag2:val2"}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil).MinTimes(1)
	cm = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(cm).Return(resClusters, nil).MinTimes(1)
	dm = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	dOps.EXPECT().CspDomainList(dm).Return(resCspDomains, nil).MinTimes(1)
	cOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(cOps)
	cOps.EXPECT().StorageRequestCreate(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "create", "-O", "REATTACH",
		"-D", "domainName", "-P", "POOL-1", "-b", "20GiB",
		"-C", "clusterName", "-N", "nodeName", "-S", "STORAGE-1", "--system-tag=tag:val", "--system-tag=tag2:val2",
		"-o", "json"})
	assert.NotNil(err)
	assert.Equal(otherErr.Error(), err.Error())

	// csp domain list failed (only 1 failure case required as others covered in list)
	cspDomainListErr := fmt.Errorf("cspDomainListErr")
	mockCtrl.Finish()
	params.Payload.SystemTags = nil
	params.Payload.ReattachNodeID = ""
	params.Payload.NodeID = models.ObjIDMutable(resNodes.Payload[0].Meta.ID)
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dm = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	dOps.EXPECT().CspDomainList(dm).Return(nil, cspDomainListErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "create", "-O", "PROVISION", "-O", "ATTACH",
		"-D", "domainName", "-P", "POOL-1", "-b", "20GiB",
		"-C", "clusterName", "-N", "nodeName", "-S", "STORAGE-1",
		"-o", "json"})
	assert.NotNil(err)
	assert.Regexp(cspDomainListErr.Error(), err.Error())

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
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "create", "-O", "PROVISION", "-A", "System"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// min size bytes parsing error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "create", "-O", "PROVISION", "-O", "ATTACH",
		"-D", "domainName", "-P", "POOL-1", "-b", "20QoB",
		"-C", "clusterName", "-N", "nodeName", "-S", "STORAGE-1",
		"-o", "json"})
	assert.NotNil(err)
	assert.Regexp("unknown unit", err.Error())

	// parcel size bytes parsing error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "create", "-O", "PROVISION", "-O", "ATTACH", "-O", "FORMAT",
		"-D", "domainName", "-P", "POOL-1", "-p", "20QoB",
		"-C", "clusterName", "-N", "nodeName", "-S", "STORAGE-1",
		"-o", "json"})
	assert.NotNil(err)
	assert.Regexp("unknown unit", err.Error())

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "create", "-O", "PROVISION", "Foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestStorageRequestDelete(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	reqToDelete := "SR-1"
	dParams := storage_request.NewStorageRequestDeleteParams()
	dParams.ID = reqToDelete
	dRet := &storage_request.StorageRequestDeleteNoContent{}

	// delete
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(cOps).MinTimes(1)
	cOps.EXPECT().StorageRequestDelete(dParams).Return(dRet, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err := parseAndRun([]string{"storage-request", "delete", reqToDelete, "--confirm"})
	assert.Nil(err)

	// delete, --confirm not specified
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "delete", "--id", reqToDelete})
	assert.NotNil(err)
	assert.Regexp("--confirm", err.Error())

	// delete, API model error
	apiErr := &storage_request.StorageRequestDeleteDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(cOps).MinTimes(1)
	cOps.EXPECT().StorageRequestDelete(dParams).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "delete", "--id", reqToDelete, "--confirm"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// delete, API arbitrary error
	otherErr := fmt.Errorf("OTHER ERROR")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(cOps).MinTimes(1)
	cOps.EXPECT().StorageRequestDelete(dParams).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "delete", "--id", reqToDelete, "--confirm"})
	assert.NotNil(err)
	assert.Regexp(otherErr.Error(), err.Error())

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
	initStorageRequest()
	err = parseAndRun([]string{"sr", "delete", "-A", "System", "--id", reqToDelete, "--confirm"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("expected id")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "delete"})
	assert.NotNil(err)
	assert.Regexp("expected --id flag or a single identifier argument", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestStorageRequestGet(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	reqToFetch := "SR-1"
	fParams := storage_request.NewStorageRequestFetchParams()
	fParams.ID = reqToFetch

	now := time.Now()
	fRet := &storage_request.StorageRequestFetchOK{
		Payload: &models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID:           "fb213ad9-4b9b-44ca-adf0-968babf7bb2c",
					Version:      1,
					TimeCreated:  strfmt.DateTime(now),
					TimeModified: strfmt.DateTime(now),
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				ClusterID:           "clusterID",
				CompleteByTime:      strfmt.DateTime(now.Add(time.Hour)),
				CspDomainID:         "CSP-DOMAIN-1",
				CspStorageType:      "Amazon gp2",
				MinSizeBytes:        swag.Int64(10737418240),
				PoolID:              "POOL-1",
				RequestedOperations: []string{"PROVISION", "ATTACH"},
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "NEW",
				},
				StorageRequestCreateMutable: models.StorageRequestCreateMutable{
					NodeID: "nodeID-1",
				},
			},
		},
	}

	// fetch
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes, loadAccounts)
	cOps := mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(cOps).MinTimes(1)
	mF := mockmgmtclient.NewStorageRequestMatcher(t, fParams)
	cOps.EXPECT().StorageRequestFetch(mF).Return(fRet, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err := parseAndRun([]string{"storage-request", "get", reqToFetch})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(storageRequestDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, 1)

	// json
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(cOps).MinTimes(1)
	mF = mockmgmtclient.NewStorageRequestMatcher(t, fParams)
	cOps.EXPECT().StorageRequestFetch(mF).Return(fRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "get", "--id", reqToFetch, "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.StorageRequest{fRet.Payload}, te.jsonData)

	// apiError
	apiErr := &storage_request.StorageRequestFetchDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(cOps).MinTimes(1)
	mF = mockmgmtclient.NewStorageRequestMatcher(t, fParams)
	cOps.EXPECT().StorageRequestFetch(mF).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "get", "--id", reqToFetch, "-o", "json"})
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
	initStorageRequest()
	err = parseAndRun([]string{"sr", "get", "-A", "System", "--id", reqToFetch})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// invalid columns
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "get", "--id", reqToFetch, "--columns", "foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	t.Log("expected id")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageRequest()
	err = parseAndRun([]string{"storage-request", "get"})
	assert.NotNil(err)
	assert.Regexp("expected --id flag or a single identifier argument", err)
	appCtx.Account, appCtx.AccountID = "", ""
}
