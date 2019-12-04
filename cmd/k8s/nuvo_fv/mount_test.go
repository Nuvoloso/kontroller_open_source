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
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/go-openapi/swag"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
)

func TestMountParser(t *testing.T) {
	assert := assert.New(t)
	// just verify that mount command is registered
	_, err := parser.ParseArgs([]string{"mount", "-h"})
	if assert.Error(err) {
		flagErr, ok := err.(*flags.Error)
		assert.True(ok)
		assert.Equal(flags.ErrHelp, flagErr.Type)
	}
}

func TestMountExecute(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	var b bytes.Buffer
	outputWriter = &b
	appCtx.ClusterID = "cluster-1"
	appCtx.NodeID = "node-1"
	appCtx.NuvoMountDir = "/mnt/nuvo"
	appCtx.SystemID = "system-1"
	appCtx.Log = tl.Logger()
	fc := &fake.Client{}
	appCtx.oCrud = fc

	assertFailure := func(msgMatch string) bool {
		o := map[string]string{}
		result := assert.True(b.Len() > 0) &&
			assert.NoError(json.Unmarshal(b.Bytes(), &o)) &&
			assert.Contains(o, "status") &&
			assert.Equal("Failure", o["status"]) &&
			assert.Contains(o, "message") &&
			assert.Regexp(msgMatch, o["message"]) &&
			assert.NotContains(o, "capabilities")
		b.Reset()
		return result
	}

	retV := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{ID: "vol-1"},
		},
	}
	retV.VolumeSeriesState = "UNBOUND"

	m := &mountCmd{}
	fm := &fakeMountOps{}
	m.ops = fm
	assert.NoError(m.Execute([]string{}))
	assertFailure("^invalid json-options")

	m.Args.JSONOptions = `{"kubernetes.io/readwrite":"ro"}`
	assert.NoError(m.Execute([]string{}))
	assertFailure("^incorrect systemId")

	m.Args.JSONOptions = `{"nuvoSystemId":"system-1","kubernetes.io/readwrite":"ro"}`
	assert.NoError(m.Execute([]string{}))
	assertFailure("^readOnly mount is not supported")

	m.Args.JSONOptions = `{"nuvoSystemId":"system-1","kubernetes.io/readwrite":"rw","kubernetes.io/fsType":"nfs"}`
	assert.NoError(m.Execute([]string{}))
	assertFailure("unsupported fsType")

	m.Args.JSONOptions = `{"kubernetes.io/fsType":"xfs","nuvoSystemId":"system-1"}`
	fc.RetVErr = errors.New("not found")
	assert.NoError(m.Execute([]string{}))
	assertFailure("volume.* lookup failed")

	m.Args.JSONOptions = `{"nuvoSystemId":"system-1"}`
	fc.RetVObj, fc.RetVErr = retV, nil
	assert.NoError(m.Execute([]string{}))
	if assert.NotNil(m.options.FsType) {
		assert.Equal("", *m.options.FsType)
	}
	if assert.NotNil(m.options.Snapshot) {
		assert.Equal("HEAD", *m.options.Snapshot)
	}
	assertFailure("not bound to cluster")

	// cover the state machine
	retV.BoundClusterID = "cluster-1"
	retV.VolumeSeriesState = "BOUND"
	fm.getInitialStateResult = CreateMountVSR
	fm.getInitialStateErr = nil
	fm.called = []string{}
	assert.NoError(m.Execute([]string{}))
	assert.Equal([]string{"GIS", "CVR", "WVR", "CLB", "FSE", "MKF", "RMI", "MFS"}, fm.called)
	assert.Regexp("status.*Success", b.String())
	b.Reset()

	fm.getInitialStateResult = CreateMountVSR
	fm.getInitialStateErr = errors.New("GIS error")
	fm.called = []string{}
	assert.NoError(m.Execute([]string{}))
	assert.Equal([]string{"GIS"}, fm.called)
	assertFailure(fm.getInitialStateErr.Error())

	fm.getInitialStateResult = CreateMountVSR
	fm.getInitialStateErr = nil
	fm.createMountVSRErr = errors.New("CVR error")
	fm.called = []string{}
	assert.NoError(m.Execute([]string{}))
	assert.Equal([]string{"GIS", "CVR"}, fm.called)
	assertFailure(fm.createMountVSRErr.Error())

	fm.getInitialStateResult = WaitForVSR
	fm.createMountVSRErr = nil
	fm.waitForVSRErr = errors.New("WVR error")
	fm.called = []string{}
	assert.NoError(m.Execute([]string{}))
	assert.Equal([]string{"GIS", "WVR"}, fm.called)
	assertFailure(fm.waitForVSRErr.Error())

	fm.getInitialStateResult = WaitForVSR
	fm.waitForVSRErr = nil
	fm.createLoopbackErr = errors.New("CLB error")
	fm.called = []string{}
	assert.NoError(m.Execute([]string{}))
	assert.Equal([]string{"GIS", "WVR", "CLB"}, fm.called)
	assertFailure(fm.createLoopbackErr.Error())

	fm.getInitialStateResult = CreateLoopback
	fm.createLoopbackErr = nil
	fm.fileSystemExistsErr = errors.New("FSE error")
	fm.called = []string{}
	assert.NoError(m.Execute([]string{}))
	assert.Equal([]string{"GIS", "CLB", "FSE"}, fm.called)
	assertFailure(fm.fileSystemExistsErr.Error())

	fm.getInitialStateResult = CreateLoopback
	fm.fileSystemExistsErr = nil
	fm.fileSystemExistsResult = true
	fm.called = []string{}
	assert.NoError(m.Execute([]string{}))
	assert.Equal([]string{"GIS", "CLB", "FSE", "RMI", "MFS"}, fm.called)
	assert.Regexp("status.*Success", b.String())
	b.Reset()

	fm.getInitialStateResult = CheckForFileSystem
	fm.makeFileSystemErr = errors.New("MKF error")
	fm.fileSystemExistsResult = false
	fm.called = []string{}
	assert.NoError(m.Execute([]string{}))
	assert.Equal([]string{"GIS", "FSE", "MKF"}, fm.called)
	assertFailure(fm.makeFileSystemErr.Error())

	fm.getInitialStateResult = CheckForFileSystem
	fm.makeFileSystemErr = nil
	fm.recordMountInfoErr = errors.New("RMI error")
	fm.fileSystemExistsResult = true
	fm.called = []string{}
	assert.NoError(m.Execute([]string{}))
	assert.Equal([]string{"GIS", "FSE", "RMI"}, fm.called)

	assertFailure(fm.recordMountInfoErr.Error())
	fm.getInitialStateResult = CheckForFileSystem
	fm.recordMountInfoErr = nil
	fm.mountFileSystemErr = errors.New("MFS error")
	fm.fileSystemExistsResult = false
	fm.called = []string{}
	assert.NoError(m.Execute([]string{}))
	assert.Equal([]string{"GIS", "FSE", "MKF", "RMI", "MFS"}, fm.called)
	assertFailure(fm.mountFileSystemErr.Error())
}

