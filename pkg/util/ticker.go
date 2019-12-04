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
	"sync"
	"time"

	logging "github.com/op/go-logging"
)

// RoundingTicker related constants
const (
	RoundingTickerInitialDelayDefault = time.Second * 5
)

// RoundingTickerArgs specifies the timer parameters
type RoundingTickerArgs struct {
	Log             *logging.Logger
	Period          time.Duration
	RoundDown       time.Duration
	CallImmediately bool
	InitialDelay    time.Duration
}

func (tka *RoundingTickerArgs) sanitize() error {
	if tka.Log == nil || tka.Period == 0 {
		return fmt.Errorf("invalid arguments")
	}
	if tka.RoundDown <= 0 || tka.RoundDown > tka.Period {
		tka.RoundDown = tka.Period
	}
	if tka.InitialDelay <= 0 {
		tka.InitialDelay = RoundingTickerInitialDelayDefault
	}
	return nil
}

// RoundingTicker provides a periodic timer that rounds each scheduled time by a specified amount.
// Optionally, the first tick can be scheduled immediately.
type RoundingTicker struct {
	RoundingTickerArgs

	mux       sync.Mutex
	timer     *time.Timer
	count     int
	started   bool
	stopped   bool
	muffled   bool
	beeper    RoundingTickerBeeper
	ctx       context.Context
	cancelRun context.CancelFunc
}

// RoundingTickerBeeper specifies the callback interface
type RoundingTickerBeeper interface {
	Beep(context.Context)
}

// NewRoundingTicker creates a new ticker
func NewRoundingTicker(args *RoundingTickerArgs) (*RoundingTicker, error) {
	tk := &RoundingTicker{}
	tk.RoundingTickerArgs = *args
	if err := tk.RoundingTickerArgs.sanitize(); err != nil {
		return nil, err
	}
	return tk, nil
}

// Start starts the ticker
func (tk *RoundingTicker) Start(beeper RoundingTickerBeeper) error {
	tk.mux.Lock()
	defer tk.mux.Unlock()
	if tk.started && !tk.stopped {
		return fmt.Errorf("timer active")
	}
	tk.started = true
	tk.count = 0
	tk.beeper = beeper
	tk.stopped = false
	tk.ctx, tk.cancelRun = context.WithCancel(context.Background())
	tk.reschedule()
	return nil
}

// Stop terminates the ticker
func (tk *RoundingTicker) Stop() {
	tk.mux.Lock()
	defer tk.mux.Unlock()
	if tk.started && !tk.stopped {
		tk.stopped = true
		tk.cancelRun()
		if !tk.timer.Stop() {
			tk.mux.Unlock()
			for !tk.muffled {
				time.Sleep(1 * time.Microsecond)
			}
			tk.mux.Lock()
		}
		tk.timer = nil
	}
}

// tickerNow is defined for UT manipulation of time when needed
type tickerNowFunctionType func() time.Time

var tickerNow tickerNowFunctionType = time.Now

func (tk *RoundingTicker) nextDuration() time.Duration {
	if tk.count == 0 && tk.CallImmediately {
		return tk.InitialDelay
	}
	now := tickerNow()
	d := now.Add(tk.Period).Truncate(tk.RoundDown).Sub(now)
	return d
}

func (tk *RoundingTicker) reschedule() {
	if !tk.stopped {
		tk.timer = time.AfterFunc(tk.nextDuration(), tk.tick)
	}
}

func (tk *RoundingTicker) tick() {
	tk.count++
	PanicLogger(tk.Log, func() { tk.beeper.Beep(tk.ctx) })
	tk.mux.Lock()
	defer tk.mux.Unlock()
	tk.reschedule()
	if tk.stopped {
		tk.muffled = true
	}
}
