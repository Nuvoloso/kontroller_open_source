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
	"strings"
	"time"

	"github.com/go-openapi/strfmt"
)

// TimeMaxUsefulUpperBound is the largest "useful" max time represented by time.Time.
// See https://stackoverflow.com/questions/25065055/what-is-the-maximum-time-time-in-go.
func TimeMaxUsefulUpperBound() time.Time {
	return time.Unix(1<<63-62135596801, 999999999)
}

// DateTimeMaxUpperBound returns the largest time that can be represented in strfmt.DateTime
// and handled within the internal test cloning framework.
func DateTimeMaxUpperBound() time.Time {
	return time.Date(9999, time.December, 31, 23, 59, 59, 999999999, time.UTC).Truncate(time.Millisecond)
}

// Timestamp supports the go-flags Marshaler and Unmarshaler interfaces and handles parsing both extended Duration and RFC3339 times
type Timestamp struct {
	raw      string
	absolute strfmt.DateTime
	relative time.Duration
}

// Specified determines if the Timestamp value was specified in the flags
func (t *Timestamp) Specified() bool {
	return t.raw != ""
}

// String returns the string representation of the object
func (t *Timestamp) String() string {
	if t.raw != "" {
		return t.raw
	}
	return t.absolute.String()
}

// Value returns the current value of the timestamp.
// In the case the timestamp was specified as a duration, the returned timestamp changes over time, relative to the current time.
// Otherwise, the same absolute timestamp is returned every call.
func (t *Timestamp) Value() strfmt.DateTime {
	if time.Time(t.absolute).IsZero() {
		return strfmt.DateTime(time.Now().Add(-t.relative))
	}
	return t.absolute
}

// MarshalFlag implements the go-flags.Marshaler interface. This implementation never fails
func (t *Timestamp) MarshalFlag() (string, error) {
	return t.String(), nil
}

// UnmarshalFlag implements the go-flags.Unmarshaler interface
func (t *Timestamp) UnmarshalFlag(value string) error {
	var err error
	t.raw = value // save so MarshalFlag can output the exact string that was parsed
	// convenient that RFC3339 does not contain letters other than "T" and "Z"
	if posSuffix := strings.IndexAny(value, "dhmsuµ"); posSuffix >= 0 {
		var dur time.Duration
		if value[posSuffix:] == "d" {
			str := value[0:posSuffix] + "h"
			dur, err = time.ParseDuration(str)
			dur *= 24
		} else {
			dur, err = time.ParseDuration(value)
		}
		t.relative, t.absolute = dur, strfmt.DateTime{}
	} else {
		var tmp time.Time
		tmp, err = time.Parse(time.RFC3339, value)
		t.relative, t.absolute = 0, strfmt.DateTime(tmp)
	}
	return err
}
