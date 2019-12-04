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


package metrics

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	fmm "github.com/Nuvoloso/kontroller/pkg/metricmover/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	fpg "github.com/Nuvoloso/kontroller/pkg/pgdb/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	fw "github.com/Nuvoloso/kontroller/pkg/util/fake"
	flags "github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestPoolMetricsInit(t *testing.T) {
	assert := assert.New(t)

	// check that arguments get set to expected defaults
	args := Args{}
	parser := flags.NewParser(&args, flags.Default&^flags.PrintErrors)
	_, err := parser.ParseArgs([]string{})
	assert.NoError(err)
	assert.Equal("localhost", args.Host)
	assert.Equal(int16(5432), args.Port)
	assert.Equal("postgres", args.User)
	assert.Equal("nuvo_metrics", args.Database)
	assert.Equal(1, args.DebugLevel)
	assert.EqualValues(1*time.Second, args.RetryInterval)              // flag default
	assert.EqualValues(10*time.Second, args.PingInterval)              // flag default
	assert.EqualValues(time.Hour, args.PoolMetricPeriod)               // flag default
	assert.EqualValues(time.Hour, args.PoolMetricTruncate)             // flag default
	assert.EqualValues(100, args.PoolMaxBuffered)                      // flag default
	assert.False(args.PoolMetricSuppressStartup)                       // flag default
	assert.EqualValues(3, args.StorageIOBurstTolerance)                // flag default
	assert.EqualValues(1000, args.StorageIOMaxBuffered)                // flag default
	assert.EqualValues(float64(0.1), args.StorageIOMaxResponseTimePct) // flag default
	assert.EqualValues(time.Hour, args.StorageMetadataMetricPeriod)    // flag default
	assert.EqualValues(time.Hour, args.StorageMetadataMetricTruncate)  // flag default
	assert.EqualValues(500, args.StorageMetadataMaxBuffered)           // flag default
	assert.False(args.StorageMetadataMetricSuppressStartup)            // flag default
	assert.EqualValues(time.Hour, args.VolumeMetadataMetricPeriod)     // flag default
	assert.EqualValues(time.Hour, args.VolumeMetadataMetricTruncate)   // flag default
	assert.EqualValues(1000, args.VolumeMetadataMaxBuffered)           // flag default
	assert.False(args.VolumeMetadataMetricSuppressStartup)             // flag default
	assert.EqualValues(3, args.VolumeIOBurstTolerance)                 // flag default
	assert.EqualValues(2000, args.VolumeIOMaxBuffered)                 // flag default
	assert.EqualValues(float64(0.1), args.VolumeIOMaxResponseTimePct)  // flag default
	assert.EqualValues(time.Hour, args.SPAMetricPeriod)                // flag default
	assert.EqualValues(time.Hour, args.SPAMetricTruncate)              // flag default
	assert.EqualValues(100, args.SPAMaxBuffered)                       // flag default

	// This actually calls centrald.AppRegisterComponent but we can't intercept that
	c := Register(&args)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log: tl.Logger(),
			Server: &restapi.Server{
				TLSCertificate:    "server.crt",
				TLSCertificateKey: "server.key",
				TLSCACertificate:  "ca.crt",
			},
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	c.Args = args
	c.PoolMetricTruncate = 2 * c.PoolMetricPeriod
	c.StorageMetadataMetricTruncate = 2 * c.StorageMetadataMetricPeriod
	c.VolumeMetadataMetricTruncate = 2 * c.VolumeMetadataMetricPeriod
	c.SPAMetricTruncate = 2 * c.SPAMetricPeriod
	c.SSLServerName = "set"
	c.UseSSL = true
	c.Init(app)
	assert.EqualValues(c.PoolMetricPeriod, c.PoolMetricTruncate)                       // sanity check applied
	assert.EqualValues(c.StorageMetadataMetricPeriod, c.StorageMetadataMetricTruncate) // sanity check applied
	assert.EqualValues(c.VolumeMetadataMetricPeriod, c.VolumeMetadataMetricTruncate)   // sanity check applied
	assert.EqualValues(c.SPAMetricPeriod, c.SPAMetricTruncate)                         // sanity check applied
	assert.Exactly(c, c.ph.c)
	assert.Exactly(c, c.vmh.c)
	assert.Exactly(c, c.spah.c)
	assert.NotNil(c.worker)
	assert.Equal("server.crt", c.TLSCertificate)
	assert.Equal("server.key", c.TLSCertificateKey)
	assert.Equal("ca.crt", c.TLSCACertificate)

	// Notify() should not panic before Start()
	c.Notify()

	dba := c.getDBArgs()
	assert.Equal("localhost", dba.Host)
	assert.Equal(int16(5432), dba.Port)
	assert.Equal("postgres", dba.User)
	assert.Equal("nuvo_metrics", dba.Database)
	assert.Equal(1, dba.DebugLevel)
	assert.True(dba.UseSSL)
	assert.Equal("server.crt", dba.TLSCertificate)
	assert.Equal("server.key", dba.TLSCertificateKey)
	assert.Equal("ca.crt", dba.TLSCACertificate)
	assert.Equal("set", dba.TLSServerName)

	// check defaults get set
	c = &Component{}
	c.Init(app)
	assert.Equal(RetryIntervalDefault, c.RetryInterval)
	assert.Equal(PingIntervalDefault, c.PingInterval)
	assert.Equal(PoolPeriodDefault, c.PoolMetricTruncate)
	assert.Equal(0, c.PoolMaxBuffered) // zero is valid
	assert.EqualValues(3, args.StorageIOBurstTolerance)
	assert.Equal(0, c.StorageIOMaxBuffered) // zero is valid
	assert.Equal(StorageIOMaxResponseTimePctDefault, c.StorageIOMaxResponseTimePct)
	assert.Equal(StorageMetadataPeriodDefault, c.StorageMetadataMetricTruncate)
	assert.Equal(0, c.StorageMetadataMaxBuffered) // zero is valid
	assert.Equal(VolumeMetadataPeriodDefault, c.VolumeMetadataMetricTruncate)
	assert.Equal(0, c.VolumeMetadataMaxBuffered) // zero is valid
	assert.EqualValues(3, args.VolumeIOBurstTolerance)
	assert.Equal(0, c.VolumeIOMaxBuffered) // zero is valid
	assert.Equal(VolumeIOMaxResponseTimePctDefault, c.VolumeIOMaxResponseTimePct)
	assert.Equal(SPAPeriodDefault, c.SPAMetricTruncate)
	assert.Equal(0, c.SPAMaxBuffered) // zero is valid

	dba = c.getDBArgs()
	assert.False(dba.UseSSL)

	c = &Component{}
	c.PoolMaxBuffered = -1
	c.StorageMetadataMaxBuffered = -1
	c.VolumeMetadataMaxBuffered = -1
	c.StorageIOMaxBuffered = -1
	c.VolumeIOMaxBuffered = -1
	c.SPAMaxBuffered = -1
	c.Init(app)
	assert.Equal(PoolMaxBufferedDefault, c.PoolMaxBuffered)
	assert.EqualValues(StorageIOMaxBufferedDefault, args.StorageIOMaxBuffered)
	assert.Equal(StorageMetadataMaxBufferedDefault, c.StorageMetadataMaxBuffered)
	assert.Equal(VolumeMetadataMaxBufferedDefault, c.VolumeMetadataMaxBuffered)
	assert.Equal(VolumeIOMaxBufferedDefault, c.VolumeIOMaxBuffered)
	assert.Equal(SPAMaxBufferedDefault, c.SPAMaxBuffered)
}

