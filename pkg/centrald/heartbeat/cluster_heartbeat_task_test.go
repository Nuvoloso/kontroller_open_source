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


package heartbeat

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/cluster"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi/operations/node"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	fal "github.com/Nuvoloso/kontroller/pkg/centrald/audit/fake"
	"github.com/Nuvoloso/kontroller/pkg/centrald/auth"
	"github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	"github.com/Nuvoloso/kontroller/pkg/common"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	hk "github.com/Nuvoloso/kontroller/pkg/housekeeping"
	fhk "github.com/Nuvoloso/kontroller/pkg/housekeeping/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestChtRegisterAndValidate(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	fts := &fhk.TaskScheduler{}
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log:                       tl.Logger(),
			ClusterTimeoutCheckPeriod: 5 * time.Minute,
		},
		TaskScheduler: fts,
	}
	hb := newHBComp()
	hb.app = app
	hb.Log = app.Log

	cht := chtRegisterAnimator(hb)
	assert.NotNil(cht)
	assert.NotNil(cht.ops)
	assert.Equal(cht, cht.ops) // initialized
	assert.Equal(app.ClusterTimeoutCheckPeriod, cht.timeoutCheckPeriod)
	assert.Equal(2, app.ClusterTimeoutAfterMisses) // default if incorrect
	assert.EqualValues(app.ClusterTimeoutAfterMisses, cht.clusterMaxMissed)

	// TaskValidate
	ca := &models.TaskCreateOnce{}
	err := cht.TaskValidate(ca)
	assert.Error(err)
	assert.Equal(hk.ErrInvalidAnimator, err)

	invalidTCO := []*models.TaskCreateOnce{
		&models.TaskCreateOnce{Operation: common.TaskClusterHeartbeat},
		&models.TaskCreateOnce{Operation: common.TaskClusterHeartbeat, ObjectID: "CL-1"},
		&models.TaskCreateOnce{
			Operation: common.TaskClusterHeartbeat,
			ObjectID:  "CL-1",
			ServiceStates: map[string]models.ServiceState{
				"node-1": models.ServiceState{},
			},
		},
		&models.TaskCreateOnce{
			Operation: common.TaskClusterHeartbeat,
			ObjectID:  "CL-1",
			ServiceStates: map[string]models.ServiceState{
				"CL-1": models.ServiceState{},
			},
		},
		&models.TaskCreateOnce{
			Operation: common.TaskClusterHeartbeat,
			ObjectID:  "CL-1",
			ServiceStates: map[string]models.ServiceState{
				"CL-1": models.ServiceState{
					HeartbeatPeriodSecs: 10,
				},
			},
		},
		&models.TaskCreateOnce{
			Operation: common.TaskClusterHeartbeat,
			ObjectID:  "CL-1",
			ServiceStates: map[string]models.ServiceState{
				"CL-1": models.ServiceState{
					HeartbeatPeriodSecs: 10,
					State:               "READY",
				},
			},
		},
	}
	for i, tc := range invalidTCO {
		err = cht.TaskValidate(tc)
		assert.Error(err, "[%d]", i)
		assert.Equal(hk.ErrInvalidArguments, err, "[%d]", i)
	}

	validTC := &models.TaskCreateOnce{
		Operation: common.TaskClusterHeartbeat,
		ObjectID:  "CL-1",
		ServiceStates: map[string]models.ServiceState{
			"CL-1": models.ServiceState{
				HeartbeatPeriodSecs: 10,
				State:               "READY",
				HeartbeatTime:       strfmt.DateTime(time.Now()),
			},
			"NODE-1": models.ServiceState{
				HeartbeatPeriodSecs: 20,
				State:               "READY",
				HeartbeatTime:       strfmt.DateTime(time.Now()),
			},
		},
	}
	assert.NoError(cht.TaskValidate(validTC))
}

