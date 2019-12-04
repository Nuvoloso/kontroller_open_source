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
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestStackLogger(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	ret := NewStackLogger(l)
	assert.NotNil(ret)
	sl, ok := ret.(*stackLogger)
	assert.True(ok)

	t.Log("case: call Dump directly")
	sl.Dump()
	assert.Equal(1, tl.CountPattern("STACK TRACE"))
	tl.Flush()

	sl.Start()
	assert.NotNil(sl.sigChan)
	time.Sleep(10 * time.Millisecond)
	for tl.CountPattern("StackLogger started") == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	tl.Flush()

	sl.sigChan <- os.Signal(syscall.SIGUSR1)
	time.Sleep(10 * time.Millisecond)
	for tl.CountPattern("STACK TRACE") == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	tl.Flush()

	sl.Stop()
	time.Sleep(10 * time.Millisecond)
	for tl.CountPattern("StackLogger stopped") == 0 {
		time.Sleep(10 * time.Millisecond)
	}
}
