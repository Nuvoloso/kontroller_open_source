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
	"strings"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/protection_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/snapshot"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	fal "github.com/Nuvoloso/kontroller/pkg/centrald/audit/fake"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	"github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestProtectionDomainCreate(t *testing.T) {
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

	pdMd := app.GetProtectionDomainMetadata()
	eA := ""
	ePP := ""
	for _, pdm := range pdMd {
		if pdm.EncryptionAlgorithm != common.EncryptionNone {
			eA = pdm.EncryptionAlgorithm
			ePP = strings.Repeat("x", int(pdm.MinPassphraseLength))
			break
		}
	}
	assert.NotEmpty(eA)
	assert.NotEmpty(ePP)

	ai := &auth.Info{}
	params := ops.ProtectionDomainCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
		Payload:     &models.ProtectionDomainCreateArgs{},
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.ProtectionDomain{
		ProtectionDomainAllOf0: models.ProtectionDomainAllOf0{
			Meta: &models.ObjMeta{
				ID:      "PD-1",
				Version: 1,
			},
		},
		ProtectionDomainCreateOnce: models.ProtectionDomainCreateOnce{
			AccountID:            "accountID",
			EncryptionAlgorithm:  eA,
			EncryptionPassphrase: &models.ValueType{Kind: "SECRET", Value: ePP},
		},
		ProtectionDomainMutable: models.ProtectionDomainMutable{
			Name: "pd1",
			Tags: []string{"tag1"},
		},
	}
	aObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{Meta: &models.ObjMeta{ID: "accountID"}},
	}
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.ProtectionDomainManagementCap: true}
	ai.AccountID = string(aObj.Meta.ID)
	assert.Error(ai.InternalOK())

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { tl.Logger().Info("/////////// ALL DONE"); tl.Flush(); mockCtrl.Finish() }()
	var retObj *models.ProtectionDomain
	testutils.Clone(obj, &retObj)
	params.Payload.AccountID = retObj.AccountID
	params.Payload.EncryptionAlgorithm = retObj.EncryptionAlgorithm
	params.Payload.EncryptionPassphrase = retObj.EncryptionPassphrase
	params.Payload.Name = retObj.Name
	params.Payload.Tags = retObj.Tags
	oP := mock.NewMockProtectionDomainOps(mockCtrl)
	oP.EXPECT().Create(ctx, params.Payload).Return(retObj, nil)
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(params.Payload.AccountID)).Return(aObj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsProtectionDomain().Return(oP).MinTimes(1)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	t.Log("case: success")
	fa.Posts = []*fal.Args{}
	hc.DS = mds
	cntRLock := hc.cntRLock
	cntRUnlock := hc.cntRUnlock
	assert.Nil(evM.InSSProps)
	ret := hc.protectionDomainCreate(params)
	assert.NotNil(ret)
	_, ok := ret.(*ops.ProtectionDomainCreateCreated)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Len(evM.InSSProps, 1)
	assert.EqualValues(retObj.Meta.ID, evM.InSSProps["meta.id"])
	assert.Equal(retObj, evM.InACScope)
	expAl := &fal.Args{AI: ai, Action: centrald.ProtectionDomainCreateAction, ObjID: retObj.Meta.ID, Name: retObj.Name, RefID: models.ObjIDMutable(ai.AccountID), Message: "Encryption: " + eA}
	assert.Equal([]*fal.Args{expAl}, fa.Posts)
	tl.Flush()

	// error cases
	tcs := []string{"create-fails", "not-owner-account", "account-not-found", "audit-log-not-ready",
		"encryption-invalid", "system-tags-set", "passphrase-vt-nil", "no-encryption-algorithm",
		"no-payload", "no-right", "ai-error"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		testutils.Clone(obj, &retObj)
		ai = &auth.Info{}
		params = ops.ProtectionDomainCreateParams{
			HTTPRequest: requestWithAuthContext(ai),
			Payload:     &models.ProtectionDomainCreateArgs{},
		}
		params.Payload.AccountID = retObj.AccountID
		params.Payload.EncryptionAlgorithm = retObj.EncryptionAlgorithm
		params.Payload.EncryptionPassphrase = retObj.EncryptionPassphrase
		params.Payload.Name = retObj.Name
		params.Payload.Tags = retObj.Tags
		ctx = params.HTTPRequest.Context()
		ai.RoleObj = &models.Role{}
		ai.RoleObj.Capabilities = map[string]bool{centrald.ProtectionDomainManagementCap: true}
		ai.AccountID = string(aObj.Meta.ID)
		fa.Posts = []*fal.Args{}
		oP = mock.NewMockProtectionDomainOps(mockCtrl)
		oA = mock.NewMockAccountOps(mockCtrl)
		mds := mock.NewMockDataStore(mockCtrl)
		mds.EXPECT().OpsProtectionDomain().Return(oP).AnyTimes()
		mds.EXPECT().OpsAccount().Return(oA).AnyTimes()
		hc.DS = mds
		errCode := centrald.ErrorMissing.C
		errMsg := centrald.ErrorMissing.M
		cntRLock = hc.cntRLock
		cntRUnlock = hc.cntRUnlock
		lockObtained := false
		t.Log("case:", tc)
		switch tc {
		case "create-fails":
			oA.EXPECT().Fetch(ctx, string(params.Payload.AccountID)).Return(aObj, nil)
			oP.EXPECT().Create(ctx, params.Payload).Return(nil, centrald.ErrorExists)
			errCode = centrald.ErrorExists.C
			errMsg = centrald.ErrorExists.M
			lockObtained = true
		case "not-owner-account":
			params.Payload.AccountID = models.ObjIDMutable(ai.AccountID + "foo")
			oA.EXPECT().Fetch(ctx, string(params.Payload.AccountID)).Return(aObj, nil)
			errCode = centrald.ErrorUnauthorizedOrForbidden.C
			errMsg = centrald.ErrorUnauthorizedOrForbidden.M
			lockObtained = true
		case "account-not-found":
			oA.EXPECT().Fetch(ctx, string(params.Payload.AccountID)).Return(nil, centrald.ErrorNotFound)
			errMsg = errMsg + ".*invalid accountId"
			lockObtained = true
		case "audit-log-not-ready":
			fa.ReadyRet = centrald.ErrorDbError
			errCode = centrald.ErrorDbError.C
			errMsg = centrald.ErrorDbError.M
			lockObtained = true
		case "encryption-invalid":
			params.Payload.EncryptionAlgorithm = eA + "foo"
		case "system-tags-set":
			params.Payload.SystemTags = []string{"stag"}
			errCode = centrald.ErrorUnauthorizedOrForbidden.C
			errMsg = centrald.ErrorUnauthorizedOrForbidden.M + ".*systemTags"
		case "passphrase-vt-nil":
			params.Payload.EncryptionPassphrase = nil
		case "no-encryption-algorithm":
			params.Payload.EncryptionAlgorithm = ""
		case "no-payload":
			params.Payload = nil
		case "no-right":
			ai.RoleObj.Capabilities = map[string]bool{centrald.AccountFetchOwnRoleCap: true}
			errCode = centrald.ErrorUnauthorizedOrForbidden.C
			errMsg = centrald.ErrorUnauthorizedOrForbidden.M
		case "ai-error":
			params.HTTPRequest = &http.Request{}
			errCode = centrald.ErrorInternalError.C
			errMsg = centrald.ErrorInternalError.M
		default:
			assert.Equal("", tc)
			continue
		}
		ret = hc.protectionDomainCreate(params)
		assert.NotNil(ret)
		_, ok = ret.(*ops.ProtectionDomainCreateCreated)
		assert.False(ok)
		mE, ok := ret.(*ops.ProtectionDomainCreateDefault)
		assert.True(ok)
		assert.EqualValues(errCode, mE.Payload.Code)
		assert.Regexp(errMsg, *mE.Payload.Message)
		if lockObtained {
			cntRLock++
			cntRUnlock++
		}
		assert.Equal(cntRLock, hc.cntRLock)
		assert.Equal(cntRUnlock, hc.cntRUnlock)
		tl.Flush()
	}
}

