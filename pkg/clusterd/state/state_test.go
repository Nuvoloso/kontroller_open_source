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
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	mockcsp "github.com/Nuvoloso/kontroller/pkg/csp/mock"
	"github.com/Nuvoloso/kontroller/pkg/layout"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestNewClusterState(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)

	fc := &fake.Client{}

	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "CLUSTER-1",
			},
		},
	}

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cSP := mockcsp.NewMockCloudServiceProvider(mockCtrl)

	tcInvalidCSA := []ClusterStateArgs{
		{},
		{OCrud: fc},
		{OCrud: fc, Cluster: clObj},
		{OCrud: fc, Cluster: clObj, CSP: cSP},
		{OCrud: fc, Cluster: clObj, CSP: cSP, Log: tl.Logger()},
		{OCrud: fc, Cluster: clObj, CSP: cSP, Log: tl.Logger(), LayoutAlgorithms: []*layout.Algorithm{}},
	}
	for i, tc := range tcInvalidCSA {
		assert.False((&tc).Verify(), "case: %d", i)
	}

	la, err := layout.FindAlgorithm(layout.AlgorithmStandaloneNetworkedShared)
	assert.NoError(err)
	args := &ClusterStateArgs{
		Log:              tl.Logger(),
		OCrud:            fc,
		Cluster:          clObj,
		CSP:              cSP,
		LayoutAlgorithms: []*layout.Algorithm{la},
	}

	cso := New(args)
	assert.NotNil(cso)
	cs, ok := cso.(*ClusterState)
	assert.True(ok)
	assert.Equal(cs, cso.CS())

	assert.Equal(fc, cs.OCrud)
	assert.Equal(args.Log, cs.Log)
	assert.Equal(clObj, cs.Cluster)
	assert.NotNil(cs.Nodes)
	assert.NotNil(cs.Storage)
	assert.NotNil(cs.StorageRequests)
	assert.NotNil(cs.pCache)

	args.CSP = nil
	cso = New(args)
	assert.Nil(cso)
}

