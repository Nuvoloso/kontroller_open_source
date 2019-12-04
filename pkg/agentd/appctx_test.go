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


package agentd

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
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	mockcsp "github.com/Nuvoloso/kontroller/pkg/csp/mock"
	mm "github.com/Nuvoloso/kontroller/pkg/metricmover"
	fmm "github.com/Nuvoloso/kontroller/pkg/metricmover/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	mockNuvo "github.com/Nuvoloso/kontroller/pkg/nuvoapi/mock"
	"github.com/Nuvoloso/kontroller/pkg/pstore"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	fsl "github.com/Nuvoloso/kontroller/pkg/util/fake"
	"github.com/go-openapi/swag"
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

func TestAppFlags(t *testing.T) {
	assert := assert.New(t)

	args := AppArgs{}
	parser := flags.NewParser(&args, flags.Default&^flags.PrintErrors)
	_, err := parser.ParseArgs([]string{})
	assert.Error(err)
	assert.Regexp("required flags.*-D.*-M.*-S.*were not specified", err)

	defer func() { os.Unsetenv("NUVO_FV_INIT") }()
	os.Setenv("NUVO_FV_INI", "/path/to/ini/file")
	parser = flags.NewParser(&args, flags.Default&^flags.PrintErrors)
	_, err = parser.ParseArgs([]string{"-D", "Domain", "-M", "ManagementHost", "-S", "SystemID", "--nuvo-vol-dir", "/foo/bar"})
	assert.NoError(err)
	// the following reflect the fake argv
	assert.Equal("Domain", args.CSPDomainID)
	assert.Equal("SystemID", args.SystemID)
	assert.Equal("ManagementHost", args.ManagementHost)
	assert.EqualValues("/foo/bar", args.NuvoVolDirPath)
	// the following reflect the fake environment
	assert.EqualValues("/path/to/ini/file", args.NuvoFVIni)
	// the following are from the flag defaults
	parser = flags.NewParser(&args, flags.Default&^flags.PrintErrors)
	_, err = parser.ParseArgs([]string{"-D", "Domain", "-M", "ManagementHost", "-S", "SystemID"})
	assert.NoError(err)
	assert.Equal("kubernetes", args.ClusterType)
	assert.Equal(20, args.ClientTimeout)
	assert.EqualValues("", args.CSISocket)
	assert.Equal("", args.ClusterID)
	assert.Equal("AWS", args.CSPDomainType)
	assert.Equal(443, args.ManagementPort)
	assert.Equal(30*time.Second, args.MetricUploadRetryInterval)
	assert.Equal(100, args.MetricVolumeIOMax)
	assert.Equal(50, args.MetricStorageIOMax)
	assert.Equal(5*time.Minute, args.MetricIoCollectionPeriod)
	assert.Equal(5*time.Minute, args.MetricIoCollectionTruncate)
	assert.Equal(30, args.HeartbeatPeriod)
	assert.Equal(30*time.Minute, args.VersionLogPeriod)
	assert.Equal(2, args.NuvoStartupPollPeriod)
	assert.Equal(30, args.NuvoStartupPollMax)
	assert.Equal(0, args.MaxMessages)
	assert.EqualValues("/var/run/nuvoloso/nuvo.sock", args.NuvoSocket)
	assert.Equal(32145, args.NuvoPort)
	assert.EqualValues("/var/local/nuvoloso/luns", args.NuvoVolDirPath)
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

	assert.False(rei.Enabled)

	aa := &AppArgs{
		Log:            tl.Logger(),
		ServiceVersion: "serviceVersion",
		Server:         server,
		NuvoSocket:     "socket",
		DebugREI:       true,
		ClientTimeout:  1,
	}
	app := AppInit(aa)
	assert.True(c1.initCalled)
	assert.Equal(app, c1.app)
	assert.True(c2.initCalled)
	assert.Equal(app, c2.app)
	assert.Equal(app, app.AppServant)
	assert.NotNil(app.LMDGuard)
	assert.Equal([]string{}, app.LMDGuard.Status())
	assert.NotNil(app.MetricMover)
	assert.True(rei.Enabled)
	assert.Equal(time.Second, app.clientAPITimeout)
	assert.NotNil(app.StackLogger)
	sl := &fsl.StackLogger{}
	app.StackLogger = sl

	app.ManagementHost = "host"
	app.ManagementPort = 8443
	app.Server.TLSCertificate = "../mgmtclient/client.crt"
	app.Server.TLSCertificateKey = "../mgmtclient/client.key"
	app.Server.TLSCACertificate = "../mgmtclient/ca.crt"
	mArgs := app.GetCentraldAPIArgs()
	assert.NotNil(mArgs)
	assert.Equal("host", mArgs.Host)
	assert.Equal(8443, mArgs.Port)
	assert.EqualValues(app.Server.TLSCertificate, mArgs.TLSCertificate)
	assert.EqualValues(app.Server.TLSCertificateKey, mArgs.TLSCertificateKey)
	assert.EqualValues(app.Server.TLSCACertificate, mArgs.TLSCACertificate)

	os.Unsetenv("CLUSTERD_SERVICE_HOST")
	os.Unsetenv("CLUSTERD_SERVICE_PORT")
	clArgs := app.GetClusterdAPIArgs()
	assert.NotNil(clArgs)
	assert.Equal(mArgs, clArgs)

	os.Setenv("CLUSTERD_SERVICE_HOST", "clusterhost")
	os.Setenv("CLUSTERD_SERVICE_PORT", "9999")
	clArgs = app.GetClusterdAPIArgs()
	assert.NotNil(clArgs)
	assert.NotEqual(mArgs, clArgs)
	assert.Equal("clusterhost", clArgs.Host)
	assert.Equal(9999, clArgs.Port)
	clArgs.Host = mArgs.Host
	clArgs.Port = mArgs.Port
	assert.Equal(mArgs, clArgs)

	clAPI, err := app.GetClusterdAPI()
	assert.NoError(err)
	assert.NotNil(clAPI)
	assert.Equal(clAPI, app.ClusterdAPI) // gets cached
	clAPI2, err := app.GetClusterdAPI()
	assert.Equal(app.ClusterdAPI, clAPI2) // from cache

	mmTxArgs := app.getMetricMoverTxArgs()
	assert.Equal(app.ClusterdAPI, mmTxArgs.API)

	fMM := &fmm.MetricMover{}
	fMM.RetStatus = &mm.Status{}
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
	app.ClusterdAPI = nil
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
	crudClient, ok := app.OCrud.(*crud.Client)
	assert.True(ok)
	assert.Equal(app.ClientAPI, crudClient.ClientAPI)
	assert.NotNil(app.NuvoAPI)
	assert.NotNil(app.ClusterdAPI) // gets cached during start
	assert.NotNil(app.ClusterdOCrud)
	crudClient, ok = app.ClusterdOCrud.(*crud.Client)
	assert.True(ok)
	assert.Equal(app.ClusterdAPI, crudClient.ClientAPI)
	assert.Equal(app.getMetricMoverTxArgs(), fMM.InCTArgs)
	assert.Equal(1, fMM.CntStartTCalled)
	assert.Equal(1, sl.StartCnt)
	tl.Flush()

	eM := &fev.Manager{}
	app.CrudeOps = eM
	fsu := &fakeStateUpdater{}
	app.StateUpdater = fsu
	app.Stop()
	assert.True(c1.stopCalled)
	assert.True(c2.stopCalled)
	assert.Equal(util.ServiceStopping, c1.appState)
	assert.Equal(util.ServiceStopped, app.Service.GetState())
	assert.Equal(1, fMM.CntStopTCalled)
	assert.Equal(1, fsu.usCalled)
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

	// test ContextWithClientTimeout
	assert.NotZero(app.clientAPITimeout)
	tB := time.Now()
	ctx, cf := app.ContextWithClientTimeout(context.Background())
	tA := time.Now()
	defer cf()
	dl, dlOk := ctx.Deadline()
	assert.True(dlOk)
	assert.WithinDuration(tB.Add(app.clientAPITimeout), dl, tA.Sub(tB))
	app.clientAPITimeout = 0
	ctx, cf = app.ContextWithClientTimeout(context.Background())
	defer cf()
	dl, dlOk = ctx.Deadline()
	assert.False(dlOk)
}

