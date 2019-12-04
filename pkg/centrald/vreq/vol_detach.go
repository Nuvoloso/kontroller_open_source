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
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/swag"
)

type volDetachStashKey struct{} // stash key type

// VolDetach implements the vra.NodeDeleter interface
func (c *Component) VolDetach(ctx context.Context, rhs *vra.RequestHandlerState) {
	var op *volDetachOp
	if v := rhs.StashGet(volDetachStashKey{}); v != nil {
		if stashedOp, ok := v.(*volDetachOp); ok {
			op = stashedOp
		}
	}
	if op == nil {
		op = &volDetachOp{}
	}
	op.c = c
	op.rhs = rhs
	op.ops = op // self-reference
	op.run(ctx)
}

type volDetachSubState int

// volDetachSubState values
const (
	// VOLUME_DETACH_WAIT
	VdStartSync volDetachSubState = iota
	VdDetachWaitBreakout

	// VOLUME_DETACHING
	VdDetachVolume
	VdDetachingBreakout

	// VOLUME_DETACHED
	VdEndSync
	VdDetachedBreakout

	VdError
	VdNoOp
)

func (ss volDetachSubState) String() string {
	switch ss {
	case VdStartSync:
		return "VdStartSync"
	case VdDetachWaitBreakout:
		return "VdDetachWaitBreakout"
	case VdDetachVolume:
		return "VdDetachVolume"
	case VdDetachingBreakout:
		return "VdDetachingBreakout"
	case VdEndSync:
		return "VdEndSync"
	case VdDetachedBreakout:
		return "VdDetachedBreakout"
	case VdError:
		return "VdError"
	}
	return fmt.Sprintf("vdSubState(%d)", ss)
}

type volDetachOp struct {
	c                 *Component
	rhs               *vra.RequestHandlerState
	ops               volDetachOperators
	planOnly          bool
	advertisedFailure bool
	sa                *vra.SyncArgs
	saa               *vra.SyncAbortArgs
}

type volDetachOperators interface {
	getInitialState(ctx context.Context) volDetachSubState
	detachVolume(ctx context.Context)
	syncFail(ctx context.Context)
	syncState(ctx context.Context)
}

func (op *volDetachOp) run(ctx context.Context) {
out:
	for ss := op.ops.getInitialState(ctx); !(op.rhs.InError || op.rhs.RetryLater); ss++ {
		op.c.Log.Debugf("VolumeSeriesRequest %s: VOL_DETACH %s %s", op.rhs.Request.Meta.ID, op.rhs.Request.VolumeSeriesRequestState, ss)
		switch ss {
		// VOLUME_DETACH_WAIT
		case VdStartSync:
			op.ops.syncState(ctx)

		// VOLUME_DETACHING
		case VdDetachVolume:
			op.ops.detachVolume(ctx)

		// VOLUME_DETACHED
		case VdEndSync:
			op.ops.syncState(ctx)

		default:
			break out
		}
	}
	if op.rhs.InError {
		op.ops.syncFail(ctx)
	}
	if !(op.rhs.RetryLater || op.rhs.InError) {
		op.rhs.StashSet(volDetachStashKey{}, op)
	}
}

func (op *volDetachOp) getInitialState(ctx context.Context) volDetachSubState {
	op.planOnly = swag.BoolValue(op.rhs.Request.PlanOnly)
	switch op.rhs.Request.VolumeSeriesRequestState {
	case common.VolReqStateVolumeDetachWait:
		return VdStartSync
	case common.VolReqStateVolumeDetaching:
		return VdDetachVolume
	case common.VolReqStateVolumeDetached:
		return VdEndSync
	}
	op.rhs.InError = true
	return VdError
}

