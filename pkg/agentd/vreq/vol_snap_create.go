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
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/snapshot"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/mount"
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	"github.com/Nuvoloso/kontroller/pkg/pstore"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

// vra.VolSnapshotCreateHandlers methods

// VolSnapshotCreate performs the VOL_SNAPSHOT_CREATE operation
// This handler will be invoked multiple times for the same request, one per externally visible VSR state.
// It stashes the state machine in the RHS to minimize the overhead of the latter calls.
func (c *Component) VolSnapshotCreate(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := c.volSnapCreateRecoverOrMakeOp(rhs)
	op.run(ctx)
}

// UndoVolSnapshotCreate undoes the VOL_SNAPSHOT_CREATE operation
// This handler will be invoked multiple times for the same request, one per externally visible VSR state.
// It stashes the state machine in the RHS to minimize the overhead of the latter calls.
func (c *Component) UndoVolSnapshotCreate(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := c.volSnapCreateRecoverOrMakeOp(rhs)
	op.run(ctx)
}

type volSnapCreateStashKey struct{} // stash key type

// volSnapCreateRecoverOrMakeOp retrieves the Op structure from the rhs stash, or creates a new one
func (c *Component) volSnapCreateRecoverOrMakeOp(rhs *vra.RequestHandlerState) *volSnapCreateOp {
	var op *volSnapCreateOp
	if v := rhs.StashGet(volSnapCreateStashKey{}); v != nil {
		if stashedOp, ok := v.(*volSnapCreateOp); ok {
			op = stashedOp
		}
	}
	if op == nil {
		op = &volSnapCreateOp{}
	}
	op.c = c
	op.mops = c // component reference
	op.rhs = rhs
	op.ops = op // self-reference
	return op
}

type volSnapCreateSubState int

// volSnapCreateSubState values (the ordering is meaningful)
const (
	// PAUSING_IO
	VolSCListPiTs volSnapCreateSubState = iota
	VolSCListSnaps
	VolSCPausingIO        // this notifies nuvo to budget capacity
	VolSCFreezeFilesystem // for the freeze
	VolSCPausingIOBreakOut

	// PAUSED_IO
	VolSCPausedIOSync
	VolSCPausedIOSyncBreakOut

	// CREATING_PIT
	VolSCTimestampPiT
	VolSCCreatePiT
	VolSCGetWriteStats
	VolSCCreatePitBreakout

	// CREATED_PIT
	VolSCCreatedPitSync
	VolSCResumeIO           // resume and unfreeze are not in
	VolSCUnfreezeFilesystem // the inverse order of pause and freeze
	VolSCCreatedPitSyncBreakout

	// SNAPSHOT_UPLOADING
	VolSCSnapshotUploadPlan
	VolSCExportTipPiT
	VolSCUploadSnapshot
	VolSCSnapshotUploadingBreakout

	// SNAPSHOT_UPLOADED
	VolSCSnapshotUploadSync
	VolSCSnapshotUploadSyncBreakout

	// FINALIZING_SNAPSHOT
	VolSCFinalizeStart
	VolSCGetServicePlan
	VolSCRemoveOlderPiTs
	VolSCUnexportTipPiT
	VolSCCreateSnapshotObject
	VolSCWaitForLMDGuard
	VolSCFinalizeSnapshotDataInVolume
	VolSCReleaseLMDGuard
	VolSCFinalizeSync
	VolSCDone

	// UNDO_SNAPSHOT_UPLOADING
	VolSCUndoSnapshotUploadStart
	VolSCUndoDeleteSnapshot
	VolSCUndoExportTipPiT
	VolSCUndoSnapshotUploadBreakout

	// UNDO_CREATING_PIT
	VolSCUndoCreatingPitStart
	VolSCUndoCreatePiT
	VolSCUndoCreatingPitBreakout

	// UNDO_PAUSING_IO
	VolSCUndoPausingIO
	VolSCUndoFreezeFilesystem
	VolSCUndoPausingIOBreakOut

	VolSCError

	// LAST: No operation is performed in this state.
	VolSCNoOp
)

func (ss volSnapCreateSubState) String() string {
	switch ss {
	case VolSCListPiTs:
		return "VolSCListPiTs"
	case VolSCListSnaps:
		return "VolSCListSnaps"
	case VolSCPausingIO:
		return "VolSCPausingIO"
	case VolSCFreezeFilesystem:
		return "VolSCFreezeFilesystem"
	case VolSCPausingIOBreakOut:
		return "VolSCPausingIOBreakOut"
	case VolSCPausedIOSync:
		return "VolSCPausedIOSync"
	case VolSCPausedIOSyncBreakOut:
		return "VolSCPausedIOSyncBreakOut"
	case VolSCTimestampPiT:
		return "VolSCTimestampPiT"
	case VolSCCreatePiT:
		return "VolSCCreatePiT"
	case VolSCGetWriteStats:
		return "VolSCGetWriteStats"
	case VolSCCreatePitBreakout:
		return "VolSCCreatePitBreakout"
	case VolSCResumeIO:
		return "VolSCResumeIO"
	case VolSCUnfreezeFilesystem:
		return "VolSCUnfreezeFilesystem"
	case VolSCCreatedPitSync:
		return "VolSCCreatedPitSync"
	case VolSCCreatedPitSyncBreakout:
		return "VolSCCreatedPitSyncBreakout"
	case VolSCSnapshotUploadPlan:
		return "VolSCSnapshotUploadPlan"
	case VolSCExportTipPiT:
		return "VolSCExportTipPiT"
	case VolSCUploadSnapshot:
		return "VolSCUploadSnapshot"
	case VolSCSnapshotUploadingBreakout:
		return "VolSCSnapshotUploadingBreakout"
	case VolSCSnapshotUploadSync:
		return "VolSCSnapshotUploadSync"
	case VolSCSnapshotUploadSyncBreakout:
		return "VolSCSnapshotUploadSyncBreakout"
	case VolSCFinalizeStart:
		return "VolSCFinalizePlan"
	case VolSCGetServicePlan:
		return "VolSCGetServicePlan"
	case VolSCRemoveOlderPiTs:
		return "VolSCRemoveOlderPiTs"
	case VolSCUnexportTipPiT:
		return "VolSCUnexportTipPiT"
	case VolSCCreateSnapshotObject:
		return "VolSCCreateSnapshotObject"
	case VolSCWaitForLMDGuard:
		return "VolSCWaitForLMDGuard"
	case VolSCFinalizeSnapshotDataInVolume:
		return "VolSCFinalizeSnapshotDataInVolume"
	case VolSCReleaseLMDGuard:
		return "VolSCReleaseLMDGuard"
	case VolSCFinalizeSync:
		return "VolSCFinalizeSync"
	case VolSCDone:
		return "VolSCDone"
	case VolSCUndoSnapshotUploadStart:
		return "VolSCUndoSnapshotUploadPlan"
	case VolSCUndoDeleteSnapshot:
		return "VolSCUndoDeleteSnapshot"
	case VolSCUndoExportTipPiT:
		return "VolSCUndoExportTipPiT"
	case VolSCUndoSnapshotUploadBreakout:
		return "VolSCUndoSnapshotUploadBreakout"
	case VolSCUndoCreatingPitStart:
		return "VolSCUndoCreatingPitPlan"
	case VolSCUndoCreatePiT:
		return "VolSCUndoCreatePiT"
	case VolSCUndoCreatingPitBreakout:
		return "VolSCUndoCreatingPitBreakout"
	case VolSCUndoPausingIO:
		return "VolSCUndoPausingIO"
	case VolSCUndoFreezeFilesystem:
		return "VolSCUndoFreezeFilesystem"
	case VolSCUndoPausingIOBreakOut:
		return "VolSCUndoPausingIOBreakOut"
	case VolSCError:
		return "VolSCError"
	}
	return fmt.Sprintf("volSnapCreateSubState(%d)", ss)
}

