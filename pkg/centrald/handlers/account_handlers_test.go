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
	"strings"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/role"
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

func TestAccountFetchFilter(t *testing.T) {
	assert := assert.New(t)
	hc := newHandlerComp()
	obj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{Meta: &models.ObjMeta{ID: "aid1"}},
		AccountMutable: models.AccountMutable{
			UserRoles: map[string]models.AuthRole{
				"uid1": {Disabled: false, RoleID: "rid1"},
				"uid2": {Disabled: true, RoleID: "rid1"},
			},
		},
	}
	rObj := &models.Role{}
	rObj.Capabilities = map[string]bool{}
	ai := &auth.Info{}

	t.Log("case: AccountFetchAllRolesCap capability")
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.AccountFetchAllRolesCap: true}
	ai.AccountID = "aid1"
	assert.Error(ai.InternalOK())
	assert.NoError(hc.accountFetchFilter(ai, obj))
	assert.Nil(obj.Secrets)

	t.Log("case: AccountFetchOwnRoleCap capability")
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.AccountFetchOwnRoleCap: true}
	ai.AccountID = "aid1"
	ai.UserID = "uid1"
	saveRoles := obj.UserRoles
	assert.NoError(hc.accountFetchFilter(ai, obj))
	assert.Len(obj.UserRoles, 1)
	obj.UserRoles = saveRoles
	assert.Len(obj.UserRoles, 2)

	t.Log("case: system admin role")
	ai.RoleObj.Capabilities = map[string]bool{centrald.ManageSpecialAccountsCap: true}
	assert.NoError(hc.accountFetchFilter(ai, obj))

	t.Log("case: tenant admin role")
	ai.AccountID = "tid1"
	ai.RoleObj.Capabilities = map[string]bool{centrald.ManageNormalAccountsCap: true}
	obj.TenantAccountID = "tid1"
	assert.NoError(hc.accountFetchFilter(ai, obj))

	t.Log("case: wrong tenant admin, user has no role")
	ai.UserID = "anotherUID"
	ai.AccountID = "tid2"
	ai.RoleObj.Capabilities = map[string]bool{centrald.ManageNormalAccountsCap: true}
	obj.TenantAccountID = "tid1"
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.accountFetchFilter(ai, obj))

	t.Log("case: wrong tenant admin, user disabled")
	ai.UserID = "uid2"
	ai.AccountID = "tid2"
	ai.RoleObj.Capabilities = map[string]bool{centrald.ManageNormalAccountsCap: true}
	obj.TenantAccountID = "tid1"
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.accountFetchFilter(ai, obj))

	t.Log("case: wrong tenant admin")
	ai.UserID = "uid1"
	ai.AccountID = "tid2"
	obj.TenantAccountID = "tid1"
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mds := mock.NewMockDataStore(mockCtrl)
	hc.DS = mds
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.accountFetchFilter(ai, obj))

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: role fetch fails")
	ai.UserID = "uid1"
	ai.AccountID = ""
	obj.TenantAccountID = "tid1"
	mds = mock.NewMockDataStore(mockCtrl)
	oR := mock.NewMockRoleOps(mockCtrl)
	oR.EXPECT().Fetch("rid1").Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsRole().Return(oR)
	hc.DS = mds
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.accountFetchFilter(ai, obj))

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: user has no account role")
	ai.UserID = "uid1"
	ai.AccountID = ""
	obj.TenantAccountID = "tid1"
	mds = mock.NewMockDataStore(mockCtrl)
	oR = mock.NewMockRoleOps(mockCtrl)
	oR.EXPECT().Fetch("rid1").Return(rObj, nil)
	mds.EXPECT().OpsRole().Return(oR)
	hc.DS = mds
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden, hc.accountFetchFilter(ai, obj))

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: user has AccountFetchAllRolesCap on account")
	ai.UserID = "uid1"
	ai.AccountID = ""
	obj.TenantAccountID = "tid1"
	rObj.Capabilities[centrald.AccountFetchAllRolesCap] = true
	mds = mock.NewMockDataStore(mockCtrl)
	oR = mock.NewMockRoleOps(mockCtrl)
	oR.EXPECT().Fetch("rid1").Return(rObj, nil)
	mds.EXPECT().OpsRole().Return(oR)
	hc.DS = mds
	assert.NoError(hc.accountFetchFilter(ai, obj))
	assert.Len(obj.UserRoles, 2)

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: user has AccountFetchOwnRoleCap on account")
	ai.UserID = "uid1"
	ai.AccountID = ""
	obj.TenantAccountID = "tid1"
	rObj.Capabilities[centrald.AccountFetchAllRolesCap] = false
	rObj.Capabilities[centrald.AccountFetchOwnRoleCap] = true
	mds = mock.NewMockDataStore(mockCtrl)
	oR = mock.NewMockRoleOps(mockCtrl)
	oR.EXPECT().Fetch("rid1").Return(rObj, nil)
	mds.EXPECT().OpsRole().Return(oR)
	hc.DS = mds
	assert.NoError(hc.accountFetchFilter(ai, obj))
	assert.Len(obj.UserRoles, 1)

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: user has AccountFetchOwnRoleCap on subordinate account")
	ai.UserID = "uid3"
	ai.AccountID = "sid1"
	ai.TenantAccountID = "aid1"
	obj.TenantAccountID = ""
	oRoles := obj.UserRoles
	ai.RoleObj.Capabilities = map[string]bool{centrald.AccountFetchOwnRoleCap: true}
	mds = mock.NewMockDataStore(mockCtrl)
	hc.DS = mds
	assert.NoError(hc.accountFetchFilter(ai, obj))
	assert.Empty(obj.UserRoles)

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: user has AccountFetchAllRolesCap on subordinate account")
	ai.UserID = "uid3"
	ai.AccountID = "sid1"
	ai.TenantAccountID = "aid1"
	obj.TenantAccountID = ""
	obj.UserRoles = oRoles
	ai.RoleObj.Capabilities = map[string]bool{centrald.AccountFetchAllRolesCap: true}
	mds = mock.NewMockDataStore(mockCtrl)
	hc.DS = mds
	assert.NoError(hc.accountFetchFilter(ai, obj))
	assert.Empty(obj.UserRoles)
}

func TestAccountApplyInheritedProperties(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()

	ctx := context.Background()

	aObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta:            &models.ObjMeta{ID: "aid1"},
			TenantAccountID: "tid",
		},
		AccountMutable: models.AccountMutable{
			UserRoles: map[string]models.AuthRole{
				"uid1": {Disabled: false, RoleID: "rid1"},
				"uid2": {Disabled: true, RoleID: "rid1"},
			},
		},
	}
	taObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{ID: "tid"},
		},
		AccountMutable: models.AccountMutable{
			SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
				CspDomainID:        "domID",
				ProtectionDomainID: "pdID",
			},
		},
	}
	sysObj := &models.System{
		SystemAllOf0: models.SystemAllOf0{
			Meta: &models.ObjMeta{
				ID:      "sys_id1",
				Version: 1,
			},
		},
		SystemMutable: models.SystemMutable{
			SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
				RetentionDurationSeconds: swag.Int32(99),
			},
			SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
				CspDomainID: "domID-s",
			},
			VsrManagementPolicy: &models.VsrManagementPolicy{
				RetentionDurationSeconds: swag.Int32(100),
				NoDelete:                 false,
			},
		},
	}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()

	t.Log("case: policies not inherited")
	aObj.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{
		RetentionDurationSeconds: swag.Int32(99),
		Inherited:                false,
	}
	aObj.SnapshotCatalogPolicy = &models.SnapshotCatalogPolicy{
		CspDomainID:        "domID",
		ProtectionDomainID: "pdID",
		Inherited:          false,
	}
	aObj.VsrManagementPolicy = &models.VsrManagementPolicy{
		RetentionDurationSeconds: swag.Int32(100),
		Inherited:                false,
	}
	err := hc.accountApplyInheritedProperties(ctx, aObj)
	assert.Nil(err)
	tl.Flush()

	t.Log("case: nil policies")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	aObj.SnapshotManagementPolicy = nil
	aObj.SnapshotCatalogPolicy = nil
	aObj.VsrManagementPolicy = nil
	oSys := mock.NewMockSystemOps(mockCtrl)
	oSys.EXPECT().Fetch().Return(sysObj, nil).MinTimes(1)
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(aObj.TenantAccountID)).Return(taObj, nil).MinTimes(1)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSystem().Return(oSys).MinTimes(1)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	err = hc.accountApplyInheritedProperties(ctx, aObj)
	assert.Nil(err)
	assert.NotEqual(aObj.SnapshotManagementPolicy, sysObj.SnapshotManagementPolicy)
	assert.NotEqual(aObj.SnapshotCatalogPolicy, sysObj.SnapshotCatalogPolicy)
	assert.NotEqual(aObj.VsrManagementPolicy, sysObj.VsrManagementPolicy)
	tl.Flush()

	t.Log("case: policies not inherited")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	aObj.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{
		Inherited: false,
	}
	aObj.SnapshotCatalogPolicy = &models.SnapshotCatalogPolicy{
		Inherited: false,
	}
	aObj.VsrManagementPolicy = &models.VsrManagementPolicy{
		Inherited: true,
	}
	oSys = mock.NewMockSystemOps(mockCtrl)
	oSys.EXPECT().Fetch().Return(sysObj, nil).MinTimes(1)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSystem().Return(oSys).MinTimes(1)
	hc.DS = mds
	err = hc.accountApplyInheritedProperties(ctx, aObj)
	assert.Nil(err)
	assert.NotEqual(aObj.SnapshotManagementPolicy, sysObj.SnapshotManagementPolicy)
	assert.NotEqual(aObj.SnapshotCatalogPolicy, sysObj.SnapshotCatalogPolicy)
	assert.NotEqual(aObj.VsrManagementPolicy, sysObj.VsrManagementPolicy)
	tl.Flush()

	t.Log("case: error fetching system object")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	aObj.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{
		RetentionDurationSeconds: swag.Int32(99),
		Inherited:                true,
	}
	aObj.SnapshotCatalogPolicy = &models.SnapshotCatalogPolicy{
		CspDomainID:        "domID",
		ProtectionDomainID: "pdID",
		Inherited:          false,
	}
	aObj.VsrManagementPolicy = &models.VsrManagementPolicy{
		RetentionDurationSeconds: swag.Int32(100),
		Inherited:                true,
	}
	oSys = mock.NewMockSystemOps(mockCtrl)
	oSys.EXPECT().Fetch().Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSystem().Return(oSys)
	hc.DS = mds
	err = hc.accountApplyInheritedProperties(ctx, aObj)
	assert.NotNil(err)
	tl.Flush()

	t.Log("case: error fetching tenant account object, system object is ok")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	aObj.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{
		RetentionDurationSeconds: swag.Int32(999),
		Inherited:                false,
	}
	aObj.SnapshotCatalogPolicy = &models.SnapshotCatalogPolicy{
		Inherited: true,
	}
	aObj.VsrManagementPolicy = &models.VsrManagementPolicy{
		RetentionDurationSeconds: swag.Int32(1000),
		Inherited:                false,
	}
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, string(aObj.TenantAccountID)).Return(nil, centrald.ErrorDbError)
	oSys = mock.NewMockSystemOps(mockCtrl)
	oSys.EXPECT().Fetch().Return(sysObj, nil).MinTimes(1)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSystem().Return(oSys).MinTimes(1)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	err = hc.accountApplyInheritedProperties(ctx, aObj)
	assert.Nil(err)
	assert.NotEqual(aObj.SnapshotManagementPolicy, sysObj.SnapshotManagementPolicy)
	assert.NotEqual(aObj.VsrManagementPolicy, sysObj.VsrManagementPolicy)
	assert.NotEqual(aObj.SnapshotCatalogPolicy.Inherited, sysObj.SnapshotCatalogPolicy.Inherited)
	assert.Equal(aObj.SnapshotCatalogPolicy.CspDomainID, sysObj.SnapshotCatalogPolicy.CspDomainID)
	assert.Equal(aObj.SnapshotCatalogPolicy.ProtectionDomainID, sysObj.SnapshotCatalogPolicy.ProtectionDomainID)
	tl.Flush()
}

func TestCheckForAvailableSnapshotCatalogPolicy(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()

	ctx := context.Background()

	taID := "tid"
	taObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{ID: models.ObjID(taID)},
		},
		AccountMutable: models.AccountMutable{
			SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
				CspDomainID:        "domID",
				ProtectionDomainID: "pdID",
			},
		},
	}
	sysObj := &models.System{
		SystemAllOf0: models.SystemAllOf0{
			Meta: &models.ObjMeta{
				ID:      "sys_id1",
				Version: 1,
			},
		},
		SystemMutable: models.SystemMutable{
			SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
				RetentionDurationSeconds: swag.Int32(99),
			},
			SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
				CspDomainID:        "domID-s",
				ProtectionDomainID: "pdID",
			},
			VsrManagementPolicy: &models.VsrManagementPolicy{
				RetentionDurationSeconds: swag.Int32(100),
				NoDelete:                 false,
			},
		},
	}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()

	t.Log("case: failure fetching tenant account, System object is found")
	mockCtrl = gomock.NewController(t)
	oSys := mock.NewMockSystemOps(mockCtrl)
	oSys.EXPECT().Fetch().Return(sysObj, nil).MinTimes(1)
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, taID).Return(nil, centrald.ErrorDbError).MinTimes(1)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSystem().Return(oSys).MinTimes(1)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	assert.True(hc.checkForAvailableSnapshotCatalogPolicy(ctx, taID))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: tenant account has policy set")
	mockCtrl = gomock.NewController(t)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, taID).Return(taObj, nil).MinTimes(1)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	assert.True(hc.checkForAvailableSnapshotCatalogPolicy(ctx, taID))
	tl.Flush()

	t.Log("case: no policies found")
	taObj.SnapshotCatalogPolicy = nil
	sysObj.SnapshotCatalogPolicy = nil
	mockCtrl = gomock.NewController(t)
	oSys = mock.NewMockSystemOps(mockCtrl)
	oSys.EXPECT().Fetch().Return(sysObj, nil).MinTimes(1)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, taID).Return(taObj, nil).MinTimes(1)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSystem().Return(oSys).MinTimes(1)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	assert.False(hc.checkForAvailableSnapshotCatalogPolicy(ctx, taID))
	tl.Flush()
}

