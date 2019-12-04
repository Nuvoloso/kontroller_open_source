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
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/pstore"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

// vra.VolSnapshotRestoreHandlers methods

// VolSnapshotRestore performs the VOL_SNAPSHOT_RESTORE operation
func (c *Component) VolSnapshotRestore(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := c.volSnapRestoreRecoverOrMakeOp(rhs)
	op.run(ctx)
}

// UndoVolSnapshotRestore undoess the VOL_SNAPSHOT_RESTORE operation
func (c *Component) UndoVolSnapshotRestore(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := c.volSnapRestoreRecoverOrMakeOp(rhs)
	op.run(ctx)
}

type volSnapRestoreStashKey struct{} // stash key type

// volSnapRestoreRecoverOrMakeOp retrieves the Op structure from the rhs stash, or creates a new one
func (c *Component) volSnapRestoreRecoverOrMakeOp(rhs *vra.RequestHandlerState) *volSnapRestoreOp {
	var op *volSnapRestoreOp
	if v := rhs.StashGet(volSnapRestoreStashKey{}); v != nil {
		if stashedOp, ok := v.(*volSnapRestoreOp); ok {
			op = stashedOp
		}
	}
	if op == nil {
		op = &volSnapRestoreOp{}
	}
	op.c = c
	op.rhs = rhs
	op.ops = op // self-reference
	op.mops = c // component reference
	return op
}

type volSnapRestoreSubState int

// volSnapRestoreSubState values (the ordering is meaningful)
const (
	// SNAPSHOT_RESTORE
	VolSRDisableMetrics volSnapRestoreSubState = iota
	VolSRDownloadSnapshot
	VolSRDownloadSnapshotBreakout

	// SNAPSHOT_RESTORE_FINALIZE
	VolSRRecordOriginInVolume
	VolSRRecordFinalizeSync
	VolSRUnexportHead
	VolSRDone
	VolSREnableMetrics
	VolSRDone2

	// UNDO_SNAPSHOT_RESTORE
	VolSRUndoDisableMetrics
	VolSRUndoDone

	// LAST: No operation is performed in this state.
	VolSRNoOp
)

func (ss volSnapRestoreSubState) String() string {
	switch ss {
	case VolSRDisableMetrics:
		return "VolSRDisableMetrics"
	case VolSRDownloadSnapshot:
		return "VolSRDownloadSnapshot"
	case VolSRDownloadSnapshotBreakout:
		return "VolSRDownloadSnapshotBreakout"
	case VolSRRecordOriginInVolume:
		return "VolSRRecordOriginInVolume"
	case VolSRRecordFinalizeSync:
		return "VolSRRecordFinalizeSync"
	case VolSRUnexportHead:
		return "VolSRUnexportHead"
	case VolSRDone:
		return "VolSRDone"
	case VolSREnableMetrics:
		return "VolSREnableMetrics"
	case VolSRDone2:
		return "VolSRDone2"
	case VolSRUndoDisableMetrics:
		return "VolSRUndoDisableMetrics"
	case VolSRUndoDone:
		return "VolSRUndoDone"
	}
	return fmt.Sprintf("volSnapRestoreSubState(%d)", ss)
}

type volSnapRestoreOp struct {
	c                 *Component
	rhs               *vra.RequestHandlerState
	ops               volSnapRestoreOperators
	mops              vra.MountHandlers
	inError           bool
	planOnly          bool
	advertisedFailure bool
	snapIdentifier    string
	sa                *vra.SyncArgs
	saa               *vra.SyncAbortArgs
	pdObj             *models.ProtectionDomain
}

type volSnapRestoreOperators interface {
	getInitialState(ctx context.Context) volSnapRestoreSubState
	psDownload(ctx context.Context)
	recordOriginInVolume(ctx context.Context)
	syncFail(ctx context.Context)
	syncState(ctx context.Context)
	unexportHeadNoSnapshot(ctx context.Context)
	disableMetrics(ctx context.Context, disable bool)
}

