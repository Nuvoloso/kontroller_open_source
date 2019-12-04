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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitExecute(t *testing.T) {
	assert := assert.New(t)
	var b bytes.Buffer
	outputWriter = &b

	i := initCmd{}
	assert.NoError(i.Execute([]string{}))
	assert.True(b.Len() > 0)
	o := &map[string]interface{}{}
	assert.NoError(json.Unmarshal(b.Bytes(), o))
	if assert.Contains(*o, "status") {
		assert.Equal("Success", (*o)["status"])
	}
	assert.NotContains(*o, "message")
	assert.Contains(*o, "capabilities")
}
