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
	"bytes"
	"runtime"

	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/service_debug"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
)

// register handlers for Debug
func (c *HandlerComp) debugRegisterHandlers() {
	c.app.API.ServiceDebugDebugPostHandler = ops.DebugPostHandlerFunc(c.debugPost)
}

// Handlers

// either internal or system admin user has permission to use API
func (c *HandlerComp) debugPost(params ops.DebugPostParams) middleware.Responder {
	ai, err := c.GetAuthInfo(params.HTTPRequest)
	if err != nil {
		return ops.NewDebugPostDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if err := ai.CapOK(centrald.SystemManagementCap); err != nil {
		return ops.NewDebugPostDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if params.Payload == nil {
		err = centrald.ErrorUpdateInvalidRequest
		return ops.NewDebugPostDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	if swag.BoolValue(params.Payload.Stack) {
		buf := make([]byte, 10*1024*1024)
		numBytes := runtime.Stack(buf, true)
		c.Log.Infof("STACK TRACE:\n%s\n", bytes.TrimSpace(buf[:numBytes])) // extra comment to enable testing
	}
	return ops.NewDebugPostNoContent()
}
