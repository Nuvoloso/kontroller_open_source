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

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/layout"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/alecthomas/units"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestSelectStandaloneNetworkSharedNewStorageVariations(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	fc := &fake.Client{}
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "CLUSTER-1",
			},
		},
	}
	args := &ClusterStateArgs{
		Log:     tl.Logger(),
		OCrud:   fc,
		Cluster: clObj,
	}
	cs := NewClusterState(args)
	ctx := context.Background()

	psb := int64(units.Gibibyte)
	asb := int64(500 * units.Gibibyte)
	cst := &models.CSPStorageType{
		Name:                         "Amazon gp2",
		PreferredAllocationSizeBytes: swag.Int64(asb),
		ParcelSizeBytes:              swag.Int64(psb),
	}
	cs.pCache["SP-1"] = cst // test with cached CST

	// variants of storage plan element computations
	itemTCs := []struct {
		SizeBytes int64
		ExpItems  []StorageItem
	}{
		{
			SizeBytes: asb - psb,
			ExpItems: []StorageItem{
				{SizeBytes: asb - psb, ShareableStorage: true, NumParcels: 499, MinSizeBytes: asb, RemainingSizeBytes: 0},
			},
		},
		{
			SizeBytes: 2 * asb,
			ExpItems: []StorageItem{
				{SizeBytes: asb - psb, ShareableStorage: true, NumParcels: 499, MinSizeBytes: asb, RemainingSizeBytes: 0},
				{SizeBytes: asb - psb, ShareableStorage: true, NumParcels: 499, MinSizeBytes: asb, RemainingSizeBytes: 0},
				{SizeBytes: 2 * psb, ShareableStorage: true, NumParcels: 2, MinSizeBytes: asb, RemainingSizeBytes: int64((499 - 2) * psb)},
			},
		},
	}
	for _, tc := range itemTCs {
		spe := &models.StoragePlanStorageElement{
			SizeBytes: swag.Int64(tc.SizeBytes),
			PoolID:    "SP-1",
			StorageParcels: map[string]models.StorageParcelElement{
				"STORAGE": models.StorageParcelElement{
					SizeBytes: swag.Int64(0 * asb), // actually ignored
				},
			},
		}
		td, err := cs.getStorageElementTypeData(ctx, spe)
		assert.NoError(err)
		ss := &standaloneSelector{}
		ss.CS = cs
		ss.stateMachine = []standaloneIterState{saIterProvisionLocalFixedSizeStorage}
		es := &ElementStorage{}
		err = ss.FillStorageElement(ctx, 0, spe, td, es)
		assert.NoError(err)
		for i := range tc.ExpItems {
			tc.ExpItems[i].PoolID = "SP-1"
		}
		assert.Equal(tc.ExpItems, es.Items)
		tl.Flush()
	}
}

