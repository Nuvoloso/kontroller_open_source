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
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	fa "github.com/Nuvoloso/kontroller/pkg/agentd/fake"
	fakeState "github.com/Nuvoloso/kontroller/pkg/agentd/state/fake"
	mcvr "github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

// fake state updater
type fakeStateUpdater struct {
	rCalled  int
	retUS    error
	usCalled int
}

func (o *fakeStateUpdater) UpdateState(ctx context.Context) error {
	o.usCalled++
	return o.retUS
}

func (o *fakeStateUpdater) Refresh() {
	o.rCalled++
}

type vscFakeMounter struct {
	eVSR         *models.VolumeSeriesRequest
	eVS          *models.VolumeSeries
	retEInError  bool
	ueVSR        *models.VolumeSeriesRequest
	ueVS         *models.VolumeSeries
	retUEInError bool
	ucVSR        *models.VolumeSeriesRequest
	ucVS         *models.VolumeSeries
	retUCInError bool
	stash        map[interface{}]interface{}
}

func (fm *vscFakeMounter) Configure(ctx context.Context, rhs *vra.RequestHandlerState) {
}

func (fm *vscFakeMounter) UndoConfigure(ctx context.Context, rhs *vra.RequestHandlerState) {
	fm.ucVSR = vsrClone(rhs.Request)
	fm.ucVS = vsClone(rhs.VolumeSeries)
	rhs.InError = fm.retUCInError
}

func (fm *vscFakeMounter) Export(ctx context.Context, rhs *vra.RequestHandlerState) {
	fm.eVSR = vsrClone(rhs.Request)
	fm.eVS = vsClone(rhs.VolumeSeries)
	rhs.InError = fm.retEInError
	fm.stash = rhs.Stash
}

func (fm *vscFakeMounter) UndoExport(ctx context.Context, rhs *vra.RequestHandlerState) {
	fm.ueVSR = vsrClone(rhs.Request)
	fm.ueVS = vsClone(rhs.VolumeSeries)
	rhs.InError = fm.retUEInError
	fm.stash = rhs.Stash
}

func (fm *vscFakeMounter) ReallocateCache(ctx context.Context, rhs *vra.RequestHandlerState) {
	// TBD, for now no-op
}

func (fm *vscFakeMounter) UndoReallocateCache(ctx context.Context, rhs *vra.RequestHandlerState) {
	// TBD, for now no-op
}

func newComponent() *Component {
	c := &Component{
		Args: Args{
			RetryInterval: time.Duration(10 * time.Second),
			StopPeriod:    time.Duration(10 * time.Second),
		},
	}
	return c
}

// The following states are processed by agentd (other than NEW)
var testNotNewStates = []string{
	"ATTACHING_FS",
	"CREATED_PIT",
	"CREATING_PIT",
	"FINALIZING_SNAPSHOT",
	"PAUSED_IO",
	"PAUSING_IO",
	"REALLOCATING_CACHE",
	"SNAPSHOT_RESTORE",
	"SNAPSHOT_RESTORE_FINALIZE",
	"SNAPSHOT_UPLOADING",
	"SNAPSHOT_UPLOAD_DONE",
	"UNDO_ATTACHING_FS",
	"UNDO_CREATED_PIT",
	"UNDO_CREATING_PIT",
	"UNDO_PAUSED_IO",
	"UNDO_PAUSING_IO",
	"UNDO_REALLOCATING_CACHE",
	"UNDO_SNAPSHOT_RESTORE",
	"UNDO_SNAPSHOT_UPLOADING",
	"UNDO_SNAPSHOT_UPLOAD_DONE",
	"UNDO_VOLUME_CONFIG",
	"UNDO_VOLUME_EXPORT",
	"VOLUME_CONFIG",
	"VOLUME_EXPORT",
}

// The NEW state is processed by agentd with the following initial operations
var testNewFirstOp = []string{
	"ATTACH_FS",
	"DETACH_FS",
	"UNMOUNT",
	"VOL_SNAPSHOT_CREATE",
	"VOL_SNAPSHOT_RESTORE",
}

func TestComponentInit(t *testing.T) {
	assert := assert.New(t)
	args := Args{
		RetryInterval: time.Second * 20,
	}
	// This actually calls agentd.AppRegisterComponent but we can't intercept that
	c := ComponentInit(&args)
	assert.Equal(args.RetryInterval, c.RetryInterval)
}

