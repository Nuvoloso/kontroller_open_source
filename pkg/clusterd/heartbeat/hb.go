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
	"strings"
	"sync"
	"time"

	mcl "github.com/Nuvoloso/kontroller/pkg/autogen/client/cluster"
	mcsp "github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/system"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	"github.com/Nuvoloso/kontroller/pkg/clusterd/state"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/strfmt"
	"github.com/op/go-logging"
	uuid "github.com/satori/go.uuid"
)

// HBComp is used to manage this handler component
type HBComp struct {
	app                      *clusterd.AppCtx
	Log                      *logging.Logger
	createdUpMon             bool
	mObj                     *models.Cluster
	updateCount              int
	mCspDomainObj            *models.CSPDomain
	sleepPeriod              time.Duration
	stopPeriod               time.Duration
	heartbeatTaskPeriod      time.Duration
	heartbeatTaskPeriodSecs  int64
	lastHeartbeatTaskTime    time.Time
	lastHealthInsertionCount int
	lastVersionLogTime       time.Time
	clusterIdentifier        string
	systemID                 string
	clusterVersion           string
	runCount                 int
	fatalError               bool
	ntd                      terminatedNodeHandler
	worker                   util.Worker
	mux                      sync.Mutex
}

var errClusterMismatch = fmt.Errorf("cluster mismatch")
var errClusterNotClaimed = fmt.Errorf("unable to claim cluster")
var errClusterStateUnexpected = fmt.Errorf("cluster in unexpected state")
var errDomainMismatch = fmt.Errorf("csp domain mismatch")
var errDomainNotFound = fmt.Errorf("csp domain not found")
var errServiceInit = fmt.Errorf("service initialization error")
var errSystemMismatch = fmt.Errorf("system identifier mismatch")
var errSecretExists = fmt.Errorf("cluster identifier secret exists error")
var errUnableToSaveSecret = fmt.Errorf("unable to save cluster identifier secret")

func newHBComp() *HBComp {
	hc := &HBComp{}
	return hc
}

func init() {
	clusterd.AppRegisterComponent(newHBComp())
}

// Init registers handlers for this component
func (c *HBComp) Init(app *clusterd.AppCtx) {
	c.app = app
	c.Log = app.Log
	c.sleepPeriod = time.Duration(app.HeartbeatPeriod) * time.Second
	if app.HeartbeatTaskPeriodMultiplier < 2 {
		app.HeartbeatTaskPeriodMultiplier = 2 // hard coded minimum
	}
	c.heartbeatTaskPeriod = c.sleepPeriod * time.Duration(app.HeartbeatTaskPeriodMultiplier)
	c.heartbeatTaskPeriodSecs = int64(c.heartbeatTaskPeriod / time.Second)
	c.stopPeriod = 10 * time.Second
	c.app.AppObjects = c   // provide the interface
	c.app.StateUpdater = c // provide the interface
	c.ntd = newTerminatedNodeHandler(c)
	wa := &util.WorkerArgs{
		Name:             "Heartbeat",
		Log:              c.Log,
		SleepInterval:    c.sleepPeriod,
		TerminationDelay: c.stopPeriod,
	}
	c.worker, _ = util.NewWorker(wa, c)
}

// Start starts this component
func (c *HBComp) Start() {
	c.Log.Info("Starting Heartbeat")
	c.clusterVersion = c.app.ClusterMD[cluster.CMDVersion]
	if c.app.ClusterID == "" { // TBD : this is deprecated.
		c.clusterIdentifier = c.app.ClusterMD[cluster.CMDIdentifier] // this is deprecated
	}
	c.runCount = 0
	c.worker.Start()
}

// Stop terminates this component
func (c *HBComp) Stop() {
	c.worker.Stop()
}

// GetCluster returns the Cluster object once available
func (c *HBComp) GetCluster() *models.Cluster {
	return c.mObj
}