func TestClaims(t *testing.T) {
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
	ctx := context.Background()

	assert.Nil(cs.FindClaimByStorageRequest("fooBar"))

	// claim1_1 grabs the entire future Storage object
	srN1T1A := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{ID: "SR-N1-T1-A"},
		},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			PoolID: "SP-GP2",
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestCreateMutable: models.StorageRequestCreateMutable{
				NodeID: "NODE-1",
				VolumeSeriesRequestClaims: &models.VsrClaim{
					Claims: map[string]models.VsrClaimElement{
						"VSR-1": {SizeBytes: swag.Int64(1000), Annotation: "claim1_1"},
					},
				},
			},
		},
	}
	claim1_1 := &VSRClaim{
		RequestID:  "VSR-1",
		SizeBytes:  1000,
		Annotation: "claim1_1",
	}
	cd := cs.TrackStorageRequest(srN1T1A)
	assert.NotNil(cd)
	assert.Equal(cs, cd.cs)
	cl, ok := cs.StorageRequests[string(srN1T1A.Meta.ID)]
	assert.True(ok)
	assert.Equal(cd, cl)
	assert.Equal(cd, cs.FindClaimByStorageRequest(string(srN1T1A.Meta.ID)))
	assert.Equal(srN1T1A, cd.StorageRequest)
	assert.EqualValues(srN1T1A.PoolID, cd.PoolID)
	assert.EqualValues(srN1T1A.NodeID, cd.NodeID)
	assert.EqualValues(0, cd.RemainingBytes)
	assert.Equal(*claim1_1, cd.VSRClaims[0])

	clA := cs.FindClaimsByRequest("VSR-1")
	assert.Len(clA, 1)
	assert.Equal(cd, clA[0])

	clA = cs.FindClaimsByPool(string(srN1T1A.PoolID), 0, "NODE-1", false)
	assert.Len(clA, 1)
	assert.Equal(cd, clA[0])
	clA = cs.FindClaimsByPool(string(srN1T1A.PoolID), 0, "NODE-2", true)
	assert.Len(clA, 1)
	assert.Equal(cd, clA[0])

	cd2 := cs.TrackStorageRequest(srN1T1A)
	assert.NotNil(cd2)
	assert.Equal(cd, cd2)

	srN1T1B := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{ID: "SR-N1-T1-B"},
		},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			PoolID: "SP-GP2",
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestCreateMutable: models.StorageRequestCreateMutable{
				NodeID: "NODE-1",
				VolumeSeriesRequestClaims: &models.VsrClaim{
					RemainingBytes: swag.Int64(100000),
					Claims: map[string]models.VsrClaimElement{
						"VSR-2": {SizeBytes: swag.Int64(1000), Annotation: "claim1_2"},
					},
				},
			},
		},
	}
	cd, err := cs.AddClaim(ctx, srN1T1B, claim1_1)
	assert.Error(err)
	assert.Regexp("StorageRequest not tracked", err)

	claim1_2 := &VSRClaim{
		RequestID:  "VSR-2",
		SizeBytes:  1000,
		Annotation: "claim1_2",
	}
	cd, err = cs.AddClaim(ctx, srN1T1A, claim1_2)
	assert.Error(err)
	assert.Regexp("Insufficient capacity", err) // already grabbed all
	cd, err = cs.AddClaim(ctx, srN1T1A, claim1_1)
	assert.Error(err)
	assert.Regexp("Insufficient capacity", err) // id doesn't matter - already grabbed all
	claim1_2.SizeBytes = 0
	cd, err = cs.AddClaim(ctx, srN1T1A, claim1_2)
	assert.Error(err)
	assert.Regexp("Invalid claim size", err)
	srN1T1B.StorageRequestState = com.StgReqStateFailed
	cd, err = cs.AddClaim(ctx, srN1T1B, claim1_2)
	assert.Error(err)
	assert.Regexp("StorageRequest.*has failed", err)
	srN1T1B.StorageRequestState = ""

	// claim1_2 leaves remaining space in the future Storage object
	claim1_2.SizeBytes = 1000
	cd = cs.TrackStorageRequest(srN1T1B)
	assert.NotNil(cd)
	cl, ok = cs.StorageRequests[string(srN1T1B.Meta.ID)]
	assert.True(ok)
	assert.Equal(cd, cl)
	assert.Equal(srN1T1B, cd.StorageRequest)
	assert.EqualValues(srN1T1B.PoolID, cd.PoolID)
	assert.EqualValues(srN1T1B.NodeID, cd.NodeID)
	assert.EqualValues(100000, cd.RemainingBytes)
	assert.Equal(*claim1_2, cd.VSRClaims[0])

	clA = cs.FindClaimsByRequest("VSR-2")
	assert.Len(clA, 1)
	assert.Equal(cd, clA[0])

	// extend the claim
	vce, ok := srN1T1B.VolumeSeriesRequestClaims.Claims[claim1_2.RequestID]
	assert.True(ok)
	assert.EqualValues(1000, swag.Int64Value(vce.SizeBytes))
	cd, err = cs.AddClaim(ctx, srN1T1B, claim1_2)
	assert.NoError(err)
	assert.NotNil(cd)
	cl, ok = cs.StorageRequests[string(srN1T1B.Meta.ID)]
	assert.True(ok)
	assert.Equal(cd, cl)
	assert.EqualValues(100000-claim1_2.SizeBytes, cd.RemainingBytes)
	assert.Len(cd.VSRClaims, 1)
	assert.Equal(claim1_2.RequestID, cd.VSRClaims[0].RequestID)
	assert.Equal(claim1_2.SizeBytes*2, cd.VSRClaims[0].SizeBytes)
	assert.EqualValues(srN1T1B.Meta.ID, fc.InSRUpdaterID)
	assert.NotNil(fc.InSRUpdaterItems)
	assert.Equal([]string{"volumeSeriesRequestClaims"}, fc.InSRUpdaterItems.Set)
	assert.Nil(fc.InSRUpdaterItems.Append)
	assert.Nil(fc.InSRUpdaterItems.Remove)
	vce, ok = srN1T1B.VolumeSeriesRequestClaims.Claims[claim1_2.RequestID]
	assert.True(ok)
	assert.EqualValues(2000, swag.Int64Value(vce.SizeBytes))
	assert.EqualValues(100000-claim1_2.SizeBytes, swag.Int64Value(srN1T1B.VolumeSeriesRequestClaims.RemainingBytes))

	clA = cs.FindClaimsByRequest("VSR-2")
	assert.Len(clA, 1)
	assert.Equal(cd, clA[0])

	// add a new claim
	claim1_3 := &VSRClaim{
		RequestID:  "VSR-3",
		SizeBytes:  1000,
		Annotation: "claim1_3",
	}
	cd, err = cs.AddClaim(ctx, srN1T1B, claim1_3)
	assert.NoError(err)
	assert.NotNil(cd)
	cl, ok = cs.StorageRequests[string(srN1T1B.Meta.ID)]
	assert.True(ok)
	assert.Equal(cd, cl)
	assert.EqualValues(100000-claim1_2.SizeBytes-claim1_3.SizeBytes, cd.RemainingBytes)
	assert.Len(cd.VSRClaims, 2)
	assert.Equal(claim1_2.RequestID, cd.VSRClaims[0].RequestID)
	assert.Equal(claim1_2.SizeBytes*2, cd.VSRClaims[0].SizeBytes)
	assert.Equal(claim1_3.RequestID, cd.VSRClaims[1].RequestID)
	assert.Equal(claim1_3.SizeBytes, cd.VSRClaims[1].SizeBytes)

	clA = cs.FindClaimsByRequest("VSR-3")
	assert.Len(clA, 1)
	assert.Equal(cd, clA[0])

	// add a VSR-1 claim to the second SR
	cd, err = cs.AddClaim(ctx, srN1T1B, claim1_1)
	assert.NoError(err)
	assert.NotNil(cd)
	cl, ok = cs.StorageRequests[string(srN1T1B.Meta.ID)]
	assert.True(ok)
	assert.Equal(cd, cl)
	assert.EqualValues(100000-claim1_2.SizeBytes-claim1_3.SizeBytes-claim1_1.SizeBytes, cd.RemainingBytes)
	assert.Len(cd.VSRClaims, 3)
	assert.Equal(claim1_2.RequestID, cd.VSRClaims[0].RequestID)
	assert.Equal(claim1_2.SizeBytes*2, cd.VSRClaims[0].SizeBytes)
	assert.Equal(claim1_3.RequestID, cd.VSRClaims[1].RequestID)
	assert.Equal(claim1_3.SizeBytes, cd.VSRClaims[1].SizeBytes)
	assert.Equal(claim1_1.RequestID, cd.VSRClaims[2].RequestID)
	assert.Equal(claim1_1.SizeBytes, cd.VSRClaims[2].SizeBytes)

	clA = cs.FindClaimsByRequest("VSR-1")
	assert.Len(clA, 2)

	vsrC, cd := clA.FindVSRClaimByAnnotation("VSR-1", "claim1_1")
	assert.NotNil(vsrC)
	assert.NotNil(cd)
	found := false
	for _, vsrC := range cd.VSRClaims {
		if vsrC.Annotation == "claim1_1" {
			found = true
			break
		}
	}
	assert.True(found)
	assert.Equal(claim1_1, vsrC)
	vsrC, cd = clA.FindVSRClaimByAnnotation("VSR-2", "claim1_2")
	assert.NotNil(vsrC)
	assert.NotNil(cd)
	found = false
	for _, vsrC := range cd.VSRClaims {
		if vsrC.Annotation == "claim1_2" {
			found = true
			break
		}
	}
	assert.True(found)
	assert.Equal(claim1_2.SizeBytes+1000, vsrC.SizeBytes)
	vsrC, cd = clA.FindVSRClaimByAnnotation("VSR-1", "foobar")
	assert.Nil(vsrC)
	assert.Nil(cd)

	// Create new SR claims for VSR-1
	srN1T2A := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{ID: "SR-N1-T2-A"},
		},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			PoolID: "SP-SSD",
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestCreateMutable: models.StorageRequestCreateMutable{
				NodeID: "NODE-1",
				VolumeSeriesRequestClaims: &models.VsrClaim{
					RemainingBytes: swag.Int64(10000),
					Claims: map[string]models.VsrClaimElement{
						"VSR-1": {SizeBytes: swag.Int64(1000), Annotation: "claim1_1"},
					},
				},
			},
		},
	}
	cd = cs.TrackStorageRequest(srN1T2A)
	assert.NotNil(cd)
	cl, ok = cs.StorageRequests[string(srN1T2A.Meta.ID)]
	assert.True(ok)
	assert.Equal(cd, cl)
	assert.Equal(srN1T2A, cd.StorageRequest)
	assert.EqualValues(srN1T2A.NodeID, cd.NodeID)
	assert.EqualValues(10000, cd.RemainingBytes)
	assert.Equal(*claim1_1, cd.VSRClaims[0])

	clA = cs.FindClaimsByRequest("VSR-1")
	assert.Len(clA, 3)

	srN2T1A := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{ID: "SR-N2-T1-A"},
		},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			PoolID: "SP-GP2",
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestCreateMutable: models.StorageRequestCreateMutable{
				NodeID: "NODE-2",
				VolumeSeriesRequestClaims: &models.VsrClaim{
					RemainingBytes: swag.Int64(10000),
					Claims: map[string]models.VsrClaimElement{
						"VSR-1": {SizeBytes: swag.Int64(1000), Annotation: "claim1_1"},
					},
				},
			},
		},
	}
	cd = cs.TrackStorageRequest(srN2T1A)
	assert.NotNil(cd)
	cl, ok = cs.StorageRequests[string(srN2T1A.Meta.ID)]
	assert.True(ok)
	assert.Equal(cd, cl)
	assert.Equal(srN2T1A, cd.StorageRequest)
	assert.EqualValues(srN2T1A.PoolID, cd.PoolID)
	assert.EqualValues(srN2T1A.NodeID, cd.NodeID)
	assert.EqualValues(10000, cd.RemainingBytes)
	assert.Equal(*claim1_1, cd.VSRClaims[0])

	// add failed SRs for each active SR.
	for _, k := range util.StringKeys(cs.StorageRequests) {
		cd := cs.StorageRequests[k]
		fsr := srClone(cd.StorageRequest)
		fsr.Meta.ID = models.ObjID(string(cd.StorageRequest.Meta.ID) + "-FAILED")
		fsr.StorageRequestState = com.StgReqStateFailed
		cs.TrackStorageRequest(fsr)
	}
	clA = cs.FindClaimsByRequest("VSR-1")
	assert.Len(clA, len(cs.StorageRequests))
	for _, cl := range clA {
		tl.Logger().Debugf("** %s %s %s %d %v", cl.StorageRequest.Meta.ID, cl.StorageRequest.PoolID, cl.NodeID, cl.RemainingBytes, cl.VSRClaims)
	}
	vsr1Claims := []struct {
		NodeID, SrID string
		SizeBytes    int64
		Prov         string
	}{
		{"NODE-1", "SR-N1-T1-A", 1000, "SP-GP2"},
		{"NODE-1", "SR-N1-T1-B", 1000, "SP-GP2"},
		{"NODE-2", "SR-N2-T1-A", 1000, "SP-GP2"},
		{"NODE-1", "SR-N1-T2-A", 1000, "SP-SSD"},
	}
	for _, tc := range vsr1Claims {
		found := -1
		for i, cl := range clA {
			if cl.NodeID == tc.NodeID && string(cl.StorageRequest.Meta.ID) == tc.SrID && cl.PoolID == tc.Prov {
				found = i
				sz := int64(0)
				for _, v := range cl.VSRClaims {
					if v.RequestID == "VSR-1" {
						sz = v.SizeBytes
					}
				}
				assert.Equal(tc.SizeBytes, sz, "Found N=%s SR=%s P='%s' Sz=%d", tc.NodeID, tc.SrID, tc.Prov, tc.SizeBytes)
				break
			}
		}
		assert.True(found >= 0, "Found N=%s SR=%s P='%s' Sz=%d", tc.NodeID, tc.SrID, tc.Prov, tc.SizeBytes)
	}
	// Failed SRs should be skipped
	clA = cs.FindClaimsByPool("SP-GP2", 1000, "NODE-1", false)
	assert.Len(clA, 1)
	assert.EqualValues("SR-N1-T1-B", clA[0].StorageRequest.Meta.ID)
	for _, cd := range clA {
		assert.True(cd.StorageRequest.StorageRequestState != com.StgReqStateFailed)
		assert.EqualValues("NODE-1", cd.NodeID)
	}
	clA = cs.FindClaimsByPool("SP-GP2", 1000, "NODE-2", true)
	assert.Len(clA, 1)
	assert.EqualValues("SR-N1-T1-B", clA[0].StorageRequest.Meta.ID)
	for _, cd := range clA {
		assert.True(cd.StorageRequest.StorageRequestState != com.StgReqStateFailed)
		assert.EqualValues("NODE-1", cd.NodeID)
	}
	clA = cs.FindClaimsByPool("SP-GP2", 1000, "NODE-2", false)
	assert.Len(clA, 1)
	assert.EqualValues("SR-N2-T1-A", clA[0].StorageRequest.Meta.ID)
	for _, cd := range clA {
		assert.True(cd.StorageRequest.StorageRequestState != com.StgReqStateFailed)
		assert.EqualValues("NODE-2", cd.NodeID)
	}
	clA = cs.FindClaimsByPool("SP-GP2", 1000, "NODE-1", true)
	assert.Len(clA, 1)
	assert.EqualValues("SR-N2-T1-A", clA[0].StorageRequest.Meta.ID)
	for _, cd := range clA {
		assert.True(cd.StorageRequest.StorageRequestState != com.StgReqStateFailed)
		assert.EqualValues("NODE-2", cd.NodeID)
	}
	clA = cs.FindClaimsByPool("SP-SSD", 1000, "NODE-2", false)
	assert.Len(clA, 0)

	// test AddClaim update invocation errors
	tl.Logger().Debugf("*** AddClaim failure test - no callback")
	tl.Flush()
	origCM := cloneClaimM(cs.StorageRequests)
	fc.RetSRUpdaterErr = fmt.Errorf("sr-updater-error")
	cd, err = cs.AddClaim(ctx, srN2T1A, claim1_3)
	assert.Error(err)
	assert.Regexp("sr-updater-error", err)
	assert.Nil(cd)
	assert.Equal(origCM, cs.StorageRequests)

	tl.Logger().Debugf("*** AddClaim failure test - callback with new claim")
	tl.Flush()
	cl, ok = cs.StorageRequests[string(srN2T1A.Meta.ID)]
	assert.True(ok)
	assert.Equal(srN2T1A, cl.StorageRequest)
	_, ok = srN2T1A.VolumeSeriesRequestClaims.Claims[claim1_3.RequestID] // this is a new claim
	assert.False(ok)
	fc.RetSRUpdaterObj = nil
	fc.RetSRUpdaterErr = nil
	fc.RetSRUpdaterUpdateErr = fmt.Errorf("sr-updater-error2")
	cd, err = cs.AddClaim(ctx, srN2T1A, claim1_3)
	assert.Error(err)
	assert.Regexp("sr-updater-error2", err)
	assert.Nil(cd)
	assert.Equal(origCM, cs.StorageRequests)

	tl.Logger().Debugf("*** AddClaim failure test - callback modifying existing claim")
	tl.Flush()
	_, ok = srN2T1A.VolumeSeriesRequestClaims.Claims[claim1_1.RequestID] // this is an existing claim
	assert.True(ok)
	cd, err = cs.AddClaim(ctx, srN2T1A, claim1_1)
	assert.Error(err)
	assert.Regexp("sr-updater-error2", err)
	assert.Nil(cd)
	assert.Equal(origCM, cs.StorageRequests)

	// SR with no claims gets initialized internally
	srNoClaims := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{ID: "SR-NO-CLAIMS"},
		},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			PoolID: "SP-GP2",
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestCreateMutable: models.StorageRequestCreateMutable{
				NodeID: "NODE-2",
			},
		},
	}
	cd = cs.TrackStorageRequest(srNoClaims)
	assert.NotNil(cd)
	assert.NotNil(cd.StorageRequest.VolumeSeriesRequestClaims) // allocated internally
	assert.Len(cd.VSRClaims, 0)

	// remove VSR1 claims
	cs.RemoveClaims("VSR-1")
	clA = cs.FindClaimsByRequest("VSR-1")
	assert.Len(clA, 0)

	clA = cs.FindClaimsByRequest("VSR-2")
	assert.Len(clA, 2)
	for _, cd := range clA {
		assert.Equal(cs, cd.cs)
	}
	clA = cs.FindClaimsByRequest("VSR-3")
	assert.Len(clA, 2)

	// RemoveClaim no-op
	assert.Len(clA[0].VSRClaims, 2)
	clA[0].RemoveClaim("VSR-no-match")
	assert.Len(clA[0].VSRClaims, 2)

	cs.RemoveClaims("VSR-2")
	cs.RemoveClaims("VSR-3")
	for _, cd := range cs.StorageRequests {
		assert.Empty(cd.VSRClaims)
	}
}

