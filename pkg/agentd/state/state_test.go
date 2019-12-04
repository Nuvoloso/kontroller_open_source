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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestNewNodeState(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	fc := &fake.Client{}

	nObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "NODE-1",
			},
		},
	}

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	args := &NodeStateArgs{
		Log:   tl.Logger(),
		OCrud: fc,
		Node:  nObj,
	}

	// success
	nObj.LocalStorage = map[string]models.NodeStorageDevice{
		"uuid1": models.NodeStorageDevice{
			DeviceName:      "d1",
			DeviceState:     "UNUSED",
			DeviceType:      "HDD",
			SizeBytes:       swag.Int64(1100),
			UsableSizeBytes: swag.Int64(1000),
		},
		"uuid2": models.NodeStorageDevice{
			DeviceName:      "d2",
			DeviceState:     "CACHE",
			DeviceType:      "SSD",
			SizeBytes:       swag.Int64(2200),
			UsableSizeBytes: swag.Int64(2000),
		},
	}
	nObj.AvailableCacheBytes = swag.Int64(3000)
	nObj.TotalCacheBytes = swag.Int64(3300)

	nso := New(args)
	ns, ok := nso.(*NodeState)
	assert.True(ok)
	assert.Equal(ns, nso.NS())
	assert.Equal(fc, ns.OCrud)
	assert.Equal(args.Log, ns.Log)
	assert.Equal(nObj, ns.Node)
	assert.NotNil(ns.Node)

	// no node info
	args.Node = nil
	nso = New(args)
	assert.Nil(nso)
}

func TestDumpState(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	fc := &fake.Client{}

	expDumpString := "Iteration:0 #VS:2 Cache bytes used/available: 25B/20B\n" +
		"   VS[VS-1] 10B\n" +
		"   VS[VS-2] 15B\n"

	nObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "NODE-1",
			},
		},
	}
	args := &NodeStateArgs{
		Log:   tl.Logger(),
		OCrud: fc,
		Node:  nObj,
	}
	ns := NewNodeState(args)
	ns.CacheAvailability = &NodeCacheUsage{}
	ns.CacheAvailability.UsedSizeBytes = 25
	ns.CacheAvailability.AvailableSizeBytes = 20
	ns.VSCacheUsage = map[string]*VolumeSeriesNodeCacheUsage{
		"VS-1": &VolumeSeriesNodeCacheUsage{
			UsedSizeBytes: 10,
		},
		"VS-2": &VolumeSeriesNodeCacheUsage{
			UsedSizeBytes: 15,
		},
	}

	b := ns.DumpState()
	tl.Logger().Infof("State:\n%s", b.String())
	assert.Equal(expDumpString, b.String())
}