func TestComponentMethods(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log:    tl.Logger(),
			Server: &restapi.Server{},
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM

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

	// replace the worker
	tw := &fw.Worker{}
	c.worker = tw

	c.Start() // invokes run/runBody asynchronously but won't do anything if c.fatalErr
	assert.NotNil(c.oCrud)
	assert.NotNil(c.ph.ctx)
	assert.NotNil(c.vmh.ctx)
	assert.Equal(1, tw.CntStart)
	assert.False(c.consumerRegistered)
	assert.Nil(fMM.InRC) // consumer not started yet
	assert.Equal(0, tl.CountPattern("Ready to receive metric data"))
	assert.False(c.vIOP.worker.Started())
	assert.False(c.vIOW.worker.Started())

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
			Log:    tl.Logger(),
			Server: &restapi.Server{},
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	c := &Component{}
	c.Init(app)
	assert.Equal(c.RetryInterval, c.worker.GetSleepInterval())
	fc := &fake.Client{}
	c.oCrud = fc
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

	// pretend tableVersions loaded => updateServicePlanData called
	c.tableVersionsLoaded = true
	assert.False(c.servicePlanDataUpdated)
	assert.False(c.storageTypeDataUpdated)
	fc.RetLsSvPErr = fmt.Errorf("service-plan-list-error")
	err = c.Buzz(ctx)
	assert.Regexp("service-plan-list-error", err)
	assert.False(c.servicePlanDataUpdated)
	assert.False(c.storageTypeDataUpdated)
	assert.False(c.consumerRegistered)
	assert.Equal(0, tl.CountPattern("Ready to receive metric data"))
	assert.False(c.vIOP.worker.Started())
	assert.False(c.vIOW.worker.Started())

	// pretend tableVersions loaded, updateServicePlanData done => updateStorageTypeData called
	c.tableVersionsLoaded = true
	c.servicePlanDataUpdated = true
	assert.False(c.storageTypeDataUpdated)
	err = c.Buzz(ctx)
	assert.Regexp("unsupported StorageTypeData version 0", err)
	assert.False(c.storageTypeDataUpdated)
	assert.False(c.consumerRegistered)
	assert.Equal(0, tl.CountPattern("Ready to receive metric data"))
	assert.False(c.vIOP.worker.Started())
	assert.False(c.vIOW.worker.Started())

	// all pre-requisites succeed so ready, lastErr cleared, capacity limiters called
	tl.Flush()
	c.dbConnected = true
	c.tableVersionsLoaded = true
	c.servicePlanDataUpdated = true
	c.storageTypeDataUpdated = true
	c.ready = false
	c.numSIOCapacityExceeded = 1
	c.numVIOCapacityExceeded = 1
	err = fmt.Errorf("some-error")
	err = c.Buzz(ctx)
	assert.True(c.ready)
	assert.Nil(err)
	assert.True(c.consumerRegistered)
	assert.Equal(c, fMM.InRC)
	assert.Equal(1, tl.CountPattern("Ready to receive metric data"))
	assert.True(c.vIOP.worker.Started())
	assert.True(c.vIOW.worker.Started())
	assert.Equal(0, c.numSIOCapacityExceeded)
	assert.Equal(0, c.numVIOCapacityExceeded)
}

