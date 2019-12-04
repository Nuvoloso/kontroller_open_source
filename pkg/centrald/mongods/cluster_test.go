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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/cluster"
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

// gomock.Matcher for Cluster
type mockClusterMatchCtx int

const (
	mockClusterInvalid mockClusterMatchCtx = iota
	mockClusterInsert
	mockClusterFind
	mockClusterUpdate
)

type mockClusterMatcher struct {
	t      *testing.T
	ctx    mockClusterMatchCtx
	ctxObj *Cluster
	retObj *Cluster
}

func newClusterMatcher(t *testing.T, ctx mockClusterMatchCtx, obj *Cluster) gomock.Matcher {
	return clusterMatcher(t, ctx).CtxObj(obj).Matcher()
}

// clusterMatcher creates a partially initialized mockClusterMatcher
func clusterMatcher(t *testing.T, ctx mockClusterMatchCtx) *mockClusterMatcher {
	return &mockClusterMatcher{t: t, ctx: ctx}
}

// Matcher ends the chain
func (o *mockClusterMatcher) Matcher() gomock.Matcher {
	if o.ctx == mockClusterInsert || o.ctx == mockClusterFind || o.ctx == mockClusterUpdate {
		assert.NotNil(o.t, o.ctxObj)
	}
	return o
}

// CtxObj adds an Cluster object
func (o *mockClusterMatcher) CtxObj(ao *Cluster) *mockClusterMatcher {
	o.ctxObj = ao
	return o
}

// Return sets the update result object
func (o *mockClusterMatcher) Return(ro *Cluster) *mockClusterMatcher {
	o.retObj = ro
	return o
}

