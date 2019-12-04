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


package audit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/audit_log"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fhk "github.com/Nuvoloso/kontroller/pkg/housekeeping/fake"
	fpg "github.com/Nuvoloso/kontroller/pkg/pgdb/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	fw "github.com/Nuvoloso/kontroller/pkg/util/fake"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/jackc/pgx"
	"github.com/stretchr/testify/assert"
)

func TestErrorHelpers(t *testing.T) {
	assert := assert.New(t)
	lh := &logHandler{}

	assert.Equal(centrald.ErrorMissing.C, lh.eCode(centrald.ErrorMissing))
	assert.Equal(http.StatusInternalServerError, lh.eCode("random"))

	exp := &models.Error{Code: int32(centrald.ErrorMissing.C), Message: swag.String(centrald.ErrorMissing.M)}
	assert.Equal(exp, lh.eError(centrald.ErrorMissing))
	exp = &models.Error{Code: http.StatusInternalServerError, Message: swag.String("random")}
	assert.Equal(exp, lh.eError("random"))
}

func TestInsert(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ctx := context.Background()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			API:    &operations.NuvolosoAPI{},
			Log:    tl.Logger(),
			Server: &restapi.Server{},
		},
	}
	fts := &fhk.TaskScheduler{}
	app.TaskScheduler = fts
	c := &Component{}
	c.Init(app)
	assert.NotNil(c.worker)
	tw := &fw.Worker{}
	c.worker = tw
	assert.NotNil(c.pgDB)
	pg := &fpg.DB{}
	c.pgDB = pg

	lh := &logHandler{}
	lh.Init(c)

	t.Log("case: version mismatch")
	c.tableVersions = map[string]int{
		"AuditLog": 0,
	}
	stmt, err := lh.prepareStmt(ctx)
	assert.Error(err)
	assert.Regexp("unsupported AuditLog version", err)
	assert.Nil(stmt)

	t.Log("case: not ready")
	ret, err := lh.insert(nil, nil)
	assert.Zero(ret)
	assert.Regexp(centrald.ErrorDbError.M+".*Audit Log is not ready", err)

	t.Log("case: prepare fails in insert")
	c.tableVersions = map[string]int{
		"AuditLog": 1,
	}
	m := &LogRecord{}
	c.db, err = pg.OpenDB(c.getDBArgs())
	assert.NoError(err)
	c.dbConnected = true
	c.ready = true
	pg.Mock.ExpectPrepare("AuditLogInsert1").WillReturnError(fmt.Errorf("prepare-error"))
	ret, err = lh.insert(ctx, m)
	assert.Zero(ret)
	assert.Regexp("prepare-error", err)
	assert.NoError(pg.Mock.ExpectationsWereMet())

	t.Log("case: insert fails on QueryRow, caches prepared statement")
	pg.Mock.ExpectPrepare("AuditLogInsert1").
		ExpectQuery().WillReturnError(fmt.Errorf("exec-error"))
	assert.Nil(c.StmtCacheGet("AuditLog"))
	ret, err = lh.insert(ctx, m)
	assert.Zero(ret)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.NotNil(c.StmtCacheGet("AuditLog"))

	t.Log("case: prepare ok, insert fails on connection error, worker notified")
	m = &LogRecord{
		Timestamp:       time.Now(),
		ParentNum:       0,
		Classification:  "audit",
		ObjectType:      "type1",
		ObjectID:        "oid1",
		ObjectName:      "name1",
		RefObjectID:     "rid1",
		TenantAccountID: "tid1",
		AccountID:       "aid1",
		AccountName:     "name1a",
		UserID:          "uid1",
		AuthIdentifier:  "u1",
		Action:          "action1",
		Error:           true,
		Message:         "m1",
	}
	conErr := pgx.PgError{Code: "08000"}
	pg.Mock.ExpectQuery(".*").WillReturnError(conErr)
	ret, err = lh.insert(ctx, m)
	assert.Zero(ret)
	assert.Error(err)
	assert.Regexp("SQLSTATE 08000", err)
	assert.Equal(1, tw.CntNotify)

	t.Log("case: success")
	columns := []string{"RecordNum"}
	rows := sqlmock.NewRows(columns).AddRow(3)
	pg.Mock.ExpectQuery(".*").
		WithArgs(m.Timestamp, m.ParentNum, m.Classification, m.ObjectType, m.ObjectID, m.ObjectName, m.RefObjectID, m.TenantAccountID, m.AccountID, m.AccountName, m.UserID, m.AuthIdentifier, m.Action, m.Error, m.Message).
		WillReturnRows(rows)
	ret, err = lh.insert(ctx, m)
	assert.EqualValues(3, ret)
	assert.NoError(err)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.Equal(1, tl.CountPattern("Inserted AuditLog record .3"))
}

