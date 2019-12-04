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


package agentd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/agentd/state"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	mm "github.com/Nuvoloso/kontroller/pkg/metricmover"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	"github.com/Nuvoloso/kontroller/pkg/pstore"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
	"github.com/jessevdk/go-flags"
	"github.com/op/go-logging"
)

// SloRPODefault is the default value for the RPO SLO
const SloRPODefault = time.Duration(4 * time.Hour)

// SnapshotSafetyBufferMultiplier is a fraction of the RPO that serves as the minimum
// duration required for a snapshot and the inter-snapshot period.
const SnapshotSafetyBufferMultiplier = float64(0.125)

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
	ClusterType                string           `long:"cluster-type" description:"Specify the type of cluster" default:"kubernetes"`
	ClientTimeout              int              `long:"client-timeout" description:"Specify the timeout duration in seconds for client operations" default:"20"`
	CSISocket                  flags.Filename   `short:"K" long:"csi-socket" description:"The pathname of the CSI sidecar socket"`
	ClusterID                  string           `short:"C" long:"cluster-id" description:"The Cluster object ID"`
	CSPDomainType              string           `short:"T" long:"csp-domain-type" description:"The type of cloud service provider" default:"AWS"`
	CSPDomainID                string           `short:"D" long:"csp-domain-id" description:"The associated CSPDomain object identifier" required:"yes"`
	SystemID                   string           `short:"S" long:"system-id" description:"The associated System object identifier" required:"yes"`
	ManagementHost             string           `short:"M" long:"management-host" description:"The hostname or address of the central management service" required:"yes"`
	ManagementPort             int              `short:"P" long:"management-port" description:"The port of the central management service" default:"443"`
	SSLServerName              string           `long:"ssl-server-name" description:"The actual server name of the management service SSL certificate, defaults to the --management-host value"`
	SSLSkipVerify              bool             `short:"k" long:"ssl-skip-verify" description:"Allow unverified upstream service certificate or host name"`
	MetricUploadRetryInterval  time.Duration    `long:"metric-upload-retry-interval" description:"The period between metric upload retries" default:"30s"`
	MetricVolumeIOMax          int              `long:"metric-volume-io-max" description:"The maximum number of volume I/O metrics that will be buffered" default:"100"`
	MetricStorageIOMax         int              `long:"metric-storage-io-max" description:"The maximum number of storage I/O metrics that will be buffered" default:"50"`
	MetricIoCollectionPeriod   time.Duration    `long:"metric-io-collection-period" description:"Periodic volume and device I/O metric collection period" default:"5m"`
	MetricIoCollectionTruncate time.Duration    `long:"metric-io-collection-period-truncate" description:"The value of the periodic I/O metric timer is truncated to this duration" default:"5m"`
	HeartbeatPeriod            int              `long:"heartbeat-period" description:"The heartbeat period in seconds" default:"30"`
	VersionLogPeriod           time.Duration    `long:"version-log-period" description:"The period for logging build version information" default:"30m"`
	NuvoStartupPollPeriod      int              `long:"nuvo-startup-poll-period" description:"The poll period in seconds while nuvo starts" default:"2"`
	NuvoStartupPollMax         int              `long:"nuvo-startup-poll-max" description:"The maximum number of times to poll at the higher rate after which the normal polling period will resume" default:"30"`
	MaxMessages                int              `long:"max-messages" description:"The maximum number of service messages to maintain"`
	NuvoSocket                 flags.Filename   `short:"N" long:"nuvo-socket" description:"The pathname of the Nuvo data management daemon socket" default:"/var/run/nuvoloso/nuvo.sock"`
	NuvoFVIni                  flags.Filename   `long:"nuvo-fv-ini" description:"The path to the nuvoloso flexVolume driver INI file" env:"NUVO_FV_INI"`
	NuvoPort                   int              `long:"nuvo-port" description:"The port number of the Nuvo data management daemon" default:"32145"`
	NuvoVolDirPath             flags.Filename   `long:"nuvo-vol-dir" description:"The directory containing Nuvo volumes" default:"/var/local/nuvoloso/luns"`
	DebugClient                bool             `long:"debug-client" description:"Log client HTTP requests and responses"`
	DebugREI                   bool             `long:"debug-permit-rei" description:"Permit runtime error injection"`
	DebugREIPath               flags.Filename   `long:"debug-rei-dir" description:"Runtime error injection arena" default:"/var/run/nuvoloso/rei"`
	CrudeOps                   crude.Ops        `json:"-"`
	Log                        *logging.Logger  `json:"-"`
	API                        *ops.NuvolosoAPI `json:"-"`
	Server                     *restapi.Server  `no-flag:"yes" json:"-"` // Has the TLS config
	ServiceVersion             string
	NuvoAPIVersion             string
	InvocationArgs             string `json:"-"`
}

