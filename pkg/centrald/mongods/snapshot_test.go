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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/snapshot"
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

// gomock.Matcher for Snapshot
type mockSnapshotMatchCtx int

const (
	mockSnapshotInvalid mockSnapshotMatchCtx = iota
	mockSnapshotInsert
	mockSnapshotFind
	mockSnapshotUpdate
)

type mockSnapshotMatcher struct {
	t      *testing.T
	ctx    mockSnapshotMatchCtx
	ctxObj *Snapshot
	retObj *Snapshot
}

func newSnapshotMatcher(t *testing.T, ctx mockSnapshotMatchCtx, obj *Snapshot) gomock.Matcher {
	return snapshotMatcher(t, ctx).CtxObj(obj).Matcher()
}

// SnapshotMatcher creates a partially initialized mockSnapshotMatcher
func snapshotMatcher(t *testing.T, ctx mockSnapshotMatchCtx) *mockSnapshotMatcher {
	return &mockSnapshotMatcher{t: t, ctx: ctx}
}

// Matcher ends the chain
func (o *mockSnapshotMatcher) Matcher() gomock.Matcher {
	if o.ctx == mockSnapshotInsert || o.ctx == mockSnapshotFind || o.ctx == mockSnapshotUpdate {
		assert.NotNil(o.t, o.ctxObj)
	}
	return o
}

// CtxObj adds a Snapshot object
func (o *mockSnapshotMatcher) CtxObj(ao *Snapshot) *mockSnapshotMatcher {
	o.ctxObj = ao
	return o
}

// Return sets the update result object
func (o *mockSnapshotMatcher) Return(ro *Snapshot) *mockSnapshotMatcher {
	o.retObj = ro
	return o
}

// Matches is from gomock.Matcher
func (o *mockSnapshotMatcher) Matches(x interface{}) bool {
	var obj *Snapshot
	switch z := x.(type) {
	case *Snapshot:
		obj = z
	default:
		panic("Did not get a recognizable object: " + reflect.TypeOf(x).String())
	}
	assert := assert.New(o.t)
	switch o.ctx {
	case mockSnapshotInsert:
		compObj := o.ctxObj
		compObj.ObjMeta = obj.ObjMeta // insert sets meta data
		return assert.NotZero(obj.ObjMeta.MetaObjID) &&
			assert.Equal(int32(1), obj.ObjMeta.MetaVersion) &&
			assert.False(obj.ObjMeta.MetaTimeCreated.IsZero()) &&
			assert.False(obj.ObjMeta.MetaTimeModified.IsZero()) &&
			assert.Equal(obj.ObjMeta.MetaTimeCreated, obj.ObjMeta.MetaTimeModified) &&
			assert.Equal(compObj, obj)
	case mockSnapshotFind:
		*obj = *o.ctxObj
		return true
	case mockSnapshotUpdate:
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
func (o *mockSnapshotMatcher) String() string {
	switch o.ctx {
	case mockSnapshotInsert:
		return "matches on insert"
	}
	return "unknown context"
}

func TestSnapshotHandler(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	assert.Equal("snapshot", odhSnapshot.CName())
	indexes := odhSnapshot.Indexes()
	expIndexes := []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: accountIDKey, Value: IndexAscending}}},
		{Keys: bsonx.Doc{{Key: tenantAccountIDKey, Value: IndexAscending}}},
		{Keys: bsonx.Doc{{Key: volumeSeriesIDKey, Value: IndexAscending}, {Key: snapIdentifierKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)}, // pair of VolumeSeriesID, SnapIdentifier is unique, order important for query performance
		{Keys: bsonx.Doc{{Key: consistencyGroupIDKey, Value: IndexAscending}}},
	}
	assert.Equal(expIndexes, indexes)
	assert.Equal(indexes, odhSnapshot.indexes)
	dIf := odhSnapshot.NewObject()
	assert.NotNil(dIf)
	_, ok := dIf.(*Snapshot)
	assert.True(ok)

	t.Log("case: Claim + Initialize success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ctx := context.Background()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	odhSnapshot.Claim(api, crud)
	assert.Equal(api, odhSnapshot.api)
	assert.Equal(crud, odhSnapshot.crud)
	assert.Equal(l, odhSnapshot.log)
	assert.Equal(odhSnapshot, odhSnapshot.Ops())
	crud.EXPECT().CreateIndexes(ctx, odhSnapshot).Return(nil)
	assert.NoError(odhSnapshot.Initialize(ctx))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize, error creating indexes")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhSnapshot).Return(errUnknownError)
	odhSnapshot.crud = crud
	odhSnapshot.api = api
	assert.Error(errUnknownError, odhSnapshot.Initialize(ctx))

	t.Log("case: Start")
	api.EXPECT().DBName().Return("dbName")
	assert.NoError(odhSnapshot.Start(ctx))
}