func TestValidateAccountSnapshotCatalogPolicy(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	fOps := &fakeOps{}
	hc.ops = fOps

	ctx := context.Background()

	aID := "accountID"
	ai := &auth.Info{}

	aObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta:            &models.ObjMeta{ID: models.ObjID(aID)},
			TenantAccountID: "tid",
		},
		AccountMutable: models.AccountMutable{
			SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
				CspDomainID:        "domID",
				ProtectionDomainID: "pdID",
				Inherited:          false,
			},
		},
	}
	pdObj := &models.ProtectionDomain{}
	pdObj.Meta = &models.ObjMeta{ID: "pdID"}
	pdObj.AccountID = models.ObjIDMutable(aID)
	pdObj.Name = "PD"
	pdID := string(pdObj.Meta.ID)

	cspObj := &models.CSPDomain{}
	cspObj.Meta = &models.ObjMeta{ID: "cspID"}
	cspObj.AccountID = models.ObjIDMutable(aID)
	cspObj.AuthorizedAccounts = []models.ObjIDMutable{pdObj.AccountID}
	cspObj.Name = "CSP"
	cspID := string(cspObj.Meta.ID)

	aObj.ProtectionDomains = map[string]models.ObjIDMutable{
		common.ProtectionStoreDefaultKey: models.ObjIDMutable(pdID),
		cspID:                            models.ObjIDMutable(pdID),
	}

	invalidSCP := []*models.SnapshotCatalogPolicy{
		nil,
		&models.SnapshotCatalogPolicy{},
		&models.SnapshotCatalogPolicy{CspDomainID: "cspID", ProtectionDomainID: "", Inherited: false},
		&models.SnapshotCatalogPolicy{CspDomainID: "", ProtectionDomainID: "pdID", Inherited: false},
		&models.SnapshotCatalogPolicy{CspDomainID: "", ProtectionDomainID: "", Inherited: false},
	}
	for i, p := range invalidSCP {
		assert.Error(hc.validateAccountSnapshotCatalogPolicy(ctx, ai, aObj, p), "p %d", i)
	}

	t.Log("policy not inherited")
	assert.NoError(hc.validateAccountSnapshotCatalogPolicy(ctx, ai, aObj, &models.SnapshotCatalogPolicy{Inherited: true}))

	t.Log("success, policy inherited")
	testutils.Clone(pdObj, &fOps.RetProtectionDomainFetchObj)
	fOps.RetProtectionDomainFetchErr = nil
	testutils.Clone(cspObj, &fOps.RetCspDomainFetchObj)
	fOps.RetCspDomainFetchErr = nil
	assert.NoError(hc.validateAccountSnapshotCatalogPolicy(ctx, ai, aObj, &models.SnapshotCatalogPolicy{CspDomainID: "cspID", ProtectionDomainID: "pdID", Inherited: false}))

	t.Log("failure to fetch protection domain")
	testutils.Clone(pdObj, &fOps.RetProtectionDomainFetchObj)
	fOps.RetProtectionDomainFetchErr = centrald.ErrorDbError
	fOps.RetCspDomainFetchErr = centrald.ErrorDbError
	assert.Error(hc.validateAccountSnapshotCatalogPolicy(ctx, ai, aObj, &models.SnapshotCatalogPolicy{CspDomainID: "cspID", ProtectionDomainID: "pdID", Inherited: false}))

	t.Log("failure to fetch csp domain")
	testutils.Clone(pdObj, &fOps.RetProtectionDomainFetchObj)
	fOps.RetProtectionDomainFetchErr = nil
	fOps.RetCspDomainFetchErr = centrald.ErrorDbError
	assert.Error(hc.validateAccountSnapshotCatalogPolicy(ctx, ai, aObj, &models.SnapshotCatalogPolicy{CspDomainID: "cspID", ProtectionDomainID: "pdID", Inherited: false}))

	t.Log("failure, account is not authorized to use CSPDomain")
	testutils.Clone(pdObj, &fOps.RetProtectionDomainFetchObj)
	fOps.RetProtectionDomainFetchErr = nil
	testutils.Clone(cspObj, &fOps.RetCspDomainFetchObj)
	fOps.RetCspDomainFetchObj.AuthorizedAccounts = []models.ObjIDMutable{}
	fOps.RetCspDomainFetchObj.AccountID = "otherID"
	fOps.RetCspDomainFetchErr = nil
	err := hc.validateAccountSnapshotCatalogPolicy(ctx, ai, aObj, &models.SnapshotCatalogPolicy{CspDomainID: "cspID", ProtectionDomainID: "pdID", Inherited: false})
	assert.Error(err)
	assert.Regexp("account is not authorized to use CSPDomain cspID", err)
}

func TestValidateUserRoles(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	obj := &models.Account{}
	cObj := &models.Account{
		AccountMutable: models.AccountMutable{
			UserRoles: map[string]models.AuthRole{
				"user-1": models.AuthRole{RoleID: "role-a"},
			},
		},
	}
	uObj := &models.AccountMutable{
		UserRoles: map[string]models.AuthRole{
			"user-1": models.AuthRole{RoleID: "role-a"},
		},
	}
	userObj := &models.User{}
	userObj.AuthIdentifier = "bender@nuvoloso.com"
	ctx := context.Background()

	// works with empty obj only
	userCache := accountUserMap{}
	assert.NoError(hc.validateUserRoles(ctx, nil, obj, nil, userCache))
	assert.Empty(userCache)

	// update: empty userRoles
	attr := &centrald.UpdateAttr{
		Name: "UserRoles",
		Actions: [centrald.NumActionTypes]centrald.UpdateActionArgs{
			centrald.UpdateSet: centrald.UpdateActionArgs{FromBody: true},
		},
	}
	assert.NoError(hc.validateUserRoles(ctx, attr, obj, &models.AccountMutable{}, userCache))
	assert.Empty(userCache)

	t.Log("user does not exist")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mds := mock.NewMockDataStore(mockCtrl)
	oU := mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, "user-1").Return(nil, centrald.ErrorNotFound)
	mds.EXPECT().OpsUser().Return(oU)
	hc.DS = mds
	err := hc.validateUserRoles(ctx, nil, cObj, nil, userCache)
	assert.Regexp("missing: invalid userId: user-1", err)
	assert.Empty(userCache)
	tl.Flush()

	t.Log("user db error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oU = mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, "user-1").Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsUser().Return(oU)
	hc.DS = mds
	err = hc.validateUserRoles(ctx, nil, cObj, nil, userCache)
	assert.Equal(centrald.ErrorDbError, err)
	assert.Empty(userCache)
	tl.Flush()

	t.Log("wrong userRole on creation")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oU = mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, "user-1").Return(userObj, nil)
	mds.EXPECT().OpsUser().Return(oU)
	hc.DS = mds
	err = hc.validateUserRoles(ctx, nil, cObj, nil, userCache)
	assert.Regexp("invalid user roleId.*role-a", err)
	assert.Len(userCache, 1)
	_, exists := userCache["user-1"]
	assert.True(exists, "user-1 in cache")
	tl.Flush()

	t.Log("success: userRole on creation, user cached")
	cObj.AccountRoles = []models.ObjIDMutable{"role-a"}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	hc.DS = mds
	assert.NoError(hc.validateUserRoles(ctx, nil, cObj, nil, userCache))
	tl.Flush()

	t.Log("success: update with userRoles")
	obj.AccountRoles = []models.ObjIDMutable{"role-a", "role-u"}
	userCache = accountUserMap{}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oU = mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, "user-1").Return(userObj, nil)
	mds.EXPECT().OpsUser().Return(oU)
	hc.DS = mds
	assert.NoError(hc.validateUserRoles(ctx, attr, obj, uObj, userCache))
	tl.Flush()

	t.Log("wrong userRole on update")
	uObj.UserRoles["user-1"] = models.AuthRole{RoleID: "role-2"}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	hc.DS = mds
	err = hc.validateUserRoles(ctx, attr, obj, uObj, userCache)
	assert.Regexp("invalid update.*: invalid user roleId.* role-2", err)
}

func TestAccountUserRolesAuditMsg(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	oObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{
				ID:      models.ObjID("aid-1"),
				Version: 1,
			},
		},
		AccountMutable: models.AccountMutable{
			UserRoles: map[string]models.AuthRole{
				"user-1": models.AuthRole{RoleID: "role-a"},
				"user-2": models.AuthRole{RoleID: "role-a"},
				"user-4": models.AuthRole{RoleID: "role-a"},
			},
		},
	}
	uObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{
				ID:      models.ObjID("aid-1"),
				Version: 1,
			},
		},
		AccountMutable: models.AccountMutable{
			UserRoles: map[string]models.AuthRole{
				"user-1": models.AuthRole{RoleID: "role-a"},
				"user-2": models.AuthRole{RoleID: "role-a", Disabled: true},
				"user-3": models.AuthRole{RoleID: "role-u"},
			},
		},
	}
	userObj := &models.User{}
	userObj.AuthIdentifier = "bender@nuvoloso.com"
	user2Obj := &models.User{}
	user2Obj.AuthIdentifier = "tad@nuvoloso.com"
	user3Obj := &models.User{}
	user3Obj.AuthIdentifier = "stephen@nuvoloso.com"
	user4Obj := &models.User{}
	user4Obj.AuthIdentifier = "carl@nuvoloso.com"

	// works with empty uObj only
	userCache := accountUserMap{}
	msg := hc.accountUserRolesAuditMsg(nil, &models.Account{}, userCache)
	assert.Empty(msg)

	// works with both empty objects
	msg = hc.accountUserRolesAuditMsg(&models.Account{}, &models.Account{}, userCache)
	assert.Empty(msg)

	// creation msg, one account role
	userCache["user-1"] = userObj
	userCache["user-2"] = user2Obj
	userCache["user-3"] = user3Obj
	uObj.AccountRoles = []models.ObjIDMutable{"role-a"}
	msg = hc.accountUserRolesAuditMsg(nil, uObj, userCache)
	assert.Equal("userRoles[bender@nuvoloso.com, stephen@nuvoloso.com; disabled: tad@nuvoloso.com]", msg)

	// update msg, one role, several cases
	userCache["user-4"] = user4Obj
	msg = hc.accountUserRolesAuditMsg(oObj, uObj, userCache)
	assert.Equal("userRoles[added: stephen@nuvoloso.com; removed: carl@nuvoloso.com; disabled: tad@nuvoloso.com]", msg)

	// multiple roles, more cases
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mds := mock.NewMockDataStore(mockCtrl)
	oR := mock.NewMockRoleOps(mockCtrl)
	oR.EXPECT().List(role.RoleListParams{}).Return(testRoles, nil)
	mds.EXPECT().OpsRole().Return(oR).MinTimes(1)
	hc.DS = mds
	uObj.AccountRoles = []models.ObjIDMutable{"role-a", "role-u"}
	uObj.UserRoles["user-1"] = models.AuthRole{RoleID: "role-u"}
	oObj.UserRoles["user-2"] = models.AuthRole{RoleID: "role-u"}
	msg = hc.accountUserRolesAuditMsg(oObj, uObj, userCache)
	assert.Equal("userRoles[added: bender@nuvoloso.com, stephen@nuvoloso.com; removed: carl@nuvoloso.com(A); disabled: tad@nuvoloso.com(A)]", msg)
	tl.Flush()

	// multiple roles, list warning, remaining cases
	oObj.UserRoles["user-1"] = models.AuthRole{RoleID: "role-a", Disabled: true}
	oObj.UserRoles["user-2"] = models.AuthRole{RoleID: "role-u", Disabled: true}
	uObj.UserRoles["user-2"] = models.AuthRole{RoleID: "role-u"}
	oR.EXPECT().List(role.RoleListParams{}).Return(nil, centrald.ErrorDbError)
	msg = hc.accountUserRolesAuditMsg(oObj, uObj, userCache)
	assert.Equal("userRoles[added: bender@nuvoloso.com, stephen@nuvoloso.com; removed: carl@nuvoloso.com; enabled: tad@nuvoloso.com]", msg)
	assert.Equal(1, tl.CountPattern("WARNING.*roles"))
}

func TestAccountSecretValidateAndMatch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()

	for _, scope := range []string{
		common.AccountSecretScopeGlobal,
		common.AccountSecretScopeCspDomain,
		common.AccountSecretScopeCluster,
	} {
		assert.True(hc.validateAccountSecretScope(scope), "validate scope %s", scope)
	}
	assert.False(hc.validateAccountSecretScope("foo"))
	assert.False(hc.validateAccountSecretScope(""))

	aObj := &models.Account{}
	secret := "aSecret"
	clID := "clusterID"
	domID := "domID"

	svm := accountSecretMapValuesByScope(clID, domID)
	svC := svm[common.AccountSecretScopeCluster]
	svD := svm[common.AccountSecretScopeCspDomain]
	svG := svm[common.AccountSecretScopeGlobal]
	assert.Equal("clusterID,domID", svC)
	assert.Equal(",domID", svD)
	assert.Equal(",", svG)

	var s1, s2 string
	s1, s2 = accountSecretMapValueParse(svC)
	assert.Equal(clID, s1)
	assert.Equal(domID, s2)
	s1, s2 = accountSecretMapValueParse(svD)
	assert.Equal("", s1)
	assert.Equal(domID, s2)
	s1, s2 = accountSecretMapValueParse(svG)
	assert.Equal("", s1)
	assert.Equal("", s2)
	s1, s2 = accountSecretMapValueParse("")
	assert.Equal("", s1)
	assert.Equal("", s2)
	s1, s2 = accountSecretMapValueParse("foo")
	assert.Equal("", s1)
	assert.Equal("", s2)
	s1, s2 = accountSecretMapValueParse("a,b,c")
	assert.Equal("a", s1)
	assert.Equal("b,c", s2)

	tl.Logger().Info("case: nil Secrets")
	assert.Nil(aObj.Secrets)
	assert.False(hc.accountSecretMatch(aObj, secret, clID, domID))

	tl.Logger().Info("case: empty secrets")
	aObj.Secrets = map[string]string{}
	assert.False(hc.accountSecretMatch(aObj, secret, clID, domID))

	tl.Logger().Info("case: secret not found")
	aObj.Secrets = map[string]string{"foo": "bar"}
	assert.False(hc.accountSecretMatch(aObj, secret, clID, domID))

	tl.Logger().Info("case: secret present scope mismatch")
	aObj.Secrets = map[string]string{secret: "wrong scope"}
	assert.False(hc.accountSecretMatch(aObj, secret, clID, domID))

	tl.Logger().Info("case: secret present cluster scope match")
	aObj.Secrets = map[string]string{secret: svC}
	assert.True(hc.accountSecretMatch(aObj, secret, clID, domID))

	tl.Logger().Info("case: secret present domain scope match")
	aObj.Secrets = map[string]string{secret: svD}
	assert.True(hc.accountSecretMatch(aObj, secret, clID, domID))

	tl.Logger().Info("case: secret present global scope match")
	aObj.Secrets = map[string]string{secret: svG}
	assert.True(hc.accountSecretMatch(aObj, secret, clID, domID))
}

