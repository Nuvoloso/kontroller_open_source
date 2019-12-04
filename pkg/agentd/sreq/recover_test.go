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
	"errors"
	"fmt"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	mcs "github.com/Nuvoloso/kontroller/pkg/autogen/client/storage"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	mockcsp "github.com/Nuvoloso/kontroller/pkg/csp/mock"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	mockNuvo "github.com/Nuvoloso/kontroller/pkg/nuvoapi/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

type netNotReady struct {
	error
	retTemporary, retTimeout bool
}

func (e *netNotReady) Temporary() bool {
	return e.retTemporary
}
func (e *netNotReady) Timeout() bool {
	return e.retTimeout
}
func (e *netNotReady) Error() string {
	return fmt.Sprintf("netNotReady: %#v", e)
}

func TestRecoverStorageInUse(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log: tl.Logger(),
		},
	}
	fao := &fakeAppObject{}
	fao.n = &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "THIS-NODE",
			},
			NodeIdentifier: "this-node-identifier",
		},
	}
	app.AppObjects = fao

	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	ctx := context.Background()
	c := &SRComp{}
	c.Init(app)
	c.thisNodeID = models.ObjIDMutable(fao.n.Meta.ID)
	c.thisNodeIdentifier = "ThisNode"
	fc := &fake.Client{}
	c.oCrud = fc

	expLsSParam := mcs.NewStorageListParams()
	expLsSParam.AttachedNodeID = swag.String(string(c.thisNodeID))
	expLsSParam.DeviceState = swag.String(com.StgDeviceStateOpen)

	resStorageList := &mcs.StorageListOK{
		Payload: []*models.Storage{
			&models.Storage{
				StorageAllOf0: models.StorageAllOf0{
					Meta:           &models.ObjMeta{ID: "STORAGE-1"},
					CspStorageType: "Amazon gp2",
				},
				StorageMutable: models.StorageMutable{
					StorageIdentifier: "sid1",
					StorageState: &models.StorageStateMutable{
						AttachedNodeID:     "THIS-NODE",
						AttachedNodeDevice: "/dev/storage1",
						AttachmentState:    "ATTACHED",
						DeviceState:        "OPEN",
						Messages:           []*models.TimestampedString{},
					},
				},
			},
			&models.Storage{
				StorageAllOf0: models.StorageAllOf0{
					Meta:           &models.ObjMeta{ID: "STORAGE-2"},
					CspStorageType: "Amazon gp2",
				},
				StorageMutable: models.StorageMutable{
					StorageIdentifier: "sid2",
					StorageState: &models.StorageStateMutable{
						AttachedNodeID:     "THIS-NODE",
						AttachedNodeDevice: "/dev/storage2",
						AttachmentState:    "ATTACHED",
						DeviceState:        "OPEN",
						Messages:           []*models.TimestampedString{},
					},
				},
			},
		},
	}

	// list error returned
	tl.Logger().Info("case: storage list error")
	tl.Flush()
	fc.InLsSObj = nil
	fc.RetLsSErr = fmt.Errorf("storage list error")
	stgList, err := c.RecoverStorageInUse(ctx)
	assert.Error(err)
	assert.Regexp("storage list error", err)
	assert.Nil(stgList)
	assert.NotNil(fc.InLsSObj)
	assert.Equal(expLsSParam, fc.InLsSObj)

	// empty list - no error
	tl.Logger().Info("case: empty list no error")
	tl.Flush()
	fc.InLsSObj = nil
	fc.RetLsSErr = nil
	fc.RetLsSOk = &mcs.StorageListOK{Payload: []*models.Storage{}}
	stgList, err = c.RecoverStorageInUse(ctx)
	assert.NoError(err)
	assert.NotNil(stgList)
	assert.Len(stgList, 0)
	assert.NotNil(fc.InLsSObj)
	assert.Equal(expLsSParam, fc.InLsSObj)

	// // storelandia timeout error
	// tl.Logger().Info("case: storelandia timeout error")
	// tl.Flush()
	// fc.InLsSObj = nil
	// fc.RetLsSErr = nil
	// fc.RetLsSOk = resStorageList
	// sObj := resStorageList.Payload[0]
	// sUUID := string(sObj.Meta.ID)
	// devName := sObj.StorageState.AttachedNodeDevice
	// mockCtrl := gomock.NewController(t)
	// defer func() { mockCtrl.Finish() }()
	// cspAPI := mockcsp.NewMockCloudServiceProvider(mockCtrl)
	// cspAPI.EXPECT().LocalInstanceDeviceName(sObj.StorageIdentifier, sObj.StorageState.AttachedNodeDevice).Return(sObj.StorageState.AttachedNodeDevice, nil)
	// app.CSP = cspAPI
	// nvAPI := mockNuvo.NewMockNuvoVM(mockCtrl)
	// c.app.NuvoAPI = nvAPI
	// netE := &netNotReady{retTimeout: true}
	// nvAPI.EXPECT().UseDevice(sUUID, devName, "SSD").Return(netE)
	// stgList, err = c.RecoverStorageInUse(ctx)
	// assert.Error(err)
	// assert.Regexp("netNotReady", err)
	// assert.Nil(stgList)
	// assert.NotNil(fc.InLsSObj)
	// assert.Equal(expLsSParam, fc.InLsSObj)

	// storelandia temporary error
	tl.Logger().Info("case: storelandia temporary error")
	tl.Flush()
	fc.InLsSObj = nil
	fc.RetLsSErr = nil
	fc.RetLsSOk = resStorageList
	sObj := resStorageList.Payload[0]
	sUUID := string(sObj.Meta.ID)
	devName := sObj.StorageState.AttachedNodeDevice
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cspAPI := mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cspAPI.EXPECT().LocalInstanceDeviceName(sObj.StorageIdentifier, sObj.StorageState.AttachedNodeDevice).Return(sObj.StorageState.AttachedNodeDevice, nil)
	cspAPI.EXPECT().GetDeviceTypeByCspStorageType(gomock.Any()).Return("SSD", nil)
	app.CSP = cspAPI
	nvAPI := mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	netE := nuvoapi.NewNuvoAPIError("tempErr", true)
	nvAPI.EXPECT().UseDevice(sUUID, devName, "SSD").Return(netE)
	stgList, err = c.RecoverStorageInUse(ctx)
	assert.Error(err)
	assert.Regexp("tempErr", err)
	assert.Nil(stgList)
	assert.NotNil(fc.InLsSObj)
	assert.Equal(expLsSParam, fc.InLsSObj)

	// other storelandia error + update error
	tl.Logger().Info("case: storelandia error + update storage error")
	tl.Flush()
	fc.InLsSObj = nil
	fc.RetLsSErr = nil
	fc.RetLsSOk = resStorageList
	fc.RetUSErr = fmt.Errorf("update storage error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cspAPI = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cspAPI.EXPECT().LocalInstanceDeviceName(sObj.StorageIdentifier, sObj.StorageState.AttachedNodeDevice).Return(sObj.StorageState.AttachedNodeDevice, nil)
	cspAPI.EXPECT().GetDeviceTypeByCspStorageType(gomock.Any()).Return("SSD", nil)
	app.CSP = cspAPI
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	nvAPI.EXPECT().UseDevice(sUUID, devName, "SSD").Return(fmt.Errorf("storelandia error"))
	assert.Empty(sObj.StorageState.Messages)
	stgList, err = c.RecoverStorageInUse(ctx)
	assert.Error(err)
	assert.Regexp("update storage error", err)
	assert.Nil(stgList)
	assert.NotNil(fc.InLsSObj)
	assert.Equal(expLsSParam, fc.InLsSObj)
	assert.Len(sObj.StorageState.Messages, 2)
	assert.Regexp("USE failed", sObj.StorageState.Messages[0].Message)
	assert.Regexp("⇒ ERROR", sObj.StorageState.Messages[1].Message)
	assert.Equal("ERROR", sObj.StorageState.DeviceState)

	// other storelandia error only
	tl.Logger().Info("case: storelandia error only")
	tl.Flush()
	fc.InLsSObj = nil
	fc.RetLsSErr = nil
	fc.RetLsSOk = &mcs.StorageListOK{
		Payload: []*models.Storage{
			resStorageList.Payload[0],
		},
	}
	fc.RetUSErr = nil
	sObj.StorageState.DeviceState = "OPEN"
	sObj.StorageState.Messages = []*models.TimestampedString{}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cspAPI = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cspAPI.EXPECT().LocalInstanceDeviceName(sObj.StorageIdentifier, sObj.StorageState.AttachedNodeDevice).Return(sObj.StorageState.AttachedNodeDevice, nil)
	cspAPI.EXPECT().GetDeviceTypeByCspStorageType(gomock.Any()).Return("SSD", nil)
	app.CSP = cspAPI
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	nvAPI.EXPECT().UseDevice(sUUID, devName, "SSD").Return(fmt.Errorf("storelandia error"))
	assert.Empty(sObj.StorageState.Messages)
	stgList, err = c.RecoverStorageInUse(ctx)
	assert.NoError(err)
	assert.NotNil(fc.InLsSObj)
	assert.Equal(expLsSParam, fc.InLsSObj)
	assert.Len(sObj.StorageState.Messages, 2)
	assert.Regexp("USE failed", sObj.StorageState.Messages[0].Message)
	assert.Regexp("⇒ ERROR", sObj.StorageState.Messages[1].Message)
	assert.NotNil(stgList)
	assert.Len(stgList, len(fc.RetLsSOk.Payload))
	for _, o := range fc.RetLsSOk.Payload {
		uuid := string(o.Meta.ID)
		found := false
		for _, stg := range stgList {
			if stg.StorageID == uuid {
				found = true
			}
		}
		assert.True(found, "Storage: %s", uuid)
	}

	tl.Logger().Info("case: LocalInstanceDeviceName error")
	tl.Flush()
	fc.InLsSObj = nil
	fc.RetUSErr = nil
	fc.RetLsSOk = resStorageList
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cspAPI = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cspAPI.EXPECT().LocalInstanceDeviceName(sObj.StorageIdentifier, sObj.StorageState.AttachedNodeDevice).Return("", errors.New("lidError"))
	app.CSP = cspAPI
	app.NuvoAPI = nil
	stgList, err = c.RecoverStorageInUse(ctx)
	assert.Nil(stgList)
	assert.Regexp("lidErr", err)

	tl.Logger().Info("case: updateAttachedNodeDevice update error")
	tl.Flush()
	var sObj2 *models.Storage
	testutils.Clone(sObj, &sObj2)
	fc.InLsSObj = nil
	fc.RetUSErr = fmt.Errorf("update storage error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cspAPI = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cspAPI.EXPECT().LocalInstanceDeviceName(sObj2.StorageIdentifier, sObj2.StorageState.AttachedNodeDevice).Return("/dev/xvdba", nil)
	app.CSP = cspAPI
	app.NuvoAPI = nil
	sObj2, err = c.updateAttachedNodeDevice(ctx, sObj2)
	assert.Nil(sObj2)
	assert.Equal(fc.RetUSErr, err)

	tl.Logger().Info("case: updateAttachedNodeDevice no longer attached")
	tl.Flush()
	testutils.Clone(sObj, &sObj2)
	fc.InLsSObj = nil
	fc.RetUSObj = sObj2
	fc.RetUSErr = nil
	fc.RetLsSOk = &mcs.StorageListOK{
		Payload: []*models.Storage{
			sObj2,
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cspAPI = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cspAPI.EXPECT().LocalInstanceDeviceName(sObj2.StorageIdentifier, sObj2.StorageState.AttachedNodeDevice).Return("", nil)
	app.CSP = cspAPI
	app.NuvoAPI = nil
	stgList, err = c.RecoverStorageInUse(ctx)
	assert.NotNil(stgList)
	assert.Empty(stgList)
	assert.Empty(sObj2.StorageState.AttachedNodeDevice)
	assert.Equal("DETACHED", sObj2.StorageState.AttachmentState)
	assert.Equal("UNUSED", sObj2.StorageState.DeviceState)
	assert.Len(sObj2.StorageState.Messages, 3)
	assert.Regexp("ATTACHED...DETACHED .externally", sObj2.StorageState.Messages[2])
	assert.NoError(err)

	tl.Logger().Info("case: updateAttachedNodeDevice update success")
	tl.Flush()
	testutils.Clone(sObj, &sObj2)
	fc.InLsSObj = nil
	fc.RetUSObj = sObj2
	fc.RetUSErr = nil
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cspAPI = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cspAPI.EXPECT().LocalInstanceDeviceName(sObj2.StorageIdentifier, sObj2.StorageState.AttachedNodeDevice).Return("/dev/xvdba", nil)
	app.CSP = cspAPI
	app.NuvoAPI = nil
	assert.Equal("/dev/storage1", sObj2.StorageState.AttachedNodeDevice)
	sObj2, err = c.updateAttachedNodeDevice(ctx, sObj2)
	assert.NotNil(sObj2)
	assert.Equal("/dev/xvdba", sObj2.StorageState.AttachedNodeDevice)
	assert.NoError(err)

	// success (sObj known)
	tl.Logger().Info("case: success (sObj known)")
	tl.Flush()
	fc.InLsSObj = nil
	fc.RetUSErr = nil
	fc.RetLsSOk = resStorageList
	sObj.StorageState.DeviceState = "OPEN"
	sObj.StorageState.Messages = []*models.TimestampedString{}
	sObj2 = resStorageList.Payload[1]
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cspAPI = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cspAPI.EXPECT().LocalInstanceDeviceName(sObj.StorageIdentifier, sObj.StorageState.AttachedNodeDevice).Return(sObj.StorageState.AttachedNodeDevice, nil)
	cspAPI.EXPECT().LocalInstanceDeviceName(sObj2.StorageIdentifier, sObj2.StorageState.AttachedNodeDevice).Return(sObj2.StorageState.AttachedNodeDevice, nil)
	cspAPI.EXPECT().GetDeviceTypeByCspStorageType(gomock.Any()).Return("SSD", nil).MaxTimes(2)
	app.CSP = cspAPI
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	mNotNil := gomock.Not(gomock.Nil())
	nvAPI.EXPECT().UseDevice(mNotNil, mNotNil, "SSD").Return(nil)
	nvAPI.EXPECT().UseDevice(mNotNil, mNotNil, "SSD").Return(errors.New(errDeviceAlreadyInUse)) // ignored
	stgList, err = c.RecoverStorageInUse(ctx)
	assert.NoError(err)
	assert.NotNil(fc.InLsSObj)
	assert.Equal(expLsSParam, fc.InLsSObj)
	assert.Equal(tl.CountPattern("UseDevice.*STORAGE-1"), 1)
	assert.Equal(tl.CountPattern("UseDevice.*STORAGE-2"), 1)
	assert.NotNil(stgList)
	assert.Len(stgList, len(resStorageList.Payload))
	for _, o := range resStorageList.Payload {
		uuid := string(o.Meta.ID)
		found := false
		for _, stg := range stgList {
			if stg.StorageID == uuid {
				found = true
			}
		}
		assert.True(found, "Storage: %s", uuid)
	}

	// success (RecoverStorage)
	tl.Logger().Info("case: success (sObj not known)")
	tl.Flush()
	storage := &agentd.Storage{
		StorageID:  "STORAGE-1",
		DeviceName: "/dev/storage1",
	}
	sObj.StorageState.DeviceState = "OPEN"
	sObj.StorageState.Messages = []*models.TimestampedString{}
	fc.StorageFetchRetObj = sObj
	fc.StorageFetchRetErr = nil
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cspAPI = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cspAPI.EXPECT().GetDeviceTypeByCspStorageType(gomock.Any()).Return("SSD", nil)
	app.CSP = cspAPI
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	nvAPI.EXPECT().UseDevice(storage.StorageID, storage.DeviceName, "SSD").Return(fmt.Errorf("error1"))
	err = c.ConfigureStorage(ctx, storage, nil)
	assert.NoError(err)
	assert.Equal(storage.StorageID, fc.StorageFetchID)
	assert.Len(sObj.StorageState.Messages, 2)

	// failure (RecoverStorage)
	tl.Logger().Info("case: success (sObj fetch error)")
	tl.Flush()
	fc.StorageFetchID = ""
	fc.StorageFetchRetObj = nil
	fc.StorageFetchRetErr = fmt.Errorf("fetch-error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cspAPI = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cspAPI.EXPECT().GetDeviceTypeByCspStorageType(gomock.Any()).Return("SSD", nil)
	app.CSP = cspAPI
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	nvAPI.EXPECT().UseDevice(storage.StorageID, storage.DeviceName, "SSD").Return(fmt.Errorf("error2"))
	err = c.ConfigureStorage(ctx, storage, nil)
	assert.Error(err)
	assert.Regexp("fetch-error", err)
	assert.Equal(storage.StorageID, fc.StorageFetchID)

	// failure (Unable to determine device type)
	tl.Logger().Info("case: failure (Unable to determine device type)")
	tl.Flush()
	fc.StorageFetchRetObj = sObj
	fc.StorageFetchRetErr = nil
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cspAPI = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cspAPI.EXPECT().GetDeviceTypeByCspStorageType(gomock.Any()).Return("", fmt.Errorf("dev-type-error"))
	app.CSP = cspAPI
	err = c.ConfigureStorage(ctx, storage, nil)
	assert.Error(err)
	assert.Regexp("dev-type-error", err)
}

func TestRecoverStorageLocations(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log: tl.Logger(),
		},
	}
	fao := &fakeAppObject{}
	fao.n = &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "THIS-NODE",
			},
			NodeIdentifier: "this-node-identifier",
		},
	}
	app.AppObjects = fao

	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	ctx := context.Background()
	c := &SRComp{}
	c.Init(app)
	c.thisNodeID = models.ObjIDMutable(fao.n.Meta.ID)
	c.thisNodeIdentifier = "ThisNode"
	fc := &fake.Client{}
	c.oCrud = fc

	stgObj := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta: &models.ObjMeta{ID: "STORAGE-1"},
		},
		StorageMutable: models.StorageMutable{
			StorageState: &models.StorageStateMutable{
				AttachmentState:    "ATTACHED",
				AttachedNodeID:     "NODE-1",
				AttachedNodeDevice: "/dev/storage1",
				DeviceState:        "OPEN",
				Messages:           []*models.TimestampedString{},
			},
		},
	}
	var stg *models.Storage
	nodeObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{ID: "NODE-1"},
		},
		NodeMutable: models.NodeMutable{
			Service: &models.NuvoService{
				NuvoServiceAllOf0: models.NuvoServiceAllOf0{
					ServiceIP: "4.3.2.1",
				},
			},
		},
	}
	var node *models.Node
	var stgIDs []string
	var err error

	tl.Logger().Info("case: remote storage ok")
	stg = stgClone(stgObj)
	fc.StorageFetchRetObj = stg
	fc.StorageFetchRetErr = nil
	node = nodeClone(nodeObj)
	fc.RetNObj = node
	fc.RetNErr = nil
	stgIDs = []string{string(stg.Meta.ID)}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	nvAPI := mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	nvAPI.EXPECT().NodeLocation(string(node.Meta.ID), node.Service.ServiceIP, uint16(c.app.NuvoPort)).Return(nil)
	nvAPI.EXPECT().DeviceLocation(string(stg.Meta.ID), string(node.Meta.ID)).Return(nil)
	err = c.RecoverStorageLocations(ctx, stgIDs)
	assert.NoError(err)

	mockCtrl.Finish()
	tl.Logger().Info("case: remote storage ok - local node")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	stg = stgClone(stgObj)
	stg.StorageState.AttachedNodeID = c.thisNodeID
	fc.StorageFetchRetObj = stg
	fc.StorageFetchRetErr = nil
	node = nodeClone(nodeObj)
	fc.RetNObj = node
	fc.RetNErr = nil
	stgIDs = []string{string(stg.Meta.ID)}
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	nvAPI.EXPECT().DeviceLocation(string(stg.Meta.ID), string(c.thisNodeID)).Return(nil)
	err = c.RecoverStorageLocations(ctx, stgIDs)
	assert.NoError(err)

	mockCtrl.Finish()
	tl.Logger().Info("case: remote storage ok - locations already set")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	stg = stgClone(stgObj)
	fc.StorageFetchRetObj = stg
	fc.StorageFetchRetErr = nil
	node = nodeClone(nodeObj)
	fc.RetNObj = node
	fc.RetNErr = nil
	stgIDs = []string{string(stg.Meta.ID)}
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	nodeLocSetErr := fmt.Errorf("%s", errNodeLocationAlreadySet)
	nvAPI.EXPECT().NodeLocation(string(node.Meta.ID), node.Service.ServiceIP, uint16(c.app.NuvoPort)).Return(nodeLocSetErr)
	devLocSetErr := fmt.Errorf("%s", errDeviceLocationAlreadySet)
	nvAPI.EXPECT().DeviceLocation(string(stg.Meta.ID), string(node.Meta.ID)).Return(devLocSetErr)
	err = c.RecoverStorageLocations(ctx, stgIDs)
	assert.NoError(err)

	mockCtrl.Finish()
	tl.Logger().Info("case: remote storage - device location fail")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	stg = stgClone(stgObj)
	fc.StorageFetchRetObj = stg
	fc.StorageFetchRetErr = nil
	node = nodeClone(nodeObj)
	fc.RetNObj = node
	fc.RetNErr = nil
	stgIDs = []string{string(stg.Meta.ID)}
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	nvAPI.EXPECT().NodeLocation(string(node.Meta.ID), node.Service.ServiceIP, uint16(c.app.NuvoPort)).Return(nil)
	nvAPI.EXPECT().DeviceLocation(string(stg.Meta.ID), string(node.Meta.ID)).Return(fmt.Errorf("device-location-error"))
	err = c.RecoverStorageLocations(ctx, stgIDs)
	assert.Error(err)
	assert.Regexp("device-location-error", err)

	mockCtrl.Finish()
	tl.Logger().Info("case: remote storage - node location fail")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	stg = stgClone(stgObj)
	fc.StorageFetchRetObj = stg
	fc.StorageFetchRetErr = nil
	node = nodeClone(nodeObj)
	fc.RetNObj = node
	fc.RetNErr = nil
	stgIDs = []string{string(stg.Meta.ID)}
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	nvAPI.EXPECT().NodeLocation(string(node.Meta.ID), node.Service.ServiceIP, uint16(c.app.NuvoPort)).Return(fmt.Errorf("node-location-error"))
	err = c.RecoverStorageLocations(ctx, stgIDs)
	assert.Error(err)
	assert.Regexp("node-location-error", err)

	mockCtrl.Finish()
	tl.Logger().Info("case: remote storage - node fetch fail")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	stg = stgClone(stgObj)
	fc.StorageFetchRetObj = stg
	fc.StorageFetchRetErr = nil
	node = nodeClone(nodeObj)
	fc.RetNObj = nil
	fc.RetNErr = fmt.Errorf("node-fetch-error")
	stgIDs = []string{string(stg.Meta.ID)}
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	err = c.RecoverStorageLocations(ctx, stgIDs)
	assert.Error(err)
	assert.Regexp("node-fetch-error", err)

	mockCtrl.Finish()
	tl.Logger().Info("case: remote storage - storage not attached")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	stg = stgClone(stgObj)
	stg.StorageState.AttachmentState = "ATTACHING"
	fc.StorageFetchRetObj = stg
	fc.StorageFetchRetErr = nil
	stgIDs = []string{string(stg.Meta.ID)}
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	err = c.RecoverStorageLocations(ctx, stgIDs)
	assert.Error(err)
	assert.Regexp("not attached", err)

	mockCtrl.Finish()
	tl.Logger().Info("case: remote storage - storage state not set")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	stg = stgClone(stgObj)
	stg.StorageState = nil
	fc.StorageFetchRetObj = stg
	fc.StorageFetchRetErr = nil
	stgIDs = []string{string(stg.Meta.ID)}
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	err = c.RecoverStorageLocations(ctx, stgIDs)
	assert.Error(err)
	assert.Regexp("not attached", err)

	mockCtrl.Finish()
	tl.Logger().Info("case: remote storage - storage fetch failed")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	fc.StorageFetchRetObj = nil
	fc.StorageFetchRetErr = fmt.Errorf("storage-fetch-error")
	stgIDs = []string{string(stg.Meta.ID)}
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.app.NuvoAPI = nvAPI
	err = c.RecoverStorageLocations(ctx, stgIDs)
	assert.Error(err)
	assert.Regexp("storage-fetch-error", err)
}

func TestStorageRecovered(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log: tl.Logger(),
		},
	}
	fao := &fakeAppObject{}
	app.AppObjects = fao

	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	c := &SRComp{}
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc

	c.notifyFlagged = true
	c.StorageRecovered()
	assert.Equal(1, c.notifyCount)
}

func stgClone(o *models.Storage) *models.Storage {
	n := new(models.Storage)
	testutils.Clone(o, n)
	return n
}

func nodeClone(o *models.Node) *models.Node {
	n := new(models.Node)
	testutils.Clone(o, n)
	return n
}