func TestInitializeNuvo(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	aa := &AppArgs{
		Log:            tl.Logger(),
		ServiceVersion: "serviceVersion",
		Server:         &restapi.Server{},
		NuvoSocket:     "socket",
	}
	app := AppInit(aa)
	assert.Equal(0, app.nuvoInitCount)

	fr := &fakeRecoverer{}
	app.AppRecoverStorage = fr
	app.AppRecoverVolume = fr
	ctx := context.Background()

	var err error

	// test getPSOArgs
	app.DebugREI = true
	app.psoArgs = nil
	psArgs := app.getPSOArgs()
	assert.Equal(app.Log, psArgs.Log)
	assert.Equal(app.NuvoAPI, psArgs.NuvoAPI)
	assert.True(psArgs.DebugREI)
	assert.True(psArgs.Validate())

	app.DebugREI = false
	app.psoArgs = nil
	psArgs = app.getPSOArgs()
	assert.Equal(app.Log, psArgs.Log)
	assert.Equal(app.NuvoAPI, psArgs.NuvoAPI)
	assert.False(psArgs.DebugREI)
	assert.True(psArgs.Validate())

	////
	// First initialization
	////
	tl.Logger().Info("case: AppObjects not set")
	tl.Flush()
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	app.Service.SetState(util.ServiceNotReady) // fake initial state to skip node update on first pass
	mCluster := mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil).MinTimes(1)
	app.AppObjects = nil
	err = app.InitializeNuvo(ctx)
	assert.Regexp("node UUID is not yet known", err)
	assert.Nil(app.PSO)
	assert.Equal(0, app.nuvoInitCount)

	tl.Logger().Info("case: AppObjects set, node UUID not known")
	tl.Flush()
	fa := &fakeAppObjects{}
	app.AppObjects = fa
	fsu := &fakeStateUpdater{}
	app.StateUpdater = fsu
	err = app.InitializeNuvo(ctx)
	assert.Regexp("node UUID is not yet known", err)
	assert.Nil(app.PSO)
	assert.Equal(0, app.nuvoInitCount)

	// Node id now known...
	fa.retNode = &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "n1",
			},
		},
	}

	// NoVolumesInUse error
	tl.Logger().Info("case: NoVolumesInUse fails")
	tl.Flush()
	fr.RetCNVErr = fmt.Errorf("no-volumes-in-use")
	err = app.InitializeNuvo(ctx)
	assert.True(fr.CalledCNV)
	assert.Error(err)
	assert.Regexp("no-volumes-in-use", err)
	assert.Nil(app.PSO)
	assert.Equal(0, app.nuvoInitCount)
	assert.False(app.IsReady())
	assert.Equal(0, fsu.usCalled)
	fr.RetCNVErr = nil

	// UseNodeUUID error
	mockCtrl.Finish()
	tl.Logger().Info("case: UseNodeUUID fails")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	nvAPI := mockNuvo.NewMockNuvoVM(mockCtrl)
	app.NuvoAPI = nvAPI
	nvAPI.EXPECT().UseNodeUUID("n1").Return(fmt.Errorf("use node uuid error"))
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil).MinTimes(1)
	err = app.InitializeNuvo(ctx)
	assert.Regexp("use node uuid error", err)
	assert.Nil(app.PSO)
	assert.Equal(0, app.nuvoInitCount)
	assert.False(app.IsReady())
	assert.Equal(0, fsu.usCalled)

	// nuvoUseCacheDevices error
	fa.retNode.LocalStorage = map[string]models.NodeStorageDevice{
		"u1": models.NodeStorageDevice{
			DeviceName:  "/dev/1",
			DeviceState: "UNUSED",
			DeviceType:  "SSD",
			SizeBytes:   swag.Int64(1024),
		},
	}
	nvAPI.EXPECT().UseNodeUUID("n1").Return(nil).AnyTimes() // clear UseNodeUUID error
	nvAPI.EXPECT().UseCacheDevice("u1", "/dev/1").Return(uint64(0), uint64(0), fmt.Errorf("UseCacheDeviceError"))
	err = app.InitializeNuvo(ctx)
	assert.Regexp("UseCacheDeviceError", err)
	fa.retNode.LocalStorage = nil // reset
	tl.Flush()

	// recover Storage but error on RecoverVolumesInUse
	tl.Logger().Info("case: RecoverVolumesInUse fails")
	tl.Flush()
	fr.RetRDIUstg = []*Storage{
		&Storage{StorageID: "S1", DeviceName: "d1", CspStorageType: "T1"},
	}
	fr.RetRDIUerr = nil
	fr.RetRVIUluns = nil
	fr.RetRVIUerr = fmt.Errorf("rviu-error")
	err = app.InitializeNuvo(ctx)
	assert.Error(err)
	assert.Regexp("rviu-error", err)
	assert.Equal(1, tl.CountPattern("set nuvo service node UUID.*n1"))
	assert.Nil(app.PSO)
	assert.Equal(0, app.nuvoInitCount)
	assert.Len(app.lunMap, 0)
	assert.Len(app.storageMap, 0)
	assert.False(app.IsReady())
	assert.Equal(0, fsu.usCalled)

	// fail to create PSO (fatal error)
	tl.Logger().Info("case: PSO creation failure")
	tl.Flush()
	assert.False(app.InFatalError)
	app.FatalErrorCount = 1
	app.InFatalError = true                // avoid calling Stop
	app.psoArgs = &pstore.ControllerArgs{} // force failure of create
	fr.RetRVIUluns = []*LUN{}
	fr.RetRVIUerr = nil
	fr.CalledRDIU = false
	fr.CalledRVIU = false
	err = app.InitializeNuvo(ctx)
	assert.Error(err)
	assert.Regexp("unable to create PSO", err)
	assert.Equal(1, tl.CountPattern("set nuvo service node UUID.*n1"))
	assert.Nil(app.PSO)
	assert.NotNil(app.psoArgs)
	assert.False(app.IsReady())
	assert.Equal(0, fsu.usCalled)
	assert.Equal(2, app.FatalErrorCount)
	app.FatalErrorCount = 0  // reset
	app.InFatalError = false // reset
	app.psoArgs = nil        // reset
	tl.Flush()

	// success on first initialization
	mockCtrl.Finish()
	tl.Logger().Info("case: success on first initialization")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceReady}).Return(nil, nil).After(
		mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil),
	)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	app.NuvoAPI = nvAPI
	nvAPI.EXPECT().UseNodeUUID("n1").Return(nil).AnyTimes()
	fr.RetRVIUluns = []*LUN{
		&LUN{VolumeSeriesID: "VS1", SnapIdentifier: "HEAD"},
	}
	fr.RetRVIUerr = nil
	fr.CalledRDIU = false
	fr.CalledRVIU = false
	assert.False(fr.CalledSR)
	assert.False(fr.CalledVR)
	assert.NoError(app.InitializeNuvo(ctx))
	assert.Equal(1, tl.CountPattern("set nuvo service node UUID.*n1"))
	assert.NotNil(app.PSO)
	assert.True(fr.CalledRDIU)
	assert.True(fr.CalledRVIU)
	assert.Equal(1, app.nuvoInitCount)
	assert.Len(app.lunMap, 1)
	assert.NotNil(app.FindLUN("VS1", "HEAD"))
	assert.Len(app.storageMap, 1)
	assert.NotNil(app.FindStorage("S1"))
	assert.True(app.IsReady())
	assert.Equal(1, fsu.usCalled)
	assert.True(fr.CalledSR)
	assert.True(fr.CalledVR)

	mockCtrl.Finish()
	tl.Flush()

	////
	// Nuvo restart
	////
	assert.Equal(1, app.nuvoInitCount)
	app.lunMap = nil
	app.storageMap = nil
	app.recState = nil
	fr.CalledSR = false
	fr.CalledVR = false

	tl.Logger().Info("case: failed update node state")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil).AnyTimes()
	fsu.usCalled = 0
	fsu.retUS = fmt.Errorf("update-node-error")
	err = app.InitializeNuvo(ctx)
	assert.Error(err)
	assert.Regexp("update-node-error", err)
	assert.Equal(1, fsu.usCalled)
	assert.False(app.IsReady())
	assert.Nil(app.recState)

	tl.Logger().Info("case: failed to change volume state")
	tl.Flush()
	app.AddLUN(&LUN{VolumeSeriesID: "vs1", SnapIdentifier: "snap1"})
	assert.Len(app.lunMap, 1)
	fsu.usCalled = 0
	fsu.retUS = nil
	app.Service.SetState(util.ServiceReady)
	fr.CalledNVIU = false
	fr.RetNVIUstg = nil
	fr.RetNVIUerr = fmt.Errorf("nviu-error")
	err = app.InitializeNuvo(ctx)
	assert.Error(err)
	assert.True(fr.CalledNVIU)
	assert.Regexp("nviu-error", err)
	assert.Len(app.lunMap, 1)
	assert.False(app.IsReady())
	assert.Equal(1, fsu.usCalled)
	assert.NotNil(app.recState)
	assert.Empty(app.recState.volStorageIDs)
	mockCtrl.Finish()

	tl.Logger().Info("case: clear lun map, nuvo not yet ready, state maintained")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	app.NuvoAPI = nvAPI
	nvAPI.EXPECT().UseNodeUUID("n1").Return(fmt.Errorf("use node uuid error"))
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil).AnyTimes()
	app.AddStorage(&Storage{StorageID: "stg1", DeviceName: "d", CspStorageType: "st"})
	assert.Len(app.storageMap, 1)
	app.Service.SetState(util.ServiceReady)
	fsu.usCalled = 0
	fr.CalledNVIU = false
	fr.RetNVIUstg = []string{"stg1", "stg2", "stg1"} // contains duplicate
	fr.RetNVIUerr = nil
	err = app.InitializeNuvo(ctx)
	assert.Error(err)
	assert.Regexp("use node uuid error", err)
	assert.Len(app.lunMap, 0)
	assert.Len(app.storageMap, 1)
	assert.False(app.IsReady())
	assert.Equal(1, fsu.usCalled)
	assert.True(fr.CalledNVIU)
	assert.NotNil(app.recState)
	assert.NotEmpty(app.recState.volStorageIDs)
	assert.Contains(app.recState.volStorageIDs, "stg1")
	assert.Contains(app.recState.volStorageIDs, "stg2")

	nvAPI.EXPECT().UseNodeUUID("n1").Return(nil).AnyTimes() // succeed for the rest

	tl.Logger().Info("case: clear lun map, fail to configure storage")
	tl.Flush()
	app.recState = nil
	app.AddStorage(&Storage{StorageID: "stg1", DeviceName: "d", CspStorageType: "st"})
	assert.Len(app.storageMap, 1)
	app.Service.SetState(util.ServiceReady)
	fsu.usCalled = 0
	fr.CalledNVIU = false
	fr.RetNVIUstg = nil
	fr.RetNVIUerr = nil
	fr.RetCSerr = fmt.Errorf("cs-error")
	err = app.InitializeNuvo(ctx)
	assert.Error(err)
	assert.Regexp("cs-error", err)
	assert.Len(app.lunMap, 0)
	assert.Len(app.storageMap, 1)
	assert.False(app.IsReady())
	assert.Equal(1, fsu.usCalled)

	tl.Logger().Info("case: reconfigure storage - recover storage locations fails")
	tl.Flush()
	app.recState = nil
	app.lunMap = nil
	app.AddLUN(&LUN{VolumeSeriesID: "vs1", SnapIdentifier: "snap1"})
	assert.Len(app.lunMap, 1)
	app.storageMap = nil
	app.AddStorage(&Storage{StorageID: "stg1", DeviceName: "d", CspStorageType: "st"})
	assert.Len(app.storageMap, 1)
	app.Service.SetState(util.ServiceReady)
	fsu.usCalled = 0
	fr.RetCSerr = nil
	fr.InCSid = ""
	fr.RetNVIUstg = []string{"stg1"}
	fr.RetRSLerr = fmt.Errorf("rsl-error")
	err = app.InitializeNuvo(ctx)
	assert.Error(err)
	assert.Regexp("rsl-error", err)
	assert.Len(app.storageMap, 1)
	assert.Equal("stg1", fr.InCSid)
	assert.Equal([]string{"stg1"}, fr.InRSLids)
	assert.False(app.IsReady())
	assert.Equal(1, fsu.usCalled)
	assert.NotNil(app.recState)
	assert.NotEmpty(app.recState.volStorageIDs)
	assert.Contains(app.recState.volStorageIDs, "stg1")

	tl.Logger().Info("case: success: reconfigure storage - recover storage locations, prev state cleared")
	tl.Flush()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	app.NuvoAPI = nvAPI
	nvAPI.EXPECT().UseNodeUUID("n1").Return(nil).AnyTimes()
	mCluster = mockcluster.NewMockClient(mockCtrl)
	app.ClusterClient = mCluster
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceNotReady}).Return(nil, nil)
	mCluster.EXPECT().RecordCondition(ctx, &cluster.Condition{Status: cluster.ServiceReady}).Return(nil, nil)
	assert.NotNil(app.recState) // prev state present
	app.lunMap = nil
	app.AddLUN(&LUN{VolumeSeriesID: "vs1", SnapIdentifier: "snap1"})
	assert.Len(app.lunMap, 1)
	app.storageMap = nil
	app.AddStorage(&Storage{StorageID: "stg1", DeviceName: "d", CspStorageType: "st"})
	assert.Len(app.storageMap, 1)
	app.Service.SetState(util.ServiceReady)
	fsu.usCalled = 0
	fr.RetCSerr = nil
	fr.InCSid = ""
	fr.RetNVIUstg = []string{"stg1"}
	fr.RetRSLerr = nil
	assert.False(fr.CalledSR)
	assert.False(fr.CalledVR)
	err = app.InitializeNuvo(ctx)
	tl.Flush()
	assert.NoError(err)
	assert.Len(app.storageMap, 1)
	assert.Equal("stg1", fr.InCSid)
	assert.Equal([]string{"stg1"}, fr.InRSLids)
	assert.True(app.IsReady())
	assert.Equal(2, fsu.usCalled)
	assert.Nil(app.recState) // state cleared
	assert.True(fr.CalledSR)
	assert.True(fr.CalledVR)

	tl.Flush()

	// test refresh via NuvoNotInitializedError callback
	assert.Equal(0, fsu.rCalled)
	app.NuvoNotInitializedError(nil)
	assert.Equal(1, fsu.rCalled)
}

