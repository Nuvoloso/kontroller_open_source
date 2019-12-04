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

package mount

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util/mock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestNewMounter(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	ma := &MounterArgs{}
	mt, err := New(ma)
	assert.Error(err)
	assert.Equal(ErrInvalidArgs, err)
	assert.Nil(mt)

	ma.Log = tl.Logger()
	mt, err = New(ma)
	assert.NoError(err)
	assert.NotNil(mt)
	m, ok := mt.(*mounter)
	assert.True(ok)
	assert.NotNil(m)
	assert.NotNil(m.exec)
	assert.Equal(m, m.ops)
}

func TestMountFilesystem(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ctx := context.Background()

	ma := &MounterArgs{
		Log: tl.Logger(),
	}
	mt, err := New(ma)
	assert.NoError(err)
	assert.NotNil(mt)
	m, ok := mt.(*mounter)
	assert.True(ok)
	assert.NotNil(m)

	tl.Logger().Info("case: invalid arguments")
	tl.Flush()
	fma := &FilesystemMountArgs{}
	err = mt.MountFilesystem(ctx, fma)
	assert.Equal(ErrInvalidArgs, err)

	fma = &FilesystemMountArgs{
		Source:    "source",
		Target:    "target",
		LogPrefix: "prefix",
	}
	errMnt1 := fmt.Errorf("mount 1 error")

	tl.Logger().Info("case: PLAN ONLY")
	tl.Flush()
	fma.PlanOnly = true
	var cmdBuffer bytes.Buffer
	fma.Commands = &cmdBuffer
	err = mt.MountFilesystem(ctx, fma)
	assert.NoError(err)
	lines := strings.Split(cmdBuffer.String(), "\n")
	for _, cmd := range lines {
		t.Log(cmd)
	}
	assert.Len(lines, 6)
	assert.Equal("fsck \"-a\" \"source\"", lines[0])
	assert.Equal("mount \"-t\" \"ext4\" \"-o\" \"loop,defaults\" \"source\" \"target\"", lines[1])
	assert.Equal("blkid \"-p\" \"-s\" \"TYPE\" \"-o\" \"export\" \"source\"", lines[2])
	assert.Equal("mkfs \"-t\" \"ext4\" \"-E\" \"lazy_itable_init=0,lazy_journal_init=0\" \"source\"", lines[3])
	assert.Equal("mount \"-t\" \"ext4\" \"-o\" \"loop,defaults\" \"source\" \"target\"", lines[4])
	fma.Commands = nil // reset

	tl.Logger().Info("case: PLAN ONLY xfs")
	tl.Flush()
	fma.PlanOnly = true
	cmdBuffer.Reset()
	fma.Commands = &cmdBuffer
	fma.FsType = "xfs"
	m.mntCnt = 0
	err = mt.MountFilesystem(ctx, fma)
	assert.NoError(err)
	lines = strings.Split(cmdBuffer.String(), "\n")
	for _, cmd := range lines {
		t.Log(cmd)
	}
	assert.Len(lines, 6)
	assert.Equal("fsck \"-a\" \"source\"", lines[0])
	assert.Equal("mount \"-t\" \"xfs\" \"-o\" \"loop,defaults\" \"source\" \"target\"", lines[1])
	assert.Equal("blkid \"-p\" \"-s\" \"TYPE\" \"-o\" \"export\" \"source\"", lines[2])
	assert.Equal("mkfs \"-t\" \"xfs\" \"source\"", lines[3])
	assert.Equal("mount \"-t\" \"xfs\" \"-o\" \"loop,defaults\" \"source\" \"target\"", lines[4])
	fma.PlanOnly = false
	fma.Commands = nil  // reset
	fma.FsType = "ext4" // reset

	tl.Logger().Info("case: SUCCESS rw, fsck ok, mount1 ok")
	tl.Flush()
	fMop := &fakeMops{}
	m.ops = fMop
	err = mt.MountFilesystem(ctx, fma)
	assert.NoError(err)
	assert.Equal(DefaultFsType, fma.FsType)

	tl.Logger().Info("case: SUCCESS rw, fsck ok, mount1 err, mkfs, mount2 ok")
	tl.Flush()
	fMop = &fakeMops{}
	fMop.RetDoMountErr = []error{errMnt1}
	m.ops = fMop
	err = mt.MountFilesystem(ctx, fma)
	assert.NoError(err)

	tl.Logger().Info("case: FAIL rw, fsck ok, mount1 deadline exceeded")
	tl.Flush()
	fMop = &fakeMops{}
	fMop.RetDoMountWaitErr = context.DeadlineExceeded
	m.ops = fMop
	err = mt.MountFilesystem(ctx, fma)
	assert.Equal(context.DeadlineExceeded, err)

	tl.Logger().Info("case: FAIL rw, fsck err")
	tl.Flush()
	fMop = &fakeMops{}
	fMop.RetFsckErr = fmt.Errorf("fsck error")
	m.ops = fMop
	err = mt.MountFilesystem(ctx, fma)
	assert.Error(err)
	assert.Regexp("fsck error", err)

	tl.Logger().Info("case: FAIL rw, fsck ok, mount1 fail, correct fs type (mount error)")
	tl.Flush()
	fMop = &fakeMops{}
	fMop.RetDoMountWaitErr = errMnt1
	fMop.RetGetFsType = DefaultFsType
	m.ops = fMop
	err = mt.MountFilesystem(ctx, fma)
	assert.Equal(errMnt1, err)

	tl.Logger().Info("case: FAIL rw, fsck ok, mount1 fail, unexpected file system")
	tl.Flush()
	fMop = &fakeMops{}
	fMop.RetDoMountWaitErr = errMnt1
	fMop.RetGetFsType = "xfs"
	m.ops = fMop
	err = mt.MountFilesystem(ctx, fma)
	assert.Error(err)
	assert.Regexp("expected file system type.*found xfs", err)

	tl.Logger().Info("case: FAIL ro, fsck ok, mount1 fail, unformatted volume")
	tl.Flush()
	fMop = &fakeMops{}
	fMop.RetFsckErr = fmt.Errorf("fsck error")
	fMop.RetDoMountWaitErr = errMnt1
	fMop.RetGetFsType = ""
	m.ops = fMop
	fma.Options = []string{"ro"}
	err = mt.MountFilesystem(ctx, fma)
	assert.Error(err)
	assert.Regexp("unformatted volume read-only", err)

	tl.Logger().Info("case: FAIL waitForDir cant find dir")
	tl.Flush()
	fMop = &fakeMops{}
	fMop.RetDoMountWaitErr = errWaitForDir
	m.ops = fMop
	err = mt.MountFilesystem(ctx, fma)
	assert.Error(err)
	assert.Regexp("waitForDirErr", err)

	tl.Logger().Info("case: REI fail-after-mkfs")
	tl.Flush()
	fMop = &fakeMops{}
	fMop.RetDoMountWaitErr = errMnt1
	m.Rei = rei.NewEPM("mount")
	m.Rei.Enabled = true
	m.Rei.SetProperty("fail-after-mkfs", &rei.Property{BoolValue: true})
	m.ops = fMop
	fma.Options = []string{}
	err = mt.MountFilesystem(ctx, fma)
	assert.Error(err)
	assert.Regexp("fail-after-mkfs", err)
}

