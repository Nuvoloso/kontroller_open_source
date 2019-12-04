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
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	hk "github.com/Nuvoloso/kontroller/pkg/housekeeping"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
)

// vsrPurgeTask is responsible for purging VSRs
type vsrPurgeTask struct {
	c *Component
}

func vptRegisterAnimator(c *Component) *vsrPurgeTask {
	vpt := &vsrPurgeTask{
		c: c,
	}
	c.App.TaskScheduler.RegisterAnimator(com.TaskVsrPurge, vpt)
	return vpt
}

func (vpt *vsrPurgeTask) TaskValidate(createArgs *models.TaskCreateOnce) error {
	if createArgs.Operation != com.TaskVsrPurge {
		return hk.ErrInvalidAnimator
	}
	return nil
}

func (vpt *vsrPurgeTask) TaskExec(ctx context.Context, tOps hk.TaskOps) {
	c := vpt.c
	app := c.App
	o := tOps.Object()
	c.Log.Debugf("Starting task %s", com.TaskVsrPurge)
	msgs := util.NewMsgList(o.Messages)
	if app.CrudHelpers == nil {
		c.Log.Errorf("Task %s: not ready", o.Meta.ID)
		msgs.Insert("Not ready")
		o.Messages = msgs.ToModel()
		tOps.SetState(hk.TaskStateFailed) // does not return
	}

	tOps.SetProgress(&models.Progress{PercentComplete: swag.Int32(0)})
	lParams := ops.VolumeSeriesRequestListParams{
		IsTerminated: swag.Bool(true),
	}
	vsrList, err := app.DS.OpsVolumeSeriesRequest().List(ctx, lParams)
	if err != nil {
		c.Log.Debugf("Failure to list volume series requests: %s, will retry on a next cycle", err.Error())
		msgs.Insert("Failure to list volume series requests")
		o.Messages = msgs.ToModel()
		return
	}
	c.Log.Debugf("Found %d volume series requests to process", len(vsrList))
	vsrToPurge := make([]*models.VolumeSeriesRequest, 0, len(vsrList))
	for _, vsr := range vsrList {
		expTime := time.Now().Add(-c.VSRRetentionPeriod)
		if (time.Time(vsr.Meta.TimeModified)).Before(expTime) {
			vsrToPurge = append(vsrToPurge, vsr)
		}
	}
	c.Log.Debugf("Found %d volume series requests to be purged", len(vsrToPurge))
	numD := 0
	for idx, vsr := range vsrToPurge {
		c.Log.Debugf("Purging volume series request: %s (operations: %s)", vsr.Meta.ID, vsr.RequestedOperations)
		err = app.DS.OpsVolumeSeriesRequest().Delete(ctx, string(vsr.Meta.ID))
		if err != nil {
			c.Log.Debugf("Failure to delete volume series request [id: %s]: %s, will retry on a next cycle", string(vsr.Meta.ID), err.Error())
		} else {
			numD++
		}
		tOps.SetProgress(&models.Progress{PercentComplete: swag.Int32(int32((idx + 1) / len(vsrToPurge) * 100))})
	}
	c.Log.Debugf("Deleted %d/%d volume series requests objects", numD, len(vsrToPurge))
	msgs.Insert("Deleted %d/%d volume series requests objects", numD, len(vsrToPurge))
	o.Messages = msgs.ToModel()
}
