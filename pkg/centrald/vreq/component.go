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
	"sync"
	"time"

	mcvr "github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
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

// Args contains settable parameters for this component
type Args struct {
	RetryInterval        time.Duration `long:"retry-interval-secs" description:"Request handler retry loop polling interval if not monitoring events. When monitoring events this is increased." default:"30s"`
	DebugREI             bool          `long:"debug-permit-rei" description:"Permit runtime error injection"`
	VSRPurgeTaskInterval time.Duration `long:"vsr-purge-task-interval" description:"The interval for running task to purge outdated VSRs" default:"12h"`
	VSRRetentionPeriod   time.Duration `long:"vsr-retention-period" description:"The retention period for outdated VSRs" default:"168h"` // 7d
}

// Component handles volume series requests
type Component struct {
	Args
	App                *centrald.AppCtx
	Log                *logging.Logger
	Animator           *vra.Animator
	runCount           int
	fatalError         bool
	startupCleanupDone bool
	oCrud              crud.Ops
	reservationCS      *util.CriticalSectionGuard
	watcherID          string
	vsrTaskID          string
	rei                *rei.EphemeralPropertyManager
	lastPurgeTaskTime  time.Time
	mux                sync.Mutex
}

// ComponentInit must be called from main to initialize and register this component.
func ComponentInit(args *Args) *Component {
	c := &Component{}
	c.Args = *args
	centrald.AppRegisterComponent(c)
	return c
}

// Init registers handlers for this component
func (c *Component) Init(app *centrald.AppCtx) {
	c.App = app
	c.Log = app.Log
	c.fatalError = false
	c.Animator = vra.NewAnimator(WatcherRetryMultiplier*c.RetryInterval, c.Log, c).
		WithCreateHandlers(c).WithBindHandlers(c).WithRenameHandlers(c).
		WithAllocateCapacityHandlers(c).WithChangeCapacityHandlers(c).
		WithNodeDeleteHandlers(c).WithVolumeDetachHandlers(c)
	if wid, err := c.App.CrudeOps.Watch(c.getWatcherArgs(), c.Animator); err == nil {
		c.watcherID = wid
	} else {
		c.Log.Errorf("Failed to create watcher: %s", err.Error())
		c.Animator.SetRetryInterval(c.RetryInterval)
	}
	c.reservationCS = util.NewCriticalSectionGuard()
	c.rei = rei.NewEPM("vreq")
	c.rei.Enabled = c.DebugREI
	c.lastPurgeTaskTime = time.Now()

	vptRegisterAnimator(c)
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
	c.reservationCS.Drain()
	c.Animator.Stop()
	c.Log.Info("Stopped VolumeSeriesHandler")
}

func (c *Component) getWatcherArgs() *models.CrudWatcherCreateArgs {
	return &models.CrudWatcherCreateArgs{
		Name: "VSR",
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{ // new VSR objects
				MethodPattern: "POST",
				URIPattern:    "^/volume-series-requests$",
				ScopePattern:  "requestedOperations:(CREATE|BIND|DELETE|RENAME|ALLOCATE_CAPACITY|CHANGE_CAPACITY|DELETE_SPA|UNBIND|NODE_DELETE|VOL_DETACH)\\b", // 1st op must be an op for this handler
			},
			&models.CrudMatcher{ // any changes to pools
				URIPattern: "^/pools/?",
			},
			&models.CrudMatcher{ // any changes to SPAs
				URIPattern: "^/service-plan-allocations/?",
			},
			&models.CrudMatcher{ // VSR undo activities (may be triggered from downstream)
				MethodPattern: "PATCH",
				URIPattern:    "^/volume-series-requests/",
				ScopePattern:  "volumeSeriesRequestState:(UNDO_)?(CREATING|BINDING|ALLOCATING_CAPACITY|CHANGING_CAPACITY)",
			},
			&models.CrudMatcher{ // cancellation
				MethodPattern: "POST",
				URIPattern:    "^/volume-series-requests/.*/cancel",
				ScopePattern:  "volumeSeriesRequestState:(CREATING|BINDING|CHANGING_CAPACITY|CAPACITY_WAIT)",
			},
		},
	}
}

// vra.Ops methods

// RunBody is called once for each iteration of the Animator
func (c *Component) RunBody(ctx context.Context) []*models.VolumeSeriesRequest {
	c.runCount++
	c.Log.Debugf("VolumeSeriesHandler RunBody %d", c.runCount)
	if c.fatalError {
		return nil
	}
	if !c.startupCleanupDone {
		c.Log.Info("VolumeSeriesHandler cleanup starting")
		// TBD
		c.startupCleanupDone = true
		c.Log.Info("VolumeSeriesHandler cleanup done")
	}
	// VSR purging
	now := time.Now()
	if now.Sub(c.lastPurgeTaskTime) >= c.VSRPurgeTaskInterval {
		taskID, err := c.App.TaskScheduler.RunTask(&models.Task{TaskCreateOnce: models.TaskCreateOnce{Operation: com.TaskVsrPurge}})
		if err == nil {
			c.lastPurgeTaskTime = now
			c.vsrTaskID = ""
		} else {
			c.Log.Errorf("Task [%s] to purge volume series requests failed: %s", taskID, err.Error())
			c.vsrTaskID = taskID
		}
	}

	lParams := mcvr.NewVolumeSeriesRequestListParams()
	lParams.VolumeSeriesRequestState = centraldStates // see below
	lParams.IsTerminated = swag.Bool(false)
	if vrs, err := c.oCrud.VolumeSeriesRequestList(ctx, lParams); err == nil {
		return vrs.Payload
	}
	return nil
}

// The VSR states processed in centrald: if not in this list the VSR won't be loaded!
// Initialized with an init() function
var centraldStates = []string{
	com.VolReqStateNew, // must be included
}

func init() {
	for _, state := range vra.SupportedVolumeSeriesRequestStates() {
		ap := vra.GetStateInfo(state).GetProcess("foo")
		if ap == vra.ApCentrald && !util.Contains(centraldStates, state) { // ensure uniqueness
			centraldStates = append(centraldStates, state)
		}
	}
}

// ShouldDispatch is called once for each active request to decide if this handler should dispatch the request for processing
func (c *Component) ShouldDispatch(ctx context.Context, rhs *vra.RequestHandlerState) bool {
	ap := vra.GetStateInfo(rhs.Request.VolumeSeriesRequestState).GetProcess(rhs.Request.RequestedOperations[0])
	if ap == vra.ApCentrald {
		return true
	}
	c.Log.Debugf("Skipping VolumeSeriesRequest[%s] (%v, %s) processed by %s", rhs.Request.Meta.ID, rhs.Request.RequestedOperations, rhs.Request.VolumeSeriesRequestState, ap)
	return false
}
