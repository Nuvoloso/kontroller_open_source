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
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/watchers"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	fws "github.com/Nuvoloso/kontroller/pkg/ws/fake"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

func TestRegisterHandlers(t *testing.T) {
	assert := assert.New(t)

	api := &operations.NuvolosoAPI{}
	assert.Nil(api.WatchersWatcherCreateHandler)
	assert.Nil(api.WatchersWatcherFetchHandler)
	fa := &fakeAccessControl{}

	m := Manager{}
	m.RegisterHandlers(api, fa)
	assert.NotNil(api.WatchersWatcherCreateHandler)
	assert.NotNil(api.WatchersWatcherFetchHandler)
	assert.Equal(fa, m.accessManager)
}

func TestRemoteWatcher(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)

	mA := &ManagerArgs{
		Log: tl.Logger(),
	}
	mo := NewManager(mA)
	assert.NotNil(mo)
	m, ok := mo.(*Manager)
	assert.True(ok)
	assert.False(m.WSKeepAlive)

	// create fails
	cParams := watchers.WatcherCreateParams{
		Payload: &models.CrudWatcherCreateArgs{
			Matchers: []*models.CrudMatcher{
				&models.CrudMatcher{}, // invalid
			},
		},
	}
	cRet := m.watcherCreate(cParams)
	wcd, ok := cRet.(*watchers.WatcherCreateDefault)
	assert.True(ok)
	assert.NotNil(wcd.Payload)
	e := wcd.Payload
	assert.EqualValues(400, e.Code)
	assert.Regexp("invalid.*\\[0] no patterns specified", swag.StringValue(e.Message))

	// create succeeds
	cParams = watchers.WatcherCreateParams{
		Payload: &models.CrudWatcherCreateArgs{
			Matchers: []*models.CrudMatcher{},
		},
	}
	assert.Empty(m.watchers)
	cRet = m.watcherCreate(cParams)
	wcc, ok := cRet.(*watchers.WatcherCreateCreated)
	assert.True(ok)
	id := string(wcc.Payload)
	assert.NotEmpty(id)
	assert.Len(m.watchers, 1)
	w := m.watchers[0]
	assert.Equal(w.id, id)
	assert.False(w.active)

	// validate the data type
	wsw, ok := w.client.(*wsWatcher)
	assert.True(ok)
	assert.Nil(wsw.w)
	assert.Equal(WSPingPeriod, wsw.pingPeriod)

	// fetch fails (invalid id)
	fParams := watchers.WatcherFetchParams{
		HTTPRequest: &http.Request{},
		ID:          "foo", // invalid id
	}
	fac := &fakeAccessControl{}
	m.accessManager = fac
	fRet := m.watcherFetch(fParams)
	assert.False(w.active)
	wfd, ok := fRet.(*watchers.WatcherFetchDefault)
	assert.True(ok)
	assert.NotNil(wcd.Payload)
	e = wfd.Payload
	assert.EqualValues(404, e.Code)
	assert.Regexp("object not found", swag.StringValue(e.Message))

	// fetch fails, auth internal error
	fac.RetErr = &models.Error{Code: 500, Message: swag.String("internal error")}
	fRet = m.watcherFetch(fParams)
	assert.False(w.active)
	wfd, ok = fRet.(*watchers.WatcherFetchDefault)
	assert.True(ok)
	assert.NotNil(wcd.Payload)
	e = wfd.Payload
	assert.EqualValues(500, e.Code)
	assert.Regexp("internal error", swag.StringValue(e.Message))
	fac.RetErr = nil

	// fetch fails, not a wsWatcher
	fParams = watchers.WatcherFetchParams{
		HTTPRequest: &http.Request{},
		ID:          id,
	}
	w.client = &testWatcherClient{}
	fRet = m.watcherFetch(fParams)
	assert.True(w.active)
	wfd, ok = fRet.(*watchers.WatcherFetchDefault)
	assert.True(ok)
	assert.NotNil(wcd.Payload)
	e = wfd.Payload
	assert.EqualValues(404, e.Code)
	assert.Regexp("object not found", swag.StringValue(e.Message))
	w.client = wsw   // restore
	w.active = false // restore

	// fetch succeeds
	fRet = m.watcherFetch(fParams)
	assert.True(w.active)
	rf, ok := fRet.(middleware.ResponderFunc)
	assert.True(ok)
	assert.NotNil(rf)
	assert.Equal(w, wsw.w) // now set

	// test the responder function with fake web sockets
	fu := &fws.FakeUpgrader{}
	m.upgrader = fu
	fu.RetUErr = fmt.Errorf("fake-upgrader-error")
	rf(nil, nil)
	assert.True(fu.CalledU)

	// use a goroutine for delivery
	fc := &fws.FakeConn{}
	fu.RetUConn = fc
	fu.RetUErr = nil
	fu.CalledU = false
	dFuncStarted := false
	dFuncStopped := false
	dFunc := func() {
		dFuncStarted = true
		rf(nil, nil)
		dFuncStopped = true
	}
	assert.False(fc.ChanRMMade)
	go dFunc()
	for !dFuncStarted {
		time.Sleep(5 * time.Millisecond)
	}
	assert.True(fu.CalledU)
	assert.True(w.active)
	// wait for the wsRecv thread to block
	for !fc.ChanRMMade {
		time.Sleep(5 * time.Millisecond)
	}
	assert.NotNil(fc.InSCHh)
	ch1 := fmt.Sprintf("%p", wsw.closeHandler)
	ch2 := fmt.Sprintf("%p", fc.InSCHh)
	assert.Equal(ch1, ch2) // VERY DICEY - function comparison not supported
	assert.True(fc.InSRDt.IsZero())

	// simulate client closure
	now := time.Now()
	wsw.closeHandler(1005, "")
	fc.RetRMErr = fmt.Errorf("read-err")
	close(fc.ChanRM)
	for !dFuncStopped {
		time.Sleep(5 * time.Millisecond)
	}
	assert.True(fc.InSWDt.After(now))
	assert.Equal(fc.InWMmt, websocket.CloseMessage)
	assert.True(w.terminate)
	assert.False(w.active)
	assert.Nil(wsw.ticker)       // no keep alive
	assert.Equal(0, wsw.pingCnt) // no keep alive

	// test the callback function directly
	ce := &CrudEvent{
		Timestamp:  time.Now(),
		Ordinal:    10,
		Method:     "PATCH",
		TrimmedURI: "/foo",
		Scope:      map[string]string{"foo": "bar"},
	}
	fc.RetWJErr = fmt.Errorf("write-json-err")
	err := wsw.CrudeNotify(WatcherEvent, ce)
	assert.Error(err)
	assert.Regexp("write-json-err", err)
	assert.NotNil(fc.InWJv)
	ev, ok := fc.InWJv.(*models.CrudEvent)
	assert.True(ok)
	expEv := &models.CrudEvent{
		Timestamp:  strfmt.DateTime(ce.Timestamp),
		Ordinal:    ce.Ordinal,
		Method:     ce.Method,
		TrimmedURI: ce.TrimmedURI,
		Scope:      ce.Scope,
	}
	assert.Equal(expEv, ev)

	// repeat with no error
	fc.RetWJErr = nil
	fc.InWJv = nil
	err = wsw.CrudeNotify(WatcherEvent, ce)
	assert.NoError(err)
	assert.NotNil(fc.InWJv)
	ev, ok = fc.InWJv.(*models.CrudEvent)
	assert.True(ok)
	assert.Equal(expEv, ev)

	//
	// test the ping handler
	//
	mA = &ManagerArgs{
		WSKeepAlive: true,
		Log:         tl.Logger(),
	}
	mo = NewManager(mA)
	assert.NotNil(mo)
	m, ok = mo.(*Manager)
	assert.True(ok)
	assert.True(m.WSKeepAlive)
	fu = &fws.FakeUpgrader{}
	m.upgrader = fu
	m.accessManager = &fakeAccessControl{}

	cRet = m.watcherCreate(cParams)
	wcc, ok = cRet.(*watchers.WatcherCreateCreated)
	assert.True(ok)
	fParams.ID = string(wcc.Payload)
	assert.Len(m.watchers, 1)
	w = m.watchers[0]
	wsw, ok = w.client.(*wsWatcher)
	assert.True(ok)
	assert.Nil(wsw.w)
	// override the ping period
	assert.Equal(WSPingPeriod, wsw.pingPeriod)
	wsw.pingPeriod = time.Duration(20 * time.Millisecond)

	fRet = m.watcherFetch(fParams)
	assert.True(w.active)
	rf, ok = fRet.(middleware.ResponderFunc)
	assert.True(ok)
	assert.NotNil(rf)
	assert.Equal(w, wsw.w) // now set

	fc = &fws.FakeConn{}
	fu.RetUConn = fc
	fu.RetUErr = nil
	fu.CalledU = false
	dFuncStarted = false
	dFuncStopped = false
	go dFunc()
	for !dFuncStarted {
		time.Sleep(5 * time.Millisecond)
	}
	// wait for the wsRecv thread to block
	for !fc.ChanRMMade {
		time.Sleep(5 * time.Millisecond)
	}

	// check that the ping handler gets called at least twice
	for wsw.pingCnt < 2 {
		time.Sleep(20 * time.Millisecond)
	}
	assert.NotNil(wsw.ticker) // keep alive

	// terminate
	now = time.Now()
	wsw.closeHandler(1005, "")
	fc.RetRMErr = fmt.Errorf("read-err")
	close(fc.ChanRM)
	for !dFuncStopped {
		time.Sleep(5 * time.Millisecond)
	}

	// test the pingHandler isolated
	w.terminate = false
	fc.RetWMErr = nil
	fc.InWMmt = -1
	fc.InSWDt = now
	wsw.pingTimer()
	assert.True(fc.InSWDt.After(now))
	assert.Equal(websocket.PingMessage, fc.InWMmt)
	assert.False(w.terminate)

	fc.RetWMErr = fmt.Errorf("write-message-error")
	wsw.pingTimer()
	assert.True(w.terminate)

	fc.RetWMErr = nil
	fc.InWMmt = -1
	fc.InSWDt = now
	wsw.pingTimer()
	assert.True(w.terminate)
	assert.Equal(-1, fc.InWMmt)
	assert.Equal(now, fc.InSWDt)
}
