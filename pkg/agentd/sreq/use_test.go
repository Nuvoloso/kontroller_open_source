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
	"fmt"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	fAppServant "github.com/Nuvoloso/kontroller/pkg/agentd/fake"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	mockcsp "github.com/Nuvoloso/kontroller/pkg/csp/mock"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	mockNuvo "github.com/Nuvoloso/kontroller/pkg/nuvoapi/mock"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestUseSteps(t *testing.T) {
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
	fAS := &fAppServant.AppServant{}
	app.AppServant = fAS
	ctx := context.Background()
	c := &SRComp{}
	c.Init(app)
	c.thisNodeID = "THIS-NODE"
	c.thisNodeIdentifier = "ThisNode"
	fc := &fake.Client{}
	c.oCrud = fc

	sr := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "STORAGE-REQ-1",
			},
		},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			ParcelSizeBytes: swag.Int64(1024),
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
				StorageRequestState: "NEW",
			},
			StorageRequestCreateMutable: models.StorageRequestCreateMutable{
				NodeID:    "THIS-NODE",
				StorageID: "STORAGE-1",
			},
		},
	}
	s := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta:           &models.ObjMeta{ID: "STORAGE-1"},
			CspStorageType: "Amazon gp2",
		},
		StorageMutable: models.StorageMutable{
			StorageState: &models.StorageStateMutable{
				AttachedNodeID:     "THIS-NODE",
				AttachedNodeDevice: "/dev/storage",
				DeviceState:        "UNUSED",
				MediaState:         "FORMATTED",
				Messages:           []*models.TimestampedString{},
			},
		},
	}
	ss := s.StorageState
	rhs := &requestHandlerState{
		c:       c,
		Request: sr,
		Storage: s,
	}
	op := &useOp{
		c:   c,
		rhs: rhs,
	}

	//  ***************************** getInitialState

	// USE
	op.rhs.HasClose = false
	ss.DeviceState = "UNUSED"
	ss.MediaState = "UNFORMATTED"
	op.rhs.RetryLater = false
	op.rhs.InError = false
	assert.Equal(UseError, op.getInitialState(ctx))
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("not formatted", op.rhs.Request.RequestMessages[0].Message)

	ss.DeviceState = "OPENING"
	ss.MediaState = "FORMATTED"
	op.rhs.RetryLater = false
	op.rhs.InError = false
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	assert.Equal(UseStorage, op.getInitialState(ctx))
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	ss.DeviceState = "UNUSED"
	assert.Equal(UseSetDeviceStateOpening, op.getInitialState(ctx))
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	// CLOSE
	op.rhs.Request.StorageRequestState = com.StgReqStateClosing
	ss.DeviceState = "USED"
	op.rhs.RetryLater = false
	op.rhs.InError = false
	assert.Equal(CloseSetDeviceStateClosing, op.getInitialState(ctx))
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	ss.DeviceState = "CLOSING"
	assert.Equal(CloseStorage, op.getInitialState(ctx))
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	//  ***************************** setDeviceState
	fc.RetUSErr = fmt.Errorf("update-storage-error")
	ss.DeviceState = "UNUSED"
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cspAPI := mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cspAPI.EXPECT().LocalInstanceDeviceName(s.StorageIdentifier, s.StorageState.AttachedNodeDevice).Return(s.StorageState.AttachedNodeDevice, nil)
	app.CSP = cspAPI
	op.setDeviceState(ctx, com.StgDeviceStateOpening)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("update-storage-error", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal("FORMATTED", ss.MediaState)
	assert.Equal("OPENING", ss.DeviceState)
	assert.NotNil(fc.InUSitems)
	assert.Equal([]string{"storageState"}, fc.InUSitems.Set)
	assert.Empty(fc.InUSitems.Append)
	assert.Empty(fc.InUSitems.Remove)

	// LocalInstanceDeviceName error
	ss.DeviceState = "UNUSED"
	ss.Messages = []*models.TimestampedString{}
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	fc.PassThroughUSObj = true
	fc.InUSitems = nil
	cspAPI.EXPECT().LocalInstanceDeviceName(s.StorageIdentifier, s.StorageState.AttachedNodeDevice).Return("", fmt.Errorf("lidError"))
	op.setDeviceState(ctx, com.StgDeviceStateOpening)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("lidError", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal("UNUSED", ss.DeviceState)

	// Success, includes AttachedNodeDevice update
	ss.DeviceState = "UNUSED"
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	fc.PassThroughUSObj = true
	fc.InUSitems = nil
	cspAPI.EXPECT().LocalInstanceDeviceName(s.StorageIdentifier, s.StorageState.AttachedNodeDevice).Return("/dev/nvme2n1", nil)
	op.setDeviceState(ctx, com.StgDeviceStateOpening)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal("/dev/nvme2n1", ss.AttachedNodeDevice)
	assert.Equal("FORMATTED", ss.MediaState)
	assert.Equal("OPENING", ss.DeviceState)
	assert.NotNil(fc.InUSitems)
	assert.Equal([]string{"storageState"}, fc.InUSitems.Set)
	assert.Empty(fc.InUSitems.Append)
	assert.Empty(fc.InUSitems.Remove)
	ss.AttachedNodeDevice = "/dev/storage" // reset

	//  ***************************** useDevice
	c.rei.SetProperty("nuvo-use-device", &rei.Property{BoolValue: true})
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	op.useDevice(ctx)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("nuvo-use-device", op.rhs.Request.RequestMessages[0].Message)

	c.rei.SetProperty("block-nuvo-use-device", &rei.Property{BoolValue: true})
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	op.useDevice(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("block-nuvo-use-device", op.rhs.Request.RequestMessages[0].Message)

	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	fc.InUSitems = nil
	fc.PassThroughUSObj = false // not called in failure path
	fc.RetUSErr = fmt.Errorf("update-storage-error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cspAPI = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cspAPI.EXPECT().GetDeviceTypeByCspStorageType(gomock.Any()).Return("SSD", nil)
	app.CSP = cspAPI
	nvAPI := mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	nvAPI.EXPECT().UseDevice(string(s.Meta.ID), ss.AttachedNodeDevice, "SSD").Return(fmt.Errorf("use-error"))
	op.useDevice(ctx)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("use-error", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal("FORMATTED", ss.MediaState)
	assert.Equal("OPENING", ss.DeviceState)
	assert.Nil(fc.InUSitems) // not called in failure path

	ss.DeviceState = "OPENING"
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	fc.RetUSErr = fmt.Errorf("update-storage-error")
	fc.InUSitems = nil
	fc.PassThroughUSObj = false
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cspAPI = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cspAPI.EXPECT().GetDeviceTypeByCspStorageType(gomock.Any()).Return("SSD", nil)
	app.CSP = cspAPI
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	nvAPI.EXPECT().UseDevice(string(s.Meta.ID), ss.AttachedNodeDevice, "SSD").Return(nil)
	op.useDevice(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("update-storage-error", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal("FORMATTED", ss.MediaState)
	assert.Equal("OPEN", ss.DeviceState)
	assert.NotNil(fc.InUSitems)
	assert.Equal([]string{"storageState.deviceState"}, fc.InUSitems.Set)
	assert.Empty(fc.InUSitems.Append)
	assert.Empty(fc.InUSitems.Remove)

	ss.DeviceState = "OPENING"
	ss.MediaState = "FORMATTED"
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	fc.InUSitems = nil
	fc.PassThroughUSObj = true
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cspAPI = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cspAPI.EXPECT().GetDeviceTypeByCspStorageType(gomock.Any()).Return("SSD", nil)
	app.CSP = cspAPI
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	nvAPI.EXPECT().UseDevice(string(s.Meta.ID), ss.AttachedNodeDevice, "SSD").Return(nil)
	op.useDevice(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal("FORMATTED", ss.MediaState)
	assert.Equal("OPEN", ss.DeviceState)
	assert.NotNil(fc.InUSitems)
	assert.Equal([]string{"storageState.deviceState"}, fc.InUSitems.Set)
	assert.Empty(fc.InUSitems.Append)
	assert.Empty(fc.InUSitems.Remove)
	assert.NotNil(fAS.InAddStorageS)
	assert.NoError(fAS.InAddStorageS.Validate())

	ss.DeviceState = "OPENING"
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	nErr := nuvoapi.NewNuvoAPIError("tempErr", true)
	cspAPI = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cspAPI.EXPECT().GetDeviceTypeByCspStorageType(gomock.Any()).Return("SSD", nil)
	app.CSP = cspAPI
	nvAPI.EXPECT().UseDevice(string(s.Meta.ID), ss.AttachedNodeDevice, "SSD").Return(nErr)
	op.useDevice(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("tempErr", op.rhs.Request.RequestMessages[0].Message)

	ss.DeviceState = "OPENING"
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cspAPI = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cspAPI.EXPECT().GetDeviceTypeByCspStorageType(gomock.Any()).Return("", fmt.Errorf("dev-type-error"))
	app.CSP = cspAPI
	op.useDevice(ctx)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("dev-type-error", op.rhs.Request.RequestMessages[0].Message)

	//  ***************************** closeDevice
	c.rei.SetProperty("nuvo-close-device", &rei.Property{BoolValue: true})
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	op.closeDevice(ctx)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("nuvo-close-device", op.rhs.Request.RequestMessages[0].Message)

	c.rei.SetProperty("block-nuvo-close-device", &rei.Property{BoolValue: true})
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	op.closeDevice(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("block-nuvo-close-device", op.rhs.Request.RequestMessages[0].Message)

	initDeviceState := ss.DeviceState
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	fc.InUSitems = nil
	fc.PassThroughUSObj = false // not called in failure path
	fc.RetUSErr = fmt.Errorf("update-storage-error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	app.CSP = nil
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	nvAPI.EXPECT().CloseDevice(string(s.Meta.ID)).Return(fmt.Errorf("close-error"))
	op.closeDevice(ctx)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("close-error", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal(initDeviceState, ss.DeviceState)
	assert.Nil(fc.InUSitems) // not called in failure path

	ss.DeviceState = "CLOSING"
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	fc.RetUSErr = fmt.Errorf("update-storage-error")
	fc.InUSitems = nil
	fc.PassThroughUSObj = false
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	nvAPI.EXPECT().CloseDevice(string(s.Meta.ID)).Return(nil)
	op.closeDevice(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("update-storage-error", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal("UNUSED", ss.DeviceState)
	assert.NotNil(fc.InUSitems)
	assert.Equal([]string{"storageState.deviceState"}, fc.InUSitems.Set)
	assert.Empty(fc.InUSitems.Append)
	assert.Empty(fc.InUSitems.Remove)

	ss.DeviceState = "CLOSING"
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	fc.InUSitems = nil
	fc.PassThroughUSObj = true
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	nvAPI.EXPECT().CloseDevice(string(s.Meta.ID)).Return(nil)
	op.closeDevice(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal("UNUSED", ss.DeviceState)
	assert.NotNil(fc.InUSitems)
	assert.Equal([]string{"storageState.deviceState"}, fc.InUSitems.Set)
	assert.Empty(fc.InUSitems.Append)
	assert.Empty(fc.InUSitems.Remove)
	assert.Equal(string(s.Meta.ID), fAS.InRemoveStorageID)

	ss.DeviceState = "CLOSING"
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	fc.InUSitems = nil
	fc.PassThroughUSObj = true
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	nvAPI.EXPECT().CloseDevice(string(s.Meta.ID)).Return(fmt.Errorf("%s", errDeviceNotFound))
	op.closeDevice(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal("UNUSED", ss.DeviceState)
	assert.NotNil(fc.InUSitems)
	assert.Equal([]string{"storageState.deviceState"}, fc.InUSitems.Set)
	assert.Empty(fc.InUSitems.Append)
	assert.Empty(fc.InUSitems.Remove)
	assert.Equal(string(s.Meta.ID), fAS.InRemoveStorageID)

	ss.DeviceState = "CLOSING"
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	nErr = nuvoapi.NewNuvoAPIError("tempErr", true)
	nvAPI.EXPECT().CloseDevice(string(s.Meta.ID)).Return(nErr)
	op.closeDevice(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("tempErr", op.rhs.Request.RequestMessages[0].Message)
}

func TestUseStorage(t *testing.T) {
	assert := assert.New(t)

	// USE
	op := &fakeUseOps{}
	op.ops = op
	op.rhs = &requestHandlerState{HasClose: false}
	op.retGIS = UseSetDeviceStateOpening
	expCalled := []string{"GIS", "SDS", "UD"}
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.Equal(com.StgDeviceStateOpen, op.sdsState)

	op = &fakeUseOps{}
	op.ops = op
	op.rhs = &requestHandlerState{HasClose: false}
	op.sdsRetryLater = true
	op.retGIS = UseSetDeviceStateOpening
	expCalled = []string{"GIS", "SDS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.True(op.rhs.RetryLater)

	op = &fakeUseOps{}
	op.ops = op
	op.rhs = &requestHandlerState{HasClose: false}
	op.sdsInError = true
	op.retGIS = UseSetDeviceStateOpening
	expCalled = []string{"GIS", "SDS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.True(op.rhs.InError)

	// CLOSE
	op = &fakeUseOps{}
	op.ops = op
	op.rhs = &requestHandlerState{HasClose: true}
	op.retGIS = CloseSetDeviceStateClosing
	expCalled = []string{"GIS", "SDS", "CD"}
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.Equal(com.StgDeviceStateUnused, op.sdsState)

	op = &fakeUseOps{}
	op.ops = op
	op.rhs = &requestHandlerState{HasClose: true}
	op.sdsRetryLater = true
	op.retGIS = CloseSetDeviceStateClosing
	expCalled = []string{"GIS", "SDS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.True(op.rhs.RetryLater)

	op = &fakeUseOps{}
	op.ops = op
	op.rhs = &requestHandlerState{HasClose: true}
	op.sdsInError = true
	op.retGIS = CloseSetDeviceStateClosing
	expCalled = []string{"GIS", "SDS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.True(op.rhs.InError)

	// call the real entry point
	sr := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "STORAGE-REQ-1",
			},
		},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			ParcelSizeBytes: swag.Int64(1024),
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
				StorageRequestState: "NEW",
			},
			StorageRequestCreateMutable: models.StorageRequestCreateMutable{
				NodeID:    "THIS-NODE",
				StorageID: "STORAGE-1",
			},
		},
	}
	s := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta: &models.ObjMeta{ID: "STORAGE-1"},
		},
		StorageMutable: models.StorageMutable{
			StorageState: &models.StorageStateMutable{
				AttachedNodeID:     "THIS-NODE",
				AttachedNodeDevice: "/dev/storage",
				DeviceState:        "UNUSED",
				MediaState:         "UNFORMATTED",
				Messages:           []*models.TimestampedString{},
			},
		},
	}
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	c := &SRComp{}
	c.Init(app)
	c.thisNodeID = "THIS-NODE"
	c.thisNodeIdentifier = "ThisNode"
	rhs := &requestHandlerState{
		c:       c,
		Request: sr,
		Storage: s,
		InError: true, // to break out
	}
	c.UseStorage(nil, rhs)
	c.CloseStorage(nil, rhs)
}

type fakeUseOps struct {
	useOp
	called        []string
	retGIS        useSubState
	sdsRetryLater bool
	sdsInError    bool
	sdsState      string
}

func (op *fakeUseOps) getInitialState(ctx context.Context) useSubState {
	op.called = append(op.called, "GIS")
	return op.retGIS
}

func (op *fakeUseOps) setDeviceState(ctx context.Context, state string) {
	if op.sdsRetryLater {
		op.rhs.RetryLater = true
	}
	if op.sdsInError {
		op.rhs.InError = true
	}
	op.sdsState = state
	op.called = append(op.called, "SDS")
}

func (op *fakeUseOps) useDevice(ctx context.Context) {
	op.sdsState = com.StgDeviceStateOpen
	op.called = append(op.called, "UD")
}

func (op *fakeUseOps) closeDevice(ctx context.Context) {
	op.sdsState = com.StgDeviceStateUnused
	op.called = append(op.called, "CD")
}
