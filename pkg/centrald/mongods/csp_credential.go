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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/csp_credential"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/op/go-logging"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// CspCredential object
type CspCredential struct {
	ObjMeta              `bson:",inline"`
	AccountID            string
	CspDomainType        string
	Name                 string
	Description          string
	CredentialAttributes StringValueMap
	Tags                 StringList
}

// ToModel converts a datastore object to a model object
func (o *CspCredential) ToModel() *M.CSPCredential {
	if o.CredentialAttributes == nil {
		o.CredentialAttributes = make(StringValueMap, 0)
	}
	if o.Tags == nil {
		o.Tags = make(StringList, 0)
	}
	return &M.CSPCredential{
		CSPCredentialAllOf0: M.CSPCredentialAllOf0{
			CspDomainType: M.CspDomainTypeMutable(o.CspDomainType),
			AccountID:     M.ObjIDMutable(o.AccountID),
			Meta:          (&o.ObjMeta).ToModel("CspCredential"),
		},
		CSPCredentialMutable: M.CSPCredentialMutable{
			Name:                 M.ObjName(o.Name),
			Description:          M.ObjDescription(o.Description),
			Tags:                 *o.Tags.ToModel(),
			CredentialAttributes: o.CredentialAttributes.ToModel(),
		},
	}
}

// FromModel converts a model object to a datastore object
func (o *CspCredential) FromModel(mObj *M.CSPCredential) {
	mMeta := mObj.Meta
	if mMeta == nil {
		mMeta = &M.ObjMeta{ObjType: "CspCredential"}
	}
	(&o.ObjMeta).FromModel(mMeta)
	o.CspDomainType = string(mObj.CspDomainType)
	o.AccountID = string(mObj.AccountID)
	o.Name = string(mObj.Name)
	o.Description = string(mObj.Description)
	if o.Tags == nil {
		o.Tags = make(StringList, len(mObj.Tags))
	}
	o.Tags.FromModel(&mObj.Tags)
	if o.CredentialAttributes == nil {
		o.CredentialAttributes = make(StringValueMap, len(mObj.CredentialAttributes))
	}
	o.CredentialAttributes.FromModel(mObj.CredentialAttributes)
}

// cspCredentialHandler is an ObjectDocumentHandler that implements the ds.CspCredentialOps operations
type cspCredentialHandler struct {
	cName   string
	api     DBAPI
	crud    ObjectDocumentHandlerCRUD
	log     *logging.Logger
	indexes []mongo.IndexModel
}

var _ = ds.CspCredentialOps(&cspCredentialHandler{})
var _ = ObjectDocumentHandler(&cspCredentialHandler{})

var odhCspCredential = &cspCredentialHandler{
	cName: "cspcredential",
	indexes: []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: accountIDKey, Value: IndexAscending}, {Key: nameKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
	},
}

func init() {
	odhRegister(odhCspCredential.cName, odhCspCredential)
}

// convertListParams is a helper to convert CspCredentialListParams to bson.M
func (op *cspCredentialHandler) convertListParams(params csp_credential.CspCredentialListParams) bson.M {
	qParams := bson.M{}
	if params.AccountID != nil && *params.AccountID != "" {
		qParams[accountIDKey] = *params.AccountID
	}
	if params.Name != nil && *params.Name != "" {
		qParams[nameKey] = *params.Name
	}
	if params.Tags != nil {
		qParams["tags"] = bson.M{"$all": params.Tags}
	}
	if params.CspDomainType != nil && *params.CspDomainType != "" {
		qParams["cspdomaintype"] = *params.CspDomainType
	}
	op.log.Info("qParams:", qParams)
	return qParams
}

// objectDocumentHandler methods

func (op *cspCredentialHandler) Claim(api DBAPI, crud ObjectDocumentHandlerCRUD) {
	op.api = api
	op.crud = crud
	op.log = api.Logger()
}

func (op *cspCredentialHandler) Initialize(ctx context.Context) error {
	return op.crud.CreateIndexes(ctx, op)
}

func (op *cspCredentialHandler) Start(ctx context.Context) error {
	dbn := op.api.DBName()
	op.log.Infof("Starting collection %s.%s", dbn, op.cName)
	return nil
}

func (op *cspCredentialHandler) Ops() interface{} {
	return op
}

func (op *cspCredentialHandler) CName() string {
	return op.cName
}

func (op *cspCredentialHandler) Indexes() []mongo.IndexModel {
	return op.indexes
}

func (op *cspCredentialHandler) NewObject() interface{} {
	return &CspCredential{}
}

// ds.CspCredentialOps methods

// Count the number of matching documents, limit is applied if non-zero
func (op *cspCredentialHandler) Count(ctx context.Context, params csp_credential.CspCredentialListParams, limit uint) (int, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Count:", err)
		return 0, err
	}
	return op.crud.Count(ctx, op, op.convertListParams(params), limit)
}

func (op *cspCredentialHandler) Create(ctx context.Context, mObj *M.CSPCredential) (*M.CSPCredential, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Create:", err)
		return nil, err
	}
	mObj.Meta = nil // ensure that meta will be put into correct creation state by FromModel
	dObj := &CspCredential{}
	dObj.FromModel(mObj)
	if err := op.crud.InsertOne(ctx, op, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *cspCredentialHandler) Delete(ctx context.Context, mID string) error {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Delete:", err)
		return err
	}
	return op.crud.DeleteOne(ctx, op, mID)
}

func (op *cspCredentialHandler) Fetch(ctx context.Context, mID string) (*M.CSPCredential, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Fetch:", err)
		return nil, err
	}
	dObj := &CspCredential{}
	if err := op.crud.FindOne(ctx, op, bson.M{objKey: mID}, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *cspCredentialHandler) List(ctx context.Context, params csp_credential.CspCredentialListParams) ([]*M.CSPCredential, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("List:", err)
		return nil, err
	}
	list := make([]*M.CSPCredential, 0, 10)
	err := op.crud.FindAll(ctx, op, params, op.convertListParams(params), func(obj interface{}) {
		dObj := obj.(*CspCredential) // or panic
		list = append(list, dObj.ToModel())
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (op *cspCredentialHandler) Update(ctx context.Context, ua *ds.UpdateArgs, param *M.CSPCredentialMutable) (*M.CSPCredential, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Update:", err)
		return nil, err
	}
	mObj := &M.CSPCredential{CSPCredentialMutable: *param}
	obj := &CspCredential{}
	obj.FromModel(mObj)
	if err := op.crud.UpdateOne(ctx, op, obj, ua); err != nil {
		return nil, err
	}
	return obj.ToModel(), nil
}
