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
	"strings"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	"github.com/Nuvoloso/kontroller/pkg/agentd/state"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

// exportSubState represents VOLUME_EXPORT sub-states.
// There is at most one database update in each sub-state.
type exportSubState int

// exportSubState values
const (
	// Set the mountState to MOUNTING
	ExportSetMounting exportSubState = iota
	// Fetch the service plan to force caching - required by metric processing
	ExportFetchServicePlan
	// Get the stat series uuid if HEAD
	ExportGetStatUUID
	// Allocate cache for volume series on the host
	ExportAllocateCache
	// Export volume series on the host
	ExportLun
	// Set the mountState to MOUNTED
	ExportSetMounted
	// Get the zero metric
	ExportGetFirstMetric
	// Return
	ExportDone
	// Save current cache allocations in VSR for UNDO path
	ExportSaveProps
	// Update cache allocation for volume series on the host
	ExportUpdateCache
	// Return
	ExportReallocCacheDone
	// Set the mountState to ERROR
	ExportSetError
	// Set the mountState to UNMOUNTING
	ExportSetUnmounting
	// Publish final metrics if HEAD
	ExportPublishFinalMetrics
	// Stop exporting the Lun on the host
	ExportUnexportLun
	// Release allocated cache for volume series on the host
	ExportReleaseCache
	// Get a count of write IOs made if HEAD
	ExportGetStatCount
	// Remove the mount entry from the mounts list
	ExportSetUnmounted
	// Unconfigure the volume if necessary
	ExportDoUnconfigure
	// Return
	ExportUndoDone
	// Restore the cache to the previous allocation
	ExportRestoreCache
	// Return
	ExportRestoreCacheUndoDone
	// NoOp
	ExportNoOp
)

func (ss exportSubState) String() string {
	switch ss {
	case ExportSetMounting:
		return "ExportSetMounting"
	case ExportFetchServicePlan:
		return "ExportFetchServicePlan"
	case ExportGetStatUUID:
		return "ExportGetStatUUID"
	case ExportAllocateCache:
		return "ExportAllocateCache"
	case ExportLun:
		return "ExportLun"
	case ExportSetMounted:
		return "ExportSetMounted"
	case ExportGetFirstMetric:
		return "ExportGetFirstMetric"
	case ExportDone:
		return "ExportDone"
	case ExportSaveProps:
		return "ExportSaveProps"
	case ExportUpdateCache:
		return "ExportUpdateCache"
	case ExportReallocCacheDone:
		return "ExportReallocCacheDone"
	case ExportSetError:
		return "ExportSetError"
	case ExportSetUnmounting:
		return "ExportSetUnmounting"
	case ExportPublishFinalMetrics:
		return "ExportPublishFinalMetrics"
	case ExportUnexportLun:
		return "ExportUnexportLun"
	case ExportReleaseCache:
		return "ExportReleaseCache"
	case ExportGetStatCount:
		return "ExportGetStatCount"
	case ExportSetUnmounted:
		return "ExportSetUnmounted"
	case ExportDoUnconfigure:
		return "ExportDoUnconfigure"
	case ExportUndoDone:
		return "ExportUndoDone"
	case ExportRestoreCache:
		return "ExportRestoreCache"
	case ExportRestoreCacheUndoDone:
		return "ExportRestoreCacheUndoDone"
	}
	return fmt.Sprintf("exportSubState(%d)", ss)
}

// well-known error messages returned by Storelandia
const (
	errLunAlreadyExported = "ERROR: Lun exported" // prefix
	errLunNotExported     = "OK:"                 // prefix
)

type exportOperators interface {
	getInitialState(ctx context.Context) exportSubState
	fetchServicePlan(ctx context.Context)
	nuvoGetWriteStat(ctx context.Context)
	setMountState(ctx context.Context, state string)
	allocateCache(ctx context.Context)
	releaseCache(ctx context.Context)
	exportLun(ctx context.Context)
	unExportLun(ctx context.Context)
	unConfigureLun(ctx context.Context)
	reportMetrics(ctx context.Context)
	vsrSaveVSProps(ctx context.Context)
	updateCache(ctx context.Context)
	restoreCache(ctx context.Context)
}

type exportOp struct {
	c                       *Component
	rhs                     *vra.RequestHandlerState
	ops                     exportOperators
	mops                    vra.MountHandlers
	inError                 bool
	snapID                  string
	pitUUID                 string
	isWritable              bool
	resetSnapSchedData      bool
	ignoreFSN               bool // ignore finalSnapshotNeeded
	finalSnapshotNeeded     bool
	mustUnconfigure         bool
	deviceName              string
	writeStats              *nuvoapi.StatsIO
	requestedCacheSizeBytes int64 // actual desired size according to Storage Plan
	allocatedCacheSizeBytes int64 // actual allocated size
}

type exportInternalArgs struct {
	snapID    string
	pitUUID   string
	ignoreFSN bool
}

// vra.AllocationHandlers methods

type exportUnconfigureOp struct{}        // stash key type
type exportInternalCallStashKey struct{} // stash key type
type exportExportOp struct{}             // stash key type for UTs to examine

// Export implements the VOLUME_EXPORT state of a VolumeSeriesRequest
// It is called directly by the Animator for a MOUNT operation but is also
// invoked indirectly via snapshot operations which is stashing snapshot ID and Pit UUID
// in exportInternalCallStashKey.
func (c *Component) Export(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &exportOp{
		c:   c,
		rhs: rhs,
	}
	if v := rhs.StashGet(exportInternalCallStashKey{}); v != nil {
		args := v.(*exportInternalArgs)
		op.pitUUID = args.pitUUID
		op.snapID = args.snapID
		op.isWritable = false
	} else if rhs.Request.SnapIdentifier == "" || rhs.Request.SnapIdentifier == com.VolMountHeadIdentifier {
		op.snapID = com.VolMountHeadIdentifier // in-memory change only
		op.isWritable = true
	}
	op.ops = op                           // self-reference
	op.mops = c                           // component reference
	op.rhs.StashSet(exportExportOp{}, op) // for UTs to examine
	op.run(ctx)
}

