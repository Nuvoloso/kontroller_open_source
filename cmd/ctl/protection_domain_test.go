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
	"os"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/protection_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestProtectionDomainMakeRecord(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	pdc := &protectionDomainCmd{}
	pdc.accounts = map[string]ctxIDName{
		"accountID": {id: "", name: "tenant 1"},
		"subId1":    {id: "accountID", name: "sub 1"},
		"subId2":    {id: "accountID", name: "sub 2"},
	}

	o := &models.ProtectionDomain{
		ProtectionDomainAllOf0: models.ProtectionDomainAllOf0{
			Meta: &models.ObjMeta{
				ID:           "id",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
		},
		ProtectionDomainCreateOnce: models.ProtectionDomainCreateOnce{
			AccountID:            "accountID",
			EncryptionAlgorithm:  "AES-256",
			EncryptionPassphrase: &models.ValueType{Kind: "SECRET", Value: "my password"},
		},
		ProtectionDomainMutable: models.ProtectionDomainMutable{
			Name:        "My PD",
			Description: "PD key",
			SystemTags:  models.ObjTags{"stag1", "stag2"},
			Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
		},
	}
	rec := pdc.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(protectionDomainHeaders))
	assert.Equal("id", rec[hID])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeCreated])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeModified])
	assert.Equal("tenant 1", rec[hAccount])
	assert.Equal("My PD", rec[hName])
	assert.Equal("PD key", rec[hDescription])
	assert.Equal("tag1, tag2, tag3", rec[hTags])

	// no caches
	pdc.accounts = map[string]ctxIDName{}
	rec = pdc.makeRecord(o)
	t.Log(rec)
	assert.Equal("accountID", rec[hAccount])
}

func TestProtectionDomainCreate(t *testing.T) {
	assert := assert.New(t)
	origPasswordHook := passwordHook
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
		passwordHook = origPasswordHook
	}()
	interactiveReader = nil
	var hookCalled bool
	var hookRet []byte
	var hookErr error
	passwordHook = func(int) ([]byte, error) {
		hookCalled = true
		return hookRet, hookErr
	}

	now := time.Now()
	res := &protection_domain.ProtectionDomainCreateCreated{
		Payload: &models.ProtectionDomain{
			ProtectionDomainAllOf0: models.ProtectionDomainAllOf0{
				Meta: &models.ObjMeta{
					ID:           "id",
					Version:      1,
					TimeCreated:  strfmt.DateTime(now),
					TimeModified: strfmt.DateTime(now),
				},
			},
			ProtectionDomainCreateOnce: models.ProtectionDomainCreateOnce{
				AccountID:            "accountID",
				EncryptionAlgorithm:  "AES-256",
				EncryptionPassphrase: &models.ValueType{Kind: "SECRET", Value: "my password"},
			},
			ProtectionDomainMutable: models.ProtectionDomainMutable{
				Name:        "My PD",
				Description: "PD key",
				SystemTags:  models.ObjTags{"stag1", "stag2"},
				Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
			},
		},
	}

	// success cases
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	var mAPI *mockmgmtclient.MockAPI
	tcs := []string{"all", "pp-stdin", "pp-blank-arg", "pp-blank-stdin", "pp-missing-ok-cols"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		appCtx.Account, appCtx.AccountID = "", ""
		mAPI = nil
		params := protection_domain.NewProtectionDomainCreateParams()
		params.Payload = &models.ProtectionDomainCreateArgs{}
		var cmdArgs []string
		tableDataColumns := protectionDomainDefaultHeaders
		hasJSON := false
		hasYaml := false
		ppStdin := false
		hookCalled = false
		hookRet = nil
		hookErr = nil
		var b bytes.Buffer
		outputWriter = &b
		t.Log("case:", tc)
		switch tc {
		case "all":
			cmdArgs = []string{"protection-domain", "create", "-n", "MyPd", "-d", "Description", "-E", "AES-256", "-p", " my secret ", "-t", "tag1", "-t", "tag2", "-c", "account,name,description"}
			params.Payload.Name = models.ObjName("MyPd")
			params.Payload.Description = models.ObjDescription("Description")
			params.Payload.EncryptionAlgorithm = "AES-256"
			params.Payload.EncryptionPassphrase = &models.ValueType{Kind: "SECRET", Value: "my secret"}
			params.Payload.Tags = []string{"tag1", "tag2"}
		case "pp-stdin":
			cmdArgs = []string{"protection-domain", "create", "-n", "MyPd", "-d", " \tdescription ", "-o", "json"}
			params.Payload.Name = models.ObjName("MyPd")
			params.Payload.Description = models.ObjDescription("description")
			params.Payload.EncryptionAlgorithm = "AES-256"
			params.Payload.EncryptionPassphrase = &models.ValueType{Kind: "SECRET", Value: "m y"}
			hookRet = []byte{' ', 'm', ' ', 'y', ' '}
			hasJSON = true
			ppStdin = true
		case "pp-blank-arg":
			cmdArgs = []string{"protection-domain", "create", "-n", "MyPd", "-E", "AES-256", "-p", " "}
			params.Payload.Name = models.ObjName("MyPd")
			params.Payload.EncryptionAlgorithm = "AES-256"
			params.Payload.EncryptionPassphrase = &models.ValueType{Kind: "SECRET", Value: ""}
		case "pp-blank-stdin":
			cmdArgs = []string{"protection-domain", "create", "-n", "MyPd", "-E", "AES-256", "-o", "yaml"}
			params.Payload.Name = models.ObjName("MyPd")
			params.Payload.EncryptionAlgorithm = "AES-256"
			params.Payload.EncryptionPassphrase = &models.ValueType{Kind: "SECRET", Value: ""}
			hookRet = []byte{' '}
			hasYaml = true
			ppStdin = true
		case "pp-missing-ok-cols":
			cmdArgs = []string{"protection-domain", "create", "-n", "MyPd", "-E", "NONE", "-c", "id"}
			params.Payload.Name = models.ObjName("MyPd")
			params.Payload.EncryptionAlgorithm = "NONE"
			params.Payload.EncryptionPassphrase = &models.ValueType{Kind: "SECRET", Value: ""}
			tableDataColumns = []string{hID}
		default:
			assert.Equal("", tc)
			continue
		}
		if mAPI == nil {
			if !(hasJSON || hasYaml) {
				mAPI = cacheLoadHelper(t, mockCtrl, loadAccounts)
			} else {
				mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			}
		}
		cOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
		mAPI.EXPECT().ProtectionDomain().Return(cOps)
		m := mockmgmtclient.NewProtectionDomainMatcher(t, params)
		cOps.EXPECT().ProtectionDomainCreate(m).Return(res, nil)
		appCtx.API = mAPI
		te := &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initProtectionDomain()
		err := parseAndRun(cmdArgs)
		assert.Nil(err)
		if ppStdin {
			assert.True(hookCalled)
			assert.Equal("Passphrase: \nRetype passphrase: \n", b.String())
		} else {
			assert.False(hookCalled)
			assert.Equal("", b.String())
		}
		if hasJSON {
			assert.NotNil(te.jsonData)
			assert.EqualValues([]*models.ProtectionDomain{res.Payload}, te.jsonData)
		} else if hasYaml {
			assert.NotNil(te.yamlData)
			assert.EqualValues([]*models.ProtectionDomain{res.Payload}, te.yamlData)
		} else {
			assert.Len(te.tableHeaders, len(tableDataColumns))
			assert.Len(te.tableData, 1)
		}
	}

	// error cases
	tcs = []string{"create-error", "pp-read-error", "init-context-error", "invalid-column", "extra-args", "missing-args"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		appCtx.Account, appCtx.AccountID = "", ""
		mAPI = nil
		var cmdArgs []string
		var expectedErr error
		ppStdin := false
		hookCalled = false
		hookRet = nil
		hookErr = nil
		errPat := ""
		t.Log("case:", tc)
		switch tc {
		case "create-error":
			cmdArgs = []string{"protection-domain", "create", "-n", "MyPd", "-E", "AES-256", "-p", " "}
			params := protection_domain.NewProtectionDomainCreateParams()
			params.Payload = &models.ProtectionDomainCreateArgs{}
			params.Payload.Name = models.ObjName("MyPd")
			params.Payload.EncryptionAlgorithm = "AES-256"
			params.Payload.EncryptionPassphrase = &models.ValueType{Kind: "SECRET", Value: ""}
			m := mockmgmtclient.NewProtectionDomainMatcher(t, params)
			e := &protection_domain.ProtectionDomainCreateDefault{
				Payload: &models.Error{Message: swag.String("create-error")},
			}
			cOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
			cOps.EXPECT().ProtectionDomainCreate(m).Return(nil, e)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			mAPI.EXPECT().ProtectionDomain().Return(cOps)
			errPat = "create-error"
		case "pp-read-error":
			cmdArgs = []string{"protection-domain", "create", "-n", "MyPd", "-d", " \tdescription ", "-o", "json"}
			ppStdin = true
			hookErr = fmt.Errorf("stdin-read-error")
			errPat = "stdin-read-error"
		case "init-context-error":
			cmdArgs = []string{"protection-domain", "create", "-n", "MyPd", "-E", "AES-256", "-p", " ", "-A", "System"}
			expectedErr = fmt.Errorf("ctx error")
			authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
			authOps.EXPECT().SetContextAccount("", "System").Return("", expectedErr)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			mAPI.EXPECT().Authentication().Return(authOps)
		case "invalid-column":
			cmdArgs = []string{"protection-domain", "create", "-n", "MyPd", "-E", "AES-256", "-p", " ", "-c", "+foo"}
		case "extra-args":
			cmdArgs = []string{"protection-domain", "create", "-n", "MyPd", "-E", "AES-256", "-p", " ", "foo"}
		case "missing-args":
			cmdArgs = []string{"protection-domain", "create"}
		default:
			assert.Equal("", tc)
			continue
		}
		appCtx.API = mAPI
		te := &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initProtectionDomain()
		err := parseAndRun(cmdArgs)
		assert.Error(err)
		if errPat != "" {
			assert.Regexp(errPat, err)
		}
		if expectedErr != nil {
			assert.Equal(expectedErr, err)
		}
		if ppStdin {
			assert.True(hookCalled)
		} else {
			assert.False(hookCalled)
		}
	}
}

