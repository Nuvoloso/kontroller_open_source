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
	"net/url"
	"testing"
	"time"

	nuvo "github.com/Nuvoloso/kontroller/pkg/autogen/client"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/watchers"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	fws "github.com/Nuvoloso/kontroller/pkg/ws/fake"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

func TestCrudMiddleware(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	props := ScopeMap{
		"prop1": "value1",
		"prop2": "",
		"prop3": "value3",
	}
	expScopeS := "prop1:value1 prop2: prop3:value3"
	ct := &collectorTester{
		props: props,
	}

	mA := &ManagerArgs{
		Log: tl.Logger(),
	}
	mo := NewManager(mA)
	handlerChain := mo.CollectorMiddleware(ct)
	m := mo.(*Manager)
	m.dispatcher = ct

	// Must survive invalid usage
	assert.NotPanics(func() { mo.SetScope(nil, nil, nil) })
	assert.NotPanics(func() { mo.SetScope(&http.Request{}, nil, nil) })

	relURI := "/object-type?query=hi"
	u, err := url.Parse("http://host.com" + nuvo.DefaultBasePath + relURI)
	assert.Nil(err)
	assert.NotNil(u)
	tl.Logger().Infof("URL: %s", u.String())
	tl.Logger().Infof("RequestURI: %s", u.RequestURI())
	tl.Logger().Infof("TrimmedRequestURI: %s", m.TrimmedRequestURI(u))
	assert.Equal(relURI, m.TrimmedRequestURI(u))
	req := &http.Request{
		Method: "GET",
		URL:    u,
	}

	ct.ev = nil
	ct.statusCode = 200
	ct.whCalled = false
	handlerChain.ServeHTTP(ct, req)
	assert.True(ct.serveCalled)
	assert.True(ct.whCalled)
	assert.Nil(ct.ev)

	req.Method = "POST"
	ct.ev = nil
	ct.statusCode = 404
	ct.whCalled = false
	handlerChain.ServeHTTP(ct, req)
	assert.True(ct.serveCalled)
	assert.True(ct.whCalled)
	assert.Nil(ct.ev)

	sc := 200
	for _, method := range []string{"POST", "PATCH", "DELETE"} {
		tl.Flush()
		req.Method = method
		ct.statusCode = sc
		ct.ev = nil
		ct.whCalled = false
		handlerChain.ServeHTTP(ct, req)
		assert.True(ct.serveCalled)
		assert.True(ct.whCalled)
		assert.NotNil(ct.ev)
		ev := ct.ev
		assert.Equal(method, ev.Method)
		assert.Equal(relURI, ev.TrimmedURI)
		assert.Equal(props, ev.Scope)
		assert.Equal(expScopeS, ev.ScopeS)
		assert.Equal(method, ev.AccessControlScope)
		assert.False(ev.FromUpstream)
		sc++
	}

	// calls to /watcher[/*] are skipped
	skippedPaths := []string{"", "/id"}
	for _, path := range skippedPaths {
		u, err = url.Parse("http://host.com" + nuvo.DefaultBasePath + WatcherURLRelPath + path)
		assert.NoError(err)
		tl.Logger().Infof("Skip %s", m.TrimmedRequestURI(u))
		ct.ev = nil
		ct.whCalled = false
		req.URL = u
		handlerChain.ServeHTTP(ct, req)
		assert.True(ct.whCalled)
		assert.Nil(ct.ev)
	}
	// call to /watcher-foo (non-existent path) is not skipped
	u, err = url.Parse("http://host.com" + nuvo.DefaultBasePath + WatcherURLRelPath + "-foo")
	assert.NoError(err)
	tl.Logger().Infof("not skipped %s", m.TrimmedRequestURI(u))
	ct.ev = nil
	ct.whCalled = false
	req.URL = u
	handlerChain.ServeHTTP(ct, req)
	assert.True(ct.whCalled)
	assert.NotNil(ct.ev)
}