func TestChtExec(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	fts := &fhk.TaskScheduler{}
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log: tl.Logger(),
		},
		TaskScheduler: fts,
	}
	hb := newHBComp()
	hb.app = app
	hb.Log = app.Log
	cht := chtRegisterAnimator(hb) // tested elsewhere
	ctx := context.Background()

	now := time.Now()
	tO := &models.Task{
		TaskAllOf0: models.TaskAllOf0{
			Meta: &models.ObjMeta{ID: "TASK-1"},
		},
		TaskCreateOnce: models.TaskCreateOnce{
			ObjectID: "CL-1",
			ServiceStates: map[string]models.ServiceState{
				"CL-1": models.ServiceState{
					HeartbeatPeriodSecs: 10,
					State:               "READY",
					HeartbeatTime:       strfmt.DateTime(now),
				},
				"NODE-1": models.ServiceState{
					HeartbeatPeriodSecs: 40,
					State:               "READY",
					HeartbeatTime:       strfmt.DateTime(now),
				},
				"NODE-2": models.ServiceState{
					HeartbeatPeriodSecs: 40,
					State:               "READY",
					HeartbeatTime:       strfmt.DateTime(now),
				},
				"NODE-3": models.ServiceState{
					HeartbeatPeriodSecs: 40,
					State:               "STARTING",
					HeartbeatTime:       strfmt.DateTime(now),
				},
				"NODE-4": models.ServiceState{
					HeartbeatPeriodSecs: 40,
					State:               "STARTING",
					HeartbeatTime:       strfmt.DateTime(now),
				},
			},
		},
	}
	ssC1 := tO.ServiceStates["CL-1"]
	ssNode := tO.ServiceStates["NODE-1"] // random node would do as well
	clObj := &models.Cluster{}
	clObj.Service = &models.NuvoService{
		ServiceState: ssC1,
	}

	// failure if app does not have a CrudHelpers
	fto := &fhk.TaskOps{
		RetO: tO,
	}
	tO.Messages = nil
	assert.Nil(app.CrudHelpers)
	assert.Panics(func() { cht.TaskExec(ctx, fto) })
	assert.Equal(hk.TaskStateFailed, fto.InSs)
	assert.NotNil(tO.Messages)
	assert.Regexp("Not ready", tO.Messages[0].Message)

	// success on cluster update, one failure on each bulk node update, timeout 1 node
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mCrudHelpers := mock.NewMockAppCrudHelpers(mockCtrl)
	mCrudHelpers.EXPECT().RLock()
	mCrudHelpers.EXPECT().RUnlock()
	mCrudHelpers.EXPECT().NodeLock()
	mCrudHelpers.EXPECT().NodeUnlock()
	mCrudHelpers.EXPECT().ClusterLock()
	mCrudHelpers.EXPECT().ClusterUnlock()
	app.CrudHelpers = mCrudHelpers
	fto = &fhk.TaskOps{
		RetO: tO,
	}
	tO.Messages = nil
	fops := &fakeChtOps{}
	cht.ops = fops
	fops.RetUCObj = clObj
	fops.InUNodeIDs = nil
	fops.RetUNErr = nil
	fops.RetUNChg = 1
	fops.RetUNFound = 2
	fops.InTOcl = nil
	fops.RetTOChg = 1
	fops.RetTOFound = 1
	fops.RetTOErr = nil
	fops.InSNCcl = nil
	fops.InSNCnc = 0
	fops.InSNCnt = 0
	assert.NotPanics(func() { cht.TaskExec(ctx, fto) }) // did not abort
	assert.EqualValues(0, fto.InSs)                     // unset because TaskExec returned
	assert.Equal(fakeChtArg{ID: "CL-1", SS: &ssC1}, fops.InUC)
	assert.False(fops.InUCisCTT)
	assert.Len(fops.InUNodeIDs, 2)
	for st, nodeIDs := range fops.InUNodeIDs {
		assert.Len(nodeIDs, 2, "State %s", st)
		for _, nid := range nodeIDs {
			ss, found := tO.ServiceStates[nid]
			assert.True(found, "Node %s State %s", nid, st)
			assert.Equal(ss.State, st, "Node %s State %s", nid, st)
		}
	}
	assert.Len(fops.InUNss, 2)
	for st, ss := range fops.InUNss {
		assert.Equal(ssNode.HeartbeatTime.String(), ss.HeartbeatTime.String(), "%s: time", st)
		assert.Equal(ssNode.HeartbeatPeriodSecs, ss.HeartbeatPeriodSecs, "%s: period", st)
		assert.NotEqual(ssC1.HeartbeatPeriodSecs, ss.HeartbeatPeriodSecs, "%s: cluster period", st)
	}
	assert.Equal(clObj, fops.InTOcl)
	assert.Len(tO.Messages, 3)
	assert.Regexp("Updated cluster", tO.Messages[0].Message)
	assert.Regexp("Updated 2/4 nodes", tO.Messages[1].Message)
	assert.Regexp("Timed out 1/1 nodes", tO.Messages[2].Message)
	assert.Equal(clObj, fops.InSNCcl)
	assert.Equal(2, fops.InSNCnc)
	assert.Equal(1, fops.InSNCnt)
	mockCtrl.Finish()
	tl.Flush()

	// failure on cluster - task aborts; pass in a cluster timeout task
	msgs := util.NewMsgList(nil)
	msgs.Insert(chtTimeoutTaskMessage)
	tO.Messages = msgs.ToModel()
	mockCtrl = gomock.NewController(t)
	mCrudHelpers = mock.NewMockAppCrudHelpers(mockCtrl)
	mCrudHelpers.EXPECT().RLock()
	mCrudHelpers.EXPECT().RUnlock()
	mCrudHelpers.EXPECT().NodeLock()
	mCrudHelpers.EXPECT().NodeUnlock()
	mCrudHelpers.EXPECT().ClusterLock()
	mCrudHelpers.EXPECT().ClusterUnlock()
	app.CrudHelpers = mCrudHelpers
	fto = &fhk.TaskOps{
		RetO: tO,
	}
	fops = &fakeChtOps{}
	cht.ops = fops
	fops.RetUCErr = fmt.Errorf("cluster-error")
	assert.Panics(func() { cht.TaskExec(ctx, fto) })
	assert.Equal(hk.TaskStateFailed, fto.InSs)
	assert.Len(tO.Messages, 2)
	assert.Regexp("Failed to update cluster", tO.Messages[1].Message)
	assert.True(fops.InUCisCTT)
}

func TestChtTimeoutClusters(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	fts := &fhk.TaskScheduler{}
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log:                       tl.Logger(),
			ClusterTimeoutCheckPeriod: 5 * time.Minute,
		},
		TaskScheduler: fts,
	}
	hb := newHBComp()
	hb.app = app
	hb.Log = app.Log
	cht := chtRegisterAnimator(hb) // tested elsewhere
	ctx := context.Background()

	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{ID: "CLUSTER-1", Version: 20},
		},
	}

	// check period constraints
	cht.lastTimeOutCheck = time.Now()
	assert.True(time.Now().Before(cht.lastTimeOutCheck.Add(cht.timeoutCheckPeriod)))
	assert.NoError(cht.TimeoutClusters(ctx))
	cht.lastTimeOutCheck = time.Time{}

	// list failure
	tl.Logger().Info("case: list failure")
	tl.Flush()
	fops := &fakeChtOps{}
	cht.ops = fops
	fops.RetGTCErr = fmt.Errorf("list-error")
	err := cht.TimeoutClusters(ctx)
	assert.Error(err)
	assert.Regexp("list-error", err)

	// task scheduler failure
	tl.Logger().Info("case: run task error")
	tl.Flush()
	fts.RetRtErr = fmt.Errorf("run-error")
	fops = &fakeChtOps{}
	cht.ops = fops
	fops.RetGTCList = []*models.Cluster{clObj}
	fops.RetMTT = &models.Task{}
	tB := time.Now()
	err = cht.TimeoutClusters(ctx)
	assert.Error(err)
	assert.Regexp("run-error", err)
	now := time.Now()
	assert.WithinDuration(tB, fops.InGTCt, now.Sub(tB))
	assert.WithinDuration(tB, fops.InMTTt, now.Sub(tB))

	// task launched
	tl.Logger().Info("case: task launched")
	tl.Flush()
	fts.RetRtErr = nil
	fts.RetRtID = "cleanup-task"
	tB = time.Now()
	err = cht.TimeoutClusters(ctx)
	assert.NoError(err)
	assert.Equal(1, tl.CountPattern("cleanup-task"))
	now = time.Now()
	assert.WithinDuration(tB, cht.lastTimeOutCheck, now.Sub(tB))
}

