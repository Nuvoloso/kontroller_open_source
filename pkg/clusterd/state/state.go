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
	"sort"
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/layout"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/go-openapi/swag"
	logging "github.com/op/go-logging"
)

// NodeData contains information on a cluster node (except for liveness data)
type NodeData struct {
	// Node object
	Node *models.Node
	// Attached Storage object IDs
	AttachedStorage []string
}

// VSRClaim expresses how much capacity a VolumeSeriesRequest has claimed in a Storage object.
type VSRClaim struct {
	// The VSR object ID
	RequestID string
	// The amount claimed. Use 0 to indicate the entire device.
	SizeBytes int64
	// Arbitrary claim description
	Annotation string
}

// ClaimData contains data on a future Storage claim
type ClaimData struct {
	// The associated StorageRequest object that is going to or has just provisioned
	// the Storage object.  This will not be tracked once all claims are satisfied.
	StorageRequest *models.StorageRequest
	// ID of the Pool used
	PoolID string
	// ID of the node to which the Storage object is to be be attached.
	NodeID string
	// The remaining (allocation) capacity in bytes in a Storage object.
	// A VSR that wants to grab the entire Storage object can set this to zero.
	// Note that in general the actual available capacity is not really known until
	// formatting completes.
	RemainingBytes int64
	// Array of active VSR claims on this Storage object in order of making the claim.
	VSRClaims []VSRClaim
	// internal
	cs *ClusterState
}

// RemoveClaim removes a VolumeSeriesRequest claim
func (cd *ClaimData) RemoveClaim(requestID string) {
	for i, v := range cd.VSRClaims {
		if v.RequestID == requestID {
			cd.VSRClaims[i] = cd.VSRClaims[len(cd.VSRClaims)-1] // replace with the last
			cd.VSRClaims = cd.VSRClaims[:len(cd.VSRClaims)-1]   // chop off the last
			if cd.cs != nil {
				cd.cs.LoadCountOnInternalChange = cd.cs.NumLoads
				cd.cs.Log.Debugf("RemoveClaim SR[%s] VSR[%s] %s %d", cd.StorageRequest.Meta.ID, v.RequestID, v.Annotation, v.SizeBytes)
			}
			break
		}
	}
}

// ClaimList is a list of claims
type ClaimList []*ClaimData

// FindVSRClaimByAnnotation searches the claim list looking for an annotated claim scoped to a VSR
func (cl ClaimList) FindVSRClaimByAnnotation(rID, ann string) (*VSRClaim, *ClaimData) {
	for _, cd := range cl {
		for _, vsr := range cd.VSRClaims {
			if vsr.Annotation == ann && vsr.RequestID == rID {
				return &vsr, cd
			}
		}
	}
	return nil, nil
}

// ClusterStateArgs contains data required to create the ClusterState object
type ClusterStateArgs struct {
	// Database update interface
	OCrud crud.Ops
	// A logger
	Log *logging.Logger
	// The cluster object concerned
	Cluster *models.Cluster
	// The cloud service provider interface
	CSP csp.CloudServiceProvider
	// The layout algorithms to use
	LayoutAlgorithms []*layout.Algorithm
}

// Verify the arguments
func (csa ClusterStateArgs) Verify() bool {
	if csa.OCrud == nil || csa.Cluster == nil || csa.CSP == nil || csa.Log == nil || len(csa.LayoutAlgorithms) == 0 {
		return false
	}
	return true
}

