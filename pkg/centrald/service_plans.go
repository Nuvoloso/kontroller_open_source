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


package centrald

import (
	com "github.com/Nuvoloso/kontroller/pkg/common"
)

// The states of a service plan
const (
	UnpublishedState = "UNPUBLISHED"
	PublishedState   = "PUBLISHED"
	RetiredState     = "RETIRED"
)

var supportedServicePlanAllocationReservationStates = []string{
	com.SPAReservationStateUnknown,
	com.SPAReservationStateOk,
	com.SPAReservationStateNoCapacity,
	com.SPAReservationStateDisabled,
}

// DefaultServicePlanAllocationReservationState is the initial state
const DefaultServicePlanAllocationReservationState = com.SPAReservationStateUnknown

// SupportedServicePlanAllocationReservationStates returns the list of supported reservation states
func (app *AppCtx) SupportedServicePlanAllocationReservationStates() []string {
	return supportedServicePlanAllocationReservationStates
}

// ValidateServicePlanAllocationReservationState validates the SPA reservation state
func (app *AppCtx) ValidateServicePlanAllocationReservationState(state string) bool {
	for _, s := range supportedServicePlanAllocationReservationStates {
		if state == s {
			return true
		}
	}
	return false
}