func (op *volDetachOp) detachVolume(ctx context.Context) {
	if op.planOnly {
		op.rhs.SetRequestMessage("Detaching volume from node %s", op.rhs.Request.NodeID)
		return
	}
	errBreakout := fmt.Errorf("volume-already-detached")
	var buf bytes.Buffer
	items := &crud.Updates{Set: []string{"cacheAllocations", "configuredNodeId", "messages", "mounts", "systemTags", "volumeSeriesState"}}
	modifyFn := func(vs *models.VolumeSeries) (*models.VolumeSeries, error) {
		buf.Reset()
		if vs == nil {
			vs = op.rhs.VolumeSeries
		}
		ml := util.NewMsgList(vs.Messages)
		sTags := util.NewTagList(vs.SystemTags)
		if vs.LifecycleManagementData == nil {
			vs.LifecycleManagementData = &models.LifecycleManagementData{}
		}
		if vs.CacheAllocations == nil {
			vs.CacheAllocations = map[string]models.CacheAllocation{}
		}
		fmt.Fprintf(&buf, "Detaching volume [%s] from node [%s]: FSN:%v", op.rhs.VolumeSeries.Meta.ID, op.rhs.Request.NodeID, vs.LifecycleManagementData.FinalSnapshotNeeded)
		changes := 0
		numMounts := len(vs.Mounts)
		newMounts := make([]*models.Mount, 0, numMounts)
		for _, m := range vs.Mounts {
			// Note: highly likely that all mounts are on the same node
			if m.MountedNodeID != op.rhs.Request.NodeID {
				newMounts = append(newMounts, m)
				fmt.Fprintf(&buf, " Im[%s, %s]", m.SnapIdentifier, m.MountedNodeID)
			} else {
				changes++
				fmt.Fprintf(&buf, " Dm[%s]", m.SnapIdentifier)
				if m.SnapIdentifier == common.VolMountHeadIdentifier {
					sTags.Set(common.SystemTagVolumeLastHeadUnexport, time.Now().String())
					sTags.Delete(common.SystemTagVolumeHeadStatSeries)
					sTags.Delete(common.SystemTagVolumeHeadStatCount)
					sTags.Delete(common.SystemTagVolumeFsAttached)
				}
			}
		}
		vs.Mounts = newMounts
		fmt.Fprintf(&buf, " #m:%d", len(newMounts))
		if _, ok := vs.CacheAllocations[string(op.rhs.Request.NodeID)]; ok {
			changes++
			fmt.Fprintf(&buf, " Dca")
			delete(vs.CacheAllocations, string(op.rhs.Request.NodeID))
		}
		fmt.Fprintf(&buf, " %s", vs.VolumeSeriesState)
		if vra.VolumeSeriesIsConfigured(vs.VolumeSeriesState) {
			changes++
			fmt.Fprintf(&buf, "⇒%s", common.VolStateProvisioned)
			ml.Insert("State change %s ⇒ %s", vs.VolumeSeriesState, common.VolStateProvisioned)
			vs.VolumeSeriesState = common.VolStateProvisioned
			vs.ConfiguredNodeID = ""
		}
		fmt.Fprintf(&buf, " #Δ:%d", changes)
		if changes == 0 {
			return nil, errBreakout
		}
		vs.Messages = ml.ToModel()
		vs.SystemTags = sTags.List()
		return vs, nil
	}
	obj, err := op.c.oCrud.VolumeSeriesUpdater(ctx, string(op.rhs.VolumeSeries.Meta.ID), modifyFn, items)
	if buf.Len() > 0 {
		op.c.Log.Debugf("%s\n", buf.String())
	}
	if err != nil && err != errBreakout {
		op.rhs.SetAndUpdateRequestMessage(ctx, "VolumeSeries update error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	if err == nil {
		op.rhs.VolumeSeries = obj
	}
}

func (op *volDetachOp) syncFail(ctx context.Context) {
	if op.advertisedFailure {
		return
	}
	op.saa = &vra.SyncAbortArgs{
		LocalKey:   string(op.rhs.Request.VolumeSeriesID),
		LocalState: common.VolReqStateFailed,
	}
	op.rhs.SyncAbort(ctx, op.saa) // ignore errors
	op.advertisedFailure = true
}

func (op *volDetachOp) syncState(ctx context.Context) {
	state := string(op.rhs.Request.VolumeSeriesRequestState)
	if op.planOnly {
		op.rhs.SetRequestMessage("%s sync", state)
		return
	}
	reiLabel := fmt.Sprintf("vd-block-sync-%s", state)
	if err := op.c.rei.ErrOnBool(reiLabel); err != nil {
		op.rhs.SetAndUpdateRequestMessageDistinct(ctx, "Sync: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.sa = &vra.SyncArgs{
		LocalKey:   string(op.rhs.Request.VolumeSeriesID),
		SyncState:  state,
		CompleteBy: time.Time(op.rhs.Request.CompleteByTime),
	}
	if err := op.rhs.SyncRequests(ctx, op.sa); err != nil { // only updates local record
		op.rhs.SetRequestError("Sync error %s", err.Error())
	}
}
