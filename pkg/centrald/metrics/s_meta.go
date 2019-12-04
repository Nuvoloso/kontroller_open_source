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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
)

// StorageMetadataMetric contains Storage Metadata metric data to be inserted into the database.
type StorageMetadataMetric struct {
	Timestamp      time.Time
	StorageID      string
	StorageType    string
	DomainID       string
	PoolID         string
	ClusterID      string
	TotalBytes     int64
	AvailableBytes int64
}

type sMetaHandler struct {
	c         *Component
	mux       sync.Mutex
	rt        *util.RoundingTicker
	watcherID string
	ctx       context.Context // for background operations
	cancelFn  context.CancelFunc
	queue     *util.Queue
}

// Init initializes the handler
func (smh *sMetaHandler) Init(c *Component) {
	smh.c = c
	smh.queue = util.NewQueue(&StorageMetadataMetric{})
	rta := &util.RoundingTickerArgs{
		Log:             c.Log,
		Period:          c.StorageMetadataMetricPeriod,
		RoundDown:       c.StorageMetadataMetricTruncate,
		CallImmediately: !c.StorageMetadataMetricSuppressStartup,
	}
	rt, _ := util.NewRoundingTicker(rta)
	smh.rt = rt
	wid, _ := smh.c.App.CrudeOps.Watch(smh.getWatcherArgs(), smh)
	smh.c.Log.Debugf("Watcher id: %s", wid)
	smh.watcherID = wid
}

// Start will enable the ticker
func (smh *sMetaHandler) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	smh.ctx = ctx
	smh.cancelFn = cancel
	smh.rt.Start(smh)
}

// Stop will disable the ticker and terminate receipt of CRUD events
func (smh *sMetaHandler) Stop() {
	smh.c.App.CrudeOps.TerminateWatcher(smh.watcherID)
	smh.watcherID = ""
	if smh.cancelFn != nil {
		smh.cancelFn()
		smh.cancelFn = nil
	}
	smh.rt.Stop()
}

// Beep satisfies the util.RoundingTickerBeeper interface
func (smh *sMetaHandler) Beep(ctx context.Context) {
	smh.GenerateStorageMetadataMetrics(ctx)
}

func (smh *sMetaHandler) prepareStmt(ctx context.Context) (*sql.Stmt, error) {
	var stmt *sql.Stmt
	var err error
	ver := smh.c.TableVersion("StorageMetadata")
	switch ver {
	case 1:
		stmt, err = smh.c.SafePrepare(ctx, "SELECT StorageMetadataInsert1($1, $2, $3, $4, $5, $6, $7, $8)")
	default:
		err = fmt.Errorf("unsupported StorageMetadata version %d", ver)
	}
	return stmt, err
}

// Drain writes all pending enqueued metric records
func (smh *sMetaHandler) Drain(ctx context.Context) error {
	var err error
	cnt := 0
	for el := smh.queue.PeekHead(); el != nil; el = smh.queue.PeekHead() {
		m := el.(*StorageMetadataMetric)
		stmt := smh.c.StmtCacheGet("StorageMetadata")
		if stmt == nil {
			stmt, err = smh.prepareStmt(ctx)
			if err != nil {
				break
			}
			smh.c.StmtCacheSet("StorageMetadata", stmt)
		}
		ver := smh.c.TableVersion("StorageMetadata")
		switch ver {
		case 1:
			_, err = stmt.ExecContext(ctx, m.Timestamp, m.StorageID, m.StorageType, m.DomainID,
				m.PoolID, m.ClusterID, m.TotalBytes, m.AvailableBytes)
		}
		if err != nil {
			err = fmt.Errorf("%s (%s)", err.Error(), smh.c.pgDB.SQLCode(err))
			smh.c.Log.Errorf("Failed to insert StorageMetadata: %s", err.Error())
			break
		}
		smh.queue.PopHead()
		cnt++
	}
	if cnt > 0 {
		smh.c.Log.Debugf("Inserted %d records", cnt)
	}
	smh.mux.Lock()
	defer smh.mux.Unlock()
	n := smh.queue.Length() - smh.c.StorageMetadataMaxBuffered
	if n > 0 {
		smh.c.Log.Warningf("Dropping %d buffered records", n)
		smh.queue.PopN(n)
	}
	return err
}

// objToMetric returns a StorageMetadataMetric type with data from a model object.
// It will return nil if the Storage object is not in a suitable state for metric collection.
func (smh *sMetaHandler) objToMetric(s *models.Storage) *StorageMetadataMetric {
	if s.StorageState == nil ||
		s.StorageState.AttachmentState != com.StgAttachmentStateAttached ||
		s.StorageState.DeviceState != com.StgDeviceStateOpen {
		return nil
	}
	return &StorageMetadataMetric{
		StorageID:      string(s.Meta.ID),
		StorageType:    string(s.CspStorageType),
		DomainID:       string(s.CspDomainID),
		PoolID:         string(s.PoolID),
		ClusterID:      string(s.ClusterID),
		TotalBytes:     swag.Int64Value(s.SizeBytes),
		AvailableBytes: swag.Int64Value(s.AvailableBytes),
	}
}

// GenerateStorageMetadataMetrics scans all Storage objects and generates statistics for them.
func (smh *sMetaHandler) GenerateStorageMetadataMetrics(ctx context.Context) error {
	lParams := storage.NewStorageListParams()
	lRes, err := smh.c.oCrud.StorageList(ctx, lParams)
	if err != nil {
		return err
	}
	cnt := 0
	now := time.Now()
	for _, s := range lRes.Payload {
		if m := smh.objToMetric(s); m != nil {
			m.Timestamp = now
			smh.queue.Add(m)
			cnt++
		}
	}
	if cnt > 0 {
		smh.c.Log.Debugf("Enqueued %d metric records [qlen=%d]", cnt, smh.queue.Length())
		smh.c.Notify() // awaken the main loop to write the records
	}
	return nil
}

func (smh *sMetaHandler) getWatcherArgs() *models.CrudWatcherCreateArgs {
	return &models.CrudWatcherCreateArgs{
		Name: "StorageMetrics",
		Matchers: []*models.CrudMatcher{
			// new Storage objects are not of interest because they have to be in use
			&models.CrudMatcher{ // relevant metadata changes
				MethodPattern: "PATCH",
				URIPattern:    "^/storage/.*(availableBytes|storageState)",
			},
		},
	}
}

// CrudeNotify handles CRUD events
func (smh *sMetaHandler) CrudeNotify(cbt crude.WatcherCallbackType, ce *crude.CrudEvent) error {
	if cbt != crude.WatcherEvent {
		return nil
	}
	id := ce.ID()
	if id != "" {
		if s, err := smh.c.oCrud.StorageFetch(smh.ctx, id); err == nil {
			if m := smh.objToMetric(s); m != nil {
				m.Timestamp = time.Now()
				smh.queue.Add(m)
				smh.c.Notify() // awaken the main loop to write the record
			}
		} else {
			smh.c.Log.Errorf("StorageFetch(%s): %s", id, err.Error())
		}
	} else {
		smh.c.Log.Debugf("Invalid event %d", ce.Ordinal)
	}
	return nil
}
