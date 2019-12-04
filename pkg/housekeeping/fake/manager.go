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


package fake

import (
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	"github.com/Nuvoloso/kontroller/pkg/housekeeping"
)

// TaskScheduler implements a fake housekeeping.TaskScheduler interface
type TaskScheduler struct {
	// CancelTask
	InCtID   string
	RetCtErr error

	// ListTasks
	InLtOp string
	RetLt  []*models.Task

	// RegisterAnimator
	InRaOp        string
	InRaTA        housekeeping.TaskAnimator
	OpsRegistered []string

	// RegisterHandler
	InRhAPI *operations.NuvolosoAPI
	InRhAM  housekeeping.AccessControl

	// RunTask
	InRt     *models.Task
	RetRtID  string
	RetRtErr error

	// Terminate
	CalledT bool
}

var _ = housekeeping.TaskScheduler(&TaskScheduler{})

// CancelTask fakes its namesake
func (m *TaskScheduler) CancelTask(id string) error {
	m.InCtID = id
	return m.RetCtErr
}

// ListTasks fakes its namesake
func (m *TaskScheduler) ListTasks(operation string) []*models.Task {
	m.InLtOp = operation
	return m.RetLt
}

// RegisterAnimator fakes its namesake
func (m *TaskScheduler) RegisterAnimator(operation string, animator housekeeping.TaskAnimator) {
	m.InRaOp = operation
	m.InRaTA = animator
	m.OpsRegistered = append(m.OpsRegistered, operation)
}

// RegisterHandlers fakes its namesake
func (m *TaskScheduler) RegisterHandlers(api *operations.NuvolosoAPI, accessTaskScheduler housekeeping.AccessControl) {
	m.InRhAPI = api
	m.InRhAM = accessTaskScheduler
}

// RunTask fakes its namesake
func (m *TaskScheduler) RunTask(task *models.Task) (string, error) {
	m.InRt = task
	return m.RetRtID, m.RetRtErr
}

// Terminate fakes its namesake
func (m *TaskScheduler) Terminate() {
	m.CalledT = true
}
