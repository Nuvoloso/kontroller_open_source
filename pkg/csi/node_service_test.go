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
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	mockcluster "github.com/Nuvoloso/kontroller/pkg/cluster/mock"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	csi "github.com/Nuvoloso/kontroller/pkg/csi/csi_pb"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNodeService(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	svc := &csiService{}
	svc.Log = tl.Logger()

	// ****************** Register
	svc.server = grpc.NewServer()
	svc.RegisterNodeServer()

	// ****************** NodeGetInfo
	// success
	svc.Node = &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "id",
			},
		},
	}
	retNGI, err := svc.NodeGetInfo(nil, nil)
	assert.NoError(err)
	assert.NotNil(retNGI)
	assert.Equal(string(svc.Node.Meta.ID), retNGI.NodeId)

	// error
	svc.Node.Meta.ID = ""
	retNGI, err = svc.NodeGetInfo(nil, nil)
	assert.Error(err)
	assert.Nil(retNGI)

	svc.Node = nil
	retNGI, err = svc.NodeGetInfo(nil, nil)
	assert.Error(err)
	assert.Nil(retNGI)

	// ****************** NodeGetCapabilities
	retNGC, err := svc.NodeGetCapabilities(nil, nil)
	assert.NoError(err)
	assert.NotNil(retNGC)
	assert.Equal(0, len(retNGC.Capabilities))

	// ****************** NodePublishVolume

	// success
	svc.Node = &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "id",
			},
		},
	}
	svc.Cluster = &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "id",
			},
		},
		ClusterCreateOnce: models.ClusterCreateOnce{
			CspDomainID: models.ObjIDMutable("DomainID"),
		},
	}
	fakePublishParams := &csi.NodePublishVolumeRequest{
		VolumeId:   cluster.K8sPvNamePrefix + "someID",
		TargetPath: "test/target/path",
		Secrets:    map[string]string{},
	}
	smv := cluster.SecretObjMV{}
	smv.Intent = cluster.SecretIntentDynamicVolumeCustomization
	smv.CustomizationData.AccountSecret = "accountSecret"
	smv.CustomizationData.ConsistencyGroupName = "cgName"
	fakePublishParams.Secrets[com.K8sSecretKey] = string(smv.Marshal())

	fakeAccount := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{
				ID: "AccountID",
			},
		},
	}
	volumeContext := map[string]string{
		com.K8sVolCtxKeyVolumeSeriesName: "vsName",
		com.K8sVolCtxKeyServicePlanID:    "spID",
		com.K8sVolCtxKeySizeBytes:        "12345",
		CsiK8sPodNamespaceKey:            "podNamespace",
		CsiPodNameKey:                    "podName",
	}

	volumeCapability := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{
				FsType: "ext4",
			},
		},
	}

	// non dynamic success
	fc := &fakeNHO{}
	fc.RetMountErr = nil
	fc.RetGas = fakeAccount
	svc.NodeOps = fc
	fakePublishParams.VolumeContext = volumeContext
	fakePublishParams.VolumeCapability = volumeCapability
	retNPV, err := svc.NodePublishVolume(nil, fakePublishParams)
	assert.NoError(err)
	assert.NotNil(retNPV)
	assert.Equal("", fc.InMountArgs.Name)
	assert.Equal("", fc.InMountArgs.ServicePlan)
	assert.EqualValues(0, fc.InMountArgs.SizeBytes)
	assert.Equal(fakePublishParams.VolumeId, fc.InMountArgs.VolumeID)
	assert.Equal(string(svc.Node.Meta.ID), fc.InMountArgs.NodeID)
	assert.Equal(fakePublishParams.TargetPath, fc.InMountArgs.TargetPath)
	assert.Equal(fakePublishParams.Readonly, fc.InMountArgs.ReadOnly)
	assert.Equal(fakeAccount.Meta.ID, fc.InMountArgs.Account.Meta.ID)
	assert.False(fc.InMountArgs.Dynamic)
	assert.Equal(fakePublishParams.VolumeContext[CsiK8sPodNamespaceKey], fc.InMountArgs.PodNamespace)
	assert.Equal(fakePublishParams.VolumeContext[CsiPodNameKey], fc.InMountArgs.PodName)
	assert.NotNil(fc.InMountArgs.AVD)
	assert.Equal("ext4", fc.InMountArgs.FsType)

	// dynamic success
	fc = &fakeNHO{}
	fc.RetMountErr = nil
	fc.RetGas = fakeAccount
	svc.NodeOps = fc
	volumeContext[com.K8sVolCtxKeyDynamic] = "true"
	fakePublishParams.VolumeContext = volumeContext
	fakePublishParams.VolumeCapability = &csi.VolumeCapability{}
	retNPV, err = svc.NodePublishVolume(nil, fakePublishParams)
	assert.NoError(err)
	assert.NotNil(retNPV)
	assert.Equal(volumeContext[com.K8sVolCtxKeyVolumeSeriesName], fc.InMountArgs.Name)
	assert.Equal(volumeContext[com.K8sVolCtxKeyServicePlanID], fc.InMountArgs.ServicePlan)
	assert.EqualValues(12345, fc.InMountArgs.SizeBytes)
	assert.Equal(fakePublishParams.VolumeId, fc.InMountArgs.VolumeID)
	assert.Equal(string(svc.Node.Meta.ID), fc.InMountArgs.NodeID)
	assert.Equal(fakePublishParams.TargetPath, fc.InMountArgs.TargetPath)
	assert.Equal(fakePublishParams.Readonly, fc.InMountArgs.ReadOnly)
	assert.Equal(fakeAccount.Meta.ID, fc.InMountArgs.Account.Meta.ID)
	assert.True(fc.InMountArgs.Dynamic)
	assert.Equal(fakePublishParams.VolumeContext[CsiK8sPodNamespaceKey], fc.InMountArgs.PodNamespace)
	assert.Equal(fakePublishParams.VolumeContext[CsiPodNameKey], fc.InMountArgs.PodName)
	assert.NotNil(fc.InMountArgs.AVD)
	assert.Equal("", fc.InMountArgs.FsType)

	// dynamic extract params failure
	delete(volumeContext, com.K8sVolCtxKeyVolumeSeriesName)
	fc = &fakeNHO{}
	fc.RetMountErr = nil
	fc.RetGas = fakeAccount
	svc.NodeOps = fc
	fakePublishParams.VolumeContext = volumeContext
	retNPV, err = svc.NodePublishVolume(nil, fakePublishParams)
	assert.Error(err)
	assert.Nil(retNPV)
	assert.Regexp("volume context invalid", err.Error())
	st := status.Convert(err)
	assert.Equal(codes.InvalidArgument, st.Code())
	delete(volumeContext, com.K8sVolCtxKeyDynamic) // disable dynamic for rest of test

	// Account fetch failure
	fc.RetGasErr = fmt.Errorf("account fetch err")
	retNPV, err = svc.NodePublishVolume(nil, fakePublishParams)
	assert.Error(err)
	assert.Nil(retNPV)
	assert.Regexp("account fetch err", err.Error())
	st = status.Convert(err)
	assert.Equal(codes.PermissionDenied, st.Code())
	fc.RetGasErr = nil // reset

	// bad volume id
	fakePublishParams.VolumeId = ""
	svc.NodeOps = fc
	retNPV, err = svc.NodePublishVolume(nil, fakePublishParams)
	assert.Error(err)
	assert.Regexp("NodePublishVolume volID invalid", err.Error())
	assert.Nil(retNPV)

	// empty target path
	fakePublishParams.VolumeId = cluster.K8sPvNamePrefix + "someID"
	fakePublishParams.TargetPath = ""
	svc.NodeOps = fc
	retNPV, err = svc.NodePublishVolume(nil, fakePublishParams)
	assert.Error(err)
	assert.Regexp("NodePublishVolume targetPath invalid", err.Error())
	assert.Nil(retNPV)

	// error
	fakePublishParams.VolumeId = cluster.K8sPvNamePrefix + "someID"
	fakePublishParams.TargetPath = "test/target/path"
	fc.RetMountErr = fmt.Errorf("mount error")
	svc.NodeOps = fc
	retNPV, err = svc.NodePublishVolume(nil, fakePublishParams)
	assert.Error(err)
	assert.Regexp("mount error", err.Error())
	assert.NotNil(retNPV)

	// ****************** NodeUnpublishVolume

	// success
	fakeUnpublishParams := &csi.NodeUnpublishVolumeRequest{
		VolumeId:   cluster.K8sPvNamePrefix + "someID",
		TargetPath: "test/target/path",
	}
	fc = &fakeNHO{}
	fc.RetUnmountErr = nil
	retNUV, err := svc.NodeUnpublishVolume(nil, fakeUnpublishParams)
	assert.NoError(err)
	assert.NotNil(retNUV)

	// bad VolumeID
	fakeUnpublishParams.VolumeId = ""
	svc.NodeOps = fc
	retNUV, err = svc.NodeUnpublishVolume(nil, fakeUnpublishParams)
	assert.Error(err)
	assert.Regexp("NodeUnpublishVolume volID invalid", err.Error())
	assert.Nil(retNUV)

	// empty target path
	fakeUnpublishParams.VolumeId = "someID"
	fakeUnpublishParams.TargetPath = ""
	svc.NodeOps = fc
	retNUV, err = svc.NodeUnpublishVolume(nil, fakeUnpublishParams)
	assert.Error(err)
	assert.Regexp("NodeUnpublishVolume targetPath invalid", err.Error())
	assert.Nil(retNUV)

	// error
	fakeUnpublishParams.VolumeId = "someID"
	fakeUnpublishParams.TargetPath = "test/target/path"
	fc.RetUnmountErr = fmt.Errorf("unmount error")
	svc.NodeOps = fc
	retNUV, err = svc.NodeUnpublishVolume(nil, fakeUnpublishParams)
	assert.Error(err)
	assert.Regexp("unmount error", err.Error())
	assert.NotNil(retNUV)
}