type volSnapCreateOp struct {
	baseOp
	ops                volSnapCreateOperators
	mops               vra.MountHandlers
	advertisedFailure  bool
	inCgRace           bool
	sa                 *vra.SyncArgs
	saa                *vra.SyncAbortArgs
	vsSnapshots        []*models.Snapshot // sorted in descending order by the time of creation (latest first)
	pits               []string
	tipPiT             string
	tipSnapID          string
	tipExported        bool
	basePiT            string // if present
	baseSnapIdentifier string // if present
	fsTarget           string // if present
	clObj              *models.Cluster
	domObj             *models.CSPDomain
	pdObj              *models.ProtectionDomain
	spObj              *models.ServicePlan
	lmdCST             *util.CriticalSectionTicket
}

type volSnapCreateOperators interface {
	createSnapshotObject(ctx context.Context)
	exportTip(ctx context.Context)
	fetchServicePlan(ctx context.Context)
	finalizeSnapshotDataInVolume(ctx context.Context)
	fsFreeze(ctx context.Context)
	fsUnfreeze(ctx context.Context)
	getInitialState(ctx context.Context) volSnapCreateSubState
	handleCGStartupRace(ctx context.Context)
	listVolumeSnapshots(ctx context.Context)
	lmdGuardLock(ctx context.Context)
	lmdGuardUnlock(ctx context.Context)
	nuvoCreatePiT(ctx context.Context)
	nuvoDeletePiT(ctx context.Context, pitUUID string)
	nuvoGetWriteStat(ctx context.Context)
	nuvoListPiTs(ctx context.Context)
	nuvoPauseIO(ctx context.Context)  // from baseOp
	nuvoResumeIO(ctx context.Context) // from baseOp
	planCopy(ctx context.Context)
	psDelete(ctx context.Context)
	psUpload(ctx context.Context)
	removeOlderPiTs(ctx context.Context)
	syncFail(ctx context.Context)
	syncState(ctx context.Context, adjustCB bool)
	timestampPiT(ctx context.Context)
	unexportTip(ctx context.Context)
}

func (op *volSnapCreateOp) run(ctx context.Context) {
	// Handle the CG startup race condition before entering the state machine.
	// Once we're past the PAUSING_IO state we can assume that this race condition does not exist.
	if op.rhs.Request.SyncCoordinatorID != "" && op.rhs.Request.VolumeSeriesRequestState == com.VolReqStatePausingIO {
		if op.ops.handleCGStartupRace(ctx); op.inCgRace || op.rhs.RetryLater {
			return
		}
	}
out:
	for ss := op.ops.getInitialState(ctx); !(op.rhs.InError || op.rhs.RetryLater); ss++ {
		op.c.Log.Debugf("VolumeSeriesRequest %s: %s %s", op.rhs.Request.Meta.ID, op.rhs.Request.VolumeSeriesRequestState, ss)
		switch ss {
		// PAUSING_IO
		case VolSCListPiTs:
			op.ops.nuvoListPiTs(ctx)
		case VolSCListSnaps:
			op.ops.listVolumeSnapshots(ctx)
		case VolSCPausingIO:
			op.ops.nuvoPauseIO(ctx)
		case VolSCFreezeFilesystem:
			op.ops.fsFreeze(ctx)
		// PAUSED_IO
		case VolSCPausedIOSync:
			op.ops.syncState(ctx, false)
		// CREATING_PIT
		case VolSCTimestampPiT:
			if !op.pitCreated() {
				op.ops.timestampPiT(ctx)
			}
		case VolSCCreatePiT:
			if !op.pitCreated() {
				op.ops.nuvoCreatePiT(ctx)
			}
		case VolSCGetWriteStats:
			op.ops.nuvoGetWriteStat(ctx)
		// CREATED_PIT
		case VolSCCreatedPitSync:
			op.ops.syncState(ctx, false)
		case VolSCResumeIO:
			op.ops.nuvoResumeIO(ctx)
		case VolSCUnfreezeFilesystem:
			op.ops.fsUnfreeze(ctx)
		// SNAPSHOT_UPLOADING
		case VolSCSnapshotUploadPlan:
			if op.pits == nil {
				op.ops.nuvoListPiTs(ctx)
			}
			if op.vsSnapshots == nil {
				op.ops.listVolumeSnapshots(ctx)
			}
			op.rhs.InError = false // ignore errors
			op.ops.planCopy(ctx)
		case VolSCExportTipPiT:
			if !op.tipExported {
				op.ops.exportTip(ctx)
			}
		case VolSCUploadSnapshot:
			op.ops.psUpload(ctx)
		// SNAPSHOT_UPLOADED
		case VolSCSnapshotUploadSync:
			op.ops.syncState(ctx, true)
		// FINALIZING_SNAPSHOT
		case VolSCFinalizeStart:
			if op.pits == nil {
				op.ops.nuvoListPiTs(ctx)
			}
			if op.vsSnapshots == nil {
				op.ops.listVolumeSnapshots(ctx)
			}
			op.rhs.InError = false // ignore errors
		case VolSCGetServicePlan:
			op.ops.fetchServicePlan(ctx)
		case VolSCRemoveOlderPiTs:
			op.ops.removeOlderPiTs(ctx)
			op.rhs.InError = false // ignore error
		case VolSCUnexportTipPiT:
			op.ops.unexportTip(ctx) // will close the log volume if no exports left
			op.rhs.InError = false  // ignore error
		case VolSCCreateSnapshotObject:
			op.ops.createSnapshotObject(ctx)
		case VolSCWaitForLMDGuard:
			op.ops.lmdGuardLock(ctx)
		case VolSCFinalizeSnapshotDataInVolume:
			op.ops.finalizeSnapshotDataInVolume(ctx)
		case VolSCReleaseLMDGuard:
			op.ops.lmdGuardUnlock(ctx)
		case VolSCFinalizeSync:
			op.ops.syncState(ctx, true)
		// UNDO_SNAPSHOT_UPLOADING
		case VolSCUndoSnapshotUploadStart:
			if op.pits == nil {
				op.ops.nuvoListPiTs(ctx)
				op.rhs.InError = false // ignore error
			}
			op.ops.planCopy(ctx)
			op.rhs.InError = false // ignore error
		case VolSCUndoDeleteSnapshot:
			op.ops.psDelete(ctx)
		case VolSCUndoExportTipPiT:
			if op.tipExported {
				op.ops.unexportTip(ctx)
				op.rhs.InError = false // ignore error
			}
		// UNDO_CREATING_PIT
		case VolSCUndoCreatingPitStart:
			if op.pits == nil {
				op.ops.nuvoListPiTs(ctx)
				op.rhs.InError = false // ignore error
			}
		case VolSCUndoCreatePiT:
			if op.pitCreated() {
				op.ops.nuvoDeletePiT(ctx, op.rhs.Request.Snapshot.PitIdentifier)
			}
		// UNDO_PAUSING_IO
		case VolSCUndoPausingIO:
			op.ops.nuvoResumeIO(ctx)
		case VolSCUndoFreezeFilesystem:
			op.ops.fsUnfreeze(ctx)
		default:
			break out
		}
	}
	if op.lmdCST != nil {
		op.ops.lmdGuardUnlock(ctx)
	}
	if op.inError {
		op.rhs.InError = true
	}
	if op.rhs.InError && op.rhs.Request.SyncCoordinatorID != "" {
		op.ops.syncFail(ctx)
	}
	if !(op.rhs.RetryLater || op.rhs.InError) {
		op.rhs.StashSet(volSnapCreateStashKey{}, op)
	}
}

