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


package heartbeat

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	"github.com/Nuvoloso/kontroller/pkg/agentd/state"
	mcl "github.com/Nuvoloso/kontroller/pkg/autogen/client/cluster"
	mcsp "github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_domain"
	mn "github.com/Nuvoloso/kontroller/pkg/autogen/client/node"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/system"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	"github.com/Nuvoloso/kontroller/pkg/nuvofv"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
	"github.com/jessevdk/go-flags"
	"github.com/op/go-logging"
	uuid "github.com/satori/go.uuid"
)

// HBComp is used to manage this handler component
type HBComp struct {
	app                *agentd.AppCtx
	Log                *logging.Logger
	createdUpMon       bool
	mObj               *models.Node
	updateCount        int
	mClusterObj        *models.Cluster
	mCspDomainObj      *models.CSPDomain
	ephemeralDevices   []*csp.EphemeralDevice
	sleepPeriod        time.Duration
	nuvoPollPeriod     time.Duration
	nuvoPollMaxCount   int
	nuvoPollCount      int
	stopPeriod         time.Duration
	lastHeartbeatTime  time.Time
	lastVersionLogTime time.Time
	clusterIdentifier  string
	clusterID          string
	systemID           string
	nodeIdentifier     string
	worker             util.Worker
	runCount           int
	fatalError         bool
	nuvoNodeUUIDSet    bool
	fvIniUpdated       bool
	mux                sync.Mutex
}

var errClusterMismatch = fmt.Errorf("cluster mismatch")
var errClusterNotFound = fmt.Errorf("cluster not found")
var errDomainMismatch = fmt.Errorf("csp domain mismatch")
var errDomainNotFound = fmt.Errorf("csp domain not found")
var errServiceInit = fmt.Errorf("service initialization error")
var errSystemMismatch = fmt.Errorf("system identifier mismatch")

// InitialRetryInterval is the initial retry period until READY
const InitialRetryInterval = time.Second

func newHBComp() *HBComp {
	hc := &HBComp{}
	return hc
}

func init() {
	agentd.AppRegisterComponent(newHBComp())
}

// Init registers handlers for this component
func (c *HBComp) Init(app *agentd.AppCtx) {
	c.app = app
	c.Log = app.Log
	c.sleepPeriod = time.Duration(app.HeartbeatPeriod) * time.Second
	c.nuvoPollPeriod = time.Duration(app.NuvoStartupPollPeriod) * time.Second
	c.nuvoPollMaxCount = app.NuvoStartupPollMax
	c.stopPeriod = 10 * time.Second
	c.app.AppObjects = c   // provide the interface
	c.app.StateUpdater = c // provide the interface
	wa := &util.WorkerArgs{
		Name:             "Heartbeat",
		Log:              c.Log,
		SleepInterval:    InitialRetryInterval,
		TerminationDelay: c.stopPeriod,
	}
	c.worker, _ = util.NewWorker(wa, c)
}

// Start starts this component
func (c *HBComp) Start() {
	if c.app.ClusterID == "" { // TBD : this is deprecated
		c.clusterIdentifier = c.app.ClusterMD[cluster.CMDIdentifier]
	}
	c.nodeIdentifier = c.app.InstanceMD[csp.IMDInstanceName]
	c.runCount = 0
	c.worker.Start()
}

// Stop terminates this component
func (c *HBComp) Stop() {
	c.worker.Stop()
}

// Refresh satisfies the agentd.StateUpdater interface
func (c *HBComp) Refresh() {
	c.Log.Debug("Refresh called")
	c.worker.Notify()
}

// UpdateState satisfies the agentd.StateUpdater interface
func (c *HBComp) UpdateState(ctx context.Context) error {
	c.mux.Lock()
	defer c.mux.Unlock()
	c.Log.Debug("UpdateState called")
	if c.mObj == nil {
		return fmt.Errorf("not ready")
	}
	c.lastHeartbeatTime = time.Time{} // force it
	return c.updateNodeObj(ctx)
}

