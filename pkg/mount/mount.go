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


package mount

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/rei"
	logging "github.com/op/go-logging"
)

// Mounter is an interface to mount or unmount Nuvo volumes.
// Concurrent operations are supported on different objects.
type Mounter interface {
	FreezeFilesystem(ctx context.Context, args *FilesystemFreezeArgs) error
	IsFilesystemMounted(ctx context.Context, args *FilesystemMountArgs) (bool, error)
	MountFilesystem(ctx context.Context, args *FilesystemMountArgs) error
	UnfreezeFilesystem(ctx context.Context, args *FilesystemFreezeArgs) error
	UnmountFilesystem(ctx context.Context, args *FilesystemUnmountArgs) error
}

// ErrInvalidArgs is returned if arguments are not correct
var ErrInvalidArgs = fmt.Errorf("invalid args")

// DefaultCommandTimeout is the default maximum time for command to complete
const DefaultCommandTimeout = time.Minute * 5

// MounterArgs contain arguments to create a Mounter.
type MounterArgs struct {
	Log *logging.Logger
	Rei *rei.EphemeralPropertyManager // optional
}

// Validate checks MounterArgs.
func (a *MounterArgs) Validate() error {
	if a.Log == nil {
		return ErrInvalidArgs
	}
	return nil
}

// FilesystemMountArgs are arguments to MountFilesystem
type FilesystemMountArgs struct {
	// Identifier prefix for log messages
	LogPrefix string
	// Source is the Nuvo device (file) path
	Source string
	// Target is the mount point path
	Target string
	// FsType is the file system type - an OS specific default is used if empty
	FsType string
	// Options are mount options.
	// Recognized values: "ro" (read-only)
	Options []string
	// Deadline for completion - a default is used if unset
	Deadline time.Time
	// Do not run the commands
	PlanOnly bool
	// Commands issued (one per line) - optional, intended for visibility into the actions taken
	Commands *bytes.Buffer
}

// Validate checks FilesystemMountArgs.
func (a *FilesystemMountArgs) Validate() error {
	if a.Source == "" || a.Target == "" {
		return ErrInvalidArgs
	}
	if a.Deadline.Equal(time.Time{}) {
		a.Deadline = time.Now().Add(DefaultCommandTimeout)
	}
	return nil
}

// FilesystemUnmountArgs are arguments to UnmountFilesystem
type FilesystemUnmountArgs struct {
	// Identifier prefix for log messages
	LogPrefix string
	// Target is the mount point path
	Target string
	// Deadline for completion - a default is used if unset
	Deadline time.Time
	// Do not run the commands
	PlanOnly bool
	// Commands issued (one per line) - optional, intended for visibility into the actions taken
	Commands *bytes.Buffer
}

// Validate checks FilesystemUnmountArgs.
func (a *FilesystemUnmountArgs) Validate() error {
	if a.Target == "" {
		return ErrInvalidArgs
	}
	if a.Deadline.Equal(time.Time{}) {
		a.Deadline = time.Now().Add(DefaultCommandTimeout)
	}
	return nil
}

// FilesystemFreezeArgs are arguments to FreezeFilesystem or UnfreezeFilesystem
type FilesystemFreezeArgs struct {
	// Identifier prefix for log messages
	LogPrefix string
	// Target is the mount point path
	Target string
	// FsType is the file system type
	// FsType string // TBD - require in the future
	// Deadline for completion - a default is used if unset
	Deadline time.Time
	// Do not run the commands
	PlanOnly bool
	// Commands issued (one per line) - optional, intended for visibility into the actions taken
	Commands *bytes.Buffer
}

// Validate checks FilesystemFreezeArgs.
func (a *FilesystemFreezeArgs) Validate() error {
	if a.Target == "" {
		return ErrInvalidArgs
	}
	if a.Deadline.Equal(time.Time{}) {
		a.Deadline = time.Now().Add(DefaultCommandTimeout)
	}
	return nil
}
