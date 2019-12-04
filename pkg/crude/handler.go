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
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/watchers"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/ws"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
	"github.com/gorilla/websocket"
)

// Websocket related constants
const (
	WSPingPeriod    = time.Duration(45 * time.Second) // nginx timeout is 60s
	WSWriteDeadline = time.Duration(10 * time.Second)
)

// wsWatcher represents a websocket watcher client
type wsWatcher struct {
	w          *watcher
	pingPeriod time.Duration
	pingCnt    int
	mux        sync.Mutex // used for write synchronization
	conn       ws.Conn
	ticker     *time.Ticker
}

// RegisterHandlers registers the Watcher handlers and the access control manager
func (m *Manager) RegisterHandlers(api *operations.NuvolosoAPI, accessManager AccessControl) {
	m.accessManager = accessManager
	api.WatchersWatcherCreateHandler = watchers.WatcherCreateHandlerFunc(m.watcherCreate)
	api.WatchersWatcherFetchHandler = watchers.WatcherFetchHandlerFunc(m.watcherFetch)
}

// watcherCreate is the REST API handler
func (m *Manager) watcherCreate(params watchers.WatcherCreateParams) middleware.Responder {
	id, err := m.newWatcher(params.Payload, &wsWatcher{pingPeriod: WSPingPeriod})
	if err != nil {
		msg := fmt.Sprintf(com.ErrorInvalidData+": %s", err.Error())
		e := &models.Error{Code: http.StatusBadRequest, Message: swag.String(msg)}
		return watchers.NewWatcherCreateDefault(int(e.Code)).WithPayload(e)
	}
	return watchers.NewWatcherCreateCreated().WithPayload(models.ObjID(id))
}

// watcherFetch is the REST API handler
func (m *Manager) watcherFetch(params watchers.WatcherFetchParams) middleware.Responder {
	subject, e := m.accessManager.GetAuth(params.HTTPRequest)
	if e != nil {
		return watchers.NewWatcherFetchDefault(int(e.Code)).WithPayload(e)
	}
	w := m.activateWatcher(params.ID)
	var wsw *wsWatcher
	var ok bool
	if w != nil {
		wsw, ok = w.client.(*wsWatcher)
	}
	if w == nil || !ok {
		m.Log.Debugf("Watcher %s not found", params.ID)
		e := &models.Error{Code: http.StatusNotFound, Message: swag.String(com.ErrorNotFound)}
		return watchers.NewWatcherFetchDefault(int(e.Code)).WithPayload(e)
	}
	w.auth = subject
	wsw.w = w
	return middleware.ResponderFunc(func(rw http.ResponseWriter, _ runtime.Producer) {
		conn, err := m.upgrader.Upgrade(rw, params.HTTPRequest, nil)
		if err != nil {
			m.Log.Errorf("Upgrade: %s", err.Error())
			return
		}
		wsw.conn = conn
		if w.m.WSKeepAlive {
			wsw.ticker = time.NewTicker(wsw.pingPeriod)
		}
		defer func() {
			if wsw.ticker != nil {
				wsw.ticker.Stop()
			}
			conn.Close()
		}()
		go util.PanicLogger(m.Log, wsw.recv)
		if wsw.ticker != nil {
			go util.PanicLogger(m.Log, func() {
				for range wsw.ticker.C {
					wsw.pingTimer()
				}
			})
		}
		w.deliverEvents()
	})
}

// CrudeNotify is the websocket sender
func (wsw *wsWatcher) CrudeNotify(ct WatcherCallbackType, ce *CrudEvent) error {
	wsw.mux.Lock() // sync with ping timer
	defer wsw.mux.Unlock()
	wsw.conn.SetWriteDeadline(time.Now().Add(WSWriteDeadline))
	if ct == WatcherQuitting {
		wsw.conn.WriteMessage(websocket.CloseMessage, []byte{})
		return nil
	}
	if err := wsw.conn.WriteJSON(ce.ToModel()); err != nil {
		wsw.w.m.Log.Errorf("%s WriteJSON: %s", wsw.w.id, err.Error())
		return err
	}
	return nil
}

// pingTimer periodically issues Ping messages to keep the connection alive because
// Nginx will close the socket after 60 seconds of idle time.
// It must serialize with the Write callback.
func (wsw *wsWatcher) pingTimer() {
	w := wsw.w
	if !w.terminate {
		wsw.mux.Lock()
		defer wsw.mux.Unlock()
		wsw.pingCnt++
		wsw.conn.SetWriteDeadline(time.Now().Add(WSWriteDeadline))
		if err := wsw.conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
			w.mustTerminate()
		}
	}
}

// recv is the websocket receiver that detects abnormal termination. It should be run as a goroutine.
func (wsw *wsWatcher) recv() {
	w := wsw.w
	wsw.conn.SetCloseHandler(wsw.closeHandler)
	wsw.conn.SetReadDeadline(time.Time{}) // infinite
	for {
		_, _, err := wsw.conn.ReadMessage()
		if err != nil {
			w.m.Log.Debugf("%s Terminated:%v ReadError: %s", w.id, w.terminate, err.Error())
			break
		}
	}
	w.mustTerminate() // racy during shutdown
}

// closeHandler is the websocket close handler
func (wsw *wsWatcher) closeHandler(code int, text string) error {
	w := wsw.w
	w.m.Log.Debugf("%s Close: %d", w.id, code)
	w.mustTerminate()
	return nil
}