func TestProtectionDomainDelete(t *testing.T) {
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
	fop := &fakeOps{}
	hc.ops = fop
	ai := &auth.Info{}
	params := ops.ProtectionDomainDeleteParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.ProtectionDomain{
		ProtectionDomainAllOf0: models.ProtectionDomainAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("objectID"),
			},
		},
	}
	obj.AccountID = "accountID"
	aObj := &models.Account{}
	aObj.Meta = &models.ObjMeta{ID: "accountID"}

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()

	// success cases
	tcs := []string{"internal", "authorized"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		ai.RoleObj = nil
		fop.RetAccountFetchObj = aObj
		fop.RetAccountRefsPD = false
		oP := mock.NewMockProtectionDomainOps(mockCtrl)
		oP.EXPECT().Fetch(ctx, string(obj.Meta.ID)).Return(obj, nil)
		oP.EXPECT().Delete(ctx, params.ID).Return(nil)
		lpSnap := snapshot.NewSnapshotListParams()
		lpSnap.ProtectionDomainID = swag.String(string(obj.Meta.ID))
		lpSnap.AccountID = swag.String(string(obj.AccountID))
		lpM := newSnapshotMatcher(t, lpSnap)
		oSnap := mock.NewMockSnapshotOps(mockCtrl)
		oSnap.EXPECT().Count(ctx, lpM, uint(1)).Return(0, nil)
		lpVSR := volume_series_request.NewVolumeSeriesRequestListParams()
		lpVSR.ProtectionDomainID = swag.String(string(obj.Meta.ID))
		lpVSR.IsTerminated = swag.Bool(false)
		vsrM := newVolumeSeriesRequestMatcher(t, lpVSR)
		oVSR := mock.NewMockVolumeSeriesRequestOps(mockCtrl)
		oVSR.EXPECT().Count(ctx, vsrM, uint(1)).Return(0, nil)
		mds := mock.NewMockDataStore(mockCtrl)
		mds.EXPECT().OpsProtectionDomain().Return(oP).MinTimes(2)
		mds.EXPECT().OpsSnapshot().Return(oSnap)
		mds.EXPECT().OpsVolumeSeriesRequest().Return(oVSR)
		switch tc {
		case "internal":
			assert.True(ai.Internal())
		case "authorized":
			ai.AccountID = string(obj.AccountID)
			ai.RoleObj = &models.Role{}
			ai.RoleObj.Capabilities = map[string]bool{centrald.ProtectionDomainManagementCap: true}
			assert.False(ai.Internal())
		default:
			assert.Equal("", tc)
		}
		hc.DS = mds
		cntLock := hc.cntLock
		cntUnlock := hc.cntUnlock
		evM.InSSProps = nil
		fa.Posts = []*fal.Args{}
		ret := hc.protectionDomainDelete(params)
		assert.NotNil(ret)
		_, ok := ret.(*ops.ProtectionDomainDeleteNoContent)
		assert.True(ok)
		assert.Equal(cntLock+1, hc.cntLock)
		assert.Equal(cntUnlock+1, hc.cntUnlock)
		assert.Len(evM.InSSProps, 0)
		assert.Equal(obj, evM.InACScope)
		expAl := &fal.Args{AI: ai, Action: centrald.ProtectionDomainDeleteAction, ObjID: obj.Meta.ID, Name: obj.Name, RefID: obj.AccountID, Message: "Deleted"}
		assert.Equal([]*fal.Args{expAl}, fa.Posts)
		tl.Flush()
	}

	// error cases
	tcs = []string{"delete-fails", "snapshot-references", "snapshots-list-fails", "vsr-references",
		"vsr-list-fails", "account-references", "account-fetch-error", "fetch-not-owner-account", "fetch-no-right",
		"audit-log-not-ready", "ai-error"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		fop.RetAccountFetchObj = aObj
		fop.RetAccountRefsPD = false
		fa.Posts = []*fal.Args{}
		var expAl *fal.Args
		ai = &auth.Info{}
		params = ops.ProtectionDomainDeleteParams{
			HTTPRequest: requestWithAuthContext(ai),
			ID:          "objectID",
		}
		ctx = params.HTTPRequest.Context()
		ai.AccountID = string(obj.AccountID)
		ai.RoleObj = &models.Role{}
		ai.RoleObj.Capabilities = map[string]bool{centrald.ProtectionDomainManagementCap: true}
		oP := mock.NewMockProtectionDomainOps(mockCtrl)
		oSnap := mock.NewMockSnapshotOps(mockCtrl)
		oVSR := mock.NewMockVolumeSeriesRequestOps(mockCtrl)
		mds := mock.NewMockDataStore(mockCtrl)
		mds.EXPECT().OpsProtectionDomain().Return(oP).AnyTimes()
		mds.EXPECT().OpsSnapshot().Return(oSnap).AnyTimes()
		mds.EXPECT().OpsVolumeSeriesRequest().Return(oVSR).AnyTimes()
		hc.DS = mds
		errCode := centrald.ErrorExists.C
		errPat := centrald.ErrorExists.M
		cntLock := hc.cntLock
		cntUnlock := hc.cntUnlock
		lockObtained := true
		t.Log("case:", tc)
		switch tc {
		case "delete-fails":
			oP.EXPECT().Fetch(ctx, string(obj.Meta.ID)).Return(obj, nil)
			oP.EXPECT().Delete(ctx, params.ID).Return(centrald.ErrorDbError)
			oSnap.EXPECT().Count(ctx, gomock.Any(), uint(1)).Return(0, nil)
			oVSR.EXPECT().Count(ctx, gomock.Any(), uint(1)).Return(0, nil)
			errCode = centrald.ErrorDbError.C
			errPat = centrald.ErrorDbError.M
		case "snapshot-references":
			oP.EXPECT().Fetch(ctx, string(obj.Meta.ID)).Return(obj, nil)
			oSnap.EXPECT().Count(ctx, gomock.Any(), uint(1)).Return(1, nil)
			oVSR.EXPECT().Count(ctx, gomock.Any(), uint(1)).Return(0, nil)
			errPat += ".*Snapshot objects"
		case "snapshots-list-fails":
			oP.EXPECT().Fetch(ctx, string(obj.Meta.ID)).Return(obj, nil)
			oSnap.EXPECT().Count(ctx, gomock.Any(), uint(1)).Return(0, centrald.ErrorDbError)
			oVSR.EXPECT().Count(ctx, gomock.Any(), uint(1)).Return(0, nil)
			errCode = centrald.ErrorDbError.C
			errPat = centrald.ErrorDbError.M
		case "vsr-references":
			oP.EXPECT().Fetch(ctx, string(obj.Meta.ID)).Return(obj, nil)
			oVSR.EXPECT().Count(ctx, gomock.Any(), uint(1)).Return(1, nil)
			errPat += ".*VolumeSeriesRequest objects"
		case "vsr-list-fails":
			oP.EXPECT().Fetch(ctx, string(obj.Meta.ID)).Return(obj, nil)
			oVSR.EXPECT().Count(ctx, gomock.Any(), uint(1)).Return(0, centrald.ErrorDbError)
			errCode = centrald.ErrorDbError.C
			errPat = centrald.ErrorDbError.M
		case "account-references":
			oP.EXPECT().Fetch(ctx, string(obj.Meta.ID)).Return(obj, nil)
			fop.RetAccountRefsPD = true
			errPat += ".*Account object"
		case "account-fetch-error":
			oP.EXPECT().Fetch(ctx, string(obj.Meta.ID)).Return(obj, nil)
			fop.RetAccountFetchObj = nil
			fop.RetAccountFetchErr = centrald.ErrorDbError
			errCode = centrald.ErrorDbError.C
			errPat = centrald.ErrorDbError.M
		case "fetch-not-owner-account":
			oP.EXPECT().Fetch(ctx, string(obj.Meta.ID)).Return(obj, nil)
			ai.AccountID = string(obj.AccountID + "foo")
			errCode = centrald.ErrorUnauthorizedOrForbidden.C
			errPat = centrald.ErrorUnauthorizedOrForbidden.M
			lockObtained = false
			expAl = &fal.Args{AI: ai, Action: centrald.ProtectionDomainDeleteAction, ObjID: obj.Meta.ID, Name: obj.Name, RefID: obj.AccountID, Err: true, Message: "Delete unauthorized"}
		case "fetch-no-right":
			ai.RoleObj.Capabilities = map[string]bool{centrald.AccountFetchOwnRoleCap: true}
			errCode = centrald.ErrorUnauthorizedOrForbidden.C
			errPat = centrald.ErrorUnauthorizedOrForbidden.M
			lockObtained = false
			expAl = &fal.Args{AI: ai, Action: centrald.ProtectionDomainDeleteAction, ObjID: obj.Meta.ID, Err: true, Message: "Delete unauthorized"}
		case "audit-log-not-ready":
			fa.ReadyRet = centrald.ErrorDbError
			errCode = centrald.ErrorDbError.C
			errPat = centrald.ErrorDbError.M
			lockObtained = false
		case "ai-error":
			params.HTTPRequest = &http.Request{}
			errCode = centrald.ErrorInternalError.C
			errPat = centrald.ErrorInternalError.M
			lockObtained = false
		default:
			assert.Equal("", tc)
			continue
		}
		ret := hc.protectionDomainDelete(params)
		assert.NotNil(ret)
		_, ok := ret.(*ops.ProtectionDomainDeleteNoContent)
		assert.False(ok)
		mE, ok := ret.(*ops.ProtectionDomainDeleteDefault)
		assert.True(ok)
		assert.EqualValues(errCode, mE.Payload.Code)
		assert.Regexp(errPat, *mE.Payload.Message)
		if lockObtained {
			assert.Equal(cntLock+1, hc.cntLock)
			assert.Equal(cntUnlock+1, hc.cntUnlock)
		}
		if expAl != nil {
			assert.Equal([]*fal.Args{expAl}, fa.Posts)
		}
		tl.Flush()
	}
}

func TestProtectionDomainFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.ProtectionDomainFetchParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.ProtectionDomain{
		ProtectionDomainAllOf0: models.ProtectionDomainAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("objectID"),
			},
		},
	}
	obj.AccountID = "accountID"

	// success (internal)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oP := mock.NewMockProtectionDomainOps(mockCtrl)
	oP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsProtectionDomain().Return(oP).MinTimes(1)
	t.Log("case: success (internal)")
	assert.True(ai.Internal())
	hc.DS = mds
	var ret middleware.Responder
	ret = hc.protectionDomainFetch(params)
	assert.NotNil(ret)
	mR, ok := ret.(*ops.ProtectionDomainFetchOK)
	assert.True(ok)
	assert.Equal(obj, mR.Payload)
	tl.Flush()

	// success (authorized)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: success (authorized)")
	ai.AccountID = string(obj.AccountID)
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.ProtectionDomainManagementCap: true}
	oP = mock.NewMockProtectionDomainOps(mockCtrl)
	oP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsProtectionDomain().Return(oP).MinTimes(1)
	hc.DS = mds
	ret = hc.protectionDomainFetch(params)
	assert.NotNil(ret)
	mR, ok = ret.(*ops.ProtectionDomainFetchOK)
	assert.True(ok)
	assert.Equal(obj, mR.Payload)
	tl.Flush()

	// error cases
	tcs := []string{"fetch-fails", "not-owner-account", "no-right", "ai-error"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		ai.AccountID = string(obj.AccountID)
		ai.RoleObj = &models.Role{}
		ai.RoleObj.Capabilities = map[string]bool{centrald.ProtectionDomainManagementCap: true}
		oP = mock.NewMockProtectionDomainOps(mockCtrl)
		mds = mock.NewMockDataStore(mockCtrl)
		mds.EXPECT().OpsProtectionDomain().Return(oP).AnyTimes()
		hc.DS = mds
		errCode := centrald.ErrorNotFound.C
		errMsg := centrald.ErrorNotFound.M
		t.Log("case:", tc)
		switch tc {
		case "fetch-fails":
			oP.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
		case "not-owner-account":
			oP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			ai.AccountID = string(obj.AccountID + "foo")
			errCode = centrald.ErrorUnauthorizedOrForbidden.C
			errMsg = centrald.ErrorUnauthorizedOrForbidden.M
		case "no-right":
			ai.RoleObj.Capabilities = map[string]bool{centrald.AccountFetchOwnRoleCap: true}
			errCode = centrald.ErrorUnauthorizedOrForbidden.C
			errMsg = centrald.ErrorUnauthorizedOrForbidden.M
		case "ai-error":
			params.HTTPRequest = &http.Request{}
			errCode = centrald.ErrorInternalError.C
			errMsg = centrald.ErrorInternalError.M
		default:
			assert.Equal("", tc)
			continue
		}
		ret = hc.protectionDomainFetch(params)
		assert.NotNil(ret)
		_, ok = ret.(*ops.ProtectionDomainFetchOK)
		assert.False(ok)
		mE, ok := ret.(*ops.ProtectionDomainFetchDefault)
		assert.True(ok)
		assert.EqualValues(errCode, mE.Payload.Code)
		assert.Regexp(errMsg, *mE.Payload.Message)
		tl.Flush()
	}
}

