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

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
)

func (c *MetricComp) publishStorageIOStats() {
	now := time.Now()
	md := []*models.IoMetricDatum{}
	for _, storage := range c.app.GetStorage() {
		m, _ := c.getStorageIOStats(now, storage)
		if m != nil {
			md = append(md, m)
		}
	}
	if len(md) > 0 {
		c.app.MetricMover.PublishStorageIoMetrics(md)
	}
	c.cntSIo++
}

func (c *MetricComp) getStorageIOStats(now time.Time, storage *agentd.Storage) (*models.IoMetricDatum, error) {
	c.Log.Debugf("NUVOAPI GetStats(true, true, false, %s) R", storage.StorageID)
	readStats, err := c.app.NuvoAPI.GetStats(true, true, false, storage.StorageID)
	if err != nil {
		c.Log.Warningf("NUVOAPI GetStats(device, read, %s): %s", storage.StorageID, err.Error())
		return nil, err
	}
	c.Log.Debugf("NUVOAPI Metrics on Storage %s (%s) READ: %v", storage.StorageID, storage.DeviceName, readStats)
	c.Log.Debugf("NUVOAPI GetStats(true, false, false, %s) W", storage.StorageID)
	writeStats, err := c.app.NuvoAPI.GetStats(true, false, false, storage.StorageID)
	if err != nil {
		c.Log.Warningf("NUVOAPI GetStats(device, write, %s): %s", storage.StorageID, err.Error())
		return nil, err
	}
	c.Log.Debugf("NUVOAPI Metrics on Storage %s (%s) WRITE: %v", storage.StorageID, storage.DeviceName, writeStats)
	var datum *models.IoMetricDatum
	if storage.LastReadStat != nil && storage.LastWriteStat != nil {
		if readStats.SeriesUUID == storage.LastReadStat.SeriesUUID && writeStats.SeriesUUID == storage.LastWriteStat.SeriesUUID {
			// Note: makeIOMetricDatum is common function for both volume and storage device metrics with the last 4 parameters being cache stats present for volumes only
			if datum, err = c.ioMDMaker.makeIOMetricDatum(now, storage.StorageID, readStats, storage.LastReadStat, writeStats, storage.LastWriteStat, nil, nil, nil, nil); err != nil {
				c.Log.Warningf("Unable to process nuvoapi metric data: %s", err.Error())
				return nil, err
			}
		} else {
			c.Log.Debugf("Storage %s: series UUID changed", storage.StorageID)
		}
	} else {
		c.Log.Debugf("Storage %s: skipping first I/O stat", storage.StorageID)
	}
	storage.LastReadStat = readStats
	storage.LastWriteStat = writeStats
	return datum, nil
}
