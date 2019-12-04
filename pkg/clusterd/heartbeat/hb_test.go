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
	"regexp"
	"strings"
	"testing"
	"time"

	mcl "github.com/Nuvoloso/kontroller/pkg/autogen/client/cluster"
	mcsp "github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/system"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	mockcluster "github.com/Nuvoloso/kontroller/pkg/cluster/mock"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	"github.com/Nuvoloso/kontroller/pkg/clusterd/state"
	fakeState "github.com/Nuvoloso/kontroller/pkg/clusterd/state/fake"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	mockcsp "github.com/Nuvoloso/kontroller/pkg/csp/mock"
	"github.com/Nuvoloso/kontroller/pkg/layout"
	"github.com/Nuvoloso/kontroller/pkg/metricmover"
	fmm "github.com/Nuvoloso/kontroller/pkg/metricmover/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient"
	fakecrud "github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	fw "github.com/Nuvoloso/kontroller/pkg/util/fake"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// Matcher for AppServant
type mockAppServantMatchCtx int

const (
	mockAppServantFatalError mockAppServantMatchCtx = iota
)

type mockAppServantMatcher struct {
	t       *testing.T
	ctx     mockAppServantMatchCtx
	pattern string
}

// Matches is from gomock.Matcher
func (o *mockAppServantMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch o.ctx {
	case mockAppServantFatalError:
		e, ok := x.(error)
		return assert.True(ok) &&
			assert.Regexp(o.pattern, e.Error())
	}
	return false
}

// String is from gomock.Matcher
func (o *mockAppServantMatcher) String() string {
	switch o.ctx {
	case mockAppServantFatalError:
		return "matches on fatalError"
	}
	return "unknown context"
}

func newAppServantMatcher(t *testing.T, ctx mockAppServantMatchCtx) *mockAppServantMatcher {
	return &mockAppServantMatcher{t: t, ctx: ctx}
}

var fakeClusterMD = map[string]string{
	cluster.CMDIdentifier: "k8s-svc-uid:c981db76-bda0-11e7-b2ce-02a3152c8208",
	cluster.CMDVersion:    "1.7",
	"CreationTimestamp":   "2017-10-30T18:33:13Z",
	"GitVersion":          "v1.7.8",
	"Platform":            "linux/amd64",
}

const fakeClusterIMDProp = csp.IMDProvisioningPrefix + "Prop"

var fakeNodeMD = map[string]string{
	csp.IMDInstanceName: "i-00cf71dee433b52db",
	csp.IMDLocalIP:      "172.31.24.56",
	csp.IMDZone:         "us-west-2a",
	"public-ipv4":       "34.212.187.125",
	"instance-type":     "t2.micro",
	"local-hostname":    "ip-172-31-24-56.us-west-2.compute.internal",
	"public-hostname":   "ec2-34-212-187-125.us-west-2.compute.amazonaws.com",
	fakeClusterIMDProp:  "cluster-imd-property-value", // added to cluster attributes
}

func TestComponentMethods(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log:             tl.Logger(),
			HeartbeatPeriod: 10,
			CSPDomainID:     "6a156de2-ba94-4035-b47f-e87373d75757",
		},
		ClusterMD:  fakeClusterMD,
		InstanceMD: fakeNodeMD,
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
	assert.Equal("", hb.clusterVersion)
	assert.Equal("", hb.clusterIdentifier)
	assert.NotNil(app.AppObjects)
	assert.EqualValues(hb, app.AppObjects)
	assert.NotNil(app.StateUpdater)
	assert.EqualValues(hb, app.StateUpdater)
	assert.Equal(2, app.HeartbeatTaskPeriodMultiplier) // hard coded default if < 2
	assert.Equal(2*hb.sleepPeriod, hb.heartbeatTaskPeriod)
	assert.NotNil(hb.ntd)

	// replace the worker and node detector
	tw := &fw.Worker{}
	hb.worker = tw
	hb.ntd = &fakeTerminatedNodeHandler{}

	// reasonable delays for this test
	hb.stopPeriod = 100 * time.Millisecond

	hb.Start()
	assert.Equal(app.ClusterMD[cluster.CMDVersion], hb.clusterVersion)
	assert.Equal(app.ClusterMD[cluster.CMDIdentifier], hb.clusterIdentifier)
	assert.Equal(1, tw.CntStart)

	hb.Stop()
	assert.Equal(1, tw.CntStop)

	// cluster attribute creation
	clAttr := hb.clusterAttrs()
	assert.Len(clAttr, len(fakeClusterMD)+1)
	for n, v := range clAttr {
		if strings.HasPrefix(n, csp.IMDProvisioningPrefix) {
			iv, ok := fakeNodeMD[n]
			assert.True(ok)
			assert.True(v.Kind == "STRING")
			assert.Equal(iv, v.Value)
			continue
		}
		mdv, ok := fakeClusterMD[n]
		assert.True(ok)
		assert.True(v.Kind == "STRING")
		assert.Equal(mdv, v.Value)
	}
}

