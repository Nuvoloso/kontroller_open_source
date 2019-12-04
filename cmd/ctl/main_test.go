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
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/common"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

type testCmd struct {
	called bool
}

func (c *testCmd) Execute(args []string) error {
	c.called = true
	return nil
}

func TestInvocation(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil // interactive authentication paths are tested separately

	// save some values
	origIniFileTemp := iniFileNameTemplate
	origHOME := os.Getenv(eHOME)
	defer func() {
		iniFileNameTemplate = origIniFileTemp
		os.Setenv(eHOME, origHOME)
		os.Setenv(iniFileEnv, "")
	}()
	curDir, err := os.Getwd()
	assert.Nil(err)

	// invalid command, default ini file non-existent
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	te := &TestEmitter{}
	appCtx.Emitter = te
	iniFileNameTemplate = "./%s-%s.ini"
	err = os.Setenv(eHOME, curDir)
	assert.Nil(err)
	err = parseAndRun([]string{"foo-command"})
	assert.NotNil(err)
	assert.Regexp("foo-command", err.Error())
	assert.Equal(fmt.Sprintf(iniFileNameTemplate, curDir, Appname), defIniFile)
	assert.Equal(iniFile, defIniFile)

	// invalid command, no HOME env, ini file non-existent
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	te = &TestEmitter{}
	appCtx.Emitter = te
	err = os.Unsetenv(eHOME)
	assert.Nil(err)
	err = parseAndRun([]string{"foo-command"})
	assert.NotNil(err)
	assert.Regexp("foo-command", err.Error())
	assert.Equal(fmt.Sprintf(iniFileNameTemplate, "/", Appname), defIniFile)
	assert.Equal(iniFile, defIniFile)

	// invalid command, non-existent ini file from environment
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	te = &TestEmitter{}
	appCtx.Emitter = te
	os.Setenv(iniFileEnv, "fooBar")
	err = parseAndRun([]string{"foo-command"})
	assert.NotNil(err)
	assert.Regexp("foo-command", err.Error())
	assert.Equal(iniFile, "fooBar")

	// invalid command, bad ini file from environment
	te = &TestEmitter{}
	appCtx.Emitter = te
	os.Setenv(iniFileEnv, "./testbad.ini")
	err = parseAndRun([]string{"foo-command"})
	assert.NotNil(err)
	assert.Regexp("INI file error", err.Error())
	assert.Equal(iniFile, "./testbad.ini")

	// test command, good ini file from environment
	t.Log("TC: good INI")
	appCtx.Host = ""
	appCtx.Port = 0
	appCtx.SocketPath = ""
	appCtx.hostArg = false
	appCtx.portArg = false
	appCtx.socketArg = false
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	tCmd := &testCmd{}
	parser.AddCommand("testCommand", "testCommand short", "testCommand long", tCmd)
	te = &TestEmitter{}
	appCtx.Emitter = te
	os.Setenv(iniFileEnv, "./testhttps.ini")
	err = parseAndRun([]string{"testCommand"})
	assert.Nil(err)
	assert.Equal(iniFile, "./testhttps.ini")
	assert.NotNil(appCtx.API)
	assert.True(tCmd.called)
	assert.False(appCtx.hostArg)
	assert.False(appCtx.portArg)
	assert.False(appCtx.socketArg)
	assert.Equal("testhost", appCtx.Host)                     // from INI
	assert.Equal(6000, appCtx.Port)                           // from INI
	assert.Equal("/var/run/nuvoloso/sock", appCtx.SocketPath) // from INI

	// test command, good ini, host port arg override, --no-login avoids prompt
	t.Log("TC: host:port overrides INI socket path")
	appCtx.API = nil
	appCtx.Host = ""
	appCtx.Port = 0
	appCtx.SocketPath = ""
	appCtx.hostArg = false
	appCtx.portArg = false
	appCtx.socketArg = false
	interactiveReader = os.Stdin
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	tCmd = &testCmd{}
	parser.AddCommand("testCommand", "testCommand short", "testCommand long", tCmd)
	te = &TestEmitter{}
	appCtx.Emitter = te
	os.Setenv(iniFileEnv, "./testhttps.ini")
	err = parseAndRun([]string{"testCommand", "--no-login", "--host", "H", "--port", "1"})
	assert.Nil(err)
	assert.Equal(iniFile, "./testhttps.ini")
	assert.NotNil(appCtx.API)
	assert.True(tCmd.called)
	assert.True(appCtx.hostArg)
	assert.True(appCtx.portArg)
	assert.False(appCtx.socketArg)
	assert.True(appCtx.NoLogin)
	assert.Equal("H", appCtx.Host)      // from cmd line
	assert.Equal(1, appCtx.Port)        // from cmd line
	assert.Equal("", appCtx.SocketPath) // INI value zapped
	interactiveReader = nil

	// test command, good ini, socket path arg, login not used
	t.Log("TC: socket path on command line")
	appCtx.API = nil
	appCtx.Host = ""
	appCtx.Port = 0
	appCtx.SocketPath = ""
	appCtx.LoginName = ""
	appCtx.hostArg = false
	appCtx.portArg = false
	appCtx.socketArg = false
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	tCmd = &testCmd{}
	parser.AddCommand("testCommand", "testCommand short", "testCommand long", tCmd)
	te = &TestEmitter{}
	appCtx.Emitter = te
	os.Setenv(iniFileEnv, "./testhttps.ini")
	err = parseAndRun([]string{"--login", "foo", "testCommand", "--socket-path", "sockpath"})
	assert.Nil(err)
	assert.Equal(iniFile, "./testhttps.ini")
	assert.NotNil(appCtx.API)
	assert.True(tCmd.called)
	assert.False(appCtx.hostArg)
	assert.False(appCtx.portArg)
	assert.True(appCtx.socketArg)
	assert.Equal("foo", appCtx.LoginName)
	assert.Equal("testhost", appCtx.Host)       // from INI
	assert.Equal(6000, appCtx.Port)             // from INI
	assert.Equal("sockpath", appCtx.SocketPath) // from cmd line
	appCtx.LoginName = ""

	// test subcommand invocation (parent, sub)
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	tCmd = &testCmd{}
	cmd, err := parser.AddCommand("testCommand", "testCommand short", "testCommand long", tCmd)
	assert.Nil(err)
	assert.NotNil(cmd)
	subCmd := &testCmd{}
	cmd.AddCommand("subCommand", "subCommand short", "subCommand long", subCmd)
	te = &TestEmitter{}
	appCtx.Emitter = te
	os.Setenv(iniFileEnv, "./testhttps.ini")
	err = parseAndRun([]string{"testCommand", "subCommand"})
	assert.Nil(err)
	assert.Equal(iniFile, "./testhttps.ini")
	assert.NotNil(appCtx.API)
	assert.True(subCmd.called)

	// test subcommand invocation (sub, parent)
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	tCmd = &testCmd{}
	cmd, err = parser.AddCommand("testCommand", "testCommand short", "testCommand long", tCmd)
	assert.Nil(err)
	assert.NotNil(cmd)
	subCmd = &testCmd{}
	cmd.AddCommand("subCommand", "subCommand short", "subCommand long", subCmd)
	te = &TestEmitter{}
	appCtx.Emitter = te
	os.Setenv(iniFileEnv, "./testhttps.ini")
	err = parseAndRun([]string{"subCommand", "testCommand"})
	assert.Nil(err)
	assert.Equal(iniFile, "./testhttps.ini")
	assert.NotNil(appCtx.API)
	assert.True(subCmd.called)

	// test invocation - no command
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	tCmd = &testCmd{}
	parser.AddCommand("testCommand", "testCommand short", "testCommand long", tCmd)
	te = &TestEmitter{}
	appCtx.Emitter = te
	os.Setenv(iniFileEnv, "./testhttps.ini")
	err = parseAndRun([]string{})
	assert.NotNil(err)
	assert.Regexp("possible to specify the action before the object", err.Error())

	// commandHandler low-level test
	err = commandHandler(nil, []string{})
	assert.Nil(err)

	// main itself
	origArgs := os.Args
	origStderr := os.Stderr
	var exitCode int
	exitHook = func(code int) {
		exitCode = code
	}
	defer func() {
		os.Args = origArgs
		os.Stderr = origStderr
		outputWriter = os.Stdout
		exitHook = os.Exit
	}()

	var b bytes.Buffer
	outputWriter = &b
	r, w, _ := os.Pipe()
	os.Stderr = w
	os.Args = []string{"nvctl", "version"}
	outC := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outC <- buf.String()
	}()
	main()
	w.Close()
	out := <-outC
	assert.Zero(exitCode)
	assert.Empty(out)
	assert.Regexp("Build ID", b.String())

	b.Reset()
	r, w, _ = os.Pipe()
	os.Stderr = w
	os.Args = []string{"nvctl", "error"}
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outC <- buf.String()
	}()
	main()
	w.Close()
	out = <-outC
	os.Stderr = origStderr
	assert.Equal(1, exitCode)
	assert.Regexp("Unknown command", out)
	assert.Empty(b.String())

	// invoke --help
	b.Reset()
	_, w, _ = os.Pipe()
	os.Stderr = w
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	tCmd = &testCmd{}
	parser.AddCommand("testCommand", "testCommand short", "testCommand long", tCmd)
	te = &TestEmitter{}
	appCtx.Emitter = te
	os.Setenv(iniFileEnv, "./testhttps.ini")
	err = parseAndRun([]string{"--help"})
	assert.NoError(err)
	assert.Regexp("^Usage:", b.String())
}