var volSnapCreateUndoStates = []string{
	com.VolReqStateUndoSnapshotUploadDone,
	com.VolReqStateUndoSnapshotUploading,
	com.VolReqStateUndoCreatedPiT,
	com.VolReqStateUndoCreatingPiT,
	com.VolReqStateUndoPausedIO,
	com.VolReqStateUndoPausingIO,
}

func (op *volSnapCreateOp) getInitialState(ctx context.Context) volSnapCreateSubState {
	op.planOnly = swag.BoolValue(op.rhs.Request.PlanOnly)
	if util.Contains(volSnapCreateUndoStates, op.rhs.Request.VolumeSeriesRequestState) {
		op.inError = op.rhs.InError
		op.rhs.InError = false // switch off to enable cleanup
		if op.planOnly {
			return VolSCDone
		}
		if err := op.c.rei.ErrOnBool("vsc-no-undo"); err != nil {
			op.rhs.SetRequestMessage("Undo logic suppressed: %s", err.Error())
			return VolSCDone
		}
	} else if !vra.VolumeSeriesIsConfigured(op.rhs.VolumeSeries.VolumeSeriesState) {
		// e.g. nuvo restarted while this VSR was running. Just fail and allow the snapper to create a new snapshot VSR
		op.rhs.SetRequestError("Volume %s is no longer %s", op.rhs.VolumeSeries.Meta.ID, com.VolStateConfigured)
		return VolSCDone
	}
	switch op.rhs.Request.VolumeSeriesRequestState {
	case com.VolReqStatePausingIO:
		if err := op.c.rei.ErrOnBool("vsc-block-on-start"); err != nil {
			op.rhs.SetAndUpdateRequestMessageDistinct(ctx, "Blocked on [%s]", err.Error())
			op.rhs.RetryLater = true
			return VolSCDone
		}
		return VolSCListPiTs
	case com.VolReqStatePausedIO:
		return VolSCPausedIOSync
	case com.VolReqStateCreatingPiT:
		return VolSCTimestampPiT
	case com.VolReqStateCreatedPiT:
		return VolSCCreatedPitSync
	case com.VolReqStateSnapshotUploading:
		return VolSCSnapshotUploadPlan
	case com.VolReqStateSnapshotUploadDone:
		return VolSCSnapshotUploadSync
	case com.VolReqStateFinalizingSnapshot:
		return VolSCFinalizeStart
	case com.VolReqStateUndoPausingIO:
		return VolSCUndoPausingIO
	case com.VolReqStateUndoCreatingPiT:
		return VolSCUndoCreatingPitStart
	case com.VolReqStateUndoSnapshotUploading:
		return VolSCUndoSnapshotUploadStart
	}
	return VolSCDone
}

var errVscCGRaceBreakout = fmt.Errorf("cg-race-not-an-error")

// handleCGStartupRace applies when launched from a CG snapshot.
// If the volume CG is not equal to the parent VSR CG then remove this VSR
// from the parent syncPeers map and fail.
func (op *volSnapCreateOp) handleCGStartupRace(ctx context.Context) {
	modifyFn := func(o *models.VolumeSeriesRequest) (*models.VolumeSeriesRequest, error) {
		if o == nil {
			return nil, nil // always fetch
		}
		var err error
		if op.checkIfCgStartupRace(o); !op.inCgRace {
			err = errVscCGRaceBreakout
		}
		return o, err
	}
	items := &crud.Updates{Set: []string{"syncPeers"}}
	// TBD: decide if we want to update the parent state. Not a common race condition so may not be worth it.
	_, err := op.c.oCrud.VolumeSeriesRequestUpdater(ctx, string(op.rhs.Request.SyncCoordinatorID), modifyFn, items)
	if err != nil && err != errVscCGRaceBreakout {
		op.rhs.RetryLater = true
	}
}

func (op *volSnapCreateOp) checkIfCgStartupRace(o *models.VolumeSeriesRequest) {
	err := op.c.rei.ErrOnBool("vsc-cg-race")
	if !op.inCgRace && (o.ConsistencyGroupID != op.rhs.VolumeSeries.ConsistencyGroupID || err != nil) {
		op.inCgRace = true
		var msg string
		if err == nil {
			msg = fmt.Sprintf("Volume consistency group [%s] changed since group snapshot launched on [%s]", op.rhs.VolumeSeries.ConsistencyGroupID, o.ConsistencyGroupID)
		} else {
			msg = fmt.Sprintf("CG startup race: %s", err.Error())
		}
		op.rhs.SetRequestError("%s", msg)
		// remove this VS from the map
		delete(o.SyncPeers, string(op.rhs.VolumeSeries.Meta.ID))
	}
}

func (op *volSnapCreateOp) exportTip(ctx context.Context) {
	if op.planOnly {
		op.rhs.SetRequestMessage("Export LUN for Snapshot %s with PiT %s", op.tipSnapID, op.tipPiT)
		return
	}
	op.exportLun(ctx, op.tipSnapID, op.tipPiT)
}

