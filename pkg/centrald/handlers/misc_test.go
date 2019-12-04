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


package handlers

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	spa "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/service_plan_allocation"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	fal "github.com/Nuvoloso/kontroller/pkg/centrald/audit/fake"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	"github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	fhk "github.com/Nuvoloso/kontroller/pkg/housekeeping/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

// updateArgsMatcher creates a mockUpdateArgsMatcher - original code moved to mock
func updateArgsMatcher(t *testing.T, ua *centrald.UpdateArgs) *mock.UpdateArgsMatcher {
	return mock.NewUpdateArgsMatcher(t, ua)
}

func TestErrorHelpers(t *testing.T) {
	assert := assert.New(t)
	ge := fmt.Errorf("generic error")
	cde := &centrald.Error{M: "centrald error", C: 400}
	msg := "not an error"

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()

	assert.Equal(500, hc.eCode(ge))
	assert.Equal(cde.C, hc.eCode(cde))
	assert.Equal(500, hc.eCode(msg))

	geM := hc.eError(ge)
	assert.Equal(ge.Error(), *geM.Message)
	assert.Equal(hc.eCode(ge), int(geM.Code))

	cdeM := hc.eError(cde)
	assert.Equal(cde.Error(), *cdeM.Message)
	assert.Equal(hc.eCode(cde), int(cdeM.Code))

	msgM := hc.eError(msg)
	assert.Equal(msg, *msgM.Message)
	assert.Equal(hc.eCode(msg), int(msgM.Code))

	err := hc.eMissingMsg("helper %s", "eMissingMsg")
	assert.Error(err)
	assert.Regexp("^"+centrald.ErrorMissing.M+".*helper eMissingMsg", err)
	e, ok := err.(*centrald.Error)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, e.C)

	err = hc.eErrorNotFound("helper %s", "eErrorNotFound")
	assert.Error(err)
	assert.Regexp("^"+centrald.ErrorNotFound.M+".*helper eErrorNotFound", err)
	e, ok = err.(*centrald.Error)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, e.C)

	err = hc.eExists("helper %s", "eExists")
	assert.Error(err)
	assert.Regexp("^"+centrald.ErrorExists.M+".*helper eExists", err)
	e, ok = err.(*centrald.Error)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, e.C)

	err = hc.eInternalError("helper %s", "eInternalError")
	assert.Error(err)
	assert.Regexp("^"+centrald.ErrorInternalError.M+".*helper eInternalError", err)
	e, ok = err.(*centrald.Error)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, e.C)

	err = hc.eInvalidData("helper %s", "eInvalidData")
	assert.Error(err)
	assert.Regexp("^"+centrald.ErrorInvalidData.M+".*helper eInvalidData", err)
	e, ok = err.(*centrald.Error)
	assert.True(ok)
	assert.Equal(centrald.ErrorInvalidData.C, e.C)

	err = hc.eInvalidState("helper %s", "eInvalidState")
	assert.Error(err)
	assert.Regexp("^"+centrald.ErrorInvalidState.M+".*helper eInvalidState", err)
	e, ok = err.(*centrald.Error)
	assert.True(ok)
	assert.Equal(centrald.ErrorInvalidState.C, e.C)

	err = hc.eUpdateInvalidMsg("helper %s", "eUpdateInvalidMsg")
	assert.Error(err)
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M+".*helper eUpdateInvalidMsg", err)
	e, ok = err.(*centrald.Error)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, e.C)

	err = hc.eRequestInConflict("helper %s", "eRequestInConflict")
	assert.Error(err)
	assert.Regexp("^"+centrald.ErrorRequestInConflict.M+".*helper eRequestInConflict", err)
	e, ok = err.(*centrald.Error)
	assert.True(ok)
	assert.Equal(centrald.ErrorRequestInConflict.C, e.C)

	err = hc.eUnauthorizedOrForbidden("helper %s", "eUnauthorizedOrForbidden")
	assert.Error(err)
	assert.Regexp("^"+centrald.ErrorUnauthorizedOrForbidden.M+".*helper eUnauthorizedOrForbidden", err)
	e, ok = err.(*centrald.Error)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, e.C)
}

func TestJSONToAttrNameMap(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()

	type NestedDepth2 struct {
		Nested2           int            `json:"nested2"`
		Nested2StringTMap map[string]int `json:"nested2stringtmap"`
	}
	type NestedDepth1 struct {
		Nested1           string         `json:"nested1"`
		Nested1StringTMap map[string]int `json:"nested1stringtmap"`
		NestedDepth2
	}
	type NamedStruct struct {
		FieldOne string `json:"fieldOne"`
		FieldTwo int    `json:"fieldTwo"`
	}
	type SomeType struct {
		StringAttr  string `json:"stringattr"`
		IntAttr     int    `json:"intattr"`
		IgnoredAttr string
		Tags        []string       `json:"tags"`
		StringTMap  map[string]int `json:"stringtmap"`
		OtherMap    map[int]string `json:"othermap"`
		StructAttr  struct {       // anonymous
			N string `json:"n"`
			V string `json:"v"`
		} `json:"structattr"`
		StructAttrN     NamedStruct     `json:"structattrN"`
		RenamedAttr     int             `json:"foobar"`
		PtrToString     *string         `json:"ptrtostring"`
		PtrToStruct     *struct{}       `json:"ptrtostruct"`
		PtrToStructN    *NamedStruct    `json:"ptrtostructN"`
		PtrToStringTMap *map[string]int `json:"ptrtostringtmap"`
		PtrToOtherMap   *map[int]int    `json:"ptrtoothermap"`
		PtrToArray      *[]string       `json:"ptrtoarray"`
		NestedDepth1
	}
	expKeys := []string{
		"stringattr",
		"intattr",
		"tags",
		"stringtmap",
		"othermap",
		"structattr",
		"structattrN",
		"foobar",
		"ptrtostring",
		"ptrtostruct",
		"ptrtostructN",
		"ptrtostringtmap",
		"ptrtoothermap",
		"ptrtoarray",
		"nested1",
		"nested2",
		"nested1stringtmap",
		"nested2stringtmap",
	}
	nMap := hc.makeJSONToAttrNameMap(SomeType{})
	assert.Len(nMap, len(expKeys))
	for _, k := range expKeys {
		assert.Contains(nMap, k)
	}

	assert.False(nMap.isStruct("stringattr"))
	assert.False(nMap.isArray("stringattr"))
	assert.False(nMap.isStringTMap("stringattr"))

	assert.False(nMap.isStruct("ptrtostring"))
	assert.False(nMap.isArray("ptrtostring"))
	assert.False(nMap.isStringTMap("ptrtostring"))

	assert.True(nMap.isArray("tags"))
	assert.False(nMap.isStringTMap("tags"))
	assert.False(nMap.isStruct("tags"))

	assert.True(nMap.isArray("ptrtoarray"))
	assert.False(nMap.isStringTMap("ptrtoarray"))
	assert.False(nMap.isStruct("ptrtoarray"))

	assert.True(nMap.isStringTMap("stringtmap"))
	assert.False(nMap.isArray("stringtmap"))
	assert.False(nMap.isStruct("stringtmap"))

	assert.True(nMap.isStringTMap("ptrtostringtmap"))
	assert.False(nMap.isArray("ptrtostringtmap"))
	assert.False(nMap.isStruct("ptrtostringtmap"))

	assert.False(nMap.isStringTMap("othermap"))
	assert.False(nMap.isArray("othermap"))
	assert.False(nMap.isStruct("othermap"))

	assert.False(nMap.isStringTMap("ptrtoothermap"))
	assert.False(nMap.isArray("ptrtoothermap"))
	assert.False(nMap.isStruct("ptrtoothermap"))

	assert.True(nMap.isStruct("structattr"))
	assert.False(nMap.isArray("structattr"))
	assert.False(nMap.isStringTMap("structattr"))
	b, sT := nMap.isStruct("structattr")
	assert.True(b)
	assert.Len(hc.structNameCache, 0)
	cntRLock := hc.cntRLock
	cntLock := hc.cntLock
	fMap := hc.getStructNameMap(sT)
	assert.NotNil(fMap)
	assert.Len(fMap, 2)
	assert.Len(hc.structNameCache, 0) // anonymous
	assert.True(hc.cntRLock == cntRLock)
	assert.True(hc.cntLock == cntLock)
	expFields := []string{"n", "v"}
	for _, f := range expFields {
		assert.Contains(fMap, f)
	}

	assert.True(nMap.isStruct("structattrN"))
	assert.False(nMap.isArray("structattrN"))
	assert.False(nMap.isStringTMap("structattrN"))
	b, sT = nMap.isStruct("structattrN")
	assert.True(b)
	assert.Len(hc.structNameCache, 0)
	fMap = hc.getStructNameMap(sT)
	assert.NotNil(fMap)
	assert.Len(fMap, 2)
	assert.Len(hc.structNameCache, 1)
	expFields = []string{"fieldOne", "fieldTwo"}
	for _, f := range expFields {
		assert.Contains(fMap, f)
	}
	newFMap := hc.getStructNameMap(sT)
	assert.NotNil(newFMap)
	assert.Equal(fMap, newFMap)
	assert.Len(hc.structNameCache, 1)

	assert.True(nMap.isStruct("ptrtostruct"))
	assert.False(nMap.isArray("ptrtostruct"))
	assert.False(nMap.isStringTMap("ptrtostruct"))
	b, sT = nMap.isStruct("ptrtostruct")
	assert.True(b)
	assert.Len(hc.structNameCache, 1)
	fMap = hc.getStructNameMap(sT)
	assert.NotNil(fMap)
	assert.Len(fMap, 0)

	assert.True(nMap.isStruct("ptrtostructN"))
	assert.False(nMap.isArray("ptrtostructN"))
	assert.False(nMap.isStringTMap("ptrtostructN"))
	b, sT = nMap.isStruct("ptrtostructN")
	assert.True(b)
	assert.Len(hc.structNameCache, 1)
	fMap = hc.getStructNameMap(sT)
	assert.NotNil(fMap)
	assert.Len(fMap, 2)
	expFields = []string{"fieldOne", "fieldTwo"}
	for _, f := range expFields {
		assert.Contains(fMap, f)
	}

	assert.False(nMap.isStruct("nested1"))
	assert.False(nMap.isArray("nested1"))
	assert.False(nMap.isStringTMap("nested1"))

	assert.False(nMap.isStruct("nested2"))
	assert.False(nMap.isArray("nested2"))
	assert.False(nMap.isStringTMap("nested2"))

	assert.True(nMap.isStringTMap("nested1stringtmap"))
	assert.False(nMap.isArray("nested1stringtmap"))
	assert.False(nMap.isStruct("nested1stringtmap"))

	assert.True(nMap.isStringTMap("nested2stringtmap"))
	assert.False(nMap.isArray("nested2stringtmap"))
	assert.False(nMap.isStruct("nested2stringtmap"))

	assert.NotPanics(func() { nMap.jName("StructAttr") })
	assert.Panics(func() { nMap.jName("StructAttr.n") })
}

