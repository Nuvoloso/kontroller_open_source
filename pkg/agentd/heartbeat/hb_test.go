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
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	"github.com/Nuvoloso/kontroller/pkg/agentd/state"
	mcl "github.com/Nuvoloso/kontroller/pkg/autogen/client/cluster"
	mcsp "github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_domain"
	mn "github.com/Nuvoloso/kontroller/pkg/autogen/client/node"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/system"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	mockcluster "github.com/Nuvoloso/kontroller/pkg/cluster/mock"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	mockCSP "github.com/Nuvoloso/kontroller/pkg/csp/mock"
	"github.com/Nuvoloso/kontroller/pkg/metricmover"
	fmm "github.com/Nuvoloso/kontroller/pkg/metricmover/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	fakecrud "github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	mockNuvo "github.com/Nuvoloso/kontroller/pkg/nuvoapi/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	fw "github.com/Nuvoloso/kontroller/pkg/util/fake"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
)

type mockAppServantMatcher struct {
	t       *testing.T
	pattern string
}

// Matches is from gomock.Matcher
func (o *mockAppServantMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch e := x.(type) {
	case error:
		return assert.Regexp(o.pattern, e.Error())
	}
	return false
}

// String is from gomock.Matcher
func (o *mockAppServantMatcher) String() string {
	return "matches on fatalError"
}

func newAppServantMatcher(t *testing.T, pattern string) *mockAppServantMatcher {
	return &mockAppServantMatcher{t: t, pattern: pattern}
}

var fakeNodeMD = map[string]string{
	csp.IMDHostname:     "ip-172-31-24-56.us-west-2.compute.internal",
	csp.IMDInstanceName: "i-00cf71dee433b52db",
	csp.IMDLocalIP:      "172.31.24.56",
	csp.IMDZone:         "us-west-2a",
	"public-ipv4":       "34.212.187.125",
	"instance-type":     "t2.micro",
	"local-hostname":    "ip-172-31-24-56.us-west-2.compute.internal",
	"public-hostname":   "ec2-34-212-187-125.us-west-2.compute.amazonaws.com",
}

var fakeClusterMD = map[string]string{
	cluster.CMDIdentifier: "k8s-svc-uid:c981db76-bda0-11e7-b2ce-02a3152c8208",
	cluster.CMDVersion:    "1.7",
	"CreationTimestamp":   "2017-10-30T18:33:13Z",
	"GitVersion":          "v1.7.8",
	"Platform":            "linux/amd64",
}

func TestComponentMethods(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log:                   tl.Logger(),
			HeartbeatPeriod:       10,
			NuvoStartupPollPeriod: 5,
			NuvoStartupPollMax:    5,
			CSPDomainID:           "6a156de2-ba94-4035-b47f-e87373d75757",
		},
		InstanceMD: fakeNodeMD,
		ClusterMD:  fakeClusterMD,
	}
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	hb := newHBComp()

	// Init
	assert.Nil(app.AppObjects)
	assert.NotPanics(func() { hb.Init(app) })
	assert.Equal(app.Log, hb.Log)
	assert.Equal(app, hb.app)
	assert.Equal(10*time.Second, hb.sleepPeriod)
	assert.Equal(5*time.Second, hb.nuvoPollPeriod)
	assert.Equal(5, hb.nuvoPollMaxCount)
	assert.NotNil(hb.worker)
	assert.Equal("", hb.nodeIdentifier)
	assert.Equal("", hb.clusterID)
	assert.Equal("", hb.clusterIdentifier)
	assert.NotNil(app.AppObjects)
	assert.EqualValues(hb, app.AppObjects)
	assert.Equal(InitialRetryInterval, hb.worker.GetSleepInterval())

	// replace the worker
	tw := &fw.Worker{}
	hb.worker = tw

	hb.Start()
	assert.NotEmpty(hb.clusterIdentifier)
	assert.NotEmpty(hb.nodeIdentifier)
	assert.Equal(1, tw.CntStart)

	hb.Refresh()
	assert.Equal(1, tw.CntNotify)

	hb.Stop()
	assert.Equal(1, tw.CntStop)

	// node attribute creation
	nAttr := hb.nodeAttrs()
	assert.Len(nAttr, len(fakeNodeMD))
	for n, v := range nAttr {
		mdv, ok := fakeNodeMD[n]
		assert.True(ok)
		assert.True(v.Kind == "STRING")
		assert.Equal(mdv, v.Value)
	}

	// test the heartbeat predicate
	now := time.Now()
	hb.lastHeartbeatTime = time.Time{}
	assert.True(hb.canPostHeartbeat(now))
	hb.lastHeartbeatTime = now
	assert.False(hb.canPostHeartbeat(now))

	// test the max nuvo sleep interval
	tw.InSSI = 0
	for i := 0; i < app.NuvoStartupPollMax; i++ {
		hb.setSleepInterval(hb.nuvoPollPeriod)
		assert.Equal(hb.nuvoPollPeriod, tw.InSSI, "nuvo poll period [i]=%d", i)
		assert.Equal(i+1, hb.nuvoPollCount, "nuvo poll count [i]=%d", i)
	}
	hb.setSleepInterval(hb.nuvoPollPeriod)
	assert.Equal(hb.sleepPeriod, tw.InSSI)
	assert.Equal(hb.nuvoPollMaxCount+1, hb.nuvoPollCount)

	hb.setSleepInterval(hb.sleepPeriod)
	assert.Equal(hb.sleepPeriod, tw.InSSI)
	assert.Equal(0, hb.nuvoPollCount)
}

