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
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_credential"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestCspDomainMakeRecord(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	dc := &cspDomainCmd{}
	dc.accounts = map[string]ctxIDName{
		"accountID": {id: "", name: "tenant 1"},
		"subId1":    {id: "accountID", name: "sub 1"},
		"subId2":    {id: "accountID", name: "sub 2"},
	}
	dc.cspCredentials = map[string]string{
		"credID": "credName",
		"cred-1": "credName1",
	}
	o := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID:           "id",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
			AccountID:     "accountID",
			CspDomainType: models.CspDomainTypeMutable("AWS"),
			CspDomainAttributes: map[string]models.ValueType{
				"aws_access_key_id":     models.ValueType{Kind: "STRING", Value: "myAwsId"},
				"aws_secret_access_key": models.ValueType{Kind: "SECRET", Value: "secret"},
				"aws_region":            models.ValueType{Kind: "STRING", Value: "us-west-2"},
			},
		},
		CSPDomainMutable: models.CSPDomainMutable{
			AuthorizedAccounts: []models.ObjIDMutable{"subId1", "subId2"},
			Name:               "MyAWS",
			Description:        "AWS domain object",
			Tags:               models.ObjTags{"tag1", "tag2", "tag3"},
			ManagementHost:     "f.q.d.n",
			CspCredentialID:    "credID",
		},
	}
	rec := dc.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(cspDomainHeaders))
	assert.Equal("id", rec[hID])
	assert.Equal("1", rec[hVersion])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeCreated])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeModified])
	assert.Equal("AWS", rec[hCspDomainType])
	assert.Equal("MyAWS", rec[hName])
	assert.Equal("AWS domain object", rec[hDescription])
	assert.Equal("f.q.d.n", rec[hManagementHost])
	assert.Equal("tag1, tag2, tag3", rec[hTags])
	assert.Equal("tenant 1", rec[hAccount])
	assert.Equal("sub 1, sub 2", rec[hAuthorizedAccounts])
	assert.Equal("credName", rec[hCspCredential])
	// note sorted order of attribute name
	al := "aws_access_key_id[S]: myAwsId\n" + "aws_region[S]: us-west-2\n" + "aws_secret_access_key[E]: ***"
	assert.Equal(al, rec[hCspDomainAttributes])

	// no caches
	dc.accounts = map[string]ctxIDName{}
	dc.cspCredentials = map[string]string{}
	rec = dc.makeRecord(o)
	t.Log(rec)
	assert.Equal("accountID", rec[hAccount])
	assert.Equal("subId1, subId2", rec[hAuthorizedAccounts])
	assert.Equal("credID", rec[hCspCredential])
}

