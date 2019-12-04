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


package common

// Constants relating to CRUD error messages
const (
	ErrorDbError                 = "database error"
	ErrorExists                  = "object exists"
	ErrorIDVerNotFound           = "object version not found"
	ErrorInternalError           = "internal server error"
	ErrorInvalidData             = "invalid or insufficient data"
	ErrorInvalidState            = "object state precludes the operation"
	ErrorMissing                 = "required properties are invalid or missing"
	ErrorNotFound                = "object not found"
	ErrorRequestInConflict       = "conflict with active operations or in use"
	ErrorUnauthorizedOrForbidden = "unauthorized or forbidden operation"
	ErrorUnsupportedOperation    = "unsupported operation"
	ErrorUpdateInvalidRequest    = "invalid update request or no change"
	ErrorFinalSnapNeeded         = "final snapshot needed"
)

// Constants relating to Accessibility Scope
const (
	AccScopeCspDomain = "CSPDOMAIN"
	AccScopeNode      = "NODE"
)

// Constants relating to the Audit Log
const (
	AuditClass      = "audit"
	EventClass      = "event"
	AnnotationClass = "annotation"

	// default annotation action
	NoteAction = "note"
)

// Constants relating to Authentication and Authorization
const (
	SystemAccount    = "System"
	DefTenantAccount = "Demo Tenant"
	DefSubAccount    = "Normal Account"
	AdminUser        = "admin"
	SystemAdminRole  = "System Admin"
	TenantAdminRole  = "Tenant Admin"
	AccountAdminRole = "Account Admin"
	AccountUserRole  = "Account User"

	AuthHeader = "X-Auth"

	AccountHeader = "X-Account"
	UserHeader    = "X-User"
)

// Constants relating to Node.LocalStorage.DeviceState
const (
	NodeDevStateCache      = "CACHE"
	NodeDevStateError      = "ERROR"
	NodeDevStateRestricted = "RESTRICTED"
	NodeDevStateUnused     = "UNUSED"
)

// Constants relating to ServicePlanAllocation
const (
	SPAReservationStateDisabled   = "DISABLED"
	SPAReservationStateNoCapacity = "NOCAPACITY"
	SPAReservationStateOk         = "OK"
	SPAReservationStateUnknown    = "UNKNOWN"
)

// Constants relating to Storage
const (
	StgAttachmentStateAttached        = "ATTACHED"
	StgAttachmentStateAttaching       = "ATTACHING"
	StgAttachmentStateDetached        = "DETACHED"
	StgAttachmentStateDetaching       = "DETACHING"
	StgAttachmentStateError           = "ERROR"
	StgDeviceStateClosing             = "CLOSING"
	StgDeviceStateError               = "ERROR"
	StgDeviceStateFormatting          = "FORMATTING"
	StgDeviceStateOpen                = "OPEN"
	StgDeviceStateOpening             = "OPENING"
	StgDeviceStateUnused              = "UNUSED"
	StgMediaStateFormatted            = "FORMATTED"
	StgMediaStateUnformatted          = "UNFORMATTED"
	StgProvisionedStateError          = "ERROR"
	StgProvisionedStateProvisioned    = "PROVISIONED"
	StgProvisionedStateProvisioning   = "PROVISIONING"
	StgProvisionedStateUnprovisioned  = "UNPROVISIONED"
	StgProvisionedStateUnprovisioning = "UNPROVISIONING"
)

// Constants relating to Storage Request
const (
	StgReqOpAttach              = "ATTACH"
	StgReqOpClose               = "CLOSE"
	StgReqOpDetach              = "DETACH"
	StgReqOpFormat              = "FORMAT"
	StgReqOpProvision           = "PROVISION"
	StgReqOpReattach            = "REATTACH"
	StgReqOpRelease             = "RELEASE"
	StgReqOpUse                 = "USE"
	StgReqStateAttaching        = "ATTACHING"
	StgReqStateCapacityWait     = "CAPACITY_WAIT"
	StgReqStateClosing          = "CLOSING"
	StgReqStateDetaching        = "DETACHING"
	StgReqStateFailed           = "FAILED"
	StgReqStateFormatting       = "FORMATTING"
	StgReqStateNew              = "NEW"
	StgReqStateProvisioning     = "PROVISIONING"
	StgReqStateReattaching      = "REATTACHING"
	StgReqStateReleasing        = "RELEASING"
	StgReqStateRemovingTag      = "REMOVING_TAG"
	StgReqStateSucceeded        = "SUCCEEDED"
	StgReqStateUndoAttaching    = "UNDO_ATTACHING"
	StgReqStateUndoDetaching    = "UNDO_DETACHING"
	StgReqStateUndoProvisioning = "UNDO_PROVISIONING"
	StgReqStateUsing            = "USING"
)

