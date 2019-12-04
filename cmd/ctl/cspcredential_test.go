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
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestCspCredentialMakeRecord(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	cc := &cspCredentialCmd{}
	cc.accounts = map[string]ctxIDName{
		"accountID": {id: "", name: "tenant 1"},
		"subId1":    {id: "accountID", name: "sub 1"},
		"subId2":    {id: "accountID", name: "sub 2"},
	}
	o := &models.CSPCredential{
		CSPCredentialAllOf0: models.CSPCredentialAllOf0{
			Meta: &models.ObjMeta{
				ID:           "id",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
			AccountID:     "accountID",
			CspDomainType: models.CspDomainTypeMutable("AWS"),
		},
		CSPCredentialMutable: models.CSPCredentialMutable{
			Name:        "MyAWS",
			Description: "AWS credential object",
			Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
			CredentialAttributes: map[string]models.ValueType{
				"aws_access_key_id":     models.ValueType{Kind: "STRING", Value: "myAwsId"},
				"aws_secret_access_key": models.ValueType{Kind: "SECRET", Value: "secret"},
			},
		},
	}
	rec := cc.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(cspCredentialHeaders))
	assert.Equal("id", rec[hID])
	assert.Equal("1", rec[hVersion])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeCreated])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeModified])
	assert.Equal("AWS", rec[hCspDomainType])
	assert.Equal("MyAWS", rec[hName])
	assert.Equal("AWS credential object", rec[hDescription])
	assert.Equal("tag1, tag2, tag3", rec[hTags])
	assert.Equal("tenant 1", rec[hAccount])
	// note sorted order of attribute name
	al := "aws_access_key_id[S]: myAwsId\n" + "aws_secret_access_key[E]: ***"
	assert.Equal(al, rec[hCredentialAttributes])

	// no caches
	cc.accounts = map[string]ctxIDName{}
	rec = cc.makeRecord(o)
	t.Log(rec)
	assert.Equal("accountID", rec[hAccount])
}