// GetNode returns the Node object once available
func (c *HBComp) GetNode() *models.Node {
	return c.mObj
}

// GetCluster returns the Cluster object once available
func (c *HBComp) GetCluster() *models.Cluster {
	return c.mClusterObj
}

// GetCspDomain returns the CSPDomain object once available
// NOTE: can be used only to provide object ID but NEVER the credentials as they may change!
func (c *HBComp) GetCspDomain() *models.CSPDomain {
	return c.mCspDomainObj
}

// UpdateNodeLocalStorage updates the Node object with the new local storage values
func (c *HBComp) UpdateNodeLocalStorage(ctx context.Context, ephemeral map[string]models.NodeStorageDevice, cacheUnitSizeBytes int64) (*models.Node, error) {
	c.mux.Lock()
	defer c.mux.Unlock()
	c.Log.Debug("UpdateNodeLocalStorage called")
	if c.mObj == nil {
		return nil, fmt.Errorf("not ready")
	}
	c.mObj.LocalStorage = ephemeral
	c.mObj.CacheUnitSizeBytes = swag.Int64(cacheUnitSizeBytes)
	c.lastHeartbeatTime, c.updateCount = time.Time{}, 0 // force it
	if err := c.updateNodeObj(ctx); err != nil {
		return nil, err
	}
	return c.mObj, nil
}

// fetchCSPDomainObj loads the CSPDomain object
func (c *HBComp) fetchCSPDomainObj(ctx context.Context) error {
	params := mcsp.NewCspDomainFetchParams()
	params.ID = c.app.CSPDomainID
	params.SetContext(ctx)
	var res *mcsp.CspDomainFetchOK
	var err error
	cspAPI := c.app.ClientAPI.CspDomain()
	c.Log.Debugf("Fetching CSPDomain [%s]", c.app.CSPDomainID)
	if res, err = cspAPI.CspDomainFetch(params); err != nil {
		c.Log.Warningf("CSPDomain fetch: %s", err.Error())
		return errDomainNotFound
	}
	c.mCspDomainObj = res.Payload
	c.Log.Infof("Found CSPDomain [%s] ⇒ %s,%s", c.app.CSPDomainID, c.mCspDomainObj.Name, c.mCspDomainObj.CspDomainType)
	return nil
}

// importCSPDomain validates the values of the CSPDomain object against empirical data and updates the app
func (c *HBComp) importCSPDomain() error {
	if string(c.mCspDomainObj.CspDomainType) != c.app.CSPDomainType {
		c.Log.Errorf("expected DomainType (%s) does not match CSPDomain object (%s,%s)", c.app.CSPDomainType, c.app.CSPDomainID, c.mCspDomainObj.CspDomainType)
		return errDomainMismatch
	}
	if err := c.app.AppServant.InitializeCSPClient(c.mCspDomainObj); err != nil {
		c.Log.Criticalf("InitializeCSPClient: %s", err.Error())
		return errServiceInit
	}
	return nil
}

// findSystemObj looks up the system object to get the system ID
func (c *HBComp) findSystemObj(ctx context.Context) error {
	params := system.NewSystemFetchParams()
	params.SetContext(ctx)
	var res *system.SystemFetchOK
	var err error
	api := c.app.ClientAPI.System()
	c.Log.Debugf("Fetching System ID")
	if res, err = api.SystemFetch(params); err != nil {
		c.Log.Warningf("System fetch: %s", err.Error())
		return err
	}
	c.systemID = string(res.Payload.Meta.ID)
	c.Log.Infof("Found System ID [%s]", c.systemID)
	if c.systemID != c.app.SystemID {
		c.Log.Criticalf("systemID mismatch: Expected[%s] Got[%s]", c.app.SystemID, c.systemID)
		return errSystemMismatch
	}
	return err
}

