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

// gorilla WebSocket abstractions
// Factory "objects" need wrapping to obtain an abstract Conn
//  - WrapUpgrade wraps a websocket.Upgrader
//  - WrapDialer wraps a websocket.Dialer

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// Upgrader is the abstraction of the Upgrader methods
type Upgrader interface {
	Upgrade(w http.ResponseWriter, r *http.Request, responseHeader http.Header) (Conn, error)
	Object() *websocket.Upgrader
}

type upgrader struct {
	u *websocket.Upgrader
}

func (wsu *upgrader) Upgrade(w http.ResponseWriter, r *http.Request, responseHeader http.Header) (Conn, error) {
	return wsu.u.Upgrade(w, r, responseHeader)
}
func (wsu *upgrader) Object() *websocket.Upgrader {
	return wsu.u
}

// WrapUpgrader adapts the real websocket data type to expose the abstract method
func WrapUpgrader(u *websocket.Upgrader) Upgrader {
	return &upgrader{u}
}

// Dialer is the abstraction of the Dialer methods
type Dialer interface {
	Dial(urlStr string, requestHeader http.Header) (Conn, *http.Response, error)
	DialContext(ctx context.Context, urlStr string, requestHeader http.Header) (Conn, *http.Response, error)
	Object() *websocket.Dialer
}

type dialer struct {
	d *websocket.Dialer
}

func (wsd *dialer) Dial(urlStr string, requestHeader http.Header) (Conn, *http.Response, error) {
	return wsd.d.Dial(urlStr, requestHeader)
}
func (wsd *dialer) DialContext(ctx context.Context, urlStr string, requestHeader http.Header) (Conn, *http.Response, error) {
	return wsd.d.DialContext(ctx, urlStr, requestHeader)
}
func (wsd *dialer) Object() *websocket.Dialer {
	return wsd.d
}

// WrapDialer adapts the real websocket data type to expose the abstract method
func WrapDialer(d *websocket.Dialer) Dialer {
	return &dialer{d}
}

// Conn is the abstraction of the Conn methods of interest
type Conn interface {
	Close() error
	ReadJSON(v interface{}) error
	ReadMessage() (messageType int, p []byte, err error)
	SetCloseHandler(h func(code int, text string) error)
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	WriteJSON(v interface{}) error
	WriteMessage(messageType int, data []byte) error
}
