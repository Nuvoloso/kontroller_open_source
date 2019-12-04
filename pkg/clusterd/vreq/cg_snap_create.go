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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/swag"
)

// vra.CGSnapshotCreateHandlers methods

// CGSnapshotCreate performs the CG_SNAPSHOT_CREATE operation
// This handler will be invoked multiple times for the same request, one per externally
// visible VSR state.
// It stashes the state machine in the RHS to minimize the overhead of the latter calls.
func (c *Component) CGSnapshotCreate(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := c.cgSnapCreateRecoverOrMakeOp(rhs)
	op.run(ctx)
}

// UndoCGSnapshotCreate undoes the UNDO_CG_SNAPSHOT_CREATE operation
func (c *Component) UndoCGSnapshotCreate(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := c.cgSnapCreateRecoverOrMakeOp(rhs)
	op.run(ctx)
}

type cgSnapCreateStashKey struct{} // stash key type

// cgSnapCreateRecoverOrMakeOp retrieves the Op structure from the rhs stash, or creates a new one
func (c *Component) cgSnapCreateRecoverOrMakeOp(rhs *vra.RequestHandlerState) *cgSnapCreateOp {
	var op *cgSnapCreateOp
	if v := rhs.StashGet(cgSnapCreateStashKey{}); v != nil {
		if stashedOp, ok := v.(*cgSnapCreateOp); ok {
			op = stashedOp
			op.skipSubordinateCheck = false
		}
	}
	if op == nil {
		op = &cgSnapCreateOp{}
	}
	op.c = c
	op.rhs = rhs
	op.ops = op // self-reference
	op.reiDelayMultiplier = time.Second
	return op
}

type cgSnapCreateSubState int

// cgSnapCreateSubState values (the ordering is meaningful)
const (
	// Initialize the subordinate map in the database
	CgSCInitMapInDB cgSnapCreateSubState = iota
	// Check for the existence of subordinate VSRs and update in-mem map if found
	CgSCCheckForSubordinates
	CgSCLaunchRemainingSubordinates
	CgSCWaitForSubordinates
	CgSCDone

	CgSCUndoCheckForSubordinates
	CgSCUndoWaitForSubordinates
	CgSCUndoDone

	CgSCError

	// LAST: No operation is performed in this state.
	CgSCNoOp
)

func (ss cgSnapCreateSubState) String() string {
	switch ss {
	case CgSCInitMapInDB:
		return "CgSCInitMapInDB"
	case CgSCCheckForSubordinates:
		return "CgSCCheckForSubordinates"
	case CgSCLaunchRemainingSubordinates:
		return "CgSCLaunchRemainingSubordinates"
	case CgSCWaitForSubordinates:
		return "CgSCWaitForSubordinates"
	case CgSCDone:
		return "CgSCDone"
	case CgSCUndoCheckForSubordinates:
		return "CgSCUndoCheckForSubordinates"
	case CgSCUndoWaitForSubordinates:
		return "CgSCUndoWaitForSubordinates"
	case CgSCUndoDone:
		return "CgSCUndoDone"
	case CgSCError:
		return "CgSCError"
	}
	return fmt.Sprintf("cgSnapCreateSubState(%d)", ss)
}

type cgSnapCreateOp struct {
	c                    *Component
	rhs                  *vra.RequestHandlerState
	ops                  cgSnapCreateOperators
	inError              bool
	planOnly             bool
	skipSubordinateCheck bool
	numCreated           int
	activeCount          int
	failedCount          int
	unknownCount         int
	reiDelayMultiplier   time.Duration
}

type cgSnapCreateOperators interface {
	checkForSubordinates(ctx context.Context)
	countActiveSubordinateVSRs(ctx context.Context)
	getInitialState(ctx context.Context) cgSnapCreateSubState
	initSubordinateMapInDB(ctx context.Context)
	launchRemainingSubordinates(ctx context.Context)
}