func TestProtectionDomainList(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	res := &protection_domain.ProtectionDomainListOK{
		Payload: []*models.ProtectionDomain{
			&models.ProtectionDomain{
				ProtectionDomainAllOf0: models.ProtectionDomainAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				ProtectionDomainCreateOnce: models.ProtectionDomainCreateOnce{
					AccountID:            "accountID",
					EncryptionAlgorithm:  "AES-256",
					EncryptionPassphrase: &models.ValueType{Kind: "SECRET", Value: "my password"},
				},
				ProtectionDomainMutable: models.ProtectionDomainMutable{
					Name:        "My PD",
					Description: "PD key",
					SystemTags:  models.ObjTags{"stag1", "stag2"},
					Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
				},
			},
		},
	}

	// success cases
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	var mAPI *mockmgmtclient.MockAPI
	tcs := []string{"list-default", "json", "yaml", "list-with-flags", "list-with-cols", "account-flag", "account-from-context", "list-with-follow"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		appCtx.Account, appCtx.AccountID = "", ""
		mAPI = nil
		params := protection_domain.NewProtectionDomainListParams()
		fw := &fakeWatcher{t: t}
		appCtx.watcher = nil
		fos := &fakeOSExecutor{}
		appCtx.osExec = nil
		var cmdArgs []string
		tableDataColumns := protectionDomainDefaultHeaders
		hasJSON := false
		hasYaml := false
		t.Log("case:", tc)
		switch tc {
		case "list-default":
			cmdArgs = []string{"protection-domain", "list"}
		case "json":
			cmdArgs = []string{"pd", "list", "-o", "json"}
			hasJSON = true
		case "yaml":
			cmdArgs = []string{"protection-domain", "list", "-o", "yaml"}
			hasYaml = true
		case "list-with-flags":
			cmdArgs = []string{"protection-domain", "list", "-n", "name.*", "-t", "aTag"}
			params.NamePattern = swag.String("name.*")
			params.Tags = []string{"aTag"}
		case "list-with-cols":
			cmdArgs = []string{"protection-domain", "list", "-c", "id,account"}
			tableDataColumns = []string{hID, hAccount}
		case "account-flag":
			cmdArgs = []string{"protection-domain", "list", "--account-name", "AuthAccount"}
			params.AccountID = swag.String("authAccountID")
		case "account-from-context":
			mAPI = cacheLoadHelper(t, mockCtrl, loadContext, loadAccounts)
			cmdArgs = []string{"protection-domain", "list", "-A", "System"}
			params.AccountID = swag.String("accountID")
		case "list-with-follow":
			cmdArgs = []string{"protection-domain", "list", "-f"}
			appCtx.watcher = fw
			fw.CallCBCount = 1
			appCtx.osExec = fos
		default:
			assert.Equal("", tc)
			continue
		}
		if mAPI == nil {
			if !(hasJSON || hasYaml) {
				mAPI = cacheLoadHelper(t, mockCtrl, loadAccounts)
			} else {
				mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			}
		}
		cOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
		mAPI.EXPECT().ProtectionDomain().Return(cOps)
		m := mockmgmtclient.NewProtectionDomainMatcher(t, params)
		cOps.EXPECT().ProtectionDomainList(m).Return(res, nil)
		appCtx.API = mAPI
		te := &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initProtectionDomain()
		err := parseAndRun(cmdArgs)
		assert.Nil(err)
		if hasJSON {
			assert.NotNil(te.jsonData)
			assert.EqualValues(res.Payload, te.jsonData)
		} else if hasYaml {
			assert.NotNil(te.yamlData)
			assert.EqualValues(res.Payload, te.yamlData)
		} else {
			assert.Len(te.tableHeaders, len(tableDataColumns))
			assert.Len(te.tableData, len(res.Payload))
		}
		switch tc {
		case "list-with-follow":
			assert.NotNil(fw.InArgs)
			expWArgs := &models.CrudWatcherCreateArgs{
				Matchers: []*models.CrudMatcher{
					&models.CrudMatcher{
						URIPattern: "/protection-domains/?",
					},
				},
			}
			assert.Equal(expWArgs, fw.InArgs)
			assert.NotNil(fw.InCB)
			assert.Equal(1, fw.NumCalls)
			assert.NoError(fw.CBErr)
		}
	}

	// error cases
	tcs = []string{"list-error", "invalid-account-name", "init-context-error", "invalid-column", "extra-args"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		appCtx.Account, appCtx.AccountID = "", ""
		mAPI = nil
		params := protection_domain.NewProtectionDomainListParams()
		var cmdArgs []string
		var expectedErr error
		errPat := ""
		t.Log("case:", tc)
		switch tc {
		case "list-error":
			cmdArgs = []string{"protection-domain", "list"}
			cOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
			m := mockmgmtclient.NewProtectionDomainMatcher(t, params)
			e := &protection_domain.ProtectionDomainListDefault{
				Payload: &models.Error{Message: swag.String("list-error")},
			}
			cOps.EXPECT().ProtectionDomainList(m).Return(nil, e)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			mAPI.EXPECT().ProtectionDomain().Return(cOps)
		case "invalid-account-name":
			cmdArgs = []string{"protection-domain", "list", "--account-name", "FooBar"}
			mAPI = cacheLoadHelper(t, mockCtrl, loadAccounts)
			errPat = "account.*not found"
		case "init-context-error":
			cmdArgs = []string{"protection-domain", "list", "-A", "System"}
			expectedErr = fmt.Errorf("ctx error")
			authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
			authOps.EXPECT().SetContextAccount("", "System").Return("", expectedErr)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			mAPI.EXPECT().Authentication().Return(authOps)
		case "invalid-column":
			cmdArgs = []string{"protection-domain", "list", "-c", "+foo"}
		case "extra-args":
			cmdArgs = []string{"protection-domain", "list", "foo"}
		default:
			assert.Equal("", tc)
			continue
		}
		appCtx.API = mAPI
		te := &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initProtectionDomain()
		err := parseAndRun(cmdArgs)
		assert.Error(err)
		if errPat != "" {
			assert.Regexp(errPat, err)
		}
		if expectedErr != nil {
			assert.Equal(expectedErr, err)
		}
	}
}

