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
	"sync"
	"time"

	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/cluster"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/go-openapi/swag"
	"github.com/op/go-logging"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// Cluster object
type Cluster struct {
	ObjMeta            `bson:",inline"`
	Name               string
	Description        string
	Tags               StringList
	ClusterAttributes  StringValueMap
	ClusterVersion     string
	Service            NuvoService
	ClusterIdentifier  string
	ClusterType        string
	CspDomainID        string
	AccountID          string
	AuthorizedAccounts ObjIDList
	ClusterUsagePolicy ClusterUsagePolicy
	State              string
	Messages           []TimestampedString
}

// ToModel converts a datastore object to a model object
func (o *Cluster) ToModel() *M.Cluster {
	if o.Tags == nil {
		o.Tags = make(StringList, 0)
	}
	if o.ClusterAttributes == nil {
		o.ClusterAttributes = make(StringValueMap, 0)
	}
	if o.AuthorizedAccounts == nil {
		o.AuthorizedAccounts = make(ObjIDList, 0)
	}
	mObj := &M.Cluster{
		ClusterAllOf0: M.ClusterAllOf0{Meta: (&o.ObjMeta).ToModel("Cluster")},
	}
	mObj.AccountID = M.ObjID(o.AccountID)
	mObj.AuthorizedAccounts = *o.AuthorizedAccounts.ToModel()
	mObj.ClusterIdentifier = o.ClusterIdentifier
	mObj.ClusterType = o.ClusterType
	mObj.CspDomainID = M.ObjIDMutable(o.CspDomainID)
	mObj.Name = M.ObjName(o.Name)
	mObj.Description = M.ObjDescription(o.Description)
	mObj.Tags = *o.Tags.ToModel()
	mObj.ClusterAttributes = o.ClusterAttributes.ToModel()
	mObj.ClusterVersion = o.ClusterVersion
	mObj.Service = o.Service.ToModel()
	mObj.ClusterUsagePolicy = o.ClusterUsagePolicy.ToModel()
	mObj.State = o.State
	if o.Messages == nil {
		o.Messages = make([]TimestampedString, 0)
	}
	mObj.Messages = make([]*M.TimestampedString, len(o.Messages))
	for i, msg := range o.Messages {
		mObj.Messages[i] = msg.ToModel()
	}
	return mObj
}

// FromModel converts a model object to a datastore object
func (o *Cluster) FromModel(mObj *M.Cluster) {
	mMeta := mObj.Meta
	if mMeta == nil {
		mMeta = &M.ObjMeta{ObjType: "Cluster"}
	}
	(&o.ObjMeta).FromModel(mMeta)
	o.Name = string(mObj.Name)
	o.Description = string(mObj.Description)
	if o.Tags == nil {
		o.Tags = make(StringList, len(mObj.Tags))
	}
	o.Tags.FromModel(&mObj.Tags)
	if o.ClusterAttributes == nil {
		o.ClusterAttributes = make(StringValueMap, len(mObj.ClusterAttributes))
	}
	o.ClusterAttributes.FromModel(mObj.ClusterAttributes)
	o.ClusterVersion = mObj.ClusterVersion
	ncc := mObj.Service
	if ncc == nil {
		ncc = &M.NuvoService{}
	}
	o.Service.FromModel(ncc)
	o.ClusterIdentifier = mObj.ClusterIdentifier
	o.ClusterType = mObj.ClusterType
	o.CspDomainID = string(mObj.CspDomainID)
	o.AccountID = string(mObj.AccountID)
	if o.AuthorizedAccounts == nil {
		o.AuthorizedAccounts = make(ObjIDList, len(mObj.AuthorizedAccounts))
	}
	o.AuthorizedAccounts.FromModel(&mObj.AuthorizedAccounts)
	o.ClusterUsagePolicy.FromModel(mObj.ClusterUsagePolicy)
	o.State = mObj.State
	if o.Messages == nil {
		o.Messages = make([]TimestampedString, len(mObj.Messages))
	}
	for i, msg := range mObj.Messages {
		o.Messages[i].FromModel(msg)
	}
}

