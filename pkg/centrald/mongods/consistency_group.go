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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/consistency_group"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/op/go-logging"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// ConsistencyGroup object
type ConsistencyGroup struct {
	ObjMeta                  `bson:",inline"`
	Name                     string
	Description              string
	AccountID                string
	TenantAccountID          string
	ApplicationGroupIds      ObjIDList
	SnapshotManagementPolicy SnapshotManagementPolicy
	Tags                     StringList
	SystemTags               StringList
}

// ToModel converts a datastore object to a model object
func (o *ConsistencyGroup) ToModel() *M.ConsistencyGroup {
	if o.ApplicationGroupIds == nil {
		o.ApplicationGroupIds = make(ObjIDList, 0)
	}
	if o.Tags == nil {
		o.Tags = make(StringList, 0)
	}
	if o.SystemTags == nil {
		o.SystemTags = make(StringList, 0)
	}
	mObj := &M.ConsistencyGroup{
		ConsistencyGroupAllOf0: M.ConsistencyGroupAllOf0{Meta: (&o.ObjMeta).ToModel("ConsistencyGroup")},
	}
	mObj.Name = M.ObjName(o.Name)
	mObj.Description = M.ObjDescription(o.Description)
	mObj.AccountID = M.ObjIDMutable(o.AccountID)
	mObj.TenantAccountID = M.ObjIDMutable(o.TenantAccountID)
	mObj.ApplicationGroupIds = *o.ApplicationGroupIds.ToModel()
	mObj.SnapshotManagementPolicy = o.SnapshotManagementPolicy.ToModel()
	mObj.Tags = *o.Tags.ToModel()
	mObj.SystemTags = *o.SystemTags.ToModel()
	return mObj
}

// FromModel converts a model object to a datastore object
func (o *ConsistencyGroup) FromModel(mObj *M.ConsistencyGroup) {
	mMeta := mObj.Meta
	if mMeta == nil {
		mMeta = &M.ObjMeta{ObjType: "ConsistencyGroup"}
	}
	(&o.ObjMeta).FromModel(mMeta)
	o.Name = string(mObj.Name)
	o.Description = string(mObj.Description)
	o.AccountID = string(mObj.AccountID)
	o.TenantAccountID = string(mObj.TenantAccountID)
	if o.ApplicationGroupIds == nil {
		o.ApplicationGroupIds = make(ObjIDList, len(mObj.ApplicationGroupIds))
	}
	o.ApplicationGroupIds.FromModel(&mObj.ApplicationGroupIds)
	o.SnapshotManagementPolicy.FromModel(mObj.SnapshotManagementPolicy)
	if o.Tags == nil {
		o.Tags = make(StringList, len(mObj.Tags))
	}
	o.Tags.FromModel(&mObj.Tags)
	if o.SystemTags == nil {
		o.SystemTags = make(StringList, len(mObj.SystemTags))
	}
	o.SystemTags.FromModel(&mObj.SystemTags)
}

// consistencyGroupHandler is an ObjectDocumentHandler that implements the ds.ConsistencyGroupOps operations
type consistencyGroupHandler struct {
	cName   string
	api     DBAPI
	crud    ObjectDocumentHandlerCRUD
	log     *logging.Logger
	indexes []mongo.IndexModel
}

var _ = ds.ConsistencyGroupOps(&consistencyGroupHandler{})
var _ = ObjectDocumentHandler(&consistencyGroupHandler{})

var odhConsistencyGroup = &consistencyGroupHandler{
	cName: "consistencygroup",
	indexes: []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: accountIDKey, Value: IndexAscending}, {Key: nameKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: applicationGroupIdsKey, Value: IndexAscending}}},
		{Keys: bsonx.Doc{{Key: tenantAccountIDKey, Value: IndexAscending}}},
	},
}

func init() {
	odhRegister(odhConsistencyGroup.cName, odhConsistencyGroup)
}

