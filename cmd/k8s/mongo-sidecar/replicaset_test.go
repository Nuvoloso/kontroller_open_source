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


package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/cluster"
	clusterMock "github.com/Nuvoloso/kontroller/pkg/cluster/mock"
	"github.com/Nuvoloso/kontroller/pkg/mongodb"
	"github.com/Nuvoloso/kontroller/pkg/mongodb/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestCheckReplicaSet(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	t.Log("case: success, ready, already configured")
	ops := &fakeOps{}
	app := &mainContext{log: l, ops: ops, PodName: "mongo-1"}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := mock.NewMockDBAPI(mockCtrl)
	app.db = api
	api.EXPECT().MustBeReady().Return(nil)
	client := mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(client)
	db := mock.NewMockDatabase(mockCtrl)
	client.EXPECT().Database("admin").Return(db)
	ctx := context.Background()
	sr := mock.NewMockSingleResult(mockCtrl)
	db.EXPECT().RunCommand(ctx, bson.M{"replSetGetStatus": 1}).Return(sr)
	sr.EXPECT().Decode(newMockDecodeMatcher(t, bson.M{"ok": 1})).Return(nil)
	api.EXPECT().ErrorCode(nil).Return(mongodb.ECKeyNotFound)
	assert.NoError(app.CheckReplicaSet())
	assert.Zero(tl.CountPattern("Waiting for"))
	assert.Equal(1, tl.CountPattern("is configured"))
	mockCtrl.Finish()
	tl.Flush()

	t.Log("case: ready after a bit, not configured, not elected, becomes configured")
	app.MongoArgs.RetryIncrement = time.Millisecond
	app.MongoArgs.MaxRetryInterval = 2 * time.Millisecond
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockDBAPI(mockCtrl)
	app.db = api
	prev := api.EXPECT().MustBeReady().Return(errUnknownError).Times(3)
	api.EXPECT().MustBeReady().Return(nil).MinTimes(1).After(prev)
	client = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(client).MinTimes(1)
	db = mock.NewMockDatabase(mockCtrl)
	client.EXPECT().Database("admin").Return(db).MinTimes(1)
	sr = mock.NewMockSingleResult(mockCtrl)
	db.EXPECT().RunCommand(ctx, bson.M{"replSetGetStatus": 1}).Return(sr).Times(2)
	cmdErr := mongo.CommandError{Code: 94}
	prev = sr.EXPECT().Decode(newMockDecodeMatcher(t, nil)).Return(cmdErr)
	sr.EXPECT().Decode(newMockDecodeMatcher(t, bson.M{"ok": 1})).Return(nil).After(prev)
	api.EXPECT().ErrorCode(nil).Return(mongodb.ECKeyNotFound).MinTimes(2)
	assert.NoError(app.CheckReplicaSet())
	assert.Zero(ops.InitiateReplicaSetCnt)
	assert.Equal(1, tl.CountPattern("Waiting for"))
	assert.Equal(1, tl.CountPattern("Not elected"))
	assert.Equal(1, tl.CountPattern("is configured"))
	mockCtrl.Finish()
	tl.Flush()

	t.Log("case: ready, not configured, cover more error retry paths, elected, becomes configured")
	app.PodName = "mongo-0"
	ops.InitiateReplicaSetFailCnt = 1
	ops.RetInitiateReplicaSetErr = errors.New("uninitiated")
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockDBAPI(mockCtrl)
	app.db = api
	api.EXPECT().MustBeReady().Return(nil).MinTimes(1)
	client = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(client).MinTimes(1)
	db = mock.NewMockDatabase(mockCtrl)
	client.EXPECT().Database("admin").Return(db).MinTimes(1)
	sr = mock.NewMockSingleResult(mockCtrl)
	db.EXPECT().RunCommand(ctx, bson.M{"replSetGetStatus": 1}).Return(sr).Times(4)
	prev = sr.EXPECT().Decode(newMockDecodeMatcher(t, nil)).Return(errUnknownError)
	prev = sr.EXPECT().Decode(newMockDecodeMatcher(t, nil)).Return(cmdErr).After(prev).Times(2)
	sr.EXPECT().Decode(newMockDecodeMatcher(t, bson.M{"ok": 1})).Return(nil).After(prev)
	api.EXPECT().ErrorCode(errUnknownError).Return(mongodb.ECUnknownError)
	api.EXPECT().ErrorCode(nil).Return(mongodb.ECKeyNotFound).MinTimes(2)
	assert.NoError(app.CheckReplicaSet())
	assert.Equal(2, ops.InitiateReplicaSetCnt)
	assert.Equal(2, tl.CountPattern("Elected to initiate"))
	assert.Equal(1, tl.CountPattern("has been initiated"))
	assert.Equal(1, tl.CountPattern("is configured"))
}

