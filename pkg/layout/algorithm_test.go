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


package layout

import (
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/stretchr/testify/assert"
)

func TestLayoutDescriptor(t *testing.T) {
	assert := assert.New(t)

	// Search
	for n := range supportedAlgorithms {
		la, err := FindAlgorithm(n)
		assert.NoError(err, "case: %s", n)
		assert.NotNil(la, "case: %s", n)
		la, err = FindAlgorithm(n + "Foo")
		assert.Error(err, "case: %sFoo", n)
		assert.Nil(la, "case: %sFoo", n)
	}

	// AlgorithmStandaloneRemoteShared capabilities
	la, err := FindAlgorithm(AlgorithmStandaloneNetworkedShared)
	assert.NoError(err)
	assert.Equal(AlgorithmStandaloneNetworkedShared, la.Name)
	assert.Equal(models.StorageLayoutStandalone, la.StorageLayout)
	assert.True(la.SharedStorageOk)
	assert.True(la.RemoteStorageOk)
	assert.False(la.VariableSizeStorageOk)

	// AlgorithmStandaloneLocalUnshared capabilities
	la, err = FindAlgorithm(AlgorithmStandaloneLocalUnshared)
	assert.NoError(err)
	assert.Equal(AlgorithmStandaloneLocalUnshared, la.Name)
	assert.Equal(models.StorageLayoutStandalone, la.StorageLayout)
	assert.False(la.SharedStorageOk)
	assert.False(la.RemoteStorageOk)
	assert.True(la.VariableSizeStorageOk)
}