func TestAccountCreate(t *testing.T) {
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
	params := ops.AccountCreateParams{
		HTTPRequest: requestWithAuthContext(ai),
		Payload: &models.Account{
			AccountMutable: models.AccountMutable{
				Name: "account1",
			},
		},
	}
	params.Payload.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{
		RetentionDurationSeconds: swag.Int32(99),
		Inherited:                false,
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{
				ID:      models.ObjID("objectID"),
				Version: 1,
			},
		},
		AccountMutable: models.AccountMutable{
			Name: params.Payload.Name,
			SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
				RetentionDurationSeconds: swag.Int32(99),
				Inherited:                false,
			},
			VsrManagementPolicy: &models.VsrManagementPolicy{
				RetentionDurationSeconds: swag.Int32(100),
				NoDelete:                 false,
				Inherited:                false,
			},
		},
	}
	userObj := &models.User{}
	userObj.AuthIdentifier = "bender@nuvoloso.com"

	sysObj := &models.System{
		SystemAllOf0: models.SystemAllOf0{
			Meta: &models.ObjMeta{
				ID:      "sys_id1",
				Version: 1,
			},
		},
		SystemMutable: models.SystemMutable{
			SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
				RetentionDurationSeconds: swag.Int32(99),
			},
			SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
				CspDomainID:        "domID",
				ProtectionDomainID: "pdID",
			},
			VsrManagementPolicy: &models.VsrManagementPolicy{
				RetentionDurationSeconds: swag.Int32(100),
				NoDelete:                 false,
			},
		},
	}

	// success, no user roles
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Create(ctx, params.Payload).Return(obj, nil)
	oR := mock.NewMockRoleOps(mockCtrl)
	oR.EXPECT().List(role.RoleListParams{}).Return(testRoles, nil)
	oSys := mock.NewMockSystemOps(mockCtrl)
	oSys.EXPECT().Fetch().Return(sysObj, nil).MinTimes(1)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	mds.EXPECT().OpsRole().Return(oR)
	mds.EXPECT().OpsSystem().Return(oSys).MinTimes(1)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.accountCreate(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.AccountCreateCreated)
	assert.True(ok)
	assert.Equal([]models.ObjIDMutable{"role-t"}, params.Payload.AccountRoles)
	assert.Len(evM.InSSProps, 1)
	assert.EqualValues("objectID", evM.InSSProps["meta.id"])
	assert.Equal(obj, evM.InACScope)
	exp := &fal.Args{AI: ai, Action: centrald.AccountCreateAction, ObjID: "objectID", Name: params.Payload.Name, Message: "Created"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	tl.Flush()

	// accountApplyInheritedProperties fails (success with warning)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: accountApplyInheritedProperties fails")
	fa.Posts = []*fal.Args{}
	params.Payload.AccountRoles = nil
	params.Payload.UserRoles = map[string]models.AuthRole{"user-1": models.AuthRole{RoleID: "role-t"}}
	obj.SnapshotManagementPolicy.Inherited = true
	obj.UserRoles = params.Payload.UserRoles
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Create(ctx, params.Payload).Return(obj, nil)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	oR = mock.NewMockRoleOps(mockCtrl)
	oR.EXPECT().List(role.RoleListParams{}).Return(testRoles, nil).MinTimes(1)
	mds.EXPECT().OpsRole().Return(oR).MinTimes(1)
	oU := mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, "user-1").Return(userObj, nil)
	mds.EXPECT().OpsUser().Return(oU)
	oSys = mock.NewMockSystemOps(mockCtrl)
	oSys.EXPECT().Fetch().Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsSystem().Return(oSys)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountCreate(params) })
	assert.NotNil(ret)
	_, ok = ret.(*ops.AccountCreateCreated)
	assert.True(ok)
	assert.Equal([]models.ObjIDMutable{"role-t"}, params.Payload.AccountRoles)
	assert.Len(evM.InSSProps, 1)
	assert.EqualValues("objectID", evM.InSSProps["meta.id"])
	assert.Equal(obj, evM.InACScope)
	exp.Message = "Created with userRoles[bender@nuvoloso.com]"
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	assert.Equal(1, tl.CountPattern("WARNING.*inherited property"))
	tl.Flush()

	// Create failed, cover TenantAccountID exists case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: Create fails")
	params.Payload.TenantAccountID = "tid1"
	params.Payload.AccountRoles = nil
	params.Payload.UserRoles = nil
	obj.UserRoles = nil
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "tid1").Return(&models.Account{}, nil)
	oA.EXPECT().Create(ctx, params.Payload).Return(nil, centrald.ErrorExists)
	oR = mock.NewMockRoleOps(mockCtrl)
	oR.EXPECT().List(role.RoleListParams{}).Return(testRoles, nil).MinTimes(1)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	mds.EXPECT().OpsRole().Return(oR).MinTimes(1)
	hc.DS = mds
	cntLock := hc.cntLock
	cntUnlock := hc.cntUnlock
	assert.NotPanics(func() { ret = hc.accountCreate(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.AccountCreateDefault)
	assert.Equal([]models.ObjIDMutable{"role-a", "role-u"}, params.Payload.AccountRoles)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorExists.M, *mE.Payload.Message)
	tl.Flush()

	t.Log("case: auditLog not ready")
	*fa = fal.AuditLog{}
	fa.ReadyRet = centrald.ErrorDbError
	assert.NotPanics(func() { ret = hc.accountCreate(params) })
	mE, ok = ret.(*ops.AccountCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.accountCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.AccountCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	// tenant account fetch fails
	mockCtrl.Finish()
	t.Log("case: tenant account fetch fails")
	mockCtrl = gomock.NewController(t)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "tid1").Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.accountCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.AccountCreateDefault)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Empty(fa.Posts)
	tl.Flush()

	// tenant account does not exist
	mockCtrl.Finish()
	t.Log("case: tenant account does not exist")
	mockCtrl = gomock.NewController(t)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "tid1").Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.accountCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.AccountCreateDefault)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
	tl.Flush()

	// tenant accountid is not a tenant account
	mockCtrl.Finish()
	t.Log("case: tenant accountid is not a tenant account")
	mockCtrl = gomock.NewController(t)
	retA := &models.Account{}
	retA.TenantAccountID = "tid2"
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, "tid1").Return(retA, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.accountCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.AccountCreateDefault)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("invalid.*tenantAccountId", *mE.Payload.Message)
	tl.Flush()

	// not authorized tenant admin
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized tenant admin")
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	assert.NotPanics(func() { ret = hc.accountCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.AccountCreateDefault)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	exp = &fal.Args{AI: ai, Action: centrald.AccountCreateAction, Name: params.Payload.Name, Err: true, Message: "Create unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	ai.RoleObj = nil
	tl.Flush()

	// not authorized system admin
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized system admin")
	fa.Posts = []*fal.Args{}
	params.Payload.TenantAccountID = ""
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	assert.NotPanics(func() { ret = hc.accountCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.AccountCreateDefault)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	exp = &fal.Args{AI: ai, Action: centrald.AccountCreateAction, Name: params.Payload.Name, Err: true, Message: "Create tenant unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	ai.RoleObj = nil
	tl.Flush()

	// accountRoles not empty
	t.Log("case: accountRoles not empty")
	params.Payload.TenantAccountID = ""
	ret = hc.accountCreate(params)
	assert.NotNil(ret)
	mE, ok = ret.(*ops.AccountCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
	assert.Regexp("accountRoles should be empty", *mE.Payload.Message)
	tl.Flush()

	// SnapshotCatalogPolicy not empty
	mockCtrl.Finish()
	t.Log("case: SnapshotCatalogPolicy not empty")
	mockCtrl = gomock.NewController(t)
	params.Payload.AccountRoles = make([]models.ObjIDMutable, 0)
	params.Payload.SnapshotCatalogPolicy = &models.SnapshotCatalogPolicy{
		Inherited:          false,
		CspDomainID:        "someDomID",
		ProtectionDomainID: "somePdID",
	}
	oR = mock.NewMockRoleOps(mockCtrl)
	oR.EXPECT().List(role.RoleListParams{}).Return(testRoles, nil).MinTimes(1)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsRole().Return(oR).MinTimes(1)
	hc.DS = mds
	ret = hc.accountCreate(params)
	assert.NotNil(ret)
	mE, ok = ret.(*ops.AccountCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
	assert.Regexp("snapshotCatalogPolicy should be empty", *mE.Payload.Message)
	tl.Flush()

	// role list fails
	mockCtrl.Finish()
	t.Log("case: role list fails")
	params.Payload.AccountRoles = make([]models.ObjIDMutable, 0)
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oR = mock.NewMockRoleOps(mockCtrl)
	oR.EXPECT().List(role.RoleListParams{}).Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsRole().Return(oR)
	hc.DS = mds
	ret = hc.accountCreate(params)
	assert.NotNil(ret)
	mE, ok = ret.(*ops.AccountCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	tl.Flush()

	// validateUserRoles fails (fully tested in separate UT)
	mockCtrl.Finish()
	t.Log("case: validateUserRoles fails")
	params.Payload.AccountRoles = nil
	params.Payload.TenantAccountID = ""
	params.Payload.UserRoles = map[string]models.AuthRole{"user-1": models.AuthRole{}}
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oR = mock.NewMockRoleOps(mockCtrl)
	oR.EXPECT().List(role.RoleListParams{}).Return(testRoles, nil)
	mds.EXPECT().OpsRole().Return(oR)
	oU = mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, "user-1").Return(nil, centrald.ErrorNotFound)
	mds.EXPECT().OpsUser().Return(oU)
	hc.DS = mds
	ret = hc.accountCreate(params)
	assert.NotNil(ret)
	mE, ok = ret.(*ops.AccountCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
	assert.Regexp("user-1$", *mE.Payload.Message)
	tl.Flush()

	// no name
	params.Payload.Name = ""
	t.Log("case: no name")
	assert.NotPanics(func() { ret = hc.accountCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.AccountCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
	assert.Regexp("name$", *mE.Payload.Message)
	tl.Flush()

	// name with slash
	params.Payload.Name = "face/off"
	t.Log("case: name with slash")
	assert.NotPanics(func() { ret = hc.accountCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.AccountCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
	assert.Regexp("name$", *mE.Payload.Message)
	tl.Flush()

	// same as System account name
	params.Payload.Name = centrald.SystemAccount
	t.Log("case: System account name")
	assert.NotPanics(func() { ret = hc.accountCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.AccountCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Regexp("^"+centrald.ErrorMissing.M, *mE.Payload.Message)
	assert.Regexp(": reserved name:", *mE.Payload.Message)
	tl.Flush()

	// no payload
	params.Payload = nil
	t.Log("case: nil payload")
	assert.NotPanics(func() { ret = hc.accountCreate(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.AccountCreateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorMissing.M, *mE.Payload.Message)
}

func TestAccountDelete(t *testing.T) {
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
	params := ops.AccountDeleteParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("objectID"),
			},
		},
		AccountMutable: models.AccountMutable{
			Name: "account",
		},
	}
	vsParams := volume_series.VolumeSeriesListParams{AccountID: &params.ID}
	vrlParams := volume_series_request.VolumeSeriesRequestListParams{AccountID: &params.ID, IsTerminated: swag.Bool(false)}
	cspParams := csp_domain.CspDomainListParams{AccountID: &params.ID}
	aParams := ops.AccountListParams{TenantAccountID: &params.ID}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oVS := mock.NewMockVolumeSeriesOps(mockCtrl)
	oVS.EXPECT().Count(ctx, vsParams, uint(1)).Return(0, nil)
	oCSP := mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Count(ctx, cspParams, uint(1)).Return(0, nil)
	oSP := mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().RemoveAccount(ctx, params.ID).Return(nil)
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Count(ctx, aParams, uint(1)).Return(0, nil)
	oA.EXPECT().Delete(ctx, params.ID).Return(nil)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oVR := mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Count(ctx, vrlParams, uint(1)).Return(0, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	mds.EXPECT().OpsCspDomain().Return(oCSP)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	t.Log("case: success")
	hc.DS = mds
	var ret middleware.Responder
	cntLock := hc.cntLock
	cntUnlock := hc.cntUnlock
	assert.NotPanics(func() { ret = hc.accountDelete(params) })
	assert.NotNil(ret)
	_, ok := ret.(*ops.AccountDeleteNoContent)
	assert.True(ok)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Empty(evM.InSSProps)
	assert.Equal(obj, evM.InACScope)
	exp := &fal.Args{AI: ai, Action: centrald.AccountDeleteAction, ObjID: models.ObjID(params.ID), Name: obj.Name, Message: "Deleted"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	tl.Flush()

	// system account
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: delete System account failure")
	obj.Name = centrald.SystemAccount
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.accountDelete(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.AccountDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Regexp(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	assert.Equal(cntLock, hc.cntLock)
	assert.Equal(cntUnlock, hc.cntUnlock)
	obj.Name = "account"
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: auditLog not ready")
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsAccount().Return(oA)
	fa.ReadyRet = centrald.ErrorDbError
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountDelete(params) })
	mE, ok = ret.(*ops.AccountDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.accountDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.AccountDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	// not authorized tenant admin
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized tenant admin")
	fa.Posts = []*fal.Args{}
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	obj.TenantAccountID = "tenantID"
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.accountDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.AccountDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Regexp(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	exp = &fal.Args{AI: ai, Action: centrald.AccountDeleteAction, ObjID: models.ObjID(params.ID), Name: obj.Name, Err: true, Message: "Delete unauthorized"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	ai.RoleObj = nil
	obj.TenantAccountID = ""
	tl.Flush()

	// not authorized system admin
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not authorized system admin")
	fa.Posts = []*fal.Args{}
	ai.AccountID = "otherID"
	ai.RoleObj = &models.Role{}
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.accountDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.AccountDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Regexp(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	ai.RoleObj = nil
	tl.Flush()

	// cascading validation failure cases
	tObj := []string{"volume series", "volume series request", "CSP domain", "account"}
	for tc := 0; tc < len(tObj)*2; tc++ {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		t.Log("case: " + tObj[tc/2] + []string{" count fails", " exists"}[tc%2])
		mds = mock.NewMockDataStore(mockCtrl)
		oA = mock.NewMockAccountOps(mockCtrl)
		oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
		mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
		count, err := tc%2, []error{centrald.ErrorDbError, nil}[tc%2]
		switch tc / 2 {
		case 3:
			oA.EXPECT().Count(ctx, aParams, uint(1)).Return(count, err)
			count, err = 0, nil
			fallthrough
		case 2:
			oCSP = mock.NewMockCspDomainOps(mockCtrl)
			oCSP.EXPECT().Count(ctx, cspParams, uint(1)).Return(count, err)
			mds.EXPECT().OpsCspDomain().Return(oCSP)
			count, err = 0, nil
			fallthrough
		case 1:
			oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
			oVR.EXPECT().Count(ctx, vrlParams, uint(1)).Return(count, err)
			mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
			count, err = 0, nil
			fallthrough
		case 0:
			oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
			oVS.EXPECT().Count(ctx, vsParams, uint(1)).Return(count, err)
			mds.EXPECT().OpsVolumeSeries().Return(oVS)
		default:
			assert.True(false)
		}
		hc.DS = mds
		cntLock = hc.cntLock
		cntUnlock = hc.cntUnlock
		assert.NotPanics(func() { ret = hc.accountDelete(params) })
		assert.NotNil(ret)
		mE, ok = ret.(*ops.AccountDeleteDefault)
		if assert.True(ok) {
			if tc%2 == 0 {
				assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
				assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
			} else {
				assert.Equal(centrald.ErrorExists.C, int(mE.Payload.Code))
				assert.Regexp(tObj[tc/2]+".* with the.* account", *mE.Payload.Message)
			}
		}
		assert.Equal(cntLock+1, hc.cntLock)
		assert.Equal(cntUnlock+1, hc.cntUnlock)
		tl.Flush()
	}

	// delete failure case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: delete failure")
	ai.AccountID = "tenantID" // cover tenant admin OK case
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.ManageNormalAccountsCap: true}
	obj.TenantAccountID = "tenantID"
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVS.EXPECT().Count(ctx, vsParams, uint(1)).Return(0, nil)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Count(ctx, cspParams, uint(1)).Return(0, nil)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().RemoveAccount(ctx, params.ID).Return(nil)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Count(ctx, aParams, uint(1)).Return(0, nil)
	oA.EXPECT().Delete(ctx, params.ID).Return(centrald.ErrorDbError)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Count(ctx, vrlParams, uint(1)).Return(0, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	mds.EXPECT().OpsCspDomain().Return(oCSP)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.accountDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.AccountDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
	ai.RoleObj = nil
	tl.Flush()

	// fetch failure case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: fetch failure")
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.accountDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.AccountDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
	assert.Equal(cntLock, hc.cntLock)
	assert.Equal(cntUnlock, hc.cntUnlock)
	tl.Flush()

	// service plan update failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: service plan update failure")
	oVS = mock.NewMockVolumeSeriesOps(mockCtrl)
	oVS.EXPECT().Count(ctx, vsParams, uint(1)).Return(0, nil)
	oCSP = mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Count(ctx, cspParams, uint(1)).Return(0, nil)
	oSP = mock.NewMockServicePlanOps(mockCtrl)
	oSP.EXPECT().RemoveAccount(ctx, params.ID).Return(centrald.ErrorDbError)
	oVR = mock.NewMockVolumeSeriesRequestOps(mockCtrl)
	oVR.EXPECT().Count(ctx, vrlParams, uint(1)).Return(0, nil)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Count(ctx, aParams, uint(1)).Return(0, nil)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsServicePlan().Return(oSP)
	mds.EXPECT().OpsCspDomain().Return(oCSP)
	mds.EXPECT().OpsVolumeSeries().Return(oVS)
	mds.EXPECT().OpsVolumeSeriesRequest().Return(oVR)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	cntLock = hc.cntLock
	cntUnlock = hc.cntUnlock
	assert.NotPanics(func() { ret = hc.accountDelete(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.AccountDeleteDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	assert.Equal(cntLock+1, hc.cntLock)
	assert.Equal(cntUnlock+1, hc.cntUnlock)
}

func TestAccountFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.AccountFetchParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          "objectID",
	}
	ctx := params.HTTPRequest.Context()
	obj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("objectID"),
			},
		},
		AccountMutable: models.AccountMutable{
			Name: "account",
			SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
				RetentionDurationSeconds: swag.Int32(99),
				Inherited:                false,
			},
			SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
				CspDomainID: "domID",
				Inherited:   false,
			},
			VsrManagementPolicy: &models.VsrManagementPolicy{
				RetentionDurationSeconds: swag.Int32(100),
				NoDelete:                 false,
				Inherited:                false,
			},
			Secrets: map[string]string{},
			UserRoles: map[string]models.AuthRole{
				"uid1": {Disabled: false, RoleID: "rid1"},
				"uid2": {Disabled: true, RoleID: "rid1"},
			},
		},
	}

	// success (internal)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	t.Log("case: AccountFetch success (internal)")
	assert.NoError(ai.InternalOK())
	hc.DS = mds
	aObj, err := hc.intAccountFetch(ctx, ai, params.ID)
	assert.NoError(err)
	assert.NotNil(aObj)
	assert.Equal(obj, aObj)
	assert.NotNil(obj.Secrets) // not cleared
	tl.Flush()

	// success (not-internal)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	var ret middleware.Responder
	var extObj *models.Account
	testutils.Clone(obj, &extObj)
	extObj.Secrets = nil
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.AccountFetchAllRolesCap: true}
	ai.AccountID = string(obj.Meta.ID)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(extObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	t.Log("case: AccountFetch success (not internal)")
	assert.Error(ai.InternalOK())
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountFetch(params) })
	assert.NotNil(ret)
	mR, ok := ret.(*ops.AccountFetchOK)
	assert.True(ok)
	assert.Equal(extObj, mR.Payload)
	tl.Flush()
	ai = &auth.Info{}

	// fetch failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	t.Log("case: AccountFetch fetch failure")
	tl.Flush()
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountFetch(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.AccountFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
	tl.Flush()

	// accountApplyInheritedProperties fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	obj.SnapshotManagementPolicy.Inherited = true
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	oSys := mock.NewMockSystemOps(mockCtrl)
	oSys.EXPECT().Fetch().Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsSystem().Return(oSys)
	t.Log("case: accountApplyInheritedProperties fails")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.AccountFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mE.Payload.Message)
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.accountFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.AccountFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mE.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	t.Log("case: unauthorized")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	ai.AccountID = "aid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.AccountFetchOwnRoleCap: true}
	assert.NotPanics(func() { ret = hc.accountFetch(params) })
	assert.NotNil(ret)
	mE, ok = ret.(*ops.AccountFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mE.Payload.Message)
}

func TestAccountList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	ai := &auth.Info{}
	params := ops.AccountListParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	ctx := params.HTTPRequest.Context()
	objects := []*models.Account{
		&models.Account{
			AccountAllOf0: models.AccountAllOf0{
				Meta: &models.ObjMeta{
					ID: models.ObjID("objectID"),
				},
			},
			AccountMutable: models.AccountMutable{
				Name: "account",
				SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
					RetentionDurationSeconds: swag.Int32(99),
					Inherited:                false,
				},
				SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
					CspDomainID: "domID",
					Inherited:   false,
				},
				VsrManagementPolicy: &models.VsrManagementPolicy{
					RetentionDurationSeconds: swag.Int32(100),
					NoDelete:                 false,
					Inherited:                false,
				},
				Secrets: map[string]string{},
			},
		},
		&models.Account{
			AccountAllOf0: models.AccountAllOf0{
				Meta: &models.ObjMeta{
					ID: models.ObjID("objectID2"),
				},
			},
			AccountMutable: models.AccountMutable{
				Name: "account",
				SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
					RetentionDurationSeconds: swag.Int32(99),
					Inherited:                false,
				},
				SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
					CspDomainID: "domID",
					Inherited:   false,
				},
				VsrManagementPolicy: &models.VsrManagementPolicy{
					RetentionDurationSeconds: swag.Int32(100),
					NoDelete:                 false,
					Inherited:                false,
				},
				Secrets: map[string]string{},
			},
		},
	}
	retList := []*models.Account{}
	for _, o := range objects {
		var n *models.Account
		testutils.Clone(o, &n)
		n.Secrets = nil
		retList = append(retList, n)
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().List(ctx, params).Return(objects, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	t.Log("case: AccountList success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.accountList(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.AccountListOK)
	assert.True(ok)
	assert.Equal(retList, mO.Payload)
	tl.Flush()

	// success with ClusterSecret lookup in internal query
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: AccountList cluster secret lookup")
	var o2 []*models.Account
	testutils.Clone(objects, &o2)
	assert.Len(o2, 2)
	o2[1].Secrets = map[string]string{"secret": "clusterID,cspDomainID"}
	assert.False(hc.accountSecretMatch(o2[0], "secret", "clusterID", "cspDomainID")) // matcher fully
	assert.True(hc.accountSecretMatch(o2[1], "secret", "clusterID", "cspDomainID"))  // tested elsewhere
	params.AccountSecret = swag.String("secret")
	params.ClusterID = swag.String("clusterID")
	params.CspDomainID = swag.String("cspDomainID")
	assert.NoError(ai.InternalOK())
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().List(ctx, params).Return(o2, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.AccountListOK)
	assert.True(ok)
	assert.Len(o2, 2)
	assert.Len(mO.Payload, 1)
	assert.EqualValues([]*models.Account{o2[1]}, mO.Payload)
	assert.Nil(mO.Payload[0].Secrets)
	tl.Flush()

	// failure due to use of ClusterSecret in non-internal query
	params.AccountSecret = swag.String("secret")
	params.ClusterID = swag.String("clusterID")
	params.CspDomainID = swag.String("cspDomainID")
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.AccountFetchAllRolesCap: true}
	assert.Error(ai.InternalOK())
	t.Log("case: AccountList fail (use of internal query param)")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountList(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.AccountListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)
	tl.Flush()

	// failure due to invalid combinations of cluster secret internal query params
	ai.RoleObj = nil
	assert.NoError(ai.InternalOK())
	invalidCombos := []struct{ ClusterID, CspDomainID *string }{
		{nil, nil},
		{swag.String("clusterId"), nil},
		{nil, swag.String("cspDomainID")},
		{swag.String(""), swag.String("")},
		{swag.String("clusterId"), swag.String("")},
		{swag.String(""), swag.String("cspDomainID")},
	}
	for i, tc := range invalidCombos {
		params.AccountSecret = swag.String("secret")
		params.ClusterID = tc.ClusterID
		params.CspDomainID = tc.CspDomainID
		objects[0].Secrets = map[string]string{}
		objects[1].Secrets = map[string]string{}
		t.Logf("case: fail AccountList (internal, combo[%d, %v])", i, tc)
		assert.NotPanics(func() { ret = hc.accountList(params) })
		assert.NotNil(ret)
		mD, ok = ret.(*ops.AccountListDefault)
		assert.True(ok)
		assert.Equal(centrald.ErrorMissing.C, int(mD.Payload.Code))
		assert.Regexp(centrald.ErrorMissing.M, *mD.Payload.Message)
		tl.Flush()
	}
	params.AccountSecret = nil
	params.ClusterID = nil
	params.CspDomainID = nil

	// list fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().List(ctx, params).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	t.Log("case: AccountList List failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AccountListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()

	// accountApplyInheritedProperties fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().List(ctx, params).Return(objects, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	oSys := mock.NewMockSystemOps(mockCtrl)
	oSys.EXPECT().Fetch().Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsSystem().Return(oSys)
	objects[0].SnapshotManagementPolicy.Inherited = true
	t.Log("case: accountApplyInheritedProperties fails")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.AccountListOK)
	assert.True(ok)
	assert.Len(mO.Payload, 1) // one obj failed to be updated and is not listed
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.accountList(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AccountListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: unauthorized filtered")
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().List(ctx, params).Return(objects, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	ai.AccountID = "objectID2"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.AccountFetchOwnRoleCap: true}
	assert.NotPanics(func() { ret = hc.accountList(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.AccountListOK)
	assert.True(ok)
	if assert.Len(mO.Payload, 1) {
		assert.EqualValues("objectID2", mO.Payload[0].Meta.ID)
	}
}

func TestAccountUpdate(t *testing.T) {
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
	assert.NotPanics(func() { nMap = hc.accountMutableNameMap() })

	// parse params
	objM := &models.AccountMutable{
		Name:     "account",
		Disabled: true,
	}
	ai := &auth.Info{}
	aID := "accountID"
	params := ops.AccountUpdateParams{
		HTTPRequest: requestWithAuthContext(ai),
		ID:          aID,
		Version:     swag.Int32(8),
		Remove:      []string{},
		Append:      []string{},
		Set:         []string{nMap.jName("Name")},
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
	rObj := &models.Role{
		RoleMutable: models.RoleMutable{
			Capabilities: map[string]bool{centrald.AccountUpdateCap: true},
		},
	}
	userObj := &models.User{}
	userObj.AuthIdentifier = "bender@nuvoloso.com"
	user2Obj := &models.User{}
	user2Obj.AuthIdentifier = "tad@nuvoloso.com"

	pdID := "pdID"
	cspID := "cspID"
	obj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{
				ID:      models.ObjID(aID),
				Version: 8,
			},
		},
		AccountMutable: models.AccountMutable{
			Name: "bender",
			SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
				RetentionDurationSeconds: swag.Int32(99),
				Inherited:                false,
			},
			SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
				CspDomainID:        "domID",
				ProtectionDomainID: "pdID",
				Inherited:          false,
			},
			VsrManagementPolicy: &models.VsrManagementPolicy{
				RetentionDurationSeconds: swag.Int32(100),
				Inherited:                false,
			},
			Secrets: map[string]string{
				"foo": "bar",
			},
			ProtectionDomains: map[string]models.ObjIDMutable{
				common.ProtectionStoreDefaultKey: models.ObjIDMutable(pdID),
				cspID:                            models.ObjIDMutable(pdID),
			},
		},
	}
	var retObj *models.Account
	now := time.Now()
	sysObj := &models.System{
		SystemAllOf0: models.SystemAllOf0{
			Meta: &models.ObjMeta{
				ID:           "sys_id1",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
		},
		SystemMutable: models.SystemMutable{
			SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
				RetentionDurationSeconds: swag.Int32(99),
			},
			SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
				CspDomainID:        "domID",
				ProtectionDomainID: "pdID",
				Inherited:          false,
			},
			VsrManagementPolicy: &models.VsrManagementPolicy{
				RetentionDurationSeconds: swag.Int32(100),
				NoDelete:                 false,
			},
		},
	}
	pdObj := &models.ProtectionDomain{}
	pdObj.Meta = &models.ObjMeta{ID: models.ObjID(pdID)}
	pdObj.AccountID = models.ObjIDMutable(aID)
	pdObj.Name = "PD"

	cspObj := &models.CSPDomain{}
	cspObj.Meta = &models.ObjMeta{ID: models.ObjID(cspID)}
	cspObj.AccountID = models.ObjIDMutable(aID)
	cspObj.AuthorizedAccounts = []models.ObjIDMutable{pdObj.AccountID}
	cspObj.ClusterUsagePolicy = &models.ClusterUsagePolicy{}
	cspObj.Name = "CSP"

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	// success
	testutils.Clone(obj, &retObj)
	assert.NoError(ai.InternalOK())
	oA := mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oA.EXPECT().Update(ctx, uaM, params.Payload).Return(retObj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	t.Log("case: success (internal)")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.accountUpdate(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.AccountUpdateOK)
	assert.True(ok)
	assert.Equal(retObj, mO.Payload)
	assert.Empty(evM.InSSProps)
	assert.Equal(retObj, evM.InACScope)
	exp := &fal.Args{AI: ai, Action: centrald.AccountUpdateAction, ObjID: models.ObjID(params.ID), Name: retObj.Name, Message: "Updated name"}
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	assert.Nil(retObj.Secrets)
	tl.Flush()

	// success (not internal)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: success (not internal)")
	fa.Posts = []*fal.Args{}
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.ManageSpecialAccountsCap: true}
	ai.AccountID = string(obj.Meta.ID)
	assert.Error(ai.InternalOK())
	testutils.Clone(obj, &retObj)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oA.EXPECT().Update(ctx, uaM, params.Payload).Return(retObj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountUpdate(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.AccountUpdateOK)
	assert.True(ok)
	assert.Equal(retObj, mO.Payload)
	assert.Nil(retObj.Secrets)
	assert.Empty(evM.InSSProps)
	assert.Equal(retObj, evM.InACScope)
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	tl.Flush()
	ai.RoleObj = nil

	// Update failed, cover remove userRoles not validated case, Messages OK with internal auth
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: Update fails 1")
	assert.NoError(ai.InternalOK())
	params.Payload.UserRoles = map[string]models.AuthRole{
		"user-1": models.AuthRole{RoleID: "role-1"},
	}
	params.Set = []string{"name", "messages"}
	params.Remove = []string{"userRoles"}
	uP[centrald.UpdateSet] = params.Set
	uP[centrald.UpdateRemove] = params.Remove
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.NoError(err)
	assert.NotNil(ua)
	uaM2 := updateArgsMatcher(t, ua).Matcher()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oA.EXPECT().Update(ctx, uaM2, params.Payload).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountUpdate(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.AccountUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()

	// no changes requested
	params.Set = []string{}
	params.Remove = []string{}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	t.Log("case: no change")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AccountUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	tl.Flush()

	// empty name
	params.Set = []string{"name"}
	params.Payload.Name = ""
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsAccount().Return(oA)
	t.Log("case: empty name")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AccountUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	tl.Flush()

	// name with slash
	params.Set = []string{"name"}
	params.Payload.Name = "face/off"
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsAccount().Return(oA)
	t.Log("case: name with slash")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AccountUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: System account name")
	params.Set = []string{"name"}
	params.Payload.Name = centrald.SystemAccount
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AccountUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	tl.Flush()

	// internal only properties
	tcs := []string{"messages", "secrets", "protectionDomains"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		t.Log("case: external update to", tc)
		ai.UserID = "uid1"
		ai.RoleObj = &models.Role{}
		assert.False(ai.Internal())
		params.Set = []string{tc}
		params.Append = nil
		params.Remove = nil
		mds = mock.NewMockDataStore(mockCtrl)
		oA = mock.NewMockAccountOps(mockCtrl)
		oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
		mds.EXPECT().OpsAccount().Return(oA)
		hc.DS = mds
		assert.NotPanics(func() { ret = hc.accountUpdate(params) })
		assert.NotNil(ret)
		mD, ok = ret.(*ops.AccountUpdateDefault)
		assert.True(ok)
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
		assert.Regexp("^"+centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)
		tl.Flush()
	}

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: success with userRoles, Disabled")
	fa.Posts = []*fal.Args{}
	params.Set = []string{"disabled", "userRoles"}
	testutils.Clone(obj, &retObj)
	retObj.UserRoles = params.Payload.UserRoles
	obj.UserRoles = map[string]models.AuthRole{
		"uid1": {Disabled: false, RoleID: "rid1"},
	}
	ai.UserID = "uid1"
	ai.RoleObj = &models.Role{}
	uP[centrald.UpdateSet] = params.Set
	uP[centrald.UpdateRemove] = params.Remove
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.NoError(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).Matcher()
	obj.AccountRoles = []models.ObjIDMutable{"role-1"}
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oA.EXPECT().Update(ctx, uaM, params.Payload).Return(retObj, nil)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	oU := mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, "uid1").Return(user2Obj, nil)
	oU.EXPECT().Fetch(ctx, "user-1").Return(userObj, nil)
	mds.EXPECT().OpsUser().Return(oU).MinTimes(1)
	oR := mock.NewMockRoleOps(mockCtrl)
	oR.EXPECT().Fetch("rid1").Return(rObj, nil)
	mds.EXPECT().OpsRole().Return(oR)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountUpdate(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.AccountUpdateOK)
	assert.True(ok)
	assert.Equal(retObj, mO.Payload)
	exp.Message = "Updated disabled, userRoles[added: bender@nuvoloso.com; removed: tad@nuvoloso.com]"
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	ai.UserID = "anotherID"
	ai.RoleObj = nil
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: success with Enabled")
	fa.Posts = []*fal.Args{}
	params.Payload.Disabled = false
	params.Set = []string{"disabled", "snapshotManagementPolicy", "vsrManagementPolicy"}
	params.Payload.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{NoDelete: true}
	params.Payload.VsrManagementPolicy = &models.VsrManagementPolicy{NoDelete: true}
	obj.Disabled = true
	uP[centrald.UpdateSet] = params.Set
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.NoError(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).Matcher()
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oA.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountUpdate(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.AccountUpdateOK)
	assert.True(ok)
	assert.Equal(obj, mO.Payload)
	exp.Message = "Updated enabled"
	assert.Equal([]*fal.Args{exp}, fa.Posts)
	tl.Flush()

	mockCtrl.Finish()
	t.Log("case: failure, modify non-inherited policy")
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	obj.SnapshotManagementPolicy.Inherited = true
	obj.SnapshotCatalogPolicy.Inherited = true
	obj.VsrManagementPolicy.Inherited = true
	params.Append = []string{}
	params.Set = []string{"snapshotManagementPolicy", "vsrManagementPolicy", "snapshotCatalogPolicy"}
	params.Payload.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{NoDelete: true}
	params.Payload.SnapshotCatalogPolicy = &models.SnapshotCatalogPolicy{CspDomainID: "new_csp_id", ProtectionDomainID: "new_pd_id"}
	params.Payload.VsrManagementPolicy = &models.VsrManagementPolicy{NoDelete: true}
	uP[centrald.UpdateAppend] = params.Append
	uP[centrald.UpdateSet] = params.Set
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	oPD := mock.NewMockProtectionDomainOps(mockCtrl)
	oPD.EXPECT().Fetch(ctx, "new_pd_id").Return(pdObj, nil)
	mds.EXPECT().OpsProtectionDomain().Return(oPD).MinTimes(1)
	oCSP := mock.NewMockCspDomainOps(mockCtrl)
	oCSP.EXPECT().Fetch(ctx, "new_csp_id").Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsCspDomain().Return(oCSP).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AccountUpdateDefault)
	assert.True(ok)
	assert.Equal(obj, mO.Payload)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()

	// removal of the snapshotCatalogPolicy when there is no effective policy available
	mockCtrl.Finish()
	t.Log("case: failure, removal of the snapshotCatalogPolicy")
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	params.Set = []string{"snapshotCatalogPolicy"}
	params.Payload.SnapshotCatalogPolicy = nil
	uP[centrald.UpdateAppend] = params.Append
	uP[centrald.UpdateSet] = params.Set
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	oSys := mock.NewMockSystemOps(mockCtrl)
	oSys.EXPECT().Fetch().Return(nil, centrald.ErrorDbError).MinTimes(1)
	mds.EXPECT().OpsSystem().Return(oSys).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AccountUpdateDefault)
	assert.True(ok)
	assert.Equal(obj, mO.Payload)
	expectedErr := &centrald.Error{M: "removal of snapshotCatalogPolicy is not allowed if there is no effective policy inherited", C: 400}
	assert.Equal(expectedErr.C, int(mD.Payload.Code))
	assert.Regexp(expectedErr.M, *mD.Payload.Message)
	tl.Flush()

	mockCtrl.Finish()
	t.Log("case: success, setting inherited management policy flag overwrites all other possible options")
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	obj.SnapshotManagementPolicy.Inherited = false
	params.Append = []string{}
	params.Set = []string{"snapshotManagementPolicy", "vsrManagementPolicy"}
	params.Payload.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{
		NoDelete:  true,
		Inherited: true,
	}
	params.Payload.VsrManagementPolicy = &models.VsrManagementPolicy{
		NoDelete:  true,
		Inherited: true,
	}
	uP[centrald.UpdateAppend] = params.Append
	uP[centrald.UpdateSet] = params.Set
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	oA.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	oSys = mock.NewMockSystemOps(mockCtrl)
	oSys.EXPECT().Fetch().Return(sysObj, nil).MinTimes(1)
	mds.EXPECT().OpsSystem().Return(oSys).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountUpdate(params) })
	assert.NotNil(ret)
	mO, ok = ret.(*ops.AccountUpdateOK)
	assert.True(ok)
	assert.Equal(obj, mO.Payload)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	mockCtrl.Finish()
	t.Log("case: failure, attempt to modify inherited snapshot management policy with NoDelete false and no retention period specified")
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	obj.SnapshotManagementPolicy.Inherited = true
	params.Append = []string{}
	params.Set = []string{"snapshotManagementPolicy"}
	params.Payload.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{
		NoDelete:                 false,
		Inherited:                false,
		RetentionDurationSeconds: swag.Int32(0),
	}
	uP[centrald.UpdateAppend] = params.Append
	uP[centrald.UpdateSet] = params.Set
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).AddsVersion(3)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AccountUpdateDefault)
	assert.True(ok)
	expectedErr = &centrald.Error{M: "proper RetentionDurationSeconds value should be provided if 'NoDelete' flag is false", C: 400}
	assert.Equal(expectedErr.C, int(mD.Payload.Code))
	assert.Regexp(".*"+expectedErr.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	mockCtrl.Finish()
	t.Log("case: failure, attempt to modify inherited VSR management policy with NoDelete false and no retention period specified")
	mockCtrl = gomock.NewController(t)
	mds = mock.NewMockDataStore(mockCtrl)
	obj.VsrManagementPolicy.Inherited = true
	params.Append = []string{}
	params.Set = []string{"vsrManagementPolicy"}
	params.Payload.VsrManagementPolicy = &models.VsrManagementPolicy{
		NoDelete:                 false,
		Inherited:                false,
		RetentionDurationSeconds: swag.Int32(0),
	}
	uP[centrald.UpdateAppend] = params.Append
	uP[centrald.UpdateSet] = params.Set
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, params.ID, params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua).AddsVersion(3)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsAccount().Return(oA).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AccountUpdateDefault)
	assert.True(ok)
	expectedErr = &centrald.Error{M: "proper RetentionDurationSeconds value should be provided if 'NoDelete' flag is false", C: 400}
	assert.Equal(expectedErr.C, int(mD.Payload.Code))
	assert.Regexp(".*"+expectedErr.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	obj.VsrManagementPolicy.Inherited = false
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: cover userRoles fields, no version, validate fails")
	params.Set = []string{"userRoles.user-1"}
	params.Version = nil
	obj.AccountRoles = []models.ObjIDMutable{"role-a", "role-u"}
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsAccount().Return(oA)
	oU = mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, "uid1").Return(user2Obj, nil)
	oU.EXPECT().Fetch(ctx, "user-1").Return(userObj, nil)
	mds.EXPECT().OpsUser().Return(oU).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AccountUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: userRoles, original user fetch fails")
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsAccount().Return(oA)
	oU = mock.NewMockUserOps(mockCtrl)
	oU.EXPECT().Fetch(ctx, "uid1").Return(nil, centrald.ErrorDbError)
	mds.EXPECT().OpsUser().Return(oU)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AccountUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: userRoles, account fetch fails")
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(nil, centrald.ErrorNotFound)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AccountUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mD.Payload.Code))
	assert.Equal("object not found", *mD.Payload.Message)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: auditLog not ready")
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsAccount().Return(oA)
	fa.ReadyRet = centrald.ErrorDbError
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountUpdate(params) })
	mD, ok = ret.(*ops.AccountUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	fa.ReadyRet = nil
	tl.Flush()

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.accountUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AccountUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	ctx = params.HTTPRequest.Context()
	tl.Flush()

	oSet := params.Set
	for _, tc := range []string{"internal", "system admin", "tenant admin", "account admin disabled"} {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		mds = mock.NewMockDataStore(mockCtrl)
		switch tc {
		case "internal":
			params.Set = []string{"messages"}
			ai.RoleObj = &models.Role{}
		case "system admin":
			ai.AccountID = "otherID"
		case "tenant admin":
			ai.AccountID = "otherID"
			obj.TenantAccountID = "tenantID"
		case "account admin disabled":
			ai.AccountID = "otherID"
			obj.UserRoles = map[string]models.AuthRole{"otherID": {Disabled: true, RoleID: "rid1"}}
		}
		t.Log("case: auth wrong role:", tc)
		fa.Posts = []*fal.Args{}
		oA = mock.NewMockAccountOps(mockCtrl)
		oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
		mds.EXPECT().OpsAccount().Return(oA)
		hc.DS = mds
		assert.NotPanics(func() { ret = hc.accountUpdate(params) })
		assert.NotNil(ret)
		mD, ok = ret.(*ops.AccountUpdateDefault)
		assert.True(ok)
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
		assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)
		if tc != "internal" {
			exp = &fal.Args{AI: ai, Action: centrald.AccountUpdateAction, ObjID: models.ObjID(params.ID), Name: obj.Name, Err: true, Message: "Update unauthorized"}
			assert.Equal([]*fal.Args{exp}, fa.Posts)
		}
		params.Set = oSet
		tl.Flush()
	}
	ai.RoleObj = nil

	params.Version = swag.Int32(7)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: wrong version")
	mds = mock.NewMockDataStore(mockCtrl)
	oA = mock.NewMockAccountOps(mockCtrl)
	oA.EXPECT().Fetch(ctx, params.ID).Return(obj, nil)
	mds.EXPECT().OpsAccount().Return(oA)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AccountUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorIDVerNotFound.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorIDVerNotFound.M, *mD.Payload.Message)
	tl.Flush()

	// no payload
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: no payload")
	mds = mock.NewMockDataStore(mockCtrl)
	params.Set = []string{"messages"}
	params.Payload = nil
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.accountUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AccountUpdateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
		assert.Regexp(centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
		assert.Regexp("missing payload", *mD.Payload.Message)
	}
}

