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
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/auth"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/audit_log"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	appAuth "github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/pgdb"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/op/go-logging"
	uuid "github.com/satori/go.uuid"
)

// Args contains settable parameters for this component
type Args struct {
	Host       string `long:"host" description:"The hostname of the audit log database service" default:"localhost"`
	Port       int16  `long:"port" description:"The port number of the audit log database service" default:"5432"`
	User       string `long:"user" description:"The audit log user account" default:"postgres"`
	Database   string `long:"database" description:"The name of the database" default:"nuvo_audit"`
	DebugLevel int    `long:"debug-level" description:"Controls debugging" default:"1"`

	UseSSL        bool   `long:"ssl" description:"Use SSL to communicate with the audit log database"`
	SSLServerName string `long:"ssl-server-name" description:"The actual server name of the audit log database SSL certificate"`

	// copied from service flags on startup
	TLSCertificate    string `no-flag:"1"`
	TLSCertificateKey string `no-flag:"1"`
	TLSCACertificate  string `no-flag:"1"`

	RetryInterval time.Duration `long:"retry-interval" description:"Database connection retry interval" default:"1s"`
	PingInterval  time.Duration `long:"ping-interval" description:"Database periodic ping interval" default:"10s"`

	AuditPurgeTaskInterval         time.Duration `long:"audit-purge-task-interval" description:"The interval for running task to purge outdated audit log records" default:"24h"`
	AuditRecordRetentionPeriod     time.Duration `long:"audit-record-retention-period" description:"The retention period for outdated audit log records" default:"61368h"`        // 7 years
	AuditEventRetentionPeriod      time.Duration `long:"audit-event-retention-period" description:"The retention period for outdated audit log events" default:"2184h"`           // 3 months
	AuditAnnotationRetentionPeriod time.Duration `long:"audit-annotation-retention-period" description:"The retention period for outdated audit log annotations" default:"2184h"` // 3 months
}

// module constants
const (
	PingIntervalDefault  = time.Second * 10
	RetryIntervalDefault = time.Second * 1
)

// Component that inserts entries into the audit log database
type Component struct {
	Args
	App *centrald.AppCtx
	Log *logging.Logger

	ready               bool
	pgDB                pgdb.DB
	db                  *sql.DB
	dbConnected         bool
	tableVersionsLoaded bool
	tableVersions       map[string]int
	stmtCache           map[string]*sql.Stmt
	handler             logHandler
	numDropped          int
	mux                 sync.Mutex
	worker              util.Worker
	purgeTaskID         string
	lastPurgeTaskTime   time.Time
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
	c.App.AuditLog = c // provide the interface
	if c.RetryInterval <= 0 {
		c.RetryInterval = RetryIntervalDefault
	}
	if c.PingInterval <= 0 {
		c.PingInterval = PingIntervalDefault
	}
	wa := &util.WorkerArgs{
		Name:          "audit log",
		Log:           c.Log,
		SleepInterval: c.RetryInterval,
	}
	c.worker, _ = util.NewWorker(wa, c)
	c.handler.Init(c)
	c.lastPurgeTaskTime = time.Now()

	c.RegisterHandlers(c.App.API)

	purgeTaskRegisterAnimator(c)
}

// Start starts this component
func (c *Component) Start() {
	c.Log.Info("Starting AuditLog")
	c.handler.Start()
	c.worker.Start()
}

// Stop terminates this component
func (c *Component) Stop() {
	c.worker.Stop()
	c.handler.Stop()
	c.Log.Info("Stopped AuditLog")
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
	ready := c.dbConnected && c.tableVersionsLoaded
	if ready && !c.ready {
		c.Log.Info("AuditLog is ready")
	}
	c.ready = ready

	// audit log records purging
	now := time.Now()
	if ready && now.Sub(c.lastPurgeTaskTime) >= c.AuditPurgeTaskInterval {
		c.Log.Debugf("It is time to run TaskAuditRecordsPurge")
		taskID, err := c.App.TaskScheduler.RunTask(&models.Task{TaskCreateOnce: models.TaskCreateOnce{Operation: com.TaskAuditRecordsPurge}})
		if err == nil {
			c.lastPurgeTaskTime = now
			c.purgeTaskID = ""
		} else {
			c.Log.Errorf("Task [%s] to purge AuditLog records failed: %s", taskID, err.Error())
			c.purgeTaskID = taskID
		}
	}
	return err
}

// Errorf produces an extended error message
func (c *Component) Errorf(format string, args ...interface{}) *centrald.Error {
	return &centrald.Error{C: centrald.ErrorDbError.C, M: fmt.Sprintf(centrald.ErrorDbError.M+": "+format, args...)}
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
			c.Log.Info("Connected to AuditLog database")
		}
		c.dbConnected = true
	} else {
		err = fmt.Errorf("ping: %s", err.Error())
		if c.dbConnected {
			c.Log.Error("Lost connection to AuditLog database")
		}
		c.dbConnected = false
		c.tableVersionsLoaded = false
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
		return nil, c.Errorf("database not connected")
	}
	return c.db.PrepareContext(ctx, query)
}

// RegisterHandlers registers the Watcher handlers and the access control manager
func (c *Component) RegisterHandlers(api *operations.NuvolosoAPI) {
	api.AuditLogAuditLogCreateHandler = audit_log.AuditLogCreateHandlerFunc(c.handler.auditLogCreate)
	api.AuditLogAuditLogListHandler = audit_log.AuditLogListHandlerFunc(c.handler.auditLogList)
}

// Ready returns a DB error when the audit log database is not ready
func (c *Component) Ready() error {
	if !c.ready {
		msg := "Audit Log is not ready"
		c.Log.Error(msg)
		return c.Errorf(msg)
	}
	return nil
}

// Annotation implements the AppAudit interface
func (c *Component) Annotation(ctx context.Context, s auth.Subject, parent int32, action centrald.AuditAction, oid models.ObjID, name models.ObjName, err bool, message string) {
	ai := s.(*appAuth.Info)
	c.handler.log(ctx, ai, com.AnnotationClass, parent, action, oid, name, "", err, message)
}

// Event implements the AppAudit interface
func (c *Component) Event(ctx context.Context, s auth.Subject, action centrald.AuditAction, oid models.ObjID, name models.ObjName, refID models.ObjIDMutable, err bool, message string) {
	ai := s.(*appAuth.Info)
	c.handler.log(ctx, ai, com.EventClass, 0, action, oid, name, refID, err, message)
}

// Post implements the AppAudit interface
func (c *Component) Post(ctx context.Context, s auth.Subject, action centrald.AuditAction, oid models.ObjID, name models.ObjName, refID models.ObjIDMutable, err bool, message string) {
	ai := s.(*appAuth.Info)
	c.handler.log(ctx, ai, com.AuditClass, 0, action, oid, name, refID, err, message)
}

// Expire implements the AppAudit interface
func (c *Component) Expire(ctx context.Context, baseTime time.Time) error {
	return c.handler.expire(ctx, baseTime)
}

var nilUUID = uuid.Nil.String()

// idOrZeroUUID replaces an empty id string with the zero uuid
func idOrZeroUUID(id string) string {
	if len(id) > 0 {
		return id
	}
	return nilUUID
}