// GetCspDomain returns the CSPDomain object once available
// NOTE: can be used only to provide object ID but NEVER the credentials as they may change!
func (c *HBComp) GetCspDomain() *models.CSPDomain {
	return c.mCspDomainObj
}

// UpdateState implements the StateUpdater interface
func (c *HBComp) UpdateState(ctx context.Context) error {
	c.mux.Lock()
	defer c.mux.Unlock()
	if c.mObj == nil {
		return fmt.Errorf("not ready")
	}
	return c.updateClusterObj(ctx)
}

// HeartbeatTaskPeriodSecs implements the StateUpdater interface
func (c *HBComp) HeartbeatTaskPeriodSecs() int64 {
	return c.heartbeatTaskPeriodSecs
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
		c.app.Log.Errorf("systemID mismatch: Expected[%s] Got[%s]", c.app.SystemID, c.systemID)
		err = errSystemMismatch
	}
	return err
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
	c.mObj = res.Payload[0]
	c.Log.Infof("Found Cluster [%s,%s] ⇒ %s", c.app.CSPDomainID, c.clusterIdentifier, c.mObj.Meta.ID)
	return nil
}

// createClusterObj attempts to create the cluster object.
// The service state and cluster version are not set because of the current API restrictions on create.
func (c *HBComp) createClusterObj(ctx context.Context) error {
	cl := &models.ClusterCreateArgs{
		ClusterCreateOnce: models.ClusterCreateOnce{
			ClusterType: c.app.ClusterType,
			CspDomainID: models.ObjIDMutable(c.app.CSPDomainID),
		},
		ClusterCreateMutable: models.ClusterCreateMutable{
			ClusterAttributes: c.clusterAttrs(),
			Name:              models.ObjName(c.clusterIdentifier),
			ClusterIdentifier: c.clusterIdentifier,
			State:             com.ClusterStateManaged,
		},
	}
	if c.app.ClusterName != "" {
		cl.Name = models.ObjName(c.app.ClusterName)
	}
	params := mcl.NewClusterCreateParams()
	params.Payload = cl
	params.SetContext(ctx)
	clAPI := c.app.ClientAPI.Cluster()
	c.Log.Debugf("Creating Cluster [%s,%s]", c.app.CSPDomainID, c.clusterIdentifier)
	res, err := clAPI.ClusterCreate(params)
	if err != nil {
		if e, ok := err.(*mcl.ClusterCreateDefault); ok {
			if e.Code() == http.StatusConflict { // object exists
				alt := ""
				if c.app.ClusterName != "" {
					alt = " or a cluster named [" + c.app.ClusterName + "]"
				}
				c.Log.Infof("Cluster [%s,%s]%s already exists", c.app.CSPDomainID, c.clusterIdentifier, alt)
				return c.findClusterObj(ctx)
			}
		}
		return err
	}
	c.mObj = res.Payload
	c.updateCount = 0 // force update to set service state
	c.Log.Infof("Created Cluster [%s,%s] ⇒ %s", c.app.CSPDomainID, c.clusterIdentifier, c.mObj.Meta.ID)
	return nil
}

