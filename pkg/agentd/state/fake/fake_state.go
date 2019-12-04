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

	ns "github.com/Nuvoloso/kontroller/pkg/agentd/state"
)

// NodeState provides a fake ns.NodeState object for testing
type NodeState struct {
	ns.NodeState

	// DumpState
	RetDS *bytes.Buffer
	DSCnt int

	// ClaimCache
	InClaimCacheCtx  context.Context
	InClaimCacheVsID string
	InClaimCacheDsb  int64 // desired size
	InClaimCacheMsb  int64 // min size
	RetClaimCacheUsb int64
	RetClaimCacheErr error

	// GetClaim
	InGetClaimCtx        context.Context
	InGetClaimVsID       string
	RetGetClaimSizeBytes int64

	// ReleaseCache
	InReleaseCacheCtx  context.Context
	InReleaseCacheVsID string

	// Reload
	InRCtx  context.Context
	RetRChg bool
	RetRErr error
	RCnt    int

	// GetNodeAvailableCacheBytes
	InGetNodeAvailableCacheBytesCtx context.Context
	RetGetNodeAvailableCacheBytes   int64

	// UpdateNodeAvailableCache
	InUpdateNodeAvailableCacheCtx  context.Context
	RetUpdateNodeAvailableCacheErr error

	// GetNodeCacheAllocationUnitSizeBytes
	RetGNCAllocationUnitSizeBytes int64
}

// NewFakeNodeState returns a new fake NodeState
func NewFakeNodeState() *NodeState {
	s := &NodeState{}
	s.CacheAvailability = &ns.NodeCacheUsage{}
	s.VSCacheUsage = make(map[string]*ns.VolumeSeriesNodeCacheUsage)
	return s
}

var _ = ns.NodeStateOperators(&NodeState{})

// NS fakes the interface of the same name
func (s *NodeState) NS() *ns.NodeState {
	return &s.NodeState
}

// DumpState fakes the interface of the same name
func (s *NodeState) DumpState() *bytes.Buffer {
	s.DSCnt++
	return s.RetDS
}

// Reload fakes the interface of the same name
func (s *NodeState) Reload(ctx context.Context) (bool, error) {
	s.InRCtx = ctx
	s.RCnt++
	return s.RetRChg, s.RetRErr
}

// GetClaim fakes the interface of the same name
func (s *NodeState) GetClaim(ctx context.Context, vsID string) int64 {
	s.InGetClaimCtx = ctx
	s.InGetClaimVsID = vsID
	return s.RetGetClaimSizeBytes
}

// ClaimCache fakes the interface of the same name
func (s *NodeState) ClaimCache(ctx context.Context, vsID string, desiredSizeBytes, minSizeBytes int64) (int64, error) {
	s.InClaimCacheCtx = ctx
	s.InClaimCacheVsID = vsID
	s.InClaimCacheDsb = desiredSizeBytes
	s.InClaimCacheMsb = minSizeBytes
	return s.RetClaimCacheUsb, s.RetClaimCacheErr
}

// ReleaseCache fakes the interface of the same name
func (s *NodeState) ReleaseCache(ctx context.Context, vsID string) {
	s.InReleaseCacheCtx = ctx
	s.InReleaseCacheVsID = vsID
}

// GetNodeAvailableCacheBytes fakes the interface of the same name
func (s *NodeState) GetNodeAvailableCacheBytes(ctx context.Context) int64 {
	s.InGetNodeAvailableCacheBytesCtx = ctx
	return s.RetGetNodeAvailableCacheBytes
}

// UpdateNodeAvailableCache fakes the interface of the same name
func (s *NodeState) UpdateNodeAvailableCache(ctx context.Context) error {
	s.InUpdateNodeAvailableCacheCtx = ctx
	return s.RetUpdateNodeAvailableCacheErr
}

// GetNodeCacheAllocationUnitSizeBytes fakes the interface of the same name
func (s *NodeState) GetNodeCacheAllocationUnitSizeBytes() int64 {
	return s.RetGNCAllocationUnitSizeBytes
}
