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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

type nodeDeleteStashKey struct{} // stash key type

// NodeDelete implements the vra.NodeDeleter interface
func (c *Component) NodeDelete(ctx context.Context, rhs *vra.RequestHandlerState) {
	var op *nodeDeleteOp
	if v := rhs.StashGet(nodeDeleteStashKey{}); v != nil {
		if stashedOp, ok := v.(*nodeDeleteOp); ok {
			op = stashedOp
		}
	}
	if op == nil {
		op = &nodeDeleteOp{}
	}
	op.c = c
	op.rhs = rhs
	op.ops = op // self-reference
	op.run(ctx)
}

type nodeDeleteSubState int

// nodeDeleteSubState values
const (
	// DRAINING_REQUESTS
	NdLoadNode nodeDeleteSubState = iota
	NdTeardownNode
	NdSearchForDrainableSRs
	NdWaitForDrainableSRs
	NdSearchForDrainableVSRs
	NdWaitForDrainableVSRs
	NdDrainingRequestsBreakout

	// CANCELING_REQUESTS
	NdFindActiveSRs
	NdFailSRs
	NdSearchForCancelableVSRs
	NdCancelVSRs
	NdWaitForCancelingVSRs
	NdSearchForRemainingVSRs
	NdForceCancelRemainingVSRs
	NdCancelingRequestsBreakout

	// DETACHING_VOLUMES
	NdFindConfiguredVolumes
	NdPrimePeerMap
	NdCheckForSubordinates
	NdLaunchRemainingSubordinates
	NdWaitForSubordinateResponses
	NdDetachingVolumesBreakout

	// DETACHING_STORAGE
	NdFindAttachedStorage
	NdFindDetachSRs
	NdLaunchDetachSR
	NdWaitForDetachSRs
	NdDetachingStorageBreakout

	// VOLUME_DETACH_WAIT
	NdVolumeDetachWaitSync
	NdVolumeDetachWaitSyncBreakout

	// VOLUME_DETACHED
	NdVolumeDetachedWaitSync
	NdVolumeDetachedWaitSyncBreakout

	// DELETING_NODE
	NdDeletingNode
	NdDeletingNodeBreakout

	// Misc
	NdError
	NdNoOp
)

func (ss nodeDeleteSubState) String() string {
	switch ss {
	case NdLoadNode:
		return "NdLoadNode"
	case NdTeardownNode:
		return "NdTeardownNode"
	case NdSearchForDrainableSRs:
		return "NdSearchForDrainableSRs"
	case NdWaitForDrainableSRs:
		return "NdWaitForDrainableSRs"
	case NdSearchForDrainableVSRs:
		return "NdSearchForDrainableVSRs"
	case NdWaitForDrainableVSRs:
		return "NdWaitForDrainableVSRs"
	case NdDrainingRequestsBreakout:
		return "NdDrainingRequestsBreakout"
	case NdFindActiveSRs:
		return "NdFindActiveSRs"
	case NdFailSRs:
		return "NdFailSRs"
	case NdSearchForCancelableVSRs:
		return "NdSearchForCancelableVSRs"
	case NdCancelVSRs:
		return "NdCancelVSRs"
	case NdWaitForCancelingVSRs:
		return "NdWaitForCancelingVSRs"
	case NdSearchForRemainingVSRs:
		return "NdSearchForRemainingVSRs"
	case NdForceCancelRemainingVSRs:
		return "NdForceCancelRemainingVSRs"
	case NdCancelingRequestsBreakout:
		return "NdCancelingRequestsBreakout"
	case NdFindConfiguredVolumes:
		return "NdFindConfiguredVolumes"
	case NdPrimePeerMap:
		return "NdPrimePeerMap"
	case NdCheckForSubordinates:
		return "NdCheckForSubordinates"
	case NdLaunchRemainingSubordinates:
		return "NdLaunchRemainingSubordinates"
	case NdWaitForSubordinateResponses:
		return "NdWaitForSubordinateResponses"
	case NdDetachingVolumesBreakout:
		return "NdDetachingVolumesBreakout"
	case NdFindAttachedStorage:
		return "NdFindAttachedStorage"
	case NdFindDetachSRs:
		return "NdFindDetachSRs"
	case NdLaunchDetachSR:
		return "NdLaunchDetachSR"
	case NdWaitForDetachSRs:
		return "NdWaitForDetachSRs"
	case NdDetachingStorageBreakout:
		return "NdDetachingStorageBreakout"
	case NdVolumeDetachWaitSync:
		return "NdVolumeDetachWaitSync"
	case NdVolumeDetachWaitSyncBreakout:
		return "NdVolumeDetachWaitSyncBreakout"
	case NdVolumeDetachedWaitSync:
		return "NdVolumeDetachedWaitSync"
	case NdVolumeDetachedWaitSyncBreakout:
		return "NdVolumeDetachedWaitSyncBreakout"
	case NdDeletingNode:
		return "NdDeletingNode"
	case NdDeletingNodeBreakout:
		return "NdDeletingNodeBreakout"
	case NdError:
		return "NdError"
	}
	return fmt.Sprintf("ndSubState(%d)", ss)
}

