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
	"reflect"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/node"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	ops "github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/node"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	fcl "github.com/Nuvoloso/kontroller/pkg/clusterd/fake"
	fakeState "github.com/Nuvoloso/kontroller/pkg/clusterd/state/fake"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestNodeError(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	hc := newHandlerComp()
	hc.Log = tl.Logger()

	mE := hc.nodeError(fmt.Errorf("unexpected error"))
	assert.NotNil(mE)
	assert.EqualValues(500, mE.Code)
	assert.Regexp("unexpected error", *mE.Message)

	// other error cases tested in context
}

func TestNodeCreate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log: tl.Logger(),
		},
	}
	hc := newHandlerComp()
	hc.app = app
	hc.Log = tl.Logger()

	inNode := &models.Node{
		NodeMutable: models.NodeMutable{
			Service: &models.NuvoService{
				ServiceState: models.ServiceState{
					HeartbeatPeriodSecs: 20, // node period
				},
			},
		},
	}
	inP := ops.NodeCreateParams{Payload: inNode, HTTPRequest: &http.Request{}}
	resD := &node.NodeCreateDefault{Payload: &models.Error{Code: 10, Message: swag.String("create-error")}}
	resOk := &node.NodeCreateCreated{Payload: &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{ID: "NODE-1"},
		},
		NodeMutable: models.NodeMutable{
			Service: &models.NuvoService{
				ServiceState: models.ServiceState{
					HeartbeatPeriodSecs: 200, // task period
				},
			},
		},
	}}

	// proxy not ready
	res := hc.nodeCreate(inP)
	assert.NotNil(res)
	errRes, ok := res.(*ops.NodeCreateDefault)
	assert.True(ok)
	assert.NotNil(errRes)
	assert.Regexp("initializing", *errRes.Payload.Message)
	assert.Equal(int32(http.StatusServiceUnavailable), errRes.Payload.Code)

	// now ready
	fs := fakeState.NewFakeClusterState()
	app.StateOps = fs
	fsu := &fcl.StateUpdater{RetHTPsec: resOk.Payload.Service.HeartbeatPeriodSecs}
	app.StateUpdater = fsu

	// error
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	nOps := mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps).MinTimes(1)
	mN := mockmgmtclient.NewNodeMatcher(t, node.NewNodeCreateParams().WithPayload(inP.Payload))
	nOps.EXPECT().NodeCreate(mN).Return(nil, resD).MinTimes(1)
	app.ClientAPI = mAPI
	assert.EqualValues(20, inP.Payload.Service.HeartbeatPeriodSecs)
	res = hc.nodeCreate(inP)
	assert.EqualValues(200, inP.Payload.Service.HeartbeatPeriodSecs)
	assert.NotNil(res)
	errRes, ok = res.(*ops.NodeCreateDefault)
	assert.True(ok)
	assert.NotNil(errRes)
	assert.Equal(resD.Payload, errRes.Payload)
	mockCtrl.Finish()
	tl.Flush()

	// success
	fs.InNSSid = ""
	fs.InNSSss = nil
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps).MinTimes(1)
	nOps.EXPECT().NodeCreate(mN).Return(resOk, nil).MinTimes(1)
	app.ClientAPI = mAPI
	res = hc.nodeCreate(inP)
	okRes, ok := res.(*ops.NodeCreateCreated)
	assert.True(ok)
	assert.NotNil(okRes)
	assert.Equal(resOk.Payload, okRes.Payload)
	assert.EqualValues(resOk.Payload.Meta.ID, fs.InNSSid)
	assert.Equal(&resOk.Payload.Service.ServiceState, fs.InNSSss)
}

