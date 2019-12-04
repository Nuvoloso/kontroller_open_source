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
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/mongodb"
	"github.com/Nuvoloso/kontroller/pkg/mongodb/mock"
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

type fakeODH struct {
	protoObj interface{} // prototype object (itself, not a pointer). see NewObject below
	indexes  []mongo.IndexModel
}

var _ = ObjectDocumentHandler(&fakeODH{})

func (h *fakeODH) Ops() interface{} {
	return nil
}

func (h *fakeODH) Claim(DBAPI, ObjectDocumentHandlerCRUD) {
}

func (h *fakeODH) Initialize(context.Context) error {
	return nil
}

func (h *fakeODH) Start(context.Context) error {
	return nil
}

func (h *fakeODH) CName() string {
	return "testCol"
}

func (h *fakeODH) Indexes() []mongo.IndexModel {
	return h.indexes
}
func (h *fakeODH) NewObject() interface{} {

	if h.protoObj != nil {
		return reflect.New(reflect.TypeOf(h.protoObj)).Interface()
	}
	return h.protoObj
}

func TestODH(t *testing.T) {
	assert := assert.New(t)

	h := &fakeODH{}
	var names = []string{"name1", "name2", "name3"}
	for _, n := range names {
		assert.NotPanics(func() { odhRegister(n, h) })
		assert.Panics(func() { odhRegister(n, h) })
	}
	// Note: the test runs the init functions in the package so the registry contains the real handlers in addition!
	for _, n := range names {
		odh, ok := odhRegistry[n]
		assert.True(ok)
		assert.Equal(h, odh)
	}
}

