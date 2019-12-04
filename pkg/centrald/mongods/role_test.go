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
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/role"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
)

// gomock.Matcher for Role
type mockRoleMatchCtx int

const (
	mockRoleInvalid mockRoleMatchCtx = iota
	mockRoleInsert
	mockRoleFind
	mockRoleUpdate
)

type mockRoleMatcher struct {
	t      *testing.T
	ctx    mockRoleMatchCtx
	ctxObj *Role
	retObj *Role
}

func newRoleMatcher(t *testing.T, ctx mockRoleMatchCtx, obj *Role) gomock.Matcher {
	return roleMatcher(t, ctx).CtxObj(obj).Matcher()
}

// roleMatcher creates a partially initialized mockRoleMatcher
func roleMatcher(t *testing.T, ctx mockRoleMatchCtx) *mockRoleMatcher {
	return &mockRoleMatcher{t: t, ctx: ctx}
}

// Matcher ends the chain
func (o *mockRoleMatcher) Matcher() gomock.Matcher {
	if o.ctx == mockRoleInsert || o.ctx == mockRoleFind || o.ctx == mockRoleUpdate {
		assert.NotNil(o.t, o.ctxObj)
	}
	return o
}

// CtxObj adds an Role object
func (o *mockRoleMatcher) CtxObj(ao *Role) *mockRoleMatcher {
	o.ctxObj = ao
	return o
}

// Return sets the update result object
func (o *mockRoleMatcher) Return(ro *Role) *mockRoleMatcher {
	o.retObj = ro
	return o
}

