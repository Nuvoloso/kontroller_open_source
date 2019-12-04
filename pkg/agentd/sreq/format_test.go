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
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	mockcsp "github.com/Nuvoloso/kontroller/pkg/csp/mock"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	mockNuvo "github.com/Nuvoloso/kontroller/pkg/nuvoapi/mock"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/docker/go-units"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestFormatSteps(t *testing.T) {
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
			ParcelSizeBytes: swag.Int64(852 * units.MiB),
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
			Meta:      &models.ObjMeta{ID: "STORAGE-1"},
			SizeBytes: swag.Int64(10 * units.GiB),
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
	ss := s.StorageState
	rhs := &requestHandlerState{
		c:       c,
		Request: sr,
		Storage: s,
	}
	op := &formatOp{
		c:   c,
		rhs: rhs,
	}

	//  ***************************** getInitialState
	ss.DeviceState = "FORMATTING"
	assert.Equal(FormatStorage, op.getInitialState(ctx))
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	ss.DeviceState = "UNUSED"
	assert.Equal(FormatSetDeviceState, op.getInitialState(ctx))
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	//  ***************************** FormatSetDeviceState
	fc.RetUSErr = fmt.Errorf("update-storage-error")
	ss.DeviceState = "UNUSED"
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cspAPI := mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cspAPI.EXPECT().LocalInstanceDeviceName(s.StorageIdentifier, s.StorageState.AttachedNodeDevice).Return(s.StorageState.AttachedNodeDevice, nil)
	app.CSP = cspAPI
	op.setDeviceState(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("update-storage-error", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal("FORMATTING", ss.DeviceState)
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
	op.setDeviceState(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("lidError", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal("UNUSED", ss.DeviceState)

	// Success, includes AttachedNodeDevice update
	ss.DeviceState = "UNUSED"
	ss.Messages = []*models.TimestampedString{}
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	fc.PassThroughUSObj = true
	fc.InUSitems = nil
	cspAPI.EXPECT().LocalInstanceDeviceName(s.StorageIdentifier, s.StorageState.AttachedNodeDevice).Return("/dev/nvme2n1", nil)
	op.setDeviceState(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(ss.Messages, 1)
	assert.Regexp("deviceState.* ⇒ FORMATTING", ss.Messages[0].Message)
	assert.Equal("/dev/nvme2n1", ss.AttachedNodeDevice)
	assert.Equal("FORMATTING", ss.DeviceState)
	assert.NotNil(fc.InUSitems)
	assert.Equal([]string{"storageState"}, fc.InUSitems.Set)
	assert.Empty(fc.InUSitems.Append)
	assert.Empty(fc.InUSitems.Remove)
	app.CSP = nil
	ss.AttachedNodeDevice = "/dev/storage" // reset

	//  ***************************** FormatStorage
	ss.Messages = []*models.TimestampedString{}
	ss.DeviceState = "FORMATTING"
	ss.MediaState = "FORMATTED" // to test resetting
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	fc.InUSitems = nil
	fc.PassThroughUSObj = false // not called in failure path
	fc.RetUSErr = fmt.Errorf("update-storage-error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI := mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	pSz := uint64(swag.Int64Value(op.rhs.Request.ParcelSizeBytes))
	nvAPI.EXPECT().FormatDevice(string(s.Meta.ID), ss.AttachedNodeDevice, pSz).Return(uint64(0), fmt.Errorf("format-error"))
	op.formatDevice(ctx)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("format-error", op.rhs.Request.RequestMessages[0].Message)
	assert.Len(ss.Messages, 2)
	assert.Regexp("format-error", ss.Messages[0].Message)
	assert.Regexp("mediaState.* ⇒ UNFORMATTED", ss.Messages[1].Message)
	assert.Equal("UNFORMATTED", ss.MediaState)
	assert.Equal("FORMATTING", ss.DeviceState)
	assert.Nil(fc.InUSitems) // not called in failure path

	s.TotalParcelCount = swag.Int64(0)
	s.AvailableBytes = s.SizeBytes
	ss.DeviceState = "FORMATTING"
	ss.MediaState = "UNFORMATTED"
	ss.Messages = []*models.TimestampedString{}
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
	nvAPI.EXPECT().FormatDevice(string(s.Meta.ID), ss.AttachedNodeDevice, pSz).Return(uint64(1000), nil)
	op.formatDevice(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("update-storage-error", op.rhs.Request.RequestMessages[0].Message)
	assert.Len(ss.Messages, 3)
	assert.Regexp("mediaState.* ⇒ FORMATTED", ss.Messages[0].Message)
	assert.Regexp("Computed .availableBytes.*parcelSizeBytes", ss.Messages[1].Message)
	assert.Regexp("deviceState.* ⇒ UNUSED", ss.Messages[2].Message)
	assert.Equal("FORMATTED", ss.MediaState)
	assert.Equal("UNUSED", ss.DeviceState)
	assert.NotNil(fc.InUSitems)
	assert.Equal([]string{"storageState", "totalParcelCount", "parcelSizeBytes", "availableBytes"}, fc.InUSitems.Set)
	assert.Empty(fc.InUSitems.Append)
	assert.Empty(fc.InUSitems.Remove)
	assert.Equal(int64(1000), swag.Int64Value(s.TotalParcelCount))
	assert.Equal(int64(pSz*1000), swag.Int64Value(s.AvailableBytes))

	// HasUse is false, also test fallback TotalParcelCount computation
	s.TotalParcelCount = swag.Int64(0)
	s.AvailableBytes = s.SizeBytes
	ss.DeviceState = "FORMATTING"
	ss.MediaState = "UNFORMATTED"
	ss.Messages = []*models.TimestampedString{}
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	op.rhs.HasUse = false
	fc.InUSitems = nil
	fc.PassThroughUSObj = true
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	nvAPI.EXPECT().FormatDevice(string(s.Meta.ID), ss.AttachedNodeDevice, pSz).Return(uint64(0), nil)
	op.formatDevice(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(ss.Messages, 3)
	assert.Regexp("mediaState.* ⇒ FORMATTED", ss.Messages[0].Message)
	assert.Regexp("Computed .availableBytes.*parcelSizeBytes", ss.Messages[1].Message)
	assert.Regexp("deviceState.* ⇒ UNUSED", ss.Messages[2].Message)
	assert.Equal("FORMATTED", ss.MediaState)
	assert.Equal("UNUSED", ss.DeviceState)
	assert.NotNil(fc.InUSitems)
	assert.Equal([]string{"storageState", "totalParcelCount", "parcelSizeBytes", "availableBytes"}, fc.InUSitems.Set)
	assert.Empty(fc.InUSitems.Append)
	assert.Empty(fc.InUSitems.Remove)
	assert.EqualValues(12, swag.Int64Value(s.TotalParcelCount))
	assert.Equal(swag.Int64Value(op.rhs.Request.ParcelSizeBytes), swag.Int64Value(s.ParcelSizeBytes))
	assert.Equal(int64(pSz*12), swag.Int64Value(s.AvailableBytes))

	s.TotalParcelCount = swag.Int64(0)
	s.AvailableBytes = s.SizeBytes
	ss.DeviceState = "FORMATTING"
	ss.MediaState = "UNFORMATTED"
	ss.Messages = []*models.TimestampedString{}
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	op.rhs.HasUse = true
	fc.InUSitems = nil
	fc.PassThroughUSObj = true
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	nvAPI.EXPECT().FormatDevice(string(s.Meta.ID), ss.AttachedNodeDevice, pSz).Return(uint64(12345), nil)
	op.formatDevice(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(ss.Messages, 3)
	assert.Regexp("mediaState.* ⇒ FORMATTED", ss.Messages[0].Message)
	assert.Regexp("Computed .availableBytes.*parcelSizeBytes", ss.Messages[1].Message)
	assert.Regexp("deviceState.* ⇒ OPENING", ss.Messages[2].Message)
	assert.Equal("FORMATTED", ss.MediaState)
	assert.Equal("OPENING", ss.DeviceState)
	assert.NotNil(fc.InUSitems)
	assert.Equal([]string{"storageState", "totalParcelCount", "parcelSizeBytes", "availableBytes"}, fc.InUSitems.Set)
	assert.Empty(fc.InUSitems.Append)
	assert.Empty(fc.InUSitems.Remove)
	assert.Equal(int64(12345), swag.Int64Value(s.TotalParcelCount))
	assert.Equal(int64(pSz*12345), swag.Int64Value(s.AvailableBytes))
	assert.Equal(swag.Int64Value(op.rhs.Request.ParcelSizeBytes), swag.Int64Value(s.ParcelSizeBytes))

	s.TotalParcelCount = swag.Int64(0)
	s.AvailableBytes = s.SizeBytes
	ss.DeviceState = "FORMATTING"
	ss.MediaState = "UNFORMATTED"
	ss.Messages = []*models.TimestampedString{}
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	nErr := nuvoapi.NewNuvoAPIError("tempErr", true)
	nvAPI.EXPECT().FormatDevice(string(s.Meta.ID), ss.AttachedNodeDevice, pSz).Return(uint64(1000), nErr)
	op.formatDevice(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("tempErr", op.rhs.Request.RequestMessages[0].Message)

	// rei error
	c.rei.SetProperty("format-tmp-error", &rei.Property{BoolValue: true})
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	op.formatDevice(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("format-tmp-error", op.rhs.Request.RequestMessages[0].Message)
}

func TestFormatStorage(t *testing.T) {
	assert := assert.New(t)

	op := &fakeFormatOps{}
	op.ops = op
	op.rhs = &requestHandlerState{}
	op.retGIS = FormatSetDeviceState
	expCalled := []string{"GIS", "SDS", "FD"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakeFormatOps{}
	op.ops = op
	op.rhs = &requestHandlerState{}
	op.sdsRetryLater = true
	op.retGIS = FormatSetDeviceState
	expCalled = []string{"GIS", "SDS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.True(op.rhs.RetryLater)

	op = &fakeFormatOps{}
	op.ops = op
	op.rhs = &requestHandlerState{}
	op.sdsInError = true
	op.retGIS = FormatSetDeviceState
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
	c.FormatStorage(nil, rhs)
}

type fakeFormatOps struct {
	formatOp
	called        []string
	retGIS        formatSubState
	sdsRetryLater bool
	sdsInError    bool
}

func (op *fakeFormatOps) getInitialState(ctx context.Context) formatSubState {
	op.called = append(op.called, "GIS")
	return op.retGIS
}

func (op *fakeFormatOps) setDeviceState(ctx context.Context) {
	if op.sdsRetryLater {
		op.rhs.RetryLater = true
	}
	if op.sdsInError {
		op.rhs.InError = true
	}
	op.called = append(op.called, "SDS")
}

func (op *fakeFormatOps) formatDevice(ctx context.Context) {
	op.called = append(op.called, "FD")
}
