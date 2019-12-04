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


package centrald

import (
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/stretchr/testify/assert"
)

func TestClusterTypes(t *testing.T) {
	assert := assert.New(t)

	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds})

	cts := app.SupportedClusterTypes()
	assert.NotNil(cts)
	assert.NotEmpty(cts)
	for _, ct := range cts {
		assert.True(ct != "")
		assert.True(app.ValidateClusterType(ct))
		assert.False(app.ValidateClusterType(ct + "FOO"))
	}
}

func TestNodeAttributes(t *testing.T) {
	assert := assert.New(t)

	ds := NewFakeDataStore()
	app := AppInit(&AppArgs{DS: ds})

	cts := app.SupportedClusterTypes()
	assert.NotNil(cts)
	ct := cts[0]

	validNA := map[string]models.ValueType{
		"str":    models.ValueType{Kind: "STRING", Value: "a string"},
		"int":    models.ValueType{Kind: "INT", Value: "1"},
		"secret": models.ValueType{Kind: "SECRET", Value: "secret"},
	}
	err := app.ValidateNodeAttributes(ct, validNA)
	assert.Nil(err)
	err = app.ValidateNodeAttributes(ct, map[string]models.ValueType{"x": models.ValueType{Kind: "INT", Value: "not int"}})
	assert.NotNil(err)
}
