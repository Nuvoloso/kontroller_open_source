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

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan_allocation"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	fmm "github.com/Nuvoloso/kontroller/pkg/metricmover/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	fpg "github.com/Nuvoloso/kontroller/pkg/pgdb/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
	"github.com/jackc/pgx"
	"github.com/stretchr/testify/assert"
)

func TestSPAInit(t *testing.T) {
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
	c.SPAMetricSuppressStartup = true // for the UT

	spah := &spaHandler{}

	// validate that the watcher gets created as it is assumed to not fail in Init()
	wid, err := evM.Watch(spah.getWatcherArgs(), spah)
	assert.NoError(err)
	evM.TerminateWatcher(wid)

	spah.Init(c)
	assert.Equal(c, spah.c)
	assert.NotNil(spah.rt)
	assert.Equal(spah.rt.Period, c.SPAMetricPeriod)
	assert.Equal(spah.rt.RoundDown, c.SPAMetricTruncate)
	assert.Equal(spah.rt.CallImmediately, !c.SPAMetricSuppressStartup)
	assert.NotEmpty(spah.watcherID)

	spah.Start()
	assert.NotNil(spah.ctx)
	assert.NotNil(spah.cancelFn)
	err = spah.rt.Start(spah)
	assert.Error(err) // already started
	assert.Regexp("active", err)

	spah.Stop()
	assert.Nil(spah.cancelFn)
}

