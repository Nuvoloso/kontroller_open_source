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


package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http/httputil"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/watchers"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/ws"
	"github.com/gorilla/websocket"
	"github.com/jessevdk/go-flags"
)

func init() {
	initWatcher()
}

func initWatcher() {
	parser.AddCommand("watch", "Watch for change", "Watch for change", &watchCmd{})
}

type watchCmd struct {
	ArgsFile      flags.Filename `short:"F" long:"args-file" description:"Name of a file containing a JSON CrudWatcherCreateArgs object. Use this to watch for multiple event types."`
	Method        string         `short:"M" long:"method-pattern" description:"Regular expression matching method name, if args-file not specified"`
	URI           string         `short:"U" long:"uri-pattern" description:"Regular expression matching a URI trimmed off its base path, if args-file not specified"`
	Scope         string         `short:"S" long:"scope-pattern" description:"Regular expression matching the scope, if args-file not specified"`
	FirstOnly     bool           `short:"1" long:"quit-on-first" description:"Quit on receipt of the first event"`
	Silent        bool           `short:"q" long:"silent" description:"Emit no output. Ignored if quit-on-first is not set."`
	LastEventFile flags.Filename `short:"L" long:"last-event-file" description:"Name of a file to which the last event received will be recorded.  The file will be overwritten each time."`
	dialer        ws.Dialer

	remainingArgsCatcher
}

func (c *watchCmd) Execute(args []string) error {
	var err error
	if err = c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	ca := &models.CrudWatcherCreateArgs{}
	if c.ArgsFile != "" {
		if err = parseWatcherArgFile(string(c.ArgsFile), ca); err != nil {
			return err
		}
	} else {
		m := &models.CrudMatcher{
			MethodPattern: c.Method,
			URIPattern:    c.URI,
			ScopePattern:  c.Scope,
		}
		if m.MethodPattern != "" || m.URIPattern != "" || m.ScopePattern != "" {
			ca.Matchers = []*models.CrudMatcher{m}
		}
	}
	var data bytes.Buffer
	saveOutputWriter := outputWriter
	outputWriter = &data
	errBreakOut := fmt.Errorf("errBreakOut")
	cbf := func(ev *models.CrudEvent) error {
		if ev == nil {
			return nil
		}
		var err error
		if c.FirstOnly && c.Silent {
			return errBreakOut
		}
		data.Reset()
		switch appCtx.OutputFormat {
		case "json":
			appCtx.EmitJSON(ev)
		case "yaml":
			appCtx.EmitYAML(ev)
		default:
			s := appCtx.CrudEventScopeString(ev)
			fmt.Fprintf(outputWriter, "%s %d %s %s%s\n", ev.Timestamp, ev.Ordinal, ev.Method, ev.TrimmedURI, s)
		}
		if c.LastEventFile != "" {
			err = ioutil.WriteFile(string(c.LastEventFile), data.Bytes(), 0600)
		} else {
			_, err = saveOutputWriter.Write(data.Bytes())
		}
		if err != nil {
			return err
		}
		if c.FirstOnly {
			return errBreakOut
		}
		return nil
	}
	if err = appCtx.Watch(ca, cbf); err != nil && err != errBreakOut {
		return err
	}
	return nil
}

func parseWatcherArgFile(filename string, ca *models.CrudWatcherCreateArgs) error {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(content, ca); err != nil {
		return fmt.Errorf("could not parse JSON in %s: %s", filename, err.Error())
	}
	return nil
}

// Watcher is the interface for Watch to enable unit testing
type Watcher interface {
	Watch(ca *models.CrudWatcherCreateArgs, cb watcherCallback) error
}

type watcherCallback func(ev *models.CrudEvent) error

