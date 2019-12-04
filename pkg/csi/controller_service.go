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
	"strconv"

	com "github.com/Nuvoloso/kontroller/pkg/common"
	csi "github.com/Nuvoloso/kontroller/pkg/csi/csi_pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (h *csiService) RegisterControllerServer() {
	grpcSRV := h.server.(*grpc.Server)
	csi.RegisterControllerServer(grpcSRV, h)
}

// CreateVolume
// Currently generates an id for a VS object; which will be used in the PV.
// TBD: If CSI addresses issues with referencing secrets during create; this will be responsible for creating volumes
func (h *csiService) CreateVolume(ctx context.Context, params *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	h.Log.Debug("CSI: CreateVolume", h.dump(params))
	volID, err := h.ControllerOps.CreateVolumeID(ctx)
	if err != nil {
		h.Log.Errorf("CSI: CreateVolume: %s", err.Error())
		return nil, err
	}
	spID, ok := params.Parameters[com.K8sVolCtxKeyServicePlanID]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "CSI: CreateVolume storageClass missing nuvoloso-service-plan parameter")
	}
	sizeBytesStr := strconv.FormatInt(params.CapacityRange.RequiredBytes, 10)
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volID,
			CapacityBytes: params.CapacityRange.RequiredBytes, // 1GB
			VolumeContext: map[string]string{
				com.K8sVolCtxKeyDynamic:          "dynamic",
				com.K8sVolCtxKeySizeBytes:        sizeBytesStr,
				com.K8sVolCtxKeyServicePlanID:    spID,
				com.K8sVolCtxKeyVolumeSeriesName: params.Name,
			},
		},
	}, nil
}

// DeleteVolume
// Currently will be stubbed out.
// TBD: In the future it will unbind the VS; and based on policy will delete the VS object
func (h *csiService) DeleteVolume(ctx context.Context, params *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	h.Log.Debug("CSI: DeleteVolume", h.dump(params))
	err := h.ControllerOps.DeleteVolume(ctx, params.VolumeId)
	if err != nil {
		h.Log.Errorf("CSI: DeleteVolume volID [%s]: %s", params.VolumeId, err.Error())
		return nil, err
	}
	return &csi.DeleteVolumeResponse{}, nil
}

// ControllerGetCapabilities returns capabilities of Controller plugin
func (h *csiService) ControllerGetCapabilities(ctx context.Context, params *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	h.Log.Debug("CSI: ControllerGetCapabilities")
	cdCap := &csi.ControllerServiceCapability{
		Type: &csi.ControllerServiceCapability_Rpc{
			Rpc: &csi.ControllerServiceCapability_RPC{
				Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
			},
		},
	}
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: []*csi.ControllerServiceCapability{
			cdCap,
		},
	}, nil
}
