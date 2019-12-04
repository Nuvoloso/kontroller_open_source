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

func TestSIOInitAndUpload(t *testing.T) {
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

	assert.NotNil(c.sIOP.worker)
	assert.NotNil(c.sIOP.Queue)
	assert.NotNil(c.sIOW.worker)
	assert.NotNil(c.sIOW.Queue)

	c.ConsumeStorageIoMetricData([]*models.IoMetricDatum{&models.IoMetricDatum{}, &models.IoMetricDatum{}})
	assert.Equal(2, c.sIOP.Queue.Length())
}

func TestSIOPBuzz(t *testing.T) {
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
	c.StorageIOMaxBuffered = StorageIOMaxBufferedDefault

	p := &c.sIOP
	w := &c.sIOW

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

	// Buzz: storage fetch error (retryable, queue not flushed, error returned)
	tl.Flush()
	d.Version = 0
	p.EnqueueIOM([]*models.IoMetricDatum{d})
	assert.Equal(1, p.Queue.Length())
	fc.StorageFetchRetErr = fmt.Errorf("storage-fetch-error")
	err = p.Buzz(nil)
	assert.Error(err)
	assert.Regexp("storage-fetch-error", err)
	assert.Equal(1, p.Queue.Length())
	assert.True(p.InError)

	// Buzz: storage fetch NotFound error (metric dropped)
	err = crud.NewError(&models.Error{Code: 404, Message: swag.String(com.ErrorNotFound)})
	ce, ok := err.(*crud.Error)
	assert.True(ok)
	assert.True(ce.NotFound())
	fc.StorageFetchRetErr = ce
	assert.Equal(1, p.Queue.Length())
	err = p.Buzz(nil)
	assert.NoError(err)
	assert.Equal(0, p.Queue.Length())
	assert.False(p.InError)

	// Buzz: storage type not cached (metric dropped)
	tl.Flush()
	p.EnqueueIOM([]*models.IoMetricDatum{d})
	assert.Empty(c.storageTypeCache)
	sObj := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			CspStorageType: "Amazon gp2",
		},
	}
	fc.StorageFetchRetErr = nil
	fc.StorageFetchRetObj = sObj
	assert.Equal(1, p.Queue.Length())
	err = p.Buzz(nil)
	assert.NoError(err)
	assert.Equal(0, p.Queue.Length())
	assert.Equal(1, tl.CountPattern("StorageType.* not cached"))
	assert.False(p.InError)

	// Buzz: no error, record enqueued for writing
	p.EnqueueIOM([]*models.IoMetricDatum{d})
	assert.Equal(0, w.Queue.Length())
	std := &StorageTypeData{}
	c.cacheStorageType(string(sObj.CspStorageType), std)
	assert.Equal(1, p.Queue.Length())
	p.InError = true
	err = p.Buzz(nil)
	assert.NoError(err)
	assert.Equal(0, p.Queue.Length())
	assert.Equal(1, w.Queue.Length())
	sd = w.Queue.PeekHead().(*sIOMSample)
	assert.Equal("OBJ-1", sd.ObjectID)
	assert.Equal(now, sd.Timestamp)
	assert.False(p.InError)
}

