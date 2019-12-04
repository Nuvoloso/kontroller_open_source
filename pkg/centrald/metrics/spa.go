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
	"strconv"
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan_allocation"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/Nuvoloso/kontroller/pkg/pgdb"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
)

// SPAMetric contains spa metric data to be inserted into the database.
type SPAMetric struct {
	Timestamp               time.Time
	ServicePlanAllocationID string
	CSPDomainID             string
	ClusterID               string
	ServicePlanID           string
	AuthorizedAccountID     string
	TotalBytes              int64
	ReservableBytes         int64
}

// spaHandler manages spa metric processing
type spaHandler struct {
	c         *Component
	mux       sync.Mutex
	rt        *util.RoundingTicker
	watcherID string
	Queue     *util.Queue
	ctx       context.Context // for background operations
	cancelFn  context.CancelFunc
}

func (spah *spaHandler) Init(c *Component) {
	spah.c = c
	rta := &util.RoundingTickerArgs{
		Log:             c.Log,
		Period:          c.SPAMetricPeriod,
		RoundDown:       c.SPAMetricTruncate,
		CallImmediately: !c.SPAMetricSuppressStartup,
	}
	rt, _ := util.NewRoundingTicker(rta)
	spah.rt = rt
	wid, _ := spah.c.App.CrudeOps.Watch(spah.getWatcherArgs(), spah)
	spah.c.Log.Debugf("Watcher id: %s", wid)
	spah.watcherID = wid
	spah.Queue = util.NewQueue(&SPAMetric{})
}

func (spah *spaHandler) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	spah.ctx = ctx
	spah.cancelFn = cancel
	spah.rt.Start(spah)
}

func (spah *spaHandler) Stop() {
	spah.c.App.CrudeOps.TerminateWatcher(spah.watcherID)
	spah.watcherID = ""
	if spah.cancelFn != nil {
		spah.cancelFn()
		spah.cancelFn = nil
	}
	spah.rt.Stop()
}

// Beep satisfies the util.RoundingTickerBeeper interface
func (spah *spaHandler) Beep(ctx context.Context) {
	spah.GenerateSPAMetrics(ctx)
}

func (spah *spaHandler) prepareStmt(ctx context.Context) (*sql.Stmt, error) {
	var stmt *sql.Stmt
	var err error
	ver := spah.c.TableVersion("SPAMetrics")
	switch ver {
	case 1:
		stmt, err = spah.c.SafePrepare(ctx, "SELECT SPAMetricsInsert1($1, $2, $3, $4, $5, $6, $7, $8)")
	default:
		err = fmt.Errorf("unsupported SPAMetrics version %d", ver)
	}
	return stmt, err
}

// Drain writes all pending enqueued metric records
func (spah *spaHandler) Drain(ctx context.Context) error {
	var err error
	cnt := 0
	for el := spah.Queue.PeekHead(); el != nil; el = spah.Queue.PeekHead() {
		m := el.(*SPAMetric)
		stmt := spah.c.StmtCacheGet("SPA")
		if stmt == nil {
			stmt, err = spah.prepareStmt(ctx)
			if err != nil {
				break
			}
			spah.c.StmtCacheSet("SPA", stmt)
		}
		ver := spah.c.TableVersion("SPAMetrics")
		switch ver {
		case 1:
			_, err = stmt.ExecContext(ctx, m.Timestamp, m.ServicePlanAllocationID, m.CSPDomainID,
				m.ClusterID, m.ServicePlanID, m.AuthorizedAccountID, m.TotalBytes, m.ReservableBytes)
		}
		if err == nil {
			cnt++
		} else {
			if spah.c.pgDB.ErrDesc(err) == pgdb.ErrConnectionError {
				break
			}
			spah.c.Log.Errorf("Failed to insert SPAMetrics: %s", err.Error())
			err = nil
		}
		spah.Queue.PopHead()
	}
	if cnt > 0 {
		spah.c.Log.Debugf("Inserted %d records", cnt)
	}
	n := spah.Queue.Length() - spah.c.SPAMaxBuffered
	if n > 0 {
		spah.c.Log.Warningf("Dropping %d buffered records", n)
		spah.Queue.PopN(n)
	}
	return err
}

// GenerateSPAMetrics scans all ServicePlanAllocation objects and generates statistics for them.
func (spah *spaHandler) GenerateSPAMetrics(ctx context.Context) error {
	lParams := service_plan_allocation.NewServicePlanAllocationListParams()
	lRes, err := spah.c.oCrud.ServicePlanAllocationList(ctx, lParams)
	if err != nil {
		return err
	}
	now := time.Now()
	spah.mux.Lock()
	for _, s := range lRes.Payload {
		m := &SPAMetric{
			Timestamp:               now,
			ServicePlanAllocationID: string(s.Meta.ID),
			CSPDomainID:             string(s.CspDomainID),
			ClusterID:               string(s.ClusterID),
			ServicePlanID:           string(s.ServicePlanID),
			AuthorizedAccountID:     string(s.AuthorizedAccountID),
			ReservableBytes:         swag.Int64Value(s.ReservableCapacityBytes),
			TotalBytes:              swag.Int64Value(s.TotalCapacityBytes),
		}
		spah.Queue.Add(m)
	}
	spah.mux.Unlock()
	if len(lRes.Payload) > 0 {
		spah.c.Log.Debugf("Enqueued %d metric records [qlen=%d]", len(lRes.Payload), spah.Queue.Length())
		spah.c.Notify() // awaken the main loop to write the records
	}
	return nil
}

func (spah *spaHandler) getWatcherArgs() *models.CrudWatcherCreateArgs {
	return &models.CrudWatcherCreateArgs{
		Name: "SPAMetrics",
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{
				MethodPattern: "PATCH",
				URIPattern:    "^/service-plan-allocations/.*(totalCapacityBytes|reservableCapacityBytes)",
			},
		},
	}
}

func (spah *spaHandler) CrudeNotify(cbt crude.WatcherCallbackType, ce *crude.CrudEvent) error {
	if cbt != crude.WatcherEvent {
		return nil
	}
	tcbS, tcbOk := ce.Scope["totalCapacityBytes"]
	rcbS, rcbOk := ce.Scope["reservableCapacityBytes"]
	cspS, cspOk := ce.Scope["cspDomainID"]
	cS, cOk := ce.Scope["clusterID"]
	spS, spOk := ce.Scope["servicePlanID"]
	aaS, aaOk := ce.Scope["authorizedAccountID"]
	id := ce.ID()
	if tcbOk && rcbOk && cspOk && cspS != "" && cOk && cS != "" && spOk && spS != "" && aaOk && aaS != "" {
		tcb, _ := strconv.ParseInt(tcbS, 10, 64)
		rcb, _ := strconv.ParseInt(rcbS, 10, 64)
		m := &SPAMetric{
			Timestamp:               ce.Timestamp,
			ServicePlanAllocationID: id,
			CSPDomainID:             cspS,
			ClusterID:               cS,
			ServicePlanID:           spS,
			AuthorizedAccountID:     aaS,
			TotalBytes:              tcb,
			ReservableBytes:         rcb,
		}
		spah.mux.Lock()
		spah.Queue.Add(m)
		spah.mux.Unlock()
		spah.c.Notify() // awaken the main loop to write the record
	} else {
		spah.c.Log.Debugf("Invalid event %d", ce.Ordinal)
	}
	return nil
}
