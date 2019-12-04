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
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	sops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/snapshot"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	fal "github.com/Nuvoloso/kontroller/pkg/centrald/audit/fake"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	mockcluster "github.com/Nuvoloso/kontroller/pkg/cluster/mock"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/docker/go-units"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestVolumeSeriesRequestValidateCreator(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ctx := context.Background()

	ai := &auth.Info{}
	aObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{ID: "tid1"},
		},
		AccountMutable: models.AccountMutable{
			UserRoles: map[string]models.AuthRole{
				"uid1": models.AuthRole{},
			},
		},
	}
	args := &models.VolumeSeriesRequestCreateArgs{}

	t.Log("no creator or auth.Info")
	assert.NoError(hc.volumeSeriesRequestValidateCreator(ctx, ai, args))
	assert.NotNil(args.Creator)

	t.Log("auth info overrides")
	ai.AccountID, ai.TenantAccountID, ai.UserID = "aid1", "tid1", "uid1"
	args.Creator.AccountID, args.Creator.TenantAccountID, args.Creator.UserID = "aid2", "tid2", "uid2"
	assert.NoError(hc.volumeSeriesRequestValidateCreator(ctx, ai, args))
	assert.EqualValues("aid1", args.Creator.AccountID)
	assert.EqualValues("tid1", args.Creator.TenantAccountID)
	assert.EqualValues("uid1", args.Creator.UserID)

	t.Log("no auth info, valid creator subordinate account and user ID")
	ai = &auth.Info{}
	args.Creator.TenantAccountID = ""
	args.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{
		VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
			AccountID:       "aid1",
			TenantAccountID: "tid1",
		},
	}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mds := mock.NewMockDataStore(mockCtrl)
	oA := mock.NewMockAccountOps(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	oA.EXPECT().Fetch(ctx, string(args.Creator.AccountID)).Return(aObj, nil)
	hc.DS = mds
	assert.NoError(hc.volumeSeriesRequestValidateCreator(ctx, ai, args))
	assert.EqualValues("aid1", args.Creator.AccountID)
	assert.EqualValues("tid1", args.Creator.TenantAccountID)
	assert.EqualValues("uid1", args.Creator.UserID)
	mockCtrl.Finish()

	t.Log("no auth info, valid creator tenant account and account lookup failure")
	args.Creator.AccountID = "tid1"
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	oA.EXPECT().Fetch(ctx, string(args.Creator.AccountID)).Return(nil, centrald.ErrorDbError)
	hc.DS = mds
	err := hc.volumeSeriesRequestValidateCreator(ctx, ai, args)
	assert.Equal(centrald.ErrorDbError, err)
	assert.EqualValues("tid1", args.Creator.AccountID)
	assert.Empty(args.Creator.TenantAccountID)
	assert.EqualValues("uid1", args.Creator.UserID)
	mockCtrl.Finish()

	t.Log("no auth info, wrong creator account")
	args.Creator.AccountID = "aid2"
	err = hc.volumeSeriesRequestValidateCreator(ctx, ai, args)
	assert.Regexp("creator.accountId", err)

	t.Log("no auth info, creator has user but no account")
	args.Creator.AccountID = ""
	err = hc.volumeSeriesRequestValidateCreator(ctx, ai, args)
	assert.Regexp("creator.userId specified without creator.accountId", err)

	t.Log("no auth info, creator user has no role in the account")
	args.Creator.AccountID = "tid1"
	args.Creator.UserID = "uid2"
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	oA.EXPECT().Fetch(ctx, string(args.Creator.AccountID)).Return(aObj, nil)
	hc.DS = mds
	err = hc.volumeSeriesRequestValidateCreator(ctx, ai, args)
	assert.Regexp("creator.userId has no role", err)
}

func TestVolumeSeriesRequestValidateRequestedOperations(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app

	// test various operation combinations
	opsValid := [][]string{ // in canonical order
		[]string{"ALLOCATE_CAPACITY"},
		[]string{"DELETE_SPA"},
		[]string{"CREATE", "BIND", "PUBLISH", "MOUNT", "VOL_SNAPSHOT_RESTORE", "ATTACH_FS"},
		[]string{"CREATE", "BIND", "PUBLISH", "MOUNT", "VOL_SNAPSHOT_RESTORE"},
		[]string{"CREATE", "BIND", "MOUNT", "VOL_SNAPSHOT_RESTORE"},
		[]string{"CREATE", "BIND", "MOUNT", "ATTACH_FS"},
		[]string{"CREATE", "BIND", "MOUNT"},
		[]string{"CREATE", "BIND", "VOL_SNAPSHOT_RESTORE"},
		[]string{"CREATE", "BIND"},
		[]string{"CREATE", "MOUNT", "VOL_SNAPSHOT_RESTORE"},
		[]string{"CREATE", "MOUNT"},
		[]string{"CREATE", "BIND", "PUBLISH", "VOL_SNAPSHOT_RESTORE"},
		[]string{"CREATE", "PUBLISH", "VOL_SNAPSHOT_RESTORE"},
		[]string{"CREATE", "PUBLISH"},
		[]string{"CREATE", "VOL_SNAPSHOT_RESTORE"},
		[]string{"BIND", "MOUNT", "VOL_SNAPSHOT_RESTORE"},
		[]string{"BIND", "MOUNT"},
		[]string{"DETACH_FS", "UNMOUNT", "UNPUBLISH", "DELETE"},
		[]string{"UNMOUNT", "UNPUBLISH", "DELETE"},
		[]string{"UNMOUNT", "DELETE"},
		[]string{"UNMOUNT", "UNPUBLISH"},
		[]string{"UNPUBLISH", "DELETE"},
		[]string{"CREATE"},
		[]string{"BIND"},
		[]string{"MOUNT"},
		[]string{"PUBLISH"},
		[]string{"VOL_SNAPSHOT_RESTORE"},
		[]string{"ATTACH_FS"},
		[]string{"DETACH_FS"},
		[]string{"UNMOUNT"},
		[]string{"UNPUBLISH"},
		[]string{"DELETE"},
		[]string{"RENAME"},
		[]string{"CREATE_FROM_SNAPSHOT"},
		[]string{"CG_SNAPSHOT_CREATE"},
		[]string{"CONFIGURE"},
		[]string{"CONFIGURE", "VOL_SNAPSHOT_CREATE"},
		[]string{"VOL_SNAPSHOT_CREATE"},
		[]string{"CHANGE_CAPACITY"},
		[]string{"UNBIND"},
		[]string{"NODE_DELETE"},
		[]string{"VOL_DETACH"},
	}
	for _, ops := range opsValid {
		tl.Logger().Infof("case: isValid %v", ops)
		sop, err := hc.volumeSeriesRequestValidateRequestedOperations(ops)
		tl.Logger().Debugf("    %v => %v", ops, sop.canonicalOrder)
		assert.NoError(err)
		assert.Equal(ops, sop.canonicalOrder)
		if len(ops) > 1 {
			for i := 1; i < len(ops); i++ {
				rotateL(&ops, 1)
				iSop, err := hc.volumeSeriesRequestValidateRequestedOperations(ops)
				tl.Logger().Debugf("    %v => %v", ops, iSop.canonicalOrder)
				assert.NoError(err)
				assert.Equal(sop.canonicalOrder, iSop.canonicalOrder)
			}
		}
	}
	// test invalid combinations
	opsInvalid := [][]string{}
	for op1, n1 := range volumeSeriesRequestOperationSequences {
		for op2, n2 := range volumeSeriesRequestOperationSequences {
			if n1 != n2 {
				opsInvalid = append(opsInvalid, []string{op1, op2})
			}
		}
	}
	assert.True(len(opsInvalid) > 0)
	for i, ops := range opsInvalid {
		tl.Logger().Infof("case %d: isNotValid %v", i, ops)
		sop, err := hc.volumeSeriesRequestValidateRequestedOperations(ops)
		assert.Nil(sop, "case %d: isNotValid %v", i, ops)
		assert.Error(err)
		assert.Regexp("invalid operation combination", err)
	}
	_, err := hc.volumeSeriesRequestValidateRequestedOperations([]string{"INVALID"})
	assert.Error(err)
	assert.Regexp("unsupported operation", err)
}

func TestVolumeSeriesRequestOverlappingOperations(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app

	assert.NotEmpty(vsrOverlappingOps)

	assert.True(opsOverlapOk(com.VolReqOpCreateFromSnapshot, com.VolReqOpBind))
	assert.True(opsOverlapOk(com.VolReqOpCreateFromSnapshot, com.VolReqOpCreateFromSnapshot))
	assert.True(opsOverlapOk(com.VolReqOpCreateFromSnapshot, com.VolReqOpMount))
	assert.True(opsOverlapOk(com.VolReqOpCreateFromSnapshot, com.VolReqOpRename))
	assert.True(opsOverlapOk(com.VolReqOpCreateFromSnapshot, com.VolReqOpUnmount))
	assert.True(opsOverlapOk(com.VolReqOpCreateFromSnapshot, com.VolReqOpConfigure))
	assert.True(opsOverlapOk(com.VolReqOpCreateFromSnapshot, com.VolReqOpVolCreateSnapshot))
	assert.True(opsOverlapOk(com.VolReqOpCreateFromSnapshot, com.VolReqOpVolRestoreSnapshot))
	assert.True(opsOverlapOk(com.VolReqOpCreateFromSnapshot, com.VolReqOpPublish))
	assert.True(opsOverlapOk(com.VolReqOpCreateFromSnapshot, com.VolReqOpAttachFs))
	assert.True(opsOverlapOk(com.VolReqOpCreateFromSnapshot, com.VolReqOpDetachFs))
	assert.False(opsOverlapOk(com.VolReqOpCreateFromSnapshot, com.VolReqOpDelete))

	assert.False(opsOverlapOk(com.VolReqOpDelete, com.VolReqOpCreateFromSnapshot))
	assert.True(opsOverlapOk(com.VolReqOpBind, com.VolReqOpCreateFromSnapshot))
	assert.True(opsOverlapOk(com.VolReqOpCreateFromSnapshot, com.VolReqOpCreateFromSnapshot))
	assert.True(opsOverlapOk(com.VolReqOpMount, com.VolReqOpCreateFromSnapshot))
	assert.True(opsOverlapOk(com.VolReqOpRename, com.VolReqOpCreateFromSnapshot))
	assert.True(opsOverlapOk(com.VolReqOpUnmount, com.VolReqOpCreateFromSnapshot))
	assert.True(opsOverlapOk(com.VolReqOpConfigure, com.VolReqOpCreateFromSnapshot))
	assert.True(opsOverlapOk(com.VolReqOpVolCreateSnapshot, com.VolReqOpCreateFromSnapshot))
	assert.True(opsOverlapOk(com.VolReqOpVolRestoreSnapshot, com.VolReqOpCreateFromSnapshot))
	assert.True(opsOverlapOk(com.VolReqOpPublish, com.VolReqOpCreateFromSnapshot))
	assert.True(opsOverlapOk(com.VolReqOpDetachFs, com.VolReqOpCreateFromSnapshot))
	assert.False(opsOverlapOk(com.VolReqOpDelete, com.VolReqOpCreateFromSnapshot))

	assert.True(opsOverlapOk(com.VolReqOpChangeCapacity, com.VolReqOpVolCreateSnapshot))

	sop := &volumeSeriesOpsParsed{}
	sop.canonicalOrder = []string{"CREATE_FROM_SNAPSHOT"}
	mkVSR := func(id string, ops []string) *models.VolumeSeriesRequest {
		vsr := &models.VolumeSeriesRequest{}
		vsr.Meta = &models.ObjMeta{ID: models.ObjID(id)}
		vsr.RequestedOperations = ops
		return vsr
	}
	vsrs := []*models.VolumeSeriesRequest{
		mkVSR("vsc", []string{"CONFIGURE", "VOL_SNAPSHOT_CREATE"}),
		mkVSR("cbm", []string{"BIND", "MOUNT", "VOL_SNAPSHOT_RESTORE"}),
		mkVSR("ren", []string{"RENAME"}),
		mkVSR("vsc", []string{"UNMOUNT"}),
	}
	err := hc.volumeSeriesRequestCreateCheckConcurrentVolOps("id", sop, vsrs)
	assert.NoError(err)

	// check for self-conflicts
	for op := range volumeSeriesRequestOperationSequences {
		expErr := true
		// exceptions
		if op == com.VolReqOpCreateFromSnapshot {
			expErr = false
		}
		ops := []string{op}
		sop := newVSOP(ops)
		vsrs := []*models.VolumeSeriesRequest{
			mkVSR("vsr", ops),
		}
		err := hc.volumeSeriesRequestCreateCheckConcurrentVolOps("id", sop, vsrs)
		if expErr {
			assert.Error(err, "op self-conflict: %s", op)
		} else {
			assert.NoError(err, "op self-conflict: %s", op)
		}
	}
}

func TestVolumeSeriesRequestFetchFilter(t *testing.T) {
	assert := assert.New(t)
	hc := newHandlerComp()
	obj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{Meta: &models.ObjMeta{ID: "id1"}},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{
				VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
					AccountID:       "aid1",
					TenantAccountID: "tid1",
				},
			},
		},
	}
	rObj := &models.Role{}
	rObj.Capabilities = map[string]bool{}
	ai := &auth.Info{}

	t.Log("case: internal user")
	assert.NoError(hc.volumeSeriesRequestFetchFilter(ai, obj))

	t.Log("case: SystemManagementCap capability fails")
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.SystemManagementCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.volumeSeriesRequestFetchFilter(ai, obj))

	t.Log("case: CSPDomainUsageCap capability fails")
	ai.RoleObj = &models.Role{}
	ai.AccountID = "aid1"
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true}
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.volumeSeriesRequestFetchFilter(ai, obj))

	t.Log("case: CSPDomainManagementCap capability correct tenant")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	ai.AccountID = "tid1"
	assert.NoError(hc.volumeSeriesRequestFetchFilter(ai, obj))

	t.Log("case: CSPDomainManagementCap capability wrong tenant")
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	ai.AccountID = "tid2"
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.volumeSeriesRequestFetchFilter(ai, obj))

	t.Log("case: owner account")
	ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: true}
	ai.AccountID = "aid1"
	assert.NoError(hc.volumeSeriesRequestFetchFilter(ai, obj))

	t.Log("case: not owner account")
	ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: true}
	ai.AccountID = "aid9"
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.volumeSeriesRequestFetchFilter(ai, obj))
}