func TestCacheClaimAndRelease(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	fc := &fake.Client{}
	ctx := context.Background()

	nObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "NODE-1",
			},
		},
		NodeMutable: models.NodeMutable{
			CacheUnitSizeBytes: swag.Int64(1),
		},
	}
	args := &NodeStateArgs{
		Log:   tl.Logger(),
		OCrud: fc,
		Node:  nObj,
	}
	ns := NewNodeState(args)
	ns.CacheAvailability = &NodeCacheUsage{}
	ns.CacheAvailability.AvailableSizeBytes = 300
	ns.VSCacheUsage = make(map[string]*VolumeSeriesNodeCacheUsage)

	usedSizeBytes, err := ns.ClaimCache(ctx, "VS-1", 100, 10)
	assert.Nil(err)
	assert.Equal(int64(100), usedSizeBytes)
	assert.Equal(int64(100), ns.VSCacheUsage["VS-1"].UsedSizeBytes)
	assert.Equal(int64(100), ns.CacheAvailability.UsedSizeBytes)
	assert.Equal(int64(200), ns.CacheAvailability.AvailableSizeBytes)
	assert.Equal(int64(200), ns.GetNodeAvailableCacheBytes(ctx))

	// test GetClaim
	usedSizeBytes = ns.GetClaim(ctx, "VS-1")
	assert.EqualValues(100, usedSizeBytes)
	usedSizeBytes = ns.GetClaim(ctx, "VS-2")
	assert.Zero(usedSizeBytes)

	usedSizeBytes, err = ns.ClaimCache(ctx, "VS-2", 300, 10)
	assert.Nil(err)
	assert.Equal(int64(200), usedSizeBytes)
	assert.Equal(int64(200), ns.VSCacheUsage["VS-2"].UsedSizeBytes)
	assert.Equal(int64(300), ns.CacheAvailability.UsedSizeBytes)
	assert.Equal(int64(0), ns.CacheAvailability.AvailableSizeBytes)
	assert.Equal(int64(0), ns.GetNodeAvailableCacheBytes(ctx))

	ns.ReleaseCache(ctx, "VS-NON-EXISTING")
	assert.Equal(int64(300), ns.CacheAvailability.UsedSizeBytes)
	assert.Equal(int64(0), ns.CacheAvailability.AvailableSizeBytes)
	assert.Equal(int64(0), ns.GetNodeAvailableCacheBytes(ctx))

	ns.ReleaseCache(ctx, "VS-2")
	_, ok := ns.VSCacheUsage["VS-2"]
	assert.False(ok)
	assert.Equal(1, len(ns.VSCacheUsage))
	assert.Equal(int64(100), ns.CacheAvailability.UsedSizeBytes)
	assert.Equal(int64(200), ns.CacheAvailability.AvailableSizeBytes)
	assert.Equal(int64(200), ns.GetNodeAvailableCacheBytes(ctx))

	usedSizeBytes, err = ns.ClaimCache(ctx, "VS-3", 2000, 250)
	assert.NotNil(err)
	ce, ok := err.(*CacheError)
	assert.True(ok)
	assert.Equal(ErrorInsufficientCache.Message, ce.Error())
	assert.Equal(1, len(ns.VSCacheUsage))
	assert.Equal(int64(0), usedSizeBytes)
	assert.Equal(int64(100), ns.CacheAvailability.UsedSizeBytes)
	assert.Equal(int64(200), ns.CacheAvailability.AvailableSizeBytes)
	assert.Equal(int64(200), ns.GetNodeAvailableCacheBytes(ctx))

	// claim for VolumeSeries which is already using cache of desired size, no-op
	usedSizeBytes, err = ns.ClaimCache(ctx, "VS-1", 100, 10)
	assert.NoError(err)
	assert.Equal(int64(100), usedSizeBytes)
	assert.Equal(int64(200), ns.GetNodeAvailableCacheBytes(ctx))

	// increase claim for VolumeSeries which is already using cache, claim updated
	usedSizeBytes, err = ns.ClaimCache(ctx, "VS-1", 300, 10)
	assert.NoError(err)
	assert.Equal(int64(300), usedSizeBytes)
	assert.Zero(ns.GetNodeAvailableCacheBytes(ctx))

	// decrease claim for VolumeSeries which is already using cache, claim updated
	usedSizeBytes, err = ns.ClaimCache(ctx, "VS-1", 200, 10)
	assert.NoError(err)
	assert.Equal(int64(200), usedSizeBytes)
	assert.Equal(int64(100), ns.GetNodeAvailableCacheBytes(ctx))

	// decrease claim to zero for VolumeSeries which is already using cache, claim released
	usedSizeBytes, err = ns.ClaimCache(ctx, "VS-1", 0, 0)
	assert.NoError(err)
	assert.Zero(usedSizeBytes)
	assert.Equal(int64(300), ns.GetNodeAvailableCacheBytes(ctx))
	_, ok = ns.VSCacheUsage["VS-1"]
	assert.False(ok)
}

