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
	"regexp"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	mockcluster "github.com/Nuvoloso/kontroller/pkg/cluster/mock"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	fcl "github.com/Nuvoloso/kontroller/pkg/clusterd/fake"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	fakecrud "github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	fw "github.com/Nuvoloso/kontroller/pkg/util/fake"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestUnbinderMethods(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log:             tl.Logger(),
			HeartbeatPeriod: 10,
			CSPDomainID:     "6a156de2-ba94-4035-b47f-e87373d75757",
		},
	}

	app.OCrud = &fakecrud.Client{}
	// init fails to create watcher
	c := newUnbinderComp()
	c.Init(app)
	assert.Empty(c.watcherID)
	assert.Equal(app, c.App)
	assert.Equal(app.Log, c.Log)
	assert.NotNil(c.worker)
	assert.Equal(FastInterval, c.worker.GetSleepInterval())

	// replace the worker
	tw := &fw.Worker{}
	c.worker = tw

	c.Start()
	assert.NotNil(c.oCrud)
	assert.Equal(0, c.runCount)
	assert.Equal(1, tw.CntStart)
	assert.NotNil(c.unBindQueue)

	c.pvWatcher = &fakePVWatcher{}
	c.Stop()
	foundStarting := 0
	foundStopped := 0
	assert.Equal(1, tw.CntStop)
	tl.Iterate(func(i uint64, s string) {
		if res, err := regexp.MatchString("Starting Unbinder", s); err == nil && res {
			foundStarting++
		}
		if res, err := regexp.MatchString("Stopped Unbinder", s); err == nil && res {
			foundStopped++
		}
	})
	assert.Equal(1, foundStarting)
	assert.Equal(1, foundStopped)
}

