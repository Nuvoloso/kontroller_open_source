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
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/application_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/cluster"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/consistency_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/node"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan_allocation"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestVolumeSeriesRequestMakeRecord(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	cmd := &volumeSeriesRequestCmd{}
	cmd.accounts = map[string]ctxIDName{
		"accountID": {"", "System"},
	}
	cmd.cspDomains = map[string]string{
		"cspDomainID": "MyAWS",
	}
	cmd.clusters = map[string]ctxIDName{
		"clusterID": ctxIDName{id: "cspDomainID", name: "MyCluster"},
	}
	cmd.consistencyGroups = map[string]ctxIDName{
		"cgID": ctxIDName{id: "accountID", name: "CG"},
	}
	cmd.nodes = map[string]ctxIDName{
		"nodeID": ctxIDName{id: "clusterID", name: "node1"},
	}
	cmd.servicePlans = map[string]string{
		"planID": "plan B",
	}
	o := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:           "id",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now.Add(-1 * time.Minute)),
				TimeModified: strfmt.DateTime(now),
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			ClusterID:           "clusterID",
			CompleteByTime:      strfmt.DateTime(now),
			RequestedOperations: []string{"CREATE", "BIND", "MOUNT"},
			VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{
				VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
					AccountID: "System",
				},
				VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
					Name:               "MyVS-1",
					Description:        "new volume series",
					ConsistencyGroupID: "cgID",
					ServicePlanID:      "planID",
					SizeBytes:          swag.Int64(1073741824),
					Tags:               []string{"tag1:value1"},
				},
			},
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "SUCCEEDED",
				Progress: &models.Progress{
					PercentComplete: swag.Int32(35),
				},
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID:         "nodeID",
				VolumeSeriesID: "VS-1",
			},
		},
	}
	rec := cmd.makeRecord(o, now)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(volumeSeriesRequestHeaders))
	assert.Equal("id", rec[hID])
	assert.Equal(time.Time(now).Format(time.RFC3339), rec[hCompleteBy])
	assert.Equal(now.Add(-60*time.Second).Format(time.RFC3339), rec[hTimeCreated])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeModified])
	assert.Equal(time.Duration(60*time.Second).String(), rec[hTimeDuration])
	assert.Equal("CREATE, BIND, MOUNT", rec[hRequestedOperations])
	assert.Equal("SUCCEEDED", rec[hRequestState])
	assert.Equal("MyCluster", rec[hCluster])
	assert.Equal("node1", rec[hNode])
	assert.Equal("VS-1", rec[hVsID])
	assert.Equal("VS-1", rec[hObjectID])
	assert.Equal("", rec[hProgress]) // terminated
	assert.False(cmd.activeReq)

	// repeat with a CG operation
	o.RequestedOperations = []string{"CG_SNAPSHOT_CREATE"}
	o.VolumeSeriesRequestState = "ACTIVE"
	o.ConsistencyGroupID = "CG-1"
	rec = cmd.makeRecord(o, now)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Equal("CG-1", rec[hObjectID])
	assert.True(cmd.activeReq)
	assert.Equal("35%", rec[hProgress]) // not terminated

	// repeat with a ALLOCATE_CAPACITY operation
	o.RequestedOperations = []string{"ALLOCATE_CAPACITY"}
	o.VolumeSeriesRequestState = "ACTIVE"
	o.ServicePlanAllocationID = "SPA-1"
	o.Progress = nil
	rec = cmd.makeRecord(o, now)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Equal("SPA-1", rec[hObjectID])
	assert.True(cmd.activeReq)
	assert.Equal("", rec[hProgress]) // not available

	// cache routines are singletons
	err := cmd.loadDCNCaches()
	assert.Nil(err)

	// repeat without the maps
	cmd.cspDomains = map[string]string{}
	cmd.clusters = map[string]ctxIDName{}
	cmd.consistencyGroups = map[string]ctxIDName{}
	cmd.nodes = map[string]ctxIDName{}
	rec = cmd.makeRecord(o, now)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(volumeSeriesRequestHeaders))
	assert.Equal("clusterID", rec[hCluster])
	assert.Equal("nodeID", rec[hNode])
}

