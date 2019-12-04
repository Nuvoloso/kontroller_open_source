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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/service_plan_allocation"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/go-openapi/swag"
	"github.com/op/go-logging"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// ServicePlanAllocation object
type ServicePlanAllocation struct {
	ObjMeta                            `bson:",inline"`
	CspDomainID                        string
	ReservableCapacityBytes            int64
	ServicePlanAllocationCreateOnce    `bson:",inline"`
	ServicePlanAllocationCreateMutable `bson:",inline"`
	ClusterDescriptor                  StringValueMap
	ChargedCostPerGiB                  float64
}

// ServicePlanAllocationCreateOnce reflects the model object of the same name
// It is defined separately so it can also be referenced in VolumeSeriesRequest.
type ServicePlanAllocationCreateOnce struct {
	AccountID           string
	AuthorizedAccountID string
	ClusterID           string
	ServicePlanID       string
}

// ServicePlanAllocationCreateMutable reflects the model object of the same name
// It is defined separately so it can also be referenced in VolumeSeriesRequest.
type ServicePlanAllocationCreateMutable struct {
	Messages            []TimestampedString
	ProvisioningHints   StringValueMap
	ReservationState    string
	StorageFormula      string
	StorageReservations StorageTypeReservationMap
	SystemTags          StringList
	Tags                StringList
	TotalCapacityBytes  int64
}

// ToModel converts a datastore object to a model object
func (o *ServicePlanAllocation) ToModel() *M.ServicePlanAllocation {
	mObj := &M.ServicePlanAllocation{
		ServicePlanAllocationAllOf0: M.ServicePlanAllocationAllOf0{Meta: (&o.ObjMeta).ToModel("ServicePlanAllocation")},
	}
	(&o.ServicePlanAllocationCreateOnce).ToModel(&mObj.ServicePlanAllocationCreateOnce)
	(&o.ServicePlanAllocationCreateMutable).ToModel(&mObj.ServicePlanAllocationCreateMutable)
	mObj.CspDomainID = M.ObjIDMutable(o.CspDomainID)
	mObj.ReservableCapacityBytes = swag.Int64(o.ReservableCapacityBytes)
	mObj.ChargedCostPerGiB = o.ChargedCostPerGiB
	if o.ClusterDescriptor == nil {
		o.ClusterDescriptor = make(StringValueMap, 0)
	}
	if len(o.ClusterDescriptor) == 0 {
		mObj.ClusterDescriptor = nil
	} else {
		mObj.ClusterDescriptor = o.ClusterDescriptor.ToModel()
	}
	return mObj
}

// ToModel converts a datastore object to a model object
// Embedded object, non-standard conversion pattern.
func (o *ServicePlanAllocationCreateOnce) ToModel(mObj *M.ServicePlanAllocationCreateOnce) {
	mObj.AccountID = M.ObjIDMutable(o.AccountID)
	mObj.AuthorizedAccountID = M.ObjIDMutable(o.AuthorizedAccountID)
	mObj.ClusterID = M.ObjIDMutable(o.ClusterID)
	mObj.ServicePlanID = M.ObjIDMutable(o.ServicePlanID)
}

// ToModel converts a datastore object to a model object
// Embedded object, non-standard conversion pattern.
func (o *ServicePlanAllocationCreateMutable) ToModel(mObj *M.ServicePlanAllocationCreateMutable) {
	if o.Messages == nil {
		o.Messages = make([]TimestampedString, 0)
	}
	if o.ProvisioningHints == nil {
		o.ProvisioningHints = make(StringValueMap, 0)
	}
	if o.ReservationState == "" {
		o.ReservationState = ds.DefaultServicePlanAllocationReservationState
	}
	if o.StorageReservations == nil {
		o.StorageReservations = make(StorageTypeReservationMap, 0)
	}
	if o.Tags == nil {
		o.Tags = make(StringList, 0)
	}
	if o.SystemTags == nil {
		o.SystemTags = make(StringList, 0)
	}
	mObj.Messages = make([]*M.TimestampedString, len(o.Messages))
	for i, msg := range o.Messages {
		mObj.Messages[i] = msg.ToModel()
	}
	mObj.ProvisioningHints = o.ProvisioningHints.ToModel()
	mObj.ReservationState = o.ReservationState
	mObj.StorageFormula = M.StorageFormulaName(o.StorageFormula)
	mObj.StorageReservations = o.StorageReservations.ToModel()
	mObj.Tags = *o.Tags.ToModel()
	mObj.SystemTags = *o.SystemTags.ToModel()
	mObj.TotalCapacityBytes = swag.Int64(o.TotalCapacityBytes)
}

