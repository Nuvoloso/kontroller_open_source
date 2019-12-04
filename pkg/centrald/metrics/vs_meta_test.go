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


package metrics

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/consistency_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	fmm "github.com/Nuvoloso/kontroller/pkg/metricmover/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	fpg "github.com/Nuvoloso/kontroller/pkg/pgdb/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/jackc/pgx"
	"github.com/stretchr/testify/assert"
)

func TestVolumeMetadataInit(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log:    tl.Logger(),
			Server: &restapi.Server{},
		},
	}
	evMA := &crude.ManagerArgs{
		Log: tl.Logger(),
	}
	evM := crude.NewManager(evMA) // the real CRUD manager
	app.CrudeOps = evM
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	c := &Component{}
	c.Init(app)
	c.VolumeMetadataMetricSuppressStartup = true // for the UT

	vmh := &vsMetaHandler{}

	// validate that the watcher gets created as it is assumed to not fail in Init()
	wid, err := evM.Watch(vmh.getWatcherArgs(), vmh)
	assert.NoError(err)
	evM.TerminateWatcher(wid)

	vmh.Init(c)
	assert.Equal(c, vmh.c)
	assert.NotNil(vmh.rt)
	assert.Equal(vmh.rt.Period, c.VolumeMetadataMetricPeriod)
	assert.Equal(vmh.rt.RoundDown, c.VolumeMetadataMetricTruncate)
	assert.Equal(vmh.rt.CallImmediately, !c.VolumeMetadataMetricSuppressStartup)
	assert.NotEmpty(vmh.watcherID)

	vmh.Start()
	assert.NotNil(vmh.ctx)
	assert.NotNil(vmh.cancelFn)
	err = vmh.rt.Start(vmh)
	assert.Error(err) // already started
	assert.Regexp("active", err)

	vmh.Stop()
	assert.Nil(vmh.cancelFn)
}

