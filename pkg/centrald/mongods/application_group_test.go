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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/application_group"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// gomock.Matcher for ApplicationGroup
type mockApplicationGroupMatchCtx int

const (
	mockApplicationGroupInvalid mockApplicationGroupMatchCtx = iota
	mockApplicationGroupInsert
	mockApplicationGroupFind
	mockApplicationGroupUpdate
)

type mockApplicationGroupMatcher struct {
	t      *testing.T
	ctx    mockApplicationGroupMatchCtx
	ctxObj *ApplicationGroup
	retObj *ApplicationGroup
}

func newApplicationGroupMatcher(t *testing.T, ctx mockApplicationGroupMatchCtx, obj *ApplicationGroup) gomock.Matcher {
	return applicationGroupMatcher(t, ctx).CtxObj(obj)
}

// applicationGroupMatcher creates a partially initialized mockApplicationGroupMatcher
func applicationGroupMatcher(t *testing.T, ctx mockApplicationGroupMatchCtx) *mockApplicationGroupMatcher {
	return &mockApplicationGroupMatcher{t: t, ctx: ctx}
}

// CtxObj adds an ApplicationGroup object
func (o *mockApplicationGroupMatcher) CtxObj(ao *ApplicationGroup) *mockApplicationGroupMatcher {
	o.ctxObj = ao
	return o
}

// Return sets the update result object
func (o *mockApplicationGroupMatcher) Return(ro *ApplicationGroup) *mockApplicationGroupMatcher {
	o.retObj = ro
	return o
}

// Matches is from gomock.Matcher
func (o *mockApplicationGroupMatcher) Matches(x interface{}) bool {
	var obj *ApplicationGroup
	switch z := x.(type) {
	case *ApplicationGroup:
		obj = z
	default:
		panic("Did not get a recognizable object: " + reflect.TypeOf(x).String())
	}
	assert := assert.New(o.t)
	switch o.ctx {
	case mockApplicationGroupInsert:
		compObj := o.ctxObj
		compObj.ObjMeta = obj.ObjMeta // insert sets meta data
		return assert.NotZero(obj.ObjMeta.MetaObjID) &&
			assert.Equal(int32(1), obj.ObjMeta.MetaVersion) &&
			assert.False(obj.ObjMeta.MetaTimeCreated.IsZero()) &&
			assert.False(obj.ObjMeta.MetaTimeModified.IsZero()) &&
			assert.Equal(obj.ObjMeta.MetaTimeCreated, obj.ObjMeta.MetaTimeModified) &&
			assert.Equal(compObj, obj)
	case mockApplicationGroupFind:
		*obj = *o.ctxObj
		return true
	case mockApplicationGroupUpdate:
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
func (o *mockApplicationGroupMatcher) String() string {
	switch o.ctx {
	case mockApplicationGroupInsert:
		return "matches on insert"
	}
	return "unknown context"
}

func TestApplicationGroupHandler(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	assert.Equal("applicationgroup", odhApplicationGroup.CName())
	indexes := odhApplicationGroup.Indexes()
	expIndexes := []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: accountIDKey, Value: IndexAscending}, {Key: nameKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: tenantAccountIDKey, Value: IndexAscending}}},
	}
	assert.Equal(expIndexes, indexes)
	assert.Equal(indexes, odhApplicationGroup.indexes)
	dIf := odhApplicationGroup.NewObject()
	assert.NotNil(dIf)
	_, ok := dIf.(*ApplicationGroup)
	assert.True(ok)

	t.Log("case: Claim + Initialize success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ctx := context.Background()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	odhApplicationGroup.Claim(api, crud)
	assert.Equal(api, odhApplicationGroup.api)
	assert.Equal(crud, odhApplicationGroup.crud)
	assert.Equal(l, odhApplicationGroup.log)
	assert.Equal(odhApplicationGroup, odhApplicationGroup.Ops())
	crud.EXPECT().CreateIndexes(ctx, odhApplicationGroup).Return(nil)
	assert.NoError(odhApplicationGroup.Initialize(ctx))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize, error creating indexes")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhApplicationGroup).Return(errUnknownError)
	odhApplicationGroup.crud = crud
	odhApplicationGroup.api = api
	assert.Error(errUnknownError, odhApplicationGroup.Initialize(ctx))

	t.Log("case: Start")
	api.EXPECT().DBName().Return("dbName")
	assert.NoError(odhApplicationGroup.Start(ctx))
}

