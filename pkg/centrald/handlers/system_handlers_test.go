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
	"fmt"
	"net/http"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/system"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	mockcluster "github.com/Nuvoloso/kontroller/pkg/cluster/mock"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestSystemFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	svcArgs := util.ServiceArgs{
		ServiceType:         "test",
		ServiceVersion:      "test-version",
		HeartbeatPeriodSecs: int64(90),
		Log:                 tl.Logger(),
		MaxMessages:         999,
		ServiceAttributes:   map[string]models.ValueType{},
	}
	app.Service = util.NewService(&svcArgs)
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	params := ops.SystemFetchParams{}
	obj := &models.System{
		SystemAllOf0: models.SystemAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("objectID"),
			},
		},
		SystemMutable: models.SystemMutable{
			Name: "system",
		},
	}

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oA := mock.NewMockSystemOps(mockCtrl)
	oA.EXPECT().Fetch().Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSystem().Return(oA).MinTimes(1)
	t.Log("case: SystemFetch success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.systemFetch(params) })
	assert.NotNil(ret)
	mR, ok := ret.(*ops.SystemFetchOK)
	assert.True(ok)
	// check that Fetch handler added non-persisted Service to returned obj
	assert.Equal(svcArgs.ServiceType, obj.Service.ServiceType)
	assert.Equal(svcArgs.ServiceVersion, obj.Service.ServiceVersion)
	assert.Equal(obj, mR.Payload)
	tl.Flush()

	// fetch failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	oA = mock.NewMockSystemOps(mockCtrl)
	oA.EXPECT().Fetch().Return(nil, centrald.ErrorNotFound)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSystem().Return(oA).MinTimes(1)
	t.Log("case: fetch failure")
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.systemFetch(params) })
	assert.NotNil(ret)
	mE, ok := ret.(*ops.SystemFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mE.Payload.Code))
	assert.Equal(centrald.ErrorNotFound.M, *mE.Payload.Message)
}

