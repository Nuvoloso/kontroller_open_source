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


package util

import (
	"fmt"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/alecthomas/units"
	"github.com/stretchr/testify/assert"
)

func TestSizeToString(t *testing.T) {
	assert := assert.New(t)

	tcsB2B := []struct {
		i   units.Base2Bytes
		str string
	}{
		{20 * units.TiB, "20TiB"},
		{1 * units.TiB, "1TiB"},
		{20 * units.GiB, "20GiB"},
		{1 * units.GiB, "1GiB"},
		{20 * units.MiB, "20MiB"},
		{1 * units.MiB, "1MiB"},
		{20 * units.KiB, "20KiB"},
		{1 * units.KiB, "1KiB"},
		{999, "999B"},
		{1, "1B"},
		{0, "0B"},
		{-20 * units.TiB, "-20TiB"},
		{-1 * units.TiB, "-1TiB"},
		{-20 * units.GiB, "-20GiB"},
		{-1 * units.GiB, "-1GiB"},
		{-20 * units.MiB, "-20MiB"},
		{-1 * units.MiB, "-1MiB"},
		{-20 * units.KiB, "-20KiB"},
		{-1 * units.KiB, "-1KiB"},
		{-999, "-999B"},
		{-1, "-1B"},
	}
	for _, tc := range tcsB2B {
		sz := SizeBytes(tc.i)
		s := fmt.Sprintf("%s", sz)
		assert.Equal(tc.str, s)
		assert.Equal(tc.str, SizeBytesToString(int64(tc.i)))
	}

	tcsMB := []struct {
		i   units.MetricBytes
		str string
	}{
		{20 * units.TB, "20TB"},
		{1 * units.TB, "1TB"},
		{20 * units.GB, "20GB"},
		{1 * units.GB, "1GB"},
		{20 * units.MB, "20MB"},
		{1 * units.MB, "1MB"},
		{20 * units.KB, "20KB"},
		{1 * units.KB, "1KB"},
		{999, "999B"},
		{1, "1B"},
		{0, "0B"},
		{-20 * units.TB, "-20TB"},
		{-1 * units.TB, "-1TB"},
		{-20 * units.GB, "-20GB"},
		{-1 * units.GB, "-1GB"},
		{-20 * units.MB, "-20MB"},
		{-1 * units.MB, "-1MB"},
		{-20 * units.KB, "-20KB"},
		{-1 * units.KB, "-1KB"},
		{-999, "-999B"},
		{-1, "-1B"},
	}
	for _, tc := range tcsMB {
		sz := SizeBytes(tc.i)
		s := fmt.Sprintf("%s", sz)
		assert.Equal(tc.str, s)
		assert.Equal(tc.str, SizeBytesToString(int64(tc.i)))
	}
}

func TestRoundUpBytes(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	sizeBytes := int64(20)
	assert.EqualValues(BytesInMiB, RoundUpBytes(sizeBytes, BytesInMiB))

	sizeBytes = int64(BytesInMiB + 20)
	assert.EqualValues(2*BytesInMiB, RoundUpBytes(sizeBytes, BytesInMiB))

	sizeBytes = int64(0)
	assert.EqualValues(0, RoundUpBytes(sizeBytes, BytesInMiB))

	sizeBytes = int64(BytesInMiB * 2)
	assert.EqualValues(BytesInMiB*2, RoundUpBytes(sizeBytes, BytesInMiB))

	sizeBytes = int64(BytesInMiB * 2)
	assert.EqualValues(BytesInMiB*2, RoundUpBytes(sizeBytes, BytesInMiB))

	sizeBytes = int64(5000000000) // 5 GB
	assert.EqualValues(5000658944, RoundUpBytes(sizeBytes, BytesInMiB))

	sizeBytes = int64(5368709120) // 5 GiB
	assert.EqualValues(5368709120, RoundUpBytes(sizeBytes, BytesInMiB))
}

func TestK8sSizeToString(t *testing.T) {
	assert := assert.New(t)

	tcsB2B := []struct {
		i   units.Base2Bytes
		str string
	}{
		{20 * units.TiB, "20Ti"},
		{1 * units.TiB, "1Ti"},
		{20 * units.GiB, "20Gi"},
		{1 * units.GiB, "1Gi"},
		{20 * units.MiB, "20Mi"},
		{1 * units.MiB, "1Mi"},
		{20 * units.KiB, "20Ki"},
		{1 * units.KiB, "1Ki"},
		{999, "999"},
		{1, "1"},
		{0, "0"},
		{-20 * units.TiB, "-20Ti"},
		{-1 * units.TiB, "-1Ti"},
		{-20 * units.GiB, "-20Gi"},
		{-1 * units.GiB, "-1Gi"},
		{-20 * units.MiB, "-20Mi"},
		{-1 * units.MiB, "-1Mi"},
		{-20 * units.KiB, "-20Ki"},
		{-1 * units.KiB, "-1Ki"},
		{-999, "-999"},
		{-1, "-1"},
	}
	for _, tc := range tcsB2B {
		sz := K8sSizeBytes(tc.i)
		s := fmt.Sprintf("%s", sz)
		assert.Equal(tc.str, s)
	}

	tcsMB := []struct {
		i   units.MetricBytes
		str string
	}{
		{20 * units.TB, "20T"},
		{1 * units.TB, "1T"},
		{20 * units.GB, "20G"},
		{1 * units.GB, "1G"},
		{20 * units.MB, "20M"},
		{1 * units.MB, "1M"},
		{20 * units.KB, "20K"},
		{1 * units.KB, "1K"},
		{999, "999"},
		{1, "1"},
		{0, "0"},
		{-20 * units.TB, "-20T"},
		{-1 * units.TB, "-1T"},
		{-20 * units.GB, "-20G"},
		{-1 * units.GB, "-1G"},
		{-20 * units.MB, "-20M"},
		{-1 * units.MB, "-1M"},
		{-20 * units.KB, "-20K"},
		{-1 * units.KB, "-1K"},
		{-999, "-999"},
		{-1, "-1"},
	}
	for _, tc := range tcsMB {
		sz := K8sSizeBytes(tc.i)
		s := fmt.Sprintf("%s", sz)
		assert.Equal(tc.str, s)
	}
}
