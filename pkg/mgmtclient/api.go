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


package mgmtclient

import (
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/application_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/audit_log"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/cluster"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/consistency_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_credential"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_storage_type"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/metrics"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/node"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/pool"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/protection_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/role"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_debug"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan_allocation"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/slo"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/snapshot"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_formula"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/system"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/task"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/user"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/watchers"
	"github.com/go-openapi/runtime"
)

// API is an interface over the underlying auto-generated client code plus authentication APIs.
// This definition must be updated to match the swagger specification, which is easily
// done with minor editing of the Nuvoloso object type.
// But... each function must return an interface to the object operations, which is a royal pain!
type API interface {
	Account() AccountClient

	ApplicationGroup() ApplicationGroupClient

	AuditLog() AuditLogClient

	Authentication() AuthenticationAPI

	Cluster() ClusterClient

	ConsistencyGroup() ConsistencyGroupClient

	CspCredential() CSPCredentialClient

	CspDomain() CSPDomainClient

	CspStorageType() CSPStorageTypeClient

	Metrics() MetricsClient

	Node() NodeClient

	Pool() PoolClient

	ProtectionDomain() ProtectionDomainClient

	Role() RoleClient

	ServiceDebug() DebugClient

	ServicePlanAllocation() ServicePlanAllocationClient

	ServicePlan() ServicePlanClient

	Slo() SLOClient

	Snapshot() SnapshotClient

	Storage() StorageClient

	StorageFormula() StorageFormulaClient

	StorageRequest() StorageRequestClient

	System() SystemClient

	Task() TaskClient

	User() UserClient

	VolumeSeries() VolumeSeriesClient

	VolumeSeriesRequest() VolumeSeriesRequestClient

	Watchers() WatchersClient

	Transport() runtime.ClientTransport // @todo
}

// AccountClient returns the operational interface
type AccountClient interface {
	AccountCreate(*account.AccountCreateParams) (*account.AccountCreateCreated, error)
	AccountDelete(*account.AccountDeleteParams) (*account.AccountDeleteNoContent, error)
	AccountFetch(*account.AccountFetchParams) (*account.AccountFetchOK, error)
	AccountList(*account.AccountListParams) (*account.AccountListOK, error)
	AccountProtectionDomainClear(params *account.AccountProtectionDomainClearParams) (*account.AccountProtectionDomainClearOK, error)
	AccountProtectionDomainSet(params *account.AccountProtectionDomainSetParams) (*account.AccountProtectionDomainSetOK, error)
	AccountSecretReset(params *account.AccountSecretResetParams) (*account.AccountSecretResetNoContent, error)
	AccountSecretRetrieve(params *account.AccountSecretRetrieveParams) (*account.AccountSecretRetrieveOK, error)
	AccountUpdate(*account.AccountUpdateParams) (*account.AccountUpdateOK, error)
}

// ApplicationGroupClient returns the operational interface
type ApplicationGroupClient interface {
	ApplicationGroupCreate(*application_group.ApplicationGroupCreateParams) (*application_group.ApplicationGroupCreateCreated, error)
	ApplicationGroupDelete(*application_group.ApplicationGroupDeleteParams) (*application_group.ApplicationGroupDeleteNoContent, error)
	ApplicationGroupFetch(*application_group.ApplicationGroupFetchParams) (*application_group.ApplicationGroupFetchOK, error)
	ApplicationGroupList(*application_group.ApplicationGroupListParams) (*application_group.ApplicationGroupListOK, error)
	ApplicationGroupUpdate(*application_group.ApplicationGroupUpdateParams) (*application_group.ApplicationGroupUpdateOK, error)
}

// AuditLogClient is the interface for audit log operations
type AuditLogClient interface {
	AuditLogCreate(*audit_log.AuditLogCreateParams) (*audit_log.AuditLogCreateCreated, error)
	AuditLogList(*audit_log.AuditLogListParams) (*audit_log.AuditLogListOK, error)
}

