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


package snapper

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	fakeC "github.com/Nuvoloso/kontroller/pkg/clusterd/fake"
	fcl "github.com/Nuvoloso/kontroller/pkg/clusterd/fake"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	fw "github.com/Nuvoloso/kontroller/pkg/util/fake"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestSnapperMethods(t *testing.T) {
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

	// init fails to create watcher
	c := newSnapperComp()
	evM.RetWErr = fmt.Errorf("watch-error")
	assert.NotPanics(func() { c.Init(app) })
	assert.Empty(c.watcherID)
	assert.Equal(app, c.App)
	assert.Equal(app.Log, c.Log)

	// init creates a watcher
	c = newSnapperComp()
	evM.RetWErr = nil
	evM.RetWID = "watcher-id"
	assert.NotPanics(func() { c.Init(app) })
	assert.Equal(app, c.App)
	assert.Equal(app.Log, c.Log)
	assert.Equal(5*time.Second, c.sleepPeriod)
	assert.Equal(0, c.runCount)
	assert.NotNil(c.worker)
	assert.Equal("watcher-id", c.watcherID)

	err := c.CrudeNotify(crude.WatcherEvent, &crude.CrudEvent{})
	assert.Nil(err)

	// replace the worker
	tw := &fw.Worker{}
	c.worker = tw

	c.Start()
	assert.NotNil(c.oCrud)
	assert.Equal(0, c.runCount)
	assert.Equal(1, tw.CntStart)

	c.Stop()
	foundStarting := 0
	foundStopped := 0
	assert.Equal(1, tw.CntStop)
	tl.Iterate(func(i uint64, s string) {
		if res, err := regexp.MatchString("Starting Snapper", s); err == nil && res {
			foundStarting++
		}
		if res, err := regexp.MatchString("Stopped Snapper", s); err == nil && res {
			foundStopped++
		}
	})
	assert.Equal(1, foundStarting)
	assert.Equal(1, foundStopped)

	// validate watcher patterns
	wa := c.getWatcherArgs()
	assert.NotNil(wa)
	assert.Len(wa.Matchers, 1)

	m := wa.Matchers[0]
	assert.NotEmpty(m.MethodPattern)
	tl.Logger().Debugf("0.MethodPattern: %s", m.MethodPattern)
	re := regexp.MustCompile(m.MethodPattern)
	assert.True(re.MatchString("PATCH"))
	assert.NotEmpty(m.URIPattern)
	tl.Logger().Debugf("0.URIPattern: %s", m.URIPattern)
	re = regexp.MustCompile(m.URIPattern)
	assert.True(re.MatchString("/volume-series-requests/19cec06e-8ec4-44de-8b5b-12cff39b2de7?set=volumeSeriesRequestState&set=requestMessages&set=mountedNodeDevice&set=snapshot&set=lifecycleManagementData&set=systemTags&version=13"))
	assert.True(re.MatchString("/volume-series-requests/19cec06e-8ec4-44de-8b5b-12cff39b2de7?set=volumeSeriesRequestState&set=lifecycleManagementData&set=requestMessages&set=mountedNodeDevice&set=snapshot&set=systemTags&version=13"))
	assert.False(re.MatchString("/volume-series-requests/19cec06e-8ec4-44de-8b5b-12cff39b2de7?set=volumeSeriesRequestState&set=requestMessages&set=mountedNodeDevice&set=snapshot&set=systemTags&version=13"))
	assert.NotEmpty(m.ScopePattern)
	tl.Logger().Debugf("0.ScopePattern: %s", m.ScopePattern)
	re = regexp.MustCompile(m.ScopePattern)
	assert.False(re.MatchString("foo:bar volumeSeriesRequestState:VOLUME_CONFIG abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:SUCCEEDED abc:123"))

}

func TestSnapperBuzz(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	fao := &fcl.AppObjects{}
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log:                   tl.Logger(),
			CGSnapshotSchedPeriod: 10 * time.Minute,
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	evM.RetWID = "watcher-id"

	c := newSnapperComp()
	c.Init(app)
	ret := c.Buzz(nil) // nil AppObjects
	assert.Nil(ret)
	assert.Empty(c.clusterID)
	assert.Equal(5*time.Second, c.sleepPeriod)
	assert.NotNil(c.worker)
	assert.Equal(1, c.runCount)

	c.App.AppObjects = fao
	ret = c.Buzz(nil) // nil cluster object
	assert.Nil(ret)

	fao.RetGCobj = &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "cl1",
			},
		},
	}
	fc := &fake.Client{}
	c.oCrud = fc
	fc.RetLsVErr = fmt.Errorf("Volume Series List failed")
	fc.RetLsVOk = nil
	c.runCount = 0
	ret = c.Buzz(nil)
	assert.Equal(fao.RetGCobj.Meta.ID, c.clusterID)
	assert.Equal(app.AppArgs.CGSnapshotSchedPeriod, c.sleepPeriod)
	assert.Equal(1, c.runCount)
}

