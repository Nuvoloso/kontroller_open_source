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

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
)

// well-known error messages returned by Storelandia
const (
	errDeviceNotFound = "DEVICE_NOT_FOUND: Cannot find device"
)

// use sub-states
// There is at most one database update in each sub-state.
type useSubState int

// useSubState values
const (
	// Set the deviceState of the Storage object to OPENING
	UseSetDeviceStateOpening useSubState = iota
	// Call Storelandia to use the device then update the Storage object
	UseStorage
	// Return
	UseDone
	// Set the deviceState of the Storage object to CLOSING
	CloseSetDeviceStateClosing
	// Call Storelandia to close the device then update the Storage object
	CloseStorage
	// There is an error
	UseError
)

type useOp struct {
	c   *SRComp
	rhs *requestHandlerState
	ops useOperators
}

type useOperators interface {
	getInitialState(ctx context.Context) useSubState
	setDeviceState(ctx context.Context, state string)
	useDevice(ctx context.Context)
	closeDevice(ctx context.Context)
}

// UseStorage enables use of a Storage object by Storelandia
func (c *SRComp) UseStorage(ctx context.Context, rhs *requestHandlerState) {
	op := &useOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// CloseStorage tells Storelandia to close a device.
func (c *SRComp) CloseStorage(ctx context.Context, rhs *requestHandlerState) {
	op := &useOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

func (op *useOp) run(ctx context.Context) {
out:
	for ss := op.ops.getInitialState(ctx); !(op.rhs.InError || op.rhs.RetryLater); ss++ {
		switch ss {
		// USE
		case UseSetDeviceStateOpening:
			op.ops.setDeviceState(ctx, com.StgDeviceStateOpening)
		case UseStorage:
			op.ops.useDevice(ctx)

		// CLOSE
		case CloseSetDeviceStateClosing:
			op.ops.setDeviceState(ctx, com.StgDeviceStateClosing)
		case CloseStorage:
			op.ops.closeDevice(ctx)
		default:
			break out
		}
	}
}

// getInitialState determines the initial sub-state.
func (op *useOp) getInitialState(ctx context.Context) useSubState {
	// Note: UseDevice/CloseDevice is supposed to be idempotent and can be re-issued even if already open/closed
	ss := op.rhs.Storage.StorageState
	if op.rhs.Request.StorageRequestState == com.StgReqStateClosing {
		if ss.DeviceState == com.StgDeviceStateClosing {
			return CloseStorage
		}
		return CloseSetDeviceStateClosing
	}
	if ss.MediaState != com.StgMediaStateFormatted {
		op.rhs.setRequestError("storage is not formatted")
		return UseError
	}
	if ss.DeviceState == com.StgDeviceStateOpening {
		return UseStorage
	}
	return UseSetDeviceStateOpening
}

func (op *useOp) setDeviceState(ctx context.Context, state string) {
	sObj := op.rhs.Storage
	ss := sObj.StorageState
	// AttachedNodeDevice can change on nodes with NVMe; update to the current value, no-op for other nodes
	nodeDevice, err := op.c.app.CSP.LocalInstanceDeviceName(sObj.StorageIdentifier, ss.AttachedNodeDevice)
	if err != nil {
		op.rhs.setRequestMessage("failed to look up device name: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	ss.AttachedNodeDevice = nodeDevice
	ss.DeviceState = state
	items := &crud.Updates{Set: []string{"storageState"}}
	obj, err := op.c.oCrud.StorageUpdate(ctx, sObj, items)
	if err != nil {
		op.rhs.setRequestMessage("failed to update Storage object: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.rhs.Storage = obj
}

func (op *useOp) useDevice(ctx context.Context) {
	if err := op.c.rei.ErrOnBool("nuvo-use-device"); err != nil {
		op.rhs.setRequestError("UseDevice: %s", err.Error())
		return
	} else if err := op.c.rei.ErrOnBool("block-nuvo-use-device"); err != nil {
		op.rhs.setAndUpdateRequestMessageDistinct(ctx, "UseDevice: %s", err.Error())
		op.rhs.RetryLater = true // we can attempt again
		return
	}
	sObj := op.rhs.Storage
	ss := sObj.StorageState
	sUUID := string(sObj.Meta.ID)
	devName := ss.AttachedNodeDevice
	devType, err := op.c.app.CSP.GetDeviceTypeByCspStorageType(sObj.CspStorageType)
	if err != nil {
		op.rhs.setRequestError("Unable to determine device type: %s", err.Error())
		// animator will set the Storage object DeviceState to UNUSED in termination failure logic
		return
	}
	op.c.Log.Debugf("StorageRequest %s: NUVOAPI UseDevice(%s, %s, %s)", op.rhs.Request.Meta.ID, sUUID, devName, devType)
	err = op.c.app.NuvoAPI.UseDevice(sUUID, devName, devType)
	if err != nil {
		if nuvoapi.ErrorIsTemporary(err) {
			op.rhs.setRequestMessage("NUVOAPI temporary error: %s", err.Error())
			op.rhs.RetryLater = true // we can attempt use again
			return
		}
		op.rhs.setRequestError("NUVOAPI UseDevice failed: %s", err.Error())
		// animator will set the Storage object DeviceState to UNUSED in termination failure logic
		return
	}
	items := &crud.Updates{Set: []string{"storageState.deviceState"}}
	ss.DeviceState = com.StgDeviceStateOpen
	obj, err := op.c.oCrud.StorageUpdate(ctx, sObj, items)
	if err != nil {
		op.rhs.setRequestMessage("failed to update Storage object: %s", err.Error())
		op.rhs.RetryLater = true // we can attempt use again
		return
	}
	op.rhs.Storage = obj
	storage := &agentd.Storage{
		StorageID:      sUUID,
		DeviceName:     devName,
		CspStorageType: obj.CspStorageType,
	}
	op.c.app.AppServant.AddStorage(storage)
}

func (op *useOp) closeDevice(ctx context.Context) {
	if err := op.c.rei.ErrOnBool("nuvo-close-device"); err != nil {
		op.rhs.setRequestError("CloseDevice: %s", err.Error())
		return
	} else if err := op.c.rei.ErrOnBool("block-nuvo-close-device"); err != nil {
		op.rhs.setAndUpdateRequestMessageDistinct(ctx, "CloseDevice: %s", err.Error())
		op.rhs.RetryLater = true // we can attempt again
		return
	}
	sObj := op.rhs.Storage
	ss := sObj.StorageState
	sUUID := string(sObj.Meta.ID)
	devName := ss.AttachedNodeDevice
	op.c.Log.Debugf("StorageRequest %s: NUVOAPI CloseDevice(%s, %s)", op.rhs.Request.Meta.ID, sUUID, devName)
	err := op.c.app.NuvoAPI.CloseDevice(sUUID)
	if err != nil {
		if nuvoapi.ErrorIsTemporary(err) {
			op.rhs.setRequestMessage("NUVOAPI temporary error: %s", err.Error())
			op.rhs.RetryLater = true // we can attempt again
			return
		}
		if err.Error() != errDeviceNotFound {
			op.rhs.setRequestError("NUVOAPI CloseDevice failed: %s", err.Error())
			return
		}
		op.c.Log.Debugf("StorageRequest %s: NUVOAPI CloseDevice(%s, %s) Ignoring error: %s", op.rhs.Request.Meta.ID, sUUID, devName, err.Error())
	}
	items := &crud.Updates{Set: []string{"storageState.deviceState"}}
	ss.DeviceState = com.StgDeviceStateUnused
	obj, err := op.c.oCrud.StorageUpdate(ctx, sObj, items)
	if err != nil {
		op.rhs.setRequestMessage("failed to update Storage object: %s", err.Error())
		op.rhs.RetryLater = true // we can attempt again
		return
	}
	op.rhs.Storage = obj
	op.c.app.AppServant.RemoveStorage(sUUID)
}
