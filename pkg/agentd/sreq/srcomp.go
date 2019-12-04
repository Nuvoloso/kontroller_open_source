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

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
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

// SRComp is used to manage this handler component
type SRComp struct {
	Args
	app                *agentd.AppCtx
	Log                *logging.Logger
	mux                sync.Mutex
	oCrud              crud.Ops
	cancelRun          context.CancelFunc
	stoppedChan        chan struct{}
	notifyFlagged      bool
	notifyChan         chan struct{}
	notifyCount        int
	wakeCount          int
	runCount           int
	thisNodeID         models.ObjIDMutable
	thisNodeIdentifier string
	requestHandlers    requestHandlers
	activeRequests     map[models.ObjID]*requestHandlerState
	doneRequests       map[models.ObjID]models.ObjVersion
	watcherID          string
	rei                *rei.EphemeralPropertyManager
}

// module constants
const (
	RetryIntervalDefault   = 30 * time.Second
	WatcherRetryMultiplier = 2
)

// ComponentInit must be called from main to initialize and register this component.
func ComponentInit(args *Args) *SRComp {
	c := &SRComp{}
	c.Args = *args
	c.notifyFlagged = true // protect against Notify() until started
	agentd.AppRegisterComponent(c)
	return c
}

// Init registers handlers for this component
func (c *SRComp) Init(app *agentd.AppCtx) {
	c.app = app
	c.Log = app.Log
	c.Log.Info("Initializing StorageRequest processor")
	if c.RetryInterval == 0 {
		c.RetryInterval = RetryIntervalDefault
	}
	c.activeRequests = make(map[models.ObjID]*requestHandlerState)
	c.doneRequests = make(map[models.ObjID]models.ObjVersion)
	c.requestHandlers = c       // self-reference
	c.app.AppRecoverStorage = c // provide the interface
	if wid, err := c.app.CrudeOps.Watch(c.getWatcherArgs(), c); err == nil {
		c.watcherID = wid
		c.RetryInterval = c.RetryInterval * WatcherRetryMultiplier
	} else {
		c.Log.Errorf("Failed to create watcher: %s", err.Error())
	}
	c.rei = rei.NewEPM("sreq")
	c.rei.Enabled = c.DebugREI
}

// Start starts this component
func (c *SRComp) Start() {
	c.Log.Info("Starting StorageRequest processor")
	c.oCrud = c.app.OCrud
	ctx, cancel := context.WithCancel(context.Background())
	c.cancelRun = cancel
	c.runCount = 0
	c.notifyCount = 0
	c.wakeCount = 0
	c.notifyReset(true)
	go util.PanicLogger(c.Log, func() { c.run(ctx) })
}

// Stop terminates this component
func (c *SRComp) Stop() {
	if c.cancelRun != nil {
		c.Log.Info("Stopping StorageRequest processor")
		c.stoppedChan = make(chan struct{})
		c.cancelRun()
		select {
		case <-c.stoppedChan: // response received
		case <-time.After(c.RetryInterval):
			c.Log.Warning("Timed out waiting for termination")
		}
	}
	c.Log.Info("Stopped StorageRequest processor")
}

// Notify is used to awaken the pool loop
func (c *SRComp) Notify() {
	c.mux.Lock()
	defer c.mux.Unlock()
	c.notifyCount++
	if !c.notifyFlagged {
		c.notifyFlagged = true
		close(c.notifyChan)
	}
}

func (c *SRComp) notifyReset(force bool) {
	c.mux.Lock()
	defer c.mux.Unlock()
	if c.notifyFlagged || force {
		c.notifyFlagged = false
		c.notifyChan = make(chan struct{})
	}
}

func (c *SRComp) run(ctx context.Context) {
	for {
		c.notifyReset(false) // notifications while body is running will force another call
		c.mux.Lock()
		// clean out old doneRequests before the new list of requests is queried
		c.doneRequests = make(map[models.ObjID]models.ObjVersion)
		c.mux.Unlock()
		c.runBody(ctx)
		select {
		case <-c.notifyChan: // channel closed on
			c.Log.Debug("StorageRequest processor woken up")
			c.wakeCount++
		case <-ctx.Done():
			close(c.stoppedChan) // notify waiter by closing the channel
			return
		case <-time.After(c.RetryInterval):
			c.Log.Debug("StorageRequest processor new interval")
			// wait
		}
	}
}

func (c *SRComp) runBody(ctx context.Context) {
	c.runCount++
	if c.thisNodeID == "" {
		nObj := c.app.AppObjects.GetNode()
		if nObj == nil {
			return // must wait until our Node is created/loaded
		}
		c.setNodeInfo(nObj)
	}
	if !c.app.AppServant.IsReady() {
		return
	}
	lParams := storage_request.NewStorageRequestListParams()
	lParams.NodeID = swag.String(string(c.thisNodeID))
	lParams.IsTerminated = swag.Bool(false)
	if srs, err := c.oCrud.StorageRequestList(ctx, lParams); err == nil {
		c.dispatchRequests(ctx, srs.Payload)
	}
}

func (c *SRComp) setNodeInfo(node *models.Node) {
	if c.thisNodeID == "" {
		c.thisNodeID = models.ObjIDMutable(node.Meta.ID)
		c.thisNodeIdentifier = node.NodeIdentifier
	}
}

// CrudeNotify satisfies the crude.Watcher interface
func (c *SRComp) CrudeNotify(cbt crude.WatcherCallbackType, ce *crude.CrudEvent) error {
	if cbt == crude.WatcherEvent {
		c.Notify()
	}
	return nil
}

// getWatcherArgs monitors events to trigger reload
// Note: Events are constrained to this nodeId courtesy the upstream monitor in hb.
func (c *SRComp) getWatcherArgs() *models.CrudWatcherCreateArgs {
	return &models.CrudWatcherCreateArgs{
		Name: "SR",
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{ // new SRs
				MethodPattern: "POST",
				URIPattern:    "^/storage-requests$",
			},
			&models.CrudMatcher{ // upstream triggers
				MethodPattern: "PATCH",
				URIPattern:    "^/storage-requests/",
				ScopePattern:  "storageRequestState:(FORMATTING|USING|CLOSING)",
			},
		},
	}
}
