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


package centrald

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/auth"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/housekeeping"
	mm "github.com/Nuvoloso/kontroller/pkg/metricmover"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient"
	"github.com/Nuvoloso/kontroller/pkg/pstore"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/jessevdk/go-flags"
	"github.com/op/go-logging"
)

// appMutex is for general use in this package
var appMutex sync.Mutex

// AppComponent defines a sub-component of the application
type AppComponent interface {
	Init(*AppCtx)
	Start()
	Stop()
}

var components = make([]AppComponent, 0)

// AppRegisterComponent is used to register a component of the application typically from an init function
func AppRegisterComponent(c AppComponent) {
	components = append(components, c)
}

// AppArgs are the creation arguments for the application
type AppArgs struct {
	AgentdCertPath             flags.Filename  `long:"agentd-cert" description:"agentd certificate file"`
	AgentdKeyPath              flags.Filename  `long:"agentd-key" description:"agentd private key file"`
	ClusterdCertPath           flags.Filename  `long:"clusterd-cert" description:"clusterd certificate file"`
	ClusterdKeyPath            flags.Filename  `long:"clusterd-key" description:"clusterd private key file"`
	ImagePullSecretPath        flags.Filename  `long:"image-pull-secret" description:"container image pull secret file for customer deployments"`
	ImagePath                  string          `long:"image-path" description:"container repository image path" default:"407798037446.dkr.ecr.us-west-2.amazonaws.com/nuvoloso"`
	ClusterDeployTag           string          `long:"cluster-deploy-tag" description:"Image tag to use when generating cluster deployment YAML" default:"v1"`
	ClientTimeout              int             `long:"client-timeout" description:"Specify the timeout duration in seconds for client operations" default:"20"`
	ClusterTimeoutAfterMisses  int             `long:"cluster-timeout-after-misses" description:"The maximum number of cluster heartbeats that can be missed" default:"2"`
	ClusterTimeoutCheckPeriod  time.Duration   `long:"cluster-timeout-check-period" description:"The duration between successive checks for cluster services that have timed out" default:"5m"`
	ClusterType                string          `long:"cluster-type" description:"The type of cluster in which the management services will be deployed" default:"kubernetes"`
	CSPTimeout                 int             `long:"csp-timeout" description:"Specify the timeout duration in seconds for csp domain operations" default:"20"`
	DebugClient                bool            `long:"debug-client" description:"Log client HTTP requests and responses"`
	DebugREI                   bool            `long:"debug-permit-rei" description:"Permit runtime error injection"`
	DebugREIPath               flags.Filename  `long:"debug-rei-dir" description:"Runtime error injection arena" default:"/var/run/nuvoloso/rei"`
	DriverType                 string          `long:"driver-type" description:"Specify the volume driver type" default:"csi" choice:"flex" choice:"csi"`
	ManagementServiceName      string          `long:"management-service-name" description:"The name of the management service" default:"nuvo-https"`
	ManagementServiceNamespace string          `long:"management=service-namespace" description:"The namespace in the cluster where the management services will be deployed" default:"nuvoloso-management"`
	Server                     *restapi.Server `no-flag:"yes" json:"-"` // Has the TLS config
	HeartbeatPeriod            int             `long:"heartbeat-period" description:"The heartbeat period in seconds" default:"30"`
	TaskPurgeDelay             int             `long:"task-purge-delay" description:"The time interval in seconds after which completed tasks are deleted" default:"60"`
	VersionLogPeriod           time.Duration   `long:"version-log-period" description:"The period for logging build version information" default:"30m"`
	ServiceVersion             string
	NuvoAPIVersion             string
	InvocationArgs             string           `json:"-"`
	CrudeOps                   crude.Ops        `json:"-"`
	API                        *ops.NuvolosoAPI `json:"-"`
	DS                         DataStore        `json:"-"`
	Log                        *logging.Logger  `json:"-"`
}

// AppAudit provides access to the audit log when available
type AppAudit interface {
	// RegisterHandlers registers REST API operation handlers
	RegisterHandlers(api *ops.NuvolosoAPI)
	// Ready returns an error if the audit log is not ready
	Ready() error
	// Annotation adds a new annotation entry to the audit log
	Annotation(ctx context.Context, ai auth.Subject, parent int32, action AuditAction, oid models.ObjID, name models.ObjName, err bool, message string)
	// Event adds a new event entry to the audit log
	Event(ctx context.Context, ai auth.Subject, action AuditAction, oid models.ObjID, name models.ObjName, refID models.ObjIDMutable, err bool, message string)
	// Post creates a new audit log entry
	Post(ctx context.Context, ai auth.Subject, action AuditAction, oid models.ObjID, name models.ObjName, refID models.ObjIDMutable, err bool, message string)
	// Expire deletes outdated records from audit log according to its settings
	Expire(ctx context.Context, baseTime time.Time) error
}

// AppCrudHelpers provides serialization and other support for object CRUD operations
type AppCrudHelpers interface {
	Lock()          // obtain a write lock
	Unlock()        // release the write lock
	RLock()         // obtain a read lock
	RUnlock()       // release the read lock
	NodeLock()      // obtain a write lock on a Node object
	NodeUnlock()    // release the write lock on a Node object
	ClusterLock()   // obtain a write lock on a Cluster object
	ClusterUnlock() // release the write lock on a Cluster object
	// MakeStdUpdateArgs creates UpdateArgs for exposed mObj types (see handlers)
	MakeStdUpdateArgs(mObj interface{}, id string, version *int32, params [NumActionTypes][]string) (*UpdateArgs, error)
}