func TestCacheClaimAndReleaseWithAllocationUnitAdjustment(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	fc := &fake.Client{}
	ctx := context.Background()

	nObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "NODE-1",
			},
		},
	}
	args := &NodeStateArgs{
		Log:   tl.Logger(),
		OCrud: fc,
		Node:  nObj,
	}
	ns := NewNodeState(args)
	ns.CacheAvailability = &NodeCacheUsage{}
	initCacheSize := int64(4096*10 + 123)
	ns.CacheAvailability.AvailableSizeBytes = initCacheSize // 41083
	ns.VSCacheUsage = make(map[string]*VolumeSeriesNodeCacheUsage)

	origAllocUnitHook := allocUnitHook
	allocUnitHook = func(ns *NodeState) int64 { return 4096 }
	defer func() {
		allocUnitHook = origAllocUnitHook
	}()

	// sufficient available cache
	usedSizeBytes, err := ns.ClaimCache(ctx, "VS-1", 3072, 1000)
	assert.Nil(err)
	assert.Equal(int64(4096), usedSizeBytes)
	assert.Equal(int64(4096), ns.VSCacheUsage["VS-1"].UsedSizeBytes)
	assert.Equal(int64(4096), ns.CacheAvailability.UsedSizeBytes)
	assert.Equal(int64(initCacheSize-4096), ns.CacheAvailability.AvailableSizeBytes)
	assert.Equal(int64(initCacheSize-4096), ns.GetNodeAvailableCacheBytes(ctx))

	// no sufficient cache available, allocate what is available and satisfies minimum requirement, rounding to the cache unit size
	usedSizeBytes, err = ns.ClaimCache(ctx, "VS-2", 4096*10+1, 4096*5)
	assert.Nil(err)
	assert.Equal(int64(4096*9), usedSizeBytes)
	assert.Equal(int64(4096*9), ns.VSCacheUsage["VS-2"].UsedSizeBytes)
	assert.Equal(int64(4096*10), ns.CacheAvailability.UsedSizeBytes)
	assert.Equal(int64(123), ns.CacheAvailability.AvailableSizeBytes)
	assert.Equal(int64(123), ns.GetNodeAvailableCacheBytes(ctx))
	tl.Flush()

	ns.ReleaseCache(ctx, "VS-2")
	_, ok := ns.VSCacheUsage["VS-2"]
	assert.False(ok)
	assert.Equal(1, len(ns.VSCacheUsage))
	assert.Equal(int64(4096), ns.CacheAvailability.UsedSizeBytes)
	assert.Equal(int64(initCacheSize-4096), ns.CacheAvailability.AvailableSizeBytes)
	assert.Equal(int64(initCacheSize-4096), ns.GetNodeAvailableCacheBytes(ctx))

	usedSizeBytes, err = ns.ClaimCache(ctx, "VS-3", 4096*100, 4096*10)
	assert.NotNil(err)
	ce, ok := err.(*CacheError)
	assert.True(ok)
	assert.Equal(ErrorInsufficientCache.Message, ce.Error())
	assert.Equal(1, len(ns.VSCacheUsage))
	assert.Equal(int64(0), usedSizeBytes)
	assert.Equal(int64(4096), ns.CacheAvailability.UsedSizeBytes)
	assert.Equal(int64(initCacheSize-4096), ns.CacheAvailability.AvailableSizeBytes)
	assert.Equal(int64(initCacheSize-4096), ns.GetNodeAvailableCacheBytes(ctx))

	// new claim for VolumeSeries which is already using cache
	usedSizeBytes, err = ns.ClaimCache(ctx, "VS-1", 300, 10)
	assert.NoError(err)
	assert.Equal(int64(4096), usedSizeBytes)
	assert.Equal(int64(initCacheSize-4096), ns.GetNodeAvailableCacheBytes(ctx))
	tl.Flush()

	// another cache unit size
	ns.CacheAvailability = &NodeCacheUsage{}
	ns.CacheAvailability.AvailableSizeBytes = initCacheSize // 41083
	ns.VSCacheUsage = make(map[string]*VolumeSeriesNodeCacheUsage)
	allocUnitHook = func(ns *NodeState) int64 { return 25 }

	// sufficient available cache
	usedSizeBytes, err = ns.ClaimCache(ctx, "VS-1", 3072, 1000)
	assert.Nil(err)
	assert.Equal(int64(25*123), usedSizeBytes)
	assert.Equal(int64(25*123), ns.VSCacheUsage["VS-1"].UsedSizeBytes)
	assert.Equal(int64(25*123), ns.CacheAvailability.UsedSizeBytes)
	assert.Equal(int64(initCacheSize-25*123), ns.CacheAvailability.AvailableSizeBytes)
	assert.Equal(int64(initCacheSize-25*123), ns.GetNodeAvailableCacheBytes(ctx))

	// no sufficient cache available, allocate what is available and satisfies minimum requirement, rounding to the cache unit size
	usedSizeBytes, err = ns.ClaimCache(ctx, "VS-2", 40000, 4096*5)
	assert.Nil(err)
	assert.Equal(int64(25*1520), usedSizeBytes)
	assert.Equal(int64(25*1520), ns.VSCacheUsage["VS-2"].UsedSizeBytes)
	assert.Equal(int64(25*(1520+123)), ns.CacheAvailability.UsedSizeBytes)
	assert.Equal(int64(initCacheSize-25*(1520+123)), ns.CacheAvailability.AvailableSizeBytes)
	assert.Equal(int64(initCacheSize-25*(1520+123)), ns.GetNodeAvailableCacheBytes(ctx))
	tl.Flush()

	ns.ReleaseCache(ctx, "VS-2")
	_, ok = ns.VSCacheUsage["VS-2"]
	assert.False(ok)
	assert.Equal(1, len(ns.VSCacheUsage))
	assert.Equal(int64(25*123), ns.CacheAvailability.UsedSizeBytes)
	assert.Equal(int64(initCacheSize-25*123), ns.CacheAvailability.AvailableSizeBytes)
	assert.Equal(int64(initCacheSize-25*123), ns.GetNodeAvailableCacheBytes(ctx))

	usedSizeBytes, err = ns.ClaimCache(ctx, "VS-3", 4096*100, 4096*10)
	assert.NotNil(err)
	ce, ok = err.(*CacheError)
	assert.True(ok)
	assert.Equal(ErrorInsufficientCache.Message, ce.Error())
	assert.Equal(1, len(ns.VSCacheUsage))
	assert.Equal(int64(0), usedSizeBytes)
	assert.Equal(int64(25*123), ns.CacheAvailability.UsedSizeBytes)
	assert.Equal(int64(initCacheSize-25*123), ns.CacheAvailability.AvailableSizeBytes)
	assert.Equal(int64(initCacheSize-25*123), ns.GetNodeAvailableCacheBytes(ctx))
}

