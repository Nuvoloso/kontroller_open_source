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
	"strings"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/alecthomas/units"
	"github.com/go-openapi/swag"
)

// Response times measured using "ioping" with mixed read/write.
// IOPS and throughput found in GCE documentation.
// TBD actually measure regional PD/PD-SSD latency (we don't currently use them)
var gcCspStorageTypes = []*models.CSPStorageType{
	&models.CSPStorageType{
		CspDomainType:                    CSPDomainType,
		Description:                      "GCE Zonal SSD persistent disk",
		Name:                             "GCE pd-ssd",
		MinAllocationSizeBytes:           swag.Int64(int64(10 * units.GiB)),
		MaxAllocationSizeBytes:           swag.Int64(int64(64 * units.TiB)),
		PreferredAllocationSizeBytes:     swag.Int64(int64(10 * units.GiB)),
		PreferredAllocationUnitSizeBytes: swag.Int64(int64(10 * units.GiB)),
		ParcelSizeBytes:                  swag.Int64(int64(units.GiB - (172 * units.MiB))), // == 852 MiB: 12 parcels per 10GiB GP2 with no wasted space
		AccessibilityScope:               "CSPDOMAIN",
		CspStorageTypeAttributes: map[string]models.ValueType{
			PAService:    models.ValueType{Kind: "STRING", Value: ServiceGCE},
			PAVolumeType: models.ValueType{Kind: "STRING", Value: "pd-ssd"},
			PAIopsGB:     models.ValueType{Kind: "INT", Value: "30"},
			PAIopsMax:    models.ValueType{Kind: "INT", Value: "30000"},
		},
		SscList: &models.SscList{SscListMutable: models.SscListMutable{
			"Response Time Average": {Kind: "DURATION", Value: "2ms"},
			"Response Time Maximum": {Kind: "DURATION", Value: "17ms"},
			"Availability":          {Kind: "PERCENTAGE", Value: "99.9%"},
		}},
		ProvisioningUnit: &models.ProvisioningUnit{
			IOPS:       swag.Int64(int64(30)),
			Throughput: swag.Int64(0),
		},
		DeviceType: "SSD",
	},
	&models.CSPStorageType{
		CspDomainType:                    CSPDomainType,
		Description:                      "GCE Regional SSD persistent disk",
		Name:                             "GCE re-pd-ssd",
		MinAllocationSizeBytes:           swag.Int64(int64(10 * units.GiB)),
		MaxAllocationSizeBytes:           swag.Int64(int64(64 * units.TiB)),
		PreferredAllocationSizeBytes:     swag.Int64(int64(10 * units.GiB)),
		PreferredAllocationUnitSizeBytes: swag.Int64(int64(10 * units.GiB)),
		ParcelSizeBytes:                  swag.Int64(int64(units.GiB - (172 * units.MiB))), // == 852 MiB: 12 parcels per 10GiB GP2 with no wasted space
		AccessibilityScope:               "CSPDOMAIN",
		CspStorageTypeAttributes: map[string]models.ValueType{
			PAService:    models.ValueType{Kind: "STRING", Value: ServiceGCE},
			PAVolumeType: models.ValueType{Kind: "STRING", Value: "re-pd-ssd"}, // TBD
			PAIopsGB:     models.ValueType{Kind: "INT", Value: "30"},
			PAIopsMax:    models.ValueType{Kind: "INT", Value: "30000"},
		},
		SscList: &models.SscList{SscListMutable: models.SscListMutable{
			"Response Time Average": {Kind: "DURATION", Value: "3ms"},
			"Response Time Maximum": {Kind: "DURATION", Value: "24ms"},
			"Availability":          {Kind: "PERCENTAGE", Value: "99.9%"},
		}},
		ProvisioningUnit: &models.ProvisioningUnit{
			IOPS:       swag.Int64(int64(30)),
			Throughput: swag.Int64(0),
		},
		DeviceType: "SSD",
	},
	&models.CSPStorageType{
		CspDomainType:                    CSPDomainType,
		Description:                      "GCE Zonal standard persistent disk",
		Name:                             "GCE pd-standard",
		MinAllocationSizeBytes:           swag.Int64(int64(10 * units.GiB)),
		MaxAllocationSizeBytes:           swag.Int64(int64(64 * units.TiB)),
		PreferredAllocationSizeBytes:     swag.Int64(int64(500 * units.GiB)),
		PreferredAllocationUnitSizeBytes: swag.Int64(int64(100 * units.GiB)),
		ParcelSizeBytes:                  swag.Int64(int64(units.GiB - (28 * units.MiB))), // == 996 MiB: 514 parcels per 500 GiB PD with 40 MiB wasted space
		AccessibilityScope:               "CSPDOMAIN",
		CspStorageTypeAttributes: map[string]models.ValueType{
			PAService:    models.ValueType{Kind: "STRING", Value: ServiceGCE},
			PAVolumeType: models.ValueType{Kind: "STRING", Value: "pd-standard"},
		},
		SscList: &models.SscList{SscListMutable: models.SscListMutable{
			"Response Time Average": {Kind: "DURATION", Value: "20ms"},
			"Response Time Maximum": {Kind: "DURATION", Value: "100ms"},
			"Availability":          {Kind: "PERCENTAGE", Value: "99.9%"},
		}},
		ProvisioningUnit: &models.ProvisioningUnit{
			IOPS:       swag.Int64(0),
			Throughput: swag.Int64(int64(360000)),
		},
		DeviceType: "HDD",
	},
	&models.CSPStorageType{
		CspDomainType:                    CSPDomainType,
		Description:                      "GCE Regional standard persistent disk",
		Name:                             "GCE re-pd-standard", // TBD
		MinAllocationSizeBytes:           swag.Int64(int64(200 * units.GiB)),
		MaxAllocationSizeBytes:           swag.Int64(int64(64 * units.TiB)),
		PreferredAllocationSizeBytes:     swag.Int64(int64(500 * units.GiB)),
		PreferredAllocationUnitSizeBytes: swag.Int64(int64(100 * units.GiB)),
		ParcelSizeBytes:                  swag.Int64(int64(units.GiB - (28 * units.MiB))), // == 996 MiB: 514 parcels per 500 GiB PD with 40 MiB wasted space
		AccessibilityScope:               "CSPDOMAIN",
		CspStorageTypeAttributes: map[string]models.ValueType{
			PAService:    models.ValueType{Kind: "STRING", Value: ServiceGCE},
			PAVolumeType: models.ValueType{Kind: "STRING", Value: "re-pd-standard"}, // TBD
		},
		SscList: &models.SscList{SscListMutable: models.SscListMutable{
			"Response Time Average": {Kind: "DURATION", Value: "40ms"},
			"Response Time Maximum": {Kind: "DURATION", Value: "200ms"},
			"Availability":          {Kind: "PERCENTAGE", Value: "99.9%"},
		}},
		ProvisioningUnit: &models.ProvisioningUnit{
			IOPS:       swag.Int64(0),
			Throughput: swag.Int64(int64(240000)),
		},
		DeviceType: "HDD",
	},
	&models.CSPStorageType{
		CspDomainType:                    CSPDomainType,
		Description:                      "GCE Local SSD Storage",
		Name:                             "GCE local-ssd",
		MinAllocationSizeBytes:           swag.Int64(int64(375 * units.GiB)),
		MaxAllocationSizeBytes:           swag.Int64(int64(375 * units.GiB)),
		PreferredAllocationSizeBytes:     swag.Int64(int64(375 * units.GiB)), // N/A
		PreferredAllocationUnitSizeBytes: swag.Int64(int64(375 * units.GiB)), // N/A
		ParcelSizeBytes:                  swag.Int64(int64(units.GiB)),       // N/A
		AccessibilityScope:               "NODE",
		CspStorageTypeAttributes: map[string]models.ValueType{
			csp.CSPEphemeralStorageType: models.ValueType{Kind: "STRING", Value: csp.EphemeralTypeSSD},
		},
		SscList: &models.SscList{SscListMutable: models.SscListMutable{
			"Response Time Average": {Kind: "DURATION", Value: "1484us"},
			"Response Time Maximum": {Kind: "DURATION", Value: "3800us"},
			"Availability":          {Kind: "PERCENTAGE", Value: "99.9%"},
		}},
		ProvisioningUnit: &models.ProvisioningUnit{
			IOPS:       swag.Int64(int64(129)),
			Throughput: swag.Int64(0),
		},
		DeviceType: "SSD",
	},
}

