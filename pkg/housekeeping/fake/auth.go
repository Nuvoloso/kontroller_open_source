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
	"github.com/Nuvoloso/kontroller/pkg/housekeeping"
)

// AccessControl is a fake housekeeping access control manager
type AccessControl struct {
	// GetAuth
	InGaReq   *http.Request
	RetGaAuth auth.Subject
	RetGaErr  *models.Error

	// TaskCancelOK
	InTcnS  auth.Subject
	InTcnOp string
	RetTcn  bool

	// TaskCreateOK
	InTcS  auth.Subject
	InTcOp string
	RetTc  bool

	// TaskViewOK
	InTvS  auth.Subject
	InTvOp string
	RetTv  bool
}

var _ = housekeeping.AccessControl(&AccessControl{})

// GetAuth fakes its namesake
func (fa *AccessControl) GetAuth(req *http.Request) (auth.Subject, *models.Error) {
	fa.InGaReq = req
	return fa.RetGaAuth, fa.RetGaErr
}

// TaskCancelOK fakes its namesake
func (fa *AccessControl) TaskCancelOK(subject auth.Subject, op string) bool {
	fa.InTcnS = subject
	fa.InTcnOp = op
	return fa.RetTcn
}

// TaskCreateOK fakes its namesake
func (fa *AccessControl) TaskCreateOK(subject auth.Subject, op string) bool {
	fa.InTcS = subject
	fa.InTcOp = op
	return fa.RetTc
}

// TaskViewOK fakes its namesake
func (fa *AccessControl) TaskViewOK(subject auth.Subject, op string) bool {
	fa.InTvS = subject
	fa.InTvOp = op
	return fa.RetTv
}
