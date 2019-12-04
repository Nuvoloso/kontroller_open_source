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


package util

import (
	"bufio"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusRW(t *testing.T) {
	assert := assert.New(t)

	frw := &FakeResponseWriter{}
	w := NewStatusExtractorResponseWriter(frw)
	assert.Equal(http.StatusOK, w.StatusCode())
	assert.True(w.StatusSuccess())

	w.WriteHeader(http.StatusNoContent)
	assert.True(frw.WHCalled)
	assert.Equal(http.StatusNoContent, w.StatusCode())
	assert.True(w.StatusSuccess())

	w.WriteHeader(http.StatusTeapot)
	assert.Equal(http.StatusTeapot, w.StatusCode())
	assert.False(w.StatusSuccess())

	// check no optional interfaces present
	var rw http.ResponseWriter = w
	_, ok := rw.(http.Hijacker)
	assert.False(ok, "Hijacker")
	_, ok = rw.(http.CloseNotifier)
	assert.False(ok, "CloseNotifier")
	_, ok = rw.(http.Flusher)
	assert.False(ok, "Flusher")
	_, ok = rw.(http.Pusher)
	assert.False(ok, "Pusher")

	// check all HTTP/1.2 interfaces present
	fCFHP := &FakeRwCnFlHiPu{}
	w = NewStatusExtractorResponseWriter(fCFHP)
	assert.Equal(http.StatusOK, w.StatusCode())
	assert.True(w.StatusSuccess())
	rw = w
	_, ok = rw.(http.Hijacker)
	assert.True(ok, "Hijacker")
	_, ok = rw.(http.CloseNotifier)
	assert.True(ok, "CloseNotifier")
	_, ok = rw.(http.Flusher)
	assert.True(ok, "Flusher")
	_, ok = rw.(http.Pusher)
	assert.False(ok, "Pusher") // not passed on

	// check that Hijacker (only) passed on
	fH := &FakeRwHi{}
	w = NewStatusExtractorResponseWriter(fH)
	assert.Equal(http.StatusOK, w.StatusCode())
	assert.True(w.StatusSuccess())
	rw = w
	_, ok = rw.(http.Hijacker)
	assert.True(ok, "Hijacker")
	_, ok = rw.(http.CloseNotifier)
	assert.False(ok, "CloseNotifier")
	_, ok = rw.(http.Flusher)
	assert.False(ok, "Flusher")
	_, ok = rw.(http.Pusher)
	assert.False(ok, "Pusher")
}

type FakeResponseWriter struct {
	WHCalled bool
}

func (frw *FakeResponseWriter) Header() http.Header {
	return nil
}
func (frw *FakeResponseWriter) Write([]byte) (int, error) {
	return 0, nil
}
func (frw *FakeResponseWriter) WriteHeader(int) {
	frw.WHCalled = true
}

type FakeRwHi struct {
	FakeResponseWriter
}

func (frwH *FakeRwHi) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, nil
}

type FakeRwCnFlHiPu struct {
	FakeResponseWriter
}

func (x *FakeRwCnFlHiPu) CloseNotify() <-chan bool {
	return nil
}
func (x *FakeRwCnFlHiPu) Flush() {}
func (x *FakeRwCnFlHiPu) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, nil
}
func (x *FakeRwCnFlHiPu) Push(target string, opts *http.PushOptions) error {
	return nil
}
