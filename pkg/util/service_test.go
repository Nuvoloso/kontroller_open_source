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
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/strfmt"
	"github.com/stretchr/testify/assert"
)

func TestService(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	var err error

	args := ServiceArgs{
		ServiceType:         "service type",
		ServiceVersion:      "v1",
		ServiceIP:           "1.2.3.4",
		ServiceLocator:      "service-pod",
		HeartbeatPeriodSecs: 30,
		ServiceAttributes: map[string]models.ValueType{
			"attr1": models.ValueType{Kind: "STRING", Value: "value1"},
		},
		Log:         tl.Logger(),
		MaxMessages: 5,
	}

	c := NewService(&args)
	assert.NotNil(c)
	svc, ok := c.(*Service)
	assert.True(ok)
	assert.EqualValues(svc.ServiceArgs, args)
	assert.Equal(ServiceStopped, svc.state)
	assert.Equal(ServiceStopped, c.GetState())

	for i := 0; i < args.MaxMessages; i++ {
		c.Message("message %d", i)
		time.Sleep(1 * time.Millisecond)
	}
	savedEarliest := svc.messages[0]
	for i := 0; i < len(svc.messages); i++ {
		tl.Logger().Info(i, svc.messages[i])
	}
	tl.Flush()
	assert.Len(svc.messages, args.MaxMessages)
	c.Message("message %d", args.MaxMessages)
	assert.Len(svc.messages, args.MaxMessages)
	for i := 0; i < len(svc.messages); i++ {
		tl.Logger().Info(i, svc.messages[i])
	}
	tl.Flush()
	for i := 0; i < len(svc.messages); i++ {
		tl.Logger().Info(svc.messages[i])
		assert.Equal(fmt.Sprintf("message %d", i+1), svc.messages[i].Message)
	}

	// check that sort by time works
	tE := time.Time(savedEarliest.Time)
	for _, m := range svc.messages {
		tM := time.Time(m.Time)
		assert.True(tE.Before(tM))
	}
	svc.messages = append(svc.messages, savedEarliest)
	svc.adjustMessages(true)
	tL := tE
	for _, m := range svc.messages {
		tM := time.Time(m.Time)
		assert.True(tM != tE)
		assert.True(tL.Before(tM))
		tL = tM
	}

	// check that replace messages works
	repMsgTCs := []struct {
		pat       string
		orig, exp []*models.TimestampedString
	}{
		{
			pat: "XXX",
			orig: []*models.TimestampedString{
				{Message: "blah XXX blah first"},
				{Message: "the crazy fox"},
				{Message: "jumps over the"},
				{Message: "lazy dog"},
				{Message: "blah XXX blah last"},
			},
			exp: []*models.TimestampedString{
				{Message: "the crazy fox"},
				{Message: "jumps over the"},
				{Message: "lazy dog"},
				{Message: "REPLACED XXX MESSAGE"},
			},
		},
		{
			pat: "XXX",
			orig: []*models.TimestampedString{
				{Message: "the crazy fox"},
				{Message: "blah XXX blah not first"},
				{Message: "blah XXX blah multiple in a row"},
				{Message: "jumps over the"},
				{Message: "blah XXX blah not last"},
				{Message: "lazy dog"},
			},
			exp: []*models.TimestampedString{
				{Message: "the crazy fox"},
				{Message: "jumps over the"},
				{Message: "lazy dog"},
				{Message: "REPLACED XXX MESSAGE"},
			},
		},
	}
	for i, tc := range repMsgTCs {
		s := &Service{}
		s.MaxMessages = 100
		s.messages = tc.orig
		s.ReplaceMessage(tc.pat, "REPLACED %s MESSAGE", tc.pat)
		assert.Len(s.messages, len(tc.exp), "RepMsg %d", i)
		for j, m := range s.messages {
			assert.Equal(tc.exp[j].Message, m.Message, "%d[%d]", i, j)
		}
	}

	expMObj := &models.NuvoService{
		NuvoServiceAllOf0: models.NuvoServiceAllOf0{
			ServiceAttributes: args.ServiceAttributes,
			ServiceType:       args.ServiceType,
			ServiceVersion:    args.ServiceVersion,
			ServiceIP:         args.ServiceIP,
			ServiceLocator:    args.ServiceLocator,
			Messages:          svc.messages,
		},
		ServiceState: models.ServiceState{
			HeartbeatPeriodSecs: args.HeartbeatPeriodSecs,
			State:               c.StateString(svc.state),
		},
	}
	mObj := c.ModelObj()
	expMObj.ServiceState.HeartbeatTime = mObj.ServiceState.HeartbeatTime
	assert.EqualValues(expMObj, mObj)

	assert.Equal(args.ServiceIP, c.GetServiceIP())
	c.SetServiceIP("9.8.7.6")
	assert.Equal("9.8.7.6", c.GetServiceIP())

	assert.Equal(args.ServiceLocator, c.GetServiceLocator())
	c.SetServiceLocator("service-locator")
	assert.Equal("service-locator", c.GetServiceLocator())

	// import
	msgTC := map[string]struct {
		mT   strfmt.DateTime
		seen bool
	}{
		"very old message": {mT: strfmt.DateTime(tE)},
		"before last":      {mT: strfmt.DateTime(tL.Add(-100))},
		"after last":       {mT: strfmt.DateTime(tL.Add(100))},
	}
	messages := make([]*models.TimestampedString, 3)
	i := 0
	for k, v := range msgTC {
		messages[i] = &models.TimestampedString{Time: v.mT, Message: k}
		i++
	}
	mObj.Messages = messages
	prevLen := len(svc.messages)
	tl.Logger().Info("Importing")
	err = c.Import(mObj)
	assert.Nil(err)
	assert.Len(svc.messages, prevLen)
	msgSeen := []string{}
	tL = tE
	for _, m := range svc.messages {
		tl.Logger().Info(m)
		tM := time.Time(m.Time)
		assert.True(tM != tE)
		assert.True(tL.Before(tM))
		tL = tM
		_, ok := msgTC[m.Message]
		tl.Logger().Infof("%s => %v", m.Message, ok)
		if ok {
			msgSeen = append(msgSeen, m.Message)
		}
	}
	tl.Logger().Info("Seen", msgSeen)
	assert.EqualValues([]string{"before last", "after last"}, msgSeen)

	// check state labels
	stateTC := map[State]string{
		ServiceStopped:  "STOPPED",
		ServiceStarting: "STARTING",
		ServiceReady:    "READY",
		ServiceError:    "ERROR",
		ServiceStopping: "STOPPING",
		ServiceNotReady: "NOT_READY",
	}
	svc.messages = make([]*models.TimestampedString, 0, svc.MaxMessages+1)
	svc.state = -1 // invalid state we transition through all states
	for s, str := range stateTC {
		assert.Equal(str, c.StateString(s))
		c.SetState(s)
		assert.Equal(s, c.GetState())
		var lM *models.TimestampedString
		if len(svc.messages) > 0 {
			lM = svc.messages[len(svc.messages)-1]
		}
		assert.NotNil(lM)
		assert.Regexp("State change.*"+str+"$", lM.Message)
	}
	assert.Equal("UNKNOWN", c.StateString(-1))
	prevState := c.GetState()
	assert.NotEqual(-1, prevState)
	err = c.SetState(-1)
	assert.NotNil(err)
	assert.Equal(prevState, c.GetState())

	// fail import
	mObj.ServiceType = "foo"
	err = c.Import(mObj)
	assert.NotNil(err)
	assert.Regexp("service type mismatch", err.Error())

	// default max messages and service attrs
	args = ServiceArgs{}
	c = NewService(&args)
	assert.NotNil(c)
	svc, ok = c.(*Service)
	assert.True(ok)
	assert.Equal(ServiceDefaultMaxMessages, svc.MaxMessages)
	assert.NotNil(svc.ServiceArgs.ServiceAttributes)

	// ServiceAttribute tests
	c.SetServiceAttribute("foo", models.ValueType{Kind: "STRING", Value: "bar"})
	fooAttr, ok := svc.ServiceArgs.ServiceAttributes["foo"]
	assert.True(ok)
	assert.Equal("STRING", fooAttr.Kind)
	assert.Equal("bar", fooAttr.Value)

	sa := c.GetServiceAttribute("foo")
	assert.NotNil(sa)
	assert.Equal(fooAttr, *sa)

	mObj = c.ModelObj()
	assert.NotNil(mObj.ServiceAttributes)
	mSA, ok := mObj.ServiceAttributes["foo"]
	assert.True(ok)
	assert.Equal(fooAttr, mSA)

	c.RemoveServiceAttribute("foo")
	fooAttr, ok = svc.ServiceArgs.ServiceAttributes["foo"]
	assert.False(ok)
	assert.Nil(c.GetServiceAttribute("foo"))
}
