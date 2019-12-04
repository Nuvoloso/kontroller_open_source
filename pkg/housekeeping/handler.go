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


package housekeeping

import (
	"fmt"
	"net/http"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/task"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
)

// RegisterHandlers is part of the TaskScheduler interface
func (m *Manager) RegisterHandlers(api *operations.NuvolosoAPI, accessManager AccessControl) {
	m.accessManager = accessManager
	api.TaskTaskCancelHandler = task.TaskCancelHandlerFunc(m.taskCancel)
	api.TaskTaskCreateHandler = task.TaskCreateHandlerFunc(m.taskCreate)
	api.TaskTaskFetchHandler = task.TaskFetchHandlerFunc(m.taskFetch)
	api.TaskTaskListHandler = task.TaskListHandlerFunc(m.taskList)
}

var eMissing = &models.Error{Code: http.StatusBadRequest, Message: swag.String(com.ErrorMissing)}
var eNotFound = &models.Error{Code: http.StatusNotFound, Message: swag.String(com.ErrorNotFound)}
var eForbidden = &models.Error{Code: http.StatusForbidden, Message: swag.String(com.ErrorUnauthorizedOrForbidden)}
var eNotSupported = &models.Error{Code: http.StatusConflict, Message: swag.String(com.ErrorUnsupportedOperation)}

func (m *Manager) handlerSetScope(r *http.Request, mObj *models.Task, isCreate bool) {
	sm := make(map[string]string)
	if isCreate {
		sm[crude.ScopeMetaID] = string(mObj.Meta.ID)
	}
	sm["operation"] = mObj.Operation
	sm["state"] = mObj.State
	m.CrudeOps.SetScope(r, sm, mObj)
}

func (m *Manager) taskCancel(params task.TaskCancelParams) middleware.Responder {
	subject, e := m.accessManager.GetAuth(params.HTTPRequest)
	if e != nil {
		return task.NewTaskCancelDefault(int(e.Code)).WithPayload(e)
	}
	tObj := m.findTask(params.ID)
	if tObj == nil {
		e = eNotFound
		return task.NewTaskCancelDefault(int(e.Code)).WithPayload(e)
	}
	if !m.accessManager.TaskCancelOK(subject, string(tObj.Operation)) {
		e = eForbidden
		return task.NewTaskCancelDefault(int(e.Code)).WithPayload(e)
	}
	var err error
	if tObj.State.IsTerminalState() {
		err = fmt.Errorf("terminated")
	} else {
		err = m.CancelTask(string(tObj.Meta.ID))
	}
	if err != nil {
		e = eNotFound
		return task.NewTaskCancelDefault(int(e.Code)).WithPayload(e)
	}
	mObj := tObj.Object()
	m.handlerSetScope(params.HTTPRequest, mObj, false)
	return task.NewTaskCancelOK().WithPayload(mObj)
}

func (m *Manager) taskCreate(params task.TaskCreateParams) middleware.Responder {
	subject, e := m.accessManager.GetAuth(params.HTTPRequest)
	if e != nil {
		return task.NewTaskCreateDefault(int(e.Code)).WithPayload(e)
	}
	if !m.accessManager.TaskCreateOK(subject, string(params.Payload.Operation)) {
		e = eForbidden
		return task.NewTaskCreateDefault(int(e.Code)).WithPayload(e)
	}
	mObj := &models.Task{
		TaskCreateOnce: *params.Payload,
	}
	tid, err := m.runTask(mObj, true)
	if err != nil {
		if err == ErrUnsupportedOperation {
			e = eNotSupported
		} else {
			e = &models.Error{
				Code:    eMissing.Code,
				Message: swag.String(fmt.Sprintf("%s: %s", swag.StringValue(eMissing.Message), err.Error())),
			}
		}
		return task.NewTaskCreateDefault(int(e.Code)).WithPayload(e)
	}
	tObj := m.findTask(tid)
	mObj = tObj.Object()
	m.handlerSetScope(params.HTTPRequest, mObj, true)
	return task.NewTaskCreateCreated().WithPayload(mObj)
}

func (m *Manager) taskFetch(params task.TaskFetchParams) middleware.Responder {
	subject, e := m.accessManager.GetAuth(params.HTTPRequest)
	if e != nil {
		return task.NewTaskFetchDefault(int(e.Code)).WithPayload(e)
	}
	tObj := m.findTask(params.ID)
	if tObj == nil {
		e = eNotFound
		return task.NewTaskFetchDefault(int(e.Code)).WithPayload(e)
	}
	if !m.accessManager.TaskViewOK(subject, string(tObj.Operation)) {
		e = eForbidden
		return task.NewTaskFetchDefault(int(e.Code)).WithPayload(e)
	}
	return task.NewTaskFetchOK().WithPayload(tObj.Object())
}

func (m *Manager) taskList(params task.TaskListParams) middleware.Responder {
	subject, e := m.accessManager.GetAuth(params.HTTPRequest)
	if e != nil {
		return task.NewTaskListDefault(int(e.Code)).WithPayload(e)
	}
	tl := []*models.Task{}
	for _, mObj := range m.ListTasks(swag.StringValue(params.Operation)) {
		if m.accessManager.TaskViewOK(subject, string(mObj.Operation)) {
			tl = append(tl, mObj)
		}
	}
	return task.NewTaskListOK().WithPayload(tl)
}
