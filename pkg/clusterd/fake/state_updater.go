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

	"github.com/Nuvoloso/kontroller/pkg/clusterd"
)

// StateUpdater fakes clusterd.StateUpdater
type StateUpdater struct {
	RetHTPsec int64

	CntUS    int
	RetUSerr error
}

var _ = clusterd.StateUpdater(&StateUpdater{})

// HeartbeatTaskPeriodSecs fakes its namesake
func (c *StateUpdater) HeartbeatTaskPeriodSecs() int64 {
	return c.RetHTPsec
}

// UpdateState fakes its namesake
func (c *StateUpdater) UpdateState(ctx context.Context) error {
	c.CntUS++
	return c.RetUSerr
}