func TestAggregate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	dbName := "testDB"
	testCol := "testCol"

	h := &fakeODH{}
	ctx := context.Background()

	t.Log("case: success")
	params := storage.StorageListParams{Sum: []string{"sizeBytes", "availableBytes"}}
	filter := bson.M{"name": "value"}
	aggArgs := []bson.M{
		bson.M{
			"$match": bson.M{"name": "value"},
		},
		bson.M{
			"$group": bson.M{
				"_id":   nil,
				"count": bson.M{"$sum": 1},
				"sum0":  bson.M{"$sum": "$sizebytes"},
				"sum1":  bson.M{"$sum": "$availablebytes"},
			},
		},
	}
	aggRes := bson.M{
		"count": 5,
		"sum0":  int64(1010101010101),
		"sum1":  int64(101010101010),
	}
	exp := []*ds.Aggregation{
		&ds.Aggregation{FieldPath: "sizeBytes", Type: "sum", Value: 1010101010101},
		&ds.Aggregation{FieldPath: "availableBytes", Type: "sum", Value: 101010101010},
	}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	api.EXPECT().DBName().Return(dbName).MinTimes(1)
	cl := mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(cl)
	db := mock.NewMockDatabase(mockCtrl)
	cl.EXPECT().Database(dbName).Return(db)
	c := mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection(testCol).Return(c)
	cur := mock.NewMockCursor(mockCtrl)
	c.EXPECT().Aggregate(ctx, aggArgs).Return(cur, nil)
	cur.EXPECT().Next(ctx).Return(true)
	cur.EXPECT().Decode(newMockDecodeMatcher(t, &aggRes)).Return(nil)
	cur.EXPECT().Err().Return(nil)
	cur.EXPECT().Close(ctx).Return(nil)
	api.EXPECT().ErrorCode(nil).Return(mongodb.ECKeyNotFound)
	crud := NewObjectDocumentHandlerCRUD(api)
	rl, n, err := crud.Aggregate(ctx, h, &params, filter)
	assert.NoError(err)
	assert.Equal(exp, rl)
	assert.Equal(5, n)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: no results")
	exp = []*ds.Aggregation{
		&ds.Aggregation{FieldPath: "sizeBytes", Type: "sum"},
		&ds.Aggregation{FieldPath: "availableBytes", Type: "sum"},
	}
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return(dbName).MinTimes(1)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	cl = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(cl)
	db = mock.NewMockDatabase(mockCtrl)
	cl.EXPECT().Database(dbName).Return(db)
	c = mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection(testCol).Return(c)
	cur = mock.NewMockCursor(mockCtrl)
	c.EXPECT().Aggregate(ctx, aggArgs).Return(cur, nil)
	cur.EXPECT().Next(ctx).Return(false)
	cur.EXPECT().Err().Return(nil)
	cur.EXPECT().Close(ctx).Return(nil)
	api.EXPECT().ErrorCode(nil).Return(mongodb.ECKeyNotFound)
	crud = NewObjectDocumentHandlerCRUD(api)
	rl, n, err = crud.Aggregate(ctx, h, &params, filter)
	assert.NoError(err)
	assert.Equal(exp, rl)
	assert.Zero(n)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Aggregate failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return(dbName).MinTimes(1)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	cl = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(cl)
	db = mock.NewMockDatabase(mockCtrl)
	cl.EXPECT().Database(dbName).Return(db)
	c = mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection(testCol).Return(c)
	c.EXPECT().Aggregate(ctx, aggArgs).Return(nil, errUnknownError)
	api.EXPECT().WrapError(errUnknownError, false).Return(errWrappedError)
	crud = NewObjectDocumentHandlerCRUD(api)
	rl, n, err = crud.Aggregate(ctx, h, &params, filter)
	assert.Equal(errWrappedError, err)
	assert.Nil(rl)
	assert.Zero(n)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Decode fails")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return(dbName).MinTimes(1)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	cl = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(cl)
	db = mock.NewMockDatabase(mockCtrl)
	cl.EXPECT().Database(dbName).Return(db)
	c = mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection(testCol).Return(c)
	cur = mock.NewMockCursor(mockCtrl)
	c.EXPECT().Aggregate(ctx, aggArgs).Return(cur, nil)
	cur.EXPECT().Next(ctx).Return(true)
	cur.EXPECT().Decode(newMockDecodeMatcher(t, &aggRes)).Return(errUnknownError)
	cur.EXPECT().Close(ctx).Return(nil)
	api.EXPECT().WrapError(errUnknownError, false).Return(errWrappedError)
	crud = NewObjectDocumentHandlerCRUD(api)
	rl, n, err = crud.Aggregate(ctx, h, &params, filter)
	assert.Equal(errWrappedError, err)
	assert.Nil(rl)
	assert.Zero(n)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Err returns error")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return(dbName).MinTimes(1)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	cl = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(cl)
	db = mock.NewMockDatabase(mockCtrl)
	cl.EXPECT().Database(dbName).Return(db)
	c = mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection(testCol).Return(c)
	cur = mock.NewMockCursor(mockCtrl)
	c.EXPECT().Aggregate(ctx, aggArgs).Return(cur, nil)
	cur.EXPECT().Next(ctx).Return(true)
	cur.EXPECT().Decode(newMockDecodeMatcher(t, &aggRes)).Return(nil)
	cur.EXPECT().Err().Return(errUnknownError)
	cur.EXPECT().Close(ctx).Return(nil)
	api.EXPECT().WrapError(errUnknownError, false).Return(errWrappedError)
	crud = NewObjectDocumentHandlerCRUD(api)
	rl, n, err = crud.Aggregate(ctx, h, &params, filter)
	assert.Equal(errWrappedError, err)
	assert.Nil(rl)
	assert.Zero(n)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: createAggregationGroup error")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	crud = NewObjectDocumentHandlerCRUD(api)
	rl, n, err = crud.Aggregate(ctx, h, &storage.StorageListParams{Sum: []string{"sizeBytes:"}}, filter)
	assert.Equal(&centrald.Error{M: centrald.ErrorInvalidData.M + ": invalid aggregate field path: sizeBytes:", C: centrald.ErrorInvalidData.C}, err)
	assert.Nil(rl)
	assert.Zero(n)
}

