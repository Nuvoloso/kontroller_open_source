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


package mongods

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/account"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// gomock.Matcher for Account
type mockAccountMatchCtx int

const (
	mockAccountInvalid mockAccountMatchCtx = iota
	mockAccountInsert
	mockAccountFind
	mockAccountUpdate
)

type mockAccountMatcher struct {
	t      *testing.T
	ctx    mockAccountMatchCtx
	ctxObj *Account
	retObj *Account
}

func newAccountMatcher(t *testing.T, ctx mockAccountMatchCtx, obj *Account) gomock.Matcher {
	return accountMatcher(t, ctx).CtxObj(obj).Matcher()
}

// accountMatcher creates a partially initialized mockAccountMatcher
func accountMatcher(t *testing.T, ctx mockAccountMatchCtx) *mockAccountMatcher {
	return &mockAccountMatcher{t: t, ctx: ctx}
}

// Matcher ends the chain
func (o *mockAccountMatcher) Matcher() gomock.Matcher {
	if o.ctx == mockAccountInsert || o.ctx == mockAccountFind || o.ctx == mockAccountUpdate {
		assert.NotNil(o.t, o.ctxObj)
	}
	return o
}

// CtxObj adds an Account object
func (o *mockAccountMatcher) CtxObj(ao *Account) *mockAccountMatcher {
	o.ctxObj = ao
	return o
}

// Return sets the update result object
func (o *mockAccountMatcher) Return(ro *Account) *mockAccountMatcher {
	o.retObj = ro
	return o
}

// Matches is from gomock.Matcher
func (o *mockAccountMatcher) Matches(x interface{}) bool {
	var obj *Account
	switch z := x.(type) {
	case *Account:
		obj = z
	default:
		panic("Did not get a recognizable object: " + reflect.TypeOf(x).String())
	}
	assert := assert.New(o.t)
	switch o.ctx {
	case mockAccountInsert:
		compObj := o.ctxObj
		compObj.ObjMeta = obj.ObjMeta // insert sets meta data
		if compObj.TenantAccountID == "insert-tenant-id" {
			o.ctxObj.TenantAccountID = obj.TenantAccountID // copy out for later validation
			compObj.TenantAccountID = obj.TenantAccountID
		}
		return assert.NotZero(obj.ObjMeta.MetaObjID) &&
			assert.Equal(int32(1), obj.ObjMeta.MetaVersion) &&
			assert.False(obj.ObjMeta.MetaTimeCreated.IsZero()) &&
			assert.False(obj.ObjMeta.MetaTimeModified.IsZero()) &&
			assert.Equal(obj.ObjMeta.MetaTimeCreated, obj.ObjMeta.MetaTimeModified) &&
			assert.Equal(compObj, obj)
	case mockAccountFind:
		*obj = *o.ctxObj
		return true
	case mockAccountUpdate:
		if assert.NotNil(obj) {
			obj.ObjMeta = o.ctxObj.ObjMeta
			assert.Equal(o.ctxObj, obj)
			*obj = *o.retObj
			return true
		}
	}
	return false
}

// String is from gomock.Matcher
func (o *mockAccountMatcher) String() string {
	switch o.ctx {
	case mockAccountInsert:
		return "matches on insert"
	}
	return "unknown context"
}

