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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContains(t *testing.T) {
	assert := assert.New(t)

	i := []int{1, 3, 5}
	s := []string{"name", "ipAddress", "clusterId"}
	v := []interface{}{"alpha", 2, true, nil}
	assert.True(Contains(s, "name"))
	assert.False(Contains(s, "description"))
	assert.False(Contains(s, 5))
	assert.False(Contains(s, nil))
	assert.True(Contains(i, 5))
	assert.True(Contains(v, 2))
	assert.False(Contains(v, 1))
	assert.True(Contains(v, nil))
	assert.False(Contains(nil, "a"))
}
