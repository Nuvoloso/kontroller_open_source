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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/snapshot"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/op/go-logging"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// Snapshot object
type Snapshot struct {
	ObjMeta            `bson:",inline"`
	AccountID          string
	ConsistencyGroupID string
	PitIdentifier      string
	SizeBytes          int64
	SnapIdentifier     string
	SnapTime           time.Time
	TenantAccountID    string
	VolumeSeriesID     string
	DeleteAfterTime    time.Time
	Locations          SnapshotLocationMap
	Messages           []TimestampedString
	SystemTags         StringList
	Tags               StringList
	ProtectionDomainID string
}

// ToModel converts a datastore object to a model object
func (o *Snapshot) ToModel() *M.Snapshot {
	if o.Locations == nil {
		o.Locations = make(SnapshotLocationMap, 0)
	}
	if o.Messages == nil {
		o.Messages = make([]TimestampedString, 0)
	}
	if o.SystemTags == nil {
		o.SystemTags = make(StringList, 0)
	}
	if o.Tags == nil {
		o.Tags = make(StringList, 0)
	}
	mObj := &M.Snapshot{
		SnapshotAllOf0: M.SnapshotAllOf0{Meta: (&o.ObjMeta).ToModel("Snapshot")},
	}
	mObj.AccountID = M.ObjID(o.AccountID)
	mObj.ConsistencyGroupID = M.ObjID(o.ConsistencyGroupID)
	mObj.PitIdentifier = o.PitIdentifier
	mObj.SizeBytes = o.SizeBytes
	mObj.SnapIdentifier = o.SnapIdentifier
	mObj.SnapTime = strfmt.DateTime(o.SnapTime)
	mObj.TenantAccountID = M.ObjID(o.TenantAccountID)
	mObj.VolumeSeriesID = M.ObjIDMutable(o.VolumeSeriesID)
	mObj.DeleteAfterTime = strfmt.DateTime(o.DeleteAfterTime)
	mObj.Locations = o.Locations.ToModel()
	mObj.Messages = make([]*M.TimestampedString, len(o.Messages))
	for i, msg := range o.Messages {
		mObj.Messages[i] = msg.ToModel()
	}
	mObj.SystemTags = *o.SystemTags.ToModel()
	mObj.Tags = *o.Tags.ToModel()
	mObj.ProtectionDomainID = M.ObjIDMutable(o.ProtectionDomainID)
	return mObj
}

// FromModel converts a model object to a datastore object
func (o *Snapshot) FromModel(mObj *M.Snapshot) {
	mMeta := mObj.Meta
	if mMeta == nil {
		mMeta = &M.ObjMeta{ObjType: "Snapshot"}
	}
	(&o.ObjMeta).FromModel(mMeta)
	o.AccountID = string(mObj.AccountID)
	o.ConsistencyGroupID = string(mObj.ConsistencyGroupID)
	o.PitIdentifier = string(mObj.PitIdentifier)
	o.SizeBytes = mObj.SizeBytes
	o.SnapIdentifier = mObj.SnapIdentifier
	o.SnapTime = time.Time(mObj.SnapTime)
	o.TenantAccountID = string(mObj.TenantAccountID)
	o.VolumeSeriesID = string(mObj.VolumeSeriesID)
	o.DeleteAfterTime = time.Time(mObj.DeleteAfterTime)
	if o.Locations == nil {
		o.Locations = make(SnapshotLocationMap, len(mObj.Locations))
	}
	o.Locations.FromModel(mObj.Locations)
	if o.Messages == nil {
		o.Messages = make([]TimestampedString, len(mObj.Messages))
	}
	for i, msg := range mObj.Messages {
		o.Messages[i].FromModel(msg)
	}
	if o.SystemTags == nil {
		o.SystemTags = make(StringList, len(mObj.SystemTags))
	}
	o.SystemTags.FromModel(&mObj.SystemTags)
	if o.Tags == nil {
		o.Tags = make(StringList, len(mObj.Tags))
	}
	o.Tags.FromModel(&mObj.Tags)
	o.ProtectionDomainID = string(mObj.ProtectionDomainID)
}

// SnapshotHandler is an ObjectDocumentHandler that implements the ds.SnapshotOps operations
type snapshotHandler struct {
	cName   string
	api     DBAPI
	crud    ObjectDocumentHandlerCRUD
	log     *logging.Logger
	indexes []mongo.IndexModel
}

var _ = ds.SnapshotOps(&snapshotHandler{})
var _ = ObjectDocumentHandler(&snapshotHandler{})

var odhSnapshot = &snapshotHandler{
	cName: "snapshot",
	indexes: []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: accountIDKey, Value: IndexAscending}}},
		{Keys: bsonx.Doc{{Key: tenantAccountIDKey, Value: IndexAscending}}},
		{Keys: bsonx.Doc{{Key: volumeSeriesIDKey, Value: IndexAscending}, {Key: snapIdentifierKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)}, // pair of VolumeSeriesID, SnapIdentifier is unique, order important for query performance
		{Keys: bsonx.Doc{{Key: consistencyGroupIDKey, Value: IndexAscending}}},
	},
}

func init() {
	odhRegister(odhSnapshot.cName, odhSnapshot)
}