type fakeMops struct {
	RetFsckErr        error
	RetDoMountErr     []error
	RetDoMkfsErr      error
	RetGetFsType      string
	RetGetFsTypeErr   error
	RetWFDErr         error
	RetDoMountWaitErr error
}

var _ = (mops)(&fakeMops{})

func (fm *fakeMops) doFsck(ctx context.Context, args *FilesystemMountArgs) error {
	return fm.RetFsckErr
}

func (fm *fakeMops) doMount(ctx context.Context, args *FilesystemMountArgs) error {
	var err error
	if len(fm.RetDoMountErr) > 0 {
		err = fm.RetDoMountErr[0]
		fm.RetDoMountErr = fm.RetDoMountErr[:len(fm.RetDoMountErr)-1]
	}
	return err
}

func (fm *fakeMops) doMkfs(ctx context.Context, args *FilesystemMountArgs) error {
	return fm.RetDoMkfsErr
}

func (fm *fakeMops) getFsType(ctx context.Context, args *FilesystemMountArgs) (string, error) {
	return fm.RetGetFsType, fm.RetGetFsTypeErr
}

func (fm *fakeMops) waitForDir(ctx context.Context, args *FilesystemMountArgs) error {
	return fm.RetWFDErr
}

func (fm *fakeMops) doMountWithWait(ctx context.Context, args *FilesystemMountArgs) error {
	return fm.RetDoMountWaitErr
}

