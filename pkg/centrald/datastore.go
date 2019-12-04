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


package centrald

import (
	"context"
	"net/http"

	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/application_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/cluster"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/consistency_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/csp_credential"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/node"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/pool"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/protection_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/role"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/service_plan"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/service_plan_allocation"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/snapshot"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/storage"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/user"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
)

// DataStore interface defines methods to establish and tear-down access to a datastore.
// Implementation should provide a NewDataStore(args) method to return such an interface.
type DataStore interface {
	// Start establishes a connection to the database and performs
	// any first time initialization necessary
	Start() error
	// Stop terminates access with the database
	Stop()
	// OpsAccount() returns the datastore interface for Account
	OpsAccount() AccountOps
	// OpsApplicationGroup() returns the datastore interface for ApplicationGroup
	OpsApplicationGroup() ApplicationGroupOps
	// OpsCluster() returns the datastore interface for Cluster
	OpsCluster() ClusterOps
	// OpsConsistencyGroup() returns the datastore interface for ConsistencyGroup
	OpsConsistencyGroup() ConsistencyGroupOps
	// OpsCspCredential() returns the datastore interface for CspCredential
	OpsCspCredential() CspCredentialOps
	// OpsCspDomain() returns the datastore interface for CspDomain
	OpsCspDomain() CspDomainOps
	// OpsNode() returns the datastore interface for Node
	OpsNode() NodeOps
	// OpsPool() returns the datastore interface for Pool
	OpsPool() PoolOps
	// OpsProtectionDomain() returns the datastore interface for ProtectionDomain
	OpsProtectionDomain() ProtectionDomainOps
	// OpsRole() returns the datastore interface for Role
	OpsRole() RoleOps
	// OpsServicePlan() returns the datastore interface for ServicePlan
	OpsServicePlan() ServicePlanOps
	// OpsServicePlanAllocation() returns the datastore interface for ServicePlan
	OpsServicePlanAllocation() ServicePlanAllocationOps
	// OpsSnapshot() returns the datastore interface for Snapshot
	OpsSnapshot() SnapshotOps
	// OpsStorage() returns the datastore interface for Storage
	OpsStorage() StorageOps
	// OpsStorageRequest() returns the datastore interface for StorageRequest
	OpsStorageRequest() StorageRequestOps
	// OpsSystem() returns the datastore interface for System
	OpsSystem() SystemOps
	// OpsUser() returns the datastore interface for User
	OpsUser() UserOps
	// OpsVolumeSeries() returns the datastore interface for VolumeSeries
	OpsVolumeSeries() VolumeSeriesOps
	// OpsVolumeSeriesRequest() returns the datastore interface for VolumeSeriesRequest
	OpsVolumeSeriesRequest() VolumeSeriesRequestOps
}

// Aggregation stores the result if a single aggregation
type Aggregation struct {
	FieldPath string
	Type      string
	Value     int64
}

// Aggregation types
const (
	SumType = "sum"
)

// Error returns an HTTP code and an error message and meets the error interface definition
// All the operations return this type of error object
type Error struct {
	M string // message
	C int    // http code
}

func (e *Error) Error() string {
	return e.M
}

// ErrorExists returned if an attempt is made to create a duplicate object
var ErrorExists = &Error{com.ErrorExists, http.StatusConflict}

// ErrorIDVerNotFound returned if a version of an object could not be found
var ErrorIDVerNotFound = &Error{com.ErrorIDVerNotFound, http.StatusNotFound}

// ErrorInvalidData returned if data not available for processing
var ErrorInvalidData = &Error{com.ErrorInvalidData, http.StatusBadRequest}

// ErrorMissing may have additional explanation
var ErrorMissing = &Error{com.ErrorMissing, http.StatusBadRequest}

// ErrorNotFound returned if an object could not be found
var ErrorNotFound = &Error{com.ErrorNotFound, http.StatusNotFound}

// ErrorDbError returned for unspecific database errors
var ErrorDbError = &Error{com.ErrorDbError, http.StatusInternalServerError}

// ErrorInternalError returned for unspecific internal errors
var ErrorInternalError = &Error{com.ErrorInternalError, http.StatusInternalServerError}

// ErrorUpdateInvalidRequest returned for invalid args or no change
// Additional explanation could be returned.
var ErrorUpdateInvalidRequest = &Error{com.ErrorUpdateInvalidRequest, http.StatusBadRequest}

// ErrorUnauthorizedOrForbidden if not authorized or operation is not permitted
var ErrorUnauthorizedOrForbidden = &Error{com.ErrorUnauthorizedOrForbidden, http.StatusForbidden}

// ErrorRequestInConflict if an active operation conflicts with a request
var ErrorRequestInConflict = &Error{com.ErrorRequestInConflict, http.StatusConflict}

