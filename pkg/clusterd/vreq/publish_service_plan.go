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
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/swag"
)

type pspSubState int

// pspSubState Values (the ordering is meaningful)
const (
	// Load the SPA object to determine the service plan
	PspLoadSpa pspSubState = iota

	// Load the ServicePlan object
	PspLoadServicePlan

	// Call the cluster API to publish service plan
	PspPublishToCluster

	// update the SPA object with the cluster descriptor
	PspUpdateSpa

	// Terminal state.
	PspDone

	// No undo

	// LAST: No operation is performed in this state
	PspNoOp
)

func (ss pspSubState) String() string {
	switch ss {
	case PspLoadSpa:
		return "PspLoadSpa"
	case PspLoadServicePlan:
		return "PspLoadServicePlan"
	case PspPublishToCluster:
		return "PspPublishToCluster"
	case PspUpdateSpa:
		return "PspUpdateSpa"
	case PspDone:
		return "PspDone"
	}
	return fmt.Sprintf("pspSubState(%d)", ss)
}

type pspOperators interface {
	getInitialState(ctx context.Context) pspSubState
	loadSP(ctx context.Context)
	loadSPA(ctx context.Context)
	publishToCluster(ctx context.Context)
	updateSPA(ctx context.Context)
}

type pspOp struct {
	c        *Component
	rhs      *vra.RequestHandlerState
	ops      pspOperators
	planOnly bool
	spa      *models.ServicePlanAllocation
	sp       *models.ServicePlan
	cd       models.ClusterDescriptor
}

// PublishServicePlan satisfies the vra.PublishServicePlanHandlers interface
func (c *Component) PublishServicePlan(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &pspOp{
		c:   c,
		rhs: rhs,
	}
	op.ops = op // self-reference
	op.run(ctx)
}

func (op *pspOp) run(ctx context.Context) {
out:
	for ss := op.ops.getInitialState(ctx); !(op.rhs.InError || op.rhs.RetryLater); ss++ {
		op.c.Log.Debugf("VolumeSeriesRequest %s: PUBLISHING_SERVICE_PLAN %s", op.rhs.Request.Meta.ID, ss)
		switch ss {
		case PspLoadSpa:
			op.ops.loadSPA(ctx)
		case PspLoadServicePlan:
			op.ops.loadSP(ctx)
		case PspPublishToCluster:
			op.ops.publishToCluster(ctx)
		case PspUpdateSpa:
			op.ops.updateSPA(ctx)
		default:
			break out
		}
	}
}

func (op *pspOp) getInitialState(ctx context.Context) pspSubState {
	if op.c.App.CSISocket == "" {
		return PspDone // no-op if CSI not configured
	}
	if err := op.c.rei.ErrOnBool("publish-sp-fail"); err != nil {
		op.rhs.SetRequestError("Fatal error: %s", err.Error())
		return PspDone
	}
	op.planOnly = swag.BoolValue(op.rhs.Request.PlanOnly)
	if op.rhs.Request.ServicePlanAllocationID == "" {
		op.rhs.SetRequestError("Missing spaAllocationId")
		return PspDone
	}
	return PspLoadSpa
}

func (op *pspOp) loadSP(ctx context.Context) {
	sp, err := op.c.oCrud.ServicePlanFetch(ctx, string(op.spa.ServicePlanID))
	if err != nil {
		op.rhs.SetRequestMessage("service plan fetch error: %s", err.Error())
		op.rhs.RetryLater = true
	}
	op.sp = sp
}

func (op *pspOp) loadSPA(ctx context.Context) {
	spa, err := op.c.oCrud.ServicePlanAllocationFetch(ctx, string(op.rhs.Request.ServicePlanAllocationID))
	if err != nil {
		op.rhs.SetRequestMessage("service plan allocation fetch error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.spa = spa
}

func (op *pspOp) publishToCluster(ctx context.Context) {
	if op.planOnly {
		op.rhs.SetRequestMessage("Publish ServicePlan")
		return
	}
	cd, err := op.c.App.ClusterClient.ServicePlanPublish(ctx, op.sp)
	if err != nil {
		op.rhs.SetRequestMessage("ServicePlanPublish error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.cd = cd
}

func (op *pspOp) updateSPA(ctx context.Context) {
	if op.planOnly {
		op.rhs.SetRequestMessage("Update clusterDescriptor in SPA")
		return
	}
	modifyFn := func(o *models.ServicePlanAllocation) (*models.ServicePlanAllocation, error) {
		if o == nil {
			o = op.spa
		}
		o.ClusterDescriptor = op.cd
		return o, nil
	}
	items := &crud.Updates{Append: []string{"clusterDescriptor"}}
	obj, err := op.c.oCrud.ServicePlanAllocationUpdater(ctx, string(op.spa.Meta.ID), modifyFn, items)
	if err != nil {
		op.rhs.SetRequestMessage("service plan allocation update error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	op.spa = obj
}
