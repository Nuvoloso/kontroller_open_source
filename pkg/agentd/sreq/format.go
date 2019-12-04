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
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

// formatter sub-states
// There is at most one database update in each sub-state.
type formatSubState int

// formatSubState values
const (
	// Set the deviceState of the Storage object to FORMATTING
	FormatSetDeviceState formatSubState = iota
	// Call Storelandia to format the device then update the Storage object
	FormatStorage
)

type formatOp struct {
	c   *SRComp
	rhs *requestHandlerState
	ops formatOperators
}

type formatOperators interface {
	getInitialState(ctx context.Context) formatSubState
	setDeviceState(ctx context.Context)
	formatDevice(ctx context.Context)
}

// FormatStorage formats a Storage object for future use by Storelandia
func (c *SRComp) FormatStorage(ctx context.Context, rhs *requestHandlerState) {
	op := &formatOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

func (op *formatOp) run(ctx context.Context) {
out:
	for ss := op.ops.getInitialState(ctx); !(op.rhs.InError || op.rhs.RetryLater); ss++ {
		switch ss {
		case FormatSetDeviceState:
			op.ops.setDeviceState(ctx)
		case FormatStorage:
			op.ops.formatDevice(ctx)
		default:
			break out
		}
	}
}

// getInitialState determines the initial sub-state.
func (op *formatOp) getInitialState(ctx context.Context) formatSubState {
	ss := op.rhs.Storage.StorageState
	if ss.DeviceState == com.StgDeviceStateFormatting {
		return FormatStorage
	}
	return FormatSetDeviceState
}

func (op *formatOp) setDeviceState(ctx context.Context) {
	sObj := op.rhs.Storage
	ss := sObj.StorageState
	// AttachedNodeDevice can change on nodes with NVMe; update to the current value, no-op for other nodes
	nodeDevice, err := op.c.app.CSP.LocalInstanceDeviceName(sObj.StorageIdentifier, ss.AttachedNodeDevice)
	if err != nil {
		op.rhs.setRequestMessage("failed to look up instance device name: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	ss.AttachedNodeDevice = nodeDevice
	ss.Messages = append(ss.Messages, &models.TimestampedString{
		Message: fmt.Sprintf("State change: deviceState %s ⇒ %s", ss.DeviceState, com.StgDeviceStateFormatting),
		Time:    strfmt.DateTime(time.Now()),
	})
	ss.DeviceState = com.StgDeviceStateFormatting
	items := &crud.Updates{Set: []string{"storageState"}}
	obj, err := op.c.oCrud.StorageUpdate(ctx, sObj, items)
	if err != nil {
		op.rhs.setRequestMessage("failed to update Storage object: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.rhs.Storage = obj
}

func (op *formatOp) formatDevice(ctx context.Context) {
	if err := op.c.rei.ErrOnBool("format-tmp-error"); err != nil {
		op.rhs.setAndUpdateRequestMessageDistinct(ctx, "inject error: %s", err.Error())
		op.rhs.RetryLater = true // we can attempt format again
		return
	}
	sObj := op.rhs.Storage
	ss := sObj.StorageState
	sUUID := string(sObj.Meta.ID)
	devName := ss.AttachedNodeDevice
	pSz := uint64(swag.Int64Value(op.rhs.Request.ParcelSizeBytes))
	op.c.Log.Debugf("StorageRequest %s: NUVOAPI FormatDevice(%s, %s, %d)", op.rhs.Request.Meta.ID, sUUID, devName, pSz)
	tpc, err := op.c.app.NuvoAPI.FormatDevice(sUUID, devName, pSz)
	now := time.Now()
	if err != nil {
		if nuvoapi.ErrorIsTemporary(err) {
			op.rhs.setRequestMessage("NUVOAPI temporary error: %s", err.Error())
			op.rhs.RetryLater = true // we can attempt format again
			return
		}
		op.rhs.setRequestError("NUVOAPI FormatDevice failed: %s", err.Error())
		ss.Messages = append(ss.Messages, &models.TimestampedString{
			Message: fmt.Sprintf("FORMAT failed [node=%s device=%s pSz=%d]: %s", op.c.thisNodeIdentifier, devName, pSz, err.Error()),
			Time:    strfmt.DateTime(now),
		})
		if ss.MediaState != com.StgMediaStateUnformatted {
			ss.Messages = append(ss.Messages, &models.TimestampedString{
				Message: fmt.Sprintf("State change: mediaState %s ⇒ %s", ss.MediaState, com.StgMediaStateUnformatted),
				Time:    strfmt.DateTime(now),
			})
			ss.MediaState = com.StgMediaStateUnformatted
		}
		// animator will set the Storage object DeviceState to UNUSED in termination failure logic
		return
	}
	items := &crud.Updates{Set: []string{"storageState", "totalParcelCount", "parcelSizeBytes", "availableBytes"}}
	if ss.MediaState != com.StgMediaStateFormatted {
		ss.Messages = append(ss.Messages, &models.TimestampedString{
			Message: fmt.Sprintf("State change: mediaState %s ⇒ %s", ss.MediaState, com.StgMediaStateFormatted),
			Time:    strfmt.DateTime(now),
		})
		ss.MediaState = com.StgMediaStateFormatted
	}
	// TBD remove this computation when the NuvoAPI returns a valid TotalParcelCount
	if tpc == 0 {
		tpc = uint64((swag.Int64Value(sObj.SizeBytes) - nuvoapi.DefaultDeviceFormatOverheadBytes) / swag.Int64Value(op.rhs.Request.ParcelSizeBytes))
	}
	sObj.TotalParcelCount = swag.Int64(int64(tpc))
	sObj.AvailableBytes = swag.Int64(int64(tpc) * swag.Int64Value(op.rhs.Request.ParcelSizeBytes))
	sObj.ParcelSizeBytes = op.rhs.Request.ParcelSizeBytes
	ss.Messages = append(ss.Messages, &models.TimestampedString{
		Message: fmt.Sprintf("Computed [availableBytes=%d] based on [parcelSizeBytes=%d]", swag.Int64Value(sObj.AvailableBytes), pSz),
		Time:    strfmt.DateTime(now),
	})
	nextDeviceState := com.StgDeviceStateUnused
	if op.rhs.HasUse {
		nextDeviceState = com.StgDeviceStateOpening // look ahead so that a cleaner state transition is recorded
	}
	ss.Messages = append(ss.Messages, &models.TimestampedString{
		Message: fmt.Sprintf("State change: deviceState %s ⇒ %s", ss.DeviceState, nextDeviceState),
		Time:    strfmt.DateTime(now),
	})
	ss.DeviceState = nextDeviceState
	obj, err := op.c.oCrud.StorageUpdate(ctx, sObj, items)
	if err != nil {
		op.rhs.setRequestMessage("failed to update Storage object: %s", err.Error())
		op.rhs.RetryLater = true // we can attempt format again
		return
	}
	op.rhs.Storage = obj
}
