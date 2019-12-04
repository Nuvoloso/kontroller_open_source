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
	"net/http"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestStorageRequestFetchFilter(t *testing.T) {
	assert := assert.New(t)
	hc := newHandlerComp()
	obj := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{Meta: &models.ObjMeta{ID: "id1"}},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			AccountID:       "aid1",
			TenantAccountID: "tid1",
		},
	}
	rObj := &models.Role{}
	rObj.Capabilities = map[string]bool{}
	ai := &auth.Info{}

	t.Log("case: internal user")
	assert.NoError(hc.storageRequestFetchFilter(ai, obj))

	t.Log("case: SystemManagementCap capability succeeds")
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.SystemManagementCap: true}
	assert.NoError(hc.storageRequestFetchFilter(ai, obj))

	t.Log("case: VolumeSeriesOwnerCap capability fails")
	ai.RoleObj = &models.Role{}
	ai.AccountID = "aid1"
	ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.storageRequestFetchFilter(ai, obj))

	t.Log("case: CSPDomainManagementCap capability correct tenant")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	ai.AccountID = "tid1"
	assert.NoError(hc.storageRequestFetchFilter(ai, obj))

	t.Log("case: CSPDomainManagementCap capability wrong tenant")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	ai.AccountID = "tid2"
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.storageRequestFetchFilter(ai, obj))

	t.Log("case: owner account")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	ai.AccountID = "aid1"
	assert.NoError(hc.storageRequestFetchFilter(ai, obj))

	t.Log("case: not owner account")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	ai.AccountID = "aid9"
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.storageRequestFetchFilter(ai, obj))
}

func TestStorageRequestCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	cspDomainTypes := app.SupportedCspDomainTypes()
	assert.NotNil(cspDomainTypes)
	assert.NotEmpty(cspDomainTypes)
	dt := models.CspDomainTypeMutable("AWS")
	assert.Contains(cspDomainTypes, dt)
	cspStorageTypes := app.SupportedCspStorageTypes()
	var cstN models.CspStorageType
	var cstMaxSz int64
	for _, st := range cspStorageTypes {
		if st.CspDomainType == models.CspDomainType(dt) && st.AccessibilityScope == "CSPDOMAIN" {
			cstN = st.Name
			cstMaxSz = swag.Int64Value(st.MaxAllocationSizeBytes)
		}
	}
	assert.NotZero(cstN)
	assert.NotZero(cstMaxSz)
	obj := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:      "objectId",
				Version: 1,
			},
		},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			AccountID:       "aid4",
			TenantAccountID: "tid1",
		},
	}
	clusterObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "clusterId",
			},
		},
		ClusterCreateOnce: models.ClusterCreateOnce{
			AccountID:   "tid1",
			CspDomainID: "cspDomainId",
		},
		ClusterMutable: models.ClusterMutable{
			ClusterCreateMutable: models.ClusterCreateMutable{
				AuthorizedAccounts: []models.ObjIDMutable{"aid1", "aid2", "aid3", "aid4"},
			},
		},
	}
	nodeObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "nodeId",
			},
			AccountID: "tid1",
			ClusterID: models.ObjIDMutable(clusterObj.Meta.ID),
		},
		NodeMutable: models.NodeMutable{
			State: com.NodeStateManaged,
			Service: &models.NuvoService{
				ServiceState: models.ServiceState{
					State: com.ServiceStateReady,
				},
			},
		},
	}
	pObj := &models.Pool{
		PoolAllOf0: models.PoolAllOf0{
			Meta: &models.ObjMeta{
				ID: "poolID",
			},
		},
		PoolCreateOnce: models.PoolCreateOnce{
			AccountID:           "tid1",
			AuthorizedAccountID: "aid4",
			CspDomainID:         "cspDomainId",
			CspStorageType:      string(cstN),
			ClusterID:           "clusterId",
		},
	}
	storageObj := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta: &models.ObjMeta{
				ID: "storageId",
			},
			AccountID:       "aid4",
			TenantAccountID: "tid1",
			CspDomainID:     "cspDomainId",
			CspStorageType:  cstN,
			ClusterID:       "clusterId",
		},
	}
	storageObj.StorageState = &models.StorageStateMutable{AttachmentState: "DETACHED"}
	ai := &auth.Info{}
	params := ops.StorageRequestCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
		Payload:     &models.StorageRequestCreateArgs{},
	}
	params.Payload.RequestedOperations = []string{"PROVISION", "ATTACH"}
	params.Payload.CspStorageType = string(cstN)
	params.Payload.NodeID = models.ObjIDMutable(nodeObj.Meta.ID)
	params.Payload.PoolID = models.ObjIDMutable(pObj.Meta.ID)
	ctx := params.HTTPRequest.Context()
	sID := string(storageObj.Meta.ID)
	listParams := ops.StorageRequestListParams{StorageID: &sID, IsTerminated: swag.Bool(false)}

	// test various operation combinations
	opsValid := [][]string{ // in canonical order
		[]string{"PROVISION", "ATTACH", "FORMAT", "USE"},
		[]string{"PROVISION", "ATTACH", "FORMAT"},
		[]string{"PROVISION", "ATTACH"},
		[]string{"ATTACH", "FORMAT", "USE"},
		[]string{"CLOSE", "DETACH", "RELEASE"},
		[]string{"CLOSE", "DETACH"},
		[]string{"DETACH", "RELEASE"},
		[]string{"FORMAT", "USE"},
		[]string{"PROVISION"},
		[]string{"ATTACH"},
		[]string{"DETACH"},
		[]string{"RELEASE"},
		[]string{"FORMAT"},
		[]string{"USE"},
		[]string{"REATTACH"},
	}
	for _, ops := range opsValid {
		tl.Logger().Infof("case: isValid %v", ops)
		sop, err := hc.storageRequestValidateRequestedOperations(ops)
		tl.Logger().Debugf("    %v => %v", ops, sop.canonicalOrder)
		assert.NoError(err)
		assert.Equal(ops, sop.canonicalOrder)
		if sop.hasReattach {
			assert.True(sop.hasReattach)
			assert.True(sop.hasClose)
			assert.True(sop.hasDetach)
			assert.True(sop.hasAttach)
			assert.True(sop.hasUse)
		}
		if len(ops) > 1 {
			for i := 1; i < len(ops); i++ {
				rotateL(&ops, 1)
				iSop, err := hc.storageRequestValidateRequestedOperations(ops)
				tl.Logger().Debugf("    %v => %v", ops, iSop.canonicalOrder)
				assert.NoError(err)
				assert.Equal(sop.canonicalOrder, iSop.canonicalOrder)
			}
		}
	}
	opsInvalid := [][]string{
		[]string{"PROVISION", "CLOSE"},
		[]string{"PROVISION", "RELEASE"},
		[]string{"PROVISION", "DETACH"},
		[]string{"ATTACH", "DETACH"},
		[]string{"ATTACH", "RELEASE"},
		[]string{"FORMAT", "DETACH"},
		[]string{"FORMAT", "RELEASE"},
		[]string{"USE", "DETACH"},
		[]string{"USE", "RELEASE"},
		[]string{"DETACH", "PROVISION"},
		[]string{"DETACH", "ATTACH"},
		[]string{"RELEASE", "PROVISION"},
		[]string{"RELEASE", "ATTACH"},
		[]string{"RELEASE", "FORMAT"},
		[]string{"RELEASE", "USE"},
		[]string{"REATTACH", "PROVISION"},
		[]string{"REATTACH", "DETACH"},
	}
	for _, ops := range opsInvalid {
		tl.Logger().Infof("case: isNotValid %v", ops)
		sop, err := hc.storageRequestValidateRequestedOperations(ops)
		assert.Nil(sop)
		assert.Error(err)
		assert.Regexp("invalid operation combination", err)
	}
	_, err := hc.storageRequestValidateRequestedOperations([]string{"INVALID"})
	assert.Error(err)
	assert.Regexp("unsupported operation", err)

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	t.Log("case: success with provision and attach")
	var retObj *models.StorageRequest
	testutils.Clone(obj, &retObj)
	retObj.ClusterID = "clusterID"
	retObj.NodeID = "nodeID"
	retObj.PoolID = "poolID"
	retObj.StorageRequestState = "storageRequestState"
	mds := mock.NewMockDataStore(mockCtrl)
	oP := mock.NewMockPoolOps(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP)
	oP.EXPECT().Fetch(ctx, string(pObj.Meta.ID)).Return(pObj, nil)
	oN := mock.NewMockNodeOps(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN)
	oN.EXPECT().Fetch(ctx, string(params.Payload.NodeID)).Return(nodeObj, nil)
	oCL := mock.NewMockClusterOps(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oCL)
	oCL.EXPECT().Fetch(ctx, string(clusterObj.Meta.ID)).Return(clusterObj, nil)
	oSR := mock.NewMockStorageRequestOps(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR)
	oSR.EXPECT().Create(ctx, params.Payload).Return(retObj, nil)
	hc.DS = mds
	var ret middleware.Responder
	cntLock := hc.cntLock
	cntUnlock := hc.cntUnlock
	assert.Nil(evM.InSSProps)
	assert.NotPanics(func() { ret = hc.storageRequestCreate(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.StorageRequestCreateCreated)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Equal(clusterObj.Meta.ID, params.Payload.ClusterID)
	assert.EqualValues("cspDomainId", params.Payload.CspDomainID)
	assert.EqualValues("poolID", params.Payload.PoolID)
	assert.Zero(params.Payload.StorageID)
	assert.NotNil(evM.InSSProps)
	assert.Len(evM.InSSProps, 5)
	assert.EqualValues("clusterID", evM.InSSProps["clusterId"])
	assert.EqualValues("nodeID", evM.InSSProps["nodeId"])
	assert.EqualValues("storageRequestState", evM.InSSProps["storageRequestState"])
	assert.EqualValues("poolID", evM.InSSProps["poolId"])
	assert.EqualValues("objectId", evM.InSSProps["meta.id"])
	assert.Equal(retObj, evM.InACScope)

	// success with poolId, provision only
	mockCtrl.Finish()
	t.Log("case: success with provision only")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	params.Payload = &models.StorageRequestCreateArgs{}
	params.Payload.RequestedOperations = []string{"PROVISION"}
	params.Payload.PoolID = models.ObjIDMutable(pObj.Meta.ID)
	// creation parameters have values filled in from the pObj
	paramsCreate := ops.StorageRequestCreateParams{
		HTTPRequest: params.HTTPRequest,
		Payload:     &models.StorageRequestCreateArgs{},
	}
	paramsCreate.Payload.AccountID = models.ObjID(pObj.AuthorizedAccountID)
	paramsCreate.Payload.TenantAccountID = models.ObjID(pObj.AccountID)
	paramsCreate.Payload.RequestedOperations = []string{"PROVISION"}
	paramsCreate.Payload.PoolID = models.ObjIDMutable(pObj.Meta.ID)
	paramsCreate.Payload.ClusterID = models.ObjID(pObj.ClusterID)
	paramsCreate.Payload.CspDomainID = models.ObjID(pObj.CspDomainID)
	paramsCreate.Payload.CspStorageType = pObj.CspStorageType
	mds = mock.NewMockDataStore(mockCtrl)
	oP = mock.NewMockPoolOps(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP)
	oP.EXPECT().Fetch(ctx, string(pObj.Meta.ID)).Return(pObj, nil)
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR)
	oSR.EXPECT().Create(ctx, paramsCreate.Payload).Return(obj, nil)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.storageRequestCreate(params) })
	assert.NotNil(ret)
	_, ok = ret.(*ops.StorageRequestCreateCreated)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Zero(params.Payload.StorageID)
	tl.Flush()

	// successful DETACH
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: success DETACH")
	storageObj.StorageState = &models.StorageStateMutable{AttachmentState: "ATTACHED", AttachedNodeID: models.ObjIDMutable(nodeObj.Meta.ID)}
	params.Payload.RequestedOperations = []string{"DETACH"}
	params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
	params.Payload.ClusterID = ""
	params.Payload.CspDomainID = ""
	params.Payload.NodeID = ""
	params.Payload.PoolID = ""
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	oS := mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Fetch(ctx, string(storageObj.Meta.ID)).Return(storageObj, nil)
	oSR.EXPECT().List(ctx, listParams).Return([]*models.StorageRequest{}, nil)
	oSR.EXPECT().Create(ctx, params.Payload).Return(nil, centrald.ErrorExists)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS)
	mds.EXPECT().OpsStorageRequest().Return(oSR).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.storageRequestCreate(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.StorageRequestCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorExists.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.EqualValues("clusterId", params.Payload.ClusterID)
	assert.Equal(storageObj.AccountID, params.Payload.AccountID)
	assert.EqualValues("cspDomainId", params.Payload.CspDomainID)
	assert.Equal(nodeObj.Meta.ID, models.ObjID(params.Payload.NodeID))
	tl.Flush()

	// CLOSE
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: CLOSE")
	storageObj.StorageState = &models.StorageStateMutable{AttachmentState: "ATTACHED", AttachedNodeID: models.ObjIDMutable(nodeObj.Meta.ID) + "foo", MediaState: "FORMATTED"} // a different node
	params.Payload = &models.StorageRequestCreateArgs{}
	params.Payload.RequestedOperations = []string{"CLOSE"}
	params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
	params.Payload.NodeID = ""
	expParams := ops.StorageRequestCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	testutils.Clone(params.Payload, &expParams.Payload)
	expParams.Payload.AccountID = storageObj.AccountID
	expParams.Payload.TenantAccountID = storageObj.TenantAccountID
	expParams.Payload.ClusterID = storageObj.ClusterID                // from Storage
	expParams.Payload.CspDomainID = storageObj.CspDomainID            // from Storage
	expParams.Payload.NodeID = storageObj.StorageState.AttachedNodeID // from Storage
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	oS = mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Fetch(ctx, string(params.Payload.StorageID)).Return(storageObj, nil)
	oSR.EXPECT().List(ctx, listParams).Return([]*models.StorageRequest{}, nil)
	oSR.EXPECT().Create(ctx, expParams.Payload).Return(nil, centrald.ErrorExists)
	vslParams := volume_series.VolumeSeriesListParams{StorageID: swag.String(string(storageObj.Meta.ID))}
	oV := mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Count(ctx, vslParams, uint(1)).Return(0, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS)
	mds.EXPECT().OpsStorageRequest().Return(oSR).MinTimes(1)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	oN = mock.NewMockNodeOps(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN)
	oN.EXPECT().Fetch(ctx, string(expParams.Payload.NodeID)).Return(nodeObj, nil)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	ret = hc.storageRequestCreate(params)
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageRequestCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorExists.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	// REATTACH (storage attached to original node)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: REATTACH")
	storageObj.StorageState = &models.StorageStateMutable{AttachmentState: "ATTACHED", AttachedNodeID: models.ObjIDMutable(nodeObj.Meta.ID) + "-not", MediaState: "FORMATTED"} // a different node
	params.Payload = &models.StorageRequestCreateArgs{}
	params.Payload.RequestedOperations = []string{"REATTACH"}
	params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
	params.Payload.ReattachNodeID = models.ObjIDMutable(nodeObj.Meta.ID)
	expParams = ops.StorageRequestCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	testutils.Clone(params.Payload, &expParams.Payload)
	expParams.Payload.ClusterID = models.ObjID(nodeObj.ClusterID)
	expParams.Payload.AccountID = storageObj.AccountID
	expParams.Payload.TenantAccountID = storageObj.TenantAccountID
	expParams.Payload.CspDomainID = storageObj.CspDomainID
	expParams.Payload.NodeID = storageObj.StorageState.AttachedNodeID // from Storage
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, string(params.Payload.ReattachNodeID)).Return(nodeObj, nil)
	oN.EXPECT().Fetch(ctx, string(storageObj.StorageState.AttachedNodeID)).Return(nodeObj, nil)
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	oS = mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Fetch(ctx, string(params.Payload.StorageID)).Return(storageObj, nil)
	oSR.EXPECT().List(ctx, listParams).Return([]*models.StorageRequest{}, nil)
	oSR.EXPECT().Create(ctx, expParams.Payload).Return(nil, centrald.ErrorExists)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).Times(2)
	mds.EXPECT().OpsStorage().Return(oS)
	mds.EXPECT().OpsStorageRequest().Return(oSR).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	ret = hc.storageRequestCreate(params)
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageRequestCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorExists.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	// REATTACH (storage already detached)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: REATTACH")
	storageObj.StorageState = &models.StorageStateMutable{AttachmentState: "DETACHED", MediaState: "FORMATTED"}
	params.Payload = &models.StorageRequestCreateArgs{}
	params.Payload.RequestedOperations = []string{"REATTACH"}
	params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
	params.Payload.ReattachNodeID = models.ObjIDMutable(nodeObj.Meta.ID)
	expParams = ops.StorageRequestCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	testutils.Clone(params.Payload, &expParams.Payload)
	expParams.Payload.ClusterID = models.ObjID(nodeObj.ClusterID)
	expParams.Payload.AccountID = storageObj.AccountID
	expParams.Payload.TenantAccountID = storageObj.TenantAccountID
	expParams.Payload.CspDomainID = storageObj.CspDomainID
	expParams.Payload.NodeID = params.Payload.ReattachNodeID // destination
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, string(params.Payload.ReattachNodeID)).Return(nodeObj, nil).Times(2)
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	oS = mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Fetch(ctx, string(params.Payload.StorageID)).Return(storageObj, nil)
	oSR.EXPECT().List(ctx, listParams).Return([]*models.StorageRequest{}, nil)
	oSR.EXPECT().Create(ctx, expParams.Payload).Return(nil, centrald.ErrorExists)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).Times(2)
	mds.EXPECT().OpsStorage().Return(oS)
	mds.EXPECT().OpsStorageRequest().Return(oSR).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	ret = hc.storageRequestCreate(params)
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageRequestCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorExists.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	// REATTACH (storage attached to new node but not in use)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: REATTACH")
	storageObj.StorageState = &models.StorageStateMutable{AttachmentState: "ATTACHED", AttachedNodeID: models.ObjIDMutable(nodeObj.Meta.ID), MediaState: "FORMATTED", DeviceState: "ERROR"}
	params.Payload = &models.StorageRequestCreateArgs{}
	params.Payload.RequestedOperations = []string{"REATTACH"}
	params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
	params.Payload.ReattachNodeID = models.ObjIDMutable(nodeObj.Meta.ID)
	expParams = ops.StorageRequestCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	testutils.Clone(params.Payload, &expParams.Payload)
	expParams.Payload.ClusterID = models.ObjID(nodeObj.ClusterID)
	expParams.Payload.AccountID = storageObj.AccountID
	expParams.Payload.TenantAccountID = storageObj.TenantAccountID
	expParams.Payload.CspDomainID = storageObj.CspDomainID
	expParams.Payload.NodeID = storageObj.StorageState.AttachedNodeID // storage at destination
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, string(params.Payload.ReattachNodeID)).Return(nodeObj, nil)
	oN.EXPECT().Fetch(ctx, string(storageObj.StorageState.AttachedNodeID)).Return(nodeObj, nil)
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	oS = mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Fetch(ctx, string(params.Payload.StorageID)).Return(storageObj, nil)
	oSR.EXPECT().List(ctx, listParams).Return([]*models.StorageRequest{}, nil)
	oSR.EXPECT().Create(ctx, expParams.Payload).Return(nil, centrald.ErrorExists)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).Times(2)
	mds.EXPECT().OpsStorage().Return(oS)
	mds.EXPECT().OpsStorageRequest().Return(oSR).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	ret = hc.storageRequestCreate(params)
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageRequestCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorExists.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	// Create fails, valid auth
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: Create fails")
	ai.AccountID, ai.RoleObj = "aid4", &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	params.Payload = &models.StorageRequestCreateArgs{}
	params.Payload.RequestedOperations = []string{"PROVISION"}
	params.Payload.CspStorageType = ""
	params.Payload.ClusterID = ""
	params.Payload.CspDomainID = ""
	params.Payload.PoolID = models.ObjIDMutable(pObj.Meta.ID)
	mds = mock.NewMockDataStore(mockCtrl)
	oP = mock.NewMockPoolOps(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP)
	oP.EXPECT().Fetch(ctx, string(pObj.Meta.ID)).Return(pObj, nil)
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().Create(ctx, params.Payload).Return(nil, centrald.ErrorExists)
	mds.EXPECT().OpsStorageRequest().Return(oSR)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.storageRequestCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageRequestCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorExists.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.EqualValues("clusterId", params.Payload.ClusterID)
	assert.EqualValues("cspDomainId", params.Payload.CspDomainID)
	assert.EqualValues("aid4", params.Payload.AccountID)
	assert.Zero(params.Payload.StorageID)
	tl.Flush()

	// case: missing parcelSizeBytes, also tests canonical rewriting of ops
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: missing parcelSizeBytes and canonical rewriting of ops")
	params.Payload.RequestedOperations = []string{"ATTACH", "USE", "FORMAT", "PROVISION"}
	mds = mock.NewMockDataStore(mockCtrl)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.storageRequestCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageRequestCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp(centrald.ErrorMissing.M+".*parcelSizeBytes", *mE.Payload.Message)
	assert.Equal(cntLock, hc.cntLock)
	assert.Equal(cntUnlock, hc.cntUnlock)
	assert.Equal([]string{"PROVISION", "ATTACH", "FORMAT", "USE"}, params.Payload.RequestedOperations)
	tl.Flush()

	// case: completeByTime is past, also covers storageID present on ATTACH only, valid auth
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	storageObj.StorageState = &models.StorageStateMutable{AttachmentState: "DETACHED"}
	params.Payload.RequestedOperations = []string{"ATTACH"}
	params.Payload.CspStorageType = ""
	params.Payload.ClusterID = ""
	params.Payload.CspDomainID = ""
	params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
	params.Payload.CompleteByTime = strfmt.DateTime(time.Now().Add(-time.Hour))
	t.Log("case: completeByTime is past")
	mds = mock.NewMockDataStore(mockCtrl)
	oS = mock.NewMockStorageOps(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS)
	oS.EXPECT().Fetch(ctx, string(storageObj.Meta.ID)).Return(storageObj, nil)
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().List(ctx, listParams).Return([]*models.StorageRequest{}, nil)
	mds.EXPECT().OpsStorageRequest().Return(oSR)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.storageRequestCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageRequestCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("completeByTime.*past", *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.EqualValues("cspDomainId", params.Payload.CspDomainID)
	params.Payload.CompleteByTime = strfmt.DateTime(time.Now().Add(time.Hour))
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.storageRequestCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageRequestCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: unauthorized on storage")
	ai.AccountID, ai.RoleObj = "other", &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	params.Payload.RequestedOperations = []string{"ATTACH"}
	params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
	mds = mock.NewMockDataStore(mockCtrl)
	oS = mock.NewMockStorageOps(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS)
	oS.EXPECT().Fetch(ctx, string(storageObj.Meta.ID)).Return(storageObj, nil)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageRequestCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageRequestCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	ai.AccountID, ai.RoleObj = "", nil
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: unauthorized on cluster, no authorized accounts")
	ai.AccountID, ai.RoleObj = "aid4", &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	params.Payload.RequestedOperations = []string{"ATTACH"}
	params.Payload.NodeID = models.ObjIDMutable(nodeObj.Meta.ID)
	params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
	var badClusterObj *models.Cluster
	testutils.Clone(clusterObj, &badClusterObj)
	badClusterObj.AuthorizedAccounts = []models.ObjIDMutable{}
	mds = mock.NewMockDataStore(mockCtrl)
	oS = mock.NewMockStorageOps(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS)
	oS.EXPECT().Fetch(ctx, string(storageObj.Meta.ID)).Return(storageObj, nil)
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR)
	oSR.EXPECT().List(ctx, listParams).Return([]*models.StorageRequest{}, nil)
	oN = mock.NewMockNodeOps(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN)
	oN.EXPECT().Fetch(ctx, string(params.Payload.NodeID)).Return(nodeObj, nil)
	oCL = mock.NewMockClusterOps(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oCL)
	oCL.EXPECT().Fetch(ctx, string(badClusterObj.Meta.ID)).Return(badClusterObj, nil)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageRequestCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageRequestCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	ai.AccountID, ai.RoleObj = "", nil
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: unauthorized on cluster, wrong authorized account")
	ai.AccountID, ai.RoleObj = "aid4", &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	params.Payload.RequestedOperations = []string{"ATTACH"}
	params.Payload.NodeID = models.ObjIDMutable(nodeObj.Meta.ID)
	params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
	testutils.Clone(clusterObj, &badClusterObj)
	badClusterObj.AuthorizedAccounts = []models.ObjIDMutable{"tid1", "aid1"}
	mds = mock.NewMockDataStore(mockCtrl)
	oS = mock.NewMockStorageOps(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS)
	oS.EXPECT().Fetch(ctx, string(storageObj.Meta.ID)).Return(storageObj, nil)
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR)
	oSR.EXPECT().List(ctx, listParams).Return([]*models.StorageRequest{}, nil)
	oN = mock.NewMockNodeOps(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN)
	oN.EXPECT().Fetch(ctx, string(params.Payload.NodeID)).Return(nodeObj, nil)
	oCL = mock.NewMockClusterOps(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oCL)
	oCL.EXPECT().Fetch(ctx, string(badClusterObj.Meta.ID)).Return(badClusterObj, nil)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageRequestCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageRequestCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	ai.AccountID, ai.RoleObj = "", nil
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: unauthorized on pool")
	ai.AccountID, ai.RoleObj = "other", &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	params.Payload.RequestedOperations = []string{"PROVISION"}
	params.Payload.PoolID = models.ObjIDMutable(pObj.Meta.ID)
	mds = mock.NewMockDataStore(mockCtrl)
	oP = mock.NewMockPoolOps(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP)
	oP.EXPECT().Fetch(ctx, string(pObj.Meta.ID)).Return(pObj, nil)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageRequestCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageRequestCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	ai.AccountID, ai.RoleObj = "", nil
	tl.Flush()

	// remaining error cases
	for tc := 1; tc <= 28; tc++ {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		oCL = mock.NewMockClusterOps(mockCtrl)
		oN = mock.NewMockNodeOps(mockCtrl)
		oS = mock.NewMockStorageOps(mockCtrl)
		oSR = mock.NewMockStorageRequestOps(mockCtrl)
		oP = mock.NewMockPoolOps(mockCtrl)
		mds = mock.NewMockDataStore(mockCtrl)
		storageObj.CspDomainID = "cspDomainId"
		storageObj.StorageState = &models.StorageStateMutable{AttachmentState: "DETACHED"}
		params.Payload.CspDomainID = ""
		params.Payload.CspStorageType = ""
		params.Payload.PoolID = models.ObjIDMutable(pObj.Meta.ID)
		params.Payload.NodeID = ""
		params.Payload.MinSizeBytes = nil
		expErr := centrald.ErrorMissing
		errPat := ".*"
		lockInc := uint32(1)
		switch tc {
		case 1:
			t.Log("case: StorageRequestCreate DETACH storageId fetch error")
			params.Payload.RequestedOperations = []string{"DETACH"}
			params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
			expErr = centrald.ErrorDbError
			mds.EXPECT().OpsStorage().Return(oS)
			oS.EXPECT().Fetch(ctx, string(storageObj.Meta.ID)).Return(nil, expErr)
		case 2:
			t.Log("case: StorageRequestCreate RELEASE storageId not found")
			params.Payload.RequestedOperations = []string{"RELEASE"}
			params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
			mds.EXPECT().OpsStorage().Return(oS)
			oS.EXPECT().Fetch(ctx, string(storageObj.Meta.ID)).Return(nil, centrald.ErrorNotFound)
		case 3:
			t.Log("case: StorageRequestCreate RELEASE storageRequest list error")
			params.Payload.RequestedOperations = []string{"RELEASE"}
			params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
			expErr = centrald.ErrorDbError
			mds.EXPECT().OpsStorage().Return(oS)
			oS.EXPECT().Fetch(ctx, string(storageObj.Meta.ID)).Return(storageObj, nil)
			mds.EXPECT().OpsStorageRequest().Return(oSR)
			oSR.EXPECT().List(ctx, listParams).Return(nil, expErr)
		case 4:
			t.Log("case: StorageRequestCreate RELEASE storageId in use")
			params.Payload.RequestedOperations = []string{"RELEASE"}
			params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
			mds.EXPECT().OpsStorage().Return(oS)
			oS.EXPECT().Fetch(ctx, string(storageObj.Meta.ID)).Return(storageObj, nil)
			mds.EXPECT().OpsStorageRequest().Return(oSR)
			oSR.EXPECT().List(ctx, listParams).Return([]*models.StorageRequest{obj}, nil)
		case 5:
			t.Log("case: StorageRequestCreate ATTACH node fetch error")
			params.Payload.RequestedOperations = []string{"ATTACH"}
			params.Payload.NodeID = models.ObjIDMutable(nodeObj.Meta.ID)
			params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
			expErr = centrald.ErrorDbError
			mds.EXPECT().OpsStorage().Return(oS)
			oS.EXPECT().Fetch(ctx, string(storageObj.Meta.ID)).Return(storageObj, nil)
			mds.EXPECT().OpsStorageRequest().Return(oSR)
			oSR.EXPECT().List(ctx, listParams).Return([]*models.StorageRequest{}, nil)
			mds.EXPECT().OpsNode().Return(oN)
			oN.EXPECT().Fetch(ctx, string(nodeObj.Meta.ID)).Return(nil, expErr)
		case 6:
			t.Log("case: StorageRequestCreate ATTACH node not found")
			params.Payload.RequestedOperations = []string{"ATTACH"}
			params.Payload.NodeID = models.ObjIDMutable(nodeObj.Meta.ID)
			params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
			mds.EXPECT().OpsStorage().Return(oS)
			oS.EXPECT().Fetch(ctx, string(storageObj.Meta.ID)).Return(storageObj, nil)
			mds.EXPECT().OpsStorageRequest().Return(oSR)
			oSR.EXPECT().List(ctx, listParams).Return([]*models.StorageRequest{}, nil)
			mds.EXPECT().OpsNode().Return(oN)
			oN.EXPECT().Fetch(ctx, string(nodeObj.Meta.ID)).Return(nil, centrald.ErrorNotFound)
		case 7:
			t.Log("case: StorageRequestCreate PROVISION+ATTACH cluster fetch fails")
			params.Payload.RequestedOperations = []string{"ATTACH", "PROVISION"}
			params.Payload.NodeID = "nodeId"
			expErr = centrald.ErrorDbError
			mds.EXPECT().OpsPool().Return(oP)
			oP.EXPECT().Fetch(ctx, string(pObj.Meta.ID)).Return(pObj, nil)
			mds.EXPECT().OpsNode().Return(oN)
			oN.EXPECT().Fetch(ctx, string(nodeObj.Meta.ID)).Return(nodeObj, nil)
			mds.EXPECT().OpsCluster().Return(oCL)
			oCL.EXPECT().Fetch(ctx, string(clusterObj.Meta.ID)).Return(nil, expErr)
		case 8:
			t.Log("case: StorageRequestCreate ATTACH wrong storage cspDomain for node/cluster cspDomain")
			params.Payload.RequestedOperations = []string{"ATTACH"}
			params.Payload.NodeID = "nodeId"
			params.Payload.StorageID = "storageId"
			storageObj.CspDomainID = "not my domain"
			mds.EXPECT().OpsStorage().Return(oS)
			oS.EXPECT().Fetch(ctx, string(storageObj.Meta.ID)).Return(storageObj, nil)
			mds.EXPECT().OpsStorageRequest().Return(oSR)
			oSR.EXPECT().List(ctx, listParams).Return([]*models.StorageRequest{}, nil)
			mds.EXPECT().OpsNode().Return(oN)
			oN.EXPECT().Fetch(ctx, string(nodeObj.Meta.ID)).Return(nodeObj, nil)
			mds.EXPECT().OpsCluster().Return(oCL)
			oCL.EXPECT().Fetch(ctx, string(clusterObj.Meta.ID)).Return(clusterObj, nil)
		case 9:
			t.Log("case: ATTACH but storage attachedNodeID already set")
			storageObj.StorageState = &models.StorageStateMutable{AttachmentState: "ATTACHED", AttachedNodeID: "node2"}
			params.Payload.RequestedOperations = []string{"ATTACH"}
			mds.EXPECT().OpsStorage().Return(oS)
			oS.EXPECT().Fetch(ctx, string(storageObj.Meta.ID)).Return(storageObj, nil)
		case 10:
			t.Log("case: PROVISION with minSizeBytes > storageType MaxAllocationBytes")
			params.Payload.RequestedOperations = []string{"PROVISION"}
			params.Payload.MinSizeBytes = swag.Int64(cstMaxSz + 100)
			mds.EXPECT().OpsPool().Return(oP)
			oP.EXPECT().Fetch(ctx, string(pObj.Meta.ID)).Return(pObj, nil)
		case 11:
			// this tests the error path - operation combinatorics are tested earlier
			t.Log("case: invalid operation sequence")
			params.Payload.RequestedOperations = []string{"PROVISION", "RELEASE"}
			lockInc = 0
		case 12:
			t.Log("case: FORMAT, storage not ATTACHED and no ATTACH op")
			storageObj.StorageState = &models.StorageStateMutable{AttachmentState: "DETACHED"}
			params.Payload.RequestedOperations = []string{"FORMAT"}
			params.Payload.ParcelSizeBytes = swag.Int64(10000)
			params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
			mds.EXPECT().OpsStorage().Return(oS)
			oS.EXPECT().Fetch(ctx, string(storageObj.Meta.ID)).Return(storageObj, nil)
		case 13:
			t.Log("case: USE, storage not ATTACHED and no ATTACH op")
			storageObj.StorageState = &models.StorageStateMutable{AttachmentState: "DETACHED"}
			params.Payload.RequestedOperations = []string{"USE"}
			params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
			mds.EXPECT().OpsStorage().Return(oS)
			oS.EXPECT().Fetch(ctx, string(storageObj.Meta.ID)).Return(storageObj, nil)
		case 14:
			t.Log("case: FORMAT, no storageId and no PROVISION+ATTACH op")
			params.Payload.RequestedOperations = []string{"FORMAT", "PROVISION"}
			params.Payload.ParcelSizeBytes = swag.Int64(10000)
			params.Payload.StorageID = ""
			mds.EXPECT().OpsPool().Return(oP)
			oP.EXPECT().Fetch(ctx, string(pObj.Meta.ID)).Return(pObj, nil)
		case 15:
			t.Log("case: USE, no storageId and no PROVISION+ATTACH op")
			params.Payload.RequestedOperations = []string{"USE", "PROVISION"}
			params.Payload.StorageID = ""
			mds.EXPECT().OpsPool().Return(oP)
			oP.EXPECT().Fetch(ctx, string(pObj.Meta.ID)).Return(pObj, nil)
		case 16:
			t.Log("case: PROVISION+ATTACH+USE no FORMAT")
			params.Payload.RequestedOperations = []string{"USE", "PROVISION", "ATTACH"}
			params.Payload.StorageID = ""
			params.Payload.NodeID = "nodeId"
			mds.EXPECT().OpsPool().Return(oP)
			oP.EXPECT().Fetch(ctx, string(pObj.Meta.ID)).Return(pObj, nil)
		case 17:
			t.Log("case: USE media attached but unformatted")
			storageObj.StorageState = &models.StorageStateMutable{AttachmentState: "ATTACHED", MediaState: "UNFORMATTED"}
			params.Payload.RequestedOperations = []string{"USE"}
			params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
			mds.EXPECT().OpsStorage().Return(oS)
			oS.EXPECT().Fetch(ctx, string(storageObj.Meta.ID)).Return(storageObj, nil)
			mds.EXPECT().OpsStorageRequest().Return(oSR)
			oSR.EXPECT().List(ctx, listParams).Return([]*models.StorageRequest{}, nil)
		case 18:
			t.Log("case: invalid poolId")
			params.Payload.PoolID = models.ObjIDMutable(pObj.Meta.ID)
			params.Payload.RequestedOperations = []string{"PROVISION"}
			mds.EXPECT().OpsPool().Return(oP)
			oP.EXPECT().Fetch(ctx, string(pObj.Meta.ID)).Return(nil, centrald.ErrorNotFound)
		case 19:
			t.Log("case: node and poolId in different CSP domains")
			badSP := &models.Pool{
				PoolAllOf0: models.PoolAllOf0{
					Meta: &models.ObjMeta{
						ID: "badSpId",
					},
				},
				PoolCreateOnce: models.PoolCreateOnce{
					CspDomainID:    "NOT-THE-CLUSTER-CSP-DOMAIN",
					CspStorageType: string(cstN),
				},
			}
			params.Payload.PoolID = models.ObjIDMutable(badSP.Meta.ID)
			params.Payload.RequestedOperations = []string{"PROVISION", "ATTACH"}
			params.Payload.NodeID = models.ObjIDMutable(nodeObj.Meta.ID)
			mds.EXPECT().OpsPool().Return(oP)
			oP.EXPECT().Fetch(ctx, string(badSP.Meta.ID)).Return(badSP, nil)
			mds.EXPECT().OpsNode().Return(oN)
			oN.EXPECT().Fetch(ctx, string(params.Payload.NodeID)).Return(nodeObj, nil)
			mds.EXPECT().OpsCluster().Return(oCL)
			oCL.EXPECT().Fetch(ctx, string(clusterObj.Meta.ID)).Return(clusterObj, nil)
		case 20:
			t.Log("case: CLOSE, storage not ATTACHED")
			storageObj.StorageState = &models.StorageStateMutable{AttachmentState: "DETACHED"}
			params.Payload.RequestedOperations = []string{"CLOSE"}
			params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
			mds.EXPECT().OpsStorage().Return(oS)
			oS.EXPECT().Fetch(ctx, string(storageObj.Meta.ID)).Return(storageObj, nil)
			mds.EXPECT().OpsStorageRequest().Return(oSR)
			oSR.EXPECT().List(ctx, listParams).Return([]*models.StorageRequest{}, nil)
		case 21:
			t.Log("case: CLOSE, storage is referenced in VolumeSeries")
			storageObj.StorageState = &models.StorageStateMutable{AttachmentState: "ATTACHED"}
			params.Payload.RequestedOperations = []string{"CLOSE"}
			params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
			mds.EXPECT().OpsStorage().Return(oS)
			oS.EXPECT().Fetch(ctx, string(storageObj.Meta.ID)).Return(storageObj, nil)
			mds.EXPECT().OpsStorageRequest().Return(oSR).AnyTimes()
			oSR.EXPECT().List(ctx, listParams).Return([]*models.StorageRequest{}, nil)
			sID := string(storageObj.Meta.ID)
			vslParams = volume_series.VolumeSeriesListParams{StorageID: &sID}
			oV = mock.NewMockVolumeSeriesOps(mockCtrl)
			oV.EXPECT().Count(ctx, vslParams, uint(1)).Return(2, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			expErr = centrald.ErrorRequestInConflict
		case 22:
			t.Log("case: REATTACH node not in same cluster")
			errPat = "reattachNodeId not in the same cluster"
			storageObj.StorageState = &models.StorageStateMutable{AttachmentState: "ATTACHED", AttachedNodeID: models.ObjIDMutable(nodeObj.Meta.ID), MediaState: "FORMATTED"} // different cluster
			params.Payload = &models.StorageRequestCreateArgs{}
			params.Payload.RequestedOperations = []string{"REATTACH"}
			params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
			params.Payload.ReattachNodeID = models.ObjIDMutable(nodeObj.Meta.ID)
			var newNode *models.Node
			testutils.Clone(nodeObj, &newNode)
			newNode.ClusterID = nodeObj.ClusterID + "foo"
			mds.EXPECT().OpsNode().Return(oN)
			oN.EXPECT().Fetch(ctx, string(params.Payload.ReattachNodeID)).Return(newNode, nil)
			mds.EXPECT().OpsStorage().Return(oS)
			oS.EXPECT().Fetch(ctx, string(params.Payload.StorageID)).Return(storageObj, nil)
			mds.EXPECT().OpsStorageRequest().Return(oSR).AnyTimes()
			oSR.EXPECT().List(ctx, listParams).Return([]*models.StorageRequest{}, nil)
		case 23:
			t.Log("case: REATTACH storage already in use on node")
			errPat = "storage already attached and in use on reattachNodeId"
			storageObj.StorageState = &models.StorageStateMutable{AttachmentState: "ATTACHED", AttachedNodeID: models.ObjIDMutable(nodeObj.Meta.ID), MediaState: "FORMATTED", DeviceState: "OPEN"}
			params.Payload = &models.StorageRequestCreateArgs{}
			params.Payload.RequestedOperations = []string{"REATTACH"}
			params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
			params.Payload.ReattachNodeID = models.ObjIDMutable(nodeObj.Meta.ID)
			mds.EXPECT().OpsNode().Return(oN)
			oN.EXPECT().Fetch(ctx, string(params.Payload.ReattachNodeID)).Return(nodeObj, nil)
			mds.EXPECT().OpsStorage().Return(oS)
			oS.EXPECT().Fetch(ctx, string(params.Payload.StorageID)).Return(storageObj, nil)
			mds.EXPECT().OpsStorageRequest().Return(oSR).AnyTimes()
			oSR.EXPECT().List(ctx, listParams).Return([]*models.StorageRequest{}, nil)
		case 24:
			t.Log("case: REATTACH with no StorageId")
			params.Payload = &models.StorageRequestCreateArgs{}
			params.Payload.RequestedOperations = []string{"REATTACH"}
			params.Payload.ReattachNodeID = models.ObjIDMutable(nodeObj.Meta.ID)
			mds.EXPECT().OpsStorage().Return(oS)
			oS.EXPECT().Fetch(ctx, "").Return(nil, centrald.ErrorNotFound)
		case 25:
			t.Log("case: REATTACH with no reattachNodeId")
			storageObj.StorageState = &models.StorageStateMutable{AttachmentState: "ATTACHED", AttachedNodeID: models.ObjIDMutable(nodeObj.Meta.ID), MediaState: "FORMATTED"}
			params.Payload = &models.StorageRequestCreateArgs{}
			params.Payload.RequestedOperations = []string{"REATTACH"}
			params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
			mds.EXPECT().OpsNode().Return(oN)
			oN.EXPECT().Fetch(ctx, "").Return(nil, centrald.ErrorNotFound)
			mds.EXPECT().OpsStorage().Return(oS)
			oS.EXPECT().Fetch(ctx, string(params.Payload.StorageID)).Return(storageObj, nil)
			mds.EXPECT().OpsStorageRequest().Return(oSR).AnyTimes()
			oSR.EXPECT().List(ctx, listParams).Return([]*models.StorageRequest{}, nil)
		case 26:
			t.Log("case: CLOSE loadNode fails")
			storageObj.StorageState = &models.StorageStateMutable{AttachmentState: "ATTACHED"}
			storageObj.StorageState.AttachedNodeID = "someID"
			params.Payload.RequestedOperations = []string{"CLOSE"}
			params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
			mds.EXPECT().OpsStorage().Return(oS)
			oS.EXPECT().Fetch(ctx, string(storageObj.Meta.ID)).Return(storageObj, nil)
			mds.EXPECT().OpsStorageRequest().Return(oSR).AnyTimes()
			oSR.EXPECT().List(ctx, listParams).Return([]*models.StorageRequest{}, nil)
			sID := string(storageObj.Meta.ID)
			vslParams = volume_series.VolumeSeriesListParams{StorageID: &sID}
			oV = mock.NewMockVolumeSeriesOps(mockCtrl)
			oV.EXPECT().Count(ctx, vslParams, uint(1)).Return(0, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			mds.EXPECT().OpsNode().Return(oN)
			oN.EXPECT().Fetch(ctx, "someID").Return(nil, centrald.ErrorNotFound)
		case 27:
			t.Log("case: CLOSE bad state")
			storageObj.StorageState = &models.StorageStateMutable{AttachmentState: "ATTACHED"}
			storageObj.StorageState.AttachedNodeID = "someID"
			params.Payload.RequestedOperations = []string{"CLOSE"}
			params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
			nodeObj.State = "TEAR_DOWN"
			mds.EXPECT().OpsStorage().Return(oS)
			oS.EXPECT().Fetch(ctx, string(storageObj.Meta.ID)).Return(storageObj, nil)
			mds.EXPECT().OpsStorageRequest().Return(oSR).AnyTimes()
			oSR.EXPECT().List(ctx, listParams).Return([]*models.StorageRequest{}, nil)
			sID := string(storageObj.Meta.ID)
			vslParams = volume_series.VolumeSeriesListParams{StorageID: &sID}
			oV = mock.NewMockVolumeSeriesOps(mockCtrl)
			oV.EXPECT().Count(ctx, vslParams, uint(1)).Return(0, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			mds.EXPECT().OpsNode().Return(oN)
			oN.EXPECT().Fetch(ctx, "someID").Return(nodeObj, nil)
			expErr = centrald.ErrorInvalidState
		case 28:
			t.Log("case: REATTACH bad state")
			storageObj.StorageState = &models.StorageStateMutable{AttachmentState: "ATTACHED"}
			storageObj.StorageState.AttachedNodeID = "someID"
			storageObj.StorageState.MediaState = com.StgMediaStateFormatted
			params.Payload.RequestedOperations = []string{"REATTACH"}
			params.Payload.StorageID = models.ObjIDMutable(storageObj.Meta.ID)
			params.Payload.ReattachNodeID = models.ObjIDMutable("reaNid")
			nodeObj.State = "TEAR_DOWN"
			nodeObj.Service = &models.NuvoService{}
			nodeObj.Service.State = com.ServiceStateUnknown
			mds.EXPECT().OpsStorage().Return(oS)
			oS.EXPECT().Fetch(ctx, string(storageObj.Meta.ID)).Return(storageObj, nil)
			mds.EXPECT().OpsStorageRequest().Return(oSR).AnyTimes()
			oSR.EXPECT().List(ctx, listParams).Return([]*models.StorageRequest{}, nil)
			mds.EXPECT().OpsNode().Return(oN)
			oN.EXPECT().Fetch(ctx, "reaNid").Return(nodeObj, nil)
			expErr = centrald.ErrorInvalidState
		default:
			assert.True(false)
		}
		hc.DS = mds
		cntLock = hc.cntLock
		cntUnlock = hc.cntUnlock
		assert.NotPanics(func() { ret = hc.storageRequestCreate(params) })
		assert.NotNil(ret)
		mE, ok = ret.(*ops.StorageRequestCreateDefault)
		assert.True(ok)
		assert.Equal(expErr.C, int(mE.Payload.Code))
		assert.Regexp("^"+expErr.M, *mE.Payload.Message)
		assert.Regexp(errPat, *mE.Payload.Message)
		assert.Equal(cntLock+lockInc, hc.cntLock)
		assert.Equal(cntUnlock+lockInc, hc.cntUnlock)
		tl.Flush()
	}

	// no payload
	params.Payload = nil
	t.Log("case: nil payload")
	assert.NotPanics(func() { ret = hc.storageRequestCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageRequestCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorMissing.M, *mE.Payload.Message)
}

func TestStorageRequestDelete(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.StorageRequestDeleteParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectId",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:      "objectId",
				Version: 2,
			},
		},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			AccountID:       "aid1",
			TenantAccountID: "tid1",
			ClusterID:       "clusterID",
			PoolID:          "poolID",
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestCreateMutable: models.StorageRequestCreateMutable{
				NodeID: "nodeID",
			},
		},
	}
	obj.StorageRequestState = "FAILED"

	// success, no claims
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oSR := mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().Delete(ctx, params.ID).Return(nil)
	oSR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR).MinTimes(1)
	t.Log("case: success, no claims")
	tl.Flush()
	hc.DS = mds
	var ret middleware.Responder
	cntLock := hc.cntLock
	cntUnlock := hc.cntUnlock
	assert.Nil(evM.InSSProps)
	assert.NotPanics(func() { ret = hc.storageRequestDelete(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.StorageRequestDeleteNoContent)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.NotNil(evM.InSSProps)
	assert.Len(evM.InSSProps, 4)
	assert.EqualValues("clusterID", evM.InSSProps["clusterId"])
	assert.EqualValues("nodeID", evM.InSSProps["nodeId"])
	assert.EqualValues("FAILED", evM.InSSProps["storageRequestState"])
	assert.EqualValues("poolID", evM.InSSProps["poolId"])
	assert.Equal(obj, evM.InACScope)

	// success, claim with VSR terminated, valid owner auth
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: success, claim with VSR terminated")
	tl.Flush()
	ai.AccountID, ai.RoleObj = "aid1", &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	obj.VolumeSeriesRequestClaims = &models.VsrClaim{
		Claims: map[string]models.VsrClaimElement{
			"VSR-1": models.VsrClaimElement{},
		},
	}
	vsrObj := &models.VolumeSeriesRequest{}
	vsrObj.VolumeSeriesRequestState = "FAILED"
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().Delete(ctx, params.ID).Return(nil)
	oSR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oVSR := mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVSR.EXPECT().Fetch(ctx, "VSR-1").Return(vsrObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR).MinTimes(1)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVSR)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.storageRequestDelete(params) })
	assert.NotNil(ret)
	_, ok = ret.(*ops.StorageRequestDeleteNoContent)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Len(evM.InSSProps, 4)
	assert.Equal(obj, evM.InACScope)

	// failure, claim with VSR active, valid tenant auth
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: success, claim with active VSR")
	tl.Flush()
	ai.AccountID, ai.RoleObj = "tid1", &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	vsrObj.VolumeSeriesRequestState = "STORAGE_WAIT"
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oVSR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVSR.EXPECT().Fetch(ctx, "VSR-1").Return(vsrObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR).MinTimes(1)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVSR)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.storageRequestDelete(params) })
	assert.NotNil(ret)
	assert.NotNil(ret)
	mE, ok := ret.(*ops.StorageRequestDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorExists.M+".*Active VolumeSeriesRequest VSR-1 has a claim", *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)

	// success, claim with VSR not found
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: success, claim with VSR not found")
	tl.Flush()
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().Delete(ctx, params.ID).Return(nil)
	oSR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oVSR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVSR.EXPECT().Fetch(ctx, "VSR-1").Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR).MinTimes(1)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVSR)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.storageRequestDelete(params) })
	assert.NotNil(ret)
	_, ok = ret.(*ops.StorageRequestDeleteNoContent)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)

	// failure, claim with other VSR fetch error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: success, claim with VSR not found")
	tl.Flush()
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oVSR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVSR.EXPECT().Fetch(ctx, "VSR-1").Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR).MinTimes(1)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVSR)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.storageRequestDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageRequestDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)

	// delete failure case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: delete failure")
	tl.Flush()
	obj.VolumeSeriesRequestClaims = nil
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().Delete(ctx, params.ID).Return(centrald.ErrorDbError)
	oSR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.storageRequestDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageRequestDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)

	// storage request active case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: storage request active")
	tl.Flush()
	obj.StorageRequestState = "PROVISIONING"
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.storageRequestDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageRequestDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Regexp("still active", *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.storageRequestDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageRequestDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized")
	ai.AccountID, ai.RoleObj = "otherID", &models.Role{}
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.storageRequestDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageRequestDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	ai.AccountID, ai.RoleObj = "", nil
	tl.Flush()

	// fetch failure case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: fetch failure")
	tl.Flush()
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.storageRequestDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageRequestDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
	assert.Equal(cntLock, hc.cntLock)
	assert.Equal(cntUnlock, hc.cntUnlock)
}

func TestStorageRequestFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.StorageRequestFetchParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectId",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "objectId",
			},
		},
		StorageRequestMutable: models.StorageRequestMutable{},
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oSR := mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR).MinTimes(1)
	t.Log("case: StorageRequestFetch success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.storageRequestFetch(params) })
	assert.NotNil(ret)
	mR, ok := ret.(*ops.StorageRequestFetchOK)
	assert.True(ok)
	assert.Equal(obj, mR.Payload)
	tl.Flush()

	// fetch failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR).MinTimes(1)
	t.Log("case: StorageRequestFetch fetch failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageRequestFetch(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.StorageRequestFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.storageRequestFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageRequestFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized")
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR)
	hc.DS = mds
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	assert.NotPanics(func() { ret = hc.storageRequestFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageRequestFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
}

func TestStorageRequestList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.StorageRequestListParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	ctx := params.HTTPRequest.Context()
	objects := []*models.StorageRequest{
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "objectId1",
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				AccountID:       "aid1",
				TenantAccountID: "tid1",
			},
		},
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "objectId2",
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				AccountID:       "aid2",
				TenantAccountID: "tid1",
			},
		},
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "objectId3",
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				AccountID:       "aid3",
				TenantAccountID: "tid3",
			},
		},
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ai.AccountID, ai.RoleObj = "sid1", &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.SystemManagementCap: true}
	oSR := mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().List(ctx, params).Return(objects, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR).MinTimes(1)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.storageRequestList(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.StorageRequestListOK)
	assert.True(ok)
	assert.Equal(objects, mO.Payload)
	tl.Flush()

	// list fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: List failure")
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().List(ctx, params).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageRequestList(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.StorageRequestListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.storageRequestList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageRequestListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: unauthorized filtered")
	ai.AccountID, ai.RoleObj = "aid2", &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().List(ctx, params).Return(objects, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageRequestList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.StorageRequestListOK)
	assert.True(ok)
	if assert.Len(mO.Payload, 1) {
		assert.EqualValues("objectId2", mO.Payload[0].Meta.ID)
	}
}