func TestMountGetInitialState(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	var b bytes.Buffer
	outputWriter = &b
	appCtx.ctx = context.Background()
	appCtx.ClusterID = "cluster-1"
	appCtx.NodeID = "node-2"
	appCtx.NuvoMountDir = "/mnt/nuvo"
	appCtx.SystemID = "system-1"
	appCtx.Log = tl.Logger()
	fc := &fake.Client{}
	appCtx.oCrud = fc

	retV := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{ID: "vol-1"},
		},
	}
	retV.BoundClusterID = "cluster-1"
	retV.VolumeSeriesState = "IN_USE"
	retV.Mounts = []*models.Mount{
		&models.Mount{SnapIdentifier: "HEAD", MountedNodeDevice: "vol-HEAD", MountedNodeID: "node-1"},
	}
	retVSR := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{ID: "vsr-1"},
		},
	}
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{}
	expVSRParams := &volume_series_request.VolumeSeriesRequestListParams{
		IsTerminated:   swag.Bool(false),
		VolumeSeriesID: swag.String("vol-1"),
		NodeID:         swag.String("node-2"),
		SystemTags:     []string{"t-1"},
	}

	fe := &fakeExec{assert: assert, cmd: "mount", cmdErr: errors.New("mount error")}
	appCtx.exec = fe
	m := &mountCmd{}
	m.Args.MountDir = "/mnt/mine"
	m.ops = m
	state, err := m.getInitialState()
	assert.Equal(MountDone, state)
	assert.Equal(fe.cmdErr, err)

	fe = &fakeExec{assert: assert, cmd: "mount", cmdResult: "/dev/loop0 on /mnt/mine type ext4 (and so on)"}
	appCtx.exec = fe
	m = &mountCmd{}
	m.Args.MountDir = "/mnt/mine"
	m.ops = m
	state, err = m.getInitialState()
	assert.Equal(MountDone, state)
	assert.NoError(err)

	fe = &fakeExec{assert: assert, cmd: "mount"}
	appCtx.exec = fe
	m = &mountCmd{options: &JSONOptions{}}
	m.vs = retV
	m.Args.MountDir = "/mnt/mine"
	m.options.VolumeID = "vol-1"
	m.options.Snapshot = swag.String("HEAD")
	m.vsrTag = "t-1"
	m.ops = m
	state, err = m.getInitialState()
	assert.Equal(MountDone, state)
	if assert.Error(err) {
		assert.Regexp("already in use on node", err.Error())
	}
	assert.Equal(expVSRParams, fc.InLsVRObj)

	fc.InLsVRObj = nil
	fc.RetLsVRErr = errors.New("VRList error")
	state, err = m.getInitialState()
	assert.Equal(expVSRParams, fc.InLsVRObj)
	assert.Equal(MountDone, state)
	if assert.Error(err) {
		assert.Regexp("volume-series-request list failed.*VRList", err.Error())
	}

	fc.InLsVRObj = nil
	fc.RetLsVRErr = nil
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{}
	fc.RetLsVRObj.Payload = []*models.VolumeSeriesRequest{retVSR}
	m.vsr = nil
	state, err = m.getInitialState()
	assert.Equal(expVSRParams, fc.InLsVRObj)
	assert.Equal(WaitForVSR, state)
	assert.NoError(err)
	assert.Equal(retVSR, m.vsr)

	retV.VolumeSeriesState = "UNBOUND"
	fc.InLsVRObj = nil
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{}
	state, err = m.getInitialState()
	assert.Equal(expVSRParams, fc.InLsVRObj)
	assert.Equal(MountDone, state)
	if assert.Error(err) {
		assert.Regexp("is not mountable", err.Error())
	}

	retV.VolumeSeriesState = "BOUND"
	fc.InLsVRObj = nil
	state, err = m.getInitialState()
	assert.Equal(expVSRParams, fc.InLsVRObj)
	assert.Equal(CreateMountVSR, state)
	assert.NoError(err)

	retV.VolumeSeriesState = "IN_USE"
	m.options.Snapshot = swag.String("NotHead")
	fc.InLsVRObj = nil
	state, err = m.getInitialState()
	assert.Equal(expVSRParams, fc.InLsVRObj)
	assert.Equal(CreateLoopback, state)
	assert.NoError(err)

	appCtx.NodeID = "node-1"
	expVSRParams.NodeID = swag.String("node-1")
	m.options.Snapshot = swag.String("HEAD")
	fe.nextCmd = append(fe.nextCmd, &fakeExec{cmd: "losetup", argCount: 2, cmdErr: errors.New("losetup fails")})
	fc.InLsVRObj = nil
	state, err = m.getInitialState()
	assert.Equal(expVSRParams, fc.InLsVRObj)
	assert.Equal(MountDone, state)
	assert.Equal("/mnt/nuvo/vol-HEAD", m.nuvoVolPath)
	if assert.Error(err) {
		assert.Regexp("losetup -j.*fails", err.Error())
	}

	fe = &fakeExec{assert: assert, cmd: "mount"}
	fe.nextCmd = append(fe.nextCmd, &fakeExec{cmd: "losetup", argCount: 2})
	appCtx.exec = fe
	fc.InLsVRObj = nil
	state, err = m.getInitialState()
	assert.Equal(expVSRParams, fc.InLsVRObj)
	assert.Equal(CreateLoopback, state)
	assert.NoError(err)

	fe = &fakeExec{assert: assert, cmd: "mount"}
	fe.nextCmd = append(fe.nextCmd, &fakeExec{cmd: "losetup", argCount: 2, cmdResult: "/dev/sd1: unexpected\n"})
	appCtx.exec = fe
	fc.InLsVRObj = nil
	state, err = m.getInitialState()
	assert.Equal(expVSRParams, fc.InLsVRObj)
	assert.Equal(CreateLoopback, state)
	assert.NoError(err)

	fe = &fakeExec{assert: assert, cmd: "mount"}
	fe.nextCmd = append(fe.nextCmd, &fakeExec{cmd: "losetup", argCount: 2, cmdResult: "/dev/loop3: expected\n"})
	fe.nextCmd = append(fe.nextCmd, &fakeExec{cmd: "lsblk", argCount: 3, cmdErr: errors.New("lsblk fails"), cmdResult: "you lose"})
	appCtx.exec = fe
	fc.InLsVRObj = nil
	state, err = m.getInitialState()
	assert.Equal(expVSRParams, fc.InLsVRObj)
	assert.Equal(m.loopDev, "/dev/loop3")
	assert.Equal(MountDone, state)
	if assert.Error(err) {
		assert.Regexp("lsblk fails", err.Error())
	}

	blkResult := `{
		"blockdevices": [
			{"name": "loop3", "fstype": "ext4", "label": null, "uuid": "vol-1", "mountpoint": "/mnt/mine"}
		]
	}`
	fe = &fakeExec{assert: assert, cmd: "mount"}
	fe.nextCmd = append(fe.nextCmd, &fakeExec{cmd: "losetup", argCount: 2, cmdResult: "/dev/loop3: expected\n"})
	fe.nextCmd = append(fe.nextCmd, &fakeExec{cmd: "lsblk", argCount: 3, cmdResult: blkResult})
	appCtx.exec = fe
	fc.InLsVRObj = nil
	m.loopDev = ""
	m.options.FsType = swag.String("")
	state, err = m.getInitialState()
	assert.Equal(expVSRParams, fc.InLsVRObj)
	assert.Equal(m.loopDev, "/dev/loop3")
	assert.Equal(MountDone, state)
	assert.NoError(err)

	fe = &fakeExec{assert: assert, cmd: "mount"}
	fe.nextCmd = append(fe.nextCmd, &fakeExec{cmd: "losetup", argCount: 2, cmdResult: "/dev/loop3: expected\n"})
	fe.nextCmd = append(fe.nextCmd, &fakeExec{cmd: "lsblk", argCount: 3, cmdResult: blkResult})
	appCtx.exec = fe
	fc.InLsVRObj = nil
	m.loopDev = ""
	m.Args.MountDir = "/another/path"
	state, err = m.getInitialState()
	assert.Equal(expVSRParams, fc.InLsVRObj)
	assert.Equal(m.loopDev, "/dev/loop3")
	assert.Equal(MountDone, state)
	if assert.Error(err) {
		assert.Regexp("already in use.*dev/loop3", err.Error())
	}

	blkResult = `{
		"blockdevices": [
			{"name": "loop3", "fstype": "ext4", "label": null, "uuid": "vol-1", "mountpoint": null}
		]
	}`
	fe = &fakeExec{assert: assert, cmd: "mount"}
	fe.nextCmd = append(fe.nextCmd, &fakeExec{cmd: "losetup", argCount: 2, cmdResult: "/dev/loop3: expected\n"})
	fe.nextCmd = append(fe.nextCmd, &fakeExec{cmd: "lsblk", argCount: 3, cmdResult: blkResult})
	appCtx.exec = fe
	fc.InLsVRObj = nil
	m.loopDev = ""
	m.Args.MountDir = "/mnt/mine"
	state, err = m.getInitialState()
	assert.Equal(expVSRParams, fc.InLsVRObj)
	assert.Equal(m.loopDev, "/dev/loop3")
	assert.Equal(MountFileSystem, state)
	assert.NoError(err)

	blkResult = `{
		"blockdevices": [
			{"name": "loop3", "fstype": null, "label": null, "uuid": "vol-1", "mountpoint": null}
		]
	}`
	fe = &fakeExec{assert: assert, cmd: "mount"}
	fe.nextCmd = append(fe.nextCmd, &fakeExec{cmd: "losetup", argCount: 2, cmdResult: "/dev/loop3: expected\n"})
	fe.nextCmd = append(fe.nextCmd, &fakeExec{cmd: "lsblk", argCount: 3, cmdResult: blkResult})
	appCtx.exec = fe
	fc.InLsVRObj = nil
	m.loopDev = ""
	m.Args.MountDir = "/mnt/mine"
	state, err = m.getInitialState()
	assert.Equal(expVSRParams, fc.InLsVRObj)
	assert.Equal(m.loopDev, "/dev/loop3")
	assert.Equal(MakeFileSystem, state)
	assert.NoError(err)
}