// internal interfaces for testing
type appInternal interface {
	getClientArgs() (*mgmtclient.APIArgs, error)
}

// AppCtx contains the runtime context of the central daemon.
type AppCtx struct {
	AppArgs
	Service        util.Controller // Keeps the state
	AppCSP         AppCloudServiceProvider
	SFC            StorageFormulaComputer
	ClientAPI      mgmtclient.API
	MetricMover    mm.MetricMover
	PSO            pstore.Operations
	AuditLog       AppAudit
	TaskScheduler  housekeeping.TaskScheduler
	CrudHelpers    AppCrudHelpers // set by the handler component
	StackLogger    util.StackLogger
	cspCache       map[models.CspDomainTypeMutable]csp.CloudServiceProvider
	cspSTCache     []*models.CSPStorageType
	cspClientCache map[models.ObjID]cspDomainClientRecord
	appInt         appInternal
	MCDeployer     cluster.MCDeployer
}

// AppInit installs the backend installerRegistry for the Nuvoloso API implemented
// by this server and returns a context object
func AppInit(args *AppArgs) *AppCtx {
	app := &AppCtx{
		AppArgs: *args,
	}
	app.appInt = app // self-ref
	app.AppCSP = app // self-ref
	app.SFC = app    // self-ref
	svcArgs := util.ServiceArgs{
		ServiceType:         "centrald",
		ServiceVersion:      args.ServiceVersion,
		HeartbeatPeriodSecs: 0, // TBD
		Log:                 args.Log,
		MaxMessages:         0, // TBD
		ServiceAttributes:   map[string]models.ValueType{},
	}
	app.Service = util.NewService(&svcArgs)
	app.MetricMover = mm.NewMetricMover(args.Log)
	tsArgs := &housekeeping.ManagerArgs{
		PurgeDelay: time.Duration(app.TaskPurgeDelay) * time.Second,
		CrudeOps:   app.CrudeOps,
		Log:        app.Log,
	}
	app.TaskScheduler, _ = housekeeping.NewManager(tsArgs)
	app.StackLogger = util.NewStackLogger(args.Log)
	app.PSO, _ = pstore.NewController(app.getPSOArgs())
	app.MCDeployer = cluster.GetMCDeployer(app.Log)
	if app.DebugREI {
		rei.Enabled = true
		rei.SetGlobalArena(string(app.DebugREIPath))
	}
	for _, c := range components {
		c.Init(app)
	}
	return app
}

// Start the application
func (app *AppCtx) Start() error {
	clientArgs, err := app.appInt.getClientArgs()
	if err != nil {
		return err
	}
	app.ClientAPI, err = mgmtclient.NewAPI(clientArgs)
	if err != nil {
		err = fmt.Errorf("failed to create client API: %s", err.Error())
		app.Log.Criticalf("Error: %s", err.Error())
		return err
	}
	app.Log.Debugf("Created client API")
	app.Service.SetState(util.ServiceStarting)
	if err = app.DS.Start(); err != nil {
		err = fmt.Errorf("failed to start data store: %s", err.Error())
		app.Log.Criticalf("Error: %s", err.Error())
		return err
	}
	app.StackLogger.Start()
	for _, c := range components {
		c.Start()
	}
	app.Service.SetState(util.ServiceReady)
	return nil
}

// Stop the application
func (app *AppCtx) Stop() {
	app.Service.SetState(util.ServiceStopping)
	app.CrudeOps.TerminateAllWatchers()
	for _, c := range components {
		c.Stop()
	}
	app.StackLogger.Stop()
	app.Service.SetState(util.ServiceStopped)
	app.DS.Stop()
}

// Returns the arguments for the management client API (back to centrald via http or unix socket)
func (app *AppCtx) getClientArgs() (*mgmtclient.APIArgs, error) {
	// Host should only be used if http server listener is enabled (by default or in the list)
	host := ""
	if app.Server != nil && app.Server.Host != "" && (len(app.Server.EnabledListeners) == 0 || util.Contains(app.Server.EnabledListeners, "http")) {
		host = app.Server.Host
	}
	// SocketPath should only be used if the unix socket server listener is enabled
	socketPath := ""
	if app.Server != nil && app.Server.SocketPath != "" && util.Contains(app.Server.EnabledListeners, mgmtclient.UnixScheme) {
		socketPath = string(app.Server.SocketPath)
	}
	if app.Server == nil || ((host == "" || app.Server.Port == 0) && socketPath == "") {
		return nil, fmt.Errorf("local client bindings not specified")
	}
	return &mgmtclient.APIArgs{
		Host:       host,
		Port:       app.Server.Port,
		SocketPath: socketPath,
		Debug:      app.DebugClient,
		Log:        app.Log,
	}, nil
}

// returns arguments to create a pstore controller
func (app *AppCtx) getPSOArgs() *pstore.ControllerArgs {
	return &pstore.ControllerArgs{
		Log:      app.Log,
		DebugREI: app.DebugREI,
	}
}
