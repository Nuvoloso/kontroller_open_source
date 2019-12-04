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
	"context"

	com "github.com/Nuvoloso/kontroller/pkg/common"
)

// reattachGetStates returns the appropriate states needed for this invocation of a REATTACH operation.
// REATTACH is composed of the special state sequence of CLOSING + DETACHING + REATTACHING + ATTACHING + USING
// involving 2 different agentds and centrald.
// It is expected to be reattempted (by placement) until it succeeds; hence the operations to perform
// are determined by the Storage object state.
func (c *Component) reattachGetStates(rhs *requestHandlerState) []string {
	var states []string
	ss := rhs.Storage.StorageState
	c.Log.Debugf("StorageRequest %s REATTACH ss:%#v rn:%s", rhs.Request.Meta.ID, ss, rhs.Request.ReattachNodeID)
	if ss.AttachmentState == com.StgAttachmentStateAttached {
		if ss.AttachedNodeID == rhs.Request.ReattachNodeID { // attached to final node
			states = append(states, com.StgReqStateUsing)
		} else { // attached to original node
			if ss.DeviceState != com.StgDeviceStateUnused {
				states = append(states, com.StgReqStateClosing)
			} else {
				states = append(states, com.StgReqStateDetaching, com.StgReqStateReattaching, com.StgReqStateAttaching, com.StgReqStateUsing)
			}
		}
	} else { // not attached (in transition)
		if rhs.Request.NodeID != rhs.Request.ReattachNodeID {
			states = append(states, com.StgReqStateReattaching)
		}
		states = append(states, com.StgReqStateAttaching, com.StgReqStateUsing)
	}
	return states
}

// ReattachSwitchNodes changes the request to the second node.
func (c *Component) ReattachSwitchNodes(ctx context.Context, rhs *requestHandlerState) {
	rhs.setRequestMessage("nodeId change: %s ⇒ %s", rhs.Request.NodeID, rhs.Request.ReattachNodeID)
	rhs.Request.NodeID = rhs.Request.ReattachNodeID
	rhs.SetNodeID = true
}

// Centrald may have to handle a new REATTACH SR if the storage object has made partial progress
func (c *Component) dispatchNewReattachSR(rhs *requestHandlerState) bool {
	if rhs.Request.StorageRequestState != com.StgReqStateNew {
		panic("invalid invocation of dispatchNewReattachSR")
	}
	ss := rhs.Storage.StorageState
	// storage closed but not detached from original node
	if ss.DeviceState == com.StgDeviceStateUnused &&
		ss.AttachmentState != com.StgAttachmentStateDetached &&
		ss.AttachedNodeID != rhs.Request.ReattachNodeID {
		return true
	}
	// storage detached (expected in node failure recovery)
	if ss.AttachmentState == com.StgAttachmentStateDetached {
		return true
	}
	return false
}
