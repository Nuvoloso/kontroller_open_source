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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/snapshot"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

// vra.CreateFromSnapshotHandlers methods

// CreateFromSnapshot performs the CREATE_FROM_SNAPSHOT operation
func (c *Component) CreateFromSnapshot(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &createFromSnapshotOp{}
	op.c = c
	op.rhs = rhs
	op.ops = op // self-reference
	op.run(ctx)
}

// UndoCreateFromSnapshot undoes the CREATE_FROM_SNAPSHOT operation
func (c *Component) UndoCreateFromSnapshot(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &createFromSnapshotOp{}
	op.c = c
	op.rhs = rhs
	op.ops = op // self-reference
	op.run(ctx)
}

type cfsSubState int

// cfsSubState values (the ordering is meaningful)
const (
	// Initialize the subordinate map in the database
	CfsInitMapInDB cfsSubState = iota
	// Check for the existence of the subordinate VSRs and update in-mem map if found
	// Also save the new volume id if known.
	CfsCheckForSubordinate
	// Launch the subordinate create, bind, mount, restore
	CfsCLaunchSubordinate
	// Analyze the state of the subordinate
	CfsAnalyzeSubordinate
	// Fetch the subordinate VSR if we do not have the new volume id
	CfsFetchSubordinate
	// Launch the subordinate publish
	CfsPublish
	// Check if publish vsr is complete
	CfsCheckPublishDone
	// Set the new volume id as this VSR's volumeSeriesId
	CfsSetResult
	CfsDone

	// UNDO path
	CfsUndoCheckForSubordinate

	// LAST: No operation is performed in this state.
	CfsNoOp
)

func (ss cfsSubState) String() string {
	switch ss {
	case CfsInitMapInDB:
		return "CfsInitMapInDB"
	case CfsCheckForSubordinate:
		return "CfsCheckForSubordinate"
	case CfsCLaunchSubordinate:
		return "CfsCLaunchSubordinate"
	case CfsAnalyzeSubordinate:
		return "CfsAnalyzeSubordinate"
	case CfsFetchSubordinate:
		return "CfsFetchSubordinate"
	case CfsPublish:
		return "CfsPublish"
	case CfsCheckPublishDone:
		return "CfsCheckPublishDone"
	case CfsSetResult:
		return "CfsSetResult"
	case CfsDone:
		return "CfsDone"
	case CfsUndoCheckForSubordinate:
		return "CfsUndoCheckForSubordinate"
	}
	return fmt.Sprintf("cfsSubState(%d)", ss)
}

type createFromSnapshotOp struct {
	c                    *Component
	rhs                  *vra.RequestHandlerState
	ops                  cfsOperators
	inError              bool
	planOnly             bool
	skipSubordinateCheck bool
	foundSubordinate     bool
	numCreated           int
	activeCount          int
	failedCount          int
	newVsID              models.ObjIDMutable
	snapIdentifier       string
	publishVsrID         string
}

type cfsOperators interface {
	analyzeSubordinate(ctx context.Context)
	checkForSubordinate(ctx context.Context)
	fetchSubordinate(ctx context.Context)
	getInitialState(ctx context.Context) cfsSubState
	initSubordinateMapInDB(ctx context.Context)
	launchSubordinate(ctx context.Context)
	setResult(ctx context.Context)
	publish(ctx context.Context)
	checkPublishDone(ctx context.Context)
	hasPublishSystemTag(ctx context.Context) bool
}

func (op *createFromSnapshotOp) run(ctx context.Context) {
	jumpToState := CfsNoOp
out:
	for ss := op.ops.getInitialState(ctx); !(op.rhs.InError || op.rhs.RetryLater); ss++ {
		if jumpToState != CfsNoOp {
			ss = jumpToState
			jumpToState = CfsNoOp
		}
		op.c.Log.Debugf("VolumeSeriesRequest %s: %s %s", op.rhs.Request.Meta.ID, op.rhs.Request.VolumeSeriesRequestState, ss)
		switch ss {
		case CfsInitMapInDB:
			op.ops.initSubordinateMapInDB(ctx)
		case CfsCheckForSubordinate:
			if !op.skipSubordinateCheck {
				op.ops.checkForSubordinate(ctx)
			}
		case CfsCLaunchSubordinate:
			if !op.foundSubordinate {
				op.ops.launchSubordinate(ctx)
			}
		case CfsAnalyzeSubordinate:
			op.ops.analyzeSubordinate(ctx)
			if op.failedCount == 0 {
				op.rhs.TimedOut = false // ignore time out if subordinate succeeded
			}
		case CfsFetchSubordinate:
			if op.newVsID == "" {
				op.ops.fetchSubordinate(ctx)
			}
		case CfsPublish:
			op.ops.publish(ctx)
		case CfsCheckPublishDone:
			op.ops.checkPublishDone(ctx)
		case CfsSetResult:
			op.ops.setResult(ctx)
		case CfsUndoCheckForSubordinate:
			op.ops.checkForSubordinate(ctx)
			if op.foundSubordinate {
				jumpToState = CfsAnalyzeSubordinate
			}
		default:
			break out
		}
	}
	if op.rhs.TimedOut {
		op.rhs.SetRequestMessage("Timed out") // suppressed in pkg/vra
	}
}

