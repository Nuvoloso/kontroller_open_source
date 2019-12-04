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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/storage_request"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/op/go-logging"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// StorageRequest object
type StorageRequest struct {
	ObjMeta                   `bson:",inline"`
	RequestedOperations       []string
	CspStorageType            string
	NodeID                    string
	ReattachNodeID            string
	AccountID                 string
	TenantAccountID           string
	CspDomainID               string
	ClusterID                 string
	MinSizeBytes              int64
	CompleteByTime            time.Time
	StorageID                 string
	StorageRequestState       string
	PoolID                    string
	ParcelSizeBytes           int64
	ShareableStorage          bool
	RequestMessages           []TimestampedString
	SystemTags                StringList
	VolumeSeriesRequestClaims VsrClaim
	Terminated                bool // derived field, not exposed in the model
}

var terminalSRStates = []string{com.StgReqStateSucceeded, com.StgReqStateFailed}

// ToModel converts a datastore object to a model object
func (o *StorageRequest) ToModel() *M.StorageRequest {
	if o.RequestedOperations == nil {
		o.RequestedOperations = make([]string, 0)
	}
	if o.RequestMessages == nil {
		o.RequestMessages = make([]TimestampedString, 0)
	}
	if o.SystemTags == nil {
		o.SystemTags = make(StringList, 0)
	}
	mObj := &M.StorageRequest{
		StorageRequestAllOf0: M.StorageRequestAllOf0{Meta: (&o.ObjMeta).ToModel("StorageRequest")},
	}
	mObj.RequestedOperations = make([]string, len(o.RequestedOperations))
	for i, s := range o.RequestedOperations {
		mObj.RequestedOperations[i] = s
	}
	mObj.CspStorageType = o.CspStorageType
	mObj.AccountID = M.ObjID(o.AccountID)
	mObj.TenantAccountID = M.ObjID(o.TenantAccountID)
	mObj.NodeID = M.ObjIDMutable(o.NodeID)
	mObj.ReattachNodeID = M.ObjIDMutable(o.ReattachNodeID)
	mObj.CspDomainID = M.ObjID(o.CspDomainID)
	mObj.ClusterID = M.ObjID(o.ClusterID)
	mObj.MinSizeBytes = swag.Int64(o.MinSizeBytes)
	mObj.CompleteByTime = strfmt.DateTime(o.CompleteByTime)
	mObj.StorageID = M.ObjIDMutable(o.StorageID)
	mObj.StorageRequestState = o.StorageRequestState
	mObj.PoolID = M.ObjIDMutable(o.PoolID)
	mObj.ParcelSizeBytes = swag.Int64(o.ParcelSizeBytes)
	mObj.ShareableStorage = o.ShareableStorage
	mObj.RequestMessages = make([]*M.TimestampedString, len(o.RequestMessages))
	for i, msg := range o.RequestMessages {
		mObj.RequestMessages[i] = msg.ToModel()
	}
	mObj.SystemTags = *o.SystemTags.ToModel()
	mObj.VolumeSeriesRequestClaims = o.VolumeSeriesRequestClaims.ToModel()
	return mObj
}

