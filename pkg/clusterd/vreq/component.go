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


package vreq

import (
	"context"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	"github.com/Nuvoloso/kontroller/pkg/clusterd/state"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/swag"
	"github.com/op/go-logging"
)

// WatcherRetryMultiplier increases the refresh polling period.
// TBD: increase once we can handle timeout differently
const WatcherRetryMultiplier = 2

// InitialRetryInterval is the initial retry period until READY
const InitialRetryInterval = time.Second

// Args contains settable parameters for this component
type Args struct {
	RetryInterval time.Duration `long:"retry-interval-secs" description:"Request handler retry loop polling interval if not monitoring events. When monitoring events this is increased." default:"30s"`
	StopPeriod    time.Duration `long:"stop-period" description:"Request handler termination duration" default:"10s"`
	DebugREI      bool          `long:"debug-permit-rei" description:"Permit runtime error injection"`
}

// Component handles volume series requests
type Component struct {
	Args
	App                   *clusterd.AppCtx
	Log                   *logging.Logger
	Animator              *vra.Animator
	runCount              int
	fatalError            bool
	startupCleanupDone    bool
	thisClusterID         models.ObjIDMutable
	thisClusterIdentifier string
	oCrud                 crud.Ops
	watcherID             string
	rei                   *rei.EphemeralPropertyManager
}

// ComponentInit must be called from main to initialize and register this component.
func ComponentInit(args *Args) *Component {
	c := &Component{}
	c.Args = *args
	clusterd.AppRegisterComponent(c)
	return c
}

func init() {
	states := append([]string{}, mountDeletePublishStates...)
	states = append(states, com.VolReqStateResizingCache, com.VolReqStateUndoResizingCache)
	states = append(states, com.VolReqStateCGSnapshotVolumes, com.VolReqStateCGSnapshotWait, com.VolReqStateCGSnapshotFinalize)
	states = append(states, com.VolReqStateCreatingFromSnapshot, com.VolReqStateSnapshotRestoreDone)
	states = append(states, com.VolReqStateUndoCGSnapshotVolumes, com.VolReqStateUndoCreatingFromSnapshot)
	states = append(states, com.VolReqStatePublishingServicePlan)
	clusterdStates = states
}

// Init registers handlers for this component
func (c *Component) Init(app *clusterd.AppCtx) {
	c.App = app
	c.Log = app.Log
	c.fatalError = false
	c.Animator = vra.NewAnimator(InitialRetryInterval, c.Log, c).WithAllocationHandlers(c).WithCGSnapshotCreateHandlers(c).WithCreateFromSnapshotHandlers(c).WithPublishHandlers(c).WithPublishServicePlanHandlers(c).WithStopPeriod(c.StopPeriod)
	// no runtime scope data in the watcher so register now
	if wid, err := c.App.CrudeOps.Watch(c.getWatcherArgs(), c.Animator); err == nil {
		c.watcherID = wid
	} else {
		c.Log.Errorf("Failed to create watcher: %s", err.Error())
	}
	c.rei = rei.NewEPM("vreq")
	c.rei.Enabled = c.DebugREI
}

// Start starts this component
func (c *Component) Start() {
	c.Log.Info("Starting VolumeSeriesHandler")
	c.oCrud = crud.NewClient(c.App.ClientAPI, c.Log)
	c.runCount = 0
	c.Animator.Start(c.oCrud, c.App.CrudeOps)
}

// Stop terminates this component
func (c *Component) Stop() {
	c.App.CrudeOps.TerminateWatcher(c.watcherID)
	c.Animator.Stop()
	c.Log.Info("Stopped VolumeSeriesHandler")
}

// getWatcherArgs returns watcher args for this component.
// Note: Only clusterScope events are visible, courtesy of the upstream monitor in hb.
func (c *Component) getWatcherArgs() *models.CrudWatcherCreateArgs {
	return &models.CrudWatcherCreateArgs{
		Name: "VSR",
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{ // new VSR objects
				MethodPattern: "POST",
				URIPattern:    "^/volume-series-requests$",
				ScopePattern:  "requestedOperations:(MOUNT|CG_SNAPSHOT_CREATE|CONFIGURE|CREATE_FROM_SNAPSHOT|PUBLISH|UNPUBLISH)\\b", // 1st op must be an op for this handler
			},
			&models.CrudMatcher{ // VSR activities triggered from upstream or downstream
				MethodPattern: "PATCH",
				URIPattern:    "^/volume-series-requests/",
				ScopePattern:  "volumeSeriesRequestState:(UNDO_)?(SIZING|PLACEMENT(_REATTACH)?|RESIZING_CACHE|CG_SNAPSHOT_FINALIZE|CHOOSING_NODE|SNAPSHOT_RESTORE_DONE|PUBLISHING(_SERVICE_PLAN)?)",
			},
			&models.CrudMatcher{ // terminated StorageRequests for this cluster
				MethodPattern: "PATCH",
				URIPattern:    "^/storage-requests/",
				ScopePattern:  "storageRequestState:(SUCCEEDED|FAILED)",
			},
			&models.CrudMatcher{ // change in node state
				URIPattern:   "^/nodes/?",
				ScopePattern: "serviceStateChanged:true",
			},
			&models.CrudMatcher{ // failure of vs cg operation
				MethodPattern: "PATCH",
				URIPattern:    "^/volume-series-requests/",
				ScopePattern:  "consistencyGroupId:.*volumeSeriesId:[^\\s].*volumeSeriesRequestState:(FAILED|CANCELED)",
			},
			&models.CrudMatcher{
				MethodPattern: "PATCH",
				URIPattern:    "^/volume-series-requests/",
				ScopePattern:  "parentId:.*volumeSeriesRequestState:(FAILED|CANCELED|SUCCEEDED)",
			},
		},
	}
}

