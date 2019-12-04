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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/storage"
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

// gomock.Matcher for Storage
type mockStorageMatchCtx int

const (
	mockStorageInvalid mockStorageMatchCtx = iota
	mockStorageInsert
	mockStorageFind
	mockStorageUpdate
)

type mockStorageMatcher struct {
	t      *testing.T
	ctx    mockStorageMatchCtx
	ctxObj *Storage
	retObj *Storage
}

func newStorageMatcher(t *testing.T, ctx mockStorageMatchCtx, obj *Storage) gomock.Matcher {
	return storageMatcher(t, ctx).CtxObj(obj).Matcher()
}

// storageMatcher creates a partially initialized mockStorageMatcher
func storageMatcher(t *testing.T, ctx mockStorageMatchCtx) *mockStorageMatcher {
	return &mockStorageMatcher{t: t, ctx: ctx}
}

// Matcher ends the chain
func (o *mockStorageMatcher) Matcher() gomock.Matcher {
	if o.ctx == mockStorageInsert || o.ctx == mockStorageFind || o.ctx == mockStorageUpdate {
		assert.NotNil(o.t, o.ctxObj)
	}
	return o
}

// CtxObj adds an Storage object
func (o *mockStorageMatcher) CtxObj(ao *Storage) *mockStorageMatcher {
	o.ctxObj = ao
	return o
}

// Return sets the update result object
func (o *mockStorageMatcher) Return(ro *Storage) *mockStorageMatcher {
	o.retObj = ro
	return o
}

