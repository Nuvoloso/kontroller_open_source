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
	"strconv"
	"strings"

	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/mongodb"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ObjectDocumentHandler describes a handler for an object represented by a Mongo document
type ObjectDocumentHandler interface {
	mongodb.ObjectDocumentHandler

	// Ops returns an object that satisfies one of the centrald.datastore "ObjectOps" interfaces
	// though this is not enforced.
	Ops() interface{}
	// Claim is called by a concrete DBAPI to rendezvous with the document handler.
	// This is called after the DBAPI is is initialized but prior to connecting the database.
	Claim(DBAPI, ObjectDocumentHandlerCRUD)
	// Indexes returns the list of indexes for the collection
	Indexes() []mongo.IndexModel
	// NewObject returns a new object appropriate to decode a Mongo document of this type
	NewObject() interface{}
}

// odhRegistry contains all registered handlers by name.
// Implementations can access this map to view the available handlers.
var odhRegistry = mongodb.ObjectDocumentHandlerMap{}

// odhRegister registers a named handler.
// It is intended to be called from init functions of the individual document handlers.
func odhRegister(name string, objH ObjectDocumentHandler) {
	if _, ok := odhRegistry[name]; ok {
		panic("Duplicate ODH named " + name)
	}
	odhRegistry[name] = objH
}

// ConsumeObjFn is called after the object is queried and decoded to consume the object, eg append it to a return list.
type ConsumeObjFn func(obj interface{})

// ObjectDocumentHandlerCRUD provides common CRUD functionality for all of the ObjectDocumentHandler functions
type ObjectDocumentHandlerCRUD interface {
	Aggregate(ctx context.Context, op ObjectDocumentHandler, params interface{}, filter bson.M) ([]*centrald.Aggregation, int, error)
	Count(ctx context.Context, odh ObjectDocumentHandler, filter bson.M, limit uint) (int, error)
	CreateIndexes(ctx context.Context, op ObjectDocumentHandler) error
	DeleteOne(ctx context.Context, odh ObjectDocumentHandler, oid string) error
	FindAll(ctx context.Context, op ObjectDocumentHandler, params interface{}, filter bson.M, consume ConsumeObjFn) error
	FindOne(ctx context.Context, op ObjectDocumentHandler, filter bson.M, destObj interface{}) error
	InsertOne(ctx context.Context, odh ObjectDocumentHandler, obj interface{}) error
	UpdateAll(ctx context.Context, op ObjectDocumentHandler, filter bson.M, obj interface{}, ua *centrald.UpdateArgs) (int, int, error)
	UpdateOne(ctx context.Context, op ObjectDocumentHandler, obj interface{}, ua *centrald.UpdateArgs, filters ...bson.E) error
}

// CollectionCRUD provides a default implementation of ObjectDocumentHandlerCRUD
type CollectionCRUD struct {
	api DBAPI
}

// NewObjectDocumentHandlerCRUD returns a CollectionCRUD
func NewObjectDocumentHandlerCRUD(api DBAPI) ObjectDocumentHandlerCRUD {
	return &CollectionCRUD{api: api}
}

// Aggregate runs an aggregation framework pipeline with 2 stages: a filter stage and a count/summation stage.
// The params parameters is expected to be a pointer to a ListParams struct containing aggregation fields.
// Currently, only a "Sum" field is supported - all other fields of params are ignored.
// The result list and a count are returned on success
func (c *CollectionCRUD) Aggregate(ctx context.Context, op ObjectDocumentHandler, params interface{}, filter bson.M) ([]*centrald.Aggregation, int, error) {
	agg, err := createAggregationGroup(c.api.Logger(), params)
	if err != nil {
		b, _ := bson.MarshalExtJSON(filter, true, false)
		c.api.Logger().Errorf("%s.Aggregate(%s): %s", op.CName(), strings.TrimSpace(string(b)), err)
		return nil, 0, err
	}
	pV := reflect.Indirect(reflect.ValueOf(params)) // or panics
	pFV := pV.FieldByName("Sum")                    // or panics
	sums := pFV.Interface().([]string)              // or panics
	pipe := make([]bson.M, 2)
	pipe[0] = bson.M{"$match": filter}
	pipe[1] = agg
	result := bson.M{}
	cur, err := c.api.Client().Database(c.api.DBName()).Collection(op.CName()).Aggregate(ctx, pipe)
	if err == nil {
		defer func() { cur.Close(ctx) }()
		if cur.Next(ctx) { // expect at most one result from this pipeline
			err = cur.Decode(&result)
		}
		if err == nil {
			if err = cur.Err(); err == nil {
				if len(result) == 0 {
					// act like List(), succeed with a count of zero
					result["count"] = 0
					for i := range sums {
						sumName := centrald.SumType + strconv.Itoa(i)
						result[sumName] = 0
					}
				}
				n, _ := result["count"].(int)
				list := createAggregationList(sums, result)
				c.api.ErrorCode(nil)
				return list, n, nil // successful return
			}
		}
	}
	err = c.api.WrapError(err, false)
	b, _ := bson.MarshalExtJSON(filter, true, false)
	c.api.Logger().Errorf("%s.Aggregate(%s): %s", op.CName(), strings.TrimSpace(string(b)), err)
	return nil, 0, err
}