// FromModel converts a model object to a datastore object
func (o *ServicePlanAllocation) FromModel(mObj *M.ServicePlanAllocation) {
	mMeta := mObj.Meta
	if mMeta == nil {
		mMeta = &M.ObjMeta{ObjType: "ServicePlanAllocation"}
	}
	(&o.ObjMeta).FromModel(mMeta)
	o.CspDomainID = string(mObj.CspDomainID)
	o.ReservableCapacityBytes = swag.Int64Value(mObj.ReservableCapacityBytes)
	o.ChargedCostPerGiB = mObj.ChargedCostPerGiB
	o.ServicePlanAllocationCreateOnce.FromModel(&mObj.ServicePlanAllocationCreateOnce)
	o.ServicePlanAllocationCreateMutable.FromModel(&mObj.ServicePlanAllocationCreateMutable)
	if mObj.ClusterDescriptor == nil {
		mObj.ClusterDescriptor = M.ClusterDescriptor{}
	}
	if o.ClusterDescriptor == nil {
		o.ClusterDescriptor = make(StringValueMap, len(mObj.ClusterDescriptor))
	}
	o.ClusterDescriptor.FromModel(mObj.ClusterDescriptor)
}

// FromModel converts a model object to a datastore object
func (o *ServicePlanAllocationCreateOnce) FromModel(mObj *M.ServicePlanAllocationCreateOnce) {
	o.AccountID = string(mObj.AccountID)
	o.AuthorizedAccountID = string(mObj.AuthorizedAccountID)
	o.ClusterID = string(mObj.ClusterID)
	o.ServicePlanID = string(mObj.ServicePlanID)
}

// FromModel converts a model object to a datastore object
func (o *ServicePlanAllocationCreateMutable) FromModel(mObj *M.ServicePlanAllocationCreateMutable) {
	if o.Messages == nil {
		o.Messages = make([]TimestampedString, len(mObj.Messages))
	}
	for i, msg := range mObj.Messages {
		o.Messages[i].FromModel(msg)
	}
	if o.ProvisioningHints == nil {
		o.ProvisioningHints = make(StringValueMap, len(mObj.ProvisioningHints))
	}
	o.ProvisioningHints.FromModel(mObj.ProvisioningHints)
	o.ReservationState = mObj.ReservationState
	o.StorageFormula = string(mObj.StorageFormula)
	if o.StorageReservations == nil {
		o.StorageReservations = make(StorageTypeReservationMap, len(mObj.StorageReservations))
	}
	o.StorageReservations.FromModel(mObj.StorageReservations)
	if o.SystemTags == nil {
		o.SystemTags = make(StringList, len(mObj.SystemTags))
	}
	o.SystemTags.FromModel(&mObj.SystemTags)
	if o.Tags == nil {
		o.Tags = make(StringList, len(mObj.Tags))
	}
	o.Tags.FromModel(&mObj.Tags)
	o.TotalCapacityBytes = swag.Int64Value(mObj.TotalCapacityBytes)
}

// servicePlanAllocationHandler is an ObjectDocumentHandler that implements the ds.ServicePlanAllocationOps operations
type servicePlanAllocationHandler struct {
	cName   string
	api     DBAPI
	crud    ObjectDocumentHandlerCRUD
	log     *logging.Logger
	indexes []mongo.IndexModel
}

var _ = ds.ServicePlanAllocationOps(&servicePlanAllocationHandler{})
var _ = ObjectDocumentHandler(&servicePlanAllocationHandler{})

var odhServicePlanAllocation = &servicePlanAllocationHandler{
	cName: "serviceplanallocation",
	indexes: []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: authorizedAccountIDKey, Value: IndexAscending}, {Key: servicePlanIDKey, Value: IndexAscending}, {Key: clusterIDKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
	},
}

func init() {
	odhRegister(odhServicePlanAllocation.cName, odhServicePlanAllocation)
}