func TestSPAGenerateAndInsertMetrics(t *testing.T) {
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
	c.SPAMaxBuffered = 100
	fc := &fake.Client{}
	c.oCrud = fc

	spah := &spaHandler{}
	spah.Init(c)

	var err error

	// spa list error
	fc.RetSPAListErr = fmt.Errorf("service-plan-allocation-list-error")
	err = spah.GenerateSPAMetrics(ctx)
	assert.Error(err)
	assert.Regexp("service-plan-allocation-list-error", err)

	// spa list ok
	spaRes := &service_plan_allocation.ServicePlanAllocationListOK{
		Payload: []*models.ServicePlanAllocation{
			&models.ServicePlanAllocation{},
			&models.ServicePlanAllocation{},
			&models.ServicePlanAllocation{},
		},
	}
	o := spaRes.Payload[0]
	o.Meta = &models.ObjMeta{ID: "SPA1"}
	o.CspDomainID = "DOM1"
	o.ClusterID = "CL1"
	o.ServicePlanID = "SP1"
	o.AuthorizedAccountID = "AA1"
	o.TotalCapacityBytes = swag.Int64(1000)
	o.ReservableCapacityBytes = swag.Int64(1200)
	o = spaRes.Payload[1]
	o.Meta = &models.ObjMeta{ID: "SPA2"}
	o.CspDomainID = "DOM2"
	o.ClusterID = "CL2"
	o.ServicePlanID = "SP2"
	o.AuthorizedAccountID = "AA2"
	o.TotalCapacityBytes = swag.Int64(2000)
	o.ReservableCapacityBytes = swag.Int64(2200)
	o = spaRes.Payload[2]
	o.Meta = &models.ObjMeta{ID: "SP3"}
	o.CspDomainID = "DOM3"
	o.ClusterID = "CL3"
	o.ServicePlanID = "SP3"
	o.AuthorizedAccountID = "AA3"
	o.TotalCapacityBytes = swag.Int64(3000)
	o.ReservableCapacityBytes = swag.Int64(3200)

	fc.RetSPAListOK = spaRes
	fc.RetSPAListErr = nil
	assert.Equal(0, spah.Queue.Length())
	nc := c.worker.GetNotifyCount()
	tsB := time.Now()
	spah.ctx = ctx
	spah.Beep(ctx)
	tsA := time.Now()
	assert.Equal(nc+1, c.worker.GetNotifyCount())
	assert.Equal(3, spah.Queue.Length())
	for i := 0; i < 3; i++ {
		o = spaRes.Payload[i]
		r := spah.Queue.PopHead().(*SPAMetric)
		assert.True(tsB.Before(r.Timestamp) && tsA.After(r.Timestamp))
		assert.Equal(string(o.Meta.ID), r.ServicePlanAllocationID)
		assert.Equal(string(o.CspDomainID), r.CSPDomainID)
		assert.Equal(string(o.ClusterID), r.ClusterID)
		assert.Equal(string(o.ServicePlanID), r.ServicePlanID)
		assert.Equal(string(o.AuthorizedAccountID), r.AuthorizedAccountID)
		assert.Equal(swag.Int64Value(o.TotalCapacityBytes), r.TotalBytes)
		assert.Equal(swag.Int64Value(o.ReservableCapacityBytes), r.ReservableBytes)
		spah.Queue.Add(r)
	}

	// test prepareStmt/Drain

	// version mismatch
	c.tableVersions = map[string]int{
		"SPAMetrics": 0,
	}
	stmt, err := spah.prepareStmt(ctx)
	assert.Error(err)
	assert.Regexp("unsupported.*version", err)
	assert.Nil(stmt)

	// prepare fails in Drain
	c.tableVersions = map[string]int{
		"SPAMetrics": 1,
	}
	pg := &fpg.DB{}
	c.pgDB = pg
	c.db, err = pg.OpenDB(c.getDBArgs())
	assert.NoError(err)
	c.dbConnected = true
	pg.Mock.ExpectPrepare("SPAMetricsInsert1").WillReturnError(fmt.Errorf("prepare-error"))
	err = spah.Drain(ctx)
	assert.Error(err)
	assert.Regexp("prepare-error", err)
	assert.NoError(pg.Mock.ExpectationsWereMet())

	// Drain fails on exec, caches prepared statement
	pg.Mock.ExpectPrepare("SPAMetricsInsert1").
		ExpectExec().WillReturnError(fmt.Errorf("exec-error"))
	assert.Nil(c.StmtCacheGet("SPA"))
	err = spah.Drain(ctx)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.NotNil(c.StmtCacheGet("SPA"))
	assert.Equal(0, spah.Queue.Length())

	spah.Beep(ctx)

	// Prepare ok, drain fails on connection error
	conErr := pgx.PgError{Code: "08000"}
	pg.Mock.ExpectExec(".*").WillReturnError(conErr)
	err = spah.Drain(ctx)
	assert.Error(err)
	assert.Regexp("SQLSTATE 08000", err)
	assert.Equal(3, spah.Queue.Length())

	// success
	for i := 0; i < 3; i++ {
		r := spah.Queue.PopHead().(*SPAMetric)
		sqlRes := sqlmock.NewResult(1, 1)
		pg.Mock.ExpectExec(".*").
			WithArgs(r.Timestamp, r.ServicePlanAllocationID, r.CSPDomainID, r.ClusterID, r.ServicePlanID,
				r.AuthorizedAccountID, r.TotalBytes, r.ReservableBytes).
			WillReturnResult(sqlRes)
		spah.Queue.Add(r)
	}
	err = spah.Drain(ctx)
	assert.NoError(err)
	assert.Equal(0, spah.Queue.Length())
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.Equal(1, tl.CountPattern("Inserted 3 records"))

	// test buffer limit enforcement after failure
	tl.Flush()
	spam1 := &SPAMetric{}
	spam2 := &SPAMetric{}
	spam3 := &SPAMetric{}
	spah.Queue.Add([]*SPAMetric{spam1, spam2, spam3})
	c.SPAMaxBuffered = 0
	pg.Mock.ExpectExec(".*").WillReturnError(conErr)
	err = spah.Drain(ctx)
	assert.Equal(0, spah.Queue.Length())
	assert.Equal(1, tl.CountPattern("Dropping 3 buffered records"))
}

