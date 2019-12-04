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


package vreq

import (
	"bytes"
	"fmt"
	"regexp"
	"testing"
	"time"

	mcvr "github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	fcl "github.com/Nuvoloso/kontroller/pkg/clusterd/fake"
	"github.com/Nuvoloso/kontroller/pkg/clusterd/state"
	fakeState "github.com/Nuvoloso/kontroller/pkg/clusterd/state/fake"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	mockcsp "github.com/Nuvoloso/kontroller/pkg/csp/mock"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func newComponent() *Component {
	c := &Component{
		Args: Args{
			RetryInterval: time.Duration(10 * time.Second),
			StopPeriod:    time.Duration(10 * time.Second),
		},
	}
	return c
}

// The following states are processed by clusterd (other than NEW)
var testNotNewStates = []string{
	"CHOOSING_NODE",
	"CG_SNAPSHOT_FINALIZE",
	"CG_SNAPSHOT_VOLUMES",
	"CG_SNAPSHOT_WAIT",
	"CREATING_FROM_SNAPSHOT",
	"PLACEMENT",
	"PLACEMENT_REATTACH",
	"PUBLISHING",
	"PUBLISHING_SERVICE_PLAN",
	"RESIZING_CACHE",
	"SIZING",
	"SNAPSHOT_RESTORE_DONE",
	"STORAGE_WAIT",
	"UNDO_CG_SNAPSHOT_FINALIZE",
	"UNDO_CG_SNAPSHOT_VOLUMES",
	"UNDO_CREATING_FROM_SNAPSHOT",
	"UNDO_PLACEMENT",
	"UNDO_PUBLISHING",
	"UNDO_PUBLISHING_SERVICE_PLAN",
	"UNDO_RESIZING_CACHE",
	"UNDO_SIZING",
	"UNDO_SNAPSHOT_RESTORE_DONE",
}

// The NEW state is processed by clusterd with the following initial operations
var testNewFirstOp = []string{
	"MOUNT",
	"CG_SNAPSHOT_CREATE",
	"CONFIGURE",
	"CREATE_FROM_SNAPSHOT",
	"PUBLISH",
	"UNPUBLISH",
}

func TestComponentInit(t *testing.T) {
	assert := assert.New(t)
	args := Args{
		RetryInterval: time.Second * 20,
	}
	// This actually calls clusterd.AppRegisterComponent but we can't intercept that
	c := ComponentInit(&args)
	assert.Equal(args.RetryInterval, c.RetryInterval)
}

func TestComponentMethods(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log:             tl.Logger(),
			HeartbeatPeriod: 10,
			CSPDomainID:     "6a156de2-ba94-4035-b47f-e87373d75757",
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	ri := InitialRetryInterval

	// init fails to create a watcher
	c := newComponent()
	evM.RetWErr = fmt.Errorf("watch-error")
	assert.NotPanics(func() { c.Init(app) })
	assert.Empty(c.watcherID)
	assert.EqualValues(ri, c.Animator.RetryInterval)
	assert.Equal(app, c.App)
	assert.Equal(app.Log, c.Log)
	assert.False(c.fatalError)

	// init creates a watcher
	evM.RetWErr = nil
	evM.RetWID = "watcher-id"
	c = newComponent()
	assert.NotPanics(func() { c.Init(app) })
	assert.Equal(ri, c.Animator.RetryInterval)
	assert.Equal(app, c.App)
	assert.Equal(app.Log, c.Log)
	assert.Equal(10*time.Second, c.Animator.StopPeriod)
	assert.False(c.fatalError)
	assert.Equal("watcher-id", c.watcherID)

	// change the interval for the UT
	utInterval := time.Millisecond * 10
	c.Animator.RetryInterval = utInterval
	c.Animator.StopPeriod = 100 * time.Millisecond
	assert.Equal(0, c.runCount)
	assert.Nil(c.oCrud)
	assert.Nil(c.App.AppObjects)
	c.Start() // invokes run/RunBody asynchronously but won't do anything if c.App.AppObjects
	assert.True(c.runCount == 0)
	for c.runCount == 0 {
		time.Sleep(utInterval)
	}
	assert.NotNil(c.oCrud)

	// stop
	c.Stop()
	foundStarting := 0
	foundStopped := 0
	tl.Iterate(func(i uint64, s string) {
		if res, err := regexp.MatchString("Starting VolumeSeriesHandler", s); err == nil && res {
			foundStarting++
		}
		if res, err := regexp.MatchString("Stopped VolumeSeriesHandler", s); err == nil && res {
			foundStopped++
		}
	})
	assert.Equal(1, foundStarting)
	assert.Equal(1, foundStopped)

	// validate watcher patterns
	wa := c.getWatcherArgs()
	assert.NotNil(wa)
	assert.Len(wa.Matchers, 6)

	m := wa.Matchers[0]
	assert.NotEmpty(m.MethodPattern)
	tl.Logger().Debugf("0.MethodPattern: %s", m.MethodPattern)
	re := regexp.MustCompile(m.MethodPattern)
	assert.True(re.MatchString("POST"))
	assert.NotEmpty(m.URIPattern)
	tl.Logger().Debugf("0.URIPattern: %s", m.URIPattern)
	re = regexp.MustCompile(m.URIPattern)
	assert.True(re.MatchString("/volume-series-requests"))
	assert.False(re.MatchString("/volume-series-requests/"))
	assert.NotEmpty(m.ScopePattern)
	re = regexp.MustCompile(m.ScopePattern)
	for _, op := range vra.SupportedVolumeSeriesRequestOperations() {
		scope := fmt.Sprintf("foo:bar requestedOperations:%s,foo abc:123", op)
		if util.Contains(testNewFirstOp, op) {
			assert.True(re.MatchString(scope), "Op %s", op)
		} else {
			assert.False(re.MatchString(scope), "Op %s", op)
		}
	}

	m = wa.Matchers[1]
	assert.NotEmpty(m.MethodPattern)
	tl.Logger().Debugf("1.MethodPattern: %s", m.MethodPattern)
	re = regexp.MustCompile(m.MethodPattern)
	assert.True(re.MatchString("PATCH"))
	assert.NotEmpty(m.URIPattern)
	tl.Logger().Debugf("1.URIPattern: %s", m.URIPattern)
	re = regexp.MustCompile(m.URIPattern)
	assert.False(re.MatchString("/volume-series-requests"))
	assert.True(re.MatchString("/volume-series-requests/"))
	assert.NotEmpty(m.ScopePattern)
	tl.Logger().Debugf("1.ScopePattern: %s", m.ScopePattern)
	re = regexp.MustCompile(m.ScopePattern)
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:CHOOSING_NODE abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:SIZING abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:PLACEMENT abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:PLACEMENT_REATTACH abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:RESIZING_CACHE abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:UNDO_SIZING abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:UNDO_PLACEMENT abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:UNDO_RESIZING_CACHE abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:CG_SNAPSHOT_FINALIZE abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:SNAPSHOT_RESTORE_DONE abc:123"))
	assert.False(re.MatchString("foo:bar volumeSeriesRequestState:BINDING abc:123"))
	assert.False(re.MatchString("foo:bar volumeSeriesRequestState:CHANGING_CAPACITY abc:123"))
	assert.False(re.MatchString("foo:bar volumeSeriesRequestState:REALLOCATING_CACHE abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:PUBLISHING abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:PUBLISHING_SERVICE_PLAN abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:UNDO_PUBLISHING abc:123"))

	m = wa.Matchers[2]
	assert.NotEmpty(m.MethodPattern)
	tl.Logger().Debugf("2.MethodPattern: %s", m.MethodPattern)
	re = regexp.MustCompile(m.MethodPattern)
	assert.True(re.MatchString("PATCH"))
	assert.NotEmpty(m.URIPattern)
	tl.Logger().Debugf("2.URIPattern: %s", m.URIPattern)
	re = regexp.MustCompile(m.URIPattern)
	assert.False(re.MatchString("/storage-requests"))
	assert.True(re.MatchString("/storage-requests/"))
	assert.NotEmpty(m.ScopePattern)
	tl.Logger().Debugf("2.ScopePattern: %s", m.ScopePattern)
	re = regexp.MustCompile(m.ScopePattern)
	assert.True(re.MatchString("foo:bar storageRequestState:SUCCEEDED abc:123"))
	assert.True(re.MatchString("foo:bar storageRequestState:FAILED abc:123"))
	assert.False(re.MatchString("foo:bar storageRequestState:NEW abc:123"))

	m = wa.Matchers[3]
	assert.Empty(m.MethodPattern)
	assert.NotEmpty(m.URIPattern)
	tl.Logger().Debugf("3.URIPattern: %s", m.URIPattern)
	re = regexp.MustCompile(m.URIPattern)
	assert.True(re.MatchString("/nodes"))
	assert.True(re.MatchString("/nodes/"))
	assert.NotEmpty(m.ScopePattern)
	tl.Logger().Debugf("3.ScopePattern: %s", m.ScopePattern)
	re = regexp.MustCompile(m.ScopePattern)
	assert.True(re.MatchString("foo:bar serviceStateChanged:true abc:123"))
	assert.False(re.MatchString("foo:bar serviceStateChanged:false abc:123"))
	assert.False(re.MatchString("foo:bar abc:123"))

	m = wa.Matchers[4]
	assert.NotEmpty(m.MethodPattern)
	tl.Logger().Debugf("1.MethodPattern: %s", m.MethodPattern)
	re = regexp.MustCompile(m.MethodPattern)
	assert.True(re.MatchString("PATCH"))
	assert.NotEmpty(m.URIPattern)
	tl.Logger().Debugf("1.URIPattern: %s", m.URIPattern)
	re = regexp.MustCompile(m.URIPattern)
	assert.False(re.MatchString("/volume-series-requests"))
	assert.True(re.MatchString("/volume-series-requests/"))
	assert.NotEmpty(m.ScopePattern)
	tl.Logger().Debugf("1.ScopePattern: %s", m.ScopePattern)
	re = regexp.MustCompile(m.ScopePattern)
	assert.True(re.MatchString("consistencyGroupId:cg volumeSeriesId:vol volumeSeriesRequestState:FAILED"))
	assert.True(re.MatchString("consistencyGroupId:cg volumeSeriesId:vol volumeSeriesRequestState:CANCELED"))
	assert.False(re.MatchString("consistencyGroupId:cg volumeSeriesId: volumeSeriesRequestState:FAILED"))
	assert.False(re.MatchString("consistencyGroupId:cg volumeSeriesId: volumeSeriesRequestState:CANCELED"))
	assert.False(re.MatchString("consistencyGroupId:cg volumeSeriesId:vol volumeSeriesRequestState:SUCCEEDED"))

	m = wa.Matchers[5]
	assert.NotEmpty(m.MethodPattern)
	tl.Logger().Debugf("1.MethodPattern: %s", m.MethodPattern)
	re = regexp.MustCompile(m.MethodPattern)
	assert.True(re.MatchString("PATCH"))
	assert.NotEmpty(m.URIPattern)
	tl.Logger().Debugf("1.URIPattern: %s", m.URIPattern)
	re = regexp.MustCompile(m.URIPattern)
	assert.False(re.MatchString("/volume-series-requests"))
	assert.True(re.MatchString("/volume-series-requests/"))
	assert.NotEmpty(m.ScopePattern)
	tl.Logger().Debugf("1.ScopePattern: %s", m.ScopePattern)
	re = regexp.MustCompile(m.ScopePattern)
	assert.True(re.MatchString("parentId:xxx volumeSeriesRequestState:FAILED"))
	assert.True(re.MatchString("parentId:xxx volumeSeriesRequestState:CANCELED"))
	assert.True(re.MatchString("parentId:xxx volumeSeriesRequestState:SUCCEEDED"))
	assert.False(re.MatchString("volumeSeriesRequestState:FAILED"))
	assert.False(re.MatchString("volumeSeriesRequestState:CANCELED"))
	assert.False(re.MatchString("volumeSeriesRequestState:SUCCEEDED"))
}

func TestRunBody(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	fao := &fcl.AppObjects{}
	fsu := &fcl.StateUpdater{}
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	svcArgs := util.ServiceArgs{
		ServiceType:         "clusterd",
		ServiceVersion:      "test",
		HeartbeatPeriodSecs: int64(app.HeartbeatPeriod),
		Log:                 app.Log,
		MaxMessages:         10,
		ServiceAttributes:   map[string]models.ValueType{},
	}
	app.Service = util.NewService(&svcArgs)
	app.StateGuard = util.NewCriticalSectionGuard()
	assert.NotNil(app.StateGuard)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	app.CSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)

	c := Component{}
	c.RetryInterval = 30 * time.Second
	c.Init(app)
	assert.Equal(InitialRetryInterval, c.Animator.RetryInterval)
	fc := &fake.Client{}
	c.oCrud = fc

	// Run body, startup logic cases
	assert.False(c.startupCleanupDone)
	assert.Nil(app.StateOps)
	c.App.Service.SetState(util.ServiceStarting)
	ret := c.RunBody(nil) // nil AppObjects
	assert.Nil(ret)
	assert.Nil(app.StateOps)
	assert.False(c.startupCleanupDone)
	assert.Equal(util.ServiceStarting, c.App.Service.GetState())
	assert.Equal(0, app.StateGuard.Used)

	app.AppObjects = fao
	app.StateUpdater = fsu
	ret = c.RunBody(nil) // nil cluster object
	assert.Nil(ret)
	assert.False(c.startupCleanupDone)
	assert.Equal(0, app.StateGuard.Used)

	fao.RetGCobj = &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "cl1",
			},
		},
		ClusterMutable: models.ClusterMutable{
			ClusterCreateMutable: models.ClusterCreateMutable{
				ClusterIdentifier: "cluster-identifier",
			},
		},
	}
	ret = c.RunBody(nil)
	assert.Nil(ret)
	assert.False(c.startupCleanupDone)
	assert.EqualValues("cl1", c.thisClusterID)
	assert.Equal("cluster-identifier", c.thisClusterIdentifier)
	assert.Equal(InitialRetryInterval, c.Animator.RetryInterval)
	c.fatalError = false

	fso := fakeState.NewFakeClusterState()
	app.StateOps = fso
	fso.RetRErr = fmt.Errorf("cluster-state-reload-error")
	fso.RetRChg = false
	ret = c.RunBody(nil)
	assert.Nil(ret)
	assert.Equal(1, fso.RCnt)
	assert.Equal(1, app.StateGuard.Used)
	assert.Equal(0, fso.RUSCnt)
	assert.False(c.App.IsReady())
	assert.Equal(InitialRetryInterval, c.Animator.RetryInterval)

	fso.RetRErr = nil
	fso.RetRChg = true
	fso.RetDS = bytes.NewBufferString("cluster-state")
	fso.RUSCnt = 0
	c.App.ReleaseStorageRequestTimeout = 5 * time.Minute
	assert.Nil(app.Service.GetServiceAttribute(com.ServiceAttrClusterResourceState))
	fc = &fake.Client{}
	c.oCrud = fc
	fc.RetLsVRObj = nil
	fc.RetLsVRErr = fmt.Errorf("VolumeSeriesRequestList fails")
	c.App.Service.SetState(util.ServiceStarting)
	ret = c.RunBody(nil)
	assert.Nil(ret)
	assert.True(c.startupCleanupDone)
	assert.Equal(util.ServiceReady, c.App.Service.GetState())
	assert.Equal(c.RetryInterval*WatcherRetryMultiplier, c.Animator.RetryInterval)
	assert.Equal(2, fso.RCnt)
	assert.Equal(1, fso.DSCnt)
	assert.Equal(2, app.StateGuard.Used)
	vtS := app.Service.GetServiceAttribute(com.ServiceAttrClusterResourceState)
	assert.NotNil(vtS)
	assert.Equal(com.ValueTypeString, vtS.Kind)
	assert.Equal(fso.RetDS.String(), vtS.Value)
	assert.Equal(1, fso.RUSCnt)
	assert.Equal(5*time.Minute, fso.InRUSTimeout)

	// unused storage release subject to REI
	c.rei.SetProperty("disable-release-unused-storage", &rei.Property{BoolValue: true})
	fso.RUSCnt = 0
	c.releaseUnusedStorage(nil)
	assert.Equal(0, fso.RUSCnt)

	fc.InLsVRObj = nil
	fc.RetLsVRObj = &mcvr.VolumeSeriesRequestListOK{
		Payload: make([]*models.VolumeSeriesRequest, 0),
	}
	fc.RetLsVRErr = nil
	expLsVR := mcvr.NewVolumeSeriesRequestListParams()
	expLsVR.ClusterID = swag.String("cl1")
	expLsVR.VolumeSeriesRequestState = clusterdStates
	expLsVR.IsTerminated = swag.Bool(false)
	assert.NotEmpty(clusterdStates)
	assert.True(len(clusterdStates) > len(mountDeletePublishStates))
	ret = c.RunBody(nil)
	assert.NotNil(ret)
	assert.True(c.startupCleanupDone)
	assert.Equal(expLsVR, fc.InLsVRObj)
	assert.Equal(3, app.StateGuard.Used)

	// force a locking error
	app.StateGuard.Drain()
	ret = c.RunBody(nil)
	assert.Nil(ret)

	foundCleanupStart := 0
	foundCleanupDone := 0
	foundReleasingLock := 0
	foundFailedLock := 0
	tl.Iterate(func(i uint64, s string) {
		if res, err := regexp.MatchString("cleanup starting", s); err == nil && res {
			foundCleanupStart++
		}
		if res, err := regexp.MatchString("cleanup done", s); err == nil && res {
			foundCleanupDone++
		}
		if res, err := regexp.MatchString("Releasing state lock", s); err == nil && res {
			foundReleasingLock++
		}
		if res, err := regexp.MatchString("Failed to get state lock", s); err == nil && res {
			foundFailedLock++
		}
	})
	assert.Equal(1, foundCleanupStart)
	assert.Equal(1, foundCleanupDone)
	assert.Equal(3, foundReleasingLock)
	assert.Equal(1, foundFailedLock)

	// ************** removeClaimsForTerminatedVSRs
	srN1T1A := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{ID: "SR-N1-T1-A"},
		},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			PoolID: "SP-GP2",
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestCreateMutable: models.StorageRequestCreateMutable{
				NodeID: "NODE-1",
				VolumeSeriesRequestClaims: &models.VsrClaim{
					Claims: map[string]models.VsrClaimElement{
						"VSR-1": {SizeBytes: swag.Int64(1000), Annotation: "claim1_1"},
					},
					RemainingBytes: swag.Int64(1000),
				},
			},
		},
	}
	claim1_1 := &state.VSRClaim{
		RequestID:  "VSR-1",
		SizeBytes:  1000,
		Annotation: "claim1_1",
	}
	srN2T1A := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{ID: "SR-N2-T1-A"},
		},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			PoolID: "SP-GP2",
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestCreateMutable: models.StorageRequestCreateMutable{
				NodeID: "NODE-1",
				VolumeSeriesRequestClaims: &models.VsrClaim{
					Claims: map[string]models.VsrClaimElement{
						"VSR-2": {SizeBytes: swag.Int64(1000), Annotation: "claim2_1"},
					},
					RemainingBytes: swag.Int64(1000),
				},
			},
		},
	}
	claim2_1 := &state.VSRClaim{
		RequestID:  "VSR-2",
		SizeBytes:  1000,
		Annotation: "claim2_1",
	}

	vsrList := []*models.VolumeSeriesRequest{
		&models.VolumeSeriesRequest{
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: models.ObjID("VSR-1"),
				},
			},
		},
	}
	fc = &fake.Client{}
	fso.ClusterState.OCrud = fc
	fc.RetSRUpdatedObj = srN1T1A
	fso.ClusterState.Log = tl.Logger()
	fso.ClusterState.TrackStorageRequest(srN1T1A)
	fso.ClusterState.TrackStorageRequest(srN2T1A)
	_, err := fso.ClusterState.AddClaim(nil, srN1T1A, claim1_1)
	assert.Nil(err)
	_, err = fso.ClusterState.AddClaim(nil, srN2T1A, claim2_1)
	assert.Nil(err)
	fso.CallRealFindClaims = true
	app.StateOps = fso
	c.Init(app)
	c.removeClaimsForTerminatedVSRs(nil, vsrList)
	assert.Equal("VSR-2", fso.InRCReq)
}