// convertListParams is a helper to convert ServicePlanAllocationListParams to bson.M
func (op *servicePlanAllocationHandler) convertListParams(params service_plan_allocation.ServicePlanAllocationListParams) bson.M {
	qParams := bson.M{}
	if params.AccountID != nil && *params.AccountID != "" {
		qParams["accountid"] = *params.AccountID
	}
	if params.AuthorizedAccountID != nil && *params.AuthorizedAccountID != "" {
		qParams[authorizedAccountIDKey] = *params.AuthorizedAccountID
	}
	if params.ClusterID != nil && *params.ClusterID != "" {
		qParams[clusterIDKey] = *params.ClusterID
	}
	if params.CspDomainID != nil && *params.CspDomainID != "" {
		qParams["cspdomainid"] = *params.CspDomainID
	}
	if len(params.ReservationState) > 0 {
		qParams["reservationstate"] = bson.M{"$in": params.ReservationState}
	}
	if len(params.ReservationStateNot) > 0 {
		qParams["reservationstate"] = bson.M{"$nin": params.ReservationStateNot}
	}
	if params.ServicePlanID != nil && *params.ServicePlanID != "" {
		qParams[servicePlanIDKey] = *params.ServicePlanID
	}
	if params.StorageFormulaName != nil && *params.StorageFormulaName != "" {
		qParams["storageformulaname"] = *params.StorageFormulaName
	}
	if params.PoolID != nil && *params.PoolID != "" {
		qParams["storagereservations."+*params.PoolID] = bson.M{"$exists": true}
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

func (op *servicePlanAllocationHandler) Claim(api DBAPI, crud ObjectDocumentHandlerCRUD) {
	op.api = api
	op.crud = crud
	op.log = api.Logger()
}

func (op *servicePlanAllocationHandler) Initialize(ctx context.Context) error {
	return op.crud.CreateIndexes(ctx, op)
}

func (op *servicePlanAllocationHandler) Start(ctx context.Context) error {
	dbn := op.api.DBName()
	op.log.Infof("Starting collection %s.%s", dbn, op.cName)
	return nil
}

func (op *servicePlanAllocationHandler) Ops() interface{} {
	return op
}

func (op *servicePlanAllocationHandler) CName() string {
	return op.cName
}

func (op *servicePlanAllocationHandler) Indexes() []mongo.IndexModel {
	return op.indexes
}

func (op *servicePlanAllocationHandler) NewObject() interface{} {
	return &ServicePlanAllocation{}
}

// ds.ServicePlanAllocationOps methods

// Count the number of matching documents, limit is applied if non-zero
func (op *servicePlanAllocationHandler) Count(ctx context.Context, params service_plan_allocation.ServicePlanAllocationListParams, limit uint) (int, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Count:", err)
		return 0, err
	}
	return op.crud.Count(ctx, op, op.convertListParams(params), limit)
}

func (op *servicePlanAllocationHandler) Create(ctx context.Context, mObj *M.ServicePlanAllocation) (*M.ServicePlanAllocation, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Create:", err)
		return nil, err
	}
	mObj.Meta = nil // ensure that meta will be put into correct creation state by FromModel
	dObj := &ServicePlanAllocation{}
	dObj.FromModel(mObj)
	if err := op.crud.InsertOne(ctx, op, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *servicePlanAllocationHandler) Delete(ctx context.Context, mID string) error {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Delete:", err)
		return err
	}
	return op.crud.DeleteOne(ctx, op, mID)
}

func (op *servicePlanAllocationHandler) Fetch(ctx context.Context, mID string) (*M.ServicePlanAllocation, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Fetch:", err)
		return nil, err
	}
	dObj := &ServicePlanAllocation{}
	if err := op.crud.FindOne(ctx, op, bson.M{objKey: mID}, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *servicePlanAllocationHandler) List(ctx context.Context, params service_plan_allocation.ServicePlanAllocationListParams) ([]*M.ServicePlanAllocation, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("List:", err)
		return nil, err
	}
	list := make([]*M.ServicePlanAllocation, 0, 10)
	err := op.crud.FindAll(ctx, op, params, op.convertListParams(params), func(obj interface{}) {
		dObj := obj.(*ServicePlanAllocation) // or panic
		list = append(list, dObj.ToModel())
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (op *servicePlanAllocationHandler) Update(ctx context.Context, ua *ds.UpdateArgs, param *M.ServicePlanAllocationMutable) (*M.ServicePlanAllocation, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Update:", err)
		return nil, err
	}
	mObj := &M.ServicePlanAllocation{ServicePlanAllocationMutable: *param}
	obj := &ServicePlanAllocation{}
	obj.FromModel(mObj)
	if err := op.crud.UpdateOne(ctx, op, obj, ua); err != nil {
		return nil, err
	}
	return obj.ToModel(), nil
}
