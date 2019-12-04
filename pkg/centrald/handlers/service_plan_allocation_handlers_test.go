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
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/pool"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/service_plan_allocation"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	fal "github.com/Nuvoloso/kontroller/pkg/centrald/audit/fake"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	mockcluster "github.com/Nuvoloso/kontroller/pkg/cluster/mock"
	"github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestServicePlanAllocationValidateSizes(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ctx := context.Background()

	oObj := &models.ServicePlanAllocation{
		ServicePlanAllocationAllOf0: models.ServicePlanAllocationAllOf0{
			Meta: &models.ObjMeta{ID: "objId"},
		},
	}
	vsListParams := volume_series.VolumeSeriesListParams{
		ServicePlanAllocationID: swag.String("objId"),
		VolumeSeriesStateNot:    []string{"DELETING"},
		Sum:                     []string{"sizeBytes"}}
	vsAggRet := []*centrald.Aggregation{
		&centrald.Aggregation{FieldPath: "sizeBytes", Type: "sum", Value: 50},
	}

	// reservableCapacityByte cases
	uObj := &models.ServicePlanAllocationMutable{}
	uObj.ReservableCapacityBytes = swag.Int64(100)
	uObj.TotalCapacityBytes = swag.Int64(200)
	ua := &centrald.UpdateArgs{
		Attributes: []centrald.UpdateAttr{
			{
				Name: "ReservableCapacityBytes",
				Actions: [centrald.NumActionTypes]centrald.UpdateActionArgs{
					centrald.UpdateSet: centrald.UpdateActionArgs{FromBody: true},
				},
			},
			{
				Name: "TotalCapacityBytes",
				Actions: [centrald.NumActionTypes]centrald.UpdateActionArgs{
					centrald.UpdateSet: centrald.UpdateActionArgs{FromBody: true},
				},
			},
		},
	}

	// case: no-update success
	t.Log("case: no-update success")
	assert.NoError(hc.servicePlanAllocationValidateSizes(ctx, nil, oObj, nil))
	tl.Flush()

	// case: update success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	t.Log("case: update success")
	oV := mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Aggregate(ctx, vsListParams).Return(vsAggRet, 1, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	hc.DS = mds
	assert.NoError(hc.servicePlanAllocationValidateSizes(ctx, ua, oObj, uObj))
	tl.Flush()

	// case: update volume series aggregate db error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: volume series aggregate db error")
	oV.EXPECT().Aggregate(ctx, vsListParams).Return(nil, 0, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	hc.DS = mds
	err := hc.servicePlanAllocationValidateSizes(ctx, ua, oObj, uObj)
	assert.Equal(centrald.ErrorDbError, err)
	tl.Flush()

	// case: failure negative reservable capacity
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: autoupdate causes negative reservableCapacityBytes")
	uObj.ReservableCapacityBytes = swag.Int64(-1)
	uObj.TotalCapacityBytes = swag.Int64(200)
	err = hc.servicePlanAllocationValidateSizes(ctx, ua, oObj, uObj)
	assert.Error(err)
	assert.Regexp("change to totalCapacityBytes would cause reservableCapacityBytes to be negative", err.Error())
	tl.Flush()

	// case: reservableCapacityBytes + reservedBytes > totalCapacityBytes
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: update success")
	uObj.ReservableCapacityBytes = swag.Int64(175)
	uObj.TotalCapacityBytes = swag.Int64(200)
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Aggregate(ctx, vsListParams).Return(vsAggRet, 1, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	hc.DS = mds
	err = hc.servicePlanAllocationValidateSizes(ctx, ua, oObj, uObj)
	assert.Error(err)
	assert.Regexp("^reservableCapacityBytes plus", err.Error())
	tl.Flush()

	// case: reservableCapacityBytes > totalCapacityBytes
	t.Log("case: update reservableCapacityBytes > totalCapacityBytes")
	ua.Attributes[0].Name = "ReservableCapacityBytes"
	uObj.ReservableCapacityBytes = swag.Int64(201)
	err = hc.servicePlanAllocationValidateSizes(ctx, ua, oObj, uObj)
	assert.Error(err)
	assert.Regexp("^reservableCapacityBytes.*greater", err.Error())
	tl.Flush()
}

func TestServicePlanAllocationCreateValidate(t *testing.T) {
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
	ctx := context.Background()

	sfl := app.SupportedStorageFormulas()
	sfN := sfl[0].Name
	obj := &models.ServicePlanAllocation{}
	obj.AccountID = "accountId"
	obj.AuthorizedAccountID = "authorizedAccountId"
	obj.ClusterID = "clusterId"
	obj.ServicePlanID = "servicePlanId"
	obj.ReservationState = "UNKNOWN"
	obj.ReservableCapacityBytes = swag.Int64(10)
	obj.TotalCapacityBytes = swag.Int64(10)
	obj.StorageReservations = map[string]models.StorageTypeReservation{
		"poolID": models.StorageTypeReservation{
			NumMirrors: 1,
			SizeBytes:  swag.Int64(1),
		},
	}
	obj.StorageFormula = models.StorageFormulaName(sfN)

	aObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID(obj.AccountID),
			},
		},
		AccountMutable: models.AccountMutable{
			Name: "AccountName",
		},
	}
	var aaObj *models.Account
	testutils.Clone(aObj, &aaObj)
	aaObj.Meta.ID = models.ObjID(obj.AuthorizedAccountID)
	aaObj.TenantAccountID = obj.AccountID
	aaObj.Name = "AuthorizedAccountName"

	cObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID(obj.ClusterID),
			},
		},
		ClusterCreateOnce: models.ClusterCreateOnce{
			AccountID:   models.ObjID(obj.AccountID),
			CspDomainID: models.ObjIDMutable("cspDomainId"),
		},
		ClusterMutable: models.ClusterMutable{
			ClusterCreateMutable: models.ClusterCreateMutable{
				State: common.ClusterStateManaged,
			},
		},
	}
	spObj := &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID(obj.ServicePlanID),
			},
		},
	}
	poolObj := &models.Pool{
		PoolAllOf0: models.PoolAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("poolID"),
			},
		},
		PoolCreateOnce: models.PoolCreateOnce{
			AccountID: obj.AccountID,
		},
	}
	ai := &auth.Info{
		AccountID: string(obj.AccountID),
		RoleObj:   &models.Role{},
	}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(obj.AccountID)).Return(aObj, nil)
	oA.EXPECT().Fetch(ctx, string(obj.AuthorizedAccountID)).Return(aaObj, nil)
	oC := mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, string(obj.ClusterID)).Return(cObj, nil)
	oSP := mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Fetch(ctx, string(obj.ServicePlanID)).Return(spObj, nil)
	oP := mock.NewMockPoolOps(mockCtrl)
	oP.EXPECT().Fetch(ctx, "poolID").Return(poolObj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(2)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	mds.EXPECT().OpsServicePlan().Return(oSP).MinTimes(1)
	mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
	hc.DS = mds
	var ret error
	auAcObj, spRObj, clObj, ret := hc.servicePlanAllocationCreateValidate(ctx, ai, obj)
	assert.Equal(aaObj, auAcObj)
	assert.Equal(spObj, spRObj)
	assert.Equal(cObj, clObj)
	assert.Nil(ret)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure poolFetch")
	mockCtrl = gomock.NewController(t)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(obj.AccountID)).Return(aObj, nil)
	oA.EXPECT().Fetch(ctx, string(obj.AuthorizedAccountID)).Return(aaObj, nil)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, string(obj.ClusterID)).Return(cObj, nil)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Fetch(ctx, string(obj.ServicePlanID)).Return(spObj, nil)
	oP = mock.NewMockPoolOps(mockCtrl)
	oP.EXPECT().Fetch(ctx, "poolID").Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(2)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	mds.EXPECT().OpsServicePlan().Return(oSP).MinTimes(1)
	mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
	hc.DS = mds
	_, _, _, ret = hc.servicePlanAllocationCreateValidate(ctx, ai, obj)
	assert.NotNil(ret)
	assert.Regexp("invalid poolId", ret)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure servicePlanFetch")
	mockCtrl = gomock.NewController(t)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(obj.AccountID)).Return(aObj, nil)
	oA.EXPECT().Fetch(ctx, string(obj.AuthorizedAccountID)).Return(aaObj, nil)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, string(obj.ClusterID)).Return(cObj, nil)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Fetch(ctx, string(obj.ServicePlanID)).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(2)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	mds.EXPECT().OpsServicePlan().Return(oSP).MinTimes(1)
	hc.DS = mds
	_, _, _, ret = hc.servicePlanAllocationCreateValidate(ctx, ai, obj)
	assert.NotNil(ret)
	assert.Regexp("invalid servicePlanId", ret)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure clusterFetch")
	mockCtrl = gomock.NewController(t)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(obj.AccountID)).Return(aObj, nil)
	oA.EXPECT().Fetch(ctx, string(obj.AuthorizedAccountID)).Return(aaObj, nil)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, string(obj.ClusterID)).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(2)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	hc.DS = mds
	_, _, _, ret = hc.servicePlanAllocationCreateValidate(ctx, ai, obj)
	assert.NotNil(ret)
	assert.Regexp("invalid clusterId", ret)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: cluster not in the MANAGED state")
	cObj.State = common.ClusterStateResetting
	mockCtrl = gomock.NewController(t)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(obj.AccountID)).Return(aObj, nil)
	oA.EXPECT().Fetch(ctx, string(obj.AuthorizedAccountID)).Return(aaObj, nil)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, string(obj.ClusterID)).Return(cObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(2)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	hc.DS = mds
	_, _, _, ret = hc.servicePlanAllocationCreateValidate(ctx, ai, obj)
	assert.NotNil(ret)
	assert.Regexp("invalid Cluster state", ret)
	cObj.State = common.ClusterStateManaged // reset
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure authorizedAccountFetch")
	mockCtrl = gomock.NewController(t)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(obj.AccountID)).Return(aObj, nil)
	oA.EXPECT().Fetch(ctx, string(obj.AuthorizedAccountID)).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(2)
	hc.DS = mds
	_, _, _, ret = hc.servicePlanAllocationCreateValidate(ctx, ai, obj)
	assert.NotNil(ret)
	assert.Regexp("invalid authorizedAccountId", ret)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure accountFetch")
	mockCtrl = gomock.NewController(t)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(obj.AccountID)).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	_, _, _, ret = hc.servicePlanAllocationCreateValidate(ctx, ai, obj)
	assert.NotNil(ret)
	assert.Regexp("invalid accountId", ret)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: wrong pool owner")
	mockCtrl = gomock.NewController(t)
	poolObj.AccountID = "otherID"
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(obj.AccountID)).Return(aObj, nil)
	oA.EXPECT().Fetch(ctx, string(obj.AuthorizedAccountID)).Return(aaObj, nil)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, string(obj.ClusterID)).Return(cObj, nil)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Fetch(ctx, string(obj.ServicePlanID)).Return(spObj, nil)
	oP = mock.NewMockPoolOps(mockCtrl)
	oP.EXPECT().Fetch(ctx, "poolID").Return(poolObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(2)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	mds.EXPECT().OpsServicePlan().Return(oSP).MinTimes(1)
	mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
	hc.DS = mds
	_, _, _, ret = hc.servicePlanAllocationCreateValidate(ctx, ai, obj)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, ret)
	poolObj.AccountID = obj.AccountID
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: wrong cluster owner")
	mockCtrl = gomock.NewController(t)
	cObj.AccountID = "otherID"
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(obj.AccountID)).Return(aObj, nil)
	oA.EXPECT().Fetch(ctx, string(obj.AuthorizedAccountID)).Return(aaObj, nil)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, string(obj.ClusterID)).Return(cObj, nil)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Fetch(ctx, string(obj.ServicePlanID)).Return(spObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(2)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	mds.EXPECT().OpsServicePlan().Return(oSP).MinTimes(1)
	hc.DS = mds
	_, _, _, ret = hc.servicePlanAllocationCreateValidate(ctx, ai, obj)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, ret)
	cObj.AccountID = models.ObjID(obj.AccountID)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not valid subordinate")
	mockCtrl = gomock.NewController(t)
	aaObj.TenantAccountID = "other"
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(obj.AccountID)).Return(aObj, nil)
	oA.EXPECT().Fetch(ctx, string(obj.AuthorizedAccountID)).Return(aaObj, nil)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, string(obj.ClusterID)).Return(cObj, nil)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Fetch(ctx, string(obj.ServicePlanID)).Return(spObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(2)
	mds.EXPECT().OpsCluster().Return(oC).MinTimes(1)
	mds.EXPECT().OpsServicePlan().Return(oSP).MinTimes(1)
	hc.DS = mds
	_, _, _, ret = hc.servicePlanAllocationCreateValidate(ctx, ai, obj)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, ret)
	aaObj.TenantAccountID = obj.AccountID
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not authorized tenant")
	mockCtrl = gomock.NewController(t)
	ai2 := &auth.Info{
		AccountID: "other",
		RoleObj:   &models.Role{},
	}
	obj.AuthorizedAccountID = obj.AccountID // skips extra account fetch
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(obj.AccountID)).Return(aObj, nil)
	oC = mock.NewMockClusterOps(mockCtrl)
	oC.EXPECT().Fetch(ctx, string(obj.ClusterID)).Return(cObj, nil)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().Fetch(ctx, string(obj.ServicePlanID)).Return(spObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	mds.EXPECT().OpsCluster().Return(oC)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	hc.DS = mds
	_, _, _, ret = hc.servicePlanAllocationCreateValidate(ctx, ai2, obj)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, ret)
	obj.AuthorizedAccountID = "authorizedAccountId"
	tl.Flush()

	// Create failure - empty params, bad state, bad size
	var badObj *models.ServicePlanAllocation
	tcs := []string{"emptyAccountId", "emptyAuthorizedAccountID", "emptyClusterID", "emptyServicePlanID", "badReservationState", "badSize",
		"invalidStorageFormula"}
	for _, tc := range tcs {
		assert.NotNil(obj)
		testutils.Clone(obj, &badObj)
		switch tc {
		case "emptyAccountId":
			badObj.AccountID = ""
		case "emptyAuthorizedAccountID":
			badObj.AuthorizedAccountID = ""
		case "emptyClusterID":
			badObj.ClusterID = ""
		case "emptyServicePlanID":
			badObj.ServicePlanID = ""
		case "badReservationState":
			badObj.ReservationState = "OK"
		case "badSize":
			badObj.TotalCapacityBytes = swag.Int64(-1)
		case "invalidStorageFormula":
			badObj.StorageFormula = models.StorageFormulaName("invalidStorageFormula")
		default:
			assert.False(true)
		}
		t.Log("case:  error case: " + tc)
		assert.NotPanics(func() { _, _, _, ret = hc.servicePlanAllocationCreateValidate(ctx, ai, badObj) })
		assert.NotNil(ret)
		assert.Regexp("^"+centrald.ErrorMissing.M, ret)
		tl.Flush()
	}

	// no payload
	obj = nil
	t.Log("case: nil payload")
	assert.NotPanics(func() { _, _, _, ret = hc.servicePlanAllocationCreateValidate(ctx, ai, obj) })
	assert.NotNil(ret)
	assert.Regexp("^"+centrald.ErrorMissing.M, ret)
}