func TestCspDomainList(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	params := csp_domain.NewCspDomainListParams()
	params.Name = swag.String("")
	params.CspDomainType = swag.String("")
	m := mockmgmtclient.NewCspDomainMatcher(t, params)
	res := &csp_domain.CspDomainListOK{
		Payload: []*models.CSPDomain{
			&models.CSPDomain{
				CSPDomainAllOf0: models.CSPDomainAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
					CspDomainType: models.CspDomainTypeMutable("AWS"),
					CspDomainAttributes: map[string]models.ValueType{
						"aws_access_key_id":     models.ValueType{Kind: "STRING", Value: "id1"},
						"aws_secret_access_key": models.ValueType{Kind: "SECRET", Value: "secret1"},
						"aws_region":            models.ValueType{Kind: "STRING", Value: "us-west-2"},
					},
				},
				CSPDomainMutable: models.CSPDomainMutable{
					Name:            "cspDomain1",
					Description:     "cspDomain1 object",
					Tags:            models.ObjTags{"tag1", "tag2", "tag3"},
					ManagementHost:  "f1.q1.d.n",
					CspCredentialID: "credID1",
				},
			},
			&models.CSPDomain{
				CSPDomainAllOf0: models.CSPDomainAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id2",
						Version:      2,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
					CspDomainType: models.CspDomainTypeMutable("AWS"),
					CspDomainAttributes: map[string]models.ValueType{
						"aws_access_key_id":     models.ValueType{Kind: "STRING", Value: "id2"},
						"aws_secret_access_key": models.ValueType{Kind: "SECRET", Value: "secret2"},
						"aws_region":            models.ValueType{Kind: "STRING", Value: "us-east-1"},
					},
				},
				CSPDomainMutable: models.CSPDomainMutable{
					Name:            "cspDomain2",
					Description:     "cspDomain2 object",
					Tags:            models.ObjTags{"tag1", "tag2", "tag3"},
					ManagementHost:  "f2.q2.d.n",
					CspCredentialID: "credID2",
				},
			},
		},
	}
	resAccounts := &account.AccountListOK{
		Payload: []*models.Account{
			&models.Account{
				AccountAllOf0:  models.AccountAllOf0{Meta: &models.ObjMeta{ID: "accountID"}},
				AccountMutable: models.AccountMutable{Name: "System"},
			},
			&models.Account{
				AccountAllOf0:  models.AccountAllOf0{Meta: &models.ObjMeta{ID: "tid1"}},
				AccountMutable: models.AccountMutable{Name: "Nuvoloso"},
			},
			&models.Account{
				AccountAllOf0:  models.AccountAllOf0{Meta: &models.ObjMeta{ID: "sid1"}, TenantAccountID: "tid1"},
				AccountMutable: models.AccountMutable{Name: "dave"},
			},
		},
	}
	resCreds := &csp_credential.CspCredentialListOK{
		Payload: []*models.CSPCredential{
			&models.CSPCredential{
				CSPCredentialAllOf0: models.CSPCredentialAllOf0{
					Meta: &models.ObjMeta{
						ID: "credID1",
					},
					AccountID:     "aid1",
					CspDomainType: "AWS",
				},
				CSPCredentialMutable: models.CSPCredentialMutable{
					Name: "cspCredential1",
				},
			},
			&models.CSPCredential{
				CSPCredentialAllOf0: models.CSPCredentialAllOf0{
					Meta: &models.ObjMeta{
						ID: "credID2",
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
	aM := mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	cM := mockmgmtclient.NewCspCredentialMatcher(t, csp_credential.NewCspCredentialListParams())

	// list, all defaults
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	dOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	dOps.EXPECT().CspDomainList(m).Return(res, nil)
	aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	cOps := mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps)
	cOps.EXPECT().CspCredentialList(cM).Return(resCreds, nil).MinTimes(1)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err := parseAndRun([]string{"csp-domain", "list"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(cspDomainDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list default, json
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	dOps.EXPECT().CspDomainList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "list", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(res.Payload, te.jsonData)

	// list default, yaml
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	dOps.EXPECT().CspDomainList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "list", "-o", "yaml"})
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
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	dOps.EXPECT().CspDomainList(m).Return(res, nil)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps)
	cOps.EXPECT().CspCredentialList(cM).Return(resCreds, nil).MinTimes(1)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "list", "-f"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(cspDomainDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list with columns, valid account and authorized account
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	params.AuthorizedAccountID = swag.String("sid1")
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "Nuvoloso").Return("tid1", nil)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	dOps.EXPECT().CspDomainList(m).Return(res, nil)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps)
	cOps.EXPECT().CspCredentialList(cM).Return(resCreds, nil).MinTimes(1)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "list", "-A", "Nuvoloso", "-Z", "dave", "--columns", "Name,ID"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, 2)
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))
	params.AuthorizedAccountID = nil
	appCtx.Account, appCtx.AccountID = "", ""

	// list valid account and owner authorized account
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	params.AuthorizedAccountID = swag.String("tid1")
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "Nuvoloso").Return("tid1", nil)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	dOps.EXPECT().CspDomainList(m).Return(res, nil)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps)
	cOps.EXPECT().CspCredentialList(cM).Return(resCreds, nil).MinTimes(1)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "list", "-A", "Nuvoloso", "--owner-auth"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(cspDomainDefaultHeaders))
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
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "list", "-A", "Nuvoloso", "--owner-auth", "-Z", "dave"})
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
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "list", "--owner-auth"})
	assert.NotNil(err)
	assert.Regexp("--owner-auth requires --account", err)
	appCtx.Account, appCtx.AccountID = "", ""

	// list with columns, valid account and invalid authorized account
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "Nuvoloso").Return("tid1", nil)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "list", "-A", "Nuvoloso", "-Z", "carl"})
	assert.NotNil(err)
	assert.Regexp("authorized account .* not found", err)
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
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "list", "-c", "Name,ID,Foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// list, API failure model error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	apiErr := &csp_domain.CspDomainListDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	dOps.EXPECT().CspDomainList(m).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "list"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// list, API failure arbitrary error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	otherErr := fmt.Errorf("OTHER ERROR")
	dOps.EXPECT().CspDomainList(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "list"})
	assert.NotNil(err)
	assert.Equal(otherErr, err)

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
	initCspDomain()
	err = parseAndRun([]string{"domain", "list", "-A", "System"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "list", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestCspDomainCreate(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	params := csp_domain.NewCspDomainCreateParams()
	params.Payload = &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			AccountID:     "id1",
			CspDomainType: models.CspDomainTypeMutable("AWS"),
			CspDomainAttributes: map[string]models.ValueType{
				"aws_access_key_id":     models.ValueType{Kind: "STRING", Value: "myAwsId"},
				"aws_secret_access_key": models.ValueType{Kind: "SECRET", Value: "secret"},
				"aws_region":            models.ValueType{Kind: "STRING", Value: "us-west-2"},
			},
		},
		CSPDomainMutable: models.CSPDomainMutable{
			Name:            models.ObjName("name"),
			Description:     models.ObjDescription("description"),
			Tags:            []string{"tag1", "tag2"},
			ManagementHost:  "f.q.d.n",
			CspCredentialID: "credId1",
			StorageCosts: map[string]models.StorageCost{
				"Amazon gp2": {CostPerGiB: 3.142},
			},
		},
	}
	res := &csp_domain.CspDomainCreateCreated{
		Payload: params.Payload,
	}

	now := time.Now()
	credListParams := csp_credential.NewCspCredentialListParams()
	credListParams.Name = swag.String("cspCredential1")
	credListParams.CspDomainType = swag.String("AWS")
	clm := mockmgmtclient.NewCspCredentialMatcher(t, credListParams)
	credListRes := &csp_credential.CspCredentialListOK{
		Payload: []*models.CSPCredential{
			&models.CSPCredential{
				CSPCredentialAllOf0: models.CSPCredentialAllOf0{
					Meta: &models.ObjMeta{
						ID:           "credId1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
					CspDomainType: models.CspDomainTypeMutable("AWS"),
				},
				CSPCredentialMutable: models.CSPCredentialMutable{
					Name:        "cspCredential1",
					Description: "cspCredential1 object",
					Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
					CredentialAttributes: map[string]models.ValueType{
						"aws_access_key_id":     models.ValueType{Kind: "STRING", Value: "id1"},
						"aws_secret_access_key": models.ValueType{Kind: "SECRET", Value: "secret1"},
					},
				},
			},
		},
	}

	m := mockmgmtclient.NewCspDomainMatcher(t, params)

	// create
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "nuvoloso").Return("id1", nil)
	dOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	dOps.EXPECT().CspDomainCreate(m).Return(res, nil)
	cCred := mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cCred)
	cCred.EXPECT().CspCredentialList(clm).Return(credListRes, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err := parseAndRun([]string{"csp-domain", "create", "-A", "nuvoloso", "-n", string(params.Payload.Name),
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-a", "aws_access_key_id:myAwsId", "-a", "aws_secret_access_key:secret",
		"-a", "aws_region:us-west-2", "-H", "f.q.d.n",
		"--cred-name", "cspCredential1", "--cost", "Amazon gp2:3.142",
		"-T", "AWS", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.CSPDomain{params.Payload}, te.jsonData)

	params.Payload.StorageCosts = nil // ignore for the rest of the test

	// create, invalid columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "create", "-n", string(params.Payload.Name),
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-a", "aws_access_key_id:myAwsId", "-a", "aws_secret_access_key:secret",
		"-a", "aws_region:us-west-2", "-H", "f.q.d.n",
		"-T", "AWS", "--columns", "Name,ID,FOO"})

	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// create, API model error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	apiErr := &csp_domain.CspDomainCreateDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	dOps.EXPECT().CspDomainCreate(m).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "create", "-n", string(params.Payload.Name),
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-a", "aws_access_key_id:myAwsId", "-a", "aws_secret_access_key:secret",
		"-a", "aws_region:us-west-2", "-H", "f.q.d.n",
		"--cred-id", "credId1",
		"-T", "AWS", "-o", "json"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// create, API failure arbitrary error
	params.Payload.CspCredentialID = ""
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	otherErr := fmt.Errorf("OTHER ERROR")
	dOps.EXPECT().CspDomainCreate(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "create", "-n", string(params.Payload.Name),
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-a", "aws_access_key_id:myAwsId", "-a", "aws_secret_access_key:secret",
		"-a", "aws_region:us-west-2", "-H", "f.q.d.n",
		"-T", "AWS", "-o", "json"})
	assert.NotNil(err)
	assert.Equal(otherErr, err)
	appCtx.Account, appCtx.AccountID = "", ""

	// create, failure to find CspCredential
	params.Payload.CspCredentialID = ""
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cCred = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cCred)
	credListParams.CspDomainType = swag.String("GCP")
	cspErr := fmt.Errorf("CspCredential LIST ERROR")
	cCred.EXPECT().CspCredentialList(clm).Return(nil, cspErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "create", "-n", string(params.Payload.Name),
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-a", "gc_zone:us-west1-b", "-H", "f.q.d.n",
		"--cred-name", "cspCredential1",
		"-T", "GCP", "-o", "json"})
	assert.NotNil(err)
	assert.Equal(cspErr, err)
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
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "create", "-A", "System", "-n", string(params.Payload.Name),
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-a", "aws_access_key_id:myAwsId", "-a", "aws_secret_access_key:secret",
		"-a", "aws_region:us-west-2", "-H", "f.q.d.n",
		"-T", "AWS", "-o", "json"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "create", "-A", "nuvoloso", "-n", string(params.Payload.Name),
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-a", "aws_access_key_id:myAwsId", "-a", "aws_secret_access_key:secret",
		"-a", "aws_region:us-west-2", "-H", "f.q.d.n",
		"--cred-name", "cspCredential1", "--cost", "Amazon gp2:3.142",
		"-T", "AWS", "-o", "json", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestCSPDomainDelete(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	lRes := &csp_domain.CspDomainListOK{
		Payload: []*models.CSPDomain{
			&models.CSPDomain{
				CSPDomainAllOf0: models.CSPDomainAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
					CspDomainType: models.CspDomainTypeMutable("AWS"),
					CspDomainAttributes: map[string]models.ValueType{
						"aws_access_key_id":     models.ValueType{Kind: "STRING", Value: "id1"},
						"aws_secret_access_key": models.ValueType{Kind: "SECRET", Value: "secret1"},
						"aws_region":            models.ValueType{Kind: "STRING", Value: "us-west-2"},
					},
				},
				CSPDomainMutable: models.CSPDomainMutable{
					Name:           "cspDomain1",
					Description:    "cspDomain1 object",
					Tags:           models.ObjTags{"tag1", "tag2", "tag3"},
					ManagementHost: "f1.q1.d.n",
				},
			},
		},
	}
	lParams := csp_domain.NewCspDomainListParams()
	lParams.Name = swag.String(string(lRes.Payload[0].Meta.ID))
	m := mockmgmtclient.NewCspDomainMatcher(t, lParams)
	dParams := csp_domain.NewCspDomainDeleteParams()
	dParams.ID = string(lRes.Payload[0].Meta.ID)
	dRet := &csp_domain.CspDomainDeleteNoContent{}

	// delete
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	dOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(m).Return(lRes, nil)
	dOps.EXPECT().CspDomainDelete(dParams).Return(dRet, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err := parseAndRun([]string{"csp-domain", "delete", "-n", string(*lParams.Name), "--confirm"})
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
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "delete", "-n", string(*lParams.Name)})
	assert.NotNil(err)
	assert.Regexp("--confirm", err.Error())

	// delete, not found
	emptyRes := &csp_domain.CspDomainListOK{
		Payload: []*models.CSPDomain{},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(m).Return(emptyRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "delete", "-n", string(*lParams.Name), "--confirm"})
	assert.NotNil(err)
	assert.Regexp("not found", err.Error())

	// delete, list error
	otherErr := fmt.Errorf("other error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "delete", "-n", string(*lParams.Name), "--confirm"})
	assert.NotNil(err)
	assert.Regexp("other error", err.Error())

	// delete, API model error
	apiErr := &csp_domain.CspDomainDeleteDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(m).Return(lRes, nil)
	dOps.EXPECT().CspDomainDelete(dParams).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "delete", "-n", string(*lParams.Name), "--confirm"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// delete, API arbitrary error, valid account
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "Nuvoloso").Return("tid1", nil)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(m).Return(lRes, nil)
	dOps.EXPECT().CspDomainDelete(dParams).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "-A", "Nuvoloso", "delete", "-n", string(*lParams.Name), "--confirm"})
	assert.NotNil(err)
	assert.Regexp("other error", err.Error())
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
	initCspDomain()
	err = parseAndRun([]string{"domain", "delete", "-A", "System", "-n", string(*lParams.Name), "--confirm"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "delete", "-n", string(*lParams.Name), "confirm"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestCSPDomainModify(t *testing.T) {
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
	lRes := &csp_domain.CspDomainListOK{
		Payload: []*models.CSPDomain{
			&models.CSPDomain{
				CSPDomainAllOf0: models.CSPDomainAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
					CspDomainType: models.CspDomainTypeMutable("AWS"),
					CspDomainAttributes: map[string]models.ValueType{
						"aws_access_key_id":     models.ValueType{Kind: "STRING", Value: "id1"},
						"aws_secret_access_key": models.ValueType{Kind: "SECRET", Value: "secret1"},
						"aws_region":            models.ValueType{Kind: "STRING", Value: "us-west-2"},
					},
				},
				CSPDomainMutable: models.CSPDomainMutable{
					Name:           "cspDomain1",
					Description:    "cspDomain1 object",
					Tags:           models.ObjTags{"tag1", "tag2", "tag3"},
					ManagementHost: "f1.q1.d.n",
					ClusterUsagePolicy: &models.ClusterUsagePolicy{
						Inherited:                   true,
						AccountSecretScope:          "CLUSTER",
						VolumeDataRetentionOnDelete: "DELETE",
					},
					StorageCosts: map[string]models.StorageCost{},
				},
			},
		},
	}
	lParams := csp_domain.NewCspDomainListParams()
	lParams.Name = swag.String(string(lRes.Payload[0].Meta.ID))
	lm := mockmgmtclient.NewCspDomainMatcher(t, lParams)
	lParams2 := csp_domain.NewCspDomainListParams()
	lParams2.Name = swag.String(string(lRes.Payload[0].Meta.ID))
	lm2 := mockmgmtclient.NewCspDomainMatcher(t, lParams2)
	aM := mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())

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
	dOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(lm).Return(lRes, nil)
	uParams := csp_domain.NewCspDomainUpdateParams()
	uParams.Payload = &models.CSPDomainMutable{
		AuthorizedAccounts: []models.ObjIDMutable{"subAID1"},
		Name:               "newName",
		Description:        "newDescription",
		ManagementHost:     "newManagementHost",
		CspCredentialID:    "newCredID",
		StorageCosts: map[string]models.StorageCost{
			"Amazon gp2": {CostPerGiB: 3.142},
		},
		Tags: []string{"tag1", "tag2"},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"name", "description", "managementHost", "cspCredentialId"}
	uParams.Append = []string{"tags", "authorizedAccounts", "storageCosts"}
	um := mockmgmtclient.NewCspDomainMatcher(t, uParams)
	uRet := csp_domain.NewCspDomainUpdateOK()
	uRet.Payload = lRes.Payload[0]
	dOps.EXPECT().CspDomainUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err := parseAndRun([]string{"csp-domain", "-A", "Nuvoloso", "modify", "-n", string(*lParams.Name),
		"-N", "newName", "-d", "newDescription", "-Z", "sub 1", "-H", "newManagementHost",
		"--cred-id", "newCredID", "--cost", "Amazon gp2:3.142",
		"-t", "tag1", "-t", "tag2", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.CSPDomain{uRet.Payload}, te.jsonData)
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
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(lm).Return(lRes, nil)
	uParams = csp_domain.NewCspDomainUpdateParams()
	uParams.Payload = &models.CSPDomainMutable{
		AuthorizedAccounts: []models.ObjIDMutable{"tenantAID", "subAID1"},
		Name:               "newName",
		Description:        "newDescription",
		ManagementHost:     "newManagementHost",
		StorageCosts: map[string]models.StorageCost{
			"Amazon gp2": {CostPerGiB: 3.142},
		},
		Tags: []string{"tag1", "tag2"},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"name", "description", "managementHost", "tags", "authorizedAccounts", "storageCosts"}
	um = mockmgmtclient.NewCspDomainMatcher(t, uParams)
	uRet = csp_domain.NewCspDomainUpdateOK()
	uRet.Payload = lRes.Payload[0]
	dOps.EXPECT().CspDomainUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "-A", "Nuvoloso", "modify", "-n", string(*lParams.Name),
		"-N", "newName", "-d", "newDescription", "-H", "newManagementHost",
		"--cost", "Amazon gp2:3.142", "--cost-action", "SET",
		"-t", "tag1", "-t", "tag2", "--tag-action", "SET", "-V", "1", "-o", "json",
		"-Z", "sub 1", "--owner-auth", "--authorized-accounts-action=SET"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.CSPDomain{uRet.Payload}, te.jsonData)
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
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(lm).Return(lRes, nil)
	uParams = csp_domain.NewCspDomainUpdateParams()
	uParams.Payload = &models.CSPDomainMutable{
		AuthorizedAccounts: []models.ObjIDMutable{"subAID1"},
		Name:               "newName",
		Description:        "newDescription",
		ManagementHost:     "newManagementHost",
		StorageCosts: map[string]models.StorageCost{
			"Amazon gp2": {CostPerGiB: 0.0},
		},
		Tags: []string{"tag1", "tag2"},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"name", "description", "managementHost"}
	uParams.Remove = []string{"authorizedAccounts", "tags", "storageCosts"}
	um = mockmgmtclient.NewCspDomainMatcher(t, uParams)
	uRet = csp_domain.NewCspDomainUpdateOK()
	uRet.Payload = lRes.Payload[0]
	dOps.EXPECT().CspDomainUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "-A", "Nuvoloso", "modify", "-n", string(*lParams.Name),
		"-N", "newName", "-d", "newDescription", "-H", "newManagementHost",
		"--cost", "Amazon gp2:0", "--cost-action", "REMOVE",
		"-t", "tag1", "-t", "tag2", "--tag-action", "REMOVE", "-V", "1", "-o", "json",
		"-Z", "sub 1", "--authorized-accounts-action=REMOVE"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.CSPDomain{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// same, but empty SET
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(lm2).Return(lRes, nil)
	uParams = csp_domain.NewCspDomainUpdateParams()
	uParams.Payload = &models.CSPDomainMutable{
		Name:           "newName",
		Description:    "newDescription",
		ManagementHost: "newManagementHost",
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"name", "description", "managementHost", "tags", "authorizedAccounts"}
	um = mockmgmtclient.NewCspDomainMatcher(t, uParams)
	uRet = csp_domain.NewCspDomainUpdateOK()
	uRet.Payload = lRes.Payload[0]
	dOps.EXPECT().CspDomainUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "modify", "-n", string(*lParams2.Name),
		"-N", "newName", "-d", "newDescription", "-H", "newManagementHost",
		"--authorized-accounts-action=SET",
		"--tag-action", "SET", "-V", "1", "-o", "json",
	})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.CSPDomain{uRet.Payload}, te.jsonData)

	// inherit CUP
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(lm2).Return(lRes, nil)
	uParams = csp_domain.NewCspDomainUpdateParams()
	uParams.Payload = &models.CSPDomainMutable{
		ClusterUsagePolicy: &models.ClusterUsagePolicy{
			Inherited: true,
		},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"clusterUsagePolicy"}
	um = mockmgmtclient.NewCspDomainMatcher(t, uParams)
	uRet = csp_domain.NewCspDomainUpdateOK()
	uRet.Payload = lRes.Payload[0]
	dOps.EXPECT().CspDomainUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "modify", "-n", string(*lParams2.Name),
		"--cup-inherit", "-V", "1", "-o", "json",
	})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.CSPDomain{uRet.Payload}, te.jsonData)

	// CUP modifications
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(lm2).Return(lRes, nil)
	uParams = csp_domain.NewCspDomainUpdateParams()
	uParams.Payload = &models.CSPDomainMutable{
		ClusterUsagePolicy: &models.ClusterUsagePolicy{
			Inherited:                   false,
			AccountSecretScope:          "GLOBAL",
			ConsistencyGroupName:        "${cluster.name}",
			VolumeDataRetentionOnDelete: "RETAIN",
		},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"clusterUsagePolicy"}
	um = mockmgmtclient.NewCspDomainMatcher(t, uParams)
	uRet = csp_domain.NewCspDomainUpdateOK()
	uRet.Payload = lRes.Payload[0]
	dOps.EXPECT().CspDomainUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "modify", "-n", string(*lParams2.Name),
		"--cup-account-secret-scope", "GLOBAL", "--cup-consistency-group-name", "${cluster.name}",
		"--cup-data-retention-on-delete", "RETAIN", "-V", "1", "-o", "json",
	})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.CSPDomain{uRet.Payload}, te.jsonData)

	// CUP flag errors
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "modify", "-n", string(*lParams2.Name),
		"--cup-inherit", "-S", "GLOBAL", "-V", "1", "-o", "json",
	})
	assert.NotNil(err)
	assert.Nil(te.jsonData)
	assert.Regexp("set.*inherit.*cluster usage policy", err.Error())
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "modify", "-n", string(*lParams2.Name),
		"--cup-inherit", "--cup-data-retention-on-delete", "DELETE", "-V", "1", "-o", "json",
	})
	assert.NotNil(err)
	assert.Nil(te.jsonData)
	assert.Regexp("set.*inherit.*cluster usage policy", err.Error())

	// validate account fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "Nuvoloso").Return("badtenantAID", nil)
	re := fmt.Errorf("authorized account.*not found")
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(lm).Return(lRes, nil)
	uParams = csp_domain.NewCspDomainUpdateParams()
	uParams.Payload = &models.CSPDomainMutable{
		AuthorizedAccounts: []models.ObjIDMutable{"subAID1"},
		Name:               "newName",
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Append = []string{"authorizedAccounts"}
	um = mockmgmtclient.NewCspDomainMatcher(t, uParams)
	uRet = csp_domain.NewCspDomainUpdateOK()
	uRet.Payload = lRes.Payload[0]
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "-A", "Nuvoloso", "modify", "-n", string(*lParams.Name), "-Z", "sub 1", "-o", "json"})
	assert.NotNil(err)
	assert.Regexp(re, err.Error())
	assert.Nil(te.jsonData)
	lParams.AccountID = swag.String(string(lRes.Payload[0].AccountID))
	appCtx.Account, appCtx.AccountID = "", ""

	// no changes
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(lm2).Return(lRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "modify", "-n", string(*lParams2.Name), "-V", "1", "-o", "json"})
	assert.NotNil(err)
	assert.Nil(te.jsonData)
	assert.Regexp("No modifications", err.Error())

	// update error
	mockCtrl.Finish()
	updateErr := csp_domain.NewCspDomainUpdateDefault(400)
	updateErr.Payload = &models.Error{Code: 400, Message: swag.String("update error")}
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(lm2).Return(lRes, nil)
	uParams = csp_domain.NewCspDomainUpdateParams()
	uParams.Payload = &models.CSPDomainMutable{
		Name:           "newName",
		Description:    "newDescription",
		ManagementHost: "newManagementHost",
		Tags:           []string{"tag1", "tag2"},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"name", "description", "managementHost"}
	uParams.Remove = []string{"tags"}
	um = mockmgmtclient.NewCspDomainMatcher(t, uParams)
	dOps.EXPECT().CspDomainUpdate(um).Return(nil, updateErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "modify", "-n", string(*lParams2.Name),
		"-N", "newName", "-d", "newDescription", "-H", "newManagementHost",
		"-t", "tag1", "-t", "tag2", "--tag-action", "REMOVE", "-V", "1", "-o", "json",
	})
	assert.NotNil(err)
	assert.Regexp("update error", err.Error())

	// other error on update
	otherErr := fmt.Errorf("other error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(lm2).Return(lRes, nil)
	uParams = csp_domain.NewCspDomainUpdateParams()
	uParams.Payload = &models.CSPDomainMutable{
		Name:           "newName",
		Description:    "newDescription",
		ManagementHost: "newManagementHost",
		Tags:           []string{"tag1", "tag2"},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"name", "description", "managementHost"}
	uParams.Remove = []string{"tags"}
	um = mockmgmtclient.NewCspDomainMatcher(t, uParams)
	dOps.EXPECT().CspDomainUpdate(um).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "modify", "-n", string(*lParams2.Name),
		"-N", "newName", "-d", "newDescription", "-H", "newManagementHost",
		"-t", "tag1", "-t", "tag2", "--tag-action", "REMOVE", "-V", "1", "-o", "json",
	})
	assert.NotNil(err)
	assert.Regexp("other error", err.Error())

	// invalid columns
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "modify", "-n", string(*lParams2.Name), "--columns", "ID,foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err.Error())

	// not found
	mockCtrl.Finish()
	emptyRes := &csp_domain.CspDomainListOK{
		Payload: []*models.CSPDomain{},
	}
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(lm2).Return(emptyRes, nil)
	appCtx.API = mAPI
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "modify", "-n", string(*lParams2.Name),
		"-N", "newName", "-d", "newDescription", "-H", "newManagementHost",
		"-t", "tag1", "-t", "tag2", "--tag-action", "SET", "-V", "1", "-o", "json",
	})
	assert.NotNil(err)
	assert.Regexp("not found", err.Error())

	// list error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(lm2).Return(nil, otherErr)
	appCtx.API = mAPI
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "modify", "-n", string(*lParams2.Name),
		"-N", "newName", "-d", "newDescription", "-H", "newManagementHost",
		"-t", "tag1", "-t", "tag2", "--tag-action", "SET", "-V", "1", "-o", "json",
	})
	assert.NotNil(err)
	assert.Regexp("other error", err.Error())

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
	initCspDomain()
	err = parseAndRun([]string{"domain", "modify", "-A", "System", "-n", *lParams.Name})
	assert.Regexp("ctx error", err)
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("bad owner-auth")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"domain", "modify", "-n", *lParams.Name, "--owner-auth"})
	assert.Regexp("owner-auth requires --account", err)

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "-A", "Nuvoloso", "modify", "-n", string(*lParams.Name),
		"-N", "newName", "-d", "newDescription", "-Z", "sub 1", "-H", "newManagementHost",
		"--cred-id", "newCredID", "--cost", "Amazon gp2:3.142",
		"-t", "tag1", "-t", "tag2", "-V", "1", "o", "json"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestCSPDomainDeployFetch(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	lRes := &csp_domain.CspDomainListOK{
		Payload: []*models.CSPDomain{
			&models.CSPDomain{
				CSPDomainAllOf0: models.CSPDomainAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
					CspDomainType: models.CspDomainTypeMutable("AWS"),
					CspDomainAttributes: map[string]models.ValueType{
						"aws_access_key_id":     models.ValueType{Kind: "STRING", Value: "id1"},
						"aws_secret_access_key": models.ValueType{Kind: "SECRET", Value: "secret1"},
						"aws_region":            models.ValueType{Kind: "STRING", Value: "us-west-2"},
					},
				},
				CSPDomainMutable: models.CSPDomainMutable{
					Name:           "cspDomain1",
					Description:    "cspDomain1 object",
					Tags:           models.ObjTags{"tag1", "tag2", "tag3"},
					ManagementHost: "f1.q1.d.n",
				},
			},
		},
	}
	lParams := csp_domain.NewCspDomainListParams()
	lParams.Name = swag.String(string(lRes.Payload[0].Meta.ID))
	lm := mockmgmtclient.NewCspDomainMatcher(t, lParams)

	// deployment fetch
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "Nuvoloso").Return("tenantAID", nil)
	dOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(lm).Return(lRes, nil)
	dfParams := csp_domain.NewCspDomainDeploymentFetchParams()
	dfParams.ClusterType = swag.String("kubernetes")
	dfParams.ID = string(lRes.Payload[0].Meta.ID)
	dfParams.Force = swag.Bool(false)
	dfRet := csp_domain.NewCspDomainDeploymentFetchOK()
	dfRet.Payload = &models.CspDomainDeploymentFetchOKBody{}
	dfRet.Payload.Format = "yaml"
	dfRet.Payload.Deployment = "deployment yaml"
	dOps.EXPECT().CspDomainDeploymentFetch(dfParams).Return(dfRet, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	outFile := "./nuvo-cluster.yaml"
	defer func() { os.Remove(outFile) }()
	err := parseAndRun([]string{"csp-domain", "get-deployment", "-A", "Nuvoloso", "-n", string(*lParams.Name)})
	assert.Nil(err)
	resBytes, err := ioutil.ReadFile(outFile)
	assert.Nil(err)
	assert.Equal(dfRet.Payload.Deployment, string(resBytes))
	lParams.AccountID = nil
	appCtx.Account, appCtx.AccountID = "", ""

	// repeat with a specified output file and cluster name
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	dfParams.Name = swag.String("customName")
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(lm).Return(lRes, nil)
	dOps.EXPECT().CspDomainDeploymentFetch(dfParams).Return(dfRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	tmpFile := "./.output"
	defer func() { os.Remove(tmpFile) }()
	err = parseAndRun([]string{"csp-domain", "get-deployment", "-n", string(*lParams.Name),
		"-O", tmpFile, "--cluster-name", *dfParams.Name,
	})
	assert.Nil(err)
	resBytes, err = ioutil.ReadFile(tmpFile)
	assert.Nil(err)
	assert.Equal(dfRet.Payload.Deployment, string(resBytes))

	// deployment fetch error, use stdout. Also tests force flag.
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(lm).Return(lRes, nil)
	dfParams = csp_domain.NewCspDomainDeploymentFetchParams()
	dfParams.ClusterType = swag.String("fooClusterType")
	dfParams.ID = string(lRes.Payload[0].Meta.ID)
	dfParams.Force = swag.Bool(true)
	dfErr := csp_domain.NewCspDomainDeploymentFetchDefault(400)
	dfErr.Payload = &models.Error{Code: 400, Message: swag.String("dfErr")}
	dOps.EXPECT().CspDomainDeploymentFetch(dfParams).Return(nil, dfErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "get-deployment", "-n", string(*lParams.Name),
		"-O", "-", "--cluster-type", "fooClusterType", "--force",
	})
	assert.NotNil(err)
	assert.Regexp("dfErr", err.Error())

	// deployment fetch, other error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(lm).Return(lRes, nil)
	dfParams = csp_domain.NewCspDomainDeploymentFetchParams()
	dfParams.ID = string(lRes.Payload[0].Meta.ID)
	dfParams.ClusterType = swag.String("kubernetes")
	dfParams.Force = swag.Bool(false)
	otherErr := fmt.Errorf("other error")
	dOps.EXPECT().CspDomainDeploymentFetch(dfParams).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "get-deployment", "-n", string(*lParams.Name)})
	assert.NotNil(err)
	assert.Regexp("other error", err.Error())

	// not found
	emptyRes := &csp_domain.CspDomainListOK{
		Payload: []*models.CSPDomain{},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(lm).Return(emptyRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "get-deployment", "-n", string(*lParams.Name)})
	assert.NotNil(err)
	assert.Regexp("not found", err.Error())

	// list other error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(lm).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "get-deployment", "-n", string(*lParams.Name)})
	assert.NotNil(err)
	assert.Regexp("other err", err.Error())

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
	initCspDomain()
	err = parseAndRun([]string{"domain", "get-deployment", "-A", "System", "-n", string(*lParams.Name)})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// invalid output file
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "get-deployment", "-n", string(*lParams.Name),
		"-O", "./bad/file"})
	assert.NotNil(err)
	assert.Regexp("no such file or directory", err.Error())

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "get-deployment", "-A", "Nuvoloso", "-n", string(*lParams.Name), "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestCSPDomainMetadata(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	mdRes := &csp_domain.CspDomainMetadataOK{
		Payload: &models.CSPDomainMetadata{
			AttributeMetadata: map[string]models.AttributeDescriptor{
				"attr1": models.AttributeDescriptor{
					Kind:        "STRING",
					Description: "Description1",
					Optional:    false,
				},
				"attr2": models.AttributeDescriptor{
					Kind:        "SECRET",
					Description: "Description2",
					Optional:    true,
				},
			},
		},
	}
	expTd := [][]string{
		[]string{"attr1", "STRING", "Description1", "true"},
		[]string{"attr2", "SECRET", "Description2", "false"},
	}
	mdParams := csp_domain.NewCspDomainMetadataParams()
	mdParams.CspDomainType = "AWS"

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	dOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainMetadata(mdParams).Return(mdRes, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err := parseAndRun([]string{"csp-domain", "metadata"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(cspDomainMetadataCols))
	assert.Equal(cspDomainMetadataCols, te.tableHeaders)
	t.Log(te.tableData)
	assert.Len(te.tableData, len(mdRes.Payload.AttributeMetadata))
	assert.Equal(expTd, te.tableData)

	// success, json
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	mdParams.CspDomainType = "GCP"
	dOps.EXPECT().CspDomainMetadata(mdParams).Return(mdRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "metadata", "--domain-type=GCP", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(mdRes.Payload, te.jsonData)
	mdParams.CspDomainType = "AWS" // reset

	// success, yaml
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainMetadata(mdParams).Return(mdRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "md", "-T", "AWS", "-o", "yaml"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(mdRes.Payload, te.yamlData)

	// failure, invalid domain type
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	mdParams.CspDomainType = "Azure"
	apiErr := csp_domain.NewCspDomainMetadataDefault(400)
	apiErr.Payload = &models.Error{Message: swag.String("invalid"), Code: 400}
	dOps.EXPECT().CspDomainMetadata(mdParams).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "md", "-T", "Azure"})
	assert.Error(err)
	assert.Regexp("invalid", err.Error())
	mdParams.CspDomainType = "AWS" // reset

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "metadata", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestCSPDomainGet(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	reqToFetch := "id1"
	fParams := csp_domain.NewCspDomainFetchParams()
	fParams.ID = reqToFetch

	now := time.Now()
	fRet := &csp_domain.CspDomainFetchOK{
		Payload: &models.CSPDomain{
			CSPDomainAllOf0: models.CSPDomainAllOf0{
				AccountID: "tenantAID",
				Meta: &models.ObjMeta{
					ID:           "id1",
					Version:      1,
					TimeCreated:  strfmt.DateTime(now),
					TimeModified: strfmt.DateTime(now),
				},
				CspDomainType: models.CspDomainTypeMutable("AWS"),
				CspDomainAttributes: map[string]models.ValueType{
					"aws_access_key_id":     models.ValueType{Kind: "STRING", Value: "id1"},
					"aws_secret_access_key": models.ValueType{Kind: "SECRET", Value: "secret1"},
					"aws_region":            models.ValueType{Kind: "STRING", Value: "us-west-2"},
				},
			},
			CSPDomainMutable: models.CSPDomainMutable{
				Name:            "cspDomain1",
				Description:     "cspDomain1 object",
				Tags:            models.ObjTags{"tag1", "tag2", "tag3"},
				ManagementHost:  "f1.q1.d.n",
				CspCredentialID: "credID1",
				ClusterUsagePolicy: &models.ClusterUsagePolicy{
					Inherited:                   true,
					AccountSecretScope:          "CLUSTER",
					VolumeDataRetentionOnDelete: "DELETE",
				},
			},
		},
	}
	resCreds := &csp_credential.CspCredentialListOK{
		Payload: []*models.CSPCredential{
			&models.CSPCredential{
				CSPCredentialAllOf0: models.CSPCredentialAllOf0{
					Meta: &models.ObjMeta{
						ID: "credID1",
					},
					AccountID:     "aid1",
					CspDomainType: "AWS",
				},
				CSPCredentialMutable: models.CSPCredentialMutable{
					Name: "cspCredential1",
				},
			},
			&models.CSPCredential{
				CSPCredentialAllOf0: models.CSPCredentialAllOf0{
					Meta: &models.ObjMeta{
						ID: "credID2",
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
	var err error
	assert.NotNil(fRet)
	cM := mockmgmtclient.NewCspCredentialMatcher(t, csp_credential.NewCspCredentialListParams())

	// fetch
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := cacheLoadHelper(t, mockCtrl, loadAccounts)
	dOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	mF := mockmgmtclient.NewCspDomainMatcher(t, fParams)
	dOps.EXPECT().CspDomainFetch(mF).Return(fRet, nil)
	cOps := mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps)
	cOps.EXPECT().CspCredentialList(cM).Return(resCreds, nil).MinTimes(1)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "get", reqToFetch})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(cspDomainDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, 1)

	// apiError
	apiErr := &csp_domain.CspDomainFetchDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	mF = mockmgmtclient.NewCspDomainMatcher(t, fParams)
	dOps.EXPECT().CspDomainFetch(mF).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "get", "--id", reqToFetch})
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
	initCspDomain()
	err = parseAndRun([]string{"domain", "get", "-A", "System", "--id", reqToFetch})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// invalid columns
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"dom", "get", "--id", reqToFetch, "--columns", "foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	t.Log("expected id")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"csp-domain", "get"})
	assert.NotNil(err)
	assert.Regexp("expected --id flag or a single identifier argument", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestCspDomainServicePlanCost(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	domObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			AccountID: "tenantAID",
			Meta: &models.ObjMeta{
				ID:           "id1",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
			CspDomainType: models.CspDomainTypeMutable("AWS"),
			CspDomainAttributes: map[string]models.ValueType{
				"aws_access_key_id":     models.ValueType{Kind: "STRING", Value: "id1"},
				"aws_secret_access_key": models.ValueType{Kind: "SECRET", Value: "secret1"},
				"aws_region":            models.ValueType{Kind: "STRING", Value: "us-west-2"},
			},
		},
		CSPDomainMutable: models.CSPDomainMutable{
			Name:            "cspDomain1",
			Description:     "cspDomain1 object",
			Tags:            models.ObjTags{"tag1", "tag2", "tag3"},
			ManagementHost:  "f1.q1.d.n",
			CspCredentialID: "credID1",
			ClusterUsagePolicy: &models.ClusterUsagePolicy{
				Inherited:                   true,
				AccountSecretScope:          "CLUSTER",
				VolumeDataRetentionOnDelete: "DELETE",
			},
		},
	}
	spc := &models.ServicePlanCost{
		CostPerGiB:             1.5,
		CostPerGiBWithoutCache: 0.9,
		CostBreakdown:          map[string]models.StorageCostFraction{},
		StorageFormula:         &models.StorageFormula{},
	}

	var err error

	t.Log("case: success (table)")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := cacheLoadHelper(t, mockCtrl, loadPlans)
	appCtx.API = mAPI
	dOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(2)
	lParams := csp_domain.NewCspDomainListParams()
	lParams.Name = swag.String(string(domObj.Name))
	lM := mockmgmtclient.NewCspDomainMatcher(t, lParams)
	lRet := csp_domain.NewCspDomainListOK()
	lRet.Payload = []*models.CSPDomain{domObj}
	dOps.EXPECT().CspDomainList(lM).Return(lRet, nil)
	spcParams := csp_domain.NewCspDomainServicePlanCostParams()
	spcParams.ID = string(domObj.Meta.ID)
	spcParams.ServicePlanID = string(resPlans.Payload[0].Meta.ID)
	spcM := mockmgmtclient.NewCspDomainMatcher(t, spcParams)
	retSPC := csp_domain.NewCspDomainServicePlanCostOK()
	retSPC.Payload = spc
	dOps.EXPECT().CspDomainServicePlanCost(spcM).Return(retSPC, nil)
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"domain", "service-plan-cost", "-n", string(domObj.Name), "-P", string(resPlans.Payload[0].Name)})
	assert.NoError(err)
	t.Log(te.tableHeaders)
	assert.Equal(cspDomainServicePlanCostHeaders, te.tableHeaders)
	t.Log(te.tableData)
	assert.Len(te.tableData, 1)
	assert.Equal("1.5", te.tableData[0][0])
	assert.Equal("0.9", te.tableData[0][1])
	mockCtrl.Finish()

	t.Log("case: service plan cost success (yaml)")
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadPlans)
	appCtx.API = mAPI
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(2)
	dOps.EXPECT().CspDomainList(lM).Return(lRet, nil)
	dOps.EXPECT().CspDomainServicePlanCost(spcM).Return(retSPC, nil)
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"domain", "service-plan-cost", "-n", string(domObj.Name), "-P", string(resPlans.Payload[0].Name), "-o", "yaml"})
	assert.NotNil(te.yamlData)
	assert.EqualValues(retSPC.Payload, te.yamlData)
	mockCtrl.Finish()

	t.Log("case: service plan cost success (json)")
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadPlans)
	appCtx.API = mAPI
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(2)
	dOps.EXPECT().CspDomainList(lM).Return(lRet, nil)
	dOps.EXPECT().CspDomainServicePlanCost(spcM).Return(retSPC, nil)
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"domain", "service-plan-cost", "-n", string(domObj.Name), "-P", string(resPlans.Payload[0].Name), "-o", "json"})
	assert.NotNil(te.jsonData)
	assert.EqualValues(retSPC.Payload, te.jsonData)
	mockCtrl.Finish()

	t.Log("case: service plan cost failure")
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadPlans)
	appCtx.API = mAPI
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(2)
	dOps.EXPECT().CspDomainList(lM).Return(lRet, nil)
	apiErr := &csp_domain.CspDomainServicePlanCostDefault{
		Payload: &models.Error{
			Code:    404,
			Message: swag.String("spc-error"),
		},
	}
	dOps.EXPECT().CspDomainServicePlanCost(spcM).Return(nil, apiErr)
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"domain", "service-plan-cost", "-n", string(domObj.Name), "-P", string(resPlans.Payload[0].Name)})
	assert.Error(err)
	assert.Regexp("spc-error", err)
	mockCtrl.Finish()

	t.Log("case: service plan not found")
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadPlans)
	appCtx.API = mAPI
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(lM).Return(lRet, nil)
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"domain", "service-plan-cost", "-n", string(domObj.Name), "-P", string(resPlans.Payload[0].Name) + "foo"})
	assert.Error(err)
	assert.Regexp("service plan.* not found", err)
	mockCtrl.Finish()

	t.Log("case: cache service plan error")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	ppm := mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	pOps := mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(pOps)
	pOps.EXPECT().ServicePlanList(ppm).Return(nil, fmt.Errorf("cache-plans"))
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(lM).Return(lRet, nil)
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"domain", "service-plan-cost", "-n", string(domObj.Name), "-P", string(resPlans.Payload[0].Name)})
	assert.Error(err)
	assert.Regexp("cache-plans", err)
	mockCtrl.Finish()

	t.Log("case: dom not found")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	lRet.Payload = []*models.CSPDomain{}
	dOps.EXPECT().CspDomainList(lM).Return(lRet, nil)
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"domain", "service-plan-cost", "-n", string(domObj.Name), "-P", string(resPlans.Payload[0].Name)})
	assert.Error(err)
	assert.Regexp("CSPDomain.* not found", err)
	mockCtrl.Finish()

	t.Log("case: dom list failure")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(lM).Return(nil, fmt.Errorf("list-error"))
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"domain", "service-plan-cost", "-n", string(domObj.Name), "-P", string(resPlans.Payload[0].Name)})
	assert.Error(err)
	assert.Regexp("list-error", err)
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
	initCspDomain()
	err = parseAndRun([]string{"domain", "service-plan-cost", "-A", "System", "-n", string(domObj.Name), "-P", string(resPlans.Payload[0].Name)})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspDomain()
	err = parseAndRun([]string{"domain", "service-plan-cost", "-n", string(domObj.Name), "-P", string(resPlans.Payload[0].Name), "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}
