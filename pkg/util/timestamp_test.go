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
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/strfmt"
	"github.com/stretchr/testify/assert"
)

func TestTimeUpperBounds(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	maxTime := TimeMaxUsefulUpperBound()
	assert.True(now.Before(maxTime))
	assert.True(maxTime.Equal(maxTime))

	maxDateTime := DateTimeMaxUpperBound()
	assert.True(now.Before(maxDateTime))
	assert.True(maxDateTime.Equal(maxDateTime))

	// API data structure clone test
	vsr1 := &models.VolumeSeriesRequest{}
	vsr1.CompleteByTime = strfmt.DateTime(maxDateTime)
	var vsr2 *models.VolumeSeriesRequest
	testutils.Clone(vsr1, &vsr2)
	assert.Equal(vsr1.CompleteByTime, vsr2.CompleteByTime)

	// demonstrate clone failure beyond this value
	vsr1.CompleteByTime = strfmt.DateTime(maxDateTime.Add(time.Millisecond))
	var vsr3 *models.VolumeSeriesRequest
	assert.Panics(func() { testutils.Clone(vsr1, &vsr3) })
}

func TestTimestamp(t *testing.T) {
	assert := assert.New(t)

	var ts Timestamp

	assert.False(ts.Specified())
	_, err := ts.MarshalFlag()
	assert.NoError(err)

	assert.Error(ts.UnmarshalFlag("BadDuration"))
	assert.Error(ts.UnmarshalFlag("3u"))
	assert.Error(ts.UnmarshalFlag("3µ"))
	assert.Error(ts.UnmarshalFlag("2006-12-12Z"))

	v := "2018-10-29T15:54:51.867Z"
	assert.NoError(ts.UnmarshalFlag(v))
	assert.Equal(v, ts.raw)
	assert.Equal(v, ts.Value().String())
	assert.Zero(ts.relative)
	ret, err := ts.MarshalFlag()
	assert.NoError(err)
	assert.Equal(v, ret)
	time.Sleep(10 * time.Millisecond)
	assert.Equal(v, ts.Value().String())

	type sd struct {
		suf string
		dur time.Duration
	}
	for _, s := range []sd{
		{"d", 24 * time.Hour},
		{"h", time.Hour},
		{"m", time.Minute},
		{"s", time.Second},
		{"ms", time.Millisecond},
		{"us", time.Microsecond},
		{"µs", time.Microsecond},
	} {
		now := time.Now()
		assert.NoError(ts.UnmarshalFlag("3" + s.suf))
		assert.Equal("3"+s.suf, ts.raw)
		assert.Equal(3*s.dur, ts.relative)
		assert.Zero(ts.absolute)
		v1 := ts.Value()
		assert.True(time.Time(v1).After(now.Add(-3 * s.dur)))
		time.Sleep(10 * time.Millisecond)
		assert.True(time.Time(ts.Value()).After(time.Time(v1)))
		assert.NoError(ts.UnmarshalFlag("3.5" + s.suf))
		assert.Equal("3.5"+s.suf, ts.raw)
		assert.Equal(35*s.dur/10, ts.relative)
		assert.Zero(ts.absolute)
		assert.True(time.Time(ts.Value()).After(now.Add(-35 * s.dur / 10)))
	}
}