func TestAccountSecretDriver(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	fOps := &fakeOps{}
	hc.ops = fOps

	ctxKey := struct{}{}
	val := struct{ x int }{20}
	ctx := context.WithValue(context.Background(), ctxKey, &val)

	ai := &auth.Info{}
	ai.RoleObj = &models.Role{}
	aID := "accountID"
	obj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{
				ID:      models.ObjID(aID),
				Version: 8,
			},
		},
	}
	var err error
	hf := &fakeSecretDriverOps{}

	// fail in account fetch
	fOps.RetAccountFetchObj = nil
	fOps.RetAccountFetchErr = fmt.Errorf("fetch-error")
	hf.InUpdateSecretObj = nil
	err = hc.accountSecretDriver(ctx, ai, aID, hf)
	assert.Error(err)
	assert.Regexp("fetch-error", err)
	assert.Nil(hf.InUpdateSecretObj)

	// fail in view account
	fOps.RetAccountFetchObj = obj
	fOps.RetAccountFetchErr = nil
	hf.RetPreUpdateCheckErr = fmt.Errorf("view-error")
	assert.Nil(obj.Secrets)
	err = hc.accountSecretDriver(ctx, ai, aID, hf)
	assert.Error(err)
	assert.Regexp("view-error", err)
	assert.NotNil(hf.InPreUpdateCheckObj)
	assert.Equal(obj, hf.InPreUpdateCheckObj)
	assert.NotNil(obj.Secrets)
	cv := hf.InPreUpdateCheckCtx.Value(ctxKey)
	assert.Equal(&val, cv) // input context was passed on
	assert.Equal(ai, hf.InPreUpdateCheckAi)

	// handler returns no-update-needed
	hf.RetPreUpdateCheckErr = nil
	hf.InUpdateSecretObj = nil
	hf.RetUpdateSecretRc = false
	err = hc.accountSecretDriver(ctx, ai, aID, hf)
	assert.NoError(err)
	assert.Equal(obj, hf.InUpdateSecretObj)

	// handler returns update-needed, update fails
	hf.InUpdateSecretObj = nil
	hf.RetUpdateSecretRc = true
	fOps.RetAccountFetchObj = obj
	fOps.RetAccountFetchErr = nil
	fOps.CntAccountUpdate = 0
	fOps.InAccountUpdateParams = nil
	fOps.InAccountUpdateAi = nil
	fOps.InAccountUpdateObj = nil
	fOps.RetAccountUpdateObj = nil
	fOps.RetAccountUpdateErr = fmt.Errorf("update-error")
	err = hc.accountSecretDriver(ctx, ai, aID, hf)
	assert.Error(err)
	assert.Regexp("update-error", err)
	assert.Equal(obj, hf.InUpdateSecretObj)
	assert.Equal(1, fOps.CntAccountUpdate)
	assert.NotNil(fOps.InAccountUpdateParams)
	p := fOps.InAccountUpdateParams
	assert.Equal(aID, p.ID)
	assert.EqualValues(obj.Meta.Version, swag.Int32Value(p.Version))
	assert.Equal([]string{"secrets"}, p.Set)
	assert.Nil(p.Append)
	assert.Nil(p.Remove)
	assert.NotNil(p.Payload)
	assert.Equal(&models.AccountMutable{Secrets: hf.InUpdateSecretObj.Secrets}, p.Payload)
	cv = p.HTTPRequest.Context().Value(ctxKey)
	assert.Equal(&val, cv) // input context was passed on
	assert.NotNil(fOps.InAccountUpdateAi)
	assert.NotEqual(ai, fOps.InAccountUpdateAi)
	assert.Equal(&auth.Info{}, fOps.InAccountUpdateAi)
	assert.Error(ai.InternalOK())
	assert.NoError(fOps.InAccountUpdateAi.InternalOK())
	assert.Equal(hf.InUpdateSecretObj, fOps.InAccountUpdateObj)

	// handler returns update-needed, update ok
	hf.InUpdateSecretObj = nil
	hf.RetUpdateSecretRc = true
	fOps.RetAccountFetchObj = obj
	fOps.RetAccountFetchErr = nil
	fOps.CntAccountUpdate = 0
	fOps.InAccountUpdateParams = nil
	fOps.InAccountUpdateAi = nil
	fOps.InAccountUpdateObj = nil
	fOps.RetAccountUpdateObj = nil
	fOps.RetAccountUpdateErr = nil
	err = hc.accountSecretDriver(ctx, ai, aID, hf)
	assert.NoError(err)
	assert.Equal(obj, hf.InUpdateSecretObj)
	assert.Equal(1, fOps.CntAccountUpdate)
}

