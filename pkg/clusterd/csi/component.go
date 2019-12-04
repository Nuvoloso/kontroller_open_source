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
	"time"

	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	"github.com/Nuvoloso/kontroller/pkg/csi"
	"github.com/Nuvoloso/kontroller/pkg/util"
	logging "github.com/op/go-logging"
)

// Args contains settable parameters for this component
type Args struct {
	StartupSleepInterval time.Duration `long:"startup-sleep-interval-secs" description:"Sleep interval set on startup." default:"5s"`
	VSRCompleteByPeriod  time.Duration `long:"vsr-complete-by-period" description:"The period within which we expect VSRs to complete execution." default:"5m"`
}

// csiComp interacts with CSI
type csiComp struct {
	Args
	app *clusterd.AppCtx
	Log *logging.Logger

	h        csi.ControllerHandler
	hStarted bool
	w        util.Worker
}

// ComponentInit must be called from main to initialize and register this component.
func ComponentInit(args *Args) interface{} {
	c := &csiComp{}
	c.Args = *args
	clusterd.AppRegisterComponent(c)
	return c
}

// Init sets up this component
func (c *csiComp) Init(app *clusterd.AppCtx) {
	c.app = app
	c.Log = app.Log
	if c.app.CSISocket == "" {
		c.Log.Info("CSI not configured")
		return
	}
	c.w, _ = util.NewWorker(c.getWorkerArgs(), c)
	c.app.VolumeDeletionHandler = c
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
	if c.app.AppObjects == nil || c.app.AppObjects.GetCluster() == nil {
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
	h, err := csi.NewControllerHandler(c.getCsiHandlerArgs())
	if err != nil {
		c.Log.Criticalf("Failed to create CSI handler: %s", err.Error())
		c.app.AppServant.FatalError(err)
		return util.ErrWorkerAborted // don't need the worker any more
	}
	c.h = h // start the handler on the next tick
	return nil
}

func (c *csiComp) getCsiHandlerArgs() *csi.ControllerHandlerArgs {
	cha := &csi.ControllerHandlerArgs{}
	cha.Socket = string(c.app.CSISocket)
	cha.Log = c.Log
	cha.Version = c.app.ServiceVersion
	cha.ClusterClient = c.app.ClusterClient
	cha.Ops = c // self-reference
	return cha
}

func (c *csiComp) getWorkerArgs() *util.WorkerArgs {
	return &util.WorkerArgs{
		Name:          "csiComp",
		Log:           c.Log,
		SleepInterval: c.Args.StartupSleepInterval,
	}
}

func (c *csiComp) IsReady() bool {
	return c.app.IsReady()
}