func TestNodeCreationOnStart(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log:                   tl.Logger(),
			HeartbeatPeriod:       10,
			NuvoStartupPollPeriod: 5,
			CSPDomainID:           "6a156de2-ba94-4035-b47f-e87373d75757",
			ClusterType:           "kubernetes",
			SystemID:              "system-1",
			ClusterID:             "CLUSTER-1",
		},
		InstanceMD: fakeNodeMD,
		ClusterMD:  fakeClusterMD,
	}
	eM := &fev.Manager{}
	app.CrudeOps = eM
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	svcArgs := util.ServiceArgs{
		ServiceType:         "agentd",
		ServiceVersion:      "test",
		HeartbeatPeriodSecs: int64(app.HeartbeatPeriod),
		Log:                 app.Log,
		MaxMessages:         app.MaxMessages,
		ServiceAttributes:   map[string]models.ValueType{},
	}
	app.Service = util.NewService(&svcArgs)
	clCrud := &fakecrud.Client{}
	app.ClusterdOCrud = clCrud
	fCrud := &fakecrud.Client{}
	app.OCrud = fCrud
	hb := newHBComp()
	now := time.Now()
	mCSPObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID:           models.ObjID(app.CSPDomainID),
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
			CspDomainType: models.CspDomainTypeMutable(app.CSPDomainType),
			CspDomainAttributes: map[string]models.ValueType{
				csp.IMDZone: models.ValueType{Kind: "STRING", Value: "zone"},
			},
		},
		CSPDomainMutable: models.CSPDomainMutable{
			Name: "CSPDomain1",
		},
	}
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID:           "CLUSTER-1",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
		},
		ClusterCreateOnce: models.ClusterCreateOnce{
			ClusterType: app.ClusterType,
			CspDomainID: models.ObjIDMutable(app.CSPDomainID),
		},
		ClusterMutable: models.ClusterMutable{
			ClusterCreateMutable: models.ClusterCreateMutable{
				ClusterIdentifier: fakeClusterMD[cluster.CMDIdentifier],
			},
		},
	}
	mObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID:           "NODE-1",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
			NodeIdentifier: fakeNodeMD[csp.IMDInstanceName],
			ClusterID:      models.ObjIDMutable(clObj.Meta.ID),
		},
		NodeMutable: models.NodeMutable{
			//Service: get this data from the service after Init
			Name: models.ObjName(fakeNodeMD[csp.IMDHostname]),
			//NodeAttributes: get this after init from hb
			State: com.NodeStateManaged,
		},
	}
	sysObj := &models.System{
		SystemAllOf0: models.SystemAllOf0{
			Meta: &models.ObjMeta{ID: "system-1"},
		},
	}
	var err error
	ctx := context.Background()
	// Init
	hb.Init(app)
	assert.Equal("", hb.nodeIdentifier)
	assert.Equal(0, hb.runCount)

	mObj.NodeAttributes = hb.nodeAttrs()
	mObj.Service = app.Service.ModelObj()
	assert.NotNil(mObj.Service)

	// replace the worker
	tw := &fw.Worker{}
	hb.worker = tw

	hb.Start()
	assert.Empty(hb.clusterIdentifier)
	assert.NotEmpty(hb.nodeIdentifier)
	assert.Equal(1, tw.CntStart)

	// first the system and cluster have to be found
	// then the search for ephemeral devices must succeed (whether any exist does not matter here)
	// then node list has to return an empty array for creation to be called
	tl.Logger().Info("first iteration")
	tl.Flush()
	fCrud.RetLClObj = clObj
	fCrud.RetLClErr = nil
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	nvAPI := mockNuvo.NewMockNuvoVM(mockCtrl)
	hb.app.NuvoAPI = nvAPI
	mServant := NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	mServant.EXPECT().InitializeCSPClient(mCSPObj).Return(nil)
	mServant.EXPECT().GetClusterdAPIArgs().Return(nil)
	mServant.EXPECT().GetClusterdAPI().Return(nil, nil)
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	sysOps := mockmgmtclient.NewMockSystemClient(mockCtrl)
	sysOps.EXPECT().SystemFetch(gomock.Not(gomock.Nil())).Return(&system.SystemFetchOK{Payload: sysObj}, nil)
	mAPI.EXPECT().System().Return(sysOps)
	cspOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cspOps).MinTimes(1)
	dm := mockmgmtclient.NewCspDomainMatcher(t, mcsp.NewCspDomainFetchParams())
	dm.FetchParam.ID = app.CSPDomainID
	cspOps.EXPECT().CspDomainFetch(dm).Return(&mcsp.CspDomainFetchOK{Payload: mCSPObj}, nil).MaxTimes(1)
	clCrud.InLsNObj = nil
	clCrud.RetLsNObj = &mn.NodeListOK{}
	clCrud.RetLsNErr = nil
	clCrud.RetNodeCreateObj = mObj
	clCrud.RetNodeCreateErr = nil
	mServant.EXPECT().InitializeNuvo(gomock.Any()).Return(nil)
	mCluster := mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceReady}).MinTimes(1)
	domClient := mockCSP.NewMockDomainClient(mockCtrl)
	app.CSPClient = domClient
	var nilDevices []*csp.EphemeralDevice
	domClient.EXPECT().LocalEphemeralDevices().Return(nilDevices, nil)
	assert.Nil(hb.mCspDomainObj)
	assert.Nil(hb.ephemeralDevices)
	assert.Nil(hb.mObj)
	assert.Nil(app.StateOps)
	assert.False(hb.nuvoNodeUUIDSet)
	assert.Zero(hb.updateCount)
	err = hb.Buzz(ctx)
	assert.NoError(err)
	assert.Equal("CLUSTER-1", fCrud.InLClID)
	expNLP := mn.NewNodeListParams()
	expNLP.NodeIdentifier = &hb.nodeIdentifier
	expNLP.ClusterID = swag.String(string(clObj.Meta.ID))
	expNLP.Context = clCrud.InLsNObj.Context
	assert.Equal(expNLP, clCrud.InLsNObj)
	assert.EqualValues(mObj.NodeIdentifier, clCrud.InNodeCreateObj.NodeIdentifier)
	assert.EqualValues(mObj.ClusterID, clCrud.InNodeCreateObj.ClusterID)
	assert.EqualValues(mObj.Name, clCrud.InNodeCreateObj.Name)
	assert.Equal(com.NodeStateManaged, clCrud.InNodeCreateObj.State)
	expSS := mObj.Service.ServiceState
	ss := clCrud.InNodeCreateObj.Service.ServiceState
	assert.EqualValues(expSS.HeartbeatPeriodSecs, ss.HeartbeatPeriodSecs)
	assert.EqualValues(expSS.State, ss.State)
	assert.WithinDuration(time.Time(expSS.HeartbeatTime), time.Time(ss.HeartbeatTime), time.Second)
	assert.EqualValues(mObj.NodeAttributes, clCrud.InNodeCreateObj.NodeAttributes)
	tl.Flush()
	assert.Equal(app.InstanceMD[csp.IMDInstanceName], hb.nodeIdentifier)
	assert.NotNil(hb.mCspDomainObj)
	assert.Equal(mCSPObj, hb.mCspDomainObj)
	assert.NotNil(hb.mObj)
	assert.Equal(mObj, hb.mObj)
	assert.True(hb.updateCount > 0)
	assert.Equal(1, hb.runCount)
	assert.True(eM.CalledMU)
	assert.True(hb.createdUpMon)
	assert.Equal(mObj, hb.GetNode())
	assert.Equal(clObj, hb.GetCluster())
	assert.Equal(mCSPObj, hb.GetCspDomain())
	assert.True(hb.nuvoNodeUUIDSet)
	assert.NotZero(hb.sleepPeriod)
	assert.Equal(hb.sleepPeriod, tw.InSSI)
	// StateOps ready
	assert.NotNil(app.StateOps)
	nState := app.StateOps.NS()
	assert.Equal(mObj, nState.Node)
	tl.Flush()

	// now fake the second iteration - only heartbeat sent
	mockCtrl.Finish()
	tl.Logger().Info("second iteration: hb only")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	hb.app.NuvoAPI = nvAPI
	hb.lastHeartbeatTime = time.Time{} // force
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	assert.NotNil(hb.mObj)
	hb.mObj.Service = nil
	hb.mObj.NodeAttributes = nil
	clCrud.InNodeUpdateObj = nil // update will be used
	clCrud.InNodeUpdateItems = nil
	clCrud.RetNodeUpdateObj = nil // will be ignored
	clCrud.RetNodeUpdateErr = nil
	nvAPI.EXPECT().NodeStatus().Return(nuvoapi.NodeStatusReport{NodeUUID: "uuid"}, nil)
	assert.True(hb.updateCount > 0)
	tB := time.Now()
	err = hb.Buzz(ctx)
	assert.NoError(err)
	assert.NotNil(hb.mObj) // unchanged
	assert.Equal(hb.mObj, clCrud.InNodeUpdateObj)
	assert.Equal([]string{"service.state", "service.heartbeatTime", "service.heartbeatPeriodSecs"}, clCrud.InNodeUpdateItems.Set)
	assert.Empty(clCrud.InNodeUpdateItems.Append)
	assert.Empty(clCrud.InNodeUpdateItems.Remove)
	assert.NotNil(hb.mObj.Service)
	assert.Equal(2, hb.runCount)
	assert.True(tB.Before(hb.lastHeartbeatTime))
	tl.Flush()

	// heartbeat only is constrained by last time issued
	now = time.Now()
	assert.False(hb.canPostHeartbeat(now))
	clCrud.RetNodeUpdateErr = fmt.Errorf("should-not-be-called")
	assert.NoError(hb.updateNodeHeartbeatOnly(nil))
	assert.False(hb.canPostHeartbeat(now))

	// hearbeat only failure - hearbeat time not updated
	mockCtrl.Finish()
	tl.Logger().Info("hb only: fake loss of connectivity")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	hb.lastHeartbeatTime = time.Time{} // force
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	hb.app.NuvoAPI = nvAPI
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	assert.NotNil(hb.mObj)
	clCrud.InNodeUpdateObj = nil // update will be used
	clCrud.InNodeUpdateItems = nil
	clCrud.RetNodeUpdateObj = nil // will be ignored
	clCrud.RetNodeUpdateErr = fmt.Errorf("update error")
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, gomock.Not(gomock.Nil())).Return(nil, nil).MinTimes(1)
	assert.True(hb.updateCount > 0)
	tB = time.Now()
	err = hb.Buzz(ctx)
	assert.Error(err)
	assert.Regexp("update error", err)
	assert.NotNil(hb.mObj)
	assert.EqualValues(hb.mObj, clCrud.InNodeUpdateObj)
	assert.Equal([]string{"service.state", "service.heartbeatTime", "service.heartbeatPeriodSecs"}, clCrud.InNodeUpdateItems.Set)
	assert.Empty(clCrud.InNodeUpdateItems.Append)
	assert.Empty(clCrud.InNodeUpdateItems.Remove)
	assert.NotNil(hb.mObj.Service)
	assert.False(tB.Before(hb.lastHeartbeatTime))
	assert.True(hb.updateCount > 0)

	// transient error for system fetch
	mockCtrl.Finish()
	tl.Logger().Info("transient error for system fetch")
	tl.Flush()
	hb.lastHeartbeatTime = time.Time{} // force
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	hb.app.NuvoAPI = nvAPI
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	hb.systemID = ""
	sysOps = mockmgmtclient.NewMockSystemClient(mockCtrl)
	sErr := errors.New("bad gateway")
	sysOps.EXPECT().SystemFetch(gomock.Not(gomock.Nil())).Return(nil, sErr)
	mAPI.EXPECT().System().Return(sysOps)
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordIncident(ctx,
		&cluster.Incident{Message: sErr.Error(),
			Severity: cluster.IncidentFatal}).Return(nil, nil).MinTimes(1)
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	asm := newAppServantMatcher(t, "bad gateway")
	mServant.EXPECT().FatalError(asm).MinTimes(1)
	assert.Equal(0, app.FatalErrorCount)
	app.InFatalError = true // don't want to exit
	err = hb.Buzz(ctx)
	assert.Error(err)
	assert.Regexp("bad gateway", err)

	// invalid system id
	mockCtrl.Finish()
	tl.Logger().Info("invalid system id")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	hb.app.NuvoAPI = nvAPI
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	app.SystemID = string(sysObj.Meta.ID) + "foo"
	hb.systemID = ""
	hb.fatalError = false
	sysOps = mockmgmtclient.NewMockSystemClient(mockCtrl)
	sysOps.EXPECT().SystemFetch(gomock.Not(gomock.Nil())).Return(&system.SystemFetchOK{Payload: sysObj}, nil)
	mAPI.EXPECT().System().Return(sysOps)
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordIncident(ctx,
		&cluster.Incident{Message: errSystemMismatch.Error(),
			Severity: cluster.IncidentFatal}).Return(nil, nil).MinTimes(1)
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	asm = newAppServantMatcher(t, errSystemMismatch.Error())
	mServant.EXPECT().FatalError(asm).MinTimes(1)
	assert.Equal(0, app.FatalErrorCount)
	app.InFatalError = true // don't want to exit
	err = hb.Buzz(ctx)
	assert.Error(err)
	assert.Equal(errSystemMismatch, err)
	assert.True(hb.fatalError)
	hb.systemID = string(sysObj.Meta.ID) // restore
	tl.Flush()

	// cluster mismatch error cases
	for _, tc := range []string{"CspDomainID", "ClusterType", "ClusterIdentifier"} {
		mockCtrl.Finish()
		tl.Logger().Info("Cluster mismatch error", tc)
		tl.Flush()
		mockCtrl = gomock.NewController(t)
		hb.fatalError = false
		app.InFatalError = true // don't want to exit
		mCluster = mockcluster.NewMockClient(mockCtrl)
		app.ClusterClient = mCluster
		mCluster.EXPECT().RecordIncident(ctx,
			&cluster.Incident{Message: errClusterMismatch.Error(),
				Severity: cluster.IncidentFatal}).Return(nil, nil).MinTimes(1)
		mServant = NewMockAppServant(mockCtrl)
		hb.app.AppServant = mServant
		asm = newAppServantMatcher(t, errClusterMismatch.Error())
		mServant.EXPECT().FatalError(asm).MinTimes(1)
		var clObj *models.Cluster
		testutils.Clone(mObj, &clObj)
		switch tc {
		case "CspDomainID":
			clObj.CspDomainID = "foo"
		case "ClusterType":
			clObj.ClusterType = "foo"
		case "ClusterIdentifier":
			clObj.ClusterIdentifier = "foo"
		}
		hb.fatalError = false
		hb.mClusterObj = nil
		hb.clusterID = ""
		var mismatchedCluster *models.Cluster
		testutils.Clone(clObj, &mismatchedCluster)
		fCrud.RetLClObj = mismatchedCluster
		fCrud.RetLClErr = nil
		err = hb.Buzz(ctx)
		assert.Equal(errClusterMismatch, err)
	}

	// cluster not found error
	mockCtrl.Finish()
	tl.Logger().Info("Cluster not found error")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	app.InFatalError = true // don't want to exit
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordIncident(ctx,
		&cluster.Incident{Message: errClusterNotFound.Error(),
			Severity: cluster.IncidentFatal}).Return(nil, nil).MinTimes(1)
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	asm = newAppServantMatcher(t, errClusterNotFound.Error())
	mServant.EXPECT().FatalError(asm).MinTimes(1)
	hb.fatalError = false
	hb.mClusterObj = nil
	hb.clusterID = ""
	fCrud.RetLClObj = nil
	fCrud.RetLClErr = &crud.Error{Payload: models.Error{Code: http.StatusNotFound, Message: swag.String(com.ErrorNotFound)}}
	err = hb.Buzz(ctx)
	assert.Equal(errClusterNotFound, err)

	// error calling LocalEphemeralDevices
	tl.Logger().Info("LocalEphemeralDevices error")
	tl.Flush()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	hb.systemID = "set"
	hb.mClusterObj = clObj
	hb.clusterID = string(clObj.Meta.ID)
	hb.ephemeralDevices = nil
	hb.fatalError = false
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	hb.app.NuvoAPI = nvAPI
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	assert.NotNil(hb.mObj)
	domClient = mockCSP.NewMockDomainClient(mockCtrl)
	app.CSPClient = domClient
	domClient.EXPECT().LocalEphemeralDevices().Return(nilDevices, errors.New("LocalEphemeralDevices error"))
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil).MinTimes(1)
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	err = hb.Buzz(ctx)
	assert.Regexp("LocalEphemeralDevices error", err)

	// error from initializeNuvo, cover LocalEphemeralDevices returns non-nil list
	tl.Flush()
	mockCtrl.Finish()
	tl.Logger().Info("initialize Nuvo error")
	tl.Flush()
	hb.lastHeartbeatTime = time.Time{} // force
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	hb.app.NuvoAPI = nvAPI
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	assert.NotNil(hb.mObj)
	hb.ephemeralDevices = nil
	domClient = mockCSP.NewMockDomainClient(mockCtrl)
	app.CSPClient = domClient
	domClient.EXPECT().LocalEphemeralDevices().Return([]*csp.EphemeralDevice{}, nil)
	clCrud.InNodeUpdateObj = nil // update will be used
	clCrud.InNodeUpdateItems = nil
	clCrud.RetNodeUpdateObj = nil // will be ignored
	clCrud.RetNodeUpdateErr = nil
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil)
	mServant = NewMockAppServant(mockCtrl)
	app.AppServant = mServant
	uErr := errors.New("nuvo uuid error")
	mServant.EXPECT().InitializeNuvo(gomock.Any()).Return(uErr)
	hb.systemID = "set"
	hb.nuvoNodeUUIDSet = false
	hb.fatalError = false
	err = hb.Buzz(ctx)
	assert.Error(err)
	assert.Regexp("nuvo uuid error", err)
	assert.False(hb.nuvoNodeUUIDSet)
}

