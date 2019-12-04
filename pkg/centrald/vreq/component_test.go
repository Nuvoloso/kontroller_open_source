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

	mcvr "github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	fhk "github.com/Nuvoloso/kontroller/pkg/housekeeping/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/swag"
	flags "github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

// The following states are processed by centrald (other than NEW)
var testNotNewStates = []string{
	"ALLOCATING_CAPACITY",
	"BINDING",
	"CANCELING_REQUESTS",
	"CAPACITY_WAIT",
	"CHANGING_CAPACITY",
	"CREATING",
	"DELETING_NODE",
	"DELETING_SPA",
	"DETACHING_STORAGE",
	"DETACHING_VOLUMES",
	"DRAINING_REQUESTS",
	"RENAMING",
	"UNDO_ALLOCATING_CAPACITY",
	"UNDO_BINDING",
	"UNDO_CHANGING_CAPACITY",
	"UNDO_CREATING",
	"UNDO_RENAMING",
	"VOLUME_DETACHED",
	"VOLUME_DETACHING",
	"VOLUME_DETACH_WAIT",
}

// The NEW state is processed by centrald with the following initial operations
var testNewFirstOp = []string{
	"ALLOCATE_CAPACITY",
	"BIND",
	"CHANGE_CAPACITY",
	"CREATE",
	"DELETE",
	"DELETE_SPA",
	"NODE_DELETE",
	"RENAME",
	"UNBIND",
	"VOL_DETACH",
}

func TestAppFlags(t *testing.T) {
	assert := assert.New(t)

	args := Args{}
	parser := flags.NewParser(&args, flags.Default&^flags.PrintErrors)
	_, err := parser.ParseArgs([]string{})
	assert.NoError(err)

	// check default values
	assert.False(args.DebugREI)
	assert.Equal(30*time.Second, args.RetryInterval)
	assert.Equal(12*time.Hour, args.VSRPurgeTaskInterval)
	assert.Equal(7*24*time.Hour, args.VSRRetentionPeriod)
}

func TestComponentInit(t *testing.T) {
	assert := assert.New(t)
	args := Args{
		RetryInterval: time.Second * 20,
	}
	// This actually calls centrald.AppRegisterComponent but we can't intercept that
	c := ComponentInit(&args)
	assert.Equal(args.RetryInterval, c.RetryInterval)

	// check that centraldStates was properly initialized
	assert.True(len(centraldStates) > 1)
	for _, state := range centraldStates {
		if state != "NEW" {
			si := vra.GetStateInfo(state)
			assert.True(si.GetProcess("foo") == vra.ApCentrald)
			assert.Contains(testNotNewStates, state)
		}
	}
	for _, state := range testNotNewStates {
		assert.Contains(centraldStates, state)
	}
}