// Count returns the number of documents matching the filter and limit
func (c *CollectionCRUD) Count(ctx context.Context, op ObjectDocumentHandler, filter bson.M, limit uint) (int, error) {
	n, err := c.api.Client().Database(c.api.DBName()).Collection(op.CName()).CountDocuments(ctx, filter, options.Count().SetLimit(int64(limit)))
	if err != nil {
		err = c.api.WrapError(err, false)
		b, _ := bson.MarshalExtJSON(filter, true, false)
		c.api.Logger().Errorf("%s.CountDocuments(%s, %d): %s", op.CName(), strings.TrimSpace(string(b)), limit, err)
		return 0, err
	}
	c.api.ErrorCode(nil)
	return int(n), nil
}

// CreateIndexes expects to only be called during initialization and creates indexes required for the collection,
// automatically creating the collection as needed
func (c *CollectionCRUD) CreateIndexes(ctx context.Context, op ObjectDocumentHandler) error {
	if err := c.api.MustBeInitializing(); err != nil {
		c.api.Logger().Errorf("%s.CreateIndexes: %s", op.CName(), err)
		return err
	}
	dbn := c.api.DBName()
	iv := c.api.Client().Database(dbn).Collection(op.CName()).Indexes()
	// create indexes, auto-creates the collection
	for _, idx := range op.Indexes() {
		if _, err := iv.CreateOne(ctx, idx); err != nil {
			code := c.api.ErrorCode(err)
			b, _ := bson.MarshalExtJSON(idx.Keys, true, false)
			c.api.Logger().Warningf("%s.%s.Indexes.CreateOne(%s): %s (%d)", dbn, op.CName(), strings.TrimSpace(string(b)), err, code)
		}
	}
	return nil
}

// DeleteOne deletes a single document from the collection. Errors are logged
func (c *CollectionCRUD) DeleteOne(ctx context.Context, op ObjectDocumentHandler, oid string) error {
	if _, err := c.api.Client().Database(c.api.DBName()).Collection(op.CName()).DeleteOne(ctx, bson.M{objKey: oid}); err != nil {
		err = c.api.WrapError(err, false)
		c.api.Logger().Errorf("%s.DeleteOne(%s): %s", op.CName(), oid, err)
		return err
	}
	c.api.ErrorCode(nil)
	return nil
}

// FindAll retrieves all of the documents from the collection that match the filter. Each document is decoded into a NewObject and passed to consume(obj).
// Errors are logged
func (c *CollectionCRUD) FindAll(ctx context.Context, op ObjectDocumentHandler, params interface{}, filter bson.M, consume ConsumeObjFn) error {
	opts := c.getFindAllOptions(params)
	cur, err := c.api.Client().Database(c.api.DBName()).Collection(op.CName()).Find(ctx, filter, opts)
	if err == nil {
		defer func() { cur.Close(ctx) }()
		for cur.Next(ctx) {
			// need to creat a new object to decode each document.
			// Especially in the case of map[] type fields, values from earlier documents leak into subsequent decoded documents
			obj := op.NewObject()
			if err = cur.Decode(obj); err != nil {
				break
			}
			consume(obj)
		}
		if err == nil {
			if err = cur.Err(); err == nil {
				c.api.ErrorCode(nil)
				return nil // successful return
			}
		}
	}
	err = c.api.WrapError(err, false)
	b, _ := bson.MarshalExtJSON(filter, true, false)
	c.api.Logger().Errorf("%s.Find(%s): %s", op.CName(), strings.TrimSpace(string(b)), err)
	return err
}

func (c *CollectionCRUD) getFindAllOptions(params interface{}) *options.FindOptions {
	opts := options.Find()
	pValue := reflect.Indirect(reflect.ValueOf(params))
	if pValue.IsValid() {
		sortMap := map[string]int{}
		sortAscF := pValue.FieldByName("SortAsc")
		if sortAscF.IsValid() && sortAscF.Kind() == reflect.Slice {
			for i := 0; i < sortAscF.Len(); i++ {
				key := sortAscF.Index(i)
				if key.Kind() == reflect.String {
					sortMap[c.convertSortKey(key.String())] = 1
				}
			}
		}
		sortDescF := pValue.FieldByName("SortDesc")
		if sortDescF.IsValid() && sortDescF.Kind() == reflect.Slice {
			for i := 0; i < sortDescF.Len(); i++ {
				key := sortDescF.Index(i)
				if key.Kind() == reflect.String {
					sortMap[c.convertSortKey(key.String())] = -1
				}
			}
		}
		if len(sortMap) > 0 {
			opts.SetSort(sortMap)
		}
		limitF := pValue.FieldByName("Limit")
		if limitF.IsValid() && limitF.Kind() == reflect.Ptr {
			limit := limitF.Elem()
			if limit.Kind() == reflect.Int32 {
				opts.SetLimit(limit.Int())
			}
		}
		skipF := pValue.FieldByName("Skip")
		if skipF.IsValid() && skipF.Kind() == reflect.Ptr {
			skip := skipF.Elem()
			if skip.Kind() == reflect.Int32 {
				opts.SetSkip(skip.Int())
			}
		}
	}
	return opts
}