func TestNodeFoundOnStart(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log:                   tl.Logger(),
			HeartbeatPeriod:       10,
			NuvoStartupPollPeriod: 5,
			CSPDomainID:           "6a156de2-ba94-4035-b47f-e87373d75757",
			ClusterType:           "kubernetes",
		},
		InstanceMD: fakeNodeMD,
		ClusterMD:  fakeClusterMD,
	}
	eM := &fev.Manager{}
	app.CrudeOps = eM
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	clCrud := &fakecrud.Client{}
	app.ClusterdOCrud = clCrud
	svcArgs := util.ServiceArgs{
		ServiceType:         "agentd",
		ServiceVersion:      "test",
		HeartbeatPeriodSecs: int64(app.HeartbeatPeriod),
		Log:                 app.Log,
		MaxMessages:         app.MaxMessages,
		ServiceAttributes:   map[string]models.ValueType{},
	}
	app.Service = util.NewService(&svcArgs)
	hb := newHBComp()
	now := time.Now()
	prevMessages := make([]*models.TimestampedString, 1)
	prevMessages[0] = &models.TimestampedString{
		Time:    strfmt.DateTime(now.Add(-2 * time.Hour)),
		Message: "previous message",
	}
	mCSPObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID:           models.ObjID(app.CSPDomainID),
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
			CspDomainType: models.CspDomainTypeMutable(app.CSPDomainType),
			CspDomainAttributes: map[string]models.ValueType{
				csp.IMDZone: models.ValueType{Kind: "STRING", Value: "zone"},
			},
		},
		CSPDomainMutable: models.CSPDomainMutable{
			Name: "CSPDomain1",
		},
	}
	mObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID:           "NODE-1",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
			NodeIdentifier: fakeNodeMD[csp.IMDInstanceName],
			ClusterID:      models.ObjIDMutable("CLUSTER-1"),
		},
		NodeMutable: models.NodeMutable{
			Service: &models.NuvoService{
				NuvoServiceAllOf0: models.NuvoServiceAllOf0{
					ServiceType:       svcArgs.ServiceType,
					ServiceAttributes: svcArgs.ServiceAttributes,
					ServiceVersion:    svcArgs.ServiceVersion,
					Messages:          prevMessages,
				},
				ServiceState: models.ServiceState{
					HeartbeatPeriodSecs: svcArgs.HeartbeatPeriodSecs,
				},
			},
			State: com.NodeStateManaged,
		},
	}
	var err error
	ctx := context.Background()

	// fake metricMover status
	fMM.RetStatus = &metricmover.Status{
		TxConfigured:      true,
		TxStarted:         true,
		VolumeIOBuffered:  10,
		VolumeIOPublished: 10,
	}
	mmStatusString := fMM.RetStatus.String()

	// Init
	hb.Init(app)
	hb.nuvoNodeUUIDSet = true // tested separately
	assert.Equal("", hb.nodeIdentifier)
	assert.Equal(0, hb.runCount)

	// replace the worker
	tw := &fw.Worker{}
	hb.worker = tw

	mObj.NodeAttributes = hb.nodeAttrs()

	// This test fakes out Start/Stop
	hb.ephemeralDevices = []*csp.EphemeralDevice{}
	hb.nodeIdentifier = hb.app.InstanceMD[csp.IMDInstanceName]
	hb.clusterID = string(mObj.ClusterID)
	hb.systemID = "system-1"
	hb.runCount = 0
	hb.createdUpMon = true
	// skip initial csp domain fetch
	hb.mCspDomainObj = mCSPObj

	// failure case: found node being torn down
	var tdObj *models.Node
	testutils.Clone(mObj, &tdObj)
	tdObj.State = com.NodeStateTearDown
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	hb.lastHeartbeatTime = time.Time{} // force
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	assert.Nil(hb.mObj)
	clCrud.InLsNObj = nil
	clCrud.RetLsNObj = &mn.NodeListOK{Payload: []*models.Node{tdObj}}
	clCrud.RetLsNErr = nil
	mCluster := mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil).MinTimes(1)
	assert.Nil(hb.mObj)
	assert.Zero(hb.updateCount)
	err = hb.Buzz(ctx)
	assert.Error(err)
	assert.Regexp("node is being torn down", err)
	assert.Nil(hb.mObj)

	// success
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	tl.Logger().Info("success")
	tl.Flush()
	hb.lastHeartbeatTime = time.Time{} // force
	nvAPI := mockNuvo.NewMockNuvoVM(mockCtrl)
	hb.app.NuvoAPI = nvAPI
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	assert.Nil(hb.mObj)
	clCrud.InLsNObj = nil
	clCrud.RetLsNObj = &mn.NodeListOK{Payload: []*models.Node{mObj}}
	clCrud.RetLsNErr = nil
	clCrud.RetNodeUpdateObj = mObj
	clCrud.RetNodeUpdateErr = nil
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceReady}).Return(nil, nil).MinTimes(1)
	assert.Nil(hb.mObj)
	nvAPI.EXPECT().NodeStatus().Return(nuvoapi.NodeStatusReport{NodeUUID: "uuid"}, nil)
	assert.Zero(hb.updateCount)
	err = hb.Buzz(ctx)
	assert.NoError(err)
	assert.NotNil(hb.mObj)
	assert.Equal(mObj, hb.mObj)
	assert.True(hb.updateCount > 0)
	expNLP := mn.NewNodeListParams()
	expNLP.NodeIdentifier = &hb.nodeIdentifier
	expNLP.ClusterID = &hb.clusterID
	expNLP.Context = clCrud.InLsNObj.Context
	assert.Equal(expNLP, clCrud.InLsNObj)
	assert.Equal(hb.mObj, clCrud.InNodeUpdateObj)
	assert.Equal([]string{"state", "service", "nodeAttributes", "localStorage", "totalCacheBytes", "cacheUnitSizeBytes"}, clCrud.InNodeUpdateItems.Set)
	assert.Empty(clCrud.InNodeUpdateItems.Append)
	assert.Empty(clCrud.InNodeUpdateItems.Remove)
	assert.NotNil(hb.mObj.Service)
	assert.NotNil(hb.mObj.NodeAttributes)
	vtMS := hb.app.Service.GetServiceAttribute(com.ServiceAttrMetricMoverStatus)
	assert.NotNil(vtMS)
	assert.Equal("STRING", vtMS.Kind)
	assert.Equal(mmStatusString, vtMS.Value)
	assert.NotZero(hb.sleepPeriod)
	assert.Equal(hb.sleepPeriod, tw.InSSI)

	// previous messages must be imported
	tl.Logger().Info("previous messages must be imported")
	tl.Logger().Debug(hb.mObj.Service.ServiceState)
	tl.Flush()
	assert.NotNil(mObj.Service)
	assert.NotNil(mObj.Service.ServiceState)
	messages := mObj.Service.Messages
	assert.True(len(messages) > 0)
	found := false
	for _, m := range messages {
		tl.Logger().Debug(m)
		if m.Message == "previous message" {
			found = true
		}
	}
	assert.True(found)

	// validate Upstream args
	tl.Logger().Info("validate upstream args")
	tl.Flush()
	wa := hb.getUpstreamMonitorArgs()
	assert.NotNil(wa)
	assert.Len(wa.Matchers, 3)
	m0 := wa.Matchers[0]
	assert.NotEmpty(m0.MethodPattern)
	tl.Logger().Debugf("0.MethodPattern: %s", m0.MethodPattern)
	re := regexp.MustCompile(m0.MethodPattern)
	assert.True(re.MatchString("POST"))
	assert.True(re.MatchString("PATCH"))
	assert.NotEmpty(m0.URIPattern)
	tl.Logger().Debugf("0.URIPattern: %s", m0.URIPattern)
	re = regexp.MustCompile(m0.URIPattern)
	assert.True(re.MatchString("/volume-series-requests"))
	assert.True(re.MatchString("/volume-series-requests/id"))
	assert.True(re.MatchString("/storage-requests"))
	assert.True(re.MatchString("/storage-requests/id"))
	assert.NotEmpty(m0.ScopePattern)
	tl.Logger().Debugf("0.ScopePattern: %s", m0.ScopePattern)
	re = regexp.MustCompile(m0.ScopePattern)
	scopeS := fmt.Sprintf("abc:def nodeId:%s xyz:123", string(hb.mObj.Meta.ID))
	tl.Logger().Debugf("*** scopeS: %s", scopeS)
	assert.True(re.MatchString(scopeS))
	m1 := wa.Matchers[1]
	assert.NotEmpty(m1.MethodPattern)
	tl.Logger().Debugf("1.MethodPattern: %s", m1.MethodPattern)
	re = regexp.MustCompile(m1.MethodPattern)
	assert.True(re.MatchString("PATCH"))
	assert.NotEmpty(m1.URIPattern)
	tl.Logger().Debugf("1.URIPattern: %s", m1.URIPattern)
	re = regexp.MustCompile(m1.URIPattern)
	assert.True(re.MatchString("/volume-series-requests/id"))
	assert.NotEmpty(m1.ScopePattern)
	tl.Logger().Debugf("1.ScopePattern: %s", m1.ScopePattern)
	re = regexp.MustCompile(m1.ScopePattern)
	scopeS = fmt.Sprintf("abc:def nodeId:foo syncCoordinatorId:something xyz:123")
	tl.Logger().Debugf("*** scopeS: %s", scopeS)
	assert.True(re.MatchString(scopeS))
	m2 := wa.Matchers[2]
	assert.NotEmpty(m2.MethodPattern)
	tl.Logger().Debugf("2.MethodPattern: %s", m2.MethodPattern)
	re = regexp.MustCompile(m2.MethodPattern)
	assert.True(re.MatchString("POST"))
	assert.NotEmpty(m2.URIPattern)
	tl.Logger().Debugf("2.URIPattern: %s", m2.URIPattern)
	re = regexp.MustCompile(m2.URIPattern)
	assert.True(re.MatchString("/volume-series-requests/id/cancel"))
	assert.Empty(m2.ScopePattern)

	// test upstream monitor failure cases
	// MonitorUpstream error
	mockCtrl.Finish()
	tl.Logger().Info("upstream monitor failure cases")
	tl.Flush()
	hb.lastHeartbeatTime = time.Time{} // force
	hb.createdUpMon = false
	eM.RetMUErr = fmt.Errorf("upstream-monitor-error")
	eM.CalledMU = false
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	hb.lastHeartbeatTime = time.Time{} // force
	hb.app.NuvoAPI = nvAPI
	mServant := NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	mServant.EXPECT().GetClusterdAPIArgs().Return(nil)
	mServant.EXPECT().GetClusterdAPI().Return(nil, nil)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	assert.NotNil(hb.mObj)
	hb.mObj.Service = nil
	hb.mObj.NodeAttributes = nil
	clCrud.RetNodeUpdateObj = hb.mObj
	clCrud.RetNodeUpdateErr = nil

	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil)
	err = hb.Buzz(ctx)
	assert.Error(err)
	assert.False(hb.createdUpMon)
	assert.True(eM.CalledMU)
	assert.Regexp("upstream-monitor-error", err)
	assert.NotNil(hb.mObj)
	assert.Equal(hb.mObj, clCrud.InNodeUpdateObj)

	// GetClusterdAPI error
	mockCtrl.Finish()
	tl.Logger().Info("cluster api error")
	tl.Flush()
	hb.createdUpMon = false
	eM.CalledMU = false
	hb.lastHeartbeatTime = time.Time{} // force
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	hb.app.NuvoAPI = nvAPI
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	mServant.EXPECT().GetClusterdAPIArgs().Return(nil)
	mServant.EXPECT().GetClusterdAPI().Return(nil, fmt.Errorf("clusterd-api-error"))
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	assert.NotNil(hb.mObj)
	hb.mObj.Service = nil
	hb.mObj.NodeAttributes = nil
	clCrud.RetNodeUpdateObj = hb.mObj
	clCrud.RetNodeUpdateErr = nil

	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil)
	err = hb.Buzz(ctx)
	assert.Error(err)
	assert.False(hb.createdUpMon)
	assert.False(eM.CalledMU)
	assert.Regexp("clusterd-api-error", err)
	assert.NotNil(hb.mObj)
	assert.Equal(hb.mObj, clCrud.InNodeUpdateObj)
}

