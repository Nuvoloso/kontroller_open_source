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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/csp_domain"
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

// gomock.Matcher for CspDomain
type mockCspDomainMatchCtx int

const (
	mockCspDomainInvalid mockCspDomainMatchCtx = iota
	mockCspDomainInsert
	mockCspDomainFind
	mockCspDomainUpdate
)

type mockCspDomainMatcher struct {
	t      *testing.T
	ctx    mockCspDomainMatchCtx
	ctxObj *CspDomain
	retObj *CspDomain
}

func newCspDomainMatcher(t *testing.T, ctx mockCspDomainMatchCtx, obj *CspDomain) gomock.Matcher {
	return cspDomainMatcher(t, ctx).CtxObj(obj).Matcher()
}

// cspDomainMatcher creates a partially initialized mockCspDomainMatcher
func cspDomainMatcher(t *testing.T, ctx mockCspDomainMatchCtx) *mockCspDomainMatcher {
	return &mockCspDomainMatcher{t: t, ctx: ctx}
}

// Matcher ends the chain
func (o *mockCspDomainMatcher) Matcher() gomock.Matcher {
	if o.ctx == mockCspDomainInsert || o.ctx == mockCspDomainFind || o.ctx == mockCspDomainUpdate {
		assert.NotNil(o.t, o.ctxObj)
	}
	return o
}

// CtxObj adds an CspDomain object
func (o *mockCspDomainMatcher) CtxObj(ao *CspDomain) *mockCspDomainMatcher {
	o.ctxObj = ao
	return o
}

// Return sets the update result object
func (o *mockCspDomainMatcher) Return(ro *CspDomain) *mockCspDomainMatcher {
	o.retObj = ro
	return o
}

