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
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	csptype_ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/csp_storage_type"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/storage_formula"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestStorageFormulaList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app

	types := app.SupportedStorageFormulas()
	assert.NotNil(types)
	assert.NotEmpty(types)

	// validate all Storage Formulas reference actual StorageTypes
	// In an ideal world this would live in storage_formula_test.go, but
	// I need to call the Handler for StorageType to do the validation.
	t.Log("case: success, all storage formulas reference real storage types")
	cspStorageTypeParams := csptype_ops.CspStorageTypeFetchParams{}
	for _, formula := range types {
		for key := range formula.StorageComponent {
			cspStorageTypeParams.Name = key
			ret := hc.cspStorageTypeFetch(cspStorageTypeParams)
			assert.NotNil(ret)
			mO, ok := ret.(*csptype_ops.CspStorageTypeFetchOK)
			assert.True(ok)
			assert.NotNil(mO.Payload)
		}
	}
	tl.Flush()

	// validate all Cache Formulas reference actual StorageTypes
	// In an ideal world this would live in storage_formula_test.go, but
	// I need to call the Handler for StorageType to do the validation.
	t.Log("case: success, all cache formulas reference real storage types")
	for _, formula := range types {
		for key := range formula.CacheComponent {
			cspStorageTypeParams.Name = key
			ret := hc.cspStorageTypeFetch(cspStorageTypeParams)
			assert.NotNil(ret)
			mO, ok := ret.(*csptype_ops.CspStorageTypeFetchOK)
			assert.True(ok)
			assert.NotNil(mO.Payload)
		}
	}
	tl.Flush()

	// success, no parameters
	t.Log("case: success, empty params")
	assert.NotEmpty(types)
	var ret middleware.Responder
	params := ops.StorageFormulaListParams{}
	assert.NotPanics(func() { ret = hc.storageFormulaList(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.StorageFormulaListOK)
	assert.True(ok)
	assert.Len(mO.Payload, len(types))
	assert.ElementsMatch(types, mO.Payload)
	tl.Flush()

	// success, match on Name
	t.Log("case: success, name match ", types[0].Name)
	params.Name = swag.String(string(types[0].Name))
	assert.NotPanics(func() { ret = hc.storageFormulaList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.StorageFormulaListOK)
	assert.True(ok)
	assert.NotEmpty(mO.Payload)
	for _, val := range mO.Payload {
		assert.Equal(types[0].Name, val.Name)
	}
	assert.True(len(types) > len(mO.Payload))
	params.Name = nil
	tl.Flush()

	// success, match on Name and on cspDomainType
	t.Log("case: success, name and domainType match ", types[0].Name, types[0].CspDomainType)
	params.Name = swag.String(string(types[0].Name))
	params.CspDomainType = swag.String(string(types[0].CspDomainType))
	assert.NotPanics(func() { ret = hc.storageFormulaList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.StorageFormulaListOK)
	assert.True(ok)
	assert.NotEmpty(mO.Payload)
	for _, val := range mO.Payload {
		assert.Equal(types[0].Name, val.Name)
		assert.Equal(types[0].CspDomainType, val.CspDomainType)
	}
	assert.True(len(types) > len(mO.Payload))
	params.Name = nil
	tl.Flush()

	// success, no match on Name
	t.Log("case: fail, name mismatch")
	params.Name = swag.String(string(types[0].Name) + "xxx")
	assert.NotPanics(func() { ret = hc.storageFormulaList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.StorageFormulaListOK)
	assert.True(ok)
	assert.Empty(mO.Payload)
	params.Name = nil
	tl.Flush()

	// success, no match on cspDomainType
	t.Log("case: fail, cspDomainType mismatch")
	params.Name = swag.String(string(types[0].Name))
	params.CspDomainType = swag.String(string(types[0].CspDomainType + "xxx"))
	assert.NotPanics(func() { ret = hc.storageFormulaList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.StorageFormulaListOK)
	assert.True(ok)
	assert.Empty(mO.Payload)
	params.Name = nil
	tl.Flush()
}
