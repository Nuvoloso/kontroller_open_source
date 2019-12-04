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
	reflect "reflect"
	"sort"
)

// StringKeys returns the string keys of a map.
// Bad things will happen if m is not a map with string keys.
func StringKeys(m interface{}) []string {
	mV := reflect.ValueOf(m)
	keys := make([]string, 0, len(mV.MapKeys()))
	for _, kV := range mV.MapKeys() {
		keys = append(keys, kV.String())
	}
	return keys
}

// SortedStringKeys returns the string keys of a map in sorted order.
// Bad things will happen if m is not a map with string keys.
func SortedStringKeys(m interface{}) []string {
	keys := StringKeys(m)
	sort.Strings(keys)
	return keys
}