func (op *cgSnapCreateOp) run(ctx context.Context) {
out:
	for ss := op.ops.getInitialState(ctx); !(op.rhs.InError || op.rhs.RetryLater); ss++ {
		op.c.Log.Debugf("VolumeSeriesRequest %s: %s %s", op.rhs.Request.Meta.ID, op.rhs.Request.VolumeSeriesRequestState, ss)
		switch ss {
		case CgSCInitMapInDB:
			op.ops.initSubordinateMapInDB(ctx)
		case CgSCCheckForSubordinates:
			if !op.skipSubordinateCheck {
				op.ops.checkForSubordinates(ctx)
			}
		case CgSCLaunchRemainingSubordinates:
			if op.rhs.Request.VolumeSeriesRequestState == com.VolReqStateCGSnapshotVolumes {
				op.ops.launchRemainingSubordinates(ctx)
				if delay := op.c.rei.GetInt("cg-snapshot-pre-wait-delay"); delay > 0 { // see CUM-1924
					op.rhs.SetRequestMessage("REI cg-snapshot-pre-wait-delay %ds", delay)
					time.Sleep(time.Duration(delay) * op.reiDelayMultiplier)
				}
				break out // will re-enter in the VolReqStateCGSnapshotWait state
			}
		case CgSCWaitForSubordinates:
			if op.numCreated > 0 {
				op.rhs.RetryLater = true // creation in same lifetime: give the subordinates a chance to run
				break out
			}
			op.ops.countActiveSubordinateVSRs(ctx)
		case CgSCUndoCheckForSubordinates:
			op.ops.countActiveSubordinateVSRs(ctx)
			op.rhs.InError = false
			op.rhs.RetryLater = false
			if op.activeCount > 0 || op.unknownCount > 0 {
				op.ops.checkForSubordinates(ctx)
			}
		case CgSCUndoWaitForSubordinates:
			op.ops.countActiveSubordinateVSRs(ctx)
			if op.failedCount == 0 {
				op.rhs.TimedOut = false // ignore timeout if completed successfully
			}
		default:
			break out
		}
	}
	if op.inError {
		op.rhs.InError = true
	}
	if !op.rhs.RetryLater {
		op.rhs.StashSet(cgSnapCreateStashKey{}, op)
	}
	op.c.Log.Debugf("VolumeSeriesRequest %s: Leaving with %s InError:%v RetryLater:%v", op.rhs.Request.Meta.ID, op.rhs.Request.VolumeSeriesRequestState, op.rhs.InError, op.rhs.RetryLater)
}

func (op *cgSnapCreateOp) getInitialState(ctx context.Context) cgSnapCreateSubState {
	op.planOnly = swag.BoolValue(op.rhs.Request.PlanOnly)
	if op.rhs.Request.VolumeSeriesRequestState == com.VolReqStateUndoCGSnapshotVolumes {
		op.inError = op.rhs.InError
		op.rhs.InError = false // switch off to enable cleanup
		if op.planOnly {
			return CgSCUndoDone
		}
		return CgSCUndoCheckForSubordinates
	}
	if op.rhs.Request.VolumeSeriesRequestState == com.VolReqStateCGSnapshotFinalize {
		return CgSCDone
	}
	if op.rhs.Request.VolumeSeriesRequestState == com.VolReqStateCGSnapshotVolumes &&
		(op.rhs.Request.SyncPeers == nil || len(op.rhs.Request.SyncPeers) == 0) {
		return CgSCInitMapInDB
	}
	return CgSCCheckForSubordinates
}

