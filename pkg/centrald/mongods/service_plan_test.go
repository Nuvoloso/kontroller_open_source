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
	"reflect"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/service_plan"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/alecthomas/units"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// gomock.Matcher for ServicePlan
type mockServicePlanMatchCtx int

const (
	mockServicePlanInvalid mockServicePlanMatchCtx = iota
	mockServicePlanInsert
	mockServicePlanFind
	mockServicePlanUpdate
)

type mockServicePlanMatcher struct {
	t      *testing.T
	ctx    mockServicePlanMatchCtx
	ctxObj *ServicePlan
	retObj *ServicePlan
}

func newServicePlanMatcher(t *testing.T, ctx mockServicePlanMatchCtx, obj *ServicePlan) gomock.Matcher {
	return servicePlanMatcher(t, ctx).CtxObj(obj).Matcher()
}

// servicePlanMatcher creates a partially initialized mockServicePlanMatcher
func servicePlanMatcher(t *testing.T, ctx mockServicePlanMatchCtx) *mockServicePlanMatcher {
	return &mockServicePlanMatcher{t: t, ctx: ctx}
}

// Matcher ends the chain
func (o *mockServicePlanMatcher) Matcher() gomock.Matcher {
	if o.ctx == mockServicePlanInsert || o.ctx == mockServicePlanFind || o.ctx == mockServicePlanUpdate {
		assert.NotNil(o.t, o.ctxObj)
	}
	return o
}

// CtxObj adds an ServicePlan object
func (o *mockServicePlanMatcher) CtxObj(ao *ServicePlan) *mockServicePlanMatcher {
	o.ctxObj = ao
	return o
}

// Return sets the update result object
func (o *mockServicePlanMatcher) Return(ro *ServicePlan) *mockServicePlanMatcher {
	o.retObj = ro
	return o
}

// Matches is from gomock.Matcher
func (o *mockServicePlanMatcher) Matches(x interface{}) bool {
	var obj *ServicePlan
	switch z := x.(type) {
	case *ServicePlan:
		obj = z
	default:
		panic("Did not get a recognizable object: " + reflect.TypeOf(x).String())
	}
	assert := assert.New(o.t)
	switch o.ctx {
	case mockServicePlanInsert:
		compObj := o.ctxObj
		compObj.ObjMeta = obj.ObjMeta // insert sets meta data
		return assert.NotZero(obj.ObjMeta.MetaObjID) &&
			assert.Equal(int32(1), obj.ObjMeta.MetaVersion) &&
			assert.False(obj.ObjMeta.MetaTimeCreated.IsZero()) &&
			assert.False(obj.ObjMeta.MetaTimeModified.IsZero()) &&
			assert.Equal(obj.ObjMeta.MetaTimeCreated, obj.ObjMeta.MetaTimeModified) &&
			assert.Equal(compObj, obj)
	case mockServicePlanFind:
		*obj = *o.ctxObj
		return true
	case mockServicePlanUpdate:
		if assert.NotNil(obj) {
			obj.ObjMeta = o.ctxObj.ObjMeta
			assert.Equal(o.ctxObj, obj)
			*obj = *o.retObj
			return true
		}
	}
	return false
}

// String is from gomock.Matcher
func (o *mockServicePlanMatcher) String() string {
	switch o.ctx {
	case mockServicePlanInsert:
		return "matches on insert"
	}
	return "unknown context"
}

func TestServicePlanHandler(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	assert.Equal("serviceplan", odhServicePlan.CName())
	indexes := odhServicePlan.Indexes()
	expIndexes := []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: nameKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
	}
	assert.Equal(expIndexes, indexes)
	assert.Equal(indexes, odhServicePlan.indexes)
	dIf := odhServicePlan.NewObject()
	assert.NotNil(dIf)
	_, ok := dIf.(*ServicePlan)
	assert.True(ok)

	t.Log("case: Claim + Initialize success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ctx := context.Background()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	odhServicePlan.Claim(api, crud)
	assert.Equal(api, odhServicePlan.api)
	assert.Equal(crud, odhServicePlan.crud)
	assert.Equal(l, odhServicePlan.log)
	assert.Equal(odhServicePlan, odhServicePlan.Ops())
	crud.EXPECT().CreateIndexes(ctx, odhServicePlan).Return(nil)
	api.EXPECT().BaseDataPathName().Return("") // Populate is tested separately
	assert.NoError(odhServicePlan.Initialize(ctx))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize, error creating indexes")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhServicePlan).Return(errUnknownError)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	assert.Error(errUnknownError, odhServicePlan.Initialize(ctx))

	t.Log("case: Start")
	api.EXPECT().DBName().Return("dbName")
	assert.NoError(odhServicePlan.Start(ctx))
}

