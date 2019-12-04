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


package state

import (
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/go-openapi/strfmt"
)

// NodeHealth contains liveness information on a node
type NodeHealth struct {
	NodeID              string
	State               string
	HeartbeatPeriodSecs int64
	TimeLastHeartbeat   time.Time
}

const (
	// NodeHeartbeatThreshold is the number of heartbeats a node can miss before being considered unhealthy
	NodeHeartbeatThreshold = 1
	// NodeDefaultHeartbeatPeriodSecs is assumed if the heartbeat period is not set
	NodeDefaultHeartbeatPeriodSecs = int64(30)
)

// HeartbeatsMissed computes the number of heartbeats missed
func (nh *NodeHealth) HeartbeatsMissed(now time.Time) int {
	return int(now.Sub(nh.TimeLastHeartbeat)/time.Second) / int(nh.HeartbeatPeriodSecs)
}

// CookedState returns the state of node subject to heartbeat data
func (nh *NodeHealth) CookedState(now time.Time) string {
	if nh.HeartbeatsMissed(now) > NodeHeartbeatThreshold {
		return com.ServiceStateUnknown
	}
	return nh.State
}

// IsHealthy indicates that the node service is working
func (nh *NodeHealth) IsHealthy(now time.Time) bool {
	return nh.CookedState(now) == com.ServiceStateReady
}

// NodeServiceState reports service state information from a node
func (cs *ClusterState) NodeServiceState(nid string, ss *models.ServiceState) {
	cs.Log.Debugf("Node ServiceState: %s, %s, %d", nid, ss.State, ss.HeartbeatPeriodSecs)
	cs.hMux.Lock()
	defer cs.hMux.Unlock()
	nh, ok := cs.nodeHealth[nid]
	if !ok {
		nh = &NodeHealth{NodeID: nid}
		cs.nodeHealth[nid] = nh
		cs.nodeHealthInsertions++
	}
	nh.State = ss.State
	nh.HeartbeatPeriodSecs = ss.HeartbeatPeriodSecs
	if nh.HeartbeatPeriodSecs <= 0 {
		nh.HeartbeatPeriodSecs = NodeDefaultHeartbeatPeriodSecs
	}
	nh.TimeLastHeartbeat = time.Now()
}

// NodeGetServiceState returns the health data on nodes (optionally a specific node)
func (cs *ClusterState) NodeGetServiceState(nid string) []*NodeHealth {
	cs.hMux.Lock()
	defer cs.hMux.Unlock()
	if nid != "" {
		res := make([]*NodeHealth, 0, 1)
		if nh, ok := cs.nodeHealth[nid]; ok {
			res = append(res, nh)
		}
		return res
	}
	res := make([]*NodeHealth, 0, len(cs.nodeHealth))
	for _, nh := range cs.nodeHealth {
		res = append(res, nh)
	}
	return res
}

// NodeDeleted removes node health data
func (cs *ClusterState) NodeDeleted(nid string) {
	cs.hMux.Lock()
	defer cs.hMux.Unlock()
	delete(cs.nodeHealth, nid)
}

// NodeUpdateState sets the state in a node object from liveness data, if known
func (cs *ClusterState) NodeUpdateState(node *models.Node, epoch time.Time) {
	nhl := cs.NodeGetServiceState(string(node.Meta.ID))
	var nh *NodeHealth
	state := com.ServiceStateUnknown
	if len(nhl) != 0 {
		nh = nhl[0]
		state = nh.CookedState(epoch)
	}
	if node.Service == nil {
		node.Service = &models.NuvoService{}
	}
	node.Service.State = state
	if nh != nil {
		node.Service.HeartbeatPeriodSecs = nh.HeartbeatPeriodSecs
		node.Service.HeartbeatTime = strfmt.DateTime(nh.TimeLastHeartbeat)
	}
}

// NodePurgeMissing removes all nodes with state "UNKNOWN" and returns the number removed
// and the current value of the insertion counter. Combined, these two values
// provide an indication of change if the previous value of the insertion
// counter is maintained by the invoker.
func (cs *ClusterState) NodePurgeMissing(epoch time.Time) (int, int) {
	cs.hMux.Lock()
	defer cs.hMux.Unlock()
	missing := []string{}
	for _, nh := range cs.nodeHealth {
		if nh.CookedState(epoch) == com.ServiceStateUnknown {
			missing = append(missing, nh.NodeID)
		}
	}
	for _, nid := range missing {
		delete(cs.nodeHealth, nid)
	}
	return len(missing), cs.nodeHealthInsertions
}
