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

	"github.com/Nuvoloso/kontroller/pkg/vra"
)

// VolumeUpdater fakes the vra.VolumeUpdater interface
type VolumeUpdater struct {
	InSVSState string
	InSVSctx   context.Context

	InSVSTags []string
	InSVSTctx context.Context

	InRVSTags []string
	InRVSTctx context.Context
}

var _ = vra.VolumeUpdater(&VolumeUpdater{})

// SetVolumeState fakes its namesake
func (u *VolumeUpdater) SetVolumeState(ctx context.Context, newState string) {
	u.InSVSState = newState
	u.InSVSctx = ctx
}

// SetVolumeSystemTags  fakes its namesake
func (u *VolumeUpdater) SetVolumeSystemTags(ctx context.Context, systemTags ...string) {
	u.InSVSTags = systemTags
	u.InSVSTctx = ctx
}

// RemoveVolumeSystemTags  fakes its namesake
func (u *VolumeUpdater) RemoveVolumeSystemTags(ctx context.Context, systemTags ...string) {
	u.InRVSTags = systemTags
	u.InRVSTctx = ctx
}
