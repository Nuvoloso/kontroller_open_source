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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/storage_request"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// gomock.Matcher for StorageRequest
type mockStorageRequestMatchCtx int

const (
	mockStorageRequestInvalid mockStorageRequestMatchCtx = iota
	mockStorageRequestInsert
	mockStorageRequestFind
	mockStorageRequestUpdate
)

type mockStorageRequestMatcher struct {
	t      *testing.T
	ctx    mockStorageRequestMatchCtx
	ctxObj *StorageRequest
	retObj *StorageRequest
}

func newStorageRequestMatcher(t *testing.T, ctx mockStorageRequestMatchCtx, obj *StorageRequest) gomock.Matcher {
	return storageRequestMatcher(t, ctx).CtxObj(obj).Matcher()
}

// storageRequestMatcher creates a partially initialized mockStorageRequestMatcher
func storageRequestMatcher(t *testing.T, ctx mockStorageRequestMatchCtx) *mockStorageRequestMatcher {
	return &mockStorageRequestMatcher{t: t, ctx: ctx}
}

// Matcher ends the chain
func (o *mockStorageRequestMatcher) Matcher() gomock.Matcher {
	if o.ctx == mockStorageRequestInsert || o.ctx == mockStorageRequestFind || o.ctx == mockStorageRequestUpdate {
		assert.NotNil(o.t, o.ctxObj)
	}
	return o
}

// CtxObj adds an StorageRequest object
func (o *mockStorageRequestMatcher) CtxObj(ao *StorageRequest) *mockStorageRequestMatcher {
	o.ctxObj = ao
	return o
}

// Return sets the update result object
func (o *mockStorageRequestMatcher) Return(ro *StorageRequest) *mockStorageRequestMatcher {
	o.retObj = ro
	return o
}

// Matches is from gomock.Matcher
func (o *mockStorageRequestMatcher) Matches(x interface{}) bool {
	var obj *StorageRequest
	switch z := x.(type) {
	case *StorageRequest:
		obj = z
	default:
		panic("Did not get a recognizable object: " + reflect.TypeOf(x).String())
	}
	assert := assert.New(o.t)
	switch o.ctx {
	case mockStorageRequestInsert:
		compObj := o.ctxObj
		compObj.ObjMeta = obj.ObjMeta // insert sets meta data
		return assert.NotZero(obj.ObjMeta.MetaObjID) &&
			assert.Equal(int32(1), obj.ObjMeta.MetaVersion) &&
			assert.False(obj.ObjMeta.MetaTimeCreated.IsZero()) &&
			assert.False(obj.ObjMeta.MetaTimeModified.IsZero()) &&
			assert.Equal(obj.ObjMeta.MetaTimeCreated, obj.ObjMeta.MetaTimeModified) &&
			assert.Equal(compObj, obj)
	case mockStorageRequestFind:
		*obj = *o.ctxObj
		return true
	case mockStorageRequestUpdate:
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
func (o *mockStorageRequestMatcher) String() string {
	switch o.ctx {
	case mockStorageRequestInsert:
		return "matches on insert"
	}
	return "unknown context"
}

func TestStorageRequestHandler(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	assert.Equal("storagerequest", odhStorageRequest.CName())
	indexes := odhStorageRequest.Indexes()
	expIndexes := []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: nodeIDKey, Value: IndexAscending}}},
		{Keys: bsonx.Doc{{Key: terminatedKey, Value: IndexAscending}}, Options: ActiveRequestsOnlyIndex},
	}
	assert.Equal(expIndexes, indexes)
	assert.Equal(indexes, odhStorageRequest.indexes)
	dIf := odhStorageRequest.NewObject()
	assert.NotNil(dIf)
	_, ok := dIf.(*StorageRequest)
	assert.True(ok)

	t.Log("case: Claim + Initialize success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ctx := context.Background()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	odhStorageRequest.Claim(api, crud)
	assert.Equal(api, odhStorageRequest.api)
	assert.Equal(crud, odhStorageRequest.crud)
	assert.Equal(l, odhStorageRequest.log)
	assert.Equal(odhStorageRequest, odhStorageRequest.Ops())
	crud.EXPECT().CreateIndexes(ctx, odhStorageRequest).Return(nil)
	assert.NoError(odhStorageRequest.Initialize(ctx))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize, error creating indexes")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhStorageRequest).Return(errUnknownError)
	odhStorageRequest.crud = crud
	odhStorageRequest.api = api
	assert.Error(errUnknownError, odhStorageRequest.Initialize(ctx))

	t.Log("case: Start")
	api.EXPECT().DBName().Return("dbName")
	assert.NoError(odhStorageRequest.Start(ctx))
}

