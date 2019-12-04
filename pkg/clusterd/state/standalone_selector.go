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
	"sort"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/layout"
	"github.com/go-openapi/swag"
)

type standaloneIterState int

const (
	saIterFindLocalShareableStorage standaloneIterState = iota
	saIterServeLocalShareableStorage
	saIterFindLocalClaims
	saIterServeLocalClaims
	saIterFindRemoteShareableStorage
	saIterServeRemoteShareableStorage
	saIterFindRemoteClaims
	saIterServeRemoteClaims
	saIterProvisionLocalFixedSizeStorage
	saIterFindLocalUnusedStorage
	saIterServeLocalUnusedStorage
	saIterProvisionLocalVariableSizeStorage

	saIterLast // positional indicator, not a state
)

func (s standaloneIterState) String() string {
	switch s {
	case saIterFindLocalShareableStorage:
		return "saIterFindLocalShareableStorage"
	case saIterServeLocalShareableStorage:
		return "saIterServeLocalShareableStorage"
	case saIterFindLocalClaims:
		return "saIterFindLocalClaims"
	case saIterServeLocalClaims:
		return "saIterServeLocalClaims"
	case saIterFindRemoteShareableStorage:
		return "saIterFindRemoteShareableStorage"
	case saIterServeRemoteShareableStorage:
		return "saIterServeRemoteShareableStorage"
	case saIterFindRemoteClaims:
		return "saIterFindRemoteClaims"
	case saIterServeRemoteClaims:
		return "saIterServeRemoteClaims"
	case saIterProvisionLocalFixedSizeStorage:
		return "saIterProvisionLocalFixedSizeStorage"
	case saIterFindLocalUnusedStorage:
		return "saIterFindLocalUnusedStorage"
	case saIterServeLocalUnusedStorage:
		return "saIterServeLocalUnusedStorage"
	case saIterProvisionLocalVariableSizeStorage:
		return "saIterProvisionLocalVariableSizeStorage"
	}
	return "UNKNOWN"
}

type standaloneSelector struct {
	SelectStorageArgs
	CS           *ClusterState
	LA           *layout.Algorithm
	Plan         *models.StoragePlan
	Response     *SelectStorageResponse
	NodeID       string
	TD           *storageElementTypeData
	stateMachine []standaloneIterState
	iterStorage  []*models.Storage
	iterClaims   ClaimList
	iterPos      int
}

func newStandaloneStorageSelector(args *SelectStorageArgs, cs *ClusterState, resp *SelectStorageResponse) (StorageSelector, error) {
	if args == nil || args.VSR == nil || args.VS == nil || cs == nil || resp == nil || resp.Elements == nil || resp.LA == nil {
		return nil, fmt.Errorf("missing arguments")
	}
	if args.VSR.StoragePlan.StorageLayout != models.StorageLayoutStandalone {
		return nil, fmt.Errorf("invalid layout %s", args.VSR.StoragePlan.StorageLayout)
	}
	ss := &standaloneSelector{}
	ss.SelectStorageArgs = *args
	ss.CS = cs
	ss.LA = resp.LA
	ss.Plan = args.VSR.StoragePlan
	ss.NodeID = string(args.VSR.NodeID)
	ss.Response = resp
	ss.stateMachine = ss.buildStateMachine(ss.LA)
	return ss, nil
}

func (ss *standaloneSelector) FillStorageElement(ctx context.Context, elNum int, el *models.StoragePlanStorageElement, td *storageElementTypeData, es *ElementStorage) error {
	ss.TD = td
	es.ParcelSizeBytes = td.ParcelSizeBytes
	numParcelsNeeded := td.SizeBytesToParcels(td.ElementSizeBytes)
	for numParcelsNeeded > 0 {
		item := ss.nextStorageItem(numParcelsNeeded)
		es.Items = append(es.Items, *item)
		numParcelsNeeded -= item.NumParcels
	}
	es.NumItems = len(es.Items)
	return nil
}