func TestStorageSearch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	args := &ClusterStateArgs{
		Log: tl.Logger(),
	}
	cs := NewClusterState(args)

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
		"STORAGE-2": &models.Storage{
			StorageAllOf0: models.StorageAllOf0{
				Meta:   &models.ObjMeta{ID: "STORAGE-2"},
				PoolID: "SP-GP2",
			},
			StorageMutable: models.StorageMutable{
				AvailableBytes: swag.Int64(5000),
				StorageState: &models.StorageStateMutable{
					AttachedNodeID: "NODE-2",
				},
				ShareableStorage: true,
			},
		},
		"STORAGE-3": &models.Storage{
			StorageAllOf0: models.StorageAllOf0{
				Meta:   &models.ObjMeta{ID: "STORAGE-3"},
				PoolID: "SP-SSD",
			},
			StorageMutable: models.StorageMutable{
				AvailableBytes: swag.Int64(10000),
				StorageState: &models.StorageStateMutable{
					AttachedNodeID: "NODE-1",
				},
				ShareableStorage: true,
			},
		},
		"STORAGE-4": &models.Storage{
			StorageAllOf0: models.StorageAllOf0{
				Meta:   &models.ObjMeta{ID: "STORAGE-4"},
				PoolID: "SP-GP2",
			},
			StorageMutable: models.StorageMutable{
				AvailableBytes:   swag.Int64(1000), // unused
				TotalParcelCount: swag.Int64(10),
				ParcelSizeBytes:  swag.Int64(100),
				StorageState: &models.StorageStateMutable{
					AttachedNodeID: "NODE-2",
				},
				ShareableStorage: false, // previously not shareable
			},
		},
		"STORAGE-5": &models.Storage{
			StorageAllOf0: models.StorageAllOf0{
				Meta:   &models.ObjMeta{ID: "STORAGE-5"},
				PoolID: "SP-GP2",
			},
			StorageMutable: models.StorageMutable{
				AvailableBytes:   swag.Int64(10), // in use
				TotalParcelCount: swag.Int64(10),
				ParcelSizeBytes:  swag.Int64(100),
				StorageState: &models.StorageStateMutable{
					AttachedNodeID: "NODE-2",
				},
				ShareableStorage: false, // not shareable
			},
		},
	}
	cs.DetachedStorage = map[string]*models.Storage{
		"STORAGE-DET-1": &models.Storage{
			StorageAllOf0: models.StorageAllOf0{
				Meta:   &models.ObjMeta{ID: "STORAGE-DET-1"},
				PoolID: "SP-GP2",
			},
			StorageMutable: models.StorageMutable{
				AvailableBytes:   swag.Int64(1000), // unused
				TotalParcelCount: swag.Int64(10),
				ParcelSizeBytes:  swag.Int64(100),
				StorageState: &models.StorageStateMutable{
					AttachmentState: "DETACHED",
				},
			},
		},
	}
	cs.Nodes = map[string]*NodeData{
		"NODE-1": &NodeData{
			Node: &models.Node{
				NodeAllOf0: models.NodeAllOf0{
					Meta: &models.ObjMeta{ID: "NODE-1"},
				},
			},
			AttachedStorage: []string{"STORAGE-1", "STORAGE-3"},
		},
		"NODE-2": &NodeData{
			Node: &models.Node{
				NodeAllOf0: models.NodeAllOf0{
					Meta: &models.ObjMeta{ID: "NODE-2"},
				},
			},
			AttachedStorage: []string{"STORAGE-2"},
		},
	}

	// FindAllStorage
	sA := cs.FindAllStorage(func(*models.Storage) bool { return true })
	assert.True(len(cs.Storage)+len(cs.DetachedStorage) == len(sA))
	for _, sObj := range sA {
		var hasA, hasD bool
		k := string(sObj.Meta.ID)
		if _, hasA = cs.Storage[k]; !hasA {
			_, hasD = cs.DetachedStorage[k]
		}
		assert.True(hasA || hasD, "storage %s", k)
	}

	getStorageIDs := func(l []*models.Storage) []string {
		ret := []string{}
		for _, o := range l {
			ret = append(ret, string(o.Meta.ID))
		}
		sort.Strings(ret)
		return ret
	}
	// FindShareableStorageByPool
	sA = cs.FindShareableStorageByPool("SP-GP2", 1000, "NODE-2", false)
	assert.Len(sA, 2)
	assert.Equal([]string{"STORAGE-2", "STORAGE-4"}, getStorageIDs(sA))
	for _, s := range sA {
		assert.EqualValues("SP-GP2", s.PoolID)
		assert.EqualValues("NODE-2", s.StorageState.AttachedNodeID)
		lsObj, lsA := cs.LookupStorage(string(s.Meta.ID))
		assert.Equal(s, lsObj)
		assert.True(lsA)
	}
	sA = cs.FindShareableStorageByPool("SP-GP2", 6000, "NODE-2", false)
	assert.Len(sA, 0)
	sA = cs.FindShareableStorageByPool("SP-GP2", 1000, "NODE-1", true)
	assert.Len(sA, 2)
	assert.Equal([]string{"STORAGE-2", "STORAGE-4"}, getStorageIDs(sA))
	for _, s := range sA {
		assert.EqualValues("SP-GP2", s.PoolID)
		assert.EqualValues("NODE-2", s.StorageState.AttachedNodeID)
	}

	// skipping claimed storage
	cs.claimedStorage = map[string]struct{}{}
	cs.claimedStorage["STORAGE-2"] = struct{}{}
	sA = cs.FindShareableStorageByPool("SP-GP2", 1000, "NODE-1", true)
	assert.Len(sA, 1)
	assert.Equal([]string{"STORAGE-4"}, getStorageIDs(sA))
	delete(cs.claimedStorage, "STORAGE-2")

	sA = cs.FindShareableStorageByPool("SP-GP2", 6000, "NODE-1", true)
	assert.Len(sA, 0)

	// FindUnusedStorageByPool
	sA = cs.FindUnusedStorageByPool("SP-GP2", 1000, "NODE-2", false)
	assert.Len(sA, 1)
	assert.Equal([]string{"STORAGE-4"}, getStorageIDs(sA))
	lsObj, lsA := cs.LookupStorage(string(sA[0].Meta.ID))
	assert.Equal(sA[0], lsObj)
	assert.True(lsA)
	sA = cs.FindUnusedStorageByPool("SP-GP2", 1000, "NODE-2", true)
	assert.Len(sA, 1)
	assert.Equal([]string{"STORAGE-1"}, getStorageIDs(sA))

	// skipping claimed storage
	cs.claimedStorage["STORAGE-1"] = struct{}{}
	sA = cs.FindUnusedStorageByPool("SP-GP2", 1000, "NODE-2", true)
	assert.Len(sA, 0)
	delete(cs.claimedStorage, "STORAGE-1")

	// LookupStorage error
	lsObj, lsA = cs.LookupStorage("foo")
	assert.Nil(lsObj)

	// look up detached storage
	lsObj, lsA = cs.LookupStorage("STORAGE-DET-1")
	assert.NotNil(lsObj)
	assert.False(lsA)
}

