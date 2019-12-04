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
	"context"

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/go-openapi/swag"
)

// AppObjects fakes the agentd.AppObjects interface
type AppObjects struct {
	Node *models.Node

	Cluster *models.Cluster

	CSPDomain *models.CSPDomain

	CalledUNO int
	RetUNOErr error
}

var _ = agentd.AppObjects(&AppObjects{})

// GetNode fakes its namesake
func (ao *AppObjects) GetNode() *models.Node {
	return ao.Node
}

// GetCluster fakes its namesake
func (ao *AppObjects) GetCluster() *models.Cluster {
	return ao.Cluster
}

// GetCspDomain fakes its namesake
func (ao *AppObjects) GetCspDomain() *models.CSPDomain {
	return ao.CSPDomain
}

// UpdateNodeLocalStorage fakes its namesake
func (ao *AppObjects) UpdateNodeLocalStorage(ctx context.Context, ephemeral map[string]models.NodeStorageDevice, cacheUnitSizeBytes int64) (*models.Node, error) {
	ao.Node.LocalStorage = ephemeral
	ao.Node.CacheUnitSizeBytes = swag.Int64(cacheUnitSizeBytes)
	return ao.Node, nil
}

// UpdateNodeObj fakes its namesake
func (ao *AppObjects) UpdateNodeObj(ctx context.Context) error {
	ao.CalledUNO++
	return ao.RetUNOErr
}
