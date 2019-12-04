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
	"net/http"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/storage"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/alecthomas/units"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestStorageFetchFilter(t *testing.T) {
	assert := assert.New(t)
	hc := newHandlerComp()
	obj := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta:            &models.ObjMeta{ID: "id1"},
			AccountID:       "aid1",
			TenantAccountID: "tid1",
		},
	}
	rObj := &models.Role{}
	rObj.Capabilities = map[string]bool{}
	ai := &auth.Info{}

	t.Log("case: internal user")
	assert.NoError(hc.storageFetchFilter(ai, obj))

	t.Log("case: SystemManagementCap capability")
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.SystemManagementCap: true}
	assert.NoError(hc.storageFetchFilter(ai, obj))

	t.Log("case: VolumeSeriesOwnerCap capability fails")
	ai.RoleObj = &models.Role{}
	ai.AccountID = "aid1"
	ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.storageFetchFilter(ai, obj))

	t.Log("case: CSPDomainManagementCap capability correct tenant")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	ai.AccountID = "tid1"
	assert.NoError(hc.storageFetchFilter(ai, obj))

	t.Log("case: CSPDomainManagementCap capability wrong tenant")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	ai.AccountID = "tid2"
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.storageFetchFilter(ai, obj))

	t.Log("case: owner account")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	ai.AccountID = "aid1"
	assert.NoError(hc.storageFetchFilter(ai, obj))

	t.Log("case: not owner account")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	ai.AccountID = "aid9"
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.storageFetchFilter(ai, obj))
}

func TestValidateStorageSizes(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app

	// success with just obj
	obj := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			SizeBytes: swag.Int64(int64(units.Tebibyte)),
		},
	}
	obj.AvailableBytes = swag.Int64(int64(units.Gibibyte))
	assert.NoError(hc.validateStorageSizes(nil, obj, nil))

	// success with both
	ua := &centrald.UpdateArgs{
		Attributes: []centrald.UpdateAttr{
			{
				Name: "AvailableBytes",
				Actions: [centrald.NumActionTypes]centrald.UpdateActionArgs{
					centrald.UpdateSet: centrald.UpdateActionArgs{FromBody: true},
				},
			},
		},
	}
	uObj := &models.StorageMutable{AvailableBytes: swag.Int64(2 * int64(units.Gibibyte))}
	assert.NoError(hc.validateStorageSizes(ua, obj, uObj))

	// fail with both, availableBytes too large path
	cO := *obj
	cU := *uObj
	cO.AvailableBytes = swag.Int64(int64(units.Gibibyte))
	cU.AvailableBytes = swag.Int64(*obj.SizeBytes + 1)
	err := hc.validateStorageSizes(ua, &cO, &cU)
	assert.Error(err)
	assert.Regexp("availableBytes.*greater", err.Error())
}

