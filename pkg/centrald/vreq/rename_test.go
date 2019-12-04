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
	"errors"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	fhk "github.com/Nuvoloso/kontroller/pkg/housekeeping/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestRenameSteps(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	fts := &fhk.TaskScheduler{}
	app.TaskScheduler = fts

	c := &Component{}
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
			RequestedOperations: []string{"RENAME"},
			VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{
				VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
					Name: "renamed",
				},
			},
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				CapacityReservationPlan:  &models.CapacityReservationPlan{},
				RequestMessages:          []*models.TimestampedString{},
				StoragePlan:              &models.StoragePlan{},
				VolumeSeriesRequestState: "NEW",
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID: "node1",
			},
		},
	}
	ag := &models.ApplicationGroup{
		ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
			Meta: &models.ObjMeta{
				ID:      "AG-1",
				Version: 1,
			},
		},
		ApplicationGroupMutable: models.ApplicationGroupMutable{
			Name: "name",
		},
	}
	cg := &models.ConsistencyGroup{
		ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
			Meta: &models.ObjMeta{
				ID:      "CG-1",
				Version: 1,
			},
		},
		ConsistencyGroupMutable: models.ConsistencyGroupMutable{
			Name:                "name",
			ApplicationGroupIds: []models.ObjIDMutable{"AG-1"},
		},
	}
	v := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID:      "VS-1",
				Version: 1,
			},
			RootParcelUUID: "uuid",
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				Messages:          []*models.TimestampedString{},
				VolumeSeriesState: "UNBOUND",
				Mounts:            []*models.Mount{},
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				Name:               "name",
				ConsistencyGroupID: "CG-1",
			},
		},
	}
	op := &renameOp{
		c: c,
		rhs: &vra.RequestHandlerState{
			A:            c.Animator,
			Request:      vr,
			VolumeSeries: v,
		},
	}

	//  ***************************** getInitialState
	op.rhs.Canceling = true
	assert.Equal(RenameDone, op.getInitialState(ctx))
	op.rhs.Canceling = false

	op.rhs.InError = true
	assert.Equal(RenameDone, op.getInitialState(ctx))
	op.rhs.InError = false

	fc.RetCGFetchErr = errors.New("db error")
	assert.Equal(RenameDone, op.getInitialState(ctx))
	assert.True(op.rhs.RetryLater)
	op.rhs.RetryLater = false

	fc.RetCGFetchErr = nil
	fc.RetCGFetchObj = cg
	fc.RetAGFetchErr = errors.New("db error")
	assert.Equal(RenameDone, op.getInitialState(ctx))
	assert.True(op.rhs.RetryLater)
	op.rhs.RetryLater = false
	fc.RetAGFetchErr = nil
	op.cg = nil

	v.Name = "renamed"
	cg.Name = "renamed"
	ag.Name = "renamed"
	vr.SystemTags = []string{com.SystemTagVsrOldName + ":name"}
	fc.RetCGFetchObj = cg
	fc.RetAGFetchObj = ag
	assert.Equal(RenameDone, op.getInitialState(ctx))
	v.Name = "name"
	cg.Name = "name"
	ag.Name = "name"
	vr.SystemTags = []string{}

	v.Name = "renamed"
	cg.Name = "renamed"
	vr.SystemTags = []string{com.SystemTagVsrOldName + ":name"}
	op.ag = nil
	op.cg = nil
	assert.Equal(RenameAG, op.getInitialState(ctx))
	assert.Nil(op.cg)
	v.Name = "name"
	cg.Name = "name"
	vr.SystemTags = []string{}

	v.Name = "renamed"
	vr.SystemTags = []string{com.SystemTagVsrOldName + ":name"}
	op.ag = nil
	op.cg = nil
	assert.Equal(RenameCG, op.getInitialState(ctx))
	assert.Equal(cg, op.cg)
	v.Name = "name"
	vr.SystemTags = []string{}

	v.ConsistencyGroupID = ""
	vr.SystemTags = []string{com.SystemTagVsrOldName + ":name"}
	op.ag = nil
	op.cg = nil
	assert.Equal(RenameVS, op.getInitialState(ctx))
	assert.Nil(op.cg)
	v.ConsistencyGroupID = "CG-1"
	vr.SystemTags = []string{}

	vr.VolumeSeriesRequestState = "UNDO_RENAMING"
	vr.SystemTags = []string{com.SystemTagVsrOldName + ":name"}
	fc.RetCGFetchErr = errors.New("db error")
	fc.RetCGFetchObj = nil
	op.ag = nil
	op.cg = nil
	assert.Equal(RenameDone, op.getInitialState(ctx))
	assert.True(op.rhs.RetryLater)
	op.rhs.RetryLater = false
	vr.SystemTags = []string{}

	vr.SystemTags = []string{com.SystemTagVsrOldName + ":name"}
	cg.Name = "renamed"
	v.Name = "renamed"
	fc.RetCGFetchErr = nil
	fc.RetCGFetchObj = cg
	op.ag = nil
	op.cg = nil
	assert.Equal(RenameRevertCG, op.getInitialState(ctx))
	v.Name = "name"
	cg.Name = "name"
	vr.SystemTags = []string{}

	vr.SystemTags = []string{com.SystemTagVsrOldName + ":name"}
	ag.Name = "renamed"
	fc.RetAGFetchErr = nil
	fc.RetAGFetchObj = ag
	op.ag = nil
	op.cg = nil
	assert.Equal(RenameRevertAG, op.getInitialState(ctx))
	ag.Name = "name"
	vr.SystemTags = []string{}

	vr.SystemTags = []string{com.SystemTagVsrOldName + ":name"}
	v.Name = "renamed"
	fc.RetCGFetchErr = nil
	fc.RetCGFetchObj = cg
	op.ag = nil
	op.cg = nil
	assert.Equal(RenameRevertVS, op.getInitialState(ctx))
	v.Name = "name"
	vr.SystemTags = []string{}

	vr.SystemTags = []string{com.SystemTagVsrOldName + ":name"}
	op.ag = nil
	op.cg = nil
	assert.Equal(RenameDone, op.getInitialState(ctx))
	vr.SystemTags = []string{}

	vr.VolumeSeriesRequestState = "RENAMING"
	fc.RetCGFetchErr = nil
	fc.RetCGFetchObj = cg
	op.ag = nil
	op.cg = nil
	assert.Equal(RenameStart, op.getInitialState(ctx))
	assert.Equal(cg, op.cg)

	//  ***************************** saveInitialState
	// error
	fc.RetVSRUpdaterErr = errors.New("db error")
	fc.RetVSRUpdaterObj = nil
	op.rhs.InError = false
	op.saveInitialState(ctx)
	assert.True(op.rhs.InError)
	op.rhs.InError = false
	// success
	vr.SystemTags = []string{}
	fc.RetVSRUpdaterErr = nil
	op.oldName = "old"
	op.saveInitialState(ctx)
	assert.False(op.rhs.InError)
	assert.EqualValues([]string{"vsr.oldName:old"}, vr.SystemTags)

	//  ***************************** renameApplicationGroup
	// success
	op.rhs.VolumeSeries = v
	op.ag = ag
	oldName := string(ag.Name)
	newName := string(ag.Name + "new")
	op.renameApplicationGroup(ctx, oldName, newName)
	assert.Equal(ctx, fc.InAGUpdaterCtx)
	assert.NoError(fc.ModAGUpdaterErr)
	assert.Equal(ag, fc.ModAGUpdaterObj)
	assert.EqualValues(newName, ag.Name)
	tl.Flush()

	// op.ag == nil, n-op
	op.ag = nil
	vr.PlanOnly = swag.Bool(true)
	vr.RequestMessages = []*models.TimestampedString{}
	op.renameApplicationGroup(ctx, oldName, newName)
	assert.Empty(vr.RequestMessages)

	// plan-only
	op.ag = ag
	fc.InAGUpdaterCtx = nil
	vr.PlanOnly = swag.Bool(true)
	vr.RequestMessages = []*models.TimestampedString{}
	op.renameApplicationGroup(ctx, oldName, newName)
	if assert.Len(vr.RequestMessages, 1) {
		assert.Regexp("Would rename ApplicationGroup", vr.RequestMessages[0].Message)
	}
	assert.Nil(fc.InAGUpdaterCtx)
	vr.PlanOnly = nil

	// already renamed
	vr.RequestMessages = []*models.TimestampedString{}
	op.renameApplicationGroup(ctx, oldName, newName)
	assert.Equal(ctx, fc.InAGUpdaterCtx)
	assert.Equal(errAlreadyRenamed, fc.ModAGUpdaterErr)
	fc.ModAGUpdaterErr = nil
	assert.False(op.rhs.InError)
	if assert.Len(vr.RequestMessages, 1) {
		assert.Regexp("Renamed ApplicationGroup", vr.RequestMessages[0].Message)
	}

	// update error
	vr.RequestMessages = []*models.TimestampedString{}
	fc.RetAGUpdaterErr = errors.New("db error")
	fc.RetAGUpdaterObj = nil
	op.renameApplicationGroup(ctx, newName, oldName)
	assert.True(op.rhs.InError)
	assert.Equal(ctx, fc.InAGUpdaterCtx)
	if assert.Len(vr.RequestMessages, 1) {
		assert.Regexp("Failed to rename ApplicationGroup", vr.RequestMessages[0].Message)
	}
	op.rhs.InError = false
	fc.RetAGUpdaterErr = nil

	//  ***************************** renameConsistencyGroup
	// success
	op.rhs.VolumeSeries = v
	op.cg = cg
	oldName = string(cg.Name)
	newName = string(cg.Name + "new")
	op.renameConsistencyGroup(ctx, oldName, newName)
	assert.Equal(ctx, fc.InCGUpdaterCtx)
	assert.NoError(fc.ModCGUpdaterErr)
	assert.Equal(cg, fc.ModCGUpdaterObj)
	assert.EqualValues(newName, cg.Name)
	tl.Flush()

	// op.cg == nil, no-op
	op.cg = nil
	vr.PlanOnly = swag.Bool(true)
	vr.RequestMessages = []*models.TimestampedString{}
	op.renameConsistencyGroup(ctx, oldName, newName)
	assert.Empty(vr.RequestMessages)

	// plan-only
	op.cg = cg
	fc.InCGUpdaterCtx = nil
	vr.PlanOnly = swag.Bool(true)
	vr.RequestMessages = []*models.TimestampedString{}
	op.renameConsistencyGroup(ctx, oldName, newName)
	assert.Nil(fc.InCGUpdaterCtx)
	if assert.Len(vr.RequestMessages, 1) {
		assert.Regexp("Would rename ConsistencyGroup", vr.RequestMessages[0].Message)
	}
	vr.PlanOnly = nil

	// already renamed
	vr.RequestMessages = []*models.TimestampedString{}
	op.renameConsistencyGroup(ctx, oldName, newName)
	assert.Equal(ctx, fc.InCGUpdaterCtx)
	assert.Equal(errAlreadyRenamed, fc.ModCGUpdaterErr)
	fc.ModCGUpdaterErr = nil
	assert.False(op.rhs.InError)
	if assert.Len(vr.RequestMessages, 1) {
		assert.Regexp("Renamed ConsistencyGroup", vr.RequestMessages[0].Message)
	}

	// update error
	vr.RequestMessages = []*models.TimestampedString{}
	fc.RetCGUpdaterErr = errors.New("db error")
	fc.RetCGUpdaterObj = nil
	op.renameConsistencyGroup(ctx, newName, oldName)
	assert.Equal(ctx, fc.InCGUpdaterCtx)
	assert.True(op.rhs.InError)
	if assert.Len(vr.RequestMessages, 1) {
		assert.Regexp("Failed to rename ConsistencyGroup", vr.RequestMessages[0].Message)
	}
	op.rhs.InError = false
	fc.RetCGUpdaterErr = nil

	//  ***************************** renameVolumeSeries
	// success
	op.rhs.VolumeSeries = v
	op.cg = cg
	oldName = string(v.Name)
	newName = string(v.Name + "new")
	op.renameVolumeSeries(ctx, oldName, newName)
	assert.Equal(ctx, fc.InVSUpdaterCtx)
	assert.NoError(fc.ModVSUpdaterErr)
	assert.Equal(v, fc.ModVSUpdaterObj)
	assert.EqualValues(newName, v.Name)

	// plan-only
	fc.InVSUpdaterCtx = nil
	vr.PlanOnly = swag.Bool(true)
	vr.RequestMessages = []*models.TimestampedString{}
	op.renameVolumeSeries(ctx, oldName, newName)
	assert.Nil(fc.InVSUpdaterCtx)
	if assert.Len(vr.RequestMessages, 1) {
		assert.Regexp("Would rename VolumeSeries", vr.RequestMessages[0].Message)
	}
	vr.PlanOnly = nil

	// already renamed
	vr.RequestMessages = []*models.TimestampedString{}
	op.renameVolumeSeries(ctx, oldName, newName)
	assert.Equal(ctx, fc.InVSUpdaterCtx)
	assert.Equal(errAlreadyRenamed, fc.ModVSUpdaterErr)
	fc.ModVSUpdaterErr = nil
	assert.False(op.rhs.InError)
	if assert.Len(vr.RequestMessages, 1) {
		assert.Regexp("Renamed VolumeSeries", vr.RequestMessages[0].Message)
	}

	// update error
	vr.RequestMessages = []*models.TimestampedString{}
	fc.RetVSUpdaterErr = errors.New("db error")
	fc.RetVSUpdaterObj = nil
	op.renameVolumeSeries(ctx, newName, oldName)
	assert.Equal(ctx, fc.InVSUpdaterCtx)
	assert.True(op.rhs.InError)
	if assert.Len(vr.RequestMessages, 1) {
		assert.Regexp("Failed to rename VolumeSeries", vr.RequestMessages[0].Message)
	}
	op.rhs.InError = false
	fc.RetCGUpdaterErr = nil
}

