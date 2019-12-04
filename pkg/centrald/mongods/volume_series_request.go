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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
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

// VolumeSeriesRequest object
type VolumeSeriesRequest struct {
	ObjMeta                         `bson:",inline"`
	Creator                         Identity
	RequestedOperations             []string
	CancelRequested                 bool
	PlanOnly                        bool
	ApplicationGroupIds             ObjIDList
	ClusterID                       string
	ConsistencyGroupID              string
	NodeID                          string
	MountedNodeDevice               string
	VolumeSeriesID                  string
	ServicePlanAllocationID         string
	SnapIdentifier                  string
	SyncCoordinatorID               string
	SyncPeers                       SyncPeerMap
	CompleteByTime                  time.Time
	ServicePlanAllocationCreateSpec ServicePlanAllocationCreateSpec
	VolumeSeriesCreateSpec          VolumeSeriesCreateSpec
	Snapshot                        SnapshotData
	SnapshotID                      string
	Progress                        Progress
	LifecycleManagementData         LifecycleManagementData
	StorageFormula                  string
	CapacityReservationPlan         CapacityReservationPlan
	CapacityReservationResult       CapacityReservationResult
	StoragePlan                     StoragePlan
	VolumeSeriesRequestState        string
	RequestMessages                 []TimestampedString
	SystemTags                      StringList
	FsType                          string
	DriverType                      string
	TargetPath                      string
	ReadOnly                        bool
	ProtectionDomainID              string
	Terminated                      bool // derived field, not exposed in the model
}

// ServicePlanAllocationCreateSpec reflects the model ServicePlanAllocationCreateArgs type
type ServicePlanAllocationCreateSpec struct {
	ServicePlanAllocationCreateOnce    `bson:",inline"`
	ServicePlanAllocationCreateMutable `bson:",inline"`
}

// VolumeSeriesCreateSpec reflects the model VolumeSeriesCreateArgs type
// It preserves the specificity of some optional fields (see converters)
type VolumeSeriesCreateSpec struct {
	VolumeSeriesCreateOnceFields    `bson:",inline"`
	VolumeSeriesCreateMutableFields `bson:",inline"`
}

var terminalVSRStates = []string{com.VolReqStateSucceeded, com.VolReqStateFailed, com.VolReqStateCanceled}

// ToModel converts a datastore object to a model object
func (o *VolumeSeriesRequest) ToModel() *M.VolumeSeriesRequest {
	if o.ApplicationGroupIds == nil {
		o.ApplicationGroupIds = make(ObjIDList, 0)
	}
	if o.RequestedOperations == nil {
		o.RequestedOperations = make([]string, 0)
	}
	if o.RequestMessages == nil {
		o.RequestMessages = make([]TimestampedString, 0)
	}
	if o.SystemTags == nil {
		o.SystemTags = make(StringList, 0)
	}
	mObj := &M.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: M.VolumeSeriesRequestAllOf0{Meta: (&o.ObjMeta).ToModel("VolumeSeriesRequest")},
	}
	mObj.RequestedOperations = make([]string, len(o.RequestedOperations))
	for i, s := range o.RequestedOperations {
		mObj.RequestedOperations[i] = s
	}
	mObj.CancelRequested = o.CancelRequested
	mObj.Creator = o.Creator.ToModel()
	mObj.PlanOnly = swag.Bool(o.PlanOnly)
	mObj.ApplicationGroupIds = *o.ApplicationGroupIds.ToModel()
	mObj.ClusterID = M.ObjIDMutable(o.ClusterID)
	mObj.ConsistencyGroupID = M.ObjIDMutable(o.ConsistencyGroupID)
	mObj.NodeID = M.ObjIDMutable(o.NodeID)
	mObj.MountedNodeDevice = o.MountedNodeDevice
	mObj.VolumeSeriesID = M.ObjIDMutable(o.VolumeSeriesID)
	mObj.SnapIdentifier = o.SnapIdentifier
	mObj.SyncCoordinatorID = M.ObjIDMutable(o.SyncCoordinatorID)
	mObj.SyncPeers = o.SyncPeers.ToModel()
	mObj.ServicePlanAllocationID = M.ObjIDMutable(o.ServicePlanAllocationID)
	mObj.CompleteByTime = strfmt.DateTime(o.CompleteByTime)
	mObj.ServicePlanAllocationCreateSpec = &M.ServicePlanAllocationCreateArgs{}
	(&o.ServicePlanAllocationCreateSpec.ServicePlanAllocationCreateOnce).ToModel(&mObj.ServicePlanAllocationCreateSpec.ServicePlanAllocationCreateOnce)
	(&o.ServicePlanAllocationCreateSpec.ServicePlanAllocationCreateMutable).ToModel(&mObj.ServicePlanAllocationCreateSpec.ServicePlanAllocationCreateMutable)
	mObj.VolumeSeriesCreateSpec = o.VolumeSeriesCreateSpec.ToModel()
	snapshot := o.Snapshot.ToModel()
	mObj.Snapshot = &snapshot
	mObj.SnapshotID = M.ObjIDMutable(o.SnapshotID)
	mObj.Progress = o.Progress.ToModel()
	mObj.LifecycleManagementData = o.LifecycleManagementData.ToModel()
	mObj.SystemTags = *o.SystemTags.ToModel()
	mObj.StorageFormula = M.StorageFormulaName(o.StorageFormula)
	mObj.CapacityReservationPlan = o.CapacityReservationPlan.ToModel()
	mObj.CapacityReservationResult = o.CapacityReservationResult.ToModel()
	mObj.StoragePlan = o.StoragePlan.ToModel()
	mObj.VolumeSeriesRequestState = o.VolumeSeriesRequestState
	mObj.RequestMessages = make([]*M.TimestampedString, len(o.RequestMessages))
	for i, msg := range o.RequestMessages {
		mObj.RequestMessages[i] = msg.ToModel()
	}
	mObj.FsType = o.FsType
	mObj.DriverType = o.DriverType
	mObj.TargetPath = o.TargetPath
	mObj.ReadOnly = o.ReadOnly
	mObj.ProtectionDomainID = M.ObjIDMutable(o.ProtectionDomainID)
	return mObj
}

