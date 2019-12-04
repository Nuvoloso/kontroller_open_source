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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
	"github.com/op/go-logging"
	uuid "github.com/satori/go.uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// VolumeSeries object
type VolumeSeries struct {
	ObjMeta                         `bson:",inline"`
	VolumeSeriesCreateOnceFields    `bson:",inline"`
	VolumeSeriesCreateMutableFields `bson:",inline"`
	BoundClusterID                  string
	BoundCspDomainID                string
	ConfiguredNodeID                string
	RootStorageID                   string // aka the RootDeviceUUID
	NuvoVolumeIdentifier            string
	RootParcelUUID                  string
	ServicePlanAllocationID         string
	VolumeSeriesState               string
	Messages                        []TimestampedString
	CacheAllocations                CacheAllocationMap
	CapacityAllocations             CapacityAllocationMap
	StorageParcels                  ParcelAllocationMap
	Mounts                          []Mount
}

// VolumeSeriesCreateOnceFields contains the subset of VolumeSeries fields that are specified during creation.
// It is defined separately so it can also be referenced in VolumeSeriesRequest.
type VolumeSeriesCreateOnceFields struct {
	AccountID       string
	TenantAccountID string
}

// VolumeSeriesCreateMutableFields contains the subset of VolumeSeries fields that are specified during creation.
// It is defined separately so it can also be referenced in VolumeSeriesRequest.
type VolumeSeriesCreateMutableFields struct {
	Name                    string
	Description             string
	SizeBytes               int64
	SpaAdditionalBytes      int64
	ConsistencyGroupID      string
	ServicePlanID           string
	LifecycleManagementData LifecycleManagementData
	Tags                    StringList
	SystemTags              StringList
	ClusterDescriptor       StringValueMap
}

// ToModel converts a datastore object to a model object
func (o *VolumeSeries) ToModel() *M.VolumeSeries {
	if o.Messages == nil {
		o.Messages = make([]TimestampedString, 0)
	}
	if o.CacheAllocations == nil {
		o.CacheAllocations = make(CacheAllocationMap, 0)
	}
	if o.CapacityAllocations == nil {
		o.CapacityAllocations = make(CapacityAllocationMap, 0)
	}
	if o.StorageParcels == nil {
		o.StorageParcels = make(ParcelAllocationMap, 0)
	}
	if o.Mounts == nil {
		o.Mounts = make([]Mount, 0)
	}
	mObj := &M.VolumeSeries{
		VolumeSeriesAllOf0: M.VolumeSeriesAllOf0{Meta: (&o.ObjMeta).ToModel("VolumeSeries")},
	}
	(&o.VolumeSeriesCreateOnceFields).ToModel(&mObj.VolumeSeriesCreateOnce)
	(&o.VolumeSeriesCreateMutableFields).ToModel(&mObj.VolumeSeriesCreateMutable)
	mObj.BoundClusterID = M.ObjIDMutable(o.BoundClusterID)
	mObj.BoundCspDomainID = M.ObjIDMutable(o.BoundCspDomainID)
	mObj.ConfiguredNodeID = M.ObjIDMutable(o.ConfiguredNodeID)
	mObj.RootStorageID = M.ObjIDMutable(o.RootStorageID)
	mObj.NuvoVolumeIdentifier = o.NuvoVolumeIdentifier
	mObj.RootParcelUUID = o.RootParcelUUID
	mObj.ServicePlanAllocationID = M.ObjIDMutable(o.ServicePlanAllocationID)
	mObj.VolumeSeriesState = o.VolumeSeriesState
	mObj.Messages = make([]*M.TimestampedString, len(o.Messages))
	for i, msg := range o.Messages {
		mObj.Messages[i] = msg.ToModel()
	}
	mObj.CacheAllocations = o.CacheAllocations.ToModel()
	mObj.CapacityAllocations = o.CapacityAllocations.ToModel()
	mObj.StorageParcels = o.StorageParcels.ToModel()
	mObj.Mounts = make([]*M.Mount, len(o.Mounts))
	for i, mount := range o.Mounts {
		mObj.Mounts[i] = mount.ToModel()
	}
	return mObj
}

// ToModel converts a datastore object to an existing model object
// Embedded object, non-standard conversion pattern.
func (o *VolumeSeriesCreateOnceFields) ToModel(mObj *M.VolumeSeriesCreateOnce) {
	mObj.AccountID = M.ObjIDMutable(o.AccountID)
	mObj.TenantAccountID = M.ObjIDMutable(o.TenantAccountID)
}

