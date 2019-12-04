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
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockcluster "github.com/Nuvoloso/kontroller/pkg/cluster/mock"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	fa "github.com/Nuvoloso/kontroller/pkg/clusterd/fake"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	fVra "github.com/Nuvoloso/kontroller/pkg/vra/fake"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestDeleteVolume(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	mockCtrl := gomock.NewController(t)
	appS := &fa.AppServant{}
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log:            tl.Logger(),
			ServiceVersion: "0.6.2",
		},
		AppServant:    appS,
		ClusterClient: mockcluster.NewMockClient(mockCtrl),
	}
	c := &csiComp{}
	c.Init(app)

	retVSR := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{ID: "vsr-1"},
		},
	}

	// VSR lookup failed
	fmo := &fakeDeleteOps{}
	op := fmo
	op.c = c
	op.ops = op
	op.vsID = "vol-1"
	fmo.checkForVSRErr = fmt.Errorf("invalid error")
	fmo.checkForVSRRet = false
	expCalled := []string{"CFV"}
	err := op.run(nil)
	assert.NotNil(err)
	assert.Regexp("invalid error", err.Error())
	assert.Equal(expCalled, op.called)

	//  pending vsr, wait, error
	fmo = &fakeDeleteOps{}
	op = fmo
	op.c = c
	op.ops = op
	op.vsID = "vol-1"
	op.vsr = retVSR
	fmo.checkForVSRRet = true
	fmo.checkForVSRErr = nil
	fmo.waitForVSRErr = fmt.Errorf("vsr failed")
	expCalled = []string{"CFV", "WFV"}
	err = op.run(nil)
	assert.NotNil(err)
	assert.Regexp("vsr failed", err.Error())
	assert.Equal(expCalled, op.called)

	// pending vsr, wait, success
	fmo = &fakeDeleteOps{}
	op = fmo
	op.c = c
	op.ops = op
	op.vsID = "vol-1"
	op.vsr = retVSR
	fmo.checkForVSRRet = true
	fmo.checkForVSRErr = nil
	fmo.waitForVSRErr = nil
	expCalled = []string{"CFV", "WFV"}
	err = op.run(nil)
	assert.Nil(err)
	assert.Equal(expCalled, op.called)

	// no pending vsr, fetchDeletePolicy, error fetching policy
	fmo = &fakeDeleteOps{}
	op = fmo
	op.c = c
	op.ops = op
	op.vsID = "vol-1"
	fmo.checkForVSRRet = false
	fmo.checkForVSRErr = nil
	fmo.fetchDeletePolicyErr = fmt.Errorf("policy fetch failed")
	expCalled = []string{"CFV", "FDP"}
	err = op.run(nil)
	assert.NotNil(err)
	assert.Regexp("policy fetch failed", err.Error())
	assert.Equal(expCalled, op.called)

	// no pending vsr, fetchDeletePolicy, createVSR, error creating vsr
	fmo = &fakeDeleteOps{}
	op = fmo
	op.c = c
	op.ops = op
	op.vsID = "vol-1"
	op.vsr = retVSR
	fmo.checkForVSRRet = false
	fmo.checkForVSRErr = nil
	fmo.fetchDeletePolicyErr = nil
	fmo.createDeleteVSRErr = fmt.Errorf("vsr create failed")
	expCalled = []string{"CFV", "FDP", "CDV"}
	err = op.run(nil)
	assert.NotNil(err)
	assert.Regexp("vsr create failed", err.Error())
	assert.Equal(expCalled, op.called)

	// no pending vsr, fetchDeletePolicy, createVSR, wait for vsr, failure
	fmo = &fakeDeleteOps{}
	op = fmo
	op.c = c
	op.ops = op
	op.vsID = "vol-1"
	op.vsr = retVSR
	fmo.checkForVSRRet = false
	fmo.checkForVSRErr = nil
	fmo.createDeleteVSRErr = nil
	fmo.fetchDeletePolicyErr = nil
	fmo.waitForVSRErr = fmt.Errorf("vsr wait failed")
	expCalled = []string{"CFV", "FDP", "CDV", "WFV"}
	err = op.run(nil)
	assert.NotNil(err)
	assert.Regexp("vsr wait failed", err.Error())
	assert.Equal(expCalled, op.called)

	// no pending vsr, fetchDeletePolicy, createVSR, wait for vsr, success
	fmo = &fakeDeleteOps{}
	op = fmo
	op.c = c
	op.ops = op
	op.vsID = "vol-1"
	op.vsr = retVSR
	fmo.checkForVSRRet = false
	fmo.checkForVSRErr = nil
	fmo.createDeleteVSRErr = nil
	fmo.waitForVSRErr = nil
	fmo.fetchDeletePolicyErr = nil
	expCalled = []string{"CFV", "FDP", "CDV", "WFV"}
	err = op.run(nil)
	assert.Nil(err)
	assert.Equal(expCalled, op.called)

	// call the real handler with error
	fc := &fake.Client{}
	fc.RetLsVRErr = fmt.Errorf("VSR list error")
	c.app.OCrud = fc
	err = c.DeleteVolume(nil, "volumeID")
	assert.Error(err)
	assert.Regexp("VSR list error", err.Error())

	// id validate fails
	err = c.DeleteVolume(nil, "")
	assert.Error(err)
	assert.Regexp("needs valid volumeID", err.Error())
}