func TestServicePlanAllocationCreate(t *testing.T) {
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
	fakeSPAv := &fakeSPAChecks{}
	hc.spaValidator = fakeSPAv
	ai := &auth.Info{}
	params := ops.ServicePlanAllocationCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
		Payload:     &models.ServicePlanAllocationCreateArgs{},
	}
	params.Payload.AccountID = "accountID"
	params.Payload.AuthorizedAccountID = "authorizedAccountID"
	params.Payload.ClusterID = "clusterID"
	params.Payload.ServicePlanID = "servicePlanId"
	ctx := params.HTTPRequest.Context()
	obj := &models.ServicePlanAllocation{
		ServicePlanAllocationCreateOnce: params.Payload.ServicePlanAllocationCreateOnce,
		ServicePlanAllocationMutable: models.ServicePlanAllocationMutable{
			ServicePlanAllocationCreateMutable: params.Payload.ServicePlanAllocationCreateMutable,
		},
	}
	aObj := &models.Account{}
	aObj.Name = "AccountName"
	cObj := &models.Cluster{
		ClusterCreateOnce: models.ClusterCreateOnce{
			CspDomainID: models.ObjIDMutable("cspDomainID"),
		},
	}
	cObj.Name = "cluster"
	spObj := &models.ServicePlan{}
	spObj.Name = "general"

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oSPA := mock.NewMockServicePlanAllocationOps(mockCtrl)
	var inObj *models.ServicePlanAllocation
	testutils.Clone(obj, &inObj)
	var retObj *models.ServicePlanAllocation
	inObj.CspDomainID = "cspDomainID"
	inObj.ReservationState = common.SPAReservationStateNoCapacity
	inObj.ReservableCapacityBytes = swag.Int64(0)
	inObj.TotalCapacityBytes = swag.Int64(0)
	testutils.Clone(obj, &retObj)
	retObj.AuthorizedAccountID = "authorizedAccountID"
	retObj.ClusterID = "clusterID"
	retObj.CspDomainID = "cspDomainID"
	retObj.ReservableCapacityBytes = swag.Int64(0)
	retObj.TotalCapacityBytes = swag.Int64(0)
	retObj.ServicePlanID = "servicePlanID"
	retObj.Meta = &models.ObjMeta{
		ID:      models.ObjID("objectID"),
		Version: 1,
	}
	oSPA.EXPECT().Create(ctx, inObj).Return(retObj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA).MinTimes(1)
	fakeSPAv.aaObj = aObj
	fakeSPAv.clObj = cObj
	fakeSPAv.spObj = spObj
	hc.DS = mds
	var ret middleware.Responder
	cntRLock := hc.cntRLock
	cntRUnlock := hc.cntRUnlock
	assert.Nil(evM.InSSProps)
	ret = hc.servicePlanAllocationCreate(params)
	assert.NotNil(ret)
	_, ok := ret.(*ops.ServicePlanAllocationCreateCreated)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Len(evM.InSSProps, 7)
	assert.EqualValues("0", evM.InSSProps["reservableCapacityBytes"])
	assert.EqualValues("0", evM.InSSProps["totalCapacityBytes"])
	assert.EqualValues("servicePlanID", evM.InSSProps["servicePlanID"])
	assert.EqualValues("cspDomainID", evM.InSSProps["cspDomainID"])
	assert.EqualValues("clusterID", evM.InSSProps["clusterID"])
	assert.EqualValues("authorizedAccountID", evM.InSSProps["authorizedAccountID"])
	assert.EqualValues("objectID", evM.InSSProps["meta.id"])
	assert.Equal(retObj, evM.InACScope)
	assert.Equal("accountID", ai.AccountID)
	exp := &fal.Args{AI: ai, Action: centrald.ServicePlanAllocationCreateAction, ObjID: "objectID", Name: "AccountName/general/cluster", Message: "Created with capacity 0B"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	ai.AccountID = ""
	tl.Flush()
	mockCtrl.Finish()

	inObj.ReservationState = common.SPAReservationStateDisabled

	// state tests
	// disabled, unknown->(no cap, f-ok), no cap->(no cap, f-ok), ok->(no cap, f-ok)
	tcs := []string{"disabled->disabled", "unknown->no cap", "unknown->ok", "no cap->no cap", "no cap->ok", "ok->no cap", "f-ok->ok"}
	for _, tc := range tcs {
		t.Log("case: success", tc)
		mockCtrl = gomock.NewController(t)
		evM := fev.NewFakeEventManager()
		app.CrudeOps = evM
		oSPA := mock.NewMockServicePlanAllocationOps(mockCtrl)
		var inObj *models.ServicePlanAllocation
		testutils.Clone(obj, &inObj)
		var retObj *models.ServicePlanAllocation
		inObj.CspDomainID = "cspDomainID"
		totalEvm := "0"
		resEvm := "0"
		testutils.Clone(obj, &retObj)
		retObj.AuthorizedAccountID = "authorizedAccountID"
		retObj.ClusterID = "clusterID"
		retObj.CspDomainID = "cspDomainID"
		switch tc {
		case "":
			params.Payload.TotalCapacityBytes = swag.Int64(0)
			inObj.TotalCapacityBytes = swag.Int64(0)
			inObj.ReservableCapacityBytes = swag.Int64(0)
			retObj.ReservableCapacityBytes = swag.Int64(0)
			retObj.TotalCapacityBytes = swag.Int64(0)
			params.Payload.ReservationState = common.SPAReservationStateDisabled
			inObj.ReservationState = common.SPAReservationStateDisabled
			totalEvm = "0"
			resEvm = "0"
		case "disabled->disabled":
			params.Payload.TotalCapacityBytes = swag.Int64(0)
			inObj.TotalCapacityBytes = swag.Int64(0)
			inObj.ReservableCapacityBytes = swag.Int64(0)
			retObj.ReservableCapacityBytes = swag.Int64(0)
			retObj.TotalCapacityBytes = swag.Int64(0)
			params.Payload.ReservationState = common.SPAReservationStateDisabled
			inObj.ReservationState = common.SPAReservationStateDisabled
			totalEvm = "0"
			resEvm = "0"
		case "unknown->no cap":
			params.Payload.TotalCapacityBytes = swag.Int64(0)
			inObj.TotalCapacityBytes = swag.Int64(0)
			inObj.ReservableCapacityBytes = swag.Int64(0)
			retObj.ReservableCapacityBytes = swag.Int64(0)
			retObj.TotalCapacityBytes = swag.Int64(0)
			params.Payload.ReservationState = common.SPAReservationStateUnknown
			inObj.ReservationState = common.SPAReservationStateNoCapacity
			totalEvm = "0"
			resEvm = "0"
		case "unknown->ok":
			params.Payload.TotalCapacityBytes = swag.Int64(10)
			inObj.TotalCapacityBytes = swag.Int64(10)
			inObj.ReservableCapacityBytes = swag.Int64(10)
			retObj.ReservableCapacityBytes = swag.Int64(10)
			retObj.TotalCapacityBytes = swag.Int64(10)
			params.Payload.ReservationState = common.SPAReservationStateUnknown
			inObj.ReservationState = common.SPAReservationStateOk
			totalEvm = "10"
			resEvm = "10"
		case "no cap->no cap":
			params.Payload.TotalCapacityBytes = swag.Int64(0)
			inObj.TotalCapacityBytes = swag.Int64(0)
			inObj.ReservableCapacityBytes = swag.Int64(0)
			retObj.ReservableCapacityBytes = swag.Int64(0)
			retObj.TotalCapacityBytes = swag.Int64(0)
			params.Payload.ReservationState = common.SPAReservationStateNoCapacity
			inObj.ReservationState = common.SPAReservationStateNoCapacity
			totalEvm = "0"
			resEvm = "0"
		case "no cap->ok":
			params.Payload.TotalCapacityBytes = swag.Int64(10)
			inObj.TotalCapacityBytes = swag.Int64(10)
			inObj.ReservableCapacityBytes = swag.Int64(10)
			retObj.ReservableCapacityBytes = swag.Int64(10)
			retObj.TotalCapacityBytes = swag.Int64(10)
			params.Payload.ReservationState = common.SPAReservationStateNoCapacity
			inObj.ReservationState = common.SPAReservationStateOk
			totalEvm = "10"
			resEvm = "10"
		case "ok->no cap":
			params.Payload.TotalCapacityBytes = swag.Int64(0)
			inObj.TotalCapacityBytes = swag.Int64(0)
			inObj.ReservableCapacityBytes = swag.Int64(0)
			retObj.ReservableCapacityBytes = swag.Int64(0)
			retObj.TotalCapacityBytes = swag.Int64(0)
			params.Payload.ReservationState = common.SPAReservationStateOk
			inObj.ReservationState = common.SPAReservationStateNoCapacity
			totalEvm = "0"
			resEvm = "0"
		default:
			params.Payload.TotalCapacityBytes = swag.Int64(0)
			inObj.TotalCapacityBytes = swag.Int64(0)
			inObj.ReservableCapacityBytes = swag.Int64(0)
			retObj.ReservableCapacityBytes = swag.Int64(0)
			retObj.TotalCapacityBytes = swag.Int64(0)
			params.Payload.ReservationState = common.SPAReservationStateDisabled
			inObj.ReservationState = common.SPAReservationStateDisabled
			totalEvm = "0"
			resEvm = "0"
		}

		retObj.ServicePlanID = "servicePlanID"
		retObj.Meta = &models.ObjMeta{
			ID:      models.ObjID("objectID"),
			Version: 1,
		}
		oSPA.EXPECT().Create(ctx, inObj).Return(retObj, nil)
		mds := mock.NewMockDataStore(mockCtrl)
		mds.EXPECT().OpsServicePlanAllocation().Return(oSPA).MinTimes(1)
		fakeSPAv.clObj = cObj
		hc.DS = mds
		var ret middleware.Responder
		cntRLock := hc.cntRLock
		cntRUnlock := hc.cntRUnlock
		assert.Nil(evM.InSSProps)
		fa.Posts = []*fal.Args{}
		ret = hc.servicePlanAllocationCreate(params)
		assert.NotNil(ret)
		_, ok := ret.(*ops.ServicePlanAllocationCreateCreated)
		assert.True(ok)
		assert.Equal(cntRLock+1, hc.cntRLock)
		assert.Equal(cntRUnlock+1, hc.cntRUnlock)
		assert.Len(evM.InSSProps, 7)
		assert.EqualValues(resEvm, evM.InSSProps["reservableCapacityBytes"])
		assert.EqualValues(totalEvm, evM.InSSProps["totalCapacityBytes"])
		assert.EqualValues("servicePlanID", evM.InSSProps["servicePlanID"])
		assert.EqualValues("cspDomainID", evM.InSSProps["cspDomainID"])
		assert.EqualValues("clusterID", evM.InSSProps["clusterID"])
		assert.EqualValues("authorizedAccountID", evM.InSSProps["authorizedAccountID"])
		assert.EqualValues("objectID", evM.InSSProps["meta.id"])
		assert.Equal(retObj, evM.InACScope)
		exp.Message = "Created with capacity " + totalEvm + "B"
		assert.Equal([]*fal.Args{exp}, fa.Posts)
		tl.Flush()
		mockCtrl.Finish()
	}
	params.Payload.ReservationState = ""           // reset
	params.Payload.TotalCapacityBytes = nil        // reset
	inObj.ReservableCapacityBytes = nil            // reset
	retObj.ReservableCapacityBytes = swag.Int64(0) // reset
	retObj.TotalCapacityBytes = swag.Int64(0)      // reset

	// Create failure - OpsServicePlanAllocation().Create fails, cover case of AccountID set from auth.Info
	mockCtrl = gomock.NewController(t)
	t.Log("case: Create fails")
	ai.AccountID = "tenantID"
	testutils.Clone(obj, &inObj)
	inObj.AccountID = "tenantID"
	inObj.CspDomainID = "cspDomainID"
	inObj.ReservationState = common.SPAReservationStateNoCapacity
	inObj.TotalCapacityBytes = swag.Int64(0)
	inObj.ReservableCapacityBytes = swag.Int64(0)
	oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
	oSPA.EXPECT().Create(ctx, inObj).Return(nil, centrald.ErrorExists)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA).MinTimes(1)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.servicePlanAllocationCreate(params) })
	assert.NotNil(ret)
	_, ok = ret.(*ops.ServicePlanAllocationCreateCreated)
	assert.False(ok)
	mE, ok := ret.(*ops.ServicePlanAllocationCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorExists.M, *mE.Payload.Message)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	ai.AccountID = ""
	params.Payload.AccountID = "accountID"
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: validation fails")
	mockCtrl = gomock.NewController(t)
	hc.DS = mds
	fakeSPAv.retErr = centrald.ErrorMissing
	fakeSPAv.aaObj = nil
	fakeSPAv.clObj = nil
	fakeSPAv.spObj = nil
	fa.Posts = []*fal.Args{}
	obj.ReservableCapacityBytes = swag.Int64(0)
	obj.TotalCapacityBytes = swag.Int64(0)
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.servicePlanAllocationCreate(params) })
	assert.NotNil(ret)
	_, ok = ret.(*ops.ServicePlanAllocationCreateCreated)
	assert.False(ok)
	mE, ok = ret.(*ops.ServicePlanAllocationCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, swag.StringValue(mE.Payload.Message))
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Equal(obj, fakeSPAv.inObj)
	assert.Empty(fa.Posts)
	fakeSPAv.aaObj = aObj
	fakeSPAv.clObj = cObj
	fakeSPAv.spObj = spObj
	tl.Flush()

	t.Log("case: unauthorized")
	fa.Posts = []*fal.Args{}
	fakeSPAv.retErr = centrald.ErrorUnauthorizedOrForbidden
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.servicePlanAllocationCreate(params) })
	assert.NotNil(ret)
	_, ok = ret.(*ops.ServicePlanAllocationCreateCreated)
	assert.False(ok)
	mE, ok = ret.(*ops.ServicePlanAllocationCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, swag.StringValue(mE.Payload.Message))
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Equal(obj, fakeSPAv.inObj)
	exp = &fal.Args{AI: ai, Action: centrald.ServicePlanAllocationCreateAction, Name: "AccountName/general/cluster", Err: true, Message: "Create unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	tl.Flush()

	t.Log("case: auditLog not ready")
	fa.ReadyRet = centrald.ErrorDbError
	assert.NotPanics(func() { ret = hc.servicePlanAllocationCreate(params) })
	mE, ok = ret.(*ops.ServicePlanAllocationCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.servicePlanAllocationCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ServicePlanAllocationCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	// no payload
	params.Payload = nil
	t.Log("case: nil payload")
	assert.NotPanics(func() { ret = hc.servicePlanAllocationCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ServicePlanAllocationCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorMissing.M, *mE.Payload.Message)
}

func TestServicePlanAllocationDelete(t *testing.T) {
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
	params := ops.ServicePlanAllocationDeleteParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.ServicePlanAllocation{
		ServicePlanAllocationAllOf0: models.ServicePlanAllocationAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("objectID"),
			},
		},
		ServicePlanAllocationCreateOnce: models.ServicePlanAllocationCreateOnce{},
		ServicePlanAllocationMutable:    models.ServicePlanAllocationMutable{},
	}
	obj.AccountID = "aid1"
	obj.AuthorizedAccountID = "authorizedAccountId"
	obj.ClusterID = "clusterId"
	obj.CspDomainID = "cspDomainId"
	obj.ReservableCapacityBytes = swag.Int64(100)
	obj.ServicePlanID = "servicePlanId"
	obj.TotalCapacityBytes = swag.Int64(200)
	aObj := &models.Account{}
	aObj.Name = "AccountName"
	clObj := &models.Cluster{}
	clObj.Name = "cluster"
	spObj := &models.ServicePlan{}
	spObj.Name = "general"
	vsParams := volume_series.VolumeSeriesListParams{ServicePlanAllocationID: &params.ID}
	plParams := pool.PoolListParams{ServicePlanAllocationID: &params.ID}
	vrlParams := volume_series_request.VolumeSeriesRequestListParams{ServicePlanAllocationID: &params.ID, IsTerminated: swag.Bool(false)}

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	obj.SystemTags = []string{common.SystemTagVsrOperator + ":id", common.SystemTagVsrDeleting + ":id"}
	mds := mock.NewMockDataStore(mockCtrl)
	oA := mock.NewMockAccountOps(mockCtrl)
	oC := mock.NewMockClusterOps(mockCtrl)
	oSP := mock.NewMockServicePlanOps(mockCtrl)
	oSPA := mock.NewMockServicePlanAllocationOps(mockCtrl)
	oP := mock.NewMockPoolOps(mockCtrl)
	oVS := mock.NewMockVolumeSeriesOps(mockCtrl)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA).MinTimes(1)
	oSPA.EXPECT().Delete(ctx, params.ID).Return(nil)
	oSPA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsAccount().Return(oA)
	oA.EXPECT().Fetch(ctx, string(obj.AuthorizedAccountID)).Return(aObj, nil)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	oSP.EXPECT().Fetch(ctx, string(obj.ServicePlanID)).Return(spObj, nil)
	mds.EXPECT().OpsCluster().Return(oC)
	oC.EXPECT().Fetch(ctx, string(obj.ClusterID)).Return(clObj, nil)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	oVS.EXPECT().Count(ctx, vsParams, uint(1)).Return(0, nil)
	mds.EXPECT().OpsPool().Return(oP)
	oP.EXPECT().Count(ctx, plParams, uint(1)).Return(0, nil)
	hc.DS = mds
	var ret middleware.Responder
	cntLock := hc.cntLock
	cntUnlock := hc.cntUnlock
	assert.Nil(evM.InSSProps)
	assert.NotPanics(func() { ret = hc.servicePlanAllocationDelete(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.ServicePlanAllocationDeleteNoContent)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Len(evM.InSSProps, 6)
	assert.EqualValues("authorizedAccountId", evM.InSSProps["authorizedAccountID"])
	assert.EqualValues("clusterId", evM.InSSProps["clusterID"])
	assert.EqualValues("cspDomainId", evM.InSSProps["cspDomainID"])
	assert.EqualValues("100", evM.InSSProps["reservableCapacityBytes"])
	assert.EqualValues("servicePlanId", evM.InSSProps["servicePlanID"])
	assert.EqualValues("200", evM.InSSProps["totalCapacityBytes"])
	assert.Equal(obj, evM.InACScope)
	assert.Equal("aid1", ai.AccountID)
	exp := &fal.Args{AI: ai, Action: centrald.ServicePlanAllocationDeleteAction, ObjID: models.ObjID(params.ID), Name: "AccountName/general/cluster", Message: "Deleted"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	obj.SystemTags = nil
	ai.AccountID = ""
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: delete failure, cover valid tenant account")
	mockCtrl = gomock.NewController(t)
	ai.AccountID = "aid1"
	ai.AccountName = "AccountName"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	obj.AuthorizedAccountID = obj.AccountID
	mds = mock.NewMockDataStore(mockCtrl)
	oC = mock.NewMockClusterOps(mockCtrl)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
	oP = mock.NewMockPoolOps(mockCtrl)
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVSR := mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA).MinTimes(1)
	oSPA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oSPA.EXPECT().Delete(ctx, params.ID).Return(centrald.ErrorDbError)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	oSP.EXPECT().Fetch(ctx, string(obj.ServicePlanID)).Return(spObj, nil)
	mds.EXPECT().OpsCluster().Return(oC)
	oC.EXPECT().Fetch(ctx, string(obj.ClusterID)).Return(clObj, nil)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	oVS.EXPECT().Count(ctx, vsParams, uint(1)).Return(0, nil)
	mds.EXPECT().OpsPool().Return(oP)
	oP.EXPECT().Count(ctx, plParams, uint(1)).Return(0, nil)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVSR)
	oVSR.EXPECT().Count(ctx, vrlParams, uint(1)).Return(0, nil)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.servicePlanAllocationDelete(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.ServicePlanAllocationDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	obj.AuthorizedAccountID = "authorizedAccountId"
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: auditLog not ready")
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
	oSPA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA).MinTimes(1)
	fa.ReadyRet = centrald.ErrorDbError
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.servicePlanAllocationDelete(params) })
	mE, ok = ret.(*ops.ServicePlanAllocationDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.servicePlanAllocationDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ServicePlanAllocationDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	t.Log("case: not authorized tenant admin")
	mockCtrl = gomock.NewController(t)
	fa.Posts = []*fal.Args{}
	ai.AccountID = "otherID"
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oC = mock.NewMockClusterOps(mockCtrl)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
	oSPA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsAccount().Return(oA)
	oA.EXPECT().Fetch(ctx, string(obj.AuthorizedAccountID)).Return(aObj, nil)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	oSP.EXPECT().Fetch(ctx, string(obj.ServicePlanID)).Return(spObj, nil)
	mds.EXPECT().OpsCluster().Return(oC)
	oC.EXPECT().Fetch(ctx, string(obj.ClusterID)).Return(clObj, nil)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.servicePlanAllocationDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ServicePlanAllocationDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Regexp(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	exp = &fal.Args{AI: ai, Action: centrald.ServicePlanAllocationDeleteAction, ObjID: models.ObjID(params.ID), Name: "AccountName/general/cluster", Err: true, Message: "Delete unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	ai.RoleObj = nil
	tl.Flush()
	mockCtrl.Finish()

	// cascading validation failure cases
	tObj := []string{"volume series", "pools", "volume series requests"}
	for tc := 0; tc < len(tObj)*2; tc++ {
		mockCtrl = gomock.NewController(t)
		t.Log("case: " + tObj[tc/2] + []string{" count fails", " exists"}[tc%2])
		mds = mock.NewMockDataStore(mockCtrl)
		oA = mock.NewMockAccountOps(mockCtrl)
		oC = mock.NewMockClusterOps(mockCtrl)
		oSP = mock.NewMockServicePlanOps(mockCtrl)
		oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
		mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
		oSPA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
		mds.EXPECT().OpsAccount().Return(oA)
		oA.EXPECT().Fetch(ctx, string(obj.AuthorizedAccountID)).Return(aObj, nil)
		mds.EXPECT().OpsServicePlan().Return(oSP)
		oSP.EXPECT().Fetch(ctx, string(obj.ServicePlanID)).Return(spObj, nil)
		mds.EXPECT().OpsCluster().Return(oC)
		oC.EXPECT().Fetch(ctx, string(obj.ClusterID)).Return(clObj, nil)
		count, err := tc%2, []error{centrald.ErrorDbError, nil}[tc%2]
		switch tc / 2 {
		case 2:
			oVSR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			oVSR.EXPECT().Count(ctx, vrlParams, uint(1)).Return(count, err)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVSR)
			count, err = 0, nil
			fallthrough
		case 1:
			oP = mock.NewMockPoolOps(mockCtrl)
			oP.EXPECT().Count(ctx, plParams, uint(1)).Return(count, err)
			mds.EXPECT().OpsPool().Return(oP)
			count, err = 0, nil
			fallthrough
		case 0:
			oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
			oVS.EXPECT().Count(ctx, vsParams, uint(1)).Return(count, err)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			count, err = 0, nil
		default:
			assert.True(false)
		}
		hc.DS = mds
		cntLock = hc.cntLock
		cntUnlock = hc.cntUnlock
		assert.NotPanics(func() { ret = hc.servicePlanAllocationDelete(params) })
		assert.NotNil(ret)
		mE, ok = ret.(*ops.ServicePlanAllocationDeleteDefault)
		if assert.True(ok) {
			if tc%2 == 0 {
				assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
				assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
			} else {
				assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
				assert.Regexp(tObj[tc/2]+".* the service plan allocation", *mE.Payload.Message)
			}
		}
		assert.Equal(cntLock+1, hc.cntLock)
		assert.Equal(cntUnlock+1, hc.cntUnlock)
		tl.Flush()
		mockCtrl.Finish()
	}

	t.Log("case: servicePlanAllocationPseudoName failure") // fully tested in Update
	mockCtrl = gomock.NewController(t)
	ai.AccountID = "aid1"
	ai.AccountName = "AccountName"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
	oSPA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsAccount().Return(oA)
	oA.EXPECT().Fetch(ctx, string(obj.AuthorizedAccountID)).Return(nil, centrald.ErrorDbError)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.servicePlanAllocationDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ServicePlanAllocationDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
}

func TestServicePlanAllocationFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.ServicePlanAllocationFetchParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.ServicePlanAllocation{
		ServicePlanAllocationAllOf0: models.ServicePlanAllocationAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("objectID"),
			},
		},
		ServicePlanAllocationMutable: models.ServicePlanAllocationMutable{},
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oSPA := mock.NewMockServicePlanAllocationOps(mockCtrl)
	oSPA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA).MinTimes(1)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.servicePlanAllocationFetch(params) })
	assert.NotNil(ret)
	mR, ok := ret.(*ops.ServicePlanAllocationFetchOK)
	assert.True(ok)
	assert.Equal(obj, mR.Payload)
	tl.Flush()

	// fetch failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
	oSPA.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA).MinTimes(1)
	t.Log("case: fetch failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.servicePlanAllocationFetch(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.ServicePlanAllocationFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.servicePlanAllocationFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.ServicePlanAllocationFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
}