func TestClusterCreationOnStart(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	la, err := layout.FindAlgorithm(layout.AlgorithmStandaloneNetworkedShared)
	assert.NoError(err)
	assert.NotNil(la)
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log:             tl.Logger(),
			HeartbeatPeriod: 10,
			CSPDomainID:     "6a156de2-ba94-4035-b47f-e87373d75757",
			ClusterType:     "kubernetes",
			SystemID:        "system-1",
		},
		ClusterMD:         fakeClusterMD,
		InstanceMD:        fakeNodeMD,
		StorageAlgorithms: []*layout.Algorithm{la},
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
	mObj := &models.Cluster{
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
			ClusterMutableAllOf0: models.ClusterMutableAllOf0{
				ClusterVersion: fakeClusterMD[cluster.CMDVersion],
				ClusterUsagePolicy: &models.ClusterUsagePolicy{
					Inherited: true,
				},
				//Service: get this data from the service after Init
			},
			ClusterCreateMutable: models.ClusterCreateMutable{
				Name:              models.ObjName(fakeClusterMD[cluster.CMDIdentifier]),
				ClusterIdentifier: fakeClusterMD[cluster.CMDIdentifier],
				//ClusterAttributes: get this after init from hb
			},
		},
	}
	sysObj := &models.System{
		SystemAllOf0: models.SystemAllOf0{
			Meta: &models.ObjMeta{ID: "system-1"},
		},
	}

	// Init
	hb.Init(app)
	assert.Equal("", hb.clusterVersion)
	assert.Equal("", hb.clusterIdentifier)
	assert.Equal(0, hb.runCount)
	assert.Zero(hb.updateCount)
	ntd := &fakeTerminatedNodeHandler{}
	hb.ntd = ntd

	// Start would set these
	hb.clusterVersion = app.ClusterMD[cluster.CMDVersion]
	hb.clusterIdentifier = app.ClusterMD[cluster.CMDIdentifier]

	mObj.ClusterAttributes = hb.clusterAttrs()
	assert.Len(mObj.ClusterAttributes, len(fakeClusterMD)+1)
	mObj.Service = app.Service.ModelObj()
	assert.NotNil(mObj.Service)

	// list has to return an empty array for creation to be called
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	ctxKey := struct{}{}
	ctx := context.WithValue(context.Background(), ctxKey, 42)
	mServant := NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	mServant.EXPECT().InitializeCSPClient(mCSPObj).Return(nil)
	mServant.EXPECT().GetAPIArgs().Return(&mgmtclient.APIArgs{})
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
	cOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).Times(3)
	lm := mockmgmtclient.NewClusterMatcher(t, mcl.NewClusterListParams())
	lm.ListParam.ClusterIdentifier = &hb.clusterIdentifier
	lm.ListParam.CspDomainID = &app.CSPDomainID
	cOps.EXPECT().ClusterList(lm).Return(&mcl.ClusterListOK{}, nil).MinTimes(1)
	cm := mockmgmtclient.NewClusterMatcher(t, mcl.NewClusterCreateParams())
	cm.CreateParam.Payload = &models.ClusterCreateArgs{
		ClusterCreateOnce: models.ClusterCreateOnce{
			ClusterType: mObj.ClusterType,
			CspDomainID: mObj.CspDomainID,
		},
		ClusterCreateMutable: models.ClusterCreateMutable{
			ClusterAttributes: mObj.ClusterAttributes,
			Name:              mObj.Name,
			ClusterIdentifier: mObj.ClusterIdentifier,
			State:             com.ClusterStateManaged,
		},
	}
	cOps.EXPECT().ClusterCreate(cm).Return(&mcl.ClusterCreateCreated{Payload: mObj}, nil)
	mObj.Service.State = app.Service.StateString(app.Service.GetState())
	um := mockmgmtclient.NewClusterMatcher(t, mcl.NewClusterUpdateParams())
	um.UpdateParam.Payload = &models.ClusterMutable{
		ClusterMutableAllOf0: models.ClusterMutableAllOf0{
			ClusterVersion: mObj.ClusterVersion,
			Service:        mObj.Service,
		},
		ClusterCreateMutable: models.ClusterCreateMutable{
			ClusterAttributes: mObj.ClusterAttributes,
		},
	}
	um.UpdateParam.ID = string(mObj.Meta.ID)
	um.UpdateParam.Set = []string{"clusterVersion", "service", "clusterAttributes"}
	cOps.EXPECT().ClusterUpdate(um).Return(&mcl.ClusterUpdateOK{Payload: mObj}, nil)
	mCluster := mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).MinTimes(1)
	app.CSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	assert.Nil(hb.mCspDomainObj)
	assert.Nil(hb.mObj)
	assert.Nil(app.StateOps)
	assert.NotNil(app.OCrud)
	assert.NotNil(app.CSP)
	err = hb.Buzz(ctx)
	assert.NoError(err)
	assert.NotNil(hb.mCspDomainObj)
	assert.Equal(mCSPObj, hb.mCspDomainObj)
	assert.NotNil(hb.mObj)
	assert.Equal(mObj, hb.mObj)
	assert.Equal(1, hb.runCount)
	assert.True(eM.CalledMU)
	assert.True(hb.createdUpMon)
	assert.Equal(1, hb.updateCount)
	// StateOps ready
	assert.NotNil(app.StateOps)
	cState := app.StateOps.CS()
	assert.Equal(mObj, cState.Cluster)
	assert.Equal(app.StorageAlgorithms, cState.LayoutAlgorithms)

	assert.Equal(mObj, hb.GetCluster())
	assert.Equal(mCSPObj, hb.GetCspDomain())
	assert.Equal(1, ntd.detectTerminatedNodesCnt)
	assert.Equal(ctx, ntd.inCtx)

	// now fake a second iteration - upstream monitor not recreated, force error in consolidated hb task creation
	tl.Logger().Info("Starting second iteration: hbt")
	tl.Flush()
	fCrud.InTaskCreateObj = nil
	fCrud.RetTaskCreateErr = fmt.Errorf("task-create")
	fCrud.RetTaskCreateObj = nil
	now = time.Now()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(nil, gomock.Not(gomock.Nil())).Return(nil, nil).MinTimes(1)
	eM.CalledMU = false
	err = hb.Buzz(nil)
	assert.Equal(2, hb.runCount)
	tl.Flush()
	assert.False(eM.CalledMU)
	assert.Equal(1, ntd.detectTerminatedNodesCnt)

	// ClusterName is used if specified
	mockCtrl.Finish()
	tl.Logger().Info("createClusterObj uses ClusterName")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	app.ClusterName = "customName"
	mObj.Name = models.ObjName(app.ClusterName)
	cm.CreateParam.Payload.Name = mObj.Name
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps)
	cOps.EXPECT().ClusterCreate(cm).Return(&mcl.ClusterCreateCreated{Payload: mObj}, nil)
	err = hb.createClusterObj(nil)
	assert.NoError(err)
	tl.Flush()

	// transient error for system fetch
	mockCtrl.Finish()
	tl.Logger().Info("System fetch transient error")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	hb.systemID = ""
	sysOps = mockmgmtclient.NewMockSystemClient(mockCtrl)
	sErr := errors.New("bad gateway")
	sysOps.EXPECT().SystemFetch(gomock.Not(gomock.Nil())).Return(nil, sErr)
	mAPI.EXPECT().System().Return(sysOps)
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordIncident(nil, &cluster.Incident{Message: sErr.Error(), Severity: cluster.IncidentFatal}).Return(nil, nil).MinTimes(1)
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	asm := newAppServantMatcher(t, mockAppServantFatalError)
	asm.pattern = "bad gateway"
	mServant.EXPECT().FatalError(asm).MinTimes(1)
	err = hb.Buzz(nil)
	assert.True(hb.fatalError)
	tl.Flush()

	// invalid system id
	mockCtrl.Finish()
	tl.Logger().Info("Invalid system id")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	app.SystemID = "not-this-system-id"
	hb.systemID = ""
	hb.fatalError = false
	sysOps = mockmgmtclient.NewMockSystemClient(mockCtrl)
	sysOps.EXPECT().SystemFetch(gomock.Not(gomock.Nil())).Return(&system.SystemFetchOK{Payload: sysObj}, nil)
	mAPI.EXPECT().System().Return(sysOps)
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordIncident(nil, &cluster.Incident{Message: errSystemMismatch.Error(), Severity: cluster.IncidentFatal}).Return(nil, nil).MinTimes(1)
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	asm = newAppServantMatcher(t, mockAppServantFatalError)
	asm.pattern = errSystemMismatch.Error()
	mServant.EXPECT().FatalError(asm).MinTimes(1)
	assert.Equal(0, app.FatalErrorCount)
	err = hb.Buzz(nil)
	assert.True(hb.fatalError)
}

// DEPRECATED
func TestClusterFoundOnStart(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log:             tl.Logger(),
			HeartbeatPeriod: 10,
			CSPDomainID:     "6a156de2-ba94-4035-b47f-e87373d75757",
			ClusterType:     "kubernetes",
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
	mObj := &models.Cluster{
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
			ClusterMutableAllOf0: models.ClusterMutableAllOf0{
				ClusterVersion: fakeClusterMD[cluster.CMDVersion],
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
			},
			ClusterCreateMutable: models.ClusterCreateMutable{
				Name:              models.ObjName(fakeClusterMD[cluster.CMDIdentifier]),
				ClusterIdentifier: fakeClusterMD[cluster.CMDIdentifier],
				//ClusterAttributes: get this after init from hb
			},
		},
	}
	tl.Logger().Debugf("initial messages: %d", len(mObj.Service.Messages))
	for _, m := range mObj.Service.Messages {
		tl.Logger().Debug(m)
	}
	mO := app.Service.ModelObj()
	tl.Logger().Debugf("len of service messages: %d", len(mO.Messages))
	for _, m := range mO.Messages {
		tl.Logger().Debug(m)
	}
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
	assert.Equal("", hb.clusterVersion)
	assert.Equal("", hb.clusterIdentifier)
	assert.Equal(0, hb.runCount)
	ntd := &fakeTerminatedNodeHandler{}
	hb.ntd = ntd

	mObj.ClusterAttributes = hb.clusterAttrs()

	// This test fakes out Start/Stop to avoid racy scheduling issues
	hb.clusterVersion = hb.app.ClusterMD[cluster.CMDVersion]
	hb.clusterIdentifier = hb.app.ClusterMD[cluster.CMDIdentifier]
	hb.systemID = "system-1"
	hb.runCount = 0

	// skip initial csp domain fetch
	hb.mCspDomainObj = mCSPObj

	// fake list failure initially
	tl.Logger().Info("Case: cluster list failure")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	cOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(1)
	lm := mockmgmtclient.NewClusterMatcher(t, mcl.NewClusterListParams())
	lm.ListParam.ClusterIdentifier = &hb.clusterIdentifier
	lm.ListParam.CspDomainID = &app.CSPDomainID
	cOps.EXPECT().ClusterList(lm).Return(nil, fmt.Errorf("fake error")).MinTimes(1)
	mCluster := mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, gomock.Not(gomock.Nil())).Return(nil, nil).MinTimes(1)
	assert.Nil(hb.mObj)
	err := hb.Buzz(ctx)
	assert.Nil(hb.mObj)
	assert.Error(err)
	assert.Regexp("fake error", err)
	assert.Zero(hb.updateCount)

	// updated cluster, MonitorUpstream error
	mockCtrl.Finish()
	tl.Logger().Info("Case: cluster updated, monitor upstream error")
	tl.Flush()
	eM.RetMUErr = fmt.Errorf("monitor-upstream-err")
	eM.CalledMU = false
	mockCtrl = gomock.NewController(t)
	mServant := NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	mServant.EXPECT().GetAPIArgs().Return(&mgmtclient.APIArgs{})
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(1)
	cOps.EXPECT().ClusterList(lm).Return(&mcl.ClusterListOK{Payload: []*models.Cluster{mObj}}, nil).MinTimes(1)
	mObj.Service.State = app.Service.StateString(app.Service.GetState())
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, gomock.Not(gomock.Nil())).Return(nil, nil).MinTimes(1)
	fCrud.RetClUpdaterObj = mObj
	fCrud.RetClUpdaterErr = nil
	assert.Nil(hb.mObj)
	err = hb.Buzz(ctx)
	assert.Error(err)
	assert.Regexp("monitor-upstream-err", err)
	assert.NotNil(hb.mObj)
	assert.Equal(mObj, hb.mObj)
	assert.False(hb.createdUpMon)
	assert.True(eM.CalledMU)
	assert.Equal(1, hb.updateCount)

	// MonitorUpstream created; cluster heartbeat (no hbt)
	mockCtrl.Finish()
	tl.Logger().Info("Case: Cluster heartbeat (no hbt), Monitor upstream created")
	tl.Flush()
	eM.RetMUErr = nil
	eM.CalledMU = false
	mockCtrl = gomock.NewController(t)
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	mServant.EXPECT().GetAPIArgs().Return(&mgmtclient.APIArgs{})
	mObj.Service.State = app.Service.StateString(app.Service.GetState())
	fCrud.RetClUpdaterObj = mObj
	fCrud.RetClUpdaterErr = nil
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, gomock.Not(gomock.Nil())).Return(nil, nil).MinTimes(1)
	assert.NotNil(hb.mObj)
	assert.False(hb.createdUpMon)
	err = hb.Buzz(ctx)
	assert.NoError(err)
	assert.NotNil(hb.mObj)
	assert.Equal(mObj, hb.mObj)
	assert.True(eM.CalledMU)
	assert.Equal(2, hb.updateCount)
	assert.True(hb.createdUpMon)
	assert.True(eM.CalledMU)

	tl.Logger().Info("Case: GetServiceAttribute")
	tl.Flush()
	vtMS := hb.app.Service.GetServiceAttribute(com.ServiceAttrMetricMoverStatus)
	assert.NotNil(vtMS)
	assert.Equal("STRING", vtMS.Kind)
	assert.Equal(mmStatusString, vtMS.Value)

	// previous messages must be imported
	tl.Logger().Info("Case: previous messages must be imported")
	tl.Logger().Debug(hb.mObj.Service.ServiceState)
	tl.Flush()
	assert.NotNil(mObj.Service)
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
	tl.Logger().Info("Case: validate upstream args")
	tl.Flush()
	wa := hb.getUpstreamMonitorArgs()
	assert.NotNil(wa)
	assert.Len(wa.Matchers, 1)
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
	scopeS := fmt.Sprintf("abc:def clusterId:%s xyz:123", string(hb.mObj.Meta.ID))
	tl.Logger().Debugf("*** scopeS: %s", scopeS)
	assert.True(re.MatchString(scopeS))
}

