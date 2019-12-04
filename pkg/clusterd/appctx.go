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


package clusterd

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	"github.com/Nuvoloso/kontroller/pkg/clusterd/state"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/layout"
	mm "github.com/Nuvoloso/kontroller/pkg/metricmover"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/util"
	flags "github.com/jessevdk/go-flags"
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
	ClusterType                   string           `long:"cluster-type" description:"Specify the type of cluster" default:"kubernetes"`
	ClusterName                   string           `long:"cluster-name" description:"The name to use when creating a new cluster object" env:"NUVO_CLUSTER_NAME"`
	ClientTimeout                 int              `long:"client-timeout" description:"Specify the timeout duration in seconds for client operations" default:"20"`
	CSISocket                     flags.Filename   `short:"K" long:"csi-socket" description:"The pathname of the CSI sidecar socket"`
	ClusterID                     string           `short:"C" long:"cluster-id" description:"The Cluster object ID"`
	CSPDomainType                 string           `short:"T" long:"csp-domain-type" description:"The type of cloud service provider" default:"AWS"`
	CSPDomainID                   string           `short:"D" long:"csp-domain-id" description:"The associated CSPDomain object identifier" required:"yes"`
	SystemID                      string           `short:"S" long:"system-id" description:"The associated System object identifier" required:"yes"`
	ManagementHost                string           `short:"M" long:"management-host" description:"The hostname or address of the central management service" required:"yes"`
	ManagementPort                int              `short:"P" long:"management-port" description:"The port of the central management service" default:"443"`
	SSLServerName                 string           `long:"ssl-server-name" description:"The actual server name of the management service SSL certificate, defaults to the --management-host value"`
	SSLSkipVerify                 bool             `short:"k" long:"ssl-skip-verify" description:"Allow unverified upstream service certificate or host name"`
	MetricRetryInterval           time.Duration    `long:"metric-retry-interval" description:"The period between metric upload retries" default:"30s"`
	MetricVolumeIOMax             int              `long:"metric-volume-io-max" description:"The maximum number of volume I/O metrics that will be buffered" default:"1000"`
	HeartbeatPeriod               int              `long:"heartbeat-period" description:"The heartbeat period in seconds" default:"30"`
	HeartbeatTaskPeriodMultiplier int              `long:"heartbeat-task-period-multiplier" description:"The period of the consolidated heartbeat task expressed as a multiple of the heartbeat period" default:"10"`
	VersionLogPeriod              time.Duration    `long:"version-log-period" description:"The period for logging build version information" default:"30m"`
	MaxMessages                   int              `long:"max-messages" description:"The maximum number of service messages to maintain"`
	DebugClient                   bool             `long:"debug-client" description:"Log client HTTP requests and responses"`
	DebugREI                      bool             `long:"debug-permit-rei" description:"Permit runtime error injection"`
	DebugREIPath                  flags.Filename   `long:"debug-rei-dir" description:"Runtime error injection arena" default:"/var/run/nuvoloso/rei"`
	CGSnapshotSchedPeriod         time.Duration    `long:"snap-period" description:"Snapshot retry interval" default:"5m"`
	CGSnapshotTimeoutMultiplier   float64          `long:"snap-timeout" description:"Specify the consistency group snapshot timeout multiplier over RPO" default:".9"`
	LayoutAlgorithms              []string         `long:"layout-algorithm" description:"Specify a layout algorithm. Repeat for each supported layout." default:"StandaloneLocalUnshared"`
	ClusterIdentifierSecretName   string           `long:"cluster-identifier-secret-name" description:"Specify the name of the secret which stores cluster identification information." default:"cluster-identifier"`
	ClusterNamespace              string           `long:"cluster-namespace" description:"Specify the cluster namespace." default:"nuvoloso-cluster"`
	NodeControllerKind            string           `long:"node-controller-kind" description:"Specify the kind of controller for the node pods" default:"DaemonSet"`
	NodeControllerName            string           `long:"node-controller-name" description:"Specify the name of the node controller" default:"nuvoloso-node"`
	ReleaseStorageRequestTimeout  time.Duration    `long:"release-storage-request-timeout" description:"Specify the time taken for a request to release unused storage." default:"5m"`
	CrudeOps                      crude.Ops        `json:"-"`
	Log                           *logging.Logger  `json:"-"`
	API                           *ops.NuvolosoAPI `json:"-"`
	Server                        *restapi.Server  `no-flag:"yes" json:"-"` // Has the TLS config
	ServiceVersion                string
	NuvoAPIVersion                string
	InvocationArgs                string `json:"-"`
}

