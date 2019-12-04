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


package handlers

import (
	"reflect"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestVolumeSeriesRequestCreate(t *testing.T) {
	assert := assert.New(t)

	hc := newHandlerComp()
	fc := &fake.Client{}
	hc.oCrud = fc
	args := &models.VolumeSeriesRequestCreateArgs{
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			ClusterID:           "cID",
			RequestedOperations: []string{"CREATE", "BIND", "MOUNT"},
		},
		VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
			NodeID:     "nID",
			SystemTags: []string{"myVR"},
		},
	}
	params := ops.VolumeSeriesRequestCreateParams{Payload: args}

	fc.RetVRCObj = &models.VolumeSeriesRequest{}
	ret := hc.volumeSeriesRequestCreate(params)
	if assert.NotNil(fc.InVRCArgs) {
		assert.Equal(args.VolumeSeriesRequestCreateOnce, fc.InVRCArgs.VolumeSeriesRequestCreateOnce)
		assert.Equal(args.VolumeSeriesRequestCreateMutable, fc.InVRCArgs.VolumeSeriesRequestCreateMutable)
	}
	obj, ok := ret.(*ops.VolumeSeriesRequestCreateCreated)
	if assert.True(ok, "VolumeSeriesRequestCreateCreated") {
		assert.Equal(fc.RetVRCObj, obj.Payload)
	}

	fc.RetVRCObj = nil
	expErr := &crud.Error{Payload: models.Error{Code: 402, Message: swag.String("Nebraska")}}
	fc.RetVRCErr = expErr
	ret = hc.volumeSeriesRequestCreate(params)
	e, ok := ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok, "VolumeSeriesRequestCreateDefault") {
		assert.Equal(&expErr.Payload, e.Payload)
	}
}

func TestVolumeSeriesRequestFetch(t *testing.T) {
	assert := assert.New(t)

	hc := newHandlerComp()
	fc := &fake.Client{}
	hc.oCrud = fc
	params := ops.VolumeSeriesRequestFetchParams{ID: "vrID"}

	fc.RetVRObj = &models.VolumeSeriesRequest{}
	ret := hc.volumeSeriesRequestFetch(params)
	obj, ok := ret.(*ops.VolumeSeriesRequestFetchOK)
	if assert.True(ok, "VolumeSeriesRequestFetchOK") {
		assert.Equal(fc.RetVRObj, obj.Payload)
	}

	fc.RetVRObj = nil
	expErr := &crud.Error{Payload: models.Error{Code: 402, Message: swag.String("Nebraska")}}
	fc.RetVRErr = expErr
	ret = hc.volumeSeriesRequestFetch(params)
	e, ok := ret.(*ops.VolumeSeriesRequestFetchDefault)
	if assert.True(ok, "VolumeSeriesRequestFetchDefault") {
		assert.Equal(&expErr.Payload, e.Payload)
	}
}

