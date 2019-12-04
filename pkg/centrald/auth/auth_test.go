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
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/user"
	app "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

func TestInternalOK(t *testing.T) {
	assert := assert.New(t)

	ai := &Info{}
	assert.NoError(ai.InternalOK())

	ai.RoleObj = &models.Role{}
	assert.Equal(app.ErrorUnauthorizedOrForbidden, ai.InternalOK())
}

func TestCapOK(t *testing.T) {
	assert := assert.New(t)

	ai := &Info{}
	assert.NoError(ai.CapOK("anyCap"))

	ai.RoleObj = &models.Role{}
	assert.Equal(app.ErrorUnauthorizedOrForbidden, ai.CapOK("anyCap"))

	ai.RoleObj.Capabilities = map[string]bool{app.SystemManagementCap: true}
	assert.NoError(ai.CapOK(app.SystemManagementCap))

	ai.AccountID = "aid1"
	assert.NoError(ai.CapOK(app.SystemManagementCap, "aid2", "aid1"))

	assert.Equal(app.ErrorUnauthorizedOrForbidden, ai.CapOK(app.SystemManagementCap, "aid2"))
}

func TestUserOK(t *testing.T) {
	assert := assert.New(t)

	ai := &Info{}
	assert.NoError(ai.UserOK("uid1"))

	ai.UserID = "uid1"
	ai.RoleObj = &models.Role{}
	assert.NoError(ai.UserOK("uid1"))

	ai.UserID = "uid2"
	ai.RoleObj = &models.Role{}
	assert.Equal(app.ErrorUnauthorizedOrForbidden, ai.UserOK("uid1"))
}

func TestInfoString(t *testing.T) {
	assert := assert.New(t)

	// empty struct means internal user on socket
	auth := Info{}
	assert.Equal("[internal client] from unix socket", auth.String())

	// unix socket can masquerade
	auth = Info{
		CertCN:         "nuvo.com",
		AuthIdentifier: "admin",
		AccountID:      "account-1",
		AccountName:    "System",
		UserID:         "user-1",
	}
	assert.Equal("U[admin,user-1] A[System,account-1] from unix socket", auth.String())

	// real remote case, no cert info, with role
	auth.CertCN = ""
	auth.RemoteAddr = "192.168.1.1:30001"
	auth.RoleObj = &models.Role{}
	auth.RoleObj.Name = "super"
	assert.Equal("noCert! U[admin,user-1] A[System,account-1,super] from 192.168.1.1:30001", auth.String())

	// real remote, certs but no auth info
	auth = Info{
		RemoteAddr:         "192.168.1.1:30001",
		CertCN:             "nuvo.com",
		ClientDN:           "a=b,c=d",
		ClientCertVerified: true,
	}
	assert.Equal("CN[nuvo.com] clientDN[a=b,c=d](V) from 192.168.1.1:30001", auth.String())

	// !verified
	auth.ClientCertVerified = false
	assert.Equal("CN[nuvo.com] clientDN[a=b,c=d] from 192.168.1.1:30001", auth.String())
}

