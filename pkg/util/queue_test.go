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
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQueue(t *testing.T) {
	assert := assert.New(t)

	type type1 struct {
		i int
	}
	type type2 struct {
		a string
	}

	// Untyped queue
	q := NewQueue(nil)
	assert.NotNil(q)
	assert.Nil(q.elType)
	assert.Nil(q.Type())
	assert.NotNil(q.queue)
	assert.Equal(0, q.Length())
	assert.Nil(q.PeekHead())
	assert.Nil(q.PopHead())
	var n int
	assert.NotPanics(func() { n = q.PopN(100) })
	assert.Equal(0, n)
	assert.NotPanics(func() { n = q.PopN(0) })
	assert.Equal(0, n)
	assert.NotPanics(func() { n = q.PopN(-100) })
	assert.Equal(0, n)

	e1t1 := &type1{}
	e2t1 := &type1{}
	e3t1 := &type1{}
	e1t2 := &type2{}

	assert.NotPanics(func() { q.Append([]interface{}{e1t1, e1t2}) })
	assert.Equal(2, q.Length())
	assert.Equal(e1t1, q.PeekHead())
	assert.Equal(2, q.Length())

	var pt1 *type1
	assert.NotPanics(func() { pt1 = q.PopHead().(*type1) })
	assert.NotNil(pt1)
	assert.Equal(e1t1, pt1)
	assert.Equal(1, q.Length())
	var pt2 *type2
	assert.Equal(e1t2, q.PeekHead())
	assert.NotPanics(func() { pt2 = q.PopHead().(*type2) })
	assert.NotNil(pt2)
	assert.Equal(e1t2, pt2)
	assert.Equal(0, q.Length())

	assert.NotPanics(func() { q.Add(e3t1) })
	assert.Equal(e3t1, q.PeekHead())
	assert.Equal(1, q.Length())
	assert.NotPanics(func() { q.Add("abc") })
	assert.Equal(2, q.Length())
	n = q.PopN(100)
	assert.Equal(2, n)
	assert.Equal(0, q.Length())

	assert.NotPanics(func() { q.Add([]*type1{e1t1, e2t1}) })
	assert.Equal(1, q.Length()) // slice is the element
	assert.Equal([]*type1{e1t1, e2t1}, q.PopHead())

	// Typed queue
	q = NewQueue(e1t1)
	assert.NotNil(q)
	assert.Equal(reflect.TypeOf(e1t1), q.elType)
	assert.Equal(reflect.TypeOf(e1t1), q.Type())
	assert.NotNil(q.queue)
	assert.Equal(0, q.Length())

	aT1 := []*type1{e1t1, e2t1} // slice of type
	assert.NotPanics(func() { q.Add(aT1) })
	assert.Equal(2, q.Length())
	assert.Equal(e1t1, q.PeekHead())
	assert.Equal(2, q.Length())

	assert.Panics(func() { q.Append([]interface{}{e3t1, e1t2}) })
	assert.Equal(2, q.Length())
	assert.Panics(func() { q.Append([]interface{}{type1{}}) }) // not a pointer
	assert.Equal(2, q.Length())
	assert.Panics(func() { q.Add(e1t2) })
	assert.Equal(2, q.Length())

	assert.NotPanics(func() { q.Add(e3t1) }) // singleton
	assert.Equal(3, q.Length())
	assert.Equal(e1t1, q.PopHead())
	assert.Equal(e2t1, q.PopHead())
	pt1 = q.PopHead().(*type1)
	assert.Equal(e1t1, pt1)

	assert.NotPanics(func() { q.Append([]interface{}{e1t1, e2t1}) })
	assert.Equal(2, q.Length())
	assert.NotPanics(func() { n = q.PopN(100) })
	assert.Equal(2, n)
	assert.Equal(0, q.Length())
	assert.Nil(q.PeekHead())
	assert.Nil(q.PopHead())
}