// fetchClusterObj loads the cluster object identified by c.app.ClusterID.
func (c *HBComp) fetchClusterObj(ctx context.Context) error {
	c.Log.Debugf("Fetching Cluster [%s]", c.app.ClusterID)
	var err error
	var clObj *models.Cluster
	if clObj, err = c.app.OCrud.ClusterFetch(ctx, c.app.ClusterID); err != nil {
		if e, ok := err.(*crud.Error); ok && e.NotFound() {
			err = errClusterNotFound
		}
		return err
	}
	if string(clObj.CspDomainID) != c.app.CSPDomainID || clObj.ClusterType != c.app.ClusterType {
		c.app.Log.Criticalf("Cluster [%s] mismatch: (%s,%s) != (%s,%s)", c.app.ClusterID,
			clObj.CspDomainID, clObj.ClusterType,
			c.app.CSPDomainID, c.app.ClusterType)
		return errClusterMismatch
	}
	c.mClusterObj = clObj
	c.clusterID = string(clObj.Meta.ID)
	return nil
}

// findClusterObj looks for the cluster object by (DomainID, ClusterIdentifier)
func (c *HBComp) findClusterObj(ctx context.Context) error {
	params := mcl.NewClusterListParams()
	params.CspDomainID = &c.app.CSPDomainID
	params.ClusterIdentifier = &c.clusterIdentifier
	params.SetContext(ctx)
	var res *mcl.ClusterListOK
	var err error
	clAPI := c.app.ClientAPI.Cluster()
	c.Log.Debugf("Searching for Cluster [%s,%s]", c.app.CSPDomainID, c.clusterIdentifier)
	if res, err = clAPI.ClusterList(params); err != nil {
		c.Log.Warningf("Cluster list: %s", err.Error())
		return err
	}
	if len(res.Payload) == 0 {
		return fmt.Errorf("cluster not found")
	}
	c.clusterID = string(res.Payload[0].Meta.ID)
	c.mClusterObj = res.Payload[0]
	c.Log.Infof("Found Cluster [%s,%s] ⇒ %s", c.app.CSPDomainID, c.clusterIdentifier, c.clusterID)
	return nil
}

// findNodeObj looks for the node object by (ClusterID, NodeIdentifier)
func (c *HBComp) findNodeObj(ctx context.Context) error {
	params := mn.NewNodeListParams()
	params.ClusterID = &c.clusterID
	params.NodeIdentifier = &c.nodeIdentifier
	var res *mn.NodeListOK
	var err error
	ctx2, cf := c.app.ContextWithClientTimeout(ctx)
	defer cf()
	c.Log.Debugf("Fetching Node [%s] via clusterd", c.nodeIdentifier)
	if res, err = c.app.ClusterdOCrud.NodeList(ctx2, params); err != nil {
		if e, ok := err.(*crud.Error); ok && e.Payload.Code == http.StatusServiceUnavailable {
			c.Log.Warningf("Node list: %s (%d)", err.Error(), e.Payload.Code)
			c.setSleepInterval(c.nuvoPollPeriod) // retry faster
		} else {
			c.Log.Warningf("Node list: %s", err.Error())
		}
		return err
	}
	c.setSleepInterval(c.sleepPeriod) // restore normal period
	if len(res.Payload) == 0 {
		return fmt.Errorf("node not found")
	}
	c.mObj = res.Payload[0]
	c.Log.Debugf("Found Node [%s] ⇒ %s (%s)", c.nodeIdentifier, c.mObj.Meta.ID, c.mObj.State)
	if c.mObj.State == com.NodeStateTearDown { // in the process of being ejected
		c.mObj = nil
		return fmt.Errorf("node is being torn down")
	}
	return nil
}

