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

	com "github.com/Nuvoloso/kontroller/pkg/common"
	csi "github.com/Nuvoloso/kontroller/pkg/csi/csi_pb"
	"github.com/golang/protobuf/ptypes/wrappers"
	"google.golang.org/grpc"
)

func (h *csiService) RegisterIdentityServer() {
	grpcSRV := h.server.(*grpc.Server)
	csi.RegisterIdentityServer(grpcSRV, h)
}

// GetPluginInfo returns the version and name of the plugin
func (h *csiService) GetPluginInfo(ctx context.Context, in *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	h.Log.Debug("CSI: GetPluginInfo")
	return &csi.GetPluginInfoResponse{
		Name:          com.CSIDriverName,
		VendorVersion: h.Version,
	}, nil
}

// GetPluginCapabilities returns available capabilities of the plugin
func (h *csiService) GetPluginCapabilities(ctx context.Context, req *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	h.Log.Debug("CSI: GetPluginCapabilities")
	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: []*csi.PluginCapability{
			{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_CONTROLLER_SERVICE,
					},
				},
			},
		},
	}, nil
}

// Probe returns the health and readiness of the plugin
func (h *csiService) Probe(ctx context.Context, req *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	h.Log.Debug("CSI: Probe")
	isReady := false
	if h.NodeOps != nil {
		isReady = h.NodeOps.IsReady()
	} else if h.ControllerOps != nil {
		isReady = h.ControllerOps.IsReady()
	}
	return &csi.ProbeResponse{Ready: &wrappers.BoolValue{Value: isReady}}, nil
}
