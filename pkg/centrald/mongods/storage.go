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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/storage"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/go-openapi/swag"
	"github.com/op/go-logging"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// Storage object
type Storage struct {
	ObjMeta              `bson:",inline"`
	AccountID            string
	TenantAccountID      string
	CspDomainID          string
	CspStorageType       string
	StorageAccessibility StorageAccessibility
	StorageIdentifier    string
	PoolID               string
	ClusterID            string
	SizeBytes            int64
	AvailableBytes       int64
	ParcelSizeBytes      int64
	TotalParcelCount     int64
	ShareableStorage     bool
	StorageState         StorageState
	SystemTags           StringList
}

// ToModel converts a datastore object to a model object
func (o *Storage) ToModel() *M.Storage {
	mObj := &M.Storage{
		StorageAllOf0: M.StorageAllOf0{
			AccountID:            M.ObjID(o.AccountID),
			TenantAccountID:      M.ObjID(o.TenantAccountID),
			CspDomainID:          M.ObjID(o.CspDomainID),
			CspStorageType:       M.CspStorageType(o.CspStorageType),
			Meta:                 (&o.ObjMeta).ToModel("Storage"),
			SizeBytes:            swag.Int64(o.SizeBytes),
			StorageAccessibility: &M.StorageAccessibility{StorageAccessibilityMutable: *o.StorageAccessibility.ToModel()},
			PoolID:               M.ObjIDMutable(o.PoolID),
			ClusterID:            M.ObjID(o.ClusterID),
		},
		StorageMutable: M.StorageMutable{
			AvailableBytes:    swag.Int64(o.AvailableBytes),
			ParcelSizeBytes:   swag.Int64(o.ParcelSizeBytes),
			TotalParcelCount:  swag.Int64(o.TotalParcelCount),
			StorageIdentifier: o.StorageIdentifier,
			StorageState:      o.StorageState.ToModel(),
			ShareableStorage:  o.ShareableStorage,
		},
	}
	if o.SystemTags == nil {
		o.SystemTags = make(StringList, 0)
	}
	mObj.SystemTags = *o.SystemTags.ToModel()
	return mObj
}

// FromModel converts a model object to a datastore object
func (o *Storage) FromModel(mObj *M.Storage) {
	mMeta := mObj.Meta
	if mMeta == nil {
		mMeta = &M.ObjMeta{ObjType: "Storage"}
	}
	(&o.ObjMeta).FromModel(mMeta)
	o.AccountID = string(mObj.AccountID)
	o.TenantAccountID = string(mObj.TenantAccountID)
	o.CspDomainID = string(mObj.CspDomainID)
	o.CspStorageType = string(mObj.CspStorageType)
	o.SizeBytes = swag.Int64Value(mObj.SizeBytes)
	sa := mObj.StorageAccessibility
	if sa == nil {
		sa = &M.StorageAccessibility{}
	}
	o.StorageAccessibility.FromModel(&sa.StorageAccessibilityMutable)
	o.StorageIdentifier = mObj.StorageIdentifier
	o.PoolID = string(mObj.PoolID)
	o.ClusterID = string(mObj.ClusterID)
	o.AvailableBytes = swag.Int64Value(mObj.AvailableBytes)
	o.ParcelSizeBytes = swag.Int64Value(mObj.ParcelSizeBytes)
	o.TotalParcelCount = swag.Int64Value(mObj.TotalParcelCount)
	ss := mObj.StorageState
	if ss == nil {
		ss = &M.StorageStateMutable{}
	}
	o.StorageState.FromModel(ss)
	o.ShareableStorage = mObj.ShareableStorage
	if o.SystemTags == nil {
		o.SystemTags = make(StringList, len(mObj.SystemTags))
	}
	o.SystemTags.FromModel(&mObj.SystemTags)
}

// storageHandler is an ObjectDocumentHandler that implements the ds.StorageOps operations
type storageHandler struct {
	cName   string
	api     DBAPI
	crud    ObjectDocumentHandlerCRUD
	log     *logging.Logger
	indexes []mongo.IndexModel
}

var _ = ds.StorageOps(&storageHandler{})
var _ = ObjectDocumentHandler(&storageHandler{})

var odhStorage = &storageHandler{
	cName: "storage",
	indexes: []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
	},
}

func init() {
	odhRegister(odhStorage.cName, odhStorage)
}

