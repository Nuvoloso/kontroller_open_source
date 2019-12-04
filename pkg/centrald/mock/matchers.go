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


package mock

import (
	"reflect"
	"testing"

	centrald "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/stretchr/testify/assert"
)

// UpdateArgsMatcher is a centrald.UpdateArgs gomock Matcher
type UpdateArgsMatcher struct {
	t            *testing.T // use assert for nice messages
	ua           *centrald.UpdateArgs
	versionAdded int32
}

// NewUpdateArgsMatcher creates a UpdateArgsMatchers
func NewUpdateArgsMatcher(t *testing.T, ua *centrald.UpdateArgs) *UpdateArgsMatcher {
	return &UpdateArgsMatcher{t: t, ua: ua}
}

// AddsVersion causes the matcher to require that no version is specified in the query parameters but is added by the handler
func (o *UpdateArgsMatcher) AddsVersion(v int32) *UpdateArgsMatcher {
	o.versionAdded = v
	return o
}

// Matcher ends the chain (unnecessary - old pattern)
func (o *UpdateArgsMatcher) Matcher() *UpdateArgsMatcher {
	return o
}

// Matches is from gomock.Matcher
func (o *UpdateArgsMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	ua, ok := x.(*centrald.UpdateArgs)
	if !ok {
		assert.True(ok, "argument ISA *centrald.UpdateArgs")
		return false
	}
	if o.ua.ID != ua.ID {
		assert.Equal(o.ua.ID, ua.ID, "UAMatcher: ID same")
		return false
	}
	if o.versionAdded != 0 {
		assert.Zero(o.ua.Version, "UAMatcher: Version should not be specified in params")
		assert.NotZero(ua.Version, "UAMatcher: Version should be added")
	} else if o.ua.Version != ua.Version {
		assert.Equal(o.ua.Version, ua.Version, "UAMatcher: Version same")
		return false
	}
	if o.ua.HasChanges != ua.HasChanges {
		assert.Equal(o.ua.HasChanges, ua.HasChanges, "UAMatcher: HasChanges same")
		return false
	}
	if len(o.ua.Attributes) != len(ua.Attributes) {
		assert.Len(ua.Attributes, len(o.ua.Attributes), "UAMatcher: Attributes length the same")
		return false
	}
	// the array elements are in arbitrary Name order
	for _, oAttr := range o.ua.Attributes {
		var attr *centrald.UpdateAttr
		for _, a := range ua.Attributes {
			if a.Name == oAttr.Name {
				attr = &a
				break
			}
		}
		if attr != nil {
			if !reflect.DeepEqual(oAttr, *attr) {
				assert.Equal(oAttr, *attr)
				return false
			}
		} else {
			assert.NotNil(attr, "UAMatcher: attribute "+oAttr.Name+" present")
			return false
		}
	}
	return true
}

// String is from gomock.Matcher
func (o *UpdateArgsMatcher) String() string {
	return "matches"
}

// MakeStdUpdateArgsObjMatcher is a centrald.AppCrudHelpers.MakeStdUpdateArgs gomock Matcher
// It should be used for all the parameters - it deduces the arg based on type.
type MakeStdUpdateArgsObjMatcher struct {
	t      *testing.T // use assert for nice messages
	obj    interface{}
	id     string
	ver    int32
	fields [centrald.NumActionTypes][]string
}

// NewMakeStdUpdateArgsObjMatcher creates a MakeStdUpdateArgsObjMatcher
func NewMakeStdUpdateArgsObjMatcher(t *testing.T) *MakeStdUpdateArgsObjMatcher {
	return &MakeStdUpdateArgsObjMatcher{t: t}
}

// WithObj sets the object whose type is to be matched
func (o *MakeStdUpdateArgsObjMatcher) WithObj(obj interface{}) *MakeStdUpdateArgsObjMatcher {
	o.obj = obj
	return o
}

// WithID sets the object id
func (o *MakeStdUpdateArgsObjMatcher) WithID(id string) *MakeStdUpdateArgsObjMatcher {
	o.id = id
	return o
}

// WithVersion sets the version if non-zero
func (o *MakeStdUpdateArgsObjMatcher) WithVersion(ver int32) *MakeStdUpdateArgsObjMatcher {
	o.ver = ver
	return o
}

// WithFields sets the fields
func (o *MakeStdUpdateArgsObjMatcher) WithFields(fields [centrald.NumActionTypes][]string) *MakeStdUpdateArgsObjMatcher {
	o.fields = fields
	return o
}

// Matches is from gomock.Matcher
func (o *MakeStdUpdateArgsObjMatcher) Matches(arg interface{}) bool {
	assert := assert.New(o.t)
	switch x := arg.(type) {
	case *int32: // version
		return assert.Equal(o.ver, *x, "MSUA version match")
	case string: // id
		return assert.Equal(o.id, x, "MSUA id match")
	case [centrald.NumActionTypes][]string:
		return assert.Equal(o.fields, x, "MSUA fields match")
	default: // object - only match type
		return assert.IsType(o.obj, x, "MSUA object type match")
	}
}

// String is from gomock.Matcher
func (o *MakeStdUpdateArgsObjMatcher) String() string {
	return "MSUA arg matches"
}
