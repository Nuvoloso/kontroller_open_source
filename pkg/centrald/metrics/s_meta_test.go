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
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage"
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
	"github.com/stretchr/testify/assert"
)

func TestStorageMetadataInit(t *testing.T) {
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
	c.StorageMetadataMetricSuppressStartup = true // for the UT

	smh := &sMetaHandler{}

	// validate that the watcher gets created as it is assumed to not fail in Init()
	wid, err := evM.Watch(smh.getWatcherArgs(), smh)
	assert.NoError(err)
	evM.TerminateWatcher(wid)

	smh.Init(c)
	assert.Equal(c, smh.c)
	assert.NotNil(smh.rt)
	assert.Equal(smh.rt.Period, c.StorageMetadataMetricPeriod)
	assert.Equal(smh.rt.RoundDown, c.StorageMetadataMetricTruncate)
	assert.Equal(smh.rt.CallImmediately, !c.StorageMetadataMetricSuppressStartup)
	assert.NotEmpty(smh.watcherID)

	smh.Start()
	assert.NotNil(smh.ctx)
	assert.NotNil(smh.cancelFn)
	err = smh.rt.Start(smh)
	assert.Error(err) // already started
	assert.Regexp("active", err)

	smh.Stop()
	assert.Nil(smh.cancelFn)
}

func TestStorageMetadataGenerateAndInsertMetrics(t *testing.T) {
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
	c.StorageMetadataMaxBuffered = 100
	fc := &fake.Client{}
	c.oCrud = fc

	smh := &sMetaHandler{}
	smh.Init(c)

	var err error

	// list error
	fc.RetLsSErr = fmt.Errorf("storage-list-error")
	err = smh.GenerateStorageMetadataMetrics(ctx)
	assert.Error(err)
	assert.Regexp("storage-list-error", err)

	// list ok
	o1 := &models.Storage{}
	o1.Meta = &models.ObjMeta{ID: "S1"}
	o1.SizeBytes = swag.Int64(10000)
	o1.AvailableBytes = swag.Int64(9999)
	o1.CspStorageType = "Amazon gp2"
	o1.CspDomainID = "DOM-1"
	o1.PoolID = "SP-1"
	o1.ClusterID = "clusterID"
	o1.StorageState = &models.StorageStateMutable{
		AttachmentState: "ATTACHED",
		DeviceState:     "OPEN",
	}
	o2 := &models.Storage{}
	testutils.Clone(o1, o2)
	o2.Meta.ID = "S2"
	o2.StorageState = nil // ignored
	o3 := &models.Storage{}
	testutils.Clone(o1, o3)
	o3.Meta.ID = "S3"
	o3.StorageState.DeviceState = "OPENING" // ignored
	o4 := &models.Storage{}
	testutils.Clone(o1, o4)
	o4.Meta.ID = "S4"
	o4.StorageState.AttachmentState = "UNATTACHED" // ignored
	sRes := &storage.StorageListOK{
		Payload: []*models.Storage{
			o2, o3, o1, o4,
		},
	}

	fc.RetLsSOk = sRes
	fc.RetLsSErr = nil
	assert.Equal(0, smh.queue.Length())
	nc := c.worker.GetNotifyCount()
	tsB := time.Now()
	smh.ctx = ctx
	smh.Beep(ctx)
	tsA := time.Now()
	assert.Equal(nc+1, c.worker.GetNotifyCount())
	assert.Equal(1, smh.queue.Length()) // only one valid object
	r := smh.queue.PeekHead().(*StorageMetadataMetric)
	assert.EqualValues(o1.Meta.ID, r.StorageID)
	assert.EqualValues(o1.CspStorageType, r.StorageType)
	assert.EqualValues(o1.CspDomainID, r.DomainID)
	assert.EqualValues(o1.PoolID, r.PoolID)
	assert.EqualValues(o1.ClusterID, r.ClusterID)
	assert.EqualValues(10000, r.TotalBytes)
	assert.EqualValues(9999, r.AvailableBytes)
	assert.True(tsB.Before(r.Timestamp) && tsA.After(r.Timestamp))

	// test prepareStmt/Drain

	// version mismatch
	c.tableVersions = map[string]int{
		"StorageMetadata": 0,
	}
	stmt, err := smh.prepareStmt(ctx)
	assert.Error(err)
	assert.Regexp("unsupported.*version", err)
	assert.Nil(stmt)

	// prepare fails in Drain
	c.tableVersions = map[string]int{
		"StorageMetadata": 1,
	}
	pg := &fpg.DB{}
	c.pgDB = pg
	c.db, err = pg.OpenDB(c.getDBArgs())
	assert.NoError(err)
	c.dbConnected = true
	pg.Mock.ExpectPrepare("StorageMetadataInsert1").WillReturnError(fmt.Errorf("prepare-error"))
	err = smh.Drain(ctx)
	assert.Error(err)
	assert.Regexp("prepare-error", err)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.Equal(1, smh.queue.Length())

	// Drain fails on exec, caches prepared statement
	pg.Mock.ExpectPrepare("StorageMetadataInsert1").
		ExpectExec().WillReturnError(fmt.Errorf("exec-error"))
	assert.Nil(c.StmtCacheGet("StorageMetadata"))
	err = smh.Drain(ctx)
	assert.Error(err)
	assert.Regexp("exec-error", err)
	assert.NotNil(c.StmtCacheGet("StorageMetadata"))
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.Equal(1, smh.queue.Length())

	// success
	sqlRes := sqlmock.NewResult(1, 1)
	pg.Mock.ExpectExec(".*").
		WithArgs(r.Timestamp, r.StorageID, r.StorageType, r.DomainID,
			r.PoolID, r.ClusterID, r.TotalBytes, r.AvailableBytes).
		WillReturnResult(sqlRes)
	err = smh.Drain(ctx)
	assert.NoError(err)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.Equal(0, smh.queue.Length())

	// test buffer limit enforcement after failure
	tl.Flush()
	m1 := &StorageMetadataMetric{}
	m2 := &StorageMetadataMetric{}
	m3 := &StorageMetadataMetric{}
	smh.queue.Add([]*StorageMetadataMetric{m1, m2, m3})
	c.StorageMetadataMaxBuffered = 0
	pg.Mock.ExpectExec(".*").WillReturnError(fmt.Errorf("exec-error"))
	err = smh.Drain(ctx)
	assert.Error(err)
	assert.Regexp("exec-error", err)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.Equal(0, smh.queue.Length())
	assert.Equal(1, tl.CountPattern("Dropping 3 buffered records"))
}

