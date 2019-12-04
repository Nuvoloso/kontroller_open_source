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
	"time"

	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/node"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/go-openapi/swag"
	"github.com/op/go-logging"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// Node object
type Node struct {
	ObjMeta             `bson:",inline"`
	AccountID           string
	ClusterID           string
	Name                string
	Description         string
	LocalStorage        NodeStorageDeviceMap
	AvailableCacheBytes int64
	CacheUnitSizeBytes  int64
	TotalCacheBytes     int64
	NodeIdentifier      string
	NodeAttributes      StringValueMap
	Service             NuvoService
	State               string
	Tags                StringList
}

// ToModel converts a datastore object to a model object
func (o *Node) ToModel() *M.Node {
	if o.Tags == nil {
		o.Tags = make(StringList, 0)
	}
	if o.LocalStorage == nil {
		o.LocalStorage = make(NodeStorageDeviceMap, 0)
	}
	if o.NodeAttributes == nil {
		o.NodeAttributes = make(StringValueMap, 0)
	}
	return &M.Node{
		NodeAllOf0: M.NodeAllOf0{
			Meta:           (&o.ObjMeta).ToModel("Node"),
			AccountID:      M.ObjIDMutable(o.AccountID),
			ClusterID:      M.ObjIDMutable(o.ClusterID),
			NodeIdentifier: o.NodeIdentifier,
		},
		NodeMutable: M.NodeMutable{
			Name:                M.ObjName(o.Name),
			Description:         M.ObjDescription(o.Description),
			Tags:                *o.Tags.ToModel(),
			LocalStorage:        o.LocalStorage.ToModel(),
			AvailableCacheBytes: swag.Int64(o.AvailableCacheBytes),
			CacheUnitSizeBytes:  swag.Int64(o.CacheUnitSizeBytes),
			TotalCacheBytes:     swag.Int64(o.TotalCacheBytes),
			Service:             o.Service.ToModel(),
			State:               o.State,
			NodeAttributes:      o.NodeAttributes.ToModel(),
		},
	}
}

// FromModel converts a model object to a datastore object
func (o *Node) FromModel(mObj *M.Node) {
	mMeta := mObj.Meta
	if mMeta == nil {
		mMeta = &M.ObjMeta{ObjType: "Node"}
	}
	(&o.ObjMeta).FromModel(mMeta)
	o.AccountID = string(mObj.AccountID)
	o.ClusterID = string(mObj.ClusterID)
	o.Name = string(mObj.Name)
	o.Description = string(mObj.Description)
	if o.LocalStorage == nil {
		o.LocalStorage = make(NodeStorageDeviceMap, len(mObj.LocalStorage))
	}
	o.LocalStorage.FromModel(mObj.LocalStorage)
	o.AvailableCacheBytes = swag.Int64Value(mObj.AvailableCacheBytes)
	o.CacheUnitSizeBytes = swag.Int64Value(mObj.CacheUnitSizeBytes)
	o.TotalCacheBytes = swag.Int64Value(mObj.TotalCacheBytes)
	o.NodeIdentifier = mObj.NodeIdentifier
	if o.NodeAttributes == nil {
		o.NodeAttributes = make(StringValueMap, len(mObj.NodeAttributes))
	}
	o.NodeAttributes.FromModel(mObj.NodeAttributes)
	if o.Tags == nil {
		o.Tags = make(StringList, len(mObj.Tags))
	}
	o.Tags.FromModel(&mObj.Tags)
	nnc := mObj.Service
	if nnc == nil {
		nnc = &M.NuvoService{}
	}
	o.Service.FromModel(nnc)
	o.State = mObj.State
}

// nodeHandler is an ObjectDocumentHandler that implements the ds.NodeOps operations
type nodeHandler struct {
	cName   string
	api     DBAPI
	crud    ObjectDocumentHandlerCRUD
	log     *logging.Logger
	indexes []mongo.IndexModel
}

var _ = ds.NodeOps(&nodeHandler{})
var _ = ObjectDocumentHandler(&nodeHandler{})

var odhNode = &nodeHandler{
	cName: "node",
	indexes: []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: clusterIDKey, Value: IndexAscending}, {Key: nameKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: clusterIDKey, Value: IndexAscending}, {Key: nodeIdentifierKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
	},
}

