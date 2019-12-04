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
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestTaskState(t *testing.T) {
	assert := assert.New(t)

	for ts := TaskStateNew; ts < TaskStateLast; ts++ {
		s := ts.String()
		assert.NotEmpty(s)
		assert.NotEqual("UNKNOWN", s)
		if ts == TaskStateSucceeded || ts == TaskStateFailed || ts == TaskStateCanceled {
			assert.True(ts.IsTerminalState())
		} else {
			assert.False(ts.IsTerminalState())
		}
	}
	ts := TaskStateLast
	assert.Equal("UNKNOWN", ts.String())
	ts = -1
	assert.EqualValues("UNKNOWN", ts.String())
}

func TestTaskSorter(t *testing.T) {
	assert := assert.New(t)

	// we cannot depend on ListTasks to give us 100% code coverage on the sort support
	// as maps may occasionally iterate in perfect sort order, resulting in Swap not being called

	tl := Tasks{
		&Task{Ordinal: 2},
		&Task{Ordinal: 3},
		&Task{Ordinal: 1},
	}
	expTl := Tasks{
		&Task{Ordinal: 1},
		&Task{Ordinal: 2},
		&Task{Ordinal: 3},
	}
	assert.NotEqual(expTl, tl)
	sort.Sort(tl)
	assert.Equal(expTl, tl)
}