// ClusterState contains data on the state of the cluster at some point in time.
// Serialization is the responsibility of the consumer.
type ClusterState struct {
	ClusterStateArgs
	// Map of node data keyed on object ID. Not up-to-date on state.
	Nodes map[string]*NodeData
	// Map of attached cluster storage objects keyed on Storage object ID.
	Storage map[string]*models.Storage
	// Map of detached cluster storage objects keyed on Storage object ID.
	// Note that this storage may be referenced by VolumeSeries objects.
	DetachedStorage map[string]*models.Storage
	// Map of "to-be" or "just" provisioned Storage object claims keyed on StorageRequest object ID.
	// Entries are dropped when all active VSR claims are removed.
	StorageRequests map[string]*ClaimData

	// Misc
	NumLoads                  int // count of the number of loads
	LoadCountOnInternalChange int // count when an internal change last made

	// Map of Pool ID to CSPStorageType
	pCache map[string]*models.CSPStorageType

	// indicates that the SR cache has been primed
	srCacheIsPrimed bool

	// mutex protecting health data
	hMux sync.Mutex
	// map of node health keyed on object ID and updated with node heartbeat info.
	nodeHealth map[string]*NodeHealth
	// count of additions to the nodeHealth map
	nodeHealthInsertions int
	// Map of Claimed Storage objects
	claimedStorage map[string]struct{}
}

// SelectStorageArgs contains the arguments to the SelectStorage method
type SelectStorageArgs struct {
	// The VolumeSeriesRequest object with the initial storagePlan
	VSR *models.VolumeSeriesRequest
	// The VolumeSeries object
	VS *models.VolumeSeries
	// Optional placement algorithm to use.
	LA *layout.Algorithm
}

// StorageItem describes one Storage choice.
type StorageItem struct {
	// Number of bytes of storage capacity to use from this object.
	SizeBytes int64
	// Number of parcels used from this object
	NumParcels int64
	// Pointer to an existing Storage object.
	// If nil then provision either from the StorageRequest object specified, or else the Pool.
	Storage *models.Storage
	// ID of the Pool from which to obtain the Storage
	PoolID string
	// The number of bytes to specify in the StorageRequest
	MinSizeBytes int64
	// The remaining size to specify in the StorageRequest claim
	RemainingSizeBytes int64
	// If true the storage item is shareable. This value should be set in the Storage object if necessary.
	ShareableStorage bool
	// The ID of the node to which the new Storage is to be attached.
	NodeID string
	// ID of an existing storage request from which to get capacity.
	StorageRequestID string
}

// ElementStorage returns the selections for a given plan element.
type ElementStorage struct {
	// The number of storage items for this element, as determined by capacity constraints.
	NumItems int
	// The parcel size for the element storage
	ParcelSizeBytes int64
	// A list of storage items for this element
	Items []StorageItem
}

// SelectStorageResponse returns a tuple of existing or to-be-provisioned Storage choices,
// one for each element in the storage plan.
type SelectStorageResponse struct {
	Elements []ElementStorage
	// The placement algorithm used
	LA *layout.Algorithm
}

// ClusterStateOperators are the exported methods on ClusterState
type ClusterStateOperators interface {
	// the following require external serialization
	AddClaim(ctx context.Context, sr *models.StorageRequest, claim *VSRClaim) (*ClaimData, error)
	CS() *ClusterState
	DumpState() *bytes.Buffer
	FindAllStorage(selector func(*models.Storage) bool) []*models.Storage
	FindClaimByStorageRequest(srID string) *ClaimData
	FindClaims(selector func(*ClaimData) bool) ClaimList
	FindClaimsByRequest(requestID string) ClaimList
	FindNodes(selector func(*NodeData) bool) []*models.Node
	FindStorage(selector func(*models.Storage) bool) []*models.Storage
	GetHealthyNode(preferredNodeID string) *models.Node
	LookupStorage(id string) (*models.Storage, bool)
	ReleaseUnusedStorage(ctx context.Context, timeout time.Duration)
	Reload(ctx context.Context) (bool, error)
	RemoveClaims(requestID string)
	SelectStorage(ctx context.Context, args *SelectStorageArgs) (*SelectStorageResponse, error)
	TrackStorageRequest(sr *models.StorageRequest) *ClaimData
	// the following require no external serialization
	FindLayoutAlgorithm(layout models.StorageLayout) *layout.Algorithm
	NodeDeleted(nid string)
	NodeGetServiceState(nid string) []*NodeHealth
	NodePurgeMissing(epoch time.Time) (int, int)
	NodeServiceState(nid string, ss *models.ServiceState)
	NodeUpdateState(node *models.Node, epoch time.Time)
}