func TestExpire(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log: tl.Logger(),
			API: api,
		},
	}
	fts := &fhk.TaskScheduler{}
	app.TaskScheduler = fts
	hc := &Component{}
	hc.Log = tl.Logger()
	hc.App = app
	pg := &fpg.DB{}
	hc.pgDB = pg
	lh := &hc.handler
	lh.c = hc
	hc.handler.c.AuditRecordRetentionPeriod = time.Duration(72 * time.Hour)
	hc.handler.c.AuditEventRetentionPeriod = time.Duration(36 * time.Hour)
	hc.handler.c.AuditAnnotationRetentionPeriod = time.Duration(24 * time.Hour)
	ctx := context.Background()
	now := time.Now()

	t.Log("case: version mismatch")
	hc.tableVersions = map[string]int{
		"AuditLog": 0,
	}
	err := hc.Expire(ctx, now)
	assert.Error(err)
	assert.Equal(1, tl.CountPattern("Unsupported AuditLog version"))
	hc.tableVersions = map[string]int{
		"AuditLog": 1,
	}

	t.Log("case: not connected")
	err = hc.Expire(ctx, now)
	assert.Error(err)

	hc.db, err = pg.OpenDB(hc.getDBArgs())
	assert.NoError(err)
	hc.dbConnected = true
	tl.Flush()

	t.Log("case: success")
	qWhere := " WHERE Classification = $1 AND TimeStamp < $2"
	q := "DELETE FROM AuditLog" + regexp.QuoteMeta(qWhere)

	pg.Mock.ExpectExec(q).WithArgs(com.AnnotationClass, strfmt.DateTime(now.Add(-hc.handler.c.AuditAnnotationRetentionPeriod)).String()).WillReturnResult(sqlmock.NewResult(1, 1))
	pg.Mock.ExpectExec(q).WithArgs(com.AuditClass, strfmt.DateTime(now.Add(-hc.handler.c.AuditRecordRetentionPeriod)).String()).WillReturnResult(sqlmock.NewResult(2, 2))
	pg.Mock.ExpectExec(q).WithArgs(com.EventClass, strfmt.DateTime(now.Add(-hc.handler.c.AuditEventRetentionPeriod)).String()).WillReturnResult(sqlmock.NewResult(3, 3))
	err = hc.Expire(ctx, now)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.NoError(err)
	assert.Equal(1, tl.CountPattern("successfully deleted 1 annotation records"))
	assert.Equal(1, tl.CountPattern("successfully deleted 2 audit records"))
	assert.Equal(1, tl.CountPattern("successfully deleted 3 event records"))

	t.Log("case: error on delete")
	pg.Mock.ExpectExec(q).WithArgs(com.AnnotationClass, strfmt.DateTime(now.Add(-hc.handler.c.AuditAnnotationRetentionPeriod)).String()).WillReturnError(errors.New("exec-error"))
	err = hc.Expire(ctx, now)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.Error(err)
	assert.Equal(1, tl.CountPattern("AuditLog delete annotation records query error"))
	assert.Equal(1, tl.CountPattern("exec-error"))

	// failure to process delete results
	tl.Logger().Info("case: failure to process delete results")
	pg.Mock.ExpectExec(q).WithArgs(com.AnnotationClass, strfmt.DateTime(now.Add(-hc.handler.c.AuditAnnotationRetentionPeriod)).String()).WillReturnResult(sqlmock.NewErrorResult(errors.New("rows-affected-error")))
	err = hc.Expire(ctx, now)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.Error(err)
	assert.Equal(1, tl.CountPattern("AuditLog error to process delete query results"))
	assert.Equal(1, tl.CountPattern("rows-affected-error"))
}

