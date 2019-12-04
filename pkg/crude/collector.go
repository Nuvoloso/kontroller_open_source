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
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	nuvo "github.com/Nuvoloso/kontroller/pkg/autogen/client"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/watchers"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/ws"
	"github.com/gorilla/websocket"
)

type crudContextKeyType struct{}

// crudContextKey is the key of the context property containing CRUD data
var crudContextKey = crudContextKeyType{}

// CollectorMiddleware collects data from successful CRUD operations
func (m *Manager) CollectorMiddleware(next http.Handler) http.Handler {
	reSkip := regexp.MustCompile("^" + WatcherURLRelPath + "(/.*)?$")
	fn := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			ev := &CrudEvent{}
			ev.Scope = make(ScopeMap)
			ctx := context.WithValue(r.Context(), crudContextKey, ev)
			sW := util.NewStatusExtractorResponseWriter(w)
			next.ServeHTTP(sW, r.WithContext(ctx))
			if sW.StatusSuccess() {
				ev.Method = r.Method
				ev.TrimmedURI = m.TrimmedRequestURI(r.URL)
				if !reSkip.MatchString(ev.TrimmedURI) {
					m.dispatcher.dispatchEvent(ev.cook())
				}
			}
		} else {
			next.ServeHTTP(w, r)
		}
	}
	return http.HandlerFunc(fn)
}

// TrimmedRequestURI is the portion of the URL path that is stored in the CrudEvent
func (m *Manager) TrimmedRequestURI(u *url.URL) string {
	return strings.TrimPrefix(u.RequestURI(), nuvo.DefaultBasePath)
}

// SetScope is used by CRUD handlers to add to scope/contextual information.
// All scope properties are string name-value pairs.
// The accessScope is a separate opaque interface used by the EventAllowed callback and is not passed on the wire.
func SetScope(r *http.Request, props ScopeMap, accessScope interface{}) {
	if r == nil || props == nil {
		return
	}
	ctx := r.Context()
	if v := ctx.Value(crudContextKey); v != nil {
		ev := v.(*CrudEvent)
		ev.AccessControlScope = accessScope
		for pN, pV := range props {
			ev.Scope[pN] = pV
		}
	}
}

// SetScope is the method form of SetScope() which is more useful for handler UTs
func (m *Manager) SetScope(r *http.Request, props ScopeMap, accessScope interface{}) {
	SetScope(r, props, accessScope)
}

// upstream monitor constants
const (
	UpstreamMonitorBackoff           = time.Duration(5 * time.Second)
	UpstreamMonitorBackoffSleep      = time.Duration(100 * time.Millisecond)
	UpstreamMonitorConnectionTimeout = time.Duration(20 * time.Second)
)

// upstreamMonitor represents a collector of upstream CRUD events
type upstreamMonitor struct {
	m                 *Manager
	api               mgmtclient.API
	dialer            ws.Dialer
	apiArgs           *mgmtclient.APIArgs
	mObj              *models.CrudWatcherCreateArgs
	conn              ws.Conn
	connMux           sync.Mutex
	connOpen          bool
	retryBackoff      time.Duration
	retryBackoffSleep time.Duration
	retryCount        int
	pingCount         int
	pingPeriod        time.Duration
	pingChan          chan bool
	pingStarted       bool
}

// UpstreamMonitorStats returns statistics on the upstream monitor
type UpstreamMonitorStats struct {
	RetryCount int
	PingCount  int
}

// run implements the upstream monitor. It should be called in a goroutine.
func (u *upstreamMonitor) run() {
	if int64(u.retryBackoffSleep) == 0 {
		u.retryBackoff = UpstreamMonitorBackoff
		u.retryBackoffSleep = UpstreamMonitorBackoffSleep
	}
	for !u.m.upMonStop {
		u.m.Log.Debugf("Upstream monitor: starting %d", u.retryCount)
		u.monitor()
		endTime := time.Now().Add(u.retryBackoff)
		u.m.Log.Debugf("Upstream monitor: %d retry after %s", u.retryCount, endTime)
		for !u.m.upMonStop && time.Now().Before(endTime) {
			time.Sleep(u.retryBackoffSleep)
		}
		u.retryCount++
	}
	// notify the manager of our demise
	u.m.mux.Lock()
	u.m.Log.Debugf("Upstream monitor: %d terminated", u.retryCount)
	u.m.upMon = nil
	u.m.cond.Signal()
	u.m.mux.Unlock()
}