type fakeChtArg struct {
	ID string
	SS *models.ServiceState
}

// fakeChtOps fakes chtOps
type fakeChtOps struct {
	InUC      fakeChtArg
	InUCisCTT bool
	RetUCObj  *models.Cluster
	RetUCErr  error

	InUNodeIDs map[string][]string
	InUNss     map[string]*models.ServiceState
	RetUNChg   int
	RetUNFound int
	RetUNErr   error

	InTOcl     *models.Cluster
	RetTOChg   int
	RetTOFound int
	RetTOErr   error

	InSNCcl   *models.Cluster
	InSNCnc   int
	InSNCnt   int
	RetSNCErr error

	InGTCt     time.Time
	RetGTCList []*models.Cluster
	RetGTCErr  error

	InMTTcl *models.Cluster
	InMTTt  time.Time
	RetMTT  *models.Task

	InRNToALCtx     context.Context
	InRNToALClObj   *models.Cluster
	InRNToALNodeIDs []string
}

func (op *fakeChtOps) updateCluster(ctx context.Context, clID string, ss *models.ServiceState, isClusterTimeoutTask bool) (*models.Cluster, error) {
	op.InUC.ID = clID
	op.InUC.SS = ss
	op.InUCisCTT = isClusterTimeoutTask
	return op.RetUCObj, op.RetUCErr
}

func (op *fakeChtOps) updateNodes(ctx context.Context, nIDs []string, ss *models.ServiceState) (int, int, error) {
	if op.InUNodeIDs == nil {
		op.InUNodeIDs = make(map[string][]string)
		op.InUNss = make(map[string]*models.ServiceState)
	}
	op.InUNodeIDs[ss.State] = nIDs
	op.InUNss[ss.State] = ss
	return op.RetUNChg, op.RetUNFound, op.RetUNErr
}

func (op *fakeChtOps) timeoutNodes(ctx context.Context, clObj *models.Cluster) (int, int, error) {
	op.InTOcl = clObj
	return op.RetTOChg, op.RetTOFound, op.RetTOErr
}

func (op *fakeChtOps) sendNodeCrudEvent(clObj *models.Cluster, numNodeChanges, numNodeTimeouts int) error {
	op.InSNCcl = clObj
	op.InSNCnc = numNodeChanges
	op.InSNCnt = numNodeTimeouts
	return op.RetSNCErr
}

func (op *fakeChtOps) getTimedoutClusters(ctx context.Context, timeoutTime time.Time) ([]*models.Cluster, error) {
	op.InGTCt = timeoutTime
	return op.RetGTCList, op.RetGTCErr
}

func (op *fakeChtOps) makeTimeoutTask(clObj *models.Cluster, timeoutTime time.Time) *models.Task {
	op.InMTTcl = clObj
	op.InMTTt = timeoutTime
	return op.RetMTT
}

func (op *fakeChtOps) reportTimedoutNodesToAuditLog(ctx context.Context, clObj *models.Cluster) {
	op.InRNToALCtx = ctx
	op.InRNToALClObj = clObj
}

func (op *fakeChtOps) reportRestartedNodesToAuditLog(ctx context.Context, clObj *models.Cluster, knownIDs []string) {
	op.InRNToALCtx = ctx
	op.InRNToALClObj = clObj
	op.InRNToALNodeIDs = knownIDs
}

