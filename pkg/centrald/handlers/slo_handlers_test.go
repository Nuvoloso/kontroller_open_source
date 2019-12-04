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
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/slo"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestSloList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	params := ops.SloListParams{}

	// success, no params
	t.Log("case: success")
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.sloList(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.SloListOK)
	assert.True(ok)
	assert.Equal(slos, mO.Payload)
	tl.Flush()

	// success, valid pattern
	t.Log("case: success with pattern")
	params.NamePattern = swag.String("^" + string(slos[0].Name)[:2])
	assert.NotPanics(func() { ret = hc.sloList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.SloListOK)
	assert.True(ok)
	assert.NotEmpty(mO.Payload)
	assert.True(len(slos) > len(mO.Payload))
	tl.Flush()

	// success, pattern does not match
	t.Log("case: success with pattern")
	params.NamePattern = swag.String("nothing to see here")
	assert.NotPanics(func() { ret = hc.sloList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.SloListOK)
	assert.True(ok)
	assert.Empty(mO.Payload)
	tl.Flush()

	// list fails
	t.Log("case: list failure")
	params.NamePattern = swag.String("Re(")
	assert.NotPanics(func() { ret = hc.sloList(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.SloListDefault)
	assert.True(ok)
	assert.Equal(400, int(mD.Payload.Code))
	assert.Regexp("namePattern", *mD.Payload.Message)
}
