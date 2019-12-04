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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/protection_domain"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	primitive "go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// gomock.Matcher for ProtectionDomain
type mockProtectionDomainMatchCtx int

const (
	mockProtectionDomainInvalid mockProtectionDomainMatchCtx = iota
	mockProtectionDomainInsert
	mockProtectionDomainFind
	mockProtectionDomainUpdate
)

type mockProtectionDomainMatcher struct {
	t      *testing.T
	ctx    mockProtectionDomainMatchCtx
	ctxObj *ProtectionDomain
	retObj *ProtectionDomain
}

func newProtectionDomainMatcher(t *testing.T, ctx mockProtectionDomainMatchCtx, obj *ProtectionDomain) gomock.Matcher {
	return protectionDomainMatcher(t, ctx).CtxObj(obj).Matcher()
}

// protectionDomainMatcher creates a partially initialized mockProtectionDomainMatcher
func protectionDomainMatcher(t *testing.T, ctx mockProtectionDomainMatchCtx) *mockProtectionDomainMatcher {
	return &mockProtectionDomainMatcher{t: t, ctx: ctx}
}

// Matcher ends the chain
func (o *mockProtectionDomainMatcher) Matcher() gomock.Matcher {
	if o.ctx == mockProtectionDomainInsert || o.ctx == mockProtectionDomainFind || o.ctx == mockProtectionDomainUpdate {
		assert.NotNil(o.t, o.ctxObj)
	}
	return o
}

// CtxObj adds an ProtectionDomain object
func (o *mockProtectionDomainMatcher) CtxObj(ao *ProtectionDomain) *mockProtectionDomainMatcher {
	o.ctxObj = ao
	return o
}

// Return sets the update result object
func (o *mockProtectionDomainMatcher) Return(ro *ProtectionDomain) *mockProtectionDomainMatcher {
	o.retObj = ro
	return o
}

