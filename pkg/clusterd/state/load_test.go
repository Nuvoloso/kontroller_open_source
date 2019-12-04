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
	"strings"
	"testing"

	mcn "github.com/Nuvoloso/kontroller/pkg/autogen/client/node"
	mcs "github.com/Nuvoloso/kontroller/pkg/autogen/client/storage"
	mcsr "github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_request"
	mcvsr "github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/alecthomas/units"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func cloneNDM(src map[string]*NodeData) map[string]*NodeData {
	var copy map[string]*NodeData
	testutils.Clone(src, &copy)
	return copy
}

func cloneClaimM(src map[string]*ClaimData) map[string]*ClaimData {
	var copy map[string]*ClaimData
	testutils.Clone(src, &copy)
	for k, v := range src { // copy unexported fields
		d := copy[k]
		d.cs = v.cs
	}
	return copy
}

func TestReload(t *testing.T) {
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

	nRes := &mcn.NodeListOK{
		Payload: []*models.Node{
			&models.Node{
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
			},
			&models.Node{
				NodeAllOf0: models.NodeAllOf0{
					Meta: &models.ObjMeta{ID: "NODE-2", Version: 2},
				},
				NodeMutable: models.NodeMutable{
					Name: "Node2",
					Service: &models.NuvoService{
						ServiceState: models.ServiceState{
							State: "READY",
						},
					},
				},
			},
		},
	}
	sRes := &mcs.StorageListOK{
		Payload: []*models.Storage{
			&models.Storage{
				StorageAllOf0: models.StorageAllOf0{
					Meta:           &models.ObjMeta{ID: "STORAGE-1", Version: 2},
					CspStorageType: "Amazon GP2",
					PoolID:         "SP-GP2",
					SizeBytes:      swag.Int64(int64(3 * units.GiB)),
				},
				StorageMutable: models.StorageMutable{
					StorageState: &models.StorageStateMutable{
						AttachedNodeID:     "NODE-1",
						AttachedNodeDevice: "/dev/xvdba",
						DeviceState:        "UNUSED",
						MediaState:         "FORMATTED",
						AttachmentState:    "ATTACHED",
					},
				},
			},
			&models.Storage{
				StorageAllOf0: models.StorageAllOf0{
					Meta:           &models.ObjMeta{ID: "STORAGE-2", Version: 2},
					CspStorageType: "Amazon SSD",
					PoolID:         "SP-SSD",
					SizeBytes:      swag.Int64(int64(10 * units.GiB)),
				},
				StorageMutable: models.StorageMutable{
					ParcelSizeBytes: swag.Int64(int64(512 * units.MiB)),
					AvailableBytes:  swag.Int64(int64(8 * units.GiB)),
					StorageState: &models.StorageStateMutable{
						AttachedNodeID:     "NODE-1",
						AttachedNodeDevice: "/dev/xvdbb",
						DeviceState:        "UNUSED",
						MediaState:         "FORMATTED",
						AttachmentState:    "ATTACHED",
					},
				},
			},
			&models.Storage{ // ignored - invalid state
				StorageAllOf0: models.StorageAllOf0{
					Meta:           &models.ObjMeta{ID: "STORAGE-NR-1", Version: 2},
					CspStorageType: "Amazon SSD",
					PoolID:         "SP-SSD",
					SizeBytes:      swag.Int64(int64(10 * units.GiB)),
				},
				StorageMutable: models.StorageMutable{
					ParcelSizeBytes: swag.Int64(int64(512 * units.MiB)),
					AvailableBytes:  swag.Int64(int64(8 * units.GiB)),
					StorageState: &models.StorageStateMutable{
						AttachedNodeID:     "NODE-1",
						AttachedNodeDevice: "/dev/xvdbc",
						DeviceState:        "UNUSED",
						MediaState:         "FORMATTED",
						AttachmentState:    "ATTACHING",
					},
				},
			},
			&models.Storage{ // ignored - invalid state
				StorageAllOf0: models.StorageAllOf0{
					Meta:           &models.ObjMeta{ID: "STORAGE-NR-2", Version: 2},
					CspStorageType: "Amazon SSD",
					PoolID:         "SP-SSD",
					SizeBytes:      swag.Int64(int64(10 * units.GiB)),
				},
				StorageMutable: models.StorageMutable{
					ParcelSizeBytes: swag.Int64(int64(512 * units.MiB)),
					AvailableBytes:  swag.Int64(int64(8 * units.GiB)),
					StorageState: &models.StorageStateMutable{
						AttachedNodeID:     "NODE-1",
						AttachedNodeDevice: "/dev/xvdbd",
						DeviceState:        "UNUSED",
						MediaState:         "UNFORMATTED",
						AttachmentState:    "ATTACHED",
					},
				},
			},
			&models.Storage{ // ignored - referenced by SR-2
				StorageAllOf0: models.StorageAllOf0{
					Meta:           &models.ObjMeta{ID: "STORAGE-NR-3", Version: 2},
					CspStorageType: "Amazon SSD",
					PoolID:         "SP-SSD",
					SizeBytes:      swag.Int64(int64(10 * units.GiB)),
				},
				StorageMutable: models.StorageMutable{
					ParcelSizeBytes: swag.Int64(int64(512 * units.MiB)),
					AvailableBytes:  swag.Int64(int64(8 * units.GiB)),
					StorageState: &models.StorageStateMutable{
						AttachedNodeID:     "NODE-1",
						AttachedNodeDevice: "/dev/xvdbe",
						DeviceState:        "UNUSED",
						MediaState:         "FORMATTED",
						AttachmentState:    "ATTACHED",
					},
				},
			},
			&models.Storage{ // ignored - referenced by SR-RELEASING
				StorageAllOf0: models.StorageAllOf0{
					Meta:           &models.ObjMeta{ID: "STORAGE-NR-4", Version: 2},
					CspStorageType: "Amazon SSD",
					PoolID:         "SP-SSD",
					SizeBytes:      swag.Int64(int64(10 * units.GiB)),
				},
				StorageMutable: models.StorageMutable{
					ParcelSizeBytes: swag.Int64(int64(512 * units.MiB)),
					AvailableBytes:  swag.Int64(int64(8 * units.GiB)),
					StorageState: &models.StorageStateMutable{
						AttachedNodeID:     "NODE-1",
						AttachedNodeDevice: "/dev/xvdbf",
						DeviceState:        "UNUSED",
						MediaState:         "FORMATTED",
						AttachmentState:    "ATTACHED",
					},
				},
			},
			&models.Storage{ // Detached storage
				StorageAllOf0: models.StorageAllOf0{
					Meta:           &models.ObjMeta{ID: "STORAGE-DET-1", Version: 2},
					CspStorageType: "Amazon SSD",
					PoolID:         "SP-SSD",
					SizeBytes:      swag.Int64(int64(10 * units.GiB)),
				},
				StorageMutable: models.StorageMutable{
					ParcelSizeBytes: swag.Int64(int64(512 * units.MiB)),
					AvailableBytes:  swag.Int64(int64(8 * units.GiB)),
					StorageState: &models.StorageStateMutable{
						DeviceState:     "UNUSED",
						MediaState:      "FORMATTED",
						AttachmentState: "DETACHED",
					},
				},
			},
		},
	}
	numStorageUsed := len(sRes.Payload)
	for _, s := range sRes.Payload {
		if strings.HasPrefix(string(s.Meta.ID), "STORAGE-NR-") || strings.HasPrefix(string(s.Meta.ID), "STORAGE-DET-") {
			numStorageUsed--
		}
	}
	srRes := &mcsr.StorageRequestListOK{
		Payload: []*models.StorageRequest{
			&models.StorageRequest{
				StorageRequestAllOf0: models.StorageRequestAllOf0{
					Meta: &models.ObjMeta{ID: "SR-1", Version: 2},
				},
				StorageRequestCreateOnce: models.StorageRequestCreateOnce{
					RequestedOperations: []string{"ATTACH", "USE"},
					CspStorageType:      "Amazon SSD",
					PoolID:              "SP-SSD",
				},
				StorageRequestMutable: models.StorageRequestMutable{
					StorageRequestCreateMutable: models.StorageRequestCreateMutable{
						NodeID:    "NODE-1",
						StorageID: "STORAGE-2",
					},
					StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
						StorageRequestState: com.StgReqStateSucceeded,
					},
				},
			},
			&models.StorageRequest{
				StorageRequestAllOf0: models.StorageRequestAllOf0{
					Meta: &models.ObjMeta{ID: "SR-2", Version: 2},
				},
				StorageRequestCreateOnce: models.StorageRequestCreateOnce{
					RequestedOperations: []string{"PROVISION", "ATTACH", "USE", "FORMAT"},
					CspStorageType:      "Amazon GP2",
					PoolID:              "SP-GP2",
				},
				StorageRequestMutable: models.StorageRequestMutable{
					StorageRequestCreateMutable: models.StorageRequestCreateMutable{
						NodeID:    "NODE-1",
						StorageID: "STORAGE-NR-3",
					},
				},
			},
			&models.StorageRequest{
				StorageRequestAllOf0: models.StorageRequestAllOf0{
					Meta: &models.ObjMeta{ID: "SR-IGNORED-NO-ATTACH-DETACH", Version: 2},
				},
				StorageRequestCreateOnce: models.StorageRequestCreateOnce{
					RequestedOperations: []string{"RELEASE"},
					CspStorageType:      "Amazon SSD",
					PoolID:              "SP-SSD",
				},
				StorageRequestMutable: models.StorageRequestMutable{
					StorageRequestCreateMutable: models.StorageRequestCreateMutable{
						NodeID: "NODE-1",
					},
				},
			},
			&models.StorageRequest{
				StorageRequestAllOf0: models.StorageRequestAllOf0{
					Meta: &models.ObjMeta{ID: "SR-RELEASING", Version: 2},
				},
				StorageRequestCreateOnce: models.StorageRequestCreateOnce{
					RequestedOperations: []string{"CLOSE", "DETACH", "RELEASE"},
					CspStorageType:      "Amazon SSD",
					PoolID:              "SP-SSD",
				},
				StorageRequestMutable: models.StorageRequestMutable{
					StorageRequestCreateMutable: models.StorageRequestCreateMutable{
						NodeID:    "NODE-1",
						StorageID: "STORAGE-NR-4",
					},
				},
			},
		},
	}

	// loadNodes
	fc.RetLsNErr = fmt.Errorf("node-list-error")
	nM, err := cs.loadNodes(ctx)
	assert.Error(err)
	assert.Regexp("node-list-error", err)
	assert.Nil(nM)
	assert.NotNil(fc.InLsNObj)
	nlP := mcn.NewNodeListParams()
	nlP.ClusterID = swag.String("CLUSTER-1")
	assert.Equal(nlP, fc.InLsNObj)
	tl.Flush()

	fc.RetLsNErr = nil
	fc.RetLsNObj = nRes
	nM, err = cs.loadNodes(ctx)
	assert.NoError(err)
	assert.NotNil(nM)
	assert.Len(nM, len(nRes.Payload))
	for _, nObj := range nRes.Payload {
		nd, has := nM[string(nObj.Meta.ID)]
		assert.True(has)
		assert.Equal(nObj, nd.Node)
		assert.Empty(nd.AttachedStorage)
	}
	tl.Flush()

	// loadStorage
	fc.RetLsSErr = fmt.Errorf("storage-list-error")
	sM, err := cs.loadStorage(ctx)
	assert.Error(err)
	assert.Regexp("storage-list-error", err)
	assert.Nil(sM)
	assert.NotNil(fc.InLsSObj)
	slP := mcs.NewStorageListParams()
	slP.ClusterID = swag.String("CLUSTER-1")
	assert.Equal(slP, fc.InLsSObj)
	tl.Flush()

	fc.RetLsSErr = nil
	fc.RetLsSOk = sRes
	sM, err = cs.loadStorage(ctx)
	assert.NoError(err)
	assert.NotNil(sM)
	assert.Len(sM, len(sRes.Payload))
	for _, sObj := range sRes.Payload {
		o, has := sM[string(sObj.Meta.ID)]
		assert.True(has)
		assert.Equal(sObj, o)
	}
	tl.Flush()

	// loadStorageRequests
	fc.RetLsSRErr = fmt.Errorf("storage-request-list-error")
	srM, err := cs.loadStorageRequests(ctx)
	assert.Error(err)
	assert.Regexp("storage-request-list-error", err)
	assert.Nil(srM)
	assert.NotNil(fc.InLsSRObj)
	srlP := mcsr.NewStorageRequestListParams()
	srlP.ClusterID = swag.String("CLUSTER-1")
	srlP.IsTerminated = swag.Bool(false)
	assert.Equal(srlP, fc.InLsSRObj)

	claimM := map[string]*ClaimData{
		"SR-1": &ClaimData{
			StorageRequest: &models.StorageRequest{
				StorageRequestAllOf0: models.StorageRequestAllOf0{
					Meta: &models.ObjMeta{ID: "SR-1", Version: 1},
				},
				StorageRequestCreateOnce: models.StorageRequestCreateOnce{
					RequestedOperations: []string{"ATTACH", "USE"},
					CspStorageType:      "Amazon SSD",
					PoolID:              "SP-SSD",
				},
			},
			PoolID:    "SP-SSD",
			NodeID:    "NODE-1",
			VSRClaims: []VSRClaim{{RequestID: "VSR-1", SizeBytes: 99}},
		},
		"SR-NO-MORE-CLAIMS": &ClaimData{
			StorageRequest: &models.StorageRequest{
				StorageRequestAllOf0: models.StorageRequestAllOf0{
					Meta: &models.ObjMeta{ID: "SR-NO-MORE-CLAIMS", Version: 1},
				},
				StorageRequestCreateOnce: models.StorageRequestCreateOnce{
					PoolID: "SP-GP2",
				},
			},
			PoolID:    "SP-GP2",
			NodeID:    "NODE-2",
			VSRClaims: []VSRClaim{},
		},
		"SR-MUST-FETCH": &ClaimData{
			StorageRequest: &models.StorageRequest{
				StorageRequestAllOf0: models.StorageRequestAllOf0{
					Meta: &models.ObjMeta{ID: "SR-MUST-FETCH", Version: 1},
				},
				StorageRequestCreateOnce: models.StorageRequestCreateOnce{
					PoolID: "SP-GP2",
				},
			},
			PoolID:    "SP-GP2",
			NodeID:    "NODE-FOO", // will not be in the cache - impacts dump
			VSRClaims: []VSRClaim{{RequestID: "VSR-2", SizeBytes: 10}},
		},
	}
	cs.StorageRequests = cloneClaimM(claimM)
	fc.RetLsSRErr = nil
	fc.RetLsSRObj = srRes
	fc.InSRFetchID = ""
	fc.RetSRFetchErr = fmt.Errorf("storage-request-fetch-error")
	srM, err = cs.loadStorageRequests(ctx)
	assert.Error(err)
	assert.Regexp("storage-request-fetch-error", err)
	assert.Nil(srM)
	assert.NotNil(fc.InLsSRObj)
	assert.Equal(srlP, fc.InLsSRObj)
	assert.Equal("SR-MUST-FETCH", fc.InSRFetchID)
	tl.Flush()

	fc.InSRFetchID = ""
	fc.RetSRFetchErr = nil
	fc.RetSRFetchObj = &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{ID: "SR-MUST-FETCH", Version: 2},
		},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			RequestedOperations: []string{"ATTACH", "USE"},
			CspStorageType:      "Amazon GP2",
			PoolID:              "SP-GP2",
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestCreateMutable: models.StorageRequestCreateMutable{
				NodeID: "NODE-FOO", // does not exist in cache
			},
		},
	}
	srM, err = cs.loadStorageRequests(ctx)
	assert.NoError(err)
	assert.NotNil(srM)
	assert.NotNil(fc.InLsSRObj)
	assert.Equal(srlP, fc.InLsSRObj)
	assert.Equal("SR-MUST-FETCH", fc.InSRFetchID)
	expSRIDs := []string{"SR-1", "SR-2", "SR-MUST-FETCH", "SR-RELEASING"}
	assert.Len(srM, len(expSRIDs))
	for _, srID := range expSRIDs {
		o, has := srM[srID]
		assert.True(has)
		assert.EqualValues(2, o.Meta.Version)
	}
	tl.Flush()

	// Reload, prime cache error
	t.Log("case: prime cache error")
	fc.RetLsVRErr = fmt.Errorf("volume-series-request-list-error")
	assert.False(cs.srCacheIsPrimed)
	_, err = cs.Reload(ctx)
	assert.Error(err)
	assert.Regexp("volume-series-request-list-error", err)
	assert.False(cs.srCacheIsPrimed)
	tl.Flush()

	// Reload, prime cache succeeds (fully tested in TestPrimeSRCache)
	t.Log("case: node list error")
	ndM := map[string]*NodeData{
		"NODE-1": &NodeData{
			Node: &models.Node{
				NodeAllOf0: models.NodeAllOf0{
					Meta: &models.ObjMeta{ID: "NODE-1", Version: 1},
				},
				NodeMutable: models.NodeMutable{
					Name: "Node1",
					Service: &models.NuvoService{
						ServiceState: models.ServiceState{
							State: "READY",
						},
					},
				},
			},
			AttachedStorage: []string{"STORAGE-1"},
		},
	}
	cs.Nodes = cloneNDM(ndM)
	fc.RetLsNErr = fmt.Errorf("node-list-error")
	fc.RetLsNObj = nil
	fc.RetLsSErr = fmt.Errorf("storage-list-error")
	fc.RetLsSOk = nil
	fc.RetLsSRErr = fmt.Errorf("storage-request-list-error")
	fc.RetLsSRObj = nil
	fc.RetLsVRErr = nil
	fc.RetLsVRObj = &mcvsr.VolumeSeriesRequestListOK{
		Payload: []*models.VolumeSeriesRequest{},
	}
	assert.False(cs.srCacheIsPrimed)
	chg, err := cs.Reload(ctx)
	assert.True(cs.srCacheIsPrimed)
	assert.Error(err)
	assert.False(chg)
	assert.Regexp("node-list-error", err)
	assert.Len(cs.Nodes, len(ndM))
	for _, nd := range cs.Nodes {
		assert.EqualValues(1, nd.Node.Meta.Version)
	}
	assert.Len(cs.Storage, 0)
	assert.Len(cs.StorageRequests, len(claimM))
	for srID, cl := range cs.StorageRequests {
		assert.EqualValues(1, cl.StorageRequest.Meta.Version, "** %s", srID)
	}
	assert.Equal(0, cs.NumLoads)
	tl.Flush()

	t.Log("case: storage request list error")
	fc.RetLsNErr = nil
	fc.RetLsNObj = nRes
	chg, err = cs.Reload(ctx)
	assert.Error(err)
	assert.False(chg)
	assert.Regexp("storage-request-list-error", err)
	assert.Len(cs.Nodes, len(ndM))
	for _, nd := range cs.Nodes {
		assert.EqualValues(1, nd.Node.Meta.Version)
	}
	assert.Len(cs.Storage, 0)
	assert.Len(cs.StorageRequests, len(claimM))
	for srID, cl := range cs.StorageRequests {
		assert.EqualValues(1, cl.StorageRequest.Meta.Version, "** %s", srID)
	}
	assert.Equal(0, cs.NumLoads)
	tl.Flush()

	t.Log("case: storage list error")
	fc.RetLsSRErr = nil
	fc.RetLsSRObj = srRes
	chg, err = cs.Reload(ctx)
	assert.Error(err)
	assert.False(chg)
	assert.Regexp("storage-list-error", err)
	assert.Len(cs.Nodes, len(ndM))
	for _, nd := range cs.Nodes {
		assert.EqualValues(1, nd.Node.Meta.Version)
	}
	assert.Len(cs.Storage, 0)
	assert.Len(cs.StorageRequests, len(claimM))
	for srID, cl := range cs.StorageRequests {
		assert.EqualValues(1, cl.StorageRequest.Meta.Version, "** %s", srID)
	}
	assert.Equal(0, cs.NumLoads)
	tl.Flush()

	t.Log("case: success")
	fc.RetLsSErr = nil
	fc.RetLsSOk = sRes
	chg, err = cs.Reload(ctx)
	tl.Flush()
	assert.NoError(err)
	assert.True(chg)
	assert.Len(cs.Nodes, len(nRes.Payload))
	assert.Equal(2, len(cs.claimedStorage))
	for _, nObj := range nRes.Payload {
		nID := string(nObj.Meta.ID)
		nd, has := cs.Nodes[nID]
		assert.True(has, "Node %s", nID)
		assert.Equal(nObj, nd.Node, "Node %s", nID)
		if nID == "NODE-1" {
			expAS := []string{"STORAGE-1", "STORAGE-2"}
			sort.Strings(nd.AttachedStorage)
			assert.Equal(expAS, nd.AttachedStorage)
		} else {
			assert.Empty(nd.AttachedStorage)
		}
	}
	assert.Len(cs.Storage, numStorageUsed)
	for _, sObj := range sRes.Payload {
		srID := string(sObj.Meta.ID)
		if strings.HasPrefix(srID, "STORAGE-NR-") {
			continue
		}
		o, has := cs.Storage[srID]
		detO, hasD := cs.DetachedStorage[srID]
		if strings.HasPrefix(srID, "STORAGE-DET-") {
			assert.True(hasD, "DetachedStorage %s", srID)
			assert.Equal(sObj, detO, "DetachedStorage %s", srID)
			assert.False(has, "Storage %s", srID)
		} else {
			assert.True(has, "Storage %s", srID)
			assert.Equal(sObj, o, "Storage %s", srID)
			assert.False(hasD, "DetachedStorage %s", srID)
		}
	}
	expSRIDs = []string{"SR-1", "SR-MUST-FETCH"}
	assert.Len(cs.StorageRequests, len(expSRIDs))
	for _, srID := range expSRIDs {
		_, has := cs.StorageRequests[srID]
		assert.True(has, "** %s", srID)
	}
	for srID, cl := range cs.StorageRequests {
		assert.EqualValues(2, cl.StorageRequest.Meta.Version, "** %s", srID)
		assert.NotEmpty(cl.PoolID, "SP set in ClaimData: %s", srID)
		assert.NotEmpty(cl.NodeID, "Node set in ClaimData: %s", srID)
	}
	assert.Equal(1, cs.NumLoads)
	cs.NodeServiceState("NODE-1", &models.ServiceState{State: "READY"}) // insert health data
	expDumpString := "Iteration:1 #Nodes:2 #Storage:3 #SR:2\n" +
		"- Node[NODE-1] READY #Attached: 2\n" +
		"   S[STORAGE-1] T:'Amazon GP2' TSz:3GiB ASz:0B PSz:0B TP:0 AP:? DS:UNUSED MS:FORMATTED AS:ATTACHED D:/dev/xvdba\n" +
		"   S[STORAGE-2] T:'Amazon SSD' TSz:10GiB ASz:8GiB PSz:512MiB TP:0 AP:16 DS:UNUSED MS:FORMATTED AS:ATTACHED D:/dev/xvdbb\n" +
		"- Node[NODE-2] UNKNOWN #Attached: 0\n" +
		"- SR[SR-1] N:'Node1' T:'Amazon SSD' RB:0B #Claims:1\n" +
		"   VSR[VSR-1] 99B\n" +
		"- SR[SR-MUST-FETCH] N:[NODE-FOO] T:'Amazon GP2' RB:0B #Claims:1\n" +
		"   VSR[VSR-2] 10B\n" +
		"- S[STORAGE-DET-1] T:'Amazon SSD' TSz:10GiB ASz:8GiB PSz:512MiB TP:0 AP:16 DS:UNUSED MS:FORMATTED AS:DETACHED D:\n"
	b := cs.DumpState()
	tl.Logger().Infof("State:\n%s", b.String())
	assert.Equal(expDumpString, b.String())

	chg, err = cs.Reload(ctx)
	assert.NoError(err)
	assert.False(chg)
	assert.Equal(2, cs.NumLoads)

	// forced change based on internal counters
	cs.LoadCountOnInternalChange = cs.NumLoads
	chg, err = cs.Reload(ctx)
	assert.NoError(err)
	assert.True(chg)
	assert.Equal(3, cs.NumLoads)
}