func TestNodeCreationRaceOnStart(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log:                   tl.Logger(),
			HeartbeatPeriod:       10,
			NuvoStartupPollPeriod: 5,
			CSPDomainID:           "6a156de2-ba94-4035-b47f-e87373d75757",
			ClusterType:           "kubernetes",
		},
		InstanceMD: fakeNodeMD,
		ClusterMD:  fakeClusterMD,
	}
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	eM := &fev.Manager{}
	app.CrudeOps = eM
	clCrud := &fakecrud.Client{}
	app.ClusterdOCrud = clCrud
	svcArgs := util.ServiceArgs{
		ServiceType:         "agentd",
		ServiceVersion:      "test",
		HeartbeatPeriodSecs: int64(app.HeartbeatPeriod),
		Log:                 app.Log,
		MaxMessages:         app.MaxMessages,
		ServiceAttributes:   map[string]models.ValueType{},
	}
	app.Service = util.NewService(&svcArgs)
	hb := newHBComp()
	now := time.Now()
	mCSPObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID:           models.ObjID(app.CSPDomainID),
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
			CspDomainType: models.CspDomainTypeMutable(app.CSPDomainType),
			CspDomainAttributes: map[string]models.ValueType{
				csp.IMDZone: models.ValueType{Kind: "STRING", Value: "zone"},
			},
		},
		CSPDomainMutable: models.CSPDomainMutable{
			Name: "CSPDomain1",
		},
	}
	mObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID:           "NODE-1",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
			NodeIdentifier: fakeNodeMD[csp.IMDInstanceName],
			ClusterID:      models.ObjIDMutable("CLUSTER-1"),
		},
		NodeMutable: models.NodeMutable{
			//Service: get this data from the service after Init
			Name: models.ObjName(fakeNodeMD[csp.IMDInstanceName]),
			//NodeAttributes: get this after init from hb
		},
	}
	var err error
	ctx := context.Background()

	// Init
	hb.Init(app)
	hb.nuvoNodeUUIDSet = true // tested separately
	assert.Equal("", hb.nodeIdentifier)
	assert.Equal(0, hb.runCount)

	mObj.NodeAttributes = hb.nodeAttrs()
	mObj.Service = app.Service.ModelObj()
	assert.NotNil(mObj.Service)

	// This test fakes out Start/Stop
	hb.ephemeralDevices = []*csp.EphemeralDevice{}
	hb.nodeIdentifier = hb.app.InstanceMD[csp.IMDInstanceName]
	hb.clusterID = "CLUSTER-1"
	hb.systemID = "system-1"
	hb.runCount = 0
	hb.createdUpMon = true
	// skip initial csp domain fetch
	hb.mCspDomainObj = mCSPObj

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	nvAPI := mockNuvo.NewMockNuvoVM(mockCtrl)
	hb.app.NuvoAPI = nvAPI
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	clCrud.InLsNObj = nil
	// first list doesn't find it
	clCrud.RetLsNObj = &mn.NodeListOK{}
	// second list succeeds
	clCrud.FutureLsNObj = []*mn.NodeListOK{&mn.NodeListOK{Payload: []*models.Node{mObj}}}
	clCrud.RetLsNErr = nil

	err = crud.NewError(&models.Error{Code: 409, Message: swag.String(com.ErrorExists)})
	e, ok := err.(*crud.Error)
	assert.True(ok)
	assert.True(e.Exists())
	clCrud.RetNodeCreateObj = nil
	clCrud.RetNodeCreateErr = err
	mObj.Service.ServiceState.State = app.Service.StateString(app.Service.GetState())
	mCluster := mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceReady}).Return(nil, nil).MinTimes(1)
	assert.Nil(hb.mObj)
	nvAPI.EXPECT().NodeStatus().Return(nuvoapi.NodeStatusReport{NodeUUID: "uuid"}, nil)
	assert.Zero(hb.updateCount)
	err = hb.Buzz(ctx)
	assert.NoError(err)
	assert.NotNil(hb.mObj)
	assert.Equal(mObj, hb.mObj)
	assert.Zero(hb.updateCount)
	expNLP := mn.NewNodeListParams()
	expNLP.NodeIdentifier = &hb.nodeIdentifier
	expNLP.ClusterID = &hb.clusterID
	expNLP.Context = clCrud.InLsNObj.Context
	assert.Equal(expNLP, clCrud.InLsNObj)
}

func TestNodeCreationMultipleErrors(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log:                   tl.Logger(),
			HeartbeatPeriod:       10,
			NuvoStartupPollPeriod: 5,
			CSPDomainID:           "6a156de2-ba94-4035-b47f-e87373d75757",
			ClusterType:           "kubernetes",
		},
		InstanceMD: fakeNodeMD,
		ClusterMD:  fakeClusterMD,
	}
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	eM := &fev.Manager{}
	app.CrudeOps = eM
	clCrud := &fakecrud.Client{}
	app.ClusterdOCrud = clCrud
	svcArgs := util.ServiceArgs{
		ServiceType:         "agentd",
		ServiceVersion:      "test",
		HeartbeatPeriodSecs: int64(app.HeartbeatPeriod),
		Log:                 app.Log,
		MaxMessages:         app.MaxMessages,
		ServiceAttributes:   map[string]models.ValueType{},
	}
	app.Service = util.NewService(&svcArgs)
	hb := newHBComp()
	now := time.Now()
	mCSPObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID:           models.ObjID(app.CSPDomainID),
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
			CspDomainType: models.CspDomainTypeMutable(app.CSPDomainType),
			CspDomainAttributes: map[string]models.ValueType{
				csp.IMDZone: models.ValueType{Kind: "STRING", Value: "zone"},
			},
		},
		CSPDomainMutable: models.CSPDomainMutable{
			Name: "CSPDomain1",
		},
	}
	mObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID:           "NODE-1",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
			NodeIdentifier: fakeNodeMD[csp.IMDInstanceName],
			ClusterID:      models.ObjIDMutable("CLUSTER-1"),
		},
		NodeMutable: models.NodeMutable{
			//Service: get this data from the service after Init
			Name: models.ObjName(fakeNodeMD[csp.IMDInstanceName]),
			//NodeAttributes: get this after init from hb
			State: com.NodeStateManaged,
		},
	}
	var err error
	ctx := context.Background()

	// Init
	hb.Init(app)
	hb.nuvoNodeUUIDSet = true // tested separately
	assert.Equal("", hb.nodeIdentifier)
	assert.Equal(0, hb.runCount)

	mObj.NodeAttributes = hb.nodeAttrs()
	mObj.Service = app.Service.ModelObj()
	assert.NotNil(mObj.Service)

	// This test fakes out Start/Stop
	hb.ephemeralDevices = []*csp.EphemeralDevice{}
	hb.nodeIdentifier = hb.app.InstanceMD[csp.IMDInstanceName]
	hb.clusterID = "CLUSTER-1"
	hb.systemID = "system-1"
	hb.runCount = 0
	hb.createdUpMon = true
	// skip initial csp domain fetch
	hb.mCspDomainObj = mCSPObj

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	clCrud.InLsNObj = nil
	clCrud.RetLsNObj = &mn.NodeListOK{}
	clCrud.RetLsNErr = nil
	clCrud.RetNodeCreateObj = nil
	clCrud.RetNodeCreateErr = fmt.Errorf("not a 409 error")
	mCluster := mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil).MinTimes(1)
	mObj.Service.ServiceState.State = app.Service.StateString(app.Service.GetState())
	assert.Nil(hb.mObj)
	assert.Equal(0, hb.runCount)
	assert.Zero(hb.updateCount)
	err = hb.Buzz(ctx)
	assert.Error(err)
	assert.Regexp("not a 409 error", err)
	assert.Nil(hb.mObj)
	assert.Zero(hb.updateCount)
	assert.Equal(1, hb.runCount)
	expNLP := mn.NewNodeListParams()
	expNLP.NodeIdentifier = &hb.nodeIdentifier
	expNLP.ClusterID = &hb.clusterID
	expNLP.Context = clCrud.InLsNObj.Context
	assert.Equal(expNLP, clCrud.InLsNObj)
}

func TestNodeObjectDisappears(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log:                   tl.Logger(),
			HeartbeatPeriod:       10,
			NuvoStartupPollPeriod: 5,
			CSPDomainID:           "6a156de2-ba94-4035-b47f-e87373d75757",
			ClusterType:           "kubernetes",
		},
		InstanceMD: fakeNodeMD,
		ClusterMD:  fakeClusterMD,
	}
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	eM := &fev.Manager{}
	app.CrudeOps = eM
	clCrud := &fakecrud.Client{}
	app.ClusterdOCrud = clCrud
	svcArgs := util.ServiceArgs{
		ServiceType:         "agentd",
		ServiceVersion:      "test",
		HeartbeatPeriodSecs: int64(app.HeartbeatPeriod),
		Log:                 app.Log,
		MaxMessages:         app.MaxMessages,
		ServiceAttributes:   map[string]models.ValueType{},
	}
	app.Service = util.NewService(&svcArgs)
	hb := newHBComp()
	now := time.Now()
	mCSPObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID:           models.ObjID(app.CSPDomainID),
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
			CspDomainType: models.CspDomainTypeMutable(app.CSPDomainType),
			CspDomainAttributes: map[string]models.ValueType{
				csp.IMDZone: models.ValueType{Kind: "STRING", Value: "zone"},
			},
		},
		CSPDomainMutable: models.CSPDomainMutable{
			Name: "CSPDomain1",
		},
	}
	mObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID:           "NODE-1",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
			NodeIdentifier: fakeNodeMD[csp.IMDInstanceName],
			ClusterID:      models.ObjIDMutable("CLUSTER-1"),
		},
		NodeMutable: models.NodeMutable{
			//Service: get this data from the service after Init
			Name: models.ObjName(fakeNodeMD[csp.IMDInstanceName]),
			//NodeAttributes: get this after init from hb
		},
	}
	var err error
	ctx := context.Background()

	// Init
	hb.Init(app)
	hb.nuvoNodeUUIDSet = true // tested separately
	assert.Equal("", hb.nodeIdentifier)
	assert.Equal(0, hb.runCount)

	mObj.NodeAttributes = hb.nodeAttrs()
	mObj.Service = app.Service.ModelObj()
	assert.NotNil(mObj.Service)

	// This test fakes out Start/Stop
	hb.ephemeralDevices = []*csp.EphemeralDevice{}
	hb.nodeIdentifier = hb.app.InstanceMD[csp.IMDInstanceName]
	hb.clusterID = "CLUSTER-1"
	hb.runCount = 0
	hb.createdUpMon = true

	// Initialize the scenario by assuming a node object exists
	hb.mObj = mObj
	hb.updateCount = 1
	// skip initial csp domain fetch
	hb.mCspDomainObj = mCSPObj

	// On update return a 404 - the in-memory object should be zapped and the error recorded
	assert.NotNil(hb.mObj)
	hb.mObj.Service = nil
	hb.mObj.NodeAttributes = nil
	clCrud.RetNodeUpdateObj = nil
	err = crud.NewError(&models.Error{Code: 404, Message: swag.String(com.ErrorNotFound)})
	e, ok := err.(*crud.Error)
	assert.True(ok)
	assert.True(e.NotFound())
	clCrud.RetNodeUpdateErr = e
	clCrud.InNodeUpdateItems = nil
	assert.NotNil(hb.mObj)
	// indirectly call updateNodeObj (forced)
	tA := time.Now()
	err = hb.UpdateState(ctx)
	tl.Flush()
	assert.NotNil(err)
	assert.NotNil(clCrud.InNodeUpdateItems)
	assert.Equal([]string{"state", "service", "nodeAttributes"}, clCrud.InNodeUpdateItems.Set)
	assert.Empty(clCrud.InNodeUpdateItems.Append)
	assert.Empty(clCrud.InNodeUpdateItems.Remove)
	assert.Nil(hb.mObj)
	assert.Zero(hb.updateCount) // cleared
	assert.Regexp("node.*object deleted.*ID="+string(mObj.Meta.ID), err.Error())
	ncc := app.Service.ModelObj()
	assert.NotNil(ncc)
	assert.NotNil(ncc.ServiceState)
	Messages := ncc.Messages
	assert.True(len(Messages) > 0)
	assert.Regexp("Node.*object deleted.*ID="+string(mObj.Meta.ID), Messages[len(Messages)-1].Message)
	assert.True(tA.After(hb.lastHeartbeatTime)) // because of failure
	tl.Flush()

	// updateNodeObj will clear updateCount on any type of error
	hb.mObj = mObj
	hb.updateCount = 1
	clCrud.RetNodeUpdateObj = nil
	clCrud.RetNodeUpdateErr = fmt.Errorf("other-error")
	clCrud.InNodeUpdateItems = nil
	err = hb.UpdateState(ctx)
	assert.Error(err)
	assert.Regexp("other-error", err)
	assert.Zero(hb.updateCount) // cleared
	assert.NotNil(clCrud.InNodeUpdateItems)

	// updateNodeObj will not fire again until the sleepPeriod expires
	hb.lastHeartbeatTime = time.Now()
	err = hb.updateNodeObj(ctx)
	assert.NoError(err)
	tl.Flush()

	// UpdateState will fail if mObj is nil
	hb.mObj = nil
	err = hb.UpdateState(ctx)
	assert.Error(err)
	assert.Regexp("not ready", err)
}