type nodeDeleteOp struct {
	c                    *Component
	rhs                  *vra.RequestHandlerState
	ops                  nodeDeleteOperators
	planOnly             bool
	skipSubordinateCheck bool
	advertisedFailure    bool
	numCreated           int
	numUnresponsiveSubs  int
	nObj                 *models.Node
	srObjs               []*models.StorageRequest // all active SRs for the node
	srsToFail            []*models.StorageRequest
	srsToWaitOn          []*models.StorageRequest
	vsObjs               []*models.VolumeSeries
	vsrObjs              []*models.VolumeSeriesRequest // all active vsrs
	vsrsToCancel         []*models.VolumeSeriesRequest
	vsrsToWaitOn         []*models.VolumeSeriesRequest
	vsrsToFail           []*models.VolumeSeriesRequest
	stgObjs              []*models.Storage // attached storage
	stgIDsWithSR         []models.ObjID
	detachSrObjs         []*models.StorageRequest // DETACH SRs to wait on
	sa                   *vra.SyncArgs
	saa                  *vra.SyncAbortArgs
	syncID               models.ObjIDMutable
}

type nodeDeleteOperators interface {
	allSubordinatesResponded() bool
	cancelVSRs(ctx context.Context)
	checkForSubordinates(ctx context.Context)
	deleteNode(ctx context.Context)
	failSRs(ctx context.Context, srs []*models.StorageRequest) []*models.StorageRequest
	fetchNode(ctx context.Context)
	filterActiveSRs(ctx context.Context)
	findAttachedStorage(ctx context.Context)
	findDetachSRs(ctx context.Context)
	forceCancelRemainingVSRs(ctx context.Context)
	getInitialState(ctx context.Context) nodeDeleteSubState
	launchDetachSRs(ctx context.Context) []*models.StorageRequest
	launchRemainingSubordinates(ctx context.Context)
	listActiveSRs(ctx context.Context)
	listAndClassifyNodeVSRs(ctx context.Context)
	listConfiguredVS(ctx context.Context)
	primePeerMap(ctx context.Context)
	syncFail(ctx context.Context)
	syncState(ctx context.Context)
	teardownNode(ctx context.Context)
	waitForSRs(ctx context.Context, srs []*models.StorageRequest) []*models.StorageRequest
	waitForVSRs(ctx context.Context, vsrs []*models.VolumeSeriesRequest) []*models.VolumeSeriesRequest
}