func TestShouldDispatch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM

	c := Component{}
	c.Init(app)
	rhs := &vra.RequestHandlerState{}
	rhs.Request = &models.VolumeSeriesRequest{}
	rhs.Request.Meta = &models.ObjMeta{ID: "id"}

	for _, state := range vra.SupportedVolumeSeriesRequestStates() {
		rhs.Request.VolumeSeriesRequestState = state
		if state == "NEW" {
			for _, op := range vra.SupportedVolumeSeriesRequestOperations() {
				rhs.Request.RequestedOperations = []string{op}
				if util.Contains(testNewFirstOp, op) {
					assert.True(c.ShouldDispatch(nil, rhs), "State %s Op:%s", state, op)
				} else {
					assert.False(c.ShouldDispatch(nil, rhs), "State %s Op:%s", state, op)
				}
			}
		} else {
			rhs.Request.RequestedOperations = []string{"not-NEW-op"}
			if util.Contains(testNotNewStates, state) {
				assert.True(c.ShouldDispatch(nil, rhs), "State %s", state)
			} else {
				assert.False(c.ShouldDispatch(nil, rhs), "State %s", state)
			}
		}
		tl.Flush()
	}
}

func vsClone(vs *models.VolumeSeries) *models.VolumeSeries {
	n := new(models.VolumeSeries)
	testutils.Clone(vs, n)
	return n
}

func vsrClone(vsr *models.VolumeSeriesRequest) *models.VolumeSeriesRequest {
	n := new(models.VolumeSeriesRequest)
	testutils.Clone(vsr, n)
	return n
}

func sClone(s *models.Storage) *models.Storage {
	n := new(models.Storage)
	testutils.Clone(s, n)
	return n
}

func srClone(sr *models.StorageRequest) *models.StorageRequest {
	n := new(models.StorageRequest)
	testutils.Clone(sr, n)
	return n
}
