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
	"fmt"
	"math"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/metricmover"
)

// IoMetricStatisticsDatum is an intermediate data structure containing common I/O metric statistics
// fields loaded from different models.IoMetricDatum versions.
// It provides helper methods to hide the input versioning.
type IoMetricStatisticsDatum struct {
	ObjectID              string
	Version               int32
	Timestamp             time.Time
	SampleDurationSec     int32
	NumOps                uint64
	NumBytes              uint64
	ReadNumOps            uint64
	ReadNumBytes          uint64
	WriteNumOps           uint64
	WriteNumBytes         uint64
	NumCacheReadUserHits  uint64
	NumCacheReadUserTotal uint64
	NumCacheReadMetaHits  uint64
	NumCacheReadMetaTotal uint64
	LatencyMeanUsec       int32
	LatencyMaxUsec        int32
	AvgIOSize             uint64
	data0                 *metricmover.IOMetricStats0
}

// Unpack initializes from a models.IoMetricDatum.
func (sd *IoMetricStatisticsDatum) Unpack(d *models.IoMetricDatum) error {
	sd.ObjectID = string(d.ObjectID)
	sd.Version = d.Version
	switch sd.Version {
	case 0:
		return sd.unpack0(d)
	}
	return fmt.Errorf("invalid metric version %d", d.Version)
}

// unpack0 supports metricmover.IOMetricStats0
func (sd *IoMetricStatisticsDatum) unpack0(d *models.IoMetricDatum) error {
	data0 := &metricmover.IOMetricStats0{}
	if err := data0.Unmarshal(d.Sample); err != nil {
		return err
	}
	sd.data0 = data0
	sd.Timestamp = time.Time(d.Timestamp)
	rM := &sd.data0.ReadMetric
	wM := &sd.data0.WriteMetric
	cuM := &sd.data0.CacheUserMetric
	cmM := &sd.data0.CacheMetaMetric
	numOps := rM.NumOperations + wM.NumOperations
	// TBD: golang 1.10 has math.Round()
	if numOps > 0 {
		sd.LatencyMeanUsec = int32((rM.LatencyNsMean*float64(rM.NumOperations) + wM.LatencyNsMean*float64(wM.NumOperations)) / float64(numOps) / float64(time.Microsecond))
	}
	rLMax := rM.LatencyMaxNs() / float64(time.Microsecond)
	wLMax := wM.LatencyMaxNs() / float64(time.Microsecond)
	sd.LatencyMaxUsec = int32(math.Max(rLMax, wLMax))
	sd.NumOps = numOps
	sd.SampleDurationSec = int32(sd.data0.DurationNs / time.Second)
	sd.NumBytes = rM.NumBytes + wM.NumBytes
	sd.ReadNumBytes = rM.NumBytes
	sd.WriteNumBytes = wM.NumBytes
	sd.ReadNumOps = rM.NumOperations
	sd.WriteNumOps = wM.NumOperations
	if sd.NumOps > 0 {
		sd.AvgIOSize = (sd.ReadNumBytes + sd.WriteNumBytes) / sd.NumOps
	}
	sd.NumCacheReadUserHits = cuM.NumReadHits
	sd.NumCacheReadUserTotal = cuM.NumReadTotal
	sd.NumCacheReadMetaHits = cmM.NumReadHits
	sd.NumCacheReadMetaTotal = cmM.NumReadTotal
	return nil
}

// LatencyCountOverThresholdNs uses the underlying metricmover method to add the
// read and write histogram values in a version independent manner.
func (sd *IoMetricStatisticsDatum) LatencyCountOverThresholdNs(maxNs uint64) uint64 {
	if sd.Version == 0 {
		return sd.data0.ReadMetric.LatencyCountOverThresholdNs(maxNs) +
			sd.data0.WriteMetric.LatencyCountOverThresholdNs(maxNs)
	}
	return 0
}
