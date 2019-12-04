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

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	mcs "github.com/Nuvoloso/kontroller/pkg/autogen/client/storage"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

// well-known error messages returned by Storelandia
const (
	errDeviceAlreadyInUse       = "ERROR: Invalid argument"
	errDeviceLocationAlreadySet = "ERROR: File exists"
	errNodeLocationAlreadySet   = "ERROR: File exists"
)

// RecoverStorageInUse must be called on startup to inform Storelandia of storage devices
// that are known to be in use.
// If Storelandia returns an unexpected error then it will mark the DeviceState of the
// Storage object as ERROR.
// It will return an error for re-tryable conditions.
func (c *SRComp) RecoverStorageInUse(ctx context.Context) ([]*agentd.Storage, error) {
	c.setNodeInfo(c.app.GetNode()) // node information available but may not be set yet in component
	c.Log.Debugf("Recovering storage previously in use on node [%s]", c.thisNodeID)
	lParams := mcs.NewStorageListParams()
	lParams.AttachedNodeID = swag.String(string(c.thisNodeID))
	lParams.DeviceState = swag.String(com.StgDeviceStateOpen)
	res, err := c.oCrud.StorageList(ctx, lParams)
	if err != nil {
		return nil, err
	}
	stgList := make([]*agentd.Storage, 0, len(res.Payload))
	for _, sObj := range res.Payload {
		if sObj, err = c.updateAttachedNodeDevice(ctx, sObj); err != nil {
			return nil, err
		}
		if sObj.StorageState.AttachmentState == com.StgAttachmentStateDetached {
			c.Log.Debugf("Storage [%s] no longer attached", string(sObj.Meta.ID))
			continue
		}
		ss := sObj.StorageState
		sUUID := string(sObj.Meta.ID)
		devName := ss.AttachedNodeDevice
		sType := sObj.CspStorageType
		storage := &agentd.Storage{
			StorageID:      sUUID,
			DeviceName:     devName,
			CspStorageType: sType,
		}
		if err = c.ConfigureStorage(ctx, storage, sObj); err != nil {
			return nil, err
		}
		stgList = append(stgList, storage)
	}
	return stgList, nil
}

// Update the AttachedNodeDevice in the storage object if it has changed, eg on an instance with NVMe storage
func (c *SRComp) updateAttachedNodeDevice(ctx context.Context, sObj *models.Storage) (*models.Storage, error) {
	devName, err := c.app.CSP.LocalInstanceDeviceName(sObj.StorageIdentifier, sObj.StorageState.AttachedNodeDevice)
	if err != nil {
		return nil, err
	} else if devName == sObj.StorageState.AttachedNodeDevice {
		return sObj, nil
	} else if devName == "" {
		// disk is no longer attached, transition disk to DETACHED
		ts := &models.TimestampedString{
			Message: fmt.Sprintf("AttachmentState change: %s ⇒ %s (externally)", sObj.StorageState.AttachmentState, com.StgAttachmentStateDetached),
			Time:    strfmt.DateTime(time.Now()),
		}
		sObj.StorageState.AttachmentState = com.StgAttachmentStateDetached
		sObj.StorageState.AttachedNodeID = ""
		sObj.StorageState.DeviceState = com.StgDeviceStateUnused
		sObj.StorageState.Messages = append(sObj.StorageState.Messages, ts)
	}
	sObj.StorageState.AttachedNodeDevice = devName
	items := &crud.Updates{Set: []string{"storageState"}}
	return c.oCrud.StorageUpdate(ctx, sObj, items)
}