func TestDoFsck(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ctx := context.Background()

	ma := &MounterArgs{
		Log: tl.Logger(),
	}
	mt, err := New(ma)
	assert.NoError(err)
	assert.NotNil(mt)
	m, ok := mt.(*mounter)
	assert.True(ok)
	assert.NotNil(m)

	fma := &FilesystemMountArgs{
		Source:    "source",
		LogPrefix: "prefix",
		Deadline:  time.Now().Add(DefaultCommandTimeout),
	}

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ex := mock.NewMockExec(mockCtrl)
	m.exec = ex
	cmd := mock.NewMockCmd(mockCtrl)

	tl.Logger().Info("case: SUCCESS, fsck not found")
	tl.Flush()
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, fma.Deadline, t), "fsck", "-a", fma.Source).Return(cmd)
	retErr := exec.ErrNotFound
	cmd.EXPECT().CombinedOutput().Return(nil, retErr)
	ex.EXPECT().IsExitError(retErr).Return(0, false)
	ex.EXPECT().IsNotFoundError(retErr).Return(true)
	err = m.doFsck(ctx, fma)
	assert.NoError(err)

	tl.Logger().Info("case: SUCCESS, fsck errors corrected")
	tl.Flush()
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, fma.Deadline, t), "fsck", "-a", fma.Source).Return(cmd)
	retErr = fmt.Errorf("fake error")
	cmd.EXPECT().CombinedOutput().Return(nil, retErr)
	ex.EXPECT().IsExitError(retErr).Return(fsckErrorsCorrected, true)
	ex.EXPECT().IsNotFoundError(retErr).Return(false)
	err = m.doFsck(ctx, fma)
	assert.NoError(err)

	tl.Logger().Info("case: SUCCESS, other fsck error")
	tl.Flush()
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, fma.Deadline, t), "fsck", "-a", fma.Source).Return(cmd)
	retErr = fmt.Errorf("fake error")
	cmd.EXPECT().CombinedOutput().Return(nil, retErr)
	ex.EXPECT().IsExitError(retErr).Return(fsckErrorsUncorrected+1, true)
	ex.EXPECT().IsNotFoundError(retErr).Return(false)
	err = m.doFsck(ctx, fma)
	assert.NoError(err)

	tl.Logger().Info("case: SUCCESS, other error")
	tl.Flush()
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, fma.Deadline, t), "fsck", "-a", fma.Source).Return(cmd)
	retErr = fmt.Errorf("fake error")
	cmd.EXPECT().CombinedOutput().Return(nil, retErr)
	ex.EXPECT().IsExitError(retErr).Return(0, false)
	ex.EXPECT().IsNotFoundError(retErr).Return(false)
	err = m.doFsck(ctx, fma)
	assert.NoError(err)

	tl.Logger().Info("case: FAILURE, fsck errors uncorrected")
	tl.Flush()
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, fma.Deadline, t), "fsck", "-a", fma.Source).Return(cmd)
	retErr = fmt.Errorf("fake error")
	cmd.EXPECT().CombinedOutput().Return(nil, retErr)
	ex.EXPECT().IsExitError(retErr).Return(fsckErrorsUncorrected, true)
	ex.EXPECT().IsNotFoundError(retErr).Return(false)
	err = m.doFsck(ctx, fma)
	assert.Error(err)
	assert.Regexp("fsck.*error", err)

	tl.Logger().Info("case: FAILURE, deadline exceeded")
	tl.Flush()
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, fma.Deadline, t), "fsck", "-a", fma.Source).Return(cmd)
	retErr = context.DeadlineExceeded
	cmd.EXPECT().CombinedOutput().Return(nil, retErr)
	ex.EXPECT().IsExitError(retErr).Return(0, false)
	err = m.doFsck(ctx, fma)
	assert.Error(err)
	assert.Equal(context.DeadlineExceeded, err)
}

func TestDoMount(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ctx := context.Background()

	ma := &MounterArgs{
		Log: tl.Logger(),
	}
	mt, err := New(ma)
	assert.NoError(err)
	assert.NotNil(mt)
	m, ok := mt.(*mounter)
	assert.True(ok)
	assert.NotNil(m)

	fma := &FilesystemMountArgs{
		Source:    "source",
		Target:    "target",
		FsType:    DefaultFsType,
		LogPrefix: "prefix",
		Deadline:  time.Now().Add(DefaultCommandTimeout),
	}

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ex := mock.NewMockExec(mockCtrl)
	m.exec = ex
	cmd := mock.NewMockCmd(mockCtrl)

	tl.Logger().Info("case: FAILURE, rw")
	tl.Flush()
	retErr := fmt.Errorf("some error")
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, fma.Deadline, t), "mount", "-t", fma.FsType, "-o", "loop,defaults", fma.Source, fma.Target).Return(cmd)
	cmd.EXPECT().CombinedOutput().Return([]byte{}, retErr)
	err = m.doMount(ctx, fma)
	assert.Equal(retErr, err)

	tl.Logger().Info("case: SUCCESS, rw")
	tl.Flush()
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, fma.Deadline, t), "mount", "-t", fma.FsType, "-o", "loop,defaults", fma.Source, fma.Target).Return(cmd)
	cmd.EXPECT().CombinedOutput().Return(nil, nil)
	err = m.doMount(ctx, fma)
	assert.NoError(err)

	tl.Logger().Info("case: SUCCESS, ro")
	tl.Flush()
	fma.Options = []string{"ro"}
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, fma.Deadline, t), "mount", "-t", fma.FsType, "-o", "ro,loop,defaults", fma.Source, fma.Target).Return(cmd)
	cmd.EXPECT().CombinedOutput().Return(nil, nil)
	err = m.doMount(ctx, fma)
	assert.NoError(err)
}