func TestNodeDelete(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log: tl.Logger(),
		},
	}
	hc := newHandlerComp()
	hc.app = app
	hc.Log = tl.Logger()

	inP := ops.NodeDeleteParams{ID: "id", HTTPRequest: &http.Request{}}
	resD := &node.NodeDeleteDefault{Payload: &models.Error{Code: 10, Message: swag.String("delete-error")}}

	// proxy not ready
	res := hc.nodeDelete(inP)
	assert.NotNil(res)
	errRes, ok := res.(*ops.NodeDeleteDefault)
	assert.True(ok)
	assert.NotNil(errRes)
	assert.Regexp("initializing", *errRes.Payload.Message)
	assert.Equal(int32(503), errRes.Payload.Code)

	// now ready
	fs := fakeState.NewFakeClusterState()
	app.StateOps = fs
	fsu := &fcl.StateUpdater{}
	app.StateUpdater = fsu

	// error
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	nOps := mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps).MinTimes(1)
	mN := mockmgmtclient.NewNodeMatcher(t, node.NewNodeDeleteParams().WithID(inP.ID))
	nOps.EXPECT().NodeDelete(mN).Return(nil, resD).MinTimes(1)
	app.ClientAPI = mAPI
	res = hc.nodeDelete(inP)
	assert.NotNil(res)
	errRes, ok = res.(*ops.NodeDeleteDefault)
	assert.True(ok)
	assert.NotNil(errRes)
	assert.Equal(resD.Payload, errRes.Payload)
	mockCtrl.Finish()
	tl.Flush()

	// success
	fs.InNDid = ""
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps).MinTimes(1)
	nOps.EXPECT().NodeDelete(mN).Return(nil, nil).MinTimes(1)
	app.ClientAPI = mAPI
	res = hc.nodeDelete(inP)
	okRes, ok := res.(*ops.NodeDeleteNoContent)
	assert.True(ok)
	assert.NotNil(okRes)
	assert.EqualValues(&node.NodeDeleteNoContent{}, okRes)
	assert.Equal(inP.ID, fs.InNDid)
}

func TestNodeFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log: tl.Logger(),
		},
	}
	hc := newHandlerComp()
	hc.app = app
	hc.Log = tl.Logger()

	inP := ops.NodeFetchParams{ID: "id", HTTPRequest: &http.Request{}}
	resD := &node.NodeFetchDefault{Payload: &models.Error{Code: 10, Message: swag.String("fetch-error")}}
	resOk := &node.NodeFetchOK{Payload: &models.Node{}}

	// proxy not ready
	res := hc.nodeFetch(inP)
	assert.NotNil(res)
	errRes, ok := res.(*ops.NodeFetchDefault)
	assert.True(ok)
	assert.NotNil(errRes)
	assert.Regexp("initializing", *errRes.Payload.Message)
	assert.Equal(int32(503), errRes.Payload.Code)

	// now ready
	fs := fakeState.NewFakeClusterState()
	app.StateOps = fs
	fsu := &fcl.StateUpdater{}
	app.StateUpdater = fsu

	// error
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	nOps := mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps).MinTimes(1)
	mN := mockmgmtclient.NewNodeMatcher(t, node.NewNodeFetchParams().WithID(inP.ID))
	nOps.EXPECT().NodeFetch(mN).Return(nil, resD).MinTimes(1)
	app.ClientAPI = mAPI
	res = hc.nodeFetch(inP)
	assert.NotNil(res)
	errRes, ok = res.(*ops.NodeFetchDefault)
	assert.True(ok)
	assert.NotNil(errRes)
	assert.Equal(resD.Payload, errRes.Payload)
	mockCtrl.Finish()
	tl.Flush()

	// success
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps).MinTimes(1)
	nOps.EXPECT().NodeFetch(mN).Return(resOk, nil).MinTimes(1)
	app.ClientAPI = mAPI
	res = hc.nodeFetch(inP)
	okRes, ok := res.(*ops.NodeFetchOK)
	assert.True(ok)
	assert.NotNil(okRes)
	assert.Equal(resOk.Payload, okRes.Payload)
}