// TestSomeModelTypes is vulnerable to model changes and should be updated as necessary
func TestSomeModelTypes(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()

	// check a map[string]T property
	nMap := hc.makeJSONToAttrNameMap(models.VolumeSeries{})
	assert.NotNil(nMap)
	sf, ok := nMap["storageParcels"]
	assert.True(ok)
	assert.Equal(reflect.Map, sf.Type.Kind())
	assert.Equal(reflect.String, sf.Type.Key().Kind())
	assert.True(nMap.isStringTMap("storageParcels"))

	// check [] property
	nMap = hc.makeJSONToAttrNameMap(models.VolumeSeriesMutable{})
	assert.NotNil(nMap)
	sf, ok = nMap["tags"]
	assert.True(ok)
	assert.Condition(func() bool { k := sf.Type.Kind(); return k == reflect.Array || k == reflect.Slice })
	assert.True(nMap.isArray("tags"))

	// check struct property
	nMap = hc.makeJSONToAttrNameMap(models.StorageMutable{})
	assert.NotNil(nMap)
	t.Log(nMap)
	sf, ok = nMap["storageState"]
	assert.True(ok)
	assert.Equal(reflect.Ptr, sf.Type.Kind())
	assert.Equal(reflect.Struct, sf.Type.Elem().Kind())
	assert.True(nMap.isStruct("storageState"))
}

// TestUpdateEmptyCases tests the simple update cases
func TestUpdateEmptyCases(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()

	type SomeType struct {
		SimpleAttr string         `json:"simpleattr"`
		ArrayAttr  []string       `json:"arrayattr"`
		StringTMap map[string]int `json:"stringtmap"`
		OtherMap   map[int]string `json:"othermap"`
		StructAttr struct {
			N string
			V string
		} `json:"structattr"`
		PtrToSimple     *string          `json:"ptrtosimple"`
		PtrToStruct     *struct{ V int } `json:"ptrtostruct"`
		PtrToStringTMap *map[string]int  `json:"ptrtostringtmap"`
		PtrToOtherMap   *map[int]int     `json:"ptrtoothermap"`
		PtrToArray      *[]string        `json:"ptrtoarray"`
	}
	nMap := hc.makeJSONToAttrNameMap(SomeType{})
	nMapI := make(map[string]string, len(nMap)) // field name  => json name
	for k, sf := range nMap {
		nMapI[sf.Name] = k
	}
	assert.Equal(len(nMap), len(nMapI))
	id := "aada2615-4819-4c26-b8ec-8579da815291"
	version := swag.Int32(2)

	// case: nothing provided
	ua, err := hc.makeStdUpdateArgs(JSONToAttrNameMap{}, id, nil, [3][]string{})
	assert.NotNil(err)
	assert.Equal(centrald.ErrorUpdateInvalidRequest, err)
	assert.NotNil(ua)
	assert.False(ua.HasChanges)
	assert.Equal(int32(0), ua.Version)
	assert.Equal(id, ua.ID)
	assert.Equal(0, len(ua.Attributes))

	// case: attribute and version but no parameters
	ua, err = hc.makeStdUpdateArgs(nMap, id, version, [3][]string{})
	assert.NotNil(err)
	assert.Equal(centrald.ErrorUpdateInvalidRequest, err)
	assert.NotNil(ua)
	assert.False(ua.HasChanges)
	assert.Equal(*version, ua.Version)
	assert.Equal(id, ua.ID)
	assert.Equal(len(nMap), len(ua.Attributes))
	for _, a := range ua.Attributes {
		assert.Contains(nMapI, a.Name)
		for _, aa := range a.Actions {
			assert.False(aa.FromBody)
			assert.Empty(aa.Fields)
			assert.Empty(aa.Indexes)
		}
	}
}

// TestUpdateRemoveIsolated tests the remove update query param
func TestUpdateRemoveIsolated(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()

	type SomeType struct {
		SimpleAttr string         `json:"simpleattr"`
		ArrayAttr  []string       `json:"arrayattr"`
		StringTMap map[string]int `json:"stringtmap"`
		OtherMap   map[int]string `json:"othermap"`
		StructAttr struct {
			N string
			V string
		} `json:"structattr"`
		PtrToSimple     *string          `json:"ptrtosimple"`
		PtrToStruct     *struct{ V int } `json:"ptrtostruct"`
		PtrToStringTMap *map[string]int  `json:"ptrtostringtmap"`
		PtrToOtherMap   *map[int]int     `json:"ptrtoothermap"`
		PtrToArray      *[]string        `json:"ptrtoarray"`
	}
	nMap := hc.makeJSONToAttrNameMap(SomeType{})
	nMapI := make(map[string]string, len(nMap)) // field name  => json name
	for k, sf := range nMap {
		nMapI[sf.Name] = k
	}
	assert.Equal(len(nMap), len(nMapI))
	id := "aada2615-4819-4c26-b8ec-8579da815291"
	version := swag.Int32(2)

	// case: remove, valid forms only
	type MSS map[string]struct{}
	type MIS map[int]struct{}
	type expValues struct {
		fb bool
		f  MSS
		i  MIS
	}
	params := [3][]string{
		centrald.UpdateRemove: []string{
			nMapI["ArrayAttr"],
			nMapI["PtrToArray"],
			nMapI["PtrToStringTMap"],
			nMapI["StringTMap"],
		},
	}
	expRV := map[string]expValues{
		"ArrayAttr":       {fb: true, f: MSS{}, i: MIS{}},
		"PtrToArray":      {fb: true, f: MSS{}, i: MIS{}},
		"StringTMap":      {fb: true, f: MSS{}, i: MIS{}},
		"PtrToStringTMap": {fb: true, f: MSS{}, i: MIS{}},
	}
	ua, err := hc.makeStdUpdateArgs(nMap, id, version, params)
	assert.Nil(err)
	assert.NotNil(ua)
	for _, a := range ua.Attributes {
		assert.Contains(nMapI, a.Name)
		for i, aa := range a.Actions {
			if i == int(centrald.UpdateRemove) {
				t.Log(a.Name, aa)
				if ev, ok := expRV[a.Name]; ok {
					assert.Equal(ev.fb, aa.FromBody)
					assert.Equal(ev.f, MSS(aa.Fields))
					assert.Equal(ev.i, MIS(aa.Indexes))
				} else {
					assert.False(aa.FromBody)
					assert.Empty(aa.Fields)
					assert.Empty(aa.Indexes)
				}
			} else {
				assert.False(aa.FromBody)
				assert.Empty(aa.Fields)
				assert.Empty(aa.Indexes)
			}
		}
	}

	// case: test gomock ua matcher
	ua2, err := hc.makeStdUpdateArgs(nMap, id, version, params)
	assert.Nil(err)
	assert.NotNil(ua2)
	uaM := updateArgsMatcher(t, ua).Matcher()
	assert.True(uaM.Matches(ua2))

	// case: parsing error cases
	remErrCases := []struct{ p, eReg string }{
		{"badattr", "remove='badattr': invalid name$"},
		{"badattr.A", "remove='badattr.A': invalid name$"},
		{"badattr.A.B", "remove='badattr.A.B': invalid name format$"},

		{nMapI["SimpleAttr"], "remove='simpleattr': invalid datatype$"},
		{nMapI["SimpleAttr"] + ".0", "remove='simpleattr.0': subfield not supported for datatype$"},
		{nMapI["PtrToSimple"], "remove='ptrtosimple': invalid datatype$"},
		{nMapI["PtrToSimple"] + ".X", "remove='ptrtosimple.X': subfield not supported for datatype$"},

		{nMapI["ArrayAttr"] + ".Foo", "remove='arrayattr.Foo': invalid index$"},
		{nMapI["ArrayAttr"] + ".1", "remove='arrayattr.1': index not permitted$"},
		{nMapI["ArrayAttr"] + ".-1", "remove='arrayattr.-1': invalid index$"},
		{nMapI["PtrToArray"] + ".Foo", "remove='ptrtoarray.Foo': invalid index$"},
		{nMapI["PtrToArray"] + ".0", "remove='ptrtoarray.0': index not permitted$"},
		{nMapI["PtrToArray"] + ".-1", "remove='ptrtoarray.-1': invalid index$"},

		{nMapI["StringTMap"] + ".foo", "remove='stringtmap.foo': field not permitted$"},
		{nMapI["PtrToStringTMap"] + ".foo", "remove='ptrtostringtmap.foo': field not permitted$"},

		{nMapI["OtherMap"], "remove='othermap': invalid datatype$"},
		{nMapI["OtherMap"] + ".Z", "remove='othermap.Z': subfield not supported for datatype$"},
		{nMapI["PtrToOtherMap"], "remove='ptrtoothermap': invalid datatype$"},
		{nMapI["PtrToOtherMap"] + ".Z", "remove='ptrtoothermap.Z': subfield not supported for datatype$"},

		{nMapI["StructAttr"], "remove='structattr': invalid datatype$"},
		{nMapI["StructAttr"] + ".foo", "remove='structattr.foo': invalid field$"},
		{nMapI["PtrToStruct"], "remove='ptrtostruct': invalid datatype$"},
		{nMapI["PtrToStruct"] + ".foo", "remove='ptrtostruct.foo': invalid field$"},
	}
	for _, tc := range remErrCases {
		params = [3][]string{
			centrald.UpdateRemove: []string{tc.p},
		}
		t.Log(tc)
		ua, err = hc.makeStdUpdateArgs(nMap, id, version, params)
		assert.NotNil(err)
		assert.Nil(ua)
		assert.Regexp(tc.eReg, err.Error())
		assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, err.Error())
	}
}