func TestMakeAuthHeaders(t *testing.T) {
	assert := assert.New(t)

	// no headers
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().GetAuthToken().Return("")
	appCtx.API = mAPI
	req := appCtx.MakeAuthHeaders()
	assert.NotNil(req)
	assert.Empty(req)

	// with headers
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	appCtx.AccountID = "aid1"
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().GetAuthToken().Return("Token")
	appCtx.API = mAPI
	req = appCtx.MakeAuthHeaders()
	assert.Len(req, 2)
	assert.Equal("Token", req.Get(common.AuthHeader))
	assert.Equal("aid1", req.Get(common.AccountHeader))
	appCtx.AccountID = ""
}

func TestInitContextAccount(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		outputWriter = os.Stdout
		parser = savedParser
	}()

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("loginName", "someAccount").Return("1234", nil)
	appCtx.API = mAPI
	appCtx.LoginName, appCtx.Account = "loginName", "someAccount"
	err := appCtx.InitContextAccount()
	assert.NoError(err)
	assert.Equal("1234", appCtx.AccountID)

	// 2nd call is a no-op
	err = appCtx.InitContextAccount()
	assert.NoError(err)
	assert.Equal("1234", appCtx.AccountID)

	// failure
	appCtx.AccountID = ""
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("loginName", "someAccount").Return("", fmt.Errorf("fail"))
	err = appCtx.InitContextAccount()
	assert.Equal("fail", err.Error())
	assert.Empty(appCtx.AccountID)
	appCtx.LoginName, appCtx.Account, appCtx.AccountID = "", "", ""
}

