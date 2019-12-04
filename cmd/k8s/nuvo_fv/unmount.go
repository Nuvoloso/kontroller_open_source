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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

func init() {
	cmd := &unmountCmd{}
	cmd.ops = cmd // self-reference
	cmd.waitInterval = 10 * time.Second
	parser.AddCommand("unmount", "Unmount command", "Unmount the volume at the MOUNT-DIR", cmd)
}

type unmountOps interface {
	getInitialState() (unmountState, error)
	unmountFileSystem() error
	readMountInfo() error
	createUnmountVSR() error
	waitForUmountVSR() error
	removeMountStash()
}

type unmountCmd struct {
	Args struct {
		MountDir string `positional-arg-name:"MOUNT-DIR"`
	} `positional-args:"yes" required:"yes"`

	ops          unmountOps
	vsr          *models.VolumeSeriesRequest
	waitInterval time.Duration
	vsrTag       string       // the tag used to identify VSR created by driver on this node
	loopDev      string       // the loopback device path
	loopInfo     *LsBlkDevice // info about the loopback block device
	mountInfo    MountInfo    // info about the nuvo vol read from from the mount stash
}

// unmountState represents states in the flexVolume unmount process
type unmountState int

// unmountState values
const (
	// Interact with OS to unmount the file system
	UnmountFileSystem unmountState = iota
	// Read the MountInfo stash previously hidden under the mounted file system
	ReadMountInfo
	// Create a VSR to UNMOUNT the volume/snapshot
	CreateUnmountVSR
	// Wait for the VSR to complete
	WaitForUnmountVSR
	// Remove the MountInfo stash
	RemoveMountStash
	// Final State
	UnmountDone
)

func (c *unmountCmd) Execute(args []string) error {
	var state unmountState
	var err error
	for state, err = c.ops.getInitialState(); state < UnmountDone && err == nil; state++ {
		switch state {
		case UnmountFileSystem:
			err = c.ops.unmountFileSystem()
		case ReadMountInfo:
			err = c.ops.readMountInfo()
		case CreateUnmountVSR:
			err = c.ops.createUnmountVSR()
		case WaitForUnmountVSR:
			err = c.ops.waitForUmountVSR()
		case RemoveMountStash:
			c.ops.removeMountStash()
		}
	}
	if err != nil {
		return PrintDriverOutput(DriverStatusFailure, err.Error())
	}
	return PrintDriverOutput(DriverStatusSuccess)
}

func (c *unmountCmd) getInitialState() (unmountState, error) {
	var err error
	c.loopInfo, err = c.getLoopbackDeviceInfo()
	if err != nil {
		return UnmountDone, err
	} else if c.loopInfo == nil {
		if err = c.readMountInfo(); err != nil {
			// error reading the stash means nothing is left to be done
			appCtx.Log.Debugf("error reading mount stash ignored: %s", err.Error())
			return UnmountDone, nil
		}
		// if there is an outstanding VSR created by this driver for this VS, wait for it
		params := &volume_series_request.VolumeSeriesRequestListParams{
			IsTerminated:   swag.Bool(false),
			VolumeSeriesID: swag.String(c.mountInfo.VolumeID),
			NodeID:         &appCtx.NodeID,
			SystemTags:     []string{c.vsrTag},
		}
		vrl, err := appCtx.oCrud.VolumeSeriesRequestList(appCtx.ctx, params)
		if err != nil {
			return UnmountDone, fmt.Errorf("volume-series-request list failed: %s", err.Error())
		}
		if len(vrl.Payload) > 0 {
			c.vsr = vrl.Payload[0]
			return WaitForUnmountVSR, nil
		}
		// is VS already unmounted or maybe even deleted? Can happen if k8s state is out of sync with nuvo state due to earlier failure
		vs, err := appCtx.oCrud.VolumeSeriesFetch(appCtx.ctx, c.mountInfo.VolumeID)
		if err == nil {
			if vs.VolumeSeriesState != com.VolStateInUse {
				appCtx.Log.Debugf("volume [%s] already unmounted", c.mountInfo.VolumeID)
				return RemoveMountStash, nil
			}
		} else {
			if e, ok := err.(*crud.Error); ok && e.NotFound() {
				appCtx.Log.Debugf("volume [%s] no longer exists", c.mountInfo.VolumeID)
				return RemoveMountStash, nil
			}
			return UnmountDone, fmt.Errorf("volume [%s] lookup failed: %s", c.mountInfo.VolumeID, err.Error())
		}
		return CreateUnmountVSR, nil
	}
	return UnmountFileSystem, nil
}

