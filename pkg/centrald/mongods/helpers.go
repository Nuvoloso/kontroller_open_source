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


// Package mongods provides a MongoDB adaptor to the data store abstraction layer
package mongods

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/op/go-logging"
	"go.mongodb.org/mongo-driver/bson"
)

// populateCollection reads json files for the collection and uses the Populator to insert these documents into the database.
// All validations are delegated to the Populator.
func populateCollection(ctx context.Context, api DBAPI, p Populator) error {
	path := api.BaseDataPathName()
	if path == "" {
		api.Logger().Infof("populateCollection: skipped loading built-in %s documents", p.CName())
		return nil
	}

	path = filepath.Join(path, p.CName())
	return filepath.Walk(path, func(fp string, _ os.FileInfo, err error) error {
		if err != nil {
			api.Logger().Errorf("populateCollection: walk failed %s: %s", fp, err.Error())
			return err
		} else if fp == path {
			return nil
		}
		return walker(ctx, api, p, fp)
	})
}

func walker(ctx context.Context, api DBAPI, p Populator, fp string) error {
	buf, err := ioutil.ReadFile(fp)
	if err != nil {
		api.Logger().Errorf("populateCollection: unable to read %s: %s", fp, err.Error())
		return err
	}
	return p.Populate(ctx, fp, buf)
}

