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


package heartbeat

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/node"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	mockcluster "github.com/Nuvoloso/kontroller/pkg/cluster/mock"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	fmm "github.com/Nuvoloso/kontroller/pkg/metricmover/fake"
	fakecrud "github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestNewTerminatedNodeHandler(t *testing.T) {
	assert := assert.New(t)

	comp := newHBComp()
	ntd := newTerminatedNodeHandler(comp)
	assert.NotNil(ntd)
	ts, ok := ntd.(*termState)
	if assert.True(ok) {
		assert.Equal(comp, ts.c)
		assert.False(ts.wasReady)
	}
}

func TestDetectTerminatedNodes(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log:                tl.Logger(),
			HeartbeatPeriod:    10,
			CSPDomainID:        "6a156de2-ba94-4035-b47f-e87373d75757",
			ClusterType:        "kubernetes",
			SystemID:           "system-1",
			ClusterNamespace:   "nuvoloso-cluster",
			NodeControllerKind: "DaemonSet",
			NodeControllerName: "nuvoloso-node",
		},
		ClusterMD:  fakeClusterMD,
		InstanceMD: fakeNodeMD,
	}
	eM := &fev.Manager{}
	app.CrudeOps = eM
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	fCrud := &fakecrud.Client{}
	app.OCrud = fCrud
	svcArgs := util.ServiceArgs{
		ServiceType:         "clusterd",
		ServiceVersion:      "test",
		HeartbeatPeriodSecs: int64(app.HeartbeatPeriod),
		Log:                 app.Log,
		MaxMessages:         app.MaxMessages,
		ServiceAttributes:   map[string]models.ValueType{},
	}
	app.Service = util.NewService(&svcArgs)
	comp := newHBComp()
	comp.app = app
	comp.Log = app.Log
	comp.mObj = &models.Cluster{ClusterAllOf0: models.ClusterAllOf0{Meta: &models.ObjMeta{ID: "cid"}}}
	comp.heartbeatTaskPeriod = 20 * time.Second
	ts := &termState{c: comp}
	ctxKey := struct{}{}
	ctx := context.WithValue(context.Background(), ctxKey, 42)

	t.Log("case: service not ready")
	assert.False(ts.c.app.IsReady())
	ts.detectTerminatedNodes(ctx)
	assert.Zero(tl.CountPattern("."))
	tl.Flush()

	t.Log("case: service becomes not ready")
	ts.wasReady = true
	assert.False(ts.c.app.IsReady())
	ts.detectTerminatedNodes(ctx)
	assert.Equal(1, tl.CountPattern("waiting for service to be ready"))
	assert.False(ts.wasReady)
	tl.Flush()

	t.Log("case: service becomes ready")
	assert.NoError(app.Service.SetState(util.ServiceReady))
	fCrud.RetLsNObj = &node.NodeListOK{}
	ts.detectTerminatedNodes(ctx)
	assert.True(ts.wasReady)
	assert.Equal(1, tl.CountPattern(": starting detection"))
	tl.Flush()

	t.Log("case: go! NodeList fails")
	fCrud.RetLsNErr = fakecrud.TransientErr
	ts.detectTerminatedNodes(ctx)
	assert.Zero(tl.CountPattern(": starting detection"))
	assert.Equal(1, tl.CountPattern(": NodeList.*transientErr"))
	expNodeListParams := &node.NodeListParams{
		ClusterID: swag.String("cid"),
		StateEQ:   swag.String("TIMED_OUT"),
	}
	expNodeListParams.SetTimeout(30 * time.Second)
	assert.Equal(expNodeListParams, fCrud.InLsNObj)
	assert.True(ts.wasReady)
	tl.Flush()

	t.Log("case: no nodes in configDB")
	fCrud.RetLsNErr = nil
	fCrud.RetLsNObj = &node.NodeListOK{}
	ts.detectTerminatedNodes(ctx)
	tl.Flush()

	t.Log("case: node missing Hostname")
	nObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{ID: "nid"},
		},
	}
	fCrud.RetLsNObj = &node.NodeListOK{Payload: []*models.Node{nObj}}
	ts.detectTerminatedNodes(ctx)
	assert.Equal(1, tl.CountPattern("no nodeAttributes.Hostname.*skipped"))
	tl.Flush()

	t.Log("case: cluster NodeFetch transientErr")
	nObj = &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{ID: "nid"},
		},
	}
	nObj.NodeAttributes = map[string]models.ValueType{
		"Hostname": models.ValueType{Kind: "STRING", Value: "node.local"},
	}
	fCrud.RetLsNObj = &node.NodeListOK{Payload: []*models.Node{nObj}}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mCluster := mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().NodeFetch(ctx, &cluster.NodeFetchArgs{Name: "node.local"}).Return(nil, fakecrud.TransientErr)
	ts.detectTerminatedNodes(ctx)
	assert.Equal(1, tl.CountPattern("cluster.NodeFetch.*transientErr"))
	mockCtrl.Finish()
	tl.Flush()

	t.Log("case: cluster NodeFetch deleted, create VSR")
	mockCtrl = gomock.NewController(t)
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	cErr := cluster.NewK8sError("", cluster.StatusReasonNotFound)
	mCluster.EXPECT().NodeFetch(ctx, &cluster.NodeFetchArgs{Name: "node.local"}).Return(nil, cErr)
	ts.detectTerminatedNodes(ctx)
	assert.Equal(1, tl.CountPattern("has been deleted"))
	assert.Equal(1, tl.CountPattern("launched VolumeSeriesRequestCreate"))
	tl.Flush()

	t.Log("case: cluster NodeFetch exists, ControllerFetch error")
	mockCtrl = gomock.NewController(t)
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().NodeFetch(ctx, &cluster.NodeFetchArgs{Name: "node.local"}).Return(&cluster.NodeObj{}, nil)
	cfa := &cluster.ControllerFetchArgs{Kind: "DaemonSet", Name: "nuvoloso-node", Namespace: "nuvoloso-cluster"}
	mCluster.EXPECT().ControllerFetch(ctx, cfa).Return(nil, fakecrud.TransientErr)
	ts.detectTerminatedNodes(ctx)
	assert.Equal(1, tl.CountPattern("cluster.ControllerFetch.*transientErr"))
	tl.Flush()

	t.Log("case: cluster NodeFetch exists, controller replicas not ready")
	mockCtrl = gomock.NewController(t)
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().NodeFetch(ctx, &cluster.NodeFetchArgs{Name: "node.local"}).Return(&cluster.NodeObj{}, nil)
	cRet := &cluster.ControllerObj{Replicas: 3, ReadyReplicas: 2, Selectors: []string{"a", "b"}}
	mCluster.EXPECT().ControllerFetch(ctx, cfa).Return(cRet, nil)
	ts.detectTerminatedNodes(ctx)
	assert.Equal(1, tl.CountPattern("DaemonSet nuvoloso-node not all replicas are ready, 2 < 3"))
	tl.Flush()

	t.Log("case: PodList fails")
	mockCtrl = gomock.NewController(t)
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().NodeFetch(ctx, &cluster.NodeFetchArgs{Name: "node.local"}).Return(&cluster.NodeObj{}, nil)
	cRet.ReadyReplicas = 3
	mCluster.EXPECT().ControllerFetch(ctx, cfa).Return(cRet, nil)
	pla := &cluster.PodListArgs{Namespace: "nuvoloso-cluster", NodeName: "node.local", Labels: []string{"a", "b"}}
	mCluster.EXPECT().PodList(ctx, pla).Return(nil, fakecrud.TransientErr)
	ts.detectTerminatedNodes(ctx)
	assert.Equal(1, tl.CountPattern("cluster.PodList.*transientErr"))
	tl.Flush()

	t.Log("case: PodList succeeds, pod still exists")
	mockCtrl = gomock.NewController(t)
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().NodeFetch(ctx, &cluster.NodeFetchArgs{Name: "node.local"}).Return(&cluster.NodeObj{}, nil)
	mCluster.EXPECT().ControllerFetch(ctx, cfa).Return(cRet, nil)
	mCluster.EXPECT().PodList(ctx, pla).Return([]*cluster.PodObj{{}}, nil)
	ts.detectTerminatedNodes(ctx)
	assert.Equal(1, tl.CountPattern(": pod found for DaemonSet nuvoloso-node on cluster node node.local"))
	tl.Flush()

	t.Log("case: PodList succeeds, pod gone, create VSR")
	mockCtrl = gomock.NewController(t)
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().NodeFetch(ctx, &cluster.NodeFetchArgs{Name: "node.local"}).Return(&cluster.NodeObj{}, nil)
	mCluster.EXPECT().ControllerFetch(ctx, cfa).Return(cRet, nil)
	mCluster.EXPECT().PodList(ctx, pla).Return([]*cluster.PodObj{}, nil)
	ts.detectTerminatedNodes(ctx)
	assert.Equal(1, tl.CountPattern(": no pod for DaemonSet nuvoloso-node found on cluster node node.local"))
	assert.Equal(1, tl.CountPattern("launched VolumeSeriesRequestCreate"))
	tl.Flush()

	// ****************** launchNodeDelete
	// success
	fCrud.RetVRCErr = nil
	inVRCArgs := &models.VolumeSeriesRequest{
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			CompleteByTime:      strfmt.DateTime(util.DateTimeMaxUpperBound()),
			RequestedOperations: []string{com.VolReqOpNodeDelete},
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID: models.ObjIDMutable("someID"),
			},
		},
	}
	ts.launchNodeDelete(ctx, "someID", "detector")
	assert.Equal(inVRCArgs, fCrud.InVRCArgs)
	assert.Equal(1, tl.CountPattern("launched VolumeSeriesRequestCreate"))
	tl.Flush()

	// error
	fCrud.RetVRCErr = fmt.Errorf("vsr create error")
	ts.launchNodeDelete(ctx, "someID", "detector")
	assert.Equal(1, tl.CountPattern("vsr create error"))
}

