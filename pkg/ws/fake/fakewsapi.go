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


package ws

import (
	"context"
	"net/http"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/ws"
	"github.com/gorilla/websocket"
)

// fake gorilla WebSocket abstractions

// FakeUpgrader fakes the ws.Upgrader interface
type FakeUpgrader struct {
	CalledU  bool
	RetUConn ws.Conn
	RetUErr  error
}

// Upgrade fakes its namesake
func (fu *FakeUpgrader) Upgrade(w http.ResponseWriter, r *http.Request, responseHeader http.Header) (ws.Conn, error) {
	fu.CalledU = true
	return fu.RetUConn, fu.RetUErr
}

// Object fakes its namesake. Only for UTs.
func (fu *FakeUpgrader) Object() *websocket.Upgrader {
	return nil
}

// FakeDialer fakes the ws.Dialer interface
type FakeDialer struct {
	InDUrl   string
	RetDConn ws.Conn
	RetDResp *http.Response
	RetDErr  error

	InDCctx   context.Context
	InDCUrl   string
	RetDCConn ws.Conn
	RetDCResp *http.Response
	RetDCErr  error
}

// Dial fakes its namesake
func (fd *FakeDialer) Dial(urlStr string, requestHeader http.Header) (ws.Conn, *http.Response, error) {
	fd.InDUrl = urlStr
	return fd.RetDConn, fd.RetDResp, fd.RetDErr
}

// DialContext fakes its namesake
func (fd *FakeDialer) DialContext(ctx context.Context, urlStr string, requestHeader http.Header) (ws.Conn, *http.Response, error) {
	fd.InDCctx = ctx
	fd.InDCUrl = urlStr
	return fd.RetDCConn, fd.RetDCResp, fd.RetDCErr
}

// Object fakes its namesake. Only for UTs.
func (fd *FakeDialer) Object() *websocket.Dialer {
	return nil
}

// FakeConn fakes the ws.Conn interface
type FakeConn struct {
	// Close
	CalledC bool

	// ReadJSON
	ChanRJ     chan struct{}
	ChanRJMade bool
	BlockRJ    bool
	InRJv      interface{}
	RetRJErr   error
	SetRJv     func(interface{})

	// ReadMessage
	ChanRM     chan struct{}
	ChanRMMade bool
	RetRMErr   error

	// SetCloseHandler
	InSCHh func(code int, text string) error

	// SetReadDeadline
	InSRDt time.Time

	// SetWriteDeadline
	InSWDt time.Time

	// WriteJSON
	InWJv    interface{}
	RetWJErr error

	// WriteMessage
	InWMmt   int
	RetWMErr error
}

// Close fakes its namesake
func (fc *FakeConn) Close() error {
	fc.CalledC = true
	return nil
}

// ReadJSON fakes its namesake
func (fc *FakeConn) ReadJSON(v interface{}) error {
	fc.InRJv = v
	if !fc.ChanRJMade {
		fc.ChanRJ = make(chan struct{})
		fc.ChanRJMade = true
	}
	if fc.BlockRJ {
		select {
		case <-fc.ChanRJ: // block until closed
			fc.ChanRJMade = false
			break
		}
	}
	if fc.SetRJv != nil {
		fc.SetRJv(v)
	}
	return fc.RetRJErr
}

// ReadMessage fakes its namesake
func (fc *FakeConn) ReadMessage() (messageType int, p []byte, err error) {
	if !fc.ChanRMMade {
		fc.ChanRM = make(chan struct{})
		fc.ChanRMMade = true
	}
	select {
	case <-fc.ChanRM: // block until closed
		fc.ChanRMMade = false
		break
	}
	return 0, nil, fc.RetRMErr
}

// SetCloseHandler fakes its namesake
func (fc *FakeConn) SetCloseHandler(h func(code int, text string) error) {
	fc.InSCHh = h
}

// SetReadDeadline fakes its namesake
func (fc *FakeConn) SetReadDeadline(t time.Time) error {
	fc.InSRDt = t
	return nil
}

// SetWriteDeadline fakes its namesake
func (fc *FakeConn) SetWriteDeadline(t time.Time) error {
	fc.InSWDt = t
	return nil
}

// WriteJSON fakes its namesake
func (fc *FakeConn) WriteJSON(v interface{}) error {
	fc.InWJv = v
	return fc.RetWJErr
}

// WriteMessage fakes its namesake
func (fc *FakeConn) WriteMessage(messageType int, data []byte) error {
	fc.InWMmt = messageType
	return fc.RetWMErr
}
