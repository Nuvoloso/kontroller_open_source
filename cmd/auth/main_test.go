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
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/auth"
	"github.com/Nuvoloso/kontroller/pkg/mongodb"
	"github.com/Nuvoloso/kontroller/pkg/mongodb/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/dgrijalva/jwt-go"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/sessions"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"
)

// helper methods

func getMockToken(username string, expiry int64, signingMethod jwt.SigningMethod) string {
	claims := AuthClaims{
		username,
		jwt.StandardClaims{
			ExpiresAt: expiry,
			Issuer:    Issuer,
		},
	}
	token := jwt.NewWithClaims(signingMethod, claims)
	tokenString, _ := token.SignedString([]byte(secretPrefix))
	return tokenString
}

func postLoginPayload(username string, password string) string {
	testUser := LoginParams{username, password}
	payload, _ := json.Marshal(testUser)
	return string(payload)
}

func postLoginTest(client *http.Client, baseURL string, body string) (Auth, error) {
	req, _ := http.NewRequest("POST", baseURL+"/auth/login", bytes.NewBuffer([]byte(body)))
	req.Header.Set("Content-Type", "application/json")

	return reqAuth(client, req)
}

func postValidateTest(client *http.Client, baseURL string, token string) (Auth, error) {
	req, err := http.NewRequest("POST", baseURL+"/auth/validate", nil)
	if err != nil {
		return Auth{}, err
	}
	req.Header.Set("token", token)

	return reqAuth(client, req)
}

func reqAuth(client *http.Client, req *http.Request) (Auth, error) {
	res, err := client.Do(req)
	if err != nil {
		return Auth{}, err
	}

	body, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return Auth{}, err
	}

	auth := Auth{}
	err = json.Unmarshal(body, &auth)
	if err != nil {
		if res.StatusCode != http.StatusOK {
			// body is an error message on failure, return an error derived from the status and message so caller can verify the error is as expected
			err = fmt.Errorf("%s (%s)", res.Status, strings.TrimSpace(string(body)))
		}
		return Auth{}, err
	}

	return auth, nil
}

// testing functions

