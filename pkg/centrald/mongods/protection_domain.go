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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/protection_domain"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/op/go-logging"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// ProtectionDomain object
type ProtectionDomain struct {
	ObjMeta              `bson:",inline"`
	AccountID            string
	EncryptionAlgorithm  string
	EncryptionPassphrase string
	Name                 string
	Description          string
	SystemTags           StringList
	Tags                 StringList
}

// ToModel converts a datastore object to a model object
func (o *ProtectionDomain) ToModel() *M.ProtectionDomain {
	if o.SystemTags == nil {
		o.SystemTags = make(StringList, 0)
	}
	if o.Tags == nil {
		o.Tags = make(StringList, 0)
	}
	return &M.ProtectionDomain{
		ProtectionDomainAllOf0: M.ProtectionDomainAllOf0{Meta: (&o.ObjMeta).ToModel("ProtectionDomain")},
		ProtectionDomainCreateOnce: M.ProtectionDomainCreateOnce{
			AccountID:            M.ObjIDMutable(o.AccountID),
			EncryptionAlgorithm:  o.EncryptionAlgorithm,
			EncryptionPassphrase: &M.ValueType{Kind: common.ValueTypeSecret, Value: o.EncryptionPassphrase},
		},
		ProtectionDomainMutable: M.ProtectionDomainMutable{
			Name:        M.ObjName(o.Name),
			Description: M.ObjDescription(o.Description),
			SystemTags:  *o.SystemTags.ToModel(),
			Tags:        *o.Tags.ToModel(),
		},
	}
}

// FromModel converts a model object to a datastore object
func (o *ProtectionDomain) FromModel(mObj *M.ProtectionDomain) {
	mMeta := mObj.Meta
	if mMeta == nil {
		mMeta = &M.ObjMeta{ObjType: "ProtectionDomain"}
	}
	(&o.ObjMeta).FromModel(mMeta)
	o.AccountID = string(mObj.AccountID)
	o.EncryptionAlgorithm = mObj.EncryptionAlgorithm
	if mObj.EncryptionPassphrase == nil {
		mObj.EncryptionPassphrase = &M.ValueType{Kind: common.ValueTypeSecret}
	}
	o.EncryptionPassphrase = mObj.EncryptionPassphrase.Value
	o.Name = string(mObj.Name)
	o.Description = string(mObj.Description)
	if o.SystemTags == nil {
		o.SystemTags = make(StringList, len(mObj.SystemTags))
	}
	o.SystemTags.FromModel(&mObj.SystemTags)
	if o.Tags == nil {
		o.Tags = make(StringList, len(mObj.Tags))
	}
	o.Tags.FromModel(&mObj.Tags)
}

// protectionDomainHandler is an ObjectDocumentHandler that implements the ds.ProtectionDomainOps operations
type protectionDomainHandler struct {
	cName   string
	api     DBAPI
	crud    ObjectDocumentHandlerCRUD
	log     *logging.Logger
	indexes []mongo.IndexModel
}

var _ = ds.ProtectionDomainOps(&protectionDomainHandler{})
var _ = ObjectDocumentHandler(&protectionDomainHandler{})

var odhProtectionDomain = &protectionDomainHandler{
	cName: "protectionDomain",
	indexes: []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: accountIDKey, Value: IndexAscending}, {Key: nameKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
	},
}

func init() {
	odhRegister(odhProtectionDomain.cName, odhProtectionDomain)
}

// convertListParams is a helper to convert ProtectionDomainListParams to bson.M
func (op *protectionDomainHandler) convertListParams(params protection_domain.ProtectionDomainListParams) bson.M {
	qParams := bson.M{}
	if params.AccountID != nil {
		qParams["accountid"] = *params.AccountID
	}
	if params.NamePattern != nil {
		qParams[nameKey] = primitive.Regex{Pattern: *params.NamePattern}
	}
	if params.Name != nil && *params.Name != "" {
		qParams[nameKey] = *params.Name
	}
	if params.SystemTags != nil {
		qParams["systemtags"] = bson.M{"$all": params.SystemTags}
	}
	if params.Tags != nil {
		qParams["tags"] = bson.M{"$all": params.Tags}
	}
	op.log.Info("qParams:", qParams)
	return qParams
}

// objectDocumentHandler methods

func (op *protectionDomainHandler) Claim(api DBAPI, crud ObjectDocumentHandlerCRUD) {
	op.api = api
	op.crud = crud
	op.log = api.Logger()
}

func (op *protectionDomainHandler) Initialize(ctx context.Context) error {
	return op.crud.CreateIndexes(ctx, op)
}

func (op *protectionDomainHandler) Start(ctx context.Context) error {
	dbn := op.api.DBName()
	op.log.Infof("Starting collection %s.%s", dbn, op.cName)
	return nil
}

func (op *protectionDomainHandler) Ops() interface{} {
	return op
}

func (op *protectionDomainHandler) CName() string {
	return op.cName
}

func (op *protectionDomainHandler) Indexes() []mongo.IndexModel {
	return op.indexes
}

func (op *protectionDomainHandler) NewObject() interface{} {
	return &ProtectionDomain{}
}

// ds.ProtectionDomainOps methods

func (op *protectionDomainHandler) Count(ctx context.Context, params protection_domain.ProtectionDomainListParams, limit uint) (int, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Count:", err)
		return 0, err
	}
	return op.crud.Count(ctx, op, op.convertListParams(params), limit)
}

func (op *protectionDomainHandler) Create(ctx context.Context, mObj *M.ProtectionDomainCreateArgs) (*M.ProtectionDomain, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Create:", err)
		return nil, err
	}
	dObj := &ProtectionDomain{}
	mProtectionDomain := &M.ProtectionDomain{
		ProtectionDomainCreateOnce: mObj.ProtectionDomainCreateOnce,
		ProtectionDomainMutable:    mObj.ProtectionDomainMutable,
	}
	dObj.FromModel(mProtectionDomain)
	if err := op.crud.InsertOne(ctx, op, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *protectionDomainHandler) Delete(ctx context.Context, mID string) error {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Delete:", err)
		return err
	}
	return op.crud.DeleteOne(ctx, op, mID)
}

func (op *protectionDomainHandler) Fetch(ctx context.Context, mID string) (*M.ProtectionDomain, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Fetch:", err)
		return nil, err
	}
	dObj := &ProtectionDomain{}
	if err := op.crud.FindOne(ctx, op, bson.M{objKey: mID}, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *protectionDomainHandler) List(ctx context.Context, params protection_domain.ProtectionDomainListParams) ([]*M.ProtectionDomain, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("List:", err)
		return nil, err
	}
	list := make([]*M.ProtectionDomain, 0, 10)
	err := op.crud.FindAll(ctx, op, params, op.convertListParams(params), func(obj interface{}) {
		dObj := obj.(*ProtectionDomain) // or panic
		list = append(list, dObj.ToModel())
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (op *protectionDomainHandler) Update(ctx context.Context, ua *ds.UpdateArgs, param *M.ProtectionDomainMutable) (*M.ProtectionDomain, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Update:", err)
		return nil, err
	}
	mObj := &M.ProtectionDomain{ProtectionDomainMutable: *param}
	obj := &ProtectionDomain{}
	obj.FromModel(mObj)
	if err := op.crud.UpdateOne(ctx, op, obj, ua); err != nil {
		return nil, err
	}
	return obj.ToModel(), nil
}
