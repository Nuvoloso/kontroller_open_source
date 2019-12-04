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
	"regexp"
	"strings"
	"time"

	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
)

func init() {
	long := `Mount the volume at the MOUNT-DIR. The JSON-OPTIONS provide additional context.

	The supported fsTypes values are "ext4" and "xfs". Currently, the readOnly flag is not supported.

	In addition to the standard flexVolume JSON-OPTIONS, the nuvo driver expects:
	nuvoSystemId: The identifier of the Nuvoloso system
	nuvoVolumeId: The identifier of the existing, bound Nuvoloso volume series to mount
	nuvoSnapshot: Optional. A snapshot identifier, defaults to the volume series HEAD`
	cmd := &mountCmd{}
	cmd.ops = cmd // self-reference
	cmd.waitInterval = 10 * time.Second
	parser.AddCommand("mount", "Mount command", long, cmd)
}

const (
	ext4 = "ext4"
	xfs  = "xfs"

	blockAndSectorSize = "4096"

	// MountStash is a file created in the mountDir before the file system is mounted on top, and stores the MountInfo needed by unmount
	MountStash = ".nuvoMountInfo"
)

// JSONOptions represents the expected properties of the json-options command line argument
type JSONOptions struct {
	FsType    *string `json:"kubernetes.io/fsType"`
	ReadWrite *string `json:"kubernetes.io/readwrite"`
	Snapshot  *string `json:"nuvoSnapshot"`
	SystemID  string  `json:"nuvoSystemId"`
	VolumeID  string  `json:"nuvoVolumeId"`
}

// LsBlkDevice represents a single device in the lsblk output (some fields ignored)
type LsBlkDevice struct {
	Name       string  `json:"name"` // device name, without the /dev/ prefix
	FsType     *string `json:"fstype"`
	MountPoint *string `json:"mountpoint"`
}

// MountInfo is used to record information about the mounted volume for use by later unmount command
type MountInfo struct {
	VolumeID       string `json:"volumeId"`
	SnapIdentifier string `json:"snapIdentifier"`
}

// LsBlkOutput represents the expected output of the lsblk -fJ command
type LsBlkOutput struct {
	BlockDevices []*LsBlkDevice `json:"blockdevices"`
}

type mountOps interface {
	getInitialState() (mountState, error)
	createMountVSR() error
	waitForVSR() error
	createLoopback() error
	fileSystemExists() (bool, error)
	makeFileSystem() error
	recordMountInfo() error
	mountFileSystem() error
}

type mountCmd struct {
	Args struct {
		MountDir    string `positional-arg-name:"MOUNT-DIR"`
		JSONOptions string `positional-arg-name:"JSON-OPTIONS"`
	} `positional-args:"yes" required:"yes"`

	ops          mountOps
	options      *JSONOptions
	vs           *models.VolumeSeries
	vsr          *models.VolumeSeriesRequest
	waitInterval time.Duration
	vsrTag       string       // the tag used to identify VSR created by driver on this node
	nuvoVolPath  string       // path of the exported nuvoVol LUN
	loopDev      string       // the loopback device path
	loopInfo     *LsBlkDevice // info about the loopback block device
}

// mountState represents states in the flexVolume mount process.
// There is at most one database update in each sub-state.
type mountState int

// mountState values
const (
	// Create a VSR to MOUNT the volume/snapshot
	CreateMountVSR mountState = iota
	// Wait for the VSR to complete
	WaitForVSR
	// Create a loopback device over the MountedNodeDevice
	CreateLoopback
	// Check if a filesystem already exists
	CheckForFileSystem
	// Make a filesystem on the device
	MakeFileSystem
	// Record information about the mount to be used by eventual unmount (and create mountDir if necessary)
	RecordMountInfo
	// Mount the filesystem
	MountFileSystem
	// Final State
	MountDone
)

