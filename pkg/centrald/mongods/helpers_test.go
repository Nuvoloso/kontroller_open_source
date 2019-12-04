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
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/mongodb"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
)

// mockError is a concrete data type used to return error codes
type mockError struct {
	M string
	C int // should be a Mongo error code
}

var _ = error(&mockError{})

// Error matches the error interface
func (e *mockError) Error() string { return e.M }

var errUnknownError = &mockError{M: "unknown error", C: mongodb.ECUnknownError}
var errNamespaceExists = &mockError{M: "namespace exists", C: mongodb.ECNamespaceExists}
var errDuplicateKey = &mockError{M: "duplicate key", C: mongodb.ECDuplicateKey}

// for use in mocking behavior of WrapError
var errWrappedError = &centrald.Error{M: centrald.ErrorDbError.M + ": wrapped error", C: centrald.ErrorDbError.C}

func TestPopulateCollection(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	badPath := "/no-such-path"

	ctx := context.Background()
	api := NewDataStore(&MongoDataStoreArgs{Args: mongodb.Args{Log: l}})
	fp := &fakePopulator{cName: odhServicePlan.cName}

	t.Log("case: no path")
	assert.NoError(populateCollection(ctx, api, fp))

	t.Log("case: bad path")
	api.BaseDataPath = badPath
	assert.Regexp(badPath, populateCollection(ctx, api, fp))

	t.Log("case: success")
	api.BaseDataPath = "test-data"
	assert.NoError(populateCollection(ctx, api, fp))
	assert.Equal(1, fp.popCount)

	t.Log("case: Populate fails")
	fp.retPopErr = errUnknownError
	assert.Equal(errUnknownError, populateCollection(ctx, api, fp))
	assert.Equal(2, fp.popCount)

	t.Log("case: read file fails") // call walker directly
	assert.Regexp(badPath, walker(ctx, api, fp, badPath))
}