func TestClusterFetchedOnStart(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log:             tl.Logger(),
			HeartbeatPeriod: 10,
			CSPDomainID:     "6a156de2-ba94-4035-b47f-e87373d75757", // flags
			ClusterType:     "kubernetes",                           // flags
			ClusterID:       "CLUSTER-1",                            // flags
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
	mObj := &models.Cluster{
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
			ClusterMutableAllOf0: models.ClusterMutableAllOf0{
				ClusterVersion: fakeClusterMD[cluster.CMDVersion],
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
			},
			ClusterCreateMutable: models.ClusterCreateMutable{
				Name:              models.ObjName(fakeClusterMD[cluster.CMDIdentifier]),
				ClusterIdentifier: fakeClusterMD[cluster.CMDIdentifier],
				State:             com.ClusterStateDeployable,
				//ClusterAttributes: get this after init from hb
			},
		},
	}
	tl.Logger().Debugf("initial messages: %d", len(mObj.Service.Messages))
	for _, m := range mObj.Service.Messages {
		tl.Logger().Debug(m)
	}
	mO := app.Service.ModelObj()
	tl.Logger().Debugf("len of service messages: %d", len(mO.Messages))
	for _, m := range mO.Messages {
		tl.Logger().Debug(m)
	}
	ctx := context.Background()

	// fake metricMover status
	fMM.RetStatus = &metricmover.Status{
		TxConfigured:      true,
		TxStarted:         true,
		VolumeIOBuffered:  10,
		VolumeIOPublished: 10,
	}
	mmStatusString := fMM.RetStatus.String()

	var err, expErr error

	fakeClusterIdentifier := "fake-cluster-identifier"
	secretObj := &cluster.SecretObjMV{
		SecretCreateArgsMV: cluster.SecretCreateArgsMV{
			Data: map[string]string{
				com.K8sClusterIdentitySecretKey: fakeClusterIdentifier,
			},
		},
	}

	// Init
	hb.Init(app)
	assert.Equal("", hb.clusterVersion)
	assert.Equal("", hb.clusterIdentifier)
	assert.Equal(0, hb.runCount)
	ntd := &fakeTerminatedNodeHandler{}
	hb.ntd = ntd

	mObj.ClusterAttributes = hb.clusterAttrs()

	// This test fakes out Start/Stop to avoid racy scheduling issues
	hb.clusterVersion = hb.app.ClusterMD[cluster.CMDVersion]
	hb.systemID = "system-1"
	hb.runCount = 0

	// skip initial csp domain fetch
	hb.mCspDomainObj = mCSPObj

	t.Log("Case: cluster fetch failure")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	expErr = errClusterNotClaimed
	fCrud.RetClUpdaterErr = expErr
	fCrud.RetClUpdaterObj = nil
	mCluster := mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordIncident(ctx, &cluster.Incident{Message: errClusterNotClaimed.Error(), Severity: cluster.IncidentFatal}).Return(nil, nil).MinTimes(1)
	mCluster.EXPECT().SecretFetchMV(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("ignored error"))
	mCluster.EXPECT().SecretCreateMV(gomock.Any(), gomock.Any()).Return(nil, nil)
	mServant := NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	mServant.EXPECT().FatalError(expErr)
	assert.Nil(hb.mObj)
	err = hb.Buzz(ctx)
	assert.Nil(hb.mObj)
	assert.Error(err)
	assert.Equal(expErr, err)
	assert.Zero(hb.updateCount)
	assert.True(hb.fatalError)
	assert.NotEmpty(hb.clusterIdentifier)
	mockCtrl.Finish()
	tl.Flush()

	t.Log("Case: cluster not found")
	hb.fatalError = false
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	expErr = errClusterNotClaimed
	fCrud.RetClUpdaterObj = nil
	fCrud.RetClUpdaterErr = errClusterNotClaimed
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordIncident(ctx, &cluster.Incident{Message: errClusterNotClaimed.Error(), Severity: cluster.IncidentFatal}).Return(nil, nil).MinTimes(1)
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	mServant.EXPECT().FatalError(expErr)
	assert.Nil(hb.mObj)
	err = hb.Buzz(ctx)
	assert.Nil(hb.mObj)
	assert.Error(err)
	assert.Equal(expErr, err)
	assert.Zero(hb.updateCount)
	assert.True(hb.fatalError)

	// cluster mismatch cases
	for _, tc := range []string{"CspDomainID", "ClusterType", "ClusterIdentifier"} {
		mockCtrl.Finish()
		tl.Flush()
		t.Log("Case: cluster mismatch", tc)
		var clObj *models.Cluster
		testutils.Clone(mObj, &clObj)
		switch tc {
		case "CspDomainID":
			clObj.CspDomainID = "foo"
		case "ClusterType":
			clObj.ClusterType = "foo"
		case "ClusterIdentifier":
			clObj.State = com.ClusterStateManaged
			clObj.ClusterIdentifier = "foo"
		}
		hb.fatalError = false
		mockCtrl = gomock.NewController(t)
		mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
		app.ClientAPI = mAPI
		expErr = errClusterMismatch
		fCrud.RetClUpdaterObj = nil
		fCrud.RetClUpdaterErr = nil
		fCrud.ForceFetchClUpdater = true
		fCrud.FetchClUpdaterObj = clObj
		mCluster = mockcluster.NewMockClient(mockCtrl)
		app.ClusterClient = mCluster
		mCluster.EXPECT().RecordIncident(ctx,
			&cluster.Incident{Message: errClusterMismatch.Error(),
				Severity: cluster.IncidentFatal}).Return(nil, nil).MinTimes(1)
		mServant = NewMockAppServant(mockCtrl)
		hb.app.AppServant = mServant
		mServant.EXPECT().FatalError(expErr)
		assert.Nil(hb.mObj)
		err = hb.Buzz(ctx)
		assert.Nil(hb.mObj)
		assert.Error(err)
		assert.Equal(expErr, err)
		assert.Zero(hb.updateCount)
		assert.True(hb.fatalError)
	}
	hb.fatalError = false

	mockCtrl.Finish()
	tl.Flush()
	t.Log("Case: secret exists incident")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	expErr = errSecretExists
	hb.clusterIdentifier = ""
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	incident := &cluster.Incident{
		Severity: cluster.IncidentFatal,
		Message:  expErr.Error(),
	}
	objectExistsError := cluster.NewK8sError("object exists", cluster.StatusReasonAlreadyExists)
	mCluster.EXPECT().RecordIncident(ctx, incident).Return(nil, nil)
	mCluster.EXPECT().SecretFetchMV(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("ignored error"))
	mCluster.EXPECT().SecretCreateMV(gomock.Any(), gomock.Any()).Return(nil, objectExistsError)
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	mServant.EXPECT().FatalError(expErr)
	assert.Nil(hb.mObj)
	err = hb.Buzz(ctx)
	assert.Error(err)
	assert.Equal(expErr, err)
	assert.Zero(hb.updateCount)
	assert.True(hb.fatalError)
	hb.mObj = nil
	hb.fatalError = false

	mockCtrl.Finish()
	tl.Flush()
	t.Log("Case: secret create error")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	var clObj *models.Cluster
	testutils.Clone(mObj, &clObj)
	hb.clusterIdentifier = ""
	expErr = errUnableToSaveSecret
	incident = &cluster.Incident{
		Severity: cluster.IncidentFatal,
		Message:  expErr.Error(),
	}
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordIncident(ctx, incident).Return(nil, nil)
	mCluster.EXPECT().SecretFetchMV(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("ignored error"))
	mCluster.EXPECT().SecretCreateMV(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("other error"))
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	mServant.EXPECT().FatalError(expErr)
	assert.Nil(hb.mObj)
	err = hb.Buzz(ctx)
	assert.Equal(expErr, err)
	assert.Equal(0, hb.updateCount)
	assert.True(hb.fatalError)
	hb.mObj = nil
	hb.fatalError = false
	hb.updateCount = 0      // reset
	hb.createdUpMon = false // reset

	mockCtrl.Finish()
	tl.Flush()
	t.Log("Case: clusterIdentifier fetched from secret, service not ready")
	app.Service.SetState(util.ServiceStarting)
	assert.False(app.IsReady())
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	testutils.Clone(mObj, &clObj)
	fCrud.RetClUpdaterErr = nil
	fCrud.RetClUpdaterObj = clObj
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil).MinTimes(1)
	mCluster.EXPECT().SecretFetchMV(gomock.Any(), gomock.Any()).Return(secretObj, nil)
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	mServant.EXPECT().GetAPIArgs().Return(&mgmtclient.APIArgs{})
	assert.Nil(hb.mObj)
	err = hb.Buzz(ctx)
	assert.NoError(err)
	assert.Equal(1, hb.updateCount)
	assert.False(hb.fatalError)
	hb.mObj = nil
	hb.updateCount = 0      // reset
	hb.createdUpMon = false // reset

	mockCtrl.Finish()
	tl.Flush()
	t.Log("Case: save secret, service ready")
	app.Service.SetState(util.ServiceReady)
	assert.True(app.IsReady())
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	testutils.Clone(mObj, &clObj)
	fCrud.RetClUpdaterErr = nil
	fCrud.RetClUpdaterObj = clObj
	hb.mObj = nil
	hb.clusterIdentifier = ""
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceReady}).Return(nil, nil).MinTimes(1)
	mCluster.EXPECT().SecretFetchMV(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("ignored error"))
	mCluster.EXPECT().SecretCreateMV(gomock.Any(), gomock.Any()).Return(nil, nil)
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	mServant.EXPECT().GetAPIArgs().Return(&mgmtclient.APIArgs{})
	err = hb.Buzz(ctx)
	assert.NoError(err)
	assert.Equal(1, hb.updateCount)
	assert.False(hb.fatalError)
	hb.mObj = nil
	hb.updateCount = 0      // reset
	hb.createdUpMon = false // reset

	mockCtrl.Finish()
	tl.Flush()
	t.Log("Case: success- saved secret, cluster found in active state")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	testutils.Clone(mObj, &clObj)
	hb.clusterIdentifier = ""
	hb.updateCount = 1
	fState := fakeState.NewFakeClusterState()
	hb.app.StateOps = fState
	fCrud.RetTaskCreateErr = fmt.Errorf("some breaking error") // to avoid second update
	clObj.ClusterIdentifier = fakeClusterIdentifier
	clObj.State = com.ClusterStateManaged
	app.ClientAPI = mAPI
	fCrud.RetClUpdaterObj = nil
	fCrud.RetClUpdaterErr = nil
	fCrud.FetchClUpdaterObj = clObj
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil).MinTimes(1)
	mCluster.EXPECT().SecretFetchMV(gomock.Any(), gomock.Any()).Return(secretObj, nil) // has fakeClusterIdentifier set
	assert.Nil(hb.mObj)
	err = hb.Buzz(ctx)
	assert.NotNil(hb.mObj)
	assert.Equal("some breaking error", err.Error())
	assert.Nil(fCrud.ModClUpdaterObj2)
	assert.Equal(1, hb.updateCount)
	assert.False(hb.fatalError)
	hb.mObj = nil
	hb.updateCount = 0      // reset
	hb.createdUpMon = false // reset

	mockCtrl.Finish()
	tl.Flush()
	t.Log("Case: saved secret, cluster found in deployable state, claimed. Forcing and error")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	testutils.Clone(mObj, &clObj)
	hb.clusterIdentifier = ""
	clObj.ClusterIdentifier = ""
	clObj.State = com.ClusterStateDeployable
	app.ClientAPI = mAPI
	fState = fakeState.NewFakeClusterState()
	hb.app.StateOps = fState
	fCrud.RetClUpdaterObj = nil
	fCrud.RetClUpdaterErr = nil
	fCrud.ForceFetchClUpdater = true
	fCrud.FetchClUpdaterObj = clObj
	fCrud.RetTaskCreateErr = fmt.Errorf("some breaking error") // to avoid second update
	hb.updateCount = 1                                         // to avoid second update
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil).MinTimes(1)
	mCluster.EXPECT().SecretFetchMV(gomock.Any(), gomock.Any()).Return(secretObj, nil)
	assert.Nil(hb.mObj)
	err = hb.Buzz(ctx)
	assert.Equal(fakeClusterIdentifier, hb.mObj.ClusterIdentifier)
	assert.Equal(com.ClusterStateManaged, hb.mObj.State)
	assert.Contains(fCrud.InClUpdaterItems.Set, "clusterIdentifier")
	assert.Contains(fCrud.InClUpdaterItems.Set, "state")
	assert.Contains(fCrud.InClUpdaterItems.Set, "messages")
	assert.NotNil(hb.mObj)
	assert.Error(err)
	assert.Equal(1, hb.updateCount)
	assert.False(hb.fatalError)
	hb.mObj = nil
	hb.updateCount = 0           // reset
	hb.createdUpMon = false      // reset
	hb.app.StateOps = nil        // reset
	fCrud.RetTaskCreateErr = nil // reset

	mockCtrl.Finish()
	tl.Flush()
	t.Log("Case: cluster found in TIMED_OUT state, update")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	testutils.Clone(mObj, &clObj)
	hb.clusterIdentifier = fakeClusterIdentifier
	clObj.ClusterIdentifier = fakeClusterIdentifier
	clObj.State = com.ClusterStateTimedOut
	app.ClientAPI = mAPI
	fState = fakeState.NewFakeClusterState()
	hb.app.StateOps = fState
	fCrud.RetClUpdaterObj = nil
	fCrud.RetClUpdaterErr = nil
	fCrud.ForceFetchClUpdater = true
	fCrud.FetchClUpdaterObj = clObj
	fCrud.RetTaskCreateErr = fmt.Errorf("some breaking error") // to avoid second update
	hb.updateCount = 1                                         // to avoid second update
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil).MinTimes(1)
	assert.Nil(hb.mObj)
	err = hb.Buzz(ctx)
	assert.Equal(fakeClusterIdentifier, hb.mObj.ClusterIdentifier)
	assert.Equal(com.ClusterStateManaged, hb.mObj.State)
	assert.Equal([]string{"state", "messages"}, fCrud.InClUpdaterItems.Set)
	assert.NotNil(hb.mObj)
	assert.Error(err)
	assert.Equal(1, hb.updateCount)
	assert.False(hb.fatalError)
	hb.mObj = nil
	hb.updateCount = 0           // reset
	hb.createdUpMon = false      // reset
	hb.app.StateOps = nil        // reset
	fCrud.RetTaskCreateErr = nil // reset

	mockCtrl.Finish()
	tl.Flush()
	t.Log("Case: success- saved secret, cluster found in bad state")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	testutils.Clone(mObj, &clObj)
	hb.clusterIdentifier = ""
	clObj.ClusterIdentifier = ""
	clObj.State = "bad state"
	app.ClientAPI = mAPI
	fCrud.RetClUpdaterObj = nil
	fCrud.RetClUpdaterErr = nil
	fCrud.ForceFetchClUpdater = true
	fCrud.FetchClUpdaterObj = clObj
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	expErr = errClusterStateUnexpected
	incident = &cluster.Incident{
		Severity: cluster.IncidentFatal,
		Message:  expErr.Error(),
	}
	mCluster.EXPECT().RecordIncident(ctx, incident).Return(nil, nil).MinTimes(1)
	mCluster.EXPECT().SecretFetchMV(gomock.Any(), gomock.Any()).Return(secretObj, nil)
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	mServant.EXPECT().FatalError(expErr)
	assert.Nil(hb.mObj)
	err = hb.Buzz(ctx)
	assert.Nil(hb.mObj)
	assert.Error(err)
	assert.Equal(0, hb.updateCount)
	assert.True(hb.fatalError)
	hb.mObj = nil
	hb.updateCount = 0           // reset
	hb.createdUpMon = false      // reset
	hb.app.StateOps = nil        // reset
	fCrud.RetTaskCreateErr = nil // reset
	hb.fatalError = false

	// updated cluster, MonitorUpstream error
	mockCtrl.Finish()
	tl.Flush()
	t.Log("Case: cluster updated, monitor upstream error")
	eM.RetMUErr = fmt.Errorf("monitor-upstream-err")
	eM.CalledMU = false
	mockCtrl = gomock.NewController(t)
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	mServant.EXPECT().GetAPIArgs().Return(&mgmtclient.APIArgs{})
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	mObj.Service.State = app.Service.StateString(app.Service.GetState())
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, gomock.Not(gomock.Nil())).Return(nil, nil).MinTimes(1)
	fCrud.RetClUpdaterObj = mObj
	fCrud.RetClUpdaterErr = nil
	assert.Nil(hb.mObj)
	err = hb.Buzz(ctx)
	assert.Regexp("monitor-upstream-err", err)
	assert.NotNil(hb.mObj)
	assert.Equal(mObj, hb.mObj)
	assert.False(hb.createdUpMon)
	assert.True(eM.CalledMU)
	assert.Equal(1, hb.updateCount)

	// MonitorUpstream created; cluster heartbeat (no hbt)
	mockCtrl.Finish()
	tl.Flush()
	t.Log("Case: Cluster heartbeat (no hbt), Monitor upstream created")
	eM.RetMUErr = nil
	eM.CalledMU = false
	mockCtrl = gomock.NewController(t)
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	mServant.EXPECT().GetAPIArgs().Return(&mgmtclient.APIArgs{})
	mObj.Service.State = app.Service.StateString(app.Service.GetState())
	fCrud.RetClUpdaterObj = mObj
	fCrud.RetClUpdaterErr = nil
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, gomock.Not(gomock.Nil())).Return(nil, nil).MinTimes(1)
	assert.NotNil(hb.mObj)
	assert.False(hb.createdUpMon)
	err = hb.Buzz(ctx)
	assert.NoError(err)
	assert.NotNil(hb.mObj)
	assert.Equal(mObj, hb.mObj)
	assert.True(eM.CalledMU)
	assert.Equal(2, hb.updateCount)
	assert.True(hb.createdUpMon)
	assert.True(eM.CalledMU)

	tl.Logger().Info("Case: GetServiceAttribute")
	tl.Flush()
	vtMS := hb.app.Service.GetServiceAttribute(com.ServiceAttrMetricMoverStatus)
	assert.NotNil(vtMS)
	assert.Equal("STRING", vtMS.Kind)
	assert.Equal(mmStatusString, vtMS.Value)

	// previous messages must be imported
	tl.Logger().Info("Case: previous messages must be imported")
	tl.Logger().Debug(hb.mObj.Service.ServiceState)
	tl.Flush()
	assert.NotNil(mObj.Service)
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
	tl.Flush()
	t.Log("Case: validate upstream args")
	wa := hb.getUpstreamMonitorArgs()
	assert.NotNil(wa)
	assert.Len(wa.Matchers, 1)
	m0 := wa.Matchers[0]
	assert.NotEmpty(m0.MethodPattern)
	t.Logf("0.MethodPattern: %s", m0.MethodPattern)
	re := regexp.MustCompile(m0.MethodPattern)
	assert.True(re.MatchString("POST"))
	assert.True(re.MatchString("PATCH"))
	assert.NotEmpty(m0.URIPattern)
	t.Logf("0.URIPattern: %s", m0.URIPattern)
	re = regexp.MustCompile(m0.URIPattern)
	assert.True(re.MatchString("/volume-series-requests"))
	assert.True(re.MatchString("/volume-series-requests/id"))
	assert.True(re.MatchString("/storage-requests"))
	assert.True(re.MatchString("/storage-requests/id"))
	assert.NotEmpty(m0.ScopePattern)
	t.Logf("0.ScopePattern: %s", m0.ScopePattern)
	re = regexp.MustCompile(m0.ScopePattern)
	scopeS := fmt.Sprintf("abc:def clusterId:%s xyz:123", string(hb.mObj.Meta.ID))
	t.Logf("*** scopeS: %s", scopeS)
	assert.True(re.MatchString(scopeS))
}