func TestUnbinderBuzz(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	fao := &fcl.AppObjects{}
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log: tl.Logger(),
		},
	}
	svcArgs := util.ServiceArgs{
		Log: app.Log,
	}
	app.Service = util.NewService(&svcArgs)
	ctx := context.Background()

	// cluster not ready
	c := newUnbinderComp()
	c.Init(app)
	c.App.Service.SetState(util.ServiceNotReady)
	err := c.Buzz(ctx)
	assert.Nil(err)
	assert.Equal(1, c.runCount)

	// appObjects not set
	c.App.Service.SetState(util.ServiceReady)
	c.clusterID = ""
	err = c.Buzz(ctx)
	assert.Nil(err)
	assert.Equal(2, c.runCount)

	// appObjects doesn't return cluster
	fao.RetGCobj = nil
	c.App.AppObjects = fao
	err = c.Buzz(ctx)
	assert.Nil(err)
	assert.Equal(3, c.runCount)

	// clusterID get set, watcher gets created, first (empty)list processed, watcher start gets called, return err
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cc := mockcluster.NewMockClient(mockCtrl)
	c.clusterClient = cc
	pvW := &fakePVWatcher{}
	pvW.iaRet = false
	pvW.startErr = fmt.Errorf("error")
	pvWArgs := &cluster.PVWatcherArgs{
		Timeout: PersistentVolmeWatcherTimeout,
		Ops:     c,
	}
	cc.EXPECT().CreatePublishedPersistentVolumeWatcher(ctx, pvWArgs).Return(pvW, []*cluster.PersistentVolumeObj{}, nil)
	fao.RetGCobj = &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "CLUSTER-1",
			},
		},
	}
	c.App.AppObjects = fao
	c.unBindQueue = util.NewQueue("")
	fc := &fake.Client{}
	c.oCrud = fc
	fc.RetLsVOk = &volume_series.VolumeSeriesListOK{Payload: []*models.VolumeSeries{}}
	timeBefore := time.Now()
	err = c.Buzz(ctx)
	timeAfter := time.Now()
	assert.Error(err)
	assert.Equal(4, c.runCount)
	assert.True(pvW.startCalled)
	assert.Equal(UnbinderWorkerInterval, c.worker.GetSleepInterval())
	assert.True(c.lastList.After(timeBefore))
	assert.True(c.lastList.Before(timeAfter))
	assert.Equal("CLUSTER-1", c.clusterID)

	// watcher is inactive, create new watcher fails, get published list returns empty, nothing is done
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cc = mockcluster.NewMockClient(mockCtrl)
	c.clusterClient = cc
	pvW = &fakePVWatcher{}
	pvW.iaRet = false
	c.pvWatcher = pvW
	listTime := time.Now().Add(-ListRefreshInterval - (time.Second * 5))
	c.lastList = listTime
	cc.EXPECT().CreatePublishedPersistentVolumeWatcher(ctx, gomock.Any()).Return(nil, nil, fmt.Errorf("pvwatcher create error"))
	err = c.Buzz(ctx)
	assert.Error(err)
	assert.Regexp("pvwatcher create error", err.Error())
	assert.Equal(5, c.runCount)
	assert.Equal(listTime, c.lastList)
	assert.Equal(FastInterval, c.worker.GetSleepInterval())
	assert.Nil(c.pvWatcher)

	// Watcher is inactive, create new watcher succeeds, get published list returns volume, volume is processed, process fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cc = mockcluster.NewMockClient(mockCtrl)
	c.clusterClient = cc
	pvW = &fakePVWatcher{}
	pvW.iaRet = false
	c.pvWatcher = pvW
	c.lastList = time.Now().Add(-ListRefreshInterval - (time.Second * 5))
	pvObj := &cluster.PersistentVolumeObj{}
	cc.EXPECT().CreatePublishedPersistentVolumeWatcher(ctx, gomock.Any()).Return(nil, nil, nil)
	cc.EXPECT().PublishedPersistentVolumeList(ctx).Return([]*cluster.PersistentVolumeObj{pvObj}, nil)
	fc = &fake.Client{}
	c.oCrud = fc
	fc.RetLsVErr = fmt.Errorf("vs list error")
	timeBefore = time.Now()
	err = c.Buzz(ctx)
	timeAfter = time.Now()
	assert.Error(err)
	assert.Regexp("vs list error", err.Error())
	assert.Equal(6, c.runCount)
	assert.True(c.lastList.After(timeBefore))
	assert.True(c.lastList.Before(timeAfter))
	assert.Equal(UnbinderWorkerInterval, c.worker.GetSleepInterval())
	assert.Nil(c.pvWatcher)
	tl.Flush()

	// Watcher is active, past ListRefreshInterval, get published list returns error, volume is processed
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cc = mockcluster.NewMockClient(mockCtrl)
	c.clusterClient = cc
	pvW = &fakePVWatcher{}
	pvW.iaRet = true
	c.pvWatcher = pvW
	c.worker.SetSleepInterval(UnbinderWorkerInterval)
	lastListPrev := time.Now().Add(-ListRefreshInterval - (time.Second * 5))
	c.lastList = lastListPrev
	cc.EXPECT().PublishedPersistentVolumeList(ctx).Return(nil, fmt.Errorf("pb list error"))
	err = c.Buzz(ctx)
	assert.Error(err)
	assert.Regexp("pb list error", err.Error())
	assert.Equal(7, c.runCount)
	assert.Equal(lastListPrev, c.lastList)
	assert.Equal(UnbinderWorkerInterval, c.worker.GetSleepInterval())
	assert.NotNil(c.pvWatcher)
	tl.Flush()
}