func TestPersistToken(t *testing.T) {
	assert := assert.New(t)
	defer func() {
		appCtx.LoginName = ""
		appCtx.token = ""
		credFile = defCredFile
	}()
	credFile = "./testcred.ini"
	os.Remove(credFile)

	// no-op if token empty initially
	appCtx.token = ""
	assert.NoError(appCtx.PersistToken())

	// token not saved if final token is empty
	appCtx.token = "set"
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().GetAuthToken().Return("")
	appCtx.API = mAPI
	assert.NoError(appCtx.PersistToken())
	_, err := os.Stat(credFile)
	assert.True(os.IsNotExist(err))

	// token not saved if final token is unchanged
	appCtx.token = "set"
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().GetAuthToken().Return("set")
	appCtx.API = mAPI
	assert.NoError(appCtx.PersistToken())
	_, err = os.Stat(credFile)
	assert.True(os.IsNotExist(err))

	// successful save
	appCtx.Host, appCtx.Port, appCtx.LoginName = "localhost", 12345, "me"
	appCtx.token = "set"
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().GetAuthToken().Return("reset")
	appCtx.API = mAPI
	assert.NoError(appCtx.PersistToken())
	auth := &authCmd{}
	assert.NoError(auth.readCredentialsFile())
	if assert.NotNil(auth.cfg) {
		sec, err := auth.cfg.GetSection("localhost,12345,me")
		if assert.NoError(err) && assert.NotNil(sec) {
			assert.Equal("reset", sec.Key("token").Value())
		}
	}

	// save fails
	assert.NoError(os.MkdirAll("./testcred.ini.new/sub", 0755))
	appCtx.Host, appCtx.Port, appCtx.LoginName = "localhost", 12345, "me"
	appCtx.token = "set"
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().GetAuthToken().Return("reset")
	appCtx.API = mAPI
	err = appCtx.PersistToken()
	assert.Regexp("file exists", err)
	assert.NoError(os.RemoveAll("./testcred.ini.new"))
	assert.NoError(os.Remove(credFile))
}