func TestClusterCreationRaceOnStart(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log:             tl.Logger(),
			HeartbeatPeriod: 10,
			CSPDomainID:     "6a156de2-ba94-4035-b47f-e87373d75757",
			ClusterType:     "kubernetes",
		},
		ClusterMD:  fakeClusterMD,
		InstanceMD: fakeNodeMD,
	}
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	eM := &fev.Manager{}
	app.CrudeOps = eM
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
	mObj := &models.Cluster{
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
			ClusterMutableAllOf0: models.ClusterMutableAllOf0{
				ClusterVersion: fakeClusterMD[cluster.CMDVersion],
				//Service: get this data from the service after Init
			},
			ClusterCreateMutable: models.ClusterCreateMutable{
				Name:              models.ObjName(fakeClusterMD[cluster.CMDIdentifier]),
				ClusterIdentifier: fakeClusterMD[cluster.CMDIdentifier],
				//ClusterAttributes: get this after init from hb
			},
		},
	}
	ctx := context.Background()

	// Init
	hb.Init(app)
	hb.createdUpMon = true // skip testing this
	assert.Equal("", hb.clusterVersion)
	assert.Equal("", hb.clusterIdentifier)
	assert.Equal(0, hb.runCount)
	ntd := &fakeTerminatedNodeHandler{}
	hb.ntd = ntd

	mObj.ClusterAttributes = hb.clusterAttrs()
	mObj.Service = app.Service.ModelObj()
	assert.NotNil(mObj.Service)

	// This test fakes out Start/Stop to avoid racy scheduling issues
	hb.clusterVersion = hb.app.ClusterMD[cluster.CMDVersion]
	hb.clusterIdentifier = hb.app.ClusterMD[cluster.CMDIdentifier]
	hb.systemID = "system-1"
	hb.runCount = 0

	// skip initial csp domain fetch
	hb.mCspDomainObj = mCSPObj

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	cOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(3)
	mCluster := mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, gomock.Not(gomock.Nil())).Return(nil, nil).MinTimes(1)
	lm := mockmgmtclient.NewClusterMatcher(t, mcl.NewClusterListParams())
	lm.ListParam.ClusterIdentifier = &hb.clusterIdentifier
	lm.ListParam.CspDomainID = &app.CSPDomainID
	// first list doesn't find it
	l1 := cOps.EXPECT().ClusterList(lm).Return(&mcl.ClusterListOK{}, nil)
	// second list succeeds
	cOps.EXPECT().ClusterList(lm).Return(&mcl.ClusterListOK{Payload: []*models.Cluster{mObj}}, nil).After(l1)
	cm := mockmgmtclient.NewClusterMatcher(t, mcl.NewClusterCreateParams())
	cm.CreateParam.Payload = &models.ClusterCreateArgs{
		ClusterCreateOnce: models.ClusterCreateOnce{
			ClusterType: mObj.ClusterType,
			CspDomainID: mObj.CspDomainID,
		},
		ClusterCreateMutable: models.ClusterCreateMutable{
			ClusterAttributes: mObj.ClusterAttributes,
			Name:              mObj.Name,
			ClusterIdentifier: mObj.ClusterIdentifier,
			State:             com.ClusterStateManaged,
		},
	}
	cErr := mcl.NewClusterCreateDefault(409)
	cErr.Payload = &models.Error{Code: 409, Message: swag.String("fake 409")}
	cOps.EXPECT().ClusterCreate(cm).Return(nil, cErr)
	mObj.Service.State = app.Service.StateString(app.Service.GetState())
	fCrud.RetClUpdaterObj = mObj
	fCrud.RetClUpdaterErr = nil
	assert.Nil(hb.mObj)
	err := hb.Buzz(ctx)
	assert.NoError(err)
	assert.NotNil(hb.mObj)
	assert.Equal(mObj, hb.mObj)
	assert.True(hb.updateCount > 0)
}

