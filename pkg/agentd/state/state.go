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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
	logging "github.com/op/go-logging"
)

// CacheError returns an error message and meets the error interface definition
// All the operations return this type of error object
type CacheError struct {
	Message string // message
}

func (e *CacheError) Error() string {
	return e.Message
}

// ErrorInsufficientCache returned if no sufficient cache is available for allocation
// (note that cache might still be available but there is not enough to satisfy
// minimun required amount for the request)
var ErrorInsufficientCache = &CacheError{com.ErrorInsufficientCache}

// NodeCacheUsage reflects the total used cache size and available cache size on the node
type NodeCacheUsage struct {
	UsedSizeBytes      int64
	AvailableSizeBytes int64
}

// VolumeSeriesNodeCacheUsage reflects node cache usage by different volume series
type VolumeSeriesNodeCacheUsage struct {
	UsedSizeBytes int64
}

// NodeStateArgs contains data required to create the NodeState object
type NodeStateArgs struct {
	// Database update interface
	OCrud crud.Ops
	// A logger
	Log *logging.Logger
	// The node object concerned
	Node *models.Node
}

// Verify the arguments
func (nsa NodeStateArgs) Verify() bool {
	if nsa.OCrud == nil || nsa.Node == nil || nsa.Log == nil {
		return false
	}
	return true
}

// NodeState contains data on the state of cache usage at some point in time:
// - tracks the cache available on the node, initialized during cache discovery
// - tracks per-VS cache usage
type NodeState struct {
	NodeStateArgs
	// information on the cache availability on the node
	CacheAvailability *NodeCacheUsage
	// Map of VS ID to amount of cache used on the node
	VSCacheUsage map[string]*VolumeSeriesNodeCacheUsage
	// Misc
	NumLoads int
}

// NodeStateOperators are the exported methods on NodeState
type NodeStateOperators interface {
	// the following require external serialization
	NS() *NodeState
	DumpState() *bytes.Buffer
	Reload(ctx context.Context) (bool, error)
	GetClaim(ctx context.Context, vsID string) int64
	ClaimCache(ctx context.Context, vsID string, desiredSizeBytes, minSizeBytes int64) (int64, error)
	ReleaseCache(ctx context.Context, vsID string)
	UpdateNodeAvailableCache(ctx context.Context) error
	UpdateNodeObj(ctx context.Context, node *models.Node)
	GetNodeAvailableCacheBytes(ctx context.Context) int64
	GetNodeCacheAllocationUnitSizeBytes() int64
}

var _ = NodeStateOperators(&NodeState{})

// NewNodeState creates and initializes the NodeState object
func NewNodeState(args *NodeStateArgs) *NodeState {
	ns := &NodeState{}
	ns.NodeStateArgs = *args
	ns.CacheAvailability = &NodeCacheUsage{}
	ns.VSCacheUsage = make(map[string]*VolumeSeriesNodeCacheUsage)
	return ns
}

// New returns NodeStateOperators
func New(args *NodeStateArgs) NodeStateOperators {
	if !args.Verify() {
		return nil
	}
	return NewNodeState(args)
}

// NS is an identity method
func (ns *NodeState) NS() *NodeState {
	return ns
}

// DumpState returns a printable dump of the state
func (ns *NodeState) DumpState() *bytes.Buffer {
	var buf bytes.Buffer
	var b = &buf
	fmt.Fprintf(b, "Iteration:%d #VS:%d Cache bytes used/available: %s/%s\n", ns.NumLoads, len(ns.VSCacheUsage),
		util.SizeBytes(ns.CacheAvailability.UsedSizeBytes), util.SizeBytes(ns.CacheAvailability.AvailableSizeBytes))
	vsIDS := util.SortedStringKeys(ns.VSCacheUsage)
	for _, vsID := range vsIDS {
		fmt.Fprintf(b, "   VS[%s] %s\n", vsID, util.SizeBytes(ns.VSCacheUsage[vsID].UsedSizeBytes))
	}
	return b
}

// Reload node state
func (ns *NodeState) Reload(ctx context.Context) (bool, error) {
	vsM, usedSizeBytes, err := ns.loadVolumeSeriesData(ctx)
	if err != nil {
		return false, err
	}
	// assemble the new state
	newNS := NewNodeState(&ns.NodeStateArgs)
	newNS.CacheAvailability = &NodeCacheUsage{}
	newNS.CacheAvailability.UsedSizeBytes = usedSizeBytes
	newNS.CacheAvailability.AvailableSizeBytes = swag.Int64Value(ns.NodeStateArgs.Node.TotalCacheBytes) - usedSizeBytes
	newNS.VSCacheUsage = vsM

	ns.NumLoads++
	changed := ns.stateChanged(newNS)
	if changed {
		ns.CacheAvailability = newNS.CacheAvailability
		ns.VSCacheUsage = newNS.VSCacheUsage
	}
	return changed, nil
}