func (c *mountCmd) Execute(args []string) error {
	c.options = &JSONOptions{}
	var err error
	if err = json.Unmarshal([]byte(c.Args.JSONOptions), &c.options); err != nil {
		return PrintDriverOutput(DriverStatusFailure, "invalid json-options: "+err.Error())
	}
	if c.options.SystemID != appCtx.SystemID {
		return PrintDriverOutput(DriverStatusFailure, fmt.Sprintf("incorrect systemId [%s]", c.options.SystemID))
	}
	if c.options.ReadWrite != nil && *c.options.ReadWrite != "rw" {
		return PrintDriverOutput(DriverStatusFailure, "readOnly mount is not supported")
	}

	if c.options.FsType == nil {
		var empty string
		c.options.FsType = &empty
	} else if !util.Contains([]string{ext4, xfs}, *c.options.FsType) {
		return PrintDriverOutput(DriverStatusFailure, fmt.Sprintf("unsupported fsType: %s", *c.options.FsType))
	}
	if c.options.Snapshot == nil || *c.options.Snapshot == "" {
		head := com.VolMountHeadIdentifier
		c.options.Snapshot = &head
	}
	c.vsrTag = fmt.Sprintf("%s:%s %s %s", com.SystemTagVsrCreator, appCtx.NodeIdentifier, com.VolReqOpMount, *c.options.Snapshot)

	if c.vs, err = appCtx.oCrud.VolumeSeriesFetch(appCtx.ctx, c.options.VolumeID); err != nil {
		return PrintDriverOutput(DriverStatusFailure, fmt.Sprintf("volume [%s] lookup failed: %s", c.options.VolumeID, err.Error()))
	}
	if string(c.vs.BoundClusterID) != appCtx.ClusterID {
		return PrintDriverOutput(DriverStatusFailure, fmt.Sprintf("volume [%s] is not bound to cluster [%s]", c.options.VolumeID, appCtx.ClusterID))
	}
	var state mountState
	for state, err = c.ops.getInitialState(); state < MountDone && err == nil; state++ {
		switch state {
		case CreateMountVSR:
			err = c.ops.createMountVSR()
		case WaitForVSR:
			err = c.ops.waitForVSR()
		case CreateLoopback:
			err = c.ops.createLoopback()
		case CheckForFileSystem:
			var exists bool
			if exists, err = c.ops.fileSystemExists(); exists {
				state++ // skip MakeFileSystem
			}
		case MakeFileSystem:
			err = c.ops.makeFileSystem()
		case RecordMountInfo:
			err = c.ops.recordMountInfo()
		case MountFileSystem:
			err = c.ops.mountFileSystem()
		}
	}
	if err != nil {
		return PrintDriverOutput(DriverStatusFailure, err.Error())
	}
	return PrintDriverOutput(DriverStatusSuccess)
}

func (c *mountCmd) getInitialState() (mountState, error) {
	if dev, err := appCtx.IsMounted(c.Args.MountDir); err != nil || dev != "" {
		return MountDone, err
	}
	// if there is an outstanding VSR created by this driver for this VS, wait for it
	params := &volume_series_request.VolumeSeriesRequestListParams{
		IsTerminated:   swag.Bool(false),
		VolumeSeriesID: &c.options.VolumeID,
		NodeID:         &appCtx.NodeID,
		SystemTags:     []string{c.vsrTag},
	}
	vrl, err := appCtx.oCrud.VolumeSeriesRequestList(appCtx.ctx, params)
	if err != nil {
		return MountDone, fmt.Errorf("volume-series-request list failed: %s", err.Error())
	}
	if len(vrl.Payload) > 0 {
		c.vsr = vrl.Payload[0]
		return WaitForVSR, nil
	}
	if c.vs.VolumeSeriesState == com.VolStateInUse {
		// This is OK if the volume/snapshot is not mounted anywhere or is mounted on this node,
		// and if it has a loopback device over it, there is either no mount point or the expected mount point
		if lun := c.findSnapLUN(); lun != nil {
			if string(lun.MountedNodeID) != appCtx.NodeID {
				return MountDone, fmt.Errorf("volume [%s,%s] is already in use on node [%s]", c.options.VolumeID, *c.options.Snapshot, lun.MountedNodeID)
			}
			// check if loopback device already exists
			c.nuvoVolPath = filepath.Join(appCtx.NuvoMountDir, lun.MountedNodeDevice)
			output, err := appCtx.exec.CommandContext(appCtx.ctx, "losetup", "-j", c.nuvoVolPath).CombinedOutput()
			if err != nil {
				return MountDone, fmt.Errorf("losetup -j failed: %s", err.Error())
			}
			if len(output) > 0 {
				re := regexp.MustCompile(`^(/dev/loop[0-9]+): `)
				if match := re.FindSubmatch(output); len(match) > 0 {
					c.loopDev = string(match[1])
					if c.loopInfo, err = c.getLoopbackDeviceInfo(); err != nil {
						return MountDone, err
					}
					if c.loopInfo != nil {
						if c.loopInfo.MountPoint != nil {
							if *c.loopInfo.MountPoint != c.Args.MountDir {
								return MountDone, fmt.Errorf("volume/snapshot is already in use on %s [%s]", c.loopDev, *c.loopInfo.MountPoint)
							}
							return MountDone, nil
						}
						if c.loopInfo.FsType != nil {
							return MountFileSystem, nil
						}
						return MakeFileSystem, nil
					}
				}
			}
		}
		return CreateLoopback, nil
	}
	if c.vs.VolumeSeriesState != com.VolStateBound && c.vs.VolumeSeriesState != com.VolStateProvisioned {
		return MountDone, fmt.Errorf("volume [%s] state is not mountable: %s", c.options.VolumeID, c.vs.VolumeSeriesState)
	}
	return CreateMountVSR, nil
}

