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
	"github.com/Nuvoloso/kontroller/pkg/housekeeping"
)

// TaskOps implements a fake housekeeping.TaskOps interface
type TaskOps struct {
	// IsCanceled
	RetIc bool

	// Object
	RetO *models.Task

	// SetProgress
	InSp *models.Progress

	// SetState
	InSs housekeeping.TaskState
}

var _ = housekeeping.TaskOps(&TaskOps{})

// IsCanceled fakes its namesake
func (top *TaskOps) IsCanceled() bool {
	return top.RetIc
}

// Object fakes its namesake
func (top *TaskOps) Object() *models.Task {
	return top.RetO
}

// SetProgress fakes its namesake
func (top *TaskOps) SetProgress(progress *models.Progress) {
	top.InSp = progress
}

// SetState fakes its namesake
func (top *TaskOps) SetState(state housekeeping.TaskState) {
	top.InSs = state
	if state.IsTerminalState() {
		panic("fakeTaskOps abort")
	}
}
