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


package metricmover

import (
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	"github.com/stretchr/testify/assert"
)

func TestIOMetric0Size(t *testing.T) {
	assert := assert.New(t)

	iom := NewIOMetric0()
	assert.Equal(len(iom.LatencyNsHist), IOMetric0NumLatencyHistBuckets)
	assert.Equal(len(iom.LatencyNsHist), len(ioMetric0LatencyNsBucketMidpoints))
	assert.Equal(len(iom.LatencyNsHist), len(IOMetric0LatencyNsHistLowerBounds))
	assert.Equal(len(iom.LatencyNsMaxHist), IOMetric0NumMaxLatencyHistBuckets)
	assert.Equal(len(iom.LatencyNsMaxHist), len(IOMetric0MaxLatencyNsHistBuckets))
	assert.Equal(len(iom.SizeBytesHist), IOMetric0NumSizeHistBuckets)
}

func TestIOMetric0Encoding(t *testing.T) {
	assert := assert.New(t)

	rM := NewIOMetric0()
	rM.NumOperations = 3
	rM.NumBytes = 10
	rM.LatencyNsMean = 1.5
	for i := 0; i < len(rM.LatencyNsHist); i++ {
		rM.LatencyNsHist[i] = 2 * uint64(i)
	}
	for i := 0; i < len(rM.LatencyNsMaxHist); i++ {
		rM.LatencyNsMaxHist[i] = 2 * uint64(i)
	}
	for i := 0; i < len(rM.SizeBytesHist); i++ {
		rM.SizeBytesHist[i] = 2 * uint64(i)
	}
	wM := NewIOMetric0()
	wM.NumOperations = 2
	wM.NumBytes = 70
	wM.LatencyNsMean = 2.9
	for i := 0; i < len(wM.LatencyNsHist); i++ {
		wM.LatencyNsHist[i] = 3 * uint64(i)
	}
	for i := 0; i < len(wM.LatencyNsMaxHist); i++ {
		wM.LatencyNsMaxHist[i] = 3 * uint64(i)
	}
	for i := 0; i < len(wM.SizeBytesHist); i++ {
		wM.SizeBytesHist[i] = 3 * uint64(i)
	}
	cuM := NewIOCacheMetric0()
	cuM.NumReadHits = 8
	cuM.NumReadTotal = 10
	cmM := NewIOCacheMetric0()
	cmM.NumReadHits = 18
	cmM.NumReadTotal = 20
	ims1 := &IOMetricStats0{
		DurationNs:      5 * time.Minute,
		ReadMetric:      *rM,
		WriteMetric:     *wM,
		CacheUserMetric: *cuM,
		CacheMetaMetric: *cmM,
	}

	s := ims1.Marshal()
	expS := "{\"d\":300000000000,\"r\":{\"o\":3,\"b\":10,\"m\":1.5,\"l\":[0,2,4,6,8,10,12,14,16,18,20,22,24,26,28,30,32,34,36,38,40,42,44,46],\"x\":[0,2,4,6,8,10,12,14,16,18,20],\"s\":[0,2,4,6,8,10,12,14,16,18,20,22]},\"w\":{\"o\":2,\"b\":70,\"m\":2.9,\"l\":[0,3,6,9,12,15,18,21,24,27,30,33,36,39,42,45,48,51,54,57,60,63,66,69],\"x\":[0,3,6,9,12,15,18,21,24,27,30],\"s\":[0,3,6,9,12,15,18,21,24,27,30,33]},\"u\":{\"h\":8,\"t\":10},\"m\":{\"h\":18,\"t\":20}}"
	assert.Equal(expS, s)
	assert.Equal(s, ims1.String())

	ims2 := &IOMetricStats0{}
	err := ims2.Unmarshal(s)
	assert.NoError(err)
	assert.Equal(ims1, ims2)
}

