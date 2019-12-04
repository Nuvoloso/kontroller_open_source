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
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/consistency_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/pgdb"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
)

// VolumeMetadataMetric contains VolumeSeries Metadata metric data to be inserted into the database.
type VolumeMetadataMetric struct {
	Timestamp               time.Time
	VolumeSeriesID          string
	AccountID               string
	ServicePlanID           string
	ClusterID               string
	ConsistencyGroupID      string
	ApplicationGroupIDs     string // string representation of ARRAY[uuids...]
	TotalBytes              int64
	CostBytes               int64
	AllocatedBytes          int64
	RequestedCacheSizeBytes int64
	AllocatedCacheSizeBytes int64
}

type vsMetaHandler struct {
	c         *Component
	mux       sync.Mutex
	rt        *util.RoundingTicker
	watcherID string
	Queue     *util.Queue
	ctx       context.Context // for background operations
	cancelFn  context.CancelFunc
}

func (vmh *vsMetaHandler) Init(c *Component) {
	vmh.c = c
	rta := &util.RoundingTickerArgs{
		Log:             c.Log,
		Period:          c.VolumeMetadataMetricPeriod,
		RoundDown:       c.VolumeMetadataMetricTruncate,
		CallImmediately: !c.VolumeMetadataMetricSuppressStartup,
	}
	rt, _ := util.NewRoundingTicker(rta)
	vmh.rt = rt
	wid, _ := vmh.c.App.CrudeOps.Watch(vmh.getWatcherArgs(), vmh)
	vmh.c.Log.Debugf("Watcher id: %s", wid)
	vmh.watcherID = wid
	vmh.Queue = util.NewQueue(&VolumeMetadataMetric{})
}

func (vmh *vsMetaHandler) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	vmh.ctx = ctx
	vmh.cancelFn = cancel
	vmh.rt.Start(vmh)
}

func (vmh *vsMetaHandler) Stop() {
	vmh.c.App.CrudeOps.TerminateWatcher(vmh.watcherID)
	vmh.watcherID = ""
	if vmh.cancelFn != nil {
		vmh.cancelFn()
		vmh.cancelFn = nil
	}
	vmh.rt.Stop()
}

// Beep satisfies the util.RoundingTickerBeeper interface
func (vmh *vsMetaHandler) Beep(ctx context.Context) {
	vmh.GenerateVolumeMetadataMetrics(ctx)
}

func (vmh *vsMetaHandler) prepareStmt(ctx context.Context) (*sql.Stmt, error) {
	var stmt *sql.Stmt
	var err error
	ver := vmh.c.TableVersion("VolumeMetadata")
	switch ver {
	case 1:
		stmt, err = vmh.c.SafePrepare(ctx, "SELECT VolumeMetadataInsert1($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)")
	default:
		err = fmt.Errorf("unsupported VolumeMetadata version %d", ver)
	}
	return stmt, err
}

// Drain writes all pending enqueued metric records
func (vmh *vsMetaHandler) Drain(ctx context.Context) error {
	var err error
	cnt := 0
	for el := vmh.Queue.PeekHead(); el != nil; el = vmh.Queue.PeekHead() {
		m := el.(*VolumeMetadataMetric)
		stmt := vmh.c.StmtCacheGet("VolumeMetadata")
		if stmt == nil {
			stmt, err = vmh.prepareStmt(ctx)
			if err != nil {
				break
			}
			vmh.c.StmtCacheSet("VolumeMetadata", stmt)
		}
		ver := vmh.c.TableVersion("VolumeMetadata")
		switch ver {
		case 1:
			_, err = stmt.ExecContext(ctx, m.Timestamp, m.VolumeSeriesID, m.AccountID, m.ServicePlanID,
				m.ClusterID, m.ConsistencyGroupID, m.ApplicationGroupIDs, m.TotalBytes, m.CostBytes, m.AllocatedBytes, m.RequestedCacheSizeBytes, m.AllocatedCacheSizeBytes)
		}
		if err == nil {
			cnt++
		} else {
			if vmh.c.pgDB.ErrDesc(err) == pgdb.ErrConnectionError {
				break
			}
			vmh.c.Log.Errorf("Failed to insert VolumeMetadata: %s", err.Error())
			err = nil
		}
		vmh.Queue.PopHead()
	}
	if cnt > 0 {
		vmh.c.Log.Debugf("Inserted %d records", cnt)
	}
	n := vmh.Queue.Length() - vmh.c.VolumeMetadataMaxBuffered
	if n > 0 {
		vmh.c.Log.Warningf("Dropping %d buffered records", n)
		vmh.Queue.PopN(n)
	}
	return err
}

// objToMetric returns a VolumeMetadataMetric type with data from the given related model objects.
func (vmh *vsMetaHandler) objToMetric(vs *models.VolumeSeries, cg *models.ConsistencyGroup) *VolumeMetadataMetric {
	costBytes := swag.Int64Value(vs.SpaAdditionalBytes)
	allocatedBytes := costBytes + swag.Int64Value(vs.SizeBytes)
	allocatedCacheSizeBytes := int64(0)
	requestedCacheSizeBytes := int64(0)
	for _, ca := range vs.CacheAllocations {
		allocatedCacheSizeBytes += swag.Int64Value(ca.AllocatedSizeBytes)
		requestedCacheSizeBytes += swag.Int64Value(ca.RequestedSizeBytes)
	}
	var b bytes.Buffer
	b.WriteRune('{')
	if cg != nil {
		for i, ag := range cg.ApplicationGroupIds {
			if i != 0 {
				b.WriteRune(',')
			}
			b.WriteString(string(ag))
		}
	}
	b.WriteRune('}')
	return &VolumeMetadataMetric{
		VolumeSeriesID:          string(vs.Meta.ID),
		AccountID:               string(vs.AccountID),
		ServicePlanID:           string(vs.ServicePlanID),
		ClusterID:               idOrZeroUUID(string(vs.BoundClusterID)),
		ConsistencyGroupID:      idOrZeroUUID(string(vs.ConsistencyGroupID)),
		ApplicationGroupIDs:     b.String(),
		TotalBytes:              swag.Int64Value(vs.SizeBytes),
		CostBytes:               costBytes,
		AllocatedBytes:          allocatedBytes,
		RequestedCacheSizeBytes: requestedCacheSizeBytes,
		AllocatedCacheSizeBytes: allocatedCacheSizeBytes,
	}
}

