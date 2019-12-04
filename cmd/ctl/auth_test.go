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
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/mgmtclient"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-ini/ini"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestAuthMakeRecord(t *testing.T) {
	assert := assert.New(t)
	cmd := &authCmd{}
	now := time.Now()
	o := &authData{
		Host:           "localhost",
		Port:           443,
		AuthIdentifier: "me@nuvoloso.com",
		Token:          "fakeToken",
		Expiry:         &now,
	}
	rec := cmd.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(authHeaders))
	assert.EqualValues("localhost:443", rec[hManagementHost])
	assert.EqualValues(o.AuthIdentifier, rec[hAuthIdentifier])
	assert.EqualValues(o.Token, rec[hToken])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeExpires])

	// expired token and no port
	expired := time.Time{}
	o.Expiry = &expired
	o.Port = 0
	rec = cmd.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(authHeaders))
	assert.EqualValues("localhost", rec[hManagementHost])
	assert.EqualValues(o.AuthIdentifier, rec[hAuthIdentifier])
	assert.EqualValues(o.Token, rec[hToken])
	assert.Equal("expired", rec[hTimeExpires])

	// no expiry
	o.Expiry = nil
	o.Port = 8443
	rec = cmd.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(authHeaders))
	assert.EqualValues("localhost:8443", rec[hManagementHost])
	assert.EqualValues(o.AuthIdentifier, rec[hAuthIdentifier])
	assert.EqualValues(o.Token, rec[hToken])
	assert.Empty(rec[hTimeExpires])
}

