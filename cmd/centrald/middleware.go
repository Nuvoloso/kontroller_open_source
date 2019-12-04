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
	"net/http"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/util"
)

// loggingHandler middleware example
func (ctx *mainContext) loggingHandler(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ai := &auth.Info{}
		if cnv := r.Context().Value(auth.InfoKey{}); cnv != nil {
			ai = cnv.(*auth.Info)
		}
		ctx.Log.Infof("R%d [%s] %q called by %s", util.RequestNumberFromContext(r.Context()), r.Method, r.URL.String(), ai)
		tStart := time.Now()
		sW := util.NewStatusExtractorResponseWriter(w)
		next.ServeHTTP(sW, r)
		tEnd := time.Now()
		ctx.Log.Infof("R%d [%s] %q ⇒ %d (%v)\n", util.RequestNumberFromContext(r.Context()), r.Method, r.URL.String(), sW.StatusCode(), tEnd.Sub(tStart))
	}
	return http.HandlerFunc(fn)
}

// outermostMiddleware guard for tracing flow
func (ctx *mainContext) outermostMiddleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		//ctx.Log.Debug("*** outermostMiddleware in")
		next.ServeHTTP(w, r)
		//ctx.Log.Debug("**** outermostMiddleware out")
	}
	return http.HandlerFunc(fn)
}

// innermostMiddleware guard for tracing flow
func (ctx *mainContext) innermostMiddleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		//ctx.Log.Debug("*** innermostMiddleware in")
		next.ServeHTTP(w, r)
		//ctx.Log.Debug("*** innermostMiddleware out")
	}
	return http.HandlerFunc(fn)
}
