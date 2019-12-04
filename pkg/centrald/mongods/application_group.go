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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/application_group"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/op/go-logging"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// ApplicationGroup object
type ApplicationGroup struct {
	ObjMeta         `bson:",inline"`
	Name            string
	Description     string
	AccountID       string
	TenantAccountID string
	Tags            StringList
	SystemTags      StringList
}

// ToModel converts a datastore object to a model object
func (o *ApplicationGroup) ToModel() *M.ApplicationGroup {
	if o.Tags == nil {
		o.Tags = make(StringList, 0)
	}
	if o.SystemTags == nil {
		o.SystemTags = make(StringList, 0)
	}
	mObj := &M.ApplicationGroup{
		ApplicationGroupAllOf0: M.ApplicationGroupAllOf0{Meta: (&o.ObjMeta).ToModel("ApplicationGroup")},
	}
	mObj.Name = M.ObjName(o.Name)
	mObj.Description = M.ObjDescription(o.Description)
	mObj.AccountID = M.ObjIDMutable(o.AccountID)
	mObj.TenantAccountID = M.ObjIDMutable(o.TenantAccountID)
	mObj.Tags = *o.Tags.ToModel()
	mObj.SystemTags = *o.SystemTags.ToModel()
	return mObj
}

// FromModel converts a model object to a datastore object
func (o *ApplicationGroup) FromModel(mObj *M.ApplicationGroup) {
	mMeta := mObj.Meta
	if mMeta == nil {
		mMeta = &M.ObjMeta{ObjType: "ApplicationGroup"}
	}
	(&o.ObjMeta).FromModel(mMeta)
	o.Name = string(mObj.Name)
	o.Description = string(mObj.Description)
	o.AccountID = string(mObj.AccountID)
	o.TenantAccountID = string(mObj.TenantAccountID)
	if o.Tags == nil {
		o.Tags = make(StringList, len(mObj.Tags))
	}
	o.Tags.FromModel(&mObj.Tags)
	if o.SystemTags == nil {
		o.SystemTags = make(StringList, len(mObj.SystemTags))
	}
	o.SystemTags.FromModel(&mObj.SystemTags)
}

// applicationGroupHandler is an ObjectDocumentHandler that implements the ds.ApplicationGroupOps operations
type applicationGroupHandler struct {
	cName   string
	api     DBAPI
	crud    ObjectDocumentHandlerCRUD
	log     *logging.Logger
	indexes []mongo.IndexModel
}

var _ = ds.ApplicationGroupOps(&applicationGroupHandler{})
var _ = ObjectDocumentHandler(&applicationGroupHandler{})

var odhApplicationGroup = &applicationGroupHandler{
	cName: "applicationgroup",
	indexes: []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: accountIDKey, Value: IndexAscending}, {Key: nameKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: tenantAccountIDKey, Value: IndexAscending}}},
	},
}

func init() {
	odhRegister(odhApplicationGroup.cName, odhApplicationGroup)
}

// convertListParams is a helper to convert ApplicationGroupListParams to bson.M
func (op *applicationGroupHandler) convertListParams(params application_group.ApplicationGroupListParams) bson.M {
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

func (op *applicationGroupHandler) Claim(api DBAPI, crud ObjectDocumentHandlerCRUD) {
	op.api = api
	op.crud = crud
	op.log = api.Logger()
}

func (op *applicationGroupHandler) Initialize(ctx context.Context) error {
	return op.crud.CreateIndexes(ctx, op)
}

func (op *applicationGroupHandler) Start(ctx context.Context) error {
	dbn := op.api.DBName()
	op.log.Infof("Starting collection %s.%s", dbn, op.cName)
	return nil
}

func (op *applicationGroupHandler) Ops() interface{} {
	return op
}

func (op *applicationGroupHandler) CName() string {
	return op.cName
}

func (op *applicationGroupHandler) Indexes() []mongo.IndexModel {
	return op.indexes
}

func (op *applicationGroupHandler) NewObject() interface{} {
	return &ApplicationGroup{}
}

// ds.ApplicationGroupOps methods

// Count the number of matching documents, limit is applied if non-zero
func (op *applicationGroupHandler) Count(ctx context.Context, params application_group.ApplicationGroupListParams, limit uint) (int, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Count:", err)
		return 0, err
	}
	return op.crud.Count(ctx, op, op.convertListParams(params), limit)
}

func (op *applicationGroupHandler) Create(ctx context.Context, mObj *M.ApplicationGroup) (*M.ApplicationGroup, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Create:", err)
		return nil, err
	}
	mObj.Meta = nil // ensure that meta will be put into correct creation state by FromModel
	dObj := &ApplicationGroup{}
	dObj.FromModel(mObj)
	if err := op.crud.InsertOne(ctx, op, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *applicationGroupHandler) Delete(ctx context.Context, mID string) error {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Delete:", err)
		return err
	}
	return op.crud.DeleteOne(ctx, op, mID)
}

func (op *applicationGroupHandler) Fetch(ctx context.Context, mID string) (*M.ApplicationGroup, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Fetch:", err)
		return nil, err
	}
	dObj := &ApplicationGroup{}
	if err := op.crud.FindOne(ctx, op, bson.M{objKey: mID}, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *applicationGroupHandler) List(ctx context.Context, params application_group.ApplicationGroupListParams) ([]*M.ApplicationGroup, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("List:", err)
		return nil, err
	}
	list := make([]*M.ApplicationGroup, 0, 10)
	err := op.crud.FindAll(ctx, op, params, op.convertListParams(params), func(obj interface{}) {
		mObj := obj.(*ApplicationGroup).ToModel() // or panic
		list = append(list, mObj)
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (op *applicationGroupHandler) Update(ctx context.Context, ua *ds.UpdateArgs, param *M.ApplicationGroupMutable) (*M.ApplicationGroup, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Update:", err)
		return nil, err
	}
	mObj := &M.ApplicationGroup{ApplicationGroupMutable: *param}
	obj := &ApplicationGroup{}
	obj.FromModel(mObj)
	if err := op.crud.UpdateOne(ctx, op, obj, ua); err != nil {
		return nil, err
	}
	return obj.ToModel(), nil
}
