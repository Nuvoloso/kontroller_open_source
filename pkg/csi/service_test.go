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
	"context"
	"io/ioutil"
	"net"
	"os"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestInit(t *testing.T) {
	assert := assert.New(t)

	svc := &csiService{}

	// failure in listen
	svc.Socket = "/bad/path/to/socket" // required for linux
	err := svc.init()
	assert.Error(err)
	assert.Nil(svc.listener)

	// success case; test removal of previous path
	svc.Socket = "./test-socket"
	defer os.Remove(svc.Socket)

	ioutil.WriteFile(svc.Socket, []byte{}, 0)
	fi, err := os.Stat(svc.Socket)
	assert.NoError(err)
	assert.NotNil(fi)

	svc.listener = nil
	svc.server = nil
	err = svc.init()
	assert.NoError(err)
	assert.NotNil(svc.listener)
	assert.NotNil(svc.server)
}

func TestStart(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	// failure during init
	svc := &csiService{}
	svc.Log = tl.Logger()
	svc.Socket = "/bad/path/for/socket"
	err := svc.Start()
	assert.Error(err)

	// success path via fakes
	fs := &fakeStuff{}
	svc = &csiService{}
	svc.server = fs
	svc.ops = fs
	err = svc.Start()
	assert.NoError(err)
	assert.True(fs.CalledRIS)
	assert.True(fs.CalledRCS)
	assert.False(fs.CalledRNS)
	for !fs.CalledServe {
		time.Sleep(5 * time.Millisecond)
	}

	// success path with nodeService registration
	fs = &fakeStuff{}
	svc = &csiService{}
	svc.server = fs
	svc.ops = fs
	svc.NodeOps = &fakeStuff{}
	err = svc.Start()
	assert.NoError(err)
	assert.True(fs.CalledRIS)
	assert.False(fs.CalledRCS)
	assert.True(fs.CalledRNS)
}

type fakeStuff struct {
	CalledServe bool
	RetServe    error

	CalledRIS bool

	CalledRNS bool

	CalledRCS bool
}

func (s *fakeStuff) Serve(net.Listener) error {
	s.CalledServe = true
	return s.RetServe
}
func (s *fakeStuff) Stop() {

}
func (s *fakeStuff) RegisterIdentityServer() {
	s.CalledRIS = true
}
func (s *fakeStuff) RegisterNodeServer() {
	s.CalledRNS = true
}
func (s *fakeStuff) RegisterControllerServer() {
	s.CalledRCS = true
}

func (s *fakeStuff) MountVolume(ctx context.Context, args *MountArgs) error {
	return nil
}

func (s *fakeStuff) UnmountVolume(ctx context.Context, args *UnmountArgs) error {
	return nil
}

func (s *fakeStuff) GetAccountFromSecret(ctx context.Context, args *AccountFetchArgs) (*models.Account, error) {
	return nil, nil
}

func (s *fakeStuff) IsReady() bool {
	return true
}