func TestReload(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	fc := &fake.Client{}
	ctx := context.Background()

	nObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "NODE-1",
			},
		},
		NodeMutable: models.NodeMutable{
			Service: &models.NuvoService{
				ServiceState: models.ServiceState{
					State: "RUNNING",
				},
			},
			Name:        "node1",
			Description: "node1 object",
			LocalStorage: map[string]models.NodeStorageDevice{
				"uuid1": models.NodeStorageDevice{
					DeviceName:      "d1",
					DeviceState:     "UNUSED",
					DeviceType:      "HDD",
					SizeBytes:       swag.Int64(1100),
					UsableSizeBytes: swag.Int64(1000),
				},
				"uuid2": models.NodeStorageDevice{
					DeviceName:      "d2",
					DeviceState:     "CACHE",
					DeviceType:      "SSD",
					SizeBytes:       swag.Int64(2200),
					UsableSizeBytes: swag.Int64(2000),
				},
			},
			AvailableCacheBytes: swag.Int64(3000),
			TotalCacheBytes:     swag.Int64(3300),
			CacheUnitSizeBytes:  swag.Int64(4096),
		},
	}
	args := &NodeStateArgs{
		Log:   tl.Logger(),
		OCrud: fc,
		Node:  nObj,
	}
	ns := NewNodeState(args)
	assert.Equal(swag.Int64Value(nObj.CacheUnitSizeBytes), ns.GetNodeCacheAllocationUnitSizeBytes())

	resVS := &volume_series.VolumeSeriesListOK{
		Payload: []*models.VolumeSeries{
			&models.VolumeSeries{
				VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
					Meta: &models.ObjMeta{
						ID:      "VS-1",
						Version: 1,
					},
					RootParcelUUID: "uuid1",
				},
				VolumeSeriesMutable: models.VolumeSeriesMutable{
					VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
						Name: "MyVolSeries",
					},
					VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
						ConfiguredNodeID: "NODE-1",
						CacheAllocations: map[string]models.CacheAllocation{
							"NODE-1": {
								AllocatedSizeBytes: swag.Int64(1),
								RequestedSizeBytes: swag.Int64(2),
							},
							"NODE-2": {
								AllocatedSizeBytes: swag.Int64(22),
								RequestedSizeBytes: swag.Int64(22),
							},
							"NODE-3": {
								AllocatedSizeBytes: swag.Int64(333),
								RequestedSizeBytes: swag.Int64(444),
							},
						},
					},
				},
			},
			&models.VolumeSeries{
				VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
					Meta: &models.ObjMeta{
						ID:      "VS-2",
						Version: 1,
					},
					RootParcelUUID: "uuid2",
				},
				VolumeSeriesMutable: models.VolumeSeriesMutable{
					VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
						Name: "OtherVolSeries",
					},
					VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
						ConfiguredNodeID: "NODE-1",
						CacheAllocations: map[string]models.CacheAllocation{
							"NODE-1": {AllocatedSizeBytes: swag.Int64(444)},
							"NODE-2": {AllocatedSizeBytes: swag.Int64(55)},
							"NODE-3": {AllocatedSizeBytes: swag.Int64(6)},
						},
					},
				},
			},
			&models.VolumeSeries{
				VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
					Meta: &models.ObjMeta{
						ID:      "VS-3",
						Version: 1,
					},
					RootParcelUUID: "uuid3",
				},
				VolumeSeriesMutable: models.VolumeSeriesMutable{
					VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
						Name: "SomeOtherVolSeries",
					},
					VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
						ConfiguredNodeID: "NODE-1",
						CacheAllocations: map[string]models.CacheAllocation{
							"NODE-1": {AllocatedSizeBytes: swag.Int64(0)},
						},
					},
				},
			},
		},
	}
	// success
	fc.RetLsVOk = resVS
	fc.RetLsVErr = nil
	chg, err := ns.Reload(ctx)
	assert.NoError(err)
	assert.True(chg)
	assert.Equal(int64(445), ns.CacheAvailability.UsedSizeBytes)
	assert.Equal(int64(2855), ns.CacheAvailability.AvailableSizeBytes)
	assert.Equal(int64(1), ns.VSCacheUsage["VS-1"].UsedSizeBytes)
	assert.Equal(int64(444), ns.VSCacheUsage["VS-2"].UsedSizeBytes)
	assert.Equal(2, len(ns.VSCacheUsage))

	// failure to list volume series
	fc.RetLsVOk = nil
	fc.RetLsVErr = fmt.Errorf("vs-list-error")
	chg, err = ns.Reload(ctx)
	assert.Error(err)
	assert.False(chg)
	assert.Regexp("vs-list-error", err)
}

