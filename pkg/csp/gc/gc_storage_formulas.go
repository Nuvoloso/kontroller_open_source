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


package gc

import (
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/go-openapi/swag"
)

// Storage Formulas are currently fully documented in pkg/csp/aws/aws_storage_formulas.go
// These formulas are placeholders and probably will result in violations. TBD fully vet and improve them.
//	A) Requirements for what we need to hit
//	B) Assumptions that snaps and replication won't impact response time
// As the system evolves, things may need to change.
var gcStorageFormulas = []*models.StorageFormula{
	{ // for OLTP Premier
		Description: "2ms, Random, Read/Write",
		Name:        "GCP-2ms-rand-rw",
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
			"GCE pd-ssd": {Percentage: swag.Int32(100), Overhead: swag.Int32(270)}, // must over-provision; cannot tune IOPS
		},
		CacheComponent: map[string]models.StorageFormulaTypeElement{
			"GCE local-ssd": {Percentage: swag.Int32(40)},
		},
		StorageLayout: "mirrored",
		CspDomainType: CSPDomainType,
	},
	{ // for OLTP
		Description: "5ms, Random, Read/Write",
		Name:        "GCP-5ms-rand-rw",
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
			"GCE pd-ssd": {Percentage: swag.Int32(100), Overhead: swag.Int32(70)},
		},
		CacheComponent: map[string]models.StorageFormulaTypeElement{
			"GCE local-ssd": {Percentage: swag.Int32(30)},
		},
		StorageLayout: "mirrored",
		CspDomainType: CSPDomainType,
	},
	{ // for General Premier
		Description: "8ms, Random, Read/Write",
		Name:        "GCP-8ms-rand-rw",
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
			"GCE pd-ssd": {Percentage: swag.Int32(100), Overhead: swag.Int32(70)},
		},
		CacheComponent: map[string]models.StorageFormulaTypeElement{
			"GCE local-ssd": {Percentage: swag.Int32(20)},
		},
		StorageLayout: "mirrored",
		CspDomainType: CSPDomainType,
	},
	{ // for Technical Applications
		Description: "8ms, Sequential, Read/Write",
		Name:        "GCP-8ms-seq-rw",
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
			"GCE pd-ssd": {Percentage: swag.Int32(100), Overhead: swag.Int32(70)},
		},
		CacheComponent: map[string]models.StorageFormulaTypeElement{
			"GCE local-ssd": {Percentage: swag.Int32(10)},
		},
		StorageLayout: "mirrored",
		CspDomainType: CSPDomainType,
	},
	{ // for DSS
		Description: "8ms, Sequential, Read Mostly",
		Name:        "GCP-8ms-seq-rm",
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
			"GCE pd-standard": {Percentage: swag.Int32(100), Overhead: swag.Int32(70)},
		},
		CacheComponent: map[string]models.StorageFormulaTypeElement{
			"GCE local-ssd": {Percentage: swag.Int32(20)},
		},
		StorageLayout: "mirrored",
		CspDomainType: CSPDomainType,
	},
	{ // for Streaming Analytics
		Description: "8ms, Streaming, Read/Write",
		Name:        "GCP-8ms-strm-rw",
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
			"GCE pd-standard": {Percentage: swag.Int32(100), Overhead: swag.Int32(70)},
		},
		CacheComponent: map[string]models.StorageFormulaTypeElement{
			"GCE local-ssd": {Percentage: swag.Int32(10)},
		},
		StorageLayout: "mirrored",
		CspDomainType: CSPDomainType,
	},
	{ // for General
		Description: "10ms, Random, Read/Write",
		Name:        "GCP-10ms-rand-rw",
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
			"GCE pd-ssd": {Percentage: swag.Int32(100), Overhead: swag.Int32(70)},
		},
		CacheComponent: map[string]models.StorageFormulaTypeElement{},
		StorageLayout:  "mirrored",
		CspDomainType:  CSPDomainType,
	},
	{ // for Online Archive
		Description: "50ms, Sequential, Write Mostly",
		Name:        "GCP-50ms-seq-wm",
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
			"GCE pd-standard": {Percentage: swag.Int32(100), Overhead: swag.Int32(70)},
		},
		CacheComponent: map[string]models.StorageFormulaTypeElement{},
		StorageLayout:  "standalone",
		CspDomainType:  CSPDomainType,
	},
}