func TestUpstreamMonitor(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	mA := &ManagerArgs{
		Log: tl.Logger(),
	}
	mo := NewManager(mA)
	assert.NotNil(mo)
	m, ok := mo.(*Manager)
	assert.True(ok)
	assert.False(m.upMonStop)

	mObj := &models.CrudWatcherCreateArgs{}
	apiArgs := &mgmtclient.APIArgs{}
	u := &upstreamMonitor{
		m:                 m,
		mObj:              mObj,
		apiArgs:           apiArgs,
		retryBackoffSleep: time.Duration(5 * time.Millisecond),
		retryBackoff:      time.Duration(10 * time.Millisecond),
	}

	// exercise the backoff logic by forcing failure in monitor()
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	wOps := mockmgmtclient.NewMockWatchersClient(mockCtrl)
	mAPI.EXPECT().Watchers().Return(wOps).MinTimes(1)
	cP := watchers.NewWatcherCreateParams()
	cP.Payload = mObj
	e := &watchers.WatcherCreateDefault{}
	e.Payload = &models.Error{Code: 404, Message: swag.String("create-fail")}
	wOps.EXPECT().WatcherCreate(cP).Return(nil, e).MinTimes(1)
	u.api = mAPI
	m.upMon = u
	go u.run()
	// validate that retry happens
	for u.retryCount < 2 {
		time.Sleep(5 * time.Millisecond)
	}
	fc := &fws.FakeConn{}
	u.conn = fc
	u.connOpen = true
	m.TerminateAllWatchers()
	assert.True(fc.CalledC)
	// wait till the run handler exits
	for m.upMon != nil {
		time.Sleep(5 * time.Millisecond)
	}

	//
	// directly call monitor()
	//

	// fail in dial
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	wOps = mockmgmtclient.NewMockWatchersClient(mockCtrl)
	mAPI.EXPECT().Watchers().Return(wOps).MinTimes(1)
	cRes := &watchers.WatcherCreateCreated{Payload: "w1"}
	wOps.EXPECT().WatcherCreate(cP).Return(cRes, nil)
	u.api = mAPI
	fd := &fws.FakeDialer{}
	u.dialer = fd
	fd.RetDCErr = fmt.Errorf("fake-dialer-err")
	tB := time.Now()
	err := u.monitor()
	tA := time.Now()
	assert.Error(err)
	assert.Regexp("fake-dialer-err", err)
	assert.Regexp("/watchers/w1", fd.InDCUrl)
	assert.NotNil(fd.InDCctx)
	dt, ok := fd.InDCctx.Deadline()
	assert.True(ok)
	assert.True(tB.Before(dt))
	assert.True(tA.Add(UpstreamMonitorConnectionTimeout).After(dt))

	// fail in ReadJSON
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	wOps = mockmgmtclient.NewMockWatchersClient(mockCtrl)
	mAPI.EXPECT().Watchers().Return(wOps).MinTimes(1)
	wOps.EXPECT().WatcherCreate(cP).Return(cRes, nil)
	u.api = mAPI
	fd = &fws.FakeDialer{}
	u.dialer = fd
	fc = &fws.FakeConn{}
	fd.RetDCConn = fc
	fc.RetRJErr = fmt.Errorf("read-json-err")
	err = u.monitor()
	assert.Error(err)
	assert.Regexp("read-json-err", err)

	// dispatch
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	wOps = mockmgmtclient.NewMockWatchersClient(mockCtrl)
	mAPI.EXPECT().Watchers().Return(wOps).MinTimes(1)
	wOps.EXPECT().WatcherCreate(cP).Return(cRes, nil)
	u.api = mAPI
	fd = &fws.FakeDialer{}
	u.dialer = fd
	fc = &fws.FakeConn{}
	fd.RetDCConn = fc
	var rjV *models.CrudEvent
	fc.SetRJv = func(v interface{}) {
		if rjV != nil {
			ce, ok := v.(*models.CrudEvent)
			assert.True(ok, "ReadJSON type is *models.CrudEvent")
			*ce = *rjV
		}
	}
	fc.BlockRJ = true
	ct := &collectorTester{}
	m.dispatcher = ct
	runStopped := false
	go func() {
		err = u.monitor()
		runStopped = true
	}()
	// inject an event
	for !fc.ChanRJMade {
		time.Sleep(5 * time.Millisecond)
	}
	fc.ChanRJMade = false
	assert.Nil(ct.ev)
	rjV = &models.CrudEvent{
		Method:     "POST",
		TrimmedURI: "/foo?bar",
		Ordinal:    99,
		Scope:      map[string]string{"foo": "bar"},
	}
	close(fc.ChanRJ)
	// wait till read blocks again
	for !fc.ChanRJMade {
		time.Sleep(5 * time.Millisecond)
	}
	assert.NotNil(ct.ev)
	expCE := &CrudEvent{
		Method:       "POST",
		TrimmedURI:   "/foo?bar",
		Ordinal:      99,
		Scope:        map[string]string{"foo": "bar"},
		ScopeS:       "foo:bar",
		FromUpstream: true,
	}
	assert.Equal(expCE, ct.ev)
	// inject error to break the loop
	fc.RetRJErr = fmt.Errorf("read-json-err2")
	close(fc.ChanRJ)
	for runStopped == false {
		time.Sleep(5 * time.Millisecond)
	}
	assert.Error(err)
	assert.Regexp("read-json-err2", err)

	// stats returned by manager
	m.upMon = u
	u.pingCount = 888
	u.retryCount = 999
	stats := m.GetStats()
	assert.Equal(u.pingCount, stats.UpstreamMonitor.PingCount)
	assert.Equal(u.retryCount, stats.UpstreamMonitor.RetryCount)
	s := stats.String()
	tl.Logger().Info("stats: %s", s)
	tl.Flush()
	expS := "{\"NumWatchersActivated\":0,\"NumActiveWatchers\":0,\"NumLocalEvents\":0,\"NumUpstreamEvents\":0,\"UpstreamMonitor\":{\"RetryCount\":999,\"PingCount\":888}}"
	assert.Equal(expS, s)
}