func TestChtUpdateCluster(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	fts := &fhk.TaskScheduler{}
	evM := fev.NewFakeEventManager()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log:      tl.Logger(),
			CrudeOps: evM,
		},
		TaskScheduler: fts,
	}
	hb := newHBComp()
	hb.app = app
	hb.Log = app.Log
	cht := chtRegisterAnimator(hb) // tested elsewhere
	ctx := context.Background()

	clusterObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{ID: "CLUSTER-1", Version: 20},
		},
	}
	clusterObj.State = com.ClusterStateManaged
	var clObj *models.Cluster
	clID := string(clusterObj.Meta.ID)
	ss := &models.ServiceState{
		HeartbeatPeriodSecs: 10,
		State:               "READY",
		HeartbeatTime:       strfmt.DateTime(time.Now()),
	}
	clUObj := &models.ClusterMutable{}
	clUObj.State = clusterObj.State
	clUObj.Service = &models.NuvoService{
		ServiceState: models.ServiceState{
			State:               ss.State,
			HeartbeatPeriodSecs: ss.HeartbeatPeriodSecs,
			HeartbeatTime:       ss.HeartbeatTime,
		},
	}

	retUA := &centrald.UpdateArgs{} // content not relevant to this test

	// test clusterTimeoutAfter
	expTimeoutAfter := time.Time(ss.HeartbeatTime).Add(time.Duration(cht.clusterMaxMissed*ss.HeartbeatPeriodSecs) * time.Second)
	assert.Equal(expTimeoutAfter, cht.clusterTimeoutAfter(ss))

	// fetch failure
	tl.Logger().Info("case: Fetch failure")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mClOps := mock.NewMockClusterOps(mockCtrl)
	mClOps.EXPECT().Fetch(ctx, clID).Return(nil, centrald.ErrorNotFound)
	mDS := mock.NewMockDataStore(mockCtrl)
	mDS.EXPECT().OpsCluster().Return(mClOps).Times(1)
	app.DS = mDS
	o, err := cht.updateCluster(ctx, clID, ss, false)
	assert.Error(err)
	assert.Regexp(centrald.ErrorNotFound.M, err)
	mockCtrl.Finish()

	// cluster cleanup race TBD
	// last heartbeat applied while TO task being launched: use 1ns after the expiry window
	tl.Logger().Info("case: detected Timeout/Heartbeat race")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	lastHbT := time.Time(ss.HeartbeatTime).Add(time.Duration(-cht.clusterMaxMissed*ss.HeartbeatPeriodSecs)*time.Second + time.Nanosecond)
	testutils.Clone(clusterObj, &clObj)
	clObj.Service = &models.NuvoService{
		ServiceState: models.ServiceState{
			HeartbeatPeriodSecs: ss.HeartbeatPeriodSecs,
			HeartbeatTime:       strfmt.DateTime(lastHbT),
		},
	}
	assert.True(time.Time(ss.HeartbeatTime).Before(cht.clusterTimeoutAfter(&clObj.Service.ServiceState)))
	mClOps = mock.NewMockClusterOps(mockCtrl)
	mClOps.EXPECT().Fetch(ctx, clID).Return(clObj, nil)
	mDS = mock.NewMockDataStore(mockCtrl)
	mDS.EXPECT().OpsCluster().Return(mClOps).Times(1)
	app.DS = mDS
	o, err = cht.updateCluster(ctx, clID, ss, true) // emulate timeout task invocation
	assert.Error(err)
	assert.Regexp("race.*timeout.*heartbeat", err)
	mockCtrl.Finish()

	// MakeStdUpdateArgs failure
	tl.Logger().Info("case: MakeStdUpdateArgs failure")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	testutils.Clone(clusterObj, &clObj)
	msuM := mock.NewMakeStdUpdateArgsObjMatcher(t).WithObj(clObj).WithID(clID).WithFields(chtServiceStateUpdatedFields)
	mCrudHelpers := mock.NewMockAppCrudHelpers(mockCtrl)
	mCrudHelpers.EXPECT().MakeStdUpdateArgs(msuM, msuM, msuM, msuM).Return(nil, fmt.Errorf("ua-error"))
	app.CrudHelpers = mCrudHelpers
	mClOps = mock.NewMockClusterOps(mockCtrl)
	mClOps.EXPECT().Fetch(ctx, clID).Return(clObj, nil)
	mDS = mock.NewMockDataStore(mockCtrl)
	mDS.EXPECT().OpsCluster().Return(mClOps).Times(1)
	app.DS = mDS
	o, err = cht.updateCluster(ctx, clID, ss, false)
	assert.Error(err)
	assert.Regexp("ua-error", err)
	mockCtrl.Finish()

	// update failure
	tl.Logger().Info("case: Update failure")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	testutils.Clone(clusterObj, &clObj)
	mCrudHelpers = mock.NewMockAppCrudHelpers(mockCtrl)
	mCrudHelpers.EXPECT().MakeStdUpdateArgs(msuM, msuM, msuM, msuM).Return(retUA, nil)
	app.CrudHelpers = mCrudHelpers
	mClOps = mock.NewMockClusterOps(mockCtrl)
	mClOps.EXPECT().Fetch(ctx, clID).Return(clObj, nil)
	uaM := mock.NewUpdateArgsMatcher(t, retUA)
	cmM := NewClusterMutableMatcher(t, clUObj)
	mClOps.EXPECT().Update(ctx, uaM, cmM).Return(nil, centrald.ErrorDbError)
	mDS = mock.NewMockDataStore(mockCtrl)
	mDS.EXPECT().OpsCluster().Return(mClOps).Times(2)
	app.DS = mDS
	o, err = cht.updateCluster(ctx, clID, ss, false)
	assert.Error(err)
	assert.Regexp(centrald.ErrorDbError.M, err)
	mockCtrl.Finish()

	// update success with transition to TimedOut state
	tl.Logger().Info("case: Update success, CrudE posted, MANAGED => TIMED_OUT")
	tl.Flush()
	testutils.Clone(clusterObj, &clObj)
	assert.Equal(com.ClusterStateManaged, clObj.State)
	ss.State = com.ServiceStateUnknown
	msuM = mock.NewMakeStdUpdateArgsObjMatcher(t).WithObj(clObj).WithID(clID).WithFields(chtClusterTimeoutStateChangeUpdateFields)
	evM.InIEev = nil
	mockCtrl = gomock.NewController(t)
	mCrudHelpers = mock.NewMockAppCrudHelpers(mockCtrl)
	mCrudHelpers.EXPECT().MakeStdUpdateArgs(msuM, msuM, msuM, msuM).Return(retUA, nil)
	app.CrudHelpers = mCrudHelpers
	mClOps = mock.NewMockClusterOps(mockCtrl)
	mClOps.EXPECT().Fetch(ctx, clID).Return(clObj, nil)
	var clUTObj *models.ClusterMutable
	testutils.Clone(clUObj, &clUTObj)
	clUTObj.State = com.ClusterStateTimedOut
	clUTObj.Messages = []*models.TimestampedString{
		&models.TimestampedString{Message: "State change: MANAGED ⇒ TIMED_OUT"},
	}
	clUTObj.Service.ServiceState.State = ss.State
	clUTObj.Service.ServiceState.HeartbeatPeriodSecs = ss.HeartbeatPeriodSecs
	clUTObj.Service.ServiceState.HeartbeatTime = ss.HeartbeatTime
	cmM = NewClusterMutableMatcher(t, clUTObj)
	mClOps.EXPECT().Update(ctx, uaM, cmM).Return(clObj, nil)
	mDS = mock.NewMockDataStore(mockCtrl)
	mDS.EXPECT().OpsCluster().Return(mClOps).Times(2)
	app.DS = mDS
	o, err = cht.updateCluster(ctx, clID, ss, false)
	assert.NoError(err)
	assert.Equal(clObj, o)
	assert.NotNil(evM.InIEev)
	expCE := &crude.CrudEvent{
		Method:             "PATCH",
		TrimmedURI:         "/clusters/CLUSTER-1?set=service.state&set=service.heartbeatTime&set=service.heartbeatPeriodSecs&set=state&set=messages&version=20",
		AccessControlScope: clObj,
	}
	assert.Equal(expCE, evM.InIEev)
}

