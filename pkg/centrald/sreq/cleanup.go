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

	mcp "github.com/Nuvoloso/kontroller/pkg/autogen/client/pool"
	mcs "github.com/Nuvoloso/kontroller/pkg/autogen/client/storage"
	mcsr "github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

// makeProvisioningStorageUnprovisioned transitions Storage objects in the PROVISIONING state
// to UNPROVISIONED so that they can be deleted.
// It also snaps the link from any active StorageRequest referencing the object.
// Should only be called on startup prior to processing any StorageRequest, and
// should be followed by deleting the UNPROVISIONED Storage objects and then
// recomputing the sizes of all pools.
func (c *Component) makeProvisioningStorageUnprovisioned(ctx context.Context) error {
	c.Log.Debug("Searching for Storage objects in the PROVISIONING state")
	lparams := mcs.NewStorageListParams()
	lparams.ProvisionedState = swag.String(com.StgProvisionedStateProvisioning)
	lRes, err := c.oCrud.StorageList(ctx, lparams)
	if err != nil {
		return err
	}
	stg := lRes.Payload
	c.Log.Infof("Found %d Storage objects in the PROVISIONING state", len(stg))
	for _, s := range stg {
		sID := string(s.Meta.ID)
		c.Log.Debugf("Searching for StorageRequest objects referencing Storage %s", sID)
		lSRParams := mcsr.NewStorageRequestListParams()
		lSRParams.StorageID = swag.String(sID)
		lSRParams.IsTerminated = swag.Bool(false)
		srs, err := c.oCrud.StorageRequestList(ctx, lSRParams)
		if err != nil {
			return err
		}
		for _, sr := range srs.Payload {
			items := &crud.Updates{}
			items.Set = []string{"requestMessages", "storageId"}
			modFn := func(o *models.StorageRequest) (*models.StorageRequest, error) {
				if o == nil {
					o = sr
				}
				o.StorageID = ""
				ts := &models.TimestampedString{
					Message: fmt.Sprintf("Clearing previous storageId %s", sID),
					Time:    strfmt.DateTime(time.Now()),
				}
				o.RequestMessages = append(o.RequestMessages, ts)
				return o, nil
			}
			c.Log.Infof("StorageRequest %s: clearing Storage %s", sr.Meta.ID, sID)
			if _, err = c.oCrud.StorageRequestUpdater(ctx, string(sr.Meta.ID), modFn, items); err != nil {
				return err
			}
		}
		c.Log.Warningf("Changing state of Storage object %s (pool %s) to UNPROVISIONED", sID, s.PoolID)
		ts := &models.TimestampedString{
			Message: fmt.Sprintf("ProvisionedState change: %s ⇒ %s", s.StorageState.ProvisionedState, com.StgProvisionedStateUnprovisioned),
			Time:    strfmt.DateTime(time.Now()),
		}
		s.StorageState.ProvisionedState = com.StgProvisionedStateUnprovisioned
		s.StorageState.Messages = append(s.StorageState.Messages, ts)
		items := &crud.Updates{}
		items.Set = []string{"storageState"}
		_, err = c.oCrud.StorageUpdate(ctx, s, items)
		if err != nil {
			return err
		}
	}
	return nil
}

// deleteUnprovisionedStorage deletes Storage objects that are in the UNPROVISIONED state.
// Should only be called on startup prior to processing any StorageRequest, and
// should be followed by recomputing the sizes of all pools.
func (c *Component) deleteUnprovisionedStorage(ctx context.Context) error {
	c.Log.Debug("Searching for Storage objects in the UNPROVISIONED state")
	lparams := mcs.NewStorageListParams()
	lparams.ProvisionedState = swag.String(com.StgProvisionedStateUnprovisioned)
	lRes, err := c.oCrud.StorageList(ctx, lparams)
	if err != nil {
		return err
	}
	stg := lRes.Payload
	c.Log.Infof("Found %d Storage objects in the UNPROVISIONED state", len(stg))
	for _, s := range stg {
		sID := string(s.Meta.ID)
		c.Log.Warningf("Deleting Storage object %s (pool %s)", sID, s.PoolID)
		err = c.oCrud.StorageDelete(ctx, sID)
		if err != nil {
			return err
		}
	}
	return nil
}

