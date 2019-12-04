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
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

func TestWrappers(t *testing.T) {
	assert := assert.New(t)

	u := &websocket.Upgrader{}
	var ui Upgrader
	assert.NotPanics(func() { ui = WrapUpgrader(u) })
	assert.Equal(u, ui.Object())
	errCalled := false
	u.Error = func(w http.ResponseWriter, r *http.Request, status int, reason error) {
		errCalled = true
	}
	r := &http.Request{Method: "NOT_GET"}
	_, err := ui.Upgrade(nil, r, nil)
	assert.Error(err)
	assert.Regexp("the client is not using the websocket protocol", err)
	assert.True(errCalled)

	d := &websocket.Dialer{}
	var di Dialer
	assert.NotPanics(func() { di = WrapDialer(d) })
	assert.Equal(d, di.Object())
	_, _, err = di.Dial("", nil)
	assert.Error(err)
	assert.Regexp("malformed.*URL", err)

	_, _, err = di.DialContext(context.Background(), "", nil)
	assert.Error(err)
	assert.Regexp("malformed.*URL", err)
}
