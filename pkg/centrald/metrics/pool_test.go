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
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/pool"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	fmm "github.com/Nuvoloso/kontroller/pkg/metricmover/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	fpg "github.com/Nuvoloso/kontroller/pkg/pgdb/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/jackc/pgx"
	"github.com/stretchr/testify/assert"
)

func TestPoolInit(t *testing.T) {
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
	c.PoolMetricSuppressStartup = true // for the UT

	ph := &poolHandler{}

	// validate that the watcher gets created as it is assumed to not fail in Init()
	wid, err := evM.Watch(ph.getWatcherArgs(), ph)
	assert.NoError(err)
	evM.TerminateWatcher(wid)

	ph.Init(c)
	assert.Equal(c, ph.c)
	assert.NotNil(ph.rt)
	assert.Equal(ph.rt.Period, c.PoolMetricPeriod)
	assert.Equal(ph.rt.RoundDown, c.PoolMetricTruncate)
	assert.Equal(ph.rt.CallImmediately, !c.PoolMetricSuppressStartup)
	assert.NotEmpty(ph.watcherID)

	ph.Start()
	assert.NotNil(ph.ctx)
	assert.NotNil(ph.cancelFn)
	err = ph.rt.Start(ph)
	assert.Error(err) // already started
	assert.Regexp("active", err)

	ph.Stop()
	assert.Nil(ph.cancelFn)
}

func TestPoolGenerateAndInsertMetrics(t *testing.T) {
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
	c.PoolMaxBuffered = 100
	fc := &fake.Client{}
	c.oCrud = fc

	ph := &poolHandler{}
	ph.Init(c)

	var err error

	// pool list error
	fc.RetPoolListErr = fmt.Errorf("pool-list-error")
	err = ph.GeneratePoolMetrics(ctx)
	assert.Error(err)
	assert.Regexp("pool-list-error", err)

	// pool list ok
	spRes := &pool.PoolListOK{
		Payload: []*models.Pool{
			&models.Pool{},
			&models.Pool{},
			&models.Pool{},
		},
	}
	o := spRes.Payload[0]
	o.Meta = &models.ObjMeta{ID: "SP1"}
	o.CspDomainID = "DOM1"
	o = spRes.Payload[1]
	o.Meta = &models.ObjMeta{ID: "SP2"}
	o.CspDomainID = "DOM2"
	o = spRes.Payload[2]
	o.Meta = &models.ObjMeta{ID: "SP3"}
	o.CspDomainID = "DOM3"

	fc.RetPoolListObj = spRes
	fc.RetPoolListErr = nil
	assert.Equal(0, ph.Queue.Length())
	nc := c.worker.GetNotifyCount()
	tsB := time.Now()
	ph.ctx = ctx
	ph.Beep(ctx)
	tsA := time.Now()
	assert.Equal(nc+1, c.worker.GetNotifyCount())
	assert.Equal(3, ph.Queue.Length())
	for i := 0; i < 3; i++ {
		o = spRes.Payload[i]
		r := ph.Queue.PopHead().(*PoolMetric)
		assert.True(tsB.Before(r.Timestamp) && tsA.After(r.Timestamp))
		assert.Equal(string(o.Meta.ID), r.PoolID)
		assert.Equal(string(o.CspDomainID), r.CSPDomainID)
		ph.Queue.Add(r)
	}

	// test prepareStmt/Drain

	// version mismatch
	c.tableVersions = map[string]int{
		"PoolMetrics": 0,
	}
	stmt, err := ph.prepareStmt(ctx)
	assert.Error(err)
	assert.Regexp("unsupported.*version", err)
	assert.Nil(stmt)

	// prepare fails in Drain
	c.tableVersions = map[string]int{
		"PoolMetrics": 1,
	}
	pg := &fpg.DB{}
	c.pgDB = pg
	c.db, err = pg.OpenDB(c.getDBArgs())
	assert.NoError(err)
	c.dbConnected = true
	pg.Mock.ExpectPrepare("PoolMetricsInsert1").WillReturnError(fmt.Errorf("prepare-error"))
	err = ph.Drain(ctx)
	assert.Error(err)
	assert.Regexp("prepare-error", err)
	assert.NoError(pg.Mock.ExpectationsWereMet())

	// Drain fails on exec, caches prepared statement
	pg.Mock.ExpectPrepare("PoolMetricsInsert1").
		ExpectExec().WillReturnError(fmt.Errorf("exec-error"))
	assert.Nil(c.StmtCacheGet("Pool"))
	err = ph.Drain(ctx)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.NotNil(c.StmtCacheGet("Pool"))
	assert.Equal(0, ph.Queue.Length())

	ph.Beep(ctx)

	// Prepare ok, drain fails on connection error
	conErr := pgx.PgError{Code: "08000"}
	pg.Mock.ExpectExec(".*").WillReturnError(conErr)
	err = ph.Drain(ctx)
	assert.Error(err)
	assert.Regexp("SQLSTATE 08000", err)
	assert.Equal(3, ph.Queue.Length())

	// success
	for i := 0; i < 3; i++ {
		r := ph.Queue.PopHead().(*PoolMetric)
		sqlRes := sqlmock.NewResult(1, 1)
		pg.Mock.ExpectExec(".*").
			WithArgs(r.Timestamp, r.PoolID, r.CSPDomainID,
				r.TotalBytes, r.AvailableBytes, r.ReservableBytes).
			WillReturnResult(sqlRes)
		ph.Queue.Add(r)
	}
	err = ph.Drain(ctx)
	assert.NoError(err)
	assert.Equal(0, ph.Queue.Length())
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.Equal(1, tl.CountPattern("Inserted 3 records"))

	// test buffer limit enforcement after failure
	tl.Flush()
	pm1 := &PoolMetric{}
	pm2 := &PoolMetric{}
	pm3 := &PoolMetric{}
	ph.Queue.Add([]*PoolMetric{pm1, pm2, pm3})
	c.PoolMaxBuffered = 0
	pg.Mock.ExpectExec(".*").WillReturnError(conErr)
	err = ph.Drain(ctx)
	assert.Equal(0, ph.Queue.Length())
	assert.Equal(1, tl.CountPattern("Dropping 3 buffered records"))
}

