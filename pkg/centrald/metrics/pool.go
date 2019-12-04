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
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/pool"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/Nuvoloso/kontroller/pkg/pgdb"
	"github.com/Nuvoloso/kontroller/pkg/util"
)

// PoolMetric contains pool metric data to be inserted into the database.
type PoolMetric struct {
	Timestamp       time.Time
	PoolID          string
	CSPDomainID     string
	TotalBytes      int64 // To be changed
	AvailableBytes  int64 // To be changed
	ReservableBytes int64 // To be changed
}

// poolHandler manages pool metric processing
type poolHandler struct {
	c         *Component
	mux       sync.Mutex
	rt        *util.RoundingTicker
	watcherID string
	Queue     *util.Queue
	ctx       context.Context // for background operations
	cancelFn  context.CancelFunc
}

func (ph *poolHandler) Init(c *Component) {
	ph.c = c
	rta := &util.RoundingTickerArgs{
		Log:             c.Log,
		Period:          c.PoolMetricPeriod,
		RoundDown:       c.PoolMetricTruncate,
		CallImmediately: !c.PoolMetricSuppressStartup,
	}
	rt, _ := util.NewRoundingTicker(rta)
	ph.rt = rt
	wid, _ := ph.c.App.CrudeOps.Watch(ph.getWatcherArgs(), ph)
	ph.c.Log.Debugf("Watcher id: %s", wid)
	ph.watcherID = wid
	ph.Queue = util.NewQueue(&PoolMetric{})
}

func (ph *poolHandler) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	ph.ctx = ctx
	ph.cancelFn = cancel
	ph.rt.Start(ph)
}

func (ph *poolHandler) Stop() {
	ph.c.App.CrudeOps.TerminateWatcher(ph.watcherID)
	ph.watcherID = ""
	if ph.cancelFn != nil {
		ph.cancelFn()
		ph.cancelFn = nil
	}
	ph.rt.Stop()
}

// Beep satisfies the util.RoundingTickerBeeper interface
func (ph *poolHandler) Beep(ctx context.Context) {
	ph.GeneratePoolMetrics(ctx)
}

func (ph *poolHandler) prepareStmt(ctx context.Context) (*sql.Stmt, error) {
	var stmt *sql.Stmt
	var err error
	ver := ph.c.TableVersion("PoolMetrics")
	switch ver {
	case 1:
		stmt, err = ph.c.SafePrepare(ctx, "SELECT PoolMetricsInsert1($1, $2, $3, $4, $5, $6)")
	default:
		err = fmt.Errorf("unsupported PoolMetrics version %d", ver)
	}
	return stmt, err
}

// Drain writes all pending enqueued metric records
func (ph *poolHandler) Drain(ctx context.Context) error {
	var err error
	cnt := 0
	for el := ph.Queue.PeekHead(); el != nil; el = ph.Queue.PeekHead() {
		m := el.(*PoolMetric)
		stmt := ph.c.StmtCacheGet("Pool")
		if stmt == nil {
			stmt, err = ph.prepareStmt(ctx)
			if err != nil {
				break
			}
			ph.c.StmtCacheSet("Pool", stmt)
		}
		ver := ph.c.TableVersion("PoolMetrics")
		switch ver {
		case 1:
			_, err = stmt.ExecContext(ctx, m.Timestamp, m.PoolID,
				m.CSPDomainID, m.TotalBytes, m.AvailableBytes, m.ReservableBytes)
		}
		if err == nil {
			cnt++
		} else {
			if ph.c.pgDB.ErrDesc(err) == pgdb.ErrConnectionError {
				break
			}
			ph.c.Log.Errorf("Failed to insert PoolMetrics: %s", err.Error())
			err = nil
		}
		ph.Queue.PopHead()
	}
	if cnt > 0 {
		ph.c.Log.Debugf("Inserted %d records", cnt)
	}
	n := ph.Queue.Length() - ph.c.PoolMaxBuffered
	if n > 0 {
		ph.c.Log.Warningf("Dropping %d buffered records", n)
		ph.Queue.PopN(n)
	}
	return err
}

// GeneratePoolMetrics scans all Pool objects and generates statistics for them.
func (ph *poolHandler) GeneratePoolMetrics(ctx context.Context) error {
	lParams := pool.NewPoolListParams()
	lRes, err := ph.c.oCrud.PoolList(ctx, lParams)
	if err != nil {
		return err
	}
	now := time.Now()
	ph.mux.Lock()
	for _, p := range lRes.Payload {
		m := &PoolMetric{
			Timestamp:   now,
			PoolID:      string(p.Meta.ID),
			CSPDomainID: string(p.CspDomainID),
		}
		ph.Queue.Add(m)
	}
	ph.mux.Unlock()
	if len(lRes.Payload) > 0 {
		ph.c.Log.Debugf("Enqueued %d metric records [qLen=%d]", len(lRes.Payload), ph.Queue.Length())
		ph.c.Notify() // awaken the main loop to write the records
	}
	return nil
}

func (ph *poolHandler) getWatcherArgs() *models.CrudWatcherCreateArgs {
	return &models.CrudWatcherCreateArgs{
		Name: "PoolMetrics",
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{ // new Pool objects
				MethodPattern: "POST",
				URIPattern:    "^/pools$",
			},
			&models.CrudMatcher{ // capacity changes
				MethodPattern: "PATCH",
				URIPattern:    "^/pools/.*(servicePlanReservations)",
			},
		},
	}
}

func (ph *poolHandler) CrudeNotify(cbt crude.WatcherCallbackType, ce *crude.CrudEvent) error {
	if cbt != crude.WatcherEvent {
		return nil
	}
	// TBD - generate the metric
	// cspS, cspOk := ce.Scope["cspDomainID"]
	// id := ce.ID()
	// if id != "" && cspOk && cspS != "" {
	// 	m := &PoolMetric{
	// 		Timestamp:       ce.Timestamp,
	// 		PoolID:          id,
	// 		CSPDomainID:     cspS,
	// 	}
	// 	ph.mux.Lock()
	// 	ph.Queue.Add(m)
	// 	ph.mux.Unlock()
	// 	ph.c.Notify() // awaken the main loop to write the record
	// } else {
	// 	ph.c.Log.Debugf("Invalid event %d", ce.Ordinal)
	// }
	return nil
}
