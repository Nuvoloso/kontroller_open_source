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


package fake

import (
	"bytes"
	"context"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	cs "github.com/Nuvoloso/kontroller/pkg/clusterd/state"
	"github.com/Nuvoloso/kontroller/pkg/layout"
)

// ClusterState provides a fake cs.ClusterState object for testing
type ClusterState struct {
	cs.ClusterState

	// AddClaim
	InACCtx  context.Context
	InACsr   *models.StorageRequest
	InACcl   *cs.VSRClaim
	RetACObj *cs.ClaimData
	RetACErr error

	// DumpState
	RetDS *bytes.Buffer
	DSCnt int

	// FindAllStorage
	CallRealFindAllStorage bool
	RetFasObj              []*models.Storage

	// FindClaimByStorageRequest
	InFCBySRid   string
	RetFCBySRObj *cs.ClaimData

	// FindClaims
	CallRealFindClaims bool
	RetFCObj           []*cs.ClaimData

	// FindClaimsByRequest
	InFCByR     string
	RetFCByRObj []*cs.ClaimData

	// FindClaimsByPool
	InFCByPid   string
	InFCByPsz   int64
	InFCByPnID  string
	RetFCByPObj []*cs.ClaimData

	// FindNodes
	RetFNObj []*models.Node

	// FindPlacementAlgorithm
	InFPAlayout models.StorageLayout
	RetFPAalgo  *layout.Algorithm

	// FindStorage
	CallRealFindStorage bool
	RetFSObj            []*models.Storage

	// FindStorageByPool
	InFSByPid   string
	InFSbyTsz   int64
	InFSByPnID  string
	RetFSByPObj []*models.Storage

	// GetHealthyNode
	InGHNid   string
	RetGHNobj *models.Node

	// LookupStorage
	CallRealLookupStorage bool
	InLSid                string
	RetLSObj              *models.Storage
	RetLSisAttached       bool

	// NodeDeleted
	InNDid string

	// NodeGetServiceState
	InNGSid  string
	RetNGSnh []*cs.NodeHealth

	// NodePurgeMissing
	InNPMt    time.Time
	RetNPMcnt int
	RetNPMadd int

	// NodeServiceState
	InNSSid string
	InNSSss *models.ServiceState

	// NodeUpdateState
	InNUSobj *models.Node
	InNUSt   time.Time

	// ReleaseUnusedStorage
	InRUSTimeout time.Duration
	RUSCnt       int

	// Reload
	InRCtx  context.Context
	RetRChg bool
	RetRErr error
	RCnt    int

	// RemoveClaims
	InRCReq string

	// SelectStorage
	InSSCtx   context.Context
	InSSArgs  *cs.SelectStorageArgs
	RetSSResp *cs.SelectStorageResponse
	RetSSErr  error

	// TrackStorageRequest
	InTSRsr  *models.StorageRequest
	RetTSRcd *cs.ClaimData
}

// NewFakeClusterState returns a new fake ClusterState
func NewFakeClusterState() *ClusterState {
	c := &ClusterState{}
	c.Nodes = make(map[string]*cs.NodeData)
	c.Storage = make(map[string]*models.Storage)
	c.DetachedStorage = make(map[string]*models.Storage)
	c.StorageRequests = make(map[string]*cs.ClaimData)
	return c
}

var _ = cs.ClusterStateOperators(&ClusterState{})

// AddClaim fakes its namesake
func (c *ClusterState) AddClaim(ctx context.Context, sr *models.StorageRequest, claim *cs.VSRClaim) (*cs.ClaimData, error) {
	c.InACCtx = ctx
	c.InACsr = sr
	c.InACcl = claim
	return c.RetACObj, c.RetACErr
}

// CS fakes its namesake
func (c *ClusterState) CS() *cs.ClusterState {
	return &c.ClusterState
}

// DumpState fakes its namesake
func (c *ClusterState) DumpState() *bytes.Buffer {
	c.DSCnt++
	return c.RetDS
}