func TestPing(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ctx := context.Background()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log:    tl.Logger(),
			Server: &restapi.Server{},
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
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
	assert.Equal(0, tl.CountPattern("Lost connection to metrics database"))

	// repeat initial ping failure, no message
	pg.InPCDb = nil
	err = c.ping(ctx)
	assert.Error(err)
	assert.Regexp("PingContext-error", err)
	assert.Equal(c.db, pg.InPCDb)
	assert.False(c.dbConnected)
	assert.Equal(0, tl.CountPattern("Lost connection to metrics database"))
	assert.Equal(0, tl.CountPattern("Connected to metrics database"))

	// ping succeeds, connection message
	pg.RetPCErr = nil
	pg.InPCDb = nil
	err = c.ping(ctx)
	assert.NoError(err)
	assert.Equal(c.db, pg.InPCDb)
	assert.True(c.dbConnected)
	assert.Equal(1, tl.CountPattern("Connected to metrics database"))

	// repeat ping succeeds, connection message not repeated
	tl.Flush()
	pg.InPCDb = nil
	err = c.ping(ctx)
	assert.NoError(err)
	assert.Equal(c.db, pg.InPCDb)
	assert.True(c.dbConnected)
	assert.Equal(0, tl.CountPattern("Connected to metrics database"))

	// subsequent ping fails, disconnect message, flags unset
	tl.Flush()
	pg.RetPCErr = fmt.Errorf("PingContext-error")
	pg.InPCDb = nil
	c.tableVersionsLoaded = true
	c.servicePlanDataUpdated = true
	err = c.ping(ctx)
	assert.Error(err)
	assert.Regexp("PingContext-error", err)
	assert.Equal(c.db, pg.InPCDb)
	assert.False(c.dbConnected)
	assert.False(c.tableVersionsLoaded)
	assert.False(c.servicePlanDataUpdated)
	assert.Equal(1, tl.CountPattern("Lost connection to metrics database"))

	// subsequent ping fails, disconnect message not repeated
	tl.Flush()
	pg.InPCDb = nil
	err = c.ping(ctx)
	assert.Error(err)
	assert.Regexp("PingContext-error", err)
	assert.Equal(c.db, pg.InPCDb)
	assert.False(c.dbConnected)
	assert.Equal(0, tl.CountPattern("Lost connection to metrics database"))
}

