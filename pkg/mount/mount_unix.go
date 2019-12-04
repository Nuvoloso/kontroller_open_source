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

// +build linux darwin

// Loosely based on code in k8s.io/pkg/mount/mount_linux.go
// Kubernetes copyright at the time of writing:
/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mount

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/Nuvoloso/kontroller/pkg/util"
)

// Linux specific constants
const (
	// The default file system type
	DefaultFsType = "ext4"
	// 'fsck' found errors and corrected them
	fsckErrorsCorrected = 1
	// 'fsck' found errors but exited without correcting them
	fsckErrorsUncorrected = 4
)

var errWaitForDir = fmt.Errorf("waitForDirErr")

// mounter implements Mounter on linux (compiles on darwin for development unit testing)
type mounter struct {
	MounterArgs
	exec   util.Exec
	ops    mops
	mntCnt int
}

// internal operations
type mops interface {
	doFsck(ctx context.Context, args *FilesystemMountArgs) error
	doMount(ctx context.Context, args *FilesystemMountArgs) error
	doMkfs(ctx context.Context, args *FilesystemMountArgs) error
	getFsType(ctx context.Context, args *FilesystemMountArgs) (string, error)
	waitForDir(ctx context.Context, args *FilesystemMountArgs) error
	doMountWithWait(ctx context.Context, args *FilesystemMountArgs) error
}

// New returns a Linux implementation of Mounter
func New(ma *MounterArgs) (Mounter, error) {
	if err := ma.Validate(); err != nil {
		return nil, err
	}
	m := &mounter{MounterArgs: *ma}
	m.exec = util.NewExec()
	m.ops = m // self-reference
	return m, nil
}

// MountFilesystem is part of the Mounter interface.
// It mounts a Nuvo volume, creating a filesystem on it if necessary when writable.
// If a file system is already present it will run fsck on it.
// A loopback device is used for the mount.
// See formatAndMount() in k8s.io/pkg/mount/mount_linux.go.
func (m *mounter) MountFilesystem(ctx context.Context, args *FilesystemMountArgs) error {
	var err error
	if err = args.Validate(); err != nil {
		return err
	}
	if args.FsType == "" {
		args.FsType = DefaultFsType
	}
	readOnly := m.isReadOnly(args)
	if !readOnly {
		err = m.ops.doFsck(ctx, args) // fails only on uncorrected fsck errors
	}
	if err == nil {
		if mntErr := m.ops.doMountWithWait(ctx, args); mntErr == context.DeadlineExceeded || mntErr == errWaitForDir {
			err = mntErr
		} else if mntErr != nil { // unformatted or unexpected file system, or a genuine mount error
			m.Log.Debugf("%s: attempting to format %s", args.LogPrefix, args.Source)
			var fsType string
			fsType, err = m.ops.getFsType(ctx, args)
			if err == nil {
				if fsType == "" { // unformatted
					if readOnly {
						err = fmt.Errorf("cannot mount unformatted volume read-only")
					} else if err = m.ops.doMkfs(ctx, args); err == nil {
						if m.Rei != nil {
							err = m.Rei.ErrOnBool("fail-after-mkfs")
						}
						if err == nil {
							err = m.ops.doMountWithWait(ctx, args)
						}
					}
				} else if args.FsType != fsType { // unexpected file system
					err = fmt.Errorf("expected file system type %s found %s", args.FsType, fsType)
				} else {
					err = mntErr // genuine mount error
				}
			}
		}
	}
	if err != nil {
		m.Log.Errorf("%s: mount of %s failed: %v", args.LogPrefix, args.Source, err)
	}
	return err
}

func (m *mounter) isReadOnly(args *FilesystemMountArgs) bool {
	for _, o := range args.Options {
		if o == "ro" {
			return true
		}
	}
	return false
}

