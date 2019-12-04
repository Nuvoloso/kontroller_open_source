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


package auth

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetAuth(t *testing.T) {
	assert := assert.New(t)

	ex := &Extractor{}
	assert.Panics(func() { ex.GetAuth(nil) })

	s, me := ex.GetAuth(&http.Request{})
	assert.Nil(me)
	ai, ok := s.(*Info)
	if assert.True(ok) {
		assert.Empty(ai.RemoteAddr)
		assert.Empty(ai.CertCN)
		assert.True(s.Internal())
		assert.Empty(s.GetAccountID())
		assert.Equal("[internal client] from unix socket", s.String())
	}

	s, me = ex.GetAuth(&http.Request{RemoteAddr: "@"})
	assert.Nil(me)
	ai, ok = s.(*Info)
	if assert.True(ok) {
		assert.Empty(ai.RemoteAddr)
		assert.Empty(ai.CertCN)
		assert.True(s.Internal())
		assert.Empty(s.GetAccountID())
		assert.Equal("[internal client] from unix socket", s.String())
	}

	s, me = ex.GetAuth(&http.Request{RemoteAddr: "1.2.3.4"})
	assert.Nil(me)
	ai, ok = s.(*Info)
	if assert.True(ok) {
		assert.Equal("1.2.3.4", ai.RemoteAddr)
		assert.Empty(ai.CertCN)
		assert.False(s.Internal())
		assert.Empty(s.GetAccountID())
		assert.Equal("noCert! from 1.2.3.4", s.String())
	}

	req := &http.Request{RemoteAddr: "1.2.3.4"}
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{
			&x509.Certificate{
				Subject: pkix.Name{CommonName: "common"},
			},
		},
	}
	s, me = ex.GetAuth(req)
	assert.Nil(me)
	ai, ok = s.(*Info)
	if assert.True(ok) {
		assert.Equal("1.2.3.4", ai.RemoteAddr)
		assert.Equal("common", ai.CertCN)
		assert.True(s.Internal())
		assert.Empty(s.GetAccountID())
		assert.Equal("CN[common] from 1.2.3.4", s.String())
	}
}