func TestServicePlanAllocationList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	fops := &fakeOps{}
	hc.ops = fops
	ai := &auth.Info{}
	params := ops.ServicePlanAllocationListParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	ctx := params.HTTPRequest.Context()
	objects := []*models.ServicePlanAllocation{
		&models.ServicePlanAllocation{
			ServicePlanAllocationAllOf0: models.ServicePlanAllocationAllOf0{
				Meta: &models.ObjMeta{
					ID: models.ObjID("objectID"),
				},
			},
			ServicePlanAllocationCreateOnce: models.ServicePlanAllocationCreateOnce{
				AccountID:           "tid1",
				AuthorizedAccountID: "aid1",
			},
			ServicePlanAllocationMutable: models.ServicePlanAllocationMutable{},
		},
		&models.ServicePlanAllocation{
			ServicePlanAllocationAllOf0: models.ServicePlanAllocationAllOf0{
				Meta: &models.ObjMeta{
					ID: models.ObjID("objectID2"),
				},
			},
			ServicePlanAllocationCreateOnce: models.ServicePlanAllocationCreateOnce{
				AccountID:           "tid1",
				AuthorizedAccountID: "aid2",
			},
			ServicePlanAllocationMutable: models.ServicePlanAllocationMutable{},
		},
		&models.ServicePlanAllocation{
			ServicePlanAllocationAllOf0: models.ServicePlanAllocationAllOf0{
				Meta: &models.ObjMeta{
					ID: models.ObjID("objectID3"),
				},
			},
			ServicePlanAllocationCreateOnce: models.ServicePlanAllocationCreateOnce{
				AccountID:           "tid2",
				AuthorizedAccountID: "aid3",
			},
			ServicePlanAllocationMutable: models.ServicePlanAllocationMutable{},
		},
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oSPA := mock.NewMockServicePlanAllocationOps(mockCtrl)
	oSPA.EXPECT().List(ctx, params).Return(objects, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA).MinTimes(1)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.servicePlanAllocationList(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.ServicePlanAllocationListOK)
	assert.True(ok)
	assert.Equal(objects, mO.Payload)
	tl.Flush()

	// list fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
	oSPA.EXPECT().List(ctx, params).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
	t.Log("case: List failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.servicePlanAllocationList(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.ServicePlanAllocationListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.servicePlanAllocationList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ServicePlanAllocationListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	t.Log("case: constrainBothQueryAccounts changes accounts")
	ai.RoleObj = &models.Role{}
	params.AuthorizedAccountID, params.AccountID = swag.String("a1"), swag.String("t1")
	fops.RetConstrainBothQueryAccountsAID, fops.RetConstrainBothQueryAccountsTID = swag.String("aid1"), swag.String("tid1")
	cParams := params
	cParams.AuthorizedAccountID, cParams.AccountID = fops.RetConstrainBothQueryAccountsAID, fops.RetConstrainBothQueryAccountsTID
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
	oSPA.EXPECT().List(ctx, cParams).Return(objects, nil)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.servicePlanAllocationList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.ServicePlanAllocationListOK)
	assert.True(ok)
	assert.Len(mO.Payload, len(objects))
	assert.Equal(3, fops.CntConstrainBothQueryAccounts)
	assert.Equal(params.AuthorizedAccountID, fops.InConstrainBothQueryAccountsAID)
	assert.Equal(params.AccountID, fops.InConstrainBothQueryAccountsTID)
	tl.Flush()

	t.Log("case: constrainBothQueryAccounts skip")
	fops.RetConstrainBothQueryAccountsSkip = true
	assert.NotPanics(func() { ret = hc.servicePlanAllocationList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.ServicePlanAllocationListOK)
	assert.True(ok)
	assert.Empty(mO.Payload)
	assert.Equal(4, fops.CntConstrainBothQueryAccounts)
}

func TestServicePlanAllocationUpdate(t *testing.T) {
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
	minSize := int64(10)
	ai := &auth.Info{}

	// make the name map
	var nMap JSONToAttrNameMap
	assert.NotPanics(func() { nMap = hc.servicePlanAllocationMutableNameMap() })
	// validate some embedded properties
	assert.Equal("messages", nMap.jName("Messages"))
	assert.Equal("reservationState", nMap.jName("ReservationState"))

	nSRMap := map[string]models.StorageTypeReservation{
		"poolID": models.StorageTypeReservation{
			NumMirrors: 1,
			SizeBytes:  swag.Int64(1),
		},
	}
	sfl := app.SupportedStorageFormulas()
	sfN := sfl[0].Name
	// parse params
	objM := &models.ServicePlanAllocationMutable{}
	objM.ReservationState = "OK"
	objM.ReservableCapacityBytes = swag.Int64(minSize)
	objM.TotalCapacityBytes = swag.Int64(3 * minSize)
	objM.StorageReservations = nSRMap
	objM.StorageFormula = models.StorageFormulaName(sfN)
	params := ops.ServicePlanAllocationUpdateParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
		Version:     swag.Int32(8),
		Remove:      []string{},
		Append:      []string{},
		Set:         []string{nMap.jName("Messages"), nMap.jName("TotalCapacityBytes")},
		Payload:     objM,
	}
	ctx := params.HTTPRequest.Context()
	var ua *centrald.UpdateArgs
	var err error
	appendedSet := append(params.Set, "reservableCapacityBytes", "reservationState") // auto-updated based on totalCapacityBytes
	var uP = [centrald.NumActionTypes][]string{
		centrald.UpdateRemove: params.Remove,
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    appendedSet,
	}
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM := updateArgsMatcher(t, ua).Matcher()
	vsListParams := volume_series.VolumeSeriesListParams{
		ServicePlanAllocationID: swag.String("objectID"),
		VolumeSeriesStateNot:    []string{"DELETING"},
		Sum:                     []string{"sizeBytes"}}
	vsAggRet := []*centrald.Aggregation{
		&centrald.Aggregation{FieldPath: "sizeBytes", Type: "sum", Value: 5},
	}

	obj := &models.ServicePlanAllocation{}
	obj.Meta = &models.ObjMeta{
		ID:      models.ObjID(params.ID),
		Version: 8,
	}
	obj.AccountID = "accountId"
	obj.AuthorizedAccountID = "authorizedAccountId"
	obj.ClusterID = "clusterId"
	obj.CspDomainID = "cspDomainId"
	obj.ServicePlanID = "servicePlanId"
	obj.TotalCapacityBytes = swag.Int64(2 * minSize)
	nSRMap2 := map[string]models.StorageTypeReservation{
		"poolID": models.StorageTypeReservation{
			NumMirrors: 1,
			SizeBytes:  swag.Int64(2),
		},
	}
	obj.StorageReservations = nSRMap2
	assert.Equal(params.Set[0], "messages")
	aObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID(obj.AuthorizedAccountID),
			},
		},
		AccountMutable: models.AccountMutable{
			Name: "AccountName",
		},
	}
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "clusterID",
			},
		},
		ClusterMutable: models.ClusterMutable{
			ClusterCreateMutable: models.ClusterCreateMutable{
				Name: "cluster",
			},
		},
	}
	poolObj := &models.Pool{
		PoolAllOf0: models.PoolAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("poolID"),
			},
		},
		PoolCreateOnce: models.PoolCreateOnce{
			AccountID: obj.AccountID,
		},
	}
	spObj := &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("servicePlanId"),
			},
		},
		ServicePlanMutable: models.ServicePlanMutable{
			Name: "general",
		},
	}

	t.Log("case: auditLog not ready")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mds := mock.NewMockDataStore(mockCtrl)
	oSPA := mock.NewMockServicePlanAllocationOps(mockCtrl)
	oSPA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
	fa.ReadyRet = centrald.ErrorDbError
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.servicePlanAllocationUpdate(params) })
	mD, ok := ret.(*ops.ServicePlanAllocationUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()
	mockCtrl.Finish()

	// various success cases
	tcs := []string{"", "storageReservations v", "storageReservations", "reservableCapacityBytes", "chargedCostPerGiB"}
	for _, tc := range tcs {
		exp := &fal.Args{AI: ai, Action: centrald.ServicePlanAllocationUpdateAction, ObjID: models.ObjID(params.ID), Name: "AccountName/general/cluster", Message: "Updated with new capacity 20B"}
		mockCtrl = gomock.NewController(t)
		oA := mock.NewMockAccountOps(mockCtrl)
		oC := mock.NewMockClusterOps(mockCtrl)
		oSP := mock.NewMockServicePlanOps(mockCtrl)
		oSPA := mock.NewMockServicePlanAllocationOps(mockCtrl)
		oV := mock.NewMockVolumeSeriesOps(mockCtrl)
		mds := mock.NewMockDataStore(mockCtrl)
		mds.EXPECT().OpsServicePlanAllocation().Return(oSPA).MinTimes(1)
		params.Set[0] = "messages"
		params.Set[1] = "totalCapacityBytes"
		params.Version = swag.Int32(int32(obj.Meta.Version))
		uP[centrald.UpdateSet] = appendedSet
		objM.ReservableCapacityBytes = swag.Int64(minSize / 2)
		objM.TotalCapacityBytes = swag.Int64(3 * minSize)
		var updObj *models.ServicePlanAllocation
		testutils.Clone(obj, &updObj)
		fa.Events = []*fal.Args{}
		fa.Posts = []*fal.Args{}
		switch tc {
		case "":
		case "storageReservations v":
			tc = "storageReservations"
			params.Version = nil
			fallthrough
		case "storageReservations":
			params.Set[1] = "messages"
			oP := mock.NewMockPoolOps(mockCtrl)
			oP.EXPECT().Fetch(ctx, "poolID").Return(poolObj, nil)
			mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
			fallthrough
		case "chargedCostPerGiB":
			exp = &fal.Args{AI: ai, Action: centrald.ServicePlanAllocationUpdateAction, ObjID: models.ObjID(params.ID), Name: "AccountName/general/cluster", Message: "Charged cost changed from [0] to [0.21]"}
			params.Set[1] = "chargedCostPerGiB"
			params.Payload.ChargedCostPerGiB = 0.21
			updObj.ChargedCostPerGiB = 0.21
			fallthrough
		default:
			if tc == "reservableCapacityBytes" {
				params.Set[1] = tc
				uP[centrald.UpdateSet] = append(params.Set, "reservationState")
			} else {
				params.Set[0] = tc
				uP[centrald.UpdateSet] = params.Set
			}
			ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP)
			assert.Nil(err)
			assert.NotNil(ua)
			if params.Version == nil {
				uaM = updateArgsMatcher(t, ua).AddsVersion(int32(obj.Meta.Version)).Matcher()
			} else {
				uaM = updateArgsMatcher(t, ua).Matcher()
			}
		}
		t.Log("case: update success", tc)
		mds.EXPECT().OpsAccount().Return(oA)
		oA.EXPECT().Fetch(ctx, string(obj.AuthorizedAccountID)).Return(aObj, nil)
		mds.EXPECT().OpsServicePlan().Return(oSP)
		oSP.EXPECT().Fetch(ctx, string(obj.ServicePlanID)).Return(spObj, nil)
		mds.EXPECT().OpsCluster().Return(oC)
		oC.EXPECT().Fetch(ctx, string(obj.ClusterID)).Return(clObj, nil)
		oSPA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
		oSPA.EXPECT().Update(ctx, uaM, params.Payload).Return(updObj, nil)
		if params.Set[1] == "totalCapacityBytes" || params.Set[1] == "reservableCapacityBytes" {
			oV.EXPECT().Aggregate(ctx, vsListParams).Return([]*centrald.Aggregation{&centrald.Aggregation{}}, 0, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
		}
		hc.DS = mds
		evM.InSSProps = nil
		assert.NotPanics(func() { ret = hc.servicePlanAllocationUpdate(params) })
		assert.NotNil(ret)
		m0, ok := ret.(*ops.ServicePlanAllocationUpdateOK)
		assert.True(ok)
		assert.Equal(updObj, m0.Payload)
		if params.Set[1] == "totalCapacityBytes" {
			assert.Equal(minSize, *params.Payload.TotalCapacityBytes-*obj.TotalCapacityBytes)
			assert.Equal([]*fal.Args{exp}, fa.Events)
			assert.Empty(fa.Posts)
		} else if params.Set[1] == "chargedCostPerGiB" {
			assert.Empty(fa.Events)
			assert.Equal([]*fal.Args{exp}, fa.Posts)
		} else {
			assert.Empty(fa.Events)
			assert.Empty(fa.Posts)
		}
		assert.Len(evM.InSSProps, 6)
		assert.Equal(updObj, evM.InACScope)
		assert.Equal("accountId", ai.AccountID)
		ai.AccountID = ""
		tl.Flush()
		mockCtrl.Finish()
	}

	// state tests
	// disabled, unknown->(no cap, f-ok), no cap->(no cap, f-ok), ok->(no cap, f-ok)
	tcs = []string{"disabled->disabled", "unknown->no cap", "unknown->ok", "no cap->no cap", "no cap->ok", "ok->no cap", "ok->ok"}
	for _, tc := range tcs {
		t.Log("case: state tests success", tc)
		mockCtrl = gomock.NewController(t)
		oA := mock.NewMockAccountOps(mockCtrl)
		oC := mock.NewMockClusterOps(mockCtrl)
		oSP := mock.NewMockServicePlanOps(mockCtrl)
		oSPA := mock.NewMockServicePlanAllocationOps(mockCtrl)
		oV := mock.NewMockVolumeSeriesOps(mockCtrl)
		mds := mock.NewMockDataStore(mockCtrl)
		mds.EXPECT().OpsServicePlanAllocation().Return(oSPA).MinTimes(1)
		stateOnUpdate := ""
		params.Version = swag.Int32(int32(obj.Meta.Version))
		uP[centrald.UpdateSet] = appendedSet
		params.Set[0] = "reservationState"
		params.Set[1] = "reservableCapacityBytes"
		switch tc {
		case "disabled->disabled":
			objM.ReservationState = common.SPAReservationStateDisabled
			objM.ReservableCapacityBytes = swag.Int64(minSize / 2)
			stateOnUpdate = common.SPAReservationStateDisabled
		case "unknown->no cap":
			objM.ReservationState = common.SPAReservationStateUnknown
			objM.ReservableCapacityBytes = swag.Int64(0)
			stateOnUpdate = common.SPAReservationStateNoCapacity
		case "unknown->ok":
			objM.ReservationState = common.SPAReservationStateUnknown
			objM.ReservableCapacityBytes = swag.Int64(minSize / 2)
			stateOnUpdate = common.SPAReservationStateOk
		case "no cap->no cap":
			objM.ReservationState = common.SPAReservationStateNoCapacity
			objM.ReservableCapacityBytes = swag.Int64(0)
			stateOnUpdate = common.SPAReservationStateNoCapacity
		case "no cap->ok":
			objM.ReservationState = common.SPAReservationStateNoCapacity
			objM.ReservableCapacityBytes = swag.Int64(minSize)
			stateOnUpdate = common.SPAReservationStateOk
		case "ok->no cap":
			objM.ReservationState = common.SPAReservationStateOk
			objM.ReservableCapacityBytes = swag.Int64(0)
			stateOnUpdate = common.SPAReservationStateNoCapacity
		case "ok->ok":
			objM.ReservationState = common.SPAReservationStateOk
			objM.ReservableCapacityBytes = nil
			obj.ReservableCapacityBytes = swag.Int64(minSize)
			stateOnUpdate = common.SPAReservationStateOk
		default:
			objM.ReservationState = common.SPAReservationStateDisabled
			objM.ReservableCapacityBytes = swag.Int64(minSize / 2)
			stateOnUpdate = common.SPAReservationStateDisabled
		}

		objM.TotalCapacityBytes = swag.Int64(3 * minSize)
		uP[centrald.UpdateSet] = params.Set
		ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP)
		assert.Nil(err)
		assert.NotNil(ua)
		uaM = updateArgsMatcher(t, ua).Matcher()
		mds.EXPECT().OpsAccount().Return(oA)
		oA.EXPECT().Fetch(ctx, string(obj.AuthorizedAccountID)).Return(aObj, nil)
		mds.EXPECT().OpsServicePlan().Return(oSP)
		oSP.EXPECT().Fetch(ctx, string(obj.ServicePlanID)).Return(spObj, nil)
		mds.EXPECT().OpsCluster().Return(oC)
		oC.EXPECT().Fetch(ctx, string(obj.ClusterID)).Return(clObj, nil)
		oSPA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
		oSPA.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
		if util.Contains(params.Set, "totalCapacityBytes") || util.Contains(params.Set, "reservableCapacityBytes") {
			oV.EXPECT().Aggregate(ctx, vsListParams).Return([]*centrald.Aggregation{&centrald.Aggregation{}}, 0, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
		}
		hc.DS = mds
		evM.InSSProps = nil
		assert.NotPanics(func() { ret = hc.servicePlanAllocationUpdate(params) })
		assert.Equal(stateOnUpdate, params.Payload.ReservationState)
		assert.NotNil(ret)
		m0, ok := ret.(*ops.ServicePlanAllocationUpdateOK)
		assert.True(ok)
		assert.Equal(obj, m0.Payload)
		assert.Len(evM.InSSProps, 6)
		assert.Equal(obj, evM.InACScope)
		tl.Flush()
		mockCtrl.Finish()
	}

	for _, tc := range []string{"SPA", "AuthAccount", "ServicePlan", "Cluster"} {
		t.Logf("case: Fetch %s fails", tc)
		mockCtrl = gomock.NewController(t)
		params.Set[0] = "messages"
		params.Set[1] = "reservableCapacityBytes"
		params.Set = append(params.Set, "reservationState")
		params.Version = swag.Int32(int32(obj.Meta.Version))
		mds = mock.NewMockDataStore(mockCtrl)
		oA := mock.NewMockAccountOps(mockCtrl)
		oC := mock.NewMockClusterOps(mockCtrl)
		oSP := mock.NewMockServicePlanOps(mockCtrl)
		oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
		mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
		if tc == "SPA" {
			oSPA.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorDbError)
		} else {
			oSPA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			mds.EXPECT().OpsAccount().Return(oA)
			if tc == "AuthAccount" {
				oA.EXPECT().Fetch(ctx, string(obj.AuthorizedAccountID)).Return(nil, centrald.ErrorDbError)
			} else {
				oA.EXPECT().Fetch(ctx, string(obj.AuthorizedAccountID)).Return(aObj, nil)
				mds.EXPECT().OpsServicePlan().Return(oSP)
				if tc == "ServicePlan" {
					oSP.EXPECT().Fetch(ctx, string(obj.ServicePlanID)).Return(nil, centrald.ErrorDbError)
				} else {
					assert.Equal("Cluster", tc)
					oSP.EXPECT().Fetch(ctx, string(obj.ServicePlanID)).Return(spObj, nil)
					mds.EXPECT().OpsCluster().Return(oC)
					oC.EXPECT().Fetch(ctx, string(obj.ClusterID)).Return(nil, centrald.ErrorDbError)
				}
			}
		}
		hc.DS = mds
		assert.NotPanics(func() { ret = hc.servicePlanAllocationUpdate(params) })
		assert.NotNil(ret)
		mD, ok = ret.(*ops.ServicePlanAllocationUpdateDefault)
		assert.True(ok)
		assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
		assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
		tl.Flush()
		mockCtrl.Finish()
	}

	t.Log("case: fail on Update (unauthorized)")
	mockCtrl = gomock.NewController(t)
	ai.AccountID = string(obj.AccountID)
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	params.Set = []string{"reservationState", "tags"}
	params.Payload.ReservationState = common.SPAReservationStateDisabled
	uP[centrald.UpdateSet] = params.Set
	ua2, err := hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP)
	assert.Nil(err)
	assert.NotNil(ua2)
	uaM2 := updateArgsMatcher(t, ua2).AddsVersion(int32(obj.Meta.Version)).Matcher()
	mds = mock.NewMockDataStore(mockCtrl)
	oA := mock.NewMockAccountOps(mockCtrl)
	oC := mock.NewMockClusterOps(mockCtrl)
	oSP := mock.NewMockServicePlanOps(mockCtrl)
	oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
	oSPA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA).MinTimes(1)
	mds.EXPECT().OpsAccount().Return(oA)
	oA.EXPECT().Fetch(ctx, string(obj.AuthorizedAccountID)).Return(aObj, nil)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	oSP.EXPECT().Fetch(ctx, string(obj.ServicePlanID)).Return(spObj, nil)
	mds.EXPECT().OpsCluster().Return(oC)
	oC.EXPECT().Fetch(ctx, string(obj.ClusterID)).Return(clObj, nil)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.servicePlanAllocationUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ServicePlanAllocationUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)
	tl.Flush()
	mockCtrl.Finish()

	// Update failed, also tests the no-version in query codepath and valid tenant account updating their own SPA
	t.Log("case: fail on Update")
	mockCtrl = gomock.NewController(t)
	ai.RoleObj = nil // internal
	ai.AccountID = string(obj.AccountID)
	ai.AccountName = "tenant"
	obj.AuthorizedAccountID = obj.AccountID
	params.Set = []string{"reservationState", "tags"}
	params.Payload.ReservationState = common.SPAReservationStateDisabled
	params.Version = nil
	uP[centrald.UpdateSet] = params.Set
	ua2, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP)
	assert.Nil(err)
	assert.NotNil(ua2)
	uaM2 = updateArgsMatcher(t, ua2).AddsVersion(int32(obj.Meta.Version)).Matcher()
	mds = mock.NewMockDataStore(mockCtrl)
	oC = mock.NewMockClusterOps(mockCtrl)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
	oV := mock.NewMockVolumeSeriesOps(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	oSP.EXPECT().Fetch(ctx, string(obj.ServicePlanID)).Return(spObj, nil)
	mds.EXPECT().OpsCluster().Return(oC)
	oC.EXPECT().Fetch(ctx, string(obj.ClusterID)).Return(clObj, nil)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA).MinTimes(1)
	oSPA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oSPA.EXPECT().Update(ctx, uaM2, params.Payload).Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	oV.EXPECT().Aggregate(ctx, vsListParams).Return(vsAggRet, 2, nil)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.servicePlanAllocationUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ServicePlanAllocationUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	obj.AuthorizedAccountID = "authorizedAccountId"
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.servicePlanAllocationUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ServicePlanAllocationUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	// validation failures
	params.Version = swag.Int32(8)
	oVersion := obj.Meta.Version
	tcs = []string{"no change", "reservationState", "pool invalid", "pool owner",
		"reservable>total", "reservable and total", "bad-version and reserved capacity",
		"bad-version and storageReservations", "storageFormula invalid", "messages", "provisioningHints", "reservableCapacityBytes",
		"storageFormula", "storageReservations", "systemTags", "totalCapacityBytes", "tags"}
	for _, tc := range tcs {
		t.Log("case: update error:", tc)
		mockCtrl = gomock.NewController(t)
		ai.RoleObj = nil
		mds = mock.NewMockDataStore(mockCtrl)
		oA = mock.NewMockAccountOps(mockCtrl)
		oC = mock.NewMockClusterOps(mockCtrl)
		oSP = mock.NewMockServicePlanOps(mockCtrl)
		oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
		oP := mock.NewMockPoolOps(mockCtrl)
		fa.Events = []*fal.Args{}
		fa.Posts = []*fal.Args{}
		exp := &fal.Args{AI: ai, Action: centrald.ServicePlanAllocationUpdateAction, ObjID: models.ObjID(params.ID), Name: "AccountName/general/cluster", Err: true, Message: "Update unauthorized"}
		expErr := &centrald.Error{
			C: centrald.ErrorUpdateInvalidRequest.C,
			M: centrald.ErrorUpdateInvalidRequest.M,
		}
		fetchExtraObjects := false
		switch tc {
		case "no change":
			params.Set = []string{}
		case "reservationState":
			params.Set = []string{"reservationState"}
			params.Payload.ReservationState = ""
			mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
			oSPA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			fetchExtraObjects = true
		case "pool invalid":
			params.Set = []string{"storageReservations"}
			params.Payload.StorageReservations = map[string]models.StorageTypeReservation{
				"poolID1": models.StorageTypeReservation{
					NumMirrors: 1,
					SizeBytes:  swag.Int64(1),
				},
			}
			mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
			oSPA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			mds.EXPECT().OpsPool().Return(oP)
			oP.EXPECT().Fetch(ctx, "poolID1").Return(nil, centrald.ErrorNotFound)
			expErr.M = "invalid update .*: invalid poolID poolID1"
			fetchExtraObjects = true
		case "pool owner":
			params.Set = []string{"storageReservations"}
			params.Payload.StorageReservations = map[string]models.StorageTypeReservation{
				"poolID1": models.StorageTypeReservation{
					NumMirrors: 1,
					SizeBytes:  swag.Int64(1),
				},
			}
			var pObj *models.Pool
			testutils.Clone(poolObj, &pObj)
			pObj.AccountID = "otherID"
			mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
			oSPA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			mds.EXPECT().OpsPool().Return(oP)
			oP.EXPECT().Fetch(ctx, "poolID1").Return(pObj, nil)
			expErr = centrald.ErrorUnauthorizedOrForbidden
			fetchExtraObjects = true
		case "reservable>total":
			// fully tested in TestPoolValidateSizes
			params.Set = []string{"reservableCapacityBytes", "reservationState"}
			params.Payload.ReservationState = common.SPAReservationStateOk
			objM.ReservableCapacityBytes = swag.Int64(1 + *obj.TotalCapacityBytes)
			mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
			oSPA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			fetchExtraObjects = true
		case "reservable and total":
			params.Set = []string{"reservableCapacityBytes", "totalCapacityBytes"}
		case "bad-version and reserved capacity":
			params.Set = []string{"reservableCapacityBytes", "reservationState"}
			obj.Meta.Version++
			objM.ReservableCapacityBytes = swag.Int64(1 + *obj.TotalCapacityBytes)
			mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
			oSPA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			expErr = centrald.ErrorIDVerNotFound
		case "bad-version and storageReservations":
			params.Set = []string{"storageReservations"}
			obj.Meta.Version++
			objM.ReservableCapacityBytes = swag.Int64(1 + *obj.TotalCapacityBytes)
			mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
			oSPA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			expErr = centrald.ErrorIDVerNotFound
		case "storageFormula invalid":
			params.Set = []string{"storageFormula"}
			params.Payload.StorageFormula = models.StorageFormulaName("invalidStorageFormula")
			expErr.M = "invalid update .*: invalid storageFormula"
		case "messages":
			fallthrough
		case "provisioningHints":
			fallthrough
		case "reservableCapacityBytes":
			fallthrough
		case "storageFormula":
			fallthrough
		case "storageReservations":
			fallthrough
		case "totalCapacityBytes":
			fallthrough
		case "systemTags":
			params.Set = []string{tc}
			ai.RoleObj = &models.Role{} // wrong role, requires internal role
			ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
			mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
			oSPA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			expErr = centrald.ErrorUnauthorizedOrForbidden
			fetchExtraObjects = true
		case "tags":
			params.Set = []string{tc}
			ai.RoleObj = &models.Role{}
			ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true} // wrong capability
			mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
			oSPA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			expErr = centrald.ErrorUnauthorizedOrForbidden
			fetchExtraObjects = true
		default:
			assert.False(true)
		}
		if fetchExtraObjects {
			mds.EXPECT().OpsAccount().Return(oA)
			oA.EXPECT().Fetch(ctx, string(obj.AuthorizedAccountID)).Return(aObj, nil)
			mds.EXPECT().OpsServicePlan().Return(oSP)
			oSP.EXPECT().Fetch(ctx, string(obj.ServicePlanID)).Return(spObj, nil)
			mds.EXPECT().OpsCluster().Return(oC)
			oC.EXPECT().Fetch(ctx, string(obj.ClusterID)).Return(clObj, nil)
		}
		hc.DS = mds
		assert.NotPanics(func() { ret = hc.servicePlanAllocationUpdate(params) })
		assert.NotNil(ret)
		mD, ok = ret.(*ops.ServicePlanAllocationUpdateDefault)
		assert.True(ok)
		assert.Equal(expErr.C, int(mD.Payload.Code))
		assert.Regexp("^"+expErr.M, *mD.Payload.Message)
		if expErr == centrald.ErrorUnauthorizedOrForbidden {
			assert.Equal([]*fal.Args{exp}, fa.Posts)
			if ai.RoleObj == nil {
				assert.Equal("accountId", ai.AccountID)
				ai.AccountID = ""
			}
		}
		obj.Meta.Version = oVersion
		objM.StorageFormula = models.StorageFormulaName(sfN)
		tl.Flush()
		mockCtrl.Finish()
	}

	t.Log("case: no payload")
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	params.Payload = nil
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.servicePlanAllocationUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.ServicePlanAllocationUpdateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
		assert.Equal(centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	}
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: state no change if disabled")
	mockCtrl = gomock.NewController(t)
	ai.RoleObj = nil
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oC = mock.NewMockClusterOps(mockCtrl)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)

	obj.ReservationState = common.SPAReservationStateDisabled
	obj.ReservableCapacityBytes = swag.Int64(minSize / 2)
	obj.TotalCapacityBytes = swag.Int64(minSize)

	objM = &models.ServicePlanAllocationMutable{}
	objM.ReservableCapacityBytes = swag.Int64(minSize / 3)
	objM.TotalCapacityBytes = nil
	objM.ReservationState = ""
	params = ops.ServicePlanAllocationUpdateParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
		Version:     swag.Int32(8),
		Remove:      []string{},
		Append:      []string{},
		Set:         []string{nMap.jName("ReservableCapacityBytes")},
		Payload:     objM,
	}
	uP[centrald.UpdateSet] = append(params.Set, "reservationState")

	objN := &models.ServicePlanAllocationMutable{}
	objN.ReservationState = common.SPAReservationStateDisabled
	objN.ReservableCapacityBytes = swag.Int64(minSize / 3)
	objN.TotalCapacityBytes = swag.Int64(minSize)
	inParams := ops.ServicePlanAllocationUpdateParams{
		ID:      "objectID",
		Version: swag.Int32(8),
		Remove:  []string{},
		Append:  []string{},
		Set:     []string{nMap.jName("ReservableCapacityBytes"), nMap.jName("ReservationState")},
		Payload: objN,
	}

	ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP)
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).Matcher()
	mds.EXPECT().OpsAccount().Return(oA)
	oA.EXPECT().Fetch(ctx, string(obj.AuthorizedAccountID)).Return(aObj, nil)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	oSP.EXPECT().Fetch(ctx, string(obj.ServicePlanID)).Return(spObj, nil)
	mds.EXPECT().OpsCluster().Return(oC)
	oC.EXPECT().Fetch(ctx, string(obj.ClusterID)).Return(clObj, nil)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA).MinTimes(1)
	oSPA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oSPA.EXPECT().Update(ctx, uaM, inParams.Payload).Return(obj, nil)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	oV.EXPECT().Aggregate(ctx, vsListParams).Return([]*centrald.Aggregation{&centrald.Aggregation{}}, 0, nil)
	hc.DS = mds
	evM.InSSProps = nil
	assert.NotPanics(func() { ret = hc.servicePlanAllocationUpdate(params) })
	assert.NotNil(ret)
	m0, ok := ret.(*ops.ServicePlanAllocationUpdateOK)
	assert.True(ok)
	assert.Equal(obj, m0.Payload)
	assert.Len(evM.InSSProps, 6)
	assert.Equal(obj, evM.InACScope)
}

