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
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
)

// RecoverVolumesInUse is part of the AppRecoverVolume interface.
// It loads the lun cache with local volumes that are IN_USE and removes non-head luns.
// case [vol-recover:nuvo-ok-agentd-restarted] (https://goo.gl/3tMWq3)
func (c *Component) RecoverVolumesInUse(ctx context.Context) ([]*agentd.LUN, error) {
	c.Log.Debugf("RecoverVolumesInUse on node")
	ret, err := c.listLocalVolumes(ctx)
	if err != nil {
		c.Log.Debug("Failed to obtain list of volumes configured on this node")
		return nil, err
	}
	c.Log.Debugf("Found %d configured volumes for this node", len(ret.Payload))
	lunList := make([]*agentd.LUN, 0, len(ret.Payload))
	for _, vs := range ret.Payload {
		vsID := string(vs.Meta.ID)
		if vs.VolumeSeriesState == com.VolStateConfigured {
			c.Log.Debugf("VolumeSeries [%s] CONFIGURED", vsID)
			continue
		}
		nuvoVolumeIdentifier := vs.NuvoVolumeIdentifier
		snapMountsPresent := false
		var lun *agentd.LUN
		for _, m := range vs.Mounts {
			if m.MountedNodeID != c.thisNodeID {
				continue
			}
			if m.SnapIdentifier != com.VolMountHeadIdentifier {
				c.Log.Debugf("VolumeSeries [%s] NUVOAPI UnexportLun(%s, %s, %s)", vsID, nuvoVolumeIdentifier, m.PitIdentifier, m.MountedNodeDevice)
				err := c.App.NuvoAPI.UnexportLun(nuvoVolumeIdentifier, m.PitIdentifier, m.MountedNodeDevice)
				if err != nil && !strings.HasPrefix(err.Error(), errLunNotExported) {
					c.Log.Errorf("VolumeSeries [%s] NUVOAPI UnexportLun(%s, %s, %s): %s", vsID, nuvoVolumeIdentifier, m.PitIdentifier, m.MountedNodeDevice, err.Error())
					return nil, err
				}
				snapMountsPresent = true
			} else {
				c.Log.Debugf("VolumeSeries [%s] IN_USE Recovering mount [%s]", vsID, m.MountedNodeDevice)
				lun = &agentd.LUN{
					VolumeSeriesID:       vsID,
					SnapIdentifier:       m.SnapIdentifier,
					NuvoVolumeIdentifier: nuvoVolumeIdentifier,
					DeviceName:           m.MountedNodeDevice,
				}
			}
		}
		if snapMountsPresent {
			// update the database
			if _, err = c.removeLocalMounts(ctx, vsID, true, com.VolStateConfigured); err != nil {
				return nil, err
			}
		}
		if lun != nil {
			lunList = append(lunList, lun)
		}
	}
	return lunList, nil
}

// CheckNuvoVolumes checks to see if there are any volumes on the node; if none exist, it calls NoVolumesInUse.
// case [vol-recover:nuvo-restarted-agentd-restarted]  (https://goo.gl/3tMWq3)
func (c *Component) CheckNuvoVolumes(ctx context.Context) error {
	c.Log.Debugf("NUVOAPI ListVols() called")
	vols, err := c.App.NuvoAPI.ListVols()
	if err != nil {
		c.Log.Warningf("NUVOAPI ListVols(): %s", err.Error())
	} else {
		c.Log.Debugf("NUVOAPI ListVols(): %v", vols)
	}
	if err != nil || len(vols) == 0 {
		if _, err = c.NoVolumesInUse(ctx); err != nil {
			return err
		}
	}
	return nil
}

// NoVolumesInUse is part of the AppRecoverVolume interface.
// It finds all previously configured volumes (IN_USE, CONFIGURED) then deletes all local mounts and changes the state to PROVISIONED.
// It returns a list of all Storage objects used by the volumes previously in use on the node - the list may contain duplicates.
// It releases cache used by volumes and updates the cache state to reflect the change.
// See https://goo.gl/3tMWq3:
//  - case [vol-recover:nuvo-restarted-agentd-ok]
//  - case [vol-recover:nuvo-restarted-agentd-restarted]
func (c *Component) NoVolumesInUse(ctx context.Context) ([]string, error) {
	c.Log.Debugf("NoVolumesInUse on node")
	ret, err := c.listLocalVolumes(ctx)
	if err != nil {
		c.Log.Debug("Failed to obtain list of volumes previously IN_USE on this node")
		return nil, err
	}
	c.Log.Debugf("Found %d volumes previously in use on this node", len(ret.Payload))
	storageIDs := []string{}
	for _, vs := range ret.Payload {
		if _, err = c.removeLocalMounts(ctx, string(vs.Meta.ID), false, com.VolStateProvisioned); err != nil {
			return nil, err
		}
		for stgID := range vs.StorageParcels {
			storageIDs = append(storageIDs, stgID) // duplicates ok
		}
	}
	return storageIDs, nil
}

// VolumesRecovered is part of the AppRecoverVolume interface.
func (c *Component) VolumesRecovered() {
	c.Log.Debug("VolumesRecovered")
	c.Animator.Notify()
}