func TestApplicationGroupCount(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	t.Log("case: success")
	ctx := context.Background()
	params := application_group.ApplicationGroupListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhApplicationGroup, fArg, uint(0)).Return(2, nil)
	odhApplicationGroup.crud = crud
	odhApplicationGroup.api = api
	odhApplicationGroup.log = l
	retMA, retErr := odhApplicationGroup.Count(ctx, params, 0)
	assert.Equal(2, retMA)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: count fails")
	mockCtrl = gomock.NewController(t)
	params = application_group.ApplicationGroupListParams{
		Name: swag.String("ag1"),
		Tags: []string{"tag1", "tag2"},
	}
	fArg = bson.M{
		"name": *params.Name,
		"tags": bson.M{"$all": params.Tags},
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhApplicationGroup, fArg, uint(5)).Return(0, errWrappedError)
	odhApplicationGroup.crud = crud
	odhApplicationGroup.api = api
	odhApplicationGroup.log = l
	retMA, retErr = odhApplicationGroup.Count(ctx, params, 5)
	assert.Zero(retMA)
	assert.Equal(errWrappedError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhApplicationGroup.api = api
	odhApplicationGroup.log = l
	retMA, retErr = odhApplicationGroup.Count(ctx, params, 0)
	assert.Equal(errWrappedError, retErr)
	assert.Zero(retMA)
}

func TestApplicationGroupCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mAG := &models.ApplicationGroup{
		ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
			AccountID:       "accountId",
			TenantAccountID: "tenant1",
		},
		ApplicationGroupMutable: models.ApplicationGroupMutable{
			Name:        "testapplicationGroup",
			Description: "test description",
			Tags:        []string{"tag1", "tag2"},
			SystemTags:  []string{"stag1", "stag2"},
		},
	}
	dApplicationGroup := &ApplicationGroup{}
	dApplicationGroup.FromModel(mAG) // initialize empty properties
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().InsertOne(ctx, odhApplicationGroup, newApplicationGroupMatcher(t, mockApplicationGroupInsert, dApplicationGroup)).Return(nil)
	odhApplicationGroup.crud = crud
	odhApplicationGroup.api = api
	odhApplicationGroup.log = l
	retMA, retErr := odhApplicationGroup.Create(ctx, mAG)
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
	crud.EXPECT().InsertOne(ctx, odhApplicationGroup, newApplicationGroupMatcher(t, mockApplicationGroupInsert, dApplicationGroup)).Return(ds.ErrorExists)
	odhApplicationGroup.crud = crud
	odhApplicationGroup.api = api
	odhApplicationGroup.log = l
	retMA, retErr = odhApplicationGroup.Create(ctx, mAG)
	assert.Equal(ds.ErrorExists, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhApplicationGroup.api = api
	odhApplicationGroup.log = l
	retMA, retErr = odhApplicationGroup.Create(ctx, mAG)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestApplicationGroupDelete(t *testing.T) {
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
	crud.EXPECT().DeleteOne(ctx, odhApplicationGroup, mID).Return(nil)
	odhApplicationGroup.crud = crud
	odhApplicationGroup.api = api
	odhApplicationGroup.log = l
	retErr := odhApplicationGroup.Delete(ctx, mID)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().DeleteOne(ctx, odhApplicationGroup, mID).Return(errUnknownError)
	odhApplicationGroup.crud = crud
	odhApplicationGroup.api = api
	odhApplicationGroup.log = l
	retErr = odhApplicationGroup.Delete(ctx, mID)
	assert.Equal(errUnknownError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhApplicationGroup.api = api
	odhApplicationGroup.log = l
	retErr = odhApplicationGroup.Delete(ctx, mID)
	assert.Equal(errWrappedError, retErr)
}

func TestApplicationGroupFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mTime := time.Now()
	cTime := mTime.AddDate(0, 0, -1)
	dApplicationGroup := &ApplicationGroup{
		ObjMeta: ObjMeta{
			MetaObjID:        "objectId",
			MetaTimeCreated:  cTime,
			MetaTimeModified: mTime,
		},
		AccountID:       "accountId",
		TenantAccountID: "tenant1",
		Name:            "testapplicationGroup",
		Description:     "test description",
		Tags:            []string{"tag1", "tag2"},
		SystemTags:      []string{"stag1", "stag2"},
	}
	fArg := bson.M{objKey: dApplicationGroup.ObjMeta.MetaObjID}
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhApplicationGroup, fArg, newApplicationGroupMatcher(t, mockApplicationGroupFind, dApplicationGroup)).Return(nil)
	odhApplicationGroup.crud = crud
	odhApplicationGroup.api = api
	odhApplicationGroup.log = l
	retMA, retErr := odhApplicationGroup.Fetch(ctx, dApplicationGroup.ObjMeta.MetaObjID)
	assert.NoError(retErr)
	assert.Equal(dApplicationGroup.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not found")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhApplicationGroup, fArg, newApplicationGroupMatcher(t, mockApplicationGroupFind, dApplicationGroup)).Return(ds.ErrorNotFound)
	odhApplicationGroup.crud = crud
	odhApplicationGroup.api = api
	odhApplicationGroup.log = l
	retMA, retErr = odhApplicationGroup.Fetch(ctx, dApplicationGroup.ObjMeta.MetaObjID)
	assert.Equal(ds.ErrorNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhApplicationGroup.api = api
	odhApplicationGroup.log = l
	retMA, retErr = odhApplicationGroup.Fetch(ctx, dApplicationGroup.ObjMeta.MetaObjID)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestApplicationGroupList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	now := time.Now()
	mTime1 := now.AddDate(-1, 0, 0)
	mTime2 := now.AddDate(0, -1, -2)
	dApplicationGroups := []interface{}{
		&ApplicationGroup{
			ObjMeta: ObjMeta{
				MetaObjID:        "objectId1",
				MetaTimeCreated:  now.AddDate(-1, -2, -1),
				MetaTimeModified: mTime1,
			},
			AccountID:       "accountId1",
			TenantAccountID: "tenant1",
			Name:            "applicationGroup1",
			Description:     "applicationGroup1 description",
			Tags:            []string{"tag1", "tag2"},
			SystemTags:      []string{"stag1", "stag2"},
		},
		&ApplicationGroup{
			ObjMeta: ObjMeta{
				MetaObjID:        "objectId2",
				MetaTimeCreated:  now.AddDate(0, -2, -1),
				MetaTimeModified: mTime2,
			},
			AccountID:       "accountId2",
			TenantAccountID: "tenant1",
			Name:            "applicationGroup2",
			Description:     "applicationGroup2 description",
		},
	}
	ctx := context.Background()

	t.Log("case: success")
	params := application_group.ApplicationGroupListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhApplicationGroup, params, fArg, newConsumeObjFnMatcher(t, dApplicationGroups...)).Return(nil)
	odhApplicationGroup.crud = crud
	odhApplicationGroup.api = api
	odhApplicationGroup.log = l
	retMA, retErr := odhApplicationGroup.List(ctx, params)
	assert.NoError(retErr)
	assert.Len(retMA, len(dApplicationGroups))
	for i, mA := range retMA {
		o := dApplicationGroups[i].(*ApplicationGroup)
		assert.Equal(o.ToModel(), mA)
	}
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	params = application_group.ApplicationGroupListParams{
		Name:      swag.String("applicationGroup"),
		AccountID: swag.String("accountId"),
	}
	fArg = bson.M{
		"name":      *params.Name,
		"accountid": *params.AccountID,
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhApplicationGroup, params, fArg, gomock.Any()).Return(errWrappedError)
	odhApplicationGroup.crud = crud
	odhApplicationGroup.api = api
	odhApplicationGroup.log = l
	retMA, retErr = odhApplicationGroup.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	// case: more query combinations
	qTcs := []struct {
		p application_group.ApplicationGroupListParams
		q bson.M
	}{
		{ // [0]
			application_group.ApplicationGroupListParams{
				TenantAccountID: swag.String("tenant1"),
				Tags:            []string{"tag1", "tag2"},
				SystemTags:      []string{"stag1", "stag2"},
			},
			bson.M{
				"tenantaccountid": "tenant1",
				"tags":            bson.M{"$all": []string{"tag1", "tag2"}},
				"systemtags":      bson.M{"$all": []string{"stag1", "stag2"}},
			},
		},
		{ // [1] both accounts
			application_group.ApplicationGroupListParams{
				AccountID:       swag.String("accountId2"),
				TenantAccountID: swag.String("tenant1"),
			},
			bson.M{
				"$or": []bson.M{
					bson.M{"accountid": "accountId2"},
					bson.M{"tenantaccountid": "tenant1"},
				},
			},
		},
	}
	for i, tc := range qTcs {
		q := odhApplicationGroup.convertListParams(tc.p)
		assert.Equal(tc.q, q, "Query[%d]", i)
	}

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhApplicationGroup.api = api
	odhApplicationGroup.log = l
	retMA, retErr = odhApplicationGroup.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestApplicationGroupUpdate(t *testing.T) {
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
				Name: "Tags",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateAppend: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	param := &models.ApplicationGroupMutable{
		Name: "newName",
		Tags: []string{"tag1", "tag2"},
	}
	mObj := &models.ApplicationGroup{ApplicationGroupMutable: *param}
	dObj := &ApplicationGroup{}
	dObj.FromModel(mObj)
	now := time.Now()
	dApplicationGroup := &ApplicationGroup{
		ObjMeta: ObjMeta{
			MetaObjID:        ua.ID,
			MetaTimeCreated:  now.AddDate(0, -2, -1),
			MetaTimeModified: now,
			MetaVersion:      8,
		},
	}
	m := applicationGroupMatcher(t, mockApplicationGroupUpdate).CtxObj(dObj).Return(dApplicationGroup)
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhApplicationGroup, m, ua).Return(nil)
	odhApplicationGroup.crud = crud
	odhApplicationGroup.api = api
	odhApplicationGroup.log = l
	retMA, retErr := odhApplicationGroup.Update(ctx, ua, param)
	assert.NoError(retErr)
	assert.Equal(dApplicationGroup.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhApplicationGroup, m, ua).Return(ds.ErrorIDVerNotFound)
	odhApplicationGroup.crud = crud
	odhApplicationGroup.api = api
	odhApplicationGroup.log = l
	retMA, retErr = odhApplicationGroup.Update(ctx, ua, param)
	assert.Equal(ds.ErrorIDVerNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhApplicationGroup.api = api
	odhApplicationGroup.log = l
	retMA, retErr = odhApplicationGroup.Update(ctx, ua, param)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}