func TestNuvoUseCacheDevices(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	aa := &AppArgs{
		Log:            tl.Logger(),
		ServiceVersion: "serviceVersion",
		Server:         &restapi.Server{},
		NuvoSocket:     "socket",
	}
	app := AppInit(aa)
	mObj := &models.Node{}
	fa := &fakeAppObjects{retNode: mObj}
	app.AppObjects = fa

	// no cache
	ret, err := app.nuvoUseCacheDevices(nil, mObj)
	assert.NoError(err)
	assert.Equal(mObj, ret)

	// NUVOAPI error
	mObj.LocalStorage = map[string]models.NodeStorageDevice{
		"u1": models.NodeStorageDevice{
			DeviceName:  "/dev/1",
			DeviceState: "UNUSED",
			DeviceType:  "SSD",
			SizeBytes:   swag.Int64(1024),
		},
	}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	nvAPI := mockNuvo.NewMockNuvoVM(mockCtrl)
	app.NuvoAPI = nvAPI
	nvAPI.EXPECT().UseCacheDevice("u1", "/dev/1").Return(uint64(0), uint64(0), fmt.Errorf("UseCacheDeviceError"))
	ret, err = app.nuvoUseCacheDevices(nil, mObj)
	assert.Nil(ret)
	assert.Regexp("UseCacheDeviceError", err)
	tl.Flush()

	// success, cover various update paths
	mObj.LocalStorage["u2"] = models.NodeStorageDevice{
		DeviceName:      "/dev/2",
		DeviceState:     "CACHE",
		DeviceType:      "SSD",
		SizeBytes:       swag.Int64(2048),
		UsableSizeBytes: swag.Int64(1024),
	}
	mObj.LocalStorage["u3"] = models.NodeStorageDevice{
		DeviceName:  "/dev/3",
		DeviceState: "RESTRICTED",
		DeviceType:  "SSD",
		SizeBytes:   swag.Int64(1024),
	}
	mObj.LocalStorage["u4"] = models.NodeStorageDevice{
		DeviceName:      "/dev/4",
		DeviceState:     "CACHE",
		DeviceType:      "SSD",
		SizeBytes:       swag.Int64(2048),
		UsableSizeBytes: swag.Int64(1024),
	}
	// order guaranteed
	cPrev := nvAPI.EXPECT().UseCacheDevice("u1", "/dev/1").Return(uint64(768), uint64(768), nil)
	cPrev = nvAPI.EXPECT().UseCacheDevice("u2", "/dev/2").Return(uint64(0), uint64(768), nil).After(cPrev)
	nvAPI.EXPECT().UseCacheDevice("u4", "/dev/4").Return(uint64(1024), uint64(800), nil).After(cPrev)
	ret, err = app.nuvoUseCacheDevices(nil, mObj)
	assert.Equal(mObj, ret)
	assert.Equal(int64(800), swag.Int64Value(mObj.CacheUnitSizeBytes))
	assert.Equal(models.NodeStorageDevice{
		DeviceName:      "/dev/1",
		DeviceState:     "CACHE",
		DeviceType:      "SSD",
		SizeBytes:       swag.Int64(1024),
		UsableSizeBytes: swag.Int64(768),
	}, mObj.LocalStorage["u1"])
	assert.Equal(models.NodeStorageDevice{
		DeviceName:      "/dev/2",
		DeviceState:     "UNUSED",
		DeviceType:      "SSD",
		SizeBytes:       swag.Int64(2048),
		UsableSizeBytes: swag.Int64(0),
	}, mObj.LocalStorage["u2"])
	assert.Equal(models.NodeStorageDevice{
		DeviceName:  "/dev/3",
		DeviceState: "RESTRICTED",
		DeviceType:  "SSD",
		SizeBytes:   swag.Int64(1024),
	}, mObj.LocalStorage["u3"])
	assert.Equal(models.NodeStorageDevice{
		DeviceName:      "/dev/4",
		DeviceState:     "CACHE",
		DeviceType:      "SSD",
		SizeBytes:       swag.Int64(2048),
		UsableSizeBytes: swag.Int64(1024),
	}, mObj.LocalStorage["u4"])
	assert.NoError(err)
}

