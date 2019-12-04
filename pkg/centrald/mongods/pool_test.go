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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/pool"
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

// gomock.Matcher for Pool
type mockPoolMatchCtx int

const (
	mockPoolInvalid mockPoolMatchCtx = iota
	mockPoolInsert
	mockPoolFind
	mockPoolUpdate
)

type mockPoolMatcher struct {
	t      *testing.T
	ctx    mockPoolMatchCtx
	ctxObj *Pool
	retObj *Pool
}

func newPoolMatcher(t *testing.T, ctx mockPoolMatchCtx, obj *Pool) gomock.Matcher {
	return poolMatcher(t, ctx).CtxObj(obj).Matcher()
}

// poolMatcher creates a partially initialized mockPoolMatcher
func poolMatcher(t *testing.T, ctx mockPoolMatchCtx) *mockPoolMatcher {
	return &mockPoolMatcher{t: t, ctx: ctx}
}

// Matcher ends the chain
func (o *mockPoolMatcher) Matcher() gomock.Matcher {
	if o.ctx == mockPoolInsert || o.ctx == mockPoolFind || o.ctx == mockPoolUpdate {
		assert.NotNil(o.t, o.ctxObj)
	}
	return o
}

// CtxObj adds an Pool object
func (o *mockPoolMatcher) CtxObj(ao *Pool) *mockPoolMatcher {
	o.ctxObj = ao
	return o
}

// Return sets the update result object
func (o *mockPoolMatcher) Return(ro *Pool) *mockPoolMatcher {
	o.retObj = ro
	return o
}

// Matches is from gomock.Matcher
func (o *mockPoolMatcher) Matches(x interface{}) bool {
	var obj *Pool
	switch z := x.(type) {
	case *Pool:
		obj = z
	default:
		panic("Did not get a recognizable object: " + reflect.TypeOf(x).String())
	}
	assert := assert.New(o.t)
	switch o.ctx {
	case mockPoolInsert:
		compObj := o.ctxObj
		compObj.ObjMeta = obj.ObjMeta // insert sets meta data
		return assert.NotZero(obj.ObjMeta.MetaObjID) &&
			assert.Equal(int32(1), obj.ObjMeta.MetaVersion) &&
			assert.False(obj.ObjMeta.MetaTimeCreated.IsZero()) &&
			assert.False(obj.ObjMeta.MetaTimeModified.IsZero()) &&
			assert.Equal(obj.ObjMeta.MetaTimeCreated, obj.ObjMeta.MetaTimeModified) &&
			assert.Equal(compObj, obj)
	case mockPoolFind:
		*obj = *o.ctxObj
		return true
	case mockPoolUpdate:
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
func (o *mockPoolMatcher) String() string {
	switch o.ctx {
	case mockPoolInsert:
		return "matches on insert"
	}
	return "unknown context"
}

func TestPoolHandler(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	assert.Equal("pool", odhPool.CName())
	indexes := odhPool.Indexes()
	expIndexes := []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: clusterIDKey, Value: IndexAscending}}},
		{Keys: bsonx.Doc{{Key: servicePlanAllocationIDKey, Value: IndexAscending}}},
	}
	assert.Equal(expIndexes, indexes)
	assert.Equal(indexes, odhPool.indexes)
	dIf := odhPool.NewObject()
	assert.NotNil(dIf)
	_, ok := dIf.(*Pool)
	assert.True(ok)

	t.Log("case: Claim + Initialize success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ctx := context.Background()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	odhPool.Claim(api, crud)
	assert.Equal(api, odhPool.api)
	assert.Equal(crud, odhPool.crud)
	assert.Equal(l, odhPool.log)
	assert.Equal(odhPool, odhPool.Ops())
	crud.EXPECT().CreateIndexes(ctx, odhPool).Return(nil)
	assert.NoError(odhPool.Initialize(ctx))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize, error creating indexes")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhPool).Return(errUnknownError)
	odhPool.crud = crud
	odhPool.api = api
	assert.Error(errUnknownError, odhPool.Initialize(ctx))

	t.Log("case: Start")
	api.EXPECT().DBName().Return("dbName")
	assert.NoError(odhPool.Start(ctx))
}