// DEPRECATED
func TestWaitForClusterToAppear(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log:                   tl.Logger(),
			HeartbeatPeriod:       10,
			NuvoStartupPollPeriod: 5,
			NuvoStartupPollMax:    5,
			CSPDomainID:           "6a156de2-ba94-4035-b47f-e87373d75757",
			ClusterType:           "kubernetes",
		},
		InstanceMD: fakeNodeMD,
		ClusterMD:  fakeClusterMD,
	}
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	eM := &fev.Manager{}
	app.CrudeOps = eM
	clCrud := &fakecrud.Client{}
	app.ClusterdOCrud = clCrud
	svcArgs := util.ServiceArgs{
		ServiceType:         "agentd",
		ServiceVersion:      "test",
		HeartbeatPeriodSecs: int64(app.HeartbeatPeriod),
		Log:                 app.Log,
		MaxMessages:         app.MaxMessages,
		ServiceAttributes:   map[string]models.ValueType{},
	}
	app.Service = util.NewService(&svcArgs)
	hb := newHBComp()
	now := time.Now()
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID:           "CLUSTER-1",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
		},
		ClusterCreateOnce: models.ClusterCreateOnce{
			ClusterType: app.ClusterType,
			CspDomainID: models.ObjIDMutable(app.CSPDomainID),
		},
		ClusterMutable: models.ClusterMutable{
			ClusterCreateMutable: models.ClusterCreateMutable{
				ClusterIdentifier: fakeClusterMD[cluster.CMDIdentifier],
			},
		},
	}
	mCSPObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID:           models.ObjID(app.CSPDomainID),
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
			CspDomainType: models.CspDomainTypeMutable(app.CSPDomainType),
			CspDomainAttributes: map[string]models.ValueType{
				csp.IMDZone: models.ValueType{Kind: "STRING", Value: "zone"},
			},
		},
		CSPDomainMutable: models.CSPDomainMutable{
			Name: "CSPDomain1",
		},
	}
	mObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID:           "NODE-1",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
			NodeIdentifier: fakeNodeMD[csp.IMDInstanceName],
			ClusterID:      models.ObjIDMutable("CLUSTER-1"),
		},
		NodeMutable: models.NodeMutable{
			//Service: get this data from the service after Init
			Name: models.ObjName(fakeNodeMD[csp.IMDHostname]),
			//NodeAttributes: get this after init from hb
		},
	}
	var err error
	ctx := context.Background()

	// Init
	hb.Init(app)
	hb.nuvoNodeUUIDSet = true // tested separately
	assert.Equal("", hb.nodeIdentifier)
	assert.Equal(0, hb.runCount)
	assert.NotZero(int(hb.sleepPeriod))
	assert.NotZero(int(hb.nuvoPollPeriod))
	assert.NotEqual(hb.nuvoPollPeriod, hb.sleepPeriod)
	assert.NotZero(hb.nuvoPollMaxCount)

	// replace the worker
	tw := &fw.Worker{}
	hb.worker = tw
	tw.RetGSI = hb.sleepPeriod
	tw.InSSI = 0

	mObj.NodeAttributes = hb.nodeAttrs()
	mObj.Service = app.Service.ModelObj()
	assert.NotNil(mObj.Service)

	// This test fakes out Start/Stop to avoid racy scheduling issues
	hb.ephemeralDevices = []*csp.EphemeralDevice{}
	hb.nodeIdentifier = hb.app.InstanceMD[csp.IMDInstanceName]
	hb.systemID = "system-1"
	hb.runCount = 0
	hb.createdUpMon = true
	// skip initial csp domain fetch
	hb.mCspDomainObj = mCSPObj

	// cluster not found
	tl.Logger().Info("case: cluster not found")
	tl.Flush()
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	nvAPI := mockNuvo.NewMockNuvoVM(mockCtrl)
	hb.app.NuvoAPI = nvAPI
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	m2API := mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClusterdAPI = m2API
	clOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps).MinTimes(1)
	clM := mockmgmtclient.NewClusterMatcher(t, mcl.NewClusterListParams())
	clM.ListParam.ClusterIdentifier = &hb.clusterIdentifier
	clM.ListParam.CspDomainID = &app.CSPDomainID
	clOps.EXPECT().ClusterList(clM).Return(&mcl.ClusterListOK{Payload: []*models.Cluster{}}, nil).MinTimes(1)
	mCluster := mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil)
	assert.Equal("", hb.clusterID)
	assert.Nil(hb.mObj)
	assert.Zero(hb.updateCount)
	err = hb.Buzz(ctx)
	assert.Error(err)
	assert.Regexp("cluster not found", err)
	assert.True(hb.runCount == 1)
	assert.Equal("", hb.clusterID)
	assert.Nil(hb.mObj)
	assert.Zero(hb.updateCount)

	// fake error in cluster list
	mockCtrl.Finish()
	tl.Logger().Info("case: cluster list error")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	hb.app.NuvoAPI = nvAPI
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	m2API = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClusterdAPI = m2API
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps).MinTimes(1)
	clM = mockmgmtclient.NewClusterMatcher(t, mcl.NewClusterListParams())
	clM.ListParam.ClusterIdentifier = &hb.clusterIdentifier
	clM.ListParam.CspDomainID = &app.CSPDomainID
	clOps.EXPECT().ClusterList(clM).Return(nil, errors.New("fake error")).MinTimes(1)
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil)
	err = hb.Buzz(ctx)
	assert.Error(err)
	assert.True(hb.runCount == 2)
	assert.Regexp("fake error", err)
	assert.Equal("", hb.clusterID)
	assert.Nil(hb.mObj)
	assert.Zero(hb.updateCount)

	// cluster is found but proxy error in node list
	mockCtrl.Finish()
	tl.Logger().Info("case: node list proxy error")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	hb.app.NuvoAPI = nvAPI
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps).MinTimes(1)
	clM = mockmgmtclient.NewClusterMatcher(t, mcl.NewClusterListParams())
	clM.ListParam.ClusterIdentifier = &hb.clusterIdentifier
	clM.ListParam.CspDomainID = &app.CSPDomainID
	clOps.EXPECT().ClusterList(clM).Return(&mcl.ClusterListOK{Payload: []*models.Cluster{clObj}}, nil).MinTimes(1)
	clCrud.InLsNObj = nil
	clCrud.RetLsNObj = nil
	clCrud.RetLsNErr = &crud.Error{Payload: models.Error{Code: http.StatusServiceUnavailable, Message: swag.String("unavailable")}}
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil).MinTimes(1)
	tw.InSSI = 0
	tw.RetGSI = hb.sleepPeriod
	hb.nuvoPollCount = 0
	err = hb.Buzz(ctx)
	assert.Error(err)
	assert.Regexp("unavailable", err)
	assert.True(hb.runCount == 3)
	assert.Equal("CLUSTER-1", hb.clusterID)
	assert.Nil(hb.mObj)
	assert.Zero(hb.updateCount)
	expNLP := mn.NewNodeListParams()
	expNLP.NodeIdentifier = &hb.nodeIdentifier
	expNLP.ClusterID = &hb.clusterID
	expNLP.Context = clCrud.InLsNObj.Context
	assert.Equal(expNLP, clCrud.InLsNObj)
	assert.Equal(1, hb.nuvoPollCount)
	assert.Equal(1, tl.CountPattern(fmt.Sprintf(" \\(%d\\)", http.StatusServiceUnavailable)))
	assert.Equal(hb.nuvoPollPeriod, tw.InSSI)

	// cluster is found but other error in node list
	mockCtrl.Finish()
	tl.Logger().Info("case: node list other error")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	hb.app.NuvoAPI = nvAPI
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	clOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps).MinTimes(1)
	clM = mockmgmtclient.NewClusterMatcher(t, mcl.NewClusterListParams())
	clM.ListParam.ClusterIdentifier = &hb.clusterIdentifier
	clM.ListParam.CspDomainID = &app.CSPDomainID
	clOps.EXPECT().ClusterList(clM).Return(&mcl.ClusterListOK{Payload: []*models.Cluster{clObj}}, nil).MinTimes(1)
	clCrud.InLsNObj = nil
	clCrud.RetLsNObj = nil
	clCrud.RetLsNErr = fmt.Errorf("list error")
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil).MinTimes(1)
	tw.InSSI = 0
	hb.nuvoPollCount = 0
	err = hb.Buzz(ctx)
	assert.Error(err)
	assert.Regexp("list error", err)
	assert.True(hb.runCount == 4)
	assert.Equal("CLUSTER-1", hb.clusterID)
	assert.Nil(hb.mObj)
	assert.Zero(hb.updateCount)
	expNLP = mn.NewNodeListParams()
	expNLP.NodeIdentifier = &hb.nodeIdentifier
	expNLP.ClusterID = &hb.clusterID
	expNLP.Context = clCrud.InLsNObj.Context
	assert.Equal(expNLP, clCrud.InLsNObj)
	assert.Equal(0, hb.nuvoPollCount)
	assert.EqualValues(0, tw.InSSI)

	// on the next iteration node list works and creation works; test for normal sleep interval being set
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	hb.app.NuvoAPI = nvAPI
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	clCrud.InLsNObj = nil
	clCrud.RetLsNObj = &mn.NodeListOK{}
	clCrud.RetLsNErr = nil
	clCrud.RetNodeCreateObj = mObj
	clCrud.RetNodeCreateErr = nil
	mObj.Service.ServiceState.State = app.Service.StateString(app.Service.GetState())
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceReady}).Return(nil, nil).MinTimes(1)
	nvAPI.EXPECT().NodeStatus().Return(nuvoapi.NodeStatusReport{NodeUUID: "uuid"}, nil)
	assert.Zero(hb.updateCount)
	tw.InSSI = 0
	hb.nuvoPollCount = 0
	tw.RetGSI = hb.nuvoPollPeriod
	err = hb.Buzz(ctx)
	assert.NoError(err)
	assert.True(hb.runCount == 5)
	assert.NotNil(hb.mObj)
	assert.Equal(mObj, hb.mObj)
	assert.True(hb.updateCount > 0)
	expNLP = mn.NewNodeListParams()
	expNLP.NodeIdentifier = &hb.nodeIdentifier
	expNLP.ClusterID = &hb.clusterID
	expNLP.Context = clCrud.InLsNObj.Context
	assert.Equal(expNLP, clCrud.InLsNObj)
	assert.EqualValues(mObj.NodeIdentifier, clCrud.InNodeCreateObj.NodeIdentifier)
	assert.EqualValues(mObj.ClusterID, clCrud.InNodeCreateObj.ClusterID)
	assert.EqualValues(mObj.Name, clCrud.InNodeCreateObj.Name)
	expSS := mObj.Service.ServiceState
	ss := clCrud.InNodeCreateObj.Service.ServiceState
	assert.EqualValues(expSS.HeartbeatPeriodSecs, ss.HeartbeatPeriodSecs)
	assert.EqualValues(expSS.State, ss.State)
	assert.WithinDuration(time.Time(expSS.HeartbeatTime), time.Time(ss.HeartbeatTime), time.Second)
	assert.EqualValues(mObj.NodeAttributes, clCrud.InNodeCreateObj.NodeAttributes)
	assert.Equal(0, hb.nuvoPollCount)
	assert.Equal(hb.sleepPeriod, tw.InSSI)
}