// TestUpdateAppendIsolated is identical to TestUpdateRemove but for append
func TestUpdateAppendIsolated(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()

	type SomeType struct {
		SimpleAttr string         `json:"simpleattr"`
		ArrayAttr  []string       `json:"arrayattr"`
		StringTMap map[string]int `json:"stringtmap"`
		OtherMap   map[int]string `json:"othermap"`
		StructAttr struct {
			N string
			V string
		} `json:"structattr"`
		PtrToSimple     *string          `json:"ptrtosimple"`
		PtrToStruct     *struct{ V int } `json:"ptrtostruct"`
		PtrToStringTMap *map[string]int  `json:"ptrtostringtmap"`
		PtrToOtherMap   *map[int]int     `json:"ptrtoothermap"`
		PtrToArray      *[]string        `json:"ptrtoarray"`
	}
	nMap := hc.makeJSONToAttrNameMap(SomeType{})
	nMapI := make(map[string]string, len(nMap)) // field name  => json name
	for k, sf := range nMap {
		nMapI[sf.Name] = k
	}
	assert.Equal(len(nMap), len(nMapI))
	id := "aada2615-4819-4c26-b8ec-8579da815291"
	version := swag.Int32(2)

	// case: append, valid forms only
	type MSS map[string]struct{}
	type MIS map[int]struct{}
	type expValues struct {
		fb bool
		f  MSS
		i  MIS
	}
	params := [3][]string{
		centrald.UpdateAppend: []string{
			nMapI["ArrayAttr"],
			nMapI["PtrToArray"],
			nMapI["PtrToStringTMap"],
			nMapI["StringTMap"],
		},
	}
	expRV := map[string]expValues{
		"ArrayAttr":       {fb: true, f: MSS{}, i: MIS{}},
		"PtrToArray":      {fb: true, f: MSS{}, i: MIS{}},
		"StringTMap":      {fb: true, f: MSS{}, i: MIS{}},
		"PtrToStringTMap": {fb: true, f: MSS{}, i: MIS{}},
	}
	ua, err := hc.makeStdUpdateArgs(nMap, id, version, params)
	assert.Nil(err)
	assert.NotNil(ua)
	for _, a := range ua.Attributes {
		assert.Contains(nMapI, a.Name)
		for i, aa := range a.Actions {
			if i == int(centrald.UpdateAppend) {
				t.Log(a.Name, aa)
				if ev, ok := expRV[a.Name]; ok {
					assert.Equal(ev.fb, aa.FromBody)
					assert.Equal(ev.f, MSS(aa.Fields))
					assert.Equal(ev.i, MIS(aa.Indexes))
				} else {
					assert.False(aa.FromBody)
					assert.Empty(aa.Fields)
					assert.Empty(aa.Indexes)
				}
			} else {
				assert.False(aa.FromBody)
				assert.Empty(aa.Fields)
				assert.Empty(aa.Indexes)
			}
		}
	}

	// case: test gomock ua matcher
	ua2, err := hc.makeStdUpdateArgs(nMap, id, version, params)
	assert.Nil(err)
	assert.NotNil(ua2)
	uaM := updateArgsMatcher(t, ua).Matcher()
	assert.True(uaM.Matches(ua2))

	// case: parsing error cases
	appErrCases := []struct{ p, eReg string }{
		{"badattr", "append='badattr': invalid name$"},
		{"badattr.A", "append='badattr.A': invalid name$"},
		{"badattr.A.B", "append='badattr.A.B': invalid name format$"},

		{nMapI["SimpleAttr"], "append='simpleattr': invalid datatype$"},
		{nMapI["SimpleAttr"] + ".0", "append='simpleattr.0': subfield not supported for datatype$"},
		{nMapI["PtrToSimple"], "append='ptrtosimple': invalid datatype$"},
		{nMapI["PtrToSimple"] + ".X", "append='ptrtosimple.X': subfield not supported for datatype$"},

		{nMapI["ArrayAttr"] + ".Foo", "append='arrayattr.Foo': invalid index$"},
		{nMapI["ArrayAttr"] + ".1", "append='arrayattr.1': index not permitted$"},
		{nMapI["ArrayAttr"] + ".-1", "append='arrayattr.-1': invalid index$"},
		{nMapI["PtrToArray"] + ".Foo", "append='ptrtoarray.Foo': invalid index$"},
		{nMapI["PtrToArray"] + ".0", "append='ptrtoarray.0': index not permitted$"},
		{nMapI["PtrToArray"] + ".-1", "append='ptrtoarray.-1': invalid index$"},

		{nMapI["StringTMap"] + ".foo", "append='stringtmap.foo': field not permitted$"},
		{nMapI["PtrToStringTMap"] + ".foo", "append='ptrtostringtmap.foo': field not permitted$"},

		{nMapI["OtherMap"], "append='othermap': invalid datatype$"},
		{nMapI["OtherMap"] + ".Z", "append='othermap.Z': subfield not supported for datatype$"},
		{nMapI["PtrToOtherMap"], "append='ptrtoothermap': invalid datatype$"},
		{nMapI["PtrToOtherMap"] + ".Z", "append='ptrtoothermap.Z': subfield not supported for datatype$"},

		{nMapI["StructAttr"], "append='structattr': invalid datatype$"},
		{nMapI["StructAttr"] + ".foo", "append='structattr.foo': invalid field$"},
		{nMapI["PtrToStruct"], "append='ptrtostruct': invalid datatype$"},
		{nMapI["PtrToStruct"] + ".foo", "append='ptrtostruct.foo': invalid field$"},
	}
	for _, tc := range appErrCases {
		params = [3][]string{
			centrald.UpdateAppend: []string{tc.p},
		}
		t.Log(tc)
		ua, err = hc.makeStdUpdateArgs(nMap, id, version, params)
		assert.NotNil(err)
		assert.Nil(ua)
		assert.Regexp(tc.eReg, err.Error())
		assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, err.Error())
	}
}