func (op *volSnapCreateOp) fetchServicePlan(ctx context.Context) {
	spID := string(op.rhs.VolumeSeries.ServicePlanID)
	if op.planOnly {
		op.rhs.SetRequestMessage("Fetch ServicePlan %s", spID)
		return
	}
	var err error
	op.spObj, err = op.c.App.AppServant.GetServicePlan(ctx, spID)
	if err != nil {
		op.rhs.RetryLater = true
	}
	return
}

// finalizeSnapshotDataInVolume sets the snapshot scheduler data and system tags in the volume
func (op *volSnapCreateOp) finalizeSnapshotDataInVolume(ctx context.Context) {
	vsID := string(op.rhs.VolumeSeries.Meta.ID)
	if op.planOnly {
		op.rhs.SetRequestMessage("Finalize snapshot in volume")
		return
	}
	rpoDur := op.c.App.GetRPO(op.spObj)
	op.rhs.Request.LifecycleManagementData.LastSnapTime = op.rhs.Request.Snapshot.SnapTime
	interimBytes := int64(0)
	if lun := op.c.App.AppServant.FindLUN(vsID, ""); lun != nil && lun.LastWriteStat != nil && lun.LastWriteStat.SeriesUUID == op.rhs.Request.LifecycleManagementData.GenUUID {
		interimBytes = int64(lun.LastWriteStat.SizeTotal) - op.rhs.Request.LifecycleManagementData.EstimatedSizeBytes
		op.c.Log.Debugf("VolumeSeriesRequest %s: interimBytes=%d", op.rhs.Request.Meta.ID, interimBytes)
	}
	var buf bytes.Buffer
	modVS := func(vs *models.VolumeSeries) (*models.VolumeSeries, error) {
		if vs == nil {
			vs = op.rhs.VolumeSeries
		}
		lmd := op.rhs.Request.LifecycleManagementData
		if vs.LifecycleManagementData != nil {
			lmd.EstimatedSizeBytes = vs.LifecycleManagementData.EstimatedSizeBytes
			lmd.SizeEstimateRatio = vs.LifecycleManagementData.SizeEstimateRatio
			lmd.FinalSnapshotNeeded = vs.LifecycleManagementData.FinalSnapshotNeeded
		}
		buf.Reset()
		vs.LifecycleManagementData, _ = op.computeVsSchedulerDataAfterSnapshot(&buf, lmd, rpoDur, interimBytes)
		sysTags := util.NewTagList(vs.SystemTags)
		sysTags.Set(com.SystemTagVolumeHeadStatSeries, lmd.GenUUID)                        // impacts finalSnapshotNeeded
		sysTags.Set(com.SystemTagVolumeHeadStatCount, fmt.Sprintf("%d", lmd.WriteIOCount)) // computation in UNMOUNT
		// clear the FinalSnapshotNeeded property if the volume HEAD was not un-exported since the start of this VSR
		vsVal, _ := sysTags.Get(com.SystemTagVolumeLastHeadUnexport)
		vsrTags := util.NewTagList(op.rhs.Request.SystemTags)
		vsrVal, _ := vsrTags.Get(com.SystemTagVolumeLastHeadUnexport)
		if vsVal == vsrVal && vs.LifecycleManagementData.FinalSnapshotNeeded {
			fmt.Fprintf(&buf, "FinalSnapshotNeeded=false ")
			vs.LifecycleManagementData.FinalSnapshotNeeded = false
		}
		vs.SystemTags = sysTags.List()
		return vs, nil
	}
	items := &crud.Updates{Set: []string{"lifecycleManagementData", "systemTags"}}
	vs, err := op.rhs.A.OCrud.VolumeSeriesUpdater(ctx, vsID, modVS, items)
	if err != nil {
		op.rhs.SetRequestMessage("Error updating volume: %s", err.Error()) // unlikely to be saved
		op.rhs.RetryLater = true
		return
	}
	op.c.Log.Debugf("VolumeSeriesRequest %s: LMD %s", op.rhs.Request.Meta.ID, buf.String())
	op.rhs.VolumeSeries = vs
	op.rhs.SetRequestMessage("Finalized snapshot in volume")
}

func (op *volSnapCreateOp) fsFreeze(ctx context.Context) {
	op.doFreezeUnfreeze(ctx, true)
}

func (op *volSnapCreateOp) fsUnfreeze(ctx context.Context) {
	op.doFreezeUnfreeze(ctx, false)
}

func (op *volSnapCreateOp) listVolumeSnapshots(ctx context.Context) {
	vsID := string(op.rhs.VolumeSeries.Meta.ID)
	if len(op.pits) > 0 {
		if op.planOnly {
			op.rhs.SetRequestMessage("Fetching snapshots for volume[%s] PiTs[%v]", vsID, op.pits)
			return
		}
		lParams := snapshot.NewSnapshotListParams()
		lParams.VolumeSeriesID = &vsID
		lParams.PitIdentifiers = op.pits
		res, err := op.rhs.A.OCrud.SnapshotList(ctx, lParams)
		if err != nil {
			op.rhs.SetRequestError("Error listing volume %s snapshots: %s", vsID, err.Error())
			return
		}
		// sort stashed snapshots by the time of creation (most recent first)
		op.vsSnapshots = util.SnapshotListToSorted(res.Payload, util.SnapshotSortOnSnapTime)
	} else {
		if op.planOnly {
			op.rhs.SetRequestMessage("No PiTs present - snapshots not fetched")
			return
		}
		op.vsSnapshots = nil
	}
}

func (op *volSnapCreateOp) nuvoCreatePiT(ctx context.Context) {
	pitUUID := op.rhs.Request.Snapshot.PitIdentifier
	if op.planOnly {
		op.rhs.SetRequestMessage("Create PiT %s", pitUUID)
		return
	}
	nuvoVolumeIdentifier := op.rhs.VolumeSeries.NuvoVolumeIdentifier
	op.c.Log.Debugf("VolumeSeriesRequest %s: VolumeSeries [%s] NUVOAPI CreatePit(%s, %s)", op.rhs.Request.Meta.ID, op.rhs.VolumeSeries.Meta.ID, nuvoVolumeIdentifier, pitUUID)
	err := op.c.App.NuvoAPI.CreatePit(nuvoVolumeIdentifier, pitUUID)
	if err != nil {
		if nuvoapi.ErrorIsTemporary(err) {
			op.rhs.SetRequestMessage("NUVOAPI temporary error: %s", err.Error())
			op.rhs.RetryLater = true
			return
		}
		op.rhs.SetRequestError("NUVOAPI CreatePit(%s, %s) failed: %s", nuvoVolumeIdentifier, pitUUID, err.Error())
		return
	}
	op.pits = append(op.pits, pitUUID)
}