type fakeRecoverer struct {
	CalledRDIU  bool
	RetRDIUstg  []*Storage
	RetRDIUerr  error
	InCSid      string
	RetCSerr    error
	InRSLids    []string
	RetRSLerr   error
	CalledRVIU  bool
	RetRVIUluns []*LUN
	RetRVIUerr  error
	CalledNVIU  bool
	RetNVIUstg  []string
	RetNVIUerr  error
	CalledSR    bool
	CalledVR    bool
	CalledCNV   bool
	RetCNVErr   error
}

func (fr *fakeRecoverer) RecoverStorageInUse(ctx context.Context) ([]*Storage, error) {
	fr.CalledRDIU = true
	return fr.RetRDIUstg, fr.RetRDIUerr
}

func (fr *fakeRecoverer) ConfigureStorage(ctx context.Context, stg *Storage, sObj *models.Storage) error {
	fr.InCSid = stg.StorageID
	return fr.RetCSerr
}

func (fr *fakeRecoverer) RecoverStorageLocations(ctx context.Context, stgIDs []string) error {
	fr.InRSLids = stgIDs
	return fr.RetRSLerr
}

func (fr *fakeRecoverer) RecoverVolumesInUse(ctx context.Context) ([]*LUN, error) {
	fr.CalledRVIU = true
	return fr.RetRVIUluns, fr.RetRVIUerr
}