// convertListParams is a helper to convert StorageListParams to bson.M
func (op *storageHandler) convertListParams(params storage.StorageListParams) bson.M {
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
	if params.CspDomainID != nil && *params.CspDomainID != "" {
		qParams["cspdomainid"] = *params.CspDomainID
	}
	if params.CspStorageType != nil && *params.CspStorageType != "" {
		qParams["cspstoragetype"] = *params.CspStorageType
	}
	if params.PoolID != nil && *params.PoolID != "" {
		qParams["poolid"] = *params.PoolID
	}
	if params.AccessibilityScope != nil && *params.AccessibilityScope != "" {
		qParams["storageaccessibility.accessibilityscope"] = *params.AccessibilityScope
	}
	if params.AccessibilityScopeObjID != nil && *params.AccessibilityScopeObjID != "" {
		qParams["storageaccessibility.accessibilityscopeobjid"] = *params.AccessibilityScopeObjID
	}
	if params.ClusterID != nil && *params.ClusterID != "" {
		qParams["clusterid"] = *params.ClusterID
	}
	if params.AttachedNodeID != nil && *params.AttachedNodeID != "" {
		qParams["storagestate.attachednodeid"] = *params.AttachedNodeID
		qParams["storagestate.attachmentstate"] = bson.M{"$ne": "DETACHED"}
	}
	if params.AvailableBytesGE != nil && params.AvailableBytesLT != nil {
		// this simple approach works because there is only a single conjunction
		qParams["$and"] = []bson.M{
			bson.M{"availablebytes": bson.M{"$gte": *params.AvailableBytesGE}},
			bson.M{"availablebytes": bson.M{"$lt": *params.AvailableBytesLT}},
		}
	} else if params.AvailableBytesGE != nil { // zero is valid
		qParams["availablebytes"] = bson.M{"$gte": *params.AvailableBytesGE}
	} else if params.AvailableBytesLT != nil { // zero is valid
		qParams["availablebytes"] = bson.M{"$lt": *params.AvailableBytesLT}
	}
	if params.ProvisionedState != nil && *params.ProvisionedState != "" {
		qParams["storagestate.provisionedstate"] = *params.ProvisionedState
	}
	if params.AttachmentState != nil && *params.AttachmentState != "" {
		qParams["storagestate.attachmentstate"] = *params.AttachmentState
	}
	if params.DeviceState != nil && *params.DeviceState != "" {
		qParams["storagestate.devicestate"] = *params.DeviceState
	}
	if params.MediaState != nil && *params.MediaState != "" {
		qParams["storagestate.mediastate"] = *params.MediaState
	}
	if params.IsProvisioned != nil {
		op := "$nin"
		if *params.IsProvisioned {
			op = "$in"
		}
		qParams["storagestate.provisionedstate"] = bson.M{op: []string{"PROVISIONED", "UNPROVISIONING"}}
	}
	if params.SystemTags != nil {
		qParams["systemtags"] = bson.M{"$all": params.SystemTags}
	}
	orParams.setQueryParam(qParams)
	op.log.Info("qParams:", qParams)
	return qParams
}

// objectDocumentHandler methods

func (op *storageHandler) Claim(api DBAPI, crud ObjectDocumentHandlerCRUD) {
	op.api = api
	op.crud = crud
	op.log = api.Logger()
}

func (op *storageHandler) Initialize(ctx context.Context) error {
	return op.crud.CreateIndexes(ctx, op)
}

func (op *storageHandler) Start(ctx context.Context) error {
	dbn := op.api.DBName()
	op.log.Infof("Starting collection %s.%s", dbn, op.cName)
	return nil
}

func (op *storageHandler) Ops() interface{} {
	return op
}

func (op *storageHandler) CName() string {
	return op.cName
}

func (op *storageHandler) Indexes() []mongo.IndexModel {
	return op.indexes
}

func (op *storageHandler) NewObject() interface{} {
	return &Storage{}
}

// ds.StorageOps methods

// Aggregate the matching documents.
// A list of aggregations and the count of matching documents is returned on success.
func (op *storageHandler) Aggregate(ctx context.Context, params storage.StorageListParams) ([]*ds.Aggregation, int, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Aggregate:", err)
		return nil, 0, err
	}
	return op.crud.Aggregate(ctx, op, &params, op.convertListParams(params))
}

// Count the number of matching documents, limit is applied if non-zero
func (op *storageHandler) Count(ctx context.Context, params storage.StorageListParams, limit uint) (int, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Count:", err)
		return 0, err
	}
	return op.crud.Count(ctx, op, op.convertListParams(params), limit)
}

// Create creates the Storage object
// Note: Validations are performed by the handler. Take care when calling internally.
func (op *storageHandler) Create(ctx context.Context, mObj *M.Storage) (*M.Storage, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Create:", err)
		return nil, err
	}
	mObj.Meta = nil // ensure that meta will be put into correct creation state by FromModel
	dObj := &Storage{}
	dObj.FromModel(mObj)
	if err := op.crud.InsertOne(ctx, op, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *storageHandler) Delete(ctx context.Context, mID string) error {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Delete:", err)
		return err
	}
	return op.crud.DeleteOne(ctx, op, mID)
}

func (op *storageHandler) Fetch(ctx context.Context, mID string) (*M.Storage, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Fetch:", err)
		return nil, err
	}
	dObj := &Storage{}
	if err := op.crud.FindOne(ctx, op, bson.M{objKey: mID}, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *storageHandler) List(ctx context.Context, params storage.StorageListParams) ([]*M.Storage, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("List:", err)
		return nil, err
	}
	list := make([]*M.Storage, 0, 10)
	err := op.crud.FindAll(ctx, op, params, op.convertListParams(params), func(obj interface{}) {
		dObj := obj.(*Storage) // or panic
		list = append(list, dObj.ToModel())
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (op *storageHandler) Update(ctx context.Context, ua *ds.UpdateArgs, param *M.StorageMutable) (*M.Storage, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Update:", err)
		return nil, err
	}
	mObj := &M.Storage{StorageMutable: *param}
	obj := &Storage{}
	obj.FromModel(mObj)
	if err := op.crud.UpdateOne(ctx, op, obj, ua); err != nil {
		return nil, err
	}
	return obj.ToModel(), nil
}
