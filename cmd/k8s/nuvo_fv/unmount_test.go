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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestUnmountParser(t *testing.T) {
	assert := assert.New(t)
	// just verify that unmount command is registered
	_, err := parser.ParseArgs([]string{"unmount", "-h"})
	if assert.Error(err) {
		flagErr, ok := err.(*flags.Error)
		assert.True(ok)
		assert.Equal(flags.ErrHelp, flagErr.Type)
	}
}

func TestUnmountExecute(t *testing.T) {
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

	c := &unmountCmd{}
	fm := &fakeUnmountOps{}
	c.ops = fm
	assert.NoError(c.Execute([]string{}))
	assert.Equal([]string{"GIS", "UFS", "RMI", "CUV", "WUV", "RMS"}, fm.called)
	assert.Regexp("status.*Success", b.String())
	b.Reset()

	fm = &fakeUnmountOps{}
	fm.getInitialStateResult = UnmountDone
	c.ops = fm
	assert.NoError(c.Execute([]string{}))
	assert.Equal([]string{"GIS"}, fm.called)
	assert.Regexp("status.*Success", b.String())
	b.Reset()

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

	fm = &fakeUnmountOps{}
	fm.getInitialStateResult = UnmountFileSystem
	fm.getInitialStateErr = errors.New("GIS error")
	c.ops = fm
	assert.NoError(c.Execute([]string{}))
	assert.Equal([]string{"GIS"}, fm.called)
	assertFailure(fm.getInitialStateErr.Error())

	fm = &fakeUnmountOps{}
	fm.unmountFileSystemErr = errors.New("Unmount error")
	c.ops = fm
	assert.NoError(c.Execute([]string{}))
	assert.Equal([]string{"GIS", "UFS"}, fm.called)
	assertFailure(fm.unmountFileSystemErr.Error())
}

