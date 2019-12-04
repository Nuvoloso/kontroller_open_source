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

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	fmm "github.com/Nuvoloso/kontroller/pkg/metricmover/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestSIOBufferLimiting(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log:    tl.Logger(),
			Server: &restapi.Server{},
		},
	}
	evMA := &crude.ManagerArgs{
		Log: tl.Logger(),
	}
	evM := crude.NewManager(evMA) // the real CRUD manager
	app.CrudeOps = evM
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	c := &Component{}
	c.Init(app)

	pEls := []*models.IoMetricDatum{}
	for i := 0; i < 3; i++ {
		pEls = append(pEls, &models.IoMetricDatum{})
	}
	wEls := []*sIOMSample{}
	for i := 0; i < 3; i++ {
		wEls = append(wEls, &sIOMSample{})
	}
	reset := func() {
		tl.Flush()
		c.sIOP.InError = false
		c.sIOW.InError = false
		assert.Len(pEls, 3)
		assert.Len(wEls, 3)
		c.sIOP.Queue.PopN(c.sIOP.Queue.Length())
		c.sIOW.Queue.PopN(c.sIOW.Queue.Length())
		c.sIOP.Queue.Add(pEls)
		c.sIOW.Queue.Add(wEls)
		assert.Equal(3, c.sIOP.Queue.Length())
		assert.Equal(3, c.sIOW.Queue.Length())
	}

	totalDropped := 0

	// empty queues
	n := c.sIoMLimitBuffering()
	assert.Equal(0, n)

	// no errors, no buffering
	reset()
	c.StorageIOMaxBuffered = 0
	n = c.sIoMLimitBuffering()
	assert.Equal(6, n)
	totalDropped += n
	assert.Equal(totalDropped, c.numSIODropped)
	assert.Equal(0, c.sIOP.Queue.Length())
	assert.Equal(0, c.sIOW.Queue.Length())
	assert.Equal(1, tl.CountPattern("3 .* active .* writer"))
	assert.Equal(1, tl.CountPattern("3 .* active .* processor"))

	// no errors, limit impacts W only
	reset()
	c.StorageIOMaxBuffered = 4
	n = c.sIoMLimitBuffering()
	assert.Equal(2, n)
	totalDropped += n
	assert.Equal(totalDropped, c.numSIODropped)
	assert.Equal(3, c.sIOP.Queue.Length())
	assert.Equal(1, c.sIOW.Queue.Length())
	assert.Equal(1, tl.CountPattern("2 .* active .* writer"))

	// no errors, limit impacts W and P
	reset()
	c.StorageIOMaxBuffered = 2
	n = c.sIoMLimitBuffering()
	assert.Equal(4, n)
	totalDropped += n
	assert.Equal(totalDropped, c.numSIODropped)
	assert.Equal(2, c.sIOP.Queue.Length())
	assert.Equal(0, c.sIOW.Queue.Length())
	assert.Equal(1, tl.CountPattern("3 .* active .* writer"))
	assert.Equal(1, tl.CountPattern("1 .* active .* processor"))

	// P error, limit impacts P only
	reset()
	c.StorageIOMaxBuffered = 2
	c.sIOP.InError = true
	c.sIOW.InError = false
	n = c.sIoMLimitBuffering()
	assert.Equal(3, n)
	totalDropped += n
	assert.Equal(totalDropped, c.numSIODropped)
	assert.Equal(0, c.sIOP.Queue.Length())
	assert.Equal(3, c.sIOW.Queue.Length())
	assert.Equal(1, tl.CountPattern("3 .* inactive .* processor"))

	// W error, limit impacts W only
	reset()
	c.StorageIOMaxBuffered = 2
	c.sIOP.InError = false
	c.sIOW.InError = true
	n = c.sIoMLimitBuffering()
	assert.Equal(3, n)
	totalDropped += n
	assert.Equal(totalDropped, c.numSIODropped)
	assert.Equal(3, c.sIOP.Queue.Length())
	assert.Equal(0, c.sIOW.Queue.Length())
	assert.Equal(1, tl.CountPattern("3 .* inactive .* writer"))

	// Both W and P in error, limit impacts W and P
	reset()
	c.StorageIOMaxBuffered = 2
	c.sIOP.InError = true
	c.sIOW.InError = true
	n = c.sIoMLimitBuffering()
	assert.Equal(4, n)
	totalDropped += n
	assert.Equal(totalDropped, c.numSIODropped)
	assert.Equal(2, c.sIOP.Queue.Length())
	assert.Equal(0, c.sIOW.Queue.Length())
	assert.Equal(1, tl.CountPattern("3 .* inactive .* writer"))
	assert.Equal(1, tl.CountPattern("1 .* inactive .* processor"))

	// Test burst tolerance
	reset()
	c.StorageIOMaxBuffered = 2
	c.StorageIOBurstTolerance = 2
	c.numSIOCapacityExceeded = 0
	n = c.sIoMLimitCapacityWithBurstTolerance()
	assert.Equal(0, n)
	totalDropped += n
	assert.Equal(totalDropped, c.numSIODropped)
	assert.Equal(1, c.numSIOCapacityExceeded)
	assert.Equal(3, c.sIOP.Queue.Length())
	assert.Equal(3, c.sIOW.Queue.Length())
	n = c.sIoMLimitCapacityWithBurstTolerance()
	assert.Equal(0, n)
	totalDropped += n
	assert.Equal(totalDropped, c.numSIODropped)
	assert.Equal(2, c.numSIOCapacityExceeded)
	assert.Equal(3, c.sIOP.Queue.Length())
	assert.Equal(3, c.sIOW.Queue.Length())
	n = c.sIoMLimitCapacityWithBurstTolerance()
	assert.Equal(4, n)
	totalDropped += n
	assert.Equal(totalDropped, c.numSIODropped)
	assert.Equal(0, c.numSIOCapacityExceeded)
	assert.Equal(2, c.sIOP.Queue.Length())
	assert.Equal(0, c.sIOW.Queue.Length())
}