type fakeSecretDriverOps struct {
	InUpdateSecretObj    *models.Account
	RetUpdateSecretRc    bool
	InPreUpdateCheckCtx  context.Context
	InPreUpdateCheckAi   *auth.Info
	InPreUpdateCheckObj  *models.Account
	RetPreUpdateCheckErr error
}

func (op *fakeSecretDriverOps) PreUpdateCheck(ctx context.Context, ai *auth.Info, aObj *models.Account) error {
	op.InPreUpdateCheckCtx = ctx
	op.InPreUpdateCheckAi = ai
	op.InPreUpdateCheckObj = aObj
	return op.RetPreUpdateCheckErr
}
func (op *fakeSecretDriverOps) UpdateSecrets(aObj *models.Account) bool {
	op.InUpdateSecretObj = aObj
	return op.RetUpdateSecretRc
}
func (op *fakeSecretDriverOps) Object() interface{} {
	return op
}

func TestAccountSecretRetrieval(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	fOps := &fakeOps{}
	hc.ops = fOps

	ai := &auth.Info{}

	saveUUIDGen := uuidGenerator
	defer func() { uuidGenerator = saveUUIDGen }()
	fakeUUID := "fake-U-U-I-D"
	uuidGenerator = func() string { return fakeUUID }

	aObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{ID: "accountID"},
		},
	}
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{ID: "clusterID"},
		},
	}
	clObj.CspDomainID = "cspDomainID"
	scopes := []string{
		common.AccountSecretScopeGlobal,
		common.AccountSecretScopeCspDomain,
		common.AccountSecretScopeCluster,
	}
	svm := accountSecretMapValuesByScope("clusterID", "cspDomainID")
	svC := svm[common.AccountSecretScopeCluster]
	svD := svm[common.AccountSecretScopeCspDomain]
	svG := svm[common.AccountSecretScopeGlobal]

	// UpdateSecrets test
	newClosure := func(scope string) *accountSecretRetriever {
		clObj.ClusterUsagePolicy = &models.ClusterUsagePolicy{
			AccountSecretScope: scope,
		}
		return &accountSecretRetriever{c: hc, clusterObj: clObj}
	}

	newSecretCases := []struct{ scope, value string }{
		{common.AccountSecretScopeCluster, svC},
		{common.AccountSecretScopeCspDomain, svD},
		{common.AccountSecretScopeGlobal, svG},
	}
	for _, tc := range newSecretCases {
		t.Logf("case: new %s secret", tc.scope)
		fakeUUID = tc.scope + "-SECRET"
		aObj.Secrets = map[string]string{}
		h := newClosure(tc.scope)
		assert.Equal(h, h.Object())
		rc := h.UpdateSecrets(aObj)
		assert.True(rc)
		assert.Equal(fakeUUID, h.secretKey)
		assert.Equal(tc.value, h.secretValue)
		assert.Equal(tc.scope, h.scope)
		assert.Len(aObj.Secrets, 1)
		sv, ok := aObj.Secrets[h.secretKey]
		assert.True(ok)
		assert.Equal(h.secretValue, sv)

		// should get back the same secret regardless of policy scope change
		expSecret := h.secretKey
		for _, scope := range scopes {
			t.Logf("case: lookup original %s secret when policy is %s", tc.scope, scope)
			fakeUUID = expSecret + "XXXX"
			h = newClosure(scope)
			rc = h.UpdateSecrets(aObj)
			assert.False(rc)
			assert.Equal(expSecret, h.secretKey)
			assert.Empty(h.secretValue)
			assert.Empty(h.scope)
			assert.Len(aObj.Secrets, 1)
			_, ok = aObj.Secrets[h.secretKey]
			assert.True(ok)
		}
	}

	// PreUpdateChecks test
	t.Log("case: cluster object not found")
	h := newClosure("CLUSTER")
	h.clusterObj = nil
	h.clusterID = string(clObj.Meta.ID)
	fOps.RetClusterFetchObj = nil
	fOps.RetClusterFetchErr = centrald.ErrorIDVerNotFound
	err := h.PreUpdateCheck(nil, ai, aObj)
	assert.Error(err)
	cE, ok := err.(*centrald.Error)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(cE.C))
	assert.Regexp(centrald.ErrorMissing.M, cE.M)
	assert.Regexp("clusterId", cE.M)
	assert.Equal(ai, fOps.InClusterFetchAi)
	assert.Equal(h.clusterID, fOps.InClusterFetchID)
	assert.Nil(h.clusterObj)

	t.Log("case: cluster object cspDomain has no protection domain")
	h = newClosure("CLUSTER")
	h.clusterID = string(clObj.Meta.ID)
	assert.Equal(clObj, h.clusterObj)
	err = h.PreUpdateCheck(nil, ai, aObj)
	assert.Error(err)
	cE, ok = err.(*centrald.Error)
	assert.True(ok)
	assert.Equal(centrald.ErrorInvalidState.C, int(cE.C))
	assert.Regexp(centrald.ErrorInvalidState.M, cE.M)
	assert.Regexp("no protection domain", cE.M)

	// add the protection domain mapping
	aObj.ProtectionDomains = make(map[string]models.ObjIDMutable)
	aObj.ProtectionDomains[common.ProtectionStoreDefaultKey] = "PD-1"

	t.Log("case: cluster object already loaded")
	fOps.RetClusterFetchObj = clObj
	fOps.RetClusterFetchErr = centrald.ErrorIDVerNotFound
	h = newClosure("CLUSTER")
	h.clusterID = string(clObj.Meta.ID)
	assert.Equal(clObj, h.clusterObj)
	err = h.PreUpdateCheck(nil, ai, aObj)
	assert.NoError(err)
	assert.Equal(clObj, h.clusterObj)

	t.Log("case: load cluster object")
	fOps.RetClusterFetchObj = clObj
	fOps.RetClusterFetchErr = nil
	h.clusterObj = nil
	h = newClosure("CLUSTER")
	h.clusterID = string(clObj.Meta.ID)
	err = h.PreUpdateCheck(nil, ai, aObj)
	assert.NoError(err)
	assert.Equal(clObj, h.clusterObj)

	// Handler tests
	params := ops.NewAccountSecretRetrieveParams()
	params.HTTPRequest = &http.Request{}
	params.ID = string(aObj.Meta.ID)

	t.Log("case: no auth in request")
	ret := hc.accountSecretRetrieve(params)
	assert.NotNil(ret)
	mD, ok := ret.(*ops.AccountSecretRetrieveDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)

	t.Log("case: invalid auth")
	ai.RoleObj = &models.Role{}
	assert.Error(ai.InternalOK())
	params.HTTPRequest = requestWithAuthContext(ai)
	ret = hc.accountSecretRetrieve(params)
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AccountSecretRetrieveDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)

	t.Log("case: missing clusterId")
	ai = &auth.Info{}
	assert.NoError(ai.InternalOK())
	params.HTTPRequest = requestWithAuthContext(ai)
	ret = hc.accountSecretRetrieve(params)
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AccountSecretRetrieveDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorMissing.M, *mD.Payload.Message)
	assert.Regexp("clusterId", *mD.Payload.Message)

	t.Log("case: other driver failure")
	params.ClusterID = string(clObj.Meta.ID)
	fOps.RetAccountSecretDriverErr = centrald.ErrorIDVerNotFound
	ret = hc.accountSecretRetrieve(params)
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AccountSecretRetrieveDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorIDVerNotFound.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorIDVerNotFound.M, *mD.Payload.Message)

	// intercept support for the driver closure
	asr := accountSecretRetriever{}
	sdCB := func(ops accountSecretDriverOps) {
		c := ops.Object()
		h, ok := c.(*accountSecretRetriever)
		assert.True(ok)
		assert.Equal(hc, h.c)
		*h = asr // inject return values
	}

	t.Log("case: secret exists")
	asr.secretKey = "secret"
	fOps.AccountSecretDriverCB = sdCB
	fOps.RetAccountSecretDriverErr = nil
	ret = hc.accountSecretRetrieve(params)
	assert.NotNil(ret)
	mO, ok := ret.(*ops.AccountSecretRetrieveOK)
	assert.True(ok)
	assert.NotNil(mO.Payload)
	assert.Equal(common.ValueTypeSecret, mO.Payload.Kind)
	assert.Equal("secret", mO.Payload.Value)

	t.Log("case: secret created")
	asr.secretKey = "secret"
	asr.scope = "CLUSTER"
	asr.secretValue = svC
	fOps.AccountSecretDriverCB = sdCB
	fOps.RetAccountSecretDriverErr = nil
	ret = hc.accountSecretRetrieve(params)
	assert.NotNil(ret)
	mO, ok = ret.(*ops.AccountSecretRetrieveOK)
	assert.True(ok)
	assert.NotNil(mO.Payload)
	assert.Equal(common.ValueTypeSecret, mO.Payload.Kind)
	assert.Equal("secret", mO.Payload.Value)
}