// AuthenticationAPI is the interface for authentication-related operations
type AuthenticationAPI interface {
	// Authenticate obtains an auth token for the authIdentifier and password
	Authenticate(authIdentifier, password string) (*AuthResp, error)
	// GetAuthToken gets the current auth token
	GetAuthToken() string
	// SetAuthToken sets a new auth token, but does not validate it
	SetAuthToken(newToken string)
	// SetContextAccount sets the account and user for subsequent API calls. The values are validated
	SetContextAccount(authIdentifier, accountName string) (string, error)
	// Validate checks if the current auth token is still valid
	Validate(updateExpiry bool) (*AuthResp, error)
}

// ClusterClient returns the operational interface
type ClusterClient interface {
	ClusterAccountSecretFetch(params *cluster.ClusterAccountSecretFetchParams) (*cluster.ClusterAccountSecretFetchOK, error)
	ClusterCreate(*cluster.ClusterCreateParams) (*cluster.ClusterCreateCreated, error)
	ClusterDelete(*cluster.ClusterDeleteParams) (*cluster.ClusterDeleteNoContent, error)
	ClusterFetch(*cluster.ClusterFetchParams) (*cluster.ClusterFetchOK, error)
	ClusterList(*cluster.ClusterListParams) (*cluster.ClusterListOK, error)
	ClusterUpdate(*cluster.ClusterUpdateParams) (*cluster.ClusterUpdateOK, error)
	ClusterOrchestratorGetDeployment(*cluster.ClusterOrchestratorGetDeploymentParams) (*cluster.ClusterOrchestratorGetDeploymentOK, error)
}

// ConsistencyGroupClient returns the operational interface
type ConsistencyGroupClient interface {
	ConsistencyGroupCreate(*consistency_group.ConsistencyGroupCreateParams) (*consistency_group.ConsistencyGroupCreateCreated, error)
	ConsistencyGroupDelete(*consistency_group.ConsistencyGroupDeleteParams) (*consistency_group.ConsistencyGroupDeleteNoContent, error)
	ConsistencyGroupFetch(*consistency_group.ConsistencyGroupFetchParams) (*consistency_group.ConsistencyGroupFetchOK, error)
	ConsistencyGroupList(*consistency_group.ConsistencyGroupListParams) (*consistency_group.ConsistencyGroupListOK, error)
	ConsistencyGroupUpdate(*consistency_group.ConsistencyGroupUpdateParams) (*consistency_group.ConsistencyGroupUpdateOK, error)
}

// CSPCredentialClient returns the operational interface
type CSPCredentialClient interface {
	CspCredentialCreate(*csp_credential.CspCredentialCreateParams) (*csp_credential.CspCredentialCreateCreated, error)
	CspCredentialDelete(*csp_credential.CspCredentialDeleteParams) (*csp_credential.CspCredentialDeleteNoContent, error)
	CspCredentialFetch(*csp_credential.CspCredentialFetchParams) (*csp_credential.CspCredentialFetchOK, error)
	CspCredentialList(*csp_credential.CspCredentialListParams) (*csp_credential.CspCredentialListOK, error)
	CspCredentialUpdate(*csp_credential.CspCredentialUpdateParams) (*csp_credential.CspCredentialUpdateOK, error)
	CspCredentialMetadata(*csp_credential.CspCredentialMetadataParams) (*csp_credential.CspCredentialMetadataOK, error)
}

// CSPDomainClient returns the operational interface
type CSPDomainClient interface {
	CspDomainCreate(*csp_domain.CspDomainCreateParams) (*csp_domain.CspDomainCreateCreated, error)
	CspDomainDelete(*csp_domain.CspDomainDeleteParams) (*csp_domain.CspDomainDeleteNoContent, error)
	CspDomainDeploymentFetch(*csp_domain.CspDomainDeploymentFetchParams) (*csp_domain.CspDomainDeploymentFetchOK, error)
	CspDomainFetch(*csp_domain.CspDomainFetchParams) (*csp_domain.CspDomainFetchOK, error)
	CspDomainList(*csp_domain.CspDomainListParams) (*csp_domain.CspDomainListOK, error)
	CspDomainMetadata(*csp_domain.CspDomainMetadataParams) (*csp_domain.CspDomainMetadataOK, error)
	CspDomainServicePlanCost(*csp_domain.CspDomainServicePlanCostParams) (*csp_domain.CspDomainServicePlanCostOK, error)
	CspDomainUpdate(*csp_domain.CspDomainUpdateParams) (*csp_domain.CspDomainUpdateOK, error)
}