func TestCreateUpdateQuery(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	lcK := func(key string) string { // lower case key only
		parts := strings.Split(key, ".")
		parts[0] = strings.ToLower(parts[0])
		return strings.Join(parts, ".")
	}
	lcKF := func(key string) string { // lower case key + field
		return strings.ToLower(key)
	}

	type SA struct {
		N string
		V string
	}
	type SomeType struct {
		SimpleAttr      string
		SliceAttr       []string
		ArrayAttr       [3]int
		StringTMap      map[string]int
		OtherMap        map[int]string
		StructAttr      SA
		PtrToSimple     *string
		PtrToStruct     *SA
		PtrToStringTMap *map[string]int
		PtrToOtherMap   *map[int]string
		PtrToSlice      *[]string
		PtrToArray      *[3]int
	}
	ua := &centrald.UpdateArgs{}

	t.Log("case: No actions specified")
	param := &SomeType{}
	var uq bson.M
	var err error
	assert.NotPanics(func() { uq, err = createUpdateQuery(l, ua, param) })
	assert.NotNil(err)
	assert.Equal(centrald.ErrorUpdateInvalidRequest, err)
	assert.Nil(uq)

	t.Log("case: set variants, no fields")
	param = &SomeType{
		SimpleAttr:      "SimpleAttr",
		SliceAttr:       []string{"elem1", "elem2"},
		ArrayAttr:       [3]int{1},
		StringTMap:      map[string]int{"a": 1, "b": 2},
		OtherMap:        map[int]string{1: "a", 2: "b"},
		StructAttr:      SA{"n", "v"},
		PtrToSimple:     swag.String("PtrToSimple"),
		PtrToStruct:     &SA{"N", "V"},
		PtrToStringTMap: &map[string]int{"A": 1, "B": 2},
		PtrToOtherMap:   &map[int]string{1: "A", 2: "B"},
		PtrToSlice:      &[]string{"Elem1", "Elem2"},
		PtrToArray:      &[3]int{2, 3, 5},
	}
	setRes := bson.M{
		"SimpleAttr":      param.SimpleAttr,
		"SliceAttr":       param.SliceAttr,
		"ArrayAttr":       param.ArrayAttr,
		"StringTMap":      param.StringTMap,
		"OtherMap":        param.OtherMap,
		"StructAttr":      param.StructAttr,
		"PtrToSimple":     *param.PtrToSimple,
		"PtrToStruct":     *param.PtrToStruct,
		"PtrToStringTMap": *param.PtrToStringTMap,
		"PtrToOtherMap":   *param.PtrToOtherMap,
		"PtrToSlice":      *param.PtrToSlice,
		"PtrToArray":      *param.PtrToArray,
	}
	setResV := reflect.ValueOf(setRes)
	setResLC := bson.M{} // lower cased keys
	for k, v := range setRes {
		setResLC[lcK(k)] = v
	}
	ua = &centrald.UpdateArgs{}
	ua.Attributes = make([]centrald.UpdateAttr, setResV.Len())
	for i, resKeyV := range setResV.MapKeys() {
		ua.Attributes[i] = centrald.UpdateAttr{}
		a := &ua.Attributes[i]
		a.Name = resKeyV.String()
		a.Actions[centrald.UpdateSet].FromBody = true
	}
	res := bson.M{
		"$inc":         bson.M{objVer: 1},
		"$currentDate": bson.M{objTM: true},
		"$set":         setResLC,
	}
	assert.NotPanics(func() { uq, err = createUpdateQuery(l, ua, param) })
	assert.Nil(err)
	assert.NotNil(uq)
	assert.Equal(res, uq)
	tl.Flush()

	t.Log("case: set variants, with fields, no errors")
	param = &SomeType{
		SliceAttr:       []string{"elem1", "elem2"},
		ArrayAttr:       [3]int{2, 3},
		StringTMap:      map[string]int{"a": 1, "b": 2},
		StructAttr:      SA{"n", "v"},
		PtrToStruct:     &SA{"N", "V"},
		PtrToStringTMap: &map[string]int{"A": 1, "B": 2},
		PtrToSlice:      &[]string{"Elem1", "Elem2"},
		PtrToArray:      &[3]int{5, 8, 13},
	}
	paramV := reflect.Indirect(reflect.ValueOf(param))
	setRes = bson.M{
		"SliceAttr.1":       param.SliceAttr[1],
		"SliceAttr.0":       param.SliceAttr[0],
		"ArrayAttr.1":       param.ArrayAttr[1],
		"ArrayAttr.2":       param.ArrayAttr[2],
		"StringTMap.b":      param.StringTMap["b"],
		"StringTMap.a":      param.StringTMap["a"],
		"StructAttr.N":      param.StructAttr.N,
		"StructAttr.V":      param.StructAttr.V,
		"PtrToStruct.V":     (*param.PtrToStruct).V,
		"PtrToStringTMap.B": (*param.PtrToStringTMap)["B"],
		"PtrToSlice.0":      (*param.PtrToSlice)[0],
		"PtrToArray.0":      (*param.PtrToArray)[0],
		"PtrToArray.2":      (*param.PtrToArray)[2],
	}
	setResLC = bson.M{} // lower cased keys
	for k, v := range setRes {
		if strings.Contains(k, "Struct") {
			setResLC[lcKF(k)] = v
		} else {
			setResLC[lcK(k)] = v
		}
	}
	t.Log("setResLC", setResLC)
	setResV = reflect.ValueOf(setRes)
	res = bson.M{
		"$inc":         bson.M{objVer: 1},
		"$currentDate": bson.M{objTM: true},
		"$set":         setResLC,
	}
	es := struct{}{}
	names := make(map[string]struct{}, 0)
	for _, resKeyV := range setResV.MapKeys() {
		parts := strings.Split(resKeyV.String(), ".")
		assert.Len(parts, 2)
		names[parts[0]] = es
	}
	ua = &centrald.UpdateArgs{}
	ua.Attributes = make([]centrald.UpdateAttr, len(names))
	i := 0
	for n := range names {
		ua.Attributes[i] = centrald.UpdateAttr{}
		a := &ua.Attributes[i]
		a.Name = n
		for j := range a.Actions {
			a.Actions[j].Fields = map[string]struct{}{}
			a.Actions[j].Indexes = map[int]struct{}{}
		}
		i++
	}
	for _, resKeyV := range setResV.MapKeys() {
		parts := strings.Split(resKeyV.String(), ".")
		assert.Len(parts, 2)
		var attrIdx int
		var attrName string
		for j, attr := range ua.Attributes {
			if attr.Name == parts[0] {
				attrName = attr.Name
				attrIdx = j
				break
			}
		}
		assert.NotNil(attrName)
		// find the type of the value from the param
		assert.True(paramV.Kind() == reflect.Struct)
		mV := paramV.FieldByName(attrName)
		if mV.Kind() == reflect.Ptr {
			t.Log("Indirect value", attrName)
			mV = reflect.Indirect(mV)
		}
		mVK := mV.Type().Kind()
		if mVK == reflect.Slice || mVK == reflect.Array {
			idx, err := strconv.Atoi(parts[1])
			assert.Nil(err)
			ua.Attributes[attrIdx].Actions[centrald.UpdateSet].Indexes[idx] = es
		} else if mVK == reflect.Map || mVK == reflect.Struct {
			ua.Attributes[attrIdx].Actions[centrald.UpdateSet].Fields[parts[1]] = es
		} else {
			assert.False(true, "should never get here: %s %s", ua.Attributes[attrIdx].Name, mVK.String())
		}
	}

	t.Log("case: set variants, with fields")
	for _, attr := range ua.Attributes {
		act := attr.Actions[centrald.UpdateSet]
		t.Log("Attribute", attr.Name, "Action", act.FromBody, act.Fields, act.Indexes)
		assert.True(act.FromBody || len(act.Fields) > 0 || len(act.Indexes) > 0)
	}
	assert.NotPanics(func() { uq, err = createUpdateQuery(l, ua, param) })
	assert.Nil(err)
	assert.NotNil(uq)
	assert.Equal(res, uq)
	tl.Flush()

	// case: set field/index errors
	param = &SomeType{
		SimpleAttr:      "SimpleAttr",
		SliceAttr:       []string{"elem1", "elem2"},
		ArrayAttr:       [3]int{1},
		StringTMap:      map[string]int{"a": 1, "b": 2},
		OtherMap:        map[int]string{1: "a", 2: "b"},
		StructAttr:      SA{"n", "v"},
		PtrToSimple:     swag.String("PtrToSimple"),
		PtrToStruct:     &SA{"N", "V"},
		PtrToStringTMap: &map[string]int{"A": 1, "B": 2},
		PtrToOtherMap:   &map[int]string{1: "A", 2: "B"},
		PtrToSlice:      &[]string{"Elem1", "Elem2"},
		PtrToArray:      &[3]int{3, 5, 7},
	}
	tcs := []struct {
		n, f string
		i    int
		eReg string
	}{
		{"foo", "", 0, "update 'foo': invalid name"},

		{"SimpleAttr", "", 0, "set='SimpleAttr': subfield not supported for datatype$"},
		{"SimpleAttr", "x", 0, "set='SimpleAttr': subfield not supported for datatype$"},
		{"SliceAttr", "", -1, "set='SliceAttr.-1': invalid index$"},
		{"SliceAttr", "", 3, "set='SliceAttr.3': invalid index$"},
		{"ArrayAttr", "", -2, "set='ArrayAttr.-2': invalid index$"},
		{"ArrayAttr", "", 4, "set='ArrayAttr.4': invalid index$"},
		{"OtherMap", "x", 0, "set='OtherMap.x': subfield not supported for datatype$"},
		{"StringTMap", "x", 0, "set='StringTMap.x': key not found$"},
		{"StructAttr", "x", 0, "set='StructAttr.x': invalid field$"},

		{"PtrToSimple", "", 0, "set='PtrToSimple': subfield not supported for datatype$"},
		{"PtrToSimple", "x", 0, "set='PtrToSimple': subfield not supported for datatype$"},
		{"PtrToSlice", "", -1, "set='PtrToSlice.-1': invalid index$"},
		{"PtrToSlice", "", 3, "set='PtrToSlice.3': invalid index$"},
		{"PtrToArray", "", -2, "set='PtrToArray.-2': invalid index$"},
		{"PtrToArray", "", 4, "set='PtrToArray.4': invalid index$"},
		{"PtrToOtherMap", "x", 0, "set='PtrToOtherMap.x': subfield not supported for datatype$"},
		{"PtrToStringTMap", "x", 0, "set='PtrToStringTMap.x': key not found$"},
		{"PtrToStruct", "x", 0, "set='PtrToStruct.x': invalid field$"},
	}
	for _, tc := range tcs {
		ua = &centrald.UpdateArgs{}
		ua.Attributes = make([]centrald.UpdateAttr, 1)
		a := &ua.Attributes[0]
		a.Name = tc.n
		a.Actions[centrald.UpdateSet].Fields = map[string]struct{}{}
		a.Actions[centrald.UpdateSet].Indexes = map[int]struct{}{}
		if tc.f != "" {
			a.Actions[centrald.UpdateSet].Fields[tc.f] = es
		} else {
			a.Actions[centrald.UpdateSet].Indexes[tc.i] = es
		}
		t.Log("case: set field/index error", tc)
		assert.NotPanics(func() { uq, err = createUpdateQuery(l, ua, param) })
		assert.NotNil(err)
		assert.Nil(uq)
		assert.Regexp(tc.eReg, err.Error())
		assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, err.Error())
		tl.Flush()
	}

	// case: referenced null pointers
	param = &SomeType{}
	tcs2 := []struct {
		n, f string
		i    int
		eReg string
	}{
		{"PtrToSimple", "", -1, "update 'PtrToSimple': value not specified"},
		{"PtrToStruct", "N", -1, "update 'PtrToStruct': value not specified"},
		{"PtrToStringTMap", "foo", -1, "update 'PtrToStringTMap': value not specified"},
		{"PtrToOtherMap", "", -1, "update 'PtrToOtherMap': value not specified"},
		{"PtrToSlice", "", -1, "update 'PtrToSlice': value not specified"},
		{"PtrToArray", "", 2, "update 'PtrToArray': value not specified"},
	}
	for i, tc := range tcs2 {
		ua = &centrald.UpdateArgs{}
		ua.Attributes = make([]centrald.UpdateAttr, 1)
		a := &ua.Attributes[0]
		a.Name = tc.n
		a.Actions[centrald.UpdateSet].Fields = map[string]struct{}{}
		a.Actions[centrald.UpdateSet].Indexes = map[int]struct{}{}
		a.Actions[centrald.UpdateAppend].Fields = map[string]struct{}{}
		a.Actions[centrald.UpdateAppend].Indexes = map[int]struct{}{}
		a.Actions[centrald.UpdateRemove].Fields = map[string]struct{}{}
		a.Actions[centrald.UpdateRemove].Indexes = map[int]struct{}{}
		action := []centrald.UpdateAction{centrald.UpdateSet, centrald.UpdateAppend, centrald.UpdateRemove}[i%3]
		if tc.f != "" {
			a.Actions[action].Fields[tc.f] = es
		} else if tc.i != -1 {
			a.Actions[action].Indexes[tc.i] = es
		} else {
			a.Actions[action].FromBody = true
		}
		t.Log("case: referenced null pointer", tc)
		assert.NotPanics(func() { uq, err = createUpdateQuery(l, ua, param) })
		assert.NotNil(err)
		assert.Nil(uq)
		assert.Regexp(tc.eReg, err.Error())
		assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, err.Error())
		tl.Flush()
	}

	// case: unreferenced null pointers (should be ignored)
	param = &SomeType{}
	tcs3 := []string{
		"PtrToSimple",
		"PtrToStruct",
		"PtrToStringTMap",
		"PtrToOtherMap",
		"PtrToSlice",
		"PtrToArray",
	}
	for _, n := range tcs3 {
		ua = &centrald.UpdateArgs{}
		ua.Attributes = make([]centrald.UpdateAttr, 1)
		a := &ua.Attributes[0]
		a.Name = n
		a.Actions[centrald.UpdateSet].Fields = map[string]struct{}{}
		a.Actions[centrald.UpdateSet].Indexes = map[int]struct{}{}
		a.Actions[centrald.UpdateAppend].Fields = map[string]struct{}{}
		a.Actions[centrald.UpdateAppend].Indexes = map[int]struct{}{}
		a.Actions[centrald.UpdateRemove].Fields = map[string]struct{}{}
		a.Actions[centrald.UpdateRemove].Indexes = map[int]struct{}{}
		t.Log("case: unreferenced null pointer", n)
		assert.NotPanics(func() { uq, err = createUpdateQuery(l, ua, param) })
		assert.NotNil(err)
		assert.Nil(uq)
		assert.Equal(centrald.ErrorUpdateInvalidRequest.M, err.Error()) // nothing set so default error
		tl.Flush()
	}

	// case: append, valid variants
	param = &SomeType{
		SliceAttr:       []string{"elem1", "elem2"},
		ArrayAttr:       [3]int{1},
		StringTMap:      map[string]int{"a": 1, "b": 2},
		PtrToStringTMap: &map[string]int{"A": 10, "B": 20},
		PtrToSlice:      &[]string{"Elem1", "Elem2"},
		PtrToArray:      &[3]int{2, 3, 5},
	}
	addToSetRes := bson.M{
		"SliceAttr":  bson.M{"$each": []string{"elem1", "elem2"}},
		"ArrayAttr":  bson.M{"$each": [3]int{1, 0, 0}},
		"PtrToSlice": bson.M{"$each": []string{"Elem1", "Elem2"}},
		"PtrToArray": bson.M{"$each": [3]int{2, 3, 5}},
	}
	addToSetResV := reflect.ValueOf(addToSetRes)
	addToSetResLC := bson.M{} // lower cased keys
	for k, v := range addToSetRes {
		addToSetResLC[lcK(k)] = v
	}
	setRes = bson.M{
		"StringTMap.a":      1,
		"StringTMap.b":      2,
		"PtrToStringTMap.A": 10,
		"PtrToStringTMap.B": 20,
	}
	setResV = reflect.ValueOf(setRes)
	setResLC = bson.M{} // lower cased keys
	for k, v := range setRes {
		setResLC[lcK(k)] = v
	}
	res = bson.M{
		"$inc":         bson.M{objVer: 1},
		"$currentDate": bson.M{objTM: true},
		"$set":         setResLC,
		"$addToSet":    addToSetResLC,
	}
	names = make(map[string]struct{}, 0)
	for _, resKeyV := range setResV.MapKeys() {
		parts := strings.Split(resKeyV.String(), ".")
		assert.Len(parts, 2)
		names[parts[0]] = es
	}
	for _, addToSetKeyV := range addToSetResV.MapKeys() {
		names[addToSetKeyV.String()] = es
	}
	ua = &centrald.UpdateArgs{}
	ua.Attributes = make([]centrald.UpdateAttr, len(names))
	i = 0
	for n := range names {
		ua.Attributes[i] = centrald.UpdateAttr{}
		a := &ua.Attributes[i]
		a.Name = n
		for j := range a.Actions {
			a.Actions[j].Fields = map[string]struct{}{}
			a.Actions[j].Indexes = map[int]struct{}{}
		}
		i++
	}
	for _, kV := range setResV.MapKeys() {
		parts := strings.Split(kV.String(), ".")
		assert.Len(parts, 2)
		var attrIdx int
		var attrName string
		for j, attr := range ua.Attributes {
			if attr.Name == parts[0] {
				attrName = attr.Name
				attrIdx = j
				break
			}
		}
		assert.NotNil(attrName)
		ua.Attributes[attrIdx].Actions[centrald.UpdateAppend].FromBody = true
	}
	for _, kV := range addToSetResV.MapKeys() {
		var attrIdx int
		var attrName string
		for j, attr := range ua.Attributes {
			if attr.Name == kV.String() {
				attrName = attr.Name
				attrIdx = j
				break
			}
		}
		assert.NotNil(attrName)
		assert.False(ua.Attributes[attrIdx].Actions[centrald.UpdateAppend].FromBody)
		ua.Attributes[attrIdx].Actions[centrald.UpdateAppend].FromBody = true
	}

	t.Log("case: append variants")
	for _, attr := range ua.Attributes {
		act := attr.Actions[centrald.UpdateAppend]
		t.Log("Attribute", attr.Name, "Action", act.FromBody, act.Fields, act.Indexes)
		assert.True(act.FromBody || len(act.Fields) > 0 || len(act.Indexes) > 0)
	}
	assert.NotPanics(func() { uq, err = createUpdateQuery(l, ua, param) })
	assert.Nil(err)
	assert.NotNil(uq)
	assert.Equal(res, uq)
	tl.Flush()

	// case: append, invalid values
	param = &SomeType{
		SimpleAttr:      "SimpleAttr",
		SliceAttr:       []string{"elem1", "elem2"},
		ArrayAttr:       [3]int{1},
		StringTMap:      map[string]int{"a": 1, "b": 2},
		OtherMap:        map[int]string{1: "a", 2: "b"},
		StructAttr:      SA{"n", "v"},
		PtrToSimple:     swag.String("PtrToSimple"),
		PtrToStruct:     &SA{"N", "V"},
		PtrToStringTMap: &map[string]int{"A": 1, "B": 2},
		PtrToOtherMap:   &map[int]string{1: "A", 2: "B"},
		PtrToSlice:      &[]string{"Elem1", "Elem2"},
		PtrToArray:      &[3]int{2, 3, 5},
	}
	tcs = []struct {
		n, f string
		i    int
		eReg string
	}{
		{"foo", "", 0, "update 'foo': invalid name"},
		{"SimpleAttr", "", 0, "append='SimpleAttr': invalid datatype for operation$"},
		{"PtrToSimple", "", 0, "append='PtrToSimple': invalid datatype for operation$"},
		{"OtherMap", "", 0, "append='OtherMap': invalid datatype for operation$"},
		{"PtrToOtherMap", "", 0, "append='PtrToOtherMap': invalid datatype for operation$"},
		{"StructAttr", "", 0, "append='StructAttr': invalid datatype for operation$"},
		{"PtrToStruct", "", 0, "append='PtrToStruct': invalid datatype for operation$"},

		{"StringTMap", "x", 0, "append='StringTMap': subfield not supported for operation$"},
		{"SliceAttr", "", 1, "append='SliceAttr': subfield not supported for operation$"},
	}
	for _, tc := range tcs {
		ua = &centrald.UpdateArgs{}
		ua.Attributes = make([]centrald.UpdateAttr, 1)
		a := &ua.Attributes[0]
		a.Name = tc.n
		a.Actions[centrald.UpdateAppend].Fields = map[string]struct{}{}
		a.Actions[centrald.UpdateAppend].Indexes = map[int]struct{}{}
		if tc.f != "" {
			a.Actions[centrald.UpdateAppend].Fields[tc.f] = es
		} else if tc.i != 0 {
			a.Actions[centrald.UpdateAppend].Indexes[tc.i] = es
		} else {
			a.Actions[centrald.UpdateAppend].FromBody = true
		}
		t.Log("case: append errors", tc)
		assert.NotPanics(func() { uq, err = createUpdateQuery(l, ua, param) })
		assert.NotNil(err)
		assert.Nil(uq)
		assert.Regexp(tc.eReg, err.Error())
		assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, err.Error())
		tl.Flush()
	}

	// case: remove, valid variants
	param = &SomeType{
		SliceAttr:       []string{"elem1", "elem2"},
		ArrayAttr:       [3]int{1},
		StringTMap:      map[string]int{"a": 1, "b": 2},
		PtrToStringTMap: &map[string]int{"A": 10, "B": 20},
		PtrToSlice:      &[]string{"Elem1", "Elem2"},
		PtrToArray:      &[3]int{2, 3, 5},
	}
	pullRes := bson.M{
		"SliceAttr":  bson.M{"$in": []string{"elem1", "elem2"}},
		"ArrayAttr":  bson.M{"$in": [3]int{1, 0, 0}},
		"PtrToSlice": bson.M{"$in": []string{"Elem1", "Elem2"}},
		"PtrToArray": bson.M{"$in": [3]int{2, 3, 5}},
	}
	pullResV := reflect.ValueOf(pullRes)
	pullResLC := bson.M{} // lower cased keys
	for k, v := range pullRes {
		pullResLC[lcK(k)] = v
	}
	unsetRes := bson.M{
		"StringTMap.a":      "",
		"StringTMap.b":      "",
		"PtrToStringTMap.A": "",
		"PtrToStringTMap.B": "",
	}
	unsetResV := reflect.ValueOf(unsetRes)
	unsetResLC := bson.M{} // lower cased keys
	for k, v := range unsetRes {
		unsetResLC[lcK(k)] = v
	}
	res = bson.M{
		"$inc":         bson.M{objVer: 1},
		"$currentDate": bson.M{objTM: true},
		"$unset":       unsetResLC,
		"$pull":        pullResLC,
	}
	names = make(map[string]struct{}, 0)
	for _, kV := range unsetResV.MapKeys() {
		parts := strings.Split(kV.String(), ".")
		assert.Len(parts, 2)
		names[parts[0]] = es
	}
	for _, kV := range pullResV.MapKeys() {
		names[kV.String()] = es
	}
	ua = &centrald.UpdateArgs{}
	ua.Attributes = make([]centrald.UpdateAttr, len(names))
	i = 0
	for n := range names {
		ua.Attributes[i] = centrald.UpdateAttr{}
		a := &ua.Attributes[i]
		a.Name = n
		for j := range a.Actions {
			a.Actions[j].Fields = map[string]struct{}{}
			a.Actions[j].Indexes = map[int]struct{}{}
		}
		i++
	}
	for _, kV := range unsetResV.MapKeys() {
		parts := strings.Split(kV.String(), ".")
		assert.Len(parts, 2)
		var attrIdx int
		var attrName string
		for j, attr := range ua.Attributes {
			if attr.Name == parts[0] {
				attrName = attr.Name
				attrIdx = j
				break
			}
		}
		assert.NotNil(attrName)
		ua.Attributes[attrIdx].Actions[centrald.UpdateRemove].FromBody = true
	}
	for _, kV := range pullResV.MapKeys() {
		var attrIdx int
		var attrName string
		for j, attr := range ua.Attributes {
			if attr.Name == kV.String() {
				attrName = attr.Name
				attrIdx = j
				break
			}
		}
		assert.NotNil(attrName)
		assert.False(ua.Attributes[attrIdx].Actions[centrald.UpdateRemove].FromBody)
		ua.Attributes[attrIdx].Actions[centrald.UpdateRemove].FromBody = true
	}

	t.Log("case: remove variants")
	for _, attr := range ua.Attributes {
		act := attr.Actions[centrald.UpdateRemove]
		t.Log("Attribute", attr.Name, "Action", act.FromBody, act.Fields, act.Indexes)
		assert.True(act.FromBody || len(act.Fields) > 0 || len(act.Indexes) > 0)
	}
	assert.NotPanics(func() { uq, err = createUpdateQuery(l, ua, param) })
	assert.Nil(err)
	assert.NotNil(uq)
	assert.Equal(res, uq)
	tl.Flush()

	// case: remove, invalid values
	param = &SomeType{
		SimpleAttr:      "SimpleAttr",
		SliceAttr:       []string{"elem1", "elem2"},
		ArrayAttr:       [3]int{1},
		StringTMap:      map[string]int{"a": 1, "b": 2},
		OtherMap:        map[int]string{1: "a", 2: "b"},
		StructAttr:      SA{"n", "v"},
		PtrToSimple:     swag.String("PtrToSimple"),
		PtrToStruct:     &SA{"N", "V"},
		PtrToStringTMap: &map[string]int{"A": 1, "B": 2},
		PtrToOtherMap:   &map[int]string{1: "A", 2: "B"},
		PtrToSlice:      &[]string{"Elem1", "Elem2"},
		PtrToArray:      &[3]int{2, 3, 5},
	}
	tcs = []struct {
		n, f string
		i    int
		eReg string
	}{
		{"foo", "", 0, "update 'foo': invalid name"},
		{"SimpleAttr", "", 0, "remove='SimpleAttr': invalid datatype for operation$"},
		{"PtrToSimple", "", 0, "remove='PtrToSimple': invalid datatype for operation$"},
		{"OtherMap", "", 0, "remove='OtherMap': invalid datatype for operation$"},
		{"PtrToOtherMap", "", 0, "remove='PtrToOtherMap': invalid datatype for operation$"},
		{"StructAttr", "", 0, "remove='StructAttr': invalid datatype for operation$"},
		{"PtrToStruct", "", 0, "remove='PtrToStruct': invalid datatype for operation$"},

		{"StringTMap", "x", 0, "remove='StringTMap': subfield not supported for operation$"},
		{"SliceAttr", "", 1, "remove='SliceAttr': subfield not supported for operation$"},
	}
	for _, tc := range tcs {
		ua = &centrald.UpdateArgs{}
		ua.Attributes = make([]centrald.UpdateAttr, 1)
		a := &ua.Attributes[0]
		a.Name = tc.n
		a.Actions[centrald.UpdateRemove].Fields = map[string]struct{}{}
		a.Actions[centrald.UpdateRemove].Indexes = map[int]struct{}{}
		if tc.f != "" {
			a.Actions[centrald.UpdateRemove].Fields[tc.f] = es
		} else if tc.i != 0 {
			a.Actions[centrald.UpdateRemove].Indexes[tc.i] = es
		} else {
			a.Actions[centrald.UpdateRemove].FromBody = true
		}
		t.Log("case: append errors", tc)
		assert.NotPanics(func() { uq, err = createUpdateQuery(l, ua, param) })
		assert.NotNil(err)
		assert.Nil(uq)
		assert.Regexp(tc.eReg, err.Error())
		assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, err.Error())
		tl.Flush()
	}
}