// getStats returns the current counter values
func (u *upstreamMonitor) getStats() UpstreamMonitorStats {
	return UpstreamMonitorStats{
		RetryCount: u.retryCount,
		PingCount:  u.pingCount,
	}
}

// stopInLock closes the upstream connection.
// Must be called within the manager lock to protect the m.upMon pointer.
func (u *upstreamMonitor) stopInLock() {
	u.m.Log.Debugf("Upstream monitor: stop")
	u.m.upMonStop = true
	u.closeConn()
}

// closeConn controls the concurrent invocation of conn.Close.
// We do not want to clear the u.conn field to avoid panics.
func (u *upstreamMonitor) closeConn() {
	u.connMux.Lock()
	defer u.connMux.Unlock()
	if u.connOpen {
		u.conn.Close()
		u.connOpen = false
	}
}

// monitor creates a Watcher on the upstream source.
func (u *upstreamMonitor) monitor() error {
	m := u.m
	params := watchers.NewWatcherCreateParams()
	params.Payload = u.mObj
	resC, err := u.api.Watchers().WatcherCreate(params)
	if err != nil {
		if e, ok := err.(*watchers.WatcherCreateDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		m.Log.Errorf("Upstream monitor: %d WatcherCreate: %s", u.retryCount, err.Error())
		return err
	}
	url := ClientURL(u.apiArgs, string(resC.Payload))
	ctx, cancel := context.WithTimeout(context.Background(), UpstreamMonitorConnectionTimeout)
	defer cancel()
	u.conn, _, err = u.dialer.DialContext(ctx, url, nil)
	if err != nil {
		m.Log.Errorf("Upstream monitor: %d Dial: %s", u.retryCount, err.Error())
		return err
	}
	u.connOpen = true
	defer u.closeConn()
	defer u.pingStop()
	go util.PanicLogger(u.m.Log, u.pingStart)
	for {
		ev := &models.CrudEvent{}
		ce := &CrudEvent{}
		if err = u.conn.ReadJSON(ev); err != nil {
			m.Log.Errorf("Upstream monitor: %d ReadJSON: %s", u.retryCount, err.Error())
			return err
		}
		ce.FromModel(ev)
		ce.FromUpstream = true
		m.dispatcher.dispatchEvent(ce)
	}
}

func (u *upstreamMonitor) pingStop() {
	if u.pingStarted {
		u.pingChan <- true
	}
}

// pingStart periodically issues a "Ping" control message until pingStop() is called
func (u *upstreamMonitor) pingStart() {
	u.pingChan = make(chan bool)
	if u.pingPeriod == 0 {
		u.pingPeriod = WSPingPeriod
	}
	u.m.Log.Debugf("Upstream monitor: %d ping starting: %s", u.retryCount, u.pingPeriod)
	ticker := time.NewTicker(u.pingPeriod)
	u.pingCount = 0
	u.pingStarted = true
	for u.pingStarted {
		select {
		case <-u.pingChan:
			u.pingStarted = false
			u.m.Log.Debugf("Upstream monitor: %d ping stopping", u.retryCount)
		case <-ticker.C:
			u.pingCount++
			err := u.conn.SetWriteDeadline(time.Now().Add(WSWriteDeadline))
			if err == nil {
				err = u.conn.WriteMessage(websocket.PingMessage, []byte{})
			}
			if err != nil {
				u.m.Log.Errorf("Upstream monitor: %d WriteMessage %d: %s", u.retryCount, u.pingCount, err.Error())
				u.closeConn() // force an error in the read loop
			}
		}
	}
	ticker.Stop()
	u.m.Log.Debugf("Upstream monitor: %d ping stopped: %d", u.retryCount, u.pingCount)
	u.pingCount = 0
}
