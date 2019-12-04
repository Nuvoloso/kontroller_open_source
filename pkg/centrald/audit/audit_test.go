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
	"database/sql"
	"fmt"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	fhk "github.com/Nuvoloso/kontroller/pkg/housekeeping/fake"
	fpg "github.com/Nuvoloso/kontroller/pkg/pgdb/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	fw "github.com/Nuvoloso/kontroller/pkg/util/fake"
	"github.com/stretchr/testify/assert"
)

func TestInit(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			API: &operations.NuvolosoAPI{},
			Log: tl.Logger(),
			Server: &restapi.Server{
				TLSCertificate:    "server.crt",
				TLSCertificateKey: "server.key",
				TLSCACertificate:  "ca.crt",
			},
		},
	}

	args := &Args{}
	fts := &fhk.TaskScheduler{}
	app.TaskScheduler = fts

	// This actually calls centrald.AppRegisterComponent but we can't intercept that
	c := Register(args)
	assert.NotNil(c)
	assert.Equal(*args, c.Args)

	// Init with defaults
	c.Init(app)
	assert.Equal(app, c.App)
	assert.Equal(c.Log, app.Log)
	assert.NotNil(c.pgDB)
	assert.Equal(c, app.AuditLog)
	assert.Equal(RetryIntervalDefault, c.RetryInterval)
	assert.Equal(PingIntervalDefault, c.PingInterval)
	assert.NotNil(c.worker)
	assert.Equal(c, c.handler.c)
	assert.NotNil(app.API.AuditLogAuditLogCreateHandler)
	assert.NotNil(app.API.AuditLogAuditLogListHandler)
	assert.Equal("server.crt", c.TLSCertificate)
	assert.Equal("server.key", c.TLSCertificateKey)
	assert.Equal("ca.crt", c.TLSCACertificate)
	assert.Empty(c.SSLServerName)

	dba := c.getDBArgs()
	assert.False(dba.UseSSL)
	assert.Equal("ca.crt", dba.TLSCACertificate)

	// Init with values set
	c.PingInterval = 2 * PingIntervalDefault
	c.RetryInterval = 2 * RetryIntervalDefault
	c.UseSSL = true
	c.SSLServerName = "set"
	c.Init(app)
	assert.Equal(2*RetryIntervalDefault, c.RetryInterval)
	assert.Equal(2*PingIntervalDefault, c.PingInterval)

	dba = c.getDBArgs()
	assert.True(dba.UseSSL)
	assert.Equal("server.crt", dba.TLSCertificate)
	assert.Equal("server.key", dba.TLSCertificateKey)
	assert.Equal("ca.crt", dba.TLSCACertificate)
	assert.Equal("set", dba.TLSServerName)

	// Notify() should not panic before Start()
	c.Notify()
}

func TestComponentMethods(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			API:    &operations.NuvolosoAPI{},
			Log:    tl.Logger(),
			Server: &restapi.Server{},
		},
	}
	fts := &fhk.TaskScheduler{}
	app.TaskScheduler = fts

	// change the interval for the UT
	ri := time.Millisecond * 10
	pi := time.Millisecond * 20
	c := &Component{
		Args: Args{RetryInterval: ri, PingInterval: pi},
	}
	assert.Nil(c.pgDB)
	c.Init(app)
	assert.NotNil(c.pgDB)
	assert.Equal(c.RetryInterval, c.worker.GetSleepInterval())

	api := &operations.NuvolosoAPI{}
	c.RegisterHandlers(api)
	assert.NotNil(api.AuditLogAuditLogListHandler)

	// replace the worker
	tw := &fw.Worker{}
	c.worker = tw

	c.Start()
	assert.Equal(1, tw.CntStart)
	assert.Equal(0, tl.CountPattern("Audit Log is ready"))

	c.Stop()
	assert.Equal(1, tw.CntStop)
}