func TestComponentMethods(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log:             tl.Logger(),
			HeartbeatPeriod: 10,
			CSPDomainID:     "6a156de2-ba94-4035-b47f-e87373d75757",
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	app.OCrud = &fake.Client{}
	fAS := &fa.AppServant{}
	app.AppServant = fAS
	ri := time.Duration(app.HeartbeatPeriod) * time.Second

	// init fails to create a watcher
	c := newComponent()
	evM.RetWErr = fmt.Errorf("watch-error")
	assert.NotPanics(func() { c.Init(app) })
	assert.Empty(c.watcherID)
	assert.EqualValues(ri, c.Animator.RetryInterval)
	assert.Equal(app, c.App)
	assert.Equal(app.Log, c.Log)

	// init creates a watcher
	evM.RetWErr = nil
	evM.RetWID = "watcher-id"
	c = newComponent()
	assert.NotPanics(func() { c.Init(app) })
	assert.Equal(time.Duration(WatcherRetryMultiplier*app.HeartbeatPeriod)*time.Second, c.Animator.RetryInterval)
	assert.Equal(app, c.App)
	assert.Equal(app.Log, c.Log)
	assert.Equal(10*time.Second, c.Animator.StopPeriod)
	assert.Equal(c, app.AppRecoverVolume)
	assert.NotNil(c.mounter)

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
	assert.Len(wa.Matchers, 2)

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
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:VOLUME_CONFIG abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:VOLUME_EXPORT abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:REALLOCATING_CACHE abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:SNAPSHOT_RESTORE"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:SNAPSHOT_RESTORE abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:SNAPSHOT_RESTORE_FINALIZE"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:SNAPSHOT_RESTORE_FINALIZE abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:UNDO_VOLUME_EXPORT abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:UNDO_VOLUME_CONFIG abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:UNDO_REALLOCATING_CACHE abc:123"))
	assert.False(re.MatchString("foo:bar volumeSeriesRequestState:BINDING abc:123"))
	assert.False(re.MatchString("foo:bar volumeSeriesRequestState:CHANGING_CAPACITY abc:123"))
	assert.False(re.MatchString("foo:bar volumeSeriesRequestState:RESIZING_CACHE abc:123"))
	assert.False(re.MatchString("foo:bar volumeSeriesRequestState:SNAPSHOT_RESTORE_DONE"))
	assert.False(re.MatchString("foo:bar volumeSeriesRequestState:SNAPSHOT_RESTORE_DONE abc:123"))
}

