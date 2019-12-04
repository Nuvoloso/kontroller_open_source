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
	"bytes"
	"context"
	"fmt"
	"runtime/debug"
	"sort"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/strfmt"
)

// TaskState is the integer type for state
type TaskState int

// TaskState values
const (
	TaskStateNew TaskState = iota
	TaskStateActive
	TaskStateWaiting
	// Terminal states
	TaskStateCanceled
	TaskStateFailed
	TaskStateSucceeded

	TaskStateLast // not a real state
)

func (cs TaskState) String() string {
	switch cs {
	case TaskStateNew:
		return "NEW"
	case TaskStateActive:
		return "ACTIVE"
	case TaskStateWaiting:
		return "WAITING"
	case TaskStateCanceled:
		return "CANCELED"
	case TaskStateFailed:
		return "FAILED"
	case TaskStateSucceeded:
		return "SUCCEEDED"
	}
	return "UNKNOWN"
}

// IsTerminalState indicates if the state is a terminal state
func (cs TaskState) IsTerminalState() bool {
	return cs == TaskStateSucceeded || cs == TaskStateFailed || cs == TaskStateCanceled
}

// TaskOps is an interface that provides operations on a Task.
type TaskOps interface {
	// IsCanceled returns true if Cancel was called on this Task.
	IsCanceled() bool
	// Object returns the model object.
	// The Meta, State, Progress, Operation and CancelRequested properties reflect the internally
	//  tracked values and cannot be modified directly by the animator.
	Object() *models.Task
	// SetProgress updates the Progress data
	SetProgress(progress *models.Progress)
	// SetState changes the state of the Task and posts related CRUDE.
	// If the state is a terminal state then the Task thread will forcibly be terminated.
	SetState(state TaskState)
}

// TaskAnimator animates Task objects
type TaskAnimator interface {
	// TaskValidate validates its arguments.
	// It may return standard errors ErrInvalidAnimator or ErrInvalidArguments or any other error.
	TaskValidate(createArgs *models.TaskCreateOnce) error
	// TaskExec executes the task. If it returns the task is assumed to have entered the SUCCEEDED state.
	// It may abort its execution by calling ops.SetState() with a terminal state.
	// If it choses to support external cancellation, it should use ops.IsCancelled() at appropriate times
	// to check if cancelled and then call ops.SetState(TaskStateCanceled) to terminate.
	TaskExec(ctx context.Context, ops TaskOps)
}

// Task encapsulates the models.Task object
type Task struct {
	A          TaskAnimator
	M          *Manager
	Obj        *models.Task // externally mutable
	Meta       models.ObjMeta
	Operation  string
	State      TaskState
	Progress   *models.Progress
	Canceled   bool
	Terminated bool
	ExpTimer   *time.Timer
	Ordinal    int
}

// Object is part of the TaskOps interface.
func (t *Task) Object() *models.Task {
	// restore vulnerable fields
	if t.Obj.Meta == nil {
		t.Obj.Meta = &models.ObjMeta{}
	}
	*t.Obj.Meta = t.Meta // copy values
	t.Obj.State = t.State.String()
	t.Obj.Operation = t.Operation
	t.Obj.CancelRequested = t.Canceled
	if t.Progress != nil { // may be unset
		if t.Obj.Progress == nil {
			t.Obj.Progress = &models.Progress{}
		}
		*t.Obj.Progress = *t.Progress
	}
	return t.Obj
}

// IsCanceled is part of the TaskOps interface.
func (t *Task) IsCanceled() bool {
	return t.Canceled
}

// SetState is part of the TaskOps interface.
func (t *Task) SetState(state TaskState) {
	if t.State == state {
		return
	}
	msg := fmt.Sprintf("State change %s ⇒ %s", t.State, state)
	t.M.Log.Debugf("Task %s: %s", t.Meta.ID, msg)
	t.State = state
	now := time.Now()
	msgList := util.NewMsgList(t.Obj.Messages).WithTimestamp(now)
	msgList.Insert(msg)
	t.Obj.Messages = msgList.ToModel()
	t.modified(now)
	if t.State.IsTerminalState() {
		t.Terminated = true
		t.scheduleDeletion()
		t.M.Log.Debugf("Task %s: terminating thread", t.Meta.ID)
		panic("task forcing termination of thread")
	}
}

