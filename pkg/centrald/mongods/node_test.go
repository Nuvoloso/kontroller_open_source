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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/node"
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

// gomock.Matcher for Node
type mockNodeMatchCtx int

const (
	mockNodeInvalid mockNodeMatchCtx = iota
	mockNodeInsert
	mockNodeFind
	mockNodeUpdate
)

type mockNodeMatcher struct {
	t      *testing.T
	ctx    mockNodeMatchCtx
	ctxObj *Node
	retObj *Node
}

func newNodeMatcher(t *testing.T, ctx mockNodeMatchCtx, obj *Node) gomock.Matcher {
	return nodeMatcher(t, ctx).CtxObj(obj).Matcher()
}

// nodeMatcher creates a partially initialized mockNodeMatcher
func nodeMatcher(t *testing.T, ctx mockNodeMatchCtx) *mockNodeMatcher {
	return &mockNodeMatcher{t: t, ctx: ctx}
}

// Matcher ends the chain
func (o *mockNodeMatcher) Matcher() gomock.Matcher {
	if o.ctx == mockNodeInsert || o.ctx == mockNodeFind || o.ctx == mockNodeUpdate {
		assert.NotNil(o.t, o.ctxObj)
	}
	return o
}

// CtxObj adds an Node object
func (o *mockNodeMatcher) CtxObj(ao *Node) *mockNodeMatcher {
	o.ctxObj = ao
	return o
}

// Return sets the update result object
func (o *mockNodeMatcher) Return(ro *Node) *mockNodeMatcher {
	o.retObj = ro
	return o
}

