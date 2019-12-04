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

// Worker constants
const (
	// The default time between periodic runs
	WorkerSleepIntervalDefault = 1 * time.Minute
	// The default maximum termination delay
	WorkerTerminationDelayDefault = 5 * time.Second
)

// WorkerArgs contains the arguments needed to create a Worker
type WorkerArgs struct {
	Name             string
	Log              *logging.Logger
	SleepInterval    time.Duration
	TerminationDelay time.Duration
}

func (wa *WorkerArgs) sanitize() {
	if wa.SleepInterval <= 0 {
		wa.SleepInterval = WorkerSleepIntervalDefault
	}
	if wa.TerminationDelay <= 0 {
		wa.TerminationDelay = WorkerTerminationDelayDefault
	}
}

// Worker performs periodic work on a thread, with the ability to wake up the worker
type Worker interface {
	Start()
	Stop()
	Started() bool
	Notify()
	LastErr() error
	SetSleepInterval(time.Duration)
	GetSleepInterval() time.Duration
	GetNotifyCount() int
}

// WorkerBee is an interface that will perform the invoker task
type WorkerBee interface {
	// Buzz is invoked to perform some work.
	// The returned error does not terminate the task but is logged if not repeated.
	Buzz(ctx context.Context) error
}

// NewWorker creates a Worker
func NewWorker(wa *WorkerArgs, bee WorkerBee) (Worker, error) {
	if wa == nil || wa.Name == "" || wa.Log == nil || bee == nil {
		return nil, fmt.Errorf("invalid arguments")
	}
	w := &worker{}
	w.WorkerArgs = *wa
	w.WorkerArgs.sanitize()
	w.bee = bee
	w.notifyFlagged = true // protect against Notify() until started
	return w, nil
}

type worker struct {
	WorkerArgs
	bee           WorkerBee
	mux           sync.Mutex
	cancelRun     context.CancelFunc
	stoppedChan   chan struct{}
	notifyFlagged bool
	notifyChan    chan struct{}
	runCount      int
	notifyCount   int
	wakeCount     int
	fatalError    bool
	lastErr       error
	errRepeat     int
}

// Start the watcher
func (w *worker) Start() {
	w.Log.Debugf("%s: starting", w.Name)
	ctx, cancel := context.WithCancel(context.Background())
	w.cancelRun = cancel
	w.runCount = 0
	w.notifyCount = 0
	w.wakeCount = 0
	w.notifyReset(true)
	go PanicLogger(w.Log, func() { w.run(ctx) })
}

// Stop the watcher
func (w *worker) Stop() {
	if w.cancelRun != nil {
		w.Log.Debugf("%s: stopping", w.Name)
		w.stoppedChan = make(chan struct{})
		w.cancelRun()
		select {
		case <-w.stoppedChan: // response received
		case <-time.After(w.TerminationDelay):
			w.Log.Warningf("%s: timed out waiting for termination", w.Name)
		}
		w.cancelRun = nil
	}
	w.Log.Debugf("%s: stopped", w.Name)
}

// Started indicates if the worker was started
func (w *worker) Started() bool {
	return w.cancelRun != nil
}

// Notify is used to wake the loop
func (w *worker) Notify() {
	w.mux.Lock()
	defer w.mux.Unlock()
	w.notifyCount++
	if !w.notifyFlagged {
		w.notifyFlagged = true
		close(w.notifyChan)
	}
}

// LastErr returns the last error reported
func (w *worker) LastErr() error {
	return w.lastErr
}

// SetSleepInterval modifies the sleep interval. Typically called from Buzz().
func (w *worker) SetSleepInterval(si time.Duration) {
	wa := &WorkerArgs{}
	*wa = w.WorkerArgs
	wa.SleepInterval = si
	wa.sanitize()
	w.SleepInterval = wa.SleepInterval
}

// GetSleepInterval returns the current sleep interval
func (w *worker) GetSleepInterval() time.Duration {
	return w.SleepInterval
}

// GetNotifyCount returns the number of times Notify() was called
func (w *worker) GetNotifyCount() int {
	return w.notifyCount
}

func (w *worker) notifyReset(force bool) {
	w.mux.Lock()
	defer w.mux.Unlock()
	if w.notifyFlagged || force {
		w.notifyFlagged = false
		w.notifyChan = make(chan struct{})
	}
}

func (w *worker) run(ctx context.Context) {
	for {
		w.notifyReset(false) // notifications while body is running will force another call
		w.runBody(ctx)
		select {
		case <-w.notifyChan: // channel closed on
			w.Log.Debugf("%s: woken up", w.Name)
			w.wakeCount++
		case <-ctx.Done():
			close(w.stoppedChan) // notify waiter by closing the channel
			return
		case <-time.After(w.SleepInterval):
			// wait
		}
	}
}

// ErrWorkerAborted can be returned by WorkerBee.Buzz to abort the worker
var ErrWorkerAborted = fmt.Errorf("worker aborted")

func (w *worker) runBody(ctx context.Context) {
	w.runCount++

	err := w.bee.Buzz(ctx)
	if err == ErrWorkerAborted {
		w.lastErr = err
		w.Log.Debugf("%s: aborted", w.Name)
		w.stoppedChan = make(chan struct{})
		w.cancelRun()
		w.cancelRun = nil
		return
	}

	if err != nil {
		if w.lastErr == nil || err.Error() != w.lastErr.Error() {
			w.Log.Errorf("%s: %s", w.Name, err.Error())
			w.errRepeat = 0
		} else {
			w.errRepeat++
		}
	}
	w.lastErr = err
}