func (fr *fakeRecoverer) NoVolumesInUse(ctx context.Context) ([]string, error) {
	fr.CalledNVIU = true
	return fr.RetNVIUstg, fr.RetNVIUerr
}

func (fr *fakeRecoverer) StorageRecovered() {
	fr.CalledSR = true
}

func (fr *fakeRecoverer) VolumesRecovered() {
	fr.CalledVR = true
}

func (fr *fakeRecoverer) CheckNuvoVolumes(ctx context.Context) error {
	fr.CalledCNV = true
	return fr.RetCNVErr
}

func TestLUNCache(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	aa := &AppArgs{
		Log:            tl.Logger(),
		ServiceVersion: "serviceVersion",
		Server:         &restapi.Server{},
		NuvoSocket:     "socket",
	}
	app := AppInit(aa)
	assert.Nil(app.lunMap)
	assert.Nil(app.FindLUN("foo", "bar")) // no panic on empty map
	ll := app.GetLUNs()
	assert.NotNil(ll)
	assert.Len(ll, 0)

	invalidLUNs := []LUN{
		LUN{},
		LUN{VolumeSeriesID: "id"},
	}
	for i, tc := range invalidLUNs {
		assert.Error(tc.Validate(), "invalid lun %d", i)
		assert.Panics(func() { app.AddLUN(&tc) })
	}

	l1 := &LUN{
		VolumeSeriesID: "VS-1",
		SnapIdentifier: com.VolMountHeadIdentifier,
	}
	l2 := &LUN{
		VolumeSeriesID: "VS-1",
		SnapIdentifier: "snap1",
	}

	app.AddLUN(l1)
	app.AddLUN(l2)
	assert.Len(app.lunMap, 2)
	assert.Equal(l1, app.FindLUN(l1.VolumeSeriesID, l1.SnapIdentifier))
	assert.Equal(l2, app.FindLUN(l2.VolumeSeriesID, l2.SnapIdentifier))
	assert.Nil(app.FindLUN("foo", "bar"))

	var l1Clone = LUN{}
	testutils.Clone(l1, &l1Clone)
	assert.Equal(*l1, l1Clone)
	app.AddLUN(&l1Clone)
	assert.Len(app.lunMap, 2)
	la := app.GetLUNs()
	assert.Len(la, 2)
	assert.Contains(la, l1)
	assert.Contains(la, l2)

	app.RemoveLUN(l2.VolumeSeriesID, l2.SnapIdentifier)
	assert.Len(app.lunMap, 1)
	assert.Nil(app.FindLUN(l2.VolumeSeriesID, l2.SnapIdentifier))
	la = app.GetLUNs()
	assert.Len(la, 1)
	assert.Contains(la, l1)
	assert.NotContains(la, l2)

	app.RemoveLUN("foo", "bar") // ignored
	assert.Len(app.lunMap, 1)
}