func TestVersionCmd(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		outputWriter = os.Stdout
		parser = savedParser
	}()

	tBuildID := "testBuildID"
	tBuildTime := "testBuildTime"
	tBuildHost := "testBuildHost"
	tBuildJob := "testBuildJob:1"
	BuildID = tBuildID
	BuildTime = tBuildTime
	BuildHost = tBuildHost
	BuildJob = tBuildJob

	// version command (default)
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	te := &TestEmitter{}
	appCtx.Emitter = te
	err := parseAndRun([]string{"version"})
	assert.Nil(err)
	t.Log(te.tableData)
	assert.Equal([][]string{{tBuildID, tBuildTime, tBuildJob, tBuildHost}}, te.tableData)

	// version command (table), accept login but do not use it
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	te = &TestEmitter{}
	appCtx.Emitter = te
	err = parseAndRun([]string{"version", "-o", "table", "--login", "login"})
	assert.Nil(err)
	assert.Equal("login", appCtx.LoginName)
	t.Log(te.tableData)
	assert.Equal([][]string{{tBuildID, tBuildTime, tBuildJob, tBuildHost}}, te.tableData)

	// version command (json), accept long login but do not use it
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	te = &TestEmitter{}
	appCtx.Emitter = te
	appCtx.LoginName = ""
	obj := struct{ BuildID, BuildTime, BuildJob, BuildHost string }{
		tBuildID, tBuildTime, tBuildJob, tBuildHost,
	}
	err = parseAndRun([]string{"--login", "login", "version", "-o", "json"})
	assert.Nil(err)
	assert.Equal("login", appCtx.LoginName)
	t.Log(te.tableData)
	assert.EqualValues(obj, te.jsonData)
	appCtx.LoginName = ""

	// version command (yaml)
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	te = &TestEmitter{}
	appCtx.Emitter = te
	err = parseAndRun([]string{"version", "-o", "yaml"})
	assert.Nil(err)
	t.Log(te.tableData)
	assert.EqualValues(obj, te.yamlData)

	// version command (unknown flag)
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	te = &TestEmitter{}
	appCtx.Emitter = te
	err = parseAndRun([]string{"version", "--foo-flag"})
	assert.NotNil(err)
	assert.Regexp("foo-flag", err.Error())
}

func TestHelpCmd(t *testing.T) {
	assert := assert.New(t)
	// Note: once help is called the state of the parser gets "frozen".
	// Hence do not use the main parser for these tests.
	savedParser := parser
	defer func() {
		outputWriter = os.Stdout
		parser = savedParser
	}()

	te := &TestEmitter{}
	appCtx.Emitter = te

	// help command (top level)
	var b bytes.Buffer
	outputWriter = &b
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	err := parseAndRun([]string{"help"})
	assert.Nil(err)
	t.Log(b.String())
	assert.True(b.Len() > 0)
	assert.Regexp("management command line interface", b.String())

	// invalid command
	b = bytes.Buffer{}
	outputWriter = &b
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	err = parseAndRun([]string{"help", "fooCmd"})
	assert.Nil(err)
	t.Log(b.String())
	assert.True(b.Len() > 0)
	assert.Regexp("management command line interface", b.String())

	// help command (version)
	b = bytes.Buffer{}
	outputWriter = &b
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	err = parseAndRun([]string{"help", "version"})
	assert.Nil(err)
	t.Log(b.String())
	assert.True(b.Len() > 0)
	assert.Regexp("build version information", b.String())

	// help sub command (faked)
	b = bytes.Buffer{}
	outputWriter = &b
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	tCmd1 := &testCmd{}
	cmd, err := parser.AddCommand("tCmd1", "testCommand1 short", "testCommand1 long", tCmd1)
	assert.Nil(err)
	tCmd2 := &testCmd{}
	_, err = cmd.AddCommand("tCmd2", "testCommand2 short", "testCommand2 long", tCmd2)
	assert.Nil(err)
	err = parseAndRun([]string{"help", "tCmd1", "tCmd2"})
	assert.Nil(err)
	t.Log(b.String())
	assert.True(b.Len() > 0)
	assert.Regexp("testCommand2 long", b.String())

	// help sub command (invalid)
	b = bytes.Buffer{}
	outputWriter = &b
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	tCmd1 = &testCmd{}
	cmd, err = parser.AddCommand("tCmd1", "testCommand1 short", "testCommand1 long", tCmd1)
	assert.Nil(err)
	tCmd2 = &testCmd{}
	_, err = cmd.AddCommand("tCmd2", "testCommand2 short", "testCommand2 long", tCmd2)
	assert.Nil(err)
	err = parseAndRun([]string{"help", "tCmd1", "tCmd3"})
	assert.Nil(err)
	t.Log(b.String())
	assert.True(b.Len() > 0)
	assert.Regexp("tCmd1 tCmd3.*not found", b.String())
}