func TestProtectionDomainList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	objects := []*models.ProtectionDomain{
		&models.ProtectionDomain{
			ProtectionDomainAllOf0: models.ProtectionDomainAllOf0{
				Meta: &models.ObjMeta{
					ID: models.ObjID("objectID"),
				},
			},
		},
		&models.ProtectionDomain{
			ProtectionDomainAllOf0: models.ProtectionDomainAllOf0{
				Meta: &models.ObjMeta{
					ID: models.ObjID("objectID"),
				},
			},
		},
	}
	objects[0].AccountID = "accountID1"
	objects[1].AccountID = "accountID2"

	// success (internal)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ai := &auth.Info{}
	params := ops.ProtectionDomainListParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	ctx := params.HTTPRequest.Context()
	oP := mock.NewMockProtectionDomainOps(mockCtrl)
	oP.EXPECT().List(ctx, params).Return(objects, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsProtectionDomain().Return(oP).MinTimes(1)
	t.Log("case: success (internal)")
	assert.True(ai.Internal())
	hc.DS = mds
	var ret middleware.Responder
	ret = hc.protectionDomainList(params)
	assert.NotNil(ret)
	assert.Nil(params.AccountID)
	mR, ok := ret.(*ops.ProtectionDomainListOK)
	assert.True(ok)
	assert.EqualValues(objects, mR.Payload)
	tl.Flush()

	// success (authorized), query scope reduced
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: success (authorized), query scope reduced")
	ai.AccountID = string(objects[0].AccountID)
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.ProtectionDomainManagementCap: true}
	assert.False(ai.Internal())
	assert.Nil(params.AccountID)
	expParams := params // shallow copy
	expParams.AccountID = swag.String(ai.AccountID)
	oP = mock.NewMockProtectionDomainOps(mockCtrl)
	oP.EXPECT().List(ctx, expParams).Return(objects, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsProtectionDomain().Return(oP).MinTimes(1)
	hc.DS = mds
	ret = hc.protectionDomainList(params)
	assert.NotNil(ret)
	mR, ok = ret.(*ops.ProtectionDomainListOK)
	assert.True(ok)
	expObjects := []*models.ProtectionDomain{objects[0]}
	assert.EqualValues(expObjects, mR.Payload)
	tl.Flush()

	// success (authorized), all parameters
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: success (authorized), all parameters")
	ai.AccountID = string(objects[1].AccountID)
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.ProtectionDomainManagementCap: true}
	assert.False(ai.Internal())
	params.AccountID = swag.String("something")
	params.Name = swag.String("name")
	params.NamePattern = swag.String("name.*")
	params.SystemTags = []string{"stag"}
	params.Tags = []string{"tag"}
	oP = mock.NewMockProtectionDomainOps(mockCtrl)
	oP.EXPECT().List(ctx, params).Return(objects, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsProtectionDomain().Return(oP).MinTimes(1)
	hc.DS = mds
	ret = hc.protectionDomainList(params)
	assert.NotNil(ret)
	mR, ok = ret.(*ops.ProtectionDomainListOK)
	assert.True(ok)
	expObjects = []*models.ProtectionDomain{objects[1]}
	assert.EqualValues(expObjects, mR.Payload)
	tl.Flush()

	// error cases
	tcs := []string{"list-fails", "no-right", "ai-error"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		ai.AccountID = string(objects[0].AccountID)
		ai.RoleObj = &models.Role{}
		ai.RoleObj.Capabilities = map[string]bool{centrald.ProtectionDomainManagementCap: true}
		oP = mock.NewMockProtectionDomainOps(mockCtrl)
		mds = mock.NewMockDataStore(mockCtrl)
		mds.EXPECT().OpsProtectionDomain().Return(oP).AnyTimes()
		hc.DS = mds
		errCode := centrald.ErrorDbError.C
		errMsg := centrald.ErrorDbError.M
		t.Log("case:", tc)
		switch tc {
		case "list-fails":
			params.AccountID = swag.String("foo")
			oP.EXPECT().List(ctx, params).Return(nil, centrald.ErrorDbError)
		case "no-right":
			ai.RoleObj.Capabilities = map[string]bool{centrald.AccountFetchOwnRoleCap: true}
			errCode = centrald.ErrorUnauthorizedOrForbidden.C
			errMsg = centrald.ErrorUnauthorizedOrForbidden.M
		case "ai-error":
			params.HTTPRequest = &http.Request{}
			errCode = centrald.ErrorInternalError.C
			errMsg = centrald.ErrorInternalError.M
		default:
			assert.Equal("", tc)
			continue
		}
		ret = hc.protectionDomainList(params)
		assert.NotNil(ret)
		mR, ok = ret.(*ops.ProtectionDomainListOK)
		assert.False(ok)
		mE, ok := ret.(*ops.ProtectionDomainListDefault)
		assert.True(ok)
		assert.EqualValues(errCode, mE.Payload.Code)
		assert.Regexp(errMsg, *mE.Payload.Message)
		tl.Flush()
	}
}