func (op *nodeDeleteOp) run(ctx context.Context) {
	jumpToState := NdNoOp
out:
	for ss := op.ops.getInitialState(ctx); !(op.rhs.InError || op.rhs.RetryLater); ss++ {
		if jumpToState != NdNoOp {
			ss = jumpToState
			jumpToState = NdNoOp
		}
		op.c.Log.Debugf("VolumeSeriesRequest %s: NODE_DELETE %s %s", op.rhs.Request.Meta.ID, op.rhs.Request.VolumeSeriesRequestState, ss)
		switch ss {
		// DRAINING_REQUESTS
		case NdLoadNode:
			op.ops.fetchNode(ctx)
			if op.nObj != nil && op.nObj.State == common.NodeStateTearDown {
				jumpToState = NdTeardownNode + 1
			}
		case NdTeardownNode:
			op.ops.teardownNode(ctx)
		case NdSearchForDrainableSRs:
			op.ops.listActiveSRs(ctx)
			if len(op.srObjs) > 0 {
				op.ops.filterActiveSRs(ctx)
			} else {
				jumpToState = NdWaitForDrainableSRs + 1
			}
		case NdWaitForDrainableSRs:
			if len(op.srsToWaitOn) > 0 {
				op.ops.waitForSRs(ctx, op.srsToWaitOn)
			}
		case NdSearchForDrainableVSRs:
			op.ops.listAndClassifyNodeVSRs(ctx)
		case NdWaitForDrainableVSRs:
			if len(op.vsrsToWaitOn) > 0 {
				op.ops.waitForVSRs(ctx, op.vsrsToWaitOn) // non-empty return ⇒ retry later set
			}

		// CANCELING_REQUESTS
		case NdFindActiveSRs:
			op.ops.listActiveSRs(ctx)
			if len(op.srObjs) == 0 {
				jumpToState = NdFailSRs + 1
			}
		case NdFailSRs:
			if len(op.srObjs) > 0 { // fail all remaining SRs (all those that not yet completed and those in agentd)
				op.ops.failSRs(ctx, op.srObjs)
			}
		case NdSearchForCancelableVSRs:
			op.ops.listAndClassifyNodeVSRs(ctx)
			if len(op.vsrsToCancel) == 0 {
				jumpToState = NdForceCancelRemainingVSRs
			}
		case NdCancelVSRs:
			op.ops.cancelVSRs(ctx)
		case NdWaitForCancelingVSRs:
			op.ops.waitForVSRs(ctx, op.vsrsToCancel)
		case NdSearchForRemainingVSRs:
			op.ops.listAndClassifyNodeVSRs(ctx)
		case NdForceCancelRemainingVSRs:
			if len(op.vsrObjs) > 0 {
				op.ops.forceCancelRemainingVSRs(ctx) // retry later set on error
			}

		// DETACHING_VOLUMES
		case NdFindConfiguredVolumes:
			op.ops.listConfiguredVS(ctx)
		case NdPrimePeerMap:
			op.ops.primePeerMap(ctx)
			if op.skipSubordinateCheck {
				jumpToState = NdLaunchRemainingSubordinates
			}
		case NdCheckForSubordinates:
			op.ops.checkForSubordinates(ctx)
		case NdLaunchRemainingSubordinates:
			op.ops.launchRemainingSubordinates(ctx)
			if op.numCreated > 0 {
				break out // give subordinates a chance to run
			}
		case NdWaitForSubordinateResponses:
			if !op.ops.allSubordinatesResponded() {
				op.rhs.RetryLater = true
			}

		// DETACHING_STORAGE
		case NdFindAttachedStorage:
			op.ops.findAttachedStorage(ctx)
		case NdFindDetachSRs:
			op.ops.findDetachSRs(ctx)
		case NdLaunchDetachSR:
			op.ops.launchDetachSRs(ctx)
		case NdWaitForDetachSRs:
			if len(op.detachSrObjs) > 0 {
				op.ops.waitForSRs(ctx, op.detachSrObjs)
			}

		// VOLUME_DETACH_WAIT
		case NdVolumeDetachWaitSync:
			if len(op.rhs.Request.SyncPeers) > 0 {
				op.ops.syncState(ctx)
			}
		// VOLUME_DETACHED
		case NdVolumeDetachedWaitSync:
			if len(op.rhs.Request.SyncPeers) > 0 {
				op.ops.syncState(ctx)
			}
		// DELETING_NODE
		case NdDeletingNode:
			op.ops.deleteNode(ctx)

		default:
			break out
		}
	}
	if op.rhs.InError {
		op.ops.syncFail(ctx)
	}
	if !(op.rhs.RetryLater || op.rhs.InError) {
		op.rhs.StashSet(nodeDeleteStashKey{}, op)
	}
}

func (op *nodeDeleteOp) getInitialState(ctx context.Context) nodeDeleteSubState {
	op.planOnly = swag.BoolValue(op.rhs.Request.PlanOnly)
	if !op.planOnly {
		op.syncID = models.ObjIDMutable(op.rhs.Request.Meta.ID)
	}
	switch op.rhs.Request.VolumeSeriesRequestState {
	case common.VolReqStateDrainingRequests:
		return NdLoadNode
	case common.VolReqStateCancelingRequests:
		return NdFindActiveSRs
	case common.VolReqStateDetachingVolumes:
		if op.rhs.Request.SyncPeers != nil {
			return NdCheckForSubordinates
		}
		return NdFindConfiguredVolumes
	case common.VolReqStateDetachingStorage:
		return NdFindAttachedStorage
	case common.VolReqStateVolumeDetachWait:
		return NdVolumeDetachWaitSync
	case common.VolReqStateVolumeDetached:
		return NdVolumeDetachedWaitSync
	case common.VolReqStateDeletingNode:
		return NdDeletingNode
	}
	op.rhs.InError = true
	return NdError
}

func (op *nodeDeleteOp) allSubordinatesResponded() bool {
	if op.planOnly {
		return true
	}
	n := 0
	for k, sub := range op.rhs.Request.SyncPeers {
		if k != string(op.rhs.Request.NodeID) && sub.ID == "" {
			n++
		}
	}
	op.c.Log.Debugf("VolumeSeriesRequest %s: #subs:%d #Unresponsive:%d", op.rhs.Request.Meta.ID, len(op.rhs.Request.SyncPeers)-1, n)
	return n == 0
}

func (op *nodeDeleteOp) cancelVSRs(ctx context.Context) {
	if op.planOnly {
		op.rhs.SetAndUpdateRequestMessageRepeated(ctx, "Canceling VSRs")
		return
	}
	rm := []*models.TimestampedString{
		&models.TimestampedString{
			Message: fmt.Sprintf("Canceled because Node [%s] was deleted", op.rhs.Request.NodeID),
		},
	}
	st := []string{fmt.Sprintf("%s:%s", common.SystemTagVsrNodeDeleted, op.rhs.Request.NodeID)}
	items := &crud.Updates{Append: []string{"requestMessages", "systemTags"}}
	newList := make([]*models.VolumeSeriesRequest, len(op.vsrsToCancel))
	for i, vsr := range op.vsrsToCancel {
		newList[i] = vsr
		if vsr.CancelRequested {
			continue
		}
		op.c.Log.Debugf("VolumeSeriesRequest %s: canceling VSR[%s]", op.rhs.Request.Meta.ID, vsr.Meta.ID)

		rm[0].Time = strfmt.DateTime(time.Now())
		vsr.RequestMessages = rm
		vsr.SystemTags = st
		op.c.oCrud.VolumeSeriesRequestUpdate(ctx, vsr, items) // ignore errors
		o, err := op.c.oCrud.VolumeSeriesRequestCancel(ctx, string(vsr.Meta.ID))
		if err != nil {
			op.rhs.SetAndUpdateRequestMessageRepeated(ctx, "VSR[%s] cancel error %s", vsr.Meta.ID, err.Error())
			op.rhs.RetryLater = true
			continue
		}
		newList[i] = o
		op.rhs.SetAndUpdateRequestMessageRepeated(ctx, "Cancelled VSR[%s]", vsr.Meta.ID)
	}
	op.vsrsToCancel = newList
}

