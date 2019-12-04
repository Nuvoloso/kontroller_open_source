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
	"regexp"

	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/slo"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
)

// slos is a hard-coded list of supported SLOs. This list may come from some other centrald component in the future.
var slos = []*M.SLO{
	&M.SLO{
		Name:        "Response Time Average",
		Description: "Average response time target for the storage",
		Choices: []*M.RestrictedValueType{
			{
				ValueType:                 M.ValueType{Kind: "DURATION", Value: "5ms"},
				RestrictedValueTypeAllOf1: M.RestrictedValueTypeAllOf1{Immutable: true},
			},
			{
				ValueType:                 M.ValueType{Kind: "DURATION", Value: "8ms"},
				RestrictedValueTypeAllOf1: M.RestrictedValueTypeAllOf1{Immutable: true},
			},
			{
				ValueType:                 M.ValueType{Kind: "DURATION", Value: "50ms"},
				RestrictedValueTypeAllOf1: M.RestrictedValueTypeAllOf1{Immutable: true},
			},
			{
				ValueType:                 M.ValueType{Kind: "DURATION", Value: "100ms"},
				RestrictedValueTypeAllOf1: M.RestrictedValueTypeAllOf1{Immutable: true},
			},
		},
	},
	&M.SLO{
		Name:        "Response Time Maximum",
		Description: "Maximum response time for the storage",
		Choices: []*M.RestrictedValueType{
			{
				ValueType:                 M.ValueType{Kind: "DURATION", Value: "10ms"},
				RestrictedValueTypeAllOf1: M.RestrictedValueTypeAllOf1{Immutable: true},
			},
			{
				ValueType:                 M.ValueType{Kind: "DURATION", Value: "20ms"},
				RestrictedValueTypeAllOf1: M.RestrictedValueTypeAllOf1{Immutable: true},
			},
			{
				ValueType:                 M.ValueType{Kind: "DURATION", Value: "50ms"},
				RestrictedValueTypeAllOf1: M.RestrictedValueTypeAllOf1{Immutable: true},
			},
			{
				ValueType:                 M.ValueType{Kind: "DURATION", Value: "100ms"},
				RestrictedValueTypeAllOf1: M.RestrictedValueTypeAllOf1{Immutable: true},
			},
			{
				ValueType:                 M.ValueType{Kind: "DURATION", Value: "2s"},
				RestrictedValueTypeAllOf1: M.RestrictedValueTypeAllOf1{Immutable: true},
			},
			{
				ValueType:                 M.ValueType{Kind: "DURATION", Value: "5s"},
				RestrictedValueTypeAllOf1: M.RestrictedValueTypeAllOf1{Immutable: true},
			},
		},
	},
	&M.SLO{
		Name:        "Availability",
		Description: "Availability in terms of percentage time of availability",
		Choices: []*M.RestrictedValueType{
			{
				ValueType:                 M.ValueType{Kind: "PERCENTAGE", Value: "99.999%"},
				RestrictedValueTypeAllOf1: M.RestrictedValueTypeAllOf1{Immutable: true},
			},
		},
	},
	&M.SLO{
		Name:        "Security",
		Description: "Security level in terms of data encryption",
		Choices: []*M.RestrictedValueType{
			{
				ValueType:                 M.ValueType{Kind: "STRING", Value: "High Encryption"},
				RestrictedValueTypeAllOf1: M.RestrictedValueTypeAllOf1{Immutable: true},
			},
			{
				ValueType:                 M.ValueType{Kind: "STRING", Value: "No Encryption"},
				RestrictedValueTypeAllOf1: M.RestrictedValueTypeAllOf1{Immutable: true},
			},
		},
	},
	&M.SLO{
		Name:        "RPO",
		Description: "Recovery point objective, the maximum time period in which data might be lost due to an incident",
		Choices: []*M.RestrictedValueType{
			{
				ValueType:                 M.ValueType{Kind: "DURATION", Value: "N/A"},
				RestrictedValueTypeAllOf1: M.RestrictedValueTypeAllOf1{Immutable: true},
			},
			{
				ValueType:                 M.ValueType{Kind: "DURATION", Value: "1h"},
				RestrictedValueTypeAllOf1: M.RestrictedValueTypeAllOf1{Immutable: true},
			},
			{
				ValueType:                 M.ValueType{Kind: "DURATION", Value: "4h"},
				RestrictedValueTypeAllOf1: M.RestrictedValueTypeAllOf1{Immutable: true},
			},
			{
				ValueType:                 M.ValueType{Kind: "DURATION", Value: "6h"},
				RestrictedValueTypeAllOf1: M.RestrictedValueTypeAllOf1{Immutable: true},
			},
			{
				ValueType:                 M.ValueType{Kind: "DURATION", Value: "24h"},
				RestrictedValueTypeAllOf1: M.RestrictedValueTypeAllOf1{Immutable: true},
			},
		},
	},
	&M.SLO{
		Name:        "Retention",
		Description: "Retention time for snapshots",
		Choices: []*M.RestrictedValueType{
			{
				ValueType:                 M.ValueType{Kind: "DURATION", Value: "N/A"},
				RestrictedValueTypeAllOf1: M.RestrictedValueTypeAllOf1{Immutable: true},
			},
			{
				ValueType:                 M.ValueType{Kind: "DURATION", Value: "1d"},
				RestrictedValueTypeAllOf1: M.RestrictedValueTypeAllOf1{Immutable: true},
			},
			{
				ValueType:                 M.ValueType{Kind: "DURATION", Value: "5d"},
				RestrictedValueTypeAllOf1: M.RestrictedValueTypeAllOf1{Immutable: true},
			},
			{
				ValueType:                 M.ValueType{Kind: "DURATION", Value: "14d"},
				RestrictedValueTypeAllOf1: M.RestrictedValueTypeAllOf1{Immutable: true},
			},
			{
				ValueType:                 M.ValueType{Kind: "DURATION", Value: "30d"},
				RestrictedValueTypeAllOf1: M.RestrictedValueTypeAllOf1{Immutable: true},
			},
		},
	},
}

// Register handlers for SLO
func (c *HandlerComp) sloRegisterHandlers() {
	c.app.API.SloSloListHandler = ops.SloListHandlerFunc(c.sloList)
}

// Handlers

func (c *HandlerComp) sloList(params ops.SloListParams) middleware.Responder {
	pattern := swag.StringValue(params.NamePattern) // NB empty pattern matches all values
	re, err := regexp.Compile(pattern)
	if err != nil {
		err = &centrald.Error{M: "invalid or unsupported namePattern: " + err.Error(), C: http.StatusBadRequest}
		return ops.NewSloListDefault(c.eCode(err)).WithPayload(c.eError(err))
	}
	var list []*M.SLO
	for _, slo := range slos {
		if re.MatchString(string(slo.Name)) {
			list = append(list, slo)
		}
	}
	return ops.NewSloListOK().WithPayload(list)
}
