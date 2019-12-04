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


package testutils

import (
	"context"
	"time"

	"github.com/golang/mock/gomock"
)

type ctxDeadlineMatcher struct {
	expected  time.Time
	isTimeout bool
}

// NewCtxDeadlineMatcher returns a context with deadline matcher
func NewCtxDeadlineMatcher(deadline time.Time) gomock.Matcher {
	return &ctxDeadlineMatcher{expected: deadline}
}

// NewCtxTimeoutMatcher returns a context with timeout matcher. Should be called before the real context is created
func NewCtxTimeoutMatcher(timeout time.Duration) gomock.Matcher {
	deadline := time.Now().Add(timeout)
	return &ctxDeadlineMatcher{expected: deadline, isTimeout: true}
}

func (m *ctxDeadlineMatcher) Matches(x interface{}) bool {
	if x != nil {
		ctx := x.(context.Context) // or panic
		if dl, ok := ctx.Deadline(); ok {
			if m.isTimeout {
				return dl.After(m.expected)
			}
			return m.expected.Equal(dl)
		}
	}
	return false
}

func (m *ctxDeadlineMatcher) String() string {
	return "ctx deadline matches"
}