func TestNodeList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log: tl.Logger(),
		},
	}
	hc := newHandlerComp()
	hc.app = app
	hc.Log = tl.Logger()

	numInputProps := 11
	dat := strfmt.DateTime(time.Now())
	inP := ops.NodeListParams{
		ClusterID:              swag.String("clusterId"),
		Name:                   swag.String("name"),
		NodeIdentifier:         swag.String("nodeId"),
		Tags:                   []string{},
		ServiceHeartbeatTimeGE: &dat,
		ServiceHeartbeatTimeLE: &dat,
		ServiceStateEQ:         swag.String("UNKNOWN"),
		ServiceStateNE:         swag.String("READY"),
		StateEQ:                swag.String("MANAGED"),
		StateNE:                swag.String("TEAR_DOWN"),
		NodeIds:                []string{},
		HTTPRequest:            &http.Request{},
	}
	assert.Equal(numInputProps+1, reflect.TypeOf(inP).NumField()) // +: HTTPRequest
	cinP := node.NewNodeListParams().WithClusterID(inP.ClusterID).WithName(inP.Name).
		WithNodeIdentifier(inP.NodeIdentifier).WithTags(inP.Tags).
		WithServiceHeartbeatTimeGE(&dat).WithServiceHeartbeatTimeLE(&dat).
		WithServiceStateEQ(swag.String("UNKNOWN")).WithServiceStateNE(swag.String("READY")).
		WithStateEQ(swag.String("MANAGED")).WithStateNE(swag.String("TEAR_DOWN")).
		WithNodeIds([]string{})
	assert.Equal(numInputProps+3, reflect.TypeOf(*cinP).NumField()) // +: HTTPClient, Context, Timeout
	resD := &node.NodeListDefault{Payload: &models.Error{Code: 10, Message: swag.String("list-error")}}
	resOk := &node.NodeListOK{Payload: []*models.Node{}}

	// proxy not ready
	res := hc.nodeList(inP)
	assert.NotNil(res)
	errRes, ok := res.(*ops.NodeListDefault)
	assert.True(ok)
	assert.NotNil(errRes)
	assert.Regexp("initializing", *errRes.Payload.Message)
	assert.Equal(int32(503), errRes.Payload.Code)

	// now ready
	fs := fakeState.NewFakeClusterState()
	app.StateOps = fs
	fsu := &fcl.StateUpdater{}
	app.StateUpdater = fsu

	// error
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	nOps := mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps).MinTimes(1)
	mN := mockmgmtclient.NewNodeMatcher(t, cinP)
	nOps.EXPECT().NodeList(mN).Return(nil, resD).MinTimes(1)
	app.ClientAPI = mAPI
	res = hc.nodeList(inP)
	assert.NotNil(res)
	errRes, ok = res.(*ops.NodeListDefault)
	assert.True(ok)
	assert.NotNil(errRes)
	assert.Equal(resD.Payload, errRes.Payload)
	mockCtrl.Finish()
	tl.Flush()

	// success
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps).MinTimes(1)
	nOps.EXPECT().NodeList(mN).Return(resOk, nil).MinTimes(1)
	app.ClientAPI = mAPI
	res = hc.nodeList(inP)
	okRes, ok := res.(*ops.NodeListOK)
	assert.True(ok)
	assert.NotNil(okRes)
	assert.Equal(resOk.Payload, okRes.Payload)
}

