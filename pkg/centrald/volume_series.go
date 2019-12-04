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
	"github.com/Nuvoloso/kontroller/pkg/util"
)

// default states for various objects
const (
	DefaultVolumeSeriesState        = com.VolStateUnbound
	DefaultVolumeSeriesRequestState = com.VolReqStateNew
)

var supportedMountModes = []string{com.VolMountModeRO, com.VolMountModeRW}

// SupportedMountModes returns a list of supported mount modes
func (app *AppCtx) SupportedMountModes() []string {
	return supportedMountModes
}

// ValidateMountMode verifies a mount mode
func (app *AppCtx) ValidateMountMode(mode string) bool {
	return util.Contains(supportedMountModes, mode)
}

var supportedMountStates = []string{com.VolMountStateError, com.VolMountStateMounting, com.VolMountStateMounted, com.VolMountStateUnmounting}

// SupportedMountStates returns a list of supported mount states
func (app *AppCtx) SupportedMountStates() []string {
	return supportedMountStates
}

// ValidateMountState verifies a mount state
func (app *AppCtx) ValidateMountState(state string) bool {
	return util.Contains(supportedMountStates, state)
}

var supportedVolumeSeriesStates = []string{com.VolStateBound, com.VolStateConfigured, com.VolStateDeleting, com.VolStateInUse, com.VolStateProvisioned, com.VolStateUnbound}

// SupportedVolumeSeriesStates returns a list of supported volume series states
func (app *AppCtx) SupportedVolumeSeriesStates() []string {
	return supportedVolumeSeriesStates
}

// ValidateVolumeSeriesState verifies a volume series state
func (app *AppCtx) ValidateVolumeSeriesState(state string) bool {
	return util.Contains(supportedVolumeSeriesStates, state)
}
