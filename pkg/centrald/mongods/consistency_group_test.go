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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/consistency_group"
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

// gomock.Matcher for ConsistencyGroup
type mockConsistencyGroupMatchCtx int

const (
	mockConsistencyGroupInvalid mockConsistencyGroupMatchCtx = iota
	mockConsistencyGroupInsert
	mockConsistencyGroupFind
	mockConsistencyGroupUpdate
)

type mockConsistencyGroupMatcher struct {
	t      *testing.T
	ctx    mockConsistencyGroupMatchCtx
	ctxObj *ConsistencyGroup
	retObj *ConsistencyGroup
}

func newConsistencyGroupMatcher(t *testing.T, ctx mockConsistencyGroupMatchCtx, obj *ConsistencyGroup) gomock.Matcher {
	return consistencyGroupMatcher(t, ctx).CtxObj(obj)
}

// consistencyGroupMatcher creates a partially initialized mockConsistencyGroupMatcher
func consistencyGroupMatcher(t *testing.T, ctx mockConsistencyGroupMatchCtx) *mockConsistencyGroupMatcher {
	return &mockConsistencyGroupMatcher{t: t, ctx: ctx}
}

// CtxObj adds an ConsistencyGroup object
func (o *mockConsistencyGroupMatcher) CtxObj(ao *ConsistencyGroup) *mockConsistencyGroupMatcher {
	o.ctxObj = ao
	return o
}

// Return sets the update result object
func (o *mockConsistencyGroupMatcher) Return(ro *ConsistencyGroup) *mockConsistencyGroupMatcher {
	o.retObj = ro
	return o
}

// Matches is from gomock.Matcher
func (o *mockConsistencyGroupMatcher) Matches(x interface{}) bool {
	var obj *ConsistencyGroup
	switch z := x.(type) {
	case *ConsistencyGroup:
		obj = z
	default:
		panic("Did not get a recognizable object: " + reflect.TypeOf(x).String())
	}
	assert := assert.New(o.t)
	switch o.ctx {
	case mockConsistencyGroupInsert:
		compObj := o.ctxObj
		compObj.ObjMeta = obj.ObjMeta // insert sets meta data
		return assert.NotZero(obj.ObjMeta.MetaObjID) &&
			assert.Equal(int32(1), obj.ObjMeta.MetaVersion) &&
			assert.False(obj.ObjMeta.MetaTimeCreated.IsZero()) &&
			assert.False(obj.ObjMeta.MetaTimeModified.IsZero()) &&
			assert.Equal(obj.ObjMeta.MetaTimeCreated, obj.ObjMeta.MetaTimeModified) &&
			assert.Equal(compObj, obj)
	case mockConsistencyGroupFind:
		*obj = *o.ctxObj
		return true
	case mockConsistencyGroupUpdate:
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
func (o *mockConsistencyGroupMatcher) String() string {
	switch o.ctx {
	case mockConsistencyGroupInsert:
		return "matches on insert"
	}
	return "unknown context"
}

func TestConsistencyGroupHandler(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	assert.Equal("consistencygroup", odhConsistencyGroup.CName())
	indexes := odhConsistencyGroup.Indexes()
	expIndexes := []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: accountIDKey, Value: IndexAscending}, {Key: nameKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: applicationGroupIdsKey, Value: IndexAscending}}},
		{Keys: bsonx.Doc{{Key: tenantAccountIDKey, Value: IndexAscending}}},
	}
	assert.Equal(expIndexes, indexes)
	assert.Equal(indexes, odhConsistencyGroup.indexes)
	dIf := odhConsistencyGroup.NewObject()
	assert.NotNil(dIf)
	_, ok := dIf.(*ConsistencyGroup)
	assert.True(ok)

	t.Log("case: Claim + Initialize success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ctx := context.Background()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	odhConsistencyGroup.Claim(api, crud)
	assert.Equal(api, odhConsistencyGroup.api)
	assert.Equal(crud, odhConsistencyGroup.crud)
	assert.Equal(l, odhConsistencyGroup.log)
	assert.Equal(odhConsistencyGroup, odhConsistencyGroup.Ops())
	crud.EXPECT().CreateIndexes(ctx, odhConsistencyGroup).Return(nil)
	assert.NoError(odhConsistencyGroup.Initialize(ctx))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize, error creating indexes")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhConsistencyGroup).Return(errUnknownError)
	odhConsistencyGroup.crud = crud
	odhConsistencyGroup.api = api
	assert.Error(errUnknownError, odhConsistencyGroup.Initialize(ctx))

	t.Log("case: Start")
	api.EXPECT().DBName().Return("dbName")
	assert.NoError(odhConsistencyGroup.Start(ctx))
}