func TestStateChanged(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	args := &ClusterStateArgs{
		Log: tl.Logger(),
	}
	cs1 := NewClusterState(args)

	// compare via attached storage
	cs1.Nodes = map[string]*NodeData{
		"NODE-1": &NodeData{
			AttachedStorage: []string{"STORAGE-1"},
			Node: &models.Node{
				NodeMutable: models.NodeMutable{
					Service: &models.NuvoService{
						ServiceState: models.ServiceState{
							State: "READY",
						},
					},
				},
			},
		},
		"NODE-2": &NodeData{
			AttachedStorage: []string{"STORAGE-2"},
			Node: &models.Node{
				NodeMutable: models.NodeMutable{
					Service: &models.NuvoService{
						ServiceState: models.ServiceState{
							State: "READY",
						},
					},
				},
			},
		},
	}
	cs1.Storage = map[string]*models.Storage{
		"STORAGE-1": &models.Storage{
			StorageMutable: models.StorageMutable{
				AvailableBytes: swag.Int64(int64(10 * units.GiB)),
				StorageState: &models.StorageStateMutable{
					DeviceState:     "UNUSED",
					MediaState:      "FORMATTED",
					AttachmentState: "ATTACHED",
				},
			},
		},
		"STORAGE-2": &models.Storage{
			StorageMutable: models.StorageMutable{
				AvailableBytes: swag.Int64(int64(10 * units.GiB)),
				StorageState: &models.StorageStateMutable{
					DeviceState:     "UNUSED",
					MediaState:      "FORMATTED",
					AttachmentState: "ATTACHED",
				},
			},
		},
	}
	cs1.DetachedStorage = map[string]*models.Storage{
		"STORAGE-DET-1": &models.Storage{
			StorageMutable: models.StorageMutable{
				AvailableBytes: swag.Int64(int64(10 * units.GiB)),
				StorageState: &models.StorageStateMutable{
					DeviceState:     "UNUSED",
					MediaState:      "FORMATTED",
					AttachmentState: "DETACHED",
				},
			},
		},
		"STORAGE-DET-2": &models.Storage{
			StorageMutable: models.StorageMutable{
				AvailableBytes: swag.Int64(int64(10 * units.GiB)),
				StorageState: &models.StorageStateMutable{
					DeviceState:     "UNUSED",
					MediaState:      "UNFORMATTED",
					AttachmentState: "DETACHED",
				},
			},
		},
	}
	sr1 := &models.StorageRequest{}
	sr1.Meta = &models.ObjMeta{ID: "SR-1"}
	sr1.StorageRequestState = "NEW"
	sr2 := &models.StorageRequest{}
	sr2.Meta = &models.ObjMeta{ID: "SR-2"}
	sr2.StorageRequestState = "NEW"
	sr3 := &models.StorageRequest{}
	sr3.Meta = &models.ObjMeta{ID: "SR-3"}
	sr3.StorageRequestState = "NEW"
	sr4 := &models.StorageRequest{}
	sr4.Meta = &models.ObjMeta{ID: "SR-4"}
	sr4.StorageRequestState = "NEW"

	cs1.StorageRequests = map[string]*ClaimData{
		"SR-1": &ClaimData{
			StorageRequest: sr1,
			VSRClaims: []VSRClaim{
				{RequestID: "VSR-1", SizeBytes: 1000},
			},
		},
		"SR-2": &ClaimData{
			StorageRequest: sr2,
			VSRClaims: []VSRClaim{
				{RequestID: "VSR-2", SizeBytes: 1000},
				{RequestID: "VSR-3", SizeBytes: 1000},
			},
		},
	}
	assert.False(cs1.stateChanged(cs1))

	cs2 := NewClusterState(args)
	testutils.Clone(cs1, &cs2)
	assert.False(cs1.stateChanged(cs2))
	cs2.Nodes["NODE-1"].AttachedStorage = []string{"STORAGE-2"}
	cs2.Nodes["NODE-2"].AttachedStorage = []string{"STORAGE-1"}
	assert.True(cs1.stateChanged(cs2))
	assert.True(cs2.stateChanged(cs1))

	// compare via node ids
	cs2 = NewClusterState(args)
	testutils.Clone(cs1, &cs2)
	assert.False(cs1.stateChanged(cs2))
	cs2.Nodes = map[string]*NodeData{
		"NODE-1": &NodeData{
			AttachedStorage: []string{"STORAGE-1"},
			Node: &models.Node{
				NodeMutable: models.NodeMutable{
					Service: &models.NuvoService{
						ServiceState: models.ServiceState{
							State: "READY",
						},
					},
				},
			},
		},
		"NODE-3": &NodeData{
			AttachedStorage: []string{"STORAGE-2"},
			Node: &models.Node{
				NodeMutable: models.NodeMutable{
					Service: &models.NuvoService{
						ServiceState: models.ServiceState{
							State: "READY",
						},
					},
				},
			},
		},
	}
	assert.True(cs1.stateChanged(cs2))
	assert.True(cs2.stateChanged(cs1))

	// compare via node state
	cs2 = NewClusterState(args)
	testutils.Clone(cs1, &cs2)
	assert.False(cs1.stateChanged(cs2))
	cs2.Nodes["NODE-1"] = &NodeData{
		AttachedStorage: []string{"STORAGE-1"},
		Node: &models.Node{
			NodeMutable: models.NodeMutable{
				Service: &models.NuvoService{
					ServiceState: models.ServiceState{
						State: "STARTING",
					},
				},
			},
		},
	}
	assert.True(cs1.stateChanged(cs2))
	assert.True(cs2.stateChanged(cs1))

	// compare via storage ids
	cs2 = NewClusterState(args)
	testutils.Clone(cs1, &cs2)
	assert.False(cs1.stateChanged(cs2))
	cs2.Storage = map[string]*models.Storage{
		"STORAGE-3": &models.Storage{
			StorageMutable: models.StorageMutable{
				AvailableBytes: swag.Int64(int64(10 * units.GiB)),
				StorageState: &models.StorageStateMutable{
					DeviceState:     "UNUSED",
					MediaState:      "FORMATTED",
					AttachmentState: "ATTACHED",
				},
			},
		},
		"STORAGE-2": &models.Storage{
			StorageMutable: models.StorageMutable{
				AvailableBytes: swag.Int64(int64(10 * units.GiB)),
				StorageState: &models.StorageStateMutable{
					DeviceState:     "UNUSED",
					MediaState:      "FORMATTED",
					AttachmentState: "ATTACHED",
				},
			},
		},
	}
	assert.True(cs1.stateChanged(cs2))
	assert.True(cs2.stateChanged(cs1))

	// compare via storage state (DeviceState)
	cs2 = NewClusterState(args)
	testutils.Clone(cs1, &cs2)
	assert.False(cs1.stateChanged(cs2))
	cs2.Storage["STORAGE-1"] = &models.Storage{
		StorageMutable: models.StorageMutable{
			AvailableBytes: swag.Int64(int64(10 * units.GiB)),
			StorageState: &models.StorageStateMutable{
				DeviceState:     "OPEN",
				MediaState:      "FORMATTED",
				AttachmentState: "ATTACHED",
			},
		},
	}
	assert.True(cs1.stateChanged(cs2))
	assert.True(cs2.stateChanged(cs1))

	// compare via storage state (MediaState)
	cs2 = NewClusterState(args)
	testutils.Clone(cs1, &cs2)
	assert.False(cs1.stateChanged(cs2))
	cs2.Storage["STORAGE-1"] = &models.Storage{
		StorageMutable: models.StorageMutable{
			AvailableBytes: swag.Int64(int64(10 * units.GiB)),
			StorageState: &models.StorageStateMutable{
				DeviceState:     "UNUSED",
				MediaState:      "UNFORMATTED",
				AttachmentState: "ATTACHED",
			},
		},
	}
	assert.True(cs1.stateChanged(cs2))
	assert.True(cs2.stateChanged(cs1))

	// compare via storage state (AttachmentState)
	cs2 = NewClusterState(args)
	testutils.Clone(cs1, &cs2)
	assert.False(cs1.stateChanged(cs2))
	cs2.Storage["STORAGE-1"] = &models.Storage{
		StorageMutable: models.StorageMutable{
			AvailableBytes: swag.Int64(int64(10 * units.GiB)),
			StorageState: &models.StorageStateMutable{
				DeviceState:     "UNUSED",
				MediaState:      "FORMATTED",
				AttachmentState: "DETACHING",
			},
		},
	}
	assert.True(cs1.stateChanged(cs2))
	assert.True(cs2.stateChanged(cs1))

	// compare via storage AvailableBytes
	cs2 = NewClusterState(args)
	testutils.Clone(cs1, &cs2)
	assert.False(cs1.stateChanged(cs2))
	cs2.Storage["STORAGE-1"] = &models.Storage{
		StorageMutable: models.StorageMutable{
			AvailableBytes: swag.Int64(int64(9 * units.GiB)),
			StorageState: &models.StorageStateMutable{
				DeviceState:     "UNUSED",
				MediaState:      "FORMATTED",
				AttachmentState: "ATTACHED",
			},
		},
	}
	assert.True(cs1.stateChanged(cs2))
	assert.True(cs2.stateChanged(cs1))

	// compare via detached storage ids
	cs2 = NewClusterState(args)
	testutils.Clone(cs1, &cs2)
	assert.False(cs1.stateChanged(cs2))
	cs2.DetachedStorage = map[string]*models.Storage{
		"STORAGE-DET-3": &models.Storage{
			StorageMutable: models.StorageMutable{
				AvailableBytes: swag.Int64(int64(10 * units.GiB)),
				StorageState: &models.StorageStateMutable{
					DeviceState:     "UNUSED",
					MediaState:      "FORMATTED",
					AttachmentState: "DETACHED",
				},
			},
		},
		"STORAGE-DET-2": &models.Storage{
			StorageMutable: models.StorageMutable{
				AvailableBytes: swag.Int64(int64(10 * units.GiB)),
				StorageState: &models.StorageStateMutable{
					DeviceState:     "UNUSED",
					MediaState:      "FORMATTED",
					AttachmentState: "DETACHED",
				},
			},
		},
	}
	assert.True(cs1.stateChanged(cs2))
	assert.True(cs2.stateChanged(cs1))

	// compare via detached storage ids count
	cs2 = NewClusterState(args)
	testutils.Clone(cs1, &cs2)
	assert.False(cs1.stateChanged(cs2))
	delete(cs2.DetachedStorage, "STORAGE-DET-1")
	assert.True(len(cs1.DetachedStorage) != len(cs2.DetachedStorage))
	assert.True(cs1.stateChanged(cs2))

	// compare via claims (only vsr count changed)
	cs2 = NewClusterState(args)
	testutils.Clone(cs1, &cs2)
	assert.False(cs1.stateChanged(cs2))
	cs2.StorageRequests = map[string]*ClaimData{
		"SR-1": &ClaimData{
			StorageRequest: sr1,
			VSRClaims: []VSRClaim{
				{RequestID: "VSR-1", SizeBytes: 1000},
			},
		},
		"SR-2": &ClaimData{
			StorageRequest: sr2,
			VSRClaims: []VSRClaim{
				{RequestID: "VSR-3", SizeBytes: 1000},
			},
		},
	}
	assert.True(cs1.stateChanged(cs2))
	assert.True(cs2.stateChanged(cs1))

	// compare via claims (only vsr ids changed)
	cs2 = NewClusterState(args)
	testutils.Clone(cs1, &cs2)
	assert.False(cs1.stateChanged(cs2))
	cs2.StorageRequests = map[string]*ClaimData{
		"SR-1": &ClaimData{
			StorageRequest: sr1,
			VSRClaims: []VSRClaim{
				{RequestID: "VSR-1", SizeBytes: 1000},
			},
		},
		"SR-2": &ClaimData{
			StorageRequest: sr2,
			VSRClaims: []VSRClaim{
				{RequestID: "VSR-4", SizeBytes: 1000},
				{RequestID: "VSR-3", SizeBytes: 1000},
			},
		},
	}
	assert.True(cs1.stateChanged(cs2))
	assert.True(cs2.stateChanged(cs1))

	// compare via claims (only sr ids changed)
	cs2 = NewClusterState(args)
	testutils.Clone(cs1, &cs2)
	assert.False(cs1.stateChanged(cs2))
	cs2.StorageRequests = map[string]*ClaimData{
		"SR-3": &ClaimData{
			StorageRequest: sr3,
			VSRClaims: []VSRClaim{
				{RequestID: "VSR-1", SizeBytes: 1000},
			},
		},
		"SR-4": &ClaimData{
			StorageRequest: sr4,
			VSRClaims: []VSRClaim{
				{RequestID: "VSR-2", SizeBytes: 1000},
				{RequestID: "VSR-3", SizeBytes: 1000},
			},
		},
	}
	assert.True(cs1.stateChanged(cs2))
	assert.True(cs2.stateChanged(cs1))

	// compare via claims (sr state changed)
	cs2 = NewClusterState(args)
	testutils.Clone(cs1, &cs2)
	assert.False(cs1.stateChanged(cs2))
	sr2f := srClone(sr2)
	sr2f.StorageRequestState = com.StgReqStateFailed
	cs2.StorageRequests = map[string]*ClaimData{
		"SR-1": &ClaimData{
			StorageRequest: sr1,
			VSRClaims: []VSRClaim{
				{RequestID: "VSR-1", SizeBytes: 1000},
			},
		},
		"SR-2": &ClaimData{
			StorageRequest: sr2f,
			VSRClaims: []VSRClaim{
				{RequestID: "VSR-2", SizeBytes: 1000},
				{RequestID: "VSR-3", SizeBytes: 1000},
			},
		},
	}
	assert.True(cs1.stateChanged(cs2))
	assert.True(cs2.stateChanged(cs1))
}

