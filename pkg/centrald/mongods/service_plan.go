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
	"encoding/json"
	"errors"

	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/service_plan"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/go-openapi/swag"
	"github.com/op/go-logging"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// ServicePlan object
type ServicePlan struct {
	ObjMeta                `bson:",inline"`
	Name                   string
	Description            string
	Tags                   StringList
	SourceServicePlanID    string
	State                  string
	IoProfile              IoProfile
	ProvisioningUnit       ProvisioningUnit
	VolumeSeriesMinMaxSize VolumeSeriesMinMaxSize
	Accounts               ObjIDList
	Slos                   RestrictedStringValueMap
}

// ToModel converts a datastore object to a model object
func (o *ServicePlan) ToModel() *M.ServicePlan {
	if o.Tags == nil {
		o.Tags = make(StringList, 0)
	}
	if o.Accounts == nil {
		o.Accounts = make(ObjIDList, 0)
	}
	if o.Slos == nil {
		o.Slos = make(RestrictedStringValueMap, 0)
	}
	mObj := &M.ServicePlan{
		ServicePlanAllOf0: M.ServicePlanAllOf0{Meta: (&o.ObjMeta).ToModel("ServicePlan")},
	}
	mObj.Name = M.ServicePlanName(o.Name)
	mObj.Description = M.ObjDescription(o.Description)
	mObj.Tags = *o.Tags.ToModel()
	mObj.SourceServicePlanID = M.ObjIDMutable(o.SourceServicePlanID)
	mObj.State = o.State
	mObj.IoProfile = o.IoProfile.ToModel()
	mObj.ProvisioningUnit = o.ProvisioningUnit.ToModel()
	mObj.VolumeSeriesMinMaxSize = o.VolumeSeriesMinMaxSize.ToModel()
	mObj.Accounts = *o.Accounts.ToModel()
	mObj.Slos = o.Slos.ToModel()
	return mObj
}

// FromModel converts a model object to a datastore object
func (o *ServicePlan) FromModel(mObj *M.ServicePlan) {
	mMeta := mObj.Meta
	if mMeta == nil {
		mMeta = &M.ObjMeta{ObjType: "ServicePlan"}
	}
	(&o.ObjMeta).FromModel(mMeta)
	o.Name = string(mObj.Name)
	o.Description = string(mObj.Description)
	if o.Tags == nil {
		o.Tags = make(StringList, len(mObj.Tags))
	}
	o.Tags.FromModel(&mObj.Tags)
	if o.Accounts == nil {
		o.Accounts = make(ObjIDList, len(mObj.Accounts))
	}
	o.Accounts.FromModel(&mObj.Accounts)
	o.SourceServicePlanID = string(mObj.SourceServicePlanID)
	o.State = string(mObj.State)
	iop := mObj.IoProfile
	if iop == nil {
		iop = &M.IoProfile{}
		iop.IoPattern = &M.IoPattern{}
		iop.ReadWriteMix = &M.ReadWriteMix{}
	}
	o.IoProfile.FromModel(iop)
	pUnit := mObj.ProvisioningUnit
	if pUnit == nil {
		pUnit = &M.ProvisioningUnit{}
	}
	o.ProvisioningUnit.FromModel(pUnit)
	minmaxsize := mObj.VolumeSeriesMinMaxSize
	if minmaxsize == nil {
		minmaxsize = &M.VolumeSeriesMinMaxSize{}
	}
	o.VolumeSeriesMinMaxSize.FromModel(minmaxsize)
	if o.Slos == nil {
		o.Slos = make(RestrictedStringValueMap, len(mObj.Slos))
	}
	o.Slos.FromModel(mObj.Slos)
}

// servicePlanHandler is an ObjectDocumentHandler that implements the ds.ServicePlanOps operations
type servicePlanHandler struct {
	cName   string
	api     DBAPI
	crud    ObjectDocumentHandlerCRUD
	log     *logging.Logger
	indexes []mongo.IndexModel
	builtIn map[string]string
}

var _ = ds.ServicePlanOps(&servicePlanHandler{})
var _ = ObjectDocumentHandler(&servicePlanHandler{})

var odhServicePlan = &servicePlanHandler{
	cName: "serviceplan",
	indexes: []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: nameKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
	},
	builtIn: map[string]string{},
}

func init() {
	odhRegister(odhServicePlan.cName, odhServicePlan)
}

