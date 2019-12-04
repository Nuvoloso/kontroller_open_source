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
	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/csp_storage_type"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
)

// Register handlers
func (c *HandlerComp) cspStorageTypesRegisterHandlers() {
	c.app.API.CspStorageTypeCspStorageTypeFetchHandler = ops.CspStorageTypeFetchHandlerFunc(c.cspStorageTypeFetch)
	c.app.API.CspStorageTypeCspStorageTypeListHandler = ops.CspStorageTypeListHandlerFunc(c.cspStorageTypeList)
}

// Handlers

func (c *HandlerComp) cspStorageTypeFetch(params ops.CspStorageTypeFetchParams) middleware.Responder {
	if cspStorageType := c.app.GetCspStorageType(M.CspStorageType(params.Name)); cspStorageType != nil {
		return ops.NewCspStorageTypeFetchOK().WithPayload(cspStorageType)
	}
	err := centrald.ErrorNotFound
	return ops.NewCspStorageTypeFetchDefault(c.eCode(err)).WithPayload(c.eError(err))
}

func (c *HandlerComp) cspStorageTypeList(params ops.CspStorageTypeListParams) middleware.Responder {
	cspDomainType := swag.StringValue(params.CspDomainType)
	list := make([]*M.CSPStorageType, 0)
	for _, cspStorageType := range c.app.SupportedCspStorageTypes() {
		if cspDomainType == "" || cspDomainType == string(cspStorageType.CspDomainType) {
			list = append(list, cspStorageType)
		}
	}
	return ops.NewCspStorageTypeListOK().WithPayload(list)
}