func TestAccountSecretReset(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	fa := &fal.AuditLog{}
	app.AuditLog = fa
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	fOps := &fakeOps{}
	hc.ops = fOps

	ai := &auth.Info{}
	aObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{ID: "accountID"},
		},
	}
	params := ops.NewAccountSecretResetParams()

	t.Log("case: no auth in request")
	params.HTTPRequest = &http.Request{}
	ret := hc.accountSecretReset(params)
	assert.NotNil(ret)
	mD, ok := ret.(*ops.AccountSecretResetDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	tl.Flush()

	t.Log("case: invalid scope")
	params.HTTPRequest = requestWithAuthContext(ai)
	params.AccountSecretScope = "foo"
	ret = hc.accountSecretReset(params)
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AccountSecretResetDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	assert.Regexp("invalid accountSecretScope", *mD.Payload.Message)
	tl.Flush()

	t.Log("case: auth failure")
	ai.RoleObj = &models.Role{}
	params.AccountSecretScope = common.AccountSecretScopeCspDomain
	fOps.RetAccountFetchObj = aObj
	ret = hc.accountSecretReset(params)
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AccountSecretResetDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)
	tl.Flush()

	t.Log("case: cspDomainId missing")
	ai.RoleObj = nil
	params.AccountSecretScope = common.AccountSecretScopeCspDomain
	params.CspDomainID = nil
	ret = hc.accountSecretReset(params)
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AccountSecretResetDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorMissing.M, *mD.Payload.Message)
	assert.Regexp("cspDomainId", *mD.Payload.Message)
	tl.Flush()

	t.Log("case: clusterId missing")
	ai.RoleObj = nil
	params.AccountSecretScope = common.AccountSecretScopeCluster
	params.ClusterID = nil
	ret = hc.accountSecretReset(params)
	assert.NotNil(ret)
	mD, ok = ret.(*ops.AccountSecretResetDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorMissing.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorMissing.M, *mD.Payload.Message)
	assert.Regexp("clusterId", *mD.Payload.Message)
	tl.Flush()

	t.Log("case: CSPDOMAIN no change")
	params.AccountSecretScope = common.AccountSecretScopeCspDomain
	params.CspDomainID = swag.String("cspDomainID")
	ret = hc.accountSecretReset(params)
	assert.NotNil(ret)
	mO, ok := ret.(*ops.AccountSecretResetNoContent)
	assert.True(ok)
	assert.NotNil(mO)
	tl.Flush()

	t.Log("case: CLUSTER no change")
	params.AccountSecretScope = common.AccountSecretScopeCluster
	params.ClusterID = swag.String("clusterID")
	ret = hc.accountSecretReset(params)
	assert.NotNil(ret)
	mO, ok = ret.(*ops.AccountSecretResetNoContent)
	assert.True(ok)
	assert.NotNil(mO)
	tl.Flush()

	// UpdateSecrets test
	svm := accountSecretMapValuesByScope("clID", "domID")
	sMap := func() map[string]string {
		return map[string]string{
			"clSecret":  svm[common.AccountSecretScopeCluster],
			"domSecret": svm[common.AccountSecretScopeCspDomain],
			"gSecret":   svm[common.AccountSecretScopeGlobal],
		}
	}
	newClosure := func(scope string, recursive bool, clID, domID string) *accountSecretResetter {
		return &accountSecretResetter{c: hc, scope: scope, recursive: recursive, clID: clID, domID: domID}
	}

	t.Log("case: CLUSTER secret (nR)")
	h := newClosure(common.AccountSecretScopeCluster, false, "clID", "")
	aObj.Secrets = sMap()
	rc := h.UpdateSecrets(aObj)
	assert.Equal(aObj, h.aObj)
	assert.True(rc)
	expMap := sMap()
	delete(expMap, "clSecret")
	assert.Equal(expMap, aObj.Secrets)
	asr := h.Object().(*accountSecretResetter)
	assert.Equal(1, asr.numDeleted)
	tl.Flush()

	t.Log("case: CLUSTER secret (nR) not found")
	h = newClosure(common.AccountSecretScopeCluster, false, "clIDNotFound", "")
	aObj.Secrets = sMap()
	rc = h.UpdateSecrets(aObj)
	assert.False(rc)
	expMap = sMap()
	assert.Equal(expMap, aObj.Secrets)
	asr = h.Object().(*accountSecretResetter)
	assert.Equal(0, asr.numDeleted)
	tl.Flush()

	t.Log("case: CLUSTER secret (R)")
	h = newClosure(common.AccountSecretScopeCluster, true, "clID", "")
	aObj.Secrets = sMap()
	rc = h.UpdateSecrets(aObj)
	assert.True(rc)
	expMap = sMap()
	delete(expMap, "clSecret")
	assert.Equal(expMap, aObj.Secrets)
	asr = h.Object().(*accountSecretResetter)
	assert.Equal(1, asr.numDeleted)
	tl.Flush()

	t.Log("case: CLUSTER secret (R) not found")
	h = newClosure(common.AccountSecretScopeCluster, true, "clIDNotFound", "")
	aObj.Secrets = sMap()
	rc = h.UpdateSecrets(aObj)
	assert.False(rc)
	expMap = sMap()
	assert.Equal(expMap, aObj.Secrets)
	asr = h.Object().(*accountSecretResetter)
	assert.Equal(0, asr.numDeleted)
	tl.Flush()

	t.Log("case: DOMAIN secret (nR)")
	h = newClosure(common.AccountSecretScopeCspDomain, false, "", "domID")
	aObj.Secrets = sMap()
	rc = h.UpdateSecrets(aObj)
	assert.True(rc)
	expMap = sMap()
	delete(expMap, "domSecret")
	assert.Equal(expMap, aObj.Secrets)
	asr = h.Object().(*accountSecretResetter)
	assert.Equal(1, asr.numDeleted)
	tl.Flush()

	t.Log("case: DOMAIN secret (nR) not found")
	h = newClosure(common.AccountSecretScopeCspDomain, false, "", "domIDNotFound")
	aObj.Secrets = sMap()
	rc = h.UpdateSecrets(aObj)
	assert.False(rc)
	expMap = sMap()
	assert.Equal(expMap, aObj.Secrets)
	asr = h.Object().(*accountSecretResetter)
	assert.Equal(0, asr.numDeleted)
	tl.Flush()

	t.Log("case: DOMAIN secret (R)")
	h = newClosure(common.AccountSecretScopeCspDomain, true, "", "domID")
	aObj.Secrets = sMap()
	rc = h.UpdateSecrets(aObj)
	assert.True(rc)
	expMap = sMap()
	delete(expMap, "clSecret")
	delete(expMap, "domSecret")
	assert.Equal(expMap, aObj.Secrets)
	asr = h.Object().(*accountSecretResetter)
	assert.Equal(2, asr.numDeleted)
	tl.Flush()

	t.Log("case: DOMAIN secret (R) not found")
	h = newClosure(common.AccountSecretScopeCspDomain, true, "", "domIDNotFound")
	aObj.Secrets = sMap()
	rc = h.UpdateSecrets(aObj)
	assert.False(rc)
	expMap = sMap()
	assert.Equal(expMap, aObj.Secrets)
	asr = h.Object().(*accountSecretResetter)
	assert.Equal(0, asr.numDeleted)
	tl.Flush()

	t.Log("case: GLOBAL secret (nR)")
	h = newClosure(common.AccountSecretScopeGlobal, false, "", "")
	aObj.Secrets = sMap()
	rc = h.UpdateSecrets(aObj)
	assert.True(rc)
	expMap = sMap()
	delete(expMap, "gSecret")
	assert.Equal(expMap, aObj.Secrets)
	asr = h.Object().(*accountSecretResetter)
	assert.Equal(1, asr.numDeleted)
	tl.Flush()

	t.Log("case: GLOBAL secret (nR) not found")
	h = newClosure(common.AccountSecretScopeGlobal, false, "", "")
	aObj.Secrets = sMap()
	delete(aObj.Secrets, "gSecret")
	rc = h.UpdateSecrets(aObj)
	assert.False(rc)
	expMap = sMap()
	delete(expMap, "gSecret")
	assert.Equal(expMap, aObj.Secrets)
	asr = h.Object().(*accountSecretResetter)
	assert.Equal(0, asr.numDeleted)
	tl.Flush()

	t.Log("case: GLOBAL secret (R)")
	h = newClosure(common.AccountSecretScopeGlobal, true, "", "")
	aObj.Secrets = sMap()
	rc = h.UpdateSecrets(aObj)
	assert.True(rc)
	assert.Empty(aObj.Secrets)
	asr = h.Object().(*accountSecretResetter)
	assert.Equal(3, asr.numDeleted)
	tl.Flush()

	t.Log("case: GLOBAL secret (R) not present")
	h = newClosure(common.AccountSecretScopeGlobal, true, "", "")
	aObj.Secrets = sMap()
	delete(aObj.Secrets, "gSecret")
	rc = h.UpdateSecrets(aObj)
	assert.True(rc)
	assert.Empty(aObj.Secrets)
	asr = h.Object().(*accountSecretResetter)
	assert.Equal(2, asr.numDeleted)
	tl.Flush()
}