func (m *mounter) debugCmd(cmdBuf *bytes.Buffer, prefix, cmd string, args []string) {
	var b bytes.Buffer
	fmt.Fprintf(&b, "%s", cmd)
	for _, a := range args {
		fmt.Fprintf(&b, " %q", a)
	}
	cmdLine := b.String()
	if cmdBuf != nil {
		fmt.Fprintf(cmdBuf, "%s\n", cmdLine)
	}
	m.Log.Debugf("%s: %s", prefix, cmdLine)
}

// doFsck runs fsck on a writable source file.
func (m *mounter) doFsck(ctx context.Context, args *FilesystemMountArgs) error {
	opts := []string{"-a", args.Source}
	m.debugCmd(args.Commands, args.LogPrefix, "fsck", opts)
	if args.PlanOnly {
		return nil
	}
	ctx, cancel := context.WithDeadline(ctx, args.Deadline)
	defer cancel()
	cmd := m.exec.CommandContext(ctx, "fsck", opts...)
	bytes, err := cmd.CombinedOutput()
	m.Log.Debugf("%s: fsck output:\n%s", args.LogPrefix, string(bytes))
	if err != nil {
		rc, isExitError := m.exec.IsExitError(err)
		switch {
		case err == context.DeadlineExceeded:
			m.Log.Errorf("%s: error running 'fsck': %v", args.LogPrefix, err)
			return err
		case m.exec.IsNotFoundError(err):
			m.Log.Warningf("%s: 'fsck' not found on system; continuing mount without running 'fsck'", args.LogPrefix)
		case isExitError && rc == fsckErrorsCorrected:
			m.Log.Infof("%s: Device %s has errors which were corrected by fsck", args.LogPrefix, args.Source)
		case isExitError && rc == fsckErrorsUncorrected:
			return fmt.Errorf("'fsck' found errors on device but could not correct them")
		case isExitError && rc > fsckErrorsUncorrected:
			m.Log.Infof("%s: `fsck` errors on media - ignored", args.LogPrefix)
		default:
			m.Log.Infof("%s: error running 'fsck': %v - ignored", args.LogPrefix, err)
		}
	}
	return nil
}

// doMountWithWait checks to see if the mountpoint exists and attempts a mount.
// If it fails to do a mount on account of mountpoint being deleted; it will wait to
// sync with the mountpoint creation cycle and try again.
func (m *mounter) doMountWithWait(ctx context.Context, args *FilesystemMountArgs) error {
	if args.PlanOnly {
		return m.ops.doMount(ctx, args)
	}
Start:
	if err := m.ops.waitForDir(ctx, args); err != nil {
		m.Log.Errorf("error waiting for mount point to become available: %s", err.Error())
		return errWaitForDir
	}
	if err := m.ops.doMount(ctx, args); err != nil {
		if _, sErr := os.Stat(args.Target); os.IsNotExist(sErr) {
			m.Log.Debugf("mount point removed, retrying mount")
			goto Start
		}
		return err
	}
	return nil
}

// doMount attempts to do a loopback mount.
// Will fail if no filesystem on the source.
// Runs:
//   mount -t fstype -o [options,]loop,defaults source target
func (m *mounter) doMount(ctx context.Context, args *FilesystemMountArgs) error {
	m.mntCnt++
	opts := make([]string, 0, len(args.Options)+2)
	for _, o := range args.Options {
		opts = append(opts, o)
	}
	opts = append(opts, "loop", "defaults")
	cmdArgs := []string{
		"-t", args.FsType,
		"-o", strings.Join(opts, ","),
		args.Source, args.Target,
	}
	m.debugCmd(args.Commands, args.LogPrefix, "mount", cmdArgs)
	if args.PlanOnly {
		if m.mntCnt == 1 {
			return fmt.Errorf("fake error")
		}
		return nil
	}
	ctx, cancel := context.WithDeadline(ctx, args.Deadline)
	defer cancel()
	cmd := m.exec.CommandContext(ctx, "mount", cmdArgs...)
	bytes, err := cmd.CombinedOutput()
	m.Log.Debugf("%s: mount output:\n%s", args.LogPrefix, string(bytes))
	if err != nil {
		m.Log.Errorf("%s: mount failed: %v", args.LogPrefix, err)
		return err
	}
	return nil
}

