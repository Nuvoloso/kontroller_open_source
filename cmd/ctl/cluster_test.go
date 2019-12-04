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
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/cluster"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

var clusterOpStateString = "Cluster state:\n" +
	"Iteration:1 #Nodes:2 #Storage:2 #SR:2\n" +
	"- Node[NODE-1] 'Node1' READY #Attached: 2\n" +
	"   Storage[STORAGE-1] SP:[SP-GP2] T:'Amazon GP2' TSz:3GiB ASz:0B PSz:0B TP:0 AP:? DS:UNUSED MS:FORMATTED AS:ATTACHED D:/dev/xvdba\n" +
	"   Storage[STORAGE-2] SP:[SP-SSD] T:'Amazon SSD' TSz:10GiB ASz:8GiB PSz:512MiB TP:0 AP:16 DS:UNUSED MS:FORMATTED AS:ATTACHED D:/dev/xvdbb\n" +
	"- Node[NODE-2] 'Node2' READY #Attached: 0\n" +
	"- SR[SR-1] SP:[SP-SSD] N:'Node1' T:'Amazon SSD' RB:0B #Claims:1\n" +
	"   VSR[VSR-1] 99B\n" +
	"- SR[SR-MUST-FETCH] SP:[SP-GP2] N:[NODE-FOO] T:'Amazon GP2' RB:0B #Claims:1\n" +
	"   VSR[VSR-2] 10B\n"
var csExtraLines = 8

func TestClusterMakeRecord(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	ac := &clusterCmd{}
	ac.accounts = map[string]ctxIDName{
		"accountID": {id: "", name: "my tenant"},
		"subId1":    {id: "accountID", name: "sub 1"},
		"subId2":    {id: "accountID", name: "sub 2"},
	}
	ac.cspDomains = map[string]string{
		"CSP-DOMAIN-1": "MyCSP Domain",
	}
	o := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID:           "CLUSTER-1",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
		},
		ClusterCreateOnce: models.ClusterCreateOnce{
			AccountID:   "accountID",
			ClusterType: "kubernetes",
			CspDomainID: models.ObjIDMutable("CSP-DOMAIN-1"),
		},
		ClusterMutable: models.ClusterMutable{
			ClusterMutableAllOf0: models.ClusterMutableAllOf0{
				ClusterVersion: "cluster-version-1",
				Service: &models.NuvoService{
					NuvoServiceAllOf0: models.NuvoServiceAllOf0{
						ServiceIP:      "1.2.3.4",
						ServiceLocator: "service-pid",
						ServiceAttributes: map[string]models.ValueType{
							common.ServiceAttrClusterResourceState: models.ValueType{Kind: common.ValueTypeString, Value: clusterOpStateString},
						},
					},
					ServiceState: models.ServiceState{
						State: "RUNNING",
					},
				},
			},
			ClusterCreateMutable: models.ClusterCreateMutable{
				AuthorizedAccounts: []models.ObjIDMutable{"subId1", "subId2"},
				Name:               "cluster1",
				Description:        "cluster1 object",
				Tags:               models.ObjTags{"tag1", "tag2", "tag3"},
				ClusterAttributes: map[string]models.ValueType{
					"something": models.ValueType{Kind: "STRING", Value: "good"},
					"smells":    models.ValueType{Kind: "STRING", Value: "bad"},
					"awfully":   models.ValueType{Kind: "STRING", Value: "ugly"},
					"nice":      models.ValueType{},
				},
				ClusterIdentifier: "CLUSTER-UUID-1",
				State:             "MANAGED",
			},
		},
	}
	rec := ac.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(clusterHeaders))
	assert.Equal("CLUSTER-1", rec[hID])
	assert.Equal("1", rec[hVersion])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeCreated])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeModified])
	assert.Equal("my tenant", rec[hAccount])
	assert.Equal("sub 1, sub 2", rec[hAuthorizedAccounts])
	assert.Equal("CLUSTER-UUID-1", rec[hClusterIdentifier])
	assert.Equal("kubernetes", rec[hClusterType])
	assert.Equal("MyCSP Domain", rec[hCspDomain])
	assert.Equal("cluster-version-1", rec[hClusterVersion])
	assert.Equal("RUNNING", rec[hControllerState])
	assert.Equal("cluster1", rec[hName])
	assert.Equal("cluster1 object", rec[hDescription])
	assert.Equal("tag1, tag2, tag3", rec[hTags])
	assert.Equal("MANAGED/RUNNING", rec[hClusterState])
	// note sorted order of attribute name
	al := "awfully[S]: ugly\nnice[]: \nsmells[S]: bad\nsomething[S]: good"
	assert.Equal(al, rec[hClusterAttributes])

	// no caches
	ac.accounts = map[string]ctxIDName{}
	ac.cspDomains = map[string]string{}
	rec = ac.makeRecord(o)
	t.Log(rec)
	assert.Equal("accountID", rec[hAccount])
	assert.Equal("subId1, subId2", rec[hAuthorizedAccounts])
	assert.Equal("CSP-DOMAIN-1", rec[hCspDomain])
}

