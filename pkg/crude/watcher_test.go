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
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestNewWatcher(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)

	mA := &ManagerArgs{
		Log: tl.Logger(),
	}
	mo := NewManager(mA)
	assert.NotNil(mo)
	m, ok := mo.(*Manager)
	assert.True(ok)

	twc := &testWatcherClient{}
	ca := &models.CrudWatcherCreateArgs{}

	// verify w.init
	now := time.Now()
	w := &watcher{}
	w.init(m, twc)
	assert.NotEmpty(w.id)
	assert.NotNil(w.cond)
	assert.NotNil(w.ceQ)
	assert.Equal(m, w.m)
	assert.Equal(twc, w.client)
	assert.True(w.activateByTime.After(now))
	assert.True(w.activateByTime.Before(time.Now().Add(WatcherActivationLimit)))
	assert.False(w.active)
	assert.False(w.terminate)

	// newWatcher, invalid args
	id, err := m.newWatcher(nil, nil)
	assert.Error(err)
	assert.Regexp("invalid arguments", err)
	id, err = m.newWatcher(ca, nil)
	assert.Error(err)
	assert.Regexp("invalid arguments", err)

	// newWatcher (no items)
	assert.Empty(m.watchers)
	id, err = m.newWatcher(ca, twc)
	assert.NoError(err)
	assert.Len(m.watchers, 1)
	w = m.watchers[0]
	assert.Equal(id, w.id)
	assert.Equal(twc, w.client)

	// newWatcher (item but no patterns)
	ca = &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{},
		},
	}
	m.watchers = []*watcher{}
	id, err = m.newWatcher(ca, twc)
	assert.Error(err)
	assert.Regexp("\\[0] no patterns specified", err)
	assert.Empty(id)
	assert.Empty(m.watchers)

	// newWatcher bad patterns in item[0]
	pats := []string{"scopePattern", "uriPattern", "methodPattern"}
	ca = &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{},
		},
	}
	for _, which := range pats {
		switch which {
		case "scopePattern":
			ca.Matchers[0].ScopePattern = "bad](pattern"
		case "uriPattern":
			ca.Matchers[0].URIPattern = "bad](pattern"
		case "methodPattern":
			ca.Matchers[0].MethodPattern = "bad](pattern"
		}
		id, err = m.newWatcher(ca, twc)
		assert.Error(err)
		assert.Regexp("\\[0]"+which+":", err)
		assert.Empty(id)
		assert.Empty(m.watchers)
	}

	// newWatcher bad patterns in item[1]
	pats = []string{"scopePattern", "uriPattern", "methodPattern"}
	ca = &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{
				MethodPattern: "POST|PATCH|DELETE",
			},
			&models.CrudMatcher{},
		},
	}
	for _, which := range pats {
		switch which {
		case "scopePattern":
			ca.Matchers[1].ScopePattern = "bad](pattern"
		case "uriPattern":
			ca.Matchers[1].URIPattern = "bad](pattern"
		case "methodPattern":
			ca.Matchers[1].MethodPattern = "bad](pattern"
		}
		id, err = m.newWatcher(ca, twc)
		assert.Error(err)
		assert.Regexp("\\[1]"+which+":", err)
		assert.Empty(id)
		assert.Empty(m.watchers)
	}

	// newWatcher good patterns count
	pats = []string{"scopePattern", "uriPattern", "methodPattern"}
	ca = &models.CrudWatcherCreateArgs{
		Name: "WATCHERNAME",
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{},
		},
	}
	for i, which := range pats {
		switch which {
		case "scopePattern":
			ca.Matchers[0].ScopePattern = "good pattern"
		case "uriPattern":
			ca.Matchers[0].URIPattern = "good pattern"
		case "methodPattern":
			ca.Matchers[0].MethodPattern = "good pattern"
		}
		m.watchers = []*watcher{}
		id, err = m.newWatcher(ca, twc)
		assert.NoError(err)
		assert.NotEmpty(id)
		assert.NotEmpty(m.watchers)
		w = m.watchers[0]
		assert.Len(w.matchers, 1)
		assert.Equal(i+1, w.matchers[0].count)
		switch which {
		case "scopePattern":
			assert.NotNil(w.matchers[0].reScope)
		case "uriPattern":
			assert.NotNil(w.matchers[0].reURI)
		case "methodPattern":
			assert.NotNil(w.matchers[0].reMethod)
		}
		assert.Equal(1, tl.CountPattern(fmt.Sprintf("New watcher WATCHERNAME \\(%s\\)", id)))
	}
	tl.Flush()
}