func TestAccountProtectionDomainsDriver(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	fa := &fal.AuditLog{}
	app.AuditLog = fa
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app

	ai := &auth.Info{}
	aID := "accountID"
	accountObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{
				ID:      models.ObjID(aID),
				Version: 8,
			},
		},
	}
	var err error
	var aObj *models.Account
	ctx := context.Background()

	// success cases
	tcs := []string{"authorized-no-version", "authorized-with-version-no-update", "internal"}
	for _, tc := range tcs {
		t.Log("case:", tc)
		ai.RoleObj = &models.Role{}
		ai.RoleObj.Capabilities = map[string]bool{
			centrald.AccountUpdateCap:              true,
			centrald.ProtectionDomainManagementCap: true,
		}
		ai.AccountID = aID
		assert.Nil(accountObj.ProtectionDomains)
		version := int32(0)
		hf := &fakeProtectionDomainsDriverOps{}
		fOps := &fakeOps{}
		hc.ops = fOps
		testutils.Clone(accountObj, &fOps.RetAccountFetchObj)
		testutils.Clone(accountObj, &fOps.RetAccountUpdateObj)
		fOps.RetAccountUpdateObj.Meta.Version++
		doesUpdate := true
		switch tc {
		case "authorized-no-version":
		case "authorized-with-version-no-update":
			version = int32(accountObj.Meta.Version)
			doesUpdate = false
		case "internal":
			ai.RoleObj = nil
		}
		hf.RetUpdatePDRc = doesUpdate
		aObj, err = hc.accountProtectionDomainsDriver(ctx, ai, aID, version, hf)
		assert.NotNil(aObj)
		assert.Equal(ai, fOps.InAccountFetchAi)
		assert.Equal(aID, fOps.InAccountFetchID)
		if doesUpdate {
			assert.NoError(err)
			assert.Equal(accountObj.Meta.Version+1, aObj.Meta.Version)
			assert.NotNil(fOps.InAccountUpdateObj)
			assert.NotNil(fOps.InAccountUpdateObj.ProtectionDomains)
			expUParams := ops.NewAccountUpdateParams()
			expUParams.HTTPRequest = (&http.Request{}).WithContext(ctx)
			expUParams.ID = aID
			expUParams.Version = swag.Int32(int32(accountObj.Meta.Version))
			expUParams.Set = []string{"protectionDomains"}
			expUParams.Payload = &models.AccountMutable{ProtectionDomains: fOps.InAccountUpdateObj.ProtectionDomains}
			assert.Equal(&expUParams, fOps.InAccountUpdateParams)
			assert.Equal(&auth.Info{}, fOps.InAccountUpdateAi) // internal
		} else {
			assert.Error(errAccountProtectionDriverNoChange, err)
			assert.Equal(accountObj.Meta.Version, aObj.Meta.Version)
			assert.Nil(fOps.InAccountUpdateObj)
		}
		tl.Flush()
	}

	// error cases
	tcs = []string{"update-fails", "pre-update-check-fails", "no-pd-cap", "no-update-cap", "version-mismatch", "fetch-fails", "audit-not-ready"}
	for _, tc := range tcs {
		t.Log("case:", tc)
		ai.RoleObj = &models.Role{}
		ai.RoleObj.Capabilities = map[string]bool{
			centrald.AccountUpdateCap:              true,
			centrald.ProtectionDomainManagementCap: true,
		}
		ai.AccountID = aID
		assert.Nil(accountObj.ProtectionDomains)
		version := int32(0)
		hf := &fakeProtectionDomainsDriverOps{}
		hf.RetUpdatePDRc = true
		fOps := &fakeOps{}
		hc.ops = fOps
		fa.Posts = []*fal.Args{}
		testutils.Clone(accountObj, &fOps.RetAccountFetchObj)
		var expErr error
		expAl := &fal.Args{AI: ai, Action: centrald.AccountUpdateAction, ObjID: accountObj.Meta.ID, Err: true, Message: "Unauthorized protection domain update"}
		switch tc {
		case "update-fails":
			expErr = centrald.ErrorDbError
			fOps.RetAccountUpdateErr = expErr
			expAl = nil
		case "pre-update-check-fails":
			expErr = centrald.ErrorUnauthorizedOrForbidden
			hf.RetPreUpdateCheckErr = expErr
		case "no-pd-cap":
			expErr = centrald.ErrorUnauthorizedOrForbidden
			delete(ai.RoleObj.Capabilities, centrald.ProtectionDomainManagementCap)
		case "no-update-cap":
			expErr = centrald.ErrorUnauthorizedOrForbidden
			delete(ai.RoleObj.Capabilities, centrald.AccountUpdateCap)
		case "version-mismatch":
			version = int32(accountObj.Meta.Version + 1)
			expAl = nil
			expErr = centrald.ErrorIDVerNotFound
		case "fetch-fails":
			expErr = centrald.ErrorDbError
			fOps.RetAccountFetchErr = expErr
			fOps.RetAccountFetchObj = nil
			expAl = nil
		case "audit-not-ready":
			expErr = centrald.ErrorInternalError
			fa.ReadyRet = expErr
			expAl = nil
		default:
			assert.Equal("", tc)
			continue
		}
		aObj, err = hc.accountProtectionDomainsDriver(ctx, ai, aID, version, hf)
		assert.Error(err)
		if expErr != nil {
			assert.Equal(expErr, err)
		}
		assert.Nil(aObj)
		if expAl != nil {
			assert.Equal([]*fal.Args{expAl}, fa.Posts)
		}
		tl.Flush()
	}
}

type fakeProtectionDomainsDriverOps struct {
	InUpdatePDObj        *models.Account
	RetUpdatePDRc        bool
	InPreUpdateCheckCtx  context.Context
	InPreUpdateCheckAi   *auth.Info
	InPreUpdateCheckObj  *models.Account
	RetPreUpdateCheckErr error
}

var _ = accountProtectionDomainsDriverOps(&fakeProtectionDomainsDriverOps{})

func (op *fakeProtectionDomainsDriverOps) PreUpdateCheck(ctx context.Context, ai *auth.Info, aObj *models.Account) error {
	return op.RetPreUpdateCheckErr
}

func (op *fakeProtectionDomainsDriverOps) UpdateProtectionDomains(aObj *models.Account) bool {
	return op.RetUpdatePDRc
}

func TestAccountProtectionDomainsSet(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	fa := &fal.AuditLog{}
	app.AuditLog = fa
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app

	ai := &auth.Info{}
	aID := "accountID"
	accountObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{
				ID:      models.ObjID(aID),
				Version: 8,
			},
		},
	}
	accountObj.Name = "Account"
	pdObj := &models.ProtectionDomain{}
	pdObj.Meta = &models.ObjMeta{ID: "pdID"}
	pdObj.AccountID = models.ObjIDMutable(aID)
	pdObj.Name = "PD"
	pdID := string(pdObj.Meta.ID)
	cspObj := &models.CSPDomain{}
	cspObj.Meta = &models.ObjMeta{ID: "cspID"}
	cspObj.AuthorizedAccounts = []models.ObjIDMutable{pdObj.AccountID}
	cspObj.Name = "CSP"
	cspID := string(cspObj.Meta.ID)

	var err error
	var aObj *models.Account
	ctx := context.Background()

	// cases
	tcs := []string{"pre-update-check-default", "pre-update-check-csp",
		"pre-update-pd-account-mismatch", "pre-update-pd-fetch-fails",
		"pre-update-csp-fetch-fails", "pre-update-dom-unauthorized",
		"update-default-new", "update-default-changed", "update-default-unchanged",
		"update-csp-new", "update-csp-changed", "update-csp-unchanged"}
	for _, tc := range tcs {
		t.Log("case:", tc)
		fOps := &fakeOps{}
		hc.ops = fOps
		testutils.Clone(pdObj, &fOps.RetProtectionDomainFetchObj)
		testutils.Clone(accountObj, &aObj)
		assert.False(hc.accountReferencesProtectionDomain(aObj, pdID))
		aObj.ProtectionDomains = make(map[string]models.ObjIDMutable)
		var expErr error
		expErrPat := ""
		expRC := false
		expMsg := ""
		expCode := centrald.ErrorInvalidData.C
		key := ""
		h := accountPDSetter{
			c:    hc,
			pdID: pdID,
		}
		switch tc {
		case "pre-update-check-default":
		case "pre-update-check-csp":
			h.cspID = cspID
			testutils.Clone(cspObj, &fOps.RetCspDomainFetchObj)
		case "pre-update-pd-account-mismatch":
			expErr = hc.eUnauthorizedOrForbidden("invalid protectionDomainId usage")
			expMsg = "Protection domain used in wrong account"
			fOps.RetProtectionDomainFetchObj.AccountID += "foo"
		case "pre-update-pd-fetch-fails":
			expErr = centrald.ErrorDbError
			fOps.RetProtectionDomainFetchObj = nil
			fOps.RetProtectionDomainFetchErr = expErr
		case "pre-update-csp-fetch-fails":
			h.cspID = cspID
			expErrPat = "invalid cspDomainId"
			fOps.RetCspDomainFetchErr = centrald.ErrorDbError
		case "pre-update-dom-unauthorized":
			h.cspID = cspID
			expErr = hc.eUnauthorizedOrForbidden("invalid cspDomainId")
			fOps.RetCspDomainFetchErr = centrald.ErrorUnauthorizedOrForbidden
		case "update-default-new":
			expRC = true
			key = common.ProtectionStoreDefaultKey
			expMsg = "Set as DEFAULT"
		case "update-default-changed":
			expRC = true
			key = common.ProtectionStoreDefaultKey
			aObj.ProtectionDomains[key] = models.ObjIDMutable(pdID + "foo")
			expMsg = "Set as DEFAULT"
		case "update-default-unchanged":
			expRC = false
			key = common.ProtectionStoreDefaultKey
			aObj.ProtectionDomains[key] = models.ObjIDMutable(pdID)
		case "update-csp-new":
			h.cspID = cspID
			expRC = true
			key = cspID
			expMsg = "Set for domain 'CSP'"
		case "update-csp-changed":
			h.cspID = cspID
			expRC = true
			key = cspID
			aObj.ProtectionDomains[key] = models.ObjIDMutable(pdID + "foo")
			expMsg = "Set for domain 'CSP'"
		case "update-csp-unchanged":
			h.cspID = cspID
			expRC = false
			key = cspID
			aObj.ProtectionDomains[key] = models.ObjIDMutable(pdID)
		default:
			assert.Equal("", tc)
			continue
		}
		assert.NotNil(aObj.ProtectionDomains)
		if strings.HasPrefix(tc, "pre-") {
			err = h.PreUpdateCheck(ctx, ai, aObj)
			if expErr != nil {
				assert.Equal(expErr, err)
			} else if expErrPat != "" {
				assert.Regexp(expErrPat, err)
				e, ok := err.(*centrald.Error)
				assert.True(ok)
				assert.Equal(expCode, e.C)
			} else {
				assert.Equal(pdObj, h.pdObj)
				if h.cspID != "" {
					assert.Equal(cspObj, h.cspObj)
				}
			}
			assert.Equal(aObj, h.aObj)
		} else {
			h.aObj = aObj
			h.pdObj = pdObj
			h.cspObj = cspObj
			t.Log(aObj.ProtectionDomains)
			rc := h.UpdateProtectionDomains(aObj)
			assert.Equal(expRC, rc)
			assert.Len(aObj.ProtectionDomains, 1)
		}
		if key != "" {
			assert.Equal(key, h.key)
			assert.Contains(aObj.ProtectionDomains, key)
			assert.EqualValues(pdID, aObj.ProtectionDomains[key])
			assert.True(hc.accountReferencesProtectionDomain(aObj, pdID))
		}
		if expMsg != "" {
			assert.Equal(expMsg, h.auditMsg)
		}
		tl.Flush()
	}

	// real accountProtectionDomainSet cases
	tcs = []string{"set-ok", "set-no-change", "set-driver-wrong-pd-account", "set-driver-unauthorized", "ai-error"}
	for _, tc := range tcs {
		t.Log("case:", tc)
		fOps := &fakeOps{}
		hc.ops = fOps
		fa.Posts = nil
		testutils.Clone(accountObj, &fOps.RetAccountFetchObj)
		testutils.Clone(accountObj, &fOps.RetAccountUpdateObj)
		fOps.RetAccountUpdateObj.Meta.Version++
		testutils.Clone(pdObj, &fOps.RetProtectionDomainFetchObj)
		testutils.Clone(cspObj, &fOps.RetCspDomainFetchObj)
		ai = &auth.Info{}
		params := ops.AccountProtectionDomainSetParams{
			HTTPRequest:        requestWithAuthContext(ai),
			ID:                 aID,
			ProtectionDomainID: pdID,
			CspDomainID:        swag.String(cspID),
			Version:            swag.Int32(int32(accountObj.Meta.Version)),
		}
		ctx = params.HTTPRequest.Context()
		ai.RoleObj = &models.Role{}
		ai.RoleObj.Capabilities = map[string]bool{
			centrald.AccountUpdateCap:              true,
			centrald.ProtectionDomainManagementCap: true,
		}
		ai.AccountID = aID
		expErrCode := 0
		expErrPat := ""
		var expPosts []*fal.Args
		hasChange := true
		switch tc {
		case "set-ok":
			expPosts = []*fal.Args{
				&fal.Args{AI: ai, Action: centrald.ProtectionDomainSetAction, ObjID: pdObj.Meta.ID, Name: "PD", RefID: models.ObjIDMutable(cspID), Message: "Set for domain 'CSP'"},
			}
		case "set-no-change":
			fOps.RetAccountFetchObj.ProtectionDomains = map[string]models.ObjIDMutable{
				cspID: models.ObjIDMutable(pdID),
			}
			hasChange = false
		case "set-driver-wrong-pd-account":
			fOps.RetProtectionDomainFetchObj.AccountID = "foo"
			expPosts = []*fal.Args{
				&fal.Args{AI: ai, Action: centrald.AccountUpdateAction, ObjID: accountObj.Meta.ID, Name: "Account", Err: true, Message: "Unauthorized protection domain update"},
				&fal.Args{AI: ai, Action: centrald.ProtectionDomainSetAction, ObjID: pdObj.Meta.ID, Name: "PD", RefID: models.ObjIDMutable(cspID), Err: true, Message: "Protection domain used in wrong account"},
			}
			expErrCode = centrald.ErrorUnauthorizedOrForbidden.C
			expErrPat = centrald.ErrorUnauthorizedOrForbidden.M
		case "set-driver-unauthorized":
			delete(ai.RoleObj.Capabilities, centrald.AccountUpdateCap)
			expPosts = []*fal.Args{
				&fal.Args{AI: ai, Action: centrald.AccountUpdateAction, ObjID: accountObj.Meta.ID, Name: "Account", Err: true, Message: "Unauthorized protection domain update"},
			}
			expErrCode = centrald.ErrorUnauthorizedOrForbidden.C
			expErrPat = centrald.ErrorUnauthorizedOrForbidden.M
		case "ai-error":
			params.HTTPRequest = &http.Request{}
			expErrCode = centrald.ErrorInternalError.C
			expErrPat = centrald.ErrorInternalError.M
		default:
			assert.Equal("", tc)
			continue
		}
		ret := hc.accountProtectionDomainSet(params)
		retOk, ok1 := ret.(*ops.AccountProtectionDomainSetOK)
		retD, ok2 := ret.(*ops.AccountProtectionDomainSetDefault)
		if expErrPat != "" {
			assert.True(ok2)
			assert.Regexp(expErrPat, *retD.Payload.Message)
			assert.EqualValues(expErrCode, retD.Payload.Code)
		} else {
			assert.True(ok1)
			aObj = retOk.Payload
			if hasChange {
				assert.Equal(accountObj.Meta.Version+1, aObj.Meta.Version)
			} else {
				assert.Equal(accountObj.Meta.Version, aObj.Meta.Version)
			}
		}
		assert.Equal(expPosts, fa.Posts)
		tl.Flush()
	}

	// test accountProtectionDomainForProtectionStore
	aObj.ProtectionDomains = make(map[string]models.ObjIDMutable)
	assert.Empty(hc.accountProtectionDomainForProtectionStore(aObj, models.ObjIDMutable(cspID)))
	aObj.ProtectionDomains["foo"] = models.ObjIDMutable(pdID)
	assert.Empty(hc.accountProtectionDomainForProtectionStore(aObj, models.ObjIDMutable(cspID)))
	aObj.ProtectionDomains[common.ProtectionStoreDefaultKey] = models.ObjIDMutable(pdID)
	assert.EqualValues(pdID, hc.accountProtectionDomainForProtectionStore(aObj, models.ObjIDMutable(cspID)))
	aObj.ProtectionDomains[cspID] = models.ObjIDMutable(pdID + "foo")
	assert.EqualValues(pdID+"foo", hc.accountProtectionDomainForProtectionStore(aObj, models.ObjIDMutable(cspID)))
}