var _ = ClusterStateOperators(&ClusterState{})

// NewClusterState creates and initializes the ClusterState object
func NewClusterState(args *ClusterStateArgs) *ClusterState {
	cs := &ClusterState{}
	cs.ClusterStateArgs = *args
	cs.Nodes = make(map[string]*NodeData)
	cs.Storage = make(map[string]*models.Storage)
	cs.StorageRequests = make(map[string]*ClaimData)
	cs.pCache = make(map[string]*models.CSPStorageType)
	cs.nodeHealth = make(map[string]*NodeHealth)
	return cs
}

// New returns ClusterStateOperators
func New(args *ClusterStateArgs) ClusterStateOperators {
	if !args.Verify() {
		return nil
	}
	return NewClusterState(args)
}

// CS is an identity method
func (cs *ClusterState) CS() *ClusterState {
	return cs
}

// TrackStorageRequest adds a StorageRequest to the set being tracked.
// It returns the internal representation of the claim data from the storage request or
// from the cache if the storage request is already being tracked - i.e. SR claim data are
// ignored except when adding to the cache.
// Note that it is expected that a new StorageRequest will contain the claim of the VSR
// that triggered its creation.
func (cs *ClusterState) TrackStorageRequest(sr *models.StorageRequest) *ClaimData {
	srID := string(sr.Meta.ID)
	if cd, exists := cs.StorageRequests[srID]; exists {
		return cd
	}
	srC := sr.VolumeSeriesRequestClaims
	if srC == nil {
		srC = &models.VsrClaim{}
		srC.Claims = make(map[string]models.VsrClaimElement)
		sr.VolumeSeriesRequestClaims = srC
	}
	cd := &ClaimData{
		cs:             cs,
		StorageRequest: sr,
		PoolID:         string(sr.PoolID),
		NodeID:         string(sr.NodeID),
		RemainingBytes: swag.Int64Value(srC.RemainingBytes),
	}
	cs.Log.Debugf("Tracking StorageRequest[%s] Node[%s] SP[%s] #Claims:%d RemB:%d", srID, cd.NodeID, cd.PoolID, len(srC.Claims), cd.RemainingBytes)
	cd.VSRClaims = make([]VSRClaim, 0, len(srC.Claims))
	for vsrID, cle := range srC.Claims {
		cl := VSRClaim{
			RequestID:  vsrID,
			SizeBytes:  swag.Int64Value(cle.SizeBytes),
			Annotation: cle.Annotation,
		}
		cd.VSRClaims = append(cd.VSRClaims, cl)
		cs.Log.Debugf("         StorageRequest[%s] claim: VSR[%s] %s %d", srID, vsrID, cl.Annotation, cl.SizeBytes)
	}
	cs.StorageRequests[srID] = cd
	cs.LoadCountOnInternalChange = cs.NumLoads
	return cd
}