// Matches is from gomock.Matcher
func (o *mockStorageMatcher) Matches(x interface{}) bool {
	var obj *Storage
	switch z := x.(type) {
	case *Storage:
		obj = z
	default:
		panic("Did not get a recognizable object: " + reflect.TypeOf(x).String())
	}
	assert := assert.New(o.t)
	switch o.ctx {
	case mockStorageInsert:
		compObj := o.ctxObj
		compObj.ObjMeta = obj.ObjMeta // insert sets meta data
		return assert.NotZero(obj.ObjMeta.MetaObjID) &&
			assert.Equal(int32(1), obj.ObjMeta.MetaVersion) &&
			assert.False(obj.ObjMeta.MetaTimeCreated.IsZero()) &&
			assert.False(obj.ObjMeta.MetaTimeModified.IsZero()) &&
			assert.Equal(obj.ObjMeta.MetaTimeCreated, obj.ObjMeta.MetaTimeModified) &&
			assert.Equal(compObj, obj)
	case mockStorageFind:
		*obj = *o.ctxObj
		return true
	case mockStorageUpdate:
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
func (o *mockStorageMatcher) String() string {
	switch o.ctx {
	case mockStorageInsert:
		return "matches on insert"
	}
	return "unknown context"
}

func TestStorageHandler(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	assert.Equal("storage", odhStorage.CName())
	indexes := odhStorage.Indexes()
	expIndexes := []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
	}
	assert.Equal(expIndexes, indexes)
	assert.Equal(indexes, odhStorage.indexes)
	dIf := odhStorage.NewObject()
	assert.NotNil(dIf)
	_, ok := dIf.(*Storage)
	assert.True(ok)

	t.Log("case: Claim + Initialize success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ctx := context.Background()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	odhStorage.Claim(api, crud)
	assert.Equal(api, odhStorage.api)
	assert.Equal(crud, odhStorage.crud)
	assert.Equal(l, odhStorage.log)
	assert.Equal(odhStorage, odhStorage.Ops())
	crud.EXPECT().CreateIndexes(ctx, odhStorage).Return(nil)
	assert.NoError(odhStorage.Initialize(ctx))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize, error creating indexes")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhStorage).Return(errUnknownError)
	odhStorage.crud = crud
	odhStorage.api = api
	assert.Error(errUnknownError, odhStorage.Initialize(ctx))

	t.Log("case: Start")
	api.EXPECT().DBName().Return("dbName")
	assert.NoError(odhStorage.Start(ctx))
}

func TestStorageAggregate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	t.Log("case: success")
	ctx := context.Background()
	params := storage.StorageListParams{Sum: []string{"sizeBytes", "availableBytes"}}
	fArg := bson.M{}
	exp := []*ds.Aggregation{
		&ds.Aggregation{FieldPath: "sizeBytes", Type: "sum", Value: 1010101010101},
		&ds.Aggregation{FieldPath: "availableBytes", Type: "sum", Value: 101010101010},
	}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Aggregate(ctx, odhStorage, &params, fArg).Return(exp, 5, nil)
	odhStorage.crud = crud
	odhStorage.api = api
	odhStorage.log = l
	retMA, retC, retErr := odhStorage.Aggregate(ctx, params)
	assert.Equal(exp, retMA)
	assert.Equal(5, retC)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	params = storage.StorageListParams{
		CspDomainID: swag.String("domain1"),
		Sum:         []string{"sizeBytes"},
	}
	fArg = bson.M{"cspdomainid": "domain1"}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Aggregate(ctx, odhStorage, &params, fArg).Return(nil, 0, errWrappedError)
	odhStorage.crud = crud
	odhStorage.api = api
	odhStorage.log = l
	retMA, retC, retErr = odhStorage.Aggregate(ctx, params)
	assert.Nil(retMA)
	assert.Zero(retC)
	assert.Equal(errWrappedError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhStorage.api = api
	odhStorage.log = l
	retMA, retC, retErr = odhStorage.Aggregate(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
	assert.Zero(retC)
}

func TestStorageCount(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	t.Log("case: success")
	ctx := context.Background()
	params := storage.StorageListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhStorage, fArg, uint(0)).Return(2, nil)
	odhStorage.crud = crud
	odhStorage.api = api
	odhStorage.log = l
	retMA, retErr := odhStorage.Count(ctx, params, 0)
	assert.Equal(2, retMA)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: count fails")
	mockCtrl = gomock.NewController(t)
	params = storage.StorageListParams{
		CspDomainID: swag.String("domain1"),
	}
	fArg = bson.M{
		"cspdomainid": *params.CspDomainID,
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhStorage, fArg, uint(5)).Return(0, errWrappedError)
	odhStorage.crud = crud
	odhStorage.api = api
	odhStorage.log = l
	retMA, retErr = odhStorage.Count(ctx, params, 5)
	assert.Zero(retMA)
	assert.Equal(errWrappedError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhStorage.api = api
	odhStorage.log = l
	retMA, retErr = odhStorage.Count(ctx, params, 0)
	assert.Equal(errWrappedError, retErr)
	assert.Zero(retMA)
}

func TestStorageCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	cTime := time.Now()
	dStorage := &Storage{
		AccountID:            "aid1",
		TenantAccountID:      "tenant1",
		CspDomainID:          "domainID",
		CspStorageType:       "Amazon gp2",
		StorageAccessibility: StorageAccessibility{"CSPDOMAIN", "domainID"},
		StorageIdentifier:    "aws-specific-storage-identifier",
		PoolID:               "poolID",
		ClusterID:            "clusterID",
		StorageState: StorageState{
			AttachedNodeDevice: "",
			AttachedNodeID:     "",
			AttachmentState:    "DETACHED",
			DeviceState:        "UNUSED",
			MediaState:         "UNFORMATTED",
			Messages: []TimestampedString{
				TimestampedString{Message: "message1", Time: cTime},
			},
			ProvisionedState: "UNPROVISIONED",
		},
	}
	mStorage := dStorage.ToModel()
	mStorage.Meta = &models.ObjMeta{}
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().InsertOne(ctx, odhStorage, newStorageMatcher(t, mockStorageInsert, dStorage)).Return(nil)
	odhStorage.crud = crud
	odhStorage.api = api
	odhStorage.log = l
	retMA, retErr := odhStorage.Create(ctx, mStorage)
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
	crud.EXPECT().InsertOne(ctx, odhStorage, newStorageMatcher(t, mockStorageInsert, dStorage)).Return(ds.ErrorExists)
	odhStorage.crud = crud
	odhStorage.api = api
	odhStorage.log = l
	retMA, retErr = odhStorage.Create(ctx, mStorage)
	assert.Equal(ds.ErrorExists, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhStorage.api = api
	odhStorage.log = l
	retMA, retErr = odhStorage.Create(ctx, mStorage)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestStorageDelete(t *testing.T) {
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
	crud.EXPECT().DeleteOne(ctx, odhStorage, mID).Return(nil)
	odhStorage.crud = crud
	odhStorage.api = api
	odhStorage.log = l
	retErr := odhStorage.Delete(ctx, mID)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().DeleteOne(ctx, odhStorage, mID).Return(errUnknownError)
	odhStorage.crud = crud
	odhStorage.api = api
	odhStorage.log = l
	retErr = odhStorage.Delete(ctx, mID)
	assert.Equal(errUnknownError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhStorage.api = api
	odhStorage.log = l
	retErr = odhStorage.Delete(ctx, mID)
	assert.Equal(errWrappedError, retErr)
}

func TestStorageFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mTime := time.Now()
	cTime := mTime.AddDate(0, 0, -1)
	dStorage := &Storage{
		ObjMeta: ObjMeta{
			MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
			MetaTimeCreated:  cTime,
			MetaTimeModified: mTime,
		},
		AccountID:         "aid1",
		TenantAccountID:   "tenant1",
		CspDomainID:       "domainID",
		PoolID:            "pool1",
		StorageIdentifier: "aws-specific-storage-identifier",
	}
	fArg := bson.M{objKey: dStorage.ObjMeta.MetaObjID}
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhStorage, fArg, newStorageMatcher(t, mockStorageFind, dStorage)).Return(nil)
	odhStorage.crud = crud
	odhStorage.api = api
	odhStorage.log = l
	retMA, retErr := odhStorage.Fetch(ctx, dStorage.ObjMeta.MetaObjID)
	assert.NoError(retErr)
	assert.Equal(dStorage.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not found")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhStorage, fArg, newStorageMatcher(t, mockStorageFind, dStorage)).Return(ds.ErrorNotFound)
	odhStorage.crud = crud
	odhStorage.api = api
	odhStorage.log = l
	retMA, retErr = odhStorage.Fetch(ctx, dStorage.ObjMeta.MetaObjID)
	assert.Equal(ds.ErrorNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhStorage.api = api
	odhStorage.log = l
	retMA, retErr = odhStorage.Fetch(ctx, dStorage.ObjMeta.MetaObjID)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestStorageList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	now := time.Now()
	mTime1 := now.AddDate(-1, 0, 0)
	mTime2 := now.AddDate(0, -1, -2)
	dStorages := []interface{}{
		&Storage{
			ObjMeta: ObjMeta{
				MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
				MetaTimeCreated:  now.AddDate(-1, -2, -1),
				MetaTimeModified: mTime1,
			},
			AccountID:         "aid1",
			TenantAccountID:   "tenant1",
			CspDomainID:       "domainID",
			StorageIdentifier: "aws-specific-storage-identifier1",
			SystemTags:        []string{"stag1", "stag2"},
		},
		&Storage{
			ObjMeta: ObjMeta{
				MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e1",
				MetaTimeCreated:  now.AddDate(0, -2, -1),
				MetaTimeModified: mTime2,
			},
			AccountID:         "aid2",
			TenantAccountID:   "tenant1",
			CspDomainID:       "domainID",
			StorageIdentifier: "aws-specific-storage-identifier2",
			SystemTags:        []string{"stag1", "stag2"},
		},
	}
	ctx := context.Background()

	t.Log("case: success")
	params := storage.StorageListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhStorage, params, fArg, newConsumeObjFnMatcher(t, dStorages...)).Return(nil)
	odhStorage.crud = crud
	odhStorage.api = api
	odhStorage.log = l
	retMA, retErr := odhStorage.List(ctx, params)
	assert.NoError(retErr)
	assert.Len(retMA, len(dStorages))
	for i, mA := range retMA {
		o := dStorages[i].(*Storage)
		assert.Equal(o.ToModel(), mA)
	}
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	params = storage.StorageListParams{
		AccountID:        swag.String("accountId"),
		CspDomainID:      swag.String("domainID"),
		CspStorageType:   swag.String("Amazon io1"),
		PoolID:           swag.String("poolID"),
		AttachedNodeID:   swag.String("nodeID"),
		AvailableBytesLT: swag.Int64(10485760),
		ProvisionedState: swag.String("PROVISIONED"),
		DeviceState:      swag.String("OPEN"),
		MediaState:       swag.String("UNFORMATTED"),
		SystemTags:       []string{"stag1", "stag2"},
	}
	fArg = bson.M{
		"accountid":                     *params.AccountID,
		"cspdomainid":                   *params.CspDomainID,
		"cspstoragetype":                *params.CspStorageType,
		"poolid":                        *params.PoolID,
		"storagestate.attachednodeid":   *params.AttachedNodeID,
		"storagestate.attachmentstate":  bson.M{"$ne": ds.DefaultStorageAttachmentState},
		"availablebytes":                bson.M{"$lt": *params.AvailableBytesLT},
		"storagestate.provisionedstate": *params.ProvisionedState,
		"storagestate.devicestate":      *params.DeviceState,
		"storagestate.mediastate":       *params.MediaState,
		"systemtags":                    bson.M{"$all": params.SystemTags},
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhStorage, params, fArg, gomock.Any()).Return(errWrappedError)
	odhStorage.crud = crud
	odhStorage.api = api
	odhStorage.log = l
	retMA, retErr = odhStorage.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	// case: more query combinations
	qTcs := []struct {
		p storage.StorageListParams
		q bson.M
	}{
		{ // [0]
			storage.StorageListParams{
				TenantAccountID:         swag.String("tenant1"),
				CspDomainID:             swag.String("domainID"),
				AccessibilityScope:      swag.String("CSPDOMAIN"),
				AccessibilityScopeObjID: swag.String("domainID"),
				AvailableBytesGE:        swag.Int64(1048576),
				ClusterID:               swag.String("clusterID"),
				AttachedNodeID:          swag.String("nodeID"),
				AttachmentState:         swag.String("ATTACHED"), // overrides not-DETACHED
				ProvisionedState:        swag.String("Value overridden by IsProvisioned"),
				IsProvisioned:           swag.Bool(true),
			},
			bson.M{
				"tenantaccountid": "tenant1",
				"cspdomainid":     "domainID",
				"storageaccessibility.accessibilityscope":      "CSPDOMAIN",
				"storageaccessibility.accessibilityscopeobjid": "domainID",
				"availablebytes":                bson.M{"$gte": int64(1048576)},
				"clusterid":                     "clusterID",
				"storagestate.attachednodeid":   "nodeID",
				"storagestate.attachmentstate":  "ATTACHED",
				"storagestate.provisionedstate": bson.M{"$in": []string{"PROVISIONED", "UNPROVISIONING"}},
			},
		},
		{ // [1]
			storage.StorageListParams{
				AccountID:        swag.String("accountId2"),
				TenantAccountID:  swag.String("tenant1"),
				CspDomainID:      swag.String("domainID"),
				AvailableBytesGE: swag.Int64(1048576),
				AvailableBytesLT: swag.Int64(10485760),
				ProvisionedState: swag.String("Value overridden by IsProvisioned"),
				IsProvisioned:    swag.Bool(false),
			},
			bson.M{
				"$or": []bson.M{
					bson.M{"accountid": "accountId2"},
					bson.M{"tenantaccountid": "tenant1"},
				},
				"cspdomainid": "domainID",
				"$and": []bson.M{
					bson.M{"availablebytes": bson.M{"$gte": int64(1048576)}},
					bson.M{"availablebytes": bson.M{"$lt": int64(10485760)}},
				},
				"storagestate.provisionedstate": bson.M{"$nin": []string{"PROVISIONED", "UNPROVISIONING"}},
			},
		},
	}
	for i, tc := range qTcs {
		q := odhStorage.convertListParams(tc.p)
		assert.Equal(tc.q, q, "Query[%d]", i)
	}

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhStorage.api = api
	odhStorage.log = l
	retMA, retErr = odhStorage.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestStorageUpdate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	ua := &ds.UpdateArgs{
		ID:      "c72bcfb7-90d4-4276-89ef-aa4bacd09ec7",
		Version: 0,
		Attributes: []ds.UpdateAttr{
			{
				Name: "ParcelSizeBytes",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			}, {
				Name: "StorageState",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	param := &models.StorageMutable{
		ParcelSizeBytes: swag.Int64(1024000),
		StorageState: &models.StorageStateMutable{
			ProvisionedState: "UNPROVISIONED",
			Messages:         []*models.TimestampedString{},
		},
	}
	mObj := &models.Storage{StorageMutable: *param}
	dObj := &Storage{}
	dObj.FromModel(mObj)
	now := time.Now()
	dStorage := &Storage{
		ObjMeta: ObjMeta{
			MetaObjID:        ua.ID,
			MetaTimeCreated:  now.AddDate(0, -2, -1),
			MetaTimeModified: now,
			MetaVersion:      1000,
		},
	}
	m := storageMatcher(t, mockStorageUpdate).CtxObj(dObj).Return(dStorage).Matcher()
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhStorage, m, ua).Return(nil)
	odhStorage.crud = crud
	odhStorage.api = api
	odhStorage.log = l
	retMA, retErr := odhStorage.Update(ctx, ua, param)
	assert.NoError(retErr)
	assert.Equal(dStorage.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhStorage, m, ua).Return(ds.ErrorIDVerNotFound)
	odhStorage.crud = crud
	odhStorage.api = api
	odhStorage.log = l
	retMA, retErr = odhStorage.Update(ctx, ua, param)
	assert.Equal(ds.ErrorIDVerNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhStorage.api = api
	odhStorage.log = l
	retMA, retErr = odhStorage.Update(ctx, ua, param)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}
