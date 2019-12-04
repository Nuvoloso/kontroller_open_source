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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/csp_credential"
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

// gomock.Matcher for CspCredential
type mockCspCredentialMatchCtx int

const (
	mockCspCredentialInvalid mockCspCredentialMatchCtx = iota
	mockCspCredentialInsert
	mockCspCredentialFind
	mockCspCredentialUpdate
)

type mockCspCredentialMatcher struct {
	t      *testing.T
	ctx    mockCspCredentialMatchCtx
	ctxObj *CspCredential
	retObj *CspCredential
}

func newCspCredentialMatcher(t *testing.T, ctx mockCspCredentialMatchCtx, obj *CspCredential) gomock.Matcher {
	return cspCredentialMatcher(t, ctx).CtxObj(obj).Matcher()
}

// cspCredentialMatcher creates a partially initialized mockCspCredentialMatcher
func cspCredentialMatcher(t *testing.T, ctx mockCspCredentialMatchCtx) *mockCspCredentialMatcher {
	return &mockCspCredentialMatcher{t: t, ctx: ctx}
}

// Matcher ends the chain
func (o *mockCspCredentialMatcher) Matcher() gomock.Matcher {
	if o.ctx == mockCspCredentialInsert || o.ctx == mockCspCredentialFind || o.ctx == mockCspCredentialUpdate {
		assert.NotNil(o.t, o.ctxObj)
	}
	return o
}

// CtxObj adds an CspCredential object
func (o *mockCspCredentialMatcher) CtxObj(ao *CspCredential) *mockCspCredentialMatcher {
	o.ctxObj = ao
	return o
}

// Return sets the update result object
func (o *mockCspCredentialMatcher) Return(ro *CspCredential) *mockCspCredentialMatcher {
	o.retObj = ro
	return o
}