// vra.Ops methods

// RunBody is called once for each iteration of the Animator
func (c *Component) RunBody(ctx context.Context) []*models.VolumeSeriesRequest {
	c.runCount++
	if c.thisClusterID == "" {
		if c.App.AppObjects == nil {
			return nil // must wait for AppObjects to be set
		}
		cObj := c.App.AppObjects.GetCluster()
		if cObj == nil {
			return nil // must wait until this Cluster is created/loaded
		}
		c.thisClusterID = models.ObjIDMutable(cObj.Meta.ID)
		c.thisClusterIdentifier = cObj.ClusterIdentifier
	}
	if c.App.StateOps == nil || c.fatalError {
		return nil
	}
	id := "clusterd.vreq"
	c.Log.Debugf("%s: Acquiring state lock", id)
	cst, err := c.App.StateGuard.Enter(id)
	if err != nil {
		c.Log.Errorf("%s: Failed to get state lock: %s", id, err.Error())
		return nil
	}
	defer func() {
		c.Log.Debugf("%s: Releasing state lock", id)
		cst.Leave()
	}()
	c.Log.Debugf("%s: Obtained state lock", id)
	if _, err := c.App.StateOps.Reload(ctx); err != nil {
		c.Log.Warningf("Error determining cluster state: %s", err.Error())
		return nil
	}
	ss := c.App.StateOps.DumpState().String()
	c.Log.Debugf("State of cluster:\n%s", ss)
	c.App.Service.SetServiceAttribute(com.ServiceAttrClusterResourceState, models.ValueType{Kind: com.ValueTypeString, Value: ss})
	if !c.startupCleanupDone {
		c.Log.Info("VolumeSeriesHandler cleanup starting")
		// TBD
		c.startupCleanupDone = true
		c.Log.Info("VolumeSeriesHandler cleanup done")
		c.App.Service.SetState(util.ServiceReady)
		c.Animator.SetRetryInterval(c.RetryInterval * WatcherRetryMultiplier)
	}
	if c.App.IsReady() {
		c.releaseUnusedStorage(ctx)
	}
	lParams := volume_series_request.NewVolumeSeriesRequestListParams()
	lParams.ClusterID = swag.String(string(c.thisClusterID))
	lParams.VolumeSeriesRequestState = clusterdStates
	lParams.IsTerminated = swag.Bool(false)
	if vsrLRet, err := c.oCrud.VolumeSeriesRequestList(ctx, lParams); err == nil {
		c.removeClaimsForTerminatedVSRs(ctx, vsrLRet.Payload)
		return vsrLRet.Payload
	}
	return nil
}

func (c *Component) removeClaimsForTerminatedVSRs(ctx context.Context, activeVSRList []*models.VolumeSeriesRequest) {
	reqIDList := map[string]struct{}{}
	c.App.StateOps.FindClaims(func(cd *state.ClaimData) bool {
		for _, claim := range cd.VSRClaims {
			reqIDList[claim.RequestID] = struct{}{}
		}
		return false
	})
	if len(reqIDList) == 0 {
		return
	}
	for _, vsr := range activeVSRList {
		delete(reqIDList, string(vsr.Meta.ID))
	}
	for reqID := range reqIDList {
		c.Log.Debugf("Removing dead claim from VSR[%s]", reqID)
		c.App.StateOps.RemoveClaims(reqID)
	}
}

func (c *Component) releaseUnusedStorage(ctx context.Context) {
	if err := c.rei.ErrOnBool("disable-release-unused-storage"); err != nil {
		c.Log.Debugf("%s", err.Error())
		return
	}
	c.App.StateOps.ReleaseUnusedStorage(ctx, c.App.ReleaseStorageRequestTimeout)
}

var mountDeletePublishStates = []string{
	com.VolReqStateNew, com.VolReqStateChoosingNode, com.VolReqStateSizing, com.VolReqStateStorageWait, com.VolReqStatePlacement, com.VolReqStatePlacementReattach, com.VolReqStatePublishing,
	com.VolReqStateUndoPlacement, com.VolReqStateUndoSizing, com.VolReqStateUndoPublishing,
}

var clusterdStates []string // derived in init()

// ShouldDispatch is called once for each active request to decide if this handler should dispatch the request for processing
func (c *Component) ShouldDispatch(ctx context.Context, rhs *vra.RequestHandlerState) bool {
	ap := vra.GetStateInfo(rhs.Request.VolumeSeriesRequestState).GetProcess(rhs.Request.RequestedOperations[0])
	if ap == vra.ApClusterd {
		return true
	}
	c.Log.Debugf("Skipping VolumeSeriesRequest[%s] (%v, %s) processed by %s", rhs.Request.Meta.ID, rhs.Request.RequestedOperations, rhs.Request.VolumeSeriesRequestState, ap)
	return false
}
