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
	"runtime/debug"

	"github.com/op/go-logging"
)

// PanicFunc defines a function that can be passed to PanicLogger. It takes no arguments and returns nothing.
type PanicFunc func()

// RecoverFunc defines a function to recover from the panic. The panic argument (the value passed as panic(arg)) and stack is passed
type RecoverFunc func(panicArg interface{}, stack []byte)

// PanicLogger is meant to be used as the entrypoint for a goroutine. It calls the provided PanicFunc which might panic.
// It recovers after a panic and logs the stack at Critical level. The caller must be sure to pass a valid logger.
// The optional recoverFunc (max 1, others are ignored) is called after the stack is logged.
func PanicLogger(log *logging.Logger, f PanicFunc, recoverFunc ...RecoverFunc) {
	defer func() {
		if r := recover(); r != nil {
			b := debug.Stack()
			log.Criticalf("PANIC: %v\n\n%s", r, bytes.TrimSpace(b))
			if len(recoverFunc) > 0 {
				recoverFunc[0](r, b)
			}
		}
	}()
	f()
}