func TestWhoAmICmd(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		outputWriter = os.Stdout
		parser = savedParser
	}()

	tcs := []string{"whoami", "whoami-A", "id", "id--login", "no-account", "no-login", "no-id", "internal"}
	for _, tc := range tcs {
		appCtx.Account = "account"
		appCtx.LoginName = "loginName"
		appCtx.ClientCert = ""
		appCtx.ClientKey = ""
		var b bytes.Buffer
		outputWriter = &b
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		cmdArgs := []string{"whoami"}
		var expErr error
		expOutput := "LoginName: loginName\n" + "  Account: account\n"
		t.Log("case:", tc)
		switch tc {
		case "whoami":
		case "whoami-A":
			cmdArgs = []string{"whoami", "-A", "account2"}
			expOutput = "LoginName: loginName\n" + "  Account: account2\n"
		case "id":
			cmdArgs = []string{"id"}
		case "id--login":
			cmdArgs = []string{"id", "--login", "userName"}
			expOutput = "LoginName: userName\n" + "  Account: account\n"
		case "no-account":
			appCtx.Account = ""
			expOutput = "LoginName: loginName\n" + "  Account: \n"
			expErr = fmt.Errorf("Specify Account in the [Application Options] section of the config file or the -A flag")
		case "no-login":
			appCtx.LoginName = ""
			expOutput = "LoginName: \n" + "  Account: account\n"
			expErr = fmt.Errorf("Specify LoginName in the [Application Options] section of the config file or the --login flag")
		case "no-id":
			appCtx.Account = ""
			appCtx.LoginName = ""
			expOutput = "LoginName: \n" + "  Account: \n"
			expErr = fmt.Errorf("Specify Account and LoginName in the [Application Options] section of the config file or use the -A and --login flags")
		case "internal":
			appCtx.Account = ""
			appCtx.LoginName = ""
			appCtx.ClientCert = "something"
			appCtx.ClientKey = "something"
			expOutput = "Internal user\n"
		default:
			assert.Equal("", tc)
			continue
		}
		err := parseAndRun(cmdArgs)
		assert.Equal(expErr, err, "case: %s", tc)
		assert.Equal(expOutput, b.String(), "case: %s", tc)
	}
}

func TestFlagOverrides(t *testing.T) {
	assert := assert.New(t)
	defer func() { appCtx = &AppCtx{} }() // restore

	// detection
	appCtx = &AppCtx{}
	args := []string{}
	checkArgsForSpecialFlagOverrides(args)
	assert.False(appCtx.hostArg)
	assert.False(appCtx.portArg)
	assert.False(appCtx.socketArg)
	assert.False(appCtx.noLoginArg)
	assert.False(appCtx.loginNameArg)

	appCtx = &AppCtx{}
	args = []string{"--host", "h", "--port", "2", "--socket-path", "path", "--no-login", "true", "--login", "login"}
	checkArgsForSpecialFlagOverrides(args)
	assert.True(appCtx.hostArg)
	assert.True(appCtx.portArg)
	assert.True(appCtx.socketArg)
	assert.True(appCtx.noLoginArg)
	assert.True(appCtx.loginNameArg)

	// application

	// socketPath override
	spTCs := []struct {
		hA, pA, sA              bool
		valueBefore, valueAfter string
	}{
		{false, false, false, "foo", "foo"},
		{true, false, false, "foo", ""},
		{false, true, false, "foo", ""},
		{true, true, false, "foo", ""},
		{false, false, true, "foo", "foo"},
		{true, false, true, "foo", "foo"},
		{false, true, true, "foo", "foo"},
		{true, true, true, "foo", "foo"},
	}
	for i, tc := range spTCs {
		appCtx = &AppCtx{
			hostArg:    tc.hA,
			portArg:    tc.pA,
			socketArg:  tc.sA,
			SocketPath: tc.valueBefore,
		}
		appCtx.processFlagOverrides()
		assert.Equal(tc.valueAfter, appCtx.SocketPath, "TC %d", i)
	}

	// auth overrides
	authTCs := []struct {
		lnA, nlA, nl            bool
		valueBefore, valueAfter string
	}{
		{false, false, false, "foo", "foo"},
		{true, false, false, "foo", "foo"},
		{false, true, false, "foo", ""},
		{true, true, false, "foo", "foo"},
		{false, false, true, "foo", ""},
		{true, false, true, "foo", "foo"},
		{false, true, true, "foo", ""},
		{true, true, true, "foo", "foo"},
	}
	for i, tc := range authTCs {
		appCtx = &AppCtx{
			loginNameArg: tc.lnA,
			noLoginArg:   tc.nlA,
			NoLogin:      tc.nl,
			LoginName:    tc.valueBefore,
		}
		appCtx.processFlagOverrides()
		assert.Equal(tc.valueAfter, appCtx.LoginName, "TC %d", i)
	}
}
