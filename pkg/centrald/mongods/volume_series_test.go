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
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// gomock.Matcher for VolumeSeries
type mockVolumeSeriesMatchCtx int

const (
	mockVolumeSeriesInvalid mockVolumeSeriesMatchCtx = iota
	mockVolumeSeriesInsert
	mockVolumeSeriesFind
	mockVolumeSeriesUpdate
)

type mockVolumeSeriesMatcher struct {
	t      *testing.T
	ctx    mockVolumeSeriesMatchCtx
	ctxObj *VolumeSeries
	retObj *VolumeSeries
}

func newVolumeSeriesMatcher(t *testing.T, ctx mockVolumeSeriesMatchCtx, obj *VolumeSeries) gomock.Matcher {
	return volumeSeriesMatcher(t, ctx).CtxObj(obj).Matcher()
}

// volumeSeriesMatcher creates a partially initialized mockVolumeSeriesMatcher
func volumeSeriesMatcher(t *testing.T, ctx mockVolumeSeriesMatchCtx) *mockVolumeSeriesMatcher {
	return &mockVolumeSeriesMatcher{t: t, ctx: ctx}
}

// Matcher ends the chain
func (o *mockVolumeSeriesMatcher) Matcher() gomock.Matcher {
	if o.ctx == mockVolumeSeriesInsert || o.ctx == mockVolumeSeriesFind || o.ctx == mockVolumeSeriesUpdate {
		assert.NotNil(o.t, o.ctxObj)
	}
	return o
}

// CtxObj adds an VolumeSeries object
func (o *mockVolumeSeriesMatcher) CtxObj(ao *VolumeSeries) *mockVolumeSeriesMatcher {
	o.ctxObj = ao
	return o
}

// Return sets the update result object
func (o *mockVolumeSeriesMatcher) Return(ro *VolumeSeries) *mockVolumeSeriesMatcher {
	o.retObj = ro
	return o
}

// Matches is from gomock.Matcher
func (o *mockVolumeSeriesMatcher) Matches(x interface{}) bool {
	var obj *VolumeSeries
	switch z := x.(type) {
	case *VolumeSeries:
		obj = z
	default:
		panic("Did not get a recognizable object: " + reflect.TypeOf(x).String())
	}
	assert := assert.New(o.t)
	switch o.ctx {
	case mockVolumeSeriesInsert:
		compObj := o.ctxObj
		compObj.ObjMeta = obj.ObjMeta // insert sets meta data
		compObj.RootParcelUUID = obj.RootParcelUUID
		return assert.NotZero(obj.ObjMeta.MetaObjID) &&
			assert.Equal(int32(1), obj.ObjMeta.MetaVersion) &&
			assert.False(obj.ObjMeta.MetaTimeCreated.IsZero()) &&
			assert.False(obj.ObjMeta.MetaTimeModified.IsZero()) &&
			assert.Equal(obj.ObjMeta.MetaTimeCreated, obj.ObjMeta.MetaTimeModified) &&
			assert.NotZero(obj.RootParcelUUID) &&
			assert.Equal(compObj, obj)
	case mockVolumeSeriesFind:
		*obj = *o.ctxObj
		return true
	case mockVolumeSeriesUpdate:
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
func (o *mockVolumeSeriesMatcher) String() string {
	switch o.ctx {
	case mockVolumeSeriesInsert:
		return "matches on insert"
	}
	return "unknown context"
}

func TestVolumeSeriesHandler(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	assert.Equal("volumeseries", odhVolumeSeries.CName())
	indexes := odhVolumeSeries.Indexes()
	expIndexes := []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: accountIDKey, Value: IndexAscending}, {Key: nameKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: tenantAccountIDKey, Value: IndexAscending}}},
		{Keys: bsonx.Doc{{Key: consistencyGroupIDKey, Value: IndexAscending}}},
		{Keys: bsonx.Doc{{Key: servicePlanIDKey, Value: IndexAscending}}},
	}
	assert.Equal(expIndexes, indexes)
	assert.Equal(indexes, odhVolumeSeries.indexes)
	dIf := odhVolumeSeries.NewObject()
	assert.NotNil(dIf)
	_, ok := dIf.(*VolumeSeries)
	assert.True(ok)

	t.Log("case: Claim + Initialize success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ctx := context.Background()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	odhVolumeSeries.Claim(api, crud)
	assert.Equal(api, odhVolumeSeries.api)
	assert.Equal(crud, odhVolumeSeries.crud)
	assert.Equal(l, odhVolumeSeries.log)
	assert.Equal(odhVolumeSeries, odhVolumeSeries.Ops())
	crud.EXPECT().CreateIndexes(ctx, odhVolumeSeries).Return(nil)
	assert.NoError(odhVolumeSeries.Initialize(ctx))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize, error creating indexes")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhVolumeSeries).Return(errUnknownError)
	odhVolumeSeries.crud = crud
	odhVolumeSeries.api = api
	assert.Error(errUnknownError, odhVolumeSeries.Initialize(ctx))

	t.Log("case: Start")
	api.EXPECT().DBName().Return("dbName")
	assert.NoError(odhVolumeSeries.Start(ctx))
}