func TestServicePlanPopulate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	dServicePlan := &ServicePlan{
		Name:  "major",
		State: "PUBLISHED",
	}
	mServicePlan := dServicePlan.ToModel()
	mServicePlan.Meta = &models.ObjMeta{}
	assert.Zero(mServicePlan.Meta.ID)
	assert.Zero(mServicePlan.Meta.Version)
	buf, err := json.Marshal(mServicePlan)
	assert.NoError(err)

	t.Log("case: successful creation")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ctx := context.Background()
	api := NewMockDBAPI(mockCtrl)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhServicePlan, bson.M{"name": "major"}, newServicePlanMatcher(t, mockServicePlanFind, dServicePlan)).Return(ds.ErrorNotFound)
	crud.EXPECT().InsertOne(ctx, odhServicePlan, newServicePlanMatcher(t, mockServicePlanInsert, dServicePlan)).Return(nil)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	odhServicePlan.log = l
	retErr := odhServicePlan.Populate(ctx, "file1", buf)
	assert.NoError(retErr)
	assert.Len(odhServicePlan.builtIn, 1)
	name, ok := odhServicePlan.BuiltInPlan(dServicePlan.MetaObjID)
	assert.True(ok)
	assert.Equal(name, dServicePlan.Name)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: insert error")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhServicePlan, bson.M{"name": "major"}, newServicePlanMatcher(t, mockServicePlanFind, dServicePlan)).Return(ds.ErrorNotFound)
	crud.EXPECT().InsertOne(ctx, odhServicePlan, newServicePlanMatcher(t, mockServicePlanInsert, dServicePlan)).Return(errUnknownError)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	odhServicePlan.log = l
	odhServicePlan.builtIn = nil
	retErr = odhServicePlan.Populate(ctx, "file1", buf)
	assert.Equal(errUnknownError, retErr)
	assert.Empty(odhServicePlan.builtIn)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: find error")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhServicePlan, bson.M{"name": "major"}, newServicePlanMatcher(t, mockServicePlanFind, dServicePlan)).Return(errUnknownError)
	odhServicePlan.crud = crud
	odhServicePlan.builtIn = nil
	odhServicePlan.api = api
	odhServicePlan.log = l
	retErr = odhServicePlan.Populate(ctx, "file1", buf)
	assert.Equal(errUnknownError, retErr)
	assert.Empty(odhServicePlan.builtIn)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: servicePlan exists")
	dServicePlan.MetaObjID = "servicePlan-id"
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhServicePlan, bson.M{"name": "major"}, newServicePlanMatcher(t, mockServicePlanFind, dServicePlan)).Return(nil)
	odhServicePlan.crud = crud
	odhServicePlan.builtIn = map[string]string{}
	odhServicePlan.api = api
	odhServicePlan.log = l
	retErr = odhServicePlan.Populate(ctx, "file1", buf)
	assert.NoError(retErr)
	assert.Len(odhServicePlan.builtIn, 1)
	name, ok = odhServicePlan.BuiltInPlan(dServicePlan.MetaObjID)
	assert.True(ok)
	assert.Equal("major", name)
	tl.Flush()

	// case: unmarshal error
	retErr = odhServicePlan.Populate(ctx, "file1", []byte("not valid json"))
	assert.Regexp("invalid character", retErr)

	// case: no name
	retErr = odhServicePlan.Populate(ctx, "file1", []byte("{}"))
	assert.Regexp("non-empty name is required", retErr)
}