func TestCount(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	dbName := "testDB"
	testCol := "testCol"

	h := &fakeODH{}
	ctx := context.Background()

	t.Log("case: success")
	filter := bson.M{"name": "value"}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return(dbName).MinTimes(1)
	cl := mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(cl)
	db := mock.NewMockDatabase(mockCtrl)
	cl.EXPECT().Database(dbName).Return(db)
	c := mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection(testCol).Return(c)
	c.EXPECT().CountDocuments(ctx, filter, options.Count().SetLimit(int64(42))).Return(int64(5), nil)
	api.EXPECT().ErrorCode(nil).Return(mongodb.ECKeyNotFound)
	crud := NewObjectDocumentHandlerCRUD(api)
	result, err := crud.Count(ctx, h, filter, 42)
	assert.Equal(5, result)
	assert.NoError(err)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return(dbName).MinTimes(1)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	cl = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(cl)
	db = mock.NewMockDatabase(mockCtrl)
	cl.EXPECT().Database(dbName).Return(db)
	c = mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection(testCol).Return(c)
	c.EXPECT().CountDocuments(ctx, filter, options.Count().SetLimit(int64(42))).Return(int64(0), errUnknownError)
	api.EXPECT().WrapError(errUnknownError, false).Return(errWrappedError)
	crud = NewObjectDocumentHandlerCRUD(api)
	result, err = crud.Count(ctx, h, filter, 42)
	assert.Zero(result)
	assert.Equal(errWrappedError, err)
	assert.Equal(1, tl.CountPattern(testCol+`.*"name":"value"`))
}

func TestDeleteOne(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	dbName := "testDB"
	testCol := "testCol"

	h := &fakeODH{}
	ctx := context.Background()

	t.Log("case: success")
	oid := "oid"
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return(dbName).MinTimes(1)
	cl := mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(cl)
	db := mock.NewMockDatabase(mockCtrl)
	cl.EXPECT().Database(dbName).Return(db)
	c := mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection(testCol).Return(c)
	c.EXPECT().DeleteOne(ctx, bson.M{objKey: oid}).Return(nil, nil)
	api.EXPECT().ErrorCode(nil).Return(mongodb.ECKeyNotFound)
	crud := NewObjectDocumentHandlerCRUD(api)
	assert.NoError(crud.DeleteOne(ctx, h, oid))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return(dbName).MinTimes(1)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	cl = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(cl)
	db = mock.NewMockDatabase(mockCtrl)
	cl.EXPECT().Database(dbName).Return(db)
	c = mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection(testCol).Return(c)
	c.EXPECT().DeleteOne(ctx, bson.M{objKey: oid}).Return(nil, errUnknownError)
	api.EXPECT().WrapError(errUnknownError, false).Return(errWrappedError)
	crud = NewObjectDocumentHandlerCRUD(api)
	err := crud.DeleteOne(ctx, h, oid)
	assert.Equal(errWrappedError, err)
	assert.Equal(1, tl.CountPattern(testCol+".*"+oid))
}