func TestComponentMethods(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	fts := &fhk.TaskScheduler{}
	app.TaskScheduler = fts

	// init fails to create a watcher
	ri := time.Duration(10 * time.Second)
	c := Component{
		Args: Args{RetryInterval: ri},
	}
	evM.RetWErr = fmt.Errorf("watch-error")
	c.Init(app)
	assert.Empty(c.watcherID)
	assert.EqualValues(ri, c.Animator.RetryInterval)
	assert.Equal(app, c.App)
	assert.Equal(app.Log, c.Log)
	assert.False(c.fatalError)

	// init creates a watcher
	c = Component{
		Args: Args{RetryInterval: ri},
	}
	evM.RetWErr = nil
	evM.RetWID = "watcher-id"
	c.Init(app)
	assert.Equal("watcher-id", c.watcherID)
	assert.EqualValues(WatcherRetryMultiplier*ri, c.Animator.RetryInterval)
	assert.Equal(app, c.App)
	assert.Equal(app.Log, c.Log)
	assert.False(c.fatalError)

	// change the interval for the UT
	utInterval := time.Millisecond * 10
	c.RetryInterval = utInterval
	assert.Equal(0, c.runCount)
	c.fatalError = true
	c.Start() // invokes run/RunBody asynchronously but won't do anything if c.fatalErr
	assert.True(c.runCount == 0)
	for c.runCount == 0 {
		time.Sleep(utInterval)
	}

	// stop
	c.Stop()
	foundRun := 0
	foundStarting := 0
	foundStopped := 0
	tl.Iterate(func(i uint64, s string) {
		if res, err := regexp.MatchString("VolumeSeriesHandler RunBody", s); err == nil && res {
			foundRun++
		}
		if res, err := regexp.MatchString("Starting VolumeSeriesHandler", s); err == nil && res {
			foundStarting++
		}
		if res, err := regexp.MatchString("Stopped VolumeSeriesHandler", s); err == nil && res {
			foundStopped++
		}
	})
	assert.True(foundRun > 0)
	assert.Equal(1, foundStarting)
	assert.Equal(1, foundStopped)

	// validate watcher patterns
	wa := c.getWatcherArgs()
	assert.NotNil(wa)
	assert.Len(wa.Matchers, 5)

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
	assert.Empty(m.MethodPattern)
	assert.NotEmpty(m.URIPattern)
	tl.Logger().Debugf("1.URIPattern: %s", m.URIPattern)
	re = regexp.MustCompile(m.URIPattern)
	assert.True(re.MatchString("/pools"))
	assert.True(re.MatchString("/pools/id"))
	assert.Empty(m.ScopePattern)

	m = wa.Matchers[2]
	assert.Empty(m.MethodPattern)
	assert.NotEmpty(m.URIPattern)
	tl.Logger().Debugf("2.URIPattern: %s", m.URIPattern)
	re = regexp.MustCompile(m.URIPattern)
	assert.True(re.MatchString("/service-plan-allocations"))
	assert.True(re.MatchString("/service-plan-allocations/id"))
	assert.Empty(m.ScopePattern)

	m = wa.Matchers[3]
	assert.NotEmpty(m.MethodPattern)
	tl.Logger().Debugf("3.MethodPattern: %s", m.MethodPattern)
	re = regexp.MustCompile(m.MethodPattern)
	assert.True(re.MatchString("PATCH"))
	assert.NotEmpty(m.URIPattern)
	tl.Logger().Debugf("3.URIPattern: %s", m.URIPattern)
	re = regexp.MustCompile(m.URIPattern)
	assert.True(re.MatchString("/volume-series-requests/id"))
	assert.NotEmpty(m.ScopePattern)
	tl.Logger().Debugf("3.ScopePattern: %s", m.ScopePattern)
	re = regexp.MustCompile(m.ScopePattern)
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:UNDO_CREATING abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:UNDO_BINDING abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:BINDING abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:UNDO_CHANGING_CAPACITY abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:CHANGING_CAPACITY abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:UNDO_ALLOCATING_CAPACITY abc:123"))
	assert.False(re.MatchString("foo:bar volumeSeriesRequestState:RESIZING_CACHE abc:123"))
	assert.False(re.MatchString("foo:bar volumeSeriesRequestState:REALLOCATING_CACHE abc:123"))

	m = wa.Matchers[4]
	assert.NotEmpty(m.MethodPattern)
	tl.Logger().Debugf("4.MethodPattern: %s", m.MethodPattern)
	re = regexp.MustCompile(m.MethodPattern)
	assert.True(re.MatchString("POST"))
	assert.NotEmpty(m.URIPattern)
	tl.Logger().Debugf("4.URIPattern: %s", m.URIPattern)
	re = regexp.MustCompile(m.URIPattern)
	assert.True(re.MatchString("/volume-series-requests/id/cancel"))
	assert.NotEmpty(m.ScopePattern)
	tl.Logger().Debugf("4.ScopePattern: %s", m.ScopePattern)
	re = regexp.MustCompile(m.ScopePattern)
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:CREATING abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:BINDING abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:CHANGING_CAPACITY abc:123"))
	assert.True(re.MatchString("foo:bar volumeSeriesRequestState:CAPACITY_WAIT abc:123"))
	assert.False(re.MatchString("foo:bar volumeSeriesRequestState:RESIZING_CACHE abc:123"))
	assert.False(re.MatchString("foo:bar volumeSeriesRequestState:REALLOCATING_CACHE abc:123"))
}

func TestRunBody(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	fts := &fhk.TaskScheduler{}
	app.TaskScheduler = fts

	c := Component{}
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc

	// Run body, startup logic cases
	fc = &fake.Client{}
	c.oCrud = fc
	fc.RetLsVRObj = nil
	fc.RetLsVRErr = fmt.Errorf("VolumeSeriesRequestList fails")
	assert.False(c.startupCleanupDone)
	ret := c.RunBody(nil)
	assert.True(c.startupCleanupDone)
	assert.Nil(ret)

	fc.InLsVRObj = nil
	fc.RetLsVRObj = &mcvr.VolumeSeriesRequestListOK{
		Payload: make([]*models.VolumeSeriesRequest, 0),
	}
	fc.RetLsVRErr = nil
	expLsVR := mcvr.NewVolumeSeriesRequestListParams()
	expLsVR.VolumeSeriesRequestState = centraldStates
	expLsVR.IsTerminated = swag.Bool(false)
	c.RunBody(nil)
	assert.True(c.startupCleanupDone)
	assert.Equal(expLsVR, fc.InLsVRObj)
	foundCleanupStart := 0
	foundCleanupDone := 0
	tl.Iterate(func(i uint64, s string) {
		if res, err := regexp.MatchString("cleanup starting", s); err == nil && res {
			foundCleanupStart++
		}
		if res, err := regexp.MatchString("cleanup done", s); err == nil && res {
			foundCleanupDone++
		}
	})
	assert.Equal(1, foundCleanupStart)
	assert.Equal(1, foundCleanupDone)
	assert.Equal(c.vsrTaskID, "")

	// VSR purge task
	// failure
	fts.RetRtID = "task_id"
	fts.RetRtErr = fmt.Errorf("task failed to run")
	c.lastPurgeTaskTime = time.Now().Add(-time.Hour)
	c.VSRPurgeTaskInterval = time.Duration(1 * time.Second)
	c.RunBody(nil)
	assert.Equal(fts.RetRtID, c.vsrTaskID)

	// success
	fts.RetRtErr = nil
	c.RunBody(nil)
	assert.Equal("", c.vsrTaskID)
}

