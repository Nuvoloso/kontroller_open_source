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

func sClone(s *models.Storage) *models.Storage {
	n := new(models.Storage)
	testutils.Clone(s, n)
	return n
}

func claimClone(s *ClaimData) *ClaimData {
	n := new(ClaimData)
	testutils.Clone(s, n)
	return n
}

func nodeClone(node *models.Node) *models.Node {
	n := new(models.Node)
	testutils.Clone(node, n)
	return n
}

func vsClone(vs *models.VolumeSeries) *models.VolumeSeries {
	n := new(models.VolumeSeries)
	testutils.Clone(vs, n)
	return n
}

func vsrClone(vsr *models.VolumeSeriesRequest) *models.VolumeSeriesRequest {
	n := new(models.VolumeSeriesRequest)
	testutils.Clone(vsr, n)
	return n
}

func srClone(sr *models.StorageRequest) *models.StorageRequest {
	n := new(models.StorageRequest)
	testutils.Clone(sr, n)
	return n
}

func TestSelectInvocation(t *testing.T) {
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
	psb := int64(852 * units.MiB)
	asb := int64(10 * units.Gibibyte)
	minSB := psb * 10
	maxSB := asb * 100
	cst := &models.CSPStorageType{
		Name:                             "Amazon gp2",
		PreferredAllocationSizeBytes:     swag.Int64(asb),
		ParcelSizeBytes:                  swag.Int64(psb),
		PreferredAllocationUnitSizeBytes: swag.Int64(2 * psb),
		MinAllocationSizeBytes:           swag.Int64(minSB),
		MaxAllocationSizeBytes:           swag.Int64(maxSB),
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
					StorageLayout: la.StorageLayout,
					StorageElements: []*models.StoragePlanStorageElement{
						&models.StoragePlanStorageElement{
							Intent:    "DATA",
							SizeBytes: swag.Int64(asb - psb),
							PoolID:    "SP-1",
							StorageParcels: map[string]models.StorageParcelElement{
								"STORAGE": models.StorageParcelElement{
									SizeBytes: swag.Int64(0 * asb), // actually ignored
								},
							},
						},
						&models.StoragePlanStorageElement{
							Intent:    "CACHE", // element should be ignored
							SizeBytes: swag.Int64(int64(200 * units.MiB)),
							StorageParcels: map[string]models.StorageParcelElement{
								"SSD": models.StorageParcelElement{
									ProvMinSizeBytes: swag.Int64(0),
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
	vsrInvalidLayout := vsrClone(vsrObj)
	vsrInvalidLayout.StoragePlan.StorageLayout = "TestLayout"

	// empty args
	ssa := &SelectStorageArgs{}
	tl.Logger().Infof("Case: no VS")
	resp, err := cs.SelectStorage(ctx, ssa)
	assert.Error(err)
	assert.Regexp("missing arguments", err)
	assert.Nil(resp)
	ssa = &SelectStorageArgs{VS: vs}
	tl.Logger().Infof("Case: no VSR")
	resp, err = cs.SelectStorage(ctx, ssa)
	assert.Error(err)
	assert.Regexp("missing arguments", err)
	assert.Nil(resp)

	ssa = &SelectStorageArgs{
		VSR: vsrInvalidLayout,
		VS:  vs,
		LA:  cs.FindLayoutAlgorithm(vsr.StoragePlan.StorageLayout),
	}
	tl.Logger().Infof("Case: incompatible placement algorithm")
	assert.NotNil(ssa.LA)
	resp, err = cs.SelectStorage(ctx, ssa)
	assert.Error(err)
	assert.Regexp("incompatible placement algorithm", err)
	assert.Nil(resp)
	tl.Flush()

	ssa = &SelectStorageArgs{
		VSR: vsrInvalidLayout,
		VS:  vs,
	}
	tl.Logger().Infof("Case: placement algorithm not found")
	resp, err = cs.SelectStorage(ctx, ssa)
	assert.Error(err)
	assert.Regexp("no placement algorithm for layout", err)
	assert.Nil(resp)
	tl.Flush()

	// invalid layout
	ssa = &SelectStorageArgs{
		VSR: vsrInvalidLayout,
		VS:  vs,
		LA:  layout.TestLayout,
	}
	tl.Logger().Infof("Case: unsupported layout")
	resp, err = cs.SelectStorage(ctx, ssa)
	assert.Error(err)
	assert.Regexp("unsupported storage layout.*TestLayout", err)
	assert.Nil(resp)
	tl.Flush()

	// error looking up pool
	ssa = &SelectStorageArgs{
		VSR: vsr,
		VS:  vs,
	}
	tl.Logger().Infof("Case: invalid SPid")
	fc.RetPoolFetchErr = fmt.Errorf("Invalid SPid")
	resp, err = cs.SelectStorage(ctx, ssa)
	assert.Error(err)
	assert.Nil(resp)
	tl.Flush()

	// cache the cst and test getting the storage type data
	cs.pCache["SP-1"] = cst
	td, err := cs.getStorageElementTypeData(ctx, vsr.StoragePlan.StorageElements[0])
	assert.NoError(err)
	assert.NotNil(td)
	assert.Equal("SP-1", td.PoolID)
	assert.Equal(cst, td.StorageType)
	assert.Equal(asb, td.PreferredAllocationSizeBytes)
	assert.Equal(psb, td.ParcelSizeBytes)
	assert.Equal(int64(2*psb), td.PreferredAllocationUnitSizeBytes)
	assert.Equal((asb-nuvoapi.DefaultDeviceFormatOverheadBytes)/psb, td.MaxParcelsFixedSizedStorage)
	assert.Equal(swag.Int64Value(vsr.StoragePlan.StorageElements[0].SizeBytes), td.ElementSizeBytes)
	assert.Equal(minSB, td.MinSizeBytes)
	assert.Equal(maxSB, td.MaxSizeBytes)
	assert.Equal(int64(1), td.SizeBytesToParcels(1))
	assert.Equal(int64(1), td.SizeBytesToParcels(td.ParcelSizeBytes))
	assert.Equal(int64(2), td.SizeBytesToParcels(td.ParcelSizeBytes+1))
	assert.Equal(asb/psb, td.MaxParcelsFixedSizedStorage)
	assert.Equal(int64(9), td.MinParcelsVariableSizedStorage)
	assert.Equal(maxSB/psb, td.MaxParcelsVariableSizedStorage)

	tl.Logger().Infof("Case: success, CACHE ignored")
	ssa = &SelectStorageArgs{
		VSR: vsr,
		VS:  vs,
	}
	resp, err = cs.SelectStorage(ctx, ssa)
	assert.NoError(err)
	if assert.NotNil(resp) {
		assert.Len(resp.Elements, 2)
		exp := ElementStorage{
			NumItems:        1,
			ParcelSizeBytes: psb,
			Items: []StorageItem{
				{
					SizeBytes:          asb - nuvoapi.DefaultDeviceFormatOverheadBytes,
					NumParcels:         12,
					PoolID:             "SP-1",
					MinSizeBytes:       asb,
					RemainingSizeBytes: 0,
					ShareableStorage:   true,
					NodeID:             "NODE-1",
				},
			},
		}
		assert.Equal(exp, resp.Elements[0])
		exp = ElementStorage{}
		assert.Equal(exp, resp.Elements[1])
	}
	tl.Flush()

	// test fill error
	fss := &fakeStorageSelector{}
	fss.Err = fmt.Errorf("fill-storage-element-error")
	err = cs.fillStorageElements(ctx, ssa, vsr.StoragePlan, fss, &SelectStorageResponse{Elements: []ElementStorage{{}}})
	assert.Error(err)
	assert.Regexp("fill-storage-element-error", err)
}

type fakeStorageSelector struct {
	Err error
}

func (fss *fakeStorageSelector) FillStorageElement(ctx context.Context, elNum int, el *models.StoragePlanStorageElement, td *storageElementTypeData, es *ElementStorage) error {
	return fss.Err
}
