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
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/pgdb"
	"github.com/Nuvoloso/kontroller/pkg/util"
	logging "github.com/op/go-logging"
	"github.com/satori/go.uuid"
)

// Args contains settable parameters for this component
type Args struct {
	Host       string `long:"host" description:"The hostname of the metrics database service" default:"localhost"`
	Port       int16  `long:"port" description:"The port number of the metrics database service" default:"5432"`
	User       string `long:"user" description:"The metrics user account" default:"postgres"`
	Database   string `long:"database" description:"The name of the database" default:"nuvo_metrics"`
	DebugLevel int    `long:"debug-level" description:"Controls debugging" default:"1"`

	UseSSL        bool   `long:"ssl" description:"Use SSL to communicate with the metrics database"`
	SSLServerName string `long:"ssl-server-name" description:"The actual server name of the metrics database SSL certificate"`

	// copied from service flags on startup
	TLSCertificate    string `no-flag:"1"`
	TLSCertificateKey string `no-flag:"1"`
	TLSCACertificate  string `no-flag:"1"`

	RetryInterval time.Duration `long:"retry-interval" description:"Database connection retry interval" default:"1s"`
	PingInterval  time.Duration `long:"ping-interval" description:"Database periodic ping interval" default:"10s"`

	PoolMetricPeriod          time.Duration `long:"pool-metric-period" description:"Periodic pool metric period" default:"1h"`
	PoolMetricTruncate        time.Duration `long:"pool-metric-period-truncate" description:"The value of the periodic pool metric timer is truncated to this duration" default:"1h"`
	PoolMetricSuppressStartup bool          `long:"pool-metric-suppress-startup-metrics" description:"Do not insert (unaligned) pool metrics on startup"`
	PoolMaxBuffered           int           `long:"pool-metric-max-buffered" description:"The maximum number of buffered pool metric records when the database is not available" default:"100"`

	StorageMetadataMetricPeriod          time.Duration `long:"storage-metadata-metric-period" description:"Periodic storage metadata metric period" default:"1h"`
	StorageMetadataMetricTruncate        time.Duration `long:"storage-metadata-metric-period-truncate" description:"The value of the periodic storage metadata metric timer is truncated to this duration" default:"1h"`
	StorageMetadataMetricSuppressStartup bool          `long:"storage-metadata-metric-suppress-startup-metrics" description:"Do not insert (unaligned) storage metadata metrics on startup"`
	StorageMetadataMaxBuffered           int           `long:"storage-metadata-metric-max-buffered" description:"The maximum number of buffered storage metadata records when the database is not available" default:"500"`

	StorageIOBurstTolerance     int     `long:"storage-io-metric-burst-tolerance" description:"The number of times the storage I/O metric buffer limits may be continuously exceeded (sampled at the database connection retry interval)" default:"3"`
	StorageIOMaxBuffered        int     `long:"storage-io-metric-max-buffered" description:"The maximum number of buffered storage I/O metric records when the database is not available" default:"1000"`
	StorageIOMaxResponseTimePct float64 `long:"storage-io-max-response-latency-pct" description:"The percentage of I/O permitted over the SSC maximum response time before a violation is flagged" default:"0.1"`

	VolumeMetadataMetricPeriod          time.Duration `long:"volume-metadata-metric-period" description:"Periodic volume metadata metric period" default:"1h"`
	VolumeMetadataMetricTruncate        time.Duration `long:"volume-metadata-metric-period-truncate" description:"The value of the periodic volume metadata metric timer is truncated to this duration" default:"1h"`
	VolumeMetadataMetricSuppressStartup bool          `long:"volume-metadata-metric-suppress-startup-metrics" description:"Do not insert (unaligned) volume metadata metrics on startup"`
	VolumeMetadataMaxBuffered           int           `long:"volume-metadata-metric-max-buffered" description:"The maximum number of buffered volume metadata records when the database is not available" default:"1000"`

	VolumeIOBurstTolerance     int     `long:"volume-io-metric-burst-tolerance" description:"The number of times the volume I/O metric buffer limits may be continuously exceeded (sampled at the database connection retry interval)" default:"3"`
	VolumeIOMaxBuffered        int     `long:"volume-io-metric-max-buffered" description:"The maximum number of buffered volume I/O metric records when the database is not available" default:"2000"`
	VolumeIOMaxResponseTimePct float64 `long:"volume-io-max-response-latency-pct" description:"The percentage of I/O permitted over the service plan maximum response time before a violation is flagged" default:"0.1"`

	SPAMetricPeriod          time.Duration `long:"spa-metric-period" description:"Periodic spa metric period" default:"1h"`
	SPAMetricTruncate        time.Duration `long:"spa-metric-period-truncate" description:"The value of the periodic spa metric timer is truncated to this duration" default:"1h"`
	SPAMetricSuppressStartup bool          `long:"spa-metric-suppress-startup-metrics" description:"Do not insert (unaligned) spa metrics on startup"`
	SPAMaxBuffered           int           `long:"spa-metric-max-buffered" description:"The maximum number of buffered spa metric records when the database is not available" default:"100"`
}

