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
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/go-openapi/strfmt"
	"github.com/stretchr/testify/assert"
)

func TestMsgList(t *testing.T) {
	assert := assert.New(t)

	tsl := []*models.TimestampedString{
		{Message: "msg 1"},
		{Message: "msg 2"},
	}
	tsl = NewMsgList(tsl).WithTimestamp(time.Now()).
		Insert("msg-with-ts %d %s", 1, "A").
		Insert("msg-with-ts %d %s", 2, "A").ToModel()
	tsl = NewMsgList(tsl).
		Insert("msg-with-default-ts %d", 1).
		Insert("msg-with-default-ts %d", 2).ToModel()

	assert.Equal("msg 1", tsl[0].Message)
	assert.Equal("msg 2", tsl[1].Message)
	assert.Equal(tsl[0].Time, tsl[1].Time)
	assert.Equal("msg-with-ts 1 A", tsl[2].Message)
	assert.Equal("msg-with-ts 2 A", tsl[3].Message)
	assert.Equal(tsl[2].Time, tsl[3].Time)
	assert.Equal("msg-with-default-ts 1", tsl[4].Message)
	assert.Equal("msg-with-default-ts 2", tsl[5].Message)
	assert.Equal(tsl[4].Time, tsl[5].Time)
	defaultTS := tsl[5].Time

	tsl = NewMsgList(tsl).WithTimestamp(time.Now()).InsertWithRepeatCount("msg A", 2).ToModel()
	assert.Equal("msg A", tsl[6].Message) // appended new
	tsl = NewMsgList(tsl).WithTimestamp(time.Now()).InsertWithRepeatCount("msg A", 2).ToModel()
	assert.Equal("msg A", tsl[7].Message)
	assert.NotEqual(tsl[6].Time, tsl[7].Time) // appended new
	tsl = NewMsgList(tsl).WithTimestamp(time.Now()).InsertWithRepeatCount("msg A", 2).ToModel()
	assert.Equal("msg A (repeated 1)", tsl[8].Message) // last updated with appendix 'repeated 1', new ts
	assert.NotEqual(tsl[7].Time, tsl[8].Time)
	tsl = NewMsgList(tsl).WithTimestamp(time.Now()).InsertWithRepeatCount("msg A", 2).ToModel()
	assert.Equal("msg A (repeated 2)", tsl[8].Message) // last updated with 'repeated' count, new ts
	assert.NotEqual(tsl[7].Time, tsl[8].Time)
	assert.NotEqual(tsl[6].Time, tsl[8].Time)

	tsB := time.Now()
	tsl = NewMsgList(tsl).WithTimestamp(tsB).InsertWithRepeatCount("msg B", 0).ToModel()
	assert.Equal("msg B", tsl[9].Message) // appended new
	tsB1 := time.Now()
	tsl = NewMsgList(tsl).WithTimestamp(tsB1).InsertWithRepeatCount("msg B", 0).ToModel()
	assert.Equal("msg B (repeated 1)", tsl[9].Message) // last updated with appendix 'repeated 1', new ts
	assert.Equal(strfmt.DateTime(tsB1), tsl[9].Time)
	tsB2 := time.Now()
	tsl = NewMsgList(tsl).WithTimestamp(tsB2).InsertWithRepeatCount("msg B", 0).ToModel()
	assert.Equal("msg B (repeated 2)", tsl[9].Message) // last updated with 'repeated' count, new ts
	assert.Equal(strfmt.DateTime(tsB2), tsl[9].Time)

	assert.Len(tsl, 10)

	cntByTS := make(map[strfmt.DateTime]int)
	for _, tss := range tsl {
		n, present := cntByTS[tss.Time]
		if present {
			n++
		} else {
			n = 1
		}
		cntByTS[tss.Time] = n
	}
	for ts, n := range cntByTS {
		if defaultTS == ts || strings.Contains(ts.String(), "0001-01-01") || ts == tsl[2].Time {
			assert.Equal(2, n)
		} else {
			assert.Equal(1, n)
		}
	}
	assert.Len(cntByTS, 7)

	tsl = NewMsgList(nil).Insert("empty-list-msg").ToModel()
	assert.Len(tsl, 1)
	assert.Equal("empty-list-msg", tsl[0].Message)

	tsl = NewMsgList(nil).InsertWithRepeatCount("empty-list-msg", 2).ToModel()
	assert.Len(tsl, 1)
	assert.Equal("empty-list-msg", tsl[0].Message)

	tsl = NewMsgList(nil).ToModel()
	assert.Nil(tsl)
}