func TestFatalErrorCases(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log:                   tl.Logger(),
			HeartbeatPeriod:       10,
			NuvoStartupPollPeriod: 5,
			CSPDomainID:           "6a156de2-ba94-4035-b47f-e87373d75757",
			ClusterType:           "kubernetes",
			ClusterID:             "CLUSTER-1", // from command line flags
		},
		InstanceMD: fakeNodeMD,
		ClusterMD:  fakeClusterMD,
	}
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	eM := &fev.Manager{}
	app.CrudeOps = eM
	fCrud := &fakecrud.Client{}
	app.OCrud = fCrud
	svcArgs := util.ServiceArgs{
		ServiceType:         "agentd",
		ServiceVersion:      "test",
		HeartbeatPeriodSecs: int64(app.HeartbeatPeriod),
		Log:                 app.Log,
		MaxMessages:         app.MaxMessages,
		ServiceAttributes:   map[string]models.ValueType{},
	}
	app.Service = util.NewService(&svcArgs)
	hb := newHBComp()
	now := time.Now()
	sysObj := &models.System{
		SystemAllOf0: models.SystemAllOf0{
			Meta: &models.ObjMeta{ID: "system-1"},
		},
	}
	mCSPObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID:           models.ObjID(app.CSPDomainID),
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
			CspDomainType: models.CspDomainTypeMutable(app.CSPDomainType),
			CspDomainAttributes: map[string]models.ValueType{
				csp.IMDZone: models.ValueType{Kind: "STRING", Value: "zone"},
			},
		},
		CSPDomainMutable: models.CSPDomainMutable{
			Name: "CSPDomain1",
		},
	}
	clusterObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID:           "CLUSTER-1",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
		},
		ClusterCreateOnce: models.ClusterCreateOnce{
			ClusterType: app.ClusterType,
			CspDomainID: models.ObjIDMutable(app.CSPDomainID),
		},
		ClusterMutable: models.ClusterMutable{
			ClusterCreateMutable: models.ClusterCreateMutable{
				ClusterIdentifier: fakeClusterMD[cluster.CMDIdentifier],
			},
		},
	}
	ctx := context.Background()

	// Init
	hb.Init(app)
	hb.nuvoNodeUUIDSet = true // tested separately
	assert.Equal("", hb.nodeIdentifier)
	assert.Equal(0, hb.runCount)

	// This test fakes out Start/Stop to avoid racy scheduling issues
	hb.runCount = 0
	hb.createdUpMon = true

	// system id fetch error
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	t.Log("fail to fetch System")
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	sysOps := mockmgmtclient.NewMockSystemClient(mockCtrl)
	sErr := fmt.Errorf("some-error")
	sysOps.EXPECT().SystemFetch(gomock.Not(gomock.Nil())).Return(nil, sErr)
	mAPI.EXPECT().System().Return(sysOps)
	mCluster := mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordIncident(ctx,
		&cluster.Incident{Message: sErr.Error(),
			Severity: cluster.IncidentFatal}).Return(nil, nil).MinTimes(1)
	mServant := NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	asm := newAppServantMatcher(t, "some-error")
	mServant.EXPECT().FatalError(asm).MinTimes(1)
	err := hb.Buzz(ctx)
	assert.Regexp("some-error", err)
	assert.True(hb.fatalError)
	tl.Flush()

	// system mismatch
	mockCtrl.Finish()
	t.Log("system mismatch")
	hb.fatalError = false
	hb.app.SystemID = string(sysObj.Meta.ID) + "foo"
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	sysOps = mockmgmtclient.NewMockSystemClient(mockCtrl)
	sysOps.EXPECT().SystemFetch(gomock.Not(gomock.Nil())).Return(&system.SystemFetchOK{Payload: sysObj}, nil)
	mAPI.EXPECT().System().Return(sysOps)
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordIncident(ctx,
		&cluster.Incident{Message: errSystemMismatch.Error(),
			Severity: cluster.IncidentFatal}).Return(nil, nil).MinTimes(1)
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	asm = newAppServantMatcher(t, errSystemMismatch.Error())
	mServant.EXPECT().FatalError(asm).MinTimes(1)
	err = hb.Buzz(ctx)
	assert.Equal(errSystemMismatch, err)
	assert.True(hb.fatalError)

	hb.app.SystemID = string(sysObj.Meta.ID)

	// fail to fetch CSPDomain object
	mockCtrl.Finish()
	t.Log("fail to fetch CSPDomain")
	hb.mCspDomainObj = nil
	hb.fatalError = false
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	m2API := mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClusterdAPI = m2API
	cspOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cspOps).MinTimes(1)
	dm := mockmgmtclient.NewCspDomainMatcher(t, mcsp.NewCspDomainFetchParams())
	dm.FetchParam.ID = app.CSPDomainID
	cspOps.EXPECT().CspDomainFetch(dm).Return(nil, errDomainNotFound).MinTimes(1)
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordIncident(ctx,
		&cluster.Incident{Message: errDomainNotFound.Error(),
			Severity: cluster.IncidentFatal}).Return(nil, nil).MinTimes(1)
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	asm = newAppServantMatcher(t, errDomainNotFound.Error())
	mServant.EXPECT().FatalError(asm).MinTimes(1)
	err = hb.Buzz(ctx)
	assert.Equal(errDomainNotFound, err)
	assert.True(hb.fatalError)
	assert.Nil(hb.mCspDomainObj)
	tl.Flush()

	// domain type mismatch
	mockCtrl.Finish()
	t.Log("CSPDomain mismatch")
	hb.mCspDomainObj = nil
	hb.fatalError = false
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	saveDT := mCSPObj.CspDomainType
	mCSPObj.CspDomainType = models.CspDomainTypeMutable("foo")
	cspOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cspOps).MinTimes(1)
	dm = mockmgmtclient.NewCspDomainMatcher(t, mcsp.NewCspDomainFetchParams())
	dm.FetchParam.ID = app.CSPDomainID
	cspOps.EXPECT().CspDomainFetch(dm).Return(&mcsp.CspDomainFetchOK{Payload: mCSPObj}, nil).MaxTimes(1)
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordIncident(ctx,
		&cluster.Incident{Message: errDomainMismatch.Error(),
			Severity: cluster.IncidentFatal}).Return(nil, nil).MinTimes(1)
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	asm = newAppServantMatcher(t, errDomainMismatch.Error())
	mServant.EXPECT().FatalError(asm).MinTimes(1)
	err = hb.Buzz(ctx)
	assert.True(hb.fatalError)
	assert.NotNil(err)
	assert.Equal(errDomainMismatch, err)
	assert.Equal(mCSPObj, hb.mCspDomainObj)
	mCSPObj.CspDomainType = saveDT
	tl.Flush()

	// initializing csp client fails application
	mockCtrl.Finish()
	t.Log("fetch csp, init csp client failure")
	hb.mCspDomainObj = nil
	hb.fatalError = false
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	m2API = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClusterdAPI = m2API
	cspOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cspOps).MinTimes(1)
	dm = mockmgmtclient.NewCspDomainMatcher(t, mcsp.NewCspDomainFetchParams())
	dm.FetchParam.ID = app.CSPDomainID
	cspOps.EXPECT().CspDomainFetch(dm).Return(&mcsp.CspDomainFetchOK{Payload: mCSPObj}, nil).MaxTimes(1)
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	mServant.EXPECT().InitializeCSPClient(mCSPObj).Return(fmt.Errorf("some-error"))
	asm = newAppServantMatcher(t, errServiceInit.Error())
	mServant.EXPECT().FatalError(asm).MinTimes(1)
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordIncident(ctx,
		&cluster.Incident{Message: errServiceInit.Error(),
			Severity: cluster.IncidentFatal}).Return(nil, nil).MinTimes(1)
	err = hb.Buzz(ctx)
	assert.Error(err)
	assert.Equal(errServiceInit, err)
	assert.Equal(mCSPObj, hb.mCspDomainObj)
	assert.True(hb.fatalError)
	tl.Flush()

	// cluster not found
	mockCtrl.Finish()
	t.Log("domain set, cluster not found")
	hb.mCspDomainObj = mCSPObj
	hb.mClusterObj = nil
	hb.clusterID = ""
	hb.fatalError = false
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	fCrud.RetLClObj = nil
	fCrud.RetLClErr = &crud.Error{Payload: models.Error{Code: http.StatusNotFound, Message: swag.String(com.ErrorNotFound)}}
	app.ClientAPI = mAPI
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordIncident(ctx,
		&cluster.Incident{Message: errClusterNotFound.Error(),
			Severity: cluster.IncidentFatal}).Return(nil, nil).MinTimes(1)
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	asm = newAppServantMatcher(t, errClusterNotFound.Error())
	mServant.EXPECT().FatalError(asm).MinTimes(1)
	err = hb.Buzz(ctx)
	assert.Error(err)
	assert.Equal(errClusterNotFound, err)
	assert.Equal("CLUSTER-1", fCrud.InLClID)
	assert.True(hb.fatalError)
	tl.Flush()

	// cluster mismatch error cases
	for _, tc := range []string{"CspDomainID", "ClusterType"} {
		mockCtrl.Finish()
		tl.Logger().Info("Cluster mismatch error", tc)
		tl.Flush()
		hb.fatalError = false
		hb.mClusterObj = nil
		hb.clusterID = ""
		mockCtrl = gomock.NewController(t)
		app.InFatalError = true // don't want to exit
		mCluster = mockcluster.NewMockClient(mockCtrl)
		app.ClusterClient = mCluster
		mCluster.EXPECT().RecordIncident(ctx,
			&cluster.Incident{Message: errClusterMismatch.Error(),
				Severity: cluster.IncidentFatal}).Return(nil, nil).MinTimes(1)
		mServant = NewMockAppServant(mockCtrl)
		hb.app.AppServant = mServant
		asm = newAppServantMatcher(t, errClusterMismatch.Error())
		mServant.EXPECT().FatalError(asm).MinTimes(1)
		var clObj *models.Cluster
		testutils.Clone(clusterObj, &clObj)
		switch tc {
		case "CspDomainID":
			clObj.CspDomainID = "foo"
		case "ClusterType":
			clObj.ClusterType = "foo"
		}
		fCrud.RetLClObj = clObj
		fCrud.RetLClErr = nil
		err = hb.Buzz(ctx)
		assert.Equal(errClusterMismatch, err)
	}

	// fake other objects
	hb.clusterID = "some-id"
	hb.ephemeralDevices = []*csp.EphemeralDevice{}
	hb.nodeIdentifier = hb.app.InstanceMD[csp.IMDInstanceName]

	// Buzz doesn't do anything again
	err = hb.Buzz(nil)
	assert.Error(err)
	assert.True(hb.fatalError)
}