func TestVolumeMetadataGenerateAndInsertMetrics(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ctx := context.Background()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log:    tl.Logger(),
			Server: &restapi.Server{},
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	c := &Component{}
	c.Init(app)
	c.VolumeMetadataMaxBuffered = 100
	fc := &fake.Client{}
	c.oCrud = fc

	vmh := &vsMetaHandler{}
	vmh.Init(c)

	var err error

	// vs list error
	fc.RetLsVErr = fmt.Errorf("volume-series-list-error")
	err = vmh.GenerateVolumeMetadataMetrics(ctx)
	assert.Error(err)
	assert.Regexp("volume-series-list-error", err)

	// cg list error
	vsRes := &volume_series.VolumeSeriesListOK{
		Payload: []*models.VolumeSeries{
			&models.VolumeSeries{},
			&models.VolumeSeries{},
			&models.VolumeSeries{},
			&models.VolumeSeries{},
		},
	}
	fc.RetLsVOk = vsRes
	fc.RetLsVErr = nil
	fc.RetCGListErr = fmt.Errorf("cg-list-error")
	err = vmh.GenerateVolumeMetadataMetrics(ctx)
	assert.Error(err)
	assert.Regexp("cg-list-error", err)

	// list ok
	cgRes := &consistency_group.ConsistencyGroupListOK{
		Payload: []*models.ConsistencyGroup{
			&models.ConsistencyGroup{
				ConsistencyGroupAllOf0:  models.ConsistencyGroupAllOf0{Meta: &models.ObjMeta{ID: "CG1"}},
				ConsistencyGroupMutable: models.ConsistencyGroupMutable{ApplicationGroupIds: []models.ObjIDMutable{"AG11"}},
			},
			&models.ConsistencyGroup{
				ConsistencyGroupAllOf0:  models.ConsistencyGroupAllOf0{Meta: &models.ObjMeta{ID: "CG2"}},
				ConsistencyGroupMutable: models.ConsistencyGroupMutable{ApplicationGroupIds: []models.ObjIDMutable{"AG21", "AG22"}},
			},
			&models.ConsistencyGroup{
				ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{Meta: &models.ObjMeta{ID: "CG3"}},
			},
		},
	}
	expAGs := []string{
		"{AG11}",
		"{AG21,AG22}",
		"{}",
		"{}",
	}
	o := vsRes.Payload[0]
	o.Meta = &models.ObjMeta{ID: "VS1"}
	o.AccountID = "ACCOUNT1"
	o.BoundClusterID = "CLUSTER1"
	o.ServicePlanID = "SERVICEPLAN1"
	o.ConsistencyGroupID = "CG1"
	o.CapacityAllocations = make(map[string]models.CapacityAllocation)
	o.CapacityAllocations["SP1"] = models.CapacityAllocation{
		ConsumedBytes: swag.Int64(1000),
	}
	o.CapacityAllocations["SP2"] = models.CapacityAllocation{
		ConsumedBytes: swag.Int64(1000),
	}
	o.ConfiguredNodeID = "node1"
	o.CacheAllocations = make(map[string]models.CacheAllocation)
	o.CacheAllocations = map[string]models.CacheAllocation{
		"node1": {
			AllocatedSizeBytes: swag.Int64(1111),
			RequestedSizeBytes: swag.Int64(1111),
		},
		"node2": {
			AllocatedSizeBytes: swag.Int64(222),
			RequestedSizeBytes: swag.Int64(333),
		},
	}
	o.SizeBytes = swag.Int64(10000)
	o.SpaAdditionalBytes = swag.Int64(100)
	o = vsRes.Payload[1]
	o.Meta = &models.ObjMeta{ID: "VS2"}
	o.AccountID = "ACCOUNT1"
	o.BoundClusterID = "CLUSTER1"
	o.ServicePlanID = "SERVICEPLAN1"
	o.ConsistencyGroupID = "CG2"
	o.CapacityAllocations = make(map[string]models.CapacityAllocation)
	o.CapacityAllocations["SP1"] = models.CapacityAllocation{
		ConsumedBytes: swag.Int64(2000),
	}
	o.CapacityAllocations["SP2"] = models.CapacityAllocation{
		ConsumedBytes: swag.Int64(2000),
	}
	o.CacheAllocations = make(map[string]models.CacheAllocation)
	o.CacheAllocations = map[string]models.CacheAllocation{
		"node1": {
			AllocatedSizeBytes: swag.Int64(1111),
			RequestedSizeBytes: swag.Int64(1111),
		},
		"node3": {
			AllocatedSizeBytes: swag.Int64(333),
			RequestedSizeBytes: swag.Int64(444),
		},
	}
	o.ConfiguredNodeID = "node1"
	o.SizeBytes = swag.Int64(20000)
	o = vsRes.Payload[2]
	o.Meta = &models.ObjMeta{ID: "VS3"}
	o.AccountID = "ACCOUNT1"
	o.BoundClusterID = "CLUSTER1"
	o.ServicePlanID = "SERVICEPLAN1"
	o.ConsistencyGroupID = "CG3"
	o.CapacityAllocations = make(map[string]models.CapacityAllocation)
	o.CapacityAllocations["SP1"] = models.CapacityAllocation{
		ConsumedBytes: swag.Int64(3000),
	}
	o.CapacityAllocations["SP2"] = models.CapacityAllocation{
		ConsumedBytes: swag.Int64(3000),
	}
	o.CacheAllocations = make(map[string]models.CacheAllocation)
	o.CacheAllocations = map[string]models.CacheAllocation{
		"node1": {
			AllocatedSizeBytes: swag.Int64(1111),
			RequestedSizeBytes: swag.Int64(1111),
		},
		"node4": {
			AllocatedSizeBytes: swag.Int64(0),
			RequestedSizeBytes: swag.Int64(0),
		},
	}
	o.ConfiguredNodeID = "node1"
	o.SizeBytes = swag.Int64(30000)
	o = vsRes.Payload[3]
	o.Meta = &models.ObjMeta{ID: "VS4"}
	o.AccountID = "ACCOUNT1"
	o.BoundClusterID = "CLUSTER1"
	o.ServicePlanID = "SERVICEPLAN1"
	o.ConsistencyGroupID = "CG4" // not found, queries are not in a transaction
	o.CapacityAllocations = make(map[string]models.CapacityAllocation)
	o.CapacityAllocations["SP1"] = models.CapacityAllocation{
		ConsumedBytes: swag.Int64(3000),
	}
	o.CapacityAllocations["SP2"] = models.CapacityAllocation{
		ConsumedBytes: swag.Int64(3000),
	}
	o.CacheAllocations = make(map[string]models.CacheAllocation)
	o.CacheAllocations = map[string]models.CacheAllocation{
		"node1": {
			AllocatedSizeBytes: swag.Int64(1111),
			RequestedSizeBytes: swag.Int64(1111),
		},
	}
	o.ConfiguredNodeID = "node1"
	o.SizeBytes = swag.Int64(30000)
	o.SpaAdditionalBytes = swag.Int64(1000)

	fc.RetCGListObj = cgRes
	fc.RetCGListErr = nil
	assert.Equal(vmh.Queue.Length(), 0)
	nc := c.worker.GetNotifyCount()
	tsB := time.Now()
	vmh.ctx = ctx
	vmh.Beep(ctx)
	tsA := time.Now()
	assert.Equal(nc+1, c.worker.GetNotifyCount())
	assert.Equal(vmh.Queue.Length(), 4)
	for i := 0; i < 4; i++ {
		o = vsRes.Payload[i]
		r := vmh.Queue.PopHead().(*VolumeMetadataMetric)
		assert.True(tsB.Before(r.Timestamp) && tsA.After(r.Timestamp))
		assert.Equal(string(o.Meta.ID), r.VolumeSeriesID)
		assert.Equal(string(o.AccountID), r.AccountID)
		assert.Equal(string(o.BoundClusterID), r.ClusterID)
		assert.Equal(string(o.ServicePlanID), r.ServicePlanID)
		assert.Equal(string(o.ConsistencyGroupID), r.ConsistencyGroupID)
		assert.Equal(expAGs[i], r.ApplicationGroupIDs)
		assert.Equal(swag.Int64Value(o.SizeBytes), r.TotalBytes)
		assert.Equal(swag.Int64Value(o.SpaAdditionalBytes), r.CostBytes)
		ab := swag.Int64Value(o.SpaAdditionalBytes) + swag.Int64Value(o.SizeBytes)
		assert.Equal(ab, r.AllocatedBytes)
		var asb, rsb int64
		for _, ca := range o.CacheAllocations {
			asb += swag.Int64Value(ca.AllocatedSizeBytes)
			rsb += swag.Int64Value(ca.RequestedSizeBytes)
		}
		assert.Equal(asb, r.AllocatedCacheSizeBytes)
		assert.Equal(rsb, r.RequestedCacheSizeBytes)
		vmh.Queue.Add(r)
	}

	// test prepareStmt/Drain

	// version mismatch
	c.tableVersions = map[string]int{
		"VolumeMetadata": 0,
	}
	stmt, err := vmh.prepareStmt(ctx)
	assert.Error(err)
	assert.Regexp("unsupported.*version", err)
	assert.Nil(stmt)

	// prepare fails in Drain
	c.tableVersions = map[string]int{
		"VolumeMetadata": 1,
	}
	pg := &fpg.DB{}
	c.pgDB = pg
	c.db, err = pg.OpenDB(c.getDBArgs())
	assert.NoError(err)
	c.dbConnected = true
	pg.Mock.ExpectPrepare("VolumeMetadataInsert1(.*1.*2.*3.*4.*5.*6.*7.*8.*9.*10.*11.*12)").
		WillReturnError(fmt.Errorf("prepare-error"))
	err = vmh.Drain(ctx)
	assert.Error(err)
	assert.Regexp("prepare-error", err)
	assert.NoError(pg.Mock.ExpectationsWereMet())

	// Drain fails on exec, caches prepared statement
	pg.Mock.ExpectPrepare("VolumeMetadataInsert1(.*1.*2.*3.*4.*5.*6.*7.*8.*9.*10.*11.*12)").
		ExpectExec().WillReturnError(fmt.Errorf("exec-error"))
	assert.Nil(c.StmtCacheGet("VolumeMetadata"))
	err = vmh.Drain(ctx)
	assert.NoError(err)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.NotNil(c.StmtCacheGet("VolumeMetadata"))
	assert.Equal(0, vmh.Queue.Length())

	vmh.Beep(ctx)

	// Prepare ok, drain fails on connection error
	conErr := pgx.PgError{Code: "08000"}
	pg.Mock.ExpectExec(".*").WillReturnError(conErr)
	err = vmh.Drain(ctx)
	assert.Error(err)
	assert.Regexp("SQLSTATE 08000", err)
	assert.Equal(4, vmh.Queue.Length())

	// success
	for i := 0; i < 4; i++ {
		tl.Logger().Infof("XXXXX")
		r := vmh.Queue.PopHead().(*VolumeMetadataMetric)
		tl.Logger().Infof("YYYYYY")
		sqlRes := sqlmock.NewResult(1, 1)
		tl.Logger().Infof("ZZZZ")
		pg.Mock.ExpectExec(".*").
			WithArgs(r.Timestamp, r.VolumeSeriesID, r.AccountID, r.ServicePlanID,
				r.ClusterID, r.ConsistencyGroupID, r.ApplicationGroupIDs, r.TotalBytes, r.CostBytes, r.AllocatedBytes, r.RequestedCacheSizeBytes, r.AllocatedCacheSizeBytes).
			WillReturnResult(sqlRes)
		vmh.Queue.Add(r)
	}
	err = vmh.Drain(ctx)
	assert.NoError(err)
	assert.Equal(0, vmh.Queue.Length())
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.Equal(1, tl.CountPattern("Inserted 4 records"))

	// test buffer limit enforcement after failure
	tl.Flush()
	vm1 := &VolumeMetadataMetric{}
	vm2 := &VolumeMetadataMetric{}
	vm3 := &VolumeMetadataMetric{}
	vmh.Queue.Add([]*VolumeMetadataMetric{vm1, vm2, vm3})
	c.VolumeMetadataMaxBuffered = 0
	pg.Mock.ExpectExec(".*").WillReturnError(conErr)
	err = vmh.Drain(ctx)
	assert.Equal(0, vmh.Queue.Length())
	assert.Equal(1, tl.CountPattern("Dropping 3 buffered records"))
}