// module constants
const (
	PingIntervalDefault                = time.Second * 10
	PoolMaxBufferedDefault             = 100
	PoolPeriodDefault                  = time.Hour
	RetryIntervalDefault               = time.Second * 1
	SloResponseTimeAverage             = "Response Time Average"
	SloResponseTimeMaximum             = "Response Time Maximum"
	SPAMaxBufferedDefault              = 100
	SPAPeriodDefault                   = time.Hour
	SscResponseTimeAverage             = "Response Time Average"
	SscResponseTimeMaximum             = "Response Time Maximum"
	StorageIOBurstToleranceDefault     = 3
	StorageIOMaxBufferedDefault        = 1000
	StorageIOMaxResponseTimePctDefault = float64(0.1)
	StorageMetadataMaxBufferedDefault  = 500
	StorageMetadataPeriodDefault       = time.Hour
	VolumeIOBurstToleranceDefault      = 3
	VolumeIOMaxBufferedDefault         = 2000
	VolumeIOMaxResponseTimePctDefault  = float64(0.1)
	VolumeMetadataMaxBufferedDefault   = 1000
	VolumeMetadataPeriodDefault        = time.Hour
)

// Component that inserts metrics into the metrics database
type Component struct {
	Args
	App *centrald.AppCtx
	Log *logging.Logger

	oCrud                  crud.Ops
	ready                  bool
	consumerRegistered     bool
	pgDB                   pgdb.DB
	db                     *sql.DB
	dbConnected            bool
	tableVersionsLoaded    bool
	tableVersions          map[string]int
	servicePlanDataUpdated bool
	servicePlanCache       map[string]*ServicePlanData
	storageTypeDataUpdated bool
	storageTypeCache       map[string]*StorageTypeData
	stmtCache              map[string]*sql.Stmt
	ph                     poolHandler
	vmh                    vsMetaHandler
	smh                    sMetaHandler
	vIOP                   vsIOProcessor // volume I/O metrics pipeline stage1
	vIOW                   vsIOWriter    // volume I/O metrics pipeline stage2
	numVIODropped          int
	numVIOCapacityExceeded int
	sIOP                   sIOProcessor // storage I/O metrics pipeline stage1
	sIOW                   sIOWriter    // storage I/O metrics pipeline stage2
	numSIODropped          int
	numSIOCapacityExceeded int
	mux                    sync.Mutex
	worker                 util.Worker
	spah                   spaHandler
}

// Register must be called from main to initialize and register this component
func Register(args *Args) *Component {
	c := &Component{}
	c.Args = *args
	centrald.AppRegisterComponent(c)
	return c
}

