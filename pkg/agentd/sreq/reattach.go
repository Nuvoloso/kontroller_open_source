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


package sreq

import (
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
)

// reattachGetStates returns the appropriate states needed for this invocation of a REATTACH operation.
// REATTACH is composed of the special state sequence of CLOSING + DETACHING + REATTACHING + ATTACHING + USING
// involving 2 different agentds and centrald.
// It is expected to be reattempted (by placement) until it succeeds; hence the operations to perform
// are determined by the Storage object state.
func (c *SRComp) reattachGetStates(rhs *requestHandlerState) []string {
	var states []string
	ss := rhs.Storage.StorageState
	c.Log.Debugf("StorageRequest %s REATTACH ss:%#v", rhs.Request.Meta.ID, ss)
	if ss.AttachmentState == com.StgAttachmentStateAttached && ss.AttachedNodeID == c.thisNodeID {
		if ss.AttachedNodeID == rhs.Request.ReattachNodeID { // attached to final node
			if util.Contains([]string{com.StgDeviceStateOpening, com.StgDeviceStateUnused, com.StgDeviceStateError}, ss.DeviceState) {
				states = append(states, com.StgReqStateUsing)
			} else if ss.DeviceState != com.StgDeviceStateOpen {
				rhs.setRequestError("Unexpected device state %s", ss.DeviceState)
			} // else already open - SR will terminate
		} else { // attached to original node
			if util.Contains([]string{com.StgDeviceStateOpen, com.StgDeviceStateClosing, com.StgDeviceStateError, com.StgDeviceStateUnused}, ss.DeviceState) {
				if ss.DeviceState != com.StgDeviceStateUnused {
					states = append(states, com.StgReqStateClosing) // not yet closed
				}
				states = append(states, com.StgReqStateDetaching) // hand over to centrald when done
			} else {
				rhs.setRequestError("Unexpected device state %s", ss.DeviceState)
			}
		}
	} else {
		rhs.RetryLater = true // skip
	}
	return states
}
