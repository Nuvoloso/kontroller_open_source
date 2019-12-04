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


package csi

import (
	"context"
	"fmt"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	"github.com/Nuvoloso/kontroller/pkg/csi"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/util"
	logging "github.com/op/go-logging"
)

// Args contains settable parameters for this component
type Args struct {
	StartupSleepInterval time.Duration `long:"startup-sleep-interval-secs" description:"Sleep interval set on startup." default:"5s"`
	VSRCompleteByPeriod  time.Duration `long:"vsr-complete-by-period" description:"The period within which we expect VSRs to complete execution." default:"20m"`
}

// csiComp interacts with CSI
type csiComp struct {
	Args
	app      *agentd.AppCtx
	Log      *logging.Logger
	h        csi.NodeHandler
	hStarted bool
	w        util.Worker
	rei      *rei.EphemeralPropertyManager
}

// ComponentInit must be called from main to initialize and register this component.
func ComponentInit(args *Args) interface{} {
	c := &csiComp{}
	c.Args = *args
	agentd.AppRegisterComponent(c)
	return c
}

// Init sets up this component
func (c *csiComp) Init(app *agentd.AppCtx) {
	c.app = app
	c.Log = app.Log
	if c.app.CSISocket == "" {
		c.Log.Info("CSI not configured")
		return
	}
	c.w, _ = util.NewWorker(c.getWorkerArgs(), c)
	c.rei = rei.NewEPM("csi")
	c.rei.Enabled = app.DebugREI
}

// Start starts this components
func (c *csiComp) Start() {
	if c.w != nil {
		c.Log.Info("CSI will be started when pre-conditions are met")
		c.w.Start()
	}
}

// Stop terminates this component
func (c *csiComp) Stop() {
	if c.w != nil {
		c.w.Stop()
	}
	if c.h != nil && c.hStarted {
		c.Log.Info("Stopping CSI handler")
		c.h.Stop()
	}
}

// Buzz satisfies util.WorkerBee
func (c *csiComp) Buzz(ctx context.Context) error {
	if c.app.AppObjects == nil || c.app.AppObjects.GetNode() == nil {
		return nil // not yet ready
	}
	if c.h != nil {
		c.Log.Info("Starting CSI handler")
		c.hStarted = true
		c.h.Start()
		c.Log.Info("Started CSI handler")
		return util.ErrWorkerAborted // don't need the worker any more
	}
	c.Log.Info("Creating CSI handler")
	h, err := csi.NewNodeHandler(c.getCsiHandlerArgs())
	if err != nil {
		c.Log.Criticalf("Failed to create CSI handler: %s", err.Error())
		c.app.AppServant.FatalError(err)
		return util.ErrWorkerAborted // don't need the worker any more
	}
	c.h = h // start the handler on the next tick
	return nil
}

func (c *csiComp) getCsiHandlerArgs() *csi.NodeHandlerArgs {
	nha := &csi.NodeHandlerArgs{}
	nha.Socket = string(c.app.CSISocket)
	nha.Log = c.Log
	nha.Node = c.app.AppObjects.GetNode()
	nha.Cluster = c.app.AppObjects.GetCluster()
	nha.Version = c.app.ServiceVersion
	nha.ClusterClient = c.app.ClusterClient
	nha.Ops = c // self-reference
	return nha
}

func (c *csiComp) getWorkerArgs() *util.WorkerArgs {
	return &util.WorkerArgs{
		Name:          "csiComp",
		Log:           c.Log,
		SleepInterval: c.Args.StartupSleepInterval,
	}
}

// vsMediaState represents volume series states in the CSI mount process
type vsMediaState int

// vsMediaState values
const (
	// invalid State
	InvalidState vsMediaState = iota
	// Volume series is created
	CreatedState
	// Volume series is bound
	BoundState
	// Volume series is mounted
	MountedState
	// Volume series is mounted and attached
	FsAttachedState
	// Volume series is not created
	NotCreatedDynamicState
	// Volume series is not bound
	NotBoundState
	// Volume series is not mounted
	NotMountedState
	// No Fs attached
	NoFsAttachedState
	// Volume is not mounted, needs snapshot restore
	SnapRestoreState
)

func (ss vsMediaState) String() string {
	switch ss {
	case InvalidState:
		return "InvalidState"
	case CreatedState:
		return "CreatedState"
	case BoundState:
		return "BoundState"
	case MountedState:
		return "MountedState"
	case FsAttachedState:
		return "FsAttachedState"
	case NotCreatedDynamicState:
		return "NotCreatedDynamicState"
	case NotBoundState:
		return "NotBoundState"
	case NotMountedState:
		return "NotMountedState"
	case NoFsAttachedState:
		return "NoFsAttachedState"
	case SnapRestoreState:
		return "SnapRestoreState"
	}
	return fmt.Sprintf("vsMediaState(%d)", ss)
}

func (c *csiComp) IsReady() bool {
	return c.app.IsReady()
}