// AppServant is an interface for application methods exposed to components.
type AppServant interface {
	AddLUN(lun *LUN)
	AddStorage(storage *Storage)
	FatalError(error)
	FindLUN(vsID string, snapID string) *LUN
	FindStorage(storageID string) *Storage
	GetCentraldAPIArgs() *mgmtclient.APIArgs
	GetClusterdAPI() (mgmtclient.API, error)
	GetClusterdAPIArgs() *mgmtclient.APIArgs
	GetLUNs() []*LUN
	GetServicePlan(ctx context.Context, spID string) (*models.ServicePlan, error)
	GetStorage() []*Storage
	InitializeCSPClient(dObj *models.CSPDomain) error
	RemoveLUN(vsID string, snapID string)
	RemoveStorage(storageID string)
	InitializeNuvo(ctx context.Context) error
	IsReady() bool
}

// StateUpdater updates agentd state
type StateUpdater interface {
	// Refresh forces a re-evaluation of agentd state
	Refresh()
	// UpdateState updates the state of the service in the node object
	UpdateState(ctx context.Context) error
}

// AppObjects provides access to significant objects when available
type AppObjects interface {
	// GetNode returns the Node object once available
	GetNode() *models.Node
	// GetCluster returns the Cluster object once available
	GetCluster() *models.Cluster
	// GetCspDomain returns the CSPDomain object once available
	GetCspDomain() *models.CSPDomain
	// UpdateNodeLocalStorage updates the Node object
	UpdateNodeLocalStorage(ctx context.Context, ephemeral map[string]models.NodeStorageDevice, cacheUnitSizeBytes int64) (*models.Node, error)
}

// AppRecoverStorage is an interface used to recover previously used Storage state in agentd and nuvo.
// The methods are invoked while holding the application mutex.
type AppRecoverStorage interface {
	// RecoverStorageInUse is called on agentd startup to tell nuvo of devices previously in use. It returns the new content of the storage map
	RecoverStorageInUse(ctx context.Context) ([]*Storage, error)
	// ConfigureStorage is called on nuvo restart to tell nuvo to use the specified devices
	ConfigureStorage(ctx context.Context, stg *Storage, sObj *models.Storage) error
	// RecoverStorageLocations is called on nuvo restart to tell Nuvo of the locations of the specified devices previously in use by volumes on this node
	RecoverStorageLocations(ctx context.Context, stgIDs []string) error
	// StorageRecovered is called on completion of Storage recovery
	StorageRecovered()
}

// AppRecoverVolume is an interface used to recover the state of Volumes after a nuvo restart.
// The methods are invoked while holding the application mutex.
type AppRecoverVolume interface {
	// CheckNuvoVolumes checks to see if there are any volumes on the node; if none exist, it calls NoVolumesInUse.
	// case [vol-recover:nuvo-restarted-agentd-restarted]  (https://goo.gl/3tMWq3)
	CheckNuvoVolumes(ctx context.Context) error
	// NoVolumesInUse is part of the AppRecoverVolume interface.
	// It finds all previously configured volumes (IN_USE, CONFIGURED) then deletes all local mounts and changes the state to PROVISIONED.
	// It returns a list of all Storage objects used by the volumes previously in use on the node - the list may contain duplicates.
	// It releases cache used by volume and updates cache state to reflect the change.
	// See https://goo.gl/3tMWq3:
	//  - case [vol-recover:nuvo-restarted-agentd-ok]
	//  - case [vol-recover:nuvo-restarted-agentd-restarted]
	NoVolumesInUse(ctx context.Context) ([]string, error)
	// RecoverVolumesInUse is invoked after initialing Nuvo.
	// It loads the lun cache with local volumes that are IN_USE and removes non-head luns.
	// case [vol-recover:nuvo-ok-agentd-restarted] (https://goo.gl/3tMWq3)
	RecoverVolumesInUse(ctx context.Context) ([]*LUN, error)
	// VolumesRecovered is called on completion of Volume recovery
	VolumesRecovered()
}