// createNodeObj attempts to create the node object.
func (c *HBComp) createNodeObj(ctx context.Context) error {
	nca := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			NodeIdentifier: c.nodeIdentifier,
			ClusterID:      models.ObjIDMutable(c.clusterID),
		},
		NodeMutable: models.NodeMutable{
			Name:           models.ObjName(c.nodeIdentifier),
			NodeAttributes: c.nodeAttrs(),
			Service:        c.app.Service.ModelObj(),
			State:          com.NodeStateManaged,
		},
	}
	if hostname, present := c.app.InstanceMD[csp.IMDHostname]; present {
		nca.Name = models.ObjName(hostname)
	}
	c.updateLocalStorage(nca)
	ctx2, cf := c.app.ContextWithClientTimeout(ctx)
	defer cf()
	c.Log.Debugf("Creating Node [%s,%s] via clusterd", c.clusterID, c.nodeIdentifier)
	res, err := c.app.ClusterdOCrud.NodeCreate(ctx2, nca)
	if err != nil {
		if e, ok := err.(*crud.Error); ok {
			if e.Exists() { // object exists
				c.Log.Info("Node [%s,%s] already exists", c.clusterID, c.nodeIdentifier)
				return c.findNodeObj(ctx)
			}
		}
		return err
	}
	c.mObj = res
	c.updateCount = 1
	c.Log.Infof("Created Node [%s,%s] ⇒ %s", c.clusterID, c.nodeIdentifier, c.mObj.Meta.ID)
	return nil
}

func (c *HBComp) canPostHeartbeat(now time.Time) bool {
	if now.Sub(c.lastHeartbeatTime) < c.sleepPeriod {
		return false
	}
	return true
}

// updateNodeObj sets the service state in the node object
func (c *HBComp) updateNodeObj(ctx context.Context) error {
	now := time.Now()
	if !c.canPostHeartbeat(now) {
		return nil
	}
	c.mObj.State = com.NodeStateManaged
	c.mObj.Service = c.app.Service.ModelObj()
	c.mObj.NodeAttributes = c.nodeAttrs()
	items := &crud.Updates{Set: []string{"state", "service", "nodeAttributes"}}
	if c.updateCount == 0 {
		c.updateLocalStorage(c.mObj)
		items.Set = append(items.Set, "localStorage", "totalCacheBytes", "cacheUnitSizeBytes") // availableCacheBytes is auto-updated in the handler
		var totalCache int64
		for _, mDev := range c.mObj.LocalStorage {
			if mDev.DeviceState == com.NodeDevStateCache {
				totalCache += swag.Int64Value(mDev.UsableSizeBytes)
			}
		}
		c.mObj.TotalCacheBytes = swag.Int64(totalCache)
	}
	ctx2, cf := c.app.ContextWithClientTimeout(ctx)
	defer cf()
	nObj, err := c.app.ClusterdOCrud.NodeUpdate(ctx2, c.mObj, items)
	if err != nil {
		if e, ok := err.(*crud.Error); ok && e.NotFound() {
			msg := fmt.Sprintf("[%s,%s] object deleted (ID=%s)", c.clusterID, c.nodeIdentifier, string(c.mObj.Meta.ID))
			c.app.Service.Message("Node%s", msg)
			c.mObj = nil // next pass will attempt to recreate
			c.clusterID = ""
			err = fmt.Errorf("node%s: %s", msg, *e.Payload.Message)
		}
		c.updateCount = 0
		return err // no log here
	}
	c.lastHeartbeatTime = now
	c.mObj = nObj
	if c.app.StateOps != nil {
		c.app.StateOps.UpdateNodeObj(ctx, nObj)
	}
	c.updateCount++
	c.Log.Debugf("Updated Node [%s,%s] ver=%d", c.clusterID, c.nodeIdentifier, c.mObj.Meta.Version)
	return nil
}

// updateNodeHeartbeatOnly sends only heartbeat data on a best effort basis
// The node object is not updated as this call is processed by clusterd only.
func (c *HBComp) updateNodeHeartbeatOnly(ctx context.Context) error {
	now := time.Now()
	if !c.canPostHeartbeat(now) {
		return nil
	}
	c.mObj.Service = c.app.Service.ModelObj()
	items := &crud.Updates{Set: []string{"service.state", "service.heartbeatTime", "service.heartbeatPeriodSecs"}}
	ctx2, cf := c.app.ContextWithClientTimeout(ctx)
	defer cf()
	_, err := c.app.ClusterdOCrud.NodeUpdate(ctx2, c.mObj, items)
	if err == nil {
		c.lastHeartbeatTime = now
	}
	return err
}