// checks the subordinate state in the map
func (op *createFromSnapshotOp) analyzeSubordinate(ctx context.Context) {
	op.activeCount = 0
	op.failedCount = 0
	if op.planOnly {
		return
	}
	if !op.foundSubordinate {
		panic(fmt.Sprintf("VolumeSeriesRequest %s: !op.foundSubordinate", op.rhs.Request.Meta.ID))
	}
	for _, sub := range op.rhs.Request.SyncPeers {
		if !vra.VolumeSeriesRequestStateIsTerminated(sub.State) {
			op.activeCount++
		} else if sub.State != com.VolReqStateSucceeded {
			op.failedCount++
		}
	}
	if op.failedCount > 0 {
		op.rhs.SetRequestError("Volume snapshot restore failed")
	} else if op.activeCount > 0 {
		op.rhs.RetryLater = true
	}
	op.c.Log.Debugf("VolumeSeriesRequest %s: activeCount:%d failedCount:%d", op.rhs.Request.Meta.ID, op.activeCount, op.failedCount)
}

// Updates the in-mem copy of the syncPeers with the ids/state of subordinates referenced in the map.
// This is to avoid creating the subordinate again and to monitor their termination later.
func (op *createFromSnapshotOp) checkForSubordinate(ctx context.Context) {
	if op.planOnly {
		return
	}
	op.foundSubordinate = false
	lParams := volume_series_request.NewVolumeSeriesRequestListParams()
	lParams.SyncCoordinatorID = swag.String(string(op.rhs.Request.Meta.ID))
	ret, err := op.c.oCrud.VolumeSeriesRequestList(ctx, lParams)
	if err != nil {
		op.rhs.SetAndUpdateRequestMessage(ctx, "VolumeSeriesRequest list error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	for _, vsr := range ret.Payload {
		sub, _ := op.rhs.Request.SyncPeers[op.snapIdentifier]
		sub.State = vsr.VolumeSeriesRequestState
		sub.ID = models.ObjIDMutable(vsr.Meta.ID)
		op.newVsID = vsr.VolumeSeriesID // will be set after CREATE
		op.rhs.Request.SyncPeers[op.snapIdentifier] = sub
		op.foundSubordinate = true
		op.c.Log.Debugf("VolumeSeriesRequest %s: found sub [%s] %s", op.rhs.Request.Meta.ID, sub.ID, sub.State)
		break
	}
}

func (op *createFromSnapshotOp) getInitialState(ctx context.Context) cfsSubState {
	op.planOnly = swag.BoolValue(op.rhs.Request.PlanOnly)
	op.snapIdentifier = string(op.rhs.Request.Snapshot.SnapIdentifier)
	if op.rhs.Request.VolumeSeriesRequestState == com.VolReqStateUndoCreatingFromSnapshot {
		op.rhs.Request.VolumeSeriesRequestState = com.VolReqStateCreatingFromSnapshot // pretend undo doesn't exist
		// TBD: pass cancel to the subordinate
		return CfsUndoCheckForSubordinate
	}
	if op.rhs.Request.SyncPeers == nil || len(op.rhs.Request.SyncPeers) == 0 {
		return CfsInitMapInDB
	}
	if op.rhs.Request.VolumeSeriesRequestState == com.VolReqStateSnapshotRestoreDone { // subordinate terminated successfully
		return CfsFetchSubordinate
	}
	return CfsCheckForSubordinate
}

// This is the only time that this VSR updates the syncPeers map: before launching the subordinate
// Note that syncPeers is not an updatable property for this handler type in the VRA package.
func (op *createFromSnapshotOp) initSubordinateMapInDB(ctx context.Context) {
	spm := make(map[string]models.SyncPeer)
	spm[string(op.snapIdentifier)] = models.SyncPeer{}
	op.rhs.Request.SyncPeers = spm
	items := &crud.Updates{Set: []string{"syncPeers"}}
	modifyFn := func(o *models.VolumeSeriesRequest) (*models.VolumeSeriesRequest, error) {
		if o == nil {
			o = op.rhs.Request
		}
		return o, nil
	}
	obj, err := op.c.oCrud.VolumeSeriesRequestUpdater(ctx, string(op.rhs.Request.Meta.ID), modifyFn, items)
	if err != nil {
		// setting an error message in the VSR won't work either - just log
		op.c.Log.Errorf("VolumeSeriesRequest %s: update error %s", op.rhs.Request.Meta.ID, err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.rhs.Request = obj
	op.skipSubordinateCheck = true // there are none...
}

func (op *createFromSnapshotOp) fetchSubordinate(ctx context.Context) {
	if op.planOnly {
		op.rhs.SetRequestMessage("Fetch subordinate VSR")
		return
	}
	sub, _ := op.rhs.Request.SyncPeers[op.snapIdentifier]
	vsr, err := op.c.oCrud.VolumeSeriesRequestFetch(ctx, string(sub.ID))
	if err != nil {
		op.rhs.RetryLater = true
		return
	}
	op.newVsID = vsr.VolumeSeriesID
}

func (op *createFromSnapshotOp) launchSubordinate(ctx context.Context) {
	op.numCreated = 0
	sub, _ := op.rhs.Request.SyncPeers[op.snapIdentifier]
	if sub.ID == "" {
		op.rhs.SetRequestMessage("Launching CREATE, BIND, MOUNT, VOL_SNAPSHOT_RESTORE [%s, %s]", op.rhs.Request.Snapshot.VolumeSeriesID, op.snapIdentifier)
		if op.planOnly {
			return
		}
		// fetch snapshot from the model
		vsID := string(op.rhs.Request.Snapshot.VolumeSeriesID)
		lParams := snapshot.NewSnapshotListParams()
		lParams.SnapIdentifier = &op.snapIdentifier
		lParams.VolumeSeriesID = &vsID
		snapshots, err := op.c.oCrud.SnapshotList(ctx, lParams)
		if err != nil {
			op.rhs.RetryLater = true
			return
		}
		if len(snapshots.Payload) != 1 {
			op.rhs.SetRequestError("Snapshot %s invalid", op.snapIdentifier)
			return
		}
		sd := &models.SnapshotData{}
		sd.PitIdentifier = snapshots.Payload[0].PitIdentifier
		sd.ProtectionDomainID = snapshots.Payload[0].ProtectionDomainID
		sd.SnapIdentifier = op.snapIdentifier
		sd.ConsistencyGroupID = models.ObjIDMutable(snapshots.Payload[0].ConsistencyGroupID)
		sd.SnapTime = snapshots.Payload[0].SnapTime
		sd.SizeBytes = &snapshots.Payload[0].SizeBytes
		sd.VolumeSeriesID = op.rhs.Request.Snapshot.VolumeSeriesID
		sd.DeleteAfterTime = snapshots.Payload[0].DeleteAfterTime
		sd.Locations = []*models.SnapshotLocation{}
		for _, loc := range snapshots.Payload[0].Locations {
			sd.Locations = append(sd.Locations, &models.SnapshotLocation{
				CreationTime: loc.CreationTime,
				CspDomainID:  loc.CspDomainID,
			})
		}

		cA := &models.VolumeSeriesRequestCreateArgs{}
		cA.RequestedOperations = []string{com.VolReqOpCreate, com.VolReqOpBind, com.VolReqOpMount, com.VolReqOpVolRestoreSnapshot}
		cA.SyncCoordinatorID = models.ObjIDMutable(op.rhs.Request.Meta.ID)
		cA.CompleteByTime = op.rhs.Request.CompleteByTime
		cA.VolumeSeriesCreateSpec = op.rhs.Request.VolumeSeriesCreateSpec
		tl := util.NewTagList(cA.VolumeSeriesCreateSpec.SystemTags)
		tl.Set(com.SystemTagVsrRestoring, string(op.rhs.Request.Meta.ID)) // prevent snapshot until restore completed
		cA.VolumeSeriesCreateSpec.SystemTags = tl.List()
		cA.NodeID = op.rhs.Request.NodeID
		cA.ClusterID = op.rhs.Request.ClusterID
		cA.Snapshot = sd
		cA.Snapshot.VolumeSeriesID = op.rhs.Request.Snapshot.VolumeSeriesID
		cA.Snapshot.SnapIdentifier = op.snapIdentifier
		cA.ProtectionDomainID = sd.ProtectionDomainID
		o := &models.VolumeSeriesRequest{}
		o.VolumeSeriesRequestCreateOnce = cA.VolumeSeriesRequestCreateOnce
		o.VolumeSeriesRequestCreateMutable = cA.VolumeSeriesRequestCreateMutable
		_, err = op.c.oCrud.VolumeSeriesRequestCreate(ctx, o)
		if err != nil {
			op.rhs.SetAndUpdateRequestMessage(ctx, "VolumeSeriesRequest create error: %s", err.Error())
			op.rhs.RetryLater = true
			return
		}
		op.numCreated++
		op.rhs.RetryLater = true // allow the new vsr to run
	}
}

func (op *createFromSnapshotOp) setResult(ctx context.Context) {
	if op.planOnly {
		op.rhs.SetRequestMessage("Set the new volumeSeries ID in systemTags")
		return
	}
	sTags := util.NewTagList(op.rhs.Request.SystemTags)
	if _, exists := sTags.Get(com.SystemTagVsrNewVS); !exists {
		sTags.Set(com.SystemTagVsrNewVS, string(op.newVsID))
		op.rhs.Request.SystemTags = sTags.List()
		op.rhs.SetRequestMessage("New volumeSeriesId is [%s]", op.newVsID)
	}
	op.rhs.Request.VolumeSeriesRequestState = com.VolReqStateSnapshotRestoreDone // in case of undo
}

func (op *createFromSnapshotOp) publish(ctx context.Context) {
	if op.planOnly {
		return
	}
	if op.hasPublishSystemTag(ctx) {
		return
	}
	vsrLParams := &volume_series_request.VolumeSeriesRequestListParams{
		IsTerminated:   swag.Bool(false),
		VolumeSeriesID: swag.String(string(op.newVsID)),
	}
	vsrList, err := op.c.oCrud.VolumeSeriesRequestList(ctx, vsrLParams)
	if err != nil {
		op.rhs.SetAndUpdateRequestMessage(ctx, "VolumeSeriesRequest list error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	if len(vsrList.Payload) > 0 {
		op.rhs.RetryLater = true
		return
	}
	op.rhs.SetAndUpdateRequestMessage(ctx, "Launching PUBLISH [%s]", op.newVsID)
	cA := &models.VolumeSeriesRequestCreateArgs{}
	cA.RequestedOperations = []string{com.VolReqOpPublish}
	cA.SyncCoordinatorID = models.ObjIDMutable(op.rhs.Request.Meta.ID)
	if time.Time(op.rhs.Request.CompleteByTime).Before(time.Now().Add(time.Minute)) {
		cA.CompleteByTime = strfmt.DateTime(time.Now().Add(time.Minute)) // adds an extra minute to ensure Publish succeeds
	} else {
		cA.CompleteByTime = op.rhs.Request.CompleteByTime
	}
	cA.ClusterID = op.rhs.Request.ClusterID
	cA.VolumeSeriesID = op.newVsID
	o := &models.VolumeSeriesRequest{}
	o.VolumeSeriesRequestCreateOnce = cA.VolumeSeriesRequestCreateOnce
	o.VolumeSeriesRequestCreateMutable = cA.VolumeSeriesRequestCreateMutable
	pVSR, err := op.c.oCrud.VolumeSeriesRequestCreate(ctx, o)
	if err != nil {
		op.rhs.SetAndUpdateRequestMessage(ctx, "VolumeSeriesRequest create error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}

	// In case of clusterd failure it may retry publish, which is ok.
	sTag := util.NewTagList(op.rhs.Request.SystemTags)
	sTag.Set(com.SystemTagVsrPublishVsr, string(pVSR.Meta.ID))
	op.rhs.Request.SystemTags = sTag.List()
	items := &crud.Updates{Set: []string{"systemTags"}}
	op.rhs.UpdateRequestWithItems(ctx, items)
	op.publishVsrID = string(pVSR.Meta.ID)
	time.Sleep(1 * time.Second) // sleeping to try and avoid race condition
}

func (op *createFromSnapshotOp) hasPublishSystemTag(ctx context.Context) bool {
	if op.rhs.Request.SystemTags == nil {
		return false
	}
	sTag := util.NewTagList(op.rhs.Request.SystemTags)
	if val, found := sTag.Get(com.SystemTagVsrPublishVsr); found {
		op.publishVsrID = val
		return true
	}
	return false
}

func (op *createFromSnapshotOp) checkPublishDone(ctx context.Context) {
	vsr, err := op.c.oCrud.VolumeSeriesRequestFetch(ctx, op.publishVsrID)
	if err != nil {
		op.rhs.RetryLater = true
		return
	}
	if !util.Contains(vra.TerminalVolumeSeriesRequestStates(), vsr.VolumeSeriesRequestState) {
		op.rhs.RetryLater = true
		return
	}
	if vsr.VolumeSeriesRequestState != com.VolReqStateSucceeded {
		op.rhs.InError = true
		return
	}
}