// Matches is from gomock.Matcher
func (o *mockRoleMatcher) Matches(x interface{}) bool {
	var obj *Role
	switch z := x.(type) {
	case *Role:
		obj = z
	default:
		panic("Did not get a recognizable object: " + reflect.TypeOf(x).String())
	}
	assert := assert.New(o.t)
	switch o.ctx {
	case mockRoleInsert:
		compObj := o.ctxObj
		compObj.ObjMeta = obj.ObjMeta // insert sets meta data
		return assert.NotZero(obj.ObjMeta.MetaObjID) &&
			assert.Equal(int32(1), obj.ObjMeta.MetaVersion) &&
			assert.False(obj.ObjMeta.MetaTimeCreated.IsZero()) &&
			assert.False(obj.ObjMeta.MetaTimeModified.IsZero()) &&
			assert.Equal(obj.ObjMeta.MetaTimeCreated, obj.ObjMeta.MetaTimeModified) &&
			assert.Equal(compObj, obj)
	case mockRoleFind:
		*obj = *o.ctxObj
		return true
	case mockRoleUpdate:
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
func (o *mockRoleMatcher) String() string {
	switch o.ctx {
	case mockRoleInsert:
		return "matches on insert"
	}
	return "unknown context"
}

func TestRoleHandler(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	assert.Equal("role", odhRole.CName())
	indexes := odhRole.Indexes()
	expIndexes := []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: nameKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
	}
	assert.Equal(expIndexes, indexes)
	assert.Equal(indexes, odhRole.indexes)
	dIf := odhRole.NewObject()
	assert.NotNil(dIf)
	_, ok := dIf.(*Role)
	assert.True(ok)

	t.Log("case: Claim + Initialize success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ctx := context.Background()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	odhRole.Claim(api, crud)
	assert.Equal(api, odhRole.api)
	assert.Equal(crud, odhRole.crud)
	assert.Equal(l, odhRole.log)
	assert.Equal(odhRole, odhRole.Ops())
	crud.EXPECT().CreateIndexes(ctx, odhRole).Return(nil)
	api.EXPECT().BaseDataPathName().Return("") // Populate is tested separately
	odhRole.roles = map[string]*models.Role{}
	roleObj := &models.Role{RoleAllOf0: models.RoleAllOf0{Meta: &models.ObjMeta{ID: "NOT NIL"}}}
	odhRole.roles[ds.SystemAdminRole] = roleObj
	odhRole.roles[ds.TenantAdminRole] = roleObj
	odhRole.roles[ds.AccountAdminRole] = roleObj
	odhRole.roles[ds.AccountUserRole] = roleObj
	assert.NoError(odhRole.Initialize(ctx))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize, error creating indexes")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhRole).Return(errUnknownError)
	odhRole.crud = crud
	odhRole.api = api
	assert.Error(errUnknownError, odhRole.Initialize(ctx))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize, error in populateCollection")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	api.EXPECT().BaseDataPathName().Return("bad-path")
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhRole).Return(nil)
	odhRole.crud = crud
	odhRole.api = api
	assert.Error(odhRole.Initialize(ctx))
	tl.Flush()
	mockCtrl.Finish()

	for _, tc := range []string{ds.SystemAdminRole, ds.TenantAdminRole, ds.AccountAdminRole, ds.AccountUserRole} {
		t.Log("case: Initialize, missing role", tc)
		mockCtrl = gomock.NewController(t)
		api = NewMockDBAPI(mockCtrl)
		api.EXPECT().Logger().Return(l).MinTimes(1)
		api.EXPECT().BaseDataPathName().Return("")
		crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
		crud.EXPECT().CreateIndexes(ctx, odhRole).Return(nil)
		odhRole.crud = crud
		delete(odhRole.roles, tc)
		odhRole.api = api
		err := odhRole.Initialize(ctx)
		assert.EqualError(err, "role "+tc+" does not exist")
		odhRole.roles[tc] = roleObj
		mockCtrl.Finish()
	}

	t.Log("case: Start")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return("dbName")
	odhRole.api = api
	assert.NoError(odhRole.Start(ctx))
}

func TestRolePopulate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	dRole := &Role{
		Name: ds.SystemAdminRole,
		Capabilities: CapabilityMap{
			"cap1": true,
			"cap2": false,
		},
	}
	mRole := dRole.ToModel()
	mRole.Meta = &models.ObjMeta{}
	assert.Zero(mRole.Meta.ID)
	assert.Zero(mRole.Meta.Version)
	buf, err := json.Marshal(mRole)
	assert.NoError(err)

	t.Log("case: successful creation")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ctx := context.Background()
	api := NewMockDBAPI(mockCtrl)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhRole, bson.M{"name": ds.SystemAdminRole}, newRoleMatcher(t, mockRoleFind, dRole)).Return(ds.ErrorNotFound)
	crud.EXPECT().InsertOne(ctx, odhRole, newRoleMatcher(t, mockRoleInsert, dRole)).Return(nil)
	odhRole.crud = crud
	odhRole.roles = map[string]*models.Role{}
	odhRole.api = api
	odhRole.log = l
	retErr := odhRole.Populate(ctx, "file1", buf)
	assert.NoError(retErr)
	assert.Len(odhRole.roles, 1)
	assert.NotNil(odhRole.roles[ds.SystemAdminRole])
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: insert error")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhRole, bson.M{"name": ds.SystemAdminRole}, newRoleMatcher(t, mockRoleFind, dRole)).Return(ds.ErrorNotFound)
	crud.EXPECT().InsertOne(ctx, odhRole, newRoleMatcher(t, mockRoleInsert, dRole)).Return(errUnknownError)
	odhRole.crud = crud
	odhRole.roles = map[string]*models.Role{}
	odhRole.api = api
	odhRole.log = l
	retErr = odhRole.Populate(ctx, "file1", buf)
	assert.Equal(errUnknownError, retErr)
	assert.Empty(odhRole.roles)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: find error")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhRole, bson.M{"name": ds.SystemAdminRole}, newRoleMatcher(t, mockRoleFind, dRole)).Return(errUnknownError)
	odhRole.crud = crud
	odhRole.roles = map[string]*models.Role{}
	odhRole.api = api
	odhRole.log = l
	retErr = odhRole.Populate(ctx, "file1", buf)
	assert.Equal(errUnknownError, retErr)
	assert.Empty(odhRole.roles)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: role exists")
	dRole.MetaObjID = "role-id"
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhRole, bson.M{"name": ds.SystemAdminRole}, newRoleMatcher(t, mockRoleFind, dRole)).Return(nil)
	odhRole.crud = crud
	odhRole.roles = map[string]*models.Role{}
	odhRole.api = api
	odhRole.log = l
	retErr = odhRole.Populate(ctx, "file1", buf)
	assert.NoError(retErr)
	assert.Len(odhRole.roles, 1)
	assert.Equal(dRole.ToModel(), odhRole.roles[ds.SystemAdminRole])
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: capabilities updated")
	dRole2 := &Role{
		Name: ds.SystemAdminRole,
		Capabilities: CapabilityMap{
			"cap1": true,
			"cap3": true,
		},
	}
	dRole2.MetaObjID = "role-id"
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhRole, bson.M{"name": ds.SystemAdminRole}, newRoleMatcher(t, mockRoleFind, dRole2)).Return(nil)
	m := roleMatcher(t, mockRoleUpdate).CtxObj(dRole).Return(dRole).Matcher()
	ua := &ds.UpdateArgs{
		ID: dRole.MetaObjID,
		Attributes: []ds.UpdateAttr{
			{Name: "Capabilities", Actions: [ds.NumActionTypes]ds.UpdateActionArgs{ds.UpdateSet: ds.UpdateActionArgs{FromBody: true}}},
		},
	}
	crud.EXPECT().UpdateOne(ctx, odhRole, m, ua).Return(nil)
	odhRole.crud = crud
	odhRole.roles = map[string]*models.Role{}
	odhRole.api = api
	odhRole.log = l
	retErr = odhRole.Populate(ctx, "file1", buf)
	assert.NoError(retErr)
	assert.Len(odhRole.roles, 1)
	if assert.NotNil(odhRole.roles[ds.SystemAdminRole]) {
		assert.Equal(dRole.Capabilities.ToModel(), odhRole.roles[ds.SystemAdminRole].Capabilities)
	}
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: capabilities update error")
	dRole2 = &Role{
		Name: ds.SystemAdminRole,
		Capabilities: CapabilityMap{
			"cap1": true,
			"cap3": true,
		},
	}
	dRole2.MetaObjID = "role-id"
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhRole, bson.M{"name": ds.SystemAdminRole}, newRoleMatcher(t, mockRoleFind, dRole2)).Return(nil)
	crud.EXPECT().UpdateOne(ctx, odhRole, m, ua).Return(errUnknownError)
	odhRole.crud = crud
	odhRole.roles = map[string]*models.Role{}
	odhRole.api = api
	odhRole.log = l
	retErr = odhRole.Populate(ctx, "file1", buf)
	assert.Equal(errUnknownError, retErr)

	// case: unmarshal error
	retErr = odhRole.Populate(ctx, "file1", []byte("not valid json"))
	assert.Regexp("invalid character", retErr)

	// case: no name
	retErr = odhRole.Populate(ctx, "file1", []byte("{}"))
	assert.Regexp("non-empty name is required", retErr)
}

func TestRoleFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mTime := time.Now()
	cTime := mTime.AddDate(0, 0, -1)
	dRole := &Role{
		ObjMeta: ObjMeta{
			MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
			MetaTimeCreated:  cTime,
			MetaTimeModified: mTime,
		},
		Name: ds.SystemAdminRole,
		Capabilities: CapabilityMap{
			"cap1": true,
			"cap2": false,
		},
	}

	t.Log("case: empty cache")
	odhRole.api = nil
	odhRole.log = l
	retMA, retErr := odhRole.Fetch("1234")
	assert.Nil(retMA)
	assert.Equal(ds.ErrorNotFound, retErr)
	tl.Flush()

	t.Log("case: success")
	odhRole.roles = map[string]*models.Role{}
	odhRole.roles[ds.SystemAdminRole] = dRole.ToModel()
	retMA, retErr = odhRole.Fetch(dRole.MetaObjID)
	assert.NoError(retErr)
	assert.Equal(dRole.ToModel(), retMA)

	t.Log("case: not found")
	retMA, retErr = odhRole.Fetch("1234")
	assert.Nil(retMA)
	assert.Equal(ds.ErrorNotFound, retErr)
}

func TestRoleList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	now := time.Now()
	dRoles := []*Role{
		&Role{
			ObjMeta: ObjMeta{
				MetaObjID:        "systemId",
				MetaTimeCreated:  now.AddDate(-1, -2, -1),
				MetaTimeModified: now.AddDate(-1, 0, 0),
			},
			Name: ds.SystemAdminRole,
			Capabilities: CapabilityMap{
				"cap1": true,
				"cap2": true,
			},
		},
		&Role{
			ObjMeta: ObjMeta{
				MetaObjID:        "tenantId",
				MetaTimeCreated:  now.AddDate(0, -2, -1),
				MetaTimeModified: now.AddDate(0, -1, -2),
			},
			Name: ds.TenantAdminRole,
			Capabilities: CapabilityMap{
				"cap1": true,
				"cap2": false,
			},
		},
	}
	odhRole.api = nil
	odhRole.log = l
	odhRole.roles = map[string]*models.Role{
		ds.SystemAdminRole: dRoles[0].ToModel(),
		ds.TenantAdminRole: dRoles[1].ToModel(),
	}

	t.Log("case: success")
	params := role.RoleListParams{}
	retMA, retErr := odhRole.List(params)
	assert.NoError(retErr)
	assert.Len(retMA, 2)
	for _, mA := range retMA {
		if mA.Meta.ID == "systemId" {
			assert.Equal(dRoles[0].ToModel(), mA)
		} else {
			assert.Equal(dRoles[1].ToModel(), mA)
		}
	}

	t.Log("case: pass matching params")
	params = role.RoleListParams{
		Name: swag.String(ds.SystemAdminRole),
	}
	retMA, retErr = odhRole.List(params)
	assert.NoError(retErr)
	assert.Len(retMA, 1)
	assert.Equal(dRoles[0].ToModel(), retMA[0])

	t.Log("case: pass non-matching params")
	params = role.RoleListParams{
		Name: swag.String(ds.AccountAdminRole),
	}
	retMA, retErr = odhRole.List(params)
	assert.NoError(retErr)
	assert.NotNil(retMA)
	assert.Empty(retMA)
}