func TestRename(t *testing.T) {
	assert := assert.New(t)

	op := &fakeRenameOps{}
	op.cg = &models.ConsistencyGroup{}
	op.ops = op
	op.rhs = &vra.RequestHandlerState{}
	op.retGIS = RenameStart
	expCalled := []string{"GIS", "SIS", "RVS", "RCG", "RAG"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakeRenameOps{}
	op.ops = op
	op.rhs = &vra.RequestHandlerState{}
	op.retGIS = RenameVS
	expCalled = []string{"GIS", "RVS", "RCG", "RAG"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakeRenameOps{}
	op.ops = op
	op.rhs = &vra.RequestHandlerState{}
	op.retGIS = RenameCG
	expCalled = []string{"GIS", "RCG", "RAG"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakeRenameOps{}
	op.ops = op
	op.rhs = &vra.RequestHandlerState{}
	op.retGIS = RenameAG
	expCalled = []string{"GIS", "RAG"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakeRenameOps{}
	op.ops = op
	op.rhs = &vra.RequestHandlerState{}
	op.retGIS = RenameDone
	expCalled = []string{"GIS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakeRenameOps{}
	op.ops = op
	op.rhs = &vra.RequestHandlerState{}
	op.retGIS = RenameRevertAG
	expCalled = []string{"GIS", "RAG", "RCG", "RVS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakeRenameOps{}
	op.ops = op
	op.rhs = &vra.RequestHandlerState{}
	op.retGIS = RenameRevertCG
	expCalled = []string{"GIS", "RCG", "RVS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakeRenameOps{}
	op.ops = op
	op.rhs = &vra.RequestHandlerState{}
	op.retGIS = RenameRevertVS
	expCalled = []string{"GIS", "RVS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// call the real handler with an error
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	fts := &fhk.TaskScheduler{}
	app.TaskScheduler = fts

	cg := &models.ConsistencyGroup{
		ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
			Meta: &models.ObjMeta{
				ID:      "CG-1",
				Version: 1,
			},
		},
		ConsistencyGroupMutable: models.ConsistencyGroupMutable{
			Name: "name",
		},
	}
	v := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: "VS-1",
			},
			RootParcelUUID: "uuid",
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				ConsistencyGroupID: "CG-1",
			},
		},
	}
	vr := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VR-1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{},
		},
	}
	vr.VolumeSeriesID = models.ObjIDMutable(v.Meta.ID)
	c := &Component{}
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc
	rhs := &vra.RequestHandlerState{
		A:            c.Animator,
		InError:      true,
		Request:      vr,
		VolumeSeries: v,
	}
	c.Rename(nil, rhs)
	assert.Zero(fc.InCGFetchID)

	// call real handler in undo, cover tag-not-present case
	rhs.VolumeSeries = v
	vr.VolumeSeriesRequestState = "UNDO_RENAMING"
	vr.SystemTags = []string{}
	fc.RetCGFetchObj = cg
	c.UndoRename(nil, rhs)
	assert.Zero(fc.InCGFetchID)
	assert.True(rhs.InError)
}

type fakeRenameOps struct {
	renameOp
	called []string
	retGIS renameSubState
}

func (op *fakeRenameOps) getInitialState(ctx context.Context) renameSubState {
	op.called = append(op.called, "GIS")
	return op.retGIS
}

func (op *fakeRenameOps) saveInitialState(ctx context.Context) {
	op.called = append(op.called, "SIS")
}

func (op *fakeRenameOps) renameApplicationGroup(ctx context.Context, old, new string) {
	op.called = append(op.called, "RAG")
}

func (op *fakeRenameOps) renameConsistencyGroup(ctx context.Context, old, new string) {
	op.called = append(op.called, "RCG")
}

func (op *fakeRenameOps) renameVolumeSeries(ctx context.Context, old, new string) {
	op.called = append(op.called, "RVS")
}