// GenerateVolumeMetadataMetrics scans all VolumeSeries objects and generates statistics for them.
func (vmh *vsMetaHandler) GenerateVolumeMetadataMetrics(ctx context.Context) error {
	lRes, err := vmh.c.oCrud.VolumeSeriesList(ctx, volume_series.NewVolumeSeriesListParams())
	if err != nil {
		return err
	}
	cgRes, err := vmh.c.oCrud.ConsistencyGroupList(ctx, consistency_group.NewConsistencyGroupListParams())
	if err != nil {
		return err
	}
	cgMap := map[models.ObjIDMutable]*models.ConsistencyGroup{}
	for _, cg := range cgRes.Payload {
		cgMap[models.ObjIDMutable(cg.Meta.ID)] = cg
	}
	now := time.Now()
	for _, vs := range lRes.Payload {
		m := vmh.objToMetric(vs, cgMap[vs.ConsistencyGroupID])
		m.Timestamp = now
		vmh.Queue.Add(m)
	}
	if len(lRes.Payload) > 0 {
		vmh.c.Log.Debugf("Enqueued %d metric records [qLen=%d]", len(lRes.Payload), vmh.Queue.Length())
		vmh.c.Notify() // awaken the main loop to write the records
	}
	return nil
}

func (vmh *vsMetaHandler) getWatcherArgs() *models.CrudWatcherCreateArgs {
	return &models.CrudWatcherCreateArgs{
		Name: "VolumeMetrics",
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{ // new VolumeSeries objects
				MethodPattern: "POST",
				URIPattern:    "^/volume-series$",
			},
			&models.CrudMatcher{ // relevant metadata changes
				MethodPattern: "PATCH",
				URIPattern:    "^/volume-series/.*(boundClusterId|cacheAllocations|capacityAllocations|consistencyGroupId|sizeBytes|spaAdditionalBytes|servicePlanId)",
			},
			// only PATCH requires watching. On creation, no VolumeSeries will be in the CG
			&models.CrudMatcher{ // relevant metadata changes
				MethodPattern: "PATCH",
				URIPattern:    "^/consistency-groups/.*applicationGroupIds",
			},
		},
	}
}

func (vmh *vsMetaHandler) CrudeNotify(cbt crude.WatcherCallbackType, ce *crude.CrudEvent) error {
	if cbt != crude.WatcherEvent {
		return nil
	}
	id := ce.ID()
	if id != "" && strings.HasPrefix(ce.TrimmedURI, "/volume-series") {
		if vs, err := vmh.c.oCrud.VolumeSeriesFetch(vmh.ctx, id); err == nil {
			var cg *models.ConsistencyGroup
			if vs.ConsistencyGroupID != "" {
				if cg, err = vmh.c.oCrud.ConsistencyGroupFetch(vmh.ctx, string(vs.ConsistencyGroupID)); err != nil {
					if e, ok := err.(*crud.Error); !ok || !e.NotFound() {
						vmh.c.Log.Errorf("ConsistencyGroupFetch(%s): %s", vs.ConsistencyGroupID, err.Error())
					} else {
						// without a transaction, slightly possible for CG to be gone
						vmh.c.Log.Debugf("ConsistencyGroupFetch(%s): %s", vs.ConsistencyGroupID, err.Error())
						err = nil
					}
				}
			}
			if err == nil {
				m := vmh.objToMetric(vs, cg)
				m.Timestamp = time.Now()
				vmh.Queue.Add(m)
				vmh.c.Notify() // awaken the main loop to write the record
			}
		} else {
			vmh.c.Log.Errorf("VolumeSeriesFetch(%s): %s", id, err.Error())
		}
	} else if id != "" && strings.HasPrefix(ce.TrimmedURI, "/consistency-groups/") {
		if cg, err := vmh.c.oCrud.ConsistencyGroupFetch(vmh.ctx, id); err == nil {
			params := volume_series.NewVolumeSeriesListParams().WithConsistencyGroupID(&id)
			if res, err := vmh.c.oCrud.VolumeSeriesList(vmh.ctx, params); err == nil {
				if len(res.Payload) > 0 {
					t := time.Now()
					for _, vs := range res.Payload {
						m := vmh.objToMetric(vs, cg)
						m.Timestamp = t
						vmh.Queue.Add(m)
					}
					vmh.c.Notify() // awaken the main loop to write the records
				}
			} else {
				vmh.c.Log.Errorf("VolumeSeriesList(cg=%s): %s", id, err.Error())
			}
		} else {
			vmh.c.Log.Errorf("ConsistencyGroupFetch(%s): %s", id, err.Error())
		}
	} else {
		vmh.c.Log.Debugf("Invalid event %d", ce.Ordinal)
	}
	return nil
}