// Updates the in-mem copy of the syncPeers with the ids/state of subordinates referenced in the map.
// This is to avoid creating the subordinate again.
func (op *nodeDeleteOp) checkForSubordinates(ctx context.Context) {
	if len(op.rhs.Request.SyncPeers) == 0 {
		return // no configured volumes
	}
	lParams := volume_series_request.NewVolumeSeriesRequestListParams()
	lParams.SyncCoordinatorID = swag.String(string(op.rhs.Request.Meta.ID))
	lParams.IsTerminated = swag.Bool(false)
	ret, err := op.c.oCrud.VolumeSeriesRequestList(ctx, lParams)
	if err != nil {
		op.rhs.RetryLater = true
		return
	}
	// assume syncPeer map is not nil
	for _, vsr := range ret.Payload {
		if sub, ok := op.rhs.Request.SyncPeers[string(vsr.VolumeSeriesID)]; ok {
			sub.State = vsr.VolumeSeriesRequestState
			sub.ID = models.ObjIDMutable(vsr.Meta.ID)
			op.rhs.Request.SyncPeers[string(vsr.VolumeSeriesID)] = sub
		} // otherwise ignore
	}
}

func (op *nodeDeleteOp) deleteNode(ctx context.Context) {
	if op.planOnly {
		op.rhs.SetAndUpdateRequestMessageRepeated(ctx, "Deleting node")
		return
	}
	err := op.c.oCrud.NodeDelete(ctx, string(op.rhs.Request.NodeID))
	if err != nil {
		op.rhs.RetryLater = true
		return
	}
	op.rhs.SetAndUpdateRequestMessageRepeated(ctx, "Deleted node")
}

// Transition any active SRs that would be executing in agentd on the node to FAILED.
func (op *nodeDeleteOp) failSRs(ctx context.Context, srs []*models.StorageRequest) []*models.StorageRequest {
	ts := &models.TimestampedString{
		Message: fmt.Sprintf("Failing request due to node being deleted"),
		Time:    strfmt.DateTime(time.Now()),
	}
	st := []string{fmt.Sprintf("%s:%s", common.SystemTagVsrNodeDeleted, op.rhs.Request.NodeID)}
	items := &crud.Updates{Set: []string{"storageRequestState"}, Append: []string{"requestMessages", "systemTags"}}
	var remainingSRs []*models.StorageRequest
	for _, sr := range srs {
		op.c.Log.Debugf("VolumeSeriesRequest %s: Failing SR [%s, ro:%v, srs:%s]", op.rhs.Request.Meta.ID, sr.Meta.ID, sr.RequestedOperations, sr.StorageRequestState)
		sr.StorageRequestState = common.StgReqStateFailed
		sr.RequestMessages = append(sr.RequestMessages, ts)
		sr.SystemTags = st
		if _, err := op.c.oCrud.StorageRequestUpdate(ctx, sr, items); err != nil {
			op.rhs.SetAndUpdateRequestMessageRepeated(ctx, "SR[%s] fail error %s", sr.Meta.ID, err.Error())
			op.rhs.RetryLater = true
			remainingSRs = append(remainingSRs, sr)
		}
		op.rhs.SetAndUpdateRequestMessageRepeated(ctx, "Terminated SR[%s]", sr.Meta.ID)
	}
	return remainingSRs
}

func (op *nodeDeleteOp) fetchNode(ctx context.Context) {
	if op.nObj != nil {
		return
	}
	nObj, err := op.c.oCrud.NodeFetch(ctx, string(op.rhs.Request.NodeID))
	if err != nil {
		op.rhs.RetryLater = true
		return
	}
	op.nObj = nObj
}