func TestVolumeSeriesRequestList(t *testing.T) {
	assert := assert.New(t)

	hc := newHandlerComp()
	fc := &fake.Client{}
	hc.oCrud = fc
	now := time.Now()
	dt := strfmt.DateTime(now.Add(-1 * time.Hour))
	params := ops.VolumeSeriesRequestListParams{
		ActiveOrTimeModifiedGE: &dt, // no swag converter for DateTime
		IsTerminated:           swag.Bool(false),
		NodeID:                 swag.String("node-1"),
		SyncCoordinatorID:      swag.String("sync-1"),
		SystemTags:             []string{"fv-identifier"},
	}
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{Payload: []*models.VolumeSeriesRequest{}}
	ret := hc.volumeSeriesRequestList(params)
	obj, ok := ret.(*ops.VolumeSeriesRequestListOK)
	if assert.True(ok, "VolumeSeriesRequestListOK") {
		assert.Equal(fc.RetLsVRObj.Payload, obj.Payload)
	}
	assert.Equal(20, reflect.TypeOf(params).NumField())        // ensure this test is updated if new fields are added
	assert.Equal(22, reflect.TypeOf(*fc.InLsVRObj).NumField()) // ditto
	assert.Equal(params.ActiveOrTimeModifiedGE, fc.InLsVRObj.ActiveOrTimeModifiedGE)
	assert.Equal(params.IsTerminated, fc.InLsVRObj.IsTerminated)
	assert.Equal(params.NodeID, fc.InLsVRObj.NodeID)
	assert.Equal(params.SyncCoordinatorID, fc.InLsVRObj.SyncCoordinatorID)
	assert.Equal(params.SystemTags, fc.InLsVRObj.SystemTags)
	assert.Nil(fc.InLsVRObj.AccountID)
	assert.Nil(fc.InLsVRObj.ClusterID)
	assert.Nil(fc.InLsVRObj.ConsistencyGroupID)
	assert.Nil(fc.InLsVRObj.PoolID)
	assert.Nil(fc.InLsVRObj.ServicePlanAllocationID)
	assert.Nil(fc.InLsVRObj.ServicePlanID)
	assert.Nil(fc.InLsVRObj.SnapshotID)
	assert.Nil(fc.InLsVRObj.StorageID)
	assert.Nil(fc.InLsVRObj.VolumeSeriesID)
	assert.Nil(fc.InLsVRObj.VolumeSeriesRequestState)

	params = ops.VolumeSeriesRequestListParams{
		AccountID:                swag.String("a-1"),
		ClusterID:                swag.String("cluster-1"),
		ConsistencyGroupID:       swag.String("cg-1"),
		PoolID:                   swag.String("pool-1"),
		ServicePlanAllocationID:  swag.String("spa-1"),
		ServicePlanID:            swag.String("plan-1"),
		SnapshotID:               swag.String("snap-1"),
		StorageID:                swag.String("s-1"),
		VolumeSeriesID:           swag.String("vs-1"),
		VolumeSeriesRequestState: []string{"SIZING", "PLACEMENT"},
	}
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{Payload: []*models.VolumeSeriesRequest{}}
	ret = hc.volumeSeriesRequestList(params)
	obj, ok = ret.(*ops.VolumeSeriesRequestListOK)
	if assert.True(ok, "VolumeSeriesRequestListOK") {
		assert.Equal(fc.RetLsVRObj.Payload, obj.Payload)
	}
	assert.Nil(params.ActiveOrTimeModifiedGE)
	assert.Nil(params.IsTerminated)
	assert.Nil(params.NodeID)
	assert.Nil(params.SyncCoordinatorID)
	assert.Nil(params.SystemTags)
	assert.Equal(params.AccountID, fc.InLsVRObj.AccountID)
	assert.Equal(params.ClusterID, fc.InLsVRObj.ClusterID)
	assert.Equal(params.ConsistencyGroupID, fc.InLsVRObj.ConsistencyGroupID)
	assert.Equal(params.PoolID, fc.InLsVRObj.PoolID)
	assert.Equal(params.ServicePlanAllocationID, fc.InLsVRObj.ServicePlanAllocationID)
	assert.Equal(params.ServicePlanID, fc.InLsVRObj.ServicePlanID)
	assert.Equal(params.SnapshotID, fc.InLsVRObj.SnapshotID)
	assert.Equal(params.StorageID, fc.InLsVRObj.StorageID)
	assert.Equal(params.VolumeSeriesID, fc.InLsVRObj.VolumeSeriesID)
	assert.Equal(params.VolumeSeriesRequestState, fc.InLsVRObj.VolumeSeriesRequestState)

	fc.RetVRObj = nil
	expErr := &crud.Error{Payload: models.Error{Code: 402, Message: swag.String("Nebraska")}}
	fc.RetLsVRErr = expErr
	ret = hc.volumeSeriesRequestList(params)
	e, ok := ret.(*ops.VolumeSeriesRequestListDefault)
	if assert.True(ok, "VolumeSeriesRequestListDefault") {
		assert.Equal(&expErr.Payload, e.Payload)
	}
}