func TestSIOPCompliance(t *testing.T) {
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

	p := &c.sIOP

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

	sdBase := &sIOMSample{}
	cloneSIOMSample(sd, sdBase) // to restore from later

	sObj := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			SizeBytes: swag.Int64(500 * units.GiB),
		},
	}

	// zero std
	std := &StorageTypeData{}
	p.checkForCompliance(sd, std, sObj)
	assert.EqualValues(0, sd.ViolationLatencyMean)
	assert.EqualValues(0, sd.ViolationLatencyMax)
	assert.EqualValues(0, sd.ViolationWorkloadRate)

	// The code below is organized as needed as we want to change the easiest operand to cause a violation
	// and this will interfere across the calculations for each type of violation.
	// Violations are all non-zero - check for non-violations first.

	// Latency
	// Bucket 18 is selected as the bucket that will violate the response time maximum
	tl.Logger().Info("*** Latency checks")
	tl.Flush()
	sd = &sIOMSample{}
	cloneSIOMSample(sdBase, sd) // restore
	stdRTMaxUsecInViolation := int32(metricmover.IOMetric0LatencyNsBucketMidpoint(18) / uint64(time.Microsecond))
	stdRTMaxUsecNotInViolation := int32(metricmover.IOMetric0LatencyNsBucketMidpoint(19) / uint64(time.Microsecond))
	// non-zero std, no violations
	std = &StorageTypeData{}
	std.ResponseTimeMeanUsec = sd.LatencyMeanUsec + 1
	assert.True(std.ResponseTimeMeanUsec > 0)
	std.ResponseTimeMeanUsec = stdRTMaxUsecNotInViolation
	p.checkForCompliance(sd, std, sObj)
	assert.EqualValues(0, sd.ViolationLatencyMean)
	assert.EqualValues(0, sd.ViolationLatencyMax)
	// non-zero std, violations
	std.ResponseTimeMeanUsec = sd.LatencyMeanUsec - 1
	assert.True(std.ResponseTimeMeanUsec > 0)
	std.ResponseTimeMaxUsec = stdRTMaxUsecInViolation
	p.checkForCompliance(sd, std, sObj)
	assert.EqualValues(1, sd.ViolationLatencyMean)
	assert.EqualValues(2, sd.ViolationLatencyMax)

	// Workload Rate (IOPS)
	std = &StorageTypeData{}
	std.IOPsPerGiB = 3
	std.BytesPerSecPerGiB = 0
	// Given 3 IOPS/GiB (e.g. GP2), a sample duration of 300s and storage size of 500GiB,
	// the permitted number of I/Os for the sample is: 3 * 500 * 300 = 450000
	numOpsInViolation := uint64(450001)
	numOpsNotInViolation := uint64(450000)
	// non-zero std, no violations
	tl.Logger().Info("*** Workload Rate IOPS: No violations")
	tl.Flush()
	sd = &sIOMSample{}
	cloneSIOMSample(sdBase, sd) // restore
	sd.NumOps = numOpsNotInViolation
	p.checkForCompliance(sd, std, sObj)
	assert.EqualValues(0, sd.ViolationWorkloadRate)
	// non-zero std, ViolationWorkloadRate
	tl.Logger().Info("*** Workload Rate IOPS: ViolationWorkloadRate")
	tl.Flush()
	sd = &sIOMSample{}
	cloneSIOMSample(sdBase, sd) // restore
	sd.NumOps = numOpsInViolation
	p.checkForCompliance(sd, std, sObj)
	assert.EqualValues(1, sd.ViolationWorkloadRate)

	// Workload Rate (Throughput)
	std = &StorageTypeData{}
	std.IOPsPerGiB = 0
	std.BytesPerSecPerGiB = 4000000
	sObj.SizeBytes = swag.Int64(20 * units.GiB)
	// Given 4000000 B/s/GiB (e.g. HDD), a sample duration of 300s and storage size of 20GiB,
	// the permitted number of bytes for the sample is: 4000000 * 20 * 300 = 24000000000
	bytesInViolation := uint64(24000000001)
	bytesNotInViolation := uint64(24000000000)
	// non-zero std, no violations
	tl.Logger().Info("*** Workload Rate Throughput: No violations")
	tl.Flush()
	sd = &sIOMSample{}
	cloneSIOMSample(sdBase, sd) // restore
	sd.NumBytes = bytesNotInViolation
	p.checkForCompliance(sd, std, sObj)
	assert.EqualValues(0, sd.ViolationWorkloadRate)
	// non-zero std, ViolationWorkloadRate
	tl.Logger().Info("*** Workload Rate Throughput: ViolationWorkloadRate")
	tl.Flush()
	sd.NumBytes = bytesInViolation
	p.checkForCompliance(sd, std, sObj)
	assert.EqualValues(1, sd.ViolationWorkloadRate)
}

func cloneSIOMSample(src, dst *sIOMSample) {
	testutils.Clone(src, dst)
	cloneIoMetricStatisticsDatum(&src.IoMetricStatisticsDatum, &dst.IoMetricStatisticsDatum)
}