// ToModel converts a datastore object to an existing model object
// Embedded object, non-standard conversion pattern.
func (o *VolumeSeriesCreateMutableFields) ToModel(mObj *M.VolumeSeriesCreateMutable) {
	if o.Tags == nil {
		o.Tags = make(StringList, 0)
	}
	if o.SystemTags == nil {
		o.SystemTags = make(StringList, 0)
	}
	if o.ClusterDescriptor == nil {
		o.ClusterDescriptor = make(StringValueMap, 0)
	}
	mObj.Name = M.ObjName(o.Name)
	mObj.Description = M.ObjDescription(o.Description)
	mObj.SizeBytes = swag.Int64(o.SizeBytes)
	mObj.SpaAdditionalBytes = swag.Int64(o.SpaAdditionalBytes)
	mObj.ConsistencyGroupID = M.ObjIDMutable(o.ConsistencyGroupID)
	mObj.ServicePlanID = M.ObjIDMutable(o.ServicePlanID)
	mObj.LifecycleManagementData = o.LifecycleManagementData.ToModel()
	mObj.Tags = *o.Tags.ToModel()
	mObj.SystemTags = *o.SystemTags.ToModel()
	if len(o.ClusterDescriptor) == 0 {
		mObj.ClusterDescriptor = nil
	} else {
		mObj.ClusterDescriptor = o.ClusterDescriptor.ToModel()
	}
}

// FromModel converts a model object to a datastore object
func (o *VolumeSeries) FromModel(mObj *M.VolumeSeries) {
	mMeta := mObj.Meta
	if mMeta == nil {
		mMeta = &M.ObjMeta{ObjType: "VolumeSeries"}
	}
	(&o.ObjMeta).FromModel(mMeta)
	(&o.VolumeSeriesCreateOnceFields).FromModel(&mObj.VolumeSeriesCreateOnce)
	(&o.VolumeSeriesCreateMutableFields).FromModel(&mObj.VolumeSeriesCreateMutable)
	o.BoundClusterID = string(mObj.BoundClusterID)
	o.BoundCspDomainID = string(mObj.BoundCspDomainID)
	o.ConfiguredNodeID = string(mObj.ConfiguredNodeID)
	o.RootStorageID = string(mObj.RootStorageID)
	o.NuvoVolumeIdentifier = mObj.NuvoVolumeIdentifier
	o.RootParcelUUID = mObj.RootParcelUUID
	o.ServicePlanAllocationID = string(mObj.ServicePlanAllocationID)
	o.VolumeSeriesState = mObj.VolumeSeriesState
	if o.Messages == nil {
		o.Messages = make([]TimestampedString, len(mObj.Messages))
	}
	for i, msg := range mObj.Messages {
		o.Messages[i].FromModel(msg)
	}
	if o.CacheAllocations == nil {
		o.CacheAllocations = make(CacheAllocationMap, len(mObj.CacheAllocations))
	}
	o.CacheAllocations.FromModel(mObj.CacheAllocations)
	if o.CapacityAllocations == nil {
		o.CapacityAllocations = make(CapacityAllocationMap, len(mObj.CapacityAllocations))
	}
	o.CapacityAllocations.FromModel(mObj.CapacityAllocations)
	if o.StorageParcels == nil {
		o.StorageParcels = make(ParcelAllocationMap, len(mObj.StorageParcels))
	}
	o.StorageParcels.FromModel(mObj.StorageParcels)
	if o.Mounts == nil {
		o.Mounts = make([]Mount, len(mObj.Mounts))
	}
	for i, mount := range mObj.Mounts {
		o.Mounts[i].FromModel(mount)
	}
}

// FromModel converts a model object to a datastore object
func (o *VolumeSeriesCreateOnceFields) FromModel(mObj *M.VolumeSeriesCreateOnce) {
	o.AccountID = string(mObj.AccountID)
	o.TenantAccountID = string(mObj.TenantAccountID)
}

// FromModel converts a model object to a datastore object
func (o *VolumeSeriesCreateMutableFields) FromModel(mObj *M.VolumeSeriesCreateMutable) {
	o.Name = string(mObj.Name)
	o.Description = string(mObj.Description)
	o.SizeBytes = swag.Int64Value(mObj.SizeBytes)
	o.SpaAdditionalBytes = swag.Int64Value(mObj.SpaAdditionalBytes)
	o.ConsistencyGroupID = string(mObj.ConsistencyGroupID)
	o.ServicePlanID = string(mObj.ServicePlanID)
	o.LifecycleManagementData.FromModel(mObj.LifecycleManagementData)
	if o.Tags == nil {
		o.Tags = make(StringList, len(mObj.Tags))
	}
	o.Tags.FromModel(&mObj.Tags)
	if o.SystemTags == nil {
		o.SystemTags = make(StringList, len(mObj.SystemTags))
	}
	o.SystemTags.FromModel(&mObj.SystemTags)
	if mObj.ClusterDescriptor == nil {
		mObj.ClusterDescriptor = M.ClusterDescriptor{}
	}
	if o.ClusterDescriptor == nil {
		o.ClusterDescriptor = make(StringValueMap, len(mObj.ClusterDescriptor))
	}
	o.ClusterDescriptor.FromModel(mObj.ClusterDescriptor)
}

