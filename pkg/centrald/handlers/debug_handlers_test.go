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
	"net/http"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/service_debug"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestDebugPost(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}

	params := ops.DebugPostParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	var ret middleware.Responder

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()

	t.Log("case: no payload")
	assert.NotPanics(func() { ret = hc.debugPost(params) })
	mE, ok := ret.(*ops.DebugPostDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUpdateInvalidRequest.M, *mE.Payload.Message)
	tl.Flush()

	t.Log("case: success, no request to dump stack")
	params.Payload = &models.DebugSettings{}
	assert.NotPanics(func() { ret = hc.debugPost(params) })
	assert.NotNil(ret)
	_, ok = ret.(*ops.DebugPostNoContent)
	assert.True(ok)
	tl.Flush()

	t.Log("case: success, request to dump stack")
	params.Payload.Stack = swag.Bool(true)
	assert.NotPanics(func() { ret = hc.debugPost(params) })
	assert.NotNil(ret)
	_, ok = ret.(*ops.DebugPostNoContent)
	assert.True(ok)
	assert.True(tl.CountPattern("STACK TRACE:") > 0)
	tl.Flush()

	t.Log("case: unauthorized")
	ai.RoleObj = &models.Role{}
	ai.AccountID = "some_account"
	assert.NotPanics(func() { ret = hc.debugPost(params) })
	mE, ok = ret.(*ops.DebugPostDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	tl.Flush()

	t.Log("case: SystemManagementCap capability succeeds")
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.SystemManagementCap: true}
	params.Payload = &models.DebugSettings{}
	params.Payload.Stack = swag.Bool(true)
	assert.NotPanics(func() { ret = hc.debugPost(params) })
	assert.NotNil(ret)
	_, ok = ret.(*ops.DebugPostNoContent)
	assert.True(ok)
	assert.True(tl.CountPattern("STACK TRACE:") > 0)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.debugPost(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.DebugPostDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()
}