func (op *volSnapRestoreOp) run(ctx context.Context) {
	jumpToState := VolSRNoOp
out:
	for ss := op.ops.getInitialState(ctx); !(op.rhs.InError || op.rhs.RetryLater); ss++ {
		if jumpToState != VolSRNoOp {
			ss = jumpToState
			jumpToState = VolSRNoOp
		}
		op.c.Log.Debugf("VolumeSeriesRequest %s: %s %s", op.rhs.Request.Meta.ID, op.rhs.Request.VolumeSeriesRequestState, ss)
		switch ss {
		// SNAPSHOT_RESTORE
		case VolSRDisableMetrics:
			op.ops.disableMetrics(ctx, true)
		case VolSRDownloadSnapshot:
			op.ops.psDownload(ctx)
		// SNAPSHOT_RESTORE_FINALIZE
		case VolSRRecordOriginInVolume:
			op.ops.recordOriginInVolume(ctx)
		case VolSRRecordFinalizeSync:
			op.ops.syncState(ctx)
			if op.rhs.HasAttachFs { // need HEAD in next operation
				jumpToState = VolSREnableMetrics
			}
		case VolSRUnexportHead:
			op.ops.unexportHeadNoSnapshot(ctx)
		case VolSREnableMetrics:
			op.ops.disableMetrics(ctx, false)
		case VolSRUndoDisableMetrics:
			op.ops.disableMetrics(ctx, false)
		default:
			break out
		}
	}
	if op.rhs.InError && op.rhs.Request.SyncCoordinatorID != "" { // error raised in forward-path triggers syncFail
		op.ops.syncFail(ctx)
	}
	if op.inError {
		op.rhs.InError = true
	}
	if !(op.rhs.RetryLater || op.rhs.InError) {
		op.rhs.StashSet(volSnapRestoreStashKey{}, op)
	}
}

func (op *volSnapRestoreOp) getInitialState(ctx context.Context) volSnapRestoreSubState {
	op.planOnly = swag.BoolValue(op.rhs.Request.PlanOnly)
	op.snapIdentifier = op.rhs.Request.Snapshot.SnapIdentifier
	if op.rhs.Request.VolumeSeriesRequestState == com.VolReqStateUndoSnapshotRestore {
		op.inError = op.rhs.InError
		op.rhs.InError = false
		return VolSRUndoDisableMetrics
	}
	switch op.rhs.Request.VolumeSeriesRequestState {
	case com.VolReqStateSnapshotRestore:
		vs := op.rhs.VolumeSeries
		if vs.VolumeSeriesState != com.VolStateInUse {
			op.rhs.SetRequestError("Invalid volume state: %s", vs.VolumeSeriesState)
			return VolSRDone
		}
		headIsMounted := false
		for _, m := range vs.Mounts {
			if m.SnapIdentifier == com.VolMountHeadIdentifier {
				headIsMounted = true
				op.rhs.Request.MountedNodeDevice = m.MountedNodeDevice
				break
			}
		}
		if !headIsMounted {
			op.rhs.SetRequestError("HEAD not mounted")
			return VolSRDone
		}
		return VolSRDisableMetrics
	case com.VolReqStateSnapshotRestoreFinalize:
		return VolSRRecordOriginInVolume
	}
	return VolSRDone
}

// psDownload downloads the snapshot from a domain protection store in the location array
func (op *volSnapRestoreOp) psDownload(ctx context.Context) {
	if op.planOnly {
		op.rhs.SetRequestMessage("Copy snapshot [%s] from a protection store", op.snapIdentifier)
		return
	}
	var err error
	op.pdObj, err = op.c.oCrud.ProtectionDomainFetch(ctx, string(op.rhs.Request.ProtectionDomainID))
	if err != nil {
		op.rhs.RetryLater = true
		return
	}
	var seenRetryableError bool
	for _, loc := range op.rhs.Request.Snapshot.Locations {
		if _, err = op.psDownloadFromPStore(ctx, loc.CspDomainID); err == nil {
			break
		}
		if e, ok := err.(pstore.Error); ok && e.IsRetryable() {
			seenRetryableError = true
		}
	}
	if err != nil && !op.rhs.RetryLater {
		if seenRetryableError {
			op.rhs.RetryLater = true
		} else {
			op.rhs.SetRequestError("Failed to download snapshot: %s", err.Error())
		}
	}
}