// FindAllStorage fakes its namesake
func (c *ClusterState) FindAllStorage(selector func(*models.Storage) bool) []*models.Storage {
	if c.CallRealFindAllStorage {
		return c.ClusterState.FindAllStorage(selector)
	}
	return c.RetFasObj
}

// FindClaimByStorageRequest fakes its namesake
func (c *ClusterState) FindClaimByStorageRequest(srID string) *cs.ClaimData {
	c.InFCBySRid = srID
	return c.RetFCBySRObj
}

// FindClaims fakes its namesake
func (c *ClusterState) FindClaims(selector func(*cs.ClaimData) bool) cs.ClaimList {
	if c.CallRealFindClaims {
		return c.ClusterState.FindClaims(selector)
	}
	return c.RetFCObj
}

// FindClaimsByRequest fakes its namesake
func (c *ClusterState) FindClaimsByRequest(requestID string) cs.ClaimList {
	c.InFCByR = requestID
	return c.RetFCByRObj
}

// FindNodes fakes its namesake
func (c *ClusterState) FindNodes(selector func(*cs.NodeData) bool) []*models.Node {
	return c.RetFNObj
}

// FindLayoutAlgorithm fakes its namesake
func (c *ClusterState) FindLayoutAlgorithm(layout models.StorageLayout) *layout.Algorithm {
	c.InFPAlayout = layout
	return c.RetFPAalgo
}

// FindStorage fakes its namesake
func (c *ClusterState) FindStorage(selector func(*models.Storage) bool) []*models.Storage {
	if c.CallRealFindStorage {
		return c.ClusterState.FindStorage(selector)
	}
	return c.RetFSObj
}

// GetHealthyNode fakes its namesake
func (c *ClusterState) GetHealthyNode(preferredNodeID string) *models.Node {
	c.InGHNid = preferredNodeID
	return c.RetGHNobj
}

// LookupStorage fakes its namesake
func (c *ClusterState) LookupStorage(id string) (*models.Storage, bool) {
	c.InLSid = id
	if c.CallRealLookupStorage {
		return c.ClusterState.LookupStorage(id)
	}
	return c.RetLSObj, c.RetLSisAttached
}

// NodeDeleted fakes its namesake
func (c *ClusterState) NodeDeleted(nid string) {
	c.InNDid = nid
}

// NodeGetServiceState fakes its namesake
func (c *ClusterState) NodeGetServiceState(nid string) []*cs.NodeHealth {
	c.InNGSid = nid
	return c.RetNGSnh
}

// NodePurgeMissing fakes its namesake
func (c *ClusterState) NodePurgeMissing(epoch time.Time) (int, int) {
	c.InNPMt = epoch
	return c.RetNPMcnt, c.RetNPMadd
}

// NodeServiceState fakes its namesake
func (c *ClusterState) NodeServiceState(nid string, ss *models.ServiceState) {
	c.InNSSid = nid
	c.InNSSss = ss
}

// NodeUpdateState fakes its namesake
func (c *ClusterState) NodeUpdateState(node *models.Node, epoch time.Time) {
	c.InNUSobj = node
	c.InNUSt = epoch
}

// ReleaseUnusedStorage fakes its namesake
func (c *ClusterState) ReleaseUnusedStorage(ctx context.Context, timeout time.Duration) {
	c.InRUSTimeout = timeout
	c.RUSCnt++
}

// Reload fakes its namesake
func (c *ClusterState) Reload(ctx context.Context) (bool, error) {
	c.InRCtx = ctx
	c.RCnt++
	return c.RetRChg, c.RetRErr
}

// RemoveClaims fakes its namesake
func (c *ClusterState) RemoveClaims(requestID string) {
	c.InRCReq = requestID
}

// SelectStorage fakes its namesake
func (c *ClusterState) SelectStorage(ctx context.Context, args *cs.SelectStorageArgs) (*cs.SelectStorageResponse, error) {
	c.InSSCtx = ctx
	c.InSSArgs = args
	return c.RetSSResp, c.RetSSErr
}

// TrackStorageRequest fakes its namesake
func (c *ClusterState) TrackStorageRequest(sr *models.StorageRequest) *cs.ClaimData {
	c.InTSRsr = sr
	return c.RetTSRcd
}