// MetricReporter is an interface cause statistical data to be reported
type MetricReporter interface {
	// ReportVolumeMetric reports statistics on the HEAD lun of a specific volume series
	ReportVolumeMetric(ctx context.Context, vsID string) error
}

// LUN describes an exposed VolumeSeries interface.
// At least the key fields must be set when the object is inserted into the lunMap cache.
// Components may update other fields as long as appropriate synchronization is used.
type LUN struct {
	VolumeSeriesID          string // key field
	SnapIdentifier          string // key field
	NuvoVolumeIdentifier    string
	DeviceName              string
	LastReadStat            *nuvoapi.StatsIO
	LastWriteStat           *nuvoapi.StatsIO
	LastWriteStatTime       time.Time
	LastEffectiveSlope      float64
	TotalWriteBytes         util.SizeBytes
	TotalWriteDuration      time.Duration
	RequestedCacheSizeBytes int64 // actual desired size according to Storage Plan
	AllocatedCacheSizeBytes int64 // actual allocated size
	LastCacheUserStat       *nuvoapi.StatsCache
	LastCacheMetaStat       *nuvoapi.StatsCache
	DisableMetrics          bool
}

// Validate checks for required fields
func (l *LUN) Validate() error {
	if l.VolumeSeriesID == "" || l.SnapIdentifier == "" {
		return fmt.Errorf("missing required LUN fields")
	}
	return nil
}

// Storage describes a Storage object in use.
// At least the key fields must be set when the object is inserted into the lunMap cache.
// Components may update other fields as long as appropriate synchronization is used.
type Storage struct {
	StorageID      string // key field
	DeviceName     string
	CspStorageType models.CspStorageType
	LastReadStat   *nuvoapi.StatsIO
	LastWriteStat  *nuvoapi.StatsIO
}

// Validate checks for required fields
func (s *Storage) Validate() error {
	if s.StorageID == "" || s.DeviceName == "" || s.CspStorageType == "" {
		return fmt.Errorf("missing required Storage fields")
	}
	return nil
}

// AppCtx contains the runtime context of the agent daemon.
type AppCtx struct {
	AppArgs
	Service        util.Controller // Keeps the state
	ClusterClient  cluster.Client
	ClusterMD      map[string]string
	ClientAPI      mgmtclient.API // centrald API
	OCrud          crud.Ops       // centrald API
	ClusterdAPI    mgmtclient.API // clusterd supports a small subset of API calls
	ClusterdOCrud  crud.Ops       // clusterd supports a small subset of API calls
	CSP            csp.CloudServiceProvider
	CSPClient      csp.DomainClient
	InstanceMD     map[string]string
	CSPDomainAttrs map[string]models.ValueType
	CSPClientReady bool
	NuvoAPI        nuvoapi.NuvoVM
	MetricMover    mm.MetricMover
	PSO            pstore.Operations
	LMDGuard       *util.CriticalSectionGuard // for lifecycle management data updates
	AppServant
	AppObjects                                 // implemented by heartbeat
	StateUpdater                               // implemented by heartbeat
	StateOps          state.NodeStateOperators // initialized by heartbeat
	AppRecoverStorage                          // implemented by sreq
	AppRecoverVolume                           // implemented by vreq
	MetricReporter                             // implemented by metrics
	StackLogger       util.StackLogger
	InFatalError      bool // tests can set this to avoid exiting
	FatalErrorCount   int
	lunMap            map[string]*LUN // key is vsID:snapID
	storageMap        map[string]*Storage
	servicePlanMap    map[string]*models.ServicePlan
	reiSP             *rei.EphemeralPropertyManager // for service plans
	nuvoInitCount     int
	nuvoConfigured    bool
	psoArgs           *pstore.ControllerArgs
	recState          *recoveryState
	clientAPITimeout  time.Duration
}

type recoveryState struct {
	volStorageIDs map[string]struct{} // hash of storageIds referenced by volumes
}