func TestIOMetric0FromStatsIO(t *testing.T) {
	assert := assert.New(t)

	si := &nuvoapi.StatsIO{
		Count:       100,
		SizeTotal:   1000,
		LatencyMean: 130.30489,
	}
	iom := NewIOMetric0()
	err := iom.FromStatsIO(si)
	assert.Error(err)
	assert.Regexp("unsupported histogram", err)

	si.LatencyHist = make([]uint64, 128)
	for i := 0; i < len(si.LatencyHist); i++ {
		si.LatencyHist[i] = uint64(i)
	}
	si.SizeHist = make([]uint64, 32)
	for i := 0; i < len(si.SizeHist); i++ {
		si.SizeHist[i] = uint64(i)
	}

	err = iom.FromStatsIO(si)
	assert.NoError(err)

	expIOM := &IOMetric0{
		NumOperations: 100,
		NumBytes:      1000,
		LatencyNsMean: 130.30489,
	}
	expIOM.LatencyNsHist = make([]uint64, IOMetric0NumLatencyHistBuckets)
	expIOM.SizeBytesHist = make([]uint64, IOMetric0NumSizeHistBuckets)
	expIOM.LatencyNsMaxHist = make([]uint64, IOMetric0NumMaxLatencyHistBuckets)
	lh := expIOM.LatencyNsHist
	lh[0] = (59 * 60) / 2 // Σ59
	lh[1] = 60 + 61 + 62 + 63
	lh[2] = 64 + 65 + 66 + 67
	lh[3] = 68 + 69 + 70 + 71
	lh[4] = 72 + 73 + 74 + 75
	lh[5] = 76 + 77 + 78 + 79
	lh[6] = 80 + 81
	lh[7] = 82 + 83
	lh[8] = 84
	lh[9] = 85 + 86 + 87
	lh[10] = 88
	lh[11] = 89
	lh[12] = 90 + 91
	lh[13] = 92
	lh[14] = 93 + 94 + 95
	lh[15] = 96 + 97
	lh[16] = 98 + 99
	lh[17] = 100 + 101 + 102 + 103
	lh[18] = 104 + 105 + 106 + 107
	lh[19] = 108 + 109 + 110 + 111
	lh[20] = 112 + 113 + 114 + 115
	lh[21] = 116 + 117 + 118 + 119
	lh[22] = 120 + 121 + 122 + 123
	lh[23] = 124 + 125 + 126 + 127
	lhm := expIOM.LatencyNsMaxHist
	lhm[0] = (75 * 76) / 2 // Σ75
	lhm[1] = 76 + 77 + 78 + 79
	lhm[2] = 80 + 81
	lhm[3] = 82 + 83
	lhm[4] = 84
	lhm[5] = 85 + 86 + 87
	lhm[6] = 88
	lhm[7] = 89 + 90 + 91 + 92 + 93 + 94 + 95
	lhm[8] = 96 + 97 + 98 + 99 + 100 + 101 + 102 + 103
	lhm[9] = 104 + 105 + 106 + 107 + 108 + 109 + 110 + 111 + 112 + 113 + 114 + 115
	lhm[10] = 116 + 117 + 118 + 119 + 120 + 121 + 122 + 123 + 124 + 125 + 126 + 127
	sh := expIOM.SizeBytesHist
	sh[0] = (8 * 9) / 2 // Σ8
	sh[1] = 9
	sh[2] = 10 + 11
	sh[3] = 12
	sh[4] = 13
	sh[5] = 14
	sh[6] = 15
	sh[7] = 16
	sh[8] = 17
	sh[9] = 18
	sh[10] = 19
	sh[11] = ((31 * 32) - (19 * 20)) / 2 // Σ(20...31) = Σ31 - Σ19
	assert.Equal(expIOM, iom)
}

func TestIOMetric0Subtract(t *testing.T) {
	assert := assert.New(t)

	zeroIOM := NewIOMetric0()

	iom0 := NewIOMetric0()
	iom0.NumOperations = 1000
	iom0.NumBytes = 100
	iom0.LatencyNsMean = 50000
	for i := 0; i < len(iom0.LatencyNsHist); i++ {
		iom0.LatencyNsHist[i] = 2 * uint64(i)
	}
	for i := 0; i < len(iom0.SizeBytesHist); i++ {
		iom0.SizeBytesHist[i] = 2 * uint64(i)
	}

	iom1 := NewIOMetric0()
	iom1.NumOperations = iom0.NumOperations
	iom := iom1.Subtract(iom0)
	assert.Equal(zeroIOM, iom)

	iom1.NumOperations = iom0.NumOperations + 500
	iom1.NumBytes = iom0.NumBytes + 50
	iom1.LatencyNsMean = 60000
	for i := 0; i < len(iom1.LatencyNsHist); i++ {
		iom1.LatencyNsHist[i] = 3 * uint64(i)
	}
	for i := 0; i < len(iom1.SizeBytesHist); i++ {
		iom1.SizeBytesHist[i] = 3 * uint64(i)
	}
	iom = iom1.Subtract(iom0)
	assert.NotEqual(zeroIOM, iom)

	expIOM := NewIOMetric0()
	expIOM.NumOperations = 500
	expIOM.NumBytes = 50
	expIOM.LatencyNsMean = 80000
	for i := 0; i < len(expIOM.LatencyNsHist); i++ {
		expIOM.LatencyNsHist[i] = uint64(i)
	}
	for i := 0; i < len(expIOM.SizeBytesHist); i++ {
		expIOM.SizeBytesHist[i] = uint64(i)
	}
	assert.Equal(expIOM, iom)
}

