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


// Package mongods provides a MongoDB adaptor to the data store abstraction layer
package mongods

import (
	"fmt"

	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/mongodb"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// MongoDataStoreArgs contains the arguments to create a MongoDataStore
type MongoDataStoreArgs struct {
	mongodb.Args

	BaseDataPath string `long:"base-data-path" description:"The path to files containing base configuration data for the datastore" default:"/opt/nuvoloso/lib/base-data"`
}

var (
	// ActiveRequestsOnlyIndex is a partial index containing only documents where the terminated field is false
	ActiveRequestsOnlyIndex = options.Index().SetPartialFilterExpression(bsonx.Doc{{Key: terminatedKey, Value: bsonx.Document(bsonx.Doc{{Key: "$eq", Value: bsonx.Boolean(false)}})}})

	// IndexAscending is the index sort order value to sort ascending
	IndexAscending = bsonx.Int32(1)
)

// NewDataStore returns a DataStore object
func NewDataStore(args *MongoDataStoreArgs) *MongoDataStore {
	ds := &MongoDataStore{
		DBAPI:        mongodb.NewDBAPI(&args.Args),
		BaseDataPath: args.BaseDataPath,
	}
	ds.dsCRUD = NewObjectDocumentHandlerCRUD(ds)
	return ds
}

// MongoDataStore contains the datastore connection information
// It satisfies the following interfaces:
// - ds.Datastore
// - DBAPI
type MongoDataStore struct {
	mongodb.DBAPI
	dsCRUD       ObjectDocumentHandlerCRUD
	BaseDataPath string

	opsAccount               ds.AccountOps
	opsApplicationGroup      ds.ApplicationGroupOps
	opsCluster               ds.ClusterOps
	opsConsistencyGroup      ds.ConsistencyGroupOps
	opsCspCredential         ds.CspCredentialOps
	opsCspDomain             ds.CspDomainOps
	opsNode                  ds.NodeOps
	opsPool                  ds.PoolOps
	opsProtectionDomain      ds.ProtectionDomainOps
	opsRole                  ds.RoleOps
	opsServicePlan           ds.ServicePlanOps
	opsServicePlanAllocation ds.ServicePlanAllocationOps
	opsSnapshot              ds.SnapshotOps
	opsStorage               ds.StorageOps
	opsStorageRequest        ds.StorageRequestOps
	opsSystem                ds.SystemOps
	opsUser                  ds.UserOps
	opsVolumeSeries          ds.VolumeSeriesOps
	opsVolumeSeriesRequest   ds.VolumeSeriesRequestOps
}

var _ = ds.DataStore(&MongoDataStore{})
var _ = DBAPI(&MongoDataStore{})

// Start initiates the connection to the Mongo database.
// Upon success, the connection may not actually be established (see mongodb.DBAPI.Connect).
// Errors are only returned when pre-connection initialization fails.
func (mds *MongoDataStore) Start() error {
	for name, h := range odhRegistry {
		mds.Logger().Info("Claiming ODH", name)
		odh := h.(ObjectDocumentHandler) // or panic
		odh.Claim(mds, mds.dsCRUD)
		ops := odh.Ops()
		switch x := ops.(type) {
		case ds.AccountOps:
			mds.opsAccount = x
		case ds.ApplicationGroupOps:
			mds.opsApplicationGroup = x
		case ds.ClusterOps:
			mds.opsCluster = x
		case ds.ConsistencyGroupOps:
			mds.opsConsistencyGroup = x
		case ds.CspCredentialOps:
			mds.opsCspCredential = x
		case ds.CspDomainOps:
			mds.opsCspDomain = x
		case ds.NodeOps:
			mds.opsNode = x
		case ds.RoleOps:
			mds.opsRole = x
		case ds.PoolOps:
			mds.opsPool = x
		case ds.ProtectionDomainOps:
			mds.opsProtectionDomain = x
		case ds.ServicePlanOps:
			mds.opsServicePlan = x
		case ds.ServicePlanAllocationOps:
			mds.opsServicePlanAllocation = x
		case ds.SnapshotOps:
			mds.opsSnapshot = x
		case ds.StorageOps:
			mds.opsStorage = x
		case ds.StorageRequestOps:
			mds.opsStorageRequest = x
		case ds.SystemOps:
			mds.opsSystem = x
		case ds.UserOps:
			mds.opsUser = x
		case ds.VolumeSeriesOps:
			mds.opsVolumeSeries = x
		case ds.VolumeSeriesRequestOps:
			mds.opsVolumeSeriesRequest = x
		default:
			mds.Logger().Info("Not using ODH", name)
		}
	}
	return mds.Connect(odhRegistry)
}

// Stop will close the connection to Mongo
func (mds *MongoDataStore) Stop() {
	mds.Terminate()
}

// OpsAccount returns operations on Accounts
func (mds *MongoDataStore) OpsAccount() ds.AccountOps {
	return mds.opsAccount
}

// OpsApplicationGroup returns operations on ApplicationGroups
func (mds *MongoDataStore) OpsApplicationGroup() ds.ApplicationGroupOps {
	return mds.opsApplicationGroup
}

// OpsCluster returns operations on Clusters
func (mds *MongoDataStore) OpsCluster() ds.ClusterOps {
	return mds.opsCluster
}

// OpsConsistencyGroup returns operations on ConsistencyGroups
func (mds *MongoDataStore) OpsConsistencyGroup() ds.ConsistencyGroupOps {
	return mds.opsConsistencyGroup
}

// OpsCspCredential returns operations on CspCredential
func (mds *MongoDataStore) OpsCspCredential() ds.CspCredentialOps {
	return mds.opsCspCredential
}

// OpsCspDomain returns operations on CspDomains
func (mds *MongoDataStore) OpsCspDomain() ds.CspDomainOps {
	return mds.opsCspDomain
}

// OpsNode returns operations on Nodes
func (mds *MongoDataStore) OpsNode() ds.NodeOps {
	return mds.opsNode
}

// OpsPool returns operations on Pool
func (mds *MongoDataStore) OpsPool() ds.PoolOps {
	return mds.opsPool
}

// OpsProtectionDomain returns operations on ProtectionDomain
func (mds *MongoDataStore) OpsProtectionDomain() ds.ProtectionDomainOps {
	return mds.opsProtectionDomain
}

// OpsRole returns operations on Roles
func (mds *MongoDataStore) OpsRole() ds.RoleOps {
	return mds.opsRole
}

// OpsSnapshot returns operations on Snapshot
func (mds *MongoDataStore) OpsSnapshot() ds.SnapshotOps {
	return mds.opsSnapshot
}

// OpsServicePlan returns operations on ServicePlan
func (mds *MongoDataStore) OpsServicePlan() ds.ServicePlanOps {
	return mds.opsServicePlan
}

// OpsServicePlanAllocation returns operations on ServicePlanAllocation
func (mds *MongoDataStore) OpsServicePlanAllocation() ds.ServicePlanAllocationOps {
	return mds.opsServicePlanAllocation
}

// OpsStorage returns operations on Storage
func (mds *MongoDataStore) OpsStorage() ds.StorageOps {
	return mds.opsStorage
}

// OpsStorageRequest returns operations on StorageRequest
func (mds *MongoDataStore) OpsStorageRequest() ds.StorageRequestOps {
	return mds.opsStorageRequest
}

// OpsSystem returns operations on the System object
func (mds *MongoDataStore) OpsSystem() ds.SystemOps {
	return mds.opsSystem
}

// OpsUser returns operations on Users
func (mds *MongoDataStore) OpsUser() ds.UserOps {
	return mds.opsUser
}

// OpsVolumeSeries returns operations on VolumeSeries
func (mds *MongoDataStore) OpsVolumeSeries() ds.VolumeSeriesOps {
	return mds.opsVolumeSeries
}

// OpsVolumeSeriesRequest returns operations on VolumeSeriesRequest
func (mds *MongoDataStore) OpsVolumeSeriesRequest() ds.VolumeSeriesRequestOps {
	return mds.opsVolumeSeriesRequest
}

// DBClient methods

// BaseDataPathName returns the root path of the data to be loaded into the database
func (mds *MongoDataStore) BaseDataPathName() string {
	return mds.BaseDataPath
}

// WrapError wraps an error resulting from a Mongo API call, handling special cases for which specific centrald errors exist (eg ErrorNotFound, ErrorExists).
// Other errors are returned as a centrald.Error with the code centrald.ErrorDbError.C
func (mds *MongoDataStore) WrapError(err error, isUpdate bool) error {
	code := mds.ErrorCode(err)
	switch code {
	case mongodb.ECDuplicateKey:
		return ds.ErrorExists
	case mongodb.ECKeyNotFound:
		if err == nil { // see ErrorCode behavior
			return err
		}
		if isUpdate {
			return ds.ErrorIDVerNotFound
		}
		return ds.ErrorNotFound
	}
	return &ds.Error{C: ds.ErrorDbError.C, M: fmt.Sprintf("%s: %s (%d)", ds.ErrorDbError.M, err.Error(), code)}
}