func TestClusterCreationMultipleErrors(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log:             tl.Logger(),
			HeartbeatPeriod: 10,
			CSPDomainID:     "6a156de2-ba94-4035-b47f-e87373d75757",
			ClusterType:     "kubernetes",
		},
		ClusterMD:  fakeClusterMD,
		InstanceMD: fakeNodeMD,
	}
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	svcArgs := util.ServiceArgs{
		ServiceType:         "clusterd",
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
	mObj := &models.Cluster{
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
			ClusterMutableAllOf0: models.ClusterMutableAllOf0{
				ClusterVersion: fakeClusterMD[cluster.CMDVersion],
				//Service: get this data from the service after Init
			},
			ClusterCreateMutable: models.ClusterCreateMutable{
				Name:              models.ObjName(fakeClusterMD[cluster.CMDIdentifier]),
				ClusterIdentifier: fakeClusterMD[cluster.CMDIdentifier],
				//ClusterAttributes: get this after init from hb
			},
		},
	}
	ctx := context.Background()

	// Init
	hb.Init(app)
	hb.createdUpMon = true // skip testing this
	assert.Equal("", hb.clusterVersion)
	assert.Equal("", hb.clusterIdentifier)
	assert.Equal(0, hb.runCount)
	ntd := &fakeTerminatedNodeHandler{}
	hb.ntd = ntd

	mObj.ClusterAttributes = hb.clusterAttrs()
	mObj.Service = app.Service.ModelObj()
	assert.NotNil(mObj.Service)

	// This test fakes out Start/Stop to avoid racy scheduling issues
	hb.clusterVersion = hb.app.ClusterMD[cluster.CMDVersion]
	hb.clusterIdentifier = hb.app.ClusterMD[cluster.CMDIdentifier]
	hb.systemID = "system-1"
	hb.runCount = 0

	// skip initial csp domain fetch
	hb.mCspDomainObj = mCSPObj

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	cOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(4)
	mCluster := mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).MinTimes(1)
	lm := mockmgmtclient.NewClusterMatcher(t, mcl.NewClusterListParams())
	lm.ListParam.ClusterIdentifier = &hb.clusterIdentifier
	lm.ListParam.CspDomainID = &app.CSPDomainID
	// list never finds it
	cOps.EXPECT().ClusterList(lm).Return(&mcl.ClusterListOK{}, nil).MinTimes(1)
	cm := mockmgmtclient.NewClusterMatcher(t, mcl.NewClusterCreateParams())
	cm.CreateParam.Payload = &models.ClusterCreateArgs{
		ClusterCreateOnce: models.ClusterCreateOnce{
			ClusterType: mObj.ClusterType,
			CspDomainID: mObj.CspDomainID,
		},
		ClusterCreateMutable: models.ClusterCreateMutable{
			ClusterAttributes: mObj.ClusterAttributes,
			Name:              mObj.Name,
			ClusterIdentifier: mObj.ClusterIdentifier,
			State:             com.ClusterStateManaged,
		},
	}
	cErr := fmt.Errorf("not a 409 error")
	cOps.EXPECT().ClusterCreate(cm).Return(nil, cErr).MinTimes(1)
	mObj.Service.State = app.Service.StateString(app.Service.GetState())
	assert.Nil(hb.mObj)
	assert.Equal(0, hb.runCount)
	err := hb.Buzz(ctx)
	assert.Nil(hb.mObj)
	assert.Equal(1, hb.runCount)

	err = hb.Buzz(ctx)
	assert.Nil(hb.mObj)
	assert.Equal(2, hb.runCount)

	// failure with name, name included in log message
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	tl.Flush()
	app.ClusterName = "customName"
	cm.CreateParam.Payload.Name = models.ObjName(app.ClusterName)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	cOps = mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).MinTimes(2)
	cdErr := mcl.NewClusterCreateDefault(409)
	cdErr.Payload = &models.Error{Code: 409, Message: swag.String("fake 409")}
	cOps.EXPECT().ClusterCreate(cm).Return(nil, cdErr)
	cOps.EXPECT().ClusterList(lm).Return(&mcl.ClusterListOK{}, nil)
	err = hb.createClusterObj(nil)
	assert.Error(err)
	assert.Regexp("not found", err.Error())
	foundMsg := false
	tl.Iterate(func(n uint64, m string) {
		if res, err := regexp.MatchString(app.ClusterName+".*already exists", m); err == nil && res {
			foundMsg = true
		}
	})
	assert.True(foundMsg)
}