// ErrorInvalidState if an operation cannot be performed because of the state of an object
var ErrorInvalidState = &Error{com.ErrorInvalidState, http.StatusConflict}

// UpdateAction specifies an action type
type UpdateAction int

const (
	// UpdateRemove specifies that the values should be removed from a list or map
	UpdateRemove UpdateAction = iota
	// UpdateAppend specifies that the new values should be appended to a list or map
	UpdateAppend
	// UpdateSet specifies that the value should be replaced
	UpdateSet
	// NumActionTypes counts the number of actions
	NumActionTypes
)

// UpdateActionArgs describes what to do for a given action type
type UpdateActionArgs struct {
	// FromBody indicates that value should be taken from the body
	FromBody bool
	// Fields specifies distinct field names
	Fields map[string]struct{}
	// Indexes specifies distinct index numbers
	Indexes map[int]struct{}
}

// IsModified determines if the action implies modification
func (actA *UpdateActionArgs) IsModified() bool {
	if actA.FromBody || len(actA.Fields) > 0 || len(actA.Indexes) > 0 {
		return true
	}
	return false
}

// UpdateAttr specifies how an attribute is to be updated
type UpdateAttr struct {
	// Name of the attribute
	Name string
	// Actions are processed in UpdateAction order
	Actions [NumActionTypes]UpdateActionArgs
}

// IsModified determines if an attribute is modified
func (a *UpdateAttr) IsModified() bool {
	for _, actA := range a.Actions {
		if actA.IsModified() {
			return true
		}
	}
	return false
}

// UpdateArgs are common arguments for all update methods
type UpdateArgs struct {
	// ID of the object being updated
	ID string
	// Version of the object being updated or 0 if not specified
	Version int32
	// Attributes contain instructions for each attribute
	Attributes []UpdateAttr
	// HasChanges indicates if there are changes requested
	HasChanges bool
}

// FindUpdateAttr returns the ua.Attribute structure for a field name, or nil if not found
func (ua *UpdateArgs) FindUpdateAttr(name string) *UpdateAttr {
	for i, upA := range ua.Attributes {
		if upA.Name == name {
			return &ua.Attributes[i]
		}
	}
	return nil
}

// IsModified determines if an attribute is modified
func (ua *UpdateArgs) IsModified(name string) bool {
	if a := ua.FindUpdateAttr(name); a != nil {
		return a.IsModified()
	}
	return false
}

// AnyModified determines if any attributes listed are modified (those not listed may also be modified)
func (ua *UpdateArgs) AnyModified(names ...string) bool {
	for i, upA := range ua.Attributes {
		if util.Contains(names, upA.Name) && ua.Attributes[i].IsModified() {
			return true
		}
	}
	return false
}

// OthersModified determines if any attributes not listed are modified (those listed may also be modified)
func (ua *UpdateArgs) OthersModified(names ...string) bool {
	for i, upA := range ua.Attributes {
		if !util.Contains(names, upA.Name) && ua.Attributes[i].IsModified() {
			return true
		}
	}
	return false
}

// AccountOps is an interface to operate on Account objects in the datastore
type AccountOps interface {
	Count(context.Context, account.AccountListParams, uint) (int, error)
	Create(context.Context, *M.Account) (*M.Account, error)
	Delete(context.Context, string) error
	Fetch(context.Context, string) (*M.Account, error)
	List(context.Context, account.AccountListParams) ([]*M.Account, error)
	Update(context.Context, *UpdateArgs, *M.AccountMutable) (*M.Account, error)
}

// ApplicationGroupOps is an interface to operate on ApplicationGroup objects in the datastore
type ApplicationGroupOps interface {
	Count(context.Context, application_group.ApplicationGroupListParams, uint) (int, error)
	Create(context.Context, *M.ApplicationGroup) (*M.ApplicationGroup, error)
	Delete(context.Context, string) error
	Fetch(context.Context, string) (*M.ApplicationGroup, error)
	List(context.Context, application_group.ApplicationGroupListParams) ([]*M.ApplicationGroup, error)
	Update(context.Context, *UpdateArgs, *M.ApplicationGroupMutable) (*M.ApplicationGroup, error)
}

// ClusterOps is an interface to operate on Cluster objects in the datastore
type ClusterOps interface {
	Count(context.Context, cluster.ClusterListParams, uint) (int, error)
	Create(context.Context, *M.ClusterCreateArgs) (*M.Cluster, error)
	Delete(context.Context, string) error
	Fetch(context.Context, string) (*M.Cluster, error)
	List(context.Context, cluster.ClusterListParams) ([]*M.Cluster, error)
	Update(context.Context, *UpdateArgs, *M.ClusterMutable) (*M.Cluster, error)
}