func (op *nodeDeleteOp) forceCancelRemainingVSRs(ctx context.Context) {
	if op.planOnly {
		op.rhs.SetAndUpdateRequestMessageRepeated(ctx, "Force canceling remaining vsrs")
		return
	}
	rm := []*models.TimestampedString{
		&models.TimestampedString{
			Message: fmt.Sprintf("Canceled because Node [%s] was deleted", op.rhs.Request.NodeID),
		},
	}
	st := []string{fmt.Sprintf("%s:%s", common.SystemTagVsrNodeDeleted, op.rhs.Request.NodeID)}
	items := &crud.Updates{Set: []string{"volumeSeriesRequestState"}, Append: []string{"requestMessages", "systemTags"}}
	for _, vsr := range op.vsrObjs {
		op.c.Log.Debugf("VolumeSeriesRequest %s: force canceling VSR[%s]", op.rhs.Request.Meta.ID, vsr.Meta.ID)
		vsr.VolumeSeriesRequestState = common.VolReqStateCanceled
		rm[0].Time = strfmt.DateTime(time.Now())
		vsr.RequestMessages = rm
		vsr.SystemTags = st
		if _, err := op.c.oCrud.VolumeSeriesRequestUpdate(ctx, vsr, items); err != nil {
			op.rhs.SetAndUpdateRequestMessageRepeated(ctx, "VSR[%s] force cancel error %s", vsr.Meta.ID, err.Error())
			op.rhs.RetryLater = true
		}
		op.rhs.SetAndUpdateRequestMessageRepeated(ctx, "Force cancelled VSR[%s]", vsr.Meta.ID)
	}
}

// filter SRs to fail (executing in agentd) and the remaining to wait on
func (op *nodeDeleteOp) filterActiveSRs(ctx context.Context) {
	var buf bytes.Buffer
	for _, sr := range op.srObjs {
		fmt.Fprintf(&buf, "- [%s] O%v S:%s", sr.Meta.ID, sr.RequestedOperations, sr.StorageRequestState)
		switch vra.GetSRProcess(sr.StorageRequestState, sr.RequestedOperations[0]) {
		case vra.ApAgentd:
			fmt.Fprint(&buf, " Agentd ⇒ FAIL")
			op.srsToFail = append(op.srsToFail, sr)
		case vra.ApCentrald:
			fmt.Fprint(&buf, " !Agentd ⇒ WAIT")
			op.srsToWaitOn = append(op.srsToWaitOn, sr)
		}
		fmt.Fprint(&buf, "\n")
	}
	op.c.Log.Debugf("VolumeSeriesRequest %s: SRs classification: #=%d F=%d W=%d\n%s", op.rhs.Request.Meta.ID, len(op.srObjs), len(op.srsToFail), len(op.srsToWaitOn), buf.String())
	op.rhs.SetAndUpdateRequestMessageRepeated(ctx, "SRs classification: #=%d F=%d W=%d\n%s", len(op.srObjs), len(op.srsToFail), len(op.srsToWaitOn), buf.String())
}

func (op *nodeDeleteOp) findAttachedStorage(ctx context.Context) {
	lParams := storage.NewStorageListParams()
	lParams.AttachedNodeID = swag.String(string(op.rhs.Request.NodeID))
	res, err := op.c.oCrud.StorageList(ctx, lParams)
	if err != nil {
		op.rhs.RetryLater = true
		return
	}
	op.stgObjs = res.Payload
}

func (op *nodeDeleteOp) findDetachSRs(ctx context.Context) {
	fdTag := fmt.Sprintf("%s:%s", common.SystemTagForceDetachNodeID, op.rhs.Request.NodeID)
	lParams := storage_request.NewStorageRequestListParams()
	lParams.NodeID = swag.String(string(op.rhs.Request.NodeID))
	lParams.IsTerminated = swag.Bool(false)
	lParams.SystemTags = []string{fdTag}
	res, err := op.c.oCrud.StorageRequestList(ctx, lParams)
	if err != nil {
		op.rhs.RetryLater = true
		return
	}
	for _, s := range op.stgObjs {
		for _, sr := range res.Payload {
			op.c.Log.Debugf("VolumeSeriesRequest %s: active SR [%s:%v] for Storage %s", op.rhs.Request.Meta.ID, sr.Meta.ID, sr.RequestedOperations, s.Meta.ID)
			op.detachSrObjs = append(op.detachSrObjs, sr)
			op.stgIDsWithSR = append(op.stgIDsWithSR, s.Meta.ID)
		}
	}
}