func TestSelectStandaloneNetworkSharedInvocationWithEmptyCache(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "CLUSTER-1",
			},
		},
	}
	la, err := layout.FindAlgorithm(layout.AlgorithmStandaloneNetworkedShared)
	assert.NoError(err)
	assert.NotNil(la)
	args := &ClusterStateArgs{
		Log:              tl.Logger(),
		Cluster:          clObj,
		LayoutAlgorithms: []*layout.Algorithm{la},
	}
	cs := NewClusterState(args)
	ctx := context.Background()

	psb := int64(units.Gibibyte)
	asb := int64(500 * units.Gibibyte)
	cst := &models.CSPStorageType{
		Name:                         "Amazon gp2",
		PreferredAllocationSizeBytes: swag.Int64(asb),
		ParcelSizeBytes:              swag.Int64(psb),
	}

	vsObj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID:      "VS-1",
				Version: 1,
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				VolumeSeriesState: "NEW",
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				SizeBytes: swag.Int64(int64(1 * units.GiB)),
			},
		},
	}
	vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VSR-1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			ClusterID: "CLUSTER-1",
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "PLACEMENT",
				StoragePlan: &models.StoragePlan{
					StorageLayout: models.StorageLayoutStandalone,
					StorageElements: []*models.StoragePlanStorageElement{
						&models.StoragePlanStorageElement{
							SizeBytes: swag.Int64(asb - psb),
							PoolID:    "SP-1",
							StorageParcels: map[string]models.StorageParcelElement{
								"STORAGE": models.StorageParcelElement{
									SizeBytes: swag.Int64(0 * asb), // actually ignored
								},
							},
						},
					},
				},
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID:         "NODE-1",
				VolumeSeriesID: "VS-1",
			},
		},
	}
	vs := vsClone(vsObj)
	vsr := vsrClone(vsrObj)
	vsrNotStandalone := vsrClone(vsrObj)
	vsrNotStandalone.StoragePlan.StorageLayout = models.StorageLayout("not" + models.StorageLayoutStandalone)

	// argument validation (internal invocation)
	argTCs := []struct {
		Args *SelectStorageArgs
		CS   *ClusterState
		Resp *SelectStorageResponse
	}{
		{nil, cs, &SelectStorageResponse{Elements: []ElementStorage{}}},
		{&SelectStorageArgs{}, cs, &SelectStorageResponse{Elements: []ElementStorage{}}},
		{&SelectStorageArgs{VS: vs}, cs, &SelectStorageResponse{Elements: []ElementStorage{}}},
		{&SelectStorageArgs{VSR: vsr}, cs, &SelectStorageResponse{Elements: []ElementStorage{}}},
		{&SelectStorageArgs{VS: vs, VSR: vsr}, nil, &SelectStorageResponse{Elements: []ElementStorage{}}},
		{&SelectStorageArgs{VS: vs, VSR: vsr}, cs, &SelectStorageResponse{}},
		{&SelectStorageArgs{VS: vs, VSR: vsr}, cs, nil},
		{&SelectStorageArgs{VS: vs, VSR: vsr}, cs, &SelectStorageResponse{Elements: []ElementStorage{}}},
		{&SelectStorageArgs{VS: vs, VSR: vsrNotStandalone}, cs, &SelectStorageResponse{Elements: []ElementStorage{}, LA: la}},
	}
	for i, tc := range argTCs {
		tl.Logger().Infof("Case: %d", i)
		ss, err := newStandaloneStorageSelector(tc.Args, tc.CS, tc.Resp)
		assert.Error(err)
		assert.Nil(ss)
	}
	tl.Flush()

	ssa := &SelectStorageArgs{
		VSR: vsr,
		VS:  vs,
	}

	// test construction of the state machine
	ss, err := newStandaloneStorageSelector(ssa, cs, &SelectStorageResponse{Elements: []ElementStorage{}, LA: la})
	assert.NoError(err)
	assert.NotNil(ss)
	ssSA, ok := ss.(*standaloneSelector)
	assert.True(ok)
	assert.NotEmpty(ssSA.stateMachine)
	expSM := []standaloneIterState{
		saIterFindLocalShareableStorage,
		saIterServeLocalShareableStorage,
		saIterFindLocalClaims,
		saIterServeLocalClaims,
		saIterFindRemoteShareableStorage,
		saIterServeRemoteShareableStorage,
		saIterFindRemoteClaims,
		saIterServeRemoteClaims,
		saIterProvisionLocalFixedSizeStorage,
	}
	assert.Equal(expSM, ssSA.stateMachine)

	// test boundary condition of the state machine
	ssSA.stateMachine = []standaloneIterState{}
	assert.Panics(func() { ssSA.nextStorageItem(1) })
	tl.Flush()

	// cache the pool storage type
	cs.pCache["SP-1"] = cst

	// 1 element, 1 item (no storage allocated)
	tl.Logger().Infof("Case: 1 element")
	assert.Empty(cs.Storage)
	resp, err := cs.SelectStorage(ctx, ssa)
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Len(resp.Elements, 1)
	tl.Logger().Debugf("[0] %v", resp.Elements[0])
	assert.Len(resp.Elements[0].Items, 1)
	tl.Logger().Debugf("[0] Items %v", resp.Elements[0].Items)
	expItems := []StorageItem{
		{SizeBytes: asb - psb, ShareableStorage: true, NumParcels: 499, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-1"},
	}
	assert.Equal(expItems, resp.Elements[0].Items)
	tl.Flush()

	// 2 elements, 1 item each (no storage allocated)
	tl.Logger().Infof("Case: 2 elements")
	assert.Empty(cs.Storage)
	vsr = vsrClone(vsrObj)
	vsr.StoragePlan.StorageElements = append(vsr.StoragePlan.StorageElements,
		&models.StoragePlanStorageElement{
			SizeBytes: swag.Int64(asb - 2*psb),
			PoolID:    "SP-1",
			StorageParcels: map[string]models.StorageParcelElement{
				"STORAGE": models.StorageParcelElement{
					SizeBytes: swag.Int64(0 * asb), // actually ignored
				},
			},
		})
	assert.Len(vsr.StoragePlan.StorageElements, 2)
	ssa.VSR = vsr
	resp, err = cs.SelectStorage(ctx, ssa)
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Len(resp.Elements, 2)
	tl.Logger().Debugf("[0] %v", resp.Elements[0])
	assert.Len(resp.Elements[0].Items, 1)
	tl.Logger().Debugf("[0] Items %v", resp.Elements[0].Items)
	expItems0 := []StorageItem{
		{SizeBytes: asb - psb, ShareableStorage: true, NumParcels: 499, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-1"},
	}
	assert.Equal(expItems0, resp.Elements[0].Items)
	tl.Logger().Debugf("[1] %v", resp.Elements[1])
	assert.Len(resp.Elements[1].Items, 1)
	tl.Logger().Debugf("[1] Items %v", resp.Elements[1].Items)
	expItems1 := []StorageItem{
		{SizeBytes: asb - 2*psb, ShareableStorage: true, NumParcels: 498, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: int64(1 * psb), NodeID: "NODE-1"},
	}
	assert.Equal(expItems1, resp.Elements[1].Items)
	tl.Flush()
}

func TestSelectStandaloneNetworkSharedLocalSharedStorage(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	fc := &fake.Client{}
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "CLUSTER-1",
			},
		},
	}
	la, err := layout.FindAlgorithm(layout.AlgorithmStandaloneNetworkedShared)
	assert.NoError(err)
	assert.NotNil(la)
	args := &ClusterStateArgs{
		Log:              tl.Logger(),
		OCrud:            fc,
		Cluster:          clObj,
		LayoutAlgorithms: []*layout.Algorithm{la},
	}
	cs := NewClusterState(args)
	ctx := context.Background()

	psb := int64(units.Gibibyte)
	asb := int64(500 * units.Gibibyte)
	cst := &models.CSPStorageType{
		Name:                         "Amazon gp2",
		PreferredAllocationSizeBytes: swag.Int64(asb),
		ParcelSizeBytes:              swag.Int64(psb),
	}
	cs.pCache["SP-1"] = cst // test with cached CST

	availBytes1 := int64(20) * psb // 20 parcels
	sObj1 := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta:           &models.ObjMeta{ID: "STORAGE-1", Version: 2},
			CspStorageType: "Amazon gp2",
			PoolID:         "SP-1",
			SizeBytes:      swag.Int64(asb),
		},
		StorageMutable: models.StorageMutable{
			AvailableBytes:   swag.Int64(availBytes1),
			ParcelSizeBytes:  swag.Int64(psb),
			TotalParcelCount: swag.Int64(499),
			ShareableStorage: true,
			StorageState: &models.StorageStateMutable{
				AttachedNodeID:     "NODE-1",
				AttachedNodeDevice: "/dev/xvdba",
				DeviceState:        "OPEN",
				MediaState:         "FORMATTED",
				AttachmentState:    "ATTACHED",
			},
		},
	}
	cs.Storage["STORAGE-1"] = sObj1

	nObj1 := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{ID: "NODE-1", Version: 2},
		},
		NodeMutable: models.NodeMutable{
			Name: "Node1",
			Service: &models.NuvoService{
				ServiceState: models.ServiceState{
					State: "READY",
				},
			},
		},
	}
	cs.Nodes["NODE-1"] = &NodeData{
		Node:            nObj1,
		AttachedStorage: []string{"STORAGE-1"},
	}

	vsObj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID:      "VS-1",
				Version: 1,
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				VolumeSeriesState: "NEW",
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				SizeBytes: swag.Int64(int64(1 * units.GiB)),
			},
		},
	}
	vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VSR-1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			ClusterID: "CLUSTER-1",
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "PLACEMENT",
				StoragePlan: &models.StoragePlan{
					StorageLayout: models.StorageLayoutStandalone,
					StorageElements: []*models.StoragePlanStorageElement{
						&models.StoragePlanStorageElement{
							PoolID: "SP-1",
							StorageParcels: map[string]models.StorageParcelElement{
								"STORAGE": models.StorageParcelElement{
									SizeBytes: swag.Int64(0 * asb), // actually ignored
								},
							},
						},
					},
				},
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID:         "NODE-1",
				VolumeSeriesID: "VS-1",
			},
		},
	}
	vs := vsClone(vsObj)
	vsr := vsrClone(vsrObj)
	ssa := &SelectStorageArgs{
		VSR: vsr,
		VS:  vs,
	}

	// Existing storage has sufficient space
	tl.Logger().Infof("Case: existing storage has sufficient space")
	assert.NotEmpty(cs.Storage)
	assert.Equal([]*models.Storage{sObj1}, cs.FindShareableStorageByPool("SP-1", availBytes1, "NODE-1", false))
	vsr.StoragePlan.StorageElements[0].SizeBytes = swag.Int64(availBytes1)
	resp, err := cs.SelectStorage(ctx, ssa)
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Len(resp.Elements, 1)
	tl.Logger().Debugf("[0] %v", resp.Elements[0])
	assert.Len(resp.Elements[0].Items, 1)
	tl.Logger().Debugf("[0] Items %v", resp.Elements[0].Items)
	expItems := []StorageItem{
		{SizeBytes: availBytes1, ShareableStorage: true, NumParcels: 20, Storage: sObj1, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-1"},
	}
	assert.Equal(expItems, resp.Elements[0].Items)
	tl.Flush()

	// Existing storage partially supplies the space
	tl.Logger().Infof("Case: existing storage partially supplies space")
	assert.NotEmpty(cs.Storage)
	assert.Equal([]*models.Storage{sObj1}, cs.FindShareableStorageByPool("SP-1", availBytes1, "NODE-1", false))
	vsr.StoragePlan.StorageElements[0].SizeBytes = swag.Int64(availBytes1 + 10*psb)
	resp, err = cs.SelectStorage(ctx, ssa)
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Len(resp.Elements, 1)
	tl.Logger().Debugf("[0] %v", resp.Elements[0])
	assert.Len(resp.Elements[0].Items, 2)
	tl.Logger().Debugf("[0] Items %v", resp.Elements[0].Items)
	expItems = []StorageItem{
		{SizeBytes: availBytes1, ShareableStorage: true, NumParcels: 20, Storage: sObj1, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-1"},
		{SizeBytes: int64(10) * psb, ShareableStorage: true, NumParcels: 10, Storage: nil, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: asb - int64(11)*psb, NodeID: "NODE-1"},
	}
	assert.Equal(expItems, resp.Elements[0].Items)
	tl.Flush()

	// Existing storage has insufficient space
	tl.Logger().Infof("Case: existing storage has insufficient space")
	sObj1.AvailableBytes = swag.Int64(psb - int64(1))
	assert.NotEmpty(cs.Storage)
	assert.Equal([]*models.Storage{}, cs.FindShareableStorageByPool("SP-1", availBytes1, "NODE-1", false))
	vsr.StoragePlan.StorageElements[0].SizeBytes = swag.Int64(availBytes1)
	resp, err = cs.SelectStorage(ctx, ssa)
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Len(resp.Elements, 1)
	tl.Logger().Debugf("[0] %v", resp.Elements[0])
	assert.Len(resp.Elements[0].Items, 1)
	tl.Logger().Debugf("[0] Items %v", resp.Elements[0].Items)
	expItems = []StorageItem{
		{SizeBytes: int64(20) * psb, ShareableStorage: true, NumParcels: 20, Storage: nil, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: asb - int64(21)*psb, NodeID: "NODE-1"},
	}
	assert.Equal(expItems, resp.Elements[0].Items)
	tl.Flush()

	// Multiple storage objects
	sObj1.AvailableBytes = swag.Int64(availBytes1)
	sObj2 := sClone(sObj1)
	sObj2.AvailableBytes = swag.Int64(availBytes1 - psb)
	sObj2.Meta.ID = "STORAGE-2"
	cs.Storage["STORAGE-2"] = sObj2
	sObj3 := sClone(sObj1)
	sObj3.AvailableBytes = swag.Int64(availBytes1 + psb)
	sObj3.Meta.ID = "STORAGE-3"
	cs.Storage["STORAGE-3"] = sObj3
	// validate sorting order
	tl.Logger().Infof("Case: multiple storage sorting order")
	ssIf, err := newStandaloneStorageSelector(&SelectStorageArgs{VS: vs, VSR: vsr}, cs, &SelectStorageResponse{Elements: []ElementStorage{}, LA: la})
	assert.NoError(err)
	assert.NotNil(ssIf)
	ss, ok := ssIf.(*standaloneSelector)
	assert.True(ok)
	td, err := cs.getStorageElementTypeData(ctx, vsr.StoragePlan.StorageElements[0])
	ss.TD = td
	assert.Equal([]*models.Storage{sObj3, sObj1, sObj2}, ss.findLocalShareableStorageByPoolOrdered())

	// select with multiple
	tl.Logger().Infof("Case: select with multiple storage available")
	vsr.StoragePlan.StorageElements[0].SizeBytes = swag.Int64(int64(4) * availBytes1)
	resp, err = cs.SelectStorage(ctx, ssa)
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Len(resp.Elements, 1)
	tl.Logger().Debugf("[0] %v", resp.Elements[0])
	assert.Len(resp.Elements[0].Items, 4)
	tl.Logger().Debugf("[0] Items %v", resp.Elements[0].Items)
	expItems = []StorageItem{
		{SizeBytes: availBytes1 + psb, ShareableStorage: true, NumParcels: 21, Storage: sObj3, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-1"},
		{SizeBytes: availBytes1, ShareableStorage: true, NumParcels: 20, Storage: sObj1, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-1"},
		{SizeBytes: availBytes1 - psb, ShareableStorage: true, NumParcels: 19, Storage: sObj2, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-1"},
		{SizeBytes: int64(20) * psb, ShareableStorage: true, NumParcels: 20, Storage: nil, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: asb - int64(21)*psb, NodeID: "NODE-1"},
	}
	assert.Equal(expItems, resp.Elements[0].Items)
	tl.Flush()
}

func TestSelectStandaloneNetworkSharedLocalSharedClaims(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	fc := &fake.Client{}
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "CLUSTER-1",
			},
		},
	}
	la, err := layout.FindAlgorithm(layout.AlgorithmStandaloneNetworkedShared)
	assert.NoError(err)
	assert.NotNil(la)
	args := &ClusterStateArgs{
		Log:              tl.Logger(),
		OCrud:            fc,
		Cluster:          clObj,
		LayoutAlgorithms: []*layout.Algorithm{la},
	}
	cs := NewClusterState(args)
	ctx := context.Background()

	psb := int64(units.Gibibyte)
	asb := int64(500 * units.Gibibyte)
	cst := &models.CSPStorageType{
		Name:                         "Amazon gp2",
		PreferredAllocationSizeBytes: swag.Int64(asb),
		ParcelSizeBytes:              swag.Int64(psb),
	}
	cs.pCache["SP-1"] = cst // test with cached CST

	availBytes1 := int64(20) * psb // 20 parcels
	cdSR1 := &ClaimData{
		StorageRequest: &models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{ID: "SR-1", Version: 2},
			},
		},
		PoolID:         "SP-1",
		NodeID:         "NODE-1",
		RemainingBytes: availBytes1,
		VSRClaims: []VSRClaim{
			{RequestID: "OTHER-VSR", SizeBytes: asb - availBytes1, Annotation: "OTHER-VSR-0"},
		},
	}
	cs.StorageRequests["SR-1"] = cdSR1

	nObj1 := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{ID: "NODE-1", Version: 2},
		},
		NodeMutable: models.NodeMutable{
			Name: "Node1",
			Service: &models.NuvoService{
				ServiceState: models.ServiceState{
					State: "READY",
				},
			},
		},
	}
	cs.Nodes["NODE-1"] = &NodeData{
		Node: nObj1,
	}

	vsObj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID:      "VS-1",
				Version: 1,
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				VolumeSeriesState: "NEW",
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				SizeBytes: swag.Int64(int64(1 * units.GiB)),
			},
		},
	}
	vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VSR-1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			ClusterID: "CLUSTER-1",
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "PLACEMENT",
				StoragePlan: &models.StoragePlan{
					StorageLayout: models.StorageLayoutStandalone,
					StorageElements: []*models.StoragePlanStorageElement{
						&models.StoragePlanStorageElement{
							PoolID: "SP-1",
							StorageParcels: map[string]models.StorageParcelElement{
								"STORAGE": models.StorageParcelElement{
									SizeBytes: swag.Int64(0 * asb), // actually ignored
								},
							},
						},
					},
				},
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID:         "NODE-1",
				VolumeSeriesID: "VS-1",
			},
		},
	}
	vs := vsClone(vsObj)
	vsr := vsrClone(vsrObj)
	ssa := &SelectStorageArgs{
		VSR: vsr,
		VS:  vs,
	}

	// Existing claim has sufficient space
	tl.Logger().Infof("Case: existing claim has sufficient space")
	assert.NotEmpty(cs.StorageRequests)
	assert.Equal(ClaimList{cdSR1}, cs.FindClaimsByPool("SP-1", availBytes1, "NODE-1", false))
	vsr.StoragePlan.StorageElements[0].SizeBytes = swag.Int64(availBytes1)
	resp, err := cs.SelectStorage(ctx, ssa)
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Len(resp.Elements, 1)
	tl.Logger().Debugf("[0] %v", resp.Elements[0])
	assert.Len(resp.Elements[0].Items, 1)
	tl.Logger().Debugf("[0] Items %v", resp.Elements[0].Items)
	expItems := []StorageItem{
		{SizeBytes: availBytes1, ShareableStorage: true, NumParcels: 20, StorageRequestID: "SR-1", PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-1"},
	}
	assert.Equal(expItems, resp.Elements[0].Items)
	tl.Flush()

	// Existing claim partially supplies the space
	tl.Logger().Infof("Case: existing claim partially supplies space")
	assert.NotEmpty(cs.StorageRequests)
	assert.Equal(ClaimList{cdSR1}, cs.FindClaimsByPool("SP-1", availBytes1, "NODE-1", false))
	vsr.StoragePlan.StorageElements[0].SizeBytes = swag.Int64(availBytes1 + 10*psb)
	resp, err = cs.SelectStorage(ctx, ssa)
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Len(resp.Elements, 1)
	tl.Logger().Debugf("[0] %v", resp.Elements[0])
	assert.Len(resp.Elements[0].Items, 2)
	tl.Logger().Debugf("[0] Items %v", resp.Elements[0].Items)
	expItems = []StorageItem{
		{SizeBytes: availBytes1, ShareableStorage: true, NumParcels: 20, StorageRequestID: "SR-1", PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-1"},
		{SizeBytes: int64(10) * psb, ShareableStorage: true, NumParcels: 10, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: asb - int64(11)*psb, NodeID: "NODE-1"},
	}
	assert.Equal(expItems, resp.Elements[0].Items)
	tl.Flush()

	// Existing claim has insufficient space
	tl.Logger().Infof("Case: existing claim has insufficient space")
	cdSR1.RemainingBytes = psb - int64(1)
	assert.NotEmpty(cs.StorageRequests)
	assert.Equal(ClaimList{}, cs.FindClaimsByPool("SP-1", availBytes1, "NODE-1", false))
	vsr.StoragePlan.StorageElements[0].SizeBytes = swag.Int64(availBytes1)
	resp, err = cs.SelectStorage(ctx, ssa)
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Len(resp.Elements, 1)
	tl.Logger().Debugf("[0] %v", resp.Elements[0])
	assert.Len(resp.Elements[0].Items, 1)
	tl.Logger().Debugf("[0] Items %v", resp.Elements[0].Items)
	expItems = []StorageItem{
		{SizeBytes: int64(20) * psb, ShareableStorage: true, NumParcels: 20, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: asb - int64(21)*psb, NodeID: "NODE-1"},
	}
	assert.Equal(expItems, resp.Elements[0].Items)
	tl.Flush()

	// Multiple claims
	cdSR1.RemainingBytes = availBytes1
	cdSR2 := claimClone(cdSR1)
	cdSR2.StorageRequest.Meta.ID = "SR-2"
	cdSR2.RemainingBytes = availBytes1 - psb
	cs.StorageRequests["SR-2"] = cdSR2
	cdSR3 := claimClone(cdSR1)
	cdSR3.StorageRequest.Meta.ID = "SR-3"
	cdSR3.RemainingBytes = availBytes1 + psb
	cs.StorageRequests["SR-3"] = cdSR3
	// validate sorting order
	tl.Logger().Infof("Case: multiple claim sorting order")
	ssIf, err := newStandaloneStorageSelector(&SelectStorageArgs{VS: vs, VSR: vsr}, cs, &SelectStorageResponse{Elements: []ElementStorage{}, LA: la})
	assert.NoError(err)
	assert.NotNil(ssIf)
	ss, ok := ssIf.(*standaloneSelector)
	assert.True(ok)
	td, err := cs.getStorageElementTypeData(ctx, vsr.StoragePlan.StorageElements[0])
	ss.TD = td
	assert.Equal(ClaimList{cdSR3, cdSR1, cdSR2}, ss.findLocalClaimsByPoolOrdered())

	// select with multiple
	tl.Logger().Infof("Case: select with multiple claim available")
	vsr.StoragePlan.StorageElements[0].SizeBytes = swag.Int64(int64(4) * availBytes1)
	resp, err = cs.SelectStorage(ctx, ssa)
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Len(resp.Elements, 1)
	tl.Logger().Debugf("[0] %v", resp.Elements[0])
	assert.Len(resp.Elements[0].Items, 4)
	tl.Logger().Debugf("[0] Items %v", resp.Elements[0].Items)
	expItems = []StorageItem{
		{SizeBytes: availBytes1 + psb, ShareableStorage: true, NumParcels: 21, StorageRequestID: "SR-3", PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-1"},
		{SizeBytes: availBytes1, ShareableStorage: true, NumParcels: 20, StorageRequestID: "SR-1", PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-1"},
		{SizeBytes: availBytes1 - psb, ShareableStorage: true, NumParcels: 19, StorageRequestID: "SR-2", PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-1"},
		{SizeBytes: int64(20) * psb, ShareableStorage: true, NumParcels: 20, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: asb - int64(21)*psb, NodeID: "NODE-1"},
	}
	assert.Equal(expItems, resp.Elements[0].Items)
	tl.Flush()
}

func TestSelectStandaloneNetworkSharedUseRemoteSharedStorage(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	fc := &fake.Client{}
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "CLUSTER-1",
			},
		},
	}
	la, err := layout.FindAlgorithm(layout.AlgorithmStandaloneNetworkedShared)
	assert.NoError(err)
	assert.NotNil(la)
	args := &ClusterStateArgs{
		Log:              tl.Logger(),
		OCrud:            fc,
		Cluster:          clObj,
		LayoutAlgorithms: []*layout.Algorithm{la},
	}
	cs := NewClusterState(args)
	ctx := context.Background()

	psb := int64(units.Gibibyte)
	asb := int64(500 * units.Gibibyte)
	cst := &models.CSPStorageType{
		Name:                         "Amazon gp2",
		PreferredAllocationSizeBytes: swag.Int64(asb),
		ParcelSizeBytes:              swag.Int64(psb),
	}
	cs.pCache["SP-1"] = cst // test with cached CST

	availBytes1 := int64(20) * psb // 20 parcels
	sObj1 := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta:           &models.ObjMeta{ID: "STORAGE-1", Version: 2},
			CspStorageType: "Amazon gp2",
			PoolID:         "SP-1",
			SizeBytes:      swag.Int64(asb),
		},
		StorageMutable: models.StorageMutable{
			AvailableBytes:   swag.Int64(availBytes1),
			ParcelSizeBytes:  swag.Int64(psb),
			TotalParcelCount: swag.Int64(499),
			ShareableStorage: true,
			StorageState: &models.StorageStateMutable{
				AttachedNodeID:     "NODE-2", // not on desired node
				AttachedNodeDevice: "/dev/xvdba",
				DeviceState:        "OPEN",
				MediaState:         "FORMATTED",
				AttachmentState:    "ATTACHED",
			},
		},
	}
	cs.Storage["STORAGE-1"] = sObj1

	nObj1 := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{ID: "NODE-1", Version: 2},
		},
		NodeMutable: models.NodeMutable{
			Name: "Node1",
			Service: &models.NuvoService{
				ServiceState: models.ServiceState{
					State: "READY",
				},
			},
		},
	}
	cs.Nodes["NODE-1"] = &NodeData{
		Node:            nObj1,
		AttachedStorage: []string{}, // desired node has no storage
	}
	nObj2 := nodeClone(nObj1)
	nObj2.Meta.ID = models.ObjID("NODE-2")
	nObj2.Name = "Node2"
	cs.Nodes["NODE-2"] = &NodeData{
		Node:            nObj2,
		AttachedStorage: []string{"STORAGE-1"}, // remote node has storage
	}

	vsObj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID:      "VS-1",
				Version: 1,
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				VolumeSeriesState: "NEW",
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				SizeBytes: swag.Int64(int64(1 * units.GiB)),
			},
		},
	}
	vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VSR-1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			ClusterID: "CLUSTER-1",
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "PLACEMENT",
				StoragePlan: &models.StoragePlan{
					StorageLayout: models.StorageLayoutStandalone,
					StorageElements: []*models.StoragePlanStorageElement{
						&models.StoragePlanStorageElement{
							PoolID: "SP-1",
							StorageParcels: map[string]models.StorageParcelElement{
								"STORAGE": models.StorageParcelElement{
									SizeBytes: swag.Int64(0 * asb), // actually ignored
								},
							},
						},
					},
				},
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID:         "NODE-1",
				VolumeSeriesID: "VS-1",
			},
		},
	}
	vs := vsClone(vsObj)
	vsr := vsrClone(vsrObj)
	ssa := &SelectStorageArgs{
		VSR: vsr,
		VS:  vs,
	}

	// Existing storage has sufficient space
	tl.Logger().Infof("Case: existing storage has sufficient space")
	assert.NotEmpty(cs.Storage)
	assert.Equal([]*models.Storage{sObj1}, cs.FindShareableStorageByPool("SP-1", availBytes1, "NODE-1", true))
	vsr.StoragePlan.StorageElements[0].SizeBytes = swag.Int64(availBytes1)
	resp, err := cs.SelectStorage(ctx, ssa)
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Len(resp.Elements, 1)
	tl.Logger().Debugf("[0] %v", resp.Elements[0])
	assert.Len(resp.Elements[0].Items, 1)
	tl.Logger().Debugf("[0] Items %v", resp.Elements[0].Items)
	expItems := []StorageItem{
		{SizeBytes: availBytes1, ShareableStorage: true, NumParcels: 20, Storage: sObj1, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-2"},
	}
	assert.Equal(expItems, resp.Elements[0].Items)
	tl.Flush()

	// Existing storage partially supplies the space
	tl.Logger().Infof("Case: existing storage partially supplies space")
	assert.NotEmpty(cs.Storage)
	assert.Equal([]*models.Storage{sObj1}, cs.FindShareableStorageByPool("SP-1", availBytes1, "NODE-1", true))
	vsr.StoragePlan.StorageElements[0].SizeBytes = swag.Int64(availBytes1 + 10*psb)
	resp, err = cs.SelectStorage(ctx, ssa)
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Len(resp.Elements, 1)
	tl.Logger().Debugf("[0] %v", resp.Elements[0])
	assert.Len(resp.Elements[0].Items, 2)
	tl.Logger().Debugf("[0] Items %v", resp.Elements[0].Items)
	expItems = []StorageItem{
		{SizeBytes: availBytes1, ShareableStorage: true, NumParcels: 20, Storage: sObj1, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-2"},
		{SizeBytes: int64(10) * psb, ShareableStorage: true, NumParcels: 10, Storage: nil, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: asb - int64(11)*psb, NodeID: "NODE-1"},
	}
	assert.Equal(expItems, resp.Elements[0].Items)
	tl.Flush()

	// Existing storage has insufficient space
	tl.Logger().Infof("Case: existing storage has insufficient space")
	sObj1.AvailableBytes = swag.Int64(psb - int64(1))
	assert.NotEmpty(cs.Storage)
	assert.Equal([]*models.Storage{}, cs.FindShareableStorageByPool("SP-1", availBytes1, "NODE-1", true))
	vsr.StoragePlan.StorageElements[0].SizeBytes = swag.Int64(availBytes1)
	resp, err = cs.SelectStorage(ctx, ssa)
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Len(resp.Elements, 1)
	tl.Logger().Debugf("[0] %v", resp.Elements[0])
	assert.Len(resp.Elements[0].Items, 1)
	tl.Logger().Debugf("[0] Items %v", resp.Elements[0].Items)
	expItems = []StorageItem{
		{SizeBytes: int64(20) * psb, ShareableStorage: true, NumParcels: 20, Storage: nil, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: asb - int64(21)*psb, NodeID: "NODE-1"},
	}
	assert.Equal(expItems, resp.Elements[0].Items)
	tl.Flush()

	// Multiple storage objects
	sObj1.AvailableBytes = swag.Int64(availBytes1)
	sObj2 := sClone(sObj1)
	sObj2.AvailableBytes = swag.Int64(availBytes1 - psb)
	sObj2.Meta.ID = "STORAGE-2"
	sObj2.StorageState.AttachedNodeID = "NODE-3"
	cs.Storage["STORAGE-2"] = sObj2
	sObj3 := sClone(sObj1)
	sObj3.AvailableBytes = swag.Int64(availBytes1 + psb)
	sObj3.Meta.ID = "STORAGE-3"
	sObj3.StorageState.AttachedNodeID = "NODE-3"
	cs.Storage["STORAGE-3"] = sObj3
	nObj3 := nodeClone(nObj1)
	nObj3.Meta.ID = models.ObjID("NODE-3")
	nObj3.Name = "Node3"
	cs.Nodes["NODE-3"] = &NodeData{
		Node:            nObj3,
		AttachedStorage: []string{"STORAGE-2", "STORAGE-3"},
	}
	// validate sorting order
	tl.Logger().Infof("Case: multiple storage sorting order")
	ssIf, err := newStandaloneStorageSelector(&SelectStorageArgs{VS: vs, VSR: vsr}, cs, &SelectStorageResponse{Elements: []ElementStorage{}, LA: la})
	assert.NoError(err)
	assert.NotNil(ssIf)
	ss, ok := ssIf.(*standaloneSelector)
	assert.True(ok)
	td, err := cs.getStorageElementTypeData(ctx, vsr.StoragePlan.StorageElements[0])
	ss.TD = td
	assert.Equal([]*models.Storage{sObj3, sObj1, sObj2}, ss.findRemoteStorageByPoolOrdered())

	// select with multiple
	tl.Logger().Infof("Case: select with multiple storage available")
	vsr.StoragePlan.StorageElements[0].SizeBytes = swag.Int64(int64(4) * availBytes1)
	resp, err = cs.SelectStorage(ctx, ssa)
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Len(resp.Elements, 1)
	tl.Logger().Debugf("[0] %v", resp.Elements[0])
	assert.Len(resp.Elements[0].Items, 4)
	tl.Logger().Debugf("[0] Items %v", resp.Elements[0].Items)
	expItems = []StorageItem{
		{SizeBytes: availBytes1 + psb, ShareableStorage: true, NumParcels: 21, Storage: sObj3, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-3"},
		{SizeBytes: availBytes1, ShareableStorage: true, NumParcels: 20, Storage: sObj1, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-2"},
		{SizeBytes: availBytes1 - psb, ShareableStorage: true, NumParcels: 19, Storage: sObj2, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-3"},
		{SizeBytes: int64(20) * psb, ShareableStorage: true, NumParcels: 20, Storage: nil, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: asb - int64(21)*psb, NodeID: "NODE-1"},
	}
	assert.Equal(expItems, resp.Elements[0].Items)
	tl.Flush()
}

func TestSelectStandaloneNetworkSharedRemoteSharedClaims(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	fc := &fake.Client{}
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "CLUSTER-1",
			},
		},
	}
	la, err := layout.FindAlgorithm(layout.AlgorithmStandaloneNetworkedShared)
	assert.NoError(err)
	assert.NotNil(la)
	args := &ClusterStateArgs{
		Log:              tl.Logger(),
		OCrud:            fc,
		Cluster:          clObj,
		LayoutAlgorithms: []*layout.Algorithm{la},
	}
	cs := NewClusterState(args)
	ctx := context.Background()

	psb := int64(units.Gibibyte)
	asb := int64(500 * units.Gibibyte)
	cst := &models.CSPStorageType{
		Name:                         "Amazon gp2",
		PreferredAllocationSizeBytes: swag.Int64(asb),
		ParcelSizeBytes:              swag.Int64(psb),
	}
	cs.pCache["SP-1"] = cst // test with cached CST

	availBytes1 := int64(20) * psb // 20 parcels
	cdSR1 := &ClaimData{
		StorageRequest: &models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{ID: "SR-1", Version: 2},
			},
		},
		PoolID:         "SP-1",
		NodeID:         "NODE-2",
		RemainingBytes: availBytes1,
		VSRClaims: []VSRClaim{
			{RequestID: "OTHER-VSR", SizeBytes: asb - availBytes1, Annotation: "OTHER-VSR-0"},
		},
	}
	cs.StorageRequests["SR-1"] = cdSR1

	nObj1 := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{ID: "NODE-1", Version: 2},
		},
		NodeMutable: models.NodeMutable{
			Name: "Node1",
			Service: &models.NuvoService{
				ServiceState: models.ServiceState{
					State: "READY",
				},
			},
		},
	}
	cs.Nodes["NODE-1"] = &NodeData{
		Node: nObj1,
	}
	nObj2 := nodeClone(nObj1)
	nObj2.Meta.ID = models.ObjID("NODE-2")
	nObj2.Name = "Node2"
	cs.Nodes["NODE-2"] = &NodeData{
		Node: nObj2,
	}

	vsObj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID:      "VS-1",
				Version: 1,
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				VolumeSeriesState: "NEW",
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				SizeBytes: swag.Int64(int64(1 * units.GiB)),
			},
		},
	}
	vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VSR-1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			ClusterID: "CLUSTER-1",
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "PLACEMENT",
				StoragePlan: &models.StoragePlan{
					StorageLayout: models.StorageLayoutStandalone,
					StorageElements: []*models.StoragePlanStorageElement{
						&models.StoragePlanStorageElement{
							PoolID: "SP-1",
							StorageParcels: map[string]models.StorageParcelElement{
								"STORAGE": models.StorageParcelElement{
									SizeBytes: swag.Int64(0 * asb), // actually ignored
								},
							},
						},
					},
				},
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID:         "NODE-1",
				VolumeSeriesID: "VS-1",
			},
		},
	}
	vs := vsClone(vsObj)
	vsr := vsrClone(vsrObj)
	ssa := &SelectStorageArgs{
		VSR: vsr,
		VS:  vs,
	}

	// Existing claim has sufficient space
	tl.Logger().Infof("Case: existing claim has sufficient space")
	assert.NotEmpty(cs.StorageRequests)
	assert.Equal(ClaimList{cdSR1}, cs.FindClaimsByPool("SP-1", availBytes1, "NODE-1", true))
	vsr.StoragePlan.StorageElements[0].SizeBytes = swag.Int64(availBytes1)
	resp, err := cs.SelectStorage(ctx, ssa)
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Len(resp.Elements, 1)
	tl.Logger().Debugf("[0] %v", resp.Elements[0])
	assert.Len(resp.Elements[0].Items, 1)
	tl.Logger().Debugf("[0] Items %v", resp.Elements[0].Items)
	expItems := []StorageItem{
		{SizeBytes: availBytes1, ShareableStorage: true, NumParcels: 20, StorageRequestID: "SR-1", PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-2"},
	}
	assert.Equal(expItems, resp.Elements[0].Items)
	tl.Flush()

	// Existing claim partially supplies the space
	tl.Logger().Infof("Case: existing claim partially supplies space")
	assert.NotEmpty(cs.StorageRequests)
	assert.Equal(ClaimList{cdSR1}, cs.FindClaimsByPool("SP-1", availBytes1, "NODE-1", true))
	vsr.StoragePlan.StorageElements[0].SizeBytes = swag.Int64(availBytes1 + 10*psb)
	resp, err = cs.SelectStorage(ctx, ssa)
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Len(resp.Elements, 1)
	tl.Logger().Debugf("[0] %v", resp.Elements[0])
	assert.Len(resp.Elements[0].Items, 2)
	tl.Logger().Debugf("[0] Items %v", resp.Elements[0].Items)
	expItems = []StorageItem{
		{SizeBytes: availBytes1, ShareableStorage: true, NumParcels: 20, StorageRequestID: "SR-1", PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-2"},
		{SizeBytes: int64(10) * psb, ShareableStorage: true, NumParcels: 10, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: asb - int64(11)*psb, NodeID: "NODE-1"},
	}
	assert.Equal(expItems, resp.Elements[0].Items)
	tl.Flush()

	// Existing claim has insufficient space
	tl.Logger().Infof("Case: existing claim has insufficient space")
	cdSR1.RemainingBytes = psb - int64(1)
	assert.NotEmpty(cs.StorageRequests)
	assert.Equal(ClaimList{}, cs.FindClaimsByPool("SP-1", availBytes1, "NODE-1", true))
	vsr.StoragePlan.StorageElements[0].SizeBytes = swag.Int64(availBytes1)
	resp, err = cs.SelectStorage(ctx, ssa)
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Len(resp.Elements, 1)
	tl.Logger().Debugf("[0] %v", resp.Elements[0])
	assert.Len(resp.Elements[0].Items, 1)
	tl.Logger().Debugf("[0] Items %v", resp.Elements[0].Items)
	expItems = []StorageItem{
		{SizeBytes: int64(20) * psb, ShareableStorage: true, NumParcels: 20, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: asb - int64(21)*psb, NodeID: "NODE-1"},
	}
	assert.Equal(expItems, resp.Elements[0].Items)
	tl.Flush()

	// Multiple claims
	cdSR1.RemainingBytes = availBytes1
	cdSR2 := claimClone(cdSR1)
	cdSR2.StorageRequest.Meta.ID = "SR-2"
	cdSR2.RemainingBytes = availBytes1 - psb
	cdSR2.NodeID = "NODE-3"
	cs.StorageRequests["SR-2"] = cdSR2
	cdSR3 := claimClone(cdSR1)
	cdSR3.StorageRequest.Meta.ID = "SR-3"
	cdSR3.RemainingBytes = availBytes1 + psb
	cdSR3.NodeID = "NODE-3"
	cs.StorageRequests["SR-3"] = cdSR3
	nObj3 := nodeClone(nObj1)
	nObj3.Meta.ID = models.ObjID("NODE-3")
	nObj3.Name = "Node3"
	cs.Nodes["NODE-3"] = &NodeData{
		Node: nObj3,
	}
	// validate sorting order
	tl.Logger().Infof("Case: multiple claim sorting order")
	ssIf, err := newStandaloneStorageSelector(&SelectStorageArgs{VS: vs, VSR: vsr}, cs, &SelectStorageResponse{Elements: []ElementStorage{}, LA: la})
	assert.NoError(err)
	assert.NotNil(ssIf)
	ss, ok := ssIf.(*standaloneSelector)
	assert.True(ok)
	td, err := cs.getStorageElementTypeData(ctx, vsr.StoragePlan.StorageElements[0])
	ss.TD = td
	assert.Equal(ClaimList{cdSR3, cdSR1, cdSR2}, ss.findRemoteClaimsByPoolOrdered())

	// select with multiple
	tl.Logger().Infof("Case: select with multiple claim available")
	vsr.StoragePlan.StorageElements[0].SizeBytes = swag.Int64(int64(4) * availBytes1)
	resp, err = cs.SelectStorage(ctx, ssa)
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Len(resp.Elements, 1)
	tl.Logger().Debugf("[0] %v", resp.Elements[0])
	assert.Len(resp.Elements[0].Items, 4)
	tl.Logger().Debugf("[0] Items %v", resp.Elements[0].Items)
	expItems = []StorageItem{
		{SizeBytes: availBytes1 + psb, ShareableStorage: true, NumParcels: 21, StorageRequestID: "SR-3", PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-3"},
		{SizeBytes: availBytes1, ShareableStorage: true, NumParcels: 20, StorageRequestID: "SR-1", PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-2"},
		{SizeBytes: availBytes1 - psb, ShareableStorage: true, NumParcels: 19, StorageRequestID: "SR-2", PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-3"},
		{SizeBytes: int64(20) * psb, ShareableStorage: true, NumParcels: 20, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: asb - int64(21)*psb, NodeID: "NODE-1"},
	}
	assert.Equal(expItems, resp.Elements[0].Items)
	tl.Flush()
}

// TestSelectStandaloneNetworkSharedMultiSource walks through all the iterator sources
func TestSelectStandaloneNetworkSharedMultiSource(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	fc := &fake.Client{}
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "CLUSTER-1",
			},
		},
	}
	la, err := layout.FindAlgorithm(layout.AlgorithmStandaloneNetworkedShared)
	assert.NoError(err)
	assert.NotNil(la)
	args := &ClusterStateArgs{
		Log:              tl.Logger(),
		OCrud:            fc,
		Cluster:          clObj,
		LayoutAlgorithms: []*layout.Algorithm{la},
	}
	cs := NewClusterState(args)
	ctx := context.Background()

	psb := int64(units.Gibibyte)
	asb := int64(500 * units.Gibibyte)
	cst := &models.CSPStorageType{
		Name:                         "Amazon gp2",
		PreferredAllocationSizeBytes: swag.Int64(asb),
		ParcelSizeBytes:              swag.Int64(psb),
	}
	cs.pCache["SP-1"] = cst // test with cached CST

	availBytes1 := int64(20) * psb // 20 parcels
	sObj1 := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta:           &models.ObjMeta{ID: "STORAGE-1", Version: 2},
			CspStorageType: "Amazon gp2",
			PoolID:         "SP-1",
			SizeBytes:      swag.Int64(asb),
		},
		StorageMutable: models.StorageMutable{
			AvailableBytes:   swag.Int64(availBytes1),
			ParcelSizeBytes:  swag.Int64(psb),
			TotalParcelCount: swag.Int64(499),
			ShareableStorage: true,
			StorageState: &models.StorageStateMutable{
				AttachedNodeID:     "NODE-1",
				AttachedNodeDevice: "/dev/xvdba",
				DeviceState:        "OPEN",
				MediaState:         "FORMATTED",
				AttachmentState:    "ATTACHED",
			},
		},
	}
	cs.Storage["STORAGE-1"] = sObj1
	sObj2 := sClone(sObj1)
	sObj2.Meta.ID = models.ObjID("STORAGE-2")
	sObj2.StorageState.AttachedNodeID = models.ObjIDMutable("NODE-2")
	cs.Storage["STORAGE-2"] = sObj2

	cdSR1 := &ClaimData{
		StorageRequest: &models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{ID: "SR-1", Version: 2},
			},
		},
		PoolID:         "SP-1",
		NodeID:         "NODE-1",
		RemainingBytes: availBytes1,
		VSRClaims: []VSRClaim{
			{RequestID: "OTHER-VSR", SizeBytes: asb - availBytes1, Annotation: "OTHER-VSR-0"},
		},
	}
	cs.StorageRequests["SR-1"] = cdSR1
	cdSR2 := claimClone(cdSR1)
	cdSR2.StorageRequest.Meta.ID = "SR-2"
	cdSR2.NodeID = "NODE-2"
	cs.StorageRequests["SR-2"] = cdSR2

	nObj1 := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{ID: "NODE-1", Version: 2},
		},
		NodeMutable: models.NodeMutable{
			Name: "Node1",
			Service: &models.NuvoService{
				ServiceState: models.ServiceState{
					State: "READY",
				},
			},
		},
	}
	cs.Nodes["NODE-1"] = &NodeData{
		Node:            nObj1,
		AttachedStorage: []string{"STORAGE-1"},
	}
	nObj2 := nodeClone(nObj1)
	nObj2.Meta.ID = models.ObjID("NODE-2")
	nObj2.Name = "Node2"
	cs.Nodes["NODE-2"] = &NodeData{
		Node:            nObj2,
		AttachedStorage: []string{"STORAGE-2"},
	}

	vsObj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID:      "VS-1",
				Version: 1,
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				VolumeSeriesState: "NEW",
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				SizeBytes: swag.Int64(int64(1 * units.GiB)),
			},
		},
	}
	vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VSR-1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			ClusterID: "CLUSTER-1",
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "PLACEMENT",
				StoragePlan: &models.StoragePlan{
					StorageLayout: models.StorageLayoutStandalone,
					StorageElements: []*models.StoragePlanStorageElement{
						&models.StoragePlanStorageElement{
							PoolID: "SP-1",
							StorageParcels: map[string]models.StorageParcelElement{
								"STORAGE": models.StorageParcelElement{
									SizeBytes: swag.Int64(0 * asb), // actually ignored
								},
							},
						},
					},
				},
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID:         "NODE-1",
				VolumeSeriesID: "VS-1",
			},
		},
	}
	vs := vsClone(vsObj)
	vsr := vsrClone(vsrObj)
	ssa := &SelectStorageArgs{
		VSR: vsr,
		VS:  vs,
	}

	tl.Logger().Infof("Case: select with multiple sources available")
	vsr.StoragePlan.StorageElements[0].SizeBytes = swag.Int64(int64(5) * availBytes1)
	expItems := []StorageItem{
		{SizeBytes: availBytes1, ShareableStorage: true, NumParcels: 20, Storage: sObj1, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-1"},
		{SizeBytes: availBytes1, ShareableStorage: true, NumParcels: 20, StorageRequestID: "SR-1", PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-1"},
		{SizeBytes: availBytes1, ShareableStorage: true, NumParcels: 20, Storage: sObj2, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-2"},
		{SizeBytes: availBytes1, ShareableStorage: true, NumParcels: 20, StorageRequestID: "SR-2", PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: 0, NodeID: "NODE-2"},
		{SizeBytes: int64(20) * psb, ShareableStorage: true, NumParcels: 20, PoolID: "SP-1", MinSizeBytes: asb, RemainingSizeBytes: asb - int64(21)*psb, NodeID: "NODE-1"},
	}
	resp, err := cs.SelectStorage(ctx, ssa)
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Len(resp.Elements, 1)
	tl.Logger().Debugf("[0] %v", resp.Elements[0])
	assert.Len(resp.Elements[0].Items, len(expItems))
	tl.Logger().Debugf("[0] Items %v", resp.Elements[0].Items)
	assert.Equal(expItems, resp.Elements[0].Items)
	tl.Flush()
}

func TestSelectStandaloneLocalUnsharedPrimitives(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "CLUSTER-1",
			},
		},
	}
	args := &ClusterStateArgs{
		Log:     tl.Logger(),
		Cluster: clObj,
	}
	cs := NewClusterState(args)
	ctx := context.Background()

	maxP := int64(10)
	psb := int64(units.Gibibyte)
	minB := int64(2 * psb)
	maxB := maxP * psb
	cst := &models.CSPStorageType{
		Name:                   "Amazon gp2",
		MaxAllocationSizeBytes: swag.Int64(maxB + nuvoapi.DefaultDeviceFormatOverheadBytes),
		MinAllocationSizeBytes: swag.Int64(minB + nuvoapi.DefaultDeviceFormatOverheadBytes),
		ParcelSizeBytes:        swag.Int64(psb),
	}
	cs.pCache["SP-1"] = cst // test with cached CST

	// test primitives with even minB

	// create appropriate storage objects for this sequence of tests
	cs.Storage = make(map[string]*models.Storage)
	availStorageParcels := []int64{maxP, 1, 6, 5}
	expSpM := []int64{maxP}
	expSp1 := []int64{1}
	expSp65 := []int64{6, 5}
	expSpM65 := []int64{maxP, 6, 5}
	for i, szP := range availStorageParcels {
		szB := szP * psb
		s := &models.Storage{}
		id := fmt.Sprintf("S-%d", i)
		s.Meta = &models.ObjMeta{ID: models.ObjID(id)}
		s.PoolID = "SP-1"
		s.StorageState = &models.StorageStateMutable{}
		s.StorageState.AttachedNodeID = "NODE-1"
		s.AvailableBytes = swag.Int64(szB)
		s.ParcelSizeBytes = swag.Int64(psb)
		s.TotalParcelCount = swag.Int64(szP)
		s.SizeBytes = swag.Int64(szB + nuvoapi.DefaultDeviceFormatOverheadBytes)
		cs.Storage[id] = s
	}
	computeTCsEven := []struct {
		npn                           int64   // number parcels needed
		expMaxCount, expRem1, expRem2 int64   // expected values
		expS                          []int64 // expected parcel sizes
	}{
		{},
		{1, 0, 1, 0, expSp1},
		{2, 0, 2, 0, nil},
		{3, 0, 3, 0, nil},
		{9, 0, 9, 0, nil},
		{10, 1, 0, 0, expSpM},
		{11, 0, 6, 5, expSp65},
		{12, 1, 2, 0, expSpM},
		{19, 1, 9, 0, expSpM},
		{20, 2, 0, 0, expSpM},
		{21, 1, 6, 5, expSpM65},
		{22, 2, 2, 0, expSpM},
	}
	for i, tc := range computeTCsEven {
		ss := &standaloneSelector{}
		ss.CS = cs
		ss.NodeID = "NODE-1"
		spe := &models.StoragePlanStorageElement{
			SizeBytes: swag.Int64(minB),
			PoolID:    "SP-1",
			StorageParcels: map[string]models.StorageParcelElement{
				"STORAGE": models.StorageParcelElement{
					SizeBytes: swag.Int64(0), // actually ignored
				},
			},
		}
		td, err := cs.getStorageElementTypeData(ctx, spe)
		assert.NoError(err)
		assert.Equal(psb, td.ParcelSizeBytes)
		assert.Equal(maxB/psb, td.MaxParcelsVariableSizedStorage)
		assert.Equal(minB/psb, td.MinParcelsVariableSizedStorage)
		ss.TD = td
		maxCount, rem1, rem2 := ss.computeMinimalVariableStorage(tc.npn)
		t.Log("***E", i, tc.npn, tc.expMaxCount, tc.expRem1, tc.expRem2, tc.expS, "/", maxCount, rem1, rem2)
		assert.EqualValues(tc.expMaxCount, maxCount, "case %d", i)
		assert.EqualValues(tc.expRem1, rem1, "case %d", i)
		assert.EqualValues(tc.expRem2, rem2, "case %d", i)

		si := ss.provisionVariableSizeStorage(tc.npn)
		assert.NotNil(si)
		np := maxB / psb
		if tc.expMaxCount == 0 {
			np = tc.expRem1
		}
		assert.EqualValues(np, si.NumParcels, "case %d", i)
		assert.EqualValues(np*psb, si.SizeBytes, "case %d", i)
		if np < minB/psb {
			assert.EqualValues(minB+nuvoapi.DefaultDeviceFormatOverheadBytes, si.MinSizeBytes, "case %d", i)
		} else {
			assert.EqualValues(np*psb+nuvoapi.DefaultDeviceFormatOverheadBytes, si.MinSizeBytes, "case %d", i)
		}
		assert.False(si.ShareableStorage, "case %d", i)
		assert.EqualValues(spe.PoolID, si.PoolID, "case %d", i)
		assert.Nil(si.Storage, "case %d", i)
		assert.Zero(si.RemainingSizeBytes, "case %d", i)
		assert.Empty(si.StorageRequestID, "case %d", i)
		assert.Equal(ss.NodeID, si.NodeID)

		sl := ss.findUnsharedStorageByPoolOrdered(tc.npn)
		assert.Equal(len(tc.expS), len(sl), "case %d", i)
		if tc.expS != nil {
			p := []int64{}
			for _, s := range sl {
				p = append(p, swag.Int64Value(s.AvailableBytes)/psb)
			}
			assert.Equal(tc.expS, p, "case %d", i)
		}

		tl.Flush()
	}

	// test primitives with odd minB
	minB = int64(3 * psb)
	cst.MinAllocationSizeBytes = swag.Int64(minB + nuvoapi.DefaultDeviceFormatOverheadBytes)
	// create appropriate storage objects for this sequence of tests
	cs.Storage = make(map[string]*models.Storage)
	availStorageParcels = []int64{maxP, 1, 6, 6, 5}
	expSp66 := []int64{6, 6}
	expSpM66 := []int64{maxP, 6, 6}
	for i, szP := range availStorageParcels {
		szB := szP * psb
		s := &models.Storage{}
		id := fmt.Sprintf("S-%d", i)
		s.Meta = &models.ObjMeta{ID: models.ObjID(id)}
		s.PoolID = "SP-1"
		s.StorageState = &models.StorageStateMutable{}
		s.StorageState.AttachedNodeID = "NODE-1"
		s.AvailableBytes = swag.Int64(szB)
		s.ParcelSizeBytes = swag.Int64(psb)
		s.TotalParcelCount = swag.Int64(szP)
		s.SizeBytes = swag.Int64(szB + nuvoapi.DefaultDeviceFormatOverheadBytes)
		cs.Storage[id] = s
	}
	computeTCsOdd := []struct {
		npn                           int64   // number parcels needed
		expMaxCount, expRem1, expRem2 int64   // expected values
		expS                          []int64 // expected parcel sizes
	}{
		{1, 0, 1, 0, expSp1},
		{2, 0, 2, 0, nil},
		{10, 1, 0, 0, expSpM},
		{11, 0, 6, 5, expSp65},
		{12, 0, 6, 6, expSp66},
		{22, 1, 6, 6, expSpM66},
	}
	for i, tc := range computeTCsOdd {
		ss := &standaloneSelector{}
		ss.CS = cs
		ss.NodeID = "NODE-1"
		spe := &models.StoragePlanStorageElement{
			SizeBytes: swag.Int64(minB),
			PoolID:    "SP-1",
			StorageParcels: map[string]models.StorageParcelElement{
				"STORAGE": models.StorageParcelElement{
					SizeBytes: swag.Int64(0), // actually ignored
				},
			},
		}
		td, err := cs.getStorageElementTypeData(ctx, spe)
		assert.NoError(err)
		assert.Equal(psb, td.ParcelSizeBytes)
		assert.Equal(maxB/psb, td.MaxParcelsVariableSizedStorage)
		assert.Equal(minB/psb, td.MinParcelsVariableSizedStorage)
		ss.TD = td
		maxCount, rem1, rem2 := ss.computeMinimalVariableStorage(tc.npn)
		t.Log("***O", i, tc.npn, tc.expMaxCount, tc.expRem1, tc.expRem2, maxCount, rem1, rem2)
		assert.EqualValues(tc.expMaxCount, maxCount, "case %d", i)
		assert.EqualValues(tc.expRem1, rem1, "case %d", i)
		assert.EqualValues(tc.expRem2, rem2, "case %d", i)

		si := ss.provisionVariableSizeStorage(tc.npn) // indirectly tests MakeUnsharedStorageItem
		assert.NotNil(si)
		np := maxB / psb
		if tc.expMaxCount == 0 {
			np = tc.expRem1
		}
		assert.EqualValues(np, si.NumParcels, "case %d", i)
		assert.EqualValues(np*psb, si.SizeBytes, "case %d", i)
		if np < minB/psb {
			assert.EqualValues(minB+nuvoapi.DefaultDeviceFormatOverheadBytes, si.MinSizeBytes, "case %d", i)
		} else {
			assert.EqualValues(np*psb+nuvoapi.DefaultDeviceFormatOverheadBytes, si.MinSizeBytes, "case %d", i)
		}
		assert.False(si.ShareableStorage, "case %d", i)
		assert.EqualValues(spe.PoolID, si.PoolID, "case %d", i)
		assert.Nil(si.Storage, "case %d", i)
		assert.Zero(si.RemainingSizeBytes, "case %d", i)
		assert.Empty(si.StorageRequestID, "case %d", i)
		assert.Equal(ss.NodeID, si.NodeID)

		sl := ss.findUnsharedStorageByPoolOrdered(tc.npn)
		assert.Equal(len(tc.expS), len(sl), "case %d", i)
		if tc.expS != nil {
			p := []int64{}
			for _, s := range sl {
				p = append(p, swag.Int64Value(s.AvailableBytes)/psb)
			}
			assert.Equal(tc.expS, p, "case %d", i)
		}

		tl.Flush()
	}
}

