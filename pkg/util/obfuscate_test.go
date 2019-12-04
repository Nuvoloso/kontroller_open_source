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
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestObfuscate tests both Obfuscate and DeObfuscate
func TestObfuscate(t *testing.T) {
	assert := assert.New(t)

	assert.Len(obfPrefix, 4)

	// empty string cases
	ret := Obfuscate("")
	assert.Empty(ret)

	ret, err := DeObfuscate(obfPrefix)
	assert.Empty(ret)
	assert.NoError(err)

	ret, err = DeObfuscate("")
	assert.Empty(ret)
	assert.NoError(err)

	// missing prefix is a no-op
	s := "random"
	ret, err = DeObfuscate(s)
	assert.Equal(s, ret)
	assert.NoError(err)

	// round trip
	obf := Obfuscate(s)
	t.Log(obf)
	ret, err = DeObfuscate(obf)
	assert.Equal(s, ret)
	assert.NotContains(obf, ret)
	assert.NoError(err)

	// some non printables
	s = string([]byte{0, 1, 2, 3, 2, 1, 0})
	obf = Obfuscate(s)
	t.Log(obf)
	ret, err = DeObfuscate(obf)
	assert.Equal(s, ret)
	assert.NotContains(obf, ret)
	assert.NoError(err)

	// these 2 tests failed before doubling the encoded range to support unsigned byte (original algorithm was for Java, where byte is signed)
	s = string([]byte{0, 255})
	obf = Obfuscate(s)
	t.Log(obf)
	ret, err = DeObfuscate(obf)
	assert.Equal(s, ret)
	assert.NotContains(obf, ret)
	assert.NoError(err)

	s = string([]byte{255, 0})
	obf = Obfuscate(s)
	t.Log(obf)
	ret, err = DeObfuscate(obf)
	assert.Equal(s, ret)
	assert.NotContains(obf, ret)
	assert.NoError(err)

	// smallest value
	s = string([]byte{0})
	obf = Obfuscate(s)
	assert.Len(obf, 8)
	t.Log(obf)
	ret, err = DeObfuscate(obf)
	assert.Equal(s, ret)
	assert.NotContains(obf, ret)
	assert.NoError(err)

	// largest value
	s = string([]byte{255})
	obf = Obfuscate(s)
	assert.Len(obf, 8)
	t.Log(obf)
	ret, err = DeObfuscate(obf)
	assert.Equal(s, ret)
	assert.NotContains(obf, ret)
	assert.NoError(err)

	// invalid string
	ret, err = DeObfuscate(obfPrefix + ":")
	assert.Empty(ret)
	assert.Error(err)
	_, ok := err.(*strconv.NumError)
	assert.True(ok)
}
