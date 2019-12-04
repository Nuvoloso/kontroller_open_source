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
	"net/http"
)

// StatusExtractorResponseWriter is a wrapped response writer to intercept the status code
type StatusExtractorResponseWriter interface {
	http.ResponseWriter
	StatusCode() int
	StatusSuccess() bool
}

// sRW implements the StatusExtractorResponseWriter interface
type sRW struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader method is intercepted to get the error code
func (sW *sRW) WriteHeader(code int) {
	sW.statusCode = code
	sW.ResponseWriter.WriteHeader(code)
}

// StatusCode returns the status code
func (sW *sRW) StatusCode() int {
	return sW.statusCode
}

// StatusIsSuccessful determines if a status code indicates success
func StatusIsSuccessful(statusCode int) bool {
	return statusCode >= 200 && statusCode < 300
}

// StatusSuccess returns true if the status code is in the range [200,300)
func (sW *sRW) StatusSuccess() bool {
	return StatusIsSuccessful(sW.statusCode)
}

// Support for all HTTP/1.2 interfaces
type sRwCnFlHi struct {
	http.CloseNotifier
	http.Flusher
	http.Hijacker
	sRW
}

// Hijacker only if present
type sRwHi struct {
	http.Hijacker
	sRW
}

// NewStatusExtractorResponseWriter returns a new StatusExtractorResponseWriter
// Note: There are 4 optional interfaces for ResponseWriter that are tested for by a cast. i.e. 16 combinatorics in all!
// We support:
//  - the 3 HTTP/1.2 interfaces if all present (this is observed to be the case at runtime)
//  - Hijacker if present (because WebSocket needs it)
func NewStatusExtractorResponseWriter(w http.ResponseWriter) StatusExtractorResponseWriter {
	sW := &sRW{w, http.StatusOK}
	cn, cnOk := w.(http.CloseNotifier)
	fl, flOk := w.(http.Flusher)
	hi, hiOk := w.(http.Hijacker)
	if cnOk && flOk && hiOk {
		return &sRwCnFlHi{cn, fl, hi, *sW}
	}
	if hiOk {
		return &sRwHi{hi, *sW}
	}
	return sW
}
