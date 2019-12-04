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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClone(t *testing.T) {
	assert := assert.New(t)

	assert.Panics(func() { Clone(nil, nil) })

	type Type1 struct {
		S  string
		I  int
		IP *int
		AS []string
	}
	type Type2 struct {
		T1 *Type1
	}

	var i = int(20)
	var srcT2 = Type2{
		T1: &Type1{
			S:  "astring",
			I:  5,
			IP: &i,
			AS: []string{"hello", "there"},
		},
	}
	var dstT2 Type2
	assert.Panics(func() { Clone(srcT2, nil) })
	assert.Panics(func() { Clone(srcT2, dstT2) })
	assert.NotPanics(func() { Clone(srcT2, &dstT2) })

	assert.Equal(srcT2, dstT2)
	i++
	assert.NotEqual(srcT2, dstT2)
	assert.Equal(1, *srcT2.T1.IP-*dstT2.T1.IP)
	assert.Equal(srcT2.T1.AS, dstT2.T1.AS)
	dstT2.T1.AS = append(dstT2.T1.AS, "!")
	assert.NotEqual(srcT2.T1.AS, dstT2.T1.AS)
	assert.Len(dstT2.T1.AS, len(srcT2.T1.AS)+1)

	srcMT1 := map[string]*Type1{
		"foo": &Type1{
			S:  "astring",
			I:  5,
			IP: &i,
			AS: []string{"hello", "there"},
		},
		//"bar": nil,  // Note: Cannot be nil!
		"bar": &Type1{},
	}
	var dstMT1 map[string]*Type1
	assert.NotPanics(func() { Clone(srcMT1, &dstMT1) })
	assert.Equal(srcMT1, dstMT1)
	assert.Len(srcMT1, len(dstMT1))
	assert.Equal(*srcMT1["foo"].IP, *dstMT1["foo"].IP)
	i++
	assert.NotEqual(*srcMT1["foo"].IP, *dstMT1["foo"].IP)
}