// FromModel converts a model object to a datastore object
func (o *VolumeSeriesRequest) FromModel(mObj *M.VolumeSeriesRequest) {
	mMeta := mObj.Meta
	if mMeta == nil {
		mMeta = &M.ObjMeta{ObjType: "VolumeSeriesRequest"}
	}
	(&o.ObjMeta).FromModel(mMeta)
	if o.RequestedOperations == nil {
		o.RequestedOperations = make([]string, len(mObj.RequestedOperations))
	}
	for i, s := range mObj.RequestedOperations {
		o.RequestedOperations[i] = s
	}
	o.CancelRequested = mObj.CancelRequested
	creator := mObj.Creator
	if creator == nil {
		creator = &M.Identity{}
	}
	o.Creator.FromModel(creator)
	o.PlanOnly = swag.BoolValue(mObj.PlanOnly)
	if o.ApplicationGroupIds == nil {
		o.ApplicationGroupIds = make(ObjIDList, len(mObj.ApplicationGroupIds))
	}
	o.ApplicationGroupIds.FromModel(&mObj.ApplicationGroupIds)
	o.ClusterID = string(mObj.ClusterID)
	o.ConsistencyGroupID = string(mObj.ConsistencyGroupID)
	o.NodeID = string(mObj.NodeID)
	o.MountedNodeDevice = mObj.MountedNodeDevice
	o.VolumeSeriesID = string(mObj.VolumeSeriesID)
	o.SnapIdentifier = mObj.SnapIdentifier
	o.SyncCoordinatorID = string(mObj.SyncCoordinatorID)
	if o.SyncPeers == nil {
		o.SyncPeers = make(map[string]SyncPeer, len(mObj.SyncPeers))
	}
	o.SyncPeers.FromModel(mObj.SyncPeers)
	o.ServicePlanAllocationID = string(mObj.ServicePlanAllocationID)
	o.CompleteByTime = time.Time(mObj.CompleteByTime)
	spaCS := mObj.ServicePlanAllocationCreateSpec
	if spaCS == nil {
		spaCS = &M.ServicePlanAllocationCreateArgs{}
	}
	o.ServicePlanAllocationCreateSpec.ServicePlanAllocationCreateOnce.FromModel(&spaCS.ServicePlanAllocationCreateOnce)
	o.ServicePlanAllocationCreateSpec.ServicePlanAllocationCreateMutable.FromModel(&spaCS.ServicePlanAllocationCreateMutable)
	vsCS := mObj.VolumeSeriesCreateSpec
	if vsCS == nil {
		vsCS = &M.VolumeSeriesCreateArgs{}
	}
	o.VolumeSeriesCreateSpec.FromModel(vsCS)
	snapshot := mObj.Snapshot
	if snapshot == nil {
		snapshot = &M.SnapshotData{}
	}
	o.Snapshot.FromModel(snapshot)
	o.SnapshotID = string(mObj.SnapshotID)
	o.Progress.FromModel(mObj.Progress)
	o.LifecycleManagementData.FromModel(mObj.LifecycleManagementData)
	if o.SystemTags == nil {
		o.SystemTags = make(StringList, len(mObj.SystemTags))
	}
	o.SystemTags.FromModel(&mObj.SystemTags)
	o.StorageFormula = string(mObj.StorageFormula)
	cr := mObj.CapacityReservationPlan
	if cr == nil {
		cr = &M.CapacityReservationPlan{}
	}
	o.CapacityReservationPlan.FromModel(cr)
	crr := mObj.CapacityReservationResult
	if crr == nil {
		crr = &M.CapacityReservationResult{}
	}
	o.CapacityReservationResult.FromModel(crr)
	sp := mObj.StoragePlan
	if sp == nil {
		sp = &M.StoragePlan{}
	}
	o.StoragePlan.FromModel(sp)
	o.VolumeSeriesRequestState = mObj.VolumeSeriesRequestState
	o.Terminated = util.Contains(terminalVSRStates, o.VolumeSeriesRequestState)
	if o.RequestMessages == nil {
		o.RequestMessages = make([]TimestampedString, len(mObj.RequestMessages))
	}
	for i, msg := range mObj.RequestMessages {
		o.RequestMessages[i].FromModel(msg)
	}
	o.FsType = string(mObj.FsType)
	o.DriverType = string(mObj.DriverType)
	o.TargetPath = string(mObj.TargetPath)
	o.ReadOnly = mObj.ReadOnly
	o.ProtectionDomainID = string(mObj.ProtectionDomainID)
}

