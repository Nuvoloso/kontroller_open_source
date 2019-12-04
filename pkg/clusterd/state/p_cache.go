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


package state

import (
	"context"
	"fmt"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
)

// GetPoolStorageType returns the type of storage obtained from the specified pool
// External serialization is required to protect the internal cache.
func (cs *ClusterState) GetPoolStorageType(ctx context.Context, spID string) (*models.CSPStorageType, error) {
	if cst, has := cs.pCache[spID]; has {
		return cst, nil
	}
	sp, err := cs.OCrud.PoolFetch(ctx, spID)
	if err != nil {
		return nil, err
	}
	for _, cst := range cs.CSP.SupportedCspStorageTypes() {
		if string(cst.Name) == sp.CspStorageType {
			cs.pCache[spID] = cst
			return cst, nil
		}
	}
	return nil, fmt.Errorf("unsupported storage type %s", sp.CspStorageType)
}
