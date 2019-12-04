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
)

// TagList is a map of tag key to value. It is not thread safe.
type TagList struct {
	m        map[string]string
	keyOrder []string
}

// NewTagList converts a tag list to a map, splitting the entries on ":".
// This will collapse duplicate keys by using the last value set and eliminate empty strings.
// Ordering is preserved.
func NewTagList(tags []string) *TagList {
	tm := &TagList{}
	tm.m = make(map[string]string)
	if tags == nil {
		tags = []string{}
	}
	tm.keyOrder = make([]string, 0, len(tags))
	for _, t := range tags {
		if t == "" {
			continue
		}
		kv := strings.SplitN(t, ":", 2)
		var v string
		if len(kv) == 2 {
			v = kv[1]
		}
		k := kv[0]
		if _, exists := tm.m[k]; !exists {
			tm.keyOrder = append(tm.keyOrder, k)
		}
		tm.m[k] = v
	}
	return tm
}

// Len returns the length
func (tm *TagList) Len() int {
	return len(tm.keyOrder)
}

// List creates a tag list from the map with values in the order of creation.
func (tm *TagList) List() []string {
	tags := make([]string, 0, len(tm.keyOrder))
	for _, k := range tm.keyOrder {
		v := tm.m[k]
		t := k
		if v != "" {
			t += ":" + v
		}
		tags = append(tags, t)
	}
	return tags
}

// Keys returns the list of tag keys that match an optional pattern.
func (tm *TagList) Keys(re *regexp.Regexp) []string {
	res := []string{}
	for _, k := range tm.keyOrder {
		if re == nil || re.MatchString(k) {
			res = append(res, k)
		}
	}
	return res
}

// Set inserts or modifies a tag. If inserted the position will be last.
func (tm *TagList) Set(k, v string) {
	if _, exists := tm.m[k]; !exists {
		tm.keyOrder = append(tm.keyOrder, k)
	}
	tm.m[k] = v
}

// Get returns the value of the tag and an indication if it previously existed.
func (tm *TagList) Get(k string) (string, bool) {
	v, exists := tm.m[k]
	return v, exists
}

// Idx returns the key-value in the specified position
func (tm *TagList) Idx(i int) (string, string) {
	if i >= 0 && i < len(tm.keyOrder) {
		return tm.keyOrder[i], tm.m[tm.keyOrder[i]]
	}
	return "", ""
}

// Delete removes a tag
func (tm *TagList) Delete(k string) {
	if _, exists := tm.m[k]; exists {
		delete(tm.m, k)
		nko := make([]string, 0, len(tm.keyOrder)-1)
		for _, ko := range tm.keyOrder {
			if ko != k {
				nko = append(nko, ko)
			}
		}
		tm.keyOrder = nko
	}
}
