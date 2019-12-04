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


package fake

import (
	"net/http"

	"github.com/Nuvoloso/kontroller/pkg/auth"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
)

// Extractor implements the AccessControl interface
type Extractor struct {
	RetSubj auth.Subject
	RetErr  *models.Error
}

var _ = auth.AccessControl(&Extractor{})

// GetAuth is a fake AccessControl extractor
func (c *Extractor) GetAuth(req *http.Request) (auth.Subject, *models.Error) {
	return c.RetSubj, c.RetErr
}

// Subject implements the Subject interface
type Subject struct {
	RetI   bool
	RetGAI string
	RetS   string
}

var _ = auth.Subject(&Subject{})

// Internal fakes its namesake
func (s *Subject) Internal() bool {
	return s.RetI
}

// GetAccountID fakes its namesake
func (s *Subject) GetAccountID() string {
	return s.RetGAI
}

// String fakes its namesake
func (s *Subject) String() string {
	return s.RetS
}