func TestCSPDomainFailureCases(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log:             tl.Logger(),
			HeartbeatPeriod: 10,
			CSPDomainID:     "6a156de2-ba94-4035-b47f-e87373d75757",
			ClusterType:     "kubernetes",
		},
		ClusterMD:  fakeClusterMD,
		InstanceMD: fakeNodeMD,
	}
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	svcArgs := util.ServiceArgs{
		ServiceType:         "clusterd",
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
	ctx := context.Background()

	// Init
	hb.Init(app)
	hb.createdUpMon = true // skip testing this
	assert.Equal("", hb.clusterVersion)
	assert.Equal("", hb.clusterIdentifier)
	assert.Equal(0, hb.runCount)
	ntd := &fakeTerminatedNodeHandler{}
	hb.ntd = ntd

	// This test fakes out Start/Stop to avoid racy scheduling issues
	hb.clusterVersion = hb.app.ClusterMD[cluster.CMDVersion]
	hb.clusterIdentifier = hb.app.ClusterMD[cluster.CMDIdentifier]
	hb.systemID = "system-1"
	hb.runCount = 0

	// fail to fetch CSPDomain object
	t.Log("fail to fetch CSPDomain")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	cspOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cspOps).MinTimes(1)
	dm := mockmgmtclient.NewCspDomainMatcher(t, mcsp.NewCspDomainFetchParams())
	dm.FetchParam.ID = app.CSPDomainID
	cspOps.EXPECT().CspDomainFetch(dm).Return(nil, fmt.Errorf("fake error")).MinTimes(1)
	err := hb.fetchCSPDomainObj(nil)
	assert.NotNil(err)
	assert.Equal(errDomainNotFound, err)
	mockCtrl.Finish()
	tl.Flush()

	// domain type mismatch
	t.Log("CSPDomain type mismatch")
	mockCtrl = gomock.NewController(t)
	saveDT := mCSPObj.CspDomainType
	mCSPObj.CspDomainType = models.CspDomainTypeMutable("foo")
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	cspOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cspOps).MinTimes(1)
	dm = mockmgmtclient.NewCspDomainMatcher(t, mcsp.NewCspDomainFetchParams())
	dm.FetchParam.ID = app.CSPDomainID
	cspOps.EXPECT().CspDomainFetch(dm).Return(&mcsp.CspDomainFetchOK{Payload: mCSPObj}, nil).MaxTimes(1)
	mCluster := mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordIncident(ctx,
		&cluster.Incident{Message: errDomainMismatch.Error(),
			Severity: cluster.IncidentFatal}).Return(nil, nil).MinTimes(1)
	mServant := NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	asm := newAppServantMatcher(t, mockAppServantFatalError)
	asm.pattern = errDomainMismatch.Error()
	mServant.EXPECT().FatalError(asm).MinTimes(1)
	err = hb.Buzz(ctx)
	assert.NotNil(err)
	assert.Equal(errDomainMismatch, err)
	mCSPObj.CspDomainType = saveDT
	mockCtrl.Finish()
	tl.Flush()

	// initializing csp client fails application
	t.Log("InitializeCSPClient error")
	hb.fatalError = false
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	cspOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cspOps).MinTimes(1)
	dm = mockmgmtclient.NewCspDomainMatcher(t, mcsp.NewCspDomainFetchParams())
	dm.FetchParam.ID = app.CSPDomainID
	cspOps.EXPECT().CspDomainFetch(dm).Return(&mcsp.CspDomainFetchOK{Payload: mCSPObj}, nil).MaxTimes(1)
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordIncident(ctx,
		&cluster.Incident{Message: errServiceInit.Error(),
			Severity: cluster.IncidentFatal}).Return(nil, nil).MinTimes(1)
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	mServant.EXPECT().InitializeCSPClient(mCSPObj).Return(fmt.Errorf("ICC fake error"))
	asm = newAppServantMatcher(t, mockAppServantFatalError)
	asm.pattern = errServiceInit.Error()
	mServant.EXPECT().FatalError(asm).MinTimes(1)
	hb.mCspDomainObj = nil
	assert.False(hb.fatalError)
	err = hb.Buzz(ctx)
	// delay to let the async Stop() call complete
	time.Sleep(100 * time.Millisecond)
	assert.True(hb.fatalError)
	mockCtrl.Finish()
	tl.Flush()

	// fetch csp fails application
	t.Log("fail to fetch CSPDomain")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	cspOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cspOps).MinTimes(1)
	dm = mockmgmtclient.NewCspDomainMatcher(t, mcsp.NewCspDomainFetchParams())
	dm.FetchParam.ID = app.CSPDomainID
	cspOps.EXPECT().CspDomainFetch(dm).Return(nil, fmt.Errorf("dom-fetch")).MaxTimes(1)
	mCluster.EXPECT().RecordIncident(ctx,
		&cluster.Incident{Message: errDomainNotFound.Error(),
			Severity: cluster.IncidentFatal}).Return(nil, nil).MinTimes(1)
	mServant = NewMockAppServant(mockCtrl)
	hb.app.AppServant = mServant
	asm = newAppServantMatcher(t, mockAppServantFatalError)
	asm.pattern = errDomainNotFound.Error()
	mServant.EXPECT().FatalError(asm).MinTimes(1)
	hb.mCspDomainObj = nil
	hb.fatalError = false
	err = hb.Buzz(ctx)
	// delay to let the async Stop() call complete
	time.Sleep(100 * time.Millisecond)
	assert.True(hb.fatalError)

	// runBody doesn't do anything again
	err = hb.Buzz(nil)
	assert.True(hb.fatalError)
}

