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
	"reflect"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/service_plan_allocation"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// gomock.Matcher for ServicePlanAllocation
type mockServicePlanAllocationMatchCtx int

const (
	mockServicePlanAllocationInvalid mockServicePlanAllocationMatchCtx = iota
	mockServicePlanAllocationInsert
	mockServicePlanAllocationFind
	mockServicePlanAllocationUpdate
)

type mockServicePlanAllocationMatcher struct {
	t      *testing.T
	ctx    mockServicePlanAllocationMatchCtx
	ctxObj *ServicePlanAllocation
	retObj *ServicePlanAllocation
}

func newServicePlanAllocationMatcher(t *testing.T, ctx mockServicePlanAllocationMatchCtx, obj *ServicePlanAllocation) gomock.Matcher {
	return servicePlanAllocationMatcher(t, ctx).CtxObj(obj).Matcher()
}

// servicePlanAllocationMatcher creates a partially initialized mockServicePlanAllocationMatcher
func servicePlanAllocationMatcher(t *testing.T, ctx mockServicePlanAllocationMatchCtx) *mockServicePlanAllocationMatcher {
	return &mockServicePlanAllocationMatcher{t: t, ctx: ctx}
}

// Matcher ends the chain
func (o *mockServicePlanAllocationMatcher) Matcher() gomock.Matcher {
	if o.ctx == mockServicePlanAllocationInsert || o.ctx == mockServicePlanAllocationFind || o.ctx == mockServicePlanAllocationUpdate {
		assert.NotNil(o.t, o.ctxObj)
	}
	return o
}

// CtxObj adds an ServicePlanAllocation object
func (o *mockServicePlanAllocationMatcher) CtxObj(ao *ServicePlanAllocation) *mockServicePlanAllocationMatcher {
	o.ctxObj = ao
	return o
}

// Return sets the update result object
func (o *mockServicePlanAllocationMatcher) Return(ro *ServicePlanAllocation) *mockServicePlanAllocationMatcher {
	o.retObj = ro
	return o
}