func TestCreateAggregationGroup(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	type ST struct {
		NotUsed int
		Sum     []string
	}

	// success
	st := ST{
		Sum: []string{"field1", "field2.subfield", "field3.subField-with-dash.subField"},
	}
	exp := bson.M{
		"$group": bson.M{
			"_id":   nil,
			"count": bson.M{"$sum": 1},
			"sum0":  bson.M{"$sum": "$" + st.Sum[0]},
			"sum1":  bson.M{"$sum": "$" + st.Sum[1]},
			"sum2":  bson.M{"$sum": "$" + strings.ToLower(st.Sum[2])},
		},
	}
	agg, err := createAggregationGroup(l, &st)
	if assert.NoError(err) && assert.NotNil(agg) {
		assert.Equal(exp, agg)
	}

	// success if you pass the struct instead of the pointer
	agg, err = createAggregationGroup(l, st)
	if assert.NoError(err) && assert.NotNil(agg) {
		assert.Equal(exp, agg)
	}

	// success with empty (nil) array
	agg, err = createAggregationGroup(l, &ST{})
	if assert.NoError(err) && assert.NotNil(agg) {
		exp = bson.M{
			"$group": bson.M{
				"_id":   nil,
				"count": bson.M{"$sum": 1},
			},
		}
		assert.Equal(exp, agg)
	}

	// error case: invalid field names
	agg, err = createAggregationGroup(l, &ST{Sum: []string{"field$"}})
	if assert.Error(err) {
		assert.Regexp("invalid aggregate field path", err.Error())
	}
	agg, err = createAggregationGroup(l, &ST{Sum: []string{"valid", "", "valid"}})
	if assert.Error(err) {
		assert.Regexp("invalid aggregate field path", err.Error())
	}
	agg, err = createAggregationGroup(l, &ST{Sum: []string{"too..many.dots"}})
	if assert.Error(err) {
		assert.Regexp("invalid aggregate field path", err.Error())
	}
	agg, err = createAggregationGroup(l, &ST{Sum: []string{".initialdot"}})
	if assert.Error(err) {
		assert.Regexp("invalid aggregate field path", err.Error())
	}
	agg, err = createAggregationGroup(l, &ST{Sum: []string{"trailingdot."}})
	if assert.Error(err) {
		assert.Regexp("invalid aggregate field path", err.Error())
	}

	// panic case: not a struct
	assert.Panics(func() { createAggregationGroup(l, int64(42)) })

	// panic case: no Sum field
	type BT1 struct {
		NoSum int
	}
	assert.Panics(func() { createAggregationGroup(l, &BT1{}) })

	// panic case: Sum field has the wrong type
	type BT2 struct {
		Sum string
	}
	assert.Panics(func() { createAggregationGroup(l, &BT2{}) })
}

