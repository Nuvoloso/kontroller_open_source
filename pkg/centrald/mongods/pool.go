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
	"context"

	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/pool"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/op/go-logging"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// Pool object
type Pool struct {
	ObjMeta                 `bson:",inline"`
	SystemTags              StringList
	AccountID               string
	AuthorizedAccountID     string
	ClusterID               string
	CspDomainID             string
	CspStorageType          string
	StorageAccessibility    StorageAccessibility
	ServicePlanReservations StorageTypeReservationMap
}

// ToModel converts a datastore object to a model object
func (o *Pool) ToModel() *M.Pool {
	if o.SystemTags == nil {
		o.SystemTags = make(StringList, 0)
	}
	if o.ServicePlanReservations == nil {
		o.ServicePlanReservations = make(StorageTypeReservationMap, 0)
	}
	meta := &ObjMeta{}
	meta.CopyFromObject(o)
	mObj := &M.Pool{
		PoolAllOf0: M.PoolAllOf0{Meta: meta.ToModel("Pool")},
	}
	mObj.AccountID = M.ObjIDMutable(o.AccountID)
	mObj.AuthorizedAccountID = M.ObjIDMutable(o.AuthorizedAccountID)
	mObj.ClusterID = M.ObjIDMutable(o.ClusterID)
	mObj.CspDomainID = M.ObjIDMutable(o.CspDomainID)
	mObj.CspStorageType = o.CspStorageType
	mObj.SystemTags = *o.SystemTags.ToModel()
	mObj.StorageAccessibility = o.StorageAccessibility.ToModel()
	mObj.ServicePlanReservations = o.ServicePlanReservations.ToModel()
	return mObj
}

// FromModel converts a model object to a datastore object
func (o *Pool) FromModel(mObj *M.Pool) {
	meta := &ObjMeta{}
	mMeta := mObj.Meta
	if mMeta == nil {
		mMeta = &M.ObjMeta{ObjType: "Pool"}
	}
	meta.FromModel(mMeta)
	meta.CopyToObject(o)
	o.AccountID = string(mObj.AccountID)
	o.AuthorizedAccountID = string(mObj.AuthorizedAccountID)
	o.ClusterID = string(mObj.ClusterID)
	o.CspDomainID = string(mObj.CspDomainID)
	o.CspStorageType = string(mObj.CspStorageType)
	if o.SystemTags == nil {
		o.SystemTags = make(StringList, len(mObj.SystemTags))
	}
	o.SystemTags.FromModel(&mObj.SystemTags)
	sa := mObj.StorageAccessibility
	if sa == nil {
		sa = &M.StorageAccessibilityMutable{}
	}
	o.StorageAccessibility.FromModel(sa)
	if o.ServicePlanReservations == nil {
		o.ServicePlanReservations = make(StorageTypeReservationMap, len(mObj.ServicePlanReservations))
	}
	o.ServicePlanReservations.FromModel(mObj.ServicePlanReservations)
}

// poolHandler is an ObjectDocumentHandler that implements the ds.PoolOps operations
type poolHandler struct {
	cName   string
	api     DBAPI
	crud    ObjectDocumentHandlerCRUD
	log     *logging.Logger
	indexes []mongo.IndexModel
}

var _ = ds.PoolOps(&poolHandler{})
var _ = ObjectDocumentHandler(&poolHandler{})

var odhPool = &poolHandler{
	cName: "pool",
	indexes: []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: clusterIDKey, Value: IndexAscending}}},
		{Keys: bsonx.Doc{{Key: servicePlanAllocationIDKey, Value: IndexAscending}}},
	},
}

func init() {
	odhRegister(odhPool.cName, odhPool)
}

