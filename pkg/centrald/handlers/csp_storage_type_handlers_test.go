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

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/csp_storage_type"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestCspStorageTypeFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	params := ops.CspStorageTypeFetchParams{}

	// success
	types := app.SupportedCspStorageTypes()
	assert.True(len(types) > 2)
	params.Name = string(types[1].Name)
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.cspStorageTypeFetch(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.CspStorageTypeFetchOK)
	assert.True(ok)
	assert.NotNil(mO.Payload)
	assert.Equal(*types[1], *mO.Payload)
	tl.Flush()

	// not found
	params.Name = "not dead yet"
	assert.NotPanics(func() { ret = hc.cspStorageTypeFetch(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.CspStorageTypeFetchDefault)
	assert.True(ok)
	assert.NotNil(mD.Payload)
	assert.Equal(centrald.ErrorNotFound.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mD.Payload.Message)
}

func TestCspStorageTypeList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	params := ops.CspStorageTypeListParams{}

	// success, no parameters
	t.Log("case: success")
	types := app.SupportedCspStorageTypes()
	assert.NotEmpty(types)
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.cspStorageTypeList(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.CspStorageTypeListOK)
	assert.True(ok)
	assert.Equal(types, mO.Payload)
	tl.Flush()

	// success, single cspDomainType
	oneCSPTypes := []*models.CSPStorageType{}
	for _, t := range types {
		if t.CspDomainType == types[0].CspDomainType {
			oneCSPTypes = append(oneCSPTypes, t)
		}
	}
	params.CspDomainType = swag.String(string(types[0].CspDomainType))
	assert.NotPanics(func() { ret = hc.cspStorageTypeList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.CspStorageTypeListOK)
	assert.True(ok)
	assert.Equal(oneCSPTypes, mO.Payload)
	tl.Flush()

	// success, no match
	params.CspDomainType = swag.String(string(types[0].CspDomainType) + "xxx")
	assert.NotPanics(func() { ret = hc.cspStorageTypeList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.CspStorageTypeListOK)
	assert.True(ok)
	assert.Empty(mO.Payload)
}