// AppServant is an interface for application methods exposed to components.
type AppServant interface {
	GetAPIArgs() *mgmtclient.APIArgs
	FatalError(error)
	InitializeCSPClient(dObj *models.CSPDomain) error
	GetServicePlan(ctx context.Context, spID string) (*models.ServicePlan, error)
}

// AppObjects provides access to significant objects when available
type AppObjects interface {
	// GetCluster returns the Cluster object once available
	GetCluster() *models.Cluster
	// GetCspDomain returns the CSPDomain object once available
	GetCspDomain() *models.CSPDomain
}

// StateUpdater updates clusterd state
type StateUpdater interface {
	// HeartbeatTaskPeriodSecs returns the period of the consolidated heartbeat task
	HeartbeatTaskPeriodSecs() int64
	// UpdateState updates the state of the service in the cluster object
	UpdateState(ctx context.Context) error
}

// VolumeDeletionHandler unbinds/deletes a volume asynchronously
type VolumeDeletionHandler interface {
	DeleteOrUnbindVolume(ctx context.Context, volumeID string, logKeyword string) error
}

// AppCtx contains the runtime context of the cluster daemon.
type AppCtx struct {
	AppArgs
	Service        util.Controller // Keeps the state
	ClusterClient  cluster.Client
	ClusterMD      map[string]string
	ClientAPI      mgmtclient.API
	CSP            csp.CloudServiceProvider
	CSPClient      csp.DomainClient
	InstanceMD     map[string]string
	CSPDomainAttrs map[string]models.ValueType
	CSPClientReady bool
	MetricMover    mm.MetricMover
	AppServant
	AppObjects            // implemented by heartbeat
	StateUpdater          // implemented by heartbeat
	StateGuard            *util.CriticalSectionGuard
	StateOps              state.ClusterStateOperators // initialized by heartbeat
	StackLogger           util.StackLogger
	InFatalError          bool // tests can set this to avoid exiting
	FatalErrorCount       int
	servicePlanMap        map[string]*models.ServicePlan
	OCrud                 crud.Ops
	VolumeDeletionHandler // implemented by csi
	// internal representation of the LayoutAlgorithms field
	StorageAlgorithms []*layout.Algorithm
}

// AppInit installs the backend installerRegistry for the Nuvoloso API implemented
// by this server and returns a context object
func AppInit(args *AppArgs) (*AppCtx, error) {
	app := &AppCtx{
		AppArgs: *args,
	}
	app.AppServant = app // self-reference
	svcArgs := util.ServiceArgs{
		ServiceType:         "clusterd",
		ServiceVersion:      app.ServiceVersion,
		HeartbeatPeriodSecs: int64(app.HeartbeatPeriod),
		Log:                 app.Log,
		MaxMessages:         app.MaxMessages,
		ServiceAttributes:   map[string]models.ValueType{},
	}
	app.Service = util.NewService(&svcArgs)
	app.StateGuard = util.NewCriticalSectionGuard()
	app.MetricMover = mm.NewMetricMover(args.Log)
	app.StorageAlgorithms = make([]*layout.Algorithm, len(app.LayoutAlgorithms))
	for i, n := range app.LayoutAlgorithms {
		ld, err := layout.FindAlgorithm(n)
		if err != nil {
			return nil, err
		}
		app.StorageAlgorithms[i] = ld
	}
	app.StackLogger = util.NewStackLogger(args.Log)
	if app.DebugREI {
		rei.Enabled = true
		rei.SetGlobalArena(string(app.DebugREIPath))
	}
	for _, c := range components {
		c.Init(app)
	}
	return app, nil
}

// Start the application
func (app *AppCtx) Start() error {
	appMutex.Lock()
	defer appMutex.Unlock()
	ctx := context.Background()
	app.Service.SetState(util.ServiceStarting)
	err := app.getInstanceMD()
	if err == nil {
		if err = app.getClusterMD(ctx); err == nil {
			if app.ClientAPI, err = mgmtclient.NewAPI(app.GetAPIArgs()); err == nil {
				app.OCrud = crud.NewClient(app.ClientAPI, app.Log)
				err = app.startMetricTx()
			}
		}
	}
	if err != nil {
		return err
	}
	app.StackLogger.Start()
	app.Service.SetServiceIP(app.ClusterMD[cluster.CMDServiceIP])
	app.Service.SetServiceLocator(app.ClusterMD[cluster.CMDServiceLocator])
	for _, c := range components {
		c.Start()
	}
	// Service state set to READY after cluster state is reloaded (in vreq)
	return nil
}

type exitFn func(int)

var appExitHook exitFn = os.Exit