// updateClusterObj sets the identifier, version and service state.
// It transitions the Cluster object to the ACTIVE state if necessary,
// at the same time ClusterUsagePolicy becomes immutable (inherited flag gets set to false).
// The Cluster object may be modified externally so the code uses the Updater pattern.
func (c *HBComp) updateClusterObj(ctx context.Context) error {
	items := &crud.Updates{}
	items.Set = []string{"clusterVersion", "service", "clusterAttributes"}
	modifyFn := func(o *models.Cluster) (*models.Cluster, error) {
		if o == nil {
			o = c.mObj
		}
		o.ClusterIdentifier = c.clusterIdentifier
		o.ClusterVersion = c.clusterVersion
		o.Service = c.app.Service.ModelObj()
		o.ClusterAttributes = c.clusterAttrs()
		if o.State != com.ClusterStateManaged {
			msgList := util.NewMsgList(o.Messages)
			msgList.Insert("State change: %s ⇒ %s", o.State, com.ClusterStateManaged)
			o.Messages = msgList.ToModel()
			if !util.Contains(items.Set, "state") {
				items.Set = append(items.Set, "state", "messages")
			}
			if o.State == com.ClusterStateDeployable && !util.Contains(items.Set, "clusterUsagePolicy") {
				items.Set = append(items.Set, "clusterUsagePolicy")
			}
			o.State = com.ClusterStateManaged
			o.ClusterUsagePolicy.Inherited = false
		}
		return o, nil
	}
	clObj, err := c.app.OCrud.ClusterUpdater(ctx, string(c.mObj.Meta.ID), modifyFn, items)
	if err != nil {
		c.updateCount = 0
		return err // no logging here
	}
	c.mObj = clObj
	c.updateCount++
	c.Log.Debugf("Updated Cluster [%s,%s] %d", c.app.CSPDomainID, c.clusterIdentifier, c.updateCount)
	return nil
}

func (c *HBComp) issueConsolidatedHeartbeat(ctx context.Context) (*models.Task, error) {
	if c.app.StateOps == nil {
		return nil, nil
	}
	// prune stale health records and get the latest value of the cumulative insertion counter
	now := time.Now()
	numPurged, insCnt := c.app.StateOps.NodePurgeMissing(now)
	if numPurged == 0 && insCnt == c.lastHealthInsertionCount && now.Sub(c.lastHeartbeatTaskTime) < c.heartbeatTaskPeriod {
		return nil, nil // no change and not scheduled
	}
	hbt := strfmt.DateTime(now) // current time
	clID := string(c.mObj.Meta.ID)
	o := &models.Task{}
	o.ObjectID = models.ObjIDMutable(clID)
	o.Operation = com.TaskClusterHeartbeat
	ssm := map[string]models.ServiceState{}
	service := c.app.Service.ModelObj()
	service.HeartbeatTime = hbt
	ssm[clID] = service.ServiceState // cluster hb period + current time
	numN, numRN := 0, 0
	for _, nh := range c.app.StateOps.NodeGetServiceState("") {
		ss := models.ServiceState{
			HeartbeatPeriodSecs: c.heartbeatTaskPeriodSecs, // task period
			HeartbeatTime:       hbt,                       // current time
			State:               nh.CookedState(now),
		}
		ssm[nh.NodeID] = ss
		numN++
		if ss.State == com.ServiceStateReady {
			numRN++
		}
	}
	o.ServiceStates = ssm
	c.Log.Debugf("Creating consolidated heartbeat task: #N=%d #RN=%d #P=%d #I=%d", numN, numRN, numPurged, insCnt)
	task, err := c.app.OCrud.TaskCreate(ctx, o)
	if err != nil {
		c.Log.Errorf("Failed to create consolidated heartbeat task: %s", err.Error())
	} else {
		c.Log.Debugf("Created consolidated heartbeat task: %s", task.Meta.ID)
		c.lastHeartbeatTaskTime = time.Now()
		c.lastHealthInsertionCount = insCnt
	}
	return task, err
}

func (c *HBComp) clusterAttrs() map[string]models.ValueType {
	ca := make(map[string]models.ValueType, len(c.app.ClusterMD))
	for n, v := range c.app.ClusterMD {
		ca[n] = models.ValueType{Kind: "STRING", Value: v}
	}
	// merge in cluster IMD properties
	for n, v := range c.app.InstanceMD {
		if strings.HasPrefix(n, csp.IMDProvisioningPrefix) {
			ca[n] = models.ValueType{Kind: "STRING", Value: v}
		}
	}
	return ca
}

// logVersion periodically logs the app version
func (c *HBComp) logVersion() {
	now := time.Now()
	if now.Sub(c.lastVersionLogTime) >= c.app.VersionLogPeriod {
		c.Log.Info(c.app.InvocationArgs)
		c.lastVersionLogTime = now
	}
}