// Watch establishes a watcher and invokes a callback with each event received.
// It always invokes the callback with a nil after establishing the watcher.
func (appCtx *AppCtx) Watch(ca *models.CrudWatcherCreateArgs, cb watcherCallback) error {
	var err error
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	params := watchers.NewWatcherCreateParams()
	params.Payload = ca
	if ca.Name == "" {
		ca.Name = appCtx.MakeWatcherName()
	}
	resC, err := appCtx.API.Watchers().WatcherCreate(params)
	if err != nil {
		if e, ok := err.(*watchers.WatcherCreateDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	if err = appCtx.InitCrudeDialer(); err != nil {
		return err
	}
	urlS := appCtx.MakeCrudeURL(string(resC.Payload))
	headers := appCtx.MakeAuthHeaders()
	conn, resp, err := appCtx.Dialer.Dial(urlS, headers)
	if err != nil {
		if len(appCtx.Verbose) > 0 && resp != nil {
			b, _ := httputil.DumpResponse(resp, true)
			fmt.Fprintf(debugWriter, "watcher dial error %q response:\n%s", urlS, string(b))
		}
		return err
	}
	err = cb(nil) // first call
	for err == nil {
		ev := &models.CrudEvent{}
		if err = conn.ReadJSON(ev); err != nil {
			return err
		}
		if len(appCtx.Verbose) > 0 {
			s := appCtx.CrudEventScopeString(ev)
			fmt.Fprintf(debugWriter, "%s %d %s %s%s\n", ev.Timestamp, ev.Ordinal, ev.Method, ev.TrimmedURI, s)
		}
		err = cb(ev)
	}
	conn.WriteMessage(websocket.CloseMessage, []byte{})
	return err
}

// CrudEventScopeString returns a string with the scope properties
func (appCtx *AppCtx) CrudEventScopeString(ev *models.CrudEvent) string {
	s := ""
	if len(ev.Scope) > 0 {
		sKeys := util.SortedStringKeys(ev.Scope)
		ss := make([]string, len(ev.Scope))
		for i, k := range sKeys {
			ss[i] = fmt.Sprintf("%s:%s", k, ev.Scope[k])
		}
		s = " " + strings.Join(ss, " ")
	}
	return s
}

// MakeWatcherName creates a unique name for the (assumed singleton) watcher of this process
func (appCtx *AppCtx) MakeWatcherName() string {
	h, _ := os.Hostname()
	return fmt.Sprintf("%s-%d@%s", Appname, os.Getpid(), h)
}

// ChangeDetector is the callback interface used by WatchForChange()
type ChangeDetector interface {
	WatcherArgs() *models.CrudWatcherCreateArgs
	ChangeDetected() error
}

// ChangeDetectorWithTicker is like ChangeDetector but invokes a periodic ticker that is reset beteween changes.
type ChangeDetectorWithTicker interface {
	ChangeDetector
	NoChangeTick() error
	NoChangeTickerPeriod() time.Duration
}

type changeWatcher struct {
	cd             ChangeDetector
	cdt            ChangeDetectorWithTicker
	period         time.Duration
	timer          *time.Timer
	mux            sync.Mutex
	cond           *sync.Cond
	detectedChange bool
	timedOut       bool
	mustQuit       bool
	stopThread     bool
	watchErr       error
	threadDone     bool
}

func (c *changeWatcher) watch() {
	terminatedErr := fmt.Errorf("thread-terminated")
	cbf := func(ev *models.CrudEvent) error {
		c.mux.Lock()
		defer c.mux.Unlock()
		c.cond.Signal()
		if c.stopThread {
			return terminatedErr
		}
		c.detectedChange = true
		return nil
	}
	if err := appCtx.watcher.Watch(c.cd.WatcherArgs(), cbf); err != nil && err != terminatedErr {
		c.watchErr = err
	}
	c.mux.Lock()
	c.mustQuit = true
	c.threadDone = true
	c.cond.Signal()
	c.mux.Unlock()
}

func (c *changeWatcher) tick() {
	c.mux.Lock()
	defer c.mux.Unlock()
	c.timedOut = true
	c.timer = nil
	c.cond.Signal()
}

// WatchForChange invokes a ChangeDetected callback on changes.
// The ChangeDetected callback will always be called once.
// Changes are detected asynchronous to the callback and multiple pending changes will be
// collapsed into a single notification.
// The ChangeDetectedWithTicker interface can be provided instead of ChangeDetector. In
// this case a time period must returned by the NoChangeTickerPeriod method, and the NoChangeTick
// method will be invoked if there is no change detected in that time period.
// It is guaranteed that the invocation of the ChangeDetected and NoChangeTick methods will not overlap.
func (appCtx *AppCtx) WatchForChange(cd ChangeDetector) error {
	c := &changeWatcher{}
	return c.WatchForChange(cd)
}

// WatchForChange implements the change watcher
func (c *changeWatcher) WatchForChange(cd ChangeDetector) error {
	c.cond = sync.NewCond(&c.mux)
	c.cd = cd
	cdt, ok := cd.(ChangeDetectorWithTicker)
	if ok {
		c.cdt = cdt
		period := cdt.NoChangeTickerPeriod()
		if period <= 0 {
			return fmt.Errorf("invalid period")
		}
		c.period = period
	}
	go c.watch()
	var mustQuit bool
	for !mustQuit {
		var detectedChange bool
		var timedOut bool
		var err error
		c.mux.Lock()
		if !(c.mustQuit || c.detectedChange || c.timedOut) {
			c.cond.Wait()
		}
		detectedChange = c.detectedChange
		timedOut = c.timedOut
		c.detectedChange = false
		c.timedOut = false
		mustQuit = c.mustQuit
		if c.timer != nil {
			c.timer.Stop()
			c.timer = nil
		}
		c.mux.Unlock()
		if detectedChange { // allow a run even if quitting
			err = c.cd.ChangeDetected()
		} else if timedOut {
			err = c.cdt.NoChangeTick()
		}
		if err != nil {
			c.stopThread = true
			c.watchErr = err
			mustQuit = true
		} else if c.period > 0 {
			c.timer = time.AfterFunc(c.period, c.tick)
		}
	}
	c.mux.Lock()
	defer c.mux.Unlock()
	for !c.threadDone {
		c.cond.Wait()
	}
	return c.watchErr
}