func TestUpdateNodeLocalStorage(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log:                   tl.Logger(),
			HeartbeatPeriod:       10,
			NuvoStartupPollPeriod: 5,
			CSPDomainID:           "6a156de2-ba94-4035-b47f-e87373d75757",
			ClusterType:           "kubernetes",
			SystemID:              "system-1",
		},
		InstanceMD: fakeNodeMD,
		ClusterMD:  fakeClusterMD,
	}
	svcArgs := util.ServiceArgs{
		ServiceType:         "agentd",
		ServiceVersion:      "test",
		HeartbeatPeriodSecs: int64(app.HeartbeatPeriod),
		Log:                 app.Log,
		MaxMessages:         app.MaxMessages,
		ServiceAttributes:   map[string]models.ValueType{},
	}
	app.Service = util.NewService(&svcArgs)
	hb := newHBComp()
	hb.app = app
	hb.Log = app.Log
	mObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID:      "NODE-1",
				Version: 1,
			},
		},
	}
	ctx := context.Background()

	// error if hb.mObj is nil
	ret, err := hb.UpdateNodeLocalStorage(ctx, nil, 0)
	assert.Nil(ret)
	assert.Regexp("not ready", err)

	// hb.mObj set, update fails
	hb.mObj = mObj
	clCrud := &fakecrud.Client{}
	app.ClusterdOCrud = clCrud
	clCrud.RetNodeUpdateErr = errors.New("fail")
	ret, err = hb.UpdateNodeLocalStorage(ctx, nil, 0)
	assert.Nil(ret)
	expItems := &crud.Updates{Set: []string{"state", "service", "nodeAttributes", "localStorage", "totalCacheBytes", "cacheUnitSizeBytes"}}
	assert.Equal(expItems, clCrud.InNodeUpdateItems)
	assert.Regexp("fail", err)

	// hb.mObj set, update succeeds
	app.ClusterdOCrud = clCrud
	clCrud.RetNodeUpdateObj = mObj
	clCrud.RetNodeUpdateErr = nil
	ret, err = hb.UpdateNodeLocalStorage(ctx, map[string]models.NodeStorageDevice{}, 0)
	assert.NoError(err)
	assert.Equal(expItems, clCrud.InNodeUpdateItems)
	assert.Equal(mObj, ret)
}

func TestUpdateLocalStorage(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	hb := newHBComp()
	mObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID:      "NODE-1",
				Version: 1,
			},
		},
	}

	t.Log("no local storage")
	assert.False(hb.updateLocalStorage(mObj))
	assert.Nil(mObj.LocalStorage)

	t.Log("add local storage")
	hb.ephemeralDevices = []*csp.EphemeralDevice{
		{
			Path:      "/dev/xvdb",
			Type:      "SSD",
			Usable:    true,
			SizeBytes: 1024,
		},
		{
			Path:      "/dev/xvdc",
			Type:      "HDD",
			Usable:    false,
			SizeBytes: 2048,
		},
	}
	assert.True(hb.updateLocalStorage(mObj))
	assert.Len(mObj.LocalStorage, 2)
	for id, mDev := range mObj.LocalStorage {
		_, err := uuid.FromString(id)
		assert.NoError(err)
		var exp *models.NodeStorageDevice
		switch mDev.DeviceName {
		case "/dev/xvdb":
			exp = &models.NodeStorageDevice{
				DeviceName:  "/dev/xvdb",
				DeviceState: com.NodeDevStateUnused,
				DeviceType:  "SSD",
				SizeBytes:   swag.Int64(1024),
			}
		case "/dev/xvdc":
			exp = &models.NodeStorageDevice{
				DeviceName:  "/dev/xvdc",
				DeviceState: com.NodeDevStateRestricted,
				DeviceType:  "HDD",
				SizeBytes:   swag.Int64(2048),
			}
		default:
			assert.Fail("unexpected DeviceName", mDev.DeviceName)
		}
		assert.Equal(exp, &mDev)
	}

	t.Log("add local storage with existing storage")
	mObj.LocalStorage = map[string]models.NodeStorageDevice{
		"id1": models.NodeStorageDevice{
			DeviceName:  "/dev/xvdb",
			DeviceState: com.NodeDevStateError,
			DeviceType:  "SSD",
			SizeBytes:   swag.Int64(1024),
		},
	}
	hb.ephemeralDevices[0].Initialized = true // ignored
	hb.ephemeralDevices[1] = &csp.EphemeralDevice{
		Path:      "/dev/xvdc",
		Type:      "SSD",
		Usable:    false,
		SizeBytes: 2048,
	}
	assert.True(hb.updateLocalStorage(mObj))
	assert.Len(mObj.LocalStorage, 2)
	cID := ""
	for id, mDev := range mObj.LocalStorage {
		var exp *models.NodeStorageDevice
		switch mDev.DeviceName {
		case "/dev/xvdb":
			assert.Equal("id1", id)
			exp = &models.NodeStorageDevice{
				DeviceName:  "/dev/xvdb",
				DeviceState: com.NodeDevStateError,
				DeviceType:  "SSD",
				SizeBytes:   swag.Int64(1024),
			}
		case "/dev/xvdc":
			assert.NotEmpty(id)
			_, err := uuid.FromString(id)
			assert.NoError(err)
			cID = id
			exp = &models.NodeStorageDevice{
				DeviceName:  "/dev/xvdc",
				DeviceState: com.NodeDevStateRestricted,
				DeviceType:  "SSD",
				SizeBytes:   swag.Int64(2048),
			}
		default:
			assert.Fail("unexpected DeviceName", mDev.DeviceName)
		}
		assert.Equal(exp, &mDev)
	}

	t.Log("update local storage state changes")
	hb.ephemeralDevices[0].Usable = false
	hb.ephemeralDevices[1].Usable = true
	assert.True(hb.updateLocalStorage(mObj))
	assert.Len(mObj.LocalStorage, 2)
	for id, mDev := range mObj.LocalStorage {
		var exp *models.NodeStorageDevice
		switch mDev.DeviceName {
		case "/dev/xvdb":
			assert.Equal("id1", id)
			exp = &models.NodeStorageDevice{
				DeviceName:  "/dev/xvdb",
				DeviceState: com.NodeDevStateRestricted,
				DeviceType:  "SSD",
				SizeBytes:   swag.Int64(1024),
			}
		case "/dev/xvdc":
			assert.Equal(cID, id)
			exp = &models.NodeStorageDevice{
				DeviceName:  "/dev/xvdc",
				DeviceState: com.NodeDevStateUnused,
				DeviceType:  "SSD",
				SizeBytes:   swag.Int64(2048),
			}
		default:
			assert.Fail("unexpected DeviceName", mDev.DeviceName)
		}
		assert.Equal(exp, &mDev)
	}

	t.Log("update local storage size and type")
	hb.ephemeralDevices[0].SizeBytes = 4096
	hb.ephemeralDevices[1].Type = "HDD"
	assert.True(hb.updateLocalStorage(mObj))
	assert.Len(mObj.LocalStorage, 2)
	for id, mDev := range mObj.LocalStorage {
		var exp *models.NodeStorageDevice
		switch mDev.DeviceName {
		case "/dev/xvdb":
			assert.Equal("id1", id)
			exp = &models.NodeStorageDevice{
				DeviceName:  "/dev/xvdb",
				DeviceState: com.NodeDevStateRestricted,
				DeviceType:  "SSD",
				SizeBytes:   swag.Int64(4096),
			}
		case "/dev/xvdc":
			assert.Equal(cID, id)
			exp = &models.NodeStorageDevice{
				DeviceName:  "/dev/xvdc",
				DeviceState: com.NodeDevStateUnused,
				DeviceType:  "HDD",
				SizeBytes:   swag.Int64(2048),
			}
		default:
			assert.Fail("unexpected DeviceName", mDev.DeviceName)
		}
		assert.Equal(exp, &mDev)
	}

	t.Log("delete local storage")
	hb.ephemeralDevices = []*csp.EphemeralDevice{}
	assert.True(hb.updateLocalStorage(mObj))
	assert.NotNil(mObj.LocalStorage)
	assert.Empty(mObj.LocalStorage)

	t.Log("updateLocalStorage called from updateNodeObj")
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log:                   tl.Logger(),
			HeartbeatPeriod:       10,
			NuvoStartupPollPeriod: 5,
			CSPDomainID:           "6a156de2-ba94-4035-b47f-e87373d75757",
			ClusterType:           "kubernetes",
			SystemID:              "system-1",
		},
		InstanceMD: fakeNodeMD,
		ClusterMD:  fakeClusterMD,
	}
	eM := &fev.Manager{}
	app.CrudeOps = eM
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	svcArgs := util.ServiceArgs{
		ServiceType:         "agentd",
		ServiceVersion:      "test",
		HeartbeatPeriodSecs: int64(app.HeartbeatPeriod),
		Log:                 app.Log,
		MaxMessages:         app.MaxMessages,
		ServiceAttributes:   map[string]models.ValueType{},
	}
	app.Service = util.NewService(&svcArgs)
	clCrud := &fakecrud.Client{}
	app.ClusterdOCrud = clCrud
	hb.app = app
	clCrud.RetNodeUpdateErr = errors.New("fail")
	hb.updateCount = 0
	hb.lastHeartbeatTime = time.Time{} // force
	hb.ephemeralDevices = []*csp.EphemeralDevice{
		{
			Path:      "/dev/xvdc",
			Type:      "SSD",
			Usable:    true,
			SizeBytes: 2048,
		},
	}
	hb.mObj = mObj
	ctx := context.Background()
	err := hb.updateNodeObj(ctx)
	assert.Equal(clCrud.RetNodeUpdateErr, err)
	expItems := &crud.Updates{Set: []string{"state", "service", "nodeAttributes", "localStorage", "totalCacheBytes", "cacheUnitSizeBytes"}}
	assert.Equal(expItems, clCrud.InNodeUpdateItems)
	assert.Len(clCrud.InNodeUpdateObj.LocalStorage, 1)
	assert.Zero(swag.Int64Value(clCrud.InNodeUpdateObj.TotalCacheBytes))
	assert.Zero(hb.updateCount)

	t.Log("local storage only updated on zero count")
	hb.updateCount = 1
	err = hb.updateNodeObj(ctx)
	assert.Equal(clCrud.RetNodeUpdateErr, err)
	expItems = &crud.Updates{Set: []string{"state", "service", "nodeAttributes"}}
	assert.Equal(expItems, clCrud.InNodeUpdateItems)
	assert.Zero(hb.updateCount)

	t.Log("local storage is cache")
	fCrud := &fakecrud.Client{}
	app.OCrud = fCrud
	nsa := &state.NodeStateArgs{
		Log:   tl.Logger(),
		OCrud: fCrud,
		Node:  mObj,
	}
	ns := state.NewNodeState(nsa)
	app.StateOps = ns
	assert.Equal(mObj, ns.Node)
	assert.Equal(models.ObjVersion(1), ns.Node.Meta.Version)
	mObj.Meta = &models.ObjMeta{Version: 2}
	mObj.LocalStorage = map[string]models.NodeStorageDevice{
		"id1": {
			DeviceName:      "/dev/xvdc",
			DeviceState:     com.NodeDevStateCache,
			DeviceType:      "SSD",
			SizeBytes:       swag.Int64(3192),
			UsableSizeBytes: swag.Int64(2048),
		},
	}
	hb.mObj = mObj
	hb.Log = tl.Logger()
	clCrud.RetNodeUpdateErr = nil
	clCrud.RetNodeUpdateObj = mObj
	err = hb.updateNodeObj(ctx)
	expItems = &crud.Updates{Set: []string{"state", "service", "nodeAttributes", "localStorage", "totalCacheBytes", "cacheUnitSizeBytes"}}
	assert.Equal(expItems, clCrud.InNodeUpdateItems)
	assert.Len(clCrud.InNodeUpdateObj.LocalStorage, 1)
	assert.EqualValues(2048, swag.Int64Value(clCrud.InNodeUpdateObj.TotalCacheBytes))
	assert.Equal(1, hb.updateCount)
	assert.Equal(app.StateOps.NS().Node, mObj)
	assert.Equal(models.ObjVersion(2), ns.Node.Meta.Version)
}