func TestSystemUpdate(t *testing.T) {
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
	assert.NotPanics(func() { nMap = hc.systemMutableNameMap() })

	// parse params
	objM := &models.SystemMutable{
		Name:       "system",
		SystemTags: []string{"stag1", "stag2"},
		SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
			Inherited:                false,
			RetentionDurationSeconds: swag.Int32(7 * 24 * 60 * 60),
		},
		SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
			CspDomainID:        "domID",
			ProtectionDomainID: "pdID",
			Inherited:          false,
		},
	VsrManagementPolicy: &models.VsrManagementPolicy{
			Inherited:                false,
			RetentionDurationSeconds: swag.Int32(7 * 24 * 60 * 60),
		},
	}
	ai := &auth.Info{}
	params := ops.SystemUpdateParams{
		HTTPRequest: requestWithAuthContext(ai),
		Version:     swag.Int32(8),
		Set:         []string{"name", "snapshotManagementPolicy", "vsrManagementPolicy", "snapshotCatalogPolicy"},
		Append:      []string{nMap.jName("SystemTags")},
		Payload:     objM,
	}
	ctx := params.HTTPRequest.Context()
	var ua *centrald.UpdateArgs
	var err error
	var uP = [centrald.NumActionTypes][]string{
		centrald.UpdateAppend: params.Append,
		centrald.UpdateSet:    params.Set,
	}
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, "", params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM := updateArgsMatcher(t, ua).Matcher()

	// success
	obj := &models.System{
		SystemAllOf0: models.SystemAllOf0{
			Meta: &models.ObjMeta{ID: models.ObjID("objectID")},
		},
	}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	oA := mock.NewMockSystemOps(mockCtrl)
	oA.EXPECT().Update(ctx, uaM, params.Payload).Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSystem().Return(oA).MinTimes(1)
	t.Log("case: SystemUpdate success")
	hc.DS = mds
	var ret middleware.Responder
	assert.NotPanics(func() { ret = hc.systemUpdate(params) })
	assert.NotNil(ret)
	mO, ok := ret.(*ops.SystemUpdateOK)
	assert.True(ok)
	assert.Equal(obj, mO.Payload)
	assert.Empty(evM.InSSProps)
	assert.Equal(obj, evM.InACScope)
	tl.Flush()

	// Update failed, cover case of valid SystemAdminRole
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: update failed")
	ai.AccountID = "aid1"
	ai.RoleObj = &models.Role{}
	ai.RoleObj.Capabilities = map[string]bool{centrald.SystemManagementCap: true}
	params.Append = []string{}
	uP[centrald.UpdateAppend] = params.Append
	assert.NotPanics(func() { ua, err = hc.makeStdUpdateArgs(nMap, "", params.Version, uP) })
	assert.Nil(err)
	assert.NotNil(ua)
	uaM = updateArgsMatcher(t, ua)
	oA = mock.NewMockSystemOps(mockCtrl)
	oA.EXPECT().Update(ctx, uaM, params.Payload).Return(nil, centrald.ErrorDbError)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSystem().Return(oA).MinTimes(1)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.systemUpdate(params) })
	assert.NotNil(ret)
	mD, ok := ret.(*ops.SystemUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorDbError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorDbError.M, *mD.Payload.Message)
	tl.Flush()

	// no changes requested
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: no change")
	params.Set = []string{}
	mds = mock.NewMockDataStore(mockCtrl)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.systemUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.SystemUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	tl.Flush()

	// empty name
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: empty name")
	params.Set = []string{"name"}
	params.Payload.Name = ""
	mds = mock.NewMockDataStore(mockCtrl)
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.systemUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.SystemUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
	assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
	tl.Flush()

	// invalid policies updates
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	tcs := []string{"clusterUsagePolicy", "snapshotManagementPolicy", "vsrManagementPolicy", "snapshotCatalogPolicy"}
	for _, tc := range tcs {
		params.Set = []string{tc}
		switch tc {
		case "clusterUsagePolicy":
			params.Payload.ClusterUsagePolicy = nil
		case "snapshotCatalogPolicy":
			params.Payload.SnapshotCatalogPolicy = nil
		case "snapshotManagementPolicy":
			params.Payload = &models.SystemMutable{
				SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
					NoDelete: false,
				},
			}
		case "vsrManagementPolicy":
			params.Payload = &models.SystemMutable{
				VsrManagementPolicy: &models.VsrManagementPolicy{
					NoDelete: false,
				},
			}
		default:
			assert.False(true)
		}
		t.Log("case: systemUpdate error case: invalid " + tc)
		assert.NotPanics(func() { ret = hc.systemUpdate(params) })
		assert.NotNil(ret)
		mD, ok = ret.(*ops.SystemUpdateDefault)
		assert.True(ok)
		assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
		assert.Regexp("^"+centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
		tl.Flush()
	}
	params.Set = []string{}

	t.Log("case: auth internal error")
	params.HTTPRequest = &http.Request{}
	assert.NotPanics(func() { ret = hc.systemUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.SystemUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInternalError.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorInternalError.M, *mD.Payload.Message)
	params.HTTPRequest = requestWithAuthContext(ai)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not internal role")
	hc.DS = mds
	params.Append = []string{"systemTags"}
	params.Payload.Name = "new"
	assert.NotPanics(func() { ret = hc.systemUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.SystemUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)
	tl.Flush()

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: not system admin role")
	hc.DS = mds
	ai.RoleObj.Capabilities = map[string]bool{centrald.ManageSpecialAccountsCap: true}
	params.Append = []string{}
	params.Set = []string{"name", "description"}
	assert.NotPanics(func() { ret = hc.systemUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.SystemUpdateDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.C, int(mD.Payload.Code))
	assert.Equal(centrald.ErrorUnauthorizedOrForbidden.M, *mD.Payload.Message)
	ai.RoleObj = nil
	tl.Flush()

	// no payload
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: no payload")
	mds = mock.NewMockDataStore(mockCtrl)
	params.Set = []string{"name", "description"}
	params.Payload = nil
	hc.DS = mds
	assert.NotPanics(func() { ret = hc.systemUpdate(params) })
	assert.NotNil(ret)
	mD, ok = ret.(*ops.SystemUpdateDefault)
	if assert.True(ok) {
		assert.Equal(centrald.ErrorUpdateInvalidRequest.C, int(mD.Payload.Code))
		assert.Regexp(centrald.ErrorUpdateInvalidRequest.M, *mD.Payload.Message)
		assert.Regexp("missing payload", *mD.Payload.Message)
	}
}

func TestSystemFetchHostname(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	api := &operations.NuvolosoAPI{}
	app := testAppCtx(api, tl.Logger())
	svcArgs := util.ServiceArgs{
		ServiceType:         "test",
		ServiceVersion:      "test-version",
		HeartbeatPeriodSecs: int64(90),
		Log:                 tl.Logger(),
		MaxMessages:         999,
		ServiceAttributes:   map[string]models.ValueType{},
	}
	app.Service = util.NewService(&svcArgs)
	hc := newHandlerComp()
	hc.Log = tl.Logger()
	hc.app = app
	var ret middleware.Responder
	ai := &auth.Info{}
	params := ops.SystemHostnameFetchParams{
		HTTPRequest: requestWithAuthContext(ai),
	}
	obj := &models.System{
		SystemAllOf0: models.SystemAllOf0{
			Meta: &models.ObjMeta{
				ID: models.ObjID("objectID"),
			},
		},
		SystemMutable: models.SystemMutable{
			Name:                "system",
			ManagementHostCName: "cname",
		},
	}
	// success, fetching from system
	mockCtrl := gomock.NewController(t)
	oA := mock.NewMockSystemOps(mockCtrl)
	oA.EXPECT().Fetch().Return(obj, nil)
	mds := mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSystem().Return(oA).MinTimes(1)
	hc.DS = mds
	ret = hc.systemHostnameFetch(params)
	assert.NotNil(ret)
	md, ok := ret.(*ops.SystemHostnameFetchOK)
	assert.True(ok)
	assert.Equal("cname", md.Payload)
	mockCtrl.Finish()

	// failure, unable to fetch system
	mockCtrl = gomock.NewController(t)
	oA = mock.NewMockSystemOps(mockCtrl)
	oA.EXPECT().Fetch().Return(nil, fmt.Errorf("system not found"))
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSystem().Return(oA).MinTimes(1)
	hc.DS = mds
	ret = hc.systemHostnameFetch(params)
	assert.NotNil(ret)
	mD, ok := ret.(*ops.SystemHostnameFetchDefault)
	assert.Equal(centrald.ErrorNotFound.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorNotFound.M, *mD.Payload.Message)
	assert.Regexp("system not found", *mD.Payload.Message)
	mockCtrl.Finish()

	obj.ManagementHostCName = ""

	// success, from cluster
	mockCtrl = gomock.NewController(t)
	hc.app.AppArgs.ClusterType = "kubernetes"
	hc.app.AppArgs.ManagementServiceName = "nuvo-https"
	hc.app.AppArgs.ManagementServiceNamespace = "nuvoloso-management"
	oA = mock.NewMockSystemOps(mockCtrl)
	oA.EXPECT().Fetch().Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSystem().Return(oA).MinTimes(1)
	hc.DS = mds
	cc := mockcluster.NewMockClient(mockCtrl)
	hc.clusterClientMap = make(map[string]cluster.Client)
	hc.clusterClientMap[cluster.K8sClusterType] = cc
	cc.EXPECT().GetService(gomock.Any(), "nuvo-https", "nuvoloso-management").Return(&cluster.Service{Hostname: "hostname"}, nil)
	ret = hc.systemHostnameFetch(params)
	assert.NotNil(ret)
	md, ok = ret.(*ops.SystemHostnameFetchOK)
	assert.True(ok)
	assert.Equal("hostname", md.Payload)
	mockCtrl.Finish()

	// hostname not found failure
	mockCtrl = gomock.NewController(t)
	oA = mock.NewMockSystemOps(mockCtrl)
	oA.EXPECT().Fetch().Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSystem().Return(oA).MinTimes(1)
	hc.DS = mds
	cc = mockcluster.NewMockClient(mockCtrl)
	hc.clusterClientMap = make(map[string]cluster.Client)
	hc.clusterClientMap[cluster.K8sClusterType] = cc
	cc.EXPECT().GetService(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("hostname not found"))
	ret = hc.systemHostnameFetch(params)
	assert.NotNil(ret)
	mD, ok = ret.(*ops.SystemHostnameFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorNotFound.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorNotFound.M, *mD.Payload.Message)
	assert.Regexp("hostname not found", *mD.Payload.Message)
	mockCtrl.Finish()

	// unable to find cluster client
	mockCtrl = gomock.NewController(t)
	oA = mock.NewMockSystemOps(mockCtrl)
	oA.EXPECT().Fetch().Return(obj, nil)
	mds = mock.NewMockDataStore(mockCtrl)
	mds.EXPECT().OpsSystem().Return(oA).MinTimes(1)
	hc.DS = mds
	cc = mockcluster.NewMockClient(mockCtrl)
	hc.clusterClientMap = make(map[string]cluster.Client)
	hc.clusterClientMap[cluster.K8sClusterType] = nil
	ret = hc.systemHostnameFetch(params)
	assert.NotNil(ret)
	mD, ok = ret.(*ops.SystemHostnameFetchDefault)
	assert.True(ok)
	assert.Equal(centrald.ErrorInvalidData.C, int(mD.Payload.Code))
	assert.Regexp(centrald.ErrorInvalidData.M, *mD.Payload.Message)
}