// Matches is from gomock.Matcher
func (o *mockCspCredentialMatcher) Matches(x interface{}) bool {
	var obj *CspCredential
	switch z := x.(type) {
	case *CspCredential:
		obj = z
	default:
		panic("Did not get a recognizable object: " + reflect.TypeOf(x).String())
	}
	assert := assert.New(o.t)
	switch o.ctx {
	case mockCspCredentialInsert:
		compObj := o.ctxObj
		compObj.ObjMeta = obj.ObjMeta // insert sets meta data
		return assert.NotZero(obj.ObjMeta.MetaObjID) &&
			assert.Equal(int32(1), obj.ObjMeta.MetaVersion) &&
			assert.False(obj.ObjMeta.MetaTimeCreated.IsZero()) &&
			assert.False(obj.ObjMeta.MetaTimeModified.IsZero()) &&
			assert.Equal(obj.ObjMeta.MetaTimeCreated, obj.ObjMeta.MetaTimeModified) &&
			assert.Equal(compObj, obj)
	case mockCspCredentialFind:
		*obj = *o.ctxObj
		return true
	case mockCspCredentialUpdate:
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
func (o *mockCspCredentialMatcher) String() string {
	switch o.ctx {
	case mockCspCredentialInsert:
		return "matches on insert"
	}
	return "unknown context"
}

func TestCspCredentialHandler(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	assert.Equal("cspcredential", odhCspCredential.CName())
	indexes := odhCspCredential.Indexes()
	expIndexes := []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: accountIDKey, Value: IndexAscending}, {Key: nameKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
	}
	assert.Equal(expIndexes, indexes)
	assert.Equal(indexes, odhCspCredential.indexes)
	dIf := odhCspCredential.NewObject()
	assert.NotNil(dIf)
	_, ok := dIf.(*CspCredential)
	assert.True(ok)

	t.Log("case: Claim + Initialize success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ctx := context.Background()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	odhCspCredential.Claim(api, crud)
	assert.Equal(api, odhCspCredential.api)
	assert.Equal(crud, odhCspCredential.crud)
	assert.Equal(l, odhCspCredential.log)
	assert.Equal(odhCspCredential, odhCspCredential.Ops())
	crud.EXPECT().CreateIndexes(ctx, odhCspCredential).Return(nil)
	assert.NoError(odhCspCredential.Initialize(ctx))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize, error creating indexes")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhCspCredential).Return(errUnknownError)
	odhCspCredential.crud = crud
	odhCspCredential.api = api
	assert.Error(errUnknownError, odhCspCredential.Initialize(ctx))

	t.Log("case: Start")
	api.EXPECT().DBName().Return("dbName")
	assert.NoError(odhCspCredential.Start(ctx))
}
func TestCspCredentialCount(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	t.Log("case: success")
	ctx := context.Background()
	params := csp_credential.CspCredentialListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhCspCredential, fArg, uint(0)).Return(2, nil)
	odhCspCredential.crud = crud
	odhCspCredential.api = api
	odhCspCredential.log = l
	retMA, retErr := odhCspCredential.Count(ctx, params, 0)
	assert.Equal(2, retMA)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: count fails")
	mockCtrl = gomock.NewController(t)
	params = csp_credential.CspCredentialListParams{
		Name: swag.String("oid1"),
		Tags: []string{"tag1", "tag2"},
	}
	fArg = bson.M{
		"name": *params.Name,
		"tags": bson.M{"$all": params.Tags},
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhCspCredential, fArg, uint(5)).Return(0, errWrappedError)
	odhCspCredential.crud = crud
	odhCspCredential.api = api
	odhCspCredential.log = l
	retMA, retErr = odhCspCredential.Count(ctx, params, 5)
	assert.Zero(retMA)
	assert.Equal(errWrappedError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhCspCredential.api = api
	odhCspCredential.log = l
	retMA, retErr = odhCspCredential.Count(ctx, params, 0)
	assert.Equal(errWrappedError, retErr)
	assert.Zero(retMA)
}

func TestCspCredentialCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	dCspCredential := &CspCredential{
		AccountID:     "systemId",
		CspDomainType: "AWS",
		Name:          "testcspCredential",
		Description:   "test description",
		Tags:          []string{"tag1", "tag2"},
		CredentialAttributes: StringValueMap{
			"property1": {Kind: "STRING", Value: "value1"},
			"property2": {Kind: "SECRET", Value: "secret1"},
		},
	}
	mCspCredential := dCspCredential.ToModel()
	mCspCredential.Meta = &models.ObjMeta{}
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().InsertOne(ctx, odhCspCredential, newCspCredentialMatcher(t, mockCspCredentialInsert, dCspCredential)).Return(nil)
	odhCspCredential.crud = crud
	odhCspCredential.api = api
	odhCspCredential.log = l
	retMA, retErr := odhCspCredential.Create(ctx, mCspCredential)
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
	crud.EXPECT().InsertOne(ctx, odhCspCredential, newCspCredentialMatcher(t, mockCspCredentialInsert, dCspCredential)).Return(ds.ErrorExists)
	odhCspCredential.crud = crud
	odhCspCredential.api = api
	odhCspCredential.log = l
	retMA, retErr = odhCspCredential.Create(ctx, mCspCredential)
	assert.Equal(ds.ErrorExists, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhCspCredential.api = api
	odhCspCredential.log = l
	retMA, retErr = odhCspCredential.Create(ctx, mCspCredential)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestCspCredentialDelete(t *testing.T) {
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
	crud.EXPECT().DeleteOne(ctx, odhCspCredential, mID).Return(nil)
	odhCspCredential.crud = crud
	odhCspCredential.api = api
	odhCspCredential.log = l
	retErr := odhCspCredential.Delete(ctx, mID)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().DeleteOne(ctx, odhCspCredential, mID).Return(errUnknownError)
	odhCspCredential.crud = crud
	odhCspCredential.api = api
	odhCspCredential.log = l
	retErr = odhCspCredential.Delete(ctx, mID)
	assert.Equal(errUnknownError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhCspCredential.api = api
	odhCspCredential.log = l
	retErr = odhCspCredential.Delete(ctx, mID)
	assert.Equal(errWrappedError, retErr)
}

func TestCspCredentialFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mTime := time.Now()
	cTime := mTime.AddDate(0, 0, -1)
	dCspCredential := &CspCredential{
		ObjMeta: ObjMeta{
			MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
			MetaTimeCreated:  cTime,
			MetaTimeModified: mTime,
		},
		CspDomainType: "AWS",
		Name:          "testcspCredential",
		Description:   "test description",
		CredentialAttributes: StringValueMap{
			"property1": {Kind: "STRING", Value: "value1"},
			"property2": {Kind: "SECRET", Value: "secret1"},
		},
		Tags: []string{"tag1", "tag2"},
	}
	fArg := bson.M{objKey: dCspCredential.ObjMeta.MetaObjID}
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhCspCredential, fArg, newCspCredentialMatcher(t, mockCspCredentialFind, dCspCredential)).Return(nil)
	odhCspCredential.crud = crud
	odhCspCredential.api = api
	odhCspCredential.log = l
	retMA, retErr := odhCspCredential.Fetch(ctx, dCspCredential.ObjMeta.MetaObjID)
	assert.NoError(retErr)
	assert.Equal(dCspCredential.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not found")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhCspCredential, fArg, newCspCredentialMatcher(t, mockCspCredentialFind, dCspCredential)).Return(ds.ErrorNotFound)
	odhCspCredential.crud = crud
	odhCspCredential.api = api
	odhCspCredential.log = l
	retMA, retErr = odhCspCredential.Fetch(ctx, dCspCredential.ObjMeta.MetaObjID)
	assert.Equal(ds.ErrorNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhCspCredential.api = api
	odhCspCredential.log = l
	retMA, retErr = odhCspCredential.Fetch(ctx, dCspCredential.ObjMeta.MetaObjID)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestCspCredentialList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	now := time.Now()
	dCspCredentials := []interface{}{
		&CspCredential{
			ObjMeta: ObjMeta{
				MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
				MetaTimeCreated:  now.AddDate(-1, -2, -1),
				MetaTimeModified: now.AddDate(-1, 0, 0),
			},
			Name:        "cspCredential1",
			Description: "cspCredential1 description",
			Tags:        []string{"tag1", "tag2"},
		},
		&CspCredential{
			ObjMeta: ObjMeta{
				MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
				MetaTimeCreated:  now.AddDate(0, -2, -1),
				MetaTimeModified: now.AddDate(0, -1, -2),
			},
			Name:        "cspCredential2",
			Description: "cspCredential2 description",
		},
	}
	ctx := context.Background()

	t.Log("case: success")
	params := csp_credential.CspCredentialListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhCspCredential, params, fArg, newConsumeObjFnMatcher(t, dCspCredentials...)).Return(nil)
	odhCspCredential.crud = crud
	odhCspCredential.api = api
	odhCspCredential.log = l
	retMA, retErr := odhCspCredential.List(ctx, params)
	assert.NoError(retErr)
	assert.Len(retMA, len(dCspCredentials))
	for i, mA := range retMA {
		o := dCspCredentials[i].(*CspCredential)
		assert.Equal(o.ToModel(), mA)
	}
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	params = csp_credential.CspCredentialListParams{
		AccountID:     swag.String("aid1"),
		Name:          swag.String("cspCredential1"),
		CspDomainType: swag.String("AWS"),
		Tags:          []string{"tag1", "tag2"},
	}
	fArg = bson.M{
		"accountid":     *params.AccountID,
		"name":          *params.Name,
		"cspdomaintype": *params.CspDomainType,
		"tags":          bson.M{"$all": params.Tags},
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhCspCredential, params, fArg, gomock.Any()).Return(errWrappedError)
	odhCspCredential.crud = crud
	odhCspCredential.api = api
	odhCspCredential.log = l
	retMA, retErr = odhCspCredential.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhCspCredential.api = api
	odhCspCredential.log = l
	retMA, retErr = odhCspCredential.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestCspCredentialUpdate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	ua := &ds.UpdateArgs{
		ID:      "c72bcfb7-90d4-4276-89ef-aa4bacd09ec7",
		Version: 0,
		Attributes: []ds.UpdateAttr{
			{
				Name: "Name",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
			{
				Name: "Tags",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateAppend: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	param := &models.CSPCredentialMutable{
		Name: "newName",
		Tags: []string{"tag1", "tag2"},
	}
	mObj := &models.CSPCredential{CSPCredentialMutable: *param}
	dObj := &CspCredential{}
	dObj.FromModel(mObj)
	now := time.Now()
	dCspCredential := &CspCredential{
		ObjMeta: ObjMeta{
			MetaObjID:        ua.ID,
			MetaTimeCreated:  now.AddDate(0, -2, -1),
			MetaTimeModified: now,
			MetaVersion:      9,
		},
	}
	m := cspCredentialMatcher(t, mockCspCredentialUpdate).CtxObj(dObj).Return(dCspCredential).Matcher()
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhCspCredential, m, ua).Return(nil)
	odhCspCredential.crud = crud
	odhCspCredential.api = api
	odhCspCredential.log = l
	retMA, retErr := odhCspCredential.Update(ctx, ua, param)
	assert.NoError(retErr)
	assert.Equal(dCspCredential.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhCspCredential, m, ua).Return(ds.ErrorIDVerNotFound)
	odhCspCredential.crud = crud
	odhCspCredential.api = api
	odhCspCredential.log = l
	retMA, retErr = odhCspCredential.Update(ctx, ua, param)
	assert.Equal(ds.ErrorIDVerNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhCspCredential.api = api
	odhCspCredential.log = l
	retMA, retErr = odhCspCredential.Update(ctx, ua, param)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}