func TestCreateIndexes(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	dbName := "testDB"
	testCol := "testCol"

	trueVal := true
	assert.Equal(&options.IndexOptions{Unique: &trueVal}, options.Index().SetUnique(true))
	assert.Equal(bsonx.Int32(1), IndexAscending)

	h := &fakeODH{}
	sparseIndex := &options.IndexOptions{Sparse: &trueVal}
	eiArg := mongo.IndexModel{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)}
	ex1 := mongo.IndexModel{Keys: bsonx.Doc{{Key: "ex1a", Value: IndexAscending}, {Key: "ex1b", Value: bsonx.Int32(-1)}}}
	ex2 := mongo.IndexModel{Keys: bsonx.Doc{{Key: "ex1a", Value: IndexAscending}}, Options: sparseIndex}
	idxArg := []mongo.IndexModel{eiArg, ex1, ex2}
	ctx := context.Background()

	t.Log("case: no indexes")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeInitializing().Return(nil)
	api.EXPECT().DBName().Return(dbName).MinTimes(1)
	cl := mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(cl)
	db := mock.NewMockDatabase(mockCtrl)
	cl.EXPECT().Database(dbName).Return(db)
	c := mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection(testCol).Return(c)
	c.EXPECT().Indexes().Return(nil)
	crud := NewObjectDocumentHandlerCRUD(api)
	assert.NoError(crud.CreateIndexes(ctx, h))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: indexes, some errors on index CreateOne")
	mockCtrl = gomock.NewController(t)
	h.indexes = idxArg
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	api.EXPECT().MustBeInitializing().Return(nil)
	api.EXPECT().DBName().Return(dbName).MinTimes(1)
	cl = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(cl)
	db = mock.NewMockDatabase(mockCtrl)
	cl.EXPECT().Database(dbName).Return(db)
	c = mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection(testCol).Return(c)
	iv := mock.NewMockIndexView(mockCtrl)
	c.EXPECT().Indexes().Return(iv)
	iv.EXPECT().CreateOne(ctx, eiArg).Return("", errUnknownError)
	iv.EXPECT().CreateOne(ctx, ex1).Return("", errUnknownError)
	iv.EXPECT().CreateOne(ctx, ex2).Return("boo", nil)
	ec1 := api.EXPECT().ErrorCode(errUnknownError).Return(mongodb.ECUnknownError)
	api.EXPECT().ErrorCode(errUnknownError).Return(mongodb.ECUnknownError).After(ec1)
	crud = NewObjectDocumentHandlerCRUD(api)
	assert.NoError(crud.CreateIndexes(ctx, h))
	assert.NotZero(tl.CountPattern(`"\$numberInt":"1"`))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not initializing")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	api.EXPECT().MustBeInitializing().Return(errUnknownError)
	crud = NewObjectDocumentHandlerCRUD(api)
	err := crud.CreateIndexes(ctx, h)
	assert.Equal(errUnknownError, err)
}