func TestValidateStorageState(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app

	// success with all nil
	assert.NoError(hc.validateStorageState(nil, nil, nil, nil))

	// success with oObj
	ctx := context.Background()
	oObj := &models.Storage{}
	oObj.StorageIdentifier = "123"
	oObj.ClusterID = "clusterID"
	oState := &models.StorageStateMutable{
		AttachmentState:  "ERROR",
		DeviceState:      "UNUSED",
		MediaState:       "UNFORMATTED",
		ProvisionedState: "PROVISIONED",
		Messages:         []*models.TimestampedString{},
	}
	oObj.StorageState = oState
	assert.NoError(hc.validateStorageState(ctx, nil, oObj, nil))

	// success with both, FromBody case
	ua := &centrald.UpdateArgs{
		Attributes: []centrald.UpdateAttr{
			{
				Name: "StorageState",
				Actions: [centrald.NumActionTypes]centrald.UpdateActionArgs{
					centrald.UpdateSet: centrald.UpdateActionArgs{FromBody: true},
				},
			},
		},
	}
	uObj := &models.StorageMutable{}
	uState := &models.StorageStateMutable{
		AttachmentState:  "DETACHED",
		DeviceState:      "UNUSED",
		MediaState:       "FORMATTED",
		ProvisionedState: "PROVISIONED",
		Messages:         []*models.TimestampedString{},
	}
	uObj.StorageState = uState
	assert.NoError(hc.validateStorageState(ctx, ua, oObj, uObj))

	// success with both, FromBody case
	ua2 := &centrald.UpdateArgs{
		Attributes: []centrald.UpdateAttr{
			{
				Name: "StorageIdentifier",
				Actions: [centrald.NumActionTypes]centrald.UpdateActionArgs{
					centrald.UpdateSet: centrald.UpdateActionArgs{FromBody: true},
				},
			},
			{
				Name: "StorageState",
				Actions: [centrald.NumActionTypes]centrald.UpdateActionArgs{
					centrald.UpdateSet: centrald.UpdateActionArgs{FromBody: true},
				},
			},
		},
	}
	uObj.StorageIdentifier = ""
	cU := *uState
	cU.ProvisionedState = "UNPROVISIONED"
	uObj.StorageState = &cU
	assert.NoError(hc.validateStorageState(ctx, ua2, oObj, uObj))

	// success with UNPROVISIONING in original, no storageIdentifier
	var objCopy *models.Storage
	testutils.Clone(oObj, &objCopy)
	objCopy.StorageIdentifier = ""
	objCopy.StorageState.ProvisionedState = "UNPROVISIONING"
	assert.NoError(hc.validateStorageState(ctx, nil, oObj, nil))

	// fail with PROVISIONED in original, no storageIdentifier
	testutils.Clone(oObj, &objCopy)
	objCopy.StorageIdentifier = ""
	err := hc.validateStorageState(ctx, nil, objCopy, nil)
	assert.Error(err)
	assert.Regexp("storageIdentifier required", err.Error())

	// fail with both, no storageIdentifier
	cU = *uState
	cU.ProvisionedState = "UNPROVISIONING"
	uObj.StorageState = &cU
	err = hc.validateStorageState(ctx, ua2, oObj, uObj)
	assert.Error(err)
	assert.Regexp("storageIdentifier required", err.Error())
	oObj.StorageIdentifier = "123"

	// fail with oObj, nodeID case
	cS := *oState
	cS.AttachmentState = "ATTACHED"
	oObj.StorageState = &cS
	err = hc.validateStorageState(ctx, nil, oObj, nil)
	assert.Error(err)
	assert.Regexp("attachedNodeId", err.Error())

	// fail with oState, nodeDevice case
	cS = *oState
	cS.AttachmentState = "ATTACHED"
	cS.AttachedNodeID = "nodeID"
	oObj.StorageState = &cS
	err = hc.validateStorageState(ctx, nil, oObj, nil)
	assert.Error(err)
	assert.Regexp("attachedNodeDevice", err.Error())
	oObj.StorageState = oState

	// fail with both, single field attachmentState cases
	ua3 := &centrald.UpdateArgs{
		Attributes: []centrald.UpdateAttr{
			{
				Name: "StorageState",
				Actions: [centrald.NumActionTypes]centrald.UpdateActionArgs{
					centrald.UpdateSet: centrald.UpdateActionArgs{
						Fields: map[string]struct{}{"AttachmentState": struct{}{}},
					},
				},
			},
		},
	}
	cU = *uState
	cU.AttachmentState = "no good"
	uObj.StorageState = &cU
	err = hc.validateStorageState(ctx, ua3, oObj, uObj)
	assert.Error(err)
	assert.Regexp("^attachmentState", err.Error())

	// fail with both, single field deviceState cases
	ua3.Attributes[0].Actions[centrald.UpdateSet].Fields = map[string]struct{}{"DeviceState": struct{}{}}
	cU = *uState
	cU.DeviceState = "no good"
	uObj.StorageState = &cU
	err = hc.validateStorageState(ctx, ua3, oObj, uObj)
	assert.Error(err)
	assert.Regexp("^deviceState", err.Error())

	// fail with both, single field mediaState cases
	ua3.Attributes[0].Actions[centrald.UpdateSet].Fields = map[string]struct{}{"MediaState": struct{}{}}
	cU = *uState
	cU.MediaState = "no good"
	uObj.StorageState = &cU
	err = hc.validateStorageState(ctx, ua3, oObj, uObj)
	assert.Error(err)
	assert.Regexp("^mediaState", err.Error())

	// fail with both, single field provisionedState cases
	ua3.Attributes[0].Actions[centrald.UpdateSet].Fields = map[string]struct{}{"ProvisionedState": struct{}{}}
	cU = *uState
	cU.ProvisionedState = "no good"
	uObj.StorageState = &cU
	err = hc.validateStorageState(ctx, ua3, oObj, uObj)
	assert.Error(err)
	assert.Regexp("^provisionedState", err.Error())

	// fail with both, single field attachedNodeId cases
	ua3.Attributes[0].Actions[centrald.UpdateSet].Fields = map[string]struct{}{"AttachedNodeID": struct{}{}}
	cS = *oState
	cS.AttachmentState = "ATTACHING"
	cS.AttachedNodeID = "nodeID"
	cS.AttachedNodeDevice = "dev"
	oObj.StorageState = &cS
	cU = *uState
	cU.AttachedNodeID = ""
	uObj.StorageState = &cU
	err = hc.validateStorageState(ctx, ua3, oObj, uObj)
	assert.Error(err)
	assert.Regexp("^attachedNodeId", err.Error())

	// fail with both, single field attachedNodeDevice cases
	ua3.Attributes[0].Actions[centrald.UpdateSet].Fields = map[string]struct{}{"AttachedNodeDevice": struct{}{}}
	cS = *oState
	cS.AttachmentState = "ATTACHED"
	cS.AttachedNodeID = "nodeID"
	cS.AttachedNodeDevice = "\\Device\\HardDisk1\\DR1"
	oObj.StorageState = &cS
	cU = *uState
	cU.AttachedNodeDevice = ""
	uObj.StorageState = &cU
	err = hc.validateStorageState(ctx, ua3, oObj, uObj)
	assert.Error(err)
	assert.Regexp("attachedNodeDevice", err.Error())

	// fail with both, single field attachedNodeDevice cases
	ua3.Attributes[0].Actions[centrald.UpdateSet].Fields = map[string]struct{}{"AttachedNodeID": struct{}{}}
	cS = *oState
	cS.AttachmentState = "DETACHING"
	cS.AttachedNodeID = "nodeID"
	cS.AttachedNodeDevice = "dev"
	oObj.StorageState = &cS
	cU = *uState
	cU.AttachedNodeID = ""
	uObj.StorageState = &cU
	err = hc.validateStorageState(ctx, ua3, oObj, uObj)
	assert.Error(err)
	assert.Regexp("attachedNodeId", err.Error())

	// success with oState, clusterID and nodeID
	nObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{ID: models.ObjID(cS.AttachedNodeID)},
		},
	}
	nObj.ClusterID = models.ObjIDMutable(oObj.ClusterID)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oN := mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, string(cS.AttachedNodeID)).Return(nObj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	hc.DS = mds
	assert.NoError(hc.validateStorageState(ctx, nil, oObj, nil))

	// fail with node not in cluster
	mockCtrl.Finish()
	nObj.ClusterID = "fooey"
	mockCtrl = gomock.NewController(t)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, string(cS.AttachedNodeID)).Return(nObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	hc.DS = mds
	err = hc.validateStorageState(ctx, nil, oObj, nil)
	assert.Error(err)
	assert.Regexp("nodeId not in cluster", err.Error())

	// fail with nodeID not found
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cU = *uState
	cU.AttachmentState = "DETACHING"
	cU.AttachedNodeID = "nodeID"
	cU.AttachedNodeDevice = "dev"
	uObj.StorageState = &cU
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, string(cU.AttachedNodeID)).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	hc.DS = mds
	err = hc.validateStorageState(ctx, ua, oObj, uObj)
	assert.Error(err)
	assert.Regexp("attachedNodeId$", err.Error())

	// fail with both, DB error fetching node
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	ua3.Attributes[0].Actions[centrald.UpdateSet].Fields = map[string]struct{}{"AttachmentState": struct{}{}}
	cS = *oState
	cS.AttachedNodeID = "nodeID"
	cS.AttachedNodeDevice = "dev"
	oObj.StorageState = &cS
	cU = *uState
	cU.AttachmentState = "DETACHING"
	uObj.StorageState = &cU
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, string(cS.AttachedNodeID)).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN).MinTimes(1)
	hc.DS = mds
	err = hc.validateStorageState(ctx, ua3, oObj, uObj)
	assert.Equal(centrald.ErrorDbError, err)
}

