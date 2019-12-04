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
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/auth"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/task"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	logging "github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func TestRegisterHandlers(t *testing.T) {
	assert := assert.New(t)

	api := &operations.NuvolosoAPI{}
	assert.Nil(api.TaskTaskCancelHandler)
	assert.Nil(api.TaskTaskCreateHandler)
	assert.Nil(api.TaskTaskFetchHandler)
	assert.Nil(api.TaskTaskListHandler)
	fa := &FakeAccessControl{}

	m := &Manager{}
	m.RegisterHandlers(api, fa)
	assert.NotNil(api.TaskTaskCancelHandler)
	assert.NotNil(api.TaskTaskCreateHandler)
	assert.NotNil(api.TaskTaskFetchHandler)
	assert.NotNil(api.TaskTaskListHandler)

	assert.Equal(fa, m.accessManager)
}

func testSetupMgr(t *testing.T, log *logging.Logger) (*Manager, *Task, *taskTester) {
	assert := assert.New(t)
	evM := fev.NewFakeEventManager()
	ma := &ManagerArgs{
		Log:      log,
		CrudeOps: evM,
	}
	ts, err := NewManager(ma)
	assert.NoError(err)
	m, ok := ts.(*Manager)
	assert.True(ok)
	assert.NotNil(m)

	tA := &taskTester{}
	m.RegisterAnimator("OP0", tA)
	tIn := &models.Task{
		TaskCreateOnce: models.TaskCreateOnce{
			Operation: "OP0",
		},
	}
	m.mux.Lock()
	tObj, err := m.newTask(tIn)
	m.mux.Unlock()
	assert.NoError(err)
	assert.NotNil(tObj)
	return m, tObj, tA
}

func TestTaskCancel(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	m, tObj, _ := testSetupMgr(t, tl.Logger())
	fa := &FakeAccessControl{}
	m.accessManager = fa
	evM := fev.NewFakeEventManager()
	m.CrudeOps = evM

	params := task.TaskCancelParams{
		HTTPRequest: &http.Request{},
	}

	// GetAuth failure
	fa.RetGaErr = eForbidden
	res := m.taskCancel(params)
	tRes, ok := res.(*task.TaskCancelDefault)
	assert.True(ok)
	assert.NotNil(tRes)
	assert.Equal(eForbidden, tRes.Payload)
	assert.Equal(fa.InGaReq, params.HTTPRequest)

	// Task not found
	fa.RetGaErr = nil
	fa.RetGaAuth = &FakeSubject{}
	params.ID = "NOT-FOUND"
	res = m.taskCancel(params)
	tRes, ok = res.(*task.TaskCancelDefault)
	assert.True(ok)
	assert.NotNil(tRes)
	assert.Equal(eNotFound, tRes.Payload)

	// forbidden
	fa.RetTcn = false
	params.ID = string(tObj.Meta.ID)
	res = m.taskCancel(params)
	tRes, ok = res.(*task.TaskCancelDefault)
	assert.True(ok)
	assert.NotNil(tRes)
	assert.Equal(eForbidden, tRes.Payload)
	assert.Equal(fa.RetGaAuth, fa.InTcnS)
	assert.Equal(tObj.Operation, fa.InTcnOp)

	// task is terminated
	fa.RetTcn = true
	tObj.State = TaskStateSucceeded
	res = m.taskCancel(params)
	tRes, ok = res.(*task.TaskCancelDefault)
	assert.True(ok)
	assert.NotNil(tRes)
	assert.Equal(eNotFound, tRes.Payload)

	// cancel succeeded
	evM.InSSProps = nil
	evM.InACScope = nil
	tObj.State = TaskStateActive
	res = m.taskCancel(params)
	tOk, ok := res.(*task.TaskCancelOK)
	assert.True(ok)
	assert.NotNil(tOk)
	assert.Equal(tObj.Object(), tOk.Payload)
	assert.NotNil(evM.InSSProps)
	assert.NotNil(evM.InACScope)
	mObj := tObj.Object()
	assert.Equal(mObj.Operation, evM.InSSProps["operation"])
	assert.Equal(mObj.State, evM.InSSProps["state"])
	scopeObj, ok := evM.InACScope.(*models.Task)
	assert.True(ok)
	assert.Equal(mObj, scopeObj)

	// task expired under us so cancel fails
	fa.TcnDeleteTask = tObj
	res = m.taskCancel(params)
	tRes, ok = res.(*task.TaskCancelDefault)
	assert.True(ok)
	assert.NotNil(tRes)
	assert.Equal(eNotFound, tRes.Payload)
}

func TestTaskCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	m, tObj, tA := testSetupMgr(t, tl.Logger())
	fa := &FakeAccessControl{}
	m.accessManager = fa
	evM := fev.NewFakeEventManager()
	m.CrudeOps = evM

	params := task.TaskCreateParams{
		HTTPRequest: &http.Request{},
	}

	// GetAuth failure
	fa.RetGaErr = eForbidden
	res := m.taskCreate(params)
	tRes, ok := res.(*task.TaskCreateDefault)
	assert.True(ok)
	assert.NotNil(tRes)
	assert.Equal(eForbidden, tRes.Payload)
	assert.Equal(fa.InGaReq, params.HTTPRequest)

	// forbidden
	fa.RetGaErr = nil
	fa.RetGaAuth = &FakeSubject{}
	fa.RetTc = false
	params.Payload = &models.TaskCreateOnce{
		Operation: "OP",
	}
	res = m.taskCreate(params)
	tRes, ok = res.(*task.TaskCreateDefault)
	assert.True(ok)
	assert.NotNil(tRes)
	assert.Equal(eForbidden, tRes.Payload)
	assert.Equal(fa.RetGaAuth, fa.InTcS)
	assert.Equal(params.Payload.Operation, fa.InTcOp)

	// unsupported operation
	fa.RetTc = true
	res = m.taskCreate(params)
	tRes, ok = res.(*task.TaskCreateDefault)
	assert.True(ok)
	assert.NotNil(tRes)
	assert.Equal(eNotSupported, tRes.Payload)

	// missing error
	params.Payload.Operation = tObj.Operation
	tA.RetTv = fmt.Errorf("objectId")
	res = m.taskCreate(params)
	tRes, ok = res.(*task.TaskCreateDefault)
	assert.True(ok)
	assert.NotNil(tRes)
	assert.Equal(eMissing.Code, tRes.Payload.Code)
	assert.Regexp(*eMissing.Message, *tRes.Payload.Message)
	assert.Regexp("objectId", *tRes.Payload.Message)

	// success
	evM.InSSProps = nil
	evM.InACScope = nil
	evM.CalledIEnum = 0
	tA.RetTv = nil
	res = m.taskCreate(params)
	tOk, ok := res.(*task.TaskCreateCreated)
	assert.True(ok)
	assert.NotNil(tOk)
	assert.NotNil(tOk.Payload)
	tNew := m.findTask(string(tOk.Payload.Meta.ID))
	assert.NotNil(tNew)
	assert.NotNil(evM.InSSProps)
	assert.NotNil(evM.InACScope)
	mObj := tNew.Object()
	assert.Equal(mObj.Operation, evM.InSSProps["operation"])
	assert.Equal(mObj.State, evM.InSSProps["state"])
	assert.EqualValues(mObj.Meta.ID, evM.InSSProps[crude.ScopeMetaID])
	scopeObj, ok := evM.InACScope.(*models.Task)
	assert.True(ok)
	assert.Equal(mObj, scopeObj)
	for evM.CalledIEnum == 0 { // wait until the task starts
		time.Sleep(time.Millisecond)
	}
	assert.NotEqual("POST", evM.InjectFirstEvent.Method) // manager will not inject CRUD event
}

func TestTaskFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	m, tObj, _ := testSetupMgr(t, tl.Logger())
	fa := &FakeAccessControl{}
	m.accessManager = fa

	params := task.TaskFetchParams{
		HTTPRequest: &http.Request{},
	}

	// GetAuth failure
	fa.RetGaErr = eForbidden
	res := m.taskFetch(params)
	tRes, ok := res.(*task.TaskFetchDefault)
	assert.True(ok)
	assert.NotNil(tRes)
	assert.Equal(eForbidden, tRes.Payload)
	assert.Equal(fa.InGaReq, params.HTTPRequest)

	// Task not found
	fa.RetGaErr = nil
	fa.RetGaAuth = &FakeSubject{}
	params.ID = "NOT-FOUND"
	res = m.taskFetch(params)
	tRes, ok = res.(*task.TaskFetchDefault)
	assert.True(ok)
	assert.NotNil(tRes)
	assert.Equal(eNotFound, tRes.Payload)

	// forbidden
	fa.RetTv = false
	params.ID = string(tObj.Meta.ID)
	res = m.taskFetch(params)
	tRes, ok = res.(*task.TaskFetchDefault)
	assert.True(ok)
	assert.NotNil(tRes)
	assert.Equal(eForbidden, tRes.Payload)
	assert.Equal(fa.RetGaAuth, fa.InTvS)
	assert.Equal(tObj.Operation, fa.InTvOp)

	// success
	fa.RetTv = true
	res = m.taskFetch(params)
	tOk, ok := res.(*task.TaskFetchOK)
	assert.True(ok)
	assert.NotNil(tOk)
	assert.NotNil(tOk.Payload)
	assert.Equal(tObj.Object(), tOk.Payload)
}