func TestGetLoopbackDeviceInfo(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	var b bytes.Buffer
	outputWriter = &b
	appCtx.ctx = context.Background()
	appCtx.ClusterID = "cluster-1"
	appCtx.NodeID = "node-2"
	appCtx.NuvoMountDir = "/mnt/nuvo"
	appCtx.SystemID = "system-1"
	appCtx.Log = tl.Logger()
	fc := &fake.Client{}
	appCtx.oCrud = fc
	fe := &fakeExec{assert: assert, cmd: "lsblk", argCount: 3, cmdResult: `{"invalid JSON" `}
	appCtx.exec = fe

	m := &mountCmd{options: &JSONOptions{}}
	m.options.VolumeID = "vol-1"
	m.options.Snapshot = swag.String("HEAD")
	m.options.FsType = swag.String("xfs")
	m.loopDev = "/dev/loop1"
	m.vsrTag = "t-1"
	m.ops = m
	res, err := m.getLoopbackDeviceInfo()
	assert.Nil(res)
	if assert.Error(err) {
		assert.Regexp("^lsblk output error:", err.Error())
	}

	blkResult := `{ "blockdevices": [] }`
	fe = &fakeExec{assert: assert, cmd: "lsblk", argCount: 3, cmdResult: blkResult}
	appCtx.exec = fe
	res, err = m.getLoopbackDeviceInfo()
	assert.Nil(res)
	assert.NoError(err)

	blkResult = `{
		"blockdevices": [
			{"name": "loop3", "fstype": "ext4", "label": null, "uuid": "vol-1", "mountpoint": null}
		]
	}`
	fe = &fakeExec{assert: assert, cmd: "lsblk", argCount: 3, cmdResult: blkResult}
	appCtx.exec = fe
	res, err = m.getLoopbackDeviceInfo()
	assert.NotNil(res)
	if assert.Error(err) {
		assert.Regexp("formatted with.*ext4.*expected.*xfs", err.Error())
	}

	blkResult = `{
		"blockdevices": [
			{"name": "loop3", "fstype": "xfs", "label": null, "uuid": "vol-1", "mountpoint": null}
		]
	}`
	fe = &fakeExec{assert: assert, cmd: "lsblk", argCount: 3, cmdResult: blkResult}
	appCtx.exec = fe
	res, err = m.getLoopbackDeviceInfo()
	if assert.NotNil(res) {
		assert.Equal("loop3", res.Name)
		assert.Equal("xfs", *res.FsType)
		assert.Nil(res.MountPoint)
	}
	assert.NoError(err)
}