// UndoExport implements the UNDO_VOLUME_EXPORT state of VolumeSeriesRequest MOUNT/UNMOUNT operations.
// It is called directly by the Animator for a MOUNT operation but is also
// invoked indirectly via snapshot operations which stash snapshot ID, Pit UUID and ignoreFSN flag
// in exportInternalCallStashKey.
// If the ignoreFSN key is set in the cache stash then the LUN will be un-configured if there are no mounts
// regardless of the value of the finalSnapshotNeeded property.
func (c *Component) UndoExport(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &exportOp{
		c:   c,
		rhs: rhs,
	}
	if v := rhs.StashGet(exportInternalCallStashKey{}); v != nil {
		args := v.(*exportInternalArgs)
		op.snapID = args.snapID
		op.pitUUID = args.pitUUID
		op.ignoreFSN = args.ignoreFSN
	} else if rhs.Request.SnapIdentifier == "" || rhs.Request.SnapIdentifier == com.VolMountHeadIdentifier {
		op.snapID = com.VolMountHeadIdentifier // in-memory change only
		op.isWritable = true
	}
	op.ops = op                           // self-reference
	op.mops = c                           // component reference
	op.rhs.StashSet(exportExportOp{}, op) // for UTs to examine
	op.run(ctx)
}

// ReallocateCache implements the REALLOCATING_CACHE state of a VolumeSeriesRequest CHANGE_CAPACITY operation.
func (c *Component) ReallocateCache(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &exportOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// UndoReallocateCache implements the UNDO_REALLOCATING_CACHE state of VolumeSeriesRequest CHANGE_CAPACITY operation.
func (c *Component) UndoReallocateCache(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &exportOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// run executes the state machine
func (op *exportOp) run(ctx context.Context) {
	jumpToState := ExportNoOp
out:
	for ss := op.ops.getInitialState(ctx); !(op.rhs.InError || op.rhs.RetryLater); ss++ {
		if jumpToState != ExportNoOp {
			ss = jumpToState
			jumpToState = ExportNoOp
		}
		op.c.Log.Debugf("VolumeSeriesRequest %s: %s %s", op.rhs.Request.Meta.ID, op.rhs.Request.VolumeSeriesRequestState, ss)
		switch ss {
		case ExportSetMounting:
			op.ops.setMountState(ctx, com.VolMountStateMounting)
		case ExportFetchServicePlan:
			op.ops.fetchServicePlan(ctx)
		case ExportGetStatUUID:
			op.ops.nuvoGetWriteStat(ctx)
		case ExportAllocateCache:
			op.ops.allocateCache(ctx)
		case ExportLun:
			op.ops.exportLun(ctx)
		case ExportSetMounted:
			op.ops.setMountState(ctx, com.VolMountStateMounted)
		case ExportGetFirstMetric:
			op.ops.reportMetrics(ctx)

		// REALLOCATING_CACHE
		case ExportSaveProps:
			op.ops.vsrSaveVSProps(ctx)
		case ExportUpdateCache:
			op.ops.updateCache(ctx)

		// UNDO_EXPORT
		case ExportSetError:
			op.ops.setMountState(ctx, com.VolMountStateError)
			jumpToState = ExportPublishFinalMetrics
		case ExportSetUnmounting:
			op.ops.setMountState(ctx, com.VolMountStateUnmounting)
		case ExportPublishFinalMetrics:
			op.ops.reportMetrics(ctx)
		case ExportUnexportLun:
			op.ops.unExportLun(ctx)
		case ExportReleaseCache:
			op.ops.releaseCache(ctx)
		case ExportGetStatCount:
			op.ops.nuvoGetWriteStat(ctx)
		case ExportSetUnmounted:
			op.ops.setMountState(ctx, com.VolMountStateUnmounted)
		case ExportDoUnconfigure:
			if op.mustUnconfigure {
				op.ops.unConfigureLun(ctx)
			}

		// UNDO_REALLOCATING_CACHE
		case ExportRestoreCache:
			op.ops.restoreCache(ctx)
		default:
			break out
		}
	}
	op.rhs.InError = op.rhs.InError || op.inError
	op.rhs.StashSet(exportUnconfigureOp{}, op) // for UTs to examine
}

func (op *exportOp) getInitialState(ctx context.Context) exportSubState {
	if swag.BoolValue(op.rhs.Request.PlanOnly) {
		// TBD walk the steps, adding messages to the Request about what Export would do
		return ExportDone
	}
	if op.rhs.Request.VolumeSeriesRequestState == com.VolReqStateReallocatingCache {
		return ExportSaveProps
	} else if op.rhs.Request.VolumeSeriesRequestState == com.VolReqStateUndoReallocatingCache {
		op.inError = op.rhs.InError
		op.rhs.InError = false // allow state machine to run
		return ExportRestoreCache
	}
	if op.snapID == "" || (op.pitUUID == "" && op.snapID != com.VolMountHeadIdentifier) {
		op.rhs.SetRequestError("Either SnapIdentifier [%s] or PiT UUID [%s] missing", op.snapID, op.pitUUID)
		return ExportDone
	}
	op.deviceName = fmt.Sprintf("%s-%s", op.rhs.VolumeSeries.Meta.ID, op.snapID)
	_, m := op.findMount(op.rhs.VolumeSeries, op.snapID)
	if op.rhs.Request.VolumeSeriesRequestState == com.VolReqStateUndoVolumeExport {
		op.inError = op.rhs.InError
		op.rhs.InError = false // allow state machine to run
		if m != nil {
			if m.MountState == com.VolMountStateMounting {
				return ExportSetError
			} else if m.MountState == com.VolMountStateMounted {
				return ExportSetUnmounting
			}
			return ExportUnexportLun
		}
		return ExportUndoDone
	}
	if m != nil {
		switch m.MountState {
		case com.VolMountStateMounting:
			return ExportGetStatUUID
		case com.VolMountStateMounted:
			return ExportDone
		}
	}
	return ExportSetMounting
}

func (op *exportOp) fetchServicePlan(ctx context.Context) {
	// this call adds the servicePlan of this VS to the cache. Metrics needs it to exist in the cache even the export step does not use the servicePlan directly
	if _, err := op.c.App.AppServant.GetServicePlan(ctx, string(op.rhs.VolumeSeries.ServicePlanID)); err != nil {
		op.rhs.RetryLater = true
	}
}

func (op *exportOp) nuvoGetWriteStat(ctx context.Context) {
	if op.snapID != com.VolMountHeadIdentifier {
		return
	}
	nuvoVolumeIdentifier := op.rhs.VolumeSeries.NuvoVolumeIdentifier
	op.c.Log.Debugf("VolumeSeriesRequest %s: VolumeSeries [%s] NUVOAPI GetVolumeStats(false, %s)", op.rhs.Request.Meta.ID, op.rhs.VolumeSeries.Meta.ID, nuvoVolumeIdentifier)
	comboStats, err := op.c.App.NuvoAPI.GetVolumeStats(false, nuvoVolumeIdentifier)
	if err != nil {
		op.c.Log.Warningf("VolumeSeriesRequest %s: NUVOAPI GetVolumeStats(%s): %s", op.rhs.Request.Meta.ID, nuvoVolumeIdentifier, err.Error())
		return // ignore error
	}
	writeStats := &comboStats.IOWrites
	op.c.Log.Debugf("VolumeSeriesRequest %s: VolumeSeries [%s] NUVOAPI GetVolumeStats for write {Count:%d, UUID:%s}", op.rhs.Request.Meta.ID, op.rhs.VolumeSeries.Meta.ID, writeStats.Count, writeStats.SeriesUUID)
	op.writeStats = writeStats
}

func (op *exportOp) setMountState(ctx context.Context, newState string) {
	rhs := op.rhs
	vsID := string(rhs.VolumeSeries.Meta.ID)
	op.resetSnapSchedData = false
	op.finalSnapshotNeeded = false
	if op.snapID == com.VolMountHeadIdentifier {
		if newState == com.VolMountStateMounted {
			op.resetSnapSchedData = true
		} else if newState == com.VolMountStateUnmounted && !op.ignoreFSN {
			op.finalSnapshotNeeded = true
			if op.writeStats != nil {
				stags := util.NewTagList(rhs.VolumeSeries.SystemTags)
				if statsUUID, ok := stags.Get(com.SystemTagVolumeHeadStatSeries); ok && op.writeStats.SeriesUUID == statsUUID {
					strCount := fmt.Sprintf("%d", op.writeStats.Count)
					if statsCount, ok := stags.Get(com.SystemTagVolumeHeadStatCount); ok && statsCount == strCount {
						op.finalSnapshotNeeded = false
					}
				}
			}
			op.c.Log.Debugf("VolumeSeriesRequest %s: finalSnapshotNeeded %v", op.rhs.Request.Meta.ID, op.finalSnapshotNeeded)
		}
	}
	mountMode := com.VolMountModeRO
	if op.isWritable {
		mountMode = com.VolMountModeRW
	}
	errVSUpdateRetryAborted := fmt.Errorf("mountState is already set")
	errMountStateMissing := fmt.Errorf("mountState for [snapID=%s] not found", op.snapID)
	var lastVS *models.VolumeSeries
	var newVolState string
	modVS := func(vs *models.VolumeSeries) (*models.VolumeSeries, error) {
		if vs == nil {
			vs = rhs.VolumeSeries
		}
		lastVS = vs
		i, m := op.findMount(vs, op.snapID)
		if m == nil {
			if newState == com.VolMountStateUnmounted {
				return nil, errVSUpdateRetryAborted
			}
			if newState != com.VolMountStateMounting {
				return nil, errMountStateMissing
			}
			m = &models.Mount{
				SnapIdentifier:    op.snapID,
				PitIdentifier:     op.pitUUID,
				MountMode:         mountMode,
				MountedNodeDevice: op.deviceName,
				MountedNodeID:     rhs.Request.NodeID,
			}
			vs.Mounts = append(vs.Mounts, m)
		}
		if m.MountState == newState {
			return nil, errVSUpdateRetryAborted
		}
		now := strfmt.DateTime(time.Now())
		state := vs.VolumeSeriesState
		stags := util.NewTagList(vs.SystemTags)
		if newState == com.VolMountStateUnmounted {
			vs.Mounts = append(vs.Mounts[:i], vs.Mounts[i+1:]...) // delete the mount from the list
			if len(vs.Mounts) == 0 {
				state = com.VolStateConfigured
			}
			if op.snapID == com.VolMountHeadIdentifier {
				stags.Set(com.SystemTagVolumeLastHeadUnexport, now.String())
				stags.Delete(com.SystemTagVolumeHeadStatSeries)
				stags.Delete(com.SystemTagVolumeHeadStatCount)
				stags.Delete(com.SystemTagVolumeFsAttached)
			}
		} else {
			m.MountState = newState
			m.MountTime = now
			state = com.VolStateInUse
			if newState == com.VolMountStateMounted && op.snapID == com.VolMountHeadIdentifier {
				stags.Delete(com.SystemTagVolumeFsAttached)
				if op.writeStats != nil {
					stags.Set(com.SystemTagVolumeHeadStatSeries, op.writeStats.SeriesUUID)
					stags.Set(com.SystemTagVolumeHeadStatCount, fmt.Sprintf("%d", op.writeStats.Count))
				} else {
					stags.Delete(com.SystemTagVolumeHeadStatSeries)
					stags.Delete(com.SystemTagVolumeHeadStatCount)
				}
			}
		}
		vs.Messages = []*models.TimestampedString{
			&models.TimestampedString{
				Message: fmt.Sprintf("Set mountState [snapID=%s state=%s]", op.snapID, newState),
				Time:    now,
			},
		}
		if state != vs.VolumeSeriesState {
			vs.Messages = append(vs.Messages, &models.TimestampedString{
				Message: fmt.Sprintf("State change %s ⇒ %s", vs.VolumeSeriesState, state),
				Time:    now,
			})
			vs.VolumeSeriesState = state
			if vs.ConfiguredNodeID == "" {
				vs.ConfiguredNodeID = rhs.Request.NodeID
			}
			newVolState = state
		}
		if vs.LifecycleManagementData == nil {
			vs.LifecycleManagementData = &models.LifecycleManagementData{}
		}
		lmd := vs.LifecycleManagementData
		if op.resetSnapSchedData {
			lmd.EstimatedSizeBytes = 0
			lmd.FinalSnapshotNeeded = false
			lmd.GenUUID = ""
			lmd.LastSnapTime = strfmt.DateTime(time.Time{})
			lmd.NextSnapshotTime = now
			lmd.LastUploadTime = strfmt.DateTime(time.Time{})
			lmd.LastUploadSizeBytes = 0
			lmd.LastUploadTransferRateBPS = op.c.App.CSP.ProtectionStoreUploadTransferRate()
			lmd.SizeEstimateRatio = 1.0
		} else if op.finalSnapshotNeeded {
			lmd.FinalSnapshotNeeded = true
			lmd.NextSnapshotTime = now
		}
		vs.SystemTags = stags.List()
		return vs, nil
	}
	items := &crud.Updates{Set: []string{"mounts", "volumeSeriesState", "configuredNodeId", "systemTags"}}
	if op.resetSnapSchedData || op.finalSnapshotNeeded {
		items.Set = append(items.Set, "lifecycleManagementData")
	}
	if !util.Contains(rhs.Request.RequestedOperations, com.VolReqOpVolCreateSnapshot) {
		items.Append = []string{"messages"}
	}
	vs, err := rhs.A.OCrud.VolumeSeriesUpdater(ctx, vsID, modVS, items)
	if err == errVSUpdateRetryAborted {
		vs = lastVS
	} else if err == errMountStateMissing {
		rhs.SetRequestError("%s", err.Error())
		return
	} else if err != nil {
		// assume transient error, update unlikely to be saved
		rhs.SetAndUpdateRequestMessage(ctx, "Failed to update VolumeSeries object [%s]: %s", vsID, err.Error())
		rhs.RetryLater = true
		return
	}
	rhs.VolumeSeries = vs
	if newState == com.VolMountStateUnmounted {
		rhs.Request.MountedNodeDevice = ""
	} else {
		rhs.Request.MountedNodeDevice = op.deviceName
	}
	if err == nil {
		rhs.SetRequestMessage("Set mountState [snapID=%s state=%s]", op.snapID, newState)
	}
	if newVolState != "" {
		rhs.SetRequestMessage("Set volume state [%s]", newVolState)
	}
	if vs.VolumeSeriesState == com.VolStateConfigured && len(vs.Mounts) == 0 && (op.ignoreFSN || !op.finalSnapshotNeeded) {
		op.mustUnconfigure = true
	}
	op.c.Log.Debugf("VolumeSeriesRequest: %s: mustUnconfigure: %v", rhs.Request.Meta.ID, op.mustUnconfigure)
}

func (op *exportOp) getNodeCacheType() (string, error) {
	nodeObj := op.c.App.AppObjects.GetNode() // should always be available at this point
	// TBD: might need to be changed in the future but right now assuming that node may have only one type of cache device (HDD or SSD)
	for _, ls := range nodeObj.LocalStorage {
		return ls.DeviceType, nil
	}
	return "", fmt.Errorf("Local storage data unavailable")
}

// getCacheRequired checks how much cache is required by StoragePlan: desired (SizeBytes in StorageElement) and minimum (provMinSizeBytes)
func (op *exportOp) getCacheRequired() (int64, int64, error) {
	for _, se := range op.rhs.Request.StoragePlan.StorageElements {
		if se.Intent == com.VolReqStgElemIntentCache {
			// as per spec, there should only be a single CACHE storage element
			desiredSB := swag.Int64Value(se.SizeBytes)
			if desiredSB == 0 {
				break
			}
			var minRequiredSB int64
			for spCacheType, spe := range se.StorageParcels {
				// note: StorageParcels is a map but it is not expected to have multiple keys (e.g. both "SSD" and "HDD")
				// TBD: if such expectation is no longer valid then different logic should be implemented
				nodeCacheType, err := op.getNodeCacheType()
				minRequiredSB = swag.Int64Value(spe.ProvMinSizeBytes)
				if (err != nil || spCacheType != nodeCacheType) && minRequiredSB > 0 {
					if err == nil {
						err = fmt.Errorf("Mismatched cache types (Node: %s, StoragePlan: %s) for cache required by StoragePlan", nodeCacheType, spCacheType)
					}
					return 0, 0, err
				}
				break
			}
			return desiredSB, minRequiredSB, nil
		}
	}
	return 0, 0, nil
}

// - create the Claim
// - call Nuvo API to set the cache amount
// - update the node
// - update the VS
func (op *exportOp) allocateCache(ctx context.Context) {
	rhs := op.rhs
	vsID := string(rhs.VolumeSeries.Meta.ID)
	nuvoVolumeIdentifier := op.rhs.VolumeSeries.NuvoVolumeIdentifier
	ca, ok := rhs.VolumeSeries.CacheAllocations[string(rhs.Request.NodeID)]
	if ok && swag.Int64Value(ca.RequestedSizeBytes) != 0 { // nothing to be done on retry if VolumeSeries already has CacheAllocations
		rhs.SetRequestMessage("Cache allocation is already complete")
		return
	}

	rID := string(op.rhs.Request.Meta.ID)
	op.c.Log.Debugf("VolumeSeriesRequest %s: acquiring state lock", rID)
	cst, err := op.c.App.LMDGuard.Enter(rID)
	if err != nil {
		op.rhs.SetRequestMessage("state lock error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.c.Log.Debugf("VolumeSeriesRequest %s: obtained state lock", rID)
	defer func() {
		op.c.Log.Debugf("VolumeSeriesRequest %s: releasing state lock", string(op.rhs.Request.Meta.ID))
		cst.Leave()
	}()

	if err = op.c.rei.ErrOnBool("fail-in-allocate-cache"); err != nil {
		rhs.SetRequestError(err.Error())
		return
	}
	// check if cache is required
	cacheDesiredBytes, cacheRequiredBytes, err := op.getCacheRequired()
	if err != nil {
		rhs.SetRequestError("AllocateCache error: %s", err.Error())
		return
	}
	if cacheDesiredBytes == 0 {
		rhs.SetRequestMessage("Cache allocation is not required")
		return
	}
	// first update agentd cache state
	claimedCacheSizeBytes, err := op.c.App.StateOps.ClaimCache(ctx, vsID, cacheDesiredBytes, cacheRequiredBytes)
	if err != nil {
		rhs.SetRequestError("Failure to update node cache state: %s", err.Error())
		return
	}
	// request Nuvo to set the cache amount
	op.c.Log.Debugf("VolumeSeriesRequest %s: VolumeSeries [%s] NUVOAPI AllocCache(%s, %d)", rhs.Request.Meta.ID, vsID, nuvoVolumeIdentifier, claimedCacheSizeBytes)
	if err = op.c.App.NuvoAPI.AllocCache(nuvoVolumeIdentifier, uint64(claimedCacheSizeBytes)); err != nil {
		op.c.App.StateOps.ReleaseCache(ctx, vsID)
		if nuvoapi.ErrorIsTemporary(err) {
			op.rhs.SetRequestMessage("NUVOAPI temporary error: %s", err.Error())
			op.rhs.RetryLater = true
			return
		}
		rhs.SetRequestError("NUVOAPI AllocCache error: %s", err.Error())
		return
	}
	// update Node object to reflect new cache allocation for this VS (if necessary)
	err = op.c.App.StateOps.UpdateNodeAvailableCache(ctx)
	if err != nil {
		rhs.SetAndUpdateRequestMessage(ctx, "%s", err.Error())
		rhs.RetryLater = true
		return
	}
	// update cache allocation in VS
	modVS := func(vs *models.VolumeSeries) (*models.VolumeSeries, error) {
		nodeID := string(rhs.Request.NodeID)
		if vs == nil {
			vs = rhs.VolumeSeries
		}
		vs.CacheAllocations = map[string]models.CacheAllocation{
			nodeID: models.CacheAllocation{
				AllocatedSizeBytes: swag.Int64(claimedCacheSizeBytes),
				RequestedSizeBytes: swag.Int64(cacheDesiredBytes),
			},
		}
		return vs, nil
	}
	items := &crud.Updates{Append: []string{"cacheAllocations"}}
	vs, err := rhs.A.OCrud.VolumeSeriesUpdater(ctx, vsID, modVS, items)
	if err != nil {
		// assume transient error, update unlikely to be saved
		rhs.SetAndUpdateRequestMessage(ctx, "Failed to update cache allocations for VolumeSeries object [%s]: %s", vsID, err.Error())
		rhs.RetryLater = true
		return
	}
	rhs.VolumeSeries = vs
	op.requestedCacheSizeBytes = cacheDesiredBytes
	op.allocatedCacheSizeBytes = claimedCacheSizeBytes
	rhs.SetRequestMessage("Allocated cache [requested=%s allocated=%s]", util.SizeBytesToString(cacheDesiredBytes), util.SizeBytesToString(claimedCacheSizeBytes))
}

func (op *exportOp) exportLun(ctx context.Context) {
	vsID := string(op.rhs.VolumeSeries.Meta.ID)
	nuvoVolumeIdentifier := op.rhs.VolumeSeries.NuvoVolumeIdentifier
	op.c.Log.Debugf("VolumeSeriesRequest %s: VolumeSeries [%s] NUVOAPI ExportLun(%s, %s, %s, %v)", op.rhs.Request.Meta.ID, vsID, nuvoVolumeIdentifier, op.pitUUID, op.deviceName, op.isWritable)
	err := op.c.rei.ErrOnBool("export-fail-in-export")
	if err == nil {
		err = op.c.App.NuvoAPI.ExportLun(nuvoVolumeIdentifier, op.pitUUID, op.deviceName, op.isWritable)
	}

	if err != nil {
		if nuvoapi.ErrorIsTemporary(err) {
			op.rhs.SetRequestMessage("NUVOAPI temporary error: %s", err.Error())
			op.rhs.RetryLater = true
			return
		} else if !strings.HasPrefix(err.Error(), errLunAlreadyExported) {
			op.rhs.SetRequestError("NUVOAPI ExportLun error: %s", err.Error())
			return
		}
		op.c.Log.Debugf("VolumeSeriesRequest %s: NUVOAPI ExportLun(%s, %s, %s, %v) failed: %s", op.rhs.Request.Meta.ID, nuvoVolumeIdentifier, op.pitUUID, op.deviceName, op.isWritable, err.Error())
	}
	lun := &agentd.LUN{
		VolumeSeriesID:       vsID,
		SnapIdentifier:       op.snapID,
		NuvoVolumeIdentifier: nuvoVolumeIdentifier,
		DeviceName:           op.deviceName,
		DisableMetrics:       op.rhs.HasVolSnapshotRestore,
	}
	if op.snapID == com.VolMountHeadIdentifier {
		lun.RequestedCacheSizeBytes = op.requestedCacheSizeBytes
		lun.AllocatedCacheSizeBytes = op.allocatedCacheSizeBytes
	}
	op.c.App.AppServant.AddLUN(lun)
	op.rhs.SetRequestMessage("Exported [snapID=%s] DisableMetrics:%v", op.snapID, lun.DisableMetrics)
}

func (op *exportOp) unExportLun(ctx context.Context) {
	vsID := string(op.rhs.VolumeSeries.Meta.ID)
	nuvoVolumeIdentifier := op.rhs.VolumeSeries.NuvoVolumeIdentifier
	op.c.Log.Debugf("VolumeSeriesRequest %s: VolumeSeries [%s] NUVOAPI UnexportLun(%s, %s, %s)", op.rhs.Request.Meta.ID, vsID, nuvoVolumeIdentifier, op.pitUUID, op.deviceName)
	err := op.c.App.NuvoAPI.UnexportLun(nuvoVolumeIdentifier, op.pitUUID, op.deviceName)
	if err != nil {
		if nuvoapi.ErrorIsTemporary(err) {
			op.rhs.SetRequestMessage("NUVOAPI temporary error: %s", err.Error())
			op.rhs.RetryLater = true
			return
		} else if !strings.HasPrefix(err.Error(), errLunNotExported) {
			op.rhs.SetRequestError("NUVOAPI UnexportLun error: %s", err.Error())
			return
		}
		op.c.Log.Debugf("VolumeSeriesRequest %s: NUVOAPI UnexportLun(%s, %s, %s) failed: %s", op.rhs.Request.Meta.ID, nuvoVolumeIdentifier, op.pitUUID, op.deviceName, err.Error())
	}
	op.c.App.AppServant.RemoveLUN(vsID, op.snapID)
	op.rhs.SetRequestMessage("Stopped export [snapID=%s]", op.snapID)
}

// - call Nuvo API to set the cache amount back to zero
// - release cache claim
// - update the node
// - update the VS
func (op *exportOp) releaseCache(ctx context.Context) {
	if op.snapID != com.VolMountHeadIdentifier {
		op.c.Log.Debugf("No cache release is needed for non HEAD [vsID=%s snapID=%s name=%s]", op.rhs.VolumeSeries.Meta.ID, op.snapID, op.rhs.VolumeSeries.Name)
		return
	}
	rhs := op.rhs
	vsID := string(rhs.VolumeSeries.Meta.ID)
	nuvoVolumeIdentifier := op.rhs.VolumeSeries.NuvoVolumeIdentifier

	if _, ok := rhs.VolumeSeries.CacheAllocations[string(rhs.Request.NodeID)]; !ok {
		// since VolumeSeries object is the last to be updated when allocating cache, both cache and Node don't require any updates here
		rhs.SetRequestMessage("VolumeSeries [vsID=%s name=%s] is not using any cache on node %s, no cache state or node updates are needed",
			vsID, rhs.VolumeSeries.Name, string(rhs.Request.NodeID))
		return
	}

	rID := string(op.rhs.Request.Meta.ID)
	op.c.Log.Debugf("VolumeSeriesRequest %s: acquiring state lock", rID)
	cst, err := op.c.App.LMDGuard.Enter(rID)
	if err != nil {
		op.rhs.SetRequestMessage("state lock error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.c.Log.Debugf("VolumeSeriesRequest %s: obtained state lock", rID)
	defer func() {
		op.c.Log.Debugf("VolumeSeriesRequest %s: releasing state lock", string(op.rhs.Request.Meta.ID))
		cst.Leave()
	}()

	// Nuvo to release cache (just set to zero)
	op.c.Log.Debugf("VolumeSeriesRequest %s: VolumeSeries [%s] NUVOAPI AllocCache(%s, 0)", rhs.Request.Meta.ID, vsID, nuvoVolumeIdentifier)
	if err = op.c.App.NuvoAPI.AllocCache(nuvoVolumeIdentifier, 0); err != nil {
		if nuvoapi.ErrorIsTemporary(err) {
			op.rhs.SetRequestMessage("NUVOAPI temporary error: %s", err.Error())
			op.rhs.RetryLater = true
			return
		}
		rhs.SetRequestError("NUVOAPI AllocCache error: %s", err.Error())
		return
	}

	// update agentd cache state
	op.c.App.StateOps.ReleaseCache(ctx, vsID)

	// update Node object to reflect released cache allocation for this VS
	if err = op.c.App.StateOps.UpdateNodeAvailableCache(ctx); err != nil {
		rhs.SetAndUpdateRequestMessage(ctx, "%s", err.Error())
		rhs.RetryLater = true
		return
	}

	// update cache allocation in VS (all cache for the node is released) if necessary
	modVS := func(vs *models.VolumeSeries) (*models.VolumeSeries, error) {
		if vs == nil {
			vs = rhs.VolumeSeries
		}
		vs.CacheAllocations = map[string]models.CacheAllocation{
			string(rhs.Request.NodeID): models.CacheAllocation{},
		}
		return vs, nil
	}
	items := &crud.Updates{Remove: []string{"cacheAllocations"}}
	vs, err := rhs.A.OCrud.VolumeSeriesUpdater(ctx, vsID, modVS, items)
	if err != nil {
		// assume transient error, update unlikely to be saved
		rhs.SetAndUpdateRequestMessage(ctx, "Failed to update released cache allocations for VolumeSeries object [%s]: %s", vsID, err.Error())
		rhs.RetryLater = true
		return
	}
	rhs.VolumeSeries = vs
	rhs.SetRequestMessage("Released cache")
}

func (op *exportOp) unConfigureLun(ctx context.Context) {
	op.c.Log.Debugf("VolumeSeriesRequest: %s: UNCONFIGURE", op.rhs.Request.Meta.ID)
	savedState := op.rhs.Request.VolumeSeriesRequestState
	op.rhs.Request.VolumeSeriesRequestState = com.VolReqStateUndoVolumeConfig
	op.mops.UndoConfigure(ctx, op.rhs)
	// restore
	op.rhs.Request.VolumeSeriesRequestState = savedState
}

func (op *exportOp) reportMetrics(ctx context.Context) {
	if op.snapID == com.VolMountHeadIdentifier {
		op.c.App.MetricReporter.ReportVolumeMetric(ctx, string(op.rhs.VolumeSeries.Meta.ID))
	}
}

// vsrSaveVSProps saves the volume series cache allocation in the VSR
// - it only supports the case where cache is allocated only on the node where the HEAD is exported (TBD any other usage)
func (op *exportOp) vsrSaveVSProps(ctx context.Context) {
	sTag := util.NewTagList(op.rhs.Request.SystemTags)
	if _, found := sTag.Get(com.SystemTagVsrCacheAllocation); !found {
		// remember, if there is no entry in the map, "ca", which is not a pointer will be the zero value, with zero allocated and requested size
		ca, _ := op.rhs.VolumeSeries.CacheAllocations[string(op.rhs.Request.NodeID)]
		sTag.Set(com.SystemTagVsrCacheAllocation, fmt.Sprintf("a=%d,r=%d", swag.Int64Value(ca.AllocatedSizeBytes), swag.Int64Value(ca.RequestedSizeBytes)))
		op.rhs.Request.SystemTags = sTag.List()
		items := &crud.Updates{Set: []string{"systemTags"}}
		op.rhs.UpdateRequestWithItems(ctx, items)
	}
}

// - update the Claim if increasing
// - call Nuvo API to set the cache amount
// - update the Claim if decreasing
// - update the node
// - update the VS
// - update the LUN state
func (op *exportOp) updateCache(ctx context.Context) {
	rhs := op.rhs
	rID := string(op.rhs.Request.Meta.ID)
	vsID := string(rhs.VolumeSeries.Meta.ID)
	nuvoVolumeIdentifier := op.rhs.VolumeSeries.NuvoVolumeIdentifier

	op.c.Log.Debugf("VolumeSeriesRequest %s: acquiring state lock", rID)
	cst, err := op.c.App.LMDGuard.Enter(rID)
	if err != nil {
		op.rhs.SetRequestMessage("state lock error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.c.Log.Debugf("VolumeSeriesRequest %s: obtained state lock", rID)
	defer func() {
		op.c.Log.Debugf("VolumeSeriesRequest %s: releasing state lock", string(op.rhs.Request.Meta.ID))
		cst.Leave()
	}()

	lun := op.c.App.AppServant.FindLUN(vsID, com.VolMountHeadIdentifier)
	if lun == nil {
		// can happen if HEAD gets unmounted, in this case nothing left to update
		op.c.Log.Infof("VolumeSeriesRequest %s: vs[%s] %s LUN not found", rID, vsID, com.VolMountHeadIdentifier)
		rhs.SetRequestMessage("%s LUN not found, skipping cache update", com.VolMountHeadIdentifier)
		return
	}
	if err = op.c.rei.ErrOnBool("fail-in-update-cache"); err != nil {
		rhs.SetRequestError(err.Error())
		return
	}

	cacheDesiredBytes, cacheRequiredBytes, err := op.getCacheRequired()
	if err != nil {
		rhs.SetRequestError("UpdateCache error: %s", err.Error())
		return
	}
	// first update agentd cache state when increasing claim
	currentClaimBytes := op.c.App.StateOps.GetClaim(ctx, vsID)
	claimedCacheSizeBytes := currentClaimBytes
	if cacheDesiredBytes > currentClaimBytes {
		if claimedCacheSizeBytes, err = op.c.App.StateOps.ClaimCache(ctx, vsID, cacheDesiredBytes, cacheRequiredBytes); err != nil {
			rhs.SetRequestError("Failure to update node cache state: %s", err.Error())
			return
		}
	} else {
		// precalculate the planned reduction in the claim
		claimedCacheSizeBytes = state.RoundSizeBytesToBlock(cacheDesiredBytes, op.c.App.StateOps.GetNodeCacheAllocationUnitSizeBytes(), false)
	}

	// request Nuvo to set the cache amount
	op.c.Log.Debugf("VolumeSeriesRequest %s: VolumeSeries [%s] NUVOAPI AllocCache(%s, %d)", rhs.Request.Meta.ID, vsID, nuvoVolumeIdentifier, claimedCacheSizeBytes)
	if err = op.c.App.NuvoAPI.AllocCache(nuvoVolumeIdentifier, uint64(claimedCacheSizeBytes)); err != nil {
		// the previous cache allocation in Nuvo is supposed to still be present, restore original claim if necessary
		if cacheDesiredBytes > currentClaimBytes {
			op.c.App.StateOps.ClaimCache(ctx, vsID, currentClaimBytes, currentClaimBytes) // reduction never fails
		}
		if nuvoapi.ErrorIsTemporary(err) {
			op.rhs.SetRequestMessage("NUVOAPI temporary error: %s", err.Error())
			op.rhs.RetryLater = true
			return
		}
		rhs.SetRequestError("NUVOAPI AllocCache error: %s", err.Error())
		return
	}
	if claimedCacheSizeBytes < currentClaimBytes {
		op.c.App.StateOps.ClaimCache(ctx, vsID, claimedCacheSizeBytes, claimedCacheSizeBytes) // reduction never fails
	}
	// update Node object to reflect new cache allocation for this VS (if necessary)
	err = op.c.App.StateOps.UpdateNodeAvailableCache(ctx)
	if err != nil {
		rhs.SetAndUpdateRequestMessage(ctx, "%s", err.Error())
		rhs.RetryLater = true
		return
	}
	old := "unknown"
	if current, ok := rhs.VolumeSeries.CacheAllocations[string(rhs.Request.NodeID)]; ok {
		old = fmt.Sprintf("requested=%s allocated=%s", util.SizeBytesToString(swag.Int64Value(current.RequestedSizeBytes)), util.SizeBytesToString(swag.Int64Value(current.AllocatedSizeBytes)))
	}
	// update cache allocation in VS
	modVS := func(vs *models.VolumeSeries) (*models.VolumeSeries, error) {
		nodeID := string(rhs.Request.NodeID)
		if vs == nil {
			vs = rhs.VolumeSeries
		}
		vs.CacheAllocations = map[string]models.CacheAllocation{
			nodeID: models.CacheAllocation{
				AllocatedSizeBytes: swag.Int64(claimedCacheSizeBytes),
				RequestedSizeBytes: swag.Int64(cacheDesiredBytes),
			},
		}
		return vs, nil
	}
	items := &crud.Updates{Append: []string{"cacheAllocations"}}
	vs, err := rhs.A.OCrud.VolumeSeriesUpdater(ctx, vsID, modVS, items)
	if err != nil {
		// assume transient error, update unlikely to be saved
		rhs.SetAndUpdateRequestMessage(ctx, "Failed to update cache allocations: %s", err.Error())
		rhs.RetryLater = true
		return
	}
	rhs.VolumeSeries = vs
	lun.RequestedCacheSizeBytes = cacheDesiredBytes
	lun.AllocatedCacheSizeBytes = claimedCacheSizeBytes
	rhs.SetRequestMessage("Updated cache allocation old:[%s] new:[requested=%s allocated=%s]", old, util.SizeBytesToString(cacheDesiredBytes), util.SizeBytesToString(claimedCacheSizeBytes))
}

// - restore cache first if restored value is an increase (could fail and abort)
// - call Nuvo API to restore cache amount
// - restore cache first if restored value is a decrease
// - update the node
// - update the VS
func (op *exportOp) restoreCache(ctx context.Context) {
	rhs := op.rhs
	rID := string(rhs.Request.Meta.ID)
	vsID := string(rhs.VolumeSeries.Meta.ID)
	nuvoVolumeIdentifier := op.rhs.VolumeSeries.NuvoVolumeIdentifier

	stags := util.NewTagList(rhs.Request.SystemTags)
	caStr, ok := stags.Get(com.SystemTagVsrCacheAllocation)
	if !ok {
		// no previous cache allocation stashed, nothing to restore
		rhs.SetRequestMessage("Cache was not updated, restore is not required")
		return
	}
	var allocatedToRestore, requestedToRestore int64
	if _, err := fmt.Sscanf(caStr, "a=%d,r=%d", &allocatedToRestore, &requestedToRestore); err != nil {
		op.c.Log.Errorf("VolumeSeriesRequest %s: invalid [%s:%s] ignored (%s)", rID, com.SystemTagVsrCacheAllocation, caStr, err.Error())
		rhs.SetRequestMessage("Cache update tag is invalid, skipping cache restore")
		return
	}

	op.c.Log.Debugf("VolumeSeriesRequest %s: acquiring state lock", rID)
	cst, err := op.c.App.LMDGuard.Enter(rID)
	if err != nil {
		op.rhs.SetRequestMessage("state lock error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.c.Log.Debugf("VolumeSeriesRequest %s: obtained state lock", rID)
	defer func() {
		op.c.Log.Debugf("VolumeSeriesRequest %s: releasing state lock", string(op.rhs.Request.Meta.ID))
		cst.Leave()
	}()

	currentClaimBytes := op.c.App.StateOps.GetClaim(ctx, vsID)
	if allocatedToRestore > currentClaimBytes {
		// It is unlikely that the VSR will fail after having reduced its claim, because updateCache() has no explicit Fail transitions
		// after successfully updating Nuvo.
		// However, if such a failure does occur (e.g. timed out), we may be unable to re-claim the cache increase. If that occurs, abort.
		_, err := op.c.App.StateOps.ClaimCache(ctx, vsID, allocatedToRestore, allocatedToRestore)
		if err != nil {
			op.rhs.SetRequestError("Restore Cache aborted: %s", err.Error())
			op.rhs.AbortUndo = true
			return
		}
	}

	// Nuvo to restore cache allocation
	op.c.Log.Debugf("VolumeSeriesRequest %s: VolumeSeries [%s] NUVOAPI AllocCache(%s, %d)", rhs.Request.Meta.ID, vsID, nuvoVolumeIdentifier, allocatedToRestore)
	if err = op.c.App.NuvoAPI.AllocCache(nuvoVolumeIdentifier, uint64(allocatedToRestore)); err != nil {
		if allocatedToRestore > currentClaimBytes {
			op.c.App.StateOps.ClaimCache(ctx, vsID, currentClaimBytes, currentClaimBytes) // reduction never fails
		}
		if nuvoapi.ErrorIsTemporary(err) {
			op.rhs.SetRequestMessage("NUVOAPI temporary error: %s", err.Error())
			op.rhs.RetryLater = true
			return
		}
		rhs.SetRequestError("NUVOAPI AllocCache error, aborted: %s", err.Error())
		// also in this case, the undo cannot continue, just abort
		op.rhs.AbortUndo = true
		return
	}
	if allocatedToRestore < currentClaimBytes {
		op.c.App.StateOps.ClaimCache(ctx, vsID, allocatedToRestore, allocatedToRestore) // reduction never fails
	}

	// update Node object to reflect restored cache allocation for this VS
	err = op.c.App.StateOps.UpdateNodeAvailableCache(ctx)
	if err != nil {
		rhs.SetAndUpdateRequestMessage(ctx, "%s", err.Error())
		rhs.RetryLater = true
		return
	}

	// update cache allocation in VS (all cache for the node is released) if necessary
	modVS := func(vs *models.VolumeSeries) (*models.VolumeSeries, error) {
		if vs == nil {
			vs = rhs.VolumeSeries
		}
		vs.CacheAllocations = map[string]models.CacheAllocation{
			string(rhs.Request.NodeID): models.CacheAllocation{
				AllocatedSizeBytes: swag.Int64(allocatedToRestore),
				RequestedSizeBytes: swag.Int64(requestedToRestore),
			},
		}
		return vs, nil
	}
	items := &crud.Updates{Append: []string{"cacheAllocations"}}
	vs, err := rhs.A.OCrud.VolumeSeriesUpdater(ctx, vsID, modVS, items)
	if err != nil {
		// assume transient error, update unlikely to be saved
		rhs.SetAndUpdateRequestMessage(ctx, "Failed to update restored cache allocations: %s", err.Error())
		rhs.RetryLater = true
		return
	}
	rhs.VolumeSeries = vs
	if lun := op.c.App.AppServant.FindLUN(vsID, com.VolMountHeadIdentifier); lun != nil {
		lun.AllocatedCacheSizeBytes = allocatedToRestore
		lun.RequestedCacheSizeBytes = requestedToRestore
	} else {
		op.c.Log.Debugf("VolumeSeriesRequest %s: FindLUN(%s, %s) not found", rID, vsID, com.VolMountHeadIdentifier)
	}
	rhs.SetRequestMessage("Restored cache allocation [requested=%s allocated=%s]", util.SizeBytesToString(requestedToRestore), util.SizeBytesToString(allocatedToRestore))
}

func (op *exportOp) findMount(vs *models.VolumeSeries, snapID string) (int, *models.Mount) {
	for i, m := range vs.Mounts {
		if m.SnapIdentifier == snapID {
			return i, m
		}
	}
	return 0, nil
}