func TestAuthRoutes(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := mock.NewMockDBAPI(mockCtrl)
	app := &mainContext{Expiry: time.Duration(2 * time.Hour), db: api, log: l, store: newCookieStore(api)}
	app.Extractor = &auth.Extractor{}
	assert.NotNil(app.store)
	app.store.secret = []byte(secretPrefix)
	store := sessions.NewCookieStore(app.store.secret)
	app.store.store = store
	ts := httptest.NewServer(app.Register())
	defer ts.Close()

	cookieJar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: cookieJar,
	}

	auth, err := postLoginTest(client, ts.URL, "adf")
	assert.Regexp("^401 Unauthorized.*invalid character 'a'", err, "[POST /auth/login] malformed payload did not receive error")

	ctx := gomock.Not(gomock.Nil())
	api.EXPECT().MustBeReady().Return(nil)
	mc := mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(mc)
	api.EXPECT().DBName().Return("mock")
	db := mock.NewMockDatabase(mockCtrl)
	mc.EXPECT().Database("mock").Return(db)
	col := mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection("user").Return(col)
	sr := mock.NewMockSingleResult(mockCtrl)
	col.EXPECT().FindOne(ctx, bson.D{{Key: "authidentifier", Value: "fake"}}).Return(sr)
	sr.EXPECT().Decode(newMockDecodeMatcher(t, nil)).Return(mongo.ErrNoDocuments)
	api.EXPECT().ErrorCode(mongo.ErrNoDocuments).Return(mongodb.ECKeyNotFound)
	auth, err = postLoginTest(client, ts.URL, postLoginPayload("fake", "fake"))
	assert.Regexp("^401 Unauthorized.*Incorrect username or password", err, "[POST /auth/login] invalid username or password did not receive error")

	api.EXPECT().MustBeReady().Return(nil)
	mc = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(mc)
	api.EXPECT().DBName().Return("mock")
	db = mock.NewMockDatabase(mockCtrl)
	mc.EXPECT().Database("mock").Return(db)
	col = mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection("user").Return(col)
	sr = mock.NewMockSingleResult(mockCtrl)
	col.EXPECT().FindOne(ctx, bson.D{{Key: "authidentifier", Value: ""}}).Return(sr)
	sr.EXPECT().Decode(newMockDecodeMatcher(t, nil)).Return(mongo.ErrNoDocuments)
	api.EXPECT().ErrorCode(mongo.ErrNoDocuments).Return(mongodb.ECKeyNotFound)
	auth, err = postLoginTest(client, ts.URL, `{"password":"`+"fake"+`"}`)
	assert.Regexp("^401 Unauthorized.*Incorrect username or password", err, "[POST /auth/login] malformed request object did not receive error")

	pw, err := bcrypt.GenerateFromPassword([]byte("fake"), 0)
	if !assert.NoError(err) {
		assert.FailNow("bcrypt.GenerateFromPassword must not fail")
	}
	userObj := bson.M{
		"authidentifier": "fake",
		"disabled":       true,
		"password":       string(pw),
	}
	api.EXPECT().MustBeReady().Return(nil)
	mc = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(mc)
	api.EXPECT().DBName().Return("mock")
	db = mock.NewMockDatabase(mockCtrl)
	mc.EXPECT().Database("mock").Return(db)
	col = mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection("user").Return(col)
	sr = mock.NewMockSingleResult(mockCtrl)
	col.EXPECT().FindOne(ctx, bson.D{{Key: "authidentifier", Value: "fake"}}).Return(sr)
	sr.EXPECT().Decode(newMockDecodeMatcher(t, userObj)).Return(nil)
	api.EXPECT().ErrorCode(nil).Return(mongodb.ECKeyNotFound)
	auth, err = postLoginTest(client, ts.URL, postLoginPayload("fake", "fake"))
	assert.Regexp("^401 Unauthorized.*User is disabled", err, "[POST /auth/login] disabled user did not receive error")

	t.Log("case: cannot initialize store")
	app.store.store = nil
	api.EXPECT().MustBeReady().Return(errUnknownError)
	auth, err = postLoginTest(client, ts.URL, postLoginPayload("fake", "fake"))
	assert.Regexp("^500 Internal Server Error .auth service is not ready", err, "[POST /auth/login] not ready")
	app.store.store = store

	t.Log("case: findUser DB error")
	api.EXPECT().MustBeReady().Return(errUnknownError)
	auth, err = postLoginTest(client, ts.URL, postLoginPayload("fake", "fake"))
	assert.Regexp("^500 Internal Server Error .auth service is not ready", err, "[POST /auth/login] not ready")

	userObj["disabled"] = false
	api.EXPECT().MustBeReady().Return(nil)
	mc = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(mc)
	api.EXPECT().DBName().Return("mock")
	db = mock.NewMockDatabase(mockCtrl)
	mc.EXPECT().Database("mock").Return(db)
	col = mock.NewMockCollection(mockCtrl)
	db.EXPECT().Collection("user").Return(col)
	sr = mock.NewMockSingleResult(mockCtrl)
	col.EXPECT().FindOne(ctx, bson.D{{Key: "authidentifier", Value: "fake"}}).Return(sr)
	sr.EXPECT().Decode(newMockDecodeMatcher(t, userObj)).Return(nil)
	api.EXPECT().ErrorCode(nil).Return(mongodb.ECKeyNotFound)
	auth, err = postLoginTest(client, ts.URL, postLoginPayload("fake", "fake"))
	assert.NoError(err)

	assert.Equal("fake", auth.Username, "[POST /auth/login] default username does not match username in response object")

	if t.Failed() {
		t.Errorf("[POST /auth/login] failed, aborting TestAuthRoutes()")
		t.FailNow()
	} else {
		t.Log("[POST /auth/login] tests successful")
	}

	validToken := auth.Token
	origExpiry := auth.Expiry

	time.Sleep(time.Second)
	auth, err = postValidateTest(client, ts.URL, validToken)
	assert.NoError(err)

	assert.False(auth.Expiry < time.Now().Unix())
	assert.True(auth.Expiry > origExpiry)
	time.Sleep(time.Second)
	req, err := http.NewRequest("POST", ts.URL+"/auth/validate?preserve-expiry=true", nil)
	assert.NoError(err)
	req.Header.Set("token", validToken)
	auth2, err := reqAuth(client, req)
	assert.NoError(err)
	if assert.NotNil(auth2) {
		assert.Equal(origExpiry, auth2.Expiry)
	}

	app.store.store = nil
	api.EXPECT().MustBeReady().Return(errUnknownError)
	auth, err = postValidateTest(client, ts.URL, "aaa.bbb.ccc")
	assert.Regexp("^500 Internal Server Error .auth service is not ready", err, "[POST /auth/validate] not ready")
	app.store.store = store

	auth, err = postValidateTest(client, ts.URL, "aaa.bbb.ccc")
	assert.Regexp("^401 Unauthorized.*Invalid token", err, "[POST /auth/validate] invalid token did not return error")

	mockToken := getMockToken("fake", time.Now().Add(-time.Minute).Unix(), jwt.SigningMethodHS256)
	auth, err = postValidateTest(client, ts.URL, mockToken)
	assert.Regexp("^401 Unauthorized.*Expired token", err, "[POST /auth/validate] expired token did not return error")

	mockToken = getMockToken("fake", time.Now().Add(2*time.Hour).Unix(), jwt.SigningMethodRS256)
	auth, err = postValidateTest(client, ts.URL, mockToken)
	assert.Regexp("^401 Unauthorized.*Unable to parse token", err, "[POST /auth/validate] token with incorrect signing method did not return error")

	mockToken = getMockToken("", time.Now().Add(2*time.Hour).Unix(), jwt.SigningMethodHS256)
	auth, err = postValidateTest(client, ts.URL, mockToken)
	assert.Regexp("^401 Unauthorized.*Invalid JWT claims", err, "[POST /auth/validate] token with incorrect claims did not return error")

	if t.Failed() {
		t.Errorf("[POST /auth/validate] failed, aborting TestAuthRoutes()")
		t.FailNow()
	} else {
		t.Log("[POST /auth/validate] tests successful")
	}

	req, _ = http.NewRequest("POST", ts.URL+"/auth/logout", nil)
	_, err = client.Do(req)
	assert.NoError(err)

	app.store.store = nil
	api.EXPECT().MustBeReady().Return(errUnknownError)
	req, _ = http.NewRequest("POST", ts.URL+"/auth/logout", nil)
	_, err = reqAuth(client, req)
	assert.Regexp("^500 Internal Server Error .auth service is not ready", err, "[POST /auth/logout] not ready")
	app.store.store = store

	req, err = http.NewRequest("POST", ts.URL+"/auth/validate", nil)
	req.Header.Set("token", validToken)

	auth, err = reqAuth(client, req)
	assert.Regexp("^403 Forbidden", err, "[POST /auth/validate] should have failed after logging out")

	if t.Failed() {
		t.Errorf("[POST /auth/logout] failed, aborting TestAuthRoutes()")
		t.FailNow()
	} else {
		t.Log("[POST /auth/logout] tests successful")
	}

	if !t.Failed() {
		t.Log("[APIs /auth] tests successful")
	}
}