func TestAccountProtectionDomainsClear(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	fa := &fal.AuditLog{}
	app.AuditLog = fa
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app

	ai := &auth.Info{}
	aID := "accountID"
	accountObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{
				ID:      models.ObjID(aID),
				Version: 8,
			},
		},
	}
	accountObj.Name = "Account"
	pdObj := &models.ProtectionDomain{}
	pdObj.Meta = &models.ObjMeta{ID: "pdID"}
	pdObj.AccountID = models.ObjIDMutable(aID)
	pdObj.Name = "PD"
	pdID := string(pdObj.Meta.ID)
	cspObj := &models.CSPDomain{}
	cspObj.Meta = &models.ObjMeta{ID: "cspID"}
	cspObj.AuthorizedAccounts = []models.ObjIDMutable{pdObj.AccountID}
	cspObj.Name = "CSP"
	cspID := string(cspObj.Meta.ID)

	var err error
	var aObj *models.Account
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()

	// cases
	tcs := []string{
		"pre-default-not-in-map", "pre-csp-not-in-map", "pre-load-pd-error", "pre-load-dom-error",
		"pre-default-not-in-map-has-bound-vs-error", "pre-default-not-in-map-has-bound-vs",
		"pre-default-not-in-map-has-no-bound-vs", "pre-default",
		"pre-csp-no-longer-authorized", "pre-csp-has-default",
		"pre-csp-no-default-has-bound-vs", "pre-csp-no-default-has-no-bound-vs",
		"update-no-key", "update-has-key",
	}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		t.Log("case:", tc)
		mds := mock.NewMockDataStore(mockCtrl)
		oCSP := mock.NewMockCspDomainOps(mockCtrl)
		domLP := csp_domain.CspDomainListParams{
			AuthorizedAccountID: swag.String(aID),
		}
		oV := mock.NewMockVolumeSeriesOps(mockCtrl)
		vsLP := volume_series.VolumeSeriesListParams{
			AccountID:        swag.String(aID),
			BoundCspDomainID: swag.String(cspID),
		}
		hc.DS = mds
		fOps := &fakeOps{}
		hc.ops = fOps
		testutils.Clone(pdObj, &fOps.RetProtectionDomainFetchObj)
		testutils.Clone(accountObj, &aObj)
		assert.False(hc.accountReferencesProtectionDomain(aObj, pdID))
		aObj.ProtectionDomains = map[string]models.ObjIDMutable{
			common.ProtectionStoreDefaultKey: models.ObjIDMutable(pdID),
			cspID:                            models.ObjIDMutable(pdID),
		}
		expPDKeys := []string{common.ProtectionStoreDefaultKey, cspID}
		expInMap := true
		var expErr error
		expErrPat := ""
		expRC := true
		expMsg := ""
		h := accountPDClearer{
			c: hc,
		}
		switch tc {
		case "pre-default-not-in-map":
			aObj.ProtectionDomains = map[string]models.ObjIDMutable{}
			expInMap = false
			expPDKeys = []string{}
		case "pre-csp-not-in-map":
			h.cspID = cspID
			delete(aObj.ProtectionDomains, cspID)
			expInMap = false
			expPDKeys = []string{common.ProtectionStoreDefaultKey}
		case "pre-load-pd-error":
			expErr = centrald.ErrorDbError
			fOps.RetProtectionDomainFetchErr = expErr
		case "pre-load-dom-error":
			expErr = centrald.ErrorNotFound
			oCSP.EXPECT().List(ctx, domLP).Return(nil, expErr)
			mds.EXPECT().OpsCspDomain().Return(oCSP)
		case "pre-default-not-in-map-has-bound-vs-error":
			delete(aObj.ProtectionDomains, cspID)
			expPDKeys = []string{common.ProtectionStoreDefaultKey}
			expErr = centrald.ErrorDbError
			oCSP.EXPECT().List(ctx, domLP).Return([]*models.CSPDomain{cspObj}, nil)
			mds.EXPECT().OpsCspDomain().Return(oCSP)
			oV.EXPECT().Count(ctx, vsLP, uint(1)).Return(0, expErr)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
		case "pre-default-not-in-map-has-bound-vs":
			delete(aObj.ProtectionDomains, cspID)
			expPDKeys = []string{common.ProtectionStoreDefaultKey}
			expErrPat = "volumeSeries are impacted by this operation"
			oCSP.EXPECT().List(ctx, domLP).Return([]*models.CSPDomain{cspObj}, nil)
			mds.EXPECT().OpsCspDomain().Return(oCSP)
			oV.EXPECT().Count(ctx, vsLP, uint(1)).Return(1, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
		case "pre-default-not-in-map-has-no-bound-vs":
			delete(aObj.ProtectionDomains, cspID)
			expPDKeys = []string{common.ProtectionStoreDefaultKey}
			oCSP.EXPECT().List(ctx, domLP).Return([]*models.CSPDomain{cspObj}, nil)
			mds.EXPECT().OpsCspDomain().Return(oCSP)
			oV.EXPECT().Count(ctx, vsLP, uint(1)).Return(0, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
			expMsg = "DEFAULT unset"
		case "pre-default":
			expMsg = "DEFAULT unset"
			oCSP.EXPECT().List(ctx, domLP).Return([]*models.CSPDomain{}, nil)
			mds.EXPECT().OpsCspDomain().Return(oCSP)
		case "pre-csp-no-longer-authorized":
			h.cspID = cspID
			expMsg = "Unset for domain [cspID]"
			oCSP.EXPECT().List(ctx, domLP).Return([]*models.CSPDomain{}, nil)
			mds.EXPECT().OpsCspDomain().Return(oCSP)
		case "pre-csp-has-default":
			h.cspID = cspID
			expMsg = "Unset for domain 'CSP'"
			oCSP.EXPECT().List(ctx, domLP).Return([]*models.CSPDomain{cspObj}, nil)
			mds.EXPECT().OpsCspDomain().Return(oCSP)
		case "pre-csp-no-default-has-bound-vs":
			h.cspID = cspID
			delete(aObj.ProtectionDomains, common.ProtectionStoreDefaultKey)
			expPDKeys = []string{cspID}
			expMsg = "Unset for domain 'CSP'"
			expErrPat = "volumeSeries are impacted by this operation"
			oCSP.EXPECT().List(ctx, domLP).Return([]*models.CSPDomain{cspObj}, nil)
			mds.EXPECT().OpsCspDomain().Return(oCSP)
			oV.EXPECT().Count(ctx, vsLP, uint(1)).Return(1, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
		case "pre-csp-no-default-has-no-bound-vs":
			h.cspID = cspID
			delete(aObj.ProtectionDomains, common.ProtectionStoreDefaultKey)
			expPDKeys = []string{cspID}
			expMsg = "Unset for domain 'CSP'"
			oCSP.EXPECT().List(ctx, domLP).Return([]*models.CSPDomain{cspObj}, nil)
			mds.EXPECT().OpsCspDomain().Return(oCSP)
			oV.EXPECT().Count(ctx, vsLP, uint(1)).Return(0, nil)
			mds.EXPECT().OpsVolumeSeries().Return(oV)
		case "update-no-key":
			expRC = false
			expInMap = false
		case "update-has-key":
			h.key = "cspID"
			expInMap = true
			expPDKeys = []string{common.ProtectionStoreDefaultKey}
		default:
			assert.Equal("", tc)
			continue
		}
		assert.NotNil(aObj.ProtectionDomains)
		if strings.HasPrefix(tc, "pre-") {
			err = h.PreUpdateCheck(ctx, ai, aObj)
			if expErr != nil {
				assert.Equal(expErr, err)
			} else if expErrPat != "" {
				assert.Regexp(expErrPat, err)
			} else {
				assert.NoError(err)
			}
			assert.Equal(aObj, h.aObj)
		} else {
			h.aObj = aObj
			h.pdObj = pdObj
			rc := h.UpdateProtectionDomains(aObj)
			assert.Equal(expRC, rc)
		}
		if expInMap {
			assert.NotEmpty(h.key)
		}
		for _, k := range expPDKeys {
			assert.Contains(aObj.ProtectionDomains, k, "key", k)
		}
		assert.Len(aObj.ProtectionDomains, len(expPDKeys))
		if expMsg != "" {
			assert.Equal(expMsg, h.auditMsg)
		}
		tl.Flush()
	}

	// real accountProtectionDomainClear cases
	tcs = []string{"clear-default", "clear-csp", "clear-no-change", "clear-driver-error", "ai-error"}
	for _, tc := range tcs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		t.Log("case:", tc)
		mds := mock.NewMockDataStore(mockCtrl)
		oCSP := mock.NewMockCspDomainOps(mockCtrl)
		domLP := csp_domain.CspDomainListParams{
			AuthorizedAccountID: swag.String(aID),
		}
		hc.DS = mds
		fOps := &fakeOps{}
		hc.ops = fOps
		fa.Posts = nil
		testutils.Clone(accountObj, &aObj)
		aObj.ProtectionDomains = map[string]models.ObjIDMutable{
			common.ProtectionStoreDefaultKey: models.ObjIDMutable(pdID),
			cspID:                            models.ObjIDMutable(pdID),
		}
		fOps.RetAccountFetchObj = aObj
		testutils.Clone(aObj, &fOps.RetAccountUpdateObj)
		fOps.RetAccountUpdateObj.Meta.Version++
		testutils.Clone(pdObj, &fOps.RetProtectionDomainFetchObj)
		testutils.Clone(cspObj, &fOps.RetCspDomainFetchObj)
		ai = &auth.Info{}
		params := ops.AccountProtectionDomainClearParams{
			HTTPRequest: requestWithAuthContext(ai),
			ID:          aID,
			Version:     swag.Int32(int32(accountObj.Meta.Version)),
		}
		ctx = params.HTTPRequest.Context()
		ai.RoleObj = &models.Role{}
		ai.RoleObj.Capabilities = map[string]bool{
			centrald.AccountUpdateCap:              true,
			centrald.ProtectionDomainManagementCap: true,
		}
		ai.AccountID = aID
		expErrCode := 0
		expErrPat := ""
		var expPosts []*fal.Args
		hasChange := true
		switch tc {
		case "clear-default":
			oCSP.EXPECT().List(ctx, domLP).Return([]*models.CSPDomain{}, nil)
			mds.EXPECT().OpsCspDomain().Return(oCSP)
			expPosts = []*fal.Args{
				&fal.Args{AI: ai, Action: centrald.ProtectionDomainClearAction, ObjID: pdObj.Meta.ID, Name: "PD", Message: "DEFAULT unset"},
			}
		case "clear-csp":
			params.CspDomainID = swag.String(cspID)
			oCSP.EXPECT().List(ctx, domLP).Return([]*models.CSPDomain{}, nil)
			mds.EXPECT().OpsCspDomain().Return(oCSP)
			expPosts = []*fal.Args{
				&fal.Args{AI: ai, Action: centrald.ProtectionDomainClearAction, ObjID: pdObj.Meta.ID, Name: "PD", RefID: models.ObjIDMutable(cspID), Message: "Unset for domain [" + cspID + "]"},
			}
		case "clear-no-change":
			aObj.ProtectionDomains = map[string]models.ObjIDMutable{} // no default
			fOps.RetAccountFetchObj.ProtectionDomains = map[string]models.ObjIDMutable{
				cspID: models.ObjIDMutable(pdID),
			}
			hasChange = false
		case "clear-driver-error":
			fOps.RetAccountFetchObj = nil
			fOps.RetAccountFetchErr = centrald.ErrorUnauthorizedOrForbidden
			expErrCode = centrald.ErrorUnauthorizedOrForbidden.C
			expErrPat = centrald.ErrorUnauthorizedOrForbidden.M
		case "ai-error":
			params.HTTPRequest = &http.Request{}
			expErrCode = centrald.ErrorInternalError.C
			expErrPat = centrald.ErrorInternalError.M
		default:
			assert.Equal("", tc)
			continue
		}
		ret := hc.accountProtectionDomainClear(params)
		retOk, ok1 := ret.(*ops.AccountProtectionDomainClearOK)
		retD, ok2 := ret.(*ops.AccountProtectionDomainClearDefault)
		if expErrPat != "" {
			assert.True(ok2)
			assert.Regexp(expErrPat, *retD.Payload.Message)
			assert.EqualValues(expErrCode, retD.Payload.Code)
		} else {
			assert.True(ok1)
			aObj = retOk.Payload
			if hasChange {
				assert.Equal(accountObj.Meta.Version+1, aObj.Meta.Version)
			} else {
				assert.Equal(accountObj.Meta.Version, aObj.Meta.Version)
			}
		}
		assert.Equal(expPosts, fa.Posts)
		tl.Flush()
	}
}

var testRoles = []*models.Role{
	&models.Role{
		RoleAllOf0:  models.RoleAllOf0{Meta: &models.ObjMeta{ID: "role-s"}},
		RoleMutable: models.RoleMutable{Name: centrald.SystemAdminRole, Capabilities: map[string]bool{centrald.AccountUpdateCap: true}},
	},
	&models.Role{
		RoleAllOf0:  models.RoleAllOf0{Meta: &models.ObjMeta{ID: "role-t"}},
		RoleMutable: models.RoleMutable{Name: centrald.TenantAdminRole, Capabilities: map[string]bool{centrald.AccountUpdateCap: true}},
	},
	&models.Role{
		RoleAllOf0:  models.RoleAllOf0{Meta: &models.ObjMeta{ID: "role-a"}},
		RoleMutable: models.RoleMutable{Name: centrald.AccountAdminRole, Capabilities: map[string]bool{centrald.AccountUpdateCap: true}},
	},
	&models.Role{
		RoleAllOf0:  models.RoleAllOf0{Meta: &models.ObjMeta{ID: "role-u"}},
		RoleMutable: models.RoleMutable{Name: centrald.AccountUserRole},
	},
}
