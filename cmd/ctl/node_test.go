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
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/node"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestNodeMakeRecord(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	cmd := &nodeCmd{}
	cmd.accounts = map[string]ctxIDName{
		"tid": ctxIDName{id: "", name: "Nuvoloso"},
	}
	cmd.cspDomains = map[string]string{
		"cspDomainID": "MyDomain",
	}
	cmd.clusters = map[string]ctxIDName{
		"CLUSTER-1": ctxIDName{id: "cspDomainID", name: "MyCluster"},
	}
	o := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID:           "NODE-1",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
			AccountID:      "tid",
			ClusterID:      "CLUSTER-1",
			NodeIdentifier: "NODEID-1",
		},
		NodeMutable: models.NodeMutable{
			Service: &models.NuvoService{
				ServiceState: models.ServiceState{
					State: "RUNNING",
				},
			},
			State:       "MANAGED",
			Name:        "node1",
			Description: "node1 object",
			Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
			NodeAttributes: map[string]models.ValueType{
				"something": models.ValueType{Kind: "STRING", Value: "good"},
				"smells":    models.ValueType{Kind: "STRING", Value: "bad"},
				"awful":     models.ValueType{Kind: "STRING", Value: "ugly"},
			},
			LocalStorage: map[string]models.NodeStorageDevice{
				"uuid1": models.NodeStorageDevice{
					DeviceName:      "d1",
					DeviceState:     "UNUSED",
					DeviceType:      "HDD",
					SizeBytes:       swag.Int64(11),
					UsableSizeBytes: swag.Int64(10),
				},
				"uuid2": models.NodeStorageDevice{
					DeviceName:      "d2",
					DeviceState:     "CACHE",
					DeviceType:      "SSD",
					SizeBytes:       swag.Int64(22),
					UsableSizeBytes: swag.Int64(20),
				},
			},
			AvailableCacheBytes: swag.Int64(30),
			TotalCacheBytes:     swag.Int64(33),
		},
	}
	rec := cmd.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(nodeHeaders))
	assert.Equal("NODE-1", rec[hID])
	assert.Equal("1", rec[hVersion])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeCreated])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeModified])
	assert.Equal("RUNNING", rec[hControllerState])
	assert.Equal("node1", rec[hName])
	assert.Equal("node1 object", rec[hDescription])
	assert.Equal("Nuvoloso", rec[hAccount])
	assert.Equal("tag1, tag2, tag3", rec[hTags])
	assert.Equal("NODEID-1", rec[hNodeIdentifier])
	assert.Equal("MyCluster", rec[hCluster])
	assert.Equal("MyDomain", rec[hCspDomain])
	assert.Equal("MANAGED/RUNNING", rec[hNodeState])
	// note sorted order of attribute name
	al := "awful[S]: ugly\nsmells[S]: bad\nsomething[S]: good"
	assert.Equal(al, rec[hNodeAttributes])
	assert.Equal("30B", rec[hAvailableCache])
	assert.Equal("33B", rec[hTotalCache])
	ls := "d1 [HDD] UNUSED: 10B/11B\nd2 [SSD] CACHE: 20B/22B"
	assert.Equal(ls, rec[hLocalStorage])

	// ids not in map
	o.AccountID = "other"
	o.ClusterID = "FOO"
	rec = cmd.makeRecord(o)
	t.Log(rec)
	assert.Equal("other", rec[hAccount])
	assert.Equal("FOO", rec[hCluster])
	assert.Equal("", rec[hCspDomain])
}