func TestGetNewAuth(t *testing.T) {
	assert := assert.New(t)
	app := &mainContext{Expiry: time.Duration(5 * time.Minute)}
	app.store = newCookieStore(nil)
	assert.NotNil(app.store)
	app.store.secret = []byte(secretPrefix)
	app.store.store = sessions.NewCookieStore(app.store.secret)

	username := "fake"
	authResp := app.getNewAuth(username, 0)

	auth := Auth{}
	err := json.Unmarshal(authResp, &auth)
	assert.NoError(err)
	assert.Equal(username, auth.Username)
	assert.NotZero(auth.Expiry)

	// marshal never fails
	_, err = json.Marshal(auth)
	assert.NoError(err)

	// expiry preserved
	expiry := auth.Expiry - 60
	authResp = app.getNewAuth(username, expiry)
	assert.NoError(err)

	auth = Auth{}
	err = json.Unmarshal(authResp, &auth)
	assert.NoError(err)
	assert.Equal(username, auth.Username)
	assert.Equal(expiry, auth.Expiry)

	if !t.Failed() {
		t.Log("[method getNewAuth()] tests successful")
	}
}

func TestGetNewExpiry(t *testing.T) {
	assert := assert.New(t)
	app := &mainContext{Expiry: time.Duration(5 * time.Minute)}
	now := time.Now()
	exp := app.getNewExpiry()
	after := now.Add(time.Duration(5 * time.Minute))
	assert.True(now.Unix() < exp && exp <= after.Unix(), "new expiry should be in range")
}