func TestSelectStandaloneLocalUnsharedStateMachine(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "CLUSTER-1",
			},
		},
	}
	la, err := layout.FindAlgorithm(layout.AlgorithmStandaloneLocalUnshared)
	assert.NoError(err)
	assert.NotNil(la)
	args := &ClusterStateArgs{
		Log:              tl.Logger(),
		Cluster:          clObj,
		LayoutAlgorithms: []*layout.Algorithm{la},
	}
	cs := NewClusterState(args)
	ctx := context.Background()

	maxP := int64(10)
	psb := int64(units.Gibibyte)
	minB := int64(2 * psb)
	maxB := maxP * psb
	// use the 11 parcel case => 0,6,5
	asb := (maxP + 1) * psb
	cst := &models.CSPStorageType{
		Name:                   "Amazon gp2",
		MaxAllocationSizeBytes: swag.Int64(maxB + nuvoapi.DefaultDeviceFormatOverheadBytes),
		MinAllocationSizeBytes: swag.Int64(minB + nuvoapi.DefaultDeviceFormatOverheadBytes),
		ParcelSizeBytes:        swag.Int64(psb),
	}
	cs.pCache["SP-1"] = cst // test with cached CST

	vsObj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID:      "VS-1",
				Version: 1,
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				VolumeSeriesState: "NEW",
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				SizeBytes: swag.Int64(int64(1 * units.GiB)),
			},
		},
	}
	vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VSR-1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			ClusterID: "CLUSTER-1",
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "PLACEMENT",
				StoragePlan: &models.StoragePlan{
					StorageLayout: models.StorageLayoutStandalone,
					StorageElements: []*models.StoragePlanStorageElement{
						&models.StoragePlanStorageElement{
							SizeBytes: swag.Int64(asb),
							PoolID:    "SP-1",
							StorageParcels: map[string]models.StorageParcelElement{
								"STORAGE": models.StorageParcelElement{
									SizeBytes: swag.Int64(0 * asb), // actually ignored
								},
							},
						},
					},
				},
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID:         "NODE-1",
				VolumeSeriesID: "VS-1",
			},
		},
	}
	vs := vsClone(vsObj)
	vsr := vsrClone(vsrObj)
	vsrNotStandalone := vsrClone(vsrObj)
	vsrNotStandalone.StoragePlan.StorageLayout = models.StorageLayout("not" + models.StorageLayoutStandalone)

	// argument validation (internal invocation)
	argTCs := []struct {
		Args *SelectStorageArgs
		CS   *ClusterState
		Resp *SelectStorageResponse
	}{
		{nil, cs, &SelectStorageResponse{Elements: []ElementStorage{}}},
		{&SelectStorageArgs{}, cs, &SelectStorageResponse{Elements: []ElementStorage{}}},
		{&SelectStorageArgs{VS: vs}, cs, &SelectStorageResponse{Elements: []ElementStorage{}}},
		{&SelectStorageArgs{VSR: vsr}, cs, &SelectStorageResponse{Elements: []ElementStorage{}}},
		{&SelectStorageArgs{VS: vs, VSR: vsr}, nil, &SelectStorageResponse{Elements: []ElementStorage{}}},
		{&SelectStorageArgs{VS: vs, VSR: vsr}, cs, &SelectStorageResponse{}},
		{&SelectStorageArgs{VS: vs, VSR: vsr}, cs, nil},
		{&SelectStorageArgs{VS: vs, VSR: vsr}, cs, &SelectStorageResponse{Elements: []ElementStorage{}}},
		{&SelectStorageArgs{VS: vs, VSR: vsrNotStandalone}, cs, &SelectStorageResponse{Elements: []ElementStorage{}, LA: la}},
	}
	for i, tc := range argTCs {
		tl.Logger().Infof("Case: %d", i)
		ss, err := newStandaloneStorageSelector(tc.Args, tc.CS, tc.Resp)
		assert.Error(err)
		assert.Nil(ss)
	}
	tl.Flush()

	ssa := &SelectStorageArgs{
		VSR: vsr,
		VS:  vs,
	}

	// test construction of the state machine
	ss, err := newStandaloneStorageSelector(ssa, cs, &SelectStorageResponse{Elements: []ElementStorage{}, LA: la})
	assert.NoError(err)
	assert.NotNil(ss)
	ssSA, ok := ss.(*standaloneSelector)
	assert.True(ok)
	assert.NotEmpty(ssSA.stateMachine)
	expSM := []standaloneIterState{
		saIterFindLocalUnusedStorage,
		saIterServeLocalUnusedStorage,
		saIterProvisionLocalVariableSizeStorage,
	}
	assert.Equal(expSM, ssSA.stateMachine)

	// end-to-end test
	cs.Storage = make(map[string]*models.Storage)
	availStorageParcels := []int64{5}
	for i, szP := range availStorageParcels {
		szB := szP * psb
		s := &models.Storage{}
		id := fmt.Sprintf("S-%d", i)
		s.Meta = &models.ObjMeta{ID: models.ObjID(id)}
		s.PoolID = "SP-1"
		s.StorageState = &models.StorageStateMutable{}
		s.StorageState.AttachedNodeID = "NODE-1"
		s.AvailableBytes = swag.Int64(szB)
		s.ParcelSizeBytes = swag.Int64(psb)
		s.TotalParcelCount = swag.Int64(szP)
		s.SizeBytes = swag.Int64(szB + nuvoapi.DefaultDeviceFormatOverheadBytes)
		cs.Storage[id] = s
	}
	plan := ssa.VSR.StoragePlan
	resp := &SelectStorageResponse{}
	resp.Elements = make([]ElementStorage, len(plan.StorageElements))
	resp.LA = la
	err = cs.fillStorageElements(ctx, ssa, plan, ss, resp)
	assert.NoError(err)
	assert.Equal(1, len(resp.Elements))
	assert.Equal(2, len(resp.Elements[0].Items))
	s0 := cs.Storage["S-0"]
	assert.NotNil(s0)
	assert.Equal(s0, resp.Elements[0].Items[0].Storage)
	assert.False(resp.Elements[0].Items[0].ShareableStorage)
	assert.Nil(resp.Elements[0].Items[1].Storage)
	assert.Equal(6*psb+nuvoapi.DefaultDeviceFormatOverheadBytes, resp.Elements[0].Items[1].MinSizeBytes)
	assert.False(resp.Elements[0].Items[1].ShareableStorage)
}

func TestIterStateString(t *testing.T) {
	assert := assert.New(t)
	var s standaloneIterState
	for s = 0; s < saIterLast; s++ {
		str := s.String()
		assert.NotEmpty(str)
		assert.NotEqual("UNKNOWN", str, "case: %d", s)
	}
	s = saIterLast
	assert.Equal("UNKNOWN", s.String())
}