func TestStateChanged(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	nObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "NODE-1",
			},
		},
		NodeMutable: models.NodeMutable{
			Service: &models.NuvoService{
				ServiceState: models.ServiceState{
					State: "RUNNING",
				},
			},
			Name:        "node1",
			Description: "node1 object",
			LocalStorage: map[string]models.NodeStorageDevice{
				"uuid1": models.NodeStorageDevice{
					DeviceName:      "d1",
					DeviceState:     "UNUSED",
					DeviceType:      "HDD",
					SizeBytes:       swag.Int64(1100),
					UsableSizeBytes: swag.Int64(1000),
				},
				"uuid2": models.NodeStorageDevice{
					DeviceName:      "d2",
					DeviceState:     "CACHE",
					DeviceType:      "SSD",
					SizeBytes:       swag.Int64(2200),
					UsableSizeBytes: swag.Int64(2000),
				},
			},
			AvailableCacheBytes: swag.Int64(3000),
			TotalCacheBytes:     swag.Int64(3300),
		},
	}
	args := &NodeStateArgs{
		Log:  tl.Logger(),
		Node: nObj,
	}

	// no change
	oldNS := NewNodeState(args)
	oldNS.CacheAvailability.UsedSizeBytes = 25
	oldNS.CacheAvailability.AvailableSizeBytes = 20
	oldNS.VSCacheUsage = map[string]*VolumeSeriesNodeCacheUsage{
		"VS-1": &VolumeSeriesNodeCacheUsage{
			UsedSizeBytes: 10,
		},
		"VS-2": &VolumeSeriesNodeCacheUsage{
			UsedSizeBytes: 15,
		},
	}
	newNS := NewNodeState(args)
	testutils.Clone(oldNS, &newNS)
	assert.False(oldNS.stateChanged(newNS))

	// change: different node cache availability
	newNS.CacheAvailability.UsedSizeBytes *= 10
	assert.True(oldNS.stateChanged(newNS))

	// change: different volume series node cache usage, same volume series
	newNS.CacheAvailability.UsedSizeBytes /= 10
	oldNS.VSCacheUsage = map[string]*VolumeSeriesNodeCacheUsage{
		"VS-1": &VolumeSeriesNodeCacheUsage{
			UsedSizeBytes: 20,
		},
		"VS-2": &VolumeSeriesNodeCacheUsage{
			UsedSizeBytes: 25,
		},
	}
	assert.True(oldNS.stateChanged(newNS))

	// change: different volume series using cache
	oldNS.VSCacheUsage = map[string]*VolumeSeriesNodeCacheUsage{
		"VS-3": &VolumeSeriesNodeCacheUsage{
			UsedSizeBytes: 555,
		},
		"VS-2": &VolumeSeriesNodeCacheUsage{
			UsedSizeBytes: 25,
		},
	}
	newNS.VSCacheUsage = map[string]*VolumeSeriesNodeCacheUsage{
		"VS-1": &VolumeSeriesNodeCacheUsage{
			UsedSizeBytes: 20,
		},
		"VS-2": &VolumeSeriesNodeCacheUsage{
			UsedSizeBytes: 25,
		},
	}
	assert.True(oldNS.stateChanged(newNS))
}

