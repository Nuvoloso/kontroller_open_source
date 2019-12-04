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


package crude

import (
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/auth"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/satori/go.uuid"
)

// matcher represents a set of regular expressions
type matcher struct {
	reMethod *regexp.Regexp
	reURI    *regexp.Regexp
	reScope  *regexp.Regexp
	count    int
}

func (wM *matcher) match(ce *CrudEvent) bool {
	matches := 0
	if wM.reMethod != nil && wM.reMethod.MatchString(ce.Method) {
		matches++
	}
	if wM.reURI != nil && wM.reURI.MatchString(ce.TrimmedURI) {
		matches++
	}
	if wM.reScope != nil && wM.reScope.MatchString(ce.ScopeS) {
		matches++
	}
	return matches == wM.count
}

// watcher represents a client interested in receiving CrudEvents
// Each watcher must have an independent delivery cadence, that is also
// distinct from the dispatchEvent cadence. To achieve this the
// delivery of events from each Watcher should be done in dedicated
// goroutines.
// This requires the following protection scheme:
//  - Existence of the watcher and the matchers are protected by the manager mutex to allow for
//    event matching across multiple watchers without extra locking.
//  - The watcher queue is protected by the watcher mutex.
//  - The watcher mutex must not be held during delivery.
//  - The manager mutex has a higher priority than the watcher mutex; if needed
//    it should be obtained before the watcher mutex.
//  - The terminate flag should never be cleared once set. A true value is valid even
//    without holding the watcher mutex.
type watcher struct {
	m              *Manager
	client         Watcher
	auth           auth.Subject
	id             string
	name           string
	activateByTime time.Time
	active         bool
	matchers       []matcher
	// delivery queue and termination is protected by the following mutex
	mux       sync.Mutex
	cond      *sync.Cond // signal when changes made to the watcher.
	terminate bool
	ceQ       []*CrudEvent // queue of events for delivery
}

// newWatcher compiles a watchers patterns and registers it with the manager on success.
// The new watcher is inactive.
func (m *Manager) newWatcher(mObj *models.CrudWatcherCreateArgs, client Watcher) (string, error) {
	if mObj == nil || client == nil {
		return "", fmt.Errorf("invalid arguments")
	}
	w := &watcher{}
	w.init(m, client)
	w.name = mObj.Name
	m.Log.Debugf("New watcher %s (%s)", w.name, w.id)
	if err := w.parseArgs(mObj); err != nil {
		return "", err
	}
	m.mux.Lock()
	m.watchers = append(m.watchers, w)
	m.mux.Unlock()
	return w.id, nil
}

func (w *watcher) init(m *Manager, client Watcher) *watcher {
	w.m = m
	w.client = client
	w.id = uuid.NewV4().String()
	w.activateByTime = time.Now().Add(WatcherActivationLimit)
	w.cond = sync.NewCond(&w.mux)
	w.ceQ = make([]*CrudEvent, 0, 8)
	return w
}

// parseArgs processes the creation arguments.
func (w *watcher) parseArgs(mObj *models.CrudWatcherCreateArgs) error {
	w.matchers = make([]matcher, len(mObj.Matchers))
	var err error
	for i, mM := range mObj.Matchers {
		cnt := 0
		wM := &w.matchers[i]
		which := ""
		if mM.MethodPattern != "" {
			wM.reMethod, err = regexp.Compile(mM.MethodPattern)
			which = "methodPattern"
			cnt++
		}
		if err == nil && mM.URIPattern != "" {
			wM.reURI, err = regexp.Compile(mM.URIPattern)
			which = "uriPattern"
			cnt++
		}
		if err == nil && mM.ScopePattern != "" {
			wM.reScope, err = regexp.Compile(mM.ScopePattern)
			which = "scopePattern"
			cnt++
		}
		if err != nil {
			return fmt.Errorf("[%d]%s: %s", i, which, err.Error())
		}
		if cnt == 0 {
			return fmt.Errorf("[%d] no patterns specified", i)
		}
		wM.count = cnt
		w.m.Log.Debugf("%s [%d] %d %#v", w.id, i, cnt, mM)
	}
	return nil
}

func (w *watcher) match(ce *CrudEvent) bool {
	if !w.m.accessManager.EventAllowed(w.auth, ce) {
		return false
	}
	if len(w.matchers) == 0 {
		return true
	}
	for _, wM := range w.matchers {
		if wM.match(ce) {
			return true
		}
	}
	return false
}

// deliverEvents must run in its own goroutine.
// It uses the Watcher interface to:
// - deliver an event
// - notify termination
// It removes the watcher on termination.
func (w *watcher) deliverEvents() {
	mustQuit := false
	for {
		w.mux.Lock()
		for !w.terminate && len(w.ceQ) == 0 {
			w.cond.Wait()
		}
		var ce *CrudEvent
		if w.terminate {
			mustQuit = true
		} else {
			ce = w.ceQ[0]
			// https://github.com/golang/go/wiki/SliceTricks
			copy(w.ceQ[0:], w.ceQ[1:])
			w.ceQ[len(w.ceQ)-1] = nil
			w.ceQ = w.ceQ[:len(w.ceQ)-1]
		}
		w.mux.Unlock()
		if mustQuit {
			w.m.Log.Debugf("WATCHER %s %s: Quitting", w.name, w.id)
			w.client.CrudeNotify(WatcherQuitting, nil)
			break
		}
		w.m.Log.Debugf("WATCHER %s %s ⇒ EVENT %d", w.name, w.id, ce.Ordinal)
		if err := w.client.CrudeNotify(WatcherEvent, ce); err != nil {
			w.m.Log.Errorf("Watcher %s: callback error: %s", w.id, err.Error())
			w.mustTerminate()
			break
		}
	}
	w.m.removeWatcher(w)
}

// mustTerminate sets the termination flag of the watcher if not set and signals the watcher CV.
func (w *watcher) mustTerminate() {
	if w.terminate {
		return
	}
	w.mux.Lock()
	if !w.terminate {
		w.m.Log.Debugf("%s %s mustTerminate", w.name, w.id)
		w.terminate = true
		w.cond.Signal()
	}
	w.mux.Unlock()
}
