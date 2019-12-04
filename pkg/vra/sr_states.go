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


package vra

import (
	"fmt"

	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
)

var terminalStorageRequestStates = []string{
	com.StgReqStateSucceeded,
	com.StgReqStateFailed,
}

// TerminalStorageRequestStates returns a list of terminal storageRequest states
func TerminalStorageRequestStates() []string {
	return terminalStorageRequestStates
}

// StorageRequestStateIsTerminated verifies if a storageRequest state is a terminal state
func StorageRequestStateIsTerminated(state string) bool {
	return util.Contains(terminalStorageRequestStates, state)
}

// GetSRProcess returns the process animating the state or ApUnknown
func GetSRProcess(state string, firstOp string) AnimatorProcess {
	if state == com.StgReqStateNew {
		ap, found := initialSRProcessOfOperation[firstOp]
		if !found {
			return ApUnknown
		}
		return ap
	}
	ap, ok := srStateToProcess[state]
	if !ok {
		panic(fmt.Sprintf("getSRAnimatorProcess: unknown state '%s'", state))
	}
	return ap
}

var initialSRProcessOfOperation = map[string]AnimatorProcess{
	com.StgReqOpAttach:    ApCentrald,
	com.StgReqOpClose:     ApAgentd,
	com.StgReqOpDetach:    ApCentrald,
	com.StgReqOpFormat:    ApAgentd,
	com.StgReqOpProvision: ApCentrald,
	com.StgReqOpReattach:  ApAgentd, // does not handle partial progress cases correctly
	com.StgReqOpRelease:   ApCentrald,
	com.StgReqOpUse:       ApAgentd,
}

var srStateToProcess = map[string]AnimatorProcess{
	com.StgReqStateAttaching:        ApCentrald,
	com.StgReqStateCapacityWait:     ApCentrald,
	com.StgReqStateClosing:          ApAgentd,
	com.StgReqStateDetaching:        ApCentrald,
	com.StgReqStateFailed:           ApUnknown,
	com.StgReqStateFormatting:       ApAgentd,
	com.StgReqStateNew:              ApUnknown,
	com.StgReqStateProvisioning:     ApCentrald,
	com.StgReqStateReattaching:      ApCentrald,
	com.StgReqStateReleasing:        ApCentrald,
	com.StgReqStateRemovingTag:      ApCentrald,
	com.StgReqStateSucceeded:        ApUnknown,
	com.StgReqStateUndoAttaching:    ApCentrald,
	com.StgReqStateUndoDetaching:    ApCentrald,
	com.StgReqStateUndoProvisioning: ApCentrald,
	com.StgReqStateUsing:            ApAgentd,
}