func TestComponentBuzz(t *testing.T) {
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
	assert.Equal(c.RetryInterval, c.worker.GetSleepInterval())
	assert.NotNil(c.pgDB)
	pg := &fpg.DB{}
	c.pgDB = pg

	// c.db fail
	var err error
	assert.Nil(c.db)
	pg.RetODErr = fmt.Errorf("OpenDB-error")
	err = c.Buzz(ctx)
	assert.False(c.ready)
	assert.Nil(c.db)
	assert.Error(err)
	assert.Regexp("OpenDB-error", err)
	assert.Equal(c.RetryInterval, c.worker.GetSleepInterval())
	tl.Flush()

	// c.db succeed, ping fails and forces db destruction including flush of cached statements
	pg.RetODErr = nil
	pg.RetPCErr = fmt.Errorf("PingContext-error")
	c.stmtCache = make(map[string]*sql.Stmt)
	err = c.Buzz(ctx)
	assert.Nil(c.db)
	assert.False(c.dbConnected)
	assert.False(c.ready)
	assert.NotNil(pg.InPCDb) // previous c.db value
	assert.Regexp("PingContext-error", err)
	assert.Nil(c.db)        // closed on failure
	assert.Nil(c.stmtCache) // destroyed
	assert.Equal(c.RetryInterval, c.worker.GetSleepInterval())
	assert.Regexp(centrald.ErrorDbError.M+".*Audit Log is not ready", c.Ready())
	assert.Equal(1, tl.CountPattern("Audit Log is not ready"))

	// ping failure forces invocation of db.Close()
	// explicitly reopen the db so we can grab the Mock again
	c.db, err = pg.OpenDB(c.getDBArgs())
	assert.NoError(err)
	c.dbConnected = true
	pg.Mock.ExpectClose()
	err = c.Buzz(ctx)
	assert.Nil(c.db)
	assert.False(c.dbConnected)
	assert.False(c.ready)
	assert.NotNil(pg.InPCDb) // previous c.db value
	assert.Regexp("PingContext-error", err)
	assert.Nil(c.db) // closed on failure
	assert.NoError(pg.Mock.ExpectationsWereMet())

	// ping succeeds, fetchTableVersions called
	// explicitly reopen the db so we can grab the Mock again
	c.db, err = pg.OpenDB(c.getDBArgs())
	assert.NoError(err)
	c.dbConnected = true
	pg.RetPCErr = nil
	pg.Mock.ExpectQuery("^SELECT TableName, Version FROM SchemaVersion").WillReturnError(fmt.Errorf("SV-error"))
	err = c.Buzz(ctx)
	assert.NotNil(c.db) // reopened
	assert.True(c.dbConnected)
	assert.False(c.tableVersionsLoaded)
	assert.False(c.ready)
	assert.Regexp("SV-error", err)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.Equal(c.PingInterval, c.worker.GetSleepInterval())
	assert.Equal(c.purgeTaskID, "")

	// all pre-requisites succeed so ready
	tl.Flush()
	c.dbConnected = true
	c.tableVersionsLoaded = true
	c.ready = false
	err = c.Buzz(ctx)
	assert.True(c.ready)
	assert.Nil(err)
	assert.Equal(1, tl.CountPattern("AuditLog is ready"))
	assert.NoError(c.Ready())

	// audit log records purge task failure
	fts.RetRtID = "task_id"
	fts.RetRtErr = fmt.Errorf("task failed to run")
	c.lastPurgeTaskTime = time.Now().Add(-time.Hour)
	c.AuditPurgeTaskInterval = time.Duration(1 * time.Second)
	err = c.Buzz(ctx)
	assert.Nil(err)
	assert.Equal(fts.RetRtID, c.purgeTaskID)

	// audit log records purge task success
	fts.RetRtErr = nil
	err = c.Buzz(ctx)
	assert.Nil(err)
	assert.Equal("", c.purgeTaskID)
}

func TestPing(t *testing.T) {
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
	assert.NotNil(c.pgDB)
	pg := &fpg.DB{}
	c.pgDB = pg
	var err error
	c.db, err = pg.OpenDB(c.getDBArgs())
	assert.NoError(err)

	// ping fails initially, no message
	pg.RetODErr = nil
	pg.RetPCErr = fmt.Errorf("PingContext-error")
	assert.False(c.dbConnected)
	err = c.ping(ctx)
	assert.Error(err)
	assert.Regexp("PingContext-error", err)
	assert.False(c.dbConnected)
	assert.Equal(c.db, pg.InPCDb)
	assert.Equal(0, tl.CountPattern("Lost connection to AuditLog database"))

	// repeat initial ping failure, no message
	pg.InPCDb = nil
	err = c.ping(ctx)
	assert.Error(err)
	assert.Regexp("PingContext-error", err)
	assert.Equal(c.db, pg.InPCDb)
	assert.False(c.dbConnected)
	assert.Equal(0, tl.CountPattern("Lost connection to AuditLog database"))
	assert.Equal(0, tl.CountPattern("Connected to AuditLog database"))

	// ping succeeds, connection message
	pg.RetPCErr = nil
	pg.InPCDb = nil
	err = c.ping(ctx)
	assert.NoError(err)
	assert.Equal(c.db, pg.InPCDb)
	assert.True(c.dbConnected)
	assert.Equal(1, tl.CountPattern("Connected to AuditLog database"))

	// repeat ping succeeds, connection message not repeated
	tl.Flush()
	pg.InPCDb = nil
	err = c.ping(ctx)
	assert.NoError(err)
	assert.Equal(c.db, pg.InPCDb)
	assert.True(c.dbConnected)
	assert.Equal(0, tl.CountPattern("Connected to AuditLog database"))

	// subsequent ping fails, disconnect message, flags unset
	tl.Flush()
	pg.RetPCErr = fmt.Errorf("PingContext-error")
	pg.InPCDb = nil
	c.tableVersionsLoaded = true
	err = c.ping(ctx)
	assert.Error(err)
	assert.Regexp("PingContext-error", err)
	assert.Equal(c.db, pg.InPCDb)
	assert.False(c.dbConnected)
	assert.False(c.tableVersionsLoaded)
	assert.Equal(1, tl.CountPattern("Lost connection to AuditLog database"))

	// subsequent ping fails, disconnect message not repeated
	tl.Flush()
	pg.InPCDb = nil
	err = c.ping(ctx)
	assert.Error(err)
	assert.Regexp("PingContext-error", err)
	assert.Equal(c.db, pg.InPCDb)
	assert.False(c.dbConnected)
	assert.Equal(0, tl.CountPattern("Lost connection to AuditLog database"))
}