func TestNodeSearch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	args := &ClusterStateArgs{
		Log: tl.Logger(),
	}
	cs := NewClusterState(args)

	cs.Nodes = map[string]*NodeData{
		"NODE-1": &NodeData{
			Node: &models.Node{
				NodeAllOf0: models.NodeAllOf0{
					Meta: &models.ObjMeta{ID: "NODE-1"},
				},
				NodeMutable: models.NodeMutable{
					Service: &models.NuvoService{
						ServiceState: models.ServiceState{
							State: com.ServiceStateReady,
						},
					},
				},
			},
			AttachedStorage: []string{"STORAGE-1"},
		},
		"NODE-2": &NodeData{
			Node: &models.Node{
				NodeAllOf0: models.NodeAllOf0{
					Meta: &models.ObjMeta{ID: "NODE-2"},
				},
			},
			AttachedStorage: []string{"STORAGE-2"},
		},
	}

	expN := []string{"NODE-1", "NODE-2"}
	nA := cs.FindNodes(func(n *NodeData) bool { return true })
	assert.Len(nA, len(expN))
	gotN := []string{}
	for _, nObj := range nA {
		gotN = append(gotN, string(nObj.Meta.ID))
		assert.Equal(com.ServiceStateUnknown, nObj.Service.State)
	}
	sort.Strings(gotN)
	assert.Equal(expN, gotN)
}

