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
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/role"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/runtime/middleware"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestRoleFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	params := ops.RoleFetchParams{ID: "objectID"}
	obj := &models.Role{
		RoleAllOf0: models.RoleAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("objectID"),
			},
		},
	}

	// success
	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oR := mock.NewMockRoleOps(mockCtrl)
	oR.EXPECT().Fetch(params.ID).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsRole().Return(oR)
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.roleFetch(params) })
	assert.NotNil(ret)
	mR, ok := ret.(*ops.RoleFetchOK)
	assert.True(ok)
	assert.Equal(obj, mR.Payload)
	tl.Flush()

	// fetch failure
	mockCtrl.Finish()
	t.Log("case: fetch failure")
	mockCtrl = gomock.NewController(t)
	oR = mock.NewMockRoleOps(mockCtrl)
	oR.EXPECT().Fetch(params.ID).Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsRole().Return(oR)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.roleFetch(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.RoleFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
}

func TestRoleList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	params := ops.RoleListParams{}
	objects := []*models.Role{
		&models.Role{
			RoleAllOf0: models.RoleAllOf0{
				Meta: &models.ObjMeta{
					ID: models.ObjID("objectID"),
				},
			},
		},
	}

	// success
	t.Log("case: success")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oR := mock.NewMockRoleOps(mockCtrl)
	oR.EXPECT().List(params).Return(objects, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsRole().Return(oR)
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.roleList(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.RoleListOK)
	assert.True(ok)
	assert.Equal(objects, mO.Payload)
	tl.Flush()

	// list fails
	mockCtrl.Finish()
	t.Log("case: List failure")
	mockCtrl = gomock.NewController(t)
	oR = mock.NewMockRoleOps(mockCtrl)
	oR.EXPECT().List(params).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsRole().Return(oR)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.roleList(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.RoleListDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
}