// Stop the application
func (app *AppCtx) Stop() {
	appMutex.Lock()
	defer appMutex.Unlock()
	app.Service.SetState(util.ServiceStopping)
	app.CrudeOps.TerminateAllWatchers()
	for _, c := range components {
		c.Stop()
	}
	app.MetricMover.StopTransmitter()
	app.StackLogger.Stop()
	app.Service.SetState(util.ServiceStopped)
	if app.StateUpdater != nil {
		app.StateUpdater.UpdateState(context.Background())
	}
	if app.InFatalError {
		appExitHook(1)
	}
}

func (app *AppCtx) getClusterMD(ctx context.Context) error {
	var err error
	if app.ClusterClient == nil {
		app.ClusterClient, err = cluster.NewClient(app.ClusterType)
		if err != nil {
			return err
		}
	}
	app.ClusterClient.SetDebugLogger(app.Log)
	app.ClusterClient.SetTimeout(app.ClientTimeout)
	app.ClusterMD, err = app.ClusterClient.MetaData(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (app *AppCtx) getInstanceMD() error {
	var err error
	if app.CSP == nil {
		if app.CSP, err = csp.NewCloudServiceProvider(models.CspDomainTypeMutable(app.CSPDomainType)); err != nil {
			return err
		}
	}
	app.CSP.LocalInstanceMetadataSetTimeout(app.ClientTimeout)
	if app.InstanceMD, err = app.CSP.LocalInstanceMetadata(); err != nil {
		return err
	}
	app.Log.Debugf("Instance Metadata: %#v", app.InstanceMD)
	return nil
}

// GetAPIArgs returns the management client arguments
func (app *AppCtx) GetAPIArgs() *mgmtclient.APIArgs {
	return &mgmtclient.APIArgs{
		Host:              app.ManagementHost,
		Port:              app.ManagementPort,
		TLSCertificate:    string(app.Server.TLSCertificate),
		TLSCertificateKey: string(app.Server.TLSCertificateKey),
		TLSCACertificate:  string(app.Server.TLSCACertificate),
		TLSServerName:     app.SSLServerName,
		Insecure:          app.SSLSkipVerify,
		Debug:             app.DebugClient,
		Log:               app.Log,
	}
}

// getMetricMoverTxArgs returns metric transmitter arguments
func (app *AppCtx) getMetricMoverTxArgs() *mm.TxArgs {
	return &mm.TxArgs{
		API:                 app.ClientAPI, // cached
		RetryInterval:       app.MetricRetryInterval,
		VolumeIOMaxBuffered: app.MetricVolumeIOMax,
	}
}

func (app *AppCtx) startMetricTx() error {
	var err error
	if err = app.MetricMover.ConfigureTransmitter(app.getMetricMoverTxArgs()); err == nil {
		err = app.MetricMover.StartTransmitter()
	}
	return err
}

// InitializeCSPClient validates domain attributes and establishes the CSP client endpoint
func (app *AppCtx) InitializeCSPClient(dObj *models.CSPDomain) error {
	if app.CSPClientReady {
		return nil
	}
	var err error
	if err = app.CSP.InDomain(dObj.CspDomainAttributes, app.InstanceMD); err != nil {
		return err
	}
	if app.CSPClient, err = app.CSP.Client(dObj); err != nil {
		return err
	}
	app.CSPClientReady = true
	return nil
}

// FatalError asynchronously terminates the application. Safe to use from components.
func (app *AppCtx) FatalError(err error) {
	appMutex.Lock()
	defer appMutex.Unlock()
	app.FatalErrorCount++
	if app.InFatalError {
		return
	}
	app.Log.Criticalf("Fatal error: %s", err.Error())
	app.InFatalError = true
	go util.PanicLogger(app.Log, app.Stop)
}

// GetServicePlan returns a ServicePlan via a cache
func (app *AppCtx) GetServicePlan(ctx context.Context, spID string) (*models.ServicePlan, error) {
	appMutex.Lock()
	if sp, ok := app.servicePlanMap[spID]; ok {
		appMutex.Unlock()
		return sp, nil
	}
	appMutex.Unlock()
	sp, err := app.OCrud.ServicePlanFetch(ctx, spID)
	if err != nil {
		return nil, err
	}
	appMutex.Lock()
	if app.servicePlanMap == nil {
		app.servicePlanMap = make(map[string]*models.ServicePlan)
	}
	app.servicePlanMap[spID] = sp
	appMutex.Unlock()
	return sp, nil
}

// IsReady is a predicate that is true when clusterd thinks it is ready
func (app *AppCtx) IsReady() bool {
	return app.Service.GetState() == util.ServiceReady
}