func (op *nodeDeleteOp) launchDetachSRs(ctx context.Context) []*models.StorageRequest {
	vsrTag := fmt.Sprintf("%s:%s", common.SystemTagVsrCreator, op.rhs.Request.Meta.ID)
	fdTag := fmt.Sprintf("%s:%s", common.SystemTagForceDetachNodeID, op.rhs.Request.NodeID)
	var detachSRs []*models.StorageRequest
	for _, s := range op.stgObjs {
		if !util.Contains(op.stgIDsWithSR, s.Meta.ID) {
			detachSR := &models.StorageRequest{
				StorageRequestCreateOnce: models.StorageRequestCreateOnce{
					RequestedOperations: []string{common.StgReqOpDetach},
					CompleteByTime:      op.rhs.Request.CompleteByTime,
				},
				StorageRequestMutable: models.StorageRequestMutable{
					StorageRequestCreateMutable: models.StorageRequestCreateMutable{
						NodeID:     models.ObjIDMutable(op.rhs.Request.NodeID),
						StorageID:  models.ObjIDMutable(s.Meta.ID),
						SystemTags: []string{vsrTag, fdTag},
					},
				},
			}
			op.c.Log.Debugf("VolumeSeriesRequest %s: creating DETACH SR for Storage %s", op.rhs.Request.Meta.ID, s.Meta.ID)
			srObj, err := op.c.oCrud.StorageRequestCreate(ctx, detachSR)
			if err != nil {
				op.rhs.SetAndUpdateRequestMessageRepeated(ctx, "StorageRequest create error %s", err.Error())
				op.rhs.RetryLater = true
				continue
			}
			detachSRs = append(detachSRs, srObj)
			op.rhs.SetAndUpdateRequestMessageRepeated(ctx, "DETACH SR[%s] created for Storage[%s]", srObj.Meta.ID, s.Meta.ID)
		}
	}
	op.detachSrObjs = detachSRs
	return detachSRs
}

// Create subordinate volume series detach VSRs if ids do not exist in the map
func (op *nodeDeleteOp) launchRemainingSubordinates(ctx context.Context) {
	op.numCreated = 0
	for vsID, sub := range op.rhs.Request.SyncPeers {
		if sub.ID == "" && vsID != string(op.rhs.Request.NodeID) {
			if op.planOnly {
				op.rhs.SetAndUpdateRequestMessageRepeated(ctx, "Launch VOL_DETACH [%s]", vsID)
				continue
			}
			cA := &models.VolumeSeriesRequestCreateArgs{}
			cA.RequestedOperations = []string{common.VolReqOpVolDetach}
			cA.NodeID = op.rhs.Request.NodeID
			cA.VolumeSeriesID = models.ObjIDMutable(vsID)
			cA.SyncCoordinatorID = models.ObjIDMutable(op.rhs.Request.Meta.ID)
			cA.CompleteByTime = op.rhs.Request.CompleteByTime
			o := &models.VolumeSeriesRequest{}
			o.VolumeSeriesRequestCreateOnce = cA.VolumeSeriesRequestCreateOnce
			o.VolumeSeriesRequestCreateMutable = cA.VolumeSeriesRequestCreateMutable
			vsrObj, err := op.c.oCrud.VolumeSeriesRequestCreate(ctx, o)
			if err != nil {
				op.rhs.SetAndUpdateRequestMessageRepeated(ctx, "VolumeSeriesRequest create error %s", err.Error())
				op.rhs.RetryLater = true
				return
			}
			op.rhs.SetAndUpdateRequestMessageRepeated(ctx, "VOL_DETACH VSR[%s] created for VS[%s]", vsrObj.Meta.ID, vsID)
			op.numCreated++
		}
	}
}

func (op *nodeDeleteOp) listActiveSRs(ctx context.Context) {
	if op.srObjs != nil {
		return
	}
	lParams := storage_request.NewStorageRequestListParams()
	// Note: REATTACH to this node will fail in the handler
	lParams.NodeID = swag.String(string(op.rhs.Request.NodeID))
	lParams.IsTerminated = swag.Bool(false)
	res, err := op.c.oCrud.StorageRequestList(ctx, lParams)
	if err != nil {
		op.rhs.RetryLater = true
		return
	}
	op.srObjs = res.Payload
}

func (op *nodeDeleteOp) listConfiguredVS(ctx context.Context) {
	if op.vsObjs != nil {
		return
	}
	lParams := volume_series.NewVolumeSeriesListParams()
	lParams.ConfiguredNodeID = swag.String(string(op.rhs.Request.NodeID))
	res, err := op.c.oCrud.VolumeSeriesList(ctx, lParams)
	if err != nil {
		op.rhs.RetryLater = true
		return
	}
	op.vsObjs = res.Payload
}

