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
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/go-openapi/strfmt"
)

// NewTimestamedString creates a formatted *models.TimestampedString
func NewTimestamedString(timeStamp time.Time, format string, args ...interface{}) *models.TimestampedString {
	msg := fmt.Sprintf(format, args...)
	ts := &models.TimestampedString{
		Message: msg,
		Time:    strfmt.DateTime(timeStamp),
	}
	return ts
}

// MsgList creates a session in which multiple messages can be inserted into a message list with the same time stamp.
type MsgList struct {
	epoch   time.Time
	tssList []*models.TimestampedString
}

// NewMsgList returns a new message list session for a given message list.
// The timestamp is set to the current time by default but can be adjusted if desired.
// The mObj list is appended to if not nil, or is allocated internally during Insert() if nil.
// Expected usage:
//   var mObj []*models.TimestampedString
//   mObj = NewMsgList(mObj).Insert(format, args)[.Insert(format, args)].ToModel()
func NewMsgList(mObj []*models.TimestampedString) *MsgList {
	ml := &MsgList{epoch: time.Now(), tssList: mObj}
	return ml
}

// Insert a message. Can be chained.
func (ml *MsgList) Insert(format string, args ...interface{}) *MsgList {
	if ml.tssList == nil {
		ml.tssList = make([]*models.TimestampedString, 0)
	}
	ml.tssList = append(ml.tssList, NewTimestamedString(ml.epoch, format, args...))
	return ml
}

// InsertWithRepeatCount appends a count to a message in case it repeats more than some N times (e.g. 'MSG (repeated 1)').
func (ml *MsgList) InsertWithRepeatCount(format string, repeatLimit int, args ...interface{}) *MsgList {
	if ml.tssList == nil {
		ml.tssList = make([]*models.TimestampedString, 0)
	}
	msg := fmt.Sprintf(format, args...)
	msgCnt := 0
	lastRepeatedMsg := ""
	lastRepeatedMsgIdx := 0
	for i := 0; i <= len(ml.tssList)-1; i++ {
		if strings.HasPrefix(ml.tssList[i].Message, msg) { // check if message already exist
			msgCnt++
			lastRepeatedMsg = ml.tssList[i].Message
			lastRepeatedMsgIdx = i
		}
	}
	if msgCnt == repeatLimit && msgCnt != 0 {
		// append message with '(repeated 1)' and update timestamp
		ml.tssList = append(ml.tssList, NewTimestamedString(ml.epoch, format+" (repeated 1)", args...))
	} else if msgCnt > repeatLimit && lastRepeatedMsg != "" { // check for appendix '(repeated N)'
		re := regexp.MustCompile("\\(repeated .*\\)")
		if loc := re.FindStringIndex(lastRepeatedMsg); loc != nil {
			re = regexp.MustCompile("[0-9]+")
			if resList := re.FindAllString(lastRepeatedMsg[loc[0]:loc[1]], -1); resList != nil {
				if cnt, err := strconv.Atoi(resList[0]); err == nil {
					ml.tssList[lastRepeatedMsgIdx].Message = fmt.Sprintf(msg+" (repeated %d)", cnt+1)
					ml.tssList[lastRepeatedMsgIdx].Time = strfmt.DateTime(ml.epoch)
				}
			}
		} else {
			ml.tssList[lastRepeatedMsgIdx].Message = fmt.Sprintf(msg + " (repeated 1)")
			ml.tssList[lastRepeatedMsgIdx].Time = strfmt.DateTime(ml.epoch)
		}
	} else {
		ml.tssList = append(ml.tssList, NewTimestamedString(time.Now(), format, args...))
	}
	return ml
}

// WithTimestamp modifies the session timestamp
func (ml *MsgList) WithTimestamp(timeStamp time.Time) *MsgList {
	ml.epoch = timeStamp
	return ml
}

// ToModel ends the chain and returns the modified list.
// Note: if the original list was nil and Insert() was not called then nil is returned.
func (ml *MsgList) ToModel() []*models.TimestampedString {
	return ml.tssList
}