func TestStorageCache(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	aa := &AppArgs{
		Log:            tl.Logger(),
		ServiceVersion: "serviceVersion",
		Server:         &restapi.Server{},
		NuvoSocket:     "socket",
	}
	app := AppInit(aa)
	assert.Nil(app.storageMap)
	assert.Nil(app.FindStorage("foo")) // no panic on empty map
	sl := app.GetStorage()
	assert.NotNil(sl)
	assert.Len(sl, 0)

	invalidStorage := []Storage{
		Storage{},
		Storage{StorageID: "id"},
		Storage{StorageID: "id", DeviceName: "d"},
	}
	for i, tc := range invalidStorage {
		assert.Error(tc.Validate(), "invalid storage %d", i)
		assert.Panics(func() { app.AddStorage(&tc) })
	}

	s1 := &Storage{
		StorageID:      "S-1",
		DeviceName:     "d1",
		CspStorageType: "st",
	}
	s2 := &Storage{
		StorageID:      "S-2",
		DeviceName:     "d1",
		CspStorageType: "st",
	}

	app.AddStorage(s1)
	app.AddStorage(s2)
	assert.Len(app.storageMap, 2)
	assert.Equal(s1, app.FindStorage(s1.StorageID))
	assert.Equal(s2, app.FindStorage(s2.StorageID))
	assert.Nil(app.FindStorage("foo"))

	var s1Clone = Storage{}
	testutils.Clone(s1, &s1Clone)
	assert.Equal(*s1, s1Clone)
	app.AddStorage(&s1Clone)
	assert.Len(app.storageMap, 2)
	sa := app.GetStorage()
	assert.Len(sa, 2)
	assert.Contains(sa, s1)
	assert.Contains(sa, s2)

	app.RemoveStorage(s2.StorageID)
	assert.Len(app.storageMap, 1)
	assert.Nil(app.FindStorage(s2.StorageID))
	sa = app.GetStorage()
	assert.Len(sa, 1)
	assert.Contains(sa, s1)
	assert.NotContains(sa, s2)

	app.RemoveStorage("foo") // ignored
	assert.Len(app.storageMap, 1)
}

