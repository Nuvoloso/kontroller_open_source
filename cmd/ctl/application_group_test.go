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
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestApplicationGroupMakeRecord(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	cmd := &applicationGroupCmd{}
	cmd.accounts = map[string]ctxIDName{
		"accountID": {"", "System"},
	}
	o := &models.ApplicationGroup{
		ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
			Meta: &models.ObjMeta{
				ID:           "id",
				Version:      2,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
		},
		ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
			AccountID: "accountID",
		},
		ApplicationGroupMutable: models.ApplicationGroupMutable{
			Description: "desc",
			Name:        "ag1",
			Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
		},
	}
	rec := cmd.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(applicationGroupHeaders))
	assert.Equal("id", rec[hID])
	assert.Equal("2", rec[hVersion])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeCreated])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeModified])
	assert.Equal("System", rec[hAccount])
	assert.Equal("ag1", rec[hName])
	assert.Equal("desc", rec[hDescription])
	assert.Equal("tag1, tag2, tag3", rec[hTags])

	// repeat without the map
	cmd.accounts = map[string]ctxIDName{}
	rec = cmd.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(applicationGroupHeaders))
	assert.Equal("accountID", rec[hAccount])
}

func TestApplicationGroupList(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	res := &application_group.ApplicationGroupListOK{
		Payload: []*models.ApplicationGroup{
			&models.ApplicationGroup{
				ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
					AccountID: "accountID",
				},
				ApplicationGroupMutable: models.ApplicationGroupMutable{
					Description: "desc",
					Name:        "ag1",
					Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
				},
			},
			&models.ApplicationGroup{
				ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id2",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
					AccountID: "authAccountID",
				},
				ApplicationGroupMutable: models.ApplicationGroupMutable{
					Description: "desc2",
					Name:        "ag2",
					Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
				},
			},
		},
	}

	// list, all defaults
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := cacheLoadHelper(t, mockCtrl, loadAccounts)
	params := application_group.NewApplicationGroupListParams()
	m := mockmgmtclient.NewApplicationGroupMatcher(t, params)
	cOps := mockmgmtclient.NewMockApplicationGroupClient(mockCtrl)
	mAPI.EXPECT().ApplicationGroup().Return(cOps)
	cOps.EXPECT().ApplicationGroupList(m).Return(res, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initApplicationGroup()
	err := parseAndRun([]string{"ags", "list"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(applicationGroupDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list, json, most options
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadContext)
	params = application_group.NewApplicationGroupListParams()
	params.AccountID = swag.String("accountID")
	params.Name = swag.String("ag")
	params.Tags = []string{"tag2"}
	m = mockmgmtclient.NewApplicationGroupMatcher(t, params)
	cOps = mockmgmtclient.NewMockApplicationGroupClient(mockCtrl)
	mAPI.EXPECT().ApplicationGroup().Return(cOps)
	cOps.EXPECT().ApplicationGroupList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initApplicationGroup()
	err = parseAndRun([]string{"application-group", "list", "-A", "System", "--owned-only", "-n", "ag", "-t", "tag2", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(res.Payload, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// list, yaml
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadContext)
	params = application_group.NewApplicationGroupListParams()
	m = mockmgmtclient.NewApplicationGroupMatcher(t, params)
	cOps = mockmgmtclient.NewMockApplicationGroupClient(mockCtrl)
	mAPI.EXPECT().ApplicationGroup().Return(cOps)
	cOps.EXPECT().ApplicationGroupList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initApplicationGroup()
	err = parseAndRun([]string{"ags", "list", "-A", "System", "-o", "yaml"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(res.Payload, te.yamlData)
	appCtx.Account, appCtx.AccountID = "", ""

	// list with columns, account list error ignored
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	apm := mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	aOps.EXPECT().AccountList(apm).Return(nil, fmt.Errorf("db error"))
	params = application_group.NewApplicationGroupListParams()
	m = mockmgmtclient.NewApplicationGroupMatcher(t, params)
	cOps = mockmgmtclient.NewMockApplicationGroupClient(mockCtrl)
	mAPI.EXPECT().ApplicationGroup().Return(cOps)
	cOps.EXPECT().ApplicationGroupList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initApplicationGroup()
	err = parseAndRun([]string{"application-group", "list", "--columns", "ID,Version,Name"})
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
	initApplicationGroup()
	err = parseAndRun([]string{"ags", "list", "--columns", "ID,Foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// list, API failure model error
	apiErr := &application_group.ApplicationGroupListDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	params = application_group.NewApplicationGroupListParams()
	m = mockmgmtclient.NewApplicationGroupMatcher(t, params)
	cOps = mockmgmtclient.NewMockApplicationGroupClient(mockCtrl)
	mAPI.EXPECT().ApplicationGroup().Return(cOps)
	cOps.EXPECT().ApplicationGroupList(m).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initApplicationGroup()
	err = parseAndRun([]string{"application-group", "list"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// list, API failure arbitrary error
	otherErr := fmt.Errorf("OTHER ERROR")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	params = application_group.NewApplicationGroupListParams()
	m = mockmgmtclient.NewApplicationGroupMatcher(t, params)
	cOps = mockmgmtclient.NewMockApplicationGroupClient(mockCtrl)
	mAPI.EXPECT().ApplicationGroup().Return(cOps)
	cOps.EXPECT().ApplicationGroupList(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initApplicationGroup()
	err = parseAndRun([]string{"application-group", "list"})
	assert.NotNil(err)
	assert.Equal(otherErr.Error(), err.Error())

	// account not found
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	args := []string{"ag", "list", "-A", "foo"}
	t.Log(args)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "foo").Return("", fmt.Errorf("account not found"))
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initApplicationGroup()
	err = parseAndRun(args)
	assert.NotNil(err)
	assert.Regexp("account.*not found", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initApplicationGroup()
	err = parseAndRun([]string{"application-group", "list", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}
