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
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/node"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	mockNuvo "github.com/Nuvoloso/kontroller/pkg/nuvoapi/mock"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	fvra "github.com/Nuvoloso/kontroller/pkg/vra/fake"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestConfigureSteps(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log:      tl.Logger(),
			NuvoPort: 31313,
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM

	c := newComponent()
	c.Init(app)
	fc := &fake.Client{}
	c.thisNodeID = "n1"
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
				VolumeSeriesRequestState: "VOLUME_CONFIG",
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
				VolumeSeriesState: "PROVISIONED",
				Mounts:            []*models.Mount{},
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				Name:          "name",
				SizeBytes:     swag.Int64(3221225472),
				ServicePlanID: "plan1",
			},
		},
	}
	s1 := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta:      &models.ObjMeta{ID: "s1"},
			SizeBytes: swag.Int64(2147483648),
		},
		StorageMutable: models.StorageMutable{
			AvailableBytes:  swag.Int64(1040187392),
			ParcelSizeBytes: swag.Int64(33554432),
			StorageState: &models.StorageStateMutable{
				AttachedNodeID: "n1",
			},
			TotalParcelCount: swag.Int64(63),
		},
	}
	s2 := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta:      &models.ObjMeta{ID: "s2"},
			SizeBytes: swag.Int64(2147483648),
		},
		StorageMutable: models.StorageMutable{
			AvailableBytes:  swag.Int64(1056964608),
			ParcelSizeBytes: swag.Int64(16777216),
			StorageState: &models.StorageStateMutable{
				AttachedNodeID: "n2",
			},
			TotalParcelCount: swag.Int64(127),
		},
	}
	s3 := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta:      &models.ObjMeta{ID: "s3"},
			SizeBytes: swag.Int64(2147483648),
		},
		StorageMutable: models.StorageMutable{
			AvailableBytes:  swag.Int64(1040187392),
			ParcelSizeBytes: swag.Int64(33554432),
			StorageState: &models.StorageStateMutable{
				AttachedNodeID: "n1",
			},
			TotalParcelCount: swag.Int64(63),
		},
	}
	n2 := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{ID: "n2"},
		},
		NodeMutable: models.NodeMutable{
			Service: &models.NuvoService{
				NuvoServiceAllOf0: models.NuvoServiceAllOf0{
					ServiceIP: "1.2.3.4",
				},
			},
		},
	}
	rhs := &vra.RequestHandlerState{
		A:            c.Animator,
		HasMount:     true,
		Request:      vr,
		VolumeSeries: v,
	}
	op := &configureOp{
		c:              c,
		rhs:            rhs,
		storageParcels: make(map[string]*parcelData, 0),
		nodeLocations:  make(map[string]*nodeLocation, 0),
	}
	logVolCreatorTag := fmt.Sprintf("%s:%s", com.SystemTagVsrSetStorageID, vr.Meta.ID)
	logVolCreatedTag := fmt.Sprintf("%s:%s", com.SystemTagVsrCreatedLogVol, vr.Meta.ID)
	restoredTag := fmt.Sprintf("%s:%s", com.SystemTagVsrRestored, vr.Meta.ID)

	nuvoVolIdentifier := "nuvo-vol-id"

	// ***************************** getInitialState
	vr.PlanOnly = swag.Bool(true)
	assert.Equal(ConfigureDone, op.getInitialState(ctx))
	vr.PlanOnly = swag.Bool(false)

	// undo done
	vr.VolumeSeriesRequestState = "UNDO_VOLUME_CONFIG"
	rhs.InError = true
	assert.Equal(ConfigureUndoDone, op.getInitialState(ctx))
	assert.True(op.inError)
	assert.False(rhs.InError)
	op.inError = false
	rhs.InError = false
	vr.VolumeSeriesRequestState = "VOLUME_CONFIG"

	// unconfigure invoked by unexport on restore-from-snapshot
	vr.VolumeSeriesRequestState = "UNDO_VOLUME_CONFIG"
	v.SystemTags = []string{restoredTag}
	v.SystemTags = append(v.SystemTags, logVolCreatorTag) // will be ignored
	v.RootStorageID = "s3"
	v.VolumeSeriesState = "CONFIGURED"
	rhs.InError = true
	assert.Equal(ConfigureSetProvisioned, op.getInitialState(ctx))
	assert.True(op.inError)
	assert.False(rhs.InError)
	op.inError = false
	rhs.InError = false
	vr.VolumeSeriesRequestState = "VOLUME_CONFIG"
	v.SystemTags = []string{}

	// undo vs create / mount path
	vr.VolumeSeriesRequestState = "UNDO_VOLUME_CONFIG"
	v.SystemTags = append(v.SystemTags, logVolCreatorTag)
	rhs.InError = true
	assert.Equal(ConfigureCloseDeleteLogVol, op.getInitialState(ctx))
	assert.True(op.inError)
	assert.False(rhs.InError)
	op.inError = false
	rhs.InError = false
	v.SystemTags = []string{}
	vr.VolumeSeriesRequestState = "VOLUME_CONFIG"

	vr.VolumeSeriesRequestState = "UNDO_VOLUME_CONFIG"
	v.RootStorageID = "s3"
	rhs.InError = true
	assert.Equal(ConfigureSetProvisioned, op.getInitialState(ctx))
	assert.True(op.inError)
	assert.False(rhs.InError)
	op.inError = false
	rhs.InError = false
	v.RootStorageID = ""
	vr.VolumeSeriesRequestState = "VOLUME_CONFIG"

	vr.VolumeSeriesRequestState = "UNDO_VOLUME_CONFIG"
	v.RootStorageID = "s3"
	v.VolumeSeriesState = "IN_USE"
	assert.Equal(ConfigureUndoDone, op.getInitialState(ctx))
	assert.False(op.inError)
	assert.False(rhs.InError)
	op.inError = false
	rhs.InError = false
	v.RootStorageID = ""
	v.VolumeSeriesState = "PROVISIONED"
	vr.VolumeSeriesRequestState = "VOLUME_CONFIG"

	vr.VolumeSeriesRequestState = "UNDO_VOLUME_CONFIG"
	v.VolumeSeriesState = "PROVISIONED"
	v.RootStorageID = "s3"
	rhs.HasUnbind = true
	assert.Equal(ConfigureCloseDeleteLogVol, op.getInitialState(ctx))
	rhs.HasUnbind = false
	vr.VolumeSeriesRequestState = "VOLUME_CONFIG"

	vr.VolumeSeriesRequestState = "UNDO_VOLUME_CONFIG"
	v.VolumeSeriesState = "PROVISIONED"
	v.RootStorageID = ""
	rhs.HasUnbind = true
	assert.Equal(ConfigureUndoDone, op.getInitialState(ctx))
	rhs.HasUnbind = false
	vr.VolumeSeriesRequestState = "VOLUME_CONFIG"

	vr.VolumeSeriesRequestState = "UNDO_VOLUME_CONFIG"
	rhs.HasDelete = true
	assert.Equal(ConfigureSetDeleting, op.getInitialState(ctx))
	rhs.HasDelete = false
	vr.VolumeSeriesRequestState = "VOLUME_CONFIG"

	vr.VolumeSeriesRequestState = "UNDO_VOLUME_CONFIG"
	v.VolumeSeriesState = "DELETING"
	v.RootStorageID = "s3"
	rhs.HasDelete = true
	assert.Equal(ConfigureCloseDeleteLogVol, op.getInitialState(ctx))
	rhs.HasDelete = false
	vr.VolumeSeriesRequestState = "VOLUME_CONFIG"

	vr.VolumeSeriesRequestState = "UNDO_VOLUME_CONFIG"
	v.VolumeSeriesState = "DELETING"
	v.RootStorageID = ""
	rhs.HasDelete = true
	assert.Equal(ConfigureUndoDone, op.getInitialState(ctx))
	rhs.HasDelete = false
	vr.VolumeSeriesRequestState = "VOLUME_CONFIG"

	v.RootStorageID = "s3"
	v.VolumeSeriesState = "IN_USE"
	assert.Equal(ConfigureDone, op.getInitialState(ctx))
	assert.False(rhs.InError)
	v.RootStorageID = ""

	v.VolumeSeriesState = "PROVISIONED"
	v.RootStorageID = "s3"
	assert.Equal(ConfigureStartExisting, op.getInitialState(ctx))
	assert.False(rhs.InError)
	v.RootStorageID = ""

	v.RootStorageID = "s3"
	v.SystemTags = append(v.SystemTags, logVolCreatorTag)
	assert.Equal(ConfigureRestartAndCreate, op.getInitialState(ctx))
	assert.False(rhs.InError)
	v.RootStorageID = ""

	v.RootStorageID = "s3"
	v.SystemTags = append(v.SystemTags, logVolCreatedTag)
	assert.Equal(ConfigureRestartAndOpenCreated, op.getInitialState(ctx))
	assert.False(rhs.InError)
	v.SystemTags = []string{}
	v.RootStorageID = ""

	assert.Equal(ConfigureStart, op.getInitialState(ctx))
	assert.False(rhs.InError)
	assert.Equal(logVolCreatorTag, op.logVolCreatorTag)
	assert.Equal(logVolCreatedTag, op.logVolCreatedTag)
	tl.Flush()

	// ***************************** queryConfigDB
	c.rei.SetProperty("config-query-fail", &rei.Property{BoolValue: true})
	op.queryConfigDB(ctx)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Config query error", vr.RequestMessages[0].Message)
	assert.Empty(op.storageParcels)
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	rhs.RetryLater = false

	vr.RequestMessages = nil
	fc.StorageFetchRetErr = fmt.Errorf("storage fetch failure")
	op.queryConfigDB(ctx)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Storage .* load failure", vr.RequestMessages[0].Message)
	assert.Empty(op.storageParcels)
	assert.True(rhs.RetryLater)
	rhs.RetryLater = false
	fc.StorageFetchRetErr = nil

	vr.RequestMessages = nil
	fc.StorageFetchRetErr = nil
	fc.StorageFetchRetObj = s2
	fc.RetNErr = fmt.Errorf("node fetch failure")
	op.queryConfigDB(ctx)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Node .* fetch failure", vr.RequestMessages[0].Message)
	assert.Len(op.storageParcels, 1)
	assert.True(rhs.RetryLater)
	delete(op.storageParcels, fc.StorageFetchID)
	rhs.RetryLater = false
	fc.StorageFetchRetObj = nil
	fc.RetNErr = nil

	resS1 := &storage.StorageFetchOK{Payload: s1}
	resS2 := &storage.StorageFetchOK{Payload: s2}
	resS3 := &storage.StorageFetchOK{Payload: s3}
	resN2 := &node.NodeFetchOK{Payload: n2}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	mc := crud.NewClient(mAPI, app.Log)
	c.oCrud = mc
	c.Animator.OCrud = mc
	sOps := mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(sOps).MinTimes(1)
	mS1 := &storageFetchMatcher{FetchParam: storage.NewStorageFetchParams()}
	mS1.FetchParam.ID = string(s1.Meta.ID)
	mS2 := &storageFetchMatcher{FetchParam: storage.NewStorageFetchParams()}
	mS2.FetchParam.ID = string(s2.Meta.ID)
	mS3 := &storageFetchMatcher{FetchParam: storage.NewStorageFetchParams()}
	mS3.FetchParam.ID = string(s3.Meta.ID)
	sOps.EXPECT().StorageFetch(mS1).Return(resS1, nil)
	sOps.EXPECT().StorageFetch(mS2).Return(resS2, nil)
	sOps.EXPECT().StorageFetch(mS3).Return(resS3, nil)
	nOps := mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps).MinTimes(1)
	mN2 := mockmgmtclient.NewNodeMatcher(t, node.NewNodeFetchParams().WithID("n2"))
	nOps.EXPECT().NodeFetch(mN2).Return(resN2, nil)
	assert.NotPanics(func() { op.queryConfigDB(ctx) })
	assert.Len(vr.RequestMessages, 1)
	assert.Len(op.storageParcels, 3)
	p, ok := op.storageParcels["s1"]
	if assert.True(ok, "parcel of s1") {
		assert.Equal(uint64(32), p.parcelCount)
		assert.Equal("n1", p.nodeUUID)
	}
	p, ok = op.storageParcels["s2"]
	if assert.True(ok, "parcel of s2") {
		assert.Equal(uint64(64), p.parcelCount)
		assert.Equal("n2", p.nodeUUID)
	}
	p, ok = op.storageParcels["s3"]
	if assert.True(ok, "parcel of s3") {
		assert.Equal(uint64(32), p.parcelCount)
		assert.Equal("n1", p.nodeUUID)
	}
	c.oCrud = fc
	c.Animator.OCrud = fc

	// ***************************** setDeviceLocations
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI := mockNuvo.NewMockNuvoVM(mockCtrl)
	c.App.NuvoAPI = nvAPI
	nvAPI.EXPECT().NodeLocation("n2", "1.2.3.4", uint16(31313)).Return(nil)
	nvAPI.EXPECT().DeviceLocation(string(s1.Meta.ID), "n1").Return(fmt.Errorf("location-error"))
	nvAPI.EXPECT().DeviceLocation(string(s2.Meta.ID), "n2").Return(nil).AnyTimes()
	nvAPI.EXPECT().DeviceLocation(string(s3.Meta.ID), "n1").Return(nil).AnyTimes()
	assert.NotPanics(func() { op.setDeviceLocations(ctx) })
	assert.True(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 2)
	rhs.InError = false
	assert.Regexp("DeviceLocation .* failed: location-error", vr.RequestMessages[1].Message)

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.App.NuvoAPI = nvAPI
	vr.RequestMessages = vr.RequestMessages[:1]
	nvAPI.EXPECT().NodeLocation("n2", "1.2.3.4", uint16(31313)).Return(fmt.Errorf("location-error"))
	assert.NotPanics(func() { op.setDeviceLocations(ctx) })
	assert.True(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 2)
	rhs.InError = false
	assert.Regexp("NodeLocation .* failed: location-error", vr.RequestMessages[1].Message)

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.App.NuvoAPI = nvAPI
	nvAPI.EXPECT().NodeLocation("n2", "1.2.3.4", uint16(31313)).Return(fmt.Errorf(errNodeLocationAlreadySet)) // ignored
	nvAPI.EXPECT().DeviceLocation(string(s1.Meta.ID), "n1").Return(fmt.Errorf(errDeviceLocationAlreadySet))   // ignored
	nvAPI.EXPECT().DeviceLocation(string(s2.Meta.ID), "n2").Return(nil)
	nvAPI.EXPECT().DeviceLocation(string(s3.Meta.ID), "n1").Return(nil)
	assert.NotPanics(func() { op.setDeviceLocations(ctx) })
	assert.False(rhs.InError)
	assert.Len(vr.RequestMessages, 2)

	// ***************************** setRootStorageID
	fc.RetVSUpdaterObj, fc.RetVSUpdaterErr = nil, fmt.Errorf("update failure")
	assert.NotPanics(func() { op.setRootStorageID(ctx) })
	assert.True(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 3)
	assert.Regexp("Failed to update VolumeSeries.*: update failure", vr.RequestMessages[2].Message)
	rhs.RetryLater = false

	fc.RetVSUpdaterObj, fc.RetVSUpdaterErr = nil, nil
	fc.InUVRObj = nil
	v.RootStorageID = "s5"
	assert.NotPanics(func() { op.setRootStorageID(ctx) })
	assert.Nil(fc.InUVRObj)
	assert.True(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 4)
	assert.Equal("rootStorageId is already set", vr.RequestMessages[3].Message)
	rhs.InError = false

	fc.RetVSUpdaterObj, fc.RetVSUpdaterErr = nil, nil
	fc.RetVSUpdaterUpdateErr = fmt.Errorf("updater error")
	fc.ModVSRUpdaterObj = nil
	v.RootStorageID = ""
	v.NuvoVolumeIdentifier = ""
	assert.NotPanics(func() { op.setRootStorageID(ctx) })
	assert.NotNil(fc.ModVSRUpdaterObj)
	assert.Empty(fc.InVSUpdaterItems.Remove)
	assert.Equal([]string{"rootStorageId", "nuvoVolumeIdentifier"}, fc.InVSUpdaterItems.Set)
	assert.Equal([]string{"messages", "systemTags"}, fc.InVSUpdaterItems.Append)
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 5)
	assert.Regexp("Failed to update VolumeSeries.*: updater error", vr.RequestMessages[4].Message)
	rhs.RetryLater = false

	fc.RetVSUpdaterUpdateErr = nil
	fc.InUVRObj = nil
	v.RootStorageID = ""
	v.NuvoVolumeIdentifier = ""
	assert.NotPanics(func() { op.setRootStorageID(ctx) })
	assert.Nil(fc.InUVRObj)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 6)
	assert.Regexp("Set rootStorageId", vr.RequestMessages[5].Message)
	assert.EqualValues("s3", v.RootStorageID)
	assert.NotEmpty(v.NuvoVolumeIdentifier)
	assert.Equal(logVolCreatorTag, v.SystemTags[0])
	assert.Len(v.Messages, 1)
	assert.Regexp("Set rootStorageId.*nuvoVolumeIdentifier", v.Messages[0].Message)
	tl.Flush()

	// ***************************** createLogVol
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.App.NuvoAPI = nvAPI
	sizeBytes := uint64(swag.Int64Value(v.SizeBytes))
	v.NuvoVolumeIdentifier = nuvoVolIdentifier
	nvAPI.EXPECT().CreateLogVol(nuvoVolIdentifier, "s3", v.RootParcelUUID, sizeBytes).Return("", fmt.Errorf(errVolumeAlreadyExists))
	assert.NotPanics(func() { op.createLogVol(ctx) })
	assert.False(rhs.InError)
	assert.False(op.alreadyOpen)
	assert.Equal(1, tl.CountPattern("CreateLogVol error.*exists"))
	tl.Flush()

	nvAPI.EXPECT().CreateLogVol(nuvoVolIdentifier, "s3", v.RootParcelUUID, sizeBytes).Return("", nil)
	assert.NotPanics(func() { op.createLogVol(ctx) })
	assert.True(rhs.InError)
	assert.False(op.alreadyOpen)
	assert.Equal(1, tl.CountPattern("CreateLogVol .* different rootParcelUUID"))
	rhs.InError = false
	op.alreadyOpen = false

	fc.RetVSUpdaterObj, fc.RetVSUpdaterErr = nil, fmt.Errorf("update failure")
	vr.RequestMessages = nil
	nvAPI.EXPECT().CreateLogVol(nuvoVolIdentifier, "s3", v.RootParcelUUID, sizeBytes).Return(v.RootParcelUUID, nil)
	assert.NotPanics(func() { op.createLogVol(ctx) })
	assert.True(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Failed to update VolumeSeries.*: update failure", vr.RequestMessages[0].Message)
	rhs.RetryLater = false

	fc.RetVSUpdaterObj, fc.RetVSUpdaterErr = nil, nil
	fc.RetVSUpdaterUpdateErr = fmt.Errorf("updater error")
	fc.ModVSRUpdaterObj = nil
	nvAPI.EXPECT().CreateLogVol(nuvoVolIdentifier, "s3", v.RootParcelUUID, sizeBytes).Return(v.RootParcelUUID, nil)
	assert.NotPanics(func() { op.createLogVol(ctx) })
	assert.NotNil(fc.ModVSRUpdaterObj)
	assert.Empty(fc.InVSUpdaterItems.Remove)
	assert.Empty(fc.InVSUpdaterItems.Set)
	assert.Equal([]string{"messages", "systemTags"}, fc.InVSUpdaterItems.Append)
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 2)
	assert.Regexp("Failed to update VolumeSeries.*: updater error", vr.RequestMessages[1].Message)
	rhs.RetryLater = false
	tl.Flush()

	fc.RetVSUpdaterUpdateErr = nil
	fc.InUVRObj = nil
	nvAPI.EXPECT().CreateLogVol(nuvoVolIdentifier, "s3", v.RootParcelUUID, sizeBytes).Return("", fmt.Errorf("bad error"))
	assert.NotPanics(func() { op.createLogVol(ctx) })
	assert.Nil(fc.InUVRObj)
	assert.True(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 3)
	assert.Regexp("CreateLogVol failed: .*", vr.RequestMessages[2].Message)
	rhs.InError = false
	tl.Flush()

	fc.RetVSUpdaterUpdateErr = nil
	fc.InUVRObj = nil
	nvAPI.EXPECT().CreateLogVol(nuvoVolIdentifier, "s3", v.RootParcelUUID, sizeBytes).Return(v.RootParcelUUID, nil)
	assert.NotPanics(func() { op.createLogVol(ctx) })
	assert.Nil(fc.InUVRObj)
	assert.True(op.alreadyOpen)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 4)
	assert.Regexp("Created LogVol .*"+v.RootParcelUUID, vr.RequestMessages[3].Message)
	assert.EqualValues("s3", v.RootStorageID)
	assert.Equal(logVolCreatedTag, v.SystemTags[0])
	assert.Len(v.Messages, 1)
	assert.Regexp("created the LogVol", v.Messages[0].Message)
	tl.Flush()

	// ***************************** openLogVol
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.App.NuvoAPI = nvAPI
	v.NuvoVolumeIdentifier = nuvoVolIdentifier
	op.alreadyOpen = true
	vr.RequestMessages = nil
	op.openLogVol(ctx)
	assert.False(rhs.InError)

	op.alreadyOpen = false
	nvAPI.EXPECT().OpenVol(nuvoVolIdentifier, "s3", v.RootParcelUUID, true).Return(fmt.Errorf("open failure"))
	op.openLogVol(ctx)
	assert.True(rhs.InError)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("OpenVol error: open failure", vr.RequestMessages[0].Message)
	rhs.InError = false

	op.alreadyOpen = false
	nvAPI.EXPECT().OpenVol(nuvoVolIdentifier, "s3", v.RootParcelUUID, true).Return(fmt.Errorf(errVolumeAlreadyOpen)) // ignored
	op.openLogVol(ctx)
	assert.False(rhs.InError)
	assert.Len(vr.RequestMessages, 2)
	assert.Regexp("Opened LogVol .*"+v.RootParcelUUID, vr.RequestMessages[1].Message)

	op.alreadyOpen = false
	nvAPI.EXPECT().OpenVol(nuvoVolIdentifier, "s3", v.RootParcelUUID, true).Return(nil)
	op.openLogVol(ctx)
	assert.False(rhs.InError)
	assert.Len(vr.RequestMessages, 3)
	assert.Regexp("Opened LogVol .*"+v.RootParcelUUID, vr.RequestMessages[2].Message)
	tl.Flush()

	// ***************************** allocParcels
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.App.NuvoAPI = nvAPI
	v.NuvoVolumeIdentifier = nuvoVolIdentifier
	vr.RequestMessages = nil
	nvAPI.EXPECT().AllocParcels(nuvoVolIdentifier, "s1", uint64(32)).Return(nil).AnyTimes()
	nvAPI.EXPECT().AllocParcels(nuvoVolIdentifier, "s2", uint64(64)).Return(fmt.Errorf("alloc failure"))
	nvAPI.EXPECT().AllocParcels(nuvoVolIdentifier, "s3", uint64(32)).Return(nil).AnyTimes()
	op.allocParcels(ctx)
	assert.True(rhs.InError)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("AllocParcels .* failed: alloc failure", vr.RequestMessages[0].Message)
	rhs.InError = false

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.App.NuvoAPI = nvAPI
	op.storageParcels["s3"].parcelCount = 0
	nvAPI.EXPECT().AllocParcels(nuvoVolIdentifier, "s1", uint64(32)).Return(nil)
	nvAPI.EXPECT().AllocParcels(nuvoVolIdentifier, "s2", uint64(64)).Return(nil)
	op.allocParcels(ctx)
	assert.False(rhs.InError)
	assert.Len(vr.RequestMessages, 2)
	assert.Regexp("Allocated storage parcels", vr.RequestMessages[1].Message)

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.App.NuvoAPI = nvAPI
	op.storageParcels["s3"].parcelCount = 32 // reset
	nvAPI.EXPECT().AllocParcels(nuvoVolIdentifier, "s1", uint64(32)).Return(nil)
	nvAPI.EXPECT().AllocParcels(nuvoVolIdentifier, "s2", uint64(64)).Return(nil)
	nvAPI.EXPECT().AllocParcels(nuvoVolIdentifier, "s3", uint64(32)).Return(nil)
	op.allocParcels(ctx)
	assert.False(rhs.InError)
	assert.Len(vr.RequestMessages, 3)
	assert.Regexp("Allocated storage parcels", vr.RequestMessages[2].Message)
	tl.Flush()

	// ***************************** closeLogVol
	mockCtrl.Finish()
	tl.Logger().Info("case: closeLogVol failure")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.App.NuvoAPI = nvAPI
	v.NuvoVolumeIdentifier = nuvoVolIdentifier
	vr.RequestMessages = nil
	nuvoErr := nuvoapi.WrapError(fmt.Errorf("close failure"))
	nei, ok := nuvoErr.(nuvoapi.ErrorInt)
	assert.True(ok)
	assert.False(nei.NotInitialized())
	nvAPI.EXPECT().CloseVol(nuvoVolIdentifier).Return(nuvoErr)
	op.closeLogVol(ctx)
	assert.True(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("CloseVol failed: close failure", vr.RequestMessages[0].Message)
	rhs.InError = false

	tl.Logger().Info("case: closeLogVol failure (rei)")
	tl.Flush()
	c.rei.SetProperty("config-close-vol-error", &rei.Property{BoolValue: true})
	vr.RequestMessages = nil
	op.closeLogVol(ctx)
	assert.True(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 2)
	assert.Regexp("CloseVol failed:.*config-close-vol-error", vr.RequestMessages[1].Message)
	rhs.InError = false

	tl.Logger().Info("case: closeLogVol failure (rei)")
	tl.Flush()
	c.rei.SetProperty("block-close-log-vol", &rei.Property{BoolValue: true})
	vr.RequestMessages = nil
	op.closeLogVol(ctx)
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("block-close-log-vol", vr.RequestMessages[0].Message)
	rhs.RetryLater = false

	tl.Logger().Info("case: closeLogVol already closed")
	tl.Flush()
	nuvoErr = nuvoapi.WrapError(fmt.Errorf("%s: more stuff", errVolumeAlreadyClosed))
	nei, ok = nuvoErr.(nuvoapi.ErrorInt)
	assert.True(ok)
	assert.False(nei.NotInitialized())
	nvAPI.EXPECT().CloseVol(nuvoVolIdentifier).Return(nuvoErr) // ignored
	vr.RequestMessages = nil
	op.closeLogVol(ctx)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 1)
	assert.Equal("LogVol is closed", vr.RequestMessages[0].Message)

	tl.Logger().Info("case: closeLogVol already closed (rei)")
	tl.Flush()
	c.rei.SetProperty("config-close-vol-nfe", &rei.Property{BoolValue: true})
	vr.RequestMessages = nil
	op.closeLogVol(ctx)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 2)
	assert.Equal("LogVol is closed", vr.RequestMessages[1].Message)

	tl.Logger().Info("case: closeLogVol not initialized")
	tl.Flush()
	nuvoErr = nuvoapi.WrapFatalError(fmt.Errorf("NOT INITIALIZED"))
	nei, ok = nuvoErr.(nuvoapi.ErrorInt)
	assert.True(ok)
	assert.True(nei.NotInitialized())
	nvAPI.EXPECT().CloseVol(nuvoVolIdentifier).Return(nuvoErr) // ignored
	op.closeLogVol(ctx)
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	rhs.RetryLater = false

	tl.Logger().Info("case: closeLogVol temp error")
	tl.Flush()
	nuvoErr = nuvoapi.NewNuvoAPIError("tempErr", true)
	nvAPI.EXPECT().CloseVol(nuvoVolIdentifier).Return(nuvoErr) // ignored
	vr.RequestMessages = nil
	op.closeLogVol(ctx)
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("tempErr", vr.RequestMessages[0].Message)
	rhs.RetryLater = false

	tl.Logger().Info("case: closeLogVol success")
	tl.Flush()
	nvAPI.EXPECT().CloseVol(nuvoVolIdentifier).Return(nil)
	vr.RequestMessages = nil
	op.closeLogVol(ctx)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 1)
	assert.Equal("LogVol is closed", vr.RequestMessages[0].Message)

	//  ***************************** setVolumeState
	tl.Logger().Info("case: setVolumeState update failure")
	tl.Flush()
	fc.RetVSUpdaterObj, fc.RetVSUpdaterErr = nil, fmt.Errorf("update failure")
	fc.InVSUpdaterItems = nil
	vr.RequestMessages = nil
	assert.NotPanics(func() { op.setVolumeState(ctx, "DELETING") })
	assert.True(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Failed to update VolumeSeries.*: update failure", vr.RequestMessages[0].Message)
	assert.Equal([]string{"volumeSeriesState", "configuredNodeId"}, fc.InVSUpdaterItems.Set)
	assert.Equal([]string{"messages"}, fc.InVSUpdaterItems.Append)
	assert.Empty(fc.InVSUpdaterItems.Remove)
	assert.Equal(v, rhs.VolumeSeries)
	rhs.RetryLater = false

	tl.Logger().Info("case: setVolumeState updater error, state change, ConfiguredNodeID cleared")
	tl.Flush()
	fc.RetVSUpdaterObj, fc.RetVSUpdaterErr = nil, nil
	fc.RetVSUpdaterUpdateErr = fmt.Errorf("updater error")
	fc.ModVSUpdaterObj = nil
	vs := vsClone(v)
	vs.Messages = nil
	vs.VolumeSeriesState = "CONFIGURED"
	vs.ConfiguredNodeID = "nodeId"
	vr.RequestMessages = nil
	op.rhs.VolumeSeries = vs
	assert.NotPanics(func() { op.setVolumeState(ctx, "DELETING") })
	assert.NotNil(fc.ModVSUpdaterObj)
	assert.Equal([]string{"volumeSeriesState", "configuredNodeId"}, fc.InVSUpdaterItems.Set)
	assert.Equal([]string{"messages"}, fc.InVSUpdaterItems.Append)
	assert.Empty(fc.InVSUpdaterItems.Remove)
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Failed to update VolumeSeries.*: updater error", vr.RequestMessages[0].Message)
	assert.EqualValues("", vs.ConfiguredNodeID)
	assert.Equal("DELETING", vs.VolumeSeriesState)
	assert.Len(vs.Messages, 1)
	assert.Regexp("State change CONFIGURED ⇒ DELETING", vs.Messages[0].Message)
	rhs.RetryLater = false

	tl.Logger().Info("case: setVolumeState updater error, no state change, ConfiguredNodeID cleared")
	tl.Flush()
	fc.RetVSUpdaterUpdateErr = fmt.Errorf("updater error no-state-change")
	fc.ModVSUpdaterObj = nil
	vs = vsClone(v)
	vs.Messages = nil
	vs.VolumeSeriesState = "DELETING"
	vs.ConfiguredNodeID = "nodeId"
	vr.RequestMessages = nil
	op.rhs.VolumeSeries = vs
	assert.NotPanics(func() { op.setVolumeState(ctx, "DELETING") })
	assert.NotNil(fc.ModVSUpdaterObj)
	assert.Equal([]string{"volumeSeriesState", "configuredNodeId"}, fc.InVSUpdaterItems.Set)
	assert.Equal([]string{"messages"}, fc.InVSUpdaterItems.Append)
	assert.Empty(fc.InVSUpdaterItems.Remove)
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 1)
	tl.Flush()
	assert.Regexp("Failed to update VolumeSeries.*: updater error no-state-change", vr.RequestMessages[0].Message)
	assert.EqualValues("", vs.ConfiguredNodeID)
	assert.Equal("DELETING", vs.VolumeSeriesState)
	assert.Len(vs.Messages, 0)
	rhs.RetryLater = false

	tl.Logger().Info("case: setVolumeState updater error, no state change, ConfiguredNodeID already cleared")
	tl.Flush()
	fc.RetVSUpdaterUpdateErr = fmt.Errorf("updater error breakout")
	fc.ModVSUpdaterObj = nil
	vs = vsClone(v)
	vs.Messages = nil
	vs.VolumeSeriesState = "DELETING"
	vs.ConfiguredNodeID = ""
	vr.RequestMessages = nil
	op.rhs.VolumeSeries = vs
	assert.NotPanics(func() { op.setVolumeState(ctx, "DELETING") })
	assert.Nil(fc.ModVSUpdaterObj)
	assert.Equal([]string{"volumeSeriesState", "configuredNodeId"}, fc.InVSUpdaterItems.Set)
	assert.Equal([]string{"messages"}, fc.InVSUpdaterItems.Append)
	assert.Empty(fc.InVSUpdaterItems.Remove)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 0)
	assert.EqualValues("", vs.ConfiguredNodeID)
	assert.Equal("DELETING", vs.VolumeSeriesState)
	assert.Len(vs.Messages, 0)

	tl.Logger().Info("case: setVolumeState updater error, state change, ConfiguredNodeID set")
	tl.Flush()
	fc.InUVRObj, fc.RetVSUpdaterUpdateErr = nil, nil
	fc.ModVSUpdaterObj = nil
	vs = vsClone(v)
	vs.Messages = nil
	vs.VolumeSeriesState = "PROVISIONED"
	vs.ConfiguredNodeID = ""
	vs.SystemTags = nil
	vr.RequestMessages = nil
	op.rhs.VolumeSeries = vs
	assert.NotPanics(func() { op.setVolumeState(ctx, "CONFIGURED") })
	assert.NotNil(fc.ModVSUpdaterObj)
	assert.Equal([]string{"volumeSeriesState", "configuredNodeId", "systemTags"}, fc.InVSUpdaterItems.Set)
	assert.Equal([]string{"messages"}, fc.InVSUpdaterItems.Append)
	assert.Empty(fc.InVSUpdaterItems.Remove)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 1)
	tl.Flush()
	assert.Regexp("Set volume state to CONFIGURED", vr.RequestMessages[0].Message)
	assert.EqualValues("node1", vs.ConfiguredNodeID)
	assert.Equal("CONFIGURED", vs.VolumeSeriesState)
	assert.Len(vs.Messages, 1)
	assert.Regexp("State change PROVISIONED ⇒ CONFIGURED", vs.Messages[0].Message)
	assert.Equal(fmt.Sprintf("%s:node1", com.SystemTagVolumeLastConfiguredNode), vs.SystemTags[0])

	tl.Flush()
	op.rhs.VolumeSeries = v

	// ***************************** deleteLogVol
	mockCtrl.Finish()
	tl.Logger().Info("case: deleteLogVol failure")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.App.NuvoAPI = nvAPI
	vr.RequestMessages = nil
	vs.NuvoVolumeIdentifier = nuvoVolIdentifier
	vs.VolumeSeriesState = "CONFIGURED"
	nuvoErr = nuvoapi.WrapError(fmt.Errorf("destroy failure"))
	nei, ok = nuvoErr.(nuvoapi.ErrorInt)
	assert.True(ok)
	assert.False(nei.NotInitialized())
	nvAPI.EXPECT().DestroyVol(nuvoVolIdentifier, "s3", v.RootParcelUUID, true).Return(nuvoErr)
	fvu := &fvra.VolumeUpdater{}
	fvu.InSVSState = ""
	rhs.VSUpdater = fvu
	op.deleteLogVol(ctx)
	assert.True(rhs.AbortUndo)
	assert.True(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("DestroyVol failed: destroy failure", vr.RequestMessages[0].Message)
	assert.Equal("PROVISIONED", fvu.InSVSState)
	rhs.AbortUndo, rhs.InError = false, false
	rhs.VSUpdater = rhs // reset

	tl.Logger().Info("case: deleteLogVol failure (rei)")
	tl.Flush()
	c.rei.SetProperty("config-destroy-vol-error", &rei.Property{BoolValue: true})
	vr.RequestMessages = nil
	fvu = &fvra.VolumeUpdater{}
	fvu.InSVSState = ""
	rhs.VSUpdater = fvu
	op.rhs.VolumeSeries.VolumeSeriesState = "DELETING"
	op.deleteLogVol(ctx)
	assert.True(rhs.AbortUndo)
	assert.True(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 2)
	assert.Regexp("DestroyVol failed:.*config-destroy-vol-error", vr.RequestMessages[1].Message)
	rhs.AbortUndo, rhs.InError = false, false
	assert.Equal("", fvu.InSVSState)
	rhs.VSUpdater = rhs                                  // reset
	op.rhs.VolumeSeries.VolumeSeriesState = "CONFIGURED" // reset

	tl.Logger().Info("case: deleteLogVol does not exist")
	tl.Flush()
	nuvoErr = nuvoapi.WrapError(fmt.Errorf("%s", errVolumeDoesNotExist))
	nei, ok = nuvoErr.(nuvoapi.ErrorInt)
	assert.True(ok)
	assert.False(nei.NotInitialized())
	nvAPI.EXPECT().DestroyVol(nuvoVolIdentifier, "s3", v.RootParcelUUID, true).Return(nuvoErr) // ignored
	vr.RequestMessages = nil
	op.deleteLogVol(ctx)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 1)
	assert.Equal("LogVol is deleted", vr.RequestMessages[0].Message)

	tl.Logger().Info("case: deleteLogVol does not exist (rei)")
	tl.Flush()
	c.rei.SetProperty("config-destroy-vol-nfe", &rei.Property{BoolValue: true})
	vr.RequestMessages = nil
	op.deleteLogVol(ctx)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 2)
	assert.Equal("LogVol is deleted", vr.RequestMessages[1].Message)

	tl.Logger().Info("case: deleteLogVol not initialized")
	tl.Flush()
	nuvoErr = nuvoapi.WrapFatalError(fmt.Errorf("NOT INITIALIZED"))
	nei, ok = nuvoErr.(nuvoapi.ErrorInt)
	assert.True(ok)
	assert.True(nei.NotInitialized())
	nvAPI.EXPECT().DestroyVol(nuvoVolIdentifier, "s3", v.RootParcelUUID, true).Return(nuvoErr) // ignored
	vr.RequestMessages = nil
	op.deleteLogVol(ctx)
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	rhs.RetryLater = false

	tl.Logger().Info("case: temp error")
	tl.Flush()
	nuvoErr = nuvoapi.NewNuvoAPIError("tempErr", true)
	nvAPI.EXPECT().DestroyVol(nuvoVolIdentifier, "s3", v.RootParcelUUID, true).Return(nuvoErr) // ignored
	vr.RequestMessages = nil
	op.deleteLogVol(ctx)
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("tempErr", vr.RequestMessages[0].Message)
	rhs.RetryLater = false

	nvAPI.EXPECT().DestroyVol(nuvoVolIdentifier, "s3", v.RootParcelUUID, true).Return(nil)
	vr.RequestMessages = nil
	op.deleteLogVol(ctx)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 1)
	assert.Equal("LogVol is deleted", vr.RequestMessages[0].Message)
	tl.Flush()

	// ***************************** resetRootStorageID
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	fc.RetVSUpdaterObj, fc.RetVSUpdaterErr = nil, fmt.Errorf("update failure")
	fc.InVSUpdaterItems = nil
	vr.RequestMessages = nil
	assert.NotPanics(func() { op.resetRootStorageID(ctx) })
	assert.True(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Failed to update VolumeSeries.*: update failure", vr.RequestMessages[0].Message)
	assert.Equal([]string{"rootStorageId", "nuvoVolumeIdentifier"}, fc.InVSUpdaterItems.Set)
	assert.Equal([]string{"messages"}, fc.InVSUpdaterItems.Append)
	assert.Equal([]string{"systemTags"}, fc.InVSUpdaterItems.Remove)
	assert.Equal(v, rhs.VolumeSeries)
	rhs.RetryLater = false

	fc.RetVSUpdaterObj, fc.RetVSUpdaterErr = nil, nil
	fc.RetVSUpdaterUpdateErr = fmt.Errorf("updater error")
	fc.ModVSRUpdaterObj = nil
	v.RootStorageID = "s3"
	v.NuvoVolumeIdentifier = "nuvo-vol-id"
	assert.NotPanics(func() { op.resetRootStorageID(ctx) })
	assert.NotNil(fc.ModVSRUpdaterObj)
	assert.Equal([]string{"rootStorageId", "nuvoVolumeIdentifier"}, fc.InVSUpdaterItems.Set)
	assert.Equal([]string{"messages"}, fc.InVSUpdaterItems.Append)
	assert.Equal([]string{"systemTags"}, fc.InVSUpdaterItems.Remove)
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 2)
	assert.Regexp("Failed to update VolumeSeries.*: updater error", vr.RequestMessages[1].Message)
	rhs.RetryLater = false

	fc.InUVRObj, fc.RetVSUpdaterUpdateErr = nil, nil
	v.RootStorageID = "s3"
	v.NuvoVolumeIdentifier = "nuvo-vol-id"
	v.LifecycleManagementData = &models.LifecycleManagementData{FinalSnapshotNeeded: true}
	assert.NotPanics(func() { op.resetRootStorageID(ctx) })
	assert.Nil(fc.InUVRObj)
	assert.Equal([]string{"rootStorageId", "nuvoVolumeIdentifier", "lifecycleManagementData.finalSnapshotNeeded"}, fc.InVSUpdaterItems.Set)
	assert.Equal([]string{"messages"}, fc.InVSUpdaterItems.Append)
	assert.Equal([]string{"systemTags"}, fc.InVSUpdaterItems.Remove)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Len(vr.RequestMessages, 3)
	assert.Equal("Cleared rootStorageId and nuvoVolumeIdentifier", vr.RequestMessages[2].Message)
	assert.Empty(v.RootStorageID)
	assert.Empty(v.NuvoVolumeIdentifier)
	assert.Empty(v.ConfiguredNodeID)
	assert.False(v.LifecycleManagementData.FinalSnapshotNeeded)
	assert.EqualValues([]string{logVolCreatorTag, logVolCreatedTag}, v.SystemTags)
	assert.Len(v.Messages, 1)
	assert.Equal("Cleared rootStorageId and nuvoVolumeIdentifier", v.Messages[0].Message)
	assert.Equal(v, rhs.VolumeSeries)
}

