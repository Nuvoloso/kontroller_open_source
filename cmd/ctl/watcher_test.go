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
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/watchers"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	fws "github.com/Nuvoloso/kontroller/pkg/ws/fake"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestWatch(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	appCtx.apiArgs = &mgmtclient.APIArgs{
		Host: "h",
		Port: 80,
	}
	urlS := appCtx.MakeCrudeURL("id")
	assert.NotEmpty(urlS)
	assert.Regexp("h:80.*/id", urlS)

	// setup the websocket interceptors
	fd := &fws.FakeDialer{}
	appCtx.Dialer = fd
	fc := &fws.FakeConn{}
	fd.RetDConn = fc
	var rjV *models.CrudEvent
	numMsgs := -1
	fakeRJErr := fmt.Errorf("fake-RJ-error")
	fc.SetRJv = func(v interface{}) {
		if rjV != nil {
			ce, ok := v.(*models.CrudEvent)
			assert.True(ok, "ReadJSON type is *models.CrudEvent")
			*ce = *rjV
		}
		if numMsgs > 0 {
			numMsgs--
			if numMsgs == 0 {
				fc.RetRJErr = fakeRJErr
			}
		}
	}

	appCtx.Verbose = []bool{true}
	debugBuffer := &bytes.Buffer{}
	debugWriter = debugBuffer // eat up debug internally

	// watch, -1
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	aOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	aOps.EXPECT().GetAuthToken().Return("")
	mAPI.EXPECT().Authentication().Return(aOps)
	wOps := mockmgmtclient.NewMockWatchersClient(mockCtrl)
	mAPI.EXPECT().Watchers().Return(wOps)
	cP := watchers.NewWatcherCreateParams()
	cP.Payload = &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{
				MethodPattern: "MP",
				URIPattern:    "UP",
				ScopePattern:  "SP",
			},
		},
		Name: appCtx.MakeWatcherName(),
	}
	cRes := &watchers.WatcherCreateCreated{Payload: "w1"}
	wOps.EXPECT().WatcherCreate(cP).Return(cRes, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initWatcher()
	saveOW := outputWriter
	defer func() { outputWriter = saveOW }()
	var b bytes.Buffer
	outputWriter = &b
	rjV = &models.CrudEvent{
		Method:     "PATCH",
		TrimmedURI: "/fakeEvent",
		Scope:      map[string]string{"foo": "bar", "abc": "123"},
	}
	err := parseAndRun([]string{"watch", "-1", "-M", "MP", "-U", "UP", "-S", "SP"})
	assert.Nil(err)
	assert.Regexp("0001-01-01T00:00:00.000Z 0 PATCH /fakeEvent abc:123 foo:bar", b.String())

	// repeat with silent
	b.Reset()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	aOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	aOps.EXPECT().GetAuthToken().Return("")
	mAPI.EXPECT().Authentication().Return(aOps)
	wOps = mockmgmtclient.NewMockWatchersClient(mockCtrl)
	mAPI.EXPECT().Watchers().Return(wOps)
	wOps.EXPECT().WatcherCreate(cP).Return(cRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initWatcher()
	err = parseAndRun([]string{"watch", "--quit-on-first", "-q", "--method-pattern", "MP", "--uri-pattern", "UP", "--scope-pattern", "SP"})
	assert.Nil(err)
	assert.Empty(b.String())

	// repeat with JSON
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	aOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	aOps.EXPECT().GetAuthToken().Return("")
	mAPI.EXPECT().Authentication().Return(aOps)
	wOps = mockmgmtclient.NewMockWatchersClient(mockCtrl)
	mAPI.EXPECT().Watchers().Return(wOps)
	wOps.EXPECT().WatcherCreate(cP).Return(cRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initWatcher()
	err = parseAndRun([]string{"watch", "-1", "-o", "json", "-M", "MP", "-U", "UP", "-S", "SP"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(rjV, te.jsonData)

	// repeat with yaml, fake failure after first message
	numMsgs = 2
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	aOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	aOps.EXPECT().GetAuthToken().Return("")
	mAPI.EXPECT().Authentication().Return(aOps)
	wOps = mockmgmtclient.NewMockWatchersClient(mockCtrl)
	mAPI.EXPECT().Watchers().Return(wOps)
	wOps.EXPECT().WatcherCreate(cP).Return(cRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initWatcher()
	err = parseAndRun([]string{"watch", "-o", "yaml", "-M", "MP", "-U", "UP", "-S", "SP"})
	assert.Error(err)
	assert.Equal(fakeRJErr, err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(rjV, te.yamlData)
	numMsgs = -1
	fc.RetRJErr = nil

	// repeat with last-event file
	leFile := "./.watcherLastEventFile"
	leF := leFile
	if err := os.Remove(leF); err != nil && !os.IsNotExist(err) {
		assert.NoError(err)
		t.Logf("err: %T %v", err, err)
	}
	defer func() { os.Remove(leFile) }() // remove at end
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	aOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	aOps.EXPECT().GetAuthToken().Return("")
	mAPI.EXPECT().Authentication().Return(aOps)
	wOps = mockmgmtclient.NewMockWatchersClient(mockCtrl)
	mAPI.EXPECT().Watchers().Return(wOps)
	wOps.EXPECT().WatcherCreate(cP).Return(cRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initWatcher()
	err = parseAndRun([]string{"watch", "-1", "-M", "MP", "-U", "UP", "-S", "SP", "-L", leF})
	assert.Nil(err)
	leData, err := ioutil.ReadFile(leF)
	assert.NoError(err)
	assert.Regexp("PATCH /fakeEvent", string(leData))

	// last-event file failure
	// repeat with last-event file
	leF = "/ICannot/be/created"
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	aOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	aOps.EXPECT().GetAuthToken().Return("")
	mAPI.EXPECT().Authentication().Return(aOps)
	wOps = mockmgmtclient.NewMockWatchersClient(mockCtrl)
	mAPI.EXPECT().Watchers().Return(wOps)
	wOps.EXPECT().WatcherCreate(cP).Return(cRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initWatcher()
	err = parseAndRun([]string{"watch", "-1", "-M", "MP", "-U", "UP", "-S", "SP", "-L", leF})
	assert.Error(err)
	assert.Regexp("no such file or directory", err)

	// file args
	cP2 := watchers.NewWatcherCreateParams()
	cP2.Payload = &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{
				MethodPattern: "PATCH",
			},
			&models.CrudMatcher{
				MethodPattern: "POST",
				URIPattern:    "metrics/storage",
			},
			&models.CrudMatcher{
				MethodPattern: "DELETE",
				ScopePattern:  "foo",
			},
		},
		Name: "goodArgs",
	}
	wca := &models.CrudWatcherCreateArgs{}
	err = parseWatcherArgFile("./IDontExist", wca)
	assert.Error(err)
	assert.Regexp("no such file or directory", err)

	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initWatcher()
	err = parseAndRun([]string{"watch", "-1", "-o", "yaml", "-F", "./watcherBadArgs.json"})
	assert.Error(err)
	assert.Regexp("could not parse JSON", err)

	wm := mockmgmtclient.NewWatcherMatcher(t, cP2)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	aOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	aOps.EXPECT().GetAuthToken().Return("")
	mAPI.EXPECT().Authentication().Return(aOps)
	wOps = mockmgmtclient.NewMockWatchersClient(mockCtrl)
	mAPI.EXPECT().Watchers().Return(wOps)
	wOps.EXPECT().WatcherCreate(wm).Return(cRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initWatcher()
	err = parseAndRun([]string{"watch", "-1", "-o", "yaml", "-F", "./watcherGoodArgs.json"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(rjV, te.yamlData)

	// read json error
	fc.RetRJErr = fmt.Errorf("read-json-error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	aOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	aOps.EXPECT().GetAuthToken().Return("")
	mAPI.EXPECT().Authentication().Return(aOps)
	wOps = mockmgmtclient.NewMockWatchersClient(mockCtrl)
	mAPI.EXPECT().Watchers().Return(wOps)
	wOps.EXPECT().WatcherCreate(cP).Return(cRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initWatcher()
	err = parseAndRun([]string{"watch", "-M", "MP", "-U", "UP", "-S", "SP"})
	assert.Error(err)
	assert.Regexp("read-json-error", err)
	fc.RetRJErr = nil
	debugBuffer.Reset()

	// dialer error
	fd.RetDResp = &http.Response{}
	fd.RetDErr = fmt.Errorf("dialer-error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	aOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	aOps.EXPECT().GetAuthToken().Return("")
	mAPI.EXPECT().Authentication().Return(aOps)
	wOps = mockmgmtclient.NewMockWatchersClient(mockCtrl)
	mAPI.EXPECT().Watchers().Return(wOps)
	wOps.EXPECT().WatcherCreate(cP).Return(cRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initWatcher()
	err = parseAndRun([]string{"watch", "-M", "MP", "-U", "UP", "-S", "SP"})
	assert.Error(err)
	assert.Regexp("dialer-error", err)
	assert.NotEmpty(debugBuffer)

	// init dialer error
	appCtx.Dialer = nil
	appCtx.apiArgs.TLSCertificate = "/badCert"
	appCtx.apiArgs.TLSCertificateKey = "/badKey"
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	wOps = mockmgmtclient.NewMockWatchersClient(mockCtrl)
	mAPI.EXPECT().Watchers().Return(wOps)
	wOps.EXPECT().WatcherCreate(cP).Return(cRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initWatcher()
	err = parseAndRun([]string{"watch", "-M", "MP", "-U", "UP", "-S", "SP"})
	assert.Error(err)
	assert.Regexp("no such file or directory", err)

	// watcher create error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	wOps = mockmgmtclient.NewMockWatchersClient(mockCtrl)
	mAPI.EXPECT().Watchers().Return(wOps)
	wce := &watchers.WatcherCreateDefault{Payload: &models.Error{Code: 400, Message: swag.String("watcher-create-error")}}
	wOps.EXPECT().WatcherCreate(cP).Return(nil, wce)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initWatcher()
	err = parseAndRun([]string{"watch", "-M", "MP", "-U", "UP", "-S", "SP"})
	assert.Error(err)
	assert.Regexp("watcher-create-error", err)

	mockCtrl.Finish()
	t.Log("init context failure")
	debugBuffer.Reset()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "System").Return("", fmt.Errorf("ctx error"))
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initWatcher()
	err = parseAndRun([]string{"watch", "-A", "System", "-M", "MP", "-U", "UP", "-S", "SP"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""
	assert.NotEmpty(debugBuffer)

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initWatcher()
	err = parseAndRun([]string{"watch", "SP"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestWatchForChange(t *testing.T) {
	assert := assert.New(t)

	// Watch returns without error
	fw := &fakeWatcher{
		t:           t,
		CallCBCount: 1,
	}
	appCtx.watcher = fw
	fch := &fakeChangeHandler{
		t:     t,
		RetWA: &models.CrudWatcherCreateArgs{},
	}
	err := appCtx.WatchForChange(fch)
	assert.NoError(err)
	assert.Equal(1, fch.WACalled)
	assert.Equal(1, fch.CDCalled)

	// Watch returns with error
	fw = &fakeWatcher{
		t:           t,
		CallCBCount: 1,
		RetErr:      fmt.Errorf("watch-error"),
	}
	appCtx.watcher = fw
	fch = &fakeChangeHandler{
		t:     t,
		RetWA: &models.CrudWatcherCreateArgs{},
	}
	err = appCtx.WatchForChange(fch)
	assert.Error(err)
	assert.Regexp("watch-error", err)
	assert.Equal(1, fch.WACalled)
	assert.Equal(1, fch.CDCalled)

	// ChangeDetected returns an error.
	// Ensure that the Watch thread is entered after the error.
	ch := make(chan int)
	fw = &fakeWatcher{
		t:           t,
		CallCBCount: 2,
		BlockChan:   &ch,
	}
	appCtx.watcher = fw
	fch = &fakeChangeHandler{
		t:          t,
		RetWA:      &models.CrudWatcherCreateArgs{},
		RetCDErr:   fmt.Errorf("cd-error"),
		NotifyChan: &ch,
	}
	err = appCtx.WatchForChange(fch)
	assert.Error(err)
	assert.Regexp("cd-error", err)
	assert.Equal(1, fch.WACalled)
	assert.Equal(1, fch.CDCalled)
}

func TestWatchForChangeWithTicker(t *testing.T) {
	assert := assert.New(t)

	// Part I - Adapted from previous test

	// Watch returns without error
	fw := &fakeWatcher{
		t:           t,
		CallCBCount: 1,
	}
	appCtx.watcher = fw
	fch := &fakeChangeHandlerWithTicker{
		fakeChangeHandler: fakeChangeHandler{
			t:     t,
			RetWA: &models.CrudWatcherCreateArgs{},
		},
		Period: 10 * time.Second,
	}
	err := appCtx.WatchForChange(fch)
	assert.NoError(err)
	assert.Equal(1, fch.WACalled)
	assert.Equal(1, fch.CDCalled)

	// Watch returns with error
	fw = &fakeWatcher{
		t:           t,
		CallCBCount: 1,
		RetErr:      fmt.Errorf("watch-error"),
	}
	appCtx.watcher = fw
	fch = &fakeChangeHandlerWithTicker{
		fakeChangeHandler: fakeChangeHandler{
			t:     t,
			RetWA: &models.CrudWatcherCreateArgs{},
		},
		Period: 10 * time.Second,
	}
	err = appCtx.WatchForChange(fch)
	assert.Error(err)
	assert.Regexp("watch-error", err)
	assert.Equal(1, fch.WACalled)
	assert.Equal(1, fch.CDCalled)

	// ChangeDetected returns an error.
	// Ensure that the Watch thread is entered after the error.
	// Ensure that the timer is started before the callback and is terminated by the end
	ch := make(chan int)
	fw = &fakeWatcher{
		t:           t,
		CallCBCount: 2,
		BlockChan:   &ch,
	}
	appCtx.watcher = fw
	fch = &fakeChangeHandlerWithTicker{
		fakeChangeHandler: fakeChangeHandler{
			t:          t,
			RetWA:      &models.CrudWatcherCreateArgs{},
			RetCDErr:   fmt.Errorf("cd-error"),
			NotifyChan: &ch,
		},
		Period: 10 * time.Second,
	}
	cw := &changeWatcher{}
	cw.timer = time.AfterFunc(fch.Period, cw.tick)
	err = cw.WatchForChange(fch)
	assert.Error(err)
	assert.Regexp("cd-error", err)
	assert.Equal(1, fch.WACalled)
	assert.Equal(1, fch.CDCalled)
	assert.Nil(cw.timer)

	// Part II - Ticker specific

	// timed out
	ch = make(chan int)
	fw = &fakeWatcher{
		t:           t,
		CallCBCount: 1,
		BlockChan:   &ch,
	}
	appCtx.watcher = fw
	fch = &fakeChangeHandlerWithTicker{
		fakeChangeHandler: fakeChangeHandler{
			t:     t,
			RetWA: &models.CrudWatcherCreateArgs{},
		},
		Period:   200 * time.Microsecond,
		RetNCT:   fmt.Errorf("nct-error"),
		TickChan: &ch,
	}
	err = appCtx.WatchForChange(fch)
	assert.Error(err)
	assert.Regexp("nct-error", err)
	assert.Equal(1, fch.TickCnt)

	// invalid period
	fch = &fakeChangeHandlerWithTicker{
		fakeChangeHandler: fakeChangeHandler{
			t: t,
		},
	}
	err = appCtx.WatchForChange(fch)
	assert.Error(err)
	assert.Regexp("invalid period", err)

	// assert.False(true)
}

type fakeWatcher struct {
	t           *testing.T
	InArgs      *models.CrudWatcherCreateArgs
	InCB        watcherCallback
	CallCBCount int
	NumCalls    int
	CBErr       error
	RetErr      error
	BlockChan   *chan int
}

func (fw *fakeWatcher) Watch(ca *models.CrudWatcherCreateArgs, cb watcherCallback) error {
	var err error
	fw.InArgs = ca
	fw.InCB = cb
	for i := 0; i < fw.CallCBCount; i++ {
		fw.NumCalls++
		fw.t.Log("*** CALLING WCB ", i)
		if i == 0 {
			err = cb(nil)
		} else {
			ev := &models.CrudEvent{}
			err = cb(ev)
		}
		if err != nil {
			fw.CBErr = err
			fw.t.Logf("*** FW: returning CBErr %s", err.Error())
			return err
		}
		if fw.BlockChan != nil {
			select {
			case <-*fw.BlockChan:
			}
		}
	}
	fw.t.Logf("*** FW: returning RetErr %v", fw.RetErr)
	return fw.RetErr
}

type fakeChangeHandler struct {
	t          *testing.T
	WACalled   int
	RetWA      *models.CrudWatcherCreateArgs
	CDCalled   int
	RetCDErr   error
	NotifyChan *chan int // to terminate the fake watcher
}

func (c *fakeChangeHandler) WatcherArgs() *models.CrudWatcherCreateArgs {
	c.WACalled++
	c.t.Log("*** WA")
	return c.RetWA
}

func (c *fakeChangeHandler) ChangeDetected() error {
	c.CDCalled++
	c.t.Log("*** CD")
	if c.NotifyChan != nil {
		close(*c.NotifyChan)
	}
	return c.RetCDErr
}

type fakeChangeHandlerWithTicker struct {
	fakeChangeHandler
	Period   time.Duration
	TickCnt  int
	TickChan *chan int // to terminate the fake watcher
	RetNCT   error
}

func (c *fakeChangeHandlerWithTicker) NoChangeTick() error {
	c.t.Log("*** Tick")
	c.TickCnt++
	if c.TickChan != nil {
		close(*c.TickChan)
	}
	return c.RetNCT
}

func (c *fakeChangeHandlerWithTicker) NoChangeTickerPeriod() time.Duration {
	c.t.Log("*** Period:", c.Period)
	return c.Period
}
