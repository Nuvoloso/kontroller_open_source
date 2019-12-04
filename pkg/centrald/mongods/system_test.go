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
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ds "github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
)

// gomock.Matcher for System
type mockSystemMatchCtx int

const (
	mockSystemInvalid mockSystemMatchCtx = iota
	mockSystemInsert
	mockSystemFind
	mockSystemUpdate
)

type mockSystemMatcher struct {
	t      *testing.T
	ctx    mockSystemMatchCtx
	ctxObj *System
	retObj *System
}

func newSystemMatcher(t *testing.T, ctx mockSystemMatchCtx, obj *System) gomock.Matcher {
	return systemMatcher(t, ctx).CtxObj(obj).Matcher()
}

// systemMatcher creates a partially initialized mockSystemMatcher
func systemMatcher(t *testing.T, ctx mockSystemMatchCtx) *mockSystemMatcher {
	return &mockSystemMatcher{t: t, ctx: ctx}
}

// Matcher ends the chain
func (o *mockSystemMatcher) Matcher() gomock.Matcher {
	if o.ctx == mockSystemInsert || o.ctx == mockSystemFind || o.ctx == mockSystemUpdate {
		assert.NotNil(o.t, o.ctxObj)
	}
	return o
}

// CtxObj adds an System object
func (o *mockSystemMatcher) CtxObj(ao *System) *mockSystemMatcher {
	o.ctxObj = ao
	return o
}

// Return sets the update result object
func (o *mockSystemMatcher) Return(ro *System) *mockSystemMatcher {
	o.retObj = ro
	return o
}

