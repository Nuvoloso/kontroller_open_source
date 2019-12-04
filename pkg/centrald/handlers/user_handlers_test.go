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
	"errors"
	"net/http"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/role"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/user"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	fal "github.com/Nuvoloso/kontroller/pkg/centrald/audit/fake"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"
)

func TestDeriveAccountRoles(t *testing.T) {
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

	uList := []*models.User{
		&models.User{
			UserAllOf0: models.UserAllOf0{
				Meta: &models.ObjMeta{ID: "uid1"},
			},
		},
		&models.User{
			UserAllOf0: models.UserAllOf0{
				Meta: &models.ObjMeta{ID: "uid2"},
			},
		},
		&models.User{
			UserAllOf0: models.UserAllOf0{
				Meta: &models.ObjMeta{ID: "uid3"},
			},
			UserMutable: models.UserMutable{
				Disabled: true,
			},
		},
	}
	aRes := []*models.Account{
		&models.Account{
			AccountAllOf0: models.AccountAllOf0{
				Meta:            &models.ObjMeta{ID: "aid1"},
				TenantAccountID: "tid1",
			},
			AccountMutable: models.AccountMutable{
				Name: "housekeeping",
				UserRoles: map[string]models.AuthRole{
					"uid1": models.AuthRole{RoleID: "rid1"},
				},
			},
		},
		&models.Account{
			AccountAllOf0: models.AccountAllOf0{
				Meta:            &models.ObjMeta{ID: "aid2"},
				TenantAccountID: "tid1",
			},
			AccountMutable: models.AccountMutable{
				Disabled: true,
				Name:     "finance",
				UserRoles: map[string]models.AuthRole{
					"uid1": models.AuthRole{RoleID: "rid1"},
				},
			},
		},
		&models.Account{
			AccountAllOf0: models.AccountAllOf0{
				Meta:            &models.ObjMeta{ID: "tid1"},
				TenantAccountID: "",
			},
			AccountMutable: models.AccountMutable{
				Name: "nuvoloso",
				UserRoles: map[string]models.AuthRole{
					"uid3": models.AuthRole{RoleID: "rid2"},
				},
			},
		},
		&models.Account{
			AccountAllOf0: models.AccountAllOf0{
				Meta:            &models.ObjMeta{ID: "tid2"},
				TenantAccountID: "",
			},
			AccountMutable: models.AccountMutable{
				Name: "generic",
				UserRoles: map[string]models.AuthRole{
					"uid1": models.AuthRole{RoleID: "rid2"},
				},
			},
		},
		&models.Account{
			AccountAllOf0: models.AccountAllOf0{
				Meta:            &models.ObjMeta{ID: "tid2"},
				TenantAccountID: "",
			},
			AccountMutable: models.AccountMutable{
				Name: "last",
				UserRoles: map[string]models.AuthRole{
					"uid1": models.AuthRole{Disabled: true, RoleID: "rid2"},
				},
			},
		},
	}
	rRes := []*models.Role{
		&models.Role{
			RoleAllOf0: models.RoleAllOf0{
				Meta: &models.ObjMeta{ID: "rid0"},
			},
		},
		&models.Role{
			RoleAllOf0: models.RoleAllOf0{
				Meta: &models.ObjMeta{ID: "rid1"},
			},
			RoleMutable: models.RoleMutable{
				Name: centrald.AccountAdminRole,
			},
		},
		&models.Role{
			RoleAllOf0: models.RoleAllOf0{
				Meta: &models.ObjMeta{ID: "rid2"},
			},
			RoleMutable: models.RoleMutable{
				Name: centrald.TenantAdminRole,
			},
		},
	}

	// success, data driven coverage of various branches
	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mds := mock.NewMockDataStore(mockCtrl)
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().List(ctx, account.AccountListParams{}).Return(aRes, nil)
	mds.EXPECT().OpsAccount().Return(oA)
	oR := mock.NewMockRoleOps(mockCtrl)
	oR.EXPECT().List(role.RoleListParams{}).Return(rRes, nil)
	mds.EXPECT().OpsRole().Return(oR)
	hc.DS = mds
	assert.NoError(hc.deriveAccountRoles(ctx, uList))
	for _, u := range uList {
		assert.NotNil(u.AccountRoles)
		if u.Meta.ID != "uid1" {
			assert.Empty(u.AccountRoles)
		} else {
			assert.Len(u.AccountRoles, 2)
			assert.Equal(&models.UserAccountRole{
				AccountID:         "aid1",
				RoleID:            "rid1",
				AccountName:       "housekeeping",
				TenantAccountName: "nuvoloso",
				RoleName:          centrald.AccountAdminRole,
			}, u.AccountRoles[0])
			assert.Equal(&models.UserAccountRole{
				AccountID:   "tid2",
				RoleID:      "rid2",
				AccountName: "generic",
				RoleName:    centrald.TenantAdminRole,
			}, u.AccountRoles[1])
		}
		u.AccountRoles = nil
	}

	mockCtrl.Finish()
	t.Log("case: account list fails")
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().List(ctx, account.AccountListParams{}).Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	err := hc.deriveAccountRoles(ctx, uList)
	assert.Equal(centrald.ErrorDbError, err)

	mockCtrl.Finish()
	t.Log("case: role list fails")
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().List(ctx, account.AccountListParams{}).Return(aRes, nil)
	mds.EXPECT().OpsAccount().Return(oA)
	oR = mock.NewMockRoleOps(mockCtrl)
	oR.EXPECT().List(role.RoleListParams{}).Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsRole().Return(oR)
	hc.DS = mds
	err = hc.deriveAccountRoles(ctx, uList)
	assert.Equal(centrald.ErrorDbError, err)
}

