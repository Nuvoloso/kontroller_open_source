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

	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func TestIdentityService(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	svc := &csiService{}
	svc.Log = tl.Logger()
	svc.Version = "1.0.1"

	// ************** Register
	svc.server = grpc.NewServer()
	svc.RegisterIdentityServer()

	// ************** GetPluginInfo
	retGPI, err := svc.GetPluginInfo(nil, nil)
	assert.NoError(err)
	assert.NotNil(retGPI)
	assert.Equal(com.CSIDriverName, retGPI.Name)
	assert.Equal(svc.Version, retGPI.VendorVersion)

	// ************** GetPluginCapabilities
	retGPC, err := svc.GetPluginCapabilities(nil, nil)
	assert.NoError(err)
	assert.NotNil(retGPC)
	assert.Equal(1, len(retGPC.Capabilities))

	// ************** Probe
	// node is ready
	fno := &fakeNHO{}
	fno.RetIsReady = true
	svc.NodeOps = fno
	retP, err := svc.Probe(nil, nil)
	assert.NoError(err)
	assert.NotNil(retP)
	assert.True(retP.Ready.Value)

	// node is not ready
	fno.RetIsReady = false
	retP, err = svc.Probe(nil, nil)
	assert.NoError(err)
	assert.NotNil(retP)
	assert.False(retP.Ready.Value)

	// controller is ready
	svc.NodeOps = nil
	fco := &fakeCHO{}
	fco.RetIsReady = true
	svc.ControllerOps = fco
	retP, err = svc.Probe(nil, nil)
	assert.NoError(err)
	assert.NotNil(retP)
	assert.True(retP.Ready.Value)

	// controller is not ready
	fco.RetIsReady = false
	retP, err = svc.Probe(nil, nil)
	assert.NoError(err)
	assert.NotNil(retP)
	assert.False(retP.Ready.Value)

	// controller ops and node ops are not initialized
	svc.ControllerOps = nil
	retP, err = svc.Probe(nil, nil)
	assert.NoError(err)
	assert.NotNil(retP)
	assert.False(retP.Ready.Value)
}