func TestVersionLog(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log:              tl.Logger(),
			HeartbeatPeriod:  10,
			VersionLogPeriod: 30 * time.Minute,
			CSPDomainID:      "6a156de2-ba94-4035-b47f-e87373d75757",
			ClusterType:      "kubernetes",
			SystemID:         "system-1",
			ServiceVersion:   "commit time host",
			NuvoAPIVersion:   "0.6.2",
			InvocationArgs:   "{ invocation flag values }",
		},
		InstanceMD: fakeNodeMD,
		ClusterMD:  fakeClusterMD,
	}
	eM := &fev.Manager{}
	app.CrudeOps = eM
	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM
	fCrud := &fakecrud.Client{}
	app.OCrud = fCrud
	fState := fakeState.NewFakeClusterState()
	app.StateOps = fState
	svcArgs := util.ServiceArgs{
		ServiceType:         "clusterd",
		ServiceVersion:      "test",
		HeartbeatPeriodSecs: int64(app.HeartbeatPeriod),
		Log:                 app.Log,
		MaxMessages:         app.MaxMessages,
	}
	app.Service = util.NewService(&svcArgs)
	hb := newHBComp()

	// Init
	hb.Init(app)
	assert.Equal(0, hb.runCount)
	ntd := &fakeTerminatedNodeHandler{}
	hb.ntd = ntd

	// This test fakes out Start/Stop to avoid racy scheduling issues
	hb.runCount = 0
	hb.systemID = "fake-system-id"
	hb.createdUpMon = true
	hb.mCspDomainObj = &models.CSPDomain{}
	mObj := &models.Cluster{ClusterAllOf0: models.ClusterAllOf0{Meta: &models.ObjMeta{ID: "cid"}}}
	mObj.ClusterIdentifier = fakeClusterMD[cluster.CMDIdentifier]
	hb.mObj = mObj
	hb.updateCount = 1 // steady state

	now := time.Now()
	ctx := context.Background()

	// first iteration logs
	t.Log("case1")
	fCrud.InTaskCreateObj = nil
	fCrud.RetTaskCreateErr = fmt.Errorf("task-create")
	fCrud.RetTaskCreateObj = nil
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	app.ClientAPI = mAPI
	mCluster := mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	hb.clusterIdentifier = mObj.ClusterIdentifier
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil)
	assert.Zero(hb.lastVersionLogTime)
	err := hb.Buzz(ctx)
	assert.Error(err)
	assert.Regexp("task-create", err)
	assert.True(hb.runCount == 1)
	assert.Equal(1, tl.CountPattern("INFO .*invocation flag values"))
	assert.True(now.Before(hb.lastVersionLogTime))
	prevLogTime := hb.lastVersionLogTime
	tl.Flush()

	// second interation, no logging
	t.Log("case2")
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil)
	err = hb.Buzz(ctx)
	assert.True(hb.runCount == 2)
	assert.Equal(0, tl.CountPattern("INFO .*invocation flag values"))
	assert.Equal(prevLogTime, hb.lastVersionLogTime)
	tl.Flush()

	// time is past, logs again
	t.Log("case3")
	hb.lastVersionLogTime = now.Add(-30 * time.Minute)
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil)
	err = hb.Buzz(ctx)
	assert.True(hb.runCount == 3)
	assert.Equal(1, tl.CountPattern("INFO .*invocation flag values"))
	assert.True(prevLogTime.Before(hb.lastVersionLogTime))
}

func TestIssueConsolidatedHeartbeat(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	la, err := layout.FindAlgorithm(layout.AlgorithmStandaloneNetworkedShared)
	assert.NoError(err)
	assert.NotNil(la)
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log:             tl.Logger(),
			HeartbeatPeriod: 10,
			CSPDomainID:     "6a156de2-ba94-4035-b47f-e87373d75757",
			ClusterType:     "kubernetes",
			SystemID:        "system-1",
		},
		ClusterMD:         fakeClusterMD,
		InstanceMD:        fakeNodeMD,
		StorageAlgorithms: []*layout.Algorithm{la},
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
	now := time.Now()
	mObj := &models.Cluster{
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
			ClusterMutableAllOf0: models.ClusterMutableAllOf0{
				ClusterVersion: fakeClusterMD[cluster.CMDVersion],
				ClusterUsagePolicy: &models.ClusterUsagePolicy{
					Inherited: true,
				},
				//Service: get this data from the service after Init
			},
			ClusterCreateMutable: models.ClusterCreateMutable{
				Name:              models.ObjName(fakeClusterMD[cluster.CMDIdentifier]),
				ClusterIdentifier: fakeClusterMD[cluster.CMDIdentifier],
				//ClusterAttributes: get this after init from hb
			},
		},
	}

	hb := newHBComp()
	hb.Init(app)
	hb.mObj = mObj
	ntd := &fakeTerminatedNodeHandler{}
	hb.ntd = ntd

	assert.True(hb.heartbeatTaskPeriod >= 2*hb.sleepPeriod)
	assert.Equal(int64(20), hb.heartbeatTaskPeriodSecs)
	assert.Equal(hb.heartbeatTaskPeriodSecs, hb.HeartbeatTaskPeriodSecs())

	// ensure that hb task is a no-op if StateOps not set
	tl.Logger().Info("case: state ops nil")
	tl.Flush()
	hb.app.StateOps = nil
	fCrud.InTaskCreateObj = nil
	fCrud.RetTaskCreateErr = fmt.Errorf("task-create")
	fCrud.RetTaskCreateObj = nil
	retT, err := hb.issueConsolidatedHeartbeat(nil)
	assert.NoError(err)
	assert.Nil(retT)
	assert.Nil(fCrud.InTaskCreateObj)

	// set StateOps
	fState := fakeState.NewFakeClusterState()
	app.StateOps = fState

	// ensure that hb task is a no-op if not due
	tl.Logger().Info("case: not scheduled, no change")
	tl.Flush()
	hb.lastHeartbeatTaskTime = time.Now()
	fCrud.InTaskCreateObj = nil
	fCrud.RetTaskCreateErr = fmt.Errorf("task-create")
	fCrud.RetTaskCreateObj = nil
	retT, err = hb.issueConsolidatedHeartbeat(nil)
	assert.NoError(err)
	assert.Nil(retT)
	assert.Nil(fCrud.InTaskCreateObj)

	// purged records, not scheduled, fake failure in issuing task
	tl.Logger().Info("case: not scheduled, purge change")
	tl.Flush()
	fCrud.InTaskCreateObj = nil
	fCrud.RetTaskCreateErr = fmt.Errorf("task-create")
	fCrud.RetTaskCreateObj = nil
	fState.RetNPMcnt = 1
	fState.RetNPMadd = 100
	hb.lastHealthInsertionCount = 100
	hb.lastHeartbeatTaskTime = time.Now() // not due
	lastHTT := hb.lastHeartbeatTaskTime
	retT, err = hb.issueConsolidatedHeartbeat(nil)
	assert.Error(err)
	assert.Nil(retT)
	assert.Regexp("task-create", err)
	assert.NotNil(fCrud.InTaskCreateObj)
	assert.Equal(lastHTT, hb.lastHeartbeatTaskTime)

	// added records, not scheduled, fake failure in issuing task
	tl.Logger().Info("case: not scheduled, insertion change")
	tl.Flush()
	fCrud.InTaskCreateObj = nil
	fCrud.RetTaskCreateErr = fmt.Errorf("task-create")
	fCrud.RetTaskCreateObj = nil
	fState.RetNPMcnt = 0
	fState.RetNPMadd = 101
	hb.lastHealthInsertionCount = 100
	hb.lastHeartbeatTaskTime = time.Now() // not due
	lastHTT = hb.lastHeartbeatTaskTime
	retT, err = hb.issueConsolidatedHeartbeat(nil)
	assert.Error(err)
	assert.Nil(retT)
	assert.Regexp("task-create", err)
	assert.NotNil(fCrud.InTaskCreateObj)
	assert.Equal(lastHTT, hb.lastHeartbeatTaskTime)

	// scheduled time, no change, create the task this time
	tl.Logger().Info("case: scheduled, no change")
	tl.Flush()
	nhl := []*state.NodeHealth{
		&state.NodeHealth{NodeID: "n0", State: com.ServiceStateReady, HeartbeatPeriodSecs: 10, TimeLastHeartbeat: now},
		&state.NodeHealth{NodeID: "n1", State: "NOT-READY", HeartbeatPeriodSecs: 30, TimeLastHeartbeat: now},
	}
	fState.RetNGSnh = nhl
	retTask := &models.Task{
		TaskAllOf0: models.TaskAllOf0{Meta: &models.ObjMeta{ID: "task1"}},
	}
	fCrud.InTaskCreateObj = nil
	fCrud.RetTaskCreateErr = nil
	fCrud.RetTaskCreateObj = retTask
	fState.RetNPMcnt = 0                                                   // no change
	fState.RetNPMadd = 101                                                 // no change
	hb.lastHealthInsertionCount = 101                                      // no change
	hb.lastHeartbeatTaskTime = time.Now().Add(-2 * hb.heartbeatTaskPeriod) // in the past
	lastHTT = hb.lastHeartbeatTaskTime
	tB := time.Now()
	retT, err = hb.issueConsolidatedHeartbeat(nil)
	tA := time.Now()
	assert.NoError(err)
	assert.NotNil(retT)
	assert.Equal(retTask, retT)
	assert.NotNil(fCrud.InTaskCreateObj)
	expNodePeriod := int64(hb.heartbeatTaskPeriod / time.Second)
	expHBTime := strfmt.DateTime(now)
	service := hb.app.Service.ModelObj()
	service.HeartbeatTime = expHBTime // Just in case... granularity such that this would match in the UT anyway
	expTask := &models.Task{
		TaskCreateOnce: models.TaskCreateOnce{
			ObjectID:  "CLUSTER-1",
			Operation: com.TaskClusterHeartbeat,
			ServiceStates: map[string]models.ServiceState{
				"CLUSTER-1": service.ServiceState,
				"n0":        models.ServiceState{HeartbeatPeriodSecs: expNodePeriod, HeartbeatTime: expHBTime, State: com.ServiceStateReady},
				"n1":        models.ServiceState{HeartbeatPeriodSecs: expNodePeriod, HeartbeatTime: expHBTime, State: "NOT-READY"},
			},
		},
	}
	tObj := fCrud.InTaskCreateObj
	assert.Equal(expTask.ObjectID, tObj.ObjectID)
	assert.Equal(expTask.Operation, tObj.Operation)
	assert.Len(tObj.ServiceStates, len(nhl)+1)
	for k, v := range expTask.ServiceStates {
		assert.Contains(tObj.ServiceStates, k, "key %s", k)
		tV := tObj.ServiceStates[k]
		assert.Equal(v.State, tV.State, "state[%s]", k)
		assert.Equal(v.HeartbeatPeriodSecs, tV.HeartbeatPeriodSecs, "hbs[%s]", k)
		assert.WithinDurationf(time.Time(v.HeartbeatTime), time.Time(tV.HeartbeatTime), time.Second, "time[%s]", k)
	}
	assert.WithinDuration(tA, hb.lastHeartbeatTaskTime, tA.Sub(tB))
}

