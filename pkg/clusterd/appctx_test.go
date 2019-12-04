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


package clusterd

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/autogen/restapi"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	mockcluster "github.com/Nuvoloso/kontroller/pkg/cluster/mock"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	mockcsp "github.com/Nuvoloso/kontroller/pkg/csp/mock"
	"github.com/Nuvoloso/kontroller/pkg/layout"
	fmm "github.com/Nuvoloso/kontroller/pkg/metricmover/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	fsl "github.com/Nuvoloso/kontroller/pkg/util/fake"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

type FakeComponent struct {
	t           *testing.T
	app         *AppCtx
	initCalled  bool
	startCalled bool
	stopCalled  bool
	appState    util.State
}

var _ = AppComponent(&FakeComponent{})

func (c *FakeComponent) Init(app *AppCtx) {
	c.app = app
	c.initCalled = true
}

func (c *FakeComponent) Start() {
	c.startCalled = true
	c.appState = c.app.Service.GetState()
}

func (c *FakeComponent) Stop() {
	c.stopCalled = true
	c.appState = c.app.Service.GetState()
}

// FakeStateUpdater fakes StateUpdater
type FakeStateUpdater struct {
	RetHTPSec int64

	CntUS    int
	RetUSErr error
}

// HeartbeatTaskPeriodSecs fakes its namesake
func (c *FakeStateUpdater) HeartbeatTaskPeriodSecs() int64 {
	return c.RetHTPSec
}

// UpdateState fakes its namesake
func (c *FakeStateUpdater) UpdateState(ctx context.Context) error {
	c.CntUS++
	return c.RetUSErr
}