// TestUpdateSetIsolated is almost identical to TestUpdateRemove but for set and with more valid cases
func TestUpdateSetIsolated(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()

	type SomeType struct {
		SimpleAttr string         `json:"simpleattr"`
		ArrayAttr  []string       `json:"arrayattr"`
		StringTMap map[string]int `json:"stringtmap"`
		OtherMap   map[int]string `json:"othermap"`
		StructAttr struct {
			N string
			V string
		} `json:"structattr"`
		PtrToSimple *string `json:"ptrtosimple"`
		PtrToStruct *struct {
			V int `json:"v"`
		} `json:"ptrtostruct"`
		PtrToStringTMap *map[string]int `json:"ptrtostringtmap"`
		PtrToOtherMap   *map[int]int    `json:"ptrtoothermap"`
		PtrToArray      *[]string       `json:"ptrtoarray"`
	}
	nMap := hc.makeJSONToAttrNameMap(SomeType{})
	nMapI := make(map[string]string, len(nMap)) // field name  => json name
	for k, sf := range nMap {
		nMapI[sf.Name] = k
	}
	assert.Equal(len(nMap), len(nMapI))
	id := "aada2615-4819-4c26-b8ec-8579da815291"
	version := swag.Int32(2)

	// case: set, valid forms only
	type MSS map[string]struct{}
	type MIS map[int]struct{}
	type expValues struct {
		fb bool
		f  MSS
		i  MIS
	}
	es := struct{}{}
	params := [3][]string{
		centrald.UpdateSet: []string{
			nMapI["ArrayAttr"],
			nMapI["OtherMap"],
			nMapI["PtrToArray"] + ".0",
			nMapI["PtrToOtherMap"],
			nMapI["PtrToSimple"],
			nMapI["PtrToStringTMap"] + ".bar",
			nMapI["PtrToStruct"] + ".v",
			nMapI["SimpleAttr"],
			nMapI["StringTMap"],
			nMapI["StructAttr"],
		},
	}
	expRV := map[string]expValues{
		"ArrayAttr":       {fb: true, f: MSS{}, i: MIS{}},
		"OtherMap":        {fb: true, f: MSS{}, i: MIS{}},
		"PtrToArray":      {f: MSS{}, i: MIS{0: es}},
		"PtrToOtherMap":   {fb: true, f: MSS{}, i: MIS{}},
		"PtrToSimple":     {fb: true, f: MSS{}, i: MIS{}},
		"PtrToStringTMap": {f: MSS{"bar": es}, i: MIS{}},
		"PtrToStruct":     {f: MSS{"V": es}, i: MIS{}},
		"SimpleAttr":      {fb: true, f: MSS{}, i: MIS{}},
		"StringTMap":      {fb: true, f: MSS{}, i: MIS{}},
		"StructAttr":      {fb: true, f: MSS{}, i: MIS{}},
	}
	ua, err := hc.makeStdUpdateArgs(nMap, id, version, params)
	assert.Nil(err)
	assert.NotNil(ua)
	for _, a := range ua.Attributes {
		assert.Contains(nMapI, a.Name)
		for i, aa := range a.Actions {
			if i == int(centrald.UpdateSet) {
				t.Log(a.Name, aa)
				if ev, ok := expRV[a.Name]; ok {
					assert.Equal(ev.fb, aa.FromBody)
					assert.Equal(ev.f, MSS(aa.Fields))
					assert.Equal(ev.i, MIS(aa.Indexes))
				} else {
					assert.False(aa.FromBody)
					assert.Empty(aa.Fields)
					assert.Empty(aa.Indexes)
				}
			} else {
				assert.False(aa.FromBody)
				assert.Empty(aa.Fields)
				assert.Empty(aa.Indexes)
			}
		}
		tl.Flush()
	}

	// case: test gomock ua matcher
	ua2, err := hc.makeStdUpdateArgs(nMap, id, version, params)
	assert.Nil(err)
	assert.NotNil(ua2)
	uaM := updateArgsMatcher(t, ua).Matcher()
	assert.True(uaM.Matches(ua2))

	// case: parsing error cases
	setErrCases := []struct{ p, eReg string }{
		{"badattr", "set='badattr': invalid name$"},
		{"badattr.A", "set='badattr.A': invalid name$"},
		{"badattr.A.B", "set='badattr.A.B': invalid name format$"},

		{nMapI["SimpleAttr"] + ".0", "set='simpleattr.0': subfield not supported for datatype$"},
		{nMapI["PtrToSimple"] + ".X", "set='ptrtosimple.X': subfield not supported for datatype$"},

		{nMapI["ArrayAttr"] + ".Foo", "set='arrayattr.Foo': invalid index$"},
		{nMapI["ArrayAttr"] + ".-1", "set='arrayattr.-1': invalid index$"},
		{nMapI["PtrToArray"] + ".Foo", "set='ptrtoarray.Foo': invalid index$"},
		{nMapI["PtrToArray"] + ".-1", "set='ptrtoarray.-1': invalid index$"},

		{nMapI["OtherMap"] + ".Z", "set='othermap.Z': subfield not supported for datatype$"},
		{nMapI["PtrToOtherMap"] + ".Z", "set='ptrtoothermap.Z': subfield not supported for datatype$"},

		{nMapI["StructAttr"] + ".foo", "set='structattr.foo': invalid field$"},
		{nMapI["PtrToStruct"] + ".foo", "set='ptrtostruct.foo': invalid field$"},
	}
	for _, tc := range setErrCases {
		params = [3][]string{
			centrald.UpdateSet: []string{tc.p},
		}
		t.Log(tc)
		ua, err = hc.makeStdUpdateArgs(nMap, id, version, params)
		assert.NotNil(err)
		assert.Nil(ua)
		assert.Regexp(tc.eReg, err.Error())
		assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, err.Error())
		tl.Flush()
	}

	// case: validation error case
	params = [3][]string{
		centrald.UpdateSet: []string{
			nMapI["ArrayAttr"], nMapI["ArrayAttr"] + ".1",
		},
	}
	ua, err = hc.makeStdUpdateArgs(nMap, id, version, params)
	assert.NotNil(err)
	assert.Nil(ua)
	assert.Regexp("set='arrayattr': both 'name' and 'name.' forms present$", err.Error())
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, err.Error())
}

// TestUpdateMulti tests multiple operations together
func TestUpdateMulti(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()

	type SomeType struct {
		SimpleAttr string         `json:"simpleattr"`
		ArrayAttr  []string       `json:"arrayattr"`
		StringTMap map[string]int `json:"stringtmap"`
		OtherMap   map[int]string `json:"othermap"`
		StructAttr struct {
			N string `json:"n"`
			V string `json:"v"`
		} `json:"structattr"`
		PtrToSimple     *string          `json:"ptrtosimple"`
		PtrToStruct     *struct{ V int } `json:"ptrtostruct"`
		PtrToStringTMap *map[string]int  `json:"ptrtostringtmap"`
		PtrToOtherMap   *map[int]int     `json:"ptrtoothermap"`
		PtrToArray      *[]string        `json:"ptrtoarray"`
	}
	nMap := hc.makeJSONToAttrNameMap(SomeType{})
	nMapI := make(map[string]string, len(nMap)) // field name  => json name
	for k, sf := range nMap {
		nMapI[sf.Name] = k
	}
	assert.Equal(len(nMap), len(nMapI))
	id := "aada2615-4819-4c26-b8ec-8579da815291"
	version := swag.Int32(2)

	// case: multiple operations, no conflict
	type MSS map[string]struct{}
	type MIS map[int]struct{}
	type expValues struct {
		fb bool
		f  MSS
		i  MIS
	}
	params := [3][]string{
		centrald.UpdateRemove: []string{
			nMapI["StringTMap"],
		},
		centrald.UpdateAppend: []string{
			nMapI["ArrayAttr"],
		},
		centrald.UpdateSet: []string{
			nMapI["StructAttr"] + ".n",
			nMapI["StructAttr"] + ".v",
		},
	}
	es := struct{}{}
	expRV := [3]map[string]expValues{
		centrald.UpdateRemove: {
			"StringTMap": {fb: true, f: MSS{}, i: MIS{}},
		},
		centrald.UpdateAppend: {
			"ArrayAttr": {fb: true, f: MSS{}, i: MIS{}},
		},
		centrald.UpdateSet: {
			"StructAttr": {fb: false, f: MSS{"N": es, "V": es}, i: MIS{}},
		},
	}
	ua, err := hc.makeStdUpdateArgs(nMap, id, version, params)
	assert.Nil(err)
	assert.NotNil(ua)
	for _, a := range ua.Attributes {
		assert.Contains(nMapI, a.Name)
		for i, aa := range a.Actions {
			t.Log(a.Name, aa)
			if ev, ok := expRV[i][a.Name]; ok {
				assert.Equal(ev.fb, aa.FromBody)
				assert.Equal(ev.f, MSS(aa.Fields))
				assert.Equal(ev.i, MIS(aa.Indexes))
			} else {
				assert.False(aa.FromBody)
				assert.Empty(aa.Fields)
				assert.Empty(aa.Indexes)
			}
		}
	}

	// case: remove/set validation conflict
	params = [3][]string{
		centrald.UpdateRemove: []string{
			nMapI["ArrayAttr"],
		},
		centrald.UpdateSet: []string{
			nMapI["ArrayAttr"],
		},
	}
	ua, err = hc.makeStdUpdateArgs(nMap, id, version, params)
	assert.NotNil(err)
	assert.Nil(ua)
	assert.Regexp("set='arrayattr': conflicts with remove or append$", err.Error())
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, err.Error())

	// case: append/set validation conflict
	params = [3][]string{
		centrald.UpdateAppend: []string{
			nMapI["ArrayAttr"],
		},
		centrald.UpdateSet: []string{
			nMapI["ArrayAttr"] + ".0",
		},
	}
	ua, err = hc.makeStdUpdateArgs(nMap, id, version, params)
	assert.NotNil(err)
	assert.Nil(ua)
	assert.Regexp("set='arrayattr': conflicts with remove or append$", err.Error())
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, err.Error())

	// case: append/remove validation conflict
	params = [3][]string{
		centrald.UpdateAppend: []string{
			nMapI["ArrayAttr"],
		},
		centrald.UpdateRemove: []string{
			nMapI["ArrayAttr"],
		},
	}
	ua, err = hc.makeStdUpdateArgs(nMap, id, version, params)
	assert.NotNil(err)
	assert.Nil(ua)
	assert.Regexp("append='arrayattr': conflicts with remove$", err.Error())
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, err.Error())

}

func TestMergeAttributes(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()

	orig := map[string]models.ValueType{
		"property1": {Kind: "STRING", Value: "string1"},
		"property2": {Kind: "STRING", Value: "string2"},
		"property3": {Kind: "STRING", Value: "string3"},
	}
	updates := map[string]models.ValueType{
		"property3": {Kind: "STRING", Value: "string3b"},
		"property4": {Kind: "STRING", Value: "string4"},
		"property5": {Kind: "STRING", Value: "string5"},
	}

	t.Log("all nil")
	ra, re := hc.mergeAttributes(nil, "nil", nil, nil)
	assert.Nil(ra)
	assert.NoError(re)

	t.Log("nil updates")
	ra, re = hc.mergeAttributes(nil, "nil", orig, nil)
	assert.Equal(orig, ra)
	assert.NoError(re)

	t.Log("set FromBody")
	a := &centrald.UpdateAttr{
		Name: "AttrName",
		Actions: [centrald.NumActionTypes]centrald.UpdateActionArgs{
			centrald.UpdateSet: centrald.UpdateActionArgs{
				FromBody: true,
			},
		},
	}
	ra, re = hc.mergeAttributes(a, "attrName", orig, updates)
	assert.Equal(updates, ra)
	assert.NoError(re)

	t.Log("set Fields")
	a.Actions = [centrald.NumActionTypes]centrald.UpdateActionArgs{
		centrald.UpdateSet: centrald.UpdateActionArgs{
			Fields: map[string]struct{}{
				"property3": {},
				"property5": {},
			},
		},
	}
	ra, re = hc.mergeAttributes(a, "attrName", orig, updates)
	assert.Len(ra, 4)
	assert.NoError(re)
	for k, v := range ra {
		if v2, ok := updates[k]; ok {
			assert.Equal(v2, v)
		} else {
			v2, ok := orig[k]
			assert.True(ok)
			assert.Equal(v2, v)
		}
	}
	assert.NotContains(ra, "property4")

	t.Log("set Fields, missing field error")
	a.Actions = [centrald.NumActionTypes]centrald.UpdateActionArgs{
		centrald.UpdateSet: centrald.UpdateActionArgs{
			Fields: map[string]struct{}{
				"property4": {},
				"property6": {},
			},
		},
	}
	ra, re = hc.mergeAttributes(a, "attrName", orig, updates)
	assert.Nil(ra)
	assert.Error(re)
	e, ok := re.(*centrald.Error)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, e.C)
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, e.M)
	assert.Regexp("set='attrName.property6'", e.M)

	t.Log("append FromBody")
	a.Actions = [centrald.NumActionTypes]centrald.UpdateActionArgs{
		centrald.UpdateAppend: centrald.UpdateActionArgs{
			FromBody: true,
		},
	}
	ra, re = hc.mergeAttributes(a, "attrName", orig, updates)
	assert.Len(ra, 5)
	assert.NoError(re)
	for k, v := range ra {
		if v2, ok := updates[k]; ok {
			assert.Equal(v2, v)
		} else {
			v2, ok := orig[k]
			assert.True(ok)
			assert.Equal(v2, v)
		}
	}

	t.Log("remove FromBody")
	a.Actions = [centrald.NumActionTypes]centrald.UpdateActionArgs{
		centrald.UpdateRemove: centrald.UpdateActionArgs{
			FromBody: true,
		},
	}
	ra, re = hc.mergeAttributes(a, "attrName", orig, updates)
	assert.Len(ra, 2)
	assert.NoError(re)
	for k, v := range ra {
		assert.NotContains(updates, k)
		v2, ok := orig[k]
		assert.True(ok)
		assert.Equal(v2, v)
	}
}

