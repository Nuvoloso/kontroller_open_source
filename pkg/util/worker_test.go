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
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestWorker(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	tw := &testWorker{}
	wa := &WorkerArgs{}
	wa.sanitize()
	assert.Equal(WorkerSleepIntervalDefault, wa.SleepInterval)
	assert.Equal(WorkerTerminationDelayDefault, wa.TerminationDelay)

	wi, err := NewWorker(nil, nil)
	assert.Error(err)
	assert.Regexp("invalid arguments", err)
	assert.Nil(wi)

	wa = &WorkerArgs{}
	wi, err = NewWorker(wa, tw)
	assert.Error(err)
	assert.Regexp("invalid arguments", err)
	assert.Nil(wi)

	wa = &WorkerArgs{Name: "TW"}
	wi, err = NewWorker(wa, tw)
	assert.Error(err)
	assert.Regexp("invalid arguments", err)
	assert.Nil(wi)

	wa = &WorkerArgs{
		Name:             "TestWorker",
		Log:              tl.Logger(),
		SleepInterval:    10 * time.Millisecond,
		TerminationDelay: 20 * time.Millisecond,
	}
	wi, err = NewWorker(wa, nil)
	assert.Error(err)
	assert.Regexp("invalid arguments", err)
	assert.Nil(wi)

	wi, err = NewWorker(wa, tw)
	assert.NoError(err)
	assert.NotNil(wi)
	w, ok := wi.(*worker)
	assert.True(ok)
	assert.NotNil(w)
	assert.Equal(tw, w.bee)
	assert.Equal(tl.Logger(), w.Log)
	assert.Equal(wa.SleepInterval, w.SleepInterval)
	assert.Equal(wa.TerminationDelay, w.TerminationDelay)
	assert.False(wi.Started())

	wi.Notify() // should not panic before Start()

	assert.Equal(0, w.runCount)
	w.Start() // invokes run/runBody asynchronously but won't do anything if w.fatalErr
	assert.True(w.wakeCount == 0)
	assert.True(w.notifyCount == 0)
	assert.True(wi.Started())
	// notify is inherently racy so keep trying in a loop
	// if we break out of the loop it was detected! :-)
	for w.wakeCount == 0 {
		w.Notify()
		time.Sleep(w.SleepInterval / 4)
	}

	// fake the stop channel so Stop() will time out
	_, cancel := context.WithCancel(context.Background())
	saveCancel := w.cancelRun
	w.cancelRun = cancel
	w.Stop()
	assert.Equal(1, tl.CountPattern("TestWorker.*timed out"))

	// now really stop
	w.cancelRun = saveCancel
	w.Stop()
	assert.Equal(w.runCount, tw.Cnt)
	assert.True(w.runCount > 0)
	assert.False(wi.Started())

	// test the last error logic
	tw.RetErr = fmt.Errorf("fake-error")
	w.runBody(nil)
	assert.Equal(tw.RetErr, wi.LastErr())
	assert.Equal(0, w.errRepeat)
	w.runBody(nil)
	w.runBody(nil)
	assert.Equal(tw.RetErr, wi.LastErr())
	assert.Equal(2, w.errRepeat)
	assert.Equal(1, tl.CountPattern("TestWorker.*fake-error"))
	tw.RetErr = fmt.Errorf("a-different-error")
	w.runBody(nil)
	assert.Equal(tw.RetErr, wi.LastErr())
	assert.Equal(0, w.errRepeat)

	wi.SetSleepInterval(-1)
	assert.Equal(WorkerSleepIntervalDefault, w.SleepInterval)
	assert.Equal(WorkerSleepIntervalDefault, wi.GetSleepInterval())
	wi.SetSleepInterval(time.Second)
	assert.Equal(time.Second, w.SleepInterval)
	assert.Equal(time.Second, wi.GetSleepInterval())

	assert.Equal(w.notifyCount, wi.GetNotifyCount())

	// Test worker abort logic
	tl.Logger().Info("case: Buzz aborts")
	tl.Flush()
	wi, err = NewWorker(wa, tw)
	assert.NoError(err)
	assert.NotNil(wi)
	w, ok = wi.(*worker)
	assert.True(ok)
	assert.NotNil(w)
	assert.Equal(tw, w.bee)

	tw.Cnt = 0
	tw.RetErr = ErrWorkerAborted
	assert.Nil(w.cancelRun)
	wi.Start()
	for w.cancelRun != nil {
		time.Sleep(1 * time.Millisecond)
	}
	assert.Equal(ErrWorkerAborted, w.lastErr)

	// subsequent stop is a no-op
	w.Stop()
}

type testWorker struct {
	Cnt    int
	RetErr error
}

func (tw *testWorker) Buzz(ctx context.Context) error {
	tw.Cnt++
	return tw.RetErr
}
