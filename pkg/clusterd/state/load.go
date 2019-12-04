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
	"bytes"
	"context"
	"fmt"
	"reflect"
	"sort"
	"time"

	mcn "github.com/Nuvoloso/kontroller/pkg/autogen/client/node"
	mcs "github.com/Nuvoloso/kontroller/pkg/autogen/client/storage"
	mcsr "github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_request"
	mcvsr "github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
)

var storageRequestTerminalStates = []string{com.StgReqStateSucceeded, com.StgReqStateFailed}

// Reload cluster state.  Prime the SR cache if necessary.
func (cs *ClusterState) Reload(ctx context.Context) (bool, error) {
	var err error
	if err = cs.primeSRCache(ctx); err != nil {
		return false, err
	}
	var nM map[string]*NodeData
	var sM map[string]*models.Storage
	var srM map[string]*models.StorageRequest
	dsM := map[string]*models.Storage{}
	if nM, err = cs.loadNodes(ctx); err == nil {
		// load SRs before S to ensure that we see the Storage provisioned by a completed SR and know those being released
		if srM, err = cs.loadStorageRequests(ctx); err == nil {
			sM, err = cs.loadStorage(ctx)
		}
	}
	if err != nil {
		return false, err
	}
	// Collect detached storage and attached storage separately; in the latter case
	// discard storage that is either referenced by an active SR or is not usable.
	// Detached storage arises in recovery situations; it may or may not be
	// in use by Volumes, and may be referenced by SRs used to recover.
	for k, s := range sM {
		ss := s.StorageState
		if ss.AttachmentState == com.StgAttachmentStateDetached {
			cs.Log.Debugf("Detached Storage[%s] found [A:%s,D:%s,M:%s]", k, ss.AttachmentState, ss.DeviceState, ss.MediaState)
			dsM[k] = s
			delete(sM, k)
			continue
		}
		if !(ss.AttachmentState == com.StgAttachmentStateAttached && ss.MediaState == com.StgMediaStateFormatted) {
			cs.Log.Debugf("Skipping Storage[%s] not ready [A:%s,D:%s,M:%s]", k, ss.AttachmentState, ss.DeviceState, ss.MediaState)
			delete(sM, k)
			continue
		}
		for _, sr := range srM {
			if k == string(sr.StorageID) {
				if !util.Contains(storageRequestTerminalStates, sr.StorageRequestState) {
					cs.Log.Debugf("Skipping Storage[%s] referenced by active SR[%s]", k, sr.Meta.ID)
					delete(sM, k)
				}
				break
			}
		}
	}
	// assemble the new state
	newCS := NewClusterState(&cs.ClusterStateArgs)
	newCS.claimedStorage = make(map[string]struct{})
	for srID, srObj := range srM { // recreate claims from storage requests
		if util.Contains(srObj.RequestedOperations, com.StgReqOpDetach) {
			continue // no claims
		}
		newCD := &ClaimData{
			StorageRequest: srObj,
			PoolID:         string(srObj.PoolID),
			NodeID:         string(srObj.NodeID),
			RemainingBytes: swag.Int64Value(srObj.MinSizeBytes),
		}
		if cd, present := cs.StorageRequests[srID]; present {
			newCD.VSRClaims = cd.VSRClaims
			newCD.RemainingBytes = cd.RemainingBytes
		}
		if len(newCD.VSRClaims) > 0 {
			newCS.StorageRequests[srID] = newCD
		}
		if srObj.StorageID != "" {
			newCS.claimedStorage[string(srObj.StorageID)] = struct{}{}
		}
	}
	// update the attached node storage
	newCS.Nodes = nM
	newCS.Storage = sM
	newCS.DetachedStorage = dsM
	for sID, sObj := range newCS.Storage {
		nID := sObj.StorageState.AttachedNodeID
		if nD, ok := newCS.Nodes[string(nID)]; ok {
			nD.AttachedStorage = append(nD.AttachedStorage, sID)
		}
	}
	cs.NumLoads++
	changed := cs.stateChanged(newCS)
	if changed {
		cs.Nodes = newCS.Nodes
		cs.Storage = newCS.Storage
		cs.DetachedStorage = newCS.DetachedStorage
		cs.StorageRequests = newCS.StorageRequests
		cs.claimedStorage = newCS.claimedStorage
	}
	if cs.LoadCountOnInternalChange == cs.NumLoads-1 {
		changed = true // internal state may not have been dumped so pretend there was a change
	}
	return changed, nil
}