func TestCreateAggregationList(t *testing.T) {
	assert := assert.New(t)

	// normal case
	fields := []string{"a", "b"}
	result := bson.M{"sum0": int64(32), "sum1": 29}
	ag := createAggregationList(fields, result)
	exp := []*centrald.Aggregation{
		&centrald.Aggregation{FieldPath: "a", Type: "sum", Value: 32},
		&centrald.Aggregation{FieldPath: "b", Type: "sum", Value: 29},
	}
	assert.Equal(exp, ag)

	// missing or invalid fields convert to 0
	ag = createAggregationList(fields, bson.M{})
	exp = []*centrald.Aggregation{
		&centrald.Aggregation{FieldPath: "a", Type: "sum", Value: 0},
		&centrald.Aggregation{FieldPath: "b", Type: "sum", Value: 0},
	}
	assert.Equal(exp, ag)

	assert.Empty(createAggregationList([]string{}, bson.M{}))
}

func TestAndOrList(t *testing.T) {
	assert := assert.New(t)

	list := newAndOrList()
	assert.NotNil(list)
	assert.Empty(list.orParamDocs)

	// Empty list results in no change to qParams
	qParams := bson.M{}
	list.setQueryParam(qParams)
	assert.Empty(qParams)

	// append a single list of documents, qParams["$or"] is set
	list.append(
		bson.M{"key": "value"},
		bson.M{"key": "another value"},
	)
	exp := bson.M{
		"$or": []bson.M{
			bson.M{"key": "value"},
			bson.M{"key": "another value"},
		},
	}
	assert.Len(list.orParamDocs, 1)
	list.setQueryParam(qParams)
	assert.Equal(exp, qParams)

	// append more then 1 list, qParams["$and" is set]
	qParams = bson.M{}
	list.append(
		bson.M{"key2": "value"},
		bson.M{"key2": "another value"},
	)
	assert.Len(list.orParamDocs, 2)
	exp = bson.M{
		"$and": []bson.M{
			bson.M{
				"$or": []bson.M{
					bson.M{"key": "value"},
					bson.M{"key": "another value"},
				},
			},
			bson.M{
				"$or": []bson.M{
					bson.M{"key2": "value"},
					bson.M{"key2": "another value"},
				},
			},
		},
	}
	list.setQueryParam(qParams)
	assert.Equal(exp, qParams)
}