func TestAccountHandler(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	assert.Equal("account", odhAccount.CName())
	indexes := odhAccount.Indexes()
	expIndexes := []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: tenantAccountIDKey, Value: IndexAscending}, {Key: nameKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
	}
	assert.Equal(expIndexes, indexes)
	assert.Equal(indexes, odhAccount.indexes)
	dIf := odhAccount.NewObject()
	assert.NotNil(dIf)
	_, ok := dIf.(*Account)
	assert.True(ok)

	t.Log("case: Claim + Initialize success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ctx := context.Background()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	odhAccount.Claim(api, crud)
	assert.Equal(api, odhAccount.api)
	assert.Equal(crud, odhAccount.crud)
	assert.Equal(l, odhAccount.log)
	assert.Equal(odhAccount, odhAccount.Ops())
	crud.EXPECT().CreateIndexes(ctx, odhAccount).Return(nil)
	assert.NoError(odhAccount.Initialize(ctx))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize, error creating indexes")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhAccount).Return(errUnknownError)
	odhAccount.crud = crud
	odhAccount.api = api
	assert.Error(errUnknownError, odhAccount.Initialize(ctx))
	tl.Flush()
	mockCtrl.Finish()

	// fake System object in cache (happens after odhSystem.Initialize() )
	dSystem := &System{
		ObjMeta: ObjMeta{
			MetaObjID: "systemID",
		},
		SnapshotManagementPolicy: SnapshotManagementPolicy{
			RetentionDurationSeconds: 99,
		},
		VsrManagementPolicy: VsrManagementPolicy{
			RetentionDurationSeconds: 100,
			NoDelete:                 false,
		},
		SystemTags: StringList{},
	}
	odhSystem.systemID = "systemID"
	odhSystem.systemObj = dSystem
	defer func() {
		odhSystem.systemID = ""
		odhSystem.systemObj = nil
	}()

	roles := odhRole.roles
	odhRole.roles = map[string]*models.Role{}
	odhRole.roles[ds.SystemAdminRole] = &models.Role{RoleAllOf0: models.RoleAllOf0{Meta: &models.ObjMeta{ID: "sysRoleId"}}}
	odhRole.roles[ds.TenantAdminRole] = &models.Role{RoleAllOf0: models.RoleAllOf0{Meta: &models.ObjMeta{ID: "taRoleId"}}}
	odhRole.roles[ds.AccountAdminRole] = &models.Role{RoleAllOf0: models.RoleAllOf0{Meta: &models.ObjMeta{ID: "naRoleId"}}}
	odhRole.roles[ds.AccountUserRole] = &models.Role{RoleAllOf0: models.RoleAllOf0{Meta: &models.ObjMeta{ID: "nuRoleId"}}}
	adminID := odhUser.adminUserID
	odhUser.adminUserID = "adminId"
	defer func() {
		odhRole.roles = roles
		odhUser.adminUserID = adminID
	}()
	findNameArg := bson.M{nameKey: ds.SystemAccount}
	dAccount := &Account{
		Name:         ds.SystemAccount,
		Description:  ds.SystemAccountDescription,
		Tags:         StringList{},
		AccountRoles: ObjIDList{"sysRoleId"},
		UserRoles: AuthRoleMap{
			"adminId": AuthRole{RoleID: "sysRoleId"},
		},
		Messages: []TimestampedString{},
		SnapshotCatalogPolicy: SnapshotCatalogPolicy{
			Inherited: true,
		},
		SnapshotManagementPolicy: SnapshotManagementPolicy{
			Inherited: true,
		},
		VsrManagementPolicy: VsrManagementPolicy{
			Inherited: true,
		},
	}
	dTenant := &Account{
		Name:         ds.DefTenantAccount,
		Description:  ds.DefTenantDescription,
		Tags:         StringList{},
		AccountRoles: ObjIDList{"taRoleId"},
		UserRoles: AuthRoleMap{
			"adminId": AuthRole{RoleID: "taRoleId"},
		},
		Messages: []TimestampedString{},
		SnapshotCatalogPolicy: SnapshotCatalogPolicy{
			Inherited: true,
		},
		SnapshotManagementPolicy: SnapshotManagementPolicy{
			Inherited: true,
		},
		VsrManagementPolicy: VsrManagementPolicy{
			Inherited: true,
		},
	}
	dSub := &Account{
		Name:            ds.DefSubAccount,
		Description:     ds.DefSubAccountDescription,
		TenantAccountID: "insert-tenant-id",
		Tags:            StringList{},
		AccountRoles:    ObjIDList{"naRoleId", "nuRoleId"},
		UserRoles: AuthRoleMap{
			"adminId": AuthRole{RoleID: "naRoleId"},
		},
		Messages: []TimestampedString{},
		SnapshotCatalogPolicy: SnapshotCatalogPolicy{
			Inherited: true,
		},
		SnapshotManagementPolicy: SnapshotManagementPolicy{
			Inherited: true,
		},
		VsrManagementPolicy: VsrManagementPolicy{
			Inherited: true,
		},
	}
	var uSystem *System
	testutils.Clone(dSystem, &uSystem)
	uSystem.SystemTags = []string{common.SystemTagDefTenantCreated}
	sm1 := systemMatcher(t, mockSystemUpdate).CtxObj(uSystem).Return(dSystem).Matcher()
	var uSystem2 *System
	testutils.Clone(dSystem, &uSystem2)
	uSystem2.SystemTags = []string{common.SystemTagDefSubAccountCreated}
	sm2 := systemMatcher(t, mockSystemUpdate).CtxObj(uSystem2).Return(dSystem).Matcher()
	uas := &ds.UpdateArgs{
		ID: "systemID",
		Attributes: []ds.UpdateAttr{
			{Name: "SystemTags", Actions: [ds.NumActionTypes]ds.UpdateActionArgs{ds.UpdateAppend: ds.UpdateActionArgs{FromBody: true}}},
		},
	}

	t.Log("case: Start - System and default accounts created")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return("dbName")
	api.EXPECT().MustBeInitializing().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhAccount, findNameArg, newAccountMatcher(t, mockAccountFind, dAccount)).Return(ds.ErrorNotFound)
	crud.EXPECT().InsertOne(ctx, odhAccount, newAccountMatcher(t, mockAccountInsert, dAccount)).Return(nil)
	crud.EXPECT().InsertOne(ctx, odhAccount, newAccountMatcher(t, mockAccountInsert, dTenant)).Return(nil)
	crud.EXPECT().InsertOne(ctx, odhAccount, newAccountMatcher(t, mockAccountInsert, dSub)).Return(nil)
	odhAccount.crud = crud
	odhAccount.sysAccountID = ""
	odhAccount.defTenantCreated = false
	odhAccount.defSubAccountCreated = false
	odhAccount.api = api
	prev := crud.EXPECT().UpdateOne(ctx, odhSystem, sm1, uas).Return(nil)
	crud.EXPECT().UpdateOne(ctx, odhSystem, sm2, uas).Return(nil).After(prev)
	odhSystem.crud = crud
	odhSystem.api = api
	odhSystem.log = l
	assert.NoError(odhAccount.Start(ctx))
	assert.NotEmpty(odhAccount.sysAccountID)
	assert.True(odhAccount.defTenantCreated)
	assert.True(odhAccount.defSubAccountCreated)
	assert.Equal(dTenant.MetaObjID, dSub.TenantAccountID)
	tenantID := dTenant.MetaObjID
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Start, system account present and valid, tenant and subordinate tags present in system object")
	mockCtrl = gomock.NewController(t)
	dSystem.SystemTags = append(dSystem.SystemTags, common.SystemTagDefTenantCreated, common.SystemTagDefSubAccountCreated)
	odhSystem.systemObj = dSystem
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return("dbName")
	api.EXPECT().MustBeInitializing().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhAccount, findNameArg, newAccountMatcher(t, mockAccountFind, dAccount)).Return(nil)
	odhAccount.crud = crud
	odhAccount.sysAccountID = ""
	odhAccount.defTenantCreated = false
	odhAccount.defSubAccountCreated = false
	odhAccount.api = api
	assert.NoError(odhAccount.Start(ctx))
	assert.NotEmpty(odhAccount.sysAccountID)
	assert.True(odhAccount.defTenantCreated)
	assert.True(odhAccount.defSubAccountCreated)

	t.Log("case: Start, sysAccountID and defTenantCreated and defSubAccountCreated already set")
	api.EXPECT().DBName().Return("dbName")
	api.EXPECT().MustBeInitializing().Return(nil)
	assert.NoError(odhAccount.Start(ctx))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Start, not initializing")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return("dbName")
	api.EXPECT().MustBeInitializing().Return(errUnknownError)
	odhAccount.sysAccountID = ""
	odhAccount.api = api
	assert.Equal(errUnknownError, odhAccount.Start(ctx))
	assert.Empty(odhAccount.sysAccountID)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Start, error creating system account")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return("dbName")
	api.EXPECT().MustBeInitializing().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhAccount, findNameArg, newAccountMatcher(t, mockAccountFind, dAccount)).Return(ds.ErrorNotFound)
	crud.EXPECT().InsertOne(ctx, odhAccount, newAccountMatcher(t, mockAccountInsert, dAccount)).Return(errWrappedError)
	odhAccount.crud = crud
	odhAccount.sysAccountID = ""
	odhAccount.api = api
	assert.Equal(errWrappedError, odhAccount.Start(ctx))
	assert.Empty(odhAccount.sysAccountID)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Start, error creating default tenant account")
	mockCtrl = gomock.NewController(t)
	dSystem.SystemTags = StringList{}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return("dbName")
	api.EXPECT().MustBeInitializing().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().InsertOne(ctx, odhAccount, newAccountMatcher(t, mockAccountInsert, dTenant)).Return(errWrappedError)
	odhAccount.crud = crud
	odhAccount.sysAccountID = "sysAccountID"
	odhAccount.defTenantCreated = false
	odhAccount.api = api
	assert.Equal(errWrappedError, odhAccount.Start(ctx))
	assert.NotEmpty(odhAccount.sysAccountID)
	assert.False(odhAccount.defTenantCreated)
	assert.False(odhAccount.defSubAccountCreated)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Start, default tenant already exists (ok), error updating system object systemTags")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return("dbName")
	api.EXPECT().MustBeInitializing().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().InsertOne(ctx, odhAccount, newAccountMatcher(t, mockAccountInsert, dTenant)).Return(ds.ErrorExists)
	odhAccount.crud = crud
	odhAccount.sysAccountID = "sysAccountID"
	odhAccount.defTenantCreated = false
	odhAccount.api = api
	crud.EXPECT().UpdateOne(ctx, odhSystem, sm1, uas).Return(errWrappedError)
	odhSystem.crud = crud
	odhSystem.api = api
	assert.Equal(errWrappedError, odhAccount.Start(ctx))
	assert.NotEmpty(odhAccount.sysAccountID)
	assert.False(odhAccount.defTenantCreated)
	assert.False(odhAccount.defSubAccountCreated)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Start, error creating default subordinate account after finding built-in tenant")
	mockCtrl = gomock.NewController(t)
	dSystem.SystemTags = StringList{common.SystemTagDefTenantCreated}
	dTenant.MetaObjID = tenantID
	findTenantArg := bson.M{nameKey: common.DefTenantAccount, tenantAccountIDKey: ""}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return("dbName")
	api.EXPECT().MustBeInitializing().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhAccount, findTenantArg, newAccountMatcher(t, mockAccountFind, dTenant)).Return(nil)
	crud.EXPECT().InsertOne(ctx, odhAccount, newAccountMatcher(t, mockAccountInsert, dSub)).Return(errWrappedError)
	odhAccount.crud = crud
	odhAccount.sysAccountID = "sysAccountID"
	odhAccount.defSubAccountCreated = false
	odhAccount.api = api
	assert.Equal(errWrappedError, odhAccount.Start(ctx))
	assert.NotEmpty(odhAccount.sysAccountID)
	assert.True(odhAccount.defTenantCreated)
	assert.False(odhAccount.defSubAccountCreated)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Start, default subordinate already exists (ok), error updating system object systemTags")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return("dbName")
	api.EXPECT().MustBeInitializing().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhAccount, findTenantArg, newAccountMatcher(t, mockAccountFind, dTenant)).Return(nil)
	crud.EXPECT().InsertOne(ctx, odhAccount, newAccountMatcher(t, mockAccountInsert, dSub)).Return(ds.ErrorExists)
	odhAccount.crud = crud
	odhAccount.sysAccountID = "sysAccountID"
	odhAccount.defSubAccountCreated = false
	odhAccount.api = api
	crud.EXPECT().UpdateOne(ctx, odhSystem, sm2, uas).Return(errWrappedError)
	odhSystem.crud = crud
	odhSystem.api = api
	assert.Equal(errWrappedError, odhAccount.Start(ctx))
	assert.NotEmpty(odhAccount.sysAccountID)
	assert.True(odhAccount.defTenantCreated)
	assert.False(odhAccount.defSubAccountCreated)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: built-in tenant not found, skip creating subordinate, set subordinate tag")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return("dbName")
	api.EXPECT().MustBeInitializing().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhAccount, findTenantArg, newAccountMatcher(t, mockAccountFind, dTenant)).Return(ds.ErrorNotFound)
	odhAccount.crud = crud
	odhAccount.sysAccountID = "sysAccountID"
	odhAccount.defSubAccountCreated = false
	odhAccount.api = api
	crud.EXPECT().UpdateOne(ctx, odhSystem, sm2, uas).Return(nil)
	odhSystem.crud = crud
	odhSystem.api = api
	assert.NoError(odhAccount.Start(ctx))
	assert.NotEmpty(odhAccount.sysAccountID)
	assert.True(odhAccount.defTenantCreated)
	assert.True(odhAccount.defSubAccountCreated)
	assert.Equal(1, tl.CountPattern("Skipped creating"))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: error finding built-in tenant")
	mockCtrl = gomock.NewController(t)
	dSystem.SystemTags = StringList{common.SystemTagDefTenantCreated}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return("dbName")
	api.EXPECT().MustBeInitializing().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhAccount, findTenantArg, newAccountMatcher(t, mockAccountFind, dTenant)).Return(errWrappedError)
	odhAccount.crud = crud
	odhAccount.sysAccountID = "sysAccountID"
	odhAccount.defSubAccountCreated = false
	odhAccount.api = api
	odhSystem.api = api
	assert.Equal(errWrappedError, odhAccount.Start(ctx))
	assert.NotEmpty(odhAccount.sysAccountID)
	assert.True(odhAccount.defTenantCreated)
	assert.False(odhAccount.defSubAccountCreated)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Start, error after find")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return("dbName")
	api.EXPECT().MustBeInitializing().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhAccount, findNameArg, newAccountMatcher(t, mockAccountFind, dAccount)).Return(errUnknownError)
	odhAccount.crud = crud
	odhAccount.sysAccountID = ""
	odhAccount.defTenantCreated = true
	odhAccount.defSubAccountCreated = true
	odhAccount.api = api
	assert.Equal(errUnknownError, odhAccount.Start(ctx))
	assert.Empty(odhAccount.sysAccountID)

	tcs := []struct {
		msg    string
		roles  AuthRoleMap
		updErr error // error for update
	}{
		{"system account present and but system user not set", AuthRoleMap{"other": AuthRole{RoleID: "foo"}}, nil},
		{"system account present and but system role not set", AuthRoleMap{"other": AuthRole{RoleID: "foo"}, odhUser.adminUserID: AuthRole{RoleID: "otherrole"}}, nil},
		{"system account present and system user is disabled", AuthRoleMap{"other": AuthRole{RoleID: "foo"}, odhUser.adminUserID: AuthRole{RoleID: "sysRoleId", Disabled: true}}, nil},
		{"error after update", AuthRoleMap{"other": AuthRole{RoleID: "foo"}, odhUser.adminUserID: AuthRole{RoleID: "otherrole"}}, errUnknownError},
	}
	for _, tc := range tcs {
		tl.Flush()
		mockCtrl.Finish()

		t.Log("case: Start,", tc.msg)
		mockCtrl = gomock.NewController(t)
		dAccount.MetaObjID = "accountID"
		dAccount.UserRoles = tc.roles
		var uAccount *Account
		testutils.Clone(dAccount, &uAccount)
		uAccount.AccountRoles = ObjIDList{} // these first 3 fields are immutable, they will be defaulted in intUpdate
		uAccount.Messages = []TimestampedString{}
		uAccount.Tags = StringList{}
		uAccount.UserRoles[odhUser.adminUserID] = AuthRole{RoleID: "sysRoleId"}
		m := accountMatcher(t, mockAccountUpdate).CtxObj(uAccount).Return(dAccount).Matcher()
		ua := &ds.UpdateArgs{
			ID:      dAccount.MetaObjID,
			Version: dAccount.MetaVersion,
			Attributes: []ds.UpdateAttr{
				{Name: "UserRoles", Actions: [ds.NumActionTypes]ds.UpdateActionArgs{ds.UpdateSet: ds.UpdateActionArgs{Fields: map[string]struct{}{odhUser.adminUserID: struct{}{}}}}},
			},
		}
		api = NewMockDBAPI(mockCtrl)
		api.EXPECT().DBName().Return("dbName")
		api.EXPECT().MustBeInitializing().Return(nil)
		crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
		crud.EXPECT().FindOne(ctx, odhAccount, findNameArg, newAccountMatcher(t, mockAccountFind, dAccount)).Return(nil)
		crud.EXPECT().UpdateOne(ctx, odhAccount, m, ua).Return(tc.updErr)
		odhAccount.crud = crud
		odhAccount.sysAccountID = ""
		odhAccount.api = api
		err := odhAccount.Start(ctx)
		assert.Equal(tc.updErr, err)
		if tc.updErr == nil {
			assert.Equal(dAccount.MetaObjID, odhAccount.sysAccountID)
		} else {
			assert.Empty(odhAccount.sysAccountID)
		}
	}
}

