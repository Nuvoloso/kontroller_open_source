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
	"fmt"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	appServant "github.com/Nuvoloso/kontroller/pkg/agentd/fake"
	fmr "github.com/Nuvoloso/kontroller/pkg/agentd/metrics/fake"
	fakeState "github.com/Nuvoloso/kontroller/pkg/agentd/state/fake"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	mockcsp "github.com/Nuvoloso/kontroller/pkg/csp/mock"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	mockNuvo "github.com/Nuvoloso/kontroller/pkg/nuvoapi/mock"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestExportSteps(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	fas := &appServant.AppServant{}
	app.AppServant = fas
	fao := &appServant.AppObjects{}
	app.AppObjects = fao
	fso := fakeState.NewFakeNodeState()
	app.StateOps = fso
	app.LMDGuard = util.NewCriticalSectionGuard()
	assert.NotNil(app.LMDGuard)

	c := newComponent()
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc
	fm := &vscFakeMounter{}
	fmReset := func() {
		fm.ucVSR = nil
		fm.ucVSR = nil
		fm.retUCInError = false
	}
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
				CapacityReservationPlan: &models.CapacityReservationPlan{},
				RequestMessages:         []*models.TimestampedString{},
				StoragePlan: &models.StoragePlan{
					LayoutAlgorithm: "TBD",
					PlacementHints: map[string]models.ValueType{
						"property1": {Kind: "STRING", Value: "string3"},
						"property2": {Kind: "STRING", Value: "string4"},
					},
					StorageElements: []*models.StoragePlanStorageElement{},
				},
				VolumeSeriesRequestState: "VOLUME_EXPORT",
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID:         "node1",
				VolumeSeriesID: "VS-1",
			},
		},
	}
	nuvoVolIdentifier := "nuvo-vol-id"
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
				Messages:             []*models.TimestampedString{},
				VolumeSeriesState:    "IN_USE",
				Mounts:               []*models.Mount{},
				NuvoVolumeIdentifier: nuvoVolIdentifier,
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				Name:          "name",
				SizeBytes:     swag.Int64(3221225472),
				ServicePlanID: "plan1",
			},
		},
	}
	vsSnapshots := []*models.Snapshot{
		&models.Snapshot{
			SnapshotAllOf0: models.SnapshotAllOf0{
				Meta: &models.ObjMeta{
					ID: "id0",
				},
				SnapTime:       strfmt.DateTime(now),
				SnapIdentifier: "2",
				PitIdentifier:  "pit2",
			},
			SnapshotMutable: models.SnapshotMutable{},
		},
	}
	spObj := &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			Meta: &models.ObjMeta{ID: "SP-1"},
		},
		ServicePlanMutable: models.ServicePlanMutable{
			Slos: map[string]models.RestrictedValueType{
				"RPO": models.RestrictedValueType{ValueType: models.ValueType{Kind: "DURATION", Value: "4h"}},
			},
		},
	}
	nObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "node1",
			},
		},
		NodeMutable: models.NodeMutable{
			Service: &models.NuvoService{
				ServiceState: models.ServiceState{
					State: "RUNNING",
				},
			},
			Name:        "node1",
			Description: "node1 object",
			LocalStorage: map[string]models.NodeStorageDevice{
				"uuid1": models.NodeStorageDevice{
					DeviceName:      "d1",
					DeviceState:     "UNUSED",
					DeviceType:      "SSD",
					SizeBytes:       swag.Int64(1100),
					UsableSizeBytes: swag.Int64(1000),
				},
				"uuid2": models.NodeStorageDevice{
					DeviceName:      "d2",
					DeviceState:     "CACHE",
					DeviceType:      "SSD",
					SizeBytes:       swag.Int64(2200),
					UsableSizeBytes: swag.Int64(2000),
				},
			},
			AvailableCacheBytes: swag.Int64(3000),
			TotalCacheBytes:     swag.Int64(3300),
		},
	}
	rhs := &vra.RequestHandlerState{
		A:            c.Animator,
		HasMount:     true,
		Request:      vr,
		VolumeSeries: v,
	}
	devName := "VS-1-HEAD"
	op := &exportOp{
		c:          c,
		rhs:        rhs,
		mops:       fm,
		deviceName: devName,
	}

	// ***************************** getInitialState
	vr.PlanOnly = swag.Bool(true)
	assert.Equal(ExportDone, op.getInitialState(ctx))
	vr.PlanOnly = swag.Bool(false)

	op.snapID = com.VolMountHeadIdentifier
	op.pitUUID = ""
	v.Mounts = []*models.Mount{{MountState: "MOUNTING", SnapIdentifier: "HEAD"}}
	assert.Equal(ExportGetStatUUID, op.getInitialState(ctx))

	op.snapID = ""
	assert.Equal(ExportDone, op.getInitialState(ctx))
	assert.True(op.rhs.InError)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Either SnapIdentifier .* or PiT UUID .* missing", vr.RequestMessages[0].Message)
	rhs.InError = false      // restore
	vr.RequestMessages = nil // restore

	op.snapID = "SNAP-ID-FROM-SNAP-CREATE-VSR"
	op.pitUUID = vsSnapshots[0].PitIdentifier
	v.Mounts = []*models.Mount{{MountState: "MOUNTED", SnapIdentifier: "SNAP-ID-FROM-SNAP-CREATE-VSR"}}
	assert.Equal(ExportDone, op.getInitialState(ctx))
	assert.Equal(op.snapID, "SNAP-ID-FROM-SNAP-CREATE-VSR")
	assert.False(op.rhs.InError)
	assert.Equal("pit2", op.pitUUID)
	assert.False(op.isWritable)
	op.snapID = ""       // reset
	op.pitUUID = ""      // reset
	op.isWritable = true // reset

	vr.VolumeSeriesRequestState = "REALLOCATING_CACHE"
	assert.Equal(ExportSaveProps, op.getInitialState(ctx))

	vr.VolumeSeriesRequestState = "UNDO_REALLOCATING_CACHE"
	rhs.InError = true
	op.inError = false
	assert.Equal(ExportRestoreCache, op.getInitialState(ctx))
	assert.True(op.inError)
	assert.False(rhs.InError)

	vr.VolumeSeriesRequestState = "UNDO_VOLUME_EXPORT"
	op.snapID = com.VolMountHeadIdentifier
	v.Mounts = []*models.Mount{{MountState: "MOUNTING", SnapIdentifier: "HEAD"}}
	rhs.InError = true
	op.inError = false
	assert.Equal(ExportSetError, op.getInitialState(ctx))
	assert.True(op.inError)
	assert.False(rhs.InError)

	vr.VolumeSeriesRequestState = "UNDO_VOLUME_EXPORT"
	v.Mounts = []*models.Mount{{MountState: "MOUNTED", SnapIdentifier: "HEAD"}}
	rhs.InError = true
	op.inError = false
	assert.Equal(ExportSetUnmounting, op.getInitialState(ctx))
	assert.True(op.inError)
	assert.False(rhs.InError)

	vr.VolumeSeriesRequestState = "UNDO_VOLUME_EXPORT"
	v.Mounts = []*models.Mount{{MountState: "UNMOUNTING", SnapIdentifier: "HEAD"}}
	rhs.InError = true
	op.inError = false
	assert.Equal(ExportUnexportLun, op.getInitialState(ctx))
	assert.True(op.inError)
	assert.False(rhs.InError)

	vr.VolumeSeriesRequestState = "UNDO_VOLUME_EXPORT"
	v.Mounts = nil
	rhs.InError = true
	op.inError = false
	assert.Equal(ExportUndoDone, op.getInitialState(ctx))
	assert.True(op.inError)
	assert.False(rhs.InError)

	vr.VolumeSeriesRequestState = "VOLUME_EXPORT"
	op.snapID = com.VolMountHeadIdentifier
	v.Mounts = []*models.Mount{{MountState: "ERROR", SnapIdentifier: "HEAD"}} // recover from previous MOUNTING ERROR
	assert.Equal(ExportSetMounting, op.getInitialState(ctx))

	vr.VolumeSeriesRequestState = "VOLUME_EXPORT"
	v.Mounts = nil
	assert.Equal(ExportSetMounting, op.getInitialState(ctx))

	// ***************************** setMountState
	tl.Logger().Info("case: update failure")
	tl.Flush()
	fc.RetVSUpdaterObj, fc.RetVSUpdaterErr = nil, fmt.Errorf("update failure")
	vr.RequestedOperations = []string{"VOL_SNAPSHOT_CREATE"}
	v.SystemTags = nil
	assert.NotPanics(func() { op.setMountState(ctx, "MOUNTING") })
	assert.True(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Failed to update VolumeSeries.*: update failure", vr.RequestMessages[0].Message)
	assert.Equal([]string{"mounts", "volumeSeriesState", "configuredNodeId", "systemTags"}, fc.InVSUpdaterItems.Set)
	assert.Empty(fc.InVSUpdaterItems.Append) // no message during snapshot
	assert.Empty(fc.InVSUpdaterItems.Remove)
	assert.Empty(v.SystemTags)
	rhs.RetryLater = false
	vr.RequestedOperations = []string{"MOUNT"} // restore
	assert.False(op.mustUnconfigure)
	assert.False(op.resetSnapSchedData)
	assert.False(op.finalSnapshotNeeded)

	tl.Logger().Info("case: no-change (HEAD)")
	tl.Flush()
	fc.RetVSUpdaterObj, fc.RetVSUpdaterErr, fc.InVSUpdaterItems = nil, nil, nil
	fc.InUVRObj = nil
	v.Mounts = []*models.Mount{{MountState: "MOUNTING", SnapIdentifier: "HEAD"}}
	v.Messages = nil
	v.SystemTags = nil
	vr.RequestMessages = nil
	assert.NotPanics(func() { op.setMountState(ctx, "MOUNTING") })
	assert.Nil(fc.InUVRObj)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Equal([]string{"mounts", "volumeSeriesState", "configuredNodeId", "systemTags"}, fc.InVSUpdaterItems.Set)
	assert.Equal([]string{"messages"}, fc.InVSUpdaterItems.Append)
	assert.Empty(fc.InVSUpdaterItems.Remove)
	assert.Empty(v.SystemTags)
	assert.Len(vr.RequestMessages, 0)
	rhs.InError = false
	assert.Equal(v, rhs.VolumeSeries)
	assert.Nil(v.LifecycleManagementData)
	assert.False(op.mustUnconfigure)
	assert.False(op.resetSnapSchedData)
	assert.False(op.finalSnapshotNeeded)
	tl.Flush()

	tl.Logger().Info("case: No change (not HEAD)")
	tl.Flush()
	vrC := vsrClone(vr) // test inserted later
	vC := vsClone(v)    // test inserted later
	vrC.RequestMessages = []*models.TimestampedString{}
	fc.RetVSUpdaterObj, fc.RetVSUpdaterErr, fc.InVSUpdaterItems = nil, nil, nil
	fc.InUVRObj = nil
	vC.Mounts = []*models.Mount{{MountState: "MOUNTING", SnapIdentifier: "SNAP-1", PitIdentifier: "PIT-1"}}
	vC.Messages = nil
	vC.SystemTags = nil
	op.rhs.VolumeSeries = vC
	op.rhs.Request = vrC
	op.snapID = "SNAP-1"
	op.pitUUID = "PIT-1"
	assert.NotPanics(func() { op.setMountState(ctx, "MOUNTING") })
	assert.Nil(fc.InUVRObj)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Equal([]string{"mounts", "volumeSeriesState", "configuredNodeId", "systemTags"}, fc.InVSUpdaterItems.Set)
	assert.Equal([]string{"messages"}, fc.InVSUpdaterItems.Append)
	assert.Empty(fc.InVSUpdaterItems.Remove)
	assert.Len(vrC.RequestMessages, 0)
	assert.Empty(vC.SystemTags)
	assert.Len(vC.Mounts, 1)
	assert.Equal(op.pitUUID, vC.Mounts[0].PitIdentifier)
	assert.Equal(op.snapID, vC.Mounts[0].SnapIdentifier)
	assert.Equal(vC, rhs.VolumeSeries)
	assert.Nil(v.LifecycleManagementData)
	rhs.InError = false     // restore
	op.pitUUID = ""         // restore
	op.rhs.Request = vr     // restore
	op.rhs.VolumeSeries = v // restore
	assert.False(op.mustUnconfigure)
	assert.False(op.resetSnapSchedData)
	assert.False(op.finalSnapshotNeeded)
	tl.Flush()

	tl.Logger().Info("case: MOUNTED HEAD, write stats available")
	tl.Flush()
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cspAPI := mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cspAPI.EXPECT().ProtectionStoreUploadTransferRate().Return(int32(999))
	app.CSP = cspAPI
	fc.InUVRObj = nil
	fc.RetVSUpdaterObj, fc.RetVSUpdaterErr, fc.InVSUpdaterItems = nil, nil, nil
	v.Mounts = []*models.Mount{{MountState: "MOUNTING", SnapIdentifier: "HEAD"}}
	v.LifecycleManagementData = nil
	v.Messages = nil
	v.SystemTags = []string{fmt.Sprintf("%s:/mnt", com.SystemTagVolumeFsAttached)} // will be cleared
	vr.RequestMessages = nil
	op.snapID = "HEAD"
	op.writeStats = &nuvoapi.StatsIO{Count: 0, SeriesUUID: "SERIES-UUID"}
	assert.NotPanics(func() { op.setMountState(ctx, "MOUNTED") })
	assert.Nil(fc.InUVRObj)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Equal([]string{"mounts", "volumeSeriesState", "configuredNodeId", "systemTags", "lifecycleManagementData"}, fc.InVSUpdaterItems.Set)
	assert.Equal([]string{"messages"}, fc.InVSUpdaterItems.Append)
	assert.Empty(fc.InVSUpdaterItems.Remove)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Set mountState.*MOUNTED", vr.RequestMessages[0].Message)
	rhs.InError = false
	assert.Equal(v, rhs.VolumeSeries)
	assert.EqualValues([]string{fmt.Sprintf("%s:SERIES-UUID", com.SystemTagVolumeHeadStatSeries), fmt.Sprintf("%s:0", com.SystemTagVolumeHeadStatCount)}, v.SystemTags)
	assert.NotZero(v.Mounts[0].MountTime)
	assert.Equal(v.Messages[0].Time, v.Mounts[0].MountTime)
	assert.NotNil(v.LifecycleManagementData)
	lmd := v.LifecycleManagementData
	assert.EqualValues(0, lmd.EstimatedSizeBytes)
	assert.False(lmd.FinalSnapshotNeeded)
	assert.Equal("", lmd.GenUUID)
	assert.Equal(time.Time{}, time.Time(lmd.LastSnapTime))
	assert.Equal(v.Mounts[0].MountTime, lmd.NextSnapshotTime)
	assert.Equal(time.Time{}, time.Time(lmd.LastUploadTime))
	assert.EqualValues(0, lmd.LastUploadSizeBytes)
	assert.EqualValues(1.0, lmd.SizeEstimateRatio)
	assert.EqualValues(999, lmd.LastUploadTransferRateBPS)
	v.LifecycleManagementData = nil // reset
	assert.False(op.mustUnconfigure)
	assert.True(op.resetSnapSchedData)
	assert.False(op.finalSnapshotNeeded)
	mockCtrl.Finish()

	tl.Logger().Info("case: MOUNTED HEAD, write stats not available")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	cspAPI = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cspAPI.EXPECT().ProtectionStoreUploadTransferRate().Return(int32(999))
	app.CSP = cspAPI
	fc.InUVRObj = nil
	fc.RetVSUpdaterObj, fc.RetVSUpdaterErr, fc.InVSUpdaterItems = nil, nil, nil
	v.Mounts = []*models.Mount{{MountState: "MOUNTING", SnapIdentifier: "HEAD"}}
	v.LifecycleManagementData = nil
	v.Messages = nil
	v.SystemTags = []string{fmt.Sprintf("%s:SOMETHING", com.SystemTagVolumeHeadStatSeries), fmt.Sprintf("%s:/mnt", com.SystemTagVolumeFsAttached)} // both will be cleared
	vr.RequestMessages = nil
	op.writeStats = nil
	assert.NotPanics(func() { op.setMountState(ctx, "MOUNTED") })
	assert.Nil(fc.InUVRObj)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Equal([]string{"mounts", "volumeSeriesState", "configuredNodeId", "systemTags", "lifecycleManagementData"}, fc.InVSUpdaterItems.Set)
	assert.Equal([]string{"messages"}, fc.InVSUpdaterItems.Append)
	assert.Empty(fc.InVSUpdaterItems.Remove)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Set mountState.*MOUNTED", vr.RequestMessages[0].Message)
	rhs.InError = false
	assert.Equal(v, rhs.VolumeSeries)
	assert.Empty(v.SystemTags)
	assert.NotZero(v.Mounts[0].MountTime)
	assert.Equal(v.Messages[0].Time, v.Mounts[0].MountTime)
	assert.NotNil(v.LifecycleManagementData)
	lmd = v.LifecycleManagementData
	assert.EqualValues(0, lmd.EstimatedSizeBytes)
	assert.False(lmd.FinalSnapshotNeeded)
	assert.Equal("", lmd.GenUUID)
	assert.Equal(time.Time{}, time.Time(lmd.LastSnapTime))
	assert.Equal(v.Mounts[0].MountTime, lmd.NextSnapshotTime)
	assert.Equal(time.Time{}, time.Time(lmd.LastUploadTime))
	assert.EqualValues(0, lmd.LastUploadSizeBytes)
	assert.EqualValues(1.0, lmd.SizeEstimateRatio)
	assert.EqualValues(999, lmd.LastUploadTransferRateBPS)
	v.LifecycleManagementData = nil // reset
	assert.False(op.mustUnconfigure)
	assert.True(op.resetSnapSchedData)
	assert.False(op.finalSnapshotNeeded)

	tl.Logger().Info("case: UNMOUNTING/updater error")
	tl.Flush()
	fc.RetVSUpdaterUpdateErr = fmt.Errorf("updater error")
	fc.ModVSRUpdaterObj = nil
	v.Mounts = []*models.Mount{{MountState: "MOUNTING", SnapIdentifier: "HEAD"}}
	v.Messages = nil
	v.SystemTags = nil
	vr.RequestMessages = nil
	assert.NotPanics(func() { op.setMountState(ctx, "UNMOUNTING") })
	assert.NotNil(fc.ModVSRUpdaterObj)
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	assert.Len(v.Messages, 1)
	assert.Regexp("Set mountState.*UNMOUNTING", v.Messages[0].Message)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("updater error", vr.RequestMessages[0].Message)
	rhs.RetryLater = false
	assert.Equal(v, rhs.VolumeSeries)
	assert.Empty(v.SystemTags)
	assert.NotZero(v.Mounts[0].MountTime)
	v.LifecycleManagementData = nil // reset
	assert.False(op.mustUnconfigure)
	assert.False(op.resetSnapSchedData)
	assert.False(op.finalSnapshotNeeded)

	tl.Logger().Info("case: UNMOUNTING")
	tl.Flush()
	fc.RetVSUpdaterUpdateErr = nil
	fc.InUVRObj = nil
	v.Mounts = nil
	v.Messages = nil
	v.SystemTags = nil
	vr.RequestMessages = nil
	assert.NotPanics(func() { op.setMountState(ctx, "UNMOUNTING") })
	assert.Nil(fc.InUVRObj)
	assert.True(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Empty(v.SystemTags)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("mountState.*not found", vr.RequestMessages[0].Message)
	rhs.InError = false
	v.LifecycleManagementData = nil // reset
	assert.False(op.mustUnconfigure)
	assert.False(op.resetSnapSchedData)
	assert.False(op.finalSnapshotNeeded)

	tl.Logger().Info("case: UNMOUNTED, no mounts")
	tl.Flush()
	fc.RetVSUpdaterUpdateErr = nil
	fc.InUVRObj = nil
	v.Mounts = nil
	v.Messages = nil
	vr.RequestMessages = nil
	assert.NotPanics(func() { op.setMountState(ctx, "UNMOUNTED") })
	assert.Nil(fc.InUVRObj)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Empty(v.SystemTags)
	assert.Len(vr.RequestMessages, 0)
	rhs.InError = false
	v.LifecycleManagementData = nil // reset
	assert.False(op.mustUnconfigure)
	assert.False(op.resetSnapSchedData)
	assert.True(op.finalSnapshotNeeded)

	lastHeadUnexportRecorded := func(v *models.VolumeSeries, tB, tA time.Time) {
		st := util.NewTagList(v.SystemTags)
		val, ok := st.Get(common.SystemTagVolumeLastHeadUnexport)
		assert.True(ok)
		stv, err := strfmt.ParseDateTime(val)
		assert.NoError(err, "Parse failed")
		// strfmt loses precision
		tB, tA = time.Time(tB).Round(time.Millisecond).Add(-1*time.Millisecond), time.Time(tA).Round(time.Millisecond)
		tv := time.Time(stv)
		assert.True(tB.Equal(tv) || tB.Before(tv), "Before check failed")
		assert.True(tA.Equal(tv) || tA.After(tv), "After check failed")
	}

	tl.Logger().Info("case: UNMOUNTED HEAD/IN_USE (2 mounts)")
	tl.Flush()
	fc.RetVSUpdaterObj, fc.RetVSUpdaterErr, fc.InVSUpdaterItems = nil, nil, nil
	v.Mounts = []*models.Mount{
		{MountState: "UNMOUNTING", SnapIdentifier: "HEAD"},
		{MountState: "MOUNTING", SnapIdentifier: "2"},
	}
	v.ConfiguredNodeID = "node1"
	v.Messages = nil
	v.SystemTags = nil
	vr.RequestMessages = nil
	vr.MountedNodeDevice = devName
	tB := time.Now()
	assert.NotPanics(func() { op.setMountState(ctx, "UNMOUNTED") })
	tA := time.Now()
	assert.Nil(fc.InUVRObj)
	assert.Equal([]string{"mounts", "volumeSeriesState", "configuredNodeId", "systemTags", "lifecycleManagementData"}, fc.InVSUpdaterItems.Set)
	assert.Equal([]string{"messages"}, fc.InVSUpdaterItems.Append)
	assert.Empty(fc.InVSUpdaterItems.Remove)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Set mountState.*UNMOUNTED", vr.RequestMessages[0].Message)
	assert.Zero(rhs.Request.MountedNodeDevice)
	assert.Len(v.Mounts, 1)
	assert.Equal("2", v.Mounts[0].SnapIdentifier)
	assert.Equal("IN_USE", v.VolumeSeriesState)
	assert.NotEmpty(v.ConfiguredNodeID)
	assert.NotNil(v.LifecycleManagementData)
	assert.True(v.LifecycleManagementData.FinalSnapshotNeeded)
	assert.True(tB.Before(time.Time(v.LifecycleManagementData.NextSnapshotTime)))
	assert.True(tA.After(time.Time(v.LifecycleManagementData.NextSnapshotTime)))
	lastHeadUnexportRecorded(v, tB, tA)
	v.LifecycleManagementData = nil // reset
	assert.False(op.mustUnconfigure)
	assert.False(op.resetSnapSchedData)
	assert.True(op.finalSnapshotNeeded)

	tl.Logger().Info("case: UNMOUNTED/CONFIGURED final snapshot required")
	tl.Flush()
	op.ignoreFSN = false
	v.Mounts = []*models.Mount{
		{MountState: "UNMOUNTING", SnapIdentifier: "HEAD"},
	}
	v.Messages = nil
	v.SystemTags = []string{fmt.Sprintf("%s:SOMETHING", com.SystemTagVolumeHeadStatSeries), fmt.Sprintf("%s:/mnt", com.SystemTagVolumeFsAttached)}
	vr.RequestMessages = nil
	tB = time.Now()
	assert.NotPanics(func() { op.setMountState(ctx, "UNMOUNTED") })
	tA = time.Now()
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(v.Mounts, 0)
	assert.Nil(fc.InUVRObj)
	assert.Len(vr.RequestMessages, 2)
	assert.Regexp("Set mountState.*UNMOUNTED", vr.RequestMessages[0].Message)
	assert.Regexp("Set volume state.*CONFIGURED", vr.RequestMessages[1].Message)
	assert.Equal(v, rhs.VolumeSeries)
	assert.Equal("CONFIGURED", v.VolumeSeriesState)
	assert.NotEmpty(v.ConfiguredNodeID)
	assert.NotNil(v.LifecycleManagementData)
	assert.True(v.LifecycleManagementData.FinalSnapshotNeeded)
	assert.True(tB.Before(time.Time(v.LifecycleManagementData.NextSnapshotTime)))
	assert.True(tA.After(time.Time(v.LifecycleManagementData.NextSnapshotTime)))
	lastHeadUnexportRecorded(v, tB, tA)
	assert.False(func() bool {
		st := util.NewTagList(v.SystemTags)
		_, ok := st.Get(common.SystemTagVolumeHeadStatSeries)
		return ok
	}())
	assert.False(func() bool {
		st := util.NewTagList(v.SystemTags)
		_, ok := st.Get(common.SystemTagVolumeFsAttached)
		return ok
	}())
	assert.False(op.mustUnconfigure) // override
	assert.False(op.resetSnapSchedData)
	assert.True(op.finalSnapshotNeeded) // ignored
	v.LifecycleManagementData = nil     // reset
	op.ignoreFSN = false                // reset
	op.finalSnapshotNeeded = false      // reset
	op.mustUnconfigure = false          // reset

	tl.Logger().Info("case: UNMOUNTED/CONFIGURED final snapshot not needed, also forces unconfigure")
	tl.Flush()
	op.ignoreFSN = true
	v.Mounts = []*models.Mount{
		{MountState: "UNMOUNTING", SnapIdentifier: "HEAD"},
	}
	v.Messages = nil
	v.SystemTags = []string{fmt.Sprintf("%s:SOMETHING", com.SystemTagVolumeHeadStatSeries)}
	vr.RequestMessages = nil
	tB = time.Now()
	assert.NotPanics(func() { op.setMountState(ctx, "UNMOUNTED") })
	tA = time.Now()
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(v.Mounts, 0)
	assert.Nil(fc.InUVRObj)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Set mountState.*UNMOUNTED", vr.RequestMessages[0].Message)
	assert.Equal(v, rhs.VolumeSeries)
	assert.Equal("CONFIGURED", v.VolumeSeriesState)
	assert.NotEmpty(v.ConfiguredNodeID)
	assert.NotNil(v.LifecycleManagementData)
	assert.False(v.LifecycleManagementData.FinalSnapshotNeeded)
	lastHeadUnexportRecorded(v, tB, tA)
	assert.False(func() bool {
		st := util.NewTagList(v.SystemTags)
		_, ok := st.Get(common.SystemTagVolumeHeadStatSeries)
		return ok
	}())
	assert.True(op.mustUnconfigure) // override
	assert.False(op.resetSnapSchedData)
	assert.False(op.finalSnapshotNeeded) // ignored
	v.LifecycleManagementData = nil      // reset
	op.ignoreFSN = false                 // reset
	op.finalSnapshotNeeded = false       // reset
	op.mustUnconfigure = false           // reset

	tl.Logger().Info("case: UNMOUNTED/IN_USE final snapshot, no unconfigure, no writeStats, finalSnapshot needed")
	tl.Flush()
	v.Mounts = []*models.Mount{
		{MountState: "UNMOUNTING", SnapIdentifier: "HEAD"},
	}
	v.VolumeSeriesState = "IN_USE"
	v.Messages = nil
	v.SystemTags = nil
	vr.RequestMessages = nil
	tB = time.Now()
	assert.NotPanics(func() { op.setMountState(ctx, "UNMOUNTED") })
	tA = time.Now()
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Nil(fc.InUVRObj)
	assert.Len(vr.RequestMessages, 2)
	assert.Regexp("Set mountState.*UNMOUNTED", vr.RequestMessages[0].Message)
	assert.Regexp("Set volume state.*CONFIGURED", vr.RequestMessages[1].Message)
	assert.Len(v.Messages, 2)
	assert.Regexp("Set mountState.*UNMOUNTED", v.Messages[0].Message)
	assert.Regexp("State change IN_USE ⇒ CONFIGURED", v.Messages[1].Message)
	assert.Len(v.Mounts, 0)
	assert.Equal(v, rhs.VolumeSeries)
	assert.Equal("CONFIGURED", v.VolumeSeriesState)
	assert.NotEmpty(v.ConfiguredNodeID)
	assert.NotNil(v.LifecycleManagementData)
	assert.True(v.LifecycleManagementData.FinalSnapshotNeeded)
	assert.True(tB.Before(time.Time(v.LifecycleManagementData.NextSnapshotTime)))
	assert.True(tA.After(time.Time(v.LifecycleManagementData.NextSnapshotTime)))
	lastHeadUnexportRecorded(v, tB, tA)
	assert.False(op.mustUnconfigure)
	assert.False(op.resetSnapSchedData)
	assert.True(op.finalSnapshotNeeded)
	op.finalSnapshotNeeded = false  // reset
	v.LifecycleManagementData = nil // reset
	op.writeStats = nil             // reset

	tl.Logger().Info("case: UNMOUNTED/IN_USE final snapshot, no unconfigure, writeStats, finalSnapshot needed")
	tl.Flush()
	v.Mounts = []*models.Mount{
		{MountState: "UNMOUNTING", SnapIdentifier: "HEAD"},
	}
	v.VolumeSeriesState = "IN_USE"
	v.Messages = nil
	v.SystemTags = nil
	vr.RequestMessages = nil
	v.SystemTags = []string{fmt.Sprintf("%s:SERIES-UUID", com.SystemTagVolumeHeadStatSeries), fmt.Sprintf("%s:10", com.SystemTagVolumeHeadStatCount)}
	op.writeStats = &nuvoapi.StatsIO{Count: 20, SeriesUUID: "SERIES-UUID"}
	assert.Equal(com.VolMountHeadIdentifier, op.snapID)
	tB = time.Now()
	assert.NotPanics(func() { op.setMountState(ctx, "UNMOUNTED") })
	tA = time.Now()
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Nil(fc.InUVRObj)
	assert.Len(vr.RequestMessages, 2)
	assert.Regexp("Set mountState.*UNMOUNTED", vr.RequestMessages[0].Message)
	assert.Regexp("Set volume state.*CONFIGURED", vr.RequestMessages[1].Message)
	assert.Len(v.Messages, 2)
	assert.Regexp("Set mountState.*UNMOUNTED", v.Messages[0].Message)
	assert.Regexp("State change IN_USE ⇒ CONFIGURED", v.Messages[1].Message)
	assert.Len(v.Mounts, 0)
	assert.Equal(v, rhs.VolumeSeries)
	assert.Equal("CONFIGURED", v.VolumeSeriesState)
	assert.NotEmpty(v.ConfiguredNodeID)
	assert.NotNil(v.LifecycleManagementData)
	assert.True(v.LifecycleManagementData.FinalSnapshotNeeded)
	assert.True(tB.Before(time.Time(v.LifecycleManagementData.NextSnapshotTime)))
	assert.True(tA.After(time.Time(v.LifecycleManagementData.NextSnapshotTime)))
	lastHeadUnexportRecorded(v, tB, tA)
	assert.False(op.mustUnconfigure)
	assert.False(op.resetSnapSchedData)
	assert.True(op.finalSnapshotNeeded)
	op.finalSnapshotNeeded = false  // reset
	v.LifecycleManagementData = nil // reset
	op.writeStats = nil             // reset

	tl.Logger().Info("case: UNMOUNTED/IN_USE final snapshot, unconfigure, writeStats, finalSnapshot not needed")
	tl.Flush()
	v.Mounts = []*models.Mount{
		{MountState: "UNMOUNTING", SnapIdentifier: "HEAD"},
	}
	v.VolumeSeriesState = "IN_USE"
	v.Messages = nil
	v.SystemTags = nil
	vr.RequestMessages = nil
	v.SystemTags = []string{fmt.Sprintf("%s:SERIES-UUID", com.SystemTagVolumeHeadStatSeries), fmt.Sprintf("%s:10", com.SystemTagVolumeHeadStatCount)}
	op.writeStats = &nuvoapi.StatsIO{Count: 10, SeriesUUID: "SERIES-UUID"}
	assert.Equal(com.VolMountHeadIdentifier, op.snapID)
	tB = time.Now()
	assert.NotPanics(func() { op.setMountState(ctx, "UNMOUNTED") })
	tA = time.Now()
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Nil(fc.InUVRObj)
	assert.Len(vr.RequestMessages, 2)
	assert.Regexp("Set mountState.*UNMOUNTED", vr.RequestMessages[0].Message)
	assert.Regexp("Set volume state.*CONFIGURED", vr.RequestMessages[1].Message)
	assert.Len(v.Messages, 2)
	assert.Regexp("Set mountState.*UNMOUNTED", v.Messages[0].Message)
	assert.Regexp("State change IN_USE ⇒ CONFIGURED", v.Messages[1].Message)
	assert.Len(v.Mounts, 0)
	assert.Equal(v, rhs.VolumeSeries)
	assert.Equal("CONFIGURED", v.VolumeSeriesState)
	assert.NotEmpty(v.ConfiguredNodeID)
	assert.NotNil(v.LifecycleManagementData)
	assert.False(v.LifecycleManagementData.FinalSnapshotNeeded)
	lastHeadUnexportRecorded(v, tB, tA)
	assert.True(op.mustUnconfigure)
	assert.False(op.resetSnapSchedData)
	assert.False(op.finalSnapshotNeeded)
	v.LifecycleManagementData = nil // reset
	op.writeStats = nil             // reset
	op.mustUnconfigure = false      // reset

	tl.Logger().Info("case: MOUNTING, PROVISIONED ⇒ IN_USE")
	tl.Flush()
	fc.InVSUpdaterItems = nil
	v.Mounts = nil
	v.Messages = nil
	v.SystemTags = nil
	vr.RequestMessages = nil
	v.VolumeSeriesState = "PROVISIONED"
	v.ConfiguredNodeID = ""
	assert.NotPanics(func() { op.setMountState(ctx, "MOUNTING") })
	assert.Nil(fc.InUVRObj)
	assert.Equal([]string{"mounts", "volumeSeriesState", "configuredNodeId", "systemTags"}, fc.InVSUpdaterItems.Set)
	assert.Equal([]string{"messages"}, fc.InVSUpdaterItems.Append)
	assert.Empty(fc.InVSUpdaterItems.Remove)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 2)
	assert.Regexp("Set mountState.*MOUNTING", vr.RequestMessages[0].Message)
	assert.Regexp("Set volume state.*IN_USE", vr.RequestMessages[1].Message)
	assert.Equal(v, rhs.VolumeSeries)
	assert.Equal("IN_USE", v.VolumeSeriesState)
	assert.NotEmpty(v.ConfiguredNodeID)
	assert.Len(v.Mounts, 1)
	assert.NotZero(v.Mounts[0].MountTime)
	expMount := models.Mount{
		MountMode:         "READ_WRITE",
		MountState:        "MOUNTING",
		MountTime:         v.Mounts[0].MountTime,
		MountedNodeDevice: devName,
		MountedNodeID:     "node1",
		SnapIdentifier:    "HEAD",
	}
	assert.EqualValues(expMount, *v.Mounts[0])
	assert.Len(v.Messages, 2)
	assert.Equal(v.Messages[0].Time, v.Mounts[0].MountTime)
	assert.Regexp("Set mountState.*MOUNTING", v.Messages[0].Message)
	assert.Regexp("State change PROVISIONED ⇒ IN_USE", v.Messages[1].Message)
	assert.Empty(v.SystemTags)
	assert.NotNil(v.LifecycleManagementData)
	assert.False(v.LifecycleManagementData.FinalSnapshotNeeded)
	assert.False(op.mustUnconfigure)
	assert.False(op.resetSnapSchedData)
	assert.False(op.finalSnapshotNeeded)

	tl.Logger().Info("case: MOUNTING, CONFIGURED ⇒ IN_USE")
	tl.Flush()
	fc.InVSUpdaterItems = nil
	v.Mounts = nil
	v.Messages = nil
	v.SystemTags = nil
	v.VolumeSeriesState = "CONFIGURED"
	vr.RequestMessages = []*models.TimestampedString{}
	assert.NotPanics(func() { op.setMountState(ctx, "MOUNTING") })
	assert.Nil(fc.InUVRObj)
	assert.Equal([]string{"mounts", "volumeSeriesState", "configuredNodeId", "systemTags"}, fc.InVSUpdaterItems.Set)
	assert.Equal([]string{"messages"}, fc.InVSUpdaterItems.Append)
	assert.Empty(fc.InVSUpdaterItems.Remove)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 2)
	assert.Regexp("Set mountState.*MOUNTING", vr.RequestMessages[0].Message)
	assert.Regexp("Set volume state.*IN_USE", vr.RequestMessages[1].Message)
	assert.Equal(v, rhs.VolumeSeries)
	assert.Equal("IN_USE", v.VolumeSeriesState)
	assert.NotEmpty(v.ConfiguredNodeID)
	assert.Len(v.Mounts, 1)
	assert.NotZero(v.Mounts[0].MountTime)
	expMount = models.Mount{
		MountMode:         "READ_WRITE",
		MountState:        "MOUNTING",
		MountTime:         v.Mounts[0].MountTime,
		MountedNodeDevice: devName,
		MountedNodeID:     "node1",
		SnapIdentifier:    "HEAD",
	}
	assert.EqualValues(expMount, *v.Mounts[0])
	assert.Len(v.Messages, 2)
	assert.Equal(v.Messages[0].Time, v.Mounts[0].MountTime)
	assert.Regexp("Set mountState.*MOUNTING", v.Messages[0].Message)
	assert.Regexp("State change CONFIGURED ⇒ IN_USE", v.Messages[1].Message)
	assert.Empty(v.SystemTags)
	assert.NotNil(v.LifecycleManagementData)
	assert.False(v.LifecycleManagementData.FinalSnapshotNeeded)
	assert.False(op.mustUnconfigure)
	assert.False(op.resetSnapSchedData)
	assert.False(op.finalSnapshotNeeded)
	tl.Flush()

	//  ***************************** fetchServicePlan

	tl.Logger().Infof("case: fetchServicePlan")
	tl.Flush()
	rhs.InError = false
	rhs.RetryLater = false
	fas.RetGSPObj = spObj
	fas.RetGSPErr = nil
	op.fetchServicePlan(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.EqualValues(op.rhs.VolumeSeries.ServicePlanID, fas.InGSPid)

	tl.Logger().Infof("case: fetchServicePlan (error)")
	tl.Flush()
	rhs.InError = false
	rhs.RetryLater = false
	fas.RetGSPObj = nil
	fas.RetGSPErr = fmt.Errorf("sp-fetch-error")
	op.fetchServicePlan(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)

	// ***************************** allocateCache
	tl.Logger().Infof("case: allocateCache (failure to get lock)")
	app.LMDGuard.Drain() // force closure
	vr.RequestMessages = nil
	vs := vsClone(v)
	vsr := vsrClone(vr)
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	assert.NotPanics(func() { op.allocateCache(ctx) })
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("state lock error", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Infof("case: allocateCache (successful locking)")
	app.LMDGuard = util.NewCriticalSectionGuard()
	vsr.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	assert.NotPanics(func() { op.allocateCache(ctx) })
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal([]string{}, app.LMDGuard.Status()) // lock released
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Cache allocation is not required", op.rhs.Request.RequestMessages[0].Message)

	// reset
	op.rhs.Request = vr
	op.rhs.VolumeSeries = v

	tl.Logger().Infof("case: allocateCache (error injection)")
	rhs.InError = false
	vr.RequestMessages = nil
	c.rei.SetProperty("fail-in-allocate-cache", &rei.Property{BoolValue: true})
	assert.NotPanics(func() { op.allocateCache(ctx) })
	assert.True(rhs.InError)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("injecting error", op.rhs.Request.RequestMessages[0])

	tl.Logger().Infof("case: allocateCache (no cache required)")
	sE1 := []*models.StoragePlanStorageElement{
		{
			Intent:    "CACHE",
			SizeBytes: swag.Int64(0),
			StorageParcels: map[string]models.StorageParcelElement{
				"SSD": models.StorageParcelElement{ProvMinSizeBytes: swag.Int64(0)},
			},
			PoolID: "provider1",
		},
	}
	vr.StoragePlan.StorageElements = sE1
	vr.RequestMessages = nil
	rhs.RetryLater = false
	rhs.InError = false
	fao.Node = nObj
	assert.NotPanics(func() { op.allocateCache(ctx) })
	assert.False(rhs.InError)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Cache allocation is not required", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Infof("case: allocateCache (no cache requirements in StoragePlan)")
	sE2 := []*models.StoragePlanStorageElement{ // no Intent CACHE
		{
			Intent:    "DATA",
			SizeBytes: swag.Int64(10),
			StorageParcels: map[string]models.StorageParcelElement{
				"SSD": models.StorageParcelElement{ProvMinSizeBytes: swag.Int64(1)},
			},
			PoolID: "provider1",
		},
	}
	vr.StoragePlan.StorageElements = sE2
	vr.RequestMessages = nil
	rhs.RetryLater = false
	rhs.InError = false
	fao.Node = nObj
	assert.NotPanics(func() { op.allocateCache(ctx) })
	assert.False(rhs.InError)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Cache allocation is not required", op.rhs.Request.RequestMessages[0].Message)

	// tests handling of getCacheRequired() failure, fully tested separately
	tl.Logger().Infof("case: allocateCache (mismatched cache types in Node and StoragePlan)")
	sE3 := []*models.StoragePlanStorageElement{
		{
			Intent:    "CACHE",
			SizeBytes: swag.Int64(10),
			StorageParcels: map[string]models.StorageParcelElement{
				"HDD": models.StorageParcelElement{ProvMinSizeBytes: swag.Int64(1)},
			},
			PoolID: "provider1",
		},
	}
	vr.StoragePlan.StorageElements = sE3
	vr.RequestMessages = nil
	rhs.RetryLater = false
	rhs.InError = false
	fao.Node = nObj
	assert.NotPanics(func() { op.allocateCache(ctx) })
	assert.True(rhs.InError)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("AllocateCache error: Mismatched cache types .* for cache required by StoragePlan", vr.RequestMessages[0].Message)

	tl.Logger().Infof("case: allocateCache (failure to update agentd cache state)")
	sE4 := []*models.StoragePlanStorageElement{
		{
			Intent:    "CACHE",
			SizeBytes: swag.Int64(10),
			StorageParcels: map[string]models.StorageParcelElement{
				"SSD": models.StorageParcelElement{ProvMinSizeBytes: swag.Int64(2)},
			},
			PoolID: "provider1",
		},
	}
	vr.StoragePlan.StorageElements = sE4
	vr.RequestMessages = nil
	rhs.RetryLater = false
	rhs.InError = false
	fao.Node = nObj
	fso.RetClaimCacheErr = fmt.Errorf("Failure to update cache state")
	assert.NotPanics(func() { op.allocateCache(ctx) })
	assert.True(rhs.InError)
	assert.EqualValues(v.Meta.ID, fso.InClaimCacheVsID)
	assert.EqualValues(2, fso.InClaimCacheMsb)
	assert.EqualValues(10, fso.InClaimCacheDsb)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Failure to update node cache state: Failure to update cache state", vr.RequestMessages[0].Message)
	mockCtrl.Finish()

	tl.Logger().Infof("case: allocateCache success (cache already claimed)")
	mockCtrl = gomock.NewController(t)
	nvAPI := mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().AllocCache(nuvoVolIdentifier, uint64(10)).Return(nil)
	c.App.NuvoAPI = nvAPI
	vr.StoragePlan.StorageElements = sE4
	vr.RequestMessages = nil
	rhs.RetryLater = false
	rhs.InError = false
	fao.Node = nObj
	fso.InClaimCacheVsID = ""
	fso.InClaimCacheMsb, fso.InClaimCacheDsb = 0, 0
	fso.RetClaimCacheUsb = 10
	fso.RetClaimCacheErr = nil
	fc.RetNObj = nObj
	fc.InVSUpdaterID = ""
	fc.InVSUpdaterItems, fc.RetVSUpdaterErr = nil, nil
	assert.NotPanics(func() { op.allocateCache(ctx) })
	assert.False(rhs.InError)
	assert.EqualValues(v.Meta.ID, fso.InClaimCacheVsID)
	assert.EqualValues(2, fso.InClaimCacheMsb)
	assert.EqualValues(10, fso.InClaimCacheDsb)
	assert.EqualValues(v.Meta.ID, fc.InVSUpdaterID)
	assert.Equal([]string{"cacheAllocations"}, fc.InVSUpdaterItems.Append)
	assert.Empty(fc.InVSUpdaterItems.Remove)
	assert.Empty(fc.InVSUpdaterItems.Set)
	expCA := map[string]models.CacheAllocation{
		"node1": {AllocatedSizeBytes: swag.Int64(10), RequestedSizeBytes: swag.Int64(10)},
	}
	assert.Equal(expCA, v.CacheAllocations)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Allocated cache .requested=10B allocated=10B.", vr.RequestMessages[0].Message)
	v.CacheAllocations = map[string]models.CacheAllocation{} // reset
	mockCtrl.Finish()

	tl.Logger().Infof("case: allocateCache (NUVOAPI error)")
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().AllocCache(nuvoVolIdentifier, uint64(10)).Return(fmt.Errorf("allocate-cache--error"))
	c.App.NuvoAPI = nvAPI
	vr.RequestMessages = nil
	rhs.InError = false
	fso.InClaimCacheVsID = ""
	fso.InClaimCacheMsb, fso.InClaimCacheDsb = 0, 0
	fso.RetClaimCacheUsb = 10
	fso.RetClaimCacheErr = nil
	fso.RetGetNodeAvailableCacheBytes = swag.Int64Value(nObj.AvailableCacheBytes)
	fso.InReleaseCacheVsID = ""
	assert.NotPanics(func() { op.allocateCache(ctx) })
	assert.True(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.EqualValues(v.Meta.ID, fso.InClaimCacheVsID)
	assert.EqualValues(2, fso.InClaimCacheMsb)
	assert.EqualValues(v.Meta.ID, fso.InReleaseCacheVsID)
	assert.EqualValues(v.Meta.ID, fc.InVSUpdaterID)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("NUVOAPI AllocCache error: .*", vr.RequestMessages[0].Message)

	tl.Logger().Infof("case: allocateCache (NUVOAPI temp error)")
	nvAPI.EXPECT().AllocCache(nuvoVolIdentifier, uint64(10)).Return(nuvoapi.NewNuvoAPIError("tempErr", true))
	c.App.NuvoAPI = nvAPI
	vr.RequestMessages = nil
	rhs.InError = false
	fso.InClaimCacheVsID = ""
	fso.InClaimCacheMsb, fso.InClaimCacheDsb = 0, 0
	fso.RetClaimCacheUsb = 10
	fso.RetClaimCacheErr = nil
	fso.RetGetNodeAvailableCacheBytes = swag.Int64Value(nObj.AvailableCacheBytes)
	fso.InReleaseCacheVsID = ""
	assert.NotPanics(func() { op.allocateCache(ctx) })
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	assert.EqualValues(v.Meta.ID, fso.InClaimCacheVsID)
	assert.EqualValues(2, fso.InClaimCacheMsb)
	assert.EqualValues(v.Meta.ID, fso.InReleaseCacheVsID)
	assert.EqualValues(v.Meta.ID, fc.InVSUpdaterID)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("NUVOAPI temporary error: .*", vr.RequestMessages[0].Message)

	tl.Logger().Infof("case: allocateCache (no need to update Node's object allocated cache)")
	nvAPI.EXPECT().AllocCache(nuvoVolIdentifier, uint64(10)).Return(nil)
	c.App.NuvoAPI = nvAPI
	vr.RequestMessages = nil
	rhs.RetryLater = false
	rhs.InError = false
	fso.RetClaimCacheErr = nil
	fso.RetGetNodeAvailableCacheBytes = swag.Int64Value(nObj.AvailableCacheBytes)
	fc.RetNErr = nil
	fc.RetNObj = nObj
	fc.RetNodeUpdateErr = fmt.Errorf("Failure to update Node")
	fc.RetVSUpdaterErr = nil
	assert.NotPanics(func() { op.allocateCache(ctx) })
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.EqualValues(v.Meta.ID, fso.InClaimCacheVsID)
	assert.EqualValues(v.Meta.ID, fc.InVSUpdaterID)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Allocated cache .requested=10B allocated=10B.", vr.RequestMessages[0].Message)
	v.CacheAllocations = map[string]models.CacheAllocation{} // reset

	tl.Logger().Infof("case: allocateCache (failure to update Node's object allocated cache)")
	nvAPI.EXPECT().AllocCache(nuvoVolIdentifier, uint64(5)).Return(nil)
	c.App.NuvoAPI = nvAPI
	vr.RequestMessages = nil
	rhs.RetryLater = false
	rhs.InError = false
	nObj.AvailableCacheBytes = swag.Int64(3000)
	fso.InClaimCacheVsID = ""
	fso.InClaimCacheMsb, fso.InClaimCacheDsb = 0, 0
	fso.RetClaimCacheErr = nil
	fso.RetClaimCacheUsb = 5
	fso.RetGetNodeAvailableCacheBytes = 2000
	fso.RetUpdateNodeAvailableCacheErr = fmt.Errorf("Failure to update Node")
	fc.RetNErr = nil
	fc.RetNObj = nObj
	assert.NotPanics(func() { op.allocateCache(ctx) })
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	assert.EqualValues(v.Meta.ID, fso.InClaimCacheVsID)
	assert.EqualValues(v.Meta.ID, fc.InVSUpdaterID)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Failure to update Node", vr.RequestMessages[0].Message)
	fso.RetUpdateNodeAvailableCacheErr = nil // reset

	tl.Logger().Infof("case: allocateCache (failure to update VolumeSeries's object cache allocation)")
	nvAPI.EXPECT().AllocCache(nuvoVolIdentifier, uint64(5)).Return(nil)
	c.App.NuvoAPI = nvAPI
	vr.RequestMessages = nil
	rhs.InError = false
	rhs.RetryLater = false
	fc.RetNodeUpdateErr = nil
	fc.RetVSUpdaterErr = fmt.Errorf("Failure to update VolumeSeries")
	assert.NotPanics(func() { op.allocateCache(ctx) })
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	assert.EqualValues(v.Meta.ID, fso.InClaimCacheVsID)
	assert.EqualValues(v.Meta.ID, fc.InVSUpdaterID)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Failed to update cache allocations for VolumeSeries object.*", vr.RequestMessages[0].Message)

	tl.Logger().Infof("case: allocateCache (success)")
	nvAPI.EXPECT().AllocCache(nuvoVolIdentifier, uint64(5)).Return(nil)
	c.App.NuvoAPI = nvAPI
	vr.RequestMessages = nil
	rhs.RetryLater = false
	rhs.InError = false
	fao.Node = nObj
	fc.RetVSUpdaterErr = nil
	assert.NotPanics(func() { op.allocateCache(ctx) })
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.EqualValues(v.Meta.ID, fc.InVSUpdaterID)
	assert.EqualValues(v.Meta.ID, fso.InClaimCacheVsID)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Allocated cache .requested=10B allocated=5B.", vr.RequestMessages[0].Message)

	tl.Logger().Infof("case: allocateCache to be skipped as already done")
	v.CacheAllocations = map[string]models.CacheAllocation{
		"node1": {RequestedSizeBytes: swag.Int64(1111)},
	}
	vr.RequestMessages = nil
	rhs.RetryLater = false
	rhs.InError = false
	assert.NotPanics(func() { op.allocateCache(ctx) })
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Cache allocation is already complete", vr.RequestMessages[0].Message)
	mockCtrl.Finish()

	// ***************************** exportLun
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.App.NuvoAPI = nvAPI
	rhs.RetryLater = false
	rhs.InError = false
	vr.RequestMessages = nil
	nvAPI.EXPECT().ExportLun(nuvoVolIdentifier, "", devName, true).Return(fmt.Errorf("export-error"))
	assert.NotPanics(func() { op.exportLun(ctx) })
	assert.True(rhs.InError)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("ExportLun error: export-error", vr.RequestMessages[0].Message)

	rhs.InError = false
	vr.RequestMessages = nil
	c.rei.SetProperty("export-fail-in-export", &rei.Property{BoolValue: true})
	assert.NotPanics(func() { op.exportLun(ctx) })
	assert.True(rhs.InError)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("injecting error", op.rhs.Request.RequestMessages[0])

	rhs.InError = false
	op.pitUUID = "thePits"
	op.isWritable = false
	nvAPI.EXPECT().ExportLun(nuvoVolIdentifier, "thePits", devName, false).Return(nil)
	fas.InAddLUNObj = nil
	assert.NotPanics(func() { op.exportLun(ctx) })
	assert.False(rhs.InError)
	assert.Len(vr.RequestMessages, 2)
	assert.Regexp("Exported .*=HEAD", vr.RequestMessages[1].Message)
	if assert.NotNil(fas.InAddLUNObj) {
		lun := fas.InAddLUNObj
		assert.NoError(lun.Validate())
		assert.EqualValues(10, lun.RequestedCacheSizeBytes)
		assert.EqualValues(5, lun.AllocatedCacheSizeBytes)
		assert.Equal(nuvoVolIdentifier, lun.NuvoVolumeIdentifier)
	}
	op.pitUUID = ""      // reset
	op.isWritable = true // reset

	rhs.InError = false
	nvAPI.EXPECT().ExportLun(nuvoVolIdentifier, "", devName, true).Return(fmt.Errorf("%s: more stuff", errLunAlreadyExported))
	fas.InAddLUNObj = nil
	assert.NotPanics(func() { op.exportLun(ctx) })
	assert.False(rhs.InError)
	assert.Len(vr.RequestMessages, 3)
	assert.Regexp("Exported .*=HEAD", vr.RequestMessages[2].Message)
	if assert.NotNil(fas.InAddLUNObj) {
		lun := fas.InAddLUNObj
		assert.NoError(lun.Validate())
		assert.EqualValues(10, lun.RequestedCacheSizeBytes)
		assert.EqualValues(5, lun.AllocatedCacheSizeBytes)
		assert.Equal(nuvoVolIdentifier, lun.NuvoVolumeIdentifier)
	}
	mockCtrl.Finish()

	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.App.NuvoAPI = nvAPI
	rhs.RetryLater = false
	rhs.InError = false
	nErr := nuvoapi.NewNuvoAPIError("tempErr", true)
	nvAPI.EXPECT().ExportLun(nuvoVolIdentifier, "", devName, true).Return(nErr)
	fas.InAddLUNObj = nil
	assert.NotPanics(func() { op.exportLun(ctx) })
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 4)
	assert.Regexp("tempErr", vr.RequestMessages[3].Message)
	assert.Nil(fas.InAddLUNObj)

	// export of non HEAD LUN doesn't record cache related stats
	rhs.InError = false
	op.snapID = "NON_HEAD"
	expMount.SnapIdentifier = "NON_HEAD"
	op.pitUUID = "theOtherPits"
	op.isWritable = false
	nvAPI.EXPECT().ExportLun(nuvoVolIdentifier, "theOtherPits", devName, false).Return(nil)
	fas.InAddLUNObj = nil
	assert.NotPanics(func() { op.exportLun(ctx) })
	assert.False(rhs.InError)
	assert.Len(vr.RequestMessages, 5)
	assert.Regexp("Exported .*=NON_HEAD", vr.RequestMessages[4].Message)
	if assert.NotNil(fas.InAddLUNObj) {
		lun := fas.InAddLUNObj
		assert.NoError(lun.Validate())
		assert.Zero(lun.RequestedCacheSizeBytes)
		assert.Zero(lun.AllocatedCacheSizeBytes)
		assert.Equal(nuvoVolIdentifier, lun.NuvoVolumeIdentifier)
	}
	op.pitUUID = ""                                      // reset
	op.isWritable = true                                 // reset
	op.snapID = com.VolMountHeadIdentifier               // reset
	expMount.SnapIdentifier = com.VolMountHeadIdentifier // reset

	// ***************************** unexportLun
	rhs.InError = false
	vr.RequestMessages = nil
	nvAPI.EXPECT().UnexportLun(nuvoVolIdentifier, "", devName).Return(fmt.Errorf("export-error"))
	assert.NotPanics(func() { op.unExportLun(ctx) })
	assert.True(rhs.InError)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("UnexportLun error: export-error", vr.RequestMessages[0].Message)

	rhs.InError = false
	op.pitUUID = "thePits"
	nvAPI.EXPECT().UnexportLun(nuvoVolIdentifier, "thePits", devName).Return(nil)
	fas.InRemoveLUNvsID, fas.InRemoveLUNvsIDSnapID = "", ""
	assert.NotPanics(func() { op.unExportLun(ctx) })
	assert.False(rhs.InError)
	assert.Len(vr.RequestMessages, 2)
	assert.Regexp("Stopped export .*=HEAD", vr.RequestMessages[1].Message)
	assert.EqualValues(v.Meta.ID, fas.InRemoveLUNvsID)
	assert.EqualValues("HEAD", fas.InRemoveLUNvsIDSnapID)
	op.pitUUID = "" // reset

	rhs.InError = false
	nvAPI.EXPECT().UnexportLun(nuvoVolIdentifier, "", devName).Return(fmt.Errorf("%s: more stuff", errLunNotExported))
	fas.InRemoveLUNvsID, fas.InRemoveLUNvsIDSnapID = "", ""
	assert.NotPanics(func() { op.unExportLun(ctx) })
	assert.False(rhs.InError)
	assert.Len(vr.RequestMessages, 3)
	assert.Regexp("Stopped export .*=HEAD", vr.RequestMessages[2].Message)
	assert.EqualValues(v.Meta.ID, fas.InRemoveLUNvsID)
	assert.EqualValues(expMount.SnapIdentifier, fas.InRemoveLUNvsIDSnapID)

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.App.NuvoAPI = nvAPI
	rhs.RetryLater = false
	rhs.InError = false
	nErr = nuvoapi.NewNuvoAPIError("tempErr", true)
	nvAPI.EXPECT().UnexportLun(nuvoVolIdentifier, "", devName).Return(nErr)
	fas.InRemoveLUNvsID, fas.InRemoveLUNvsIDSnapID = "", ""
	assert.NotPanics(func() { op.unExportLun(ctx) })
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 4)
	assert.Regexp("tempErr", vr.RequestMessages[3].Message)
	assert.Empty(fas.InRemoveLUNvsID)
	assert.Empty(fas.InRemoveLUNvsIDSnapID)
	tl.Flush()

	// ***************************** releaseCache
	releaseCacheReset := func() {
		v.CacheAllocations = map[string]models.CacheAllocation{ // reset needed as VSR updates VS object
			"node1": {AllocatedSizeBytes: swag.Int64(1111), RequestedSizeBytes: swag.Int64(1111)},
			"node2": {AllocatedSizeBytes: swag.Int64(22), RequestedSizeBytes: swag.Int64(1111)},
			"node3": {AllocatedSizeBytes: swag.Int64(333), RequestedSizeBytes: swag.Int64(1111)},
		}
		vr.RequestMessages = nil
		rhs.RetryLater = false
		rhs.InError = false
		fc.RetNObj = nObj
		fc.RetNErr = nil
		fc.RetVSUpdaterErr = nil
		fc.RetNodeUpdateErr = nil
	}

	vs.ConfiguredNodeID = "node1"
	vs.CacheAllocations = map[string]models.CacheAllocation{
		"node1": {AllocatedSizeBytes: swag.Int64(1111), RequestedSizeBytes: swag.Int64(1111)},
	}

	tl.Logger().Infof("case: releaseCache (failure to get lock)")
	vsr.RequestMessages = []*models.TimestampedString{}
	app.LMDGuard.Drain() // force closure
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	assert.NotPanics(func() { op.releaseCache(ctx) })
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("state lock error", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Infof("case: releaseCache (successful locking)")
	app.LMDGuard = util.NewCriticalSectionGuard()
	vsr.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().AllocCache(nuvoVolIdentifier, uint64(0)).Return(nil)
	c.App.NuvoAPI = nvAPI
	assert.NotPanics(func() { op.releaseCache(ctx) })
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal([]string{}, app.LMDGuard.Status()) // lock released
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Equal("Released cache", op.rhs.Request.RequestMessages[0].Message)

	// reset
	op.rhs.Request = vr
	op.rhs.VolumeSeries = v

	tl.Logger().Infof("case: releaseCache (no updates needed)")
	v.CacheAllocations = map[string]models.CacheAllocation{
		"node2": {AllocatedSizeBytes: swag.Int64(0)},
	}
	vr.RequestMessages = nil
	rhs.InError = false
	rhs.RetryLater = false
	assert.NotPanics(func() { op.releaseCache(ctx) })
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("VolumeSeries.* is not using any cache on node node1, no cache state or node updates are needed", vr.RequestMessages[0].Message)

	tl.Logger().Infof("case: releaseCache (failure to update VolumeSeries's object cache allocation)")
	releaseCacheReset()
	fc.RetVSUpdaterErr = fmt.Errorf("Failure to update VolumeSeries")
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().AllocCache(nuvoVolIdentifier, uint64(0)).Return(nil)
	c.App.NuvoAPI = nvAPI
	assert.NotPanics(func() { op.releaseCache(ctx) })
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	assert.EqualValues(v.Meta.ID, fc.InVSUpdaterID)
	assert.Equal(v.CacheAllocations["node1"].AllocatedSizeBytes, swag.Int64(1111))
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Failed to update released cache allocations for VolumeSeries object.*: Failure to update VolumeSeries", vr.RequestMessages[0].Message)

	tl.Logger().Infof("case: releaseCache (failure to update Node's object allocated cache)")
	releaseCacheReset()
	nObj.AvailableCacheBytes = swag.Int64(3000)
	fso.RetUpdateNodeAvailableCacheErr = fmt.Errorf("Failure to update Node")
	fso.RetGetNodeAvailableCacheBytes = 2000
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().AllocCache(nuvoVolIdentifier, uint64(0)).Return(nil)
	c.App.NuvoAPI = nvAPI
	assert.NotPanics(func() { op.releaseCache(ctx) })
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	assert.EqualValues(v.Meta.ID, fc.InVSUpdaterID)
	assert.Equal(v.CacheAllocations["node1"].AllocatedSizeBytes, swag.Int64(1111)) // VS object not updated
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Failure to update Node", vr.RequestMessages[0].Message)
	fso.RetUpdateNodeAvailableCacheErr = nil // reset

	tl.Logger().Infof("case: releaseCache (NUVOAPI error)")
	releaseCacheReset()
	nObj.AvailableCacheBytes = swag.Int64(3000)
	fso.RetGetNodeAvailableCacheBytes = 2000
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().AllocCache(nuvoVolIdentifier, uint64(0)).Return(fmt.Errorf("allocate-cache--error"))
	c.App.NuvoAPI = nvAPI
	assert.NotPanics(func() { op.releaseCache(ctx) })
	assert.True(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.EqualValues(v.Meta.ID, fso.InClaimCacheVsID)
	assert.EqualValues(2, fso.InClaimCacheMsb)
	assert.EqualValues(v.Meta.ID, fc.InVSUpdaterID)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("NUVOAPI AllocCache error: .*", vr.RequestMessages[0].Message)

	tl.Logger().Infof("case: releaseCache (NUVOAPI temp error)")
	releaseCacheReset()
	nObj.AvailableCacheBytes = swag.Int64(3000)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().AllocCache(nuvoVolIdentifier, uint64(0)).Return(nuvoapi.NewNuvoAPIError("tempErr", true))
	c.App.NuvoAPI = nvAPI
	assert.NotPanics(func() { op.releaseCache(ctx) })
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	assert.EqualValues(v.Meta.ID, fso.InClaimCacheVsID)
	assert.EqualValues(2, fso.InClaimCacheMsb)
	assert.EqualValues(v.Meta.ID, fc.InVSUpdaterID)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("NUVOAPI temporary error: .*", vr.RequestMessages[0].Message)

	tl.Logger().Infof("case: releaseCache (success)")
	releaseCacheReset()
	fso.InReleaseCacheVsID = ""
	nObj.AvailableCacheBytes = swag.Int64(3000)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().AllocCache(nuvoVolIdentifier, uint64(0)).Return(nil)
	c.App.NuvoAPI = nvAPI
	assert.NotPanics(func() { op.releaseCache(ctx) })
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.EqualValues(v.Meta.ID, fc.InVSUpdaterID)
	assert.Empty(fc.InVSUpdaterItems.Append)
	assert.Equal([]string{"cacheAllocations"}, fc.InVSUpdaterItems.Remove)
	assert.Empty(fc.InVSUpdaterItems.Set)
	assert.Len(v.CacheAllocations, 1)
	assert.Nil(v.CacheAllocations["node1"].AllocatedSizeBytes) // VS object updated
	assert.EqualValues(v.Meta.ID, fso.InReleaseCacheVsID)
	assert.Len(vr.RequestMessages, 1)
	assert.Equal("Released cache", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Infof("case: releaseCache (no cache release is needed for non HEAD)")
	releaseCacheReset()
	op.snapID = "some_snap_id"
	fc.InVSUpdaterID = ""
	assert.NotPanics(func() { op.releaseCache(ctx) })
	assert.Equal("", fc.InVSUpdaterID)
	assert.Equal(1, tl.CountPattern("No cache release is needed for non HEAD"))
	op.snapID = com.VolMountHeadIdentifier // reset
	tl.Flush()

	// ***************************** findMounts
	m1 := &models.Mount{MountState: "MOUNTING", SnapIdentifier: "2"}
	m2 := &models.Mount{MountState: "MOUNTING", SnapIdentifier: "HEAD"}
	v.Mounts = []*models.Mount{m1, m2}
	i, m := op.findMount(v, "2")
	assert.Equal(0, i)
	assert.Equal(m1, m)
	i, m = op.findMount(v, "HEAD")
	assert.Equal(1, i)
	assert.Equal(m2, m)
	i, m = op.findMount(v, "3")
	assert.Equal(0, i)
	assert.Nil(m)

	// ***************************** unConfigureLun
	vrC = vsrClone(vr)
	vrC.VolumeSeriesRequestState = "VOLUME_EXPORT"
	op.rhs.Request = vrC

	tl.Logger().Info("case unConfigure: ok")
	fmReset()
	op.unConfigureLun(ctx)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.NotNil(fm.ucVSR)
	assert.Equal("UNDO_VOLUME_CONFIG", fm.ucVSR.VolumeSeriesRequestState)
	assert.Equal("VOLUME_EXPORT", vrC.VolumeSeriesRequestState)

	tl.Logger().Info("case unConfigure: error")
	fmReset()
	fm.retUCInError = true
	op.unConfigureLun(ctx)
	assert.True(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.NotNil(fm.ucVSR)
	assert.Equal("UNDO_VOLUME_CONFIG", fm.ucVSR.VolumeSeriesRequestState)
	assert.Equal("VOLUME_EXPORT", vrC.VolumeSeriesRequestState)

	// ***************************** nuvoGetWriteStat
	vrC = vsrClone(vr)
	vrC.VolumeSeriesRequestState = "VOLUME_EXPORT"
	op.rhs.Request = vrC
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.writeStats = nil

	tl.Logger().Infof("case: nuvoGetWriteStat")
	tl.Flush()
	writeStats := &nuvoapi.StatsIO{
		SizeTotal:  1000,
		SeriesUUID: "UUID",
	}
	userStats := &nuvoapi.StatsCache{}
	metaStats := &nuvoapi.StatsCache{}
	comboStats := &nuvoapi.StatsCombinedVolume{
		IOWrites:      *writeStats,
		CacheUser:     *userStats,
		CacheMetadata: *metaStats,
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().GetVolumeStats(false, nuvoVolIdentifier).Return(comboStats, nil)
	c.App.NuvoAPI = nvAPI
	op.nuvoGetWriteStat(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(writeStats, op.writeStats)

	tl.Logger().Infof("case: nuvoGetWriteStat (error)")
	tl.Flush()
	op.writeStats = nil
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().GetVolumeStats(false, nuvoVolIdentifier).Return(nil, fmt.Errorf("get-stats"))
	c.App.NuvoAPI = nvAPI
	op.nuvoGetWriteStat(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Nil(op.writeStats)

	tl.Logger().Infof("case: nuvoGetWriteStat not HEAD")
	tl.Flush()
	op.writeStats = nil
	op.snapID = "foo"
	c.App.NuvoAPI = nil
	op.nuvoGetWriteStat(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Nil(op.writeStats)

	// ***************************** reportMetrics
	mr := &fmr.MetricReporter{}
	app.MetricReporter = mr

	vrC = vsrClone(vr)
	vrC.VolumeSeriesRequestState = "VOLUME_UNEXPORT"
	op.rhs.Request = vrC
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.snapID = com.VolMountHeadIdentifier
	op.reportMetrics(ctx)
	assert.EqualValues(op.rhs.VolumeSeries.Meta.ID, mr.InRVMid)
	tl.Flush()

	// ***************************** vsrSaveVSProps
	vsr = vsrClone(vr)
	vs = vsClone(v)
	vsr.NodeID = "node1"
	vs.CacheAllocations = map[string]models.CacheAllocation{
		"node1": {AllocatedSizeBytes: swag.Int64(1111), RequestedSizeBytes: swag.Int64(2222)},
	}
	expTag := fmt.Sprintf("%s:a=%d,r=%d", com.SystemTagVsrCacheAllocation, 1111, 2222)
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs

	t.Log("case: vsrSaveVSProps not yet saved")
	fc.InVSRUpdaterItems = nil
	fc.ModVSRUpdaterObj = nil
	fc.RetVSRUpdaterErr = nil
	op.vsrSaveVSProps(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(fc.InVSRUpdaterItems)
	assert.Equal([]string{"systemTags"}, fc.InVSRUpdaterItems.Set)
	assert.Empty(fc.InVSRUpdaterItems.Append)
	assert.Empty(fc.InVSRUpdaterItems.Remove)
	assert.EqualValues([]string{expTag}, fc.ModVSRUpdaterObj.SystemTags)
	assert.Equal(fc.ModVSRUpdaterObj, op.rhs.Request)

	t.Log("case: vsrSaveVSProps already saved")
	fc.InVSRUpdaterItems = nil
	fc.ModVSRUpdaterObj = nil
	fc.RetVSRUpdaterErr = fmt.Errorf("should-not-fail")
	op.rhs.Request.SystemTags = []string{expTag}
	op.vsrSaveVSProps(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Nil(fc.InVSRUpdaterItems)
	tl.Flush()

	// ***************************** updateCache
	t.Log("case: updateCache (failure to get lock)")
	app.LMDGuard.Drain() // force closure
	vr.RequestMessages = nil
	vsr = vsrClone(vr)
	vs = vsClone(v)
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	assert.NotPanics(func() { op.updateCache(ctx) })
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("state lock error", op.rhs.Request.RequestMessages[0].Message)
	tl.Flush()

	t.Log("case: updateCache (successful lock, LUN not found)")
	app.LMDGuard = util.NewCriticalSectionGuard()
	vsr = vsrClone(vr)
	op.rhs.Request = vsr
	op.rhs.RetryLater = false
	op.rhs.InError = false
	fas.RetFLObj = nil
	assert.NotPanics(func() { op.updateCache(ctx) })
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal([]string{}, app.LMDGuard.Status()) // lock released
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("HEAD LUN not found", op.rhs.Request.RequestMessages[0].Message)
	tl.Flush()

	t.Log("case: updateCache (error injection)")
	vsr = vsrClone(vr)
	op.rhs.Request = vsr
	op.rhs.InError = false
	c.rei.SetProperty("fail-in-update-cache", &rei.Property{BoolValue: true})
	fas.RetFLObj = &agentd.LUN{}
	assert.NotPanics(func() { op.updateCache(ctx) })
	assert.True(op.rhs.InError)
	assert.Len(vsr.RequestMessages, 1)
	assert.Regexp("injecting error", op.rhs.Request.RequestMessages[0])
	tl.Flush()

	t.Log("case: updateCache (getCacheRequired failure)") // fully tested elsewhere
	vsr = vsrClone(vr)
	vsr.StoragePlan.StorageElements = sE3
	op.rhs.Request = vsr
	op.rhs.InError = false
	fas.RetFLObj = &agentd.LUN{}
	assert.NotPanics(func() { op.updateCache(ctx) })
	assert.True(op.rhs.InError)
	assert.Len(vsr.RequestMessages, 1)
	assert.Regexp("Mismatched cache types", op.rhs.Request.RequestMessages[0])
	tl.Flush()

	t.Log("case: updateCache (ClaimCache failure)") // fully tested elsewhere
	vsr = vsrClone(vr)
	vsr.StoragePlan.StorageElements = sE4
	op.rhs.Request = vsr
	rhs.RetryLater = false
	rhs.InError = false
	fao.Node = nObj
	fso.InClaimCacheVsID = ""
	fso.InClaimCacheMsb, fso.InClaimCacheDsb = 0, 0
	fso.RetGetClaimSizeBytes = 5
	fso.RetClaimCacheErr = fmt.Errorf("Failure to update cache state")
	assert.NotPanics(func() { op.updateCache(ctx) })
	assert.True(rhs.InError)
	assert.EqualValues(v.Meta.ID, fso.InClaimCacheVsID)
	assert.EqualValues(2, fso.InClaimCacheMsb)
	assert.EqualValues(10, fso.InClaimCacheDsb)
	assert.Len(vsr.RequestMessages, 1)
	assert.Regexp("Failure to update node cache state: Failure to update cache state", vsr.RequestMessages[0].Message)
	tl.Flush()

	t.Log("case: updateCache cache increase (NuvoAPI temporary failure)")
	vsr = vsrClone(vr)
	vsr.StoragePlan.StorageElements = sE4
	op.rhs.Request = vsr
	rhs.RetryLater = false
	rhs.InError = false
	fao.Node = nObj
	fso.RetGetClaimSizeBytes = 4
	fso.InClaimCacheVsID = ""
	fso.InClaimCacheMsb, fso.InClaimCacheDsb = 0, 0
	fso.RetClaimCacheUsb = 12
	fso.RetClaimCacheErr = nil
	fso.RetGNCAllocationUnitSizeBytes = 4
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().AllocCache(nuvoVolIdentifier, uint64(12)).Return(nuvoapi.NewNuvoAPIError("tempErr", true))
	c.App.NuvoAPI = nvAPI
	assert.NotPanics(func() { op.updateCache(ctx) })
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	assert.EqualValues(v.Meta.ID, fso.InClaimCacheVsID)
	assert.EqualValues(4, fso.InClaimCacheMsb) // releasing new claim
	assert.EqualValues(4, fso.InClaimCacheDsb)
	assert.Len(vsr.RequestMessages, 1)
	assert.Regexp("NUVOAPI temporary error.*tempErr", vsr.RequestMessages[0].Message)

	t.Log("case: updateCache cache decrease (NuvoAPI failure)")
	vsr = vsrClone(vr)
	vsr.StoragePlan.StorageElements = sE4
	op.rhs.Request = vsr
	rhs.RetryLater = false
	rhs.InError = false
	fao.Node = nObj
	fso.RetGetClaimSizeBytes = 16
	fso.InClaimCacheVsID = ""
	fso.InClaimCacheMsb, fso.InClaimCacheDsb = 0, 0
	fso.RetClaimCacheErr = nil
	fso.RetGNCAllocationUnitSizeBytes = 4
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().AllocCache(nuvoVolIdentifier, uint64(12)).Return(nuvoapi.NewNuvoAPIError("failure", false))
	c.App.NuvoAPI = nvAPI
	assert.NotPanics(func() { op.updateCache(ctx) })
	assert.True(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Empty(fso.InClaimCacheVsID) // not called
	assert.Len(vsr.RequestMessages, 1)
	assert.Regexp("NUVOAPI AllocCache error.*failure", vsr.RequestMessages[0].Message)

	t.Log("case: updateCache cache decrease (UpdateNode failure)")
	vsr = vsrClone(vr)
	vsr.StoragePlan.StorageElements = sE4
	op.rhs.Request = vsr
	rhs.RetryLater = false
	rhs.InError = false
	fao.Node = nObj
	fso.RetGetClaimSizeBytes = 16
	fso.InClaimCacheVsID = ""
	fso.InClaimCacheMsb, fso.InClaimCacheDsb = 0, 0
	fso.RetClaimCacheErr = nil
	fso.RetGNCAllocationUnitSizeBytes = 4
	fso.RetUpdateNodeAvailableCacheErr = fmt.Errorf("nodeFailure")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().AllocCache(nuvoVolIdentifier, uint64(12)).Return(nil)
	c.App.NuvoAPI = nvAPI
	assert.NotPanics(func() { op.updateCache(ctx) })
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	assert.EqualValues(v.Meta.ID, fso.InClaimCacheVsID)
	assert.EqualValues(12, fso.InClaimCacheDsb)
	assert.EqualValues(12, fso.InClaimCacheMsb)
	assert.Len(vsr.RequestMessages, 1)
	assert.Regexp("nodeFailure", vsr.RequestMessages[0].Message)

	t.Log("case: updateCache cache increase (VolumeSeriesUpdater failure)")
	vsr = vsrClone(vr)
	vsr.StoragePlan.StorageElements = sE4
	op.rhs.Request = vsr
	rhs.RetryLater = false
	rhs.InError = false
	fao.Node = nObj
	fso.RetGetClaimSizeBytes = 4
	fso.InClaimCacheVsID = ""
	fso.InClaimCacheMsb, fso.InClaimCacheDsb = 0, 0
	fso.RetClaimCacheErr = nil
	fso.RetGNCAllocationUnitSizeBytes = 4
	fso.RetUpdateNodeAvailableCacheErr = nil
	fc.RetVSUpdaterErr = fmt.Errorf("updaterError")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().AllocCache(nuvoVolIdentifier, uint64(12)).Return(nil)
	c.App.NuvoAPI = nvAPI
	assert.NotPanics(func() { op.updateCache(ctx) })
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	assert.EqualValues(v.Meta.ID, fso.InClaimCacheVsID)
	assert.EqualValues(2, fso.InClaimCacheMsb)
	assert.EqualValues(10, fso.InClaimCacheDsb)
	assert.Len(vsr.RequestMessages, 1)
	assert.Regexp("updaterError", vsr.RequestMessages[0].Message)

	t.Log("case: updateCache cache success")
	vsr = vsrClone(vr)
	vsr.StoragePlan.StorageElements = sE4
	op.rhs.Request = vsr
	rhs.RetryLater = false
	rhs.InError = false
	fao.Node = nObj
	fas.RetFLObj = &agentd.LUN{}
	fso.RetGetClaimSizeBytes = 4
	fso.InClaimCacheVsID = ""
	fso.InClaimCacheMsb, fso.InClaimCacheDsb = 0, 0
	fso.RetClaimCacheErr = nil
	fso.RetGNCAllocationUnitSizeBytes = 4
	fso.RetUpdateNodeAvailableCacheErr = nil
	fc.InVSUpdaterID = ""
	fc.InVSUpdaterItems, fc.RetVSUpdaterErr = nil, nil
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().AllocCache(nuvoVolIdentifier, uint64(12)).Return(nil)
	c.App.NuvoAPI = nvAPI
	assert.NotPanics(func() { op.updateCache(ctx) })
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.EqualValues(v.Meta.ID, fso.InClaimCacheVsID)
	assert.EqualValues(2, fso.InClaimCacheMsb)
	assert.EqualValues(10, fso.InClaimCacheDsb)
	assert.EqualValues(v.Meta.ID, fc.InVSUpdaterID)
	assert.Equal([]string{"cacheAllocations"}, fc.InVSUpdaterItems.Append)
	expCA = map[string]models.CacheAllocation{
		"node1": {AllocatedSizeBytes: swag.Int64(12), RequestedSizeBytes: swag.Int64(10)},
	}
	assert.Equal(expCA, vs.CacheAllocations)
	assert.Nil(fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Remove)
	assert.EqualValues(12, fas.RetFLObj.AllocatedCacheSizeBytes)
	assert.EqualValues(10, fas.RetFLObj.RequestedCacheSizeBytes)
	assert.Len(vsr.RequestMessages, 1)
	assert.Regexp("Updated cache allocation old:.requested=1111B allocated=1111B. new:.requested=10B allocated=12B", vsr.RequestMessages[0].Message)
	tl.Flush()

	// ***************************** restoreCache
	t.Log("case: restoreCache (no systemTag)")
	vr.RequestMessages = nil
	vsr = vsrClone(vr)
	vs = vsClone(v)
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	rhs.RetryLater = false
	rhs.InError = false
	assert.NotPanics(func() { op.restoreCache(ctx) })
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Cache was not updated", op.rhs.Request.RequestMessages[0].Message)
	tl.Flush()

	t.Log("case: restoreCache (invalid systemTag)")
	vsr = vsrClone(vr)
	op.rhs.Request = vsr
	vsr.SystemTags = []string{fmt.Sprintf("%s:a=huh", com.SystemTagVsrCacheAllocation)}
	rhs.RetryLater = false
	rhs.InError = false
	assert.NotPanics(func() { op.restoreCache(ctx) })
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Cache update tag is invalid, skipping", op.rhs.Request.RequestMessages[0].Message)
	tl.Flush()

	t.Log("case: restoreCache (failure to get lock)")
	app.LMDGuard.Drain() // force closure
	vr.SystemTags = []string{fmt.Sprintf("%s:a=30,r=50", com.SystemTagVsrCacheAllocation)}
	vr.RequestMessages = nil
	vsr = vsrClone(vr)
	vs = vsClone(v)
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	rhs.RetryLater = false
	rhs.InError = false
	assert.NotPanics(func() { op.restoreCache(ctx) })
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("state lock error", op.rhs.Request.RequestMessages[0].Message)
	tl.Flush()

	t.Log("case: restoreCache increase (successful lock, failure to increase claim)")
	app.LMDGuard = util.NewCriticalSectionGuard()
	vsr = vsrClone(vr)
	op.rhs.Request = vsr
	op.rhs.RetryLater = false
	op.rhs.InError = false
	op.rhs.AbortUndo = false
	fso.InClaimCacheVsID = ""
	fso.InClaimCacheMsb, fso.InClaimCacheDsb = 0, 0
	fso.RetGetClaimSizeBytes = 5
	fso.RetClaimCacheErr = fmt.Errorf("Failure to update cache state")
	assert.NotPanics(func() { op.restoreCache(ctx) })
	assert.True(op.rhs.AbortUndo)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal([]string{}, app.LMDGuard.Status()) // lock released
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Restore Cache aborted: Failure to update cache state", op.rhs.Request.RequestMessages[0].Message)
	tl.Flush()

	t.Log("case: restoreCache increase (NUVOAPI temporary error)")
	app.LMDGuard = util.NewCriticalSectionGuard()
	vsr = vsrClone(vr)
	op.rhs.Request = vsr
	op.rhs.RetryLater = false
	op.rhs.InError = false
	op.rhs.AbortUndo = false
	fso.InClaimCacheVsID = ""
	fso.InClaimCacheMsb, fso.InClaimCacheDsb = 0, 0
	fso.RetGetClaimSizeBytes = 5
	fso.RetClaimCacheErr = nil
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().AllocCache(nuvoVolIdentifier, uint64(30)).Return(nuvoapi.NewNuvoAPIError("tempErr", true))
	c.App.NuvoAPI = nvAPI
	assert.NotPanics(func() { op.restoreCache(ctx) })
	assert.False(op.rhs.AbortUndo)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.EqualValues(v.Meta.ID, fso.InClaimCacheVsID)
	assert.EqualValues(5, fso.InClaimCacheMsb) // releasing new claim
	assert.EqualValues(5, fso.InClaimCacheDsb)
	assert.Len(vsr.RequestMessages, 1)
	assert.Regexp("NUVOAPI temporary error.*tempErr", vsr.RequestMessages[0].Message)
	tl.Flush()

	t.Log("case: restoreCache decrease (NUVOAPI error)")
	vsr = vsrClone(vr)
	op.rhs.Request = vsr
	op.rhs.RetryLater = false
	op.rhs.InError = false
	op.rhs.AbortUndo = false
	fso.InClaimCacheVsID = ""
	fso.InClaimCacheMsb, fso.InClaimCacheDsb = 0, 0
	fso.RetGetClaimSizeBytes = 60
	fso.RetClaimCacheErr = nil
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().AllocCache(nuvoVolIdentifier, uint64(30)).Return(nuvoapi.NewNuvoAPIError("failure", false))
	c.App.NuvoAPI = nvAPI
	assert.NotPanics(func() { op.restoreCache(ctx) })
	assert.True(op.rhs.AbortUndo)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Empty(fso.InClaimCacheVsID) // not called
	assert.Len(vsr.RequestMessages, 1)
	assert.Regexp("NUVOAPI AllocCache error, aborted.*failure", vsr.RequestMessages[0].Message)
	tl.Flush()

	t.Log("case: restoreCache decrease (UpdateNode failure)")
	vsr = vsrClone(vr)
	op.rhs.Request = vsr
	op.rhs.RetryLater = false
	op.rhs.InError = false
	op.rhs.AbortUndo = false
	fso.InClaimCacheVsID = ""
	fso.InClaimCacheMsb, fso.InClaimCacheDsb = 0, 0
	fso.RetGetClaimSizeBytes = 60
	fso.RetClaimCacheErr = nil
	fso.RetUpdateNodeAvailableCacheErr = fmt.Errorf("nodeFailure")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().AllocCache(nuvoVolIdentifier, uint64(30)).Return(nil)
	c.App.NuvoAPI = nvAPI
	assert.NotPanics(func() { op.restoreCache(ctx) })
	assert.False(op.rhs.AbortUndo)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.EqualValues(v.Meta.ID, fso.InClaimCacheVsID)
	assert.EqualValues(30, fso.InClaimCacheMsb)
	assert.EqualValues(30, fso.InClaimCacheDsb)
	assert.Len(vsr.RequestMessages, 1)
	assert.Regexp("nodeFailure", vsr.RequestMessages[0].Message)
	tl.Flush()

	t.Log("case: restoreCache increase (VolumeSeriesUpdater failure)")
	vsr = vsrClone(vr)
	op.rhs.Request = vsr
	op.rhs.RetryLater = false
	op.rhs.InError = false
	op.rhs.AbortUndo = false
	fso.InClaimCacheVsID = ""
	fso.InClaimCacheMsb, fso.InClaimCacheDsb = 0, 0
	fso.RetGetClaimSizeBytes = 5
	fso.RetClaimCacheErr = nil
	fso.RetUpdateNodeAvailableCacheErr = nil
	fc.RetVSUpdaterErr = fmt.Errorf("updaterError")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().AllocCache(nuvoVolIdentifier, uint64(30)).Return(nil)
	c.App.NuvoAPI = nvAPI
	assert.NotPanics(func() { op.restoreCache(ctx) })
	assert.False(op.rhs.AbortUndo)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.EqualValues(v.Meta.ID, fso.InClaimCacheVsID)
	assert.EqualValues(30, fso.InClaimCacheMsb)
	assert.EqualValues(30, fso.InClaimCacheDsb)
	assert.Len(vsr.RequestMessages, 1)
	assert.Regexp("Failed to update restored cache allocations: updaterError", vsr.RequestMessages[0].Message)
	tl.Flush()

	t.Log("case: restoreCache increase success (FindLUN nil)")
	vsr = vsrClone(vr)
	op.rhs.Request = vsr
	op.rhs.RetryLater = false
	op.rhs.InError = false
	op.rhs.AbortUndo = false
	fas.RetFLObj = nil
	fso.InClaimCacheVsID = ""
	fso.InClaimCacheMsb, fso.InClaimCacheDsb = 0, 0
	fso.RetGetClaimSizeBytes = 5
	fso.RetClaimCacheErr = nil
	fso.RetUpdateNodeAvailableCacheErr = nil
	fc.InVSUpdaterID = ""
	fc.InVSUpdaterItems, fc.RetVSUpdaterErr = nil, nil
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().AllocCache(nuvoVolIdentifier, uint64(30)).Return(nil)
	c.App.NuvoAPI = nvAPI
	assert.NotPanics(func() { op.restoreCache(ctx) })
	assert.False(op.rhs.AbortUndo)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.EqualValues(v.Meta.ID, fso.InClaimCacheVsID)
	assert.EqualValues(30, fso.InClaimCacheMsb)
	assert.EqualValues(30, fso.InClaimCacheDsb)
	assert.EqualValues(v.Meta.ID, fc.InVSUpdaterID)
	assert.Equal([]string{"cacheAllocations"}, fc.InVSUpdaterItems.Append)
	expCA = map[string]models.CacheAllocation{
		"node1": {AllocatedSizeBytes: swag.Int64(30), RequestedSizeBytes: swag.Int64(50)},
	}
	assert.Equal(expCA, vs.CacheAllocations)
	assert.Nil(fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Remove)
	assert.Len(vsr.RequestMessages, 1)
	assert.Regexp("Restored cache allocation .requested=50B allocated=30B", vsr.RequestMessages[0].Message)

	t.Log("case: restoreCache decrease success")
	vsr = vsrClone(vr)
	op.rhs.Request = vsr
	op.rhs.RetryLater = false
	op.rhs.InError = false
	op.rhs.AbortUndo = false
	fas.RetFLObj = &agentd.LUN{}
	fso.InClaimCacheVsID = ""
	fso.InClaimCacheMsb, fso.InClaimCacheDsb = 0, 0
	fso.RetGetClaimSizeBytes = 60
	fso.RetClaimCacheErr = nil
	fso.RetUpdateNodeAvailableCacheErr = nil
	fc.InVSUpdaterID = ""
	fc.InVSUpdaterItems, fc.RetVSUpdaterErr = nil, nil
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	nvAPI.EXPECT().AllocCache(nuvoVolIdentifier, uint64(30)).Return(nil)
	c.App.NuvoAPI = nvAPI
	assert.NotPanics(func() { op.restoreCache(ctx) })
	assert.False(op.rhs.AbortUndo)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.EqualValues(v.Meta.ID, fso.InClaimCacheVsID)
	assert.EqualValues(30, fso.InClaimCacheMsb)
	assert.EqualValues(30, fso.InClaimCacheDsb)
	assert.EqualValues(v.Meta.ID, fc.InVSUpdaterID)
	assert.Equal([]string{"cacheAllocations"}, fc.InVSUpdaterItems.Append)
	expCA = map[string]models.CacheAllocation{
		"node1": {AllocatedSizeBytes: swag.Int64(30), RequestedSizeBytes: swag.Int64(50)},
	}
	assert.Equal(expCA, vs.CacheAllocations)
	assert.Nil(fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Remove)
	assert.EqualValues(30, fas.RetFLObj.AllocatedCacheSizeBytes)
	assert.EqualValues(50, fas.RetFLObj.RequestedCacheSizeBytes)
	assert.Len(vsr.RequestMessages, 1)
	assert.Regexp("Restored cache allocation .requested=50B allocated=30B", vsr.RequestMessages[0].Message)
}

func TestGetCacheRequired(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	fas := &appServant.AppServant{}
	app.AppServant = fas
	fao := &appServant.AppObjects{}
	app.AppObjects = fao
	fso := fakeState.NewFakeNodeState()
	app.StateOps = fso
	app.LMDGuard = util.NewCriticalSectionGuard()
	assert.NotNil(app.LMDGuard)

	c := newComponent()
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc
	fm := &vscFakeMounter{}

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
				StoragePlan: &models.StoragePlan{
					StorageElements: []*models.StoragePlanStorageElement{
						{
							Intent:    "CACHE",
							SizeBytes: swag.Int64(10),
							StorageParcels: map[string]models.StorageParcelElement{
								"SSD": models.StorageParcelElement{ProvMinSizeBytes: swag.Int64(1)},
							},
						},
					},
				},
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID:         "node1",
				VolumeSeriesID: "VS-1",
			},
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
				VolumeSeriesState: "IN_USE",
				Mounts:            []*models.Mount{},
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				Name:          "name",
				SizeBytes:     swag.Int64(3221225472),
				ServicePlanID: "plan1",
			},
		},
	}
	rhs := &vra.RequestHandlerState{
		A:            c.Animator,
		HasMount:     true,
		Request:      vr,
		VolumeSeries: v,
	}
	op := &exportOp{
		c:    c,
		rhs:  rhs,
		mops: fm,
	}
	fao.Node = &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "nid1",
			},
			NodeIdentifier: "node-identifier",
		},
		NodeMutable: models.NodeMutable{
			Service: &models.NuvoService{
				ServiceState: models.ServiceState{
					State: "RUNNING",
				},
			},
			Name:        "node1",
			Description: "node1 object",
			LocalStorage: map[string]models.NodeStorageDevice{
				"uuid2": models.NodeStorageDevice{
					DeviceName:      "d1",
					DeviceState:     "CACHE",
					DeviceType:      "SSD",
					SizeBytes:       swag.Int64(2200),
					UsableSizeBytes: swag.Int64(2000),
				},
			},
			AvailableCacheBytes: swag.Int64(2000),
			TotalCacheBytes:     swag.Int64(2200),
		},
	}

	// success
	desired, min, err := op.getCacheRequired()
	assert.NoError(err)
	assert.EqualValues(10, desired)
	assert.EqualValues(1, min)

	// empty plan
	plan := vr.StoragePlan
	vr.StoragePlan = &models.StoragePlan{}
	desired, min, err = op.getCacheRequired()
	assert.NoError(err)
	assert.Zero(desired)
	assert.Zero(min)
	vr.StoragePlan = plan

	// wrong type
	fao.Node.LocalStorage["uuid2"] = models.NodeStorageDevice{
		DeviceName:      "d1",
		DeviceState:     "CACHE",
		DeviceType:      "HDD",
		SizeBytes:       swag.Int64(2200),
		UsableSizeBytes: swag.Int64(2000),
	}
	desired, min, err = op.getCacheRequired()
	assert.Regexp("Mismatched .*HDD,.* SSD", err)
	assert.Zero(desired)
	assert.Zero(min)

	// no local storage
	fao.Node.LocalStorage = nil
	desired, min, err = op.getCacheRequired()
	assert.Regexp("Local storage data unavailable", err)
	assert.Zero(desired)
	assert.Zero(min)

	// zero min: success
	plan.StorageElements[0].StorageParcels["SSD"] = models.StorageParcelElement{}
	desired, min, err = op.getCacheRequired()
	assert.NoError(err)
	assert.EqualValues(10, desired)
	assert.Zero(min)

	// no cache in plan
	plan.StorageElements[0].Intent = "STORAGE"
	desired, min, err = op.getCacheRequired()
	assert.NoError(err)
	assert.Zero(desired)
	assert.Zero(min)

	// cache in plan, empty StorageParcels
	plan.StorageElements[0].Intent = "CACHE"
	plan.StorageElements[0].StorageParcels = nil
	desired, min, err = op.getCacheRequired()
	assert.NoError(err)
	assert.EqualValues(10, desired)
	assert.Zero(min)

	// cache in plan, zero sizeBytes
	plan.StorageElements[0].SizeBytes = nil
	desired, min, err = op.getCacheRequired()
	assert.NoError(err)
	assert.Zero(desired)
	assert.Zero(min)
}

func TestExport(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log:             tl.Logger(),
			HeartbeatPeriod: 10,
			CSPDomainID:     "6a156de2-ba94-4035-b47f-e87373d75757",
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	app.LMDGuard = util.NewCriticalSectionGuard()
	assert.NotNil(app.LMDGuard)

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
				VolumeSeriesRequestState: "VOLUME_EXPORT",
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID:         "node1",
				VolumeSeriesID: "VS-1",
			},
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
				BoundClusterID: "cl-1",
				StorageParcels: map[string]models.ParcelAllocation{
					"s1": {SizeBytes: swag.Int64(1073741824)},
					"s2": {SizeBytes: swag.Int64(1073741824)},
					"s3": {SizeBytes: swag.Int64(1073741824)},
				},
				Messages:          []*models.TimestampedString{},
				VolumeSeriesState: "IN_USE",
				Mounts:            []*models.Mount{},
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				Name:          "name",
				SizeBytes:     swag.Int64(3221225472),
				ServicePlanID: "plan1",
			},
		},
	}
	c := newComponent()
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc

	newFakeExportOp := func() *fakeExportOp {
		op := &fakeExportOp{}
		op.ops = op
		op.c = c
		op.rhs = &vra.RequestHandlerState{Request: vr}
		return op
	}
	var op *fakeExportOp

	op = newFakeExportOp()
	op.retGIS = ExportDone
	expCalled := []string{"GIS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeExportOp()
	op.retGIS = ExportGetStatUUID
	expCalled = []string{"GIS", "GWS", "AC", "EL", "SS-MOUNTED", "RM"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeExportOp()
	op.retGIS = ExportSetMounting
	expCalled = []string{"GIS", "SS-MOUNTING", "FSP", "GWS", "AC", "EL", "SS-MOUNTED", "RM"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeExportOp()
	op.retGIS = ExportSaveProps
	expCalled = []string{"GIS", "VS", "UC"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeExportOp()
	op.retGIS = ExportSetUnmounting
	expCalled = []string{"GIS", "SS-UNMOUNTING", "RM", "UL", "RC", "GWS", "SS-UNMOUNTED"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeExportOp()
	op.retGIS = ExportSetUnmounting
	op.smsMustUnconfigure = true
	expCalled = []string{"GIS", "SS-UNMOUNTING", "RM", "UL", "RC", "GWS", "SS-UNMOUNTED", "UC"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeExportOp()
	op.retGIS = ExportSetError
	expCalled = []string{"GIS", "SS-ERROR", "RM", "UL", "RC", "GWS", "SS-UNMOUNTED"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeExportOp()
	op.retGIS = ExportSetError
	op.smsMustUnconfigure = true
	expCalled = []string{"GIS", "SS-ERROR", "RM", "UL", "RC", "GWS", "SS-UNMOUNTED", "UC"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeExportOp()
	op.retGIS = ExportUnexportLun
	expCalled = []string{"GIS", "UL", "RC", "GWS", "SS-UNMOUNTED"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeExportOp()
	op.retGIS = ExportUnexportLun
	op.smsMustUnconfigure = true
	expCalled = []string{"GIS", "UL", "RC", "GWS", "SS-UNMOUNTED", "UC"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeExportOp()
	op.retGIS = ExportRestoreCache
	expCalled = []string{"GIS", "REC"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeExportOp()
	op.retGIS = ExportUndoDone
	expCalled = []string{"GIS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// call the real handler with an error
	rhs := &vra.RequestHandlerState{
		A:            c.Animator,
		InError:      true,
		HasMount:     true,
		Request:      vr,
		VolumeSeries: v,
	}

	// call real handler in export without exportInternalCallStashKey
	c.Export(nil, rhs)
	assert.True(rhs.InError)
	sv := rhs.StashGet(exportExportOp{})
	assert.NotNil(sv)
	eOp, ok := sv.(*exportOp)
	assert.True(ok)
	assert.Equal(eOp.snapID, "HEAD")
	assert.Equal(eOp.pitUUID, "")
	assert.True(eOp.isWritable)
	assert.False(eOp.ignoreFSN)

	// call real handler in export with exportInternalCallStashKey
	exportIntArgs := &exportInternalArgs{
		snapID:  "snapID",
		pitUUID: "pitUUID",
	}
	rhs.StashSet(exportInternalCallStashKey{}, exportIntArgs)
	c.Export(nil, rhs)
	assert.True(rhs.InError)
	sv = rhs.StashGet(exportExportOp{})
	assert.NotNil(sv)
	eOp, ok = sv.(*exportOp)
	assert.True(ok)
	assert.Equal(eOp.snapID, exportIntArgs.snapID)
	assert.Equal(eOp.pitUUID, exportIntArgs.pitUUID)
	assert.False(eOp.isWritable)
	assert.False(eOp.ignoreFSN)

	// call real handler in undo with exportInternalCallStashKey
	vr.VolumeSeriesRequestState = "UNDO_VOLUME_EXPORT"
	v.Mounts = nil
	c.Export(nil, rhs)
	rhs.InError = true
	rhs.RetryLater = false
	exportIntArgs = &exportInternalArgs{
		snapID:    "HEAD",
		pitUUID:   "PitUUID",
		ignoreFSN: true,
	}
	rhs.StashSet(exportInternalCallStashKey{}, exportIntArgs)
	c.UndoExport(nil, rhs)
	assert.False(rhs.RetryLater)
	assert.True(rhs.InError)
	sv = rhs.StashGet(exportExportOp{})
	assert.NotNil(sv)
	eOp, ok = sv.(*exportOp)
	assert.True(ok)
	assert.True(eOp.ignoreFSN)
	assert.Equal(eOp.snapID, "HEAD")
	assert.Equal(eOp.pitUUID, "PitUUID")
	assert.False(eOp.isWritable)
	assert.True(eOp.ignoreFSN)

	// call real handler in undo without exportInternalCallStashKey
	vr.VolumeSeriesRequestState = "UNDO_VOLUME_EXPORT"
	v.Mounts = nil
	rhs.StashSet(exportInternalCallStashKey{}, nil)
	c.Export(nil, rhs)
	rhs.InError = true
	rhs.RetryLater = false
	c.UndoExport(nil, rhs)
	assert.False(rhs.RetryLater)
	assert.True(rhs.InError)
	sv = rhs.StashGet(exportExportOp{})
	assert.NotNil(sv)
	eOp, ok = sv.(*exportOp)
	assert.True(ok)
	assert.Equal(eOp.snapID, "HEAD")
	assert.Equal(eOp.pitUUID, "")
	assert.True(eOp.isWritable)
	assert.False(eOp.ignoreFSN)

	// call real handler in export with bad exportInternalCallStashKey
	rhs.StashSet(exportInternalCallStashKey{}, &exportUnconfigureOp{})
	assert.Panics(func() { c.Export(nil, rhs) })

	// check state strings exists
	var ss exportSubState
	for ss = ExportSetMounting; ss < ExportNoOp; ss++ {
		s := ss.String()
		tl.Logger().Debugf("Testing %d %s", ss, s)
		assert.Regexp("^Export", s)
	}
	assert.Regexp("^exportSubState", ss.String())

	// call real ReallocateCache handler with error
	rhs = &vra.RequestHandlerState{
		A:                 c.Animator,
		InError:           true,
		HasChangeCapacity: true,
		Request:           vr,
		VolumeSeries:      v,
	}
	vr.VolumeSeriesRequestState = "REALLOCATING_CACHE"
	c.ReallocateCache(nil, rhs)
	assert.True(rhs.InError)

	// call real UndoReallocateCache handler with error
	vr.VolumeSeriesRequestState = "UNDO_REALLOCATING_CACHE"
	c.UndoReallocateCache(nil, rhs)
	assert.True(rhs.InError)
}

type fakeExportOp struct {
	exportOp
	called             []string
	retGIS             exportSubState
	smsMustUnconfigure bool
}

func (op *fakeExportOp) getInitialState(ctx context.Context) exportSubState {
	op.called = append(op.called, "GIS")
	return op.retGIS
}

func (op *fakeExportOp) fetchServicePlan(ctx context.Context) {
	op.called = append(op.called, "FSP")
}

func (op *fakeExportOp) setMountState(ctx context.Context, state string) {
	op.called = append(op.called, "SS-"+state)
	if state == "UNMOUNTED" {
		op.mustUnconfigure = op.smsMustUnconfigure
	}
}

func (op *fakeExportOp) exportLun(ctx context.Context) {
	op.called = append(op.called, "EL")
}

func (op *fakeExportOp) allocateCache(ctx context.Context) {
	op.called = append(op.called, "AC")
}

func (op *fakeExportOp) vsrSaveVSProps(ctx context.Context) {
	op.called = append(op.called, "VS")
}

func (op *fakeExportOp) updateCache(ctx context.Context) {
	op.called = append(op.called, "UC")
}

func (op *fakeExportOp) unExportLun(ctx context.Context) {
	op.called = append(op.called, "UL")
}

func (op *fakeExportOp) releaseCache(ctx context.Context) {
	op.called = append(op.called, "RC")
}

func (op *fakeExportOp) unConfigureLun(ctx context.Context) {
	op.called = append(op.called, "UC")
}

func (op *fakeExportOp) nuvoGetWriteStat(ctx context.Context) {
	op.called = append(op.called, "GWS")
}

func (op *fakeExportOp) reportMetrics(ctx context.Context) {
	op.called = append(op.called, "RM")
}
func (op *fakeExportOp) restoreCache(ctx context.Context) {
	op.called = append(op.called, "REC")
}