// ConsistencyGroupOps is an interface to operate on ConsistencyGroup objects in the datastore
type ConsistencyGroupOps interface {
	Count(context.Context, consistency_group.ConsistencyGroupListParams, uint) (int, error)
	Create(context.Context, *M.ConsistencyGroup) (*M.ConsistencyGroup, error)
	Delete(context.Context, string) error
	Fetch(context.Context, string) (*M.ConsistencyGroup, error)
	List(context.Context, consistency_group.ConsistencyGroupListParams) ([]*M.ConsistencyGroup, error)
	Update(context.Context, *UpdateArgs, *M.ConsistencyGroupMutable) (*M.ConsistencyGroup, error)
}

// CspCredentialOps is an interface to operate on CspCredential objects in the datastore
type CspCredentialOps interface {
	Count(context.Context, csp_credential.CspCredentialListParams, uint) (int, error)
	Create(context.Context, *M.CSPCredential) (*M.CSPCredential, error)
	Delete(context.Context, string) error
	Fetch(context.Context, string) (*M.CSPCredential, error)
	List(context.Context, csp_credential.CspCredentialListParams) ([]*M.CSPCredential, error)
	Update(context.Context, *UpdateArgs, *M.CSPCredentialMutable) (*M.CSPCredential, error)
}

// CspDomainOps is an interface to operate on CspDomain objects in the datastore
type CspDomainOps interface {
	Count(context.Context, csp_domain.CspDomainListParams, uint) (int, error)
	Create(context.Context, *M.CSPDomain) (*M.CSPDomain, error)
	Delete(context.Context, string) error
	Fetch(context.Context, string) (*M.CSPDomain, error)
	List(context.Context, csp_domain.CspDomainListParams) ([]*M.CSPDomain, error)
	Update(context.Context, *UpdateArgs, *M.CSPDomainMutable) (*M.CSPDomain, error)
}

// NodeOps is an interface to operate on Node objects in the datastore
type NodeOps interface {
	Count(context.Context, node.NodeListParams, uint) (int, error)
	Create(context.Context, *M.Node) (*M.Node, error)
	Delete(context.Context, string) error
	Fetch(context.Context, string) (*M.Node, error)
	List(context.Context, node.NodeListParams) ([]*M.Node, error)
	Update(context.Context, *UpdateArgs, *M.NodeMutable) (*M.Node, error)
	UpdateMultiple(context.Context, node.NodeListParams, *UpdateArgs, *M.NodeMutable) (int, int, error)
}

// PoolOps is an interface to operate on Pool objects in the datastore
type PoolOps interface {
	Count(context.Context, pool.PoolListParams, uint) (int, error)
	Create(context.Context, *M.PoolCreateArgs) (*M.Pool, error)
	Delete(context.Context, string) error
	Fetch(context.Context, string) (*M.Pool, error)
	List(context.Context, pool.PoolListParams) ([]*M.Pool, error)
	Update(context.Context, *UpdateArgs, *M.PoolMutable) (*M.Pool, error)
}

// ProtectionDomainOps is an interface to operate on ProtectionDomain objects in the datastore
type ProtectionDomainOps interface {
	Count(context.Context, protection_domain.ProtectionDomainListParams, uint) (int, error)
	Create(context.Context, *M.ProtectionDomainCreateArgs) (*M.ProtectionDomain, error)
	Delete(context.Context, string) error
	Fetch(context.Context, string) (*M.ProtectionDomain, error)
	List(context.Context, protection_domain.ProtectionDomainListParams) ([]*M.ProtectionDomain, error)
	Update(context.Context, *UpdateArgs, *M.ProtectionDomainMutable) (*M.ProtectionDomain, error)
}

// RoleOps is an interface to operate on Role objects in the datastore
type RoleOps interface {
	Fetch(string) (*M.Role, error)
	List(role.RoleListParams) ([]*M.Role, error)
}

// ServicePlanOps is an interface to operate on ServicePlan objects in the datastore
type ServicePlanOps interface {
	BuiltInPlan(string) (string, bool)
	Clone(context.Context, service_plan.ServicePlanCloneParams) (*M.ServicePlan, error)
	Count(context.Context, service_plan.ServicePlanListParams, uint) (int, error)
	Delete(context.Context, string) error
	Fetch(context.Context, string) (*M.ServicePlan, error)
	List(context.Context, service_plan.ServicePlanListParams) ([]*M.ServicePlan, error)
	Publish(context.Context, service_plan.ServicePlanPublishParams) (*M.ServicePlan, error)
	RemoveAccount(context.Context, string) error
	Retire(context.Context, service_plan.ServicePlanRetireParams) (*M.ServicePlan, error)
	Update(context.Context, *UpdateArgs, *M.ServicePlanMutable) (*M.ServicePlan, error)
}