func TestVolumeSeriesRequestCancel(t *testing.T) {
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
	obj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:      "objectId",
				Version: 8,
			},
			CancelRequested: false,
		},
	}
	obj.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{}
	obj.VolumeSeriesCreateSpec.AccountID = "aid1"
	obj.VolumeSeriesCreateSpec.TenantAccountID = "tid1"
	obj.VolumeSeriesRequestState = com.VolReqStateNew
	ai := &auth.Info{}
	params := ops.VolumeSeriesRequestCancelParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectId",
	}
	ctx := params.HTTPRequest.Context()

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	var ret middleware.Responder
	t.Log("case: success")
	mds := mock.NewMockDataStore(mockCtrl)
	oVR := mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(1)
	oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	var retObj *models.VolumeSeriesRequest
	testutils.Clone(obj, &retObj)
	retObj.Meta.Version++
	retObj.CancelRequested = true
	oVR.EXPECT().Cancel(ctx, params.ID, int32(obj.Meta.Version)).Return(retObj, nil)
	hc.DS = mds
	cntLock := hc.cntLock
	cntUnlock := hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCancel(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.VolumeSeriesRequestCancelOK)
	if assert.True(ok) {
		assert.Equal(retObj, mD.Payload)
	}
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Len(evM.InSSProps, 5)
	assert.Empty(evM.InSSProps["clusterId"])
	assert.EqualValues("9", evM.InSSProps["meta.version"])
	assert.Empty("", evM.InSSProps["nodeId"])
	assert.EqualValues("NEW", evM.InSSProps["volumeSeriesRequestState"])
	assert.Empty(evM.InSSProps["volumeSeriesId"])
	assert.Equal(retObj, evM.InACScope)
	tl.Flush()

	// success, already canceled
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: success, already canceled, cover authorized account")
	ai.AccountID = "aid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: true}
	obj.CancelRequested = true
	mds = mock.NewMockDataStore(mockCtrl)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(1)
	oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCancel(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.VolumeSeriesRequestCancelOK)
	if assert.True(ok) {
		assert.Equal(obj, mD.Payload)
	}
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Len(evM.InSSProps, 5)
	assert.Empty(evM.InSSProps["clusterId"])
	assert.EqualValues("8", evM.InSSProps["meta.version"])
	assert.Empty("", evM.InSSProps["nodeId"])
	assert.EqualValues("NEW", evM.InSSProps["volumeSeriesRequestState"])
	assert.Empty(evM.InSSProps["volumeSeriesId"])
	assert.Equal(obj, evM.InACScope)
	tl.Flush()

	// case: already terminated
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: already terminated, cover authorized tenant")
	ai.AccountID = "tid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	obj.VolumeSeriesRequestState = com.VolReqStateSucceeded
	mds = mock.NewMockDataStore(mockCtrl)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCancel(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.VolumeSeriesRequestCancelDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
		assert.Regexp("already terminated", *mE.Payload.Message)
	}
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	// case: fetch fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: fetch fails")
	obj.CancelRequested = false
	mds = mock.NewMockDataStore(mockCtrl)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	oVR.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCancel(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesRequestCancelDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
		assert.Regexp(centrald.ErrorNotFound.M, *mE.Payload.Message)
	}
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCancel(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesRequestCancelDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized")
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	mds = mock.NewMockDataStore(mockCtrl)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCancel(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesRequestCancelDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	ai.AccountID, ai.RoleObj = "", nil
	tl.Flush()

	// case: update fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: update fails")
	obj.VolumeSeriesRequestState = com.VolReqStateCreating
	mds = mock.NewMockDataStore(mockCtrl)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(1)
	oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oVR.EXPECT().Cancel(ctx, params.ID, int32(obj.Meta.Version)).Return(nil, centrald.ErrorDbError)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCancel(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesRequestCancelDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
		assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	}
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()
}

func TestVolumeSeriesRequestCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	saveUUIDGen := uuidGenerator
	defer func() { uuidGenerator = saveUUIDGen }()
	fakeUUID := "fake-U-U-I-D"
	uuidGenerator = func() string { return fakeUUID }
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	fa := &fal.AuditLog{}
	app.AuditLog = fa
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	fops := &fakeOps{}
	hc.ops = fops
	obj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:      "objectId",
				Version: 1,
			},
		},
	}
	aObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta:            &models.ObjMeta{ID: "account2"},
			TenantAccountID: "tid1",
		},
		AccountMutable: models.AccountMutable{
			SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
				CspDomainID:        "domID",
				ProtectionDomainID: "pdID",
			},
		},
	}
	aObj.ProtectionDomains = make(map[string]models.ObjIDMutable)
	aObj.ProtectionDomains[com.ProtectionStoreDefaultKey] = models.ObjIDMutable("pdID")
	spObj := &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			Meta:  &models.ObjMeta{ID: "spID"},
			State: centrald.PublishedState,
		},
		ServicePlanMutable: models.ServicePlanMutable{
			Accounts: []models.ObjIDMutable{"aID", "account2"},
		},
	}
	clusterObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{ID: "clID"},
		},
		ClusterCreateOnce: models.ClusterCreateOnce{
			AccountID:   "tid1",
			CspDomainID: "dom1",
			ClusterType: "kubernetes",
		},
		ClusterMutable: models.ClusterMutable{
			ClusterCreateMutable: models.ClusterCreateMutable{
				AuthorizedAccounts: []models.ObjIDMutable{"aID", "account2"},
			},
		},
	}
	nodeObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta:      &models.ObjMeta{ID: "nodeId"},
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
	agObj := &models.ApplicationGroup{
		ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
			Meta: &models.ObjMeta{ID: "ag1"},
		},
		ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
			AccountID: "aID",
		},
		ApplicationGroupMutable: models.ApplicationGroupMutable{
			Name: "app",
		},
	}
	cgObj := &models.ConsistencyGroup{
		ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
			Meta: &models.ObjMeta{ID: "cgId"},
		},
		ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
			AccountID:       "aID",
			TenantAccountID: "tid1",
		},
		ConsistencyGroupMutable: models.ConsistencyGroupMutable{
			ApplicationGroupIds: []models.ObjIDMutable{"ag1"},
		},
	}
	vsObj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{ID: "vsID1"},
		},
		VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
			AccountID:       models.ObjIDMutable(aObj.Meta.ID),
			TenantAccountID: "tid1",
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				ConsistencyGroupID: "volCgId",
				SizeBytes:          swag.Int64(util.BytesInMiB),
				ServicePlanID:      "spID",
			},
		},
	}
	vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "someID",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{
				VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
					Name: "somename",
				},
			},
			RequestedOperations: []string{com.VolReqOpCreate},
		},
	}
	rObj := &models.Role{}
	rObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true, centrald.VolumeSeriesOwnerCap: true}
	trObj := &models.Role{}
	trObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	badRoleObj := &models.Role{}
	badRoleObj.Capabilities = map[string]bool{centrald.VolumeSeriesOwnerCap: true}
	ai := &auth.Info{
		AccountID:       "aID",
		TenantAccountID: "tID",
		UserID:          "uID",
		RoleObj:         rObj,
	}
	params := ops.VolumeSeriesRequestCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
		Payload:     &models.VolumeSeriesRequestCreateArgs{},
	}
	params.Payload.RequestedOperations = []string{"CREATE", "BIND", "MOUNT", "VOL_SNAPSHOT_RESTORE", "ATTACH_FS"}
	params.Payload.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{}
	params.Payload.VolumeSeriesCreateSpec.AccountID = "ignored"
	params.Payload.VolumeSeriesCreateSpec.ServicePlanID = "spID"
	params.Payload.VolumeSeriesCreateSpec.Name = "name"
	params.Payload.VolumeSeriesCreateSpec.SizeBytes = swag.Int64(util.BytesInMiB)
	params.Payload.ClusterID = "clID"
	params.Payload.NodeID = models.ObjIDMutable(nodeObj.Meta.ID)
	params.Payload.ServicePlanAllocationID = "will-be-zapped-for-now"
	snapshotData := &models.SnapshotData{
		SnapIdentifier:     "snapid",
		PitIdentifier:      "pitid",
		ProtectionDomainID: "pdID",
		Locations: []*models.SnapshotLocation{
			&models.SnapshotLocation{CspDomainID: "ps1"},
		},
		VolumeSeriesID: "sourceVS",
		SizeBytes:      swag.Int64(3 * units.GiB),
	}
	params.Payload.FsType = "ext4"
	params.Payload.TargetPath = "/var/lib/kubelet/vol-path"
	ctx := params.HTTPRequest.Context()
	cfsVSR := &models.VolumeSeriesRequest{}
	cfsVSR.Meta = &models.ObjMeta{ID: "cfsVSR"}
	cfsVSR.RequestedOperations = []string{"CREATE_FROM_SNAPSHOT"}
	params.Payload.SyncCoordinatorID = models.ObjIDMutable(cfsVSR.Meta.ID)
	vsID := string(vsObj.Meta.ID)
	listParams := ops.VolumeSeriesRequestListParams{VolumeSeriesID: &vsID, IsTerminated: swag.Bool(false)}

	// success (with snapshotID)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	var ret middleware.Responder
	t.Log("case: success (CREATE, BIND, MOUNT, VOL_SNAPSHOT_RESTORE, ATTACH_FS) (with snapshotID, syncCoordinator)")
	params.Payload.SnapshotID = "snapID"
	assert.NotEmpty(snapshotData.ProtectionDomainID)
	params.Payload.ProtectionDomainID = ""
	fops.InSnapshotFetchDataID = ""
	fops.InSnapshotFetchDataAi = nil
	fops.RetSnapshotFetchDataObj = snapshotData
	fops.RetSnapshotFetchDataErr = nil
	fops.RetAccountFetchObj = aObj
	mds := mock.NewMockDataStore(mockCtrl)
	oSP := mock.NewMockServicePlanOps(mockCtrl)
	oCL := mock.NewMockClusterOps(mockCtrl)
	oN := mock.NewMockNodeOps(mockCtrl)
	oVR := mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oV := mock.NewMockVolumeSeriesOps(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	oSP.EXPECT().Fetch(ctx, string(spObj.Meta.ID)).Return(spObj, nil)
	mds.EXPECT().OpsCluster().Return(oCL)
	oCL.EXPECT().Fetch(ctx, string(clusterObj.Meta.ID)).Return(clusterObj, nil)
	mds.EXPECT().OpsNode().Return(oN)
	oN.EXPECT().Fetch(ctx, string(params.Payload.NodeID)).Return(nodeObj, nil)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).Times(3)
	oVR.EXPECT().List(ctx, gomock.Any()).Return([]*models.VolumeSeriesRequest{vsrObj}, nil)
	oVR.EXPECT().Fetch(ctx, string(params.Payload.SyncCoordinatorID)).Return(cfsVSR, nil)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	oV.EXPECT().Count(ctx, gomock.Any(), uint(1)).Return(0, nil)
	var retObj *models.VolumeSeriesRequest
	testutils.Clone(obj, &retObj)
	retObj.RequestedOperations = params.Payload.RequestedOperations
	retObj.ClusterID = "clusterID"
	retObj.NodeID = "nodeID"
	retObj.VolumeSeriesRequestState = "NEW"
	retObj.VolumeSeriesID = "volumeSeriesID"
	retObj.ConsistencyGroupID = "cgID"
	oVR.EXPECT().Create(ctx, params.Payload).Return(retObj, nil)
	hc.DS = mds
	cntLock := hc.cntLock
	cntUnlock := hc.cntUnlock
	assert.Nil(evM.InSSProps)
	ret = hc.volumeSeriesRequestCreate(params)
	assert.NotNil(ret)
	_, ok := ret.(*ops.VolumeSeriesRequestCreateCreated)
	assert.True(ok)
	assert.Empty(params.Payload.ServicePlanAllocationID) // TBD: zapped on create
	assert.EqualValues(ai.AccountID, params.Payload.VolumeSeriesCreateSpec.AccountID)
	assert.EqualValues(ai.TenantAccountID, params.Payload.VolumeSeriesCreateSpec.TenantAccountID)
	assert.Equal(snapshotData.ProtectionDomainID, params.Payload.ProtectionDomainID)
	assert.Equal("snapID", fops.InSnapshotFetchDataID)
	assert.Equal(ai, fops.InSnapshotFetchDataAi)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Len(evM.InSSProps, 8)
	assert.EqualValues("clusterID", evM.InSSProps["clusterId"])
	assert.EqualValues("cgID", evM.InSSProps["consistencyGroupId"])
	assert.EqualValues("1", evM.InSSProps["meta.version"])
	assert.EqualValues("nodeID", evM.InSSProps["nodeId"])
	assert.EqualValues("CREATE,BIND,MOUNT,VOL_SNAPSHOT_RESTORE,ATTACH_FS", evM.InSSProps["requestedOperations"])
	assert.EqualValues("NEW", evM.InSSProps["volumeSeriesRequestState"])
	assert.EqualValues("volumeSeriesID", evM.InSSProps["volumeSeriesId"])
	assert.EqualValues("objectId", evM.InSSProps["meta.id"])
	assert.Equal(retObj, evM.InACScope)
	// cleanup params
	params.Payload.SnapshotID = ""
	params.Payload.VolumeSeriesCreateSpec.AccountID = "ignored"
	params.Payload.VolumeSeriesCreateSpec.TenantAccountID = "ignored"
	tl.Flush()

	// success (no snapshotId)
	mockCtrl.Finish()
	params.Payload.SyncCoordinatorID = models.ObjIDMutable(cfsVSR.Meta.ID)
	params.Payload.Snapshot = snapshotData
	params.Payload.ProtectionDomainID = ""
	fops.InSnapshotFetchDataID = ""
	fops.InSnapshotFetchDataAi = nil
	fops.RetSnapshotFetchDataObj = nil
	fops.RetSnapshotFetchDataErr = nil
	mockCtrl = gomock.NewController(t)
	t.Log("case: success (CREATE, BIND, MOUNT, VOL_SNAPSHOT_RESTORE, ATTACH_FS) (with syncCoordinator)")
	mds = mock.NewMockDataStore(mockCtrl)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oN = mock.NewMockNodeOps(mockCtrl)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oV = mock.NewMockVolumeSeriesOps(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	oSP.EXPECT().Fetch(ctx, string(spObj.Meta.ID)).Return(spObj, nil)
	mds.EXPECT().OpsCluster().Return(oCL)
	oCL.EXPECT().Fetch(ctx, string(clusterObj.Meta.ID)).Return(clusterObj, nil)
	mds.EXPECT().OpsNode().Return(oN)
	oN.EXPECT().Fetch(ctx, string(params.Payload.NodeID)).Return(nodeObj, nil)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).Times(3)
	oVR.EXPECT().List(ctx, gomock.Any()).Return([]*models.VolumeSeriesRequest{vsrObj}, nil)
	oVR.EXPECT().Fetch(ctx, string(params.Payload.SyncCoordinatorID)).Return(cfsVSR, nil)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	oV.EXPECT().Count(ctx, gomock.Any(), uint(1)).Return(0, nil)
	testutils.Clone(obj, &retObj)
	retObj.RequestedOperations = params.Payload.RequestedOperations
	retObj.ClusterID = "clusterID"
	retObj.NodeID = "nodeID"
	retObj.VolumeSeriesRequestState = "NEW"
	retObj.VolumeSeriesID = "volumeSeriesID"
	retObj.ConsistencyGroupID = "cgID"
	oVR.EXPECT().Create(ctx, params.Payload).Return(retObj, nil)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	evM.InSSProps = nil
	ret = hc.volumeSeriesRequestCreate(params)
	assert.NotNil(ret)
	_, ok = ret.(*ops.VolumeSeriesRequestCreateCreated)
	assert.True(ok)
	assert.EqualValues(ai.AccountID, params.Payload.VolumeSeriesCreateSpec.AccountID)
	assert.EqualValues(ai.TenantAccountID, params.Payload.VolumeSeriesCreateSpec.TenantAccountID)
	assert.Equal(snapshotData.ProtectionDomainID, params.Payload.ProtectionDomainID)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Len(evM.InSSProps, 8)
	assert.EqualValues("clusterID", evM.InSSProps["clusterId"])
	assert.EqualValues("cgID", evM.InSSProps["consistencyGroupId"])
	assert.EqualValues("1", evM.InSSProps["meta.version"])
	assert.EqualValues("nodeID", evM.InSSProps["nodeId"])
	assert.EqualValues("CREATE,BIND,MOUNT,VOL_SNAPSHOT_RESTORE,ATTACH_FS", evM.InSSProps["requestedOperations"])
	assert.EqualValues("NEW", evM.InSSProps["volumeSeriesRequestState"])
	assert.EqualValues("volumeSeriesID", evM.InSSProps["volumeSeriesId"])
	assert.EqualValues("objectId", evM.InSSProps["meta.id"])
	assert.Equal(retObj, evM.InACScope)
	// cleanup params
	params.Payload.Snapshot = nil
	params.Payload.SyncCoordinatorID = ""
	params.Payload.FsType = ""
	params.Payload.TargetPath = ""
	params.Payload.NodeID = ""
	tl.Flush()

	// success
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: success (CREATE, BIND, PUBLISH)")
	params.Payload.RequestedOperations = []string{"CREATE", "BIND", "PUBLISH"}
	params.Payload.FsType = "ext4"
	params.Payload.DriverType = "flex"
	evM = fev.NewFakeEventManager()
	app.CrudeOps = evM
	mds = mock.NewMockDataStore(mockCtrl)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oClCl := mockcluster.NewMockClient(mockCtrl)
	hc.clusterClientMap = make(map[string]cluster.Client)
	hc.clusterClientMap[clusterObj.ClusterType] = oClCl
	mds.EXPECT().OpsServicePlan().Return(oSP)
	oSP.EXPECT().Fetch(ctx, string(spObj.Meta.ID)).Return(spObj, nil)
	mds.EXPECT().OpsCluster().Return(oCL)
	oCL.EXPECT().Fetch(ctx, string(clusterObj.Meta.ID)).Return(clusterObj, nil)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).Times(2)
	oVR.EXPECT().List(ctx, gomock.Any()).Return([]*models.VolumeSeriesRequest{}, nil)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	oV.EXPECT().Count(ctx, gomock.Any(), uint(1)).Return(0, nil)
	testutils.Clone(obj, &retObj)
	retObj.RequestedOperations = params.Payload.RequestedOperations
	retObj.ClusterID = "clusterID"
	retObj.VolumeSeriesRequestState = "NEW"
	retObj.VolumeSeriesID = "volumeSeriesID"
	retObj.ConsistencyGroupID = "cgID"
	oVR.EXPECT().Create(ctx, params.Payload).Return(retObj, nil)
	oClCl.EXPECT().GetFileSystemTypes().Return([]string{"ext4"}).MinTimes(1)
	oClCl.EXPECT().GetDriverTypes().Return([]string{"flex"}).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.Nil(evM.InSSProps)
	ret = hc.volumeSeriesRequestCreate(params)
	assert.NotNil(ret)
	_, ok = ret.(*ops.VolumeSeriesRequestCreateCreated)
	assert.True(ok)
	assert.Empty(params.Payload.ServicePlanAllocationID) // TBD: zapped on create
	assert.EqualValues(ai.AccountID, params.Payload.VolumeSeriesCreateSpec.AccountID)
	assert.EqualValues(ai.TenantAccountID, params.Payload.VolumeSeriesCreateSpec.TenantAccountID)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Len(evM.InSSProps, 8)
	assert.EqualValues("clusterID", evM.InSSProps["clusterId"])
	assert.EqualValues("cgID", evM.InSSProps["consistencyGroupId"])
	assert.EqualValues("1", evM.InSSProps["meta.version"])
	assert.EqualValues("CREATE,BIND,PUBLISH", evM.InSSProps["requestedOperations"])
	assert.EqualValues("NEW", evM.InSSProps["volumeSeriesRequestState"])
	assert.EqualValues("volumeSeriesID", evM.InSSProps["volumeSeriesId"])
	assert.EqualValues("objectId", evM.InSSProps["meta.id"])
	assert.Equal(retObj, evM.InACScope)
	tl.Flush()

	// Create fails, also test canonical rewrite rules, valid AGs, valid non-zero completeByTime
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	ai.AccountID, ai.UserID, ai.RoleObj = "", "", nil
	params.Payload.Creator.TenantAccountID, params.Payload.Creator.UserID = "", ""
	params.Payload.ApplicationGroupIds = []models.ObjIDMutable{"ag1", "ag2"}
	params.Payload.RequestedOperations = []string{"MOUNT", "CREATE", "ATTACH_FS", "BIND"}
	params.Payload.FsType = "xfs"
	params.Payload.TargetPath = "/mnt/foo"
	params.Payload.CompleteByTime = strfmt.DateTime(time.Now().Add(time.Hour))
	aObj.ProtectionDomains["dom1"] = models.ObjIDMutable("dom1-pdID")
	t.Log("case: create fails")
	mds = mock.NewMockDataStore(mockCtrl)
	oA := mock.NewMockAccountOps(mockCtrl)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oN = mock.NewMockNodeOps(mockCtrl)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	oA.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesCreateSpec.AccountID)).Return(aObj, nil)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	oSP.EXPECT().Fetch(ctx, string(spObj.Meta.ID)).Return(spObj, nil)
	mds.EXPECT().OpsCluster().Return(oCL)
	oCL.EXPECT().Fetch(ctx, string(clusterObj.Meta.ID)).Return(clusterObj, nil)
	mds.EXPECT().OpsNode().Return(oN)
	oN.EXPECT().Fetch(ctx, string(params.Payload.NodeID)).Return(nodeObj, nil)
	oAG := mock.NewMockApplicationGroupOps(mockCtrl)
	oAG.EXPECT().Fetch(ctx, "ag1").Return(agObj, nil)
	oAG.EXPECT().Fetch(ctx, "ag2").Return(agObj, nil)
	mds.EXPECT().OpsApplicationGroup().Return(oAG).MinTimes(1)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).Times(2)
	oVR.EXPECT().List(ctx, gomock.Any()).Return([]*models.VolumeSeriesRequest{}, nil)
	oVR.EXPECT().Create(ctx, params.Payload).Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	oV.EXPECT().Count(ctx, gomock.Any(), uint(1)).Return(0, nil)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(params) })
	assert.NotNil(ret)
	assert.Equal([]string{"CREATE", "BIND", "MOUNT", "ATTACH_FS"}, params.Payload.RequestedOperations)
	assert.EqualValues("aID", params.Payload.VolumeSeriesCreateSpec.AccountID)
	assert.EqualValues("tid1", params.Payload.VolumeSeriesCreateSpec.TenantAccountID)
	assert.EqualValues("tid1", params.Payload.Creator.TenantAccountID)
	assert.EqualValues("dom1-pdID", params.Payload.ProtectionDomainID)
	mE, ok := ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
		assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	}
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	params.Payload.FsType = ""
	params.Payload.TargetPath = ""
	delete(aObj.ProtectionDomains, "dom1") // reset
	tl.Flush()

	// CREATE operation and unauthorized
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: Create with CREATE op, no volumeSeriesOwner capability")
	ai.AccountID, ai.TenantAccountID, ai.RoleObj = "tid1", "", trObj
	ret = hc.volumeSeriesRequestCreate(params)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	}
	tl.Flush()

	// BIND operation and unauthorized
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: Create with BIND op, no CSPDomainUsage capability")
	ai.AccountID, ai.TenantAccountID, ai.RoleObj = "account2", "tid1", badRoleObj
	params.Payload.ApplicationGroupIds = []models.ObjIDMutable{}
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	oSP.EXPECT().Fetch(ctx, string(spObj.Meta.ID)).Return(spObj, nil)
	mds.EXPECT().OpsCluster().Return(oCL)
	oCL.EXPECT().Fetch(ctx, string(clusterObj.Meta.ID)).Return(clusterObj, nil)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	oVR.EXPECT().List(ctx, gomock.Any()).Return([]*models.VolumeSeriesRequest{}, nil)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	oV.EXPECT().Count(ctx, gomock.Any(), uint(1)).Return(0, nil)
	hc.DS = mds
	ret = hc.volumeSeriesRequestCreate(params)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	}
	tl.Flush()

	// Create with UNMOUNT, DETACH_FS op
	t.Log("case: Create with DETACH_FS, UNMOUNT op, valid volumeSeriesOwner")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	ai.AccountID, ai.TenantAccountID, ai.RoleObj = "account2", "tid1", rObj
	params.Payload.RequestedOperations = []string{"UNMOUNT", "DETACH_FS"}
	params.Payload.VolumeSeriesID = models.ObjIDMutable(vsID)
	params.Payload.ClusterID = ""
	params.Payload.VolumeSeriesCreateSpec.AccountID = ""
	params.Payload.VolumeSeriesCreateSpec.TenantAccountID = ""
	params.Payload.TargetPath = "/mnt/foo"
	params.Payload.NodeID = models.ObjIDMutable(nodeObj.Meta.ID)
	vsObj.BoundClusterID = "cl1"
	vsObj.VolumeSeriesState = com.VolStateInUse
	vsObj.Mounts = []*models.Mount{
		{MountedNodeID: models.ObjIDMutable(nodeObj.Meta.ID), SnapIdentifier: com.VolMountHeadIdentifier, MountState: "MOUNTED"},
	}
	vsObj.SystemTags = []string{fmt.Sprintf("%s:%s", com.SystemTagVolumeFsAttached, params.Payload.TargetPath)}
	mds = mock.NewMockDataStore(mockCtrl)
	oVS := mock.NewMockVolumeSeriesOps(mockCtrl)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oN = mock.NewMockNodeOps(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
	mds.EXPECT().OpsNode().Return(oN)
	oN.EXPECT().Fetch(ctx, string(params.Payload.NodeID)).Return(nodeObj, nil)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(1)
	oVR.EXPECT().Create(ctx, params.Payload).Return(nil, centrald.ErrorDbError)
	oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
		assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	}
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Equal([]string{"DETACH_FS", "UNMOUNT"}, params.Payload.RequestedOperations)
	assert.EqualValues("cl1", params.Payload.ClusterID)
	assert.EqualValues(ai.AccountID, params.Payload.VolumeSeriesCreateSpec.AccountID)
	assert.EqualValues(ai.TenantAccountID, params.Payload.VolumeSeriesCreateSpec.TenantAccountID)
	params.Payload.TargetPath = ""
	vsObj.SystemTags = nil
	params.Payload.NodeID = ""
	tl.Flush()

	// UNMOUNT operation and unauthorized
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: Create with UNMOUNT op, no volumeSeriesOwner capability")
	ai.AccountID, ai.TenantAccountID, ai.RoleObj = "tid1", "", trObj
	mds = mock.NewMockDataStore(mockCtrl)
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
	hc.DS = mds
	ret = hc.volumeSeriesRequestCreate(params)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	}

	// MOUNT works when volume state is valid
	for _, vsState := range []string{"BOUND", "PROVISIONED", "CONFIGURED"} {
		mockCtrl.Finish()
		tl.Flush()
		mockCtrl = gomock.NewController(t)
		mds = mock.NewMockDataStore(mockCtrl)
		ai.AccountID, ai.RoleObj = "", nil
		params.Payload.RequestedOperations = []string{"MOUNT"}
		params.Payload.VolumeSeriesID = models.ObjIDMutable(vsID)
		params.Payload.CompleteByTime = strfmt.DateTime(time.Now().Add(time.Hour))
		t.Logf("case: mount VS state is %s", vsState)
		vsObj.BoundClusterID = "clID"
		vsObj.VolumeSeriesState = vsState
		oN = mock.NewMockNodeOps(mockCtrl)
		oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
		oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
		oSP.EXPECT().Fetch(ctx, string(spObj.Meta.ID)).Return(spObj, nil)
		mds.EXPECT().OpsNode().Return(oN)
		oN.EXPECT().Fetch(ctx, string(params.Payload.NodeID)).Return(nodeObj, nil)
		mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).Times(2)
		oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
		oVR.EXPECT().Create(ctx, params.Payload).Return(nil, centrald.ErrorDbError)
		mds.EXPECT().OpsVolumeSeries().Return(oVS)
		oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
		hc.DS = mds
		cntLock = hc.cntLock
		cntUnlock = hc.cntUnlock
		assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(params) })
		assert.NotNil(ret)
		assert.Equal([]string{"MOUNT"}, params.Payload.RequestedOperations)
		mE, ok := ret.(*ops.VolumeSeriesRequestCreateDefault)
		if assert.True(ok) {
			assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
			assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
		}
		assert.Equal(cntLock+1, hc.cntLock)
		assert.Equal(cntUnlock+1, hc.cntUnlock)
	}

	// Create with RENAME op
	t.Log("case: Create with RENAME op")
	mockCtrl.Finish()
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	ai.AccountID, ai.TenantAccountID, ai.RoleObj = "account2", "tid1", rObj
	oSpec := params.Payload.VolumeSeriesCreateSpec
	params.Payload.RequestedOperations = []string{"RENAME"}
	params.Payload.VolumeSeriesID = models.ObjIDMutable(vsID)
	params.Payload.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{}
	params.Payload.VolumeSeriesCreateSpec.Name = "renamed"
	mds = mock.NewMockDataStore(mockCtrl)
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(1)
	oVR.EXPECT().Create(ctx, params.Payload).Return(nil, centrald.ErrorDbError)
	oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
		assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	}
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.EqualValues(ai.AccountID, params.Payload.VolumeSeriesCreateSpec.AccountID)
	assert.EqualValues(ai.TenantAccountID, params.Payload.VolumeSeriesCreateSpec.TenantAccountID)
	params.Payload.VolumeSeriesCreateSpec = oSpec
	tl.Flush()

	// Create with DELETE op
	t.Log("case: Create with DELETE op")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	params.Payload.RequestedOperations = []string{"DELETE"}
	params.Payload.VolumeSeriesID = models.ObjIDMutable(vsID)
	vsObj.BoundClusterID = ""
	vsObj.VolumeSeriesState = com.VolStateUnbound
	mds = mock.NewMockDataStore(mockCtrl)
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(1)
	oVR.EXPECT().Create(ctx, params.Payload).Return(nil, centrald.ErrorDbError)
	oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(params) })
	assert.NotNil(ret)
	assert.Equal([]string{"DELETE"}, params.Payload.RequestedOperations)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
		assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	}
	assert.Zero(params.Payload.ClusterID)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	params.Payload.ClusterID = "clID" //reset
	tl.Flush()

	// Create with NODE_DELETE op, not internal
	t.Log("case: Create with NODE_DELETE op, not internal")
	ai.RoleObj = rObj
	params.Payload.RequestedOperations = []string{"NODE_DELETE"}
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(params) })
	assert.NotNil(ret)
	assert.Equal([]string{"NODE_DELETE"}, params.Payload.RequestedOperations)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	}
	ai.RoleObj = nil // reset
	tl.Flush()

	// Create with NODE_DELETE op, no nodeId
	t.Log("case: Create with NODE_DELETE op, no nodeId")
	nodeID := models.ObjIDMutable(nodeObj.Meta.ID)
	params.Payload.NodeID = ""
	params.Payload.RequestedOperations = []string{"NODE_DELETE"}
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(params) })
	assert.NotNil(ret)
	assert.Equal([]string{"NODE_DELETE"}, params.Payload.RequestedOperations)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
		assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
		assert.Regexp("nodeId is required", *mE.Payload.Message)
	}
	tl.Flush()

	// Create with NODE_DELETE op, node not found
	t.Log("case: Create with NODE_DELETE op, node not found")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	params.Payload.NodeID = nodeID
	params.Payload.RequestedOperations = []string{"NODE_DELETE"}
	mds = mock.NewMockDataStore(mockCtrl)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, string(nodeID)).Return(nil, centrald.ErrorNotFound)
	mds.EXPECT().OpsNode().Return(oN)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(params) })
	assert.NotNil(ret)
	assert.Equal([]string{"NODE_DELETE"}, params.Payload.RequestedOperations)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
		assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
		assert.Regexp("invalid nodeId", *mE.Payload.Message)
	}
	tl.Flush()

	// Create with NODE_DELETE op, failure to check for duplicate VSR
	for _, state := range vra.ValidNodeStatesForNodeDelete() {
		t.Logf("case: Create with NODE_DELETE op, failure to check for duplicate VSR with node state %s", state)
		vrlParams := ops.VolumeSeriesRequestListParams{
			IsTerminated: swag.Bool(false),
			NodeID:       swag.String(string(params.Payload.NodeID)),
		}
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		params.Payload.RequestedOperations = []string{"NODE_DELETE"}
		nodeObj.State = state
		mds = mock.NewMockDataStore(mockCtrl)
		oN = mock.NewMockNodeOps(mockCtrl)
		oN.EXPECT().Fetch(ctx, string(nodeID)).Return(nodeObj, nil)
		mds.EXPECT().OpsNode().Return(oN)
		oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
		oVR.EXPECT().List(ctx, vrlParams).Return(nil, centrald.ErrorDbError)
		mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
		hc.DS = mds
		assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(params) })
		assert.NotNil(ret)
		assert.Equal([]string{"NODE_DELETE"}, params.Payload.RequestedOperations)
		mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
		if assert.True(ok) {
			assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
			assert.Regexp(centrald.ErrorDbError.M, *mE.Payload.Message)
		}
		tl.Flush()
	}

	// Create with NODE_DELETE op, duplicate VSR
	t.Log("case: Create with NODE_DELETE op, duplicate VSR")
	dupVSRs := []*models.VolumeSeriesRequest{
		&models.VolumeSeriesRequest{
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "id1",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"NODE_DELETE"},
				VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{
					VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
						AccountID:       "aid1",
						TenantAccountID: "tid1",
					},
				},
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					NodeID: nodeID,
				},
			},
		},
	}
	mockCtrl.Finish()
	vrlParams := ops.VolumeSeriesRequestListParams{
		IsTerminated: swag.Bool(false),
		NodeID:       swag.String(string(params.Payload.NodeID)),
	}
	mockCtrl = gomock.NewController(t)
	params.Payload.RequestedOperations = []string{"NODE_DELETE"}
	mds = mock.NewMockDataStore(mockCtrl)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, string(nodeID)).Return(nodeObj, nil)
	mds.EXPECT().OpsNode().Return(oN)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().List(ctx, vrlParams).Return(dupVSRs, nil)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(params) })
	assert.NotNil(ret)
	assert.Equal([]string{"NODE_DELETE"}, params.Payload.RequestedOperations)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorRequestInConflict.C, int(mE.Payload.Code))
		assert.Regexp("^"+centrald.ErrorRequestInConflict.M, *mE.Payload.Message)
		assert.Regexp("pending NODE_DELETE VSR for node "+nodeID, *mE.Payload.Message)
	}
	tl.Flush()

	// Create with NODE_DELETE op, fail in create, complete by time set
	t.Log("case: Create with NODE_DELETE op, complete by time set")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	ndParams := ops.VolumeSeriesRequestCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
		Payload:     &models.VolumeSeriesRequestCreateArgs{},
	}
	ndParams.Payload.RequestedOperations = []string{"NODE_DELETE"}
	ndParams.Payload.NodeID = nodeID
	ndParams.Payload.CompleteByTime = strfmt.DateTime(time.Now().Add(time.Hour))
	mds = mock.NewMockDataStore(mockCtrl)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, string(nodeID)).Return(nodeObj, nil)
	mds.EXPECT().OpsNode().Return(oN)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().List(ctx, vrlParams).Return([]*models.VolumeSeriesRequest{}, nil)
	oVR.EXPECT().Create(ctx, ndParams.Payload).Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).AnyTimes()
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(ndParams) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
		assert.Regexp("^"+centrald.ErrorDbError.M, *mE.Payload.Message)
	}
	if assert.NotNil(ndParams.Payload.VolumeSeriesCreateSpec) {
		assert.Equal(nodeObj.AccountID, ndParams.Payload.VolumeSeriesCreateSpec.AccountID)
		assert.Empty(ndParams.Payload.VolumeSeriesCreateSpec.TenantAccountID)
	}
	assert.Equal(util.DateTimeMaxUpperBound(), time.Time(ndParams.Payload.CompleteByTime))
	tl.Flush()

	// Create with VOL_DETACH op, invalid node state (MANAGED)
	t.Log("case: Create with VOL_DETACH op, invalid node state")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	params.Payload.RequestedOperations = []string{"VOL_DETACH"}
	nodeObj.State = com.NodeStateManaged
	mds = mock.NewMockDataStore(mockCtrl)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, string(nodeID)).Return(nodeObj, nil)
	mds.EXPECT().OpsNode().Return(oN)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(params) })
	assert.NotNil(ret)
	assert.Equal([]string{"VOL_DETACH"}, params.Payload.RequestedOperations)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
		assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
		assert.Regexp("invalid node state: "+com.NodeStateManaged, *mE.Payload.Message)
	}
	tl.Flush()

	// Create with VOL_DETACH op, missing params
	t.Log("case: Create with VOL_DETACH op, missing params")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	params.Payload.RequestedOperations = []string{"VOL_DETACH"}
	params.Payload.VolumeSeriesID = ""
	nodeObj.State = com.NodeStateTearDown
	mds = mock.NewMockDataStore(mockCtrl)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, string(nodeID)).Return(nodeObj, nil)
	mds.EXPECT().OpsNode().Return(oN)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(params) })
	assert.NotNil(ret)
	assert.Equal([]string{"VOL_DETACH"}, params.Payload.RequestedOperations)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
		assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
		assert.Regexp("syncCoordinatorID and volumeSeriesID must be specified", *mE.Payload.Message)
	}
	params.Payload.VolumeSeriesID = models.ObjIDMutable(vsID) // reset
	tl.Flush()

	// Create with VOL_DETACH op, wrong sync coordinator
	t.Log("case: Create with VOL_DETACH op, wrong sync coordinator")
	scID := params.Payload.SyncCoordinatorID
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	params.Payload.RequestedOperations = []string{"VOL_DETACH"}
	params.Payload.SyncCoordinatorID = models.ObjIDMutable(obj.Meta.ID)
	nodeObj.State = com.NodeStateTearDown
	mds = mock.NewMockDataStore(mockCtrl)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, string(nodeID)).Return(nodeObj, nil)
	mds.EXPECT().OpsNode().Return(oN)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Fetch(ctx, string(params.Payload.SyncCoordinatorID)).Return(nil, centrald.ErrorNotFound)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(params) })
	assert.NotNil(ret)
	assert.Equal([]string{"VOL_DETACH"}, params.Payload.RequestedOperations)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
		assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
		assert.Regexp("invalid syncCoordinatorId", *mE.Payload.Message)
	}
	tl.Flush()

	// Create with VOL_DETACH op, failure fetching VS
	t.Log("case: Create with VOL_DETACH op, failure fetching VS")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	params.Payload.RequestedOperations = []string{"VOL_DETACH"}
	params.Payload.SyncCoordinatorID = models.ObjIDMutable(obj.Meta.ID)
	mds = mock.NewMockDataStore(mockCtrl)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, string(nodeID)).Return(nodeObj, nil)
	mds.EXPECT().OpsNode().Return(oN)
	ndSyncVSR := &models.VolumeSeriesRequest{}
	ndSyncVSR.Meta = &models.ObjMeta{ID: models.ObjID(params.Payload.SyncCoordinatorID)}
	ndSyncVSR.RequestedOperations = []string{"NODE_DELETE"}
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Fetch(ctx, string(params.Payload.SyncCoordinatorID)).Return(ndSyncVSR, nil)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVS.EXPECT().Fetch(ctx, vsID).Return(nil, centrald.ErrorNotFound)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(params) })
	assert.NotNil(ret)
	assert.Equal([]string{"VOL_DETACH"}, params.Payload.RequestedOperations)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
		assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
		assert.Regexp("invalid volumeSeriesId", *mE.Payload.Message)
	}
	params.Payload.SyncCoordinatorID = scID // reset
	tl.Flush()

	// Create with VOL_DETACH op, fail create, complete by time set
	t.Log("case: Create with VOL_DETACH op, complete by time set")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	vdParams := ops.VolumeSeriesRequestCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
		Payload:     &models.VolumeSeriesRequestCreateArgs{},
	}
	vdParams.Payload.CompleteByTime = strfmt.DateTime(time.Now().Add(time.Hour))
	vdParams.Payload.RequestedOperations = []string{"VOL_DETACH"}
	vdParams.Payload.SyncCoordinatorID = models.ObjIDMutable(obj.Meta.ID)
	vdParams.Payload.NodeID = nodeID
	vdParams.Payload.VolumeSeriesID = models.ObjIDMutable(vsID)
	mds = mock.NewMockDataStore(mockCtrl)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, string(nodeID)).Return(nodeObj, nil)
	mds.EXPECT().OpsNode().Return(oN)
	ndSyncVSR = &models.VolumeSeriesRequest{}
	ndSyncVSR.Meta = &models.ObjMeta{ID: models.ObjID(vdParams.Payload.SyncCoordinatorID)}
	ndSyncVSR.RequestedOperations = []string{"NODE_DELETE"}
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Fetch(ctx, string(vdParams.Payload.SyncCoordinatorID)).Return(ndSyncVSR, nil)
	oVR.EXPECT().Create(ctx, vdParams.Payload).Return(nil, centrald.ErrorDbError)
	oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).AnyTimes()
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(vdParams) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
		assert.Regexp("^"+centrald.ErrorDbError.M, *mE.Payload.Message)
	}
	assert.Equal(util.DateTimeMaxUpperBound(), time.Time(vdParams.Payload.CompleteByTime))
	tl.Flush()

	t.Log("case: Create with DELETE op and rootStorageId")
	sObj := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta: &models.ObjMeta{ID: "s1"},
		},
		StorageMutable: models.StorageMutable{
			StorageState: &models.StorageStateMutable{
				AttachmentState: "ATTACHED",
				AttachedNodeID:  "n1",
			},
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	params.Payload.NodeID = ""
	params.Payload.RequestedOperations = []string{"DELETE"}
	params.Payload.VolumeSeriesID = models.ObjIDMutable(vsID)
	nodeObj.State = com.NodeStateManaged
	vsObj.VolumeSeriesState = com.VolStateProvisioned
	vsObj.BoundClusterID = "c1"
	vsObj.RootStorageID = "s1"
	mds = mock.NewMockDataStore(mockCtrl)
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oS := mock.NewMockStorageOps(mockCtrl)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, "n1").Return(nodeObj, nil)
	mds.EXPECT().OpsNode().Return(oN)
	mds.EXPECT().OpsStorage().Return(oS)
	oS.EXPECT().Fetch(ctx, "s1").Return(sObj, nil)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(1)
	oVR.EXPECT().Create(ctx, params.Payload).Return(nil, centrald.ErrorDbError)
	oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(params) })
	assert.NotNil(ret)
	assert.Equal([]string{"DELETE"}, params.Payload.RequestedOperations)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
		assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	}
	assert.EqualValues("c1", params.Payload.ClusterID)
	assert.EqualValues("n1", params.Payload.NodeID)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	params.Payload.ClusterID = "clID"                            // reset
	params.Payload.NodeID = models.ObjIDMutable(nodeObj.Meta.ID) // reset
	tl.Flush()
	mockCtrl.Finish()

	// UNBIND is very similar to DELETE
	t.Logf("case: UNBIND")
	tl.Flush()
	var vsUnbind *models.VolumeSeries
	testutils.Clone(vsObj, &vsUnbind)
	vsUnbind.VolumeSeriesState = com.VolStateProvisioned
	vsUnbind.LifecycleManagementData = &models.LifecycleManagementData{}
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	ai.AccountID, ai.RoleObj = "", nil
	params.Payload.RequestedOperations = []string{com.VolReqOpUnbind}
	params.Payload.VolumeSeriesID = models.ObjIDMutable(vsID)
	params.Payload.CompleteByTime = strfmt.DateTime(time.Now().Add(time.Hour))
	params.Payload.ClusterID = ""
	oS = mock.NewMockStorageOps(mockCtrl)
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, "n1").Return(nodeObj, nil)
	mds.EXPECT().OpsNode().Return(oN)
	mds.EXPECT().OpsStorage().Return(oS)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).Times(2)
	oS.EXPECT().Fetch(ctx, "s1").Return(sObj, nil)
	oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
	oVR.EXPECT().Create(ctx, params.Payload).Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	oVS.EXPECT().Fetch(ctx, vsID).Return(vsUnbind, nil)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
		assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	}
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.NotEmpty(params.Payload.ClusterID)
	assert.Equal(vsUnbind.BoundClusterID, params.Payload.ClusterID)
	assert.Equal(sObj.StorageState.AttachedNodeID, params.Payload.NodeID)
	tl.Flush()
	mockCtrl.Finish()

	// Create with ALLOCATE_CAPACITY op (success)
	t.Log("case: Create with ALLOCATE_CAPACITY op")
	mockCtrl = gomock.NewController(t)
	fakeSPAv := &fakeSPAChecks{}
	hc.spaValidator = fakeSPAv
	ai.AccountID, ai.TenantAccountID, ai.RoleObj = "tid1", "", trObj
	spaSpec := &models.ServicePlanAllocationCreateArgs{}
	spaSpec.AccountID = "tid1"
	spaSpec.AuthorizedAccountID = "account2"
	spaSpec.ClusterID = "clID"
	spaSpec.Messages = []*models.TimestampedString{}
	spaSpec.ServicePlanID = "spID"
	spaSpec.StorageFormula = "formula1"
	spaSpec.SystemTags = []string{"stag1"}
	spaSpec.Tags = []string{"tag1"}
	var acInParams, acCallParams ops.VolumeSeriesRequestCreateParams
	acInParams.HTTPRequest = requestWithAuthContext(ai)
	acInParams.Payload = &models.VolumeSeriesRequestCreateArgs{}
	acInParams.Payload.Creator = &models.Identity{
		AccountID:       models.ObjIDMutable(ai.AccountID),
		TenantAccountID: models.ObjID(ai.TenantAccountID),
	}
	acInParams.Payload.CompleteByTime = params.Payload.CompleteByTime
	acInParams.Payload.VolumeSeriesCreateSpec = nil
	acInParams.Payload.ServicePlanAllocationCreateSpec = spaSpec
	acInParams.Payload.RequestedOperations = []string{"ALLOCATE_CAPACITY"}
	testutils.Clone(acInParams, &acCallParams)
	acCallParams.Payload.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{}
	acCallParams.Payload.VolumeSeriesCreateSpec.AccountID = spaSpec.AuthorizedAccountID
	acCallParams.Payload.VolumeSeriesCreateSpec.TenantAccountID = spaSpec.AccountID
	acCallParams.Payload.ClusterID = "clID"
	mds = mock.NewMockDataStore(mockCtrl)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	cM := newVolumeSeriesRequestMatcher(t, acCallParams.Payload)
	oVR.EXPECT().Create(ctx, cM).Return(obj, nil)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(acInParams) })
	assert.NotNil(ret)
	_, ok = ret.(*ops.VolumeSeriesRequestCreateCreated)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	spaObj := &models.ServicePlanAllocation{
		ServicePlanAllocationCreateOnce: spaSpec.ServicePlanAllocationCreateOnce,
		ServicePlanAllocationMutable: models.ServicePlanAllocationMutable{
			ServicePlanAllocationCreateMutable: spaSpec.ServicePlanAllocationCreateMutable,
		},
	}
	assert.Equal(spaObj, fakeSPAv.inObj)
	if assert.NotNil(acInParams.Payload.VolumeSeriesCreateSpec) {
		assert.EqualValues("account2", acInParams.Payload.VolumeSeriesCreateSpec.AccountID)
		assert.EqualValues("tid1", acInParams.Payload.VolumeSeriesCreateSpec.TenantAccountID)
	}
	tl.Flush()
	mockCtrl.Finish()

	// ALLOCATE_CAPACITY operation and unauthorized
	mockCtrl = gomock.NewController(t)
	t.Log("case: Create with ALLOCATE_CAPACITY op, no tenantAdmin capability")
	fakeSPAv.retErr = centrald.ErrorUnauthorizedOrForbidden
	ai.AccountID, ai.TenantAccountID, ai.RoleObj = "account2", "tid1", rObj
	hc.DS = mds
	ret = hc.volumeSeriesRequestCreate(acInParams)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	}
	fakeSPAv.retErr = nil
	tl.Flush()
	mockCtrl.Finish()

	// DELETE_SPA success
	mockCtrl = gomock.NewController(t)
	t.Log("case: Create with DELETE_SPA op")
	params.Payload.NodeID = ""
	spaObj = &models.ServicePlanAllocation{
		ServicePlanAllocationAllOf0: models.ServicePlanAllocationAllOf0{
			Meta: &models.ObjMeta{ID: "spa1"},
		},
		ServicePlanAllocationCreateOnce: models.ServicePlanAllocationCreateOnce{
			AccountID:           "tid1",
			AuthorizedAccountID: "account2",
			ClusterID:           "cl1",
			ServicePlanID:       "awesome",
		},
		ServicePlanAllocationMutable: models.ServicePlanAllocationMutable{
			ServicePlanAllocationMutableAllOf0: models.ServicePlanAllocationMutableAllOf0{
				ReservableCapacityBytes: swag.Int64(2 * units.GiB),
			},
			ServicePlanAllocationCreateMutable: models.ServicePlanAllocationCreateMutable{
				TotalCapacityBytes: swag.Int64(2 * units.GiB),
				ReservationState:   "OK",
				StorageFormula:     "formulaic",
				StorageReservations: map[string]models.StorageTypeReservation{
					"prov1": models.StorageTypeReservation{
						NumMirrors: 1,
						SizeBytes:  swag.Int64(2 * units.GiB),
					},
				},
			},
		},
	}
	ai.AccountID, ai.TenantAccountID, ai.RoleObj = "tid1", "", trObj
	testutils.Clone(&params, &acInParams)
	acInParams.HTTPRequest = requestWithAuthContext(ai)
	acInParams.Payload.VolumeSeriesCreateSpec = nil
	acInParams.Payload.ServicePlanAllocationCreateSpec = nil
	acInParams.Payload.RequestedOperations = []string{"DELETE_SPA"}
	acInParams.Payload.ServicePlanAllocationID = "spa1"
	mds = mock.NewMockDataStore(mockCtrl)
	oSPA := mock.NewMockServicePlanAllocationOps(mockCtrl)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
	oSPA.EXPECT().Fetch(ctx, "spa1").Return(spaObj, nil)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	oVR.EXPECT().Create(ctx, acInParams.Payload).Return(obj, nil)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(acInParams) })
	assert.NotNil(ret)
	_, ok = ret.(*ops.VolumeSeriesRequestCreateCreated)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Empty(acInParams.Payload.ServicePlanAllocationID)
	if assert.NotNil(acInParams.Payload.ServicePlanAllocationCreateSpec) {
		var tmpSPA *models.ServicePlanAllocation
		testutils.Clone(spaObj, &tmpSPA)
		tmpSPA.TotalCapacityBytes = swag.Int64(0)
		assert.Equal(tmpSPA.ServicePlanAllocationCreateOnce, acInParams.Payload.ServicePlanAllocationCreateSpec.ServicePlanAllocationCreateOnce)
		assert.Equal(tmpSPA.ServicePlanAllocationCreateMutable, acInParams.Payload.ServicePlanAllocationCreateSpec.ServicePlanAllocationCreateMutable)
	}
	if assert.NotNil(acInParams.Payload.VolumeSeriesCreateSpec) {
		assert.Equal(spaObj.AccountID, acInParams.Payload.VolumeSeriesCreateSpec.TenantAccountID)
		assert.Equal(spaObj.AuthorizedAccountID, acInParams.Payload.VolumeSeriesCreateSpec.AccountID)
	}
	tl.Flush()
	mockCtrl.Finish()

	// DELETE_SPA unauthorized
	mockCtrl = gomock.NewController(t)
	t.Log("case: Create with DELETE_SPA op, no tenantAdmin capability")
	ai.AccountID, ai.TenantAccountID, ai.RoleObj = "account2", "tid1", rObj
	acInParams.Payload.ServicePlanAllocationID = "spa1"
	mds = mock.NewMockDataStore(mockCtrl)
	oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
	oSPA.EXPECT().Fetch(ctx, "spa1").Return(spaObj, nil)
	hc.DS = mds
	ret = hc.volumeSeriesRequestCreate(acInParams)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	}
	tl.Flush()
	mockCtrl.Finish()

	// DELETE_SPA db error
	mockCtrl = gomock.NewController(t)
	t.Log("case: Create with DELETE_SPA op, db error")
	mds = mock.NewMockDataStore(mockCtrl)
	oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
	oSPA.EXPECT().Fetch(ctx, "spa1").Return(nil, centrald.ErrorDbError)
	hc.DS = mds
	ret = hc.volumeSeriesRequestCreate(acInParams)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
		assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	}
	tl.Flush()
	mockCtrl.Finish()

	// DELETE_SPA not found
	mockCtrl = gomock.NewController(t)
	t.Log("case: Create with DELETE_SPA op, SPA not found")
	mds = mock.NewMockDataStore(mockCtrl)
	oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA)
	oSPA.EXPECT().Fetch(ctx, "spa1").Return(nil, centrald.ErrorNotFound)
	hc.DS = mds
	ret = hc.volumeSeriesRequestCreate(acInParams)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
		assert.Regexp(centrald.ErrorMissing.M, *mE.Payload.Message)
	}
	tl.Flush()
	mockCtrl.Finish()

	// Create CG_SNAPSHOT_CREATE
	t.Log("case: Create with CG_SNAPSHOT_CREATE op")
	mockCtrl = gomock.NewController(t)
	ai.AccountID, ai.TenantAccountID, ai.RoleObj = "aID", "tid1", rObj
	cgSCParams := ops.VolumeSeriesRequestCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
		Payload:     &models.VolumeSeriesRequestCreateArgs{},
	}
	cgSCParams.Payload.RequestedOperations = []string{"CG_SNAPSHOT_CREATE"}
	cgSCParams.Payload.ConsistencyGroupID = models.ObjIDMutable(cgObj.Meta.ID)
	cgSCParams.Payload.ClusterID = models.ObjIDMutable(clusterObj.Meta.ID)
	cgSCLParams := ops.VolumeSeriesRequestListParams{
		ConsistencyGroupID: swag.String(string(cgSCParams.Payload.ConsistencyGroupID)),
		ClusterID:          swag.String(string(cgSCParams.Payload.ClusterID)),
		IsTerminated:       swag.Bool(false),
	}
	mds = mock.NewMockDataStore(mockCtrl)
	oCG := mock.NewMockConsistencyGroupOps(mockCtrl)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	mds.EXPECT().OpsCluster().Return(oCL)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(2)
	oCG.EXPECT().Fetch(ctx, string(cgObj.Meta.ID)).Return(cgObj, nil)
	oCL.EXPECT().Fetch(ctx, string(clusterObj.Meta.ID)).Return(clusterObj, nil)
	oVR.EXPECT().Create(ctx, cgSCParams.Payload).Return(nil, centrald.ErrorDbError) // note fake failure
	oVR.EXPECT().Count(ctx, cgSCLParams, uint(1)).Return(0, nil)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(cgSCParams) })
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
		assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	}
	if assert.NotNil(cgSCParams.Payload.VolumeSeriesCreateSpec) {
		assert.EqualValues(ai.AccountID, cgSCParams.Payload.VolumeSeriesCreateSpec.AccountID)
		assert.EqualValues(ai.TenantAccountID, cgSCParams.Payload.VolumeSeriesCreateSpec.TenantAccountID)
	}
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	// CG_SNAPSHOT_CREATE operation and unauthorized
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: Create with CG_SNAPSHOT_CREATE op, no volumeSeriesOwner capability on CG")
	ai.AccountID, ai.TenantAccountID, ai.RoleObj = "tid1", "", trObj
	mds = mock.NewMockDataStore(mockCtrl)
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	oCG.EXPECT().Fetch(ctx, string(cgObj.Meta.ID)).Return(cgObj, nil)
	hc.DS = mds
	ret = hc.volumeSeriesRequestCreate(cgSCParams)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	}
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: Create with CG_SNAPSHOT_CREATE op, no CSPDomainUsageCap on Cluster")
	ai.AccountID, ai.TenantAccountID, ai.RoleObj = "account2", "tid1", badRoleObj
	mds = mock.NewMockDataStore(mockCtrl)
	oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	oCG.EXPECT().Fetch(ctx, string(cgObj.Meta.ID)).Return(cgObj, nil)
	hc.DS = mds
	ret = hc.volumeSeriesRequestCreate(cgSCParams)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	}
	tl.Flush()

	// Create VOL_SNAPSHOT_CREATE (direct)
	var vscParams ops.VolumeSeriesRequestCreateParams
	var vscParams2 ops.VolumeSeriesRequestCreateParams
	var vscVol *models.VolumeSeries
	var vscLParams1 ops.VolumeSeriesRequestListParams
	for _, volState := range []string{com.VolStateProvisioned, com.VolStateConfigured, com.VolStateInUse} {
		mockCtrl.Finish()
		t.Logf("case: Create with VOL_SNAPSHOT_CREATE, no syncCoordinator state=%s", volState)
		mockCtrl = gomock.NewController(t)
		ai.AccountID, ai.TenantAccountID, ai.RoleObj = "account2", "tid1", rObj
		testutils.Clone(vsObj, &vscVol)
		vscVol.BoundClusterID = "cl1"
		vscVol.BoundCspDomainID = "dom1"
		vscVol.ConsistencyGroupID = models.ObjIDMutable(cgObj.Meta.ID)
		vscVol.ConfiguredNodeID = "nodeH"
		vscVol.VolumeSeriesState = volState
		if volState == com.VolStateInUse {
			vscVol.Mounts = []*models.Mount{
				&models.Mount{
					SnapIdentifier: "pit1",
					MountedNodeID:  "nodeP",
					MountState:     "MOUNTED",
				},
				&models.Mount{
					SnapIdentifier: "HEAD",
					MountedNodeID:  "nodeH",
					MountState:     "MOUNTED",
				},
			}
		} else {
			vscVol.Mounts = nil
			if volState == com.VolStateProvisioned {
				vscVol.ConfiguredNodeID = ""
			}
		}
		vscParams = ops.VolumeSeriesRequestCreateParams{
			HTTPRequest: requestWithAuthContext(ai),
			Payload:     &models.VolumeSeriesRequestCreateArgs{},
		}
		vscParams.Payload.RequestedOperations = []string{"VOL_SNAPSHOT_CREATE"}
		if volState == com.VolStateProvisioned {
			vscParams.Payload.RequestedOperations = []string{"CONFIGURE", "VOL_SNAPSHOT_CREATE"}
		}
		vscParams.Payload.VolumeSeriesID = models.ObjIDMutable(vscVol.Meta.ID)
		testutils.Clone(&vscParams, &vscParams2)
		vscParams2.Payload.Creator = &models.Identity{
			AccountID:       models.ObjIDMutable(ai.AccountID),
			TenantAccountID: models.ObjID(ai.TenantAccountID),
			UserID:          models.ObjIDMutable(ai.UserID),
		}
		vscParams2.Payload.NodeID = "nodeH"
		vscParams2.Payload.ClusterID = vscVol.BoundClusterID
		vscParams2.Payload.ConsistencyGroupID = vscVol.ConsistencyGroupID
		vscParams2.Payload.Snapshot = &models.SnapshotData{
			PitIdentifier: fakeUUID,
		}
		vscParams2.Payload.SnapIdentifier = fakeUUID
		vscParams2.Payload.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{}
		vscParams2.Payload.VolumeSeriesCreateSpec.AccountID = vscVol.AccountID
		vscParams2.Payload.VolumeSeriesCreateSpec.TenantAccountID = vscVol.TenantAccountID
		vscLParams1 = ops.VolumeSeriesRequestListParams{
			VolumeSeriesID: swag.String(string(vscVol.Meta.ID)),
			IsTerminated:   swag.Bool(false),
		}
		vscLParams2 := ops.VolumeSeriesRequestListParams{
			ConsistencyGroupID: swag.String(string(vscVol.ConsistencyGroupID)),
			IsTerminated:       swag.Bool(false),
		}
		vscNonConflictingVSRS := []*models.VolumeSeriesRequest{&models.VolumeSeriesRequest{}}
		vscNonConflictingVSRS[0].RequestedOperations = []string{"CG_SNAPSHOT_CREATE"}
		vscNonConflictingVSRS[0].ClusterID = vscVol.BoundClusterID + "foo" // different cluster
		fops.RetAccountFetchObj = aObj
		fops.RetAccountFetchErr = nil
		vscParams2.Payload.ProtectionDomainID = models.ObjIDMutable("pdID")
		oN = mock.NewMockNodeOps(mockCtrl)
		oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
		oVS.EXPECT().Fetch(ctx, string(vscVol.Meta.ID)).Return(vscVol, nil)
		oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
		oVR.EXPECT().List(ctx, vscLParams1).Return([]*models.VolumeSeriesRequest{}, nil)
		oVR.EXPECT().List(ctx, vscLParams2).Return(vscNonConflictingVSRS, nil)
		oVR.EXPECT().Create(ctx, vscParams2.Payload).Return(nil, centrald.ErrorDbError) // note fake failure
		mds = mock.NewMockDataStore(mockCtrl)
		mds.EXPECT().OpsVolumeSeries().Return(oVS)
		mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(2)
		vscParams.Payload.NodeID = "nodeH"
		mds.EXPECT().OpsNode().Return(oN)
		var nClone *models.Node
		testutils.Clone(nodeObj, &nClone)
		nClone.ClusterID = vscVol.BoundClusterID
		oN.EXPECT().Fetch(ctx, string(vscParams.Payload.NodeID)).Return(nClone, nil)
		hc.DS = mds
		cntLock = hc.cntLock
		cntUnlock = hc.cntUnlock
		assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(vscParams) })
		mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
		if assert.True(ok) {
			assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
			assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
		}
		assert.Equal(cntLock+1, hc.cntLock)
		assert.Equal(cntUnlock+1, hc.cntUnlock)
		tl.Flush()
	}

	// Create VOL_SNAPSHOT_CREATE (indirect)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: Create with VOL_SNAPSHOT_CREATE, with syncCoordinator, state=IN_USE")
	vscSyncVSR := &models.VolumeSeriesRequest{}
	vscSyncVSR.Meta = &models.ObjMeta{ID: "syncVSR"}
	vscSyncVSR.RequestedOperations = []string{"CG_SNAPSHOT_CREATE"}
	vscParams = ops.VolumeSeriesRequestCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
		Payload:     &models.VolumeSeriesRequestCreateArgs{},
	}
	vscParams.Payload.RequestedOperations = []string{"VOL_SNAPSHOT_CREATE"}
	vscParams.Payload.VolumeSeriesID = models.ObjIDMutable(vscVol.Meta.ID)
	vscParams.Payload.SyncCoordinatorID = models.ObjIDMutable(vscSyncVSR.Meta.ID)
	testutils.Clone(&vscParams, &vscParams2)
	vscParams2.Payload.Creator = &models.Identity{
		AccountID:       models.ObjIDMutable(ai.AccountID),
		TenantAccountID: models.ObjID(ai.TenantAccountID),
		UserID:          models.ObjIDMutable(ai.UserID),
	}
	vscParams2.Payload.NodeID = "nodeH"
	vscParams2.Payload.ClusterID = vscVol.BoundClusterID
	vscParams2.Payload.ConsistencyGroupID = vscVol.ConsistencyGroupID
	vscParams2.Payload.Snapshot = &models.SnapshotData{
		PitIdentifier: fakeUUID,
	}
	vscParams2.Payload.SnapIdentifier = "syncVSR"
	vscParams2.Payload.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{}
	vscParams2.Payload.VolumeSeriesCreateSpec.AccountID = vscVol.AccountID
	vscParams2.Payload.VolumeSeriesCreateSpec.TenantAccountID = vscVol.TenantAccountID
	fops.RetAccountFetchObj = aObj
	fops.RetAccountFetchErr = nil
	vscParams2.Payload.ProtectionDomainID = models.ObjIDMutable("pdID")
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVS.EXPECT().Fetch(ctx, string(vscVol.Meta.ID)).Return(vscVol, nil)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().List(ctx, vscLParams1).Return([]*models.VolumeSeriesRequest{}, nil)
	oVR.EXPECT().Fetch(ctx, string(vscParams.Payload.SyncCoordinatorID)).Return(vscSyncVSR, nil)
	oVR.EXPECT().Create(ctx, vscParams2.Payload).Return(nil, centrald.ErrorDbError) // note fake failure
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(2)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, "nodeH").Return(nodeObj, nil)
	mds.EXPECT().OpsNode().Return(oN)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(vscParams) })
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
		assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	}
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	// Create VOL_SNAPSHOT_RESTORE (mounted)
	t.Log("case: Create with VOL_SNAPSHOT_RESTORE, mounted, no syncCoordinator")
	tl.Flush()
	var vsrVol *models.VolumeSeries
	testutils.Clone(vsObj, &vsrVol)
	vsrVol.VolumeSeriesState = "IN_USE"
	vsrVol.SizeBytes = swag.Int64(3 * units.GiB)
	vsrVol.Mounts = []*models.Mount{
		&models.Mount{
			SnapIdentifier: "HEAD",
			MountedNodeID:  "nodeH",
			MountState:     "MOUNTED",
		},
	}
	vsrParams := ops.VolumeSeriesRequestCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
		Payload:     &models.VolumeSeriesRequestCreateArgs{},
	}
	vsrParams.Payload.RequestedOperations = []string{"VOL_SNAPSHOT_RESTORE"}
	vsrParams.Payload.VolumeSeriesID = models.ObjIDMutable(vsrVol.Meta.ID)
	vsrParams.Payload.Snapshot = &models.SnapshotData{
		SnapIdentifier: "snapid",
		PitIdentifier:  "pitid",
		Locations: []*models.SnapshotLocation{
			&models.SnapshotLocation{CspDomainID: "ps1"},
		},
		VolumeSeriesID: "sourceVS",
		SizeBytes:      vsrVol.SizeBytes,
	}
	mockCtrl = gomock.NewController(t)
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVS.EXPECT().Fetch(ctx, string(vsrVol.Meta.ID)).Return(vsrVol, nil)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
	oVR.EXPECT().Create(ctx, vsrParams.Payload).Return(retObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(2)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.Nil(vsrParams.Payload.VolumeSeriesCreateSpec)
	ret = hc.volumeSeriesRequestCreate(vsrParams)
	assert.NotNil(ret)
	_, ok = ret.(*ops.VolumeSeriesRequestCreateCreated)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	if assert.NotNil(vsrParams.Payload.VolumeSeriesCreateSpec) {
		assert.EqualValues("account2", vsrParams.Payload.VolumeSeriesCreateSpec.AccountID)
		assert.EqualValues("tid1", vsrParams.Payload.VolumeSeriesCreateSpec.TenantAccountID)
	}
	tl.Flush()
	mockCtrl.Finish()

	// Create CREATE_FROM_SNAPSHOT (with SnapshotID)
	mockCtrl = gomock.NewController(t)
	t.Log("case: Create with CREATE_FROM_SNAPSHOT using SnapshotID, other VSRs exist for source vol")
	var cfsVol *models.VolumeSeries
	testutils.Clone(vsObj, &cfsVol)
	sizeBytes := int64(3 * units.GiB)
	snapshotData = &models.SnapshotData{
		SnapIdentifier:     "snapid",
		PitIdentifier:      "pitid",
		ProtectionDomainID: "pdId",
		Locations: []*models.SnapshotLocation{
			&models.SnapshotLocation{CspDomainID: "ps1"},
		},
		VolumeSeriesID: models.ObjIDMutable(cfsVol.Meta.ID),
		SizeBytes:      swag.Int64(sizeBytes),
	}
	cfsParams := ops.VolumeSeriesRequestCreateParams{ // input params
		HTTPRequest: requestWithAuthContext(ai),
		Payload:     &models.VolumeSeriesRequestCreateArgs{},
	}
	cfsParams.Payload.RequestedOperations = []string{"CREATE_FROM_SNAPSHOT"}
	cfsParams.Payload.SnapshotID = "snapshotObjectID"
	cfsParams.Payload.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{}
	cfsParams.Payload.VolumeSeriesCreateSpec.Name = "name"
	cfsParams.Payload.NodeID = models.ObjIDMutable(nodeObj.Meta.ID)
	cfsParams.Payload.ConsistencyGroupID = models.ObjIDMutable(cgObj.Meta.ID)
	var cfsParams2 *ops.VolumeSeriesRequestCreateParams // cooked params
	testutils.Clone(cfsParams, &cfsParams2)
	cfsParams2.Payload.Creator = &models.Identity{
		AccountID:       models.ObjIDMutable(ai.AccountID),
		TenantAccountID: models.ObjID(ai.TenantAccountID),
		UserID:          models.ObjIDMutable(ai.UserID),
	}
	cfsParams2.Payload.Snapshot = snapshotData
	cfsParams2.Payload.ProtectionDomainID = snapshotData.ProtectionDomainID
	cfsParams2.Payload.VolumeSeriesID = snapshotData.VolumeSeriesID
	cfsParams2.Payload.VolumeSeriesCreateSpec.SizeBytes = swag.Int64(sizeBytes)
	cfsParams2.Payload.VolumeSeriesCreateSpec.AccountID = cfsVol.AccountID
	cfsParams2.Payload.VolumeSeriesCreateSpec.TenantAccountID = aObj.TenantAccountID // added by validator
	cfsParams2.Payload.VolumeSeriesCreateSpec.ServicePlanID = cfsVol.ServicePlanID
	cfsParams2.Payload.ClusterID = nodeObj.ClusterID
	lM := newVolumeSeriesRequestMatcher(t, ops.VolumeSeriesRequestListParams{VolumeSeriesID: swag.String(string(cfsVol.Meta.ID)), IsTerminated: swag.Bool(false)})
	sourceVSRs := []*models.VolumeSeriesRequest{
		&models.VolumeSeriesRequest{},
	}
	sourceVSRs[0].Meta = &models.ObjMeta{ID: "otherVSR"}
	sourceVSRs[0].RequestedOperations = []string{"UNMOUNT"}
	lSM := newSnapshotMatcher(t, sops.SnapshotListParams{VolumeSeriesID: swag.String(string(cfsVol.Meta.ID)), SnapIdentifier: swag.String("snapid")})
	oN = mock.NewMockNodeOps(mockCtrl)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, string(cfsParams.Payload.NodeID)).Return(nodeObj, nil)
	oSP.EXPECT().Fetch(ctx, string(cfsParams2.Payload.VolumeSeriesCreateSpec.ServicePlanID)).Return(spObj, nil)
	oVS.EXPECT().Fetch(ctx, string(cfsVol.Meta.ID)).Return(cfsVol, nil)
	oVR.EXPECT().List(ctx, lM).Return(sourceVSRs, nil)
	cM = newVolumeSeriesRequestMatcher(t, cfsParams2.Payload)
	oVR.EXPECT().Create(ctx, cM).Return(retObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(2)
	hc.DS = mds
	fops = &fakeOps{}
	hc.ops = fops
	fops.InSnapshotFetchDataID = ""
	fops.InSnapshotFetchDataAi = nil
	fops.RetSnapshotFetchDataObj = snapshotData
	fops.RetSnapshotFetchDataErr = nil
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	ret = hc.volumeSeriesRequestCreate(cfsParams)
	assert.NotNil(ret)
	_, ok = ret.(*ops.VolumeSeriesRequestCreateCreated)
	assert.True(ok)
	assert.Equal("snapshotObjectID", fops.InSnapshotFetchDataID)
	assert.Equal(ai, fops.InSnapshotFetchDataAi)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()
	mockCtrl.Finish()

	// CREATE_FROM_SNAPSHOT (with SnapshotData)
	mockCtrl = gomock.NewController(t)
	t.Log("case: Create with CREATE_FROM_SNAPSHOT using SnapshotData, other VSRs exist for source vol")
	testutils.Clone(vsObj, &cfsVol)
	now := time.Now()
	snapList := []*models.Snapshot{
		&models.Snapshot{
			SnapshotAllOf0: models.SnapshotAllOf0{
				Meta: &models.ObjMeta{
					ID:      "id1",
					Version: 1,
				},
				AccountID:          models.ObjID(ai.AccountID),
				ConsistencyGroupID: "c-g-1",
				PitIdentifier:      "pitid",
				ProtectionDomainID: "pdId",
				SizeBytes:          sizeBytes,
				SnapIdentifier:     "snapid",
				SnapTime:           strfmt.DateTime(now),
				TenantAccountID:    models.ObjID(ai.TenantAccountID),
				VolumeSeriesID:     models.ObjIDMutable(cfsVol.Meta.ID),
			},
			SnapshotMutable: models.SnapshotMutable{
				DeleteAfterTime: strfmt.DateTime(now.Add(90 * 24 * time.Hour)),
				Locations: map[string]models.SnapshotLocation{
					"ps1": {CreationTime: strfmt.DateTime(now), CspDomainID: "ps1"},
				},
			},
		},
	}
	cfsParams = ops.VolumeSeriesRequestCreateParams{ // input params
		HTTPRequest: requestWithAuthContext(ai),
		Payload:     &models.VolumeSeriesRequestCreateArgs{},
	}
	cfsParams.Payload.RequestedOperations = []string{"CREATE_FROM_SNAPSHOT"}
	cfsParams.Payload.Snapshot = &models.SnapshotData{
		SnapIdentifier: "snapid",
		VolumeSeriesID: models.ObjIDMutable(cfsVol.Meta.ID),
	}
	cfsParams.Payload.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{}
	cfsParams.Payload.VolumeSeriesCreateSpec.Name = "name"
	cfsParams.Payload.NodeID = models.ObjIDMutable(nodeObj.Meta.ID)
	cfsParams.Payload.ConsistencyGroupID = models.ObjIDMutable(vsObj.ConsistencyGroupID)
	testutils.Clone(cfsParams, &cfsParams2)
	cfsParams2.Payload.Creator = &models.Identity{
		AccountID:       models.ObjIDMutable(ai.AccountID),
		TenantAccountID: models.ObjID(ai.TenantAccountID),
		UserID:          models.ObjIDMutable(ai.UserID),
	}
	cfsParams2.Payload.VolumeSeriesID = cfsParams.Payload.Snapshot.VolumeSeriesID
	cfsParams2.Payload.VolumeSeriesCreateSpec.SizeBytes = swag.Int64(sizeBytes)
	cfsParams2.Payload.VolumeSeriesCreateSpec.AccountID = cfsVol.AccountID
	cfsParams2.Payload.VolumeSeriesCreateSpec.TenantAccountID = aObj.TenantAccountID // added by validator
	cfsParams2.Payload.VolumeSeriesCreateSpec.ServicePlanID = cfsVol.ServicePlanID
	cfsParams2.Payload.ClusterID = nodeObj.ClusterID
	cfsParams2.Payload.ProtectionDomainID = snapList[0].ProtectionDomainID
	lM = newVolumeSeriesRequestMatcher(t, ops.VolumeSeriesRequestListParams{VolumeSeriesID: swag.String(string(cfsVol.Meta.ID)), IsTerminated: swag.Bool(false)})
	lSM = newSnapshotMatcher(t, sops.SnapshotListParams{VolumeSeriesID: swag.String(string(cfsVol.Meta.ID)), SnapIdentifier: swag.String("snapid")})
	oN = mock.NewMockNodeOps(mockCtrl)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSnap := mock.NewMockSnapshotOps(mockCtrl)
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, string(cfsParams.Payload.NodeID)).Return(nodeObj, nil)
	oSP.EXPECT().Fetch(ctx, string(cfsParams2.Payload.VolumeSeriesCreateSpec.ServicePlanID)).Return(spObj, nil)
	oVS.EXPECT().Fetch(ctx, string(cfsVol.Meta.ID)).Return(cfsVol, nil)
	oVR.EXPECT().List(ctx, lM).Return(sourceVSRs, nil)
	oSnap.EXPECT().List(ctx, lSM).Return(snapList, nil)
	oVR.EXPECT().Create(ctx, cfsParams2.Payload).Return(retObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN)
	mds.EXPECT().OpsSnapshot().Return(oSnap)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(2)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	ret = hc.volumeSeriesRequestCreate(cfsParams)
	assert.NotNil(ret)
	_, ok = ret.(*ops.VolumeSeriesRequestCreateCreated)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()
	mockCtrl.Finish()

	// CHANGE_CAPACITY op with both sizeBytes and spaAdditionalBytes
	mockCtrl = gomock.NewController(t)
	t.Log("case: CHANGE_CAPACITY op")
	testutils.Clone(vsObj, &vscVol)
	vscVol.VolumeSeriesState = com.VolStateBound // suitable state for both properties
	vscVol.SizeBytes = swag.Int64(int64(units.GiB))
	ai.AccountID, ai.TenantAccountID, ai.RoleObj = "account2", "tid1", rObj
	oSpec = params.Payload.VolumeSeriesCreateSpec
	params.Payload.RequestedOperations = []string{"CHANGE_CAPACITY"}
	params.Payload.VolumeSeriesID = models.ObjIDMutable(vsID)
	params.Payload.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{}
	params.Payload.VolumeSeriesCreateSpec.SizeBytes = swag.Int64(swag.Int64Value(vscVol.SizeBytes) * 2)
	params.Payload.VolumeSeriesCreateSpec.SpaAdditionalBytes = swag.Int64(0)
	mds = mock.NewMockDataStore(mockCtrl)
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	oVS.EXPECT().Fetch(ctx, vsID).Return(vscVol, nil)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(1)
	oVR.EXPECT().Create(ctx, params.Payload).Return(nil, centrald.ErrorDbError)
	oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
		assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	}
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.EqualValues(ai.AccountID, params.Payload.VolumeSeriesCreateSpec.AccountID)
	assert.EqualValues(ai.TenantAccountID, params.Payload.VolumeSeriesCreateSpec.TenantAccountID)
	params.Payload.VolumeSeriesCreateSpec = oSpec
	tl.Flush()

	// CHANGE_CAPACITY op with cache and only spaAdditionalBytes
	mockCtrl = gomock.NewController(t)
	t.Log("case: CHANGE_CAPACITY with cache")
	testutils.Clone(vsObj, &vscVol)
	vscVol.VolumeSeriesState = com.VolStateInUse
	vscVol.BoundClusterID = "cl1"
	vscVol.CacheAllocations = map[string]models.CacheAllocation{
		"nodeH": models.CacheAllocation{RequestedSizeBytes: swag.Int64(1)},
	}
	vscVol.ConfiguredNodeID = "nodeH"
	vsrVol.Mounts = []*models.Mount{
		&models.Mount{SnapIdentifier: "HEAD", MountedNodeID: "nodeH", MountState: "MOUNTED"},
	}
	vscVol.SizeBytes = swag.Int64(int64(units.GiB))
	ai.AccountID, ai.TenantAccountID, ai.RoleObj = "account2", "tid1", rObj
	oSpec = params.Payload.VolumeSeriesCreateSpec
	params.Payload.ClusterID = ""
	params.Payload.NodeID = ""
	params.Payload.RequestedOperations = []string{"CHANGE_CAPACITY"}
	params.Payload.VolumeSeriesID = models.ObjIDMutable(vsID)
	params.Payload.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{}
	params.Payload.VolumeSeriesCreateSpec.SpaAdditionalBytes = swag.Int64(units.GiB)
	mds = mock.NewMockDataStore(mockCtrl)
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	oVS.EXPECT().Fetch(ctx, vsID).Return(vscVol, nil)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(1)
	oVR.EXPECT().Create(ctx, params.Payload).Return(nil, centrald.ErrorDbError)
	oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
	oN = mock.NewMockNodeOps(mockCtrl)
	oN.EXPECT().Fetch(ctx, "nodeH").Return(nodeObj, nil)
	mds.EXPECT().OpsNode().Return(oN)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
		assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	}
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.EqualValues(ai.AccountID, params.Payload.VolumeSeriesCreateSpec.AccountID)
	assert.EqualValues(ai.TenantAccountID, params.Payload.VolumeSeriesCreateSpec.TenantAccountID)
	assert.EqualValues("cl1", params.Payload.ClusterID)
	assert.EqualValues("nodeH", params.Payload.NodeID)
	params.Payload.ClusterID = "clID"                            // reset
	params.Payload.NodeID = models.ObjIDMutable(nodeObj.Meta.ID) // reset
	params.Payload.VolumeSeriesCreateSpec = oSpec
	tl.Flush()

	// Auth error
	t.Log("case: auth internal error")
	tl.Flush()
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	// PUBLISH works when volume state is valid
	vsState := "BOUND"
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Logf("case: publish VS state is %s", vsState)
	mds = mock.NewMockDataStore(mockCtrl)
	ai.AccountID, ai.RoleObj = "", nil
	params.Payload.NodeID = ""
	params.Payload.RequestedOperations = []string{"PUBLISH"}
	params.Payload.VolumeSeriesID = models.ObjIDMutable(vsID)
	params.Payload.CompleteByTime = strfmt.DateTime(time.Now().Add(time.Hour))
	vsObj.BoundClusterID = "clID"
	vsObj.VolumeSeriesState = vsState
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oCL = mock.NewMockClusterOps(mockCtrl)
	oSP.EXPECT().Fetch(ctx, string(spObj.Meta.ID)).Return(spObj, nil)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).Times(2)
	oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
	oVR.EXPECT().Create(ctx, params.Payload).Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
	mds.EXPECT().OpsCluster().Return(oCL)
	oCL.EXPECT().Fetch(ctx, string(clusterObj.Meta.ID)).Return(clusterObj, nil)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(params) })
	assert.NotNil(ret)
	assert.Equal([]string{"PUBLISH"}, params.Payload.RequestedOperations)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
		assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	}
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)

	// Create with UNPUBLISH op
	mockCtrl.Finish()
	t.Log("case: Create with UNPUBLISH op")
	mockCtrl = gomock.NewController(t)
	params.Payload.RequestedOperations = []string{"UNPUBLISH"}
	vsObj.BoundClusterID = "clID"
	params.Payload.VolumeSeriesID = models.ObjIDMutable(vsID)
	params.Payload.ClusterID = ""
	vsObj.VolumeSeriesState = com.VolStateBound
	vsObj.ClusterDescriptor = models.ClusterDescriptor{
		"string": models.ValueType{Kind: "STRING", Value: "somestring"},
	}
	vsObj.Mounts = nil
	mds = mock.NewMockDataStore(mockCtrl)
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(1)
	oVR.EXPECT().Create(ctx, params.Payload).Return(nil, centrald.ErrorDbError)
	oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(params) })
	assert.NotNil(ret)
	assert.Equal([]string{"UNPUBLISH"}, params.Payload.RequestedOperations)
	assert.NotEqual("", params.Payload.ClusterID)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
		assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	}
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	// ATTACH_FS/DETACH work when volume state is valid
	for _, op := range []string{"ATTACH_FS", "DETACH_FS"} {
		mockCtrl.Finish()
		t.Logf("case: %s VS HEAD mounted", op)
		tl.Flush()
		mockCtrl = gomock.NewController(t)
		var vsrVol *models.VolumeSeries
		testutils.Clone(vsObj, &vsrVol)
		vsrVol.VolumeSeriesState = "IN_USE"
		vsrVol.Mounts = []*models.Mount{
			&models.Mount{
				SnapIdentifier: "HEAD",
				MountedNodeID:  "nodeH",
				MountState:     "MOUNTED",
			},
		}
		mds = mock.NewMockDataStore(mockCtrl)
		ai.AccountID, ai.RoleObj = "", nil
		params.Payload.ClusterID, params.Payload.NodeID = "", ""
		params.Payload.RequestedOperations = []string{op}
		params.Payload.VolumeSeriesID = models.ObjIDMutable(vsID)
		params.Payload.TargetPath = "/mnt/foo"
		if op == "ATTACH_FS" {
			params.Payload.FsType = "xfs"
			vsrVol.SystemTags = nil
		} else {
			vsrVol.SystemTags = []string{fmt.Sprintf("%s:%s", com.SystemTagVolumeFsAttached, params.Payload.TargetPath)}
		}
		params.Payload.CompleteByTime = strfmt.DateTime(time.Now().Add(time.Hour))
		oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
		oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
		mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).Times(2)
		oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
		oVR.EXPECT().Create(ctx, params.Payload).Return(nil, centrald.ErrorDbError)
		mds.EXPECT().OpsVolumeSeries().Return(oVS)
		oVS.EXPECT().Fetch(ctx, vsID).Return(vsrVol, nil)
		oN = mock.NewMockNodeOps(mockCtrl)
		oN.EXPECT().Fetch(ctx, "nodeH").Return(nodeObj, nil)
		mds.EXPECT().OpsNode().Return(oN)
		hc.DS = mds
		cntLock = hc.cntLock
		cntUnlock = hc.cntUnlock
		assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(params) })
		assert.NotNil(ret)
		assert.Equal([]string{op}, params.Payload.RequestedOperations)
		assert.EqualValues("clID", params.Payload.ClusterID)
		assert.EqualValues("nodeH", params.Payload.NodeID)
		mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
		if assert.True(ok) {
			assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
			assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
		}
		assert.Equal(cntLock+1, hc.cntLock)
		assert.Equal(cntUnlock+1, hc.cntUnlock)
		params.Payload.FsType = ""
		params.Payload.TargetPath = ""
	}

	t.Log("case: auditLog not ready")
	fa.ReadyRet = centrald.ErrorDbError
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestCreate(params) })
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	// remaining error cases
	ai.AccountID, ai.TenantAccountID, ai.RoleObj = "", "", nil
	params.Payload.Creator = nil
	params.Payload.NodeID = models.ObjIDMutable(nodeObj.Meta.ID)
	for tc := 0; tc <= 118; tc++ {
		mockCtrl.Finish()
		fops := &fakeOps{}
		hc.ops = fops
		mockCtrl = gomock.NewController(t)
		oA = mock.NewMockAccountOps(mockCtrl)
		oSP = mock.NewMockServicePlanOps(mockCtrl)
		oSnap = mock.NewMockSnapshotOps(mockCtrl)
		oCG := mock.NewMockConsistencyGroupOps(mockCtrl)
		oCL = mock.NewMockClusterOps(mockCtrl)
		oN = mock.NewMockNodeOps(mockCtrl)
		oS = mock.NewMockStorageOps(mockCtrl)
		oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
		oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
		mds = mock.NewMockDataStore(mockCtrl)
		var payload *models.VolumeSeriesRequestCreateArgs
		testutils.Clone(params.Payload, &payload)
		vsObj.BoundClusterID = models.ObjIDMutable(clusterObj.Meta.ID)
		vsObj.VolumeSeriesState = com.VolStateUnbound
		expErr := centrald.ErrorMissing
		expInc := uint32(1)
		switch tc {
		case 0:
			t.Log("case: invalid volumeSeriesCreateSpec")
			payload.RequestedOperations = []string{"CREATE"}
			payload.VolumeSeriesCreateSpec = nil
		case 1:
			t.Log("case: invalid name")
			payload.RequestedOperations = []string{"CREATE"}
			payload.VolumeSeriesCreateSpec.Name = ""
		case 2:
			t.Log("case: accountId does not exist")
			payload.RequestedOperations = []string{"CREATE"}
			mds.EXPECT().OpsAccount().Return(oA)
			oA.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesCreateSpec.AccountID)).Return(nil, centrald.ErrorNotFound)
		case 3:
			t.Log("case: invalid servicePlanId")
			payload.RequestedOperations = []string{"CREATE"}
			payload.VolumeSeriesCreateSpec.ServicePlanID = ""
		case 4:
			t.Log("case: servicePlan does not exist")
			payload.RequestedOperations = []string{"CREATE"}
			mds.EXPECT().OpsAccount().Return(oA)
			oA.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesCreateSpec.AccountID)).Return(aObj, nil)
			mds.EXPECT().OpsServicePlan().Return(oSP)
			oSP.EXPECT().Fetch(ctx, string(spObj.Meta.ID)).Return(nil, centrald.ErrorNotFound)
		case 5:
			t.Log("case: servicePlan lookup fails")
			payload.RequestedOperations = []string{"CREATE"}
			mds.EXPECT().OpsAccount().Return(oA)
			oA.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesCreateSpec.AccountID)).Return(aObj, nil)
			mds.EXPECT().OpsServicePlan().Return(oSP)
			oSP.EXPECT().Fetch(ctx, string(spObj.Meta.ID)).Return(nil, centrald.ErrorDbError)
			expErr = centrald.ErrorDbError
		case 6:
			t.Log("case: invalid operation")
			payload.RequestedOperations = []string{"BIND", "INVALID"}
			expInc = 0
		case 7:
			t.Log("case: createByTime in the past")
			payload.CompleteByTime = strfmt.DateTime(time.Now().Add(-time.Hour))
			expInc = 0
		case 8:
			t.Log("case: volumeSeries in use")
			payload.RequestedOperations = []string{"BIND"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			var vsr *models.VolumeSeriesRequest
			testutils.Clone(obj, &vsr)
			vsr.RequestedOperations = []string{"MOUNT"}
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{vsr}, nil)
			expErr = centrald.ErrorRequestInConflict
		case 9:
			t.Log("case: list fails")
			payload.RequestedOperations = []string{"BIND"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return(nil, centrald.ErrorDbError)
			expErr = centrald.ErrorDbError
		case 10:
			t.Log("case: volumeSeries does not exist")
			payload.RequestedOperations = []string{"MOUNT"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(nil, centrald.ErrorNotFound)
		case 11:
			t.Log("case: volumeSeries lookup fails")
			payload.RequestedOperations = []string{"MOUNT"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, string(vsID)).Return(nil, centrald.ErrorDbError)
			expErr = centrald.ErrorDbError
		case 12:
			t.Log("case: BIND volume not UNBOUND")
			payload.RequestedOperations = []string{"BIND"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			vsObj.VolumeSeriesState = com.VolStateBound
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
		case 13:
			t.Log("case: cluster does not exist")
			payload.RequestedOperations = []string{"BIND"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsCluster().Return(oCL)
			oCL.EXPECT().Fetch(ctx, (string(clusterObj.Meta.ID))).Return(nil, centrald.ErrorNotFound)
		case 14:
			t.Log("case: cluster lookup fails")
			payload.RequestedOperations = []string{"BIND"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsCluster().Return(oCL)
			oCL.EXPECT().Fetch(ctx, (string(clusterObj.Meta.ID))).Return(nil, centrald.ErrorDbError)
			expErr = centrald.ErrorDbError
		case 15:
			t.Log("case: CREATE+MOUNT fails")
			fops.RetAccountFetchObj = aObj
			payload.RequestedOperations = []string{"CREATE", "MOUNT"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			mds.EXPECT().OpsAccount().Return(oA)
			oA.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesCreateSpec.AccountID)).Return(aObj, nil)
			mds.EXPECT().OpsServicePlan().Return(oSP)
			oSP.EXPECT().Fetch(ctx, string(spObj.Meta.ID)).Return(spObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, gomock.Any()).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oV.EXPECT().Count(ctx, gomock.Any(), uint(1)).Return(0, nil)
		case 16:
			t.Log("case: MOUNT and volume is not BOUND")
			fops.RetAccountFetchObj = aObj
			payload.RequestedOperations = []string{"MOUNT"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
		case 17:
			t.Log("case: MOUNT PROVISIONED and invalid snapIdentifier")
			fops.RetAccountFetchObj = aObj
			payload.RequestedOperations = []string{"MOUNT"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			payload.SnapIdentifier = "INVALID"
			vsObj.VolumeSeriesState = com.VolStateProvisioned
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
		case 18:
			t.Log("case: node does not exist")
			fops.RetAccountFetchObj = aObj
			payload.RequestedOperations = []string{"MOUNT"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			vsObj.VolumeSeriesState = com.VolStateBound
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsNode().Return(oN)
			oN.EXPECT().Fetch(ctx, (string(nodeObj.Meta.ID))).Return(nil, centrald.ErrorNotFound)
		case 19:
			t.Log("case: node lookup fails")
			fops.RetAccountFetchObj = aObj
			payload.RequestedOperations = []string{"MOUNT"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			vsObj.VolumeSeriesState = com.VolStateBound
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsNode().Return(oN)
			oN.EXPECT().Fetch(ctx, (string(nodeObj.Meta.ID))).Return(nil, centrald.ErrorDbError)
			expErr = centrald.ErrorDbError
		case 20:
			t.Log("case: cluster mismatch")
			fops.RetAccountFetchObj = aObj
			payload.RequestedOperations = []string{"MOUNT"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			vsObj.VolumeSeriesState = com.VolStateBound
			vsObj.BoundClusterID = "anotherCluster"
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsNode().Return(oN)
			oN.EXPECT().Fetch(ctx, (string(nodeObj.Meta.ID))).Return(nodeObj, nil)
		case 21:
			t.Log("case: DELETE but wrong state")
			payload.RequestedOperations = []string{"DELETE"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			vsObj.VolumeSeriesState = com.VolStateInUse
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
		case 22:
			t.Log("case: UNMOUNT but wrong state")
			payload.RequestedOperations = []string{"UNMOUNT"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			vsObj.VolumeSeriesState = com.VolStateProvisioned
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
		case 23:
			t.Log("case: UNMOUNT but node lookup fails")
			payload.RequestedOperations = []string{"UNMOUNT"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			vsObj.VolumeSeriesState = com.VolStateInUse
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsNode().Return(oN)
			oN.EXPECT().Fetch(ctx, (string(nodeObj.Meta.ID))).Return(nil, centrald.ErrorDbError)
			expErr = centrald.ErrorDbError
		case 24:
			t.Log("case: UNMOUNT but node does not exist")
			payload.RequestedOperations = []string{"UNMOUNT"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			vsObj.VolumeSeriesState = com.VolStateInUse
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsNode().Return(oN)
			oN.EXPECT().Fetch(ctx, (string(nodeObj.Meta.ID))).Return(nil, centrald.ErrorNotFound)
		case 25:
			t.Log("case: UNMOUNT and invalid snapIdentifier")
			payload.RequestedOperations = []string{"UNMOUNT"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			payload.SnapIdentifier = "INVALID"
			vsObj.VolumeSeriesState = com.VolStateInUse
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
		case 26:
			t.Log("case: UNMOUNT and snapIdentifier not mounted")
			payload.RequestedOperations = []string{"UNMOUNT"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			payload.SnapIdentifier = "HEAD"
			vsObj.VolumeSeriesState = com.VolStateInUse
			vsObj.Mounts = []*models.Mount{}
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsNode().Return(oN)
			oN.EXPECT().Fetch(ctx, (string(nodeObj.Meta.ID))).Return(nodeObj, nil)
		case 27:
			t.Log("case: DELETE and rootStorageId fetch fails")
			payload.RequestedOperations = []string{"DELETE"}
			vsObj.RootStorageID = "s1"
			vsObj.VolumeSeriesState = "PROVISIONED"
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsStorage().Return(oS)
			oS.EXPECT().Fetch(ctx, "s1").Return(nil, centrald.ErrorDbError)
			expErr = centrald.ErrorDbError
		case 28:
			t.Log("case: invalid volumeSeriesCreateSpec for RENAME")
			payload.RequestedOperations = []string{"RENAME"}
			payload.VolumeSeriesCreateSpec = nil
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
		case 29:
			t.Log("case: invalid name for RENAME")
			payload.RequestedOperations = []string{"RENAME"}
			payload.VolumeSeriesCreateSpec.Name = ""
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
		case 30:
			t.Log("case: no change to name for RENAME")
			payload.RequestedOperations = []string{"RENAME"}
			vsObj.Name = params.Payload.VolumeSeriesCreateSpec.Name
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
		case 31:
			t.Log("case: ALLOCATE_CAPACITY missing spec")
			payload.RequestedOperations = []string{"ALLOCATE_CAPACITY"}
			payload.ServicePlanAllocationCreateSpec = nil
		case 32:
			t.Log("case: ALLOCATE_CAPACITY invalid spec")
			payload.RequestedOperations = []string{"ALLOCATE_CAPACITY"}
			payload.ServicePlanAllocationCreateSpec = &models.ServicePlanAllocationCreateArgs{}
			fakeSPAv.retErr = centrald.ErrorMissing
		case 33:
			t.Log("case: ALLOCATE_CAPACITY db error")
			payload.RequestedOperations = []string{"ALLOCATE_CAPACITY"}
			payload.ServicePlanAllocationCreateSpec = &models.ServicePlanAllocationCreateArgs{}
			fakeSPAv.retErr = centrald.ErrorDbError
			expErr = centrald.ErrorDbError
		case 34:
			t.Log("case: CG_SNAPSHOT_CREATE missing cg") // also see case 44
			payload.ConsistencyGroupID = ""
			payload.RequestedOperations = []string{"CG_SNAPSHOT_CREATE"}
		case 35:
			t.Log("case: CG_SNAPSHOT_CREATE invalid cg")
			payload.RequestedOperations = []string{"CG_SNAPSHOT_CREATE"}
			payload.ConsistencyGroupID = "cgId"
			mds.EXPECT().OpsConsistencyGroup().Return(oCG)
			oCG.EXPECT().Fetch(ctx, "cgId").Return(nil, centrald.ErrorDbError)
			expErr = centrald.ErrorDbError
		case 36:
			t.Log("case: CG_SNAPSHOT_CREATE invalid cl")
			payload.RequestedOperations = []string{"CG_SNAPSHOT_CREATE"}
			payload.ConsistencyGroupID = "cgId"
			payload.ClusterID = "clId"
			mds.EXPECT().OpsConsistencyGroup().Return(oCG)
			mds.EXPECT().OpsCluster().Return(oCL)
			oCG.EXPECT().Fetch(ctx, "cgId").Return(cgObj, nil)
			oCL.EXPECT().Fetch(ctx, "clId").Return(nil, centrald.ErrorNotFound)
		case 37:
			t.Log("case: CG_SNAPSHOT_CREATE count failed")
			payload.RequestedOperations = []string{"CG_SNAPSHOT_CREATE"}
			payload.ConsistencyGroupID = "cgId"
			payload.ClusterID = "clId"
			mds.EXPECT().OpsConsistencyGroup().Return(oCG)
			mds.EXPECT().OpsCluster().Return(oCL)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oCG.EXPECT().Fetch(ctx, "cgId").Return(cgObj, nil)
			oCL.EXPECT().Fetch(ctx, "clId").Return(clusterObj, nil)
			oVR.EXPECT().Count(ctx, gomock.Any(), uint(1)).Return(0, centrald.ErrorDbError)
			expErr = centrald.ErrorDbError
		case 38:
			t.Log("case: CG_SNAPSHOT_CREATE conflicting cg ops")
			payload.RequestedOperations = []string{"CG_SNAPSHOT_CREATE"}
			payload.ConsistencyGroupID = "cgId"
			payload.ClusterID = "clId"
			mds.EXPECT().OpsConsistencyGroup().Return(oCG)
			mds.EXPECT().OpsCluster().Return(oCL)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oCG.EXPECT().Fetch(ctx, "cgId").Return(cgObj, nil)
			oCL.EXPECT().Fetch(ctx, "clId").Return(clusterObj, nil)
			oVR.EXPECT().Count(ctx, gomock.Any(), uint(1)).Return(1, nil)
			expErr = centrald.ErrorRequestInConflict
		case 39:
			t.Log("case: VOL_SNAPSHOT_CREATE vol state")
			fops.RetAccountFetchObj = aObj
			payload.RequestedOperations = []string{"VOL_SNAPSHOT_CREATE"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsObj.Meta.ID)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
		case 40:
			t.Log("case: VOL_SNAPSHOT_CREATE (no sync) 2nd vsr list fails")
			fops.RetAccountFetchObj = aObj
			fops.RetAccountFetchErr = nil
			payload.RequestedOperations = []string{"VOL_SNAPSHOT_CREATE"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsObj.Meta.ID)
			vsObj.VolumeSeriesState = "IN_USE"
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(2)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
			listParams2 := ops.VolumeSeriesRequestListParams{
				ConsistencyGroupID: swag.String(string(vsObj.ConsistencyGroupID)),
				IsTerminated:       swag.Bool(false),
			}
			oVR.EXPECT().List(ctx, listParams2).Return(nil, centrald.ErrorDbError)
			expErr = centrald.ErrorDbError
		case 41:
			t.Log("case: VOL_SNAPSHOT_CREATE (no sync) conflict with unspecific cluster CG_SNAPSHOT_CREATE")
			fops.RetAccountFetchObj = aObj
			fops.RetAccountFetchErr = nil
			payload.RequestedOperations = []string{"VOL_SNAPSHOT_CREATE"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsObj.Meta.ID)
			vsObj.VolumeSeriesState = "IN_USE"
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(2)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
			listParams2 := ops.VolumeSeriesRequestListParams{
				ConsistencyGroupID: swag.String(string(vsObj.ConsistencyGroupID)),
				IsTerminated:       swag.Bool(false),
			}
			conflictingVSR := []*models.VolumeSeriesRequest{&models.VolumeSeriesRequest{}}
			conflictingVSR[0].Meta = &models.ObjMeta{ID: "cgVSR"}
			conflictingVSR[0].RequestedOperations = []string{"CG_SNAPSHOT_CREATE"}
			oVR.EXPECT().List(ctx, listParams2).Return(conflictingVSR, nil)
			expErr = centrald.ErrorRequestInConflict
		case 42:
			t.Log("case: VOL_SNAPSHOT_CREATE (sync) coordinator not found")
			fops.RetAccountFetchObj = aObj
			fops.RetAccountFetchErr = nil
			payload.RequestedOperations = []string{"VOL_SNAPSHOT_CREATE"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsObj.Meta.ID)
			payload.SyncCoordinatorID = "coordinatorVSRId"
			vsObj.VolumeSeriesState = "IN_USE"
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(2)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
			oVR.EXPECT().Fetch(ctx, string(payload.SyncCoordinatorID)).Return(nil, centrald.ErrorNotFound)
		case 43:
			t.Log("case: VOL_SNAPSHOT_CREATE (sync) coordinator not CG_SNAPSHOT_CREATE")
			fops.RetAccountFetchObj = aObj
			fops.RetAccountFetchErr = nil
			payload.RequestedOperations = []string{"VOL_SNAPSHOT_CREATE"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsObj.Meta.ID)
			payload.SyncCoordinatorID = "coordinatorVSRId"
			vsObj.VolumeSeriesState = "IN_USE"
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(2)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
			syncVSR := &models.VolumeSeriesRequest{}
			syncVSR.RequestedOperations = []string{"something-wrong"}
			oVR.EXPECT().Fetch(ctx, string(payload.SyncCoordinatorID)).Return(syncVSR, nil)
		case 44:
			t.Log("case: VOL_SNAPSHOT_CREATE nodeID not found")
			fops.RetAccountFetchObj = aObj
			fops.RetAccountFetchErr = nil
			payload.RequestedOperations = []string{"VOL_SNAPSHOT_CREATE"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsObj.Meta.ID)
			payload.SyncCoordinatorID = "coordinatorVSRId"
			testutils.Clone(vsObj, &vscVol)
			vscVol.VolumeSeriesState = "IN_USE"
			vscVol.Mounts = []*models.Mount{
				&models.Mount{MountState: "MOUNTING", SnapIdentifier: "HEAD"},
			}
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(2)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
			syncVSR := &models.VolumeSeriesRequest{}
			syncVSR.RequestedOperations = []string{"CG_SNAPSHOT_CREATE"}
			oVR.EXPECT().Fetch(ctx, string(payload.SyncCoordinatorID)).Return(syncVSR, nil)
		case 45:
			t.Log("case: CG_SNAPSHOT_CREATE missing cl")
			payload.RequestedOperations = []string{"CG_SNAPSHOT_CREATE"}
			payload.ConsistencyGroupID = "cgId"
			payload.ClusterID = ""
		case 46:
			t.Log("case: CREATE_FROM_SNAPSHOT invalid nodeId")
			testutils.Clone(cfsParams.Payload, payload)
			oA.EXPECT().Fetch(ctx, string(cfsParams2.Payload.VolumeSeriesCreateSpec.AccountID)).Return(aObj, nil)
			oN.EXPECT().Fetch(ctx, string(cfsParams.Payload.NodeID)).Return(nil, centrald.ErrorNotFound)
			oSP.EXPECT().Fetch(ctx, string(cfsParams2.Payload.VolumeSeriesCreateSpec.ServicePlanID)).Return(spObj, nil)
			oVS.EXPECT().Fetch(ctx, string(cfsVol.Meta.ID)).Return(cfsVol, nil)
			oVR.EXPECT().List(ctx, lM).Return(sourceVSRs, nil)
			oSnap.EXPECT().List(ctx, lSM).Return(snapList, nil)
			mds.EXPECT().OpsAccount().Return(oA)
			mds.EXPECT().OpsNode().Return(oN)
			mds.EXPECT().OpsServicePlan().Return(oSP)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			mds.EXPECT().OpsSnapshot().Return(oSnap)
		case 47:
			t.Log("case: CREATE_FROM_SNAPSHOT invalid createspec")
			testutils.Clone(cfsParams.Payload, payload)
			payload.VolumeSeriesCreateSpec.Name = ""
			oVS.EXPECT().Fetch(ctx, string(cfsVol.Meta.ID)).Return(cfsVol, nil)
			oVR.EXPECT().List(ctx, lM).Return(sourceVSRs, nil)
			oSnap.EXPECT().List(ctx, lSM).Return(snapList, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			mds.EXPECT().OpsSnapshot().Return(oSnap)
		case 48:
			t.Log("case: CREATE_FROM_SNAPSHOT invalid snapIdentifier (failure to list snapshots)")
			testutils.Clone(cfsParams.Payload, payload)
			oVS.EXPECT().Fetch(ctx, string(cfsVol.Meta.ID)).Return(cfsVol, nil)
			oVR.EXPECT().List(ctx, lM).Return(sourceVSRs, nil)
			oSnap.EXPECT().List(ctx, lSM).Return(nil, centrald.ErrorDbError)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			mds.EXPECT().OpsSnapshot().Return(oSnap)
			expErr = centrald.ErrorDbError
		case 49:
			t.Log("case: CREATE_FROM_SNAPSHOT invalid snapIdentifier (SizeBytes)")
			snapList[0].SizeBytes = 0
			testutils.Clone(cfsParams.Payload, payload)
			oVS.EXPECT().Fetch(ctx, string(cfsVol.Meta.ID)).Return(cfsVol, nil)
			oVR.EXPECT().List(ctx, lM).Return(sourceVSRs, nil)
			oSnap.EXPECT().List(ctx, lSM).Return(snapList, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			mds.EXPECT().OpsSnapshot().Return(oSnap)
		case 50:
			t.Log("case: CREATE_FROM_SNAPSHOT invalid snapIdentifier (empty Locations)")
			snapList[0].SizeBytes = int64(sizeBytes)
			snapList[0].Locations = map[string]models.SnapshotLocation{}
			testutils.Clone(cfsParams.Payload, payload)
			oVS.EXPECT().Fetch(ctx, string(cfsVol.Meta.ID)).Return(cfsVol, nil)
			oVR.EXPECT().List(ctx, lM).Return(sourceVSRs, nil)
			oSnap.EXPECT().List(ctx, lSM).Return(snapList, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			mds.EXPECT().OpsSnapshot().Return(oSnap)
		case 51:
			t.Log("case: CREATE_FROM_SNAPSHOT conflicting VSR (Delete)")
			testutils.Clone(cfsParams.Payload, payload)
			sourceVSRs := []*models.VolumeSeriesRequest{
				&models.VolumeSeriesRequest{},
			}
			sourceVSRs[0].RequestedOperations = []string{"DELETE"}
			sourceVSRs[0].Meta = &models.ObjMeta{ID: "conflictingDelete"}
			oVS.EXPECT().Fetch(ctx, string(cfsVol.Meta.ID)).Return(cfsVol, nil)
			oVR.EXPECT().List(ctx, lM).Return(sourceVSRs, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			expErr = centrald.ErrorRequestInConflict
		case 52:
			t.Log("case: CREATE_FROM_SNAPSHOT requires nodeID")
			testutils.Clone(cfsParams.Payload, payload)
			payload.NodeID = ""
		case 53:
			t.Log("case: CREATE_FROM_SNAPSHOT requires volumeSeriesCreateSpec")
			payload = cfsParams.Payload
			payload.VolumeSeriesCreateSpec = nil
		case 54:
			t.Log("case: CREATE_FROM_SNAPSHOT requires snapshot.snapIdentifier")
			testutils.Clone(cfsParams.Payload, payload)
			payload.Snapshot.SnapIdentifier = ""
		case 55:
			t.Log("case: CREATE_FROM_SNAPSHOT requires snapshot.volumeSeriesId")
			testutils.Clone(cfsParams.Payload, payload)
			payload.Snapshot.VolumeSeriesID = ""
		case 56:
			t.Log("case: CREATE_FROM_SNAPSHOT requires snapshot")
			testutils.Clone(cfsParams.Payload, payload)
			payload.Snapshot = nil
		case 57:
			t.Log("case: VOL_SNAPSHOT_RESTORE syncCoordinator invalid")
			testutils.Clone(vsrParams.Payload, payload)
			payload.SyncCoordinatorID = models.ObjIDMutable(cfsVSR.Meta.ID)
			cfsVSR.RequestedOperations = []string{"MOUNT"}
			oVS.EXPECT().Fetch(ctx, string(vsrVol.Meta.ID)).Return(vsrVol, nil)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
			oVR.EXPECT().Fetch(ctx, string(payload.SyncCoordinatorID)).Return(cfsVSR, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(2)
		case 58:
			t.Log("case: VOL_SNAPSHOT_RESTORE syncCoordinator not found")
			testutils.Clone(vsrParams.Payload, payload)
			payload.SyncCoordinatorID = models.ObjIDMutable(cfsVSR.Meta.ID)
			cfsVSR.RequestedOperations = []string{"MOUNT"}
			oVS.EXPECT().Fetch(ctx, string(vsrVol.Meta.ID)).Return(vsrVol, nil)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
			oVR.EXPECT().Fetch(ctx, string(payload.SyncCoordinatorID)).Return(nil, centrald.ErrorNotFound)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(2)
		case 59:
			t.Log("case: VOL_SNAPSHOT_RESTORE size mismatch")
			testutils.Clone(vsrParams.Payload, payload)
			payload.Snapshot = &models.SnapshotData{
				SnapIdentifier: "snapid",
				PitIdentifier:  "pitid",
				Locations: []*models.SnapshotLocation{
					&models.SnapshotLocation{CspDomainID: "ps1"},
				},
				VolumeSeriesID: "sourceVS",
				SizeBytes:      swag.Int64(swag.Int64Value(vsrVol.SizeBytes) + 1000),
			}
			oVS.EXPECT().Fetch(ctx, string(vsrVol.Meta.ID)).Return(vsrVol, nil)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
		case 60:
			t.Log("case: VOL_SNAPSHOT_RESTORE snapshot invalid (pitIdentifier)")
			testutils.Clone(vsrParams.Payload, payload)
			payload.Snapshot = &models.SnapshotData{
				SnapIdentifier: "snapid",
				Locations: []*models.SnapshotLocation{
					&models.SnapshotLocation{CspDomainID: "ps1"},
				},
				VolumeSeriesID: "sourceVS",
				SizeBytes:      vsrVol.SizeBytes,
			}
			oVS.EXPECT().Fetch(ctx, string(vsrVol.Meta.ID)).Return(vsrVol, nil)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
		case 61:
			t.Log("case: VOL_SNAPSHOT_RESTORE snapshot invalid (locations)")
			testutils.Clone(vsrParams.Payload, payload)
			payload.Snapshot = &models.SnapshotData{
				SnapIdentifier: "snapid",
				VolumeSeriesID: "sourceVS",
				SizeBytes:      vsrVol.SizeBytes,
			}
			oVS.EXPECT().Fetch(ctx, string(vsrVol.Meta.ID)).Return(vsrVol, nil)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
		case 62:
			t.Log("case: VOL_SNAPSHOT_RESTORE snapshot invalid (snapIdentifier)")
			testutils.Clone(vsrParams.Payload, payload)
			payload.Snapshot = &models.SnapshotData{
				VolumeSeriesID: "sourceVS",
				SizeBytes:      vsrVol.SizeBytes,
			}
			oVS.EXPECT().Fetch(ctx, string(vsrVol.Meta.ID)).Return(vsrVol, nil)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
		case 63:
			t.Log("case: VOL_SNAPSHOT_RESTORE snapshot invalid (volumeSeriesId)")
			testutils.Clone(vsrParams.Payload, payload)
			payload.Snapshot = &models.SnapshotData{
				SizeBytes: vsrVol.SizeBytes,
			}
			oVS.EXPECT().Fetch(ctx, string(vsrVol.Meta.ID)).Return(vsrVol, nil)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
		case 64:
			t.Log("case: VOL_SNAPSHOT_RESTORE snapshot invalid (size)")
			testutils.Clone(vsrParams.Payload, payload)
			payload.Snapshot = &models.SnapshotData{}
			oVS.EXPECT().Fetch(ctx, string(vsrVol.Meta.ID)).Return(vsrVol, nil)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
		case 65:
			t.Log("case: VOL_SNAPSHOT_RESTORE snapshot invalid (snapshot)")
			testutils.Clone(vsrParams.Payload, payload)
			payload.Snapshot = nil
			oVS.EXPECT().Fetch(ctx, string(vsrVol.Meta.ID)).Return(vsrVol, nil)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
		case 66:
			t.Log("case: VOL_SNAPSHOT_RESTORE head not mounted")
			testutils.Clone(vsrParams.Payload, payload)
			vsrVol.Mounts = []*models.Mount{}
			oVS.EXPECT().Fetch(ctx, string(vsrVol.Meta.ID)).Return(vsrVol, nil)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
		case 67:
			t.Log("case: VOL_SNAPSHOT_RESTORE not IN_USE")
			testutils.Clone(vsrParams.Payload, payload)
			vsrVol.VolumeSeriesState = "MOUNTING"
			oVS.EXPECT().Fetch(ctx, string(vsrVol.Meta.ID)).Return(vsrVol, nil)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
		case 68:
			t.Log("case: VOL_SNAPSHOT_CREATE PROVISIONED and nodeId not set")
			fops.RetAccountFetchObj = aObj
			fops.RetAccountFetchErr = nil
			payload.RequestedOperations = []string{"VOL_SNAPSHOT_CREATE"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsObj.Meta.ID)
			payload.SyncCoordinatorID = "coordinatorVSRId"
			payload.NodeID = ""
			testutils.Clone(vsObj, &vscVol)
			vscVol.VolumeSeriesState = "PROVISIONED"
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(2)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
			syncVSR := &models.VolumeSeriesRequest{}
			syncVSR.RequestedOperations = []string{"CG_SNAPSHOT_CREATE"}
			oVR.EXPECT().Fetch(ctx, string(payload.SyncCoordinatorID)).Return(syncVSR, nil)
		case 69:
			t.Log("case: VOL_SNAPSHOT_CREATE PROVISIONED and nodeId invalid")
			fops.RetAccountFetchObj = aObj
			fops.RetAccountFetchErr = nil
			payload.RequestedOperations = []string{"VOL_SNAPSHOT_CREATE"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsObj.Meta.ID)
			payload.SyncCoordinatorID = "coordinatorVSRId"
			payload.NodeID = "someNode"
			testutils.Clone(vsObj, &vscVol)
			vscVol.VolumeSeriesState = "PROVISIONED"
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(2)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
			syncVSR := &models.VolumeSeriesRequest{}
			syncVSR.RequestedOperations = []string{"CG_SNAPSHOT_CREATE"}
			oVR.EXPECT().Fetch(ctx, string(payload.SyncCoordinatorID)).Return(syncVSR, nil)
			oN.EXPECT().Fetch(ctx, string(payload.NodeID)).Return(nil, centrald.ErrorNotFound)
			mds.EXPECT().OpsNode().Return(oN)
		case 70:
			t.Log("case: VOL_SNAPSHOT_CREATE PROVISIONED and nodeId wrong cluster")
			fops.RetAccountFetchObj = aObj
			fops.RetAccountFetchErr = nil
			payload.RequestedOperations = []string{"VOL_SNAPSHOT_CREATE"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsObj.Meta.ID)
			payload.SyncCoordinatorID = "coordinatorVSRId"
			payload.NodeID = "someNode"
			testutils.Clone(vsObj, &vscVol)
			vscVol.VolumeSeriesState = "PROVISIONED"
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(2)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
			syncVSR := &models.VolumeSeriesRequest{}
			syncVSR.RequestedOperations = []string{"CG_SNAPSHOT_CREATE"}
			oVR.EXPECT().Fetch(ctx, string(payload.SyncCoordinatorID)).Return(syncVSR, nil)
			var nClone *models.Node
			testutils.Clone(nodeObj, &nClone)
			nClone.ClusterID = vscVol.BoundClusterID + "foo"
			oN.EXPECT().Fetch(ctx, string(payload.NodeID)).Return(nClone, nil)
			mds.EXPECT().OpsNode().Return(oN)
		case 71:
			t.Log("case: ValidateCreator failure handled")
			payload.NodeID = ""
			payload.RequestedOperations = []string{"RENAME"} // the op doesn't matter, this is minimal
			payload.VolumeSeriesCreateSpec.Name = "whatever"
			payload.Creator = &models.Identity{AccountID: "mismatch"}
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
		case 72:
			t.Log("case: agIDs conflict with cgID")
			payload.VolumeSeriesCreateSpec.AccountID = "aID"
			payload.ApplicationGroupIds = []models.ObjIDMutable{"any"}
			payload.VolumeSeriesCreateSpec.ConsistencyGroupID = "cgId"
			payload.RequestedOperations = []string{"CREATE"}
			mds.EXPECT().OpsAccount().Return(oA)
			oA.EXPECT().Fetch(ctx, string(payload.VolumeSeriesCreateSpec.AccountID)).Return(aObj, nil)
			mds.EXPECT().OpsServicePlan().Return(oSP)
			oSP.EXPECT().Fetch(ctx, string(spObj.Meta.ID)).Return(spObj, nil)
			mds.EXPECT().OpsConsistencyGroup().Return(oCG)
			oCG.EXPECT().Fetch(ctx, "cgId").Return(cgObj, nil)
			expErr = &centrald.Error{C: expErr.C, M: ".*applicationGroupIds must not be specified"}
		case 73:
			t.Log("case: validateApplicationGroupIds fails")
			payload.VolumeSeriesCreateSpec.AccountID = "aID"
			payload.ApplicationGroupIds = []models.ObjIDMutable{"any"}
			payload.RequestedOperations = []string{"CREATE"}
			mds.EXPECT().OpsAccount().Return(oA)
			oA.EXPECT().Fetch(ctx, string(payload.VolumeSeriesCreateSpec.AccountID)).Return(aObj, nil)
			mds.EXPECT().OpsServicePlan().Return(oSP)
			oSP.EXPECT().Fetch(ctx, string(spObj.Meta.ID)).Return(spObj, nil)
			oAG = mock.NewMockApplicationGroupOps(mockCtrl)
			oAG.EXPECT().Fetch(ctx, "any").Return(nil, centrald.ErrorDbError)
			mds.EXPECT().OpsApplicationGroup().Return(oAG)
			expErr = centrald.ErrorDbError
		case 74:
			t.Log("case: CONFIGURE volume not PROVISIONED")
			payload.RequestedOperations = []string{"CONFIGURE"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsObj.Meta.ID)
			testutils.Clone(vsObj, &vscVol)
			vscVol.VolumeSeriesState = "BOUND"
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
		case 75:
			t.Log("case: CONFIGURE missing node")
			payload.RequestedOperations = []string{"CONFIGURE"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsObj.Meta.ID)
			testutils.Clone(vsObj, &vscVol)
			vscVol.VolumeSeriesState = "PROVISIONED"
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsNode().Return(oN)
			oN.EXPECT().Fetch(ctx, string(payload.NodeID)).Return(nil, centrald.ErrorNotFound)
		case 76:
			t.Log("case: CONFIGURE node binding wrong")
			payload.RequestedOperations = []string{"CONFIGURE"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsObj.Meta.ID)
			payload.NodeID = "someNode"
			testutils.Clone(vsObj, &vscVol)
			vscVol.VolumeSeriesState = "PROVISIONED"
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsNode().Return(oN)
			var nClone *models.Node
			testutils.Clone(nodeObj, &nClone)
			nClone.ClusterID = vscVol.BoundClusterID + "foo"
			oN.EXPECT().Fetch(ctx, string(payload.NodeID)).Return(nClone, nil)
		case 77:
			t.Log("case: CHANGE_CAPACITY missing volumeSeriesId")
			payload.RequestedOperations = []string{"CHANGE_CAPACITY"}
			payload.VolumeSeriesID = ""
		case 78:
			t.Log("case: CHANGE_CAPACITY missing volumeSeriesCreateSpec")
			payload.RequestedOperations = []string{"CHANGE_CAPACITY"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsObj.Meta.ID)
			payload.VolumeSeriesCreateSpec = nil
		case 79:
			t.Log("case: CHANGE_CAPACITY empty volumeSeriesCreateSpec")
			payload.RequestedOperations = []string{"CHANGE_CAPACITY"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsObj.Meta.ID)
			payload.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{}
		case 80:
			t.Log("case: CHANGE_CAPACITY sizeBytes not increased")
			testutils.Clone(vsObj, &vscVol)
			vscVol.VolumeSeriesState = "UNBOUND"
			vscVol.SizeBytes = swag.Int64(1000)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, string(vscVol.Meta.ID)).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
			payload.RequestedOperations = []string{"CHANGE_CAPACITY"}
			payload.VolumeSeriesID = models.ObjIDMutable(vscVol.Meta.ID)
			payload.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{
				VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
					SizeBytes: vscVol.SizeBytes,
				},
			}
		case 81:
			t.Log("case: CHANGE_CAPACITY spaAdditionalBytes unchanged and no sizeBytes")
			testutils.Clone(vsObj, &vscVol)
			vscVol.VolumeSeriesState = "PROVISIONED"
			vscVol.SpaAdditionalBytes = swag.Int64(1000)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, string(vscVol.Meta.ID)).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
			payload.RequestedOperations = []string{"CHANGE_CAPACITY"}
			payload.VolumeSeriesID = models.ObjIDMutable(vscVol.Meta.ID)
			payload.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{
				VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
					SpaAdditionalBytes: vscVol.SpaAdditionalBytes,
				},
			}
		case 82:
			t.Log("case: CHANGE_CAPACITY sizeBytes and IsProvisioned()")
			testutils.Clone(vsObj, &vscVol)
			vscVol.VolumeSeriesState = "PROVISIONED"
			vscVol.SizeBytes = swag.Int64(1000)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, string(vscVol.Meta.ID)).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
			payload.RequestedOperations = []string{"CHANGE_CAPACITY"}
			payload.VolumeSeriesID = models.ObjIDMutable(vscVol.Meta.ID)
			payload.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{
				VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
					SizeBytes: swag.Int64(1001),
				},
			}
		case 83:
			t.Log("case: CHANGE_CAPACITY spaAdditionalBytes and !IsBound()")
			testutils.Clone(vsObj, &vscVol)
			vscVol.VolumeSeriesState = "UNBOUND"
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, string(vscVol.Meta.ID)).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
			payload.RequestedOperations = []string{"CHANGE_CAPACITY"}
			payload.VolumeSeriesID = models.ObjIDMutable(vscVol.Meta.ID)
			payload.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{
				VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
					SpaAdditionalBytes: swag.Int64(100),
				},
			}
		case 84:
			t.Log("case: PUBLISH and volume is not BOUND")
			testutils.Clone(vsObj, &vscVol)
			payload.RequestedOperations = []string{"PUBLISH"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
		case 85:
			t.Log("case: UNPUBLISH and volume is IN_USE")
			testutils.Clone(vsObj, &vscVol)
			payload.RequestedOperations = []string{"UNPUBLISH"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			vscVol.VolumeSeriesState = com.VolStateInUse
			vscVol.ClusterDescriptor = models.ClusterDescriptor{
				"string": models.ValueType{Kind: "STRING", Value: "somestring"},
			}
			vscVol.Mounts = []*models.Mount{
				{MountedNodeID: models.ObjIDMutable(nodeObj.Meta.ID), SnapIdentifier: com.VolMountHeadIdentifier, MountState: com.VolMountStateMounted},
			}
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
		case 86:
			t.Log("case: PUBLISH and ClusterID not specified")
			payload.RequestedOperations = []string{"PUBLISH"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			payload.ClusterID = ""
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
		case 87:
			t.Log("case: PUBLISH and cluster client lookup failed")
			payload.RequestedOperations = []string{"PUBLISH"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			vsObj.BoundClusterID = "clID"
			vsObj.VolumeSeriesState = com.VolStateBound
			hc.clusterClientMap = make(map[string]cluster.Client)
			hc.clusterClientMap[clusterObj.ClusterType] = nil
			mds.EXPECT().OpsCluster().Return(oCL)
			oCL.EXPECT().Fetch(ctx, "clID").Return(clusterObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
		case 88:
			t.Log("case: PUBLISH and failed to load cluster Object")
			payload.RequestedOperations = []string{"PUBLISH"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			vsObj.BoundClusterID = "clID"
			vsObj.VolumeSeriesState = com.VolStateBound
			oClCl := mockcluster.NewMockClient(mockCtrl)
			hc.clusterClientMap = make(map[string]cluster.Client)
			hc.clusterClientMap[clusterObj.ClusterType] = oClCl
			mds.EXPECT().OpsCluster().Return(oCL)
			oCL.EXPECT().Fetch(ctx, "clID").Return(nil, centrald.ErrorNotFound)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
		case 89:
			t.Log("case: PUBLISH and invalid DriverType")
			payload.RequestedOperations = []string{"PUBLISH"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			payload.DriverType = "invalidType"
			vsObj.BoundClusterID = "clID"
			vsObj.VolumeSeriesState = com.VolStateBound
			oClCl := mockcluster.NewMockClient(mockCtrl)
			hc.clusterClientMap = make(map[string]cluster.Client)
			hc.clusterClientMap[clusterObj.ClusterType] = oClCl
			mds.EXPECT().OpsCluster().Return(oCL)
			oCL.EXPECT().Fetch(ctx, "clID").Return(clusterObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
			oClCl.EXPECT().GetDriverTypes().Return([]string{"flex"}).Times(1)
		case 90:
			t.Log("case: PUBLISH and invalid DriverType")
			payload.RequestedOperations = []string{"PUBLISH"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			payload.DriverType = "flex"
			payload.FsType = "invalidtype"
			vsObj.BoundClusterID = "clID"
			vsObj.VolumeSeriesState = com.VolStateBound
			oClCl := mockcluster.NewMockClient(mockCtrl)
			hc.clusterClientMap = make(map[string]cluster.Client)
			hc.clusterClientMap[clusterObj.ClusterType] = oClCl
			mds.EXPECT().OpsCluster().Return(oCL)
			oCL.EXPECT().Fetch(ctx, "clID").Return(clusterObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
			oClCl.EXPECT().GetDriverTypes().Return([]string{"flex"}).Times(1)
			oClCl.EXPECT().GetFileSystemTypes().Return([]string{"ext4"}).Times(1)
		case 91:
			t.Log("case: UNPUBLISH and volume is not published")
			testutils.Clone(vsObj, &vscVol)
			payload.RequestedOperations = []string{"UNPUBLISH"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			vscVol.ClusterDescriptor = nil
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
		case 92:
			t.Log("case: ATTACH_FS create but no mount")
			payload.RequestedOperations = []string{"CREATE", "ATTACH_FS"}
			payload.VolumeSeriesID = ""
			payload.FsType = "ext4"
			payload.TargetPath = "/foo"
			mds.EXPECT().OpsAccount().Return(oA)
			oA.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesCreateSpec.AccountID)).Return(aObj, nil)
			mds.EXPECT().OpsServicePlan().Return(oSP)
			oSP.EXPECT().Fetch(ctx, string(spObj.Meta.ID)).Return(spObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, gomock.Any()).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oV.EXPECT().Count(ctx, gomock.Any(), uint(1)).Return(0, nil)
			expErr = centrald.ErrorMissing
		case 93:
			t.Log("case: ATTACH_FS and no volume specified")
			payload.RequestedOperations = []string{"ATTACH_FS"}
			payload.VolumeSeriesID = ""
			payload.FsType = "ext4"
			payload.TargetPath = "/foo"
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, "").Return(nil, centrald.ErrorNotFound)
			expErr = centrald.ErrorMissing
		case 94:
			t.Log("case: ATTACH_FS and invalid volume state")
			testutils.Clone(vsObj, &vscVol)
			vscVol.VolumeSeriesState = com.VolStateBound
			payload.RequestedOperations = []string{"ATTACH_FS"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			payload.FsType = "ext4"
			payload.TargetPath = "/foo"
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
		case 95:
			t.Log("case: ATTACH_FS and head not mounted")
			testutils.Clone(vsObj, &vscVol)
			vscVol.VolumeSeriesState = com.VolStateInUse
			vscVol.Mounts = []*models.Mount{
				{MountedNodeID: models.ObjIDMutable(nodeObj.Meta.ID), SnapIdentifier: "snap", MountState: "MOUNTED"},
			}
			payload.RequestedOperations = []string{"ATTACH_FS"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			payload.FsType = "ext4"
			payload.TargetPath = "/foo"
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
		case 96:
			t.Log("case: ATTACH_FS already attached")
			testutils.Clone(vsObj, &vscVol)
			vscVol.VolumeSeriesState = com.VolStateInUse
			vscVol.Mounts = []*models.Mount{
				{MountedNodeID: models.ObjIDMutable(nodeObj.Meta.ID), SnapIdentifier: com.VolMountHeadIdentifier, MountState: "MOUNTED"},
			}
			vscVol.SystemTags = []string{fmt.Sprintf("%s:/foo", com.SystemTagVolumeFsAttached)}
			payload.RequestedOperations = []string{"ATTACH_FS"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			payload.FsType = "ext4"
			payload.TargetPath = "/foo"
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
		case 97:
			t.Log("case: ATTACH_FS targetPath missing")
			testutils.Clone(vsObj, &vscVol)
			vscVol.VolumeSeriesState = com.VolStateInUse
			payload.RequestedOperations = []string{"ATTACH_FS"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			payload.FsType = "ext4"
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
		case 98:
			t.Log("case: DETACH_FS and no volume specified")
			payload.RequestedOperations = []string{"DETACH_FS"}
			payload.VolumeSeriesID = ""
			payload.FsType = "ext4"
			payload.TargetPath = "/foo"
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, "").Return(nil, centrald.ErrorNotFound)
			expErr = centrald.ErrorMissing
		case 99:
			t.Log("case: DETACH_FS and invalid volume state")
			testutils.Clone(vsObj, &vscVol)
			vscVol.VolumeSeriesState = com.VolStateBound
			payload.RequestedOperations = []string{"DETACH_FS"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			payload.FsType = "ext4"
			payload.TargetPath = "/foo"
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
		case 100:
			t.Log("case: DETACH_FS head is not mounted")
			testutils.Clone(vsObj, &vscVol)
			vscVol.VolumeSeriesState = com.VolStateInUse
			vscVol.Mounts = []*models.Mount{
				{MountedNodeID: models.ObjIDMutable(nodeObj.Meta.ID), SnapIdentifier: "snap", MountState: "MOUNTED"},
			}
			payload.RequestedOperations = []string{"DETACH_FS"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			payload.FsType = "ext4"
			payload.TargetPath = "/foo"
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
		case 101:
			t.Log("case: DETACH_FS not attached")
			testutils.Clone(vsObj, &vscVol)
			vscVol.VolumeSeriesState = com.VolStateInUse
			vscVol.Mounts = []*models.Mount{
				{MountedNodeID: models.ObjIDMutable(nodeObj.Meta.ID), SnapIdentifier: com.VolMountHeadIdentifier, MountState: "MOUNTED"},
			}
			payload.RequestedOperations = []string{"DETACH_FS"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			payload.FsType = "ext4"
			payload.TargetPath = "/foo"
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
		case 102:
			t.Log("case: DETACH_FS targetPath missing")
			testutils.Clone(vsObj, &vscVol)
			vscVol.VolumeSeriesState = com.VolStateInUse
			payload.RequestedOperations = []string{"DETACH_FS"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			payload.FsType = "ext4"
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
		case 103:
			t.Log("case: CREATE vsr list fails")
			payload.RequestedOperations = []string{"CREATE"}
			mds.EXPECT().OpsAccount().Return(oA)
			oA.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesCreateSpec.AccountID)).Return(aObj, nil)
			mds.EXPECT().OpsServicePlan().Return(oSP)
			oSP.EXPECT().Fetch(ctx, string(spObj.Meta.ID)).Return(spObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oV.EXPECT().Count(ctx, gomock.Any(), uint(1)).Return(0, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, gomock.Any()).Return(nil, centrald.ErrorDbError)
			expErr = centrald.ErrorDbError
		case 104:
			t.Log("case: CREATE vsr list returns existing vsr")
			payload.RequestedOperations = []string{"CREATE"}
			vsrObj = &models.VolumeSeriesRequest{
				VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
					Meta: &models.ObjMeta{
						ID: "someID",
					},
				},
				VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
					VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{
						VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
							Name: params.Payload.VolumeSeriesCreateSpec.Name,
						},
					},
					RequestedOperations: []string{com.VolReqOpCreate},
				},
			}
			vrlParams := ops.VolumeSeriesRequestListParams{
				IsTerminated: swag.Bool(false),
				AccountID:    swag.String(string(params.Payload.VolumeSeriesCreateSpec.AccountID)),
			}
			mds.EXPECT().OpsAccount().Return(oA)
			oA.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesCreateSpec.AccountID)).Return(aObj, nil)
			mds.EXPECT().OpsServicePlan().Return(oSP)
			oSP.EXPECT().Fetch(ctx, string(spObj.Meta.ID)).Return(spObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oV.EXPECT().Count(ctx, gomock.Any(), uint(1)).Return(0, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, vrlParams).Return([]*models.VolumeSeriesRequest{vsrObj}, nil)
			expErr = centrald.ErrorRequestInConflict
		case 105:
			t.Log("case: CREATE vs list fails")
			payload.RequestedOperations = []string{"CREATE"}
			mds.EXPECT().OpsAccount().Return(oA)
			oA.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesCreateSpec.AccountID)).Return(aObj, nil)
			mds.EXPECT().OpsServicePlan().Return(oSP)
			oSP.EXPECT().Fetch(ctx, string(spObj.Meta.ID)).Return(spObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oV.EXPECT().Count(ctx, gomock.Any(), uint(1)).Return(0, centrald.ErrorDbError)
			expErr = centrald.ErrorDbError
		case 106:
			t.Log("case: CREATE vsr list returns existing vsr")
			payload.RequestedOperations = []string{"CREATE"}
			vsObj = &models.VolumeSeries{
				VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
					Meta: &models.ObjMeta{
						ID: "someID",
					},
				},
			}
			vlParams := volume_series.VolumeSeriesListParams{
				AccountID: swag.String(string(params.Payload.VolumeSeriesCreateSpec.AccountID)),
				Name:      swag.String(string(params.Payload.VolumeSeriesCreateSpec.Name)),
			}
			mds.EXPECT().OpsAccount().Return(oA)
			oA.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesCreateSpec.AccountID)).Return(aObj, nil)
			mds.EXPECT().OpsServicePlan().Return(oSP)
			oSP.EXPECT().Fetch(ctx, string(spObj.Meta.ID)).Return(spObj, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oV.EXPECT().Count(ctx, vlParams, uint(1)).Return(1, nil)
			expErr = centrald.ErrorExists
		case 107:
			t.Log("case: UNBIND invalid state")
			payload.RequestedOperations = []string{com.VolReqOpUnbind}
			vsUnbind.VolumeSeriesState = com.VolStateConfigured
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsUnbind, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
		case 108:
			t.Log("case: UNBIND final snapshot needed")
			payload.RequestedOperations = []string{com.VolReqOpUnbind}
			vsUnbind.VolumeSeriesState = com.VolStateProvisioned
			vsUnbind.LifecycleManagementData.FinalSnapshotNeeded = true
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsUnbind, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
		case 109:
			t.Log("case: VOL_SNAPSHOT_RESTORE invalid snapshotID")
			payload.RequestedOperations = []string{com.VolReqOpVolRestoreSnapshot}
			payload.SnapshotID = "foo"
			fops.RetSnapshotFetchDataErr = centrald.ErrorMissing
			expErr = centrald.ErrorMissing
			vsObj.VolumeSeriesState = com.VolStateInUse
			vsObj.Mounts = []*models.Mount{
				{MountedNodeID: models.ObjIDMutable(nodeObj.Meta.ID), SnapIdentifier: com.VolMountHeadIdentifier, MountState: "MOUNTED"},
			}
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, gomock.Not(gomock.Nil())).Return([]*models.VolumeSeriesRequest{}, nil)
		case 110:
			t.Log("case: CREATE_FROM_SNAPSHOT invalid snapshotID")
			payload.RequestedOperations = []string{com.VolReqOpCreateFromSnapshot}
			payload.SnapshotID = "foo"
			fops.RetSnapshotFetchDataErr = centrald.ErrorMissing
			expErr = centrald.ErrorMissing
		case 111:
			t.Log("case: VOL_SNAPSHOT_CREATE cannot retrieve volume account")
			fops.RetAccountFetchObj = nil
			fops.RetAccountFetchErr = centrald.ErrorMissing
			payload.RequestedOperations = []string{"VOL_SNAPSHOT_CREATE"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsObj.Meta.ID)
			payload.SyncCoordinatorID = "coordinatorVSRId"
			testutils.Clone(vsObj, &vscVol)
			vscVol.VolumeSeriesState = "PROVISIONED"
			vscVol.AccountID = models.ObjIDMutable(aObj.Meta.ID)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, string(vsObj.Meta.ID)).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(1)
			oVR.EXPECT().List(ctx, gomock.Any()).Return([]*models.VolumeSeriesRequest{}, nil)
		case 112:
			t.Log("case: loadNodeObject fails")
			payload.RequestedOperations = []string{"RENAME"} // the op doesn't matter, this is minimal
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsNode().Return(oN)
			oN.EXPECT().Fetch(ctx, string(payload.NodeID)).Return(nil, centrald.ErrorNotFound)
		case 113:
			t.Log("case: Node in invalid state")
			payload.RequestedOperations = []string{"RENAME"} // the op doesn't matter, this is minimal
			nodeObj.State = com.NodeStateTearDown
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsNode().Return(oN)
			oN.EXPECT().Fetch(ctx, string(payload.NodeID)).Return(nodeObj, nil)
			expErr = centrald.ErrorInvalidState
		case 114:
			t.Log("case: Node nil service")
			payload.RequestedOperations = []string{"RENAME"} // the op doesn't matter, this is minimal
			nodeObj.State = com.NodeStateTearDown
			nodeObj.Service = nil
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsNode().Return(oN)
			oN.EXPECT().Fetch(ctx, string(payload.NodeID)).Return(nodeObj, nil)
			expErr = centrald.ErrorInvalidState
		case 115:
			t.Log("case: Node bad service state")
			payload.RequestedOperations = []string{"RENAME"} // the op doesn't matter, this is minimal
			nodeObj.State = com.NodeStateTearDown
			nodeObj.Service = &models.NuvoService{}
			nodeObj.Service.State = com.ServiceStateUnknown
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vsObj, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
			mds.EXPECT().OpsNode().Return(oN)
			oN.EXPECT().Fetch(ctx, string(payload.NodeID)).Return(nodeObj, nil)
			expErr = centrald.ErrorInvalidState
		case 116:
			t.Log("case: UNMOUNT and fs attached")
			payload.RequestedOperations = []string{"UNMOUNT"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			payload.SnapIdentifier = "HEAD"
			testutils.Clone(vsObj, &vscVol)
			vscVol.VolumeSeriesState = com.VolStateInUse
			vscVol.Mounts = []*models.Mount{
				{MountedNodeID: models.ObjIDMutable(nodeObj.Meta.ID), SnapIdentifier: com.VolMountHeadIdentifier, MountState: "MOUNTED"},
			}
			vscVol.SystemTags = []string{fmt.Sprintf("%s:/mnt", com.SystemTagVolumeFsAttached)}
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
			expErr = &centrald.Error{C: expErr.C, M: ".*UNMOUNT requires filesystem to not be attached"}
		case 117:
			t.Log("case: VOL_SNAPSHOT_CREATE no protection domain")
			payload.RequestedOperations = []string{"VOL_SNAPSHOT_CREATE"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			var acObj *models.Account
			testutils.Clone(aObj, &acObj)
			acObj.ProtectionDomains = make(map[string]models.ObjIDMutable)
			acObj.ProtectionDomains[com.ProtectionStoreDefaultKey] = models.ObjIDMutable("")
			fops.RetAccountFetchObj = acObj
			testutils.Clone(vsObj, &vscVol)
			vscVol.VolumeSeriesState = "IN_USE"
			vscVol.AccountID = models.ObjIDMutable(acObj.Meta.ID)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
		case 118:
			t.Log("case: VOL_SNAPSHOT_CREATE no Snapshot Catalog Policy")
			payload.RequestedOperations = []string{"VOL_SNAPSHOT_CREATE"}
			payload.VolumeSeriesID = models.ObjIDMutable(vsID)
			var acObj *models.Account
			testutils.Clone(aObj, &acObj)
			acObj.SnapshotCatalogPolicy = &models.SnapshotCatalogPolicy{
				CspDomainID:        "domID",
				ProtectionDomainID: "",
			}
			fops.RetAccountFetchObj = acObj
			testutils.Clone(vsObj, &vscVol)
			vscVol.VolumeSeriesState = "IN_USE"
			vscVol.AccountID = models.ObjIDMutable(acObj.Meta.ID)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
			oVS.EXPECT().Fetch(ctx, vsID).Return(vscVol, nil)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().List(ctx, listParams).Return([]*models.VolumeSeriesRequest{}, nil)
		default:
			assert.Fail("unhandled", "case %d", tc)
		}
		hc.DS = mds
		cntLock = hc.cntLock
		cntUnlock = hc.cntUnlock
		ret = hc.volumeSeriesRequestCreate(ops.VolumeSeriesRequestCreateParams{HTTPRequest: requestWithAuthContext(ai), Payload: payload})
		assert.NotNil(ret)
		mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
		assert.True(ok)
		assert.Equal(expErr.C, int(mE.Payload.Code))
		assert.Regexp("^"+expErr.M, *mE.Payload.Message)
		assert.Equal(cntLock+expInc, hc.cntLock)
		assert.Equal(cntUnlock+expInc, hc.cntUnlock)
		tl.Flush()
	}

	// no payload
	params.Payload = nil
	t.Log("case: nil payload")
	ret = hc.volumeSeriesRequestCreate(params)
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesRequestCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorMissing.M, *mE.Payload.Message)
}

func TestVolumeSeriesRequestDelete(t *testing.T) {
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
	params := ops.VolumeSeriesRequestDeleteParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectId",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:      "objectId",
				Version: 2,
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{
				VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
					AccountID:       "aid1",
					TenantAccountID: "tid1",
				},
			},
		},
	}
	obj.VolumeSeriesRequestState = "FAILED"
	obj.ClusterID = "clusterID"
	obj.NodeID = "nodeID"
	obj.VolumeSeriesID = "volumeSeriesID"
	rObj := &models.Role{}
	rObj.Capabilities = map[string]bool{centrald.CSPDomainUsageCap: true, centrald.VolumeSeriesOwnerCap: true}
	trObj := &models.Role{}
	trObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}

	// success, no active SR claims
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oVR := mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Delete(ctx, params.ID).Return(nil)
	oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oSR := mock.NewMockStorageRequestOps(mockCtrl)
	srLParams := storage_request.NewStorageRequestListParams()
	srLParams.IsTerminated = swag.Bool(false)
	srLParams.VolumeSeriesRequestID = swag.String(params.ID)
	oSR.EXPECT().List(ctx, srLParams).Return([]*models.StorageRequest{}, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(1)
	mds.EXPECT().OpsStorageRequest().Return(oSR)
	t.Log("case: success, no SR claims")
	hc.DS = mds
	var ret middleware.Responder
	cntLock := hc.cntLock
	cntUnlock := hc.cntUnlock
	assert.Nil(evM.InSSProps)
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestDelete(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.VolumeSeriesRequestDeleteNoContent)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Len(evM.InSSProps, 5)
	assert.EqualValues("clusterID", evM.InSSProps["clusterId"])
	assert.EqualValues("2", evM.InSSProps["meta.version"])
	assert.EqualValues("nodeID", evM.InSSProps["nodeId"])
	assert.EqualValues("FAILED", evM.InSSProps["volumeSeriesRequestState"])
	assert.EqualValues("volumeSeriesID", evM.InSSProps["volumeSeriesId"])
	assert.Equal(obj, evM.InACScope)
	tl.Flush()

	// failure, SR list error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().List(ctx, srLParams).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(1)
	mds.EXPECT().OpsStorageRequest().Return(oSR)
	t.Log("case: SR list failure")
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestDelete(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.VolumeSeriesRequestDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	// failure, active SR claims
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	srLRet := []*models.StorageRequest{&models.StorageRequest{}}
	oSR.EXPECT().List(ctx, srLParams).Return(srLRet, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(1)
	mds.EXPECT().OpsStorageRequest().Return(oSR)
	t.Log("case: SR list failure")
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesRequestDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorExists.M+".*has claims in active StorageRequest", *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	// delete failure case, cover valid tenant
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	ai.AccountID, ai.RoleObj = "tid1", trObj
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Delete(ctx, params.ID).Return(centrald.ErrorDbError)
	oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oSR = mock.NewMockStorageRequestOps(mockCtrl)
	oSR.EXPECT().List(ctx, srLParams).Return([]*models.StorageRequest{}, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(1)
	mds.EXPECT().OpsStorageRequest().Return(oSR)
	t.Log("case: delete failure")
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesRequestDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesRequestDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized")
	ai.AccountID, ai.RoleObj = "otherID", &models.Role{}
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesRequestDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	tl.Flush()

	// request active case, cover valid account
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: request active")
	ai.AccountID, ai.RoleObj = "aid1", rObj
	obj.VolumeSeriesRequestState = "PROVISIONED"
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(1)
	t.Log("case: request active")
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesRequestDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Regexp("still active", *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	// fetch failure case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(1)
	t.Log("case: fetch failure")
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesRequestDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
	assert.Equal(cntLock, hc.cntLock)
	assert.Equal(cntUnlock, hc.cntUnlock)
}

func TestVolumeSeriesRequestFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.VolumeSeriesRequestFetchParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectId",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "objectId",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{
				VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
					AccountID:       "aid1",
					TenantAccountID: "tid1",
				},
			},
		},
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oVR := mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(1)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestFetch(params) })
	assert.NotNil(ret)
	mR, ok := ret.(*ops.VolumeSeriesRequestFetchOK)
	assert.True(ok)
	assert.Equal(obj, mR.Payload)
	tl.Flush()

	// fetch failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(1)
	t.Log("case: fetch failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestFetch(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.VolumeSeriesRequestFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesRequestFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: unauthorized")
	ai.AccountID, ai.RoleObj = "tid1", &models.Role{}
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesRequestFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
}

func TestVolumeSeriesRequestList(t *testing.T) {
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
	params := ops.VolumeSeriesRequestListParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	ctx := params.HTTPRequest.Context()
	objects := []*models.VolumeSeriesRequest{
		&models.VolumeSeriesRequest{
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "objectId",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{
					VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
						AccountID:       "aid1",
						TenantAccountID: "tid1",
					},
				},
			},
		},
		&models.VolumeSeriesRequest{
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{ID: "objectId2"},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{
					VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
						AccountID:       "aid2",
						TenantAccountID: "tid1",
					},
				},
			},
		},
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oVR := mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().List(ctx, params).Return(objects, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(1)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestList(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.VolumeSeriesRequestListOK)
	assert.True(ok)
	assert.Equal(objects, mO.Payload)
	assert.Equal(1, fops.CntConstrainEOQueryAccounts)
	assert.Nil(fops.InConstrainEOQueryAccountsAID)
	assert.Nil(fops.InConstrainEOQueryAccountsTID)
	tl.Flush()

	// list fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().List(ctx, params).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(1)
	t.Log("case: List failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestList(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.VolumeSeriesRequestListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.VolumeSeriesRequestListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
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
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().List(ctx, cParams).Return(objects, nil)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.VolumeSeriesRequestListOK)
	assert.True(ok)
	assert.Equal(3, fops.CntConstrainEOQueryAccounts)
	assert.Equal(params.AccountID, fops.InConstrainEOQueryAccountsAID)
	assert.Equal(params.TenantAccountID, fops.InConstrainEOQueryAccountsTID)
	tl.Flush()

	t.Log("case: constrainEitherOrQueryAccounts nil accounts")
	ai.RoleObj = &models.Role{}
	params.AccountID, params.TenantAccountID = swag.String("a1"), swag.String("t1")
	fops.RetConstrainEOQueryAccountsAID, fops.RetConstrainEOQueryAccountsTID = nil, nil
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.VolumeSeriesRequestListOK)
	assert.True(ok)
	assert.Equal(4, fops.CntConstrainEOQueryAccounts)
	assert.Equal(params.AccountID, fops.InConstrainEOQueryAccountsAID)
	assert.Equal(params.TenantAccountID, fops.InConstrainEOQueryAccountsTID)
	tl.Flush()

	t.Log("case: constrainEitherOrQueryAccounts error")
	ai.RoleObj = &models.Role{}
	params.AccountID, params.TenantAccountID = swag.String("a1"), swag.String("t1")
	fops.RetConstrainEOQueryAccountsErr = centrald.ErrorDbError
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.VolumeSeriesRequestListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	assert.Equal(5, fops.CntConstrainEOQueryAccounts)
}

func TestVolumeSeriesRequestUpdate(t *testing.T) {
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

	// make the name map
	var nMap JSONToAttrNameMap
	assert.NotPanics(func() { nMap = hc.volumeSeriesRequestMutableNameMap() })
	// validate some embedded properties
	assert.Equal("requestMessages", nMap.jName("RequestMessages"))
	assert.Equal("volumeSeriesId", nMap.jName("VolumeSeriesID"))

	// parse params
	sf := models.StorageFormulaName(app.SupportedStorageFormulas()[0].Name)
	storagePlan := &models.StoragePlan{
		LayoutAlgorithm: "TBD",
		PlacementHints:  map[string]models.ValueType{},
		StorageElements: []*models.StoragePlanStorageElement{
			&models.StoragePlanStorageElement{
				Intent:    "DATA",
				SizeBytes: swag.Int64(1010),
				StorageParcels: map[string]models.StorageParcelElement{
					"st1":         {SizeBytes: swag.Int64(10)},
					"STORAGE":     {SizeBytes: swag.Int64(1000)},
					"STORAGE-3-2": {SizeBytes: swag.Int64(1000), ProvMinSizeBytes: swag.Int64(2), ProvParcelSizeBytes: swag.Int64(3), ProvRemainingSizeBytes: swag.Int64(4)},
				},
				PoolID: "prov1",
			},
			&models.StoragePlanStorageElement{
				Intent:    "CACHE",
				SizeBytes: swag.Int64(1010),
				StorageParcels: map[string]models.StorageParcelElement{
					"SSD": {ProvMinSizeBytes: swag.Int64(0)},
				},
				PoolID: "",
			},
		},
	}
	objM := &models.VolumeSeriesRequestMutable{}
	objM.VolumeSeriesRequestState = com.VolReqStateSizing
	objM.StorageFormula = sf
	objM.StoragePlan = storagePlan
	objM.VolumeSeriesID = "vsID"
	objM.ServicePlanAllocationID = "spaID"
	objM.ConsistencyGroupID = "cgID"
	params := ops.VolumeSeriesRequestUpdateParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectId",
		Version:     8,
		Remove:      []string{},
		Append:      []string{},
		Set:         []string{nMap.jName("VolumeSeriesRequestState"), nMap.jName("StorageFormula"), "storagePlan.layoutAlgorithm", nMap.jName("VolumeSeriesID"), nMap.jName("ServicePlanAllocationID"), nMap.jName("ConsistencyGroupID")},
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
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, &params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM := updateArgsMatcher(t, ua).Matcher()

	obj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:      models.ObjID(params.ID),
				Version: 8,
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{
				VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
					AccountID:       "aid1",
					TenantAccountID: "tid1",
				},
			},
		},
	}
	nodeObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta:      &models.ObjMeta{ID: "nodeId"},
			ClusterID: "clusterId",
		},
	}
	provObj := &models.Pool{
		PoolAllOf0: models.PoolAllOf0{
			Meta: &models.ObjMeta{
				ID: "prov1",
			},
		},
	}
	sObj := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta: &models.ObjMeta{
				ID: "st1",
			},
		},
	}
	vsObj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID(objM.VolumeSeriesID),
			},
		},
	}
	spaObj := &models.ServicePlanAllocation{
		ServicePlanAllocationAllOf0: models.ServicePlanAllocationAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID(objM.ServicePlanAllocationID),
			},
		},
	}
	cgObj := &models.ConsistencyGroup{
		ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID(objM.ConsistencyGroupID),
			},
		},
		ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
			AccountID: "aid1",
		},
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	var ret middleware.Responder
	t.Log("case: update success")
	mds := mock.NewMockDataStore(mockCtrl)
	oSPA := mock.NewMockServicePlanAllocationOps(mockCtrl)
	mds.EXPECT().OpsServicePlanAllocation().Return(oSPA).MinTimes(1)
	oSPA.EXPECT().Fetch(ctx, string(params.Payload.ServicePlanAllocationID)).Return(spaObj, nil)
	oV := mock.NewMockVolumeSeriesOps(mockCtrl)
	mds.EXPECT().OpsVolumeSeries().Return(oV)
	oV.EXPECT().Fetch(ctx, string(params.Payload.VolumeSeriesID)).Return(vsObj, nil)
	oCG := mock.NewMockConsistencyGroupOps(mockCtrl)
	mds.EXPECT().OpsConsistencyGroup().Return(oCG)
	oCG.EXPECT().Fetch(ctx, string(params.Payload.ConsistencyGroupID)).Return(cgObj, nil)
	oVR := mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(1)
	oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	var retObj *models.VolumeSeriesRequest
	testutils.Clone(obj, &retObj)
	retObj.Meta.Version++
	retObj.ClusterID = "clusterID"
	retObj.NodeID = "nodeID"
	retObj.VolumeSeriesRequestState = "FAILED"
	retObj.VolumeSeriesID = "volumeSeriesID"
	retObj.SyncPeers = map[string]models.SyncPeer{"p1": models.SyncPeer{}}
	oVR.EXPECT().Update(ctx, uaM, params.Payload).Return(retObj, nil)
	hc.DS = mds
	assert.Nil(evM.InSSProps)
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestUpdate(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.VolumeSeriesRequestUpdateOK)
	if assert.True(ok) {
		assert.Equal(retObj, mD.Payload)
	}
	assert.Len(evM.InSSProps, 6)
	assert.EqualValues("clusterID", evM.InSSProps["clusterId"])
	assert.EqualValues("9", evM.InSSProps["meta.version"])
	assert.EqualValues("nodeID", evM.InSSProps["nodeId"])
	assert.EqualValues("FAILED", evM.InSSProps["volumeSeriesRequestState"])
	assert.EqualValues("volumeSeriesID", evM.InSSProps["volumeSeriesId"])
	assert.EqualValues(string(retObj.Meta.ID), evM.InSSProps["syncCoordinatorId"])
	assert.Equal(retObj, evM.InACScope)
	tl.Flush()

	// success: StorageFormula, ServicePlanAllocationID can be cleared, NodeID and VolumeSeriesID no-op is allowed, CapacityReservationPlan/Result success path
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: StorageFormula, ServicePlanAllocationID can be cleared, NodeID and VolumeSeriesID no-op is allowed")
	capPlan := &models.CapacityReservationPlan{
		StorageTypeReservations: map[string]models.StorageTypeReservation{
			"Amazon gp2": {NumMirrors: 1, SizeBytes: swag.Int64(10)},
		},
	}
	capRes := &models.CapacityReservationResult{
		CurrentReservations: map[string]models.PoolReservation{
			"prov1": {NumMirrors: 1, SizeBytes: swag.Int64(1001)},
		},
		DesiredReservations: map[string]models.PoolReservation{
			"prov1": {NumMirrors: 2, SizeBytes: swag.Int64(1001)},
			"prov2": {NumMirrors: 2, SizeBytes: swag.Int64(1002)},
		},
	}
	objM.NodeID = "nodeID"
	objM.VolumeSeriesID = "volumeSeriesID"
	objM.StorageFormula = ""
	objM.ServicePlanAllocationID = ""
	objM.CapacityReservationPlan = capPlan
	objM.CapacityReservationResult = capRes
	params.Set = []string{"nodeId", "storageFormula", "servicePlanAllocationId", "volumeSeriesId", "capacityReservationPlan", "capacityReservationResult"}
	uP[centrald.UpdateSet] = params.Set
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, &params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).Matcher()
	mds = mock.NewMockDataStore(mockCtrl)
	oP := mock.NewMockPoolOps(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
	oP.EXPECT().Fetch(ctx, "prov1").Return(provObj, nil)
	oP.EXPECT().Fetch(ctx, "prov2").Return(provObj, nil)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).MinTimes(1)
	obj.NodeID = "nodeID"
	obj.VolumeSeriesID = "volumeSeriesID"
	oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	testutils.Clone(obj, &retObj)
	retObj.Meta.Version++
	retObj.ClusterID = "clusterID"
	retObj.VolumeSeriesRequestState = "FAILED"
	retObj.SyncPeers = map[string]models.SyncPeer{} // empty
	retObj.SyncCoordinatorID = "syncCoordinatorID"
	oVR.EXPECT().Update(ctx, uaM, params.Payload).Return(retObj, nil)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.VolumeSeriesRequestUpdateOK)
	assert.True(ok)
	assert.Equal(retObj, mD.Payload)
	assert.Len(evM.InSSProps, 6)
	assert.EqualValues("clusterID", evM.InSSProps["clusterId"])
	assert.EqualValues("9", evM.InSSProps["meta.version"])
	assert.EqualValues("nodeID", evM.InSSProps["nodeId"])
	assert.EqualValues("syncCoordinatorID", evM.InSSProps["parentId"])
	assert.EqualValues("FAILED", evM.InSSProps["volumeSeriesRequestState"])
	assert.EqualValues("volumeSeriesID", evM.InSSProps["volumeSeriesId"])
	assert.Equal(retObj, evM.InACScope)
	tl.Flush()
	obj.NodeID = ""                        // reset
	obj.VolumeSeriesID = ""                // reset
	objM.NodeID = ""                       // reset
	objM.StorageFormula = sf               // reset
	objM.VolumeSeriesID = "vsID"           // reset
	objM.ServicePlanAllocationID = "spaID" // reset

	// Update failed, NodeID, VolumeSeriesID, StoragePlan success path
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: fail on update")
	objM.NodeID = "nodeID"
	objM.StoragePlan = storagePlan
	params.Set = []string{"nodeId", "volumeSeriesRequestState", "storagePlan"}
	uP[centrald.UpdateSet] = params.Set
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, &params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).Matcher()
	mds = mock.NewMockDataStore(mockCtrl)
	oN := mock.NewMockNodeOps(mockCtrl)
	mds.EXPECT().OpsNode().Return(oN)
	oN.EXPECT().Fetch(ctx, string(params.Payload.NodeID)).Return(nodeObj, nil)
	oP = mock.NewMockPoolOps(mockCtrl)
	mds.EXPECT().OpsPool().Return(oP)
	oP.EXPECT().Fetch(ctx, "prov1").Return(provObj, nil)
	oS := mock.NewMockStorageOps(mockCtrl)
	mds.EXPECT().OpsStorage().Return(oS)
	oS.EXPECT().Fetch(ctx, "st1").Return(sObj, nil)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR).Times(2)
	oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oVR.EXPECT().Update(ctx, uaM, params.Payload).Return(nil, centrald.ErrorDbError)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestUpdate(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.VolumeSeriesRequestUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	tl.Flush()
	objM.NodeID = "" // reset

	t.Log("case: unauthorized")
	ai.AccountID = "tid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.CSPDomainManagementCap: true}
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestUpdate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesRequestUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	ai.AccountID, ai.RoleObj = "", nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestUpdate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesRequestUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	// various error cases
	for tc := 1; tc <= 34; tc++ {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		mds = mock.NewMockDataStore(mockCtrl)
		obj.NodeID = ""
		obj.VolumeSeriesID = ""
		expErr := centrald.ErrorUpdateInvalidRequest
		switch tc {
		case 1:
			t.Log("case: invalid volumeSeriesRequestState")
			params.Set = []string{"volumeSeriesRequestState"}
			objM.VolumeSeriesRequestState = "--Unsupported--"
		case 2:
			t.Log("case: VolumeSeries not found")
			params.Set = []string{nMap.jName("VolumeSeriesID")}
			oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			oV = mock.NewMockVolumeSeriesOps(mockCtrl)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oV.EXPECT().Fetch(ctx, string(objM.VolumeSeriesID)).Return(nil, centrald.ErrorNotFound)
		case 3:
			t.Log("case: VolumeSeries lookup fails")
			params.Set = []string{nMap.jName("VolumeSeriesID")}
			oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			oV = mock.NewMockVolumeSeriesOps(mockCtrl)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			oV.EXPECT().Fetch(ctx, string(objM.VolumeSeriesID)).Return(nil, centrald.ErrorDbError)
			expErr = centrald.ErrorDbError
		case 4:
			t.Log("case: VolumeSeriesRequest not found")
			params.Set = []string{nMap.jName("VolumeSeriesID")}
			oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
			expErr = centrald.ErrorNotFound
		case 5:
			t.Log("case: VolumeSeriesID cannot be changed once set")
			obj.VolumeSeriesID = "vs0"
			params.Set = []string{nMap.jName("VolumeSeriesID")}
			oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
		case 6:
			t.Log("case: storage formula not found")
			params.Set = []string{nMap.jName("StorageFormula")}
			objM.StorageFormula = "Invalid"
		case 7:
			t.Log("case: nil capacityReservationPlan")
			params.Set = []string{nMap.jName("CapacityReservationPlan")}
			objM.CapacityReservationPlan = nil
		case 8:
			t.Log("case: capacityReservationResult.desiredReservations not found")
			params.Set = []string{"capacityReservationResult.desiredReservations"}
			objM.CapacityReservationResult = capRes
			oP = mock.NewMockPoolOps(mockCtrl)
			mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
			oP.EXPECT().Fetch(ctx, "prov1").Return(provObj, nil).AnyTimes()
			oP.EXPECT().Fetch(ctx, "prov2").Return(nil, centrald.ErrorNotFound)
		case 9:
			t.Log("case: capacityReservationResult.desiredReservations db error")
			params.Set = []string{nMap.jName("CapacityReservationResult")}
			objM.CapacityReservationResult = capRes
			oP = mock.NewMockPoolOps(mockCtrl)
			mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
			oP.EXPECT().Fetch(ctx, "prov1").Return(nil, centrald.ErrorDbError)
			oP.EXPECT().Fetch(ctx, "prov2").Return(provObj, nil).AnyTimes()
			expErr = centrald.ErrorDbError
		case 10:
			t.Log("case: nil StoragePlan")
			params.Set = []string{nMap.jName("StoragePlan")}
			objM.StoragePlan = nil
		case 11:
			t.Log("case: storagePlan.storageElements provider not found")
			params.Set = []string{"storagePlan.storageElements"}
			objM.StoragePlan = storagePlan
			oP = mock.NewMockPoolOps(mockCtrl)
			mds.EXPECT().OpsPool().Return(oP)
			oP.EXPECT().Fetch(ctx, "prov1").Return(nil, centrald.ErrorNotFound)
		case 12:
			t.Log("case: storagePlan provider db error")
			params.Set = []string{"storagePlan"}
			objM.StoragePlan = storagePlan
			oP = mock.NewMockPoolOps(mockCtrl)
			mds.EXPECT().OpsPool().Return(oP)
			oP.EXPECT().Fetch(ctx, "prov1").Return(nil, centrald.ErrorDbError)
			expErr = centrald.ErrorDbError
		case 13:
			t.Log("case: storagePlan storage not found")
			params.Set = []string{"storagePlan"}
			oP = mock.NewMockPoolOps(mockCtrl)
			mds.EXPECT().OpsPool().Return(oP)
			oP.EXPECT().Fetch(ctx, "prov1").Return(provObj, nil)
			oS = mock.NewMockStorageOps(mockCtrl)
			mds.EXPECT().OpsStorage().Return(oS)
			oS.EXPECT().Fetch(ctx, "st1").Return(nil, centrald.ErrorNotFound)
		case 14:
			t.Log("case: storagePlan.storageElements storage db error")
			params.Set = []string{"storagePlan.storageElements"}
			oP = mock.NewMockPoolOps(mockCtrl)
			mds.EXPECT().OpsPool().Return(oP)
			oP.EXPECT().Fetch(ctx, "prov1").Return(provObj, nil)
			oS = mock.NewMockStorageOps(mockCtrl)
			mds.EXPECT().OpsStorage().Return(oS)
			oS.EXPECT().Fetch(ctx, "st1").Return(nil, centrald.ErrorDbError)
			expErr = centrald.ErrorDbError
		case 15:
			t.Log("case: storagePlan.storageElements.intent")
			params.Set = []string{"storagePlan.storageElements"}
			objM.StoragePlan.StorageElements[0].Intent = "PARTY"
		case 16:
			t.Log("case: version mismatch")
			obj.VolumeSeriesID = "vs0"
			params.Version = 2
			params.Set = []string{nMap.jName("VolumeSeriesID")}
			oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			expErr = centrald.ErrorIDVerNotFound
		case 17:
			t.Log("case: VolumeSeriesID is cannot be changed once set")
			obj.VolumeSeriesID = "vs0"
			params.Version = 8
			params.Set = []string{nMap.jName("VolumeSeriesID")}
			oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			expErr = &centrald.Error{C: expErr.C, M: ".*volumeSeriesId cannot be modified"}
		case 18:
			t.Log("case: no change")
			params.Set = []string{}
		case 19:
			t.Log("case: capacityReservationPlan.storageTypeReservations not found")
			params.Set = []string{"capacityReservationPlan.storageTypeReservations"}
			objM.CapacityReservationPlan = &models.CapacityReservationPlan{
				StorageTypeReservations: map[string]models.StorageTypeReservation{
					"invalid-type": {NumMirrors: 1, SizeBytes: swag.Int64(10)},
				},
			}
		case 20:
			t.Log("case: nil capacityReservationResult")
			params.Set = []string{nMap.jName("CapacityReservationResult")}
			objM.CapacityReservationResult = nil
		case 21:
			t.Log("case: capacityReservationResult.currentReservations not found error")
			params.Set = []string{nMap.jName("CapacityReservationResult")}
			capRes2 := &models.CapacityReservationResult{
				CurrentReservations: map[string]models.PoolReservation{
					"prov1": {NumMirrors: 1, SizeBytes: swag.Int64(1001)},
					"prov3": {NumMirrors: 1, SizeBytes: swag.Int64(1003)},
				},
				DesiredReservations: map[string]models.PoolReservation{
					"prov1": {NumMirrors: 2, SizeBytes: swag.Int64(1001)},
					"prov2": {NumMirrors: 2, SizeBytes: swag.Int64(1002)},
				},
			}
			objM.CapacityReservationResult = capRes2
			oP = mock.NewMockPoolOps(mockCtrl)
			mds.EXPECT().OpsPool().Return(oP).MinTimes(1)
			oP.EXPECT().Fetch(ctx, "prov1").Return(provObj, nil).AnyTimes()
			oP.EXPECT().Fetch(ctx, "prov2").Return(provObj, nil).AnyTimes()
			oP.EXPECT().Fetch(ctx, "prov3").Return(nil, centrald.ErrorNotFound)
		case 22:
			t.Log("case: ServicePlanAllocation not found")
			params.Payload.ServicePlanAllocationID = "spaID"
			params.Set = []string{nMap.jName("ServicePlanAllocationID")}
			oSPA = mock.NewMockServicePlanAllocationOps(mockCtrl)
			mds.EXPECT().OpsServicePlanAllocation().Return(oSPA).MinTimes(1)
			oSPA.EXPECT().Fetch(ctx, string(params.Payload.ServicePlanAllocationID)).Return(nil, centrald.ErrorNotFound)
		case 23:
			t.Log("case: invalid volumeSeriesRequest load fails")
			params.Set = []string{"volumeSeriesRequestState"}
			objM.VolumeSeriesRequestState = com.VolReqStateBinding
			oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
			expErr = centrald.ErrorNotFound
		case 24:
			t.Log("case: volumeSeriesRequestState should not modify terminal state")
			params.Set = []string{"volumeSeriesRequestState"}
			objM.VolumeSeriesRequestState = com.VolReqStateBinding
			var retObj *models.VolumeSeriesRequest
			testutils.Clone(obj, &retObj)
			retObj.VolumeSeriesRequestState = com.VolReqStateCanceled
			oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().Fetch(ctx, params.ID).Return(retObj, nil)
			expErr = centrald.ErrorInvalidState
		case 25:
			t.Log("case: volumeSeriesRequestState version mismatch")
			params.Set = []string{"volumeSeriesRequestState"}
			params.Version = 1
			objM.VolumeSeriesRequestState = com.VolReqStateBinding
			var retObj *models.VolumeSeriesRequest
			testutils.Clone(obj, &retObj)
			retObj.Meta.Version = 2
			oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().Fetch(ctx, params.ID).Return(retObj, nil)
			expErr = centrald.ErrorIDVerNotFound
		case 26:
			t.Log("case: NodeID is cannot be changed once set")
			obj.NodeID = "nodeID"
			params.Version = 8
			params.Set = []string{nMap.jName("NodeID")}
			oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			expErr = &centrald.Error{C: expErr.C, M: ".*nodeId cannot be modified"}
		case 27:
			t.Log("case: nodeId version mismatch")
			params.Set = []string{"nodeId"}
			params.Version = 1
			objM.NodeID = "newNodeID"
			var retObj *models.VolumeSeriesRequest
			testutils.Clone(obj, &retObj)
			retObj.Meta.Version = 2
			oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().Fetch(ctx, params.ID).Return(retObj, nil)
			expErr = centrald.ErrorIDVerNotFound
		case 28:
			t.Log("case: node not found")
			params.Set = []string{nMap.jName("NodeID")}
			params.Version = 8
			oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			oN = mock.NewMockNodeOps(mockCtrl)
			mds.EXPECT().OpsNode().Return(oN)
			oN.EXPECT().Fetch(ctx, string(params.Payload.NodeID)).Return(nil, centrald.ErrorNotFound)
			expErr = &centrald.Error{C: expErr.C, M: ".*invalid nodeId"}
		case 29:
			t.Log("case: node db error")
			params.Set = []string{nMap.jName("NodeID")}
			params.Version = 8
			oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			oN = mock.NewMockNodeOps(mockCtrl)
			mds.EXPECT().OpsNode().Return(oN)
			oN.EXPECT().Fetch(ctx, string(params.Payload.NodeID)).Return(nil, centrald.ErrorDbError)
			expErr = centrald.ErrorDbError
		case 30:
			t.Log("case: cgID already set in vsrObj")
			params.Set = []string{nMap.jName("ConsistencyGroupID")}
			params.Version = 8
			params.Payload.ConsistencyGroupID = "newCgID"
			obj.ConsistencyGroupID = "cgID"
			oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			expErr = &centrald.Error{C: expErr.C, M: ".*consistencyGroupId cannot be modified"}
		case 31:
			t.Log("case: cgID version mismatch")
			params.Set = []string{nMap.jName("ConsistencyGroupID")}
			params.Version = 1
			objM.ConsistencyGroupID = "newCgID"
			var retObj *models.VolumeSeriesRequest
			testutils.Clone(obj, &retObj)
			retObj.Meta.Version = 2
			oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().Fetch(ctx, params.ID).Return(retObj, nil)
			expErr = centrald.ErrorIDVerNotFound
		case 32:
			t.Log("case: cg not found")
			params.Set = []string{nMap.jName("ConsistencyGroupID")}
			params.Version = 8
			obj.ConsistencyGroupID = ""
			oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
			mds.EXPECT().OpsConsistencyGroup().Return(oCG)
			oCG.EXPECT().Fetch(ctx, string(params.Payload.ConsistencyGroupID)).Return(nil, centrald.ErrorNotFound)
			expErr = &centrald.Error{C: expErr.C, M: ".*invalid consistencyGroupId"}
		case 33:
			t.Log("case: cg db error")
			params.Set = []string{nMap.jName("ConsistencyGroupID")}
			params.Version = 8
			oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
			mds.EXPECT().OpsConsistencyGroup().Return(oCG)
			oCG.EXPECT().Fetch(ctx, string(params.Payload.ConsistencyGroupID)).Return(nil, centrald.ErrorDbError)
			expErr = centrald.ErrorDbError
		case 34:
			t.Log("case: cg-vs account mismatch")
			params.Set = []string{nMap.jName("ConsistencyGroupID")}
			params.Version = 8
			var retCGObj *models.ConsistencyGroup
			testutils.Clone(cgObj, &retCGObj)
			retCGObj.AccountID = "differentAccountID"
			oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			oVR.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			oCG = mock.NewMockConsistencyGroupOps(mockCtrl)
			mds.EXPECT().OpsConsistencyGroup().Return(oCG)
			oCG.EXPECT().Fetch(ctx, string(params.Payload.ConsistencyGroupID)).Return(retCGObj, nil)
			expErr = &centrald.Error{C: expErr.C, M: ".*consistencyGroup and volumeSeries account mismatch"}
		default:
			assert.True(false)
		}
		hc.DS = mds
		assert.NotPanics(func() { ret = hc.volumeSeriesRequestUpdate(params) })
		assert.NotNil(ret)
		mE, ok = ret.(*ops.VolumeSeriesRequestUpdateDefault)
		assert.True(ok)
		assert.Equal(expErr.C, int(mE.Payload.Code))
		assert.Regexp("^"+expErr.M, *mE.Payload.Message)
		tl.Flush()
	}

	// no payload
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: no payload")
	mds = mock.NewMockDataStore(mockCtrl)
	params.Set = []string{"storagePlan"}
	params.Payload = nil
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.volumeSeriesRequestUpdate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.VolumeSeriesRequestUpdateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mE.Payload.Code))
		assert.Equal(centrald.ErrorUpdateInvalidRequest.M, *mE.Payload.Message)
	}
}

