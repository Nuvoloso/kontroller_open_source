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
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/auth"
	nuvo "github.com/Nuvoloso/kontroller/pkg/autogen/client"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestManager(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)

	mA := &ManagerArgs{
		WSKeepAlive: true,
		Log:         tl.Logger(),
	}
	mo := NewManager(mA)
	assert.NotNil(mo)
	m, ok := mo.(*Manager)
	assert.True(ok)
	assert.Equal(mA.Log, m.Log)
	assert.NotNil(m.cond)
	assert.NotZero(m.ceCounter)
	assert.Equal(&m.oUpgrader, m.upgrader.Object())
	assert.True(m.WSKeepAlive)

	// test watcher management functions
	newW := func(id string) *watcher {
		w := &watcher{}
		w.init(m, nil)
		w.id = id
		return w
	}
	w1 := newW("w1")
	w2 := newW("w2")
	w3 := newW("w3")
	w4 := newW("w4")
	m.watchers = append(m.watchers, w1, w2, w3, w4)

	// activate, purge expired
	tl.Flush()
	w4.activateByTime = time.Now().Add(-2 * WatcherActivationLimit)
	assert.False(w1.active)
	assert.False(w2.active)
	assert.False(w3.active)
	assert.False(w4.active)
	w := m.activateWatcher("w1")
	assert.NotNil(w)
	assert.Equal(w1, w)
	assert.True(w1.active)
	assert.False(w2.active)
	assert.Len(m.watchers, 3)
	w = m.activateWatcher("w4")
	assert.Nil(w)
	assert.Equal(1, tl.CountPattern("Watcher w4 expired"))
	stats := m.GetStats()
	assert.Equal(1, stats.NumWatchersActivated)
	assert.Equal(1, stats.NumActiveWatchers)

	// activate, protect against reuse
	w5 := newW("w5")
	w5.active = true
	m.watchers = append(m.watchers, w5)
	w = m.activateWatcher("w5")
	assert.Nil(w)
	stats = m.GetStats()
	assert.Equal(1, stats.NumWatchersActivated)
	assert.Equal(2, stats.NumActiveWatchers)

	// activate, not found
	w = m.activateWatcher("foo")
	assert.Nil(w)
	stats = m.GetStats()
	assert.Equal(1, stats.NumWatchersActivated)
	assert.Equal(2, stats.NumActiveWatchers)

	// remove
	inCS := false
	gotSignal := false
	checkMSignal := func(mux *sync.Mutex, cond *sync.Cond) {
		mux.Lock()
		inCS = true
		cond.Wait()
		gotSignal = true
		mux.Unlock()
	}
	go checkMSignal(&m.mux, m.cond)
	for inCS == false {
		time.Sleep(5 * time.Millisecond)
	}
	w2.active = true
	w2.ceQ = []*CrudEvent{&CrudEvent{}, &CrudEvent{}}
	assert.Len(m.watchers, 4)
	assert.Equal(w1, m.watchers[0])
	assert.Equal(w2, m.watchers[1])
	assert.Equal(w3, m.watchers[2])
	m.removeWatcher(w2)
	assert.Len(m.watchers, 3)
	assert.Equal(w1, m.watchers[0])
	assert.Equal(w3, m.watchers[1])
	assert.False(w2.active)
	assert.Len(w2.ceQ, 2)
	for _, ce := range w2.ceQ {
		assert.Nil(ce)
	}
	assert.True(w2.terminate)
	for gotSignal == false { // or hang forever
		time.Sleep(5 * time.Millisecond)
	}
	stats = m.GetStats()
	assert.Equal(1, stats.NumWatchersActivated)
	assert.Equal(2, stats.NumActiveWatchers)

	// dispatchEvent to active watcher
	assert.Equal(m, m.dispatcher)
	ce := &CrudEvent{
		Method:     "POST",
		TrimmedURI: "/accounts",
	}
	m.watchers = []*watcher{w1}
	m.accessManager = &fakeAccessControl{RetAllowed: true}
	w1.active = true
	assert.True(w1.match(ce))
	assert.Empty(w1.ceQ)
	now := time.Now()
	cnt := m.ceCounter
	inCS = false
	gotSignal = false
	go checkMSignal(&w1.mux, w1.cond)
	for inCS == false {
		time.Sleep(5 * time.Millisecond)
	}
	m.dispatchEvent(ce)
	assert.True(ce.Timestamp.After(now))
	assert.Equal(cnt, ce.Ordinal)
	assert.Equal(cnt+1, m.ceCounter)
	assert.Len(w1.ceQ, 1)
	assert.Equal(ce, w1.ceQ[0])
	for gotSignal == false { // or hang forever
		time.Sleep(5 * time.Millisecond)
	}
	stats = m.GetStats()
	tl.Logger().Infof("stats: %v", stats)
	tl.Flush()
	assert.Equal(1, stats.NumWatchersActivated)
	assert.Equal(1, stats.NumActiveWatchers)
	assert.Equal(1, stats.NumLocalEvents)
	assert.Equal(0, stats.NumUpstreamEvents)

	// dispatchEvent skips inactive watcher, use upstream event
	m.watchers = []*watcher{w3}
	w3.active = false
	assert.Empty(w3.ceQ)
	now = time.Now()
	ce = &CrudEvent{
		Method:       "PATCH",
		TrimmedURI:   "/accounts",
		FromUpstream: true,
		Timestamp:    now,
		Ordinal:      99999,
	}
	cnt = m.ceCounter
	assert.True(w3.match(ce))
	m.dispatchEvent(ce)
	assert.False(ce.Timestamp.After(now))
	assert.Equal(now, ce.Timestamp)
	assert.NotEqual(cnt, ce.Ordinal)
	assert.Equal(cnt, m.ceCounter) // unchanged
	assert.Empty(w3.ceQ)
	stats = m.GetStats()
	tl.Logger().Infof("stats: %v", stats)
	tl.Flush()
	assert.Equal(1, stats.NumWatchersActivated)
	assert.Equal(0, stats.NumActiveWatchers)
	assert.Equal(1, stats.NumLocalEvents)
	assert.Equal(1, stats.NumUpstreamEvents)

	// InjectEvent
	ce = &CrudEvent{Method: "PATCH", TrimmedURI: "/foo"}
	now = time.Now()
	cnt = m.ceCounter
	err := m.InjectEvent(ce)
	assert.NoError(err)
	assert.True(ce.Timestamp.After(now))
	assert.True(ce.Timestamp.After(now))
	assert.Equal(cnt, ce.Ordinal)
	assert.Equal(cnt+1, m.ceCounter)

	tcBadCE := []*CrudEvent{
		&CrudEvent{},
		&CrudEvent{Method: "GET"},
		&CrudEvent{Method: "POST"},
		&CrudEvent{Method: "PATCH"},
		&CrudEvent{Method: "DELETE"},
		&CrudEvent{Method: "PATCH", TrimmedURI: "foo"},
	}
	for i, tc := range tcBadCE {
		err = m.InjectEvent(tc)
		assert.Error(err, "%d", i)
	}
	tcGoodCE := []*CrudEvent{
		&CrudEvent{Method: "POST", TrimmedURI: "/foo"},
		&CrudEvent{Method: "PATCH", TrimmedURI: "/foo"},
		&CrudEvent{Method: "DELETE", TrimmedURI: "/foo"},
	}
	for i, tc := range tcGoodCE {
		err = tc.Validate()
		assert.NoError(err, "%d", i)
	}

	// terminate all
	tl.Flush()
	w1 = newW("w1")
	w2 = newW("w2")
	w3 = newW("w3")
	w4 = newW("w4")
	m.watchers = []*watcher{w1, w2, w3, w4}
	w1.active = true
	w3.terminate = true
	w4.active, w4.terminate = true, true
	tawStarted := false
	tawDone := false
	taw := func() {
		tawStarted = true
		m.TerminateAllWatchers()
		tawDone = true
	}
	go taw()
	for tawStarted == false {
		time.Sleep(5 * time.Millisecond)
	}
	for tl.CountPattern("Terminating all watchers") != 1 {
		time.Sleep(5 * time.Millisecond) // wait until TAW is in the mux
	}
	m.mux.Lock()
	// TAW now on a CV, inactive, not-terminating watcher has been removed from the list
	assert.Len(m.watchers, 3)
	for _, w := range m.watchers {
		assert.True(w.terminate)
		assert.NotEqual("w2", w.id)
	}
	m.mux.Unlock()
	m.removeWatcher(w1)
	m.removeWatcher(w3)
	m.removeWatcher(w4)
	for tawDone == false {
		time.Sleep(5 * time.Millisecond)
	}
	assert.Equal(1, tl.CountPattern("All watchers terminated"))

	// check the stat counters get returned
	m.activatedCount = 999
	m.localEventCount = 888
	m.upstreamEventCount = 777
	stats = m.GetStats()
	tl.Logger().Infof("stats: %v", stats)
	tl.Flush()
	assert.Equal(999, stats.NumWatchersActivated)
	assert.Equal(888, stats.NumLocalEvents)
	assert.Equal(777, stats.NumUpstreamEvents)

	s := stats.String()
	tl.Logger().Info("stats: %s", s)
	tl.Flush()
	expS := "{\"NumWatchersActivated\":999,\"NumActiveWatchers\":0,\"NumLocalEvents\":888,\"NumUpstreamEvents\":777,\"UpstreamMonitor\":{\"RetryCount\":0,\"PingCount\":0}}"
	assert.Equal(expS, s)
}

