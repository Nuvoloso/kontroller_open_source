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
	"syscall"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/cluster"
	clusterMock "github.com/Nuvoloso/kontroller/pkg/cluster/mock"
	"github.com/Nuvoloso/kontroller/pkg/mongodb"
	"github.com/Nuvoloso/kontroller/pkg/mongodb/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

var errUnknownError = errors.New("unknown error")

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

	t.Log("case: invalid INI file")
	app := &mainContext{log: l}
	var err error
	assert.NotPanics(func() { err = app.ParseArgs() })
	assert.Regexp("Not an INI file", err)
	assert.Zero(tl.CountPattern("."))

	t.Log("case: CLI parse failure exits, also INI file does not exist")
	Appname = "no-such-file"
	os.Args = []string{Appname, "-x"}
	app = &mainContext{log: l}
	r, w, _ := os.Pipe()
	os.Stderr = w
	outC := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outC <- buf.String()
	}()
	assert.PanicsWithValue("exitHook called", func() { app.ParseArgs() })
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
	app = &mainContext{log: l}
	r, w, _ = os.Pipe()
	os.Stdout = w
	outC = make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outC <- buf.String()
	}()
	assert.PanicsWithValue("exitHook called", func() { app.ParseArgs() })
	w.Close()
	out = <-outC
	assert.Regexp("Usage:", out)
	assert.Zero(exitCode)
	expCtx := &mainContext{
		MongoArgs: mongodb.Args{
			URL:           "mongodb://localhost",
			MaxPoolSize:   100,
			Timeout:       25 * time.Second,
			UseSSL:        true,
			SSLServerName: "localhost",
		},
		ClientTimeout: 25,
		log:           l,
		SSLSkipVerify: true,
		CACert:        "ca.crt",
		ClientCert:    "sidecar.crt",
		ClientKey:     "sidecar.key",
		LogLevel:      "INFO",
	}
	assert.Equal(expCtx, app)
	assert.Equal(1, tl.CountPattern("Read configuration defaults from ./testgood.ini"))
	tl.Flush()

	// no ini file, --version prints and exits
	b.Reset()
	Appname = "no-such-file"
	BuildJob = "kontroller:1"
	os.Args = []string{Appname, "--version"}
	app = &mainContext{log: l}
	r, w, _ = os.Pipe()
	os.Stdout = w
	outC = make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outC <- buf.String()
	}()
	assert.PanicsWithValue("exitHook called", func() { app.ParseArgs() })
	w.Close()
	out = <-outC
	assert.Zero(exitCode)
	assert.Regexp("Build ID:", out)
	tl.Flush()

	// write-config
	Appname = "no-such-file"
	outFile := "./test-cfg.ini"
	os.Args = []string{Appname, "--write-config=" + outFile}
	app = &mainContext{log: l}
	os.Remove(outFile)
	r, w, _ = os.Pipe()
	os.Stdout = w
	outC = make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outC <- buf.String()
	}()
	assert.PanicsWithValue("exitHook called", func() { app.ParseArgs() })
	w.Close()
	out = <-outC
	assert.Zero(exitCode)
	assert.Regexp("Wrote configuration to "+outFile, out)
	stat, err := os.Stat(outFile)
	assert.NoError(err)
	assert.NotZero(stat.Size)
	os.Remove(outFile)

	// success, no args
	os.Args = []string{Appname}
	app = &mainContext{log: l}
	r, w, _ = os.Pipe()
	os.Stdout = w
	outC = make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outC <- buf.String()
	}()
	assert.NoError(app.ParseArgs())
	w.Close()
	out = <-outC
	assert.Empty(out)
}

func TestSetupLogging(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	origStderr := os.Stderr
	exitCode := -1
	exitHook = func(code int) {
		exitCode = code
		panic("exitHook called")
	}
	defer func() {
		os.Stderr = origStderr
		exitHook = os.Exit
	}()

	t.Log("case: success")
	app := &mainContext{LogLevel: "DEBUG"}
	assert.NotPanics(app.SetupLogging)

	t.Log("case: invalid log level")
	var b bytes.Buffer
	app = &mainContext{LogLevel: "other"}
	r, w, _ := os.Pipe()
	os.Stderr = w
	doneC := make(chan struct{})
	go func() {
		io.Copy(&b, r)
		close(doneC)
	}()
	assert.PanicsWithValue("exitHook called", app.SetupLogging)
	assert.Equal(1, exitCode)
	w.Close()
	<-doneC
	line, _ := b.ReadString('\n')
	assert.Regexp("Invalid log level", line)
}

