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
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/strfmt"
	"github.com/stretchr/testify/assert"
)

func TestNodeHealth(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	hbp := 30 * time.Second
	now := time.Now()
	t1 := now.Add(-hbp)
	t0 := now.Add(-hbp * time.Duration(NodeHeartbeatThreshold+1))

	nh0 := &NodeHealth{
		NodeID:              "n0",
		State:               "READY",
		HeartbeatPeriodSecs: int64(hbp / time.Second),
		TimeLastHeartbeat:   t0,
	}
	assert.EqualValues(30, nh0.HeartbeatPeriodSecs)

	assert.Equal(1, nh0.HeartbeatsMissed(t1))
	assert.Equal(nh0.State, nh0.CookedState(t1))
	assert.Equal(2, nh0.HeartbeatsMissed(now))
	assert.Equal(nh0.State, nh0.CookedState(now.Add(-1)))
	assert.Equal(common.ServiceStateUnknown, nh0.CookedState(now))
	assert.False(nh0.IsHealthy(now))

	cs := &ClusterState{}
	cs.Log = tl.Logger()
	cs.nodeHealth = make(map[string]*NodeHealth)
	cs.nodeHealth["n0"] = nh0
	tB := time.Now()
	ni := cs.nodeHealthInsertions
	cs.NodeServiceState("n1", &models.ServiceState{State: "READY"})
	assert.Equal(ni+1, cs.nodeHealthInsertions)
	assert.Len(cs.nodeHealth, 2)
	nh1, ok := cs.nodeHealth["n1"]
	assert.True(ok)
	assert.Equal(NodeDefaultHeartbeatPeriodSecs, nh1.HeartbeatPeriodSecs)
	now = time.Now()
	assert.WithinDuration(nh1.TimeLastHeartbeat, now, now.Sub(tB))

	nhl := cs.NodeGetServiceState("n0")
	assert.Len(nhl, 1)
	assert.Equal(nh0, nhl[0])
	nhl = cs.NodeGetServiceState("foo")
	assert.NotNil(nhl)
	assert.Len(nhl, 0)

	nhl = cs.NodeGetServiceState("")
	assert.Len(nhl, 2)
	assert.Contains(nhl, nh0)
	assert.Contains(nhl, nh1)

	cs.NodeDeleted("n1")
	nhl = cs.NodeGetServiceState("")
	assert.Len(nhl, 1)
	assert.NotContains(nhl, nh1)
	assert.Contains(nhl, nh0)

	cs.NodeDeleted("foo") // no-op

	ni = cs.nodeHealthInsertions
	cs.NodeServiceState("n0", &models.ServiceState{State: "READY", HeartbeatPeriodSecs: 30})
	assert.Equal(ni, cs.nodeHealthInsertions) // was already in the map
	now = time.Now()
	assert.Equal(common.ServiceStateReady, nh0.CookedState(now))
	assert.True(nh0.IsHealthy(now))

	node := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{ID: "n0"},
		},
	}
	cs.NodeUpdateState(node, now)
	assert.NotNil(node.Service)
	assert.NotNil(node.Service.ServiceState)
	assert.Equal(nh0.CookedState(now), node.Service.State)
	assert.Equal(nh0.HeartbeatPeriodSecs, node.Service.HeartbeatPeriodSecs)
	assert.Equal(strfmt.DateTime(nh0.TimeLastHeartbeat).String(), node.Service.HeartbeatTime.String())

	node = &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{ID: "foo"},
		},
	}
	cs.NodeUpdateState(node, now)
	assert.NotNil(node.Service)
	assert.NotNil(node.Service.ServiceState)
	assert.Equal(common.ServiceStateUnknown, node.Service.State)
	assert.EqualValues(0, node.Service.HeartbeatPeriodSecs)
	assert.Equal(strfmt.DateTime(time.Time{}).String(), node.Service.HeartbeatTime.String())

	// purge
	numPurged, numInserts := cs.NodePurgeMissing(now)
	assert.Equal(0, numPurged)
	assert.Equal(1, numInserts)
	expNP := len(cs.nodeHealth)
	assert.True(expNP > 0)
	numPurged, numInserts = cs.NodePurgeMissing(now.Add(time.Hour))
	assert.Equal(expNP, numPurged)
	assert.Equal(1, numInserts)
}