func TestVolCanBeSnapped(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log:                         tl.Logger(),
			CGSnapshotSchedPeriod:       10 * time.Minute,
			CGSnapshotTimeoutMultiplier: .9,
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	evM.RetWID = "watcher-id"

	appS := &fakeC.AppServant{}
	app.AppServant = appS
	c := newSnapperComp()
	c.Init(app)

	vs := &models.VolumeSeries{}
	assert.False(c.volInUse(vs))

	vs.VolumeSeriesState = com.VolStateProvisioned
	assert.False(c.volInUse(vs))

	vs.VolumeSeriesState = com.VolStateInUse
	assert.True(c.volInUse(vs))

	vs.SystemTags = []string{fmt.Sprintf("%s:XXX", com.SystemTagVsrRestoring)}
	assert.False(c.volInUse(vs))
}

func TestPendingVSRs(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log:                         tl.Logger(),
			CGSnapshotSchedPeriod:       10 * time.Minute,
			CGSnapshotTimeoutMultiplier: .9,
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	evM.RetWID = "watcher-id"

	appS := &fakeC.AppServant{}
	app.AppServant = appS
	c := newSnapperComp()
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc
	vs := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: "VS-1",
			},
		},
	}
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{}
	pending, err := c.pendingVSRs(nil, vs)
	assert.Nil(err)
	assert.False(pending)
	assert.Equal(vs.Meta.ID, models.ObjID(swag.StringValue(fc.InLsVRObj.VolumeSeriesID)))
	assert.False(swag.BoolValue(fc.InLsVRObj.IsTerminated))

	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{
		Payload: []*models.VolumeSeriesRequest{
			&models.VolumeSeriesRequest{
				VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
					Meta: &models.ObjMeta{ID: "vsr-1"},
				},
			},
		},
	}
	pending, err = c.pendingVSRs(nil, vs)
	assert.Nil(err)
	assert.True(pending)

	fc.RetLsVRErr = fmt.Errorf("vsr list error")
	pending, err = c.pendingVSRs(nil, vs)
	assert.NotNil(err)
	assert.Regexp("vsr list error", err.Error())
	assert.True(pending)
}

