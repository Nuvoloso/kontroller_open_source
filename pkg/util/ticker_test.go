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
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestRoundingTickerPeriod(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	tk, err := NewRoundingTicker(&RoundingTickerArgs{Log: tl.Logger(), Period: time.Hour})
	assert.NoError(err)

	tk.count = 0
	tk.CallImmediately = true
	assert.Equal(RoundingTickerInitialDelayDefault, tk.nextDuration())
	tk.CallImmediately = false
	tk.RoundDown = 0
	assert.Equal(tk.Period, tk.nextDuration())

	defer func() {
		tickerNow = time.Now
	}()
	nowTime := time.Date(2018, 4, 13, 13, 40, 1, 2, time.UTC)
	tickerNow = func() time.Time {
		return nowTime
	}

	// 1h period, 1h truncate run
	tk.Period = time.Hour
	tk.RoundDown = time.Hour
	expTime := time.Date(2018, 4, 13, 14, 0, 0, 0, time.UTC)
	expPeriod := expTime.Sub(nowTime)
	tl.Logger().Infof("exp:%v got:%v", expPeriod, tk.nextDuration())
	assert.Equal(expPeriod, tk.nextDuration())

	nowTime = nowTime.Add(expPeriod)
	expTime = time.Date(2018, 4, 13, 15, 00, 0, 0, time.UTC)
	expPeriod = expTime.Sub(nowTime)
	tl.Logger().Infof("exp:%v got:%v", expPeriod, tk.nextDuration())
	assert.Equal(expPeriod, tk.nextDuration())

	// 1h period, 30m truncate run
	nowTime = time.Date(2018, 4, 13, 13, 40, 1, 2, time.UTC)
	tk.Period = time.Hour
	tk.RoundDown = 30 * time.Minute
	expTime = time.Date(2018, 4, 13, 14, 30, 0, 0, time.UTC)
	expPeriod = expTime.Sub(nowTime)
	tl.Logger().Infof("exp:%v got:%v", expPeriod, tk.nextDuration())
	assert.Equal(expPeriod, tk.nextDuration())

	nowTime = nowTime.Add(expPeriod)
	expTime = time.Date(2018, 4, 13, 15, 30, 0, 0, time.UTC)
	expPeriod = expTime.Sub(nowTime)
	tl.Logger().Infof("exp:%v got:%v", expPeriod, tk.nextDuration())
	assert.Equal(expPeriod, tk.nextDuration())

	// 30m period, 30m truncate run
	nowTime = time.Date(2018, 4, 13, 13, 40, 1, 2, time.UTC)
	tk.Period = 30 * time.Minute
	tk.RoundDown = 30 * time.Minute
	expTime = time.Date(2018, 4, 13, 14, 0, 0, 0, time.UTC)
	expPeriod = expTime.Sub(nowTime)
	tl.Logger().Infof("exp:%v got:%v", expPeriod, tk.nextDuration())
	assert.Equal(expPeriod, tk.nextDuration())

	nowTime = nowTime.Add(expPeriod)
	expTime = time.Date(2018, 4, 13, 14, 30, 0, 0, time.UTC)
	expPeriod = expTime.Sub(nowTime)
	tl.Logger().Infof("exp:%v got:%v", expPeriod, tk.nextDuration())
	assert.Equal(expPeriod, tk.nextDuration())
}

func TestRoundingTickerTimer(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	args := &RoundingTickerArgs{}
	tk, err := NewRoundingTicker(args)
	assert.Error(err)
	assert.Regexp("invalid arguments", err)

	args = &RoundingTickerArgs{Log: tl.Logger()}
	tk, err = NewRoundingTicker(args)
	assert.Error(err)
	assert.Regexp("invalid arguments", err)

	args = &RoundingTickerArgs{Period: time.Hour}
	tk, err = NewRoundingTicker(args)
	assert.Error(err)
	assert.Regexp("invalid arguments", err)

	args = &RoundingTickerArgs{Log: tl.Logger(), Period: time.Hour, RoundDown: 2 * time.Hour, CallImmediately: true}
	tk, err = NewRoundingTicker(args)
	assert.NoError(err)
	assert.NotNil(tk)
	assert.Equal(RoundingTickerInitialDelayDefault, tk.InitialDelay)
	assert.Equal(tk.Period, tk.RoundDown)
	assert.True(tk.CallImmediately)

	// There is a rare race condition in that the tick() thread could be running when the timer stop is called.
	// See https://golang.org/pkg/time/#Timer.Stop for the AfterFunc() case for the reason.
	// Dropping the period into the nanosecond range causes this race to become more likely but it is still not predictable.
	// The UT contrives to literally force the tick() thread to be running when stop is called, with a
	// two way handshake with fakeRoundingTickerBeeper.Beep().
	period := 5 * time.Millisecond
	args = &RoundingTickerArgs{Log: tl.Logger(), Period: period}
	tk, err = NewRoundingTicker(args)
	assert.NoError(err)
	assert.NotNil(tk)
	assert.Equal(RoundingTickerInitialDelayDefault, tk.InitialDelay)
	assert.Equal(period, tk.Period)
	assert.Equal(tk.Period, tk.RoundDown)
	assert.False(tk.CallImmediately)
	assert.False(tk.stopped)

	fB := &fakeRoundingTickerBeeper{}
	err = tk.Start(fB)
	assert.NoError(err)
	assert.NotNil(tk.ctx)
	assert.NotNil(tk.cancelRun)

	// wait for a few iterations then stop
	fB.mustBlock = false
	fB.canProceed = false
	fB.beepBlocking = false
	for tk.count < 10 {
		time.Sleep(30 * time.Microsecond)
	}
	fB.mustBlock = true
	for !fB.beepBlocking {
		time.Sleep(10 * time.Microsecond)
	}
	go func() { fB.canProceed = true }()
	tk.Stop()
	assert.True(tk.stopped)
	assert.Equal(tk.count, fB.beepCalled)
	err = nil
	select {
	case <-tk.ctx.Done():
		err = tk.ctx.Err()
		break
	}
	assert.Error(err)
	assert.Equal(context.Canceled, err)

	// restart
	fB.beepCalled = 0
	fB.mustBlock = false
	fB.canProceed = false
	fB.beepBlocking = false
	err = tk.Start(fB)
	assert.NoError(err)

	// cannot call start without stop
	err = tk.Start(fB)
	assert.Error(err)

	// wait for a few iterations then stop; include a panic
	fB.panicOnce = true
	for tk.count < 10 {
		time.Sleep(30 * time.Millisecond)
	}
	fB.mustBlock = true
	for !fB.beepBlocking {
		time.Sleep(10 * time.Microsecond)
	}
	go func() { fB.canProceed = true }()
	tk.Stop()
	assert.True(tk.stopped)
	assert.Equal(tk.count, fB.beepCalled)

	// stop is forgiving
	tk.Stop()

	assert.Equal(1, tl.CountPattern("PANIC"))
	assert.Equal(1, tl.CountPattern("Beep panic"))
}

type fakeRoundingTickerBeeper struct {
	beepCalled   int
	beepBlocking bool
	mustBlock    bool
	canProceed   bool
	panicOnce    bool
}

func (f *fakeRoundingTickerBeeper) Beep(ctx context.Context) {
	f.beepCalled++
	if f.panicOnce {
		f.panicOnce = false
		panic("Beep panic")
	}
	if f.mustBlock {
		f.beepBlocking = true
		for !f.canProceed {
			time.Sleep(10 * time.Microsecond)
		}
	}
}
