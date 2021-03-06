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


package csi

import (
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi"
	mockcluster "github.com/Nuvoloso/kontroller/pkg/cluster/mock"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	fa "github.com/Nuvoloso/kontroller/pkg/clusterd/fake"
	fcsi "github.com/Nuvoloso/kontroller/pkg/csi/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	fu "github.com/Nuvoloso/kontroller/pkg/util/fake"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestComponentInit(t *testing.T) {
	assert := assert.New(t)
	args := Args{
		StartupSleepInterval: time.Second * 5,
		VSRCompleteByPeriod:  time.Minute * 5,
	}
	// This actually calls agentd.AppRegisterComponent but we can't intercept that
	c, ok := ComponentInit(&args).(*csiComp)
	assert.True(ok)
	assert.Equal(args.StartupSleepInterval, c.StartupSleepInterval)
	assert.Equal(args.VSRCompleteByPeriod, c.VSRCompleteByPeriod)
}
func TestCSIComponentMethods(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	mockCtrl := gomock.NewController(t)
	appS := &fa.AppServant{}
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log:            tl.Logger(),
			ServiceVersion: "0.6.2",
		},
		AppServant:    appS,
		ClusterClient: mockcluster.NewMockClient(mockCtrl),
	}
	c := &csiComp{}
	var err error

	// init with no CSI - nothing created
	c.Init(app)
	assert.Equal(app.Log, c.Log)
	assert.Equal(app, c.app)
	assert.Nil(c.h)
	assert.Nil(c.w)

	// Start and stop are no-ops
	assert.NotPanics(func() { c.Start() })
	assert.NotPanics(func() { c.Stop() })

	// CSI configured
	app.CSISocket = "/the/csi/socket"

	// test worker arg construction
	wa := c.getWorkerArgs()
	assert.Equal("csiComp", wa.Name)
	assert.Equal(c.Log, wa.Log)
	assert.Equal(c.Args.StartupSleepInterval, wa.SleepInterval)

	// init with CSI - creates worker
	c = &csiComp{}
	c.Init(app)
	assert.Equal(app.Log, c.Log)
	assert.Equal(app, c.app)
	assert.Nil(c.h)
	assert.NotNil(c.w)

	// replace the worker with a fake
	fw := &fu.Worker{}
	c.w = fw

	// Start starts the worker
	c.Start()
	assert.True(fw.CntStart == 1)

	// Buzz does nothing until the Cluster is ready
	assert.Nil(app.AppObjects)
	c.Buzz(nil)
	assert.Nil(c.h)
	assert.True(fw.CntStop == 0)

	ao := &fa.AppObjects{}
	app.AppObjects = ao
	c.Buzz(nil)
	assert.Nil(c.h)
	assert.True(fw.CntStop == 0)

	// Cluster now available
	ao.RetGCobj = &models.Cluster{}

	// test handler arg construction
	cha := c.getCsiHandlerArgs()
	assert.NotNil(cha)
	assert.Equal(c.Log, cha.Log)
	assert.EqualValues(app.CSISocket, cha.Socket)
	assert.Equal(app.ServiceVersion, cha.Version)
	assert.Equal(app.ClusterClient, cha.ClusterClient)
	assert.Equal(c, cha.Ops)
	assert.NoError(cha.Validate())

	// Force failure during handler creation
	app.CSISocket = ""
	assert.Error(c.getCsiHandlerArgs().Validate())
	appS.CountFatalError = 0
	err = c.Buzz(nil)
	assert.Equal(util.ErrWorkerAborted, err)
	assert.Nil(c.h)
	assert.True(appS.CountFatalError == 1)
	app.CSISocket = "/the/csi/socket" // reset

	// Buzz creates handler; handler not started
	fw.CntStop = 0
	err = c.Buzz(nil)
	assert.NoError(err)
	assert.NotNil(c.h)
	assert.True(fw.CntStop == 0)
	assert.False(c.hStarted)
	fw.CntStop = 0 // reset

	// replace the handler with a fake
	fh := &fcsi.ControllerHandler{}
	c.h = fh

	// Stop without hStarted does not call the handler Stop
	assert.False(c.hStarted)
	fh.CalledStop = false
	fw.CntStop = 0
	c.Stop()
	assert.False(fh.CalledStop)
	assert.True(fw.CntStop == 1)

	// Buzz starts the handler and stops the worker
	fh.CalledStart = false
	err = c.Buzz(nil)
	assert.Equal(util.ErrWorkerAborted, err)
	assert.True(fh.CalledStart)

	// Stop stops the handler and the worker
	fw.CntStop = 0
	fh.CalledStop = false
	c.Stop()
	assert.True(fh.CalledStop)
}

func TestAppIsReady(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c := &csiComp{}
	aa := &clusterd.AppArgs{
		Log:            tl.Logger(),
		ServiceVersion: "serviceVersion",
		Server:         &restapi.Server{},
	}
	app, err := clusterd.AppInit(aa)
	assert.NoError(err)
	app.Service.SetState(util.ServiceReady)
	c.app = app
	assert.NotNil(app.Service)
	ready := c.IsReady()
	assert.True(ready)

	app.Service.SetState(util.ServiceNotReady)
	ready = c.IsReady()
	assert.False(ready)
}