func TestGetHealthyNode(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	args := &ClusterStateArgs{
		Log: tl.Logger(),
	}
	cs := NewClusterState(args)

	cs.Nodes = map[string]*NodeData{
		"N1": &NodeData{Node: &models.Node{}},
		"N2": &NodeData{Node: &models.Node{}},
		"N3": &NodeData{Node: &models.Node{}},
	}
	cs.Nodes["N1"].Node.Meta = &models.ObjMeta{ID: "N1"}
	cs.Nodes["N1"].AttachedStorage = []string{"S10"}
	cs.Nodes["N2"].AttachedStorage = []string{"S20", "S21"}
	cs.Nodes["N2"].Node.Meta = &models.ObjMeta{ID: "N2"}
	cs.Nodes["N3"].AttachedStorage = []string{"S30", "S31", "S32"}
	cs.Nodes["N3"].Node.Meta = &models.ObjMeta{ID: "N3"}

	ss := &models.ServiceState{State: com.ServiceStateReady}
	cs.NodeServiceState("N1", ss)
	cs.NodeServiceState("N2", ss)
	cs.NodeServiceState("N3", ss)

	// preferred
	nObj := cs.GetHealthyNode("N2")
	assert.NotNil(nObj)
	assert.Equal(cs.Nodes["N2"].Node, nObj)

	// next choice
	ss.State = "STOPPED"
	cs.NodeServiceState("N2", ss)
	nObj = cs.GetHealthyNode("N2")
	assert.NotNil(nObj)
	assert.Equal(cs.Nodes["N1"].Node, nObj)

	// next choice
	cs.NodeServiceState("N1", ss)
	nObj = cs.GetHealthyNode("N2")
	assert.NotNil(nObj)
	assert.Equal(cs.Nodes["N3"].Node, nObj)

	// no active nodes
	cs.NodeServiceState("N3", ss)
	nObj = cs.GetHealthyNode("N2")
	assert.Nil(nObj)
}

func TestStorageClaimed(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	args := &ClusterStateArgs{
		Log: tl.Logger(),
	}
	cs := NewClusterState(args)

	cs.claimedStorage = make(map[string]struct{})

	ret := cs.storageClaimed("id")
	assert.False(ret)

	cs.claimedStorage["id"] = struct{}{}
	ret = cs.storageClaimed("id")
	assert.True(ret)
}
