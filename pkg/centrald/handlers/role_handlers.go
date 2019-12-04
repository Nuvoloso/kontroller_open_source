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
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/role"
	"github.com/go-openapi/runtime/middleware"
)

// Register handlers
func (c *HandlerComp) roleRegisterHandlers() {
	c.app.API.RoleRoleFetchHandler = ops.RoleFetchHandlerFunc(c.roleFetch)
	c.app.API.RoleRoleListHandler = ops.RoleListHandlerFunc(c.roleList)
}

// Handlers

func (c *HandlerComp) roleFetch(params ops.RoleFetchParams) middleware.Responder {
	obj, err := c.DS.OpsRole().Fetch(params.ID)
	if err != nil {
		return ops.NewRoleFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewRoleFetchOK().WithPayload(obj)
}

func (c *HandlerComp) roleList(params ops.RoleListParams) middleware.Responder {
	list, err := c.DS.OpsRole().List(params)
	if err != nil {
		return ops.NewRoleListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	return ops.NewRoleListOK().WithPayload(list)
}
