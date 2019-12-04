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
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	fa "github.com/Nuvoloso/kontroller/pkg/agentd/fake"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/application_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/consistency_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/snapshot"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	mockcluster "github.com/Nuvoloso/kontroller/pkg/cluster/mock"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/csi"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	fVra "github.com/Nuvoloso/kontroller/pkg/vra/fake"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMountVolume(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	appS := &fa.AppServant{}
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log: tl.Logger(),
		},
		AppServant: appS,
	}
	c := &csiComp{}
	c.Init(app)

	retVSR := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{ID: "vsr-1"},
		},
	}
	// VSR lookup failed
	fmo := &fakeMountOps{}
	op := fmo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.MountArgs{
		VolumeID: "vol-1",
	}
	fmo.checkForVSRErr = fmt.Errorf("invalid error")
	fmo.checkForVSRret = false
	expCalled := []string{"CFV"}
	err := op.run(nil)
	assert.NotNil(err)
	assert.Regexp("invalid error", err.Error())
	assert.Equal(expCalled, op.called)

	//  pending vsr, wait, success
	fmo = &fakeMountOps{}
	op = fmo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.MountArgs{
		VolumeID: "vol-1",
	}
	op.vsr = retVSR
	fmo.checkForVSRret = true
	fmo.checkForVSRErr = nil
	fmo.waitForVSRErr = nil
	expCalled = []string{"CFV", "WFV"}
	err = op.run(nil)
	assert.Nil(err)
	assert.Equal(expCalled, op.called)

	//  pending vsr, wait, success
	fmo = &fakeMountOps{}
	op = fmo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.MountArgs{
		VolumeID: "vol-1",
	}
	op.vsr = retVSR
	fmo.checkForVSRret = true
	fmo.checkForVSRErr = nil
	fmo.waitForVSRErr = fmt.Errorf("vsr failed")
	expCalled = []string{"CFV", "WFV"}
	err = op.run(nil)
	assert.NotNil(err)
	assert.Regexp("vsr failed", err.Error())
	assert.Equal(expCalled, op.called)

	// no existing vsrs, invalid state
	fmo = &fakeMountOps{}
	op = fmo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.MountArgs{
		VolumeID: "vol-1",
	}
	fmo.checkForVSRErr = nil
	fmo.checkForVSRret = false
	fmo.volPublishedState = InvalidState
	fmo.volPublishedErr = fmt.Errorf("invalid error")
	expCalled = []string{"CFV", "GVS"}
	err = op.run(nil)
	assert.NotNil(err)
	assert.Regexp("invalid error", err.Error())
	assert.Equal(expCalled, op.called)

	// no exitsing vsr, already mounted
	fmo = &fakeMountOps{}
	op = fmo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.MountArgs{
		VolumeID: "vol-1",
	}
	fmo.volPublishedState = MountedState
	expCalled = []string{"CFV", "GVS"}
	err = op.run(nil)
	assert.Nil(err)
	assert.Equal(expCalled, op.called)

	// no existing vsr, not created, create, wait, success
	fmo = &fakeMountOps{}
	op = fmo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.MountArgs{
		VolumeID: "vol-1",
	}
	fmo.volPublishedState = NotCreatedDynamicState
	fmo.createMountVSRErr = nil
	fmo.waitForVSRErr = nil
	expCalled = []string{"CFV", "GVS", "BTM", "GCG", "CMV", "WFV"}
	err = op.run(nil)
	assert.Nil(err)
	assert.Equal(expCalled, op.called)

	// no existing vsr, not created, create, wait, error
	fmo = &fakeMountOps{}
	op = fmo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.MountArgs{
		VolumeID: "vol-1",
	}
	fmo.volPublishedState = NotCreatedDynamicState
	fmo.createMountVSRErr = nil
	fmo.waitForVSRErr = fmt.Errorf("create err")
	expCalled = []string{"CFV", "GVS", "BTM", "GCG", "CMV", "WFV"}
	err = op.run(nil)
	assert.NotNil(err)
	assert.Regexp("create err", err.Error())
	assert.Equal(expCalled, op.called)

	// no existing vsr, not created, getCG fails
	fmo = &fakeMountOps{}
	op = fmo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.MountArgs{
		VolumeID: "vol-1",
	}
	fmo.volPublishedState = NotCreatedDynamicState
	fmo.createMountVSRErr = nil
	fmo.getCGErr = fmt.Errorf("getCG error")
	expCalled = []string{"CFV", "GVS", "BTM", "GCG"}
	err = op.run(nil)
	assert.NotNil(err)
	assert.Regexp("getCG error", err.Error())
	assert.Equal(expCalled, op.called)

	// not bound, create, wait, success
	fmo = &fakeMountOps{}
	op = fmo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.MountArgs{
		VolumeID: "vol-1",
	}
	fmo.volPublishedState = NotBoundState
	fmo.createMountVSRErr = nil
	fmo.waitForVSRErr = nil
	expCalled = []string{"CFV", "GVS", "CMV", "WFV"}
	err = op.run(nil)
	assert.Nil(err)
	assert.Equal(expCalled, op.called)

	// not bound, create, wait, error
	fmo = &fakeMountOps{}
	op = fmo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.MountArgs{
		VolumeID: "vol-1",
	}
	fmo.volPublishedState = NotBoundState
	fmo.createMountVSRErr = nil
	fmo.waitForVSRErr = fmt.Errorf("bind err")
	expCalled = []string{"CFV", "GVS", "CMV", "WFV"}
	err = op.run(nil)
	assert.NotNil(err)
	assert.Regexp("bind err", err.Error())
	assert.Equal(expCalled, op.called)

	// not mounted, create, wait, success
	fmo = &fakeMountOps{}
	op = fmo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.MountArgs{
		VolumeID: "vol-1",
	}
	fmo.volPublishedState = NotMountedState
	fmo.createMountVSRErr = nil
	fmo.waitForVSRErr = nil
	expCalled = []string{"CFV", "GVS", "CMV", "WFV"}
	err = op.run(nil)
	assert.Nil(err)
	assert.Equal(expCalled, op.called)

	// not mounted, create, wait, error
	fmo = &fakeMountOps{}
	op = fmo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.MountArgs{
		VolumeID: "vol-1",
	}
	fmo.volPublishedState = NotMountedState
	fmo.createMountVSRErr = nil
	fmo.waitForVSRErr = fmt.Errorf("mount err")
	expCalled = []string{"CFV", "GVS", "CMV", "WFV"}
	err = op.run(nil)
	assert.NotNil(err)
	assert.Regexp("mount err", err.Error())
	assert.Equal(expCalled, op.called)

	// not attached, create, wait, success
	fmo = &fakeMountOps{}
	op = fmo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.MountArgs{
		VolumeID: "vol-1",
	}
	fmo.volPublishedState = NoFsAttachedState
	fmo.createMountVSRErr = nil
	fmo.waitForVSRErr = nil
	expCalled = []string{"CFV", "GVS", "CMV", "WFV"}
	err = op.run(nil)
	assert.Nil(err)
	assert.Equal(expCalled, op.called)

	// not attached, create, wait, error
	fmo = &fakeMountOps{}
	op = fmo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.MountArgs{
		VolumeID: "vol-1",
	}
	fmo.volPublishedState = NoFsAttachedState
	fmo.createMountVSRErr = nil
	fmo.waitForVSRErr = fmt.Errorf("mount err")
	expCalled = []string{"CFV", "GVS", "CMV", "WFV"}
	err = op.run(nil)
	assert.NotNil(err)
	assert.Regexp("mount err", err.Error())
	assert.Equal(expCalled, op.called)

	// not attached, create, error
	fmo = &fakeMountOps{}
	op = fmo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.MountArgs{
		VolumeID: "vol-1",
	}
	fmo.volPublishedState = NoFsAttachedState
	fmo.createMountVSRErr = fmt.Errorf("vsr create err")
	expCalled = []string{"CFV", "GVS", "CMV"}
	err = op.run(nil)
	assert.NotNil(err)
	assert.Regexp("vsr create err", err.Error())
	assert.Equal(expCalled, op.called)

	// not mounted, create, wait, success
	fmo = &fakeMountOps{}
	op = fmo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.MountArgs{
		VolumeID: "vol-1",
	}
	fmo.volPublishedState = SnapRestoreState
	fmo.createMountVSRErr = nil
	fmo.waitForVSRErr = nil
	expCalled = []string{"CFV", "GVS", "CMV", "WFV"}
	err = op.run(nil)
	assert.Nil(err)
	assert.Equal(expCalled, op.called)

	// not mounted, create, wait, error
	fmo = &fakeMountOps{}
	op = fmo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.MountArgs{
		VolumeID: "vol-1",
	}
	fmo.volPublishedState = SnapRestoreState
	fmo.createMountVSRErr = nil
	fmo.waitForVSRErr = fmt.Errorf("mount err")
	expCalled = []string{"CFV", "GVS", "CMV", "WFV"}
	err = op.run(nil)
	assert.NotNil(err)
	assert.Regexp("mount err", err.Error())
	assert.Equal(expCalled, op.called)

	// call the real handler with error
	node := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "NODE-1",
			},
		},
	}
	cluster := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "CLUSTER-1",
			},
		},
	}
	domain := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID: "CLUSTER-1",
			},
		},
	}
	account := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{
				ID: "someID",
			},
		},
	}

	fao := &fakeAppObjects{}
	fao.retCluster = cluster
	fao.retNode = node
	fao.retDomain = domain
	op.app.AppObjects = fao
	retV := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{ID: "vol-1"},
		},
	}
	retV.BoundClusterID = "bad clusterid"
	fc := &fake.Client{}
	fc.RetLsVRErr = fmt.Errorf("VSR list error")
	fc.RetVObj = retV
	fc.RetVErr = nil
	c.app.OCrud = fc
	err = c.MountVolume(nil, &csi.MountArgs{VolumeID: "vol-1", TargetPath: "somepath", NodeID: "somenode", Account: account})
	assert.Error(err)
	assert.Regexp("VSR list error", err.Error())

	// args validate fails
	err = c.MountVolume(nil, &csi.MountArgs{VolumeID: ""})
	assert.Error(err)
	assert.Regexp("args invalid or missing", err.Error())
}

