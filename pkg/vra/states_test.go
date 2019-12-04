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
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestVolumeSeriesRequestState(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	rss := SupportedVolumeSeriesRequestStates()
	assert.NotNil(rss)
	assert.NotEmpty(rss)
	assert.False(ValidateVolumeSeriesRequestState(""))
	for _, rs := range rss {
		assert.True(rs != "")
		_, ok := stateData[rs]
		assert.True(ok)
		assert.True(ValidateVolumeSeriesRequestState(rs))
		assert.False(ValidateVolumeSeriesRequestState(rs + "x"))
	}

	rss = TerminalVolumeSeriesRequestStates()
	assert.NotNil(rss)
	assert.NotEmpty(rss)
	assert.Equal(rss, terminalVolumeSeriesRequestStates)
	assert.True(len(rss) < len(stateData))
	assert.False(ValidateVolumeSeriesRequestState(""))
	for _, rs := range rss {
		assert.True(rs != "")
		assert.True(ValidateVolumeSeriesRequestState(rs))
		assert.True(VolumeSeriesRequestStateIsTerminated(rs))
		assert.False(VolumeSeriesRequestStateIsTerminated(rs + "x"))
	}
}

func TestVolumeSeriesRequestOperations(t *testing.T) {
	allOps := SupportedVolumeSeriesRequestOperations()
	for op := range initialProcessOfOperation {
		assert.Contains(t, allOps, op)
	}
}

func TestVolumeSeriesStatePredicates(t *testing.T) {
	assert := assert.New(t)

	for _, s := range configuredVolumeSeriesStates {
		assert.True(VolumeSeriesIsConfigured(s))
		assert.True(VolumeSeriesIsProvisioned(s))
		assert.True(VolumeSeriesIsBound(s))
		assert.False(VolumeSeriesIsConfigured(s + "x"))
		assert.False(VolumeSeriesIsProvisioned(s + "x"))
		assert.False(VolumeSeriesIsBound(s + "x"))
	}
	assert.False(VolumeSeriesIsConfigured(common.VolStateProvisioned))

	assert.True(VolumeSeriesIsBound(common.VolStateProvisioned))
	assert.False(VolumeSeriesIsBound(common.VolStateProvisioned + "x"))
	assert.True(VolumeSeriesIsBound(common.VolStateBound))
	assert.False(VolumeSeriesIsBound(common.VolStateBound + "x"))
}

func TestVolumeSeriesIsPublished(t *testing.T) {
	assert := assert.New(t)

	vs := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: "VS-1",
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				VolumeSeriesState: "BOUND",
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				Name: "MyVolSeries",
				ClusterDescriptor: models.ClusterDescriptor{
					"k8sPvcYaml": models.ValueType{Kind: "STRING", Value: "some string"},
				},
			},
		},
	}
	assert.True(VolumeSeriesIsPublished(vs))

	for _, s := range configuredVolumeSeriesStates {
		vs.VolumeSeriesState = s
		assert.True(VolumeSeriesIsPublished(vs))
		vs.VolumeSeriesState = s + "x"
		assert.False(VolumeSeriesIsPublished(vs))
	}

	vs.ClusterDescriptor = nil
	vs.VolumeSeriesState = "BOUND"
	assert.False(VolumeSeriesIsPublished(vs))
}

func TestVolumeSeriesFsIsAttached(t *testing.T) {
	assert := assert.New(t)

	vs := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: "VS-1",
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				VolumeSeriesState: "IN_USE",
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				SystemTags: []string{fmt.Sprintf("%s:/mnt/pt", common.SystemTagVolumeFsAttached)},
			},
		},
	}

	assert.True(VolumeSeriesFsIsAttached(vs))
	vs.VolumeSeriesState = "BOUND"
	assert.False(VolumeSeriesFsIsAttached(vs))
	vs.VolumeSeriesState = "IN_USE"
	assert.True(VolumeSeriesFsIsAttached(vs))
	vs.SystemTags = nil
	assert.False(VolumeSeriesFsIsAttached(vs))
}