func TestParseArgs(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	var b bytes.Buffer
	origStderr := os.Stderr
	origStdout := os.Stdout
	savedArgs := os.Args
	savedTemplate := iniFileNameTemplate
	iniFileNameTemplate = "./%s.ini"
	exitCode := -1
	exitHook = func(code int) {
		exitCode = code
		panic("exitHook called")
	}
	defer func() {
		os.Args = savedArgs
		os.Stderr = origStderr
		os.Stdout = origStdout
		exitHook = os.Exit
		iniFileNameTemplate = savedTemplate
	}()
	Appname = "testbad"

	app := &mainContext{ServerArgs: &ServerArgs{}, log: l}
	var err error
	assert.NotPanics(func() { err = app.parseArgs() })
	assert.Regexp("Not an INI file", err)
	assert.Zero(tl.CountPattern("."))

	// CLI parse failure exits
	Appname = "no-such-file"
	os.Args = []string{Appname, "-x"}
	app = &mainContext{ServerArgs: &ServerArgs{}, log: l}
	r, w, _ := os.Pipe()
	os.Stderr = w
	outC := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outC <- buf.String()
	}()
	assert.PanicsWithValue("exitHook called", func() { app.parseArgs() })
	assert.Equal(1, exitCode)
	assert.Equal(1, tl.CountPattern("file .* not found"))
	w.Close()
	out := <-outC
	assert.Regexp("unknown flag", out)
	exitCode = -1
	os.Stderr = origStderr
	tl.Flush()

	// good ini file, --help prints and exits
	b.Reset()
	Appname = "testgood"
	os.Args = []string{Appname, "--help"}
	app = &mainContext{ServerArgs: &ServerArgs{}, log: l}
	r, w, _ = os.Pipe()
	os.Stdout = w
	outC = make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outC <- buf.String()
	}()
	assert.PanicsWithValue("exitHook called", func() { app.parseArgs() })
	w.Close()
	out = <-outC
	assert.Regexp("Usage:", out)
	assert.Zero(exitCode)
	assert.Equal(1, tl.CountPattern("Read configuration defaults from ./testgood.ini"))
	assert.Equal(time.Hour, app.Expiry)
	expArgs := &ServerArgs{
		EnabledListener:   "https",
		CleanupTimeout:    5 * time.Second,
		MaxHeaderSize:     1000 * 1000,
		Host:              "another",
		Port:              80,
		ReadTimeout:       8 * time.Second,
		WriteTimeout:      9 * time.Second,
		TLSHost:           "1.2.3.4",
		TLSPort:           443,
		TLSCertificate:    "auth.crt",
		TLSCertificateKey: "auth.key",
		TLSCACertificate:  "ca.crt",
		TLSReadTimeout:    13 * time.Second,
		TLSWriteTimeout:   19 * time.Second,
	}
	assert.Equal(expArgs, app.ServerArgs)
	tl.Flush()

	// no ini file, --version prints and exits
	b.Reset()
	Appname = "no-such-file"
	BuildJob = "kontroller:1"
	os.Args = []string{Appname, "--version"}
	app = &mainContext{ServerArgs: &ServerArgs{}, log: l}
	r, w, _ = os.Pipe()
	os.Stdout = w
	outC = make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outC <- buf.String()
	}()
	assert.PanicsWithValue("exitHook called", func() { app.parseArgs() })
	w.Close()
	out = <-outC
	assert.Zero(exitCode)
	assert.Regexp("Build ID:", out)
	tl.Flush()

	// write-config
	Appname = "no-such-file"
	outFile := "./test-cfg.ini"
	os.Args = []string{Appname, "--write-config=" + outFile}
	app = &mainContext{ServerArgs: &ServerArgs{}, log: l}
	os.Remove(outFile)
	r, w, _ = os.Pipe()
	os.Stdout = w
	outC = make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outC <- buf.String()
	}()
	assert.PanicsWithValue("exitHook called", func() { app.parseArgs() })
	w.Close()
	out = <-outC
	assert.Zero(exitCode)
	assert.Regexp("Wrote configuration to "+outFile, out)
	stat, err := os.Stat(outFile)
	assert.NoError(err)
	assert.NotZero(stat.Size)
	os.Remove(outFile)
	tl.Flush()

	// success
	app = &mainContext{ServerArgs: &ServerArgs{}, log: l}
	os.Args = []string{Appname, "--scheme=https", "--host=0.0.0.0", "--tls-port=443"}
	assert.NotPanics(func() { err = app.parseArgs() })
	assert.NoError(err)
	assert.Equal(2*time.Hour, app.Expiry)
	expArgs = &ServerArgs{
		EnabledListener:   "https",
		CleanupTimeout:    10 * time.Second,
		MaxHeaderSize:     1000 * 1000,
		Host:              "0.0.0.0",
		Port:              5555,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      time.Minute,
		TLSHost:           "",
		TLSPort:           443,
		TLSCertificate:    "",
		TLSCertificateKey: "",
		TLSCACertificate:  "",
		TLSReadTimeout:    0,
		TLSWriteTimeout:   0,
	}
	assert.Equal(expArgs, app.ServerArgs)
}

