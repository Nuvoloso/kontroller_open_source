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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestStorageMakeRecord(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	cmd := &storageCmd{}
	cmd.accounts = map[string]ctxIDName{
		"accountID": {"", "System"},
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
	o := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta: &models.ObjMeta{
				ID:           "id",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
			AccountID:      "accountID",
			CspDomainID:    "cspDomainID",
			CspStorageType: "Amazon gp2",
			SizeBytes:      swag.Int64(107374182400),
			StorageAccessibility: &models.StorageAccessibility{
				StorageAccessibilityMutable: models.StorageAccessibilityMutable{
					AccessibilityScope:      "CSPDOMAIN",
					AccessibilityScopeObjID: "87956a70-4108-4a9a-8ca9-e391505db88c",
				},
			},
			PoolID:    "cspPoolID",
			ClusterID: "clusterID",
		},
		StorageMutable: models.StorageMutable{
			AvailableBytes:    swag.Int64(10737418240),
			ParcelSizeBytes:   swag.Int64(1024),
			TotalParcelCount:  swag.Int64(987654321),
			StorageIdentifier: "vol:volume-2",
			StorageState: &models.StorageStateMutable{
				AttachedNodeDevice: "/dev/xvb0",
				AttachedNodeID:     "nodeID",
				AttachmentState:    "DETACHED",
				DeviceState:        "UNUSED",
				MediaState:         "FORMATTED",
				ProvisionedState:   "PROVISIONED",
			},
		},
	}
	rec := cmd.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(storageHeaders))
	assert.Equal("CSPDOMAIN", rec[hAccessibilityScope])
	assert.Equal("87956a70-4108-4a9a-8ca9-e391505db88c", rec[hAccessibilityScopeID])
	assert.Equal("System", rec[hAccount])
	assert.Equal("MyCluster", rec[hAttachedCluster])
	assert.Equal("node1", rec[hAttachedNode])
	assert.Equal("DETACHED", rec[hAttachmentState])
	assert.Equal("10GiB", rec[hAvailableBytes])
	assert.Equal("Amazon gp2", rec[hCSPStorageType])
	assert.Equal("MyAWS", rec[hCspDomain])
	assert.Equal("UNUSED", rec[hDeviceState])
	assert.Equal("id", rec[hID])
	assert.Equal("FORMATTED", rec[hMediaState])
	assert.Equal("/dev/xvb0", rec[hNodeDevice])
	assert.Equal("1KiB", rec[hParcelSizeBytes])
	assert.Equal("987654321", rec[hTotalParcelCount])
	assert.Equal("PROVISIONED", rec[hProvisionedState])
	assert.Equal("100GiB", rec[hSizeBytes])
	assert.Equal("cspPoolID", rec[hPool])
	assert.Equal("vol:volume-2", rec[hStorageIdentifier])

	// cache routines are singletons
	assert.NoError(cmd.loadDCNCaches())

	// repeat without the maps
	cmd.accounts = map[string]ctxIDName{}
	cmd.cspDomains = map[string]string{}
	cmd.clusters = map[string]ctxIDName{}
	cmd.nodes = map[string]ctxIDName{}
	rec = cmd.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(storageHeaders))
	assert.Equal("accountID", rec[hAccount])
	assert.Equal("cspDomainID", rec[hCspDomain])
	assert.Equal("clusterID", rec[hAttachedCluster])
	assert.Equal("nodeID", rec[hAttachedNode])
	assert.Equal("cspPoolID", rec[hPool])
}