func TestGetInitialState(t *testing.T) {
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
	appCtx.ctx = context.Background()

	c := &unmountCmd{}
	c.ops = c
	fe := &fakeExec{assert: assert, cmd: "lsblk", argCount: 2, cmdErr: errors.New("lsblk fails")}
	appCtx.exec = fe
	state, err := c.getInitialState()
	assert.Equal(UnmountDone, state)
	assert.Nil(c.loopInfo)
	if assert.Error(err) {
		assert.Regexp("lsblk fails", err)
	}

	blkResult := `{
		"blockdevices": [
		   {"name": "vda", "fstype": null, "label": null, "uuid": null, "mountpoint": null,
			  "children": [
				 {"name": "vda1", "fstype": "ext4", "label": "rootFs", "uuid": "3e13556e-d28d-407b-bcc6-97160eafebe1", "mountpoint": "/"}
			  ]
		   },
		   {"name": "loop1", "fstype": "ext4", "label": null, "uuid": "118aec8d-2496-4ce3-9b02-b292441f9685", "mountpoint": "/home/ubuntu/newvol2"},
		   {"name": "xvdbb", "fstype": null, "label": null, "uuid": null, "mountpoint": null}
		]
	}`
	fe = &fakeExec{assert: assert, cmd: "lsblk", argCount: 2, cmdResult: blkResult}
	appCtx.exec = fe
	c.Args.MountDir = "/home/ubuntu/newvol2"
	c.loopInfo = nil
	state, err = c.getInitialState()
	assert.Equal(UnmountFileSystem, state)
	assert.NoError(err)
	if assert.NotNil(c.loopInfo) {
		assert.Equal("loop1", c.loopInfo.Name)
		assert.Equal(c.Args.MountDir, *c.loopInfo.MountPoint)
		assert.Equal("ext4", *c.loopInfo.FsType)
	}
	tl.Flush()

	fe = &fakeExec{assert: assert, cmd: "lsblk", argCount: 2, cmdResult: blkResult}
	appCtx.exec = fe
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{}
	c.Args.MountDir = "./tmpMount"
	os.RemoveAll(c.Args.MountDir)
	c.loopInfo = nil
	state, err = c.getInitialState()
	assert.NoError(err)
	assert.Equal(UnmountDone, state)
	assert.Equal(1, tl.CountPattern("error reading.*stash"))
	tl.Flush()

	retVS := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{ID: "vol-1"},
		},
	}
	retVS.VolumeSeriesState = "IN_USE"
	fc.RetVObj = retVS
	mountInfo := &MountInfo{VolumeID: "vol-1", SnapIdentifier: "HEAD"}
	buf, err := json.Marshal(&mountInfo)
	assert.NoError(err)
	assert.NoError(os.Mkdir(c.Args.MountDir, 0755))
	assert.NoError(ioutil.WriteFile("./tmpMount/.nuvoMountInfo", buf, 0644))
	c.loopInfo = nil
	state, err = c.getInitialState()
	assert.Equal(CreateUnmountVSR, state)
	assert.NoError(err)
	assert.Nil(c.loopInfo)
	tl.Flush()

	retVS.VolumeSeriesState = "PROVISIONED"
	c.loopInfo = nil
	state, err = c.getInitialState()
	assert.Equal(RemoveMountStash, state)
	assert.NoError(err)
	assert.Nil(c.loopInfo)
	assert.Equal(1, tl.CountPattern("already unmounted"))
	tl.Flush()

	fc.RetVObj, fc.RetVErr = nil, crud.NewError(&models.Error{Code: 404, Message: swag.String(common.ErrorNotFound)})
	state, err = c.getInitialState()
	assert.NoError(err)
	assert.Equal(RemoveMountStash, state)
	assert.Equal(1, tl.CountPattern("no longer exists"))
	tl.Flush()

	fc.RetVErr = errors.New("fetch VS error")
	state, err = c.getInitialState()
	if assert.Error(err) {
		assert.Regexp("lookup failed.*fetch VS error", err.Error())
	}
	assert.Equal(UnmountDone, state)
	tl.Flush()

	retVSR := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{ID: "vsr-1"},
		},
	}
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{Payload: []*models.VolumeSeriesRequest{retVSR}}
	c.loopInfo = nil
	state, err = c.getInitialState()
	assert.Equal(WaitForUnmountVSR, state)
	assert.NoError(err)
	assert.Nil(c.loopInfo)

	fc.RetLsVRObj = nil
	fc.RetLsVRErr = errors.New("list VSR error")
	state, err = c.getInitialState()
	if assert.Error(err) {
		assert.Regexp("list failed.*list VSR error", err.Error())
	}
	assert.Equal(UnmountDone, state)
	os.RemoveAll(c.Args.MountDir)
}

func TestUnmountGetLoopbackDeviceInfo(t *testing.T) {
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
	fe := &fakeExec{assert: assert, cmd: "lsblk", argCount: 2, cmdResult: `{"invalid JSON" `}
	appCtx.exec = fe

	c := &unmountCmd{}
	c.ops = c
	res, err := c.getLoopbackDeviceInfo()
	assert.Nil(res)
	if assert.Error(err) {
		assert.Regexp("^lsblk output error:", err.Error())
	}

	fe = &fakeExec{assert: assert, cmd: "lsblk", argCount: 2, cmdErr: errors.New("you lose"), cmdResult: "verbose output"}
	appCtx.exec = fe
	res, err = c.getLoopbackDeviceInfo()
	assert.Nil(res)
	if assert.Error(err) {
		assert.Regexp("^lsblk failed:.*you lose", err.Error())
	}
	assert.Equal(1, tl.CountPattern("verbose output"))
}

