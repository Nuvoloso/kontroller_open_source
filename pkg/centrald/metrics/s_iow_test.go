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
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	fmm "github.com/Nuvoloso/kontroller/pkg/metricmover/fake"
	fpg "github.com/Nuvoloso/kontroller/pkg/pgdb/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/jackc/pgx"
	"github.com/stretchr/testify/assert"
)

func TestSIOWBuzz(t *testing.T) {
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
	c := Component{}
	c.Init(app)
	assert.NotNil(c.pgDB)
	pg := &fpg.DB{}
	c.pgDB = pg
	var err error
	c.db, err = pg.OpenDB(c.getDBArgs())
	assert.NoError(err)
	c.dbConnected = true
	c.StorageIOMaxBuffered = StorageIOMaxBufferedDefault

	w := &c.sIOW
	assert.NotNil(w.c)

	// Assemble a sample (adapted from TestVsIOPCompliance)
	now := time.Now()
	iom, err := makeSampleIoMetricStatisticsDatum(now)
	assert.NoError(err)
	sd := &sIOMSample{
		IoMetricStatisticsDatum: *iom,
		ViolationLatencyMean:    1,
		ViolationLatencyMax:     12,
		ViolationWorkloadRate:   300,
	}
	w.EnqueueSample(sd)
	assert.Equal(1, w.Queue.Length())

	// invalid version
	assert.Empty(c.tableVersions)
	err = w.Buzz(ctx)
	assert.Error(err)
	assert.Regexp("unsupported StorageMetrics version", err)
	assert.Equal(1, w.Queue.Length())

	// prepare error
	c.tableVersions = map[string]int{"StorageMetrics": 1}
	assert.Equal(1, c.TableVersion("StorageMetrics"))
	assert.Nil(c.StmtCacheGet("StorageMetrics"))
	pg.Mock.ExpectPrepare("StorageMetricsInsert1(.*1.*2.*3.*4.*5.*6.*7.*8.*9.*10.*11.*12)").
		WillReturnError(fmt.Errorf("prepare-error"))
	err = w.Buzz(ctx)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.Error(err)
	assert.Regexp("prepare-error", err)
	assert.Equal(1, w.Queue.Length())
	assert.Nil(c.StmtCacheGet("StorageMetrics"))

	// prepare ok, exec fails (connection error)
	connErr := pgx.PgError{Code: "08000"}
	pg.Mock.ExpectPrepare("StorageMetricsInsert1(.*1.*2.*3.*4.*5.*6.*7.*8.*9.*10.*11.*12)").
		ExpectExec().WillReturnError(connErr)
	err = w.Buzz(ctx)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.Error(err)
	assert.Regexp("SQLSTATE 08000", err)
	assert.Equal(1, w.Queue.Length())
	assert.NotNil(c.StmtCacheGet("StorageMetrics"))

	// cached statement, exec fails (non-connection error)
	pg.Mock.ExpectExec(".*").WillReturnError(fmt.Errorf("exec-error"))
	err = w.Buzz(ctx)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.NoError(err)
	assert.Equal(0, w.Queue.Length())
	assert.NotNil(c.StmtCacheGet("StorageMetrics"))

	// success
	w.EnqueueSample(sd)
	sqlRes := sqlmock.NewResult(1, 1)
	pg.Mock.ExpectExec("StorageMetricsInsert1(.*1.*2.*3.*4.*5.*6.*7.*8.*9.*10.*11.*12)").
		WithArgs(sd.Timestamp, sd.ObjectID,
			sd.ReadNumBytes, sd.WriteNumBytes, sd.ReadNumOps, sd.WriteNumOps,
			sd.LatencyMeanUsec, sd.LatencyMaxUsec, sd.SampleDurationSec,
			sd.ViolationLatencyMean, sd.ViolationLatencyMax, sd.ViolationWorkloadRate).
		WillReturnResult(sqlRes)
	err = w.Buzz(ctx)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.NoError(err)
	assert.Equal(0, w.Queue.Length())

	// test buffer limit enforcement after connection error
	tl.Flush()
	sd1 := &sIOMSample{}
	sd2 := &sIOMSample{}
	sd3 := &sIOMSample{}
	w.Queue.Add([]*sIOMSample{sd1, sd2, sd3})
	c.StorageIOMaxBuffered = 0
	pg.Mock.ExpectExec(".*").WillReturnError(connErr)
	err = w.Buzz(ctx)
	assert.NoError(pg.Mock.ExpectationsWereMet())
	assert.Error(err)
	assert.Regexp("SQLSTATE 08000", err)
	assert.Equal(0, w.Queue.Length())
	assert.Equal(1, tl.CountPattern("Dropped 3 .* inactive .* writer"))
}