// convertListParams is a helper to convert SnapshotListParams to bson.M
func (op *snapshotHandler) convertListParams(params snapshot.SnapshotListParams) bson.M {
	qParams := bson.M{}
	andQuery := []bson.M{}
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
	if v := swag.StringValue(params.ConsistencyGroupID); v != "" {
		qParams["consistencygroupid"] = v
	}
	if v := swag.StringValue(params.CspDomainID); v != "" {
		qParams["locations."+*params.CspDomainID] = bson.M{"$exists": true}
	}
	if params.DeleteAfterTimeLE != nil {
		datLE := swag.TimeValue((*time.Time)(params.DeleteAfterTimeLE))
		qParams["deleteaftertime"] = bson.M{"$lte": datLE}
	}
	if params.ProtectionDomainID != nil {
		qParams["protectiondomainid"] = *params.ProtectionDomainID
	}
	if v := swag.StringValue(params.SnapIdentifier); v != "" {
		qParams["snapidentifier"] = v
	}
	if params.SnapTimeLE != nil && params.SnapTimeGE != nil {
		stLE := swag.TimeValue((*time.Time)(params.SnapTimeLE))
		stGE := swag.TimeValue((*time.Time)(params.SnapTimeGE))
		andQuery = append(andQuery,
			bson.M{"snaptime": bson.M{"$gte": stGE}},
			bson.M{"snaptime": bson.M{"$lte": stLE}},
		)
	} else if params.SnapTimeGE != nil {
		stGE := swag.TimeValue((*time.Time)(params.SnapTimeGE))
		qParams["snaptime"] = bson.M{"$gte": stGE}
	} else if params.SnapTimeLE != nil {
		stLE := swag.TimeValue((*time.Time)(params.SnapTimeLE))
		qParams["snaptime"] = bson.M{"$lte": stLE}
	}
	if params.SystemTags != nil {
		qParams["systemtags"] = bson.M{"$all": params.SystemTags}
	}
	if params.Tags != nil {
		qParams["tags"] = bson.M{"$all": params.Tags}
	}
	if v := swag.StringValue(params.VolumeSeriesID); v != "" {
		qParams["volumeseriesid"] = v
	}
	if len(params.PitIdentifiers) > 0 {
		qParams["pitidentifier"] = bson.M{"$in": params.PitIdentifiers}
	}
	if len(andQuery) > 0 {
		qParams["$and"] = andQuery
	}
	orParams.setQueryParam(qParams)
	op.log.Info("qParams:", qParams)
	return qParams
}

// objectDocumentHandler methods

func (op *snapshotHandler) Claim(api DBAPI, crud ObjectDocumentHandlerCRUD) {
	op.api = api
	op.crud = crud
	op.log = api.Logger()
}

func (op *snapshotHandler) Initialize(ctx context.Context) error {
	return op.crud.CreateIndexes(ctx, op)
}

func (op *snapshotHandler) Start(ctx context.Context) error {
	dbn := op.api.DBName()
	op.log.Infof("Starting collection %s.%s", dbn, op.cName)
	return nil
}

func (op *snapshotHandler) Ops() interface{} {
	return op
}

func (op *snapshotHandler) CName() string {
	return op.cName
}

func (op *snapshotHandler) Indexes() []mongo.IndexModel {
	return op.indexes
}

func (op *snapshotHandler) NewObject() interface{} {
	return &Snapshot{}
}

// ds.snapshotOps methods

// Count the number of matching documents, limit is applied if non-zero
func (op *snapshotHandler) Count(ctx context.Context, params snapshot.SnapshotListParams, limit uint) (int, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Count:", err)
		return 0, err
	}
	return op.crud.Count(ctx, op, op.convertListParams(params), limit)
}

// Create creates the Snapshot object
// Note: Validations are performed by the handler. Take care when calling internally.
func (op *snapshotHandler) Create(ctx context.Context, mObj *M.Snapshot) (*M.Snapshot, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Create:", err)
		return nil, err
	}
	mObj.Meta = nil // ensure that meta will be put into correct creation state by FromModel
	dObj := &Snapshot{}
	dObj.FromModel(mObj)
	if err := op.crud.InsertOne(ctx, op, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *snapshotHandler) Delete(ctx context.Context, mID string) error {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Delete:", err)
		return err
	}
	return op.crud.DeleteOne(ctx, op, mID)
}

func (op *snapshotHandler) Fetch(ctx context.Context, mID string) (*M.Snapshot, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Fetch:", err)
		return nil, err
	}
	dObj := &Snapshot{}
	if err := op.crud.FindOne(ctx, op, bson.M{objKey: mID}, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *snapshotHandler) List(ctx context.Context, params snapshot.SnapshotListParams) ([]*M.Snapshot, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("List:", err)
		return nil, err
	}
	list := make([]*M.Snapshot, 0, 10)
	err := op.crud.FindAll(ctx, op, params, op.convertListParams(params), func(obj interface{}) {
		dObj := obj.(*Snapshot) // or panic
		list = append(list, dObj.ToModel())
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (op *snapshotHandler) Update(ctx context.Context, ua *ds.UpdateArgs, param *M.SnapshotMutable) (*M.Snapshot, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Update:", err)
		return nil, err
	}
	mObj := &M.Snapshot{SnapshotMutable: *param}
	obj := &Snapshot{}
	obj.FromModel(mObj)
	if err := op.crud.UpdateOne(ctx, op, obj, ua); err != nil {
		return nil, err
	}
	return obj.ToModel(), nil
}
