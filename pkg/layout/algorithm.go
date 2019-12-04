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
	"fmt"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
)

// Algorithm represents a layout algorithm
type Algorithm struct {
	// Name returns the name of the algorithm
	Name string
	// StorageLayout returns the layout
	StorageLayout models.StorageLayout
	// SharedStorageOk returns true if storage can be shared between volumes
	SharedStorageOk bool
	// RemoteStorageOk returns true if remote storage may be used during placement
	RemoteStorageOk bool
	// VariableSizeStorageOk returns true if provisioned storage size may vary.
	VariableSizeStorageOk bool
}

// Layout related constants
const (
	AlgorithmStandaloneLocalUnshared   = "StandaloneLocalUnshared"
	AlgorithmStandaloneNetworkedShared = "StandaloneNetworkedShared"
)

// supportedAlgorithms contains the supported layout algorithms
var supportedAlgorithms = map[string]Algorithm{
	AlgorithmStandaloneNetworkedShared: {
		Name:            AlgorithmStandaloneNetworkedShared,
		StorageLayout:   models.StorageLayoutStandalone,
		SharedStorageOk: true,
		RemoteStorageOk: true,
	},
	AlgorithmStandaloneLocalUnshared: {
		Name:                  AlgorithmStandaloneLocalUnshared,
		StorageLayout:         models.StorageLayoutStandalone,
		VariableSizeStorageOk: true,
	},
}

// TestLayout is an unsupported layout algorithm provided for UTs
var TestLayout = &Algorithm{
	Name:          "Test",
	StorageLayout: "TestLayout",
}

// FindAlgorithm returns the layout algorithm
func FindAlgorithm(name string) (*Algorithm, error) {
	if d, found := supportedAlgorithms[name]; found {
		return &d, nil
	}
	return nil, fmt.Errorf("layout algorithm '%s' not found", name)
}