func TestAccountCount(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	t.Log("case: success")
	ctx := context.Background()
	params := account.AccountListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhAccount, fArg, uint(0)).Return(2, nil)
	odhAccount.crud = crud
	odhAccount.api = api
	odhAccount.log = l
	retMA, retErr := odhAccount.Count(ctx, params, 0)
	assert.Equal(2, retMA)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: count fails")
	mockCtrl = gomock.NewController(t)
	params = account.AccountListParams{
		Name: swag.String("account1"),
		Tags: []string{"tag1", "tag2"},
	}
	fArg = bson.M{
		"name": *params.Name,
		"tags": bson.M{"$all": params.Tags},
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().Count(ctx, odhAccount, fArg, uint(5)).Return(0, errWrappedError)
	odhAccount.crud = crud
	odhAccount.api = api
	odhAccount.log = l
	retMA, retErr = odhAccount.Count(ctx, params, 5)
	assert.Zero(retMA)
	assert.Equal(errWrappedError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhAccount.api = api
	odhAccount.log = l
	retMA, retErr = odhAccount.Count(ctx, params, 0)
	assert.Equal(errWrappedError, retErr)
	assert.Zero(retMA)
}

func TestAccountCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	// fake System object in cache
	dSystem := &System{
		ObjMeta: ObjMeta{
			MetaObjID: "systemID",
		},
		SnapshotManagementPolicy: SnapshotManagementPolicy{
			RetentionDurationSeconds: 99,
		},
		VsrManagementPolicy: VsrManagementPolicy{
			RetentionDurationSeconds: 100,
			NoDelete:                 false,
		},
	}
	odhSystem.systemObj = dSystem
	defer func() { odhSystem.systemObj = nil }()

	dAccount := &Account{
		Name:        "testaccount",
		Description: "test description",
		Tags:        []string{"tag1", "tag2"},
		UserRoles: AuthRoleMap{
			ds.SystemUser: AuthRole{RoleID: ds.SystemAdminRole},
		},
	}
	dAccount.SnapshotCatalogPolicy.Inherited = true
	dAccount.SnapshotManagementPolicy.Inherited = true
	dAccount.VsrManagementPolicy.Inherited = true
	mAccount := dAccount.ToModel()
	mAccount.Meta = &models.ObjMeta{}
	assert.Zero(mAccount.Meta.ID)
	assert.Zero(mAccount.Meta.Version)
	dAccount.FromModel(mAccount) // initialize empty properties
	assert.True(dAccount.SnapshotCatalogPolicy.Inherited)
	assert.True(dAccount.SnapshotManagementPolicy.Inherited)
	assert.True(dAccount.VsrManagementPolicy.Inherited)
	assert.Nil(mAccount.SnapshotCatalogPolicy)
	assert.Nil(mAccount.SnapshotManagementPolicy)
	assert.Nil(mAccount.VsrManagementPolicy)

	var retAccount *Account
	testutils.Clone(dAccount, &retAccount)
	retAccount.AccountRoles = ObjIDList{}
	retAccount.Messages = []TimestampedString{}
	ctx := context.Background()

	t.Log("case: success with nil smp, vsr-mp")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().InsertOne(ctx, odhAccount, newAccountMatcher(t, mockAccountInsert, dAccount)).Return(nil)
	odhAccount.crud = crud
	odhAccount.api = api
	odhAccount.log = l
	retMA, retErr := odhAccount.Create(ctx, mAccount)
	assert.NoError(retErr)
	assert.NotZero(retMA.Meta.ID)
	assert.Equal(models.ObjVersion(1), retMA.Meta.Version)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: success with empty smp, vsr-mp")
	mockCtrl = gomock.NewController(t)
	mAccount.SnapshotCatalogPolicy = &models.SnapshotCatalogPolicy{}
	mAccount.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{}
	mAccount.VsrManagementPolicy = &models.VsrManagementPolicy{}
	retAccount.SnapshotCatalogPolicy = SnapshotCatalogPolicy{}
	retAccount.SnapshotManagementPolicy = SnapshotManagementPolicy{}
	retAccount.VsrManagementPolicy = VsrManagementPolicy{}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().InsertOne(ctx, odhAccount, newAccountMatcher(t, mockAccountInsert, retAccount)).Return(nil)
	odhAccount.crud = crud
	odhAccount.api = api
	odhAccount.log = l
	retMA, retErr = odhAccount.Create(ctx, mAccount)
	assert.NoError(retErr)
	assert.NotZero(retMA.Meta.ID)
	assert.Equal(models.ObjVersion(1), retMA.Meta.Version)
	assert.False(mAccount.SnapshotCatalogPolicy.Inherited)
	assert.False(mAccount.SnapshotManagementPolicy.Inherited)
	assert.False(mAccount.VsrManagementPolicy.Inherited)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: insert error (with policies)")
	mockCtrl = gomock.NewController(t)
	mAccount.SnapshotCatalogPolicy = &models.SnapshotCatalogPolicy{
		CspDomainID: "foo",
	}
	dAccount.SnapshotCatalogPolicy = SnapshotCatalogPolicy{
		CspDomainID: "foo",
	}
	mAccount.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{
		RetentionDurationSeconds: swag.Int32(1),
	}
	dAccount.SnapshotManagementPolicy = SnapshotManagementPolicy{
		RetentionDurationSeconds: 1,
	}
	mAccount.VsrManagementPolicy = &models.VsrManagementPolicy{
		RetentionDurationSeconds: swag.Int32(100),
		NoDelete:                 false,
	}
	dAccount.VsrManagementPolicy = VsrManagementPolicy{
		RetentionDurationSeconds: 100,
		NoDelete:                 false,
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().InsertOne(ctx, odhAccount, newAccountMatcher(t, mockAccountInsert, dAccount)).Return(ds.ErrorExists)
	odhAccount.crud = crud
	odhAccount.api = api
	odhAccount.log = l
	retMA, retErr = odhAccount.Create(ctx, mAccount)
	assert.Equal(ds.ErrorExists, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhAccount.api = api
	odhAccount.log = l
	retMA, retErr = odhAccount.Create(ctx, mAccount)
	assert.Equal(errWrappedError, retErr)
	assert.Zero(retMA)
}

func TestAccountDelete(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	mID := "c72bcfb7-90d4-4276-89ef-aa4bacd09ec7"
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().DeleteOne(ctx, odhAccount, mID).Return(nil)
	odhAccount.crud = crud
	odhAccount.api = api
	odhAccount.log = l
	retErr := odhAccount.Delete(ctx, mID)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().DeleteOne(ctx, odhAccount, mID).Return(errUnknownError)
	odhAccount.crud = crud
	odhAccount.api = api
	odhAccount.log = l
	retErr = odhAccount.Delete(ctx, mID)
	assert.Equal(errUnknownError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhAccount.api = api
	odhAccount.log = l
	retErr = odhAccount.Delete(ctx, mID)
	assert.Equal(errWrappedError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	// case: attempt to remove the system account
	mockCtrl = gomock.NewController(t)
	odhAccount.api = nil
	odhAccount.log = l
	odhAccount.sysAccountID = "SysAccountID"
	retErr = odhAccount.Delete(ctx, odhAccount.sysAccountID)
	assert.Equal(ds.ErrorUnauthorizedOrForbidden, retErr)
}

func TestAccountFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	// fake System object in cache
	dSystem := &System{
		ObjMeta: ObjMeta{
			MetaObjID: "systemID",
		},
		SnapshotManagementPolicy: SnapshotManagementPolicy{
			RetentionDurationSeconds: 99,
			Inherited:                false,
		},
		VsrManagementPolicy: VsrManagementPolicy{
			RetentionDurationSeconds: 100,
			NoDelete:                 false,
			Inherited:                false,
		},
	}
	odhSystem.systemObj = dSystem
	defer func() { odhSystem.systemObj = nil }()

	mTime := time.Now()
	cTime := mTime.AddDate(0, 0, -1)
	dAccount := &Account{
		ObjMeta: ObjMeta{
			MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
			MetaTimeCreated:  cTime,
			MetaTimeModified: mTime,
		},
		Name:        "testaccount",
		Description: "test description",
		Tags:        []string{"tag1", "tag2"},
		SnapshotCatalogPolicy: SnapshotCatalogPolicy{
			CspDomainID: "foo",
		},
		SnapshotManagementPolicy: SnapshotManagementPolicy{
			RetentionDurationSeconds: 99,
		},
		VsrManagementPolicy: VsrManagementPolicy{
			RetentionDurationSeconds: 100,
			NoDelete:                 false,
		},
	}
	mAccount := dAccount.ToModel()
	fArg := bson.M{objKey: dAccount.ObjMeta.MetaObjID}
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhAccount, fArg, newAccountMatcher(t, mockAccountFind, dAccount)).Return(nil)
	odhAccount.crud = crud
	odhAccount.api = api
	odhAccount.log = l
	retMA, retErr := odhAccount.Fetch(ctx, dAccount.ObjMeta.MetaObjID)
	assert.NoError(retErr)
	assert.Equal(mAccount, retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: success (Snapshot policies and the VSR purge policies are nil)")
	mockCtrl = gomock.NewController(t)
	dAccount.SnapshotCatalogPolicy = SnapshotCatalogPolicy{
		Inherited: true,
	}
	dAccount.SnapshotManagementPolicy = SnapshotManagementPolicy{
		Inherited: true,
	}
	dAccount.VsrManagementPolicy = VsrManagementPolicy{
		Inherited: true,
	}
	mAccount.SnapshotCatalogPolicy = nil
	mAccount.SnapshotManagementPolicy = nil
	mAccount.VsrManagementPolicy = nil
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhAccount, fArg, newAccountMatcher(t, mockAccountFind, dAccount)).Return(nil)
	odhAccount.crud = crud
	odhAccount.api = api
	odhAccount.log = l
	retMA, retErr = odhAccount.Fetch(ctx, dAccount.ObjMeta.MetaObjID)
	assert.NoError(retErr)
	assert.Equal(mAccount, retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhAccount, fArg, newAccountMatcher(t, mockAccountFind, dAccount)).Return(ds.ErrorNotFound)
	odhAccount.crud = crud
	odhAccount.api = api
	odhAccount.log = l
	retMA, retErr = odhAccount.Fetch(ctx, dAccount.ObjMeta.MetaObjID)
	assert.Equal(ds.ErrorNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhAccount.api = api
	odhAccount.log = l
	retMA, retErr = odhAccount.Fetch(ctx, dAccount.ObjMeta.MetaObjID)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestAccountList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	// fake System object in cache
	dSystem := &System{
		ObjMeta: ObjMeta{
			MetaObjID: "systemID",
		},
		SnapshotManagementPolicy: SnapshotManagementPolicy{
			RetentionDurationSeconds: 99,
			Inherited:                false,
		},
		VsrManagementPolicy: VsrManagementPolicy{
			RetentionDurationSeconds: 100,
			NoDelete:                 false,
			Inherited:                false,
		},
	}
	odhSystem.systemObj = dSystem
	defer func() { odhSystem.systemObj = nil }()

	now := time.Now()
	dAccounts := []interface{}{
		&Account{
			ObjMeta: ObjMeta{
				MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
				MetaTimeCreated:  now.AddDate(-1, -2, -1),
				MetaTimeModified: now.AddDate(-1, 0, 0),
			},
			Name:        "account1",
			Description: "account1 description",
			Tags:        []string{"tag1", "tag2"},
			SnapshotCatalogPolicy: SnapshotCatalogPolicy{
				CspDomainID:        "csp-1",
				ProtectionDomainID: "pd-1",
			},
			SnapshotManagementPolicy: SnapshotManagementPolicy{
				RetentionDurationSeconds: 88,
			},
			VsrManagementPolicy: VsrManagementPolicy{
				RetentionDurationSeconds: 90,
				NoDelete:                 true,
			},
		},
		&Account{
			ObjMeta: ObjMeta{
				MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
				MetaTimeCreated:  now.AddDate(0, -2, -1),
				MetaTimeModified: now.AddDate(0, -1, -2),
			},
			Name:        "account2",
			Description: "account2 description",
			SnapshotCatalogPolicy: SnapshotCatalogPolicy{
				Inherited: true,
			},
			SnapshotManagementPolicy: SnapshotManagementPolicy{
				Inherited: true,
			},
			VsrManagementPolicy: VsrManagementPolicy{
				Inherited: true,
			},
		},
	}
	ctx := context.Background()

	t.Log("case: success")
	params := account.AccountListParams{
		Name:        swag.String("account1"),
		Tags:        []string{"tag1", "tag2"},
		CspDomainID: swag.String("cspDomainId"),
	}
	fArg := bson.M{
		"name": *params.Name,
		"tags": bson.M{"$all": params.Tags},
		"$or": []bson.M{
			bson.M{"protectiondomains.cspDomainId": bson.M{"$exists": true}},
			bson.M{"snapshotcatalogpolicy.cspdomainid": "cspDomainId"},
		},
	}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhAccount, params, fArg, newConsumeObjFnMatcher(t, dAccounts...)).Return(nil)
	odhAccount.crud = crud
	odhAccount.api = api
	odhAccount.log = l
	retMA, retErr := odhAccount.List(ctx, params)
	assert.NoError(retErr)
	assert.Len(retMA, len(dAccounts))
	for i, mA := range retMA {
		o := dAccounts[i].(*Account)
		m := o.ToModel()
		assert.Equal(m, mA)
	}
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	params = account.AccountListParams{
		Name:            swag.String("account1"),
		Tags:            []string{"tag1", "tag2"},
		TenantAccountID: swag.String("accountId1"),
		UserID:          swag.String("userId1"),
		AccountSecret:   swag.String("secret"),
		CspDomainID:     swag.String("cspDomainId"), // will be ignored
	}
	fArg = bson.M{
		"name":              *params.Name,
		"tags":              bson.M{"$all": params.Tags},
		"tenantaccountid":   *params.TenantAccountID,
		"userroles.userId1": bson.M{"$exists": true},
		"secrets.secret":    bson.M{"$exists": true},
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhAccount, params, fArg, gomock.Any()).Return(errWrappedError)
	odhAccount.crud = crud
	odhAccount.api = api
	odhAccount.log = l
	retMA, retErr = odhAccount.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhAccount.api = api
	odhAccount.log = l
	retMA, retErr = odhAccount.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestAccountUpdate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	// fake System object in cache
	dSystem := &System{
		ObjMeta: ObjMeta{
			MetaObjID: "systemID",
		},
		SnapshotManagementPolicy: SnapshotManagementPolicy{
			RetentionDurationSeconds: 99,
			Inherited:                false,
		},
		VsrManagementPolicy: VsrManagementPolicy{
			RetentionDurationSeconds: 100,
			NoDelete:                 false,
			Inherited:                false,
		},
	}
	odhSystem.systemObj = dSystem
	defer func() { odhSystem.systemObj = nil }()

	roles := odhRole.roles
	odhRole.roles = map[string]*models.Role{}
	odhRole.roles[ds.SystemAdminRole] = &models.Role{RoleAllOf0: models.RoleAllOf0{Meta: &models.ObjMeta{ID: "sysRoleId"}}}
	adminID := odhUser.adminUserID
	odhUser.adminUserID = "adminId"
	defer func() {
		odhRole.roles = roles
		odhUser.adminUserID = adminID
	}()

	ua := &ds.UpdateArgs{
		ID:      "c72bcfb7-90d4-4276-89ef-aa4bacd09ec7",
		Version: 0,
		Attributes: []ds.UpdateAttr{
			{
				Name: "Name",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
			{
				Name: "UserRoles",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	param := &models.AccountMutable{
		Name:        "newName",
		Description: "description",
		Disabled:    true,
		Tags:        []string{"tag1", "tag2"},
		UserRoles: map[string]models.AuthRole{
			"someUserID":        models.AuthRole{RoleID: "someRoleID"},
			odhUser.adminUserID: models.AuthRole{RoleID: "someRoleID"},
		},
		SnapshotCatalogPolicy:    &models.SnapshotCatalogPolicy{},
		SnapshotManagementPolicy: &models.SnapshotManagementPolicy{},
		VsrManagementPolicy:      nil,
	}
	mObj := &models.Account{AccountMutable: *param}
	dObj := &Account{}
	dObj.FromModel(mObj)
	now := time.Now()
	dAccount := &Account{
		ObjMeta: ObjMeta{
			MetaObjID:        ua.ID,
			MetaTimeCreated:  now.AddDate(0, -2, -1),
			MetaTimeModified: now,
			MetaVersion:      1000,
		},
		SnapshotCatalogPolicy:    SnapshotCatalogPolicy{Inherited: false},
		SnapshotManagementPolicy: SnapshotManagementPolicy{Inherited: false},
	}
	m := accountMatcher(t, mockAccountUpdate).CtxObj(dObj).Return(dAccount).Matcher()
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhAccount, m, ua).Return(nil)
	odhAccount.crud = crud
	odhAccount.api = api
	odhAccount.log = l
	odhAccount.sysAccountID = "SysAccountID"
	retMA, retErr := odhAccount.Update(ctx, ua, param)
	assert.NoError(retErr)
	rObj := dAccount.ToModel()
	assert.Equal(rObj, retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhAccount, m, ua).Return(ds.ErrorIDVerNotFound)
	odhAccount.crud = crud
	odhAccount.api = api
	odhAccount.log = l
	retMA, retErr = odhAccount.Update(ctx, ua, param)
	assert.Equal(ds.ErrorIDVerNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhAccount.api = api
	odhAccount.log = l
	retMA, retErr = odhAccount.Update(ctx, ua, param)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: successful update, system account user role updated")
	odhAccount.sysAccountID = "SysAccountID"
	param.UserRoles[odhUser.adminUserID] = models.AuthRole{RoleID: "sysRoleId"}
	mObj = &models.Account{AccountMutable: *param}
	dObj = &Account{}
	dObj.FromModel(mObj)
	ua = &ds.UpdateArgs{
		ID:      odhAccount.sysAccountID,
		Version: 100,
		Attributes: []ds.UpdateAttr{
			{
				Name: "UserRoles",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	m = accountMatcher(t, mockAccountUpdate).CtxObj(dObj).Return(dAccount).Matcher()
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhAccount, m, ua).Return(nil)
	odhAccount.crud = crud
	odhAccount.api = api
	odhAccount.log = l
	odhAccount.sysAccountID = ua.ID
	retMA, retErr = odhAccount.Update(ctx, ua, param)
	assert.NoError(retErr)
	assert.Equal(rObj, retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: attempt to rename the system account")
	odhAccount.sysAccountID = "SysAccountID"
	ua = &ds.UpdateArgs{
		ID:      odhAccount.sysAccountID,
		Version: 0,
		Attributes: []ds.UpdateAttr{
			{
				Name: "Name",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	odhAccount.crud = nil
	odhAccount.api = nil
	retMA, retErr = odhAccount.intUpdate(ctx, ua, param)
	assert.Equal(ds.ErrorUnauthorizedOrForbidden, retErr)
	tl.Flush()

	t.Log("case: attempt to remove system user")
	odhAccount.sysAccountID = "SysAccountID"
	ua = &ds.UpdateArgs{
		ID:      odhAccount.sysAccountID,
		Version: 0,
		Attributes: []ds.UpdateAttr{
			{
				Name: "UserRoles",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateRemove: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	param.UserRoles = make(map[string]models.AuthRole, 0)
	param.UserRoles[odhUser.adminUserID] = models.AuthRole{}
	odhAccount.api = nil
	retMA, retErr = odhAccount.intUpdate(ctx, ua, param)
	assert.Equal(ds.ErrorUnauthorizedOrForbidden, retErr)
	tl.Flush()

	t.Log("case: attempt to drop system user")
	odhAccount.sysAccountID = "SysAccountID"
	ua = &ds.UpdateArgs{
		ID:      odhAccount.sysAccountID,
		Version: 0,
		Attributes: []ds.UpdateAttr{
			{
				Name: "UserRoles",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	param.UserRoles = make(map[string]models.AuthRole, 0)
	param.UserRoles["notAdmin"] = models.AuthRole{}
	odhAccount.api = nil
	retMA, retErr = odhAccount.intUpdate(ctx, ua, param)
	assert.Equal(ds.ErrorUnauthorizedOrForbidden, retErr)
	tl.Flush()

	t.Log("case: attempt to drop system role")
	odhAccount.sysAccountID = "SysAccountID"
	ua = &ds.UpdateArgs{
		ID:      odhAccount.sysAccountID,
		Version: 0,
		Attributes: []ds.UpdateAttr{
			{
				Name: "UserRoles",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	param.UserRoles = make(map[string]models.AuthRole, 0)
	param.UserRoles[odhUser.adminUserID] = models.AuthRole{}
	odhAccount.api = nil
	retMA, retErr = odhAccount.intUpdate(ctx, ua, param)
	assert.Equal(ds.ErrorUnauthorizedOrForbidden, retErr)
	tl.Flush()

	t.Log("case: attempt to disable system role (set body)")
	odhAccount.sysAccountID = "SysAccountID"
	ua = &ds.UpdateArgs{
		ID:      odhAccount.sysAccountID,
		Version: 0,
		Attributes: []ds.UpdateAttr{
			{
				Name: "UserRoles",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	param.UserRoles = make(map[string]models.AuthRole, 0)
	param.UserRoles[odhUser.adminUserID] = models.AuthRole{RoleID: "sysRoleId", Disabled: true}
	odhAccount.api = nil
	retMA, retErr = odhAccount.intUpdate(ctx, ua, param)
	assert.Equal(ds.ErrorUnauthorizedOrForbidden, retErr)
	tl.Flush()

	t.Log("case: attempt to disable system role (append body)")
	odhAccount.sysAccountID = "SysAccountID"
	ua = &ds.UpdateArgs{
		ID:      odhAccount.sysAccountID,
		Version: 0,
		Attributes: []ds.UpdateAttr{
			{
				Name: "UserRoles",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateAppend: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	param.UserRoles = make(map[string]models.AuthRole, 0)
	param.UserRoles[odhUser.adminUserID] = models.AuthRole{RoleID: "sysRoleId", Disabled: true}
	odhAccount.api = nil
	retMA, retErr = odhAccount.intUpdate(ctx, ua, param)
	assert.Equal(ds.ErrorUnauthorizedOrForbidden, retErr)
	tl.Flush()

	t.Log("case: attempt to disable system role (set field)")
	mockCtrl = gomock.NewController(t)
	odhAccount.sysAccountID = "SysAccountID"
	ua = &ds.UpdateArgs{
		ID:      odhAccount.sysAccountID,
		Version: 0,
		Attributes: []ds.UpdateAttr{
			{
				Name: "UserRoles",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						Fields: map[string]struct{}{odhUser.adminUserID: struct{}{}},
					},
				},
			},
		},
	}
	param.UserRoles = make(map[string]models.AuthRole, 0)
	param.UserRoles[odhUser.adminUserID] = models.AuthRole{RoleID: "sysRoleId", Disabled: true}
	odhAccount.api = nil
	retMA, retErr = odhAccount.intUpdate(ctx, ua, param)
	assert.Equal(ds.ErrorUnauthorizedOrForbidden, retErr)
}