// doMkfs attempts to create the filesystem.
func (m *mounter) doMkfs(ctx context.Context, args *FilesystemMountArgs) error {
	cmdArgs := []string{"-t", args.FsType}
	if args.FsType == "ext4" {
		cmdArgs = append(cmdArgs, "-E", "lazy_itable_init=0,lazy_journal_init=0")
	}
	cmdArgs = append(cmdArgs, args.Source)
	m.debugCmd(args.Commands, args.LogPrefix, "mkfs", cmdArgs)
	if args.PlanOnly {
		return nil
	}
	ctx, cancel := context.WithDeadline(ctx, args.Deadline)
	defer cancel()
	cmd := m.exec.CommandContext(ctx, "mkfs", cmdArgs...)
	bytes, err := cmd.CombinedOutput()
	m.Log.Debugf("%s: mkfs output:\n%s", args.LogPrefix, string(bytes))
	if err != nil {
		m.Log.Errorf("%s: mkfs failed: %v", args.LogPrefix, err)
		return err
	}
	return nil
}

// getFsType returns the file system type
func (m *mounter) getFsType(ctx context.Context, args *FilesystemMountArgs) (string, error) {
	cmdArgs := []string{"-p", "-s", "TYPE", "-o", "export", args.Source}
	m.debugCmd(args.Commands, args.LogPrefix, "blkid", cmdArgs)
	if args.PlanOnly {
		return "", nil
	}
	ctx, cancel := context.WithDeadline(ctx, args.Deadline)
	defer cancel()
	cmd := m.exec.CommandContext(ctx, "blkid", cmdArgs...)
	bytes, err := cmd.CombinedOutput()
	output := string(bytes)
	m.Log.Debugf("%s: blkid output:\n%s", args.LogPrefix, output)
	if err != nil {
		rc, isExitError := m.exec.IsExitError(err)
		if isExitError && rc == 2 {
			m.Log.Debugf("%s: device %s is not formatted", args.LogPrefix, args.Source)
			return "", nil
		}
		m.Log.Errorf("%s: blkid failed: %v", args.LogPrefix, err)
		return "", err
	}
	for _, l := range strings.Split(output, "\n") {
		if len(l) <= 0 {
			continue
		}
		fields := strings.SplitN(l, "=", 2)
		if fields[0] == "TYPE" {
			m.Log.Debugf("%s: device %s file system is %s", args.LogPrefix, args.Source, fields[1])
			return fields[1], nil
		}
	}
	return "", nil
}

// UnmountFilesystem is part of the Mounter interface.
// It dismounts a previously mounted NuvoVolume with a filesystem.
// The loopback device is released.
func (m *mounter) UnmountFilesystem(ctx context.Context, args *FilesystemUnmountArgs) error {
	if err := args.Validate(); err != nil {
		return err
	}
	cmdArgs := []string{"-d", args.Target}
	m.debugCmd(args.Commands, args.LogPrefix, "umount", cmdArgs)
	if args.PlanOnly {
		return nil
	}
	ctx, cancel := context.WithDeadline(ctx, args.Deadline)
	defer cancel()
	cmd := m.exec.CommandContext(ctx, "umount", cmdArgs...)
	bytes, err := cmd.CombinedOutput()
	m.Log.Debugf("%s: umount output:\n%s", args.LogPrefix, string(bytes))
	if err != nil {
		m.Log.Errorf("%s: umount failed: %v", args.LogPrefix, err)
		return err
	}
	return nil
}

// regex to match a mount record of interest
var reUnixMountRec = regexp.MustCompile("^(.*) on (.*) type (\\w+) \\((.*)\\)")

