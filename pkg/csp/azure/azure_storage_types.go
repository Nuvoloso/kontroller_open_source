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
	"github.com/docker/go-units"
	"github.com/go-openapi/swag"
)

// TBD: Adapted from AWS - all numbers are unverified placeholders
// TBD: Need to determine if CspStorageTypeAttributes are needed
// No instance storage available in Azure - TBD check if temporary disk would work
var azureCspStorageTypes = []*models.CSPStorageType{
	&models.CSPStorageType{ // AWS: gp2
		CspDomainType:                    CSPDomainType,
		Description:                      "https://docs.microsoft.com/en-us/azure/virtual-machines/windows/disks-types#standard-ssd",
		Name:                             "Azure Standard SSD",
		MinAllocationSizeBytes:           swag.Int64(int64(units.GiB)),
		MaxAllocationSizeBytes:           swag.Int64(int64(32767 * units.GiB)),
		PreferredAllocationSizeBytes:     swag.Int64(int64(10 * units.GiB)),
		PreferredAllocationUnitSizeBytes: swag.Int64(int64(10 * units.GiB)),
		ParcelSizeBytes:                  swag.Int64(int64(units.GiB - (172 * units.MiB))), // == 852 MiB: 12 parcels per 10GiB GP2 with no wasted space
		AccessibilityScope:               "CSPDOMAIN",
		CspStorageTypeAttributes: map[string]models.ValueType{
			PADiskType: models.ValueType{Kind: "STRING", Value: "StandardSSD_LRS"},
		},
		SscList: &models.SscList{SscListMutable: models.SscListMutable{
			"Response Time Average": {Kind: "DURATION", Value: "5ms"},
			"Response Time Maximum": {Kind: "DURATION", Value: "200ms"},
			"Availability":          {Kind: "PERCENTAGE", Value: "99.9%"},
		}},
		ProvisioningUnit: &models.ProvisioningUnit{
			IOPS:       swag.Int64(int64(4)),
			Throughput: swag.Int64(0),
		},
		DeviceType: "SSD",
	},
	&models.CSPStorageType{ // AWS: st1
		CspDomainType:                    CSPDomainType,
		Description:                      "https://docs.microsoft.com/en-us/azure/virtual-machines/windows/disks-types#standard-hdd",
		Name:                             "Azure Standard HDD",
		MinAllocationSizeBytes:           swag.Int64(int64(1 * units.GiB)),
		MaxAllocationSizeBytes:           swag.Int64(int64(32767 * units.GiB)),
		PreferredAllocationSizeBytes:     swag.Int64(int64(500 * units.GiB)),
		PreferredAllocationUnitSizeBytes: swag.Int64(int64(100 * units.GiB)),
		ParcelSizeBytes:                  swag.Int64(int64(units.GiB - (28 * units.MiB))), // == 996 MiB: 514 parcels per 500 GiB ST1 with 40 MiB wasted space
		AccessibilityScope:               "CSPDOMAIN",
		CspStorageTypeAttributes: map[string]models.ValueType{
			PADiskType: models.ValueType{Kind: "STRING", Value: "Standard_LRS"},
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
	&models.CSPStorageType{ // AWS: ~io1
		CspDomainType:                    CSPDomainType,
		Description:                      "https://docs.microsoft.com/en-us/azure/virtual-machines/windows/disks-types#premium-ssd",
		Name:                             "Azure Premium SSD",
		MinAllocationSizeBytes:           swag.Int64(int64(1 * units.GiB)),
		MaxAllocationSizeBytes:           swag.Int64(int64(32767 * units.GiB)),
		PreferredAllocationSizeBytes:     swag.Int64(int64(4 * units.GiB)),
		PreferredAllocationUnitSizeBytes: swag.Int64(int64(4 * units.GiB)),
		ParcelSizeBytes:                  swag.Int64(int64(units.GiB - (4 * units.MiB))), // == 1020 MiB: 4 parcels per 4 GiB IO1 with no wasted space
		AccessibilityScope:               "CSPDOMAIN",
		CspStorageTypeAttributes: map[string]models.ValueType{
			PADiskType: models.ValueType{Kind: "STRING", Value: "Premium_LRS"},
		},
		SscList: &models.SscList{SscListMutable: models.SscListMutable{
			"Response Time Average": {Kind: "DURATION", Value: "5ms"},
			"Response Time Maximum": {Kind: "DURATION", Value: "10ms"},
			"Availability":          {Kind: "PERCENTAGE", Value: "99.9%"},
		}},
		ProvisioningUnit: &models.ProvisioningUnit{
			IOPS:       swag.Int64(int64(5)),
			Throughput: swag.Int64(0),
		},
		DeviceType: "SSD",
	},
	// TBD: UltraSSD_LRS
}

// CspStorageTypeToDiskType finds the disk type for a CspStorageType
func CspStorageTypeToDiskType(stName models.CspStorageType) (string, *models.CSPStorageType) {
	for _, st := range azureCspStorageTypes {
		if st.Name == stName {
			var diskType string
			// ignore Kind when parsing these internal attributes
			if vt, ok := st.CspStorageTypeAttributes[PADiskType]; ok {
				diskType = vt.Value
			}
			return diskType, st
		}
	}
	return "", nil
}

var azureDiskTypeToCSPStorageType = map[string]models.CspStorageType{}

func init() {
	azureCreateSTMap()
}

func azureCreateSTMap() {
	for _, st := range azureCspStorageTypes {
		var diskType string
		// ignore Kind when parsing these internal attributes
		if vt, ok := st.CspStorageTypeAttributes[PADiskType]; ok {
			diskType = vt.Value
		}
		azureDiskTypeToCSPStorageType[diskType] = st.Name
	}
}

// DiskTypeToCspStorageType returns the CspStorageType for an Azure disk
func DiskTypeToCspStorageType(diskType string) models.CspStorageType {
	return azureDiskTypeToCSPStorageType[diskType]
}
