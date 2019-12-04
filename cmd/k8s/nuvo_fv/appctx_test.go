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
	"errors"
	"os/exec"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/Nuvoloso/kontroller/pkg/testutils"
)

func TestInit(t *testing.T) {
	assert := assert.New(t)
	savedWriter := logWriter
	defer func() {
		logWriter = savedWriter
	}()

	// no socket
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	app := AppCtx{}
	app.ClusterID = "cluster-1"
	app.NodeID = "node-1"
	app.NodeIdentifier = "i-1"
	app.SystemID = "system-1"
	app.api = nil
	app.oCrud = nil
	err := app.Init()
	if assert.Error(err) {
		assert.Regexp("socket path", err.Error())
	}
	assert.Nil(appCtx.api)

	// not configured
	app = AppCtx{}
	app.SocketPath = "/some/path"
	app.Verbose = make([]bool, 1)
	app.NuvoMountDir = "preset"
	err = app.Init()
	if assert.Error(err) {
		assert.Regexp("system properties are not yet configured", err.Error())
	}
	assert.NotNil(app.Log)
	assert.Nil(app.api)
	assert.Nil(app.oCrud)

	app = AppCtx{}
	app.ClusterID = "cluster-1"
	app.NodeID = "node-1"
	app.NodeIdentifier = "i-1"
	app.SystemID = "system-1"
	app.SocketPath = "/some/path"
	app.Verbose = make([]bool, 1)
	app.NuvoMountDir = "preset"
	assert.NoError(app.Init())
	assert.NotNil(app.Log)
	assert.NotNil(app.api)
	assert.NotNil(app.oCrud)
	_, ok := logWriter.(*bytes.Buffer)
	assert.True(ok, "logWriter is a Buffer")

	app = AppCtx{}
	app.ClusterID = "cluster-1"
	app.NodeID = "node-1"
	app.NodeIdentifier = "i-1"
	app.SystemID = "system-1"
	app.exec = &fakeExec{assert: assert, cmd: "mount"}
	curLogWriter := logWriter
	app.SocketPath = "/some/path"
	err = app.Init()
	if assert.Error(err) {
		assert.Regexp("mount point not found", err.Error())
	}
	assert.Equal(curLogWriter, logWriter)
}

func TestFindNuvoMountDir(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)

	app := &AppCtx{}
	app.ctx = context.Background()
	app.Log = tl.Logger()
	fe := &fakeExec{assert: assert, cmd: "mount", cmdErr: errors.New("mount fails")}
	app.exec = fe
	err := app.FindNuvoMountDir()
	if assert.Error(err) {
		assert.Equal(fe.cmdErr, err)
	}

	fe.cmdResult = "/dev/sd1 on / type ext4 (rw,relatime,discard,data=ordered)\n" +
		"tmpFs on /run/user/1000 type tmpFs (rw,relatime,size=404524k,mode=700,uid=1000,gid=1000)\n"
	fe.cmdErr = nil
	err = app.FindNuvoMountDir()
	if assert.Error(err) {
		assert.Regexp("nuvo mount point not found", err.Error())
	}

	fe.cmdResult = "/dev/sd1 on / type ext4 (rw,relatime,discard,data=ordered)\n" +
		"nuvo on /var/local/nuvoloso type fuse.nuvo (rw,relatime,user_id=0,group_id=0)\n" +
		"/dev/loop0 on /mnt/mine type ext4 (rw,relatime,data=ordered)\n"
	fe.cmdErr = nil
	assert.NoError(app.FindNuvoMountDir())
	assert.Equal(1, tl.CountPattern("nuvo is mounted.*/var/local/nuvoloso"))
}

func TestIsMounted(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)

	app := &AppCtx{}
	app.ctx = context.Background()
	app.Log = tl.Logger()
	fe := &fakeExec{assert: assert, cmd: "mount", cmdErr: errors.New("mount fails")}
	app.exec = fe
	dev, err := app.IsMounted("")
	if assert.Error(err) {
		assert.Equal(fe.cmdErr, err)
	}

	fe.cmdResult = "/dev/sd1 on / type ext4 (rw,relatime,discard,data=ordered)\n" +
		"tmpFs on /run/user/1000 type tmpFs (rw,relatime,size=404524k,mode=700,uid=1000,gid=1000)\n"
	fe.cmdErr = nil
	dev, err = app.IsMounted("/mnt/mine")
	assert.Zero(dev)
	assert.NoError(err)

	fe.cmdResult = "/dev/sd1 on / type ext4 (rw,relatime,discard,data=ordered)\n" +
		"nuvo on /var/local/nuvoloso type fuse.nuvo (rw,relatime,user_id=0,group_id=0)\n" +
		"/dev/loop0 on /mnt/mine type ext4 (rw,relatime,data=ordered)\n"
	fe.cmdErr = nil
	dev, err = app.IsMounted("/mnt/mine")
	assert.Equal("/dev/loop0", dev)
	assert.Equal(1, tl.CountPattern("mount-dir.* is mounted"))
}

func TestCommandContext(t *testing.T) {
	assert := assert.New(t)

	app := &AppCtx{}
	app.ClusterID = "cluster-1"
	app.NodeID = "node-1"
	app.NodeIdentifier = "i-1"
	app.SystemID = "system-1"
	app.SocketPath = "/some/path"
	app.NuvoMountDir = "preset"
	assert.NoError(app.Init())
	cmd := app.exec.CommandContext(app.ctx, "ls")
	_, ok := cmd.(*exec.Cmd)
	assert.True(ok)
	output, err := cmd.CombinedOutput()
	assert.NoError(err)
	assert.True(len(output) > 0)
}

type fakeExec struct {
	assert    *assert.Assertions
	cmd       string
	argCount  int
	nextCmd   []*fakeExec
	cmdErr    error
	cmdResult string
}

func (exec *fakeExec) CommandContext(ctx context.Context, name string, arg ...string) Cmd {
	exec.assert.NotNil(ctx, "appCtx ctx should not be null")
	exec.assert.Equal(exec.cmd, name)
	exec.assert.Len(arg, exec.argCount)
	res := &fakeCmd{cmdErr: exec.cmdErr, cmdResult: []byte(exec.cmdResult)}
	if len(exec.nextCmd) > 0 {
		next := exec.nextCmd[0]
		exec.nextCmd = exec.nextCmd[1:]
		exec.cmd = next.cmd
		exec.argCount = next.argCount
		exec.cmdErr = next.cmdErr
		exec.cmdResult = next.cmdResult
	}
	return res
}

type fakeCmd struct {
	cmdErr    error
	cmdResult []byte
}

func (cmd *fakeCmd) CombinedOutput() ([]byte, error) {
	return cmd.cmdResult, cmd.cmdErr
}