func TestNewInfo(t *testing.T) {
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

	t.Log("macOS-style unix domain trusted, success without any other auth info")
	exp := &Info{}
	r := &http.Request{}
	auth, err := ctx.NewInfo(r)
	assert.NoError(err)
	assert.Equal(exp, auth)
	assert.Empty(auth.GetAccountID())

	t.Log("missing X-Auth token")
	r.RemoteAddr = "1.2.3.4"
	r.Header = make(http.Header)
	auth, err = ctx.NewInfo(r)
	assert.NotNil(auth)
	assert.Error(err)

	t.Log("linux-style unix domain trusted, success without any other auth info")
	r.RemoteAddr = "@"
	r.Header = make(http.Header)
	auth, err = ctx.NewInfo(r)
	assert.NoError(err)
	assert.Equal(exp, auth)
	assert.True(auth.Internal())

	t.Log("unix domain trusted, can pass user ID, no account ID")
	r.Header.Set("X-User", "uid-1")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mds := mock.NewMockDataStore(mockCtrl)
	oU := mock.NewMockUserOps(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU)
	oU.EXPECT().Fetch(r.Context(), "uid-1").Return(uObj, nil)
	exp = &Info{
		AuthIdentifier: "admin",
		UserID:         "uid-1",
	}
	ctx.DS = mds
	auth, err = ctx.NewInfo(r)
	assert.NoError(err)
	assert.Equal(exp, auth)
	assert.True(auth.Internal())
	tl.Flush()
	mockCtrl.Finish()

	t.Log("trusted, pass both user and account")
	r.Header.Set("X-Account", "account-1")
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oU = mock.NewMockUserOps(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU)
	oU.EXPECT().Fetch(r.Context(), "uid-1").Return(uObj, nil)
	oA := mock.NewMockAccountOps(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	oA.EXPECT().Fetch(r.Context(), "account-1").Return(aObj, nil)
	exp = &Info{
		AuthIdentifier: "admin",
		AccountID:      "account-1",
		AccountName:    "System",
		UserID:         "uid-1",
	}
	ctx.DS = mds
	auth, err = ctx.NewInfo(r)
	assert.NoError(err)
	assert.Equal(exp, auth)
	assert.True(auth.Internal())
	assert.Equal(auth.AccountID, auth.GetAccountID())
	tl.Flush()
	mockCtrl.Finish()

	t.Log("user not found")
	r.Header.Del("X-Account")
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oU = mock.NewMockUserOps(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU)
	oU.EXPECT().Fetch(r.Context(), "uid-1").Return(nil, app.ErrorNotFound)
	ctx.DS = mds
	auth, err = ctx.NewInfo(r)
	assert.NotNil(auth)
	assert.Regexp("user not found", err)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("user db error")
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oU = mock.NewMockUserOps(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU)
	oU.EXPECT().Fetch(r.Context(), "uid-1").Return(nil, app.ErrorDbError)
	ctx.DS = mds
	auth, err = ctx.NewInfo(r)
	assert.NotNil(auth)
	assert.Equal(app.ErrorDbError, err)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("user disabled")
	uObj.Disabled = true
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oU = mock.NewMockUserOps(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU)
	oU.EXPECT().Fetch(r.Context(), "uid-1").Return(uObj, nil)
	ctx.DS = mds
	auth, err = ctx.NewInfo(r)
	assert.NotNil(auth)
	assert.Equal("user is disabled", err.Error())
	tl.Flush()
	uObj.Disabled = false
	mockCtrl.Finish()

	t.Log("user role disabled")
	r.Header.Set("X-Account", "account-1")
	aObj.UserRoles["uid-1"] = models.AuthRole{Disabled: true, RoleID: "role-1"}
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oU = mock.NewMockUserOps(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU)
	oU.EXPECT().Fetch(r.Context(), "uid-1").Return(uObj, nil)
	oA = mock.NewMockAccountOps(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	oA.EXPECT().Fetch(r.Context(), "account-1").Return(aObj, nil)
	ctx.DS = mds
	auth, err = ctx.NewInfo(r)
	assert.NotNil(auth)
	assert.Regexp("user is disabled in the account", err)
	tl.Flush()
	aObj.UserRoles["uid-1"] = models.AuthRole{RoleID: "role-1"}
	mockCtrl.Finish()

	t.Log("account disabled")
	aObj.Disabled = true
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oU = mock.NewMockUserOps(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU)
	oU.EXPECT().Fetch(r.Context(), "uid-1").Return(uObj, nil)
	oA = mock.NewMockAccountOps(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	oA.EXPECT().Fetch(r.Context(), "account-1").Return(aObj, nil)
	ctx.DS = mds
	auth, err = ctx.NewInfo(r)
	assert.NotNil(auth)
	assert.Regexp("account is disabled", err)
	tl.Flush()
	aObj.Disabled = false
	mockCtrl.Finish()

	t.Log("Auth disabled, cover account not found")
	r.RemoteAddr = "1.2.3.4"
	r.Header = make(http.Header)
	r.Header.Set("X-Account", "account-1")
	ctx.Disabled = true
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	oA.EXPECT().Fetch(r.Context(), "account-1").Return(nil, app.ErrorNotFound)
	ctx.DS = mds
	auth, err = ctx.NewInfo(r)
	assert.NotNil(auth)
	assert.Equal("account not found", err.Error())
	tl.Flush()
	ctx.Disabled = false
	mockCtrl.Finish()

	t.Log("Auth disabled with X-Auth present, cover account db error")
	r.RemoteAddr = "1.2.3.4"
	r.Header = make(http.Header)
	r.Header.Set("X-Auth", "titus")
	r.Header.Set("X-Account", "account-1")
	ctx.Disabled = true
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	oA.EXPECT().Fetch(r.Context(), "account-1").Return(nil, app.ErrorDbError)
	ctx.DS = mds
	auth, err = ctx.NewInfo(r)
	assert.NotNil(auth)
	assert.Equal(app.ErrorDbError, err)
	tl.Flush()
	ctx.Disabled = false
	mockCtrl.Finish()

	t.Log("X-Auth valid, X-User not allowed")
	r.Header.Set("X-User", "user-1")
	resp.Body = ioutil.NopCloser(strings.NewReader(fakeValidateResp))
	ft.resp = resp
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oU = mock.NewMockUserOps(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU)
	oU.EXPECT().List(r.Context(), user.UserListParams{AuthIdentifier: swag.String("me")}).Return(uList, nil)
	ctx.DS = mds
	auth, err = ctx.NewInfo(r)
	assert.NotNil(auth)
	assert.Regexp("not a trusted client", err)
	tl.Flush()

	t.Log("TLS and nginx headers, success path")
	r.Header = make(http.Header)
	r.Header.Set("X-Ssl-Client-Dn", "client-dn,a=b,c=d")
	r.Header.Set("X-Ssl-Client-Verify", "SUCCESS")
	r.Header.Set("X-Real-IP", "5.6.7.8")
	r.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{
			&x509.Certificate{
				Subject: pkix.Name{CommonName: "common"},
			},
		},
	}
	exp = &Info{
		CertCN:             "common",
		ClientDN:           "client-dn,a=b,c=d",
		ClientCertVerified: true,
		RemoteAddr:         "5.6.7.8",
	}
	auth, err = ctx.NewInfo(r)
	assert.NoError(err)
	assert.Equal(exp, auth)
	tl.Flush()

	t.Log("TLS and nginx headers, unverified client")
	r.Header.Del("X-Ssl-Client-Verify")
	auth, err = ctx.NewInfo(r)
	assert.NotNil(auth)
	assert.Regexp("missing auth token", err)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("X-Auth, wrong X-Account")
	r.Header.Set("X-Auth", "titus")
	r.Header.Set("X-Account", "account-1")
	resp.Body = ioutil.NopCloser(strings.NewReader(fakeValidateResp))
	ft.resp = resp
	aObj.UserRoles = make(map[string]models.AuthRole)
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oU = mock.NewMockUserOps(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU)
	oU.EXPECT().List(r.Context(), user.UserListParams{AuthIdentifier: swag.String("me")}).Return(uList, nil)
	oA = mock.NewMockAccountOps(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	oA.EXPECT().Fetch(r.Context(), "account-1").Return(aObj, nil)
	ctx.DS = mds
	auth, err = ctx.NewInfo(r)
	assert.NotNil(auth)
	assert.Regexp("user is not associated with account", err)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("X-Auth, wrong AuthIdentifier")
	resp.Body = ioutil.NopCloser(strings.NewReader(fakeValidateResp))
	ft.resp = resp
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oU = mock.NewMockUserOps(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU)
	oU.EXPECT().List(r.Context(), user.UserListParams{AuthIdentifier: swag.String("me")}).Return(nil, nil)
	ctx.DS = mds
	auth, err = ctx.NewInfo(r)
	assert.NotNil(auth)
	assert.Regexp("user not found", err)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("X-Auth, role not found")
	r.Header.Set("X-Auth", "titus")
	r.Header.Set("X-Account", "account-1")
	aObj.UserRoles["uid-1"] = models.AuthRole{RoleID: "role-1"}
	resp.Body = ioutil.NopCloser(strings.NewReader(fakeValidateResp))
	ft.resp = resp
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oU = mock.NewMockUserOps(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU)
	oU.EXPECT().List(r.Context(), user.UserListParams{AuthIdentifier: swag.String("me")}).Return(uList, nil)
	oA = mock.NewMockAccountOps(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	oA.EXPECT().Fetch(r.Context(), "account-1").Return(aObj, nil)
	oR := mock.NewMockRoleOps(mockCtrl)
	mds.EXPECT().OpsRole().Return(oR)
	oR.EXPECT().Fetch("role-1").Return(nil, app.ErrorNotFound)
	ctx.DS = mds
	auth, err = ctx.NewInfo(r)
	assert.NotNil(auth)
	assert.Regexp("role not found", err)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("X-Auth, no X-Account")
	r.Header.Set("X-Auth", "titus")
	r.Header.Del("X-Account")
	resp.Body = ioutil.NopCloser(strings.NewReader(fakeValidateResp))
	ft.resp = resp
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oU = mock.NewMockUserOps(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU)
	oU.EXPECT().List(r.Context(), user.UserListParams{AuthIdentifier: swag.String("me")}).Return(uList, nil)
	ctx.DS = mds
	auth, err = ctx.NewInfo(r)
	assert.NoError(err)
	if assert.NotNil(auth) {
		assert.Equal(&unknownRole, auth.RoleObj)
	}
	tl.Flush()
	mockCtrl.Finish()

	t.Log("X-Auth, role db error")
	r.Header.Set("X-Auth", "titus")
	r.Header.Set("X-Account", "account-1")
	resp.Body = ioutil.NopCloser(strings.NewReader(fakeValidateResp))
	ft.resp = resp
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oU = mock.NewMockUserOps(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU)
	oU.EXPECT().List(r.Context(), user.UserListParams{AuthIdentifier: swag.String("me")}).Return(uList, nil)
	oA = mock.NewMockAccountOps(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	oA.EXPECT().Fetch(r.Context(), "account-1").Return(aObj, nil)
	oR = mock.NewMockRoleOps(mockCtrl)
	mds.EXPECT().OpsRole().Return(oR)
	oR.EXPECT().Fetch("role-1").Return(nil, app.ErrorDbError)
	ctx.DS = mds
	auth, err = ctx.NewInfo(r)
	assert.NotNil(auth)
	assert.Equal(app.ErrorDbError, err)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("X-Auth, db error")
	resp.Body = ioutil.NopCloser(strings.NewReader(fakeValidateResp))
	ft.resp = resp
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oU = mock.NewMockUserOps(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU)
	oU.EXPECT().List(r.Context(), user.UserListParams{AuthIdentifier: swag.String("me")}).Return(nil, app.ErrorDbError)
	ctx.DS = mds
	auth, err = ctx.NewInfo(r)
	assert.NotNil(auth)
	assert.Equal(app.ErrorDbError, err)
	tl.Flush()

	t.Log("X-Auth, validation error")
	ft.resp.Body = ft
	auth, err = ctx.NewInfo(r)
	assert.NotNil(auth)
	assert.Regexp("fake read error", err)
}

func TestCreateHTTPClient(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	ctx := &Config{
		Host: "author",
		Port: 5555,
		Log:  tl.Logger(),
	}

	// success, no-SSL
	assert.NoError(ctx.createHTTPClient())
	assert.NotNil(ctx.client)
	assert.Nil(clientTransport.TLSClientConfig)

	// no-op when initialized
	origClient := ctx.client
	assert.NoError(ctx.createHTTPClient())
	assert.True(origClient == ctx.client)
	assert.Nil(clientTransport.TLSClientConfig)

	// error reading crt file
	ctx.client = nil
	ctx.UseSSL = true
	ctx.TLSCACertificate = "no-such-file.crt"
	err := ctx.createHTTPClient()
	assert.Regexp("no such file", err)
	assert.Nil(ctx.client)
	assert.Nil(clientTransport.TLSClientConfig)

	// success
	ctx.TLSCACertificate = "./ca.crt"
	ctx.SSLServerName = "set"
	assert.NoError(ctx.createHTTPClient())
	assert.NotNil(ctx.client)
	assert.NotNil(clientTransport.TLSClientConfig)
	assert.NotNil(clientTransport.TLSClientConfig.RootCAs)
	assert.Equal("set", clientTransport.TLSClientConfig.ServerName)
	assert.False(clientTransport.TLSClientConfig.InsecureSkipVerify)
	clientTransport.TLSClientConfig = nil
}

func TestValidate(t *testing.T) {
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

	// failure creating http client
	ctx.UseSSL = true
	ctx.TLSCACertificate = "no-such-file.crt"
	auth, err := ctx.Validate("my-token")
	assert.Nil(auth)
	assert.Regexp("no such file", err)
	ctx.UseSSL = false
	ctx.TLSCACertificate = ""

	// success
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
	auth, err = ctx.Validate("my-token")
	assert.NoError(err)
	assert.NotNil(ctx.client)
	assert.Equal("http", ft.req.URL.Scheme)
	assert.Equal("author:5555", ft.req.Host)
	assert.Equal("POST", ft.req.Method)
	assert.Equal("/auth/validate", ft.req.URL.Path)
	assert.Equal("my-token", ft.req.Header.Get("Token"))
	assert.Zero(ft.req.ContentLength)
	assert.Equal("new-token", auth.Token)
	assert.EqualValues(12345, auth.Expiry)
	assert.Equal("Nuvoloso", auth.Issuer)
	assert.Equal("me", auth.Username)
	savedClient := ctx.client

	// NewRequest fails, verify client is re-used
	ft.req = nil
	ft.resp = nil
	ctx.Host = "@["
	auth, err = ctx.Validate("my-token")
	assert.Nil(ft.req)
	assert.Nil(auth)
	if assert.Error(err) {
		_, ok := err.(*url.Error)
		assert.True(ok)
	}
	assert.True(savedClient == ctx.client)
	ctx.Host = "author"
	tl.Flush()

	// failure, with SSL and debug enabled
	ctx.UseSSL = true
	ctx.DebugClient = true
	resp.StatusCode = 403
	resp.Status = "403 Forbidden"
	resp.Header["Content-Type"] = []string{"text/plain"}
	resp.Body = ioutil.NopCloser(strings.NewReader(`you lose`))
	ft.req = nil
	ft.resp = resp
	auth, err = ctx.Validate("bad-token")
	assert.Nil(auth)
	if assert.Error(err) {
		assert.Equal("you lose", err.Error())
	}
	assert.True(savedClient == ctx.client)
	assert.Equal("https", ft.req.URL.Scheme)
	assert.Equal(1, tl.CountPattern("Content-Length: 0"))
	assert.Equal(1, tl.CountPattern("Token: bad-token"))
	assert.Equal(1, tl.CountPattern("Content-Type")) // only in response
	ctx.UseSSL = false
	tl.Flush()

	// invalid response
	resp.StatusCode = 200
	resp.Status = "200 OK"
	resp.Header["Content-Type"] = []string{"application/json"}
	resp.Body = ioutil.NopCloser(strings.NewReader(`you lose`))
	ft.req = nil
	ft.resp = resp
	auth, err = ctx.Validate("good-token")
	assert.Nil(auth)
	if assert.Error(err) {
		assert.Regexp("invalid character", err.Error())
	}
	assert.Equal(1, tl.CountPattern("invalid response body"))
	tl.Flush()

	// cover response dump failure
	resp.Body = ft
	auth, err = ctx.Validate("good-token")
	assert.Nil(auth)
	assert.Error(err)
	assert.Equal(1, tl.CountPattern("DumpResponse"))
	tl.Flush()

	// transport error
	ft.failConnect = true
	auth, err = ctx.Validate("good-token")
	assert.Nil(auth)
	assert.Error(err)
	assert.Equal(1, tl.CountPattern("transport failure"))
}

// fakeTransport is a RoundTripper and a ReadCloser
type fakeTransport struct {
	t           *testing.T
	log         *logging.Logger
	resp        *http.Response
	req         *http.Request
	failConnect bool
}

func (tr *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tr.req = req
	if tr.failConnect {
		return nil, errors.New("failed connect")
	}
	tr.resp.Request = req
	return tr.resp, nil
}

func (tr *fakeTransport) Read(p []byte) (n int, err error) {
	return 0, errors.New("fake read error")
}

func (tr *fakeTransport) Close() error {
	return nil
}

var fakeValidateResp = `{"token": "new-token", "exp": 12345, "iss": "Nuvoloso", "username": "me"}`