// StorageTypeToServiceVolumeType finds the GCP service and volume type for a CspStorageType
func StorageTypeToServiceVolumeType(stName models.CspStorageType) (string, string, *models.CSPStorageType) {
	for _, st := range gcCspStorageTypes {
		if st.Name == stName {
			var service, volType string
			// ignore Kind when parsing these internal attributes
			if vt, ok := st.CspStorageTypeAttributes[PAService]; ok {
				service = vt.Value
			}
			if vt, ok := st.CspStorageTypeAttributes[PAVolumeType]; ok {
				volType = vt.Value
			}
			return service, volType, st
		}
	}
	return "", "", nil
}

var volTypeToCSPStorageType = map[string]models.CspStorageType{}

func init() {
	createSTMap()
}

func createSTMap() {
	for _, st := range gcCspStorageTypes {
		var service, volType string
		// ignore Kind when parsing these internal attributes
		if vt, ok := st.CspStorageTypeAttributes[PAService]; ok {
			service = vt.Value
		}
		if vt, ok := st.CspStorageTypeAttributes[PAVolumeType]; ok {
			volType = vt.Value
		}
		if service == ServiceGCE {
			volTypeToCSPStorageType[volType] = st.Name
		}
	}
}

// VolTypeToCSPStorageType returns the CSPStorageType name for a GCE volume type
func VolTypeToCSPStorageType(volType string) models.CspStorageType {
	if i := strings.LastIndex(volType, "/"); i >= 0 {
		// volType is typically a URL in the form of volTypeURL (see gc.go), actual type is the final part of the path
		volType = volType[i+1:]
	}
	st, _ := volTypeToCSPStorageType[volType]
	return st
}