func TestFindAll(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	dbName := "testDB"
	testCol := "testCol"

	h := &fakeODH{protoObj: User{}}
	now := time.Now()
	dUsers := []*User{
		&User{
			ObjMeta: ObjMeta{
				MetaObjID:        "id1",
				MetaTimeCreated:  now.AddDate(-1, -2, -1),
				MetaTimeModified: now.AddDate(-1, 0, 0),
			},
			AuthIdentifier: "ann.user@nuvoloso.com",
			Disabled:       false,
			Profile: StringValueMap{
				"userName": ValueType{Kind: "STRING", Value: "ann user"},
				"attr2":    ValueType{Kind: "INT", Value: "1"},
			},
		},
		&User{
			ObjMeta: ObjMeta{
				MetaObjID:        "id2",
				MetaTimeCreated:  now.AddDate(0, -2, -1),
				MetaTimeModified: now.AddDate(0, -1, -2),
			},
			AuthIdentifier: "admin",
			Disabled:       false,
			Profile: StringValueMap{
				"userName": ValueType{Kind: "STRING", Value: "default user"},
				"attr2":    ValueType{Kind: "INT", Value: "1"},
			},
		},
	}
	mObj0 := dUsers[0].ToModel()
	mObj1 := dUsers[1].ToModel()
	var consumedCount int
	consumer := func(obj interface{}) {
		n, ok := obj.(*User)
		if assert.True(ok) {
			assert.Equal(dUsers[consumedCount], n)
		}
		consumedCount++
	}
	ctx := context.Background()
	var params interface{}
	opts := options.Find()

	t.Log("case: success")
	filter := bson.M{"name": "value"}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return(dbName).MinTimes(1)
	cl := mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(cl)
	db := mock.NewMockDatabase(mockCtrl)
	cl.EXPECT().Database(dbName).Return(db)
	c := mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection(testCol).Return(c)
	cur := mock.NewMockCursor(mockCtrl)
	c.EXPECT().Find(ctx, filter, opts).Return(cur, nil)
	prev := cur.EXPECT().Next(ctx).Return(true).Times(2)
	cur.EXPECT().Next(ctx).Return(false).After(prev)
	prev = cur.EXPECT().Decode(newMockDecodeMatcher(t, mObj0)).Return(nil)
	cur.EXPECT().Decode(newMockDecodeMatcher(t, mObj1)).Return(nil).After(prev)
	cur.EXPECT().Close(ctx).Return(nil)
	cur.EXPECT().Err().Return(nil)
	api.EXPECT().ErrorCode(nil).Return(mongodb.ECKeyNotFound)
	crud := NewObjectDocumentHandlerCRUD(api)
	assert.NoError(crud.FindAll(ctx, h, params, filter, consumer))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Find fails")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return(dbName).MinTimes(1)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	cl = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(cl)
	db = mock.NewMockDatabase(mockCtrl)
	cl.EXPECT().Database(dbName).Return(db)
	c = mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection(testCol).Return(c)
	c.EXPECT().Find(ctx, filter, opts).Return(nil, errUnknownError)
	api.EXPECT().WrapError(errUnknownError, false).Return(errWrappedError)
	crud = NewObjectDocumentHandlerCRUD(api)
	err := crud.FindAll(ctx, h, params, filter, consumer)
	assert.Equal(errWrappedError, err)
	assert.Equal(1, tl.CountPattern(testCol+`.*"name":"value"`))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Decode fails")
	consumedCount = 0
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return(dbName).MinTimes(1)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	cl = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(cl)
	db = mock.NewMockDatabase(mockCtrl)
	cl.EXPECT().Database(dbName).Return(db)
	c = mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection(testCol).Return(c)
	c.EXPECT().Find(ctx, filter, opts).Return(cur, nil)
	cur.EXPECT().Next(ctx).Return(true)
	cur.EXPECT().Decode(newMockDecodeMatcher(t, nil)).Return(errUnknownError)
	cur.EXPECT().Close(ctx).Return(nil)
	api.EXPECT().WrapError(errUnknownError, false).Return(errWrappedError)
	crud = NewObjectDocumentHandlerCRUD(api)
	err = crud.FindAll(ctx, h, params, filter, consumer)
	assert.Equal(errWrappedError, err)
	assert.Equal(1, tl.CountPattern(testCol+`.*"name":"value"`))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Err returns an error")
	consumedCount = 0
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return(dbName).MinTimes(1)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	cl = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(cl)
	db = mock.NewMockDatabase(mockCtrl)
	cl.EXPECT().Database(dbName).Return(db)
	c = mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection(testCol).Return(c)
	c.EXPECT().Find(ctx, filter, opts).Return(cur, nil)
	prev = cur.EXPECT().Next(ctx).Return(true)
	cur.EXPECT().Next(ctx).Return(false).After(prev)
	prev = cur.EXPECT().Decode(newMockDecodeMatcher(t, mObj0)).Return(nil)
	cur.EXPECT().Err().Return(errUnknownError)
	cur.EXPECT().Close(ctx).Return(errDuplicateKey)
	api.EXPECT().WrapError(errUnknownError, false).Return(errWrappedError)
	crud = NewObjectDocumentHandlerCRUD(api)
	err = crud.FindAll(ctx, h, params, filter, consumer)
	assert.Equal(errWrappedError, err)
	assert.Equal(1, tl.CountPattern(testCol+`.*"name":"value"`))
}

func TestGetFindAllOptions(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	type FindParams struct {
		SortAsc, SortDesc []string
		Skip, Limit       *int32
	}

	var params FindParams
	crud := &CollectionCRUD{}
	opts := crud.getFindAllOptions(params)
	assert.Nil(opts.Sort)
	assert.Nil(opts.Limit)
	assert.Nil(opts.Skip)

	params.Limit = swag.Int32(10)
	params.Skip = swag.Int32(2)
	params.SortAsc = []string{"key1", "key2"}
	params.SortDesc = []string{"key3"}
	opts = crud.getFindAllOptions(params)
	assert.NotNil(opts.Limit)
	assert.Equal(int64(10), *opts.Limit)
	assert.NotNil(opts.Skip)
	assert.Equal(int64(2), *opts.Skip)
	assert.NotNil(opts.Sort)
	leMap, ok := opts.Sort.(map[string]int)
	assert.True(ok)
	assert.Contains(leMap, "key1")
	assert.Contains(leMap, "key2")
	assert.Contains(leMap, "key3")
	assert.Equal(1, leMap["key1"])
	assert.Equal(1, leMap["key2"])
	assert.Equal(-1, leMap["key3"])

	type InvalidFindParams struct {
		SortAsc, SortDesc []int
		Skip, Limit       string
	}

	invalidParams := InvalidFindParams{
		SortAsc:  []int{1, 2, 3, 4},
		SortDesc: []int{4, 5, 6},
		Skip:     "skip",
		Limit:    "limit",
	}
	opts = crud.getFindAllOptions(invalidParams)
	assert.Nil(opts.Sort)
	assert.Nil(opts.Limit)
	assert.Nil(opts.Skip)

	// test convertSortKey
	cases := map[string]string{
		"meta.Id":           "MetaID",
		"meta.timeCreated":  "MetaTC",
		"meta.timeModified": "MetaTM",
		"meta.Version":      "MetaVer",
		"AAaa":              "aaaa",
		"abc.DEF":           "abc.def",
		"AbC123":            "abc123",
	}
	for k, v := range cases {
		retVal := crud.convertSortKey(k)
		assert.Equal(v, retVal)
	}
}