// convertListParams is a helper to convert ConsistencyGroupListParams to bson.M
func (op *consistencyGroupHandler) convertListParams(params consistency_group.ConsistencyGroupListParams) bson.M {
	qParams := bson.M{}
	orParams := newAndOrList()
	if params.AccountID != nil && *params.AccountID != "" {
		if params.TenantAccountID != nil && *params.TenantAccountID != "" {
			orParams.append(
				bson.M{accountIDKey: *params.AccountID},
				bson.M{tenantAccountIDKey: *params.TenantAccountID},
			)
		} else {
			qParams[accountIDKey] = *params.AccountID
		}
	} else if params.TenantAccountID != nil && *params.TenantAccountID != "" {
		qParams[tenantAccountIDKey] = *params.TenantAccountID
	}
	if params.Name != nil && *params.Name != "" {
		qParams[nameKey] = *params.Name
	}
	if params.ApplicationGroupID != nil && *params.ApplicationGroupID != "" {
		qParams[applicationGroupIdsKey] = *params.ApplicationGroupID
	}
	if params.Tags != nil {
		qParams["tags"] = bson.M{"$all": params.Tags}
	}
	if params.SystemTags != nil {
		qParams["systemtags"] = bson.M{"$all": params.SystemTags}
	}
	orParams.setQueryParam(qParams)
	op.log.Info("qParams:", qParams)
	return qParams
}

// objectDocumentHandler methods

func (op *consistencyGroupHandler) Claim(api DBAPI, crud ObjectDocumentHandlerCRUD) {
	op.api = api
	op.crud = crud
	op.log = api.Logger()
}

func (op *consistencyGroupHandler) Initialize(ctx context.Context) error {
	return op.crud.CreateIndexes(ctx, op)
}

func (op *consistencyGroupHandler) Start(ctx context.Context) error {
	dbn := op.api.DBName()
	op.log.Infof("Starting collection %s.%s", dbn, op.cName)
	return nil
}

func (op *consistencyGroupHandler) Ops() interface{} {
	return op
}

func (op *consistencyGroupHandler) CName() string {
	return op.cName
}

func (op *consistencyGroupHandler) Indexes() []mongo.IndexModel {
	return op.indexes
}

func (op *consistencyGroupHandler) NewObject() interface{} {
	return &ConsistencyGroup{}
}

// ds.ConsistencyGroupOps methods

// Count the number of matching documents, limit is applied if non-zero
func (op *consistencyGroupHandler) Count(ctx context.Context, params consistency_group.ConsistencyGroupListParams, limit uint) (int, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Count:", err)
		return 0, err
	}
	return op.crud.Count(ctx, op, op.convertListParams(params), limit)
}

func (op *consistencyGroupHandler) Create(ctx context.Context, mObj *M.ConsistencyGroup) (*M.ConsistencyGroup, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Create:", err)
		return nil, err
	}
	mObj.Meta = nil // ensure that meta will be put into correct creation state by FromModel
	dObj := &ConsistencyGroup{}
	dObj.FromModel(mObj)
	if err := op.crud.InsertOne(ctx, op, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *consistencyGroupHandler) Delete(ctx context.Context, mID string) error {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Delete:", err)
		return err
	}
	return op.crud.DeleteOne(ctx, op, mID)
}

func (op *consistencyGroupHandler) Fetch(ctx context.Context, mID string) (*M.ConsistencyGroup, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Fetch:", err)
		return nil, err
	}
	dObj := &ConsistencyGroup{}
	if err := op.crud.FindOne(ctx, op, bson.M{objKey: mID}, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *consistencyGroupHandler) List(ctx context.Context, params consistency_group.ConsistencyGroupListParams) ([]*M.ConsistencyGroup, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("List:", err)
		return nil, err
	}
	list := make([]*M.ConsistencyGroup, 0, 10)
	err := op.crud.FindAll(ctx, op, params, op.convertListParams(params), func(obj interface{}) {
		dObj := obj.(*ConsistencyGroup) // or panic
		list = append(list, dObj.ToModel())
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (op *consistencyGroupHandler) Update(ctx context.Context, ua *ds.UpdateArgs, param *M.ConsistencyGroupMutable) (*M.ConsistencyGroup, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Update:", err)
		return nil, err
	}
	mObj := &M.ConsistencyGroup{ConsistencyGroupMutable: *param}
	obj := &ConsistencyGroup{}
	obj.FromModel(mObj)
	if err := op.crud.UpdateOne(ctx, op, obj, ua); err != nil {
		return nil, err
	}
	return obj.ToModel(), nil
}