// Matches is from gomock.Matcher
func (o *mockSystemMatcher) Matches(x interface{}) bool {
	var obj *System
	switch z := x.(type) {
	case *System:
		obj = z
	default:
		panic("Did not get a recognizable object: " + reflect.TypeOf(x).String())
	}
	assert := assert.New(o.t)
	switch o.ctx {
	case mockSystemInsert:
		compObj := o.ctxObj
		compObj.ObjMeta = obj.ObjMeta // insert sets meta data
		return assert.NotZero(obj.ObjMeta.MetaObjID) &&
			assert.Equal(int32(1), obj.ObjMeta.MetaVersion) &&
			assert.False(obj.ObjMeta.MetaTimeCreated.IsZero()) &&
			assert.False(obj.ObjMeta.MetaTimeModified.IsZero()) &&
			assert.Equal(obj.ObjMeta.MetaTimeCreated, obj.ObjMeta.MetaTimeModified) &&
			assert.Equal(compObj, obj)
	case mockSystemFind:
		*obj = *o.ctxObj
		return true
	case mockSystemUpdate:
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
func (o *mockSystemMatcher) String() string {
	switch o.ctx {
	case mockSystemInsert:
		return "matches on insert"
	}
	return "unknown context"
}

func TestSystemHandler(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()
	dSystem := &System{
		Name:                  SystemObjName,
		Description:           SystemObjDescription,
		SnapshotCatalogPolicy: SnapshotCatalogPolicy{},
		SnapshotManagementPolicy: SnapshotManagementPolicy{
			RetentionDurationSeconds: SystemObjSmpRDS,
		},
		VsrManagementPolicy: VsrManagementPolicy{
			RetentionDurationSeconds: SystemObjVSRmpRDS,
		},
		ClusterUsagePolicy: ClusterUsagePolicy{
			AccountSecretScope:          common.AccountSecretScopeCluster,
			ConsistencyGroupName:        common.ConsistencyGroupNameDefault,
			VolumeDataRetentionOnDelete: common.VolumeDataRetentionOnDeleteRetain,
		},
		UserPasswordPolicy: UserPasswordPolicy{
			MinLength: SystemObjMinPasswordLength,
		},
		SystemTags: StringList{},
	}

	assert.Equal("system", odhSystem.CName())
	assert.Nil(odhSystem.Indexes())
	dIf := odhSystem.NewObject()
	assert.NotNil(dIf)
	_, ok := dIf.(*System)
	assert.True(ok)

	t.Log("case: Claim + Initialize - System object created")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ctx := context.Background()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().Logger().Return(l).MinTimes(1)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhSystem).Return(nil)
	crud.EXPECT().FindOne(ctx, odhSystem, bson.M{}, newSystemMatcher(t, mockSystemFind, dSystem)).Return(ds.ErrorNotFound)
	crud.EXPECT().InsertOne(ctx, odhSystem, newSystemMatcher(t, mockSystemInsert, dSystem)).Return(nil)
	odhSystem.systemObj = nil
	odhSystem.Claim(api, crud)
	assert.Equal(api, odhSystem.api)
	assert.Equal(crud, odhSystem.crud)
	assert.Equal(l, odhSystem.log)
	assert.Equal(odhSystem, odhSystem.Ops())
	assert.Empty(odhSystem.systemID)
	assert.NoError(odhSystem.Initialize(ctx))
	assert.NotEmpty(odhSystem.systemID)
	assert.Equal(dSystem, odhSystem.systemObj) // cached

	t.Log("case: systemID already set")
	assert.NoError(odhSystem.Initialize(ctx))
	assert.NotEmpty(odhSystem.systemID)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize, system object present and valid")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhSystem).Return(nil)
	crud.EXPECT().FindOne(ctx, odhSystem, bson.M{}, newSystemMatcher(t, mockSystemFind, dSystem)).Return(nil)
	odhSystem.crud = crud
	odhSystem.systemID = ""
	odhSystem.systemObj = nil
	odhSystem.api = api
	assert.NoError(odhSystem.Initialize(ctx))
	assert.NotEmpty(odhSystem.systemID)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize, error in CreateIndexes")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhSystem).Return(errUnknownError)
	odhSystem.crud = crud
	odhSystem.systemID = ""
	odhSystem.systemObj = nil
	odhSystem.api = api
	assert.Equal(errUnknownError, odhSystem.Initialize(ctx))
	assert.Empty(odhSystem.systemID)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize, Find failure")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhSystem).Return(nil)
	crud.EXPECT().FindOne(ctx, odhSystem, bson.M{}, newSystemMatcher(t, mockSystemFind, dSystem)).Return(errWrappedError)
	odhSystem.crud = crud
	odhSystem.systemID = ""
	odhSystem.systemObj = nil
	odhSystem.api = api
	assert.Equal(errWrappedError, odhSystem.Initialize(ctx))
	assert.Empty(odhSystem.systemID)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: Initialize, error creating system object")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().CreateIndexes(ctx, odhSystem).Return(nil)
	crud.EXPECT().FindOne(ctx, odhSystem, bson.M{}, newSystemMatcher(t, mockSystemFind, dSystem)).Return(ds.ErrorNotFound)
	crud.EXPECT().InsertOne(ctx, odhSystem, newSystemMatcher(t, mockSystemInsert, dSystem)).Return(errWrappedError)
	odhSystem.crud = crud
	odhSystem.systemID = ""
	odhSystem.systemObj = nil
	odhSystem.api = api
	assert.Equal(errWrappedError, odhSystem.Initialize(ctx))
	assert.Empty(odhSystem.systemID)

	t.Log("case: Start")
	api.EXPECT().DBName().Return("dbName")
	assert.NoError(odhSystem.Start(ctx))
}

func TestSystemFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	mTime := time.Now()
	cTime := mTime.AddDate(0, 0, -1)
	dSystem := &System{
		ObjMeta: ObjMeta{
			MetaObjID:        "systemID",
			MetaTimeCreated:  cTime,
			MetaTimeModified: mTime,
		},
		Name:        "testsystem",
		Description: "test description",
		SnapshotCatalogPolicy: SnapshotCatalogPolicy{
			CspDomainID:        "csp-1",
			ProtectionDomainID: "pd-1",
		},
		SnapshotManagementPolicy: SnapshotManagementPolicy{
			RetentionDurationSeconds: 1,
			DeleteLast:               true,
		},
		ClusterUsagePolicy: ClusterUsagePolicy{
			AccountSecretScope:          "CLUSTER",
			ConsistencyGroupName:        "${k8sPod.name}",
			VolumeDataRetentionOnDelete: "DELETE",
		},
		ManagementHostCName: "cname",
	}

	t.Log("case: fetch from cache")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	odhSystem.systemObj = dSystem
	retMA, retErr := odhSystem.Fetch()
	assert.NoError(retErr)
	assert.Equal(dSystem.ToModel(), retMA)

	t.Log("case: not initialized")
	odhSystem.systemObj = nil
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().WrapError(fmt.Errorf("database not available"), false).Return(errWrappedError)
	odhSystem.api = api
	odhSystem.log = l
	retMA, retErr = odhSystem.Fetch()
	assert.Equal(errWrappedError, retErr)
	assert.Nil(retMA)
}

