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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/application_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/consistency_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/snapshot"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	fhk "github.com/Nuvoloso/kontroller/pkg/housekeeping/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	fvra "github.com/Nuvoloso/kontroller/pkg/vra/fake"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestCreateSteps(t *testing.T) {
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
			RequestedOperations: []string{"CREATE"},
			VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{
				VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
					AccountID: "account1",
				},
				VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
					Name:               "name",
					Description:        "description",
					SizeBytes:          swag.Int64(1073741824),
					ConsistencyGroupID: "",
					ServicePlanID:      "plan1",
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
		ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
			AccountID: "account1",
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
		ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
			AccountID: "account1",
		},
		ConsistencyGroupMutable: models.ConsistencyGroupMutable{
			Name: "name",
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
		VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
			AccountID: "account1",
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				Messages:          []*models.TimestampedString{},
				VolumeSeriesState: "UNBOUND",
				Mounts:            []*models.Mount{},
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				Name:               "name",
				Description:        "description",
				SizeBytes:          swag.Int64(1073741824),
				ConsistencyGroupID: "",
				ServicePlanID:      "plan1",
			},
		},
	}
	op := &createOp{
		c: c,
		rhs: &vra.RequestHandlerState{
			A:         c.Animator,
			Request:   vr,
			HasCreate: true,
		},
	}
	snapList := &snapshot.SnapshotListOK{
		Payload: []*models.Snapshot{
			&models.Snapshot{
				SnapshotAllOf0: models.SnapshotAllOf0{
					Meta: &models.ObjMeta{
						ID:      "id1",
						Version: 1,
					},
					ConsistencyGroupID: models.ObjID(v.ConsistencyGroupID),
					PitIdentifier:      "pitid-1",
					SizeBytes:          swag.Int64Value(v.SizeBytes),
					SnapIdentifier:     "snap-1",
					SnapTime:           strfmt.DateTime(now),
					VolumeSeriesID:     models.ObjIDMutable(v.Meta.ID),
				},
				SnapshotMutable: models.SnapshotMutable{
					DeleteAfterTime: strfmt.DateTime(now.Add(90 * 24 * time.Hour)),
					Locations: map[string]models.SnapshotLocation{
						"ps1": {CreationTime: strfmt.DateTime(now), CspDomainID: "ps1"},
					},
				},
			},
			&models.Snapshot{
				SnapshotAllOf0: models.SnapshotAllOf0{
					Meta: &models.ObjMeta{
						ID:      "id2",
						Version: 1,
					},
					ConsistencyGroupID: models.ObjID(v.ConsistencyGroupID),
					PitIdentifier:      "pitid-2",
					SizeBytes:          swag.Int64Value(v.SizeBytes),
					SnapIdentifier:     "snap-2",
					SnapTime:           strfmt.DateTime(now),
					VolumeSeriesID:     models.ObjIDMutable(v.Meta.ID),
				},
				SnapshotMutable: models.SnapshotMutable{
					DeleteAfterTime: strfmt.DateTime(now.Add(90 * 24 * time.Hour)),
					Locations: map[string]models.SnapshotLocation{
						"ps2": {CreationTime: strfmt.DateTime(now), CspDomainID: "ps2"},
					},
				},
			},
		},
	}

	//  ***************************** getInitialState
	op.rhs.Canceling = true
	assert.Equal(CreateDone, op.getInitialState(ctx))
	op.rhs.Canceling = false

	op.rhs.InError = true
	assert.Equal(CreateDone, op.getInitialState(ctx))
	op.rhs.InError = false

	// everything exists, set ID in VSR
	fc.RetAGListObj = &application_group.ApplicationGroupListOK{Payload: []*models.ApplicationGroup{ag}}
	fc.RetCGListObj = &consistency_group.ConsistencyGroupListOK{Payload: []*models.ConsistencyGroup{cg}}
	fc.RetLsVOk = &volume_series.VolumeSeriesListOK{Payload: []*models.VolumeSeries{v}}
	assert.Equal(CreateSetID, op.getInitialState(ctx))
	assert.Equal(cg, op.cg)
	if assert.Len(op.ags, 1) {
		assert.Equal(ag, op.ags[0])
	}
	assert.Equal(v, op.rhs.VolumeSeries)
	op.cg = nil // reset

	// AG and CG exist, create VS
	op.rhs.VolumeSeries = nil
	fc.RetAGListObj = &application_group.ApplicationGroupListOK{Payload: []*models.ApplicationGroup{ag}}
	fc.RetCGListObj = &consistency_group.ConsistencyGroupListOK{Payload: []*models.ConsistencyGroup{cg}}
	fc.RetLsVOk = &volume_series.VolumeSeriesListOK{Payload: []*models.VolumeSeries{}}
	assert.Equal(CreateVS, op.getInitialState(ctx))
	assert.Equal(cg, op.cg)
	op.cg = nil // reset

	// all steps complete, complete VSR
	fc.RetCGFetchObj = cg
	fc.RetAGFetchObj = ag
	fc.RetAGListObj = nil
	fc.RetAGListObj = nil
	cg.ApplicationGroupIds = []models.ObjIDMutable{"AG-1"}
	vr.VolumeSeriesCreateSpec.ConsistencyGroupID = "CG-2"
	op.rhs.Request.VolumeSeriesID = models.ObjIDMutable(v.Meta.ID)
	op.rhs.VolumeSeries = v
	assert.Equal(CreateDone, op.getInitialState(ctx))
	op.cg = nil                                       // reset
	vr.VolumeSeriesCreateSpec.ConsistencyGroupID = "" // reset
	op.rhs.VolumeSeries = nil                         // reset
	op.rhs.Request.VolumeSeriesID = ""                // reset

	fc.RetCGFetchErr = errors.New("db error")
	vr.VolumeSeriesCreateSpec.ConsistencyGroupID = "CG-2"
	op.rhs.Request.VolumeSeriesID = models.ObjIDMutable(v.Meta.ID)
	assert.Equal(CreateAG, op.getInitialState(ctx))
	assert.True(op.rhs.RetryLater)
	op.rhs.RetryLater = false          // reset
	op.rhs.Request.VolumeSeriesID = "" // reset

	fc.RetCGFetchErr = nil
	fc.RetCGFetchObj = cg
	fc.RetAGFetchErr = errors.New("db error")
	vr.VolumeSeriesCreateSpec.ConsistencyGroupID = "CG-2"
	op.rhs.Request.VolumeSeriesID = models.ObjIDMutable(v.Meta.ID)
	assert.Equal(CreateAG, op.getInitialState(ctx))
	assert.True(op.rhs.RetryLater)
	cg.ApplicationGroupIds = nil       // reset
	op.rhs.RetryLater = false          // reset
	op.rhs.Request.VolumeSeriesID = "" // reset

	fc.RetAGFetchErr = nil
	fc.RetAGFetchObj = ag
	fc.RetLsVErr = errors.New("db error")
	assert.Equal(CreateAG, op.getInitialState(ctx))
	assert.True(op.rhs.RetryLater)
	assert.Equal(cg, op.cg)
	op.rhs.RetryLater = false                         // reset
	op.cg = nil                                       // reset
	op.ags = []*models.ApplicationGroup{}             // reset
	vr.VolumeSeriesCreateSpec.ConsistencyGroupID = "" // reset

	fc.RetCGListErr = errors.New("db error")
	fc.RetLsVErr = nil
	assert.Equal(CreateAG, op.getInitialState(ctx))
	assert.Nil(op.cg)
	assert.True(op.rhs.RetryLater)
	op.rhs.RetryLater = false // reset

	fc.RetCGListErr = nil
	fc.RetCGListObj = &consistency_group.ConsistencyGroupListOK{Payload: []*models.ConsistencyGroup{}}
	fc.RetAGListErr = errors.New("db error")
	fc.RetLsVErr = nil
	assert.Equal(CreateAG, op.getInitialState(ctx))
	assert.Nil(op.cg)
	assert.True(op.rhs.RetryLater)
	op.rhs.RetryLater = false // reset

	fc.RetAGListErr = nil
	fc.RetAGListObj = &application_group.ApplicationGroupListOK{Payload: []*models.ApplicationGroup{}}
	fc.RetCGListObj = &consistency_group.ConsistencyGroupListOK{Payload: []*models.ConsistencyGroup{}}
	fc.RetLsVOk = &volume_series.VolumeSeriesListOK{Payload: []*models.VolumeSeries{}}
	assert.Equal(CreateAG, op.getInitialState(ctx))
	assert.Nil(op.cg)
	assert.Empty(op.ags)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	fc.RetAGListObj = &application_group.ApplicationGroupListOK{Payload: []*models.ApplicationGroup{ag}}
	assert.Equal(CreateCG, op.getInitialState(ctx))
	assert.Len(op.ags, 1)
	assert.Nil(op.cg)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	op.ags = nil

	// AGs specified in VSR
	vr.ApplicationGroupIds = []models.ObjIDMutable{"vr-agID"}
	fc.RetAGListObj = &application_group.ApplicationGroupListOK{Payload: []*models.ApplicationGroup{}}
	fc.RetAGListErr = centrald.ErrorDbError
	assert.Equal(CreateCG, op.getInitialState(ctx))
	assert.Equal("vr-agID", fc.InAGFetchID)
	assert.Len(op.ags, 1)
	assert.Nil(op.cg)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	vr.ApplicationGroupIds = nil // reset
	op.ags = nil                 // reset

	op.rhs.InError = false // reset
	op.rhs.Request.PlanOnly = swag.Bool(true)
	assert.Equal(CreateDone, op.getInitialState(ctx))
	assert.True(op.rhs.InError)

	op.rhs.InError = false        // reset
	op.rhs.Request.PlanOnly = nil // reset
	op.rhs.Request.VolumeSeriesRequestState = "UNDO_CREATING"
	fc.RetAGListObj = &application_group.ApplicationGroupListOK{Payload: []*models.ApplicationGroup{}}
	fc.RetAGListErr = nil
	fc.RetCGListObj = &consistency_group.ConsistencyGroupListOK{Payload: []*models.ConsistencyGroup{}}
	fc.RetCGListErr = nil
	assert.Equal(CreateDone, op.getInitialState(ctx))
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Nil(op.cg)

	fc.RetCGListErr = nil
	fc.RetCGListObj = &consistency_group.ConsistencyGroupListOK{Payload: []*models.ConsistencyGroup{cg}}
	cg.SystemTags = []string{com.SystemTagVsrCreator + ":VR-1"}
	assert.Equal(CreateDeleteCGAndAG, op.getInitialState(ctx))
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(cg, op.cg)
	assert.Empty(op.ags)

	fc.RetCGListErr = errors.New("db error")
	assert.Equal(CreateDone, op.getInitialState(ctx))
	assert.True(op.rhs.RetryLater)
	op.rhs.RetryLater = false // reset

	// Delete VS, start by setting DELETING
	fc.RetAGListObj = &application_group.ApplicationGroupListOK{Payload: []*models.ApplicationGroup{}}
	fc.RetAGListErr = nil
	fc.RetAGFetchObj = nil
	fc.RetCGListObj = &consistency_group.ConsistencyGroupListOK{Payload: []*models.ConsistencyGroup{}}
	fc.RetCGListErr = nil
	fc.RetCGFetchObj = nil
	op.rhs.Request.VolumeSeriesID = models.ObjIDMutable(v.Meta.ID)
	op.rhs.VolumeSeries = v
	op.cg = nil
	assert.Equal(CreateSetDeleting, op.getInitialState(ctx))

	// Delete VS, already in DELETING state
	fc.RetAGListObj = &application_group.ApplicationGroupListOK{Payload: []*models.ApplicationGroup{}}
	fc.RetAGListErr = nil
	fc.RetAGFetchObj = nil
	fc.RetCGListObj = &consistency_group.ConsistencyGroupListOK{Payload: []*models.ConsistencyGroup{}}
	fc.RetCGListErr = nil
	fc.RetCGFetchObj = nil
	op.rhs.Request.VolumeSeriesID = models.ObjIDMutable(v.Meta.ID)
	op.rhs.VolumeSeries = v
	v.VolumeSeriesState = "DELETING"
	op.cg = nil
	assert.Equal(CreateDeleteSnapshots, op.getInitialState(ctx))

	// cg set in vs, not in DELETING, start by setting state
	fc.RetCGFetchObj = cg
	cg.SystemTags = []string{com.SystemTagVsrCreator + ":VR-1"}
	v.ConsistencyGroupID = "CG-1"
	v.VolumeSeriesState = "UNBOUND"
	assert.Equal(CreateSetDeleting, op.getInitialState(ctx))
	assert.NotNil(op.cg)
	assert.Empty(op.ags)
	op.cg = nil                // reset
	v.ConsistencyGroupID = ""  // reset
	cg.SystemTags = []string{} // reset

	// cg set in vs, start by resetting IDs
	fc.RetCGFetchObj = cg
	cg.SystemTags = []string{com.SystemTagVsrCreator + ":VR-1"}
	v.ConsistencyGroupID = "CG-1"
	v.VolumeSeriesState = "DELETING"
	assert.Equal(CreateDeleteSnapshots, op.getInitialState(ctx))
	assert.NotNil(op.cg)
	assert.Empty(op.ags)
	op.cg = nil                // reset
	v.ConsistencyGroupID = ""  // reset
	cg.SystemTags = []string{} // reset

	// delete path, removed-tag detected, attempt cg delete, delete vs
	fc.InCGFetchID = ""
	op.rhs.HasCreate = false
	op.rhs.HasDelete = true
	v.ConsistencyGroupID = ""
	v.SystemTags = []string{com.SystemTagVsrCGReset + ":VR-1,cg-1"}
	op.cg = nil
	op.ags = []*models.ApplicationGroup{}
	assert.Equal(CreateDeleteCG, op.getInitialState(ctx))
	assert.Equal("cg-1", fc.InCGFetchID)
	assert.False(op.rhs.RetryLater)
	assert.Equal(cg, op.cg)

	// delete path, removed-tag detected, cg deleted by something else, delete vs
	fc.InCGFetchID = ""
	fc.RetCGFetchObj = nil
	fc.RetCGFetchErr = fake.NotFoundErr
	op.rhs.HasDelete = true
	v.ConsistencyGroupID = ""
	v.SystemTags = []string{com.SystemTagVsrCGReset + ":VR-1,cg-1"}
	op.cg = nil
	assert.Equal(CreateDeleteSnapshots, op.getInitialState(ctx))
	assert.Equal("cg-1", fc.InCGFetchID)
	assert.False(op.rhs.RetryLater)
	assert.Nil(op.cg)

	// delete path, removed-tag detected, attempt ag delete, delete vs
	fc.InAGFetchID = ""
	fc.RetAGFetchObj = ag
	op.rhs.HasCreate = false
	op.rhs.HasDelete = true
	v.SystemTags = []string{com.SystemTagVsrAGReset + ":VR-1,ag-1"}
	op.cg = nil
	op.ags = []*models.ApplicationGroup{}
	assert.Equal(CreateDeleteAGs, op.getInitialState(ctx))
	assert.Equal("ag-1", fc.InAGFetchID)
	assert.False(op.rhs.RetryLater)
	assert.Equal(ag, op.ags[0])

	// delete path, removed-tag detected, ag deleted by something else, delete vs
	fc.InAGFetchID = ""
	fc.RetAGFetchObj = nil
	fc.RetAGFetchErr = fake.NotFoundErr
	op.rhs.HasDelete = true
	v.SystemTags = []string{com.SystemTagVsrAGReset + ":VR-1,ag-1"}
	op.cg = nil
	op.ags = []*models.ApplicationGroup{}
	assert.Equal(CreateDeleteSnapshots, op.getInitialState(ctx))
	assert.Equal("ag-1", fc.InAGFetchID)
	assert.False(op.rhs.RetryLater)
	assert.Empty(op.ags)

	// delete path, remove vs from cg, attempt cg delete, delete vs
	fc.RetCGFetchObj = cg
	fc.RetCGFetchErr = nil
	op.rhs.HasDelete = true
	v.ConsistencyGroupID = "CG-1"
	v.SystemTags = []string{}
	assert.Equal(CreateDeleteSnapshots, op.getInitialState(ctx))
	v.ConsistencyGroupID = ""                       // reset
	v.VolumeSeriesState = "UNBOUND"                 //reset
	op.rhs.HasCreate = true                         // reset
	op.rhs.HasDelete = false                        // reset
	op.rhs.Request.VolumeSeriesID = ""              // reset
	op.rhs.VolumeSeries = nil                       // reset
	op.rhs.Request.VolumeSeriesRequestState = "NEW" // reset

	//  ***************************** createConsistencyGroup
	// failure
	op.rhs.RetryLater = false                          // reset
	op.rhs.Canceling = false                           // reset
	op.rhs.InError = false                             // reset
	op.cg = nil                                        // reset
	vr.RequestMessages = []*models.TimestampedString{} // reset
	fc.RetCGCreateObj = nil
	fc.RetCGCreateErr = errors.New("create ConsistencyGroup error")
	op.createConsistencyGroup(ctx)
	assert.False(op.rhs.Canceling)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Empty(op.rhs.VolumeSeries)
	assert.Regexp("Failed to create ConsistencyGroup object", vr.RequestMessages[0].Message)
	assert.Equal(ctx, fc.InCGCreateCtx)

	// success
	op.ags = []*models.ApplicationGroup{ag}
	op.rhs.InError = false                             // reset
	vr.RequestMessages = []*models.TimestampedString{} // reset
	fc.RetCGCreateObj = cg
	fc.RetCGCreateErr = nil
	op.createConsistencyGroup(ctx)
	assert.False(op.rhs.Canceling)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(cg, op.cg)
	assert.EqualValues("name", fc.InCGCreateObj.Name)
	assert.EqualValues("account1", fc.InCGCreateObj.AccountID)
	assert.Len(fc.InCGCreateObj.ApplicationGroupIds, 1)
	assert.Equal(ctx, fc.InCGCreateCtx)
	assert.Contains(fc.InCGCreateObj.SystemTags, op.creatorTag)

	//  ***************************** createApplicationGroup
	// failure
	op.rhs.RetryLater = false                          // reset
	op.rhs.Canceling = false                           // reset
	op.rhs.InError = false                             // reset
	vr.RequestMessages = []*models.TimestampedString{} // reset
	fc.RetAGCreateObj = nil
	fc.RetAGCreateErr = errors.New("create ApplicationGroup error")
	op.createApplicationGroup(ctx)
	assert.False(op.rhs.Canceling)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Empty(op.rhs.VolumeSeries)
	assert.Regexp("Failed to create ApplicationGroup object", vr.RequestMessages[0].Message)
	assert.Equal(ctx, fc.InAGCreateCtx)

	// success
	op.ags = nil
	op.rhs.InError = false                             // reset
	vr.RequestMessages = []*models.TimestampedString{} // reset
	fc.RetAGCreateObj = ag
	fc.RetAGCreateErr = nil
	op.createApplicationGroup(ctx)
	assert.False(op.rhs.Canceling)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	if assert.Len(op.ags, 1) {
		assert.Equal(ag, op.ags[0])
	}
	assert.Equal(ctx, fc.InAGCreateCtx)
	assert.EqualValues("name", fc.InAGCreateObj.Name)
	assert.EqualValues("account1", fc.InAGCreateObj.AccountID)
	assert.Contains(fc.InAGCreateObj.SystemTags, op.creatorTag)

	//  ***************************** createVolumeSeries
	// failure
	op.rhs.RetryLater = false                          // reset
	op.rhs.Canceling = false                           // reset
	op.rhs.InError = false                             // reset
	vr.RequestMessages = []*models.TimestampedString{} // reset
	assert.Empty(op.rhs.VolumeSeries)
	fc.RetCVObj = nil
	fc.RetCVErr = errors.New("create VolumeSeries error")
	op.createVolumeSeries(ctx)
	assert.False(op.rhs.Canceling)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Empty(op.rhs.VolumeSeries)
	assert.Regexp("Failed to create VolumeSeries object", vr.RequestMessages[0].Message)
	assert.Equal(ctx, fc.InCVCtx)

	// success
	op.rhs.InError = false                             // reset
	vr.RequestMessages = []*models.TimestampedString{} // reset
	fc.RetCVObj = v
	fc.RetCVErr = nil
	op.createVolumeSeries(ctx)
	assert.False(op.rhs.Canceling)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(v, op.rhs.VolumeSeries)
	assert.Contains(fc.InCVObj.SystemTags, op.creatorTag)
	assert.Equal(ctx, fc.InCVCtx)
	assert.EqualValues(cg.Meta.ID, fc.InCVObj.ConsistencyGroupID)

	// ***************************** setID
	assert.Empty(op.rhs.Request.VolumeSeriesID)
	assert.NotNil(op.rhs.VolumeSeries)
	// failure case
	fc.RetVSRUpdaterObj = nil
	fc.RetVSRUpdaterErr = errors.New("update storage request error")
	op.setID(ctx)
	assert.True(op.rhs.InError)
	assert.Equal(vr, op.rhs.Request)
	assert.Regexp("Created volume series", vr.RequestMessages[0].Message)
	assert.Regexp("Failed to update VolumeSeriesRequest.*request error", vr.RequestMessages[1].Message)
	op.rhs.Request.VolumeSeriesID = ""                 // reset
	vr.RequestMessages = []*models.TimestampedString{} // reset

	// inject error
	c.rei.SetProperty("create-fail-in-setID", &rei.Property{BoolValue: true})
	op.setID(ctx)
	assert.True(op.rhs.InError)
	assert.Equal(vr, op.rhs.Request)
	assert.Regexp("Created volume series", vr.RequestMessages[0].Message)
	assert.Regexp("Failed to update VolumeSeriesRequest.*create-fail-in-setID", vr.RequestMessages[1].Message)
	assert.EqualValues(op.rhs.VolumeSeries.Meta.ID, vr.VolumeSeriesID)
	vr.RequestMessages = []*models.TimestampedString{} // reset

	// success
	op.rhs.InError = false // reset
	vr.VolumeSeriesID = "" // reset
	newVr := &models.VolumeSeriesRequest{}
	*newVr = *vr // shallow copy
	newVr.Meta = &models.ObjMeta{}
	*newVr.Meta = *vr.Meta // shallow copy
	newVr.Meta.Version++
	fc.InUVRItems = nil
	assert.NotEqual(newVr.Meta.Version, vr.Meta.Version)
	fc.RetVSRUpdaterObj = newVr
	fc.RetVSRUpdaterErr = nil
	op.setID(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(newVr, op.rhs.Request) // updated obj in po
	assert.Equal(newVr.Meta.Version, op.rhs.Request.Meta.Version)
	assert.NotNil(fc.InVSRUpdaterItems)
	op.rhs.Request = vr // put the original obj back
	assert.EqualValues(op.rhs.VolumeSeries.Meta.ID, vr.VolumeSeriesID)

	//  ***************************** setDeleting (fully tested in vra package)
	fvu := &fvra.VolumeUpdater{}
	op.rhs.VSUpdater = fvu
	v.VolumeSeriesState = "UNBOUND"
	op.setDeleting(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(ctx, fvu.InSVSctx)
	assert.Equal("DELETING", fvu.InSVSState)

	//  ***************************** removeVSFromCG
	// no errors
	cg.ApplicationGroupIds = []models.ObjIDMutable{"ag-1", "ag-2"}
	op.rhs.VolumeSeries = v
	v.ConsistencyGroupID = "con"
	fc.InUVItems = nil
	fc.InUVObj = nil
	fc.PassThroughUVObj = true
	op.removeVSFromCG(ctx)
	assert.EqualValues([]string{"consistencyGroupId"}, fc.InUVItems.Set)
	assert.EqualValues([]string{"systemTags"}, fc.InUVItems.Append)
	assert.False(op.rhs.InError)
	assert.Equal(v, op.rhs.VolumeSeries)
	assert.Empty(v.ConsistencyGroupID)
	assert.Len(v.SystemTags, 3)
	assert.Regexp(",con$", v.SystemTags[0])
	assert.Regexp(",ag-1$", v.SystemTags[1])
	assert.Regexp(",ag-2$", v.SystemTags[2])

	// db error
	fc.RetUVErr = errors.New("db error")
	fc.RetUVObj = nil
	fc.PassThroughUVObj = false
	fc.InUVItems = nil
	op.removeVSFromCG(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.EqualValues([]string{"consistencyGroupId"}, fc.InUVItems.Set)
	assert.EqualValues([]string{"systemTags"}, fc.InUVItems.Append)
	op.rhs.RetryLater = false

	//  ***************************** deleteVS
	// success
	op.rhs.VolumeSeries = v
	fc.RetDVErr = nil
	op.deleteVS(ctx)
	assert.Nil(op.rhs.VolumeSeries)
	assert.EqualValues(v.Meta.ID, op.rhs.Request.VolumeSeriesID)
	assert.EqualValues(ctx, fc.InDVCtx)
	assert.EqualValues(v.Meta.ID, fc.InDVObj)

	// fail to delete volume series (non-delete case)
	op.rhs.VolumeSeries = v
	vr.VolumeSeriesID = models.ObjIDMutable(v.Meta.ID)
	fc.RetDVErr = errors.New("delete volume series error")
	op.rhs.InError = false
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.deleteVS(ctx)
	assert.False(op.rhs.InError)
	assert.Regexp("Failed to delete VolumeSeries.*delete volume series error", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal(v, op.rhs.VolumeSeries)
	assert.NotEmpty(vr.VolumeSeriesID)
	assert.EqualValues(ctx, fc.InDVCtx)

	// fail to delete volume series (delete case)
	op.rhs.VolumeSeries = v
	vr.VolumeSeriesID = models.ObjIDMutable(v.Meta.ID)
	fc.RetDVErr = errors.New("delete volume series error")
	op.rhs.InError = false
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.rhs.HasDelete = true
	op.deleteVS(ctx)
	assert.True(op.rhs.InError)
	assert.Regexp("Failed to delete VolumeSeries.*delete volume series error", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal(v, op.rhs.VolumeSeries)
	assert.NotEmpty(vr.VolumeSeriesID)
	assert.EqualValues(ctx, fc.InDVCtx)
	op.rhs.HasDelete = false

	//  ***************************** deleteAGs
	// success, auto-created
	op.rhs.VolumeSeries = v
	op.ags = []*models.ApplicationGroup{ag}
	ag.SystemTags = []string{op.creatorTag}
	fc.RetAGDeleteErr = nil
	fc.InAGDeleteID = ""
	op.deleteAGs(ctx)
	assert.Equal(ctx, fc.InAGDeleteCtx)
	assert.EqualValues(ag.Meta.ID, fc.InAGDeleteID)
	tl.Flush()

	// success, created externally, has mustDelete tag
	op.rhs.VolumeSeries = v
	op.ags = []*models.ApplicationGroup{ag}
	ag.SystemTags = []string{com.SystemTagMustDeleteOnUndoCreate}
	fc.RetAGDeleteErr = nil
	fc.InAGDeleteID = ""
	op.deleteAGs(ctx)
	assert.Equal(ctx, fc.InAGDeleteCtx)
	assert.EqualValues(ag.Meta.ID, fc.InAGDeleteID)
	tl.Flush()

	// not auto created, skip
	ag.SystemTags = []string{}
	fc.InAGDeleteID = ""
	op.deleteAGs(ctx)
	assert.Zero(fc.InAGDeleteID)

	// application group not empty, cover delete path
	op.rhs.HasCreate = false
	op.rhs.HasDelete = true
	fc.RetAGDeleteErr = &crud.Error{Payload: models.Error{Code: 409}}
	op.deleteAGs(ctx)
	assert.Equal(1, tl.CountPattern("Ignoring failure to delete"))
	tl.Flush()
	// application group already deleted
	fc.RetAGDeleteErr = fake.NotFoundErr
	op.deleteAGs(ctx)
	assert.Equal(1, tl.CountPattern("Ignoring failure to delete"))
	// fail to delete application group
	fc.RetAGDeleteErr = errors.New("db error")
	op.deleteAGs(ctx)
	assert.Equal(1, tl.CountPattern("Failed to delete Application"))
	op.rhs.HasCreate = true
	op.rhs.HasDelete = false

	//  ***************************** deleteCG
	// success, auto-created
	op.rhs.VolumeSeries = v
	op.cg = cg
	cg.SystemTags = []string{op.creatorTag}
	fc.RetCGDeleteErr = nil
	fc.InCGDeleteID = ""
	op.deleteCG(ctx)
	assert.Equal(ctx, fc.InCGDeleteCtx)
	assert.EqualValues(cg.Meta.ID, fc.InCGDeleteID)
	tl.Flush()

	// success, created externally, has mustDelete tag
	op.rhs.VolumeSeries = v
	op.cg = cg
	cg.SystemTags = []string{com.SystemTagMustDeleteOnUndoCreate}
	fc.RetCGDeleteErr = nil
	fc.InCGDeleteID = ""
	op.deleteCG(ctx)
	assert.Equal(ctx, fc.InCGDeleteCtx)
	assert.EqualValues(cg.Meta.ID, fc.InCGDeleteID)
	tl.Flush()

	// not auto created, skip
	cg.SystemTags = []string{}
	fc.InCGDeleteID = ""
	op.deleteCG(ctx)
	assert.Zero(fc.InCGDeleteID)

	// consistency group not empty, cover delete path
	op.rhs.HasCreate = false
	op.rhs.HasDelete = true
	fc.RetCGDeleteErr = &crud.Error{Payload: models.Error{Code: 409}}
	op.deleteCG(ctx)
	assert.Equal(1, tl.CountPattern("Ignoring failure to delete"))
	tl.Flush()
	// consistency group already deleted
	fc.RetCGDeleteErr = fake.NotFoundErr
	op.deleteCG(ctx)
	assert.Equal(1, tl.CountPattern("Ignoring failure to delete"))
	// fail to delete consistency group
	fc.RetCGDeleteErr = errors.New("db error")
	op.deleteCG(ctx)
	assert.Equal(1, tl.CountPattern("Failed to delete ConsistencyGroup"))

	//  ***************************** deleteVSSnapshots
	// success
	fc.RetLsSnapshotOk = snapList
	fc.RetLsSnapshotErr = nil
	fc.RetDSnapshotErr = nil
	op.deleteVSSnapshots(ctx)
	assert.EqualValues(snapList.Payload[1].Meta.ID, fc.InDSnapshotObj)

	// fail to list volume series snapshots
	fc.RetLsSnapshotOk = nil
	fc.RetLsSnapshotErr = errors.New("list volume series snapshots error")
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.deleteVSSnapshots(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)

	// fail to delete volume series snapshots
	fc.RetLsSnapshotOk = snapList
	fc.RetLsSnapshotErr = nil
	fc.RetDSnapshotErr = errors.New("delete snapshots for volume series error")
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.deleteVSSnapshots(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	op.rhs.HasDelete = false
}

func TestCreate(t *testing.T) {
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
	sfc := &fakeSFC{}
	app.SFC = sfc
	fts := &fhk.TaskScheduler{}
	app.TaskScheduler = fts
	c := &Component{}
	c.Init(app)
	assert.NotNil(c.Log)

	vsr := &models.VolumeSeriesRequest{}
	vsr.Meta = &models.ObjMeta{ID: "vsr-id"}

	newFakeCreateOp := func() *fakeCreateOps {
		op := &fakeCreateOps{}
		op.c = c
		op.ops = op
		op.rhs = &vra.RequestHandlerState{Request: vsrClone(vsr)}
		return op
	}

	op := newFakeCreateOp()
	op.retGIS = CreateAG
	expCalled := []string{"GIS", "CAG", "CCG", "CVS", "SID"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeCreateOp()
	op.retGIS = CreateCG
	expCalled = []string{"GIS", "CCG", "CVS", "SID"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeCreateOp()
	op.retGIS = CreateVS
	expCalled = []string{"GIS", "CVS", "SID"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeCreateOp()
	op.retGIS = CreateSetID
	expCalled = []string{"GIS", "SID"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeCreateOp()
	op.retGIS = CreateDone
	expCalled = []string{"GIS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// only VS remains, DELETING already set
	op = newFakeCreateOp()
	op.retGIS = CreateDeleteVS
	expCalled = []string{"GIS", "DVS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// only VS remains, DELETING not set
	op = newFakeCreateOp()
	op.retGIS = CreateSetDeleting
	expCalled = []string{"GIS", "SD", "DS", "DVS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeCreateOp()
	op.retGIS = CreateDeleteCGAndAG
	expCalled = []string{"GIS", "DCG", "DAG"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// VS and CG exist, DELETING not set in VS
	op = newFakeCreateOp()
	op.retGIS = CreateDeleteSnapshots
	expCalled = []string{"GIS", "DS", "DVS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeCreateOp()
	op.gisCG = &models.ConsistencyGroup{}
	op.retGIS = CreateDeleteSnapshots
	expCalled = []string{"GIS", "DS", "RVC", "DCG", "DAG", "DVS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// VS and CG exist, DELETING set in VS
	op = newFakeCreateOp()
	op.gisCG = &models.ConsistencyGroup{}
	op.retGIS = CreateRemoveVSFromCG
	expCalled = []string{"GIS", "RVC", "DCG", "DAG", "DVS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// VS and CG exist, cgID already empty
	op = newFakeCreateOp()
	op.gisCG = &models.ConsistencyGroup{}
	op.retGIS = CreateDeleteCG
	expCalled = []string{"GIS", "DCG", "DAG", "DVS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// VS and AGs exists
	op = newFakeCreateOp()
	op.gisCG = nil
	op.retGIS = CreateDeleteAGs
	expCalled = []string{"GIS", "DAG", "DVS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeCreateOp()
	op.rhs.RetryLater = true
	op.retGIS = CreateCG
	expCalled = []string{"GIS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// call the real handler with an error
	cg := &models.ConsistencyGroup{
		ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
			Meta: &models.ObjMeta{
				ID:      "CG-1",
				Version: 1,
			},
		},
		ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
			AccountID: "account1",
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
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc
	rhs := &vra.RequestHandlerState{
		A:       c.Animator,
		InError: true,
		Request: vr,
	}
	c.Create(nil, rhs)
	assert.Empty(fc.InCGListParams)
	assert.Empty(fc.InAGListParams)

	// call real handler in undo
	rhs.VolumeSeries = v
	rhs.HasCreate = true
	vr.VolumeSeriesRequestState = "UNDO_CREATING"
	vr.VolumeSeriesCreateSpec.ConsistencyGroupID = "CG-2"
	fc.RetCGFetchObj = cg
	fc.RetAGListObj = &application_group.ApplicationGroupListOK{Payload: []*models.ApplicationGroup{}}
	c.UndoCreate(nil, rhs)
	assert.EqualValues("CG-2", fc.InCGFetchID)
	assert.EqualValues(v.Meta.ID, fc.InDVObj)
	assert.Nil(rhs.VolumeSeries)
	assert.EqualValues(v.Meta.ID, vr.VolumeSeriesID)
	assert.Empty(fc.InCGDeleteID)
	assert.True(rhs.InError)

	// check state strings exist
	var ss createSubState
	for ss = CreateCG; ss <= CreateDeleteAGOnly; ss++ {
		s := ss.String()
		tl.Logger().Debugf("Testing %d %s", ss, s)
		assert.Regexp("^Create", s)
	}
	assert.Regexp("^createSubState", ss.String())
}

type fakeCreateOps struct {
	createOp
	called []string
	retGIS createSubState
	gisCG  *models.ConsistencyGroup
}

func (op *fakeCreateOps) getInitialState(ctx context.Context) createSubState {
	op.called = append(op.called, "GIS")
	op.cg = op.gisCG
	return op.retGIS
}

func (op *fakeCreateOps) createConsistencyGroup(ctx context.Context) {
	op.called = append(op.called, "CCG")
}

func (op *fakeCreateOps) createApplicationGroup(ctx context.Context) {
	op.called = append(op.called, "CAG")
}

func (op *fakeCreateOps) createVolumeSeries(ctx context.Context) {
	op.called = append(op.called, "CVS")
}

func (op *fakeCreateOps) setID(ctx context.Context) {
	op.called = append(op.called, "SID")
}

func (op *fakeCreateOps) setDeleting(ctx context.Context) {
	op.called = append(op.called, "SD")
}

func (op *fakeCreateOps) removeVSFromCG(ctx context.Context) {
	op.called = append(op.called, "RVC")
}

func (op *fakeCreateOps) deleteAGs(ctx context.Context) {
	op.called = append(op.called, "DAG")
}

func (op *fakeCreateOps) deleteCG(ctx context.Context) {
	op.called = append(op.called, "DCG")
}

func (op *fakeCreateOps) deleteVS(ctx context.Context) {
	op.called = append(op.called, "DVS")
}

func (op *fakeCreateOps) deleteVSSnapshots(ctx context.Context) {
	op.called = append(op.called, "DS")
}