func TestDoMkfs(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ctx := context.Background()

	ma := &MounterArgs{
		Log: tl.Logger(),
	}
	mt, err := New(ma)
	assert.NoError(err)
	assert.NotNil(mt)
	m, ok := mt.(*mounter)
	assert.True(ok)
	assert.NotNil(m)

	fma := &FilesystemMountArgs{
		Source:    "source",
		Target:    "target",
		FsType:    DefaultFsType,
		LogPrefix: "prefix",
		Deadline:  time.Now().Add(DefaultCommandTimeout),
	}

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ex := mock.NewMockExec(mockCtrl)
	m.exec = ex
	cmd := mock.NewMockCmd(mockCtrl)

	tl.Logger().Info("case: FAILURE")
	tl.Flush()
	retErr := fmt.Errorf("some error")
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, fma.Deadline, t), "mkfs", "-t", fma.FsType, "-E", "lazy_itable_init=0,lazy_journal_init=0", fma.Source).Return(cmd)
	cmd.EXPECT().CombinedOutput().Return([]byte{}, retErr)
	err = m.doMkfs(ctx, fma)
	assert.Equal(retErr, err)

	tl.Logger().Info("case: SUCCESS")
	tl.Flush()
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, fma.Deadline, t), "mkfs", "-t", fma.FsType, "-E", "lazy_itable_init=0,lazy_journal_init=0", fma.Source).Return(cmd)
	cmd.EXPECT().CombinedOutput().Return(nil, nil)
	err = m.doMkfs(ctx, fma)
	assert.NoError(err)
}

func TestGetFsType(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ctx := context.Background()

	ma := &MounterArgs{
		Log: tl.Logger(),
	}
	mt, err := New(ma)
	assert.NoError(err)
	assert.NotNil(mt)
	m, ok := mt.(*mounter)
	assert.True(ok)
	assert.NotNil(m)

	fma := &FilesystemMountArgs{
		Source:    "source",
		Target:    "target",
		FsType:    DefaultFsType,
		LogPrefix: "prefix",
		Deadline:  time.Now().Add(DefaultCommandTimeout),
	}

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ex := mock.NewMockExec(mockCtrl)
	m.exec = ex
	cmd := mock.NewMockCmd(mockCtrl)

	tl.Logger().Info("case: FAILURE")
	tl.Flush()
	retErr := fmt.Errorf("some error")
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, fma.Deadline, t), "blkid", "-p", "-s", "TYPE", "-o", "export", fma.Source).Return(cmd)
	cmd.EXPECT().CombinedOutput().Return([]byte{}, retErr)
	ex.EXPECT().IsExitError(retErr).Return(0, false)
	fs, err := m.getFsType(ctx, fma)
	assert.Equal(retErr, err)
	assert.Equal("", fs)

	tl.Logger().Info("case: SUCCESS, formatted")
	tl.Flush()
	output := "DEVNAME=source\n" +
		"\n" +
		"TYPE=xfs"
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, fma.Deadline, t), "blkid", "-p", "-s", "TYPE", "-o", "export", fma.Source).Return(cmd)
	cmd.EXPECT().CombinedOutput().Return([]byte(output), nil)
	fs, err = m.getFsType(ctx, fma)
	assert.NoError(err)
	assert.Equal("xfs", fs)

	tl.Logger().Info("case: SUCCESS, unformatted")
	tl.Flush()
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, fma.Deadline, t), "blkid", "-p", "-s", "TYPE", "-o", "export", fma.Source).Return(cmd)
	cmd.EXPECT().CombinedOutput().Return([]byte{}, retErr)
	ex.EXPECT().IsExitError(retErr).Return(2, true)
	fs, err = m.getFsType(ctx, fma)
	assert.NoError(err)
	assert.Equal("", fs)

	tl.Logger().Info("case: SUCCESS, type not found")
	tl.Flush()
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, fma.Deadline, t), "blkid", "-p", "-s", "TYPE", "-o", "export", fma.Source).Return(cmd)
	cmd.EXPECT().CombinedOutput().Return([]byte{}, nil)
	fs, err = m.getFsType(ctx, fma)
	assert.NoError(err)
	assert.Equal("", fs)
}

