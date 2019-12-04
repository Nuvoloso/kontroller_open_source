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
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/user"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestUserMakeRecord(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	ac := &userCmd{}
	o := &models.User{
		UserAllOf0: models.UserAllOf0{
			Meta: &models.ObjMeta{
				ID:           "id",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
		},
		UserMutable: models.UserMutable{
			AuthIdentifier: centrald.SystemUser,
			Profile: map[string]models.ValueType{
				"userName": models.ValueType{Kind: "STRING", Value: "fred"},
				"theme":    models.ValueType{Kind: "STRING", Value: "dark"},
			},
		},
	}
	rec := ac.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(userHeaders))
	assert.Equal("id", rec[hID])
	assert.Equal("1", rec[hVersion])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeCreated])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeModified])
	assert.Equal(centrald.SystemUser, rec[hAuthIdentifier])
	assert.Equal("false", rec[hDisabled])
	// sorted
	assert.Equal("theme:dark\nuserName:fred", rec[hProfile])
}

func TestUserCreate(t *testing.T) {
	assert := assert.New(t)
	savedPasswordHook := passwordHook
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
		passwordHook = savedPasswordHook
	}()
	interactiveReader = nil
	var hookCnt int
	var hookRet [][]byte
	var hookErr []error
	passwordHook = func(int) ([]byte, error) {
		i := hookCnt
		hookCnt++
		return hookRet[i], hookErr[i]
	}

	params := user.NewUserCreateParams()
	params.Payload = &models.User{
		UserMutable: models.UserMutable{
			AuthIdentifier: "user@host",
			Password:       "user",
			Profile: map[string]models.ValueType{
				"userName": {Kind: "STRING", Value: "full name"},
				"guiTheme": {Kind: "STRING", Value: "dark"},
			},
		},
	}
	res := &user.UserCreateCreated{
		Payload: params.Payload,
	}
	m := mockmgmtclient.NewUserMatcher(t, params)

	t.Log("create, password on CLI")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(cOps)
	cOps.EXPECT().UserCreate(m).Return(res, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	err := parseAndRun([]string{"user", "create", "-I", string(params.Payload.AuthIdentifier),
		"--password=user", "-P", "userName:full name", "-P", "guiTheme:dark",
		"-o", "json"})
	assert.NoError(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.User{params.Payload}, te.jsonData)

	t.Log("create, invalid columns")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	err = parseAndRun([]string{"user", "create", "-I", string(params.Payload.AuthIdentifier),
		"-P", "userName:full name", "-P", "guiTheme:dark",
		"--columns", "Name,ID,FOO"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	t.Log("create, API model error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(cOps).MinTimes(1)
	apiErr := &user.UserCreateDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	cOps.EXPECT().UserCreate(m).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	err = parseAndRun([]string{"user", "create", "-I", string(params.Payload.AuthIdentifier),
		"-c", "authIdentifier, ID",
		"-p", "user", "-P", "userName:full name", "-P", "guiTheme:dark"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	t.Log("API failure arbitrary error after successful interactive password")
	var b bytes.Buffer
	outputWriter = &b
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(cOps)
	otherErr := fmt.Errorf("API ERROR")
	cOps.EXPECT().UserCreate(m).Return(nil, otherErr)
	appCtx.API = mAPI
	assert.Zero(hookCnt)
	hookRet = [][]byte{[]byte("user"), []byte("user")}
	hookErr = []error{nil, nil}
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	params.Payload.Disabled = true
	err = parseAndRun([]string{"user", "create", "-I", string(params.Payload.AuthIdentifier),
		"-P", "userName:full name", "-P", "guiTheme:dark", "--disabled"})
	assert.NotNil(err)
	assert.Equal(otherErr, err)
	assert.Equal(2, hookCnt)
	assert.Equal("Password: \nRetype password: \n", b.String())

	t.Log("password mismatch")
	b.Reset()
	hookCnt = 0
	hookRet = [][]byte{[]byte("user"), []byte("used")}
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	params.Payload.Disabled = true
	err = parseAndRun([]string{"user", "create", "-I", string(params.Payload.AuthIdentifier),
		"-P", "userName:full name", "-P", "guiTheme:dark", "--disabled"})
	assert.NotNil(err)
	assert.Regexp("passwords do not match", err)
	assert.Equal(2, hookCnt)

	t.Log("first password prompt fails")
	b.Reset()
	hookCnt = 0
	hookRet = [][]byte{nil}
	hookErr = []error{errors.New("failure")}
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	params.Payload.Disabled = true
	err = parseAndRun([]string{"user", "create", "-I", string(params.Payload.AuthIdentifier),
		"-P", "userName:full name", "-P", "guiTheme:dark", "--disabled"})
	assert.NotNil(err)
	assert.Equal(hookErr[0], err)
	assert.Equal(1, hookCnt)

	t.Log("second password prompt fails")
	b.Reset()
	hookCnt = 0
	hookRet = [][]byte{[]byte("user"), nil}
	hookErr = []error{nil, errors.New("failure")}
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	params.Payload.Disabled = true
	err = parseAndRun([]string{"user", "create", "-I", string(params.Payload.AuthIdentifier),
		"-P", "userName:full name", "-P", "guiTheme:dark", "--disabled"})
	assert.NotNil(err)
	assert.Equal(hookErr[1], err)
	assert.Equal(2, hookCnt)

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
	initUser()
	err = parseAndRun([]string{"user", "create", "-A", "System", "-I", string(params.Payload.AuthIdentifier)})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	err = parseAndRun([]string{"user", "create", "-I", string(params.Payload.AuthIdentifier),
		"-c", "authIdentifier, ID", "-P", "userName:full name", "p", "guiTheme:dark"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestUserDelete(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	lRes := &user.UserListOK{
		Payload: []*models.User{
			&models.User{
				UserAllOf0: models.UserAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				UserMutable: models.UserMutable{
					AuthIdentifier: "user@host",
					Disabled:       true,
					Profile:        map[string]models.ValueType{},
				},
			},
		},
	}
	m := mockmgmtclient.NewUserMatcher(t, user.NewUserListParams().WithAuthIdentifier(&lRes.Payload[0].AuthIdentifier))
	dParams := user.NewUserDeleteParams()
	dParams.ID = string(lRes.Payload[0].Meta.ID)
	dRet := &user.UserDeleteNoContent{}

	// delete
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(cOps).MinTimes(1)
	cOps.EXPECT().UserList(m).Return(lRes, nil)
	cOps.EXPECT().UserDelete(dParams).Return(dRet, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	err := parseAndRun([]string{"user", "delete", "-I", lRes.Payload[0].AuthIdentifier, "--confirm"})
	assert.NoError(err)

	// delete, --confirm not specified
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	err = parseAndRun([]string{"user", "delete", "-I", lRes.Payload[0].AuthIdentifier})
	assert.NotNil(err)
	assert.Regexp("--confirm", err.Error())

	// delete, not found
	emptyRes := &user.UserListOK{
		Payload: []*models.User{},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(cOps).MinTimes(1)
	cOps.EXPECT().UserList(m).Return(emptyRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	err = parseAndRun([]string{"user", "delete", "-I", lRes.Payload[0].AuthIdentifier, "--confirm"})
	assert.NotNil(err)
	assert.Regexp("not found", err.Error())

	// delete, list error
	otherErr := fmt.Errorf("other error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(cOps).MinTimes(1)
	cOps.EXPECT().UserList(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	err = parseAndRun([]string{"user", "delete", "--auth-identifier", lRes.Payload[0].AuthIdentifier, "--confirm"})
	assert.NotNil(err)
	assert.Regexp("other error", err.Error())

	// delete, API model error
	apiErr := &user.UserDeleteDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(cOps).MinTimes(1)
	cOps.EXPECT().UserList(m).Return(lRes, nil)
	cOps.EXPECT().UserDelete(dParams).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	err = parseAndRun([]string{"user", "delete", "-I", lRes.Payload[0].AuthIdentifier, "--confirm"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// delete, API arbitrary error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(cOps).MinTimes(1)
	cOps.EXPECT().UserList(m).Return(lRes, nil)
	cOps.EXPECT().UserDelete(dParams).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	err = parseAndRun([]string{"user", "delete", "-I", lRes.Payload[0].AuthIdentifier, "--confirm"})
	assert.NotNil(err)
	assert.Regexp("other error", err.Error())

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
	initUser()
	err = parseAndRun([]string{"user", "delete", "-A", "System", "-I", lRes.Payload[0].AuthIdentifier, "--confirm"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	err = parseAndRun([]string{"user", "delete", "-I", lRes.Payload[0].AuthIdentifier, "confirm"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestUserList(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	m := mockmgmtclient.NewUserMatcher(t, user.NewUserListParams().WithAuthIdentifier(swag.String("")).WithUserNamePattern(swag.String("")))
	res := &user.UserListOK{
		Payload: []*models.User{
			&models.User{
				UserAllOf0: models.UserAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				UserMutable: models.UserMutable{
					AuthIdentifier: "user1",
				},
			},
			&models.User{
				UserAllOf0: models.UserAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id2",
						Version:      2,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				UserMutable: models.UserMutable{
					AuthIdentifier: "user2",
				},
			},
		},
	}

	// list, all defaults
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(cOps)
	cOps.EXPECT().UserList(m).Return(res, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	err := parseAndRun([]string{"user", "list"})
	assert.NoError(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(userDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list with filter, json
	m2 := mockmgmtclient.NewUserMatcher(t, user.NewUserListParams().WithAuthIdentifier(swag.String("admin")).WithUserNamePattern(swag.String("")))
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(cOps)
	cOps.EXPECT().UserList(m2).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	err = parseAndRun([]string{"user", "list", "-o", "json", "-I", "admin"})
	assert.NoError(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(res.Payload, te.jsonData)

	// list with filter, yaml
	m2 = mockmgmtclient.NewUserMatcher(t, user.NewUserListParams().WithAuthIdentifier(swag.String("")).WithUserNamePattern(swag.String("Titus")))
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(cOps)
	cOps.EXPECT().UserList(m2).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	err = parseAndRun([]string{"user", "list", "-o", "yaml", "-n", "Titus"})
	assert.NoError(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(res.Payload, te.yamlData)

	// list with columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(cOps)
	cOps.EXPECT().UserList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	err = parseAndRun([]string{"user", "list", "--columns", "AuthIdentifier,ID"})
	assert.NoError(err)
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
	initUser()
	err = parseAndRun([]string{"user", "list", "-c", "Name,ID"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// list, API failure model error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(cOps)
	apiErr := &user.UserListDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	cOps.EXPECT().UserList(m).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	err = parseAndRun([]string{"user", "list"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// list, API failure arbitrary error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(cOps)
	otherErr := fmt.Errorf("OTHER ERROR")
	cOps.EXPECT().UserList(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	err = parseAndRun([]string{"user", "list"})
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
	initUser()
	err = parseAndRun([]string{"user", "list", "-A", "System"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	err = parseAndRun([]string{"user", "list", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestUserModify(t *testing.T) {
	assert := assert.New(t)

	savedPasswordHook := passwordHook
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
		passwordHook = savedPasswordHook
	}()
	interactiveReader = nil
	var hookCnt int
	var hookRet [][]byte
	var hookErr []error
	passwordHook = func(int) ([]byte, error) {
		i := hookCnt
		hookCnt++
		return hookRet[i], hookErr[i]
	}

	now := time.Now()
	lRes := &user.UserListOK{
		Payload: []*models.User{
			&models.User{
				UserAllOf0: models.UserAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				UserMutable: models.UserMutable{
					AuthIdentifier: "user@host",
					Disabled:       true,
					Profile: map[string]models.ValueType{
						"userName": {Kind: "STRING", Value: "Tad LeBeck"},
					},
				},
			},
		},
	}
	m := mockmgmtclient.NewUserMatcher(t, user.NewUserListParams().WithAuthIdentifier(&lRes.Payload[0].AuthIdentifier))

	t.Log("update (APPEND) profile, password on CLI")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(cOps).MinTimes(1)
	cOps.EXPECT().UserList(m).Return(lRes, nil)
	uParams := user.NewUserUpdateParams()
	uParams.Payload = &models.UserMutable{
		AuthIdentifier: "new@host",
		Password:       "user",
		Profile:        map[string]models.ValueType{"guiTheme": {Kind: "STRING", Value: "dark"}},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Set = []string{"authIdentifier", "password"}
	uParams.Append = []string{"profile"}
	um := mockmgmtclient.NewUserMatcher(t, uParams)
	uRet := user.NewUserUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().UserUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	err := parseAndRun([]string{"user", "modify", "-I", lRes.Payload[0].AuthIdentifier,
		"--new-auth-identifier", "new@host", "--password=user", "--profile", "guiTheme:dark", "-o", "json"})
	assert.NoError(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.User{uRet.Payload}, te.jsonData)
	mockCtrl.Finish()

	t.Log("same but with SET")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(cOps).MinTimes(1)
	cOps.EXPECT().UserList(m).Return(lRes, nil)
	uParams = user.NewUserUpdateParams()
	uParams.Payload = &models.UserMutable{
		AuthIdentifier: "new@host",
		Disabled:       false,
		Password:       "user",
		Profile:        map[string]models.ValueType{"guiTheme": {Kind: "STRING", Value: "dark"}},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"authIdentifier", "disabled", "password", "profile"}
	um = mockmgmtclient.NewUserMatcher(t, uParams)
	uRet = user.NewUserUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().UserUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	err = parseAndRun([]string{"user", "modify", "--auth-identifier", lRes.Payload[0].AuthIdentifier,
		"-N", "new@host", "--enable", "-p" + "user", "-P", "guiTheme:dark",
		"--profile-action=SET", "--version", "1", "-o", "json"})
	assert.NoError(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.User{uRet.Payload}, te.jsonData)
	mockCtrl.Finish()

	t.Log("update (REMOVE) profile, specify password interactively")
	var b bytes.Buffer
	outputWriter = &b
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(cOps).MinTimes(1)
	cOps.EXPECT().UserList(m).Return(lRes, nil)
	uParams = user.NewUserUpdateParams()
	uParams.Payload = &models.UserMutable{
		AuthIdentifier: "new@host",
		Disabled:       true,
		Password:       "user",
		Profile:        map[string]models.ValueType{"guiTheme": {Kind: "STRING", Value: "dark"}},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"authIdentifier", "disabled", "password"}
	uParams.Remove = []string{"profile"}
	um = mockmgmtclient.NewUserMatcher(t, uParams)
	uRet = user.NewUserUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().UserUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	assert.Zero(hookCnt)
	hookRet = [][]byte{[]byte("user"), []byte("user")}
	hookErr = []error{nil, nil}
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	err = parseAndRun([]string{"user", "modify", "--login", lRes.Payload[0].AuthIdentifier,
		"-N", "new@host", "--disable", "-p", "-P", "guiTheme:dark",
		"--profile-action", "REMOVE", "-V", "1", "-o", "json"})
	assert.NoError(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.User{uRet.Payload}, te.jsonData)
	assert.Equal("New password: \nRetype new password: \n", b.String())
	appCtx.LoginName = ""
	mockCtrl.Finish()

	t.Log("profile empty SET, change other user password")
	b.Reset()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(cOps).MinTimes(1)
	cOps.EXPECT().UserList(m).Return(lRes, nil)
	uParams = user.NewUserUpdateParams()
	uParams.Payload = &models.UserMutable{
		AuthIdentifier: "new@host",
		Password:       "user",
		Profile:        map[string]models.ValueType{},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"authIdentifier", "password", "profile"}
	um = mockmgmtclient.NewUserMatcher(t, uParams)
	uRet = user.NewUserUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().UserUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	hookCnt = 0
	hookRet = [][]byte{[]byte("user"), []byte("user")}
	hookErr = []error{nil, nil}
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	err = parseAndRun([]string{"user", "modify", "-I", lRes.Payload[0].AuthIdentifier,
		"-N", "new@host", "--password", "--profile-action", "SET", "-V", "1", "-o", "json"})
	assert.NoError(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.User{uRet.Payload}, te.jsonData)
	assert.Equal("New password: \nRetype new password: \n", b.String())
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
	initUser()
	err = parseAndRun([]string{"user", "modify", "-I", lRes.Payload[0].AuthIdentifier, "-A", "System"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	for i := 0; i < 3; i++ {
		t.Logf("read password %d fails", i)
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
		cOps = mockmgmtclient.NewMockUserClient(mockCtrl)
		mAPI.EXPECT().User().Return(cOps).MinTimes(1)
		cOps.EXPECT().UserList(m).Return(lRes, nil)
		expErr := errors.New("read error")
		appCtx.API = mAPI
		hookCnt = 0
		hookRet = [][]byte{[]byte("u"), nil}
		hookErr = []error{expErr}
		if i == 1 {
			hookErr = []error{nil, expErr}
		} else if i == 2 {
			expErr = errors.New("passwords do not match")
			hookRet = [][]byte{[]byte("u"), []byte("v")}
			hookErr = []error{nil, nil}
		}
		te = &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initUser()
		err = parseAndRun([]string{"user", "modify", "-I", lRes.Payload[0].AuthIdentifier, "-p"})
		assert.NotNil(err)
		assert.Equal(expErr.Error(), err.Error())
	}

	// other error cases
	updateErr := user.NewUserUpdateDefault(400)
	updateErr.Payload = &models.Error{Code: 400, Message: swag.String("update error")}
	otherErr := fmt.Errorf("other error")
	mNotNil := gomock.Not(gomock.Nil)
	errTCs := []struct {
		name      string
		args      []string
		re        string
		ulErr     error
		ulRC      *user.UserListOK
		updateErr error
		noMock    bool
	}{
		{
			name: "No modifications",
			args: []string{"user", "modify", "--auth-identifier", lRes.Payload[0].AuthIdentifier, "-V", "1", "-o", "json"},
			re:   "No modifications",
		},
		{
			name:   "No login or auth-identifier",
			args:   []string{"user", "modify", "-V", "1", "-o", "json"},
			re:     "either --login or --auth-identifier is required",
			noMock: true,
		},
		{
			name:      "update default error",
			args:      []string{"user", "modify", "-I", lRes.Payload[0].AuthIdentifier, "-N", "NewName", "-V", "1", "-o", "json"},
			re:        "update error",
			updateErr: updateErr,
		},
		{
			name:      "update other error",
			args:      []string{"user", "modify", "-I", lRes.Payload[0].AuthIdentifier, "-N", "NewName", "-V", "1", "-o", "json"},
			re:        "other error",
			updateErr: otherErr,
		},
		{
			name:   "invalid columns",
			args:   []string{"user", "modify", "-I", lRes.Payload[0].AuthIdentifier, "--columns", "ID,foo"},
			re:     "invalid column",
			noMock: true,
		},
		{
			name: "user not found",
			args: []string{"user", "modify", "-I", lRes.Payload[0].AuthIdentifier, "-N", "NewName", "-V", "1", "-o", "json"},
			re:   "user.*not found",
			ulRC: &user.UserListOK{Payload: []*models.User{}},
		},
		{
			name:  "user list error",
			args:  []string{"user", "modify", "-I", lRes.Payload[0].AuthIdentifier, "-N", "NewName", "-V", "1", "-o", "json"},
			re:    "other error",
			ulErr: otherErr,
		},
		{
			name: "enable/disable",
			args: []string{"user", "modify", "-I", lRes.Payload[0].AuthIdentifier, "--enable", "--disable", "-o", "json"},
			re:   "enable.*disable.*together",
		},
	}
	for _, tc := range errTCs {
		t.Logf("case: %s", tc.name)
		if !tc.noMock {
			mockCtrl.Finish()
			mockCtrl = gomock.NewController(t)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			cOps = mockmgmtclient.NewMockUserClient(mockCtrl)
			mAPI.EXPECT().User().Return(cOps).MinTimes(1)
			if tc.ulErr != nil {
				cOps.EXPECT().UserList(m).Return(nil, tc.ulErr)
			} else {
				if tc.ulRC == nil {
					tc.ulRC = lRes
				}
				cOps.EXPECT().UserList(m).Return(tc.ulRC, nil)
			}
			if tc.updateErr != nil {
				cOps.EXPECT().UserUpdate(mNotNil).Return(nil, tc.updateErr)
			}
			appCtx.API = mAPI
		}
		te = &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initUser()
		err = parseAndRun(tc.args)
		assert.NotNil(err)
		assert.Nil(te.jsonData)
		assert.Regexp(tc.re, err.Error())
	}

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initUser()
	err = parseAndRun([]string{"user", "modify", "-I", lRes.Payload[0].AuthIdentifier,
		"--new-auth-identifier", "new@host", "--profile", "guiTheme:dark", "o", "json"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}