func TestInitiateReplicaSet(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	t.Log("case: PodFetch fails")
	ops := &fakeOps{}
	app := &mainContext{log: l, ops: ops, ClientTimeout: 25, PodName: "mongo-0", Namespace: "ns"}
	to := time.Duration(app.ClientTimeout) * time.Second
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cc := clusterMock.NewMockClient(mockCtrl)
	app.clusterClient = cc
	pArgs := &cluster.PodFetchArgs{Name: "mongo-0", Namespace: "ns"}
	cc.EXPECT().PodFetch(testutils.NewCtxTimeoutMatcher(to), pArgs).Return(nil, errUnknownError)
	err := app.InitiateReplicaSet()
	assert.Equal(errUnknownError, err)
	mockCtrl.Finish()

	t.Log("case: PodFetch no controller")
	app = &mainContext{log: l, ops: ops, ClientTimeout: 25, PodName: "mongo-0", Namespace: "ns"}
	mockCtrl = gomock.NewController(t)
	cc = clusterMock.NewMockClient(mockCtrl)
	app.clusterClient = cc
	pArgs = &cluster.PodFetchArgs{Name: "mongo-0", Namespace: "ns"}
	cc.EXPECT().PodFetch(testutils.NewCtxTimeoutMatcher(to), pArgs).Return(&cluster.PodObj{}, nil)
	err = app.InitiateReplicaSet()
	assert.Regexp("pod mongo-0 is not in", err)
	mockCtrl.Finish()

	t.Log("case: wrong controller kind")
	app = &mainContext{log: l, ops: ops, ClientTimeout: 25, PodName: "mongo-0", Namespace: "ns"}
	mockCtrl = gomock.NewController(t)
	cc = clusterMock.NewMockClient(mockCtrl)
	app.clusterClient = cc
	cc.EXPECT().PodFetch(testutils.NewCtxTimeoutMatcher(to), pArgs).Return(&cluster.PodObj{ControllerName: "foo"}, nil)
	err = app.InitiateReplicaSet()
	assert.Regexp("pod mongo-0 is not in", err)
	mockCtrl.Finish()

	t.Log("case: ControllerFetch fails")
	app = &mainContext{log: l, ops: ops, ClientTimeout: 25, PodName: "mongo-0", Namespace: "ns"}
	mockCtrl = gomock.NewController(t)
	cc = clusterMock.NewMockClient(mockCtrl)
	app.clusterClient = cc
	pRet := &cluster.PodObj{Name: "mongo-0", Namespace: "ns", ControllerName: "mongo", ControllerKind: "StatefulSet"}
	cc.EXPECT().PodFetch(testutils.NewCtxTimeoutMatcher(to), pArgs).Return(pRet, nil)
	cArgs := &cluster.ControllerFetchArgs{Kind: "StatefulSet", Name: "mongo", Namespace: "ns"}
	cc.EXPECT().ControllerFetch(testutils.NewCtxTimeoutMatcher(to), cArgs).Return(nil, errUnknownError)
	err = app.InitiateReplicaSet()
	assert.Equal(errUnknownError, err)
	mockCtrl.Finish()

	t.Log("case: ControllerFetch invalid replicas")
	app = &mainContext{log: l, ops: ops, ClientTimeout: 25, PodName: "mongo-0", Namespace: "ns"}
	mockCtrl = gomock.NewController(t)
	cc = clusterMock.NewMockClient(mockCtrl)
	app.clusterClient = cc
	cc.EXPECT().PodFetch(testutils.NewCtxTimeoutMatcher(to), pArgs).Return(pRet, nil)
	cRet := &cluster.ControllerObj{Replicas: 2, ReadyReplicas: 2}
	cc.EXPECT().ControllerFetch(testutils.NewCtxTimeoutMatcher(to), cArgs).Return(cRet, nil)
	err = app.InitiateReplicaSet()
	assert.Regexp("replicas must be 1 or 3", err)
	mockCtrl.Finish()

	t.Log("case: ControllerFetch replicas not ready")
	app = &mainContext{log: l, ops: ops, ClientTimeout: 25, PodName: "mongo-0", Namespace: "ns"}
	mockCtrl = gomock.NewController(t)
	cc = clusterMock.NewMockClient(mockCtrl)
	app.clusterClient = cc
	cc.EXPECT().PodFetch(testutils.NewCtxTimeoutMatcher(to), pArgs).Return(pRet, nil)
	cRet = &cluster.ControllerObj{Replicas: 3, ReadyReplicas: 1}
	cc.EXPECT().ControllerFetch(testutils.NewCtxTimeoutMatcher(to), cArgs).Return(cRet, nil)
	err = app.InitiateReplicaSet()
	assert.Regexp("some replicas are not ready", err)
	mockCtrl.Finish()

	t.Log("case: 1 replica, no port in URL, Decode error")
	app = &mainContext{log: l, ops: ops, ClientTimeout: 25, ClusterDNSName: "cluster.local", PodName: "mongo-0", Namespace: "ns", ReplicaSetName: "rs"}
	app.MongoArgs.URL = "mongodb://localhost"
	mockCtrl = gomock.NewController(t)
	cc = clusterMock.NewMockClient(mockCtrl)
	app.clusterClient = cc
	api := mock.NewMockDBAPI(mockCtrl)
	app.db = api
	cc.EXPECT().PodFetch(testutils.NewCtxTimeoutMatcher(to), pArgs).Return(pRet, nil)
	cRet = &cluster.ControllerObj{Kind: "StatefulSet", Name: "mongo", Namespace: "ns", ServiceName: "config", Replicas: 1, ReadyReplicas: 1}
	cc.EXPECT().ControllerFetch(testutils.NewCtxTimeoutMatcher(to), cArgs).Return(cRet, nil)
	client := mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(client)
	db := mock.NewMockDatabase(mockCtrl)
	client.EXPECT().Database("admin").Return(db)
	ctx := context.Background()
	sr := mock.NewMockSingleResult(mockCtrl)
	cmd := bson.M{
		"replSetInitiate": bson.M{
			"_id": "rs",
			"members": bson.A{
				bson.M{"_id": 0, "host": "mongo-0.config.ns.svc.cluster.local:27017"},
			},
		},
	}
	db.EXPECT().RunCommand(ctx, cmd).Return(sr)
	sr.EXPECT().Decode(newMockDecodeMatcher(t, nil)).Return(errUnknownError)
	api.EXPECT().ErrorCode(errUnknownError).Return(mongodb.ECUnknownError)
	err = app.InitiateReplicaSet()
	assert.Equal(errUnknownError, err)
	mockCtrl.Finish()

	t.Log("case: success, 3 replicas, port in URL")
	app = &mainContext{log: l, ops: ops, ClientTimeout: 25, ClusterDNSName: "cluster.local", PodName: "mongo-0", Namespace: "ns", ReplicaSetName: "rs"}
	app.MongoArgs.URL = "mongodb://localhost:27018"
	mockCtrl = gomock.NewController(t)
	cc = clusterMock.NewMockClient(mockCtrl)
	app.clusterClient = cc
	api = mock.NewMockDBAPI(mockCtrl)
	app.db = api
	cc.EXPECT().PodFetch(testutils.NewCtxTimeoutMatcher(to), pArgs).Return(pRet, nil)
	cRet = &cluster.ControllerObj{Kind: "StatefulSet", Name: "mongo", Namespace: "ns", ServiceName: "config", Replicas: 3, ReadyReplicas: 3}
	cc.EXPECT().ControllerFetch(testutils.NewCtxTimeoutMatcher(to), cArgs).Return(cRet, nil)
	client = mock.NewMockClient(mockCtrl)
	api.EXPECT().Client().Return(client)
	db = mock.NewMockDatabase(mockCtrl)
	client.EXPECT().Database("admin").Return(db)
	sr = mock.NewMockSingleResult(mockCtrl)
	cmd = bson.M{
		"replSetInitiate": bson.M{
			"_id": "rs",
			"members": bson.A{
				bson.M{"_id": 0, "host": "mongo-0.config.ns.svc.cluster.local:27018"},
				bson.M{"_id": 1, "host": "mongo-1.config.ns.svc.cluster.local:27018"},
				bson.M{"_id": 2, "host": "mongo-2.config.ns.svc.cluster.local:27018"},
			},
		},
	}
	db.EXPECT().RunCommand(ctx, cmd).Return(sr)
	sr.EXPECT().Decode(newMockDecodeMatcher(t, bson.M{"ok": 1})).Return(nil)
	api.EXPECT().ErrorCode(nil).Return(mongodb.ECKeyNotFound)
	assert.NoError(app.InitiateReplicaSet())
}