func TestMountSteps(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	appS := &fa.AppServant{}
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log:       tl.Logger(),
			CSISocket: "/some/Socket",
		},
		AppServant: appS,
	}
	retVS := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{ID: "vol-1"},
		},
	}
	nodeID := "node-id"
	nodeRes := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "node-id",
			},
			ClusterID: models.ObjIDMutable("clusterID"),
		},
		NodeMutable: models.NodeMutable{
			Name: "nodeName",
		},
	}

	clusterRes := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "clusterID",
			},
		},
		ClusterCreateOnce: models.ClusterCreateOnce{
			CspDomainID: models.ObjIDMutable("CSP-DOMAIN-1"),
		},
		ClusterMutable: models.ClusterMutable{
			ClusterCreateMutable: models.ClusterCreateMutable{
				Name: "clusterName",
			},
		},
	}

	accountRes := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{
				ID: "accountID",
			},
		},
		AccountMutable: models.AccountMutable{
			Name: "System",
		},
	}

	cgRes := &models.ConsistencyGroup{
		ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
			Meta: &models.ObjMeta{
				ID: "cgID",
			},
		},
		ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
			AccountID: models.ObjIDMutable("accountID"),
		},
	}

	op := mountOp{
		args: &csi.MountArgs{
			VolumeID:    "vol-1",
			Name:        "vsName",
			ServicePlan: "servicePlanID",
			Account:     accountRes,
			AVD:         &cluster.AccountVolumeData{},
		},
		node:    nodeRes,
		cluster: clusterRes,
		vs:      retVS,
		c:       &csiComp{},
	}
	op.c.Init(app)

	retVSR := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{ID: "vsr-1"},
		},
	}

	retSnap := &models.Snapshot{
		SnapshotAllOf0: models.SnapshotAllOf0{
			Meta: &models.ObjMeta{
				ID: "snapID",
			},
		},
	}
	fc := &fake.Client{}

	// *********************** checkForVSR

	// list empty
	op.vsr = nil
	fc.RetLsVRErr = nil
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{Payload: []*models.VolumeSeriesRequest{}}
	op.app = app
	op.app.OCrud = fc
	conflictVSRs, err := op.checkForVSR(nil)
	assert.False(conflictVSRs)
	assert.Nil(err)
	assert.Nil(op.vsr)
	assert.Equal([]string{fmt.Sprintf("%s:%s", com.SystemTagVsrK8sNodePublish, op.args.VolumeID)}, fc.InLsVRObj.SystemTags)

	// list empty, dynamic
	op.vsr = nil
	fc.RetLsVRErr = nil
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{Payload: []*models.VolumeSeriesRequest{}}
	op.app = app
	op.app.OCrud = fc
	op.args.Dynamic = true
	conflictVSRs, err = op.checkForVSR(nil)
	assert.False(conflictVSRs)
	assert.Nil(err)
	assert.Nil(op.vsr)
	assert.Equal([]string{fmt.Sprintf("%s:%s", com.SystemTagVsrK8sNodePublish, op.args.VolumeID)}, fc.InLsVRObj.SystemTags)
	assert.Nil(fc.InLsVRObj.VolumeSeriesID)

	// pending mount vsrs
	op.vsr = nil
	fc.RetLsVRErr = nil
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{Payload: []*models.VolumeSeriesRequest{retVSR}}
	op.app = app
	op.app.OCrud = fc
	conflictVSRs, err = op.checkForVSR(nil)
	assert.True(conflictVSRs)
	assert.Nil(err)
	assert.Equal(retVSR, op.vsr)

	// vsr list failed
	fc.RetLsVRErr = fmt.Errorf("vsr list failed")
	op.app = app
	op.app.OCrud = fc
	conflictVSRs, err = op.checkForVSR(nil)
	assert.False(conflictVSRs)
	assert.NotNil(err)
	assert.Regexp("vsr list failed", err.Error())
	assert.Equal(1, tl.CountPattern("vsr list failed"))
	tl.Flush()

	// ***********************createMountVSR

	// success, static
	fc.RetVRCErr = nil
	fc.RetVRCObj = retVSR
	op.vsr = nil
	op.args.Dynamic = false
	err = op.createMountVSR(nil)
	assert.Nil(err)
	assert.Equal(retVSR, op.vsr)
	assert.Equal(models.ObjIDMutable(op.args.VolumeID), fc.InVRCArgs.VolumeSeriesID)
	assert.Nil(fc.InVRCArgs.VolumeSeriesCreateSpec)
	assert.Equal(models.ObjTags([]string{fmt.Sprintf("%s:%s", com.SystemTagVsrK8sNodePublish, op.args.VolumeID)}), fc.InVRCArgs.SystemTags)

	// success, static
	fc.RetVRCErr = nil
	fc.RetVRCObj = retVSR
	op.vsr = nil
	op.args.Dynamic = false
	op.vsState = SnapRestoreState
	op.snapshot = retSnap
	err = op.createMountVSR(nil)
	assert.Nil(err)
	assert.Equal(retVSR, op.vsr)
	assert.Equal(models.ObjIDMutable(op.args.VolumeID), fc.InVRCArgs.VolumeSeriesID)
	assert.Nil(fc.InVRCArgs.VolumeSeriesCreateSpec)
	assert.Equal(models.ObjTags([]string{fmt.Sprintf("%s:%s", com.SystemTagVsrK8sNodePublish, op.args.VolumeID)}), fc.InVRCArgs.SystemTags)
	assert.Equal(models.ObjIDMutable(retSnap.Meta.ID), fc.InVRCArgs.SnapshotID)
	op.snapshot = nil // reset

	// success, dynamic
	fc.RetVRCErr = nil
	fc.RetVRCObj = retVSR
	op.vsr = nil
	op.cg = cgRes
	op.args.Dynamic = true
	op.args.AVD.VolumeTags = []string{"tag1", "tag2"}
	op.vsState = NotCreatedDynamicState
	err = op.createMountVSR(nil)
	assert.Nil(err)
	assert.Equal(retVSR, op.vsr)
	assert.Empty(fc.InVRCArgs.VolumeSeriesID)
	assert.NotNil(fc.InVRCArgs.VolumeSeriesCreateSpec)
	assert.Equal(models.ObjIDMutable(op.args.Account.Meta.ID), fc.InVRCArgs.VolumeSeriesCreateSpec.AccountID)
	assert.Equal(models.ObjName(op.args.Name), fc.InVRCArgs.VolumeSeriesCreateSpec.Name)
	assert.Equal(models.ObjIDMutable(op.args.ServicePlan), fc.InVRCArgs.VolumeSeriesCreateSpec.ServicePlanID)
	assert.Equal(swag.Int64(op.args.SizeBytes), fc.InVRCArgs.VolumeSeriesCreateSpec.SizeBytes)
	assert.Equal(models.ObjTags([]string{fmt.Sprintf("%s:%s", com.SystemTagVolumeSeriesID, op.args.VolumeID)}),
		fc.InVRCArgs.VolumeSeriesCreateSpec.SystemTags)
	assert.Equal(models.ObjTags([]string{fmt.Sprintf("%s:%s", com.SystemTagVsrK8sNodePublish, op.args.VolumeID)}), fc.InVRCArgs.SystemTags)
	assert.Equal(models.ObjIDMutable(cgRes.Meta.ID), fc.InVRCArgs.VolumeSeriesCreateSpec.ConsistencyGroupID)
	assert.Equal(models.ObjTags(op.args.AVD.VolumeTags), fc.InVRCArgs.VolumeSeriesCreateSpec.Tags)
	assert.NotNil(fc.InVRCArgs.VolumeSeriesCreateSpec.ClusterDescriptor)
	assert.Contains(fc.InVRCArgs.VolumeSeriesCreateSpec.ClusterDescriptor, com.ClusterDescriptorPVName)
	assert.Equal(op.args.Name, fc.InVRCArgs.VolumeSeriesCreateSpec.ClusterDescriptor[com.ClusterDescriptorPVName].Value)

	// success, dynamic (no create)
	fc.RetVRCErr = nil
	fc.RetVRCObj = retVSR
	op.vsr = nil
	op.args.Dynamic = true
	op.vsState = NotMountedState
	err = op.createMountVSR(nil)
	assert.Nil(err)
	assert.Equal(retVSR, op.vsr)
	assert.Equal(models.ObjIDMutable(op.args.VolumeID), fc.InVRCArgs.VolumeSeriesID)
	assert.Nil(fc.InVRCArgs.VolumeSeriesCreateSpec)
	assert.Equal(models.ObjTags([]string{fmt.Sprintf("%s:%s", com.SystemTagVsrK8sNodePublish, op.args.VolumeID)}), fc.InVRCArgs.SystemTags)

	// VSR create failed
	op.args.Dynamic = false
	tp := "/some/target/path"
	fc.RetVRCErr = fmt.Errorf("VSR create failed")
	fc.RetVRCObj = nil
	op.vsr = nil
	op.args.TargetPath = tp
	op.args.Account = accountRes
	err = op.createMountVSR(nil)
	assert.NotNil(err)
	assert.Regexp("VSR create failed", err.Error())
	assert.NotNil(op.vsr)
	assert.Equal(models.ObjIDMutable(op.args.VolumeID), op.vsr.VolumeSeriesID)
	assert.Equal(models.ObjIDMutable(nodeID), op.vsr.NodeID)
	assert.Equal(tp, op.vsr.TargetPath)
	assert.Equal(models.ObjIDMutable(accountRes.Meta.ID), op.vsr.Creator.AccountID)

	// rei error
	op.c.rei.SetProperty("csi-mount-vsr", &rei.Property{BoolValue: true})
	err = op.createMountVSR(nil)
	assert.NotNil(err)
	assert.Regexp("csi-mount-vsr", err.Error())

	// *********************** waitForVSR

	// success
	op.ops = &fakeMountOps{}
	fw := fVra.NewFakeRequestWaiter()
	retVSR.VolumeSeriesRequestState = com.VolReqStateSucceeded
	fw.Vsr = retVSR
	fw.Error = nil
	fw.Ctx, fw.Can = context.WithCancel(context.Background())
	defer fw.Can()
	op.vW = fw
	op.vsr = retVSR
	err = op.waitForVSR(nil)
	assert.Nil(err)

	// vsr failed
	op.vsr.VolumeSeriesRequestState = "FAILED"
	err = op.waitForVSR(nil)
	assert.Regexp("request.*FAILED", err)

	// vsr failed with error detail
	op.vsr.VolumeSeriesRequestState = "FAILED"
	op.vsr.RequestMessages = []*models.TimestampedString{{Message: "Error: you lose"}}
	err = op.waitForVSR(nil)
	assert.Regexp("request.*FAILED: Error: you lose", err)
	op.vsr.RequestMessages = nil

	// vsr ongoing, fetch returns error
	fw.Error = errors.New("wait failure")
	err = op.waitForVSR(nil)
	if assert.Error(err) {
		assert.Regexp("waitForVSR.*wait failure", err.Error())
	}

	// vsr ongoing, timeout expires
	fw.Error = vra.ErrRequestWaiterCtxExpired
	err = op.waitForVSR(nil)
	if assert.Error(err) {
		assert.Regexp("request.*in progress", err.Error())
	}

	// call real waiter with bad args to force panic
	op.vW = nil
	op.app.OCrud = nil
	assert.Panics(func() { op.waitForVSR(nil) })

	// *********************** getVolumeState

	// Mounted but not attached fs
	retVS.BoundClusterID = models.ObjIDMutable(clusterRes.Meta.ID)
	retVS.ConfiguredNodeID = "node-id"
	retVS.VolumeSeriesState = com.VolStateInUse
	retVS.Mounts = []*models.Mount{
		&models.Mount{
			SnapIdentifier: "HEAD",
			MountedNodeID:  "node-id",
			MountState:     "MOUNTED",
		},
	}
	fc.RetVObj = retVS
	op.app.OCrud = fc
	state, err := op.getVolumeState(nil, "vsID")
	assert.NotNil(err)
	assert.Regexp("fs is not attached", err.Error())
	assert.Equal(NoFsAttachedState, state)

	// Mounted
	retVS.BoundClusterID = models.ObjIDMutable(clusterRes.Meta.ID)
	retVS.VolumeSeriesState = com.VolStateInUse
	retVS.SystemTags = []string{com.SystemTagVolumeFsAttached}
	fc.RetVObj = retVS
	op.app.OCrud = fc
	state, err = op.getVolumeState(nil, "vsID")
	assert.Nil(err)
	assert.Equal(MountedState, state)

	// NotMounted on head
	retVS.Mounts = []*models.Mount{}
	fc.RetVObj = retVS
	op.app.OCrud = fc
	state, err = op.getVolumeState(nil, "vsID")
	assert.NotNil(err)
	assert.Regexp("is not mounted", err.Error())
	st := status.Convert(err)
	assert.Equal(codes.Internal, st.Code())
	assert.Equal(NotMountedState, state)

	// InvalidState, mounted on another node
	retVS.ConfiguredNodeID = "node-O"
	retVS.Mounts = []*models.Mount{&models.Mount{
		SnapIdentifier: "HEAD",
		MountedNodeID:  "node-O",
		MountState:     "MOUNTED",
	}}
	fc.RetVObj = retVS
	op.app.OCrud = fc
	state, err = op.getVolumeState(nil, "vsID")
	assert.NotNil(err)
	assert.Regexp("is mounted on node .node-O", err)
	st = status.Convert(err)
	assert.Equal(codes.Internal, st.Code())
	assert.Equal(InvalidState, state)

	// NotMounted vs state
	retVS.ConfiguredNodeID = "node-id"
	retVS.Mounts = []*models.Mount{&models.Mount{
		SnapIdentifier: "HEAD",
		MountedNodeID:  "node-id",
		MountState:     "MOUNTED",
	}}
	retVS.VolumeSeriesState = com.VolStateProvisioned
	fc.RetVObj = retVS
	op.app.OCrud = fc
	state, err = op.getVolumeState(nil, "vsID")
	assert.NotNil(err)
	assert.Regexp("is not mounted", err.Error())
	st = status.Convert(err)
	assert.Equal(codes.Internal, st.Code())
	assert.Equal(NotMountedState, state)

	// NotMounted vs state (BOUND)
	retVS.VolumeSeriesState = com.VolStateBound
	fc.RetVObj = retVS
	fc.RetLsSnapshotOk = &snapshot.SnapshotListOK{Payload: []*models.Snapshot{}}
	fc.RetLsSnapshotErr = nil
	op.app.OCrud = fc
	state, err = op.getVolumeState(nil, "vsID")
	assert.NotNil(err)
	assert.Regexp("is not mounted", err.Error())
	st = status.Convert(err)
	assert.Equal(codes.Internal, st.Code())
	assert.Equal(NotMountedState, state)
	assert.Nil(op.snapshot)

	// SnapshotRestoreState (bound and has snapshots)
	retVS.VolumeSeriesState = com.VolStateBound
	fc.RetVObj = retVS
	fc.RetLsSnapshotOk = &snapshot.SnapshotListOK{Payload: []*models.Snapshot{retSnap}}
	fc.RetLsSnapshotErr = nil
	op.app.OCrud = fc
	state, err = op.getVolumeState(nil, "vsID")
	assert.NotNil(err)
	assert.Regexp("being restored", err.Error())
	st = status.Convert(err)
	assert.Equal(codes.Internal, st.Code())
	assert.Equal(SnapRestoreState, state)
	assert.Equal(retSnap, op.snapshot)

	// SnapshotRestoreState (bound and has snapshots) fails
	retVS.VolumeSeriesState = com.VolStateBound
	fc.RetVObj = retVS
	fc.RetLsSnapshotOk = nil
	fc.RetLsSnapshotErr = fmt.Errorf("snap list error")
	op.app.OCrud = fc
	state, err = op.getVolumeState(nil, "vsID")
	assert.NotNil(err)
	assert.Regexp("snap list error", err.Error())
	st = status.Convert(err)
	assert.Equal(codes.Internal, st.Code())
	assert.Equal(InvalidState, state)
	assert.Nil(op.snapshot)

	// VS in bad state
	retVS.VolumeSeriesState = "BAD STATE"
	fc.RetVObj = retVS
	op.app.OCrud = fc
	state, err = op.getVolumeState(nil, "vsID")
	assert.NotNil(err)
	assert.Regexp("is not mountable", err.Error())
	st = status.Convert(err)
	assert.Equal(codes.Internal, st.Code())
	assert.Equal(InvalidState, state)

	// clusterID mismatch
	retVS.BoundClusterID = "wrongID"
	fc.RetVObj = retVS
	op.app.OCrud = fc
	state, err = op.getVolumeState(nil, "vsID")
	assert.NotNil(err)
	assert.Regexp("not bound to cluster", err.Error())
	st = status.Convert(err)
	assert.Equal(codes.Internal, st.Code())
	assert.Equal(InvalidState, state)

	// fetch error
	fc.RetVErr = fmt.Errorf("fetch error")
	op.app.OCrud = fc
	state, err = op.getVolumeState(nil, "vsID")
	assert.NotNil(err)
	assert.Regexp("fetch failed", err.Error())
	st = status.Convert(err)
	assert.Equal(codes.Internal, st.Code())
	assert.Equal(InvalidState, state)

	// vs not found
	fc.RetVErr = &crud.Error{Payload: models.Error{Code: http.StatusNotFound, Message: swag.String(com.ErrorNotFound)}}
	op.app.OCrud = fc
	state, err = op.getVolumeState(nil, "vsID")
	assert.NotNil(err)
	assert.Regexp("not found", err.Error())
	st = status.Convert(err)
	assert.Equal(codes.NotFound, st.Code())
	assert.Equal(InvalidState, state)

	// vs not found, not created
	fc.RetVErr = &crud.Error{Payload: models.Error{Code: http.StatusNotFound, Message: swag.String(com.ErrorNotFound)}}
	op.app.OCrud = fc
	op.args.Dynamic = true
	state, err = op.getVolumeState(nil, "vsID")
	assert.NotNil(err)
	assert.Regexp("not yet created", err.Error())
	st = status.Convert(err)
	assert.Equal(codes.Internal, st.Code())
	assert.Equal(NotCreatedDynamicState, state)

	// boundClusterID mismatch && dynamic; error
	retVS.BoundClusterID = "wrongID"
	fc.RetVErr = nil
	fc.RetVObj = retVS
	op.app.OCrud = fc
	op.args.Dynamic = true
	state, err = op.getVolumeState(nil, "vsID")
	assert.NotNil(err)
	assert.Regexp("not bound to cluster", err.Error())
	st = status.Convert(err)
	assert.Equal(codes.Internal, st.Code())
	assert.Equal(InvalidState, state)

	// *********************** opsForState
	op.vsState = NotCreatedDynamicState
	ops := op.opsForState()
	assert.Equal([]string{com.VolReqOpCreate, com.VolReqOpBind, com.VolReqOpMount, com.VolReqOpAttachFs}, ops)

	op.vsState = NotBoundState
	ops = op.opsForState()
	assert.Equal([]string{com.VolReqOpBind, com.VolReqOpMount, com.VolReqOpAttachFs}, ops)

	op.vsState = NotMountedState
	ops = op.opsForState()
	assert.Equal([]string{com.VolReqOpMount, com.VolReqOpAttachFs}, ops)

	op.vsState = NoFsAttachedState
	ops = op.opsForState()
	assert.Equal([]string{com.VolReqOpAttachFs}, ops)

	// *********************** fetchLatestSnapshot

	// single snapshot
	fc.RetLsSnapshotOk = &snapshot.SnapshotListOK{Payload: []*models.Snapshot{retSnap}}
	fc.RetLsSnapshotErr = nil
	op.app.OCrud = fc
	snap, err := op.fetchLatestSnapshot(nil, "vsID")
	assert.Nil(err)
	assert.Equal(retSnap, snap)

	// empty list
	fc.RetLsSnapshotOk = &snapshot.SnapshotListOK{Payload: []*models.Snapshot{}}
	fc.RetLsSnapshotErr = nil
	op.app.OCrud = fc
	snap, err = op.fetchLatestSnapshot(nil, "vsID")
	assert.Nil(err)
	assert.Nil(snap)

	// error
	fc.RetLsSnapshotOk = nil
	fc.RetLsSnapshotErr = fmt.Errorf("snap list error")
	op.app.OCrud = fc
	snap, err = op.fetchLatestSnapshot(nil, "vsID")
	assert.NotNil(err)
	assert.Regexp("snap list error", err.Error())
	assert.Nil(snap)
}