func TestNodeObj(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	tl.Flush()

	ctx := context.Background()

	nObj1 := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "NODE-1",
			},
		},
		NodeMutable: models.NodeMutable{
			Service: &models.NuvoService{
				ServiceState: models.ServiceState{
					State: "RUNNING",
				},
			},
			Name:        "node1",
			Description: "node1 object",
			LocalStorage: map[string]models.NodeStorageDevice{
				"uuid1": models.NodeStorageDevice{
					DeviceName:      "d1",
					DeviceState:     "UNUSED",
					DeviceType:      "HDD",
					SizeBytes:       swag.Int64(1100),
					UsableSizeBytes: swag.Int64(1000),
				},
			},
			AvailableCacheBytes: swag.Int64(3000),
			TotalCacheBytes:     swag.Int64(3300),
		},
	}
	nObj2 := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "NODE-2",
			},
		},
		NodeMutable: models.NodeMutable{
			Service: &models.NuvoService{
				ServiceState: models.ServiceState{
					State: "RUNNING",
				},
			},
			Name:        "node2",
			Description: "node2 object",
			LocalStorage: map[string]models.NodeStorageDevice{
				"uuid1": models.NodeStorageDevice{
					DeviceName:      "d2",
					DeviceState:     "CACHE",
					DeviceType:      "SSD",
					SizeBytes:       swag.Int64(2200),
					UsableSizeBytes: swag.Int64(2000),
				},
			},
			AvailableCacheBytes: swag.Int64(1000),
			TotalCacheBytes:     swag.Int64(2000),
		},
	}
	args := &NodeStateArgs{
		Log:  tl.Logger(),
		Node: nObj1,
	}

	oldNS := NewNodeState(args)
	ns := oldNS.NS()
	ns.UpdateNodeObj(ctx, nObj2)
	assert.Equal(ns.NodeStateArgs.Node, nObj2)
}