func TestMatcher(t *testing.T) {
	assert := assert.New(t)

	pats := []struct {
		which string
		pat   string
	}{
		{"scope", "x:y"},
		{"uri", "/account/?"},
		{"method", "P.*"},
	}
	wM := &matcher{}
	ce := &CrudEvent{}
	for i, tc := range pats {
		switch tc.which {
		case "scope":
			wM.reScope = regexp.MustCompile(tc.pat)
			ce.ScopeS = "a:b x:y"
		case "uri":
			wM.reURI = regexp.MustCompile(tc.pat)
			ce.TrimmedURI = "/account/id?set=name"
		case "method":
			wM.reMethod = regexp.MustCompile(tc.pat)
			ce.Method = "PATCH"
		}
		wM.count = i + 1
		assert.True(wM.match(ce))
	}

	// empty matcher matches anything
	w := &watcher{}
	w.m = &Manager{}
	fac := &fakeAccessControl{RetAllowed: true}
	w.m.accessManager = fac
	w.matchers = []matcher{}
	assert.True(w.match(ce))

	// ... but access control occurs before scope checks
	fac.RetAllowed = false
	assert.False(w.match(ce))
	fac.RetAllowed = true

	// multi-matcher
	w.matchers = []matcher{
		{reMethod: regexp.MustCompile("POST"), count: 1},
		*wM,
	}
	assert.False(w.matchers[0].match(ce))
	assert.True(w.matchers[1].match(ce))
	assert.True(w.match(ce))

	// event that doesn't match
	ce.Method = "DELETE"
	assert.False(w.matchers[0].match(ce))
	assert.False(w.matchers[1].match(ce))
	assert.False(w.match(ce))
}

func TestWatcherOperation(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)

	mA := &ManagerArgs{
		Log: tl.Logger(),
	}
	mo := NewManager(mA)
	assert.NotNil(mo)
	m, ok := mo.(*Manager)
	assert.True(ok)

	twc := &testWatcherClient{}
	twc.init()

	w := &watcher{}
	w.init(m, twc)
	assert.Equal(m, w.m)
	assert.Equal(twc, w.client)
	m.watchers = []*watcher{w}
	assert.Len(w.ceQ, 0)
	w.active = true

	dFuncStarted := false
	dFuncStopped := false
	dFunc := func() {
		dFuncStarted = true
		w.deliverEvents()
		dFuncStopped = true
	}
	go dFunc()
	for dFuncStarted == false {
		time.Sleep(5 * time.Millisecond)
	}

	// dispatch a couple of events
	ce1 := &CrudEvent{}
	ce2 := &CrudEvent{}
	w.mux.Lock()
	w.ceQ = append(w.ceQ, ce1, ce2)
	w.cond.Signal()
	assert.Empty(twc.InCNct)
	assert.Empty(twc.InCNce)
	w.mux.Unlock()

	// wait till they are delivered
	for len(w.ceQ) > 0 {
		time.Sleep(5 * time.Millisecond)
	}
	assert.Len(twc.InCNct, 2)
	assert.Len(twc.InCNce, 2)
	assert.Equal(WatcherEvent, twc.InCNct[0])
	assert.Equal(ce1, twc.InCNce[0])
	assert.Equal(WatcherEvent, twc.InCNct[1])
	assert.Equal(ce2, twc.InCNce[1])

	// stop the watcher
	assert.False(w.terminate)
	w.mustTerminate()
	assert.True(w.terminate)
	for dFuncStopped == false {
		time.Sleep(5 * time.Millisecond)
	}
	assert.Len(twc.InCNct, 3)
	assert.Len(twc.InCNce, 3)
	assert.Equal(WatcherQuitting, twc.InCNct[2])
	assert.Equal((*CrudEvent)(nil), twc.InCNce[2])

	// terminate is safe for retry
	assert.True(w.terminate)
	w.mux.Lock()
	w.mustTerminate() // would block if it tried to get the lock
	w.mux.Unlock()

	// test termination on callback error
	w.terminate = false
	w.active = true
	w.ceQ = []*CrudEvent{ce1}
	twc.init() // reset
	twc.RetCNErr = fmt.Errorf("callback-error")
	w.deliverEvents()
	assert.Equal(1, twc.CallsCN)
	assert.True(w.terminate)
}

type testWatcherClient struct {
	CallsCN  int
	RetCNErr error
	InCNct   []WatcherCallbackType
	InCNce   []*CrudEvent
}

func (twc *testWatcherClient) init() {
	twc.CallsCN = 0
	twc.InCNct = make([]WatcherCallbackType, 0)
	twc.InCNce = make([]*CrudEvent, 0)
}

func (twc *testWatcherClient) CrudeNotify(ct WatcherCallbackType, ce *CrudEvent) error {
	twc.CallsCN++
	twc.InCNct = append(twc.InCNct, ct)
	twc.InCNce = append(twc.InCNce, ce)
	return twc.RetCNErr
}
