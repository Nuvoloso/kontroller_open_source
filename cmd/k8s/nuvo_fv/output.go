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


package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// DriverOutput contains the data needed to output the JSON result a driver command or error
// It supports the subset of the documented fields needed by the nuvo driver.
type DriverOutput struct {
	// Status must be one of Success/Failure/Not supported
	Status DriverStatus `json:"status"`
	// Optional message, eg err.Error() value
	Message string `json:"message,omitempty"`
	// Optional Capabilities, eg in the result of init
	Capabilities map[string]interface{} `json:"capabilities,omitempty"`
}

// DriverOutputOps is an interface for printing driver output
type DriverOutputOps interface {
	Print()
}

// DriverStatus represents the valid flexvolume status values
type DriverStatus int

// Valid DriverStatus values
const (
	// "Success" status
	DriverStatusSuccess DriverStatus = iota
	// "Failure" status
	DriverStatusFailure
	// "Not supported" status
	DriverStatusNotSupported
)

// MarshalText implements the encoding.TextMarshaler interface for DriverStatus
func (s DriverStatus) MarshalText() (text []byte, err error) {
	if s == DriverStatusSuccess {
		return []byte(`Success`), nil
	} else if s == DriverStatusFailure {
		return []byte(`Failure`), nil
	}
	return []byte(`Not supported`), nil
}

// NewDriverOutput returns an initialized DriverOutput object
func NewDriverOutput() *DriverOutput {
	return &DriverOutput{}
}

// PrintDriverOutput prints the JSON-encoded output of the status and optional message (only the 1st message is output)
func PrintDriverOutput(status DriverStatus, message ...string) error {
	o := NewDriverOutput()
	o.Status = status
	if len(message) > 0 {
		o.Message = message[0]
	}
	o.Print()
	return nil
}

func (o *DriverOutput) cleanUpMessage() string {
	cleaned := o.Message
	if strings.ContainsRune(o.Message, '"') {
		cleaned = strings.Replace(o.Message, `"`, `'`, -1)
	}
	return cleaned
}

// Print prints the JSON-encoded output to stdout
// If the Message contains double quotes, they will be printed as single quotes.
// If logWriter is a Buffer, its contents are automatically appended to the message as a "Trace Log"
func (o *DriverOutput) Print() {
	saved := o.Message
	if b, ok := logWriter.(*bytes.Buffer); ok {
		log := b.Bytes()
		if len(log) > 0 {
			o.Message = fmt.Sprintf("%s\nTrace Log:\n%s", o.Message, log)
		}
	}
	o.Message = o.cleanUpMessage()
	m, _ := json.Marshal(o)
	fmt.Fprintf(outputWriter, "%s", m) // no newline!
	o.Message = saved
}
