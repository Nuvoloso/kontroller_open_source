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
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/user"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/x/bsonx"
	"golang.org/x/crypto/bcrypt"
)

// gomock.Matcher for User
type mockUserMatchCtx int

const (
	mockUserInvalid mockUserMatchCtx = iota
	mockUserInsert
	mockUserFind
	mockUserUpdate
)

type mockUserMatcher struct {
	t      *testing.T
	ctx    mockUserMatchCtx
	ctxObj *User
	retObj *User
}

func newUserMatcher(t *testing.T, ctx mockUserMatchCtx, obj *User) gomock.Matcher {
	return userMatcher(t, ctx).CtxObj(obj).Matcher()
}

// userMatcher creates a partially initialized mockUserMatcher
func userMatcher(t *testing.T, ctx mockUserMatchCtx) *mockUserMatcher {
	return &mockUserMatcher{t: t, ctx: ctx}
}

// Matcher ends the chain
func (o *mockUserMatcher) Matcher() gomock.Matcher {
	if o.ctx == mockUserInsert || o.ctx == mockUserFind || o.ctx == mockUserUpdate {
		assert.NotNil(o.t, o.ctxObj)
	}
	return o
}

// CtxObj adds an User object
func (o *mockUserMatcher) CtxObj(ao *User) *mockUserMatcher {
	o.ctxObj = ao
	return o
}

// Return sets the update result object
func (o *mockUserMatcher) Return(ro *User) *mockUserMatcher {
	o.retObj = ro
	return o
}

