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

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/Nuvoloso/kontroller/pkg/vra"
)

// RWaiter fakes the vra.Waiter object for testing
type RWaiter struct {
	vra.RWaiter

	Error error
	Vsr   *models.VolumeSeriesRequest
	Sr    *models.StorageRequest
	Ctx   context.Context
	Can   context.CancelFunc
}

// NewFakeRequestWaiter returns a new fake RWaiter
func NewFakeRequestWaiter() *RWaiter {
	return &RWaiter{}
}

var _ = vra.RequestWaiter(&RWaiter{})

// RW fakes the interface of the same name
func (rw *RWaiter) RW() *vra.RWaiter {
	return &rw.RWaiter
}

// WaitForVSR fakes its namesake
func (rw *RWaiter) WaitForVSR(ctx context.Context) (*models.VolumeSeriesRequest, error) {
	return rw.Vsr, rw.Error
}

// WaitForSR fakes its namesake
func (rw *RWaiter) WaitForSR(ctx context.Context) (*models.StorageRequest, error) {
	return rw.Sr, rw.Error
}

// CrudeNotify fakes its namesake
func (rw *RWaiter) CrudeNotify(cbt crude.WatcherCallbackType, ce *crude.CrudEvent) error {
	return nil
}

// HastenContextDeadline fakes its namesake
func (rw *RWaiter) HastenContextDeadline(ctx context.Context, t time.Duration) (context.Context, context.CancelFunc) {
	return rw.Ctx, rw.Can
}