// Matches is from gomock.Matcher
func (o *mockNodeMatcher) Matches(x interface{}) bool {
	var obj *Node
	switch z := x.(type) {
	case *Node:
		obj = z
	default:
		panic("Did not get a recognizable object: " + reflect.TypeOf(x).String())
	}
	assert := assert.New(o.t)
	switch o.ctx {
	case mockNodeInsert:
		compObj := o.ctxObj
		compObj.ObjMeta = obj.ObjMeta // insert sets meta data
		return assert.NotZero(obj.ObjMeta.MetaObjID) &&
			assert.Equal(int32(1), obj.ObjMeta.MetaVersion) &&
			assert.False(obj.ObjMeta.MetaTimeCreated.IsZero()) &&
			assert.False(obj.ObjMeta.MetaTimeModified.IsZero()) &&
			assert.Equal(obj.ObjMeta.MetaTimeCreated, obj.ObjMeta.MetaTimeModified) &&
			assert.Equal(compObj, obj)
	case mockNodeFind:
		*obj = *o.ctxObj
		return true
	case mockNodeUpdate:
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
func (o *mockNodeMatcher) String() string {
	switch o.ctx {
	case mockNodeInsert:
		return "matches on insert"
	}
	return "unknown context"
}

func TestNodeHandler(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	assert.Equal("node", odhNode.CName())
	indexes := odhNode.Indexes()
	expIndexes := []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: clusterIDKey, Value: IndexAscending}, {Key: nameKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: clusterIDKey, Value: IndexAscending}, {Key: nodeIdentifierKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
	}
	assert.Equal(expIndexes, indexes)
	assert.Equal(indexes, odhNode.indexes)
	dIf := odhNode.NewObject()
	assert.NotNil(dIf)
	_, ok := dIf.(*Node)
	assert.True(ok)

	t.Log("case: Claim + Initialize success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ctx := context.Background()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	odhNode.Claim(api, crud)
	assert.Equal(api, odhNode.api)
	assert.Equal(crud, odhNode.crud)
	assert.Equal(l, odhNode.log)
	assert.Equal(odhNode, odhNode.Ops())
	crud.EXPECT().CreateIndexes(ctx, odhNode).Return(nil)
	assert.NoError(odhNode.Initialize(ctx))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize, error creating indexes")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhNode).Return(errUnknownError)
	odhNode.crud = crud
	odhNode.api = api
	assert.Error(errUnknownError, odhNode.Initialize(ctx))

	t.Log("case: Start")
	api.EXPECT().DBName().Return("dbName")
	assert.NoError(odhNode.Start(ctx))
}

func TestNodeCount(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	t.Log("case: success")
	ctx := context.Background()
	params := node.NodeListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhNode, fArg, uint(0)).Return(2, nil)
	odhNode.crud = crud
	odhNode.api = api
	odhNode.log = l
	retMA, retErr := odhNode.Count(ctx, params, 0)
	assert.Equal(2, retMA)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: count fails")
	mockCtrl = gomock.NewController(t)
	params = node.NodeListParams{
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
	crud.EXPECT().Count(ctx, odhNode, fArg, uint(5)).Return(0, errWrappedError)
	odhNode.crud = crud
	odhNode.api = api
	odhNode.log = l
	retMA, retErr = odhNode.Count(ctx, params, 5)
	assert.Zero(retMA)
	assert.Equal(errWrappedError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhNode.api = api
	odhNode.log = l
	retMA, retErr = odhNode.Count(ctx, params, 0)
	assert.Equal(errWrappedError, retErr)
	assert.Zero(retMA)
}

func TestNodeCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	cTime := time.Now()
	dNode := &Node{
		AccountID:      "aid1",
		ClusterID:      "clusterID",
		Name:           "testnode",
		Description:    "test description",
		NodeIdentifier: "127.0.0.1",
		Service: NuvoService{
			ServiceType:    "nvagentd",
			ServiceVersion: "0.1.0",
			ServiceState: ServiceState{
				HeartbeatPeriodSecs: 90,
				HeartbeatTime:       cTime,
				State:               "STARTING",
			},
			ServiceAttributes: StringValueMap{
				"attr1": ValueType{Kind: "INT", Value: "1"},
				"attr2": ValueType{Kind: "STRING", Value: "a string"},
			},
			Messages: []TimestampedString{
				TimestampedString{Message: "message1", Time: cTime},
				TimestampedString{Message: "message2", Time: cTime},
				TimestampedString{Message: "message3", Time: cTime},
			},
		},
		State: "MANAGED",
		Tags:  []string{"tag1", "tag2"},
	}
	mNode := dNode.ToModel()
	mNode.Meta = &models.ObjMeta{}
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().InsertOne(ctx, odhNode, newNodeMatcher(t, mockNodeInsert, dNode)).Return(nil)
	odhNode.crud = crud
	odhNode.api = api
	odhNode.log = l
	retMA, retErr := odhNode.Create(ctx, mNode)
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
	crud.EXPECT().InsertOne(ctx, odhNode, newNodeMatcher(t, mockNodeInsert, dNode)).Return(ds.ErrorExists)
	odhNode.crud = crud
	odhNode.api = api
	odhNode.log = l
	retMA, retErr = odhNode.Create(ctx, mNode)
	assert.Equal(ds.ErrorExists, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhNode.api = api
	odhNode.log = l
	retMA, retErr = odhNode.Create(ctx, mNode)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestNodeDelete(t *testing.T) {
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
	crud.EXPECT().DeleteOne(ctx, odhNode, mID).Return(nil)
	odhNode.crud = crud
	odhNode.api = api
	odhNode.log = l
	retErr := odhNode.Delete(ctx, mID)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().DeleteOne(ctx, odhNode, mID).Return(errUnknownError)
	odhNode.crud = crud
	odhNode.api = api
	odhNode.log = l
	retErr = odhNode.Delete(ctx, mID)
	assert.Equal(errUnknownError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhNode.api = api
	odhNode.log = l
	retErr = odhNode.Delete(ctx, mID)
	assert.Equal(errWrappedError, retErr)
}

func TestNodeFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mTime := time.Now()
	cTime := mTime.AddDate(0, 0, -1)
	dNode := &Node{
		ObjMeta: ObjMeta{
			MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
			MetaTimeCreated:  cTime,
			MetaTimeModified: mTime,
		},
		AccountID:      "aid1",
		ClusterID:      "clusterID",
		Name:           "testnode",
		Description:    "test description",
		NodeIdentifier: "127.0.0.1",
		Service: NuvoService{
			ServiceType:    "nvagentd",
			ServiceVersion: "0.1.0",
			ServiceState: ServiceState{
				HeartbeatPeriodSecs: 90,
				HeartbeatTime:       mTime,
				State:               "READY",
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
		State: "TIMED_OUT",
		Tags:  []string{"tag1", "tag2"},
	}
	fArg := bson.M{objKey: dNode.ObjMeta.MetaObjID}
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhNode, fArg, newNodeMatcher(t, mockNodeFind, dNode)).Return(nil)
	odhNode.crud = crud
	odhNode.api = api
	odhNode.log = l
	retMA, retErr := odhNode.Fetch(ctx, dNode.ObjMeta.MetaObjID)
	assert.NoError(retErr)
	assert.Equal(dNode.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not found")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhNode, fArg, newNodeMatcher(t, mockNodeFind, dNode)).Return(ds.ErrorNotFound)
	odhNode.crud = crud
	odhNode.api = api
	odhNode.log = l
	retMA, retErr = odhNode.Fetch(ctx, dNode.ObjMeta.MetaObjID)
	assert.Equal(ds.ErrorNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhNode.api = api
	odhNode.log = l
	retMA, retErr = odhNode.Fetch(ctx, dNode.ObjMeta.MetaObjID)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestNodeList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	now := time.Now()
	mTime1 := now.AddDate(-1, 0, 0)
	mTime2 := now.AddDate(0, -1, -2)
	dNodes := []interface{}{
		&Node{
			ObjMeta: ObjMeta{
				MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
				MetaTimeCreated:  now.AddDate(-1, -2, -1),
				MetaTimeModified: mTime1,
			},
			AccountID:      "aid1",
			ClusterID:      "clusterID",
			Name:           "node1",
			Description:    "node1 description",
			NodeIdentifier: "127.0.0.1",
			Service: NuvoService{
				ServiceType:    "nvagentd",
				ServiceVersion: "0.1.0",
				ServiceState: ServiceState{
					HeartbeatPeriodSecs: 90,
					HeartbeatTime:       mTime1,
					State:               "READY",
				},
				ServiceAttributes: StringValueMap{
					"attr1": ValueType{Kind: "INT", Value: "1"},
					"attr2": ValueType{Kind: "STRING", Value: "a string"},
				},
				Messages: []TimestampedString{
					TimestampedString{Message: "message1", Time: mTime1},
					TimestampedString{Message: "message2", Time: mTime1},
					TimestampedString{Message: "message3", Time: mTime1},
				},
			},
			Tags: []string{"tag1", "tag2"},
		},
		&Node{
			ObjMeta: ObjMeta{
				MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e1",
				MetaTimeCreated:  now.AddDate(0, -2, -1),
				MetaTimeModified: mTime2,
			},
			AccountID:      "aid1",
			ClusterID:      "clusterID",
			Name:           "node2",
			Description:    "node2 description",
			NodeIdentifier: "127.0.0.2",
			Service: NuvoService{
				ServiceType:    "nvagentd",
				ServiceVersion: "0.1.0",
				ServiceState: ServiceState{
					HeartbeatPeriodSecs: 90,
					HeartbeatTime:       mTime2,
					State:               "READY",
				},
				ServiceAttributes: StringValueMap{
					"attr1": ValueType{Kind: "INT", Value: "1"},
					"attr2": ValueType{Kind: "STRING", Value: "a string"},
				},
				Messages: []TimestampedString{
					TimestampedString{Message: "message1", Time: mTime2},
					TimestampedString{Message: "message2", Time: mTime2},
					TimestampedString{Message: "message3", Time: mTime2},
				},
			},
		},
	}
	ctx := context.Background()

	t.Log("case: success")
	params := node.NodeListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhNode, params, fArg, newConsumeObjFnMatcher(t, dNodes...)).Return(nil)
	odhNode.crud = crud
	odhNode.api = api
	odhNode.log = l
	retMA, retErr := odhNode.List(ctx, params)
	assert.NoError(retErr)
	assert.Len(retMA, len(dNodes))
	for i, mA := range retMA {
		o := dNodes[i].(*Node)
		assert.Equal(o.ToModel(), mA)
	}
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	params = node.NodeListParams{
		Name:           swag.String("node1"),
		ClusterID:      swag.String("clusterID"),
		NodeIdentifier: swag.String("127.0.0.1"),
		Tags:           []string{"tag1", "tag2"},
	}
	fArg = bson.M{
		"name":           *params.Name,
		"clusterid":      *params.ClusterID,
		"nodeidentifier": *params.NodeIdentifier,
		"tags":           bson.M{"$all": params.Tags},
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhNode, params, fArg, gomock.Any()).Return(errWrappedError)
	odhNode.crud = crud
	odhNode.api = api
	odhNode.log = l
	retMA, retErr = odhNode.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	// case: more query combinations
	tGE := time.Now().Add(-time.Minute)
	hbtGE := strfmt.DateTime(tGE)
	tLE := time.Now().Add(-time.Hour)
	hbtLE := strfmt.DateTime(tLE)
	nodeIDs := []string{"nid1", "nid2"}
	qTcs := []struct {
		p node.NodeListParams
		q bson.M
	}{
		{ // [0]
			node.NodeListParams{ServiceHeartbeatTimeGE: &hbtGE, ServiceHeartbeatTimeLE: &hbtLE},
			bson.M{"$and": []bson.M{
				bson.M{"service.heartbeattime": bson.M{"$gte": tGE}},
				bson.M{"service.heartbeattime": bson.M{"$lte": tLE}}},
			},
		},
		{ // [1]
			node.NodeListParams{ServiceHeartbeatTimeGE: &hbtGE},
			bson.M{"service.heartbeattime": bson.M{"$gte": tGE}},
		},
		{ // [2]
			node.NodeListParams{ServiceHeartbeatTimeLE: &hbtLE},
			bson.M{"service.heartbeattime": bson.M{"$lte": tLE}},
		},
		{ // [3]
			node.NodeListParams{ServiceStateEQ: swag.String("READY")},
			bson.M{"service.state": "READY"},
		},
		{ // [4]
			node.NodeListParams{ServiceStateNE: swag.String("READY")},
			bson.M{"service.state": bson.M{"$ne": "READY"}},
		},
		{ // [5]
			node.NodeListParams{ServiceStateEQ: swag.String("READY"), ServiceStateNE: swag.String("READY")},
			bson.M{"service.state": bson.M{"$ne": "READY"}},
		},
		{ // [6]
			node.NodeListParams{NodeIds: nodeIDs},
			bson.M{"MetaID": bson.M{"$in": nodeIDs}},
		},
		{ // [7]
			node.NodeListParams{StateEQ: swag.String("ACTIVE")},
			bson.M{"state": "ACTIVE"},
		},
		{ // [8]
			node.NodeListParams{StateNE: swag.String("ACTIVE")},
			bson.M{"state": bson.M{"$ne": "ACTIVE"}},
		},
		{ // [9]
			node.NodeListParams{StateEQ: swag.String("ACTIVE"), StateNE: swag.String("ACTIVE")},
			bson.M{"state": bson.M{"$ne": "ACTIVE"}},
		},
	}
	for i, tc := range qTcs {
		q := odhNode.convertListParams(tc.p)
		assert.Equal(tc.q, q, "Query[%d]", i)
	}

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhNode.api = api
	odhNode.log = l
	retMA, retErr = odhNode.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestNodeUpdate(t *testing.T) {
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
			}, {
				Name: "LocalStorage",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	param := &models.NodeMutable{
		Name: "newName",
		LocalStorage: map[string]models.NodeStorageDevice{
			"uuid1": models.NodeStorageDevice{
				DeviceName:      "d1",
				DeviceState:     "s1",
				DeviceType:      "t1",
				SizeBytes:       swag.Int64(11),
				UsableSizeBytes: swag.Int64(10)},
		},
	}
	mObj := &models.Node{NodeMutable: *param}
	dObj := &Node{}
	dObj.FromModel(mObj)
	now := time.Now()
	dNode := &Node{
		ObjMeta: ObjMeta{
			MetaObjID:        ua.ID,
			MetaTimeCreated:  now.AddDate(0, -2, -1),
			MetaTimeModified: now,
			MetaVersion:      1000,
		},
	}
	m := nodeMatcher(t, mockNodeUpdate).CtxObj(dObj).Return(dNode).Matcher()
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhNode, m, ua).Return(nil)
	odhNode.crud = crud
	odhNode.api = api
	odhNode.log = l
	retMA, retErr := odhNode.Update(ctx, ua, param)
	assert.NoError(retErr)
	assert.Equal(dNode.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhNode, m, ua).Return(ds.ErrorIDVerNotFound)
	odhNode.crud = crud
	odhNode.api = api
	odhNode.log = l
	retMA, retErr = odhNode.Update(ctx, ua, param)
	assert.Equal(ds.ErrorIDVerNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhNode.api = api
	odhNode.log = l
	retMA, retErr = odhNode.Update(ctx, ua, param)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestNodeUpdateAll(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	odhNode.log = l

	now := time.Now()
	hbt := now
	lParam := node.NodeListParams{
		NodeIds: []string{"NODE-1", "NODE-2"},
	}
	findArgs := bson.M{"MetaID": bson.M{"$in": lParam.NodeIds}}
	ua := &ds.UpdateArgs{
		Attributes: []ds.UpdateAttr{
			{
				Name: "Service",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						Fields: map[string]struct{}{
							"State": struct{}{},
						},
					},
				},
			}, {
				Name: "Service",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						Fields: map[string]struct{}{
							"HeartbeatPeriodSecs": struct{}{},
						},
					},
				},
			}, {
				Name: "Service",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						Fields: map[string]struct{}{
							"HeartbeatTime": struct{}{},
						},
					},
				},
			}, {
				Name: "State",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	uParam := &models.NodeMutable{
		Service: &models.NuvoService{
			ServiceState: models.ServiceState{
				HeartbeatPeriodSecs: 90,
				HeartbeatTime:       strfmt.DateTime(hbt),
				State:               "READY",
			},
		},
		State: "TEAR_DOWN",
	}
	mObj := &models.Node{NodeMutable: *uParam}
	dObj := &Node{}
	dObj.FromModel(mObj)
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateAll(ctx, odhNode, findArgs, newNodeMatcher(t, mockNodeInsert, dObj), ua).Return(3, 4, nil)
	odhNode.crud = crud
	odhNode.api = api
	mod, matched, retErr := odhNode.UpdateMultiple(ctx, lParam, ua, uParam)
	assert.NoError(retErr)
	assert.Equal(3, mod)
	assert.Equal(4, matched)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateAll(ctx, odhNode, findArgs, newNodeMatcher(t, mockNodeInsert, dObj), ua).Return(0, 0, ds.ErrorDbError)
	odhNode.crud = crud
	odhNode.api = api
	mod, matched, retErr = odhNode.UpdateMultiple(ctx, lParam, ua, uParam)
	assert.Equal(ds.ErrorDbError, retErr)
	assert.Zero(mod)
	assert.Zero(matched)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhNode.api = api
	mod, matched, retErr = odhNode.UpdateMultiple(ctx, lParam, ua, uParam)
	assert.Equal(errWrappedError, retErr)
	assert.Zero(mod)
	assert.Zero(matched)
}