// ToModel converts a datastore object to a model object
func (o *VolumeSeriesCreateSpec) ToModel() *M.VolumeSeriesCreateArgs {
	mObj := &M.VolumeSeriesCreateArgs{}
	(&o.VolumeSeriesCreateOnceFields).ToModel(&mObj.VolumeSeriesCreateOnce)
	(&o.VolumeSeriesCreateMutableFields).ToModel(&mObj.VolumeSeriesCreateMutable)
	if swag.Int64Value(mObj.SizeBytes) < 0 {
		mObj.SizeBytes = nil // was not set on input
	}
	if swag.Int64Value(mObj.SpaAdditionalBytes) < 0 {
		mObj.SpaAdditionalBytes = nil // was not set on input
	}
	return mObj
}

// FromModel converts a model object to a datastore object
func (o *VolumeSeriesCreateSpec) FromModel(mObj *M.VolumeSeriesCreateArgs) {
	o.VolumeSeriesCreateOnceFields.FromModel(&mObj.VolumeSeriesCreateOnce)
	o.VolumeSeriesCreateMutableFields.FromModel(&mObj.VolumeSeriesCreateMutable)
	if mObj.SizeBytes == nil {
		o.SizeBytes = -1 // never negative on input
	}
	if mObj.SpaAdditionalBytes == nil {
		o.SpaAdditionalBytes = -1 // never negative on input
	}
}

// volumeSeriesRequestHandler is an ObjectDocumentHandler that implements the ds.VolumeSeriesRequestOps operations
type volumeSeriesRequestHandler struct {
	cName   string
	api     DBAPI
	crud    ObjectDocumentHandlerCRUD
	log     *logging.Logger
	indexes []mongo.IndexModel
}

var _ = ds.VolumeSeriesRequestOps(&volumeSeriesRequestHandler{})
var _ = ObjectDocumentHandler(&volumeSeriesRequestHandler{})

var odhVolumeSeriesRequest = &volumeSeriesRequestHandler{
	cName: "volumeseriesrequest",
	indexes: []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: clusterIDKey, Value: IndexAscending}}},
		{Keys: bsonx.Doc{{Key: nodeIDKey, Value: IndexAscending}}},
		{Keys: bsonx.Doc{{Key: terminatedKey, Value: IndexAscending}}, Options: ActiveRequestsOnlyIndex},
	},
}

