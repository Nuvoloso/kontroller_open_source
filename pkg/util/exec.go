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
	"context"
	"os/exec"
	"strings"
	"syscall"
)

// Exec is an interface over the exec pkg API, + error analysis
type Exec interface {
	CommandContext(context.Context, string, ...string) Cmd
	IsNotFoundError(err error) bool
	IsExitError(err error) (int, bool)
}

// Cmd is an interface over the exec.Cmd object
type Cmd interface {
	CombinedOutput() ([]byte, error)
}

type execObj struct{}

// NewExec returns a new command factory
func NewExec() Exec {
	return &execObj{}
}

// CommandContext creates a new Cmd, note that nil ctx is not allowed by exec.CommandContext
func (*execObj) CommandContext(ctx context.Context, name string, arg ...string) Cmd {
	return exec.CommandContext(ctx, name, arg...)
}

// IsNotFoundError returns true if the error indicates that the command was not found
func (*execObj) IsNotFoundError(err error) bool {
	return strings.Index(err.Error(), "executable file not found") >= 0
}

// IsExitError returns the non-zero exit code if applicable (on a best-effort basis).
// See: https://stackoverflow.com/questions/10385551/get-exit-code-go
func (*execObj) IsExitError(err error) (int, bool) {
	if ee, ok := err.(*exec.ExitError); ok {
		if status, ok := ee.Sys().(syscall.WaitStatus); ok {
			return status.ExitStatus(), true
		}
	}
	return 0, false
}