func TestCreateMountVSR(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	var b bytes.Buffer
	outputWriter = &b
	appCtx.ctx = context.Background()
	appCtx.ClusterID = "cluster-1"
	appCtx.NodeID = "node-2"
	appCtx.NuvoMountDir = "/mnt/nuvo"
	appCtx.SystemID = "system-1"
	appCtx.Log = tl.Logger()
	fc := &fake.Client{}
	appCtx.oCrud = fc
	retV := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{ID: "vol-1"},
		},
	}
	m := &mountCmd{options: &JSONOptions{}}
	m.vs = retV
	m.Args.MountDir = "./tmpMount"
	m.options.VolumeID = "vol-1"
	m.options.Snapshot = swag.String("HEAD")
	m.options.FsType = swag.String("")
	m.loopDev = "/dev/loop2"
	m.vsrTag = "t-1"
	m.ops = m

	fc.RetVRCErr = errors.New("VSR Create error")
	err := m.createMountVSR()
	if assert.Error(err) {
		assert.Regexp("error creating MOUNT.*VSR Create error", err.Error())
	}
	assert.NotNil(m.vsr)
	assert.Equal(m.vsr, fc.InVRCArgs)

	fc.InVRCArgs, fc.RetVRCErr = nil, nil
	fc.PassThroughVRCObj = true
	m.vsr = nil
	err = m.createMountVSR()
	assert.NoError(err)
	assert.NotNil(m.vsr)
	assert.Equal(m.vsr, fc.InVRCArgs)
}