func TestMergeStorageCosts(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()

	orig := map[string]models.StorageCost{
		"st1": {CostPerGiB: 1.0},
		"st2": {CostPerGiB: 2.0},
		"st3": {CostPerGiB: 3.0},
	}
	updates := map[string]models.StorageCost{
		"st3": {CostPerGiB: 3.1},
		"st4": {CostPerGiB: 4.0},
		"st5": {CostPerGiB: 5.0},
	}
	mChg3 := fmt.Sprintf(stgCostChgFmt, "st3", 3.0, 3.1)
	mChg4 := fmt.Sprintf(stgCostSetFmt, "st4", 4.0)
	mChg5 := fmt.Sprintf(stgCostSetFmt, "st5", 5.0)
	mClr3 := fmt.Sprintf(stgCostChgFmt, "st3", 3.0, 0.0)

	t.Log("all nil")
	ra, msgs, re := hc.mergeStorageCosts(nil, "nil", nil, nil)
	assert.Nil(ra)
	assert.NoError(re)
	assert.Empty(msgs)

	t.Log("nil updates")
	ra, msgs, re = hc.mergeStorageCosts(nil, "nil", orig, nil)
	assert.Equal(orig, ra)
	assert.NoError(re)
	assert.Empty(msgs)

	t.Log("set FromBody")
	a := &centrald.UpdateAttr{
		Name: "AttrName",
		Actions: [centrald.NumActionTypes]centrald.UpdateActionArgs{
			centrald.UpdateSet: centrald.UpdateActionArgs{
				FromBody: true,
			},
		},
	}
	ra, msgs, re = hc.mergeStorageCosts(a, "attrName", orig, updates)
	assert.Equal(updates, ra)
	assert.NoError(re)
	assert.NotEmpty(msgs)
	sort.Strings(msgs)
	assert.Equal([]string{mChg3, mChg4, mChg5}, msgs)

	t.Log("set Fields")
	a.Actions = [centrald.NumActionTypes]centrald.UpdateActionArgs{
		centrald.UpdateSet: centrald.UpdateActionArgs{
			Fields: map[string]struct{}{
				"st3": {},
				"st5": {},
			},
		},
	}
	ra, msgs, re = hc.mergeStorageCosts(a, "attrName", orig, updates)
	assert.Len(ra, 4)
	assert.NoError(re)
	for k, v := range ra {
		if v2, ok := updates[k]; ok {
			assert.Equal(v2, v)
		} else {
			v2, ok := orig[k]
			assert.True(ok)
			assert.Equal(v2, v)
		}
	}
	assert.NotContains(ra, "st4")
	assert.NotEmpty(msgs)
	sort.Strings(msgs)
	assert.Equal([]string{mChg3, mChg5}, msgs)

	t.Log("set Fields, missing field error")
	a.Actions = [centrald.NumActionTypes]centrald.UpdateActionArgs{
		centrald.UpdateSet: centrald.UpdateActionArgs{
			Fields: map[string]struct{}{
				"st4": {},
				"st6": {},
			},
		},
	}
	ra, msgs, re = hc.mergeStorageCosts(a, "attrName", orig, updates)
	assert.Nil(ra)
	assert.Error(re)
	assert.Regexp("set='attrName.st6'", re)

	t.Log("append FromBody")
	a.Actions = [centrald.NumActionTypes]centrald.UpdateActionArgs{
		centrald.UpdateAppend: centrald.UpdateActionArgs{
			FromBody: true,
		},
	}
	ra, msgs, re = hc.mergeStorageCosts(a, "attrName", orig, updates)
	assert.Len(ra, 5)
	assert.NoError(re)
	for k, v := range ra {
		if v2, ok := updates[k]; ok {
			assert.Equal(v2, v)
		} else {
			v2, ok := orig[k]
			assert.True(ok)
			assert.Equal(v2, v)
		}
	}
	assert.NotEmpty(msgs)
	sort.Strings(msgs)
	assert.Equal([]string{mChg3, mChg4, mChg5}, msgs)

	t.Log("remove FromBody")
	a.Actions = [centrald.NumActionTypes]centrald.UpdateActionArgs{
		centrald.UpdateRemove: centrald.UpdateActionArgs{
			FromBody: true,
		},
	}
	ra, msgs, re = hc.mergeStorageCosts(a, "attrName", orig, updates)
	assert.Len(ra, 2)
	assert.NoError(re)
	for k, v := range ra {
		assert.NotContains(updates, k)
		v2, ok := orig[k]
		assert.True(ok)
		assert.Equal(v2, v)
	}
	assert.NotEmpty(msgs)
	sort.Strings(msgs)
	assert.Equal([]string{mClr3}, msgs)
}

func TestValidateStorageCosts(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := testAppCtx(nil, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app

	costs := map[string]models.StorageCost{
		"st1": {},
	}
	domType := "domainType"

	t.Log("case: unknown storage type")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mCSP := mock.NewMockAppCloudServiceProvider(mockCtrl)
	hc.app.AppCSP = mCSP
	mCSP.EXPECT().GetCspStorageType(models.CspStorageType("st1")).Return(nil)
	err := hc.validateStorageCosts(costs, domType)
	assert.Error(err)
	assert.Regexp("invalid storage type", err)
	mockCtrl.Finish()

	t.Log("case: invalid storage type for domain type")
	stObj := &models.CSPStorageType{}
	stObj.CspDomainType = models.CspDomainType(domType + "foo")
	mockCtrl = gomock.NewController(t)
	mCSP = mock.NewMockAppCloudServiceProvider(mockCtrl)
	hc.app.AppCSP = mCSP
	mCSP.EXPECT().GetCspStorageType(models.CspStorageType("st1")).Return(stObj)
	err = hc.validateStorageCosts(costs, domType)
	assert.Error(err)
	assert.Regexp("invalid storage type", err)
	mockCtrl.Finish()

	t.Log("case: negative cost value")
	costs["st1"] = models.StorageCost{CostPerGiB: -1.0}
	stObj.CspDomainType = models.CspDomainType(domType)
	mockCtrl = gomock.NewController(t)
	mCSP = mock.NewMockAppCloudServiceProvider(mockCtrl)
	hc.app.AppCSP = mCSP
	mCSP.EXPECT().GetCspStorageType(models.CspStorageType("st1")).Return(stObj)
	err = hc.validateStorageCosts(costs, domType)
	assert.Error(err)
	assert.Regexp("invalid cost", err)
	mockCtrl.Finish()

	t.Log("case: ok")
	costs["st1"] = models.StorageCost{CostPerGiB: 1.0}
	stObj.CspDomainType = models.CspDomainType(domType)
	mockCtrl = gomock.NewController(t)
	mCSP = mock.NewMockAppCloudServiceProvider(mockCtrl)
	hc.app.AppCSP = mCSP
	mCSP.EXPECT().GetCspStorageType(models.CspStorageType("st1")).Return(stObj)
	err = hc.validateStorageCosts(costs, domType)
	assert.NoError(err)
}

func TestValidateApplicationGroupIds(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	fa := &fal.AuditLog{}
	app.AuditLog = fa
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ctx := context.Background()

	agObj1 := &models.ApplicationGroup{
		ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
			Meta: &models.ObjMeta{ID: "ag1"},
		},
		ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
			AccountID: "aid1",
		},
		ApplicationGroupMutable: models.ApplicationGroupMutable{
			Name: "app",
		},
	}
	agObj2 := &models.ApplicationGroup{
		ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
			Meta: &models.ObjMeta{ID: "ag2"},
		},
		ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
			AccountID: "aid1",
		},
		ApplicationGroupMutable: models.ApplicationGroupMutable{
			Name: "ag",
		},
	}

	t.Log("case: success with nil list")
	agNames, err := hc.validateApplicationGroupIds(ctx, "", nil)
	assert.Empty(agNames)
	assert.NoError(err)

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mds := mock.NewMockDataStore(mockCtrl)
	accountID := models.ObjIDMutable("aid1")
	agIDs := []models.ObjIDMutable{"ag1", "ag2"}
	oAG := mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Fetch(ctx, "ag1").Return(agObj1, nil)
	oAG.EXPECT().Fetch(ctx, "ag2").Return(agObj2, nil)
	mds.EXPECT().OpsApplicationGroup().Return(oAG).MinTimes(1)
	hc.DS = mds
	agNames, err = hc.validateApplicationGroupIds(ctx, accountID, agIDs)
	assert.Equal([]string{"app", "ag"}, agNames)
	assert.NoError(err)

	// all the remaining error cases
	for tc := 0; tc <= 2; tc++ {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		mds = mock.NewMockDataStore(mockCtrl)
		oAG = mock.NewMockApplicationGroupOps(mockCtrl)
		expectedErr := centrald.ErrorMissing
		switch tc {
		case 0:
			t.Log("case: ApplicationGroupID not found")
			oAG.EXPECT().Fetch(ctx, string(agIDs[0])).Return(nil, centrald.ErrorNotFound)
			mds.EXPECT().OpsApplicationGroup().Return(oAG)
		case 1:
			t.Log("case: ApplicationGroupIDs not unique")
			agIDs = []models.ObjIDMutable{"ag1", "ag2", "ag1"}
			expectedErr = &centrald.Error{M: ".*applicationGroupIds.* unique: ag1", C: centrald.ErrorMissing.C}
			oAG.EXPECT().Fetch(ctx, gomock.Any()).Return(agObj1, nil).Times(2)
			mds.EXPECT().OpsApplicationGroup().Return(oAG).Times(2)
		case 2:
			t.Log("case: ApplicationGroupID not available to Account")
			accountID = "another"
			expectedErr = &centrald.Error{M: "applicationGroup", C: centrald.ErrorUnauthorizedOrForbidden.C}
			oAG.EXPECT().Fetch(ctx, string(agIDs[0])).Return(agObj1, nil)
			mds.EXPECT().OpsApplicationGroup().Return(oAG)
		default:
			assert.True(false)
		}
		hc.DS = mds
		agNames, err = hc.validateApplicationGroupIds(ctx, accountID, agIDs)
		assert.Nil(agNames)
		if e, ok := err.(*centrald.Error); assert.True(ok) {
			assert.Equal(expectedErr.C, e.C)
			assert.Regexp("^"+expectedErr.M, e.M)
		}
	}
}