// AppInit installs the backend installerRegistry for the Nuvoloso API implemented
// by this server and returns a context object
func AppInit(args *AppArgs) *AppCtx {
	app := &AppCtx{
		AppArgs: *args,
	}
	app.AppServant = app // self-reference
	app.clientAPITimeout = time.Second * time.Duration(app.ClientTimeout)
	svcArgs := util.ServiceArgs{
		ServiceType:         "agentd",
		ServiceVersion:      app.ServiceVersion,
		HeartbeatPeriodSecs: int64(app.HeartbeatPeriod),
		Log:                 app.Log,
		MaxMessages:         app.MaxMessages,
		ServiceAttributes:   map[string]models.ValueType{},
	}
	app.Service = util.NewService(&svcArgs)
	app.LMDGuard = util.NewCriticalSectionGuard()
	app.MetricMover = mm.NewMetricMover(args.Log)
	app.StackLogger = util.NewStackLogger(args.Log)
	if app.DebugREI {
		rei.Enabled = true
		rei.SetGlobalArena(string(app.DebugREIPath))
	}
	app.reiSP = rei.NewEPM("sp")
	app.reiSP.Enabled = app.DebugREI
	for _, c := range components {
		c.Init(app)
	}
	return app
}

// Start the application
func (app *AppCtx) Start() error {
	appMutex.Lock()
	defer appMutex.Unlock()
	ctx := context.Background()
	app.Service.SetState(util.ServiceStarting)
	app.NuvoAPI = nuvoapi.NewNuvoVMWithNotInitializedAlerter(string(app.NuvoSocket), app)
	err := app.getInstanceMD()
	if err == nil {
		if err = app.getClusterMD(ctx); err == nil {
			if app.ClientAPI, err = mgmtclient.NewAPI(app.GetCentraldAPIArgs()); err == nil {
				app.OCrud = crud.NewClient(app.ClientAPI, app.Log)
				if _, err = app.GetClusterdAPI(); err == nil { // clusterdAPI gets cached
					app.ClusterdOCrud = crud.NewClient(app.ClusterdAPI, app.Log)
					err = app.startMetricTx()
				}
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
	// app.Service.SetState(util.ServiceReady) READY when Nuvo is initialized
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
	// Needs context
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

// getMetricMoverTxArgs returns metric transmitter arguments
func (app *AppCtx) getMetricMoverTxArgs() *mm.TxArgs {
	return &mm.TxArgs{
		API:                  app.ClusterdAPI, // cached
		RetryInterval:        app.MetricUploadRetryInterval,
		VolumeIOMaxBuffered:  app.MetricVolumeIOMax,
		StorageIOMaxBuffered: app.MetricStorageIOMax,
	}
}

func (app *AppCtx) startMetricTx() error {
	var err error
	if err = app.MetricMover.ConfigureTransmitter(app.getMetricMoverTxArgs()); err == nil {
		err = app.MetricMover.StartTransmitter()
	}
	return err
}

// GetCentraldAPIArgs returns the API args for centrald on the management host.
func (app *AppCtx) GetCentraldAPIArgs() *mgmtclient.APIArgs {
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

// GetClusterdAPIArgs returns the API args for clusterd
func (app *AppCtx) GetClusterdAPIArgs() *mgmtclient.APIArgs {
	args := app.GetCentraldAPIArgs()
	h := os.Getenv("CLUSTERD_SERVICE_HOST")
	p := os.Getenv("CLUSTERD_SERVICE_PORT")
	n := os.Getenv("CLUSTERD_SSL_SERVER_NAME") // not required, not a k8s environment variable
	if h == "" || p == "" {
		app.Log.Error("Missing CLUSTERD service environment variables")
		return args
	}
	app.Log.Debugf("CLUSTERD service: %s:%s (SSL Server Name:%s)", h, p, n)
	args.Host = h
	args.Port, _ = strconv.Atoi(p)
	args.TLSServerName = n // never allow it to fall back to the value used for centrald
	return args
}

// GetClusterdAPI returns an API for clusterd
func (app *AppCtx) GetClusterdAPI() (mgmtclient.API, error) {
	if app.ClusterdAPI != nil {
		return app.ClusterdAPI, nil
	}
	var err error
	app.ClusterdAPI, err = mgmtclient.NewAPI(app.GetClusterdAPIArgs())
	return app.ClusterdAPI, err
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
	app.fatalError(err)
}

func (app *AppCtx) fatalError(err error) {
	app.FatalErrorCount++
	if app.InFatalError {
		return
	}
	app.Log.Criticalf("Fatal error: %s", err.Error())
	app.InFatalError = true
	go util.PanicLogger(app.Log, app.Stop)
}

// TBD move all of the scattered meaningful error constants to a common place, perhaps even nuvoapi itself
// or replace with some nuvoapi Error interface that has meaningful errors
const (
	// ErrNodeUUIDNotSet is returned by Storelandia if UseNodeUuid has not yet been called
	ErrNodeUUIDNotSet = "BAD_ORDER: Not allowed before use node UUID command"
	// ErrWrongNodeUUID is returned by Storelandia if UseNodeUuid is called with the wrong UUID
	ErrWrongNodeUUID = "ERROR: Operation not permitted"
)

// IsReady is a predicate that is true when agentd thinks it is capable of processing Storage and VolumeSeries requests.
func (app *AppCtx) IsReady() bool {
	return app.Service.GetState() == util.ServiceReady
}

// NuvoNotInitializedError satisfies the nuvoapi.NotInitializedAlerter interface
func (app *AppCtx) NuvoNotInitializedError(err error) {
	if app.IsReady() {
		app.StateUpdater.Refresh()
	}
}

// InitializeNuvo sets the Node UUID on the nuvo service via the NuvoAPI. Safe to use from components.
// It also handles re-initialization of nuvo in case of restart.
// If an error is returned, IsReady() will return false until a subsequent invocation of this function returns without an error.
//
// The recovery logic is split into 3 sections and some cases (https://goo.gl/3tMWq3) are spread over multiple sections:
//  1. Before setting the node uuid
//      - case [vol-recover:nuvo-restarted-agentd-restarted]
//      - case [vol-recover:nuvo-restarted-agentd-ok]
//  2. Set the node uuid
//  3. After setting the node uuid
//      - Inform Nuvo of the available cache devices [cache:nuvo-use-cache]
//      - case [vol-recover:nuvo-ok-agentd-restarted]
//      - case [vol-recover:nuvo-restarted-agentd-ok]
// It is important to note that errors can arise at any point and this sub will be retried until it succeeds.
// This means that state must be cached in a context that persists across this recovery process.
func (app *AppCtx) InitializeNuvo(ctx context.Context) error {
	appMutex.Lock()
	defer appMutex.Unlock()
	app.Log.Warning("Agentd is NOT_READY")
	app.ClusterClient.RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady})
	curState := app.Service.GetState()
	app.Service.SetState(util.ServiceNotReady) // force a reconfigure
	var node *models.Node
	if app.AppObjects != nil {
		node = app.GetNode()
	}
	if node == nil {
		return errors.New("node UUID is not yet known, try again later")
	}
	var err error
	if curState != util.ServiceNotReady {
		if err = app.StateUpdater.UpdateState(ctx); err != nil {
			return err
		}
	}
	if app.recState == nil {
		app.recState = &recoveryState{}
		app.recState.volStorageIDs = make(map[string]struct{})
	}
	rs := app.recState
	if app.nuvoInitCount == 0 { // agentd initializing
		if err = app.CheckNuvoVolumes(ctx); err != nil {
			return err
		}
	} else { // nuvo restarted beneath agentd
		sl, err := app.NoVolumesInUse(ctx)
		if err != nil {
			return err
		}
		// collect previous storage object references to restore locations later
		for _, stgID := range sl {
			app.Log.Debugf("Found reference to Storage [%s]", stgID)
			rs.volStorageIDs[stgID] = struct{}{}
		}
		app.Log.Debug("Clearing LUN cache")
		app.lunMap = nil
	}
	////////////////////////////////////////////////////////////
	// pre-nuvo-init above
	////////////////////////////////////////////////////////////
	app.Log.Debugf("NUVOAPI [%d] UseNodeUUID(%s)", app.nuvoInitCount, node.Meta.ID)
	if err = app.NuvoAPI.UseNodeUUID(string(node.Meta.ID)); err != nil {
		app.Log.Warningf("NUVOAPI UseNodeUUID(%s): %s", node.Meta.ID, err.Error())
		return err
	}
	app.Log.Infof("Successfully set nuvo service node UUID [%s]", node.Meta.ID)
	////////////////////////////////////////////////////////////
	// post-nuvo-init below
	////////////////////////////////////////////////////////////
	if node, err = app.nuvoUseCacheDevices(ctx, node); err != nil {
		return err
	}
	if app.nuvoInitCount == 0 { // agentd initializing
		// load storage cache from db (devices on this node that are OPEN) and tell nuvo of devices from storage cache
		var stgList []*Storage
		var lunList []*LUN
		stgList, err = app.RecoverStorageInUse(ctx)
		if err == nil {
			// IN_USE volumes: load lun cache from db and unexport non-HEAD mounts; CONFIGURED volumes untouched
			lunList, err = app.RecoverVolumesInUse(ctx)
		}
		if err != nil {
			return err
		}
		app.storageMap = nil
		for _, stg := range stgList {
			app.addStorage(stg)
		}
		app.lunMap = nil
		for _, lun := range lunList {
			app.addLUN(lun)
		}

	} else { // nuvo restarts beneath agentd
		// TBD: determine if there is a race condition with remote Nuvo processes
		// tell nuvo of devices from storage cache
		for _, s := range app.storageMap {
			if err = app.ConfigureStorage(ctx, s, nil); err != nil {
				return err
			}
		}
		// tell nuvo of device locations previously referenced by volumes
		if err = app.RecoverStorageLocations(ctx, util.StringKeys(rs.volStorageIDs)); err != nil {
			return err
		}
	}
	if app.PSO == nil { // PSO creation delayed because we want to pass it a working nuvo
		if app.PSO, err = pstore.NewController(app.getPSOArgs()); err != nil {
			err = fmt.Errorf("unable to create PSO: %s", err.Error())
			app.fatalError(err)
			return err
		}
	}
	app.recState = nil // clear the state
	app.nuvoInitCount++
	app.Log.Info("Agentd is now READY")
	app.ClusterClient.RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceReady})
	app.Service.SetState(util.ServiceReady)
	app.StateUpdater.UpdateState(ctx) // ignore error
	app.StorageRecovered()            // tell sreq to restart
	app.VolumesRecovered()            // tell vreq to restart
	return nil
}

// nuvoUseCacheDevices is an un-exported helper for InitializeNuvo to inform Nuvo of the available cache devices.
// It must be called only after the NodeUUID is set. The node object may be updated as a side-effect.
func (app *AppCtx) nuvoUseCacheDevices(ctx context.Context, node *models.Node) (*models.Node, error) {
	changed := false
	var cacheUnitSizeBytes int64
	// sort by UUID: Storelandia may not use all the devices and forcing the order ensures consistent behavior
	for _, uuid := range util.SortedStringKeys(node.LocalStorage) {
		dev := node.LocalStorage[uuid]
		if dev.DeviceState == common.NodeDevStateCache || dev.DeviceState == common.NodeDevStateUnused {
			app.Log.Debugf("NUVOAPI UseCacheDevice(%s, %s)", uuid, dev.DeviceName)
			usableSizeBytes, cuSizeBytes, err := app.NuvoAPI.UseCacheDevice(uuid, dev.DeviceName)
			if err != nil {
				app.Log.Warningf("NUVOAPI UseCacheDevice(%s, %s) failed: %s", uuid, dev.DeviceName, err.Error())
				return nil, err
			}
			if swag.Int64Value(dev.UsableSizeBytes) != int64(usableSizeBytes) {
				dev.UsableSizeBytes = swag.Int64(int64(usableSizeBytes))
				changed = true
			}
			if usableSizeBytes == 0 {
				if dev.DeviceState != common.NodeDevStateUnused {
					dev.DeviceState = common.NodeDevStateUnused
					changed = true
				}
			} else if dev.DeviceState != common.NodeDevStateCache {
				dev.DeviceState = common.NodeDevStateCache
				changed = true
			}
			cacheUnitSizeBytes = int64(cuSizeBytes)
			app.Log.Debugf("NUVOAPI UseCacheDevice(%s, %s) succeeded: usableSizeBytes:%d cacheUnitSizeBytes:%d", uuid, dev.DeviceName, usableSizeBytes, cacheUnitSizeBytes)
			if changed {
				node.LocalStorage[uuid] = dev // dev is a copy, need to update the value
			}
		}
		if cacheUnitSizeBytes != swag.Int64Value(node.CacheUnitSizeBytes) {
			changed = true
		}
	}
	if changed {
		return app.UpdateNodeLocalStorage(ctx, node.LocalStorage, cacheUnitSizeBytes)
	}
	return node, nil
}

func (app *AppCtx) getPSOArgs() *pstore.ControllerArgs {
	if app.psoArgs == nil {
		app.psoArgs = &pstore.ControllerArgs{
			NuvoAPI:  app.NuvoAPI,
			Log:      app.Log,
			DebugREI: app.DebugREI,
		}
	}
	return app.psoArgs
}

// AddLUN caches the reference to the exposed LUN if not already present
func (app *AppCtx) AddLUN(lun *LUN) {
	appMutex.Lock()
	defer appMutex.Unlock()
	app.addLUN(lun)
}

func (app *AppCtx) addLUN(lun *LUN) {
	if err := lun.Validate(); err != nil {
		panic(fmt.Sprintf("Attempt to insert invalid LUN: %v", lun))
	}
	k := lun.VolumeSeriesID + ":" + lun.SnapIdentifier
	if app.lunMap == nil {
		app.lunMap = make(map[string]*LUN)
	}
	if _, ok := app.lunMap[k]; !ok {
		app.lunMap[k] = lun
	}
}

// RemoveLUN removes the reference to a LUN
func (app *AppCtx) RemoveLUN(vsID string, snapID string) {
	appMutex.Lock()
	defer appMutex.Unlock()
	k := vsID + ":" + snapID
	delete(app.lunMap, k)
}

// GetLUNs returns the list of exposed LUNs
func (app *AppCtx) GetLUNs() []*LUN {
	appMutex.Lock()
	defer appMutex.Unlock()
	la := make([]*LUN, len(app.lunMap))
	i := 0
	for _, l := range app.lunMap {
		la[i] = l
		i++
	}
	return la
}

// FindLUN returns a specific LUN or nil
func (app *AppCtx) FindLUN(vsID string, snapID string) *LUN {
	appMutex.Lock()
	defer appMutex.Unlock()
	k := vsID + ":" + snapID
	if l, ok := app.lunMap[k]; ok {
		return l
	}
	return nil
}

// AddStorage caches the reference to the exposed Storage if not already present
func (app *AppCtx) AddStorage(storage *Storage) {
	appMutex.Lock()
	defer appMutex.Unlock()
	app.addStorage(storage)
}

func (app *AppCtx) addStorage(storage *Storage) {
	if err := storage.Validate(); err != nil {
		panic(fmt.Sprintf("Attempt to insert invalid Storage: %v", storage))
	}
	if app.storageMap == nil {
		app.storageMap = make(map[string]*Storage)
	}
	if _, ok := app.storageMap[storage.StorageID]; !ok {
		app.storageMap[storage.StorageID] = storage
	}
}

// RemoveStorage removes the reference to a Storage
func (app *AppCtx) RemoveStorage(storageID string) {
	appMutex.Lock()
	defer appMutex.Unlock()
	delete(app.storageMap, storageID)
}

// GetStorage returns the list of exposed Storage
func (app *AppCtx) GetStorage() []*Storage {
	appMutex.Lock()
	defer appMutex.Unlock()
	sa := make([]*Storage, len(app.storageMap))
	i := 0
	for _, s := range app.storageMap {
		sa[i] = s
		i++
	}
	return sa
}

// FindStorage returns a specific Storage or nil
func (app *AppCtx) FindStorage(storageID string) *Storage {
	appMutex.Lock()
	defer appMutex.Unlock()
	if s, ok := app.storageMap[storageID]; ok {
		return s
	}
	return nil
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

// GetRPO is a helper that returns the parsed RPO slo of a service plan, subject to REI
func (app *AppCtx) GetRPO(sp *models.ServicePlan) time.Duration {
	var vt models.ValueType
	vt = sp.Slos["RPO"].ValueType
	if app.DebugREI {
		if s := app.reiSP.GetString(string(sp.Name) + "-rpo"); s != "" {
			vt.Value = s
		}
	}
	return util.ParseValueType(vt, common.ValueTypeDuration, SloRPODefault).(time.Duration)
}

// ContextWithClientTimeout returns a context with the client timeout applied
func (app *AppCtx) ContextWithClientTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if app.clientAPITimeout > 0 {
		return context.WithTimeout(ctx, app.clientAPITimeout)
	}
	return context.WithCancel(ctx)
}