// Constants relating to Tasks
const (
	TaskAuditRecordsPurge = "AUDIT_RECORDS_PURGE"
	TaskClusterHeartbeat  = "CLUSTER_HEARTBEAT"
	TaskVsrPurge          = "VSR_PURGE"
)

// Constants related to ValueType
const (
	ValueTypeDuration   = "DURATION"
	ValueTypeInt        = "INT"
	ValueTypePercentage = "PERCENTAGE"
	ValueTypeString     = "STRING"
	ValueTypeSecret     = "SECRET"
)

// ValueTypeKindAbbrev specifies single character abbreviations for value type
var ValueTypeKindAbbrev = map[string]string{
	ValueTypeDuration:   "D",
	ValueTypeInt:        "I",
	ValueTypePercentage: "%",
	ValueTypeSecret:     "E",
	ValueTypeString:     "S",
}

// Constants related to VolumeSeries
const (
	VolMountHeadIdentifier  = "HEAD"
	VolMountModeRO          = "READ_ONLY"
	VolMountModeRW          = "READ_WRITE"
	VolMountStateError      = "ERROR"
	VolMountStateMounting   = "MOUNTING"
	VolMountStateMounted    = "MOUNTED"
	VolMountStateUnmounting = "UNMOUNTING"
	VolMountStateUnmounted  = "UNMOUNTED" // pseudo-state, not present in mounts array
	VolStateBound           = "BOUND"
	VolStateConfigured      = "CONFIGURED"
	VolStateDeleting        = "DELETING"
	VolStateInUse           = "IN_USE"
	VolStateProvisioned     = "PROVISIONED"
	VolStateUnbound         = "UNBOUND"
)

