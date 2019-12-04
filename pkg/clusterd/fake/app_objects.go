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


package fake

import (
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
)

// AppObjects fakes clusterd.AppObjects
type AppObjects struct {
	RetGCobj *models.Cluster

	RetGCDobj *models.CSPDomain
}

var _ = clusterd.AppObjects(&AppObjects{})

// GetCluster fakes its namesake
func (c *AppObjects) GetCluster() *models.Cluster {
	return c.RetGCobj
}

// GetCspDomain fakes its namesake
func (c *AppObjects) GetCspDomain() *models.CSPDomain {
	return c.RetGCDobj
}