func (c *mountCmd) findSnapLUN() *models.Mount {
	for _, lun := range c.vs.Mounts {
		if lun.SnapIdentifier == *c.options.Snapshot {
			return lun
		}
	}
	return nil
}

func (c *mountCmd) getLoopbackDeviceInfo() (*LsBlkDevice, error) {
	output, err := appCtx.exec.CommandContext(appCtx.ctx, "lsblk", "-f", "-J", c.loopDev).CombinedOutput() // get output in JSON
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
	if len(blk.BlockDevices) > 0 {
		dev := blk.BlockDevices[0]
		if dev.FsType != nil {
			if *c.options.FsType == "" {
				c.options.FsType = dev.FsType
			} else if *c.options.FsType != *dev.FsType {
				return dev, fmt.Errorf("volume is formatted with fsType [%s] expected [%s]", *dev.FsType, *c.options.FsType)
			}
		}
		return dev, nil
	}
	return nil, nil
}

func (c *mountCmd) createMountVSR() error {
	c.vsr = &models.VolumeSeriesRequest{}
	c.vsr.VolumeSeriesID = models.ObjIDMutable(c.vs.Meta.ID)
	c.vsr.SnapIdentifier = *c.options.Snapshot
	c.vsr.NodeID = models.ObjIDMutable(appCtx.NodeID)
	c.vsr.SystemTags = []string{c.vsrTag}
	c.vsr.RequestedOperations = []string{com.VolReqOpMount}
	c.vsr.CompleteByTime = strfmt.DateTime(time.Now().Add(5 * time.Minute))
	vsr, err := appCtx.oCrud.VolumeSeriesRequestCreate(appCtx.ctx, c.vsr)
	if err != nil {
		return fmt.Errorf("error creating MOUNT volume-series-request: %s", err.Error())
	}
	c.vsr = vsr
	return nil
}

func (c *mountCmd) waitForVSR() error {
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

	var err error
	if c.vs, err = appCtx.oCrud.VolumeSeriesFetch(appCtx.ctx, c.options.VolumeID); err != nil {
		return fmt.Errorf("volume [%s] lookup failed: %s", c.options.VolumeID, err.Error())
	}
	lun := c.findSnapLUN()
	if lun == nil {
		return fmt.Errorf("volume [%s,%s] LUN not found", c.options.VolumeID, *c.options.Snapshot)
	}
	c.nuvoVolPath = filepath.Join(appCtx.NuvoMountDir, lun.MountedNodeDevice)
	appCtx.Log.Debugf("volume-series-request [%s] LUN mounted at %s", c.vsr.Meta.ID, c.nuvoVolPath)
	return nil
}