func TestFetchTableVersions(t *testing.T) {
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
	assert.NotNil(c.pgDB)
	pg := &fpg.DB{}
	c.pgDB = pg
	var err error
	c.db, err = pg.OpenDB(c.getDBArgs())
	assert.NoError(err)
	c.dbConnected = true

	// check tableVersion before map is set
	assert.Nil(c.tableVersions)
	assert.Equal(0, c.TableVersion("AuditLog"))

	// error during select
	pg.Mock.ExpectQuery("^SELECT TableName, Version FROM SchemaVersion").WillReturnError(fmt.Errorf("SV-error"))
	err = c.fetchTableVersions(ctx)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.Error(err)
	assert.Regexp("SV-error", err)
	assert.False(c.tableVersionsLoaded)

	// select ok, error during Row
	rows := sqlmock.NewRows([]string{"TableName", "Version"}).
		AddRow("AuditLog", 1).
		RowError(0, fmt.Errorf("Row-error"))
	pg.Mock.ExpectQuery("^SELECT TableName, Version FROM SchemaVersion").WillReturnRows(rows)
	err = c.fetchTableVersions(ctx)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.Error(err)
	assert.Regexp("Row-error", err)
	assert.False(c.tableVersionsLoaded)

	// error during scan
	rows = sqlmock.NewRows([]string{"TableName"}).
		AddRow("AuditLog")
	pg.Mock.ExpectQuery("^SELECT TableName, Version FROM SchemaVersion").WillReturnRows(rows)
	err = c.fetchTableVersions(ctx)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.Error(err)
	assert.Regexp("Scan", err)
	assert.False(c.tableVersionsLoaded)

	// no error
	rows = sqlmock.NewRows([]string{"TableName", "Version"}).
		AddRow("AuditLog", 1).
		AddRow("Log2", 2).
		AddRow("Log3", 3).
		AddRow("Log4", 4)
	pg.Mock.ExpectQuery("^SELECT TableName, Version FROM SchemaVersion").WillReturnRows(rows)
	assert.Nil(c.tableVersions)
	err = c.fetchTableVersions(ctx)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.NoError(err)
	assert.True(c.tableVersionsLoaded)
	assert.NotNil(c.tableVersions)
	assert.Equal(1, c.TableVersion("AuditLog"))
	assert.Equal(2, c.TableVersion("Log2"))
	assert.Equal(3, c.TableVersion("Log3"))
	assert.Equal(4, c.TableVersion("Log4"))
	assert.Equal(0, c.TableVersion("Foo"))
}

func TestStmtCache(t *testing.T) {
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
	assert.NotNil(c.pgDB)
	pg := &fpg.DB{}
	c.pgDB = pg
	var err error
	c.db, err = pg.OpenDB(c.getDBArgs())
	assert.NoError(err)
	c.dbConnected = true

	pg.Mock.ExpectPrepare("junk").WillBeClosed()

	stmt, err := c.SafePrepare(ctx, "SELECT junk()")
	assert.NoError(err)

	assert.Empty(c.stmtCache)
	c.StmtCacheSet("junk", stmt)
	assert.NotEmpty(c.stmtCache)

	assert.Equal(stmt, c.StmtCacheGet("junk"))
	assert.Nil(c.StmtCacheGet("foo"))

	c.closeDB()
	assert.False(c.dbConnected)
	assert.Nil(c.db)
	assert.Nil(c.stmtCache)
	assert.Nil(c.StmtCacheGet("junk"))
	assert.NoError(pg.Mock.ExpectationsWereMet())

	stmt, err = c.SafePrepare(ctx, "SELECT junk()")
	assert.Error(err)
	assert.Regexp(centrald.ErrorDbError.M+".*database not connected", err)
}

func TestNormalizedID(t *testing.T) {
	assert := assert.New(t)

	assert.Equal("00000000-0000-0000-0000-000000000000", nilUUID)
	assert.Equal(nilUUID, idOrZeroUUID(""))
	assert.Equal("anythingElse", idOrZeroUUID("anythingElse"))
}