func TestServicePlanCache(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	aa := &AppArgs{
		Log:            tl.Logger(),
		ServiceVersion: "serviceVersion",
		Server:         &restapi.Server{},
		NuvoSocket:     "socket",
	}
	app := AppInit(aa)

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

	// Test RPO intercept
	spObj = &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			Meta: &models.ObjMeta{ID: "SP-1"},
		},
		ServicePlanMutable: models.ServicePlanMutable{
			Name: "General Virtualized Applications",
			Slos: map[string]models.RestrictedValueType{
				"RPO": models.RestrictedValueType{ValueType: models.ValueType{Kind: "DURATION", Value: "4h"}},
			},
		},
	}
	app.DebugREI = false
	rpoDur := app.GetRPO(spObj)
	expDur := 4 * time.Hour
	assert.Equal(expDur, rpoDur)
	app.DebugREI = true
	rpoDur = app.GetRPO(spObj)
	expDur = 4 * time.Hour
	assert.Equal(expDur, rpoDur)
	app.reiSP.SetProperty(string(spObj.Name)+"-rpo", &rei.Property{StringValue: "2h"}) // single use
	rpoDur = app.GetRPO(spObj)
	expDur = 2 * time.Hour
	assert.Equal(expDur, rpoDur)
	rpoDur = app.GetRPO(spObj)
	expDur = 4 * time.Hour
	assert.Equal(expDur, rpoDur)
}

type fakeAppObjects struct {
	retNode *models.Node
}

func (o *fakeAppObjects) GetNode() *models.Node {
	return o.retNode
}

func (o *fakeAppObjects) GetCluster() *models.Cluster {
	return nil
}

func (o *fakeAppObjects) GetCspDomain() *models.CSPDomain {
	return nil
}

func (o *fakeAppObjects) UpdateNodeLocalStorage(ctx context.Context, ephemeral map[string]models.NodeStorageDevice, cacheUnitSizeBytes int64) (*models.Node, error) {
	o.retNode.LocalStorage = ephemeral
	o.retNode.CacheUnitSizeBytes = swag.Int64(cacheUnitSizeBytes)
	return o.retNode, nil
}

type fakeStateUpdater struct {
	rCalled  int
	retUS    error
	usCalled int
}

func (o *fakeStateUpdater) UpdateState(ctx context.Context) error {
	o.usCalled++
	return o.retUS
}

func (o *fakeStateUpdater) Refresh() {
	o.rCalled++
}