func TestProtectionDomainMetadataHandler(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app

	params := ops.NewProtectionDomainMetadataParams()
	rc := hc.protectionDomainMetadata(params)
	assert.NotNil(rc)
	ret, ok := rc.(*ops.ProtectionDomainMetadataOK)
	assert.True(ok)
	pdm := app.GetProtectionDomainMetadata()
	assert.EqualValues(pdm, ret.Payload)
}

func TestProtectionDomainUpdate(t *testing.T) {
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

	nMap := hc.protectionDomainMutableNameMap()

	makeUA := func(params ops.ProtectionDomainUpdateParams) (*centrald.UpdateArgs, *mock.UpdateArgsMatcher) {
		var uP = [centrald.NumActionTypes][]string{
			centrald.UpdateRemove: params.Remove,
			centrald.UpdateAppend: params.Append,
			centrald.UpdateSet:    params.Set,
		}
		ua, err := hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP)
		assert.Nil(err)
		assert.NotNil(ua)
		return ua, updateArgsMatcher(t, ua)
	}

	oObj := &models.ProtectionDomain{
		ProtectionDomainAllOf0: models.ProtectionDomainAllOf0{
			Meta: &models.ObjMeta{
				ID:      models.ObjID("objectID"),
				Version: 9,
			},
		},
	}
	oObj.Name = "oldName"
	oObj.AccountID = "accountID"
	obj := &models.ProtectionDomain{
		ProtectionDomainAllOf0: models.ProtectionDomainAllOf0{
			Meta: &models.ObjMeta{
				ID:      models.ObjID("objectID"),
				Version: 10,
			},
		},
	}
	obj.Name = "newName"
	obj.AccountID = "accountID"

	// success (internal)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	t.Log("case: success (internal)")
	ai := &auth.Info{}
	params := ops.ProtectionDomainUpdateParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
		Version:     swag.Int32(int32(oObj.Meta.Version)),
		Append:      []string{"systemTags"},
		Payload:     &oObj.ProtectionDomainMutable,
	}
	ctx := params.HTTPRequest.Context()
	ua, uaM := makeUA(params)
	assert.EqualValues(oObj.Meta.Version, ua.Version)
	oP := mock.NewMockProtectionDomainOps(mockCtrl)
	oP.EXPECT().Fetch(ctx, params.ID).Return(oObj, nil)
	oP.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsProtectionDomain().Return(oP).MinTimes(2)
	hc.DS = mds
	fa.Posts = []*fal.Args{}
	cntRLock := hc.cntRLock
	cntRUnlock := hc.cntRUnlock
	assert.Nil(evM.InSSProps)
	ret := hc.protectionDomainUpdate(params)
	assert.NotNil(ret)
	mR, ok := ret.(*ops.ProtectionDomainUpdateOK)
	assert.True(ok)
	assert.Equal(obj, mR.Payload)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Len(evM.InSSProps, 0)
	assert.Equal(obj, evM.InACScope)
	assert.Empty(fa.Posts)
	tl.Flush()

	// success (authorized)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: success (authorized)")
	ai.AccountID = string(oObj.AccountID)
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.ProtectionDomainManagementCap: true}
	params = ops.ProtectionDomainUpdateParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
		Remove:      []string{"tags"},
		Set:         []string{"name", "description"},
		Payload:     &oObj.ProtectionDomainMutable,
	}
	ctx = params.HTTPRequest.Context()
	ua, uaM = makeUA(params)
	assert.EqualValues(0, ua.Version)
	uaM.AddsVersion(int32(oObj.Meta.Version))
	oP = mock.NewMockProtectionDomainOps(mockCtrl)
	oP.EXPECT().Fetch(ctx, params.ID).Return(oObj, nil)
	oP.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsProtectionDomain().Return(oP).MinTimes(2)
	hc.DS = mds
	fa.Posts = []*fal.Args{}
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	evM.InSSProps = nil
	ret = hc.protectionDomainUpdate(params)
	assert.NotNil(ret)
	mR, ok = ret.(*ops.ProtectionDomainUpdateOK)
	assert.True(ok)
	assert.Equal(obj, mR.Payload)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Len(evM.InSSProps, 0)
	assert.Equal(obj, evM.InACScope)
	expAl := &fal.Args{AI: ai, Action: centrald.ProtectionDomainUpdateAction, ObjID: obj.Meta.ID, Name: obj.Name, RefID: models.ObjIDMutable(obj.AccountID), Message: "Renamed from 'oldName'"}
	assert.Equal([]*fal.Args{expAl}, fa.Posts)
	tl.Flush()

	// error cases
	tcs := []string{"update-fails", "version-mismatch", "fetch-fails", "not-owner-account", "no-right",
		"audit-log-not-ready", "no-payload", "system-tags-modified", "invalid-argument", "no-change", "ai-error"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		ai.AccountID = string(obj.AccountID)
		ai.RoleObj = &models.Role{}
		ai.RoleObj.Capabilities = map[string]bool{centrald.ProtectionDomainManagementCap: true}
		params = ops.ProtectionDomainUpdateParams{
			HTTPRequest: requestWithAuthContext(ai),
			ID:          "objectID",
			Set:         []string{"description"}, // trivial update case
			Payload:     &oObj.ProtectionDomainMutable,
		}
		ctx = params.HTTPRequest.Context()
		oP = mock.NewMockProtectionDomainOps(mockCtrl)
		mds = mock.NewMockDataStore(mockCtrl)
		mds.EXPECT().OpsProtectionDomain().Return(oP).AnyTimes()
		hc.DS = mds
		errCode := centrald.ErrorUpdateInvalidRequest.C
		errMsg := centrald.ErrorUpdateInvalidRequest.M
		cntRLock = hc.cntRLock
		cntRUnlock = hc.cntRUnlock
		lockObtained := false
		t.Log("case:", tc)
		switch tc {
		case "update-fails":
			oP.EXPECT().Fetch(ctx, params.ID).Return(oObj, nil)
			_, uaM = makeUA(params)
			uaM.AddsVersion(int32(oObj.Meta.Version))
			oP.EXPECT().Update(ctx, uaM, params.Payload).Return(nil, centrald.ErrorDbError)
			errCode = centrald.ErrorDbError.C
			errMsg = centrald.ErrorDbError.M
			lockObtained = true
		case "version-mismatch":
			params.Version = swag.Int32(int32(oObj.Meta.Version) - 2)
			oP.EXPECT().Fetch(ctx, params.ID).Return(oObj, nil)
			errCode = centrald.ErrorIDVerNotFound.C
			errMsg = centrald.ErrorIDVerNotFound.M
			lockObtained = true
		case "fetch-fails":
			oP.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
			errCode = centrald.ErrorNotFound.C
			errMsg = centrald.ErrorNotFound.M
			lockObtained = true
		case "not-owner-account":
			oP.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
			ai.AccountID = string(obj.AccountID + "foo")
			errCode = centrald.ErrorUnauthorizedOrForbidden.C
			errMsg = centrald.ErrorUnauthorizedOrForbidden.M
			lockObtained = true
		case "no-right":
			ai.RoleObj.Capabilities = map[string]bool{centrald.AccountFetchOwnRoleCap: true}
			errCode = centrald.ErrorUnauthorizedOrForbidden.C
			errMsg = centrald.ErrorUnauthorizedOrForbidden.M
			lockObtained = true
		case "audit-log-not-ready":
			params.Set = []string{"name"} // audited update
			fa.ReadyRet = centrald.ErrorDbError
			errCode = centrald.ErrorDbError.C
			errMsg = centrald.ErrorDbError.M
			lockObtained = true
		case "no-payload":
			params.Payload = nil
		case "system-tags-modified":
			params.Append = []string{"systemTags"}
			errCode = centrald.ErrorUnauthorizedOrForbidden.C
			errMsg = centrald.ErrorUnauthorizedOrForbidden.M
		case "invalid-argument":
			params.Set = []string{"encryptionAlgorithm"}
		case "no-change":
			params.Set = []string{}
		case "ai-error":
			params.HTTPRequest = &http.Request{}
			errCode = centrald.ErrorInternalError.C
			errMsg = centrald.ErrorInternalError.M
		default:
			assert.Equal("", tc)
			continue
		}
		ret = hc.protectionDomainUpdate(params)
		assert.NotNil(ret)
		_, ok = ret.(*ops.ProtectionDomainUpdateOK)
		assert.False(ok)
		mE, ok := ret.(*ops.ProtectionDomainUpdateDefault)
		assert.True(ok)
		assert.EqualValues(errCode, mE.Payload.Code)
		assert.Regexp(errMsg, *mE.Payload.Message)
		if lockObtained {
			cntRLock++
			cntRUnlock++
		}
		assert.Equal(cntRLock, hc.cntRLock)
		assert.Equal(cntRUnlock, hc.cntRUnlock)
		tl.Flush()
	}
}
