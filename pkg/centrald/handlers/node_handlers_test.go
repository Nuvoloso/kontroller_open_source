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
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/node"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/pool"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/storage"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	fal "github.com/Nuvoloso/kontroller/pkg/centrald/audit/fake"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	"github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestNodeFetchFilter(t *testing.T) {
	assert := assert.New(t)
	hc := newHandlerComp()
	nObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{Meta: &models.ObjMeta{ID: "id1"}, AccountID: "tid1"},
	}
	cObj := &models.Cluster{
		ClusterAllOf0:     models.ClusterAllOf0{Meta: &models.ObjMeta{ID: "idc"}},
		ClusterCreateOnce: models.ClusterCreateOnce{AccountID: "tid1"},
		ClusterMutable: models.ClusterMutable{
			ClusterCreateMutable: models.ClusterCreateMutable{
				AuthorizedAccounts: []models.ObjIDMutable{"aid1", "aid2"},
			},
		},
	}
	rObj := &models.Role{}
	rObj.Capabilities = map[string]bool{}
	ai := &auth.Info{}

	t.Log("case: internal user")
	assert.NoError(hc.nodeFetchFilter(ai, nObj, cObj))

	t.Log("case: SystemManagementCap capability fails")
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.SystemManagementCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.nodeFetchFilter(ai, nObj, cObj))

	t.Log("case: CSPDomainManagementCap capability correct tenant")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	ai.AccountID = "tid1"
	assert.NoError(hc.nodeFetchFilter(ai, nObj, cObj))

	t.Log("case: CSPDomainManagementCap capability wrong tenant")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	ai.AccountID = "tid2"
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.nodeFetchFilter(ai, nObj, cObj))

	t.Log("case: authorized account")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	ai.AccountID = "aid2"
	assert.NoError(hc.nodeFetchFilter(ai, nObj, cObj))

	t.Log("case: not authorized account")
	ai.AccountID = "aid9"
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.nodeFetchFilter(ai, nObj, cObj))

	t.Log("case: no authorized accounts")
	cObj.AuthorizedAccounts = nil
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.nodeFetchFilter(ai, nObj, cObj))
}

func TestNodeCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	fa := &fal.AuditLog{}
	app.AuditLog = fa
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.NodeCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
		Payload:     &models.Node{},
	}
	now := time.Now()
	params.Payload.ClusterID = "clusterId"
	params.Payload.Name = "node1"
	params.Payload.Description = "a node"
	params.Payload.NodeIdentifier = "192.168.1.1"
	params.Payload.Service = &models.NuvoService{
		NuvoServiceAllOf0: models.NuvoServiceAllOf0{
			ServiceType:    "nvagentd",
			ServiceVersion: "0.1.1",
			Messages:       []*models.TimestampedString{},
		},
		ServiceState: models.ServiceState{
			HeartbeatPeriodSecs: 90,
			HeartbeatTime:       strfmt.DateTime(now),
			State:               "STARTING",
		},
	}
	params.Payload.NodeAttributes = map[string]models.ValueType{
		"attr": models.ValueType{Kind: "STRING", Value: "foo"},
	}
	params.Payload.Tags = []string{"prod"}
	ctx := params.HTTPRequest.Context()
	obj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID:      "objectID",
				Version: 1,
			},
			ClusterID: "CLUSTER-1",
		},
		NodeMutable: models.NodeMutable{
			State: "MANAGED",
		},
	}
	obj.Name = params.Payload.Name
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID(params.Payload.ClusterID),
			},
		},
	}
	clObj.AccountID = "tid1"
	clObj.Name = "bunch"
	aObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{ID: "tid1"},
		},
	}
	aObj.Name = "aName"

	// success (state not set)
	params.Payload.State = ""
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "tid1").Return(aObj, nil)
	oN := mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Create(ctx, params.Payload).Return(obj, nil)
	oCL := mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(params.Payload.ClusterID)).Return(clObj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	mds.EXPECT().OpsCluster().Return(oCL).MinTimes(1)
	t.Log("case: success, internal role")
	hc.DS = mds
	assert.Nil(evM.InSSProps)
	var ret middleware.Responder
	cntRLock := hc.cntRLock
	cntRUnlock := hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.nodeCreate(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.NodeCreateCreated)
	assert.True(ok)
	assert.EqualValues("tid1", params.Payload.AccountID)
	assert.Equal(common.NodeStateManaged, params.Payload.State)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Len(evM.InSSProps, 5)
	assert.EqualValues("CLUSTER-1", evM.InSSProps["clusterId"])
	assert.EqualValues("true", evM.InSSProps["stateChanged"])
	assert.EqualValues("MANAGED", evM.InSSProps["state"])
	assert.EqualValues("true", evM.InSSProps["serviceStateChanged"])
	assert.EqualValues("objectID", evM.InSSProps["meta.id"])
	assert.Equal(&NodeAndCluster{Node: obj, Cluster: clObj}, evM.InACScope)
	exp := &fal.Args{AI: ai, Action: centrald.ClusterUpdateAction, ObjID: clObj.Meta.ID, Name: clObj.Name, RefID: "objectID", Message: "Added node [node1]"}
	exp2 := &fal.Args{AI: ai, Action: centrald.NodeUpdateAction, ObjID: obj.Meta.ID, Name: obj.Name, Message: "Started heartbeat"}
	assert.Equal([]*fal.Args{exp, exp2}, fa.Events)
	assert.Empty(fa.Posts)
	assert.Equal("tid1", ai.AccountID)
	assert.Equal("aName", ai.AccountName)
	tl.Flush()

	// failure, state set
	params.Payload.State = common.NodeStateTimedOut
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: Create fails because object exists")
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Create(ctx, params.Payload).Return(nil, centrald.ErrorExists)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(params.Payload.ClusterID)).Return(clObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	mds.EXPECT().OpsCluster().Return(oCL).MinTimes(1)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.nodeCreate(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.NodeCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorExists.M, *mE.Payload.Message)
	assert.Equal(common.NodeStateTimedOut, params.Payload.State)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: Create fails because clusterID db error")
	oCL = mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(params.Payload.ClusterID)).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oCL).MinTimes(1)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.nodeCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.NodeCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: Create fails because clusterID not found")
	oCL = mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(params.Payload.ClusterID)).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oCL).MinTimes(1)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.nodeCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.NodeCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
	assert.Regexp("clusterId$", *mE.Payload.Message)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: account fetch fails")
	ai.AccountID = ""
	mds = mock.NewMockDataStore(mockCtrl)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(params.Payload.ClusterID)).Return(clObj, nil)
	mds.EXPECT().OpsCluster().Return(oCL)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "tid1").Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.nodeCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.NodeCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	ai.AccountID = "aid1"
	tl.Flush()

	// missing property failure cases, invalid state
	tcs := []string{"name", "clusterId", "state"}
	for _, tc := range tcs {
		t.Log("case: missing", tc)
		cl := *params.Payload // copy
		switch tc {
		case "name":
			cl.Name = ""
		case "clusterId":
			cl.ClusterID = ""
		case "state":
			cl.State = common.NodeStateManaged + "foo"
		default:
			assert.False(true)
		}
		cntRLock = hc.cntRLock
		cntRUnlock = hc.cntRUnlock
		assert.NotPanics(func() {
			ret = hc.nodeCreate(ops.NodeCreateParams{HTTPRequest: requestWithAuthContext(ai), Payload: &cl})
		})
		assert.NotNil(ret)
		_, ok = ret.(*ops.NodeCreateCreated)
		assert.False(ok)
		mE, ok := ret.(*ops.NodeCreateDefault)
		assert.True(ok)
		assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
		assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
		assert.Equal(cntRLock, hc.cntRLock)
		assert.Equal(cntRUnlock, hc.cntRUnlock)
		tl.Flush()
	}

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: invalid node attrs, valid tenant admin")
	ai.AccountID = "tid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	oCL = mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(params.Payload.ClusterID)).Return(clObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsCluster().Return(oCL).MinTimes(1)
	params.Payload.NodeAttributes = map[string]models.ValueType{
		"attr": models.ValueType{Kind: "INT", Value: "foo"},
	}
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	ret = hc.nodeCreate(params)
	assert.NotNil(ret)
	_, ok = ret.(*ops.NodeCreateCreated)
	assert.False(ok)
	mE, ok = ret.(*ops.NodeCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
	assert.Regexp("nodeAttributes", *mE.Payload.Message)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	tl.Flush()

	t.Log("case: auditLog not ready")
	fa.ReadyRet = centrald.ErrorDbError
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.nodeCreate(params) })
	mE, ok = ret.(*ops.NodeCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.nodeCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.NodeCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized tenant admin")
	fa.Events = []*fal.Args{}
	mds = mock.NewMockDataStore(mockCtrl)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(params.Payload.ClusterID)).Return(clObj, nil)
	mds.EXPECT().OpsCluster().Return(oCL)
	hc.DS = mds
	ai.AccountID = "otherID"
	assert.NotPanics(func() { ret = hc.nodeCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.NodeCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	exp = &fal.Args{AI: ai, Action: centrald.NodeCreateAction, Name: params.Payload.Name, Err: true, Message: "Create unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	assert.Empty(fa.Events)
	ai.RoleObj = nil
	tl.Flush()

	// no payload
	params.Payload = nil
	t.Log("case: nil payload")
	assert.NotPanics(func() { ret = hc.nodeCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.NodeCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorMissing.M, *mE.Payload.Message)
}

func TestNodeDelete(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	fa := &fal.AuditLog{}
	app.AuditLog = fa
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.NodeDeleteParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "objectID",
			},
			AccountID: "tid1",
			ClusterID: "CLUSTER-1",
		},
	}
	obj.Name = "node1"
	obj.Service = &models.NuvoService{}
	obj.Service.State = "UNKNOWN"
	obj.State = "TEAR_DOWN"
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "CLUSTER-1",
			},
		},
	}
	clObj.AccountID = "tid1"
	clObj.Name = "bunch"
	srlParams := storage_request.StorageRequestListParams{NodeID: &params.ID, IsTerminated: swag.Bool(false)}
	saParams := storage.StorageListParams{AttachedNodeID: &params.ID}
	spParams := pool.PoolListParams{AccessibilityScope: swag.String("NODE"), AccessibilityScopeObjID: &params.ID}
	vsParams := volume_series.VolumeSeriesListParams{ConfiguredNodeID: &params.ID}
	vrlParams := volume_series_request.VolumeSeriesRequestListParams{NodeID: &params.ID, IsTerminated: swag.Bool(false), RequestedOperationsNot: []string{common.VolReqOpNodeDelete}}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	t.Log("case: NodeDelete success")
	oN := mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Delete(ctx, params.ID).Return(nil)
	oN.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCL := mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(clObj.Meta.ID)).Return(clObj, nil)
	oSR := mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().Count(ctx, srlParams, uint(1)).Return(0, nil)
	oV := mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Count(ctx, vsParams, uint(1)).Return(0, nil)
	oS := mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Count(ctx, saParams, uint(1)).Return(0, nil)
	oSP := mock.NewMockPoolOps(mockCtrl)
	oSP.EXPECT().Count(ctx, spParams, uint(1)).Return(0, nil)
	oVR := mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Count(ctx, vrlParams, uint(1)).Return(0, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	mds.EXPECT().OpsCluster().Return(oCL)
	mds.EXPECT().OpsStorageRequest().Return(oSR)
	mds.EXPECT().OpsStorage().Return(oS)
	mds.EXPECT().OpsPool().Return(oSP)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	hc.DS = mds
	assert.Nil(evM.InSSProps)
	var ret middleware.Responder
	cntLock := hc.cntLock
	cntUnlock := hc.cntUnlock
	assert.NotPanics(func() { ret = hc.nodeDelete(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.NodeDeleteNoContent)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Len(evM.InSSProps, 5)
	assert.EqualValues("CLUSTER-1", evM.InSSProps["clusterId"])
	assert.EqualValues("true", evM.InSSProps["stateChanged"])
	assert.EqualValues("true", evM.InSSProps["serviceStateChanged"])
	assert.EqualValues("TEAR_DOWN", evM.InSSProps["state"])
	assert.EqualValues("UNKNOWN", evM.InSSProps["serviceState"])
	assert.Equal(&NodeAndCluster{Node: obj, Cluster: clObj}, evM.InACScope)
	exp := &fal.Args{AI: ai, Action: centrald.ClusterUpdateAction, ObjID: clObj.Meta.ID, Name: clObj.Name, RefID: models.ObjIDMutable(params.ID), Message: "Deleted node [node1]"}
	assert.Equal([]*fal.Args{exp}, fa.Events)
	assert.Empty(fa.Posts)
	tl.Flush()

	// delete failure case, cover valid tenant auth
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: delete failure")
	ai.AccountID = "tid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Delete(ctx, params.ID).Return(centrald.ErrorDbError)
	oN.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(clObj.Meta.ID)).Return(clObj, nil)
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().Count(ctx, srlParams, uint(1)).Return(0, nil)
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Count(ctx, vsParams, uint(1)).Return(0, nil)
	oS = mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Count(ctx, saParams, uint(1)).Return(0, nil)
	oSP = mock.NewMockPoolOps(mockCtrl)
	oSP.EXPECT().Count(ctx, spParams, uint(1)).Return(0, nil)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Count(ctx, vrlParams, uint(1)).Return(0, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	mds.EXPECT().OpsCluster().Return(oCL)
	mds.EXPECT().OpsStorageRequest().Return(oSR)
	mds.EXPECT().OpsStorage().Return(oS)
	mds.EXPECT().OpsPool().Return(oSP)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.nodeDelete(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.NodeDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: cluster fetch db error")
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(clObj.Meta.ID)).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	mds.EXPECT().OpsCluster().Return(oCL)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.nodeDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.NodeDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: auditLog not ready")
	mds = mock.NewMockDataStore(mockCtrl)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsNode().Return(oN)
	fa.ReadyRet = centrald.ErrorDbError
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.nodeDelete(params) })
	mE, ok = ret.(*ops.NodeDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.nodeDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.NodeDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized tenant admin")
	fa.Events = []*fal.Args{}
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(clObj.Meta.ID)).Return(clObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	mds.EXPECT().OpsCluster().Return(oCL)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.nodeDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.NodeDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Regexp(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	exp = &fal.Args{AI: ai, Action: centrald.NodeDeleteAction, ObjID: models.ObjID(params.ID), Name: obj.Name, Err: true, Message: "Delete unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	assert.Empty(fa.Events)
	ai.RoleObj = nil
	tl.Flush()

	// cascading validation failure cases
	tObj := []string{"storage request", "volume series", "storage", "pool", "volume series request"}
	for tc := 0; tc < len(tObj)*2; tc++ {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		t.Log("case: " + tObj[tc/2] + []string{" count fails", " exists"}[tc%2])
		mds = mock.NewMockDataStore(mockCtrl)
		oN = mock.NewMockNodeOps(mockCtrl)
		oN.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
		oCL = mock.NewMockClusterOps(mockCtrl)
		oCL.EXPECT().Fetch(ctx, string(clObj.Meta.ID)).Return(clObj, nil)
		mds.EXPECT().OpsNode().Return(oN)
		mds.EXPECT().OpsCluster().Return(oCL)
		count, err := tc%2, []error{centrald.ErrorDbError, nil}[tc%2]
		switch tc / 2 {
		case 4:
			oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			oVR.EXPECT().Count(ctx, vrlParams, uint(1)).Return(count, err)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			count, err = 0, nil
			fallthrough
		case 3:
			oSP = mock.NewMockPoolOps(mockCtrl)
			oSP.EXPECT().Count(ctx, spParams, uint(1)).Return(count, err)
			mds.EXPECT().OpsPool().Return(oSP)
			count, err = 0, nil
			fallthrough
		case 2:
			oS = mock.NewMockStorageOps(mockCtrl)
			oS.EXPECT().Count(ctx, saParams, uint(1)).Return(count, err)
			mds.EXPECT().OpsStorage().Return(oS)
			count, err = 0, nil
			fallthrough
		case 1:
			oV = mock.NewMockVolumeSeriesOps(mockCtrl)
			oV.EXPECT().Count(ctx, vsParams, uint(1)).Return(count, err)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			count, err = 0, nil
			fallthrough
		case 0:
			oSR = mock.NewMockStorageRequestOps(mockCtrl)
			oSR.EXPECT().Count(ctx, srlParams, uint(1)).Return(count, err)
			mds.EXPECT().OpsStorageRequest().Return(oSR)
		default:
			assert.True(false)
		}
		hc.DS = mds
		cntLock = hc.cntLock
		cntUnlock = hc.cntUnlock
		assert.NotPanics(func() { ret = hc.nodeDelete(params) })
		assert.NotNil(ret)
		mE, ok = ret.(*ops.NodeDeleteDefault)
		if assert.True(ok) {
			if tc%2 == 0 {
				assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
				assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
			} else {
				assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
				assert.Regexp(tObj[tc/2]+".* the node", *mE.Payload.Message)
			}
		}
		assert.Equal(cntLock+1, hc.cntLock)
		assert.Equal(cntUnlock+1, hc.cntUnlock)
		tl.Flush()
	}

	// fetch failure case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	t.Log("case: NodeDelete fetch failure")
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.nodeDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.NodeDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
	assert.Equal(cntLock, hc.cntLock)
	assert.Equal(cntUnlock, hc.cntUnlock)
}

func TestNodeFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.NodeFetchParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "objectID",
			},
			AccountID: "tid1",
			ClusterID: "CLUSTER-1",
		},
	}
	obj.Name = "node"
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "CLUSTER-1",
			},
		},
	}
	clObj.AccountID = "tid1"

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oN := mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCL := mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(clObj.Meta.ID)).Return(clObj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	mds.EXPECT().OpsCluster().Return(oCL)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.nodeFetch(params) })
	assert.NotNil(ret)
	mR, ok := ret.(*ops.NodeFetchOK)
	assert.True(ok)
	assert.Equal(obj, mR.Payload)
	tl.Flush()

	// fetch failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	t.Log("case: node fetch failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.nodeFetch(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.NodeFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)

	// cluster fetch db error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: fetch cluster db error")
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(clObj.Meta.ID)).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	mds.EXPECT().OpsCluster().Return(oCL)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.nodeFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.NodeFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.nodeFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.NodeFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	// unauthorized
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: unauthorized")
	ai.AccountID = "other"
	ai.RoleObj = &models.Role{}
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(clObj.Meta.ID)).Return(clObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	mds.EXPECT().OpsCluster().Return(oCL)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.nodeFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.NodeFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
}