func TestWaitFirVSR(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	var b bytes.Buffer
	outputWriter = &b
	appCtx.ctx = context.Background()
	appCtx.ClusterID = "cluster-1"
	appCtx.NodeID = "node-2"
	appCtx.NuvoMountDir = "/mnt/nuvo"
	appCtx.SystemID = "system-1"
	appCtx.Log = tl.Logger()
	fc := &fake.Client{}
	appCtx.oCrud = fc
	retV := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{ID: "vol-1"},
		},
	}
	inVSR := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{ID: "vsr-1"},
		},
	}
	retVSR := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{ID: "vsr-1"},
		},
	}
	retVSR.VolumeSeriesRequestState = "FAILED"
	m := &mountCmd{options: &JSONOptions{}}
	m.vsr = inVSR
	m.waitInterval = time.Microsecond
	m.Args.MountDir = "./tmpMount"
	m.options.VolumeID = "vol-1"
	m.options.Snapshot = swag.String("HEAD")
	m.options.FsType = swag.String("")
	m.loopDev = "/dev/loop2"
	m.vsrTag = "t-1"
	m.ops = m

	fc.RetVRErr = errors.New("fetch error")
	err := m.waitForVSR()
	if assert.Error(err) {
		assert.Regexp("lookup failed.*fetch error", err.Error())
	}

	fc.RetVRObj = retVSR
	fc.RetVRErr = nil
	err = m.waitForVSR()
	if assert.Error(err) {
		assert.Regexp("volume-series-request.*FAILED", err.Error())
	}
	assert.Equal(m.vsr, retVSR)

	retVSR.VolumeSeriesRequestState = "SUCCEEDED"
	fc.RetVErr = errors.New("fetch fails")
	err = m.waitForVSR()
	if assert.Error(err) {
		assert.Regexp("volume .*lookup failed: fetch fails", err.Error())
	}
	assert.Equal(m.vsr, retVSR)

	m.vsr = inVSR
	fc.RetVObj = retV
	fc.RetVErr = nil
	err = m.waitForVSR()
	if assert.Error(err) {
		assert.Regexp("LUN not found", err.Error())
	}
	assert.Equal(m.vsr, retVSR)

	retV.Mounts = []*models.Mount{
		&models.Mount{SnapIdentifier: "HEAD", MountedNodeDevice: "vol-HEAD", MountedNodeID: "node-1"},
	}
	m.vsr = inVSR
	fc.RetVObj = retV
	fc.RetVErr = nil
	assert.NoError(m.waitForVSR())
	assert.Equal(m.vsr, retVSR)
	assert.Equal("/mnt/nuvo/vol-HEAD", m.nuvoVolPath)
	assert.Equal(1, tl.CountPattern("LUN mounted at "+m.nuvoVolPath))
}