func (op *volSnapCreateOp) nuvoDeletePiT(ctx context.Context, pitUUID string) {
	if op.planOnly {
		op.rhs.SetRequestMessage("Delete PiT %s", pitUUID)
		return
	}
	nuvoVolumeIdentifier := op.rhs.VolumeSeries.NuvoVolumeIdentifier
	op.c.Log.Debugf("VolumeSeriesRequest %s: VolumeSeries [%s] NUVOAPI DeletePit(%s, %s)", op.rhs.Request.Meta.ID, op.rhs.VolumeSeries.Meta.ID, nuvoVolumeIdentifier, pitUUID)
	err := op.c.App.NuvoAPI.DeletePit(nuvoVolumeIdentifier, pitUUID)
	if err != nil {
		if nuvoapi.ErrorIsTemporary(err) {
			op.rhs.SetRequestMessage("NUVOAPI temporary error: %s", err.Error())
			op.rhs.RetryLater = true
			return
		}
		op.rhs.SetRequestError("NUVOAPI DeletePit(%s, %s) failed: %s", nuvoVolumeIdentifier, pitUUID, err.Error())
		return
	}
	pits := make([]string, 0, len(op.pits))
	for _, p := range op.pits {
		if p != pitUUID {
			pits = append(pits, p)
		}
	}
	op.pits = pits
}

func (op *volSnapCreateOp) nuvoGetWriteStat(ctx context.Context) {
	nuvoVolumeIdentifier := op.rhs.VolumeSeries.NuvoVolumeIdentifier
	if op.planOnly {
		op.rhs.SetRequestMessage("Get Write Stat %s", nuvoVolumeIdentifier)
		return
	}
	op.c.Log.Debugf("VolumeSeriesRequest %s: VolumeSeries [%s] NUVOAPI GetVolumeStats(false, %s)", op.rhs.Request.Meta.ID, op.rhs.VolumeSeries.Meta.ID, nuvoVolumeIdentifier)
	comboStats, err := op.c.App.NuvoAPI.GetVolumeStats(false, nuvoVolumeIdentifier)
	if err != nil {
		op.c.Log.Warningf("VolumeSeriesRequest %s: NUVOAPI GetVolumeStats(%s): %s", op.rhs.Request.Meta.ID, nuvoVolumeIdentifier, err.Error())
		// ignore error
		return
	}
	writeStats := &comboStats.IOWrites
	op.c.Log.Debugf("VolumeSeriesRequest %s: VolumeSeries [%s] NUVOAPI GetVolumeStats(%s) for write: %v", op.rhs.Request.Meta.ID, op.rhs.VolumeSeries.Meta.ID, nuvoVolumeIdentifier, writeStats)
	lmd := &models.LifecycleManagementData{ // note: ensure that these survive the psUpload step
		EstimatedSizeBytes: int64(writeStats.SizeTotal),
		GenUUID:            writeStats.SeriesUUID,
		WriteIOCount:       int64(writeStats.Count),
	}
	op.rhs.Request.LifecycleManagementData = lmd
	op.c.Log.Debugf("VolumeSeriesRequest %s: LMD initialized {%d, %s, %d}", op.rhs.Request.Meta.ID, lmd.EstimatedSizeBytes, lmd.GenUUID, lmd.WriteIOCount)
}

func (op *volSnapCreateOp) nuvoListPiTs(ctx context.Context) {
	nuvoVolumeIdentifier := op.rhs.VolumeSeries.NuvoVolumeIdentifier
	op.c.Log.Debugf("VolumeSeriesRequest %s: VolumeSeries [%s] NUVOAPI ListPits(%s)", op.rhs.Request.Meta.ID, op.rhs.VolumeSeries.Meta.ID, nuvoVolumeIdentifier)
	pits, err := op.c.App.NuvoAPI.ListPits(nuvoVolumeIdentifier)
	if err != nil {
		if nuvoapi.ErrorIsTemporary(err) {
			op.rhs.SetRequestMessage("NUVOAPI temporary error: %s", err.Error())
			op.rhs.RetryLater = true
			return
		}
		op.rhs.SetRequestError("NUVOAPI ListPits(%s) failed: %s", nuvoVolumeIdentifier, err.Error())
		op.pits = nil
		return
	}
	op.pits = pits
}

// establish the arguments for the snapshot upload
func (op *volSnapCreateOp) planCopy(ctx context.Context) {
	op.tipPiT = op.rhs.Request.Snapshot.PitIdentifier
	op.tipSnapID = op.rhs.Request.SnapIdentifier
	op.tipExported = op.snapIsMounted(op.tipSnapID)
	if len(op.vsSnapshots) > 0 && util.Contains(op.pits, op.vsSnapshots[0].PitIdentifier) {
		if op.rhs.Request.ProtectionDomainID != op.vsSnapshots[0].ProtectionDomainID {
			op.rhs.SetRequestMessage("Forcing baseline snapshot because protection domain changed")
		} else {
			op.basePiT = op.vsSnapshots[0].PitIdentifier
			op.baseSnapIdentifier = op.vsSnapshots[0].SnapIdentifier
		}
	} else {
		op.rhs.SetRequestMessage("Baseline snapshot")
	}
	op.c.Log.Debugf("VolumeSeriesRequest %s: tip[s=%s,p=%s,e=%v] base[s=%s,p=%s]", op.rhs.Request.Meta.ID, op.tipSnapID, op.tipPiT, op.tipExported, op.baseSnapIdentifier, op.basePiT)
}

// psDelete deletes the snapshot from the domain protection store
func (op *volSnapCreateOp) psDelete(ctx context.Context) {
	if err := op.fetchDomain(ctx); err != nil {
		op.rhs.RetryLater = true
		return
	}
	sda := &pstore.SnapshotDeleteArgs{
		SnapIdentifier: op.rhs.Request.SnapIdentifier,
		ID:             string(op.rhs.Request.Meta.ID),
		PStore: &pstore.ProtectionStoreDescriptor{
			CspDomainType:       string(op.domObj.CspDomainType),
			CspDomainAttributes: op.domObj.CspDomainAttributes,
		},
		SourceSnapshot: op.rhs.Request.Snapshot.PitIdentifier,
	}
	op.c.Log.Debugf("VolumeSeriesRequest %s: PSO SnapshotDelete(sn:%s p:%s d:%s)", sda.ID, sda.SnapIdentifier, sda.SourceSnapshot, op.domObj.Meta.ID)
	tStart := time.Now()
	res, err := op.c.App.PSO.SnapshotDelete(ctx, sda)
	if err != nil {
		op.c.Log.Errorf("VolumeSeriesRequest %s: PSO SnapshotDelete(sn:%s p:%s d:%s): %s", sda.ID, sda.SnapIdentifier, sda.SourceSnapshot, op.domObj.Meta.ID, err.Error())
		op.rhs.RetryLater = true
		op.c.App.StateUpdater.Refresh() // check connection with nuvo
		return
	}
	uploadDuration := time.Now().Sub(tStart)
	op.c.Log.Debugf("VolumeSeriesRequest %s: PSO SnapshotDelete(sn:%s p:%s) ⇒ %#v, %s", sda.ID, sda.SnapIdentifier, sda.SourceSnapshot, res, uploadDuration)
}

