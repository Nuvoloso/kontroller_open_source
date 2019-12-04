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
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	"github.com/docker/go-units"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestAwsCspStorageTypes(t *testing.T) {
	assert := assert.New(t)

	// minimal tests regarding parcel size
	for i, cst := range awsCspStorageTypes {
		parcelSize := swag.Int64Value(cst.ParcelSizeBytes)
		assert.Zerof(parcelSize%nuvoapi.DefaultSegmentSizeBytes,
			"Expected parcel size %d of storage type %d:%s to be a multiple of the NuvoAPI segment size", parcelSize, i, cst.Name)
		var numParcels, wasted int64
		switch cst.Name {
		case "Amazon gp2":
			numParcels, wasted = 12, 0
		case "Amazon io1":
			numParcels, wasted = 4, 0
		case "Amazon st1":
			numParcels, wasted = 514, 40*units.MiB
		case "Amazon sc1":
			numParcels, wasted = 514, 40*units.MiB
		default:
			if cst.AccessibilityScope == "NODE" { // ephemeral storage does not use ParcelSizeBytes or PreferredAllocationSizeBytes
				continue
			}
			assert.Failf("failed", "Unexpected storage type %d:%s", i, cst.Name)
		}
		// only the preferred size matters for fixed size storage capacity allocation, TBD generalize this
		formattedSize := swag.Int64Value(cst.PreferredAllocationSizeBytes) - nuvoapi.DefaultDeviceFormatOverheadBytes
		assert.Equalf(wasted, formattedSize-numParcels*parcelSize, "Expected %d wasted for storage type %d:%s", wasted, i, cst.Name)

		// ensure that the minimum size less format overhead is >= 1 parcel for variable size storage capacity allocation
		assert.True(parcelSize <= (swag.Int64Value(cst.MinAllocationSizeBytes) - nuvoapi.DefaultDeviceFormatOverheadBytes))
	}
}