func TestStorageMetadataWatcher(t *testing.T) {
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

	smh := &sMetaHandler{}
	smh.Init(c)

	// validate watcher patterns
	wa := smh.getWatcherArgs()
	assert.NotNil(wa)
	assert.Len(wa.Matchers, 1)

	m := wa.Matchers[0]
	assert.NotEmpty(m.MethodPattern)
	tl.Logger().Debugf("1.MethodPattern: %s", m.MethodPattern)
	re := regexp.MustCompile(m.MethodPattern)
	assert.True(re.MatchString("PATCH"))
	assert.NotEmpty(m.URIPattern)
	tl.Logger().Debugf("1.URIPattern: %s", m.URIPattern)
	re = regexp.MustCompile(m.URIPattern)
	assert.True(re.MatchString("/storage/80ceb62c-671d-4d07-9abd-3688808aa704?set=availableBytes"))
	assert.True(re.MatchString("/storage/80ceb62c-671d-4d07-9abd-3688808aa704?set=storageState"))
	assert.False(re.MatchString("/storage/80ceb62c-671d-4d07-9abd-3688808aa704?set=totalParcelCount"))
	assert.Empty(m.ScopePattern)

	// ignored event type
	now := time.Now()
	ce := &crude.CrudEvent{
		Timestamp:  now,
		Ordinal:    1,
		TrimmedURI: "/storage/objectId?set=availableBytes",
	}
	assert.Equal(0, smh.queue.Length())
	err := smh.CrudeNotify(crude.WatcherQuitting, nil)
	assert.NoError(err)
	assert.Equal(0, smh.queue.Length())

	// invalid id error
	ce.Method = "POST"
	tl.Flush()
	assert.Equal(0, smh.queue.Length())
	err = smh.CrudeNotify(crude.WatcherEvent, ce)
	assert.NoError(err)
	assert.Equal(0, smh.queue.Length())
	assert.Equal(1, tl.CountPattern("Invalid event"))

	// fetch error
	ce.Method = "PATCH"
	tl.Flush()
	fc.StorageFetchRetErr = fmt.Errorf("storage-fetch-error")
	assert.Equal(0, smh.queue.Length())
	err = smh.CrudeNotify(crude.WatcherEvent, ce)
	assert.NoError(err)
	assert.Equal(0, smh.queue.Length())
	assert.Equal(1, tl.CountPattern("storage-fetch-error"))

	// fetch succeeds
	o := &models.Storage{}
	o.Meta = &models.ObjMeta{ID: "S1"}
	o.SizeBytes = swag.Int64(10000)
	o.AvailableBytes = swag.Int64(9999)
	o.CspStorageType = "Amazon gp2"
	o.CspDomainID = "DOM-1"
	o.PoolID = "SP-1"
	o.ClusterID = "CLUSTER-1"
	o.StorageState = &models.StorageStateMutable{
		AttachmentState: "ATTACHED",
		DeviceState:     "OPEN",
	}
	fc.StorageFetchRetErr = nil
	fc.StorageFetchRetObj = o
	assert.Equal(0, smh.queue.Length())
	now = time.Now()
	err = smh.CrudeNotify(crude.WatcherEvent, ce)
	assert.NoError(err)
	assert.Equal(1, smh.queue.Length())
	assert.Equal("objectId", fc.StorageFetchID)
	sm := smh.queue.PeekHead().(*StorageMetadataMetric)
	assert.True(sm.Timestamp.After(now))
	assert.True(sm.Timestamp.Before(time.Now()))
	expM := &StorageMetadataMetric{
		Timestamp:      sm.Timestamp,
		StorageID:      string(o.Meta.ID),
		StorageType:    string(o.CspStorageType),
		DomainID:       string(o.CspDomainID),
		PoolID:         string(o.PoolID),
		ClusterID:      string(o.ClusterID),
		TotalBytes:     10000,
		AvailableBytes: 9999,
	}
	assert.Equal(expM, sm)
}