// ConfigureStorage tells nuvo to use a Storage object from the cache, optionally with the database object
func (c *SRComp) ConfigureStorage(ctx context.Context, stg *agentd.Storage, sObj *models.Storage) error {
	sUUID := stg.StorageID
	devName := stg.DeviceName
	devType, err := c.app.CSP.GetDeviceTypeByCspStorageType(stg.CspStorageType)
	if err != nil {
		c.Log.Errorf("Unable to determine device type for [%s,%s,%s]: %s", stg.StorageID, stg.DeviceName, stg.CspStorageType, err.Error())
		// animator will set the Storage object DeviceState to UNUSED in termination failure logic
		return err
	}
	c.Log.Debugf("Recovery: NUVOAPI UseDevice(%s, %s, %s)", sUUID, devName, devType)
	err = c.app.NuvoAPI.UseDevice(sUUID, devName, devType)
	if err != nil && err.Error() != errDeviceAlreadyInUse {
		if nuvoapi.ErrorIsTemporary(err) {
			c.Log.Debugf("Recovery: NUVOAPI Storelandia is not ready: %#v", err)
			return err
		}
		c.Log.Errorf("Recovery: NUVOAPI UseDevice(%s, %s, %s): %#v", sUUID, devName, devType, err)
		if sObj == nil {
			var e2 error
			if sObj, e2 = c.oCrud.StorageFetch(ctx, sUUID); e2 != nil {
				sObj = nil
				err = e2
			}
		}
		if sObj != nil {
			now := time.Now()
			ss := sObj.StorageState
			ss.Messages = append(ss.Messages,
				&models.TimestampedString{
					Message: fmt.Sprintf("USE failed [node=%s device=%s]: %s", c.thisNodeIdentifier, devName, err.Error()),
					Time:    strfmt.DateTime(now),
				},
				&models.TimestampedString{
					Message: fmt.Sprintf("State change: deviceState %s ⇒ %s", ss.DeviceState, com.StgDeviceStateError),
					Time:    strfmt.DateTime(now),
				})
			ss.DeviceState = com.StgDeviceStateError
			items := &crud.Updates{Set: []string{"storageState"}}
			_, err = c.oCrud.StorageUpdate(ctx, sObj, items)
		}
	} else {
		err = nil
	}
	return err
}

// RecoverStorageLocations is called on nuvo restart to tell Nuvo of the locations of the specified devices previously in use by volumes on this node
func (c *SRComp) RecoverStorageLocations(ctx context.Context, stgIDs []string) error {
	c.Log.Debugf("RecoverStorageLocations (%d)", len(stgIDs))
	// load storage objects and collect node objects
	stgLocs := map[string]string{}
	nodeIPs := map[string]string{}
	for _, stgID := range stgIDs {
		s, err := c.oCrud.StorageFetch(ctx, stgID)
		if err == nil && (s.StorageState == nil || s.StorageState.AttachmentState != com.StgAttachmentStateAttached) {
			err = fmt.Errorf("not attached")
		}
		if err != nil {
			c.Log.Debugf("Failed to recover location of Storage [%s]: %s", stgID, err.Error())
			return err
		}
		nodeID := string(s.StorageState.AttachedNodeID)
		c.Log.Debugf("Storage [%s] is attached to node [%s]", stgID, nodeID)
		stgLocs[stgID] = nodeID
		nodeIPs[nodeID] = ""
	}
	// load node objects and collect IPs
	for _, nodeID := range util.StringKeys(nodeIPs) {
		if nodeID == string(c.thisNodeID) {
			continue
		}
		n, err := c.oCrud.NodeFetch(ctx, nodeID)
		if err != nil {
			c.Log.Debugf("Failed to fetch node [%s]: %s", nodeID, err.Error())
			return err
		}
		nodeIPs[nodeID] = n.Service.ServiceIP
		c.Log.Debugf("Node [%s] IP at %s", nodeID, n.Service.ServiceIP)
	}
	for nodeID, ipAddr := range nodeIPs {
		if nodeID == string(c.thisNodeID) {
			continue
		}
		c.Log.Debugf("NUVOAPI NodeLocation(%s, %s, %d)", nodeID, ipAddr, uint16(c.app.NuvoPort))
		err := c.app.NuvoAPI.NodeLocation(nodeID, ipAddr, uint16(c.app.NuvoPort))
		if err != nil {
			if err.Error() != errNodeLocationAlreadySet {
				c.Log.Errorf("NUVOAPI NodeLocation(%s, %s, %d) failed: %s", nodeID, ipAddr, uint16(c.app.NuvoPort), err.Error())
				return err
			}
			c.Log.Debugf("NUVOAPI NodeLocation(%s, %s, %d): %s", nodeID, ipAddr, uint16(c.app.NuvoPort), err.Error())
		}
	}
	for stgID, nodeID := range stgLocs {
		c.Log.Debugf("NUVOAPI DeviceLocation(%s, %s)", stgID, nodeID)
		err := c.app.NuvoAPI.DeviceLocation(stgID, nodeID)
		if err != nil {
			if err.Error() != errDeviceLocationAlreadySet {
				c.Log.Debugf("NUVOAPI DeviceLocation(%s, %s) failed: %s", stgID, nodeID, err.Error())
				return err
			}
			c.Log.Debugf("NUVOAPI DeviceLocation(%s, %s): %s", stgID, nodeID, err.Error())
		}
	}
	return nil
}

// StorageRecovered is part of the agentd.AppRecoverStorage interface
func (c *SRComp) StorageRecovered() {
	c.Log.Debug("StorageRecovered")
	c.Notify()
}