func TestClientURL(t *testing.T) {
	assert := assert.New(t)

	bp := nuvo.DefaultBasePath
	args := &mgmtclient.APIArgs{
		SocketPath:        "/foo",
		TLSCertificate:    "/a/cert",
		TLSCertificateKey: "/a/cert-key",
		Host:              "host",
		Port:              99,
	}
	exp := fmt.Sprintf("ws://localhost%s/watchers/ID", bp)
	assert.Equal(exp, ClientURL(args, "ID"))

	args.SocketPath = ""
	exp = fmt.Sprintf("wss://host:99%s/watchers/ID", bp)
	assert.Equal(exp, ClientURL(args, "ID"))

	args.TLSCertificate = ""
	exp = fmt.Sprintf("ws://host:99%s/watchers/ID", bp)
	assert.Equal(exp, ClientURL(args, "ID"))

	args.TLSCertificateKey = ""
	args.TLSCertificate = "notempty"
	exp = fmt.Sprintf("ws://host:99%s/watchers/ID", bp)
	assert.Equal(exp, ClientURL(args, "ID"))

	args.ForceTLS = true
	exp = fmt.Sprintf("wss://host:99%s/watchers/ID", bp)
	assert.Equal(exp, ClientURL(args, "ID"))
}

func TestEventID(t *testing.T) {
	assert := assert.New(t)

	ce := &CrudEvent{
		Method: "POST",
		Scope:  map[string]string{},
	}
	assert.Equal("", ce.ID())

	id := "514eacc3-4cbc-455c-8d30-0fb2c5e51f53"
	ce.Scope[ScopeMetaID] = id
	assert.Equal(id, ce.ID())

	ce.Method = "PATCH"
	assert.Equal("", ce.ID())
	ce.Method = "DELETE"
	assert.Equal("", ce.ID())

	ce.Scope = map[string]string{} // remove all scope properties

	ce.TrimmedURI = "/foo/bar/" + id
	ce.Method = "DELETE"
	assert.Equal(id, ce.ID())

	ce.TrimmedURI = "/foo/bar/" + id + "?set=x"
	ce.Method = "PATCH"
	assert.Equal(id, ce.ID())

}