func TestInit(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	Appname = "test"

	savedDbHook := dbAPIHook
	savedShutdownHook := shutdownHook
	defer func() {
		dbAPIHook = savedDbHook
		shutdownHook = savedShutdownHook
		systemObj = System{}
	}()

	t.Log("case: success, http")
	app := &mainContext{
		MongoArgs: mongodb.Args{
			URL:          "mongodb://localhost:17",
			DatabaseName: "testDB",
		},
		ServerArgs: &ServerArgs{
			EnabledListener: "http",
			Host:            "localhost",
			Port:            42,
			MaxHeaderSize:   1024 * 1024,
			CleanupTimeout:  time.Duration(10 * time.Millisecond),
			ReadTimeout:     time.Duration(11 * time.Millisecond),
			WriteTimeout:    time.Duration(12 * time.Millisecond),
		},
		log: l,
	}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := mock.NewMockDBAPI(mockCtrl)
	dbAPIHook = func(*mongodb.Args) mongodb.DBAPI {
		return api
	}
	api.EXPECT().Connect(nil).Return(nil)
	assert.NoError(app.init())
	assert.NotNil(app.Extractor)
	assert.Equal(Appname, app.MongoArgs.AppName)
	assert.Equal(l, app.MongoArgs.Log)
	assert.False(app.MongoArgs.UseSSL)
	// terminate the signal handler go routine
	assert.NotNil(app.idleConnsClosed)
	for app.sigChan == nil {
		time.Sleep(10 * time.Millisecond)
	}
	api.EXPECT().Terminate()
	app.sigChan <- syscall.SIGTERM
	<-app.idleConnsClosed
	if assert.NotNil(app.server) {
		s := app.server
		assert.NotNil(s.Handler)
		assert.Equal("localhost:42", s.Addr)
		assert.EqualValues(app.ServerArgs.MaxHeaderSize, s.MaxHeaderBytes)
		assert.Equal(app.ServerArgs.ReadTimeout, s.ReadTimeout)
		assert.Equal(app.ServerArgs.ReadTimeout, s.ReadHeaderTimeout)
		assert.Equal(app.ServerArgs.WriteTimeout, s.WriteTimeout)
		assert.Nil(s.TLSConfig)
	}
	assert.Equal(1, tl.CountPattern("shutting down"))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: success, https")
	app.MongoArgs.Log = nil
	app.MongoArgs.AppName = ""
	app.MongoArgs.UseSSL = true
	app.ServerArgs.EnabledListener = "https"
	app.ServerArgs.TLSCACertificate = "./ca.crt"
	app.ServerArgs.TLSCertificate = "./not-empty.crt"
	app.ServerArgs.TLSCertificateKey = "./not-empty.key"
	app.ServerArgs.TLSReadTimeout = time.Duration(13 * time.Millisecond)
	app.ServerArgs.TLSWriteTimeout = time.Duration(14 * time.Millisecond)
	app.idleConnsClosed = nil
	app.sigChan = nil
	app.server = nil
	app.Extractor = nil
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockDBAPI(mockCtrl)
	api.EXPECT().Connect(nil).Return(nil)
	assert.NoError(app.init())
	assert.NotNil(app.Extractor)
	assert.Equal(Appname, app.MongoArgs.AppName)
	assert.Equal(l, app.MongoArgs.Log)
	assert.EqualValues(app.ServerArgs.TLSCACertificate, app.MongoArgs.TLSCACertificate)
	assert.EqualValues(app.ServerArgs.TLSCertificate, app.MongoArgs.TLSCertificate)
	assert.EqualValues(app.ServerArgs.TLSCertificateKey, app.MongoArgs.TLSCertificateKey)
	assert.True(app.MongoArgs.UseSSL)
	// terminate the signal handler go routine
	assert.NotNil(app.idleConnsClosed)
	for app.sigChan == nil {
		time.Sleep(10 * time.Millisecond)
	}
	api.EXPECT().Terminate()
	app.sigChan <- syscall.SIGTERM
	<-app.idleConnsClosed
	if assert.NotNil(app.server) {
		s := app.server
		assert.NotNil(s.Handler)
		assert.Equal("localhost:0", s.Addr) // TLSHost defaults to Host, TLSPort is not defaulted
		assert.EqualValues(app.ServerArgs.MaxHeaderSize, s.MaxHeaderBytes)
		assert.Equal(app.ServerArgs.TLSReadTimeout, s.ReadTimeout)
		assert.Equal(app.ServerArgs.TLSReadTimeout, s.ReadHeaderTimeout)
		assert.Equal(app.ServerArgs.TLSWriteTimeout, s.WriteTimeout)
		if assert.NotNil(s.TLSConfig) {
			c := s.TLSConfig
			assert.NotNil(c.ClientCAs)
			assert.Equal(tls.VerifyClientCertIfGiven, c.ClientAuth)
			assert.Equal([]string{"http/1.1"}, c.NextProtos)
			assert.True(c.PreferServerCipherSuites)
		}
	}
	assert.Equal(1, tl.CountPattern("shutting down"))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: https, specify TLSHost")
	app.ServerArgs.TLSHost = "0.0.0.0"
	app.ServerArgs.TLSPort = 443
	app.idleConnsClosed = nil
	app.sigChan = nil
	app.server = nil
	app.Extractor = nil
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockDBAPI(mockCtrl)
	api.EXPECT().Connect(nil).Return(nil)
	assert.NoError(app.init())
	assert.NotNil(app.Extractor)
	// terminate the signal handler go routine
	assert.NotNil(app.idleConnsClosed)
	for app.sigChan == nil {
		time.Sleep(10 * time.Millisecond)
	}
	api.EXPECT().Terminate()
	app.sigChan <- syscall.SIGTERM
	<-app.idleConnsClosed
	if assert.NotNil(app.server) {
		s := app.server
		assert.NotNil(s.Handler)
		assert.Equal("0.0.0.0:443", s.Addr)
		assert.EqualValues(app.ServerArgs.MaxHeaderSize, s.MaxHeaderBytes)
		assert.Equal(app.ServerArgs.TLSReadTimeout, s.ReadTimeout)
		assert.Equal(app.ServerArgs.TLSReadTimeout, s.ReadHeaderTimeout)
		assert.Equal(app.ServerArgs.TLSWriteTimeout, s.WriteTimeout)
		if assert.NotNil(s.TLSConfig) {
			c := s.TLSConfig
			assert.NotNil(c.ClientCAs)
			assert.Equal(tls.VerifyClientCertIfGiven, c.ClientAuth)
			assert.Equal([]string{"http/1.1"}, c.NextProtos)
			assert.True(c.PreferServerCipherSuites)
		}
	}
	assert.Equal(1, tl.CountPattern("shutting down"))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Connect error")
	app.idleConnsClosed = nil
	app.sigChan = nil
	app.server = nil
	app.Extractor = nil
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockDBAPI(mockCtrl)
	api.EXPECT().Connect(nil).Return(errUnknownError)
	assert.Equal(errUnknownError, app.init())
	assert.Nil(app.idleConnsClosed)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: simulate shutdown failure")
	shutdownHook = func(context.Context, *http.Server) error {
		return errors.New("Shutdown failure")
	}
	app.idleConnsClosed = nil
	app.sigChan = nil
	app.server = nil
	app.Extractor = nil
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockDBAPI(mockCtrl)
	api.EXPECT().Connect(nil).Return(nil)
	assert.NoError(app.init())
	assert.NotNil(app.Extractor)
	// terminate the signal handler go routine
	assert.NotNil(app.idleConnsClosed)
	for app.sigChan == nil {
		time.Sleep(10 * time.Millisecond)
	}
	api.EXPECT().Terminate()
	app.sigChan <- syscall.SIGINT
	<-app.idleConnsClosed
	shutdownHook = savedShutdownHook // reset
	assert.Equal(1, tl.CountPattern("shutting down"))
	assert.Equal(1, tl.CountPattern("server shutdown: Shutdown failure"))
	tl.Flush()

	t.Log("case: tlsConfig fails, no key")
	app.ServerArgs.TLSCertificateKey = ""
	app.idleConnsClosed = nil
	app.sigChan = nil
	app.server = nil
	app.Extractor = nil
	assert.Error(app.init())
	cfg, err := app.tlsConfig()
	assert.Nil(cfg)
	assert.Regexp("tls-.* required with scheme=https", err)
	assert.Nil(app.idleConnsClosed)

	// tlsConfig fails, no cert
	app.ServerArgs.TLSCertificate = ""
	app.ServerArgs.TLSCertificateKey = "key"
	cfg, err = app.tlsConfig()
	assert.Nil(cfg)
	assert.Regexp("tls-.* required with scheme=https", err)
	assert.Nil(app.idleConnsClosed)

	// tlsConfig fails, no ca cert
	app.ServerArgs.TLSCertificate = "crt"
	app.ServerArgs.TLSCACertificate = ""
	cfg, err = app.tlsConfig()
	assert.Nil(cfg)
	assert.Regexp("tls-.* required with scheme=https", err)
	assert.Nil(app.idleConnsClosed)

	// tlsConfig fails, file read error
	app.ServerArgs.TLSCACertificate = "./no-such-ca.crt"
	cfg, err = app.tlsConfig()
	assert.Nil(cfg)
	assert.Regexp("no such file", err)
	assert.Nil(app.idleConnsClosed)

	// tlsConfig fails, no certs found
	app.ServerArgs.TLSCACertificate = "./testbad.ini"
	cfg, err = app.tlsConfig()
	assert.Nil(cfg)
	assert.Regexp("no CA certs found", err)
	assert.Nil(app.idleConnsClosed)
}

