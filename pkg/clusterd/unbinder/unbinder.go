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


package unbinder

import (
	"context"
	"fmt"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
	"github.com/op/go-logging"
)

// unbinder constants
const (
	UnbinderWorkerInterval        = 5 * time.Minute
	PersistentVolmeWatcherTimeout = 9 * time.Minute
	ListRefreshInterval           = 10 * time.Minute
	FastInterval                  = 5 * time.Second
)

// Comp is used to manage this handler component
type Comp struct {
	App           *clusterd.AppCtx
	Log           *logging.Logger
	runCount      int
	worker        util.Worker
	watcherID     string
	oCrud         crud.Ops
	clusterClient cluster.Client
	pvWatcher     cluster.PVWatcher
	clusterID     string
	unBindQueue   *util.Queue
	lastList      time.Time
}

func newUnbinderComp() *Comp {
	uc := &Comp{}
	return uc
}

func init() {
	clusterd.AppRegisterComponent(newUnbinderComp())
}

// Init registers handlers for this component
func (c *Comp) Init(app *clusterd.AppCtx) {
	c.App = app
	c.Log = app.Log
	wa := &util.WorkerArgs{
		Name:          "unbinder",
		Log:           c.Log,
		SleepInterval: FastInterval,
	}
	c.worker, _ = util.NewWorker(wa, c)
}

// Start starts this component
func (c *Comp) Start() {
	c.Log.Info("Starting Unbinder")
	c.oCrud = c.App.OCrud
	c.clusterClient = c.App.ClusterClient
	c.runCount = 0
	c.unBindQueue = util.NewQueue("")
	c.worker.Start()
}

// Stop stops this component
func (c *Comp) Stop() {
	c.worker.Stop()
	if c.pvWatcher != nil {
		c.pvWatcher.Stop()
	}
	c.Log.Info("Stopped Unbinder")
}

// Buzz satisfies the util.WorkerBee interface
func (c *Comp) Buzz(ctx context.Context) error {
	var err error
	c.runCount++
	if !c.App.IsReady() {
		return nil
	}
	if c.clusterID == "" {
		if c.App.AppObjects == nil {
			return nil
		}
		cObj := c.App.AppObjects.GetCluster()
		if cObj == nil {
			return nil
		}
		c.clusterID = string(cObj.Meta.ID)
	}
	if c.pvWatcher != nil && !c.pvWatcher.IsActive() {
		c.pvWatcher = nil
		c.worker.SetSleepInterval(FastInterval)
		c.Log.Debug("inactive watcher, resetting")
	}
	var pvList []*cluster.PersistentVolumeObj
	if c.pvWatcher == nil {
		c.Log.Debug("creating watcher")
		args := &cluster.PVWatcherArgs{
			Timeout: PersistentVolmeWatcherTimeout,
			Ops:     c,
		}
		if c.pvWatcher, pvList, err = c.clusterClient.CreatePublishedPersistentVolumeWatcher(ctx, args); err != nil {
			return fmt.Errorf("create pv watcher: %s", err.Error())
		}
		c.worker.SetSleepInterval(UnbinderWorkerInterval)
	}
	if pvList == nil && c.lastList.Add(ListRefreshInterval).Before(time.Now()) {
		pvList, err = c.clusterClient.PublishedPersistentVolumeList(ctx)
	}
	if err == nil && pvList != nil {
		c.lastList = time.Now()
		if err = c.processList(ctx, pvList); err != nil {
			return err
		}
	}
	c.doUnbind(ctx)
	if c.pvWatcher != nil && !c.pvWatcher.IsActive() {
		c.Log.Debug("start watcher")
		if err = c.pvWatcher.Start(ctx); err != nil {
			return err
		}
	}
	return err
}

func (c *Comp) doUnbind(ctx context.Context) {
	for vsID := c.unBindQueue.PeekHead(); vsID != nil; vsID = c.unBindQueue.PeekHead() {
		volumeSeriesID := vsID.(string)
		if err := c.App.DeleteOrUnbindVolume(ctx, volumeSeriesID, "STATIC"); err != nil {
			c.Log.Warningf("volume (%s) delete/unbind failed:%s", volumeSeriesID, err.Error())
		}
		c.unBindQueue.PopHead()
	}
	return
}

func (c *Comp) processList(ctx context.Context, pvList []*cluster.PersistentVolumeObj) error {
	lParams := &volume_series.VolumeSeriesListParams{
		BoundClusterID:    swag.String(c.clusterID),
		VolumeSeriesState: []string{com.VolStateBound, com.VolStateProvisioned},
		SystemTags:        []string{com.SystemTagVolumePublished},
	}
	vsListRes, err := c.oCrud.VolumeSeriesList(ctx, lParams)
	if err != nil {
		return err
	}
	for _, vs := range vsListRes.Payload {
		found := false
		for _, pv := range pvList {
			if pv.VolumeSeriesID == string(vs.Meta.ID) {
				found = true
				break
			}
		}
		if !found {
			c.unBindQueue.Add(string(vs.Meta.ID))
		}
	}
	return nil
}

// PVDeleted is a call back that adds a volume to the unbind queue when its PV is deleted
func (c *Comp) PVDeleted(pv *cluster.PersistentVolumeObj) error {
	c.Log.Infof("persistent volume (%s) deleted", pv.VolumeSeriesID)
	c.unBindQueue.Add(pv.VolumeSeriesID)
	c.worker.Notify()
	return nil
}