// ServicePlanAllocationOps is an interface to operate on ServicePlanAllocation objects in the datastore
type ServicePlanAllocationOps interface {
	Count(context.Context, service_plan_allocation.ServicePlanAllocationListParams, uint) (int, error)
	Create(context.Context, *M.ServicePlanAllocation) (*M.ServicePlanAllocation, error)
	Delete(context.Context, string) error
	Fetch(context.Context, string) (*M.ServicePlanAllocation, error)
	List(context.Context, service_plan_allocation.ServicePlanAllocationListParams) ([]*M.ServicePlanAllocation, error)
	Update(context.Context, *UpdateArgs, *M.ServicePlanAllocationMutable) (*M.ServicePlanAllocation, error)
}

// SnapshotOps is an interface to operate on Snapshot objects in the datastore
type SnapshotOps interface {
	Count(context.Context, snapshot.SnapshotListParams, uint) (int, error)
	Create(context.Context, *M.Snapshot) (*M.Snapshot, error)
	Delete(context.Context, string) error
	Fetch(context.Context, string) (*M.Snapshot, error)
	List(context.Context, snapshot.SnapshotListParams) ([]*M.Snapshot, error)
	Update(context.Context, *UpdateArgs, *M.SnapshotMutable) (*M.Snapshot, error)
}

// StorageOps is an interface to operate on Storage objects in the datastore
type StorageOps interface {
	Aggregate(context.Context, storage.StorageListParams) ([]*Aggregation, int, error)
	Count(context.Context, storage.StorageListParams, uint) (int, error)
	Create(context.Context, *M.Storage) (*M.Storage, error)
	Delete(context.Context, string) error
	Fetch(context.Context, string) (*M.Storage, error)
	List(context.Context, storage.StorageListParams) ([]*M.Storage, error)
	Update(context.Context, *UpdateArgs, *M.StorageMutable) (*M.Storage, error)
}

// StorageRequestOps is an interface to operate on StorageRequest objects in the datastore
type StorageRequestOps interface {
	Count(context.Context, storage_request.StorageRequestListParams, uint) (int, error)
	Create(context.Context, *M.StorageRequestCreateArgs) (*M.StorageRequest, error)
	Delete(context.Context, string) error
	Fetch(context.Context, string) (*M.StorageRequest, error)
	List(context.Context, storage_request.StorageRequestListParams) ([]*M.StorageRequest, error)
	Update(context.Context, *UpdateArgs, *M.StorageRequestMutable) (*M.StorageRequest, error)
}

// SystemOps is an interface to operate on the System object in the datastore
type SystemOps interface {
	Fetch() (*M.System, error)
	Update(context.Context, *UpdateArgs, *M.SystemMutable) (*M.System, error)
}

// UserOps is an interface to operate on User objects in the datastore
type UserOps interface {
	Create(context.Context, *M.User) (*M.User, error)
	Delete(context.Context, string) error
	Fetch(context.Context, string) (*M.User, error)
	List(context.Context, user.UserListParams) ([]*M.User, error)
	Update(context.Context, *UpdateArgs, *M.UserMutable) (*M.User, error)
}

// VolumeSeriesOps is an interface to operate on VolumeSeries objects in the datastore
type VolumeSeriesOps interface {
	NewID() string
	Aggregate(context.Context, volume_series.VolumeSeriesListParams) ([]*Aggregation, int, error)
	Count(context.Context, volume_series.VolumeSeriesListParams, uint) (int, error)
	Create(context.Context, *M.VolumeSeriesCreateArgs) (*M.VolumeSeries, error)
	Delete(context.Context, string) error
	Fetch(context.Context, string) (*M.VolumeSeries, error)
	List(context.Context, volume_series.VolumeSeriesListParams) ([]*M.VolumeSeries, error)
	Update(context.Context, *UpdateArgs, *M.VolumeSeriesMutable) (*M.VolumeSeries, error)
}

// VolumeSeriesRequestOps is an interface to operate on VolumeSeriesRequest objects in the datastore
type VolumeSeriesRequestOps interface {
	Cancel(context.Context, string, int32) (*M.VolumeSeriesRequest, error)
	Count(context.Context, volume_series_request.VolumeSeriesRequestListParams, uint) (int, error)
	Create(context.Context, *M.VolumeSeriesRequestCreateArgs) (*M.VolumeSeriesRequest, error)
	Delete(context.Context, string) error
	Fetch(context.Context, string) (*M.VolumeSeriesRequest, error)
	List(context.Context, volume_series_request.VolumeSeriesRequestListParams) ([]*M.VolumeSeriesRequest, error)
	Update(context.Context, *UpdateArgs, *M.VolumeSeriesRequestMutable) (*M.VolumeSeriesRequest, error)
}