func TestUserFetchFilter(t *testing.T) {
	assert := assert.New(t)
	hc := newHandlerComp()

	ai := &auth.Info{}
	uObj := &models.User{UserAllOf0: models.UserAllOf0{Meta: &models.ObjMeta{ID: "uid1"}}}
	assert.NoError(hc.userFetchFilter(ai, uObj))

	ai.RoleObj = &models.Role{}
	ai.UserID = "uid2"
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.userFetchFilter(ai, uObj))

	ai.UserID = "uid1"
	assert.NoError(hc.userFetchFilter(ai, uObj))

	ai.RoleObj.Capabilities = map[string]bool{centrald.UserManagementCap: true}
	ai.UserID = "userWithAdminRole"
	assert.NoError(hc.userFetchFilter(ai, uObj))
}

func TestUserValidateCryptPassword(t *testing.T) {
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

	pw := "12345"
	params := &models.UserMutable{
		AuthIdentifier: "user1@nuvoloso.com",
		Password:       pw,
	}
	sysObj := &models.System{
		SystemMutable: models.SystemMutable{
			UserPasswordPolicy: &models.SystemMutableUserPasswordPolicy{
				MinLength: 3,
			},
		},
	}
	defer func() { cryptPWHook = bcrypt.GenerateFromPassword }()

	t.Log("case: success")
	cryptPWHook = func(password []byte, cost int) ([]byte, error) {
		return []byte("crypt:" + string(password)), nil
	}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mds := mock.NewMockDataStore(mockCtrl)
	oS := mock.NewMockSystemOps(mockCtrl)
	oS.EXPECT().Fetch().Return(sysObj, nil)
	mds.EXPECT().OpsSystem().Return(oS)
	hc.DS = mds
	assert.NoError(hc.userValidateCryptPassword(params, false))
	assert.Equal("crypt:"+pw, params.Password)

	t.Log("case: success, trimmed")
	params.Password = "  " + pw + " \t"
	oS.EXPECT().Fetch().Return(sysObj, nil)
	mds.EXPECT().OpsSystem().Return(oS)
	hc.DS = mds
	assert.NoError(hc.userValidateCryptPassword(params, false))
	assert.Equal("crypt:"+pw, params.Password)

	t.Log("case: System fetch fails")
	oS.EXPECT().Fetch().Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsSystem().Return(oS)
	assert.Equal(centrald.ErrorDbError, hc.userValidateCryptPassword(params, false))

	t.Log("case: invalid password, set")
	params.Password = "12"
	oS.EXPECT().Fetch().Return(sysObj, nil)
	mds.EXPECT().OpsSystem().Return(oS)
	err, ok := hc.userValidateCryptPassword(params, false).(*centrald.Error)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorMissing.C, err.C)
		assert.Regexp("^"+centrald.ErrorMissing.M, err.M)
	}

	t.Log("case: invalid password, update")
	params.Password = "12"
	oS.EXPECT().Fetch().Return(sysObj, nil)
	mds.EXPECT().OpsSystem().Return(oS)
	err, ok = hc.userValidateCryptPassword(params, true).(*centrald.Error)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorUpdateInvalidRequest.C, err.C)
		assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, err.M)
	}
	tl.Flush()

	t.Log("case: crypt failure")
	params.Password = pw
	cryptPWHook = func(password []byte, cost int) ([]byte, error) {
		return nil, errors.New("crypt failure")
	}
	oS.EXPECT().Fetch().Return(sysObj, nil)
	mds.EXPECT().OpsSystem().Return(oS)
	err, ok = hc.userValidateCryptPassword(params, true).(*centrald.Error)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorInternalError.C, err.C)
		assert.Regexp("^"+centrald.ErrorInternalError.M+": crypt failure", err.M)
	}
	assert.Equal(1, tl.CountPattern("crypt failure"))
}