func TestUnmountFilesystem(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ctx := context.Background()

	ma := &MounterArgs{
		Log: tl.Logger(),
	}
	mt, err := New(ma)
	assert.NoError(err)
	assert.NotNil(mt)
	m, ok := mt.(*mounter)
	assert.True(ok)
	assert.NotNil(m)

	tl.Logger().Info("case: FAILURE - invalid args")
	tl.Flush()
	fua := &FilesystemUnmountArgs{}
	err = m.UnmountFilesystem(ctx, fua)
	assert.Equal(ErrInvalidArgs, err)

	fua = &FilesystemUnmountArgs{
		Target:    "target",
		LogPrefix: "prefix",
	}

	tl.Logger().Info("case: PLAN ONLY")
	tl.Flush()
	fua.PlanOnly = true
	var cmdBuffer bytes.Buffer
	fua.Commands = &cmdBuffer
	err = mt.UnmountFilesystem(ctx, fua)
	assert.NoError(err)
	lines := strings.Split(cmdBuffer.String(), "\n")
	for _, cmd := range lines {
		t.Log(cmd)
	}
	assert.Len(lines, 2)
	assert.Equal("umount \"-d\" \"target\"", lines[0])

	fua.PlanOnly = false
	fua.Commands = nil

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ex := mock.NewMockExec(mockCtrl)
	m.exec = ex
	cmd := mock.NewMockCmd(mockCtrl)

	tl.Logger().Info("case: FAILURE")
	tl.Flush()
	retErr := fmt.Errorf("some error")
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, fua.Deadline, t), "umount", "-d", fua.Target).Return(cmd)
	cmd.EXPECT().CombinedOutput().Return([]byte{}, retErr)
	err = m.UnmountFilesystem(ctx, fua)
	assert.Equal(retErr, err)

	tl.Logger().Info("case: SUCCESS")
	tl.Flush()
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, fua.Deadline, t), "umount", "-d", fua.Target).Return(cmd)
	cmd.EXPECT().CombinedOutput().Return(nil, nil)
	err = m.UnmountFilesystem(ctx, fua)
	assert.NoError(err)
}

