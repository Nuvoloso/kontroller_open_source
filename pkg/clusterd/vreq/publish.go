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

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/swag"
)

type publishSubState int

// publishSubState Values (the ordering is meaningful)
const (
	// Load the ServicePlan
	LoadServicePlan publishSubState = iota

	// Call the cluster API to publish VS
	PublishDoPublish

	// Set the systemTags
	PublishVsPublish

	// Terminal state during publish. Must precede any undo state
	PublishDone

	// Call the cluster API to unPublish a VS
	PublishUndoPublish

	// Delete the systemTags
	PublishUndoVsUnpublish

	// Undo completion
	PublishUndoDone

	// LAST: No operation is performed in this state
	PublishNoOp
)

func (ss publishSubState) String() string {
	switch ss {
	case LoadServicePlan:
		return "LoadServicePlan"
	case PublishDoPublish:
		return "PublishDoPublish"
	case PublishVsPublish:
		return "PublishVsPublish"
	case PublishDone:
		return "PublishDone"
	case PublishUndoPublish:
		return "PublishUndoPublish"
	case PublishUndoVsUnpublish:
		return "PublishUndoVsUnpublish"
	case PublishUndoDone:
		return "PublishUndoDone"
	}
	return fmt.Sprintf("publishSubState(%d)", ss)
}

type publishOperators interface {
	getInitialState(ctx context.Context) publishSubState
	isPublished(ctx context.Context) bool
	loadServicePlan(ctx context.Context)
	vsPublish(ctx context.Context)
	vsUnpublish(ctx context.Context)
	publish(ctx context.Context)
	unPublish(ctx context.Context)
}

type publishOp struct {
	c         *Component
	rhs       *vra.RequestHandlerState
	ops       publishOperators
	cd        models.ClusterDescriptor
	sp        *models.ServicePlan
	inError   bool
	published bool
}

// Publish satisfies the vra.PublishHandlers interface
func (c *Component) Publish(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &publishOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// UndoPublish satisfies the vra.PublishHandlers interface
func (c *Component) UndoPublish(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &publishOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// run executes the state machine
func (op *publishOp) run(ctx context.Context) {
out:
	for ss := op.ops.getInitialState(ctx); !(op.rhs.InError || op.rhs.RetryLater); ss++ {
		op.c.Log.Debugf("VolumeSeriesRequest %s: PUBLISH %s", op.rhs.Request.Meta.ID, ss)
		switch ss {
		case LoadServicePlan:
			op.ops.loadServicePlan(ctx)
		case PublishDoPublish:
			op.ops.publish(ctx)
		case PublishVsPublish:
			op.ops.vsPublish(ctx)
		case PublishUndoPublish:
			op.ops.unPublish(ctx)
		case PublishUndoVsUnpublish:
			op.ops.vsUnpublish(ctx)
		default:
			break out
		}
	}
	if op.inError {
		op.rhs.InError = true
	}
}

func (op *publishOp) getInitialState(ctx context.Context) publishSubState {
	if op.rhs.Request.VolumeSeriesRequestState == com.VolReqStateUndoPublishing {
		if !op.published && !op.ops.isPublished(ctx) {
			return PublishUndoDone
		}
		op.inError = op.rhs.InError
		op.rhs.InError = false // to enable cleanup
		return PublishUndoPublish
	}
	if op.rhs.Canceling || op.rhs.InError {
		return PublishDone
	}
	return LoadServicePlan
}

func (op *publishOp) loadServicePlan(ctx context.Context) {
	sp, err := op.c.oCrud.ServicePlanFetch(ctx, string(op.rhs.VolumeSeries.ServicePlanID))
	if err != nil {
		op.rhs.SetRequestMessage("service plan fetch error: %s", err.Error())
		op.rhs.RetryLater = true
	}
	op.sp = sp
}

func (op *publishOp) unPublish(ctx context.Context) {
	pvda := &cluster.PersistentVolumeDeleteArgs{
		VolumeID: string(op.rhs.VolumeSeries.Meta.ID),
	}
	if _, err := op.c.App.ClusterClient.PersistentVolumeDelete(ctx, pvda); err != nil {
		if e, ok := err.(cluster.Error); ok {
			if e.NotFound() {
				return
			}
			if e.PVIsBound() {
				op.rhs.SetAndUpdateRequestMessage(ctx, "VolumeSeriesRequest UnPublishing error: %s", err.Error())
				op.rhs.InError = true
				return
			}
		}
		op.c.Log.Debugf("VolumeSeriesRequest %s: UnPublishing error: %s", op.rhs.Request.Meta.ID, err.Error())
		op.rhs.RetryLater = true
		return
	}
}

func (op *publishOp) publish(ctx context.Context) {
	pvca := &cluster.PersistentVolumeCreateArgs{
		VolumeID:        string(op.rhs.VolumeSeries.Meta.ID),
		SizeBytes:       swag.Int64Value(op.rhs.VolumeSeries.SizeBytes),
		FsType:          op.rhs.Request.FsType,
		AccountID:       string(op.rhs.VolumeSeries.AccountID),
		SystemID:        op.c.App.AppArgs.SystemID,
		DriverType:      op.rhs.Request.DriverType,
		ServicePlanName: string(op.sp.Name),
	}
	if pvca.DriverType == "" {
		if op.c.App.CSISocket != "" {
			pvca.DriverType = com.K8sDriverTypeCSI
		}
	}
	if cd, err := op.c.App.ClusterClient.PersistentVolumePublish(ctx, pvca); err != nil {
		op.rhs.SetAndUpdateRequestMessage(ctx, "VolumeSeriesRequest Publishing error: %s", err.Error())
		op.rhs.InError = true
	} else {
		op.cd = cd
		op.published = true
	}
}

func (op *publishOp) isPublished(ctx context.Context) bool {
	return len(op.rhs.VolumeSeries.ClusterDescriptor) != 0
}

func (op *publishOp) vsPublish(ctx context.Context) {
	items := &crud.Updates{
		Set:    []string{"clusterDescriptor"},
		Append: []string{"systemTags"},
	}
	setVS := func(vs *models.VolumeSeries) (*models.VolumeSeries, error) {
		if vs == nil {
			vs = op.rhs.VolumeSeries
		}
		vs.ClusterDescriptor = op.cd
		vs.SystemTags = []string{com.SystemTagVolumePublished}
		return vs, nil
	}
	vs, err := op.c.oCrud.VolumeSeriesUpdater(ctx, string(op.rhs.VolumeSeries.Meta.ID), setVS, items)
	if err != nil {
		op.rhs.SetRequestMessage("Failed to update VolumeSeries object: %s", err.Error())
		op.rhs.InError = true
		return
	}
	op.rhs.VolumeSeries = vs
}

func (op *publishOp) vsUnpublish(ctx context.Context) {
	items := &crud.Updates{
		Set:    []string{"clusterDescriptor"},
		Remove: []string{"systemTags"},
	}
	delVS := func(vs *models.VolumeSeries) (*models.VolumeSeries, error) {
		if vs == nil {
			vs = op.rhs.VolumeSeries
		}
		vs.ClusterDescriptor = nil
		vs.SystemTags = []string{com.SystemTagVolumePublished}
		return vs, nil
	}
	vs, err := op.c.oCrud.VolumeSeriesUpdater(ctx, string(op.rhs.VolumeSeries.Meta.ID), delVS, items)
	if err != nil {
		op.rhs.SetRequestMessage("Failed to update VolumeSeries object: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.rhs.VolumeSeries = vs
}