func TestFetchTableVersions(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ctx := context.Background()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log:    tl.Logger(),
			Server: &restapi.Server{},
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
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
	assert.Equal(0, c.TableVersion("VolumeMetadata"))

	// error during select
	pg.Mock.ExpectQuery("^SELECT TableName, Version FROM SchemaVersion").WillReturnError(fmt.Errorf("SV-error"))
	err = c.fetchTableVersions(ctx)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.Error(err)
	assert.Regexp("SV-error", err)
	assert.False(c.tableVersionsLoaded)

	// select ok, error during Row
	rows := sqlmock.NewRows([]string{"TableName", "Version"}).
		AddRow("ServicePlanData", 1).
		RowError(0, fmt.Errorf("Row-error"))
	pg.Mock.ExpectQuery("^SELECT TableName, Version FROM SchemaVersion").WillReturnRows(rows)
	err = c.fetchTableVersions(ctx)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.Error(err)
	assert.Regexp("Row-error", err)
	assert.False(c.tableVersionsLoaded)

	// error during scan
	rows = sqlmock.NewRows([]string{"TableName"}).
		AddRow("ServicePlanData")
	pg.Mock.ExpectQuery("^SELECT TableName, Version FROM SchemaVersion").WillReturnRows(rows)
	err = c.fetchTableVersions(ctx)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.Error(err)
	assert.Regexp("Scan", err)
	assert.False(c.tableVersionsLoaded)

	// no error
	rows = sqlmock.NewRows([]string{"TableName", "Version"}).
		AddRow("ServicePlanData", 1).
		AddRow("PoolMetrics", 2).
		AddRow("VolumeMetadata", 3).
		AddRow("VolumeMetrics", 4)
	pg.Mock.ExpectQuery("^SELECT TableName, Version FROM SchemaVersion").WillReturnRows(rows)
	assert.Nil(c.tableVersions)
	err = c.fetchTableVersions(ctx)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.NoError(err)
	assert.True(c.tableVersionsLoaded)
	assert.NotNil(c.tableVersions)
	assert.Equal(1, c.TableVersion("ServicePlanData"))
	assert.Equal(2, c.TableVersion("PoolMetrics"))
	assert.Equal(3, c.TableVersion("VolumeMetadata"))
	assert.Equal(4, c.TableVersion("VolumeMetrics"))
	assert.Equal(0, c.TableVersion("Foo"))
}

func TestStmtCache(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ctx := context.Background()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log:    tl.Logger(),
			Server: &restapi.Server{},
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
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

	var stmt *sql.Stmt
	stmt, err = c.SafePrepare(ctx, "SELECT junk()")
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
	assert.Regexp("database not connected", err)
}

func TestNormalizedID(t *testing.T) {
	assert := assert.New(t)

	assert.Equal("00000000-0000-0000-0000-000000000000", nilUUID)
	assert.Equal(nilUUID, idOrZeroUUID(""))
	assert.Equal("anythingElse", idOrZeroUUID("anythingElse"))
}