func TestValidateAuthorizedAccountsUpdate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	fa := &fal.AuditLog{}
	app.AuditLog = fa
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app

	aObj1 := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta:            &models.ObjMeta{ID: "aid1"},
			TenantAccountID: "tid1",
		},
		AccountMutable: models.AccountMutable{
			Name: "aName1",
		},
	}
	aObj2 := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta:            &models.ObjMeta{ID: "aid2"},
			TenantAccountID: "tid1",
		},
		AccountMutable: models.AccountMutable{
			Name: "aName2",
		},
	}
	aObj3 := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta:            &models.ObjMeta{ID: "aid3"},
			TenantAccountID: "tid1",
		},
		AccountMutable: models.AccountMutable{
			Name: "aName3",
		},
	}
	aObj4 := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta:            &models.ObjMeta{ID: "aid4"},
			TenantAccountID: "tid1",
		},
		AccountMutable: models.AccountMutable{
			Name: "aName4",
		},
	}

	t.Log("case: empty lists")
	action := &centrald.UpdateAttr{}
	ret, err := hc.validateAuthorizedAccountsUpdate(nil, nil, centrald.AccountUpdateAction, "", "", action, nil, nil)
	assert.Empty(ret)
	assert.NoError(err)

	t.Log("case: append, full overlap, no-op")
	action = &centrald.UpdateAttr{
		Actions: [centrald.NumActionTypes]centrald.UpdateActionArgs{
			centrald.UpdateAppend: centrald.UpdateActionArgs{FromBody: true},
		},
	}
	old := []models.ObjIDMutable{"aid1", "aid2"}
	upd := []models.ObjIDMutable{"aid1", "aid2"}
	ret, err = hc.validateAuthorizedAccountsUpdate(nil, nil, centrald.AccountUpdateAction, "", "", action, old, upd)
	assert.Empty(ret)
	assert.NoError(err)

	t.Log("case: set, full overlap, no-op")
	action = &centrald.UpdateAttr{
		Actions: [centrald.NumActionTypes]centrald.UpdateActionArgs{
			centrald.UpdateSet: centrald.UpdateActionArgs{FromBody: true},
		},
	}
	old = []models.ObjIDMutable{"aid1", "aid2"}
	upd = []models.ObjIDMutable{"aid1", "aid2"}
	ret, err = hc.validateAuthorizedAccountsUpdate(nil, nil, centrald.AccountUpdateAction, "", "", action, old, upd)
	assert.Empty(ret)
	assert.NoError(err)

	t.Log("case: remove, no overlap, no-op")
	action = &centrald.UpdateAttr{
		Actions: [centrald.NumActionTypes]centrald.UpdateActionArgs{
			centrald.UpdateRemove: centrald.UpdateActionArgs{FromBody: true},
		},
	}
	old = []models.ObjIDMutable{"aid1", "aid2"}
	upd = []models.ObjIDMutable{"aid3", "aid4"}
	ret, err = hc.validateAuthorizedAccountsUpdate(nil, nil, centrald.AccountUpdateAction, "", "", action, old, upd)
	assert.Empty(ret)
	assert.NoError(err)

	t.Log("case: append, no overlap, internal role")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ai := &auth.Info{}
	action = &centrald.UpdateAttr{
		Actions: [centrald.NumActionTypes]centrald.UpdateActionArgs{
			centrald.UpdateAppend: centrald.UpdateActionArgs{FromBody: true},
		},
	}
	old = []models.ObjIDMutable{"aid1", "aid2"}
	upd = []models.ObjIDMutable{"aid3", "aid4"}
	mds := mock.NewMockDataStore(mockCtrl)
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(nil, "aid3").Return(aObj3, nil)
	oA.EXPECT().Fetch(nil, "aid4").Return(aObj4, nil)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	ret, err = hc.validateAuthorizedAccountsUpdate(nil, ai, centrald.ServicePlanUpdateAction, "oid", "oName", action, old, upd)
	assert.Equal("added[aName3, aName4]", ret)
	assert.NoError(err)

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: remove, overlap, internal role")
	ai = &auth.Info{}
	action = &centrald.UpdateAttr{
		Actions: [centrald.NumActionTypes]centrald.UpdateActionArgs{
			centrald.UpdateRemove: centrald.UpdateActionArgs{FromBody: true},
		},
	}
	spaParams1 := spa.ServicePlanAllocationListParams{AuthorizedAccountID: swag.String("aid1"), ServicePlanID: swag.String("oid")}
	spaParams2 := spa.ServicePlanAllocationListParams{AuthorizedAccountID: swag.String("aid2"), ServicePlanID: swag.String("oid")}
	old = []models.ObjIDMutable{"aid1", "aid2"}
	upd = []models.ObjIDMutable{"aid1", "aid2"}
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(nil, "aid1").Return(aObj1, nil)
	oA.EXPECT().Fetch(nil, "aid2").Return(aObj2, nil)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	oSPA := mock.NewMockServicePlanAllocationOps(mockCtrl)
	oSPA.EXPECT().Count(nil, spaParams1, uint(1)).Return(0, nil)
	oSPA.EXPECT().Count(nil, spaParams2, uint(1)).Return(0, nil)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA).MinTimes(1)
	hc.DS = mds
	ret, err = hc.validateAuthorizedAccountsUpdate(nil, ai, centrald.ServicePlanUpdateAction, "oid", "oName", action, old, upd)
	assert.Equal("removed[aName1, aName2]", ret)
	assert.NoError(err)

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: set from body, partial overlap, tenant role")
	ai = &auth.Info{
		AccountID: "tid1",
		RoleObj:   &models.Role{},
	}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	action = &centrald.UpdateAttr{
		Actions: [centrald.NumActionTypes]centrald.UpdateActionArgs{
			centrald.UpdateSet: centrald.UpdateActionArgs{FromBody: true},
		},
	}
	spaParams1 = spa.ServicePlanAllocationListParams{AuthorizedAccountID: swag.String("aid1"), ClusterID: swag.String("oid")}
	old = []models.ObjIDMutable{"aid1", "aid2"}
	upd = []models.ObjIDMutable{"aid2", "aid4"}
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(nil, "aid1").Return(aObj1, nil)
	oA.EXPECT().Fetch(nil, "aid4").Return(aObj4, nil)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
	oSPA.EXPECT().Count(nil, spaParams1, uint(1)).Return(0, nil)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA).MinTimes(1)
	hc.DS = mds
	ret, err = hc.validateAuthorizedAccountsUpdate(nil, ai, centrald.ClusterUpdateAction, "oid", "oName", action, old, upd)
	assert.Equal("added[aName4] removed[aName1]", ret)
	assert.NoError(err)

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: set fields, partial overlap, tenant role")
	ai = &auth.Info{
		AccountID: "tid1",
		RoleObj:   &models.Role{},
	}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	action = &centrald.UpdateAttr{
		Actions: [centrald.NumActionTypes]centrald.UpdateActionArgs{
			centrald.UpdateSet: centrald.UpdateActionArgs{
				Indexes: map[int]struct{}{
					1: struct{}{},
					4: struct{}{},
				},
			},
		},
	}
	old = []models.ObjIDMutable{"aid1", "aid2"}
	upd = []models.ObjIDMutable{"nid1", "aid2", "nid3", "aid3", "aid4", "nid4"}
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(nil, "aid4").Return(aObj4, nil)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	ret, err = hc.validateAuthorizedAccountsUpdate(nil, ai, centrald.ServicePlanUpdateAction, "oid", "oName", action, old, upd)
	assert.Equal("added[aName4]", ret)
	assert.NoError(err)
	assert.Empty(fa.Posts)

	t.Log("case: unauthorized add")
	ai = &auth.Info{
		AccountID: "tid1",
		RoleObj:   &models.Role{},
	}
	action = &centrald.UpdateAttr{
		Actions: [centrald.NumActionTypes]centrald.UpdateActionArgs{
			centrald.UpdateAppend: centrald.UpdateActionArgs{FromBody: true},
		},
	}
	old = []models.ObjIDMutable{"aid1", "aid2"}
	upd = []models.ObjIDMutable{"aid3"}
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(nil, "aid3").Return(aObj3, nil)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	ret, err = hc.validateAuthorizedAccountsUpdate(nil, ai, centrald.ServicePlanUpdateAction, "oid", "oName", action, old, upd)
	assert.Empty(ret)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, err)
	exp := &fal.Args{AI: ai, Action: centrald.ServicePlanUpdateAction, ObjID: "oid", Name: "oName", Err: true, Message: "Update unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)

	t.Log("case: unauthorized remove")
	fa.Posts = []*fal.Args{}
	ai = &auth.Info{
		AccountID: "tid1",
		RoleObj:   &models.Role{},
	}
	action = &centrald.UpdateAttr{
		Actions: [centrald.NumActionTypes]centrald.UpdateActionArgs{
			centrald.UpdateRemove: centrald.UpdateActionArgs{FromBody: true},
		},
	}
	old = []models.ObjIDMutable{"aid1", "aid2"}
	upd = []models.ObjIDMutable{"aid2"}
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(nil, "aid2").Return(aObj2, nil)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	ret, err = hc.validateAuthorizedAccountsUpdate(nil, ai, centrald.ServicePlanUpdateAction, "oid", "oName", action, old, upd)
	assert.Empty(ret)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, err)
	assert.Equal([]*fal.Args{exp}, fa.Posts)

	t.Log("case: add, account lookup failure")
	action = &centrald.UpdateAttr{
		Actions: [centrald.NumActionTypes]centrald.UpdateActionArgs{
			centrald.UpdateAppend: centrald.UpdateActionArgs{FromBody: true},
		},
	}
	old = []models.ObjIDMutable{}
	upd = []models.ObjIDMutable{"aid1"}
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(nil, "aid1").Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	ret, err = hc.validateAuthorizedAccountsUpdate(nil, ai, centrald.ServicePlanUpdateAction, "oid", "oName", action, old, upd)
	assert.Empty(ret)
	assert.Equal(centrald.ErrorDbError, err)

	t.Log("case: add, account not found")
	action = &centrald.UpdateAttr{
		Actions: [centrald.NumActionTypes]centrald.UpdateActionArgs{
			centrald.UpdateAppend: centrald.UpdateActionArgs{FromBody: true},
		},
	}
	old = []models.ObjIDMutable{}
	upd = []models.ObjIDMutable{"aid1"}
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(nil, "aid1").Return(nil, centrald.ErrorNotFound)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	ret, err = hc.validateAuthorizedAccountsUpdate(nil, ai, centrald.ServicePlanUpdateAction, "oid", "oName", action, old, upd)
	assert.Empty(ret)
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M+": invalid account", err)

	t.Log("case: remove, account lookup failure")
	action = &centrald.UpdateAttr{
		Actions: [centrald.NumActionTypes]centrald.UpdateActionArgs{
			centrald.UpdateRemove: centrald.UpdateActionArgs{FromBody: true},
		},
	}
	old = []models.ObjIDMutable{"aid1"}
	upd = []models.ObjIDMutable{"aid1"}
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(nil, "aid1").Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	ret, err = hc.validateAuthorizedAccountsUpdate(nil, ai, centrald.ServicePlanUpdateAction, "oid", "oName", action, old, upd)
	assert.Empty(ret)
	assert.Equal(centrald.ErrorDbError, err)

	t.Log("case: remove, account not found ignored")
	action = &centrald.UpdateAttr{
		Actions: [centrald.NumActionTypes]centrald.UpdateActionArgs{
			centrald.UpdateRemove: centrald.UpdateActionArgs{FromBody: true},
		},
	}
	old = []models.ObjIDMutable{"aid1"}
	upd = []models.ObjIDMutable{"aid1"}
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(nil, "aid1").Return(nil, centrald.ErrorNotFound)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	ret, err = hc.validateAuthorizedAccountsUpdate(nil, ai, centrald.ServicePlanUpdateAction, "oid", "oName", action, old, upd)
	assert.Empty(ret)
	assert.NoError(err)

	t.Log("case: remove, SPA in use")
	ai = &auth.Info{
		AccountID: "tid1",
		RoleObj:   &models.Role{},
	}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	action = &centrald.UpdateAttr{
		Actions: [centrald.NumActionTypes]centrald.UpdateActionArgs{
			centrald.UpdateRemove: centrald.UpdateActionArgs{FromBody: true},
		},
	}
	spaParams1 = spa.ServicePlanAllocationListParams{AuthorizedAccountID: swag.String("aid1"), CspDomainID: swag.String("oid")}
	old = []models.ObjIDMutable{"aid1"}
	upd = []models.ObjIDMutable{"aid1"}
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(nil, "aid1").Return(aObj1, nil)
	mds.EXPECT().OpsAccount().Return(oA)
	oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
	oSPA.EXPECT().Count(nil, spaParams1, uint(1)).Return(1, nil)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
	hc.DS = mds
	ret, err = hc.validateAuthorizedAccountsUpdate(nil, ai, centrald.CspDomainUpdateAction, "oid", "oName", action, old, upd)
	assert.Empty(ret)
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M+": .*being used", err)

	t.Log("case: remove, SPA lookup failure")
	old = []models.ObjIDMutable{"aid1"}
	upd = []models.ObjIDMutable{"aid1"}
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(nil, "aid1").Return(aObj1, nil)
	mds.EXPECT().OpsAccount().Return(oA)
	oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
	oSPA.EXPECT().Count(nil, spaParams1, uint(1)).Return(0, centrald.ErrorDbError)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
	hc.DS = mds
	ret, err = hc.validateAuthorizedAccountsUpdate(nil, ai, centrald.CspDomainUpdateAction, "oid", "oName", action, old, upd)
	assert.Empty(ret)
	assert.Equal(centrald.ErrorDbError, err)

	t.Log("case: future proof panic")
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(nil, "aid1").Return(aObj1, nil)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	assert.Panics(func() {
		hc.validateAuthorizedAccountsUpdate(nil, ai, centrald.NodeUpdateAction, "oid", "oName", action, old, upd)
	})
}

