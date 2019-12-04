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


package centrald

import (
	"errors"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	fsl "github.com/Nuvoloso/kontroller/pkg/util/fake"
	flags "github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

type FakeDataStore struct {
	startCalled bool
	stopCalled  bool
	startErr    error
}

var _ = DataStore(&FakeDataStore{})

func (fds *FakeDataStore) Start() error {
	fds.startCalled = true
	return fds.startErr
}

func (fds *FakeDataStore) Stop() {
	fds.stopCalled = true
}

func (fds *FakeDataStore) OpsAccount() AccountOps {
	return nil
}

func (fds *FakeDataStore) OpsApplicationGroup() ApplicationGroupOps {
	return nil
}

func (fds *FakeDataStore) OpsCluster() ClusterOps {
	return nil
}

func (fds *FakeDataStore) OpsConsistencyGroup() ConsistencyGroupOps {
	return nil
}

func (fds *FakeDataStore) OpsCspCredential() CspCredentialOps {
	return nil
}

func (fds *FakeDataStore) OpsCspDomain() CspDomainOps {
	return nil
}

func (fds *FakeDataStore) OpsNode() NodeOps {
	return nil
}

func (fds *FakeDataStore) OpsPool() PoolOps {
	return nil
}

func (fds *FakeDataStore) OpsProtectionDomain() ProtectionDomainOps {
	return nil
}

func (fds *FakeDataStore) OpsRole() RoleOps {
	return nil
}

func (fds *FakeDataStore) OpsServicePlan() ServicePlanOps {
	return nil
}

func (fds *FakeDataStore) OpsServicePlanAllocation() ServicePlanAllocationOps {
	return nil
}

func (fds *FakeDataStore) OpsSnapshot() SnapshotOps {
	return nil
}

func (fds *FakeDataStore) OpsStorage() StorageOps {
	return nil
}

func (fds *FakeDataStore) OpsStorageRequest() StorageRequestOps {
	return nil
}

func (fds *FakeDataStore) OpsSystem() SystemOps {
	return nil
}

func (fds *FakeDataStore) OpsUser() UserOps {
	return nil
}

func (fds *FakeDataStore) OpsVolumeSeries() VolumeSeriesOps {
	return nil
}

func (fds *FakeDataStore) OpsVolumeSeriesRequest() VolumeSeriesRequestOps {
	return nil
}

func NewFakeDataStore() *FakeDataStore {
	return &FakeDataStore{}
}

type FakeComponent struct {
	t           *testing.T
	app         *AppCtx
	ds          *FakeDataStore
	initCalled  bool
	startCalled bool
	stopCalled  bool
}

var _ = AppComponent(&FakeComponent{})

func (c *FakeComponent) Init(app *AppCtx) {
	c.app = app
	c.initCalled = true
}

func (c *FakeComponent) Start() {
	assert.True(c.t, c.ds.startCalled)
	c.startCalled = true
}

func (c *FakeComponent) Stop() {
	assert.False(c.t, c.ds.stopCalled)
	c.stopCalled = true
}

type FakeInternal struct {
	retGcaArgs *mgmtclient.APIArgs
	retGcaErr  error
}

func (fi *FakeInternal) getClientArgs() (*mgmtclient.APIArgs, error) {
	return fi.retGcaArgs, fi.retGcaErr
}

var _ = appInternal(&FakeInternal{})

func TestAppFlags(t *testing.T) {
	assert := assert.New(t)

	args := AppArgs{}
	parser := flags.NewParser(&args, flags.Default&^flags.PrintErrors)
	_, err := parser.ParseArgs([]string{})
	assert.NoError(err)

	// check default values
	assert.EqualValues("", args.AgentdCertPath)
	assert.EqualValues("", args.AgentdKeyPath)
	assert.EqualValues("", args.ClusterdCertPath)
	assert.EqualValues("", args.ClusterdKeyPath)
	assert.EqualValues("", args.ImagePullSecretPath)
	assert.Equal("407798037446.dkr.ecr.us-west-2.amazonaws.com/nuvoloso", args.ImagePath)
	assert.Equal("v1", args.ClusterDeployTag)
	assert.Equal(20, args.ClientTimeout)
	assert.Equal(20, args.CSPTimeout)
	assert.False(args.DebugClient)
	assert.False(args.DebugREI)
	assert.EqualValues("/var/run/nuvoloso/rei", args.DebugREIPath)
	assert.EqualValues("csi", args.DriverType)
	assert.Equal(30, args.HeartbeatPeriod)
	assert.Equal(60, args.TaskPurgeDelay)
	assert.Equal(30*time.Minute, args.VersionLogPeriod)
	assert.Equal(5*time.Minute, args.ClusterTimeoutCheckPeriod)
	assert.Equal(2, args.ClusterTimeoutAfterMisses)
	assert.Equal("kubernetes", args.ClusterType)
	assert.Equal("nuvoloso-management", args.ManagementServiceNamespace)
	assert.Equal("nuvo-https", args.ManagementServiceName)
}

func TestAppCtx(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	ds := NewFakeDataStore()
	c1 := FakeComponent{t: t, ds: ds}
	AppRegisterComponent(&c1)
	c2 := FakeComponent{t: t, ds: ds}
	AppRegisterComponent(&c2)

	assert.False(rei.Enabled)

	// test PSO args
	for _, reiEnabled := range []bool{true, false} {
		app := &AppCtx{}
		app.Log = tl.Logger()
		app.DebugREI = reiEnabled
		assert.True(app.getPSOArgs().Validate())
	}

	// full success case
	fakeServer := &restapi.Server{
		Host: "host",
		Port: 8080,
	}
	evM := fev.NewFakeEventManager()

	app := AppInit(&AppArgs{DS: ds, Server: fakeServer, DebugREI: true, Log: tl.Logger(), CrudeOps: evM})
	assert.NotNil(app.Service)
	assert.Equal(ds, app.DS)
	assert.True(c1.initCalled)
	assert.Equal(app, c1.app)
	assert.True(c2.initCalled)
	assert.Equal(app, c2.app)
	assert.Equal(app, app.appInt)
	assert.Equal(app, app.AppCSP)
	assert.Equal(app, app.SFC)
	assert.NotNil(app.MetricMover)
	assert.NotNil(app.TaskScheduler)
	assert.True(rei.Enabled)
	assert.NotNil(app.StackLogger)
	assert.NotNil(app.PSO)
	assert.NotNil(app.MCDeployer)
	sl := &fsl.StackLogger{}
	app.StackLogger = sl

	err := app.Start()
	assert.Nil(err)
	assert.NotNil(app.ClientAPI)
	assert.True(ds.startCalled)
	assert.True(c1.startCalled)
	assert.True(c2.startCalled)
	assert.Equal(1, sl.StartCnt)

	app.Stop()
	assert.True(ds.stopCalled)
	assert.True(c1.stopCalled)
	assert.True(c2.stopCalled)
	assert.True(evM.CalledTAW)
	assert.Equal(1, sl.StopCnt)

	// success with just http enabled, just cover getClientArgs()
	app.Server = &restapi.Server{
		Host:             "host",
		Port:             8080,
		SocketPath:       "/sock/path",
		EnabledListeners: []string{"http"},
	}
	apiArgs, err := app.getClientArgs()
	assert.NoError(err)
	assert.Equal(app.Server.Host, apiArgs.Host)
	assert.Equal(app.Server.Port, apiArgs.Port)
	assert.Empty(apiArgs.SocketPath)

	// success with unix enabled (ignores host and port)
	app.Server = &restapi.Server{
		Host:             "host",
		SocketPath:       "/sock/path",
		EnabledListeners: []string{"unix"},
	}
	apiArgs, err = app.getClientArgs()
	assert.NoError(err)
	assert.Empty(apiArgs.Host)
	assert.Equal(string(app.Server.SocketPath), apiArgs.SocketPath)

	// failure cases
	rei.Enabled = false
	app = AppInit(&AppArgs{DS: ds})
	assert.False(rei.Enabled)
	app.StackLogger = sl
	err = app.Start()
	assert.NotNil(err)
	assert.Regexp("local client bindings not specified", err)
	assert.Equal(1, sl.StartCnt)

	app = AppInit(&AppArgs{DS: ds, Server: &restapi.Server{}})
	app.StackLogger = sl
	err = app.Start()
	assert.NotNil(err)
	assert.Regexp("local client bindings not specified", err)
	assert.Equal(1, sl.StartCnt)

	app = AppInit(&AppArgs{DS: ds, Server: &restapi.Server{Host: "host"}})
	app.StackLogger = sl
	err = app.Start()
	assert.NotNil(err)
	assert.Regexp("local client bindings not specified", err)
	assert.Equal(1, sl.StartCnt)

	app = AppInit(&AppArgs{DS: ds, Server: &restapi.Server{Host: "host", Port: 80, EnabledListeners: []string{"https"}}})
	app.StackLogger = sl
	err = app.Start()
	assert.NotNil(err)
	assert.Regexp("local client bindings not specified", err)
	assert.Equal(1, sl.StartCnt)

	app = AppInit(&AppArgs{DS: ds, Server: &restapi.Server{SocketPath: "/socket/path"}})
	app.StackLogger = sl
	err = app.Start()
	assert.NotNil(err)
	assert.Regexp("local client bindings not specified", err)
	assert.Equal(1, sl.StartCnt)

	app = AppInit(&AppArgs{DS: ds, Server: fakeServer, Log: tl.Logger(), CrudeOps: evM})
	app.StackLogger = sl
	ds.startErr = errors.New("DS start error")
	err = app.Start()
	assert.NotNil(err)
	assert.Regexp("failed to start data store: DS start error", err)
	assert.Equal(1, sl.StartCnt)
	ds.startErr = nil

	// force NewAPI to fail with fake client args
	app = AppInit(&AppArgs{DS: ds, Log: tl.Logger(), Server: &restapi.Server{Host: "host", Port: 8080, SocketPath: "/sock/path", EnabledListeners: []string{"http"}}})
	app.appInt = &FakeInternal{
		retGcaArgs: &mgmtclient.APIArgs{},
	}
	app.StackLogger = sl
	err = app.Start()
	assert.NotNil(err)
	assert.Regexp("failed to create client API", err)
	assert.Equal(1, sl.StartCnt)

	realCA, err := app.getClientArgs()
	assert.NoError(err)
	assert.NotNil(realCA)
}