func TestNodeList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.NodeListParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	ctx := params.HTTPRequest.Context()
	objects := []*models.Node{
		&models.Node{
			NodeAllOf0: models.NodeAllOf0{
				Meta: &models.ObjMeta{
					ID: "objectID1",
				},
				AccountID: "tid1",
				ClusterID: "cluster1",
			},
			NodeMutable: models.NodeMutable{
				Name: "name1",
			},
		},
		&models.Node{
			NodeAllOf0: models.NodeAllOf0{
				Meta: &models.ObjMeta{
					ID: "objectID2",
				},
				AccountID: "tid1",
				ClusterID: "cluster1",
			},
			NodeMutable: models.NodeMutable{
				Name: "name2",
			},
		},
		&models.Node{
			NodeAllOf0: models.NodeAllOf0{
				Meta: &models.ObjMeta{
					ID: "objectID3",
				},
				AccountID: "tid2",
				ClusterID: "cluster2",
			},
			NodeMutable: models.NodeMutable{
				Name: "name3",
			},
		},
	}
	clObj1 := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{Meta: &models.ObjMeta{ID: "cluster1"}},
	}
	clObj1.AccountID = "tid1"
	clObj1.AuthorizedAccounts = []models.ObjIDMutable{"aid1", "aid2"}
	clObj2 := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{Meta: &models.ObjMeta{ID: "cluster2"}},
	}
	clObj2.AccountID = "tid2"

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oN := mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().List(ctx, params).Return(objects, nil)
	oCL := mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(clObj1.Meta.ID)).Return(clObj1, nil)
	oCL.EXPECT().Fetch(ctx, string(clObj2.Meta.ID)).Return(clObj2, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	mds.EXPECT().OpsCluster().Return(oCL).MinTimes(1)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.nodeList(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.NodeListOK)
	assert.True(ok)
	assert.Equal(objects, mO.Payload)
	tl.Flush()

	// list fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: List failure")
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().List(ctx, params).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.nodeList(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.NodeListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().List(ctx, params).Return(objects, nil)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(clObj1.Meta.ID)).Return(clObj1, nil)
	oCL.EXPECT().Fetch(ctx, string(clObj2.Meta.ID)).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN)
	mds.EXPECT().OpsCluster().Return(oCL).MinTimes(1)
	t.Log("case: cluster fetch failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.nodeList(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.NodeListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.nodeList(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.NodeListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	t.Log("case: unauthorized filtered")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	ai.AccountID = "aid2"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().List(ctx, params).Return(objects, nil)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(clObj1.Meta.ID)).Return(clObj1, nil)
	oCL.EXPECT().Fetch(ctx, string(clObj2.Meta.ID)).Return(clObj2, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN)
	mds.EXPECT().OpsCluster().Return(oCL).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.nodeList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.NodeListOK)
	assert.True(ok)
	if assert.Len(mO.Payload, 2) {
		assert.EqualValues("objectID1", mO.Payload[0].Meta.ID)
		assert.EqualValues("objectID2", mO.Payload[1].Meta.ID)
	}
}