func (c *CollectionCRUD) convertSortKey(key string) string {
	lowerKey := strings.ToLower(key)
	switch lowerKey {
	case "meta.id":
		return "MetaID"
	case "meta.timecreated":
		return "MetaTC"
	case "meta.timemodified":
		return "MetaTM"
	case "meta.version":
		return "MetaVer"
	default:
		return lowerKey
	}
}

// FindOne retrieves a single document from the collection given the filter. The result is decoded into obj. Errors are logged
func (c *CollectionCRUD) FindOne(ctx context.Context, op ObjectDocumentHandler, filter bson.M, obj interface{}) error {
	if err := c.api.Client().Database(c.api.DBName()).Collection(op.CName()).FindOne(ctx, filter).Decode(obj); err != nil {
		err = c.api.WrapError(err, false)
		b, _ := bson.MarshalExtJSON(filter, true, false)
		c.api.Logger().Errorf("%s.FindOne(%s): %s", op.CName(), strings.TrimSpace(string(b)), err)
		return err
	}
	c.api.ErrorCode(nil)
	return nil
}

// InsertOne inserts a single document into the collection. Errors are logged
func (c *CollectionCRUD) InsertOne(ctx context.Context, op ObjectDocumentHandler, obj interface{}) error {
	if _, err := c.api.Client().Database(c.api.DBName()).Collection(op.CName()).InsertOne(ctx, obj); err != nil {
		err = c.api.WrapError(err, false)
		c.api.Logger().Errorf("%s.InsertOne: %s", op.CName(), err)
		return err
	}
	c.api.ErrorCode(nil)
	return nil
}

// UpdateAll updates all documents matching the filter. The number of updated and matched documents are returned
func (c *CollectionCRUD) UpdateAll(ctx context.Context, op ObjectDocumentHandler, filter bson.M, obj interface{}, ua *centrald.UpdateArgs) (int, int, error) {
	updates, err := createUpdateQuery(c.api.Logger(), ua, obj)
	if err != nil {
		return 0, 0, err
	}
	fb, _ := bson.MarshalExtJSON(filter, true, false)
	ub, _ := bson.MarshalExtJSON(updates, true, false)
	c.api.Logger().Debugf("%s.UpdateMany(%s, %s)", op.CName(), strings.TrimSpace(string(fb)), strings.TrimSpace(string(ub)))
	result, err := c.api.Client().Database(c.api.DBName()).Collection(op.CName()).UpdateMany(ctx, filter, updates)
	if err != nil {
		err = c.api.WrapError(err, true)
		c.api.Logger().Errorf("%s.UpdateMany: %s", op.CName(), err)
		return 0, 0, err
	}
	c.api.ErrorCode(nil)
	return int(result.ModifiedCount), int(result.MatchedCount), nil
}

// UpdateOne updates the object identified by obj and ua, with logging. On success, obj is updated to contain the result contents.
// The optional filters can provides additional query parameters. ID and non-zero Version are added to the query from the UpdateArgs
func (c *CollectionCRUD) UpdateOne(ctx context.Context, op ObjectDocumentHandler, obj interface{}, ua *centrald.UpdateArgs, filters ...bson.E) error {
	updates, err := createUpdateQuery(c.api.Logger(), ua, obj)
	if err != nil {
		return err
	}
	b, _ := bson.MarshalExtJSON(updates, true, false)
	c.api.Logger().Debugf("%s.FindOneAndUpdate(%s, %s)", op.CName(), ua.ID, strings.TrimSpace(string(b)))
	filter := bson.M{}
	for _, f := range filters {
		filter[f.Key] = f.Value
	}
	filter[objKey] = ua.ID
	if ua.Version != 0 {
		filter[objVer] = ua.Version
	}
	if err = c.api.Client().Database(c.api.DBName()).Collection(op.CName()).FindOneAndUpdate(ctx, filter, updates, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(obj); err != nil {
		err = c.api.WrapError(err, true)
		c.api.Logger().Errorf("%s.FindOneAndUpdate(%s, %d): %s", op.CName(), ua.ID, ua.Version, err)
		return err
	}
	c.api.ErrorCode(nil)
	return nil
}
