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
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/op/go-logging"
)

// HBComp is used to manage this handler component
type HBComp struct {
	app                *centrald.AppCtx
	Log                *logging.Logger
	sleepPeriod        time.Duration
	stopPeriod         time.Duration
	lastVersionLogTime time.Time
	worker             util.Worker
	runCount           int
	cht                *clusterHeartbeatTask
}

func newHBComp() *HBComp {
	hc := &HBComp{}
	return hc
}

func init() {
	centrald.AppRegisterComponent(newHBComp())
}

// Init registers handlers for this component
func (c *HBComp) Init(app *centrald.AppCtx) {
	c.app = app
	c.Log = app.Log
	c.sleepPeriod = time.Duration(app.HeartbeatPeriod) * time.Second
	c.stopPeriod = 10 * time.Second
	wa := &util.WorkerArgs{
		Name:             "Heartbeat",
		Log:              c.Log,
		SleepInterval:    c.sleepPeriod,
		TerminationDelay: c.stopPeriod,
	}
	c.worker, _ = util.NewWorker(wa, c)
	c.cht = chtRegisterAnimator(c)
}

// Start starts this component
func (c *HBComp) Start() {
	c.runCount = 0
	c.worker.Start()
}

// Stop terminates this component
func (c *HBComp) Stop() {
	c.worker.Stop()
}

// logVersion periodically logs the app version
func (c *HBComp) logVersion() {
	now := time.Now()
	if now.Sub(c.lastVersionLogTime) >= c.app.VersionLogPeriod {
		c.Log.Info(c.app.InvocationArgs)
		c.lastVersionLogTime = now
	}
}

// add mover stats to in-memory service object
func (c *HBComp) updateMetricMoverStats() {
	vt := models.ValueType{
		Kind:  "STRING",
		Value: c.app.MetricMover.Status().String(),
	}
	c.app.Service.SetServiceAttribute(com.ServiceAttrMetricMoverStatus, vt)
}

// add CRUDE stats to in-memory service object
func (c *HBComp) updateCrudeStatistics() {
	vt := models.ValueType{
		Kind:  "STRING",
		Value: c.app.CrudeOps.GetStats().String(),
	}
	c.app.Service.SetServiceAttribute(com.ServiceAttrCrudeStatistics, vt)
}

// Buzz satisfies the util.WorkerBee interface
func (c *HBComp) Buzz(ctx context.Context) error {
	c.runCount++
	c.updateMetricMoverStats()
	c.updateCrudeStatistics()
	c.logVersion()
	c.cht.TimeoutClusters(ctx)
	return nil
}
