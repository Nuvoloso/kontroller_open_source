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

func TestVsIOBufferLimiting(t *testing.T) {
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
	wEls := []*vsIOMSample{}
	for i := 0; i < 3; i++ {
		wEls = append(wEls, &vsIOMSample{})
	}
	reset := func() {
		tl.Flush()
		c.vIOP.InError = false
		c.vIOW.InError = false
		assert.Len(pEls, 3)
		assert.Len(wEls, 3)
		c.vIOP.Queue.PopN(c.vIOP.Queue.Length())
		c.vIOW.Queue.PopN(c.vIOW.Queue.Length())
		c.vIOP.Queue.Add(pEls)
		c.vIOW.Queue.Add(wEls)
		assert.Equal(3, c.vIOP.Queue.Length())
		assert.Equal(3, c.vIOW.Queue.Length())
	}

	totalDropped := 0

	// empty queues
	n := c.vsIoMLimitBuffering()
	assert.Equal(0, n)

	// no errors, no buffering
	reset()
	c.VolumeIOMaxBuffered = 0
	n = c.vsIoMLimitBuffering()
	assert.Equal(6, n)
	totalDropped += n
	assert.Equal(totalDropped, c.numVIODropped)
	assert.Equal(0, c.vIOP.Queue.Length())
	assert.Equal(0, c.vIOW.Queue.Length())
	assert.Equal(1, tl.CountPattern("3 .* active .* writer"))
	assert.Equal(1, tl.CountPattern("3 .* active .* processor"))

	// no errors, limit impacts W only
	reset()
	c.VolumeIOMaxBuffered = 4
	n = c.vsIoMLimitBuffering()
	assert.Equal(2, n)
	totalDropped += n
	assert.Equal(totalDropped, c.numVIODropped)
	assert.Equal(3, c.vIOP.Queue.Length())
	assert.Equal(1, c.vIOW.Queue.Length())
	assert.Equal(1, tl.CountPattern("2 .* active .* writer"))

	// no errors, limit impacts W and P
	reset()
	c.VolumeIOMaxBuffered = 2
	n = c.vsIoMLimitBuffering()
	assert.Equal(4, n)
	totalDropped += n
	assert.Equal(totalDropped, c.numVIODropped)
	assert.Equal(2, c.vIOP.Queue.Length())
	assert.Equal(0, c.vIOW.Queue.Length())
	assert.Equal(1, tl.CountPattern("3 .* active .* writer"))
	assert.Equal(1, tl.CountPattern("1 .* active .* processor"))

	// P error, limit impacts P only
	reset()
	c.VolumeIOMaxBuffered = 2
	c.vIOP.InError = true
	c.vIOW.InError = false
	n = c.vsIoMLimitBuffering()
	assert.Equal(3, n)
	totalDropped += n
	assert.Equal(totalDropped, c.numVIODropped)
	assert.Equal(0, c.vIOP.Queue.Length())
	assert.Equal(3, c.vIOW.Queue.Length())
	assert.Equal(1, tl.CountPattern("3 .* inactive .* processor"))

	// W error, limit impacts W only
	reset()
	c.VolumeIOMaxBuffered = 2
	c.vIOP.InError = false
	c.vIOW.InError = true
	n = c.vsIoMLimitBuffering()
	assert.Equal(3, n)
	totalDropped += n
	assert.Equal(totalDropped, c.numVIODropped)
	assert.Equal(3, c.vIOP.Queue.Length())
	assert.Equal(0, c.vIOW.Queue.Length())
	assert.Equal(1, tl.CountPattern("3 .* inactive .* writer"))

	// Both W and P in error, limit impacts W and P
	reset()
	c.VolumeIOMaxBuffered = 2
	c.vIOP.InError = true
	c.vIOW.InError = true
	n = c.vsIoMLimitBuffering()
	assert.Equal(4, n)
	totalDropped += n
	assert.Equal(totalDropped, c.numVIODropped)
	assert.Equal(2, c.vIOP.Queue.Length())
	assert.Equal(0, c.vIOW.Queue.Length())
	assert.Equal(1, tl.CountPattern("3 .* inactive .* writer"))
	assert.Equal(1, tl.CountPattern("1 .* inactive .* processor"))

	// Test burst tolerance
	reset()
	c.VolumeIOMaxBuffered = 2
	c.VolumeIOBurstTolerance = 2
	c.numVIOCapacityExceeded = 0
	n = c.vsIoMLimitCapacityWithBurstTolerance()
	assert.Equal(0, n)
	totalDropped += n
	assert.Equal(totalDropped, c.numVIODropped)
	assert.Equal(1, c.numVIOCapacityExceeded)
	assert.Equal(3, c.vIOP.Queue.Length())
	assert.Equal(3, c.vIOW.Queue.Length())
	n = c.vsIoMLimitCapacityWithBurstTolerance()
	assert.Equal(0, n)
	totalDropped += n
	assert.Equal(totalDropped, c.numVIODropped)
	assert.Equal(2, c.numVIOCapacityExceeded)
	assert.Equal(3, c.vIOP.Queue.Length())
	assert.Equal(3, c.vIOW.Queue.Length())
	n = c.vsIoMLimitCapacityWithBurstTolerance()
	assert.Equal(4, n)
	totalDropped += n
	assert.Equal(totalDropped, c.numVIODropped)
	assert.Equal(0, c.numVIOCapacityExceeded)
	assert.Equal(2, c.vIOP.Queue.Length())
	assert.Equal(0, c.vIOW.Queue.Length())
}
