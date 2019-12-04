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
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
)

var errInvalidValueTypeKind = fmt.Errorf("invalid value type Kind")
var errInvalidValueTypeValue = fmt.Errorf("invalid value type Value")

// ValueTypeDuration parses a models.ValueType DURATION
func ValueTypeDuration(vt models.ValueType) (time.Duration, error) {
	if vt.Kind != com.ValueTypeDuration {
		return 0, errInvalidValueTypeKind
	}
	posSuffix := strings.IndexAny(vt.Value, "dhmsuµ")
	if posSuffix < 1 {
		return 0, errInvalidValueTypeValue
	}
	if string(vt.Value[posSuffix:]) == "d" {
		str := string(vt.Value[0:posSuffix]) + "h"
		dur, err := time.ParseDuration(str)
		if err == nil {
			return 24 * dur, nil
		}
		return 0, errInvalidValueTypeValue
	}
	d, err := time.ParseDuration(vt.Value)
	if err != nil {
		return 0, errInvalidValueTypeValue
	}
	return d, nil
}

// ValueTypeInt parses a models.ValueType INT
func ValueTypeInt(vt models.ValueType) (int64, error) {
	if vt.Kind != com.ValueTypeInt {
		return 0, errInvalidValueTypeKind
	}
	n, err := strconv.ParseInt(vt.Value, 0, 64)
	if err != nil {
		return 0, errInvalidValueTypeValue
	}
	return n, nil
}

// ValueTypePercentage parses a models.ValueType PERCENTAGE
func ValueTypePercentage(vt models.ValueType) (float64, error) {
	if vt.Kind != com.ValueTypePercentage {
		return 0, errInvalidValueTypeKind
	}
	var f float64
	if n, err := fmt.Sscanf(vt.Value, "%f%%", &f); err != nil || n != 1 {
		return 0, errInvalidValueTypeValue
	}
	return f, nil
}

// ValueTypeString parses a models.ValueType STRING or SECRET
func ValueTypeString(vt models.ValueType) (string, error) {
	if !(vt.Kind == com.ValueTypeString || vt.Kind == com.ValueTypeSecret) {
		return "", errInvalidValueTypeKind
	}
	return vt.Value, nil
}

// ParseValueType parses a models.ValueType
func ParseValueType(vt models.ValueType, expKind string, defaultValue interface{}) interface{} {
	if vt.Kind != expKind {
		return defaultValue
	}
	var v interface{}
	var err error
	switch vt.Kind {
	case com.ValueTypeDuration:
		v, err = ValueTypeDuration(vt)
	case com.ValueTypeInt:
		v, err = ValueTypeInt(vt)
	case com.ValueTypePercentage:
		v, err = ValueTypePercentage(vt)
	case com.ValueTypeSecret:
		v, err = ValueTypeString(vt)
	case com.ValueTypeString:
		v, err = ValueTypeString(vt)
	default:
		err = errInvalidValueTypeKind
	}
	if err != nil {
		return defaultValue
	}
	return v
}