func TestListenAndServe(t *testing.T) {
	assert := assert.New(t)
	app := &mainContext{ServerArgs: &ServerArgs{EnabledListener: "http"}}

	// http, invalid port
	app.server = &http.Server{Addr: "4.4.4.4:123456789"}
	assert.Regexp("invalid port", app.listenAndServe())

	// https, invalid port
	app.server = &http.Server{Addr: "4.4.4.4:123456789"}
	app.ServerArgs.EnabledListener = "https"
	assert.Regexp("invalid port", app.listenAndServe())

	// success with shutdown
	app.idleConnsClosed = make(chan struct{})
	close(app.idleConnsClosed)
	app.server = &http.Server{Addr: "localhost:0"}
	app.ServerArgs.EnabledListener = "http"
	assert.NoError(app.server.Shutdown(context.Background()))
	assert.NoError(app.listenAndServe())
}

func TestLogStart(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	app := &mainContext{ServerArgs: &ServerArgs{EnabledListener: "http"}, log: l}
	app.server = &http.Server{Addr: "localhost:42"}
	Appname = "test"

	BuildID = "1"
	BuildTime = "Z"
	BuildJob = "kontroller:1"
	BuildHost = "host"

	// prefer job
	app.logStart()
	assert.Equal("1 Z kontroller:1", app.ServiceVersion)
	assert.Equal(1, tl.CountPattern(`"ServiceVersion": "`+app.ServiceVersion))
	assert.Equal(1, tl.CountPattern(".test. running on http:localhost:42"))
	tl.Flush()

	// no job
	BuildID = "1"
	BuildTime = "Z"
	BuildJob = ""
	BuildHost = "host"
	app.ServerArgs.EnabledListener = "https"
	app.logStart()
	assert.Equal("1 Z host", app.ServiceVersion)
	assert.Equal(1, tl.CountPattern(`"ServiceVersion": "`+app.ServiceVersion))
	assert.Equal(1, tl.CountPattern(".test. running on https:localhost:42"))
}