func (cs *ClusterState) stateChanged(ncs *ClusterState) bool {
	if len(cs.Nodes) != len(ncs.Nodes) || len(cs.Storage) != len(ncs.Storage) || len(cs.DetachedStorage) != len(ncs.DetachedStorage) || len(cs.StorageRequests) != len(ncs.StorageRequests) {
		return true
	}
	// compare SR state and claims (SR claim lengths identical)
	for srID, ncd := range ncs.StorageRequests {
		ocd, ok := cs.StorageRequests[srID]
		if !ok {
			return true
		}
		if ocd.StorageRequest.StorageRequestState != ncd.StorageRequest.StorageRequestState {
			return true
		}
		if len(ncd.VSRClaims) != len(ocd.VSRClaims) {
			return true
		}
		oV := make([]string, len(ocd.VSRClaims))
		for i, v := range ocd.VSRClaims {
			oV[i] = v.RequestID
		}
		sort.Strings(oV)
		nV := make([]string, len(ncd.VSRClaims))
		for i, v := range ncd.VSRClaims {
			nV[i] = v.RequestID
		}
		sort.Strings(nV)
		if !reflect.DeepEqual(nV, oV) {
			return true
		}
		// id match sufficient as VSRClaims are carried over
	}
	// compare storage ids (map lengths identical)
	for sID, nS := range ncs.Storage {
		oS, ok := cs.Storage[sID]
		if !ok {
			return true
		}
		nSS := nS.StorageState
		oSS := oS.StorageState
		if swag.Int64Value(nS.AvailableBytes) != swag.Int64Value(oS.AvailableBytes) ||
			nSS.DeviceState != oSS.DeviceState || nSS.MediaState != oSS.MediaState || nSS.AttachmentState != oSS.AttachmentState {
			cs.Log.Debugf("Storage state changed: o=%v\nn=%v", oSS, nSS)
			return true
		}
	}
	// compare detached storage ids (map lengths identical)
	for sID := range ncs.DetachedStorage {
		_, ok := cs.DetachedStorage[sID]
		if !ok {
			return true
		}
	}
	// compare node ids (map lengths identical)
	for nID, nN := range ncs.Nodes {
		oN, ok := cs.Nodes[nID]
		if !ok {
			return true
		}
		nSS := nN.Node.Service.ServiceState
		oSS := oN.Node.Service.ServiceState
		if nSS.State != oSS.State {
			cs.Log.Debugf("Node state changed: o=%v\nn=%v", oSS, nSS)
			return true
		}
	}
	// compare attached storage
	for nID, nNd := range ncs.Nodes {
		sort.Strings(nNd.AttachedStorage)
		oNd := cs.Nodes[nID]
		sort.Strings(oNd.AttachedStorage)
		if !reflect.DeepEqual(nNd.AttachedStorage, oNd.AttachedStorage) {
			return true
		}
	}
	return false
}

func storageDescription(s *models.Storage) string {
	ss := s.StorageState
	psb := swag.Int64Value(s.ParcelSizeBytes)
	ab := swag.Int64Value(s.AvailableBytes)
	var ap string
	if psb <= 0 {
		ap = "?"
	} else {
		ap = fmt.Sprintf("%d", ab/psb)
	}
	return fmt.Sprintf("T:'%s' TSz:%s ASz:%s PSz:%s TP:%d AP:%s DS:%s MS:%s AS:%s D:%s",
		s.CspStorageType,
		util.SizeBytes(swag.Int64Value(s.SizeBytes)), util.SizeBytes(ab),
		util.SizeBytes(psb), swag.Int64Value(s.TotalParcelCount), ap,
		ss.DeviceState, ss.MediaState, ss.AttachmentState, ss.AttachedNodeDevice)
}

// DumpState returns a printable dump of the state
func (cs *ClusterState) DumpState() *bytes.Buffer {
	var buf bytes.Buffer
	var b = &buf
	fmt.Fprintf(b, "Iteration:%d #Nodes:%d #Storage:%d #SR:%d\n", cs.NumLoads, len(cs.Nodes), len(cs.Storage)+len(cs.DetachedStorage), len(cs.StorageRequests))
	now := time.Now()
	nodeKeys := util.SortedStringKeys(cs.Nodes)
	for _, nk := range nodeKeys {
		nd := cs.Nodes[nk]
		cs.NodeUpdateState(nd.Node, now)
		fmt.Fprintf(b, "- Node[%s] %s #Attached: %d\n", nd.Node.Meta.ID, nd.Node.Service.ServiceState.State, len(nd.AttachedStorage))
		sort.Strings(nd.AttachedStorage)
		for _, sID := range nd.AttachedStorage {
			s := cs.Storage[sID] // guaranteed in cache based on how AttachedStorage is created
			fmt.Fprintf(b, "   S[%s] %s\n", sID, storageDescription(s))
		}
	}
	srIDS := util.SortedStringKeys(cs.StorageRequests)
	for _, srID := range srIDS {
		cd := cs.StorageRequests[srID]
		var nodeName string
		if n, ok := cs.Nodes[cd.NodeID]; ok {
			nodeName = "'" + string(n.Node.Name) + "'"
		} else { // just in case not in cache
			nodeName = "[" + cd.NodeID + "]"
		}
		fmt.Fprintf(b, "- SR[%s] N:%s T:'%s' RB:%s #Claims:%d\n", srID, nodeName, cd.StorageRequest.CspStorageType, util.SizeBytes(cd.RemainingBytes), len(cd.VSRClaims))
		for _, v := range cd.VSRClaims {
			fmt.Fprintf(b, "   VSR[%s] %s\n", v.RequestID, util.SizeBytes(v.SizeBytes))
		}
	}
	dsIDs := util.SortedStringKeys(cs.DetachedStorage)
	for _, sID := range dsIDs {
		s := cs.DetachedStorage[sID]
		fmt.Fprintf(b, "- S[%s] %s\n", sID, storageDescription(s))

	}
	return b
}

