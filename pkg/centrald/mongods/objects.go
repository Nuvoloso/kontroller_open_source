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


package mongods

import (
	"time"
)

// This file contains additional object types to support the database model conversions

// ObjMeta contains metadata of the object. It is typically embedded in each object.
// We follow the pattern of in-lining the fields of this struct in the Mongo document, and hence
// rename each property in Mongo to use a "Meta" prefix.
// BSON ignores non-external symbols in a struct and defaults to lower-casing the symbol name so
// there will be no name clash with model symbols.
// Note: we do not want to override the default BSON naming rules for model objects to support embedded document types.
type ObjMeta struct {
	MetaObjID        string    `bson:"MetaID"`
	MetaVersion      int32     `bson:"MetaVer"`
	MetaTimeCreated  time.Time `bson:"MetaTC"`
	MetaTimeModified time.Time `bson:"MetaTM"`
}

// various common field names used in indexes and queries
const (
	accountIDKey               = "accountid"
	authorizedAccountIDKey     = "authorizedaccountid"
	applicationGroupIdsKey     = "applicationgroupids"
	authIdentifierKey          = "authidentifier"
	clusterIDKey               = "clusterid"
	clusterIdentifierKey       = "clusteridentifier"
	consistencyGroupIDKey      = "consistencygroupid"
	cspDomainIDKey             = "cspdomainid"
	nameKey                    = "name"
	nodeIDKey                  = "nodeid"
	nodeIdentifierKey          = "nodeidentifier"
	objKey                     = "MetaID"
	objTM                      = "MetaTM"
	objVer                     = "MetaVer"
	servicePlanIDKey           = "serviceplanid"
	servicePlanAllocationIDKey = "serviceplanallocationid"
	snapIdentifierKey          = "snapidentifier"
	storageIdentifierKey       = "storageidentifier"
	tenantAccountIDKey         = "tenantaccountid"
	terminatedKey              = "terminated"
	volumeSeriesIDKey          = "volumeseriesid"
)

// StringList is used for tags
type StringList []string

// ValueType is used to store an arbitrary data type as a string
type ValueType struct {
	Kind  string
	Value string
}

// StringValueMap is used as the base type in attribute maps
type StringValueMap map[string]ValueType

// StringValueMapArray is used to store collections of attribute maps
type StringValueMapArray []*StringValueMap

// RestrictedValueType is used to store an arbitrary data type as a string, with a flag to track if modifications are allowed
type RestrictedValueType struct {
	Kind      string
	Value     string
	Immutable bool
}

// RestrictedStringValueMap is used as the base type in slo lists
type RestrictedStringValueMap map[string]RestrictedValueType

// AuthRole is used to store data regarding a single authorization user role
type AuthRole struct {
	RoleID   string
	Disabled bool
}

// AuthRoleMap is used for user roles
type AuthRoleMap map[string]AuthRole

// CapabilityMap contains the capabilities for a user role
type CapabilityMap map[string]bool

// UserPasswordPolicy reflects the model type SystemMutableUserPasswordPolicy
type UserPasswordPolicy struct {
	MinLength int32
}

// Identity reflects the model type of the same name
type Identity struct {
	AccountID       string
	TenantAccountID string
	UserID          string
}

// ObjIDList is used for lists of object IDs
type ObjIDList []string

// TimestampedString reflects the model type of the same name
type TimestampedString struct {
	Message string
	Time    time.Time
}

// ServiceState reflects the model type of the same name
type ServiceState struct {
	HeartbeatPeriodSecs int64
	HeartbeatTime       time.Time
	State               string
}

// NuvoService reflects the model type of the same name
type NuvoService struct {
	ServiceState      `bson:",inline"`
	Messages          []TimestampedString
	ServiceAttributes StringValueMap
	ServiceType       string
	ServiceIP         string
	ServiceLocator    string
	ServiceVersion    string
}

// IoPattern reflects the model type of the same name
type IoPattern struct {
	Name            string
	MinSizeBytesAvg int32
	MaxSizeBytesAvg int32
}