func (op *nodeDeleteOp) listAndClassifyNodeVSRs(ctx context.Context) {
	op.vsrObjs, op.vsrsToCancel, op.vsrsToFail, op.vsrsToWaitOn = nil, nil, nil, nil
	lParams := volume_series_request.NewVolumeSeriesRequestListParams()
	lParams.NodeID = swag.String(string(op.rhs.Request.NodeID))
	lParams.IsTerminated = swag.Bool(false)
	lParams.RequestedOperationsNot = []string{common.VolReqOpNodeDelete} // skip self!
	res, err := op.c.oCrud.VolumeSeriesRequestList(ctx, lParams)
	if err != nil {
		op.rhs.RetryLater = true
		return
	}
	op.vsrObjs = res.Payload
	op.vsrsToCancel = make([]*models.VolumeSeriesRequest, 0, len(op.vsrObjs))
	op.vsrsToFail = make([]*models.VolumeSeriesRequest, 0, len(op.vsrObjs))
	op.vsrsToWaitOn = make([]*models.VolumeSeriesRequest, 0, len(op.vsrObjs))
	var buf bytes.Buffer
	for _, vsr := range op.vsrObjs {
		fmt.Fprintf(&buf, "- [%s] O%v S:%s CR:%v", vsr.Meta.ID, vsr.RequestedOperations, vsr.VolumeSeriesRequestState, vsr.CancelRequested)
		si := vra.GetStateInfo(vsr.VolumeSeriesRequestState)
		switch si.GetProcess(vsr.RequestedOperations[0]) {
		case vra.ApAgentd:
			fmt.Fprint(&buf, " Agentd ⇒ FAIL")
			op.vsrsToFail = append(op.vsrsToFail, vsr)
		case vra.ApCentrald:
			fallthrough
		case vra.ApClusterd:
			fmt.Fprint(&buf, " !Agentd")
			if si.IsUndo() || util.Contains(vsr.RequestedOperations, common.VolReqOpUnbind) {
				fmt.Fprint(&buf, " ⇒ WAIT")
				op.vsrsToWaitOn = append(op.vsrsToWaitOn, vsr)
			} else {
				fmt.Fprint(&buf, " ⇒ CANCEL")
				op.vsrsToCancel = append(op.vsrsToCancel, vsr)
			}
		}
		fmt.Fprint(&buf, "\n")
	}
	op.c.Log.Debugf("VolumeSeriesRequest %s: NodeVSR classification: #=%d F=%d W=%d C=%d\n%s", op.rhs.Request.Meta.ID, len(op.vsrObjs), len(op.vsrsToFail), len(op.vsrsToWaitOn), len(op.vsrsToCancel), buf.String())
	op.rhs.SetAndUpdateRequestMessageRepeated(ctx, "NodeVSR classification: #=%d F=%d W=%d C=%d\n%s", len(op.vsrObjs), len(op.vsrsToFail), len(op.vsrsToWaitOn), len(op.vsrsToCancel), buf.String())
}

// primePeerMap updates the syncPeers map in the database.
// The length of this map is 0 if there are no configured volumes.
func (op *nodeDeleteOp) primePeerMap(ctx context.Context) {
	if op.rhs.Request.SyncPeers != nil {
		op.skipSubordinateCheck = false
		return
	}
	spm := make(map[string]models.SyncPeer)
	var configuredVSIDs []string
	if len(op.vsObjs) > 0 {
		for _, vol := range op.vsObjs {
			spm[string(vol.Meta.ID)] = models.SyncPeer{}
			configuredVSIDs = append(configuredVSIDs, string(vol.Meta.ID))
		}
		spm[string(op.rhs.Request.NodeID)] = models.SyncPeer{} // node represents the coordinator itself
	}
	items := &crud.Updates{Set: []string{"syncPeers", "requestMessages"}}
	modifyFn := func(o *models.VolumeSeriesRequest) (*models.VolumeSeriesRequest, error) {
		if o == nil {
			o = op.rhs.Request
		}
		op.rhs.Request = o
		op.rhs.SetRequestMessage("Configured volumes found: %v", configuredVSIDs)
		o.SyncPeers = spm
		return o, nil
	}
	obj, err := op.c.oCrud.VolumeSeriesRequestUpdater(ctx, string(op.rhs.Request.Meta.ID), modifyFn, items)
	if err != nil {
		op.rhs.RetryLater = true
		return
	}
	op.rhs.Request = obj
	op.skipSubordinateCheck = true // there are none...
}

func (op *nodeDeleteOp) syncFail(ctx context.Context) {
	if op.advertisedFailure {
		return
	}
	op.rhs.Request.SyncCoordinatorID = op.syncID // in-memory only
	op.saa = &vra.SyncAbortArgs{
		LocalKey:   string(op.rhs.Request.NodeID),
		LocalState: common.VolReqStateFailed,
	}
	op.rhs.SyncAbort(ctx, op.saa) // ignore errors
	op.advertisedFailure = true
}