func TestInit(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	Appname = "test"

	defer func() {
		clusterClientHook = cluster.NewClient
		dbAPIHook = mongodb.NewDBAPI
		os.Unsetenv(EnvClusterDNSName)
		os.Unsetenv(EnvReplicaSetName)
		os.Unsetenv(cluster.NuvoEnvPodNamespace)
		os.Unsetenv(cluster.NuvoEnvPodIP)
		os.Unsetenv(cluster.NuvoEnvPodName)
	}()

	t.Log("case: success, required args and env")
	os.Setenv(EnvReplicaSetName, "rs")
	os.Setenv(cluster.NuvoEnvPodNamespace, "ns")
	os.Setenv(cluster.NuvoEnvPodIP, "1.2.3.4")
	os.Setenv(cluster.NuvoEnvPodName, "pod")
	app := &mainContext{
		MongoArgs: mongodb.Args{
			URL:          "mongodb://localhost:17",
			DatabaseName: "testDB",
		},
		CACert:        "ca.crt",
		ClientCert:    "client.crt",
		ClientKey:     "client.key",
		ClientTimeout: 13,
		log:           l,
		ops:           &fakeOps{},
	}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cc := clusterMock.NewMockClient(mockCtrl)
	var ccErr error
	clusterClientHook = func(kind string) (cluster.Client, error) {
		assert.Equal("kubernetes", kind)
		return cc, ccErr
	}
	cc.EXPECT().SetTimeout(13)
	api := mock.NewMockDBAPI(mockCtrl)
	dbAPIHook = func(args *mongodb.Args) mongodb.DBAPI {
		assert.True(args == &app.MongoArgs)
		assert.True(args.DirectConnect)
		assert.Equal("ca.crt", args.TLSCACertificate)
		assert.Equal("client.crt", args.TLSCertificate)
		assert.Equal("client.key", args.TLSCertificateKey)
		return api
	}
	api.EXPECT().Connect(nil).Return(nil)
	assert.NoError(app.Init())
	assert.Equal(DefaultClusterDNSName, app.ClusterDNSName)
	assert.Equal("ns", app.Namespace)
	assert.Equal("1.2.3.4", app.PodIP)
	assert.Equal("pod", app.PodName)
	assert.Equal("rs", app.ReplicaSetName)
	assert.Equal(Appname, app.MongoArgs.AppName)
	assert.Equal(l, app.MongoArgs.Log)
	assert.False(app.MongoArgs.UseSSL)
	assert.True(cc == app.clusterClient)
	assert.True(api == app.db)
	// terminate the signal handler go routine
	assert.NotNil(app.stopMonitor)
	for app.sigChan == nil {
		time.Sleep(10 * time.Millisecond)
	}
	api.EXPECT().Terminate()
	app.sigChan <- syscall.SIGTERM
	<-app.stopMonitor
	assert.Equal(1, tl.CountPattern("shutting down"))
	tl.Flush()
	mockCtrl.Finish()

	for _, envName := range []string{EnvReplicaSetName, cluster.NuvoEnvPodNamespace, cluster.NuvoEnvPodIP, cluster.NuvoEnvPodName} {
		t.Logf("case: %s not set", envName)
		app.clusterClient = nil
		app.db = nil
		app.Namespace, app.PodIP, app.PodName, app.ReplicaSetName = "", "", "", ""
		v := os.Getenv(envName)
		assert.NotEmpty(v)
		os.Unsetenv(envName)
		assert.Regexp("One or more of", app.Init())
		assert.Nil(app.clusterClient)
		assert.Nil(app.db)
		os.Setenv(envName, v)
		tl.Flush()
	}

	t.Log("case: Connect error, set CLUSTER_DNS_NAME")
	os.Setenv(EnvClusterDNSName, "a.b")
	app.stopMonitor = nil
	app.sigChan = nil
	mockCtrl = gomock.NewController(t)
	cc = clusterMock.NewMockClient(mockCtrl)
	cc.EXPECT().SetTimeout(13)
	api = mock.NewMockDBAPI(mockCtrl)
	api.EXPECT().Connect(nil).Return(errUnknownError)
	assert.Equal(errUnknownError, app.Init())
	assert.Equal("a.b", app.ClusterDNSName)
	assert.Nil(app.stopMonitor)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: clusterClient error")
	app.stopMonitor = nil
	app.sigChan = nil
	mockCtrl = gomock.NewController(t)
	cc = clusterMock.NewMockClient(mockCtrl)
	ccErr = errUnknownError
	assert.Equal(errUnknownError, app.Init())
	assert.Nil(app.stopMonitor)
}

func TestLogStart(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	app := &mainContext{log: l, ops: &fakeOps{}}
	Appname = "test"

	BuildID = "1"
	BuildTime = "Z"
	BuildJob = "kontroller:1"
	BuildHost = "host"

	// prefer job
	app.LogStart()
	assert.Equal("1 Z kontroller:1", app.ServiceVersion)
	assert.Equal(1, tl.CountPattern(`"ServiceVersion": "`+app.ServiceVersion))
	tl.Flush()

	// no job
	BuildID = "1"
	BuildTime = "Z"
	BuildJob = ""
	BuildHost = "host"
	app.LogStart()
	assert.Equal("1 Z host", app.ServiceVersion)
	assert.Equal(1, tl.CountPattern(`"ServiceVersion": "`+app.ServiceVersion))
}

