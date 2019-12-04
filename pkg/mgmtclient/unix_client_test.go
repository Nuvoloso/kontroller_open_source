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


package mgmtclient

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUnixClientTransport(t *testing.T) {
	assert := assert.New(t)

	// create new client, no options, put socket in /tmp in case CWD does not support sockets (eg VMWare hg fs)
	socket := path.Join(os.TempDir(), fmt.Sprintf("socket%d.sock", os.Getpid()))
	cl := NewUnixClient(socket, nil)
	assert.NotNil(cl)
	assert.NotNil(cl.Transport)

	// with options (timeouts are very short for testing only)
	opts := &UnixClientOpts{
		DialTimeout:     10 * time.Millisecond,
		RequestTimeout:  1 * time.Second,
		ResponseTimeout: 1 * time.Second,
	}
	cl = NewUnixClient(socket, opts)
	assert.NotNil(cl)
	assert.NotNil(cl.Transport)

	// various error cases
	u := UnixTransport{}
	assert.Panics(func() { u.RoundTrip(nil) })

	req := &http.Request{}
	resp, err := u.RoundTrip(req)
	assert.Nil(resp)
	if assert.Error(err) {
		assert.Regexp("^"+UnixScheme+".* nil.*URL", err)
	}

	req.URL = &url.URL{}
	resp, err = u.RoundTrip(req)
	assert.Nil(resp)
	if assert.Error(err) {
		assert.Regexp("scheme", err)
	}

	req.URL.Scheme = UnixScheme
	req.URL.Host = "not expected"
	resp, err = u.RoundTrip(req)
	assert.Nil(resp)
	if assert.Error(err) {
		assert.Regexp("unexpected host", err)
	}

	// Dial error: empty SocketPath
	req.URL.Host = ""
	resp, err = u.RoundTrip(req)
	assert.Nil(resp)
	if assert.Error(err) {
		_, ok := err.(*net.OpError)
		assert.True(ok)
	}

	os.Remove(socket) // just in case
	l, err := net.Listen(UnixScheme, socket)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	// timeout, pass a body to cover case that body gets closed, localhost is accepted
	u.SocketPath = socket
	u.RequestTimeout = 1 * time.Nanosecond
	req.URL.Host = "localhost"
	req.Body = ioutil.NopCloser(strings.NewReader("body\n"))
	resp, err = u.RoundTrip(req)
	assert.Nil(resp)
	if assert.Error(err) {
		_, ok := err.(*net.OpError)
		assert.True(ok)
		assert.Regexp("timeout", err)
	}

	req.Body = nil
	s := &http.Server{}
	done := false

	go func() {
		err := s.Serve(l)
		t.Log(err)
		done = true
	}()

	// read timeout
	u.RequestTimeout = 10 * time.Second
	u.ResponseTimeout = 1 * time.Nanosecond
	resp, err = u.RoundTrip(req)
	assert.Nil(resp)
	if assert.Error(err) {
		_, ok := err.(*net.OpError)
		assert.True(ok)
		assert.Regexp("read .* timeout", err)
	}

	// successful round trip with no timeouts
	u.DialTimeout = 0
	u.RequestTimeout = 0
	u.ResponseTimeout = 0
	resp, err = u.RoundTrip(req)
	assert.NotNil(resp)
	assert.NoError(err)
	assert.NotNil(resp.Body)
	buf := make([]byte, 4096, 4096)
	n, err := resp.Body.Read(buf)
	assert.NotZero(n)
	assert.Equal(io.EOF, err)
	resp.Body.Close()

	// more integrated success (no mux configured, so expect 404)
	opts = &u.UnixClientOpts
	cl = NewUnixClient(socket, opts)
	assert.NotNil(cl)
	resp, err = cl.Get("unix:///api/v1/nodes")
	if assert.NotNil(resp) {
		assert.Equal(404, resp.StatusCode)
		resp.Body.Close()
	}
	assert.NoError(err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = s.Shutdown(ctx)
	assert.NoError(err)
	cancel()

	// wait for the go-routine to terminate
	for !done {
		time.Sleep(10 * time.Millisecond)
	}

	// Dial error, no such file after shutdown
	u.SocketPath = socket
	resp, err = u.RoundTrip(req)
	assert.Nil(resp)
	if assert.Error(err) {
		_, ok := err.(*net.OpError)
		assert.True(ok)
		assert.Regexp("no such file", err)
	}
}