func TestWatch(t *testing.T) {
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
	mObj := &models.CrudWatcherCreateArgs{}

	id, err := m.Watch(nil, nil)
	assert.Error(err)
	assert.Regexp("invalid arguments", err)
	id, err = m.Watch(mObj, nil)
	assert.Error(err)
	assert.Regexp("invalid arguments", err)

	// mObj errors handled in TestNewWatcher
	// activateWatcher tested in TestManager
	// deliverEvents tested in TestWatcherOperation
	assert.Len(m.watchers, 0)
	id, err = m.Watch(mObj, twc)
	assert.NoError(err)
	assert.NotEmpty(id)
	assert.Len(m.watchers, 1)

	// terminate the watcher
	m.TerminateWatcher(id)
	for len(m.watchers) > 0 {
		time.Sleep(5 * time.Millisecond)
	}
}

func TestMonitorUpstream(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)

	mA := &ManagerArgs{
		Log: tl.Logger(),
	}
	mo := NewManager(mA)
	assert.NotNil(mo)
	m, ok := mo.(*Manager)
	assert.True(ok)

	// invalid args
	err := m.MonitorUpstream(nil, nil, nil)
	assert.Error(err)
	assert.Regexp("invalid arguments", err)

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	err = m.MonitorUpstream(mAPI, nil, nil)
	assert.Error(err)
	assert.Regexp("invalid arguments", err)

	apiArgs := &mgmtclient.APIArgs{
		SocketPath: "/foo",
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	err = m.MonitorUpstream(mAPI, apiArgs, nil)
	assert.Error(err)
	assert.Regexp("invalid arguments", err)

	// success
	mockCtrl.Finish()
	assert.Nil(m.upMon)
	m.upMonStop = true // to force a stop
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	mObj := &models.CrudWatcherCreateArgs{}
	err = m.MonitorUpstream(mAPI, apiArgs, mObj)
	assert.NoError(err)
	for m.upMon != nil {
		time.Sleep(5 * time.Millisecond)
	}

	// bad dialer args
	apiArgs = &mgmtclient.APIArgs{
		TLSCertificate: "/bad/cert",
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	err = m.MonitorUpstream(mAPI, apiArgs, mObj)
	assert.Error(err)
	assert.Regexp("no such file or directory", err)

	// parse Args error
	mObj = &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{},
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	err = m.MonitorUpstream(mAPI, apiArgs, mObj)
	assert.Error(err)
	assert.Regexp("\\[0] no patterns specified", err)

	// already configured
	m.upMon = &upstreamMonitor{}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	err = m.MonitorUpstream(mAPI, apiArgs, mObj)
	assert.Error(err)
	assert.Regexp("already configured", err)
}

type fakeAccessControl struct {
	InReq   *http.Request
	RetAuth *auth.Info
	RetErr  *models.Error

	InS        auth.Subject
	InCE       *CrudEvent
	RetAllowed bool
}

func (fa *fakeAccessControl) GetAuth(req *http.Request) (auth.Subject, *models.Error) {
	fa.InReq = req
	return fa.RetAuth, fa.RetErr
}

func (fa *fakeAccessControl) EventAllowed(s auth.Subject, ce *CrudEvent) bool {
	fa.InS, fa.InCE = s, ce
	return fa.RetAllowed
}
