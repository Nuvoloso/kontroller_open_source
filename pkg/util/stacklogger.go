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


package util

import (
	"bytes"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/op/go-logging"
)

// StackLogger provides an interface that logs stacks of all go routines
type StackLogger interface {
	Dump()
	Start()
	Stop()
}

type stackLogger struct {
	log     *logging.Logger
	sigChan chan os.Signal
}

// NewStackLogger creates a new, stopped StackLogger
func NewStackLogger(log *logging.Logger) StackLogger {
	return &stackLogger{
		log: log,
	}
}

// Dump logs the stacks of all go routines. It can be called even if the StackLogger is stopped
func (sl *stackLogger) Dump() {
	buf := make([]byte, 10*1024*1024)
	numBytes := runtime.Stack(buf, true)
	sl.log.Infof("STACK TRACE:\n%s", bytes.TrimSpace(buf[:numBytes]))
}

// Start starts a go routine that will dump all go routine stacks to the log each time it receives a USR1 signal
func (sl *stackLogger) Start() {
	sl.sigChan = make(chan os.Signal, 1)
	signal.Notify(sl.sigChan, os.Signal(syscall.SIGUSR1))
	go sl.run()
}

// Stop causes a previously started StackLogger to terminate
func (sl *stackLogger) Stop() {
	signal.Stop(sl.sigChan)
	close(sl.sigChan)
	sl.sigChan = nil
}

func (sl *stackLogger) run() {
	sl.log.Infof("StackLogger started")
	for range sl.sigChan {
		sl.Dump()
	}
	sl.log.Infof("StackLogger stopped")
}