func TestUnmountFileSystem(t *testing.T) {
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
	appCtx.ctx = context.Background()

	fe := &fakeExec{assert: assert, cmd: "umount", argCount: 2}
	appCtx.exec = fe
	c := &unmountCmd{}
	c.loopInfo = &LsBlkDevice{Name: "loop1"}
	c.ops = c
	assert.NoError(c.unmountFileSystem())
	assert.Equal("/dev/loop1", c.loopDev)

	fe.cmdResult, fe.cmdErr = "verbose output", errors.New("unmount fails")
	err := c.unmountFileSystem()
	if assert.Error(err) {
		assert.Regexp("umount -d failed:.*unmount fails", err.Error())
	}
	assert.Equal(1, tl.CountPattern("verbose output"))
}

func TestReadMountInfo(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	var b bytes.Buffer
	outputWriter = &b
	appCtx.ClusterID = "cluster-1"
	appCtx.NodeID = "node-1"
	appCtx.NodeIdentifier = "i-1234"
	appCtx.NuvoMountDir = "/mnt/nuvo"
	appCtx.SystemID = "system-1"
	appCtx.Log = tl.Logger()
	fc := &fake.Client{}
	appCtx.oCrud = fc
	appCtx.ctx = context.Background()
	c := &unmountCmd{}
	c.loopInfo = &LsBlkDevice{Name: "loop1"}
	c.Args.MountDir = "./tmpMount"
	c.ops = c

	os.RemoveAll(c.Args.MountDir)
	err := c.readMountInfo()
	if assert.Error(err) {
		_, ok := err.(*os.PathError)
		assert.True(ok)
	}

	mountInfo := &MountInfo{VolumeID: "vol-2", SnapIdentifier: "TAIL"}
	buf, err := json.Marshal(&mountInfo)
	assert.NoError(err)
	assert.NoError(os.Mkdir(c.Args.MountDir, 0755))
	assert.NoError(ioutil.WriteFile("./tmpMount/.nuvoMountInfo", buf, 0644))
	c.vsrTag = ""
	err = c.readMountInfo()
	assert.NoError(err)
	assert.Equal("vsr.creator:i-1234 UNMOUNT TAIL", c.vsrTag)

	assert.NoError(ioutil.WriteFile("./tmpMount/.nuvoMountInfo", []byte("this is not valid JSON"), 0644))
	c.vsrTag = ""
	err = c.readMountInfo()
	assert.Zero(c.vsrTag)
	if assert.Error(err) {
		assert.Regexp("invalid JSON in stash", err.Error())
	}
}

func TestCreateUnmountVSR(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	var b bytes.Buffer
	outputWriter = &b
	appCtx.ClusterID = "cluster-1"
	appCtx.NodeID = "node-1"
	appCtx.NodeIdentifier = "i-1234"
	appCtx.NuvoMountDir = "/mnt/nuvo"
	appCtx.SystemID = "system-1"
	appCtx.Log = tl.Logger()
	fc := &fake.Client{}
	appCtx.oCrud = fc
	appCtx.ctx = context.Background()
	c := &unmountCmd{}
	c.mountInfo.VolumeID = "vol-1"
	c.mountInfo.SnapIdentifier = "HEAD"
	c.vsrTag = "t-1"
	c.ops = c

	c.vsr = nil
	fc.PassThroughVRCObj = true
	assert.NoError(c.createUnmountVSR())
	if assert.NotNil(c.vsr) {
		assert.EqualValues("vol-1", c.vsr.VolumeSeriesID)
		assert.Equal("HEAD", c.vsr.SnapIdentifier)
		assert.EqualValues("node-1", c.vsr.NodeID)
		assert.EqualValues([]string{"t-1"}, c.vsr.SystemTags)
		assert.Equal([]string{"UNMOUNT"}, c.vsr.RequestedOperations)
	}

	fc.PassThroughVRCObj = false
	fc.RetVRCErr = errors.New("VSR create fails")
	err := c.createUnmountVSR()
	if assert.Error(err) {
		assert.Regexp("creating UNMOUNT", err.Error())
	}
}