func (c *HBComp) nodeAttrs() map[string]models.ValueType {
	ca := make(map[string]models.ValueType, len(c.app.InstanceMD))
	for n, v := range c.app.InstanceMD {
		ca[n] = models.ValueType{Kind: "STRING", Value: v}
	}
	return ca
}

// update the LocalStorage in the node, returns true iff any changes were made
func (c *HBComp) updateLocalStorage(node *models.Node) bool {
	changed := false
	if len(c.ephemeralDevices) > 0 && node.LocalStorage == nil {
		node.LocalStorage = map[string]models.NodeStorageDevice{}
	}

DeleteOrUpdateDevices:
	for _, uuid := range util.StringKeys(node.LocalStorage) {
		mDev := node.LocalStorage[uuid]
		for _, dev := range c.ephemeralDevices {
			if mDev.DeviceName == dev.Path {
				if (mDev.DeviceState != com.NodeDevStateRestricted) != dev.Usable ||
					mDev.DeviceType != dev.Type ||
					swag.Int64Value(mDev.SizeBytes) != dev.SizeBytes {
					if !dev.Usable {
						mDev.DeviceState = com.NodeDevStateRestricted
					} else if mDev.DeviceState == com.NodeDevStateRestricted {
						mDev.DeviceState = com.NodeDevStateUnused
					}
					mDev.DeviceType = dev.Type
					mDev.SizeBytes = swag.Int64(dev.SizeBytes)
					node.LocalStorage[uuid] = mDev
					changed = true
				}
				continue DeleteOrUpdateDevices
			}
		}
		delete(node.LocalStorage, uuid)
		changed = true
	}

AddNewDevices:
	for _, dev := range c.ephemeralDevices {
		for _, mDev := range node.LocalStorage {
			if mDev.DeviceName == dev.Path {
				continue AddNewDevices
			}
		}
		state := com.NodeDevStateUnused
		if !dev.Usable {
			state = com.NodeDevStateRestricted
		}
		node.LocalStorage[uuid.NewV4().String()] = models.NodeStorageDevice{
			DeviceName:  dev.Path,
			DeviceState: state,
			DeviceType:  dev.Type,
			SizeBytes:   swag.Int64(dev.SizeBytes),
		}
		changed = true
	}
	return changed
}

func (c *HBComp) setSleepInterval(desiredSleepInterval time.Duration) {
	if desiredSleepInterval == c.nuvoPollPeriod {
		c.nuvoPollCount++
		if c.nuvoPollCount > c.nuvoPollMaxCount {
			desiredSleepInterval = c.sleepPeriod // restore normal heartbeat period
		}
	} else {
		c.nuvoPollCount = 0
	}
	if c.worker.GetSleepInterval() != desiredSleepInterval {
		c.Log.Debugf("Sleep interval: %s", desiredSleepInterval)
		c.worker.SetSleepInterval(desiredSleepInterval)
	}
}

// initializeNuvo sets the node UUID for the nuvo service (Storelandia) plus other initialization.
func (c *HBComp) initializeNuvo(ctx context.Context) error {
	err := c.app.AppServant.InitializeNuvo(ctx)
	if err != nil {
		c.setSleepInterval(c.nuvoPollPeriod)
		return err
	}
	c.nuvoNodeUUIDSet = true
	return nil
}

func (c *HBComp) pingNuvo(ctx context.Context) error {
	desiredSleepInterval := c.sleepPeriod
	c.Log.Debug("NUVOAPI NodeStatus()")
	nsr, err := c.app.NuvoAPI.NodeStatus()
	if err == nil {
		c.Log.Debugf("NUVOAPI NodeStatus() succeeded: %v", nsr)
	} else {
		c.Log.Debugf("NUVOAPI NodeStatus() failed: [%v, %s]", nsr, err.Error())
	}
	if err == nil || !nuvoapi.ErrorIsTemporary(err) {
		if err != nil || nsr.NodeUUID == "" {
			c.Log.Info("Nuvo requires re-initialization")
			err = c.app.AppServant.InitializeNuvo(ctx)
		}
	}
	if err != nil {
		desiredSleepInterval = c.nuvoPollPeriod
		if nuvoapi.ErrorIsTemporary(err) {
			err = nil
		}
	}
	c.setSleepInterval(desiredSleepInterval)
	return err
}