// psUpload copies the snapshot to the domain protection store
func (op *volSnapCreateOp) psUpload(ctx context.Context) {
	if op.planOnly {
		op.rhs.SetRequestMessage("Copy snapshot to protection store")
		return
	}
	if err := op.c.rei.ErrOnBool("vsc-upload-error"); err != nil {
		op.rhs.SetRequestError("%s", err.Error())
		return
	}
	if err := op.fetchDomain(ctx); err != nil {
		op.rhs.RetryLater = true
		return
	}
	if err := op.fetchProtectionDomain(ctx); err != nil {
		op.rhs.RetryLater = true
		return
	}
	sba := &pstore.SnapshotBackupArgs{
		SnapIdentifier:     op.rhs.Request.SnapIdentifier,
		BaseSnapIdentifier: op.baseSnapIdentifier,
		ID:                 string(op.rhs.Request.Meta.ID),
		PStore: &pstore.ProtectionStoreDescriptor{
			CspDomainType:       string(op.domObj.CspDomainType),
			CspDomainAttributes: op.domObj.CspDomainAttributes,
		},
		SourceFile:            op.c.App.NuvoAPI.LunPath(string(op.c.App.NuvoVolDirPath), op.rhs.Request.MountedNodeDevice),
		VsID:                  string(op.rhs.VolumeSeries.Meta.ID),
		NuvoVolumeIdentifier:  op.rhs.VolumeSeries.NuvoVolumeIdentifier,
		NuvoAPISocketPathName: string(op.c.App.NuvoSocket),
		BasePiT:               op.basePiT,
		IncrPiT:               op.tipPiT,
		Reporter:              op.rhs,
	}
	sba.EncryptionAlgorithm = op.pdObj.EncryptionAlgorithm
	sba.Passphrase = op.pdObj.EncryptionPassphrase.Value
	sba.ProtectionDomainID = string(op.pdObj.Meta.ID)
	op.c.Log.Debugf("VolumeSeriesRequest %s: VolumeSeries [%s] PSO SnapshotBackup(sn:%s bsn:%s p:%s d:%s s:%s b:%s pd:%s)", sba.ID, sba.NuvoVolumeIdentifier, sba.SnapIdentifier, sba.BaseSnapIdentifier, sba.IncrPiT, op.domObj.Meta.ID, sba.SourceFile, sba.BasePiT, sba.ProtectionDomainID)
	tStart := time.Now()
	res, err := op.c.App.PSO.SnapshotBackup(ctx, sba)
	if err != nil {
		op.c.Log.Errorf("VolumeSeriesRequest %s: VolumeSeries [%s] PSO SnapshotBackup(sn:%s bsn:%s p:%s d:%s s:%s b:%s pd:%s): %s", sba.ID, sba.NuvoVolumeIdentifier, sba.SnapIdentifier, sba.BaseSnapIdentifier, sba.IncrPiT, op.domObj.Meta.ID, sba.SourceFile, sba.BasePiT, sba.ProtectionDomainID, err.Error())
		if e, ok := err.(pstore.Error); ok && e.IsRetryable() {
			op.rhs.RetryLater = true
		} else {
			op.rhs.SetRequestError("Upload error: %s", err.Error())
		}
		op.c.App.StateUpdater.Refresh() // check connection with nuvo
		return
	}
	uploadDuration := time.Now().Sub(tStart)
	op.c.Log.Debugf("VolumeSeriesRequest %s: VolumeSeries [%s] PSO SnapshotBackup(sn:%s p:%s) ⇒ %#v, %s", sba.ID, sba.NuvoVolumeIdentifier, sba.SnapIdentifier, sba.IncrPiT, res, uploadDuration)
	// post copy meta-data
	loc := &models.SnapshotLocation{
		CreationTime: strfmt.DateTime(tStart),
		CspDomainID:  models.ObjIDMutable(op.domObj.Meta.ID),
	}
	op.rhs.Request.Snapshot.Locations = []*models.SnapshotLocation{loc}
	lmd := &models.LifecycleManagementData{}
	if op.rhs.VolumeSeries.LifecycleManagementData != nil {
		*lmd = *op.rhs.VolumeSeries.LifecycleManagementData // copy
	}
	lmd.LastUploadTime = loc.CreationTime
	lmd.LastUploadSizeBytes = res.Stats.BytesTransferred
	if res.Stats.BytesTransferred > 0 {
		uploadDurationRoundedSec := (int64(uploadDuration) + int64(time.Second) - 1) / int64(time.Second)
		lmd.LastUploadTransferRateBPS = int32(int64(res.Stats.BytesTransferred) / uploadDurationRoundedSec)
	}
	if op.rhs.Request.LifecycleManagementData != nil { // propagate values set in this operation
		lmd.EstimatedSizeBytes = op.rhs.Request.LifecycleManagementData.EstimatedSizeBytes
		lmd.GenUUID = op.rhs.Request.LifecycleManagementData.GenUUID
		lmd.WriteIOCount = op.rhs.Request.LifecycleManagementData.WriteIOCount
	} else {
		lmd.EstimatedSizeBytes = 0 // override vs value
	}
	op.rhs.Request.LifecycleManagementData = lmd
}

// createSnapshotObject records snapshot object in database with data from the VSR snapshotData property
func (op *volSnapCreateOp) createSnapshotObject(ctx context.Context) {
	modSnap := &models.Snapshot{}
	modSnap.PitIdentifier = op.rhs.Request.Snapshot.PitIdentifier
	modSnap.ProtectionDomainID = op.rhs.Request.ProtectionDomainID
	modSnap.SnapIdentifier = op.rhs.Request.Snapshot.SnapIdentifier
	modSnap.SnapTime = op.rhs.Request.Snapshot.SnapTime
	modSnap.VolumeSeriesID = models.ObjIDMutable(op.rhs.VolumeSeries.Meta.ID)
	modSnap.DeleteAfterTime = op.rhs.Request.Snapshot.DeleteAfterTime
	modSnap.Locations = map[string]models.SnapshotLocation{}
	for _, loc := range op.rhs.Request.Snapshot.Locations {
		modSnap.Locations[string(loc.CspDomainID)] = models.SnapshotLocation{
			CreationTime: loc.CreationTime,
			CspDomainID:  loc.CspDomainID,
		}
	}
	stags := util.NewTagList(modSnap.SystemTags)
	stags.Set(com.SystemTagVsrCreator, string(op.rhs.Request.SyncCoordinatorID))
	modSnap.SystemTags = stags.List()
	// messages and tags are empty for now, might be necessary for replication
	_, err := op.rhs.A.OCrud.SnapshotCreate(ctx, modSnap)
	if err != nil {
		if oErr, ok := err.(*crud.Error); ok && oErr.Exists() { // ignore error
			op.c.Log.Debugf("VolumeSeriesRequest %s: snapshot object (PitUUID %s, SnapID %s and VS %s) already exists",
				op.rhs.Request.Meta.ID, op.rhs.Request.Snapshot.PitIdentifier, op.rhs.Request.Snapshot.SnapIdentifier, op.rhs.VolumeSeries.Meta.ID)
			return
		}
		op.rhs.SetRequestMessage("Error creating snapshot object: %s", err.Error()) // unlikely to be saved
		op.rhs.InError = true
		op.rhs.RetryLater = true
		return
	}
	op.rhs.SetRequestMessage("Snapshot object created")
}

