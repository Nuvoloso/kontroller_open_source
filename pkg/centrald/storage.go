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
	"fmt"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
)

// default states for various objects
const (
	DefaultStorageAttachmentState  = com.StgAttachmentStateDetached
	DefaultStorageDeviceState      = com.StgDeviceStateUnused
	DefaultStorageMediaState       = com.StgMediaStateUnformatted
	DefaultStorageProvisionedState = com.StgProvisionedStateUnprovisioned
	DefaultStorageRequestState     = com.StgReqStateNew
)

var supportedStorageAccessibilityScopes = []string{
	com.AccScopeNode,
	com.AccScopeCspDomain,
}

// SupportedStorageAccessibilityScopes returns a list of supported storage accessibility scopes
func (app *AppCtx) SupportedStorageAccessibilityScopes() []string {
	return supportedStorageAccessibilityScopes
}

// ValidateStorageAccessibility verifies the properties of the StorageAccessibility.
// The datastore is not queried, so the caller must perform node or cluster existence checks.
func (app *AppCtx) ValidateStorageAccessibility(cspDomainID models.ObjIDMutable,
	cspStorageType models.CspStorageType, sa *models.StorageAccessibilityMutable) error {
	if sa == nil {
		return fmt.Errorf("storageAccessibility is required")
	} else if sa.AccessibilityScope == com.AccScopeCspDomain {
		if sa.AccessibilityScopeObjID != cspDomainID {
			return fmt.Errorf("cspDomainId and accessibilityScopeObjId differ for accessibilityScope CSPDOMAIN")
		}
	} else if sa.AccessibilityScope == com.AccScopeNode {
		if sa.AccessibilityScopeObjID == "" {
			return fmt.Errorf("NODE accessibilityScope requires an accessibilityScopeObjId")
		}
	} else {
		return fmt.Errorf("accessibilityScope must be one of %s", app.SupportedStorageAccessibilityScopes())
	}
	storageType := app.GetCspStorageType(cspStorageType)
	if storageType == nil {
		return fmt.Errorf("invalid cspStorageType")
	} else if sa.AccessibilityScope != storageType.AccessibilityScope {
		return fmt.Errorf("accessibilityScope %s is incompatible with cspStorageType %s", sa.AccessibilityScope, string(cspStorageType))
	}
	return nil
}

var supportedStorageAttachmentStates = []string{
	DefaultStorageAttachmentState,
	com.StgAttachmentStateAttached,
	com.StgAttachmentStateDetaching,
	com.StgAttachmentStateAttaching,
	com.StgAttachmentStateError,
}

// SupportedStorageAttachmentStates returns a list of supported storage attachment states
func (app *AppCtx) SupportedStorageAttachmentStates() []string {
	return supportedStorageAttachmentStates
}

// ValidateStorageAttachmentState verifies a storage attachment state
func (app *AppCtx) ValidateStorageAttachmentState(state string) bool {
	return util.Contains(supportedStorageAttachmentStates, state)
}

var supportedStorageDeviceStates = []string{
	DefaultStorageDeviceState,
	com.StgDeviceStateFormatting,
	com.StgDeviceStateOpening,
	com.StgDeviceStateOpen,
	com.StgDeviceStateClosing,
	com.StgDeviceStateError,
}

// SupportedStorageDeviceStates returns a list of supported storage attachment states
func (app *AppCtx) SupportedStorageDeviceStates() []string {
	return supportedStorageDeviceStates
}

// ValidateStorageDeviceState verifies a storage attachment state
func (app *AppCtx) ValidateStorageDeviceState(state string) bool {
	return util.Contains(supportedStorageDeviceStates, state)
}

var supportedStorageMediaStates = []string{
	DefaultStorageMediaState,
	com.StgMediaStateFormatted,
}

// SupportedStorageMediaStates returns a list of supported storage attachment states
func (app *AppCtx) SupportedStorageMediaStates() []string {
	return supportedStorageMediaStates
}

// ValidateStorageMediaState verifies a storage attachment state
func (app *AppCtx) ValidateStorageMediaState(state string) bool {
	return util.Contains(supportedStorageMediaStates, state)
}

var supportedStorageProvisionedStates = []string{
	DefaultStorageProvisionedState,
	com.StgProvisionedStateProvisioning,
	com.StgProvisionedStateProvisioned,
	com.StgProvisionedStateUnprovisioning,
	com.StgProvisionedStateError,
}

// SupportedStorageProvisionedStates returns a list of supported storage attachment states
func (app *AppCtx) SupportedStorageProvisionedStates() []string {
	return supportedStorageProvisionedStates
}

// ValidateStorageProvisionedState verifies a storage attachment state
func (app *AppCtx) ValidateStorageProvisionedState(state string) bool {
	return util.Contains(supportedStorageProvisionedStates, state)
}

var supportedStorageRequestStates = []string{
	DefaultStorageRequestState,
	com.StgReqStateCapacityWait,
	com.StgReqStateProvisioning,
	com.StgReqStateAttaching,
	com.StgReqStateFormatting,
	com.StgReqStateUsing,
	com.StgReqStateClosing,
	com.StgReqStateDetaching,
	com.StgReqStateReattaching,
	com.StgReqStateReleasing,
	com.StgReqStateRemovingTag,
	com.StgReqStateUndoAttaching,
	com.StgReqStateUndoDetaching,
	com.StgReqStateUndoProvisioning,
	com.StgReqStateSucceeded,
	com.StgReqStateFailed,
}

// SupportedStorageRequestStates returns a list of supported storage request states
func (app *AppCtx) SupportedStorageRequestStates() []string {
	return supportedStorageRequestStates
}

// ValidateStorageRequestState verifies a storage request state
func (app *AppCtx) ValidateStorageRequestState(state string) bool {
	return util.Contains(supportedStorageRequestStates, state)
}

// TerminalStorageRequestStates returns a list of terminal storage request states
func (app *AppCtx) TerminalStorageRequestStates() []string {
	return []string{com.StgReqStateSucceeded, com.StgReqStateFailed}
}