// Constants relating to VolumeSeriesRequest
const (
	// Operation name constants reflect the swagger requestedOperations enumeration.
	// Also update pkg/centrald/handlers, pkg/vra, and individual daemon ShouldDispatch() methods to support these operations.
	VolReqOpAllocateCapacity   = "ALLOCATE_CAPACITY"
	VolReqOpAttachFs           = "ATTACH_FS"
	VolReqOpBind               = "BIND"
	VolReqOpCGCreateSnapshot   = "CG_SNAPSHOT_CREATE"
	VolReqOpChangeCapacity     = "CHANGE_CAPACITY"
	VolReqOpConfigure          = "CONFIGURE"
	VolReqOpCreate             = "CREATE"
	VolReqOpCreateFromSnapshot = "CREATE_FROM_SNAPSHOT"
	VolReqOpDelete             = "DELETE"
	VolReqOpDeleteSPA          = "DELETE_SPA"
	VolReqOpDetachFs           = "DETACH_FS"
	VolReqOpMount              = "MOUNT"
	VolReqOpNodeDelete         = "NODE_DELETE"
	VolReqOpPublish            = "PUBLISH"
	VolReqOpRename             = "RENAME"
	VolReqOpUnbind             = "UNBIND"
	VolReqOpUnmount            = "UNMOUNT"
	VolReqOpUnpublish          = "UNPUBLISH"
	VolReqOpVolCreateSnapshot  = "VOL_SNAPSHOT_CREATE"
	VolReqOpVolDetach          = "VOL_DETACH"
	VolReqOpVolRestoreSnapshot = "VOL_SNAPSHOT_RESTORE"
	// Update pkg/vra to reflect these states
	VolReqStateAllocatingCapacity       = "ALLOCATING_CAPACITY"
	VolReqStateAttachingFs              = "ATTACHING_FS"
	VolReqStateBinding                  = "BINDING"
	VolReqStateCGSnapshotFinalize       = "CG_SNAPSHOT_FINALIZE"
	VolReqStateCGSnapshotVolumes        = "CG_SNAPSHOT_VOLUMES"
	VolReqStateCGSnapshotWait           = "CG_SNAPSHOT_WAIT"
	VolReqStateCanceled                 = "CANCELED"
	VolReqStateCancelingRequests        = "CANCELING_REQUESTS"
	VolReqStateCapacityWait             = "CAPACITY_WAIT"
	VolReqStateChangingCapacity         = "CHANGING_CAPACITY"
	VolReqStateChoosingNode             = "CHOOSING_NODE"
	VolReqStateCreatedPiT               = "CREATED_PIT"
	VolReqStateCreating                 = "CREATING"
	VolReqStateCreatingFromSnapshot     = "CREATING_FROM_SNAPSHOT"
	VolReqStateCreatingPiT              = "CREATING_PIT"
	VolReqStateDeletingNode             = "DELETING_NODE"
	VolReqStateDeletingSPA              = "DELETING_SPA"
	VolReqStateDetachingStorage         = "DETACHING_STORAGE"
	VolReqStateDetachingVolumes         = "DETACHING_VOLUMES"
	VolReqStateDrainingRequests         = "DRAINING_REQUESTS"
	VolReqStateFailed                   = "FAILED"
	VolReqStateFinalizingSnapshot       = "FINALIZING_SNAPSHOT"
	VolReqStateNew                      = "NEW"
	VolReqStatePausedIO                 = "PAUSED_IO"
	VolReqStatePausingIO                = "PAUSING_IO"
	VolReqStatePlacement                = "PLACEMENT"
	VolReqStatePlacementReattach        = "PLACEMENT_REATTACH"
	VolReqStatePublishing               = "PUBLISHING"
	VolReqStatePublishingServicePlan    = "PUBLISHING_SERVICE_PLAN"
	VolReqStateReallocatingCache        = "REALLOCATING_CACHE"
	VolReqStateRenaming                 = "RENAMING"
	VolReqStateResizingCache            = "RESIZING_CACHE"
	VolReqStateSizing                   = "SIZING"
	VolReqStateSnapshotRestore          = "SNAPSHOT_RESTORE"
	VolReqStateSnapshotRestoreDone      = "SNAPSHOT_RESTORE_DONE"
	VolReqStateSnapshotRestoreFinalize  = "SNAPSHOT_RESTORE_FINALIZE"
	VolReqStateSnapshotUploadDone       = "SNAPSHOT_UPLOAD_DONE"
	VolReqStateSnapshotUploading        = "SNAPSHOT_UPLOADING"
	VolReqStateStorageWait              = "STORAGE_WAIT"
	VolReqStateSucceeded                = "SUCCEEDED"
	VolReqStateUndoAllocatingCapacity   = "UNDO_ALLOCATING_CAPACITY"
	VolReqStateUndoAttachingFs          = "UNDO_ATTACHING_FS"
	VolReqStateUndoBinding              = "UNDO_BINDING"
	VolReqStateUndoCGSnapshotVolumes    = "UNDO_CG_SNAPSHOT_VOLUMES"
	VolReqStateUndoChangingCapacity     = "UNDO_CHANGING_CAPACITY"
	VolReqStateUndoCreatedPiT           = "UNDO_CREATED_PIT"
	VolReqStateUndoCreating             = "UNDO_CREATING"
	VolReqStateUndoCreatingFromSnapshot = "UNDO_CREATING_FROM_SNAPSHOT"
	VolReqStateUndoCreatingPiT          = "UNDO_CREATING_PIT"
	VolReqStateUndoPausedIO             = "UNDO_PAUSED_IO"
	VolReqStateUndoPausingIO            = "UNDO_PAUSING_IO"
	VolReqStateUndoPlacement            = "UNDO_PLACEMENT"
	VolReqStateUndoPublishing           = "UNDO_PUBLISHING"
	VolReqStateUndoReallocatingCache    = "UNDO_REALLOCATING_CACHE"
	VolReqStateUndoRenaming             = "UNDO_RENAMING"
	VolReqStateUndoResizingCache        = "UNDO_RESIZING_CACHE"
	VolReqStateUndoSizing               = "UNDO_SIZING"
	VolReqStateUndoSnapshotRestore      = "UNDO_SNAPSHOT_RESTORE"
	VolReqStateUndoSnapshotUploadDone   = "UNDO_SNAPSHOT_UPLOAD_DONE"
	VolReqStateUndoSnapshotUploading    = "UNDO_SNAPSHOT_UPLOADING"
	VolReqStateUndoVolumeConfig         = "UNDO_VOLUME_CONFIG"
	VolReqStateUndoVolumeExport         = "UNDO_VOLUME_EXPORT"
	VolReqStateVolumeConfig             = "VOLUME_CONFIG"
	VolReqStateVolumeDetachWait         = "VOLUME_DETACH_WAIT"
	VolReqStateVolumeDetached           = "VOLUME_DETACHED"
	VolReqStateVolumeDetaching          = "VOLUME_DETACHING"
	VolReqStateVolumeExport             = "VOLUME_EXPORT"
	VolReqStgElemIntentCache            = "CACHE"
	VolReqStgElemIntentData             = "DATA"
	VolReqStgHDDParcelKey               = "HDD"
	VolReqStgPseudoParcelKey            = "STORAGE"
	VolReqStgSSDParcelKey               = "SSD"
)