func TestVolumeSeriesRequestList(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	resVS := &volume_series.VolumeSeriesListOK{
		Payload: []*models.VolumeSeries{
			&models.VolumeSeries{
				VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
					Meta: &models.ObjMeta{
						ID:           "VS-1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
					RootParcelUUID: "uuid1",
				},
				VolumeSeriesMutable: models.VolumeSeriesMutable{
					VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
						Name: "MyVolSeries",
					},
				},
			},
		},
	}
	res := &volume_series_request.VolumeSeriesRequestListOK{
		Payload: []*models.VolumeSeriesRequest{
			&models.VolumeSeriesRequest{
				VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
					ClusterID:           "clusterID",
					CompleteByTime:      strfmt.DateTime(now.Add(time.Hour)),
					RequestedOperations: []string{"CREATE", "BIND", "MOUNT"},
					VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{
						VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
							AccountID: "System",
						},
						VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
							Name:               "MyVS-1",
							Description:        "new volume series",
							ConsistencyGroupID: "c-g-1",
							ServicePlanID:      "planID",
							SizeBytes:          swag.Int64(1073741824),
							Tags:               []string{"tag1:value1"},
						},
					},
				},
				VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
					VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
						VolumeSeriesRequestState: "NEW",
					},
					VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
						NodeID:         "nodeID",
						VolumeSeriesID: "VS-1",
					},
				},
			},
		},
	}

	// list, all defaults
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
	params := volume_series_request.NewVolumeSeriesRequestListParams()
	m := mockmgmtclient.NewVolumeSeriesRequestMatcher(t, params)
	cOps := mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	cOps.EXPECT().VolumeSeriesRequestList(m).Return(res, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err := parseAndRun([]string{"volume-series-request", "list"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(volumeSeriesRequestDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list, json, volume series, other options
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes, loadContext, loadProtectionDomains)
	mVS := mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesListParams())
	mVS.ListParam.AccountID = swag.String("accountID")
	mVS.ListParam.Name = swag.String(string(resVS.Payload[0].Name))
	vsOps := mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vsOps)
	vsOps.EXPECT().VolumeSeriesList(mVS).Return(resVS, nil)
	params = volume_series_request.NewVolumeSeriesRequestListParams()
	m = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, params)
	params.VolumeSeriesRequestState = []string{"BINDING"}
	params.ClusterID = swag.String("clusterID")
	params.NodeID = swag.String("nodeID-1")
	params.ProtectionDomainID = swag.String("PD-1")
	params.VolumeSeriesID = swag.String("VS-1")
	params.IsTerminated = swag.Bool(true)
	params.RequestedOperations = []string{"MOUNT", "ATTACH_FS"}
	params.RequestedOperationsNot = []string{"NODE_DELETE"}
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	cOps.EXPECT().VolumeSeriesRequestList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "list", "-D", "domainName", "-C", "clusterName",
		"-N", "nodeName", "--protection-domain", "PD1", "--state", "BINDING", "-n", "MyVolSeries", "-A", "System", "--terminated",
		"-O", "MOUNT", "-O", "ATTACH_FS", "-O", "!NODE_DELETE",
		"-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(res.Payload, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// list, yaml, remaining args
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
	params = volume_series_request.NewVolumeSeriesRequestListParams()
	m = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, params)
	params.VolumeSeriesRequestState = []string{"BINDING", "SIZING"}
	params.ClusterID = swag.String("clusterID")
	params.NodeID = swag.String("nodeID-1")
	params.PoolID = swag.String("pool-1")
	params.ProtectionDomainID = swag.String("PD-1")
	params.SnapshotID = swag.String("snapshotID")
	params.VolumeSeriesID = swag.String("VS-1")
	aTmGe := strfmt.DateTime(time.Now().Add(-10 * time.Minute))
	params.ActiveOrTimeModifiedGE = &aTmGe
	m.DActiveOrTimeModifiedGE = 10 * time.Minute
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	cOps.EXPECT().VolumeSeriesRequestList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "list", "-D", "domainName", "-C", "clusterName",
		"-N", "nodeName", "--protection-domain-id", "PD-1", "-s", "BINDING", "-s", "SIZING", "-R", "10m", "-V", "VS-1", "--pool-id", "pool-1", "-S", "snapshotID", "-o", "yaml"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(res.Payload, te.yamlData)

	// list with follow
	fw := &fakeWatcher{t: t}
	appCtx.watcher = fw
	fw.CallCBCount = 1
	fos := &fakeOSExecutor{}
	appCtx.osExec = fos
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
	params = volume_series_request.NewVolumeSeriesRequestListParams()
	m = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, params)
	params.ClusterID = swag.String("clusterID")
	params.NodeID = swag.String("nodeID-1")
	params.VolumeSeriesID = swag.String("VS-1")
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	cOps.EXPECT().VolumeSeriesRequestList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "list", "-f",
		"-D", "domainName", "-C", "clusterName", "-N", "nodeName", "-V", "VS-1"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(volumeSeriesRequestDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))
	expWArgs := &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{
				URIPattern:   "/volume-series-requests/?",
				ScopePattern: ".*clusterId:clusterID.*nodeId:nodeID-1.*volumeSeriesId:VS-1",
			},
		},
	}
	assert.NotNil(fw.InArgs)
	assert.Equal(expWArgs, fw.InArgs)
	assert.NotNil(fw.InCB)
	assert.Equal(1, fw.NumCalls)
	assert.NoError(fw.CBErr)
	// test the idle callback (still active)
	cmd := &volumeSeriesRequestListCmd{}
	cmd.tableCols = volumeSeriesRequestDefaultHeaders
	testutils.Clone(res.Payload, &cmd.res)
	assert.Len(cmd.res, 1)
	t.Log(cmd.res[0])
	cmd.activeReq = true
	cmd.clusters = make(map[string]ctxIDName)
	cmd.nodes = make(map[string]ctxIDName)
	te = &TestEmitter{}
	appCtx.Emitter = te
	err = cmd.NoChangeTick()
	assert.NoError(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(volumeSeriesRequestDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))
	// test the idle callback (completed)
	cmd.activeReq = false
	cmd.res[0].VolumeSeriesRequestState = "FAILED"
	te = &TestEmitter{}
	appCtx.Emitter = te
	err = cmd.NoChangeTick()
	assert.NoError(err)
	assert.Len(te.tableHeaders, 0)
	assert.Len(te.tableData, 0)
	assert.False(cmd.activeReq)

	// list with columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
	params = volume_series_request.NewVolumeSeriesRequestListParams()
	m = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, params)
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	cOps.EXPECT().VolumeSeriesRequestList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "list", "--columns", "ID,%,Operations"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, 3)
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
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "list", "--columns", "Name,Foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// list, API failure model error
	apiErr := &volume_series_request.VolumeSeriesRequestListDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
	params = volume_series_request.NewVolumeSeriesRequestListParams()
	m = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, params)
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	cOps.EXPECT().VolumeSeriesRequestList(m).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "list"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// list, API failure arbitrary error (covers use of --node-id)
	otherErr := fmt.Errorf("OTHER ERROR")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
	params = volume_series_request.NewVolumeSeriesRequestListParams()
	params.NodeID = swag.String("nodeId")
	m = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, params)
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	cOps.EXPECT().VolumeSeriesRequestList(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "list", "--node-id", "nodeId"})
	assert.NotNil(err)
	assert.Equal(otherErr.Error(), err.Error())

	// cspdomain list error (covers call to loadDCNCaches)
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
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "list", "-D", "domain"})
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
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"vsr", "list", "-A", "System"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// not found cases
	notFoundTCs := []struct {
		args        []string
		pat         string
		needAccount bool
		vsLP        *volume_series.VolumeSeriesListParams
		vsLPRet     *volume_series.VolumeSeriesListOK
		cacheItems  []loadCache
	}{
		{
			args: []string{"vsr", "list", "-D", "foo"},
			pat:  "csp domain.*not found",
		},
		{
			args: []string{"vsr", "list", "-D", "domainName", "-C", "foo"},
			pat:  "cluster.*not found",
		},
		{
			args: []string{"vsr", "list", "-D", "domainName", "-C", "clusterName", "-N", "foo"},
			pat:  "node.*not found",
		},
		{
			args:        []string{"vsr", "list", "-n", "vsName", "-A", "System"},
			pat:         "volume series.*not found",
			needAccount: true,
			vsLP: &volume_series.VolumeSeriesListParams{ // warning timeout cannot be set
				AccountID: swag.String("accountID"),
				Name:      swag.String("vsName"),
			},
			vsLPRet: &volume_series.VolumeSeriesListOK{
				Payload: []*models.VolumeSeries{},
			},
		},
		{
			args:       []string{"vsr", "list", "--protection-domain", "foo"},
			pat:        "protection domain.*not found",
			cacheItems: []loadCache{loadDomains, loadClusters, loadNodes, loadProtectionDomains},
		},
	}
	for _, tc := range notFoundTCs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		if tc.cacheItems == nil {
			mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
		} else {
			mAPI = cacheLoadHelper(t, mockCtrl, tc.cacheItems...)
		}
		if tc.needAccount {
			authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
			mAPI.EXPECT().Authentication().Return(authOps)
			authOps.EXPECT().SetContextAccount("", "System").Return("accountID", nil)
		}
		if tc.vsLP != nil {
			mVS := mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesListParams())
			mVS.ListParam.AccountID = tc.vsLP.AccountID // assemble props because of internal timeout
			mVS.ListParam.Name = tc.vsLP.Name
			vsOps := mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
			mAPI.EXPECT().VolumeSeries().Return(vsOps)
			vsOps.EXPECT().VolumeSeriesList(mVS).Return(tc.vsLPRet, nil)
		}
		appCtx.API = mAPI
		te = &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initVolumeSeriesRequest()
		err = parseAndRun(tc.args)
		assert.NotNil(err)
		assert.Regexp(tc.pat, err.Error())
		if tc.needAccount {
			appCtx.Account, appCtx.AccountID = "", ""
		}
	}

	// invalid flag combinations
	invalidFlagTCs := []struct {
		args []string
		pat  string
	}{
		{
			args: []string{"vsr", "list", "--terminated", "--active"},
			pat:  "terminated.*active",
		},
		{
			args: []string{"vsr", "list", "-N", "nodeName"},
			pat:  "node-name requires cluster-name and domain",
		},
		{
			args: []string{"vsr", "list", "-N", "nodeName", "-C", "clusterName"},
			pat:  "cluster-name requires domain",
		},
		{
			args: []string{"vsr", "list", "-n", "vsName"},
			pat:  "volume series name requires account name",
		},
		{
			args: []string{"vsr", "list", "-N", "nodeName", "--node-id", "nodeId"},
			pat:  "node-name.*node-id",
		},
	}
	for _, tc := range invalidFlagTCs {
		t.Log(tc.pat)
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
		appCtx.API = mAPI
		te = &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initVolumeSeriesRequest()
		err = parseAndRun(tc.args)
		assert.NotNil(err)
		assert.Regexp(tc.pat, err.Error())
	}

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "list", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestVolumeSeriesRequestCreate(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	resSPA := &service_plan_allocation.ServicePlanAllocationListOK{
		Payload: []*models.ServicePlanAllocation{
			&models.ServicePlanAllocation{
				ServicePlanAllocationAllOf0: models.ServicePlanAllocationAllOf0{
					Meta: &models.ObjMeta{
						ID: "SPA-1",
					},
				},
				ServicePlanAllocationCreateOnce: models.ServicePlanAllocationCreateOnce{
					AccountID:           "accountID",
					AuthorizedAccountID: "authAccountID",
					ClusterID:           "clusterID",
					ServicePlanID:       "plan-1",
				},
			},
		},
	}
	resVS := &volume_series.VolumeSeriesListOK{
		Payload: []*models.VolumeSeries{
			&models.VolumeSeries{
				VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
					Meta: &models.ObjMeta{
						ID:           "VS-1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
					RootParcelUUID: "uuid1",
				},
				VolumeSeriesMutable: models.VolumeSeriesMutable{
					VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
						Name: "MyVolSeries",
					},
				},
			},
		},
	}
	resSourceVS := &volume_series.VolumeSeriesListOK{
		Payload: []*models.VolumeSeries{
			&models.VolumeSeries{
				VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
					Meta: &models.ObjMeta{
						ID: "SOURCE-VS",
					},
				},
				VolumeSeriesMutable: models.VolumeSeriesMutable{
					VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
						Name: "SourceVolume",
					},
				},
			},
		},
	}
	// success, create, bind
	res := &volume_series_request.VolumeSeriesRequestCreateCreated{
		Payload: &models.VolumeSeriesRequest{
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID:           "id",
					Version:      1,
					TimeCreated:  strfmt.DateTime(now),
					TimeModified: strfmt.DateTime(now),
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				ApplicationGroupIds: []models.ObjIDMutable{"a-g-1"},
				ClusterID:           "clusterID",
				CompleteByTime:      strfmt.DateTime(now.Add(time.Hour)),
				RequestedOperations: []string{"CREATE", "BIND", "MOUNT"},
				VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{
					VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
						AccountID: "accountID",
					},
					VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
						Name:               "MyVolSeries",
						Description:        "new volume series",
						ConsistencyGroupID: "c-g-1",
						ServicePlanID:      "plan-1",
						SizeBytes:          swag.Int64(21474836480),
						Tags:               []string{"tag1:value1"},
					},
				},
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "VS-1",
				},
			},
		},
	}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := cacheLoadHelper(t, mockCtrl, loadPlans, loadAGs, loadCGs, loadDomains, loadClusters, loadNodes, loadContext)
	params := volume_series_request.NewVolumeSeriesRequestCreateParams()
	params.Payload = &models.VolumeSeriesRequestCreateArgs{}
	params.Payload.RequestedOperations = []string{"CREATE", "BIND"}
	params.Payload.CompleteByTime = res.Payload.CompleteByTime
	params.Payload.ClusterID = models.ObjIDMutable("clusterID")
	params.Payload.ApplicationGroupIds = res.Payload.ApplicationGroupIds
	params.Payload.ConsistencyGroupID = "c-g-1"
	params.Payload.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{}
	params.Payload.VolumeSeriesCreateSpec.AccountID = models.ObjIDMutable("accountID")
	params.Payload.VolumeSeriesCreateSpec.ConsistencyGroupID = res.Payload.VolumeSeriesCreateSpec.ConsistencyGroupID
	params.Payload.VolumeSeriesCreateSpec.Description = res.Payload.VolumeSeriesCreateSpec.Description
	params.Payload.VolumeSeriesCreateSpec.Name = res.Payload.VolumeSeriesCreateSpec.Name
	params.Payload.VolumeSeriesCreateSpec.ServicePlanID = models.ObjIDMutable("plan-1")
	params.Payload.VolumeSeriesCreateSpec.SizeBytes = res.Payload.VolumeSeriesCreateSpec.SizeBytes
	params.Payload.VolumeSeriesCreateSpec.Tags = res.Payload.VolumeSeriesCreateSpec.Tags
	m := mockmgmtclient.NewVolumeSeriesRequestMatcher(t, params)
	m.D = time.Hour
	cOps := mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	cOps.EXPECT().VolumeSeriesRequestCreate(m).Return(res, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err := parseAndRun([]string{"volume-series-request", "create", "-O", "CREATE", "-O", "BIND",
		"-D", "domainName", "-C", "clusterName",
		"-n", "MyVolSeries", "-A", "System", "-d", "new volume series", "-P", "plan B", "-b", "20GiB",
		"--application-group", "AG", "--consistency-group", "CG", "-t", "tag1:value1", "-x", "1h", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.VolumeSeriesRequest{res.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// success, create, bind, publish
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadPlans, loadAGs, loadCGs, loadDomains, loadClusters, loadNodes, loadContext)
	params = volume_series_request.NewVolumeSeriesRequestCreateParams()
	params.Payload = &models.VolumeSeriesRequestCreateArgs{}
	params.Payload.RequestedOperations = []string{"CREATE", "BIND", "PUBLISH"}
	params.Payload.CompleteByTime = res.Payload.CompleteByTime
	params.Payload.ClusterID = models.ObjIDMutable("clusterID")
	params.Payload.ApplicationGroupIds = res.Payload.ApplicationGroupIds
	params.Payload.ConsistencyGroupID = "c-g-1"
	params.Payload.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{}
	params.Payload.VolumeSeriesCreateSpec.AccountID = models.ObjIDMutable("accountID")
	params.Payload.VolumeSeriesCreateSpec.ConsistencyGroupID = res.Payload.VolumeSeriesCreateSpec.ConsistencyGroupID
	params.Payload.VolumeSeriesCreateSpec.Description = res.Payload.VolumeSeriesCreateSpec.Description
	params.Payload.VolumeSeriesCreateSpec.Name = res.Payload.VolumeSeriesCreateSpec.Name
	params.Payload.VolumeSeriesCreateSpec.ServicePlanID = models.ObjIDMutable("plan-1")
	params.Payload.VolumeSeriesCreateSpec.SizeBytes = res.Payload.VolumeSeriesCreateSpec.SizeBytes
	params.Payload.VolumeSeriesCreateSpec.Tags = res.Payload.VolumeSeriesCreateSpec.Tags
	params.Payload.FsType = "ext4"
	params.Payload.DriverType = "flex"
	m = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, params)
	m.D = time.Hour
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	cOps.EXPECT().VolumeSeriesRequestCreate(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "create", "-O", "CREATE", "-O", "BIND", "-O", "PUBLISH",
		"-D", "domainName", "-C", "clusterName", "-F", "ext4", "--driver-type", "flex",
		"-n", "MyVolSeries", "-A", "System", "-d", "new volume series", "-P", "plan B", "-b", "20GiB",
		"--application-group", "AG", "--consistency-group", "CG", "-t", "tag1:value1", "-x", "1h", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.VolumeSeriesRequest{res.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// success, bind, mount, plan-only, existing VolumeSeries snapshot
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes, loadContext)
	mVS := mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesListParams())
	mVS.ListParam.AccountID = swag.String("accountID")
	mVS.ListParam.Name = swag.String(string(res.Payload.VolumeSeriesCreateSpec.Name))
	vsOps := mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vsOps)
	vsOps.EXPECT().VolumeSeriesList(mVS).Return(resVS, nil)
	params = volume_series_request.NewVolumeSeriesRequestCreateParams()
	params.Payload = &models.VolumeSeriesRequestCreateArgs{}
	params.Payload.RequestedOperations = []string{"BIND", "MOUNT", "ATTACH_FS"}
	params.Payload.CompleteByTime = res.Payload.CompleteByTime
	params.Payload.ClusterID = models.ObjIDMutable("clusterID")
	params.Payload.NodeID = models.ObjIDMutable("nodeID-1")
	params.Payload.VolumeSeriesID = models.ObjIDMutable("VS-1")
	params.Payload.SnapIdentifier = "SNAP1"
	params.Payload.PlanOnly = swag.Bool(true)
	params.Payload.FsType = "ext4"
	params.Payload.TargetPath = "/mnt/foo"
	params.Payload.ReadOnly = true
	m = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, params)
	m.D = time.Hour
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	cOps.EXPECT().VolumeSeriesRequestCreate(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "create", "-O", "BIND", "-O", "MOUNT", "-O", "ATTACH_FS",
		"-D", "domainName", "-C", "clusterName", "-N", "nodeName", "--plan-only", "--fs-type", "ext4", "--target-path", "/mnt/foo", "--read-only",
		"-n", "MyVolSeries", "-A", "System", "--snap-identifier", "SNAP1", "-x", "1h", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.VolumeSeriesRequest{res.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// create, API failure model error
	apiErr := &volume_series_request.VolumeSeriesRequestCreateDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	params = volume_series_request.NewVolumeSeriesRequestCreateParams()
	params.Payload = &models.VolumeSeriesRequestCreateArgs{}
	params.Payload.RequestedOperations = []string{"BIND", "MOUNT"}
	params.Payload.CompleteByTime = res.Payload.CompleteByTime
	params.Payload.ClusterID = models.ObjIDMutable("clusterID")
	params.Payload.NodeID = models.ObjIDMutable("nodeID-1")
	params.Payload.VolumeSeriesID = models.ObjIDMutable("VS-1")
	params.Payload.SnapIdentifier = "SNAP1"
	params.Payload.PlanOnly = swag.Bool(true)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes, loadContext)
	vsOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vsOps)
	vsOps.EXPECT().VolumeSeriesList(mVS).Return(resVS, nil)
	m = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, params)
	m.D = time.Hour
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	cOps.EXPECT().VolumeSeriesRequestCreate(m).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "create", "-O", "BIND", "-O", "MOUNT",
		"-D", "domainName", "-C", "clusterName", "-N", "nodeName", "--plan-only",
		"-n", "MyVolSeries", "-A", "System", "--snap-identifier", "SNAP1", "-x", "1h", "-o", "json"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// create, API failure arbitrary error
	otherErr := fmt.Errorf("OTHER ERROR")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes, loadContext)
	vsOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vsOps)
	vsOps.EXPECT().VolumeSeriesList(mVS).Return(resVS, nil)
	m = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, params)
	m.D = time.Hour
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	cOps.EXPECT().VolumeSeriesRequestCreate(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "create", "-O", "BIND", "-O", "MOUNT",
		"-D", "domainName", "-C", "clusterName", "-N", "nodeName", "--plan-only",
		"-n", "MyVolSeries", "-A", "System", "--snap-identifier", "SNAP1", "-x", "1h", "-o", "json"})
	assert.NotNil(err)
	assert.Equal(otherErr.Error(), err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// create, CONFIGURE, API failure arbitrary error
	otherErr = fmt.Errorf("OTHER ERROR")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes, loadContext)
	vsOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vsOps)
	vsOps.EXPECT().VolumeSeriesList(mVS).Return(resVS, nil)
	params.Payload.RequestedOperations = []string{"CONFIGURE"}
	params.Payload.SnapIdentifier = ""
	m = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, params)
	m.D = time.Hour
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	cOps.EXPECT().VolumeSeriesRequestCreate(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "create", "-O", "CONFIGURE",
		"-D", "domainName", "-C", "clusterName", "-N", "nodeName", "--plan-only",
		"-n", "MyVolSeries", "-A", "System", "-x", "1h", "-o", "json"})
	assert.NotNil(err)
	assert.Equal(otherErr.Error(), err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

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
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"vsr", "create", "-A", "System", "-O", "CREATE"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// success, rename
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes, loadContext)
	mVS = mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesListParams())
	mVS.ListParam.AccountID = swag.String("accountID")
	mVS.ListParam.Name = swag.String(string(res.Payload.VolumeSeriesCreateSpec.Name))
	vsOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vsOps)
	vsOps.EXPECT().VolumeSeriesList(mVS).Return(resVS, nil)
	params = volume_series_request.NewVolumeSeriesRequestCreateParams()
	params.Payload = &models.VolumeSeriesRequestCreateArgs{}
	params.Payload.RequestedOperations = []string{"RENAME"}
	params.Payload.CompleteByTime = res.Payload.CompleteByTime
	params.Payload.VolumeSeriesID = models.ObjIDMutable("VS-1")
	params.Payload.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{}
	params.Payload.VolumeSeriesCreateSpec.AccountID = models.ObjIDMutable("accountID")
	params.Payload.VolumeSeriesCreateSpec.Name = "renamed"
	m = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, params)
	m.D = time.Hour
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	cOps.EXPECT().VolumeSeriesRequestCreate(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "create", "-O", "RENAME",
		"-n", "MyVolSeries", "-A", "System", "--new-name=renamed", "-x", "1h", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.VolumeSeriesRequest{res.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// success, unmount, delete, existing VolumeSeries snapshot
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
	params = volume_series_request.NewVolumeSeriesRequestCreateParams()
	params.Payload = &models.VolumeSeriesRequestCreateArgs{}
	params.Payload.RequestedOperations = []string{"UNMOUNT", "DELETE", "DETACH_FS"}
	params.Payload.CompleteByTime = res.Payload.CompleteByTime
	params.Payload.ClusterID = models.ObjIDMutable("clusterID")
	params.Payload.NodeID = models.ObjIDMutable("nodeID-1")
	params.Payload.VolumeSeriesID = models.ObjIDMutable("VS-1")
	params.Payload.SnapIdentifier = "SNAP1"
	params.Payload.TargetPath = "/mnt/foo"
	m = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, params)
	m.D = time.Hour
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	cOps.EXPECT().VolumeSeriesRequestCreate(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "create", "-O", "UNMOUNT", "-O", "DELETE", "-O", "DETACH_FS",
		"-D", "domainName", "-C", "clusterName", "-N", "nodeName", "--target-path", "/mnt/foo",
		"-V", "VS-1", "--snap-identifier", "SNAP1", "-x", "1h", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.VolumeSeriesRequest{res.Payload}, te.jsonData)

	// success, allocate capacity
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadPlans, loadAccounts, loadDomains, loadClusters, loadNodes, loadContext)
	params = volume_series_request.NewVolumeSeriesRequestCreateParams()
	params.Payload = &models.VolumeSeriesRequestCreateArgs{}
	params.Payload.RequestedOperations = []string{"ALLOCATE_CAPACITY"}
	params.Payload.CompleteByTime = res.Payload.CompleteByTime
	params.Payload.ClusterID = ""
	params.Payload.ServicePlanAllocationCreateSpec = &models.ServicePlanAllocationCreateArgs{}
	params.Payload.ServicePlanAllocationCreateSpec.AccountID = "accountID"
	params.Payload.ServicePlanAllocationCreateSpec.AuthorizedAccountID = "authAccountID"
	params.Payload.ServicePlanAllocationCreateSpec.ClusterID = models.ObjIDMutable("clusterID")
	params.Payload.ServicePlanAllocationCreateSpec.ServicePlanID = "plan-1"
	params.Payload.ServicePlanAllocationCreateSpec.Tags = []string{"tag1:value1"}
	params.Payload.ServicePlanAllocationCreateSpec.TotalCapacityBytes = swag.Int64(200000000000)
	m = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, params)
	m.D = time.Hour
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	cOps.EXPECT().VolumeSeriesRequestCreate(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "create", "-O", "ALLOCATE_CAPACITY",
		"-D", "domainName", "-C", "clusterName", "-A", "System", "-Z", "AuthAccount",
		"-P", "plan B", "-b", "200GB", "-t", "tag1:value1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.VolumeSeriesRequest{res.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// success, allocate capacity, default authorized account
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadPlans, loadDomains, loadClusters, loadNodes, loadContext)
	params = volume_series_request.NewVolumeSeriesRequestCreateParams()
	params.Payload = &models.VolumeSeriesRequestCreateArgs{}
	params.Payload.RequestedOperations = []string{"ALLOCATE_CAPACITY"}
	params.Payload.CompleteByTime = res.Payload.CompleteByTime
	params.Payload.ClusterID = ""
	params.Payload.ServicePlanAllocationCreateSpec = &models.ServicePlanAllocationCreateArgs{}
	params.Payload.ServicePlanAllocationCreateSpec.AccountID = "accountID"
	params.Payload.ServicePlanAllocationCreateSpec.AuthorizedAccountID = "accountID"
	params.Payload.ServicePlanAllocationCreateSpec.ClusterID = models.ObjIDMutable("clusterID")
	params.Payload.ServicePlanAllocationCreateSpec.ServicePlanID = "plan-1"
	params.Payload.ServicePlanAllocationCreateSpec.Tags = []string{"tag1:value1"}
	params.Payload.ServicePlanAllocationCreateSpec.TotalCapacityBytes = swag.Int64(200000000000)
	m = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, params)
	m.D = time.Hour
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	cOps.EXPECT().VolumeSeriesRequestCreate(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "create", "-O", "ALLOCATE_CAPACITY",
		"-D", "domainName", "-C", "clusterName", "-A", "System",
		"-P", "plan B", "-b", "200GB", "-t", "tag1:value1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.VolumeSeriesRequest{res.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""
	mockCtrl.Finish()

	// success, delete SPA
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadPlans, loadAccounts, loadDomains, loadClusters, loadNodes, loadContext)
	spaOps := mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
	mAPI.EXPECT().ServicePlanAllocation().Return(spaOps).MinTimes(1)
	spaListP := service_plan_allocation.NewServicePlanAllocationListParams()
	spaListP.AuthorizedAccountID = swag.String("authAccountID")
	spaListP.ClusterID = swag.String("clusterID")
	spaListP.ServicePlanID = swag.String("plan-1")
	spam := mockmgmtclient.NewServicePlanAllocationMatcher(t, spaListP)
	spaOps.EXPECT().ServicePlanAllocationList(spam).Return(resSPA, nil)
	params = volume_series_request.NewVolumeSeriesRequestCreateParams()
	params.Payload = &models.VolumeSeriesRequestCreateArgs{}
	params.Payload.RequestedOperations = []string{"DELETE_SPA"}
	params.Payload.CompleteByTime = res.Payload.CompleteByTime
	params.Payload.ClusterID = models.ObjIDMutable("clusterID")
	params.Payload.ServicePlanAllocationID = "SPA-1"
	m = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, params)
	m.D = time.Hour
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	cOps.EXPECT().VolumeSeriesRequestCreate(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "create", "-O", "DELETE_SPA",
		"-D", "domainName", "-C", "clusterName", "-A", "System", "-Z", "AuthAccount",
		"-P", "plan B", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.VolumeSeriesRequest{res.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""
	mockCtrl.Finish()

	// success, create from snapshot with snapshot-id
	cfsRes := &volume_series_request.VolumeSeriesRequestCreateCreated{
		Payload: &models.VolumeSeriesRequest{
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID:           "id",
					Version:      1,
					TimeCreated:  strfmt.DateTime(now),
					TimeModified: strfmt.DateTime(now),
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				ClusterID:           "clusterID",
				CompleteByTime:      strfmt.DateTime(now.Add(time.Hour)),
				RequestedOperations: []string{"CREATE_FROM_SNAPSHOT"},
				VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{
					VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
						AccountID: "accountID",
					},
					VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
						Name:          "NewVol",
						Description:   "Restored from snapshot",
						ServicePlanID: "plan-1",
						Tags:          []string{"tag1:value1"},
					},
				},
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					NodeID: "nodeID-1",
					Snapshot: &models.SnapshotData{
						VolumeSeriesID: "VS-1",
						SnapIdentifier: "snapId",
					},
				},
			},
		},
	}
	cfsParams := volume_series_request.NewVolumeSeriesRequestCreateParams()
	cfsParams.Payload = &models.VolumeSeriesRequestCreateArgs{}
	cfsParams.Payload.RequestedOperations = []string{"CREATE_FROM_SNAPSHOT"}
	cfsParams.Payload.CompleteByTime = cfsRes.Payload.CompleteByTime
	cfsParams.Payload.ClusterID = models.ObjIDMutable("clusterID")
	cfsParams.Payload.NodeID = models.ObjIDMutable("nodeID-1")
	cfsParams.Payload.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{}
	cfsParams.Payload.VolumeSeriesCreateSpec.AccountID = cfsRes.Payload.VolumeSeriesCreateSpec.AccountID
	cfsParams.Payload.VolumeSeriesCreateSpec.Name = cfsRes.Payload.VolumeSeriesCreateSpec.Name
	cfsParams.Payload.VolumeSeriesCreateSpec.Description = cfsRes.Payload.VolumeSeriesCreateSpec.Description
	cfsParams.Payload.VolumeSeriesCreateSpec.ServicePlanID = models.ObjIDMutable("plan-1")
	cfsParams.Payload.VolumeSeriesCreateSpec.Tags = cfsRes.Payload.VolumeSeriesCreateSpec.Tags
	cfsParams.Payload.SnapshotID = "snapshotID"
	cfsM := mockmgmtclient.NewVolumeSeriesRequestMatcher(t, cfsParams)
	cfsM.D = time.Hour
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadPlans, loadDomains, loadClusters, loadNodes, loadContext)
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	cOps.EXPECT().VolumeSeriesRequestCreate(cfsM).Return(cfsRes, nil)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "create", "-O", "CREATE_FROM_SNAPSHOT",
		"-D", "domainName", "-C", "clusterName", "-N", "nodeName", "-A", "System",
		"-S", "snapshotID", "--new-name", "NewVol", "-d", "Restored from snapshot",
		"-P", "plan B", "-t", "tag1:value1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.VolumeSeriesRequest{cfsRes.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""
	mockCtrl.Finish()

	// CREATE_FROM_SNAPSHOT with snapshot object
	cfsParams = volume_series_request.NewVolumeSeriesRequestCreateParams()
	cfsParams.Payload = &models.VolumeSeriesRequestCreateArgs{}
	cfsParams.Payload.RequestedOperations = []string{"CREATE_FROM_SNAPSHOT"}
	cfsParams.Payload.CompleteByTime = cfsRes.Payload.CompleteByTime
	cfsParams.Payload.ClusterID = models.ObjIDMutable("clusterID")
	cfsParams.Payload.NodeID = models.ObjIDMutable("nodeID-1")
	cfsParams.Payload.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{}
	cfsParams.Payload.VolumeSeriesCreateSpec.AccountID = cfsRes.Payload.VolumeSeriesCreateSpec.AccountID
	cfsParams.Payload.VolumeSeriesCreateSpec.Name = cfsRes.Payload.VolumeSeriesCreateSpec.Name
	cfsParams.Payload.VolumeSeriesCreateSpec.Description = cfsRes.Payload.VolumeSeriesCreateSpec.Description
	cfsParams.Payload.VolumeSeriesCreateSpec.ServicePlanID = models.ObjIDMutable("plan-1")
	cfsParams.Payload.VolumeSeriesCreateSpec.Tags = cfsRes.Payload.VolumeSeriesCreateSpec.Tags
	cfsParams.Payload.Snapshot = &models.SnapshotData{}
	cfsParams.Payload.Snapshot.VolumeSeriesID = cfsRes.Payload.Snapshot.VolumeSeriesID
	cfsParams.Payload.Snapshot.SnapIdentifier = cfsRes.Payload.Snapshot.SnapIdentifier
	cfsM = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, cfsParams)
	cfsM.D = time.Hour
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadPlans, loadDomains, loadClusters, loadNodes, loadContext)
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	cOps.EXPECT().VolumeSeriesRequestCreate(cfsM).Return(cfsRes, nil)
	vsOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mVS = mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesListParams())
	mVS.ListParam.AccountID = swag.String("accountID")
	mVS.ListParam.Name = swag.String("MyVolSeries")
	vsOps.EXPECT().VolumeSeriesList(mVS).Return(resVS, nil)
	mAPI.EXPECT().VolumeSeries().Return(vsOps)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "create", "-O", "CREATE_FROM_SNAPSHOT",
		"-D", "domainName", "-C", "clusterName", "-N", "nodeName", "-A", "System",
		"-n", "MyVolSeries", "--snap-identifier", "snapId", "--new-name", "NewVol", "-d", "Restored from snapshot",
		"-P", "plan B", "-t", "tag1:value1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.VolumeSeriesRequest{cfsRes.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""
	mockCtrl.Finish()

	// success vol snapshot restore (with snapshotID)
	vsrRes := &volume_series_request.VolumeSeriesRequestCreateCreated{
		Payload: &models.VolumeSeriesRequest{
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID:           "id",
					Version:      1,
					TimeCreated:  strfmt.DateTime(now),
					TimeModified: strfmt.DateTime(now),
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				CompleteByTime:      strfmt.DateTime(now.Add(time.Hour)),
				RequestedOperations: []string{"VOL_SNAPSHOT_RESTORE"},
				SnapshotID:          "snapshotID",
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "VS-1",
				},
			},
		},
	}
	vsrParams := volume_series_request.NewVolumeSeriesRequestCreateParams()
	vsrParams.Payload = &models.VolumeSeriesRequestCreateArgs{}
	vsrParams.Payload.RequestedOperations = []string{"VOL_SNAPSHOT_RESTORE"}
	vsrParams.Payload.CompleteByTime = vsrRes.Payload.CompleteByTime
	vsrParams.Payload.VolumeSeriesID = vsrRes.Payload.VolumeSeriesID
	vsrParams.Payload.SnapshotID = models.ObjIDMutable(vsrRes.Payload.SnapshotID)
	vsrM := mockmgmtclient.NewVolumeSeriesRequestMatcher(t, vsrParams)
	vsrM.D = time.Hour
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes, loadContext)
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	cOps.EXPECT().VolumeSeriesRequestCreate(vsrM).Return(vsrRes, nil)
	vsOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mVS = mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesListParams())
	mVS.ListParam.AccountID = swag.String("accountID")
	mVS.ListParam.Name = swag.String("MyVolSeries")
	vsOps.EXPECT().VolumeSeriesList(mVS).Return(resVS, nil)
	mAPI.EXPECT().VolumeSeries().Return(vsOps).MinTimes(1)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "create",
		"-O", "VOL_SNAPSHOT_RESTORE",
		"-A", "System", "-n", "MyVolSeries", "-b", "20GiB",
		"-S", "snapshotID", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.VolumeSeriesRequest{vsrRes.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""
	mockCtrl.Finish()

	// success vol snapshot restore (with snap-volume-name)
	vsrRes = &volume_series_request.VolumeSeriesRequestCreateCreated{
		Payload: &models.VolumeSeriesRequest{
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID:           "id",
					Version:      1,
					TimeCreated:  strfmt.DateTime(now),
					TimeModified: strfmt.DateTime(now),
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				CompleteByTime:      strfmt.DateTime(now.Add(time.Hour)),
				RequestedOperations: []string{"VOL_SNAPSHOT_RESTORE"},
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "VS-1",
					Snapshot: &models.SnapshotData{
						VolumeSeriesID: models.ObjIDMutable(resSourceVS.Payload[0].Meta.ID),
						SnapIdentifier: "snapId",
						Locations: []*models.SnapshotLocation{
							&models.SnapshotLocation{CspDomainID: "CSP-DOMAIN-1"},
						},
						SizeBytes:     swag.Int64(21474836480),
						PitIdentifier: "pitId",
					},
				},
			},
		},
	}
	vsrParams = volume_series_request.NewVolumeSeriesRequestCreateParams()
	vsrParams.Payload = &models.VolumeSeriesRequestCreateArgs{}
	vsrParams.Payload.RequestedOperations = []string{"VOL_SNAPSHOT_RESTORE"}
	vsrParams.Payload.CompleteByTime = vsrRes.Payload.CompleteByTime
	vsrParams.Payload.VolumeSeriesID = vsrRes.Payload.VolumeSeriesID
	vsrParams.Payload.Snapshot = &models.SnapshotData{}
	vsrParams.Payload.Snapshot.VolumeSeriesID = vsrRes.Payload.Snapshot.VolumeSeriesID
	vsrParams.Payload.Snapshot.SnapIdentifier = vsrRes.Payload.Snapshot.SnapIdentifier
	vsrParams.Payload.Snapshot.Locations = vsrRes.Payload.Snapshot.Locations
	vsrParams.Payload.Snapshot.SizeBytes = vsrRes.Payload.Snapshot.SizeBytes
	vsrParams.Payload.Snapshot.PitIdentifier = vsrRes.Payload.Snapshot.PitIdentifier
	vsrM = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, vsrParams)
	vsrM.D = time.Hour
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes, loadContext)
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	cOps.EXPECT().VolumeSeriesRequestCreate(vsrM).Return(vsrRes, nil)
	vsOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mVS = mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesListParams())
	mVS.ListParam.AccountID = swag.String("accountID")
	mVS.ListParam.Name = swag.String("MyVolSeries")
	vsOps.EXPECT().VolumeSeriesList(mVS).Return(resVS, nil)
	mSourceVS := mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesListParams())
	mSourceVS.ListParam.AccountID = swag.String("accountID")
	mSourceVS.ListParam.Name = swag.String("SourceVolume")
	vsOps.EXPECT().VolumeSeriesList(mSourceVS).Return(resSourceVS, nil)
	mAPI.EXPECT().VolumeSeries().Return(vsOps).MinTimes(2)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "create", "-O", "VOL_SNAPSHOT_RESTORE",
		"-A", "System", "-n", "MyVolSeries", "-b", "20GiB",
		"--snap-identifier", "snapId", "--snap-volume-name", "SourceVolume", "--snap-location", "domainName", "--snap-pit-identifier", "pitId",
		"-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.VolumeSeriesRequest{vsrRes.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""
	mockCtrl.Finish()

	// success vol snapshot restore (with snap-volume-id)
	vsrRes = &volume_series_request.VolumeSeriesRequestCreateCreated{
		Payload: &models.VolumeSeriesRequest{
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID:           "id",
					Version:      1,
					TimeCreated:  strfmt.DateTime(now),
					TimeModified: strfmt.DateTime(now),
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				CompleteByTime:      strfmt.DateTime(now.Add(time.Hour)),
				RequestedOperations: []string{"VOL_SNAPSHOT_RESTORE"},
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "VS-1",
					Snapshot: &models.SnapshotData{
						VolumeSeriesID: "source-vs-id",
						SnapIdentifier: "snapId",
					},
				},
			},
		},
	}
	vsrParams = volume_series_request.NewVolumeSeriesRequestCreateParams()
	vsrParams.Payload = &models.VolumeSeriesRequestCreateArgs{}
	vsrParams.Payload.RequestedOperations = []string{"VOL_SNAPSHOT_RESTORE"}
	vsrParams.Payload.CompleteByTime = vsrRes.Payload.CompleteByTime
	vsrParams.Payload.VolumeSeriesID = vsrRes.Payload.VolumeSeriesID
	vsrParams.Payload.Snapshot = &models.SnapshotData{}
	vsrParams.Payload.Snapshot.VolumeSeriesID = vsrRes.Payload.Snapshot.VolumeSeriesID
	vsrParams.Payload.Snapshot.SnapIdentifier = vsrRes.Payload.Snapshot.SnapIdentifier
	vsrM = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, vsrParams)
	vsrM.D = time.Hour
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes, loadContext)
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	cOps.EXPECT().VolumeSeriesRequestCreate(vsrM).Return(vsrRes, nil)
	vsOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mVS = mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesListParams())
	mVS.ListParam.AccountID = swag.String("accountID")
	mVS.ListParam.Name = swag.String("MyVolSeries")
	vsOps.EXPECT().VolumeSeriesList(mVS).Return(resVS, nil)
	mAPI.EXPECT().VolumeSeries().Return(vsOps)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "create", "-O", "VOL_SNAPSHOT_RESTORE",
		"-A", "System",
		"-n", "MyVolSeries", "--snap-identifier", "snapId", "--snap-volume-id", "source-vs-id",
		"-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.VolumeSeriesRequest{vsrRes.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// success CHANGE_CAPACITY
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
	params = volume_series_request.NewVolumeSeriesRequestCreateParams()
	params.Payload = &models.VolumeSeriesRequestCreateArgs{}
	params.Payload.RequestedOperations = []string{"CHANGE_CAPACITY"}
	params.Payload.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{
		VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
			SizeBytes:          swag.Int64(1000000000),
			SpaAdditionalBytes: swag.Int64(0),
		},
	}
	params.Payload.CompleteByTime = res.Payload.CompleteByTime
	params.Payload.VolumeSeriesID = models.ObjIDMutable("VS-1")
	m = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, params)
	m.D = time.Hour
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	cOps.EXPECT().VolumeSeriesRequestCreate(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "create", "-O", "CHANGE_CAPACITY",
		"-V", "VS-1", "-b", "1GB", "--spa-additional-bytes", "0", "-x", "1h", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.VolumeSeriesRequest{res.Payload}, te.jsonData)

	// success UNPUBLISH
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
	params = volume_series_request.NewVolumeSeriesRequestCreateParams()
	params.Payload = &models.VolumeSeriesRequestCreateArgs{}
	params.Payload.RequestedOperations = []string{"UNPUBLISH"}
	params.Payload.CompleteByTime = res.Payload.CompleteByTime
	params.Payload.VolumeSeriesID = models.ObjIDMutable("VS-1")
	m = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, params)
	m.D = time.Hour
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	cOps.EXPECT().VolumeSeriesRequestCreate(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "create", "-O", "UNPUBLISH",
		"-V", "VS-1", "-x", "1h", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.VolumeSeriesRequest{res.Payload}, te.jsonData)

	// success, unbind
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes, loadContext)
	mVS = mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesListParams())
	mVS.ListParam.AccountID = swag.String("accountID")
	mVS.ListParam.Name = swag.String(string(res.Payload.VolumeSeriesCreateSpec.Name))
	vsOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vsOps)
	vsOps.EXPECT().VolumeSeriesList(mVS).Return(resVS, nil)
	params = volume_series_request.NewVolumeSeriesRequestCreateParams()
	params.Payload = &models.VolumeSeriesRequestCreateArgs{}
	params.Payload.RequestedOperations = []string{"UNBIND"}
	params.Payload.CompleteByTime = res.Payload.CompleteByTime
	params.Payload.VolumeSeriesID = models.ObjIDMutable("VS-1")
	m = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, params)
	m.D = time.Hour
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps)
	cOps.EXPECT().VolumeSeriesRequestCreate(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "create", "-O", "UNBIND",
		"-n", "MyVolSeries", "-A", "System", "-x", "1h", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.VolumeSeriesRequest{res.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// validation error cases
	vTCs := []struct {
		args             []string
		pat              string
		dcnpCacheErr     error
		accountContext   bool
		accountLookup    bool
		accountLookupErr error
		cgLookup         bool
		cgLookupErr      error
		agLookup         bool
		agLookupErr      error
		spaLookup        bool
		spaLookupErr     error
		svcPlanLookup    bool
		svcPlanLookupErr error
		vsLookup         bool
		vsLookupErr      error
		snapVsLookup     bool
		snapVsLookupErr  error
	}{
		{
			args:         []string{"vsr", "create", "-O", "BIND", "-D", "foo"},
			pat:          "dcnp cache error",
			dcnpCacheErr: fmt.Errorf("dcnp cache error"),
		},
		{
			args:          []string{"vsr", "create", "-O", "ALLOCATE_CAPACITY", "-Z", "foo"},
			pat:           "account.*not found",
			accountLookup: true,
		},
		{
			args:           []string{"vsr", "create", "-O", "DELETE_SPA", "-D", "domainName", "-C", "clusterName", "-A", "System", "-P", "plan B"},
			pat:            "service plan allocation not found",
			accountContext: true,
			spaLookup:      true,
			svcPlanLookup:  true,
		},
		{
			args:           []string{"vsr", "create", "-O", "DELETE_SPA", "-D", "domainName", "-C", "clusterName", "-A", "System", "-P", "plan B"},
			pat:            "SPA lookup error",
			accountContext: true,
			spaLookup:      true,
			spaLookupErr:   fmt.Errorf("SPA lookup error"),
			svcPlanLookup:  true,
		},
		{
			args: []string{"vsr", "create", "-O", "CREATE", "--consistency-group", "inconsistent"},
			pat:  "consistency-group requires account",
		},
		{
			args:           []string{"vsr", "create", "-O", "CREATE", "-A", "System", "--consistency-group", "inconsistent"},
			pat:            "cg lookup error",
			accountContext: true,
			cgLookup:       true,
			cgLookupErr:    fmt.Errorf("cg lookup error"),
		},
		{
			args:           []string{"vsr", "create", "-O", "CREATE", "-A", "System", "--consistency-group", "inconsistent"},
			pat:            "consistency group.*not found",
			accountContext: true,
			cgLookup:       true,
		},
		{
			args: []string{"vsr", "create", "-O", "CREATE", "--application-group", "inconsistent"},
			pat:  "application-group requires account",
		},
		{
			args:           []string{"vsr", "create", "-O", "CREATE", "-A", "System", "--application-group", "inconsistent"},
			pat:            "ag lookup error",
			accountContext: true,
			agLookup:       true,
			agLookupErr:    fmt.Errorf("ag lookup error"),
		},
		{
			args:           []string{"vsr", "create", "-O", "CREATE", "-A", "System", "--application-group", "inconsistent"},
			pat:            "application group.*not found",
			accountContext: true,
			agLookup:       true,
		},
		{
			args:             []string{"vsr", "create", "-O", "BIND", "-P", "Plan B"},
			pat:              "plan lookup error",
			svcPlanLookup:    true,
			svcPlanLookupErr: fmt.Errorf("plan lookup error"),
		},
		{
			args:          []string{"vsr", "create", "-O", "BIND", "-P", "Plan B"},
			pat:           "plan.*not found",
			svcPlanLookup: true,
		},
		{
			args:           []string{"vsr", "create", "-O", "BIND", "-A", "System", "-n", "MyVolSeries"},
			pat:            "vs lookup error",
			accountContext: true,
			vsLookup:       true,
			vsLookupErr:    fmt.Errorf("vs lookup error"),
		},
		{
			args:            []string{"vsr", "create", "-O", "VOL_SNAPSHOT_RESTORE", "-A", "System", "-n", "MyVolSeries", "--snap-volume-name", "SourceVolume"},
			pat:             "snapVS lookup error",
			accountContext:  true,
			vsLookup:        true,
			snapVsLookup:    true,
			snapVsLookupErr: fmt.Errorf("snapVS lookup error"),
		},
		{
			args:           []string{"vsr", "create", "-O", "VOL_SNAPSHOT_RESTORE", "-A", "System", "-n", "MyVolSeries", "--snap-location", "SnapLoc"},
			pat:            "snapshot location .* not found",
			accountContext: true,
			vsLookup:       true,
		},
	}
	for _, tc := range vTCs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		if tc.dcnpCacheErr == nil {
			mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
		} else {
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			dm := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
			dOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
			mAPI.EXPECT().CspDomain().Return(dOps)
			dOps.EXPECT().CspDomainList(dm).Return(nil, tc.dcnpCacheErr).MinTimes(1)
		}
		if tc.accountContext {
			authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
			mAPI.EXPECT().Authentication().Return(authOps)
			authOps.EXPECT().SetContextAccount("", "System").Return("accountID", nil)
		}
		if tc.accountLookup {
			apm := mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
			aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
			mAPI.EXPECT().Account().Return(aOps)
			if tc.accountLookupErr == nil {
				aOps.EXPECT().AccountList(apm).Return(resAccounts, nil)
			} else {
				aOps.EXPECT().AccountList(apm).Return(nil, tc.accountLookupErr)
			}
		}
		if tc.cgLookup {
			cgm := mockmgmtclient.NewConsistencyGroupMatcher(t, consistency_group.NewConsistencyGroupListParams())
			cgOps := mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
			mAPI.EXPECT().ConsistencyGroup().Return(cgOps)
			if tc.cgLookupErr == nil {
				cgOps.EXPECT().ConsistencyGroupList(cgm).Return(resCGs, nil)
			} else {
				cgOps.EXPECT().ConsistencyGroupList(cgm).Return(nil, tc.cgLookupErr)
			}
		}
		if tc.agLookup {
			agm := mockmgmtclient.NewApplicationGroupMatcher(t, application_group.NewApplicationGroupListParams())
			agOps := mockmgmtclient.NewMockApplicationGroupClient(mockCtrl)
			mAPI.EXPECT().ApplicationGroup().Return(agOps)
			if tc.agLookupErr == nil {
				agOps.EXPECT().ApplicationGroupList(agm).Return(resAGs, nil)
			} else {
				agOps.EXPECT().ApplicationGroupList(agm).Return(nil, tc.agLookupErr)
			}
		}
		if tc.svcPlanLookup {
			ppm := mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
			pOps := mockmgmtclient.NewMockServicePlanClient(mockCtrl)
			mAPI.EXPECT().ServicePlan().Return(pOps).MinTimes(1)
			if tc.svcPlanLookupErr == nil {
				pOps.EXPECT().ServicePlanList(ppm).Return(resPlans, nil)
			} else {
				pOps.EXPECT().ServicePlanList(ppm).Return(nil, tc.svcPlanLookupErr)
			}
		}
		if tc.spaLookup {
			spaListP := service_plan_allocation.NewServicePlanAllocationListParams()
			spaListP.AuthorizedAccountID = swag.String("accountID")
			spaListP.ClusterID = swag.String("clusterID")
			spaListP.ServicePlanID = swag.String("plan-1")
			spam := mockmgmtclient.NewServicePlanAllocationMatcher(t, spaListP)
			spaOps := mockmgmtclient.NewMockServicePlanAllocationClient(mockCtrl)
			mAPI.EXPECT().ServicePlanAllocation().Return(spaOps).MinTimes(1)
			if tc.spaLookupErr == nil {
				resSPA := &service_plan_allocation.ServicePlanAllocationListOK{Payload: []*models.ServicePlanAllocation{}}
				spaOps.EXPECT().ServicePlanAllocationList(spam).Return(resSPA, nil)
			} else {
				spaOps.EXPECT().ServicePlanAllocationList(spam).Return(nil, tc.spaLookupErr)
			}
		}
		vsOpCnt := 0
		if tc.vsLookup {
			vsOpCnt++
		}
		if tc.snapVsLookup {
			vsOpCnt++
		}
		if vsOpCnt > 0 {
			vsOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
			mAPI.EXPECT().VolumeSeries().Return(vsOps).MinTimes(vsOpCnt)
		}
		if tc.vsLookup {
			if tc.vsLookupErr == nil {
				vsOps.EXPECT().VolumeSeriesList(mVS).Return(resVS, nil)
			} else {
				vsOps.EXPECT().VolumeSeriesList(mVS).Return(nil, tc.vsLookupErr)
			}
		}
		if tc.snapVsLookup {
			if tc.snapVsLookupErr == nil {
				vsOps.EXPECT().VolumeSeriesList(mSourceVS).Return(resSourceVS, nil)
			} else {
				vsOps.EXPECT().VolumeSeriesList(mSourceVS).Return(nil, tc.snapVsLookupErr)
			}
		}
		appCtx.API = mAPI
		te = &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initVolumeSeriesRequest()
		t.Logf("** TC: %v", tc)
		err = parseAndRun(tc.args)
		assert.NotNil(err)
		assert.Regexp(tc.pat, err)
		if tc.accountContext {
			appCtx.Account, appCtx.AccountID = "", ""
		}
	}

	// invalid flag combinations / values
	invalidFlagTCs := []struct {
		args []string
		pat  string
	}{
		{
			args: []string{"vsr", "create", "-O", "BIND", "--columns", "FOO,BAR"},
			pat:  "invalid column",
		},
		{
			args: []string{"vsr", "create", "-O", "BIND", "-n", "foo"},
			pat:  "volume series name requires account name",
		},
		{
			args: []string{"vsr", "create", "-O", "BIND", "-b", "10"},
			pat:  "unknown unit",
		},
		{
			args: []string{"vsr", "create", "-O", "CHANGE_CAPACITY", "--spa-additional-bytes", "11"},
			pat:  "unknown unit",
		},
		{
			args: []string{"vsr", "create", "-O", "FOO"},
			pat:  "invalid operation",
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
		initVolumeSeriesRequest()
		err = parseAndRun(tc.args)
		assert.NotNil(err)
		assert.Regexp(tc.pat, err.Error())
	}

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "create", "-O", "CREATE", "-O", "BIND",
		"-D", "domainName", "-C", "clusterName",
		"-n", "MyVolSeries", "-A", "System", "-d", "new volume series", "-P", "plan B", "-b", "20GiB",
		"--application-group", "AG", "--consistency-group", "CG", "-t", "tag1:value1", "-x", "1h", "o", "json"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestVolumeSeriesRequestDelete(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	reqToDelete := "VSR-1"
	dParams := volume_series_request.NewVolumeSeriesRequestDeleteParams()
	dParams.ID = reqToDelete
	dRet := &volume_series_request.VolumeSeriesRequestDeleteNoContent{}

	// delete
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps).MinTimes(1)
	cOps.EXPECT().VolumeSeriesRequestDelete(dParams).Return(dRet, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err := parseAndRun([]string{"volume-series-request", "delete", reqToDelete, "--confirm"})
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
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "delete", "--id", reqToDelete})
	assert.NotNil(err)
	assert.Regexp("--confirm", err.Error())

	// delete, API model error
	apiErr := &volume_series_request.VolumeSeriesRequestDeleteDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps).MinTimes(1)
	cOps.EXPECT().VolumeSeriesRequestDelete(dParams).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "delete", "--id", reqToDelete, "--confirm"})

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
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"vsr", "delete", "-A", "System", "--id", reqToDelete, "--confirm"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("expected id")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "delete", "id", reqToDelete, "--confirm"})
	assert.NotNil(err)
	assert.Regexp("expected --id flag or a single identifier argument", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestVolumeSeriesRequestGet(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	reqToFetch := "VSR-1"
	fParams := volume_series_request.NewVolumeSeriesRequestFetchParams()
	fParams.ID = reqToFetch

	now := time.Now()
	fRet := &volume_series_request.VolumeSeriesRequestFetchOK{
		Payload: &models.VolumeSeriesRequest{
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID:           "VSR-1",
					Version:      1,
					TimeCreated:  strfmt.DateTime(now),
					TimeModified: strfmt.DateTime(now),
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				ClusterID:           "clusterID",
				CompleteByTime:      strfmt.DateTime(now.Add(time.Hour)),
				RequestedOperations: []string{"CREATE", "BIND", "MOUNT"},
				VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{
					VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
						AccountID: "System",
					},
					VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
						Name:               "MyVS-1",
						Description:        "new volume series",
						ConsistencyGroupID: "c-g-1",
						ServicePlanID:      "planID",
						SizeBytes:          swag.Int64(1073741824),
						Tags:               []string{"tag1:value1"},
					},
				},
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					NodeID:         "nodeID",
					VolumeSeriesID: "VS-1",
				},
			},
		},
	}
	resClusters := &cluster.ClusterListOK{Payload: []*models.Cluster{}}
	resNodes := &node.NodeListOK{Payload: []*models.Node{}}

	// fetch
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps).MinTimes(1)
	mF := mockmgmtclient.NewVolumeSeriesRequestMatcher(t, fParams)
	cOps.EXPECT().VolumeSeriesRequestFetch(mF).Return(fRet, nil)
	nm := mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps := mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil)
	cm := mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(cm).Return(resClusters, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err := parseAndRun([]string{"volume-series-request", "get", reqToFetch})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(volumeSeriesRequestDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, 1)

	// apiError
	apiErr := &volume_series_request.VolumeSeriesRequestFetchDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps).MinTimes(1)
	mF = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, fParams)
	cOps.EXPECT().VolumeSeriesRequestFetch(mF).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "get", "--id", reqToFetch})
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
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"vsr", "get", "-A", "System", "--id", reqToFetch})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// invalid columns
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "get", "--id", reqToFetch, "--columns", "foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	t.Log("expected id")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "get"})
	assert.NotNil(err)
	assert.Regexp("expected --id flag or a single identifier argument", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestVolumeSeriesRequestCancel(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	reqToCancel := "VSR-1"
	cParams := volume_series_request.NewVolumeSeriesRequestCancelParams()
	cParams.ID = reqToCancel

	now := time.Now()
	cRet := &volume_series_request.VolumeSeriesRequestCancelOK{
		Payload: &models.VolumeSeriesRequest{
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID:           "VSR-1",
					Version:      1,
					TimeCreated:  strfmt.DateTime(now),
					TimeModified: strfmt.DateTime(now),
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				ClusterID:           "clusterID",
				CompleteByTime:      strfmt.DateTime(now.Add(time.Hour)),
				RequestedOperations: []string{"CREATE", "BIND", "MOUNT"},
				VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{
					VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
						AccountID: "System",
					},
					VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
						Name:               "MyVS-1",
						Description:        "new volume series",
						ConsistencyGroupID: "c-g-1",
						ServicePlanID:      "planID",
						SizeBytes:          swag.Int64(1073741824),
						Tags:               []string{"tag1:value1"},
					},
				},
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					NodeID:         "nodeID",
					VolumeSeriesID: "VS-1",
				},
			},
		},
	}
	resClusters := &cluster.ClusterListOK{Payload: []*models.Cluster{}}
	resNodes := &node.NodeListOK{Payload: []*models.Node{}}

	// cancel
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps).MinTimes(1)
	cOps.EXPECT().VolumeSeriesRequestCancel(cParams).Return(cRet, nil)
	nm := mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps := mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(resNodes, nil)
	cm := mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(cm).Return(resClusters, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err := parseAndRun([]string{"volume-series-request", "cancel", "--id", reqToCancel, "--confirm"})
	assert.Nil(err)

	// cancel, --confirm not specified
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "cancel", "--id", reqToCancel})
	assert.NotNil(err)
	assert.Regexp("--confirm", err.Error())

	// cancel, API model error
	apiErr := &volume_series_request.VolumeSeriesRequestCancelDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(cOps).MinTimes(1)
	cOps.EXPECT().VolumeSeriesRequestCancel(cParams).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "cancel", "--id", reqToCancel, "--confirm"})

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
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"vsr", "cancel", "-A", "System", "--id", reqToCancel, "--confirm"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// invalid columns
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "cancel", "--id", reqToCancel, "--confirm", "--columns", "foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	t.Log("expected id")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeriesRequest()
	err = parseAndRun([]string{"volume-series-request", "cancel", "id", reqToCancel, "--confirm"})
	assert.NotNil(err)
	assert.Regexp("expected --id flag or a single identifier argument", err)
	appCtx.Account, appCtx.AccountID = "", ""
}
