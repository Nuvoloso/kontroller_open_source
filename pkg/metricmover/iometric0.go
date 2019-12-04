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
	"encoding/json"
	"fmt"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
)

// IOMetricStats0 is version 0 of an I/O metric statistic data point,
// containing a read and a write sample.
type IOMetricStats0 struct {
	DurationNs      time.Duration  `json:"d"`
	ReadMetric      IOMetric0      `json:"r"`
	WriteMetric     IOMetric0      `json:"w"`
	CacheUserMetric IOCacheMetric0 `json:"u"`
	CacheMetaMetric IOCacheMetric0 `json:"m"`
}

// Marshal returns a transmittable form
func (ioms *IOMetricStats0) Marshal() string {
	b, _ := json.Marshal(ioms)
	return string(b)
}

// String is the printable form
func (ioms *IOMetricStats0) String() string {
	return ioms.Marshal()
}

// Unmarshal decodes the transmittable form
func (ioms *IOMetricStats0) Unmarshal(s string) error {
	return json.Unmarshal([]byte(s), ioms)
}

// IOMetric0 related constants
const (
	IOMetric0NumLatencyHistBuckets    = 24
	IOMetric0NumMaxLatencyHistBuckets = 11
	IOMetric0NumSizeHistBuckets       = 12
	StatIONumLatencyHistBuckets       = 128
	StatIONumSizeHistBuckets          = 32
)

// IOMetric0 is version 0 of an I/O metric read or write data point
type IOMetric0 struct {
	NumOperations    uint64   `json:"o"`
	NumBytes         uint64   `json:"b"`
	LatencyNsMean    float64  `json:"m"`
	LatencyNsHist    []uint64 `json:"l"`
	LatencyNsMaxHist []uint64 `json:"x"`
	SizeBytesHist    []uint64 `json:"s"`
}

// IOCacheMetric0 is version 0 of an cache I/O metric read data point
type IOCacheMetric0 struct {
	NumReadHits  uint64 `json:"h"`
	NumReadTotal uint64 `json:"t"`
}

// NewIOMetric0 returns an initialized IOMetric0
func NewIOMetric0() *IOMetric0 {
	iom := &IOMetric0{}
	iom.Init()
	return iom
}

// NewIOCacheMetric0 returns an initialized IOMetric0
func NewIOCacheMetric0() *IOCacheMetric0 {
	iocm := &IOCacheMetric0{}
	return iocm
}

// IOMetric0LatencyNsHistLowerBounds contains the lower bound of each
// latency histogram bucket in IOMetric0. Values are in nanoseconds.
// See https://docs.google.com/document/d/10JpH9orBXBCyy7EEcRcSgeeFDztiLczrwxOQdzeK4Ac/edit?ts=5ade6453#
var IOMetric0LatencyNsHistLowerBounds = []uint64{
	// ns       // Idx Bins
	0,          // [0] 0-59
	65536,      // [1] 60-63
	131072,     // [2] 64-67
	262144,     // [3] 68-71
	524288,     // [4] 72-75
	1048576,    // [5] 76-79
	2097152,    // [6] 80-81
	3145728,    // [7] 82-83
	4194304,    // [8] 84
	5242880,    // [9] 85-87
	8388608,    // [10] 88
	10485760,   // [11] 89
	12582912,   // [12] 90-91
	16777216,   // [13] 92
	20971520,   // [14] 93-95
	33554432,   // [15] 96-97
	50331648,   // [16] 98-99
	67108864,   // [17] 100-103
	134217728,  // [18] 104-107
	268435456,  // [19] 108-111
	536870912,  // [20] 112-115
	1073741824, // [21] 116-119
	2147483648, // [22] 120-123
	4294967296, // [23] 124-127
}

