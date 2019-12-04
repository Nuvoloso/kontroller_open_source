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
	"context"
	"net/http"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/util"
)

// Use a struct as key to guarantee uniqueness
type clientCertCommonName struct{}

// loggingHandler middleware example
func (ctx *mainContext) loggingHandler(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		cnDn := "?"
		if cnv := r.Context().Value(clientCertCommonName{}); cnv != nil {
			cnDn = cnv.(string)
		} else if r.RemoteAddr == "@" || r.RemoteAddr == "" {
			// The HTTP server renders the unix socket as "@" on linux, empty string on MacOS, always host:port for http and https
			cnDn = "unix socket client"
		}
		// headers added by nginx when it verifies a client cert on our behalf
		if dn, ok := r.Header["X-Ssl-Client-Dn"]; ok && len(dn) == 1 {
			cnDn += " clientDN[" + dn[0] + "]"
			if verified, ok := r.Header["X-Ssl-Client-Verify"]; ok && len(verified) > 0 && verified[0] == "SUCCESS" {
				cnDn += "(V)"
			}
		} else if r.TLS != nil && len(r.TLS.VerifiedChains) > 0 {
			cnDn += "(V)"
		}
		ctx.Log.Infof("R%d [%s] %q called by %s", util.RequestNumberFromContext(r.Context()), r.Method, r.URL.String(), cnDn)
		tStart := time.Now()
		next.ServeHTTP(w, r)
		tEnd := time.Now()
		ctx.Log.Infof("R%d [%s] %q return %v\n", util.RequestNumberFromContext(r.Context()), r.Method, r.URL.String(), tEnd.Sub(tStart))
	}
	return http.HandlerFunc(fn)
}

// peepAtClientCertMiddleware ... just curious to see what we can get out of the cert
func (ctx *mainContext) peepAtClientCertMiddleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
			cert := r.TLS.PeerCertificates[0]
			// push to later handlers
			ctx := context.WithValue(r.Context(), clientCertCommonName{}, cert.Subject.CommonName)
			next.ServeHTTP(w, r.WithContext(ctx))
		} else {
			ctx.Log.Info("No client certificate found")
			next.ServeHTTP(w, r)
		}
	}
	return http.HandlerFunc(fn)
}

// outermostMiddleware guard for tracing flow
func (ctx *mainContext) outermostMiddleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx.Log.Debug("*** outermostMiddleware in")
		next.ServeHTTP(w, r)
		ctx.Log.Debug("**** outermostMiddleware out")
	}
	return http.HandlerFunc(fn)
}

// innermostMiddleware guard for tracing flow
func (ctx *mainContext) innermostMiddleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx.Log.Debug("*** innermostMiddleware in")
		next.ServeHTTP(w, r)
		ctx.Log.Debug("*** innermostMiddleware out")
	}
	return http.HandlerFunc(fn)
}