func (ss *standaloneSelector) buildStateMachine(la *layout.Algorithm) []standaloneIterState {
	sm := make([]standaloneIterState, 0, saIterLast)
	if la.SharedStorageOk {
		sm = append(sm, saIterFindLocalShareableStorage, saIterServeLocalShareableStorage)
		sm = append(sm, saIterFindLocalClaims, saIterServeLocalClaims)
	} else {
		sm = append(sm, saIterFindLocalUnusedStorage, saIterServeLocalUnusedStorage)
	}
	if la.RemoteStorageOk {
		if la.SharedStorageOk {
			sm = append(sm, saIterFindRemoteShareableStorage, saIterServeRemoteShareableStorage)
			sm = append(sm, saIterFindRemoteClaims, saIterServeRemoteClaims)
		}
	}
	if la.VariableSizeStorageOk {
		sm = append(sm, saIterProvisionLocalVariableSizeStorage)
	} else {
		sm = append(sm, saIterProvisionLocalFixedSizeStorage)
	}
	ss.CS.Log.Debugf("iter StateMachine for %s: %v", la.Name, sm)
	return sm
}

func (ss *standaloneSelector) nextStorageItem(numParcelsNeeded int64) *StorageItem {
	td := ss.TD
	for len(ss.stateMachine) > 0 {
		currentState := ss.stateMachine[0]
		ss.CS.Log.Debugf("iterState: %s", currentState)
		switch currentState {
		case saIterFindLocalShareableStorage:
			ss.iterStorage = ss.findLocalShareableStorageByPoolOrdered()
			ss.iterPos = 0
		case saIterServeLocalShareableStorage:
			if ss.iterPos < len(ss.iterStorage) {
				sObj, np := ss.getCapacityFromIterStorage("LShS", numParcelsNeeded)
				ss.iterPos++
				return td.MakeShareableStorageItem(np, sObj, 0, ss.NodeID, "")
			}
		case saIterFindLocalClaims:
			ss.iterClaims = ss.findLocalClaimsByPoolOrdered()
			ss.iterPos = 0
		case saIterServeLocalClaims:
			if ss.iterPos < len(ss.iterClaims) {
				cd, np := ss.getCapacityFromIterClaims("LC", numParcelsNeeded)
				ss.iterPos++
				return td.MakeShareableStorageItem(np, nil, 0, cd.NodeID, string(cd.StorageRequest.Meta.ID))
			}
		case saIterFindRemoteShareableStorage:
			ss.iterStorage = ss.findRemoteStorageByPoolOrdered()
			ss.iterPos = 0
		case saIterServeRemoteShareableStorage:
			if ss.iterPos < len(ss.iterStorage) {
				sObj, np := ss.getCapacityFromIterStorage("RShS", numParcelsNeeded)
				ss.iterPos++
				return td.MakeShareableStorageItem(np, sObj, 0, string(sObj.StorageState.AttachedNodeID), "")
			}
		case saIterFindRemoteClaims:
			ss.iterClaims = ss.findRemoteClaimsByPoolOrdered()
			ss.iterPos = 0
		case saIterServeRemoteClaims:
			if ss.iterPos < len(ss.iterClaims) {
				cd, np := ss.getCapacityFromIterClaims("RC", numParcelsNeeded)
				ss.iterPos++
				return td.MakeShareableStorageItem(np, nil, 0, string(cd.NodeID), string(cd.StorageRequest.Meta.ID))
			}
		case saIterProvisionLocalFixedSizeStorage:
			return ss.provisionFixedSizeStorage(numParcelsNeeded)
		case saIterFindLocalUnusedStorage:
			ss.iterStorage = ss.findUnsharedStorageByPoolOrdered(numParcelsNeeded)
			ss.iterPos = 0
		case saIterServeLocalUnusedStorage:
			if ss.iterPos < len(ss.iterStorage) {
				sObj, np := ss.getCapacityFromIterStorage("LUnS", numParcelsNeeded)
				ss.iterPos++
				return td.MakeUnsharedStorageItem(np, sObj, ss.NodeID, "")
			}
		case saIterProvisionLocalVariableSizeStorage:
			return ss.provisionVariableSizeStorage(numParcelsNeeded)
		}
		ss.stateMachine = ss.stateMachine[1:]
	}
	panic("standaloneIter state machine exhausted")
}