func TestShouldDispatch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	fts := &fhk.TaskScheduler{}
	app.TaskScheduler = fts

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

func crpClone(o *models.CapacityReservationPlan) *models.CapacityReservationPlan {
	n := new(models.CapacityReservationPlan)
	testutils.Clone(o, n)
	return n
}

func poolClone(o *models.Pool) *models.Pool {
	n := new(models.Pool)
	testutils.Clone(o, n)
	return n
}

func poolListClone(o []*models.Pool) []*models.Pool {
	n := make([]*models.Pool, 0, len(o))
	testutils.Clone(o, &n)
	return n
}

func spaClone(o *models.ServicePlanAllocation) *models.ServicePlanAllocation {
	n := new(models.ServicePlanAllocation)
	testutils.Clone(o, n)
	return n
}

func vsClone(o *models.VolumeSeries) *models.VolumeSeries {
	n := new(models.VolumeSeries)
	testutils.Clone(o, n)
	return n
}

func vsrClone(o *models.VolumeSeriesRequest) *models.VolumeSeriesRequest {
	n := new(models.VolumeSeriesRequest)
	testutils.Clone(o, n)
	return n
}

func spClone(o *models.ServicePlan) *models.ServicePlan {
	n := new(models.ServicePlan)
	testutils.Clone(o, n)
	return n
}

func clClone(o *models.Cluster) *models.Cluster {
	n := new(models.Cluster)
	testutils.Clone(o, n)
	return n
}

func dClone(o *models.CSPDomain) *models.CSPDomain {
	n := new(models.CSPDomain)
	testutils.Clone(o, n)
	return n
}

type fakeSFC struct {
	InFSFName   models.StorageFormulaName
	RetFSFObj   *models.StorageFormula
	InCRPArgs   *centrald.CreateCapacityReservationPlanArgs
	RetCRPObj   *models.CapacityReservationPlan
	InCRPArgsL  []*centrald.CreateCapacityReservationPlanArgs
	RetCRPObjL  []*models.CapacityReservationPlan
	InSVPArgs   *centrald.SelectVolumeProvisionersArgs
	RetSVPObj   []*models.Pool
	RetSVPErr   error
	InSFfSPid   models.ObjIDMutable
	RetSFfSPObj []*models.StorageFormula
	RetSFfSPErr error
	InSFfDo     *models.CSPDomain
	InSFfDf     []*models.StorageFormula
	RetSFfDf    *models.StorageFormula
	RetSFfDErr  error
}

var _ = centrald.StorageFormulaComputer(&fakeSFC{})

func (sfc *fakeSFC) SupportedStorageFormulas() []*models.StorageFormula {
	return nil
}
func (sfc *fakeSFC) FindStorageFormula(formulaName models.StorageFormulaName) *models.StorageFormula {
	sfc.InFSFName = formulaName
	return sfc.RetFSFObj
}
func (sfc *fakeSFC) ValidateStorageFormula(formulaName models.StorageFormulaName) bool {
	return true
}
func (sfc *fakeSFC) StorageFormulaForAttrs(ioProfile *models.IoProfile, slos models.SloListMutable) []*models.StorageFormula {
	return nil
}
func (sfc *fakeSFC) CreateCapacityReservationPlan(args *centrald.CreateCapacityReservationPlanArgs) *models.CapacityReservationPlan {
	sfc.InCRPArgs = args
	if len(sfc.RetCRPObjL) > 0 { // multi-call support
		if sfc.InCRPArgsL == nil {
			sfc.InCRPArgsL = []*centrald.CreateCapacityReservationPlanArgs{}
		}
		sfc.InCRPArgsL = append(sfc.InCRPArgsL, args)
		sfc.RetCRPObj, sfc.RetCRPObjL = sfc.RetCRPObjL[0], sfc.RetCRPObjL[1:]
	}
	return sfc.RetCRPObj
}
func (sfc *fakeSFC) SelectVolumeProvisioners(args *centrald.SelectVolumeProvisionersArgs) ([]*models.Pool, error) {
	sfc.InSVPArgs = args
	return sfc.RetSVPObj, sfc.RetSVPErr
}

func (sfc *fakeSFC) StorageFormulasForServicePlan(ctx context.Context, spID models.ObjIDMutable, spObj *models.ServicePlan) ([]*models.StorageFormula, error) {
	sfc.InSFfSPid = spID
	return sfc.RetSFfSPObj, sfc.RetSFfSPErr
}

func (sfc *fakeSFC) SelectFormulaForDomain(cspDomain *models.CSPDomain, formulas []*models.StorageFormula) (*models.StorageFormula, error) {
	sfc.InSFfDo = cspDomain
	sfc.InSFfDf = formulas
	return sfc.RetSFfDf, sfc.RetSFfDErr
}

func (sfc *fakeSFC) StorageFormulaComputeCost(sf *models.StorageFormula, storageCost map[string]models.StorageCost) *models.ServicePlanCost {
	return nil
}