// getSavedClusterIdentifier get the clusterIdentifier stored in a cluster secret
func (c *HBComp) getSavedClusterIdentifier(ctx context.Context) {
	sfa := &cluster.SecretFetchArgs{
		Name:      c.app.AppArgs.ClusterIdentifierSecretName,
		Namespace: c.app.AppArgs.ClusterNamespace,
	}
	secret, err := c.app.ClusterClient.SecretFetchMV(ctx, sfa)
	if err == nil {
		c.clusterIdentifier = secret.Data[com.K8sClusterIdentitySecretKey]
	}
}

// createAndSaveClusterIdentifier saves the clusterIdentifier as a secret in the cluster
func (c *HBComp) createAndSaveClusterIdentifier(ctx context.Context) error {
	clusterIdentifier := uuid.NewV4().String()
	sca := &cluster.SecretCreateArgsMV{
		Name:      c.app.AppArgs.ClusterIdentifierSecretName,
		Namespace: c.app.AppArgs.ClusterNamespace,
		Data: map[string]string{
			com.K8sClusterIdentitySecretKey: clusterIdentifier,
		},
		Intent: cluster.SecretIntentGeneral,
	}
	if _, err := c.app.ClusterClient.SecretCreateMV(ctx, sca); err != nil {
		c.Log.Warningf("unable to save clusterIdentifier: %s", err.Error())
		if e, ok := err.(cluster.Error); ok && e.ObjectExists() {
			return errSecretExists
		}
		return errUnableToSaveSecret
	}
	c.clusterIdentifier = clusterIdentifier
	return nil
}

// claimClusterObj will fetch the cluster object and update it with the clusterIdentifier
func (c *HBComp) claimClusterObj(ctx context.Context) error {
	items := &crud.Updates{}
	errIgnored := fmt.Errorf("ignore error")
	var clObj2 *models.Cluster
	modifyFn := func(o *models.Cluster) (*models.Cluster, error) {
		if o == nil {
			return nil, nil
		}
		if string(o.CspDomainID) != c.app.CSPDomainID || o.ClusterType != c.app.ClusterType {
			c.app.Log.Criticalf("Cluster [%s] mismatch: (%s,%s) != (%s,%s)", c.app.ClusterID,
				o.CspDomainID, o.ClusterType,
				c.app.CSPDomainID, c.app.ClusterType)
			return nil, errClusterMismatch
		}
		if o.State == com.ClusterStateDeployable {
			o.ClusterIdentifier = c.clusterIdentifier
			msgList := util.NewMsgList(o.Messages)
			msgList.Insert("State change: %s ⇒ %s", o.State, com.ClusterStateManaged)
			o.Messages = msgList.ToModel()
			o.State = com.ClusterStateManaged
			items.Set = append(items.Set, "clusterIdentifier", "state", "messages")
			return o, nil
		} else if o.State == com.ClusterStateManaged || o.State == com.ClusterStateTimedOut {
			if o.ClusterIdentifier != c.clusterIdentifier {
				c.app.Log.Criticalf("Cluster [%s] identifier mismatch: %s != %s", c.app.ClusterID,
					o.ClusterIdentifier, c.clusterIdentifier)
				return nil, errClusterMismatch
			}
			if o.State == com.ClusterStateTimedOut {
				msgList := util.NewMsgList(o.Messages)
				msgList.Insert("State change: %s ⇒ %s", o.State, com.ClusterStateManaged)
				o.Messages = msgList.ToModel()
				o.State = com.ClusterStateManaged
				items.Set = append(items.Set, "state", "messages")
				return o, nil
			}
			clObj2 = o
			return nil, errIgnored
		}
		c.app.Log.Criticalf("Cluster [%s] in unexpected state (%s)", c.app.ClusterID, o.State)
		return nil, errClusterStateUnexpected
	}
	clObj, err := c.app.OCrud.ClusterUpdater(ctx, string(c.app.ClusterID), modifyFn, items)
	if err != nil && err != errIgnored {
		if err != errClusterMismatch && err != errClusterStateUnexpected {
			c.Log.Errorf("cluster not claimed: %s", err.Error())
			err = errClusterNotClaimed
		}
		return err
	}
	if clObj2 != nil {
		clObj = clObj2
	}
	c.mObj = clObj
	c.Log.Debugf("Claimed Cluster [%s,%s]", c.app.CSPDomainID, c.clusterIdentifier)
	return nil
}