func TestPoolWatcher(t *testing.T) {
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

	ph := &poolHandler{}
	ph.Init(c)

	// validate watcher patterns
	wa := ph.getWatcherArgs()
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
	assert.True(re.MatchString("/pools"))
	assert.False(re.MatchString("/pools/"))
	assert.Empty(m.ScopePattern)

	m = wa.Matchers[1]
	assert.NotEmpty(m.MethodPattern)
	tl.Logger().Debugf("2.MethodPattern: %s", m.MethodPattern)
	re = regexp.MustCompile(m.MethodPattern)
	assert.True(re.MatchString("PATCH"))
	assert.NotEmpty(m.URIPattern)
	tl.Logger().Debugf("2.URIPattern: %s", m.URIPattern)
	re = regexp.MustCompile(m.URIPattern)
	assert.True(re.MatchString("/pools/80ceb62c-671d-4d07-9abd-3688808aa704?set=servicePlanReservations"))
	assert.Empty(m.ScopePattern)

	// TBD: to be changed
	now := time.Now()
	ce := &crude.CrudEvent{
		Timestamp:  now,
		Ordinal:    1,
		Method:     "PATCH",
		TrimmedURI: "/pools/objectId?set=totalCapacityBytes",
		Scope: map[string]string{
			"cspDomainID": "cspDomainId",
		},
	}
	assert.Equal(0, ph.Queue.Length())
	err := ph.CrudeNotify(crude.WatcherQuitting, nil) // ignore this type
	assert.NoError(err)

	assert.Equal(0, ph.Queue.Length())
	err = ph.CrudeNotify(crude.WatcherEvent, ce)
	assert.NoError(err)
	// assert.Equal(1, ph.Queue.Length())
	// pm := ph.Queue.PeekHead().(*PoolMetric)
	// assert.Equal(now, pm.Timestamp)
	// assert.Equal("objectId", pm.PoolID)
	// assert.Equal("cspDomainId", pm.CSPDomainID)

	// ce.Method = "POST"
	// ce.TrimmedURI = "/pools"
	// ce.Scope[crude.ScopeMetaID] = "objectId"
	// err = ph.CrudeNotify(crude.WatcherEvent, ce)
	// assert.NoError(err)
	// assert.Equal(2, ph.Queue.Length())
	// pm = ph.Queue.PopHead().(*PoolMetric)
	// pm = ph.Queue.PeekHead().(*PoolMetric)
	// //pm = ph.queue[1]
	// assert.Equal(now, pm.Timestamp)
	// assert.Equal("objectId", pm.PoolID)
	// assert.Equal("cspDomainId", pm.CSPDomainID)

	// // invalid cases
	// ph.Queue = util.NewQueue(&PoolMetric{})
	// assert.Equal(ph.Queue.Length(), 0)
	// for _, tc := range []string{"csp", "id", "cspS"} {
	// 	var ev crude.CrudEvent
	// 	testutils.Clone(ce, &ev)
	// 	switch tc {
	// 	case "csp":
	// 		delete(ev.Scope, "cspDomainID")
	// 	case "id":
	// 		delete(ev.Scope, crude.ScopeMetaID)
	// 		ev.Method = "POST"
	// 	case "cspS":
	// 		ev.Scope["cspDomainID"] = ""
	// 	default:
	// 		assert.False(true)
	// 	}
	// 	err = ph.CrudeNotify(crude.WatcherEvent, &ev)
	// 	assert.NoError(err)
	// 	assert.Equal(0, ph.Queue.Length())
	// }
}