func TestRoundupSizeBytesToBlock(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	tl.Flush()

	assert.Equal(int64(4096*2), RoundSizeBytesToBlock(4099, 4096, false))
	assert.Equal(int64(4096), RoundSizeBytesToBlock(4096, 4096, false))
	assert.Equal(int64(4096), RoundSizeBytesToBlock(4095, 4096, false))
	assert.Equal(int64(4096), RoundSizeBytesToBlock(4095, 2, false))
	assert.Equal(int64(4095), RoundSizeBytesToBlock(4095, 1, false))
	assert.Equal(int64(1), RoundSizeBytesToBlock(1, 1, false))
	assert.Equal(int64(0), RoundSizeBytesToBlock(0, 0, false))

	assert.Equal(int64(4096), RoundSizeBytesToBlock(4099, 4096, true))
	assert.Equal(int64(4096), RoundSizeBytesToBlock(4096, 4096, true))
	assert.Equal(int64(0), RoundSizeBytesToBlock(4095, 4096, true))
	assert.Equal(int64(4094), RoundSizeBytesToBlock(4095, 2, true))
	assert.Equal(int64(4095), RoundSizeBytesToBlock(4095, 1, true))
	assert.Equal(int64(1), RoundSizeBytesToBlock(1, 1, true))
	assert.Equal(int64(0), RoundSizeBytesToBlock(0, 0, true))
}

func TestUpdateNodeAvailableCache(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	tl.Flush()

	fc := &fake.Client{}
	ctx := context.Background()

	nObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "NODE-1",
			},
		},
		NodeMutable: models.NodeMutable{
			CacheUnitSizeBytes: swag.Int64(1),
		},
	}
	args := &NodeStateArgs{
		Log:   tl.Logger(),
		OCrud: fc,
		Node:  nObj,
	}
	ns := NewNodeState(args)
	ns.CacheAvailability = &NodeCacheUsage{}
	ns.CacheAvailability.AvailableSizeBytes = 10
	ns.VSCacheUsage = make(map[string]*VolumeSeriesNodeCacheUsage)

	fc.RetNodeUpdateObj = nObj
	err := ns.UpdateNodeAvailableCache(ctx)
	assert.Nil(err)
	assert.Equal(int64(10), swag.Int64Value(ns.NodeStateArgs.Node.AvailableCacheBytes))
	assert.Equal(int64(10), ns.GetNodeAvailableCacheBytes(ctx))

	err = ns.UpdateNodeAvailableCache(ctx)
	assert.Nil(err)
	assert.Equal(int64(10), swag.Int64Value(ns.NodeStateArgs.Node.AvailableCacheBytes)) // Node object is updated

	// failure to update Node
	ns.CacheAvailability.AvailableSizeBytes = 30
	fc.RetNodeUpdateErr = fmt.Errorf("node-update-error")
	err = ns.UpdateNodeAvailableCache(ctx)
	assert.NotNil(err)
	assert.Regexp("node-update-error", err)
	assert.Equal(int64(10), swag.Int64Value(ns.NodeStateArgs.Node.AvailableCacheBytes)) // Node object is not updated
}
