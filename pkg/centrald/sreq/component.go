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


package sreq

import (
	"context"
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
	"github.com/op/go-logging"
)

// Args contains settable parameters for this component
type Args struct {
	RetryInterval time.Duration `long:"retry-interval-secs" description:"Request handler retry loop polling interval if not monitoring events. When monitoring events this is increased." default:"30s"`
	DebugREI      bool          `long:"debug-permit-rei" description:"Permit runtime error injection"`
}

// Component animates CSPDomain level provisioning
type Component struct {
	Args
	App                      *centrald.AppCtx
	Log                      *logging.Logger
	mux                      sync.Mutex
	cancelRun                context.CancelFunc
	stoppedChan              chan struct{}
	notifyFlagged            bool
	notifyChan               chan struct{}
	runCount                 int
	notifyCount              int
	wakeCount                int
	fatalError               bool
	systemID                 string
	startupCleanupDone       bool
	updateCapacityRetryCount int
	activeRequests           map[models.ObjID]*requestHandlerState
	doneRequests             map[models.ObjID]models.ObjVersion
	requestHandlers          requestHandlers
	oCrud                    crud.Ops
	watcherID                string
	rei                      *rei.EphemeralPropertyManager
}

// module constants
const (
	RetryIntervalDefault     = time.Second * 30
	WatcherRetryMultiplier   = 2
	UpdateCapacityRetryCount = 3
)

// PoolInit must be called from main to initialize and register this component.
func PoolInit(args *Args) *Component {
	c := &Component{}
	c.Args = *args
	c.notifyFlagged = true // protect against Notify() until started
	centrald.AppRegisterComponent(c)
	return c
}

// Init registers handlers for this component
func (c *Component) Init(app *centrald.AppCtx) {
	c.App = app
	c.Log = app.Log
	if c.RetryInterval == 0 {
		c.RetryInterval = RetryIntervalDefault
	}
	if wid, err := c.App.CrudeOps.Watch(c.getWatcherArgs(), c); err == nil {
		c.watcherID = wid
		c.RetryInterval = c.RetryInterval * WatcherRetryMultiplier
	} else {
		c.Log.Errorf("Failed to create watcher: %s", err.Error())
	}
	c.activeRequests = make(map[models.ObjID]*requestHandlerState)
	c.doneRequests = make(map[models.ObjID]models.ObjVersion)
	c.requestHandlers = c // self-reference
	c.updateCapacityRetryCount = UpdateCapacityRetryCount
	c.rei = rei.NewEPM("sreq")
	c.rei.Enabled = c.DebugREI
}

// Start starts this component
func (c *Component) Start() {
	c.Log.Info("Starting Pool")
	c.oCrud = crud.NewClient(c.App.ClientAPI, c.Log)
	ctx, cancel := context.WithCancel(context.Background())
	c.cancelRun = cancel
	c.runCount = 0
	c.notifyCount = 0
	c.wakeCount = 0
	c.notifyReset(true)
	go util.PanicLogger(c.Log, func() { c.run(ctx) })
}

// Stop terminates this component
func (c *Component) Stop() {
	c.App.CrudeOps.TerminateWatcher(c.watcherID)
	if c.cancelRun != nil {
		c.Log.Info("Stopping Pool")
		c.stoppedChan = make(chan struct{})
		c.cancelRun()
		select {
		case <-c.stoppedChan: // response received
		case <-time.After(c.RetryInterval):
			c.Log.Warning("Timed out waiting for termination")
		}
	}
	c.Log.Info("Stopped Pool")
}

// Notify is used to awaken the pool loop
func (c *Component) Notify() {
	c.mux.Lock()
	defer c.mux.Unlock()
	c.notifyCount++
	if !c.notifyFlagged {
		c.notifyFlagged = true
		close(c.notifyChan)
	}
}

func (c *Component) notifyReset(force bool) {
	c.mux.Lock()
	defer c.mux.Unlock()
	if c.notifyFlagged || force {
		c.notifyFlagged = false
		c.notifyChan = make(chan struct{})
	}
}

func (c *Component) run(ctx context.Context) {
	for {
		c.notifyReset(false) // notifications while body is running will force another call
		c.mux.Lock()
		// clean out old doneRequests before the new list of requests is queried
		c.doneRequests = make(map[models.ObjID]models.ObjVersion)
		c.mux.Unlock()
		c.runBody(ctx)
		select {
		case <-c.notifyChan: // channel closed on
			c.Log.Debug("Pool woken up")
			c.wakeCount++
		case <-ctx.Done():
			close(c.stoppedChan) // notify waiter by closing the channel
			return
		case <-time.After(c.RetryInterval):
			// wait
		}
	}
}

func (c *Component) runBody(ctx context.Context) {
	c.runCount++
	c.Log.Debugf("Pool runBody %d", c.runCount)
	if c.fatalError {
		return
	}
	if c.systemID == "" {
		if sObj, err := c.oCrud.SystemFetch(ctx); err == nil {
			c.systemID = string(sObj.Meta.ID)
			c.Log.Info("SystemID:", c.systemID)
		} else {
			c.Log.Warningf("Failed to load System object: %s", err.Error())
			return
		}
	}
	if !c.startupCleanupDone {
		c.Log.Info("Pool cleanup starting")
		var err error
		if err = c.releaseOrphanedCSPVolumes(ctx); err == nil {
			if err = c.makeProvisioningStorageUnprovisioned(ctx); err == nil {
				err = c.deleteUnprovisionedStorage(ctx)
			}
		}
		if err != nil {
			return
		}
		c.startupCleanupDone = true
		c.Log.Info("Pool cleanup done")
	}
	lparams := storage_request.NewStorageRequestListParams()
	lparams.IsTerminated = swag.Bool(false)
	if srs, err := c.oCrud.StorageRequestList(ctx, lparams); err == nil {
		c.dispatchRequests(ctx, srs.Payload)
	}
}

// CrudeNotify satisfies the crude.Watcher interface
func (c *Component) CrudeNotify(cbt crude.WatcherCallbackType, ce *crude.CrudEvent) error {
	if cbt == crude.WatcherEvent {
		c.Notify()
	}
	return nil
}

func (c *Component) getWatcherArgs() *models.CrudWatcherCreateArgs {
	return &models.CrudWatcherCreateArgs{
		Name: "SR",
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{ // new SRs
				MethodPattern: "POST",
				URIPattern:    "^/storage-requests$",
			},
			&models.CrudMatcher{ // any changes to pools
				URIPattern: "^/pools/?",
			},
			&models.CrudMatcher{ // SR undo activities triggered from downstream, REMOVING_TAG after successful downstream steps
				MethodPattern: "PATCH",
				URIPattern:    "^/storage-requests/",
				ScopePattern:  "storageRequestState:(UNDO_(PROVISIONING|ATTACHING|DETACHING)|REMOVING_TAG)",
			},
		},
	}
}