// AddClaim adds a claim to the capacity of a Storage Object being provisioned by a StorageRequest.
// The StorageRequest must be active or have completed successfully.
// It will increase an existing claim by a VSR if such a claim already exists.
// The StorageRequest is updated to persist the claim - no in-memory changes are made if the update fails.
func (cs *ClusterState) AddClaim(ctx context.Context, sr *models.StorageRequest, claim *VSRClaim) (*ClaimData, error) {
	srID := string(sr.Meta.ID)
	if sr.StorageRequestState == com.StgReqStateFailed {
		return nil, fmt.Errorf("StorageRequest [%s] has failed", srID)
	}
	cd, exists := cs.StorageRequests[srID]
	if !exists {
		return nil, fmt.Errorf("StorageRequest not tracked")
	}
	if claim.SizeBytes <= 0 {
		return nil, fmt.Errorf("Invalid claim size")
	}
	if claim.SizeBytes > cd.RemainingBytes {
		return nil, fmt.Errorf("Insufficient capacity")
	}
	// find the vsr claim if it exists
	vsrClaimIdx := -1
	prevAnnotation := ""
	prevSize := int64(0)
	newSize := claim.SizeBytes
	for i, cl := range cd.VSRClaims {
		if cl.RequestID == claim.RequestID {
			vsrClaimIdx = i
			prevAnnotation = cl.Annotation
			prevSize = cl.SizeBytes
			newSize += cl.SizeBytes
			break
		}
	}
	newRemBytes := cd.RemainingBytes - claim.SizeBytes
	// update/insert the claim in the SR with retry on version mismatch
	changedSR := false
	modFn := func(o *models.StorageRequest) (*models.StorageRequest, error) {
		if o == nil {
			o = cd.StorageRequest
		}
		o.VolumeSeriesRequestClaims.RemainingBytes = swag.Int64(newRemBytes)
		o.VolumeSeriesRequestClaims.Claims[claim.RequestID] = models.VsrClaimElement{
			SizeBytes:  swag.Int64(newSize),
			Annotation: claim.Annotation, // always use the latest
		}
		changedSR = true
		return o, nil
	}
	items := &crud.Updates{}
	items.Set = []string{"volumeSeriesRequestClaims"}
	srObj, err := cs.OCrud.StorageRequestUpdater(ctx, srID, modFn, items)
	if err != nil {
		cs.Log.Errorf("VolumeSeriesRequest[%s] error updating claims in StorageRequest[%s]: %s", claim.RequestID, srID, err.Error())
		// undo in-memory changes to SR
		if changedSR {
			if vsrClaimIdx >= 0 {
				cd.StorageRequest.VolumeSeriesRequestClaims.Claims[claim.RequestID] = models.VsrClaimElement{
					SizeBytes:  swag.Int64(prevSize),
					Annotation: prevAnnotation,
				}
			} else {
				delete(cd.StorageRequest.VolumeSeriesRequestClaims.Claims, claim.RequestID)
			}
		}
		cd.StorageRequest.VolumeSeriesRequestClaims.RemainingBytes = swag.Int64(cd.RemainingBytes)
		return nil, err
	}
	// now update the in-memory data
	if vsrClaimIdx >= 0 {
		cd.VSRClaims[vsrClaimIdx].SizeBytes = newSize
		cd.VSRClaims[vsrClaimIdx].Annotation = claim.Annotation // always use latest
	} else {
		cd.VSRClaims = append(cd.VSRClaims, *claim)
	}
	cd.RemainingBytes = newRemBytes
	cd.StorageRequest = srObj
	cs.LoadCountOnInternalChange = cs.NumLoads
	cs.Log.Debugf("AddClaim StorageRequest[%s] VSR[%s] %s %d", srID, claim.RequestID, claim.Annotation, claim.SizeBytes)
	return cd, nil
}

// RemoveClaims removes all claims against a VolumeSeriesRequest
func (cs *ClusterState) RemoveClaims(requestID string) {
	for _, cd := range cs.StorageRequests {
		cd.RemoveClaim(requestID)
	}
}

// FindClaimByStorageRequest finds the claim data on the specified StorageRequest, whether or not the associated StorageRequest has failed.
func (cs *ClusterState) FindClaimByStorageRequest(srID string) *ClaimData {
	cd, has := cs.StorageRequests[srID]
	if !has {
		return nil
	}
	return cd
}

// FindClaims traverses the Claims list and returns selected claims.
func (cs *ClusterState) FindClaims(selector func(*ClaimData) bool) ClaimList {
	rc := make([]*ClaimData, 0, len(cs.StorageRequests))
	for _, cd := range cs.StorageRequests {
		if selector(cd) {
			rc = append(rc, cd)
		}
	}
	return rc
}

