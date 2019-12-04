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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/csp_domain"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/op/go-logging"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// CspDomain object
type CspDomain struct {
	ObjMeta             `bson:",inline"`
	CspDomainType       string
	AccountID           string
	Name                string
	Description         string
	ManagementHost      string
	Tags                StringList
	CspDomainAttributes StringValueMap
	AuthorizedAccounts  ObjIDList
	ClusterUsagePolicy  ClusterUsagePolicy
	CspCredentialID     string
	StorageCosts        StorageCostMap
}

// ToModel converts a datastore object to a model object
func (o *CspDomain) ToModel() *M.CSPDomain {
	if o.CspDomainAttributes == nil {
		o.CspDomainAttributes = make(StringValueMap, 0)
	}
	if o.Tags == nil {
		o.Tags = make(StringList, 0)
	}
	if o.AuthorizedAccounts == nil {
		o.AuthorizedAccounts = make(ObjIDList, 0)
	}
	return &M.CSPDomain{
		CSPDomainAllOf0: M.CSPDomainAllOf0{
			CspDomainType:       M.CspDomainTypeMutable(o.CspDomainType),
			AccountID:           M.ObjIDMutable(o.AccountID),
			Meta:                (&o.ObjMeta).ToModel("CspDomain"),
			CspDomainAttributes: o.CspDomainAttributes.ToModel(),
		},
		CSPDomainMutable: M.CSPDomainMutable{
			Name:               M.ObjName(o.Name),
			Description:        M.ObjDescription(o.Description),
			ManagementHost:     o.ManagementHost,
			Tags:               *o.Tags.ToModel(),
			AuthorizedAccounts: *o.AuthorizedAccounts.ToModel(),
			ClusterUsagePolicy: o.ClusterUsagePolicy.ToModel(),
			CspCredentialID:    M.ObjIDMutable(o.CspCredentialID),
			StorageCosts:       o.StorageCosts.ToModel(),
		},
	}
}

// FromModel converts a model object to a datastore object
func (o *CspDomain) FromModel(mObj *M.CSPDomain) {
	mMeta := mObj.Meta
	if mMeta == nil {
		mMeta = &M.ObjMeta{ObjType: "CspDomain"}
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
	o.ManagementHost = mObj.ManagementHost
	if o.CspDomainAttributes == nil {
		o.CspDomainAttributes = make(StringValueMap, len(mObj.CspDomainAttributes))
	}
	o.CspDomainAttributes.FromModel(mObj.CspDomainAttributes)
	if o.AuthorizedAccounts == nil {
		o.AuthorizedAccounts = make(ObjIDList, len(mObj.AuthorizedAccounts))
	}
	o.AuthorizedAccounts.FromModel(&mObj.AuthorizedAccounts)
	o.ClusterUsagePolicy.FromModel(mObj.ClusterUsagePolicy)
	o.CspCredentialID = string(mObj.CspCredentialID)
	if o.StorageCosts == nil {
		o.StorageCosts = make(StorageCostMap, len(mObj.StorageCosts))
	}
	o.StorageCosts.FromModel(mObj.StorageCosts)
}

// cspDomainHandler is an ObjectDocumentHandler that implements the ds.CspDomainOps operations
type cspDomainHandler struct {
	cName   string
	api     DBAPI
	crud    ObjectDocumentHandlerCRUD
	log     *logging.Logger
	indexes []mongo.IndexModel
}

var _ = ds.CspDomainOps(&cspDomainHandler{})
var _ = ObjectDocumentHandler(&cspDomainHandler{})

var odhCspDomain = &cspDomainHandler{
	cName: "cspdomain",
	indexes: []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: accountIDKey, Value: IndexAscending}, {Key: nameKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
	},
}

func init() {
	odhRegister(odhCspDomain.cName, odhCspDomain)
}

// convertListParams is a helper to convert CspDomainListParams to bson.M
func (op *cspDomainHandler) convertListParams(params csp_domain.CspDomainListParams) bson.M {
	qParams := bson.M{}
	if params.AuthorizedAccountID != nil && *params.AuthorizedAccountID != "" {
		qParams["authorizedaccounts"] = *params.AuthorizedAccountID
	}
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
	if params.CspCredentialID != nil && *params.CspCredentialID != "" {
		qParams["cspcredentialid"] = *params.CspCredentialID
	}
	op.log.Info("qParams:", qParams)
	return qParams
}

// objectDocumentHandler methods

func (op *cspDomainHandler) Claim(api DBAPI, crud ObjectDocumentHandlerCRUD) {
	op.api = api
	op.crud = crud
	op.log = api.Logger()
}

func (op *cspDomainHandler) Initialize(ctx context.Context) error {
	return op.crud.CreateIndexes(ctx, op)
}

func (op *cspDomainHandler) Start(ctx context.Context) error {
	dbn := op.api.DBName()
	op.log.Infof("Starting collection %s.%s", dbn, op.cName)
	return nil
}

func (op *cspDomainHandler) Ops() interface{} {
	return op
}

func (op *cspDomainHandler) CName() string {
	return op.cName
}

func (op *cspDomainHandler) Indexes() []mongo.IndexModel {
	return op.indexes
}

func (op *cspDomainHandler) NewObject() interface{} {
	return &CspDomain{}
}

// ds.CspDomainOps methods

// Count the number of matching documents, limit is applied if non-zero
func (op *cspDomainHandler) Count(ctx context.Context, params csp_domain.CspDomainListParams, limit uint) (int, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Count:", err)
		return 0, err
	}
	return op.crud.Count(ctx, op, op.convertListParams(params), limit)
}

func (op *cspDomainHandler) Create(ctx context.Context, mObj *M.CSPDomain) (*M.CSPDomain, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Create:", err)
		return nil, err
	}
	mObj.Meta = nil // ensure that meta will be put into correct creation state by FromModel
	dObj := &CspDomain{}
	dObj.FromModel(mObj)
	if err := op.crud.InsertOne(ctx, op, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *cspDomainHandler) Delete(ctx context.Context, mID string) error {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Delete:", err)
		return err
	}
	return op.crud.DeleteOne(ctx, op, mID)
}

func (op *cspDomainHandler) Fetch(ctx context.Context, mID string) (*M.CSPDomain, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Fetch:", err)
		return nil, err
	}
	dObj := &CspDomain{}
	if err := op.crud.FindOne(ctx, op, bson.M{objKey: mID}, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *cspDomainHandler) List(ctx context.Context, params csp_domain.CspDomainListParams) ([]*M.CSPDomain, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("List:", err)
		return nil, err
	}
	list := make([]*M.CSPDomain, 0, 10)
	err := op.crud.FindAll(ctx, op, params, op.convertListParams(params), func(obj interface{}) {
		dObj := obj.(*CspDomain) // or panic
		list = append(list, dObj.ToModel())
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (op *cspDomainHandler) Update(ctx context.Context, ua *ds.UpdateArgs, param *M.CSPDomainMutable) (*M.CSPDomain, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Update:", err)
		return nil, err
	}
	mObj := &M.CSPDomain{CSPDomainMutable: *param}
	obj := &CspDomain{}
	obj.FromModel(mObj)
	if err := op.crud.UpdateOne(ctx, op, obj, ua); err != nil {
		return nil, err
	}
	return obj.ToModel(), nil
}