// CSP Volume tag names
const (
	VolTagSnapPrefix         = "snap-"
	VolTagSnapExistsTemplate = "snap-%s:exists"
	VolTagStorageID          = "nuvoloso-storage-id"
	VolTagPoolID             = "nuvoloso-pool-id"
	VolTagStorageRequestID   = "nuvoloso-storage-request-id"
	VolTagSystem             = "nuvoloso-system"
)

// System tag keys
const (
	SystemTagDefSubAccountCreated     = "sub-account:created"
	SystemTagDefTenantCreated         = "tenant:created"
	SystemTagForceDetachNodeID        = "sr.forceDetachNodeID"
	SystemTagVolClusterPrefix         = "volume.cluster."                        // such tags are cleared automatically from VolumeSeries objects when they get BOUND or UNBOUND
	SystemTagVolumeFsAttached         = SystemTagVolClusterPrefix + "fsAttached" // this is valid only when the state is IN_USE: export/unexport also unsets it
	SystemTagVolumeHeadStatCount      = SystemTagVolClusterPrefix + "headStatCount"
	SystemTagVolumeHeadStatSeries     = SystemTagVolClusterPrefix + "headStatSeries"
	SystemTagVolumeLastConfiguredNode = SystemTagVolClusterPrefix + "lastConfiguredNode"
	SystemTagVolumeLastHeadUnexport   = SystemTagVolClusterPrefix + "lastHeadUnexport"
	SystemTagVolumePublished          = SystemTagVolClusterPrefix + "published"
	SystemTagVolumeRestoredSnap       = "volume.restoredFromSnapshot"
	SystemTagVolumeSeriesID           = "volumeSeriesId"
	SystemTagVsrAGReset               = "vsr.agReset"
	SystemTagVsrCGReset               = "vsr.cgReset"
	SystemTagVsrCacheAllocation       = "vsr.cacheAllocation"
	SystemTagVsrCapacity              = "vsr.capacity"
	SystemTagVsrCreatedLogVol         = "vsr.createdLogVol"
	SystemTagVsrCreator               = "vsr.creator"
	SystemTagVsrDeleting              = "vsr.deleting"
	SystemTagVsrNodeDeleted           = "vsr.nodeDeleted"
	SystemTagVsrOldName               = "vsr.oldName"
	SystemTagVsrOldSizeBytes          = "vsr.oldSizeBytes"
	SystemTagVsrOldSpaAdditionalBytes = "vsr.oldSPAAdditionalBytes"
	SystemTagVsrOperator              = "vsr.op"
	SystemTagVsrPlacement             = "vsr.placement"
	SystemTagVsrReserve               = "vsr.reserve"
	SystemTagVsrRestored              = "vsr.restored"
	SystemTagVsrRestoring             = "vsr.restoring"
	SystemTagVsrSetStorageID          = "vsr.setStorageId"
	SystemTagVsrSpaAdditionalBytes    = "vsr.spaAdditionalBytes"
	SystemTagVsrSpaCapacityWait       = "vsr.spaCapacityWait"
	SystemTagVsrCreateSPA             = "vsr.createSPA"
	SystemTagVsrNewVS                 = "vsr.newVolumeSeries"
	SystemTagVsrPublishVsr            = "vsr.publishVSR"
	SystemTagVsrK8sNodePublish        = "vsr.k8sNodePublish"
	SystemTagVsrK8sVolDelete          = "vsr.k8sVolumeDelete"
	SystemTagMustDeleteOnUndoCreate   = "vsr.mustDeleteOnUndoCreate" // this tag is primarily used for AGs and CGs during CREATE vsr
)

// Constants related to NuvoService attributes
const (
	ServiceAttrCrudeStatistics      = "crudeStatistics"
	ServiceAttrMetricMoverStatus    = "metricMoverStatus"
	ServiceAttrClusterResourceState = "clusterResourceState"
	ServiceStateReady               = "READY"
	ServiceStateStopped             = "STOPPED"
	ServiceStateUnknown             = "UNKNOWN"
)