func TestChtUpdateNodes(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	fts := &fhk.TaskScheduler{}
	evM := fev.NewFakeEventManager()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log:      tl.Logger(),
			CrudeOps: evM,
		},
		TaskScheduler: fts,
	}
	hb := newHBComp()
	hb.app = app
	hb.Log = app.Log
	cht := chtRegisterAnimator(hb) // tested elsewhere
	ctx := context.Background()

	ss := &models.ServiceState{
		HeartbeatPeriodSecs: 10,
		State:               "UNKNOWN",
		HeartbeatTime:       strfmt.DateTime(time.Now()),
	}
	nIDs := []string{"NODE-1", "NODE-2"}
	nodeMutable := &models.NodeMutable{
		Service: &models.NuvoService{
			ServiceState: *ss,
		},
		State: common.NodeStateTimedOut,
	}

	retUA := &centrald.UpdateArgs{} // content not relevant to this test

	// MakeStdUpdateArgs failure
	tl.Logger().Info("case: MakeStdUpdateArgs failure")
	tl.Flush()
	msuM := mock.NewMakeStdUpdateArgsObjMatcher(t).WithObj(&models.Node{}).WithFields(chtNodeStateUpdatedFields)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mCrudHelpers := mock.NewMockAppCrudHelpers(mockCtrl)
	mCrudHelpers.EXPECT().MakeStdUpdateArgs(msuM, msuM, msuM, msuM).Return(nil, fmt.Errorf("ua-error"))
	app.CrudHelpers = mCrudHelpers
	chgN, foundN, err := cht.updateNodes(ctx, nIDs, ss)
	assert.Error(err)
	assert.Zero(chgN)
	assert.Zero(foundN)
	assert.Regexp("ua-error", err)
	mockCtrl.Finish()

	// success
	tl.Logger().Info("case: Update success (TIMED_OUT/UNKNOWN)")
	tl.Flush()
	uaM := mock.NewUpdateArgsMatcher(t, retUA)
	nLP := node.NewNodeListParams()
	nLP.NodeIds = nIDs
	nLP.StateNE = swag.String(common.NodeStateTearDown)
	mockCtrl = gomock.NewController(t)
	mCrudHelpers = mock.NewMockAppCrudHelpers(mockCtrl)
	mCrudHelpers.EXPECT().MakeStdUpdateArgs(msuM, msuM, msuM, msuM).Return(retUA, nil)
	app.CrudHelpers = mCrudHelpers
	mNOps := mock.NewMockNodeOps(mockCtrl)
	mNOps.EXPECT().UpdateMultiple(ctx, nLP, uaM, nodeMutable).Return(1, 2, nil)
	mDS := mock.NewMockDataStore(mockCtrl)
	mDS.EXPECT().OpsNode().Return(mNOps).Times(1)
	app.DS = mDS
	chgN, foundN, err = cht.updateNodes(ctx, nIDs, ss)
	assert.NoError(err)
	assert.Equal(1, chgN)
	assert.Equal(2, foundN)

	// success
	tl.Logger().Info("case: Update success (MANAGED/READY)")
	tl.Flush()
	ss.State = common.ServiceStateReady
	nodeMutable.State = common.NodeStateManaged
	nodeMutable.Service.State = common.ServiceStateReady
	uaM = mock.NewUpdateArgsMatcher(t, retUA)
	nLP = node.NewNodeListParams()
	nLP.NodeIds = nIDs
	nLP.StateNE = swag.String(common.NodeStateTearDown)
	mockCtrl = gomock.NewController(t)
	mCrudHelpers = mock.NewMockAppCrudHelpers(mockCtrl)
	mCrudHelpers.EXPECT().MakeStdUpdateArgs(msuM, msuM, msuM, msuM).Return(retUA, nil)
	app.CrudHelpers = mCrudHelpers
	mNOps = mock.NewMockNodeOps(mockCtrl)
	mNOps.EXPECT().UpdateMultiple(ctx, nLP, uaM, nodeMutable).Return(1, 2, nil)
	mDS = mock.NewMockDataStore(mockCtrl)
	mDS.EXPECT().OpsNode().Return(mNOps).Times(1)
	app.DS = mDS
	chgN, foundN, err = cht.updateNodes(ctx, nIDs, ss)
	assert.NoError(err)
	assert.Equal(1, chgN)
	assert.Equal(2, foundN)
}

func TestChtTimeoutNodes(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	fts := &fhk.TaskScheduler{}
	evM := fev.NewFakeEventManager()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log:      tl.Logger(),
			CrudeOps: evM,
		},
		TaskScheduler: fts,
	}
	hb := newHBComp()
	hb.app = app
	hb.Log = app.Log
	cht := chtRegisterAnimator(hb) // tested elsewhere
	ctx := context.Background()

	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{ID: "CLUSTER-1"},
		},
		ClusterMutable: models.ClusterMutable{
			ClusterMutableAllOf0: models.ClusterMutableAllOf0{
				Service: &models.NuvoService{
					ServiceState: models.ServiceState{
						HeartbeatPeriodSecs: 10,
						State:               "READY",
						HeartbeatTime:       strfmt.DateTime(time.Now()),
					},
				},
			},
		},
	}
	nodeMutable := &models.NodeMutable{
		Service: &models.NuvoService{
			ServiceState: models.ServiceState{
				State: common.ServiceStateUnknown,
			},
		},
		State: common.NodeStateTimedOut,
	}

	retUA := &centrald.UpdateArgs{} // content not relevant to this test

	// MakeStdUpdateArgs failure
	tl.Logger().Info("case: MakeStdUpdateArgs failure")
	tl.Flush()
	msuM := mock.NewMakeStdUpdateArgsObjMatcher(t).WithObj(&models.Node{}).WithFields(chtNodeTimeoutUpdateFields)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mCrudHelpers := mock.NewMockAppCrudHelpers(mockCtrl)
	mCrudHelpers.EXPECT().MakeStdUpdateArgs(msuM, msuM, msuM, msuM).Return(nil, fmt.Errorf("ua-error"))
	app.CrudHelpers = mCrudHelpers
	chgN, foundN, err := cht.timeoutNodes(ctx, clObj)
	assert.Error(err)
	assert.Zero(chgN)
	assert.Zero(foundN)
	assert.Regexp("ua-error", err)
	mockCtrl.Finish()

	// success
	tl.Logger().Info("case: Update success")
	tl.Flush()
	uaM := mock.NewUpdateArgsMatcher(t, retUA)
	nLP := node.NewNodeListParams()
	nLP.ClusterID = swag.String(string(clObj.Meta.ID))
	nLP.ServiceStateNE = swag.String(common.ServiceStateUnknown)
	nLP.StateNE = swag.String(common.NodeStateTearDown)
	timeOutTime := strfmt.DateTime(time.Time(clObj.Service.HeartbeatTime).Add(-time.Nanosecond))
	nLP.ServiceHeartbeatTimeLE = &timeOutTime
	mockCtrl = gomock.NewController(t)
	mCrudHelpers = mock.NewMockAppCrudHelpers(mockCtrl)
	mCrudHelpers.EXPECT().MakeStdUpdateArgs(msuM, msuM, msuM, msuM).Return(retUA, nil)
	app.CrudHelpers = mCrudHelpers
	mNOps := mock.NewMockNodeOps(mockCtrl)
	mNOps.EXPECT().UpdateMultiple(ctx, nLP, uaM, nodeMutable).Return(1, 2, nil)
	mDS := mock.NewMockDataStore(mockCtrl)
	mDS.EXPECT().OpsNode().Return(mNOps).Times(1)
	app.DS = mDS
	chgN, foundN, err = cht.timeoutNodes(ctx, clObj)
	assert.NoError(err)
	assert.Equal(1, chgN)
	assert.Equal(2, foundN)
}