// Matches is from gomock.Matcher
func (o *mockProtectionDomainMatcher) Matches(x interface{}) bool {
	var obj *ProtectionDomain
	switch z := x.(type) {
	case *ProtectionDomain:
		obj = z
	default:
		panic("Did not get a recognizable object: " + reflect.TypeOf(x).String())
	}
	assert := assert.New(o.t)
	switch o.ctx {
	case mockProtectionDomainInsert:
		compObj := o.ctxObj
		compObj.ObjMeta = obj.ObjMeta // insert sets meta data
		return assert.NotZero(obj.ObjMeta.MetaObjID) &&
			assert.Equal(int32(1), obj.ObjMeta.MetaVersion) &&
			assert.False(obj.ObjMeta.MetaTimeCreated.IsZero()) &&
			assert.False(obj.ObjMeta.MetaTimeModified.IsZero()) &&
			assert.Equal(obj.ObjMeta.MetaTimeCreated, obj.ObjMeta.MetaTimeModified) &&
			assert.Equal(compObj, obj)
	case mockProtectionDomainFind:
		*obj = *o.ctxObj
		return true
	case mockProtectionDomainUpdate:
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
func (o *mockProtectionDomainMatcher) String() string {
	switch o.ctx {
	case mockProtectionDomainInsert:
		return "matches on insert"
	}
	return "unknown context"
}

func TestProtectionDomainHandler(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	assert.Equal("protectionDomain", odhProtectionDomain.CName())
	indexes := odhProtectionDomain.Indexes()
	expIndexes := []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: accountIDKey, Value: IndexAscending}, {Key: nameKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
	}
	assert.Equal(expIndexes, indexes)
	assert.Equal(indexes, odhProtectionDomain.indexes)
	dIf := odhProtectionDomain.NewObject()
	assert.NotNil(dIf)
	_, ok := dIf.(*ProtectionDomain)
	assert.True(ok)

	t.Log("case: Claim + Initialize success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ctx := context.Background()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	odhProtectionDomain.Claim(api, crud)
	assert.Equal(api, odhProtectionDomain.api)
	assert.Equal(crud, odhProtectionDomain.crud)
	assert.Equal(l, odhProtectionDomain.log)
	assert.Equal(odhProtectionDomain, odhProtectionDomain.Ops())
	crud.EXPECT().CreateIndexes(ctx, odhProtectionDomain).Return(nil)
	assert.NoError(odhProtectionDomain.Initialize(ctx))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize, error creating indexes")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhProtectionDomain).Return(errUnknownError)
	odhProtectionDomain.crud = crud
	odhProtectionDomain.api = api
	assert.Error(errUnknownError, odhProtectionDomain.Initialize(ctx))

	t.Log("case: Start")
	api.EXPECT().DBName().Return("dbName")
	assert.NoError(odhProtectionDomain.Start(ctx))
}

func TestProtectionDomainCount(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	t.Log("case: success")
	ctx := context.Background()
	params := protection_domain.ProtectionDomainListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhProtectionDomain, fArg, uint(0)).Return(2, nil)
	odhProtectionDomain.crud = crud
	odhProtectionDomain.api = api
	odhProtectionDomain.log = l
	retMA, retErr := odhProtectionDomain.Count(ctx, params, 0)
	assert.Equal(2, retMA)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: count fails")
	mockCtrl = gomock.NewController(t)
	params = protection_domain.ProtectionDomainListParams{
		SystemTags: []string{"tag1", "tag2"},
	}
	fArg = bson.M{
		"systemtags": bson.M{"$all": params.SystemTags},
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhProtectionDomain, fArg, uint(5)).Return(0, errWrappedError)
	odhProtectionDomain.crud = crud
	odhProtectionDomain.api = api
	odhProtectionDomain.log = l
	retMA, retErr = odhProtectionDomain.Count(ctx, params, 5)
	assert.Zero(retMA)
	assert.Equal(errWrappedError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhProtectionDomain.api = api
	odhProtectionDomain.log = l
	retMA, retErr = odhProtectionDomain.Count(ctx, params, 0)
	assert.Equal(errWrappedError, retErr)
	assert.Zero(retMA)
}

func TestProtectionDomainCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mPdCA := &models.ProtectionDomainCreateArgs{
		ProtectionDomainCreateOnce: models.ProtectionDomainCreateOnce{},
		ProtectionDomainMutable: models.ProtectionDomainMutable{
			SystemTags: []string{"stag1", "stag2"},
		},
	}
	mC := &models.ProtectionDomain{
		ProtectionDomainAllOf0: models.ProtectionDomainAllOf0{
			Meta: &models.ObjMeta{ObjType: "ProtectionDomain"},
		},
		ProtectionDomainCreateOnce: mPdCA.ProtectionDomainCreateOnce,
		ProtectionDomainMutable:    mPdCA.ProtectionDomainMutable,
	}
	dProtectionDomain := &ProtectionDomain{}
	dProtectionDomain.FromModel(mC) // initialize empty properties
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().InsertOne(ctx, odhProtectionDomain, newProtectionDomainMatcher(t, mockProtectionDomainInsert, dProtectionDomain)).Return(nil)
	odhProtectionDomain.crud = crud
	odhProtectionDomain.api = api
	odhProtectionDomain.log = l
	retMA, retErr := odhProtectionDomain.Create(ctx, mPdCA)
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
	crud.EXPECT().InsertOne(ctx, odhProtectionDomain, newProtectionDomainMatcher(t, mockProtectionDomainInsert, dProtectionDomain)).Return(ds.ErrorExists)
	odhProtectionDomain.crud = crud
	odhProtectionDomain.api = api
	odhProtectionDomain.log = l
	retMA, retErr = odhProtectionDomain.Create(ctx, mPdCA)
	assert.Equal(ds.ErrorExists, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhProtectionDomain.api = api
	odhProtectionDomain.log = l
	retMA, retErr = odhProtectionDomain.Create(ctx, mPdCA)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestProtectionDomainDelete(t *testing.T) {
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
	crud.EXPECT().DeleteOne(ctx, odhProtectionDomain, mID).Return(nil)
	odhProtectionDomain.crud = crud
	odhProtectionDomain.api = api
	odhProtectionDomain.log = l
	retErr := odhProtectionDomain.Delete(ctx, mID)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().DeleteOne(ctx, odhProtectionDomain, mID).Return(errUnknownError)
	odhProtectionDomain.crud = crud
	odhProtectionDomain.api = api
	odhProtectionDomain.log = l
	retErr = odhProtectionDomain.Delete(ctx, mID)
	assert.Equal(errUnknownError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhProtectionDomain.api = api
	odhProtectionDomain.log = l
	retErr = odhProtectionDomain.Delete(ctx, mID)
	assert.Equal(errWrappedError, retErr)
}

func TestProtectionDomainFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mTime := time.Now()
	cTime := mTime.AddDate(0, 0, -1)
	dProtectionDomain := &ProtectionDomain{
		ObjMeta: ObjMeta{
			MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
			MetaTimeCreated:  cTime,
			MetaTimeModified: mTime,
		},
		SystemTags: []string{"stag1", "stag2"},
	}
	fArg := bson.M{objKey: dProtectionDomain.ObjMeta.MetaObjID}
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhProtectionDomain, fArg, newProtectionDomainMatcher(t, mockProtectionDomainFind, dProtectionDomain)).Return(nil)
	odhProtectionDomain.crud = crud
	odhProtectionDomain.api = api
	odhProtectionDomain.log = l
	retMA, retErr := odhProtectionDomain.Fetch(ctx, dProtectionDomain.ObjMeta.MetaObjID)
	assert.NoError(retErr)
	assert.Equal(dProtectionDomain.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not found")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhProtectionDomain, fArg, newProtectionDomainMatcher(t, mockProtectionDomainFind, dProtectionDomain)).Return(ds.ErrorNotFound)
	odhProtectionDomain.crud = crud
	odhProtectionDomain.api = api
	odhProtectionDomain.log = l
	retMA, retErr = odhProtectionDomain.Fetch(ctx, dProtectionDomain.ObjMeta.MetaObjID)
	assert.Equal(ds.ErrorNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhProtectionDomain.api = api
	odhProtectionDomain.log = l
	retMA, retErr = odhProtectionDomain.Fetch(ctx, dProtectionDomain.ObjMeta.MetaObjID)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestProtectionDomainList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	now := time.Now()
	mTime1 := now.AddDate(-1, 0, 0)
	mTime2 := now.AddDate(0, -1, -2)
	dProtectionDomains := []interface{}{
		&ProtectionDomain{
			ObjMeta: ObjMeta{
				MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
				MetaTimeCreated:  now.AddDate(-1, -2, -1),
				MetaTimeModified: mTime1,
			},
			AccountID:  "accountID",
			SystemTags: []string{"stag1", "stag2"},
			Tags:       []string{"tag1"},
		},
		&ProtectionDomain{
			ObjMeta: ObjMeta{
				MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e1",
				MetaTimeCreated:  now.AddDate(0, -2, -1),
				MetaTimeModified: mTime2,
			},
			AccountID: "accountID",
		},
	}
	ctx := context.Background()

	t.Log("case: success")
	params := protection_domain.ProtectionDomainListParams{
		AccountID:   swag.String("accountID"),
		Name:        swag.String("name"),
		NamePattern: swag.String("namepattern"),
		SystemTags:  []string{"stag1", "stag2"},
		Tags:        []string{"tag1"},
	}
	fArg := bson.M{
		"accountid":  *params.AccountID,
		"name":       *params.Name, // name overrides namePattern
		"systemtags": bson.M{"$all": params.SystemTags},
		"tags":       bson.M{"$all": params.Tags},
	}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhProtectionDomain, params, fArg, newConsumeObjFnMatcher(t, dProtectionDomains...)).Return(nil)
	odhProtectionDomain.crud = crud
	odhProtectionDomain.api = api
	odhProtectionDomain.log = l
	retMA, retErr := odhProtectionDomain.List(ctx, params)
	assert.NoError(retErr)
	assert.Len(retMA, len(dProtectionDomains))
	for i, mA := range retMA {
		o := dProtectionDomains[i].(*ProtectionDomain)
		assert.Equal(o.ToModel(), mA)
	}
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	params = protection_domain.ProtectionDomainListParams{
		AccountID:   swag.String("accountID"),
		NamePattern: swag.String("namepattern"),
		SystemTags:  []string{"stag1", "stag2"},
		Tags:        []string{"tag1"},
	}
	fArg = bson.M{
		"accountid":  *params.AccountID,
		"name":       primitive.Regex{Pattern: *params.NamePattern},
		"systemtags": bson.M{"$all": params.SystemTags},
		"tags":       bson.M{"$all": params.Tags},
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhProtectionDomain, params, fArg, gomock.Any()).Return(errWrappedError)
	odhProtectionDomain.crud = crud
	odhProtectionDomain.api = api
	odhProtectionDomain.log = l
	retMA, retErr = odhProtectionDomain.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhProtectionDomain.api = api
	odhProtectionDomain.log = l
	retMA, retErr = odhProtectionDomain.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestProtectionDomainUpdate(t *testing.T) {
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
	param := &models.ProtectionDomainMutable{
		SystemTags: []string{"stag1", "stag2"},
	}
	mObj := &models.ProtectionDomain{ProtectionDomainMutable: *param}
	dObj := &ProtectionDomain{}
	dObj.FromModel(mObj)
	now := time.Now()
	dProtectionDomain := &ProtectionDomain{
		ObjMeta: ObjMeta{
			MetaObjID:        ua.ID,
			MetaTimeCreated:  now.AddDate(0, -2, -1),
			MetaTimeModified: now,
			MetaVersion:      1000,
		},
	}
	m := protectionDomainMatcher(t, mockProtectionDomainUpdate).CtxObj(dObj).Return(dProtectionDomain).Matcher()
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhProtectionDomain, m, ua).Return(nil)
	odhProtectionDomain.crud = crud
	odhProtectionDomain.api = api
	odhProtectionDomain.log = l
	retMA, retErr := odhProtectionDomain.Update(ctx, ua, param)
	assert.NoError(retErr)
	assert.Equal(dProtectionDomain.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhProtectionDomain, m, ua).Return(ds.ErrorIDVerNotFound)
	odhProtectionDomain.crud = crud
	odhProtectionDomain.api = api
	odhProtectionDomain.log = l
	retMA, retErr = odhProtectionDomain.Update(ctx, ua, param)
	assert.Equal(ds.ErrorIDVerNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhProtectionDomain.api = api
	odhProtectionDomain.log = l
	retMA, retErr = odhProtectionDomain.Update(ctx, ua, param)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}
