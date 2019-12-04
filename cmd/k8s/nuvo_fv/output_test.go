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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMarshalText(t *testing.T) {
	assert := assert.New(t)

	d := DriverStatusSuccess
	b, err := d.MarshalText()
	assert.NoError(err)
	assert.Equal("Success", string(b))

	d = DriverStatusFailure
	b, err = d.MarshalText()
	assert.NoError(err)
	assert.Equal("Failure", string(b))

	d = DriverStatusNotSupported
	b, err = d.MarshalText()
	assert.NoError(err)
	assert.Equal("Not supported", string(b))
}

func TestPrint(t *testing.T) {
	assert := assert.New(t)

	var b bytes.Buffer
	outputWriter = &b
	o := NewDriverOutput()
	o.Status = DriverStatusSuccess
	// do not support the attachable interface methods
	o.Message = `Test message with it"s double quote`
	o.Print()
	assert.Regexp("it's", b.String())

	b.Reset()
	var l bytes.Buffer
	logWriter = &l
	l.WriteString("this should be appended")
	o.Print()
	assert.Regexp("Test message.*Trace Log.*should be appended", b.String())
}