func TestSnapshotCount(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	t.Log("case: success")
	ctx := context.Background()
	params := snapshot.SnapshotListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhSnapshot, fArg, uint(0)).Return(2, nil)
	odhSnapshot.crud = crud
	odhSnapshot.api = api
	odhSnapshot.log = l
	retMA, retErr := odhSnapshot.Count(ctx, params, 0)
	assert.Equal(2, retMA)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: count fails")
	mockCtrl = gomock.NewController(t)
	params = snapshot.SnapshotListParams{
		AccountID: swag.String("accountId"),
	}
	fArg = bson.M{
		"accountid": *params.AccountID,
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhSnapshot, fArg, uint(5)).Return(0, errWrappedError)
	odhSnapshot.crud = crud
	odhSnapshot.api = api
	odhSnapshot.log = l
	retMA, retErr = odhSnapshot.Count(ctx, params, 5)
	assert.Zero(retMA)
	assert.Equal(errWrappedError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhSnapshot.api = api
	odhSnapshot.log = l
	retMA, retErr = odhSnapshot.Count(ctx, params, 0)
	assert.Equal(errWrappedError, retErr)
	assert.Zero(retMA)
}

func TestSnapshotCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	now := time.Now()
	dSnapshot := &Snapshot{
		AccountID:          "myAccountID",
		ConsistencyGroupID: "myConsistencyGroupID",
		PitIdentifier:      "8e0e60e2-063f-4a29-ba79-e380ab08e75d",
		VolumeSeriesID:     "myVolumeSeriesID",
		SnapIdentifier:     "mySnapID",
		Locations: SnapshotLocationMap{
			"csp-1": SnapshotLocation{CreationTime: now, CspDomainID: "csp-1"},
			"csp-2": SnapshotLocation{CreationTime: now.Add(10 * time.Minute), CspDomainID: "csp-2"},
		},
		Messages: []TimestampedString{},
		Tags:     []string{"tag1", "tag2"},
	}
	mSnapshot := dSnapshot.ToModel()
	mSnapshot.Meta = &models.ObjMeta{}
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().InsertOne(ctx, odhSnapshot, newSnapshotMatcher(t, mockSnapshotInsert, dSnapshot)).Return(nil)
	odhSnapshot.crud = crud
	odhSnapshot.api = api
	odhSnapshot.log = l
	retMA, retErr := odhSnapshot.Create(ctx, mSnapshot)
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
	crud.EXPECT().InsertOne(ctx, odhSnapshot, newSnapshotMatcher(t, mockSnapshotInsert, dSnapshot)).Return(ds.ErrorExists)
	odhSnapshot.crud = crud
	odhSnapshot.api = api
	odhSnapshot.log = l
	retMA, retErr = odhSnapshot.Create(ctx, mSnapshot)
	assert.Equal(ds.ErrorExists, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhSnapshot.api = api
	odhSnapshot.log = l
	retMA, retErr = odhSnapshot.Create(ctx, mSnapshot)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestSnapshotDelete(t *testing.T) {
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
	crud.EXPECT().DeleteOne(ctx, odhSnapshot, mID).Return(nil)
	odhSnapshot.crud = crud
	odhSnapshot.api = api
	odhSnapshot.log = l
	retErr := odhSnapshot.Delete(ctx, mID)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().DeleteOne(ctx, odhSnapshot, mID).Return(errUnknownError)
	odhSnapshot.crud = crud
	odhSnapshot.api = api
	odhSnapshot.log = l
	retErr = odhSnapshot.Delete(ctx, mID)
	assert.Equal(errUnknownError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhSnapshot.api = api
	odhSnapshot.log = l
	retErr = odhSnapshot.Delete(ctx, mID)
	assert.Equal(errWrappedError, retErr)
}

func TestSnapshotFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mTime := time.Now()
	cTime := mTime.AddDate(0, 0, -1)
	dSnapshot := &Snapshot{
		ObjMeta: ObjMeta{
			MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
			MetaTimeCreated:  cTime,
			MetaTimeModified: mTime,
		},
		AccountID:  "myAccountID",
		SystemTags: []string{"stag1", "stag2"},
	}
	fArg := bson.M{objKey: dSnapshot.ObjMeta.MetaObjID}
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhSnapshot, fArg, newSnapshotMatcher(t, mockSnapshotFind, dSnapshot)).Return(nil)
	odhSnapshot.crud = crud
	odhSnapshot.api = api
	odhSnapshot.log = l
	retMA, retErr := odhSnapshot.Fetch(ctx, dSnapshot.ObjMeta.MetaObjID)
	assert.NoError(retErr)
	assert.Equal(dSnapshot.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not found")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhSnapshot, fArg, newSnapshotMatcher(t, mockSnapshotFind, dSnapshot)).Return(ds.ErrorNotFound)
	odhSnapshot.crud = crud
	odhSnapshot.api = api
	odhSnapshot.log = l
	retMA, retErr = odhSnapshot.Fetch(ctx, dSnapshot.ObjMeta.MetaObjID)
	assert.Equal(ds.ErrorNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhSnapshot.api = api
	odhSnapshot.log = l
	retMA, retErr = odhSnapshot.Fetch(ctx, dSnapshot.ObjMeta.MetaObjID)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestSnapshotList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	now := time.Now()
	nowDT := strfmt.DateTime(now)
	mTime1 := now.AddDate(-1, 0, 0)
	mTime2 := now.AddDate(0, -1, -2)
	dSnapshots := []interface{}{
		&Snapshot{
			ObjMeta: ObjMeta{
				MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
				MetaTimeCreated:  now.AddDate(-1, -2, -1),
				MetaTimeModified: mTime1,
			},
			AccountID:          "myAccountID",
			ConsistencyGroupID: "myConsistencyGroupID",
			PitIdentifier:      "8e0e60e2-063f-4a29-ba79-e380ab08e75a",
			Locations: SnapshotLocationMap{
				"csp-1": SnapshotLocation{CreationTime: now, CspDomainID: "csp-1"},
				"csp-2": SnapshotLocation{CreationTime: now.Add(10 * time.Minute), CspDomainID: "csp-2"},
			},
			SystemTags: []string{"stag1", "stag2"},
		},
		&Snapshot{
			ObjMeta: ObjMeta{
				MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e1",
				MetaTimeCreated:  now.AddDate(0, -2, -1),
				MetaTimeModified: mTime2,
			},
			AccountID:          "myAccountID",
			ConsistencyGroupID: "myConsistencyGroupID",
			PitIdentifier:      "8e0e60e2-063f-4a29-ba79-e380ab08e75b",
			Locations: SnapshotLocationMap{
				"csp-1": SnapshotLocation{CreationTime: now, CspDomainID: "csp-1"},
				"csp-2": SnapshotLocation{CreationTime: now.Add(10 * time.Minute), CspDomainID: "csp-2"},
			},
			Tags: []string{"tag1", "tag2"},
		},
	}
	ctx := context.Background()

	t.Log("case: success")
	params := snapshot.SnapshotListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhSnapshot, params, fArg, newConsumeObjFnMatcher(t, dSnapshots...)).Return(nil)
	odhSnapshot.crud = crud
	odhSnapshot.api = api
	odhSnapshot.log = l
	retMA, retErr := odhSnapshot.List(ctx, params)
	assert.NoError(retErr)
	assert.Len(retMA, len(dSnapshots))
	for i, mA := range retMA {
		o := dSnapshots[i].(*Snapshot)
		assert.Equal(o.ToModel(), mA)
	}
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	params = snapshot.SnapshotListParams{
		AccountID:          swag.String("someAccountID"),
		ConsistencyGroupID: swag.String("someConsistencyGroupID"),
		SystemTags:         []string{"stag1", "stag2"},
	}
	fArg = bson.M{
		"accountid":          *params.AccountID,
		"consistencygroupid": *params.ConsistencyGroupID,
		"systemtags":         bson.M{"$all": params.SystemTags},
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhSnapshot, params, fArg, gomock.Any()).Return(errWrappedError)
	odhSnapshot.crud = crud
	odhSnapshot.api = api
	odhSnapshot.log = l
	retMA, retErr = odhSnapshot.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	// case: more query combinations
	for tc := 1; tc <= 12; tc++ {
		switch tc {
		case 1:
			t.Log("case: query with AccountID, TenantAccountID and ConsistencyGroupID")
			params = snapshot.SnapshotListParams{
				AccountID:          swag.String("accountID"),
				ConsistencyGroupID: swag.String("myConsistencyGroupID"),
				TenantAccountID:    swag.String("myTenantAccountID"),
			}
			fArg = bson.M{
				"consistencygroupid": *params.ConsistencyGroupID,
				"$or": []bson.M{
					bson.M{"accountid": *params.AccountID},
					bson.M{"tenantaccountid": *params.TenantAccountID},
				},
			}
		case 2:
			t.Log("case: query with CspDomainID")
			params = snapshot.SnapshotListParams{
				CspDomainID: swag.String("csp1"),
			}
			fArg = bson.M{
				"locations.csp1": bson.M{"$exists": true},
			}
		case 3:
			t.Log("case: query with SnapIdentifier")
			params = snapshot.SnapshotListParams{
				SnapIdentifier: swag.String("mySnapID"),
			}
			fArg = bson.M{"snapidentifier": *params.SnapIdentifier}
		case 4:
			t.Log("case: query with Tags/SystemTags")
			params = snapshot.SnapshotListParams{
				SystemTags: []string{"stag1", "stag2"},
				Tags:       []string{"tag1", "tag2"},
			}
			fArg = bson.M{
				"systemtags": bson.M{"$all": []string{"stag1", "stag2"}},
				"tags":       bson.M{"$all": []string{"tag1", "tag2"}},
			}
		case 5:
			t.Log("case: query by TenantAccountID")
			params = snapshot.SnapshotListParams{
				TenantAccountID: swag.String("myTenantAccountID"),
			}
			fArg = bson.M{"tenantaccountid": *params.TenantAccountID}
		case 6:
			t.Log("case: query by VolumeSeriesID")
			params = snapshot.SnapshotListParams{
				VolumeSeriesID: swag.String("myVolumeSeriesID"),
			}
			fArg = bson.M{"volumeseriesid": *params.VolumeSeriesID}
		case 7:
			t.Log("case: query by SnapTimeGE")
			beforeTime := time.Now().Add(-2 * time.Hour)
			beforeDT := strfmt.DateTime(beforeTime)
			params = snapshot.SnapshotListParams{
				SnapTimeGE: &beforeDT,
			}
			fArg = bson.M{"snaptime": bson.M{"$gte": beforeTime}}
		case 8:
			t.Log("case: query by SnapTimeLE")
			params = snapshot.SnapshotListParams{
				SnapTimeLE: &nowDT,
			}
			fArg = bson.M{"snaptime": bson.M{"$lte": now}}
		case 9:
			t.Log("case: query by DeleteAfterTimeLE")
			params = snapshot.SnapshotListParams{
				DeleteAfterTimeLE: &nowDT,
			}
			fArg = bson.M{"deleteaftertime": bson.M{"$lte": now}}
		case 10:
			t.Log("case: query by both SnapTimeLE and SnapTimeGE")
			beforeTime := time.Now().Add(-2 * time.Hour)
			beforeDT := strfmt.DateTime(beforeTime)
			params = snapshot.SnapshotListParams{
				SnapTimeLE: &nowDT,
				SnapTimeGE: &beforeDT,
			}
			fArg = bson.M{
				"$and": []bson.M{
					bson.M{"snaptime": bson.M{"$gte": beforeTime}},
					bson.M{"snaptime": bson.M{"$lte": now}},
				},
			}
		case 11:
			t.Log("case: query by pit identifier")
			params = snapshot.SnapshotListParams{
				PitIdentifiers: []string{"p1", "p2", "p3"},
			}
			fArg = bson.M{"pitidentifier": bson.M{"$in": []string{"p1", "p2", "p3"}}}
		case 12:
			t.Log("case: query with AccountID and ProtectionDomainID")
			params = snapshot.SnapshotListParams{
				AccountID:          swag.String("accountID"),
				ProtectionDomainID: swag.String("protectionDomainID"),
			}
			fArg = bson.M{
				"protectiondomainid": *params.ProtectionDomainID,
				"accountid":          *params.AccountID,
			}
		default:
			assert.False(true)
		}

		q := odhSnapshot.convertListParams(params)
		assert.Equal(fArg, q, "Query[%d]", tc)
	}

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhSnapshot.api = api
	odhSnapshot.log = l
	retMA, retErr = odhSnapshot.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestSnapshotUpdate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	ua := &ds.UpdateArgs{
		ID:      "58290457-139b-4c55-b6e0-e76b4913ebb6",
		Version: 999,
		Attributes: []ds.UpdateAttr{
			{
				Name: "DeleteAfterTime",
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
	now := time.Now()
	param := &models.SnapshotMutable{
		DeleteAfterTime: strfmt.DateTime(now.Add(90 * 24 * time.Hour)),
		SystemTags:      []string{"stag1", "stag2"},
	}
	mObj := &models.Snapshot{SnapshotMutable: *param}
	dObj := &Snapshot{}
	dObj.FromModel(mObj)
	dSnapshot := &Snapshot{
		ObjMeta: ObjMeta{
			MetaObjID:        ua.ID,
			MetaTimeCreated:  now.AddDate(0, -2, -1),
			MetaTimeModified: now,
			MetaVersion:      1000,
		},
	}
	m := snapshotMatcher(t, mockSnapshotUpdate).CtxObj(dObj).Return(dSnapshot).Matcher()
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhSnapshot, m, ua).Return(nil)
	odhSnapshot.crud = crud
	odhSnapshot.api = api
	odhSnapshot.log = l
	retMA, retErr := odhSnapshot.Update(ctx, ua, param)
	assert.NoError(retErr)
	assert.Equal(dSnapshot.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhSnapshot, m, ua).Return(ds.ErrorIDVerNotFound)
	odhSnapshot.crud = crud
	odhSnapshot.api = api
	odhSnapshot.log = l
	retMA, retErr = odhSnapshot.Update(ctx, ua, param)
	assert.Equal(ds.ErrorIDVerNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhSnapshot.api = api
	odhSnapshot.log = l
	retMA, retErr = odhSnapshot.Update(ctx, ua, param)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}