func TestAppCtx(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	savedComps := components
	defer func() {
		components = savedComps
	}()
	components = make([]AppComponent, 0)
	clMD := map[string]string{
		cluster.CMDIdentifier:     "cluster id",
		cluster.CMDVersion:        "cluster version",
		cluster.CMDServiceIP:      "100.2.3.4",
		cluster.CMDServiceLocator: "service-pod",
	}
	iMD := map[string]string{
		csp.IMDInstanceName: "instance name",
		csp.IMDLocalIP:      "1.2.3.4",
		csp.IMDZone:         "zone",
	}
	dObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID: "CSP-DOMAIN-1",
			},
			CspDomainType: "DOMAIN-TYPE",
			CspDomainAttributes: map[string]models.ValueType{
				"zoneProp": models.ValueType{Kind: "STRING", Value: "zone"},
			},
		},
		CSPDomainMutable: models.CSPDomainMutable{},
	}
	server := &restapi.Server{}
	ctx := context.Background()

	c1 := FakeComponent{t: t}
	AppRegisterComponent(&c1)
	c2 := FakeComponent{t: t}
	AppRegisterComponent(&c2)

	la, err := layout.FindAlgorithm(layout.AlgorithmStandaloneNetworkedShared)
	assert.NoError(err)
	assert.NotNil(la)
	aa := &AppArgs{
		Log:              tl.Logger(),
		ServiceVersion:   "serviceVersion",
		Server:           server,
		DebugREI:         true,
		LayoutAlgorithms: []string{la.Name + "foo"}, // invalid
	}
	app, err := AppInit(aa)
	assert.Error(err)
	assert.False(c1.initCalled)

	aa.LayoutAlgorithms = []string{la.Name} // now valid
	app, err = AppInit(aa)
	assert.NoError(err)
	assert.True(c1.initCalled)
	assert.Equal(app, c1.app)
	assert.True(c2.initCalled)
	assert.Equal(app, c2.app)
	assert.Equal(app, app.AppServant)
	assert.NotNil(app.StateGuard)
	assert.NotNil(app.MetricMover)
	assert.NotEmpty(app.StorageAlgorithms)
	assert.NotNil(app.StackLogger)
	sl := &fsl.StackLogger{}
	app.StackLogger = sl

	app.ManagementHost = "host"
	app.ManagementPort = 8443
	app.Server.TLSCertificate = "../mgmtclient/client.crt"
	app.Server.TLSCertificateKey = "../mgmtclient/client.key"
	app.Server.TLSCACertificate = "../mgmtclient/ca.crt"
	mArgs := app.GetAPIArgs()
	assert.NotNil(mArgs)
	assert.Equal("host", mArgs.Host)
	assert.Equal(8443, mArgs.Port)
	assert.EqualValues(app.Server.TLSCertificate, mArgs.TLSCertificate)
	assert.EqualValues(app.Server.TLSCertificateKey, mArgs.TLSCertificateKey)
	assert.EqualValues(app.Server.TLSCACertificate, mArgs.TLSCACertificate)

	fMM := &fmm.MetricMover{}
	app.MetricMover = fMM

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cl := mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = cl
	cl.EXPECT().SetDebugLogger(app.Log)
	app.ClientTimeout = 11
	cl.EXPECT().SetTimeout(app.ClientTimeout)
	cl.EXPECT().MetaData(ctx).Return(clMD, nil)
	cSP := mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cSP.EXPECT().LocalInstanceMetadata().Return(iMD, nil)
	cSP.EXPECT().LocalInstanceMetadataSetTimeout(app.ClientTimeout)
	app.CSP = cSP
	err = app.Start()
	assert.Nil(err)
	assert.True(c1.startCalled)
	assert.True(c2.startCalled)
	assert.Equal(util.ServiceStarting, c1.appState)
	assert.Equal(util.ServiceStarting, app.Service.GetState())
	assert.EqualValues(clMD, app.ClusterMD)
	assert.EqualValues(iMD, app.InstanceMD)
	assert.NotNil(app.ClientAPI)
	assert.NotNil(app.OCrud)
	mmTxArgs := app.getMetricMoverTxArgs()
	assert.Equal(app.ClientAPI, mmTxArgs.API)
	assert.Equal(app.getMetricMoverTxArgs(), fMM.InCTArgs)
	assert.Equal(1, fMM.CntStartTCalled)
	assert.Equal(1, sl.StartCnt)
	tl.Flush()

	eM := &fev.Manager{}
	app.CrudeOps = eM
	fSU := &FakeStateUpdater{}
	app.StateUpdater = fSU
	app.Stop()
	assert.True(c1.stopCalled)
	assert.True(c2.stopCalled)
	assert.Equal(util.ServiceStopping, c1.appState)
	assert.Equal(util.ServiceStopped, app.Service.GetState())
	assert.True(eM.CalledTAW)
	assert.Equal(1, fMM.CntStopTCalled)
	assert.Equal(1, fSU.CntUS)
	assert.Equal(1, sl.StopCnt)

	// instance md fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	iMDErr := fmt.Errorf("iMD error")
	cSP.EXPECT().LocalInstanceMetadata().Return(nil, iMDErr)
	cSP.EXPECT().LocalInstanceMetadataSetTimeout(app.ClientTimeout)
	app.CSP = cSP
	err = app.Start()
	assert.NotNil(err)
	assert.Equal(iMDErr, err)
	assert.Equal(1, sl.StartCnt)
	tl.Flush()

	// call the real csp but fail
	mockCtrl.Finish()
	app.CSP = nil
	app.CSPDomainType = "BadDomainType"
	err = app.Start()
	assert.NotNil(err)
	assert.Regexp("unsupported CSPDomainType", err.Error())
	assert.Equal(1, sl.StartCnt)

	// cluster md fails
	mockCtrl = gomock.NewController(t)
	cl = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = cl
	cl.EXPECT().SetDebugLogger(app.Log)
	cl.EXPECT().SetTimeout(app.ClientTimeout)
	clMDErr := fmt.Errorf("clientMD error")
	cl.EXPECT().MetaData(ctx).Return(nil, clMDErr)
	cSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cSP.EXPECT().LocalInstanceMetadata().Return(iMD, nil)
	cSP.EXPECT().LocalInstanceMetadataSetTimeout(app.ClientTimeout)
	app.CSP = cSP
	err = app.Start()
	assert.NotNil(err)
	assert.Equal(clMDErr, err)
	assert.Equal(1, sl.StartCnt)
	tl.Flush()

	// call the real cluster client but fail
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cSP.EXPECT().LocalInstanceMetadata().Return(iMD, nil)
	cSP.EXPECT().LocalInstanceMetadataSetTimeout(app.ClientTimeout)
	app.CSP = cSP
	app.ClusterClient = nil
	app.ClusterType = "BadClusterType"
	err = app.Start()
	assert.NotNil(err)
	assert.Regexp("invalid cluster type", err.Error())
	assert.Equal(1, sl.StartCnt)
	tl.Flush()

	// initialize the CSP client (success)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	clSP := mockcsp.NewMockDomainClient(mockCtrl)
	cSP.EXPECT().InDomain(dObj.CspDomainAttributes, iMD).Return(nil).MaxTimes(1)
	cSP.EXPECT().Client(dObj).Return(clSP, nil).MaxTimes(1)
	app.CSP = cSP
	assert.False(app.CSPClientReady)
	app.InstanceMD = iMD
	err = app.InitializeCSPClient(dObj)
	assert.Nil(err)
	assert.True(app.CSPClientReady)
	assert.Equal(clSP, app.CSPClient)
	assert.Equal(clMD[cluster.CMDServiceIP], app.Service.GetServiceIP())
	assert.Equal(clMD[cluster.CMDServiceLocator], app.Service.GetServiceLocator())

	// initialize the CSP client (2nd time)
	err = app.InitializeCSPClient(dObj)
	assert.Nil(err)

	// initialize the CSP client (fail InDomain)
	app.CSPClientReady = false
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cSP.EXPECT().InDomain(dObj.CspDomainAttributes, iMD).Return(fmt.Errorf("InDomain fake error")).MaxTimes(1)
	app.CSP = cSP
	assert.False(app.CSPClientReady)
	err = app.InitializeCSPClient(dObj)
	assert.NotNil(err)
	assert.Regexp("InDomain fake error", err.Error())
	assert.False(app.CSPClientReady)

	// initialize the CSP client (fail ClientInit)
	app.CSPClientReady = false
	app.CSPClient = nil
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cSP.EXPECT().InDomain(dObj.CspDomainAttributes, iMD).Return(nil).MaxTimes(1)
	cSP.EXPECT().Client(dObj).Return(nil, fmt.Errorf("ClientInit fake error")).MaxTimes(1)
	app.CSP = cSP
	assert.False(app.CSPClientReady)
	err = app.InitializeCSPClient(dObj)
	assert.NotNil(err)
	assert.Regexp("ClientInit fake error", err.Error())
	assert.Nil(app.CSPClient)
	assert.False(app.CSPClientReady)

	// fatalError call
	app.Service.SetState(util.ServiceReady)
	assert.False(app.InFatalError)
	saveExitFn := appExitHook
	defer func() { appExitHook = saveExitFn }()
	var rc int
	ch := make(chan struct{})
	appExitHook = func(code int) {
		rc = code
		close(ch)
	}
	app.FatalError(fmt.Errorf("first fatal error"))
	assert.True(app.InFatalError)
	assert.Equal(1, app.FatalErrorCount)
	// second call to fatalError should be ignored
	app.FatalError(fmt.Errorf("second fatal error"))
	assert.Equal(2, app.FatalErrorCount)
	select {
	case <-ch:
		assert.Equal(1, rc)
	case <-time.After(100 * time.Millisecond):
		assert.False(true, "appExitHook called")
	}
	assert.Equal(util.ServiceStopped, app.Service.GetState())
	// validate that the second call to fatalError was ignored
	var foundFirst, foundSecond bool
	tl.Iterate(func(n uint64, m string) {
		if res, err := regexp.MatchString("first fatal error", m); err == nil && res {
			foundFirst = true
		}
		if res, err := regexp.MatchString("second fatal error", m); err == nil && res {
			foundSecond = true
		}
	})
	assert.True(foundFirst)
	assert.False(foundSecond)
}