func (cs *ClusterState) loadNodes(ctx context.Context) (map[string]*NodeData, error) {
	// TBD: this could come from a cache in future
	var err error
	var nRes *mcn.NodeListOK
	nl := mcn.NewNodeListParams()
	nl.ClusterID = swag.String(string(cs.Cluster.Meta.ID))
	if nRes, err = cs.OCrud.NodeList(ctx, nl); err != nil {
		return nil, err
	}
	nM := make(map[string]*NodeData)
	for _, nObj := range nRes.Payload {
		nd := &NodeData{Node: nObj, AttachedStorage: []string{}}
		nM[string(nObj.Meta.ID)] = nd
	}
	return nM, nil
}

func (cs *ClusterState) loadStorage(ctx context.Context) (map[string]*models.Storage, error) {
	var err error
	var sRes *mcs.StorageListOK
	// load storage associated with the cluster
	sl := mcs.NewStorageListParams()
	sl.ClusterID = swag.String(string(cs.Cluster.Meta.ID))
	if sRes, err = cs.OCrud.StorageList(ctx, sl); err != nil {
		return nil, err
	}
	sM := make(map[string]*models.Storage)
	for _, sObj := range sRes.Payload {
		sM[string(sObj.Meta.ID)] = sObj
	}
	return sM, nil
}

func (cs *ClusterState) loadStorageRequests(ctx context.Context) (map[string]*models.StorageRequest, error) {
	// First load all active SRs, then individually the inactive ones we're interested in
	var err error
	var srRes *mcsr.StorageRequestListOK
	srl := mcsr.NewStorageRequestListParams()
	srl.ClusterID = swag.String(string(cs.Cluster.Meta.ID))
	srl.IsTerminated = swag.Bool(false)
	if srRes, err = cs.OCrud.StorageRequestList(ctx, srl); err != nil {
		return nil, err
	}
	srM := make(map[string]*models.StorageRequest)
	for _, srObj := range srRes.Payload {
		srID := string(srObj.Meta.ID)
		if util.Contains(srObj.RequestedOperations, com.StgReqOpDetach) {
			// must track release attempts so we can drop the concerned Storage
			cs.Log.Debugf("Found release SR %s", srID)
		} else if _, wanted := cs.StorageRequests[srID]; !util.Contains(srObj.RequestedOperations, com.StgReqOpAttach) && !wanted {
			cs.Log.Debugf("Ignoring SR %s: wanted=%v ops:%v", srID, wanted, srObj.RequestedOperations)
			continue
		}
		srM[srID] = srObj
	}
	for srID, cd := range cs.StorageRequests {
		if _, has := srM[srID]; has || len(cd.VSRClaims) == 0 {
			continue // already loaded or not wanted any more
		}
		srObj, err := cs.OCrud.StorageRequestFetch(ctx, srID)
		if err != nil {
			return nil, err
		}
		srM[srID] = srObj
	}
	return srM, nil
}

// primeSRCache should be called on startup to prime the SR cache.
// It does nothing if the cache is already primed.
func (cs *ClusterState) primeSRCache(ctx context.Context) error {
	if cs.srCacheIsPrimed {
		return nil
	}
	// fetch active VSRs
	vsrLP := mcvsr.NewVolumeSeriesRequestListParams()
	vsrLP.ClusterID = swag.String(string(cs.Cluster.Meta.ID))
	vsrLP.IsTerminated = swag.Bool(false)
	vsrLRet, err := cs.OCrud.VolumeSeriesRequestList(ctx, vsrLP)
	if err != nil {
		return err
	}
	// fetch associated SRs for VSRs in the PLACEMENT, PLACEMENT_REATTACH or STORAGE_WAIT states
	for _, vsr := range vsrLRet.Payload {
		if !(vsr.VolumeSeriesRequestState == com.VolReqStatePlacement ||
			vsr.VolumeSeriesRequestState == com.VolReqStatePlacementReattach ||
			vsr.VolumeSeriesRequestState == com.VolReqStateStorageWait) {
			continue
		}
		srLP := mcsr.NewStorageRequestListParams()
		srLP.VolumeSeriesRequestID = swag.String(string(vsr.Meta.ID))
		srLRet, err := cs.OCrud.StorageRequestList(ctx, srLP)
		if err != nil {
			return err
		}
		for _, sr := range srLRet.Payload {
			cs.TrackStorageRequest(sr)
		}
	}
	cs.srCacheIsPrimed = true
	return nil
}