func TestStorageList(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	// validate choice options
	initParser()
	initStorage()
	cmdStg := parser.Find("storage")
	assert.NotNil(cmdStg)
	cmdStgList := cmdStg.Find("list")
	assert.NotNil(cmdStgList)
	choiceTCs := []struct {
		longName string
		values   []string
	}{
		{"scope", []string{"NODE", "CSPDOMAIN"}},
		{"provisioned-state", []string{"UNPROVISIONED", "PROVISIONING", "PROVISIONED", "UNPROVISIONING", "ERROR"}},
		{"attachment-state", []string{"DETACHED", "ATTACHED", "DETACHING", "ATTACHING", "ERROR"}},
		{"device-state", []string{"UNUSED", "FORMATTING", "OPENING", "OPEN", "CLOSING", "ERROR"}},
		{"media-state", []string{"UNFORMATTED", "FORMATTED"}},
	}
	for _, tc := range choiceTCs {
		opt := cmdStgList.FindOptionByLongName(tc.longName)
		assert.NotNil(opt)
		assert.Equal(tc.values, opt.Choices)
	}

	now := time.Now()
	res := &storage.StorageListOK{
		Payload: []*models.Storage{
			&models.Storage{
				StorageAllOf0: models.StorageAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
					AccountID:      "authAccountID",
					CspDomainID:    "CSP-DOMAIN-1",
					CspStorageType: "Amazon gp2",
					SizeBytes:      swag.Int64(107374182400),
					StorageAccessibility: &models.StorageAccessibility{
						StorageAccessibilityMutable: models.StorageAccessibilityMutable{
							AccessibilityScope:      "CSPDOMAIN",
							AccessibilityScopeObjID: "CSP-DOMAIN-1",
						},
					},
					PoolID:    "poolID-1",
					ClusterID: "clusterID",
				},
				StorageMutable: models.StorageMutable{
					AvailableBytes:    swag.Int64(10737418240),
					ParcelSizeBytes:   swag.Int64(0),
					StorageIdentifier: "vol:volume-1",
					StorageState: &models.StorageStateMutable{
						AttachedNodeDevice: "/dev/xvb0",
						AttachedNodeID:     "nodeID-1",
						AttachmentState:    "DETACHED",
						DeviceState:        "UNUSED",
						MediaState:         "UNFORMATTED",
						ProvisionedState:   "PROVISIONED",
					},
				},
			},
			&models.Storage{
				StorageAllOf0: models.StorageAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id2",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
					AccountID:      "authAccountID",
					CspDomainID:    "CSP-DOMAIN-1",
					CspStorageType: "Amazon gp2",
					SizeBytes:      swag.Int64(107374182400),
					StorageAccessibility: &models.StorageAccessibility{
						StorageAccessibilityMutable: models.StorageAccessibilityMutable{
							AccessibilityScope:      "CSPDOMAIN",
							AccessibilityScopeObjID: "CSP-DOMAIN-1",
						},
					},
					PoolID: "poolID-1",
				},
				StorageMutable: models.StorageMutable{
					AvailableBytes:    swag.Int64(10737418240),
					StorageIdentifier: "vol:volume-2",
					StorageState: &models.StorageStateMutable{
						AttachedNodeDevice: "/dev/xvb1",
						AttachedNodeID:     "nodeID-1",
						AttachmentState:    "DETACHED",
						ProvisionedState:   "PROVISIONED",
					},
				},
			},
		},
	}

	// list, all defaults
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes, loadAccounts)
	params := storage.NewStorageListParams()
	m := mockmgmtclient.NewStorageMatcher(t, params)
	cOps := mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(cOps)
	cOps.EXPECT().StorageList(m).Return(res, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorage()
	err := parseAndRun([]string{"storage", "list"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(storageDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list, json, most options
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
	params = storage.NewStorageListParams()
	m = mockmgmtclient.NewStorageMatcher(t, params)
	params.CspDomainID = swag.String("CSP-DOMAIN-1")
	params.ClusterID = swag.String("clusterID")
	params.AttachedNodeID = swag.String("nodeID-1")
	params.PoolID = swag.String("poolID-1")
	params.AccessibilityScope = swag.String(asCSPDOMAIN)
	params.AccessibilityScopeObjID = swag.String("scopeObjID")
	params.CspStorageType = swag.String("Amazon gp2")
	params.AvailableBytesGE = swag.Int64(1000)
	params.AvailableBytesLT = swag.Int64(10000)
	params.ProvisionedState = swag.String("PROVISIONED")
	params.AttachmentState = swag.String("DETACHED")
	params.DeviceState = swag.String("OPEN")
	params.MediaState = swag.String("UNFORMATTED")
	params.IsProvisioned = swag.Bool(true)
	cOps = mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(cOps)
	cOps.EXPECT().StorageList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorage()
	err = parseAndRun([]string{"storage", "list", "-D", "domainName", "-C", "clusterName",
		"-N", "nodeName", "-P", "poolID-1", "-T", "Amazon gp2", "--ge", "1000B", "--lt", "10000B",
		"-a", "CSPDOMAIN", "--scope-object-id", "scopeObjID",
		"--provisioned-state", "PROVISIONED", "--is-provisioned",
		"--attachment-state", "DETACHED", "--device-state", "OPEN", "--media-state", "UNFORMATTED",
		"-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(res.Payload, te.jsonData)

	// list, yaml
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
	params = storage.NewStorageListParams()
	m = mockmgmtclient.NewStorageMatcher(t, params)
	params.IsProvisioned = swag.Bool(false)
	cOps = mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(cOps)
	cOps.EXPECT().StorageList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorage()
	err = parseAndRun([]string{"storage", "list", "--not-provisioned", "-o", "yaml"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(res.Payload, te.yamlData)

	// list with follow
	fw := &fakeWatcher{t: t}
	appCtx.watcher = fw
	fw.CallCBCount = 1
	fos := &fakeOSExecutor{}
	appCtx.osExec = fos
	storageListCmdRunCacheThreshold = 0 // force a cache refresh
	defer func() { storageListCmdRunCacheThreshold = 1 }()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes, loadAccounts)
	params = storage.NewStorageListParams()
	m = mockmgmtclient.NewStorageMatcher(t, params)
	cOps = mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(cOps)
	cOps.EXPECT().StorageList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorage()
	err = parseAndRun([]string{"storage", "list", "-f"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(storageDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))
	expWArgs := &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{
				URIPattern: "/storage/?",
			},
		},
	}
	assert.NotNil(fw.InArgs)
	assert.Equal(expWArgs, fw.InArgs)
	assert.NotNil(fw.InCB)
	assert.Equal(1, fw.NumCalls)
	assert.NoError(fw.CBErr)

	// list with columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes, loadAccounts)
	params = storage.NewStorageListParams()
	m = mockmgmtclient.NewStorageMatcher(t, params)
	cOps = mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(cOps)
	cOps.EXPECT().StorageList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorage()
	err = parseAndRun([]string{"storage", "list", "--columns", "ID,StorageType,Account"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, 3)
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list with aggregation
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
	params = storage.NewStorageListParams()
	agParams := storage.NewStorageListParams()
	m = mockmgmtclient.NewStorageMatcher(t, agParams)
	agParams.Sum = []string{"sizeBytes", "availableBytes"}
	resAg := storage.NewStorageListOK()
	resAg.Payload = nil
	resAg.TotalCount = 20
	resAg.Aggregations = []string{"sizeBytes:sum:10", "availableBytes:sum:10"}
	cOps = mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(cOps)
	cOps.EXPECT().StorageList(m).Return(resAg, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorage()
	err = parseAndRun([]string{"storage", "list", "--sum", "Size", "--sum", "Avail", "-o", "json"})
	assert.Nil(err)
	expData := agResult{
		NumberObjects: 20,
		Aggregations: []agResultField{
			agResultField{"sizeBytes", "sum", "10"},
			agResultField{"availableBytes", "sum", "10"},
		},
	}
	assert.NotNil(te.jsonData)
	assert.NotNil(expData)
	assert.EqualValues(expData, te.jsonData)

	// list with invalid columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorage()
	err = parseAndRun([]string{"storage", "list", "--columns", "ID,Foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// list sum with invalid fields
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorage()
	err = parseAndRun([]string{"storage", "list", "--sum", "Name"})
	assert.NotNil(err)
	assert.Regexp("Unsupported field for sum", err)

	// list sum with columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorage()
	err = parseAndRun([]string{"storage", "list", "--sum", "Size", "--columns", "ID"})
	assert.NotNil(err)
	assert.Regexp("do not specify --columns when using --sum", err)

	// list is-provisioned and not-provisioned
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorage()
	err = parseAndRun([]string{"storage", "list", "--is-provisioned", "--not-provisioned"})
	assert.NotNil(err)
	assert.Regexp("do not specify both --is-provisioned and --not-provisioned", err)

	// list, API failure model error
	apiErr := &storage.StorageListDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
	params = storage.NewStorageListParams()
	m = mockmgmtclient.NewStorageMatcher(t, params)
	cOps = mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(cOps)
	cOps.EXPECT().StorageList(m).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorage()
	err = parseAndRun([]string{"storage", "list"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// list, API failure arbitrary error
	otherErr := fmt.Errorf("OTHER ERROR")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadContext, loadDomains, loadClusters, loadNodes)
	params = storage.NewStorageListParams()
	m = mockmgmtclient.NewStorageMatcher(t, params)
	cOps = mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(cOps)
	cOps.EXPECT().StorageList(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorage()
	err = parseAndRun([]string{"storage", "list", "-A", "System"})
	assert.NotNil(err)
	assert.Equal(otherErr.Error(), err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// validateDomainClusterNodeNames domain list error (fully covered elsewhere)
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
	initStorage()
	err = parseAndRun([]string{"storage", "list", "-D", "domain"})
	assert.NotNil(err)
	assert.Equal(cspDomainListErr.Error(), err.Error())

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
	initStorage()
	err = parseAndRun([]string{"storage", "list", "-A", "System"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// not found cases
	notFoundTCs := []struct {
		args []string
		pat  string
	}{
		{
			args: []string{"storage", "list", "-D", "foo"},
			pat:  "csp domain.*not found",
		},
		{
			args: []string{"storage", "list", "-D", "domainName", "-C", "foo"},
			pat:  "cluster.*not found",
		},
		{
			args: []string{"storage", "list", "-D", "domainName", "-C", "clusterName", "-N", "foo"},
			pat:  "node.*not found",
		},
	}
	for _, tc := range notFoundTCs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
		appCtx.API = mAPI
		te = &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initStorage()
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
			args: []string{"storage", "list", "--ge", "1000"},
			pat:  "unknown unit",
		},
		{
			args: []string{"storage", "list", "--lt", "10000GoB"},
			pat:  "unknown unit",
		},
		{
			args: []string{"storage", "list", "-N", "nodeName"},
			pat:  "node-name requires cluster-name and domain",
		},
		{
			args: []string{"storage", "list", "-N", "nodeName", "-C", "clusterName"},
			pat:  "cluster-name requires domain",
		},
	}
	for _, tc := range invalidFlagTCs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
		appCtx.API = mAPI
		te = &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initStorage()
		err = parseAndRun(tc.args)
		assert.NotNil(err)
		assert.Regexp(tc.pat, err.Error())
	}

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorage()
	err = parseAndRun([]string{"storage", "list", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestStorageDelete(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	storageToDelete := "STORAGE-1"
	dParams := storage.NewStorageDeleteParams()
	dParams.ID = storageToDelete
	dRet := &storage.StorageDeleteNoContent{}

	// delete
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(cOps).MinTimes(1)
	cOps.EXPECT().StorageDelete(dParams).Return(dRet, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorage()
	err := parseAndRun([]string{"storage", "delete", storageToDelete, "--confirm"})
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
	initStorage()
	err = parseAndRun([]string{"storage", "delete", "--id", storageToDelete})
	assert.NotNil(err)
	assert.Regexp("--confirm", err.Error())

	// delete, API model error
	apiErr := &storage.StorageDeleteDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(cOps).MinTimes(1)
	cOps.EXPECT().StorageDelete(dParams).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorage()
	err = parseAndRun([]string{"storage", "delete", "--id", storageToDelete, "--confirm"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// delete, API arbitrary error
	otherErr := fmt.Errorf("OTHER ERROR")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(cOps).MinTimes(1)
	cOps.EXPECT().StorageDelete(dParams).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorage()
	err = parseAndRun([]string{"storage", "delete", "--id", storageToDelete, "--confirm"})
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
	initStorage()
	err = parseAndRun([]string{"storage", "delete", "-A", "System", "--id", storageToDelete, "--confirm"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("expected id")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorage()
	err = parseAndRun([]string{"storage", "delete", "--confirm"})
	assert.NotNil(err)
	assert.Regexp("expected --id flag or a single identifier argument", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestStorageGet(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	reqToFetch := "id1"
	fParams := storage.NewStorageFetchParams()
	fParams.ID = reqToFetch

	now := time.Now()
	fRet := &storage.StorageFetchOK{
		Payload: &models.Storage{
			StorageAllOf0: models.StorageAllOf0{
				Meta: &models.ObjMeta{
					ID:           "id1",
					Version:      1,
					TimeCreated:  strfmt.DateTime(now),
					TimeModified: strfmt.DateTime(now),
				},
				AccountID:      "authAccountID",
				CspDomainID:    "CSP-DOMAIN-1",
				CspStorageType: "Amazon gp2",
				SizeBytes:      swag.Int64(107374182400),
				StorageAccessibility: &models.StorageAccessibility{
					StorageAccessibilityMutable: models.StorageAccessibilityMutable{
						AccessibilityScope:      "CSPDOMAIN",
						AccessibilityScopeObjID: "CSP-DOMAIN-1",
					},
				},
				PoolID:    "poolID-1",
				ClusterID: "clusterID",
			},
			StorageMutable: models.StorageMutable{
				AvailableBytes:    swag.Int64(10737418240),
				ParcelSizeBytes:   swag.Int64(0),
				StorageIdentifier: "vol:volume-1",
				StorageState: &models.StorageStateMutable{
					AttachedNodeDevice: "/dev/xvb0",
					AttachedNodeID:     "nodeID-1",
					AttachmentState:    "DETACHED",
					DeviceState:        "UNUSED",
					MediaState:         "UNFORMATTED",
					ProvisionedState:   "PROVISIONED",
				},
			},
		},
	}

	// fetch
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes, loadAccounts)
	cOps := mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(cOps).MinTimes(1)
	mF := mockmgmtclient.NewStorageMatcher(t, fParams)
	cOps.EXPECT().StorageFetch(mF).Return(fRet, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorage()
	err := parseAndRun([]string{"storage", "get", "--id", reqToFetch})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(storageDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, 1)

	// apiError
	apiErr := &storage.StorageFetchDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(cOps).MinTimes(1)
	mF = mockmgmtclient.NewStorageMatcher(t, fParams)
	cOps.EXPECT().StorageFetch(mF).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorage()
	err = parseAndRun([]string{"storage", "get", "--id", reqToFetch})
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
	initStorage()
	err = parseAndRun([]string{"storage", "get", "-A", "System", "--id", reqToFetch})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// invalid columns
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorage()
	err = parseAndRun([]string{"storage", "get", "--id", reqToFetch, "--columns", "foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	t.Log("expected id")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorage()
	err = parseAndRun([]string{"storage", "get"})
	assert.NotNil(err)
	assert.Regexp("expected --id flag or a single identifier argument", err)
	appCtx.Account, appCtx.AccountID = "", ""
}