//ReadWriteMix reflects the model type of the same name
type ReadWriteMix struct {
	Name           string
	MinReadPercent int32
	MaxReadPercent int32
}

// IoProfile reflects the model type of the same name
type IoProfile struct {
	IoPattern    IoPattern
	ReadWriteMix ReadWriteMix
}

// ProvisioningUnit reflects the model type of the same name
type ProvisioningUnit struct {
	IOPS       int64
	Throughput int64
}

// VolumeSeriesMinMaxSize reflects the model type of the same name
type VolumeSeriesMinMaxSize struct {
	MinSizeBytes int64
	MaxSizeBytes int64
}

// StorageTypeReservation reflects the model type of the same name
type StorageTypeReservation struct {
	SizeBytes  int64
	NumMirrors int32
}

// StorageTypeReservationMap reflects the model type CapacityReservationPlanStorageTypeReservations (!!!)
// and ServicePlanAllocationCreateMutableStorageReservations
type StorageTypeReservationMap map[string]StorageTypeReservation

// CapacityReservationPlan reflects the model type of the same name
type CapacityReservationPlan struct {
	StorageTypeReservations StorageTypeReservationMap
}

// PoolReservation reflects the model type of the same name
type PoolReservation struct {
	SizeBytes  int64
	NumMirrors int32
}

// PoolReservationMap reflects the model type CapacityReservationResultPoolReservations (!!!)
type PoolReservationMap map[string]PoolReservation

// CapacityReservationResult reflects the model type of the same name
type CapacityReservationResult struct {
	CurrentReservations PoolReservationMap
	DesiredReservations PoolReservationMap
}

// NodeStorageDevice reflects the model type of the same name
type NodeStorageDevice struct {
	DeviceName      string
	DeviceState     string
	DeviceType      string
	SizeBytes       int64
	UsableSizeBytes int64
}

// NodeStorageDeviceMap reflects the model type NodeMutableLocalStorage
type NodeStorageDeviceMap map[string]NodeStorageDevice

// StorageAccessibility reflects the model type of the same name
type StorageAccessibility struct {
	AccessibilityScope      string
	AccessibilityScopeObjID string
}

// StorageParcelElement tracks actual or planned Storage useage
type StorageParcelElement struct {
	SizeBytes              int64
	ShareableStorage       bool
	ProvMinSizeBytes       int64
	ProvParcelSizeBytes    int64
	ProvRemainingSizeBytes int64
	ProvNodeID             string
	ProvStorageRequestID   string
}

// StorageParcelMap is a map of a string (Storage ID or pseudo-key) to a StorageParcelElement
type StorageParcelMap map[string]StorageParcelElement

// StorageElement reflects the model type StoragePlanStorageElement
type StorageElement struct {
	Intent         string
	SizeBytes      int64
	StorageParcels StorageParcelMap
	PoolID         string
}

// StoragePlan reflects the model type of the same name
type StoragePlan struct {
	StorageLayout   string
	LayoutAlgorithm string
	PlacementHints  StringValueMap
	StorageElements []StorageElement
}

// PoolState reflects the model type of the same name
type PoolState struct {
	Messages        []TimestampedString
	AllocationState string
}

// StorageState reflects the model type of the same name
type StorageState struct {
	AttachedNodeDevice string
	AttachedNodeID     string
	AttachmentState    string
	DeviceState        string
	MediaState         string
	Messages           []TimestampedString
	ProvisionedState   string
}

// CacheAllocation reflects the model type of the same name
type CacheAllocation struct {
	AllocatedSizeBytes int64
	RequestedSizeBytes int64
}

// CacheAllocationMap is a map of nodeId to cache allocations
type CacheAllocationMap map[string]CacheAllocation

// CapacityAllocation reflects the model type of the same name
type CapacityAllocation struct {
	ReservedBytes int64
	ConsumedBytes int64
}

