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


package gc

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util/mock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestLocalEphemeralDevices(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	gcCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(gcCSP)
	cGC, ok := gcCSP.(*CSP)
	assert.True(ok)
	cGC.SetDebugLogger(tl.Logger())

	fakeReadDir := &fakeReadDirData{}
	origReadDirHook := readDirHook
	readDirHook = func(dirName string) ([]os.FileInfo, error) {
		fakeReadDir.inDirName = dirName
		return fakeReadDir.retDirList, fakeReadDir.retError
	}
	defer func() {
		readDirHook = origReadDirHook
	}()

	retDirList := []os.FileInfo{
		&fakeFileInfo{"google-local-ssd-0"},
		&fakeFileInfo{"google-local-ssd-0-part1"},
		&fakeFileInfo{"google-local-ssd-1"},
		&fakeFileInfo{"google-nuvoloso-uuid-here"},
		&fakeFileInfo{"google-persistent-disk-0"},
		&fakeFileInfo{"google-persistent-disk-0-part1"},
		&fakeFileInfo{"scsi-0Google_PersistentDisk_persistent-disk-0"},
		&fakeFileInfo{"scsi-0Google_PersistentDisk_persistent-disk-0-part1"},
	}

	t.Log("case: ReadDir fails")
	gcCl := &Client{
		csp: cGC,
	}
	fakeReadDir.retError = errors.New("readDir")
	list, err := gcCl.LocalEphemeralDevices()
	assert.Nil(list)
	assert.Regexp("readDir", err)
	fakeReadDir.retError = nil

	t.Log("case: empty dir")
	list, err = gcCl.LocalEphemeralDevices()
	assert.NotNil(list)
	assert.Empty(list)
	assert.NoError(err)
	tl.Flush()

	t.Log("case: blkid reports formatted device")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	blkCmd := mock.NewMockCmd(mockCtrl)
	ex := mock.NewMockExec(mockCtrl)
	ex.EXPECT().CommandContext(gomock.Not(gomock.Nil()), "blkid", "/dev/disk/by-id/google-local-ssd-1").Return(blkCmd)
	cGC.exec = ex
	blkCmd.EXPECT().CombinedOutput().Return([]byte("/dev/disk/by-id/google-local-ssd-1: ID=1\n"), nil)
	fakeReadDir.retDirList = retDirList
	list, err = gcCl.LocalEphemeralDevices()
	expList := []*csp.EphemeralDevice{
		{Path: "/dev/disk/by-id/google-local-ssd-0", Type: "SSD", Initialized: true, Usable: false, SizeBytes: localSSDSizeBytes},
		{Path: "/dev/disk/by-id/google-local-ssd-1", Type: "SSD", Initialized: true, Usable: false, SizeBytes: localSSDSizeBytes},
	}
	assert.Equal(expList, list)
	assert.NoError(err)
	tl.Flush()

	t.Log("case: success, also cover nvme matches")
	retDirList[0] = &fakeFileInfo{"google-local-nvme-ssd-0"}
	retDirList[1] = &fakeFileInfo{"google-local-nvme-ssd-0-part1"}
	blkCmd = mock.NewMockCmd(mockCtrl)
	ex.EXPECT().CommandContext(gomock.Not(gomock.Nil()), "blkid", "/dev/disk/by-id/google-local-ssd-1").Return(blkCmd)
	blkCmd.EXPECT().CombinedOutput().Return([]byte(""), errors.New("no match"))
	list, err = gcCl.LocalEphemeralDevices()
	expList = []*csp.EphemeralDevice{
		{Path: "/dev/disk/by-id/google-local-nvme-ssd-0", Type: "SSD", Initialized: true, Usable: false, SizeBytes: localSSDSizeBytes},
		{Path: "/dev/disk/by-id/google-local-ssd-1", Type: "SSD", Initialized: true, Usable: true, SizeBytes: localSSDSizeBytes},
	}
	assert.Equal(expList, list)
	assert.NoError(err)
}

type fakeReadDirData struct {
	inDirName  string
	retDirList []os.FileInfo
	retError   error
}

// fakeFileInfo implements os.FileInfo
type fakeFileInfo struct {
	name string
}

func (f *fakeFileInfo) Name() string {
	return f.name
}

func (f *fakeFileInfo) Size() int64 {
	return int64(len(f.name))
}

func (f *fakeFileInfo) Mode() os.FileMode {
	return os.ModeSymlink
}

func (f *fakeFileInfo) ModTime() time.Time {
	return time.Now()
}

func (f *fakeFileInfo) IsDir() bool {
	return false
}

func (f *fakeFileInfo) Sys() interface{} {
	return nil
}