// listLocalVolumes is a support sub that fetches locally configured volumes (IN_USE, CONFIGURED)
func (c *Component) listLocalVolumes(ctx context.Context) (*volume_series.VolumeSeriesListOK, error) {
	c.setNodeInfo(c.App.GetNode()) // node information available but may not be set yet in component
	c.Log.Debugf("Searching for volumes previously configured on node [%s]", c.thisNodeID)
	lParams := volume_series.NewVolumeSeriesListParams()
	lParams.ConfiguredNodeID = swag.String(string(c.thisNodeID))
	return c.oCrud.VolumeSeriesList(ctx, lParams)
}

// removeLocalMounts is a support sub that removes local mounts of a VolumeSeries object and changes the object state if necessary.
// It also releases previous cache allocations.
// It returns the VolumeSeries if modified.
func (c *Component) removeLocalMounts(ctx context.Context, vsID string, onlySnapMounts bool, newState string) (*models.VolumeSeries, error) {
	errBreakout := fmt.Errorf("vs-updater-breakout")
	items := &crud.Updates{Set: []string{"messages", "mounts", "volumeSeriesState", "configuredNodeId"}}
	if onlySnapMounts {
		c.Log.Debugf("Volume [%s] attempting to release snapshot mounts on node [%s]", vsID, c.thisNodeID)
	} else {
		c.Log.Debugf("Volume [%s] attempting to release all mounts on node [%s]", vsID, c.thisNodeID)
		items.Set = append(items.Set, "lifecycleManagementData", "systemTags")
	}
	var buf bytes.Buffer
	modVS := func(vs *models.VolumeSeries) (*models.VolumeSeries, error) {
		if vs == nil {
			return nil, nil
		}
		buf.Reset()
		fmt.Fprintf(&buf, "Volume [%s] %s", vsID, vs.VolumeSeriesState)
		// skip if the volume is not IN_USE/CONFIGURED, or is already in the newState
		if !(vs.VolumeSeriesState == com.VolStateInUse || vs.VolumeSeriesState == com.VolStateConfigured) || vs.VolumeSeriesState == newState {
			return nil, errBreakout
		}
		ml := util.NewMsgList(vs.Messages)
		sTags := util.NewTagList(vs.SystemTags)
		numMounts := len(vs.Mounts)
		newMounts := make([]*models.Mount, 0, numMounts)
		for _, m := range vs.Mounts {
			// Note: highly likely that all mounts are on the same node
			if m.MountedNodeID != c.thisNodeID || (onlySnapMounts && m.SnapIdentifier == com.VolMountHeadIdentifier) {
				newMounts = append(newMounts, m)
				fmt.Fprintf(&buf, " I[%s, %s]", m.SnapIdentifier, m.MountedNodeID)
			} else {
				fmt.Fprintf(&buf, " D[%s]", m.SnapIdentifier)
				if !onlySnapMounts {
					ml.Insert("Mount [%s] on node [%s] crashed", m.SnapIdentifier, m.MountedNodeID)
					if m.SnapIdentifier == com.VolMountHeadIdentifier {
						if vs.LifecycleManagementData == nil {
							vs.LifecycleManagementData = &models.LifecycleManagementData{}
						}
						vs.LifecycleManagementData.FinalSnapshotNeeded = true
						fmt.Fprintf(&buf, " Head-Unexport-Time-Set")
						sTags.Set(com.SystemTagVolumeLastHeadUnexport, time.Now().String())
						sTags.Delete(com.SystemTagVolumeHeadStatSeries)
						sTags.Delete(com.SystemTagVolumeHeadStatCount)
						sTags.Delete(com.SystemTagVolumeFsAttached)
					}
				}
			}
		}
		vs.Mounts = newMounts
		fmt.Fprintf(&buf, " #Mounts:%d", len(newMounts))
		if len(newMounts) == 0 {
			ml.Insert("State change %s ⇒ %s", vs.VolumeSeriesState, newState)
			vs.VolumeSeriesState = newState
			vs.ConfiguredNodeID = ""
		} else if len(newMounts) == numMounts {
			fmt.Fprintf(&buf, " #D:0")
			return nil, errBreakout
		}
		// remove cache allocations for the volume on this node
		if _, ok := vs.CacheAllocations[string(c.thisNodeID)]; ok {
			items.Remove = append(items.Remove, "cacheAllocations")
			vs.CacheAllocations = map[string]models.CacheAllocation{
				string(c.thisNodeID): models.CacheAllocation{},
			}

			c.Log.Debugf("Releasing cache used for volume [%s] on node [%s]", vsID, c.thisNodeID)
			c.App.StateOps.ReleaseCache(ctx, vsID)

			c.Log.Debugf("Updating Node object to reflect released cache allocation for volume [%s] on node [%s]", vsID, c.thisNodeID)
			if err := c.App.StateOps.UpdateNodeAvailableCache(ctx); err != nil {
				return nil, err
			}
		}

		fmt.Fprintf(&buf, " ⇒ %s", vs.VolumeSeriesState)
		vs.Messages = ml.ToModel()
		vs.SystemTags = sTags.List()
		return vs, nil
	}
	vsObj, err := c.oCrud.VolumeSeriesUpdater(ctx, vsID, modVS, items)
	if buf.Len() > 0 {
		c.Log.Debugf("%s\n", buf.String())
	}
	if err != nil && err != errBreakout {
		c.Log.Errorf("Failed to transition volume [%s] from IN_USE: %s", vsID, err.Error())
		return nil, err
	}
	return vsObj, nil
}
