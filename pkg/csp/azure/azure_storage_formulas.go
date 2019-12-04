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


package azure

import (
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/go-openapi/swag"
)

// TBD: Adapted from AWS. All values are unverified.
// There is no equivalent of instance storage in Azure.
var azureStorageFormulas = []*models.StorageFormula{
	{
		Description: "2ms, Random, Read/Write",
		Name:        "Azure-2ms-rand-rw",
		IoProfile: &models.IoProfile{
			IoPattern:    &models.IoPattern{Name: "random", MinSizeBytesAvg: swag.Int32(0), MaxSizeBytesAvg: swag.Int32(16384)},
			ReadWriteMix: &models.ReadWriteMix{Name: "read-write", MinReadPercent: swag.Int32(30), MaxReadPercent: swag.Int32(70)},
		},
		SscList: models.SscListMutable{
			"Response Time Average": {Kind: "DURATION", Value: "2ms"},
			"Response Time Maximum": {Kind: "DURATION", Value: "5ms"},
			"Availability":          {Kind: "PERCENTAGE", Value: "99.999%"},
		},
		StorageComponent: map[string]models.StorageFormulaTypeElement{
			"Azure Premium SSD": {Percentage: swag.Int32(100), Overhead: swag.Int32(70)},
		},
		CacheComponent: map[string]models.StorageFormulaTypeElement{},
		StorageLayout:  "mirrored",
		CspDomainType:  CSPDomainType,
	},
	{
		Description: "5ms, Random, Read/Write",
		Name:        "Azure-5ms-rand-rw",
		IoProfile: &models.IoProfile{
			IoPattern:    &models.IoPattern{Name: "random", MinSizeBytesAvg: swag.Int32(0), MaxSizeBytesAvg: swag.Int32(16384)},
			ReadWriteMix: &models.ReadWriteMix{Name: "read-write", MinReadPercent: swag.Int32(30), MaxReadPercent: swag.Int32(70)},
		},
		SscList: models.SscListMutable{
			"Response Time Average": {Kind: "DURATION", Value: "5ms"},
			"Response Time Maximum": {Kind: "DURATION", Value: "10ms"},
			"Availability":          {Kind: "PERCENTAGE", Value: "99.999%"},
		},
		StorageComponent: map[string]models.StorageFormulaTypeElement{
			"Azure Premium SSD": {Percentage: swag.Int32(100), Overhead: swag.Int32(70)},
		},
		CacheComponent: map[string]models.StorageFormulaTypeElement{},
		StorageLayout:  "mirrored",
		CspDomainType:  CSPDomainType,
	},
	{
		Description: "8ms, Random, Read/Write",
		Name:        "Azure-8ms-rand-rw",
		IoProfile: &models.IoProfile{
			IoPattern:    &models.IoPattern{Name: "random", MinSizeBytesAvg: swag.Int32(0), MaxSizeBytesAvg: swag.Int32(16384)},
			ReadWriteMix: &models.ReadWriteMix{Name: "read-write", MinReadPercent: swag.Int32(30), MaxReadPercent: swag.Int32(70)},
		},
		SscList: models.SscListMutable{
			"Response Time Average": {Kind: "DURATION", Value: "8ms"},
			"Response Time Maximum": {Kind: "DURATION", Value: "50ms"},
			"Availability":          {Kind: "PERCENTAGE", Value: "99.999%"},
		},
		StorageComponent: map[string]models.StorageFormulaTypeElement{
			"Azure Standard SSD": {Percentage: swag.Int32(100), Overhead: swag.Int32(70)},
		},
		CacheComponent: map[string]models.StorageFormulaTypeElement{},
		StorageLayout:  "mirrored",
		CspDomainType:  CSPDomainType,
	},
	{
		Description: "8ms, Sequential, Read/Write",
		Name:        "Azure-8ms-seq-rw",
		IoProfile: &models.IoProfile{
			IoPattern:    &models.IoPattern{Name: "sequential", MinSizeBytesAvg: swag.Int32(16384), MaxSizeBytesAvg: swag.Int32(65536)},
			ReadWriteMix: &models.ReadWriteMix{Name: "read-write", MinReadPercent: swag.Int32(30), MaxReadPercent: swag.Int32(70)},
		},
		SscList: models.SscListMutable{
			"Response Time Average": {Kind: "DURATION", Value: "8ms"},
			"Response Time Maximum": {Kind: "DURATION", Value: "50ms"},
			"Availability":          {Kind: "PERCENTAGE", Value: "99.999%"},
		},
		StorageComponent: map[string]models.StorageFormulaTypeElement{
			"Azure Standard SSD": {Percentage: swag.Int32(100), Overhead: swag.Int32(70)},
		},
		CacheComponent: map[string]models.StorageFormulaTypeElement{},
		StorageLayout:  "mirrored",
		CspDomainType:  CSPDomainType,
	},
	{
		Description: "8ms, Sequential, Read Mostly",
		Name:        "Azure-8ms-seq-rm",
		IoProfile: &models.IoProfile{
			IoPattern:    &models.IoPattern{Name: "sequential", MinSizeBytesAvg: swag.Int32(16384), MaxSizeBytesAvg: swag.Int32(65536)},
			ReadWriteMix: &models.ReadWriteMix{Name: "read-mostly", MinReadPercent: swag.Int32(70), MaxReadPercent: swag.Int32(100)},
		},
		SscList: models.SscListMutable{
			"Response Time Average": {Kind: "DURATION", Value: "8ms"},
			"Response Time Maximum": {Kind: "DURATION", Value: "100ms"},
			"Availability":          {Kind: "PERCENTAGE", Value: "99.999%"},
		},
		StorageComponent: map[string]models.StorageFormulaTypeElement{
			"Azure Standard HDD": {Percentage: swag.Int32(100), Overhead: swag.Int32(70)},
		},
		CacheComponent: map[string]models.StorageFormulaTypeElement{},
		StorageLayout:  "mirrored",
		CspDomainType:  CSPDomainType,
	},
	{
		Description: "8ms, Streaming, Read/Write",
		Name:        "Azure-8ms-strm-rw",
		IoProfile: &models.IoProfile{
			IoPattern:    &models.IoPattern{Name: "streaming", MinSizeBytesAvg: swag.Int32(65536), MaxSizeBytesAvg: swag.Int32(131072)},
			ReadWriteMix: &models.ReadWriteMix{Name: "read-write", MinReadPercent: swag.Int32(30), MaxReadPercent: swag.Int32(70)},
		},
		SscList: models.SscListMutable{
			"Response Time Average": {Kind: "DURATION", Value: "8ms"},
			"Response Time Maximum": {Kind: "DURATION", Value: "20ms"},
			"Availability":          {Kind: "PERCENTAGE", Value: "99.999%"},
		},
		StorageComponent: map[string]models.StorageFormulaTypeElement{
			"Azure Standard HDD": {Percentage: swag.Int32(100), Overhead: swag.Int32(70)},
		},
		CacheComponent: map[string]models.StorageFormulaTypeElement{},
		StorageLayout:  "mirrored",
		CspDomainType:  CSPDomainType,
	},
	{
		Description: "10ms, Random, Read/Write",
		Name:        "Azure-10ms-rand-rw",
		IoProfile: &models.IoProfile{
			IoPattern:    &models.IoPattern{Name: "random", MinSizeBytesAvg: swag.Int32(0), MaxSizeBytesAvg: swag.Int32(16384)},
			ReadWriteMix: &models.ReadWriteMix{Name: "read-write", MinReadPercent: swag.Int32(30), MaxReadPercent: swag.Int32(70)},
		},
		SscList: models.SscListMutable{
			"Response Time Average": {Kind: "DURATION", Value: "10ms"},
			"Response Time Maximum": {Kind: "DURATION", Value: "100ms"},
			"Availability":          {Kind: "PERCENTAGE", Value: "99.999%"},
		},
		StorageComponent: map[string]models.StorageFormulaTypeElement{
			"Azure Standard SSD": {Percentage: swag.Int32(100), Overhead: swag.Int32(70)},
		},
		CacheComponent: map[string]models.StorageFormulaTypeElement{},
		StorageLayout:  "mirrored",
		CspDomainType:  CSPDomainType,
	},
	{
		Description: "50ms, Sequential, Write Mostly",
		Name:        "Azure-50ms-seq-wm",
		IoProfile: &models.IoProfile{
			IoPattern:    &models.IoPattern{Name: "sequential", MinSizeBytesAvg: swag.Int32(16384), MaxSizeBytesAvg: swag.Int32(65536)},
			ReadWriteMix: &models.ReadWriteMix{Name: "write-mostly", MinReadPercent: swag.Int32(0), MaxReadPercent: swag.Int32(30)},
		},
		SscList: models.SscListMutable{
			"Response Time Average": {Kind: "DURATION", Value: "50ms"},
			"Response Time Maximum": {Kind: "DURATION", Value: "2s"},
			"Availability":          {Kind: "PERCENTAGE", Value: "99.999%"},
		},
		StorageComponent: map[string]models.StorageFormulaTypeElement{
			"Azure Standard SSD": {Percentage: swag.Int32(100), Overhead: swag.Int32(70)},
		},
		CacheComponent: map[string]models.StorageFormulaTypeElement{},
		StorageLayout:  "standalone",
		CspDomainType:  CSPDomainType,
	},
}