// getCapacityFromIterStorage gets capacity from the iter Storage at the head of the list (which must not be empty)
// The position is not changed.
func (ss *standaloneSelector) getCapacityFromIterStorage(which string, numParcelsNeeded int64) (*models.Storage, int64) {
	sObj := ss.iterStorage[ss.iterPos]
	availP := ss.TD.SizeBytesToParcels(swag.Int64Value(sObj.AvailableBytes))
	np := numParcelsNeeded
	if np > availP {
		np = availP
	}
	ss.CS.Log.Debugf("iter(%s): P:[%s] S:[%s] N:[%s] (Ask:%d Avail:%d Took:%d)", which, ss.TD.PoolID, sObj.Meta.ID, sObj.StorageState.AttachedNodeID, numParcelsNeeded, availP, np)
	return sObj, np
}

// getCapacityFromIterClaims gets capacity from the iter claim datum at the head of the list (which must not be empty)
// The position is not changed.
func (ss *standaloneSelector) getCapacityFromIterClaims(which string, numParcelsNeeded int64) (*ClaimData, int64) {
	cd := ss.iterClaims[ss.iterPos]
	availP := ss.TD.SizeBytesToParcels(cd.RemainingBytes)
	np := numParcelsNeeded
	if np > availP {
		np = availP
	}
	ss.CS.Log.Debugf("iter(%s): P:[%s] SR:[%s] N:[%s] (Ask:%d Avail:%d Took:%d)", which, ss.TD.PoolID, cd.StorageRequest.Meta.ID, cd.NodeID, numParcelsNeeded, availP, np)
	return cd, np
}

func (ss *standaloneSelector) provisionFixedSizeStorage(numParcelsNeeded int64) *StorageItem {
	td := ss.TD
	np := numParcelsNeeded
	remB := int64(0)
	if np > td.MaxParcelsFixedSizedStorage {
		np = td.MaxParcelsFixedSizedStorage
	} else {
		remB = (td.MaxParcelsFixedSizedStorage - np) * td.ParcelSizeBytes
	}
	si := td.MakeShareableStorageItem(np, nil, remB, ss.NodeID, "")
	ss.CS.Log.Debugf("iter(PFS): P:[%s] N:[%s] (Ask:%d Avail:0 Created:%d Took:%d)", td.PoolID, ss.NodeID, numParcelsNeeded, si.MinSizeBytes/td.ParcelSizeBytes, si.NumParcels)
	return si
}

func (ss *standaloneSelector) findLocalShareableStorageByPoolOrdered() []*models.Storage {
	td := ss.TD
	sl := ss.CS.FindShareableStorageByPool(td.PoolID, td.ParcelSizeBytes, ss.NodeID, false) // at least 1 parcel
	if len(sl) > 1 {
		sort.Slice(sl, func(i, j int) bool {
			return swag.Int64Value(sl[i].AvailableBytes) > swag.Int64Value(sl[j].AvailableBytes)
		})
	}
	return sl
}

func (ss *standaloneSelector) findLocalClaimsByPoolOrdered() ClaimList {
	td := ss.TD
	cl := ss.CS.FindClaimsByPool(td.PoolID, td.ParcelSizeBytes, ss.NodeID, false)
	if len(cl) > 1 {
		sort.Slice(cl, func(i, j int) bool {
			return cl[i].RemainingBytes > cl[j].RemainingBytes
		})
	}
	return cl
}

func (ss *standaloneSelector) findRemoteStorageByPoolOrdered() []*models.Storage {
	td := ss.TD
	sl := ss.CS.FindShareableStorageByPool(td.PoolID, td.ParcelSizeBytes, ss.NodeID, true) // at least 1 parcel
	if len(sl) > 1 {
		sort.Slice(sl, func(i, j int) bool {
			return swag.Int64Value(sl[i].AvailableBytes) > swag.Int64Value(sl[j].AvailableBytes)
		})
	}
	return sl
}