func TestSystemUpdate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	odhSystem.systemID = "systemID"
	ua := &ds.UpdateArgs{
		// handler does not set ID on update of system object
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
				Name: "Description",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
			{
				Name: "SnapshotCatalogPolicy",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
			{
				Name: "SnapshotManagementPolicy",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
			{
				Name: "VsrManagementPolicy",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
			{
				Name: "ClusterUsagePolicy",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
			{
				Name: "SystemTags",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateAppend: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
			{
				Name: "ManagementHostCName",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	param := &models.SystemMutable{
		Name:        "newName",
		Description: "description",
		SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
			CspDomainID:        "csp-1",
			ProtectionDomainID: "pd-1",
		},
		SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
			RetentionDurationSeconds: swag.Int32(1),
			NoDelete:                 true,
		},
		VsrManagementPolicy: &models.VsrManagementPolicy{
			RetentionDurationSeconds: swag.Int32(10),
			NoDelete:                 false,
		},
		ClusterUsagePolicy: &models.ClusterUsagePolicy{
			AccountSecretScope: common.AccountSecretScopeGlobal,
		},
		SystemTags:          []string{"stag1", "stag2"},
		ManagementHostCName: "cname",
	}
	mObj := &models.System{SystemMutable: *param}
	inObj := &System{}
	inObj.FromModel(mObj)
	now := time.Now()
	dSystem := &System{
		ObjMeta: ObjMeta{
			MetaObjID:        odhSystem.systemID,
			MetaTimeCreated:  now.AddDate(0, -2, -1),
			MetaTimeModified: now,
			MetaVersion:      1000,
		},
	}
	m := systemMatcher(t, mockSystemUpdate).CtxObj(inObj).Return(dSystem).Matcher()
	ctx := context.Background()

	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() {
		mockCtrl.Finish()
		odhSystem.systemID = ""
	}()
	api := NewMockDBAPI(mockCtrl)
	api.EXPECT().MustBeReady().Return(nil)
	crud := NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhSystem, m, ua).Return(nil)
	odhSystem.crud = crud
	odhSystem.api = api
	odhSystem.log = l
	odhSystem.systemObj = nil
	var retMA *models.System
	var retErr error
	assert.NotPanics(func() { retMA, retErr = odhSystem.Update(ctx, ua, param) })
	assert.NoError(retErr)
	assert.Equal(dSystem.ToModel(), retMA)
	assert.Equal(odhSystem.systemID, ua.ID)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: not ready")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	ua.ID = ""
	api.EXPECT().MustBeReady().Return(errUnknownError)
	odhSystem.api = api
	odhSystem.log = l
	odhSystem.systemObj = nil
	retMA, retErr = odhSystem.Update(ctx, ua, param) // ua,params are known to work
	assert.Equal(errUnknownError, retErr)
	assert.Nil(retMA)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: update fails")
	mockCtrl = gomock.NewController(t)
	api = NewMockDBAPI(mockCtrl)
	crud = NewMockObjectDocumentHandlerCRUD(mockCtrl)
	crud.EXPECT().UpdateOne(ctx, odhSystem, m, ua).Return(ds.ErrorIDVerNotFound)
	odhSystem.crud = crud
	odhSystem.api = api
	odhSystem.log = l
	odhSystem.systemObj = nil
	ua.ID = ""
	retMA, retErr = odhSystem.intUpdate(ctx, ua, param)
	assert.Equal(ds.ErrorIDVerNotFound, retErr)
	assert.Nil(retMA)
	assert.Equal(odhSystem.systemID, ua.ID)
	tl.Flush()

	t.Log("case: attempt to delete snapshot catalog policy")
	param = &models.SystemMutable{}
	ua = &ds.UpdateArgs{
		Version: 0,
		Attributes: []ds.UpdateAttr{
			{
				Name: "SnapshotCatalogPolicy",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	retMA, retErr = odhSystem.intUpdate(ctx, ua, param)
	assert.Error(retErr)
	assert.Regexp("invalid.*snapshotCatalogPolicy", retErr)
	assert.Nil(retMA)
	param.SnapshotCatalogPolicy = &models.SnapshotCatalogPolicy{
		Inherited: true,
	}
	retMA, retErr = odhSystem.intUpdate(ctx, ua, param)
	assert.Error(retErr)
	assert.Regexp("invalid.*snapshotCatalogPolicy", retErr)
	assert.Nil(retMA)

	t.Log("case: attempt to delete snapshot management policy")
	param = &models.SystemMutable{}
	ua = &ds.UpdateArgs{
		Version: 0,
		Attributes: []ds.UpdateAttr{
			{
				Name: "SnapshotManagementPolicy",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	retMA, retErr = odhSystem.intUpdate(ctx, ua, param)
	assert.Error(retErr)
	assert.Regexp("invalid.*snapshotManagementPolicy", retErr)
	assert.Nil(retMA)
	param.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{}
	retMA, retErr = odhSystem.intUpdate(ctx, ua, param)
	assert.Error(retErr)
	assert.Regexp("invalid.*snapshotManagementPolicy", retErr)
	assert.Nil(retMA)

	t.Log("case: attempt to delete VSR management policy")
	param = &models.SystemMutable{}
	// param.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{
	// 	RetentionDurationSeconds: swag.Int32(1),
	// 	NoDelete:                 true,
	// }
	ua = &ds.UpdateArgs{
		Version: 0,
		Attributes: []ds.UpdateAttr{
			{
				Name: "VsrManagementPolicy",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	retMA, retErr = odhSystem.intUpdate(ctx, ua, param)
	assert.Error(retErr)
	assert.Regexp("invalid.*vsrManagementPolicy", retErr)
	assert.Nil(retMA)
	param.VsrManagementPolicy = &models.VsrManagementPolicy{}
	retMA, retErr = odhSystem.intUpdate(ctx, ua, param)
	assert.Error(retErr)
	assert.Regexp("invalid.*vsrManagementPolicy", retErr)
	assert.Nil(retMA)

	t.Log("case: attempt to delete cluster usage policy")
	odhSystem.log = l
	param = &models.SystemMutable{}
	ua = &ds.UpdateArgs{
		Version: 0,
		Attributes: []ds.UpdateAttr{
			{
				Name: "ClusterUsagePolicy",
				Actions: [ds.NumActionTypes]ds.UpdateActionArgs{
					ds.UpdateSet: ds.UpdateActionArgs{
						FromBody: true,
					},
				},
			},
		},
	}
	retMA, retErr = odhSystem.intUpdate(ctx, ua, param)
	assert.Error(retErr)
	assert.Regexp("invalid.*clusterUsagePolicy", retErr)
	assert.Nil(retMA)
	param.ClusterUsagePolicy = &models.ClusterUsagePolicy{
		Inherited: true,
	}
	retMA, retErr = odhSystem.intUpdate(ctx, ua, param)
	assert.Error(retErr)
	assert.Regexp("invalid.*clusterUsagePolicy", retErr)
	assert.Nil(retMA)
	tl.Flush()
	param.ClusterUsagePolicy = &models.ClusterUsagePolicy{
		Inherited: true,
	}
	retMA, retErr = odhSystem.intUpdate(ctx, ua, param)
	assert.Error(retErr)
	assert.Regexp("invalid.*clusterUsagePolicy", retErr)
	assert.Nil(retMA)
}
