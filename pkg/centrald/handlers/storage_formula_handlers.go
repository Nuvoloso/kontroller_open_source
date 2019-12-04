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
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/storage_formula"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
)

// Register handlers
func (c *HandlerComp) storageFormulaRegisterHandlers() {
	c.app.API.StorageFormulaStorageFormulaListHandler = ops.StorageFormulaListHandlerFunc(c.storageFormulaList)
}

// Handlers
func (c *HandlerComp) storageFormulaList(params ops.StorageFormulaListParams) middleware.Responder {
	name := swag.StringValue(params.Name)
	cspDomainType := swag.StringValue(params.CspDomainType)

	list := make([]*M.StorageFormula, 0)
	for _, storageFormula := range c.app.SupportedStorageFormulas() {
		if name == "" || name == string(storageFormula.Name) {
			if cspDomainType == "" || cspDomainType == string(storageFormula.CspDomainType) {
				list = append(list, storageFormula)
			}
		}
	}
	return ops.NewStorageFormulaListOK().WithPayload(list)
}