// TestUpdateCluster tests updateClusterObj and UpdateState
func TestUpdateCluster(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log:             tl.Logger(),
			HeartbeatPeriod: 10,
			CSPDomainID:     "6a156de2-ba94-4035-b47f-e87373d75757",
			ClusterType:     "kubernetes",
			SystemID:        "system-1",
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
	now := time.Now()
	mObj := &models.Cluster{
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
			ClusterMutableAllOf0: models.ClusterMutableAllOf0{
				ClusterUsagePolicy: &models.ClusterUsagePolicy{
					Inherited: true,
				},
			},
		},
	}

	ctx := context.Background()

	hb := newHBComp()
	hb.Init(app)
	hb.clusterVersion = "cluster version"
	ntd := &fakeTerminatedNodeHandler{}
	hb.ntd = ntd

	t.Log("case: UpdateState - cluster not known")
	err := hb.UpdateState(ctx)
	assert.Error(err)
	assert.Regexp("not ready", err)

	t.Log("case: Updater error")
	testutils.Clone(mObj, &hb.mObj)
	hb.updateCount = 10
	fCrud.RetClUpdaterErr = fmt.Errorf("updater error")
	err = hb.UpdateState(ctx)
	assert.Error(err)
	assert.Regexp("updater error", err)
	assert.Equal(ctx, fCrud.InClUpdaterCtx)
	assert.EqualValues(hb.mObj.Meta.ID, fCrud.InClUpdaterID)
	assert.Equal([]string{"clusterVersion", "service", "clusterAttributes"}, fCrud.InClUpdaterItems.Set)
	assert.Empty(fCrud.InClUpdaterItems.Remove)
	assert.Empty(fCrud.InClUpdaterItems.Append)
	assert.Equal(0, hb.updateCount)

	t.Log("case: Update with state change")
	app.Service.SetState(util.ServiceStarting)
	testutils.Clone(mObj, &hb.mObj)
	hb.mObj.State = com.ClusterStateDeployable
	hb.updateCount = 0
	fCrud.RetClUpdaterErr = nil
	fCrud.ModClUpdaterObj = nil
	err = hb.updateClusterObj(ctx)
	assert.NoError(err)
	assert.Equal(1, hb.updateCount)
	assert.NotNil(fCrud.ModClUpdaterObj)
	assert.Equal(fCrud.ModClUpdaterObj, hb.mObj)
	assert.Equal(hb.clusterVersion, hb.mObj.ClusterVersion)
	appService := app.Service.ModelObj()
	appService.HeartbeatTime = strfmt.DateTime(time.Now())
	assert.NotNil(hb.mObj.Service)
	hb.mObj.Service.HeartbeatTime = appService.HeartbeatTime
	assert.Equal(appService, hb.mObj.Service)
	assert.EqualValues(hb.clusterAttrs(), hb.mObj.ClusterAttributes)
	assert.Equal(com.ClusterStateManaged, hb.mObj.State)
	assert.Len(hb.mObj.Messages, 1)
	assert.Regexp("State change.*", hb.mObj.Messages[0].Message)
	assert.Equal([]string{"clusterVersion", "service", "clusterAttributes", "state", "messages", "clusterUsagePolicy"}, fCrud.InClUpdaterItems.Set)

	t.Log("case: Update with TIMED_OUT state change")
	app.Service.SetState(util.ServiceStarting)
	testutils.Clone(mObj, &hb.mObj)
	hb.mObj.State = com.ClusterStateTimedOut
	hb.updateCount = 0
	fCrud.RetClUpdaterErr = nil
	fCrud.ModClUpdaterObj = nil
	err = hb.updateClusterObj(ctx)
	assert.NoError(err)
	assert.Equal(1, hb.updateCount)
	assert.NotNil(fCrud.ModClUpdaterObj)
	assert.Equal(fCrud.ModClUpdaterObj, hb.mObj)
	assert.Equal(hb.clusterVersion, hb.mObj.ClusterVersion)
	appService = app.Service.ModelObj()
	appService.HeartbeatTime = strfmt.DateTime(time.Now())
	assert.NotNil(hb.mObj.Service)
	hb.mObj.Service.HeartbeatTime = appService.HeartbeatTime
	assert.Equal(appService, hb.mObj.Service)
	assert.EqualValues(hb.clusterAttrs(), hb.mObj.ClusterAttributes)
	assert.Equal(com.ClusterStateManaged, hb.mObj.State)
	assert.Len(hb.mObj.Messages, 1)
	assert.Regexp("State change.*", hb.mObj.Messages[0].Message)
	assert.Equal([]string{"clusterVersion", "service", "clusterAttributes", "state", "messages"}, fCrud.InClUpdaterItems.Set)

	t.Log("case: Update without state change")
	app.Service.SetState(util.ServiceReady)
	testutils.Clone(mObj, &hb.mObj)
	hb.updateCount = 0
	hb.mObj.State = com.ClusterStateManaged
	fCrud.ModClUpdaterObj = nil
	err = hb.updateClusterObj(ctx)
	assert.NoError(err)
	assert.Equal(1, hb.updateCount)
	assert.NotNil(fCrud.ModClUpdaterObj)
	assert.Equal(fCrud.ModClUpdaterObj, hb.mObj)
	assert.Equal(hb.clusterVersion, hb.mObj.ClusterVersion)
	appService = app.Service.ModelObj()
	appService.HeartbeatTime = strfmt.DateTime(time.Now())
	assert.NotNil(hb.mObj.Service)
	hb.mObj.Service.HeartbeatTime = appService.HeartbeatTime
	assert.Equal(appService, hb.mObj.Service)
	assert.EqualValues(hb.clusterAttrs(), hb.mObj.ClusterAttributes)
	assert.Equal(com.ClusterStateManaged, hb.mObj.State)
	assert.Len(hb.mObj.Messages, 0)
	assert.Equal([]string{"clusterVersion", "service", "clusterAttributes"}, fCrud.InClUpdaterItems.Set)
}