func TestIsFilesystemMounted(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ctx := context.Background()

	ma := &MounterArgs{
		Log: tl.Logger(),
	}
	mt, err := New(ma)
	assert.NoError(err)
	assert.NotNil(mt)
	m, ok := mt.(*mounter)
	assert.True(ok)
	assert.NotNil(m)

	var rc bool

	tl.Logger().Info("case: invalid arguments")
	tl.Flush()
	fma := &FilesystemMountArgs{}
	rc, err = mt.IsFilesystemMounted(ctx, fma)
	assert.Equal(ErrInvalidArgs, err)
	assert.False(rc)

	fma = &FilesystemMountArgs{
		Source:    "source",
		Target:    "target",
		FsType:    DefaultFsType,
		LogPrefix: "prefix",
		Deadline:  time.Now().Add(DefaultCommandTimeout),
	}
	tl.Logger().Info("case: PLAN ONLY")
	tl.Flush()
	fma.PlanOnly = true
	rc, err = mt.IsFilesystemMounted(ctx, fma)
	assert.NoError(err)
	assert.False(rc)
	fma.PlanOnly = false

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ex := mock.NewMockExec(mockCtrl)
	m.exec = ex
	cmd := mock.NewMockCmd(mockCtrl)

	tl.Logger().Info("case: mount FAILURE")
	tl.Flush()
	retErr := fmt.Errorf("some error")
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, fma.Deadline, t), "mount").Return(cmd)
	cmd.EXPECT().CombinedOutput().Return([]byte{}, retErr)
	rc, err = mt.IsFilesystemMounted(ctx, fma)
	assert.Equal(retErr, err)
	assert.False(rc)

	mountResRW := []byte(strings.Join([]string{
		"cgroup on /sys/fs/cgroup/blkio type cgroup (rw,nosuid,nodev,noexec,relatime,blkio)",
		"172.16.139.1:/Users on /mnt/hgfs type nfs (rw,relatime,vers=3,rsize=65536,wsize=65536,namlen=255,acregmin=60,acdirmin=60,hard,proto=tcp,timeo=600,retrans=2,sec=sys,mountaddr=172.16.139.1,mountvers=3,mountport=887,mountproto=udp,local_lock=none,addr=172.16.139.1)",
		"/tmp/foo on /tmp/bar type ext4 (rw,relatime,block_validity,delalloc,barrier,user_xattr,acl)",
	}, "\n"))
	mountResRO := []byte(strings.Join([]string{
		"cgroup on /sys/fs/cgroup/blkio type cgroup (rw,nosuid,nodev,noexec,relatime,blkio)",
		"/tmp/foo on /tmp/bar type ext4 (ro,relatime,block_validity,delalloc,barrier,user_xattr,acl)",
		"172.16.139.1:/Users on /mnt/hgfs type nfs (rw,relatime,vers=3,rsize=65536,wsize=65536,namlen=255,acregmin=60,acdirmin=60,hard,proto=tcp,timeo=600,retrans=2,sec=sys,mountaddr=172.16.139.1,mountvers=3,mountport=887,mountproto=udp,local_lock=none,addr=172.16.139.1)",
	}, "\n"))

	tl.Logger().Info("case: Mounted")
	tl.Flush()
	assert.Equal(DefaultFsType, "ext4")
	fma = &FilesystemMountArgs{
		Source:    "/tmp/foo",
		Target:    "/tmp/bar",
		LogPrefix: "prefix",
		Deadline:  time.Now().Add(DefaultCommandTimeout),
	}
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, fma.Deadline, t), "mount").Return(cmd)
	cmd.EXPECT().CombinedOutput().Return(mountResRW, nil)
	rc, err = mt.IsFilesystemMounted(ctx, fma)
	assert.NoError(err)
	assert.True(rc)

	tl.Logger().Info("case: Not mounted")
	tl.Flush()
	assert.Equal(DefaultFsType, "ext4")
	fma = &FilesystemMountArgs{
		Source:    "/tmp/foobar",
		Target:    "/tmp/bar",
		LogPrefix: "prefix",
		Deadline:  time.Now().Add(DefaultCommandTimeout),
	}
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, fma.Deadline, t), "mount").Return(cmd)
	cmd.EXPECT().CombinedOutput().Return(mountResRW, nil)
	rc, err = mt.IsFilesystemMounted(ctx, fma)
	assert.NoError(err)
	assert.False(rc)

	tl.Logger().Info("case: mode mismatch (ro)")
	tl.Flush()
	assert.Equal(DefaultFsType, "ext4")
	fma = &FilesystemMountArgs{
		Source:    "/tmp/foo",
		Target:    "/tmp/bar",
		LogPrefix: "prefix",
		Options:   []string{"ro"},
		Deadline:  time.Now().Add(DefaultCommandTimeout),
	}
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, fma.Deadline, t), "mount").Return(cmd)
	cmd.EXPECT().CombinedOutput().Return(mountResRW, nil)
	rc, err = mt.IsFilesystemMounted(ctx, fma)
	assert.Regexp("mismatch.*ro.*rw", err)
	assert.False(rc)

	tl.Logger().Info("case: mode mismatch (rw)")
	tl.Flush()
	assert.Equal(DefaultFsType, "ext4")
	fma = &FilesystemMountArgs{
		Source:    "/tmp/foo",
		Target:    "/tmp/bar",
		LogPrefix: "prefix",
		Deadline:  time.Now().Add(DefaultCommandTimeout),
	}
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, fma.Deadline, t), "mount").Return(cmd)
	cmd.EXPECT().CombinedOutput().Return(mountResRO, nil)
	rc, err = mt.IsFilesystemMounted(ctx, fma)
	assert.Regexp("mismatch.*rw.*ro", err)
	assert.False(rc)

	tl.Logger().Info("case: fsType mismatch")
	tl.Flush()
	assert.Equal(DefaultFsType, "ext4")
	fma = &FilesystemMountArgs{
		Source:    "/tmp/foo",
		Target:    "/tmp/bar",
		FsType:    "xfs",
		LogPrefix: "prefix",
		Deadline:  time.Now().Add(DefaultCommandTimeout),
	}
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, fma.Deadline, t), "mount").Return(cmd)
	cmd.EXPECT().CombinedOutput().Return(mountResRW, nil)
	rc, err = mt.IsFilesystemMounted(ctx, fma)
	assert.Regexp("mismatch.*xfs.*ext4", err)
	assert.False(rc)

	tl.Logger().Info("case: target mismatch")
	tl.Flush()
	assert.Equal(DefaultFsType, "ext4")
	fma = &FilesystemMountArgs{
		Source:    "/tmp/foo",
		Target:    "/tmp/foobar",
		FsType:    "xfs",
		LogPrefix: "prefix",
		Deadline:  time.Now().Add(DefaultCommandTimeout),
	}
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, fma.Deadline, t), "mount").Return(cmd)
	cmd.EXPECT().CombinedOutput().Return(mountResRW, nil)
	rc, err = mt.IsFilesystemMounted(ctx, fma)
	assert.Regexp("mismatch.*foobar.*bar", err)
	assert.False(rc)
}

