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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
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

// gomock.Matcher for VolumeSeriesRequest
type mockVolumeSeriesRequestMatchCtx int

const (
	mockVolumeSeriesRequestInvalid mockVolumeSeriesRequestMatchCtx = iota
	mockVolumeSeriesRequestInsert
	mockVolumeSeriesRequestFind
	mockVolumeSeriesRequestUpdate
)

type mockVolumeSeriesRequestMatcher struct {
	t      *testing.T
	ctx    mockVolumeSeriesRequestMatchCtx
	ctxObj *VolumeSeriesRequest
	retObj *VolumeSeriesRequest
}

func newVolumeSeriesRequestMatcher(t *testing.T, ctx mockVolumeSeriesRequestMatchCtx, obj *VolumeSeriesRequest) gomock.Matcher {
	return volumeSeriesRequestMatcher(t, ctx).CtxObj(obj).Matcher()
}

// volumeSeriesRequestMatcher creates a partially initialized mockVolumeSeriesRequestMatcher
func volumeSeriesRequestMatcher(t *testing.T, ctx mockVolumeSeriesRequestMatchCtx) *mockVolumeSeriesRequestMatcher {
	return &mockVolumeSeriesRequestMatcher{t: t, ctx: ctx}
}

// Matcher ends the chain
func (o *mockVolumeSeriesRequestMatcher) Matcher() gomock.Matcher {
	if o.ctx == mockVolumeSeriesRequestInsert || o.ctx == mockVolumeSeriesRequestFind || o.ctx == mockVolumeSeriesRequestUpdate {
		assert.NotNil(o.t, o.ctxObj)
	}
	return o
}

// CtxObj adds an VolumeSeriesRequest object
func (o *mockVolumeSeriesRequestMatcher) CtxObj(ao *VolumeSeriesRequest) *mockVolumeSeriesRequestMatcher {
	o.ctxObj = ao
	return o
}

// Return sets the update result object
func (o *mockVolumeSeriesRequestMatcher) Return(ro *VolumeSeriesRequest) *mockVolumeSeriesRequestMatcher {
	o.retObj = ro
	return o
}

// Matches is from gomock.Matcher
func (o *mockVolumeSeriesRequestMatcher) Matches(x interface{}) bool {
	var obj *VolumeSeriesRequest
	switch z := x.(type) {
	case *VolumeSeriesRequest:
		obj = z
	default:
		panic("Did not get a recognizable object: " + reflect.TypeOf(x).String())
	}
	assert := assert.New(o.t)
	switch o.ctx {
	case mockVolumeSeriesRequestInsert:
		compObj := o.ctxObj
		compObj.ObjMeta = obj.ObjMeta // insert sets meta data
		return assert.NotZero(obj.ObjMeta.MetaObjID) &&
			assert.Equal(int32(1), obj.ObjMeta.MetaVersion) &&
			assert.False(obj.ObjMeta.MetaTimeCreated.IsZero()) &&
			assert.False(obj.ObjMeta.MetaTimeModified.IsZero()) &&
			assert.Equal(obj.ObjMeta.MetaTimeCreated, obj.ObjMeta.MetaTimeModified) &&
			assert.Equal(compObj, obj)
	case mockVolumeSeriesRequestFind:
		*obj = *o.ctxObj
		return true
	case mockVolumeSeriesRequestUpdate:
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
func (o *mockVolumeSeriesRequestMatcher) String() string {
	switch o.ctx {
	case mockVolumeSeriesRequestInsert:
		return "matches on insert"
	}
	return "unknown context"
}

func TestVolumeSeriesRequestHandler(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	assert.Equal("volumeseriesrequest", odhVolumeSeriesRequest.CName())
	indexes := odhVolumeSeriesRequest.Indexes()
	expIndexes := []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: clusterIDKey, Value: IndexAscending}}},
		{Keys: bsonx.Doc{{Key: nodeIDKey, Value: IndexAscending}}},
		{Keys: bsonx.Doc{{Key: terminatedKey, Value: IndexAscending}}, Options: ActiveRequestsOnlyIndex},
	}
	assert.Equal(expIndexes, indexes)
	assert.Equal(indexes, odhVolumeSeriesRequest.indexes)
	dIf := odhVolumeSeriesRequest.NewObject()
	assert.NotNil(dIf)
	_, ok := dIf.(*VolumeSeriesRequest)
	assert.True(ok)

	t.Log("case: Claim + Initialize success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ctx := context.Background()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	odhVolumeSeriesRequest.Claim(api, crud)
	assert.Equal(api, odhVolumeSeriesRequest.api)
	assert.Equal(crud, odhVolumeSeriesRequest.crud)
	assert.Equal(l, odhVolumeSeriesRequest.log)
	assert.Equal(odhVolumeSeriesRequest, odhVolumeSeriesRequest.Ops())
	crud.EXPECT().CreateIndexes(ctx, odhVolumeSeriesRequest).Return(nil)
	assert.NoError(odhVolumeSeriesRequest.Initialize(ctx))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize, error creating indexes")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhVolumeSeriesRequest).Return(errUnknownError)
	odhVolumeSeriesRequest.crud = crud
	odhVolumeSeriesRequest.api = api
	assert.Error(errUnknownError, odhVolumeSeriesRequest.Initialize(ctx))

	t.Log("case: Start")
	api.EXPECT().DBName().Return("dbName")
	assert.NoError(odhVolumeSeriesRequest.Start(ctx))
}