func TestExtractCustomizationData(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	svc := &csiService{}
	svc.Log = tl.Logger()

	// ****************** Register
	svc.server = grpc.NewServer()
	svc.RegisterNodeServer()

	svc.Cluster = &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "id",
			},
		},
		ClusterCreateOnce: models.ClusterCreateOnce{
			CspDomainID: models.ObjIDMutable("DomainID"),
		},
	}

	fakePublishParams := &csi.NodePublishVolumeRequest{
		VolumeId:   cluster.K8sPvNamePrefix + "someID",
		TargetPath: "test/target/path",
		Secrets: map[string]string{
			com.K8sSecretKey: "invalid-secret-data",
		},
		VolumeContext: map[string]string{
			CsiK8sPodNamespaceKey: "someNamespace",
		},
	}

	sfa := cluster.SecretFetchArgs{
		Name:      com.AccountSecretClusterObjectName,
		Namespace: "someNamespace",
	}
	fakeSecret := &cluster.SecretObjMV{}
	fakeSecret.Name = sfa.Name
	fakeSecret.Namespace = sfa.Namespace
	fakeSecret.Intent = cluster.SecretIntentAccountIdentity
	fakeSecret.CustomizationData.AccountSecret = "accountSecret"

	ctx := context.Background()

	t.Log("case: unmarshal secret error")
	avd, err := svc.extractCustomizationData(ctx, fakePublishParams)
	assert.Error(err)
	assert.Regexp("unmarshal:", err)
	assert.Nil(avd)

	t.Log("case: missing account information")
	smv := cluster.SecretObjMV{}
	smv.Data = map[string]string{"foo": "bar"}
	fakePublishParams.Secrets[com.K8sSecretKey] = string(smv.Marshal())
	avd, err = svc.extractCustomizationData(ctx, fakePublishParams)
	assert.Error(err)
	assert.Regexp("missing account information", err)
	assert.Nil(avd)

	t.Log("case: success with param secret (SecretIntentAccountIdentity)")
	smv = cluster.SecretObjMV{}
	smv.Intent = cluster.SecretIntentAccountIdentity
	smv.CustomizationData.AccountSecret = "accountSecret"
	fakePublishParams.Secrets[com.K8sSecretKey] = string(smv.Marshal())
	avd, err = svc.extractCustomizationData(ctx, fakePublishParams)
	assert.NoError(err)
	assert.NotNil(avd)
	assert.Equal(&smv.CustomizationData, avd)

	t.Log("case: success with param secret (SecretIntentDynamicVolumeCustomization)")
	smv = cluster.SecretObjMV{}
	smv.Intent = cluster.SecretIntentDynamicVolumeCustomization
	smv.CustomizationData.AccountSecret = "accountSecret"
	smv.CustomizationData.ConsistencyGroupName = "cgName"
	fakePublishParams.Secrets[com.K8sSecretKey] = string(smv.Marshal())
	avd, err = svc.extractCustomizationData(ctx, fakePublishParams)
	assert.NoError(err)
	assert.NotNil(avd)
	assert.Equal(&smv.CustomizationData, avd)

	t.Log("case: Success with secretFetch")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	fakePublishParams.Secrets = nil
	mCluster := mockcluster.NewMockClient(mockCtrl)
	svc.ClusterClient = mCluster
	mCluster.EXPECT().SecretFetchMV(ctx, &sfa).Return(fakeSecret, nil)
	avd, err = svc.extractCustomizationData(ctx, fakePublishParams)
	assert.Nil(err)
	assert.NotNil(avd)

	t.Log("case: Error with secretFetch")
	mockCtrl = gomock.NewController(t)
	mCluster = mockcluster.NewMockClient(mockCtrl)
	svc.ClusterClient = mCluster
	mCluster.EXPECT().SecretFetchMV(ctx, &sfa).Return(nil, fmt.Errorf("secret fetch error"))
	avd, err = svc.extractCustomizationData(ctx, fakePublishParams)
	assert.NotNil(err)
	assert.Regexp("secret fetch error", err.Error())
	assert.Nil(avd)
}