// CSPStorageTypeClient returns the operational interface
type CSPStorageTypeClient interface {
	CspStorageTypeFetch(*csp_storage_type.CspStorageTypeFetchParams) (*csp_storage_type.CspStorageTypeFetchOK, error)
	CspStorageTypeList(*csp_storage_type.CspStorageTypeListParams) (*csp_storage_type.CspStorageTypeListOK, error)
}

// DebugClient returns the operational interface
type DebugClient interface {
	DebugPost(*service_debug.DebugPostParams) (*service_debug.DebugPostNoContent, error)
}

// MetricsClient returns the operational interface
type MetricsClient interface {
	StorageIOMetricUpload(*metrics.StorageIOMetricUploadParams) (*metrics.StorageIOMetricUploadNoContent, error)
	VolumeSeriesIOMetricUpload(*metrics.VolumeSeriesIOMetricUploadParams) (*metrics.VolumeSeriesIOMetricUploadNoContent, error)
}

// NodeClient returns the operational interface
type NodeClient interface {
	NodeCreate(*node.NodeCreateParams) (*node.NodeCreateCreated, error)
	NodeDelete(*node.NodeDeleteParams) (*node.NodeDeleteNoContent, error)
	NodeFetch(*node.NodeFetchParams) (*node.NodeFetchOK, error)
	NodeList(*node.NodeListParams) (*node.NodeListOK, error)
	NodeUpdate(*node.NodeUpdateParams) (*node.NodeUpdateOK, error)
}

// PoolClient returns the operational interface
type PoolClient interface {
	PoolCreate(*pool.PoolCreateParams) (*pool.PoolCreateCreated, error)
	PoolDelete(*pool.PoolDeleteParams) (*pool.PoolDeleteNoContent, error)
	PoolFetch(*pool.PoolFetchParams) (*pool.PoolFetchOK, error)
	PoolList(*pool.PoolListParams) (*pool.PoolListOK, error)
	PoolUpdate(*pool.PoolUpdateParams) (*pool.PoolUpdateOK, error)
}

// ProtectionDomainClient returns the operational interface
type ProtectionDomainClient interface {
	ProtectionDomainCreate(*protection_domain.ProtectionDomainCreateParams) (*protection_domain.ProtectionDomainCreateCreated, error)
	ProtectionDomainDelete(*protection_domain.ProtectionDomainDeleteParams) (*protection_domain.ProtectionDomainDeleteNoContent, error)
	ProtectionDomainFetch(*protection_domain.ProtectionDomainFetchParams) (*protection_domain.ProtectionDomainFetchOK, error)
	ProtectionDomainList(*protection_domain.ProtectionDomainListParams) (*protection_domain.ProtectionDomainListOK, error)
	ProtectionDomainMetadata(*protection_domain.ProtectionDomainMetadataParams) (*protection_domain.ProtectionDomainMetadataOK, error)
	ProtectionDomainUpdate(*protection_domain.ProtectionDomainUpdateParams) (*protection_domain.ProtectionDomainUpdateOK, error)
}

// RoleClient returns the operational interface
type RoleClient interface {
	RoleList(*role.RoleListParams) (*role.RoleListOK, error)
}