// psDownloadFromPStore downloads the snapshot from a given domain protection store
func (op *volSnapRestoreOp) psDownloadFromPStore(ctx context.Context, domID models.ObjIDMutable) (*pstore.SnapshotRestoreResult, error) {
	op.c.Log.Debugf("VolumeSeriesRequest %s: using pStore %s", op.rhs.Request.Meta.ID, domID)
	if err := op.c.rei.ErrOnBool("vsc-download-error"); err != nil {
		return nil, err
	}
	domObj, err := op.c.oCrud.CSPDomainFetch(ctx, string(domID))
	if err != nil {
		op.rhs.RetryLater = true
		return nil, err
	}
	sra := &pstore.SnapshotRestoreArgs{
		SnapIdentifier: op.rhs.Request.Snapshot.SnapIdentifier,
		ID:             string(op.rhs.Request.Meta.ID),
		PStore: &pstore.ProtectionStoreDescriptor{
			CspDomainType:       string(domObj.CspDomainType),
			CspDomainAttributes: domObj.CspDomainAttributes,
		},
		SourceSnapshot: op.rhs.Request.Snapshot.PitIdentifier,
		DestFile:       op.c.App.NuvoAPI.LunPath(string(op.c.App.NuvoVolDirPath), op.rhs.Request.MountedNodeDevice),
		Reporter:       op.rhs,
	}
	sra.EncryptionAlgorithm = op.pdObj.EncryptionAlgorithm
	sra.Passphrase = op.pdObj.EncryptionPassphrase.Value
	sra.ProtectionDomainID = string(op.pdObj.Meta.ID)
	op.c.Log.Debugf("VolumeSeriesRequest %s: PSO SnapshotRestore(sn:%s p:%s f:%s d:%s pd:%s)", sra.ID, sra.SnapIdentifier, sra.SourceSnapshot, sra.DestFile, domObj.Meta.ID, sra.ProtectionDomainID)
	tStart := time.Now()
	res, err := op.c.App.PSO.SnapshotRestore(ctx, sra)
	if err != nil {
		op.c.Log.Errorf("VolumeSeriesRequest %s: PSO SnapshotRestore(sn:%s p:%s f:%s d:%s pd:%s): %s", sra.ID, sra.SnapIdentifier, sra.SourceSnapshot, sra.DestFile, domObj.Meta.ID, sra.ProtectionDomainID, err.Error())
		op.c.App.StateUpdater.Refresh() // check connection with nuvo
		return nil, err
	}
	downloadDuration := time.Now().Sub(tStart)
	op.c.Log.Debugf("VolumeSeriesRequest %s: PSO SnapshotRestore(sn:%s p:%s) ⇒ %#v, %s", sra.ID, sra.SnapIdentifier, sra.SourceSnapshot, res, downloadDuration)
	op.rhs.SetRequestMessage("Downloaded snapshot [%s] from location [%s]", op.snapIdentifier, domID)
	return res, nil
}

// recordOriginInVolume inserts an origin record in the volume
func (op *volSnapRestoreOp) recordOriginInVolume(ctx context.Context) {
	snapVol := op.rhs.Request.Snapshot.VolumeSeriesID
	vsID := string(op.rhs.VolumeSeries.Meta.ID)
	if op.planOnly {
		op.rhs.SetRequestMessage("Insert origin message in volume")
		return
	}
	modVS := func(vs *models.VolumeSeries) (*models.VolumeSeries, error) {
		if vs == nil {
			vs = op.rhs.VolumeSeries
		}
		msg := fmt.Sprintf("Initialized from Volume[%s] Snapshot[%s]", snapVol, op.snapIdentifier)
		vs.Messages = append(vs.Messages, &models.TimestampedString{
			Message: msg,
			Time:    strfmt.DateTime(time.Now()),
		})
		tl := util.NewTagList(vs.SystemTags)
		tl.Set(com.SystemTagVolumeRestoredSnap, op.snapIdentifier)       // TBD: future use snapshotId
		tl.Delete(com.SystemTagVsrRestoring)                             // remove if set
		tl.Set(com.SystemTagVsrRestored, string(op.rhs.Request.Meta.ID)) // for downstream UNCONFIGURE after UNEXPORT
		vs.SystemTags = tl.List()
		return vs, nil
	}
	items := &crud.Updates{Set: []string{"messages", "systemTags"}}
	vs, err := op.rhs.A.OCrud.VolumeSeriesUpdater(ctx, vsID, modVS, items)
	if err != nil {
		op.rhs.SetRequestMessage("Error updating volume: %s", err.Error()) // unlikely to be saved
		op.rhs.RetryLater = true
		return
	}
	op.rhs.VolumeSeries = vs
}