func TestServicePlanCache(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	server := &restapi.Server{}
	aa := &AppArgs{
		Log:            tl.Logger(),
		ServiceVersion: "serviceVersion",
		Server:         server,
	}
	app, err := AppInit(aa)
	assert.NoError(err)

	fc := &fake.Client{}
	app.OCrud = fc

	spObj := &models.ServicePlan{}
	ctx := context.Background()
	spID := "service-plan"

	tl.Logger().Infof("case: empty map, fetch error")
	fc.RetSvPFetchErr = fmt.Errorf("service-plan-fetch")
	assert.Nil(app.servicePlanMap)
	sp, err := app.GetServicePlan(ctx, spID)
	assert.Error(err)
	assert.Regexp("service-plan-fetch", err)
	assert.Nil(sp)

	tl.Logger().Infof("case: insert into empty map")
	fc.RetSvPFetchErr = nil
	fc.RetSvPFetchObj = spObj
	assert.Nil(app.servicePlanMap)
	sp, err = app.GetServicePlan(ctx, spID)
	assert.NoError(err)
	assert.Equal(spObj, sp)
	assert.Equal(spID, fc.InSvPFetchID)
	assert.NotEmpty(app.servicePlanMap)
	sp, ok := app.servicePlanMap[spID]
	assert.True(ok)
	assert.Equal(spObj, sp)

	tl.Logger().Infof("case: fetch from map")
	fc.RetSvPFetchErr = fmt.Errorf("service-plan-fetch")
	sp, err = app.GetServicePlan(ctx, spID)
	assert.NoError(err)
	assert.Equal(spObj, sp)

	tl.Logger().Infof("case: fetch from map (repeat)")
	fc.RetSvPFetchErr = fmt.Errorf("service-plan-fetch")
	sp, err = app.GetServicePlan(ctx, spID)
	assert.NoError(err)
	assert.Equal(spObj, sp)
}