// IOMetric0MaxLatencyNsHistBuckets contains the lower bound of each
// max latency histogram bucket in IOMetric0. Values are in nanoseconds.
// See https://docs.google.com/document/d/1stAbCUQXm_wJoxUKRwmEKL2gx4tcMtL3dO4BL453J6k/edit
var IOMetric0MaxLatencyNsHistBuckets = []uint64{
	// ns       // Idx Bins
	0,          // [0] 0-75
	1048576,    // [1] 76-79
	2097152,    // [2] 80-81
	3145728,    // [3] 82-83
	4194304,    // [4] 84
	5242880,    // [5] 85-87
	8388608,    // [6] 88
	10485760,   // [7] 89-95
	33554432,   // [8] 96-103
	134217728,  // [9] 104-115
	1073741824, // [10] 116-127
}

// ioMetric0LatencyNsBucketMidpoints contains precomputed midpoints
var ioMetric0LatencyNsBucketMidpoints = []uint64{
	32768,      // [0]
	98304,      // [1]
	196608,     // [2]
	393216,     // [3]
	786432,     // [4]
	1572864,    // [5]
	2621440,    // [6]
	3670016,    // [7]
	4718592,    // [8]
	6815744,    // [9]
	9437184,    // [10]
	11534336,   // [11]
	14680064,   // [12]
	18874368,   // [13]
	27262976,   // [14]
	41943040,   // [15]
	58720256,   // [16]
	100663296,  // [17]
	201326592,  // [18]
	402653184,  // [19]
	805306368,  // [20]
	1610612736, // [21]
	3221225472, // [22]
	4294967296, // [23] Note: This is the lower bound.
}

// IOMetric0LatencyNsBucketMidpoint returns the midpoint of the bucket's upper
// and lower bounds except for the last bucket where the lower bound returned.
func IOMetric0LatencyNsBucketMidpoint(i int) uint64 {
	if i < 0 || i >= len(IOMetric0LatencyNsHistLowerBounds) {
		return 0
	}
	// The midpoint table above was generated with the following code:
	// lb := IOMetric0LatencyNsHistLowerBounds[i]
	// if i == len(IOMetric0LatencyNsHistLowerBounds)-1 {
	// 	return lb
	// }
	// ub := IOMetric0LatencyNsHistLowerBounds[i+1]
	// return lb + ((ub - lb) / 2)
	return ioMetric0LatencyNsBucketMidpoints[i]
}

// IOMetric0SizeBytesHistLowerBounds contains the lower bound of each
// size histogram bucket in IOMetrics0. Values are in bytes.
// See https://docs.google.com/document/d/10JpH9orBXBCyy7EEcRcSgeeFDztiLczrwxOQdzeK4Ac/edit?ts=5ade6453#
var IOMetric0SizeBytesHistLowerBounds = []uint64{
	// bytes // Idx Bins
	0,       // [0] 0-8
	512,     // [1] 9
	1024,    // [2] 10-11
	4096,    // [3] 12
	8192,    // [4] 13
	16384,   // [5] 14
	32768,   // [6] 15
	65536,   // [7] 16
	131072,  // [8] 17
	262144,  // [9] 18
	524288,  // [10] 19
	1048576, // [11] 20-31
}

// Init allocates the arrays referenced in the structure if they do not exist.
func (iom *IOMetric0) Init() {
	if iom.LatencyNsHist == nil {
		iom.LatencyNsHist = make([]uint64, IOMetric0NumLatencyHistBuckets)
	}
	if iom.LatencyNsMaxHist == nil {
		iom.LatencyNsMaxHist = make([]uint64, IOMetric0NumMaxLatencyHistBuckets)
	}
	if iom.SizeBytesHist == nil {
		iom.SizeBytesHist = make([]uint64, IOMetric0NumSizeHistBuckets)
	}
}

