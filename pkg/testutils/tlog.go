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
	"fmt"
	"regexp"
	"testing"

	logging "github.com/op/go-logging"
)

// TestLogger represents a logger for use in UTs
type TestLogger struct {
	t            *testing.T
	logger       *logging.Logger
	lbe          *logging.MemoryBackend
	lastID       uint64
	LogToConsole bool
}

// NewTestLogger returns a test logger
func NewTestLogger(t *testing.T) *TestLogger {
	lbe := logging.InitForTesting(logging.DEBUG)
	logging.SetFormatter(logging.MustStringFormatter("%{id} %{shortfile} %{level} %{message}"))
	return &TestLogger{
		t:      t,
		lbe:    lbe,
		logger: logging.MustGetLogger(""),
	}
}

// Logger returns the logger
func (tl *TestLogger) Logger() *logging.Logger {
	return tl.logger
}

// Flush sends accumulated records to the tester log or to the console (if LogToConsole is true - useful in hanging UTs)
func (tl *TestLogger) Flush() {
	if tl.LogToConsole {
		// See https://stackoverflow.com/questions/23205419/how-do-you-print-in-a-go-test-using-the-testing-package
		tl.dump(func(args ...interface{}) { fmt.Println(args) })
	} else {
		tl.dump(tl.t.Log)
	}
}

// Iterate iterates over all the records
func (tl *TestLogger) Iterate(cb func(uint64, string)) {
	for n := tl.lbe.Head(); n != nil; n = n.Next() {
		cb(n.Record.ID, n.Record.Formatted(2))
	}
}

// CountPattern counts the number of times a pattern occurs in the un-flushed records.
// It does not change the position but this relies on no concurrent logging.
func (tl *TestLogger) CountPattern(rePattern string) int {
	lastID := tl.lastID
	count := 0
	re := regexp.MustCompile(rePattern)
	tl.dump(func(args ...interface{}) {
		if re.MatchString(args[0].(string)) {
			count++
		}
	})
	tl.lastID = lastID
	return count
}

// dump is the internal testable backend beneath Flush
func (tl *TestLogger) dump(showStringFn func(args ...interface{})) {
	show := false
	if tl.lastID == 0 {
		show = true
	}
	for n := tl.lbe.Head(); n != nil; n = n.Next() {
		if show {
			showStringFn(n.Record.Formatted(2))
			tl.lastID = n.Record.ID
		} else {
			if n.Record.ID == tl.lastID {
				show = true
			}
		}
	}
}