type volumeSeriesRequestMatcher struct {
	t           *testing.T
	CreateParam *models.VolumeSeriesRequestCreateArgs
	ListParam   ops.VolumeSeriesRequestListParams
	isList      bool
}

func newVolumeSeriesRequestMatcher(t *testing.T, param interface{}) *volumeSeriesRequestMatcher {
	m := &volumeSeriesRequestMatcher{t: t}
	switch p := param.(type) {
	case *models.VolumeSeriesRequestCreateArgs:
		m.CreateParam = p
	case ops.VolumeSeriesRequestListParams:
		m.ListParam = p
		m.isList = true
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *volumeSeriesRequestMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case *models.VolumeSeriesRequestCreateArgs:
		// ignore completeBy time
		o.CreateParam.CompleteByTime = p.CompleteByTime
		return assert.EqualValues(o.CreateParam, p)
	case ops.VolumeSeriesRequestListParams:
		return assert.EqualValues(o.ListParam, p)
	}
	return false
}

// String is from gomock.Matcher
func (o *volumeSeriesRequestMatcher) String() string {
	if o.CreateParam != nil {
		return "volumeSeriesRequestMatcher matches on create"
	} else if o.isList {
		return "volumeSeriesRequestMatcher matches on list"
	}
	return "volumeSeriesRequestMatcher unknown context"
}

type snapshotMatcher struct {
	t         *testing.T
	ListParam sops.SnapshotListParams
	isList    bool
}

func newSnapshotMatcher(t *testing.T, param interface{}) *snapshotMatcher {
	m := &snapshotMatcher{t: t}
	switch p := param.(type) {
	case sops.SnapshotListParams:
		m.ListParam = p
		m.isList = true
	default:
		assert.Fail(t, "invalid param type")
	}
	return m
}

// Matches is from gomock.Matcher
func (o *snapshotMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch p := x.(type) {
	case sops.SnapshotListParams:
		return assert.EqualValues(o.ListParam, p)
	}
	return false
}

// String is from gomock.Matcher
func (o *snapshotMatcher) String() string {
	if o.isList {
		return "snapshotMatcher matches on list"
	}
	return "snapshotMatcher unknown context"
}