type ctxDeadlineMatcher struct {
	t        *testing.T
	pCtx     context.Context
	deadline time.Time
}

func newCtxDeadlineMatcher(pCtx context.Context, deadline time.Time, t *testing.T) gomock.Matcher {
	return &ctxDeadlineMatcher{t: t, pCtx: pCtx, deadline: deadline}
}

func (m *ctxDeadlineMatcher) Matches(x interface{}) bool {
	ctx := x.(context.Context) // or barf
	if dl, ok := ctx.Deadline(); ok {
		return assert.True(m.t, m.deadline.Equal(dl))
	}
	return false
}

func (m *ctxDeadlineMatcher) String() string {
	return "ctx deadline matches"
}

func TestWaitForDir(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ctx := context.Background()

	ma := &MounterArgs{
		Log: tl.Logger(),
	}
	mt, err := New(ma)
	assert.NoError(err)
	assert.NotNil(mt)
	m, ok := mt.(*mounter)
	assert.True(ok)
	assert.NotNil(m)

	// target path not specified
	ctx = context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Millisecond*50)
	fma := &FilesystemMountArgs{}
	err = m.waitForDir(ctx, fma)
	assert.NotNil(err)
	assert.Regexp("empty dirPath", err.Error())
	tl.Flush()

	// timeout, target not found
	ctx, cancel = context.WithTimeout(ctx, time.Millisecond*50)
	defer cancel()
	fma.Target = "./sometestDir"
	fma.PlanOnly = false
	err = m.waitForDir(ctx, fma)
	assert.NotNil(err)
	assert.Regexp("context expired", err.Error())

	// path found but is a file
	testFile := "./sometestFile"
	os.Create(testFile)
	defer os.Remove(testFile)
	fma.Target = testFile
	err = m.waitForDir(ctx, fma)
	assert.NotNil(err)
	assert.Regexp("not a directory", err)

	// target found
	os.Mkdir("./sometestDir", 0700)
	defer os.Remove("./sometestDir")
	fma.Target = "./sometestDir"
	fma.PlanOnly = false
	err = m.waitForDir(ctx, fma)
	assert.Nil(err)

	// created while executing
	dirPath := "./someotherdir"
	fma.Deadline = time.Now().Add(time.Millisecond * 50)
	go func() {
		time.Sleep(5 * time.Millisecond)
		os.Mkdir(dirPath, 0700)
	}()
	defer os.Remove(dirPath)
	fma.Target = dirPath
	err = m.waitForDir(ctx, fma)
	assert.Nil(err)
	assert.Equal(1, tl.CountPattern("not present at mount time, became available after*"))

	// plan only
	fma.PlanOnly = true
	err = m.waitForDir(ctx, fma)
	assert.Nil(err)
}

func TestFreezeFilesystem(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ctx := context.Background()

	ma := &MounterArgs{
		Log: tl.Logger(),
	}
	mt, err := New(ma)
	assert.NoError(err)
	assert.NotNil(mt)
	m, ok := mt.(*mounter)
	assert.True(ok)
	assert.NotNil(m)

	tl.Logger().Info("case: FAILURE - invalid args")
	tl.Flush()
	ffa := &FilesystemFreezeArgs{}
	err = m.FreezeFilesystem(ctx, ffa)
	assert.Equal(ErrInvalidArgs, err)

	ffa = &FilesystemFreezeArgs{
		Target:    "target",
		LogPrefix: "prefix",
	}

	tl.Logger().Info("case: PLAN ONLY")
	tl.Flush()
	ffa.PlanOnly = true
	var cmdBuffer bytes.Buffer
	ffa.Commands = &cmdBuffer
	err = mt.FreezeFilesystem(ctx, ffa)
	assert.NoError(err)
	lines := strings.Split(cmdBuffer.String(), "\n")
	for _, cmd := range lines {
		t.Log(cmd)
	}
	assert.Len(lines, 2)
	assert.Equal("fsfreeze \"-f\" \"target\"", lines[0])

	ffa.PlanOnly = false
	ffa.Commands = nil

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ex := mock.NewMockExec(mockCtrl)
	m.exec = ex
	cmd := mock.NewMockCmd(mockCtrl)

	tl.Logger().Info("case: FAILURE")
	tl.Flush()
	retErr := fmt.Errorf("some error")
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, ffa.Deadline, t), "fsfreeze", "-f", ffa.Target).Return(cmd)
	cmd.EXPECT().CombinedOutput().Return([]byte{}, retErr)
	err = m.FreezeFilesystem(ctx, ffa)
	assert.Equal(retErr, err)

	tl.Logger().Info("case: SUCCESS")
	tl.Flush()
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, ffa.Deadline, t), "fsfreeze", "-f", ffa.Target).Return(cmd)
	cmd.EXPECT().CombinedOutput().Return(nil, nil)
	err = m.FreezeFilesystem(ctx, ffa)
	assert.NoError(err)
}