func TestCreateLoopback(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	var b bytes.Buffer
	outputWriter = &b
	appCtx.ctx = context.Background()
	appCtx.ClusterID = "cluster-1"
	appCtx.NodeID = "node-2"
	appCtx.NuvoMountDir = "/mnt/nuvo"
	appCtx.SystemID = "system-1"
	appCtx.Log = tl.Logger()
	fe := &fakeExec{assert: assert, cmd: "losetup", argCount: 3, cmdErr: errors.New("losetup error"), cmdResult: "verbose output"}
	appCtx.exec = fe
	m := &mountCmd{options: &JSONOptions{}}
	m.nuvoVolPath = "/some/path"
	m.loopDev = ""
	err := m.createLoopback()
	if assert.Error(err) {
		assert.Regexp("losetup failed: losetup error", err.Error())
	}
	assert.Equal(1, tl.CountPattern("verbose output"))
	tl.Flush()

	m.loopDev = ""
	fe = &fakeExec{assert: assert, cmd: "losetup", argCount: 3, cmdResult: "/dev/loop2"}
	fe.nextCmd = append(fe.nextCmd, &fakeExec{cmd: "blkid", argCount: 1, cmdErr: errors.New("blkid failure ignored")})
	appCtx.exec = fe
	assert.NoError(m.createLoopback())
	assert.Equal("/dev/loop2", m.loopDev)
	assert.Nil(m.loopInfo)
	assert.Equal(1, tl.CountPattern("blkid .* error:.*failure ignored"))
	tl.Flush()

	m.loopDev = ""
	fe = &fakeExec{assert: assert, cmd: "losetup", argCount: 3, cmdResult: "/dev/loop2"}
	fe.nextCmd = append(fe.nextCmd, &fakeExec{cmd: "blkid", argCount: 1, cmdResult: "/dev/loop2: UUID=\"118aec8d-2496-4ce3-9b02-b292441f9685\" TYPE=\"ext4\"\n"})
	appCtx.exec = fe
	assert.NoError(m.createLoopback())
	assert.Equal("/dev/loop2", m.loopDev)
	if assert.NotNil(m.loopInfo) {
		assert.Equal("loop2", m.loopInfo.Name)
		assert.Equal("ext4", *m.loopInfo.FsType)
	}
}

