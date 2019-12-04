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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCtxDeadlineMatcher(t *testing.T) {
	assert := assert.New(t)

	to := 5 * time.Minute
	dl := time.Now().Add(to)
	m := NewCtxDeadlineMatcher(dl)
	cm, ok := m.(*ctxDeadlineMatcher)
	if assert.True(ok) {
		assert.Equal(cm.expected, dl)
		assert.False(cm.isTimeout)
	}

	assert.Regexp(" matches", m.String())

	ctx, cancel := context.WithDeadline(context.Background(), dl)
	assert.True(m.Matches(ctx))
	cancel()

	dl = time.Now()
	ctx, cancel = context.WithDeadline(context.Background(), dl)
	assert.False(m.Matches(ctx))
	cancel()

	ctx, cancel = context.WithTimeout(context.Background(), to)
	assert.False(m.Matches(ctx))
	cancel()

	then := time.Now().Add(to)
	m = NewCtxTimeoutMatcher(to)
	cm, ok = m.(*ctxDeadlineMatcher)
	if assert.True(ok) {
		assert.True(cm.expected.After(then))
		assert.True(cm.isTimeout)
	}

	ctx, cancel = context.WithTimeout(context.Background(), to)
	assert.True(m.Matches(ctx))
	cancel()

	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	assert.False(m.Matches(ctx))
	cancel()

	assert.False(m.Matches(nil))
	assert.Panics(func() { m.Matches("not context") })
}
