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
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/centrald"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	fhk "github.com/Nuvoloso/kontroller/pkg/housekeeping/fake"
	fmm "github.com/Nuvoloso/kontroller/pkg/metricmover/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	fw "github.com/Nuvoloso/kontroller/pkg/util/fake"
	"github.com/stretchr/testify/assert"
)

func TestComponentMethods(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	fts := &fhk.TaskScheduler{}
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log:              tl.Logger(),
			HeartbeatPeriod:  10,
			VersionLogPeriod: 30 * time.Minute,
		},
		TaskScheduler: fts,
	}
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	hb := newHBComp()

	// Init
	assert.NotPanics(func() { hb.Init(app) })
	assert.Equal(app.Log, hb.Log)
	assert.Equal(app, hb.app)
	assert.Equal(10*time.Second, hb.sleepPeriod)
	assert.NotNil(hb.worker)

	assert.True(util.Contains(fts.OpsRegistered, com.TaskClusterHeartbeat))
	assert.NotNil(hb.cht)

	// replace the worker
	tw := &fw.Worker{}
	hb.worker = tw

	hb.Start()
	assert.Equal(1, tw.CntStart)

	hb.Stop()
	assert.Equal(1, tw.CntStop)
}

func TestVersionLog(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	fts := &fhk.TaskScheduler{}
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log:              tl.Logger(),
			HeartbeatPeriod:  10,
			VersionLogPeriod: 30 * time.Minute,
			ServiceVersion:   "commit time host",
			NuvoAPIVersion:   "0.6.2",
			InvocationArgs:   "{ invocation flag values }",
		},
		TaskScheduler: fts,
	}
	eM := &fev.Manager{}
	app.CrudeOps = eM
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	svcArgs := util.ServiceArgs{
		ServiceType:    "centrald",
		ServiceVersion: "test",
		Log:            app.Log,
	}
	app.Service = util.NewService(&svcArgs)
	hb := newHBComp()

	// Init
	hb.Init(app)
	assert.Equal(0, hb.runCount)
	hb.cht.ops = &fakeChtOps{}

	// first call logs
	now := time.Now()
	assert.Zero(hb.lastVersionLogTime)
	err := hb.Buzz(nil)
	assert.NoError(err)
	assert.True(hb.runCount == 1)
	assert.Equal(1, tl.CountPattern("INFO .*invocation flag values"))
	assert.True(now.Before(hb.lastVersionLogTime))
	prevLogTime := hb.lastVersionLogTime
	tl.Flush()

	// second interation, no logging
	err = hb.Buzz(nil)
	assert.NoError(err)
	assert.True(hb.runCount == 2)
	assert.Equal(0, tl.CountPattern("INFO .*invocation flag values"))
	assert.Equal(prevLogTime, hb.lastVersionLogTime)
	tl.Flush()

	// time is past, logs again
	hb.lastVersionLogTime = now.Add(-30 * time.Minute)
	err = hb.Buzz(nil)
	assert.NoError(err)
	assert.True(hb.runCount == 3)
	assert.Equal(1, tl.CountPattern("INFO .*invocation flag values"))
	assert.True(prevLogTime.Before(hb.lastVersionLogTime))

	// service object has stat fields
	assert.NotNil(app.Service.GetServiceAttribute(com.ServiceAttrMetricMoverStatus))
	assert.NotNil(app.Service.GetServiceAttribute(com.ServiceAttrCrudeStatistics))
}