func TestConsistencyGroupCount(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	t.Log("case: success")
	ctx := context.Background()
	params := consistency_group.ConsistencyGroupListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhConsistencyGroup, fArg, uint(0)).Return(2, nil)
	odhConsistencyGroup.crud = crud
	odhConsistencyGroup.api = api
	odhConsistencyGroup.log = l
	retMA, retErr := odhConsistencyGroup.Count(ctx, params, 0)
	assert.Equal(2, retMA)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: count fails")
	mockCtrl = gomock.NewController(t)
	params = consistency_group.ConsistencyGroupListParams{
		ApplicationGroupID: swag.String("ag1"),
		Tags:               []string{"tag1", "tag2"},
	}
	fArg = bson.M{
		"applicationgroupids": *params.ApplicationGroupID,
		"tags":                bson.M{"$all": params.Tags},
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhConsistencyGroup, fArg, uint(5)).Return(0, errWrappedError)
	odhConsistencyGroup.crud = crud
	odhConsistencyGroup.api = api
	odhConsistencyGroup.log = l
	retMA, retErr = odhConsistencyGroup.Count(ctx, params, 5)
	assert.Zero(retMA)
	assert.Equal(errWrappedError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhConsistencyGroup.api = api
	odhConsistencyGroup.log = l
	retMA, retErr = odhConsistencyGroup.Count(ctx, params, 0)
	assert.Equal(errWrappedError, retErr)
	assert.Zero(retMA)
}

func TestConsistencyGroupCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mCG := &models.ConsistencyGroup{
		ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
			AccountID:       "accountId",
			TenantAccountID: "tenant1",
		},
		ConsistencyGroupMutable: models.ConsistencyGroupMutable{
			Name:        "testconsistencyGroup",
			Description: "test description",
			Tags:        []string{"tag1", "tag2"},
			SystemTags:  []string{"stag1", "stag2"},
		},
	}
	dConsistencyGroup := &ConsistencyGroup{}
	dConsistencyGroup.FromModel(mCG) // initialize empty properties
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().InsertOne(ctx, odhConsistencyGroup, newConsistencyGroupMatcher(t, mockConsistencyGroupInsert, dConsistencyGroup)).Return(nil)
	odhConsistencyGroup.crud = crud
	odhConsistencyGroup.api = api
	odhConsistencyGroup.log = l
	retMA, retErr := odhConsistencyGroup.Create(ctx, mCG)
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
	crud.EXPECT().InsertOne(ctx, odhConsistencyGroup, newConsistencyGroupMatcher(t, mockConsistencyGroupInsert, dConsistencyGroup)).Return(ds.ErrorExists)
	odhConsistencyGroup.crud = crud
	odhConsistencyGroup.api = api
	odhConsistencyGroup.log = l
	retMA, retErr = odhConsistencyGroup.Create(ctx, mCG)
	assert.Equal(ds.ErrorExists, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhConsistencyGroup.api = api
	odhConsistencyGroup.log = l
	retMA, retErr = odhConsistencyGroup.Create(ctx, mCG)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestConsistencyGroupDelete(t *testing.T) {
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
	crud.EXPECT().DeleteOne(ctx, odhConsistencyGroup, mID).Return(nil)
	odhConsistencyGroup.crud = crud
	odhConsistencyGroup.api = api
	odhConsistencyGroup.log = l
	retErr := odhConsistencyGroup.Delete(ctx, mID)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().DeleteOne(ctx, odhConsistencyGroup, mID).Return(errUnknownError)
	odhConsistencyGroup.crud = crud
	odhConsistencyGroup.api = api
	odhConsistencyGroup.log = l
	retErr = odhConsistencyGroup.Delete(ctx, mID)
	assert.Equal(errUnknownError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhConsistencyGroup.api = api
	odhConsistencyGroup.log = l
	retErr = odhConsistencyGroup.Delete(ctx, mID)
	assert.Equal(errWrappedError, retErr)
}

func TestConsistencyGroupFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mTime := time.Now()
	cTime := mTime.AddDate(0, 0, -1)
	dConsistencyGroup := &ConsistencyGroup{
		ObjMeta: ObjMeta{
			MetaObjID:        "objectId",
			MetaTimeCreated:  cTime,
			MetaTimeModified: mTime,
		},
		AccountID:       "accountId",
		TenantAccountID: "tenant1",
		Name:            "testconsistencyGroup",
		Description:     "test description",
		Tags:            []string{"tag1", "tag2"},
		SystemTags:      []string{"stag1", "stag2"},
	}
	fArg := bson.M{objKey: dConsistencyGroup.ObjMeta.MetaObjID}
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhConsistencyGroup, fArg, newConsistencyGroupMatcher(t, mockConsistencyGroupFind, dConsistencyGroup)).Return(nil)
	odhConsistencyGroup.crud = crud
	odhConsistencyGroup.api = api
	odhConsistencyGroup.log = l
	retMA, retErr := odhConsistencyGroup.Fetch(ctx, dConsistencyGroup.ObjMeta.MetaObjID)
	assert.NoError(retErr)
	assert.Equal(dConsistencyGroup.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not found")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhConsistencyGroup, fArg, newConsistencyGroupMatcher(t, mockConsistencyGroupFind, dConsistencyGroup)).Return(ds.ErrorNotFound)
	odhConsistencyGroup.crud = crud
	odhConsistencyGroup.api = api
	odhConsistencyGroup.log = l
	retMA, retErr = odhConsistencyGroup.Fetch(ctx, dConsistencyGroup.ObjMeta.MetaObjID)
	assert.Equal(ds.ErrorNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhConsistencyGroup.api = api
	odhConsistencyGroup.log = l
	retMA, retErr = odhConsistencyGroup.Fetch(ctx, dConsistencyGroup.ObjMeta.MetaObjID)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestConsistencyGroupList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	now := time.Now()
	mTime1 := now.AddDate(-1, 0, 0)
	mTime2 := now.AddDate(0, -1, -2)
	dConsistencyGroups := []interface{}{
		&ConsistencyGroup{
			ObjMeta: ObjMeta{
				MetaObjID:        "objectId1",
				MetaTimeCreated:  now.AddDate(-1, -2, -1),
				MetaTimeModified: mTime1,
			},
			AccountID:       "accountId1",
			TenantAccountID: "tenant1",
			Name:            "consistencyGroup1",
			Description:     "consistencyGroup1 description",
			Tags:            []string{"tag1", "tag2"},
			SystemTags:      []string{"stag1", "stag2"},
		},
		&ConsistencyGroup{
			ObjMeta: ObjMeta{
				MetaObjID:        "objectId2",
				MetaTimeCreated:  now.AddDate(0, -2, -1),
				MetaTimeModified: mTime2,
			},
			AccountID:       "accountId2",
			TenantAccountID: "tenant1",
			Name:            "consistencyGroup2",
			Description:     "consistencyGroup2 description",
		},
	}
	ctx := context.Background()

	t.Log("case: success")
	params := consistency_group.ConsistencyGroupListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhConsistencyGroup, params, fArg, newConsumeObjFnMatcher(t, dConsistencyGroups...)).Return(nil)
	odhConsistencyGroup.crud = crud
	odhConsistencyGroup.api = api
	odhConsistencyGroup.log = l
	retMA, retErr := odhConsistencyGroup.List(ctx, params)
	assert.NoError(retErr)
	assert.Len(retMA, len(dConsistencyGroups))
	for i, mA := range retMA {
		o := dConsistencyGroups[i].(*ConsistencyGroup)
		assert.Equal(o.ToModel(), mA)
	}
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	params = consistency_group.ConsistencyGroupListParams{
		Name:      swag.String("consistencyGroup"),
		AccountID: swag.String("accountId"),
	}
	fArg = bson.M{
		"name":      *params.Name,
		"accountid": *params.AccountID,
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhConsistencyGroup, params, fArg, gomock.Any()).Return(errWrappedError)
	odhConsistencyGroup.crud = crud
	odhConsistencyGroup.api = api
	odhConsistencyGroup.log = l
	retMA, retErr = odhConsistencyGroup.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	// case: more query combinations
	qTcs := []struct {
		p consistency_group.ConsistencyGroupListParams
		q bson.M
	}{
		{ // [0]
			consistency_group.ConsistencyGroupListParams{
				TenantAccountID:    swag.String("tenant1"),
				ApplicationGroupID: swag.String("app"),
				Tags:               []string{"tag1", "tag2"},
				SystemTags:         []string{"stag1", "stag2"},
			},
			bson.M{
				"tenantaccountid":     "tenant1",
				"applicationgroupids": "app",
				"tags":                bson.M{"$all": []string{"tag1", "tag2"}},
				"systemtags":          bson.M{"$all": []string{"stag1", "stag2"}},
			},
		},
		{ // [1] both accounts
			consistency_group.ConsistencyGroupListParams{
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
		q := odhConsistencyGroup.convertListParams(tc.p)
		assert.Equal(tc.q, q, "Query[%d]", i)
	}

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhConsistencyGroup.api = api
	odhConsistencyGroup.log = l
	retMA, retErr = odhConsistencyGroup.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestConsistencyGroupUpdate(t *testing.T) {
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
	param := &models.ConsistencyGroupMutable{
		Name: "newName",
		Tags: []string{"tag1", "tag2"},
	}
	mObj := &models.ConsistencyGroup{ConsistencyGroupMutable: *param}
	dObj := &ConsistencyGroup{}
	dObj.FromModel(mObj)
	now := time.Now()
	dConsistencyGroup := &ConsistencyGroup{
		ObjMeta: ObjMeta{
			MetaObjID:        ua.ID,
			MetaTimeCreated:  now.AddDate(0, -2, -1),
			MetaTimeModified: now,
			MetaVersion:      8,
		},
	}
	m := consistencyGroupMatcher(t, mockConsistencyGroupUpdate).CtxObj(dObj).Return(dConsistencyGroup)
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhConsistencyGroup, m, ua).Return(nil)
	odhConsistencyGroup.crud = crud
	odhConsistencyGroup.api = api
	odhConsistencyGroup.log = l
	retMA, retErr := odhConsistencyGroup.Update(ctx, ua, param)
	assert.NoError(retErr)
	assert.Equal(dConsistencyGroup.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhConsistencyGroup, m, ua).Return(ds.ErrorIDVerNotFound)
	odhConsistencyGroup.crud = crud
	odhConsistencyGroup.api = api
	odhConsistencyGroup.log = l
	retMA, retErr = odhConsistencyGroup.Update(ctx, ua, param)
	assert.Equal(ds.ErrorIDVerNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhConsistencyGroup.api = api
	odhConsistencyGroup.log = l
	retMA, retErr = odhConsistencyGroup.Update(ctx, ua, param)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}