// volumeSeriesHandler is an ObjectDocumentHandler that implements the ds.VolumeSeriesOps operations
type volumeSeriesHandler struct {
	cName   string
	api     DBAPI
	crud    ObjectDocumentHandlerCRUD
	log     *logging.Logger
	indexes []mongo.IndexModel
}

var _ = ds.VolumeSeriesOps(&volumeSeriesHandler{})
var _ = ObjectDocumentHandler(&volumeSeriesHandler{})

var odhVolumeSeries = &volumeSeriesHandler{
	cName: "volumeseries",
	indexes: []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: accountIDKey, Value: IndexAscending}, {Key: nameKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: tenantAccountIDKey, Value: IndexAscending}}},
		{Keys: bsonx.Doc{{Key: consistencyGroupIDKey, Value: IndexAscending}}},
		{Keys: bsonx.Doc{{Key: servicePlanIDKey, Value: IndexAscending}}},
	},
}

func init() {
	odhRegister(odhVolumeSeries.cName, odhVolumeSeries)
}

// convertListParams is a helper to convert VolumeSeriesListParams to bson.M
func (op *volumeSeriesHandler) convertListParams(params volume_series.VolumeSeriesListParams) bson.M {
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
	if params.NamePattern != nil && *params.NamePattern != "" {
		qParams[nameKey] = primitive.Regex{Pattern: *params.NamePattern}
	}
	if params.Name != nil && *params.Name != "" {
		qParams[nameKey] = *params.Name
	}
	if len(params.Tags) > 0 {
		qParams["tags"] = bson.M{"$all": params.Tags}
	}
	if len(params.SystemTags) > 0 {
		qParams["systemtags"] = bson.M{"$all": params.SystemTags}
	}
	if params.BoundClusterID != nil && *params.BoundClusterID != "" {
		qParams["boundclusterid"] = *params.BoundClusterID
	}
	if params.BoundCspDomainID != nil && *params.BoundCspDomainID != "" {
		qParams["boundcspdomainid"] = *params.BoundCspDomainID
	}
	if params.ConfiguredNodeID != nil && *params.ConfiguredNodeID != "" {
		qParams["configurednodeid"] = *params.ConfiguredNodeID
	}
	if params.MountedNodeID != nil && *params.MountedNodeID != "" {
		qParams["mounts.mountednodeid"] = *params.MountedNodeID
	}
	if params.ServicePlanAllocationID != nil && *params.ServicePlanAllocationID != "" {
		qParams["serviceplanallocationid"] = *params.ServicePlanAllocationID
	}
	if params.ServicePlanID != nil && *params.ServicePlanID != "" {
		qParams[servicePlanIDKey] = *params.ServicePlanID
	}
	if params.StorageID != nil && *params.StorageID != "" {
		qParams["storageparcels."+*params.StorageID] = bson.M{"$exists": true}
	}
	if params.PoolID != nil && *params.PoolID != "" {
		qParams["capacityallocations."+*params.PoolID] = bson.M{"$exists": true}
	}
	if params.ConsistencyGroupID != nil && *params.ConsistencyGroupID != "" {
		qParams[consistencyGroupIDKey] = *params.ConsistencyGroupID
	}
	if len(params.VolumeSeriesState) > 0 {
		qParams["volumeseriesstate"] = bson.M{"$in": params.VolumeSeriesState}
	}
	if len(params.VolumeSeriesStateNot) > 0 {
		qParams["volumeseriesstate"] = bson.M{"$nin": params.VolumeSeriesStateNot}
	}
	if params.NextSnapshotTimeLE != nil {
		tLE := swag.TimeValue((*time.Time)(params.NextSnapshotTimeLE))
		qParams["lifecyclemanagementdata.nextsnapshottime"] = bson.M{"$lte": tLE}
	}
	orParams.setQueryParam(qParams)
	op.log.Info("qParams:", qParams)
	return qParams
}

// objectDocumentHandler methods

