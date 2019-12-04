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
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/auth"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	logging "github.com/op/go-logging"
)

// TaskScheduler offers methods to operate on Tasks
type TaskScheduler interface {
	// CancelTask marks a local task as cancelled. It is up to the Task animator to detect this and change the state.
	CancelTask(id string) error
	// ListTasks returns a list of tasks, optionally filtered by operation.
	ListTasks(operation string) []*models.Task
	// RegisterAnimator provides an animator for an operation
	RegisterAnimator(operation string, animator TaskAnimator)
	// RegisterHandler registers REST API handlers
	RegisterHandlers(api *operations.NuvolosoAPI, accessManager AccessControl)
	// RunTask runs a task in the local process. It returns the task identifier.
	// Only the creation time properties should be set.
	RunTask(task *models.Task) (string, error)
	// Terminate sends a termination request to all tasks
	Terminate()
}

// ErrUnsupportedOperation is returned if the operation is not supported
var ErrUnsupportedOperation = fmt.Errorf("%s", com.ErrorUnsupportedOperation)

// ErrInvalidAnimator may be returned if an animator is handed the wrong operation
var ErrInvalidAnimator = fmt.Errorf("invalid animator")

// ErrInvalidArguments may be returned if a task's arguments are not correct
var ErrInvalidArguments = fmt.Errorf("invalid arguments")

// AccessControl is the interface to support access control on tasks through the REST API
type AccessControl interface {
	auth.AccessControl
	TaskViewOK(subject auth.Subject, op string) bool
	TaskCreateOK(subject auth.Subject, op string) bool
	TaskCancelOK(subject auth.Subject, op string) bool
}

// DefaultPurgeDelay is the default value for PurgeDelay if it is not set
const DefaultPurgeDelay = time.Duration(60 * time.Second)

// ManagerArgs contains the arguments required to create a Manager object
type ManagerArgs struct {
	PurgeDelay time.Duration
	CrudeOps   crude.Ops
	Log        *logging.Logger
}

// Validate checks the correctness of the arguments
func (ma *ManagerArgs) Validate() error {
	if ma.PurgeDelay == 0 {
		ma.PurgeDelay = DefaultPurgeDelay
	}
	if ma.PurgeDelay < 0 || ma.CrudeOps == nil || ma.Log == nil {
		return fmt.Errorf("invalid arguments")
	}
	return nil
}

// Manager implements the TaskScheduler interface
type Manager struct {
	ManagerArgs
	mux           sync.Mutex
	registry      map[string]TaskAnimator
	idCounter     int
	tasks         map[string]*Task
	accessManager AccessControl
	ctx           context.Context
	cancelFn      context.CancelFunc
	terminated    bool
}

// NewManager returns an object that implements the TaskScheduler interface
func NewManager(args *ManagerArgs) (TaskScheduler, error) {
	if err := args.Validate(); err != nil {
		return nil, err
	}
	m := &Manager{ManagerArgs: *args}
	m.registry = make(map[string]TaskAnimator)
	m.tasks = make(map[string]*Task)
	m.ctx, m.cancelFn = context.WithCancel(context.Background())
	return m, nil
}

// RegisterAnimator is part of the TaskScheduler interface
func (m *Manager) RegisterAnimator(operation string, animator TaskAnimator) {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.registry[operation] = animator
}

// RunTask is part of the TaskScheduler interface
func (m *Manager) RunTask(mObj *models.Task) (string, error) {
	return m.runTask(mObj, false)
}

// runTask is the internal implementation of RunTask
func (m *Manager) runTask(mObj *models.Task, fromHandler bool) (string, error) {
	m.mux.Lock()
	defer m.mux.Unlock()
	t, err := m.newTask(mObj)
	if err != nil {
		return "", err
	}
	t.run()
	if !fromHandler {
		t.postCE("POST")
	}
	return string(t.Meta.ID), nil
}

// ListTasks is part of the TaskScheduler interface
func (m *Manager) ListTasks(operation string) []*models.Task {
	m.mux.Lock()
	defer m.mux.Unlock()
	ret := []*models.Task{}
	var tl Tasks = make([]*Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		tl = append(tl, t)
	}
	sort.Sort(tl)
	for _, t := range tl {
		mObj := t.Object()
		if operation == "" || mObj.Operation == operation {
			ret = append(ret, mObj)
		}
	}
	return ret
}

// CancelTask is part of the TaskScheduler interface
func (m *Manager) CancelTask(id string) error {
	m.mux.Lock()
	defer m.mux.Unlock()
	if t, found := m.tasks[id]; found {
		t.Canceled = true
	} else {
		return fmt.Errorf("not found")
	}
	return nil
}

// Terminate is part of the TaskScheduler interface
func (m *Manager) Terminate() {
	if m.terminated {
		return
	}
	m.mux.Lock()
	for _, t := range m.tasks {
		t.Canceled = true
	}
	m.terminated = true
	m.mux.Unlock()
	defer m.cancelFn()
}

// opAnimator finds the animator for an operation. Call within the mutex.
func (m *Manager) opAnimator(op string) (TaskAnimator, error) {
	animator, found := m.registry[op]
	if !found {
		return nil, ErrUnsupportedOperation
	}
	return animator, nil
}

// trackTask tracks a task. Call within the mutex.
func (m *Manager) trackTask(t *Task) {
	m.tasks[string(t.Meta.ID)] = t
}

// delete a task. Obtains the mutex.
func (m *Manager) deleteTask(t *Task) {
	t.M.mux.Lock()
	defer t.M.mux.Unlock()
	delete(t.M.tasks, string(t.Meta.ID))
	t.M.Log.Debugf("Task %s: deleted (%s)", t.Meta.ID, t.State)
}

// findTask looks up a task by id. Obtains the mutex.
func (m *Manager) findTask(id string) *Task {
	m.mux.Lock()
	defer m.mux.Unlock()
	if t, found := m.tasks[id]; found {
		return t
	}
	return nil
}