func TestNodeList(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	params := node.NewNodeListParams().WithName(swag.String(""))
	m := mockmgmtclient.NewNodeMatcher(t, params)
	res := &node.NodeListOK{
		Payload: []*models.Node{
			&models.Node{
				NodeAllOf0: models.NodeAllOf0{
					Meta: &models.ObjMeta{
						ID:           "NODE-1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
					AccountID: "tid1",
					ClusterID: "CLUSTER-1",
				},
				NodeMutable: models.NodeMutable{
					Service: &models.NuvoService{
						ServiceState: models.ServiceState{
							State: "RUNNING",
						},
					},
					Name:        "node1",
					Description: "node1 object",
					Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
					LocalStorage: map[string]models.NodeStorageDevice{
						"uuid1": models.NodeStorageDevice{
							DeviceName:      "/dev/xvdb",
							DeviceState:     "UNUSED",
							DeviceType:      "HDD",
							SizeBytes:       swag.Int64(800165027840),
							UsableSizeBytes: swag.Int64(4096),
						},
						"uuid2": models.NodeStorageDevice{
							DeviceName:      "/dev/xvdc",
							DeviceState:     "CACHE",
							DeviceType:      "SSD",
							SizeBytes:       swag.Int64(800165027840),
							UsableSizeBytes: swag.Int64(8192),
						},
					},
					NodeAttributes: map[string]models.ValueType{
						"something": models.ValueType{Kind: "STRING", Value: "good"},
						"smells":    models.ValueType{Kind: "STRING", Value: "bad"},
						"awful":     models.ValueType{Kind: "STRING", Value: "ugly"},
					},
				},
			},
			&models.Node{
				NodeAllOf0: models.NodeAllOf0{
					Meta: &models.ObjMeta{
						ID:           "NODE-2",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
					AccountID: "tid1",
					ClusterID: "CLUSTER-1",
				},
				NodeMutable: models.NodeMutable{
					Service: &models.NuvoService{
						ServiceState: models.ServiceState{
							State: "RUNNING",
						},
					},
					Name:        "node2",
					Description: "node2 object",
					Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
					NodeAttributes: map[string]models.ValueType{
						"no": models.ValueType{Kind: "STRING", Value: "joke"},
					},
				},
			},
		},
	}
	aM := mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	aRes := &account.AccountListOK{
		Payload: []*models.Account{
			&models.Account{
				AccountAllOf0: models.AccountAllOf0{
					Meta: &models.ObjMeta{
						ID: "aid1",
					},
					TenantAccountID: "tid1",
				},
			},
			&models.Account{
				AccountAllOf0: models.AccountAllOf0{
					Meta: &models.ObjMeta{
						ID: "tid1",
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
					CspDomainID: models.ObjIDMutable("CSP-DOMAIN-1"),
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
	domM := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
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

	// list, all defaults
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	t.Log("*** list all defaults")
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(cOps)
	cOps.EXPECT().NodeList(m).Return(res, nil)
	domOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domM).Return(resCspDomains, nil)
	clOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clM).Return(clRes, nil)
	aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(aRes, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initNode()
	err := parseAndRun([]string{"node", "list"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(nodeDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))
	assert.Contains(te.tableData[0], "cluster1")
	assert.NotContains(te.tableData[0], "CLUSTER-1")

	// list default, json
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domM).Return(resCspDomains, nil)
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clM).Return(clRes, nil)
	cOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(cOps)
	cOps.EXPECT().NodeList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initNode()
	err = parseAndRun([]string{"node", "list", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(res.Payload, te.jsonData)

	// list default, yaml
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("*** list yaml")
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domM).Return(resCspDomains, nil)
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clM).Return(clRes, nil)
	cOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(cOps)
	cOps.EXPECT().NodeList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initNode()
	err = parseAndRun([]string{"node", "list", "-o", "yaml"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(res.Payload, te.yamlData)

	// list with columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("*** list with columns")
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(cOps)
	cOps.EXPECT().NodeList(m).Return(res, nil)
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domM).Return(resCspDomains, nil)
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clM).Return(clRes, nil)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(aRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initNode()
	err = parseAndRun([]string{"node", "list", "--columns", "Name,ID,LocalStorage"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, 3)
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list with filters, including cluster name
	mockCtrl.Finish()
	t.Log("*** list with filters")
	params.Name = swag.String("Name")
	params.Tags = []string{"tag"}
	params.ClusterID = swag.String("CLUSTER-1")
	dat1 := "2019-10-29T15:54:51Z"
	dat1v, err := time.Parse(time.RFC3339, dat1)
	assert.NoError(err)
	dat1Dt := strfmt.DateTime(dat1v)
	dat2 := "2018-10-29T15:54:51Z"
	dat2v, err := time.Parse(time.RFC3339, dat2)
	assert.NoError(err)
	dat2Dt := strfmt.DateTime(dat2v)
	params.ServiceHeartbeatTimeGE = &dat1Dt
	params.ServiceHeartbeatTimeLE = &dat2Dt
	params.ServiceStateEQ = swag.String("READY")
	params.ServiceStateNE = swag.String("UNKNOWN")
	params.NodeIds = []string{"nid1"}
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(cOps)
	cOps.EXPECT().NodeList(m).Return(res, nil)
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domM).Return(resCspDomains, nil)
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clM).Return(clRes, nil)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(aRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initNode()
	err = parseAndRun([]string{"node", "list", "-n", "Name", "-t", "tag", "-D", "domainName", "-C", "cluster1",
		"--svc-hb-time-ge", dat1, "--svc-hb-time-le", dat2, "--svc-state-eq", "READY", "--svc-state-ne", "UNKNOWN", "--id", "nid1"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(nodeDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))
	params.Name = swag.String("") // reset
	params.Tags = nil
	params.ClusterID = nil
	params.ServiceHeartbeatTimeGE = nil
	params.ServiceHeartbeatTimeLE = nil
	params.ServiceStateEQ = nil
	params.ServiceStateNE = nil
	params.NodeIds = nil

	// list with follow
	fw := &fakeWatcher{t: t}
	appCtx.watcher = fw
	fw.CallCBCount = 1
	fos := &fakeOSExecutor{}
	appCtx.osExec = fos
	nodeListCmdRunCacheThreshold = 0 // force a cache refresh
	defer func() { nodeListCmdRunCacheThreshold = 1 }()
	mockCtrl.Finish()
	t.Log("*** list with follow")
	params.ClusterID = swag.String("CLUSTER-1")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(cOps)
	cOps.EXPECT().NodeList(m).Return(res, nil)
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps).MinTimes(1)
	domOps.EXPECT().CspDomainList(domM).Return(resCspDomains, nil).MinTimes(1)
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps).MinTimes(1)
	clOps.EXPECT().ClusterList(clM).Return(clRes, nil).MinTimes(1)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps).MinTimes(1)
	aOps.EXPECT().AccountList(aM).Return(aRes, nil).MinTimes(1)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initNode()
	err = parseAndRun([]string{"node", "list", "-f", "-D", "domainName", "-C", "cluster1"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(nodeDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))
	params.ClusterID = nil
	expWArgs := &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{
				URIPattern:   "/nodes/?",
				ScopePattern: ".*clusterId:CLUSTER-1",
			},
		},
	}
	assert.NotNil(fw.InArgs)
	assert.Equal(expWArgs, fw.InArgs)
	assert.NotNil(fw.InCB)
	assert.Equal(1, fw.NumCalls)
	assert.NoError(fw.CBErr)

	// list with cluster name, not found
	mockCtrl.Finish()
	t.Log("*** list with cluster not found")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domM).Return(resCspDomains, nil)
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clM).Return(clRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initNode()
	err = parseAndRun([]string{"node", "list", "-D", "domainName", "-C", "fooBar"})
	assert.NotNil(err)
	assert.Regexp("fooBar.*not found", err)

	// list with invalid columns
	mockCtrl.Finish()
	t.Log("*** list with invalid columns")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initNode()
	err = parseAndRun([]string{"node", "list", "--columns", "Name,ID,Foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// list, API failure model error
	mockCtrl.Finish()
	t.Log("*** list api error, id")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domM).Return(resCspDomains, nil)
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clM).Return(clRes, nil)
	cOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(cOps)
	apiErr := &node.NodeListDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	params.NodeIdentifier = swag.String("node-1")
	cOps.EXPECT().NodeList(m).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initNode()
	err = parseAndRun([]string{"node", "list", "-I", "node-1"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// list, API failure arbitrary error
	mockCtrl.Finish()
	t.Log("*** list api failure")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domM).Return(resCspDomains, nil)
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clM).Return(clRes, nil)
	cOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(cOps)
	otherErr := fmt.Errorf("OTHER ERROR")
	params.NodeIdentifier = nil
	cOps.EXPECT().NodeList(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initNode()
	err = parseAndRun([]string{"node", "list"})
	assert.NotNil(err)
	assert.Equal(otherErr, err)

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
	initNode()
	err = parseAndRun([]string{"node", "list", "-A", "System"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initNode()
	err = parseAndRun([]string{"node", "list", "-n", "Name", "-t", "tag", "-D", "domainName", "-C", "cluster1",
		"--svc-hb-time-ge", dat1, "--svc-hb-time-le", dat2, "--svc-state-eq", "READY", "--svc-state-ne", "UNKNOWN", "--id", "nid1", "nid2"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestNodeDelete(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	lRes := &node.NodeListOK{
		Payload: []*models.Node{
			&models.Node{
				NodeAllOf0: models.NodeAllOf0{
					Meta: &models.ObjMeta{
						ID:           "NODE-1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
					ClusterID: "CLUSTER-1",
				},
				NodeMutable: models.NodeMutable{
					Service: &models.NuvoService{
						ServiceState: models.ServiceState{
							State: "RUNNING",
						},
					},
					Name:        "node1",
					Description: "node1 object",
					Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
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
					CspDomainID: models.ObjIDMutable("CSP-DOMAIN-1"),
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
	domM := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
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
	lParams := node.NewNodeListParams()
	m := mockmgmtclient.NewNodeMatcher(t, lParams)
	dParams := node.NewNodeDeleteParams()
	dParams.ID = string(lRes.Payload[0].Meta.ID)
	dRet := &node.NodeDeleteNoContent{}
	// delete
	mockCtrl := gomock.NewController(t)
	t.Log("*** delete ok")
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(cOps).MinTimes(1)
	cOps.EXPECT().NodeList(m).Return(lRes, nil)
	cOps.EXPECT().NodeDelete(dParams).Return(dRet, nil)
	domOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domM).Return(resCspDomains, nil)
	clOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps).MinTimes(1)
	clOps.EXPECT().ClusterList(clM).Return(clRes, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initNode()
	err := parseAndRun([]string{"node", "delete", "-n", "node1", "-D", "domainName", "-C", "cluster1", "--confirm"})
	assert.Nil(err)

	// delete, --confirm not specified
	mockCtrl.Finish()
	t.Log("*** delete no confirm")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initNode()
	err = parseAndRun([]string{"node", "delete", "-n", "node1", "-D", "domainName", "-C", "cluster1"})
	assert.NotNil(err)
	assert.Regexp("--confirm", err.Error())

	// delete, node/cluster/domain not found
	emptyRes := &node.NodeListOK{
		Payload: []*models.Node{},
	}
	mockCtrl.Finish()
	t.Log("*** delete object not found")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(cOps).MinTimes(1)
	cOps.EXPECT().NodeList(m).Return(emptyRes, nil)
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domM).Return(resCspDomains, nil)
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps).MinTimes(1)
	clOps.EXPECT().ClusterList(clM).Return(clRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initNode()
	err = parseAndRun([]string{"node", "delete", "-n", "node1", "-D", "domainName", "-C", "cluster1", "--confirm"})
	assert.NotNil(err)
	assert.Regexp("node1.*not found", err.Error())

	// delete, API model error
	apiErr := &node.NodeDeleteDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(cOps).MinTimes(1)
	cOps.EXPECT().NodeList(m).Return(lRes, nil)
	cOps.EXPECT().NodeDelete(dParams).Return(nil, apiErr)
	domOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domM).Return(resCspDomains, nil)
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps).MinTimes(1)
	clOps.EXPECT().ClusterList(clM).Return(clRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initNode()
	err = parseAndRun([]string{"node", "delete", "-n", "node1", "-D", "domainName", "-C", "cluster1", "--confirm"})
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
	initNode()
	err = parseAndRun([]string{"node", "delete", "-A", "System", "-n", "node1", "-D", "domainName", "-C", "cluster1", "--confirm"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initNode()
	err = parseAndRun([]string{"node", "delete", "-n", "node1", "-D", "domainName", "-C", "cluster1", "--confirm", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestNodeGet(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	objToFetch := "NODE-1"
	fParams := node.NewNodeFetchParams()
	fParams.ID = objToFetch

	now := time.Now()
	fRet := &node.NodeFetchOK{
		Payload: &models.Node{
			NodeAllOf0: models.NodeAllOf0{
				Meta: &models.ObjMeta{
					ID:           "NODE-1",
					Version:      1,
					TimeCreated:  strfmt.DateTime(now),
					TimeModified: strfmt.DateTime(now),
				},
				ClusterID: "CLUSTER-1",
			},
			NodeMutable: models.NodeMutable{
				Service: &models.NuvoService{
					ServiceState: models.ServiceState{
						State: "RUNNING",
					},
				},
				Name:        "node1",
				Description: "node1 object",
				Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
			},
		},
	}

	aRes := &account.AccountListOK{
		Payload: []*models.Account{
			&models.Account{
				AccountAllOf0: models.AccountAllOf0{
					Meta: &models.ObjMeta{
						ID: "aid1",
					},
					TenantAccountID: "tid1",
				},
			},
			&models.Account{
				AccountAllOf0: models.AccountAllOf0{
					Meta: &models.ObjMeta{
						ID: "tid1",
					},
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

	// fetch
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(cOps).MinTimes(1)
	mF := mockmgmtclient.NewNodeMatcher(t, fParams)
	cOps.EXPECT().NodeFetch(mF).Return(fRet, nil)
	domMatcher := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	domOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(domOps)
	domOps.EXPECT().CspDomainList(domMatcher).Return(resCspDomains, nil)
	clMatcher := mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(clMatcher).Return(resClusters, nil)
	aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())).Return(aRes, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initNode()
	err := parseAndRun([]string{"node", "get", objToFetch})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(nodeDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, 1)

	// apiError
	apiErr := &node.NodeFetchDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(cOps).MinTimes(1)
	mF = mockmgmtclient.NewNodeMatcher(t, fParams)
	cOps.EXPECT().NodeFetch(mF).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initNode()
	err = parseAndRun([]string{"node", "get", "--id", objToFetch})
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
	initNode()
	err = parseAndRun([]string{"node", "get", "-A", "System", "--id", objToFetch})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// invalid columns
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initNode()
	err = parseAndRun([]string{"node", "get", "--id", objToFetch, "--columns", "foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	t.Log("expected id")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initNode()
	err = parseAndRun([]string{"node", "get"})
	assert.NotNil(err)
	assert.Regexp("expected --id flag or a single identifier argument", err)
	appCtx.Account, appCtx.AccountID = "", ""
}
