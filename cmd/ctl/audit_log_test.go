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
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/audit_log"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestAuditLogMakeRecord(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	ac := &auditLogCmd{}
	ac.accounts = map[string]ctxIDName{
		"tid":    {id: "", name: "tenant 1"},
		"subId1": {id: "tid", name: "sub 1"},
		"subId2": {id: "tid", name: "sub 2"},
	}
	o := &models.AuditLogRecord{
		AccountID:       "subId2",
		AccountName:     "aName",
		Action:          "create",
		AuthIdentifier:  "a@b.com",
		Classification:  "audit",
		Error:           false,
		Message:         "message",
		Name:            "oName",
		ObjectID:        "oID",
		ObjectType:      "user",
		ParentNum:       0,
		RecordNum:       5,
		TenantAccountID: "tid",
		Timestamp:       strfmt.DateTime(now),
		UserID:          "uid1",
	}
	rec := ac.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(auditLogHeaders))
	assert.Equal("5", rec[hRecordNum])
	assert.Equal("0", rec[hParentNum])
	assert.Equal(now.Format(time.RFC3339), rec[hTimestamp])
	assert.Equal("audit", rec[hClassification])
	assert.Equal(o.ObjectType, rec[hObjectType])
	assert.EqualValues(o.ObjectID, rec[hID])
	assert.Equal(o.Name, rec[hName])
	assert.Equal(o.Action, rec[hAction])
	assert.Equal("tenant 1", rec[hTenant])
	assert.EqualValues(o.AccountID, rec[hAccountID])
	assert.Equal(o.AccountName, rec[hAccount])
	assert.EqualValues(o.UserID, rec[hUserID])
	assert.Equal(o.AuthIdentifier, rec[hAuthIdentifier])
	assert.Equal("", rec[hError])
	assert.Equal(o.Message, rec[hMessage])

	// no caches, true error, parent
	ac.accounts = map[string]ctxIDName{}
	o.Error = true
	o.ParentNum = 2
	rec = ac.makeRecord(o)
	t.Log(rec)
	assert.Equal("tid", rec[hTenant])
	assert.Equal("2", rec[hParentNum])
	assert.Equal("E", rec[hError])
}