func TestVolumeMetadataWatcher(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	//ctx := context.Background()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log:    tl.Logger(),
			Server: &restapi.Server{},
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	c := &Component{}
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc

	vmh := &vsMetaHandler{}
	vmh.Init(c)

	// validate watcher patterns
	wa := vmh.getWatcherArgs()
	assert.NotNil(wa)
	assert.Len(wa.Matchers, 3)

	m := wa.Matchers[0]
	assert.NotEmpty(m.MethodPattern)
	tl.Logger().Debugf("0.MethodPattern: %s", m.MethodPattern)
	re := regexp.MustCompile(m.MethodPattern)
	assert.True(re.MatchString("POST"))
	assert.NotEmpty(m.URIPattern)
	tl.Logger().Debugf("0.URIPattern: %s", m.URIPattern)
	re = regexp.MustCompile(m.URIPattern)
	assert.True(re.MatchString("/volume-series"))
	assert.False(re.MatchString("/volume-series/"))
	assert.Empty(m.ScopePattern)

	m = wa.Matchers[1]
	assert.NotEmpty(m.MethodPattern)
	tl.Logger().Debugf("1.MethodPattern: %s", m.MethodPattern)
	re = regexp.MustCompile(m.MethodPattern)
	assert.True(re.MatchString("PATCH"))
	assert.NotEmpty(m.URIPattern)
	tl.Logger().Debugf("1.URIPattern: %s", m.URIPattern)
	re = regexp.MustCompile(m.URIPattern)
	assert.True(re.MatchString("/volume-series/80ceb62c-671d-4d07-9abd-3688808aa704?set=boundClusterId"))
	assert.True(re.MatchString("/volume-series/80ceb62c-671d-4d07-9abd-3688808aa704?set=cacheAllocations"))
	assert.True(re.MatchString("/volume-series/80ceb62c-671d-4d07-9abd-3688808aa704?set=capacityAllocations"))
	assert.True(re.MatchString("/volume-series/80ceb62c-671d-4d07-9abd-3688808aa704?set=consistencyGroupId"))
	assert.True(re.MatchString("/volume-series/80ceb62c-671d-4d07-9abd-3688808aa704?set=sizeBytes"))
	assert.True(re.MatchString("/volume-series/80ceb62c-671d-4d07-9abd-3688808aa704?set=spaAdditionalBytes"))
	assert.True(re.MatchString("/volume-series/80ceb62c-671d-4d07-9abd-3688808aa704?set=servicePlanId"))
	assert.False(re.MatchString("/volume-series/80ceb62c-671d-4d07-9abd-3688808aa704?set=name"))
	assert.False(re.MatchString("/consistency-groups/80ceb62c-671d-4d07-9abd-3688808aa704?set=applicationGroupIds"))
	assert.Empty(m.ScopePattern)

	m = wa.Matchers[2]
	assert.NotEmpty(m.MethodPattern)
	tl.Logger().Debugf("2.MethodPattern: %s", m.MethodPattern)
	re = regexp.MustCompile(m.MethodPattern)
	assert.True(re.MatchString("PATCH"))
	assert.NotEmpty(m.URIPattern)
	tl.Logger().Debugf("2.URIPattern: %s", m.URIPattern)
	re = regexp.MustCompile(m.URIPattern)
	assert.False(re.MatchString("/volume-series/80ceb62c-671d-4d07-9abd-3688808aa704?set=boundClusterId"))
	assert.True(re.MatchString("/consistency-groups/80ceb62c-671d-4d07-9abd-3688808aa704?set=applicationGroupIds"))
	assert.False(re.MatchString("/consistency-groups/80ceb62c-671d-4d07-9abd-3688808aa704?set=name"))
	assert.Empty(m.ScopePattern)

	// ignored event type
	now := time.Now()
	ce := &crude.CrudEvent{
		Timestamp:  now,
		Ordinal:    1,
		TrimmedURI: "/volume-series",
	}
	assert.Equal(0, vmh.Queue.Length())
	err := vmh.CrudeNotify(crude.WatcherQuitting, nil)
	assert.NoError(err)
	assert.Equal(0, vmh.Queue.Length())

	// invalid id error
	ce.Method = "POST"
	tl.Flush()
	assert.Equal(0, vmh.Queue.Length())
	err = vmh.CrudeNotify(crude.WatcherEvent, ce)
	assert.NoError(err)
	assert.Equal(0, vmh.Queue.Length())
	assert.Equal(1, tl.CountPattern("Invalid event"))

	// vs fetch error
	ce.Method = "POST"
	ce.Scope = crude.ScopeMap{crude.ScopeMetaID: "objectID"}
	tl.Flush()
	fc.RetVErr = fmt.Errorf("volume-series-fetch-error")
	assert.Equal(0, vmh.Queue.Length())
	err = vmh.CrudeNotify(crude.WatcherEvent, ce)
	assert.NoError(err)
	assert.Equal(0, vmh.Queue.Length())
	assert.Equal(1, tl.CountPattern("volume-series-fetch-error"))
	assert.Equal("objectID", fc.InVFetchID)

	// vs fetch succeeds, cg fetch error
	ce.Method = "PATCH"
	ce.TrimmedURI = "/volume-series/objectId?set=capacityAllocations"
	o := &models.VolumeSeries{}
	o.Meta = &models.ObjMeta{ID: "VS1"}
	o.AccountID = "ACCOUNT1"
	o.BoundClusterID = "CLUSTER1"
	o.ServicePlanID = "SERVICEPLAN1"
	o.ConsistencyGroupID = "CG1"
	o.CapacityAllocations = make(map[string]models.CapacityAllocation)
	o.CapacityAllocations["SP1"] = models.CapacityAllocation{
		ConsumedBytes: swag.Int64(1000),
	}
	o.CapacityAllocations["SP2"] = models.CapacityAllocation{
		ConsumedBytes: swag.Int64(1000),
	}
	o.SizeBytes = swag.Int64(10000)
	o.SpaAdditionalBytes = swag.Int64(1000)
	fc.RetVErr = nil
	fc.RetVObj = o
	fc.RetCGFetchErr = fmt.Errorf("cg-fetch-error")
	assert.Equal(0, vmh.Queue.Length())
	err = vmh.CrudeNotify(crude.WatcherEvent, ce)
	assert.NoError(err)
	assert.Equal(0, vmh.Queue.Length())
	assert.Equal(1, tl.CountPattern("cg-fetch-error"))

	// both succeed
	cg := &models.ConsistencyGroup{
		ConsistencyGroupAllOf0:  models.ConsistencyGroupAllOf0{Meta: &models.ObjMeta{ID: "CG1"}},
		ConsistencyGroupMutable: models.ConsistencyGroupMutable{ApplicationGroupIds: []models.ObjIDMutable{"AG1"}},
	}
	fc.RetCGFetchErr = nil
	fc.RetCGFetchObj = cg
	assert.Equal(0, vmh.Queue.Length())
	now = time.Now()
	err = vmh.CrudeNotify(crude.WatcherEvent, ce)
	assert.NoError(err)
	assert.Equal(1, vmh.Queue.Length())
	assert.Equal("objectId", fc.InVFetchID)
	vm := vmh.Queue.PeekHead().(*VolumeMetadataMetric)
	assert.True(vm.Timestamp.After(now))
	assert.True(vm.Timestamp.Before(time.Now()))
	expM := &VolumeMetadataMetric{
		Timestamp:           vm.Timestamp,
		VolumeSeriesID:      string(o.Meta.ID),
		AccountID:           string(o.AccountID),
		ClusterID:           string(o.BoundClusterID),
		ServicePlanID:       string(o.ServicePlanID),
		ConsistencyGroupID:  string(o.ConsistencyGroupID),
		ApplicationGroupIDs: "{AG1}",
		TotalBytes:          10000,
		CostBytes:           1000,
		AllocatedBytes:      swag.Int64Value(o.SizeBytes) + swag.Int64Value(o.SpaAdditionalBytes),
	}
	assert.Equal(expM, vm)

	// cg not found
	fc.RetCGFetchErr = fake.NotFoundErr
	fc.RetCGFetchObj = nil
	now = time.Now()
	err = vmh.CrudeNotify(crude.WatcherEvent, ce)
	assert.NoError(err)
	assert.Equal(2, vmh.Queue.Length())
	assert.Equal("objectId", fc.InVFetchID)
	vm = vmh.Queue.PopHead().(*VolumeMetadataMetric)
	vm = vmh.Queue.PeekHead().(*VolumeMetadataMetric)
	assert.True(vm.Timestamp.After(now))
	assert.True(vm.Timestamp.Before(time.Now()))
	expM = &VolumeMetadataMetric{
		Timestamp:           vm.Timestamp,
		VolumeSeriesID:      string(o.Meta.ID),
		AccountID:           string(o.AccountID),
		ClusterID:           string(o.BoundClusterID),
		ServicePlanID:       string(o.ServicePlanID),
		ConsistencyGroupID:  string(o.ConsistencyGroupID),
		ApplicationGroupIDs: "{}",
		TotalBytes:          10000,
		CostBytes:           1000,
		AllocatedBytes:      swag.Int64Value(o.SizeBytes) + swag.Int64Value(o.SpaAdditionalBytes),
	}
	assert.Equal(expM, vm)

	// check use of zero uuid if ids not set
	var o2 *models.VolumeSeries
	testutils.Clone(o, &o2)
	o2.BoundClusterID = ""
	o2.ConsistencyGroupID = ""
	fc.RetVObj = o2
	err = vmh.CrudeNotify(crude.WatcherEvent, ce)
	assert.NoError(err)
	assert.Equal(2, vmh.Queue.Length())
	vm = vmh.Queue.PopHead().(*VolumeMetadataMetric)
	vm = vmh.Queue.PeekHead().(*VolumeMetadataMetric)
	expM.ClusterID = nilUUID
	expM.ConsistencyGroupID = nilUUID
	expM.ApplicationGroupIDs = "{}"
	expM.Timestamp = vm.Timestamp
	assert.Equal(expM, vm)
	vm = vmh.Queue.PopHead().(*VolumeMetadataMetric)

	// cg fetch error
	ce.TrimmedURI = "/consistency-groups/objectId?set=applicationGroupIds"
	ce.Method = "PATCH"
	tl.Flush()
	fc.RetCGFetchErr = fmt.Errorf("cg-fetch-error")
	fc.RetVObj = nil
	assert.Equal(0, vmh.Queue.Length())
	err = vmh.CrudeNotify(crude.WatcherEvent, ce)
	assert.NoError(err)
	assert.Equal(0, vmh.Queue.Length())
	assert.Equal(1, tl.CountPattern("cg-fetch-error"))

	// cg fetch succeeds, vs list error
	fc.RetCGFetchErr = nil
	fc.RetCGFetchObj = cg
	fc.RetLsVErr = fmt.Errorf("vs-list-error")
	tl.Flush()
	assert.Equal(0, vmh.Queue.Length())
	err = vmh.CrudeNotify(crude.WatcherEvent, ce)
	assert.NoError(err)
	assert.Equal(0, vmh.Queue.Length())
	assert.Equal(1, tl.CountPattern("vs-list-error"))

	// cg fetch succeeds, vs list is empty (ok)
	fc.RetLsVErr = nil
	fc.RetLsVOk = &volume_series.VolumeSeriesListOK{}
	tl.Flush()
	assert.Equal(0, vmh.Queue.Length())
	nc := vmh.c.worker.GetNotifyCount()
	err = vmh.CrudeNotify(crude.WatcherEvent, ce)
	assert.NoError(err)
	assert.Equal(0, vmh.Queue.Length())
	assert.Equal(0, tl.CountPattern("."))
	assert.Equal(nc, vmh.c.worker.GetNotifyCount())

	// both succeed, non-empty list, one event per vs list element
	testutils.Clone(o, &o2)
	o2.Meta.ID = "VS2"
	o2.ServicePlanID = "SERVICEPLAN2"
	fc.RetLsVOk.Payload = []*models.VolumeSeries{o, o2}
	tl.Flush()
	assert.Equal(0, vmh.Queue.Length())
	now = time.Now()
	err = vmh.CrudeNotify(crude.WatcherEvent, ce)
	assert.NoError(err)
	assert.Equal(2, vmh.Queue.Length())
	vm = vmh.Queue.PopHead().(*VolumeMetadataMetric)
	assert.True(vm.Timestamp.After(now))
	assert.True(vm.Timestamp.Before(time.Now()))
	expM = &VolumeMetadataMetric{
		Timestamp:           vm.Timestamp,
		VolumeSeriesID:      string(o.Meta.ID),
		AccountID:           string(o.AccountID),
		ClusterID:           string(o.BoundClusterID),
		ServicePlanID:       string(o.ServicePlanID),
		ConsistencyGroupID:  string(o.ConsistencyGroupID),
		ApplicationGroupIDs: "{AG1}",
		TotalBytes:          10000,
		CostBytes:           1000,
		AllocatedBytes:      swag.Int64Value(o.SizeBytes) + swag.Int64Value(o.SpaAdditionalBytes),
	}
	assert.Equal(expM, vm)
	t1 := vm.Timestamp
	vm = vmh.Queue.PopHead().(*VolumeMetadataMetric)
	assert.Equal(t1, vm.Timestamp)
	expM = &VolumeMetadataMetric{
		Timestamp:           vm.Timestamp,
		VolumeSeriesID:      string(o2.Meta.ID),
		AccountID:           string(o2.AccountID),
		ClusterID:           string(o2.BoundClusterID),
		ServicePlanID:       string(o2.ServicePlanID),
		ConsistencyGroupID:  string(o2.ConsistencyGroupID),
		ApplicationGroupIDs: "{AG1}",
		TotalBytes:          10000,
		CostBytes:           1000,
		AllocatedBytes:      swag.Int64Value(o2.SizeBytes) + swag.Int64Value(o2.SpaAdditionalBytes),
	}
	assert.Equal(expM, vm)
	assert.Equal(nc+1, vmh.c.worker.GetNotifyCount())
}