// Matches is from gomock.Matcher
func (o *mockUserMatcher) Matches(x interface{}) bool {
	var obj *User
	switch z := x.(type) {
	case *User:
		obj = z
	default:
		panic("Did not get a recognizable object: " + reflect.TypeOf(x).String())
	}
	assert := assert.New(o.t)
	switch o.ctx {
	case mockUserInsert:
		compObj := o.ctxObj
		compObj.ObjMeta = obj.ObjMeta // insert sets meta data
		if compObj.Password != "" || obj.Password != "" {
			pw := compObj.Password // o.ctxObj contains cleartext value
			defer func() { o.ctxObj.Password = pw }()
			assert.NoError(bcrypt.CompareHashAndPassword([]byte(obj.Password), []byte(pw)))
			compObj.Password = obj.Password
		}
		return assert.NotZero(obj.ObjMeta.MetaObjID) &&
			assert.Equal(int32(1), obj.ObjMeta.MetaVersion) &&
			assert.False(obj.ObjMeta.MetaTimeCreated.IsZero()) &&
			assert.False(obj.ObjMeta.MetaTimeModified.IsZero()) &&
			assert.Equal(obj.ObjMeta.MetaTimeCreated, obj.ObjMeta.MetaTimeModified) &&
			assert.Equal(compObj, obj)
	case mockUserFind:
		*obj = *o.ctxObj
		return true
	case mockUserUpdate:
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
func (o *mockUserMatcher) String() string {
	switch o.ctx {
	case mockUserInsert:
		return "matches on insert"
	}
	return "unknown context"
}

func TestUserHandler(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	assert.Equal("user", odhUser.CName())
	indexes := odhUser.Indexes()
	expIndexes := []mongo.IndexModel{
		{Keys: bsonx.Doc{{Key: objKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
		{Keys: bsonx.Doc{{Key: authIdentifierKey, Value: IndexAscending}}, Options: options.Index().SetUnique(true)},
	}
	assert.Equal(expIndexes, indexes)
	assert.Equal(indexes, odhUser.indexes)
	dIf := odhUser.NewObject()
	assert.NotNil(dIf)
	_, ok := dIf.(*User)
	assert.True(ok)

	t.Log("case: Claim + Initialize success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ctx := context.Background()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	odhUser.Claim(api, crud)
	assert.Equal(api, odhUser.api)
	assert.Equal(crud, odhUser.crud)
	assert.Equal(l, odhUser.log)
	assert.Equal(odhUser, odhUser.Ops())

	findIDArg := bson.M{authIdentifierKey: ds.SystemUser}
	iUser := &User{
		ObjMeta:        ObjMeta{MetaObjID: "adminId"},
		AuthIdentifier: "admin",
		Disabled:       false,
		Password:       "admin", // cleartext, compared with the hash
		Profile: StringValueMap{
			"userName": ValueType{Kind: "STRING", Value: "admin"},
		},
	}
	pw, err := bcrypt.GenerateFromPassword([]byte(iUser.Password), 0)
	if !assert.NoError(err) {
		assert.FailNow("bcrypt.GenerateFromPassword must not fail")
	}
	dUser := &User{
		ObjMeta:        ObjMeta{MetaObjID: "adminId"},
		AuthIdentifier: "admin",
		Disabled:       false,
		Password:       string(pw),
		Profile: StringValueMap{
			"userName": ValueType{Kind: "STRING", Value: "admin"},
		},
	}
	crud.EXPECT().CreateIndexes(ctx, odhUser).Return(nil)
	crud.EXPECT().FindOne(ctx, odhUser, findIDArg, newUserMatcher(t, mockUserFind, dUser)).Return(nil)
	odhUser.adminUserID = ""
	assert.NoError(odhUser.Initialize(ctx))
	assert.Equal("adminId", odhUser.adminUserID)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: already initialized success")
	assert.NoError(odhUser.ensureBuiltInUsers(ctx))
	assert.Equal("adminId", odhUser.adminUserID)
	tl.Flush()

	t.Log("case: Initialize, error creating indexes")
	mockCtrl = gomock.NewController(t)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhUser).Return(errUnknownError)
	odhUser.crud = crud
	odhUser.adminUserID = ""
	odhUser.api = api
	assert.Equal(errUnknownError, odhUser.Initialize(ctx))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize - System user created")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhUser).Return(nil)
	odhUser.crud = crud
	crud.EXPECT().FindOne(ctx, odhUser, findIDArg, newUserMatcher(t, mockUserFind, dUser)).Return(ds.ErrorNotFound)
	crud.EXPECT().InsertOne(ctx, odhUser, newUserMatcher(t, mockUserInsert, iUser)).Return(nil)
	odhUser.adminUserID = ""
	odhUser.api = api
	assert.NoError(odhUser.Initialize(ctx))
	assert.NotEmpty(odhUser.adminUserID)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: GenerateFromPassword failure")
	mockCtrl = gomock.NewController(t)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhUser, findIDArg, newUserMatcher(t, mockUserFind, dUser)).Return(ds.ErrorNotFound)
	odhUser.crud = crud
	odhUser.adminUserID = ""
	defer func() { cryptPWHook = bcrypt.GenerateFromPassword }()
	cryptPWHook = func(password []byte, cost int) ([]byte, error) {
		return nil, errUnknownError
	}
	assert.Equal(errUnknownError, odhUser.ensureBuiltInUsers(ctx))
	cryptPWHook = bcrypt.GenerateFromPassword
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize, error creating system user")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhUser).Return(nil)
	odhUser.crud = crud
	crud.EXPECT().FindOne(ctx, odhUser, findIDArg, newUserMatcher(t, mockUserFind, dUser)).Return(ds.ErrorNotFound)
	crud.EXPECT().InsertOne(ctx, odhUser, newUserMatcher(t, mockUserInsert, iUser)).Return(errWrappedError)
	odhUser.adminUserID = ""
	odhUser.api = api
	assert.Equal(errWrappedError, odhUser.Initialize(ctx))
	assert.Empty(odhUser.adminUserID)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize, find db error")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhUser).Return(nil)
	odhUser.crud = crud
	crud.EXPECT().FindOne(ctx, odhUser, findIDArg, newUserMatcher(t, mockUserFind, dUser)).Return(errUnknownError)
	odhUser.adminUserID = ""
	odhUser.api = api
	assert.Equal(errUnknownError, odhUser.Initialize(ctx))
	assert.Empty(odhUser.adminUserID)
	tl.Flush()
	mockCtrl.Finish()

	tcs := []struct {
		msg    string
		updErr error // error for update
	}{
		{"system user present but disabled", nil},
		{"system user present with bad hashed password", nil},
		{"error in cryptPWHook", nil},
		{"error after update", errUnknownError},
	}
	for i, tc := range tcs {
		t.Log("case: Initialize,", tc.msg)
		mockCtrl = gomock.NewController(t)
		dUser.MetaObjID = "userID"
		dUser.MetaVersion = 8
		dUser.Disabled = true
		expObj := &User{Profile: StringValueMap{}}
		var retObj *User
		testutils.Clone(dUser, &retObj)
		retObj.Disabled = false
		ua := &ds.UpdateArgs{
			ID:      "userID",
			Version: 8,
			Attributes: []ds.UpdateAttr{
				{Name: "Disabled", Actions: [ds.NumActionTypes]ds.UpdateActionArgs{ds.UpdateSet: ds.UpdateActionArgs{FromBody: true}}},
			},
		}
		if i > 0 {
			dUser.Password = ""
			expObj.Password = string(pw)
			cryptPWHook = func(password []byte, cost int) ([]byte, error) {
				return pw, nil
			}
			if i == 1 {
				dUser.Disabled = false
				ua.Attributes = nil
			}
			if i == 2 {
				cryptPWHook = func(password []byte, cost int) ([]byte, error) {
					return nil, errUnknownError
				}
			}
			ua.Attributes = append(ua.Attributes, ds.UpdateAttr{Name: "Password", Actions: [ds.NumActionTypes]ds.UpdateActionArgs{ds.UpdateSet: ds.UpdateActionArgs{FromBody: true}}})
		}
		m := userMatcher(t, mockUserUpdate).CtxObj(expObj).Return(retObj).Matcher()
		api = NewMockDBAPI(mockCtrl)
		crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
		crud.EXPECT().CreateIndexes(ctx, odhUser).Return(nil)
		odhUser.crud = crud
		crud.EXPECT().FindOne(ctx, odhUser, findIDArg, newUserMatcher(t, mockUserFind, dUser)).Return(nil)
		if i != 2 {
			crud.EXPECT().UpdateOne(ctx, odhUser, m, ua).Return(tc.updErr)
		}
		odhUser.adminUserID = ""
		odhUser.api = api
		err := odhUser.Initialize(ctx)
		if tc.updErr == nil && i != 2 {
			assert.NoError(err)
			assert.Equal(dUser.MetaObjID, odhUser.adminUserID)
			if i == 0 {
				assert.Equal(1, tl.CountPattern("Re-enabling user"))
			} else {
				assert.Equal(1, tl.CountPattern("Resetting password"))
			}
		} else {
			assert.Equal(errUnknownError, err)
			assert.Empty(odhUser.adminUserID)
		}
		tl.Flush()
		mockCtrl.Finish()
	}

	t.Log("case: Start")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().DBName().Return("dbName")
	odhUser.api = api
	assert.NoError(odhUser.Start(ctx))
}

func TestUserCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	dUser := &User{
		AuthIdentifier: "ann.user@nuvoloso.com",
		Disabled:       false,
		Profile: StringValueMap{
			"userName": ValueType{Kind: "STRING", Value: "ann user"},
			"attr2":    ValueType{Kind: "INT", Value: "1"},
		},
	}
	mUser := dUser.ToModel()
	mUser.Meta = &models.ObjMeta{}
	assert.Zero(mUser.Meta.ID)
	assert.Zero(mUser.Meta.Version)
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().InsertOne(ctx, odhUser, newUserMatcher(t, mockUserInsert, dUser)).Return(nil)
	odhUser.crud = crud
	odhUser.api = api
	odhUser.log = l
	retMA, retErr := odhUser.Create(ctx, mUser)
	assert.NoError(retErr)
	assert.NotZero(retMA.Meta.ID)
	assert.Equal(models.ObjVersion(1), retMA.Meta.Version)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: duplicate")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().InsertOne(ctx, odhUser, newUserMatcher(t, mockUserInsert, dUser)).Return(ds.ErrorExists)
	odhUser.crud = crud
	odhUser.api = api
	odhUser.log = l
	retMA, retErr = odhUser.Create(ctx, mUser)
	assert.Equal(ds.ErrorExists, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhUser.api = api
	odhUser.log = l
	retMA, retErr = odhUser.Create(ctx, mUser)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestUserDelete(t *testing.T) {
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
	crud.EXPECT().DeleteOne(ctx, odhUser, mID).Return(nil)
	odhUser.crud = crud
	odhUser.api = api
	odhUser.log = l
	retErr := odhUser.Delete(ctx, mID)
	assert.NoError(retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not found")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().DeleteOne(ctx, odhUser, mID).Return(ds.ErrorNotFound)
	odhUser.crud = crud
	odhUser.api = api
	odhUser.log = l
	retErr = odhUser.Delete(ctx, mID)
	assert.Equal(ds.ErrorNotFound, retErr)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhUser.api = api
	odhUser.log = l
	retErr = odhUser.Delete(ctx, mID)
	assert.Equal(errWrappedError, retErr)
	tl.Flush()
	mockCtrl.Finish()

	// case: attempt to remove the system user
	mockCtrl = gomock.NewController(t)
	odhUser.api = nil
	odhUser.log = l
	odhUser.adminUserID = "SysUserID"
	retErr = odhUser.Delete(ctx, odhUser.adminUserID)
	assert.Equal(ds.ErrorUnauthorizedOrForbidden, retErr)
}

func TestUserFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mTime := time.Now()
	cTime := mTime.AddDate(0, 0, -1)
	dUser := &User{
		ObjMeta: ObjMeta{
			MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
			MetaTimeCreated:  cTime,
			MetaTimeModified: mTime,
		},
		AuthIdentifier: "ann.user@nuvoloso.com",
		Disabled:       false,
		Profile: StringValueMap{
			"userName": ValueType{Kind: "STRING", Value: "ann user"},
			"attr2":    ValueType{Kind: "INT", Value: "1"},
		},
	}
	fArg := bson.M{objKey: dUser.ObjMeta.MetaObjID}
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhUser, fArg, newUserMatcher(t, mockUserFind, dUser)).Return(nil)
	odhUser.crud = crud
	odhUser.api = api
	odhUser.log = l
	retMA, retErr := odhUser.Fetch(ctx, dUser.ObjMeta.MetaObjID)
	assert.NoError(retErr)
	assert.Equal(dUser.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not found")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindOne(ctx, odhUser, fArg, newUserMatcher(t, mockUserFind, dUser)).Return(ds.ErrorNotFound)
	odhUser.crud = crud
	odhUser.api = api
	odhUser.log = l
	retMA, retErr = odhUser.Fetch(ctx, dUser.ObjMeta.MetaObjID)
	assert.Equal(ds.ErrorNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhUser.api = api
	odhUser.log = l
	retMA, retErr = odhUser.Fetch(ctx, dUser.ObjMeta.MetaObjID)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestUserList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	now := time.Now()
	dUsers := []interface{}{
		&User{
			ObjMeta: ObjMeta{
				MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
				MetaTimeCreated:  now.AddDate(-1, -2, -1),
				MetaTimeModified: now.AddDate(-1, 0, 0),
			},
			AuthIdentifier: "ann.user@nuvoloso.com",
			Disabled:       false,
			Profile: StringValueMap{
				"userName": ValueType{Kind: "STRING", Value: "ann user"},
				"attr2":    ValueType{Kind: "INT", Value: "1"},
			},
		},
		&User{
			ObjMeta: ObjMeta{
				MetaObjID:        "11ac2fdc-226e-4e3c-ad34-cb6ed88f65e0",
				MetaTimeCreated:  now.AddDate(0, -2, -1),
				MetaTimeModified: now.AddDate(0, -1, -2),
			},
			AuthIdentifier: "admin",
			Disabled:       false,
			Profile: StringValueMap{
				"userName": ValueType{Kind: "STRING", Value: "default user"},
				"attr2":    ValueType{Kind: "INT", Value: "1"},
			},
		},
	}
	ctx := context.Background()

	t.Log("case: success")
	params := user.UserListParams{}
	fArg := bson.M{}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhUser, params, fArg, newConsumeObjFnMatcher(t, dUsers...)).Return(nil)
	odhUser.crud = crud
	odhUser.api = api
	odhUser.log = l
	retMA, retErr := odhUser.List(ctx, params)
	assert.NoError(retErr)
	assert.Len(retMA, len(dUsers))
	for i, mA := range retMA {
		user := dUsers[i].(*User)
		assert.Equal(user.ToModel(), mA)
	}
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: failure")
	mockCtrl = gomock.NewController(t)
	params = user.UserListParams{
		AuthIdentifier:  swag.String("admin"),
		UserNamePattern: swag.String("pattern"),
	}
	fArg = bson.M{
		"authidentifier":   *params.AuthIdentifier,
		"profile.userName": primitive.Regex{Pattern: *params.UserNamePattern},
	}
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().FindAll(ctx, odhUser, params, fArg, gomock.Any()).Return(errWrappedError)
	odhUser.crud = crud
	odhUser.api = api
	odhUser.log = l
	retMA, retErr = odhUser.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhUser.api = api
	odhUser.log = l
	retMA, retErr = odhUser.List(ctx, params)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestUserUpdate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
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
				Name: "AuthIdentifier",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
			{
				Name: "Disabled",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
			{
				Name: "Profile",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateAppend: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	param := &models.UserMutable{
		AuthIdentifier: "new@nuvoloso.com",
		Disabled:       true,
		Profile: map[string]models.ValueType{
			"userName": models.ValueType{Kind: "STRING", Value: "droid"},
		},
	}
	mObj := &models.User{UserMutable: *param}
	dObj := &User{}
	dObj.FromModel(mObj)
	now := time.Now()
	dUser := &User{
		ObjMeta: ObjMeta{
			MetaObjID:        ua.ID,
			MetaTimeCreated:  now.AddDate(0, -2, -1),
			MetaTimeModified: now,
			MetaVersion:      1000,
		},
	}
	m := userMatcher(t, mockUserUpdate).CtxObj(dObj).Return(dUser).Matcher()
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhUser, m, ua).Return(nil)
	odhUser.crud = crud
	odhUser.api = api
	odhUser.log = l
	retMA, retErr := odhUser.Update(ctx, ua, param)
	assert.NoError(retErr)
	assert.Equal(dUser.ToModel(), retMA)
	tl.Flush()
	mockCtrl.Finish()

	mockCtrl = gomock.NewController(t)
	t.Log("case: failure")
	odhUser.adminUserID = ua.ID
	param.AuthIdentifier = ds.SystemUser // cover unchanged AuthIdentifier and Disabled fields
	param.Disabled = false
	dObj.AuthIdentifier = ds.SystemUser
	dObj.Disabled = false
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhUser, m, ua).Return(ds.ErrorIDVerNotFound)
	odhUser.crud = crud
	odhUser.api = api
	odhUser.log = l
	retMA, retErr = odhUser.Update(ctx, ua, param)
	assert.Equal(ds.ErrorIDVerNotFound, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: modify admin user authIdentifier")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	odhUser.api = api
	odhUser.log = l
	odhUser.adminUserID = ua.ID
	param.AuthIdentifier = "new@one.com"
	param.Disabled = true
	retMA, retErr = odhUser.Update(ctx, ua, param)
	assert.Equal(ds.ErrorUnauthorizedOrForbidden, retErr)
	assert.Nil(retMA)
	assert.Equal(1, tl.CountPattern("Attempt to modify authIdentifier"))
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: disable admin user")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	odhUser.api = api
	odhUser.log = l
	odhUser.adminUserID = ua.ID
	param.AuthIdentifier = ds.SystemUser
	retMA, retErr = odhUser.Update(ctx, ua, param)
	assert.Equal(ds.ErrorUnauthorizedOrForbidden, retErr)
	assert.Nil(retMA)
	assert.Equal(1, tl.CountPattern("Attempt to disable"))
	odhUser.adminUserID = "adminId"
	param.AuthIdentifier = "new@nuvoloso.com"
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(errWrappedError)
	odhUser.api = api
	odhUser.log = l
	retMA, retErr = odhUser.Update(ctx, ua, param)
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}
