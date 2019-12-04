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
	"strings"
	"testing"

	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestAzureCspStorageTypes(t *testing.T) {
	assert := assert.New(t)
	assert.NotZero(len(azureCspStorageTypes))
	names := map[string]struct{}{}
	for _, st := range azureCspStorageTypes {
		// distinct name
		assert.NotEmpty((st.Name))
		assert.NotContains(names, string(st.Name))
		names[string(st.Name)] = struct{}{}
		assert.True(strings.HasPrefix(string(st.Name), CSPDomainType))
		// has necessary attributes
		assert.EqualValues(st.CspDomainType, CSPDomainType)
		assert.Greater(swag.Int64Value(st.MinAllocationSizeBytes), int64(0))
		assert.NotEmpty(st.CspStorageTypeAttributes)
		assert.Contains(st.CspStorageTypeAttributes, PADiskType)
		dtA, ok := st.CspStorageTypeAttributes[PADiskType]
		assert.True(ok)
		dt, cspT := CspStorageTypeToDiskType(st.Name)
		assert.NotNil(cspT)
		assert.Equal(dt, dtA.Value)
		assert.Equal(st, cspT)
		assert.Equal(st.Name, DiskTypeToCspStorageType(dtA.Value))
	}
}