func TestObjectScopeHelpers(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()

	// test addNewObjIDToScopeMap with a few different object types
	m := map[string]string{}
	stgObj := &models.Storage{}
	stgObj.Meta = &models.ObjMeta{ID: "storageId", Version: 1}
	hc.addNewObjIDToScopeMap(stgObj, m)
	assert.NotEmpty(m)
	s, ok := m["meta.id"]
	assert.True(ok)
	assert.Equal("storageId", s)

	m = map[string]string{}
	stgObj.Meta = &models.ObjMeta{ID: "storageId", Version: 2}
	hc.addNewObjIDToScopeMap(stgObj, m)
	assert.Empty(m)

	m = map[string]string{}
	vsObj := &models.VolumeSeries{}
	vsObj.Meta = &models.ObjMeta{ID: "volumeSeriesId", Version: 1}
	hc.addNewObjIDToScopeMap(vsObj, m)
	assert.NotEmpty(m)
	s, ok = m["meta.id"]
	assert.True(ok)
	assert.Equal("volumeSeriesId", s)

	m = map[string]string{}
	vsObj.Meta = &models.ObjMeta{ID: "volumeSeriesId", Version: 399}
	hc.addNewObjIDToScopeMap(vsObj, m)
	assert.Empty(m)

	m = map[string]string{}
	aObj := &models.Account{}
	aObj.Meta = &models.ObjMeta{ID: "accountId", Version: 1}
	hc.addNewObjIDToScopeMap(aObj, m)
	assert.NotEmpty(m)
	s, ok = m["meta.id"]
	assert.True(ok)
	assert.Equal("accountId", s)

	m = map[string]string{}
	aObj.Meta = &models.ObjMeta{ID: "accountId", Version: 2}
	hc.addNewObjIDToScopeMap(aObj, m)
	assert.Empty(m)

	// test setNewObjectScope (uses addNewObjIDToScopeMap)
	app := testAppCtx(nil, tl.Logger())
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	hc.app = app

	req := &http.Request{}
	cspObj := &models.CSPDomain{}
	cspObj.Meta = &models.ObjMeta{ID: "cspDomainId", Version: 1}
	assert.Nil(evM.InSSProps)
	hc.setDefaultObjectScope(req, cspObj)
	assert.NotNil(evM.InSSProps)
	assert.NotEmpty(evM.InSSProps)
	s, ok = evM.InSSProps["meta.id"]
	assert.Equal(cspObj, evM.InACScope)
	assert.True(ok)
	assert.Equal("cspDomainId", s)
}

func testAppCtx(api *operations.NuvolosoAPI, log *logging.Logger) *centrald.AppCtx {
	fts := &fhk.TaskScheduler{}
	return &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log: log,
			API: api,
		},
		TaskScheduler: fts,
	}
}

func TestMakeAggregationResult(t *testing.T) {
	assert := assert.New(t)
	hc := newHandlerComp()

	ag := []*centrald.Aggregation{
		&centrald.Aggregation{FieldPath: "a", Type: centrald.SumType, Value: 10101010101},
		&centrald.Aggregation{FieldPath: "b", Type: centrald.SumType, Value: 23232},
	}
	exp := []string{"a:sum:10101010101", "b:sum:23232"}
	ret := hc.makeAggregationResult(ag)
	assert.Equal(exp, ret)

	ag = nil
	ret = hc.makeAggregationResult(ag)
	assert.Equal([]string{}, ret)
}

