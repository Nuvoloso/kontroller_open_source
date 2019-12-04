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
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/Nuvoloso/kontroller/pkg/metricmover"
	fmm "github.com/Nuvoloso/kontroller/pkg/metricmover/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	units "github.com/docker/go-units"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestVsIOPInitAndUpload(t *testing.T) {
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

	assert.NotNil(c.vIOP.worker)
	assert.NotNil(c.vIOP.Queue)
	assert.NotNil(c.vIOW.worker)
	assert.NotNil(c.vIOW.Queue)

	c.ConsumeVolumeIoMetricData([]*models.IoMetricDatum{&models.IoMetricDatum{}, &models.IoMetricDatum{}})
	assert.Equal(2, c.vIOP.Queue.Length())
}

func TestVsIOPBuzz(t *testing.T) {
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
	fc := &fake.Client{}
	c.oCrud = fc
	c.VolumeIOMaxBuffered = VolumeIOMaxBufferedDefault

	p := &c.vIOP
	w := &c.vIOW

	// construct a non-zero sample
	now := time.Now()
	d := makeSampleIOMetricDatum(now)
	sd, err := p.unpackDatum(d)
	assert.NoError(err)
	assert.NotNil(sd)

	// Buzz: unpack error (metric dropped)
	tl.Flush()
	p.EnqueueIOM([]*models.IoMetricDatum{d})
	assert.Equal(1, p.Queue.Length())
	d.Version = -1
	p.InError = true
	err = p.Buzz(nil)
	assert.NoError(err)
	assert.Equal(0, p.Queue.Length())
	assert.Equal(1, tl.CountPattern("unpack"))
	assert.False(p.InError)

	// Buzz: volume fetch error (retryable, queue not flushed, error returned)
	d.Version = 0
	p.EnqueueIOM([]*models.IoMetricDatum{d})
	assert.Equal(1, p.Queue.Length())
	fc.RetVErr = fmt.Errorf("volume-fetch-error")
	err = p.Buzz(nil)
	assert.Error(err)
	assert.Regexp("volume-fetch-error", err)
	assert.Equal(1, p.Queue.Length())
	assert.True(p.InError)

	// Buzz: volume fetch NotFound error (metric dropped)
	err = crud.NewError(&models.Error{Code: 404, Message: swag.String(com.ErrorNotFound)})
	ce, ok := err.(*crud.Error)
	assert.True(ok)
	assert.True(ce.NotFound())
	fc.RetVErr = ce
	assert.Equal(1, p.Queue.Length())
	err = p.Buzz(nil)
	assert.NoError(err)
	assert.Equal(0, p.Queue.Length())
	assert.False(p.InError)

	// Buzz: service plan not cached (metric dropped)
	tl.Flush()
	p.EnqueueIOM([]*models.IoMetricDatum{d})
	assert.Empty(c.servicePlanCache)
	vsObj := &models.VolumeSeries{
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				ServicePlanID: "SP-1",
			},
		},
	}
	fc.RetVErr = nil
	fc.RetVObj = vsObj
	assert.Equal(1, p.Queue.Length())
	err = p.Buzz(nil)
	assert.NoError(err)
	assert.Equal(0, p.Queue.Length())
	assert.Equal(1, tl.CountPattern("ServicePlan.* not cached"))
	assert.False(p.InError)

	// Buzz: no error, record enqueued for writing
	p.EnqueueIOM([]*models.IoMetricDatum{d})
	assert.Equal(0, w.Queue.Length())
	spd := &ServicePlanData{}
	c.cacheServicePlan(string(vsObj.ServicePlanID), spd)
	assert.Equal(1, p.Queue.Length())
	p.InError = true
	err = p.Buzz(nil)
	assert.NoError(err)
	assert.Equal(0, p.Queue.Length())
	assert.Equal(1, w.Queue.Length())
	sd = w.Queue.PeekHead().(*vsIOMSample)
	assert.Equal("OBJ-1", sd.ObjectID)
	assert.Equal(now, sd.Timestamp)
	assert.False(p.InError)
}