// updateNuvoFVIni sets properties like NodeID in the flexVolume driver INI file.
// The properties are all constant, so the update is skipped once it has succeeded
func (c *HBComp) updateNuvoFVIni(ctx context.Context) error {
	if c.app.NuvoFVIni == "" || c.fvIniUpdated {
		return nil
	}
	c.Log.Debugf("Updating flexVolume driver INI file [%s]", c.app.NuvoFVIni)
	args := &nuvofv.AppArgs{}
	parser := flags.NewParser(args, flags.Default)
	if e := flags.NewIniParser(parser).ParseFile(string(c.app.NuvoFVIni)); e != nil {
		return e // caller logs
	}
	if args.NodeID == string(c.mObj.Meta.ID) && args.NodeIdentifier == c.nodeIdentifier && args.ClusterID == c.clusterID && args.SystemID == c.systemID {
		c.Log.Debugf("Update flexVolume driver INI file [%s] already up to date", c.app.NuvoFVIni)
		c.fvIniUpdated = true
		return nil
	}
	args.NodeID = string(c.mObj.Meta.ID)
	args.NodeIdentifier = c.nodeIdentifier
	args.ClusterID = c.clusterID
	args.SystemID = c.systemID
	iniP := flags.NewIniParser(parser)
	if e := iniP.WriteFile(string(c.app.NuvoFVIni), flags.IniIncludeDefaults); e != nil {
		return e // caller logs
	}

	c.fvIniUpdated = true
	c.Log.Debugf("Updated flexVolume driver INI file [%s,%s,%s,%s]", c.app.NuvoFVIni, c.mObj.Meta.ID, c.nodeIdentifier, c.mObj.ClusterID)
	return nil
}

// logVersion periodically logs the app version
func (c *HBComp) logVersion() {
	now := time.Now()
	if now.Sub(c.lastVersionLogTime) >= c.app.VersionLogPeriod {
		c.Log.Info(c.app.InvocationArgs)
		c.lastVersionLogTime = now
	}
}