func TestStorageCreate(t *testing.T) {
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
	params := ops.StorageCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
		Payload:     &models.Storage{},
	}
	ctx := params.HTTPRequest.Context()
	params.Payload.TenantAccountID = "cleared"
	params.Payload.SizeBytes = swag.Int64(int64(units.Tebibyte))
	params.Payload.StorageIdentifier = "aws-specific-identifier"
	params.Payload.PoolID = "poolID"
	params.Payload.AvailableBytes = swag.Int64(int64(units.Tebibyte))
	params.Payload.StorageState = &models.StorageStateMutable{Messages: []*models.TimestampedString{}}
	obj := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta: &models.ObjMeta{
				ID:      models.ObjID("objectID"),
				Version: 1,
			},
			PoolID: params.Payload.PoolID,
		},
	}
	pObj := &models.Pool{
		PoolAllOf0: models.PoolAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID(params.Payload.PoolID),
			},
		},
	}
	pObj.AccountID = "tid1"
	pObj.AuthorizedAccountID = "tid1"
	pObj.CspDomainID = "cspDomainID"
	pObj.ClusterID = "clusterID"
	pObj.CspStorageType = "Amazon gp2"
	pObj.StorageAccessibility = &models.StorageAccessibilityMutable{
		AccessibilityScope:      "CSPDOMAIN",
		AccessibilityScopeObjID: "cspDomainID",
	}
	cParams := params // creation parameters are modified from original
	cParams.Payload.ClusterID = M.ObjID(pObj.ClusterID)
	cParams.Payload.AccountID = M.ObjID(pObj.AuthorizedAccountID)
	cParams.Payload.TenantAccountID = ""
	cParams.Payload.CspDomainID = M.ObjID(pObj.CspDomainID)
	cParams.Payload.CspStorageType = M.CspStorageType(pObj.CspStorageType)
	cParams.Payload.StorageAccessibility = &M.StorageAccessibility{}
	cParams.Payload.StorageAccessibility.StorageAccessibilityMutable = *pObj.StorageAccessibility

	// success, tenant is authorized
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oS := mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Create(ctx, cParams.Payload).Return(obj, nil)
	oP := mock.NewMockPoolOps(mockCtrl)
	oP.EXPECT().Fetch(ctx, string(params.Payload.PoolID)).Return(pObj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS).MinTimes(1)
	mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	cntRLock := hc.cntRLock
	cntRUnlock := hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.storageCreate(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.StorageCreateCreated)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Len(evM.InSSProps, 1)
	assert.EqualValues("objectID", evM.InSSProps["meta.id"])
	assert.Equal(obj, evM.InACScope)
	tl.Flush()

	// object exists, cover valid authorized subordinate account
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: object exists")
	pObj.AuthorizedAccountID = "aid1"
	ai.AccountID, ai.TenantAccountID = "aid1", "tid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	params.Payload.AccountID, params.Payload.TenantAccountID = "over", "ride"
	oS = mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Create(ctx, params.Payload).Return(nil, centrald.ErrorExists)
	oP = mock.NewMockPoolOps(mockCtrl)
	oP.EXPECT().Fetch(ctx, string(params.Payload.PoolID)).Return(pObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS).MinTimes(1)
	mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.storageCreate(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.StorageCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorExists.M, *mE.Payload.Message)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.EqualValues(params.Payload.AccountID, pObj.AuthorizedAccountID)
	assert.EqualValues(params.Payload.TenantAccountID, pObj.AccountID)
	tl.Flush()

	// Create failed - pool object not found
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oP = mock.NewMockPoolOps(mockCtrl)
	oP.EXPECT().Fetch(ctx, string(params.Payload.PoolID)).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
	t.Log("case: pool not found")
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.storageCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
	assert.Regexp("poolID$", *mE.Payload.Message)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	tl.Flush()

	// Create failed - fetch pool object gets db error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: pool fetch db error")
	oP = mock.NewMockPoolOps(mockCtrl)
	oP.EXPECT().Fetch(ctx, string(params.Payload.PoolID)).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.storageCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.storageCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized")
	oP = mock.NewMockPoolOps(mockCtrl)
	oP.EXPECT().Fetch(ctx, string(params.Payload.PoolID)).Return(pObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	assert.NotPanics(func() { ret = hc.storageCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	ai.AccountID, ai.RoleObj = "", nil
	tl.Flush()

	// missing/incorrect property failure cases
	tcs := []string{"poolID", "availableBytes", "attachmentState"}
	for _, tc := range tcs {
		cl := *params.Payload // copy
		cntRLock = hc.cntRLock
		cntRUnlock = hc.cntRUnlock
		switch tc {
		case "poolID":
			cl.PoolID = ""
		case "availableBytes":
			// size cases are fully tested in TestValidateStorageSizes
			cl.AvailableBytes = swag.Int64(swag.Int64Value(params.Payload.SizeBytes) + 1)
		case "attachmentState":
			// StorageState cases are fully tested in TestValidateStorageState
			cl.StorageState.AttachmentState = "what?"
			cntRLock++
			cntRUnlock++
		default:
			assert.False(true)
		}
		t.Log("case: error case: " + tc)
		assert.NotPanics(func() {
			ret = hc.storageCreate(ops.StorageCreateParams{HTTPRequest: requestWithAuthContext(ai), Payload: &cl})
		})
		assert.NotNil(ret)
		_, ok = ret.(*ops.StorageCreateCreated)
		assert.False(ok)
		mE, ok := ret.(*ops.StorageCreateDefault)
		assert.True(ok)
		assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
		assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
		assert.Equal(cntRLock, hc.cntRLock)
		assert.Equal(cntRUnlock, hc.cntRUnlock)
		tl.Flush()
	}

	// no payload
	params.Payload = nil
	t.Log("case: nil payload")
	assert.NotPanics(func() { ret = hc.storageCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorMissing.M, *mE.Payload.Message)
}

func TestStorageDelete(t *testing.T) {
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
	params := ops.StorageDeleteParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("objectID"),
			},
			AccountID:       "aid1",
			TenantAccountID: "tid1",
		},
	}
	obj.StorageState = &models.StorageStateMutable{ProvisionedState: "UNPROVISIONING"}
	vsParams := volume_series.VolumeSeriesListParams{StorageID: &params.ID}
	vrlParams := volume_series_request.VolumeSeriesRequestListParams{StorageID: &params.ID, IsTerminated: swag.Bool(false)}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oS := mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Delete(ctx, params.ID).Return(nil)
	oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oV := mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Count(ctx, vsParams, uint(1)).Return(0, nil)
	oVR := mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Count(ctx, vrlParams, uint(1)).Return(0, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS).MinTimes(1)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	cntLock := hc.cntLock
	cntUnlock := hc.cntUnlock
	assert.NotPanics(func() { ret = hc.storageDelete(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.StorageDeleteNoContent)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Empty(evM.InSSProps)
	assert.Equal(obj, evM.InACScope)
	tl.Flush()

	// delete failure case, also test deletion of UNPROVISIONED storage and correct owner
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: delete failure")
	ai.AccountID, ai.TenantAccountID = "aid1", "tid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	obj.StorageState = &models.StorageStateMutable{ProvisionedState: "UNPROVISIONED"}
	oS = mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Delete(ctx, params.ID).Return(centrald.ErrorDbError)
	oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	oV.EXPECT().Count(ctx, vsParams, uint(1)).Return(0, nil)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Count(ctx, vrlParams, uint(1)).Return(0, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS).MinTimes(1)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.storageDelete(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.StorageDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	// cascading validation failure cases
	tObj := []string{"volume series", "volume series request"}
	for tc := 0; tc < len(tObj)*2; tc++ {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		t.Log("case: " + tObj[tc/2] + []string{" count fails", " exists"}[tc%2])
		mds = mock.NewMockDataStore(mockCtrl)
		oS = mock.NewMockStorageOps(mockCtrl)
		oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
		mds.EXPECT().OpsStorage().Return(oS)
		count, err := tc%2, []error{centrald.ErrorDbError, nil}[tc%2]
		switch tc / 2 {
		case 1:
			oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			oVR.EXPECT().Count(ctx, vrlParams, uint(1)).Return(count, err)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			count, err = 0, nil
			fallthrough
		case 0:
			oV = mock.NewMockVolumeSeriesOps(mockCtrl)
			oV.EXPECT().Count(ctx, vsParams, uint(1)).Return(count, err)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
		default:
			assert.True(false)
		}
		hc.DS = mds
		cntLock = hc.cntLock
		cntUnlock = hc.cntUnlock
		assert.NotPanics(func() { ret = hc.storageDelete(params) })
		assert.NotNil(ret)
		mE, ok = ret.(*ops.StorageDeleteDefault)
		if assert.True(ok) {
			if tc%2 == 0 {
				assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
				assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
			} else {
				assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
				assert.Regexp(tObj[tc/2]+".* the storage", *mE.Payload.Message)
			}
		}
		assert.Equal(cntLock+1, hc.cntLock)
		assert.Equal(cntUnlock+1, hc.cntUnlock)
		tl.Flush()
	}

	// not unprovisioning
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not UNPROVISIONED/UNPROVISIONING")
	obj.StorageState.ProvisionedState = "PROVISIONED"
	oS = mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.storageDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Regexp("UNPROVISIONING", *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.storageDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized")
	oS = mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	assert.NotPanics(func() { ret = hc.storageDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageDeleteDefault)
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
	oS = mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS).MinTimes(1)
	t.Log("case: fetch failure")
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.storageDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
	assert.Equal(cntLock, hc.cntLock)
	assert.Equal(cntUnlock, hc.cntUnlock)
}

func TestStorageFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.StorageFetchParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("objectID"),
			},
			AccountID:       "aid1",
			TenantAccountID: "tid1",
		},
	}

	// success, correct owner
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ai.AccountID, ai.TenantAccountID = "aid1", "tid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	oS := mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS).MinTimes(1)
	t.Log("case: StorageFetch success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.storageFetch(params) })
	assert.NotNil(ret)
	mR, ok := ret.(*ops.StorageFetchOK)
	assert.True(ok)
	assert.Equal(obj, mR.Payload)
	tl.Flush()

	// fetch failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oS = mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS).MinTimes(1)
	t.Log("case: StorageDelete fetch failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageFetch(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.StorageFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.storageFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized")
	oS = mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS)
	hc.DS = mds
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	assert.NotPanics(func() { ret = hc.storageFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.StorageFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
}

func TestStorageList(t *testing.T) {
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
	params := ops.StorageListParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	ctx := params.HTTPRequest.Context()
	objects := []*models.Storage{
		&models.Storage{
			StorageAllOf0: models.StorageAllOf0{
				Meta: &models.ObjMeta{
					ID: models.ObjID("objectID"),
				},
			},
		},
	}
	dsAggregates := []*centrald.Aggregation{
		&centrald.Aggregation{FieldPath: "availableBytes", Type: "sum", Value: 42},
	}
	aggregates := []string{
		"availableBytes:sum:42",
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oS := mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().List(ctx, params).Return(objects, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS).MinTimes(1)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.storageList(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.StorageListOK)
	assert.True(ok)
	assert.EqualValues(objects, mO.Payload)
	assert.Empty(mO.Aggregations)
	assert.Len(objects, int(mO.TotalCount))
	assert.Equal(1, fops.CntConstrainEOQueryAccounts)
	assert.Nil(fops.InConstrainEOQueryAccountsAID)
	assert.Nil(fops.InConstrainEOQueryAccountsTID)
	tl.Flush()

	// list fails, cover systemAdmin authorized
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.SystemManagementCap: true}
	oS = mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().List(ctx, params).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS).MinTimes(1)
	t.Log("case: List failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageList(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.StorageListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	assert.Equal(2, fops.CntConstrainEOQueryAccounts)

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.storageList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	assert.Equal(2, fops.CntConstrainEOQueryAccounts)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	t.Log("case: constrainEitherOrQueryAccounts changes accounts")
	ai.RoleObj = &models.Role{}
	params.AccountID, params.TenantAccountID = swag.String("a1"), swag.String("t1")
	fops.RetConstrainEOQueryAccountsAID, fops.RetConstrainEOQueryAccountsTID = swag.String("aid1"), swag.String("tid1")
	cParams := params
	cParams.AccountID, cParams.TenantAccountID = fops.RetConstrainEOQueryAccountsAID, fops.RetConstrainEOQueryAccountsTID
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oS = mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().List(ctx, cParams).Return(objects, nil)
	mds.EXPECT().OpsStorage().Return(oS)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.StorageListOK)
	assert.True(ok)
	assert.Equal(3, fops.CntConstrainEOQueryAccounts)
	assert.Equal(params.AccountID, fops.InConstrainEOQueryAccountsAID)
	assert.Equal(params.TenantAccountID, fops.InConstrainEOQueryAccountsTID)
	tl.Flush()

	t.Log("case: constrainEitherOrQueryAccounts nil accounts")
	ai.RoleObj = &models.Role{}
	params.AccountID, params.TenantAccountID = swag.String("a1"), swag.String("t1")
	fops.RetConstrainEOQueryAccountsAID, fops.RetConstrainEOQueryAccountsTID = nil, nil
	assert.NotPanics(func() { ret = hc.storageList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.StorageListOK)
	assert.True(ok)
	assert.Equal(4, fops.CntConstrainEOQueryAccounts)
	assert.Equal(params.AccountID, fops.InConstrainEOQueryAccountsAID)
	assert.Equal(params.TenantAccountID, fops.InConstrainEOQueryAccountsTID)
	tl.Flush()

	t.Log("case: constrainEitherOrQueryAccounts error")
	ai.RoleObj = &models.Role{}
	params.AccountID, params.TenantAccountID = swag.String("a1"), swag.String("t1")
	fops.RetConstrainEOQueryAccountsErr = centrald.ErrorDbError
	assert.NotPanics(func() { ret = hc.storageList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	assert.Equal(5, fops.CntConstrainEOQueryAccounts)
	fops.RetConstrainEOQueryAccountsAID, fops.RetConstrainEOQueryAccountsTID = swag.String("a1"), swag.String("t1")
	fops.RetConstrainEOQueryAccountsErr = nil
	ai.RoleObj = nil
	tl.Flush()

	// success with aggregation
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: aggregation success")
	params.Sum = []string{"availableBytes"}
	oS = mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Aggregate(ctx, params).Return(dsAggregates, 5, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.StorageListOK)
	assert.True(ok)
	assert.Len(mO.Payload, 0)
	assert.Equal(aggregates, mO.Aggregations)
	assert.Equal(5, int(mO.TotalCount))
	tl.Flush()

	// aggregation fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: aggregation failure")
	oS = mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Aggregate(ctx, params).Return(nil, 0, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
}

func TestStorageUpdate(t *testing.T) {
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
	assert.NotPanics(func() { nMap = hc.storageMutableNameMap() })
	// validate embedded properties
	assert.Equal("availableBytes", nMap.jName("AvailableBytes"))
	assert.Equal("storageState", nMap.jName("StorageState"))
	assert.Equal("parcelSizeBytes", nMap.jName("ParcelSizeBytes"))
	assert.Equal("totalParcelCount", nMap.jName("TotalParcelCount"))
	assert.Equal("storageIdentifier", nMap.jName("StorageIdentifier"))

	// parse params
	objM := &models.StorageMutable{}
	objM.AvailableBytes = swag.Int64(int64(units.Gibibyte))
	objM.ParcelSizeBytes = swag.Int64(int64(units.Gibibyte))
	objM.TotalParcelCount = swag.Int64(12345)
	objM.StorageIdentifier = "storage-identifier"
	objM.StorageState = &models.StorageStateMutable{}
	objM.StorageState.AttachmentState = "DETACHED"
	objM.StorageState.DeviceState = "CLOSING"
	objM.StorageState.MediaState = "FORMATTED"
	objM.StorageState.ProvisionedState = "PROVISIONED"
	objM.ShareableStorage = true
	ai := &auth.Info{}
	params := ops.StorageUpdateParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
		Version:     8,
		Remove:      []string{},
		Append:      []string{},
		Set: []string{nMap.jName("AvailableBytes"), nMap.jName("StorageState"), nMap.jName("ShareableStorage"),
			nMap.jName("ParcelSizeBytes"), nMap.jName("TotalParcelCount"), nMap.jName("StorageIdentifier")},
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
	assert.Nil(err)
	assert.NotNil(ua)
	uaM := updateArgsMatcher(t, ua).Matcher()

	obj := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta: &models.ObjMeta{
				ID:      models.ObjID("objectID"),
				Version: 8,
			},
			AccountID:       "aid1",
			TenantAccountID: "tid1",
		},
	}
	obj.CspDomainID = "cspDomainID"
	obj.CspStorageType = "Amazon gp2"
	obj.SizeBytes = swag.Int64(int64(units.Tebibyte))
	obj.StorageAccessibility = &models.StorageAccessibility{}
	obj.StorageAccessibility.StorageAccessibilityMutable.AccessibilityScope = "CSPDOMAIN"
	obj.StorageAccessibility.StorageAccessibilityMutable.AccessibilityScopeObjID = models.ObjIDMutable(obj.CspDomainID)
	obj.StorageIdentifier = "aws-specific-identifier"
	obj.PoolID = "poolID"
	obj.AvailableBytes = swag.Int64(int64(units.Tebibyte))
	obj.StorageState = &models.StorageStateMutable{Messages: []*models.TimestampedString{}}
	obj.StorageState.AttachmentState = centrald.DefaultStorageAttachmentState
	obj.StorageState.ProvisionedState = centrald.DefaultStorageProvisionedState
	obj.ParcelSizeBytes = swag.Int64(int64(units.Gibibyte))
	obj.TotalParcelCount = swag.Int64(1024)
	obj.ShareableStorage = false

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oS := mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oS.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS).MinTimes(1)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.storageUpdate(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.StorageUpdateOK)
	assert.True(ok)
	assert.Equal(obj, mO.Payload)
	assert.Empty(evM.InSSProps)
	assert.Equal(obj, evM.InACScope)
	tl.Flush()

	// Update failed, also test updating provisionedState to UNPROVISIONING
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: Update failed")
	objM.StorageState.ProvisionedState = "UNPROVISIONING"
	oS = mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oS.EXPECT().Update(ctx, uaM, params.Payload).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageUpdate(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.StorageUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()

	// Fetch failed
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: fetch failed")
	oS = mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mD.Payload.Message)
	tl.Flush()

	// Version check fails after fetch
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: version check failed")
	oS = mock.NewMockStorageOps(mockCtrl)
	oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS).MinTimes(1)
	hc.DS = mds
	cP := params
	cP.Version = 1
	assert.NotPanics(func() { ret = hc.storageUpdate(cP) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorIDVerNotFound.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorIDVerNotFound.M, *mD.Payload.Message)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.storageUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	t.Log("case: unauthorized")
	ai.AccountID, ai.TenantAccountID = "aid1", "tid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	assert.NotPanics(func() { ret = hc.storageUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)
	ai.AccountID, ai.RoleObj = "", nil
	tl.Flush()

	// no changes requested
	params.Set = []string{}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	t.Log("case: no change")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)

	// missing invalid updates
	tcs := []string{"availableBytes", "storageState", "shareableStorage"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		cl := *params.Payload // copy
		switch tc {
		case "availableBytes":
			// size cases are fully tested in TestValidateStorageSizes
			cl.AvailableBytes = swag.Int64(swag.Int64Value(obj.SizeBytes) + 1)
		case "storageState":
			// StorageState cases are fully tested in TestValidateStorageState
			cl.StorageState.AttachmentState = "whatever"
		case "shareableStorage":
			obj.ShareableStorage = !params.Payload.ShareableStorage // change while
			obj.AvailableBytes = swag.Int64(0)                      // provisioning...
		default:
			assert.False(true)
		}
		t.Log("case: error case: " + tc)
		oS = mock.NewMockStorageOps(mockCtrl)
		oS.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
		mds = mock.NewMockDataStore(mockCtrl)
		mds.EXPECT().OpsStorage().Return(oS).MinTimes(1)
		hc.DS = mds
		assert.NotPanics(func() {
			ret = hc.storageUpdate(ops.StorageUpdateParams{HTTPRequest: requestWithAuthContext(ai), ID: params.ID, Version: params.Version, Payload: &cl, Set: []string{tc}})
		})
		assert.NotNil(ret)
		mD, ok = ret.(*ops.StorageUpdateDefault)
		assert.True(ok)
		assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
		assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
		tl.Flush()
	}

	// no payload
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: no payload")
	mds = mock.NewMockDataStore(mockCtrl)
	params.Set = []string{"storageState"}
	params.Payload = nil
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.storageUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.StorageUpdateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
		assert.Equal(centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	}
}
