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

	"github.com/stretchr/testify/assert"
)

func TestDataStoreError(t *testing.T) {
	assert := assert.New(t)

	e1 := Error{"test", 1}
	assert.Equal("test", e1.Error())
}

func TestUpdateModified(t *testing.T) {
	assert := assert.New(t)

	es := struct{}{}
	emptyFields := make(map[string]struct{}, 0)
	setFields := map[string]struct{}{"f": es}
	emptyIndex := make(map[int]struct{}, 0)
	setIndex := map[int]struct{}{1: es}
	ua := UpdateArgs{
		Attributes: []UpdateAttr{
			{
				Name: "unmodified",
				Actions: [NumActionTypes]UpdateActionArgs{
					UpdateSet: UpdateActionArgs{
						Fields:  emptyFields,
						Indexes: emptyIndex,
					},
				},
			},
			{
				Name: "fromBodyRemove",
				Actions: [NumActionTypes]UpdateActionArgs{
					UpdateRemove: UpdateActionArgs{
						FromBody: true,
						Fields:   emptyFields,
						Indexes:  emptyIndex,
					},
				},
			},
			{
				Name: "fromBodyAppend",
				Actions: [NumActionTypes]UpdateActionArgs{
					UpdateAppend: UpdateActionArgs{
						FromBody: true,
						Fields:   emptyFields,
						Indexes:  emptyIndex,
					},
				},
			},
			{
				Name: "fromBodySet",
				Actions: [NumActionTypes]UpdateActionArgs{
					UpdateSet: UpdateActionArgs{
						FromBody: true,
						Fields:   emptyFields,
						Indexes:  emptyIndex,
					},
				},
			},
			{
				Name: "fieldsRemove",
				Actions: [NumActionTypes]UpdateActionArgs{
					UpdateRemove: UpdateActionArgs{
						Fields:  setFields,
						Indexes: emptyIndex,
					},
				},
			},
			{
				Name: "fieldsAppend",
				Actions: [NumActionTypes]UpdateActionArgs{
					UpdateAppend: UpdateActionArgs{
						Fields:  setFields,
						Indexes: emptyIndex,
					},
				},
			},
			{
				Name: "fieldsSet",
				Actions: [NumActionTypes]UpdateActionArgs{
					UpdateSet: UpdateActionArgs{
						Fields:  setFields,
						Indexes: emptyIndex,
					},
				},
			},
			{
				Name: "indexRemove",
				Actions: [NumActionTypes]UpdateActionArgs{
					UpdateRemove: UpdateActionArgs{
						Fields:  emptyFields,
						Indexes: setIndex,
					},
				},
			},
			{
				Name: "indexAppend",
				Actions: [NumActionTypes]UpdateActionArgs{
					UpdateAppend: UpdateActionArgs{
						Fields:  emptyFields,
						Indexes: setIndex,
					},
				},
			},
			{
				Name: "indexSet",
				Actions: [NumActionTypes]UpdateActionArgs{
					UpdateSet: UpdateActionArgs{
						Fields:  emptyFields,
						Indexes: setIndex,
					},
				},
			},
		},
	}

	modified := []string{
		"fromBodyRemove", "fromBodyAppend", "fromBodySet",
		"fieldsRemove", "fieldsAppend", "fieldsSet",
		"indexRemove", "indexAppend", "indexSet",
	}
	for _, n := range modified {
		t.Log("isModified", n)
		assert.True(ua.IsModified(n))
		assert.True(ua.OthersModified(n))
	}
	assert.True(ua.AnyModified(modified...))
	assert.False(ua.OthersModified(modified...))

	unmodified := []string{"unmodified", "notpresent"}
	for _, n := range unmodified {
		t.Log("!isModified", n)
		assert.False(ua.IsModified(n))
		assert.False(ua.AnyModified(n))
	}
	assert.False(ua.AnyModified(unmodified...))
	assert.True(ua.OthersModified(unmodified...))

	assert.False(ua.AnyModified())
	assert.True(ua.OthersModified())

	for _, n := range modified {
		t.Log("FindUpdateAttr", n)
		assert.NotNil(ua.FindUpdateAttr(n))
	}
	assert.Nil(ua.FindUpdateAttr("notpresent"))

	t.Log("Empty ua")
	ua = UpdateArgs{}
	assert.False(ua.IsModified("anything"))
	assert.Nil(ua.FindUpdateAttr("anything"))
	assert.False(ua.AnyModified())
	assert.False(ua.OthersModified())
}