func TestAppFlags(t *testing.T) {
	assert := assert.New(t)

	args := AppArgs{}
	parser := flags.NewParser(&args, flags.Default&^flags.PrintErrors)
	_, err := parser.ParseArgs([]string{})
	assert.Error(err)
	assert.Regexp("required flags.*-D.*-M.*-S.*were not specified", err)

	// min required args
	parser = flags.NewParser(&args, flags.Default&^flags.PrintErrors)
	_, err = parser.ParseArgs([]string{"-D", "Domain", "-M", "ManagementHost", "-S", "SystemID"})
	assert.NoError(err)
	// the following reflect the fake argv
	assert.Equal("Domain", args.CSPDomainID)
	assert.Equal("SystemID", args.SystemID)
	assert.Equal("ManagementHost", args.ManagementHost)
	// the following are from the flag defaults
	assert.Equal("kubernetes", args.ClusterType)
	assert.Equal(20, args.ClientTimeout)
	assert.Equal("", args.ClusterName)
	assert.Equal("", args.ClusterID)
	assert.EqualValues("", args.CSISocket)
	assert.Equal("AWS", args.CSPDomainType)
	assert.Equal(443, args.ManagementPort)
	assert.Equal(30*time.Second, args.MetricRetryInterval)
	assert.Equal(1000, args.MetricVolumeIOMax)
	assert.Equal(30, args.HeartbeatPeriod)
	assert.Equal(10, args.HeartbeatTaskPeriodMultiplier)
	assert.Equal(30*time.Minute, args.VersionLogPeriod)
	assert.Equal(0, args.MaxMessages)
	assert.False(args.DebugClient)
	assert.Equal(5*time.Minute, args.CGSnapshotSchedPeriod)
	assert.Equal(float64(0.9), args.CGSnapshotTimeoutMultiplier)
	assert.NotEmpty(args.LayoutAlgorithms)
	assert.Equal("cluster-identifier", args.ClusterIdentifierSecretName)
	assert.Equal("nuvoloso-cluster", args.ClusterNamespace)
	assert.Equal(5*time.Minute, args.ReleaseStorageRequestTimeout)
	for _, name := range args.LayoutAlgorithms {
		la, err := layout.FindAlgorithm(name)
		assert.NoError(err, "algorithm: %s", name)
		assert.NotNil(la, "algorithm: %s", name)
	}

	// min required + env
	defer func() { os.Unsetenv("NUVO_CLUSTER_NAME") }()
	os.Setenv("NUVO_CLUSTER_NAME", "MyCluster")
	parser = flags.NewParser(&args, flags.Default&^flags.PrintErrors)
	_, err = parser.ParseArgs([]string{"-D", "Domain", "-M", "ManagementHost", "-S", "SystemID"})
	assert.NoError(err)
	// the following reflect the fake argv
	assert.Equal("Domain", args.CSPDomainID)
	assert.Equal("SystemID", args.SystemID)
	assert.Equal("ManagementHost", args.ManagementHost)
	// the following reflect the environment
	assert.EqualValues("MyCluster", args.ClusterName)
}

func TestAppIsReady(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	server := &restapi.Server{}
	aa := &AppArgs{
		Log:            tl.Logger(),
		ServiceVersion: "serviceVersion",
		Server:         server,
	}
	app, err := AppInit(aa)
	assert.NoError(err)
	app.Service.SetState(util.ServiceReady)
	ready := app.IsReady()
	assert.True(ready)

	app.Service.SetState(util.ServiceNotReady)
	ready = app.IsReady()
	assert.False(ready)
}
