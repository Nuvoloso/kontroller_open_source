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

	"github.com/Nuvoloso/kontroller/pkg/testutils"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/metricmover"
	"github.com/go-openapi/strfmt"
	"github.com/stretchr/testify/assert"
)

func TestIOMStatisticsDatum0(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	sd, err := makeSampleIoMetricStatisticsDatum(now)
	assert.NoError(err)
	assert.NotNil(sd)
	assert.EqualValues(0, sd.Version)
	assert.Equal("OBJ-1", sd.ObjectID)
	assert.EqualValues(5*60, sd.SampleDurationSec)
	assert.Equal(now, sd.Timestamp)
	assert.Equal(int32(2333), sd.LatencyMeanUsec)
	assert.Equal(int32(201326), sd.LatencyMaxUsec) // midpoint of write bucket 18
	assert.EqualValues(15, sd.NumOps)
	assert.EqualValues(21000, sd.NumBytes)
	assert.EqualValues(10, sd.ReadNumOps)
	assert.EqualValues(5, sd.WriteNumOps)
	assert.EqualValues(1000, sd.ReadNumBytes)
	assert.EqualValues(20000, sd.WriteNumBytes)
	assert.EqualValues(1400, sd.AvgIOSize)
	assert.EqualValues(7, sd.NumCacheReadUserHits)
	assert.EqualValues(10, sd.NumCacheReadUserTotal)
	assert.EqualValues(17, sd.NumCacheReadMetaHits)
	assert.EqualValues(20, sd.NumCacheReadMetaTotal)

	assert.NotNil(sd.data0)
	maxNs := metricmover.IOMetric0LatencyNsBucketMidpoint(18) / uint64(time.Microsecond)
	assert.EqualValues(201326, maxNs)
	cnt := sd.LatencyCountOverThresholdNs(maxNs)
	rCnt := sd.data0.ReadMetric.LatencyCountOverThresholdNs(maxNs)
	wCnt := sd.data0.WriteMetric.LatencyCountOverThresholdNs(maxNs)
	assert.NotZero(cnt)
	assert.EqualValues(10, rCnt)
	assert.EqualValues(10, wCnt)
	assert.Equal(cnt, rCnt+wCnt)

	// latency count default
	sd.Version = -1
	assert.EqualValues(0, sd.LatencyCountOverThresholdNs(maxNs))

	// zero sample should not cause a divide-by-zero error
	d := &models.IoMetricDatum{
		Timestamp: strfmt.DateTime(now),
		ObjectID:  "OBJ-1",
		Version:   0,
	}
	s0 := &metricmover.IOMetricStats0{
		ReadMetric:  *metricmover.NewIOMetric0(),
		WriteMetric: *metricmover.NewIOMetric0(),
	}
	d.Sample = s0.Marshal()
	sd = &IoMetricStatisticsDatum{}
	err = sd.Unpack(d)
	assert.NoError(err)
	assert.Equal(now, sd.Timestamp)
	assert.Equal(int32(0), sd.LatencyMeanUsec)
	assert.Equal(int32(0), sd.LatencyMaxUsec)
	assert.EqualValues(0, sd.AvgIOSize)

	// unmarshal error
	d.Sample = "foobar"
	sd = &IoMetricStatisticsDatum{}
	err = sd.Unpack(d)
	assert.Error(err)
}

// useful samples lifted from TestVsIOPCompliance
func makeSampleIOMetricDatum(now time.Time) *models.IoMetricDatum {
	ios := &metricmover.IOMetricStats0{
		DurationNs:      5 * time.Minute,
		ReadMetric:      *metricmover.NewIOMetric0(),
		WriteMetric:     *metricmover.NewIOMetric0(),
		CacheUserMetric: *metricmover.NewIOCacheMetric0(),
		CacheMetaMetric: *metricmover.NewIOCacheMetric0(),
	}
	rM := &ios.ReadMetric
	rM.NumOperations = 10
	rM.NumBytes = 1000
	rM.LatencyNsMean = 2300000.999
	rM.LatencyNsHist[13] = 4
	rM.LatencyNsHist[14] = 6
	wM := &ios.WriteMetric
	wM.NumOperations = 5
	wM.NumBytes = 20000
	wM.LatencyNsMean = 2400000.001
	wM.LatencyNsHist[14] = 8
	wM.LatencyNsHist[18] = 2
	cuM := &ios.CacheUserMetric
	cuM.NumReadHits = 7
	cuM.NumReadTotal = 10
	cmM := &ios.CacheMetaMetric
	cmM.NumReadHits = 17
	cmM.NumReadTotal = 20
	return &models.IoMetricDatum{
		Timestamp: strfmt.DateTime(now),
		ObjectID:  "OBJ-1",
		Version:   0,
		Sample:    ios.Marshal(),
	}
}

func makeSampleIoMetricStatisticsDatum(now time.Time) (*IoMetricStatisticsDatum, error) {
	d := makeSampleIOMetricDatum(now)
	sd := &IoMetricStatisticsDatum{}
	return sd, sd.Unpack(d)
}

func cloneIoMetricStatisticsDatum(src, dst *IoMetricStatisticsDatum) {
	testutils.Clone(src, dst)
	if src.Version == 0 {
		dst.data0 = &metricmover.IOMetricStats0{}
		testutils.Clone(src.data0, dst.data0)
	}
}