func TestFindOne(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	dbName := "testDB"
	testCol := "testCol"

	h := &fakeODH{}
	dObj := &User{}
	mObj := &models.User{}
	mObj.AuthIdentifier = "user"
	ctx := context.Background()

	t.Log("case: success")
	filter := bson.M{"name": "value"}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return(dbName).MinTimes(1)
	cl := mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(cl)
	db := mock.NewMockDatabase(mockCtrl)
	cl.EXPECT().Database(dbName).Return(db)
	c := mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection(testCol).Return(c)
	sr := mock.NewMockSingleResult(mockCtrl)
	c.EXPECT().FindOne(ctx, filter).Return(sr)
	sr.EXPECT().Decode(newMockDecodeMatcher(t, mObj)).Return(nil)
	api.EXPECT().ErrorCode(nil).Return(mongodb.ECKeyNotFound)
	crud := NewObjectDocumentHandlerCRUD(api)
	assert.NoError(crud.FindOne(ctx, h, filter, dObj))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return(dbName).MinTimes(1)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	cl = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(cl)
	db = mock.NewMockDatabase(mockCtrl)
	cl.EXPECT().Database(dbName).Return(db)
	c = mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection(testCol).Return(c)
	sr = mock.NewMockSingleResult(mockCtrl)
	c.EXPECT().FindOne(ctx, filter).Return(sr)
	sr.EXPECT().Decode(newMockDecodeMatcher(t, nil)).Return(errUnknownError)
	api.EXPECT().WrapError(errUnknownError, false).Return(errWrappedError)
	crud = NewObjectDocumentHandlerCRUD(api)
	err := crud.FindOne(ctx, h, filter, dObj)
	assert.Equal(errWrappedError, err)
	assert.Equal(1, tl.CountPattern(testCol+`.*"name":"value"`))
}

func TestInsertOne(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	dbName := "testDB"
	testCol := "testCol"

	h := &fakeODH{}
	ctx := context.Background()

	t.Log("case: success")
	obj := bson.M{"name": "value"}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return(dbName).MinTimes(1)
	cl := mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(cl)
	db := mock.NewMockDatabase(mockCtrl)
	cl.EXPECT().Database(dbName).Return(db)
	c := mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection(testCol).Return(c)
	c.EXPECT().InsertOne(ctx, obj).Return(nil, nil)
	api.EXPECT().ErrorCode(nil).Return(mongodb.ECKeyNotFound)
	crud := NewObjectDocumentHandlerCRUD(api)
	assert.NoError(crud.InsertOne(ctx, h, obj))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return(dbName).MinTimes(1)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	cl = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(cl)
	db = mock.NewMockDatabase(mockCtrl)
	cl.EXPECT().Database(dbName).Return(db)
	c = mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection(testCol).Return(c)
	c.EXPECT().InsertOne(ctx, obj).Return(nil, errUnknownError)
	api.EXPECT().WrapError(errUnknownError, false).Return(errWrappedError)
	crud = NewObjectDocumentHandlerCRUD(api)
	err := crud.InsertOne(ctx, h, obj)
	assert.Equal(errWrappedError, err)
}