// ServicePlanAllocationClient returns the operational interface
type ServicePlanAllocationClient interface {
	ServicePlanAllocationCreate(*service_plan_allocation.ServicePlanAllocationCreateParams) (*service_plan_allocation.ServicePlanAllocationCreateCreated, error)
	ServicePlanAllocationCustomizeProvisioning(params *service_plan_allocation.ServicePlanAllocationCustomizeProvisioningParams) (*service_plan_allocation.ServicePlanAllocationCustomizeProvisioningOK, error)
	ServicePlanAllocationDelete(*service_plan_allocation.ServicePlanAllocationDeleteParams) (*service_plan_allocation.ServicePlanAllocationDeleteNoContent, error)
	ServicePlanAllocationFetch(*service_plan_allocation.ServicePlanAllocationFetchParams) (*service_plan_allocation.ServicePlanAllocationFetchOK, error)
	ServicePlanAllocationList(*service_plan_allocation.ServicePlanAllocationListParams) (*service_plan_allocation.ServicePlanAllocationListOK, error)
	ServicePlanAllocationUpdate(*service_plan_allocation.ServicePlanAllocationUpdateParams) (*service_plan_allocation.ServicePlanAllocationUpdateOK, error)
}

// ServicePlanClient returns the operational interface
type ServicePlanClient interface {
	ServicePlanClone(*service_plan.ServicePlanCloneParams) (*service_plan.ServicePlanCloneCreated, error)
	ServicePlanDelete(*service_plan.ServicePlanDeleteParams) (*service_plan.ServicePlanDeleteNoContent, error)
	ServicePlanFetch(*service_plan.ServicePlanFetchParams) (*service_plan.ServicePlanFetchOK, error)
	ServicePlanList(*service_plan.ServicePlanListParams) (*service_plan.ServicePlanListOK, error)
	ServicePlanPublish(*service_plan.ServicePlanPublishParams) (*service_plan.ServicePlanPublishOK, error)
	ServicePlanRetire(*service_plan.ServicePlanRetireParams) (*service_plan.ServicePlanRetireOK, error)
	ServicePlanUpdate(*service_plan.ServicePlanUpdateParams) (*service_plan.ServicePlanUpdateOK, error)
}

// SLOClient returns the operational interface
type SLOClient interface {
	SloList(*slo.SloListParams) (*slo.SloListOK, error)
}

// SnapshotClient returns the operational interface
type SnapshotClient interface {
	SnapshotCreate(*snapshot.SnapshotCreateParams) (*snapshot.SnapshotCreateCreated, error)
	SnapshotDelete(*snapshot.SnapshotDeleteParams) (*snapshot.SnapshotDeleteNoContent, error)
	SnapshotFetch(*snapshot.SnapshotFetchParams) (*snapshot.SnapshotFetchOK, error)
	SnapshotList(*snapshot.SnapshotListParams) (*snapshot.SnapshotListOK, error)
	SnapshotUpdate(*snapshot.SnapshotUpdateParams) (*snapshot.SnapshotUpdateOK, error)
}

// StorageClient returns the operational interface
type StorageClient interface {
	StorageCreate(*storage.StorageCreateParams) (*storage.StorageCreateCreated, error)
	StorageDelete(*storage.StorageDeleteParams) (*storage.StorageDeleteNoContent, error)
	StorageFetch(*storage.StorageFetchParams) (*storage.StorageFetchOK, error)
	StorageList(*storage.StorageListParams) (*storage.StorageListOK, error)
	StorageUpdate(*storage.StorageUpdateParams) (*storage.StorageUpdateOK, error)
}

// StorageFormulaClient returns the operational interface
type StorageFormulaClient interface {
	StorageFormulaList(*storage_formula.StorageFormulaListParams) (*storage_formula.StorageFormulaListOK, error)
}

// StorageRequestClient returns the operational interface
type StorageRequestClient interface {
	StorageRequestCreate(*storage_request.StorageRequestCreateParams) (*storage_request.StorageRequestCreateCreated, error)
	StorageRequestDelete(*storage_request.StorageRequestDeleteParams) (*storage_request.StorageRequestDeleteNoContent, error)
	StorageRequestFetch(*storage_request.StorageRequestFetchParams) (*storage_request.StorageRequestFetchOK, error)
	StorageRequestList(*storage_request.StorageRequestListParams) (*storage_request.StorageRequestListOK, error)
	StorageRequestUpdate(*storage_request.StorageRequestUpdateParams) (*storage_request.StorageRequestUpdateOK, error)
}

