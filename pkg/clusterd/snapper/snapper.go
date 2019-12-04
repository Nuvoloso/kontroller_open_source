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


package snapper

import (
	"context"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	logging "github.com/op/go-logging"
)

// Snapper constants
const (
	// The default time between periodic runs
	SnapperStartSleepIntervalDefault = 5 * time.Second
	SloRPODefault                    = time.Duration(4 * time.Hour)
	MaxRPO                           = time.Duration(10000 * time.Hour)
)

// Comp is used to manage this handler component
type Comp struct {
	App               *clusterd.AppCtx
	Log               *logging.Logger
	sleepPeriod       time.Duration
	clusterID         models.ObjID
	runCount          int
	oCrud             crud.Ops
	worker            util.Worker
	timeoutMultiplier float64
	watcherID         string
	notifyCount       int
}

func newSnapperComp() *Comp {
	sc := &Comp{}
	return sc
}

func init() {
	clusterd.AppRegisterComponent(newSnapperComp())
}

// Init registers handlers for this component
func (c *Comp) Init(app *clusterd.AppCtx) {
	c.App = app
	c.Log = app.Log
	c.sleepPeriod = SnapperStartSleepIntervalDefault
	c.timeoutMultiplier = c.App.CGSnapshotTimeoutMultiplier
	wa := &util.WorkerArgs{
		Name:          "snapper",
		Log:           c.Log,
		SleepInterval: c.sleepPeriod,
	}
	c.runCount = 0
	c.worker, _ = util.NewWorker(wa, c)
	if wid, err := c.App.CrudeOps.Watch(c.getWatcherArgs(), c); err == nil {
		c.watcherID = wid
	} else {
		c.Log.Errorf("Failed to create watcher: %s", err.Error())
	}
}

// Start starts this component
func (c *Comp) Start() {
	c.Log.Info("Starting Snapper")
	c.oCrud = crud.NewClient(c.App.ClientAPI, c.Log)
	c.runCount = 0
	c.worker.Start()
}

// Stop terminates this component
func (c *Comp) Stop() {
	c.App.CrudeOps.TerminateWatcher(c.watcherID)
	c.worker.Stop()
	c.Log.Info("Stopped Snapper")
}

// Buzz satisfies the util.WorkerBee interface
func (c *Comp) Buzz(ctx context.Context) error {
	c.runCount++
	if c.clusterID == "" {
		if c.App.AppObjects == nil {
			return nil
		}
		cObj := c.App.AppObjects.GetCluster()
		if cObj == nil {
			return nil
		}
		c.clusterID = models.ObjID(cObj.Meta.ID)
		c.sleepPeriod = time.Duration(c.App.CGSnapshotSchedPeriod)
		c.worker.SetSleepInterval(c.sleepPeriod)
	}
	return c.runSnapperBody(ctx)
}

type cgSnapOK struct {
	volOk          bool
	lastSnapOk     bool
	lastSnapNeeded bool
	minTime        time.Time
	minRPO         time.Duration
	pendingVSRs    bool
}

func (c *Comp) volInUse(vsObj *models.VolumeSeries) bool {
	if vsObj.VolumeSeriesState != com.VolStateInUse {
		return false
	}
	tl := util.NewTagList(vsObj.SystemTags)
	if _, ok := tl.Get(com.SystemTagVsrRestoring); ok {
		return false
	}
	return true
}

func (c *Comp) pendingVSRs(ctx context.Context, vsObj *models.VolumeSeries) (bool, error) {
	lParams := &volume_series_request.VolumeSeriesRequestListParams{
		IsTerminated:   swag.Bool(false),
		VolumeSeriesID: swag.String(string(vsObj.Meta.ID)),
	}
	vsrLRet, err := c.oCrud.VolumeSeriesRequestList(ctx, lParams)
	if err != nil {
		return true, err
	}
	if len(vsrLRet.Payload) != 0 {
		return true, nil
	}
	return false, nil
}