func TestRunSnapperBody(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log:                         tl.Logger(),
			CGSnapshotSchedPeriod:       10 * time.Minute,
			CGSnapshotTimeoutMultiplier: .9,
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	evM.RetWID = "watcher-id"

	appS := &fakeC.AppServant{}
	app.AppServant = appS
	ctx := context.Background()
	c := newSnapperComp()
	c.Init(app)
	c.clusterID = models.ObjID("clusterID")
	fc := &fake.Client{}
	c.oCrud = fc

	resVS := &volume_series.VolumeSeriesListOK{
		Payload: []*models.VolumeSeries{
			&models.VolumeSeries{
				VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
					Meta: &models.ObjMeta{
						ID: "VS-1",
					},
				},
				VolumeSeriesMutable: models.VolumeSeriesMutable{
					VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
						VolumeSeriesState: "IN_USE",
					},
					VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
						ConsistencyGroupID: "CG-1",
						LifecycleManagementData: &models.LifecycleManagementData{
							NextSnapshotTime: strfmt.DateTime(time.Now().Add(-10 * time.Minute)),
						},
						ServicePlanID: "SP-1",
					},
				},
			},
			&models.VolumeSeries{
				VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
					Meta: &models.ObjMeta{
						ID: "VS-2",
					},
				},
				VolumeSeriesMutable: models.VolumeSeriesMutable{
					VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
						VolumeSeriesState: "IN_USE",
					},
					VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
						ConsistencyGroupID: "CG-1",
						LifecycleManagementData: &models.LifecycleManagementData{
							NextSnapshotTime: strfmt.DateTime(time.Now().Add(-8 * time.Minute)),
						},
						ServicePlanID: "SP-1",
					},
				},
			},
			&models.VolumeSeries{
				VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
					Meta: &models.ObjMeta{
						ID: "VS-3",
					},
				},
				VolumeSeriesMutable: models.VolumeSeriesMutable{
					VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
						VolumeSeriesState: "IN_USE",
					},
					VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
						ConsistencyGroupID: "CG-1",
						LifecycleManagementData: &models.LifecycleManagementData{
							NextSnapshotTime: strfmt.DateTime(time.Now().Add(-7 * time.Minute)),
						},
						ServicePlanID: "SP-1",
					},
				},
			},
		},
	}

	spObj := &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			Meta: &models.ObjMeta{
				ID: "SP-1",
			},
		},
		ServicePlanMutable: models.ServicePlanMutable{
			Slos: models.SloListMutable{
				"RPO": {ValueType: models.ValueType{Kind: "DURATION", Value: "10s"}},
			},
		},
	}

	cgObj := &models.ConsistencyGroup{
		ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
			Meta: &models.ObjMeta{
				ID: "CG-1",
			},
		},
		ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
			AccountID: "Account1",
		},
		ConsistencyGroupMutable: models.ConsistencyGroupMutable{
			SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
				DisableSnapshotCreation: false,
			},
		},
	}

	aObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{
				ID: "Account1",
			},
		},
		AccountMutable: models.AccountMutable{
			SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
				DisableSnapshotCreation: false,
			},
		},
	}

	vsrCreate := &models.VolumeSeriesRequest{}
	vsrCreate.ConsistencyGroupID = models.ObjIDMutable("CG-1")
	vsrCreate.ClusterID = models.ObjIDMutable("clusterID")
	vsrCreate.RequestedOperations = []string{com.VolReqOpCGCreateSnapshot}

	// One CG, disableSnapshotCreation is False in CG.
	fc.RetLsVErr = nil
	fc.RetLsVOk = resVS
	fc.RetVRCErr = nil
	appS.RetGSPObj = spObj
	appS.RetGSPErr = nil
	fc.RetCGFetchObj = cgObj
	fc.RetCGFetchErr = nil
	fc.RetAccFetchObj = aObj
	fc.RetAccFetchErr = nil
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{}
	timeBefore := time.Now()
	err := c.runSnapperBody(ctx)
	timeAfter := time.Now()
	assert.Nil(err)
	assert.Equal(1, tl.CountPattern("Created VSR for snapshot of consistency group CG-1"))
	assert.EqualValues(vsrCreate.ConsistencyGroupID, fc.InVRCArgs.ConsistencyGroupID)
	assert.EqualValues(vsrCreate.ClusterID, fc.InVRCArgs.ClusterID)
	assert.EqualValues(vsrCreate.RequestedOperations, fc.InVRCArgs.RequestedOperations)
	assert.True(timeBefore.Add(time.Duration(9) * time.Second).Before(time.Time(fc.InVRCArgs.CompleteByTime)))
	assert.True(timeAfter.Add(time.Duration(9) * time.Second).After(time.Time(fc.InVRCArgs.CompleteByTime)))
	assert.EqualValues("SP-1", appS.InGSPid)
	assert.EqualValues("CG-1", fc.InCGFetchID)
	tl.Flush()

	// One CG, snapshot policy in nil in CG
	cgObj.SnapshotManagementPolicy = nil
	fc.RetLsVErr = nil
	fc.RetLsVOk = resVS
	fc.RetVRCErr = nil
	appS.RetGSPObj = spObj
	appS.RetGSPErr = nil
	fc.RetCGFetchObj = cgObj
	fc.RetCGFetchErr = nil
	fc.RetAccFetchObj = aObj
	fc.RetAccFetchErr = nil
	err = c.runSnapperBody(ctx)
	assert.Nil(err)
	assert.Equal(1, tl.CountPattern("Created VSR for snapshot of consistency group CG-1"))
	assert.EqualValues(vsrCreate.ConsistencyGroupID, fc.InVRCArgs.ConsistencyGroupID)
	assert.EqualValues(vsrCreate.ClusterID, fc.InVRCArgs.ClusterID)
	assert.EqualValues(vsrCreate.RequestedOperations, fc.InVRCArgs.RequestedOperations)
	assert.EqualValues("SP-1", appS.InGSPid)
	assert.EqualValues("CG-1", fc.InCGFetchID)
	assert.EqualValues("Account1", fc.InAccFetchID)
	tl.Flush()

	// One CG, VSR create fails
	fc.RetLsVErr = nil
	fc.RetLsVOk = resVS
	fc.RetVRCErr = fmt.Errorf("Volume Series Request Create failed")
	err = c.runSnapperBody(ctx)
	assert.Nil(err)
	assert.Equal(1, tl.CountPattern("Snapshot VSR create failed for CG *"))
	assert.Equal(0, tl.CountPattern("Created VSR for snapshot of consistency group CG-1"))
	assert.EqualValues(vsrCreate.ConsistencyGroupID, fc.InVRCArgs.ConsistencyGroupID)
	assert.EqualValues(vsrCreate.ClusterID, fc.InVRCArgs.ClusterID)
	assert.EqualValues(vsrCreate.RequestedOperations, fc.InVRCArgs.RequestedOperations)
	fc.RetVRCErr = nil // reset
	tl.Flush()

	// One CG, Account Fetch Fails
	fc.RetLsVErr = nil
	fc.RetLsVOk = resVS
	fc.RetAccFetchErr = fmt.Errorf("Account Fetch failed")
	err = c.runSnapperBody(ctx)
	assert.Nil(err)
	assert.Equal(1, tl.CountPattern("Failed to fetch account *"))
	assert.Equal(0, tl.CountPattern("Created VSR for snapshot of consistency group CG-1"))
	assert.EqualValues(vsrCreate.ConsistencyGroupID, fc.InVRCArgs.ConsistencyGroupID)
	assert.EqualValues(vsrCreate.ClusterID, fc.InVRCArgs.ClusterID)
	assert.EqualValues(vsrCreate.RequestedOperations, fc.InVRCArgs.RequestedOperations)
	fc.RetAccFetchErr = nil // reset
	tl.Flush()

	// One CG, CG Fetch Fails
	fc.RetLsVErr = nil
	fc.RetLsVOk = resVS
	fc.RetCGFetchErr = fmt.Errorf("Consistency Group failed")
	err = c.runSnapperBody(ctx)
	assert.Nil(err)
	assert.Equal(1, tl.CountPattern("Failed to fetch CG *"))
	assert.Equal(0, tl.CountPattern("Created VSR for snapshot of consistency group CG-1"))
	assert.EqualValues(vsrCreate.ConsistencyGroupID, fc.InVRCArgs.ConsistencyGroupID)
	assert.EqualValues(vsrCreate.ClusterID, fc.InVRCArgs.ClusterID)
	assert.EqualValues(vsrCreate.RequestedOperations, fc.InVRCArgs.RequestedOperations)
	fc.RetCGFetchErr = nil // reset
	tl.Flush()

	// Multiple CGs
	resVS.Payload[0].ConsistencyGroupID = "CG-2"
	fc.RetLsVErr = nil
	fc.RetLsVOk = resVS
	err = c.runSnapperBody(ctx)
	assert.Nil(err)
	assert.Equal(1, tl.CountPattern("Created VSR for snapshot of consistency group CG-1"))
	assert.Equal(1, tl.CountPattern("Created VSR for snapshot of consistency group CG-2"))
	tl.Flush()

	// CG-2 not ready for snapshot
	resVS.Payload[0].LifecycleManagementData.NextSnapshotTime = strfmt.DateTime(time.Now().Add(7 * time.Minute))
	err = c.runSnapperBody(ctx)
	assert.Nil(err)
	assert.Equal(1, tl.CountPattern("Created VSR for snapshot of consistency group CG-1"))
	assert.Equal(0, tl.CountPattern("Created VSR for snapshot of consistency group CG-2"))
	resVS.Payload[0].LifecycleManagementData.NextSnapshotTime = strfmt.DateTime(time.Now().Add(-7 * time.Minute)) //reset
	tl.Flush()

	// CG-1 has existing VSRs, shouldn't skip second CG
	fc.RetVRCErr = centrald.ErrorRequestInConflict
	err = c.runSnapperBody(ctx)
	assert.Nil(err)
	assert.Equal(2, tl.CountPattern("Snapshot VSR create failed for CG"))
	fc.RetVRCErr = nil // reset
	tl.Flush()

	// Account fetch error, shouldn't skip second CG
	fc.RetAccFetchErr = fmt.Errorf("Account Fetch failed")
	err = c.runSnapperBody(ctx)
	assert.Nil(err)
	assert.Equal(2, tl.CountPattern("Failed to fetch account"))
	fc.RetAccFetchErr = nil // reset
	tl.Flush()

	// CG fetch error, shouldn't skip second CG
	fc.RetCGFetchErr = fmt.Errorf("Consistency Group failed")
	err = c.runSnapperBody(ctx)
	assert.Nil(err)
	assert.Equal(2, tl.CountPattern("Failed to fetch CG"))
	fc.RetCGFetchErr = nil // reset
	tl.Flush()

	// CG-1 1 volume in not ready state, other in ready state. still take snapshot
	resVS.Payload[1].VolumeSeriesState = "NOT_READY"
	fc.RetLsVOk = resVS
	err = c.runSnapperBody(ctx)
	assert.Nil(err)
	assert.Equal(1, tl.CountPattern("Created VSR for snapshot of consistency group CG-1"))
	assert.Equal(1, tl.CountPattern("Created VSR for snapshot of consistency group CG-2"))
	resVS.Payload[1].VolumeSeriesState = "IN_USE" // reset
	tl.Flush()

	// CG-1 both volumes in not ready state. No snapshots taken
	resVS.Payload[1].VolumeSeriesState = "NOT_READY"
	resVS.Payload[2].VolumeSeriesState = "NOT_READY"
	err = c.runSnapperBody(ctx)
	assert.Nil(err)
	assert.Equal(0, tl.CountPattern("Created VSR for snapshot of consistency group CG-1"))
	assert.Equal(1, tl.CountPattern("Created VSR for snapshot of consistency group CG-2"))
	resVS.Payload[1].VolumeSeriesState = "IN_USE" // reset
	resVS.Payload[2].VolumeSeriesState = "IN_USE" // reset
	tl.Flush()

	// CG-1 both volumes in ready state but have pending VSRs. Snap not taken
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{
		Payload: []*models.VolumeSeriesRequest{
			&models.VolumeSeriesRequest{
				VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
					Meta: &models.ObjMeta{ID: "vsr-1"},
				},
			},
		},
	}
	err = c.runSnapperBody(ctx)
	assert.Nil(err)
	assert.Equal(0, tl.CountPattern("Created VSR for snapshot of consistency group CG-1"))
	assert.Equal(0, tl.CountPattern("Created VSR for snapshot of consistency group CG-2"))
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{} // reset
	tl.Flush()

	// VSR list fails
	fc.RetLsVRErr = fmt.Errorf("vsr list failed")
	err = c.runSnapperBody(ctx)
	assert.NotNil(err)
	assert.Errorf(err, "vsr list failed")
	fc.RetLsVRErr = nil
	tl.Flush()

	// GetServicePlan fails
	appS.RetGSPErr = fmt.Errorf("Get Service Plan failed")
	err = c.runSnapperBody(ctx)
	assert.NotNil(err)
	assert.Errorf(err, "Get Service Plan failed")
	tl.Flush()

	// VSL fails
	fc.RetLsVErr = fmt.Errorf("Volume Series List failed")
	fc.RetLsVOk = nil
	err = c.runSnapperBody(ctx)
	assert.NotNil(err)
	assert.Errorf(err, "Volume Series List failed")

	// VSL empty
	fc.RetLsVErr = nil
	fc.RetLsVOk = &volume_series.VolumeSeriesListOK{
		Payload: []*models.VolumeSeries{},
	}
	err = c.runSnapperBody(ctx)
	assert.Nil(err)
}

