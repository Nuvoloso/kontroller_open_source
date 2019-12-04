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


package csi

import (
	"context"
	"fmt"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type deleteOperators interface {
	checkForVSR(ctx context.Context) (bool, error)
	createDeleteVSR(ctx context.Context) error
	waitForVSR(ctx context.Context) error
	fetchDeletePolicy(ctx context.Context) error
}

type deleteOp struct {
	c                     *csiComp
	ops                   deleteOperators
	vsr                   *models.VolumeSeriesRequest
	vsID                  string
	vW                    vra.RequestWaiter
	dataRetentionOnDelete string
	logKeyword            string
}

func (c *csiComp) DeleteOrUnbindVolume(ctx context.Context, volumeID string, logKeyword string) error {
	if volumeID == "" {
		return status.Error(codes.InvalidArgument, "needs valid volumeID")
	}
	op := &deleteOp{
		c:          c,
		vsID:       volumeID,
		logKeyword: logKeyword,
	}
	op.ops = op // self-reference
	return op.run(ctx)
}

// DeleteVolume calls a vsr to delete the volume serires
func (c *csiComp) DeleteVolume(ctx context.Context, volumeID string) error {
	return c.DeleteOrUnbindVolume(ctx, volumeID, "CSI")
}

func (o *deleteOp) run(ctx context.Context) error {
	pendingVSR, err := o.ops.checkForVSR(ctx)
	if err != nil {
		return err
	}
	if pendingVSR {
		o.c.app.Log.Debugf("%s: VSR [%s] pending for volume-series [%s]", o.logKeyword, o.vsr.Meta.ID, o.vsID)
		return o.ops.waitForVSR(ctx)
	}
	if err = o.ops.fetchDeletePolicy(ctx); err != nil {
		return err
	}
	if err = o.ops.createDeleteVSR(ctx); err != nil {
		return err
	}
	return o.ops.waitForVSR(ctx)
}

func (o *deleteOp) checkForVSR(ctx context.Context) (bool, error) {
	o.c.app.Log.Debugf("%s: check for pending VSR of volume-series [%s]", o.logKeyword, o.vsID)
	params := &volume_series_request.VolumeSeriesRequestListParams{
		IsTerminated: swag.Bool(false),
		SystemTags:   []string{fmt.Sprintf("%s:%s", com.SystemTagVsrK8sVolDelete, o.vsID)},
	}
	vrl, err := o.c.app.OCrud.VolumeSeriesRequestList(ctx, params)
	if err != nil {
		o.c.app.Log.Errorf("%s: checkForVSR: %s", o.logKeyword, err.Error())
		return false, status.Errorf(codes.Internal, "checkForVSR: %s", err.Error())
	}
	if len(vrl.Payload) >= 1 {
		o.vsr = vrl.Payload[0]
		return true, nil
	}
	return false, nil
}

func (o *deleteOp) waitForVSR(ctx context.Context) error {
	o.c.app.Log.Debugf("%s: wait for VSR [%s] for volume-series [%s]", o.logKeyword, o.vsr.Meta.ID, o.vsID)
	var err error
	wArgs := &vra.RequestWaiterArgs{
		VSR:       o.vsr,
		CrudeOps:  o.c.app.CrudeOps,
		ClientOps: o.c.app.OCrud,
		Log:       o.c.app.Log,
	}
	if o.vW == nil { // to aid in UT
		o.vW = vra.NewRequestWaiter(wArgs)
	}
	if err == nil {
		o.vsr, err = o.vW.WaitForVSR(ctx)
	}
	if err != nil {
		o.c.app.Log.Errorf("%s: waitForVSR: %s", o.logKeyword, err.Error())
		return status.Errorf(codes.Internal, "waitForVSR: %s", err.Error())
	}
	if o.vsr.VolumeSeriesRequestState != com.VolReqStateSucceeded {
		o.c.app.Log.Errorf("%s: waitForVSR: volume-series-request [%s] %s", o.logKeyword, o.vsr.Meta.ID, o.vsr.VolumeSeriesRequestState)
		return status.Errorf(codes.Internal, "waitForVSR: volume-series-request [%s] %s", o.vsr.Meta.ID, o.vsr.VolumeSeriesRequestState)
	}
	return nil
}

func (o *deleteOp) fetchDeletePolicy(ctx context.Context) error {
	if vs, err := o.c.app.OCrud.VolumeSeriesFetch(ctx, o.vsID); err == nil {
		if cg, err := o.c.app.OCrud.ConsistencyGroupFetch(ctx, string(vs.ConsistencyGroupID)); err == nil {
			o.dataRetentionOnDelete = cg.SnapshotManagementPolicy.VolumeDataRetentionOnDelete
		} else {
			o.c.app.Log.Errorf("%s: fetchDeletePolicy: vs[%s] cg[%s] %s", o.logKeyword, o.vsID, vs.ConsistencyGroupID, err.Error())
			return status.Errorf(codes.Internal, "fetchDeletePolicy: %s", err.Error())
		}
	} else {
		o.c.app.Log.Errorf("%s: fetchDeletePolicy: %s", o.logKeyword, err.Error())
		return status.Errorf(codes.Internal, "fetchDeletePolicy: vs[%s] %s", o.vsID, err.Error())
	}
	return nil
}

func (o *deleteOp) createDeleteVSR(ctx context.Context) error {
	vsrOp := com.VolReqOpUnbind
	if o.dataRetentionOnDelete == com.VolumeDataRetentionOnDeleteDelete {
		vsrOp = com.VolReqOpDelete
	}
	o.c.app.Log.Debugf("%s: delete volume-series [%s] with data retention policy (%s)", o.logKeyword, o.vsID, o.dataRetentionOnDelete)
	o.vsr = &models.VolumeSeriesRequest{
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			RequestedOperations: []string{vsrOp},
			CompleteByTime:      strfmt.DateTime(time.Now().Add(o.c.Args.VSRCompleteByPeriod)),
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				VolumeSeriesID: models.ObjIDMutable(o.vsID),
				SystemTags:     models.ObjTags([]string{fmt.Sprintf("%s:%s", com.SystemTagVsrK8sVolDelete, o.vsID)}),
			},
		},
	}
	vsr, err := o.c.app.OCrud.VolumeSeriesRequestCreate(ctx, o.vsr)
	if err != nil {
		o.c.app.Log.Errorf("%s: createDeleteVSR: %s", o.logKeyword, err.Error())
		if err.Error() == com.ErrorFinalSnapNeeded {
			return status.Errorf(codes.FailedPrecondition, "createDeleteVSR: operation delayed until final snapshot completed")
		}
		return status.Errorf(codes.Internal, "createDeleteVSR: %s", err.Error())
	}
	o.vsr = vsr
	return nil
}