func TestRelaunchFailedNodeDelete(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log: tl.Logger(),
		},
		ClusterMD:  fakeClusterMD,
		InstanceMD: fakeNodeMD,
	}
	fCrud := &fakecrud.Client{}
	app.OCrud = fCrud
	comp := newHBComp()
	comp.app = app
	comp.Log = app.Log
	comp.mObj = &models.Cluster{ClusterAllOf0: models.ClusterAllOf0{Meta: &models.ObjMeta{ID: "cid"}}}
	comp.heartbeatTaskPeriod = 20 * time.Second
	ts := &termState{c: comp}
	ctx := context.Background()

	// success
	fCrud.RetLsNErr = nil
	fCrud.RetLsNObj = &node.NodeListOK{
		Payload: []*models.Node{
			&models.Node{
				NodeAllOf0: models.NodeAllOf0{
					Meta: &models.ObjMeta{
						ID: "node1",
					},
				},
				NodeMutable: models.NodeMutable{
					State: com.NodeStateTearDown,
				},
			},
		},
	}
	fCrud.RetLsVRObj = &volume_series_request.VolumeSeriesRequestListOK{
		Payload: []*models.VolumeSeriesRequest{
			&models.VolumeSeriesRequest{
				VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
					VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
						NodeID: models.ObjIDMutable("node1"),
					},
				},
			},
		},
	}
	ts.relaunchFailedNodeDelete(ctx)
	assert.Equal(1, tl.CountPattern("launched VolumeSeriesRequestCreate"))
	tl.Flush()

	// failed vsr list, continue, fail again
	fCrud.RetLsNObj.Payload = append(fCrud.RetLsNObj.Payload, &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "node2",
			},
		},
	})
	fCrud.RetLsVRErr = fmt.Errorf("vsr list error")
	ts.relaunchFailedNodeDelete(ctx)
	assert.Equal(2, tl.CountPattern("vsr list error"))
	tl.Flush()

	// node list error
	fCrud.RetLsNErr = fmt.Errorf("node list error")
	ts.relaunchFailedNodeDelete(ctx)
	assert.Equal(1, tl.CountPattern("node list error"))
}

