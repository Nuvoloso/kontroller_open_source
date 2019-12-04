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
	"fmt"
	"testing"

	com "github.com/Nuvoloso/kontroller/pkg/common"
	csi "github.com/Nuvoloso/kontroller/pkg/csi/csi_pb"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func TestControllerService(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	svc := &csiService{}
	svc.Log = tl.Logger()

	// Register
	svc.server = grpc.NewServer()
	svc.RegisterControllerServer()

	// ControllerGetCapabilities
	// success
	retCGC, err := svc.ControllerGetCapabilities(nil, nil)
	assert.NoError(err)
	assert.NotNil(retCGC)

	// ******** CreateVolume
	createVolumeParams := &csi.CreateVolumeRequest{
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: 10000000000,
		},
		Parameters: map[string]string{
			com.K8sVolCtxKeyServicePlanID: "someSPName",
		},
	}
	// success
	fc := &fakeCHO{}
	fc.RetCVID = "someid"
	fc.RetCVIDErr = nil
	svc.ControllerOps = fc
	retvalCV, err := svc.CreateVolume(nil, createVolumeParams)
	assert.NoError(err)
	assert.NotNil(retvalCV)

	// sp name not provided
	createVolumeParams.Parameters = nil
	retvalCV, err = svc.CreateVolume(nil, createVolumeParams)
	assert.NotNil(err)
	assert.Empty(retvalCV)
	createVolumeParams.Parameters = map[string]string{
		com.K8sVolCtxKeyServicePlanID: "someSPName",
	} // reset

	// failure to get id
	fc = &fakeCHO{}
	fc.RetCVID = ""
	fc.RetCVIDErr = fmt.Errorf("somerror")
	svc.ControllerOps = fc
	retvalCV, err = svc.CreateVolume(nil, nil)
	assert.NotNil(err)
	assert.Empty(retvalCV)

	// ******** DeleteVolume
	deleteVolumeParams := &csi.DeleteVolumeRequest{
		VolumeId: "SomeID",
	}

	// success
	fc = &fakeCHO{}
	fc.RetDVErr = nil
	svc.ControllerOps = fc
	retvalDV, err := svc.DeleteVolume(nil, deleteVolumeParams)
	assert.NoError(err)
	assert.NotNil(retvalDV)
	assert.Equal(deleteVolumeParams.VolumeId, fc.InDVID)

	// error in deleting volume
	fc = &fakeCHO{}
	fc.RetDVErr = fmt.Errorf("somerror")
	svc.ControllerOps = fc
	retvalDV, err = svc.DeleteVolume(nil, deleteVolumeParams)
	assert.NotNil(err)
	assert.Nil(retvalDV)
	assert.Equal(deleteVolumeParams.VolumeId, fc.InDVID)
}