func TestRunSnapperBodyWLastSnap(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log:                         tl.Logger(),
			CGSnapshotSchedPeriod:       10 * time.Minute,
			CGSnapshotTimeoutMultiplier: .9,
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	evM.RetWID = "watcher-id"

	ctx := context.Background()
	c := newSnapperComp()
	c.Init(app)
	c.clusterID = models.ObjID("clusterID")
	fc := &fake.Client{}
	c.oCrud = fc

	resVS := &volume_series.VolumeSeriesListOK{
		Payload: []*models.VolumeSeries{
			&models.VolumeSeries{
				VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
					Meta: &models.ObjMeta{
						ID: "VS-1",
					},
				},
				VolumeSeriesMutable: models.VolumeSeriesMutable{
					VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
						VolumeSeriesState: "CONFIGURED",
					},
					VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
						ConsistencyGroupID: "CG-1",
						LifecycleManagementData: &models.LifecycleManagementData{
							NextSnapshotTime: strfmt.DateTime(time.Now().Add(-10 * time.Minute)),
						},
						ServicePlanID: "SP-1",
					},
				},
			},
			&models.VolumeSeries{
				VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
					Meta: &models.ObjMeta{
						ID: "VS-2",
					},
				},
				VolumeSeriesMutable: models.VolumeSeriesMutable{
					VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
						VolumeSeriesState: "CONFIGURED",
					},
					VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
						ConsistencyGroupID: "CG-1",
						LifecycleManagementData: &models.LifecycleManagementData{
							NextSnapshotTime: strfmt.DateTime(time.Now().Add(-8 * time.Minute)),
						},
						ServicePlanID: "SP-1",
					},
				},
			},
			&models.VolumeSeries{
				VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
					Meta: &models.ObjMeta{
						ID: "VS-3",
					},
				},
				VolumeSeriesMutable: models.VolumeSeriesMutable{
					VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
						VolumeSeriesState: "CONFIGURED",
					},
					VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
						ConsistencyGroupID: "CG-1",
						LifecycleManagementData: &models.LifecycleManagementData{
							NextSnapshotTime:    strfmt.DateTime(time.Now().Add(-7 * time.Minute)),
							FinalSnapshotNeeded: true,
						},
						ServicePlanID: "SP-1",
					},
				},
			},
		},
	}

	cgObj := &models.ConsistencyGroup{
		ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
			Meta: &models.ObjMeta{
				ID: "CG-1",
			},
		},
		ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
			AccountID: "Account1",
		},
		ConsistencyGroupMutable: models.ConsistencyGroupMutable{
			SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
				DisableSnapshotCreation: false,
			},
		},
	}

	aObj := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{
				ID: "Account1",
			},
		},
		AccountMutable: models.AccountMutable{
			SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
				DisableSnapshotCreation: false,
			},
		},
	}

	vsrCreate := &models.VolumeSeriesRequest{}
	vsrCreate.ConsistencyGroupID = models.ObjIDMutable("CG-1")
	vsrCreate.ClusterID = models.ObjIDMutable("clusterID")
	vsrCreate.RequestedOperations = []string{com.VolReqOpCGCreateSnapshot}

	// One CG, disableSnapshotCreation is False in CG.
	fc.RetLsVErr = nil
	fc.RetLsVOk = resVS
	fc.RetVRCErr = nil
	fc.RetCGFetchObj = cgObj
	fc.RetCGFetchErr = nil
	fc.RetAccFetchObj = aObj
	fc.RetAccFetchErr = nil
	fc.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{}
	err := c.runSnapperBody(ctx)
	assert.Nil(err)
	assert.Equal(1, tl.CountPattern("Created VSR for snapshot of consistency group CG-1"))
	assert.EqualValues(vsrCreate.ConsistencyGroupID, fc.InVRCArgs.ConsistencyGroupID)
	assert.EqualValues(vsrCreate.ClusterID, fc.InVRCArgs.ClusterID)
	assert.EqualValues(vsrCreate.RequestedOperations, fc.InVRCArgs.RequestedOperations)
	assert.EqualValues("CG-1", fc.InCGFetchID)
	tl.Flush()

	// One CG, disableSnapshotCreation is False in CG.
	fc.RetLsVErr = nil
	fc.RetLsVOk = resVS
	fc.RetVRCErr = nil
	fc.RetCGFetchObj = cgObj
	fc.RetCGFetchErr = nil
	fc.RetAccFetchObj = aObj
	fc.RetAccFetchErr = nil
	resVS.Payload[1].VolumeSeriesState = "PROVISIONED"
	err = c.runSnapperBody(ctx)
	assert.Nil(err)
	assert.Equal(1, tl.CountPattern("Created VSR for snapshot of consistency group CG-1"))
	assert.EqualValues(vsrCreate.ConsistencyGroupID, fc.InVRCArgs.ConsistencyGroupID)
	assert.EqualValues(vsrCreate.ClusterID, fc.InVRCArgs.ClusterID)
	assert.EqualValues(vsrCreate.RequestedOperations, fc.InVRCArgs.RequestedOperations)
	assert.EqualValues("CG-1", fc.InCGFetchID)
	tl.Flush()

	// One CG, finalSnapshotNeeded is set to false
	resVS.Payload[2].LifecycleManagementData.FinalSnapshotNeeded = false
	fc.RetLsVErr = nil
	fc.RetLsVOk = resVS
	fc.RetVRCErr = nil
	fc.RetCGFetchObj = cgObj
	fc.RetCGFetchErr = nil
	fc.RetAccFetchObj = aObj
	fc.RetAccFetchErr = nil
	err = c.runSnapperBody(ctx)
	assert.Nil(err)
	assert.Equal(0, tl.CountPattern("Created VSR for snapshot of consistency group CG-1"))
	tl.Flush()
	resVS.Payload[2].LifecycleManagementData.FinalSnapshotNeeded = true // reset
}
