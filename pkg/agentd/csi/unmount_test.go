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
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/csi"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	fVra "github.com/Nuvoloso/kontroller/pkg/vra/fake"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestUnmountVolume(t *testing.T) {
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

	// invalid state
	fuo := &fakeUnmountOps{}
	op := fuo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.UnmountArgs{
		VolumeID: "vol-1",
	}
	fuo.volUnpublishedState = InvalidState
	fuo.volUnpublishedErr = fmt.Errorf("invalid error")
	expCalled := []string{"VU"}
	err := op.run(nil)
	assert.NotNil(err)
	assert.Regexp("invalid error", err.Error())
	assert.Equal(expCalled, op.called)

	// already unmounted
	fuo = &fakeUnmountOps{}
	op = fuo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.UnmountArgs{
		VolumeID: "vol-1",
	}
	fuo.volUnpublishedState = NotMountedState
	expCalled = []string{"VU"}
	err = op.run(nil)
	assert.Nil(err)
	assert.Equal(expCalled, op.called)

	// mounted, create, wait, success
	fuo = &fakeUnmountOps{}
	op = fuo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.UnmountArgs{
		VolumeID: "vol-1",
	}
	fuo.volUnpublishedState = MountedState
	fuo.createUnmountVSRErr = nil
	fuo.waitForVSRErr = nil
	expCalled = []string{"VU", "CUV", "WFV"}
	err = op.run(nil)
	assert.Nil(err)
	assert.Equal(expCalled, op.called)
	assert.Equal([]string{com.VolReqOpUnmount}, fuo.createUnmountVSRopsIn)

	// mounted and FS attached, create, wait, success
	fuo = &fakeUnmountOps{}
	op = fuo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.UnmountArgs{
		VolumeID: "vol-1",
	}
	fuo.volUnpublishedState = FsAttachedState
	fuo.createUnmountVSRErr = nil
	fuo.waitForVSRErr = nil
	expCalled = []string{"VU", "CUV", "WFV"}
	err = op.run(nil)
	assert.Nil(err)
	assert.Equal(expCalled, op.called)
	assert.Equal([]string{com.VolReqOpDetachFs, com.VolReqOpUnmount}, fuo.createUnmountVSRopsIn)

	// mounted and FS attached, create, wait, error
	fuo = &fakeUnmountOps{}
	op = fuo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.UnmountArgs{
		VolumeID: "vol-1",
	}
	fuo.volUnpublishedState = FsAttachedState
	fuo.createUnmountVSRErr = nil
	fuo.waitForVSRErr = fmt.Errorf("unmount err")
	expCalled = []string{"VU", "CUV", "WFV"}
	err = op.run(nil)
	assert.NotNil(err)
	assert.Regexp("unmount err", err.Error())
	assert.Equal(expCalled, op.called)
	assert.Equal([]string{com.VolReqOpDetachFs, com.VolReqOpUnmount}, fuo.createUnmountVSRopsIn)

	// mounted, create, wait, success
	fuo = &fakeUnmountOps{}
	op = fuo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.UnmountArgs{
		VolumeID: "vol-1",
	}
	fuo.volUnpublishedState = MountedState
	fuo.createUnmountVSRErr = nil
	fuo.waitForVSRErr = fmt.Errorf("unmount err")
	expCalled = []string{"VU", "CUV", "WFV"}
	err = op.run(nil)
	assert.NotNil(err)
	assert.Regexp("unmount err", err.Error())
	assert.Equal(expCalled, op.called)
	assert.Equal([]string{com.VolReqOpUnmount}, fuo.createUnmountVSRopsIn)

	// mounted and FS attached, create, pending vsr, success
	fuo = &fakeUnmountOps{}
	op = fuo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.UnmountArgs{
		VolumeID: "vol-1",
	}
	op.vsr = retVSR
	fuo.volUnpublishedState = FsAttachedState
	fuo.createUnmountVSRErr = &crud.Error{Payload: models.Error{Code: http.StatusConflict, Message: swag.String(com.ErrorRequestInConflict)}}
	fuo.checkForVSRret = true
	fuo.waitForVSRErr = nil
	expCalled = []string{"VU", "CUV", "CFV", "WFV"}
	err = op.run(nil)
	assert.Nil(err)
	assert.Equal(expCalled, op.called)

	// mounted and FS attached, create, pending vsr, error
	fuo = &fakeUnmountOps{}
	op = fuo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.UnmountArgs{
		VolumeID: "vol-1",
	}
	op.vsr = retVSR
	fuo.volUnpublishedState = FsAttachedState
	fuo.createUnmountVSRErr = &crud.Error{Payload: models.Error{Code: http.StatusConflict, Message: swag.String(com.ErrorRequestInConflict)}}
	fuo.checkForVSRret = true
	fuo.waitForVSRErr = fmt.Errorf("VSR error")
	expCalled = []string{"VU", "CUV", "CFV", "WFV"}
	err = op.run(nil)
	assert.NotNil(err)
	assert.Regexp("VSR error", err.Error())
	assert.Equal(expCalled, op.called)

	// mounted and FS attached, create, pending vsr, no pending vsrs, un mounted
	fuo = &fakeUnmountOps{}
	op = fuo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.UnmountArgs{
		VolumeID: "vol-1",
	}
	fuo.volUnpublishedState = FsAttachedState
	fuo.volUnpublishedErr = nil
	fuo.createUnmountVSRErr = &crud.Error{Payload: models.Error{Code: http.StatusConflict, Message: swag.String(com.ErrorRequestInConflict)}}
	fuo.checkForVSRret = false
	expCalled = []string{"VU", "CUV", "CFV", "VU"}
	err = op.run(nil)
	assert.Nil(err)
	assert.Equal(expCalled, op.called)

	// mounted and FS attached, create, pending vsr, no pending vsrs, error
	fuo = &fakeUnmountOps{}
	op = fuo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.UnmountArgs{
		VolumeID: "vol-1",
	}
	fuo.volUnpublishedState = FsAttachedState
	fuo.volUnpublishedErr = fmt.Errorf("unmount err")
	fuo.createUnmountVSRErr = &crud.Error{Payload: models.Error{Code: http.StatusConflict, Message: swag.String(com.ErrorRequestInConflict)}}
	fuo.checkForVSRret = false
	expCalled = []string{"VU", "CUV", "CFV", "VU"}
	err = op.run(nil)
	assert.NotNil(err)
	assert.Regexp("unmount err", err.Error())
	assert.Equal(expCalled, op.called)

	// mounted and FS attached, create, create error, error
	fuo = &fakeUnmountOps{}
	op = fuo
	op.c = c
	op.ops = op
	op.app = app
	op.args = &csi.UnmountArgs{
		VolumeID: "vol-1",
	}
	fuo.volUnpublishedState = FsAttachedState
	fuo.volUnpublishedErr = fmt.Errorf("unmount err")
	fuo.createUnmountVSRErr = fmt.Errorf("other error") // gets eaten up
	expCalled = []string{"VU", "CUV", "VU"}
	err = op.run(nil)
	assert.NotNil(err)
	assert.Regexp("unmount err", err.Error())
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
	fao := &fakeAppObjects{}
	fao.retCluster = cluster
	fao.retNode = node
	op.app.AppObjects = fao
	retV := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{ID: "vol-1"},
		},
	}
	retV.BoundClusterID = "bad clusterID"
	fc := &fake.Client{}
	fc.RetVObj = retV
	fc.RetVErr = nil
	c.app.OCrud = fc
	err = c.UnmountVolume(nil, &csi.UnmountArgs{VolumeID: "vol-1", TargetPath: "sometarget"})
	assert.Error(err)
	assert.Regexp("not bound to cluster", err.Error())

	// args validate fails
	err = c.UnmountVolume(nil, &csi.UnmountArgs{VolumeID: ""})
	assert.Error(err)
	assert.Regexp("args invalid or missing", err.Error())
}