func TestUserCreate(t *testing.T) {
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
	params := ops.UserCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
		Payload: &models.User{
			UserMutable: models.UserMutable{
				AuthIdentifier: "user1@nuvoloso.com",
				Password:       "1",
			},
		},
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.User{
		UserAllOf0: models.UserAllOf0{
			Meta: &models.ObjMeta{
				ID:      models.ObjID("objectID"),
				Version: 1,
			},
		},
		UserMutable: models.UserMutable{
			AuthIdentifier: params.Payload.AuthIdentifier,
		},
	}
	sysObj := &models.System{
		SystemMutable: models.SystemMutable{
			UserPasswordPolicy: &models.SystemMutableUserPasswordPolicy{
				MinLength: 1,
			},
		},
	}

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oS := mock.NewMockSystemOps(mockCtrl)
	oS.EXPECT().Fetch().Return(sysObj, nil)
	oU := mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Create(ctx, params.Payload).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSystem().Return(oS)
	mds.EXPECT().OpsUser().Return(oU).MinTimes(1)
	hc.DS = mds
	cntRLock := hc.cntRLock
	cntRUnlock := hc.cntRUnlock
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.userCreate(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.UserCreateCreated)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Len(evM.InSSProps, 1)
	assert.EqualValues("objectID", evM.InSSProps["meta.id"])
	assert.Equal(obj, evM.InACScope)
	exp := &fal.Args{AI: ai, Action: centrald.UserCreateAction, ObjID: "objectID", Name: models.ObjName(params.Payload.AuthIdentifier), Message: "Created"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Create fails")
	mockCtrl = gomock.NewController(t)
	oS = mock.NewMockSystemOps(mockCtrl)
	oS.EXPECT().Fetch().Return(sysObj, nil)
	oU = mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Create(ctx, params.Payload).Return(nil, centrald.ErrorExists)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSystem().Return(oS)
	mds.EXPECT().OpsUser().Return(oU).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.userCreate(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.UserCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorExists.M, *mE.Payload.Message)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: auditLog not ready")
	fa.ReadyRet = centrald.ErrorDbError
	params.Payload.Password = "1"
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oS = mock.NewMockSystemOps(mockCtrl)
	mds.EXPECT().OpsSystem().Return(oS)
	oS.EXPECT().Fetch().Return(sysObj, nil)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.userCreate(params) })
	mE, ok = ret.(*ops.UserCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.userCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.UserCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	t.Log("case: unauthorized")
	fa.Posts = []*fal.Args{}
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{}
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oS = mock.NewMockSystemOps(mockCtrl)
	mds.EXPECT().OpsSystem().Return(oS)
	oS.EXPECT().Fetch().Return(sysObj, nil)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.userCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.UserCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	exp = &fal.Args{AI: ai, Action: centrald.UserCreateAction, Name: models.ObjName(params.Payload.AuthIdentifier), Err: true, Message: "Create unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	ai.RoleObj = nil
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: invalid password")
	params.Payload.Password = ""
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oS = mock.NewMockSystemOps(mockCtrl)
	mds.EXPECT().OpsSystem().Return(oS)
	oS.EXPECT().Fetch().Return(sysObj, nil)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.userCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.UserCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
	params.Payload.Password = "123"
	tl.Flush()

	t.Log("case: no authIdentifier")
	params.Payload.AuthIdentifier = ""
	assert.NotPanics(func() { ret = hc.userCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.UserCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
	assert.Regexp("authIdentifier$", *mE.Payload.Message)
	tl.Flush()

	t.Log("case: nil payload")
	params.Payload = nil
	assert.NotPanics(func() { ret = hc.userCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.UserCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorMissing.M, *mE.Payload.Message)
}

func TestUserDelete(t *testing.T) {
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
	params := ops.UserDeleteParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.User{
		UserAllOf0: models.UserAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("objectID"),
			},
		},
		UserMutable: models.UserMutable{
			AuthIdentifier: "user@nuvoloso.com",
		},
	}
	aParams := account.AccountListParams{UserID: &params.ID}

	// success
	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Count(ctx, aParams, uint(1)).Return(0, nil)
	oU := mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oU.EXPECT().Delete(ctx, params.ID).Return(nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	mds.EXPECT().OpsUser().Return(oU).MinTimes(1)
	hc.DS = mds
	var ret middleware.Responder
	cntLock := hc.cntLock
	cntUnlock := hc.cntUnlock
	assert.NotPanics(func() { ret = hc.userDelete(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.UserDeleteNoContent)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Empty(evM.InSSProps)
	assert.Equal(obj, evM.InACScope)
	exp := &fal.Args{AI: ai, Action: centrald.UserDeleteAction, ObjID: models.ObjID(params.ID), Name: models.ObjName(obj.AuthIdentifier), Message: "Deleted"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	tl.Flush()

	// admin user
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: delete admin user failure")
	obj.AuthIdentifier = centrald.SystemUser
	oU = mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.userDelete(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.UserDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Regexp(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	assert.Equal(cntLock, hc.cntLock)
	assert.Equal(cntUnlock, hc.cntUnlock)
	obj.AuthIdentifier = "user@nuvoloso.com"
	tl.Flush()

	// user in account(s)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: user in account(s)")
	oU = mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Count(ctx, aParams, uint(1)).Return(1, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	mds.EXPECT().OpsUser().Return(oU)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.userDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.UserDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Regexp("accounts are still associated", *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: auditLog not ready")
	mds = mock.NewMockDataStore(mockCtrl)
	oU = mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsUser().Return(oU)
	fa.ReadyRet = centrald.ErrorDbError
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.userDelete(params) })
	mE, ok = ret.(*ops.UserDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.userDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.UserDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	// unauthorized
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: unauthorized")
	fa.Posts = []*fal.Args{}
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{}
	oU = mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.userDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.UserDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	exp = &fal.Args{AI: ai, Action: centrald.UserDeleteAction, ObjID: models.ObjID(params.ID), Name: models.ObjName(obj.AuthIdentifier), Err: true, Message: "Delete unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	ai.RoleObj = nil
	tl.Flush()

	// delete failure case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: delete failure")
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Count(ctx, aParams, uint(1)).Return(0, nil)
	oU = mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oU.EXPECT().Delete(ctx, params.ID).Return(centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	mds.EXPECT().OpsUser().Return(oU).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.userDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.UserDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	tl.Flush()

	// fetch failure case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: fetch failure")
	oU = mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.userDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.UserDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
	assert.Equal(cntLock, hc.cntLock)
	assert.Equal(cntUnlock, hc.cntUnlock)
	tl.Flush()
}

func TestUserFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	params := ops.UserFetchParams{
		HTTPRequest: requestWithAuthContext(&auth.Info{}),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.User{
		UserAllOf0: models.UserAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("objectID"),
			},
		},
	}
	aRes := []*models.Account{}
	rRes := []*models.Role{}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mds := mock.NewMockDataStore(mockCtrl)
	oU := mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsUser().Return(oU)
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().List(ctx, account.AccountListParams{UserID: swag.String(params.ID)}).Return(aRes, nil)
	mds.EXPECT().OpsAccount().Return(oA)
	oR := mock.NewMockRoleOps(mockCtrl)
	oR.EXPECT().List(role.RoleListParams{}).Return(rRes, nil)
	mds.EXPECT().OpsRole().Return(oR)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.userFetch(params) })
	assert.NotNil(ret)
	mR, ok := ret.(*ops.UserFetchOK)
	assert.True(ok)
	assert.Equal(obj, mR.Payload)
	assert.NotNil(obj.AccountRoles)
	tl.Flush()

	// fetch failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oU = mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU).MinTimes(1)
	t.Log("case: fetch failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.userFetch(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.UserFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)

	// deriveAccountRoles fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oU = mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsUser().Return(oU)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().List(ctx, account.AccountListParams{UserID: swag.String(params.ID)}).Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsAccount().Return(oA)
	t.Log("case: deriveAccountRoles failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.userFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.UserFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.userFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.UserFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
}

func TestUserList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	params := ops.UserListParams{HTTPRequest: requestWithAuthContext(&auth.Info{})}
	ctx := params.HTTPRequest.Context()
	oID := "objectID"
	objects := []*models.User{
		&models.User{
			UserAllOf0: models.UserAllOf0{
				Meta: &models.ObjMeta{
					ID: models.ObjID(oID),
				},
			},
		},
	}
	aRes := []*models.Account{}
	rRes := []*models.Role{}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mds := mock.NewMockDataStore(mockCtrl)
	oU := mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().List(ctx, params).Return(objects, nil)
	mds.EXPECT().OpsUser().Return(oU)
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().List(ctx, account.AccountListParams{UserID: swag.String(oID)}).Return(aRes, nil)
	mds.EXPECT().OpsAccount().Return(oA)
	oR := mock.NewMockRoleOps(mockCtrl)
	oR.EXPECT().List(role.RoleListParams{}).Return(rRes, nil)
	mds.EXPECT().OpsRole().Return(oR)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.userList(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.UserListOK)
	assert.True(ok)
	assert.Equal(objects, mO.Payload)
	assert.NotNil(objects[0].AccountRoles)
	tl.Flush()

	// list fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oU = mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().List(ctx, params).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU).MinTimes(1)
	t.Log("case: List failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.userList(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.UserListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)

	// deriveAccountRoles fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oU = mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().List(ctx, params).Return(objects, nil)
	mds.EXPECT().OpsUser().Return(oU)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().List(ctx, account.AccountListParams{UserID: swag.String(oID)}).Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsAccount().Return(oA)
	t.Log("case: deriveAccountRoles failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.userList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.UserListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.userList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.UserListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
}

func TestUserUpdate(t *testing.T) {
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
	assert.NotPanics(func() { nMap = hc.userMutableNameMap() })

	// parse params
	objM := &models.UserMutable{
		AuthIdentifier: "user2@nuvoloso.com",
		Password:       "1",
	}
	ai := &auth.Info{}
	params := ops.UserUpdateParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
		Version:     swag.Int32(8),
		Remove:      []string{},
		Append:      []string{},
		Set:         []string{nMap.jName("AuthIdentifier")},
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

	obj := &models.User{
		UserAllOf0: models.UserAllOf0{
			Meta: &models.ObjMeta{ID: models.ObjID("objectID"), Version: 8},
		},
		UserMutable: models.UserMutable{
			AuthIdentifier: "user1@nuvoloso.com",
		},
	}
	sysObj := &models.System{
		SystemMutable: models.SystemMutable{
			UserPasswordPolicy: &models.SystemMutableUserPasswordPolicy{
				MinLength: 1,
			},
		},
	}

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oU := mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oU.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU).MinTimes(1)
	hc.DS = mds
	cntRLock := hc.cntRLock
	cntRUnlock := hc.cntRUnlock
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.userUpdate(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.UserUpdateOK)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Equal(obj, mO.Payload)
	assert.Empty(evM.InSSProps)
	assert.Equal(obj, evM.InACScope)
	exp := &fal.Args{AI: ai, Action: centrald.UserUpdateAction, ObjID: models.ObjID(params.ID), Name: models.ObjName(obj.AuthIdentifier), Message: "Updated authIdentifier"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: success enabled, valid password")
	mockCtrl = gomock.NewController(t)
	fa.Posts = []*fal.Args{}
	ai.UserID = params.ID
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{}
	params.Set = []string{nMap.jName("Disabled"), nMap.jName("Password")}
	uP[centrald.UpdateSet] = params.Set
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	uaM = updateArgsMatcher(t, ua).Matcher()
	oS := mock.NewMockSystemOps(mockCtrl)
	oS.EXPECT().Fetch().Return(sysObj, nil)
	oU = mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oU.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSystem().Return(oS)
	mds.EXPECT().OpsUser().Return(oU).MinTimes(1)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.userUpdate(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.UserUpdateOK)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Equal(obj, mO.Payload)
	assert.Empty(evM.InSSProps)
	assert.Equal(obj, evM.InACScope)
	exp.Message = "Updated enabled, password"
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: success authIdentifier and enabled")
	mockCtrl = gomock.NewController(t)
	fa.Posts = []*fal.Args{}
	ai.UserID = params.ID
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{}
	params.Set = []string{nMap.jName("AuthIdentifier"), nMap.jName("Disabled")}
	params.Payload.Disabled = true
	uP[centrald.UpdateSet] = params.Set
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	uaM = updateArgsMatcher(t, ua).Matcher()
	oU = mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oU.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU).MinTimes(1)
	hc.DS = mds
	cntRLock = hc.cntRLock
	cntRUnlock = hc.cntRUnlock
	assert.NotPanics(func() { ret = hc.userUpdate(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.UserUpdateOK)
	assert.True(ok)
	assert.Equal(cntRLock+1, hc.cntRLock)
	assert.Equal(cntRUnlock+1, hc.cntRUnlock)
	assert.Equal(obj, mO.Payload)
	assert.Empty(evM.InSSProps)
	assert.Equal(obj, evM.InACScope)
	exp.Message = "Updated authIdentifier, disabled"
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: update failure") // also cover version added case
	mockCtrl = gomock.NewController(t)
	ai.UserID = params.ID
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{}
	params.Version = nil
	ua.Version = 0
	uaM = updateArgsMatcher(t, ua).AddsVersion(8).Matcher()
	oU = mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oU.EXPECT().Update(ctx, uaM, params.Payload).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.userUpdate(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.UserUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: fetch failure")
	mockCtrl = gomock.NewController(t)
	oU = mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.userUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.UserUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: version mismatch")
	mockCtrl = gomock.NewController(t)
	params.Version = swag.Int32(7)
	oU = mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.userUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.UserUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorIDVerNotFound.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorIDVerNotFound.M, *mD.Payload.Message)
	params.Version = swag.Int32(8)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: auditLog not ready")
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oU = mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsUser().Return(oU)
	fa.ReadyRet = centrald.ErrorDbError
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.userUpdate(params) })
	mD, ok = ret.(*ops.UserUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.userUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.UserUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	t.Log("case: invalid password")
	params.Set = []string{nMap.jName("Password")}
	params.Payload.Password = ""
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oU = mock.NewMockUserOps(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU).MinTimes(1)
	oU.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oS = mock.NewMockSystemOps(mockCtrl)
	mds.EXPECT().OpsSystem().Return(oS)
	oS.EXPECT().Fetch().Return(sysObj, nil)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.userUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.UserUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	params.Payload.Password = "123"
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: unauthorized")
	mockCtrl = gomock.NewController(t)
	params.Set = []string{nMap.jName("AuthIdentifier"), nMap.jName("Disabled")}
	ai.UserID = "otherId"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{}
	fa.Posts = []*fal.Args{}
	oU = mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.userUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.UserUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
	exp = &fal.Args{AI: ai, Action: centrald.UserUpdateAction, ObjID: models.ObjID(params.ID), Name: models.ObjName(obj.AuthIdentifier), Err: true, Message: "Update unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: unauthorized admin password update")
	ai.RoleObj.Capabilities[centrald.UserManagementCap] = true
	params.Set = []string{nMap.jName("Password")}
	params.Payload.Password = "123"
	obj.AuthIdentifier = "admin"
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oU = mock.NewMockUserOps(mockCtrl)
	mds.EXPECT().OpsUser().Return(oU).MinTimes(1)
	oU.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oS = mock.NewMockSystemOps(mockCtrl)
	mds.EXPECT().OpsSystem().Return(oS)
	oS.EXPECT().Fetch().Return(sysObj, nil)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.userUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.UserUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)
	ai.RoleObj = nil
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: no change")
	params.Set = []string{}
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.userUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.UserUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: empty authIdentifier")
	params.Set = []string{"authIdentifier"}
	params.Payload.AuthIdentifier = ""
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.userUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.UserUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: no payload")
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	params.Payload = nil
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.userUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.UserUpdateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
		assert.Equal(centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	}
}