func TestStorageRequestCount(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	t.Log("case: success")
	ctx := context.Background()
	params := storage_request.StorageRequestListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhStorageRequest, fArg, uint(0)).Return(2, nil)
	odhStorageRequest.crud = crud
	odhStorageRequest.api = api
	odhStorageRequest.log = l
	retMA, retErr := odhStorageRequest.Count(ctx, params, 0)
	assert.Equal(2, retMA)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: count fails")
	mockCtrl = gomock.NewController(t)
	params = storage_request.StorageRequestListParams{
		ClusterID: swag.String("clusterId"),
	}
	fArg = bson.M{
		"clusterid": *params.ClusterID,
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhStorageRequest, fArg, uint(5)).Return(0, errWrappedError)
	odhStorageRequest.crud = crud
	odhStorageRequest.api = api
	odhStorageRequest.log = l
	retMA, retErr = odhStorageRequest.Count(ctx, params, 5)
	assert.Zero(retMA)
	assert.Equal(errWrappedError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhStorageRequest.api = api
	odhStorageRequest.log = l
	retMA, retErr = odhStorageRequest.Count(ctx, params, 0)
	assert.Equal(errWrappedError, retErr)
	assert.Zero(retMA)
}

func TestStorageRequestCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mSR := &models.StorageRequestCreateArgs{
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			RequestedOperations: []string{"PROVISION"},
			AccountID:           "aid1",
			CspDomainID:         "cspDomainId",
			CspStorageType:      "Amazon gp2",
		},
		StorageRequestCreateMutable: models.StorageRequestCreateMutable{
			StorageID:  "storageID",
			SystemTags: []string{"stag1", "stag2"},
		},
	}
	mC := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{ObjType: "StorageRequest"},
		},
		StorageRequestCreateOnce: mSR.StorageRequestCreateOnce,
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
				RequestMessages:     []*models.TimestampedString{},
				StorageRequestState: ds.DefaultStorageRequestState,
			},
			StorageRequestCreateMutable: mSR.StorageRequestCreateMutable,
		},
	}
	dStorageRequest := &StorageRequest{}
	dStorageRequest.FromModel(mC) // initialize empty properties
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().InsertOne(ctx, odhStorageRequest, newStorageRequestMatcher(t, mockStorageRequestInsert, dStorageRequest)).Return(nil)
	odhStorageRequest.crud = crud
	odhStorageRequest.api = api
	odhStorageRequest.log = l
	retMA, retErr := odhStorageRequest.Create(ctx, mSR)
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
	crud.EXPECT().InsertOne(ctx, odhStorageRequest, newStorageRequestMatcher(t, mockStorageRequestInsert, dStorageRequest)).Return(ds.ErrorExists)
	odhStorageRequest.crud = crud
	odhStorageRequest.api = api
	odhStorageRequest.log = l
	retMA, retErr = odhStorageRequest.Create(ctx, mSR)
	assert.Equal(ds.ErrorExists, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhStorageRequest.api = api
	odhStorageRequest.log = l
	retMA, retErr = odhStorageRequest.Create(ctx, mSR)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestStorageRequestDelete(t *testing.T) {
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
	crud.EXPECT().DeleteOne(ctx, odhStorageRequest, mID).Return(nil)
	odhStorageRequest.crud = crud
	odhStorageRequest.api = api
	odhStorageRequest.log = l
	retErr := odhStorageRequest.Delete(ctx, mID)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().DeleteOne(ctx, odhStorageRequest, mID).Return(errUnknownError)
	odhStorageRequest.crud = crud
	odhStorageRequest.api = api
	odhStorageRequest.log = l
	retErr = odhStorageRequest.Delete(ctx, mID)
	assert.Equal(errUnknownError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhStorageRequest.api = api
	odhStorageRequest.log = l
	retErr = odhStorageRequest.Delete(ctx, mID)
	assert.Equal(errWrappedError, retErr)
}

func TestStorageRequestFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mTime := time.Now()
	cTime := mTime.AddDate(0, 0, -1)
	dStorageRequest := &StorageRequest{
		ObjMeta: ObjMeta{
			MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
			MetaTimeCreated:  cTime,
			MetaTimeModified: mTime,
		},
		RequestedOperations: []string{"PROVISION"},
		CspStorageType:      "Amazon gp2",
		StorageRequestState: ds.DefaultStorageRequestState,
		SystemTags:          []string{"stag1", "stag2"},
	}
	fArg := bson.M{objKey: dStorageRequest.ObjMeta.MetaObjID}
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhStorageRequest, fArg, newStorageRequestMatcher(t, mockStorageRequestFind, dStorageRequest)).Return(nil)
	odhStorageRequest.crud = crud
	odhStorageRequest.api = api
	odhStorageRequest.log = l
	retMA, retErr := odhStorageRequest.Fetch(ctx, dStorageRequest.ObjMeta.MetaObjID)
	assert.NoError(retErr)
	assert.Equal(dStorageRequest.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not found")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhStorageRequest, fArg, newStorageRequestMatcher(t, mockStorageRequestFind, dStorageRequest)).Return(ds.ErrorNotFound)
	odhStorageRequest.crud = crud
	odhStorageRequest.api = api
	odhStorageRequest.log = l
	retMA, retErr = odhStorageRequest.Fetch(ctx, dStorageRequest.ObjMeta.MetaObjID)
	assert.Equal(ds.ErrorNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhStorageRequest.api = api
	odhStorageRequest.log = l
	retMA, retErr = odhStorageRequest.Fetch(ctx, dStorageRequest.ObjMeta.MetaObjID)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestStorageRequestList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	now := time.Now()
	mTime1 := now.AddDate(-1, 0, 0)
	mTime2 := now.AddDate(0, -1, -2)
	dStorageRequests := []interface{}{
		&StorageRequest{
			ObjMeta: ObjMeta{
				MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
				MetaTimeCreated:  now.AddDate(-1, -2, -1),
				MetaTimeModified: mTime1,
			},
			CspDomainID:         "cspDomainId",
			StorageRequestState: ds.DefaultStorageRequestState,
			SystemTags:          []string{"stag1", "stag2"},
		},
		&StorageRequest{
			ObjMeta: ObjMeta{
				MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e1",
				MetaTimeCreated:  now.AddDate(0, -2, -1),
				MetaTimeModified: mTime2,
			},
			CspDomainID:         "cspDomainId",
			StorageRequestState: "SUCCEEDED",
		},
	}
	ctx := context.Background()

	t.Log("case: success")
	params := storage_request.StorageRequestListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhStorageRequest, params, fArg, newConsumeObjFnMatcher(t, dStorageRequests...)).Return(nil)
	odhStorageRequest.crud = crud
	odhStorageRequest.api = api
	odhStorageRequest.log = l
	retMA, retErr := odhStorageRequest.List(ctx, params)
	assert.NoError(retErr)
	assert.Len(retMA, len(dStorageRequests))
	for i, mA := range retMA {
		o := dStorageRequests[i].(*StorageRequest)
		assert.Equal(o.ToModel(), mA)
	}
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	params = storage_request.StorageRequestListParams{
		CspDomainID:           swag.String("cspDomainId"),
		IsTerminated:          swag.Bool(true),
		NodeID:                swag.String("nodeId"),
		PoolID:                swag.String("poolId"),
		SystemTags:            []string{"stag1", "stag2"},
		VolumeSeriesRequestID: swag.String("vsrId"),
	}
	fArg = bson.M{
		"cspdomainid":                            *params.CspDomainID,
		"nodeid":                                 *params.NodeID,
		"poolid":                                 *params.PoolID,
		"systemtags":                             bson.M{"$all": params.SystemTags},
		"terminated":                             true,
		"volumeseriesrequestclaims.claims.vsrId": bson.M{"$exists": true},
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhStorageRequest, params, fArg, gomock.Any()).Return(errWrappedError)
	odhStorageRequest.crud = crud
	odhStorageRequest.api = api
	odhStorageRequest.log = l
	retMA, retErr = odhStorageRequest.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	// case: more query combinations
	for tc := 1; tc <= 6; tc++ {
		switch tc {
		case 1:
			t.Log("case: query with ClusterID, active state")
			params = storage_request.StorageRequestListParams{
				ClusterID:           swag.String("clusterID"),
				StorageRequestState: swag.String(ds.DefaultStorageRequestState),
			}
			fArg = bson.M{
				"clusterid":           *params.ClusterID,
				"storagerequeststate": *params.StorageRequestState,
				"terminated":          false,
			}
		case 2:
			t.Log("case: query with !IsTerminated")
			params = storage_request.StorageRequestListParams{
				IsTerminated: swag.Bool(false),
			}
			fArg = bson.M{
				"terminated": false,
			}
		case 3:
			t.Log("case: query IsTerminated overrides StorageRequestState")
			params = storage_request.StorageRequestListParams{
				IsTerminated:        swag.Bool(true),
				StorageRequestState: swag.String("SUCCEEDED"),
			}
			fArg = bson.M{
				"terminated": true,
			}
		case 4:
			t.Log("case: !IsTerminated overrides StorageRequestState")
			params = storage_request.StorageRequestListParams{
				IsTerminated:        swag.Bool(false),
				StorageRequestState: swag.String("SUCCEEDED"),
			}
			fArg = bson.M{
				"terminated": false,
			}
		case 5:
			t.Log("case: query by storageId, terminal state")
			params = storage_request.StorageRequestListParams{
				StorageID:           swag.String("storageId"),
				StorageRequestState: swag.String("SUCCEEDED"),
			}
			fArg = bson.M{
				"storageid":           *params.StorageID,
				"storagerequeststate": "SUCCEEDED",
			}
		case 6:
			t.Log("case: query activeOrTimeModifiedGE")
			tATmGE := time.Now().Add(-10 * time.Minute)
			aTmGE := strfmt.DateTime(tATmGE)
			params = storage_request.StorageRequestListParams{
				ActiveOrTimeModifiedGE: &aTmGE,
			}
			fArg = bson.M{
				"$or": []bson.M{
					bson.M{"MetaTM": bson.M{"$gte": tATmGE}},
					bson.M{"terminated": false},
				},
			}
		default:
			assert.False(true)
		}

		q := odhStorageRequest.convertListParams(params)
		assert.Equal(fArg, q, "Query[%d]", tc)
	}

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhStorageRequest.api = api
	odhStorageRequest.log = l
	retMA, retErr = odhStorageRequest.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestStorageRequestUpdate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	ua := &ds.UpdateArgs{
		ID:      "58290457-139b-4c55-b6e0-e76b4913ebb6",
		Version: 999,
		Attributes: []ds.UpdateAttr{
			{
				Name: "StorageID",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
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
	param := &models.StorageRequestMutable{
		StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
			StorageRequestState: "SUCCEEDED",
		},
		StorageRequestCreateMutable: models.StorageRequestCreateMutable{
			StorageID:  "storageID",
			SystemTags: []string{"stag1", "stag2"},
		},
	}
	mObj := &models.StorageRequest{StorageRequestMutable: *param}
	dObj := &StorageRequest{}
	dObj.FromModel(mObj)
	now := time.Now()
	dStorageRequest := &StorageRequest{
		ObjMeta: ObjMeta{
			MetaObjID:        ua.ID,
			MetaTimeCreated:  now.AddDate(0, -2, -1),
			MetaTimeModified: now,
			MetaVersion:      1000,
		},
	}
	m := storageRequestMatcher(t, mockStorageRequestUpdate).CtxObj(dObj).Return(dStorageRequest)
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhStorageRequest, m, ua).Return(nil)
	odhStorageRequest.crud = crud
	odhStorageRequest.api = api
	odhStorageRequest.log = l
	retMA, retErr := odhStorageRequest.Update(ctx, ua, param)
	assert.NoError(retErr)
	assert.Equal(dStorageRequest.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure, cover auto-update of Terminated field")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhStorageRequest, m, ua).Return(ds.ErrorIDVerNotFound)
	odhStorageRequest.crud = crud
	odhStorageRequest.api = api
	odhStorageRequest.log = l
	ua.Attributes = append(ua.Attributes, ds.UpdateAttr{
		Name: "StorageRequestState",
		Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
			ds.UpdateSet: ds.UpdateActionArgs{
				FromBody: true,
			},
		},
	})
	retMA, retErr = odhStorageRequest.Update(ctx, ua, param)
	assert.Equal(ds.ErrorIDVerNotFound, retErr)
	assert.Nil(retMA)
	assert.True(ua.IsModified("Terminated"))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhStorageRequest.api = api
	odhStorageRequest.log = l
	retMA, retErr = odhStorageRequest.Update(ctx, ua, param)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}
