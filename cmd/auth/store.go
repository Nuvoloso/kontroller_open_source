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
	"sync"

	"github.com/Nuvoloso/kontroller/pkg/mongodb"
	"github.com/gorilla/sessions"
)

// TBD we could store a secret in mongo. For now we just derive our secret from this value and the system object ID
const secretPrefix = "nuvoloso-secret"

type cookieStore struct {
	secret []byte
	store  *sessions.CookieStore
	db     mongodb.DBAPI
	mux    sync.Mutex
}

// newCookieStore creates a new cookieStore
func newCookieStore(db mongodb.DBAPI) *cookieStore {
	return &cookieStore{db: db}
}

// Secret returns the secret used to sign tokens. An empty slice is returned if the store is not ready
func (s *cookieStore) Secret() []byte {
	return s.secret
}

// Store returns the CookieStore or an error if the store is not ready
func (s *cookieStore) Store(ctx context.Context) (*sessions.CookieStore, error) {
	if s.store == nil {
		s.mux.Lock()
		defer s.mux.Unlock()
		if s.store == nil {
			id, err := getSystemID(ctx, s.db)
			if err != nil {
				return nil, err
			}
			s.secret = append([]byte(secretPrefix), id...)
			s.store = sessions.NewCookieStore(s.secret)
		}
	}

	return s.store, nil
}