// releasePoolOrphanedCSPVolumes searches for orphaned CSP Volumes (with StorageRequest tags) and deletes them.
// Such tags are set only by the PROVISION operation but exist until the StorageRequest terminates.
// This function should be called only when it can be guaranteed that there are no concurrent PROVISION or RELEASE operations in progress, such as at startup.
// It works on a best-effort basis, but fails if configDB queries fail in unexpected ways; no changes are made to the Pool or CSPDomain objects.
func (c *Component) releasePoolOrphanedCSPVolumes(ctx context.Context, sp *models.Pool, dom *models.CSPDomain, cluster *models.Cluster) error {
	dc, err := c.App.AppCSP.GetDomainClient(dom)
	if err != nil {
		c.Log.Warningf("CSPDomain %s %s client failure: %s", dom.Name, dom.Meta.ID, err.Error())
		return nil
	}
	tags := []string{
		com.VolTagSystem + ":" + c.systemID,
		com.VolTagPoolID + ":" + string(sp.Meta.ID), // this pool only
		com.VolTagStorageRequestID,                  // existence check only
	}
	vla := &csp.VolumeListArgs{
		StorageTypeName:        models.CspStorageType(sp.CspStorageType),
		Tags:                   tags,
		ProvisioningAttributes: cluster.ClusterAttributes,
	}
	vols, err := dc.VolumeList(ctx, vla)
	if err != nil {
		c.Log.Warningf("Pool %s: Search for orphaned CSP volumes failed: %s", sp.Meta.ID, err.Error())
		return nil
	}
	// Not orphaned if the Storage object exists and is in PROVISIONED state, eg StorageRequest is still active perhaps in agentd
	filtered := make([]*csp.Volume, 0, len(vols))
	for _, vol := range vols {
		tags := util.NewTagList(vol.Tags)
		if sID, ok := tags.Get(com.VolTagStorageID); ok {
			if stg, err := c.oCrud.StorageFetch(ctx, sID); err != nil {
				if oErr, ok := err.(*crud.Error); !ok || !oErr.NotFound() {
					c.Log.Warningf("Failed to load Storage[%s]: %s", sID, err.Error())
					return err
				}
			} else if stg.StorageState.ProvisionedState == com.StgProvisionedStateProvisioned {
				srID, _ := tags.Get(com.VolTagStorageRequestID) // guaranteed to exist, it was in the query
				// if StorageRequest no longer exists or it is terminated, remove the tag (best effort)
				sr, err := c.oCrud.StorageRequestFetch(ctx, srID)
				if err != nil {
					if oErr, ok := err.(*crud.Error); !ok || !oErr.NotFound() {
						c.Log.Warningf("Failed to load StorageRequest[%s]: %s", srID, err.Error())
						return err
					}
				}
				// err != nil must be NotFound after previous check
				if err != nil || util.Contains([]string{com.StgReqStateSucceeded, com.StgReqStateFailed}, sr.StorageRequestState) {
					vta := &csp.VolumeTagArgs{
						VolumeIdentifier: vol.Identifier,
						Tags: []string{
							com.VolTagStorageRequestID + ":" + srID,
						},
						ProvisioningAttributes: cluster.ClusterAttributes,
					}
					c.Log.Debugf("VolumeTagsDelete %s: %v", vta.VolumeIdentifier, vta.Tags)
					if _, err = dc.VolumeTagsDelete(ctx, vta); err != nil {
						c.Log.Debugf("Ignoring VolumeTagsDelete error: %s", err.Error())
					}
				}
				continue
			}
		}
		filtered = append(filtered, vol)
	}
	vols = filtered
	c.Log.Infof("Pool %s: Found %d orphaned CSP volume(s)", sp.Meta.ID, len(vols))
	for _, vol := range vols {
		vda := &csp.VolumeDeleteArgs{
			VolumeIdentifier:       vol.Identifier,
			ProvisioningAttributes: cluster.ClusterAttributes,
		}
		c.Log.Infof("Pool %s: Deleting orphaned CSP volume %s", sp.Meta.ID, vda.VolumeIdentifier)
		if err = dc.VolumeDelete(ctx, vda); err != nil {
			if err != csp.ErrorVolumeNotFound {
				c.Log.Warningf("Pool %s: Failed to delete orphaned CSP volume %s: %s", sp.Meta.ID, vda.VolumeIdentifier, err.Error())
			}
		}
	}
	return nil
}

// releaseOrphanedCSPVolumes applies releasePoolOrphanedCSPVolumes to all Pools.
// It stops on the first database error encountered, but will ignore CSP operation errors.
// No changes are made to any database object.
// This function should be called only when it can be guaranteed that there are no concurrent PROVISION or RELEASE operations in progress, such as at startup.
func (c *Component) releaseOrphanedCSPVolumes(ctx context.Context) error {
	c.Log.Debug("Releasing orphaned CSP Volumes for all pools")
	dMap := map[models.ObjIDMutable]*models.CSPDomain{}
	cMap := map[models.ObjIDMutable]*models.Cluster{}
	lparams := mcp.NewPoolListParams()
	lsp, err := c.oCrud.PoolList(ctx, lparams)
	if err != nil {
		return err
	}
	for _, sp := range lsp.Payload {
		// fetch the CSPDomain object via a cache
		dObj, ok := dMap[sp.CspDomainID]
		if !ok {
			if dObj, err = c.oCrud.CSPDomainFetch(ctx, string(sp.CspDomainID)); err != nil {
				return err
			}
			dMap[sp.CspDomainID] = dObj
		}
		// fetch the Cluster object via a cache
		cObj, ok := cMap[sp.ClusterID]
		if !ok {
			if cObj, err = c.oCrud.ClusterFetch(ctx, string(sp.ClusterID)); err != nil {
				return err
			}
			cMap[sp.ClusterID] = cObj
		}
	}
	// clean up orphaned CSP volumes on a best-effort basis
	for _, sp := range lsp.Payload {
		dObj := dMap[sp.CspDomainID]
		cObj := cMap[sp.ClusterID]
		if err = c.releasePoolOrphanedCSPVolumes(ctx, sp, dObj, cObj); err != nil {
			return err
		}
	}
	return nil
}