func TestUpstreamMonitorPing(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	mA := &ManagerArgs{
		Log: tl.Logger(),
	}
	mo := NewManager(mA)
	assert.NotNil(mo)
	m, ok := mo.(*Manager)
	assert.True(ok)
	assert.False(m.upMonStop)

	mObj := &models.CrudWatcherCreateArgs{}
	apiArgs := &mgmtclient.APIArgs{}
	u := &upstreamMonitor{
		m:                 m,
		mObj:              mObj,
		apiArgs:           apiArgs,
		retryBackoffSleep: time.Duration(5 * time.Millisecond),
		retryBackoff:      time.Duration(10 * time.Millisecond),
	}

	// pingStart sets period, started, etc.
	assert.EqualValues(0, u.pingPeriod)
	assert.False(u.pingStarted)
	go u.pingStart()
	for u.pingStarted == false {
		time.Sleep(5 * time.Millisecond)
	}
	u.pingStop()
	for u.pingStarted {
		time.Sleep(5 * time.Millisecond)
	}
	assert.Equal(WSPingPeriod, u.pingPeriod)

	// use a smaller period for testing
	tB := time.Now()
	u.pingPeriod = time.Millisecond * 2
	fc := &fws.FakeConn{}
	fc.RetWMErr = fmt.Errorf("fake-wm-err")
	u.conn = fc
	u.connOpen = true
	u.pingCount = 0
	go u.pingStart()
	for u.pingCount == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	u.pingStop()
	for u.pingStarted {
		time.Sleep(5 * time.Millisecond)
	}
	tA := time.Now()
	assert.Equal(websocket.PingMessage, fc.InWMmt)
	assert.True(fc.CalledC)
	assert.True(tB.Before(fc.InSWDt))                     // bracket the SetWriteDeadline time
	assert.True(tA.Add(WSWriteDeadline).After(fc.InSWDt)) // bracket the SetWriteDeadline time
	assert.Equal(0, u.pingCount)

	ums := u.getStats()
	assert.Equal(u.pingCount, ums.PingCount)
}

// collectorTester provides the following interfaces:
//  - http.Handler (ServeHTTP)
//  - http.ResponseWriter (Header, Write, WriteHeader)
type collectorTester struct {
	serveCalled bool
	whCalled    bool
	statusCode  int
	props       ScopeMap
	ev          *CrudEvent
}

func (h *collectorTester) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.serveCalled = true
	SetScope(r, h.props, r.Method)
	w.WriteHeader(h.statusCode)
}

func (h *collectorTester) Header() http.Header {
	return nil
}

func (h *collectorTester) Write([]byte) (int, error) {
	return 0, nil
}

func (h *collectorTester) WriteHeader(int) {
	h.whCalled = true
}

func (h *collectorTester) dispatchEvent(ev *CrudEvent) {
	h.ev = ev
}