// convertListParams is a helper to convert ServicePlanListParams to bson.M
func (op *servicePlanHandler) convertListParams(params service_plan.ServicePlanListParams) bson.M {
	qParams := bson.M{}
	if params.AuthorizedAccountID != nil && *params.AuthorizedAccountID != "" {
		qParams["accounts"] = *params.AuthorizedAccountID
	}
	if params.Name != nil && *params.Name != "" {
		qParams[nameKey] = *params.Name
	}
	if params.Tags != nil {
		qParams["tags"] = bson.M{"$all": params.Tags}
	}
	if params.SourceServicePlanID != nil && *params.SourceServicePlanID != "" {
		qParams["sourceserviceplanid"] = *params.SourceServicePlanID
	}
	op.log.Info("qParams:", qParams)
	return qParams
}

// objectDocumentHandler methods

func (op *servicePlanHandler) Claim(api DBAPI, crud ObjectDocumentHandlerCRUD) {
	op.api = api
	op.crud = crud
	op.log = api.Logger()
}

func (op *servicePlanHandler) Initialize(ctx context.Context) error {
	if err := op.crud.CreateIndexes(ctx, op); err != nil {
		return err
	}
	op.builtIn = make(map[string]string)
	return populateCollection(ctx, op.api, op)
}

func (op *servicePlanHandler) Start(ctx context.Context) error {
	dbn := op.api.DBName()
	op.log.Infof("Starting collection %s.%s", dbn, op.cName)
	return nil
}

func (op *servicePlanHandler) Ops() interface{} {
	return op
}

func (op *servicePlanHandler) CName() string {
	return op.cName
}

func (op *servicePlanHandler) Indexes() []mongo.IndexModel {
	return op.indexes
}

func (op *servicePlanHandler) NewObject() interface{} {
	return &ServicePlan{}
}

// Populator methods

func (op *servicePlanHandler) Populate(ctx context.Context, fp string, buf []byte) error {
	mSP := &M.ServicePlan{}
	err := json.Unmarshal(buf, mSP)
	if err != nil {
		op.log.Errorf("Populate: json unmarshal failed for %s: %s", fp, err.Error())
		return err
	}
	if mSP.Name == "" {
		err = errors.New("non-empty name is required")
		op.log.Errorf("Populate: invalid service plan read from %s: %s", fp, err.Error())
		return err
	}
	// TBD additional validations
	dSP := &ServicePlan{}
	if err = op.crud.FindOne(ctx, op, bson.M{nameKey: string(mSP.Name)}, dSP); err != nil {
		if err != ds.ErrorNotFound {
			return err
		}
		mSP.Meta = nil // ensure that meta will be put into correct creation state by FromModel
		mSP.SourceServicePlanID = ""
		mSP.State = ds.PublishedState
		dSP.FromModel(mSP)
		dSP.Accounts = make(ObjIDList, 0)
		op.log.Infof("Creating built-in service plan '%s'", dSP.Name)
		if err = op.crud.InsertOne(ctx, op, dSP); err == nil {
			op.builtIn[dSP.MetaObjID] = dSP.Name
		}
	} else {
		op.builtIn[dSP.MetaObjID] = dSP.Name
	}
	return err
}

// ds.ServicePlanOps methods

func (op *servicePlanHandler) BuiltInPlan(planObjID string) (string, bool) {
	name, exists := op.builtIn[planObjID]
	return name, exists
}

func (op *servicePlanHandler) Clone(ctx context.Context, params service_plan.ServicePlanCloneParams) (*M.ServicePlan, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Clone:", err)
		return nil, err
	}
	qParams := bson.M{objKey: params.ID}
	if version := swag.Int32Value(params.Version); version != 0 {
		qParams[objVer] = version
	}
	dObj := &ServicePlan{}
	if err := op.crud.FindOne(ctx, op, qParams, dObj); err != nil {
		return nil, err
	}
	dObj.ObjMeta.FromModel(&M.ObjMeta{ObjType: "ServicePlan"})
	dObj.Name = string(params.Payload.Name)
	dObj.State = ds.UnpublishedState
	dObj.SourceServicePlanID = params.ID
	if err := op.crud.InsertOne(ctx, op, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

// Count the number of matching documents, limit is applied if non-zero
func (op *servicePlanHandler) Count(ctx context.Context, params service_plan.ServicePlanListParams, limit uint) (int, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Count:", err)
		return 0, err
	}
	return op.crud.Count(ctx, op, op.convertListParams(params), limit)
}

func (op *servicePlanHandler) Delete(ctx context.Context, mID string) error {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Delete:", err)
		return err
	}
	return op.crud.DeleteOne(ctx, op, mID)
}