func TestCustomizeOps(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	appS := &fa.AppServant{}
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log:       tl.Logger(),
			CSISocket: "/some/Socket",
		},
		AppServant: appS,
	}

	clusterRes := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "clusterID",
			},
		},
		ClusterCreateOnce: models.ClusterCreateOnce{
			CspDomainID: models.ObjIDMutable("CSP-DOMAIN-1"),
		},
		ClusterMutable: models.ClusterMutable{
			ClusterCreateMutable: models.ClusterCreateMutable{
				Name: "clusterName",
			},
		},
	}

	accountRes := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{
				ID: "accountID",
			},
		},
		AccountMutable: models.AccountMutable{
			Name: "System",
		},
	}

	domainRes := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID: "CSP-DOMAIN-1",
			},
		},
		CSPDomainMutable: models.CSPDomainMutable{
			Name: models.ObjName("DomainName"),
		},
	}

	op := mountOp{
		args: &csi.MountArgs{
			VolumeID:     "vol-1",
			Name:         "vsName",
			ServicePlan:  "servicePlanID",
			Account:      accountRes,
			PodName:      "PodName",
			PodNamespace: "PodNamespace",
			AVD:          &cluster.AccountVolumeData{},
		},
		cluster: clusterRes,
		domain:  domainRes,
		c:       &csiComp{},
	}
	op.c.Init(app)
	op.app = app

	retPod := &cluster.PodObj{
		Name:           "PodName",
		Namespace:      "PodNamespace",
		Labels:         map[string]string{},
		Annotations:    map[string]string{},
		ControllerName: "ControllerName",
		ControllerID:   "ControllerUID",
	}
	// ***************** pod fetch fails
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mCluster := mockcluster.NewMockClient(mockCtrl)
	op.c.app.ClusterClient = mCluster
	pfa := &cluster.PodFetchArgs{
		Name:      op.args.PodName,
		Namespace: op.args.PodNamespace,
	}
	mCluster.EXPECT().PodFetch(nil, pfa).Return(nil, fmt.Errorf("ignored error"))
	op.annotationMap = nil
	op.variableMap = nil
	op.buildVarMap(nil)
	assert.NotNil(op.variableMap)
	assert.NotNil(op.annotationMap)
	assert.Equal(string(accountRes.Meta.ID), op.variableMap[com.TemplateVarAccountID])
	assert.Equal(string(accountRes.Name), op.variableMap[com.TemplateVarAccountName])
	assert.Equal(string(clusterRes.Meta.ID), op.variableMap[com.TemplateVarClusterID])
	assert.Equal(string(clusterRes.Name), op.variableMap[com.TemplateVarClusterName])
	assert.Equal(string(domainRes.Meta.ID), op.variableMap[com.TemplateVarCspDomainID])
	assert.Equal(string(domainRes.Name), op.variableMap[com.TemplateVarCspDomainName])
	assert.Equal(string(op.args.VolumeID), op.variableMap[com.TemplateVarVolumeSeriesID])
	assert.Equal(string(op.args.Name), op.variableMap[com.TemplateVarVolumeSeriesName])

	// ***************** pod fetch succeeds, no labels or annotations

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mCluster = mockcluster.NewMockClient(mockCtrl)
	op.c.app.ClusterClient = mCluster
	pfa = &cluster.PodFetchArgs{
		Name:      op.args.PodName,
		Namespace: op.args.PodNamespace,
	}
	mCluster.EXPECT().PodFetch(nil, pfa).Return(retPod, nil)
	op.buildVarMap(nil)
	assert.NotNil(op.variableMap)
	assert.Equal(string(retPod.Name), op.variableMap[com.TemplateVarK8sPodName])
	assert.Equal(string(retPod.Namespace), op.variableMap[com.TemplateVarK8sPodNamespace])
	assert.Equal(string(retPod.ControllerName), op.variableMap[com.TemplateVarK8sPodControllerName])
	assert.Equal(string(retPod.ControllerID), op.variableMap[com.TemplateVarK8sPodControllerUID])

	// ***************** with labels, and annotations
	retPod.Labels = map[string]string{
		"label1": "label1value",
		"label2": "label2value",
	}
	retPod.Annotations = map[string]string{
		"annotation1": "annotation1value",
		"annotation2": "annotation2value",
	}

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mCluster = mockcluster.NewMockClient(mockCtrl)
	op.c.app.ClusterClient = mCluster
	pfa = &cluster.PodFetchArgs{
		Name:      op.args.PodName,
		Namespace: op.args.PodNamespace,
	}
	mCluster.EXPECT().PodFetch(nil, pfa).Return(retPod, nil)
	op.buildVarMap(nil)
	assert.NotNil(op.variableMap)
	assert.NotNil(op.annotationMap)
	assert.EqualValues(retPod.Annotations, op.annotationMap)
	for k := range retPod.Labels {
		_, ok := op.variableMap[fmt.Sprintf(com.TemplateVarK8sPodLabelsFmt, k)]
		assert.True(ok)
	}

	// ****************** fetch tags

	annotationFmts := []string{com.K8sPodAnnotationCGTagsFmt, com.K8sPodAnnotationVolTagsFmt, com.K8sPodAnnotationAGTagsFmt}
	// without AVD, and tags don't start with #1
	op.annotationMap = map[string]string{
		"nuvoloso.com/application-group-tag-2": "some-tag-that-will-be-ignored",
		"nuvoloso.com/consistency-group-tag-2": "some-tag-that-will-be-ignored",
		"nuvoloso.com/volume-tag-2":            "some-tag-that-will-be-ignored",
	}
	for _, fmt := range annotationFmts {
		tags := op.fetchTags([]string{}, fmt)
		assert.Empty(tags)
	}

	// without AVD, and tags correctly specified in annotations
	annotationMap := map[string][]string{
		"nuvoloso.com/application-group-name":        []string{"somevar", "somevar"},
		"nuvoloso.com/application-group-description": []string{"${account.id}", string(accountRes.Meta.ID)},
		"nuvoloso.com/application-group-tag-1":       []string{"${account.name}", string(accountRes.Name)},
		"nuvoloso.com/application-group-tag-2":       []string{"${cluster.id}", string(clusterRes.Meta.ID)},
		"nuvoloso.com/consistency-group-name":        []string{"${cluster.name}", string(clusterRes.Name)},
		"nuvoloso.com/consistency-group-description": []string{"${cspDomain.id}", string(domainRes.Meta.ID)},
		"nuvoloso.com/consistency-group-tag-1":       []string{"${cspDomain.name}", string(domainRes.Name)},
		"nuvoloso.com/consistency-group-tag-2":       []string{"${k8sPod.controller.uid}", string(retPod.ControllerID)},
		"nuvoloso.com/volume-tag-1":                  []string{"${k8sPod.controller.name}", string(retPod.ControllerName)},
		"nuvoloso.com/volume-tag-2":                  []string{"${k8sPod.labels['label1']}", string(retPod.Labels["label1"])},
		"nuvoloso.com/volume-tag-3":                  []string{"${k8sPod.namespace}", string(retPod.Namespace)},
		"nuvoloso.com/volume-tag-4":                  []string{"${k8sPod.name}", string(retPod.Name)},
		"nuvoloso.com/volume-tag-5":                  []string{"${volumeSeries.id}", string(op.args.VolumeID)},
		"nuvoloso.com/volume-tag-6":                  []string{"${volumeSeries.name}", string(op.args.Name)},
		"randomkey":                                  []string{"randomname", "randomname"},
	}
	for k, v := range annotationMap {
		op.annotationMap[k] = v[0]
		assert.Equal(v[1], op.substituteVariables(v[0])) // verify substitution is good.
	}

	for _, fmt := range annotationFmts {
		tags := op.fetchTags([]string{}, fmt)
		assert.NotEmpty(tags)
	}

	// with AVD tags specified
	tags := op.fetchTags([]string{"tag1, tag2"}, "ignoredAnnotationKey")
	assert.Equal([]string{"tag1, tag2"}, tags)

	// ****************** fetchOrCreateCG
	fc := &fake.Client{}
	fmo := &fakeMountOps{}
	op.ops = fmo
	// with AVD and create new CG
	op.args.AVD = &cluster.AccountVolumeData{
		ConsistencyGroupName:        "CGName-${account.name}",             // "CGName-System"
		ConsistencyGroupDescription: "CGDescription-${cspDomain.name}",    // "CGDescription-DomainName"
		ConsistencyGroupTags:        []string{"sometag", "${cluster.id}"}, //"clusterID"
	}
	expCG := &models.ConsistencyGroup{
		ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
			AccountID: models.ObjIDMutable(accountRes.Meta.ID),
		},
		ConsistencyGroupMutable: models.ConsistencyGroupMutable{
			Name:                models.ObjName("CGName-System"),
			Description:         models.ObjDescription("CGDescription-DomainName"),
			Tags:                models.ObjTags([]string{"sometag", "clusterID"}),
			ApplicationGroupIds: []models.ObjIDMutable{"agID"},
			SystemTags:          models.ObjTags([]string{com.SystemTagMustDeleteOnUndoCreate}),
		},
	}
	expAG := &models.ApplicationGroup{
		ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("agID"),
			},
		},
	}
	fc.RetCGListObj = &consistency_group.ConsistencyGroupListOK{}
	fc.RetCGListErr = nil
	fc.RetCGCreateObj = expCG
	fc.RetCGCreateErr = nil
	op.ag = expAG
	op.app = app
	op.app.OCrud = fc
	err := op.fetchOrCreateCG(nil)
	assert.Nil(err)
	assert.Equal(expCG, fc.InCGCreateObj)
	assert.Equal(expCG, op.cg)

	// with pod annotations set and AVD not set
	op.args.AVD = &cluster.AccountVolumeData{}
	op.annotationMap = map[string]string{}
	op.annotationMap[com.K8sPodAnnotationCGName] = "annotationCGName"
	op.annotationMap[com.K8sPodAnnotationCGDesc] = "annotationCGDesc"
	op.annotationMap[fmt.Sprintf(com.K8sPodAnnotationCGTagsFmt, 1)] = "tag1"
	expCG = &models.ConsistencyGroup{
		ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
			AccountID: models.ObjIDMutable(accountRes.Meta.ID),
		},
		ConsistencyGroupMutable: models.ConsistencyGroupMutable{
			Name:                models.ObjName("annotationCGName"), // ConsistencyGroupNameUnspecified
			Tags:                models.ObjTags([]string{"tag1"}),
			Description:         models.ObjDescription("annotationCGDesc"),
			ApplicationGroupIds: []models.ObjIDMutable{"agID"},
			SystemTags:          models.ObjTags([]string{com.SystemTagMustDeleteOnUndoCreate}),
		},
	}
	fc.RetCGListObj = &consistency_group.ConsistencyGroupListOK{}
	fc.RetCGListErr = nil
	fc.RetCGCreateObj = expCG
	fc.RetCGCreateErr = nil
	op.app.OCrud = fc
	err = op.fetchOrCreateCG(nil)
	assert.Nil(err)
	assert.Equal(expCG, fc.InCGCreateObj)
	assert.Equal(expCG, op.cg)

	// without AVD, without pod labels
	// this results in auto generated CG name, will not have description or cgTags
	op.annotationMap = map[string]string{}
	expCG = &models.ConsistencyGroup{
		ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
			AccountID: models.ObjIDMutable(accountRes.Meta.ID),
		},
		ConsistencyGroupMutable: models.ConsistencyGroupMutable{
			Name:                models.ObjName("clusterID-PodNamespace-PodName"), // ConsistencyGroupNameUnspecified
			Tags:                models.ObjTags([]string{}),
			ApplicationGroupIds: []models.ObjIDMutable{"agID"},
			SystemTags:          models.ObjTags([]string{com.SystemTagMustDeleteOnUndoCreate}),
		},
	}
	fc.RetCGListObj = &consistency_group.ConsistencyGroupListOK{}
	fc.RetCGListErr = nil
	fc.RetCGCreateObj = expCG
	fc.RetCGCreateErr = nil
	op.app.OCrud = fc
	err = op.fetchOrCreateCG(nil)
	assert.Nil(err)
	assert.Equal(expCG, fc.InCGCreateObj)
	assert.Equal(expCG, op.cg)

	// cg create fails
	fc.RetCGListObj = &consistency_group.ConsistencyGroupListOK{}
	fc.RetCGListErr = nil
	fc.RetCGCreateObj = nil
	fc.RetCGCreateErr = fmt.Errorf("cg create error")
	op.app.OCrud = fc
	err = op.fetchOrCreateCG(nil)
	assert.NotNil(err)
	assert.Regexp("cg create error", err.Error())

	// get AG fails
	fc.RetCGListObj = &consistency_group.ConsistencyGroupListOK{}
	fc.RetCGListErr = nil
	fmo.getAGErr = fmt.Errorf("get AG error")
	op.app.OCrud = fc
	err = op.fetchOrCreateCG(nil)
	assert.NotNil(err)
	assert.Regexp("get AG error", err.Error())

	// cg exists
	op.args.AVD.ConsistencyGroupName = "existingCG"
	expCG = &models.ConsistencyGroup{
		ConsistencyGroupMutable: models.ConsistencyGroupMutable{
			Name: models.ObjName("existingCG"),
		},
	}
	fc.RetCGListObj = &consistency_group.ConsistencyGroupListOK{
		Payload: []*models.ConsistencyGroup{
			expCG,
		},
	}
	fc.RetCGListErr = nil
	op.app.OCrud = fc
	err = op.fetchOrCreateCG(nil)
	assert.Nil(err)
	assert.Equal("existingCG", swag.StringValue(fc.InCGListParams.Name))
	assert.Equal(string(op.args.Account.Meta.ID), swag.StringValue(fc.InCGListParams.AccountID))
	assert.Equal(expCG, op.cg)

	// cglist error
	fc.RetCGListObj = nil
	fc.RetCGListErr = fmt.Errorf("cglist error")
	op.app.OCrud = fc
	err = op.fetchOrCreateCG(nil)
	assert.NotNil(err)
	assert.Regexp("cglist error", err.Error())

	// ****************** fetchOrCreateAG
	createAG := &models.ApplicationGroup{
		ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
			AccountID: models.ObjIDMutable(accountRes.Meta.ID),
		},
	}
	// using AVD
	op.args.AVD = &cluster.AccountVolumeData{
		ApplicationGroupName:        "AGName-${account.name}",             // "AGName-System"
		ApplicationGroupDescription: "AGDescription-${cspDomain.name}",    // "CGDescription-DomainName"
		ApplicationGroupTags:        []string{"sometag", "${cluster.id}"}, //"clusterID"
	}
	createAG.Name = models.ObjName("AGName-System")
	createAG.Description = models.ObjDescription("AGDescription-DomainName")
	createAG.Tags = models.ObjTags([]string{"sometag", "clusterID"})
	createAG.SystemTags = models.ObjTags([]string{com.SystemTagMustDeleteOnUndoCreate})
	fc.RetAGListObj = &application_group.ApplicationGroupListOK{}
	fc.RetAGListErr = nil
	fc.RetAGCreateObj = expAG
	fc.RetAGCreateErr = nil
	op.app = app
	op.app.OCrud = fc
	err = op.fetchOrCreateAG(nil, "cgName") // cgName not used
	assert.Nil(err)
	assert.Equal(createAG, fc.InAGCreateObj)
	assert.Equal(expAG, op.ag)

	// using pod annotations
	op.args.AVD = &cluster.AccountVolumeData{}
	op.annotationMap = map[string]string{}
	op.annotationMap[com.K8sPodAnnotationAGName] = "annotationAGName"
	op.annotationMap[com.K8sPodAnnotationAGDesc] = "annotationAGDesc"
	op.annotationMap[fmt.Sprintf(com.K8sPodAnnotationAGTagsFmt, 1)] = "tag1"
	createAG.Name = models.ObjName("annotationAGName")
	createAG.Description = models.ObjDescription("annotationAGDesc")
	createAG.Tags = models.ObjTags([]string{"tag1"})
	fc.RetAGListObj = &application_group.ApplicationGroupListOK{}
	fc.RetAGListErr = nil
	fc.RetAGCreateObj = expAG
	fc.RetAGCreateErr = nil
	op.app = app
	op.app.OCrud = fc
	err = op.fetchOrCreateAG(nil, "cgName") // cgName not used
	assert.Nil(err)
	assert.Equal(createAG, fc.InAGCreateObj)
	assert.Equal(expAG, op.ag)

	// using default agName {controller name}
	op.annotationMap = map[string]string{}
	createAG.Name = models.ObjName(retPod.ControllerName)
	createAG.Description = ""
	createAG.Tags = []string{}
	fc.RetAGListObj = &application_group.ApplicationGroupListOK{}
	fc.RetAGListErr = nil
	fc.RetAGCreateObj = expAG
	fc.RetAGCreateErr = nil
	op.app = app
	op.app.OCrud = fc
	err = op.fetchOrCreateAG(nil, "cgName")
	assert.Nil(err)
	assert.Equal(createAG, fc.InAGCreateObj)
	assert.Equal(expAG, op.ag)

	// using the cgName
	op.variableMap[com.TemplateVarK8sPodControllerName] = ""
	createAG.Name = "cgName"
	fc.RetAGListObj = &application_group.ApplicationGroupListOK{}
	fc.RetAGListErr = nil
	fc.RetAGCreateObj = expAG
	fc.RetAGCreateErr = nil
	op.app = app
	op.app.OCrud = fc
	err = op.fetchOrCreateAG(nil, "cgName")
	assert.Nil(err)
	assert.Equal(createAG, fc.InAGCreateObj)
	assert.Equal(expAG, op.ag)

	// app group create error
	createAG.Name = "cgName"
	fc.RetAGListObj = &application_group.ApplicationGroupListOK{}
	fc.RetAGListErr = nil
	fc.RetAGCreateObj = nil
	fc.RetAGCreateErr = fmt.Errorf("ag create error")
	op.app = app
	op.app.OCrud = fc
	err = op.fetchOrCreateAG(nil, "cgName")
	assert.NotNil(err)
	assert.Regexp("ag create error", err.Error())
	assert.Equal(createAG, fc.InAGCreateObj)
	assert.Nil(op.ag)

	// app group list return ag
	createAG.Name = "cgName"
	fc.RetAGListObj = &application_group.ApplicationGroupListOK{
		Payload: []*models.ApplicationGroup{
			expAG,
		},
	}
	fc.RetAGListErr = nil
	op.app = app
	op.app.OCrud = fc
	err = op.fetchOrCreateAG(nil, "cgName")
	assert.Nil(err)
	assert.Equal(expAG, op.ag)

	// app group list err
	createAG.Name = "cgName"
	inLParams := &application_group.ApplicationGroupListParams{
		Name:      swag.String("cgName"),
		AccountID: swag.String(string(accountRes.Meta.ID)),
	}
	fc.RetAGListObj = nil
	fc.RetAGListErr = fmt.Errorf("ag list error")
	op.app = app
	op.app.OCrud = fc
	err = op.fetchOrCreateAG(nil, "cgName")
	assert.NotNil(err)
	assert.Regexp("ag list error", err.Error())
	assert.Equal(expAG, op.ag)
	assert.Equal(inLParams, fc.InAGListParams)
}

