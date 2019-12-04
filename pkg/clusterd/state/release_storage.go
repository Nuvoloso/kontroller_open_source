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
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/go-openapi/strfmt"
)

// ReleaseUnusedStorage Verifies if storage is being used and releases it back into the wild.
func (cs *ClusterState) ReleaseUnusedStorage(ctx context.Context, timeout time.Duration) {
	for id, storage := range cs.Storage {
		if !cs.storageClaimed(id) && cs.StorageCapacityNotAllocated(storage) {
			sr := &models.StorageRequest{
				StorageRequestCreateOnce: models.StorageRequestCreateOnce{
					RequestedOperations: []string{com.StgReqOpClose, com.StgReqOpDetach, com.StgReqOpRelease},
					CompleteByTime:      strfmt.DateTime(time.Now().Add(timeout)),
				},
				StorageRequestMutable: models.StorageRequestMutable{
					StorageRequestCreateMutable: models.StorageRequestCreateMutable{
						StorageID: models.ObjIDMutable(id),
					},
				},
			}
			srObj, err := cs.OCrud.StorageRequestCreate(ctx, sr)
			if err != nil {
				cs.Log.Errorf("Release Storage[%s]: failed to create StorageRequest: %s", id, err.Error())
			} else {
				cs.Log.Debugf("Release Storage[%s]: created StorageRequest [%s]", id, string(srObj.Meta.ID))
				delete(cs.Storage, id)
				cs.LoadCountOnInternalChange = cs.NumLoads
			}
		}
	}
	for id, storage := range cs.DetachedStorage {
		// Detached storage that is not actively being used will not have storage capacity allocated to a volume.
		// Storage may fall into this state when we experience a node failure during CLOSE,DETACH,RELEASE
		// Must skip storage in the process of being provisioned.
		foundInSR := false
		for _, cd := range cs.StorageRequests {
			if string(cd.StorageRequest.StorageID) == id {
				cs.Log.Debugf("Skipping detached Storage[%s] used in SR[%s]", id, cd.StorageRequest.Meta.ID)
				foundInSR = true
				break
			}
		}
		if cs.StorageCapacityIsNotAllocated(storage) && !foundInSR {
			sr := &models.StorageRequest{
				StorageRequestCreateOnce: models.StorageRequestCreateOnce{
					RequestedOperations: []string{com.StgReqOpRelease},
					CompleteByTime:      strfmt.DateTime(time.Now().Add(timeout)),
				},
				StorageRequestMutable: models.StorageRequestMutable{
					StorageRequestCreateMutable: models.StorageRequestCreateMutable{
						StorageID: models.ObjIDMutable(id),
					},
				},
			}
			srObj, err := cs.OCrud.StorageRequestCreate(ctx, sr)
			if err != nil {
				cs.Log.Errorf("Release detached Storage[%s]: failed to create StorageRequest: %s", id, err.Error())
			} else {
				cs.Log.Debugf("Release detached Storage[%s]: created SR[%s]", id, string(srObj.Meta.ID))
				delete(cs.DetachedStorage, id)
				cs.LoadCountOnInternalChange = cs.NumLoads
			}
		}
	}
}