// GetNodeCacheAllocationUnitSizeBytes returns cache allocation unit size of the Node object
func (ns *NodeState) GetNodeCacheAllocationUnitSizeBytes() int64 {
	return swag.Int64Value(ns.NodeStateArgs.Node.CacheUnitSizeBytes)
}

type allocUnitFn func(ns *NodeState) int64

// allocUnitHook can be replaced during UT
var allocUnitHook allocUnitFn = GetNodeCacheAllocationUnitSizeBytes

// GetNodeCacheAllocationUnitSizeBytes returns cache allocation unit size of the Node object
func GetNodeCacheAllocationUnitSizeBytes(ns *NodeState) int64 {
	return ns.GetNodeCacheAllocationUnitSizeBytes()
}

// GetClaim returns the current claim held by the given VS
func (ns *NodeState) GetClaim(ctx context.Context, vsID string) int64 {
	if cu, _ := ns.VSCacheUsage[vsID]; cu != nil {
		return cu.UsedSizeBytes
	}
	return 0
}

// ClaimCache updates state of node cache usage.
// Returns actual allocated bytes (either requested desired size or remaining available cache size
// if there is no sufficient amount present in case it satisfies minimum required specification)
// and an error if there is no sufficient cache amount is available.
// The LMDGuard lock should be held.
//
// Note: Since Storelandia operates on cache sizes in multiple of block size of cacheAllocationUnitSizeBytes,
// any cache allocations should take that into consideration and round up actual allocations
// (e.g. when desired size bytes are available the rounded value should actually be used,
// same case with allocating less than required).
//
// ClaimCache does nothing if the desiredSizeBytes is already claimed. If the desiredSizeBytes differs from the
// current claim, either increased or decreased, the current claim will be updated. If the new claim cannot be
// granted (ErrorInsufficientCache) the current claim is left unchanged. The call will never fail when the
// desiredSizeBytes is less than or equal to the current claim. In addition, as long as the LMD lock is held,
// a decrease followed by an increase back to the original claim will not fail.
//
// Note: Node object will be updated by the VOLUME_EXPORT VSR, VS object will be updated after it.
func (ns *NodeState) ClaimCache(ctx context.Context, vsID string, desiredSizeBytes, minSizeBytes int64) (int64, error) {
	var curUsed int64
	if cu, _ := ns.VSCacheUsage[vsID]; cu != nil {
		if desiredSizeBytes == cu.UsedSizeBytes {
			ns.Log.Debugf("Cache [%d] has already been allocated for VolumeSeries [%s]", cu.UsedSizeBytes, vsID)
			return desiredSizeBytes, nil
		}
		curUsed = cu.UsedSizeBytes
	}
	cacheUnitSizeBytes := allocUnitHook(ns)
	usedSizeBytes := RoundSizeBytesToBlock(desiredSizeBytes, cacheUnitSizeBytes, false) - curUsed
	if usedSizeBytes+curUsed == 0 {
		ns.ReleaseCache(ctx, vsID)
		return 0, nil
	}
	// When claim is decreasing usedSizeBytes will be negative, and AvailableSizeBytes is never negative, so no error is possible
	// When the claim is restoring a previous decrease and the LMD lock is held, the required claim will still be available, so no error is possible
	if usedSizeBytes > ns.CacheAvailability.AvailableSizeBytes {
		usedSizeBytes = RoundSizeBytesToBlock(ns.CacheAvailability.AvailableSizeBytes, cacheUnitSizeBytes, true)
		if minSizeBytes <= usedSizeBytes {
			ns.Log.Debugf("No desired cache size [%d] is available, remaining available size [%d] adjusted to cache allocation unit size [%d] will be allocated [%d]",
				desiredSizeBytes, ns.CacheAvailability.AvailableSizeBytes, cacheUnitSizeBytes, usedSizeBytes)
		} else {
			ns.Log.Errorf("Neither desired cache size [%d] nor min acceptable size [%d] is available (total remaining: %d)",
				desiredSizeBytes, minSizeBytes, ns.CacheAvailability.AvailableSizeBytes)
			return 0, ErrorInsufficientCache
		}
	}
	ns.VSCacheUsage[vsID] = &VolumeSeriesNodeCacheUsage{
		UsedSizeBytes: usedSizeBytes + curUsed,
	}
	ns.CacheAvailability.UsedSizeBytes += usedSizeBytes
	ns.CacheAvailability.AvailableSizeBytes -= usedSizeBytes
	return usedSizeBytes + curUsed, nil
}

// ReleaseCache updates state of node cache usage, releasing any previous cache allocation for a VolumeSeries
func (ns *NodeState) ReleaseCache(ctx context.Context, vsID string) {
	if cu, ok := ns.VSCacheUsage[vsID]; !ok {
		ns.Log.Debugf("VolumeSeries [%s] has no cache claim present", vsID)
	} else {
		curUsed := cu.UsedSizeBytes
		delete(ns.VSCacheUsage, vsID)
		ns.CacheAvailability.UsedSizeBytes -= curUsed
		ns.CacheAvailability.AvailableSizeBytes += curUsed
	}
}