// FindClaimsByRequest returns all claims from a request, whether or not the associated StorageRequest has failed.
func (cs *ClusterState) FindClaimsByRequest(requestID string) ClaimList {
	sf := func(cd *ClaimData) bool {
		for _, cl := range cd.VSRClaims {
			if cl.RequestID == requestID {
				return true
			}
		}
		return false
	}
	return cs.FindClaims(sf)
}

// FindClaimsByPool returns claims that match the provided search criteria, on or not-on a specific node.
// Claims on failed StorageRequest objects are skipped.
func (cs *ClusterState) FindClaimsByPool(provID string, availBytes int64, nodeID string, nodeIsRemote bool) ClaimList {
	sf := func(cd *ClaimData) bool {
		if cd.PoolID != provID {
			return false
		}
		if cd.RemainingBytes < availBytes {
			return false
		}
		if nodeIsRemote && cd.NodeID == nodeID {
			return false
		}
		if !nodeIsRemote && cd.NodeID != nodeID {
			return false
		}
		if cd.StorageRequest.StorageRequestState == com.StgReqStateFailed {
			return false
		}
		return true
	}
	return cs.FindClaims(sf)
}

// LookupStorage returns a Storage object given its ID.
// The Storage may be attached or detached, as indicated by the second return value.
func (cs *ClusterState) LookupStorage(id string) (*models.Storage, bool) {
	if sObj, hasAttached := cs.Storage[id]; hasAttached {
		return sObj, true
	}
	return cs.DetachedStorage[id], false
}

// FindAllStorage traverses the Storage and DetachedStorage maps and returns selected objects.
// It does not filter out claimed storage.
func (cs *ClusterState) FindAllStorage(selector func(*models.Storage) bool) []*models.Storage {
	rc := make([]*models.Storage, 0)
	for _, sObj := range cs.Storage {
		if selector(sObj) {
			rc = append(rc, sObj)
		}
	}
	for _, sObj := range cs.DetachedStorage {
		if selector(sObj) {
			rc = append(rc, sObj)
		}
	}
	return rc
}

// FindStorage traverses the (attached) Storage map and returns selected objects.
// It skips Storage with claims against it.
func (cs *ClusterState) FindStorage(selector func(*models.Storage) bool) []*models.Storage {
	rc := make([]*models.Storage, 0)
	for _, sObj := range cs.Storage {
		if !cs.storageClaimed(string(sObj.Meta.ID)) && selector(sObj) {
			rc = append(rc, sObj)
		}
	}
	return rc
}

// FindShareableStorageByPool returns Storage that match the specified pool and size, on or not-on a specific node.
// The returned Storage is suitable for shared use though the shareableStorage flag may not reflect this.
// Will not return Storage that has claims against it.
func (cs *ClusterState) FindShareableStorageByPool(provID string, availBytes int64, nodeID string, nodeIsRemote bool) []*models.Storage {
	sf := func(sObj *models.Storage) bool {
		if !cs.StorageCanBeShared(sObj) {
			return false
		}
		if string(sObj.PoolID) != provID || swag.Int64Value(sObj.AvailableBytes) < availBytes {
			return false
		}
		if nodeIsRemote && string(sObj.StorageState.AttachedNodeID) == nodeID {
			return false
		}
		if !nodeIsRemote && string(sObj.StorageState.AttachedNodeID) != nodeID {
			return false
		}
		return true
	}
	return cs.FindStorage(sf)
}