// Init registers handlers for this component
func (c *Component) Init(app *centrald.AppCtx) {
	c.App = app
	c.Log = app.Log
	c.TLSCertificate = string(app.Server.TLSCertificate)
	c.TLSCertificateKey = string(app.Server.TLSCertificateKey)
	c.TLSCACertificate = string(app.Server.TLSCACertificate)
	c.pgDB = pgdb.New(c.Log)
	if c.RetryInterval <= 0 {
		c.RetryInterval = RetryIntervalDefault
	}
	if c.PingInterval <= 0 {
		c.PingInterval = PingIntervalDefault
	}
	if c.PoolMetricPeriod <= 0 {
		c.PoolMetricPeriod = PoolPeriodDefault
	}
	if c.PoolMetricTruncate <= 0 || c.PoolMetricTruncate > c.PoolMetricPeriod {
		c.PoolMetricTruncate = c.PoolMetricPeriod
	}
	if c.PoolMaxBuffered < 0 {
		c.PoolMaxBuffered = PoolMaxBufferedDefault
	}
	if c.StorageIOBurstTolerance <= 0 {
		c.StorageIOBurstTolerance = StorageIOBurstToleranceDefault
	}
	if c.StorageIOMaxBuffered < 0 {
		c.StorageIOMaxBuffered = StorageIOMaxBufferedDefault
	}
	if c.StorageIOMaxResponseTimePct <= 0.0 {
		c.StorageIOMaxResponseTimePct = StorageIOMaxResponseTimePctDefault
	}
	if c.StorageMetadataMetricPeriod <= 0 {
		c.StorageMetadataMetricPeriod = StorageMetadataPeriodDefault
	}
	if c.StorageMetadataMetricTruncate <= 0 || c.StorageMetadataMetricTruncate > c.StorageMetadataMetricPeriod {
		c.StorageMetadataMetricTruncate = c.StorageMetadataMetricPeriod
	}
	if c.StorageMetadataMaxBuffered < 0 {
		c.StorageMetadataMaxBuffered = StorageMetadataMaxBufferedDefault
	}
	if c.VolumeMetadataMetricPeriod <= 0 {
		c.VolumeMetadataMetricPeriod = VolumeMetadataPeriodDefault
	}
	if c.VolumeMetadataMetricTruncate <= 0 || c.VolumeMetadataMetricTruncate > c.VolumeMetadataMetricPeriod {
		c.VolumeMetadataMetricTruncate = c.VolumeMetadataMetricPeriod
	}
	if c.VolumeMetadataMaxBuffered < 0 {
		c.VolumeMetadataMaxBuffered = VolumeMetadataMaxBufferedDefault
	}
	if c.VolumeIOBurstTolerance <= 0 {
		c.VolumeIOBurstTolerance = VolumeIOBurstToleranceDefault
	}
	if c.VolumeIOMaxBuffered < 0 {
		c.VolumeIOMaxBuffered = VolumeIOMaxBufferedDefault
	}
	if c.VolumeIOMaxResponseTimePct <= 0.0 {
		c.VolumeIOMaxResponseTimePct = VolumeIOMaxResponseTimePctDefault
	}
	if c.SPAMetricPeriod <= 0 {
		c.SPAMetricPeriod = SPAPeriodDefault
	}
	if c.SPAMetricTruncate <= 0 || c.SPAMetricTruncate > c.SPAMetricPeriod {
		c.SPAMetricTruncate = c.SPAMetricPeriod
	}
	if c.SPAMaxBuffered < 0 {
		c.SPAMaxBuffered = SPAMaxBufferedDefault
	}
	wa := &util.WorkerArgs{
		Name:          "metrics",
		Log:           c.Log,
		SleepInterval: c.RetryInterval,
	}
	c.worker, _ = util.NewWorker(wa, c)
	c.ph.Init(c)
	c.vmh.Init(c)
	c.smh.Init(c)
	c.vIOP.Init(c)
	c.vIOW.Init(c)
	c.sIOP.Init(c)
	c.sIOW.Init(c)
	c.spah.Init(c)
}

// Start starts this component
func (c *Component) Start() {
	c.Log.Info("Starting Metrics")
	c.oCrud = crud.NewClient(c.App.ClientAPI, c.Log)
	c.ph.Start()
	c.vmh.Start()
	c.smh.Start()
	c.worker.Start()
	c.spah.Start()
}

// Stop terminates this component
func (c *Component) Stop() {
	c.ph.Stop()
	c.vmh.Stop()
	c.smh.Stop()
	c.worker.Stop()
	c.vIOP.Stop()
	c.vIOW.Stop()
	c.sIOP.Stop()
	c.sIOW.Stop()
	c.spah.Stop()
	c.Log.Info("Stopped Metrics")
}

// Notify is used to wake the loop
func (c *Component) Notify() {
	c.worker.Notify()
}

// Buzz satisfies the util.WorkerBee interface
func (c *Component) Buzz(ctx context.Context) error {
	var err error
	if c.db == nil {
		c.db, err = c.pgDB.OpenDB(c.getDBArgs())
	}
	if err == nil && c.db != nil {
		if err = c.ping(ctx); err != nil {
			c.closeDB()
			c.worker.SetSleepInterval(c.RetryInterval)
		} else {
			c.worker.SetSleepInterval(c.PingInterval)
		}
	}
	if err == nil && !c.tableVersionsLoaded {
		err = c.fetchTableVersions(ctx)
	}
	if err == nil && !c.servicePlanDataUpdated {
		err = c.updateServicePlanData(ctx)
	}
	if err == nil && !c.storageTypeDataUpdated {
		err = c.updateStorageTypeData(ctx)
	}
	c.ready = c.dbConnected && c.tableVersionsLoaded && c.servicePlanDataUpdated && c.storageTypeDataUpdated
	if err == nil {
		c.ph.Drain(ctx)
		c.vmh.Drain(ctx)
		c.smh.Drain(ctx)
		c.spah.Drain(ctx)
	}
	if c.ready && !c.consumerRegistered {
		c.Log.Info("Ready to receive metric data")
		c.App.MetricMover.RegisterConsumer(c)
		c.vIOP.Start()
		c.vIOW.Start()
		c.sIOP.Start()
		c.sIOW.Start()
		c.consumerRegistered = true
	}
	if err == nil && c.consumerRegistered {
		c.vsIoMLimitCapacityWithBurstTolerance()
		c.sIoMLimitCapacityWithBurstTolerance()
	}
	return err
}