func TestTaskList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	m, _, _ := testSetupMgr(t, tl.Logger())
	fa := &FakeAccessControl{}
	m.accessManager = fa

	tA1 := &taskTester{}
	m.RegisterAnimator("OP1", tA1)

	moreTasks := []*models.Task{
		&models.Task{TaskCreateOnce: models.TaskCreateOnce{Operation: "OP0"}},
		&models.Task{TaskCreateOnce: models.TaskCreateOnce{Operation: "OP1"}},
		&models.Task{TaskCreateOnce: models.TaskCreateOnce{Operation: "OP1"}},
		&models.Task{TaskCreateOnce: models.TaskCreateOnce{Operation: "OP1"}},
	}
	for i, aTask := range moreTasks {
		m.mux.Lock()
		obj, err := m.newTask(aTask)
		m.mux.Unlock()
		assert.NoError(err, "%d", i)
		assert.NotNil(obj, "%d", i)
	}

	params := task.TaskListParams{
		HTTPRequest: &http.Request{},
	}

	// GetAuth failure
	fa.RetGaErr = eForbidden
	res := m.taskList(params)
	tRes, ok := res.(*task.TaskListDefault)
	assert.True(ok)
	assert.NotNil(tRes)
	assert.Equal(eForbidden, tRes.Payload)
	assert.Equal(fa.InGaReq, params.HTTPRequest)

	// all forbidden
	fa.RetGaErr = nil
	fa.RetGaAuth = &FakeSubject{}
	fa.RetTv = false
	res = m.taskList(params)
	tOk, ok := res.(*task.TaskListOK)
	assert.True(ok)
	assert.NotNil(tOk)
	assert.NotNil(tOk.Payload)
	assert.Empty(tOk.Payload)

	// all permitted
	fa.RetTv = true
	res = m.taskList(params)
	tOk, ok = res.(*task.TaskListOK)
	assert.True(ok)
	assert.NotNil(tOk)
	assert.NotNil(tOk.Payload)
	assert.Len(tOk.Payload, len(moreTasks)+1)

	// permit only OP0
	fa.TvPermitOp = "OP0"
	res = m.taskList(params)
	tOk, ok = res.(*task.TaskListOK)
	assert.True(ok)
	assert.NotNil(tOk)
	assert.NotNil(tOk.Payload)
	assert.Len(tOk.Payload, 2)
	for _, mObj := range tOk.Payload {
		assert.Equal("OP0", mObj.Operation)
	}

	// filter by operation
	fa.TvPermitOp = ""
	params.Operation = swag.String("OP1")
	res = m.taskList(params)
	tOk, ok = res.(*task.TaskListOK)
	assert.True(ok)
	assert.NotNil(tOk)
	assert.NotNil(tOk.Payload)
	assert.Len(tOk.Payload, 3)
	for _, mObj := range tOk.Payload {
		assert.Equal("OP1", mObj.Operation)
	}
}

// FakeAccessControl is a fake housekeeping access control manager
type FakeAccessControl struct {
	// GetAuth
	InGaReq   *http.Request
	RetGaAuth auth.Subject
	RetGaErr  *models.Error

	// TaskCancelOK
	InTcnS        auth.Subject
	InTcnOp       string
	RetTcn        bool
	TcnDeleteTask *Task

	// TaskCreateOK
	InTcS  auth.Subject
	InTcOp string
	RetTc  bool

	// TaskViewOK
	InTvS      auth.Subject
	InTvOp     string
	RetTv      bool
	TvPermitOp string
}

var _ = AccessControl(&FakeAccessControl{})

// GetAuth fakes its namesake
func (fa *FakeAccessControl) GetAuth(req *http.Request) (auth.Subject, *models.Error) {
	fa.InGaReq = req
	return fa.RetGaAuth, fa.RetGaErr
}

// TaskCancelOK fakes its namesake
func (fa *FakeAccessControl) TaskCancelOK(subject auth.Subject, op string) bool {
	fa.InTcnS = subject
	fa.InTcnOp = op
	if fa.TcnDeleteTask != nil {
		fa.TcnDeleteTask.M.deleteTask(fa.TcnDeleteTask)
		fa.TcnDeleteTask = nil
	}
	return fa.RetTcn
}

// TaskCreateOK fakes its namesake
func (fa *FakeAccessControl) TaskCreateOK(subject auth.Subject, op string) bool {
	fa.InTcS = subject
	fa.InTcOp = op
	return fa.RetTc
}

// TaskViewOK fakes its namesake
func (fa *FakeAccessControl) TaskViewOK(subject auth.Subject, op string) bool {
	fa.InTvS = subject
	fa.InTvOp = op
	if fa.TvPermitOp != "" {
		return op == fa.TvPermitOp
	}
	return fa.RetTv
}

type FakeSubject struct{}

var _ = auth.Subject(&FakeSubject{})

func (fs *FakeSubject) Internal() bool {
	return false
}

func (fs *FakeSubject) GetAccountID() string {
	return ""
}

func (fs *FakeSubject) String() string {
	return ""
}
