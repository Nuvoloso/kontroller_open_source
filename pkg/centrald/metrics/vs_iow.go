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
	"database/sql"
	"fmt"

	"github.com/Nuvoloso/kontroller/pkg/pgdb"
	"github.com/Nuvoloso/kontroller/pkg/util"
)

// vsIOWriter writes VolumeSeries I/O metrics to the database
type vsIOWriter struct {
	c       *Component
	InError bool // indicates that the stage is not dequeuing
	Queue   *util.Queue
	worker  util.Worker
}

func (w *vsIOWriter) Init(c *Component) {
	w.c = c
	wa := &util.WorkerArgs{
		Name:          "vsIOWriter",
		Log:           w.c.Log,
		SleepInterval: w.c.RetryInterval,
	}
	w.worker, _ = util.NewWorker(wa, w)
	w.Queue = util.NewQueue(&vsIOMSample{})
}

func (w *vsIOWriter) Start() {
	w.c.Log.Info("Starting vsIO Metric Writer")
	w.worker.Start()
}

func (w *vsIOWriter) Stop() {
	w.worker.Stop()
	w.c.Log.Info("Stopped vsIO Metric Writer")
}

func (w *vsIOWriter) EnqueueSample(sd *vsIOMSample) {
	w.Queue.Add(sd)
	w.worker.Notify()
}

// Buzz satisfies the util.WorkerBee interface
func (w *vsIOWriter) Buzz(ctx context.Context) error {
	var err error
	cnt := 0
	w.InError = false
	for el := w.Queue.PeekHead(); el != nil; el = w.Queue.PeekHead() {
		sd := el.(*vsIOMSample)
		stmt := w.c.StmtCacheGet("VolumeMetrics")
		if stmt == nil {
			stmt, err = w.prepareStmt(ctx)
			if err != nil {
				break
			}
			w.c.StmtCacheSet("VolumeMetrics", stmt)
		}
		ver := w.c.TableVersion("VolumeMetrics")
		switch ver {
		case 1:
			_, err = stmt.ExecContext(ctx, sd.Timestamp, sd.ObjectID,
				sd.ReadNumBytes, sd.WriteNumBytes, sd.ReadNumOps, sd.WriteNumOps,
				sd.LatencyMeanUsec, sd.LatencyMaxUsec, sd.SampleDurationSec,
				sd.ViolationLatencyMean, sd.ViolationLatencyMax, sd.ViolationWorkloadRate,
				sd.ViolationWorkloadMixRead, sd.ViolationWorkloadMixWrite,
				sd.ViolationWorkloadAvgSizeMin, sd.ViolationWorkloadAvgSizeMax,
				sd.NumCacheReadUserHits, sd.NumCacheReadUserTotal, sd.NumCacheReadMetaHits, sd.NumCacheReadMetaTotal)
		}
		if err == nil {
			cnt++
		} else {
			if w.c.pgDB.ErrDesc(err) == pgdb.ErrConnectionError {
				break
			}
			// This can happen if metrics refer to expired metadata
			// Not considered a queue limiting error
			w.c.Log.Errorf("Failed to insert VolumeMetrics: %s", err.Error())
			err = nil
		}
		w.Queue.PopHead()
	}
	if cnt > 0 {
		w.c.Log.Debugf("Inserted %d records", cnt)
	}
	if err != nil {
		w.InError = true
		w.c.vsIoMLimitBuffering()
	}
	return err
}

func (w *vsIOWriter) prepareStmt(ctx context.Context) (*sql.Stmt, error) {
	var stmt *sql.Stmt
	var err error
	ver := w.c.TableVersion("VolumeMetrics")
	switch ver {
	case 1:
		stmt, err = w.c.SafePrepare(ctx, "SELECT VolumeMetricsInsert1($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)")
	default:
		err = fmt.Errorf("unsupported VolumeMetrics version %d", ver)
	}
	return stmt, err
}