func (op *volumeSeriesHandler) Claim(api DBAPI, crud ObjectDocumentHandlerCRUD) {
	op.api = api
	op.crud = crud
	op.log = api.Logger()
}

func (op *volumeSeriesHandler) Initialize(ctx context.Context) error {
	return op.crud.CreateIndexes(ctx, op)
}

func (op *volumeSeriesHandler) Start(ctx context.Context) error {
	dbn := op.api.DBName()
	op.log.Infof("Starting collection %s.%s", dbn, op.cName)
	return nil
}

func (op *volumeSeriesHandler) Ops() interface{} {
	return op
}

func (op *volumeSeriesHandler) CName() string {
	return op.cName
}

func (op *volumeSeriesHandler) Indexes() []mongo.IndexModel {
	return op.indexes
}

func (op *volumeSeriesHandler) NewObject() interface{} {
	return &VolumeSeries{}
}

// ds.VolumeSeriesOps methods

// NewID returns an identifier for a future VolumeSeries object
func (op *volumeSeriesHandler) NewID() string {
	oMeta := &ObjMeta{}
	oMeta.FromModel(&M.ObjMeta{})
	return oMeta.MetaObjID
}

// Aggregate the matching documents.
// A list of aggregations the count of matching documents is returned on success.
func (op *volumeSeriesHandler) Aggregate(ctx context.Context, params volume_series.VolumeSeriesListParams) ([]*ds.Aggregation, int, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Aggregate:", err)
		return nil, 0, err
	}
	return op.crud.Aggregate(ctx, op, &params, op.convertListParams(params))
}

// Count the number of matching documents, limit is applied if non-zero
func (op *volumeSeriesHandler) Count(ctx context.Context, params volume_series.VolumeSeriesListParams, limit uint) (int, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Count:", err)
		return 0, err
	}
	return op.crud.Count(ctx, op, op.convertListParams(params), limit)
}

func (op *volumeSeriesHandler) Create(ctx context.Context, mObj *M.VolumeSeriesCreateArgs) (*M.VolumeSeries, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Create:", err)
		return nil, err
	}
	dObj := &VolumeSeries{}
	mVS := &M.VolumeSeries{
		VolumeSeriesAllOf0: M.VolumeSeriesAllOf0{
			RootParcelUUID: uuid.NewV4().String(),
		},
		VolumeSeriesCreateOnce: mObj.VolumeSeriesCreateOnce,
		VolumeSeriesMutable: M.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: M.VolumeSeriesMutableAllOf0{
				VolumeSeriesState: ds.DefaultVolumeSeriesState,
			},
			VolumeSeriesCreateMutable: mObj.VolumeSeriesCreateMutable,
		},
	}
	dObj.FromModel(mVS)
	if len(dObj.SystemTags) > 0 {
		sTags := util.NewTagList(dObj.SystemTags)
		if id, ok := sTags.Get(common.SystemTagVolumeSeriesID); ok {
			dObj.MetaObjID = id // use specified identifier instead
			sTags.Delete(common.SystemTagVolumeSeriesID)
			dObj.SystemTags = sTags.List()
		}
	}
	if err := op.crud.InsertOne(ctx, op, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *volumeSeriesHandler) Delete(ctx context.Context, mID string) error {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Delete:", err)
		return err
	}
	return op.crud.DeleteOne(ctx, op, mID)
}

func (op *volumeSeriesHandler) Fetch(ctx context.Context, mID string) (*M.VolumeSeries, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Fetch:", err)
		return nil, err
	}
	dObj := &VolumeSeries{}
	if err := op.crud.FindOne(ctx, op, bson.M{objKey: mID}, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *volumeSeriesHandler) List(ctx context.Context, params volume_series.VolumeSeriesListParams) ([]*M.VolumeSeries, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("List:", err)
		return nil, err
	}
	list := make([]*M.VolumeSeries, 0, 10)
	err := op.crud.FindAll(ctx, op, params, op.convertListParams(params), func(obj interface{}) {
		dObj := obj.(*VolumeSeries) // or panic
		list = append(list, dObj.ToModel())
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (op *volumeSeriesHandler) Update(ctx context.Context, ua *ds.UpdateArgs, param *M.VolumeSeriesMutable) (*M.VolumeSeries, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Update:", err)
		return nil, err
	}
	mObj := &M.VolumeSeries{VolumeSeriesMutable: *param}
	obj := &VolumeSeries{}
	obj.FromModel(mObj)
	if err := op.crud.UpdateOne(ctx, op, obj, ua); err != nil {
		return nil, err
	}
	return obj.ToModel(), nil
}