func TestUnfreezeFilesystem(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ctx := context.Background()

	ma := &MounterArgs{
		Log: tl.Logger(),
	}
	mt, err := New(ma)
	assert.NoError(err)
	assert.NotNil(mt)
	m, ok := mt.(*mounter)
	assert.True(ok)
	assert.NotNil(m)

	tl.Logger().Info("case: FAILURE - invalid args")
	tl.Flush()
	ffa := &FilesystemFreezeArgs{}
	err = m.UnfreezeFilesystem(ctx, ffa)
	assert.Equal(ErrInvalidArgs, err)

	ffa = &FilesystemFreezeArgs{
		Target:    "target",
		LogPrefix: "prefix",
	}

	tl.Logger().Info("case: PLAN ONLY")
	tl.Flush()
	ffa.PlanOnly = true
	var cmdBuffer bytes.Buffer
	ffa.Commands = &cmdBuffer
	err = mt.UnfreezeFilesystem(ctx, ffa)
	assert.NoError(err)
	lines := strings.Split(cmdBuffer.String(), "\n")
	for _, cmd := range lines {
		t.Log(cmd)
	}
	assert.Len(lines, 2)
	assert.Equal("fsfreeze \"-u\" \"target\"", lines[0])

	ffa.PlanOnly = false
	ffa.Commands = nil

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ex := mock.NewMockExec(mockCtrl)
	m.exec = ex
	cmd := mock.NewMockCmd(mockCtrl)

	tl.Logger().Info("case: FAILURE")
	tl.Flush()
	retErr := fmt.Errorf("some error")
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, ffa.Deadline, t), "fsfreeze", "-u", ffa.Target).Return(cmd)
	cmd.EXPECT().CombinedOutput().Return([]byte{}, retErr)
	err = m.UnfreezeFilesystem(ctx, ffa)
	assert.Equal(retErr, err)

	tl.Logger().Info("case: SUCCESS")
	tl.Flush()
	ex.EXPECT().CommandContext(newCtxDeadlineMatcher(ctx, ffa.Deadline, t), "fsfreeze", "-u", ffa.Target).Return(cmd)
	cmd.EXPECT().CombinedOutput().Return(nil, nil)
	err = m.UnfreezeFilesystem(ctx, ffa)
	assert.NoError(err)
}

func TestDoMountWithWait(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ctx := context.Background()

	ma := &MounterArgs{
		Log: tl.Logger(),
	}
	mt, err := New(ma)
	assert.NoError(err)
	assert.NotNil(mt)
	m, ok := mt.(*mounter)
	assert.True(ok)
	assert.NotNil(m)

	// success
	fma := &FilesystemMountArgs{}
	fMop := &fakeMops{}
	m.ops = fMop
	err = m.doMountWithWait(ctx, fma)
	assert.Nil(err)

	// waitForDir error
	fMop = &fakeMops{}
	fMop.RetWFDErr = fmt.Errorf("err while waiting for dir")
	m.ops = fMop
	err = m.doMountWithWait(ctx, fma)
	assert.NotNil(err)
	assert.Regexp("waitForDirErr", err.Error())

	// file exists, but mount actually fails
	os.Mkdir("./sometestDir", 0700)
	defer os.Remove("./sometestDir")
	fMop = &fakeMops{}
	fMop.RetDoMountErr = []error{fmt.Errorf("mount err")}
	m.ops = fMop
	fma.Target = "./sometestDir"
	err = m.doMountWithWait(ctx, fma)
	assert.NotNil(err)
	assert.Regexp("mount err", err.Error())

	// ensure retrys occur once and succeeds
	tl.Flush()
	os.Remove("./sometestDir")
	fMop = &fakeMops{}
	fMop.RetDoMountErr = []error{fmt.Errorf("mount err")}
	m.ops = fMop
	fma.Target = "./sometestDir"
	err = m.doMountWithWait(ctx, fma)
	assert.Nil(err)
	assert.Regexp(1, tl.CountPattern("mount point removed, retrying mount"))

	// plan only
	fma.PlanOnly = true
	fMop = &fakeMops{}
	fMop.RetDoMountErr = []error{fmt.Errorf("plan only error")}
	m.ops = fMop
	err = m.doMountWithWait(ctx, fma)
	assert.NotNil(err)
	assert.Regexp("plan only error", err.Error())
}
