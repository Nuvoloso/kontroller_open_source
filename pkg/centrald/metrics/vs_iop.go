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

// ConsumeVolumeIoMetricData is part of the metricsmover.MetricsConsumer interface
func (c *Component) ConsumeVolumeIoMetricData(data []*models.IoMetricDatum) {
	c.Log.Debugf("ConsumeVolumeIoMetricData: %d", len(data))
	c.vIOP.EnqueueIOM(data)
}

// vsIOMSample collects all the data required to construct a VolumeSeries I/O metric record in the database
type vsIOMSample struct {
	IoMetricStatisticsDatum
	// VolumeSeries compliance violations
	ViolationLatencyMean        int32
	ViolationLatencyMax         int32
	ViolationWorkloadRate       int64
	ViolationWorkloadMixRead    int64
	ViolationWorkloadMixWrite   int64
	ViolationWorkloadAvgSizeMin int64
	ViolationWorkloadAvgSizeMax int64
}

// vsIOProcessor determines compliance fields of VolumeSeries I/O metric data
type vsIOProcessor struct {
	c       *Component
	InError bool // indicates that the stage is not dequeuing
	Queue   *util.Queue
	worker  util.Worker
}

func (p *vsIOProcessor) Init(c *Component) {
	p.c = c
	wa := &util.WorkerArgs{
		Name:          "vsIOProcessor",
		Log:           p.c.Log,
		SleepInterval: p.c.RetryInterval,
	}
	p.worker, _ = util.NewWorker(wa, p)
	p.Queue = util.NewQueue(&models.IoMetricDatum{})
}

func (p *vsIOProcessor) Start() {
	p.c.Log.Info("Starting vsIO Metric Processor")
	p.worker.Start()
}

func (p *vsIOProcessor) Stop() {
	p.worker.Stop()
	p.c.Log.Info("Stopped vsIO Metric Processor")
}

func (p *vsIOProcessor) EnqueueIOM(data []*models.IoMetricDatum) {
	p.Queue.Add(data)
	p.worker.Notify()
}