func TestServicePlanClone(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	params := service_plan.ServicePlanCloneParams{
		ID:      "servicePlanID1",
		Payload: &models.ServicePlanCloneArgs{Name: "newSPName"},
		Version: nil,
	}

	now := time.Now()
	mC := &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			Meta: &models.ObjMeta{
				ID:      "servicePlanID1",
				ObjType: "ServicePlan",
				Version: 8,
			},
			SourceServicePlanID: "", // a built-in service plan
			State:               ds.PublishedState,
		},
		ServicePlanMutable: models.ServicePlanMutable{Name: "spName"},
	}
	dServicePlan := &ServicePlan{}
	dServicePlan.FromModel(mC) // initialize empty properties
	dServicePlan.MetaTimeCreated = now.AddDate(-1, -2, -1)
	dServicePlan.MetaTimeModified = now.AddDate(-1, 0, 0)
	dClone := &ServicePlan{}
	mC.Meta = &models.ObjMeta{ObjType: "ServicePlan"}
	dClone.FromModel(mC)
	dClone.Name = string(params.Payload.Name)
	dClone.SourceServicePlanID = dServicePlan.MetaObjID
	dClone.State = ds.UnpublishedState
	fArg := bson.M{objKey: params.ID}
	ctx := context.Background()

	t.Log("case: successful clone, no version")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhServicePlan, fArg, newServicePlanMatcher(t, mockServicePlanFind, dServicePlan)).Return(nil)
	crud.EXPECT().InsertOne(ctx, odhServicePlan, newServicePlanMatcher(t, mockServicePlanInsert, dClone)).Return(nil)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr := odhServicePlan.Clone(ctx, params)
	assert.NoError(retErr)
	assert.NotZero(retMA.Meta.ID)
	assert.Equal(models.ObjVersion(1), retMA.Meta.Version)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: successful clone, version set")
	fArg[objVer] = dServicePlan.MetaVersion
	params.Version = swag.Int32(dServicePlan.MetaVersion)
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhServicePlan, fArg, newServicePlanMatcher(t, mockServicePlanFind, dServicePlan)).Return(nil)
	crud.EXPECT().InsertOne(ctx, odhServicePlan, newServicePlanMatcher(t, mockServicePlanInsert, dClone)).Return(nil)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr = odhServicePlan.Clone(ctx, params)
	assert.NoError(retErr)
	assert.NotZero(retMA.Meta.ID)
	assert.Equal(models.ObjVersion(1), retMA.Meta.Version)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not found")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhServicePlan, fArg, newServicePlanMatcher(t, mockServicePlanFind, dServicePlan)).Return(ds.ErrorIDVerNotFound)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr = odhServicePlan.Clone(ctx, params)
	assert.Equal(ds.ErrorIDVerNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: duplicate")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhServicePlan, fArg, newServicePlanMatcher(t, mockServicePlanFind, dServicePlan)).Return(nil)
	crud.EXPECT().InsertOne(ctx, odhServicePlan, newServicePlanMatcher(t, mockServicePlanInsert, dClone)).Return(ds.ErrorExists)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr = odhServicePlan.Clone(ctx, params)
	assert.Equal(ds.ErrorExists, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr = odhServicePlan.Clone(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestServicePlanCount(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	t.Log("case: success")
	ctx := context.Background()
	params := service_plan.ServicePlanListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhServicePlan, fArg, uint(0)).Return(2, nil)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr := odhServicePlan.Count(ctx, params, 0)
	assert.Equal(2, retMA)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: count fails")
	mockCtrl = gomock.NewController(t)
	params = service_plan.ServicePlanListParams{
		Name: swag.String("oid1"),
		Tags: []string{"tag1", "tag2"},
	}
	fArg = bson.M{
		"name": *params.Name,
		"tags": bson.M{"$all": params.Tags},
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhServicePlan, fArg, uint(5)).Return(0, errWrappedError)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr = odhServicePlan.Count(ctx, params, 5)
	assert.Zero(retMA)
	assert.Equal(errWrappedError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr = odhServicePlan.Count(ctx, params, 0)
	assert.Equal(errWrappedError, retErr)
	assert.Zero(retMA)
}

func TestServicePlanDelete(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	mID := "c72bcfb7-90d4-4276-89ef-aa4bacd09ec7"
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().DeleteOne(ctx, odhServicePlan, mID).Return(nil)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	odhServicePlan.log = l
	retErr := odhServicePlan.Delete(ctx, mID)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().DeleteOne(ctx, odhServicePlan, mID).Return(errUnknownError)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	odhServicePlan.log = l
	retErr = odhServicePlan.Delete(ctx, mID)
	assert.Equal(errUnknownError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhServicePlan.api = api
	odhServicePlan.log = l
	retErr = odhServicePlan.Delete(ctx, mID)
	assert.Equal(errWrappedError, retErr)
}

func TestServicePlanFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mTime := time.Now()
	cTime := mTime.AddDate(0, 0, -1)
	data := &ServicePlan{
		ObjMeta: ObjMeta{
			MetaObjID:        "servicePlanID1",
			MetaTimeCreated:  cTime,
			MetaTimeModified: mTime,
		},
		Name:        "testservicePlan",
		Description: "test description",
		Tags:        []string{"tag1", "tag2"},
		Slos: RestrictedStringValueMap{
			"slo1": {Kind: "INT", Value: "1"},
			"slo2": {Kind: "STRING", Value: "a string"},
		},
		SourceServicePlanID: "servicePlanBuiltinID",
		State:               ds.PublishedState,
		Accounts:            ObjIDList{"accountID1", "accountID2"},
	}
	mObj := data.ToModel()
	dServicePlan := &ServicePlan{}
	dServicePlan.FromModel(mObj) // initialize all fields
	fArg := bson.M{objKey: dServicePlan.ObjMeta.MetaObjID}
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhServicePlan, fArg, newServicePlanMatcher(t, mockServicePlanFind, dServicePlan)).Return(nil)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr := odhServicePlan.Fetch(ctx, dServicePlan.ObjMeta.MetaObjID)
	assert.NoError(retErr)
	assert.Equal(dServicePlan.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhServicePlan, fArg, newServicePlanMatcher(t, mockServicePlanFind, dServicePlan)).Return(ds.ErrorNotFound)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr = odhServicePlan.Fetch(ctx, dServicePlan.ObjMeta.MetaObjID)
	assert.Equal(ds.ErrorNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr = odhServicePlan.Fetch(ctx, dServicePlan.ObjMeta.MetaObjID)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestServicePlanList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	now := time.Now()
	dServicePlans := []interface{}{
		&ServicePlan{
			ObjMeta: ObjMeta{
				MetaObjID:        "spID1",
				MetaTimeCreated:  now.AddDate(-1, -2, -1),
				MetaTimeModified: now.AddDate(-1, 0, 0),
			},
			Name:        "servicePlan1",
			Description: "servicePlan1 description",
			Tags:        []string{"tag1", "tag2"},
			Slos: RestrictedStringValueMap{
				"slo1": {Kind: "INT", Value: "1"},
				"slo2": {Kind: "STRING", Value: "a string"},
			},
			IoProfile: IoProfile{
				IoPattern:    IoPattern{Name: "sequential", MinSizeBytesAvg: 16384, MaxSizeBytesAvg: 262144},
				ReadWriteMix: ReadWriteMix{Name: "read-write", MinReadPercent: 30, MaxReadPercent: 70},
			},
			ProvisioningUnit:       ProvisioningUnit{IOPS: 0, Throughput: 4 * int64(units.MB)},
			VolumeSeriesMinMaxSize: VolumeSeriesMinMaxSize{MinSizeBytes: 1 * int64(units.GiB), MaxSizeBytes: 64 * int64(units.TiB)},
			SourceServicePlanID:    "",
			State:                  ds.PublishedState,
			Accounts:               ObjIDList{},
		},
		&ServicePlan{
			ObjMeta: ObjMeta{
				MetaObjID:        "spID2",
				MetaTimeCreated:  now.AddDate(0, -2, -1),
				MetaTimeModified: now.AddDate(0, -1, -2),
			},
			Name:        "servicePlan2",
			Description: "servicePlan2 description",
			Slos: RestrictedStringValueMap{
				"slo1": {Kind: "INT", Value: "2"},
				"slo2": {Kind: "STRING", Value: "b string"},
			},
			IoProfile: IoProfile{
				IoPattern:    IoPattern{Name: "sequential", MinSizeBytesAvg: 16384, MaxSizeBytesAvg: 262144},
				ReadWriteMix: ReadWriteMix{Name: "read-write", MinReadPercent: 30, MaxReadPercent: 70},
			},
			ProvisioningUnit:       ProvisioningUnit{IOPS: 4, Throughput: 0},
			VolumeSeriesMinMaxSize: VolumeSeriesMinMaxSize{MinSizeBytes: 1 * int64(units.GiB), MaxSizeBytes: 64 * int64(units.TiB)},
			SourceServicePlanID:    "spID1",
			State:                  ds.UnpublishedState,
			Accounts:               ObjIDList{},
		},
	}
	ctx := context.Background()

	t.Log("case: success")
	params := service_plan.ServicePlanListParams{
		Name: swag.String("servicePlan1"),
		Tags: []string{"tag1", "tag2"},
	}
	fArg := bson.M{
		"name": *params.Name,
		"tags": bson.M{"$all": params.Tags},
	}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhServicePlan, params, fArg, newConsumeObjFnMatcher(t, dServicePlans...)).Return(nil)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr := odhServicePlan.List(ctx, params)
	assert.NoError(retErr)
	assert.Len(retMA, len(dServicePlans))
	for i, mA := range retMA {
		o := dServicePlans[i].(*ServicePlan)
		assert.Equal(o.ToModel(), mA)
	}
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	params = service_plan.ServicePlanListParams{
		AuthorizedAccountID: swag.String("aid1"),
		SourceServicePlanID: swag.String("servicePlan1"),
	}
	fArg = bson.M{
		"accounts":            *params.AuthorizedAccountID,
		"sourceserviceplanid": *params.SourceServicePlanID,
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhServicePlan, params, fArg, gomock.Any()).Return(errWrappedError)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr = odhServicePlan.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr = odhServicePlan.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestServicePlanPublish(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	params := service_plan.ServicePlanPublishParams{
		ID:      "spID2",
		Version: 8,
	}
	now := time.Now()
	data := &ServicePlan{
		ObjMeta: ObjMeta{
			MetaObjID:        "spID2",
			MetaTimeCreated:  now.AddDate(0, -2, -1),
			MetaTimeModified: now.AddDate(0, -1, -2),
			MetaVersion:      params.Version + 1,
		},
		Name:        "servicePlan2",
		Description: "servicePlan2 description",
		Tags:        []string{"tag1", "tag2"},
		Slos: RestrictedStringValueMap{
			"slo1": {Kind: "INT", Value: "2"},
			"slo2": {Kind: "STRING", Value: "b string"},
		},
		SourceServicePlanID: "spID1",
		State:               ds.PublishedState,
		Accounts:            ObjIDList{},
	}
	extra := bson.E{Key: "state", Value: bson.M{"$ne": "PUBLISHED"}}
	dObj := &ServicePlan{State: "PUBLISHED"}
	ua := &ds.UpdateArgs{
		ID:      params.ID,
		Version: params.Version,
		Attributes: []ds.UpdateAttr{
			{Name: "State", Actions: [ds.NumActionTypes]ds.UpdateActionArgs{ds.UpdateSet: ds.UpdateActionArgs{FromBody: true}}},
		},
	}
	m := servicePlanMatcher(t, mockServicePlanUpdate).CtxObj(dObj).Return(data).Matcher()
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhServicePlan, m, ua, extra).Return(nil)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr := odhServicePlan.Publish(ctx, params)
	assert.NoError(retErr)
	assert.Equal(data.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhServicePlan, m, ua, extra).Return(ds.ErrorIDVerNotFound)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr = odhServicePlan.Publish(ctx, params)
	assert.Regexp("^"+ds.ErrorIDVerNotFound.M, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: db error")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhServicePlan, m, ua, extra).Return(ds.ErrorDbError)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr = odhServicePlan.Publish(ctx, params)
	assert.Equal(ds.ErrorDbError, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr = odhServicePlan.Publish(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestServicePlanRemoveAccount(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	// success
	accountID := "accountId"
	selector := bson.M{}
	dObj := &ServicePlan{Accounts: ObjIDList{accountID}}
	ua := &ds.UpdateArgs{
		Attributes: []ds.UpdateAttr{
			{Name: "Accounts", Actions: [ds.NumActionTypes]ds.UpdateActionArgs{ds.UpdateRemove: ds.UpdateActionArgs{FromBody: true}}},
		},
	}
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateAll(ctx, odhServicePlan, selector, dObj, ua).Return(3, 3, nil)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	odhServicePlan.log = l
	retErr := odhServicePlan.RemoveAccount(ctx, accountID)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateAll(ctx, odhServicePlan, selector, dObj, ua).Return(0, 0, ds.ErrorDbError)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	odhServicePlan.log = l
	retErr = odhServicePlan.RemoveAccount(ctx, accountID)
	assert.Equal(ds.ErrorDbError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhServicePlan.api = api
	odhServicePlan.log = l
	retErr = odhServicePlan.RemoveAccount(ctx, accountID)
	assert.Equal(errWrappedError, retErr)
}

func TestServicePlanRetire(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	params := service_plan.ServicePlanRetireParams{
		ID:      "spID2",
		Version: 8,
	}
	now := time.Now()
	data := &ServicePlan{
		ObjMeta: ObjMeta{
			MetaObjID:        "spID2",
			MetaTimeCreated:  now.AddDate(0, -2, -1),
			MetaTimeModified: now.AddDate(0, -1, -2),
			MetaVersion:      params.Version + 1,
		},
		Name:        "servicePlan2",
		Description: "servicePlan2 description",
		Tags:        []string{"tag1", "tag2"},
		Slos: RestrictedStringValueMap{
			"slo1": {Kind: "INT", Value: "2"},
			"slo2": {Kind: "STRING", Value: "b string"},
		},
		SourceServicePlanID: "spID1",
		State:               ds.RetiredState,
		Accounts:            ObjIDList{},
	}
	extra := bson.E{Key: "state", Value: "PUBLISHED"}
	dObj := &ServicePlan{State: "RETIRED"}
	ua := &ds.UpdateArgs{
		ID:      params.ID,
		Version: params.Version,
		Attributes: []ds.UpdateAttr{
			{Name: "State", Actions: [ds.NumActionTypes]ds.UpdateActionArgs{ds.UpdateSet: ds.UpdateActionArgs{FromBody: true}}},
		},
	}
	m := servicePlanMatcher(t, mockServicePlanUpdate).CtxObj(dObj).Return(data).Matcher()
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhServicePlan, m, ua, extra).Return(nil)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr := odhServicePlan.Retire(ctx, params)
	assert.NoError(retErr)
	assert.Equal(data.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhServicePlan, m, ua, extra).Return(ds.ErrorIDVerNotFound)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr = odhServicePlan.Retire(ctx, params)
	assert.Regexp("^"+ds.ErrorIDVerNotFound.M, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: db error")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhServicePlan, m, ua, extra).Return(ds.ErrorDbError)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr = odhServicePlan.Retire(ctx, params)
	assert.Equal(ds.ErrorDbError, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr = odhServicePlan.Retire(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestServicePlanUpdate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	ua := &ds.UpdateArgs{
		ID:      "c72bcfb7-90d4-4276-89ef-aa4bacd09ec7",
		Version: 0,
		Attributes: []ds.UpdateAttr{
			{
				Name: "Name",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
			{
				Name: "Description",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
			{
				Name: "Tags",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateAppend: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
			{
				Name: "Slos",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateRemove: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
			{
				Name: "Accounts",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	param := &models.ServicePlanMutable{
		Name:        "newName",
		Description: "description",
		Tags:        []string{"tag1", "tag2"},
		Slos: map[string]models.RestrictedValueType{
			"slo1": {
				ValueType:                 models.ValueType{Kind: "INT", Value: "1"},
				RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: true},
			},
			"slo2": {
				ValueType:                 models.ValueType{Kind: "STRING", Value: "a string"},
				RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: false},
			},
		},
		Accounts: []models.ObjIDMutable{"account1", "account2"},
	}
	mObj := &models.ServicePlan{ServicePlanMutable: *param}
	dObj := &ServicePlan{}
	dObj.FromModel(mObj)
	now := time.Now()
	dServicePlan := &ServicePlan{
		ObjMeta: ObjMeta{
			MetaObjID:        ua.ID,
			MetaTimeCreated:  now.AddDate(0, -2, -1),
			MetaTimeModified: now,
			MetaVersion:      1000,
		},
	}
	m := servicePlanMatcher(t, mockServicePlanUpdate).CtxObj(dObj).Return(dServicePlan).Matcher()
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhServicePlan, m, ua).Return(nil)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr := odhServicePlan.Update(ctx, ua, param)
	assert.NoError(retErr)
	assert.Equal(dServicePlan.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhServicePlan, m, ua).Return(ds.ErrorIDVerNotFound)
	odhServicePlan.crud = crud
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr = odhServicePlan.Update(ctx, ua, param)
	assert.Equal(ds.ErrorIDVerNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhServicePlan.api = api
	odhServicePlan.log = l
	retMA, retErr = odhServicePlan.Update(ctx, ua, param)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}
