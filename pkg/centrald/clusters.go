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


package centrald

import (
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
)

// SupportedClusterTypes returns a list of supported cluster types
func (app *AppCtx) SupportedClusterTypes() []string {
	return cluster.SupportedClusterTypes()
}

// ValidateClusterType verifies a cluster type
func (app *AppCtx) ValidateClusterType(clusterType string) bool {
	for _, ct := range cluster.SupportedClusterTypes() {
		if ct == clusterType {
			return true
		}
	}
	return false
}

// ValidateNodeAttributes validates node attributes
func (app *AppCtx) ValidateNodeAttributes(clusterType string, nodeAttrs map[string]models.ValueType) error {
	// TBD: Determine the right attributes based on cluster type (assumed valid for now)
	for _, av := range nodeAttrs {
		if err := app.ValidateValueType(av); err != nil {
			return err
		}
	}
	return nil
}