// Buzz satisfies the util.WorkerBee interface
func (p *vsIOProcessor) Buzz(ctx context.Context) error {
	var err error
	p.InError = false
	for el := p.Queue.PeekHead(); el != nil; el = p.Queue.PeekHead() {
		d := el.(*models.IoMetricDatum)
		var sd *vsIOMSample
		if sd, err = p.unpackDatum(d); err == nil {
			var vs *models.VolumeSeries
			vsID := string(d.ObjectID)
			if vs, err = p.c.oCrud.VolumeSeriesFetch(ctx, vsID); err == nil {
				spd := p.c.lookupServicePlan(string(vs.ServicePlanID))
				if spd != nil {
					p.checkForCompliance(sd, spd, vs)
					p.c.vIOW.EnqueueSample(sd)
				} else {
					p.c.Log.Errorf("VolumeSeries %s ServicePlan %s not cached", vsID, vs.ServicePlanID)
					// drop the metric
				}
			} else {
				if e, ok := err.(*crud.Error); !ok || !e.NotFound() {
					break // retry later
				}
				// VolumeSeries not found: deletion race? ⇒ drop the metric
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
		p.c.vsIoMLimitBuffering()
	}
	return err
}

// unpackDatum converts the datum to internal form and sets derivable fields
func (p *vsIOProcessor) unpackDatum(d *models.IoMetricDatum) (*vsIOMSample, error) {
	sd := &vsIOMSample{}
	if err := sd.IoMetricStatisticsDatum.Unpack(d); err != nil {
		return nil, err
	}
	return sd, nil
}

// checkForCompliance determines compliance with the service plan and sets the appropriate properties
func (p *vsIOProcessor) checkForCompliance(sd *vsIOMSample, spd *ServicePlanData, vs *models.VolumeSeries) {
	var b bytes.Buffer
	if spd.ResponseTimeMeanUsec > 0 && sd.LatencyMeanUsec > spd.ResponseTimeMeanUsec {
		fmt.Fprintf(&b, " ViolationLatencyMean(mean:%d)", spd.ResponseTimeMeanUsec)
		sd.ViolationLatencyMean = 1
	}
	if spd.ResponseTimeMaxUsec > 0 && sd.NumOps > 0 {
		spdMaxNs := uint64(spd.ResponseTimeMaxUsec) * uint64(time.Microsecond)
		cnt := sd.LatencyCountOverThresholdNs(spdMaxNs)
		if sd.NumOps >= cnt { // sanity check
			pctOverMax := float64(cnt*100) / float64(sd.NumOps)
			if pctOverMax > p.c.VolumeIOMaxResponseTimePct {
				fmt.Fprintf(&b, " LatencyMax(cnt:%d pctOverMax:%f)", cnt, pctOverMax)
				sd.ViolationLatencyMax = int32(cnt)
			}
		}
	}
	// Note: IOPsPerGiB and ThroughputBytesPerGiB are mutually exclusive
	volSizeBytes := swag.Int64Value(vs.SizeBytes) + swag.Int64Value(vs.SpaAdditionalBytes)
	bytesPerGiB := float64(units.GiB)
	maxWritePercent := 100 - spd.MinReadPercent
	if spd.IOPsPerGiB > 0 && sd.NumOps > 0 && volSizeBytes > 0 {
		permittedSampleIOs := float64(spd.IOPsPerGiB) * float64(volSizeBytes) * float64(sd.SampleDurationSec) / bytesPerGiB
		diff := float64(sd.NumOps) - permittedSampleIOs
		if diff > 0.0 {
			fmt.Fprintf(&b, " WorkloadRateIOPs(io/s/GiB:%d s:%d permitted:%d actual:%d)", spd.IOPsPerGiB, sd.SampleDurationSec, int(permittedSampleIOs), sd.NumOps)
			sd.ViolationWorkloadRate = int64(diff)
		}
		permittedReadIOs := permittedSampleIOs * float64(spd.MaxReadPercent) / 100.0
		diff = float64(sd.ReadNumOps) - permittedReadIOs
		if diff > 0.0 {
			fmt.Fprintf(&b, " ViolationWorkloadMixRead(io/s/GiB:%d s:%d maxR%%:%d permitted:%d actual:%d)", spd.IOPsPerGiB, sd.SampleDurationSec, int(spd.MaxReadPercent), int(permittedReadIOs), sd.ReadNumOps)
			sd.ViolationWorkloadMixRead = int64(diff)
		}
		permittedWriteIOs := permittedSampleIOs * float64(maxWritePercent) / 100.0
		diff = float64(sd.WriteNumOps) - permittedWriteIOs
		if diff > 0.0 {
			fmt.Fprintf(&b, " ViolationWorkloadMixWrite(io/s/GiB:%d s:%d maxW%%:%d permitted:%d actual:%d)", spd.IOPsPerGiB, sd.SampleDurationSec, maxWritePercent, int(permittedWriteIOs), sd.WriteNumOps)
			sd.ViolationWorkloadMixWrite = int64(diff)
		}
	} else if spd.BytesPerSecPerGiB > 0 && sd.NumBytes > 0 && volSizeBytes > 0 {
		permittedSampleBytes := float64(spd.BytesPerSecPerGiB) * float64(volSizeBytes) * float64(sd.SampleDurationSec) / bytesPerGiB
		diff := float64(sd.NumBytes) - permittedSampleBytes
		if diff > 0.0 {
			fmt.Fprintf(&b, " WorkloadRateThroughput(bytes/s/GiB:%d s:%d permitted:%d actual:%d)", spd.BytesPerSecPerGiB, sd.SampleDurationSec, int(permittedSampleBytes), sd.NumBytes)
			sd.ViolationWorkloadRate = int64(diff)
		}
		permittedReadBytes := permittedSampleBytes * float64(spd.MaxReadPercent) / 100.0
		diff = float64(sd.ReadNumBytes) - permittedReadBytes
		if diff > 0.0 {
			fmt.Fprintf(&b, " ViolationWorkloadMixRead(bytes/s/GiB:%d s:%d maxR%%:%d permitted:%d actual:%d)", spd.BytesPerSecPerGiB, sd.SampleDurationSec, int(spd.MaxReadPercent), int(permittedSampleBytes), sd.ReadNumBytes)
			sd.ViolationWorkloadMixRead = int64(diff)
		}
		permittedWriteBytes := permittedSampleBytes * float64(maxWritePercent) / 100.0
		diff = float64(sd.WriteNumBytes) - permittedWriteBytes
		if diff > 0.0 {
			fmt.Fprintf(&b, " ViolationWorkloadMixWrite(bytes/s/GiB:%d s:%d maxR%%:%d permitted:%d actual:%d)", spd.BytesPerSecPerGiB, sd.SampleDurationSec, int(maxWritePercent), int(permittedSampleBytes), sd.WriteNumBytes)
			sd.ViolationWorkloadMixWrite = int64(diff)
		}
	}
	if sd.NumOps > 0 {
		diff := int64(spd.MinAvgSizeBytes) - int64(sd.AvgIOSize)
		if diff > 0 {
			fmt.Fprintf(&b, " ViolationWorkloadAvgSizeMin(minAvgSizeBytes:%d actual:%d)", spd.MinAvgSizeBytes, sd.AvgIOSize)
			sd.ViolationWorkloadAvgSizeMin = diff
		}
		diff = int64(sd.AvgIOSize) - int64(spd.MaxAvgSizeBytes)
		if diff > 0 {
			fmt.Fprintf(&b, " ViolationWorkloadAvgSizeMax(maxAvgSizeBytes:%d actual:%d)", spd.MaxAvgSizeBytes, sd.AvgIOSize)
			sd.ViolationWorkloadAvgSizeMax = diff
		}
	}
	if b.Len() > 0 {
		p.c.Log.Debugf("Violations in %#v ⇒%s", sd, b.String())
	}
}