// CapacityAllocationMap is a map of poolId to capacity allocations
type CapacityAllocationMap map[string]CapacityAllocation

// ParcelAllocation reflects the model type of the same name
type ParcelAllocation struct {
	SizeBytes int64
}

// ParcelAllocationMap is a map of storageId to parcel allocations
type ParcelAllocationMap map[string]ParcelAllocation

// Mount reflects the model type of the same name
type Mount struct {
	SnapIdentifier    string
	PitIdentifier     string
	MountedNodeDevice string
	MountedNodeID     string
	MountMode         string
	MountState        string
	MountTime         time.Time
}

// Progress reflects the model type of the same name
type Progress struct {
	OffsetBytes      int64
	PercentComplete  int32
	Timestamp        time.Time
	TotalBytes       int64
	TransferredBytes int64
}

// SnapshotLocationMap is a map of CspDomainID to SnapshotLocation
type SnapshotLocationMap map[string]SnapshotLocation

// SnapshotLocation reflects the model type of the same name
type SnapshotLocation struct {
	CreationTime time.Time
	CspDomainID  string
}

// SnapshotData reflects the model type of the same name
type SnapshotData struct {
	ConsistencyGroupID string
	DeleteAfterTime    time.Time
	Locations          []SnapshotLocation
	PitIdentifier      string
	ProtectionDomainID string
	SizeBytes          int64
	SnapIdentifier     string
	SnapTime           time.Time
	VolumeSeriesID     string
}

// SnapshotMap is a map of snapNumber to a SnapshotData and represents model VolumeSeriesMutableAllOf0Snapshots
type SnapshotMap map[string]SnapshotData

// SnapshotCatalogPolicy reflects the model type of the same name
type SnapshotCatalogPolicy struct {
	CspDomainID        string
	ProtectionDomainID string
	Inherited          bool
}

// SnapshotManagementPolicy reflects the model type of the same name
type SnapshotManagementPolicy struct {
	DeleteLast                  bool
	DeleteVolumeWithLast        bool
	DisableSnapshotCreation     bool
	NoDelete                    bool
	RetentionDurationSeconds    int32
	VolumeDataRetentionOnDelete string
	Inherited                   bool
}

// LifecycleManagementData reflects the model type of the same name
type LifecycleManagementData struct {
	EstimatedSizeBytes        int64
	FinalSnapshotNeeded       bool
	GenUUID                   string
	WriteIOCount              int64
	LastSnapTime              time.Time
	LastUploadTime            time.Time
	LastUploadSizeBytes       int64
	LastUploadTransferRateBPS int32
	NextSnapshotTime          time.Time
	SizeEstimateRatio         float64
	LayoutAlgorithm           string
}

// SyncPeer reflects the model type of the same name
type SyncPeer struct {
	ID         string
	State      string
	GenCounter int32
	Annotation string
}

// SyncPeerMap reflects the model type of the same name
type SyncPeerMap map[string]SyncPeer

// VsrClaimElement reflects the model type of the same name
type VsrClaimElement struct {
	SizeBytes  int64
	Annotation string
}

// VsrClaimMap is a map of volumeSeriesRequestId to its claim
type VsrClaimMap map[string]VsrClaimElement

// VsrClaim reflects the model type of the same name
type VsrClaim struct {
	Claims         VsrClaimMap
	RemainingBytes int64
}

// VsrManagementPolicy reflects the model type of the same name
type VsrManagementPolicy struct {
	NoDelete                 bool
	RetentionDurationSeconds int32
	Inherited                bool
}

// ClusterUsagePolicy reflects the model type of the same name
type ClusterUsagePolicy struct {
	AccountSecretScope          string
	ConsistencyGroupName        string
	VolumeDataRetentionOnDelete string
	Inherited                   bool
}

// StorageCost reflects the model type of the same name
type StorageCost struct {
	CostPerGiB float64
}

// StorageCostMap is a map of storage type to storage cost and reflects the CSPDomainMutableStorageCost model
type StorageCostMap map[string]StorageCost