// clusterHandler is an ObjectDocumentHandler that implements the ds.ClusterOps operations
type clusterHandler struct {
	cName   string
	api     DBAPI
	crud    ObjectDocumentHandlerCRUD
	log     *logging.Logger
	indexes []mongo.IndexModel
	mux     sync.Mutex
	cache   map[string]*Cluster
	// TBD limit the size of the cache
}

var _ = ds.ClusterOps(&clusterHandler{})
var _ = ObjectDocumentHandler(&clusterHandler{})

var odhCluster = &clusterHandler{
	cName: "cluster",
	indexes: []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: cspDomainIDKey, Value: IndexAscending}, {Key: nameKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
	},
	cache: map[string]*Cluster{},
}

func init() {
	odhRegister(odhCluster.cName, odhCluster)
}

// convertListParams is a helper to convert ClusterListParams to bson.M
func (op *clusterHandler) convertListParams(params cluster.ClusterListParams) bson.M {
	qParams := bson.M{}
	andQuery := []bson.M{}
	if params.AccountID != nil && *params.AccountID != "" {
		qParams[accountIDKey] = *params.AccountID
	}
	if params.AuthorizedAccountID != nil && *params.AuthorizedAccountID != "" {
		qParams["authorizedaccounts"] = *params.AuthorizedAccountID
	}
	if params.Name != nil && *params.Name != "" {
		qParams["name"] = *params.Name
	}
	if params.ClusterIdentifier != nil && *params.ClusterIdentifier != "" {
		qParams["clusteridentifier"] = *params.ClusterIdentifier
	}
	if params.ClusterType != nil && *params.ClusterType != "" {
		qParams["clustertype"] = *params.ClusterType
	}
	if params.ClusterVersion != nil && *params.ClusterVersion != "" {
		qParams["clusterversion"] = *params.ClusterVersion
	}
	if params.CspDomainID != nil && *params.CspDomainID != "" {
		qParams["cspdomainid"] = *params.CspDomainID
	}
	if params.ServiceHeartbeatTimeGE != nil && params.ServiceHeartbeatTimeLE != nil {
		hbTmGE := swag.TimeValue((*time.Time)(params.ServiceHeartbeatTimeGE))
		hbTmLE := swag.TimeValue((*time.Time)(params.ServiceHeartbeatTimeLE))
		andQuery = append(andQuery,
			bson.M{"service.heartbeattime": bson.M{"$gte": hbTmGE}},
			bson.M{"service.heartbeattime": bson.M{"$lte": hbTmLE}},
		)
	} else if params.ServiceHeartbeatTimeGE != nil {
		hbTmGE := swag.TimeValue((*time.Time)(params.ServiceHeartbeatTimeGE))
		qParams["service.heartbeattime"] = bson.M{"$gte": hbTmGE}
	} else if params.ServiceHeartbeatTimeLE != nil {
		hbTmLE := swag.TimeValue((*time.Time)(params.ServiceHeartbeatTimeLE))
		qParams["service.heartbeattime"] = bson.M{"$lte": hbTmLE}
	}
	if params.ServiceStateEQ != nil && *params.ServiceStateEQ != "" {
		qParams["service.state"] = *params.ServiceStateEQ
	}
	if params.ServiceStateNE != nil && *params.ServiceStateNE != "" {
		qParams["service.state"] = bson.M{"$ne": *params.ServiceStateNE}
	}
	if params.StateEQ != nil && *params.StateEQ != "" {
		qParams["state"] = *params.StateEQ
	}
	if params.StateNE != nil && *params.StateNE != "" {
		qParams["state"] = bson.M{"$ne": *params.StateNE}
	}
	if params.Tags != nil {
		qParams["tags"] = bson.M{"$all": params.Tags}
	}
	if len(andQuery) > 0 {
		qParams["$and"] = andQuery
	}
	op.log.Info("qParams:", qParams)
	return qParams
}