func TestUnmountSteps(t *testing.T) {
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
	clusterID := "cluster-id"

	op := unmountOp{
		args: &csi.UnmountArgs{
			VolumeID: "vol-1",
		},
		nodeIdentifier:    nodeID,
		clusterIdentifier: clusterID,
		vs:                retVS,
		c:                 &csiComp{},
	}
	op.c.Init(app)

	retVSR := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{ID: "vsr-1"},
		},
	}
	fc := &fake.Client{}

	// *********************** checkForVSRs
	// list empty
	op.vsr = nil
	fc.RetLsVRErr = nil
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{Payload: []*models.VolumeSeriesRequest{}}
	op.app = app
	op.app.OCrud = fc
	conflictVSRs := op.checkForVSRs(nil)
	assert.False(conflictVSRs)
	assert.Nil(op.vsr)

	// pending unmount vsrs
	op.vsr = nil
	fc.RetLsVRErr = nil
	retVSR.RequestedOperations = []string{"UNMOUNT"}
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{Payload: []*models.VolumeSeriesRequest{retVSR}}
	op.app = app
	op.app.OCrud = fc
	conflictVSRs = op.checkForVSRs(nil)
	assert.True(conflictVSRs)
	assert.Equal(retVSR, op.vsr)

	// pending detach fs vsrs
	op.vsr = nil
	fc.RetLsVRErr = nil
	retVSR.RequestedOperations = []string{"DETACH_FS"}
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{Payload: []*models.VolumeSeriesRequest{retVSR}}
	op.app = app
	op.app.OCrud = fc
	conflictVSRs = op.checkForVSRs(nil)
	assert.True(conflictVSRs)
	assert.Equal(retVSR, op.vsr)

	// no pending unmount vsrs
	op.vsr = nil
	fc.RetLsVRErr = nil
	retVSR.RequestedOperations = []string{"SNAPSHOT"}
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{Payload: []*models.VolumeSeriesRequest{retVSR}}
	op.app = app
	op.app.OCrud = fc
	conflictVSRs = op.checkForVSRs(nil)
	assert.False(conflictVSRs)
	assert.Nil(op.vsr)

	// vsr list failed
	fc.RetLsVRErr = fmt.Errorf("vsr list failed")
	op.app = app
	op.app.OCrud = fc
	conflictVSRs = op.checkForVSRs(nil)
	assert.False(conflictVSRs)
	assert.Equal(1, tl.CountPattern("vsr list failed"))
	tl.Flush()

	// ***********************createUnmountVSR

	// success
	fc.RetVRCErr = nil
	fc.RetVRCObj = retVSR
	op.vsr = nil
	err := op.createUnmountVSR(nil, []string{"UNMOUNT"})
	assert.Nil(err)
	assert.Equal(retVSR, op.vsr)

	// VSR create failed
	tp := "/some/target/path"
	fc.RetVRCErr = fmt.Errorf("VSR create failed")
	fc.RetVRCObj = nil
	op.vsr = nil
	op.args.TargetPath = tp
	err = op.createUnmountVSR(nil, []string{"UNMOUNT", "DETACH_FS"})
	assert.NotNil(err)
	assert.Regexp("VSR create failed", err.Error())
	assert.NotNil(op.vsr)
	assert.Equal(models.ObjIDMutable(retVS.Meta.ID), op.vsr.VolumeSeriesID)
	assert.Equal(models.ObjIDMutable(nodeID), op.vsr.NodeID)
	assert.Equal(2, len(op.vsr.RequestedOperations))
	assert.Equal(tp, op.vsr.TargetPath)

	// rei error
	op.c.rei.SetProperty("csi-unmount-vsr", &rei.Property{BoolValue: true})
	err = op.createUnmountVSR(nil, []string{"UNMOUNT", "DETACH_FS"})
	assert.NotNil(err)
	assert.Regexp("csi-unmount-vsr", err.Error())

	// *********************** waitForVSR

	// success
	op.ops = &fakeUnmountOps{}
	fw := fVra.NewFakeRequestWaiter()
	retVSR.VolumeSeriesRequestState = com.VolReqStateSucceeded
	fw.Vsr = retVSR
	fw.Error = nil
	op.vW = fw
	op.vsr = retVSR
	err = op.waitForVSR(nil)
	assert.Nil(err)

	// vsr failed
	op.vsr.VolumeSeriesRequestState = "FAILED"
	err = op.waitForVSR(nil)
	if assert.Error(err) {
		assert.Regexp("volume-series-request.*FAILED", err.Error())
	}

	// vsr ongoing, fetch returns error
	fw.Error = errors.New("wait failure")
	err = op.waitForVSR(nil)
	if assert.Error(err) {
		assert.Regexp("wait failed.*wait failure", err.Error())
	}

	// vsr ongoing, context expired
	fw.Error = vra.ErrRequestWaiterCtxExpired
	err = op.waitForVSR(nil)
	if assert.Error(err) {
		assert.Regexp("request.*in progress", err.Error())
	}

	// call real waiter with bad args to force panic
	op.vW = nil
	op.app.OCrud = nil
	assert.Panics(func() { op.waitForVSR(nil) })

	// *********************** volUnpublished

	// Mounted and attached fs
	retVS.Mounts = []*models.Mount{
		&models.Mount{
			SnapIdentifier: "HEAD",
			MountedNodeID:  "nodeH",
			MountState:     "MOUNTED",
		},
	}
	retVS.BoundClusterID = models.ObjIDMutable(clusterID)
	retVS.VolumeSeriesState = com.VolStateInUse
	retVS.SystemTags = []string{com.SystemTagVolumeFsAttached}
	fc.RetVObj = retVS
	op.app.OCrud = fc
	state, err := op.volUnpublished(nil, "vsID")
	assert.NotNil(err)
	st := status.Convert(err)
	assert.Equal(codes.Internal, st.Code())
	assert.Regexp("fs is still attached", err.Error())
	assert.Equal(FsAttachedState, state)

	// Mounted but not attached
	retVS.BoundClusterID = models.ObjIDMutable(clusterID)
	retVS.VolumeSeriesState = com.VolStateInUse
	retVS.SystemTags = []string{}
	fc.RetVObj = retVS
	op.app.OCrud = fc
	state, err = op.volUnpublished(nil, "vsID")
	assert.NotNil(err)
	st = status.Convert(err)
	assert.Equal(codes.Internal, st.Code())
	assert.Regexp("is still mounted", err.Error())
	assert.Equal(MountedState, state)

	// NotMounted
	retVS.VolumeSeriesState = com.VolStateBound
	fc.RetVObj = retVS
	op.app.OCrud = fc
	state, err = op.volUnpublished(nil, "vsID")
	assert.Nil(err)
	assert.Equal(NotMountedState, state)

	// clusterID mismatch
	retVS.BoundClusterID = "wrongID"
	fc.RetVObj = retVS
	op.app.OCrud = fc
	state, err = op.volUnpublished(nil, "vsID")
	assert.NotNil(err)
	assert.Regexp("not bound to cluster", err.Error())
	st = status.Convert(err)
	assert.Equal(codes.Internal, st.Code())
	assert.Equal(InvalidState, state)

	// fetch error
	fc.RetVErr = fmt.Errorf("fetch error")
	op.app.OCrud = fc
	state, err = op.volUnpublished(nil, "vsID")
	assert.NotNil(err)
	assert.Regexp("fetch failed", err.Error())
	st = status.Convert(err)
	assert.Equal(codes.Internal, st.Code())
	assert.Equal(InvalidState, state)

	// vs not found
	fc.RetVErr = &crud.Error{Payload: models.Error{Code: http.StatusNotFound, Message: swag.String(com.ErrorNotFound)}}
	op.app.OCrud = fc
	state, err = op.volUnpublished(nil, "vsID")
	assert.NotNil(err)
	assert.Regexp("not found", err.Error())
	st = status.Convert(err)
	assert.Equal(codes.NotFound, st.Code())
	assert.Equal(InvalidState, state)
}

type fakeUnmountOps struct {
	unmountOp
	called                []string
	createUnmountVSRopsIn []string
	createUnmountVSRErr   error
	checkForVSRret        bool
	waitForVSRErr         error
	volUnpublishedState   vsMediaState
	volUnpublishedErr     error
}

func (c *fakeUnmountOps) checkForVSRs(ctx context.Context) bool {
	c.called = append(c.called, "CFV")
	return c.checkForVSRret
}
func (c *fakeUnmountOps) createUnmountVSR(ctx context.Context, ops []string) error {
	c.called = append(c.called, "CUV")
	c.createUnmountVSRopsIn = ops
	return c.createUnmountVSRErr
}
func (c *fakeUnmountOps) waitForVSR(ctx context.Context) error {
	c.called = append(c.called, "WFV")
	return c.waitForVSRErr
}
func (c *fakeUnmountOps) volUnpublished(ctx context.Context, vsID string) (vsMediaState, error) {
	c.called = append(c.called, "VU")
	return c.volUnpublishedState, c.volUnpublishedErr
}
