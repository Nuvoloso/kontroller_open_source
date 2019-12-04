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
	"context"
	"net/http"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/go-openapi/swag"
)

// Middleware inspects headers for authentication data, validates it, adds AuthInfo to the context and
// sets the X-Auth header with the updated token in the response.
// A request with invalid authentication data is immediately rejected and is not passed on to the next handler
func (ctx *Config) Middleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ai, err := ctx.NewInfo(r)
		if err != nil {
			// all APIs return an Error on failure, but since this is middleware, must marshal and write it manually
			e := &models.Error{Code: http.StatusForbidden, Message: swag.String(err.Error())}
			ctx.Log.Errorf("[%s] %q called by %s ⇒ %d (%s)", r.Method, r.URL, ai, e.Code, *e.Message)
			buf, _ := e.MarshalBinary()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(int(e.Code))
			w.Write(buf)
		} else {
			ctx := context.WithValue(r.Context(), InfoKey{}, ai)
			if ai.Token != "" {
				w.Header().Set("X-Auth", ai.Token)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		}
	}
	return http.HandlerFunc(fn)
}