type fakeMountOps struct {
	mountOp
	called            []string
	createMountVSRErr error
	waitForVSRErr     error
	checkForVSRret    bool
	checkForVSRErr    error
	volPublishedState vsMediaState
	volPublishedErr   error
	getCGErr          error
	getAGErr          error
	getAGcgNameIn     string
}

func (c *fakeMountOps) createMountVSR(ctx context.Context) error {
	c.called = append(c.called, "CMV")
	return c.createMountVSRErr
}

func (c *fakeMountOps) waitForVSR(ctx context.Context) error {
	c.called = append(c.called, "WFV")
	return c.waitForVSRErr
}
func (c *fakeMountOps) checkForVSR(ctx context.Context) (bool, error) {
	c.called = append(c.called, "CFV")
	return c.checkForVSRret, c.checkForVSRErr
}
func (c *fakeMountOps) getVolumeState(ctx context.Context, vsID string) (vsMediaState, error) {
	c.called = append(c.called, "GVS")
	return c.volPublishedState, c.volPublishedErr
}

func (c *fakeMountOps) fetchOrCreateCG(ctx context.Context) error {
	c.called = append(c.called, "GCG")
	return c.getCGErr
}

func (c *fakeMountOps) buildVarMap(ctx context.Context) {
	c.called = append(c.called, "BTM")
}

func (c *fakeMountOps) fetchOrCreateAG(ctx context.Context, cgName string) error {
	c.called = append(c.called, "GAG")
	c.getAGcgNameIn = cgName
	return c.getAGErr
}

type fakeAppObjects struct {
	retNode    *models.Node
	retCluster *models.Cluster
	retDomain  *models.CSPDomain
}

func (o *fakeAppObjects) GetNode() *models.Node {
	return o.retNode
}

func (o *fakeAppObjects) GetCluster() *models.Cluster {
	return o.retCluster
}

func (o *fakeAppObjects) GetCspDomain() *models.CSPDomain {
	return o.retDomain
}

func (o *fakeAppObjects) UpdateNodeLocalStorage(ctx context.Context, ephemeral map[string]models.NodeStorageDevice, cacheUnitSizeBytes int64) (*models.Node, error) {
	return nil, nil
}