// convertListParams is a helper to convert PoolListParams to bson.M
func (op *poolHandler) convertListParams(params pool.PoolListParams) bson.M {
	qParams := bson.M{}
	if params.SystemTags != nil {
		qParams["systemtags"] = bson.M{"$all": params.SystemTags}
	}
	if params.AuthorizedAccountID != nil && *params.AuthorizedAccountID != "" {
		qParams["authorizedaccountid"] = *params.AuthorizedAccountID
	}
	if params.ClusterID != nil && *params.ClusterID != "" {
		qParams["clusterid"] = *params.ClusterID
	}
	if params.CspDomainID != nil && *params.CspDomainID != "" {
		qParams["cspdomainid"] = *params.CspDomainID
	}
	if params.CspStorageType != nil && *params.CspStorageType != "" {
		qParams["cspstoragetype"] = *params.CspStorageType
	}
	if params.AccessibilityScope != nil && *params.AccessibilityScope != "" {
		qParams["storageaccessibility.accessibilityscope"] = *params.AccessibilityScope
	}
	if params.AccessibilityScopeObjID != nil && *params.AccessibilityScopeObjID != "" {
		qParams["storageaccessibility.accessibilityscopeobjid"] = *params.AccessibilityScopeObjID
	}
	if params.ServicePlanAllocationID != nil && *params.ServicePlanAllocationID != "" {
		qParams["serviceplanreservations."+*params.ServicePlanAllocationID] = bson.M{"$exists": true}
	}
	op.log.Info("qParams:", qParams)
	return qParams
}

// objectDocumentHandler methods

func (op *poolHandler) Claim(api DBAPI, crud ObjectDocumentHandlerCRUD) {
	op.api = api
	op.crud = crud
	op.log = api.Logger()
}

func (op *poolHandler) Initialize(ctx context.Context) error {
	return op.crud.CreateIndexes(ctx, op)
}

func (op *poolHandler) Start(ctx context.Context) error {
	dbn := op.api.DBName()
	op.log.Infof("Starting collection %s.%s", dbn, op.cName)
	return nil
}

func (op *poolHandler) Ops() interface{} {
	return op
}

func (op *poolHandler) CName() string {
	return op.cName
}

func (op *poolHandler) Indexes() []mongo.IndexModel {
	return op.indexes
}

func (op *poolHandler) NewObject() interface{} {
	return &Pool{}
}

// ds.PoolOps methods

// Count the number of matching documents, limit is applied if non-zero
func (op *poolHandler) Count(ctx context.Context, params pool.PoolListParams, limit uint) (int, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Count:", err)
		return 0, err
	}
	return op.crud.Count(ctx, op, op.convertListParams(params), limit)
}

func (op *poolHandler) Create(ctx context.Context, mObj *M.PoolCreateArgs) (*M.Pool, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Create:", err)
		return nil, err
	}
	dObj := &Pool{}
	mPool := &M.Pool{
		PoolCreateOnce: mObj.PoolCreateOnce,
		PoolMutable: M.PoolMutable{
			PoolCreateMutable: mObj.PoolCreateMutable,
		},
	}
	dObj.FromModel(mPool)
	if err := op.crud.InsertOne(ctx, op, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *poolHandler) Delete(ctx context.Context, mID string) error {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Delete:", err)
		return err
	}
	return op.crud.DeleteOne(ctx, op, mID)
}

func (op *poolHandler) Fetch(ctx context.Context, mID string) (*M.Pool, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Fetch:", err)
		return nil, err
	}
	dObj := &Pool{}
	if err := op.crud.FindOne(ctx, op, bson.M{objKey: mID}, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *poolHandler) List(ctx context.Context, params pool.PoolListParams) ([]*M.Pool, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("List:", err)
		return nil, err
	}
	list := make([]*M.Pool, 0, 10)
	err := op.crud.FindAll(ctx, op, params, op.convertListParams(params), func(obj interface{}) {
		dObj := obj.(*Pool) // or panic
		list = append(list, dObj.ToModel())
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (op *poolHandler) Update(ctx context.Context, ua *ds.UpdateArgs, param *M.PoolMutable) (*M.Pool, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Update:", err)
		return nil, err
	}
	mObj := &M.Pool{PoolMutable: *param}
	obj := &Pool{}
	obj.FromModel(mObj)
	if err := op.crud.UpdateOne(ctx, op, obj, ua); err != nil {
		return nil, err
	}
	return obj.ToModel(), nil
}