// removeOlderPiTs attempts to remove older pits in an oldest first manner, stopping at the first error.
func (op *volSnapCreateOp) removeOlderPiTs(ctx context.Context) {
	for i := len(op.vsSnapshots) - 1; !op.rhs.InError && i >= 0; i-- {
		if util.Contains(op.pits, op.vsSnapshots[i].PitIdentifier) {
			op.ops.nuvoDeletePiT(ctx, op.vsSnapshots[i].PitIdentifier)
		}
	}
}

func (op *volSnapCreateOp) lmdGuardLock(ctx context.Context) {
	if op.planOnly {
		return
	}
	op.c.Log.Debugf("VolumeSeriesRequest %s: acquiring LMDGuard lock", op.rhs.Request.Meta.ID)
	cst, err := op.c.App.LMDGuard.Enter(string(op.rhs.Request.Meta.ID))
	if err != nil {
		op.rhs.SetRequestMessage("LMDGuard lock error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.c.Log.Debugf("VolumeSeriesRequest %s: obtained LMDGuard lock", op.rhs.Request.Meta.ID)
	op.lmdCST = cst
}

func (op *volSnapCreateOp) lmdGuardUnlock(ctx context.Context) {
	if op.lmdCST == nil {
		return
	}
	op.c.Log.Debugf("VolumeSeriesRequest %s: releasing LMDGuard lock", op.rhs.Request.Meta.ID)
	op.lmdCST.Leave()
	op.lmdCST = nil
}

// advertise failure to peers
func (op *volSnapCreateOp) syncFail(ctx context.Context) {
	if op.advertisedFailure {
		return
	}
	finalState := com.VolReqStateFailed
	if op.rhs.Canceling {
		finalState = com.VolReqStateCanceled
	}
	op.saa = &vra.SyncAbortArgs{
		LocalKey:   string(op.rhs.Request.VolumeSeriesID),
		LocalState: finalState,
	}
	op.rhs.SyncAbort(ctx, op.saa) // ignore errors
	op.advertisedFailure = true
}

// synchronize the current state, do not wake up the coordinator except in the finalizing state
// Snapshots can take a long time and are not interruptable and so the CompleteBy time may have expired during processing.
// Once the snapshot operation has succeeded set adjustCB to true to avoid time out error by recomputing a reasonable sync duration.
func (op *volSnapCreateOp) syncState(ctx context.Context, adjustCB bool) {
	state := string(op.rhs.Request.VolumeSeriesRequestState)
	if op.planOnly {
		op.rhs.SetRequestMessage("%s sync", state)
		return
	}
	op.sa = &vra.SyncArgs{
		LocalKey:   string(op.rhs.Request.VolumeSeriesID),
		SyncState:  state,
		CompleteBy: time.Time(op.rhs.Request.CompleteByTime),
	}
	if state == com.VolReqStateFinalizingSnapshot {
		op.sa.CoordinatorStateOnSync = com.VolReqStateCGSnapshotFinalize
	}
	if adjustCB {
		origDur := time.Time(op.rhs.Request.CompleteByTime).Sub(time.Time(op.rhs.Request.Meta.TimeCreated))
		op.sa.CompleteBy = time.Now().Add(origDur)
	}
	if err := op.rhs.SyncRequests(ctx, op.sa); err != nil {
		op.rhs.SetRequestError("Sync error %s", err.Error())
	}
}

func (op *volSnapCreateOp) timestampPiT(ctx context.Context) {
	if op.planOnly {
		op.rhs.SetRequestMessage("Record PiT timestamp")
		return
	}
	s := &models.SnapshotData{}
	s.PitIdentifier = op.rhs.Request.Snapshot.PitIdentifier
	s.SnapIdentifier = op.rhs.Request.SnapIdentifier
	s.ConsistencyGroupID = op.rhs.VolumeSeries.ConsistencyGroupID
	s.SnapTime = strfmt.DateTime(time.Now())
	s.SizeBytes = op.rhs.VolumeSeries.SizeBytes
	op.rhs.Request.Snapshot = s
	// remember the HEAD last unexport time if set
	vsTags := util.NewTagList(op.rhs.VolumeSeries.SystemTags)
	if val, ok := vsTags.Get(com.SystemTagVolumeLastHeadUnexport); ok {
		vsrTags := util.NewTagList(op.rhs.Request.SystemTags)
		vsrTags.Set(com.SystemTagVolumeLastHeadUnexport, val)
		op.rhs.Request.SystemTags = vsrTags.List()
	}
	if err := op.rhs.UpdateRequest(ctx); err != nil {
		op.rhs.RetryLater = true
	}
	op.rhs.SetRequestMessage("Recorded PiT timestamp")
}

func (op *volSnapCreateOp) unexportTip(ctx context.Context) {
	pitUUID := op.rhs.Request.Snapshot.PitIdentifier
	if op.planOnly {
		op.rhs.SetRequestMessage("Unexport LUN for Snapshot %s PiT %s", op.tipSnapID, pitUUID)
		return
	}
	op.unexportLun(ctx, op.tipSnapID, pitUUID)
}

// helpers

// returns true if the op.pits list contains the PiT identified in the VSR
func (op *volSnapCreateOp) pitCreated() bool {
	return op.pitExists(op.rhs.Request.Snapshot.PitIdentifier)
}

// returns true if the op.pits list contains a specific pit
func (op *volSnapCreateOp) pitExists(pitUUID string) bool {
	return util.Contains(op.pits, pitUUID)
}

func (op *volSnapCreateOp) snapIsMounted(snapID string) bool {
	for _, m := range op.rhs.VolumeSeries.Mounts {
		if m.SnapIdentifier == snapID && m.MountState == com.VolMountStateMounted {
			return true
		}
	}
	return false
}

// exportLun invokes the Export operation to export a LUN corresponding to a snapID.
// It uses the same rhs structure.
func (op *volSnapCreateOp) exportLun(ctx context.Context, snapID, pitUUID string) {
	savedState := op.rhs.Request.VolumeSeriesRequestState
	op.rhs.Request.VolumeSeriesRequestState = com.VolReqStateVolumeExport
	exportInternalArgs := &exportInternalArgs{
		snapID:  snapID,
		pitUUID: pitUUID,
	}
	op.rhs.StashSet(exportInternalCallStashKey{}, exportInternalArgs)
	op.mops.Export(ctx, op.rhs)
	// restore
	op.rhs.Request.VolumeSeriesRequestState = savedState
}

// unexportLun invokes the Export operation to export a LUN corresponding to a snapID.
// It uses the same rhs structure.
func (op *volSnapCreateOp) unexportLun(ctx context.Context, snapID, pitUUID string) {
	savedState := op.rhs.Request.VolumeSeriesRequestState
	op.rhs.Request.VolumeSeriesRequestState = com.VolReqStateUndoVolumeExport
	exportInternalArgs := &exportInternalArgs{
		snapID:  snapID,
		pitUUID: pitUUID,
	}
	op.rhs.StashSet(exportInternalCallStashKey{}, exportInternalArgs)
	op.mops.UndoExport(ctx, op.rhs)
	// restore
	op.rhs.Request.VolumeSeriesRequestState = savedState
}

// fetchCluster obtains the cluster object
func (op *volSnapCreateOp) fetchCluster(ctx context.Context) error {
	if op.clObj != nil {
		return nil
	}
	clObj, err := op.c.oCrud.ClusterFetch(ctx, string(op.rhs.VolumeSeries.BoundClusterID))
	if err == nil {
		op.clObj = clObj
	}
	return err
}

// fetchDomain obtains the domain object via the cluster object
func (op *volSnapCreateOp) fetchDomain(ctx context.Context) error {
	if op.domObj != nil {
		return nil
	}
	var err error
	if op.clObj == nil {
		err = op.fetchCluster(ctx)
	}
	if err == nil {
		op.domObj, err = op.c.oCrud.CSPDomainFetch(ctx, string(op.clObj.CspDomainID))
	}
	return err
}

// fetchProtectionDomain obtains the protection domain object
func (op *volSnapCreateOp) fetchProtectionDomain(ctx context.Context) error {
	if op.pdObj != nil {
		return nil
	}
	var err error
	op.pdObj, err = op.c.oCrud.ProtectionDomainFetch(ctx, string(op.rhs.Request.ProtectionDomainID))
	return err
}

// computeVsSchedulerDataAfterSnapshot constructs the LifecycleManagementData for the VS based on the data accumulated in the VSR.
// The LMD passed as input must be initialized with the following from the previous VS state:
//  - EstimatedSizeBytes
//  - SizeEstimateRatio
//  - FinalSnapshotNeeded
// The LMD passed as input must be initialized with the following from the VSR execution:
//  - LastSnapTime
//  - LastUploadTime
//  - LastUploadSizeBytes
//  - LastUploadTransferRateBPS
// The only LMD fields modifed in the return value are:
//  - EstimatedSizeBytes
//  - NextSnapshotTime
//  - SizeEstimateRatio
// The interimBytes value is the estimated number of bytes of the next snapshot since I/O was resumed.
// See https://goo.gl/Bt4fG6 for details.
func (op *volSnapCreateOp) computeVsSchedulerDataAfterSnapshot(b *bytes.Buffer, lmd *models.LifecycleManagementData, rpoDur time.Duration, interimBytes int64) (*models.LifecycleManagementData, int64) {
	rpoDurSec := int64(rpoDur / time.Second)
	safetyBufferSec := int64(float64(rpoDurSec) * agentd.SnapshotSafetyBufferMultiplier)
	transferRate := lmd.LastUploadTransferRateBPS
	if transferRate <= 0 {
		transferRate = op.c.App.CSP.ProtectionStoreUploadTransferRate()
	}
	fmt.Fprintf(b, "RPO=%ds SafetyBuffer=%s BPS=%d ", rpoDurSec, util.SizeBytes(safetyBufferSec), transferRate)
	nextOffsetSec := rpoDurSec - (lmd.LastUploadSizeBytes / int64(transferRate)) - safetyBufferSec
	fmt.Fprintf(b, "NextOffsetSec=%ds-(%s/%dbps)-%ds=%ds ", rpoDurSec, util.SizeBytes(lmd.LastUploadSizeBytes), transferRate, safetyBufferSec, nextOffsetSec)
	if nextOffsetSec < safetyBufferSec {
		nextOffsetSec = safetyBufferSec
	}
	snapTime := time.Time(lmd.LastSnapTime)
	nextOffset := time.Duration(nextOffsetSec) * time.Second
	nextSnapshotTime := snapTime.Add(nextOffset)
	fmt.Fprintf(b, "NextSnapshotTime={%s}+%s={%s} ", snapTime.Format(time.RFC3339), nextOffset, nextSnapshotTime.Format(time.RFC3339))
	oldRatio := lmd.SizeEstimateRatio
	newRatio := oldRatio
	if lmd.EstimatedSizeBytes > 0 {
		newRatio = float64(lmd.LastUploadSizeBytes) / float64(lmd.EstimatedSizeBytes)
		fmt.Fprintf(b, "OldSizeRatio=%.2f NewSizeRatio=%s/%s=%.2f ", oldRatio, util.SizeBytes(lmd.LastUploadSizeBytes), util.SizeBytes(lmd.EstimatedSizeBytes), newRatio)
	}
	retLmd := &models.LifecycleManagementData{}
	*retLmd = *lmd // shallow copy
	retLmd.EstimatedSizeBytes = interimBytes
	retLmd.NextSnapshotTime = strfmt.DateTime(nextSnapshotTime)
	retLmd.SizeEstimateRatio = newRatio
	return retLmd, nextOffsetSec
}

func (op *volSnapCreateOp) doFreezeUnfreeze(ctx context.Context, isFreeze bool) {
	vs := op.rhs.VolumeSeries
	sysTags := util.NewTagList(vs.SystemTags)
	target, fsAttached := sysTags.Get(com.SystemTagVolumeFsAttached)
	if !fsAttached {
		cmd := "Unfreeze"
		if isFreeze {
			cmd = "Freeze"
		}
		op.rhs.SetRequestMessage("%s: No filesystem attached", cmd)
		return
	}
	req := op.rhs.Request
	ffa := &mount.FilesystemFreezeArgs{
		LogPrefix: fmt.Sprintf("VolumeSeriesRequest %s: Vol [%s]", req.Meta.ID, req.VolumeSeriesID),
		Target:    target,
		Deadline:  time.Time(req.CompleteByTime),
		PlanOnly:  op.planOnly,
		Commands:  &bytes.Buffer{},
	}
	// ignore errors because frozen state cannot be determined
	if isFreeze {
		op.c.mounter.FreezeFilesystem(ctx, ffa)
	} else {
		op.c.mounter.UnfreezeFilesystem(ctx, ffa)
	}
	op.recordCommands(ffa.Commands)
}

func (op *volSnapCreateOp) recordCommands(buf *bytes.Buffer) {
	for _, cmd := range strings.Split(buf.String(), "\n") {
		if len(cmd) == 0 {
			continue
		}
		op.rhs.SetRequestMessage("%s", cmd)
	}
}