func TestUpdateAll(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	dbName := "testDB"
	testCol := "testCol"

	h := &fakeODH{}
	ctx := context.Background()

	t.Log("case: success")
	filter := bson.M{"name": "value"}
	obj := &Node{
		Service: NuvoService{
			ServiceState: ServiceState{State: "READY"},
		},
	}
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
			},
		},
	}
	updates := bson.M{
		"$inc":         bson.M{objVer: 1},
		"$currentDate": bson.M{objTM: true},
		"$set": bson.M{
			"service.state": obj.Service.State,
		},
	}
	ur := &mongo.UpdateResult{MatchedCount: 3, ModifiedCount: 1}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return(dbName).MinTimes(1)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	cl := mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(cl)
	db := mock.NewMockDatabase(mockCtrl)
	cl.EXPECT().Database(dbName).Return(db)
	c := mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection(testCol).Return(c)
	c.EXPECT().UpdateMany(ctx, filter, updates).Return(ur, nil)
	api.EXPECT().ErrorCode(nil).Return(mongodb.ECKeyNotFound)
	crud := NewObjectDocumentHandlerCRUD(api)
	mod, match, err := crud.UpdateAll(ctx, h, filter, obj, ua)
	assert.NoError(err)
	assert.Equal(1, mod)
	assert.Equal(3, match)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return(dbName).MinTimes(1)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	cl = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(cl)
	db = mock.NewMockDatabase(mockCtrl)
	cl.EXPECT().Database(dbName).Return(db)
	c = mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection(testCol).Return(c)
	c.EXPECT().UpdateMany(ctx, filter, updates).Return(nil, errUnknownError)
	api.EXPECT().WrapError(errUnknownError, true).Return(errWrappedError)
	crud = NewObjectDocumentHandlerCRUD(api)
	mod, match, err = crud.UpdateAll(ctx, h, filter, obj, ua)
	assert.Equal(errWrappedError, err)
	assert.Zero(mod)
	assert.Zero(match)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: createUpdateQuery fails - ensure the reason is passed on")
	ua.Attributes[0].Actions[ds.UpdateAppend].FromBody = true // invalid
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	crud = NewObjectDocumentHandlerCRUD(api)
	mod, match, err = crud.UpdateAll(ctx, h, filter, obj, ua)
	assert.Regexp("append='Service': invalid datatype for operation$", err)
	assert.Regexp("^"+ds.ErrorUpdateInvalidRequest.M, err)
	assert.Zero(mod)
	assert.Zero(match)
}

