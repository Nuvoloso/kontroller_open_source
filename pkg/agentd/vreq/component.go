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

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	mcvr "github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/mount"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/swag"
	"github.com/op/go-logging"
)

// WatcherRetryMultiplier increases the refresh polling period.
// TBD: increase once we can handle timeout differently
const WatcherRetryMultiplier = 2

// Args contains settable parameters for this component
type Args struct {
	RetryInterval time.Duration `long:"retry-interval-secs" description:"Request handler retry loop polling interval if not monitoring events. When monitoring events this is increased." default:"30s"`
	StopPeriod    time.Duration `long:"stop-period" description:"Request handler termination duration" default:"10s"`
	DebugREI      bool          `long:"debug-permit-rei" description:"Permit runtime error injection"`
}

// Component handles volume series requests
type Component struct {
	Args
	App                *agentd.AppCtx
	Log                *logging.Logger
	Animator           *vra.Animator
	mounter            mount.Mounter // filesystem mounter
	runCount           int
	thisNodeID         models.ObjIDMutable
	thisNodeIdentifier string
	oCrud              crud.Ops
	watcherID          string
	rei                *rei.EphemeralPropertyManager
}

// ComponentInit must be called from main to initialize and register this component.
func ComponentInit(args *Args) *Component {
	c := &Component{}
	c.Args = *args
	agentd.AppRegisterComponent(c)
	return c
}

// Init registers handlers for this component
func (c *Component) Init(app *agentd.AppCtx) {
	c.App = app
	c.Log = app.Log
	app.AppRecoverVolume = c // provide the interface
	c.Animator = vra.NewAnimator(WatcherRetryMultiplier*c.RetryInterval, c.Log, c).WithMountHandlers(c).WithAttachFsHandlers(c).WithVolSnapshotCreateHandlers(c).WithVolSnapshotRestoreHandlers(c).WithStopPeriod(c.StopPeriod)
	// no runtime scope data in the watcher so register now
	if wid, err := c.App.CrudeOps.Watch(c.getWatcherArgs(), c.Animator); err == nil {
		c.watcherID = wid
	} else {
		c.Log.Errorf("Failed to create watcher: %s", err.Error())
		c.Animator.SetRetryInterval(c.RetryInterval)
	}
	c.rei = rei.NewEPM("vreq")
	c.rei.Enabled = c.DebugREI
	c.mounter, _ = mount.New(&mount.MounterArgs{Log: c.Log, Rei: c.rei})
}

// Start starts this component
func (c *Component) Start() {
	c.Log.Info("Starting VolumeSeriesHandler")
	c.oCrud = c.App.OCrud
	c.runCount = 0
	c.Animator.Start(c.oCrud, c.App.CrudeOps)
}

// Stop terminates this component
func (c *Component) Stop() {
	c.Animator.Stop()
	c.Log.Info("Stopped VolumeSeriesHandler")
}

// getWatcherArgs returns watcher args for this component.
// Note: Only node scoped events are visible, courtesy of the upstream monitor in hb.
func (c *Component) getWatcherArgs() *models.CrudWatcherCreateArgs {
	return &models.CrudWatcherCreateArgs{
		Name: "VSR",
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{ // new VSR objects
				MethodPattern: "POST",
				URIPattern:    "^/volume-series-requests$",
				ScopePattern:  "requestedOperations:(UNMOUNT|VOL_SNAPSHOT_CREATE|VOL_SNAPSHOT_RESTORE|ATTACH_FS|DETACH_FS)\\b", // 1st op must be an op for this handler
			},
			&models.CrudMatcher{ // VSR activities triggered from upstream or downstream
				MethodPattern: "PATCH",
				URIPattern:    "^/volume-series-requests/",
				ScopePattern:  "volumeSeriesRequestState:(UNDO_)?(VOLUME_(CONFIG|EXPORT)|REALLOCATING_CACHE|SNAPSHOT_RESTORE(\\b|_FINALIZE))",
			},
		},
	}
}

// vra.Ops methods

// RunBody is called once for each iteration of the Animator
func (c *Component) RunBody(ctx context.Context) []*models.VolumeSeriesRequest {
	c.runCount++
	if c.thisNodeID == "" {
		if c.App.AppObjects == nil {
			return nil // must wait for AppObjects to be set
		}
		nObj := c.App.AppObjects.GetNode()
		if nObj == nil {
			return nil // must wait until this Node is created/loaded
		}
		c.setNodeInfo(nObj)
	}
	if !c.App.AppServant.IsReady() || c.App.StateOps == nil {
		return nil
	}
	id := "agentd.vreq"
	c.Log.Debugf("%s: Acquiring state lock", id)
	nst, err := c.App.LMDGuard.Enter(id)
	if err != nil {
		c.Log.Errorf("%s: Failed to get state lock: %s", id, err.Error())
		return nil
	}
	defer func() {
		c.Log.Debugf("%s: Releasing state lock", id)
		nst.Leave()
	}()
	c.Log.Debugf("%s: Obtained state lock", id)
	if chg, err := c.App.StateOps.Reload(ctx); err != nil {
		c.Log.Warningf("Error determining node state: %s", err.Error())
		return nil
	} else if chg {
		ss := c.App.StateOps.DumpState().String()
		c.Log.Debugf("New state of node:\n%s", ss)
		c.App.Service.ReplaceMessage("^Node state", "Node state:\n%s", ss) // becomes visible in the node object
		if c.App.StateUpdater != nil {
			c.App.StateUpdater.UpdateState(ctx)
		}
	}
	lParams := mcvr.NewVolumeSeriesRequestListParams()
	lParams.NodeID = swag.String(string(c.thisNodeID))
	lParams.IsTerminated = swag.Bool(false)
	if vrs, err := c.oCrud.VolumeSeriesRequestList(ctx, lParams); err == nil {
		return vrs.Payload
	}
	return nil
}

func (c *Component) setNodeInfo(node *models.Node) {
	if c.thisNodeID == "" {
		c.thisNodeID = models.ObjIDMutable(node.Meta.ID)
		c.thisNodeIdentifier = node.NodeIdentifier
	}
}

// ShouldDispatch is called once for each active request to decide if this handler should dispatch the request for processing
func (c *Component) ShouldDispatch(ctx context.Context, rhs *vra.RequestHandlerState) bool {
	ap := vra.GetStateInfo(rhs.Request.VolumeSeriesRequestState).GetProcess(rhs.Request.RequestedOperations[0])
	if ap == vra.ApAgentd {
		return true
	}
	c.Log.Debugf("Skipping VolumeSeriesRequest[%s] (%v, %s) processed by %s", rhs.Request.Meta.ID, rhs.Request.RequestedOperations, rhs.Request.VolumeSeriesRequestState, ap)
	return false
}