func (op *nodeDeleteOp) syncState(ctx context.Context) {
	state := string(op.rhs.Request.VolumeSeriesRequestState)
	if op.planOnly {
		op.rhs.SetAndUpdateRequestMessageRepeated(ctx, "%s sync", state)
		return
	}
	reiLabel := fmt.Sprintf("nd-block-sync-%s", state)
	if err := op.c.rei.ErrOnBool(reiLabel); err != nil {
		op.rhs.SetAndUpdateRequestMessageDistinct(ctx, "Sync: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.rhs.Request.SyncCoordinatorID = op.syncID // in-memory only
	op.sa = &vra.SyncArgs{
		LocalKey:   string(op.rhs.Request.NodeID),
		SyncState:  state,
		CompleteBy: time.Time(op.rhs.Request.CompleteByTime),
	}
	if err := op.rhs.SyncRequests(ctx, op.sa); err != nil { // only updates local record
		op.rhs.SetRequestError("Sync error %s", err.Error())
	}
}

func (op *nodeDeleteOp) teardownNode(ctx context.Context) {
	if op.planOnly {
		op.rhs.SetAndUpdateRequestMessageRepeated(ctx, "Set node state to [%s]", common.NodeStateTearDown)
		return
	}
	op.nObj.State = common.NodeStateTearDown
	items := &crud.Updates{Set: []string{"state"}}
	obj, err := op.c.oCrud.NodeUpdate(ctx, op.nObj, items)
	if err != nil {
		op.rhs.RetryLater = true
		return
	}
	op.nObj = obj
	if err := op.c.rei.ErrOnBool("teardown-fatal-error"); err != nil {
		op.rhs.SetAndUpdateRequestMessageDistinct(ctx, "TeardownNode error: %s", err.Error())
		op.rhs.InError = true
		return
	}
}

// waitForSRs waits on a list of SRs to terminate.
// It continues through the list on error and returns the list of remaining SRs
func (op *nodeDeleteOp) waitForSRs(ctx context.Context, srs []*models.StorageRequest) []*models.StorageRequest {
	var remainingSRs []*models.StorageRequest
	var err error
	for _, sr := range srs {
		op.c.Log.Debugf("VolumeSeriesRequest %s: Waiting for SR[%s]", op.rhs.Request.Meta.ID, sr.Meta.ID)
		srW := requestWaiterFactory(&vra.RequestWaiterArgs{
			SR:          sr,
			CrudeOps:    op.c.App.CrudeOps,
			ClientOps:   op.c.oCrud,
			Log:         op.c.Log,
			SrInspector: op, // fail if it enters agentd
		})
		if _, err = srW.WaitForSR(ctx); err != nil {
			op.c.Log.Errorf("VolumeSeriesRequest %s: WaitForSR[%s]: %s", op.rhs.Request.Meta.ID, sr.Meta.ID, err.Error())
			op.rhs.SetAndUpdateRequestMessageRepeated(ctx, "SR[%s] failed to complete", sr.Meta.ID)
			op.rhs.RetryLater = true
			remainingSRs = append(remainingSRs, sr)
		}
		op.rhs.SetAndUpdateRequestMessageRepeated(ctx, "SR[%s] completed", sr.Meta.ID)
	}
	return remainingSRs
}

// waitForVSRs waits on a list of VSRs to terminate or transition into an agentd state
// It continues through the list on error and returns the list of remaining VSRs
func (op *nodeDeleteOp) waitForVSRs(ctx context.Context, vsrs []*models.VolumeSeriesRequest) []*models.VolumeSeriesRequest {
	var remainingVSRs []*models.VolumeSeriesRequest
	var err error
	for _, vsr := range vsrs {
		op.c.Log.Debugf("VolumeSeriesRequest %s: Waiting for VSR[%s]", op.rhs.Request.Meta.ID, vsr.Meta.ID)
		vsrW := requestWaiterFactory(&vra.RequestWaiterArgs{
			VSR:          vsr,
			CrudeOps:     op.c.App.CrudeOps,
			ClientOps:    op.c.oCrud,
			Log:          op.c.Log,
			VsrInspector: op, // fail if it enters agentd
		})
		if _, err = vsrW.WaitForVSR(ctx); err != nil {
			op.c.Log.Errorf("VolumeSeriesRequest %s: WaitForVSR:[%s]: %s", op.rhs.Request.Meta.ID, vsr.Meta.ID, err.Error())
			op.rhs.SetAndUpdateRequestMessageRepeated(ctx, "VSR[%s] failed to complete", vsr.Meta.ID)
			op.rhs.RetryLater = true
			remainingVSRs = append(remainingVSRs, vsr)
		}
		op.rhs.SetAndUpdateRequestMessageRepeated(ctx, "VSR[%s] completed", vsr.Meta.ID)
	}
	return remainingVSRs
}

// helpers
type vraWaiterFactory func(args *vra.RequestWaiterArgs) vra.RequestWaiter

var requestWaiterFactory vraWaiterFactory = vra.NewRequestWaiter // UT intercept point

var errExecutingInAgentd = fmt.Errorf("executing in agentd")

// CanWaitOnSR satisfies the vra.RequestWaiterSRInspector interface
// It returns errExecutingInAgentd if the SR enters an agentd state
func (op *nodeDeleteOp) CanWaitOnSR(sr *models.StorageRequest) error {
	if vra.GetSRProcess(sr.StorageRequestState, sr.RequestedOperations[0]) == vra.ApAgentd {
		return errExecutingInAgentd
	}
	return nil
}

// CanWaitOnVSR satisfies the vra.RequestWaiterVSRInspector interface
// It returns errExecutingInAgentd if the VSR enters an agentd state
func (op *nodeDeleteOp) CanWaitOnVSR(vsr *models.VolumeSeriesRequest) error {
	si := vra.GetStateInfo(vsr.VolumeSeriesRequestState)
	if si.GetProcess(vsr.RequestedOperations[0]) == vra.ApAgentd {
		return errExecutingInAgentd
	}
	return nil
}