// Matches is from gomock.Matcher
func (o *mockCspDomainMatcher) Matches(x interface{}) bool {
	var obj *CspDomain
	switch z := x.(type) {
	case *CspDomain:
		obj = z
	default:
		panic("Did not get a recognizable object: " + reflect.TypeOf(x).String())
	}
	assert := assert.New(o.t)
	switch o.ctx {
	case mockCspDomainInsert:
		compObj := o.ctxObj
		compObj.ObjMeta = obj.ObjMeta // insert sets meta data
		return assert.NotZero(obj.ObjMeta.MetaObjID) &&
			assert.Equal(int32(1), obj.ObjMeta.MetaVersion) &&
			assert.False(obj.ObjMeta.MetaTimeCreated.IsZero()) &&
			assert.False(obj.ObjMeta.MetaTimeModified.IsZero()) &&
			assert.Equal(obj.ObjMeta.MetaTimeCreated, obj.ObjMeta.MetaTimeModified) &&
			assert.Equal(compObj, obj)
	case mockCspDomainFind:
		*obj = *o.ctxObj
		return true
	case mockCspDomainUpdate:
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
func (o *mockCspDomainMatcher) String() string {
	switch o.ctx {
	case mockCspDomainInsert:
		return "matches on insert"
	}
	return "unknown context"
}

func TestCspDomainHandler(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	assert.Equal("cspdomain", odhCspDomain.CName())
	indexes := odhCspDomain.Indexes()
	expIndexes := []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: accountIDKey, Value: IndexAscending}, {Key: nameKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
	}
	assert.Equal(expIndexes, indexes)
	assert.Equal(indexes, odhCspDomain.indexes)
	dIf := odhCspDomain.NewObject()
	assert.NotNil(dIf)
	_, ok := dIf.(*CspDomain)
	assert.True(ok)

	t.Log("case: Claim + Initialize success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ctx := context.Background()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	odhCspDomain.Claim(api, crud)
	assert.Equal(api, odhCspDomain.api)
	assert.Equal(crud, odhCspDomain.crud)
	assert.Equal(l, odhCspDomain.log)
	assert.Equal(odhCspDomain, odhCspDomain.Ops())
	crud.EXPECT().CreateIndexes(ctx, odhCspDomain).Return(nil)
	assert.NoError(odhCspDomain.Initialize(ctx))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize, error creating indexes")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhCspDomain).Return(errUnknownError)
	odhCspDomain.crud = crud
	odhCspDomain.api = api
	assert.Error(errUnknownError, odhCspDomain.Initialize(ctx))

	t.Log("case: Start")
	api.EXPECT().DBName().Return("dbName")
	assert.NoError(odhCspDomain.Start(ctx))
}

func TestCspDomainCount(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	t.Log("case: success")
	ctx := context.Background()
	params := csp_domain.CspDomainListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhCspDomain, fArg, uint(0)).Return(2, nil)
	odhCspDomain.crud = crud
	odhCspDomain.api = api
	odhCspDomain.log = l
	retMA, retErr := odhCspDomain.Count(ctx, params, 0)
	assert.Equal(2, retMA)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: count fails")
	mockCtrl = gomock.NewController(t)
	params = csp_domain.CspDomainListParams{
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
	crud.EXPECT().Count(ctx, odhCspDomain, fArg, uint(5)).Return(0, errWrappedError)
	odhCspDomain.crud = crud
	odhCspDomain.api = api
	odhCspDomain.log = l
	retMA, retErr = odhCspDomain.Count(ctx, params, 5)
	assert.Zero(retMA)
	assert.Equal(errWrappedError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhCspDomain.api = api
	odhCspDomain.log = l
	retMA, retErr = odhCspDomain.Count(ctx, params, 0)
	assert.Equal(errWrappedError, retErr)
	assert.Zero(retMA)
}

func TestCspDomainCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	dCspDomain := &CspDomain{
		AccountID:          "systemId",
		CspDomainType:      "AWS",
		Name:               "testcspDomain",
		Description:        "test description",
		ManagementHost:     "host",
		Tags:               []string{"tag1", "tag2"},
		AuthorizedAccounts: []string{"aid1", "aid2"},
		CspDomainAttributes: StringValueMap{
			"property1": {Kind: "STRING", Value: "value1"},
			"property2": {Kind: "SECRET", Value: "secret1"},
		},
		StorageCosts: map[string]StorageCost{},
	}
	mCspDomain := dCspDomain.ToModel()
	mCspDomain.Meta = &models.ObjMeta{}
	mCspDomain.StorageCosts = nil
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().InsertOne(ctx, odhCspDomain, newCspDomainMatcher(t, mockCspDomainInsert, dCspDomain)).Return(nil)
	odhCspDomain.crud = crud
	odhCspDomain.api = api
	odhCspDomain.log = l
	retMA, retErr := odhCspDomain.Create(ctx, mCspDomain)
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
	crud.EXPECT().InsertOne(ctx, odhCspDomain, newCspDomainMatcher(t, mockCspDomainInsert, dCspDomain)).Return(ds.ErrorExists)
	odhCspDomain.crud = crud
	odhCspDomain.api = api
	odhCspDomain.log = l
	retMA, retErr = odhCspDomain.Create(ctx, mCspDomain)
	assert.Equal(ds.ErrorExists, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhCspDomain.api = api
	odhCspDomain.log = l
	retMA, retErr = odhCspDomain.Create(ctx, mCspDomain)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestCspDomainDelete(t *testing.T) {
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
	crud.EXPECT().DeleteOne(ctx, odhCspDomain, mID).Return(nil)
	odhCspDomain.crud = crud
	odhCspDomain.api = api
	odhCspDomain.log = l
	retErr := odhCspDomain.Delete(ctx, mID)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().DeleteOne(ctx, odhCspDomain, mID).Return(errUnknownError)
	odhCspDomain.crud = crud
	odhCspDomain.api = api
	odhCspDomain.log = l
	retErr = odhCspDomain.Delete(ctx, mID)
	assert.Equal(errUnknownError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhCspDomain.api = api
	odhCspDomain.log = l
	retErr = odhCspDomain.Delete(ctx, mID)
	assert.Equal(errWrappedError, retErr)
}

func TestCspDomainFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mTime := time.Now()
	cTime := mTime.AddDate(0, 0, -1)
	dCspDomain := &CspDomain{
		ObjMeta: ObjMeta{
			MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
			MetaTimeCreated:  cTime,
			MetaTimeModified: mTime,
		},
		CspDomainType:  "AWS",
		Name:           "testcspDomain",
		Description:    "test description",
		ManagementHost: "f.q.d.n",
		CspDomainAttributes: StringValueMap{
			"property1": {Kind: "STRING", Value: "value1"},
			"property2": {Kind: "SECRET", Value: "secret1"},
		},
		Tags: []string{"tag1", "tag2"},
	}
	fArg := bson.M{objKey: dCspDomain.ObjMeta.MetaObjID}
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhCspDomain, fArg, newCspDomainMatcher(t, mockCspDomainFind, dCspDomain)).Return(nil)
	odhCspDomain.crud = crud
	odhCspDomain.api = api
	odhCspDomain.log = l
	retMA, retErr := odhCspDomain.Fetch(ctx, dCspDomain.ObjMeta.MetaObjID)
	assert.NoError(retErr)
	assert.Equal(dCspDomain.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not found")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhCspDomain, fArg, newCspDomainMatcher(t, mockCspDomainFind, dCspDomain)).Return(ds.ErrorNotFound)
	odhCspDomain.crud = crud
	odhCspDomain.api = api
	odhCspDomain.log = l
	retMA, retErr = odhCspDomain.Fetch(ctx, dCspDomain.ObjMeta.MetaObjID)
	assert.Equal(ds.ErrorNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhCspDomain.api = api
	odhCspDomain.log = l
	retMA, retErr = odhCspDomain.Fetch(ctx, dCspDomain.ObjMeta.MetaObjID)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestCspDomainList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	now := time.Now()
	dCspDomains := []interface{}{
		&CspDomain{
			ObjMeta: ObjMeta{
				MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
				MetaTimeCreated:  now.AddDate(-1, -2, -1),
				MetaTimeModified: now.AddDate(-1, 0, 0),
			},
			Name:            "cspDomain1",
			Description:     "cspDomain1 description",
			ManagementHost:  "1.2.3.4",
			Tags:            []string{"tag1", "tag2"},
			CspCredentialID: "cspCred1",
		},
		&CspDomain{
			ObjMeta: ObjMeta{
				MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
				MetaTimeCreated:  now.AddDate(0, -2, -1),
				MetaTimeModified: now.AddDate(0, -1, -2),
			},
			Name:        "cspDomain2",
			Description: "cspDomain2 description",
		},
	}
	ctx := context.Background()

	t.Log("case: success")
	params := csp_domain.CspDomainListParams{
		Name: swag.String("servicePlan1"),
		Tags: []string{"tag1", "tag2"},
	}
	fArg := bson.M{
		"name": *params.Name,
		"tags": bson.M{"$all": params.Tags},
	}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhCspDomain, params, fArg, newConsumeObjFnMatcher(t, dCspDomains...)).Return(nil)
	odhCspDomain.crud = crud
	odhCspDomain.api = api
	odhCspDomain.log = l
	retMA, retErr := odhCspDomain.List(ctx, params)
	assert.NoError(retErr)
	assert.Len(retMA, len(dCspDomains))
	for i, mA := range retMA {
		o := dCspDomains[i].(*CspDomain)
		assert.Equal(o.ToModel(), mA)
	}
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	params = csp_domain.CspDomainListParams{
		AccountID:           swag.String("aid1"),
		AuthorizedAccountID: swag.String("aid2"),
		CspDomainType:       swag.String("AWS"),
		CspCredentialID:     swag.String("cspCred1"),
	}
	fArg = bson.M{
		"accountid":          *params.AccountID,
		"authorizedaccounts": *params.AuthorizedAccountID,
		"cspdomaintype":      *params.CspDomainType,
		"cspcredentialid":    *params.CspCredentialID,
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhCspDomain, params, fArg, gomock.Any()).Return(errWrappedError)
	odhCspDomain.crud = crud
	odhCspDomain.api = api
	odhCspDomain.log = l
	retMA, retErr = odhCspDomain.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhCspDomain.api = api
	odhCspDomain.log = l
	retMA, retErr = odhCspDomain.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestCspDomainUpdate(t *testing.T) {
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
	param := &models.CSPDomainMutable{
		Name: "newName",
		Tags: []string{"tag1", "tag2"},
	}
	mObj := &models.CSPDomain{CSPDomainMutable: *param}
	dObj := &CspDomain{}
	dObj.FromModel(mObj)
	now := time.Now()
	dCspDomain := &CspDomain{
		ObjMeta: ObjMeta{
			MetaObjID:        ua.ID,
			MetaTimeCreated:  now.AddDate(0, -2, -1),
			MetaTimeModified: now,
			MetaVersion:      9,
		},
	}
	m := cspDomainMatcher(t, mockCspDomainUpdate).CtxObj(dObj).Return(dCspDomain).Matcher()
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhCspDomain, m, ua).Return(nil)
	odhCspDomain.crud = crud
	odhCspDomain.api = api
	odhCspDomain.log = l
	retMA, retErr := odhCspDomain.Update(ctx, ua, param)
	assert.NoError(retErr)
	assert.Equal(dCspDomain.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhCspDomain, m, ua).Return(ds.ErrorIDVerNotFound)
	odhCspDomain.crud = crud
	odhCspDomain.api = api
	odhCspDomain.log = l
	retMA, retErr = odhCspDomain.Update(ctx, ua, param)
	assert.Equal(ds.ErrorIDVerNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhCspDomain.api = api
	odhCspDomain.log = l
	retMA, retErr = odhCspDomain.Update(ctx, ua, param)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}