// FromModel converts a model object to a datastore object
func (o *StorageRequest) FromModel(mObj *M.StorageRequest) {
	mMeta := mObj.Meta
	if mMeta == nil {
		mMeta = &M.ObjMeta{ObjType: "StorageRequest"}
	}
	(&o.ObjMeta).FromModel(mMeta)
	if o.RequestedOperations == nil {
		o.RequestedOperations = make([]string, len(mObj.RequestedOperations))
	}
	for i, s := range mObj.RequestedOperations {
		o.RequestedOperations[i] = s
	}
	o.CspStorageType = mObj.CspStorageType
	o.AccountID = string(mObj.AccountID)
	o.TenantAccountID = string(mObj.TenantAccountID)
	o.NodeID = string(mObj.NodeID)
	o.ReattachNodeID = string(mObj.ReattachNodeID)
	o.CspDomainID = string(mObj.CspDomainID)
	o.ClusterID = string(mObj.ClusterID)
	o.MinSizeBytes = swag.Int64Value(mObj.MinSizeBytes)
	o.CompleteByTime = time.Time(mObj.CompleteByTime)
	o.StorageID = string(mObj.StorageID)
	o.StorageRequestState = mObj.StorageRequestState
	o.Terminated = util.Contains(terminalSRStates, o.StorageRequestState)
	o.PoolID = string(mObj.PoolID)
	o.ParcelSizeBytes = swag.Int64Value(mObj.ParcelSizeBytes)
	o.ShareableStorage = mObj.ShareableStorage
	if o.RequestMessages == nil {
		o.RequestMessages = make([]TimestampedString, len(mObj.RequestMessages))
	}
	for i, msg := range mObj.RequestMessages {
		o.RequestMessages[i].FromModel(msg)
	}
	if o.SystemTags == nil {
		o.SystemTags = make(StringList, len(mObj.SystemTags))
	}
	o.SystemTags.FromModel(&mObj.SystemTags)
	vsrC := mObj.VolumeSeriesRequestClaims
	if vsrC == nil {
		vsrC = &M.VsrClaim{}
	}
	o.VolumeSeriesRequestClaims.FromModel(vsrC)
}

// storageRequestHandler is an ObjectDocumentHandler that implements the ds.StorageRequestOps operations
type storageRequestHandler struct {
	cName   string
	api     DBAPI
	crud    ObjectDocumentHandlerCRUD
	log     *logging.Logger
	indexes []mongo.IndexModel
}

var _ = ds.StorageRequestOps(&storageRequestHandler{})
var _ = ObjectDocumentHandler(&storageRequestHandler{})

var odhStorageRequest = &storageRequestHandler{
	cName: "storagerequest",
	indexes: []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: nodeIDKey, Value: IndexAscending}}},
		{Keys: bsonx.Doc{{Key: terminatedKey, Value: IndexAscending}}, Options: ActiveRequestsOnlyIndex},
	},
}

func init() {
	odhRegister(odhStorageRequest.cName, odhStorageRequest)
}

// convertListParams is a helper to convert StorageRequestListParams to bson.M
func (op *storageRequestHandler) convertListParams(params storage_request.StorageRequestListParams) bson.M {
	qParams := bson.M{}
	orParams := newAndOrList()
	if params.CspDomainID != nil && *params.CspDomainID != "" {
		qParams[cspDomainIDKey] = *params.CspDomainID
	}
	if params.ClusterID != nil && *params.ClusterID != "" {
		qParams[clusterIDKey] = *params.ClusterID
	}
	if params.NodeID != nil && *params.NodeID != "" {
		qParams["nodeid"] = *params.NodeID
	}
	if params.StorageID != nil && *params.StorageID != "" {
		qParams["storageid"] = *params.StorageID
	}
	if params.PoolID != nil && *params.PoolID != "" {
		qParams["poolid"] = *params.PoolID
	}
	if params.ActiveOrTimeModifiedGE != nil {
		aTmGE := swag.TimeValue((*time.Time)(params.ActiveOrTimeModifiedGE))
		orParams.append(
			bson.M{"MetaTM": bson.M{"$gte": aTmGE}},
			bson.M{terminatedKey: false},
		)
	} else {
		if params.IsTerminated != nil {
			qParams[terminatedKey] = *params.IsTerminated
		} else if params.StorageRequestState != nil && *params.StorageRequestState != "" {
			if !util.Contains(terminalSRStates, *params.StorageRequestState) {
				qParams[terminatedKey] = false
			}
			qParams["storagerequeststate"] = *params.StorageRequestState
		}
	}
	if params.SystemTags != nil {
		qParams["systemtags"] = bson.M{"$all": params.SystemTags}
	}
	if params.VolumeSeriesRequestID != nil {
		qParams["volumeseriesrequestclaims.claims."+*params.VolumeSeriesRequestID] = bson.M{"$exists": true}
	}
	orParams.setQueryParam(qParams)
	op.log.Info("qParams:", qParams)
	return qParams
}