// IsFilesystemMounted is part of the Mounter interface.
// It runs "mount" to fetch and check all the filesystems visible to the process.
// It can detect mismatch of target, filesystem type and mode.
func (m *mounter) IsFilesystemMounted(ctx context.Context, args *FilesystemMountArgs) (bool, error) {
	var err error
	if err = args.Validate(); err != nil {
		return false, err
	}
	if args.FsType == "" {
		args.FsType = DefaultFsType
	}
	if args.PlanOnly {
		return false, nil
	}
	expOp := "rw"
	if m.isReadOnly(args) {
		expOp = "ro"
	}
	m.debugCmd(args.Commands, args.LogPrefix, "mount", []string{})
	ctx, cancel := context.WithDeadline(ctx, args.Deadline)
	defer cancel()
	cmd := m.exec.CommandContext(ctx, "mount")
	bytes, err := cmd.CombinedOutput()
	if err != nil {
		m.Log.Errorf("%s: mount error: %v", args.LogPrefix, err)
		return false, err
	}
	for _, rec := range strings.Split(string(bytes), "\n") {
		if res := reUnixMountRec.FindAllStringSubmatch(rec, -1); res != nil {
			source, target, fsType, opts := res[0][1], res[0][2], res[0][3], res[0][4]
			if source != args.Source {
				continue
			}
			if target == args.Target && fsType == args.FsType && util.Contains(strings.Split(opts, ","), expOp) {
				m.Log.Debugf("%s: \"%s\" is mounted on \"%s\"", args.LogPrefix, args.Source, args.Target)
				return true, nil
			}
			return false, fmt.Errorf("filesystem target, fstype or mode mismatch: expected[%s,%s,(%s,*)] found[%s,%s,(%s)]", args.Target, args.FsType, expOp, target, fsType, opts)
		}
	}
	m.Log.Debugf("%s: \"%s\" is not mounted", args.LogPrefix, args.Source)
	return false, nil
}

func (m *mounter) waitForDir(ctx context.Context, args *FilesystemMountArgs) error {
	if args.PlanOnly {
		return nil
	}
	ctx, cancel := context.WithDeadline(ctx, args.Deadline)
	defer cancel()
	wfpArgs := &util.WaitForPathArgs{
		Path: args.Target,
	}
	wfpRet, err := util.WaitForPath(ctx, wfpArgs)
	if err != nil {
		return err
	}
	if wfpRet.Retry {
		m.Log.Infof("%v target directory %v not present at mount time, became available after %s",
			args.LogPrefix, args.Target, wfpRet.TimeTaken)
	}
	if !wfpRet.FInfo.IsDir() {
		return fmt.Errorf("target path is not a directory")
	}
	return nil
}

// FreezeFilesystem is part of the Mounter interface.
// It stops I/O in a previously mounted filesystem that supports the fsfreeze operation.
// Note: there is no way to determine filesystem frozen state and the command is not idempotent
// TBD: require and validate FsType
func (m *mounter) FreezeFilesystem(ctx context.Context, args *FilesystemFreezeArgs) error {
	return m.doFreeze(ctx, args, "-f")
}

// UnfreezeFilesystem is part of the Mounter interface.
// It resumes I/O in a previously frozen filesystem that supports the fsfreeze operation.
// Note: there is no way to determine filesystem frozen state and the command is not idempotent
// TBD: require and validate FsType
func (m *mounter) UnfreezeFilesystem(ctx context.Context, args *FilesystemFreezeArgs) error {
	return m.doFreeze(ctx, args, "-u")
}

func (m *mounter) doFreeze(ctx context.Context, args *FilesystemFreezeArgs, flag string) error {
	if err := args.Validate(); err != nil {
		return err
	}
	cmdArgs := []string{flag, args.Target}
	m.debugCmd(args.Commands, args.LogPrefix, "fsfreeze", cmdArgs)
	if args.PlanOnly {
		return nil
	}
	ctx, cancel := context.WithDeadline(ctx, args.Deadline)
	defer cancel()
	cmd := m.exec.CommandContext(ctx, "fsfreeze", cmdArgs...)
	bytes, err := cmd.CombinedOutput()
	m.Log.Debugf("%s: 'fsfreeze %s' output:\n%s", args.LogPrefix, flag, string(bytes))
	if err != nil {
		m.Log.Errorf("%s: 'fsfreeze %s' failed: %v", args.LogPrefix, flag, err)
		return err
	}
	return nil
}
