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
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestManagerArgsValidate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	evM := fev.NewFakeEventManager()

	tcs := []ManagerArgs{
		{}, // [0]
		{
			PurgeDelay: -1,
		},
		{
			PurgeDelay: 90 * time.Second,
			Log:        tl.Logger(),
		},
		{
			CrudeOps: evM,
		},
	}
	for i, tc := range tcs {
		assert.Error(tc.Validate(), "[%d]", i)
	}

	ma := &ManagerArgs{
		Log:      tl.Logger(),
		CrudeOps: evM,
	}
	assert.NoError(ma.Validate())
	assert.Equal(DefaultPurgeDelay, ma.PurgeDelay)
}

func TestManager(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	evM := fev.NewFakeEventManager()

	// NewManager
	ts, err := NewManager(&ManagerArgs{})
	assert.Error(err)
	assert.Nil(ts)

	ma := &ManagerArgs{
		Log:      tl.Logger(),
		CrudeOps: evM,
	}
	ts, err = NewManager(ma)
	assert.NoError(err)
	assert.NotNil(ts)
	m, ok := ts.(*Manager)
	assert.True(ok)
	assert.NotNil(m.tasks)
	assert.NotNil(m.cancelFn)
	assert.NotNil(m.registry)

	// RegisterAnimator
	assert.Empty(m.registry)
	tA1 := &taskTester{}
	tA2 := &taskTester{}
	ts.RegisterAnimator("OP1", tA1)
	assert.Len(m.registry, 1)
	ts.RegisterAnimator("OP2", tA2)
	assert.Len(m.registry, 2)
	a, err := m.opAnimator("OP1")
	assert.NoError(err)
	assert.Equal(tA1, a)
	a, err = m.opAnimator("OP2")
	assert.NoError(err)
	assert.Equal(tA2, a)
	a, err = m.opAnimator("OP3")
	assert.Error(err)
	assert.Nil(a)

	// ListTasks
	makeTask := func(op string, ordinal int) *Task {
		id := fmt.Sprintf("%d", ordinal)
		t := &Task{
			Obj:       &models.Task{},
			Meta:      models.ObjMeta{ID: models.ObjID(id)},
			Operation: op,
			Ordinal:   ordinal,
		}
		m.trackTask(t)
		return t
	}
	op1t1 := makeTask("op1", 1)
	op1t2 := makeTask("op1", 2)
	op2t1 := makeTask("op2", 3)
	assert.Len(m.tasks, 3)

	task, found := m.tasks["1"]
	assert.True(found)
	assert.Equal(op1t1, task)
	assert.Equal(op1t1, m.findTask("1"))
	task, found = m.tasks["2"]
	assert.True(found)
	assert.Equal(op1t2, task)
	assert.Equal(op1t2, m.findTask("2"))
	task, found = m.tasks["3"]
	assert.True(found)
	assert.Equal(op2t1, task)
	assert.Equal(op2t1, m.findTask("3"))

	tList := m.ListTasks("")
	assert.NotNil(tList)
	assert.Len(tList, 3)
	assert.Contains(tList, op1t1.Obj)
	assert.Contains(tList, op1t2.Obj)
	assert.Contains(tList, op2t1.Obj)
	assert.Equal(op1t1.Obj, tList[0])
	assert.Equal(op1t2.Obj, tList[1])
	assert.Equal(op2t1.Obj, tList[2])

	tList = m.ListTasks("op1")
	assert.NotNil(tList)
	assert.Len(tList, 2)
	assert.Contains(tList, op1t1.Obj)
	assert.Contains(tList, op1t2.Obj)

	tList = m.ListTasks("op2")
	assert.NotNil(tList)
	assert.Len(tList, 1)
	assert.Contains(tList, op2t1.Obj)

	tList = m.ListTasks("op3")
	assert.NotNil(tList)
	assert.Len(tList, 0)

	// CancelTask
	assert.False(op1t1.Canceled)
	err = m.CancelTask(string(op1t1.Meta.ID))
	assert.NoError(err)
	assert.True(op1t1.Canceled)
	assert.True(op1t1.IsCanceled())
	err = m.CancelTask("foo")
	assert.Error(err)

	// Terminate
	assert.False(m.terminated)
	m.Terminate()
	assert.Len(m.tasks, 3)
	for _, task := range m.tasks {
		assert.True(task.Canceled)
		assert.True(task.IsCanceled())
	}
	assert.True(m.terminated)
	select {
	case <-m.ctx.Done(): // will not block forever
	}
	m.Terminate() // no-op
	assert.True(m.terminated)
}

func TestRunTask(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	evM := fev.NewFakeEventManager()
	ma := &ManagerArgs{
		Log:      tl.Logger(),
		CrudeOps: evM,
	}
	ts, err := NewManager(ma)
	assert.NoError(err)
	m, ok := ts.(*Manager)
	assert.True(ok)
	assert.NotNil(m)

	m.PurgeDelay = time.Millisecond * 500
	tIn := &models.Task{
		TaskCreateOnce: models.TaskCreateOnce{
			Operation: "OP1",
		},
	}
	tid, err := m.RunTask(tIn)
	assert.Error(err)
	assert.Empty(tid)

	tA := &taskTester{}
	m.RegisterAnimator("OP1", tA)

	tid, err = m.RunTask(tIn)
	assert.NoError(err)
	assert.NotEmpty(tid)
	assert.True(evM.CalledIEnum > 0) // external interface injects CRUD events
	tObj := m.findTask(tid)
	ce := evM.InjectFirstEvent
	assert.NotNil(ce)
	assert.Equal("POST", ce.Method)
	assert.Equal("/tasks", ce.TrimmedURI)
	assert.Len(ce.Scope, 3)
	assert.EqualValues(tObj.Meta.ID, ce.Scope[crude.ScopeMetaID])
	assert.Equal(tObj.Operation, ce.Scope["operation"])
	assert.Equal("NEW", ce.Scope["state"])
	for len(m.tasks) > 0 { // wait until the expire timer ticks
		time.Sleep(time.Millisecond)
	}
}