// FromStatsIO imports data from a nuvoapi.StatsIO structure.
// Note that the values are cumulative.
func (iom *IOMetric0) FromStatsIO(si *nuvoapi.StatsIO) error {
	iom.Init()
	iom.NumOperations = si.Count
	iom.NumBytes = si.SizeTotal
	iom.LatencyNsMean = si.LatencyMean
	if si.LatencyHist == nil || si.SizeHist == nil ||
		len(si.LatencyHist) != StatIONumLatencyHistBuckets || len(si.SizeHist) != StatIONumSizeHistBuckets {
		return fmt.Errorf("unsupported histograms in StatIO")
	}
	// latency histogram
	iom.LatencyNsHist[0] = aSum(si.LatencyHist, 0, 59)
	iom.LatencyNsHist[1] = aSum(si.LatencyHist, 60, 63)
	iom.LatencyNsHist[2] = aSum(si.LatencyHist, 64, 67)
	iom.LatencyNsHist[3] = aSum(si.LatencyHist, 68, 71)
	iom.LatencyNsHist[4] = aSum(si.LatencyHist, 72, 75)
	iom.LatencyNsHist[5] = aSum(si.LatencyHist, 76, 79)
	iom.LatencyNsHist[6] = aSum(si.LatencyHist, 80, 81)
	iom.LatencyNsHist[7] = aSum(si.LatencyHist, 82, 83)
	iom.LatencyNsHist[8] = si.LatencyHist[84]
	iom.LatencyNsHist[9] = aSum(si.LatencyHist, 85, 87)
	iom.LatencyNsHist[10] = si.LatencyHist[88]
	iom.LatencyNsHist[11] = si.LatencyHist[89]
	iom.LatencyNsHist[12] = aSum(si.LatencyHist, 90, 91)
	iom.LatencyNsHist[13] = si.LatencyHist[92]
	iom.LatencyNsHist[14] = aSum(si.LatencyHist, 93, 95)
	iom.LatencyNsHist[15] = aSum(si.LatencyHist, 96, 97)
	iom.LatencyNsHist[16] = aSum(si.LatencyHist, 98, 99)
	iom.LatencyNsHist[17] = aSum(si.LatencyHist, 100, 103)
	iom.LatencyNsHist[18] = aSum(si.LatencyHist, 104, 107)
	iom.LatencyNsHist[19] = aSum(si.LatencyHist, 108, 111)
	iom.LatencyNsHist[20] = aSum(si.LatencyHist, 112, 115)
	iom.LatencyNsHist[21] = aSum(si.LatencyHist, 116, 119)
	iom.LatencyNsHist[22] = aSum(si.LatencyHist, 120, 123)
	iom.LatencyNsHist[23] = aSum(si.LatencyHist, 124, 127)
	// size histogram
	iom.SizeBytesHist[0] = aSum(si.SizeHist, 0, 8)
	iom.SizeBytesHist[1] = si.SizeHist[9]
	iom.SizeBytesHist[2] = aSum(si.SizeHist, 10, 11)
	for i, j := 3, 12; i <= 10; i, j = i+1, j+1 {
		iom.SizeBytesHist[i] = si.SizeHist[j]
	}
	iom.SizeBytesHist[11] = aSum(si.SizeHist, 20, 31)
	// max latency histogram
	iom.LatencyNsMaxHist[0] = aSum(si.LatencyHist, 0, 75)
	iom.LatencyNsMaxHist[1] = aSum(si.LatencyHist, 76, 79)
	iom.LatencyNsMaxHist[2] = aSum(si.LatencyHist, 80, 81)
	iom.LatencyNsMaxHist[3] = aSum(si.LatencyHist, 82, 83)
	iom.LatencyNsMaxHist[4] = si.LatencyHist[84]
	iom.LatencyNsMaxHist[5] = aSum(si.LatencyHist, 85, 87)
	iom.LatencyNsMaxHist[6] = si.LatencyHist[88]
	iom.LatencyNsMaxHist[7] = aSum(si.LatencyHist, 89, 95)
	iom.LatencyNsMaxHist[8] = aSum(si.LatencyHist, 96, 103)
	iom.LatencyNsMaxHist[9] = aSum(si.LatencyHist, 104, 115)
	iom.LatencyNsMaxHist[10] = aSum(si.LatencyHist, 116, 127)
	
	return nil
}