func TestAuthList(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
		credFile = defCredFile
	}()
	interactiveReader = nil

	now := time.Now()
	authResp := &mgmtclient.AuthResp{
		Token:    "good",
		Expiry:   now.Unix(),
		Issuer:   "me",
		Username: "admin",
	}

	// list, all defaults, no credentials file
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetAuthToken("")
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	assert.Equal(Appname+"_CREDENTIALS_FILE", credFileEnv)
	homeDir, _ := os.LookupEnv(eHOME)
	assert.Regexp(homeDir+"/.nuvoloso/"+Appname+"-credentials.ini", defCredFile)
	assert.Equal(defCredFile, credFile)
	err := parseAndRun([]string{"auth", "list"})
	assert.NoError(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(authDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, 0)

	// invalid columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	err = parseAndRun([]string{"auth", "list", "-c", "Name,ID,Foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// success with list, columns
	r := strings.NewReader(goodCred)
	os.Remove("./testcred.ini")
	outFile, err := os.OpenFile("./testcred.ini", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, PrivatePerm)
	_, err = io.Copy(outFile, r)
	assert.NoError(err)
	outFile.Close()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps).MinTimes(1)
	authOps.EXPECT().SetAuthToken("good")
	authOps.EXPECT().SetAuthToken("")
	authOps.EXPECT().Validate(false).Return(authResp, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	credFile = "./testcred.ini"
	err = parseAndRun([]string{"auth", "list", "--all", "-c", "TimeExpires,Token"})
	assert.NoError(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, 2)
	t.Log(te.tableData)
	assert.Len(te.tableData, 2)

	// success, skip validation, yaml
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetAuthToken("")
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	credFile = "./testcred.ini"
	assert.NoError(parseAndRun([]string{"auth", "list", "-x", "-o", "yaml"}))

	// success, skip validation, json
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetAuthToken("")
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	credFile = "./testcred.ini"
	assert.NoError(parseAndRun([]string{"auth", "list", "--skip-validation", "-o", "json"}))

	// success, filter host
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetAuthToken("")
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	credFile = "./testcred.ini"
	assert.NoError(parseAndRun([]string{"auth", "list", "--host=noMatch", "-o", "json"}))
	assert.Empty(te.jsonData)

	// success, filter login
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetAuthToken("")
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	credFile = "./testcred.ini"
	assert.NoError(parseAndRun([]string{"auth", "list", "--host=", "--login=noMatch", "-o", "json"}))
	assert.Empty(te.jsonData)
	appCtx.LoginName = ""

	// success, filter port
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetAuthToken("")
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	credFile = "./testcred.ini"
	assert.NoError(parseAndRun([]string{"auth", "list", "--host=", "--port=12345", "-o", "json"}))
	assert.Empty(te.jsonData)

	// bad gateway
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps).MinTimes(1)
	authOps.EXPECT().SetAuthToken("good")
	authOps.EXPECT().Validate(false).Return(nil, errors.New("502 Bad Gateway"))
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	credFile = "./testcred.ini"
	err = parseAndRun([]string{"auth", "list", "-a"})
	if assert.Error(err) {
		assert.Equal("502 Bad Gateway", err.Error())
	}

	// readCredentials fails (bad permissions)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	credFile = "./testcred.ini"
	assert.NoError(os.Chmod(credFile, 0666))
	assert.Regexp("file permissions", parseAndRun([]string{"auth", "list"}).Error())

	// readCredentials no-op
	c := authCmd{}
	c.cfg = &ini.File{}
	assert.NoError(c.readCredentialsFile())
	os.Remove(credFile)

	// not 3-part section name
	r = strings.NewReader(badCred1)
	outFile, err = os.OpenFile("./testcred.ini", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, PrivatePerm)
	_, err = io.Copy(outFile, r)
	assert.NoError(err)
	outFile.Close()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	credFile = "./testcred.ini"
	assert.Regexp("invalid section", parseAndRun([]string{"auth", "list"}).Error())
	os.Remove(credFile)

	// port is not a number
	r = strings.NewReader(badCred2)
	outFile, err = os.OpenFile("./testcred.ini", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, PrivatePerm)
	_, err = io.Copy(outFile, r)
	assert.NoError(err)
	outFile.Close()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	credFile = "./testcred.ini"
	assert.Regexp("invalid section", parseAndRun([]string{"auth", "list"}).Error())
	os.Remove(credFile)

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	err = parseAndRun([]string{"auth", "list", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestAuthLogin(t *testing.T) {
	assert := assert.New(t)

	auth := &authLoginCmd{}
	now := time.Now()
	authResp := &mgmtclient.AuthResp{
		Token:    "good",
		Expiry:   now.Unix(),
		Issuer:   "me",
		Username: "foo",
	}

	// error with SocketPath
	appCtx.SocketPath = "/path"
	assert.Regexp("unix socket", auth.Execute(nil))
	appCtx.SocketPath = ""

	// error without Login
	assert.Regexp("required flag.*login", auth.Execute(nil))

	// error with no-login
	appCtx.NoLogin = true
	assert.Regexp("flag.*no-login", auth.Execute(nil))
	appCtx.NoLogin = false

	origPasswordHook := passwordHook
	savedParser := parser
	var hookCalled bool
	var hookRet []byte
	var hookErr error
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
		passwordHook = origPasswordHook
		credFile = defCredFile
		appCtx.LoginName = ""
	}()
	interactiveReader = nil
	passwordHook = func(int) ([]byte, error) {
		hookCalled = true
		return hookRet, hookErr
	}

	// no prompt when both login and password are specified
	te := &TestEmitter{}
	appCtx.Emitter = te
	appCtx.LoginName = authResp.Username
	appCtx.Host = "localhost"
	appCtx.Port = 12345
	auth.Password = "bar%(FLY)s"
	auth.cfg = nil
	auth.OutputFormat = "json"
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps).MinTimes(1)
	authOps.EXPECT().Authenticate(appCtx.LoginName, auth.Password).Return(authResp, nil)
	authOps.EXPECT().SetAuthToken("")
	appCtx.API = mAPI
	credFile = "./testCred.ini"
	os.Remove(credFile)
	assert.NoError(auth.Execute(nil))
	if assert.NotNil(auth.cfg) {
		sec, err := auth.cfg.GetSection("localhost,12345,foo")
		if assert.NoError(err) && assert.NotNil(sec) {
			assert.Equal(util.Obfuscate("bar%(FLY)s"), sec.Key("password").Value())
			assert.Equal(authResp.Token, sec.Key("token").Value())
		}
	}
	stat, err := os.Stat(credFile)
	assert.NoError(err)
	assert.Equal(PrivatePerm, stat.Mode())
	assert.False(hookCalled)
	appCtx.Host, appCtx.LoginName = "", ""

	// read password interactively, cover parse
	var b bytes.Buffer
	outputWriter = &b
	auth.Password = ""
	auth.OutputFormat = "json"
	hookRet = []byte{' ', 'a', 'x', ' '}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps).MinTimes(1)
	authOps.EXPECT().Authenticate("foo", "ax").Return(authResp, nil)
	authOps.EXPECT().SetAuthToken("")
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	credFile = "./testCred.ini"
	os.Remove(credFile)
	err = parseAndRun([]string{"auth", "login", "--login", "foo", "--port=12345"})
	assert.Equal("foo", appCtx.LoginName)
	assert.Equal("Password: \n", b.String())
	auth.cfg = nil
	assert.NoError(auth.readCredentialsFile())
	if assert.NotNil(auth.cfg) {
		sec, err := auth.cfg.GetSection("localhost,12345,foo")
		if assert.NoError(err) && assert.NotNil(sec) {
			assert.Equal(util.Obfuscate("ax"), sec.Key("password").Value())
			assert.Equal(authResp.Token, sec.Key("token").Value())
		}
	}
	assert.True(hookCalled)

	// invalid stored password, prompt
	b.Reset()
	auth.Password = ""
	auth.OutputFormat = "json"
	sec, _ := auth.cfg.GetSection("localhost,12345,foo")
	sec.Key("password").SetValue(util.Obfuscate("ax") + "/2")
	assert.NoError(auth.writeCredentialsFile())
	hookRet = []byte{' ', 'a', 'x', ' '}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps).MinTimes(1)
	authOps.EXPECT().Authenticate("foo", "ax").Return(authResp, nil)
	authOps.EXPECT().SetAuthToken("")
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	credFile = "./testCred.ini"
	err = parseAndRun([]string{"auth", "login", "--login", "foo", "--port=12345"})
	assert.Equal("foo", appCtx.LoginName)
	assert.Equal("Ignoring invalid stored password...\nPassword: \n", b.String())
	auth.cfg = nil
	assert.NoError(auth.readCredentialsFile())
	if assert.NotNil(auth.cfg) {
		sec, err := auth.cfg.GetSection("localhost,12345,foo")
		if assert.NoError(err) && assert.NotNil(sec) {
			assert.Equal(util.Obfuscate("ax"), sec.Key("password").Value())
			assert.Equal(authResp.Token, sec.Key("token").Value())
		}
	}
	assert.True(hookCalled)

	// no prompt with password in file, forget password
	b.Reset()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps).MinTimes(1)
	authOps.EXPECT().Authenticate("foo", "ax").Return(authResp, nil)
	authOps.EXPECT().SetAuthToken("")
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	credFile = "./testCred.ini"
	assert.NoError(parseAndRun([]string{"auth", "login", "-f", "--login", "foo", "--port=12345"}))
	auth.cfg = nil
	assert.NoError(auth.readCredentialsFile())
	if assert.NotNil(auth.cfg) {
		sec, err := auth.cfg.GetSection("localhost,12345,foo")
		if assert.NoError(err) && assert.NotNil(sec) {
			assert.Empty(sec.Key("password").Value())
			assert.Equal(authResp.Token, sec.Key("token").Value())
		}
	}

	// invalid columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	credFile = "./testCred.ini"
	err = parseAndRun([]string{"auth", "login", "-c", "Name,ID,Foo"})
	assert.Error(err)
	assert.Regexp("invalid column", err)

	// error reading password
	b.Reset()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("error reading password")
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	auth.Password = ""
	hookErr = errors.New("failure")
	assert.Equal(hookErr, auth.Execute(nil))
	assert.Empty(auth.Password)
	appCtx.LoginName = ""

	// read credentials fails
	assert.NoError(os.Chmod(credFile, 0666))
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	credFile = "./testCred.ini"
	err = parseAndRun([]string{"auth", "login", "--login=foo", "-c", "Token,TimeExpires"})
	assert.Error(err)
	assert.Regexp("file permissions", err)
	os.Remove(credFile)

	// authenticate error
	appCtx.LoginName = "foo"
	auth.Password = "ax"
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().Authenticate("foo", "ax").Return(nil, errors.New("auth error"))
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	err = auth.Execute(nil)
	assert.Error(err)
	assert.Regexp("auth error", err)

	// setToken fails
	appCtx.LoginName = "foo"
	auth.Password = "ax"
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().Authenticate("foo", "ax").Return(authResp, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	credFile = "/no/such/path.ini"
	err = auth.Execute(nil)
	assert.Error(err)
	assert.Regexp("no such file or directory", err)

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	err = parseAndRun([]string{"auth", "login", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestAuthLogout(t *testing.T) {
	assert := assert.New(t)

	auth := &authLogoutCmd{}
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
		credFile = defCredFile
		appCtx.LoginName = ""
	}()
	interactiveReader = nil

	// error without Login
	assert.Regexp("required flag.*login", auth.Execute(nil))

	// error with no-login
	appCtx.NoLogin = true
	assert.Regexp("flag.*no-login", auth.Execute(nil))
	appCtx.NoLogin = false

	// success, preserve password
	r := strings.NewReader(goodCred)
	os.Remove("./testcred.ini")
	outFile, err := os.OpenFile("./testcred.ini", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, PrivatePerm)
	_, err = io.Copy(outFile, r)
	assert.NoError(err)
	outFile.Close()
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	credFile = "./testcred.ini"
	assert.NoError(parseAndRun([]string{"auth", "logout", "-s", "--login=admin", "--host=localhost", "--port=443"}))
	auth.cfg = nil
	assert.NoError(auth.readCredentialsFile())
	if assert.NotNil(auth.cfg) {
		sec, err := auth.cfg.GetSection("localhost,443,admin")
		if assert.NoError(err) && assert.NotNil(sec) {
			assert.Equal("ad", sec.Key("password").Value())
			assert.Empty(sec.Key("token").Value())
		}
	}

	// forget password
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	credFile = "./testcred.ini"
	assert.NoError(parseAndRun([]string{"auth", "logout", "-f", "--login=admin", "--host=localhost", "--port=443"}))
	auth.cfg = nil
	assert.NoError(auth.readCredentialsFile())
	if assert.NotNil(auth.cfg) {
		sec, err := auth.cfg.GetSection("localhost,443,admin")
		assert.Error(err)
		assert.Nil(sec)
	}

	// no section
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	credFile = "./testcred.ini"
	assert.NoError(parseAndRun([]string{"auth", "logout", "--login=admin", "--host=localhost", "--port=443"}))
	auth.cfg = nil
	assert.NoError(auth.readCredentialsFile())
	if assert.NotNil(auth.cfg) {
		sec, err := auth.cfg.GetSection("localhost,443,admin")
		assert.Error(err)
		assert.Nil(sec)
	}

	// write error (file exists)
	auth.cfg = nil
	assert.NoError(auth.setToken("localhost", 443, "admin", "good"))
	assert.NoError(os.MkdirAll("./testcred.ini.new/sub", 0755))
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	credFile = "./testcred.ini"
	err = parseAndRun([]string{"auth", "logout", "--login=admin", "--host=localhost", "--port=443"})
	assert.Regexp("file exists", err)
	assert.NoError(os.RemoveAll("./testcred.ini.new"))

	// readCredentials fails (bad permissions)
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	credFile = "./testcred.ini"
	assert.NoError(os.Chmod(credFile, 0666))
	assert.Regexp("file permissions", parseAndRun([]string{"auth", "logout", "--login=admin", "--host=localhost", "--port=8443"}).Error())
	os.Remove(credFile)

	// unix socket
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	credFile = "./testcred.ini"
	assert.Regexp("not supported", parseAndRun([]string{"auth", "logout", "--socket-path=/foo/bar"}).Error())

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAuth()
	credFile = "./testcred.ini"
	err = parseAndRun([]string{"auth", "logout", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestGetSetToken(t *testing.T) {
	assert := assert.New(t)

	auth := &authCmd{}
	defer func() {
		credFile = defCredFile
		appCtx.LoginName = ""
	}()

	// getToken read fails
	credFile = "./testcred.ini"
	assert.NoError(os.MkdirAll(credFile, 0755))
	token, err := auth.getToken("", 0, "")
	assert.Empty(token)
	assert.Regexp("file permissions", err)

	// no read
	auth.cfg = ini.Empty()
	token, err = auth.getToken("", 0, "")
	assert.Empty(token)
	assert.NoError(err)

	// setToken read fails
	auth.cfg = nil
	assert.Regexp("file permissions", auth.setToken("", 0, "", ""))

	// setToken, no read, write fails on rename
	auth.cfg = ini.Empty()
	assert.Regexp("file exists", auth.setToken("host", 12345, "user", "token"))
	assert.NoError(os.RemoveAll(credFile))
	assert.NoError(os.Remove(credFile + ".new"))

	// validation errors
	auth.cfg = ini.Empty()
	assert.Regexp("invalid host", auth.setToken("", 0, "", ""))
	assert.Regexp("invalid authIdentifier", auth.setToken("host", 0, "", ""))
	assert.Regexp("invalid port", auth.setToken("host", 0, "user", ""))
	assert.Regexp("invalid token", auth.setToken("host", 12345, "user", ""))
}

var goodCred = "[localhost,8443,admin]\npassword = admin\n\n[localhost,443,admin]\npassword = ad\n token = good\n\n"
var badCred1 = "[localhost,443,admin,andMore]\npassword = admin\ntoken = bad\n\n"
var badCred2 = "[localhost,fourfourthree,admin]\npassword = admin\ntoken = bad\n\n"