// Buzz satisfies the util.WorkerBee interface
func (c *HBComp) Buzz(ctx context.Context) error {
	if c.fatalError {
		return fmt.Errorf("heartbeat fatal error")
	}
	c.runCount++
	var err error
	if c.systemID == "" {
		err = c.findSystemObj(ctx)
	}
	if err == nil && c.mCspDomainObj == nil {
		if err = c.fetchCSPDomainObj(ctx); err == nil {
			err = c.importCSPDomain()
		}
	}
	if err == nil && c.clusterID == "" && c.app.ClusterID != "" { // TBD: c.app.ClusterID will become required
		err = c.fetchClusterObj(ctx)
	}
	if err != nil {
		c.app.ClusterClient.RecordIncident(ctx, &cluster.Incident{
			Message:  err.Error(),
			Severity: cluster.IncidentFatal,
		})
		c.fatalError = true
		c.app.AppServant.FatalError(err)
		return err
	}
	if err == nil && c.clusterID == "" {
		err = c.findClusterObj(ctx) // DEPRECATED
	}
	if err == nil && c.ephemeralDevices == nil {
		// this call can block, so list the ephemeral devices before taking the lock
		if c.ephemeralDevices, err = c.app.CSPClient.LocalEphemeralDevices(); err != nil {
			c.Log.Warningf("LocalEphemeralDevices list: %s", err.Error())
			c.ephemeralDevices = nil
		} else if c.ephemeralDevices == nil {
			c.ephemeralDevices = []*csp.EphemeralDevice{}
		}
	}
	func() { // serialize with direct calls to update state; need to release mux before potentially lengthy blocking nuvo calls
		c.mux.Lock()
		defer c.mux.Unlock()
		if err == nil && c.mObj == nil && c.clusterID != "" {
			err = c.findNodeObj(ctx)
			if err == nil && c.mObj.Service != nil {
				err = c.app.Service.Import(c.mObj.Service)
			}
			if err != nil && err.Error() == "node not found" {
				c.Log.Debug("Ignoring node not found")
				err = nil
			}
			if err == nil {
				c.setSleepInterval(c.sleepPeriod) // initial period ends
			}
		}
		if err == nil {
			c.updateMetricMoverStats()
			c.updateCrudeStatistics()
			if c.mObj == nil {
				err = c.createNodeObj(ctx)
			} else if c.updateCount == 0 {
				err = c.updateNodeObj(ctx)
			} else {
				err = c.updateNodeHeartbeatOnly(ctx)
			}
			if c.mObj != nil && c.app.StateOps == nil {
				sa := &state.NodeStateArgs{
					OCrud: c.app.OCrud,
					Log:   c.Log,
					Node:  c.mObj,
				}
				c.Log.Debug("Creating StateOps")
				c.app.StateOps = state.New(sa)
				if c.app.StateOps != nil {
					c.Log.Debug("StateOps ready")
				}
			}
		}
		if err == nil && c.mObj != nil && !c.createdUpMon {
			if err = c.makeUpstreamMonitor(); err == nil {
				c.createdUpMon = true
			}
		}
	}()
	if err == nil {
		if !c.nuvoNodeUUIDSet {
			err = c.initializeNuvo(ctx)
		} else {
			err = c.pingNuvo(ctx)
		}
	}
	if err == nil {
		err = c.updateNuvoFVIni(ctx)
	}
	if err == nil {
		c.app.ClusterClient.RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceReady})
	} else {
		c.app.ClusterClient.RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady})
	}
	c.logVersion()
	return err
}

// getUpstreamMonitorArgs returns arguments for the upstream monitor
// Requires existential object meta-data.
func (c *HBComp) getUpstreamMonitorArgs() *models.CrudWatcherCreateArgs {
	reqScopePat := fmt.Sprintf("nodeId:%s", c.mObj.Meta.ID)
	return &models.CrudWatcherCreateArgs{
		Name: fmt.Sprintf("%s", c.nodeIdentifier),
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{
				MethodPattern: "POST|PATCH",
				URIPattern:    "^/(volume-series|storage)-requests/?",
				ScopePattern:  reqScopePat,
			},
			&models.CrudMatcher{
				MethodPattern: "PATCH",
				URIPattern:    "^/volume-series-requests/",
				ScopePattern:  "syncCoordinatorId:", // on any node in this cluster if this is set
			},
			&models.CrudMatcher{
				MethodPattern: "POST",
				URIPattern:    "^/volume-series-requests/.*/cancel", // any cancel in this cluster
			},
		},
	}
}

func (c *HBComp) makeUpstreamMonitor() error {
	clArgs := c.app.AppServant.GetClusterdAPIArgs()
	clAPI, err := c.app.AppServant.GetClusterdAPI()
	if err != nil {
		c.Log.Errorf("Unable to create clusterd API: %s", err.Error())
		return err
	}
	return c.app.CrudeOps.MonitorUpstream(clAPI, clArgs, c.getUpstreamMonitorArgs())
}

func (c *HBComp) updateMetricMoverStats() {
	vt := models.ValueType{
		Kind:  "STRING",
		Value: c.app.MetricMover.Status().String(),
	}
	c.app.Service.SetServiceAttribute(com.ServiceAttrMetricMoverStatus, vt)
}

func (c *HBComp) updateCrudeStatistics() {
	vt := models.ValueType{
		Kind:  "STRING",
		Value: c.app.CrudeOps.GetStats().String(),
	}
	c.app.Service.SetServiceAttribute(com.ServiceAttrCrudeStatistics, vt)
}
