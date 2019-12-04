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
	"fmt"
	"strconv"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	csi "github.com/Nuvoloso/kontroller/pkg/csi/csi_pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (h *csiService) RegisterNodeServer() {
	grpcSRV := h.server.(*grpc.Server)
	csi.RegisterNodeServer(grpcSRV, h)
}

// NodePublishVolume mount the volume from staging to target path
func (h *csiService) NodePublishVolume(ctx context.Context, params *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	h.Log.Debug("CSI: NodePublishVolume", h.dump(params))
	var avd *cluster.AccountVolumeData
	var account *models.Account
	var err error
	if avd, err = h.extractCustomizationData(ctx, params); err == nil {
		account, err = h.authorize(ctx, avd)
	}
	if err != nil {
		h.Log.Errorf("CSI: NodePublishVolume invalid secret: %s", err.Error())
		return nil, status.Errorf(codes.PermissionDenied, "NodePublishVolume not authorized: %s", err.Error())
	}
	dynamic := false
	var vsName string
	var servicePlan string
	var sizeBytes int64
	if _, ok := params.VolumeContext[com.K8sVolCtxKeyDynamic]; ok {
		vsName, servicePlan, sizeBytes, err = h.extractDynamicParams(ctx, params.VolumeContext)
		if err != nil {
			h.Log.Errorf("CSI: NodePublishVolume dynamic: %s", err.Error())
			return nil, status.Error(codes.InvalidArgument, "NodePublishVolume volume context invalid")
		}
		dynamic = true
	}
	volumeID := params.VolumeId
	if volumeID == "" {
		h.Log.Errorf("CSI: NodePublishVolume volID [%s] invalid", volumeID)
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume volID invalid")
	}
	if params.TargetPath == "" {
		h.Log.Errorf("CSI: NodePublishVolume targetPath invalid")
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume targetPath invalid")
	}
	fsType := ""
	if mount := params.VolumeCapability.GetMount(); mount != nil {
		fsType = mount.FsType
	}
	mountArgs := &MountArgs{
		Name:         vsName,
		ServicePlan:  servicePlan,
		SizeBytes:    sizeBytes,
		VolumeID:     volumeID,
		NodeID:       string(h.Node.Meta.ID),
		TargetPath:   params.TargetPath,
		ReadOnly:     params.Readonly,
		Account:      account,
		Dynamic:      dynamic,
		AVD:          avd,
		PodName:      params.VolumeContext[CsiPodNameKey],
		PodNamespace: params.VolumeContext[CsiK8sPodNamespaceKey],
		FsType:       fsType,
	}
	// TBD validate - ReadWriteOnce mode only
	err = h.NodeOps.MountVolume(ctx, mountArgs)
	if err != nil {
		h.Log.Errorf("CSI: NodePublishVolume volID [%s]: %s", volumeID, err.Error())
	}
	return &csi.NodePublishVolumeResponse{}, err
}

func (h *csiService) extractDynamicParams(ctx context.Context, volumeContext map[string]string) (string, string, int64, error) {
	var err error
	vsName, ok1 := volumeContext[com.K8sVolCtxKeyVolumeSeriesName]
	servicePlan, ok2 := volumeContext[com.K8sVolCtxKeyServicePlanID]
	sizeByteString, ok3 := volumeContext[com.K8sVolCtxKeySizeBytes]
	sizeBytes, err := strconv.ParseInt(sizeByteString, 10, 64)
	if !(ok1 && ok2 && ok3) || err != nil {
		return "", "", 0, fmt.Errorf("invalid volume context parameters (%v,%v,%v)", ok1, ok2, ok3)
	}
	return vsName, servicePlan, sizeBytes, nil
}

// extractCustomizationData extracts data on the account and volume from params, either directly or indirectly:
// no secret data present in params for pre-provisioned volumes - must fetch the account secret.
func (h *csiService) extractCustomizationData(ctx context.Context, params *csi.NodePublishVolumeRequest) (*cluster.AccountVolumeData, error) {
	var sMV *cluster.SecretObjMV
	var err error
	if val, ok := params.Secrets[com.K8sSecretKey]; !ok {
		sfa := cluster.SecretFetchArgs{
			Name:      com.AccountSecretClusterObjectName,
			Namespace: params.VolumeContext[CsiK8sPodNamespaceKey],
		}
		sMV, err = h.ClusterClient.SecretFetchMV(ctx, &sfa)
		if err != nil {
			return nil, err
		}
	} else {
		sMV = &cluster.SecretObjMV{}
		if err = sMV.Unmarshal([]byte(val)); err != nil {
			return nil, fmt.Errorf("unmarshal: %s", err.Error())
		}
	}
	if sMV.Intent != cluster.SecretIntentAccountIdentity && sMV.Intent != cluster.SecretIntentDynamicVolumeCustomization {
		return nil, fmt.Errorf("missing account information")
	}
	return &sMV.CustomizationData, nil
}

// authorize Authorizes a call to use the mount operation.
func (h *csiService) authorize(ctx context.Context, avd *cluster.AccountVolumeData) (*models.Account, error) {
	afArgs := &AccountFetchArgs{
		Secret:      avd.AccountSecret,
		ClusterID:   string(h.Cluster.Meta.ID),
		CSPDomainID: string(h.Cluster.CspDomainID),
	}
	account, err := h.NodeOps.GetAccountFromSecret(ctx, afArgs)
	if err != nil {
		return nil, err
	}
	return account, nil
}

// NodeUnpublishVolume unmounts volume from target path
func (h *csiService) NodeUnpublishVolume(ctx context.Context, params *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	h.Log.Debug("CSI: NodeUnpublishVolume", h.dump(params))
	volumeID := params.VolumeId
	if volumeID == "" {
		h.Log.Errorf("CSI: NodeUnpublishVolume volID [%s] invalid", volumeID)
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume volID invalid")
	}
	if params.TargetPath == "" {
		h.Log.Errorf("CSI: NodeUnpublishVolume targetPath invalid")
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume targetPath invalid")
	}
	unmountArgs := &UnmountArgs{
		VolumeID:   volumeID,
		TargetPath: params.TargetPath,
	}
	err := h.NodeOps.UnmountVolume(ctx, unmountArgs)
	if err != nil {
		h.Log.Errorf("CSI: NodeUnpublishVolume volID [%s]: %s", volumeID, err.Error())
	}
	return &csi.NodeUnpublishVolumeResponse{}, err
}

// NodeGetCapabilities returns the capabilites of node
func (h *csiService) NodeGetCapabilities(ctx context.Context, params *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	h.Log.Debug("CSI: NodeGetCapabilities", h.dump(params))
	return &csi.NodeGetCapabilitiesResponse{}, nil
}

// NodeGetInfo get info including node id.
// Prior to CSI 1.0 - CSI plugins MUST implement both NodeGetId and
// NodeGetInfo RPC calls.
func (h *csiService) NodeGetInfo(context.Context, *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	h.Log.Debug("CSI: NodeGetInfo")
	if h.Node == nil || h.Node.Meta.ID == "" {
		return nil, fmt.Errorf("CSI: empty node id")
	}
	return &csi.NodeGetInfoResponse{
		NodeId: string(h.Node.Meta.ID),
	}, nil
}