func TestPoolCount(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	t.Log("case: success")
	ctx := context.Background()
	params := pool.PoolListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhPool, fArg, uint(0)).Return(2, nil)
	odhPool.crud = crud
	odhPool.api = api
	odhPool.log = l
	retMA, retErr := odhPool.Count(ctx, params, 0)
	assert.Equal(2, retMA)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: count fails")
	mockCtrl = gomock.NewController(t)
	params = pool.PoolListParams{
		SystemTags: []string{"tag1", "tag2"},
	}
	fArg = bson.M{
		"systemtags": bson.M{"$all": params.SystemTags},
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhPool, fArg, uint(5)).Return(0, errWrappedError)
	odhPool.crud = crud
	odhPool.api = api
	odhPool.log = l
	retMA, retErr = odhPool.Count(ctx, params, 5)
	assert.Zero(retMA)
	assert.Equal(errWrappedError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhPool.api = api
	odhPool.log = l
	retMA, retErr = odhPool.Count(ctx, params, 0)
	assert.Equal(errWrappedError, retErr)
	assert.Zero(retMA)
}

func TestPoolCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mPCA := &models.PoolCreateArgs{
		PoolCreateOnce: models.PoolCreateOnce{
			CspDomainID:    "cspDomainId",
			CspStorageType: "EBS-GP2",
			// storage accessibility
			StorageAccessibility: &models.StorageAccessibilityMutable{
				AccessibilityScope:      "CSPDOMAIN",
				AccessibilityScopeObjID: "cspDomainId",
			},
		},
		PoolCreateMutable: models.PoolCreateMutable{
			SystemTags: []string{"stag1", "stag2"},
		},
	}
	mC := &models.Pool{
		PoolAllOf0: models.PoolAllOf0{
			Meta: &models.ObjMeta{ObjType: "Pool"},
		},
		PoolCreateOnce: mPCA.PoolCreateOnce,
		PoolMutable: models.PoolMutable{
			PoolCreateMutable: mPCA.PoolCreateMutable,
		},
	}
	dPool := &Pool{}
	dPool.FromModel(mC) // initialize empty properties
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().InsertOne(ctx, odhPool, newPoolMatcher(t, mockPoolInsert, dPool)).Return(nil)
	odhPool.crud = crud
	odhPool.api = api
	odhPool.log = l
	retMA, retErr := odhPool.Create(ctx, mPCA)
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
	crud.EXPECT().InsertOne(ctx, odhPool, newPoolMatcher(t, mockPoolInsert, dPool)).Return(ds.ErrorExists)
	odhPool.crud = crud
	odhPool.api = api
	odhPool.log = l
	retMA, retErr = odhPool.Create(ctx, mPCA)
	assert.Equal(ds.ErrorExists, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhPool.api = api
	odhPool.log = l
	retMA, retErr = odhPool.Create(ctx, mPCA)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestPoolDelete(t *testing.T) {
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
	crud.EXPECT().DeleteOne(ctx, odhPool, mID).Return(nil)
	odhPool.crud = crud
	odhPool.api = api
	odhPool.log = l
	retErr := odhPool.Delete(ctx, mID)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().DeleteOne(ctx, odhPool, mID).Return(errUnknownError)
	odhPool.crud = crud
	odhPool.api = api
	odhPool.log = l
	retErr = odhPool.Delete(ctx, mID)
	assert.Equal(errUnknownError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhPool.api = api
	odhPool.log = l
	retErr = odhPool.Delete(ctx, mID)
	assert.Equal(errWrappedError, retErr)
}

func TestPoolFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mTime := time.Now()
	cTime := mTime.AddDate(0, 0, -1)
	dPool := &Pool{
		ObjMeta: ObjMeta{
			MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
			MetaTimeCreated:  cTime,
			MetaTimeModified: mTime,
		},
		CspDomainID: "cspDomainId",
		SystemTags:  []string{"stag1", "stag2"},
	}
	fArg := bson.M{objKey: dPool.ObjMeta.MetaObjID}
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhPool, fArg, newPoolMatcher(t, mockPoolFind, dPool)).Return(nil)
	odhPool.crud = crud
	odhPool.api = api
	odhPool.log = l
	retMA, retErr := odhPool.Fetch(ctx, dPool.ObjMeta.MetaObjID)
	assert.NoError(retErr)
	assert.Equal(dPool.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not found")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhPool, fArg, newPoolMatcher(t, mockPoolFind, dPool)).Return(ds.ErrorNotFound)
	odhPool.crud = crud
	odhPool.api = api
	odhPool.log = l
	retMA, retErr = odhPool.Fetch(ctx, dPool.ObjMeta.MetaObjID)
	assert.Equal(ds.ErrorNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhPool.api = api
	odhPool.log = l
	retMA, retErr = odhPool.Fetch(ctx, dPool.ObjMeta.MetaObjID)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestPoolList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	now := time.Now()
	mTime1 := now.AddDate(-1, 0, 0)
	mTime2 := now.AddDate(0, -1, -2)
	dPools := []interface{}{
		&Pool{
			ObjMeta: ObjMeta{
				MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
				MetaTimeCreated:  now.AddDate(-1, -2, -1),
				MetaTimeModified: mTime1,
			},
			AuthorizedAccountID: "authAccountID",
			ClusterID:           "clusterId",
			CspDomainID:         "cspDomainId",
			StorageAccessibility: StorageAccessibility{
				AccessibilityScope:      "CSPDOMAIN",
				AccessibilityScopeObjID: "cspDomainID",
			},
			SystemTags: []string{"stag1", "stag2"},
		},
		&Pool{
			ObjMeta: ObjMeta{
				MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e1",
				MetaTimeCreated:  now.AddDate(0, -2, -1),
				MetaTimeModified: mTime2,
			},
			AuthorizedAccountID: "authAccountID",
			ClusterID:           "clusterId",
			CspDomainID:         "cspDomainId",
			StorageAccessibility: StorageAccessibility{
				AccessibilityScope:      "CSPDOMAIN",
				AccessibilityScopeObjID: "cspDomainID",
			},
		},
	}
	ctx := context.Background()

	t.Log("case: success")
	params := pool.PoolListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhPool, params, fArg, newConsumeObjFnMatcher(t, dPools...)).Return(nil)
	odhPool.crud = crud
	odhPool.api = api
	odhPool.log = l
	retMA, retErr := odhPool.List(ctx, params)
	assert.NoError(retErr)
	assert.Len(retMA, len(dPools))
	for i, mA := range retMA {
		o := dPools[i].(*Pool)
		assert.Equal(o.ToModel(), mA)
	}
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	params = pool.PoolListParams{
		AuthorizedAccountID:     swag.String("authAccountId"),
		ClusterID:               swag.String("clusterId"),
		CspDomainID:             swag.String("cspDomainId"),
		CspStorageType:          swag.String("EBS-GP2"),
		AccessibilityScope:      swag.String("CSPDOMAIN"),
		AccessibilityScopeObjID: swag.String("cspDomainID"),
		ServicePlanAllocationID: swag.String("servicePlanAllocationID"),
		SystemTags:              []string{"stag1", "stag2"},
	}
	fArg = bson.M{
		"authorizedaccountid": *params.AuthorizedAccountID,
		"clusterid":           *params.ClusterID,
		"cspdomainid":         *params.CspDomainID,
		"cspstoragetype":      *params.CspStorageType,
		"storageaccessibility.accessibilityscope":         *params.AccessibilityScope,
		"storageaccessibility.accessibilityscopeobjid":    *params.AccessibilityScopeObjID,
		"serviceplanreservations.servicePlanAllocationID": bson.M{"$exists": true},
		"systemtags": bson.M{"$all": params.SystemTags},
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhPool, params, fArg, gomock.Any()).Return(errWrappedError)
	odhPool.crud = crud
	odhPool.api = api
	odhPool.log = l
	retMA, retErr = odhPool.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhPool.api = api
	odhPool.log = l
	retMA, retErr = odhPool.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestPoolUpdate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	ua := &ds.UpdateArgs{
		ID:      "58290457-139b-4c55-b6e0-e76b4913ebb6",
		Version: 0,
		Attributes: []ds.UpdateAttr{
			{
				Name: "SystemTags",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateAppend: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	param := &models.PoolMutable{
		PoolCreateMutable: models.PoolCreateMutable{
			SystemTags: []string{"stag1", "stag2"},
		},
	}
	mObj := &models.Pool{PoolMutable: *param}
	dObj := &Pool{}
	dObj.FromModel(mObj)
	now := time.Now()
	dPool := &Pool{
		ObjMeta: ObjMeta{
			MetaObjID:        ua.ID,
			MetaTimeCreated:  now.AddDate(0, -2, -1),
			MetaTimeModified: now,
			MetaVersion:      1000,
		},
	}
	m := poolMatcher(t, mockPoolUpdate).CtxObj(dObj).Return(dPool).Matcher()
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhPool, m, ua).Return(nil)
	odhPool.crud = crud
	odhPool.api = api
	odhPool.log = l
	retMA, retErr := odhPool.Update(ctx, ua, param)
	assert.NoError(retErr)
	assert.Equal(dPool.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhPool, m, ua).Return(ds.ErrorIDVerNotFound)
	odhPool.crud = crud
	odhPool.api = api
	odhPool.log = l
	retMA, retErr = odhPool.Update(ctx, ua, param)
	assert.Equal(ds.ErrorIDVerNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhPool.api = api
	odhPool.log = l
	retMA, retErr = odhPool.Update(ctx, ua, param)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}