func TestStorageRequestUpdate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app

	// make the name map
	var nMap JSONToAttrNameMap
	assert.NotPanics(func() { nMap = hc.storageRequestMutableNameMap() })
	// validate some embedded properties
	assert.Equal("requestMessages", nMap.jName("RequestMessages"))
	assert.Equal("storageId", nMap.jName("StorageID"))
	assert.Equal("volumeSeriesRequestClaims", nMap.jName("VolumeSeriesRequestClaims"))

	// parse params
	objM := &models.StorageRequestMutable{
		StorageRequestCreateMutable: models.StorageRequestCreateMutable{
			VolumeSeriesRequestClaims: &models.VsrClaim{
				Claims: map[string]models.VsrClaimElement{
					"VSR-1": models.VsrClaimElement{},
				},
			},
		},
	}
	objM.StorageRequestState = "CAPACITY_WAIT"
	objM.StorageID = "storageID"
	ai := &auth.Info{}
	params := ops.StorageRequestUpdateParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectId",
		Version:     8,
		Remove:      []string{},
		Append:      []string{},
		Set: []string{
			nMap.jName("StorageRequestState"),
			nMap.jName("StorageID"),
			nMap.jName("VolumeSeriesRequestClaims"),
		},
		Payload: objM,
	}
	ctx := params.HTTPRequest.Context()
	var ua *centrald.UpdateArgs
	var err error
	var uP = [centrald.NumActionTypes][]string{
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    params.Set,
	}
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, &params.Version, uP) })
	assert.NoError(err)
	assert.NotNil(ua)
	uaM := updateArgsMatcher(t, ua).Matcher()

	// success, all properties set
	obj := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:      models.ObjID(params.ID),
				Version: 8,
			},
		},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			AccountID:           "aid1",
			TenantAccountID:     "tid1",
			PoolID:              "pid1",
			RequestedOperations: []string{"PROVISION"},
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
				StorageRequestState: com.StgReqStateProvisioning,
			},
		},
	}
	obj.NodeID = "nodeID"
	sObj := &models.Storage{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	var ret middleware.Responder
	oSR := mock.NewMockStorageRequestOps(mockCtrl)
	oS := mock.NewMockStorageOps(mockCtrl)
	oVSR := mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS)
	mds.EXPECT().OpsStorageRequest().Return(oSR).Times(2)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVSR)
	oS.EXPECT().Fetch(ctx, string(objM.StorageID)).Return(sObj, nil)
	var retObj *models.StorageRequest
	testutils.Clone(obj, &retObj)
	retObj.ClusterID = "clusterID"
	retObj.NodeID = "nodeID"
	retObj.PoolID = "poolID"
	retObj.StorageRequestState = "CAPACITY_WAIT"
	oSR.EXPECT().Update(ctx, uaM, params.Payload).Return(retObj, nil)
	oSR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oVSR.EXPECT().Fetch(ctx, "VSR-1").Return(&models.VolumeSeriesRequest{}, nil)
	t.Log("case: update success")
	tl.Flush()
	hc.DS = mds
	assert.Nil(evM.InSSProps)
	ret = hc.storageRequestUpdate(params)
	assert.NotNil(ret)
	mO, ok := ret.(*ops.StorageRequestUpdateOK)
	assert.True(ok)
	assert.Equal(retObj, mO.Payload)
	assert.NotNil(evM.InSSProps)
	assert.Len(evM.InSSProps, 4)
	assert.EqualValues("clusterID", evM.InSSProps["clusterId"])
	assert.EqualValues("nodeID", evM.InSSProps["nodeId"])
	assert.EqualValues("CAPACITY_WAIT", evM.InSSProps["storageRequestState"])
	assert.EqualValues("poolID", evM.InSSProps["poolId"])
	assert.Equal(retObj, evM.InACScope)

	// success: StorageID cleared, NodeID modified, explicit Set of volumeSeriesRequestClaims.Claim
	mockCtrl.Finish()
	nodeObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "nodeId",
			},
		},
		NodeMutable: models.NodeMutable{
			State: com.NodeStateManaged,
			Service: &models.NuvoService{
				ServiceState: models.ServiceState{
					State: com.ServiceStateReady,
				},
			},
		},
	}
	mockCtrl = gomock.NewController(t)
	t.Log("case: StorageID is cleared")
	objM.StorageID = ""
	params.Set = []string{
		nMap.jName("StorageID"),
		nMap.jName("NodeID"),
		nMap.jName("VolumeSeriesRequestClaims") + ".claims",
	}
	params.Payload.NodeID = models.ObjIDMutable(nodeObj.Meta.ID)
	uP[centrald.UpdateSet] = params.Set
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, &params.Version, uP) })
	assert.NoError(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).Matcher()
	mds = mock.NewMockDataStore(mockCtrl)
	oN := mock.NewMockNodeOps(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN)
	oN.EXPECT().Fetch(ctx, string(params.Payload.NodeID)).Return(nodeObj, nil)
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR)
	oSR.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	oVSR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVSR)
	oVSR.EXPECT().Fetch(ctx, "VSR-1").Return(&models.VolumeSeriesRequest{}, nil)
	tl.Flush()
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageRequestUpdate(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.StorageRequestUpdateOK)
	assert.True(ok)
	assert.Equal(obj, mO.Payload)
	tl.Flush()
	objM.StorageID = "storageID" // reset

	// Update failed, cover valid transition to FAILED on inactive node
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: fail on Update")
	testutils.Clone(obj, &retObj)
	retObj.StorageRequestState = "CLOSING"
	retObj.NodeID = models.ObjIDMutable(nodeObj.Meta.ID)
	nodeObj.State = "TEAR_DOWN"
	params.Payload.StorageRequestState = "FAILED"
	params.Set = []string{"storageRequestState"}
	uP[centrald.UpdateSet] = params.Set
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, &params.Version, uP) })
	assert.NoError(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).Matcher()
	mds = mock.NewMockDataStore(mockCtrl)
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR).Times(2)
	oSR.EXPECT().Fetch(ctx, params.ID).Return(retObj, nil)
	oSR.EXPECT().Update(ctx, uaM, params.Payload).Return(nil, centrald.ErrorDbError)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageRequestUpdate(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.StorageRequestUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	nodeObj.State = "MANAGED"
	tl.Flush()

	// invalid storageRequestState
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: invalid storageRequestState")
	mds = mock.NewMockDataStore(mockCtrl)
	objM.StorageRequestState = "--Unsupported--"
	params.Set = []string{"storageRequestState"}
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageRequestUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageRequestUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	tl.Flush()

	// request terminated, cant change state
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: request terminated, cant change state")
	mds = mock.NewMockDataStore(mockCtrl)
	objM.StorageRequestState = com.StgReqStateProvisioning
	var retSRObj *models.StorageRequest
	testutils.Clone(obj, &retSRObj)
	retSRObj.StorageRequestState = com.StgReqStateSucceeded
	params.Set = []string{"storageRequestState"}
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR)
	oSR.EXPECT().Fetch(ctx, params.ID).Return(retSRObj, nil)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageRequestUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageRequestUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInvalidState.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorInvalidState.M, *mD.Payload.Message)
	tl.Flush()

	// sr fetch bad version
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: sr fetch bad version")
	mds = mock.NewMockDataStore(mockCtrl)
	objM.StorageRequestState = com.StgReqStateProvisioning
	testutils.Clone(obj, &retSRObj)
	retSRObj.Meta.Version = 2
	params.Set = []string{"storageRequestState"}
	params.Version = 1
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR)
	oSR.EXPECT().Fetch(ctx, params.ID).Return(retSRObj, nil)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageRequestUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageRequestUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorIDVerNotFound.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorIDVerNotFound.M, *mD.Payload.Message)
	tl.Flush()

	// sr fetch fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: sr fetch fails")
	mds = mock.NewMockDataStore(mockCtrl)
	objM.StorageRequestState = com.StgReqStateProvisioning
	params.Set = []string{"storageRequestState"}
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR)
	oSR.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageRequestUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageRequestUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorNotFound.M, *mD.Payload.Message)
	tl.Flush()

	// srState changed during op on failed node
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: srState changed during op on failed node")
	mds = mock.NewMockDataStore(mockCtrl)
	retSRObj.StorageRequestState = com.StgReqStateUsing
	params.Payload.StorageRequestState = com.StgReqStateClosing
	params.Version = int32(retSRObj.Meta.Version)
	params.Set = []string{"storageRequestState"}
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	mds.EXPECT().OpsStorageRequest().Return(oSR)
	oSR.EXPECT().Fetch(ctx, params.ID).Return(retSRObj, nil)
	oN = mock.NewMockNodeOps(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN)
	oN.EXPECT().Fetch(ctx, string(retSRObj.NodeID)).Return(nil, centrald.ErrorNotFound)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageRequestUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageRequestUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, *mD.Payload.Message)
	tl.Flush()

	// StorageID not found
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: StorageID not found")
	params.Set = []string{nMap.jName("StorageID")}
	mds = mock.NewMockDataStore(mockCtrl)
	oS = mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Fetch(ctx, string(objM.StorageID)).Return(nil, centrald.ErrorNotFound)
	mds.EXPECT().OpsStorage().Return(oS)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageRequestUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageRequestUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	tl.Flush()

	// StorageID fetch failed
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: StorageID fetch failed")
	params.Set = []string{nMap.jName("StorageID")}
	mds = mock.NewMockDataStore(mockCtrl)
	oS = mock.NewMockStorageOps(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS)
	oS.EXPECT().Fetch(ctx, string(objM.StorageID)).Return(nil, centrald.ErrorDbError)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageRequestUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageRequestUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()

	// NodeID not found
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: NodeID not found")
	params.Set = []string{nMap.jName("NodeID")}
	mds = mock.NewMockDataStore(mockCtrl)
	oN = mock.NewMockNodeOps(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN)
	oN.EXPECT().Fetch(ctx, string(params.Payload.NodeID)).Return(nil, centrald.ErrorNotFound)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageRequestUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageRequestUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, *mD.Payload.Message)
	tl.Flush()

	// Bad Node state
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: Bad Node state")
	params.Set = []string{nMap.jName("NodeID")}
	var nObj *models.Node
	testutils.Clone(nodeObj, &nObj)
	nObj.State = com.NodeStateTearDown
	mds = mock.NewMockDataStore(mockCtrl)
	oN = mock.NewMockNodeOps(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN)
	oN.EXPECT().Fetch(ctx, string(params.Payload.NodeID)).Return(nObj, nil)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageRequestUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageRequestUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInvalidState.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorInvalidState.M, *mD.Payload.Message)
	tl.Flush()

	// VolumeSeriesRequest fetch failed
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: VolumeSeriesRequest fetch failed")
	params.Set = []string{nMap.jName("VolumeSeriesRequestClaims")}
	mds = mock.NewMockDataStore(mockCtrl)
	oVSR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVSR)
	oVSR.EXPECT().Fetch(ctx, "VSR-1").Return(nil, centrald.ErrorNotFound)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageRequestUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageRequestUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	tl.Flush()

	// Missing volumeSeriesRequestClaims
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: StorageRequestUpdate missing volumeSeriesRequestClaims")
	objM.VolumeSeriesRequestClaims = nil
	assert.Nil(params.Payload.VolumeSeriesRequestClaims)
	params.Set = []string{
		nMap.jName("VolumeSeriesRequestClaims"),
	}
	uP[centrald.UpdateSet] = params.Set
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, &params.Version, uP) })
	assert.NoError(err)
	assert.NotNil(ua)
	mds = mock.NewMockDataStore(mockCtrl)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageRequestUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageRequestUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M+".*missing volumeSeriesRequestClaims", *mD.Payload.Message)
	tl.Flush()

	// no changes requested
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: CspDomainUpdate no change")
	mds = mock.NewMockDataStore(mockCtrl)
	params.Set = []string{}
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageRequestUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageRequestUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.storageRequestUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageRequestUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	t.Log("case: unauthorized")
	ai.AccountID, ai.TenantAccountID = "aid1", "tid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	assert.NotPanics(func() { ret = hc.storageRequestUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageRequestUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)
	ai.AccountID, ai.RoleObj = "", nil
	tl.Flush()

	// no payload
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: no payload")
	mds = mock.NewMockDataStore(mockCtrl)
	params.Set = []string{"storageId"}
	params.Payload = nil
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageRequestUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageRequestUpdateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
		assert.Equal(centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	}
	params.Payload = objM // reset
}
