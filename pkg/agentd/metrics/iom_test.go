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
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/metricmover"
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

type fakeIoMetricDatumMaker struct {
	InNow                        time.Time
	InDuration                   time.Duration
	InObjID                      string
	InRS, InLRS, InWS, InLWS     *nuvoapi.StatsIO
	InCUS, InLCUS, InCMS, InLCMS *nuvoapi.StatsCache
	RetD                         *models.IoMetricDatum
	RetErr                       error
}

var _ = ioMetricDatumMaker(&fakeIoMetricDatumMaker{})

func (f *fakeIoMetricDatumMaker) makeIOMetricDatum(now time.Time, objID string, readStats, lastReadStats, writeStats, lastWriteStats *nuvoapi.StatsIO, cacheUserStat, lastCacheUserStat, cacheMetaStat, lastCacheMetaStat *nuvoapi.StatsCache) (*models.IoMetricDatum, error) {
	f.InNow = now
	f.InObjID = objID
	f.InRS = readStats
	f.InLRS = lastReadStats
	f.InWS = writeStats
	f.InLWS = lastWriteStats
	f.InCUS = cacheUserStat
	f.InLCUS = lastCacheUserStat
	f.InCMS = cacheMetaStat
	f.InLCMS = lastCacheMetaStat
	return f.RetD, f.RetErr
}

func TestDiffStatsIOVersion0(t *testing.T) {
	assert := assert.New(t)
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			MetricIoCollectionPeriod:   5 * time.Minute,
			MetricIoCollectionTruncate: 5 * time.Minute,
		},
	}
	c := &MetricComp{}
	c.app = app

	oS := &nuvoapi.StatsIO{
		Count:     1,
		SizeTotal: 100,
	}
	oS.LatencyHist = make([]uint64, 128)
	oS.SizeHist = make([]uint64, 32)
	nS := &nuvoapi.StatsIO{}
	nS.LatencyHist = make([]uint64, 128)
	nS.SizeHist = make([]uint64, 32)
	// Note: histogram tests done in metricmover
	cUS := &nuvoapi.StatsCache{
		IOReadTotal:               123,
		CacheIOReadLineTotalCount: 111,
		CacheIOReadLineHitCount:   99,
	}
	cMS := &nuvoapi.StatsCache{
		IOReadTotal:               246,
		CacheIOReadLineTotalCount: 222,
		CacheIOReadLineHitCount:   135,
	}

	// sanity check
	testutils.Clone(oS, nS)
	iom, err := c.diffStatsIO(nS, oS)
	assert.NoError(err)
	assert.EqualValues(0, iom.NumBytes)
	assert.EqualValues(0, iom.NumOperations)

	// diff check
	testutils.Clone(oS, nS)
	nS.Count += 2
	nS.SizeTotal += 200
	iom, err = c.diffStatsIO(nS, oS)
	assert.NoError(err)
	assert.EqualValues(2, iom.NumOperations)
	assert.EqualValues(200, iom.NumBytes)

	// make metric
	zIoM := &metricmover.IOMetricStats0{
		DurationNs:      c.app.MetricIoCollectionPeriod,
		ReadMetric:      *metricmover.NewIOMetric0(),
		WriteMetric:     *metricmover.NewIOMetric0(),
		CacheUserMetric: *metricmover.NewIOCacheMetric0(),
		CacheMetaMetric: *metricmover.NewIOCacheMetric0(),
	}
	sZIoM := zIoM.Marshal()
	now := time.Now()
	d, err := c.makeIOMetricDatum(now, "id", oS, oS, nS, nS, cUS, cUS, cMS, cMS) // zero data points ok for testing
	assert.NoError(err)
	assert.NotNil(d)
	assert.EqualValues(0, d.Version)
	assert.EqualValues("id", d.ObjectID)
	assert.Equal(now, time.Time(d.Timestamp))
	assert.Equal(sZIoM, d.Sample)

	// import error
	oS.SizeHist = make([]uint64, metricmover.StatIONumSizeHistBuckets-1)
	d, err = c.makeIOMetricDatum(now, "id", oS, oS, nS, nS, cUS, cUS, cMS, cMS) // zero data points ok for testing
	assert.Error(err)
}