func TestWaitForUmountVSR(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	var b bytes.Buffer
	outputWriter = &b
	appCtx.ClusterID = "cluster-1"
	appCtx.NodeID = "node-1"
	appCtx.NodeIdentifier = "i-1234"
	appCtx.NuvoMountDir = "/mnt/nuvo"
	appCtx.SystemID = "system-1"
	appCtx.Log = tl.Logger()
	fc := &fake.Client{}
	appCtx.oCrud = fc
	appCtx.ctx = context.Background()
	c := &unmountCmd{}
	c.loopInfo = &LsBlkDevice{Name: "loop1"}
	c.Args.MountDir = "./tmpMount"
	c.ops = c

	c.vsr = &models.VolumeSeriesRequest{}
	c.vsr.VolumeSeriesRequestState = "SUCCEEDED"
	assert.NoError(c.waitForUmountVSR())

	c.waitInterval = time.Microsecond
	inVSR := c.vsr
	c.vsr.VolumeSeriesRequestState = "NEW"
	c.vsr.Meta = &models.ObjMeta{ID: "vsr-1"}
	retVSR := &models.VolumeSeriesRequest{}
	retVSR.Meta = &models.ObjMeta{ID: "vsr-1"}
	retVSR.VolumeSeriesRequestState = "FAILED"
	fc.RetVRObj = retVSR
	err := c.waitForUmountVSR()
	if assert.Error(err) {
		assert.Regexp("vsr-1.*FAILED", err.Error())
	}
	assert.Equal(retVSR, c.vsr)

	fc.RetVRObj, fc.RetVRErr = nil, errors.New("VSR fetch fails")
	c.vsr = inVSR
	err = c.waitForUmountVSR()
	if assert.Error(err) {
		assert.Regexp("lookup failed.*fetch fails", err.Error())
	}
}

func TestRemoveMountStash(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	var b bytes.Buffer
	outputWriter = &b
	appCtx.Log = tl.Logger()
	c := &unmountCmd{}
	c.loopInfo = &LsBlkDevice{Name: "loop1"}
	c.Args.MountDir = "./tmpMount"
	c.ops = c

	os.RemoveAll(c.Args.MountDir)
	c.removeMountStash()
	assert.Equal(1, tl.CountPattern("failed to remove"))
	tl.Flush()

	assert.NoError(os.Mkdir(c.Args.MountDir, 0755))
	assert.NoError(ioutil.WriteFile("./tmpMount/.nuvoMountInfo", []byte("anything"), 0644))
	c.removeMountStash()
	assert.Equal(1, tl.CountPattern("removed.*nuvoMountInfo"))
	_, err := os.Stat("./tmpMount/.nuvoMountInfo")
	assert.Error(err)
	os.RemoveAll(c.Args.MountDir)
}

type fakeUnmountOps struct {
	called                []string
	getInitialStateErr    error
	getInitialStateResult unmountState
	unmountFileSystemErr  error
	readMountInfoErr      error
	createUnmountVSRErr   error
	waitForUnmountVSRErr  error
}

func (c *fakeUnmountOps) getInitialState() (unmountState, error) {
	c.called = append(c.called, "GIS")
	return c.getInitialStateResult, c.getInitialStateErr
}

func (c *fakeUnmountOps) unmountFileSystem() error {
	c.called = append(c.called, "UFS")
	return c.unmountFileSystemErr
}

func (c *fakeUnmountOps) readMountInfo() error {
	c.called = append(c.called, "RMI")
	return c.readMountInfoErr
}
func (c *fakeUnmountOps) createUnmountVSR() error {
	c.called = append(c.called, "CUV")
	return c.createUnmountVSRErr
}

func (c *fakeUnmountOps) waitForUmountVSR() error {
	c.called = append(c.called, "WUV")
	return c.waitForUnmountVSRErr
}

func (c *fakeUnmountOps) removeMountStash() {
	c.called = append(c.called, "RMS")
}