func (c *Comp) runSnapperBody(ctx context.Context) error {
	maxTime := util.TimeMaxUsefulUpperBound()
	now := time.Now()
	lParams := volume_series.NewVolumeSeriesListParams()
	lParams.BoundClusterID = swag.String(string(c.clusterID))
	vsLRet, err := c.oCrud.VolumeSeriesList(ctx, lParams)
	if err != nil {
		return err
	}
	if len(vsLRet.Payload) == 0 {
		return nil
	}

	cgMap := make(map[models.ObjIDMutable]*cgSnapOK)
	for _, vsObj := range vsLRet.Payload {
		cgID := vsObj.ConsistencyGroupID
		if _, ok := cgMap[cgID]; !ok {
			cgMap[cgID] = &cgSnapOK{
				volOk:          false,
				lastSnapNeeded: false,
				minTime:        maxTime,
				minRPO:         MaxRPO, // set max minRPO
				pendingVSRs:    false,
			}
		}
		pendingVSR, err := c.pendingVSRs(ctx, vsObj)
		if err != nil {
			return err
		}
		cgMap[cgID].pendingVSRs = pendingVSR

		if c.volInUse(vsObj) {
			cgMap[cgID].volOk = true
			if time.Time(vsObj.LifecycleManagementData.NextSnapshotTime).Before(cgMap[cgID].minTime) {
				cgMap[cgID].minTime = time.Time(vsObj.LifecycleManagementData.NextSnapshotTime)
			}
			spObj, err := c.App.AppServant.GetServicePlan(ctx, string(vsObj.ServicePlanID))
			if err != nil {
				return err
			}
			rpoDur := util.ParseValueType(spObj.Slos["RPO"].ValueType, com.ValueTypeDuration, SloRPODefault).(time.Duration)
			if rpoDur < cgMap[cgID].minRPO {
				cgMap[cgID].minRPO = rpoDur
			}
		}
		// If any single vs has a this flag set, it indicates that the last snapshot should be taken
		if vsObj.LifecycleManagementData.FinalSnapshotNeeded == true {
			cgMap[cgID].lastSnapNeeded = true
		}
	}

	accountSnapPolicyMap := make(map[models.ObjIDMutable]bool)
	for cgID, snapOK := range cgMap {
		cgObj, err := c.oCrud.ConsistencyGroupFetch(ctx, string(cgID))
		if err != nil {
			c.Log.Errorf("Failed to fetch CG [%s]", cgID)
			continue
		}
		disableSnapshotCreation := false
		if cgObj.SnapshotManagementPolicy == nil {
			if _, ok := accountSnapPolicyMap[cgObj.AccountID]; !ok {
				aObj, err := c.oCrud.AccountFetch(ctx, string(cgObj.AccountID))
				if err != nil {
					c.Log.Errorf("Failed to fetch account [%s]", cgObj.AccountID)
					continue
				}
				accountSnapPolicyMap[cgObj.AccountID] = aObj.SnapshotManagementPolicy.DisableSnapshotCreation
			}
			disableSnapshotCreation = accountSnapPolicyMap[cgObj.AccountID]
		} else {
			disableSnapshotCreation = cgObj.SnapshotManagementPolicy.DisableSnapshotCreation
		}
		c.Log.Debugf("CG [%s]: volOk[%v], minTime[%v], pendingVSRs[%v], lastSnapNeeded[%v], disableSnap[%v]",
			cgID, snapOK.volOk, snapOK.minTime, snapOK.pendingVSRs, snapOK.lastSnapNeeded, disableSnapshotCreation)
		if !disableSnapshotCreation {
			if (snapOK.volOk && snapOK.minTime.Before(now) && !snapOK.pendingVSRs) || snapOK.lastSnapNeeded { // attempt last snapshot even if pending VSRs
				c.Log.Debugf("Attempting to create VSR for snapshot of CG [%s]", cgID)
				vsrCreate := &models.VolumeSeriesRequest{}
				vsrCreate.ConsistencyGroupID = models.ObjIDMutable(string(cgID))
				vsrCreate.ClusterID = models.ObjIDMutable(string(c.clusterID))
				vsrCreate.RequestedOperations = []string{com.VolReqOpCGCreateSnapshot}
				minRPOSeconds := int64(float64(snapOK.minRPO.Seconds()) * c.timeoutMultiplier)
				completeByDuration := time.Duration(minRPOSeconds) * time.Second
				vsrCreate.CompleteByTime = strfmt.DateTime(time.Now().Add(completeByDuration))
				_, err := c.oCrud.VolumeSeriesRequestCreate(ctx, vsrCreate)
				if err == nil {
					c.Log.Infof("Created VSR for snapshot of consistency group %s", cgID)
				} else {
					c.Log.Errorf("Snapshot VSR create failed for CG [%s]: %s", cgID, err.Error())
				}
			}
		}
	}
	return nil
}

// CrudeNotify is used to wake the loop
func (c *Comp) CrudeNotify(cbt crude.WatcherCallbackType, ce *crude.CrudEvent) error {
	if cbt == crude.WatcherEvent {
		c.worker.Notify()
	}
	return nil
}

// getWatcherArgs returns watcher args for this component.
// Note: Only node scoped events are visible, courtesy of the upstream monitor in hb.
func (c *Comp) getWatcherArgs() *models.CrudWatcherCreateArgs {
	return &models.CrudWatcherCreateArgs{
		Name: "VSR",
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{ // VSR activities triggered from upstream or downstream
				MethodPattern: "PATCH",
				URIPattern:    "^/volume-series-requests/.*(set=mountedNodeDevice.*set=lifecycleManagementData)|(set=lifecycleManagementData.*set=mountedNodeDevice)",
				ScopePattern:  ".*volumeSeriesRequestState:SUCCEEDED",
			},
		},
	}
}
