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

	"github.com/alecthomas/units"
)

// Constants describing Bytes in Multiples
const (
	BytesInKiB = 1024
	BytesInMiB = 1048576
	BytesInGiB = 1073741824
)

var sizeBuckets = []struct {
	lowerBound int64
	unit       string
}{
	{int64(units.TiB), "TiB"},
	{int64(units.TB), "TB"},
	{int64(units.GiB), "GiB"},
	{int64(units.GB), "GB"},
	{int64(units.MiB), "MiB"},
	{int64(units.MB), "MB"},
	{int64(units.KiB), "KiB"},
	{int64(units.KB), "KB"},
	{1, "B"},
}

// SizeBytesToString converts a 64 bit size into a string with highest suffix
func SizeBytesToString(size int64) string {
	if size == 0 {
		return "0B"
	}
	sign := ""
	if size < 0 {
		sign = "-"
		size = -size
	}
	type choice struct {
		i, f int64
		u    string
	}
	choices := []choice{}
	for _, b := range sizeBuckets {
		if size >= b.lowerBound {
			i := size / b.lowerBound
			f := size - i*b.lowerBound
			choices = append(choices, choice{i, f, b.unit})
			if f == 0 {
				break
			}
		}
	}
	// best choice has a 0 fractional part
	str := ""
	for _, c := range choices {
		if c.f == 0 {
			str = fmt.Sprintf("%s%d%s", sign, c.i, c.u)
			break
		}
	}
	return str
}

// SizeBytes can print its value using byte multiplier suffixes
type SizeBytes int64

func (sz SizeBytes) String() string {
	return SizeBytesToString(int64(sz))
}

// RoundUpBytes makes sure the SizeBytes argument is a multiple of N. If its not it will pad it till it reaches the closest multiple.
func RoundUpBytes(sizeBytes int64, n int64) int64 {
	return (sizeBytes + n - 1) / n * n
}

// K8sSizeBytes can print its value using k8s byte multiplier suffixes
type K8sSizeBytes int64

func (ksz K8sSizeBytes) String() string {
	sizeString := SizeBytesToString(int64(ksz))
	stringLen := len(sizeString)
	return sizeString[:stringLen-1]
}