func TestClusterCreate(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	resAccounts := &account.AccountListOK{
		Payload: []*models.Account{
			&models.Account{
				AccountAllOf0:  models.AccountAllOf0{Meta: &models.ObjMeta{ID: "tenantAID"}},
				AccountMutable: models.AccountMutable{Name: "Nuvoloso"},
			},
			&models.Account{
				AccountAllOf0:  models.AccountAllOf0{Meta: &models.ObjMeta{ID: "subAID1"}, TenantAccountID: "tenantAID"},
				AccountMutable: models.AccountMutable{Name: "sub 1"},
			},
			&models.Account{
				AccountAllOf0:  models.AccountAllOf0{Meta: &models.ObjMeta{ID: "subAID2"}, TenantAccountID: "tenantAID"},
				AccountMutable: models.AccountMutable{Name: "sub 2"},
			},
		},
	}
	res := &cluster.ClusterCreateCreated{
		Payload: &models.Cluster{
			ClusterAllOf0: models.ClusterAllOf0{
				Meta: &models.ObjMeta{
					ID:           "id2",
					Version:      1,
					TimeCreated:  strfmt.DateTime(now),
					TimeModified: strfmt.DateTime(now),
				},
			},
			ClusterCreateOnce: models.ClusterCreateOnce{
				AccountID:   "tenantAID",
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
					AuthorizedAccounts: []models.ObjIDMutable{"subAID1"},
					Name:               "cluster2",
					Description:        "cluster2 object",
					Tags:               models.ObjTags{"tag1", "tag2"},
				},
			},
		},
	}
	clArgs := &models.ClusterCreateArgs{
		ClusterCreateOnce: models.ClusterCreateOnce{
			ClusterType: res.Payload.ClusterType,
			CspDomainID: res.Payload.CspDomainID,
		},
		ClusterCreateMutable: models.ClusterCreateMutable{
			AuthorizedAccounts: res.Payload.AuthorizedAccounts,
			Description:        res.Payload.Description,
			Name:               res.Payload.Name,
			Tags:               res.Payload.Tags,
		},
	}
	params := cluster.NewClusterCreateParams()
	params.Payload = clArgs

	m := mockmgmtclient.NewClusterMatcher(t, params)
	aM := mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	cdM := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())

	// create (success)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "Nuvoloso").Return("tenantAID", nil)
	cOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps)
	cOps.EXPECT().ClusterCreate(m).Return(res, nil)
	aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	cdOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(resCspDomains, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err := parseAndRun([]string{"cluster", "-A", "Nuvoloso", "create",
		"-D", "domainName", "-T", string(params.Payload.ClusterType),
		"-n", string(params.Payload.Name), "-d", string(params.Payload.Description), "-Z", "sub 1",
		"-t", "tag1", "-t", "tag2", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.Cluster{res.Payload}, te.jsonData)

	// create, invalid columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "create", "-n", string(params.Payload.Name),
		"-D", "domainName", "-T", string(params.Payload.ClusterType),
		"-n", string(params.Payload.Name), "-d", string(params.Payload.Description), "-Z", "sub 1",
		"-t", "tag1", "-t", "tag2", "--columns", "Name,ID,FOO"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// domain caching fails
	mockCtrl.Finish()
	dlErr := fmt.Errorf("DOMAIN LIST FAILURE")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cdOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(nil, dlErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "-A", "Nuvoloso", "create",
		"-D", "domainName", "-T", string(params.Payload.ClusterType),
		"-n", string(params.Payload.Name), "-d", string(params.Payload.Description), "-Z", "sub 1",
		"-t", "tag1", "-t", "tag2", "-o", "json"})
	assert.NotNil(err)
	assert.Equal(dlErr, err)

	// domain name not found
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cdOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(resCspDomains, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "-A", "Nuvoloso", "create",
		"-D", "BAD_DOMAIN", "-T", string(params.Payload.ClusterType),
		"-n", string(params.Payload.Name), "-d", string(params.Payload.Description), "-Z", "sub 1",
		"-t", "tag1", "-t", "tag2", "-o", "json"})
	assert.NotNil(err)
	assert.Regexp("BAD_DOMAIN.*not found", err)

	// create, API failure model error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps)
	apiErr := &cluster.ClusterCreateDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	cOps.EXPECT().ClusterCreate(m).Return(nil, apiErr)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	cdOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(resCspDomains, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "-A", "Nuvoloso", "create",
		"-D", "domainName", "-T", string(params.Payload.ClusterType),
		"-n", string(params.Payload.Name), "-d", string(params.Payload.Description), "-Z", "sub 1",
		"-t", "tag1", "-t", "tag2", "-o", "json"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// create, API failure arbitrary error, valid owner-auth
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "Nuvoloso").Return("tenantAID", nil)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps)
	otherErr := fmt.Errorf("OTHER ERROR")
	params.Payload.AuthorizedAccounts = []models.ObjIDMutable{"tenantAID", "subAID1"}
	cOps.EXPECT().ClusterCreate(m).Return(nil, otherErr)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	cdOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(resCspDomains, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "-A", "Nuvoloso", "create",
		"-D", "domainName", "-T", string(params.Payload.ClusterType),
		"-n", string(params.Payload.Name), "-d", string(params.Payload.Description), "-Z", "sub 1", "--owner-auth",
		"-t", "tag1", "-t", "tag2", "-o", "json"})
	assert.NotNil(err)
	assert.Equal(otherErr, err)
	params.Payload.AuthorizedAccounts = []models.ObjIDMutable{"subAID1"}

	// validateAccount failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	cdOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(resCspDomains, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "-A", "Nuvoloso", "create",
		"-D", "domainName", "-T", string(params.Payload.ClusterType),
		"-n", string(params.Payload.Name), "-d", string(params.Payload.Description), "-Z", "UNKNOWN",
		"-t", "tag1", "-t", "tag2", "-o", "json"})
	assert.NotNil(err)
	assert.Regexp("authorized account.*not found", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("init context failure")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "Nuvoloso").Return("", fmt.Errorf("ctx error"))
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "create", "-n", string(params.Payload.Name), "-A", "Nuvoloso",
		"-D", "domainName", "-T", string(params.Payload.ClusterType),
		"-d", string(params.Payload.Description), "-Z", "sub 1",
		"-t", "tag1", "-t", "tag2"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("bad owner-auth")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "create", "-D", "domainName", "-n", string(params.Payload.Name), "--owner-auth"})
	assert.Regexp("owner-auth requires --account", err)

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "create", "-n", string(params.Payload.Name),
		"-D", "domainName", "-T", string(params.Payload.ClusterType),
		"-d", string(params.Payload.Description), "-Z", "sub 1",
		"-t", "tag1", "-t", "tag2", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestClusterList(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	params := cluster.NewClusterListParams()
	params.Name = swag.String("")
	params.ClusterIdentifier = swag.String("")
	m := mockmgmtclient.NewClusterMatcher(t, params)
	res := &cluster.ClusterListOK{
		Payload: []*models.Cluster{
			&models.Cluster{
				ClusterAllOf0: models.ClusterAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				ClusterCreateOnce: models.ClusterCreateOnce{
					AccountID:   "accountID",
					ClusterType: "kubernetes",
					CspDomainID: models.ObjIDMutable("CSP-DOMAIN-1"),
				},
				ClusterMutable: models.ClusterMutable{
					ClusterMutableAllOf0: models.ClusterMutableAllOf0{
						ClusterVersion: "cluster-version-1",
						Service: &models.NuvoService{
							NuvoServiceAllOf0: models.NuvoServiceAllOf0{
								ServiceIP:      "1.2.3.4",
								ServiceLocator: "service-pid",
								ServiceAttributes: map[string]models.ValueType{
									common.ServiceAttrClusterResourceState: models.ValueType{Kind: common.ValueTypeString, Value: clusterOpStateString},
								},
							},
							ServiceState: models.ServiceState{
								State: "RUNNING",
							},
						},
					},
					ClusterCreateMutable: models.ClusterCreateMutable{
						AuthorizedAccounts: []models.ObjIDMutable{"accountID"},
						Name:               "cluster1",
						Description:        "cluster1 object",
						Tags:               models.ObjTags{"tag1", "tag2", "tag3"},
						ClusterAttributes: map[string]models.ValueType{
							"something": models.ValueType{Kind: "STRING", Value: "good"},
							"smells":    models.ValueType{Kind: "STRING", Value: "bad"},
							"awful":     models.ValueType{Kind: "STRING", Value: "ugly"},
						},
						ClusterIdentifier: "CLUSTER-ID-1",
						State:             "ACTIVE",
					},
				},
			},
			&models.Cluster{
				ClusterAllOf0: models.ClusterAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id2",
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
						Name:        "cluster2",
						Description: "cluster2 object",
						Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
						ClusterAttributes: map[string]models.ValueType{
							"no": models.ValueType{Kind: "STRING", Value: "joke"},
						},
						ClusterIdentifier: "CLUSTER-ID-2",
						State:             "ACTIVE",
					},
				},
			},
		},
	}
	aM := mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	cdM := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	// list, all defaults
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps)
	cOps.EXPECT().ClusterList(m).Return(res, nil)
	aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	cdOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(resCspDomains, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err := parseAndRun([]string{"cluster", "list"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(clusterDefaultHeaders))
	for _, r := range te.tableData {
		t.Log(r)
	}
	assert.Len(te.tableData, len(res.Payload)+1*csExtraLines)
	assert.Contains(te.tableData[0], "domainName")
	assert.NotContains(te.tableData[0], "CSP-DOMAIN-1")

	// list default, json
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps)
	cOps.EXPECT().ClusterList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "list", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(res.Payload, te.jsonData)

	// list default, yaml
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps)
	cOps.EXPECT().ClusterList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "list", "-o", "yaml"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(res.Payload, te.yamlData)

	// list with follow
	fw := &fakeWatcher{t: t}
	appCtx.watcher = fw
	fw.CallCBCount = 1
	fos := &fakeOSExecutor{}
	appCtx.osExec = fos
	clusterListCmdRunCacheThreshold = 0 // force a cache refresh
	defer func() { clusterListCmdRunCacheThreshold = 1 }()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	cdOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(resCspDomains, nil)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps)
	cOps.EXPECT().ClusterList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "list", "-f"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(clusterDefaultHeaders))
	assert.Len(te.tableData, len(res.Payload)+1*csExtraLines)

	// list with columns, valid authorized account
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps)
	params.AuthorizedAccountID = swag.String("authAccountID")
	cOps.EXPECT().ClusterList(m).Return(res, nil)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	cdOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(resCspDomains, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "list", "-Z", "AuthAccount", "--columns", "Name,ID"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, 2)
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))
	params.AuthorizedAccountID = nil

	// list succeeds even if caching fails (no domain name filter), specify context account, owner-auth
	mockCtrl.Finish()
	dlErr := fmt.Errorf("DOMAIN LIST FAILURE")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "Nuvoloso").Return("tenantAID", nil)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps)
	params.AuthorizedAccountID = swag.String("tenantAID")
	cOps.EXPECT().ClusterList(m).Return(res, nil)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(nil, dlErr)
	cdOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(nil, dlErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "list", "-A", "Nuvoloso", "--owner-auth"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(clusterDefaultHeaders))
	for _, r := range te.tableData {
		t.Log(r)
	}
	assert.Len(te.tableData, len(res.Payload)+1*csExtraLines)
	assert.NotContains(te.tableData[0], "System")
	assert.Contains(te.tableData[0], "accountID")
	assert.NotContains(te.tableData[0], "domainName")
	assert.Contains(te.tableData[0], "CSP-DOMAIN-1")
	params.AuthorizedAccountID = nil
	appCtx.Account, appCtx.AccountID = "", ""

	// list with filters, including domain name
	mockCtrl.Finish()
	params.Name = swag.String("Name")
	params.Tags = []string{"tag"}
	params.CspDomainID = swag.String("CSP-DOMAIN-1")
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
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps)
	cOps.EXPECT().ClusterList(m).Return(res, nil)
	cdOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(resCspDomains, nil)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "list", "-n", "Name", "-t", "tag", "-D", "domainName",
		"--svc-hb-time-ge", dat1, "--svc-hb-time-le", dat2, "--svc-state-eq", "READY", "--svc-state-ne", "UNKNOWN"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(clusterDefaultHeaders))
	for _, r := range te.tableData {
		t.Log(r)
	}
	assert.Len(te.tableData, len(res.Payload)+1*csExtraLines)
	params.Name = swag.String("") // reset
	params.Tags = nil
	params.CspDomainID = nil
	params.ServiceHeartbeatTimeGE = nil
	params.ServiceHeartbeatTimeLE = nil
	params.ServiceStateEQ = nil
	params.ServiceStateNE = nil

	// list with domain name filter, caching fails
	mockCtrl.Finish()
	dlErr = fmt.Errorf("DOMAIN LIST FAILURE")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cdOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(nil, dlErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "list", "-D", "domainName"})
	assert.NotNil(err)
	assert.Equal(dlErr, err)

	// list with domain name, not found
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cdOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(resCspDomains, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "list", "-D", "fooBar"})
	assert.NotNil(err)
	assert.Regexp("fooBar.*not found", err)

	// list with invalid columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "list", "-c", "Name,ID,Foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// list, API failure model error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps)
	apiErr := &cluster.ClusterListDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	cOps.EXPECT().ClusterList(m).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "list"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// list, API failure arbitrary error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps)
	otherErr := fmt.Errorf("OTHER ERROR")
	cOps.EXPECT().ClusterList(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "list"})
	assert.NotNil(err)
	assert.Equal(otherErr, err)

	// list invalid authorized account
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "list", "-Z", "sub"})
	assert.Regexp("authorized account .* not found", err)
	appCtx.Account, appCtx.AccountID = "", ""

	// list with both authorized-account and owner-auth
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "list", "-Z", "sub", "--owner-auth"})
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
	initCluster()
	err = parseAndRun([]string{"cluster", "list", "--owner-auth"})
	assert.Regexp("--owner-auth requires --account", err)
	appCtx.Account, appCtx.AccountID = "", ""

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
	initCluster()
	err = parseAndRun([]string{"cluster", "list", "-A", "System"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "list", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestClusterDelete(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	lRes := &cluster.ClusterListOK{
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
	cdM := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	lParams := cluster.NewClusterListParams()
	lParams.Name = swag.String(string(lRes.Payload[0].Meta.ID))
	lParams.CspDomainID = swag.String(string(lRes.Payload[0].CspDomainID))
	m := mockmgmtclient.NewClusterMatcher(t, lParams)
	dParams := cluster.NewClusterDeleteParams()
	dParams.ID = string(lRes.Payload[0].Meta.ID)
	dRet := &cluster.ClusterDeleteNoContent{}

	// delete
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(1)
	cOps.EXPECT().ClusterList(m).Return(lRes, nil)
	cOps.EXPECT().ClusterDelete(dParams).Return(dRet, nil)
	cdOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(resCspDomains, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err := parseAndRun([]string{"cluster", "delete", "-D", "domainName", "-n", string(*lParams.Name), "--confirm"})
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
	initCluster()
	err = parseAndRun([]string{"cluster", "delete", "-D", "domainName", "-n", string(*lParams.Name)})
	assert.NotNil(err)
	assert.Regexp("--confirm", err.Error())

	// delete, not found
	emptyRes := &cluster.ClusterListOK{
		Payload: []*models.Cluster{},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(1)
	cOps.EXPECT().ClusterList(m).Return(emptyRes, nil)
	cdOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(resCspDomains, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "delete", "-D", "domainName", "-n", string(*lParams.Name), "--confirm"})
	assert.NotNil(err)
	assert.Regexp("not found", err.Error())

	// delete, list error
	otherErr := fmt.Errorf("other error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(1)
	cOps.EXPECT().ClusterList(m).Return(nil, otherErr)
	cdOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(resCspDomains, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "delete", "-D", "domainName", "-n", string(*lParams.Name), "--confirm"})
	assert.NotNil(err)
	assert.Regexp("other error", err.Error())

	// delete, API model error
	apiErr := &cluster.ClusterDeleteDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(1)
	cOps.EXPECT().ClusterList(m).Return(lRes, nil)
	cOps.EXPECT().ClusterDelete(dParams).Return(nil, apiErr)
	cdOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(resCspDomains, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "delete", "-D", "domainName", "-n", string(*lParams.Name), "--confirm"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// delete, API arbitrary error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(1)
	cOps.EXPECT().ClusterList(m).Return(lRes, nil)
	cOps.EXPECT().ClusterDelete(dParams).Return(nil, otherErr)
	cdOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(resCspDomains, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "delete", "-D", "domainName", "-n", string(*lParams.Name), "--confirm"})
	assert.NotNil(err)
	assert.Regexp("other error", err.Error())

	// delete, domain list fails
	mockCtrl.Finish()
	dlErr := fmt.Errorf("DOMAIN LIST FAILURE")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cdOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(nil, dlErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "delete", "-D", "domainName", "-n", string(*lParams.Name), "--confirm"})
	assert.NotNil(err)
	assert.Equal(dlErr, err)

	// delete, domain name not found
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	cdOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(resCspDomains, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "delete", "-D", "foobar", "-n", string(*lParams.Name), "--confirm"})
	assert.NotNil(err)
	assert.Regexp("CSP domain.*not found", err)

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
	initCluster()
	err = parseAndRun([]string{"cluster", "delete", "-A", "System", "-D", "foobar", "-n", string(*lParams.Name), "--confirm"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "delete", "-D", "domainName", "-n", string(*lParams.Name), "--confirm", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestClusterModify(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	resAccounts := &account.AccountListOK{
		Payload: []*models.Account{
			&models.Account{
				AccountAllOf0:  models.AccountAllOf0{Meta: &models.ObjMeta{ID: "tenantAID"}},
				AccountMutable: models.AccountMutable{Name: "Nuvoloso"},
			},
			&models.Account{
				AccountAllOf0:  models.AccountAllOf0{Meta: &models.ObjMeta{ID: "subAID1"}, TenantAccountID: "tenantAID"},
				AccountMutable: models.AccountMutable{Name: "sub 1"},
			},
			&models.Account{
				AccountAllOf0:  models.AccountAllOf0{Meta: &models.ObjMeta{ID: "subAID2"}, TenantAccountID: "tenantAID"},
				AccountMutable: models.AccountMutable{Name: "sub 2"},
			},
		},
	}
	lRes := &cluster.ClusterListOK{
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
					AccountID:   "tenantAID",
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
						ClusterUsagePolicy: &models.ClusterUsagePolicy{
							Inherited:                   true,
							AccountSecretScope:          "CLUSTER",
							VolumeDataRetentionOnDelete: "DELETE",
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
	cdM := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	lParams := cluster.NewClusterListParams()
	lParams.Name = swag.String(string(lRes.Payload[0].Meta.ID))
	lParams.CspDomainID = swag.String(string(lRes.Payload[0].CspDomainID))
	m := mockmgmtclient.NewClusterMatcher(t, lParams)

	// update (APPEND)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "Nuvoloso").Return("tenantAID", nil)
	aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	cdOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(resCspDomains, nil)
	cOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(1)
	cOps.EXPECT().ClusterList(m).Return(lRes, nil)
	uParams := cluster.NewClusterUpdateParams()
	uParams.Payload = &models.ClusterMutable{
		ClusterCreateMutable: models.ClusterCreateMutable{
			AuthorizedAccounts: []models.ObjIDMutable{"subAID1"},
			Name:               "newName",
			Description:        "new Description",
			Tags:               []string{"tag1", "tag2"},
		},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Set = []string{"name", "description"}
	uParams.Append = []string{"authorizedAccounts", "tags"}
	um := mockmgmtclient.NewClusterMatcher(t, uParams)
	uRet := cluster.NewClusterUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().ClusterUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err := parseAndRun([]string{"cluster", "-A", "Nuvoloso", "modify", "-D", "domainName", "-n", string(*lParams.Name),
		"-N", "newName", "-d", " new Description ", "-Z", "sub 1", "-t", "tag1", "-t", "tag2", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.Cluster{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// same but with SET, owner-auth
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "Nuvoloso").Return("tenantAID", nil)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	cdOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(resCspDomains, nil)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(1)
	cOps.EXPECT().ClusterList(m).Return(lRes, nil)
	uParams = cluster.NewClusterUpdateParams()
	uParams.Payload = &models.ClusterMutable{
		ClusterCreateMutable: models.ClusterCreateMutable{
			AuthorizedAccounts: []models.ObjIDMutable{"tenantAID", "subAID1"},
			Name:               "newName",
			Description:        "new Description",
			Tags:               []string{"tag1", "tag2"},
		},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"name", "description", "authorizedAccounts", "tags"}
	um = mockmgmtclient.NewClusterMatcher(t, uParams)
	uRet = cluster.NewClusterUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().ClusterUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "-A", "Nuvoloso", "modify", "-D", "domainName", "-n", string(*lParams.Name),
		"-N", "newName", "-d", " new Description ", "-Z", "sub 1", "--owner-auth", "--authorized-accounts-action=SET",
		"-t", "tag1", "-t", "tag2", "--tag-action", "SET", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.Cluster{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// same but with REMOVE
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "Nuvoloso").Return("tenantAID", nil)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	cdOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(resCspDomains, nil)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(1)
	cOps.EXPECT().ClusterList(m).Return(lRes, nil)
	uParams = cluster.NewClusterUpdateParams()
	uParams.Payload = &models.ClusterMutable{
		ClusterCreateMutable: models.ClusterCreateMutable{
			AuthorizedAccounts: []models.ObjIDMutable{"subAID1"},
			Name:               "newName",
			Description:        "new Description",
			Tags:               []string{"tag1", "tag2"},
		},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"name", "description"}
	uParams.Remove = []string{"authorizedAccounts", "tags"}
	um = mockmgmtclient.NewClusterMatcher(t, uParams)
	uRet = cluster.NewClusterUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().ClusterUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "-A", "Nuvoloso", "modify", "-D", "domainName", "-n", string(*lParams.Name),
		"-N", "newName", "-d", " new Description ", "-Z", "sub 1", "--authorized-accounts-action=REMOVE",
		"-t", "tag1", "-t", "tag2", "--tag-action", "REMOVE", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.Cluster{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// same but empty SET
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cdOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(resCspDomains, nil)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(1)
	cOps.EXPECT().ClusterList(m).Return(lRes, nil)
	uParams = cluster.NewClusterUpdateParams()
	uParams.Payload = &models.ClusterMutable{
		ClusterCreateMutable: models.ClusterCreateMutable{
			Name:        "newName",
			Description: "new Description",
		},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"name", "description", "authorizedAccounts", "tags"}
	um = mockmgmtclient.NewClusterMatcher(t, uParams)
	uRet = cluster.NewClusterUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().ClusterUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "modify", "-D", "domainName", "-n", string(*lParams.Name),
		"-N", "newName", "-d", " new Description ", "--authorized-accounts-action=SET",
		"--tag-action", "SET", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.Cluster{uRet.Payload}, te.jsonData)

	// inherit CUP
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cdOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(resCspDomains, nil)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(1)
	cOps.EXPECT().ClusterList(m).Return(lRes, nil)
	uParams = cluster.NewClusterUpdateParams()
	uParams.Payload = &models.ClusterMutable{
		ClusterMutableAllOf0: models.ClusterMutableAllOf0{
			ClusterUsagePolicy: &models.ClusterUsagePolicy{
				Inherited: true,
			},
		},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"clusterUsagePolicy"}
	um = mockmgmtclient.NewClusterMatcher(t, uParams)
	uRet = cluster.NewClusterUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().ClusterUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "modify", "-D", "domainName", "-n", string(*lParams.Name),
		"--cup-inherit", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.Cluster{uRet.Payload}, te.jsonData)

	// CUP modifications
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cdOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdOps.EXPECT().CspDomainList(cdM).Return(resCspDomains, nil)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(1)
	cOps.EXPECT().ClusterList(m).Return(lRes, nil)
	uParams = cluster.NewClusterUpdateParams()
	uParams.Payload = &models.ClusterMutable{
		ClusterMutableAllOf0: models.ClusterMutableAllOf0{
			ClusterUsagePolicy: &models.ClusterUsagePolicy{
				Inherited:                   false,
				AccountSecretScope:          "GLOBAL",
				ConsistencyGroupName:        "${cluster.name}",
				VolumeDataRetentionOnDelete: "DELETE",
			},
		},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"clusterUsagePolicy"}
	um = mockmgmtclient.NewClusterMatcher(t, uParams)
	uRet = cluster.NewClusterUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().ClusterUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "modify", "-D", "domainName", "-n", string(*lParams.Name),
		"--cup-account-secret-scope", "GLOBAL", "--cup-consistency-group-name", "${cluster.name}",
		"--cup-data-retention-on-delete", "DELETE", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.Cluster{uRet.Payload}, te.jsonData)

	mockCtrl.Finish()
	t.Log("owner-auth missing account")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "modify", "--owner-auth", "-D", "foobar", "-n", *lParams.Name})
	assert.Regexp("--owner-auth requires --account", err)
	appCtx.Account, appCtx.AccountID = "", ""

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
	initCluster()
	err = parseAndRun([]string{"cluster", "modify", "-A", "System", "-D", "foobar", "-n", string(*lParams.Name)})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// other error cases
	updateErr := cluster.NewClusterUpdateDefault(400)
	updateErr.Payload = &models.Error{Code: 400, Message: swag.String("update error")}
	otherErr := fmt.Errorf("other error")
	mNotNil := gomock.Not(gomock.Nil)
	errTCs := []struct {
		name      string
		args      []string
		re        string
		alRC      *account.AccountListOK
		clErr     error
		clRC      *cluster.ClusterListOK
		dlErr     error
		dlRC      *csp_domain.CspDomainListOK
		updateErr error
		noMock    bool
		noCL      bool
	}{
		{
			name: "No modifications",
			args: []string{"cluster", "modify", "-D", "domainName", "-n", string(*lParams.Name), "-V", "1", "-o", "json"},
			re:   "No modifications",
		},
		{
			name:      "Update default error",
			args:      []string{"cluster", "modify", "-D", "domainName", "-n", string(*lParams.Name), "-N", "NewName", "-V", "1", "-o", "json"},
			re:        "update error",
			updateErr: updateErr,
		},
		{
			name:      "Update other error",
			args:      []string{"cluster", "modify", "-D", "domainName", "-n", string(*lParams.Name), "-N", "NewName", "-V", "1", "-o", "json"},
			re:        "other error",
			updateErr: otherErr,
		},
		{
			name:   "invalid columns",
			args:   []string{"cluster", "modify", "-D", "domainName", "-n", string(*lParams.Name), "--columns", "ID,foo"},
			re:     "invalid column",
			noMock: true,
		},
		{
			name: "cluster not found",
			args: []string{"cluster", "modify", "-D", "domainName", "-n", string(*lParams.Name), "-N", "NewName", "-V", "1", "-o", "json"},
			re:   "Cluster.*not found",
			clRC: &cluster.ClusterListOK{Payload: []*models.Cluster{}},
		},
		{
			name:  "cluster list error",
			args:  []string{"cluster", "modify", "-D", "domainName", "-n", string(*lParams.Name), "-N", "NewName", "-V", "1", "-o", "json"},
			re:    "other error",
			clErr: otherErr,
		},
		{
			name: "domain not found",
			args: []string{"cluster", "modify", "-D", "domainName", "-n", string(*lParams.Name), "-N", "NewName", "-V", "1", "-o", "json"},
			re:   "domain.*not found",
			dlRC: &csp_domain.CspDomainListOK{Payload: []*models.CSPDomain{}},
			noCL: true,
		},
		{
			name:  "domain list error",
			args:  []string{"cluster", "modify", "-D", "domainName", "-n", string(*lParams.Name), "-N", "NewName", "-V", "1", "-o", "json"},
			re:    "other error",
			dlErr: otherErr,
			noCL:  true,
		},
		{
			name: "validateAccount fails",
			args: []string{"cluster", "modify", "-D", "domainName", "-n", string(*lParams.Name), "-Z", "other", "-V", "1", "-o", "json"},
			re:   "authorized account.*not found",
			alRC: resAccounts,
		},
		{
			name:   "CUP set and inherit (account secret scope)",
			args:   []string{"cluster", "modify", "-D", "domainName", "-n", string(*lParams.Name), "--cup-inherit", "-S", "GLOBAL", "-V", "1", "-o", "json"},
			re:     "set.*inherit.*cluster usage policy",
			noMock: true,
		},
		{
			name:   "CUP set and inherit (delete action)",
			args:   []string{"cluster", "modify", "-D", "domainName", "-n", string(*lParams.Name), "--cup-inherit", "--cup-data-retention-on-delete", "RETAIN", "-V", "1", "-o", "json"},
			re:     "set.*inherit.*cluster usage policy",
			noMock: true,
		},
	}
	for _, tc := range errTCs {
		t.Logf("case: %s", tc.name)
		if !tc.noMock {
			mockCtrl.Finish()
			mockCtrl = gomock.NewController(t)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			cdOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
			mAPI.EXPECT().CspDomain().Return(cdOps)
			if tc.dlErr != nil {
				cdOps.EXPECT().CspDomainList(cdM).Return(nil, tc.dlErr)
			} else {
				if tc.dlRC == nil {
					tc.dlRC = resCspDomains
				}
				cdOps.EXPECT().CspDomainList(cdM).Return(tc.dlRC, nil)
			}
			if tc.alRC != nil {
				aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
				mAPI.EXPECT().Account().Return(aOps)
				aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
			}
			if !tc.noCL {
				cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
				mAPI.EXPECT().Cluster().Return(cOps).MinTimes(1)
				if tc.clErr != nil {
					cOps.EXPECT().ClusterList(m).Return(nil, tc.clErr)
				} else {
					if tc.clRC == nil {
						tc.clRC = lRes
					}
					cOps.EXPECT().ClusterList(m).Return(tc.clRC, nil)
				}
				if tc.updateErr != nil {
					cOps.EXPECT().ClusterUpdate(mNotNil).Return(nil, tc.updateErr)
				}
			}
			appCtx.API = mAPI
		}
		te = &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initCluster()
		err = parseAndRun(tc.args)
		assert.NotNil(err)
		assert.Nil(te.jsonData)
		assert.Regexp(tc.re, err.Error())
		appCtx.Account, appCtx.AccountID = "", ""
	}

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "-A", "Nuvoloso", "modify", "-D", "domainName", "-n", string(*lParams.Name),
		"-N", "newName", "-d", " new Description ", "-Z", "sub 1", "-t", "tag1", "-t", "tag2", "o", "json"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestClusterSecretFetch(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	domName := string(resCspDomains.Payload[0].Name)
	domID := string(resCspDomains.Payload[0].Meta.ID)
	clName := string(resClusters.Payload[0].Name)
	clID := string(resClusters.Payload[0].Meta.ID)
	authAccountName := "AuthAccount"
	authAccountID := "authAccountID"

	outFile := "./account-secret.yaml"
	defer func() { os.Remove(outFile) }()

	t.Log("case: fetch for context account")
	appCtx.AccountID = authAccountID
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := cacheLoadHelper(t, mockCtrl, loadDomains)
	clOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps).MinTimes(1)
	cm := mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	cm.ListParam.CspDomainID = swag.String(domID)
	cm.ListParam.Name = swag.String(clName)
	clOps.EXPECT().ClusterList(cm).Return(resClusters, nil).MinTimes(1)
	asfParams := cluster.NewClusterAccountSecretFetchParams()
	asfParams.ID = clID
	asfParams.AuthorizedAccountID = appCtx.AccountID
	mA := mockmgmtclient.NewClusterMatcher(t, asfParams)
	asfRet := cluster.NewClusterAccountSecretFetchOK()
	asfRet.Payload = &models.ValueType{Kind: "SECRET", Value: "secret data"}
	clOps.EXPECT().ClusterAccountSecretFetch(mA).Return(asfRet, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err := parseAndRun([]string{"cluster", "get-secret", "-D", domName, "-n", clName})
	assert.Nil(err)
	resBytes, err := ioutil.ReadFile(outFile)
	assert.Nil(err)
	assert.Equal(asfRet.Payload.Value, string(resBytes))
	mockCtrl.Finish()
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("case: fetch for specified account")
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadAccounts, loadDomains)
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps).MinTimes(1)
	cm = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	cm.ListParam.CspDomainID = swag.String(domID)
	cm.ListParam.Name = swag.String(clName)
	clOps.EXPECT().ClusterList(cm).Return(resClusters, nil).MinTimes(1)
	asfParams = cluster.NewClusterAccountSecretFetchParams()
	asfParams.ID = clID
	asfParams.AuthorizedAccountID = authAccountID
	mA = mockmgmtclient.NewClusterMatcher(t, asfParams)
	asfRet = cluster.NewClusterAccountSecretFetchOK()
	asfRet.Payload = &models.ValueType{Kind: "SECRET", Value: "secret data"}
	clOps.EXPECT().ClusterAccountSecretFetch(mA).Return(asfRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "get-secret", "-D", domName, "-n", clName, "-Z", authAccountName})
	assert.Nil(err)
	resBytes, err = ioutil.ReadFile(outFile)
	assert.Nil(err)
	assert.Equal(asfRet.Payload.Value, string(resBytes))
	mockCtrl.Finish()
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("case: fail secret fetch, use stdout, k8sNamespace")
	appCtx.Account, appCtx.AccountID = "", authAccountID
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains)
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps).MinTimes(1)
	cm = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	cm.ListParam.CspDomainID = swag.String(domID)
	cm.ListParam.Name = swag.String(clName)
	clOps.EXPECT().ClusterList(cm).Return(resClusters, nil).MinTimes(1)
	asfParams = cluster.NewClusterAccountSecretFetchParams()
	asfParams.ID = clID
	asfParams.AuthorizedAccountID = authAccountID
	asfParams.K8sNamespace = swag.String("k8sNamespace")
	mA = mockmgmtclient.NewClusterMatcher(t, asfParams)
	asfErr := cluster.NewClusterAccountSecretFetchDefault(400)
	asfErr.Payload = &models.Error{Code: 400, Message: swag.String("asfErr")}
	clOps.EXPECT().ClusterAccountSecretFetch(mA).Return(nil, asfErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "get-secret", "-D", domName, "-n", clName, "--k8s-namespace", "k8sNamespace", "-O", "-"})
	assert.NotNil(err)
	assert.Regexp("asfErr", err)
	mockCtrl.Finish()
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("case: fail on specified account")
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadAccounts, loadDomains)
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps).MinTimes(1)
	cm = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	cm.ListParam.CspDomainID = swag.String(domID)
	cm.ListParam.Name = swag.String(clName)
	clOps.EXPECT().ClusterList(cm).Return(resClusters, nil).MinTimes(1)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "get-secret", "-D", domName, "-n", clName, "-Z", authAccountName + "foo"})
	assert.NotNil(err)
	assert.Regexp("foo", err)
	mockCtrl.Finish()
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("case: cluster not found")
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains)
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps).MinTimes(1)
	cm = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	cm.ListParam.CspDomainID = swag.String(domID)
	cm.ListParam.Name = swag.String(clName)
	resClEmpty := cluster.NewClusterListOK()
	resClEmpty.Payload = []*models.Cluster{}
	clOps.EXPECT().ClusterList(cm).Return(resClEmpty, nil).MinTimes(1)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "get-secret", "-D", domName, "-n", clName})
	assert.NotNil(err)
	assert.Regexp(clName, err)
	mockCtrl.Finish()
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("case: fail on cluster list")
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains)
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps).MinTimes(1)
	cm = mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	cm.ListParam.CspDomainID = swag.String(domID)
	cm.ListParam.Name = swag.String(clName)
	clOps.EXPECT().ClusterList(cm).Return(nil, fmt.Errorf("cluster-list")).MinTimes(1)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "get-secret", "-D", domName, "-n", clName})
	assert.NotNil(err)
	assert.Regexp("cluster-list", err)
	mockCtrl.Finish()
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("case: domain not found")
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "get-secret", "-D", domName + "foo", "-n", clName})
	assert.NotNil(err)
	assert.Regexp("foo", err)
	mockCtrl.Finish()
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("case: domain caching fails")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dm := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	dOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(dm).Return(nil, fmt.Errorf("dom-list")).MinTimes(1)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "get-secret", "-D", domName, "-n", clName})
	assert.NotNil(err)
	assert.Regexp("dom-list", err)
	mockCtrl.Finish()
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("case: invalid output file")
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "get-secret", "-D", domName, "-n", clName, "-O", "./bad/file"})
	assert.NotNil(err)
	assert.Regexp("no such file or directory", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("case: init context failure")
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
	initCluster()
	err = parseAndRun([]string{"cluster", "get-secret", "-A", "System", "-D", domName, "-n", clName})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "get-secret", "-D", domName, "-n", clName, "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestClusterGet(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	reqToFetch := "id1"
	fParams := cluster.NewClusterFetchParams()
	fParams.ID = reqToFetch

	now := time.Now()
	fRet := &cluster.ClusterFetchOK{
		Payload: &models.Cluster{
			ClusterAllOf0: models.ClusterAllOf0{
				Meta: &models.ObjMeta{
					ID:           "id2",
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
					Name:        "cluster2",
					Description: "cluster2 object",
					Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
					ClusterAttributes: map[string]models.ValueType{
						"no": models.ValueType{Kind: "STRING", Value: "joke"},
					},
					ClusterIdentifier: "CLUSTER-ID-2",
				},
			},
		},
	}
	var err error
	assert.NotNil(fRet)

	// fetch
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := cacheLoadHelper(t, mockCtrl, loadDomains, loadAccounts)
	cOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(1)
	mF := mockmgmtclient.NewClusterMatcher(t, fParams)
	cOps.EXPECT().ClusterFetch(mF).Return(fRet, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "get", reqToFetch})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(clusterDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, 1)

	// apiError
	apiErr := &cluster.ClusterFetchDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(1)
	mF = mockmgmtclient.NewClusterMatcher(t, fParams)
	cOps.EXPECT().ClusterFetch(mF).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "get", "--id", reqToFetch})
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
	initCluster()
	err = parseAndRun([]string{"cluster", "get", "-A", "System", "--id", reqToFetch})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// invalid columns
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "get", "--id", reqToFetch, "--columns", "foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	t.Log("expected id")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "get"})
	assert.NotNil(err)
	assert.Regexp("expected --id flag or a single identifier argument", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestClusterGetDeployment(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	lRes := &cluster.ClusterListOK{
		Payload: []*models.Cluster{
			&models.Cluster{
				ClusterAllOf0: models.ClusterAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id2",
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
						Name:        "cluster2",
						Description: "cluster2 object",
						Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
						ClusterAttributes: map[string]models.ValueType{
							"no": models.ValueType{Kind: "STRING", Value: "joke"},
						},
						ClusterIdentifier: "",
					},
				},
			},
		},
	}
	gdRes := &cluster.ClusterOrchestratorGetDeploymentOK{}
	gdRes.Payload = &models.ClusterOrchestratorGetDeploymentOKBody{}
	gdRes.Payload.Format = "yaml"
	gdRes.Payload.Deployment = "cluster deployment yaml"

	var err error

	gdParams := cluster.NewClusterOrchestratorGetDeploymentParams().WithID("id2").WithOrchestratorType(swag.String("")).WithOrchestratorVersion(swag.String(""))

	t.Log("success with data string and specified format")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := cacheLoadHelper(t, mockCtrl, loadDomains, loadNodes)
	cOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(1)
	cOps.EXPECT().ClusterList(gomock.Any()).Return(lRes, nil).MinTimes(1)
	cOps.EXPECT().ClusterOrchestratorGetDeployment(gdParams).Return(gdRes, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	outFile := "./nuvo-cluster.yaml"
	defer func() { os.Remove(outFile) }()
	err = parseAndRun([]string{"cluster", "get-deployment", "-D", "domainName", "-n", "cluster2"})
	assert.Nil(err)
	resBytes, err := ioutil.ReadFile(outFile)
	assert.Nil(err)
	assert.Equal(gdRes.Payload.Deployment, string(resBytes))

	t.Log("success with data string and specified format and specified cluster type and version")
	mockCtrl.Finish()
	gdParams2 := cluster.NewClusterOrchestratorGetDeploymentParams().WithID("id2").WithOrchestratorType(swag.String("mesos")).WithOrchestratorVersion(swag.String("v1.13.5"))
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadNodes)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(1)
	cOps.EXPECT().ClusterList(gomock.Any()).Return(lRes, nil).MinTimes(1)
	cOps.EXPECT().ClusterOrchestratorGetDeployment(gdParams2).Return(gdRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	outFile = "./nuvo-cluster.yaml"
	defer func() { os.Remove(outFile) }()
	err = parseAndRun([]string{"cluster", "get-deployment", "-D", "domainName", "-n", "cluster2", "-T", "mesos", "-V", "v1.13.5"})
	assert.Nil(err)
	resBytes, err = ioutil.ReadFile(outFile)
	assert.Nil(err)
	assert.Equal(gdRes.Payload.Deployment, string(resBytes))

	t.Log("success with specified output file")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadNodes)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(1)
	cOps.EXPECT().ClusterList(gomock.Any()).Return(lRes, nil).MinTimes(1)
	cOps.EXPECT().ClusterOrchestratorGetDeployment(gdParams).Return(gdRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	tmpFile1 := "./.output.yaml"
	defer func() { os.Remove(tmpFile1) }()
	err = parseAndRun([]string{"cluster", "get-deployment", "-D", "domainName", "-n", "cluster2", "-O", tmpFile1})
	assert.Nil(err)
	resBytes, err = ioutil.ReadFile(tmpFile1)
	assert.Nil(err)
	assert.Equal(gdRes.Payload.Deployment, string(resBytes))

	t.Log("success with stdout")
	mockCtrl.Finish()
	defer func() {
		outputWriter = os.Stdout
	}()
	var b bytes.Buffer
	outputWriter = &b
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadNodes)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(1)
	cOps.EXPECT().ClusterList(gomock.Any()).Return(lRes, nil).MinTimes(1)
	cOps.EXPECT().ClusterOrchestratorGetDeployment(gdParams).Return(gdRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "get-deployment", "-D", "domainName", "-n", "cluster2", "-O", "-"})
	assert.Nil(err)
	assert.Equal(gdRes.Payload.Deployment, b.String())

	t.Log("case: cluster not found")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadNodes)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(1)
	cOps.EXPECT().ClusterList(gomock.Any()).Return(lRes, nil).MinTimes(1)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	tmpFile2 := "./output_file.yaml"
	defer func() { os.Remove(tmpFile2) }()
	err = parseAndRun([]string{"cluster", "get-deployment", "-D", "badDomainName", "-n", "cluster2", "-O", tmpFile2})
	assert.NotNil(err)
	assert.Equal("csp domain 'badDomainName' not found", err.Error())

	t.Log("case: invalid output file")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadNodes)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(1)
	cOps.EXPECT().ClusterList(gomock.Any()).Return(lRes, nil).MinTimes(1)
	cOps.EXPECT().ClusterOrchestratorGetDeployment(gdParams).Return(gdRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "get-deployment", "-D", "domainName", "-n", "cluster2", "-O", "./bad/file"})
	assert.NotNil(err)
	assert.Regexp("no such file or directory", err.Error())

	t.Log("case: get deployment error")
	apiErr := &cluster.ClusterOrchestratorGetDeploymentDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadNodes)
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(1)
	cOps.EXPECT().ClusterList(gomock.Any()).Return(lRes, nil).MinTimes(1)
	cOps.EXPECT().ClusterOrchestratorGetDeployment(gdParams).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "get-deployment", "-D", "domainName", "-n", "cluster2", "-O", "-"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	t.Log("case: init context failure, stdout")
	mockCtrl.Finish()
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
	initCluster()
	err = parseAndRun([]string{"cluster", "get-deployment", "-A", "System", "-D", "domainName", "-n", "cluster2", "-O", "-"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("case: unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCluster()
	err = parseAndRun([]string{"cluster", "get-deployment", "-D", "domainName", "-n", "cluster2", "-O", "-", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments:", err)
}