// Subtract performs a field-by-field subtraction, recomputes the sample mean and returns the result.
// This is to be used to convert a cumulative data point (loaded from nuvoapi.StatsIO)
// into a delta sample, and hence will blindly return a zero sample if the operand
// value of NumOperations is >= the value of NumOperations.
func (iom *IOMetric0) Subtract(o *IOMetric0) *IOMetric0 {
	n := NewIOMetric0()
	num := iom.NumOperations - o.NumOperations
	if num <= 0 {
		return n
	}
	n.NumOperations = num
	n.NumBytes = iom.NumBytes - o.NumBytes
	n.LatencyNsMean = ((iom.LatencyNsMean * float64(iom.NumOperations)) - (o.LatencyNsMean * float64(o.NumOperations))) / float64(num)
	for i := 0; i < len(n.LatencyNsHist); i++ {
		n.LatencyNsHist[i] = iom.LatencyNsHist[i] - o.LatencyNsHist[i]
	}
	for i := 0; i < len(n.SizeBytesHist); i++ {
		n.SizeBytesHist[i] = iom.SizeBytesHist[i] - o.SizeBytesHist[i]
	}
	return n
}

// LatencyMaxBucket returns the highest bucket index with a latency value set or -1
func (iom *IOMetric0) LatencyMaxBucket() int {
	for i := len(iom.LatencyNsHist) - 1; i >= 0; i-- {
		if iom.LatencyNsHist[i] != 0 {
			return i
		}
	}
	return -1
}

// LatencyMaxNs computes the maximum latency from the histogram.
func (iom *IOMetric0) LatencyMaxNs() float64 {
	lm := float64(IOMetric0LatencyNsBucketMidpoint(iom.LatencyMaxBucket()))
	if lm < iom.LatencyNsMean { // sanity check since we're estimating
		lm = iom.LatencyNsMean
	}
	return lm
}

// LatencyCountOverThresholdNs returns a count of the number of operations (in the latency
// histogram) where the latency value exceeds a threshold.
// The starting histogram bucket selected is one where [lb <= threshold < midpoint]
func (iom *IOMetric0) LatencyCountOverThresholdNs(thresholdNs uint64) uint64 {
	sum := uint64(0)
	// sum downward, but special case the highest bucket
	n := len(iom.LatencyNsHist) - 1
	for i := n; i >= 0; i-- {
		if thresholdNs >= IOMetric0LatencyNsBucketMidpoint(i) {
			if i == n {
				sum = iom.LatencyNsHist[n]
			}
			break
		}
		sum += iom.LatencyNsHist[i]
	}
	return sum
}

func aSum(a []uint64, lowerBoundInclusive, upperBoundInclusive int) uint64 {
	var total uint64
	for i := lowerBoundInclusive; i <= upperBoundInclusive; i++ {
		total += a[i]
	}
	return total
}

