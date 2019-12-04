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


package audit

import (
	"context"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	hk "github.com/Nuvoloso/kontroller/pkg/housekeeping"
	"github.com/Nuvoloso/kontroller/pkg/util"
)

// expiredRecordsPurgeTask is responsible for purging expired audit log records
type expiredRecordsPurgeTask struct {
	c *Component
}

func purgeTaskRegisterAnimator(c *Component) *expiredRecordsPurgeTask {
	arpt := &expiredRecordsPurgeTask{
		c: c,
	}
	c.App.TaskScheduler.RegisterAnimator(com.TaskAuditRecordsPurge, arpt)
	return arpt
}

func (arpt *expiredRecordsPurgeTask) TaskValidate(createArgs *models.TaskCreateOnce) error {
	if createArgs.Operation != com.TaskAuditRecordsPurge {
		return hk.ErrInvalidAnimator
	}
	return nil
}

func (arpt *expiredRecordsPurgeTask) TaskExec(ctx context.Context, tOps hk.TaskOps) {
	c := arpt.c
	app := c.App
	o := tOps.Object()
	c.Log.Debugf("Starting task %s", com.TaskAuditRecordsPurge)
	msgs := util.NewMsgList(o.Messages)
	// establish an existential lock
	if app.CrudHelpers == nil {
		c.Log.Errorf("Task %s: not ready", o.Meta.ID)
		msgs.Insert("Not ready")
		o.Messages = msgs.ToModel()
		tOps.SetState(hk.TaskStateFailed) // does not return
	}
	app.CrudHelpers.Lock()
	defer app.CrudHelpers.Unlock()

	c.Log.Debugf("Purging audit log records")
	if err := c.App.AuditLog.Expire(ctx, time.Now()); err != nil {
		c.Log.Debugf("Failure to expire audit log records: %s, will retry on a next cycle", err.Error())
		msgs.Insert("Failure to expire audit log records")
		o.Messages = msgs.ToModel()
	}
}