func TestProtectionDomainDelete(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	o := &models.ProtectionDomain{
		ProtectionDomainAllOf0: models.ProtectionDomainAllOf0{
			Meta: &models.ObjMeta{
				ID:           "id",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
		},
		ProtectionDomainCreateOnce: models.ProtectionDomainCreateOnce{
			AccountID:            "accountID",
			EncryptionAlgorithm:  "AES-256",
			EncryptionPassphrase: &models.ValueType{Kind: "SECRET", Value: "my password"},
		},
		ProtectionDomainMutable: models.ProtectionDomainMutable{
			Name:        "My PD",
			Description: "PD key",
			SystemTags:  models.ObjTags{"stag1", "stag2"},
			Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
		},
	}
	lRes := &protection_domain.ProtectionDomainListOK{Payload: []*models.ProtectionDomain{o}}

	// success cases (fetch tests all parameter variants)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	var mAPI *mockmgmtclient.MockAPI
	tcs := []string{"delete"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		appCtx.Account, appCtx.AccountID = "", ""
		mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
		lParams := protection_domain.NewProtectionDomainListParams()
		cOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
		var cmdArgs []string
		t.Log("case:", tc)
		switch tc {
		case "delete":
			cmdArgs = []string{"protection-domain", "delete", "-n", "My PD", "--confirm"}
			lParams.Name = swag.String("My PD")
			m := mockmgmtclient.NewProtectionDomainMatcher(t, lParams)
			cOps.EXPECT().ProtectionDomainList(m).Return(lRes, nil)
			dParams := protection_domain.NewProtectionDomainDeleteParams()
			dParams.ID = string(o.Meta.ID)
			cOps.EXPECT().ProtectionDomainDelete(dParams).Return(nil, nil)
		default:
			assert.Equal("", tc)
			continue
		}
		mAPI.EXPECT().ProtectionDomain().Return(cOps).MaxTimes(2)
		appCtx.API = mAPI
		te := &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initProtectionDomain()
		err := parseAndRun(cmdArgs)
		assert.Nil(err)
	}

	// error cases (supplemented by fetch tests)
	tcs = []string{"delete-error", "no-confirm", "init-context-error", "missing-id-or-name", "remaining-args"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		appCtx.Account, appCtx.AccountID = "", ""
		mAPI = nil
		var cmdArgs []string
		var expectedErr error
		errPat := ""
		t.Log("case:", tc)
		switch tc {
		case "delete-error":
			cmdArgs = []string{"protection-domain", "delete", "-n", "My PD", "--confirm"}
			lParams := protection_domain.NewProtectionDomainListParams()
			lParams.Name = swag.String("My PD")
			m := mockmgmtclient.NewProtectionDomainMatcher(t, lParams)
			cOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
			cOps.EXPECT().ProtectionDomainList(m).Return(lRes, nil)
			dParams := protection_domain.NewProtectionDomainDeleteParams()
			dParams.ID = string(o.Meta.ID)
			e := &protection_domain.ProtectionDomainDeleteDefault{
				Payload: &models.Error{Message: swag.String("delete-error")},
			}
			cOps.EXPECT().ProtectionDomainDelete(dParams).Return(nil, e)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			mAPI.EXPECT().ProtectionDomain().Return(cOps).MaxTimes(2)
			errPat = "delete-error"
		case "no-confirm":
			cmdArgs = []string{"protection-domain", "delete", "-n", "MyPD"}
			errPat = "specify --confirm"
		case "init-context-error":
			cmdArgs = []string{"protection-domain", "delete", "-A", "System", "id", "--confirm"}
			expectedErr = fmt.Errorf("ctx error")
			authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
			authOps.EXPECT().SetContextAccount("", "System").Return("", expectedErr)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			mAPI.EXPECT().Authentication().Return(authOps)
		case "missing-id-or-name":
			cmdArgs = []string{"protection-domain", "delete", "--confirm"}
			expectedErr = fmt.Errorf("either name or id should be specified")
		case "remaining-args":
			cmdArgs = []string{"protection-domain", "delete", "id", "confirm"}
			errPat = "unexpected arguments"
		default:
			assert.Equal("", tc)
			continue
		}
		appCtx.API = mAPI
		te := &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initProtectionDomain()
		err := parseAndRun(cmdArgs)
		assert.Error(err)
		if errPat != "" {
			assert.Regexp(errPat, err)
		}
		if expectedErr != nil {
			assert.Equal(expectedErr, err)
		}
	}
}

func TestProtectionDomainFetch(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	o := &models.ProtectionDomain{
		ProtectionDomainAllOf0: models.ProtectionDomainAllOf0{
			Meta: &models.ObjMeta{
				ID:           "id",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
		},
		ProtectionDomainCreateOnce: models.ProtectionDomainCreateOnce{
			AccountID:            "accountID",
			EncryptionAlgorithm:  "AES-256",
			EncryptionPassphrase: &models.ValueType{Kind: "SECRET", Value: "my password"},
		},
		ProtectionDomainMutable: models.ProtectionDomainMutable{
			Name:        "My PD",
			Description: "PD key",
			SystemTags:  models.ObjTags{"stag1", "stag2"},
			Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
		},
	}
	fRes := &protection_domain.ProtectionDomainFetchOK{Payload: o}
	lRes := &protection_domain.ProtectionDomainListOK{Payload: []*models.ProtectionDomain{o}}

	// success cases
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	var mAPI *mockmgmtclient.MockAPI
	tcs := []string{"list-name", "fetch-id-cols", "fetch-id-json", "fetch-id-no-flag-yaml"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		appCtx.Account, appCtx.AccountID = "", ""
		mAPI = nil
		fParams := protection_domain.NewProtectionDomainFetchParams()
		lParams := protection_domain.NewProtectionDomainListParams()
		cOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
		var cmdArgs []string
		tableDataColumns := protectionDomainDefaultHeaders
		hasJSON := false
		hasYaml := false
		t.Log("case:", tc)
		switch tc {
		case "list-name":
			cmdArgs = []string{"protection-domain", "get", "-n", "My PD"}
			lParams.Name = swag.String("My PD")
			m := mockmgmtclient.NewProtectionDomainMatcher(t, lParams)
			cOps.EXPECT().ProtectionDomainList(m).Return(lRes, nil)
		case "fetch-id-cols":
			cmdArgs = []string{"protection-domain", "get", "--id", "id", "-c", "account,name"}
			fParams.WithID("id")
			m := mockmgmtclient.NewProtectionDomainMatcher(t, fParams)
			cOps.EXPECT().ProtectionDomainFetch(m).Return(fRes, nil)
			tableDataColumns = []string{hAccount, hName}
		case "fetch-id-json":
			cmdArgs = []string{"protection-domain", "get", "--id", "id", "-o", "json"}
			fParams.WithID("id")
			m := mockmgmtclient.NewProtectionDomainMatcher(t, fParams)
			cOps.EXPECT().ProtectionDomainFetch(m).Return(fRes, nil)
			hasJSON = true
		case "fetch-id-no-flag-yaml":
			cmdArgs = []string{"protection-domain", "get", "id", "-o", "yaml"}
			fParams.WithID("id")
			m := mockmgmtclient.NewProtectionDomainMatcher(t, fParams)
			cOps.EXPECT().ProtectionDomainFetch(m).Return(fRes, nil)
			hasYaml = true
		default:
			assert.Equal("", tc)
			continue
		}
		if mAPI == nil {
			if !(hasJSON || hasYaml) {
				mAPI = cacheLoadHelper(t, mockCtrl, loadAccounts)
			} else {
				mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			}
		}
		mAPI.EXPECT().ProtectionDomain().Return(cOps)
		appCtx.API = mAPI
		te := &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initProtectionDomain()
		err := parseAndRun(cmdArgs)
		assert.Nil(err)
		if hasJSON {
			assert.NotNil(te.jsonData)
			assert.EqualValues([]*models.ProtectionDomain{o}, te.jsonData)
		} else if hasYaml {
			assert.NotNil(te.yamlData)
			assert.EqualValues([]*models.ProtectionDomain{o}, te.yamlData)
		} else {
			assert.Len(te.tableHeaders, len(tableDataColumns))
			assert.Len(te.tableData, 1)
		}
	}

	// error cases
	tcs = []string{"list-error", "list-not-found", "fetch-error", "init-context-error", "invalid-column", "missing-id", "remaining-args"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		appCtx.Account, appCtx.AccountID = "", ""
		mAPI = nil
		var cmdArgs []string
		var expectedErr error
		errPat := ""
		t.Log("case:", tc)
		switch tc {
		case "list-error":
			cmdArgs = []string{"protection-domain", "get", "-n", "My PD"}
			lParams := protection_domain.NewProtectionDomainListParams()
			lParams.Name = swag.String("My PD")
			m := mockmgmtclient.NewProtectionDomainMatcher(t, lParams)
			e := &protection_domain.ProtectionDomainListDefault{
				Payload: &models.Error{Message: swag.String("list-error")},
			}
			cOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
			cOps.EXPECT().ProtectionDomainList(m).Return(nil, e)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			mAPI.EXPECT().ProtectionDomain().Return(cOps)
			errPat = "list-error"
		case "list-not-found":
			cmdArgs = []string{"protection-domain", "get", "-n", "My PD"}
			lParams := protection_domain.NewProtectionDomainListParams()
			lParams.Name = swag.String("My PD")
			m := mockmgmtclient.NewProtectionDomainMatcher(t, lParams)
			cOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
			lRes := &protection_domain.ProtectionDomainListOK{Payload: []*models.ProtectionDomain{}}
			cOps.EXPECT().ProtectionDomainList(m).Return(lRes, nil)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			mAPI.EXPECT().ProtectionDomain().Return(cOps)
			expectedErr = fmt.Errorf("not found")
		case "fetch-error":
			cmdArgs = []string{"protection-domain", "get", "--id", "id", "-o", "json"}
			fParams := protection_domain.NewProtectionDomainFetchParams().WithID("id")
			m := mockmgmtclient.NewProtectionDomainMatcher(t, fParams)
			e := &protection_domain.ProtectionDomainFetchDefault{
				Payload: &models.Error{Message: swag.String("fetch-error")},
			}
			cOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
			cOps.EXPECT().ProtectionDomainFetch(m).Return(nil, e)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			mAPI.EXPECT().ProtectionDomain().Return(cOps)
			errPat = "fetch-error"
		case "init-context-error":
			cmdArgs = []string{"protection-domain", "get", "-A", "System", "id"}
			expectedErr = fmt.Errorf("ctx error")
			authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
			authOps.EXPECT().SetContextAccount("", "System").Return("", expectedErr)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			mAPI.EXPECT().Authentication().Return(authOps)
		case "invalid-column":
			cmdArgs = []string{"protection-domain", "get", "-c", "+foo"}
		case "missing-id":
			cmdArgs = []string{"protection-domain", "get"}
			errPat = "either name or id should be specified"
		case "remaining-args":
			cmdArgs = []string{"protection-domain", "get", "id", "extra-arg"}
			errPat = "unexpected arguments"
		default:
			assert.Equal("", tc)
			continue
		}
		appCtx.API = mAPI
		te := &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initProtectionDomain()
		err := parseAndRun(cmdArgs)
		assert.Error(err)
		if errPat != "" {
			assert.Regexp(errPat, err)
		}
		if expectedErr != nil {
			assert.Equal(expectedErr, err)
		}
	}
}

func TestProtectionDomainModify(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	o := &models.ProtectionDomain{
		ProtectionDomainAllOf0: models.ProtectionDomainAllOf0{
			Meta: &models.ObjMeta{
				ID:           "id",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
		},
		ProtectionDomainCreateOnce: models.ProtectionDomainCreateOnce{
			AccountID:            "accountID",
			EncryptionAlgorithm:  "AES-256",
			EncryptionPassphrase: &models.ValueType{Kind: "SECRET", Value: "my password"},
		},
		ProtectionDomainMutable: models.ProtectionDomainMutable{
			Name:        "My PD",
			Description: "PD key",
			SystemTags:  models.ObjTags{"stag1", "stag2"},
			Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
		},
	}
	fRes := &protection_domain.ProtectionDomainFetchOK{Payload: o}
	lRes := &protection_domain.ProtectionDomainListOK{Payload: []*models.ProtectionDomain{o}}
	uRes := &protection_domain.ProtectionDomainUpdateOK{Payload: o}

	// success cases
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	var mAPI *mockmgmtclient.MockAPI
	tcs := []string{"modify-all-list-name", "modify-set-fetch-id-cols", "modify-remove-fetch-id-json", "modify-trim-fetch-id-no-flag-yaml"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		appCtx.Account, appCtx.AccountID = "", ""
		mAPI = nil
		fParams := protection_domain.NewProtectionDomainFetchParams()
		lParams := protection_domain.NewProtectionDomainListParams()
		uParams := protection_domain.NewProtectionDomainUpdateParams().WithPayload(&models.ProtectionDomainMutable{}).WithID(string(o.Meta.ID))
		cOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
		var cmdArgs []string
		tableDataColumns := protectionDomainDefaultHeaders
		hasJSON := false
		hasYaml := false
		t.Log("case:", tc)
		switch tc {
		case "modify-all-list-name":
			cmdArgs = []string{"protection-domain", "modify", "-n", "My PD", "-N", "new name", "-d", "new desc", "-t", "tag1"}
			lParams.Name = swag.String("My PD")
			lM := mockmgmtclient.NewProtectionDomainMatcher(t, lParams)
			cOps.EXPECT().ProtectionDomainList(lM).Return(lRes, nil)
			uParams.Payload.Name = models.ObjName("new name")
			uParams.Payload.Description = models.ObjDescription("new desc")
			uParams.Payload.Tags = []string{"tag1"}
			uParams.Set = []string{"name", "description"}
			uParams.Append = []string{"tags"}
			uM := mockmgmtclient.NewProtectionDomainMatcher(t, uParams)
			cOps.EXPECT().ProtectionDomainUpdate(uM).Return(uRes, nil)
		case "modify-set-fetch-id-cols":
			cmdArgs = []string{"protection-domain", "modify", "--id", "id", "-c", "account,name", "-t", "tag1", "--tag-action", "SET"}
			fParams.WithID("id")
			fM := mockmgmtclient.NewProtectionDomainMatcher(t, fParams)
			cOps.EXPECT().ProtectionDomainFetch(fM).Return(fRes, nil)
			uParams.Payload.Tags = []string{"tag1"}
			uParams.Set = []string{"tags"}
			uM := mockmgmtclient.NewProtectionDomainMatcher(t, uParams)
			cOps.EXPECT().ProtectionDomainUpdate(uM).Return(uRes, nil)
			tableDataColumns = []string{hAccount, hName}
		case "modify-remove-fetch-id-json":
			cmdArgs = []string{"protection-domain", "modify", "--id", "id", "-o", "json", "-t", "tag1", "--tag-action", "REMOVE"}
			fParams.WithID("id")
			fM := mockmgmtclient.NewProtectionDomainMatcher(t, fParams)
			cOps.EXPECT().ProtectionDomainFetch(fM).Return(fRes, nil)
			uParams.Payload.Tags = []string{"tag1"}
			uParams.Remove = []string{"tags"}
			uM := mockmgmtclient.NewProtectionDomainMatcher(t, uParams)
			cOps.EXPECT().ProtectionDomainUpdate(uM).Return(uRes, nil)
			hasJSON = true
		case "modify-trim-fetch-id-no-flag-yaml":
			cmdArgs = []string{"protection-domain", "modify", "id", "-o", "yaml", "-d", " new desc "}
			fParams.WithID("id")
			fM := mockmgmtclient.NewProtectionDomainMatcher(t, fParams)
			cOps.EXPECT().ProtectionDomainFetch(fM).Return(fRes, nil)
			uParams.Payload.Description = models.ObjDescription("new desc")
			uParams.Set = []string{"description"}
			uM := mockmgmtclient.NewProtectionDomainMatcher(t, uParams)
			cOps.EXPECT().ProtectionDomainUpdate(uM).Return(uRes, nil)
			hasYaml = true
		default:
			assert.Equal("", tc)
			continue
		}
		if mAPI == nil {
			if !(hasJSON || hasYaml) {
				mAPI = cacheLoadHelper(t, mockCtrl, loadAccounts)
			} else {
				mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			}
		}
		mAPI.EXPECT().ProtectionDomain().Return(cOps).Times(2)
		appCtx.API = mAPI
		te := &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initProtectionDomain()
		err := parseAndRun(cmdArgs)
		assert.Nil(err)
		if hasJSON {
			assert.NotNil(te.jsonData)
			assert.EqualValues([]*models.ProtectionDomain{o}, te.jsonData)
		} else if hasYaml {
			assert.NotNil(te.yamlData)
			assert.EqualValues([]*models.ProtectionDomain{o}, te.yamlData)
		} else {
			assert.Len(te.tableHeaders, len(tableDataColumns))
			assert.Len(te.tableData, 1)
		}
	}

	// error cases
	tcs = []string{"update-error", "raw-fetch-error", "no-mods", "init-context-error", "invalid-column", "missing-id", "remaining-args"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		appCtx.Account, appCtx.AccountID = "", ""
		mAPI = nil
		var cmdArgs []string
		var expectedErr error
		errPat := ""
		t.Log("case:", tc)
		switch tc {
		case "update-error":
			cmdArgs = []string{"protection-domain", "modify", "id", "-d", " new desc "}
			fParams := protection_domain.NewProtectionDomainFetchParams()
			fParams.WithID("id")
			fM := mockmgmtclient.NewProtectionDomainMatcher(t, fParams)
			cOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
			cOps.EXPECT().ProtectionDomainFetch(fM).Return(fRes, nil)
			uParams := protection_domain.NewProtectionDomainUpdateParams().WithPayload(&models.ProtectionDomainMutable{}).WithID(string(o.Meta.ID))
			uParams.Payload.Description = models.ObjDescription("new desc")
			uParams.Set = []string{"description"}
			uM := mockmgmtclient.NewProtectionDomainMatcher(t, uParams)
			e := &protection_domain.ProtectionDomainUpdateDefault{
				Payload: &models.Error{Message: swag.String("update-error")},
			}
			cOps.EXPECT().ProtectionDomainUpdate(uM).Return(nil, e)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			mAPI.EXPECT().ProtectionDomain().Return(cOps).Times(2)
			errPat = "update-error"
		case "raw-fetch-error": // only one case - others covered in Fetch
			cmdArgs = []string{"protection-domain", "modify", "id", "-d", " new desc "}
			fParams := protection_domain.NewProtectionDomainFetchParams()
			fParams.WithID("id")
			fM := mockmgmtclient.NewProtectionDomainMatcher(t, fParams)
			e := &protection_domain.ProtectionDomainFetchDefault{
				Payload: &models.Error{Message: swag.String("fetch-error")},
			}
			cOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
			cOps.EXPECT().ProtectionDomainFetch(fM).Return(nil, e)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			mAPI.EXPECT().ProtectionDomain().Return(cOps)
			errPat = "fetch-error"
		case "no-mods":
			cmdArgs = []string{"protection-domain", "modify", "id"}
		case "init-context-error":
			cmdArgs = []string{"protection-domain", "modify", "-A", "System", "id"}
			expectedErr = fmt.Errorf("ctx error")
			authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
			authOps.EXPECT().SetContextAccount("", "System").Return("", expectedErr)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			mAPI.EXPECT().Authentication().Return(authOps)
		case "invalid-column":
			cmdArgs = []string{"protection-domain", "modify", "-c", "+foo"}
		case "missing-id":
			cmdArgs = []string{"protection-domain", "modify", "-d", "desc"}
			errPat = "either name or id should be specified"
		case "remaining-args":
			cmdArgs = []string{"protection-domain", "modify", "id", "foo"}
			errPat = "unexpected arguments"
		default:
			assert.Equal("", tc)
			continue
		}
		appCtx.API = mAPI
		te := &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initProtectionDomain()
		err := parseAndRun(cmdArgs)
		assert.Error(err)
		if errPat != "" {
			assert.Regexp(errPat, err)
		}
		if expectedErr != nil {
			assert.Equal(expectedErr, err)
		}
	}
}

func TestProtectionDomainMetadata(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	res := &protection_domain.ProtectionDomainMetadataOK{
		Payload: []*models.ProtectionDomainMetadata{
			&models.ProtectionDomainMetadata{EncryptionAlgorithm: "AES-256", Description: "AES", MinPassphraseLength: 10},
			&models.ProtectionDomainMetadata{EncryptionAlgorithm: "NONE", Description: "none", MinPassphraseLength: 0},
		},
	}

	// success cases
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	var mAPI *mockmgmtclient.MockAPI
	tcs := []string{"table", "json", "yaml"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		appCtx.Account, appCtx.AccountID = "", ""
		mAPI = nil
		params := protection_domain.NewProtectionDomainMetadataParams()
		var cmdArgs []string
		tableDataColumns := protectionDomainMetadataCols
		hasJSON := false
		hasYaml := false
		t.Log("case:", tc)
		switch tc {
		case "table":
			cmdArgs = []string{"protection-domain", "metadata"}
		case "json":
			cmdArgs = []string{"protection-domain", "md", "-o", "json"}
			hasJSON = true
		case "yaml":
			cmdArgs = []string{"pd", "metadata", "-o", "yaml"}
			hasYaml = true
		default:
			assert.Equal("", tc)
			continue
		}
		mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
		cOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
		mAPI.EXPECT().ProtectionDomain().Return(cOps)
		cOps.EXPECT().ProtectionDomainMetadata(params).Return(res, nil)
		appCtx.API = mAPI
		te := &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initProtectionDomain()
		err := parseAndRun(cmdArgs)
		assert.Nil(err)
		if hasJSON {
			assert.NotNil(te.jsonData)
			assert.EqualValues(res.Payload, te.jsonData)
		} else if hasYaml {
			assert.NotNil(te.yamlData)
			assert.EqualValues(res.Payload, te.yamlData)
		} else {
			assert.Len(te.tableHeaders, len(tableDataColumns))
			assert.Len(te.tableData, len(res.Payload))
			for i, md := range res.Payload {
				row := te.tableData[i]
				assert.Equalf(md.EncryptionAlgorithm, row[0], "[%d]", i)
				assert.Equalf(md.Description, row[1], "[%d]", i)
				assert.Equalf(fmt.Sprintf("%d", md.MinPassphraseLength), row[2], "[%d]", i)
			}
		}
	}

	// error cases
	tcs = []string{"metadata-error", "remaining-args"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		appCtx.Account, appCtx.AccountID = "", ""
		mAPI = nil
		var cmdArgs []string
		var expectedErr error
		errPat := ""
		t.Log("case:", tc)
		switch tc {
		case "metadata-error":
			cmdArgs = []string{"protection-domain", "metadata"}
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			cOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
			mAPI.EXPECT().ProtectionDomain().Return(cOps)
			e := &protection_domain.ProtectionDomainMetadataDefault{
				Payload: &models.Error{Message: swag.String("metadata-error")},
			}
			params := protection_domain.NewProtectionDomainMetadataParams()
			cOps.EXPECT().ProtectionDomainMetadata(params).Return(nil, e)
			appCtx.API = mAPI
			errPat = "metadata-error"
		case "remaining-args":
			cmdArgs = []string{"protection-domain", "metadata", "foo"}
			errPat = "unexpected arguments"
		default:
			assert.Equal("", tc)
			continue
		}
		appCtx.API = mAPI
		te := &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initProtectionDomain()
		err := parseAndRun(cmdArgs)
		assert.Error(err)
		if errPat != "" {
			assert.Regexp(errPat, err)
		}
		if expectedErr != nil {
			assert.Equal(expectedErr, err)
		}
	}
}

func TestProtectionDomainClear(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	pdObj := resProtectionDomains.Payload[0]
	aObj := resAccounts.Payload[0]
	domObj := resCspDomains.Payload[0]

	// success cases (fetch tests all parameter variants)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	var mAPI *mockmgmtclient.MockAPI
	tcs := []string{"clear-default", "clear-default-json", "clear-csp-version-yaml"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		appCtx.Account, appCtx.AccountID = "", ""
		mAPI = nil
		aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
		authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
		res := &account.AccountProtectionDomainClearOK{}
		testutils.Clone(aObj, &res.Payload)
		res.Payload.ProtectionDomains = map[string]models.ObjIDMutable{
			common.ProtectionStoreDefaultKey: models.ObjIDMutable(pdObj.Meta.ID),
			string(domObj.Meta.ID):           models.ObjIDMutable(pdObj.Meta.ID),
		}
		expTableData := [][]string{
			[]string{"", string(pdObj.Name)},
			[]string{string(domObj.Name), string(pdObj.Name)},
		}
		var cmdArgs []string
		tableDataColumns := accountPDHeaders
		hasJSON := false
		hasYaml := false
		t.Log("case:", tc)
		switch tc {
		case "clear-default":
			cmdArgs = []string{"protection-domain", "clear", "-A", string(aObj.Name)}
			mAPI = cacheLoadHelper(t, mockCtrl, loadProtectionDomains, loadDomains)
			authOps.EXPECT().SetContextAccount("", string(aObj.Name)).Return(string(aObj.Meta.ID), nil)
			cParams := account.NewAccountProtectionDomainClearParams()
			cParams.ID = string(aObj.Meta.ID)
			aOps.EXPECT().AccountProtectionDomainClear(cParams).Return(res, nil)
		case "clear-default-json":
			cmdArgs = []string{"protection-domain", "clear", "-A", string(aObj.Name), "-o", "json"}
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			authOps.EXPECT().SetContextAccount("", string(aObj.Name)).Return(string(aObj.Meta.ID), nil)
			cParams := account.NewAccountProtectionDomainClearParams()
			cParams.ID = string(aObj.Meta.ID)
			aOps.EXPECT().AccountProtectionDomainClear(cParams).Return(res, nil)
			hasJSON = true
		case "clear-csp-version-yaml":
			cmdArgs = []string{"protection-domain", "clear", "-A", string(aObj.Name), "-D", string(domObj.Name), "-V", "3", "-o", "yaml"}
			mAPI = cacheLoadHelper(t, mockCtrl, loadDomains)
			authOps.EXPECT().SetContextAccount("", string(aObj.Name)).Return(string(aObj.Meta.ID), nil)
			cParams := account.NewAccountProtectionDomainClearParams()
			cParams.ID = string(aObj.Meta.ID)
			cParams.CspDomainID = swag.String(string(domObj.Meta.ID))
			cParams.Version = swag.Int32(3)
			aOps.EXPECT().AccountProtectionDomainClear(cParams).Return(res, nil)
			hasYaml = true
		default:
			assert.Equal("", tc)
			continue
		}
		mAPI.EXPECT().Account().Return(aOps).AnyTimes()
		mAPI.EXPECT().Authentication().Return(authOps)
		appCtx.API = mAPI
		te := &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initProtectionDomain()
		err := parseAndRun(cmdArgs)
		assert.Nil(err)
		if hasJSON {
			assert.NotNil(te.jsonData)
			assert.EqualValues(res.Payload, te.jsonData)
		} else if hasYaml {
			assert.NotNil(te.yamlData)
			assert.EqualValues(res.Payload, te.yamlData)
		} else {
			assert.Len(te.tableHeaders, len(tableDataColumns))
			assert.Equal(expTableData, te.tableData)
		}
	}

	// error cases (supplemented by fetch tests)
	tcs = []string{"clear-fails", "dom-name-invalid", "account-not-set", "init-context-error", "remaining-args"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		appCtx.Account, appCtx.AccountID = "", ""
		mAPI = nil
		var cmdArgs []string
		var expectedErr error
		errPat := ""
		t.Log("case:", tc)
		switch tc {
		case "clear-fails":
			cmdArgs = []string{"protection-domain", "clear", "-A", string(aObj.Name)}
			aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
			authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
			authOps.EXPECT().SetContextAccount("", string(aObj.Name)).Return(string(aObj.Meta.ID), nil)
			cParams := account.NewAccountProtectionDomainClearParams()
			cParams.ID = string(aObj.Meta.ID)
			e := &account.AccountProtectionDomainClearDefault{
				Payload: &models.Error{Message: swag.String("clear-error")},
			}
			errPat = "clear-error"
			aOps.EXPECT().AccountProtectionDomainClear(cParams).Return(nil, e)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			mAPI.EXPECT().Account().Return(aOps).AnyTimes()
			mAPI.EXPECT().Authentication().Return(authOps)
		case "dom-name-invalid":
			cmdArgs = []string{"protection-domain", "clear", "-A", string(aObj.Name), "-D", string(domObj.Name) + "foo"}
			aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
			authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
			authOps.EXPECT().SetContextAccount("", string(aObj.Name)).Return(string(aObj.Meta.ID), nil)
			mAPI = cacheLoadHelper(t, mockCtrl, loadDomains)
			mAPI.EXPECT().Account().Return(aOps).AnyTimes()
			mAPI.EXPECT().Authentication().Return(authOps)
			errPat = "domain.*not found"
		case "account-not-set":
			cmdArgs = []string{"protection-domain", "clear"}
			expectedErr = fmt.Errorf("The --account flag was not specified")
		case "init-context-error":
			cmdArgs = []string{"protection-domain", "clear", "-A", "System"}
			authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
			expectedErr = fmt.Errorf("ctx error")
			authOps.EXPECT().SetContextAccount("", "System").Return("", expectedErr)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			mAPI.EXPECT().Authentication().Return(authOps)
		case "remaining-args":
			cmdArgs = []string{"protection-domain", "clear", "extra"}
			errPat = "unexpected arguments"
		default:
			assert.Equal("", tc)
			continue
		}
		appCtx.API = mAPI
		te := &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initProtectionDomain()
		err := parseAndRun(cmdArgs)
		assert.Error(err)
		if errPat != "" {
			assert.Regexp(errPat, err)
		}
		if expectedErr != nil {
			assert.Equal(expectedErr, err)
		}
	}
}

func TestProtectionDomainSet(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	pdObj := resProtectionDomains.Payload[0]
	aObj := resAccounts.Payload[0]
	domObj := resCspDomains.Payload[0]

	// success cases (fetch tests all parameter variants)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	var mAPI *mockmgmtclient.MockAPI
	tcs := []string{"set-default", "set-default-by-id-json", "set-default-by-id-version-yaml", "set-csp-version"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		appCtx.Account, appCtx.AccountID = "", ""
		mAPI = nil
		aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
		authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
		res := &account.AccountProtectionDomainSetOK{}
		testutils.Clone(aObj, &res.Payload)
		res.Payload.ProtectionDomains = map[string]models.ObjIDMutable{
			common.ProtectionStoreDefaultKey: models.ObjIDMutable(pdObj.Meta.ID),
			string(domObj.Meta.ID):           models.ObjIDMutable(pdObj.Meta.ID),
		}
		expTableData := [][]string{
			[]string{"", string(pdObj.Name)},
			[]string{string(domObj.Name), string(pdObj.Name)},
		}
		var cmdArgs []string
		tableDataColumns := accountPDHeaders
		hasJSON := false
		hasYaml := false
		t.Log("case:", tc)
		switch tc {
		case "set-default":
			cmdArgs = []string{"protection-domain", "set", "-A", string(aObj.Name), "-n", string(pdObj.Name)}
			mAPI = cacheLoadHelper(t, mockCtrl, loadProtectionDomains, loadDomains)
			authOps.EXPECT().SetContextAccount("", string(aObj.Name)).Return(string(aObj.Meta.ID), nil)
			sParams := account.NewAccountProtectionDomainSetParams()
			sParams.ID = string(aObj.Meta.ID)
			sParams.ProtectionDomainID = string(pdObj.Meta.ID)
			aOps.EXPECT().AccountProtectionDomainSet(sParams).Return(res, nil)
		case "set-default-by-id-json":
			cmdArgs = []string{"protection-domain", "set", "-A", string(aObj.Name), "-o", "json", string(pdObj.Meta.ID)}
			mAPI = cacheLoadHelper(t, mockCtrl, loadProtectionDomains)
			authOps.EXPECT().SetContextAccount("", string(aObj.Name)).Return(string(aObj.Meta.ID), nil)
			sParams := account.NewAccountProtectionDomainSetParams()
			sParams.ID = string(aObj.Meta.ID)
			sParams.ProtectionDomainID = string(pdObj.Meta.ID)
			aOps.EXPECT().AccountProtectionDomainSet(sParams).Return(res, nil)
			hasJSON = true
		case "set-default-by-id-version-yaml":
			cmdArgs = []string{"protection-domain", "set", "-A", string(aObj.Name), "-o", "yaml", "-V", "3", "--id", string(pdObj.Meta.ID)}
			mAPI = cacheLoadHelper(t, mockCtrl, loadProtectionDomains)
			authOps.EXPECT().SetContextAccount("", string(aObj.Name)).Return(string(aObj.Meta.ID), nil)
			sParams := account.NewAccountProtectionDomainSetParams()
			sParams.ID = string(aObj.Meta.ID)
			sParams.ProtectionDomainID = string(pdObj.Meta.ID)
			sParams.Version = swag.Int32(3)
			aOps.EXPECT().AccountProtectionDomainSet(sParams).Return(res, nil)
			hasYaml = true
		case "set-csp-version":
			cmdArgs = []string{"protection-domain", "set", "-A", string(aObj.Name), "-n", string(pdObj.Name), "-D", string(domObj.Name)}
			mAPI = cacheLoadHelper(t, mockCtrl, loadProtectionDomains, loadDomains)
			authOps.EXPECT().SetContextAccount("", string(aObj.Name)).Return(string(aObj.Meta.ID), nil)
			sParams := account.NewAccountProtectionDomainSetParams()
			sParams.ID = string(aObj.Meta.ID)
			sParams.ProtectionDomainID = string(pdObj.Meta.ID)
			sParams.CspDomainID = swag.String(string(domObj.Meta.ID))
			aOps.EXPECT().AccountProtectionDomainSet(sParams).Return(res, nil)
		default:
			assert.Equal("", tc)
			continue
		}
		mAPI.EXPECT().Account().Return(aOps).AnyTimes()
		mAPI.EXPECT().Authentication().Return(authOps)
		appCtx.API = mAPI
		te := &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initProtectionDomain()
		err := parseAndRun(cmdArgs)
		assert.Nil(err)
		if hasJSON {
			assert.NotNil(te.jsonData)
			assert.EqualValues(res.Payload, te.jsonData)
		} else if hasYaml {
			assert.NotNil(te.yamlData)
			assert.EqualValues(res.Payload, te.yamlData)
		} else {
			assert.Len(te.tableHeaders, len(tableDataColumns))
			assert.Equal(expTableData, te.tableData)
		}
	}

	// error cases (supplemented by fetch tests)
	tcs = []string{"set-fails", "dom-name-invalid", "missing-id-or-name", "pd-list-fails",
		"account-not-set", "init-context-error", "remaining-args"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		appCtx.Account, appCtx.AccountID = "", ""
		mAPI = nil
		var cmdArgs []string
		var expectedErr error
		errPat := ""
		t.Log("case:", tc)
		switch tc {
		case "set-fails":
			cmdArgs = []string{"protection-domain", "set", "-A", string(aObj.Name), "-n", string(pdObj.Name)}
			aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
			authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
			authOps.EXPECT().SetContextAccount("", string(aObj.Name)).Return(string(aObj.Meta.ID), nil)
			sParams := account.NewAccountProtectionDomainSetParams()
			sParams.ID = string(aObj.Meta.ID)
			sParams.ProtectionDomainID = string(pdObj.Meta.ID)
			e := &account.AccountProtectionDomainSetDefault{
				Payload: &models.Error{Message: swag.String("set-error")},
			}
			errPat = "set-error"
			aOps.EXPECT().AccountProtectionDomainSet(sParams).Return(nil, e)
			mAPI = cacheLoadHelper(t, mockCtrl, loadProtectionDomains)
			mAPI.EXPECT().Account().Return(aOps).AnyTimes()
			mAPI.EXPECT().Authentication().Return(authOps)
		case "dom-name-invalid":
			cmdArgs = []string{"protection-domain", "set", "-A", string(aObj.Name), "-n", string(pdObj.Name), "-D", string(domObj.Name) + "foo"}
			aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
			authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
			authOps.EXPECT().SetContextAccount("", string(aObj.Name)).Return(string(aObj.Meta.ID), nil)
			mAPI = cacheLoadHelper(t, mockCtrl, loadProtectionDomains, loadDomains)
			mAPI.EXPECT().Account().Return(aOps).AnyTimes()
			mAPI.EXPECT().Authentication().Return(authOps)
			errPat = "domain.*not found"
		case "missing-id-or-name":
			cmdArgs = []string{"protection-domain", "set", "-A", string(aObj.Name)}
			aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
			authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
			authOps.EXPECT().SetContextAccount("", string(aObj.Name)).Return(string(aObj.Meta.ID), nil)
			mAPI = cacheLoadHelper(t, mockCtrl, loadProtectionDomains)
			mAPI.EXPECT().Account().Return(aOps).AnyTimes()
			mAPI.EXPECT().Authentication().Return(authOps)
			expectedErr = fmt.Errorf("either name or id should be specified")
		case "pd-list-fails":
			cmdArgs = []string{"protection-domain", "set", "-A", string(aObj.Name), "-n", string(pdObj.Name)}
			aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
			authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
			authOps.EXPECT().SetContextAccount("", string(aObj.Name)).Return(string(aObj.Meta.ID), nil)
			pdOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
			pdOps.EXPECT().ProtectionDomainList(gomock.Any()).Return(nil, fmt.Errorf("pd-list")).MinTimes(1)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			mAPI.EXPECT().Account().Return(aOps).AnyTimes()
			mAPI.EXPECT().Authentication().Return(authOps)
			mAPI.EXPECT().ProtectionDomain().Return(pdOps).MinTimes(1)
			errPat = "pd-list"
		case "account-not-set":
			cmdArgs = []string{"protection-domain", "set", "-n", string(pdObj.Name)}
			expectedErr = fmt.Errorf("The --account flag was not specified")
		case "init-context-error":
			cmdArgs = []string{"protection-domain", "set", "-A", "System", "-n", string(pdObj.Name)}
			authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
			expectedErr = fmt.Errorf("ctx error")
			authOps.EXPECT().SetContextAccount("", "System").Return("", expectedErr)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			mAPI.EXPECT().Authentication().Return(authOps)
		case "remaining-args":
			cmdArgs = []string{"protection-domain", "set", "id", "extra"}
			errPat = "unexpected arguments"
		default:
			assert.Equal("", tc)
			continue
		}
		appCtx.API = mAPI
		te := &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initProtectionDomain()
		err := parseAndRun(cmdArgs)
		assert.Error(err)
		if errPat != "" {
			assert.Regexp(errPat, err)
		}
		if expectedErr != nil {
			assert.Equal(expectedErr, err)
		}
	}
}

func TestProtectionDomainShow(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	pdObj := resProtectionDomains.Payload[0]
	aObj := resAccounts.Payload[0]
	domObj := resCspDomains.Payload[0]

	// success cases
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	var mAPI *mockmgmtclient.MockAPI
	tcs := []string{"show"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		appCtx.Account, appCtx.AccountID = "", ""
		mAPI = nil
		aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
		authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
		res := &account.AccountListOK{
			Payload: []*models.Account{
				&models.Account{},
			},
		}
		testutils.Clone(aObj, &res.Payload[0])
		res.Payload[0].ProtectionDomains = map[string]models.ObjIDMutable{
			common.ProtectionStoreDefaultKey: models.ObjIDMutable(pdObj.Meta.ID),
			string(domObj.Meta.ID):           models.ObjIDMutable(pdObj.Meta.ID),
		}
		expTableData := [][]string{
			[]string{"", string(pdObj.Name)},
			[]string{string(domObj.Name), string(pdObj.Name)},
		}
		aOps.EXPECT().AccountList(gomock.Any()).Return(res, nil).MinTimes(1)
		var cmdArgs []string
		tableDataColumns := accountPDHeaders
		hasJSON := false
		hasYaml := false
		t.Log("case:", tc)
		switch tc {
		case "show":
			cmdArgs = []string{"protection-domain", "show", "-A", string(aObj.Name)}
			mAPI = cacheLoadHelper(t, mockCtrl, loadProtectionDomains, loadDomains)
			authOps.EXPECT().SetContextAccount("", string(aObj.Name)).Return(string(aObj.Meta.ID), nil)
		default:
			assert.Equal("", tc)
			continue
		}
		mAPI.EXPECT().Account().Return(aOps).AnyTimes()
		mAPI.EXPECT().Authentication().Return(authOps)
		appCtx.API = mAPI
		te := &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initProtectionDomain()
		err := parseAndRun(cmdArgs)
		assert.Nil(err)
		if hasJSON {
			assert.NotNil(te.jsonData)
			assert.EqualValues(res.Payload, te.jsonData)
		} else if hasYaml {
			assert.NotNil(te.yamlData)
			assert.EqualValues(res.Payload, te.yamlData)
		} else {
			assert.Len(te.tableHeaders, len(tableDataColumns))
			assert.Equal(expTableData, te.tableData)
		}
	}

	// error cases
	tcs = []string{"account-not-set", "init-context-error", "pd-list-fails", "dom-list-fails", "account-list-fails"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		appCtx.Account, appCtx.AccountID = "", ""
		mAPI = nil
		var cmdArgs []string
		var expectedErr error
		errPat := ""
		t.Log("case:", tc)
		switch tc {
		case "account-list-fails":
			cmdArgs = []string{"protection-domain", "show", "-A", string(aObj.Name)}
			aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
			aOps.EXPECT().AccountList(gomock.Any()).Return(nil, fmt.Errorf("account-list")).MinTimes(1)
			authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
			authOps.EXPECT().SetContextAccount("", string(aObj.Name)).Return(string(aObj.Meta.ID), nil)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			mAPI.EXPECT().Account().Return(aOps).AnyTimes()
			mAPI.EXPECT().Authentication().Return(authOps)
			errPat = "account-list"
		case "dom-list-fails":
			cmdArgs = []string{"protection-domain", "show", "-A", string(aObj.Name)}
			aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
			aOps.EXPECT().AccountList(gomock.Any()).Return(resAccounts, nil).MinTimes(1)
			authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
			authOps.EXPECT().SetContextAccount("", string(aObj.Name)).Return(string(aObj.Meta.ID), nil)
			dOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
			dOps.EXPECT().CspDomainList(gomock.Any()).Return(nil, fmt.Errorf("dom-list")).MinTimes(1)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			mAPI.EXPECT().Account().Return(aOps).AnyTimes()
			mAPI.EXPECT().Authentication().Return(authOps)
			mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
			errPat = "dom-list"
		case "pd-list-fails":
			cmdArgs = []string{"protection-domain", "show", "-A", string(aObj.Name)}
			aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
			aOps.EXPECT().AccountList(gomock.Any()).Return(resAccounts, nil).MinTimes(1)
			authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
			authOps.EXPECT().SetContextAccount("", string(aObj.Name)).Return(string(aObj.Meta.ID), nil)
			dOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
			dOps.EXPECT().CspDomainList(gomock.Any()).Return(resCspDomains, nil).MinTimes(1)
			pdOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
			pdOps.EXPECT().ProtectionDomainList(gomock.Any()).Return(nil, fmt.Errorf("pd-list")).MinTimes(1)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			mAPI.EXPECT().Account().Return(aOps).AnyTimes()
			mAPI.EXPECT().Authentication().Return(authOps)
			mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
			mAPI.EXPECT().ProtectionDomain().Return(pdOps).MinTimes(1)
			errPat = "pd-list"
		case "account-not-set":
			cmdArgs = []string{"protection-domain", "show"}
			expectedErr = fmt.Errorf("The --account flag was not specified")
		case "init-context-error":
			cmdArgs = []string{"protection-domain", "show", "-A", "System"}
			authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
			expectedErr = fmt.Errorf("ctx error")
			authOps.EXPECT().SetContextAccount("", "System").Return("", expectedErr)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			mAPI.EXPECT().Authentication().Return(authOps)
		default:
			assert.Equal("", tc)
			continue
		}
		appCtx.API = mAPI
		te := &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initProtectionDomain()
		err := parseAndRun(cmdArgs)
		assert.Error(err)
		if errPat != "" {
			assert.Regexp(errPat, err)
		}
		if expectedErr != nil {
			assert.Equal(expectedErr, err)
		}
	}
}
