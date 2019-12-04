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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAtomicCounter(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()

	// count not set in context
	retCount := RequestNumberFromContext(ctx)
	assert.Equal(uint64(0), retCount)

	// context key set
	ctx = context.WithValue(ctx, contextKeyCount, uint64(5))
	retCount = RequestNumberFromContext(ctx)
	assert.EqualValues(5, retCount)

	// test middleware
	var count uint64
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		val := r.Context().Value(contextKeyCount)
		if val == nil {
			t.Error("count not present") // should not be encountered
		}
		valCount, ok := val.(uint64)
		if !ok {
			t.Error("not string") // should not be encountered
		}
		count = valCount
	})

	assert.EqualValues(0, count)
	h1 := NumberRequestMiddleware(nextHandler)
	r, _ := http.NewRequest("GET", "https://example.com/foo/", nil)
	rWCtx := r.WithContext(context.Background())
	h1.ServeHTTP(nil, rWCtx)
	assert.EqualValues(1, count)
}

// GetTestHandler returns a http.HandlerFunc for testing http middleware
func GetTestHandler(t *testing.T) http.HandlerFunc {
	fn := func(w http.ResponseWriter, r *http.Request) {
		val := r.Context().Value(contextKeyCount)
		if val == nil {
			t.Error("count not present")
		}
		_, ok := val.(uint64)
		if !ok {
			t.Error("not string")
		}
	}
	return http.HandlerFunc(fn)
}
