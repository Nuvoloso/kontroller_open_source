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
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestPanicLogger(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	// no panic, nothing logged
	PanicLogger(tl.Logger(), func() {})
	msgCount := 0
	tl.Iterate(func(i uint64, s string) {
		msgCount++
	})
	assert.Zero(msgCount, "should be nothing logged")
	tl.Flush()

	// panic, stack logged, actually use goroutine, racy
	go PanicLogger(tl.Logger(), func() { funcThatPanics(nil) })
	msgCount = 0
	msg := ""
	for i := 0; i < 10 && msgCount == 0; i++ {
		time.Sleep(100 * time.Millisecond)
		tl.Iterate(func(i uint64, s string) {
			msgCount++
			msg = s
		})
	}
	assert.Equal(1, msgCount)
	assert.Regexp("(?s)PANIC: runtime error.*funcThatPanics.*pkg/util/panic_test", msg)
	assert.Equal(strings.TrimSpace(msg), msg)

	// use a RecoverFunc
	var inR interface{}
	msg = ""
	go PanicLogger(tl.Logger(), func() { funcThatPanics(nil) }, func(r interface{}, b []byte) {
		inR = r
		msg = string(bytes.TrimSpace(b))
	})
	for i := 0; i < 10 && len(msg) == 0; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	assert.NotNil(inR)
	assert.Regexp("(?s).*funcThatPanics.*pkg/util/panic_test", msg)
}

func funcThatPanics(o *models.VolumeSeries) {
	fmt.Println(o.Meta.ID)
}