func TestMain(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	exitCode := -1
	exitHook = func(code int) {
		exitCode = code
		panic("exitHook called")
	}
	defer func() {
		app.ops = app
		exitHook = os.Exit
	}()

	t.Log("case: success")
	assert.True(app.ops == app)
	ops := &fakeOps{}
	app.ops = ops
	assert.NotPanics(main)
	assert.Equal(2, ops.SetupLoggingCnt)
	assert.Equal(1, ops.ParseArgsCnt)
	assert.Equal(1, ops.InitCnt)
	assert.Equal(1, ops.LogStartCnt)
	assert.Equal(1, ops.CheckReplicaSetCnt)
	assert.Equal(1, ops.MonitorCnt)

	t.Log("case: ParseArgs fails")
	ops = &fakeOps{RetParseArgsErr: errUnknownError}
	app.ops = ops
	app.log = l
	assert.PanicsWithValue("exitHook called", main)
	assert.Equal(1, exitCode)
	assert.Equal(1, ops.SetupLoggingCnt)
	assert.Equal(1, ops.ParseArgsCnt)
	assert.Zero(ops.InitCnt)
	assert.Zero(ops.LogStartCnt)
	assert.Zero(ops.CheckReplicaSetCnt)
	assert.Zero(ops.MonitorCnt)
	assert.Equal(1, tl.CountPattern(errUnknownError.Error()))
	tl.Flush()

	t.Log("case: Init fails")
	ops = &fakeOps{RetInitErr: errUnknownError}
	app.ops = ops
	app.log = l
	assert.PanicsWithValue("exitHook called", main)
	assert.Equal(1, exitCode)
	assert.Equal(2, ops.SetupLoggingCnt)
	assert.Equal(1, ops.ParseArgsCnt)
	assert.Equal(1, ops.InitCnt)
	assert.Zero(ops.LogStartCnt)
	assert.Zero(ops.CheckReplicaSetCnt)
	assert.Zero(ops.MonitorCnt)
	assert.Equal(1, tl.CountPattern(errUnknownError.Error()))
	tl.Flush()

	t.Log("case: CheckReplicaSet fails")
	ops = &fakeOps{RetCheckReplicaSetErr: errUnknownError}
	app.ops = ops
	app.log = l
	assert.PanicsWithValue("exitHook called", main)
	assert.Equal(1, exitCode)
	assert.Equal(2, ops.SetupLoggingCnt)
	assert.Equal(1, ops.ParseArgsCnt)
	assert.Equal(1, ops.InitCnt)
	assert.Equal(1, ops.LogStartCnt)
	assert.Equal(1, ops.CheckReplicaSetCnt)
	assert.Zero(ops.MonitorCnt)
	assert.Equal(1, tl.CountPattern(errUnknownError.Error()))
}

type fakeOps struct {
	app *mainContext

	CheckReplicaSetCnt    int
	RetCheckReplicaSetErr error

	InitCnt    int
	RetInitErr error

	InitiateReplicaSetCnt     int
	InitiateReplicaSetFailCnt int
	RetInitiateReplicaSetErr  error

	LogStartCnt int

	MonitorCnt  int
	MonitorWait bool

	ParseArgsCnt    int
	RetParseArgsErr error

	SetupLoggingCnt int
}

func NewFakeOps(app *mainContext) Ops {
	return &fakeOps{app: app}
}

func (ops *fakeOps) CheckReplicaSet() error {
	ops.CheckReplicaSetCnt++
	return ops.RetCheckReplicaSetErr
}

func (ops *fakeOps) Init() error {
	ops.InitCnt++
	return ops.RetInitErr
}

func (ops *fakeOps) InitiateReplicaSet() (ret error) {
	ops.InitiateReplicaSetCnt++
	if ops.InitiateReplicaSetFailCnt > 0 {
		ops.InitiateReplicaSetFailCnt--
		ret = ops.RetInitiateReplicaSetErr
	} else {
		ops.RetInitiateReplicaSetErr = nil
	}
	return
}

func (ops *fakeOps) LogStart() {
	ops.LogStartCnt++
}

func (ops *fakeOps) Monitor() {
	ops.MonitorCnt++
	if ops.MonitorWait {
		<-ops.app.stopMonitor
	}
}

func (ops *fakeOps) ParseArgs() error {
	ops.ParseArgsCnt++
	return ops.RetParseArgsErr
}

func (ops *fakeOps) SetupLogging() {
	ops.SetupLoggingCnt++
}