func TestVolumeSeriesHeadIsMounted(t *testing.T) {
	assert := assert.New(t)

	vs := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: "VS-1",
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				VolumeSeriesState: "IN_USE",
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				SystemTags: []string{fmt.Sprintf("%s:/mnt/pt", common.SystemTagVolumeFsAttached)},
			},
		},
	}
	vs.Mounts = []*models.Mount{
		&models.Mount{
			SnapIdentifier: "pit1",
			MountedNodeID:  "nodeP",
			MountState:     "MOUNTED",
		},
		&models.Mount{
			SnapIdentifier: "HEAD",
			MountedNodeID:  "nodeH",
			MountState:     "MOUNTED",
		},
	}
	assert.True(VolumeSeriesHeadIsMounted(vs))

	vs.Mounts = []*models.Mount{}
	assert.False(VolumeSeriesHeadIsMounted(vs))
}

func TestVolumeSeriesCacheIsRequested(t *testing.T) {
	assert := assert.New(t)

	vs := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: "VS-1",
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				CacheAllocations: map[string]models.CacheAllocation{
					"nodeP": models.CacheAllocation{
						RequestedSizeBytes: swag.Int64(1),
					},
				},
				ConfiguredNodeID: "nodeP",
				Mounts: []*models.Mount{
					&models.Mount{
						SnapIdentifier: "pit1",
						MountedNodeID:  "nodeP",
						MountState:     "MOUNTED",
					},
					&models.Mount{
						SnapIdentifier: "HEAD",
						MountedNodeID:  "nodeH",
						MountState:     "MOUNTED",
					},
				},
				VolumeSeriesState: "IN_USE",
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				SystemTags: []string{fmt.Sprintf("%s:/mnt/pt", common.SystemTagVolumeFsAttached)},
			},
		},
	}
	assert.True(VolumeSeriesCacheIsRequested(vs))

	m := vs.Mounts
	vs.Mounts = []*models.Mount{}
	assert.False(VolumeSeriesCacheIsRequested(vs))

	vs.Mounts = m
	vs.CacheAllocations["nodeP"] = models.CacheAllocation{}
	assert.False(VolumeSeriesCacheIsRequested(vs))

	vs.CacheAllocations = nil
	assert.False(VolumeSeriesCacheIsRequested(vs))
}

func TestStateDataMethods(t *testing.T) {
	assert := assert.New(t)

	for op, p := range initialProcessOfOperation {
		assert.NotEqual(ApUnknown, p, "operation %s", op)
		assert.Equal(p, stateData.Process(common.VolReqStateNew, op), "operation %s => %s", op, p)
	}
	assert.Equal(ApUnknown, stateData.Process(common.VolReqStateNew, ""))

	for state, sd := range stateData {
		assert.Equal(state, sd.name, "name[%s]", state)
		assert.Equal(sd.Order, stateData.Order(state), "Order[%s]", state)
		assert.Equal(sd.Undo, stateData.Undo(state), "Undo[%s]", state)
		if state != common.VolReqStateNew {
			assert.Equal(sd.Process, stateData.Process(state, ""))
		}
		si := GetStateInfo(state)
		assert.Equal(sd, si)
		assert.Equal(state, si.Name())
		switch {
		case sd.Order < soMinUndoStateOrder:
			assert.True(si.IsNormal())
			assert.False(si.IsUndo())
			assert.False(si.IsTerminal())
		case sd.Order >= soMinUndoStateOrder && sd.Order < soTerminationStatesOrder:
			assert.False(si.IsNormal())
			assert.True(si.IsUndo())
			assert.False(si.IsTerminal())
		case sd.Order >= soTerminationStatesOrder:
			assert.False(si.IsNormal())
			assert.False(si.IsUndo())
			assert.True(si.IsTerminal())
		}
	}
	assert.Equal(-1, stateData.Order(""))
	assert.Equal("", stateData.Undo(""))
	assert.Equal(ApUnknown, stateData.Process("", ""))
	assert.Panics(func() { GetStateInfo("") })

	assert.Equal("agentd", (ApAgentd).String())
	assert.Equal("centrald", (ApCentrald).String())
	assert.Equal("clusterd", (ApClusterd).String())
	assert.Equal("unknown", (ApUnknown).String())
}

func TestValidNodeStates(t *testing.T) {
	assert := assert.New(t)

	states := ValidNodeStatesForNodeDelete()
	assert.Contains(states, com.NodeStateTearDown)
	assert.Contains(states, com.NodeStateTimedOut)

	states = ValidNodeStatesForVolDetach()
	assert.Contains(states, com.NodeStateTearDown)
}