// getLoopbackDeviceInfo gets the device info given the mount-dir
func (c *unmountCmd) getLoopbackDeviceInfo() (*LsBlkDevice, error) {
	output, err := appCtx.exec.CommandContext(appCtx.ctx, "lsblk", "-f", "-J").CombinedOutput() // get output in JSON
	if err != nil {
		if len(output) > 0 {
			appCtx.Log.Error(string(output))
		}
		return nil, fmt.Errorf("lsblk failed: %s", err.Error())
	}
	blk := &LsBlkOutput{}
	if err = json.Unmarshal(output, blk); err != nil {
		return nil, fmt.Errorf("lsblk output error: %s", err.Error())
	}
	for _, dev := range blk.BlockDevices {
		if dev.MountPoint != nil && *dev.MountPoint == c.Args.MountDir {
			return dev, nil
		}
	}
	return nil, nil
}

func (c *unmountCmd) unmountFileSystem() error {
	c.loopDev = filepath.Join("/dev", c.loopInfo.Name)
	// the -d flag causes the loopback device to be detached as well, no need to execute losetup -d separately
	output, err := appCtx.exec.CommandContext(appCtx.ctx, "umount", "-d", c.loopDev).CombinedOutput()
	if err != nil {
		if len(output) > 0 {
			appCtx.Log.Error(string(output))
		}
		return fmt.Errorf("umount -d failed: %s", err.Error())
	}
	return nil
}

func (c *unmountCmd) readMountInfo() error {
	mountStash := filepath.Join(c.Args.MountDir, MountStash)
	buf, err := ioutil.ReadFile(mountStash)
	if err != nil {
		return err
	}
	err = json.Unmarshal(buf, &c.mountInfo)
	if err != nil {
		return fmt.Errorf("invalid JSON in stash [%s]: %s", mountStash, err.Error())
	}
	c.vsrTag = fmt.Sprintf("%s:%s %s %s", com.SystemTagVsrCreator, appCtx.NodeIdentifier, com.VolReqOpUnmount, c.mountInfo.SnapIdentifier)
	return nil
}

func (c *unmountCmd) createUnmountVSR() error {
	c.vsr = &models.VolumeSeriesRequest{}
	c.vsr.VolumeSeriesID = models.ObjIDMutable(c.mountInfo.VolumeID)
	c.vsr.SnapIdentifier = c.mountInfo.SnapIdentifier
	c.vsr.NodeID = models.ObjIDMutable(appCtx.NodeID)
	c.vsr.SystemTags = []string{c.vsrTag}
	c.vsr.RequestedOperations = []string{com.VolReqOpUnmount}
	c.vsr.CompleteByTime = strfmt.DateTime(time.Now().Add(5 * time.Minute))
	vsr, err := appCtx.oCrud.VolumeSeriesRequestCreate(appCtx.ctx, c.vsr)
	if err != nil {
		return fmt.Errorf("error creating UNMOUNT volume-series-request: %s", err.Error())
	}
	c.vsr = vsr
	return nil
}

func (c *unmountCmd) waitForUmountVSR() error {
	for !vra.VolumeSeriesRequestStateIsTerminated(c.vsr.VolumeSeriesRequestState) {
		select {
		// TBD cancel conditions
		case <-time.After(c.waitInterval):
			// wait
		}
		vsr, err := appCtx.oCrud.VolumeSeriesRequestFetch(appCtx.ctx, string(c.vsr.Meta.ID))
		if err != nil {
			return fmt.Errorf("volume-series-request [%s] lookup failed: %s", c.vsr.Meta.ID, err.Error())
		}
		c.vsr = vsr
	}
	if c.vsr.VolumeSeriesRequestState != com.VolReqStateSucceeded {
		return fmt.Errorf("volume-series-request [%s] %s", c.vsr.Meta.ID, c.vsr.VolumeSeriesRequestState)
	}
	return nil
}

func (c *unmountCmd) removeMountStash() {
	mountStash := filepath.Join(c.Args.MountDir, MountStash)
	// best effort
	if err := os.Remove(mountStash); err != nil {
		appCtx.Log.Warningf("unmount: failed to remove [%s] continuing: %s", mountStash, err.Error())
	} else {
		appCtx.Log.Debugf("umount: removed [%s]", mountStash)
	}
}
