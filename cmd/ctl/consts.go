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


package main

// environment
const (
	eHOME = "HOME"
)

// header constants
const (
	hAccessibilityScope               = "Scope"
	hAccessibilityScopeID             = "ScopeID"
	hAccount                          = "Account"
	hAccountID                        = "AccountID"
	hAccountRoles                     = "AccountRoles"
	hAccounts                         = "Accounts"
	hAction                           = "Action"
	hAdjustedSizeBytes                = "AdjSz"
	hApplicationGroups                = "AppGroups"
	hAttachedCluster                  = "Cluster"
	hAttachedNode                     = "Node"
	hAttachmentState                  = "AttState"
	hAuthIdentifier                   = "AuthIdentifier"
	hAuthorizedAccount                = "Authorized"
	hAuthorizedAccounts               = "Authorized"
	hAvailableCache                   = "AvailableCache"
	hAvailableCapacityBytes           = "AvailCap"
	hAvailableBytes                   = "Avail"
	hBoundCluster                     = "Cluster"
	hCapacityAllocations              = "CapAlloc"
	hCacheComponent                   = "Cache"
	hCacheAllocations                 = "CacheAllocations"
	hCSPStorageType                   = "StorageType"
	hChoices                          = "Choices"
	hClassification                   = "Class"
	hCluster                          = "Cluster"
	hClusterAttributes                = "ClusterAttributes"
	hClusterIdentifier                = "ClusterIdentifier"
	hPersistentVolumeName             = "PersistentVolumeName"
	hClusterState                     = "ClusterState"
	hClusterType                      = "ClusterType"
	hClusterVersion                   = "ClusterVersion"
	hCompleteBy                       = "CompleteBy"
	hConfiguredNode                   = "ConfiguredNode"
	hConsistencyGroup                 = "CGroup"
	hControllerState                  = "State"
	hCspCredential                    = "CspCredential"
	hCspDomain                        = "CSPDomain"
	hCspDomainAttributes              = "DomainAttributes"
	hCspDomainType                    = "DomainType"
	hCredentialAttributes             = "CredentialAttributes"
	hDeleteAfterTime                  = "DeleteAfterTime"
	hDescription                      = "Description"
	hDeviceState                      = "DevState"
	hDisabled                         = "Disabled"
	hError                            = "Err"
	hHeadNode                         = "Node"
	hID                               = "ID"
	hIOProfile                        = "IOProfile"
	hLocalStorage                     = "LocalStorage"
	hLocations                        = "Locations"
	hManagementHost                   = "ManagementHost"
	hMaxAllocationSizeBytes           = "MaxSize"
	hMediaState                       = "MediaState"
	hMessage                          = "Message"
	hMessages                         = "Messages"
	hMinAllocationSizeBytes           = "MinSize"
	hMinSizeBytes                     = "Size"
	hMounts                           = "Mounts"
	hName                             = "Name"
	hNode                             = "Node"
	hNodeAttributes                   = "NodeAttributes"
	hNodeDevice                       = "Device"
	hNodeIdentifier                   = "NodeIdentifier"
	hNodeState                        = "NodeState"
	hObjectID                         = "ObjectID"
	hObjectType                       = "Type"
	hOperation                        = "Operation"
	hParcelSizeBytes                  = "ParcelSize"
	hParentNum                        = "Ref#"
	hPitIdentifier                    = "PitIdentifier"
	hPreferredAllocationSizeBytes     = "TargetSize"
	hPreferredAllocationUnitSizeBytes = "TargetUnitSize"
	hProgress                         = " %"
	hProfile                          = "Profile"
	hProtectionDomain                 = "ProtectionDomain"
	hProvisionedState                 = "ProvState"
	hProvisioningUnit                 = "ProvisioningUnit"
	hRequestState                     = "State"
	hRequestedOperations              = "Operations"
	hReservableCapacityBytes          = "ResvCap"
	hResourceState                    = "ResourceState"
	hRootParcelUUID                   = "RootParcelUUID"
	hRootStorageID                    = "RootStorage"
	hRecordNum                        = "Seq#"
	hServiceIP                        = "ServiceIP"
	hServiceLocator                   = "ServiceLocator"
	hServicePlan                      = "ServicePlan"
	hSLO                              = "SLOs"
	hSizeBytes                        = "Size"
	hSnapIdentifier                   = "SnapIdentifier"
	hSnapTime                         = "SnapTime"
	hSnapshots                        = "Snapshots"
	hSourceServicePlan                = "SourceServicePlan"
	hSscList                          = "Capabilities"
	hState                            = "State"
	hStorageID                        = "Storage"
	hStorageComponent                 = "Storage"
	hStorageIdentifier                = "StorageIdentifier"
	hStorageFormula                   = "StorageFormula"
	hStorageLayout                    = "Layout"
	hStorageParcels                   = "StorageParcels"
	hPool                             = "Pool"
	hPoolAllocationState              = "AllocState"
	hPoolMessages                     = "Messages"
	hSnapshotManagementPolicy         = "SnapshotManagementPolicy"
	hSnapshotCatalogPolicy            = "SnapshotCatalogPolicy"
	hSystemAttributes                 = "SystemAttributes"
	hSystemTags                       = "SystemTags"
	hSystemVersion                    = "SystemVersion"
	hTags                             = "Tags"
	hTenant                           = "Tenant"
	hTimeCreated                      = "TimeCreated"
	hTimeDuration                     = "Duration"
	hTimeExpires                      = "TimeExpires"
	hTimeModified                     = "TimeModified"
	hTimestamp                        = "Timestamp"
	hToken                            = "Token"
	hTotalCache                       = "TotalCache"
	hTotalCapacityBytes               = "TotalCap"
	hTotalParcelCount                 = "Parcels"
	hUnitDimension                    = "Dimension"
	hUserID                           = "UserID"
	hUserRoles                        = "User:Roles"
	hVersion                          = "Version"
	hVolumeSeries                     = "VolumeSeries"
	hVSMinSize                        = "VolumeMinSize"
	hVSMaxSize                        = "VolumeMaxSize"
	hVsID                             = "VolumeSeriesID"
	hVSRManagementPolicy              = "VSRManagementPolicy"
)

