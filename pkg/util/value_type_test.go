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
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/stretchr/testify/assert"
)

func TestValueTypeParser(t *testing.T) {
	assert := assert.New(t)

	d, err := ValueTypeDuration(models.ValueType{Kind: "DURATION", Value: "3h"})
	assert.NoError(err)
	assert.Equal(3*time.Hour, d)
	d, err = ValueTypeDuration(models.ValueType{Kind: "DURATION", Value: "3.5h"})
	assert.NoError(err)
	assert.Equal(210*time.Minute, d)
	d, err = ValueTypeDuration(models.ValueType{Kind: "DURATION", Value: "3m"})
	assert.NoError(err)
	assert.Equal(3*time.Minute, d)
	d, err = ValueTypeDuration(models.ValueType{Kind: "DURATION", Value: "3.5s"})
	assert.NoError(err)
	assert.Equal(3500*time.Millisecond, d)
	d, err = ValueTypeDuration(models.ValueType{Kind: "DURATION", Value: "3ms"})
	assert.NoError(err)
	assert.Equal(3*time.Millisecond, d)
	d, err = ValueTypeDuration(models.ValueType{Kind: "DURATION", Value: "3us"})
	assert.NoError(err)
	assert.Equal(3*time.Microsecond, d)
	d, err = ValueTypeDuration(models.ValueType{Kind: "DURATION", Value: "3µs"})
	assert.NoError(err)
	assert.Equal(3*time.Microsecond, d)
	d, err = ValueTypeDuration(models.ValueType{Kind: "DURATION", Value: "90d"})
	assert.NoError(err)
	assert.Equal(90*24*time.Hour, d)
	d, err = ValueTypeDuration(models.ValueType{Kind: "DURATION", Value: "90.5d"})
	assert.NoError(err)
	assert.Equal((90*24+12)*time.Hour, d)
	d, err = ValueTypeDuration(models.ValueType{Kind: "notDURATION", Value: "90d"})
	assert.Error(err)
	assert.Equal(errInvalidValueTypeKind, err)
	assert.EqualValues(0, d)
	d, err = ValueTypeDuration(models.ValueType{Kind: "DURATION", Value: "d"})
	assert.Error(err)
	assert.Equal(errInvalidValueTypeValue, err)
	assert.EqualValues(0, d)
	d, err = ValueTypeDuration(models.ValueType{Kind: "DURATION", Value: "bad"})
	assert.Error(err)
	assert.Equal(errInvalidValueTypeValue, err)
	assert.EqualValues(0, d)
	d, err = ValueTypeDuration(models.ValueType{Kind: "DURATION", Value: "bah"})
	assert.Error(err)
	assert.Equal(errInvalidValueTypeValue, err)
	assert.EqualValues(0, d)

	i, err := ValueTypeInt(models.ValueType{Kind: "INT", Value: "12345"})
	assert.NoError(err)
	assert.EqualValues(12345, i)
	i, err = ValueTypeInt(models.ValueType{Kind: "notINT", Value: "12345"})
	assert.Error(err)
	assert.Equal(errInvalidValueTypeKind, err)
	assert.EqualValues(0, i)
	i, err = ValueTypeInt(models.ValueType{Kind: "INT", Value: "not an int"})
	assert.Error(err)
	assert.Equal(errInvalidValueTypeValue, err)
	assert.EqualValues(0, i)

	s, err := ValueTypeString(models.ValueType{Kind: "STRING", Value: "A String"})
	assert.NoError(err)
	assert.Equal("A String", s)
	s, err = ValueTypeString(models.ValueType{Kind: "notSTRING", Value: "A String"})
	assert.Error(err)
	assert.Equal(errInvalidValueTypeKind, err)
	s, err = ValueTypeString(models.ValueType{Kind: "SECRET", Value: "A Secret"})
	assert.NoError(err)
	assert.Equal("A Secret", s)
	s, err = ValueTypeString(models.ValueType{Kind: "notSECRET", Value: "A Secret"})
	assert.Error(err)
	assert.Equal(errInvalidValueTypeKind, err)

	p, err := ValueTypePercentage(models.ValueType{Kind: "PERCENTAGE", Value: "12.5%"})
	assert.NoError(err)
	assert.EqualValues(12.5, p)
	p, err = ValueTypePercentage(models.ValueType{Kind: "notPERCENTAGE", Value: "12.5%"})
	assert.Error(err)
	assert.Equal(errInvalidValueTypeKind, err)
	p, err = ValueTypePercentage(models.ValueType{Kind: "PERCENTAGE", Value: "12.5"})
	assert.Error(err)
	assert.Equal(errInvalidValueTypeValue, err)
	assert.EqualValues(0.0, p)
	p, err = ValueTypePercentage(models.ValueType{Kind: "PERCENTAGE", Value: "abc%"})
	assert.Error(err)
	assert.Equal(errInvalidValueTypeValue, err)
	assert.EqualValues(0.0, p)

	// defaulting style
	d = ParseValueType(models.ValueType{Kind: "DURATION", Value: "90d"}, com.ValueTypeDuration, time.Duration(10)).(time.Duration)
	assert.Equal(90*24*time.Hour, d)
	d = ParseValueType(models.ValueType{Kind: "DURATION", Value: "bad"}, com.ValueTypeDuration, time.Duration(10)).(time.Duration)
	assert.EqualValues(10, d)
	i = ParseValueType(models.ValueType{Kind: "INT", Value: "12345"}, com.ValueTypeInt, int64(-1)).(int64)
	assert.EqualValues(12345, i)
	i = ParseValueType(models.ValueType{Kind: "INT", Value: "notInt"}, com.ValueTypeInt, int64(-1)).(int64)
	assert.EqualValues(-1, i)
	s = ParseValueType(models.ValueType{Kind: "STRING", Value: "A String"}, com.ValueTypeString, "").(string)
	assert.Equal("A String", s)
	s = ParseValueType(models.ValueType{Kind: "SECRET", Value: "A Secret"}, com.ValueTypeSecret, "").(string)
	assert.Equal("A Secret", s)
	p = ParseValueType(models.ValueType{Kind: "PERCENTAGE", Value: "12.5%"}, com.ValueTypePercentage, float64(24.0)).(float64)
	assert.EqualValues(12.5, p)
	p = ParseValueType(models.ValueType{Kind: "PERCENTAGE", Value: "bad%"}, com.ValueTypePercentage, float64(24.0)).(float64)
	assert.EqualValues(24.0, p)
	s = ParseValueType(models.ValueType{Kind: "WRONG KIND", Value: "A String"}, com.ValueTypeString, "foo").(string)
	assert.Equal("foo", s)
	s = ParseValueType(models.ValueType{Kind: "BAD KIND", Value: "A String"}, "BAD KIND", "foo").(string)
	assert.Equal("foo", s)
}
