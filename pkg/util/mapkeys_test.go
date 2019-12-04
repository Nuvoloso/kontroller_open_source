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

func TestSortedStringKeys(t *testing.T) {
	assert := assert.New(t)

	m1 := map[string]int{
		"one":   1,
		"two":   2,
		"three": 3,
	}
	assert.Equal([]string{"one", "three", "two"}, SortedStringKeys(m1))

	m2 := map[string]struct{}{
		"1": struct{}{},
		"2": struct{}{},
		"3": struct{}{},
	}
	assert.Equal([]string{"1", "2", "3"}, SortedStringKeys(m2))

	m2 = nil
	assert.Equal([]string{}, SortedStringKeys(m2))
}