// Matches is from gomock.Matcher
func (o *mockServicePlanAllocationMatcher) Matches(x interface{}) bool {
	var obj *ServicePlanAllocation
	switch z := x.(type) {
	case *ServicePlanAllocation:
		obj = z
	default:
		panic("Did not get a recognizable object: " + reflect.TypeOf(x).String())
	}
	assert := assert.New(o.t)
	switch o.ctx {
	case mockServicePlanAllocationInsert:
		compObj := o.ctxObj
		compObj.ObjMeta = obj.ObjMeta // insert sets meta data
		return assert.NotZero(obj.ObjMeta.MetaObjID) &&
			assert.Equal(int32(1), obj.ObjMeta.MetaVersion) &&
			assert.False(obj.ObjMeta.MetaTimeCreated.IsZero()) &&
			assert.False(obj.ObjMeta.MetaTimeModified.IsZero()) &&
			assert.Equal(obj.ObjMeta.MetaTimeCreated, obj.ObjMeta.MetaTimeModified) &&
			assert.Equal(compObj, obj)
	case mockServicePlanAllocationFind:
		*obj = *o.ctxObj
		return true
	case mockServicePlanAllocationUpdate:
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
func (o *mockServicePlanAllocationMatcher) String() string {
	switch o.ctx {
	case mockServicePlanAllocationInsert:
		return "matches on insert"
	}
	return "unknown context"
}

func TestServicePlanAllocationHandler(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	assert.Equal("serviceplanallocation", odhServicePlanAllocation.CName())
	indexes := odhServicePlanAllocation.Indexes()
	expIndexes := []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: authorizedAccountIDKey, Value: IndexAscending}, {Key: servicePlanIDKey, Value: IndexAscending}, {Key: clusterIDKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
	}
	assert.Equal(expIndexes, indexes)
	assert.Equal(indexes, odhServicePlanAllocation.indexes)
	dIf := odhServicePlanAllocation.NewObject()
	assert.NotNil(dIf)
	_, ok := dIf.(*ServicePlanAllocation)
	assert.True(ok)

	t.Log("case: Claim + Initialize success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ctx := context.Background()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	odhServicePlanAllocation.Claim(api, crud)
	assert.Equal(api, odhServicePlanAllocation.api)
	assert.Equal(crud, odhServicePlanAllocation.crud)
	assert.Equal(l, odhServicePlanAllocation.log)
	assert.Equal(odhServicePlanAllocation, odhServicePlanAllocation.Ops())
	crud.EXPECT().CreateIndexes(ctx, odhServicePlanAllocation).Return(nil)
	assert.NoError(odhServicePlanAllocation.Initialize(ctx))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize, error creating indexes")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhServicePlanAllocation).Return(errUnknownError)
	odhServicePlanAllocation.crud = crud
	odhServicePlanAllocation.api = api
	assert.Error(errUnknownError, odhServicePlanAllocation.Initialize(ctx))

	t.Log("case: Start")
	api.EXPECT().DBName().Return("dbName")
	assert.NoError(odhServicePlanAllocation.Start(ctx))
}

func TestServicePlanAllocationCount(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	t.Log("case: success")
	ctx := context.Background()
	params := service_plan_allocation.ServicePlanAllocationListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhServicePlanAllocation, fArg, uint(0)).Return(2, nil)
	odhServicePlanAllocation.crud = crud
	odhServicePlanAllocation.api = api
	odhServicePlanAllocation.log = l
	retMA, retErr := odhServicePlanAllocation.Count(ctx, params, 0)
	assert.Equal(2, retMA)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: count fails")
	mockCtrl = gomock.NewController(t)
	params = service_plan_allocation.ServicePlanAllocationListParams{
		AuthorizedAccountID: swag.String("oid1"),
		Tags:                []string{"tag1", "tag2"},
	}
	fArg = bson.M{
		"authorizedaccountid": *params.AuthorizedAccountID,
		"tags":                bson.M{"$all": params.Tags},
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhServicePlanAllocation, fArg, uint(5)).Return(0, errWrappedError)
	odhServicePlanAllocation.crud = crud
	odhServicePlanAllocation.api = api
	odhServicePlanAllocation.log = l
	retMA, retErr = odhServicePlanAllocation.Count(ctx, params, 5)
	assert.Zero(retMA)
	assert.Equal(errWrappedError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhServicePlanAllocation.api = api
	odhServicePlanAllocation.log = l
	retMA, retErr = odhServicePlanAllocation.Count(ctx, params, 0)
	assert.Equal(errWrappedError, retErr)
	assert.Zero(retMA)
}

func TestServicePlanAllocationCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mSPA := &models.ServicePlanAllocation{
		ServicePlanAllocationAllOf0: models.ServicePlanAllocationAllOf0{
			Meta:        &models.ObjMeta{ObjType: "ServicePlanAllocation"},
			CspDomainID: "derivedPropertyOnCreate",
		},
		ServicePlanAllocationCreateOnce: models.ServicePlanAllocationCreateOnce{
			AccountID:           "ownerAccountId",
			AuthorizedAccountID: "authorizedAccountId",
			ClusterID:           "clusterId",
			ServicePlanID:       "servicePlanId",
		},
		ServicePlanAllocationMutable: models.ServicePlanAllocationMutable{
			ServicePlanAllocationMutableAllOf0: models.ServicePlanAllocationMutableAllOf0{
				ReservableCapacityBytes: swag.Int64(200000000000),
			},
			ServicePlanAllocationCreateMutable: models.ServicePlanAllocationCreateMutable{
				Messages:            []*models.TimestampedString{},
				ProvisioningHints:   map[string]models.ValueType{},
				StorageFormula:      "Formula1",
				StorageReservations: map[string]models.StorageTypeReservation{},
				SystemTags:          []string{"stag1", "stag2"},
				Tags:                models.ObjTags{"tag1", "tag2", "tag3"},
				TotalCapacityBytes:  swag.Int64(200000000000),
			},
		},
	}
	dServicePlanAllocation := &ServicePlanAllocation{}
	dServicePlanAllocation.FromModel(mSPA) // initialize empty properties
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().InsertOne(ctx, odhServicePlanAllocation, newServicePlanAllocationMatcher(t, mockServicePlanAllocationInsert, dServicePlanAllocation)).Return(nil)
	odhServicePlanAllocation.crud = crud
	odhServicePlanAllocation.api = api
	odhServicePlanAllocation.log = l
	retMA, retErr := odhServicePlanAllocation.Create(ctx, mSPA)
	assert.NoError(retErr)
	assert.NotZero(retMA.Meta.ID)
	assert.Equal(models.ObjVersion(1), retMA.Meta.Version)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: duplicate")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().InsertOne(ctx, odhServicePlanAllocation, newServicePlanAllocationMatcher(t, mockServicePlanAllocationInsert, dServicePlanAllocation)).Return(ds.ErrorExists)
	odhServicePlanAllocation.crud = crud
	odhServicePlanAllocation.api = api
	odhServicePlanAllocation.log = l
	retMA, retErr = odhServicePlanAllocation.Create(ctx, mSPA)
	assert.Equal(ds.ErrorExists, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhServicePlanAllocation.api = api
	odhServicePlanAllocation.log = l
	retMA, retErr = odhServicePlanAllocation.Create(ctx, mSPA)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestServicePlanAllocationDelete(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	mID := "58290457-139b-4c55-b6e0-e76b4913ebb6"
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().DeleteOne(ctx, odhServicePlanAllocation, mID).Return(nil)
	odhServicePlanAllocation.crud = crud
	odhServicePlanAllocation.api = api
	odhServicePlanAllocation.log = l
	retErr := odhServicePlanAllocation.Delete(ctx, mID)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().DeleteOne(ctx, odhServicePlanAllocation, mID).Return(errUnknownError)
	odhServicePlanAllocation.crud = crud
	odhServicePlanAllocation.api = api
	odhServicePlanAllocation.log = l
	retErr = odhServicePlanAllocation.Delete(ctx, mID)
	assert.Equal(errUnknownError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhServicePlanAllocation.api = api
	odhServicePlanAllocation.log = l
	retErr = odhServicePlanAllocation.Delete(ctx, mID)
	assert.Equal(errWrappedError, retErr)
}

func TestServicePlanAllocationFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mTime := time.Now()
	cTime := mTime.AddDate(0, 0, -1)
	dServicePlanAllocation := &ServicePlanAllocation{
		ObjMeta: ObjMeta{
			MetaObjID:        uuid.NewV4().String(),
			MetaVersion:      1,
			MetaTimeCreated:  cTime,
			MetaTimeModified: mTime,
		},
		CspDomainID:             "cspDomainId",
		ReservableCapacityBytes: 1000000000,
		ServicePlanAllocationCreateOnce: ServicePlanAllocationCreateOnce{
			AccountID:           "ownerAccountId",
			AuthorizedAccountID: "authorizedAccountId",
			ClusterID:           "clusterId",
			ServicePlanID:       "servicePlanId",
		},
		ServicePlanAllocationCreateMutable: ServicePlanAllocationCreateMutable{
			Messages: []TimestampedString{},
			ProvisioningHints: StringValueMap{
				"attr1": ValueType{Kind: "INT", Value: "1"},
				"attr2": ValueType{Kind: "STRING", Value: "a string"},
				"attr3": ValueType{Kind: "SECRET", Value: "a secret string"},
			},
			ReservationState: "UNKNOWN",
			StorageFormula:   "Formula1",
			StorageReservations: StorageTypeReservationMap{
				"prov1": {NumMirrors: 1, SizeBytes: 1},
				"prov2": {NumMirrors: 2, SizeBytes: 22},
				"prov3": {NumMirrors: 3, SizeBytes: 333},
			},
			TotalCapacityBytes: 200000000000,
		},
	}
	fArg := bson.M{objKey: dServicePlanAllocation.ObjMeta.MetaObjID}
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhServicePlanAllocation, fArg, newServicePlanAllocationMatcher(t, mockServicePlanAllocationFind, dServicePlanAllocation)).Return(nil)
	odhServicePlanAllocation.crud = crud
	odhServicePlanAllocation.api = api
	odhServicePlanAllocation.log = l
	retMA, retErr := odhServicePlanAllocation.Fetch(ctx, dServicePlanAllocation.ObjMeta.MetaObjID)
	assert.NoError(retErr)
	assert.Equal(dServicePlanAllocation.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not found")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhServicePlanAllocation, fArg, newServicePlanAllocationMatcher(t, mockServicePlanAllocationFind, dServicePlanAllocation)).Return(ds.ErrorNotFound)
	odhServicePlanAllocation.crud = crud
	odhServicePlanAllocation.api = api
	odhServicePlanAllocation.log = l
	retMA, retErr = odhServicePlanAllocation.Fetch(ctx, dServicePlanAllocation.ObjMeta.MetaObjID)
	assert.Equal(ds.ErrorNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhServicePlanAllocation.api = api
	odhServicePlanAllocation.log = l
	retMA, retErr = odhServicePlanAllocation.Fetch(ctx, dServicePlanAllocation.ObjMeta.MetaObjID)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestServicePlanAllocationList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	now := time.Now()
	mTime1 := now.AddDate(-1, 0, 0)
	mTime2 := now.AddDate(0, -1, -2)
	dServicePlanAllocations := []interface{}{
		&ServicePlanAllocation{
			ObjMeta: ObjMeta{
				MetaObjID:        uuid.NewV4().String(),
				MetaVersion:      1,
				MetaTimeCreated:  now.AddDate(-1, -2, -1),
				MetaTimeModified: mTime1,
			},
			CspDomainID:             "cspDomainId",
			ReservableCapacityBytes: 1000000000,
			ServicePlanAllocationCreateOnce: ServicePlanAllocationCreateOnce{
				AccountID:           "ownerAccountId",
				AuthorizedAccountID: "authorizedAccountId",
				ClusterID:           "clusterId",
				ServicePlanID:       "servicePlanId",
			},
			ServicePlanAllocationCreateMutable: ServicePlanAllocationCreateMutable{
				Messages: []TimestampedString{},
				ProvisioningHints: StringValueMap{
					"attr1": ValueType{Kind: "INT", Value: "2"},
				},
				ReservationState: "UNKNOWN",
				StorageFormula:   "Formula1",
				StorageReservations: StorageTypeReservationMap{
					"prov1": {NumMirrors: 1, SizeBytes: 1},
					"prov2": {NumMirrors: 2, SizeBytes: 22},
					"prov3": {NumMirrors: 3, SizeBytes: 333},
				},
				TotalCapacityBytes: 200000000000,
			},
		},
		&ServicePlanAllocation{
			ObjMeta: ObjMeta{
				MetaObjID:        uuid.NewV4().String(),
				MetaVersion:      1,
				MetaTimeCreated:  now.AddDate(0, -2, -1),
				MetaTimeModified: mTime2,
			},
			CspDomainID:             "cspDomainId",
			ReservableCapacityBytes: 1000000001,
			ServicePlanAllocationCreateOnce: ServicePlanAllocationCreateOnce{
				AccountID:           "ownerAccountId",
				AuthorizedAccountID: "authorizedAccountId",
				ClusterID:           "clusterId",
				ServicePlanID:       "servicePlanId",
			},
			ServicePlanAllocationCreateMutable: ServicePlanAllocationCreateMutable{
				Messages: []TimestampedString{},
				ProvisioningHints: StringValueMap{
					"attr1": ValueType{Kind: "INT", Value: "1"},
				},
				ReservationState: "UNKNOWN",
				StorageFormula:   "Formula1",
				StorageReservations: StorageTypeReservationMap{
					"prov1": {NumMirrors: 1, SizeBytes: 1},
					"prov2": {NumMirrors: 2, SizeBytes: 22},
					"prov3": {NumMirrors: 3, SizeBytes: 333},
				},
				TotalCapacityBytes: 200000000002,
			},
		},
	}
	ctx := context.Background()

	t.Log("case: success")
	params := service_plan_allocation.ServicePlanAllocationListParams{
		ReservationState:    []string{"OK", "UNKNOWN"},
		ReservationStateNot: []string{"NO_CAPACITY", "DISABLED"},
	}
	fArg := bson.M{
		"reservationstate": bson.M{"$nin": []string{"NO_CAPACITY", "DISABLED"}},
	}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhServicePlanAllocation, params, fArg, newConsumeObjFnMatcher(t, dServicePlanAllocations...)).Return(nil)
	odhServicePlanAllocation.crud = crud
	odhServicePlanAllocation.api = api
	odhServicePlanAllocation.log = l
	retMA, retErr := odhServicePlanAllocation.List(ctx, params)
	assert.NoError(retErr)
	assert.Len(retMA, len(dServicePlanAllocations))
	for i, mA := range retMA {
		o := dServicePlanAllocations[i].(*ServicePlanAllocation)
		assert.Equal(o.ToModel(), mA)
	}
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	params = service_plan_allocation.ServicePlanAllocationListParams{
		AccountID:           swag.String("Account"),
		AuthorizedAccountID: swag.String("AuthorizedAccount"),
		ClusterID:           swag.String("clusterId"),
		CspDomainID:         swag.String("cspDomainId"),
		ReservationState:    []string{"OK", "UNKNOWN"},
		ServicePlanID:       swag.String("servicePlanID"),
		StorageFormulaName:  swag.String("Formula1"),
		PoolID:              swag.String("PoolID"),
		Tags:                []string{"tag1", "tag2"},
		SystemTags:          []string{"stag1", "stag2"},
	}
	fArg = bson.M{
		"accountid":                  *params.AccountID,
		"authorizedaccountid":        *params.AuthorizedAccountID,
		"clusterid":                  *params.ClusterID,
		"cspdomainid":                *params.CspDomainID,
		"reservationstate":           bson.M{"$in": []string{"OK", "UNKNOWN"}},
		"serviceplanid":              *params.ServicePlanID,
		"storageformulaname":         *params.StorageFormulaName,
		"storagereservations.PoolID": bson.M{"$exists": true},
		"tags":                       bson.M{"$all": params.Tags},
		"systemtags":                 bson.M{"$all": params.SystemTags},
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhServicePlanAllocation, params, fArg, gomock.Any()).Return(errWrappedError)
	odhServicePlanAllocation.crud = crud
	odhServicePlanAllocation.api = api
	odhServicePlanAllocation.log = l
	retMA, retErr = odhServicePlanAllocation.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhServicePlanAllocation.api = api
	odhServicePlanAllocation.log = l
	retMA, retErr = odhServicePlanAllocation.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestServicePlanAllocationUpdate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	ua := &ds.UpdateArgs{
		ID:      "58290457-139b-4c55-b6e0-e76b4913ebb6",
		Version: 0,
		Attributes: []ds.UpdateAttr{
			{
				Name: "ReservableCapacityBytes",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			}, {
				Name: "Messages",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateAppend: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	param := &models.ServicePlanAllocationMutable{
		ServicePlanAllocationMutableAllOf0: models.ServicePlanAllocationMutableAllOf0{
			ReservableCapacityBytes: swag.Int64(1000),
		},
		ServicePlanAllocationCreateMutable: models.ServicePlanAllocationCreateMutable{
			Messages: []*models.TimestampedString{},
		},
	}
	mObj := &models.ServicePlanAllocation{ServicePlanAllocationMutable: *param}
	dObj := &ServicePlanAllocation{}
	dObj.FromModel(mObj)
	now := time.Now()
	dServicePlanAllocation := &ServicePlanAllocation{
		ObjMeta: ObjMeta{
			MetaObjID:        ua.ID,
			MetaTimeCreated:  now.AddDate(0, -2, -1),
			MetaTimeModified: now,
			MetaVersion:      1000,
		},
	}
	m := servicePlanAllocationMatcher(t, mockServicePlanAllocationUpdate).CtxObj(dObj).Return(dServicePlanAllocation).Matcher()
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhServicePlanAllocation, m, ua).Return(nil)
	odhServicePlanAllocation.crud = crud
	odhServicePlanAllocation.api = api
	odhServicePlanAllocation.log = l
	retMA, retErr := odhServicePlanAllocation.Update(ctx, ua, param)
	assert.NoError(retErr)
	assert.Equal(dServicePlanAllocation.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhServicePlanAllocation, m, ua).Return(ds.ErrorIDVerNotFound)
	odhServicePlanAllocation.crud = crud
	odhServicePlanAllocation.api = api
	odhServicePlanAllocation.log = l
	retMA, retErr = odhServicePlanAllocation.Update(ctx, ua, param)
	assert.Equal(ds.ErrorIDVerNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhServicePlanAllocation.api = api
	odhServicePlanAllocation.log = l
	retMA, retErr = odhServicePlanAllocation.Update(ctx, ua, param)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}