func TestFileSystemExists(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	var b bytes.Buffer
	outputWriter = &b
	appCtx.ctx = context.Background()
	appCtx.Log = tl.Logger()
	fe := &fakeExec{assert: assert, cmd: "lsblk", argCount: 3, cmdErr: errors.New("lose again")}
	appCtx.exec = fe
	m := &mountCmd{options: &JSONOptions{}}
	m.loopDev = "/dev/loop0"
	m.loopInfo = &LsBlkDevice{}
	exists, err := m.fileSystemExists()
	assert.NoError(err)
	assert.False(exists)

	m.loopInfo = nil
	exists, err = m.fileSystemExists()
	if assert.Error(err) {
		assert.Regexp("lose again", err.Error())
	}
	assert.False(exists)

	blkResult := `{
		"blockdevices": [
			{"name": "loop3", "fstype": "ext4", "label": null, "uuid": "vol-1", "mountpoint": "/mnt/mine"}
		]
	}`
	m.loopInfo = nil
	m.options.FsType = swag.String("ext4")
	fe = &fakeExec{assert: assert, cmd: "lsblk", argCount: 3, cmdResult: blkResult}
	appCtx.exec = fe
	exists, err = m.fileSystemExists()
	assert.NoError(err)
	assert.True(exists)
	if assert.NotNil(m.loopInfo) {
		assert.Equal("loop3", m.loopInfo.Name)
		assert.Equal("ext4", *m.loopInfo.FsType)
		assert.Equal("/mnt/mine", *m.loopInfo.MountPoint)
	}
}

func TestMakeFileSystem(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	var b bytes.Buffer
	outputWriter = &b
	appCtx.ctx = context.Background()
	appCtx.ClusterID = "cluster-1"
	appCtx.NodeID = "node-2"
	appCtx.NuvoMountDir = "/mnt/nuvo"
	appCtx.SystemID = "system-1"
	appCtx.Log = tl.Logger()
	fc := &fake.Client{}
	appCtx.oCrud = fc
	fe := &fakeExec{assert: assert, cmd: "mkfs.xfs", argCount: 7, cmdErr: errors.New("mkfs error"), cmdResult: "verbose output"}
	appCtx.exec = fe

	retV := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{ID: "vol-1"},
		},
	}
	m := &mountCmd{options: &JSONOptions{}}
	m.vs = retV
	m.options.VolumeID = "vol-1"
	m.options.Snapshot = swag.String("HEAD")
	m.options.FsType = swag.String("")
	m.loopDev = "/dev/loop2"
	m.vsrTag = "t-1"
	m.ops = m
	err := m.makeFileSystem()
	if assert.Error(err) {
		assert.Regexp("mkfs failed:.*mkfs error", err.Error())
	}
	assert.Equal(1, tl.CountPattern("verbose output"))
	tl.Flush()

	m.options.FsType = swag.String("ext4")
	fe = &fakeExec{assert: assert, cmd: "mkfs.ext4", argCount: 5, cmdResult: "normal output"}
	appCtx.exec = fe
	assert.NoError(m.makeFileSystem())
	assert.Equal(1, tl.CountPattern("normal output"))
	assert.Equal("4096", os.Getenv("MKE2FS_DEVICE_SECTSIZE"))
}