func init() {
	odhRegister(odhVolumeSeriesRequest.cName, odhVolumeSeriesRequest)
}

// convertListParams is a helper to convert VolumeSeriesRequestListParams to bson.M
func (op *volumeSeriesRequestHandler) convertListParams(params volume_series_request.VolumeSeriesRequestListParams) bson.M {
	qParams := bson.M{}
	orParams := newAndOrList()
	if params.AccountID != nil && *params.AccountID != "" {
		if params.TenantAccountID != nil && *params.TenantAccountID != "" {
			orParams.append(
				bson.M{"volumeseriescreatespec.accountid": *params.AccountID},
				bson.M{"volumeseriescreatespec.tenantaccountid": *params.TenantAccountID},
			)
		} else {
			qParams["volumeseriescreatespec.accountid"] = *params.AccountID
		}
	} else if params.TenantAccountID != nil && *params.TenantAccountID != "" {
		qParams["volumeseriescreatespec.tenantaccountid"] = *params.TenantAccountID
	}
	if v := swag.StringValue(params.ClusterID); v != "" {
		qParams["clusterid"] = v
	}
	if v := swag.StringValue(params.ConsistencyGroupID); v != "" {
		qParams["consistencygroupid"] = v
	}
	if v := swag.StringValue(params.NodeID); v != "" {
		qParams["nodeid"] = v
	}
	if params.ProtectionDomainID != nil {
		qParams["protectiondomainid"] = *params.ProtectionDomainID
	}
	if len(params.RequestedOperations) > 0 {
		qParams["requestedoperations"] = bson.M{"$in": params.RequestedOperations}
	}
	if len(params.RequestedOperationsNot) > 0 {
		qParams["requestedoperations"] = bson.M{"$nin": params.RequestedOperationsNot}
	}
	if v := swag.StringValue(params.ServicePlanID); v != "" {
		qParams["serviceplanid"] = v
	}
	if v := swag.StringValue(params.ServicePlanAllocationID); v != "" {
		qParams["serviceplanallocationid"] = v
	}
	if v := swag.StringValue(params.StorageID); v != "" {
		qParams["storageplan.storageelements.storageparcels."+*params.StorageID] = bson.M{"$exists": true}
	}
	if v := swag.StringValue(params.PoolID); v != "" {
		orParams.append(
			bson.M{"capacityreservationresult.currentreservations." + *params.PoolID: bson.M{"$exists": true}},
			bson.M{"capacityreservationresult.desiredreservations." + *params.PoolID: bson.M{"$exists": true}},
			// bson.M{"serviceplanallocationcreatespec.storagereservations." + *params.PoolID: bson.M{"$exists": true}}, // internal only
			bson.M{"storageplan.storageelements.poolid": *params.PoolID},
		)
	}
	if v := swag.StringValue(params.VolumeSeriesID); v != "" {
		qParams["volumeseriesid"] = v
	}
	if v := swag.StringValue(params.SnapshotID); v != "" {
		qParams["snapshotid"] = v
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
		} else if len(params.VolumeSeriesRequestState) > 0 {
			activeOnly := true
			for _, state := range params.VolumeSeriesRequestState {
				if util.Contains(terminalVSRStates, state) {
					activeOnly = false
					break
				}
			}
			if activeOnly {
				qParams[terminatedKey] = false
			}
			qParams["volumeseriesrequeststate"] = bson.M{"$in": params.VolumeSeriesRequestState}
		}
	}
	if v := swag.StringValue(params.SyncCoordinatorID); v != "" {
		qParams["synccoordinatorid"] = v
	}
	if params.SystemTags != nil {
		qParams["systemtags"] = bson.M{"$all": params.SystemTags}
	}
	orParams.setQueryParam(qParams)
	op.log.Info("qParams:", qParams)
	return qParams
}

// objectDocumentHandler methods

func (op *volumeSeriesRequestHandler) Claim(api DBAPI, crud ObjectDocumentHandlerCRUD) {
	op.api = api
	op.crud = crud
	op.log = api.Logger()
}

func (op *volumeSeriesRequestHandler) Initialize(ctx context.Context) error {
	return op.crud.CreateIndexes(ctx, op)
}