// getDBArgs constructs the connection arguments
func (c *Component) getDBArgs() *pgdb.DBArgs {
	return &pgdb.DBArgs{
		Host:              c.Host,
		Port:              c.Port,
		User:              c.User,
		Database:          c.Database,
		DebugLevel:        c.DebugLevel,
		UseSSL:            c.UseSSL,
		TLSCertificate:    c.TLSCertificate,
		TLSCertificateKey: c.TLSCertificateKey,
		TLSCACertificate:  c.TLSCACertificate,
		TLSServerName:     c.SSLServerName,
	}
}

// ping checks the connection to the database and issues transition messages.
// It also resets the following flags when connection is lost:
//  - tableVersionsLoaded
//  - servicePlanDataUpdated
func (c *Component) ping(ctx context.Context) error {
	var err error
	if err = c.pgDB.PingContext(ctx, c.db); err == nil {
		if !c.dbConnected {
			c.Log.Info("Connected to metrics database")
		}
		c.dbConnected = true
	} else {
		err = fmt.Errorf("ping: %s", err.Error())
		if c.dbConnected {
			c.Log.Error("Lost connection to metrics database")
		}
		c.dbConnected = false
		c.tableVersionsLoaded = false
		c.servicePlanDataUpdated = false
	}
	return err
}

// fetchTableVersions loads the table schema versions from the database
func (c *Component) fetchTableVersions(ctx context.Context) error {
	q := "SELECT TableName, Version FROM SchemaVersion"
	rs, err := c.db.QueryContext(ctx, q)
	if err != nil {
		c.Log.Errorf("SchemaVersion query error:%s (%s)", err.Error(), c.pgDB.SQLCode(err))
		return err
	}
	defer rs.Close()
	sv := make(map[string]int)
	for rs.Next() {
		var tn string
		var ver int
		if err = rs.Scan(&tn, &ver); err != nil {
			c.Log.Errorf("SchemaVersion scan error:%s (%s)", err.Error(), c.pgDB.SQLCode(err))
			return err
		}
		sv[tn] = ver
	}
	if err = rs.Err(); err != nil {
		c.Log.Errorf("SchemaVersion row error:%s (%s)", err.Error(), c.pgDB.SQLCode(err))
		return err
	}
	c.tableVersions = sv
	c.tableVersionsLoaded = true
	c.Log.Info("Schema loaded")
	return nil
}

// TableVersion returns the schema table version number or 0
func (c *Component) TableVersion(tn string) int {
	if c.tableVersions != nil {
		if ver, ok := c.tableVersions[tn]; ok {
			return ver
		}
	}
	return 0
}

// closeDB closes the database and empties the statement cache, closing all statements
func (c *Component) closeDB() {
	c.mux.Lock()
	c.db.Close()
	c.db = nil
	c.dbConnected = false
	sc := c.stmtCache
	c.stmtCache = nil
	c.mux.Unlock()
	if sc != nil {
		for n, stmt := range sc {
			c.Log.Debugf("Purging cached statement %s", n)
			stmt.Close()
		}
	}
}

// StmtCacheGet retrieves a cached prepared statement
func (c *Component) StmtCacheGet(n string) *sql.Stmt {
	c.mux.Lock()
	defer c.mux.Unlock()
	if c.stmtCache != nil {
		stmt, ok := c.stmtCache[n]
		if ok {
			return stmt
		}
	}
	return nil
}

// StmtCacheSet inserts a prepared statement into the cache
func (c *Component) StmtCacheSet(n string, stmt *sql.Stmt) {
	c.mux.Lock()
	defer c.mux.Unlock()
	if c.stmtCache == nil {
		c.stmtCache = make(map[string]*sql.Stmt)
	}
	c.stmtCache[n] = stmt
	c.Log.Debugf("Caching statement %s", n)
}

// SafePrepare provides a thread safe way to prepare a statement.
// It addresses the race conditions inherent with the concurrent loss of connectivity
// and closure of the database handle done in the component body.
func (c *Component) SafePrepare(ctx context.Context, query string) (*sql.Stmt, error) {
	c.mux.Lock()
	defer c.mux.Unlock()
	if !c.dbConnected {
		return nil, fmt.Errorf("database not connected")
	}
	return c.db.PrepareContext(ctx, query)
}

var nilUUID = uuid.Nil.String()

// idOrZeroUUID replaces an empty id string with the zero uuid
func idOrZeroUUID(id string) string {
	if len(id) > 0 {
		return id
	}
	return nilUUID
}