func TestRecordMountInfo(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	var b bytes.Buffer
	outputWriter = &b
	appCtx.Log = tl.Logger()
	m := &mountCmd{options: &JSONOptions{}}
	m.loopDev = "/dev/loop2"
	m.nuvoVolPath = "/some/path"
	m.Args.MountDir = "./tmpMount"
	m.options.VolumeID = "vol-1"
	m.options.Snapshot = swag.String("HEAD")
	m.options.FsType = swag.String("ext4")
	m.vsrTag = "t-1"
	os.RemoveAll(m.Args.MountDir)
	assert.NoError(ioutil.WriteFile(m.Args.MountDir, []byte("data"), 0644))
	err := m.recordMountInfo()
	if assert.Error(err) {
		assert.Regexp("directory.*creation failed", err.Error())
	}
	assert.NoError(os.Remove(m.Args.MountDir))
	tl.Flush()

	assert.NoError(m.recordMountInfo())
	_, err = os.Stat(m.Args.MountDir)
	assert.NoError(err)
	assert.Equal(1, tl.CountPattern("created mount-dir"))
	stash := "./tmpMount/.nuvoMountInfo"
	_, err = os.Stat(stash)
	buf, err := ioutil.ReadFile(stash)
	assert.NoError(err)
	var mi MountInfo
	assert.NoError(json.Unmarshal(buf, &mi))
	assert.Equal(mi.SnapIdentifier, "HEAD")
	assert.Equal(mi.VolumeID, "vol-1")
	tl.Flush()

	assert.NoError(os.Remove(stash))
	assert.NoError(os.Mkdir(stash, 0755))
	err = m.recordMountInfo()
	if assert.Error(err) {
		assert.Regexp("stash.*creation failed", err.Error())
	}
	assert.NoError(os.RemoveAll(stash))
	tl.Flush()

	assert.NoError(m.recordMountInfo())
	assert.Equal(1, tl.CountPattern("mount-dir exists"))

	stat, err := os.Stat(m.Args.MountDir)
	if assert.NoError(err) {
		assert.True(stat.IsDir())
		assert.NoError(os.RemoveAll(m.Args.MountDir))
	}
}

func TestMountFileSystem(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	var b bytes.Buffer
	outputWriter = &b
	appCtx.ctx = context.Background()
	appCtx.ClusterID = "cluster-1"
	appCtx.NodeID = "node-2"
	appCtx.NuvoMountDir = "/mnt/nuvo"
	appCtx.SystemID = "system-1"
	appCtx.Log = tl.Logger()
	fe := &fakeExec{assert: assert, cmd: "mount", argCount: 2, cmdErr: errors.New("mount error"), cmdResult: "verbose output"}
	appCtx.exec = fe
	m := &mountCmd{options: &JSONOptions{}}
	m.loopDev = "/dev/loop2"
	m.Args.MountDir = "/mnt/mine"
	err := m.mountFileSystem()
	if assert.Error(err) {
		assert.Regexp("mount failed: mount error", err.Error())
	}
	assert.Equal(1, tl.CountPattern("verbose output"))
	tl.Flush()

	fe = &fakeExec{assert: assert, cmd: "mount", argCount: 2, cmdResult: "normal output"}
	appCtx.exec = fe
	assert.NoError(m.mountFileSystem())
	assert.Equal(1, tl.CountPattern("normal output"))
}

type fakeMountOps struct {
	called                 []string
	getInitialStateErr     error
	getInitialStateResult  mountState
	createMountVSRErr      error
	waitForVSRErr          error
	createLoopbackErr      error
	fileSystemExistsErr    error
	fileSystemExistsResult bool
	makeFileSystemErr      error
	recordMountInfoErr     error
	mountFileSystemErr     error
}

func (c *fakeMountOps) getInitialState() (mountState, error) {
	c.called = append(c.called, "GIS")
	return c.getInitialStateResult, c.getInitialStateErr
}

func (c *fakeMountOps) createMountVSR() error {
	c.called = append(c.called, "CVR")
	return c.createMountVSRErr
}

func (c *fakeMountOps) waitForVSR() error {
	c.called = append(c.called, "WVR")
	return c.waitForVSRErr
}

func (c *fakeMountOps) createLoopback() error {
	c.called = append(c.called, "CLB")
	return c.createLoopbackErr
}

func (c *fakeMountOps) fileSystemExists() (bool, error) {
	c.called = append(c.called, "FSE")
	return c.fileSystemExistsResult, c.fileSystemExistsErr
}

func (c *fakeMountOps) makeFileSystem() error {
	c.called = append(c.called, "MKF")
	return c.makeFileSystemErr
}

func (c *fakeMountOps) recordMountInfo() error {
	c.called = append(c.called, "RMI")
	return c.recordMountInfoErr
}

func (c *fakeMountOps) mountFileSystem() error {
	c.called = append(c.called, "MFS")
	return c.mountFileSystemErr
}
