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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/role"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestRoleMakeRecord(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	ac := &roleCmd{}
	o := &models.Role{
		RoleAllOf0: models.RoleAllOf0{
			Meta: &models.ObjMeta{
				ID:           "id",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
		},
		RoleMutable: models.RoleMutable{
			Name: centrald.SystemAdminRole,
		},
	}
	rec := ac.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(roleHeaders))
	assert.Equal("id", rec[hID])
	assert.Equal("1", rec[hVersion])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeCreated])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeModified])
	assert.Equal(centrald.SystemAdminRole, rec[hName])
}

func TestRoleList(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	m := mockmgmtclient.NewRoleMatcher(t, role.NewRoleListParams().WithName(swag.String("")))
	res := &role.RoleListOK{
		Payload: []*models.Role{
			&models.Role{
				RoleAllOf0: models.RoleAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				RoleMutable: models.RoleMutable{
					Name: "role1",
				},
			},
			&models.Role{
				RoleAllOf0: models.RoleAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id2",
						Version:      2,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				RoleMutable: models.RoleMutable{
					Name: "role2",
				},
			},
		},
	}

	// list, all defaults
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockRoleClient(mockCtrl)
	mAPI.EXPECT().Role().Return(cOps)
	cOps.EXPECT().RoleList(m).Return(res, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initRole()
	err := parseAndRun([]string{"role", "list"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(roleDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list default, json
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockRoleClient(mockCtrl)
	mAPI.EXPECT().Role().Return(cOps)
	cOps.EXPECT().RoleList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initRole()
	err = parseAndRun([]string{"role", "list", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(res.Payload, te.jsonData)

	// list default, yaml
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockRoleClient(mockCtrl)
	mAPI.EXPECT().Role().Return(cOps)
	cOps.EXPECT().RoleList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initRole()
	err = parseAndRun([]string{"role", "list", "-o", "yaml"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(res.Payload, te.yamlData)

	// list with columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockRoleClient(mockCtrl)
	mAPI.EXPECT().Role().Return(cOps)
	cOps.EXPECT().RoleList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initRole()
	err = parseAndRun([]string{"role", "list", "--columns", "Name,ID"})
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
	initRole()
	err = parseAndRun([]string{"role", "list", "-c", "Name,ID,Foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// list, API failure model error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockRoleClient(mockCtrl)
	mAPI.EXPECT().Role().Return(cOps)
	apiErr := &role.RoleListDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	cOps.EXPECT().RoleList(m).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initRole()
	err = parseAndRun([]string{"role", "list"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// list, API failure arbitrary error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockRoleClient(mockCtrl)
	mAPI.EXPECT().Role().Return(cOps)
	otherErr := fmt.Errorf("OTHER ERROR")
	cOps.EXPECT().RoleList(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initRole()
	err = parseAndRun([]string{"role", "list"})
	assert.NotNil(err)
	assert.Equal(otherErr, err)

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initRole()
	err = parseAndRun([]string{"role", "list", "o", "json"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}
