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


package vreq

import (
	"context"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	fakeState "github.com/Nuvoloso/kontroller/pkg/clusterd/state/fake"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestChooseNodeSteps(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	fso := fakeState.NewFakeClusterState()
	app.StateOps = fso

	c := newComponent()
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc
	ctx := context.Background()

	// Invoke the steps in order (success cases last to fall through)
	now := time.Now()
	vr := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:      "VR-1",
				Version: 1,
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			ClusterID:           "cl1",
			CompleteByTime:      strfmt.DateTime(now),
			RequestedOperations: []string{"MOUNT"},
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				CapacityReservationPlan:  &models.CapacityReservationPlan{},
				RequestMessages:          []*models.TimestampedString{},
				StoragePlan:              &models.StoragePlan{},
				VolumeSeriesRequestState: "NEW",
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID:         "node1",
				VolumeSeriesID: "VS-1",
			},
		},
	}
	rhs := &vra.RequestHandlerState{
		A:         c.Animator,
		HasDelete: true,
		Request:   vr,
	}
	op := &chooseNodeOp{
		c:   c,
		rhs: rhs,
	}

	// ***************************** getInitialState
	op.rhs.TimedOut = true
	assert.Equal(ChooseNodeDone, op.getInitialState(ctx))
	op.rhs.TimedOut = false

	op.rhs.Canceling = true
	assert.Equal(ChooseNodeDone, op.getInitialState(ctx))
	op.rhs.Canceling = false

	op.rhs.InError = true
	assert.Equal(ChooseNodeDone, op.getInitialState(ctx))
	op.rhs.InError = false

	// case: nodeId is set
	assert.Equal(ChooseNodeDone, op.getInitialState(ctx))

	// case: nodeId is not set
	vr.NodeID = ""
	assert.Equal(ChooseNodeSelectHealthyNode, op.getInitialState(ctx))

	// ***************************** selectHealthyNode
	// case: planOnly
	vr.PlanOnly = swag.Bool(true)
	op.selectHealthyNode(ctx)
	if assert.Len(vr.RequestMessages, 1) {
		assert.Regexp("Plan: choose a healthy node", op.rhs.Request.RequestMessages[0].Message)
		vr.RequestMessages = vr.RequestMessages[:0]
	}
	vr.PlanOnly = nil
	assert.False(rhs.RetryLater)
	assert.False(rhs.InError)

	// case: rei
	c.rei.SetProperty("choose-node-block-on-select", &rei.Property{BoolValue: true})
	op.selectHealthyNode(ctx)
	if assert.Len(vr.RequestMessages, 1) {
		assert.Regexp("choose-node-block-on-select", op.rhs.Request.RequestMessages[0].Message)
		vr.RequestMessages = vr.RequestMessages[:0]
	}
	assert.True(rhs.RetryLater)
	assert.False(rhs.InError)
	op.rhs.RetryLater = false // reset

	// case: no healthy node
	fso.RetGHNobj = nil
	op.selectHealthyNode(ctx)
	assert.Empty(fso.InGHNid)
	if assert.Len(vr.RequestMessages, 1) {
		assert.Regexp("waiting for a healthy node", op.rhs.Request.RequestMessages[0].Message)
	}
	assert.True(rhs.RetryLater)
	assert.False(rhs.InError)
	op.rhs.RetryLater = false // reset

	// case: no healthy node, no new message
	op.selectHealthyNode(ctx)
	assert.Len(vr.RequestMessages, 1)
	assert.True(rhs.RetryLater)
	assert.False(rhs.InError)
	vr.RequestMessages = vr.RequestMessages[:0]
	op.rhs.RetryLater = false // reset

	// case: success
	fso.RetGHNobj = &models.Node{
		NodeAllOf0: models.NodeAllOf0{Meta: &models.ObjMeta{ID: "nodeID"}},
	}
	op.selectHealthyNode(ctx)
	assert.EqualValues("nodeID", vr.NodeID)
	assert.Empty(vr.RequestMessages)
	assert.False(rhs.RetryLater)
	assert.False(rhs.InError)
}

func TestChooseNode(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	c := newComponent()
	c.Init(app)

	vr := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VR-1",
			},
		},
	}

	op := &fakeChooseNodeOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = ChooseNodeSelectHealthyNode
	expCalled := []string{"GIS", "SHN"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op.retGIS = ChooseNodeDone
	op.called = op.called[:0]
	expCalled = []string{"GIS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// call the real handler with an error

	vr = &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VR-1",
			},
		},
	}
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc
	rhs := &vra.RequestHandlerState{
		A:       c.Animator,
		InError: true,
		Request: vr,
	}
	c.ChooseNode(nil, rhs)

	// check place state strings exist up to SizeNoOp
	var ss chooseNodeSubState
	for ss = ChooseNodeSelectHealthyNode; ss < ChooseNodeNoOp; ss++ {
		s := ss.String()
		tl.Logger().Debugf("Testing %d %s", ss, s)
		assert.Regexp("^ChooseNode", s)
	}
	assert.Regexp("^chooseNodeSubState", ss.String())
}

type fakeChooseNodeOps struct {
	chooseNodeOp
	called []string
	retGIS chooseNodeSubState
}

func (op *fakeChooseNodeOps) getInitialState(ctx context.Context) chooseNodeSubState {
	op.called = append(op.called, "GIS")
	return op.retGIS
}

func (op *fakeChooseNodeOps) selectHealthyNode(ctx context.Context) {
	op.called = append(op.called, "SHN")
}