func TestVsIOPCompliance(t *testing.T) {
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

	p := &c.vIOP

	// construct an sd
	now := time.Now()
	d := &models.IoMetricDatum{
		Timestamp: strfmt.DateTime(now),
		ObjectID:  "VS-1",
		Version:   0,
		Sample:    "badsample",
	}
	ios := &metricmover.IOMetricStats0{
		DurationNs:  5 * time.Minute,
		ReadMetric:  *metricmover.NewIOMetric0(),
		WriteMetric: *metricmover.NewIOMetric0(),
	}
	rM := &ios.ReadMetric
	rM.NumOperations = 10
	rM.NumBytes = 1000
	rM.LatencyNsMean = 2300000.999
	rM.LatencyNsHist[13] = 4
	rM.LatencyNsHist[14] = 6
	wM := &ios.WriteMetric
	wM.NumOperations = 10
	wM.NumBytes = 1000
	wM.LatencyNsMean = 2400000.001
	wM.LatencyNsHist[14] = 8
	wM.LatencyNsHist[18] = 2
	d.Sample = ios.Marshal()
	sd, err := p.unpackDatum(d)
	assert.NoError(err)
	assert.NotNil(sd)
	assert.EqualValues(5*60, sd.SampleDurationSec)
	assert.Equal(now, sd.Timestamp)
	assert.Equal(int32(2350), sd.LatencyMeanUsec)
	assert.Equal(int32(201326), sd.LatencyMaxUsec) // midpoint of write bucket 18
	assert.EqualValues(20, sd.NumOps)
	assert.EqualValues(2000, sd.NumBytes)
	assert.EqualValues(10, sd.ReadNumOps)
	assert.EqualValues(10, sd.WriteNumOps)
	assert.EqualValues(1000, sd.ReadNumBytes)
	assert.EqualValues(1000, sd.WriteNumBytes)

	sdBase := &vsIOMSample{}
	cloneVsIOMSample(sd, sdBase) // to restore from later

	vsObj := &models.VolumeSeries{
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				ServicePlanID: "SP-1",
				SizeBytes:     swag.Int64(20 * units.GiB),
			},
		},
	}
	adjVs := vsClone(vsObj)
	adjVs.SpaAdditionalBytes = swag.Int64(10 * units.GiB)

	// zero spd
	spd := &ServicePlanData{}
	p.checkForCompliance(sd, spd, vsObj)
	assert.EqualValues(0, sd.ViolationLatencyMean)
	assert.EqualValues(0, sd.ViolationLatencyMax)
	assert.EqualValues(0, sd.ViolationWorkloadRate)
	assert.EqualValues(0, sd.ViolationWorkloadMixRead)
	assert.EqualValues(0, sd.ViolationWorkloadMixWrite)

	// The code below is organized as needed as we want to change the easiest operand to cause a violation
	// and this will interfere across the calculations for each type of violation.
	// Violations are all non-zero - check for non-violations first.

	// Latency
	// Bucket 18 is selected as the bucket that will violate the response time maximum
	tl.Logger().Info("*** Latency checks")
	tl.Flush()
	sd = &vsIOMSample{}
	cloneVsIOMSample(sdBase, sd) // restore
	spdRTMaxUsecInViolation := int32(metricmover.IOMetric0LatencyNsBucketMidpoint(18) / uint64(time.Microsecond))
	spdRTMaxUsecNotInViolation := int32(metricmover.IOMetric0LatencyNsBucketMidpoint(19) / uint64(time.Microsecond))
	// non-zero spd, no violations
	spd = &ServicePlanData{}
	spd.ResponseTimeMeanUsec = sd.LatencyMeanUsec + 1
	assert.True(spd.ResponseTimeMeanUsec > 0)
	spd.ResponseTimeMaxUsec = spdRTMaxUsecNotInViolation
	p.checkForCompliance(sd, spd, vsObj)
	assert.EqualValues(0, sd.ViolationLatencyMean)
	assert.EqualValues(0, sd.ViolationLatencyMax)
	// non-zero spd, violations
	spd.ResponseTimeMeanUsec = sd.LatencyMeanUsec - 1
	assert.True(spd.ResponseTimeMeanUsec > 0)
	spd.ResponseTimeMaxUsec = spdRTMaxUsecInViolation
	p.checkForCompliance(sd, spd, vsObj)
	assert.EqualValues(1, sd.ViolationLatencyMean)
	assert.EqualValues(2, sd.ViolationLatencyMax)

	// Workload Rate (IOPS)
	spd = &ServicePlanData{}
	spd.IOPsPerGiB = 40
	spd.BytesPerSecPerGiB = 0
	spd.MaxReadPercent = 70
	spd.MinReadPercent = 45
	// Given 40IOPS/GiB, a sample duration of 300s and volume size of 20GiB,
	// the permitted number of I/Os for the sample is: 40 * 20 * 300 = 240000
	// 70 maxR% ⇒ permitted read I/O = 168000
	// 45 minR% ⇒ 55 maxW% ⇒ permitted write I/O = 132000
	numOpsInViolation := uint64(240001)
	numOpsNotInViolation := uint64(240000)
	numROpsInViolation := uint64(168001)
	numWOpsInViolation := uint64(132001)
	// non-zero spd, no violations
	tl.Logger().Info("*** Workload Rate IOPS: No violations")
	tl.Flush()
	sd = &vsIOMSample{}
	cloneVsIOMSample(sdBase, sd) // restore
	sd.NumOps = numOpsNotInViolation
	p.checkForCompliance(sd, spd, vsObj)
	assert.EqualValues(0, sd.ViolationWorkloadRate)
	assert.EqualValues(0, sd.ViolationWorkloadMixRead)
	assert.EqualValues(0, sd.ViolationWorkloadMixWrite)
	// non-zero spd, ViolationWorkloadRate
	tl.Logger().Info("*** Workload Rate IOPS: ViolationWorkloadRate")
	tl.Flush()
	sd = &vsIOMSample{}
	cloneVsIOMSample(sdBase, sd) // restore
	sd.NumOps = numOpsInViolation
	p.checkForCompliance(sd, spd, vsObj)
	assert.EqualValues(1, sd.ViolationWorkloadRate)
	assert.EqualValues(0, sd.ViolationWorkloadMixRead)
	assert.EqualValues(0, sd.ViolationWorkloadMixWrite)
	// non-zero spd, ViolationWorkloadRate erased by adjusted size
	tl.Logger().Info("*** Workload Rate IOPS: ViolationWorkloadRate erased by adjusted size")
	tl.Flush()
	sd = &vsIOMSample{}
	cloneVsIOMSample(sdBase, sd) // restore
	sd.NumOps = numOpsInViolation
	p.checkForCompliance(sd, spd, adjVs)
	assert.EqualValues(0, sd.ViolationWorkloadRate)
	assert.EqualValues(0, sd.ViolationWorkloadMixRead)
	assert.EqualValues(0, sd.ViolationWorkloadMixWrite)
	// non-zero spd, ViolationWorkloadMixRead
	tl.Logger().Info("*** Workload Rate IOPS: ViolationWorkloadMixRead")
	tl.Flush()
	sd = &vsIOMSample{}
	cloneVsIOMSample(sdBase, sd) // restore
	sd.ReadNumOps = numROpsInViolation
	p.checkForCompliance(sd, spd, vsObj)
	assert.EqualValues(0, sd.ViolationWorkloadRate)
	assert.EqualValues(1, sd.ViolationWorkloadMixRead)
	assert.EqualValues(0, sd.ViolationWorkloadMixWrite)
	// non-zero spd, ViolationWorkloadMixRead erased by adjusted size
	tl.Logger().Info("*** Workload Rate IOPS: ViolationWorkloadMixRead erased by adjusted size")
	tl.Flush()
	sd = &vsIOMSample{}
	cloneVsIOMSample(sdBase, sd) // restore
	sd.ReadNumOps = numROpsInViolation
	p.checkForCompliance(sd, spd, adjVs)
	assert.EqualValues(0, sd.ViolationWorkloadRate)
	assert.EqualValues(0, sd.ViolationWorkloadMixRead)
	assert.EqualValues(0, sd.ViolationWorkloadMixWrite)
	// non-zero spd, ViolationWorkloadMixWrite
	tl.Logger().Info("*** Workload Rate IOPS: ViolationWorkloadMixWrite")
	tl.Flush()
	sd = &vsIOMSample{}
	cloneVsIOMSample(sdBase, sd) // restore
	sd.WriteNumOps = numWOpsInViolation
	p.checkForCompliance(sd, spd, vsObj)
	assert.EqualValues(0, sd.ViolationWorkloadRate)
	assert.EqualValues(0, sd.ViolationWorkloadMixRead)
	assert.EqualValues(1, sd.ViolationWorkloadMixWrite)
	// non-zero spd, ViolationWorkloadMixWrite erased by adjusted size
	tl.Logger().Info("*** Workload Rate IOPS: ViolationWorkloadMixWrite erased by adjusted size")
	tl.Flush()
	sd = &vsIOMSample{}
	cloneVsIOMSample(sdBase, sd) // restore
	sd.WriteNumOps = numWOpsInViolation
	p.checkForCompliance(sd, spd, adjVs)
	assert.EqualValues(0, sd.ViolationWorkloadRate)
	assert.EqualValues(0, sd.ViolationWorkloadMixRead)
	assert.EqualValues(0, sd.ViolationWorkloadMixWrite)

	// Workload Rate (Throughput)
	spd = &ServicePlanData{}
	spd.IOPsPerGiB = 0
	spd.BytesPerSecPerGiB = 2000000
	spd.MaxReadPercent = 70
	spd.MinReadPercent = 40
	// Given 2000000 B/s/GiB, a sample duration of 300s and a volume size of 20GiB
	// the permitted number of bytes for the sample is: 2000000 * 20 * 300 = 12000000000
	// 70 maxR% ⇒ permitted read bytes = 8400000000
	// 40 minR% ⇒ 60 maxW% ⇒ permitted write bytes = 7200000000
	bytesInViolation := uint64(12000000001)
	bytesNotInViolation := uint64(12000000000)
	bytesRInViolation := uint64(8400000001)
	bytesWInViolation := uint64(7200000001)
	// non-zero spd, no violations
	tl.Logger().Info("*** Workload Rate Throughput: No violations")
	tl.Flush()
	sd = &vsIOMSample{}
	cloneVsIOMSample(sdBase, sd) // restore
	sd.NumBytes = bytesNotInViolation
	p.checkForCompliance(sd, spd, vsObj)
	assert.EqualValues(0, sd.ViolationWorkloadRate)
	// non-zero spd, ViolationWorkloadRate
	tl.Logger().Info("*** Workload Rate Throughput: ViolationWorkloadRate")
	tl.Flush()
	sd.NumBytes = bytesInViolation
	p.checkForCompliance(sd, spd, vsObj)
	assert.EqualValues(1, sd.ViolationWorkloadRate)
	// non-zero spd, ViolationWorkloadRate erased by adjusted size
	tl.Logger().Info("*** Workload Rate Throughput: ViolationWorkloadRate erased by adjusted size")
	tl.Flush()
	sd = &vsIOMSample{}
	cloneVsIOMSample(sdBase, sd) // restore
	sd.NumBytes = bytesInViolation
	p.checkForCompliance(sd, spd, adjVs)
	assert.EqualValues(0, sd.ViolationWorkloadRate)
	// non-zero spd, ViolationWorkloadMixRead
	tl.Logger().Info("*** Workload Rate IOPS: ViolationWorkloadMixRead")
	tl.Flush()
	sd = &vsIOMSample{}
	cloneVsIOMSample(sdBase, sd) // restore
	sd.ReadNumBytes = bytesRInViolation
	p.checkForCompliance(sd, spd, vsObj)
	assert.EqualValues(0, sd.ViolationWorkloadRate)
	assert.EqualValues(1, sd.ViolationWorkloadMixRead)
	assert.EqualValues(0, sd.ViolationWorkloadMixWrite)
	// non-zero spd, ViolationWorkloadMixRead erased by adjusted size
	tl.Logger().Info("*** Workload Rate IOPS: ViolationWorkloadMixRead erased by adjusted size")
	tl.Flush()
	sd = &vsIOMSample{}
	cloneVsIOMSample(sdBase, sd) // restore
	sd.ReadNumBytes = bytesRInViolation
	p.checkForCompliance(sd, spd, adjVs)
	assert.EqualValues(0, sd.ViolationWorkloadRate)
	assert.EqualValues(0, sd.ViolationWorkloadMixRead)
	assert.EqualValues(0, sd.ViolationWorkloadMixWrite)
	// non-zero spd, ViolationWorkloadMixWrite
	tl.Logger().Info("*** Workload Rate IOPS: ViolationWorkloadMixWrite")
	tl.Flush()
	sd = &vsIOMSample{}
	cloneVsIOMSample(sdBase, sd) // restore
	sd.WriteNumBytes = bytesWInViolation
	p.checkForCompliance(sd, spd, vsObj)
	assert.EqualValues(0, sd.ViolationWorkloadRate)
	assert.EqualValues(0, sd.ViolationWorkloadMixRead)
	assert.EqualValues(1, sd.ViolationWorkloadMixWrite)
	// non-zero spd, ViolationWorkloadMixWrite erased by adjusted size
	tl.Logger().Info("*** Workload Rate IOPS: ViolationWorkloadMixWrite erased by adjusted size")
	tl.Flush()
	sd = &vsIOMSample{}
	cloneVsIOMSample(sdBase, sd) // restore
	sd.WriteNumBytes = bytesWInViolation
	p.checkForCompliance(sd, spd, adjVs)
	assert.EqualValues(0, sd.ViolationWorkloadRate)
	assert.EqualValues(0, sd.ViolationWorkloadMixRead)
	assert.EqualValues(0, sd.ViolationWorkloadMixWrite)

	// Average I/O size
	spd = &ServicePlanData{}
	spd.MinAvgSizeBytes = 100
	spd.MaxAvgSizeBytes = 10000
	tl.Logger().Info("*** Workload Average Size: No violation")
	tl.Flush()
	sd = &vsIOMSample{}
	cloneVsIOMSample(sdBase, sd) // restore
	sd.AvgIOSize = 500
	p.checkForCompliance(sd, spd, vsObj)
	assert.EqualValues(0, sd.ViolationWorkloadAvgSizeMin)
	assert.EqualValues(0, sd.ViolationWorkloadAvgSizeMax)
	tl.Logger().Info("*** Workload Average Size: ViolationWorkloadAvgSizeMin")
	tl.Flush()
	sd = &vsIOMSample{}
	cloneVsIOMSample(sdBase, sd) // restore
	sd.AvgIOSize = 30
	p.checkForCompliance(sd, spd, vsObj)
	assert.EqualValues(70, sd.ViolationWorkloadAvgSizeMin)
	assert.EqualValues(0, sd.ViolationWorkloadAvgSizeMax)
	tl.Logger().Info("*** Workload Average Size: ViolationWorkloadAvgSizeMax")
	tl.Flush()
	sd = &vsIOMSample{}
	cloneVsIOMSample(sdBase, sd) // restore
	sd.AvgIOSize = 20000
	p.checkForCompliance(sd, spd, vsObj)
	assert.EqualValues(0, sd.ViolationWorkloadAvgSizeMin)
	assert.EqualValues(10000, sd.ViolationWorkloadAvgSizeMax)
}

func cloneVsIOMSample(src, dst *vsIOMSample) {
	testutils.Clone(src, dst)
	cloneIoMetricStatisticsDatum(&src.IoMetricStatisticsDatum, &dst.IoMetricStatisticsDatum)
}

func vsClone(o *models.VolumeSeries) *models.VolumeSeries {
	n := new(models.VolumeSeries)
	testutils.Clone(o, n)
	return n
}