func TestVersionLog(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log:                   tl.Logger(),
			HeartbeatPeriod:       10,
			VersionLogPeriod:      30 * time.Minute,
			NuvoStartupPollPeriod: 5,
			CSPDomainID:           "6a156de2-ba94-4035-b47f-e87373d75757",
			ClusterType:           "kubernetes",
			SystemID:              "system-1",
			ServiceVersion:        "commit time host",
			NuvoAPIVersion:        "0.6.2",
			InvocationArgs:        "{ invocation flag values }",
		},
		InstanceMD: fakeNodeMD,
		ClusterMD:  fakeClusterMD,
	}
	eM := &fev.Manager{}
	app.CrudeOps = eM
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	svcArgs := util.ServiceArgs{
		ServiceType:         "agentd",
		ServiceVersion:      "test",
		HeartbeatPeriodSecs: int64(app.HeartbeatPeriod),
		Log:                 app.Log,
		MaxMessages:         app.MaxMessages,
		ServiceAttributes:   map[string]models.ValueType{},
	}
	app.Service = util.NewService(&svcArgs)
	hb := newHBComp()
	ctx := context.Background()

	// Init
	hb.Init(app)
	hb.nuvoNodeUUIDSet = true // tested separately
	assert.Equal("", hb.nodeIdentifier)
	assert.Equal(0, hb.runCount)

	// This test fakes out Start/Stop to avoid racy scheduling issues
	hb.ephemeralDevices = []*csp.EphemeralDevice{}
	hb.nodeIdentifier = hb.app.InstanceMD[csp.IMDInstanceName]
	hb.runCount = 0
	hb.systemID = "fake-system-id"
	hb.createdUpMon = true

	now := time.Now()
	hb.mCspDomainObj = &models.CSPDomain{}

	// fake cluster list error to skip most of Buzz(), version gets logged
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	m2API := mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClusterdAPI = m2API
	clOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps).MinTimes(1)
	clM := mockmgmtclient.NewClusterMatcher(t, mcl.NewClusterListParams())
	clM.ListParam.ClusterIdentifier = &hb.clusterIdentifier
	clM.ListParam.CspDomainID = &app.CSPDomainID
	clOps.EXPECT().ClusterList(clM).Return(&mcl.ClusterListOK{Payload: []*models.Cluster{}}, nil)
	mCluster := mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil)
	assert.Equal("", hb.clusterID)
	assert.Zero(hb.lastVersionLogTime)
	err := hb.Buzz(ctx)
	assert.Error(err)
	assert.True(hb.runCount == 1)
	assert.Equal(1, tl.CountPattern("INFO .*invocation flag values"))
	assert.True(now.Before(hb.lastVersionLogTime))
	prevLogTime := hb.lastVersionLogTime
	tl.Flush()

	// second interation, no logging
	clOps.EXPECT().ClusterList(clM).Return(&mcl.ClusterListOK{Payload: []*models.Cluster{}}, nil)
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil)
	err = hb.Buzz(ctx)
	assert.Error(err)
	assert.True(hb.runCount == 2)
	assert.Equal(0, tl.CountPattern("INFO .*invocation flag values"))
	assert.Equal(prevLogTime, hb.lastVersionLogTime)
	tl.Flush()

	// time is past, logs again
	hb.lastVersionLogTime = now.Add(-30 * time.Minute)
	clOps.EXPECT().ClusterList(clM).Return(&mcl.ClusterListOK{Payload: []*models.Cluster{}}, nil)
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil)
	err = hb.Buzz(ctx)
	assert.Error(err)
	assert.True(hb.runCount == 3)
	assert.Equal(1, tl.CountPattern("INFO .*invocation flag values"))
	assert.True(prevLogTime.Before(hb.lastVersionLogTime))
}

func TestInitializeNuvo(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log:                   tl.Logger(),
			HeartbeatPeriod:       10,
			NuvoStartupPollPeriod: 5,
			NuvoStartupPollMax:    10,
		},
		InstanceMD: fakeNodeMD,
		ClusterMD:  fakeClusterMD,
	}
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	eM := &fev.Manager{}
	app.CrudeOps = eM
	hb := newHBComp()
	hb.Init(app)
	// replace the worker
	tw := &fw.Worker{}
	hb.worker = tw
	tw.RetGSI = hb.sleepPeriod
	tw.InSSI = 0

	// error
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mServant := NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	mServant.EXPECT().InitializeNuvo(gomock.Any()).Return(errors.New("unexpected uuid error"))
	err := hb.initializeNuvo(nil)
	if assert.Error(err) {
		assert.Regexp("unexpected uuid error", err.Error())
	}
	assert.Equal(hb.nuvoPollPeriod, tw.InSSI) // changed

	// success
	tw.RetGSI = hb.sleepPeriod
	tw.InSSI = 0
	mServant.EXPECT().InitializeNuvo(gomock.Any()).Return(nil)
	assert.NoError(hb.initializeNuvo(nil))
	assert.True(hb.nuvoNodeUUIDSet)
	assert.EqualValues(0, tw.InSSI) // unchanged
}

// copyFile is a simple file copier
func copyFile(src string, dst string) error {
	data, err := ioutil.ReadFile(src)
	if err == nil {
		err = ioutil.WriteFile(dst, data, 0644)
	}
	return err
}

func TestUpdateNuvoFVIni(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log:                   tl.Logger(),
			HeartbeatPeriod:       10,
			NuvoStartupPollPeriod: 5,
			CSPDomainID:           "6a156de2-ba94-4035-b47f-e87373d75757",
			ClusterType:           "kubernetes",
		},
		InstanceMD: fakeNodeMD,
		ClusterMD:  fakeClusterMD,
	}
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	hb := newHBComp()
	hb.Init(app)
	hb.mObj = &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{ID: "node-1"},
		},
	}
	hb.clusterID = "cl-1"
	hb.nodeIdentifier = "abc-xyz"
	hb.systemID = "sys-1"

	// ini name not set
	assert.Nil(hb.updateNuvoFVIni(nil))
	assert.False(hb.fvIniUpdated)

	// invalid ini name
	app.NuvoFVIni = "no-such-file.ini"
	e := hb.updateNuvoFVIni(nil)
	if assert.Error(e) {
		assert.True(os.IsNotExist(e))
	}
	assert.False(hb.fvIniUpdated)
	tl.Flush()

	// update needed
	iniFile := "test-temp-fv.ini"
	copyFile("fv-template.ini", iniFile)
	app.NuvoFVIni = flags.Filename(iniFile)
	e = hb.updateNuvoFVIni(nil)
	assert.NoError(e)
	data, err := ioutil.ReadFile(iniFile)
	if assert.NoError(err) {
		s := string(data)
		assert.Regexp("SocketPath = /some/path", s)
		assert.Regexp("NodeID = node-1", s)
		assert.Regexp("NodeIdentifier = abc-xyz", s)
		assert.Regexp("ClusterID = cl-1", s)
		assert.Regexp("SystemID = sys-1", s)
	}
	assert.True(hb.fvIniUpdated)
	stat, err := os.Stat(iniFile)
	assert.NoError(err)
	assert.Equal(1, tl.CountPattern("Updated flexVolume driver"))
	tl.Flush()

	// no updated needed
	hb.fvIniUpdated = false
	e = hb.updateNuvoFVIni(nil)
	assert.NoError(e)
	assert.True(hb.fvIniUpdated)
	stat2, err := os.Stat(iniFile)
	assert.NoError(err)
	assert.Equal(stat.ModTime(), stat2.ModTime())
	assert.Equal(1, tl.CountPattern("already up to date"))
	assert.Equal(0, tl.CountPattern("Updated flexVolume driver"))
	tl.Flush()

	// update skipped after success
	hb.fvIniUpdated = true
	e = hb.updateNuvoFVIni(nil)
	assert.NoError(e)
	assert.True(hb.fvIniUpdated)
	assert.NoError(err)
	assert.Equal(stat.ModTime(), stat2.ModTime())
	assert.Equal(0, tl.CountPattern("already up to date"))
	assert.Equal(0, tl.CountPattern("Updated flexVolume driver"))
	os.Remove(iniFile)

	// write error
	if os.Getuid() != 0 {
		copyFile("fv-template.ini", iniFile)
		assert.NoError(os.Chmod(iniFile, 0400))
		hb.fvIniUpdated = false
		e = hb.updateNuvoFVIni(nil)
		if assert.Error(e) {
			assert.True(os.IsPermission(e))
		}
		assert.NoError(os.Chmod(iniFile, 0644))
		os.Remove(iniFile)
	}
}

func TestPingNuvo(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log:                   tl.Logger(),
			HeartbeatPeriod:       10,
			NuvoStartupPollPeriod: 5,
			NuvoStartupPollMax:    10,
		},
		InstanceMD: fakeNodeMD,
		ClusterMD:  fakeClusterMD,
	}
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	eM := &fev.Manager{}
	app.CrudeOps = eM
	hb := newHBComp()
	hb.Init(app)

	nsr := nuvoapi.NodeStatusReport{
		NodeUUID: "node-uuid",
		Volumes: []nuvoapi.VolStatus{
			nuvoapi.VolStatus{VolUUID: "vol-1",
				ClassSpace: []nuvoapi.VolClassSpace{
					nuvoapi.VolClassSpace{Class: 0, BlocksUsed: 1, BlocksAllocated: 2, BlocksTotal: 2},
				}},
		},
	}
	// replace the worker
	tw := &fw.Worker{}
	hb.worker = tw
	tw.RetGSI = hb.nuvoPollPeriod
	tw.InSSI = 0

	// success, initialized
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mServant := NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	nvAPI := mockNuvo.NewMockNuvoVM(mockCtrl)
	hb.app.NuvoAPI = nvAPI
	nvAPI.EXPECT().NodeStatus().Return(nsr, nil)
	err := hb.pingNuvo(nil)
	assert.NoError(err)
	mockCtrl.Finish()
	assert.Equal(1, tl.CountPattern("NodeStatus.*succeeded"))
	assert.Equal(0, tl.CountPattern("NodeStatus.*failed"))
	assert.Equal(0, tl.CountPattern("Nuvo requires re-initialization"))
	assert.Equal(hb.sleepPeriod, tw.InSSI) // reset normal period
	tl.Flush()

	// success, node uuid not set
	tw.RetGSI = hb.sleepPeriod
	tw.InSSI = 0
	nsr = nuvoapi.NodeStatusReport{}
	mockCtrl = gomock.NewController(t)
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	mServant.EXPECT().InitializeNuvo(gomock.Any()).Return(fmt.Errorf("init-error"))
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	hb.app.NuvoAPI = nvAPI
	nvAPI.EXPECT().NodeStatus().Return(nsr, nil)
	err = hb.pingNuvo(nil)
	assert.Error(err)
	assert.Regexp("init-error", err)
	mockCtrl.Finish()
	assert.Equal(1, tl.CountPattern("NodeStatus.*succeeded"))
	assert.Equal(0, tl.CountPattern("NodeStatus.*failed"))
	assert.Equal(1, tl.CountPattern("Nuvo requires re-initialization"))
	assert.Equal(hb.nuvoPollPeriod, tw.InSSI) // changed
	tl.Flush()

	// any error
	tw.RetGSI = hb.sleepPeriod
	tw.InSSI = 0
	mockCtrl = gomock.NewController(t)
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	hb.app.NuvoAPI = nvAPI
	nuvoErr := nuvoapi.NewNuvoAPIError("tempErr", true)
	nvAPI.EXPECT().NodeStatus().Return(nsr, nuvoErr)
	err = hb.pingNuvo(nil)
	assert.Nil(err)
	assert.Equal(0, tl.CountPattern("NodeStatus.*succeeded"))
	assert.Equal(1, tl.CountPattern("NodeStatus.*failed"))
	assert.Equal(0, tl.CountPattern("Nuvo requires re-initialization"))
	assert.Equal(hb.nuvoPollPeriod, tw.InSSI) // changed
	tl.Flush()
	mockCtrl.Finish()

	// any error
	tw.RetGSI = hb.sleepPeriod
	tw.InSSI = 0
	mockCtrl = gomock.NewController(t)
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	hb.app.NuvoAPI = nvAPI
	nuvoErr = nuvoapi.WrapError(fmt.Errorf("some-error"))
	nvAPI.EXPECT().NodeStatus().Return(nsr, nuvoErr)
	mServant.EXPECT().InitializeNuvo(gomock.Any()).Return(fmt.Errorf("init-error"))
	err = hb.pingNuvo(nil)
	assert.Error(err)
	assert.Regexp("init-error", err)
	assert.Equal(0, tl.CountPattern("NodeStatus.*succeeded"))
	assert.Equal(1, tl.CountPattern("NodeStatus.*failed"))
	assert.Equal(1, tl.CountPattern("Nuvo requires re-initialization"))
	assert.Equal(hb.nuvoPollPeriod, tw.InSSI) // changed
}