// cmpObjToModel validates that a database object matches a model object.
// Field exceptions are described in fMap, a map of model fields to corresponding fields in
// the object.
func cmpObjToModel(t *testing.T, o interface{}, mObj interface{}, fMap map[string]string) {
	assert := assert.New(t)
	bsonKey := func(sf reflect.StructField) string {
		bson := sf.Tag.Get("bson")
		parts := strings.Split(bson, ",")
		return parts[0]
	}
	isLower := func(s string) bool {
		return s == strings.ToLower(s)
	}
	oV := reflect.ValueOf(o)
	mObjV := reflect.ValueOf(mObj)
	assert.True(oV.Kind() == reflect.Struct)
	assert.True(mObjV.Kind() == reflect.Struct)
	oT := oV.Type()
	mObjT := mObjV.Type()
	for i := 0; i < mObjT.NumField(); i++ {
		mObjSF := mObjT.Field(i)
		t.Log("Model field", mObjSF)
		if mObjSF.Anonymous { // embedded struct
			cmpObjToModel(t, o, mObjV.Field(i).Interface(), fMap)
			continue
		}
		oSF, ok := oT.FieldByName(mObjSF.Name)
		if ok {
			t.Log("Model field", mObjSF.Name, "present in object")
			bKey := bsonKey(oSF)
			if bKey != "" {
				ok = isLower(bKey)
				if !ok {
					assert.True(ok, "Model field "+mObjSF.Name+" BSON='"+bKey+"' case is not lower")
				}
			}
			continue
		}
		if fMap == nil {
			assert.True(ok, "model field '%s' should be in database object", mObjSF.Name)
			continue
		}
		n, ok := fMap[mObjSF.Name]
		if !ok {
			assert.True(ok, "model field '%s' should be in database object or be remapped", mObjSF.Name)
			continue
		}
		if n == "" {
			// ignore missing model field in database object
			t.Log("Ignoring missing model field", mObjSF.Name)
			continue
		}
		_, ok = oT.FieldByName(n)
		if !ok {
			assert.True(ok, "model field '%s' should be mapped to object field", mObjSF.Name, n)
			continue
		}
		t.Log("Model field", mObjSF.Name, "is mapped to object field", n)
	}
	// Bson marshal can fail if we have embedded Go data structures with conflicting keys.
	_, err := bson.Marshal(o)
	assert.NoError(err)
}

type fakePopulator struct {
	cName string

	popCount  int
	inCtx     context.Context
	inPopPath string
	inPopBuf  []byte
	retPopErr error
}

func (fp *fakePopulator) CName() string {
	return fp.cName
}

func (fp *fakePopulator) Populate(ctx context.Context, path string, buf []byte) error {
	fp.popCount++
	fp.inCtx = ctx
	fp.inPopPath, fp.inPopBuf = path, buf
	return fp.retPopErr
}
