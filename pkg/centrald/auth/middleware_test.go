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
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/user"
	"github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestMiddleware(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	ctx := &Config{
		Host: "author",
		Port: 5555,
		Log:  tl.Logger(),
	}

	ft := &fakeTransport{
		t:   t,
		log: tl.Logger(),
	}
	authTransport = ft
	defer func() {
		authTransport = clientTransport
	}()

	fh := &fakeHandler{}

	uObj := &models.User{}
	uObj.AuthIdentifier = "admin"
	uObj.Meta = &models.ObjMeta{ID: "uid-1"}
	uList := []*models.User{uObj}
	aObj := &models.Account{}
	aObj.Meta = &models.ObjMeta{ID: "account-1"}
	aObj.Name = "System"
	aObj.UserRoles = make(map[string]models.AuthRole)
	aObj.UserRoles["uid-1"] = models.AuthRole{RoleID: "role-1"}
	roleObj := &models.Role{}
	roleObj.Meta = &models.ObjMeta{ID: "role-1"}
	roleObj.Name = "super"

	t.Log("Middleware returns a function")
	h := ctx.Middleware(fh)
	assert.NotNil(h)
	tl.Flush()

	t.Log("NewInfo error")
	req := &http.Request{}
	req.RemoteAddr = "1.2.3.4"
	writer := httptest.NewRecorder()
	writer.Body = new(bytes.Buffer)
	h.ServeHTTP(writer, req)
	assert.Nil(fh.w)
	assert.Nil(fh.r)
	assert.Equal(403, writer.Code)
	assert.Len(writer.HeaderMap, 1)
	assert.Equal("application/json", writer.Header().Get("Content-Type"))
	bs := writer.Body.String()
	assert.Equal(`{"code":403,"message":"missing auth token"}`, bs)
	assert.Equal(1, tl.CountPattern("called by .*1.2.3.4.* ⇒ 403"))

	t.Log("success, X-Auth updated")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	writer = httptest.NewRecorder()
	writer.Body = new(bytes.Buffer)
	resp := &http.Response{
		Header:     make(http.Header),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Status:     "200 OK",
		StatusCode: http.StatusOK,
	}
	resp.StatusCode = 200
	resp.Header["Content-Type"] = []string{"application/json"}
	resp.Body = ioutil.NopCloser(strings.NewReader(fakeValidateResp))
	ft.resp = resp
	req = &http.Request{}
	req.RemoteAddr = "1.2.3.4"
	req.Header = make(http.Header)
	req.Header.Set("X-Auth", "titus")
	mds := mock.NewMockDataStore(mockCtrl)
	oU := mock.NewMockUserOps(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU)
	oU.EXPECT().List(req.Context(), user.UserListParams{AuthIdentifier: swag.String("me")}).Return(uList, nil)
	ctx.DS = mds
	h.ServeHTTP(writer, req)
	assert.True(writer == fh.w)
	if assert.NotNil(fh.r) {
		// request should be like req but with context added
		r2 := req.WithContext(fh.r.Context())
		assert.Equal(r2, fh.r)
		ai := &Info{}
		if cnv := fh.r.Context().Value(InfoKey{}); cnv != nil {
			ai = cnv.(*Info)
		}
		assert.NotNil(ai, "request context contains auth.Info")
		assert.Equal("new-token", ai.Token)
	}
	assert.Equal(200, writer.Code)
	assert.Len(writer.HeaderMap, 1)
	assert.Equal("new-token", writer.Header().Get("X-Auth"))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("success without X-Auth")
	mockCtrl = gomock.NewController(t)
	writer = httptest.NewRecorder()
	writer.Body = new(bytes.Buffer)
	ft.req = nil
	ft.resp = nil
	req = &http.Request{}
	req.RemoteAddr = "@"
	req.Header = make(http.Header)
	ctx.DS = nil
	h.ServeHTTP(writer, req)
	assert.True(writer == fh.w)
	if assert.NotNil(fh.r) {
		// request should be like req but with context added
		r2 := req.WithContext(fh.r.Context())
		assert.Equal(r2, fh.r)
		ai := &Info{}
		if cnv := fh.r.Context().Value(InfoKey{}); cnv != nil {
			ai = cnv.(*Info)
		}
		assert.NotNil(ai, "request context contains auth.Info")
		assert.Empty(ai.Token)
		assert.Empty(ai.RemoteAddr)
	}
	assert.Equal(200, writer.Code)
	assert.Empty(writer.Header().Get("X-Auth"))
}

type fakeHandler struct {
	w http.ResponseWriter
	r *http.Request
}

func (fh *fakeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fh.w = w
	fh.r = r
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("{}"))
}