func TestServicePlanAllocationCustomizeProvisioning(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	fOps := &fakeOps{}
	hc.ops = fOps

	ai := &auth.Info{}
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "clusterID",
			},
		},
	}
	spaObj := &models.ServicePlanAllocation{
		ServicePlanAllocationAllOf0: models.ServicePlanAllocationAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("objectID"),
			},
		},
		ServicePlanAllocationCreateOnce: models.ServicePlanAllocationCreateOnce{
			AuthorizedAccountID: "authorizedAccount",
			ClusterID:           "clusterID",
		},
	}

	t.Log("case: missing auth")
	params := ops.NewServicePlanAllocationCustomizeProvisioningParams()
	params.HTTPRequest = &http.Request{}
	ret := hc.servicePlanAllocationCustomizeProvisioning(params)
	mD, ok := ret.(*ops.ServicePlanAllocationCustomizeProvisioningDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)

	params.HTTPRequest = requestWithAuthContext(ai)

	t.Log("case: internal call")
	ret = hc.servicePlanAllocationCustomizeProvisioning(params)
	mD, ok = ret.(*ops.ServicePlanAllocationCustomizeProvisioningDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)
	assert.Regexp("not for internal use", *mD.Payload.Message)

	ai.RoleObj = &models.Role{}

	t.Log("case: insufficient params")
	ret = hc.servicePlanAllocationCustomizeProvisioning(params)
	mD, ok = ret.(*ops.ServicePlanAllocationCustomizeProvisioningDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorMissing.M, *mD.Payload.Message)
	assert.Regexp("required", *mD.Payload.Message)

	params.VolumeSeriesTag = []string{"k:v"}

	t.Log("case: spa fetch failure")
	fOps.RetServicePlanAllocationFetchErr = centrald.ErrorNotFound
	ret = hc.servicePlanAllocationCustomizeProvisioning(params)
	mD, ok = ret.(*ops.ServicePlanAllocationCustomizeProvisioningDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorNotFound.M, *mD.Payload.Message)

	fOps.RetServicePlanAllocationFetchObj = spaObj
	fOps.RetServicePlanAllocationFetchErr = nil

	t.Log("case: not authorized account")
	ai.AccountID = "un" + string(spaObj.AuthorizedAccountID)
	ret = hc.servicePlanAllocationCustomizeProvisioning(params)
	mD, ok = ret.(*ops.ServicePlanAllocationCustomizeProvisioningDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)

	ai.AccountID = string(spaObj.AuthorizedAccountID)

	t.Log("case: cluster fetch failure")
	fOps.RetClusterFetchErr = centrald.ErrorNotFound
	ret = hc.servicePlanAllocationCustomizeProvisioning(params)
	mD, ok = ret.(*ops.ServicePlanAllocationCustomizeProvisioningDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorNotFound.M, *mD.Payload.Message)

	fOps.RetClusterFetchObj = clObj
	fOps.RetClusterFetchErr = nil

	t.Log("case: cluster client failure")
	clObj.ClusterType = "foo"
	ret = hc.servicePlanAllocationCustomizeProvisioning(params)
	mD, ok = ret.(*ops.ServicePlanAllocationCustomizeProvisioningDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInvalidData.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorInvalidData.M, *mD.Payload.Message)

	clObj.ClusterType = cluster.K8sClusterType

	t.Log("case: secret retrieval error")
	fOps.RetAccountSecretRetrieveVT = nil
	fOps.RetAccountSecretRetrieveErr = centrald.ErrorInternalError
	ret = hc.servicePlanAllocationCustomizeProvisioning(params)
	mD, ok = ret.(*ops.ServicePlanAllocationCustomizeProvisioningDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorInternalError.M, *mD.Payload.Message)

	fOps.RetAccountSecretRetrieveErr = nil
	fOps.RetAccountSecretRetrieveVT = &models.ValueType{Kind: common.ValueTypeSecret, Value: "the secret"}

	// mock the cluster client to validate the arguments passed
	t.Log("case: secret format ok (default name/namespace)")
	params.ApplicationGroupName = swag.String("ag-name")
	params.ApplicationGroupDescription = swag.String("ag-desc")
	params.ApplicationGroupTag = []string{"agk:agv"}
	params.ConsistencyGroupName = swag.String("cg-name")
	params.ConsistencyGroupDescription = swag.String("cg-desc")
	params.ConsistencyGroupTag = []string{"cgk:cgv"}
	mockCtrl := gomock.NewController(t)
	cc := mockcluster.NewMockClient(mockCtrl)
	hc.clusterClientMap = map[string]cluster.Client{}
	hc.clusterClientMap[clObj.ClusterType] = cc
	expSCA := &cluster.SecretCreateArgsMV{
		Intent: cluster.SecretIntentDynamicVolumeCustomization,
		Name:   common.CustomizedSecretNameDefault,
	}
	expSCA.CustomizationData.AccountSecret = fOps.RetAccountSecretRetrieveVT.Value
	expSCA.CustomizationData.ApplicationGroupName = swag.StringValue(params.ApplicationGroupName)
	expSCA.CustomizationData.ApplicationGroupDescription = swag.StringValue(params.ApplicationGroupDescription)
	expSCA.CustomizationData.ApplicationGroupTags = params.ApplicationGroupTag
	expSCA.CustomizationData.ConsistencyGroupName = swag.StringValue(params.ConsistencyGroupName)
	expSCA.CustomizationData.ConsistencyGroupDescription = swag.StringValue(params.ConsistencyGroupDescription)
	expSCA.CustomizationData.ConsistencyGroupTags = params.ConsistencyGroupTag
	expSCA.CustomizationData.VolumeTags = params.VolumeSeriesTag
	cc.EXPECT().SecretFormatMV(gomock.Any(), expSCA).Return("encoded secret", nil)
	ret = hc.servicePlanAllocationCustomizeProvisioning(params)
	mO, ok := ret.(*ops.ServicePlanAllocationCustomizeProvisioningOK)
	assert.True(ok)
	assert.Equal(common.ValueTypeSecret, mO.Payload.Kind)
	assert.Regexp("encoded secret", mO.Payload.Value)
	mockCtrl.Finish()

	t.Log("case: secret format ok (default k8sName/k8sNamespace)")
	mockCtrl = gomock.NewController(t)
	params.K8sName = swag.String("k8sName")
	params.K8sNamespace = swag.String("k8sNamespace")
	cc = mockcluster.NewMockClient(mockCtrl)
	hc.clusterClientMap = map[string]cluster.Client{}
	hc.clusterClientMap[clObj.ClusterType] = cc
	expSCA.Name = swag.StringValue(params.K8sName)
	expSCA.Namespace = swag.StringValue(params.K8sNamespace)
	cc.EXPECT().SecretFormatMV(gomock.Any(), expSCA).Return("encoded secret", nil)
	ret = hc.servicePlanAllocationCustomizeProvisioning(params)
	mO, ok = ret.(*ops.ServicePlanAllocationCustomizeProvisioningOK)
	assert.True(ok)
	assert.Equal(common.ValueTypeSecret, mO.Payload.Kind)
	assert.Regexp("encoded secret", mO.Payload.Value)
	mockCtrl.Finish()

	t.Log("case: secret format error")
	mockCtrl = gomock.NewController(t)
	cc = mockcluster.NewMockClient(mockCtrl)
	hc.clusterClientMap[clObj.ClusterType] = cc
	cc.EXPECT().SecretFormatMV(gomock.Any(), expSCA).Return("", fmt.Errorf("secret-format"))
	ret = hc.servicePlanAllocationCustomizeProvisioning(params)
	mD, ok = ret.(*ops.ServicePlanAllocationCustomizeProvisioningDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInvalidData.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorInvalidData.M, *mD.Payload.Message)
	assert.Regexp("secret-format", *mD.Payload.Message)
}

type fakeSPAChecks struct {
	inObj  *models.ServicePlanAllocation
	retErr error
	aaObj  *models.Account
	spObj  *models.ServicePlan
	clObj  *models.Cluster
}

func (op *fakeSPAChecks) servicePlanAllocationCreateValidate(ctx context.Context, ai *auth.Info, obj *models.ServicePlanAllocation) (*models.Account, *models.ServicePlan, *models.Cluster, error) {
	testutils.Clone(obj, &op.inObj)
	return op.aaObj, op.spObj, op.clObj, op.retErr
}