func TestAuditLogCreate(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	params := audit_log.NewAuditLogCreateParams()
	params.Payload = &models.AuditLogRecord{
		Classification: "annotation",
		Error:          false,
		ParentNum:      5,
	}
	m := mockmgmtclient.NewAuditLogMatcher(t, params)
	res := &audit_log.AuditLogCreateCreated{}
	res.Payload = &models.AuditLogRecord{
		AccountID:       "tid1",
		AccountName:     "aName1",
		Action:          "create",
		AuthIdentifier:  "a@b.com",
		Classification:  "audit",
		Error:           false,
		Message:         "message1",
		Name:            "oName1",
		ObjectID:        "oID1",
		ObjectType:      "user",
		ParentNum:       5,
		RecordNum:       8,
		TenantAccountID: "",
		Timestamp:       strfmt.DateTime(now),
		UserID:          "uid1",
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

	// create
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	lOps := mockmgmtclient.NewMockAuditLogClient(mockCtrl)
	mAPI.EXPECT().AuditLog().Return(lOps).MinTimes(1)
	lOps.EXPECT().AuditLogCreate(m).Return(res, nil)
	aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuditLog()
	err := parseAndRun([]string{"al", "annotate", "--parent=5"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(auditLogDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, 1)

	// create with trimmed params arbitrary error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	params.Payload.Action = "fixed"
	params.Payload.Message = "boo"
	params.Payload.Name = "mine"
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	lOps = mockmgmtclient.NewMockAuditLogClient(mockCtrl)
	mAPI.EXPECT().AuditLog().Return(lOps).MinTimes(1)
	lOps.EXPECT().AuditLogCreate(m).Return(nil, fmt.Errorf("arbitrary-error"))
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuditLog()
	err = parseAndRun([]string{"audit-log", "create", "--action= Fixed ", "--message= boo  ", "--name= mine ", "-p", "5"})
	assert.NotNil(err)
	assert.Regexp("arbitrary-error", err)

	// create with params, API model error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	params.Payload.Action = "oops"
	params.Payload.Error = true
	params.Payload.Message = "my bad"
	params.Payload.Name = ""
	apiErr := &audit_log.AuditLogCreateDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	lOps = mockmgmtclient.NewMockAuditLogClient(mockCtrl)
	mAPI.EXPECT().AuditLog().Return(lOps).MinTimes(1)
	lOps.EXPECT().AuditLogCreate(m).Return(nil, apiErr)
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuditLog()
	err = parseAndRun([]string{"audit-log", "create", "-a", "oops", "--error", "-m", "my bad", "-p", "5"})
	assert.NotNil(err)
	assert.Regexp("^api error$", err)

	// missing parent
	err = parseAndRun([]string{"al", "ann"})
	assert.NotNil(err)
	assert.Regexp("required flag.*parent.*not specified", err)

	// create with invalid columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuditLog()
	err = parseAndRun([]string{"audit-log", "ann", "-c", "Name,Foo", "-p", "0"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

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
	initAuditLog()
	err = parseAndRun([]string{"al", "ann", "-A", "System", "-p", "4"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuditLog()
	err = parseAndRun([]string{"al", "annotate", "--parent=5", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestAuditLogList(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	params := audit_log.NewAuditLogListParams()
	m := mockmgmtclient.NewAuditLogMatcher(t, params)
	res := &audit_log.AuditLogListOK{
		Payload: []*models.AuditLogRecord{
			&models.AuditLogRecord{
				AccountID:       "tid1",
				AccountName:     "aName1",
				Action:          "create",
				AuthIdentifier:  "a@b.com",
				Classification:  "audit",
				Error:           false,
				Message:         "message1",
				Name:            "oName1",
				ObjectID:        "oID1",
				ObjectType:      "user",
				ParentNum:       0,
				RecordNum:       5,
				TenantAccountID: "",
				Timestamp:       strfmt.DateTime(now),
				UserID:          "uid1",
			},
			&models.AuditLogRecord{
				AccountID:       "sid1",
				AccountName:     "aName",
				Action:          "update",
				AuthIdentifier:  "a@b.com",
				Classification:  "event",
				Error:           true,
				Message:         "message2",
				Name:            "oName2",
				ObjectID:        "oID2",
				ObjectType:      "storage",
				ParentNum:       0,
				RecordNum:       6,
				TenantAccountID: "tid1",
				Timestamp:       strfmt.DateTime(now),
				UserID:          "uid2",
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
	lOps := mockmgmtclient.NewMockAuditLogClient(mockCtrl)
	mAPI.EXPECT().AuditLog().Return(lOps)
	lOps.EXPECT().AuditLogList(m).Return(res, nil)
	aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuditLog()
	err := parseAndRun([]string{"al", "list"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(auditLogDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list, json, several options
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	params = audit_log.NewAuditLogListParams()
	params.AccountID = swag.String("tid1")
	params.Name = swag.String("myVol")
	params.ObjectType = swag.String("volumeseries")
	params.Error = swag.Bool(true)
	params.Count = swag.Int32(20)
	aTmGe := strfmt.DateTime(time.Now().Add(-2 * 24 * time.Hour))
	params.TimeStampGE = &aTmGe
	m = mockmgmtclient.NewAuditLogMatcher(t, params)
	m.DTimeStampGE = 2 * 24 * time.Hour
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "Nuvoloso").Return("tid1", nil)
	lOps = mockmgmtclient.NewMockAuditLogClient(mockCtrl)
	mAPI.EXPECT().AuditLog().Return(lOps)
	lOps.EXPECT().AuditLogList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuditLog()
	err = parseAndRun([]string{"audit-log", "list", "-T", "volume-series", "-n", "myVol", "--error", "-R", "2d", "-A", "Nuvoloso", "--account-only", "--count=20", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(res.Payload, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// list, yaml, remaining options
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	ts := "2018-10-29T15:54:51Z"
	tv, err := time.Parse(time.RFC3339, ts)
	assert.NoError(err)
	dt := strfmt.DateTime(tv)
	params = audit_log.NewAuditLogListParams()
	params.AccountID = swag.String("tid1")
	params.Classification = swag.String("audit")
	params.ObjectType = swag.String("user")
	params.ObjectID = swag.String("oid4")
	params.Related = swag.Bool(true)
	params.Error = swag.Bool(false)
	params.RecordNumGE = swag.Int32(2)
	params.RecordNumLE = swag.Int32(99)
	params.TimeStampLE = &dt
	m = mockmgmtclient.NewAuditLogMatcher(t, params)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "System").Return("accountID", nil)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	lOps = mockmgmtclient.NewMockAuditLogClient(mockCtrl)
	mAPI.EXPECT().AuditLog().Return(lOps)
	lOps.EXPECT().AuditLogList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuditLog()
	err = parseAndRun([]string{"audit-log", "-A", "System", "list", "-S", "Nuvoloso", "-C", "audit", "--type=user", "--id=oid4", "-r", "--no-error", "-B", ts, "--record-num-ge=2", "--record-num-le=99", "-o", "yaml"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(res.Payload, te.yamlData)
	appCtx.Account, appCtx.AccountID = "", ""

	// list with conflict
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuditLog()
	err = parseAndRun([]string{"audit-log", "list", "--error", "--no-error", "-c", "+ID"})
	assert.Regexp("do not specify.*no-error", err)

	// account conflict
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "System").Return("accountID", nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuditLog()
	err = parseAndRun([]string{"audit-log", "list", "-A", "System", "--account-only", "-S", "Nuvoloso", "--related"})
	assert.Regexp("do not specify.*only.*subordinate", err)
	appCtx.Account, appCtx.AccountID = "", ""

	// invalid subordinate
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "System").Return("accountID", nil)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuditLog()
	err = parseAndRun([]string{"audit-log", "list", "-A", "System", "--subordinate-account", "dave"})
	assert.Regexp("account 'dave' not found", err)
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
	initAuditLog()
	err = parseAndRun([]string{"audit-log", "list", "-c", "Name,ID,Foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// list, API failure model error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	params = audit_log.NewAuditLogListParams()
	m = mockmgmtclient.NewAuditLogMatcher(t, params)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	lOps = mockmgmtclient.NewMockAuditLogClient(mockCtrl)
	mAPI.EXPECT().AuditLog().Return(lOps)
	apiErr := &audit_log.AuditLogListDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	lOps.EXPECT().AuditLogList(m).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuditLog()
	err = parseAndRun([]string{"audit-log", "list"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// list, API failure arbitrary error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	lOps = mockmgmtclient.NewMockAuditLogClient(mockCtrl)
	mAPI.EXPECT().AuditLog().Return(lOps)
	otherErr := fmt.Errorf("OTHER ERROR")
	lOps.EXPECT().AuditLogList(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuditLog()
	err = parseAndRun([]string{"audit-log", "list"})
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
	initAuditLog()
	err = parseAndRun([]string{"al", "list", "-A", "System"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("id + extra")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuditLog()
	err = parseAndRun([]string{"al", "list", "--id", "id", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}