func TestChtSendNodeCrude(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	fts := &fhk.TaskScheduler{}
	evM := fev.NewFakeEventManager()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log:      tl.Logger(),
			CrudeOps: evM,
		},
		TaskScheduler: fts,
	}
	hb := newHBComp()
	hb.app = app
	hb.Log = app.Log
	cht := chtRegisterAnimator(hb) // tested elsewhere

	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{ID: "CLUSTER-1", Version: 20},
		},
	}
	clID := string(clObj.Meta.ID)

	tl.Logger().Info("case: no node updated")
	evM.InIEev = nil
	err := cht.sendNodeCrudEvent(clObj, 0, 0)
	assert.NoError(err)
	assert.Nil(evM.InIEev)

	tl.Logger().Info("case: nodes updated, no timeout")
	evM.InIEev = nil
	err = cht.sendNodeCrudEvent(clObj, 1, 0)
	assert.NoError(err)
	assert.NotNil(evM.InIEev)
	expCE := &crude.CrudEvent{
		Method:     "PATCH",
		TrimmedURI: "/nodes/summary-heartbeat",
		Scope: map[string]string{
			"clusterId":           clID,
			"serviceStateChanged": "false",
		},
		AccessControlScope: clObj,
	}
	assert.Equal(evM.InIEev, expCE)

	tl.Logger().Info("case: nodes timed out")
	evM.InIEev = nil
	err = cht.sendNodeCrudEvent(clObj, 0, 1)
	assert.NoError(err)
	assert.NotNil(evM.InIEev)
	expCE.Scope["serviceStateChanged"] = "true"
	assert.Equal(evM.InIEev, expCE)
}