func TestUpdateOne(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	dbName := "testDB"
	testCol := "testCol"

	h := &fakeODH{}
	ctx := context.Background()

	t.Log("case: success")
	obj := &User{
		AuthIdentifier: "a@b",
	}
	now := time.Now()
	then := now.Add(-1 * time.Minute)
	mObj := &models.User{}
	mObj.Meta = &models.ObjMeta{
		ID:           "uid",
		Version:      25,
		ObjType:      "User",
		TimeCreated:  strfmt.DateTime(then),
		TimeModified: strfmt.DateTime(now),
	}
	mObj.AuthIdentifier = "user"
	expObj := &User{}
	expObj.FromModel(mObj)
	ua := &ds.UpdateArgs{
		ID: "uid",
		Attributes: []ds.UpdateAttr{
			{
				Name: "AuthIdentifier",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	filter := bson.M{
		"MetaID": "uid",
	}
	updates := bson.M{
		"$inc":         bson.M{objVer: 1},
		"$currentDate": bson.M{objTM: true},
		"$set": bson.M{
			"authidentifier": obj.AuthIdentifier,
		},
	}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return(dbName).MinTimes(1)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	cl := mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(cl)
	db := mock.NewMockDatabase(mockCtrl)
	cl.EXPECT().Database(dbName).Return(db)
	c := mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection(testCol).Return(c)
	sr := mock.NewMockSingleResult(mockCtrl)
	c.EXPECT().FindOneAndUpdate(ctx, filter, updates, opts).Return(sr)
	sr.EXPECT().Decode(newMockDecodeMatcher(t, mObj)).Return(nil)
	api.EXPECT().ErrorCode(nil).Return(mongodb.ECKeyNotFound)
	crud := NewObjectDocumentHandlerCRUD(api)
	err := crud.UpdateOne(ctx, h, obj, ua)
	assert.NoError(err)
	assert.Equal(expObj, obj)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure, include extra filters, ua.Version wins")
	obj = &User{
		AuthIdentifier: "a@b",
	}
	expObj = &User{
		AuthIdentifier: "a@b",
	}
	extra := []bson.E{{Key: "MetaVer", Value: int32(12)}, {Key: "nodeidentifier", Value: "123"}}
	filter = bson.M{
		"MetaID":         "uid",
		"MetaVer":        int32(23),
		"nodeidentifier": "123",
	}
	ua.Version = 23
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return(dbName).MinTimes(1)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	cl = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(cl)
	db = mock.NewMockDatabase(mockCtrl)
	cl.EXPECT().Database(dbName).Return(db)
	c = mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection(testCol).Return(c)
	sr = mock.NewMockSingleResult(mockCtrl)
	c.EXPECT().FindOneAndUpdate(ctx, filter, updates, opts).Return(sr)
	sr.EXPECT().Decode(newMockDecodeMatcher(t, nil)).Return(errUnknownError)
	api.EXPECT().WrapError(errUnknownError, true).Return(errWrappedError)
	crud = NewObjectDocumentHandlerCRUD(api)
	err = crud.UpdateOne(ctx, h, obj, ua, extra...)
	assert.Equal(errWrappedError, err)
	assert.Equal(expObj, obj)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: createUpdateQuery fails - ensure the reason is passed on")
	ua.Attributes[0].Actions[ds.UpdateAppend].FromBody = true // invalid
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	crud = NewObjectDocumentHandlerCRUD(api)
	err = crud.UpdateOne(ctx, h, obj, ua)
	assert.Regexp("append='AuthIdentifier': invalid datatype for operation$", err)
	assert.Regexp("^"+ds.ErrorUpdateInvalidRequest.M, err)
}

type decodeMatcher struct {
	obj *models.User // any model document type would suffice, User is convenient
	m   bson.M
}

// if obj is nil the matcher copies out nothing, allowing this to be used in the case mock Decode returns an error
func newMockDecodeMatcher(t *testing.T, obj interface{}) gomock.Matcher {
	if obj == nil {
		return &decodeMatcher{}
	}
	switch p := obj.(type) {
	case *models.User:
		return &decodeMatcher{obj: p}
	case *bson.M:
		return &decodeMatcher{m: *p}
	}
	assert.Fail(t, "invalid param type")
	return &decodeMatcher{}
}

func (m *decodeMatcher) Matches(x interface{}) bool {
	if m.obj == nil && m.m == nil { // nothing to copy, used to cover Decode returning an error, destination object must not be nil
		return x != nil
	}
	switch p := x.(type) {
	case *User:
		if m.obj != nil && p != nil {
			p.FromModel(m.obj)
			return true
		}
	case *bson.M:
		if m.m != nil && p != nil {
			for k, v := range m.m {
				(*p)[k] = v
			}
			return true
		}
	}
	return false
}

func (m *decodeMatcher) String() string {
	if m.obj != nil {
		return "decoder matches *User"
	} else if m.m != nil {
		return "decoder matches bson.M"
	}
	return "decoder matches nil"
}

type consumeObjFnMatcher struct {
	t       *testing.T
	objList []interface{}
}

func newConsumeObjFnMatcher(t *testing.T, list ...interface{}) gomock.Matcher {
	return &consumeObjFnMatcher{t: t, objList: list}
}

func (m *consumeObjFnMatcher) Matches(x interface{}) bool {
	switch p := x.(type) {
	case ConsumeObjFn:
		for _, obj := range m.objList {
			p(obj)
		}
		return true
	}
	return false
}

func (m *consumeObjFnMatcher) String() string {
	return "matches ConsumeObjFn"
}