func TestTaskOps(t *testing.T) {
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
	assert.Equal(0, m.idCounter)

	// newTask
	newTask := func(aTask *models.Task) (*Task, error) {
		m.mux.Lock()
		defer m.mux.Unlock()
		return m.newTask(aTask)
	}
	tIn := &models.Task{
		TaskCreateOnce: models.TaskCreateOnce{
			Operation: "OP1",
		},
	}
	tObj, err := newTask(tIn)
	assert.Error(err)
	assert.Equal(ErrUnsupportedOperation, err)
	assert.Nil(tObj)
	assert.Equal(0, m.idCounter)

	tA := &taskTester{}
	m.RegisterAnimator("OP1", tA)

	tA.RetTv = fmt.Errorf("something not right")
	tObj, err = newTask(tIn)
	assert.Error(err)
	assert.Regexp("something not right", err)
	assert.Nil(tObj)
	assert.Equal(0, m.idCounter)

	tA.RetTv = nil
	tObj, err = newTask(tIn)
	assert.NoError(err)
	assert.NotNil(tObj)
	assert.Equal(1, m.idCounter)
	assert.NotNil(tObj.Meta)
	assert.NotZero(tObj.Meta)
	assert.NotZero(tObj.Meta.TimeCreated)
	assert.Equal(tObj.Meta.TimeCreated, tObj.Meta.TimeModified)
	assert.EqualValues(0, tObj.Meta.Version)
	assert.Equal(TaskStateNew, tObj.State)
	assert.Nil(tObj.Progress)
	ver := tObj.Meta.Version

	tList := m.ListTasks("")
	assert.Len(tList, 1)
	tO, found := m.tasks[string(tList[0].Meta.ID)]
	assert.True(found)
	assert.Equal(tObj, tO)

	// Object (no progress)
	assert.True(tIn == tObj.Obj)
	tIn.Meta = nil
	tIn.Operation = ""
	tIn.State = ""
	tIn.Progress = nil
	mObj := tObj.Object()
	assert.Equal(tList[0], mObj)
	assert.Equal("OP1", mObj.Operation)
	assert.NotNil(mObj.Meta)
	assert.Equal(tObj.Meta.Version, mObj.Meta.Version)
	assert.Equal("NEW", mObj.State)
	assert.Nil(mObj.Progress)
	assert.True(tIn == mObj)

	// SetProgress
	evM.InIEev = nil
	assert.Nil(tObj.Progress)
	tObj.SetProgress(&models.Progress{PercentComplete: swag.Int32(89)})
	assert.NotNil(tObj.Progress)
	assert.EqualValues(89, swag.Int32Value(tObj.Progress.PercentComplete))
	assert.Equal(tObj.Meta.TimeModified, tObj.Progress.Timestamp)
	assert.EqualValues(ver+1, tObj.Meta.Version)
	ce := evM.InIEev
	assert.NotNil(ce)
	assert.Equal("PATCH", ce.Method)
	assert.Equal(fmt.Sprintf("/tasks/%s", tObj.Meta.ID), ce.TrimmedURI)
	assert.Len(ce.Scope, 2)
	assert.Equal(tObj.Operation, ce.Scope["operation"])
	assert.Equal(tObj.State.String(), ce.Scope["state"])

	// Object (with progress)
	assert.True(tIn == tObj.Obj)
	tIn.Meta = nil
	tIn.Operation = ""
	tIn.State = ""
	tIn.Progress = nil
	mObj = tObj.Object()
	assert.Equal(tList[0], mObj)
	assert.Equal("OP1", mObj.Operation)
	assert.NotNil(mObj.Meta)
	assert.Equal(tObj.Meta.Version, mObj.Meta.Version)
	assert.Equal("NEW", mObj.State)
	assert.NotNil(mObj.Progress)
	assert.EqualValues(89, swag.Int32Value(mObj.Progress.PercentComplete))
	assert.True(tIn == mObj)

	// IsCanceled, Object with cancelRequested
	assert.False(tObj.Canceled)
	assert.False(tObj.IsCanceled())
	tObj.Canceled = true
	assert.True(tObj.IsCanceled())
	mObj = tObj.Object()
	assert.True(mObj.CancelRequested)
	tObj.Canceled = false
	assert.False(tObj.IsCanceled())
	mObj = tObj.Object()
	assert.False(mObj.CancelRequested)

	// SetState
	mObj.Messages = []*models.TimestampedString{}
	ver = tObj.Meta.Version
	tObj.SetState(tObj.State)
	assert.Empty(mObj.Messages)
	assert.NotEqual(TaskStateWaiting, tObj.State)
	tObj.SetState(TaskStateWaiting)
	assert.Equal(TaskStateWaiting, tObj.State)
	assert.Len(mObj.Messages, 1)
	assert.Regexp("State change.*NEW.*WAITING", mObj.Messages[0].Message)
	assert.Equal(tObj.Meta.TimeModified, mObj.Messages[0].Time)
	assert.EqualValues(ver+1, tObj.Meta.Version)

	// runBody, falloff TaskExec
	// sets state to ACTIVE then SUCCEEDED after Exec returns
	tl.Flush()
	tA.ExecCalled = false
	m.PurgeDelay = time.Millisecond * 5
	assert.Nil(tObj.ExpTimer)
	tObj.runBody()
	assert.True(tA.ExecCalled)
	assert.Equal(TaskStateSucceeded, tObj.State)
	assert.Len(mObj.Messages, 3)
	assert.Regexp("State change.*WAITING.*ACTIVE", mObj.Messages[1].Message)
	assert.Regexp("State change.*ACTIVE.*SUCCEEDED", mObj.Messages[2].Message)
	assert.NotNil(tObj.ExpTimer)
	assert.Equal(TaskStateSucceeded, tObj.State)
	for len(m.tasks) > 0 { // wait until the expire timer ticks
		time.Sleep(time.Millisecond)
	}
	assert.Equal(1, tl.CountPattern("terminating thread"))
	assert.Equal(1, tl.CountPattern("terminated"))

	// run, panic in TaskExec
	// sets state to ACTIVE then FAILED after panic
	tl.Flush()
	tA.ExecCalled = false
	tA.MustPanic = true
	m.PurgeDelay = time.Millisecond * 5
	tIn = &models.Task{
		TaskCreateOnce: models.TaskCreateOnce{
			Operation: "OP1",
		},
	}
	tObj, err = newTask(tIn)
	tO, found = m.tasks[string(tObj.Meta.ID)]
	assert.True(found)
	assert.Equal(tObj, tO)
	mObj = tObj.Object()
	evM.InIEev = nil
	tObj.run()             // asynchronous
	for len(m.tasks) > 0 { // wait until the expire timer ticks
		time.Sleep(time.Millisecond)
	}
	assert.Equal(0, tl.CountPattern("terminating thread"))
	assert.Equal(0, tl.CountPattern("terminated"))
	assert.Equal(1, tl.CountPattern("PANIC"))
	ce = evM.InIEev
	assert.NotNil(ce)
	assert.Equal("DELETE", ce.Method)
	assert.Equal(fmt.Sprintf("/tasks/%s", tObj.Meta.ID), ce.TrimmedURI)
	assert.Len(ce.Scope, 2)
	assert.Equal(tObj.Operation, ce.Scope["operation"])
	assert.Equal(tObj.State.String(), ce.Scope["state"])
}

// taskTester satisfies TaskAnimator
type taskTester struct {
	// TaskExec
	ExecCalled bool
	MustPanic  bool

	// TaskValidate
	InTv  *models.TaskCreateOnce
	RetTv error
}

// TaskExec is part of the TaskAnimator interface
func (tt *taskTester) TaskExec(ctx context.Context, ops TaskOps) {
	tt.ExecCalled = true
	if tt.MustPanic {
		panic("animator panic")
	}
}

// TaskValidate is part of the TaskAnimator interface
func (tt *taskTester) TaskValidate(createArgs *models.TaskCreateOnce) error {
	tt.InTv = createArgs
	return tt.RetTv
}