func TestPurgeTerminatedNodes(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log: tl.Logger(),
		},
		ClusterMD:  fakeClusterMD,
		InstanceMD: fakeNodeMD,
	}
	fCrud := &fakecrud.Client{}
	app.OCrud = fCrud
	comp := newHBComp()
	comp.app = app
	comp.Log = app.Log
	comp.mObj = &models.Cluster{ClusterAllOf0: models.ClusterAllOf0{Meta: &models.ObjMeta{ID: "cid"}}}
	comp.heartbeatTaskPeriod = 20 * time.Second
	ts := &termState{c: comp}
	ctx := context.Background()
	svcArgs := util.ServiceArgs{
		ServiceType:         "clusterd",
		ServiceVersion:      "test",
		HeartbeatPeriodSecs: int64(app.HeartbeatPeriod),
		Log:                 app.Log,
		MaxMessages:         app.MaxMessages,
		ServiceAttributes:   map[string]models.ValueType{},
	}
	app.Service = util.NewService(&svcArgs)
	app.Service.SetState(util.ServiceNotReady)

	ts.wasReady = true
	fCrud.RetLsNErr = fmt.Errorf("node list error")
	ts.handleTerminatedNodes(ctx)
	assert.Equal(1, tl.CountPattern("waiting for service to be ready"))
	assert.Equal(1, tl.CountPattern("node list error"))
}

type fakeTerminatedNodeHandler struct {
	inCtx                    context.Context
	detectTerminatedNodesCnt int
}

var _ = terminatedNodeHandler(&fakeTerminatedNodeHandler{})

func (ts *fakeTerminatedNodeHandler) handleTerminatedNodes(ctx context.Context) {
	ts.inCtx = ctx
	ts.detectTerminatedNodesCnt++
}