func TestCspCredentialList(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	params := csp_credential.NewCspCredentialListParams()
	params.Name = swag.String("")
	params.CspDomainType = swag.String("")
	m := mockmgmtclient.NewCspCredentialMatcher(t, params)
	res := &csp_credential.CspCredentialListOK{
		Payload: []*models.CSPCredential{
			&models.CSPCredential{
				CSPCredentialAllOf0: models.CSPCredentialAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id1",
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
			&models.CSPCredential{
				CSPCredentialAllOf0: models.CSPCredentialAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id2",
						Version:      2,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
					CspDomainType: models.CspDomainTypeMutable("AWS"),
				},
				CSPCredentialMutable: models.CSPCredentialMutable{
					Name:        "cspCredential2",
					Description: "cspCredential2 object",
					Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
					CredentialAttributes: map[string]models.ValueType{
						"aws_access_key_id":     models.ValueType{Kind: "STRING", Value: "id2"},
						"aws_secret_access_key": models.ValueType{Kind: "SECRET", Value: "secret2"},
					},
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

	aM := mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	// list, all defaults
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps)
	cOps.EXPECT().CspCredentialList(m).Return(res, nil)
	aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err := parseAndRun([]string{"csp-credential", "list"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(cspCredentialDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list default, json
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps)
	cOps.EXPECT().CspCredentialList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "list", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(res.Payload, te.jsonData)

	// list default, yaml
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps)
	cOps.EXPECT().CspCredentialList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "list", "-o", "yaml"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(res.Payload, te.yamlData)

	// list with columns, valid account
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "Nuvoloso").Return("tid1", nil)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps)
	cOps.EXPECT().CspCredentialList(m).Return(res, nil)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "list", "-A", "Nuvoloso", "--columns", "Name,ID"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, 2)
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))
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
	initCspCredential()
	err = parseAndRun([]string{"credential", "list", "-c", "Name,ID,Foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// list, API failure model error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps)
	apiErr := &csp_credential.CspCredentialListDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	cOps.EXPECT().CspCredentialList(m).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"cred", "list"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// list, API failure arbitrary error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps)
	otherErr := fmt.Errorf("OTHER ERROR")
	cOps.EXPECT().CspCredentialList(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "list"})
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
	initCspCredential()
	err = parseAndRun([]string{"credential", "list", "-A", "System"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// helper list with no params
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	dm := mockmgmtclient.NewCspCredentialMatcher(t, csp_credential.NewCspCredentialListParams())
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps)
	cOps.EXPECT().CspCredentialList(dm).Return(&csp_credential.CspCredentialListOK{}, nil)
	appCtx.API = mAPI
	cc := &cspCredentialCmd{}
	_, err = cc.list(nil)
	assert.NoError(err)

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "list", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestCspCredentialCreate(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	awsCredAttrs := map[string]models.ValueType{
		"aws_access_key_id":     models.ValueType{Kind: "STRING", Value: "myAwsId"},
		"aws_secret_access_key": models.ValueType{Kind: "SECRET", Value: "secret"},
	}
	gcCredsFileContent, _ := ioutil.ReadFile("./gc_creds.json")
	gcCredAttrs := map[string]models.ValueType{
		"gc_cred": models.ValueType{Kind: "SECRET", Value: string(gcCredsFileContent)},
	}

	params := csp_credential.NewCspCredentialCreateParams()
	params.Payload = &models.CSPCredential{
		CSPCredentialAllOf0: models.CSPCredentialAllOf0{
			AccountID: "id1",
		},
		CSPCredentialMutable: models.CSPCredentialMutable{
			Name:        models.ObjName("name"),
			Description: models.ObjDescription("description"),
			Tags:        []string{"tag1", "tag2"},
		},
	}
	params.Payload.CspDomainType = models.CspDomainTypeMutable("AWS")
	params.Payload.CredentialAttributes = awsCredAttrs
	res := &csp_credential.CspCredentialCreateCreated{
		Payload: params.Payload,
	}
	mdAWS := &csp_credential.CspCredentialMetadataOK{
		Payload: &models.CSPCredentialMetadata{
			AttributeMetadata: map[string]models.AttributeDescriptor{
				"aws_access_key_id": models.AttributeDescriptor{
					Kind:     "STRING",
					Optional: false,
				},
				"aws_secret_access_key": models.AttributeDescriptor{
					Kind:     "SECRET",
					Optional: true,
				},
			},
		},
	}
	mdGC := &csp_credential.CspCredentialMetadataOK{
		Payload: &models.CSPCredentialMetadata{
			AttributeMetadata: map[string]models.AttributeDescriptor{
				"gc_cred": models.AttributeDescriptor{
					Kind:     "SECRET",
					Optional: true,
				},
			},
		},
	}
	mdRes := mdAWS
	mdParams := csp_credential.NewCspCredentialMetadataParams()
	mdParams.CspDomainType = "AWS"

	m := mockmgmtclient.NewCspCredentialMatcher(t, params)

	// create with attrs list
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "nuvoloso").Return("id1", nil)
	cOps := mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	cOps.EXPECT().CspCredentialMetadata(mdParams).Return(mdRes, nil)
	cOps.EXPECT().CspCredentialCreate(m).Return(res, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err := parseAndRun([]string{"csp-credential", "create", "-A", "nuvoloso", "-n", string(params.Payload.Name),
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-a", "aws_access_key_id:myAwsId", "-a", "aws_secret_access_key:secret",
		"-T", "AWS", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.CSPCredential{params.Payload}, te.jsonData)

	// successful create with creds file for GC (Google Cloud) domain type
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	params.Payload.CredentialAttributes = gcCredAttrs
	params.Payload.CspDomainType = models.CspDomainTypeMutable("GCP")
	mdParams.CspDomainType = "GCP"
	mdRes = mdGC
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	cOps.EXPECT().CspCredentialMetadata(mdParams).Return(mdRes, nil)
	cOps.EXPECT().CspCredentialCreate(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "create", "-A", "nuvoloso", "-n", string(params.Payload.Name),
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-F", "./gc_creds.json",
		"-T", "GCP", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.CSPCredential{params.Payload}, te.jsonData)
	params.Payload.CredentialAttributes = awsCredAttrs                // restore
	params.Payload.CspDomainType = models.CspDomainTypeMutable("AWS") // restore

	// bad creds file for GC (Google Cloud) domain type
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	cOps.EXPECT().CspCredentialMetadata(mdParams).Return(mdRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "create", "-A", "nuvoloso", "-n", string(params.Payload.Name),
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-F", "./IDoNotExist",
		"-T", "GCP", "-o", "json"})
	assert.Error(err)
	assert.Regexp("no such file or directory", err)
	mdParams.CspDomainType = "AWS" // restore
	mdRes = mdAWS                  // restore

	// create, invalid columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "create", "-n", string(params.Payload.Name),
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-a", "aws_access_key_id:myAwsId", "-a", "aws_secret_access_key:secret",
		"-T", "AWS", "--columns", "Name,ID,FOO"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// create, API model error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	cOps.EXPECT().CspCredentialMetadata(mdParams).Return(mdRes, nil)
	apiErr := &csp_credential.CspCredentialCreateDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	cOps.EXPECT().CspCredentialCreate(m).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "create", "-n", string(params.Payload.Name),
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-a", "aws_access_key_id:myAwsId", "-a", "aws_secret_access_key:secret",
		"-T", "AWS", "-o", "json"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// create, API failure arbitrary error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	otherErr := fmt.Errorf("OTHER ERROR")
	cOps.EXPECT().CspCredentialMetadata(mdParams).Return(mdRes, nil)
	cOps.EXPECT().CspCredentialCreate(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "create", "-n", string(params.Payload.Name),
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-a", "aws_access_key_id:myAwsId", "-a", "aws_secret_access_key:secret",
		"-T", "AWS", "-o", "json"})
	assert.NotNil(err)
	assert.Equal(otherErr, err)
	appCtx.Account, appCtx.AccountID = "", ""

	// create, API failure to convert attributes
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	otherErr = fmt.Errorf("ANOTHER ERROR")
	cOps.EXPECT().CspCredentialMetadata(mdParams).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "create", "-n", string(params.Payload.Name),
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-a", "aws_access_key_id:myAwsId", "-a", "aws_secret_access_key:secret",
		"-T", "AWS", "-o", "json"})
	assert.NotNil(err)
	assert.Equal(otherErr, err)
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
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "create", "-A", "System", "-n", string(params.Payload.Name),
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-a", "aws_access_key_id:myAwsId", "-a", "aws_secret_access_key:secret",
		"-T", "AWS", "-o", "json"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("missing required arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "create", "-n", string(params.Payload.Name),
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-T", "AWS"})
	assert.NotNil(err)
	assert.Regexp("Attributes are required for CSPCredential object of domain type AWS", err)
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "create", "-n", string(params.Payload.Name),
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-a", "aws_access_key_id:myAwsId", "-a", "aws_secret_access_key:secret",
		"-T", "AWS", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestCSPCredentialDelete(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	lRes := &csp_credential.CspCredentialListOK{
		Payload: []*models.CSPCredential{
			&models.CSPCredential{
				CSPCredentialAllOf0: models.CSPCredentialAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id1",
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
	lParams := csp_credential.NewCspCredentialListParams()
	lParams.Name = swag.String(string(lRes.Payload[0].Meta.ID))
	m := mockmgmtclient.NewCspCredentialMatcher(t, lParams)
	dParams := csp_credential.NewCspCredentialDeleteParams()
	dParams.ID = string(lRes.Payload[0].Meta.ID)
	dRet := &csp_credential.CspCredentialDeleteNoContent{}

	// delete
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	cOps.EXPECT().CspCredentialList(m).Return(lRes, nil)
	cOps.EXPECT().CspCredentialDelete(dParams).Return(dRet, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err := parseAndRun([]string{"csp-credential", "delete", "-n", string(*lParams.Name), "--confirm"})
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
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "delete", "-n", string(*lParams.Name)})
	assert.NotNil(err)
	assert.Regexp("--confirm", err.Error())

	// delete, not found
	emptyRes := &csp_credential.CspCredentialListOK{
		Payload: []*models.CSPCredential{},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	cOps.EXPECT().CspCredentialList(m).Return(emptyRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "delete", "-n", string(*lParams.Name), "--confirm"})
	assert.NotNil(err)
	assert.Regexp("not found", err.Error())

	// delete, list error
	otherErr := fmt.Errorf("other error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	cOps.EXPECT().CspCredentialList(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "delete", "-n", string(*lParams.Name), "--confirm"})
	assert.NotNil(err)
	assert.Regexp("other error", err.Error())

	// delete, API model error
	apiErr := &csp_credential.CspCredentialDeleteDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	cOps.EXPECT().CspCredentialList(m).Return(lRes, nil)
	cOps.EXPECT().CspCredentialDelete(dParams).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "delete", "-n", string(*lParams.Name), "--confirm"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// delete, API arbitrary error, valid account
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "Nuvoloso").Return("tid1", nil)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	cOps.EXPECT().CspCredentialList(m).Return(lRes, nil)
	cOps.EXPECT().CspCredentialDelete(dParams).Return(nil, otherErr)
	lParams.AccountID = swag.String("tid1")
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "-A", "Nuvoloso", "delete", "-n", string(*lParams.Name), "--confirm"})
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
	initCspCredential()
	err = parseAndRun([]string{"credential", "delete", "-A", "System", "-n", string(*lParams.Name), "--confirm"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "delete", "-n", string(*lParams.Name), "confirm"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestCSPCredentialModify(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	lRes := &csp_credential.CspCredentialListOK{
		Payload: []*models.CSPCredential{
			&models.CSPCredential{
				CSPCredentialAllOf0: models.CSPCredentialAllOf0{
					AccountID: "tenantAID",
					Meta: &models.ObjMeta{
						ID:           "id1",
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
	mdRes := &csp_credential.CspCredentialMetadataOK{
		Payload: &models.CSPCredentialMetadata{
			AttributeMetadata: map[string]models.AttributeDescriptor{
				"aws_access_key_id": models.AttributeDescriptor{
					Kind:     "STRING",
					Optional: false,
				},
				"aws_secret_access_key": models.AttributeDescriptor{
					Kind:     "SECRET",
					Optional: true,
				},
			},
		},
	}
	mdParams := csp_credential.NewCspCredentialMetadataParams()
	mdParams.CspDomainType = "AWS"

	lParams := csp_credential.NewCspCredentialListParams()
	lParams.Name = swag.String(string(lRes.Payload[0].Meta.ID))
	lParams.AccountID = swag.String(string(lRes.Payload[0].AccountID))
	lm := mockmgmtclient.NewCspCredentialMatcher(t, lParams)
	lParams2 := csp_credential.NewCspCredentialListParams()
	lParams2.Name = swag.String(string(lRes.Payload[0].Meta.ID))
	lm2 := mockmgmtclient.NewCspCredentialMatcher(t, lParams2)

	// update (APPEND)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "Nuvoloso").Return("tenantAID", nil)
	cOps := mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	cOps.EXPECT().CspCredentialMetadata(mdParams).Return(mdRes, nil)
	cOps.EXPECT().CspCredentialList(lm).Return(lRes, nil)
	uParams := csp_credential.NewCspCredentialUpdateParams()
	uParams.Payload = &models.CSPCredentialMutable{
		Name:        "newName",
		Description: "newDescription",
		CredentialAttributes: map[string]models.ValueType{
			"aws_access_key_id":     models.ValueType{Kind: "STRING", Value: "myAwsId"},
			"aws_secret_access_key": models.ValueType{Kind: "SECRET", Value: "secret"},
		},
		Tags: []string{"tag1", "tag2"},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"name", "description"}
	uParams.Append = []string{"credentialAttributes", "tags"}
	um := mockmgmtclient.NewCspCredentialMatcher(t, uParams)
	uRet := csp_credential.NewCspCredentialUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().CspCredentialUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err := parseAndRun([]string{"csp-credential", "-A", "Nuvoloso", "modify", "-n", string(*lParams.Name),
		"-N", "newName", "-d", "newDescription",
		"-a", "aws_access_key_id:myAwsId", "-a", "aws_secret_access_key:secret",
		"-t", "tag1", "-t", "tag2", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.CSPCredential{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// same but with SET
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "Nuvoloso").Return("tenantAID", nil)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	cOps.EXPECT().CspCredentialMetadata(mdParams).Return(mdRes, nil)
	cOps.EXPECT().CspCredentialList(lm).Return(lRes, nil)
	uParams = csp_credential.NewCspCredentialUpdateParams()
	uParams.Payload = &models.CSPCredentialMutable{
		Name:        "newName",
		Description: "newDescription",
		CredentialAttributes: map[string]models.ValueType{
			"aws_access_key_id":     models.ValueType{Kind: "STRING", Value: "myAwsId"},
			"aws_secret_access_key": models.ValueType{Kind: "SECRET", Value: "secret"},
		},
		Tags: []string{"tag1", "tag2"},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"name", "description", "credentialAttributes", "tags"}
	um = mockmgmtclient.NewCspCredentialMatcher(t, uParams)
	uRet = csp_credential.NewCspCredentialUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().CspCredentialUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "-A", "Nuvoloso", "modify", "-n", string(*lParams.Name),
		"-N", "newName", "-d", "newDescription",
		"-a", "aws_access_key_id:myAwsId", "-a", "aws_secret_access_key:secret", "--attribute-action", "SET",
		"-t", "tag1", "-t", "tag2", "--tag-action", "SET", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.CSPCredential{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// same but with REMOVE
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "Nuvoloso").Return("tenantAID", nil)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	cOps.EXPECT().CspCredentialMetadata(mdParams).Return(mdRes, nil)
	cOps.EXPECT().CspCredentialList(lm).Return(lRes, nil)
	uParams = csp_credential.NewCspCredentialUpdateParams()
	uParams.Payload = &models.CSPCredentialMutable{
		Name:        "newName",
		Description: "newDescription",
		CredentialAttributes: map[string]models.ValueType{
			"aws_access_key_id":     models.ValueType{Kind: "STRING", Value: "myAwsId"},
			"aws_secret_access_key": models.ValueType{Kind: "SECRET", Value: "secret"},
		},
		Tags: []string{"tag1", "tag2"},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"name", "description"}
	uParams.Remove = []string{"credentialAttributes", "tags"}
	um = mockmgmtclient.NewCspCredentialMatcher(t, uParams)
	uRet = csp_credential.NewCspCredentialUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().CspCredentialUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "-A", "Nuvoloso", "modify", "-n", string(*lParams.Name),
		"-N", "newName", "-d", "newDescription",
		"-a", "aws_access_key_id:myAwsId", "-a", "aws_secret_access_key:secret", "--attribute-action", "REMOVE",
		"-t", "tag1", "-t", "tag2", "--tag-action", "REMOVE", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.CSPCredential{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// same, but empty SET
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	cOps.EXPECT().CspCredentialList(lm2).Return(lRes, nil)
	uParams = csp_credential.NewCspCredentialUpdateParams()
	uParams.Payload = &models.CSPCredentialMutable{
		Name:        "newName",
		Description: "newDescription",
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"name", "description", "credentialAttributes", "tags"}
	um = mockmgmtclient.NewCspCredentialMatcher(t, uParams)
	uRet = csp_credential.NewCspCredentialUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().CspCredentialUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "modify", "-n", string(*lParams2.Name),
		"-N", "newName", "-d", "newDescription",
		"--attribute-action", "SET",
		"--tag-action", "SET", "-V", "1", "-o", "json",
	})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.CSPCredential{uRet.Payload}, te.jsonData)

	// no changes
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	cOps.EXPECT().CspCredentialList(lm2).Return(lRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "modify", "-n", string(*lParams2.Name), "-V", "1", "-o", "json"})
	assert.NotNil(err)
	assert.Nil(te.jsonData)
	assert.Regexp("No modifications", err.Error())

	// failure to convert attributes
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	cOps.EXPECT().CspCredentialList(lm2).Return(lRes, nil)
	cOps.EXPECT().CspCredentialMetadata(mdParams).Return(nil, fmt.Errorf("metadata error"))
	uParams = csp_credential.NewCspCredentialUpdateParams()
	uParams.Payload = &models.CSPCredentialMutable{
		Name:        "newName",
		Description: "newDescription",
		Tags:        []string{"tag1", "tag2"},
		CredentialAttributes: map[string]models.ValueType{
			"aws_access_key_id":     models.ValueType{Kind: "STRING", Value: "myAwsId"},
			"aws_secret_access_key": models.ValueType{Kind: "SECRET", Value: "secret"},
		},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"name", "description"}
	uParams.Remove = []string{"tags", "credentialAttributes"}
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "modify", "-n", string(*lParams2.Name),
		"-N", "newName", "-d", "newDescription",
		"-a", "aws_access_key_id:myAwsId", "-a", "aws_secret_access_key:secret", "--attribute-action", "REMOVE",
		"-t", "tag1", "-t", "tag2", "--tag-action", "REMOVE", "-V", "1", "-o", "json",
	})
	assert.NotNil(err)
	assert.Regexp("metadata error", err.Error())

	// update error
	mockCtrl.Finish()
	updateErr := csp_credential.NewCspCredentialUpdateDefault(400)
	updateErr.Payload = &models.Error{Code: 400, Message: swag.String("update error")}
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	cOps.EXPECT().CspCredentialList(lm2).Return(lRes, nil)
	uParams = csp_credential.NewCspCredentialUpdateParams()
	uParams.Payload = &models.CSPCredentialMutable{
		Name:        "newName",
		Description: "newDescription",
		Tags:        []string{"tag1", "tag2"},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"name", "description"}
	uParams.Remove = []string{"tags"}
	um = mockmgmtclient.NewCspCredentialMatcher(t, uParams)
	cOps.EXPECT().CspCredentialUpdate(um).Return(nil, updateErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "modify", "-n", string(*lParams2.Name),
		"-N", "newName", "-d", "newDescription",
		"-t", "tag1", "-t", "tag2", "--tag-action", "REMOVE", "-V", "1", "-o", "json",
	})
	assert.NotNil(err)
	assert.Regexp("update error", err.Error())

	// other error on update
	otherErr := fmt.Errorf("other error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	cOps.EXPECT().CspCredentialList(lm2).Return(lRes, nil)
	uParams = csp_credential.NewCspCredentialUpdateParams()
	uParams.Payload = &models.CSPCredentialMutable{
		Name:        "newName",
		Description: "newDescription",
		Tags:        []string{"tag1", "tag2"},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"name", "description"}
	uParams.Remove = []string{"tags"}
	um = mockmgmtclient.NewCspCredentialMatcher(t, uParams)
	cOps.EXPECT().CspCredentialUpdate(um).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "modify", "-n", string(*lParams2.Name),
		"-N", "newName", "-d", "newDescription",
		"-t", "tag1", "-t", "tag2", "--tag-action", "REMOVE", "-V", "1", "-o", "json",
	})
	assert.NotNil(err)
	assert.Regexp("other error", err.Error())

	// invalid columns
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "modify", "-n", string(*lParams2.Name), "--columns", "ID,foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err.Error())

	// not found
	mockCtrl.Finish()
	emptyRes := &csp_credential.CspCredentialListOK{
		Payload: []*models.CSPCredential{},
	}
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	cOps.EXPECT().CspCredentialList(lm2).Return(emptyRes, nil)
	appCtx.API = mAPI
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "modify", "-n", string(*lParams2.Name),
		"-N", "newName", "-d", "newDescription",
		"-t", "tag1", "-t", "tag2", "--tag-action", "SET", "-V", "1", "-o", "json",
	})
	assert.NotNil(err)
	assert.Regexp("not found", err.Error())

	// list error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	cOps.EXPECT().CspCredentialList(lm2).Return(nil, otherErr)
	appCtx.API = mAPI
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "modify", "-n", string(*lParams2.Name),
		"-N", "newName", "-d", "newDescription",
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
	initCspCredential()
	err = parseAndRun([]string{"credential", "modify", "-A", "System", "-n", string(*lParams.Name)})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "-A", "Nuvoloso", "modify", "-n", string(*lParams.Name),
		"-N", "newName", "-d", "newDescription",
		"-a", "aws_access_key_id:myAwsId", "-a", "aws_secret_access_key:secret",
		"-t", "tag1", "-t", "tag2", "-V", "1", "o", "json"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestCSPCredentialMetadata(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	mdRes := &csp_credential.CspCredentialMetadataOK{
		Payload: &models.CSPCredentialMetadata{
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
	mdParams := csp_credential.NewCspCredentialMetadataParams()
	mdParams.CspDomainType = "AWS"

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	cOps.EXPECT().CspCredentialMetadata(mdParams).Return(mdRes, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err := parseAndRun([]string{"csp-credential", "metadata"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(cspCredentialMetadataCols))
	assert.Equal(cspCredentialMetadataCols, te.tableHeaders)
	t.Log(te.tableData)
	assert.Len(te.tableData, len(mdRes.Payload.AttributeMetadata))
	assert.Equal(expTd, te.tableData)

	// success, json, GCP type
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	mdParams.CspDomainType = "GCP"
	cOps.EXPECT().CspCredentialMetadata(mdParams).Return(mdRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "metadata", "-T", "GCP", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(mdRes.Payload, te.jsonData)
	mdParams.CspDomainType = "AWS" // reset

	// success, yaml
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	cOps.EXPECT().CspCredentialMetadata(mdParams).Return(mdRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "md", "-T", "AWS", "-o", "yaml"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(mdRes.Payload, te.yamlData)

	// failure, invalid credential type
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	mdParams.CspDomainType = "Azure"
	apiErr := csp_credential.NewCspCredentialMetadataDefault(400)
	apiErr.Payload = &models.Error{Message: swag.String("invalid"), Code: 400}
	cOps.EXPECT().CspCredentialMetadata(mdParams).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "md", "-T", "Azure"})
	assert.Error(err)
	assert.Regexp("invalid", err.Error())
	mdParams.CspDomainType = "AWS" // reset

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initCspCredential()
	err = parseAndRun([]string{"csp-credential", "metadata", "o", "json"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestConvertAttrs(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	mdRes := &csp_credential.CspCredentialMetadataOK{
		Payload: &models.CSPCredentialMetadata{
			AttributeMetadata: map[string]models.AttributeDescriptor{
				"aws_access_key_id": models.AttributeDescriptor{
					Kind:     "STRING",
					Optional: false,
				},
				"aws_secret_access_key": models.AttributeDescriptor{
					Kind:     "SECRET",
					Optional: true,
				},
			},
		},
	}
	mdParams := csp_credential.NewCspCredentialMetadataParams()
	mdParams.CspDomainType = "AWS"

	cspDomainAttrKinds := map[string]string{
		"aws_access_key_id":     "STRING",
		"aws_secret_access_key": "SECRET",
	}
	attrs := map[string]string{
		"aws_access_key_id":     "my_key",
		"aws_secret_access_key": "my_secret",
	}
	attrsWithUnknown := map[string]string{
		"aws_access_key_id":     "my_key",
		"aws_secret_access_key": "my_secret",
		"UNKNOWN_KEY":           "MY_UNKNOWN_KEY_VALUE",
	}
	expectedAttrs := map[string]models.ValueType{
		"aws_access_key_id":     models.ValueType{Kind: "STRING", Value: "my_key"},
		"aws_secret_access_key": models.ValueType{Kind: "SECRET", Value: "my_secret"},
	}
	expectedAttrsWithUnknown := map[string]models.ValueType{
		"aws_access_key_id":     models.ValueType{Kind: "STRING", Value: "my_key"},
		"aws_secret_access_key": models.ValueType{Kind: "SECRET", Value: "my_secret"},
		"UNKNOWN_KEY":           models.ValueType{Kind: "STRING", Value: "MY_UNKNOWN_KEY_VALUE"},
	}
	cc := &cspCredentialCmd{}

	// cache present
	cc.cspDomainAttrKinds = cspDomainAttrKinds
	convertedAttrs, err := cc.convertAttrs(mdParams.CspDomainType, attrs, "")
	assert.NoError(err)
	assert.Equal(expectedAttrs, convertedAttrs)

	// unknown attribute
	convertedAttrs, err = cc.convertAttrs(mdParams.CspDomainType, attrsWithUnknown, "")
	assert.NoError(err)
	assert.Equal(expectedAttrsWithUnknown, convertedAttrs)

	// unexpected attributes
	convertedAttrs, err = cc.convertAttrs("GCP", attrsWithUnknown, "some_file_path")
	assert.Error(err)
	assert.Regexp("Unexpected attributes for domain type GCP", err.Error())

	// no cache
	cc.cspDomainAttrKinds = nil
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	cOps.EXPECT().CspCredentialMetadata(mdParams).Return(mdRes, nil)
	appCtx.API = mAPI
	convertedAttrs, err = cc.convertAttrs(mdParams.CspDomainType, attrs, "")
	assert.NoError(err)
	t.Log(convertedAttrs)
	assert.Equal(expectedAttrs, convertedAttrs)
	assert.Equal(cspDomainAttrKinds, cc.cspDomainAttrKinds)

	// failure to retrieve metadata
	cc.cspDomainAttrKinds = nil
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockCSPCredentialClient(mockCtrl)
	mAPI.EXPECT().CspCredential().Return(cOps).MinTimes(1)
	apiErr := csp_credential.NewCspCredentialMetadataDefault(400)
	apiErr.Payload = &models.Error{Message: swag.String("invalid"), Code: 400}
	cOps.EXPECT().CspCredentialMetadata(mdParams).Return(nil, apiErr)
	appCtx.API = mAPI
	convertedAttrs, err = cc.convertAttrs(mdParams.CspDomainType, attrs, "")
	assert.Error(err)
	t.Log(convertedAttrs)
}
