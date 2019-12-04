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
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestComponentMethods(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log:                        tl.Logger(),
			MetricIoCollectionPeriod:   5 * time.Minute,
			MetricIoCollectionTruncate: 5 * time.Minute,
		},
	}
	mc := &MetricComp{}

	mc.Init(app)
	assert.NotNil(mc.rt)
	assert.Equal(mc, mc.ioMDMaker)
	assert.Equal(mc, mc.volLMDUpdater)
	assert.Equal(mc, app.MetricReporter)

	rta := mc.getRTArgs()
	assert.Equal(app.Log, rta.Log)
	assert.Equal(app.MetricIoCollectionPeriod, rta.Period)
	assert.Equal(app.MetricIoCollectionTruncate, rta.RoundDown)
	assert.False(rta.CallImmediately)

	mc.Start()
	err := mc.rt.Start(mc)
	assert.Error(err)
	assert.Regexp("active", err)

	// no objects cached so Beep is safe to call
	mc.Beep(nil)
	assert.Equal(1, mc.cntVSIo)
	assert.Equal(1, mc.cntSIo)

	mc.Stop()
}