// createUpdateQuery constructs a Mongo query to perform an update operation.
// The returned BSON is of the form:
//   {
//     "$inc": { <objVer>: 1 },
//     "$currentDate": { <objTM>: true },
//     "$set": { fieldWithOptSubfield: value, ... }
//     "$unset": { field: value, ... }
//     "$addToSet": { field : { "$each": [value, ...]}, ... }
//     "$pull": { field: {"$in": [value,...]}, ...}
//   }
// where the operations depend on the ua parameter.
// The param parameter is a pointer to any object Mutable - reflection is used to access
// the values.
// All field names are translated to lower case as per the default BSON rules.
// An error is returned if there are no updates.
func createUpdateQuery(log *logging.Logger, ua *centrald.UpdateArgs, param interface{}) (bson.M, error) {
	//log.Debug("createUpdateQuery")
	setDoc := bson.M{}
	unsetDoc := bson.M{}
	addToSetDoc := bson.M{}
	pullDoc := bson.M{}
	// uErr produces an extended error message
	uErr := func(format string, args ...interface{}) error {
		msg := fmt.Sprintf(centrald.ErrorUpdateInvalidRequest.M+": "+format, args...)
		log.Error(msg)
		return &centrald.Error{M: msg, C: centrald.ErrorUpdateInvalidRequest.C}
	}
	// lc lower cases a string
	lc := func(s string) string {
		return strings.ToLower(s)
	}
	// use reflection to construct the query
	pV := reflect.Indirect(reflect.ValueOf(param))
	setDocV := reflect.ValueOf(setDoc)
	unsetDocV := reflect.ValueOf(unsetDoc)
	addToSetDocV := reflect.ValueOf(addToSetDoc)
	pullDocV := reflect.ValueOf(pullDoc)
	inOpV := reflect.ValueOf("$in")
	eachOpV := reflect.ValueOf("$each")
	esV := reflect.ValueOf("")
	for _, a := range ua.Attributes {
		pFV := pV.FieldByName(a.Name)
		if !pFV.IsValid() {
			return nil, uErr("update '%s': invalid name", a.Name)
		}
		if pFV.Type().Kind() == reflect.Ptr {
			log.Debug("Indirect call", a.Name)
			pFV = reflect.Indirect(pFV)
			if !pFV.IsValid() {
				if !a.IsModified() {
					log.Debug("Ignoring nil pointer for", a.Name)
					continue
				}
				return nil, uErr("update '%s': value not specified", a.Name)
			}
		}
		//log.Debug("Processing", a.Name)
		for act, actA := range a.Actions {
			t := pFV.Type()
			if act == int(centrald.UpdateSet) {
				if actA.FromBody {
					setDocV.SetMapIndex(reflect.ValueOf(lc(a.Name)), pFV)
				} else if len(actA.Fields) > 0 {
					log.Debug("Set", a.Name, ".Fields", actA.Fields)
					if t.Kind() == reflect.Map {
						if t.Key().Kind() != reflect.String {
							for f := range actA.Fields {
								return nil, uErr("set='%s.%s': subfield not supported for datatype", a.Name, f)
							}
						}
						for f := range actA.Fields {
							k := reflect.ValueOf(lc(a.Name) + "." + f) // lc map name only
							v := pFV.MapIndex(reflect.ValueOf(f))
							if !v.IsValid() {
								return nil, uErr("set='%s.%s': key not found", a.Name, f)
							}
							setDocV.SetMapIndex(k, v)
						}
					} else if t.Kind() == reflect.Struct {
						for f := range actA.Fields {
							k := reflect.ValueOf(lc(a.Name + "." + f)) // lc struct + field names
							v := pFV.FieldByName(f)
							if !v.IsValid() {
								return nil, uErr("set='%s.%s': invalid field", a.Name, f)
							}
							setDocV.SetMapIndex(k, v)
						}
					} else {
						return nil, uErr("set='%s': subfield not supported for datatype", a.Name)
					}
				} else if len(actA.Indexes) > 0 {
					log.Debug("Set", a.Name, ".Indexes", actA.Indexes)
					if t.Kind() != reflect.Slice && t.Kind() != reflect.Array {
						return nil, uErr("set='%s': subfield not supported for datatype", a.Name)
					}
					for idx := range actA.Indexes {
						if idx < 0 || idx >= pFV.Len() {
							return nil, uErr("set='%s.%d': invalid index", a.Name, idx)
						}
						k := reflect.ValueOf(lc(fmt.Sprintf("%s.%d", a.Name, idx)))
						v := pFV.Index(idx)
						setDocV.SetMapIndex(k, v)
					}
				}
			} else {
				actS := "remove"
				isRemove := true
				if act == int(centrald.UpdateAppend) {
					actS = "append"
					isRemove = false
				}
				if (len(actA.Fields) == 0 && len(actA.Indexes) == 0) && !actA.FromBody {
					continue
				}
				if !actA.FromBody {
					return nil, uErr("%s='%s': subfield not supported for operation", actS, a.Name)
				}
				var isArray, isStringTMap bool
				tK := t.Kind()
				if tK == reflect.Slice || tK == reflect.Array {
					isArray = true
				}
				if !isArray {
					if tK == reflect.Map && t.Key().Kind() == reflect.String {
						isStringTMap = true
					}
				}
				if !isArray && !isStringTMap {
					return nil, uErr("%s='%s': invalid datatype for operation", actS, a.Name)
				}
				if isArray {
					opDoc := bson.M{}
					opDocV := reflect.ValueOf(opDoc)
					var docV reflect.Value
					if isRemove {
						docV = pullDocV
						opDocV.SetMapIndex(inOpV, pFV)
					} else {
						docV = addToSetDocV
						opDocV.SetMapIndex(eachOpV, pFV)
					}
					docV.SetMapIndex(reflect.ValueOf(lc(a.Name)), opDocV)
				}
				if isStringTMap {
					for _, kV := range pFV.MapKeys() {
						kfV := reflect.ValueOf(lc(a.Name) + "." + kV.String()) // lc map name only
						if isRemove {
							unsetDocV.SetMapIndex(kfV, esV)
						} else {
							setDocV.SetMapIndex(kfV, pFV.MapIndex(kV))
						}
					}
				}
			}
		}
	}
	updates := bson.M{
		"$inc":         bson.M{objVer: 1},
		"$currentDate": bson.M{objTM: true},
	}
	haveUpdates := false
	if len(setDoc) > 0 {
		updates["$set"] = setDoc
		haveUpdates = true
	}
	if len(unsetDoc) > 0 {
		updates["$unset"] = unsetDoc
		haveUpdates = true
	}
	if len(addToSetDoc) > 0 {
		updates["$addToSet"] = addToSetDoc
		haveUpdates = true
	}
	if len(pullDoc) > 0 {
		updates["$pull"] = pullDoc
		haveUpdates = true
	}
	if !haveUpdates {
		log.Error(centrald.ErrorUpdateInvalidRequest.M)
		return nil, centrald.ErrorUpdateInvalidRequest
	}
	return updates, nil
}