// objectDocumentHandler methods

func (op *clusterHandler) Claim(api DBAPI, crud ObjectDocumentHandlerCRUD) {
	op.api = api
	op.crud = crud
	op.log = api.Logger()
}

func (op *clusterHandler) Initialize(ctx context.Context) error {
	return op.crud.CreateIndexes(ctx, op)
}

func (op *clusterHandler) Start(ctx context.Context) error {
	dbn := op.api.DBName()
	op.log.Infof("Starting collection %s.%s", dbn, op.cName)
	return nil
}

func (op *clusterHandler) Ops() interface{} {
	return op
}

func (op *clusterHandler) CName() string {
	return op.cName
}

func (op *clusterHandler) Indexes() []mongo.IndexModel {
	return op.indexes
}

func (op *clusterHandler) NewObject() interface{} {
	return &Cluster{}
}

// ds.ClusterOps methods

// Count the number of matching documents, limit is applied if non-zero
func (op *clusterHandler) Count(ctx context.Context, params cluster.ClusterListParams, limit uint) (int, error) {
	// bypass the cache
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Count:", err)
		return 0, err
	}
	return op.crud.Count(ctx, op, op.convertListParams(params), limit)
}

func (op *clusterHandler) Create(ctx context.Context, mObj *M.ClusterCreateArgs) (*M.Cluster, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Create:", err)
		return nil, err
	}
	dObj := &Cluster{}
	mCluster := &M.Cluster{
		ClusterCreateOnce: mObj.ClusterCreateOnce,
		ClusterMutable: M.ClusterMutable{
			ClusterCreateMutable: mObj.ClusterCreateMutable,
		},
	}
	dObj.FromModel(mCluster)
	if err := op.crud.InsertOne(ctx, op, dObj); err != nil {
		return nil, err
	}
	op.mux.Lock()
	defer op.mux.Unlock()
	op.cache[dObj.MetaObjID] = dObj
	return dObj.ToModel(), nil
}

func (op *clusterHandler) Delete(ctx context.Context, mID string) error {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Delete:", err)
		return err
	}
	op.mux.Lock()
	defer op.mux.Unlock()
	delete(op.cache, mID)
	return op.crud.DeleteOne(ctx, op, mID)
}

func (op *clusterHandler) Fetch(ctx context.Context, mID string) (*M.Cluster, error) {
	op.mux.Lock()
	defer op.mux.Unlock()
	if dObj, ok := op.cache[mID]; ok {
		return dObj.ToModel(), nil
	}
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Fetch:", err)
		return nil, err
	}
	dObj := &Cluster{}
	if err := op.crud.FindOne(ctx, op, bson.M{objKey: mID}, dObj); err != nil {
		return nil, err
	}
	op.cache[mID] = dObj
	return dObj.ToModel(), nil
}

func (op *clusterHandler) List(ctx context.Context, params cluster.ClusterListParams) ([]*M.Cluster, error) {
	// bypass the cache to avoid re-implementing mongo query but add results to the cache
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("List:", err)
		return nil, err
	}
	list := make([]*M.Cluster, 0, 10)
	op.mux.Lock()
	defer op.mux.Unlock()
	err := op.crud.FindAll(ctx, op, params, op.convertListParams(params), func(obj interface{}) {
		dObj := obj.(*Cluster) // or panic
		op.cache[dObj.MetaObjID] = dObj
		list = append(list, dObj.ToModel())
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (op *clusterHandler) Update(ctx context.Context, ua *ds.UpdateArgs, param *M.ClusterMutable) (*M.Cluster, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Update:", err)
		return nil, err
	}
	mObj := &M.Cluster{ClusterMutable: *param}
	obj := &Cluster{}
	obj.FromModel(mObj)
	if err := op.crud.UpdateOne(ctx, op, obj, ua); err != nil {
		return nil, err
	}
	op.mux.Lock()
	defer op.mux.Unlock()
	op.cache[ua.ID] = obj
	return obj.ToModel(), nil
}
