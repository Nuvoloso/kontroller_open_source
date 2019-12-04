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
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestReleaseUnusedStorage(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	fc := &fake.Client{}

	args := &ClusterStateArgs{
		Log:     tl.Logger(),
		OCrud:   fc,
		Cluster: &models.Cluster{},
	}
	cs := NewClusterState(args)
	cs.NumLoads = 99
	ctx := context.Background()

	storage1 := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta:   &models.ObjMeta{ID: "STORAGE-1"},
			PoolID: "SP-GP2",
		},
		StorageMutable: models.StorageMutable{
			AvailableBytes:   swag.Int64(1000), // unused
			TotalParcelCount: swag.Int64(10),
			ParcelSizeBytes:  swag.Int64(100),
			StorageState: &models.StorageStateMutable{
				AttachedNodeID: "NODE-1",
			},
			ShareableStorage: true, // previously shareable
		},
	}
	storageused := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta:   &models.ObjMeta{ID: "STORAGE-1"},
			PoolID: "SP-GP2",
		},
		StorageMutable: models.StorageMutable{
			AvailableBytes:   swag.Int64(10), // unused
			TotalParcelCount: swag.Int64(10),
			ParcelSizeBytes:  swag.Int64(100),
			StorageState: &models.StorageStateMutable{
				AttachedNodeID: "NODE-1",
			},
			ShareableStorage: true, // previously shareable
		},
	}

	cs.Storage = map[string]*models.Storage{
		"STORAGE-1": &models.Storage{
			StorageAllOf0: models.StorageAllOf0{
				Meta:   &models.ObjMeta{ID: "STORAGE-1"},
				PoolID: "SP-GP2",
			},
			StorageMutable: models.StorageMutable{
				AvailableBytes:   swag.Int64(1000), // unused
				TotalParcelCount: swag.Int64(10),
				ParcelSizeBytes:  swag.Int64(100),
				StorageState: &models.StorageStateMutable{
					AttachedNodeID: "NODE-1",
				},
				ShareableStorage: true, // previously shareable
			},
		},
	}

	cs.claimedStorage = make(map[string]struct{})

	srRet := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("SR-1"),
			},
		},
	}

	// success
	assert.NotEqual(cs.NumLoads, cs.LoadCountOnInternalChange)
	cs.Storage["STORAGE-1"] = storage1
	fc.RetSRCErr = nil
	fc.RetSRCObj = srRet
	fc.InSRCArgs = &models.StorageRequest{}
	cs.OCrud = fc
	timeout := 5 * time.Second
	timeBefore := time.Now().Add(timeout)
	cs.ReleaseUnusedStorage(ctx, timeout)
	timeAfter := time.Now().Add(timeout)
	assert.Equal([]string{com.StgReqOpClose, com.StgReqOpDetach, com.StgReqOpRelease}, fc.InSRCArgs.RequestedOperations)
	assert.Equal("STORAGE-1", string(fc.InSRCArgs.StorageID))
	assert.True(timeBefore.Before(time.Time(fc.InSRCArgs.CompleteByTime)))
	assert.True(timeAfter.After(time.Time(fc.InSRCArgs.CompleteByTime)))
	assert.Equal(1, tl.CountPattern("created StorageRequest"))
	_, ok := cs.Storage["STORAGE-1"]
	assert.False(ok)
	assert.Equal(cs.NumLoads, cs.LoadCountOnInternalChange)
	tl.Flush()

	// client error
	cs.Storage["STORAGE-1"] = storage1 // reset
	fc.RetSRCErr = fmt.Errorf("client error")
	fc.RetSRCObj = nil
	cs.OCrud = fc
	cs.ReleaseUnusedStorage(ctx, timeout)
	assert.Equal(1, tl.CountPattern("failed to create StorageRequest"))
	tl.Flush()

	// storage has claims against it
	cs.Storage["STORAGE-1"] = storage1
	cs.claimedStorage["STORAGE-1"] = struct{}{}
	fc.RetSRCErr = nil
	fc.RetSRCObj = srRet
	fc.InSRCArgs = &models.StorageRequest{}
	cs.OCrud = fc
	cs.ReleaseUnusedStorage(ctx, timeout)
	assert.Empty(fc.InSRCArgs.RequestedOperations)
	assert.Equal("", string(fc.InSRCArgs.StorageID))
	assert.True(time.Time(fc.InSRCArgs.CompleteByTime).IsZero())
	assert.Equal(0, tl.CountPattern("Storage"))
	tl.Flush()

	// no unused storage
	cs.Storage["STORAGE-1"] = storageused
	cs.claimedStorage = map[string]struct{}{} // no claimed storage
	fc.RetSRCErr = nil
	fc.RetSRCObj = srRet
	fc.InSRCArgs = &models.StorageRequest{}
	cs.OCrud = fc
	cs.ReleaseUnusedStorage(ctx, timeout)
	assert.Empty(fc.InSRCArgs.RequestedOperations)
	assert.Equal("", string(fc.InSRCArgs.StorageID))
	assert.True(time.Time(fc.InSRCArgs.CompleteByTime).IsZero())
	assert.Equal(0, tl.CountPattern("Storage"))
	tl.Flush()

	// ****************** DETACHED storage cases
	cs.DetachedStorage = map[string]*models.Storage{
		"STORAGE-D": &models.Storage{
			StorageAllOf0: models.StorageAllOf0{
				Meta:   &models.ObjMeta{ID: "STORAGE-D"},
				PoolID: "SP-GP2",
			},
			StorageMutable: models.StorageMutable{
				AvailableBytes:   swag.Int64(1000), // unused
				TotalParcelCount: swag.Int64(10),
				ParcelSizeBytes:  swag.Int64(100),
				StorageState: &models.StorageStateMutable{
					AttachedNodeID:   "NODE-1",
					ProvisionedState: com.StgProvisionedStateProvisioned,
				},
				ShareableStorage: true, // previously shareable
			},
		},
	}
	cs.StorageRequests = map[string]*ClaimData{}
	cs.Storage = map[string]*models.Storage{}
	cs.LoadCountOnInternalChange = 1
	assert.NotEqual(cs.NumLoads, cs.LoadCountOnInternalChange)
	fc.RetSRCErr = nil
	fc.RetSRCObj = srRet
	fc.InSRCArgs = &models.StorageRequest{}
	cs.OCrud = fc
	timeout = 5 * time.Second
	timeBefore = time.Now().Add(timeout)
	cs.ReleaseUnusedStorage(ctx, timeout)
	timeAfter = time.Now().Add(timeout)
	assert.Equal([]string{com.StgReqOpRelease}, fc.InSRCArgs.RequestedOperations)
	assert.Equal("STORAGE-D", string(fc.InSRCArgs.StorageID))
	assert.True(timeBefore.Before(time.Time(fc.InSRCArgs.CompleteByTime)))
	assert.True(timeAfter.After(time.Time(fc.InSRCArgs.CompleteByTime)))
	assert.Equal(1, tl.CountPattern("created SR"))
	_, ok = cs.DetachedStorage["STORAGE-D"]
	assert.False(ok)
	assert.Equal(cs.NumLoads, cs.LoadCountOnInternalChange)
	tl.Flush()

	// client error
	storage1.StorageState.ProvisionedState = com.StgProvisionedStateProvisioned
	cs.DetachedStorage["STORAGE-D"] = storage1 // reset
	fc.RetSRCErr = fmt.Errorf("client error")
	fc.RetSRCObj = nil
	cs.OCrud = fc
	cs.ReleaseUnusedStorage(ctx, timeout)
	assert.Equal(1, tl.CountPattern("Release detached Storage.*failed to create StorageRequest"))
	tl.Flush()

	// no unused storage
	cs.DetachedStorage["STORAGE-D"] = storageused
	fc.RetSRCErr = nil
	fc.RetSRCObj = srRet
	fc.InSRCArgs = &models.StorageRequest{}
	cs.OCrud = fc
	cs.ReleaseUnusedStorage(ctx, timeout)
	assert.Empty(fc.InSRCArgs.RequestedOperations)
	assert.Equal("", string(fc.InSRCArgs.StorageID))
	assert.True(time.Time(fc.InSRCArgs.CompleteByTime).IsZero())
	assert.Equal(0, tl.CountPattern("Storage"))
	tl.Flush()

	// storage not in provisioning state
	storage1.StorageState.ProvisionedState = com.StgProvisionedStateProvisioning
	cs.DetachedStorage["STORAGE-D"] = storage1
	fc.RetSRCErr = nil
	fc.RetSRCObj = srRet
	fc.InSRCArgs = &models.StorageRequest{}
	cs.OCrud = fc
	cs.ReleaseUnusedStorage(ctx, timeout)
	assert.Empty(fc.InSRCArgs.RequestedOperations)
	assert.Equal("", string(fc.InSRCArgs.StorageID))
	assert.True(time.Time(fc.InSRCArgs.CompleteByTime).IsZero())
	assert.Equal(0, tl.CountPattern("Storage"))
	tl.Flush()

	// storage has active SR
	cs.StorageRequests["abc"] = &ClaimData{
		StorageRequest: &models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: models.ObjID("SomeID"),
				},
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestCreateMutable: models.StorageRequestCreateMutable{
					StorageID: models.ObjIDMutable("STORAGE-D"),
				},
			},
		},
	}
	storage1.StorageState.ProvisionedState = com.StgProvisionedStateProvisioned
	cs.DetachedStorage["STORAGE-D"] = storage1
	fc.RetSRCErr = nil
	fc.RetSRCObj = srRet
	fc.InSRCArgs = &models.StorageRequest{}
	cs.OCrud = fc
	cs.ReleaseUnusedStorage(ctx, timeout)
	assert.Empty(fc.InSRCArgs.RequestedOperations)
	assert.Equal("", string(fc.InSRCArgs.StorageID))
	assert.True(time.Time(fc.InSRCArgs.CompleteByTime).IsZero())
	assert.Equal(1, tl.CountPattern("Skipping detached Storage"))
	tl.Flush()
}