func TestVolumeSeriesNewID(t *testing.T) {
	assert := assert.New(t)

	id := odhVolumeSeries.NewID()
	assert.NotEmpty(id)
}

func TestVolumeSeriesAggregate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	t.Log("case: success")
	ctx := context.Background()
	parcelPath := "storageParcels.12c4c95e-e5b0-4302-ad17-797e02f80edd.sizeBytes"
	params := volume_series.VolumeSeriesListParams{Sum: []string{"sizeBytes", parcelPath}}
	fArg := bson.M{}
	exp := []*ds.Aggregation{
		&ds.Aggregation{FieldPath: "sizeBytes", Type: "sum", Value: 1010101010101},
		&ds.Aggregation{FieldPath: parcelPath, Type: "sum", Value: 101010101010},
	}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Aggregate(ctx, odhVolumeSeries, &params, fArg).Return(exp, 5, nil)
	odhVolumeSeries.crud = crud
	odhVolumeSeries.api = api
	odhVolumeSeries.log = l
	retMA, retC, retErr := odhVolumeSeries.Aggregate(ctx, params)
	assert.Equal(exp, retMA)
	assert.Equal(5, retC)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	params = volume_series.VolumeSeriesListParams{
		ServicePlanID: swag.String("plan1"),
		Sum:           []string{"sizeBytes"},
	}
	fArg = bson.M{"serviceplanid": "plan1"}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Aggregate(ctx, odhVolumeSeries, &params, fArg).Return(nil, 0, errWrappedError)
	odhVolumeSeries.crud = crud
	odhVolumeSeries.api = api
	odhVolumeSeries.log = l
	retMA, retC, retErr = odhVolumeSeries.Aggregate(ctx, params)
	assert.Nil(retMA)
	assert.Zero(retC)
	assert.Equal(errWrappedError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhVolumeSeries.api = api
	odhVolumeSeries.log = l
	retMA, retC, retErr = odhVolumeSeries.Aggregate(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
	assert.Zero(retC)
}

func TestVolumeSeriesCount(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	t.Log("case: success")
	ctx := context.Background()
	params := volume_series.VolumeSeriesListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhVolumeSeries, fArg, uint(0)).Return(2, nil)
	odhVolumeSeries.crud = crud
	odhVolumeSeries.api = api
	odhVolumeSeries.log = l
	retMA, retErr := odhVolumeSeries.Count(ctx, params, 0)
	assert.Equal(2, retMA)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: count fails")
	mockCtrl = gomock.NewController(t)
	params = volume_series.VolumeSeriesListParams{
		NamePattern: swag.String("volume1"),
		Tags:        []string{"tag1", "tag2"},
	}
	fArg = bson.M{
		"name": primitive.Regex{Pattern: *params.NamePattern},
		"tags": bson.M{"$all": params.Tags},
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhVolumeSeries, fArg, uint(5)).Return(0, errWrappedError)
	odhVolumeSeries.crud = crud
	odhVolumeSeries.api = api
	odhVolumeSeries.log = l
	retMA, retErr = odhVolumeSeries.Count(ctx, params, 5)
	assert.Zero(retMA)
	assert.Equal(errWrappedError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhVolumeSeries.api = api
	odhVolumeSeries.log = l
	retMA, retErr = odhVolumeSeries.Count(ctx, params, 0)
	assert.Equal(errWrappedError, retErr)
	assert.Zero(retMA)
}

func TestVolumeSeriesCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mVCA := &models.VolumeSeriesCreateArgs{
		VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
			AccountID:       "accountId",
			TenantAccountID: "tenant1",
		},
		VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
			Name:               "testvolumeSeries",
			Description:        "test description",
			ConsistencyGroupID: "con",
			ServicePlanID:      "servicePlanId",
			SizeBytes:          swag.Int64(200000000000),
			SpaAdditionalBytes: swag.Int64(1000),
			Tags:               []string{"tag1", "tag2"},
			SystemTags:         []string{"stag1", "stag2"},
		},
	}
	mV := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{ObjType: "VolumeSeries"},
		},
		VolumeSeriesCreateOnce: mVCA.VolumeSeriesCreateOnce,
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				Messages:          []*models.TimestampedString{},
				VolumeSeriesState: ds.DefaultVolumeSeriesState,
			},
			VolumeSeriesCreateMutable: mVCA.VolumeSeriesCreateMutable,
		},
	}
	dVolumeSeries := &VolumeSeries{}
	dVolumeSeries.FromModel(mV) // initialize empty properties
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().InsertOne(ctx, odhVolumeSeries, newVolumeSeriesMatcher(t, mockVolumeSeriesInsert, dVolumeSeries)).Return(nil)
	odhVolumeSeries.crud = crud
	odhVolumeSeries.api = api
	odhVolumeSeries.log = l
	retMA, retErr := odhVolumeSeries.Create(ctx, mVCA)
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
	crud.EXPECT().InsertOne(ctx, odhVolumeSeries, newVolumeSeriesMatcher(t, mockVolumeSeriesInsert, dVolumeSeries)).Return(ds.ErrorExists)
	odhVolumeSeries.crud = crud
	odhVolumeSeries.api = api
	odhVolumeSeries.log = l
	retMA, retErr = odhVolumeSeries.Create(ctx, mVCA)
	assert.Equal(ds.ErrorExists, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: create with specific identifier")
	mockCtrl = gomock.NewController(t)
	newID := "NewID"
	var mVCA2 *models.VolumeSeriesCreateArgs
	testutils.Clone(mVCA, &mVCA2)
	mVCA2.SystemTags = append(mVCA2.SystemTags, fmt.Sprintf("%s:%s", common.SystemTagVolumeSeriesID, newID))
	dVolumeSeries = &VolumeSeries{} // reset the returned object
	dVolumeSeries.FromModel(mV)     // against mVCA (without this tag)
	dVolumeSeries.MetaObjID = newID // and expect this ID
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().InsertOne(ctx, odhVolumeSeries, newVolumeSeriesMatcher(t, mockVolumeSeriesInsert, dVolumeSeries)).Return(nil)
	odhVolumeSeries.crud = crud
	odhVolumeSeries.api = api
	odhVolumeSeries.log = l
	retMA, retErr = odhVolumeSeries.Create(ctx, mVCA2)
	assert.NoError(retErr)
	assert.NotZero(retMA.Meta.ID)
	assert.Equal(models.ObjVersion(1), retMA.Meta.Version)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhVolumeSeries.api = api
	odhVolumeSeries.log = l
	retMA, retErr = odhVolumeSeries.Create(ctx, mVCA)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestVolumeSeriesDelete(t *testing.T) {
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
	crud.EXPECT().DeleteOne(ctx, odhVolumeSeries, mID).Return(nil)
	odhVolumeSeries.crud = crud
	odhVolumeSeries.api = api
	odhVolumeSeries.log = l
	retErr := odhVolumeSeries.Delete(ctx, mID)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().DeleteOne(ctx, odhVolumeSeries, mID).Return(errUnknownError)
	odhVolumeSeries.crud = crud
	odhVolumeSeries.api = api
	odhVolumeSeries.log = l
	retErr = odhVolumeSeries.Delete(ctx, mID)
	assert.Equal(errUnknownError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhVolumeSeries.api = api
	odhVolumeSeries.log = l
	retErr = odhVolumeSeries.Delete(ctx, mID)
	assert.Equal(errWrappedError, retErr)
}

func TestVolumeSeriesFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mTime := time.Now()
	cTime := mTime.AddDate(0, 0, -1)
	dVolumeSeries := &VolumeSeries{
		ObjMeta: ObjMeta{
			MetaObjID:        "objectId",
			MetaTimeCreated:  cTime,
			MetaTimeModified: mTime,
		},
		VolumeSeriesCreateOnceFields: VolumeSeriesCreateOnceFields{
			AccountID:       "accountId",
			TenantAccountID: "tenant1",
		},
		VolumeSeriesCreateMutableFields: VolumeSeriesCreateMutableFields{
			Name:               "testvolumeSeries",
			Description:        "test description",
			SizeBytes:          int64(200000000000),
			SpaAdditionalBytes: int64(1000),
			ConsistencyGroupID: "con",
			ServicePlanID:      "servicePlan1",
			Tags:               []string{"tag1", "tag2"},
			SystemTags:         []string{"stag1", "stag2"},
		},
		ServicePlanAllocationID: "spa1",
		Messages: []TimestampedString{
			{Message: "message1", Time: mTime},
			{Message: "message2", Time: mTime},
			{Message: "message3", Time: mTime},
		},
		StorageParcels: ParcelAllocationMap{
			"storageId1": ParcelAllocation{SizeBytes: 1},
			"storageId2": ParcelAllocation{SizeBytes: 22},
			"storageId3": ParcelAllocation{SizeBytes: 333},
		},
	}
	fArg := bson.M{objKey: dVolumeSeries.ObjMeta.MetaObjID}
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhVolumeSeries, fArg, newVolumeSeriesMatcher(t, mockVolumeSeriesFind, dVolumeSeries)).Return(nil)
	odhVolumeSeries.crud = crud
	odhVolumeSeries.api = api
	odhVolumeSeries.log = l
	retMA, retErr := odhVolumeSeries.Fetch(ctx, dVolumeSeries.ObjMeta.MetaObjID)
	assert.NoError(retErr)
	assert.Equal(dVolumeSeries.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not found")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhVolumeSeries, fArg, newVolumeSeriesMatcher(t, mockVolumeSeriesFind, dVolumeSeries)).Return(ds.ErrorNotFound)
	odhVolumeSeries.crud = crud
	odhVolumeSeries.api = api
	odhVolumeSeries.log = l
	retMA, retErr = odhVolumeSeries.Fetch(ctx, dVolumeSeries.ObjMeta.MetaObjID)
	assert.Equal(ds.ErrorNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhVolumeSeries.api = api
	odhVolumeSeries.log = l
	retMA, retErr = odhVolumeSeries.Fetch(ctx, dVolumeSeries.ObjMeta.MetaObjID)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestVolumeSeriesList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	now := time.Now()
	mTime1 := now.AddDate(-1, 0, 0)
	mTime2 := now.AddDate(0, -1, -2)
	dVolumeSeries := []interface{}{
		&VolumeSeries{
			ObjMeta: ObjMeta{
				MetaObjID:        "objectId1",
				MetaTimeCreated:  now.AddDate(-1, -2, -1),
				MetaTimeModified: mTime1,
			},
			VolumeSeriesCreateOnceFields: VolumeSeriesCreateOnceFields{
				AccountID:       "accountId1",
				TenantAccountID: "tenant1",
			},
			VolumeSeriesCreateMutableFields: VolumeSeriesCreateMutableFields{
				Name:               "volumeSeries1",
				Description:        "volumeSeries1 description",
				SizeBytes:          int64(200000000000),
				SpaAdditionalBytes: int64(1000),
				ConsistencyGroupID: "con",
				ServicePlanID:      "servicePlan1",
				Tags:               []string{"tag1", "tag2"},
				SystemTags:         []string{"stag1", "stag2"},
			},
			ServicePlanAllocationID: "spa1",
			Messages: []TimestampedString{
				{Message: "message1", Time: mTime1},
				{Message: "message2", Time: mTime1},
				{Message: "message3", Time: mTime1},
			},
			StorageParcels: ParcelAllocationMap{
				"storageId1": ParcelAllocation{SizeBytes: 1},
				"storageId2": ParcelAllocation{SizeBytes: 22},
				"storageId3": ParcelAllocation{SizeBytes: 333},
			},
		},
		&VolumeSeries{
			ObjMeta: ObjMeta{
				MetaObjID:        "objectId2",
				MetaTimeCreated:  now.AddDate(0, -2, -1),
				MetaTimeModified: mTime2,
			},
			VolumeSeriesCreateOnceFields: VolumeSeriesCreateOnceFields{
				AccountID:       "accountId2",
				TenantAccountID: "tenant1",
			},
			VolumeSeriesCreateMutableFields: VolumeSeriesCreateMutableFields{
				Name:               "volumeSeries2",
				Description:        "volumeSeries2 description",
				ConsistencyGroupID: "con",
				ServicePlanID:      "servicePlan1",
				SizeBytes:          int64(200000000000),
			},
			ServicePlanAllocationID: "spa1",
			Messages: []TimestampedString{
				{Message: "message1", Time: mTime2},
				{Message: "message2", Time: mTime2},
			},
			StorageParcels: ParcelAllocationMap{
				"storageId1": ParcelAllocation{SizeBytes: 11},
				"storageId2": ParcelAllocation{SizeBytes: 222},
				"storageId4": ParcelAllocation{SizeBytes: 4444},
			},
		},
	}
	ctx := context.Background()

	t.Log("case: success")
	params := volume_series.VolumeSeriesListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhVolumeSeries, params, fArg, newConsumeObjFnMatcher(t, dVolumeSeries...)).Return(nil)
	odhVolumeSeries.crud = crud
	odhVolumeSeries.api = api
	odhVolumeSeries.log = l
	retMA, retErr := odhVolumeSeries.List(ctx, params)
	assert.NoError(retErr)
	assert.Len(retMA, len(dVolumeSeries))
	for i, mA := range retMA {
		o := dVolumeSeries[i].(*VolumeSeries)
		assert.Equal(o.ToModel(), mA)
	}
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	nowDT := strfmt.DateTime(now)
	params = volume_series.VolumeSeriesListParams{
		NamePattern:        swag.String("volumeSeries.*"),
		AccountID:          swag.String("accountId"),
		BoundClusterID:     swag.String("cl1"),
		BoundCspDomainID:   swag.String("csp1"),
		ConfiguredNodeID:   swag.String("node1"),
		MountedNodeID:      swag.String("node1"),
		Tags:               []string{"tag1", "tag2"},
		SystemTags:         []string{"stag1", "stag2"},
		VolumeSeriesState:  []string{"CONFIGURED", "BOUND"},
		NextSnapshotTimeLE: &nowDT,
	}
	fArg = bson.M{
		"name":                 primitive.Regex{Pattern: *params.NamePattern},
		"accountid":            *params.AccountID,
		"boundclusterid":       *params.BoundClusterID,
		"boundcspdomainid":     *params.BoundCspDomainID,
		"configurednodeid":     *params.ConfiguredNodeID,
		"mounts.mountednodeid": *params.MountedNodeID,
		"tags":                 bson.M{"$all": params.Tags},
		"systemtags":           bson.M{"$all": params.SystemTags},
		"volumeseriesstate":    bson.M{"$in": params.VolumeSeriesState},
		"lifecyclemanagementdata.nextsnapshottime": bson.M{"$lte": now},
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhVolumeSeries, params, fArg, gomock.Any()).Return(errWrappedError)
	odhVolumeSeries.crud = crud
	odhVolumeSeries.api = api
	odhVolumeSeries.log = l
	retMA, retErr = odhVolumeSeries.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	// case: more query combinations
	qTcs := []struct {
		p volume_series.VolumeSeriesListParams
		q bson.M
	}{
		{ // [0]
			volume_series.VolumeSeriesListParams{
				Name:                    swag.String("vol"), // overrides NamePattern
				NamePattern:             swag.String("volumeSeries.*"),
				ServicePlanID:           swag.String("servicePlanId"),
				ServicePlanAllocationID: swag.String("servicePlanAllocationID"),
				StorageID:               swag.String("storageId1"),
				ConsistencyGroupID:      swag.String("con"),
				PoolID:                  swag.String("prov1"),
				TenantAccountID:         swag.String("tenant1"),
				Tags:                    []string{},
				VolumeSeriesState:       []string{"CONFIGURED", "BOUND"},
				VolumeSeriesStateNot:    []string{"DELETING"}, // overrides VolumeSeriesState
			},
			bson.M{
				"name":                      "vol",
				"capacityallocations.prov1": bson.M{"$exists": true},
				"consistencygroupid":        "con",
				"serviceplanid":             "servicePlanId",
				"serviceplanallocationid":   "servicePlanAllocationID",
				"storageparcels.storageId1": bson.M{"$exists": true},
				"tenantaccountid":           "tenant1",
				"volumeseriesstate":         bson.M{"$nin": []string{"DELETING"}},
			},
		},
		{ // [1] both accounts
			volume_series.VolumeSeriesListParams{
				AccountID:       swag.String("accountId2"),
				TenantAccountID: swag.String("tenant1"),
				NamePattern:     swag.String("volumeSeries.*"),
			},
			bson.M{
				"name": primitive.Regex{Pattern: "volumeSeries.*"},
				"$or": []bson.M{
					bson.M{"accountid": "accountId2"},
					bson.M{"tenantaccountid": "tenant1"},
				},
			},
		},
	}
	for i, tc := range qTcs {
		q := odhVolumeSeries.convertListParams(tc.p)
		assert.Equal(tc.q, q, "Query[%d]", i)
	}

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhVolumeSeries.api = api
	odhVolumeSeries.log = l
	retMA, retErr = odhVolumeSeries.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestVolumeSeriesUpdate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	ua := &ds.UpdateArgs{
		ID:      "58290457-139b-4c55-b6e0-e76b4913ebb6",
		Version: 0,
		Attributes: []ds.UpdateAttr{
			{
				Name: "Name",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			}, {
				Name: "SizeBytes",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	param := &models.VolumeSeriesMutable{
		VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
			Name:      "newName",
			SizeBytes: swag.Int64(200000000000),
		},
	}
	mObj := &models.VolumeSeries{VolumeSeriesMutable: *param}
	dObj := &VolumeSeries{}
	dObj.FromModel(mObj)
	now := time.Now()
	dVolumeSeries := &VolumeSeries{
		ObjMeta: ObjMeta{
			MetaObjID:        ua.ID,
			MetaTimeCreated:  now.AddDate(0, -2, -1),
			MetaTimeModified: now,
			MetaVersion:      8,
		},
	}
	m := volumeSeriesMatcher(t, mockVolumeSeriesUpdate).CtxObj(dObj).Return(dVolumeSeries).Matcher()
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhVolumeSeries, m, ua).Return(nil)
	odhVolumeSeries.crud = crud
	odhVolumeSeries.api = api
	odhVolumeSeries.log = l
	retMA, retErr := odhVolumeSeries.Update(ctx, ua, param)
	assert.NoError(retErr)
	assert.Equal(dVolumeSeries.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhVolumeSeries, m, ua).Return(ds.ErrorIDVerNotFound)
	odhVolumeSeries.crud = crud
	odhVolumeSeries.api = api
	odhVolumeSeries.log = l
	retMA, retErr = odhVolumeSeries.Update(ctx, ua, param)
	assert.Equal(ds.ErrorIDVerNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhVolumeSeries.api = api
	odhVolumeSeries.log = l
	retMA, retErr = odhVolumeSeries.Update(ctx, ua, param)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}
