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
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTagList(t *testing.T) {
	assert := assert.New(t)

	tags := []string{
		"k1:v1orig",
		"k2",
		"",
		"k3:v3",
		"k4:",
		"k1:v1",
	}
	tl := NewTagList(tags)
	assert.Equal(4, tl.Len())
	tcs1 := []struct {
		k, v   string
		exists bool
	}{
		{"k1", "v1", true},
		{"k2", "", true},
		{"k3", "v3", true},
		{"k4", "", true},
		{"k5", "", false},
	}
	for i, tc := range tcs1 {
		v, exists := tl.Get(tc.k)
		assert.Equal(tc.exists, exists, "case %d: %s", i, tc.k)
		assert.Equal(tc.v, v, "case %d: %s", i, tc.k)
	}
	k, v := tl.Idx(-1)
	assert.Equal("", k)
	assert.Equal("", v)
	k, v = tl.Idx(len(tcs1))
	assert.Equal("", k)
	assert.Equal("", v)
	expTags := []string{
		"k1:v1",
		"k2",
		"k3:v3",
		"k4",
	}
	assert.Equal(expTags, tl.List())
	for i, tag := range expTags {
		k, v := tl.Idx(i)
		f := strings.SplitN(tag, ":", 2)
		assert.Equal(f[0], k, "case %d: %s", i)
		if len(f) == 2 {
			assert.Equal(f[1], v, "case %d: %s", i)
		} else {
			assert.Equal("", v, "case %d: %s", i)
		}
	}
	assert.Equal([]string{"k1", "k2", "k3", "k4"}, tl.Keys(nil))
	assert.Equal([]string{"k1", "k2", "k3"}, tl.Keys(regexp.MustCompile("k[1-3]")))
	tl.Set("k3", "v3new")
	v, exists := tl.Get("k3")
	assert.True(exists)
	assert.Equal("v3new", v)
	tl.Set("k5", "v5")
	v, exists = tl.Get("k5")
	assert.True(exists)
	assert.Equal("v5", v)
	tl.Set("k6", "v6")
	tl.Set("7k", "")
	tl.Delete("k5")
	tl.Delete("k2")
	tl.Set("2k", "v2")
	tl.Set("x.y.z", "val")
	expTags = []string{
		"k1:v1",
		"k3:v3new",
		"k4",
		"k6:v6",
		"7k",
		"2k:v2",
		"x.y.z:val",
	}
	assert.Equal(expTags, tl.List())
	assert.Equal([]string{"k1", "k3", "k4", "k6", "7k", "2k", "x.y.z"}, tl.Keys(nil))
	for _, k := range tl.Keys(regexp.MustCompile("^k")) {
		tl.Delete(k)
	}
	assert.Equal(3, tl.Len())
	_, exists = tl.Get("7k")
	assert.True(exists)
	_, exists = tl.Get("2k")
	assert.True(exists)
	_, exists = tl.Get("x.y.z")
	assert.True(exists)
	prefix := "x.y."
	pat := regexp.MustCompile("^" + strings.Replace(prefix, ".", "\\.", -1))
	assert.Equal([]string{"x.y.z"}, tl.Keys(pat))

	tl = NewTagList(nil)
	assert.Equal(0, tl.Len())
}