func TestUnbinderOtherMethods(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log: tl.Logger(),
		},
	}
	svcArgs := util.ServiceArgs{
		Log: app.Log,
	}
	app.Service = util.NewService(&svcArgs)
	ctx := context.Background()

	fvdh := &fakeVolDelHandler{}
	app.VolumeDeletionHandler = fvdh

	c := newUnbinderComp()
	c.Init(app)

	// *********** doUnbind
	// success
	c.unBindQueue = util.NewQueue("")
	c.unBindQueue.Add("vs1")
	c.unBindQueue.Add("vs2")
	c.doUnbind(ctx)
	assert.Equal(2, fvdh.cntDUV)

	// errors on create vsrs
	fvdh.outDUVerr = fmt.Errorf("delete error")
	fvdh.cntDUV = 0
	c.unBindQueue.Add("vs1")
	c.unBindQueue.Add("vs2")
	c.doUnbind(ctx)
	assert.Equal(2, tl.CountPattern("delete error"))
	assert.Equal(2, fvdh.cntDUV)

	// *********** processList
	resVS := &volume_series.VolumeSeriesListOK{
		Payload: []*models.VolumeSeries{
			&models.VolumeSeries{
				VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
					Meta: &models.ObjMeta{
						ID: "VS-1",
					},
				},
			},
			&models.VolumeSeries{
				VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
					Meta: &models.ObjMeta{
						ID: "VS-2",
					},
				},
			},
			&models.VolumeSeries{
				VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
					Meta: &models.ObjMeta{
						ID: "VS-3",
					},
				},
			},
		},
	}
	pvList := []*cluster.PersistentVolumeObj{
		&cluster.PersistentVolumeObj{
			VolumeSeriesID: "VS-1",
		},
		&cluster.PersistentVolumeObj{
			VolumeSeriesID: "VS-2",
		},
		&cluster.PersistentVolumeObj{
			VolumeSeriesID: "VS-3",
		},
	}

	// No missing PVs in cluster
	fc := &fake.Client{}
	c.oCrud = fc
	fc.RetLsVOk = resVS
	c.clusterID = "cluster-1"
	assert.Equal(0, c.unBindQueue.Length())
	err := c.processList(ctx, pvList)
	assert.Nil(err)
	assert.Equal(0, c.unBindQueue.Length())
	assert.Equal(swag.String("cluster-1"), fc.InLsVObj.BoundClusterID)
	assert.Equal([]string{com.VolStateBound, com.VolStateProvisioned}, fc.InLsVObj.VolumeSeriesState)
	assert.Equal([]string{com.SystemTagVolumePublished}, fc.InLsVObj.SystemTags)

	// missing PVs
	assert.Equal(0, c.unBindQueue.Length())
	err = c.processList(ctx, []*cluster.PersistentVolumeObj{})
	assert.Nil(err)
	assert.Equal(3, c.unBindQueue.Length())

	// Error for vsl
	fc.RetLsVErr = fmt.Errorf("vsl error")
	err = c.processList(ctx, pvList)
	assert.Error(err)
	assert.Regexp("vsl error", err.Error())

	// *********** PVdeleted
	c.unBindQueue = util.NewQueue("")
	err = c.PVDeleted(&cluster.PersistentVolumeObj{VolumeSeriesID: "VS-1"})
	assert.Nil(err)
	assert.Equal(1, c.unBindQueue.Length())
	assert.Equal(1, c.worker.GetNotifyCount())
}

type fakePVWatcher struct {
	startErr    error
	startCalled bool
	iaRet       bool
}

func (fpvw *fakePVWatcher) Start(ctx context.Context) error {
	fpvw.startCalled = true
	if fpvw.startErr == nil {
		fpvw.iaRet = true
	}
	return fpvw.startErr
}

func (fpvw *fakePVWatcher) Stop() {}

func (fpvw *fakePVWatcher) IsActive() bool {
	return fpvw.iaRet
}

type fakeVolDelHandler struct {
	cntDUV          int
	inDUVId         []string
	inDUVLogKeyword []string
	outDUVerr       error
}

func (fvdh *fakeVolDelHandler) DeleteOrUnbindVolume(ctx context.Context, volumeID string, logKeyword string) error {
	fvdh.cntDUV++
	fvdh.inDUVId = append(fvdh.inDUVId, volumeID)
	fvdh.inDUVLogKeyword = append(fvdh.inDUVLogKeyword, logKeyword)
	return fvdh.outDUVerr
}

type testWorker struct {
	Cnt    int
	RetErr error
}

func (tw *testWorker) Notify() error {
	tw.Cnt++
	return tw.RetErr
}
