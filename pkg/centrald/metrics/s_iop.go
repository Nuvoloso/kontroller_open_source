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
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/util"
	units "github.com/docker/go-units"
	"github.com/go-openapi/swag"
)

// ConsumeStorageIoMetricData is part of the metricsmover.MetricsConsumer interface
func (c *Component) ConsumeStorageIoMetricData(data []*models.IoMetricDatum) {
	c.Log.Debugf("ConsumeStorageIoMetricData: %d", len(data))
	c.sIOP.EnqueueIOM(data)
}

// sIOMSample collects all the data required to construct a storage I/O metric record in the database
type sIOMSample struct {
	IoMetricStatisticsDatum
	// Storage compliance violations
	ViolationLatencyMean  int32
	ViolationLatencyMax   int32
	ViolationWorkloadRate int64
}

// sIOProcessor determines compliance fields of Storage I/O metric data
type sIOProcessor struct {
	c       *Component
	InError bool // indicates that the stage is not dequeuing
	Queue   *util.Queue
	worker  util.Worker
}

func (p *sIOProcessor) Init(c *Component) {
	p.c = c
	wa := &util.WorkerArgs{
		Name:          "sIOProcessor",
		Log:           p.c.Log,
		SleepInterval: p.c.RetryInterval,
	}
	p.worker, _ = util.NewWorker(wa, p)
	p.Queue = util.NewQueue(&models.IoMetricDatum{})
}

func (p *sIOProcessor) Start() {
	p.c.Log.Info("Starting sIO Metric Processor")
	p.worker.Start()
}

func (p *sIOProcessor) Stop() {
	p.worker.Stop()
	p.c.Log.Info("Stopped sIO Metric Processor")
}

func (p *sIOProcessor) EnqueueIOM(data []*models.IoMetricDatum) {
	p.Queue.Add(data)
	p.worker.Notify()
}

// Buzz satisfies the util.WorkerBee interface
func (p *sIOProcessor) Buzz(ctx context.Context) error {
	var err error
	p.InError = false
	for el := p.Queue.PeekHead(); el != nil; el = p.Queue.PeekHead() {
		d := el.(*models.IoMetricDatum)
		var sd *sIOMSample
		if sd, err = p.unpackDatum(d); err == nil {
			var s *models.Storage
			sID := string(d.ObjectID)
			if s, err = p.c.oCrud.StorageFetch(ctx, sID); err == nil {
				std := p.c.lookupStorageType(string(s.CspStorageType))
				if std != nil {
					p.checkForCompliance(sd, std, s)
					p.c.sIOW.EnqueueSample(sd)
				} else {
					p.c.Log.Errorf("Storage %s StorageType %s not cached", sID, s.CspStorageType)
					// drop the metric
				}
			} else {
				if e, ok := err.(*crud.Error); !ok || !e.NotFound() {
					break // retry later
				}
				// Storage not found: deletion race? ⇒ drop the metric
				err = nil
			}
		} else {
			p.c.Log.Errorf("Unable to unpack metric: %s", err.Error())
			err = nil // does not impact the queue
		}
		p.Queue.PopHead()
	}
	if err != nil {
		p.InError = true
		p.c.sIoMLimitBuffering()
	}
	return err
}

// unpackDatum converts the datum to internal form and sets derivable fields
func (p *sIOProcessor) unpackDatum(d *models.IoMetricDatum) (*sIOMSample, error) {
	sd := &sIOMSample{}
	if err := sd.IoMetricStatisticsDatum.Unpack(d); err != nil {
		return nil, err
	}
	return sd, nil
}

// checkForCompliance determines compliance with the storage type and sets the appropriate properties
func (p *sIOProcessor) checkForCompliance(sd *sIOMSample, std *StorageTypeData, s *models.Storage) {
	var b bytes.Buffer
	if std.ResponseTimeMeanUsec > 0 && sd.LatencyMeanUsec > std.ResponseTimeMeanUsec {
		fmt.Fprintf(&b, " ViolationLatencyMean(mean:%d)", std.ResponseTimeMeanUsec)
		sd.ViolationLatencyMean = 1
	}
	if std.ResponseTimeMaxUsec > 0 && sd.NumOps > 0 {
		stdMaxNs := uint64(std.ResponseTimeMaxUsec) * uint64(time.Microsecond)
		cnt := sd.LatencyCountOverThresholdNs(stdMaxNs)
		if sd.NumOps >= cnt { // sanity check
			pctOverMax := float64(cnt*100) / float64(sd.NumOps)
			if pctOverMax > p.c.StorageIOMaxResponseTimePct {
				fmt.Fprintf(&b, " LatencyMax(cnt:%d pctOverMax:%f)", cnt, pctOverMax)
				sd.ViolationLatencyMax = int32(cnt)
			}
		}
	}
	// Note: IOPsPerGiB and ThroughputBytesPerGiB are mutually exclusive
	sizeBytes := swag.Int64Value(s.SizeBytes)
	bytesPerGiB := float64(units.GiB)
	if std.IOPsPerGiB > 0 && sd.NumOps > 0 && sizeBytes > 0 {
		permittedSampleIOs := float64(std.IOPsPerGiB) * float64(sizeBytes) * float64(sd.SampleDurationSec) / bytesPerGiB
		diff := float64(sd.NumOps) - permittedSampleIOs
		if diff > 0.0 {
			fmt.Fprintf(&b, " WorkloadRateIOPs(io/s/GiB:%d s:%d permitted:%d actual:%d)", std.IOPsPerGiB, sd.SampleDurationSec, int(permittedSampleIOs), sd.NumOps)
			sd.ViolationWorkloadRate = int64(diff)
		}
	} else if std.BytesPerSecPerGiB > 0 && sd.NumBytes > 0 && sizeBytes > 0 {
		permittedSampleBytes := float64(std.BytesPerSecPerGiB) * float64(sizeBytes) * float64(sd.SampleDurationSec) / bytesPerGiB
		diff := float64(sd.NumBytes) - permittedSampleBytes
		if diff > 0.0 {
			fmt.Fprintf(&b, " WorkloadRateThroughput(bytes/s/GiB:%d s:%d permitted:%d actual:%d)", std.BytesPerSecPerGiB, sd.SampleDurationSec, int(permittedSampleBytes), sd.NumBytes)
			sd.ViolationWorkloadRate = int64(diff)
		}
	}
	if b.Len() > 0 {
		p.c.Log.Debugf("Violations in %#v ⇒%s", sd, b.String())
	}
}