// Constants related to filesystem types
const (
	FSTypeExt4 = "ext4"
	FSTypeXfs  = "xfs"
)

// Constants related to Cluster types
const (
	AccountSecretClusterObjectName    = "nuvoloso-account"
	AccountSecretScopeCluster         = "CLUSTER"
	AccountSecretScopeCspDomain       = "CSPDOMAIN"
	AccountSecretScopeGlobal          = "GLOBAL"
	K8sSecretKey                      = "nuvoloso-secret"
	K8sClusterIdentitySecretKey       = "cluster-secret"
	ConsistencyGroupNameDefault       = "" // empty string
	ConsistencyGroupNameUnspecified   = "${cluster.id}-${k8sPod.namespace}-${k8sPod.name}"
	ApplicationGroupNameDefault       = "${k8sPod.controller.name}"
	CustomizedSecretNameDefault       = "customized-volume-secret"
	CSIDriverName                     = "csi.nuvoloso.com"
	K8sDriverTypeCSI                  = "csi"
	K8sDriverTypeFlex                 = "flex"
	K8sPvcSecretAnnotationKey         = "nuvoloso.com/provisioning-secret"
	K8sPodAnnotationCGName            = "nuvoloso.com/consistency-group-name"
	K8sPodAnnotationCGDesc            = "nuvoloso.com/consistency-group-description"
	K8sPodAnnotationCGTagsFmt         = "nuvoloso.com/consistency-group-tag-%d"
	K8sPodAnnotationVolTagsFmt        = "nuvoloso.com/volume-tag-%d"
	K8sPodAnnotationAGName            = "nuvoloso.com/application-group-name"
	K8sPodAnnotationAGDesc            = "nuvoloso.com/application-group-description"
	K8sPodAnnotationAGTagsFmt         = "nuvoloso.com/application-group-tag-%d"
	K8sVolCtxKeyServicePlanID         = "nuvoloso-service-plan-id"
	K8sVolCtxKeySizeBytes             = "nuvoloso-size-bytes"
	K8sVolCtxKeyDynamic               = "nuvoloso-dynamic"
	K8sVolCtxKeyVolumeSeriesName      = "nuvoloso-vs-name"
	VolumeDataRetentionOnDeleteDelete = "DELETE"
	VolumeDataRetentionOnDeleteRetain = "RETAIN"
	TemplateVarAccountID              = "account.id"
	TemplateVarAccountName            = "account.name"
	TemplateVarClusterID              = "cluster.id"
	TemplateVarClusterName            = "cluster.name"
	TemplateVarCspDomainID            = "cspDomain.id"
	TemplateVarCspDomainName          = "cspDomain.name"
	TemplateVarK8sPodLabelsFmt        = "k8sPod.labels['%s']"
	TemplateVarK8sPodName             = "k8sPod.name"
	TemplateVarK8sPodNamespace        = "k8sPod.namespace"
	TemplateVarVolumeSeriesID         = "volumeSeries.id"
	TemplateVarVolumeSeriesName       = "volumeSeries.name"
	TemplateVarK8sPodControllerName   = "k8sPod.controller.name"
	TemplateVarK8sPodControllerUID    = "k8sPod.controller.uid"
)

// Constants related to Cluster state
const (
	ClusterStateDeployable = "DEPLOYABLE"
	ClusterStateManaged    = "MANAGED"
	ClusterStateTimedOut   = "TIMED_OUT"
	ClusterStateResetting  = "RESETTING"
	ClusterStateTearDown   = "TEAR_DOWN"
)

// Constants related to Node state
const (
	NodeStateManaged  = "MANAGED"
	NodeStateTimedOut = "TIMED_OUT"
	NodeStateTearDown = "TEAR_DOWN"
)

// Constants related to Volume Series cluster descriptor types
const (
	K8sPvcYaml              = "k8sPvcYaml"
	K8sDynamicPvcYaml       = "k8sDynamicPvcYaml"
	ClusterDescriptorPVName = "pvName"
)

// Constants related to agentd cache state
const (
	ErrorCacheAlreadyAllocated = "cache already allocated for volume series"
	ErrorInsufficientCache     = "no sufficient cache available"
)

// Constants relating to encryption
const (
	EncryptionNone   = "NONE"
	EncryptionAES256 = "AES-256"
)

// Constants relating to protection stores
const (
	ProtectionStoreDefaultKey = "DEFAULT"
)