// For reference:
// Storelandia v0 latency buckets (in nanoseconds)
// 0: 0-0
// 1: 1-1
// 2: 2-2
// 3: 3-3
// 4: 4-4
// 5: 5-5
// 6: 6-6
// 7: 7-7
// 8: 8-9
// 9: 10-11
// 10: 12-13
// 11: 14-15
// 12: 16-19
// 13: 20-23
// 14: 24-27
// 15: 28-31
// 16: 32-39
// 17: 40-47
// 18: 48-55
// 19: 56-63
// 20: 64-79
// 21: 80-95
// 22: 96-111
// 23: 112-127
// 24: 128-159
// 25: 160-191
// 26: 192-223
// 27: 224-255
// 28: 256-319
// 29: 320-383
// 30: 384-447
// 31: 448-511
// 32: 512-639
// 33: 640-767
// 34: 768-895
// 35: 896-1023
// 36: 1024-1279
// 37: 1280-1535
// 38: 1536-1791
// 39: 1792-2047
// 40: 2048-2559
// 41: 2560-3071
// 42: 3072-3583
// 43: 3584-4095
// 44: 4096-5119
// 45: 5120-6143
// 46: 6144-7167
// 47: 7168-8191
// 48: 8192-10239
// 49: 10240-12287
// 50: 12288-14335
// 51: 14336-16383
// 52: 16384-20479
// 53: 20480-24575
// 54: 24576-28671
// 55: 28672-32767
// 56: 32768-40959
// 57: 40960-49151
// 58: 49152-57343
// 59: 57344-65535
// 60: 65536-81919
// 61: 81920-98303
// 62: 98304-114687
// 63: 114688-131071
// 64: 131072-163839
// 65: 163840-196607
// 66: 196608-229375
// 67: 229376-262143
// 68: 262144-327679
// 69: 327680-393215
// 70: 393216-458751
// 71: 458752-524287
// 72: 524288-655359
// 73: 655360-786431
// 74: 786432-917503
// 75: 917504-1048575
// 76: 1048576-1310719
// 77: 1310720-1572863
// 78: 1572864-1835007
// 79: 1835008-2097151
// 80: 2097152-2621439
// 81: 2621440-3145727
// 82: 3145728-3670015
// 83: 3670016-4194303
// 84: 4194304-5242879
// 85: 5242880-6291455
// 86: 6291456-7340031
// 87: 7340032-8388607
// 88: 8388608-10485759
// 89: 10485760-12582911
// 90: 12582912-14680063
// 91: 14680064-16777215
// 92: 16777216-20971519
// 93: 20971520-25165823
// 94: 25165824-29360127
// 95: 29360128-33554431
// 96: 33554432-41943039
// 97: 41943040-50331647
// 98: 50331648-58720255
// 99: 58720256-67108863
// 100: 67108864-83886079
// 101: 83886080-100663295
// 102: 100663296-117440511
// 103: 117440512-134217727
// 104: 134217728-167772159
// 105: 167772160-201326591
// 106: 201326592-234881023
// 107: 234881024-268435455
// 108: 268435456-335544319
// 109: 335544320-402653183
// 110: 402653184-469762047
// 111: 469762048-536870911
// 112: 536870912-671088639
// 113: 671088640-805306367
// 114: 805306368-939524095
// 115: 939524096-1073741823
// 116: 1073741824-1342177279
// 117: 1342177280-1610612735
// 118: 1610612736-1879048191
// 119: 1879048192-2147483647
// 120: 2147483648-2684354559
// 121: 2684354560-3221225471
// 122: 3221225472-3758096383
// 123: 3758096384-4294967295
// 124: 4294967296-5368709119
// 125: 5368709120-6442450943
// 126: 6442450944-7516192767
// 127: 7516192768-MAX
//
// Storelandia v0 size buckets (in bytes)
// 0: 0-1
// 1: 2-3
// 2: 4-7
// 3: 8-15
// 4: 16-31
// 5: 32-63
// 6: 64-127
// 7: 128-255
// 8: 256-511
// 9: 512-1023
// 10: 1024-2047
// 11: 2048-4095
// 12: 4096-8191
// 13: 8192-16383
// 14: 16384-32767
// 15: 32768-65535
// 16: 65536-131071
// 17: 131072-262143
// 18: 262144-524287
// 19: 524288-1048575
// 20: 1048576-2097151
// 21: 2097152-4194303
// 22: 4194304-8388607
// 23: 8388608-16777215
// 24: 16777216-33554431
// 25: 33554432-67108863
// 26: 67108864-134217727
// 27: 134217728-268435455
// 28: 268435456-536870911
// 29: 536870912-1073741823
// 30: 1073741824-2147483647
// 31: 2147483648-MAX