func TestConfigure(t *testing.T) {
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
	c := newComponent()
	c.Init(app)
	assert.NotNil(c.Log)

	vsr := &models.VolumeSeriesRequest{}
	vsr.Meta = &models.ObjMeta{ID: "vsr-id"}

	newFakeConfigureOp := func() *fakeConfigureOp {
		op := &fakeConfigureOp{}
		op.c = c
		op.ops = op
		op.rhs = &vra.RequestHandlerState{Request: vsrClone(vsr)}
		return op
	}

	op := newFakeConfigureOp()
	op.retGIS = ConfigureDone
	expCalled := []string{"GIS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeConfigureOp()
	op.retGIS = ConfigureSetDeleting
	expCalled = []string{"GIS", "SVS-DELETING", "CLV", "QDB", "SDL", "DLV", "RID"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeConfigureOp()
	op.retGIS = ConfigureCloseDeleteLogVol
	expCalled = []string{"GIS", "CLV", "QDB", "SDL", "DLV", "RID"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeConfigureOp()
	op.sdlInError = true
	op.retGIS = ConfigureCloseDeleteLogVol
	expCalled = []string{"GIS", "CLV", "QDB", "SDL"}
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.True(op.rhs.AbortUndo)

	op = newFakeConfigureOp()
	op.retGIS = ConfigureUndoDone
	expCalled = []string{"GIS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeConfigureOp()
	op.retGIS = ConfigureSetProvisioned
	expCalled = []string{"GIS", "SVS-PROVISIONED", "CLV"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeConfigureOp()
	op.retGIS = ConfigureRestartAndOpenCreated
	expCalled = []string{"GIS", "QDB", "OPV", "APS", "SVS-CONFIGURED"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeConfigureOp()
	op.retGIS = ConfigureRestartAndCreate
	expCalled = []string{"GIS", "QDB", "NPV", "OPV", "APS", "SVS-CONFIGURED"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeConfigureOp()
	op.retGIS = ConfigureStartExisting
	expCalled = []string{"GIS", "QDB", "SDL", "OPV", "SVS-CONFIGURED"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = newFakeConfigureOp()
	op.retGIS = ConfigureStart
	expCalled = []string{"GIS", "QDB", "SDL", "SID", "NPV", "OPV", "APS", "SVS-CONFIGURED"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

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
				VolumeSeriesRequestState: "VOLUME_CONFIG",
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

	// call the real handler with an error
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc
	rhs := &vra.RequestHandlerState{
		A:            c.Animator,
		InError:      true,
		HasMount:     true,
		Request:      vr,
		VolumeSeries: v,
	}
	c.Configure(nil, rhs)
	assert.True(rhs.InError)
	assert.Empty(fc.StorageFetchID)

	// call real handler in undo
	vr.VolumeSeriesRequestState = "UNDO_VOLUME_CONFIG"
	v.RootStorageID = ""
	rhs.AbortUndo = true
	rhs.InError = true
	rhs.RetryLater = false
	c.UndoConfigure(nil, rhs)
	assert.False(rhs.RetryLater)
	assert.True(rhs.InError)
	assert.True(rhs.AbortUndo)

	// check state strings exist
	var ss configureSubState
	for ss = ConfigureStart; ss < ConfigureNoOp; ss++ {
		s := ss.String()
		tl.Logger().Debugf("Testing %d %s", ss, s)
		assert.Regexp("^Configure", s)
	}
	assert.Regexp("^configureSubState", ss.String())
}

// a matcher for StorageFetch that supports unordered calls
type storageFetchMatcher struct {
	FetchParam *storage.StorageFetchParams
}

func (o *storageFetchMatcher) Matches(x interface{}) bool {
	p := x.(*storage.StorageFetchParams)
	// avoid matching the context
	o.FetchParam.Context = p.Context
	return assert.ObjectsAreEqualValues(o.FetchParam, p)
}

func (o *storageFetchMatcher) String() string {
	return fmt.Sprintf("storageFetchMatcher matches %v", o.FetchParam)
}

type fakeConfigureOp struct {
	configureOp
	called     []string
	retGIS     configureSubState
	sdlInError bool
}

func (op *fakeConfigureOp) getInitialState(ctx context.Context) configureSubState {
	op.called = append(op.called, "GIS")
	return op.retGIS
}

func (op *fakeConfigureOp) queryConfigDB(ctx context.Context) {
	op.called = append(op.called, "QDB")
}

func (op *fakeConfigureOp) setDeviceLocations(ctx context.Context) {
	op.called = append(op.called, "SDL")
	op.rhs.InError = op.sdlInError
}

func (op *fakeConfigureOp) setRootStorageID(ctx context.Context) {
	op.called = append(op.called, "SID")
}

func (op *fakeConfigureOp) createLogVol(ctx context.Context) {
	op.called = append(op.called, "NPV")
}

func (op *fakeConfigureOp) openLogVol(ctx context.Context) {
	op.called = append(op.called, "OPV")
}

func (op *fakeConfigureOp) allocParcels(ctx context.Context) {
	op.called = append(op.called, "APS")
}

func (op *fakeConfigureOp) setVolumeState(ctx context.Context, newState string) {
	op.called = append(op.called, "SVS-"+newState)
}

func (op *fakeConfigureOp) closeLogVol(ctx context.Context) {
	op.called = append(op.called, "CLV")
}

func (op *fakeConfigureOp) deleteLogVol(ctx context.Context) {
	op.called = append(op.called, "DLV")
}

func (op *fakeConfigureOp) resetRootStorageID(ctx context.Context) {
	op.called = append(op.called, "RID")
}