func TestNodeUpdate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	fa := &fal.AuditLog{}
	app.AuditLog = fa
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app

	// make the name map
	var nMap JSONToAttrNameMap
	assert.NotPanics(func() { nMap = hc.nodeMutableNameMap() })
	// validate some embedded properties
	assert.Equal("name", nMap.jName("Name"))
	assert.Equal("service", nMap.jName("Service"))

	// parse params
	objM := &models.NodeMutable{
		AvailableCacheBytes: swag.Int64(2048),
		Name:                "node",
		Service: &models.NuvoService{
			ServiceState: models.ServiceState{
				State: "READY",
			},
		},
		State: "MANAGED",
	}
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{Meta: &models.ObjMeta{ID: "CLUSTER-1"}},
	}
	clObj.AccountID = "tid1"
	ai := &auth.Info{}
	params := ops.NodeUpdateParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
		Version:     swag.Int32(8),
		Remove:      []string{},
		Append:      []string{},
		Set:         []string{nMap.jName("AvailableCacheBytes"), nMap.jName("Name"), nMap.jName("State"), nMap.jName("Service")},
		Payload:     objM,
	}
	ctx := params.HTTPRequest.Context()
	var ua *centrald.UpdateArgs
	var err error
	var uP = [centrald.NumActionTypes][]string{
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    params.Set,
	}
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM := updateArgsMatcher(t, ua).Matcher()

	// success with state change
	obj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID:      models.ObjID(params.ID),
				Version: 8,
			},
			AccountID: "tid1",
			ClusterID: "CLUSTER-1",
		},
		NodeMutable: models.NodeMutable{
			Name: "nodeName",
			Service: &models.NuvoService{
				ServiceState: models.ServiceState{
					State: "UNKNOWN",
				},
			},
			State:           "TIMED_OUT",
			TotalCacheBytes: swag.Int64(4096),
		},
	}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oN := mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oN.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	oCL := mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(clObj.Meta.ID)).Return(clObj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	mds.EXPECT().OpsCluster().Return(oCL)
	t.Log("case: success with state change")
	hc.DS = mds
	cntRLock := hc.cntRLock
	cntRUnlock := hc.cntRUnlock
	assert.Nil(evM.InSSProps)
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.nodeUpdate(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.NodeUpdateOK)
	assert.True(ok)
	assert.Equal(obj, mO.Payload)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Len(evM.InSSProps, 5)
	assert.EqualValues("CLUSTER-1", evM.InSSProps["clusterId"])
	assert.EqualValues("true", evM.InSSProps["stateChanged"])
	assert.EqualValues("true", evM.InSSProps["serviceStateChanged"])
	assert.EqualValues("TIMED_OUT", evM.InSSProps["state"])
	assert.EqualValues("UNKNOWN", evM.InSSProps["serviceState"])
	assert.Equal(&NodeAndCluster{Node: obj, Cluster: clObj}, evM.InACScope)
	assert.Empty(fa.Posts)
	assert.Empty(fa.Events)
	tl.Flush()

	retObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID:      models.ObjID(params.ID),
				Version: 8,
			},
			AccountID: "tid1",
			ClusterID: "CLUSTER-1",
		},
		NodeMutable: models.NodeMutable{
			Name: "nodeName",
			Service: &models.NuvoService{
				ServiceState: models.ServiceState{
					State: "READY",
				},
			},
			TotalCacheBytes: swag.Int64(4096),
		},
	}
	// success with state change to UNKNOWN (node stopped)
	mockCtrl.Finish()
	t.Log("case: success with state change to UNKNOWN")
	params.Payload.Service.State = "UNKNOWN"
	mockCtrl = gomock.NewController(t)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, params.ID).Return(retObj, nil)
	oN.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(clObj.Meta.ID)).Return(clObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	mds.EXPECT().OpsCluster().Return(oCL)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	evM.InSSProps = nil
	assert.NotPanics(func() { ret = hc.nodeUpdate(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.NodeUpdateOK)
	assert.True(ok)
	assert.Equal(obj, mO.Payload)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Len(evM.InSSProps, 5)
	assert.EqualValues("CLUSTER-1", evM.InSSProps["clusterId"])
	assert.Equal(&NodeAndCluster{Node: obj, Cluster: clObj}, evM.InACScope)
	exp := &fal.Args{AI: ai, Action: centrald.NodeUpdateAction, ObjID: obj.Meta.ID, Name: obj.Name, Message: "Stopped heartbeat"}
	assert.Equal([]*fal.Args{exp}, fa.Events)
	assert.Empty(fa.Posts)
	tl.Flush()

	// success with state change from UNKNOWN (node restarted)
	mockCtrl.Finish()
	t.Log("case: success with state change from UNKNOWN")
	mockCtrl = gomock.NewController(t)
	fa.Events = []*fal.Args{}
	retObj.Service.State = "UNKNOWN"
	obj.Service.State = "STARTING"
	params.Payload.Service.State = "STARTING"
	obj.State = common.NodeStateTimedOut
	params.Payload.State = common.NodeStateManaged
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, params.ID).Return(retObj, nil)
	oN.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(clObj.Meta.ID)).Return(clObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	mds.EXPECT().OpsCluster().Return(oCL)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	evM.InSSProps = nil
	assert.NotPanics(func() { ret = hc.nodeUpdate(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.NodeUpdateOK)
	assert.True(ok)
	assert.Equal(obj, mO.Payload)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Len(evM.InSSProps, 5)
	assert.EqualValues("CLUSTER-1", evM.InSSProps["clusterId"])
	assert.Equal(&NodeAndCluster{Node: obj, Cluster: clObj}, evM.InACScope)
	exp.Message = "Started heartbeat"
	assert.Equal([]*fal.Args{exp}, fa.Events)
	assert.Empty(fa.Posts)
	obj.Service.State = "UNKNOWN" // restore
	tl.Flush()

	// success with state change, no previous state
	mockCtrl.Finish()
	t.Log("case: success no previous state")
	obj.Service = nil
	mockCtrl = gomock.NewController(t)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oN.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(clObj.Meta.ID)).Return(clObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	mds.EXPECT().OpsCluster().Return(oCL)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	evM.InSSProps = nil
	assert.NotPanics(func() { ret = hc.nodeUpdate(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.NodeUpdateOK)
	assert.True(ok)
	assert.Equal(obj, mO.Payload)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Len(evM.InSSProps, 4)
	assert.EqualValues("CLUSTER-1", evM.InSSProps["clusterId"])
	assert.EqualValues("true", evM.InSSProps["serviceStateChanged"])
	assert.Equal(&NodeAndCluster{Node: obj, Cluster: clObj}, evM.InACScope)
	tl.Flush()

	mockCtrl.Finish()
	t.Log("case: success with TotalCacheBytes, CacheUnitSizeBytes")
	mockCtrl = gomock.NewController(t)
	params.Payload.TotalCacheBytes = swag.Int64(5120)
	params.Payload.CacheUnitSizeBytes = swag.Int64(4096)
	params.Set = []string{nMap.jName("TotalCacheBytes"), nMap.jName("CacheUnitSizeBytes")}
	uP = [centrald.NumActionTypes][]string{
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    append(params.Set, "availableCacheBytes"),
	}
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).Matcher()
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oN.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(clObj.Meta.ID)).Return(clObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	mds.EXPECT().OpsCluster().Return(oCL)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	evM.InSSProps = nil
	assert.NotPanics(func() { ret = hc.nodeUpdate(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.NodeUpdateOK)
	assert.True(ok)
	assert.EqualValues(1024, swag.Int64Value(params.Payload.AvailableCacheBytes))
	assert.Equal(obj, mO.Payload)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Len(evM.InSSProps, 4)
	assert.EqualValues("CLUSTER-1", evM.InSSProps["clusterId"])
	assert.EqualValues("false", evM.InSSProps["serviceStateChanged"])
	assert.Equal(&NodeAndCluster{Node: obj, Cluster: clObj}, evM.InACScope)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: wrong version")
	params.Version = swag.Int32(7)
	mds = mock.NewMockDataStore(mockCtrl)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsNode().Return(oN)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.nodeUpdate(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.NodeUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorIDVerNotFound.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorIDVerNotFound.M, *mD.Payload.Message)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	tl.Flush()

	// success with no state change, valid tenant auth, no version
	mockCtrl.Finish()
	t.Log("case: success without state change")
	mockCtrl = gomock.NewController(t)
	params.Set = []string{nMap.jName("Name")}
	params.Version = nil
	uP = [centrald.NumActionTypes][]string{
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    params.Set,
	}
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).AddsVersion(8).Matcher()
	ai.AccountID = "tid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oN.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(clObj.Meta.ID)).Return(clObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	mds.EXPECT().OpsCluster().Return(oCL)
	hc.DS = mds
	evM.InSSProps = nil
	assert.NotPanics(func() { ret = hc.nodeUpdate(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.NodeUpdateOK)
	assert.True(ok)
	assert.Equal(obj, mO.Payload)
	assert.Len(evM.InSSProps, 4)
	assert.EqualValues("CLUSTER-1", evM.InSSProps["clusterId"])
	assert.EqualValues("false", evM.InSSProps["serviceStateChanged"])
	assert.Equal(&NodeAndCluster{Node: obj, Cluster: clObj}, evM.InACScope)
	tl.Flush()

	// Update failed
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: Update failed")
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oN.EXPECT().Update(ctx, uaM, params.Payload).Return(nil, centrald.ErrorDbError)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(clObj.Meta.ID)).Return(clObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	mds.EXPECT().OpsCluster().Return(oCL)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.nodeUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.NodeUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: fetch fails")
	mds = mock.NewMockDataStore(mockCtrl)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.nodeUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.NodeUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mD.Payload.Message)
	tl.Flush()

	// cluster fetch db error, authorized nodeAttributes update, totalCacheBytes resets availableCacheBytes
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: cluster fetch db error")
	ai.RoleObj = nil
	params.Append = []string{nMap.jName("NodeAttributes")}
	params.Set = []string{nMap.jName("TotalCacheBytes")}
	params.Payload.AvailableCacheBytes = swag.Int64(2048)
	params.Payload.TotalCacheBytes = swag.Int64(1024)
	obj.AvailableCacheBytes = swag.Int64(1024)
	obj.TotalCacheBytes = swag.Int64(4096)
	mds = mock.NewMockDataStore(mockCtrl)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsNode().Return(oN)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oCL.EXPECT().Fetch(ctx, string(clObj.Meta.ID)).Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsCluster().Return(oCL)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.nodeUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.NodeUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	assert.Zero(swag.Int64Value(params.Payload.AvailableCacheBytes))
	params.Append = []string{}
	tl.Flush()

	// auth failures
	ai.AccountID = "tid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	aiN := &auth.Info{
		AccountID: "aid1",
		RoleObj:   &models.Role{},
	}
	aiN.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	for k := range nMap {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		t.Log("case: auth failure", k)
		params.Set = []string{k}
		fa.Posts = []*fal.Args{}
		mds = mock.NewMockDataStore(mockCtrl)
		if k == "service" || k == "state" || k == "nodeAttributes" || k == "availableCacheBytes" || k == "localStorage" || k == "totalCacheBytes" {
			params.HTTPRequest = requestWithAuthContext(ai)
		} else {
			params.HTTPRequest = requestWithAuthContext(aiN)
		}
		ctx = params.HTTPRequest.Context()
		oN = mock.NewMockNodeOps(mockCtrl)
		oN.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
		mds.EXPECT().OpsNode().Return(oN)
		hc.DS = mds
		assert.NotPanics(func() { ret = hc.nodeUpdate(params) })
		assert.NotNil(ret)
		mD, ok = ret.(*ops.NodeUpdateDefault)
		assert.True(ok)
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)
		exp = &fal.Args{AI: ai, Action: centrald.NodeUpdateAction, ObjID: models.ObjID(params.ID), Name: obj.Name, Err: true, Message: "Update unauthorized"}
		if k != "service" && k != "state" && k != "nodeAttributes" && k != "availableCacheBytes" && k != "localStorage" && k != "totalCacheBytes" {
			exp.AI = aiN
		}
		assert.Equal([]*fal.Args{exp}, fa.Posts)
	}
	ai.RoleObj = nil

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.nodeUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.NodeUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: auditLog not ready")
	mds = mock.NewMockDataStore(mockCtrl)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsNode().Return(oN)
	fa.ReadyRet = centrald.ErrorDbError
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.nodeUpdate(params) })
	mD, ok = ret.(*ops.NodeUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	// no changes requested
	params.Set = []string{}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	t.Log("case: no change")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.nodeUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.NodeUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)

	// missing invalid updates
	tcs := []string{"name", "cache", "availableCacheBytes", "cacheUnitSizeBytes", "state", "invalid state transition"}
	for _, tc := range tcs {
		cl := *params.Payload // copy
		items := []string{tc}
		expErr := centrald.ErrorUpdateInvalidRequest
		expPat := ".*"
		switch tc {
		case "availableCacheBytes":
			cl.AvailableCacheBytes = swag.Int64(1024 * 1024)
			oN.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			mds.EXPECT().OpsNode().Return(oN)
		case "cache":
			items = []string{"availableCacheBytes", "totalCacheBytes"}
		case "cacheUnitSizeBytes":
			cl.CacheUnitSizeBytes = swag.Int64(8192)
			oN.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			mds.EXPECT().OpsNode().Return(oN)
		case "name":
			cl.Name = ""
		case "state":
			expPat = "invalid node state"
			cl.State = ""
			oN.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			mds.EXPECT().OpsNode().Return(oN)
		case "invalid state transition":
			items = []string{"state"}
			expErr = centrald.ErrorInvalidState
			expPat = "invalid node state transition"
			cl.State = common.NodeStateManaged
			var prevStateObj *models.Node
			testutils.Clone(obj, &prevStateObj)
			prevStateObj.State = common.NodeStateTearDown
			oN.EXPECT().Fetch(ctx, params.ID).Return(prevStateObj, nil)
			mds.EXPECT().OpsNode().Return(oN)
		default:
			assert.False(true)
		}
		t.Log("case: error case: " + tc)
		assert.NotPanics(func() {
			ret = hc.nodeUpdate(ops.NodeUpdateParams{HTTPRequest: requestWithAuthContext(ai), ID: params.ID, Payload: &cl, Set: items})
		})
		assert.NotNil(ret)
		mD, ok = ret.(*ops.NodeUpdateDefault)
		assert.True(ok)
		assert.Equal(expErr.C, int(mD.Payload.Code))
		assert.Regexp("^"+expErr.M, *mD.Payload.Message)
		assert.Regexp(expPat, *mD.Payload.Message)
		tl.Flush()
	}
	tl.Flush()

	// no payload
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: no payload")
	mds = mock.NewMockDataStore(mockCtrl)
	params.Set = []string{"name"}
	params.Payload = nil
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.nodeUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.NodeUpdateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
		assert.Equal(centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	}
}
