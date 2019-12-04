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
	"context"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/util"
)

// Worker fakes a util.Worker
type Worker struct {
	// Buzz
	CntB    int
	RetBErr error

	// Start
	CntStart int

	// Started
	RetStarted bool

	// Stop
	CntStop int

	// Notify, GetNotifyCount
	CntNotify int

	// LastErr
	RetLErr error

	// SetSleepInterval
	InSSI time.Duration

	// GetSleepInterval
	RetGSI time.Duration
}

var _ = util.Worker(&Worker{})

// Buzz fakes its namespace
func (fw *Worker) Buzz(ctx context.Context) error {
	fw.CntB++
	return fw.RetBErr
}

// Start fakes its namespace
func (fw *Worker) Start() {
	fw.CntStart++
}

// Started fakes its namespace
func (fw *Worker) Started() bool {
	return fw.RetStarted
}

// Stop fakes its namespace
func (fw *Worker) Stop() {
	fw.CntStop++
}

// Notify fakes its namespace
func (fw *Worker) Notify() {
	fw.CntNotify++
}

// LastErr fakes its namespace
func (fw *Worker) LastErr() error {
	return fw.RetLErr
}

// SetSleepInterval fakes its namespace
func (fw *Worker) SetSleepInterval(si time.Duration) {
	fw.InSSI = si
}

// GetSleepInterval fakes its namespace
func (fw *Worker) GetSleepInterval() time.Duration {
	return fw.RetGSI
}

// GetNotifyCount fakes its namespace
func (fw *Worker) GetNotifyCount() int {
	return fw.CntNotify
}