func TestAuthorize(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	svc := &csiService{}
	svc.Log = tl.Logger()

	// ****************** Register
	svc.server = grpc.NewServer()
	svc.RegisterNodeServer()

	svc.Cluster = &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "id",
			},
		},
		ClusterCreateOnce: models.ClusterCreateOnce{
			CspDomainID: models.ObjIDMutable("DomainID"),
		},
	}

	fakeAccount := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{
				ID: "AccountID",
			},
		},
	}

	avd := &cluster.AccountVolumeData{
		AccountSecret: "accountSecret",
	}

	ctx := context.Background()

	// ****************** Success with account fetch
	fc := &fakeNHO{}
	fc.RetGas = fakeAccount
	svc.NodeOps = fc
	account, err := svc.authorize(ctx, avd)
	assert.Nil(err)
	assert.NotNil(account)
	assert.EqualValues(fakeAccount, account)

	// ****************** accountFetchErr
	fc.RetGasErr = fmt.Errorf("fetch error")
	account, err = svc.authorize(ctx, avd)
	assert.NotNil(err)
	assert.Regexp("fetch error", err.Error())
	assert.Nil(account)
}

func TestExtractDynamicParameters(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	svc := &csiService{}
	svc.Log = tl.Logger()

	volumeContext := map[string]string{
		com.K8sVolCtxKeyDynamic:          "true",
		com.K8sVolCtxKeyVolumeSeriesName: "vsName",
		com.K8sVolCtxKeyServicePlanID:    "spID",
		com.K8sVolCtxKeySizeBytes:        "12345",
	}

	v, s, b, err := svc.extractDynamicParams(nil, volumeContext)
	assert.Nil(err)
	assert.Equal(volumeContext[com.K8sVolCtxKeyVolumeSeriesName], v)
	assert.Equal(volumeContext[com.K8sVolCtxKeyServicePlanID], s)
	assert.EqualValues(12345, b)

	// invalid cases
	invalidTCs := []map[string]string{
		map[string]string{},
		map[string]string{com.K8sVolCtxKeyVolumeSeriesName: "a", com.K8sVolCtxKeyServicePlanID: "a", com.K8sVolCtxKeySizeBytes: "a"},
		map[string]string{com.K8sVolCtxKeyServicePlanID: "a", com.K8sVolCtxKeySizeBytes: "1"},
		map[string]string{com.K8sVolCtxKeyVolumeSeriesName: "a", com.K8sVolCtxKeySizeBytes: "1"},
		map[string]string{com.K8sVolCtxKeyVolumeSeriesName: "a", com.K8sVolCtxKeyServicePlanID: "a"},
	}
	for _, tc := range invalidTCs {
		_, _, _, err := svc.extractDynamicParams(nil, tc)
		assert.NotNil(err)
	}
}