// objectDocumentHandler methods

func (op *storageRequestHandler) Claim(api DBAPI, crud ObjectDocumentHandlerCRUD) {
	op.api = api
	op.crud = crud
	op.log = api.Logger()
}

func (op *storageRequestHandler) Initialize(ctx context.Context) error {
	return op.crud.CreateIndexes(ctx, op)
}

func (op *storageRequestHandler) Start(ctx context.Context) error {
	dbn := op.api.DBName()
	op.log.Infof("Starting collection %s.%s", dbn, op.cName)
	return nil
}

func (op *storageRequestHandler) Ops() interface{} {
	return op
}

func (op *storageRequestHandler) CName() string {
	return op.cName
}

func (op *storageRequestHandler) Indexes() []mongo.IndexModel {
	return op.indexes
}

func (op *storageRequestHandler) NewObject() interface{} {
	return &StorageRequest{}
}

// ds.StorageRequestOps methods

// Count the number of matching documents, limit is applied if non-zero
func (op *storageRequestHandler) Count(ctx context.Context, params storage_request.StorageRequestListParams, limit uint) (int, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Count:", err)
		return 0, err
	}
	return op.crud.Count(ctx, op, op.convertListParams(params), limit)
}

// Create creates the StorageRequest object
// Note: Validations are performed by the handler. Take care when calling internally.
func (op *storageRequestHandler) Create(ctx context.Context, mObj *M.StorageRequestCreateArgs) (*M.StorageRequest, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Create:", err)
		return nil, err
	}
	dObj := &StorageRequest{}
	mSR := &M.StorageRequest{
		StorageRequestCreateOnce: mObj.StorageRequestCreateOnce,
		StorageRequestMutable: M.StorageRequestMutable{
			StorageRequestMutableAllOf0: M.StorageRequestMutableAllOf0{
				StorageRequestState: ds.DefaultStorageRequestState,
			},
			StorageRequestCreateMutable: mObj.StorageRequestCreateMutable,
		},
	}
	dObj.FromModel(mSR)
	if err := op.crud.InsertOne(ctx, op, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *storageRequestHandler) Delete(ctx context.Context, mID string) error {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Delete:", err)
		return err
	}
	return op.crud.DeleteOne(ctx, op, mID)
}

func (op *storageRequestHandler) Fetch(ctx context.Context, mID string) (*M.StorageRequest, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Fetch:", err)
		return nil, err
	}
	dObj := &StorageRequest{}
	if err := op.crud.FindOne(ctx, op, bson.M{objKey: mID}, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *storageRequestHandler) List(ctx context.Context, params storage_request.StorageRequestListParams) ([]*M.StorageRequest, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("List:", err)
		return nil, err
	}
	list := make([]*M.StorageRequest, 0, 10)
	err := op.crud.FindAll(ctx, op, params, op.convertListParams(params), func(obj interface{}) {
		dObj := obj.(*StorageRequest) // or panic
		list = append(list, dObj.ToModel())
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (op *storageRequestHandler) Update(ctx context.Context, ua *ds.UpdateArgs, param *M.StorageRequestMutable) (*M.StorageRequest, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Update:", err)
		return nil, err
	}
	mObj := &M.StorageRequest{StorageRequestMutable: *param}
	obj := &StorageRequest{}
	obj.FromModel(mObj)
	if ua.IsModified("StorageRequestState") {
		// Terminated field gets auto-updated on StorageRequestState change
		ua.Attributes = append(ua.Attributes, ds.UpdateAttr{
			Name: "Terminated",
			Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
				ds.UpdateSet: ds.UpdateActionArgs{
					FromBody: true,
				},
			},
		})
	}
	if err := op.crud.UpdateOne(ctx, op, obj, ua); err != nil {
		return nil, err
	}
	return obj.ToModel(), nil
}