func TestPrimeSRCache(t *testing.T) {
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
	assert.False(cs.srCacheIsPrimed)
	ctx := context.Background()

	// vsr list error
	fc.RetLsVRErr = fmt.Errorf("volume-series-request-list-error")
	fc.InLsVRObj = nil
	err := cs.primeSRCache(ctx)
	assert.Error(err)
	assert.Regexp("volume-series-request-list-error", err)
	assert.False(cs.srCacheIsPrimed)
	vsrLP := mcvsr.NewVolumeSeriesRequestListParams()
	vsrLP.ClusterID = swag.String("CLUSTER-1")
	vsrLP.IsTerminated = swag.Bool(false)
	assert.NotNil(fc.InLsVRObj)
	assert.Equal(vsrLP, fc.InLsVRObj)

	// sr list error (vsr in PLACEMENT)
	vsrLRet := &mcvsr.VolumeSeriesRequestListOK{
		Payload: []*models.VolumeSeriesRequest{
			&models.VolumeSeriesRequest{
				VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
					Meta: &models.ObjMeta{ID: "VSR-IN-PLACEMENT"},
				},
				VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
					VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
						VolumeSeriesRequestState: com.VolReqStatePlacement,
					},
				},
			},
			&models.VolumeSeriesRequest{
				VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
					Meta: &models.ObjMeta{ID: "VSR-IGNORED"},
				},
				VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
					VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
						VolumeSeriesRequestState: com.VolReqStateSizing,
					},
				},
			},
		},
	}
	fc.RetLsVRErr = nil
	fc.RetLsVRObj = vsrLRet
	fc.InLsSRObj = nil
	fc.RetLsSRErr = fmt.Errorf("storage-request-list-error")
	err = cs.primeSRCache(ctx)
	assert.Error(err)
	assert.Regexp("storage-request-list-error", err)
	assert.False(cs.srCacheIsPrimed)
	srLP := mcsr.NewStorageRequestListParams()
	srLP.VolumeSeriesRequestID = swag.String("VSR-IN-PLACEMENT")
	assert.NotNil(fc.InLsSRObj)
	assert.Equal(srLP, fc.InLsSRObj)

	// sr list error (vsr in PLACEMENT_REATTACH)
	vsrLRet.Payload[0].Meta.ID = "VSR-IN-PLACEMENT_REATTACH"
	vsrLRet.Payload[0].VolumeSeriesRequestState = com.VolReqStateStorageWait
	fc.RetLsVRErr = nil
	fc.RetLsVRObj = vsrLRet
	fc.InLsSRObj = nil
	fc.RetLsSRErr = fmt.Errorf("storage-request-list-error")
	err = cs.primeSRCache(ctx)
	assert.Error(err)
	assert.Regexp("storage-request-list-error", err)
	assert.False(cs.srCacheIsPrimed)
	srLP = mcsr.NewStorageRequestListParams()
	srLP.VolumeSeriesRequestID = swag.String("VSR-IN-PLACEMENT_REATTACH")
	assert.NotNil(fc.InLsSRObj)
	assert.Equal(srLP, fc.InLsSRObj)

	// no errors (vsr in STORAGE_WAIT)
	vsrLRet.Payload[0].Meta.ID = "VSR-IN-STORAGE-WAIT"
	vsrLRet.Payload[0].VolumeSeriesRequestState = com.VolReqStateStorageWait
	srLRet := &mcsr.StorageRequestListOK{
		Payload: []*models.StorageRequest{
			&models.StorageRequest{
				StorageRequestAllOf0: models.StorageRequestAllOf0{
					Meta: &models.ObjMeta{ID: "SR-1"},
				},
				StorageRequestCreateOnce: models.StorageRequestCreateOnce{
					PoolID: "SP-GP2",
				},
				StorageRequestMutable: models.StorageRequestMutable{
					StorageRequestCreateMutable: models.StorageRequestCreateMutable{
						NodeID: "NODE-1",
						VolumeSeriesRequestClaims: &models.VsrClaim{
							Claims: map[string]models.VsrClaimElement{
								"VSR-IN-STORAGE-WAIT": {SizeBytes: swag.Int64(1000), Annotation: "STORAGE-0-0"},
							},
						},
					},
				},
			},
		},
	}
	fc.InLsSRObj = nil
	fc.RetLsSRErr = nil
	fc.RetLsSRObj = srLRet
	assert.Empty(cs.StorageRequests)
	err = cs.primeSRCache(ctx)
	assert.Nil(err)
	assert.True(cs.srCacheIsPrimed)
	srLP.VolumeSeriesRequestID = swag.String("VSR-IN-STORAGE-WAIT")
	assert.NotNil(fc.InLsSRObj)
	assert.Equal(srLP, fc.InLsSRObj)
	assert.NotEmpty(cs.StorageRequests)
	_, has := cs.StorageRequests["SR-1"]
	assert.True(has)
	cl := cs.FindClaimsByRequest("VSR-IN-STORAGE-WAIT")
	assert.NotNil(cl)
	assert.Len(cl, 1)
	assert.Equal("VSR-IN-STORAGE-WAIT", cl[0].VSRClaims[0].RequestID)

	// already primed
	fc.RetLsVRErr = fmt.Errorf("volume-series-request-list-error")
	fc.RetLsSRErr = fmt.Errorf("storage-request-list-error")
	err = cs.primeSRCache(ctx)
	assert.Nil(err)
}