// common descriptions
const (
	dAccessibilityScope = "The accessibility scope."
	dAccount            = "The account that owns the object."
	dAuthorizedAccounts = "The accounts that are authorized to use the object."
	dCluster            = "The name of a cluster."
	dClusterIdentifier  = "The cluster identifier. It is unique within the CSP domain."
	dController         = "The controller associated with the object."
	dConfiguredNode     = "The node on which the volume is configured"
	dControllerState    = "The state of the controller."
	dCspDomain          = "The name of the cloud service provider domain."
	dCspDomainType      = "The type of cloud service provider."
	dCspStorageType     = "The Cloud Service Provider storage type name."
	dDescription        = "The description of the object."
	dHeadNode           = "The node on which the volume HEAD is mounted"
	dID                 = "The object identifier."
	dName               = "The object name."
	dNodeIdentifier     = "The identifier of a node.  It is unique within its cluster."
	dNodeName           = "The name of a node."
	dPitIdentifier      = "The identifier of point-in-time"
	dServiceIP          = "The cluster IP of the service."
	dServiceLocator     = "The cluster specific locator for the service."
	dSnapIdentifier     = "The identifier of a snapshot."
	dTags               = "Arbitrary user specified string tags used for searching."
	dTimeCreated        = "The time the object was created."
	dTimeModified       = "The time the object was last modified."
	dVersion            = "The version of the object."
)

// request operations
const (
	opPROVISION = "PROVISION"
	opATTACH    = "ATTACH"
	opDETACH    = "DETACH"
	opRELEASE   = "RELEASE"
)

// accessibility scope constants
const (
	asCSPDOMAIN = "CSPDOMAIN"
	asNODE      = "NODE"
)

// accounts
const (
	TenantAccountType = "tenant"
)