func (c *mountCmd) createLoopback() error {
	output, err := appCtx.exec.CommandContext(appCtx.ctx, "losetup", "--find", "--show", c.nuvoVolPath).CombinedOutput()
	if err != nil {
		if len(output) > 0 {
			appCtx.Log.Error(string(output))
		}
		return fmt.Errorf("losetup failed: %s", err.Error())
	}
	c.loopDev = strings.TrimSpace(string(output))
	// cannot run lsblk immediately after losetup because it only reads cached data that may not be present
	// blkid reads the device directly and always returns valid data on success (but requires screen scraping)
	output, err = appCtx.exec.CommandContext(appCtx.ctx, "blkid", c.loopDev).CombinedOutput()
	if err != nil {
		// blkid exits with an error if the device does not contain a formatted filesystem, so ignore the error
		appCtx.Log.Debugf("blkid %s error: %s", c.loopDev, err.Error())
	} else {
		re := regexp.MustCompile(`^/dev/(loop[0-9]+): .*TYPE="([^ ]+)"`)
		if match := re.FindSubmatch(output); len(match) > 0 {
			name := string(match[1])
			fsType := string(match[2])
			c.loopInfo = &LsBlkDevice{Name: name, FsType: &fsType} // cannot possibly be mounted yet, we haven't mounted it
		}
	}
	return nil
}

func (c *mountCmd) fileSystemExists() (bool, error) {
	if c.loopInfo == nil {
		var err error
		if c.loopInfo, err = c.getLoopbackDeviceInfo(); err != nil {
			return false, err
		}
	}
	return c.loopInfo != nil && c.loopInfo.FsType != nil, nil
}

func (c *mountCmd) makeFileSystem() error {
	var cmd Cmd
	switch *c.options.FsType {
	case ext4:
		os.Setenv("MKE2FS_DEVICE_SECTSIZE", blockAndSectorSize)
		cmd = appCtx.exec.CommandContext(appCtx.ctx, "mkfs.ext4", "-b", blockAndSectorSize, "-U", string(c.vs.Meta.ID), c.loopDev)
	default:
		uuid := fmt.Sprintf("uuid=%s", c.vs.Meta.ID)
		bSize := fmt.Sprintf("size=%s", blockAndSectorSize)
		cmd = appCtx.exec.CommandContext(appCtx.ctx, "mkfs.xfs", "-b", bSize, "-s", bSize, "-m", uuid, c.loopDev)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		if len(output) > 0 {
			appCtx.Log.Error(string(output))
		}
		return fmt.Errorf("mkfs failed: %s", err.Error())
	} else if len(output) > 0 {
		appCtx.Log.Info(string(output))
	}
	return err
}

func (c *mountCmd) recordMountInfo() error {
	if info, err := os.Stat(c.Args.MountDir); err != nil || !info.IsDir() {
		// prefer result of MkdirAll over whatever Stat might return
		if err = os.MkdirAll(c.Args.MountDir, 0755); err != nil {
			return fmt.Errorf("directory [%s] creation failed: %s", c.Args.MountDir, err.Error())
		}
		appCtx.Log.Infof("mount: created mount-dir [%s]", c.Args.MountDir)
	} else {
		appCtx.Log.Debugf("mount: mount-dir exists [%s]", c.Args.MountDir)
	}

	mountStash := filepath.Join(c.Args.MountDir, MountStash)
	mountInfo := &MountInfo{VolumeID: c.options.VolumeID, SnapIdentifier: *c.options.Snapshot}
	buf, _ := json.Marshal(mountInfo)
	err := ioutil.WriteFile(mountStash, buf, 0644)
	if err != nil {
		return fmt.Errorf("mount stash [%s] creation failed: %s", mountStash, err.Error())
	}
	return nil
}

func (c *mountCmd) mountFileSystem() error {
	output, err := appCtx.exec.CommandContext(appCtx.ctx, "mount", c.loopDev, c.Args.MountDir).CombinedOutput()
	if err != nil {
		if len(output) > 0 {
			appCtx.Log.Error(string(output))
		}
		return fmt.Errorf("mount failed: %s", err.Error())
	} else if len(output) > 0 {
		appCtx.Log.Info(string(output))
	}
	return err
}
