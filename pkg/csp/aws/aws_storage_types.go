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


package aws

import (
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/alecthomas/units"
	"github.com/go-openapi/swag"
)

var awsCspStorageTypes = []*models.CSPStorageType{
	&models.CSPStorageType{
		CspDomainType:                    CSPDomainType,
		Description:                      "EBS General Purpose SSD (gp2) Volume",
		Name:                             "Amazon gp2",
		MinAllocationSizeBytes:           swag.Int64(int64(units.GiB)),
		MaxAllocationSizeBytes:           swag.Int64(int64(16 * units.TiB)),
		PreferredAllocationSizeBytes:     swag.Int64(int64(10 * units.GiB)),
		PreferredAllocationUnitSizeBytes: swag.Int64(int64(10 * units.GiB)),
		ParcelSizeBytes:                  swag.Int64(int64(units.GiB - (172 * units.MiB))), // == 852 MiB: 12 parcels per 10GiB GP2 with no wasted space
		AccessibilityScope:               "CSPDOMAIN",
		CspStorageTypeAttributes: map[string]models.ValueType{
			PAService:       models.ValueType{Kind: "STRING", Value: ServiceEC2},
			PAEC2VolumeType: models.ValueType{Kind: "STRING", Value: "gp2"},
		},
		SscList: &models.SscList{SscListMutable: models.SscListMutable{
			"Response Time Average": {Kind: "DURATION", Value: "5ms"},
			"Response Time Maximum": {Kind: "DURATION", Value: "200ms"},
			"Availability":          {Kind: "PERCENTAGE", Value: "99.9%"},
		}},
		ProvisioningUnit: &models.ProvisioningUnit{
			IOPS:       swag.Int64(int64(3)),
			Throughput: swag.Int64(0),
		},
		DeviceType: "SSD",
	},
	&models.CSPStorageType{
		CspDomainType:                    CSPDomainType,
		Description:                      "EBS Provisioned IOPS SSD (io1) Volume",
		Name:                             "Amazon io1",
		MinAllocationSizeBytes:           swag.Int64(int64(4 * units.GiB)),
		MaxAllocationSizeBytes:           swag.Int64(int64(16 * units.TiB)),
		PreferredAllocationSizeBytes:     swag.Int64(int64(4 * units.GiB)),
		PreferredAllocationUnitSizeBytes: swag.Int64(int64(4 * units.GiB)),
		ParcelSizeBytes:                  swag.Int64(int64(units.GiB - (4 * units.MiB))), // == 1020 MiB: 4 parcels per 4 GiB IO1 with no wasted space
		AccessibilityScope:               "CSPDOMAIN",
		CspStorageTypeAttributes: map[string]models.ValueType{
			PAService:       models.ValueType{Kind: "STRING", Value: ServiceEC2},
			PAEC2VolumeType: models.ValueType{Kind: "STRING", Value: "io1"},
			PAIopsGB:        models.ValueType{Kind: "INT", Value: "50"},
			PAIopsMin:       models.ValueType{Kind: "INT", Value: "100"},
			PAIopsMax:       models.ValueType{Kind: "INT", Value: "32000"},
		},
		SscList: &models.SscList{SscListMutable: models.SscListMutable{
			"Response Time Average": {Kind: "DURATION", Value: "5ms"},
			"Response Time Maximum": {Kind: "DURATION", Value: "10ms"},
			"Availability":          {Kind: "PERCENTAGE", Value: "99.9%"},
		}},
		ProvisioningUnit: &models.ProvisioningUnit{
			IOPS:       swag.Int64(int64(4)),
			Throughput: swag.Int64(0),
		},
		DeviceType: "SSD",
	},
	&models.CSPStorageType{
		CspDomainType:                    CSPDomainType,
		Description:                      "EBS Throughput Optimized HDD (st1) Volume",
		Name:                             "Amazon st1",
		MinAllocationSizeBytes:           swag.Int64(int64(500 * units.GiB)),
		MaxAllocationSizeBytes:           swag.Int64(int64(16 * units.TiB)),
		PreferredAllocationSizeBytes:     swag.Int64(int64(500 * units.GiB)),
		PreferredAllocationUnitSizeBytes: swag.Int64(int64(100 * units.GiB)),
		ParcelSizeBytes:                  swag.Int64(int64(units.GiB - (28 * units.MiB))), // == 996 MiB: 514 parcels per 500 GiB ST1 with 40 MiB wasted space
		AccessibilityScope:               "CSPDOMAIN",
		CspStorageTypeAttributes: map[string]models.ValueType{
			PAService:       models.ValueType{Kind: "STRING", Value: ServiceEC2},
			PAEC2VolumeType: models.ValueType{Kind: "STRING", Value: "st1"},
		},
		SscList: &models.SscList{SscListMutable: models.SscListMutable{
			"Response Time Average": {Kind: "DURATION", Value: "175ms"},
			"Response Time Maximum": {Kind: "DURATION", Value: "2s"},
			"Availability":          {Kind: "PERCENTAGE", Value: "99.9%"},
		}},
		ProvisioningUnit: &models.ProvisioningUnit{
			IOPS:       swag.Int64(0),
			Throughput: swag.Int64(int64(400000)),
		},
		DeviceType: "HDD",
	},
	&models.CSPStorageType{
		CspDomainType:                    CSPDomainType,
		Description:                      "EBS Cold HDD (sc1) Volume",
		Name:                             "Amazon sc1",
		MinAllocationSizeBytes:           swag.Int64(int64(500 * units.GiB)),
		MaxAllocationSizeBytes:           swag.Int64(int64(16 * units.TiB)),
		PreferredAllocationSizeBytes:     swag.Int64(int64(500 * units.GiB)),
		PreferredAllocationUnitSizeBytes: swag.Int64(int64(100 * units.GiB)),
		ParcelSizeBytes:                  swag.Int64(int64(units.GiB - (28 * units.MiB))), // == 996 MiB: 514 parcels per 500 GiB ST1 with 40 MiB wasted space
		AccessibilityScope:               "CSPDOMAIN",
		CspStorageTypeAttributes: map[string]models.ValueType{
			PAService:       models.ValueType{Kind: "STRING", Value: ServiceEC2},
			PAEC2VolumeType: models.ValueType{Kind: "STRING", Value: "sc1"},
		},
		SscList: &models.SscList{SscListMutable: models.SscListMutable{
			"Response Time Average": {Kind: "DURATION", Value: "1s"},
			"Response Time Maximum": {Kind: "DURATION", Value: "5s"},
			"Availability":          {Kind: "PERCENTAGE", Value: "99.9%"},
		}},
		ProvisioningUnit: &models.ProvisioningUnit{
			IOPS:       swag.Int64(0),
			Throughput: swag.Int64(int64(12000)),
		},
		DeviceType: "HDD",
	},
	&models.CSPStorageType{
		CspDomainType:                    CSPDomainType,
		Description:                      "Amazon SSD Instance Storage",
		Name:                             "Amazon SSD Instance Store",
		MinAllocationSizeBytes:           swag.Int64(int64(units.GiB)),
		MaxAllocationSizeBytes:           swag.Int64(int64(1900 * units.GiB)),
		PreferredAllocationSizeBytes:     swag.Int64(int64(800 * units.GiB)), // N/A
		PreferredAllocationUnitSizeBytes: swag.Int64(int64(100 * units.GiB)), // N/A
		ParcelSizeBytes:                  swag.Int64(int64(units.GiB)),       // N/A
		AccessibilityScope:               "NODE",
		CspStorageTypeAttributes: map[string]models.ValueType{
			csp.CSPEphemeralStorageType: models.ValueType{Kind: "STRING", Value: csp.EphemeralTypeSSD},
		},
		SscList: &models.SscList{SscListMutable: models.SscListMutable{
			"Response Time Average": {Kind: "DURATION", Value: "375us"},
			"Response Time Maximum": {Kind: "DURATION", Value: "1ms"},
			"Availability":          {Kind: "PERCENTAGE", Value: "99.99%"},
		}},
		ProvisioningUnit: &models.ProvisioningUnit{
			IOPS:       swag.Int64(int64(137)),
			Throughput: swag.Int64(0),
		},
		DeviceType: "SSD",
	},
}