func TestPost(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ctx := context.Background()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			API:    &operations.NuvolosoAPI{},
			Log:    tl.Logger(),
			Server: &restapi.Server{},
		},
	}
	fts := &fhk.TaskScheduler{}
	app.TaskScheduler = fts
	c := Component{}
	c.Init(app)
	assert.NotNil(c.worker)
	tw := &fw.Worker{}
	c.worker = tw
	fi := &fakeInserter{}
	c.handler.i = fi
	assert.NotNil(c.pgDB)

	assert.EqualValues(0, centrald.NoOpAuditAction)
	assert.Len(actionMap, int(centrald.LastAuditAction)-1, "all actions should be in the actionMap")

	assert.Panics(func() { c.Post(ctx, nil, centrald.UserUpdateAction, "oid1", "name", "rid1", false, "messy") })
	assert.Zero(fi.cnt)
	assert.Nil(fi.m)

	// zeros
	now := time.Now()
	c.Post(ctx, &auth.Info{}, centrald.UserCreateAction, "", "", "", false, "")
	assert.Equal(1, fi.cnt)
	m := fi.m
	assert.True(m.Timestamp.After(now))
	assert.Zero(m.ParentNum)
	assert.Equal("audit", m.Classification)
	assert.Equal("user", m.ObjectType)
	assert.Equal(nilUUID, m.ObjectID)
	assert.Empty(m.ObjectName)
	assert.Equal(nilUUID, m.TenantAccountID)
	assert.Equal(nilUUID, m.AccountID)
	assert.Empty(m.AccountName)
	assert.Equal(nilUUID, m.UserID)
	assert.Empty(m.AuthIdentifier)
	assert.Equal("create", m.Action)
	assert.False(m.Error)
	assert.Empty(m.Message)
	tl.Flush()

	// non-zero
	ai := &auth.Info{
		AuthIdentifier:  "who",
		AccountID:       "aid1",
		AccountName:     "Normal",
		TenantAccountID: "tid1",
		UserID:          "uid1",
	}
	c.handler.log(ctx, ai, "audit", 0, centrald.UserUpdateAction, "oid1", "name", "rid1", true, "mess")
	assert.Equal(2, fi.cnt)
	m = fi.m
	assert.True(m.Timestamp.After(now))
	assert.Zero(m.ParentNum)
	assert.Equal("audit", m.Classification)
	assert.Equal("user", m.ObjectType)
	assert.Equal("oid1", m.ObjectID)
	assert.Equal("name", m.ObjectName)
	assert.Equal("rid1", m.RefObjectID)
	assert.Equal("tid1", m.TenantAccountID)
	assert.Equal("aid1", m.AccountID)
	assert.Equal("Normal", m.AccountName)
	assert.Equal("uid1", m.UserID)
	assert.Equal("who", m.AuthIdentifier)
	assert.Equal("update", m.Action)
	assert.True(m.Error)
	assert.Equal("mess", m.Message)

	// Annotation entry point
	c.Annotation(ctx, ai, 2, centrald.UserUpdateAction, "oid1", "name", false, "I did it")
	assert.Equal(3, fi.cnt)
	m = fi.m
	assert.True(m.Timestamp.After(now))
	assert.Equal("annotation", m.Classification)
	assert.EqualValues(2, m.ParentNum)
	assert.Equal("user", m.ObjectType)
	assert.Equal("oid1", m.ObjectID)
	assert.Equal("name", m.ObjectName)
	assert.Equal(nilUUID, m.RefObjectID)
	assert.Equal("tid1", m.TenantAccountID)
	assert.Equal("aid1", m.AccountID)
	assert.Equal("Normal", m.AccountName)
	assert.Equal("uid1", m.UserID)
	assert.Equal("who", m.AuthIdentifier)
	assert.Equal("update", m.Action)
	assert.False(m.Error)
	assert.Equal("I did it", m.Message)

	// Event entry point
	c.Event(ctx, ai, centrald.UserUpdateAction, "oid1", "name", "rid1", true, "bad")
	assert.Equal(4, fi.cnt)
	m = fi.m
	assert.True(m.Timestamp.After(now))
	assert.Equal("event", m.Classification)
	assert.Zero(m.ParentNum)
	assert.Equal("user", m.ObjectType)
	assert.Equal("oid1", m.ObjectID)
	assert.Equal("name", m.ObjectName)
	assert.Equal("rid1", m.RefObjectID)
	assert.Equal("tid1", m.TenantAccountID)
	assert.Equal("aid1", m.AccountID)
	assert.Equal("Normal", m.AccountName)
	assert.Equal("uid1", m.UserID)
	assert.Equal("who", m.AuthIdentifier)
	assert.Equal("update", m.Action)
	assert.True(m.Error)
	assert.Equal("bad", m.Message)
}

var listColumns = []string{"RecordNum", "Timestamp", "ParentNum", "Classification", "ObjectType", "ObjectID", "ObjectName", "RefObjectID", "TenantAccountID", "AccountID", "AccountName", "UserID", "AuthIdentifier", "Action", "Error", "Message"}

func TestAuditLogCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log: tl.Logger(),
			API: api,
		},
	}
	fts := &fhk.TaskScheduler{}
	app.TaskScheduler = fts
	hc := &Component{}
	hc.Log = tl.Logger()
	hc.App = app
	pg := &fpg.DB{}
	hc.pgDB = pg
	lh := &hc.handler
	lh.c = hc
	fi := &fakeInserter{}
	lh.i = fi
	ai := &auth.Info{}
	params := ops.AuditLogCreateParams{}
	now := time.Now()
	then := time.Now().Add(-10 * time.Minute)
	hc.tableVersions = map[string]int{
		"AuditLog": 1,
	}

	t.Log("case: auth internal error")
	var ret middleware.Responder
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = lh.auditLogCreate(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.AuditLogCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)

	t.Log("case: unauthorized")
	ai.RoleObj = &models.Role{}
	params.HTTPRequest = requestWithAuthContext(ai)
	assert.NotPanics(func() { ret = lh.auditLogCreate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AuditLogCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)

	t.Log("case: not annotation")
	ai.RoleObj.Capabilities = map[string]bool{centrald.AuditLogAnnotateCap: true}
	params.Payload = &models.AuditLogRecord{Classification: "audit"}
	assert.NotPanics(func() { ret = lh.auditLogCreate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AuditLogCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorMissing.M+": .*annotation", *mD.Payload.Message)

	t.Log("case: not annotation")
	params.Payload = &models.AuditLogRecord{Classification: "event"}
	assert.NotPanics(func() { ret = lh.auditLogCreate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AuditLogCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorMissing.M+": .*annotation", *mD.Payload.Message)

	t.Log("case: no account")
	params.Payload.Classification = "annotation"
	assert.NotPanics(func() { ret = lh.auditLogCreate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AuditLogCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorMissing.M+": accountId", *mD.Payload.Message)

	t.Log("case: account in object, no parent or objectId")
	ai.RoleObj = nil
	params.Payload.AccountID, params.Payload.AccountName = "aid1", "aName"
	assert.NotPanics(func() { ret = lh.auditLogCreate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AuditLogCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorMissing.M+": .*parentNum or objectId", *mD.Payload.Message)

	t.Log("case: parent lookup error") // not connected
	params.Payload.ParentNum = 1
	assert.NotPanics(func() { ret = lh.auditLogCreate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AuditLogCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorDbError.M+".*database not connected", *mD.Payload.Message)

	var err error
	hc.db, err = pg.OpenDB(hc.getDBArgs())
	assert.NoError(err)
	hc.dbConnected = true

	t.Log("case: invalid parentNum")
	rows := sqlmock.NewRows(listColumns)
	qBase := fmt.Sprintf("^SELECT %s FROM AuditLog", strings.Join(listColumns, ", "))
	order := " ORDER BY RecordNum ASC"
	where := " WHERE RecordNum >= $1 AND RecordNum <= $2"
	q := qBase + regexp.QuoteMeta(where) + order
	pg.Mock.ExpectQuery(q).WithArgs(1, 1).WillReturnRows(rows)
	assert.NotPanics(func() { ret = lh.auditLogCreate(params) })
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AuditLogCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorMissing.M+": invalid parentNum", *mD.Payload.Message)

	t.Log("case: objectId, invalid objectType")
	params.Payload.ParentNum = 0
	params.Payload.ObjectID = "123"
	params.Payload.ObjectType = "unknown"
	assert.NotPanics(func() { ret = lh.auditLogCreate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AuditLogCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorMissing.M+": invalid objectType", *mD.Payload.Message)

	t.Log("case: objectId, empty invalid objectType")
	params.Payload.ParentNum = 0
	params.Payload.ObjectID = "123"
	params.Payload.ObjectType = ""
	assert.NotPanics(func() { ret = lh.auditLogCreate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AuditLogCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorMissing.M+": invalid objectType", *mD.Payload.Message)

	t.Log("case: objectId, insert unexpected error")
	params.Payload.ObjectType = "  VolumeSeries  "
	params.Payload.Action = " Working "
	params.Payload.Message = " Free Form "
	fi.retErr = errors.New("insert-error")
	assert.NotPanics(func() { ret = lh.auditLogCreate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AuditLogCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Regexp("insert-error", *mD.Payload.Message)
	assert.Equal(1, fi.cnt)
	m := fi.m
	assert.Zero(params.Payload.RecordNum)
	assert.True(m.Timestamp.After(now))
	assert.Equal("annotation", m.Classification)
	assert.Zero(m.ParentNum)
	assert.Equal("volumeseries", m.ObjectType)
	assert.Equal("123", m.ObjectID)
	assert.Empty(m.ObjectName)
	assert.Equal(nilUUID, m.TenantAccountID)
	assert.Equal("aid1", m.AccountID)
	assert.Equal("aName", m.AccountName)
	assert.Equal(nilUUID, m.UserID)
	assert.Empty(m.AuthIdentifier)
	assert.Equal("working", m.Action)
	assert.False(m.Error)
	assert.Equal("Free Form", m.Message)

	t.Log("case: account in authInfo, insert error")
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.AuditLogAnnotateCap: true, centrald.AccountFetchAllRolesCap: true}
	ai.AccountID, ai.AccountName, ai.TenantAccountID = "aid2", "another", "tid1"
	ai.UserID, ai.AuthIdentifier = "uid1", "me@work"
	params.Payload.Action = ""
	params.Payload.Message = "free Form"
	params.Payload.ParentNum = 1
	params.Payload.ObjectID = ""
	params.Payload.ObjectType = "volumeseries"
	params.Payload.Name = "named"
	params.Payload.Error = true
	fi.retErr = centrald.ErrorInternalError
	rows = sqlmock.NewRows(listColumns).
		AddRow(1, then, 0, "audit", "node", "oID1", "n1", "rID1", "tID1", "aID1", "a1", "uID1", "i1", "create", false, "m1")
	where = " WHERE AccountID = $1 AND RecordNum >= $2 AND RecordNum <= $3"
	q = qBase + regexp.QuoteMeta(where) + order
	pg.Mock.ExpectQuery(q).WithArgs(ai.AccountID, 1, 1).WillReturnRows(rows)
	assert.NotPanics(func() { ret = lh.auditLogCreate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AuditLogCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	assert.Equal(2, fi.cnt)
	m = fi.m
	assert.Zero(params.Payload.RecordNum)
	assert.True(m.Timestamp.After(now))
	assert.Equal("annotation", m.Classification)
	assert.EqualValues(1, m.ParentNum)
	assert.Equal("node", m.ObjectType) // payload value is overridden
	assert.Equal("oID1", m.ObjectID)   // payload value is overridden
	assert.Equal("named", m.ObjectName)
	assert.Equal("tid1", m.TenantAccountID)
	assert.Equal("aid2", m.AccountID)
	assert.Equal("another", m.AccountName)
	assert.Equal("uid1", m.UserID)
	assert.Equal("me@work", m.AuthIdentifier)
	assert.Equal("note", m.Action) // default
	assert.True(m.Error)
	assert.Equal("free Form", m.Message)

	t.Log("case: success")
	params.Payload.ParentNum = 3
	params.Payload.ObjectID = "both"
	params.Payload.Action = "resolved"
	params.Payload.Error = false
	fi.retSeq = 5 // gets incremented
	fi.retErr = nil
	rows = sqlmock.NewRows(listColumns).
		AddRow(1, then, 0, "audit", "node", "oID1", "n1", "rID1", "tID1", "aID1", "a1", "uID1", "i1", "create", false, "m1")
	pg.Mock.ExpectQuery(q).WithArgs(ai.AccountID, 3, 3).WillReturnRows(rows)
	assert.NotPanics(func() { ret = lh.auditLogCreate(params) })
	assert.NotNil(ret)
	mC, ok := ret.(*ops.AuditLogCreateCreated)
	assert.True(ok)
	assert.Equal(3, fi.cnt)
	m = fi.m
	assert.Equal(params.Payload, mC.Payload)
	assert.EqualValues(6, params.Payload.RecordNum)
	assert.EqualValues(params.Payload.Timestamp, m.Timestamp)
	assert.True(m.Timestamp.After(now))
	assert.Equal("annotation", m.Classification)
	assert.EqualValues(3, m.ParentNum)
	assert.Equal("node", m.ObjectType) // payload value is overridden
	assert.Equal("oID1", m.ObjectID)   // payload value is overridden
	assert.Equal("named", m.ObjectName)
	assert.Equal("tid1", m.TenantAccountID)
	assert.Equal("aid2", m.AccountID)
	assert.Equal("another", m.AccountName)
	assert.Equal("uid1", m.UserID)
	assert.Equal("me@work", m.AuthIdentifier)
	assert.Equal("resolved", m.Action)
	assert.False(m.Error)
	assert.Equal("free Form", m.Message)
}

func TestAuditLogList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log: tl.Logger(),
			API: api,
		},
	}
	fts := &fhk.TaskScheduler{}
	app.TaskScheduler = fts
	hc := &Component{}
	hc.Log = tl.Logger()
	hc.App = app
	pg := &fpg.DB{}
	hc.pgDB = pg
	lh := &hc.handler
	lh.c = hc
	ai := &auth.Info{}
	params := ops.AuditLogListParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	now := time.Now()
	then := time.Now().Add(-10 * time.Minute)
	rList := []*models.AuditLogRecord{
		{RecordNum: 1, Timestamp: strfmt.DateTime(then), ParentNum: 0, Classification: "audit", ObjectType: "node", ObjectID: "oID1", Name: "n1", RefObjectID: "rID1", TenantAccountID: "tID1", AccountID: "aID1", AccountName: "a1", UserID: "uID1", AuthIdentifier: "i1", Action: "create", Error: false, Message: "m1"},
		{RecordNum: 3, Timestamp: strfmt.DateTime(now), ParentNum: 0, Classification: "event", ObjectType: "pool", ObjectID: "", Name: "n2", TenantAccountID: "", AccountID: "", AccountName: "a2", UserID: "", AuthIdentifier: "i2", Action: "fetch", Error: true, Message: "m2"},
	}

	t.Log("case: version mismatch")
	hc.tableVersions = map[string]int{
		"AuditLog": 0,
	}
	var ret middleware.Responder
	assert.NotPanics(func() { ret = lh.auditLogList(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.AuditLogListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorDbError.M+".*Unsupported AuditLog version", *mD.Payload.Message)
	assert.Equal(1, tl.CountPattern("Unsupported AuditLog version"))
	hc.tableVersions = map[string]int{
		"AuditLog": 1,
	}

	t.Log("case: not connected")
	assert.NotPanics(func() { ret = lh.auditLogList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AuditLogListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorDbError.M+".*database not connected", *mD.Payload.Message)

	var err error
	hc.db, err = pg.OpenDB(hc.getDBArgs())
	assert.NoError(err)
	hc.dbConnected = true

	t.Log("case: success, internal role, no params")
	rows := sqlmock.NewRows(listColumns).
		AddRow(1, then, 0, "audit", "node", "oID1", "n1", "rID1", "tID1", "aID1", "a1", "uID1", "i1", "create", false, "m1").
		AddRow(3, now, 0, "event", "pool", nilUUID, "n2", nilUUID, nilUUID, nilUUID, "a2", nilUUID, "i2", "fetch", true, "m2")
	qBase := fmt.Sprintf("^SELECT %s FROM AuditLog", strings.Join(listColumns, ", "))
	order := " ORDER BY RecordNum ASC"
	q := qBase + order
	pg.Mock.ExpectQuery(q).WillReturnRows(rows)
	assert.NotPanics(func() { ret = lh.auditLogList(params) })
	assert.NoError(pg.Mock.ExpectationsWereMet())
	mO, ok := ret.(*ops.AuditLogListOK)
	assert.True(ok)
	assert.NotNil(mO)
	assert.Equal(rList, mO.Payload)

	t.Log("case: pass params, tenant admin defaults, error during select")
	ai.AccountID = "tid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.AccountFetchAllRolesCap: true, centrald.ManageNormalAccountsCap: true}
	nowDT := strfmt.DateTime(now)
	thenDT := strfmt.DateTime(then)
	params = ops.AuditLogListParams{
		HTTPRequest: requestWithAuthContext(ai),
		Action:      []string{"fetch"},
		Error:       swag.Bool(true),
		Name:        swag.String("n"),
		ObjectID:    swag.String("oID"),
		RecordNumGE: swag.Int32(1),
		RecordNumLE: swag.Int32(2),
		TimeStampGE: &thenDT,
		TimeStampLE: &nowDT,
	}
	where := " WHERE ObjectID = $1 AND " +
		"(TenantAccountID = $2 OR AccountID = $3) AND " +
		"RecordNum >= $4 AND RecordNum <= $5 AND " +
		"TimeStamp >= $6 AND TimeStamp <= $7 AND " +
		"ObjectName = $8 AND " +
		"(Action = $9) AND " +
		"Error"
	q = qBase + regexp.QuoteMeta(where) + order
	pg.Mock.ExpectQuery(q).WithArgs("oID", "tid1", "tid1", 1, 2, thenDT.String(), nowDT.String(), "n", "fetch").WillReturnError(errors.New("SV-error"))
	assert.NotPanics(func() { ret = lh.auditLogList(params) })
	assert.NoError(pg.Mock.ExpectationsWereMet())
	mD, ok = ret.(*ops.AuditLogListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorDbError.M+".*SV-error", *mD.Payload.Message)
	assert.Equal(1, tl.CountPattern("query error"))
	tl.Flush()

	t.Log("case: pass other params, case-independent, system admin defaults, error during row")
	ai.AccountID = "sid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.AccountFetchAllRolesCap: true, centrald.ManageSpecialAccountsCap: true}
	params = ops.AuditLogListParams{
		HTTPRequest:    requestWithAuthContext(ai),
		Classification: swag.String("Audit"),
		Action:         []string{"create", "DeLeTe"},
		Error:          swag.Bool(false),
		ObjectType:     swag.String("USER"),
		RecordNumGE:    swag.Int32(3),
		TimeStampGE:    &thenDT,
		UserID:         swag.String("uID"),
	}
	where = " WHERE TenantAccountID = $1 AND UserID = $2 AND " +
		"RecordNum >= $3 AND " +
		"TimeStamp >= $4 AND " +
		"Classification = $5 AND " +
		"ObjectType = $6 AND " +
		"(Action = $7 OR Action = $8) AND " +
		"NOT Error"
	q = qBase + regexp.QuoteMeta(where) + order
	rErr := sqlmock.NewRows(listColumns).
		AddRow(1, then, 0, "audit", "node", "oID1", "n1", "rID1", "tID1", "aID1", "a1", "uID1", "i1", "create", false, "m1").
		RowError(0, fmt.Errorf("Row-error"))
	pg.Mock.ExpectQuery(q).WithArgs(nilUUID, "uID", 3, thenDT.String(), "audit", "user", "create", "delete").WillReturnRows(rErr)
	assert.NotPanics(func() { ret = lh.auditLogList(params) })
	assert.NoError(pg.Mock.ExpectationsWereMet())
	mD, ok = ret.(*ops.AuditLogListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorDbError.M+".*Row-error", *mD.Payload.Message)
	assert.Equal(1, tl.CountPattern(" row error"))
	tl.Flush()

	t.Log("case: pass few params, account admin defaults, error during scan")
	ai.AccountID = "aid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.AccountFetchAllRolesCap: true}
	params = ops.AuditLogListParams{
		HTTPRequest: requestWithAuthContext(ai),
		RecordNumLE: swag.Int32(8),
		TimeStampLE: &thenDT,
		Count:       swag.Int32(5),
	}
	where = " WHERE AccountID = $1 AND " +
		"RecordNum <= $2 AND " +
		"TimeStamp <= $3"
	q = qBase + regexp.QuoteMeta(where+order+" LIMIT $4")
	rErr = sqlmock.NewRows([]string{"RecordNum"}).AddRow(2)
	pg.Mock.ExpectQuery(q).WithArgs("aid1", 8, thenDT.String(), 5).WillReturnRows(rErr)
	assert.NotPanics(func() { ret = lh.auditLogList(params) })
	assert.NoError(pg.Mock.ExpectationsWereMet())
	mD, ok = ret.(*ops.AuditLogListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorDbError.M+".*in Scan", *mD.Payload.Message)
	assert.Equal(1, tl.CountPattern(" scan error"))
	tl.Flush()

	t.Log("case: success, account admin, valid related query with some filters")
	params = ops.AuditLogListParams{
		HTTPRequest: requestWithAuthContext(ai),
		ObjectID:    swag.String("oID"),
		Related:     swag.Bool(true),
		RecordNumGE: swag.Int32(3),
		TimeStampGE: &thenDT,
	}
	qRelated := "WITH RECURSIVE related AS ( " +
		"SELECT * FROM AuditLog WHERE ObjectID = $1 AND AccountID = $2 UNION SELECT a.* FROM AuditLog a INNER JOIN related r ON r.RefObjectID = a.ObjectID " +
		") SELECT " + strings.Join(listColumns, ", ") + " FROM related WHERE " +
		"RecordNum >= $3 AND TimeStamp >= $4"
	rows = sqlmock.NewRows(listColumns).
		AddRow(1, then, 0, "audit", "node", "oID1", "n1", "rID1", "tID1", "aID1", "a1", "uID1", "i1", "create", false, "m1").
		AddRow(3, now, 0, "event", "pool", nilUUID, "n2", nilUUID, nilUUID, nilUUID, "a2", nilUUID, "i2", "fetch", true, "m2")
	q = regexp.QuoteMeta(qRelated) + order
	pg.Mock.ExpectQuery("^"+q).WithArgs("oID", "aid1", 3, thenDT.String()).WillReturnRows(rows)
	assert.NotPanics(func() { ret = lh.auditLogList(params) })
	assert.NoError(pg.Mock.ExpectationsWereMet())
	mO, ok = ret.(*ops.AuditLogListOK)
	assert.True(ok)
	assert.NotNil(mO)
	assert.Equal(rList, mO.Payload)

	t.Log("case: related, objectId missing")
	params = ops.AuditLogListParams{
		HTTPRequest: requestWithAuthContext(ai),
		Related:     swag.Bool(true),
	}
	assert.NotPanics(func() { ret = lh.auditLogList(params) })
	mD, ok = ret.(*ops.AuditLogListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mD.Payload.Code))
	assert.Regexp(": objectId is required", *mD.Payload.Message)
	tl.Flush()

	t.Log("case: success, account user defaults")
	ai.AccountID = "aid1"
	ai.TenantAccountID = "tid1"
	ai.UserID = "uid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.AccountFetchOwnRoleCap: true}
	params = ops.AuditLogListParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	where = " WHERE AccountID = $1 AND UserID = $2"
	rows = sqlmock.NewRows(listColumns).
		AddRow(1, then, 0, "audit", "node", "oID1", "n1", "rID1", "tID1", "aID1", "a1", "uID1", "i1", "create", false, "m1").
		AddRow(3, now, 0, "event", "pool", nilUUID, "n2", nilUUID, nilUUID, nilUUID, "a2", nilUUID, "i2", "fetch", true, "m2")
	q = qBase + regexp.QuoteMeta(where) + order
	pg.Mock.ExpectQuery(q).WithArgs("aid1", "uid1").WillReturnRows(rows)
	assert.NotPanics(func() { ret = lh.auditLogList(params) })
	assert.NoError(pg.Mock.ExpectationsWereMet())
	mO, ok = ret.(*ops.AuditLogListOK)
	assert.True(ok)
	assert.NotNil(mO)
	assert.Equal(rList, mO.Payload)

	t.Log("case: success, user only defaults")
	ai.AccountID = ""
	ai.TenantAccountID = ""
	ai.UserID = "uid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{}
	params = ops.AuditLogListParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	where = " WHERE UserID = $1"
	rows = sqlmock.NewRows(listColumns).
		AddRow(1, then, 0, "audit", "node", "oID1", "n1", "rID1", "tID1", "aID1", "a1", "uID1", "i1", "create", false, "m1").
		AddRow(3, now, 0, "event", "pool", nilUUID, "n2", nilUUID, nilUUID, nilUUID, "a2", nilUUID, "i2", "fetch", true, "m2")
	q = qBase + regexp.QuoteMeta(where) + order
	pg.Mock.ExpectQuery(q).WithArgs("uid1").WillReturnRows(rows)
	assert.NotPanics(func() { ret = lh.auditLogList(params) })
	assert.NoError(pg.Mock.ExpectationsWereMet())
	mO, ok = ret.(*ops.AuditLogListOK)
	assert.True(ok)
	assert.NotNil(mO)
	assert.Equal(rList, mO.Payload)

	t.Log("case: account user, invalid userID skips query")
	origDB := hc.db
	hc.db, hc.dbConnected = nil, false
	ai.RoleObj.Capabilities = map[string]bool{centrald.AccountFetchOwnRoleCap: true}
	params.UserID = swag.String("uid2")
	assert.NotPanics(func() { ret = lh.auditLogList(params) })
	mO, ok = ret.(*ops.AuditLogListOK)
	assert.True(ok)
	assert.NotNil(mO)
	assert.Empty(mO.Payload)

	t.Log("case: account user, valid tenant and invalid accountID skips query")
	params.AccountID = swag.String("aid2")
	params.TenantAccountID = swag.String(ai.TenantAccountID)
	params.UserID = swag.String("uid1")
	assert.NotPanics(func() { ret = lh.auditLogList(params) })
	mO, ok = ret.(*ops.AuditLogListOK)
	assert.True(ok)
	assert.NotNil(mO)
	assert.Empty(mO.Payload)
	hc.db, hc.dbConnected = origDB, true

	t.Log("case: account user, invalid tenantAccountID skips query")
	params.AccountID = swag.String(ai.AccountID)
	params.TenantAccountID = swag.String("tid9")
	params.UserID = nil
	assert.NotPanics(func() { ret = lh.auditLogList(params) })
	mO, ok = ret.(*ops.AuditLogListOK)
	assert.True(ok)
	assert.NotNil(mO)
	assert.Empty(mO.Payload)

	t.Log("case: wrong tenant admin skips query")
	ai.AccountID = "tid1"
	ai.TenantAccountID = ""
	ai.UserID = "uid1"
	params.AccountID = nil
	params.TenantAccountID = swag.String("tid9")
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.AccountFetchAllRolesCap: true, centrald.ManageNormalAccountsCap: true}
	assert.NotPanics(func() { ret = lh.auditLogList(params) })
	mO, ok = ret.(*ops.AuditLogListOK)
	assert.True(ok)
	assert.NotNil(mO)
	assert.Empty(mO.Payload)
	hc.db, hc.dbConnected = origDB, true

	t.Log("case: success, correct tenant")
	params = ops.AuditLogListParams{
		HTTPRequest:     requestWithAuthContext(ai),
		TenantAccountID: swag.String(ai.AccountID),
	}
	where = " WHERE TenantAccountID = $1"
	rows = sqlmock.NewRows(listColumns).
		AddRow(1, then, 0, "audit", "node", "oID1", "n1", "rID1", "tID1", "aID1", "a1", "uID1", "i1", "create", false, "m1").
		AddRow(3, now, 0, "event", "pool", nilUUID, "n2", nilUUID, nilUUID, nilUUID, "a2", nilUUID, "i2", "fetch", true, "m2")
	q = qBase + regexp.QuoteMeta(where) + order
	pg.Mock.ExpectQuery(q).WithArgs("tid1").WillReturnRows(rows)
	assert.NotPanics(func() { ret = lh.auditLogList(params) })
	assert.NoError(pg.Mock.ExpectationsWereMet())
	mO, ok = ret.(*ops.AuditLogListOK)
	assert.True(ok)
	assert.NotNil(mO)
	assert.Equal(rList, mO.Payload)

	t.Log("case: success, correct tenant query only for subordinate account")
	params = ops.AuditLogListParams{
		HTTPRequest: requestWithAuthContext(ai),
		AccountID:   swag.String("aID1"),
	}
	where = " WHERE TenantAccountID = $1 AND AccountID = $2"
	rows = sqlmock.NewRows(listColumns).
		AddRow(1, then, 0, "audit", "node", "oID1", "n1", "rID1", "tID1", "aID1", "a1", "uID1", "i1", "create", false, "m1").
		AddRow(3, now, 0, "event", "pool", nilUUID, "n2", nilUUID, nilUUID, nilUUID, "a2", nilUUID, "i2", "fetch", true, "m2")
	q = qBase + regexp.QuoteMeta(where) + order
	pg.Mock.ExpectQuery(q).WithArgs("tid1", "aID1").WillReturnRows(rows)
	assert.NotPanics(func() { ret = lh.auditLogList(params) })
	assert.NoError(pg.Mock.ExpectationsWereMet())
	mO, ok = ret.(*ops.AuditLogListOK)
	assert.True(ok)
	assert.NotNil(mO)
	assert.Equal(rList, mO.Payload)

	t.Log("case: success, correct tenant query only for tenant account")
	params = ops.AuditLogListParams{
		HTTPRequest: requestWithAuthContext(ai),
		AccountID:   swag.String(ai.AccountID),
	}
	where = " WHERE AccountID = $1"
	rows = sqlmock.NewRows(listColumns).
		AddRow(1, then, 0, "audit", "node", "oID1", "n1", "rID1", "tID1", "aID1", "a1", "uID1", "i1", "create", false, "m1").
		AddRow(3, now, 0, "event", "pool", nilUUID, "n2", nilUUID, nilUUID, nilUUID, "a2", nilUUID, "i2", "fetch", true, "m2")
	q = qBase + regexp.QuoteMeta(where) + order
	pg.Mock.ExpectQuery(q).WithArgs("tid1").WillReturnRows(rows)
	assert.NotPanics(func() { ret = lh.auditLogList(params) })
	assert.NoError(pg.Mock.ExpectationsWereMet())
	mO, ok = ret.(*ops.AuditLogListOK)
	assert.True(ok)
	assert.NotNil(mO)
	assert.Equal(rList, mO.Payload)

	t.Log("case: system admin specifies tenantAccountID skips query")
	ai.AccountID = "sid1"
	ai.TenantAccountID = ""
	ai.UserID = "uid1"
	params.AccountID = nil
	params.TenantAccountID = swag.String("tid1")
	ai.RoleObj.Capabilities = map[string]bool{centrald.AccountFetchAllRolesCap: true, centrald.ManageSpecialAccountsCap: true}
	assert.NotPanics(func() { ret = lh.auditLogList(params) })
	mO, ok = ret.(*ops.AuditLogListOK)
	assert.True(ok)
	assert.NotNil(mO)
	assert.Empty(mO.Payload)

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = lh.auditLogList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AuditLogListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
}

func requestWithAuthContext(ai *auth.Info) *http.Request {
	req := &http.Request{}
	ctx := context.WithValue(req.Context(), auth.InfoKey{}, ai)
	return req.WithContext(ctx)
}

type fakeInserter struct {
	cnt    int
	ctx    context.Context
	m      *LogRecord
	retSeq int32
	retErr error
}

func (f *fakeInserter) insert(ctx context.Context, m *LogRecord) (int32, error) {
	f.cnt++
	f.ctx, f.m = ctx, m
	if f.retErr != nil {
		return 0, f.retErr
	}
	f.retSeq++
	return f.retSeq, f.retErr
}