func (op *servicePlanHandler) Fetch(ctx context.Context, mID string) (*M.ServicePlan, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Fetch:", err)
		return nil, err
	}
	dObj := &ServicePlan{}
	if err := op.crud.FindOne(ctx, op, bson.M{objKey: mID}, dObj); err != nil {
		return nil, err
	}
	return dObj.ToModel(), nil
}

func (op *servicePlanHandler) List(ctx context.Context, params service_plan.ServicePlanListParams) ([]*M.ServicePlan, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("List:", err)
		return nil, err
	}
	list := make([]*M.ServicePlan, 0, 10)
	err := op.crud.FindAll(ctx, op, params, op.convertListParams(params), func(obj interface{}) {
		dObj := obj.(*ServicePlan) // or panic
		list = append(list, dObj.ToModel())
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (op *servicePlanHandler) Publish(ctx context.Context, params service_plan.ServicePlanPublishParams) (*M.ServicePlan, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Publish:", err)
		return nil, err
	}
	qParams := bson.E{Key: "state", Value: bson.M{"$ne": ds.PublishedState}}
	ua := &ds.UpdateArgs{
		ID:      params.ID,
		Version: params.Version,
		Attributes: []ds.UpdateAttr{
			{Name: "State", Actions: [ds.NumActionTypes]ds.UpdateActionArgs{ds.UpdateSet: ds.UpdateActionArgs{FromBody: true}}},
		},
	}
	op.log.Info("Publish", params.ID, params.Version)
	dObj := &ServicePlan{State: ds.PublishedState}
	if err := op.crud.UpdateOne(ctx, op, dObj, ua, qParams); err != nil {
		if err == ds.ErrorIDVerNotFound {
			err = &ds.Error{M: ds.ErrorIDVerNotFound.M + " or state is already " + ds.PublishedState, C: ds.ErrorIDVerNotFound.C}
		}
		op.log.Errorf("ServicePlan(%s, %d) publish: %s", params.ID, params.Version, err)
		return nil, err
	}
	return dObj.ToModel(), nil
}

// RemoveAccount removes the given account from all service plans
func (op *servicePlanHandler) RemoveAccount(ctx context.Context, accountID string) error {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("RemoveAccount:", err)
		return err
	}
	dObj := &ServicePlan{Accounts: ObjIDList{accountID}}
	ua := &ds.UpdateArgs{
		Attributes: []ds.UpdateAttr{
			{Name: "Accounts", Actions: [ds.NumActionTypes]ds.UpdateActionArgs{ds.UpdateRemove: ds.UpdateActionArgs{FromBody: true}}},
		},
	}
	op.log.Info("RemoveAccount", accountID)
	if _, _, err := op.crud.UpdateAll(ctx, op, bson.M{}, dObj, ua); err != nil {
		return err
	}
	return nil
}

func (op *servicePlanHandler) Retire(ctx context.Context, params service_plan.ServicePlanRetireParams) (*M.ServicePlan, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Retire:", err)
		return nil, err
	}
	qParams := bson.E{Key: "state", Value: ds.PublishedState}
	ua := &ds.UpdateArgs{
		ID:      params.ID,
		Version: params.Version,
		Attributes: []ds.UpdateAttr{
			{Name: "State", Actions: [ds.NumActionTypes]ds.UpdateActionArgs{ds.UpdateSet: ds.UpdateActionArgs{FromBody: true}}},
		},
	}
	op.log.Info("Retire", params.ID, params.Version)
	dObj := &ServicePlan{State: ds.RetiredState}
	if err := op.crud.UpdateOne(ctx, op, dObj, ua, qParams); err != nil {
		if err == ds.ErrorIDVerNotFound {
			err = &ds.Error{M: ds.ErrorIDVerNotFound.M + " or state is not " + ds.PublishedState, C: ds.ErrorIDVerNotFound.C}
		}
		op.log.Errorf("ServicePlan(%s, %d) retire: %s", params.ID, params.Version, err)
		return nil, err
	}
	return dObj.ToModel(), nil
}

// Update updates the ServicePlan
// Note: Validations are performed by the handler. Take care when calling internally.
func (op *servicePlanHandler) Update(ctx context.Context, ua *ds.UpdateArgs, param *M.ServicePlanMutable) (*M.ServicePlan, error) {
	if err := op.api.MustBeReady(); err != nil {
		op.log.Error("Update:", err)
		return nil, err
	}
	mObj := &M.ServicePlan{ServicePlanMutable: *param}
	obj := &ServicePlan{}
	obj.FromModel(mObj)
	if err := op.crud.UpdateOne(ctx, op, obj, ua); err != nil {
		return nil, err
	}
	return obj.ToModel(), nil
}