func TestSPAWatcher(t *testing.T) {
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

	spah := &spaHandler{}
	spah.Init(c)

	// validate watcher patterns
	wa := spah.getWatcherArgs()
	assert.NotNil(wa)
	assert.Len(wa.Matchers, 1)

	m := wa.Matchers[0]
	assert.NotEmpty(m.MethodPattern)
	tl.Logger().Debugf("MethodPattern: %s", m.MethodPattern)
	re := regexp.MustCompile(m.MethodPattern)
	assert.True(re.MatchString("PATCH"))
	assert.NotEmpty(m.URIPattern)
	tl.Logger().Debugf("URIPattern: %s", m.URIPattern)
	re = regexp.MustCompile(m.URIPattern)
	assert.True(re.MatchString("/service-plan-allocations/80ceb62c-671d-4d07-9abd-3688808aa704?set=totalCapacityBytes"))
	assert.True(re.MatchString("/service-plan-allocations/80ceb62c-671d-4d07-9abd-3688808aa704?set=reservableCapacityBytes"))
	assert.False(re.MatchString("/service-plan-allocations/80ceb62c-671d-4d07-9abd-3688808aa704?set=poolState "))
	assert.Empty(m.ScopePattern)

	now := time.Now()
	ce := &crude.CrudEvent{
		Timestamp:  now,
		Ordinal:    1,
		Method:     "PATCH",
		TrimmedURI: "/service-plan-allocations/objectId?set=totalCapacityBytes",
		Scope: map[string]string{
			"totalCapacityBytes":      "1000",
			"reservableCapacityBytes": "800",
			"cspDomainID":             "cspDomainId",
			"clusterID":               "clusterID",
			"servicePlanID":           "servicePlanID",
			"authorizedAccountID":     "authorizedAccountID",
		},
	}
	assert.Equal(0, spah.Queue.Length())
	err := spah.CrudeNotify(crude.WatcherQuitting, nil) // ignore this type
	assert.NoError(err)

	assert.Equal(0, spah.Queue.Length())
	err = spah.CrudeNotify(crude.WatcherEvent, ce)
	assert.NoError(err)
	assert.Equal(1, spah.Queue.Length())
	sm := spah.Queue.PeekHead().(*SPAMetric)
	assert.Equal(now, sm.Timestamp)
	assert.Equal("objectId", sm.ServicePlanAllocationID)
	assert.Equal("cspDomainId", sm.CSPDomainID)
	assert.Equal("clusterID", sm.ClusterID)
	assert.Equal("servicePlanID", sm.ServicePlanID)
	assert.Equal("authorizedAccountID", sm.AuthorizedAccountID)
	assert.Equal(int64(1000), sm.TotalBytes)
	assert.Equal(int64(800), sm.ReservableBytes)

	err = spah.CrudeNotify(crude.WatcherEvent, ce)
	assert.NoError(err)
	assert.Equal(2, spah.Queue.Length())
	sm = spah.Queue.PopHead().(*SPAMetric)
	sm = spah.Queue.PeekHead().(*SPAMetric)
	assert.Equal(now, sm.Timestamp)
	assert.Equal("objectId", sm.ServicePlanAllocationID)
	assert.Equal("cspDomainId", sm.CSPDomainID)
	assert.Equal("clusterID", sm.ClusterID)
	assert.Equal("servicePlanID", sm.ServicePlanID)
	assert.Equal("authorizedAccountID", sm.AuthorizedAccountID)
	assert.Equal(int64(1000), sm.TotalBytes)
	assert.Equal(int64(800), sm.ReservableBytes)

	// invalid cases
	spah.Queue = util.NewQueue(&SPAMetric{})
	assert.Equal(spah.Queue.Length(), 0)
	for _, tc := range []string{"tcb", "rcb", "csp", "cl", "sp", "aa", "cspS", "cS", "spS", "aaS"} {
		var ev crude.CrudEvent
		testutils.Clone(ce, &ev)
		switch tc {
		case "tcb":
			delete(ev.Scope, "totalCapacityBytes")
		case "acb":
			delete(ev.Scope, "availableCapacityBytes")
		case "rcb":
			delete(ev.Scope, "reservableCapacityBytes")
		case "csp":
			delete(ev.Scope, "cspDomainID")
		case "cl":
			delete(ev.Scope, "clusterID")
		case "sp":
			delete(ev.Scope, "servicePlanID")
		case "aa":
			delete(ev.Scope, "authorizedAccountID")
		case "cspS":
			ev.Scope["cspDomainID"] = ""
		case "cS":
			ev.Scope["clusterID"] = ""
		case "spS":
			ev.Scope["servicePlanID"] = ""
		case "aaS":
			ev.Scope["authorizedAccountID"] = ""
		default:
			assert.False(true)
		}
		err = spah.CrudeNotify(crude.WatcherEvent, &ev)
		assert.NoError(err)
		assert.Equal(0, spah.Queue.Length())
	}
}
