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
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// UnixScheme is the URL scheme to use Unix domain sockets
const UnixScheme = "unix"

// UnixClientOpts are options for a Unix domain socket client
type UnixClientOpts struct {
	DialTimeout     time.Duration
	RequestTimeout  time.Duration
	ResponseTimeout time.Duration
}

// UnixTransport is a RoundTripper implemented using a Unix domain socket
type UnixTransport struct {
	SocketPath string
	UnixClientOpts
}

// UnixBody wraps the Body returned by ReadResponse to allow the connection to be closed when Body.Close() is called
type UnixBody struct {
	body     io.ReadCloser
	conn     net.Conn
	isClosed bool
}

var _ http.RoundTripper = (*UnixTransport)(nil)

// NewUnixClient creates an http.Client using a Unix domain socket
// It handles URLs of the form unix:///api/v1/ and optionally allows a hostname of "localhost". All error checking is in RoundTrip.
func NewUnixClient(socketPath string, opts *UnixClientOpts) *http.Client {
	unixTransport := &UnixTransport{SocketPath: socketPath}
	if opts != nil {
		unixTransport.UnixClientOpts = *opts
	}
	t := &http.Transport{}
	t.RegisterProtocol(UnixScheme, unixTransport)
	return &http.Client{Transport: t}
}

// RoundTrip implements the RoundTripper interface
// It currently opens a new connection for each request
func (t *UnixTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	closeBody := func() {
		if req.Body != nil {
			req.Body.Close()
		}
	}
	if req.URL == nil {
		closeBody()
		return nil, fmt.Errorf("%s: nil Request.URL", UnixScheme)
	}
	if req.URL.Scheme != UnixScheme {
		closeBody()
		return nil, fmt.Errorf("unsupported protocol scheme: %s", req.URL.Scheme)
	}
	if req.URL.Host != "" && req.URL.Host != "localhost" {
		closeBody()
		return nil, fmt.Errorf("%s: unexpected host in Request.URL: %s", UnixScheme, req.URL.Host)
	}
	conn, err := net.DialTimeout(UnixScheme, t.SocketPath, t.DialTimeout)
	if err == nil {
		if t.RequestTimeout > 0 {
			conn.SetWriteDeadline(time.Now().Add(t.RequestTimeout))
		}
		if err = req.Write(conn); err == nil {
			if t.ResponseTimeout > 0 {
				conn.SetReadDeadline(time.Now().Add(t.ResponseTimeout))
			}
			var resp *http.Response
			resp, err = http.ReadResponse(bufio.NewReader(conn), req)
			if err == nil {
				resp.Body = &UnixBody{body: resp.Body, conn: conn}
				return resp, err
			}
		}
		conn.Close()
	}
	closeBody()
	return nil, err
}

var _ = io.ReadCloser(&UnixBody{})

// Read implements io.Reader
func (b *UnixBody) Read(p []byte) (n int, err error) {
	return b.body.Read(p)
}

// Close implements io.Closer
func (b *UnixBody) Close() error {
	var err error
	if !b.isClosed {
		err = b.body.Close()
		err2 := b.conn.Close()
		if err == nil {
			err = err2
		}
		b.isClosed = true
	}
	return err
}