func TestRunBody(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	fAS := &fa.AppServant{}
	app.AppServant = fAS
	app.LMDGuard = util.NewCriticalSectionGuard()
	assert.NotNil(app.LMDGuard)

	svcArgs := util.ServiceArgs{
		ServiceType:       "agentd",
		ServiceVersion:    "test",
		Log:               app.Log,
		MaxMessages:       10,
		ServiceAttributes: map[string]models.ValueType{},
	}
	app.Service = util.NewService(&svcArgs)

	c := Component{}
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc

	// Run body, startup logic cases
	ret := c.RunBody(nil) // nil AppObjects
	assert.Nil(ret)
	assert.Equal(0, app.LMDGuard.Used)

	fao := &fa.AppObjects{}
	app.AppObjects = fao
	fsu := &fakeStateUpdater{}
	app.StateUpdater = fsu
	ret = c.RunBody(nil) // nil node object
	assert.Nil(ret)
	assert.Equal(0, app.LMDGuard.Used)

	fao.Node = &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "nid1",
			},
			NodeIdentifier: "node-identifier",
		},
		NodeMutable: models.NodeMutable{
			Service: &models.NuvoService{
				ServiceState: models.ServiceState{
					State: "RUNNING",
				},
			},
			Name:        "node1",
			Description: "node1 object",
			LocalStorage: map[string]models.NodeStorageDevice{
				"uuid1": models.NodeStorageDevice{
					DeviceName:      "d1",
					DeviceState:     "UNUSED",
					DeviceType:      "HDD",
					SizeBytes:       swag.Int64(1100),
					UsableSizeBytes: swag.Int64(1000),
				},
				"uuid2": models.NodeStorageDevice{
					DeviceName:      "d2",
					DeviceState:     "CACHE",
					DeviceType:      "SSD",
					SizeBytes:       swag.Int64(2200),
					UsableSizeBytes: swag.Int64(2000),
				},
			},
			AvailableCacheBytes: swag.Int64(3000),
			TotalCacheBytes:     swag.Int64(3300),
		},
	}
	// agent is not ready
	ret = c.RunBody(nil)
	assert.Nil(ret)
	assert.Equal(0, app.LMDGuard.Used)
	assert.EqualValues("nid1", c.thisNodeID)
	assert.Equal("node-identifier", c.thisNodeIdentifier)

	// node state reload error
	fAS.RetIsReady = true
	fso := fakeState.NewFakeNodeState()
	fso.RetRErr = fmt.Errorf("node-state-reload-error")
	fso.RetRChg = false
	app.StateOps = fso
	ret = c.RunBody(nil)
	assert.Nil(ret)
	assert.Equal(1, fso.RCnt)
	assert.Equal(1, app.LMDGuard.Used)
	assert.Equal(0, fsu.usCalled)
	assert.Equal(0, fso.DSCnt)

	// failure to list volume series
	fso.RetRErr = nil
	fso.RetRChg = true
	fc = &fake.Client{}
	fc.RetLsVRObj = nil
	fc.RetLsVRErr = fmt.Errorf("VolumeSeriesRequestList fails")
	c.oCrud = fc
	ret = c.RunBody(nil)
	assert.Nil(ret)
	assert.Equal(2, app.LMDGuard.Used)
	assert.Equal(1, fsu.usCalled)
	assert.Equal(1, fso.DSCnt)

	// success
	fc.InLsVRObj = nil
	fc.RetLsVRObj = &mcvr.VolumeSeriesRequestListOK{
		Payload: make([]*models.VolumeSeriesRequest, 0),
	}
	fc.RetLsVRErr = nil
	expLsVR := mcvr.NewVolumeSeriesRequestListParams()
	expLsVR.NodeID = swag.String("nid1")
	expLsVR.IsTerminated = swag.Bool(false) // list active requests only
	ret = c.RunBody(nil)
	assert.NotNil(ret)
	assert.Equal(3, app.LMDGuard.Used)
	assert.Equal(2, fsu.usCalled)
	assert.Equal(2, fso.DSCnt)
	assert.Equal(expLsVR, fc.InLsVRObj)

	// force a locking error
	app.LMDGuard.Drain()
	ret = c.RunBody(nil)
	assert.Nil(ret)

	foundReleasingLock := 0
	foundFailedLock := 0
	tl.Iterate(func(i uint64, s string) {
		if res, err := regexp.MatchString("Releasing state lock", s); err == nil && res {
			foundReleasingLock++
		}
		if res, err := regexp.MatchString("Failed to get state lock", s); err == nil && res {
			foundFailedLock++
		}
	})
	assert.Equal(3, foundReleasingLock)
	assert.Equal(1, foundFailedLock)
}

func TestShouldDispatch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
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

func TestShouldDispatchReallocatingCache(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM

	c := Component{}
	c.Init(app)
	rhs := &vra.RequestHandlerState{}

	req := &models.VolumeSeriesRequest{}
	req.Meta = &models.ObjMeta{ID: "vr1"}
	req.RequestedOperations = []string{com.VolReqOpChangeCapacity}
	rhs.Request = req
	rhs.HasChangeCapacity = true

	for _, state := range []string{"REALLOCATING_CACHE", "UNDO_REALLOCATING_CACHE"} {
		req.VolumeSeriesRequestState = state
		assert.True(c.ShouldDispatch(nil, rhs))
	}

	for _, state := range []string{"NEW", "CHANGING_CAPACITY", "UNDO_CHANGING_CAPACITY", "RESIZING_CACHE", "UNDO_RESIZING_CACHE"} {
		req.VolumeSeriesRequestState = state
		assert.False(c.ShouldDispatch(nil, rhs))
	}
}

func vsrClone(o *models.VolumeSeriesRequest) *models.VolumeSeriesRequest {
	n := new(models.VolumeSeriesRequest)
	testutils.Clone(o, n)
	return n
}

func vsClone(vs *models.VolumeSeries) *models.VolumeSeries {
	n := new(models.VolumeSeries)
	testutils.Clone(vs, n)
	return n
}

func lmdClone(lmd *models.LifecycleManagementData) *models.LifecycleManagementData {
	n := new(models.LifecycleManagementData)
	testutils.Clone(lmd, n)
	return n
}