// loadVolumeSeriesData returns map of VS ID by its used cache, total used cache by all volume series related to the node and error
func (ns *NodeState) loadVolumeSeriesData(ctx context.Context) (map[string]*VolumeSeriesNodeCacheUsage, int64, error) {
	var err error
	totalUsedSizeBytes := int64(0)
	var vsRes *volume_series.VolumeSeriesListOK
	// load data for volume series cache usage on the node
	lParams := volume_series.NewVolumeSeriesListParams()
	lParams.ConfiguredNodeID = swag.String(string(ns.Node.Meta.ID))
	if vsRes, err = ns.OCrud.VolumeSeriesList(ctx, lParams); err != nil {
		return nil, totalUsedSizeBytes, err
	}
	vsM := make(map[string]*VolumeSeriesNodeCacheUsage)
	for _, vsObj := range vsRes.Payload {
		if vsCA, ok := vsObj.CacheAllocations[string(ns.Node.Meta.ID)]; ok {
			usedSizeBytes := swag.Int64Value(vsCA.AllocatedSizeBytes)
			if usedSizeBytes == 0 {
				continue
			}
			totalUsedSizeBytes += usedSizeBytes
			vsM[string(vsObj.Meta.ID)] = &VolumeSeriesNodeCacheUsage{
				UsedSizeBytes: usedSizeBytes,
			}
		}
	}
	return vsM, totalUsedSizeBytes, nil
}

func (ns *NodeState) stateChanged(nns *NodeState) bool {
	ns.Log.Debugf("original: %d/%d, new: %d/%d", ns.CacheAvailability.UsedSizeBytes, ns.CacheAvailability.AvailableSizeBytes,
		nns.CacheAvailability.UsedSizeBytes, nns.CacheAvailability.AvailableSizeBytes)
	if len(ns.VSCacheUsage) != len(nns.VSCacheUsage) ||
		ns.CacheAvailability.UsedSizeBytes != nns.CacheAvailability.UsedSizeBytes || ns.CacheAvailability.AvailableSizeBytes != nns.CacheAvailability.AvailableSizeBytes {
		return true
	}
	// compare volume series cache usage
	for vsID, nCU := range nns.VSCacheUsage {
		oCU, ok := ns.VSCacheUsage[vsID]
		if !ok {
			return true
		}
		if nCU.UsedSizeBytes != oCU.UsedSizeBytes {
			ns.Log.Debugf("Volume series cache usage changed: o=%v\nn=%v", oCU, nCU)
			return true
		}
	}
	return false
}

// UpdateNodeAvailableCache updates Node object AvailableCacheBytes to reflect current cache usage,
// no in-memory changes are made if the update fails.
// Caller holds the lock
func (ns *NodeState) UpdateNodeAvailableCache(ctx context.Context) error {
	curNodeAvailableCacheBytes := swag.Int64Value(ns.NodeStateArgs.Node.AvailableCacheBytes)
	newAvailableCacheBytes := ns.GetNodeAvailableCacheBytes(ctx)
	if curNodeAvailableCacheBytes == newAvailableCacheBytes { // already up-to-date
		return nil
	}
	ns.NodeStateArgs.Node.AvailableCacheBytes = swag.Int64(newAvailableCacheBytes)
	items := &crud.Updates{Set: []string{"availableCacheBytes"}}
	updatedNodeObj, err := ns.OCrud.NodeUpdate(ctx, ns.NodeStateArgs.Node, items)
	if err != nil {
		// undo in-memory changes to Node object
		ns.NodeStateArgs.Node.AvailableCacheBytes = swag.Int64(curNodeAvailableCacheBytes)
		return fmt.Errorf("Failure to update Node cache usage: %s", err.Error())
	}
	ns.UpdateNodeObj(ctx, updatedNodeObj)
	return nil
}

// UpdateNodeObj updates node object
func (ns *NodeState) UpdateNodeObj(ctx context.Context, node *models.Node) {
	ns.NodeStateArgs.Node = node
}

// GetNodeAvailableCacheBytes return current value for AvailableCacheBytes
func (ns *NodeState) GetNodeAvailableCacheBytes(ctx context.Context) int64 {
	return ns.CacheAvailability.AvailableSizeBytes
}

// RoundSizeBytesToBlock takes an integer and a block size and returns the closest multiple of such block size,
// the greatest value that is less or equal to given if param 'floor' is true, or
// the least integer value greater than or equal to given if 'floor' is false
func RoundSizeBytesToBlock(sizeBytes, blockSizeBytes int64, floor bool) int64 {
	if blockSizeBytes == 0 {
		return 0
	}
	mod := sizeBytes % blockSizeBytes
	if mod == 0 || floor {
		return sizeBytes / blockSizeBytes * blockSizeBytes
	}
	return (sizeBytes/blockSizeBytes + 1) * blockSizeBytes
}