// SystemClient returns the operational interface
type SystemClient interface {
	SystemFetch(*system.SystemFetchParams) (*system.SystemFetchOK, error)
	SystemUpdate(*system.SystemUpdateParams) (*system.SystemUpdateOK, error)
	SystemHostnameFetch(*system.SystemHostnameFetchParams) (*system.SystemHostnameFetchOK, error)
}

// TaskClient returns the operational interface
type TaskClient interface {
	TaskCancel(params *task.TaskCancelParams) (*task.TaskCancelOK, error)
	TaskCreate(params *task.TaskCreateParams) (*task.TaskCreateCreated, error)
	TaskFetch(params *task.TaskFetchParams) (*task.TaskFetchOK, error)
	TaskList(params *task.TaskListParams) (*task.TaskListOK, error)
}

// UserClient returns the operational interface
type UserClient interface {
	UserCreate(*user.UserCreateParams) (*user.UserCreateCreated, error)
	UserDelete(*user.UserDeleteParams) (*user.UserDeleteNoContent, error)
	UserFetch(*user.UserFetchParams) (*user.UserFetchOK, error)
	UserList(*user.UserListParams) (*user.UserListOK, error)
	UserUpdate(*user.UserUpdateParams) (*user.UserUpdateOK, error)
}

// VolumeSeriesClient returns the operational interface
type VolumeSeriesClient interface {
	VolumeSeriesCreate(*volume_series.VolumeSeriesCreateParams) (*volume_series.VolumeSeriesCreateCreated, error)
	VolumeSeriesDelete(*volume_series.VolumeSeriesDeleteParams) (*volume_series.VolumeSeriesDeleteNoContent, error)
	VolumeSeriesFetch(*volume_series.VolumeSeriesFetchParams) (*volume_series.VolumeSeriesFetchOK, error)
	VolumeSeriesList(*volume_series.VolumeSeriesListParams) (*volume_series.VolumeSeriesListOK, error)
	VolumeSeriesNewID(params *volume_series.VolumeSeriesNewIDParams) (*volume_series.VolumeSeriesNewIDOK, error)
	VolumeSeriesPVSpecFetch(*volume_series.VolumeSeriesPVSpecFetchParams) (*volume_series.VolumeSeriesPVSpecFetchOK, error)
	VolumeSeriesUpdate(*volume_series.VolumeSeriesUpdateParams) (*volume_series.VolumeSeriesUpdateOK, error)
}

// VolumeSeriesRequestClient returns the operational interface
type VolumeSeriesRequestClient interface {
	VolumeSeriesRequestCancel(*volume_series_request.VolumeSeriesRequestCancelParams) (*volume_series_request.VolumeSeriesRequestCancelOK, error)
	VolumeSeriesRequestCreate(*volume_series_request.VolumeSeriesRequestCreateParams) (*volume_series_request.VolumeSeriesRequestCreateCreated, error)
	VolumeSeriesRequestDelete(*volume_series_request.VolumeSeriesRequestDeleteParams) (*volume_series_request.VolumeSeriesRequestDeleteNoContent, error)
	VolumeSeriesRequestFetch(*volume_series_request.VolumeSeriesRequestFetchParams) (*volume_series_request.VolumeSeriesRequestFetchOK, error)
	VolumeSeriesRequestList(*volume_series_request.VolumeSeriesRequestListParams) (*volume_series_request.VolumeSeriesRequestListOK, error)
	VolumeSeriesRequestUpdate(*volume_series_request.VolumeSeriesRequestUpdateParams) (*volume_series_request.VolumeSeriesRequestUpdateOK, error)
}

// WatchersClient returns the operational interface
type WatchersClient interface {
	WatcherCreate(*watchers.WatcherCreateParams) (*watchers.WatcherCreateCreated, error)
	// WatcherFetch(*watchers.WatcherFetchParams) (*watchers.WatcherFetchOK, error) WebSocket client needed
}