// advertise failure to peers
func (op *volSnapRestoreOp) syncFail(ctx context.Context) {
	if op.advertisedFailure {
		return
	}
	finalState := com.VolReqStateFailed
	if op.rhs.Canceling {
		finalState = com.VolReqStateCanceled
	}
	op.saa = &vra.SyncAbortArgs{
		LocalKey:   op.snapIdentifier,
		LocalState: finalState,
	}
	op.rhs.SyncAbort(ctx, op.saa) // ignore errors
	op.advertisedFailure = true
}

// synchronize the current state, do not wake up the coordinator except in the finalizing state
func (op *volSnapRestoreOp) syncState(ctx context.Context) {
	if op.planOnly {
		op.rhs.SetRequestMessage("sync")
		return
	}
	// Restore takes a long time and is not interruptable so the CompleteBy time may have expired.
	// Since the restore has completed avoid time out related failures by recomputing a reasonable sync duration.
	origDur := time.Time(op.rhs.Request.CompleteByTime).Sub(time.Time(op.rhs.Request.Meta.TimeCreated))
	op.sa = &vra.SyncArgs{
		LocalKey:               op.snapIdentifier,
		SyncState:              com.VolReqStateSucceeded,
		CoordinatorStateOnSync: com.VolReqStateSnapshotRestoreDone,
		CompleteBy:             time.Now().Add(origDur),
	}
	if err := op.rhs.SyncRequests(ctx, op.sa); err != nil {
		op.rhs.SetRequestError("Sync error %s", err.Error())
	}
}

// unexportHeadNoSnapshot invokes the Export operation to unexport the HEAD LUN on a best effort basis without triggerring a snapshot.
// It uses the same rhs structure.
func (op *volSnapRestoreOp) unexportHeadNoSnapshot(ctx context.Context) {
	if op.planOnly {
		op.rhs.SetRequestMessage("Unexport HEAD no snapshot")
		return
	}
	savedState := op.rhs.Request.VolumeSeriesRequestState
	op.rhs.Request.VolumeSeriesRequestState = com.VolReqStateUndoVolumeExport
	exportInternalArgs := &exportInternalArgs{
		snapID:    com.VolMountHeadIdentifier,
		ignoreFSN: true,
	}
	op.rhs.StashSet(exportInternalCallStashKey{}, exportInternalArgs)
	op.mops.UndoExport(ctx, op.rhs)
	// restore
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.rhs.Request.VolumeSeriesRequestState = savedState
}

// disableMetrics  disables metric collection during restore. setting "disable" to false enables metric collection
func (op *volSnapRestoreOp) disableMetrics(ctx context.Context, disable bool) {
	if op.planOnly {
		op.rhs.SetRequestMessage("Disable Metric collection")
		return
	}
	lun := op.c.App.AppServant.FindLUN(string(op.rhs.VolumeSeries.Meta.ID), com.VolMountHeadIdentifier)
	if lun != nil {
		if op.rhs.HasMount && !op.rhs.HasAttachFs {
			// always disable in standalone mount+vol_snapshot_restore because the volume will get unmounted
			disable = true
		}
		lun.DisableMetrics = disable
		op.rhs.SetRequestMessage("DisableMetrics: %v", disable)
	} else {
		op.c.Log.Debugf("Failed to set disable metric collection to %t", disable)
	}
}