func TestChtGetTimedoutClusters(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	fts := &fhk.TaskScheduler{}
	evM := fev.NewFakeEventManager()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log:      tl.Logger(),
			CrudeOps: evM,
		},
		TaskScheduler: fts,
	}
	hb := newHBComp()
	hb.app = app
	hb.Log = app.Log
	cht := chtRegisterAnimator(hb) // tested elsewhere
	ctx := context.Background()

	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{ID: "CLUSTER-1", Version: 20},
		},
	}
	ss := &models.ServiceState{
		HeartbeatPeriodSecs: 10,
		State:               "READY",
	}
	timeoutTime := time.Now()

	// test clusterTimeoutAfter
	expTimeoutAfter := time.Time(ss.HeartbeatTime).Add(time.Duration(cht.clusterMaxMissed*ss.HeartbeatPeriodSecs) * time.Second)
	assert.Equal(expTimeoutAfter, cht.clusterTimeoutAfter(ss))

	// list failure
	tl.Logger().Info("case: List failure")
	tl.Flush()
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mClOps := mock.NewMockClusterOps(mockCtrl)
	lP := cluster.NewClusterListParams()
	lP.ServiceStateNE = swag.String(common.ServiceStateUnknown)
	mClOps.EXPECT().List(ctx, lP).Return(nil, centrald.ErrorDbError)
	mDS := mock.NewMockDataStore(mockCtrl)
	mDS.EXPECT().OpsCluster().Return(mClOps).Times(1)
	app.DS = mDS
	ret, err := cht.getTimedoutClusters(ctx, timeoutTime)
	assert.Error(err)
	assert.Nil(ret)
	assert.Regexp(centrald.ErrorDbError.M, err)
	mockCtrl.Finish()

	// empty list
	tl.Logger().Info("case: Empty list")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	mClOps = mock.NewMockClusterOps(mockCtrl)
	mClOps.EXPECT().List(ctx, lP).Return([]*models.Cluster{}, nil)
	mDS = mock.NewMockDataStore(mockCtrl)
	mDS.EXPECT().OpsCluster().Return(mClOps).Times(1)
	app.DS = mDS
	ret, err = cht.getTimedoutClusters(ctx, timeoutTime)
	assert.NoError(err)
	assert.NotNil(ret)
	assert.Empty(ret)
	mockCtrl.Finish()

	// cluster with no service
	tl.Logger().Info("case: cluster with no NuvoService")
	tl.Flush()
	assert.Nil(clObj.Service)
	mockCtrl = gomock.NewController(t)
	mClOps = mock.NewMockClusterOps(mockCtrl)
	mClOps.EXPECT().List(ctx, lP).Return([]*models.Cluster{clObj}, nil)
	mDS = mock.NewMockDataStore(mockCtrl)
	mDS.EXPECT().OpsCluster().Return(mClOps).Times(1)
	app.DS = mDS
	ret, err = cht.getTimedoutClusters(ctx, timeoutTime)
	assert.NoError(err)
	assert.Len(ret, 1)
	assert.Equal(clObj, ret[0])
	assert.NotNil(clObj.Service)
	mockCtrl.Finish()

	// cluster with empty service
	tl.Logger().Info("case: cluster with empty NuvoService")
	tl.Flush()
	clObj.Service = &models.NuvoService{}
	mockCtrl = gomock.NewController(t)
	mClOps = mock.NewMockClusterOps(mockCtrl)
	mClOps.EXPECT().List(ctx, lP).Return([]*models.Cluster{clObj}, nil)
	mDS = mock.NewMockDataStore(mockCtrl)
	mDS.EXPECT().OpsCluster().Return(mClOps).Times(1)
	app.DS = mDS
	ret, err = cht.getTimedoutClusters(ctx, timeoutTime)
	assert.NoError(err)
	assert.Len(ret, 1)
	assert.Equal(clObj, ret[0])
	assert.NotNil(clObj.Service)
	mockCtrl.Finish()

	// cluster that has timed out
	tl.Logger().Info("case: timed out clusters")
	tl.Flush()
	clObj.Service = &models.NuvoService{
		ServiceState: *ss,
	}
	lastHB := timeoutTime.Add(time.Duration(-cht.clusterMaxMissed*ss.HeartbeatPeriodSecs)*time.Second - time.Nanosecond)
	clObj.Service.HeartbeatTime = strfmt.DateTime(lastHB)
	assert.True(timeoutTime.After(cht.clusterTimeoutAfter(&clObj.Service.ServiceState)))
	mockCtrl = gomock.NewController(t)
	mClOps = mock.NewMockClusterOps(mockCtrl)
	mClOps.EXPECT().List(ctx, lP).Return([]*models.Cluster{clObj, clObj}, nil) // multiple
	mDS = mock.NewMockDataStore(mockCtrl)
	mDS.EXPECT().OpsCluster().Return(mClOps).Times(1)
	app.DS = mDS
	ret, err = cht.getTimedoutClusters(ctx, timeoutTime)
	assert.NoError(err)
	assert.Len(ret, 2)
	assert.Equal(clObj, ret[0])
	assert.Equal(clObj, ret[1])
	assert.NotNil(clObj.Service)
	mockCtrl.Finish()

	// cluster that has not timed out
	tl.Logger().Info("case: no timed out clusters")
	tl.Flush()
	clObj.Service = &models.NuvoService{
		ServiceState: *ss,
	}
	lastHB = timeoutTime.Add(time.Duration(-cht.clusterMaxMissed*ss.HeartbeatPeriodSecs) * time.Second)
	clObj.Service.HeartbeatTime = strfmt.DateTime(lastHB)
	assert.False(timeoutTime.After(cht.clusterTimeoutAfter(&clObj.Service.ServiceState)))
	mockCtrl = gomock.NewController(t)
	mClOps = mock.NewMockClusterOps(mockCtrl)
	mClOps.EXPECT().List(ctx, lP).Return([]*models.Cluster{clObj}, nil)
	mDS = mock.NewMockDataStore(mockCtrl)
	mDS.EXPECT().OpsCluster().Return(mClOps).Times(1)
	app.DS = mDS
	ret, err = cht.getTimedoutClusters(ctx, timeoutTime)
	assert.NoError(err)
	assert.NotNil(ret)
	assert.Empty(ret)
}

func TestMakeTimeoutTask(t *testing.T) {
	assert := assert.New(t)
	cht := &clusterHeartbeatTask{} // no initialization required for this test

	timeoutTime := time.Now()
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{ID: "CLUSTER-1"},
		},
		ClusterMutable: models.ClusterMutable{
			ClusterMutableAllOf0: models.ClusterMutableAllOf0{
				Service: &models.NuvoService{
					ServiceState: models.ServiceState{
						HeartbeatPeriodSecs: 10,
						State:               "READY",
						HeartbeatTime:       strfmt.DateTime(timeoutTime.Add(-30 * time.Minute)),
					},
				},
			},
		},
	}
	expTask := &models.Task{
		TaskCreateOnce: models.TaskCreateOnce{
			Operation: common.TaskClusterHeartbeat,
			ObjectID:  "CLUSTER-1",
			ServiceStates: map[string]models.ServiceState{
				"CLUSTER-1": models.ServiceState{
					HeartbeatPeriodSecs: 10,
					State:               common.ServiceStateUnknown,
					HeartbeatTime:       strfmt.DateTime(timeoutTime),
				},
			},
		},
	}
	msgs := util.NewMsgList(nil)
	msgs.Insert(chtTimeoutTaskMessage)
	expTask.Messages = msgs.ToModel()
	task := cht.makeTimeoutTask(clObj, timeoutTime)
	assert.NotNil(task)
	assert.Equal(len(expTask.Messages), len(task.Messages))
	expTask.Messages[0].Time = task.Messages[0].Time
	assert.Equal(expTask, task)
	assert.NoError(cht.TaskValidate(&task.TaskCreateOnce))
}