func TestIOMetric0Latency(t *testing.T) {
	assert := assert.New(t)

	for i := 0; i < len(IOMetric0LatencyNsHistLowerBounds); i++ {
		t.Logf("%d, // [%d]", IOMetric0LatencyNsBucketMidpoint(i), i)
	}

	assert.Equal(uint64(0), IOMetric0LatencyNsBucketMidpoint(-1))
	assert.Equal(uint64(0+(65536-0)/2), IOMetric0LatencyNsBucketMidpoint(0))
	assert.Equal(uint64(65536+(131072-65536)/2), IOMetric0LatencyNsBucketMidpoint(1))
	assert.Equal(uint64(131072+(262144-131072)/2), IOMetric0LatencyNsBucketMidpoint(2))
	assert.Equal(uint64(262144+(524288-262144)/2), IOMetric0LatencyNsBucketMidpoint(3))
	assert.Equal(uint64(524288+(1048576-524288)/2), IOMetric0LatencyNsBucketMidpoint(4))
	assert.Equal(uint64(1048576+(2097152-1048576)/2), IOMetric0LatencyNsBucketMidpoint(5))
	assert.Equal(uint64(2097152+(3145728-2097152)/2), IOMetric0LatencyNsBucketMidpoint(6))
	assert.Equal(uint64(3145728+(4194304-3145728)/2), IOMetric0LatencyNsBucketMidpoint(7))
	assert.Equal(uint64(4194304+(5242880-4194304)/2), IOMetric0LatencyNsBucketMidpoint(8))
	assert.Equal(uint64(5242880+(8388608-5242880)/2), IOMetric0LatencyNsBucketMidpoint(9))
	assert.Equal(uint64(8388608+(10485760-8388608)/2), IOMetric0LatencyNsBucketMidpoint(10))
	assert.Equal(uint64(10485760+(12582912-10485760)/2), IOMetric0LatencyNsBucketMidpoint(11))
	assert.Equal(uint64(12582912+(16777216-12582912)/2), IOMetric0LatencyNsBucketMidpoint(12))
	assert.Equal(uint64(16777216+(20971520-16777216)/2), IOMetric0LatencyNsBucketMidpoint(13))
	assert.Equal(uint64(20971520+(33554432-20971520)/2), IOMetric0LatencyNsBucketMidpoint(14))
	assert.Equal(uint64(33554432+(50331648-33554432)/2), IOMetric0LatencyNsBucketMidpoint(15))
	assert.Equal(uint64(50331648+(67108864-50331648)/2), IOMetric0LatencyNsBucketMidpoint(16))
	assert.Equal(uint64(67108864+(134217728-67108864)/2), IOMetric0LatencyNsBucketMidpoint(17))
	assert.Equal(uint64(134217728+(268435456-134217728)/2), IOMetric0LatencyNsBucketMidpoint(18))
	assert.Equal(uint64(268435456+(536870912-268435456)/2), IOMetric0LatencyNsBucketMidpoint(19))
	assert.Equal(uint64(536870912+(1073741824-536870912)/2), IOMetric0LatencyNsBucketMidpoint(20))
	assert.Equal(uint64(1073741824+(2147483648-1073741824)/2), IOMetric0LatencyNsBucketMidpoint(21))
	assert.Equal(uint64(2147483648+(4294967296-2147483648)/2), IOMetric0LatencyNsBucketMidpoint(22))
	assert.Equal(uint64(4294967296), IOMetric0LatencyNsBucketMidpoint(23))
	assert.Equal(24, len(IOMetric0LatencyNsHistLowerBounds))
	assert.Equal(uint64(0), IOMetric0LatencyNsBucketMidpoint(24))

	iom := NewIOMetric0()
	assert.Equal(float64(0), iom.LatencyMaxNs())
	for i := 0; i < len(iom.LatencyNsHist); i++ {
		iom.LatencyNsHist[i] = 1
		assert.Equal(float64(IOMetric0LatencyNsBucketMidpoint(i)), iom.LatencyMaxNs())
	}

	iom = NewIOMetric0()
	iom.LatencyNsMean = 100.0
	assert.Equal(float64(100.0), iom.LatencyMaxNs())
	for i := 0; i < len(iom.LatencyNsHist); i++ {
		iom.LatencyNsHist[i] = 1
		mp := float64(IOMetric0LatencyNsBucketMidpoint(i))
		iom.LatencyNsMean = mp + 1.0
		assert.Equal(mp+1.0, iom.LatencyMaxNs())
	}
}

func TestIOMetric0LatencyCountOverThreshold(t *testing.T) {
	assert := assert.New(t)

	iom := NewIOMetric0()
	for i := 0; i < len(iom.LatencyNsHist); i++ {
		iom.LatencyNsHist[i] = 1
	}
	t.Log(iom.LatencyNsHist)

	// threshold at lower bound
	for i := 0; i < len(iom.LatencyNsHist); i++ {
		threshold := IOMetric0LatencyNsHistLowerBounds[i]
		sum := iom.LatencyCountOverThresholdNs(threshold)
		j := len(iom.LatencyNsHist) - i
		expSum := j
		assert.EqualValues(expSum, sum, "Threshold: i=%d j=%d t=%d sum=%d", i, j, threshold, sum)
	}

	// threshold at midpoint
	for i := 0; i < len(iom.LatencyNsHist); i++ {
		threshold := ioMetric0LatencyNsBucketMidpoints[i]
		sum := iom.LatencyCountOverThresholdNs(threshold)
		j := len(iom.LatencyNsHist) - i
		expSum := j - 1
		if j == 1 {
			expSum = 1 // special case of last bucket
		}
		assert.EqualValues(expSum, sum, "Threshold: i=%d j=%d t=%d sum=%d", i, j, threshold, sum)
	}
}