// createAggregationGroup constructs a Mongo group document to to be used in a Mongo aggregation pipeline.
// The returned BSON is of the form:
//   {
//     "$group": {
//       "_id": nil,
//       "count": { "$sum": 1 },
//       "sum0": { "$sum": "$fieldPath0" },
//       "sum1": { "$sum": "$fieldPath1" },
//       ...
//     }
//   }
// The params parameters is expected to be a pointer to a ListParams struct containing aggregation fields.
// Currently, only a "Sum" field is supported - all other fields of params are ignored.
// The params "Sum" field must be of type []string and contains field paths (top level attribute or attribute.field.optionalSubfields).
// The array may be empty or nil, in which case the returned group BSON contains only the "_id" and "count" fields.
// All field names are translated to lower case as per the default BSON rules.
// The N of sumN and fieldPathN refer to the indexes and elements of the "Sum" array.
// An error is returned if any fieldPath is empty or contains unexpected characters; only dot-separated fields of alphanumeric and
// dash characters are allowed. createAggregationGroup panics if params is not a (pointer to) struct with at least a "Sum" field.
// TBD: support additional aggregations and perform a more thorough validation of the fieldPaths.
func createAggregationGroup(log *logging.Logger, params interface{}) (bson.M, error) {
	// aErr produces an extended error message
	aErr := func(format string, args ...interface{}) error {
		msg := fmt.Sprintf(centrald.ErrorInvalidData.M+": "+format, args...)
		log.Error(msg)
		return &centrald.Error{M: msg, C: centrald.ErrorInvalidData.C}
	}
	pV := reflect.Indirect(reflect.ValueOf(params)) // or panics
	pFV := pV.FieldByName("Sum")                    // or panics
	sums := pFV.Interface().([]string)              // or panics
	group := bson.M{"_id": nil, "count": bson.M{"$sum": 1}}
	for n, fieldPath := range sums {
		if ok, _ := regexp.MatchString("^[a-zA-Z0-9-]+(\\.[a-zA-Z0-9-]+)*$", fieldPath); !ok {
			return nil, aErr("invalid aggregate field path: %s", fieldPath)
		}
		fieldPath := "$" + strings.ToLower(fieldPath)
		sumName := centrald.SumType + strconv.Itoa(n)
		group[sumName] = bson.M{"$sum": fieldPath}
	}
	group = bson.M{"$group": group}
	log.Info("aggregation group:", group)
	return group, nil
}

// createAggregationList creates an aggregation list from the Mongo aggregation result object
func createAggregationList(fieldPaths []string, result bson.M) []*centrald.Aggregation {
	list := make([]*centrald.Aggregation, len(fieldPaths))
	for i, fieldPath := range fieldPaths {
		sumName := centrald.SumType + strconv.Itoa(i)
		var value int64
		switch t := result[sumName].(type) {
		case int:
			value = int64(t)
		case int64:
			value = t
		}
		list[i] = &centrald.Aggregation{FieldPath: fieldPath, Type: centrald.SumType, Value: value}
	}
	return list
}

// andOrList provides an accumulator of $or documents and a generator for the $and or $or document needed in the query
type andOrList struct {
	orParamDocs []bson.M
}

// newAndOrList returns a new, empty andOrList
func newAndOrList() *andOrList {
	return &andOrList{}
}

// append adds a single $or document to the andOrList for the list of expressions.
// The new element in the andOrList will be of the form:
// { "$or": [ <expr1>, <expr2>, ... , <exprN> ] }
func (list *andOrList) append(exprs ...bson.M) {
	orDoc := bson.M{"$or": exprs}
	list.orParamDocs = append(list.orParamDocs, orDoc)
}

// setQueryParam sets a query document key containing all of the previously appended $or documents.
// If there is a single $or document, a single "$or" key is set to the queryDoc.
// If there are 2 or more, a single "$and" key is set and its value is the list of "$or" documents.
// If there are no documents in the andOrList, the queryDoc is unchanged.
func (list *andOrList) setQueryParam(queryDoc bson.M) {
	switch len(list.orParamDocs) {
	case 0:
		// set nothing
	case 1:
		queryDoc["$or"] = list.orParamDocs[0]["$or"]
	default:
		queryDoc["$and"] = list.orParamDocs
	}
}