// StorageTypeToServiceVolumeType finds the AWS service and volume type for a CspStorageType
func StorageTypeToServiceVolumeType(stName models.CspStorageType) (string, string, *models.CSPStorageType) {
	for _, st := range awsCspStorageTypes {
		if st.Name == stName {
			var service, volType string
			// ignore Kind when parsing these internal attributes
			if vt, ok := st.CspStorageTypeAttributes[PAService]; ok {
				service = vt.Value
			}
			if vt, ok := st.CspStorageTypeAttributes[PAEC2VolumeType]; ok {
				volType = vt.Value
			}
			return service, volType, st
		}
	}
	return "", "", nil
}

var awsEC2VolTypeToCSPStorageType = map[string]models.CspStorageType{}

func init() {
	awsCreateSTMap()
}

func awsCreateSTMap() {
	for _, st := range awsCspStorageTypes {
		var service, volType string
		// ignore Kind when parsing these internal attributes
		if vt, ok := st.CspStorageTypeAttributes[PAService]; ok {
			service = vt.Value
		}
		if vt, ok := st.CspStorageTypeAttributes[PAEC2VolumeType]; ok {
			volType = vt.Value
		}
		if service == ServiceEC2 {
			awsEC2VolTypeToCSPStorageType[volType] = st.Name
		}
	}
}

// EC2VolTypeToCSPStorageType returns the CSPStorageType name for an EC2 volume type
func EC2VolTypeToCSPStorageType(volType string) models.CspStorageType {
	st, _ := awsEC2VolTypeToCSPStorageType[volType]
	return st
}