func init() {
	odhRegister(odhNode.cName, odhNode)
}

// convertListParams is a helper to convert NodeListParams to bson.M
func (op *nodeHandler) convertListParams(params node.NodeListParams) bson.M {
	qParams := bson.M{}
	andQuery := []bson.M{}
	if params.Name != nil && *params.Name != "" {
		qParams[nameKey] = *params.Name
	}
	if params.Tags != nil {
		qParams["tags"] = bson.M{"$all": params.Tags}
	}
	if params.ClusterID != nil && *params.ClusterID != "" {
		qParams["clusterid"] = *params.ClusterID
	}
	if params.NodeIdentifier != nil && *params.NodeIdentifier != "" {
		qParams["nodeidentifier"] = *params.NodeIdentifier
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
	if len(params.NodeIds) > 0 {
		qParams["MetaID"] = bson.M{"$in": params.NodeIds}
	}
	if len(andQuery) > 0 {
		qParams["$and"] = andQuery
	}
	op.log.Info("qParams:", qParams)
	return qParams
}

// objectDocumentHandler methods

func (op *nodeHandler) Claim(api DBAPI, crud ObjectDocumentHandlerCRUD) {
	op.api = api
	op.crud = crud
	op.log = api.Logger()
}

func (op *nodeHandler) Initialize(ctx context.Context) error {
	return op.crud.CreateIndexes(ctx, op)
}

func (op *nodeHandler) Start(ctx context.Context) error {
	dbn := op.api.DBName()
	op.log.Infof("Starting collection %s.%s", dbn, op.cName)
	return nil
}

func (op *nodeHandler) Ops() interface{} {
	return op
}

func (op *nodeHandler) CName() string {
	return op.cName
}

func (op *nodeHandler) Indexes() []mongo.IndexModel {
	return op.indexes
}

func (op *nodeHandler) NewObject() interface{} {
	return &Node{}
}

// ds.NodeOps methods

// Count the number of matching documents, limit is applied if non-zero
func (op *nodeHandler) Count(ctx context.Context, params node.NodeListParams, limit uint) (int, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Count:", err)
		return 0, err
	}
	return op.crud.Count(ctx, op, op.convertListParams(params), limit)
}

func (op *nodeHandler) Create(ctx context.Context, mObj *M.Node) (*M.Node, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Create:", err)
		return nil, err
	}
	mObj.Meta = nil // ensure that meta will be put into correct creation state by FromModel
	dObj := &Node{}
	dObj.FromModel(mObj)
	if err := op.crud.InsertOne(ctx, op, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *nodeHandler) Delete(ctx context.Context, mID string) error {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Delete:", err)
		return err
	}
	return op.crud.DeleteOne(ctx, op, mID)
}

func (op *nodeHandler) Fetch(ctx context.Context, mID string) (*M.Node, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Fetch:", err)
		return nil, err
	}
	dObj := &Node{}
	if err := op.crud.FindOne(ctx, op, bson.M{objKey: mID}, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *nodeHandler) List(ctx context.Context, params node.NodeListParams) ([]*M.Node, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("List:", err)
		return nil, err
	}
	list := make([]*M.Node, 0, 10)
	err := op.crud.FindAll(ctx, op, params, op.convertListParams(params), func(obj interface{}) {
		dObj := obj.(*Node) // or panic
		list = append(list, dObj.ToModel())
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (op *nodeHandler) Update(ctx context.Context, ua *ds.UpdateArgs, param *M.NodeMutable) (*M.Node, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Update:", err)
		return nil, err
	}
	mObj := &M.Node{NodeMutable: *param}
	obj := &Node{}
	obj.FromModel(mObj)
	if err := op.crud.UpdateOne(ctx, op, obj, ua); err != nil {
		return nil, err
	}
	return obj.ToModel(), nil
}

func (op *nodeHandler) UpdateMultiple(ctx context.Context, lParam node.NodeListParams, ua *ds.UpdateArgs, uParam *M.NodeMutable) (int, int, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Update:", err)
		return 0, 0, err
	}
	mObj := &M.Node{NodeMutable: *uParam}
	obj := &Node{}
	obj.FromModel(mObj)
	return op.crud.UpdateAll(ctx, op, op.convertListParams(lParam), obj, ua)
}