func TestReportNodesToAuditLog(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	ctx := context.Background()
	fts := &fhk.TaskScheduler{}
	evM := fev.NewFakeEventManager()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log:      tl.Logger(),
			CrudeOps: evM,
		},
		TaskScheduler: fts,
	}
	hb := newHBComp()
	hb.app = app
	hb.Log = app.Log
	fa := &fal.AuditLog{}
	app.AuditLog = fa

	cht := chtRegisterAnimator(hb) // tested elsewhere

	timeoutTime := time.Now()
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{ID: "CLUSTER-1"},
		},
		ClusterMutable: models.ClusterMutable{
			ClusterMutableAllOf0: models.ClusterMutableAllOf0{
				Service: &models.NuvoService{
					ServiceState: models.ServiceState{
						HeartbeatPeriodSecs: 10,
						State:               "READY",
						HeartbeatTime:       strfmt.DateTime(timeoutTime.Add(-30 * time.Minute)),
					},
				},
			},
		},
	}
	clObj.AccountID = "aid1"
	lRes := []*models.Node{
		&models.Node{
			NodeAllOf0: models.NodeAllOf0{
				Meta: &models.ObjMeta{
					ID:           "NODE-1",
					Version:      2,
					TimeModified: strfmt.DateTime(timeoutTime.Add(-5 * time.Minute)), // node was marked missing after last cluster heart beat
				},
				ClusterID: "CLUSTER-1",
			},
			NodeMutable: models.NodeMutable{
				Name: "NODE-1",
				Service: &models.NuvoService{
					ServiceState: models.ServiceState{
						State: "UNKNOWN",
					},
				},
			},
		},
		&models.Node{
			NodeAllOf0: models.NodeAllOf0{
				Meta: &models.ObjMeta{
					ID:           "NODE-2",
					Version:      2,
					TimeModified: strfmt.DateTime(timeoutTime.Add(-45 * time.Minute)), // node went missing long ago
				},
				ClusterID: "CLUSTER-1",
			},
			NodeMutable: models.NodeMutable{
				Name: "NODE-2",
				Service: &models.NuvoService{
					ServiceState: models.ServiceState{
						State: "UNKNOWN",
					},
				},
			},
		},
	}
	nLP := node.NewNodeListParams()
	nLP.NodeIds = []string{"NODE-1", "NODE-2"}
	nLP.ServiceStateNE = swag.String(com.ServiceStateReady)

	// failure to list cluster nodes for nodes restarted
	tl.Logger().Info("case: failure to list cluster nodes for nodes restarted")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	nOps := mock.NewMockNodeOps(mockCtrl)
	nOps.EXPECT().List(ctx, nLP).Return(nil, centrald.ErrorDbError)
	mDS := mock.NewMockDataStore(mockCtrl)
	mDS.EXPECT().OpsNode().Return(nOps).Times(1)
	app.DS = mDS
	cht.reportRestartedNodesToAuditLog(ctx, clObj, []string{"NODE-1", "NODE-2"})
	assert.Empty(fa.Posts)
	assert.Empty(fa.Events)
	*fa = fal.AuditLog{}
	tl.Flush()

	// success for nodes restarted
	tl.Logger().Info("case: success for nodes restarted")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nOps = mock.NewMockNodeOps(mockCtrl)
	nOps.EXPECT().List(ctx, nLP).Return(lRes, nil)
	mDS = mock.NewMockDataStore(mockCtrl)
	mDS.EXPECT().OpsNode().Return(nOps).Times(1)
	app.DS = mDS
	cht.reportRestartedNodesToAuditLog(ctx, clObj, []string{"NODE-1", "NODE-2"})
	ai := &auth.Info{AccountID: "aid1"}
	assert.Len(fa.Events, 2)
	for _, event := range fa.Events {
		assert.Equal(ai, event.AI)
		assert.Equal(centrald.NodeUpdateAction, event.Action)
		assert.False(event.Err)
		assert.True(event.ObjID == "NODE-1" || event.ObjID == "NODE-2")
		assert.True(event.Name == "NODE-1" || event.Name == "NODE-2")
		assert.Equal("Started heartbeat", event.Message)
	}
	*fa = fal.AuditLog{}
	tl.Flush()

	// failure to list cluster nodes for timedout nodes
	tl.Logger().Info("case: failure to list cluster nodes for timedout nodes")
	nLP = node.NewNodeListParams()
	nLP.ClusterID = swag.String(string(clObj.Meta.ID))
	nLP.ServiceStateEQ = swag.String(common.ServiceStateUnknown)
	timeOutTime := strfmt.DateTime(time.Time(clObj.Service.HeartbeatTime).Add(-time.Nanosecond))
	nLP.ServiceHeartbeatTimeLE = &timeOutTime
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nOps = mock.NewMockNodeOps(mockCtrl)
	nOps.EXPECT().List(ctx, nLP).Return(nil, centrald.ErrorDbError)
	mDS = mock.NewMockDataStore(mockCtrl)
	mDS.EXPECT().OpsNode().Return(nOps).Times(1)
	app.DS = mDS
	cht.reportTimedoutNodesToAuditLog(ctx, clObj)
	assert.Empty(fa.Posts)
	assert.Empty(fa.Events)
	*fa = fal.AuditLog{}
	tl.Flush()

	// success for timedout nodes
	tl.Logger().Info("case: success for timedout nodes")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nOps = mock.NewMockNodeOps(mockCtrl)
	nOps.EXPECT().List(ctx, nLP).Return(lRes, nil)
	mDS = mock.NewMockDataStore(mockCtrl)
	mDS.EXPECT().OpsNode().Return(nOps).Times(1)
	app.DS = mDS
	cht.reportTimedoutNodesToAuditLog(ctx, clObj)
	exp := &fal.Args{AI: ai, Action: centrald.NodeUpdateAction, ObjID: "NODE-1", Name: "NODE-1", Message: "Stopped heartbeat"}
	assert.Equal([]*fal.Args{exp}, fa.Events)
}

// ClusterMutableMatcher is a gomock Matcher
type ClusterMutableMatcher struct {
	t  *testing.T
	cm *models.ClusterMutable
}

func NewClusterMutableMatcher(t *testing.T, cm *models.ClusterMutable) *ClusterMutableMatcher {
	return &ClusterMutableMatcher{t: t, cm: cm}
}

// Matches is from gomock.Matcher
func (o *ClusterMutableMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	cm, ok := x.(*models.ClusterMutable)
	if !ok {
		assert.True(ok, "argument ISA *models.ClusterMutable")
		return false
	}
	if len(cm.Messages) == len(o.cm.Messages) { // skip timestamp matching
		for i, m := range o.cm.Messages {
			m.Time = cm.Messages[i].Time
		}
	}
	return assert.Equal(o.cm, cm)
}

// String is from gomock.Matcher
func (o *ClusterMutableMatcher) String() string {
	return "ClusterMutable matches"
}