func TestMountSteps(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	mockCtrl := gomock.NewController(t)
	appS := &fa.AppServant{}
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log:            tl.Logger(),
			ServiceVersion: "0.6.2",
		},
		AppServant:    appS,
		ClusterClient: mockcluster.NewMockClient(mockCtrl),
	}

	op := deleteOp{
		vsID: "volumeID",
		c:    &csiComp{},
	}
	op.c.Init(app)

	retVSR := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{ID: "vsr-1"},
		},
	}

	retVS := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{ID: "vs-1"},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				ConsistencyGroupID: models.ObjIDMutable("cg-1"),
			},
		},
	}

	retCG := &models.ConsistencyGroup{
		ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
			Meta: &models.ObjMeta{ID: "cg-1"},
		},
		ConsistencyGroupMutable: models.ConsistencyGroupMutable{
			SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
				VolumeDataRetentionOnDelete: com.VolumeDataRetentionOnDeleteDelete,
			},
		},
	}

	fc := &fake.Client{}

	// ********************** checkForVSR

	// list empty
	op.vsr = nil
	fc.RetLsVRErr = nil
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{Payload: []*models.VolumeSeriesRequest{}}
	op.c.app = app
	op.c.app.OCrud = fc
	conflictVSRs, err := op.checkForVSR(nil)
	assert.False(conflictVSRs)
	assert.Nil(err)
	assert.Nil(op.vsr)
	assert.Equal([]string{fmt.Sprintf("%s:%s", com.SystemTagVsrK8sVolDelete, op.vsID)}, fc.InLsVRObj.SystemTags)

	// pending vsrs
	op.vsr = nil
	fc.RetLsVRErr = nil
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{Payload: []*models.VolumeSeriesRequest{retVSR}}
	op.c.app = app
	op.c.app.OCrud = fc
	conflictVSRs, err = op.checkForVSR(nil)
	assert.True(conflictVSRs)
	assert.Nil(err)
	assert.Equal(retVSR, op.vsr)

	// vsr list failed
	fc.RetLsVRErr = fmt.Errorf("vsr list failed")
	op.c.app = app
	op.c.app.OCrud = fc
	conflictVSRs, err = op.checkForVSR(nil)
	assert.False(conflictVSRs)
	assert.NotNil(err)
	assert.Regexp("vsr list failed", err.Error())
	assert.Equal(1, tl.CountPattern("vsr list failed"))
	tl.Flush()

	// ********************** fetchDeletePolicy

	// success
	fc.RetVObj = retVS
	fc.RetVErr = nil
	retCG.SnapshotManagementPolicy.VolumeDataRetentionOnDelete = "RETPOLICY"
	fc.RetCGFetchObj = retCG
	fc.RetCGFetchErr = nil
	op.vsID = "vs-1"
	err = op.fetchDeletePolicy(nil)
	assert.Nil(err)
	assert.Equal("RETPOLICY", op.dataRetentionOnDelete)
	assert.Equal(op.vsID, fc.InVFetchID)
	assert.Equal(string(retVS.ConsistencyGroupID), fc.InCGFetchID)

	// consistency group fetch failure
	op.dataRetentionOnDelete = ""
	fc.RetVObj = retVS
	fc.RetVErr = nil
	fc.RetCGFetchObj = nil
	fc.RetCGFetchErr = fmt.Errorf("some error")
	op.vsID = "vs-1"
	err = op.fetchDeletePolicy(nil)
	assert.NotNil(err)
	assert.Empty(op.dataRetentionOnDelete)

	// volume series fetch failure
	op.dataRetentionOnDelete = ""
	fc.RetVObj = nil
	fc.RetVErr = fmt.Errorf("some error")
	op.vsID = "vs-1"
	err = op.fetchDeletePolicy(nil)
	assert.NotNil(err)
	assert.Empty(op.dataRetentionOnDelete)

	// ********************** createDeleteVSR

	// success
	fc.RetVRCErr = nil
	fc.RetVRCObj = retVSR
	op.vsr = nil
	op.dataRetentionOnDelete = "RETAIN"
	err = op.createDeleteVSR(nil)
	assert.Nil(err)
	assert.Equal(retVSR, op.vsr)
	assert.Equal(models.ObjIDMutable(op.vsID), fc.InVRCArgs.VolumeSeriesID)
	assert.Equal(models.ObjTags([]string{fmt.Sprintf("%s:%s", com.SystemTagVsrK8sVolDelete, op.vsID)}), fc.InVRCArgs.SystemTags)
	assert.NotEmpty(fc.InVRCArgs.CompleteByTime)
	assert.Equal([]string{com.VolReqOpUnbind}, fc.InVRCArgs.RequestedOperations)

	// VSR create failed
	fc.RetVRCErr = fmt.Errorf("VSR create failed")
	fc.RetVRCObj = nil
	op.vsr = nil
	op.dataRetentionOnDelete = com.VolumeDataRetentionOnDeleteDelete
	err = op.createDeleteVSR(nil)
	assert.NotNil(err)
	assert.Regexp("VSR create failed", err.Error())
	assert.NotNil(op.vsr)
	assert.Equal(models.ObjIDMutable(op.vsID), op.vsr.VolumeSeriesID)
	assert.Equal([]string{com.VolReqOpDelete}, fc.InVRCArgs.RequestedOperations)

	// VSR create failed with missing precondition
	fc.RetVRCErr = fmt.Errorf(com.ErrorFinalSnapNeeded)
	fc.RetVRCObj = nil
	op.vsr = nil
	err = op.createDeleteVSR(nil)
	assert.NotNil(err)
	assert.Regexp("operation delayed until final snapshot completed", err.Error())
	assert.NotNil(op.vsr)
	assert.Equal(models.ObjIDMutable(op.vsID), op.vsr.VolumeSeriesID)
	assert.Equal([]string{com.VolReqOpDelete}, fc.InVRCArgs.RequestedOperations)
	st := status.Convert(err)
	assert.Equal(codes.FailedPrecondition, st.Code())

	// *********************** waitForVSR

	// success
	op.ops = &fakeDeleteOps{}
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
		assert.Regexp("waitForVSR.*wait failure", err.Error())
	}

	// call real waiter with bad args to force panic
	op.vW = nil
	op.c.app.OCrud = nil
	assert.Panics(func() { op.waitForVSR(nil) })
}

type fakeDeleteOps struct {
	deleteOp
	called               []string
	checkForVSRRet       bool
	checkForVSRErr       error
	createDeleteVSRErr   error
	waitForVSRErr        error
	fetchDeletePolicyErr error
}

func (c *fakeDeleteOps) checkForVSR(ctx context.Context) (bool, error) {
	c.called = append(c.called, "CFV")
	return c.checkForVSRRet, c.checkForVSRErr
}

func (c *fakeDeleteOps) createDeleteVSR(ctx context.Context) error {
	c.called = append(c.called, "CDV")
	return c.createDeleteVSRErr
}

func (c *fakeDeleteOps) waitForVSR(ctx context.Context) error {
	c.called = append(c.called, "WFV")
	return c.waitForVSRErr
}

func (c *fakeDeleteOps) fetchDeletePolicy(ctx context.Context) error {
	c.called = append(c.called, "FDP")
	return c.fetchDeletePolicyErr
}