// SetProgress is part of the TaskOps interface.
func (t *Task) SetProgress(progress *models.Progress) {
	if t.Progress == nil {
		t.Progress = &models.Progress{}
	}
	*t.Progress = *progress
	now := time.Now()
	t.Progress.Timestamp = strfmt.DateTime(now)
	t.modified(now)
}

func (t *Task) modified(now time.Time) {
	t.Meta.TimeModified = strfmt.DateTime(now)
	t.Meta.Version++
	t.postCE("PATCH")
}

func (t *Task) scheduleDeletion() {
	t.ExpTimer = time.AfterFunc(t.M.PurgeDelay, t.deleteTask)
}

func (t *Task) deleteTask() {
	t.M.deleteTask(t)
	t.postCE("DELETE")
}

func (t *Task) postCE(method string) {
	ce := &crude.CrudEvent{
		Method: method,
		Scope:  map[string]string{},
	}
	if method != "POST" {
		ce.TrimmedURI = fmt.Sprintf("/tasks/%s", t.Meta.ID)
	} else {
		ce.TrimmedURI = "/tasks"
		ce.Scope[crude.ScopeMetaID] = string(t.Meta.ID)
	}
	ce.Scope["operation"] = t.Operation
	ce.Scope["state"] = t.State.String()
	t.M.CrudeOps.InjectEvent(ce) // ignore error
}

// run calls runBody on a goroutine.
func (t *Task) run() {
	go t.runBody()
}

// runBody should be called on a go routine.
// It sets the state to Active and then invokes the task animator.
// It expects a panic on active termination, or will force a successful termination if TaskExec returns.
// If it gets a panic without termination it fails the task.
func (t *Task) runBody() {
	t.M.Log.Debugf("Task %s: Starting", t.Meta.ID)
	t.SetState(TaskStateActive)
	defer func() {
		if r := recover(); r != nil {
			if !t.Terminated {
				b := debug.Stack()
				t.M.Log.Criticalf("Task %s: PANIC: %v\n\n%s", t.Meta.ID, r, bytes.TrimSpace(b))
				t.State = TaskStateFailed
				t.scheduleDeletion()
			} else {
				t.M.Log.Debugf("Task %s: terminated (%s)", t.Meta.ID, t.State)
			}
		}
	}()
	t.A.TaskExec(t.M.ctx, t)
	// should not return but if it does terminate
	t.SetState(TaskStateSucceeded)
}

// newTask is the internal constructor.
// It should be called within the mutex, and it tracks but does not run the Task.
// No CRUDE posted.
func (m *Manager) newTask(mObj *models.Task) (*Task, error) {
	animator, err := m.opAnimator(mObj.Operation)
	if err != nil {
		return nil, err
	}
	// We need the mutex to access the animator map and the manager task tracking support
	// after task creation. However, we are vulnerable to a misbehaving validator so release
	// it across the call; all state is on the stack so this is safe.
	m.mux.Unlock()
	err = animator.TaskValidate(&mObj.TaskCreateOnce)
	m.mux.Lock()
	if err != nil {
		return nil, err
	}
	t := &Task{Obj: mObj, A: animator, M: m}
	t.Ordinal = m.idCounter
	t.Meta.ID = models.ObjID(fmt.Sprintf("%s%06d", mObj.Operation, m.idCounter))
	m.idCounter++
	t.Meta.TimeCreated = strfmt.DateTime(time.Now())
	t.Meta.TimeModified = t.Meta.TimeCreated
	t.Meta.ObjType = "Task"
	t.Operation = mObj.Operation
	t.State = TaskStateNew
	m.trackTask(t)
	return t, nil
}

// Tasks is a sortable list of tasks
type Tasks []*Task

// Len returns the length of the task list
func (tl Tasks) Len() int { return len(tl) }

// Less is a predicate for the natural (creation) sorting order of tasks
func (tl Tasks) Less(i, j int) bool { return tl[i].Ordinal < tl[j].Ordinal }

// Swap exchanges two elements of the task list
func (tl Tasks) Swap(i, j int) { tl[i], tl[j] = tl[j], tl[i] }

var _ = sort.Interface(Tasks{})