func TestMain(t *testing.T) {
	assert := assert.New(t)

	var b bytes.Buffer
	origStderr := os.Stderr
	savedArgs := os.Args
	savedTemplate := iniFileNameTemplate
	iniFileNameTemplate = "./no-template-%s.ini"
	savedDbHook := dbAPIHook
	exitCode := -1
	exitHook = func(code int) {
		exitCode = code
		panic("exitHook called")
	}
	defer func() {
		os.Args = savedArgs
		os.Stderr = origStderr
		dbAPIHook = savedDbHook
		exitHook = os.Exit
		iniFileNameTemplate = savedTemplate
	}()

	t.Log("case: success, most defaults but random port to ensure listen success")
	Appname = "nvauth"
	BuildID = "1"
	BuildTime = "Z"
	BuildJob = "kontroller:1"
	BuildHost = "host"
	os.Args = []string{"nvauth", "--port=0"}
	go func() {
		for {
			time.Sleep(10 * time.Millisecond)
			if strings.Contains(b.String(), " running on ") {
				time.Sleep(200 * time.Millisecond) // Ok if listenAndServe called after sleep, it returns ErrServerClosed either way
				break
			}
		}
		syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	}()
	r, w, _ := os.Pipe()
	os.Stderr = w
	doneC := make(chan struct{})
	go func() {
		io.Copy(&b, r)
		close(doneC)
	}()
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := mock.NewMockDBAPI(mockCtrl)
	api.EXPECT().Connect(nil).Return(nil)
	api.EXPECT().Terminate()
	dbAPIHook = func(*mongodb.Args) mongodb.DBAPI {
		return api
	}
	assert.NotPanics(main)
	assert.Equal(-1, exitCode)
	w.Close()
	<-doneC
	line, _ := b.ReadString('\n')
	assert.Regexp("Configuration file.* not found", line)
	var err error
	for {
		if line, err = b.ReadString('\n'); err != nil || strings.HasSuffix(line, "}\n") {
			break
		}
	}
	assert.NoError(err)
	line, _ = b.ReadString('\n')
	assert.Regexp(".nvauth. running on http:localhost:0", line)
	line, _ = b.ReadString('\n')
	assert.Regexp("http server shutting down", line)
	line, _ = b.ReadString('\n')
	assert.Regexp("http server shutdown succeeded", line)
	mockCtrl.Finish()

	t.Log("case: bad log level")
	b.Reset()
	app := &mainContext{}
	r, w, _ = os.Pipe()
	os.Stderr = w
	doneC = make(chan struct{})
	go func() {
		io.Copy(&b, r)
		close(doneC)
	}()
	assert.PanicsWithValue("exitHook called", app.setupLogging)
	assert.Equal(1, exitCode)
	w.Close()
	<-doneC
	line, _ = b.ReadString('\n')
	assert.Regexp("Invalid log level", line)

	t.Log("case: parse failure (other paths tested elsewhere)")
	b.Reset()
	iniFileNameTemplate = "./%s.ini"
	Appname = "testbad"
	os.Args = []string{"nvauth", "--help"}
	r, w, _ = os.Pipe()
	os.Stderr = w
	doneC = make(chan struct{})
	go func() {
		io.Copy(&b, r)
		close(doneC)
	}()
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockDBAPI(mockCtrl)
	assert.PanicsWithValue("exitHook called", main)
	assert.Equal(1, exitCode)
	w.Close()
	<-doneC
	line, _ = b.ReadString('\n')
	assert.Regexp("./testbad.ini.*Not an INI file", line)
}
