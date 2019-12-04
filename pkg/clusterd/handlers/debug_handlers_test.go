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
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/service_debug"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	com "github.com/Nuvoloso/kontroller/pkg/common"
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
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log: tl.Logger(),
		},
	}
	hc := newHandlerComp()
	hc.app = app
	hc.Log = tl.Logger()

	req := &http.Request{RemoteAddr: ""}
	params := ops.DebugPostParams{
		HTTPRequest: req,
	}
	var ret middleware.Responder

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()

	t.Log("case: no payload")
	assert.NotPanics(func() { ret = hc.debugPost(params) })
	mE, ok := ret.(*ops.DebugPostDefault)
	assert.True(ok)
	assert.Equal(com.ErrorUpdateInvalidRequest, *mE.Payload.Message)
	assert.Equal(http.StatusBadRequest, int(mE.Payload.Code))
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
	req.RemoteAddr = "1.2.3.4"
	params.HTTPRequest = req
	assert.NotPanics(func() { ret = hc.debugPost(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.DebugPostDefault)
	assert.True(ok)
	assert.Equal(http.StatusForbidden, int(mE.Payload.Code))
	assert.Equal(com.ErrorUnauthorizedOrForbidden, *mE.Payload.Message)
	tl.Flush()
}
