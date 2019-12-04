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
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFetchConstantsFromGoFile(t *testing.T) {
	assert := assert.New(t)

	bc, err := FetchBasicConstantsFromGoFile("./introspect.go")
	assert.NoError(err)
	assert.NotNil(bc)

	sMap := bc.GetStringConstants(nil)
	assert.Len(sMap, 1)
	assert.NotContains(sMap, "testConstInt")
	assert.Contains(sMap, "testConstString")
	assert.Equal("string", sMap["testConstString"])

	sMap = bc.GetStringConstants(regexp.MustCompile("test"))
	assert.Len(sMap, 1)
	assert.NotContains(sMap, "testConstInt")
	assert.Contains(sMap, "testConstString")
	assert.Equal("string", sMap["testConstString"])

	iMap := bc.GetIntConstants(nil)
	assert.Len(iMap, 1)
	assert.Contains(iMap, "testConstInt")
	assert.NotContains(iMap, "testConstString")
	assert.Equal(3, iMap["testConstInt"])

	iMap = bc.GetIntConstants(regexp.MustCompile("test"))
	assert.Len(iMap, 1)
	assert.Contains(iMap, "testConstInt")
	assert.NotContains(iMap, "testConstString")
	assert.Equal(3, iMap["testConstInt"])

	bc, err = FetchBasicConstantsFromGoFile("./foo")
	assert.Error(err)
	assert.Nil(bc)

	bc, err = FetchBasicConstantsFromGoFile("./README.md")
	assert.Error(err)
	assert.Nil(bc)
}