// FindUnusedStorageByPool returns Storage of full capacity that match the specified pool and size, on or not-on a specific node.
// The returned Storage is suitable for unshared use though the shareableStorage flag may not reflect this.
// Will not return Storage that has claims against it.
func (cs *ClusterState) FindUnusedStorageByPool(provID string, availBytes int64, nodeID string, nodeIsRemote bool) []*models.Storage {
	sf := func(sObj *models.Storage) bool {
		if !cs.StorageCapacityNotAllocated(sObj) {
			return false
		}
		if string(sObj.PoolID) != provID || swag.Int64Value(sObj.AvailableBytes) != availBytes { // exact match
			return false
		}
		if nodeIsRemote && string(sObj.StorageState.AttachedNodeID) == nodeID {
			return false
		}
		if !nodeIsRemote && string(sObj.StorageState.AttachedNodeID) != nodeID {
			return false
		}
		return true
	}
	return cs.FindStorage(sf)
}

// FindNodes traverses the Nodes map and returns selected objects.
// It also updates the state based on tracked node health.
func (cs *ClusterState) FindNodes(selector func(*NodeData) bool) []*models.Node {
	rc := make([]*models.Node, 0)
	now := time.Now()
	for _, nd := range cs.Nodes {
		if selector(nd) {
			cs.NodeUpdateState(nd.Node, now)
			rc = append(rc, nd.Node)
		}
	}
	return rc
}

// GetHealthyNode returns a Node object whose animator is responsive, if any.
// A preferred node may be provided and will be returned if healthy.
// Otherwise, a healthy node with the least amount of attached storage is returned.
func (cs *ClusterState) GetHealthyNode(preferredNodeID string) *models.Node {
	now := time.Now()
	if preferredNodeID != "" {
		nd, found := cs.Nodes[preferredNodeID]
		if found {
			cs.NodeUpdateState(nd.Node, now)
			if nd.Node.Service.ServiceState.State == com.ServiceStateReady {
				return nd.Node // found and healthy
			}
		}
	}
	// search for first active node with least load
	oNd := map[int][]*NodeData{}
	for _, nd := range cs.Nodes {
		numAttachedStorage := len(nd.AttachedStorage)
		l, found := oNd[numAttachedStorage]
		if !found {
			l = []*NodeData{}
		}
		l = append(l, nd)
		oNd[numAttachedStorage] = l
	}
	keys := []int{}
	for k := range oNd {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	for _, k := range keys {
		for _, nd := range oNd[k] {
			cs.NodeUpdateState(nd.Node, now)
			if nd.Node.Service.ServiceState.State == com.ServiceStateReady {
				return nd.Node
			}
		}
	}
	return nil
}

// FindLayoutAlgorithm finds the first algorithm for a given layout
func (cs *ClusterState) FindLayoutAlgorithm(layout models.StorageLayout) *layout.Algorithm {
	for _, d := range cs.LayoutAlgorithms {
		if d.StorageLayout == layout {
			return d
		}
	}
	return nil
}

// StorageCapacityNotAllocated is a predicate
func (cs *ClusterState) StorageCapacityNotAllocated(o *models.Storage) bool {
	rc := swag.Int64Value(o.AvailableBytes) == (swag.Int64Value(o.TotalParcelCount) * swag.Int64Value(o.ParcelSizeBytes))
	return rc
}

// StorageCapacityIsNotAllocated is a predicate
func (cs *ClusterState) StorageCapacityIsNotAllocated(o *models.Storage) bool {
	rc := swag.Int64Value(o.AvailableBytes) >= (swag.Int64Value(o.TotalParcelCount) * swag.Int64Value(o.ParcelSizeBytes))
	return rc && o.StorageState.ProvisionedState == com.StgProvisionedStateProvisioned
}

// StorageCanBeShared is a predicate
func (cs *ClusterState) StorageCanBeShared(o *models.Storage) bool {
	return o.ShareableStorage || cs.StorageCapacityNotAllocated(o)
}

// storageClaimed returns true if Storage represented by id is claimed or has pending operations
func (cs *ClusterState) storageClaimed(id string) bool {
	_, claimed := cs.claimedStorage[id]
	return claimed
}
