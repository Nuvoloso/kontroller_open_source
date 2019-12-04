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


package metrics

import (
	"context"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	"github.com/Nuvoloso/kontroller/pkg/util"
	logging "github.com/op/go-logging"
)

// MetricComp handles the collection and publication of metric data
type MetricComp struct {
	app *agentd.AppCtx
	Log *logging.Logger

	rt            *util.RoundingTicker
	cntVSIo       int
	cntSIo        int
	ioMDMaker     ioMetricDatumMaker
	volLMDUpdater volLifecycleManagementDataUpdater
}

// internal interface to make an IO metric datum
type ioMetricDatumMaker interface {
	makeIOMetricDatum(now time.Time, objID string, readStats, lastReadStats, writeStats, lastWriteStats *nuvoapi.StatsIO, cacheUserStat, lastCacheUserStat, cacheMetaStat, lastCacheMetaStat *nuvoapi.StatsCache) (*models.IoMetricDatum, error)
}

// internal interface to update volume snapshot data
type volLifecycleManagementDataUpdater interface {
	updateVolLMD(ctx context.Context, vsID string, args *SnapCalculationArgs) error
	snapComputeNextSnapTime(args *SnapCalculationArgs)
	snapComputeEffectiveSlope(args *SnapCalculationArgs)
	snapInterpolateNextSnapTime(args *SnapCalculationArgs)
}

func newMetricComp() *MetricComp {
	c := &MetricComp{}
	return c
}

func init() {
	agentd.AppRegisterComponent(newMetricComp())
}

// Init sets up this component
func (c *MetricComp) Init(app *agentd.AppCtx) {
	c.app = app
	c.Log = app.Log
	rta := c.getRTArgs()
	c.rt, _ = util.NewRoundingTicker(rta)
	c.ioMDMaker = c          // self-reference
	c.volLMDUpdater = c      // self-reference
	c.app.MetricReporter = c // provide the interface
}

// Start starts metric collection
func (c *MetricComp) Start() {
	c.Log.Info("Starting metric collection")
	c.rt.Start(c)
}

// Stop terminates this component
func (c *MetricComp) Stop() {
	c.rt.Stop()
	c.Log.Info("Stopped metric collection")
}

func (c *MetricComp) getRTArgs() *util.RoundingTickerArgs {
	return &util.RoundingTickerArgs{
		Log:       c.Log,
		Period:    c.app.MetricIoCollectionPeriod,
		RoundDown: c.app.MetricIoCollectionTruncate,
	}
}

// Beep satisfies the util.RoundingTickerBeeper interface
func (c *MetricComp) Beep(ctx context.Context) {
	c.publishVolumeIOStats(ctx)
	c.publishStorageIOStats()
}
