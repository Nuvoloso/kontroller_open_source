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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExec(t *testing.T) {
	assert := assert.New(t)

	e := NewExec()
	assert.NotNil(e)

	assert.Panics(func() { e.CommandContext(nil, "command") }, "nil context panics")

	cmd := e.CommandContext(context.Background(), "no_such_command_i_hope", "arg1", "arg2")
	assert.NotNil(cmd)
	output, err := cmd.CombinedOutput()
	assert.Len(output, 0)
	assert.NotNil(err)
	assert.True(e.IsNotFoundError(err))
	rc, ok := e.IsExitError(err)
	assert.False(ok)

	cmd = e.CommandContext(context.Background(), "false")
	assert.NotNil(cmd)
	output, err = cmd.CombinedOutput()
	assert.Len(output, 0)
	assert.NotNil(err)
	assert.False(e.IsNotFoundError(err))
	rc, ok = e.IsExitError(err)
	assert.True(ok)
	assert.True(rc > 0)

	cmd = e.CommandContext(context.Background(), "true")
	assert.NotNil(cmd)
	output, err = cmd.CombinedOutput()
	assert.Len(output, 0)
	assert.Nil(err)
}
