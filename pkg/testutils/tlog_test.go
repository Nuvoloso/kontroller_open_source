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
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoggerFunctionality(t *testing.T) {
	assert := assert.New(t)

	tl := NewTestLogger(t)
	l := tl.Logger()

	msgs1 := []string{"msg1", "msg2", "msg3"}
	for _, m := range msgs1 {
		l.Info(m)
	}
	lastID := tl.lastID
	count := tl.CountPattern("msg[0-2]")
	assert.Equal(2, count)
	assert.Equal(lastID, tl.lastID)
	got := []string{}
	tl.dump(func(args ...interface{}) {
		if s, ok := args[0].(string); ok {
			got = append(got, s)
			t.Log(s)
		} else {
			assert.True(ok)
		}
	})
	assert.Len(got, len(msgs1))
	for i, gm := range got {
		assert.Regexp(msgs1[i]+"$", gm)
		assert.Regexp("INFO", gm)
	}
	iterGot := []string{}
	tl.Iterate(func(n uint64, s string) { iterGot = append(iterGot, s) })
	assert.Len(iterGot, len(msgs1))
	for i, gm := range iterGot {
		assert.Regexp(msgs1[i]+"$", gm)
		assert.Regexp("INFO", gm)
	}

	msgs2 := []string{"msg4", "msg5", "msg6"}
	for _, m := range msgs2 {
		l.Error(m)
	}
	got = []string{}
	tl.dump(func(args ...interface{}) {
		if s, ok := args[0].(string); ok {
			got = append(got, s)
			t.Log(s)
		} else {
			assert.True(ok)
		}
	})
	assert.Len(got, len(msgs2))
	for i, gm := range got {
		assert.Regexp(msgs2[i]+"$", gm)
		assert.Regexp("ERROR", gm)
	}
	iterGot = []string{}
	tl.Iterate(func(n uint64, s string) { iterGot = append(iterGot, s) })
	assert.Len(iterGot, len(msgs1)+len(msgs2))
	for i, gm := range iterGot {
		if i < len(msgs1) {
			assert.Regexp(msgs1[i]+"$", gm)
			assert.Regexp("INFO", gm)
		} else {
			assert.Regexp(msgs2[i-len(msgs1)]+"$", gm)
			assert.Regexp("ERROR", gm)
		}
	}
}

func TestLoggerRecommendedUsage(t *testing.T) {
	tl := NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	l.Info("info msg")
	l.Error("error msg")
	tl.Flush()
	l.Critical("critical msg")
	l.Debug("debug msg")
}

func TestLogToConsole(t *testing.T) {
	assert := assert.New(t)
	tl := NewTestLogger(t)
	tl.LogToConsole = true
	l := tl.Logger()
	l.Info("one")
	l.Info("two")
	l.Info("three")

	// https://stackoverflow.com/questions/10473800/in-go-how-do-i-capture-stdout-of-a-function-into-a-string
	origStdout := os.Stdout
	defer func() {
		os.Stdout = origStdout
	}()
	r, w, _ := os.Pipe()
	os.Stdout = w
	outC := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outC <- buf.String()
	}()
	tl.Flush()
	w.Close()
	out := <-outC
	assert.Regexp("(?s)one.*two.*three", out)
}