func TestNodeUpdate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log: tl.Logger(),
		},
	}
	hc := newHandlerComp()
	hc.app = app
	hc.Log = tl.Logger()

	// test heartbeat predicates
	nodeHbPlus := append(nodeHbOnlySet, "something")
	minHbP := &models.NodeMutable{
		Service: &models.NuvoService{},
	}
	hboInP := ops.NodeUpdateParams{ID: "hboID", Set: nodeHbOnlySet, Payload: minHbP}
	hbTCs := []struct {
		hhd bool
		hou bool
		p   ops.NodeUpdateParams
	}{
		{false, false, ops.NodeUpdateParams{}},
		{false, false, ops.NodeUpdateParams{Set: []string{"servicePrefixNotMatching"}, Payload: minHbP}},
		{false, false, ops.NodeUpdateParams{Set: []string{"service.state", "service.heartbeatTime", "something-else"}, Payload: minHbP}},
		{false, true, ops.NodeUpdateParams{Set: nodeHbOnlySet, Payload: minHbP}},
		{true, false, ops.NodeUpdateParams{ID: "x", Set: nodeHbPlus, Payload: minHbP}},
		{true, false, ops.NodeUpdateParams{ID: "x", Set: []string{"service.messages"}, Payload: minHbP}},
		{true, true, hboInP},
		{false, true, ops.NodeUpdateParams{Set: nodeHbOnlySet, Payload: minHbP}},
		{true, false, ops.NodeUpdateParams{ID: "x", Set: nodeHbOnlySet, Append: []string{"something"}, Payload: minHbP}},
		{true, false, ops.NodeUpdateParams{ID: "x", Set: nodeHbOnlySet, Remove: []string{"something"}, Payload: minHbP}},
		{true, false, ops.NodeUpdateParams{ID: "x", Set: []string{"service"}, Payload: minHbP}},
	}
	for i, tc := range hbTCs {
		assert.Equal(tc.hhd, hc.nodeUpdateHasHeartbeatData(tc.p), "[%d] HeartbeatData", i)
		assert.Equal(tc.hou, hc.isNodeHeartbeatOnlyUpdate(tc.p), "[%d] HeartbeatOnly", i)
	}

	numInputProps := 6
	inP := ops.NodeUpdateParams{
		Append:      []string{"a"},
		Remove:      []string{"r"},
		Set:         nodeHbPlus,
		ID:          "nonHboID",
		Version:     swag.Int32(1),
		Payload:     minHbP,
		HTTPRequest: &http.Request{},
	}
	assert.Equal(numInputProps+1, reflect.TypeOf(inP).NumField()) // +: HTTPRequest
	cinP := node.NewNodeUpdateParams().WithAppend(inP.Append).WithRemove(inP.Remove).WithSet(inP.Set).
		WithID(inP.ID).WithVersion(inP.Version).WithPayload(inP.Payload)
	assert.Equal(numInputProps+3, reflect.TypeOf(*cinP).NumField()) // +: HTTPClient, Context, Timeout
	resD := &node.NodeUpdateDefault{Payload: &models.Error{Code: 10, Message: swag.String("update-error")}}
	resOk := &node.NodeUpdateOK{Payload: &models.Node{}}

	assert.False(hc.isNodeHeartbeatOnlyUpdate(inP))
	assert.True(hc.nodeUpdateHasHeartbeatData(inP))

	// proxy not ready
	res := hc.nodeUpdate(inP)
	assert.NotNil(res)
	errRes, ok := res.(*ops.NodeUpdateDefault)
	assert.True(ok)
	assert.NotNil(errRes)
	assert.Regexp("initializing", *errRes.Payload.Message)
	assert.Equal(int32(503), errRes.Payload.Code)

	// now ready
	fs := fakeState.NewFakeClusterState()
	app.StateOps = fs
	fsu := &fcl.StateUpdater{RetHTPsec: 2019}
	app.StateUpdater = fsu

	// error
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	nOps := mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps).MinTimes(1)
	mN := mockmgmtclient.NewNodeMatcher(t, cinP)
	nOps.EXPECT().NodeUpdate(mN).Return(nil, resD).MinTimes(1)
	app.ClientAPI = mAPI
	res = hc.nodeUpdate(inP)
	assert.NotNil(res)
	errRes, ok = res.(*ops.NodeUpdateDefault)
	assert.True(ok)
	assert.NotNil(errRes)
	assert.Equal(resD.Payload, errRes.Payload)
	mockCtrl.Finish()
	tl.Flush()

	// success
	fs.InNSSid = ""
	fs.InNSSss = nil
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	nOps = mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps).MinTimes(1)
	nOps.EXPECT().NodeUpdate(mN).Return(resOk, nil).MinTimes(1)
	app.ClientAPI = mAPI
	res = hc.nodeUpdate(inP)
	okRes, ok := res.(*ops.NodeUpdateOK)
	assert.True(ok)
	assert.NotNil(okRes)
	assert.Equal(resOk.Payload, okRes.Payload)
	assert.Equal(inP.ID, fs.InNSSid)
	assert.Equal(&inP.Payload.Service.ServiceState, fs.InNSSss)
	assert.EqualValues(2019, inP.Payload.Service.HeartbeatPeriodSecs)

	// test update with heartbeat-only signature
	fs.InNSSid = ""
	fs.InNSSss = nil
	res = hc.nodeUpdate(hboInP)
	okRes, ok = res.(*ops.NodeUpdateOK)
	assert.True(ok)
	assert.NotNil(okRes)
	assert.Equal(&models.Node{}, okRes.Payload)
	assert.Equal(hboInP.ID, fs.InNSSid)
	assert.Equal(&hboInP.Payload.Service.ServiceState, fs.InNSSss)
}