func (ss *standaloneSelector) findRemoteClaimsByPoolOrdered() ClaimList {
	td := ss.TD
	cl := ss.CS.FindClaimsByPool(td.PoolID, td.ParcelSizeBytes, ss.NodeID, true)
	if len(cl) > 1 {
		sort.Slice(cl, func(i, j int) bool {
			return cl[i].RemainingBytes > cl[j].RemainingBytes
		})
	}
	return cl
}

// findUnsharedStorageByPoolOrdered returns the list of suitable unused Storage objects.
// The returned list is of the form:
//   [MaxSizeStorage+] [<MaxSizedStorage]
// i.e. a sequence of maximally sized Storage objects followed by at most 2 non-maximal sized object.
func (ss *standaloneSelector) findUnsharedStorageByPoolOrdered(numParcelsNeeded int64) []*models.Storage {
	td := ss.TD
	maxCount, rem1, rem2 := ss.computeMinimalVariableStorage(numParcelsNeeded)
	retList := []*models.Storage{}
	if maxCount > 0 {
		l := ss.CS.FindUnusedStorageByPool(td.PoolID, td.MaxParcelsVariableSizedStorage*td.ParcelSizeBytes, ss.NodeID, false)
		for i := 0; i < len(l) && i < int(maxCount); i++ {
			retList = append(retList, l[i])
		}
	}
	if rem1 > 0 { // add the first split
		l := ss.CS.FindUnusedStorageByPool(td.PoolID, rem1*td.ParcelSizeBytes, ss.NodeID, false)
		need := 1
		if rem2 == rem1 { // even split - don't search again
			need++
		}
		for i := 0; i < len(l) && i < need; i++ {
			retList = append(retList, l[i])
		}
	}
	if rem2 > 0 && rem2 != rem1 { // add the second split if necessary
		l := ss.CS.FindUnusedStorageByPool(td.PoolID, rem2*td.ParcelSizeBytes, ss.NodeID, false)
		if len(l) > 0 {
			retList = append(retList, l[0]) // just the one
		}
	}
	return retList
}

// provisionVariableSizeStorage returns the size of the next unshared Storage object to provision.
// It attempts to minimize the number of Storage objects used and wastage.
func (ss *standaloneSelector) provisionVariableSizeStorage(numParcelsNeeded int64) *StorageItem {
	td := ss.TD
	maxCount, rem1, _ := ss.computeMinimalVariableStorage(numParcelsNeeded)
	np := td.MaxParcelsVariableSizedStorage
	if maxCount == 0 {
		np = rem1
	}
	si := td.MakeUnsharedStorageItem(np, nil, ss.NodeID, "")
	ss.CS.Log.Debugf("iter(PVU): P:[%s] N:[%s] (Ask:%d Avail:0 Created:%d Took:%d)", td.PoolID, ss.NodeID, numParcelsNeeded, si.MinSizeBytes/td.ParcelSizeBytes, si.NumParcels)
	return si
}

// computeMinimalVariableStorage returns the minimum number of maximal sized Storage objects needed
// and the number of remaining non maximal Storage parcels necessary to provision numParcelsNeeded.
// We got to ensure that there is no wastage on the non-maximal Storage so we may require the use of
// 2 non-maximal Storage objects.
func (ss *standaloneSelector) computeMinimalVariableStorage(numParcelsNeeded int64) (int64, int64, int64) {
	td := ss.TD
	maxCount := int64(0)
	parcelsAllocated := int64(0)
	for parcelsAllocated+td.MaxParcelsVariableSizedStorage <= numParcelsNeeded {
		maxCount++
		parcelsAllocated += td.MaxParcelsVariableSizedStorage
	}
	rem1 := numParcelsNeeded - parcelsAllocated
	rem2 := int64(0)
	if rem1 > 0 && rem1 < td.MinParcelsVariableSizedStorage { // it is guaranteed that min size can contain >= 1 parcel
		if maxCount > 0 {
			maxCount--                                                 // split last maximal with rem1
			combinedSize := (td.MaxParcelsVariableSizedStorage + rem1) // to avoid wastage
			rem2 = combinedSize / 2
			rem1 = combinedSize - rem2
		} // else case when invoked with < min size
	}
	return maxCount, rem1, rem2
}