// Matches is from gomock.Matcher
func (o *mockClusterMatcher) Matches(x interface{}) bool {
	var obj *Cluster
	switch z := x.(type) {
	case *Cluster:
		obj = z
	default:
		panic("Did not get a recognizable object: " + reflect.TypeOf(x).String())
	}
	assert := assert.New(o.t)
	switch o.ctx {
	case mockClusterInsert:
		compObj := o.ctxObj
		compObj.ObjMeta = obj.ObjMeta // insert sets meta data
		return assert.NotZero(obj.ObjMeta.MetaObjID) &&
			assert.Equal(int32(1), obj.ObjMeta.MetaVersion) &&
			assert.False(obj.ObjMeta.MetaTimeCreated.IsZero()) &&
			assert.False(obj.ObjMeta.MetaTimeModified.IsZero()) &&
			assert.Equal(obj.ObjMeta.MetaTimeCreated, obj.ObjMeta.MetaTimeModified) &&
			assert.Equal(compObj, obj)
	case mockClusterFind:
		*obj = *o.ctxObj
		return true
	case mockClusterUpdate:
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
func (o *mockClusterMatcher) String() string {
	switch o.ctx {
	case mockClusterInsert:
		return "matches on insert"
	}
	return "unknown context"
}

func TestClusterHandler(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	assert.Equal("cluster", odhCluster.CName())
	indexes := odhCluster.Indexes()
	expIndexes := []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: cspDomainIDKey, Value: IndexAscending}, {Key: nameKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
	}
	assert.Equal(expIndexes, indexes)
	assert.Equal(indexes, odhCluster.indexes)
	dIf := odhCluster.NewObject()
	assert.NotNil(dIf)
	_, ok := dIf.(*Cluster)
	assert.True(ok)

	t.Log("case: Claim + Initialize success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ctx := context.Background()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	odhCluster.Claim(api, crud)
	assert.Equal(api, odhCluster.api)
	assert.Equal(crud, odhCluster.crud)
	assert.Equal(l, odhCluster.log)
	assert.Equal(odhCluster, odhCluster.Ops())
	crud.EXPECT().CreateIndexes(ctx, odhCluster).Return(nil)
	assert.NoError(odhCluster.Initialize(ctx))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize, error creating indexes")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhCluster).Return(errUnknownError)
	odhCluster.crud = crud
	odhCluster.api = api
	assert.Error(errUnknownError, odhCluster.Initialize(ctx))

	t.Log("case: Start")
	api.EXPECT().DBName().Return("dbName")
	assert.NoError(odhCluster.Start(ctx))
}

func TestClusterCount(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	t.Log("case: success")
	ctx := context.Background()
	params := cluster.ClusterListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhCluster, fArg, uint(0)).Return(2, nil)
	odhCluster.crud = crud
	odhCluster.api = api
	odhCluster.log = l
	retMA, retErr := odhCluster.Count(ctx, params, 0)
	assert.Equal(2, retMA)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: count fails")
	mockCtrl = gomock.NewController(t)
	params = cluster.ClusterListParams{
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
	crud.EXPECT().Count(ctx, odhCluster, fArg, uint(5)).Return(0, errWrappedError)
	odhCluster.crud = crud
	odhCluster.api = api
	odhCluster.log = l
	retMA, retErr = odhCluster.Count(ctx, params, 5)
	assert.Zero(retMA)
	assert.Equal(errWrappedError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhCluster.api = api
	odhCluster.log = l
	retMA, retErr = odhCluster.Count(ctx, params, 0)
	assert.Equal(errWrappedError, retErr)
	assert.Zero(retMA)
}

func TestClusterCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	defer func() { odhCluster.cache = map[string]*Cluster{} }()

	mCCA := &models.ClusterCreateArgs{
		ClusterCreateOnce: models.ClusterCreateOnce{
			AccountID:   "aid1",
			ClusterType: "Kubernetes",
			CspDomainID: "csp-domain-id",
		},
		ClusterCreateMutable: models.ClusterCreateMutable{
			Name:               "testcluster",
			Description:        "test description",
			Tags:               []string{"tag1", "tag2"},
			AuthorizedAccounts: []models.ObjIDMutable{"aid1", "aid2"},
			ClusterIdentifier:  "cluster-identifier",
			ClusterAttributes: map[string]models.ValueType{
				"attr1": models.ValueType{Kind: "INT", Value: "1"},
				"attr2": models.ValueType{Kind: "STRING", Value: "a string"},
			},
		},
	}
	mC := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{ObjType: "Cluster"},
		},
		ClusterCreateOnce: mCCA.ClusterCreateOnce,
		ClusterMutable: models.ClusterMutable{
			ClusterCreateMutable: mCCA.ClusterCreateMutable,
		},
	}
	dCluster := &Cluster{}
	dCluster.FromModel(mC) // initialize empty properties
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().InsertOne(ctx, odhCluster, newClusterMatcher(t, mockClusterInsert, dCluster)).Return(nil)
	odhCluster.crud = crud
	odhCluster.api = api
	odhCluster.log = l
	retMA, retErr := odhCluster.Create(ctx, mCCA)
	assert.NoError(retErr)
	assert.NotZero(retMA.Meta.ID)
	assert.Equal(models.ObjVersion(1), retMA.Meta.Version)
	cCluster, ok := odhCluster.cache[dCluster.MetaObjID]
	if assert.True(ok) {
		assert.Equal(cCluster, dCluster)
	}
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: duplicate")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().InsertOne(ctx, odhCluster, newClusterMatcher(t, mockClusterInsert, dCluster)).Return(ds.ErrorExists)
	odhCluster.crud = crud
	odhCluster.api = api
	odhCluster.log = l
	retMA, retErr = odhCluster.Create(ctx, mCCA)
	assert.Equal(ds.ErrorExists, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhCluster.api = api
	odhCluster.log = l
	retMA, retErr = odhCluster.Create(ctx, mCCA)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestClusterDelete(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	mID := "c72bcfb7-90d4-4276-89ef-aa4bacd09ec7"
	defer func() { odhCluster.cache = map[string]*Cluster{} }()
	odhCluster.cache[mID] = &Cluster{}
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().DeleteOne(ctx, odhCluster, mID).Return(nil)
	odhCluster.crud = crud
	odhCluster.api = api
	odhCluster.log = l
	retErr := odhCluster.Delete(ctx, mID)
	assert.NoError(retErr)
	_, ok := odhCluster.cache[mID]
	assert.False(ok)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().DeleteOne(ctx, odhCluster, mID).Return(errUnknownError)
	odhCluster.crud = crud
	odhCluster.api = api
	odhCluster.log = l
	retErr = odhCluster.Delete(ctx, mID)
	assert.Equal(errUnknownError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhCluster.api = api
	odhCluster.log = l
	retErr = odhCluster.Delete(ctx, mID)
	assert.Equal(errWrappedError, retErr)
}

func TestClusterFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	odhCluster.cache = map[string]*Cluster{}
	defer func() { odhCluster.cache = map[string]*Cluster{} }()

	mTime := time.Now()
	cTime := mTime.AddDate(0, 0, -1)
	dCluster := &Cluster{
		ObjMeta: ObjMeta{
			MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
			MetaTimeCreated:  cTime,
			MetaTimeModified: mTime,
		},
		Name:        "testcluster",
		Description: "test description",
		Tags:        []string{"tag1", "tag2"},
		ClusterAttributes: StringValueMap{
			"attr1": ValueType{Kind: "INT", Value: "1"},
			"attr2": ValueType{Kind: "STRING", Value: "a string"},
		},
		ClusterVersion: "1.7",
		Service: NuvoService{
			ServiceType:    "Clusterd",
			ServiceVersion: "1.1.1",
			ServiceState: ServiceState{
				HeartbeatPeriodSecs: 90,
				HeartbeatTime:       mTime,
				State:               "UNKNOWN",
			},
			ServiceAttributes: StringValueMap{
				"attr1": ValueType{Kind: "INT", Value: "1"},
				"attr2": ValueType{Kind: "STRING", Value: "a string"},
			},
			Messages: []TimestampedString{
				TimestampedString{Message: "message1", Time: mTime},
				TimestampedString{Message: "message2", Time: mTime},
				TimestampedString{Message: "message3", Time: mTime},
			},
		},
		ClusterIdentifier: "myCluster",
		ClusterType:       "Kubernetes",
		CspDomainID:       "cspDomainId",
	}
	fArg := bson.M{objKey: dCluster.ObjMeta.MetaObjID}
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhCluster, fArg, newClusterMatcher(t, mockClusterFind, dCluster)).Return(nil)
	odhCluster.crud = crud
	odhCluster.api = api
	odhCluster.log = l
	retMA, retErr := odhCluster.Fetch(ctx, dCluster.ObjMeta.MetaObjID)
	assert.NoError(retErr)
	assert.Equal(dCluster.ToModel(), retMA)
	cCluster, ok := odhCluster.cache[dCluster.MetaObjID]
	if assert.True(ok) {
		assert.Equal(cCluster, dCluster)
	}
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: cached")
	mockCtrl = gomock.NewController(t)
	api.EXPECT().MustBeReady().Return(nil)
	odhCluster.crud = nil
	odhCluster.api = api
	retMA, retErr = odhCluster.Fetch(ctx, dCluster.ObjMeta.MetaObjID)
	assert.NoError(retErr)
	assert.Equal(dCluster.ToModel(), retMA)
	delete(odhCluster.cache, dCluster.ObjMeta.MetaObjID)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not found")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhCluster, fArg, newClusterMatcher(t, mockClusterFind, dCluster)).Return(ds.ErrorNotFound)
	odhCluster.crud = crud
	odhCluster.api = api
	odhCluster.log = l
	retMA, retErr = odhCluster.Fetch(ctx, dCluster.ObjMeta.MetaObjID)
	assert.Equal(ds.ErrorNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhCluster.api = api
	odhCluster.log = l
	retMA, retErr = odhCluster.Fetch(ctx, dCluster.ObjMeta.MetaObjID)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestClusterList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	defer func() { odhCluster.cache = map[string]*Cluster{} }()

	now := time.Now()
	dClusters := []interface{}{
		&Cluster{
			ObjMeta: ObjMeta{
				MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
				MetaTimeCreated:  now.AddDate(-1, -2, -1),
				MetaTimeModified: now.AddDate(-1, 0, 0),
			},
			Name:        "cluster1",
			Description: "cluster1 description",
			Tags:        []string{"tag1", "tag2"},
		},
		&Cluster{
			ObjMeta: ObjMeta{
				MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e1",
				MetaTimeCreated:  now.AddDate(0, -2, -1),
				MetaTimeModified: now.AddDate(0, -1, -2),
			},
			Name:        "cluster2",
			Description: "cluster2 description",
		},
	}
	ctx := context.Background()

	t.Log("case: success")
	params := cluster.ClusterListParams{
		Name: swag.String("oid1"),
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
	crud.EXPECT().FindAll(ctx, odhCluster, params, fArg, newConsumeObjFnMatcher(t, dClusters...)).Return(nil)
	odhCluster.crud = crud
	odhCluster.api = api
	odhCluster.log = l
	retMA, retErr := odhCluster.List(ctx, params)
	assert.NoError(retErr)
	assert.Len(retMA, len(dClusters))
	for i, mA := range retMA {
		o := dClusters[i].(*Cluster)
		assert.Equal(o.ToModel(), mA)
	}
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	params = cluster.ClusterListParams{
		AccountID:           swag.String("aid1"),
		AuthorizedAccountID: swag.String("aid1"),
		ClusterIdentifier:   swag.String("cluster id"),
		ClusterType:         swag.String("cluster type"),
		ClusterVersion:      swag.String("cluster version"),
		CspDomainID:         swag.String("csp domain ID"),
	}
	fArg = bson.M{
		"accountid":          *params.AccountID,
		"authorizedaccounts": *params.AuthorizedAccountID,
		"clusteridentifier":  *params.ClusterIdentifier,
		"clustertype":        *params.ClusterType,
		"clusterversion":     *params.ClusterVersion,
		"cspdomainid":        *params.CspDomainID,
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhCluster, params, fArg, gomock.Any()).Return(errWrappedError)
	odhCluster.crud = crud
	odhCluster.api = api
	odhCluster.log = l
	retMA, retErr = odhCluster.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	// case: more query combinations
	tGE := time.Now().Add(-time.Minute)
	hbtGE := strfmt.DateTime(tGE)
	tLE := time.Now().Add(-time.Hour)
	hbtLE := strfmt.DateTime(tLE)
	qTcs := []struct {
		p cluster.ClusterListParams
		q bson.M
	}{
		{ // [0]
			cluster.ClusterListParams{ServiceHeartbeatTimeGE: &hbtGE, ServiceHeartbeatTimeLE: &hbtLE},
			bson.M{"$and": []bson.M{
				bson.M{"service.heartbeattime": bson.M{"$gte": tGE}},
				bson.M{"service.heartbeattime": bson.M{"$lte": tLE}}},
			},
		},
		{ // [1]
			cluster.ClusterListParams{ServiceHeartbeatTimeGE: &hbtGE},
			bson.M{"service.heartbeattime": bson.M{"$gte": tGE}},
		},
		{ // [2]
			cluster.ClusterListParams{ServiceHeartbeatTimeLE: &hbtLE},
			bson.M{"service.heartbeattime": bson.M{"$lte": tLE}},
		},
		{ // [3]
			cluster.ClusterListParams{ServiceStateEQ: swag.String("READY")},
			bson.M{"service.state": "READY"},
		},
		{ // [4]
			cluster.ClusterListParams{ServiceStateNE: swag.String("READY")},
			bson.M{"service.state": bson.M{"$ne": "READY"}},
		},
		{ // [5]
			cluster.ClusterListParams{ServiceStateEQ: swag.String("READY"), ServiceStateNE: swag.String("READY")},
			bson.M{"service.state": bson.M{"$ne": "READY"}},
		},
		{ // [6]
			cluster.ClusterListParams{StateEQ: swag.String("ACTIVE")},
			bson.M{"state": "ACTIVE"},
		},
		{ // [7]
			cluster.ClusterListParams{StateNE: swag.String("ACTIVE")},
			bson.M{"state": bson.M{"$ne": "ACTIVE"}},
		},
		{ // [8]
			cluster.ClusterListParams{StateEQ: swag.String("ACTIVE"), StateNE: swag.String("ACTIVE")},
			bson.M{"state": bson.M{"$ne": "ACTIVE"}},
		},
		{ // [9]
			cluster.ClusterListParams{
				ClusterType:    swag.String("cluster type"),
				ClusterVersion: swag.String("cluster version"),
				CspDomainID:    swag.String("csp domain ID"),
			},
			bson.M{
				"clustertype":    *params.ClusterType,
				"clusterversion": *params.ClusterVersion,
				"cspdomainid":    *params.CspDomainID,
			},
		},
	}
	for i, tc := range qTcs {
		q := odhCluster.convertListParams(tc.p)
		assert.Equal(tc.q, q, "Query[%d]", i)
	}

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhCluster.api = api
	odhCluster.log = l
	retMA, retErr = odhCluster.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestClusterUpdate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	odhCluster.cache = map[string]*Cluster{}
	defer func() { odhCluster.cache = map[string]*Cluster{} }()

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
				Name: "Service",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	param := &models.ClusterMutable{
		ClusterCreateMutable: models.ClusterCreateMutable{
			Name: "newName",
		},
		ClusterMutableAllOf0: models.ClusterMutableAllOf0{
			Service: &models.NuvoService{
				NuvoServiceAllOf0: models.NuvoServiceAllOf0{
					ServiceType:       "Clusterd",
					ServiceVersion:    "1.1.1",
					ServiceAttributes: map[string]models.ValueType{},
					Messages:          []*models.TimestampedString{},
				},
				ServiceState: models.ServiceState{
					HeartbeatPeriodSecs: 90,
					State:               "UNKNOWN",
				},
			},
		},
	}
	mObj := &models.Cluster{ClusterMutable: *param}
	dObj := &Cluster{}
	dObj.FromModel(mObj)
	now := time.Now()
	dCluster := &Cluster{
		ObjMeta: ObjMeta{
			MetaObjID:        ua.ID,
			MetaTimeCreated:  now.AddDate(0, -2, -1),
			MetaTimeModified: now,
			MetaVersion:      1000,
		},
	}
	m := clusterMatcher(t, mockClusterUpdate).CtxObj(dObj).Return(dCluster).Matcher()
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhCluster, m, ua).Return(nil)
	odhCluster.crud = crud
	odhCluster.api = api
	odhCluster.log = l
	retMA, retErr := odhCluster.Update(ctx, ua, param)
	assert.NoError(retErr)
	assert.Equal(dCluster.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhCluster, m, ua).Return(ds.ErrorIDVerNotFound)
	odhCluster.crud = crud
	odhCluster.api = api
	odhCluster.log = l
	retMA, retErr = odhCluster.Update(ctx, ua, param)
	assert.Equal(ds.ErrorIDVerNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhCluster.api = api
	odhCluster.log = l
	retMA, retErr = odhCluster.Update(ctx, ua, param)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}
