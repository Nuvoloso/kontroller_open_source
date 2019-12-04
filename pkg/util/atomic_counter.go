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
	"context"
	"net/http"
	"sync/atomic"
)

// ReqRespCounter is a counter to keep track of request and responses
var reqRespCounter uint64

type contextKey string

var contextKeyCount = contextKey("ReqRespCount")

// NumberRequestMiddleware increment the number and sets it in the context
func NumberRequestMiddleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w,
			r.WithContext(context.WithValue(r.Context(),
				contextKeyCount,
				atomic.AddUint64(&reqRespCounter, 1))))
	}
	return http.HandlerFunc(fn)
}

// RequestNumberFromContext gets the number from the context, if it can't find it returns 0.
func RequestNumberFromContext(ctx context.Context) uint64 {
	if v, ok := ctx.Value(contextKeyCount).(uint64); ok {
		return v
	}
	return 0

}