func TestVolumeSeriesRequestCancel(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	id := "vsr1"
	version := int32(8)
	now := time.Now()
	data := &VolumeSeriesRequest{
		ObjMeta: ObjMeta{
			MetaObjID:        "vsr1",
			MetaTimeCreated:  now.AddDate(0, -2, -1),
			MetaTimeModified: now.AddDate(0, -1, -2),
			MetaVersion:      version + 1,
		},
		CancelRequested: true,
	}
	dObj := &VolumeSeriesRequest{CancelRequested: true}
	ua := &ds.UpdateArgs{
		ID:      id,
		Version: version,
		Attributes: []ds.UpdateAttr{
			{Name: "CancelRequested", Actions: [ds.NumActionTypes]ds.UpdateActionArgs{ds.UpdateSet: ds.UpdateActionArgs{FromBody: true}}},
		},
	}
	m := volumeSeriesRequestMatcher(t, mockVolumeSeriesRequestUpdate).CtxObj(dObj).Return(data).Matcher()
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhVolumeSeriesRequest, m, ua).Return(nil)
	odhVolumeSeriesRequest.crud = crud
	odhVolumeSeriesRequest.api = api
	odhVolumeSeriesRequest.log = l
	retMA, retErr := odhVolumeSeriesRequest.Cancel(ctx, id, version)
	assert.NoError(retErr)
	assert.Equal(data.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhVolumeSeriesRequest, m, ua).Return(ds.ErrorIDVerNotFound)
	odhVolumeSeriesRequest.crud = crud
	odhVolumeSeriesRequest.api = api
	odhVolumeSeriesRequest.log = l
	retMA, retErr = odhVolumeSeriesRequest.Cancel(ctx, id, version)
	assert.Equal(ds.ErrorIDVerNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhVolumeSeriesRequest.api = api
	odhVolumeSeriesRequest.log = l
	retMA, retErr = odhVolumeSeriesRequest.Cancel(ctx, id, version)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestVolumeSeriesRequestCount(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	t.Log("case: success")
	ctx := context.Background()
	params := volume_series_request.VolumeSeriesRequestListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhVolumeSeriesRequest, fArg, uint(0)).Return(2, nil)
	odhVolumeSeriesRequest.crud = crud
	odhVolumeSeriesRequest.api = api
	odhVolumeSeriesRequest.log = l
	retMA, retErr := odhVolumeSeriesRequest.Count(ctx, params, 0)
	assert.Equal(2, retMA)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: count fails")
	mockCtrl = gomock.NewController(t)
	params = volume_series_request.VolumeSeriesRequestListParams{
		ClusterID: swag.String("clusterId"),
	}
	fArg = bson.M{
		"clusterid": *params.ClusterID,
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhVolumeSeriesRequest, fArg, uint(5)).Return(0, errWrappedError)
	odhVolumeSeriesRequest.crud = crud
	odhVolumeSeriesRequest.api = api
	odhVolumeSeriesRequest.log = l
	retMA, retErr = odhVolumeSeriesRequest.Count(ctx, params, 5)
	assert.Zero(retMA)
	assert.Equal(errWrappedError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhVolumeSeriesRequest.api = api
	odhVolumeSeriesRequest.log = l
	retMA, retErr = odhVolumeSeriesRequest.Count(ctx, params, 0)
	assert.Equal(errWrappedError, retErr)
	assert.Zero(retMA)
}

func TestVolumeSeriesRequestCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mVSR := &models.VolumeSeriesRequestCreateArgs{
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			RequestedOperations: []string{"CREATE", "BIND"},
			ClusterID:           "cl1",
		},
		VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
			NodeID:                  "node1",
			VolumeSeriesID:          "vs1",
			ServicePlanAllocationID: "spa1",
		},
	}
	mC := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{ObjType: "VolumeSeriesRequest"},
		},
		VolumeSeriesRequestCreateOnce: mVSR.VolumeSeriesRequestCreateOnce,
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				RequestMessages:          []*models.TimestampedString{},
				VolumeSeriesRequestState: ds.DefaultVolumeSeriesRequestState,
			},
			VolumeSeriesRequestCreateMutable: mVSR.VolumeSeriesRequestCreateMutable,
		},
	}
	dVolumeSeriesRequest := &VolumeSeriesRequest{}
	dVolumeSeriesRequest.FromModel(mC) // initialize empty properties
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().InsertOne(ctx, odhVolumeSeriesRequest, newVolumeSeriesRequestMatcher(t, mockVolumeSeriesRequestInsert, dVolumeSeriesRequest)).Return(nil)
	odhVolumeSeriesRequest.crud = crud
	odhVolumeSeriesRequest.api = api
	odhVolumeSeriesRequest.log = l
	retMA, retErr := odhVolumeSeriesRequest.Create(ctx, mVSR)
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
	crud.EXPECT().InsertOne(ctx, odhVolumeSeriesRequest, newVolumeSeriesRequestMatcher(t, mockVolumeSeriesRequestInsert, dVolumeSeriesRequest)).Return(ds.ErrorExists)
	odhVolumeSeriesRequest.crud = crud
	odhVolumeSeriesRequest.api = api
	odhVolumeSeriesRequest.log = l
	retMA, retErr = odhVolumeSeriesRequest.Create(ctx, mVSR)
	assert.Equal(ds.ErrorExists, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhVolumeSeriesRequest.api = api
	odhVolumeSeriesRequest.log = l
	retMA, retErr = odhVolumeSeriesRequest.Create(ctx, mVSR)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestVolumeSeriesRequestDelete(t *testing.T) {
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
	crud.EXPECT().DeleteOne(ctx, odhVolumeSeriesRequest, mID).Return(nil)
	odhVolumeSeriesRequest.crud = crud
	odhVolumeSeriesRequest.api = api
	odhVolumeSeriesRequest.log = l
	retErr := odhVolumeSeriesRequest.Delete(ctx, mID)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().DeleteOne(ctx, odhVolumeSeriesRequest, mID).Return(errUnknownError)
	odhVolumeSeriesRequest.crud = crud
	odhVolumeSeriesRequest.api = api
	odhVolumeSeriesRequest.log = l
	retErr = odhVolumeSeriesRequest.Delete(ctx, mID)
	assert.Equal(errUnknownError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhVolumeSeriesRequest.api = api
	odhVolumeSeriesRequest.log = l
	retErr = odhVolumeSeriesRequest.Delete(ctx, mID)
	assert.Equal(errWrappedError, retErr)
}

func TestVolumeSeriesRequestFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mTime := time.Now()
	cTime := mTime.AddDate(0, 0, -1)
	dVolumeSeriesRequest := &VolumeSeriesRequest{
		ObjMeta: ObjMeta{
			MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
			MetaTimeCreated:  cTime,
			MetaTimeModified: mTime,
		},
		RequestedOperations: []string{"CREATE"},
		VolumeSeriesCreateSpec: VolumeSeriesCreateSpec{
			VolumeSeriesCreateMutableFields: VolumeSeriesCreateMutableFields{
				Name: "vol1",
			},
		},
		VolumeSeriesRequestState: ds.DefaultVolumeSeriesRequestState,
	}
	fArg := bson.M{objKey: dVolumeSeriesRequest.ObjMeta.MetaObjID}
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhVolumeSeriesRequest, fArg, newVolumeSeriesRequestMatcher(t, mockVolumeSeriesRequestFind, dVolumeSeriesRequest)).Return(nil)
	odhVolumeSeriesRequest.crud = crud
	odhVolumeSeriesRequest.api = api
	odhVolumeSeriesRequest.log = l
	retMA, retErr := odhVolumeSeriesRequest.Fetch(ctx, dVolumeSeriesRequest.ObjMeta.MetaObjID)
	assert.NoError(retErr)
	assert.Equal(dVolumeSeriesRequest.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not found")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhVolumeSeriesRequest, fArg, newVolumeSeriesRequestMatcher(t, mockVolumeSeriesRequestFind, dVolumeSeriesRequest)).Return(ds.ErrorNotFound)
	odhVolumeSeriesRequest.crud = crud
	odhVolumeSeriesRequest.api = api
	odhVolumeSeriesRequest.log = l
	retMA, retErr = odhVolumeSeriesRequest.Fetch(ctx, dVolumeSeriesRequest.ObjMeta.MetaObjID)
	assert.Equal(ds.ErrorNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhVolumeSeriesRequest.api = api
	odhVolumeSeriesRequest.log = l
	retMA, retErr = odhVolumeSeriesRequest.Fetch(ctx, dVolumeSeriesRequest.ObjMeta.MetaObjID)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestVolumeSeriesRequestList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	now := time.Now()
	mTime1 := now.AddDate(-1, 0, 0)
	mTime2 := now.AddDate(0, -1, -2)
	dVolumeSeriesRequests := []interface{}{
		&VolumeSeriesRequest{
			ObjMeta: ObjMeta{
				MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
				MetaTimeCreated:  now.AddDate(-1, -2, -1),
				MetaTimeModified: mTime1,
			},
			ClusterID:                "cl1",
			NodeID:                   "node1",
			ServicePlanAllocationID:  "spa1",
			VolumeSeriesRequestState: ds.DefaultVolumeSeriesRequestState,
			SystemTags:               []string{"stag1", "stag2"},
		},
		&VolumeSeriesRequest{
			ObjMeta: ObjMeta{
				MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e1",
				MetaTimeCreated:  now.AddDate(0, -2, -1),
				MetaTimeModified: mTime2,
			},
			ClusterID:                "cl1",
			NodeID:                   "node2",
			ServicePlanAllocationID:  "spa1",
			VolumeSeriesRequestState: ds.DefaultVolumeSeriesRequestState,
		},
	}
	ctx := context.Background()

	t.Log("case: success")
	params := volume_series_request.VolumeSeriesRequestListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhVolumeSeriesRequest, params, fArg, newConsumeObjFnMatcher(t, dVolumeSeriesRequests...)).Return(nil)
	odhVolumeSeriesRequest.crud = crud
	odhVolumeSeriesRequest.api = api
	odhVolumeSeriesRequest.log = l
	retMA, retErr := odhVolumeSeriesRequest.List(ctx, params)
	assert.NoError(retErr)
	assert.Len(retMA, len(dVolumeSeriesRequests))
	for i, mA := range retMA {
		o := dVolumeSeriesRequests[i].(*VolumeSeriesRequest)
		assert.Equal(o.ToModel(), mA)
	}
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	params = volume_series_request.VolumeSeriesRequestListParams{
		ClusterID:          swag.String("cluster"),
		ConsistencyGroupID: swag.String("cgId"),
		IsTerminated:       swag.Bool(true),
		NodeID:             swag.String("nodeId"),
		SnapshotID:         swag.String("snapshotId"),
		SystemTags:         []string{"stag1", "stag2"},
		SyncCoordinatorID:  swag.String("syncId"),
	}
	fArg = bson.M{
		"clusterid":          *params.ClusterID,
		"consistencygroupid": *params.ConsistencyGroupID,
		"nodeid":             *params.NodeID,
		"terminated":         true,
		"snapshotid":         *params.SnapshotID,
		"systemtags":         bson.M{"$all": params.SystemTags},
		"synccoordinatorid":  *params.SyncCoordinatorID,
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhVolumeSeriesRequest, params, fArg, gomock.Any()).Return(errWrappedError)
	odhVolumeSeriesRequest.crud = crud
	odhVolumeSeriesRequest.api = api
	odhVolumeSeriesRequest.log = l
	retMA, retErr = odhVolumeSeriesRequest.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	// case: more query combinations
	for tc := 1; tc <= 15; tc++ {
		switch tc {
		case 1:
			t.Log("case: query with ClusterID and active VSR states")
			params = volume_series_request.VolumeSeriesRequestListParams{
				ClusterID:                swag.String("clusterID"),
				VolumeSeriesRequestState: []string{ds.DefaultVolumeSeriesRequestState, "CREATING"},
			}
			fArg = bson.M{
				"clusterid":                *params.ClusterID,
				"terminated":               false,
				"volumeseriesrequeststate": bson.M{"$in": params.VolumeSeriesRequestState},
			}
		case 2:
			t.Log("case: query with !IsTerminated")
			params = volume_series_request.VolumeSeriesRequestListParams{
				IsTerminated: swag.Bool(false),
			}
			fArg = bson.M{
				"terminated": false,
			}
		case 3:
			t.Log("case: query IsTerminated overrides VolumeSeriesRequestState")
			params = volume_series_request.VolumeSeriesRequestListParams{
				IsTerminated:             swag.Bool(true),
				VolumeSeriesRequestState: []string{"SUCCEEDED"},
			}
			fArg = bson.M{
				"terminated": true,
			}
		case 4:
			t.Log("case: !IsTerminated overrides VolumeSeriesRequestState")
			params = volume_series_request.VolumeSeriesRequestListParams{
				IsTerminated:             swag.Bool(false),
				VolumeSeriesRequestState: []string{"SUCCEEDED"},
			}
			fArg = bson.M{
				"terminated": false,
			}
		case 5:
			t.Log("case: query by volumeSeriesId")
			params = volume_series_request.VolumeSeriesRequestListParams{
				VolumeSeriesID: swag.String("vs1"),
			}
			fArg = bson.M{
				"volumeseriesid": *params.VolumeSeriesID,
			}
		case 6:
			t.Log("case: query by account and service plan")
			params = volume_series_request.VolumeSeriesRequestListParams{
				AccountID:     swag.String("a1"),
				ServicePlanID: swag.String("sp1"),
			}
			fArg = bson.M{
				"volumeseriescreatespec.accountid": *params.AccountID,
				"serviceplanid":                    *params.ServicePlanID,
			}
		case 7:
			t.Log("case: query by storageId and tenantAccountId")
			params = volume_series_request.VolumeSeriesRequestListParams{
				StorageID:       swag.String("st1"),
				TenantAccountID: swag.String("t1"),
			}
			fArg = bson.M{
				"storageplan.storageelements.storageparcels.st1": bson.M{"$exists": true},
				"volumeseriescreatespec.tenantaccountid":         *params.TenantAccountID,
			}
		case 8:
			t.Log("case: query by poolId")
			params = volume_series_request.VolumeSeriesRequestListParams{
				PoolID: swag.String("prov1"),
			}
			fArg = bson.M{
				"$or": []bson.M{
					bson.M{"capacityreservationresult.currentreservations.prov1": bson.M{"$exists": true}},
					bson.M{"capacityreservationresult.desiredreservations.prov1": bson.M{"$exists": true}},
					bson.M{"storageplan.storageelements.poolid": "prov1"},
				},
			}
		case 9:
			t.Log("case: query activeOrTimeModifiedGE")
			tATmGE := time.Now().Add(-10 * time.Minute)
			aTmGE := strfmt.DateTime(tATmGE)
			params = volume_series_request.VolumeSeriesRequestListParams{
				ActiveOrTimeModifiedGE: &aTmGE,
			}
			fArg = bson.M{
				"$or": []bson.M{
					bson.M{"MetaTM": bson.M{"$gte": tATmGE}},
					bson.M{"terminated": false},
				},
			}
		case 10:
			t.Log("case: query by accounts, poolId and activeOrTimeModifiedGE")
			tATmGE := time.Now().Add(-10 * time.Minute)
			aTmGE := strfmt.DateTime(tATmGE)
			params = volume_series_request.VolumeSeriesRequestListParams{
				AccountID:              swag.String("a1"),
				ActiveOrTimeModifiedGE: &aTmGE,
				PoolID:                 swag.String("prov1"),
				TenantAccountID:        swag.String("t1"),
			}
			fArg = bson.M{
				"$and": []bson.M{
					bson.M{
						"$or": []bson.M{
							bson.M{"volumeseriescreatespec.accountid": *params.AccountID},
							bson.M{"volumeseriescreatespec.tenantaccountid": *params.TenantAccountID},
						},
					},
					bson.M{
						"$or": []bson.M{
							bson.M{"capacityreservationresult.currentreservations.prov1": bson.M{"$exists": true}},
							bson.M{"capacityreservationresult.desiredreservations.prov1": bson.M{"$exists": true}},
							bson.M{"storageplan.storageelements.poolid": "prov1"},
						},
					},
					bson.M{
						"$or": []bson.M{
							bson.M{"MetaTM": bson.M{"$gte": tATmGE}},
							bson.M{"terminated": false},
						},
					},
				},
			}
		case 11:
			t.Log("case: query servicePlanAllocationID, mix of active and terminal states")
			params = volume_series_request.VolumeSeriesRequestListParams{
				ServicePlanAllocationID:  swag.String("spaID"),
				VolumeSeriesRequestState: []string{"NEW", "SUCCEEDED"},
			}
			fArg = bson.M{
				"serviceplanallocationid":  *params.ServicePlanAllocationID,
				"volumeseriesrequeststate": bson.M{"$in": params.VolumeSeriesRequestState},
			}
		case 12:
			t.Log("case: query with ProtectionDomainID")
			params = volume_series_request.VolumeSeriesRequestListParams{
				ProtectionDomainID: swag.String("protectionDomainID"),
			}
			fArg = bson.M{
				"protectiondomainid": *params.ProtectionDomainID,
			}
		case 13:
			t.Log("case: query with requestedOperations")
			params = volume_series_request.VolumeSeriesRequestListParams{
				RequestedOperations: []string{"OP1", "OP2"},
			}
			fArg = bson.M{
				"requestedoperations": bson.M{"$in": params.RequestedOperations},
			}
		case 14:
			t.Log("case: query with requestedOperationsNot")
			params = volume_series_request.VolumeSeriesRequestListParams{
				RequestedOperationsNot: []string{"NOP1", "NOP2"},
			}
			fArg = bson.M{
				"requestedoperations": bson.M{"$nin": params.RequestedOperationsNot},
			}
		case 15:
			t.Log("case: query with requestedOperations and requestedOperationsNot")
			params = volume_series_request.VolumeSeriesRequestListParams{
				RequestedOperations:    []string{"OP1", "OP2"},
				RequestedOperationsNot: []string{"NOP1", "NOP2"},
			}
			fArg = bson.M{
				"requestedoperations": bson.M{"$nin": params.RequestedOperationsNot},
			}
		default:
			assert.False(true, "case %d", tc)
			continue
		}

		q := odhVolumeSeriesRequest.convertListParams(params)
		assert.Equal(fArg, q, "Query[%d]", tc)
	}

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhVolumeSeriesRequest.api = api
	odhVolumeSeriesRequest.log = l
	retMA, retErr = odhVolumeSeriesRequest.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestVolumeSeriesRequestUpdate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	ua := &ds.UpdateArgs{
		ID:      "58290457-139b-4c55-b6e0-e76b4913ebb6",
		Version: 999,
		Attributes: []ds.UpdateAttr{
			{
				Name: "VolumeSeriesID",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			}, {
				Name: "SystemTags",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateAppend: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}

	param := &models.VolumeSeriesRequestMutable{
		VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
			VolumeSeriesRequestState: "SUCCEEDED",
		},
		VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
			VolumeSeriesID: "vs1",
			SystemTags:     []string{"stag1", "stag2"},
		},
	}
	mObj := &models.VolumeSeriesRequest{VolumeSeriesRequestMutable: *param}
	dObj := &VolumeSeriesRequest{}
	dObj.FromModel(mObj)
	now := time.Now()
	dVolumeSeriesRequest := &VolumeSeriesRequest{
		ObjMeta: ObjMeta{
			MetaObjID:        ua.ID,
			MetaTimeCreated:  now.AddDate(0, -2, -1),
			MetaTimeModified: now,
			MetaVersion:      1000,
		},
	}
	m := volumeSeriesRequestMatcher(t, mockVolumeSeriesRequestUpdate).CtxObj(dObj).Return(dVolumeSeriesRequest)
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhVolumeSeriesRequest, m, ua).Return(nil)
	odhVolumeSeriesRequest.crud = crud
	odhVolumeSeriesRequest.api = api
	odhVolumeSeriesRequest.log = l
	retMA, retErr := odhVolumeSeriesRequest.Update(ctx, ua, param)
	assert.NoError(retErr)
	assert.Equal(dVolumeSeriesRequest.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure, cover auto-update of Terminated field")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhVolumeSeriesRequest, m, ua).Return(ds.ErrorIDVerNotFound)
	odhVolumeSeriesRequest.crud = crud
	odhVolumeSeriesRequest.api = api
	odhVolumeSeriesRequest.log = l
	ua.Attributes = append(ua.Attributes, ds.UpdateAttr{
		Name: "VolumeSeriesRequestState",
		Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
			ds.UpdateSet: ds.UpdateActionArgs{
				FromBody: true,
			},
		},
	})
	retMA, retErr = odhVolumeSeriesRequest.Update(ctx, ua, param)
	assert.Equal(ds.ErrorIDVerNotFound, retErr)
	assert.Nil(retMA)
	assert.True(ua.IsModified("Terminated"))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhVolumeSeriesRequest.api = api
	odhVolumeSeriesRequest.log = l
	retMA, retErr = odhVolumeSeriesRequest.Update(ctx, ua, param)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}
