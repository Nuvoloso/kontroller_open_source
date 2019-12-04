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
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/metricmover"
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	"github.com/go-openapi/strfmt"
)

// makeIOMetricDatum constructs an IO metric datum against the build version of the nuvoapi StatsIO structure
// Note: it is common function for both volume and storage device metrics with the last 4 parameters being cache stats present for volumes only
func (c *MetricComp) makeIOMetricDatum(now time.Time, objID string, readStats, lastReadStats, writeStats, lastWriteStats *nuvoapi.StatsIO, cacheUserStat, lastCacheUserStat, cacheMetaStat, lastCacheMetaStat *nuvoapi.StatsCache) (*models.IoMetricDatum, error) {
	var rM, wM *metricmover.IOMetric0
	var err error
	if rM, err = c.diffStatsIO(readStats, lastReadStats); err == nil {
		wM, err = c.diffStatsIO(writeStats, lastWriteStats)
	}
	if err != nil {
		return nil, err
	}
	ioMS := &metricmover.IOMetricStats0{
		DurationNs:  c.app.MetricIoCollectionPeriod,
		ReadMetric:  *rM,
		WriteMetric: *wM,
	}
	if cacheUserStat != nil && lastCacheUserStat != nil && cacheMetaStat != nil && lastCacheMetaStat != nil {
		cuM := c.diffStatsCache(cacheUserStat, lastCacheUserStat)
		cmM := c.diffStatsCache(cacheMetaStat, lastCacheMetaStat)
		ioMS.CacheUserMetric = *cuM
		ioMS.CacheMetaMetric = *cmM
	}
	datum := &models.IoMetricDatum{
		Timestamp: strfmt.DateTime(now),
		ObjectID:  models.ObjIDMutable(objID),
		Version:   0,
		Sample:    ioMS.Marshal(),
	}
	return datum, nil
}

func (c *MetricComp) diffStatsIO(nS, oS *nuvoapi.StatsIO) (*metricmover.IOMetric0, error) {
	nIOM := metricmover.NewIOMetric0()
	oIOM := metricmover.NewIOMetric0()
	var err error
	if err = nIOM.FromStatsIO(nS); err == nil {
		err = oIOM.FromStatsIO(oS)
	}
	if err != nil {
		return nil, err
	}
	return nIOM.Subtract(oIOM), nil
}

func (c *MetricComp) diffStatsCache(nS, oS *nuvoapi.StatsCache) *metricmover.IOCacheMetric0 {
	cM := metricmover.NewIOCacheMetric0()
	cM.NumReadHits = nS.CacheIOReadLineHitCount - oS.CacheIOReadLineHitCount
	cM.NumReadTotal = nS.IOReadTotal - oS.IOReadTotal
	return cM
}