// This is the only time that this VSR updates the syncPeers map: before launching the subordinate
// VSRs which will compete among themselves to update this map.
// Note that syncPeers is not an updatable property for this handler type in the VRA package.
func (op *cgSnapCreateOp) initSubordinateMapInDB(ctx context.Context) {
	lParams := volume_series.NewVolumeSeriesListParams()
	lParams.ConsistencyGroupID = swag.String(string(op.rhs.Request.ConsistencyGroupID))
	lParams.BoundClusterID = swag.String(string(op.rhs.Request.ClusterID))
	lParams.VolumeSeriesState = []string{com.VolStateInUse, com.VolStateConfigured, com.VolStateProvisioned}
	ret, err := op.c.oCrud.VolumeSeriesList(ctx, lParams)
	if err != nil {
		op.rhs.SetAndUpdateRequestMessage(ctx, "VolumeSeries list error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	spm := make(map[string]models.SyncPeer)
	for _, vol := range ret.Payload {
		sp := &models.SyncPeer{}
		if vol.VolumeSeriesState == com.VolStateProvisioned {
			// Save the last configured node hint as an indication that a CONFIGURE must be done.
			sTag := util.NewTagList(vol.SystemTags)
			nodeID, _ := sTag.Get(com.SystemTagVolumeLastConfiguredNode)
			op.c.Log.Debugf("VolumeSeriesRequest %s: PROVISONED volume [%s] LCN:%s", op.rhs.Request.Meta.ID, vol.Meta.ID, nodeID)
			if nodeID == "" {
				continue
			}
			sp.Annotation = nodeID
		}
		spm[string(vol.Meta.ID)] = *sp
	}
	if len(spm) == 0 {
		op.rhs.SetRequestError("No applicable volumes found for this cluster scope")
		return
	}
	op.rhs.SetRequestMessage("Number of volumes in cluster scope: %d", len(spm))
	items := &crud.Updates{Set: []string{"syncPeers", "requestMessages"}}
	modifyFn := func(o *models.VolumeSeriesRequest) (*models.VolumeSeriesRequest, error) {
		if o == nil {
			o = op.rhs.Request
		}
		o.SyncPeers = spm
		return o, nil
	}
	obj, err := op.c.oCrud.VolumeSeriesRequestUpdater(ctx, string(op.rhs.Request.Meta.ID), modifyFn, items)
	if err != nil {
		if e, ok := err.(*crud.Error); ok && e.IsTransient() {
			// setting an error message in the VSR won't work either - just log
			op.c.Log.Errorf("VolumeSeriesRequest %s: update error %s", op.rhs.Request.Meta.ID, err.Error())
			op.rhs.RetryLater = true
		} else {
			op.rhs.SetRequestError("VolumeSeriesRequest update error: %s", err.Error())
		}
		return
	}
	op.rhs.Request = obj
	op.skipSubordinateCheck = true // there are none...
}

// Updates the in-mem copy of the syncPeers with the ids/state of subordinates referenced in the map.
// This is to avoid creating the subordinate again and to monitor their termination later.
func (op *cgSnapCreateOp) checkForSubordinates(ctx context.Context) {
	lParams := volume_series_request.NewVolumeSeriesRequestListParams()
	lParams.SyncCoordinatorID = swag.String(string(op.rhs.Request.Meta.ID))
	ret, err := op.c.oCrud.VolumeSeriesRequestList(ctx, lParams)
	if err != nil {
		op.rhs.SetAndUpdateRequestMessage(ctx, "VolumeSeriesRequest list error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	if op.rhs.Request.SyncPeers == nil { // possible on the undo path
		op.rhs.Request.SyncPeers = make(map[string]models.SyncPeer)
	}
	for _, vsr := range ret.Payload {
		if sub, ok := op.rhs.Request.SyncPeers[string(vsr.VolumeSeriesID)]; ok {
			sub.State = vsr.VolumeSeriesRequestState
			sub.ID = models.ObjIDMutable(vsr.Meta.ID)
			op.rhs.Request.SyncPeers[string(vsr.VolumeSeriesID)] = sub
		} // otherwise ignore (a subordinate can remove itself from the map)
	}
}

// Create subordinate volume series snapshot VSRs if ids do not exist in the map
func (op *cgSnapCreateOp) launchRemainingSubordinates(ctx context.Context) {
	op.numCreated = 0
	for vsID, sub := range op.rhs.Request.SyncPeers {
		if sub.ID == "" {
			if op.planOnly {
				op.rhs.SetRequestMessage("Launch VOL_SNAPSHOT_CREATE [%s]", vsID)
				continue
			}
			cA := &models.VolumeSeriesRequestCreateArgs{}
			if sub.Annotation != "" {
				nObj := op.c.App.StateOps.GetHealthyNode(sub.Annotation)
				if nObj == nil {
					op.rhs.SetRequestError("VolumeSeriesRequest: no alternative available for Node[%s]", sub.Annotation)
					return
				}
				cA.RequestedOperations = []string{com.VolReqOpConfigure, com.VolReqOpVolCreateSnapshot}
				cA.NodeID = models.ObjIDMutable(nObj.Meta.ID)
			} else {
				cA.RequestedOperations = []string{com.VolReqOpVolCreateSnapshot}
			}
			cA.VolumeSeriesID = models.ObjIDMutable(vsID)
			cA.SyncCoordinatorID = models.ObjIDMutable(op.rhs.Request.Meta.ID)
			cA.CompleteByTime = op.rhs.Request.CompleteByTime
			o := &models.VolumeSeriesRequest{}
			o.VolumeSeriesRequestCreateOnce = cA.VolumeSeriesRequestCreateOnce
			o.VolumeSeriesRequestCreateMutable = cA.VolumeSeriesRequestCreateMutable
			_, err := op.c.oCrud.VolumeSeriesRequestCreate(ctx, o)
			if err != nil {
				if e, ok := err.(*crud.Error); ok && e.IsTransient() {
					op.rhs.SetAndUpdateRequestMessage(ctx, "VolumeSeriesRequest create error: %s", err.Error())
					op.rhs.RetryLater = true
				} else {
					op.rhs.SetRequestError("VolumeSeriesRequest create failed: %s", err.Error())
				}
				return
			}
			op.numCreated++
		}
	}
}

func (op *cgSnapCreateOp) countActiveSubordinateVSRs(ctx context.Context) {
	op.activeCount = 0
	op.failedCount = 0
	op.unknownCount = 0
	if op.planOnly {
		return
	}
	for _, sub := range op.rhs.Request.SyncPeers {
		if sub.State == "" {
			op.unknownCount++
		} else if !vra.VolumeSeriesRequestStateIsTerminated(sub.State) {
			op.activeCount++
		} else if sub.State != com.VolReqStateSucceeded {
			op.failedCount++
		}
	}
	if op.failedCount > 0 {
		op.rhs.SetRequestError("Volume snapshots failed")
	} else if op.activeCount > 0 {
		op.rhs.RetryLater = true
	}
	op.c.Log.Debugf("VolumeSeriesRequest %s: activeCount:%d failedCount:%d unknownCount:%d", op.rhs.Request.Meta.ID, op.activeCount, op.failedCount, op.unknownCount)
}