func TestMonitor(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	l := tl.Logger()

	app := &mainContext{log: l, stopMonitor: make(chan struct{})}
	go app.Monitor()
	for tl.CountPattern("Blocking until terminated") == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	assert.Zero(tl.CountPattern("Monitor stopped"))
	close(app.stopMonitor)
	for tl.CountPattern("Monitor stopped") == 0 {
		time.Sleep(10 * time.Millisecond)
	}
}

type decodeMatcher struct {
	buf []byte
}

// newMockDecodeMatcher constructs a matcher for the mongo Decode() argument.
// The m parameter is any type that can be passed to bson.Marshal(), eg bson.M or a struct pointer.
// m may also be nil to skip attempting to use the value in Matches, for use in error conditions.
func newMockDecodeMatcher(t *testing.T, m interface{}) gomock.Matcher {
	if m == nil {
		return &decodeMatcher{}
	}
	buf, err := bson.Marshal(m)
	if !assert.NoError(t, err) {
		assert.FailNow(t, "bson.Marshal should not fail")
	}
	return &decodeMatcher{buf: buf}
}

func (m *decodeMatcher) Matches(x interface{}) bool {
	if len(m.buf) == 0 { // nothing to copy, used to cover Decode returning an error, destination object must not be nil
		return x != nil
	}
	if x != nil {
		return bson.Unmarshal(m.buf, x) == nil
	}
	return false
}

func (m *decodeMatcher) String() string {
	if m.buf != nil {
		return "decoder matches object"
	}
	return "decoder matches nil"
}