// Buzz satisfies the util.WorkerBee interface
func (c *HBComp) Buzz(ctx context.Context) error {
	if c.fatalError {
		return fmt.Errorf("fatal error")
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
	if err == nil && c.clusterIdentifier == "" {
		c.getSavedClusterIdentifier(ctx)
	}
	if err == nil && c.clusterIdentifier == "" {
		err = c.createAndSaveClusterIdentifier(ctx)
	}
	if err == nil && c.mObj == nil && c.app.ClusterID != "" { // TBD: c.app.ClusterID will become required
		err = c.claimClusterObj(ctx)
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
	c.mux.Lock() // serialize with UpdateState
	defer c.mux.Unlock()
	if err == nil && c.mObj == nil { // DEPRECATED
		if err = c.findClusterObj(ctx); err == nil && c.mObj.Service != nil {
			err = c.app.Service.Import(c.mObj.Service)
		}
		if err != nil && err.Error() == "cluster not found" {
			c.Log.Debug("Ignoring cluster not found")
			err = nil
		}
	}
	if err == nil {
		if c.mObj == nil { // DEPRECATED
			err = c.createClusterObj(ctx)
		}
		if c.mObj != nil {
			if c.updateCount == 0 {
				err = c.updateClusterObj(ctx)
			} else {
				var retT *models.Task
				if retT, err = c.issueConsolidatedHeartbeat(ctx); err == nil && retT == nil {
					err = c.updateClusterObj(ctx) // no task: issue cluster heartbeat
				}
			}
		}
		if c.mObj != nil && c.app.StateOps == nil {
			sa := &state.ClusterStateArgs{
				OCrud:            c.app.OCrud,
				Log:              c.Log,
				Cluster:          c.mObj,
				CSP:              c.app.CSP,
				LayoutAlgorithms: c.app.StorageAlgorithms,
			}
			c.Log.Debug("Creating StateOps")
			c.app.StateOps = state.New(sa)
			if c.app.StateOps != nil {
				c.Log.Debug("StateOps ready")
			}
		}
	}
	if err == nil && c.mObj != nil && !c.createdUpMon {
		if err = c.app.CrudeOps.MonitorUpstream(c.app.ClientAPI, c.app.AppServant.GetAPIArgs(), c.getUpstreamMonitorArgs()); err == nil {
			c.createdUpMon = true
		}
	}
	if err == nil && c.app.IsReady() {
		c.app.ClusterClient.RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceReady})
	} else {
		c.app.ClusterClient.RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady})
	}
	if err == nil && c.mObj != nil {
		c.ntd.handleTerminatedNodes(ctx)
		c.updateMetricMoverStats()
		c.updateCrudeStatistics()
	}
	c.logVersion()
	return err
}

// getUpstreamMonitorArgs returns arguments for the upstream monitor
// Requires existential object meta-data.
func (c *HBComp) getUpstreamMonitorArgs() *models.CrudWatcherCreateArgs {
	reqScopePat := fmt.Sprintf("clusterId:%s", c.mObj.Meta.ID)
	return &models.CrudWatcherCreateArgs{
		Name: c.clusterIdentifier,
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{
				MethodPattern: "POST|PATCH",
				URIPattern:    "^/(volume-series|storage)-requests/?",
				ScopePattern:  reqScopePat,
			},
		},
	}
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