func (op *volumeSeriesRequestHandler) Start(ctx context.Context) error {
	dbn := op.api.DBName()
	op.log.Infof("Starting collection %s.%s", dbn, op.cName)
	return nil
}

func (op *volumeSeriesRequestHandler) Ops() interface{} {
	return op
}

func (op *volumeSeriesRequestHandler) CName() string {
	return op.cName
}

func (op *volumeSeriesRequestHandler) Indexes() []mongo.IndexModel {
	return op.indexes
}

func (op *volumeSeriesRequestHandler) NewObject() interface{} {
	return &VolumeSeriesRequest{}
}

// ds.VolumeSeriesRequestOps methods

// Cancel sets the cancelRequested flag on a VolumeSeriesRequest object, assuming all checks were made in the handler
func (op *volumeSeriesRequestHandler) Cancel(ctx context.Context, id string, version int32) (*M.VolumeSeriesRequest, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Cancel:", err)
		return nil, err
	}
	ua := &ds.UpdateArgs{
		ID:      id,
		Version: version,
		Attributes: []ds.UpdateAttr{
			{Name: "CancelRequested", Actions: [ds.NumActionTypes]ds.UpdateActionArgs{ds.UpdateSet: ds.UpdateActionArgs{FromBody: true}}},
		},
	}
	dObj := &VolumeSeriesRequest{CancelRequested: true}
	if err := op.crud.UpdateOne(ctx, op, dObj, ua); err != nil {
		op.log.Errorf("VolumeSeriesRequest(%s, %d) cancel: %s", id, version, err)
		return nil, err
	}
	return dObj.ToModel(), nil
}

// Count the number of matching documents, limit is applied if non-zero
func (op *volumeSeriesRequestHandler) Count(ctx context.Context, params volume_series_request.VolumeSeriesRequestListParams, limit uint) (int, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Count:", err)
		return 0, err
	}
	return op.crud.Count(ctx, op, op.convertListParams(params), limit)
}

// Create creates the VolumeSeriesRequest object
// Note: Validations are performed by the handler. Take care when calling internally.
func (op *volumeSeriesRequestHandler) Create(ctx context.Context, mObj *M.VolumeSeriesRequestCreateArgs) (*M.VolumeSeriesRequest, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Create:", err)
		return nil, err
	}
	dObj := &VolumeSeriesRequest{}
	mVSR := &M.VolumeSeriesRequest{
		VolumeSeriesRequestCreateOnce: mObj.VolumeSeriesRequestCreateOnce,
		VolumeSeriesRequestMutable: M.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: M.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: ds.DefaultVolumeSeriesRequestState,
			},
			VolumeSeriesRequestCreateMutable: mObj.VolumeSeriesRequestCreateMutable,
		},
	}
	dObj.FromModel(mVSR)
	if err := op.crud.InsertOne(ctx, op, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *volumeSeriesRequestHandler) Delete(ctx context.Context, mID string) error {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Delete:", err)
		return err
	}
	return op.crud.DeleteOne(ctx, op, mID)
}

func (op *volumeSeriesRequestHandler) Fetch(ctx context.Context, mID string) (*M.VolumeSeriesRequest, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Fetch:", err)
		return nil, err
	}
	dObj := &VolumeSeriesRequest{}
	if err := op.crud.FindOne(ctx, op, bson.M{objKey: mID}, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *volumeSeriesRequestHandler) List(ctx context.Context, params volume_series_request.VolumeSeriesRequestListParams) ([]*M.VolumeSeriesRequest, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("List:", err)
		return nil, err
	}
	list := make([]*M.VolumeSeriesRequest, 0, 10)
	err := op.crud.FindAll(ctx, op, params, op.convertListParams(params), func(obj interface{}) {
		dObj := obj.(*VolumeSeriesRequest) // or panic
		list = append(list, dObj.ToModel())
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (op *volumeSeriesRequestHandler) Update(ctx context.Context, ua *ds.UpdateArgs, param *M.VolumeSeriesRequestMutable) (*M.VolumeSeriesRequest, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Update:", err)
		return nil, err
	}
	mObj := &M.VolumeSeriesRequest{VolumeSeriesRequestMutable: *param}
	obj := &VolumeSeriesRequest{}
	obj.FromModel(mObj)
	if ua.IsModified("VolumeSeriesRequestState") {
		// Terminated field gets auto-updated on VolumeSeriesRequestState change
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
