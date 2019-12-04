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
	"encoding/json"
	"fmt"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	"github.com/go-openapi/swag"
)

// storageElementTypeData contains information derived from a CSPStorageType
type storageElementTypeData struct {
	PoolID                           string
	StorageType                      *models.CSPStorageType
	PreferredAllocationSizeBytes     int64
	MaxSizeBytes                     int64
	MinSizeBytes                     int64
	ParcelSizeBytes                  int64
	PreferredAllocationUnitSizeBytes int64 // TBD: drop this?
	MaxParcelsFixedSizedStorage      int64
	MaxParcelsVariableSizedStorage   int64
	MinParcelsVariableSizedStorage   int64
	ElementSizeBytes                 int64
}

// SizeBytesToParcels returns the number of parcels for a given size
func (td *storageElementTypeData) SizeBytesToParcels(szb int64) int64 {
	return (szb + td.ParcelSizeBytes - 1) / td.ParcelSizeBytes
}

// MakeShareableStorageItem creates a shareable StorageItem with the specified parameters
func (td *storageElementTypeData) MakeShareableStorageItem(np int64, sObj *models.Storage, remB int64, nodeID string, srID string) *StorageItem {
	return &StorageItem{
		SizeBytes:          np * td.ParcelSizeBytes,
		NumParcels:         np,
		Storage:            sObj,
		PoolID:             td.PoolID,
		MinSizeBytes:       td.PreferredAllocationSizeBytes,
		RemainingSizeBytes: remB,
		ShareableStorage:   true,
		NodeID:             nodeID,
		StorageRequestID:   srID,
	}
}

// MakeUnsharedStorageItem creates a non-shareable StorageItem with the specified parameters
func (td *storageElementTypeData) MakeUnsharedStorageItem(np int64, sObj *models.Storage, nodeID string, srID string) *StorageItem {
	minNP := np
	if minNP < td.MinParcelsVariableSizedStorage {
		minNP = td.MinParcelsVariableSizedStorage // extra space wasted
	}
	return &StorageItem{
		SizeBytes:          np * td.ParcelSizeBytes,
		NumParcels:         np,
		Storage:            sObj,
		PoolID:             td.PoolID,
		MinSizeBytes:       minNP*td.ParcelSizeBytes + nuvoapi.DefaultDeviceFormatOverheadBytes,
		RemainingSizeBytes: 0,
		ShareableStorage:   false,
		NodeID:             nodeID,
		StorageRequestID:   srID,
	}
}

// getStorageElementTypeData returns the type of storage obtained from the specified pool with derived information.
// External serialization is required to protect the internal cache.
func (cs *ClusterState) getStorageElementTypeData(ctx context.Context, el *models.StoragePlanStorageElement) (*storageElementTypeData, error) {
	spID := string(el.PoolID)
	cst, err := cs.GetPoolStorageType(ctx, spID)
	if err != nil {
		return nil, err
	}
	asb := swag.Int64Value(cst.PreferredAllocationSizeBytes)
	psb := swag.Int64Value(cst.ParcelSizeBytes)
	szb := swag.Int64Value(el.SizeBytes)
	mab := swag.Int64Value(cst.MaxAllocationSizeBytes)
	mib := swag.Int64Value(cst.MinAllocationSizeBytes)
	td := &storageElementTypeData{
		PoolID:                           spID,
		StorageType:                      cst,
		PreferredAllocationSizeBytes:     asb,
		MaxSizeBytes:                     mab,
		MinSizeBytes:                     mib,
		ParcelSizeBytes:                  psb,
		PreferredAllocationUnitSizeBytes: swag.Int64Value(cst.PreferredAllocationUnitSizeBytes),
		MaxParcelsFixedSizedStorage:      (asb - nuvoapi.DefaultDeviceFormatOverheadBytes) / psb,
		MaxParcelsVariableSizedStorage:   (mab - nuvoapi.DefaultDeviceFormatOverheadBytes) / psb,
		MinParcelsVariableSizedStorage:   (mib - nuvoapi.DefaultDeviceFormatOverheadBytes) / psb,
		ElementSizeBytes:                 szb,
	}
	return td, nil
}

// StorageSelector provides an interface to acquire storage
type StorageSelector interface {
	FillStorageElement(ctx context.Context, elNum int, el *models.StoragePlanStorageElement, td *storageElementTypeData, es *ElementStorage) error
}

// SelectStorage selects new or existing storage for a storage plan.
func (cs *ClusterState) SelectStorage(ctx context.Context, args *SelectStorageArgs) (*SelectStorageResponse, error) {
	if args.VSR == nil || args.VS == nil {
		return nil, fmt.Errorf("missing arguments")
	}
	plan := args.VSR.StoragePlan
	resp := &SelectStorageResponse{}
	resp.Elements = make([]ElementStorage, len(plan.StorageElements))
	resp.LA = args.LA
	if resp.LA == nil {
		resp.LA = cs.FindLayoutAlgorithm(plan.StorageLayout)
		if resp.LA == nil {
			return nil, fmt.Errorf("no placement algorithm for layout '%s'", plan.StorageLayout)
		}
	} else if resp.LA.StorageLayout != plan.StorageLayout {
		return nil, fmt.Errorf("incompatible placement algorithm '%s' for layout '%s'", resp.LA.Name, plan.StorageLayout)
	}
	var ss StorageSelector
	var err error
	switch plan.StorageLayout {
	case models.StorageLayoutStandalone:
		ss, err = newStandaloneStorageSelector(args, cs, resp)
	default:
		err = fmt.Errorf("unsupported storage layout '%s'", plan.StorageLayout)
	}
	if err != nil {
		return nil, err
	}
	if err = cs.fillStorageElements(ctx, args, plan, ss, resp); err != nil {
		return nil, err
	}
	if b, err := json.Marshal(resp); err == nil {
		cs.Log.Debugf("VolumeSeriesRequest %s: StoragePlan: %s", args.VSR.Meta.ID, b)
	}
	return resp, nil
}

func (cs *ClusterState) fillStorageElements(ctx context.Context, args *SelectStorageArgs, plan *models.StoragePlan, ss StorageSelector, resp *SelectStorageResponse) error {
	for i, el := range plan.StorageElements {
		if el.Intent == com.VolReqStgElemIntentCache {
			continue
		}
		td, err := cs.getStorageElementTypeData(ctx, el)
		if err != nil {
			cs.Log.Debugf("VolumeSeriesRequest: %s[%d] Unable to determine pool storage type: %s", args.VSR.Meta.ID, i, err.Error())
			return err
		}
		es := &resp.Elements[i]
		if err = ss.FillStorageElement(ctx, i, el, td, es); err != nil {
			return err
		}
		resp.Elements[i] = *es
	}
	return nil
}