func TestValidateClusterUsagePolicy(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()

	scopes := []string{common.AccountSecretScopeGlobal, common.AccountSecretScopeCspDomain, common.AccountSecretScopeCluster}

	invalidCups := []*models.ClusterUsagePolicy{
		nil,
		&models.ClusterUsagePolicy{},
		&models.ClusterUsagePolicy{AccountSecretScope: "foo"},
		&models.ClusterUsagePolicy{AccountSecretScope: "GLOBAL"},
		&models.ClusterUsagePolicy{AccountSecretScope: "GLOBAL", VolumeDataRetentionOnDelete: "FOO"},
	}
	for i, cup := range invalidCups {
		assert.Error(hc.validateClusterUsagePolicy(cup, common.AccountSecretScopeGlobal), "cup %d", i)
	}
	validCups := []*models.ClusterUsagePolicy{
		&models.ClusterUsagePolicy{
			AccountSecretScope: common.AccountSecretScopeCluster,
		},
		&models.ClusterUsagePolicy{
			Inherited: true,
		},
	}
	for i, cup := range validCups {
		// test all valid delete pv actions
		for _, volumeDataRetentionOnDelete := range validVolumeDataRetentionValues {
			cup.VolumeDataRetentionOnDelete = volumeDataRetentionOnDelete
			// test the combination of secret scope and object scope
			for _, secretScope := range scopes {
				for _, objScope := range scopes {
					cup.AccountSecretScope = secretScope
					if cup.Inherited == true || util.Contains(validAccountSecretScopesByObj[objScope], secretScope) {
						assert.NoError(hc.validateClusterUsagePolicy(cup, objScope), "cup %d (%s,%s)", i, secretScope, objScope)
					} else {
						assert.Error(hc.validateClusterUsagePolicy(cup, objScope), "cup %d (%s,%s)", i, secretScope, objScope)
					}
				}
			}
		}
	}
}

func TestValidateSnapshotManagementPolicy(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()

	invalidSMP := []*models.SnapshotManagementPolicy{
		nil,
		&models.SnapshotManagementPolicy{},
		&models.SnapshotManagementPolicy{NoDelete: true, VolumeDataRetentionOnDelete: "foo"},
	}
	for i, p := range invalidSMP {
		assert.Error(hc.validateSnapshotManagementPolicy(p), "p %d", i)
	}

	validSMP := []*models.SnapshotManagementPolicy{
		&models.SnapshotManagementPolicy{RetentionDurationSeconds: swag.Int32(10)},
		&models.SnapshotManagementPolicy{NoDelete: true},
		&models.SnapshotManagementPolicy{Inherited: true},
	}
	for i, p := range validSMP {
		assert.NoError(hc.validateSnapshotManagementPolicy(p), "p %d", i)
		for _, v := range validVolumeDataRetentionValues {
			p.VolumeDataRetentionOnDelete = v
			assert.NoError(hc.validateSnapshotManagementPolicy(p), "p %d %s", i, v)
		}
	}
}

func TestValidateVSRManagementPolicy(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()

	invalidVMP := []*models.VsrManagementPolicy{
		nil,
		&models.VsrManagementPolicy{},
	}
	for i, p := range invalidVMP {
		assert.Error(hc.validateVSRManagementPolicy(p), "p %d", i)
	}

	validVMP := []*models.VsrManagementPolicy{
		&models.VsrManagementPolicy{RetentionDurationSeconds: swag.Int32(10)},
		&models.VsrManagementPolicy{NoDelete: true},
		&models.VsrManagementPolicy{Inherited: true},
	}
	for i, p := range validVMP {
		assert.NoError(hc.validateVSRManagementPolicy(p), "p %d", i)
	}

}

func TestValidateClusterState(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()

	for _, state := range validClusterStates {
		assert.True(hc.validateClusterState(state))
		assert.False(hc.validateClusterState(state + "foo"))
	}
}

func TestValidateNodeState(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()

	for _, state := range validNodeStates {
		assert.True(hc.validateNodeState(state))
		assert.False(hc.validateNodeState(state + "foo"))
	}
}

func TestVerifySortKeys(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()

	type SomeType struct {
		SimpleAttr string         `json:"simpleattr"`
		ArrayAttr  []string       `json:"arrayattr"`
		StringTMap map[string]int `json:"stringtmap"`
		OtherMap   map[int]string `json:"othermap"`
		StructAttr struct {
			N string `json:"n"`
			V string `json:"v"`
		} `json:"structattr"`
		PtrToSimple *string `json:"ptrtosimple"`
		PtrToStruct *struct {
			V int `json:"v"`
		} `json:"ptrtostruct"`
		PtrToStringTMap *map[string]int `json:"ptrtostringtmap"`
		PtrToOtherMap   *map[int]int    `json:"ptrtoothermap"`
		PtrToArray      *[]string       `json:"ptrtoarray"`
		Boolean         bool            `json:"bool"`
		Function        func()          `json:"func"`
	}
	nMap := hc.makeJSONToAttrNameMap(SomeType{})
	nMapI := make(map[string]string, len(nMap)) // field name  => json name
	for k, sf := range nMap {
		nMapI[sf.Name] = k
	}
	assert.Equal(len(nMap), len(nMapI))

	validKeys1 := []string{
		"simpleattr",
		"structattr.v",
		"ptrtosimple",
		"ptrtostruct.v",
	}
	validKeys2 := []string{
		"ptrtosimple",
		"ptrtostruct.v",
		"bool",
	}

	err := hc.verifySortKeys(nMap, validKeys1, validKeys2)
	assert.Nil(err)

	invalidKeys := []string{
		"arrayattr",
		"othermap",
		"ptrtoothermap",
		"ptrtoothermap.something",
		"ptrtoarray",
		"something",
		"ptrtostruct.something",
		"some.thing.else",
		"stringtmap.value",
		"structattr",
		"func",
	}
	for _, key := range invalidKeys {
		err = hc.verifySortKeys(nMap, []string{key}, []string{})
		assert.NotNil(err)
	}

	err = hc.verifySortKeys(nMap, []string{}, []string{})
	assert.Nil(err)

	type SortTypes struct {
		StrAttr         string           `json:"strattr"`
		IntAttr         int              `json:"intattr"`
		Int8Attr        int8             `json:"int8attr"`
		Int16Attr       int16            `json:"int16attr"`
		Int32Attr       int32            `json:"int32attr"`
		Int64Attr       int64            `json:"int64attr"`
		UIntAttr        uint             `json:"uintattr"`
		UInt8Attr       uint8            `json:"uint8attr"`
		UInt16Attr      uint16           `json:"uint16attr"`
		UInt32Attr      uint32           `json:"uint32attr"`
		UInt64Attr      uint64           `json:"uint64attr"`
		Float32Attr     float32          `json:"float32attr"`
		Float64Attr     float64          `json:"float64attr"`
		PtrStrAttr      *string          `json:"ptrstrattr"`
		PtrIntAttr      *int             `json:"ptrintattr"`
		PtrInt8Attr     *int8            `json:"ptrint8attr"`
		PtrInt16Attr    *int16           `json:"ptrint16attr"`
		PtrInt32Attr    *int32           `json:"ptrint32attr"`
		PtrInt64Attr    *int64           `json:"ptrint64attr"`
		PtrUIntAttr     *uint            `json:"ptruintattr"`
		PtrUInt8Attr    *uint8           `json:"ptruint8attr"`
		PtrUInt16Attr   *uint16          `json:"ptruint16attr"`
		PtrUInt32Attr   *uint32          `json:"ptruint32attr"`
		PtrUInt64Attr   *uint64          `json:"ptruint64attr"`
		PtrFloat32Attr  *float32         `json:"ptrfloat32attr"`
		PtrFloat64Attr  *float64         `json:"ptrfloat64attr"`
		DateTimeAttr    strfmt.DateTime  `json:"datetimeattr"`
		PtrDateTimeAttr *strfmt.DateTime `json:"ptrdatetimeattr"`
		BoolAttr        bool             `json:"boolattr"`
		PtrBoolAttr     *bool            `json:"ptrboolattr"`
	}
	sMap := hc.makeJSONToAttrNameMap(SortTypes{})
	for i := range sMap {
		assert.True(sMap.isSortable(i))
	}
}

type fakeAuthAccountValidator struct {
	inCtx          context.Context
	inAI           *auth.Info
	inAction       centrald.AuditAction
	inObjID        string
	inObjName      models.ObjName
	inAttr         *centrald.UpdateAttr
	inOldIDList    []models.ObjIDMutable
	inUpdateIDList []models.ObjIDMutable

	retString string
	retError  error
}

func (f *fakeAuthAccountValidator) validateAuthorizedAccountsUpdate(ctx context.Context, ai *auth.Info, action centrald.AuditAction, objID string, objName models.ObjName, a *centrald.UpdateAttr, old, upd []models.ObjIDMutable) (string, error) {
	f.inCtx = ctx
	f.inAI = ai
	f.inAction = action
	f.inObjID = objID
	f.inObjName = objName
	f.inAttr = a
	f.inOldIDList = old
	f.inUpdateIDList = upd
	return f.retString, f.retError
}

// rotateL see https://stackoverflow.com/questions/33059420/is-this-a-reasonable-and-idiomatic-golang-circular-shift-implementation
func rotateL(a *[]string, i int) {
	x, b := (*a)[:i], (*a)[i:]
	*a = append(b, x...)
}

func requestWithAuthContext(ai *auth.Info) *http.Request {
	req := &http.Request{}
	app := context.WithValue(req.Context(), auth.InfoKey{}, ai)
	return req.WithContext(app)
}
