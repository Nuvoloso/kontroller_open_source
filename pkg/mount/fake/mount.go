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
	"bytes"
	"context"

	"github.com/Nuvoloso/kontroller/pkg/mount"
)

// Mounter fakes mount.Mounter
type Mounter struct {
	InFsFArgs   *mount.FilesystemFreezeArgs
	RetFsFfCmds *bytes.Buffer
	RetFsFErr   error

	InIfmArgs  *mount.FilesystemMountArgs
	RetIfmCmds *bytes.Buffer
	RetIfmBool bool
	RetIfmErr  error

	InMfArgs  *mount.FilesystemMountArgs
	RetMfCmds *bytes.Buffer
	RetMfErr  error

	InUFsFArgs  *mount.FilesystemFreezeArgs
	RetUFsFCmds *bytes.Buffer
	RetUFsFErr  error

	InUfArgs  *mount.FilesystemUnmountArgs
	RetUfCmds *bytes.Buffer
	RetUfErr  error
}

var _ = mount.Mounter(&Mounter{})

// FreezeFilesystem fakes its namesake
func (m *Mounter) FreezeFilesystem(ctx context.Context, args *mount.FilesystemFreezeArgs) error {
	m.InFsFArgs = args
	if m.RetFsFfCmds != nil {
		args.Commands = m.RetFsFfCmds
	}
	return m.RetFsFErr
}

// IsFilesystemMounted fakes its namesake
func (m *Mounter) IsFilesystemMounted(ctx context.Context, args *mount.FilesystemMountArgs) (bool, error) {
	m.InIfmArgs = args
	if m.RetIfmCmds != nil {
		args.Commands = m.RetIfmCmds
	}
	return m.RetIfmBool, m.RetIfmErr
}

// MountFilesystem fakes its namesake
func (m *Mounter) MountFilesystem(ctx context.Context, args *mount.FilesystemMountArgs) error {
	m.InMfArgs = args
	if m.RetMfCmds != nil {
		args.Commands = m.RetMfCmds
	}
	return m.RetMfErr
}

// UnfreezeFilesystem fakes its namesake
func (m *Mounter) UnfreezeFilesystem(ctx context.Context, args *mount.FilesystemFreezeArgs) error {
	m.InUFsFArgs = args
	if m.RetUFsFCmds != nil {
		args.Commands = m.RetUFsFCmds
	}
	return m.RetUFsFErr
}

// UnmountFilesystem fakes its namesake
func (m *Mounter) UnmountFilesystem(ctx context.Context, args *mount.FilesystemUnmountArgs) error {
	m.InUfArgs = args
	if m.RetUfCmds != nil {
		args.Commands = m.RetUfCmds
	}
	return m.RetUfErr
}
