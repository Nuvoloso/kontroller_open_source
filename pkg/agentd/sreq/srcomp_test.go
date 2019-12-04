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


package sreq

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	fAppServant "github.com/Nuvoloso/kontroller/pkg/agentd/fake"
	mcsr "github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

var fakeNodeMD = map[string]string{
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

// fake object fetcher
type fakeAppObject struct {
	n   *models.Node
	cl  *models.Cluster
	csp *models.CSPDomain
}

var _ = agentd.AppObjects(&fakeAppObject{})

func (fao *fakeAppObject) GetNode() *models.Node { return fao.n }

func (fao *fakeAppObject) GetCluster() *models.Cluster { return fao.cl }

func (fao *fakeAppObject) GetCspDomain() *models.CSPDomain { return fao.csp }

func (fao *fakeAppObject) UpdateNodeLocalStorage(ctx context.Context, ephemeral map[string]models.NodeStorageDevice, cacheUnitSizeBytes int64) (*models.Node, error) {
	fao.n.LocalStorage = ephemeral
	fao.n.CacheUnitSizeBytes = swag.Int64(cacheUnitSizeBytes)
	return fao.n, nil
}

func (fao *fakeAppObject) UpdateNodeObj(ctx context.Context) error {
	return nil
}

func TestComponentInit(t *testing.T) {
	assert := assert.New(t)
	args := Args{
		RetryInterval: 20 * time.Second,
	}
	// This actually calls agentd.AppRegisterComponent but we can't intercept that
	c := ComponentInit(&args)
	assert.Equal(args.RetryInterval, c.RetryInterval)
}

func TestComponentMethods(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	fao := &fakeAppObject{}
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log:             tl.Logger(),
			HeartbeatPeriod: 10,
			CSPDomainID:     "6a156de2-ba94-4035-b47f-e87373d75757",
		},
		InstanceMD: fakeNodeMD,
		ClusterMD:  fakeClusterMD,
		AppObjects: fao,
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	app.OCrud = &fake.Client{}
	fAS := &fAppServant.AppServant{}
	app.AppServant = fAS

	args := &Args{
		RetryInterval: 20 * time.Second,
	}

	// init fails to create a watcher
	c := ComponentInit(args)
	evM.RetWErr = fmt.Errorf("watch-error")
	c.Init(app)
	assert.Empty(c.watcherID)
	assert.Equal(app.Log, c.Log)
	assert.Equal(c, c.app.AppRecoverStorage)

	// init creates a watcher
	evM.RetWErr = nil
	evM.RetWID = "watcher-id"
	c = ComponentInit(args)
	assert.NotPanics(func() { c.Init(app) })
	assert.Equal("watcher-id", c.watcherID)
	assert.Equal(app.Log, c.Log)
	assert.Equal(app, c.app)
	assert.Equal(WatcherRetryMultiplier*args.RetryInterval, c.RetryInterval)
	assert.Equal(c, c.requestHandlers)
	assert.Nil(c.cancelRun)
	assert.Nil(c.stoppedChan)
	assert.NotNil(c.activeRequests)
	assert.NotNil(c.doneRequests)

	// reasonable delays for this test
	utInterval := 10 * time.Millisecond
	c.RetryInterval = utInterval
	c.doneRequests["fakeID"] = 42
	assert.Equal(0, c.runCount)

	assert.Nil(c.oCrud)
	c.Start()
	tl.Flush()
	assert.NotNil(c.cancelRun)
	assert.Nil(c.stoppedChan)
	assert.NotNil(c.oCrud)
	assert.True(c.wakeCount == 0)
	assert.True(c.notifyCount == 0)
	// notify is inherently racy so keep trying in a loop
	// if we break out of the loop it was detected! :-)
	for c.wakeCount == 0 {
		c.CrudeNotify(crude.WatcherEvent, &crude.CrudEvent{}) // uses Notify()
		time.Sleep(utInterval / 4)
	}

	// verify sleepPeriod timer wakes the run loop
	count := c.runCount
	time.Sleep(2 * utInterval)
	assert.True(c.runCount > count)

	c.Stop()
	tl.Flush()
	assert.NotNil(c.stoppedChan)
	assert.Panics(func() { close(c.stoppedChan) }) // channel is closed
	assert.Empty(c.doneRequests)

	// exercise the timed out on stop path
	c.stoppedChan = make(chan struct{})
	_, cf := context.WithCancel(context.Background())
	c.cancelRun = cf
	c.Stop()

	// don't actually dispatch
	fAS.RetIsReady = false
	c.runCount = 0

	// check that thisNodeID gets set once only
	assert.Empty(c.thisNodeID)
	c.runBody(nil)
	assert.Equal(1, c.runCount)
	assert.Empty(c.thisNodeID)
	fao.n = &models.Node{NodeAllOf0: models.NodeAllOf0{Meta: &models.ObjMeta{ID: "THIS-NODE"}, NodeIdentifier: "NodeId"}}
	c.runBody(nil)
	assert.Equal(2, c.runCount)
	assert.EqualValues("THIS-NODE", c.thisNodeID)
	assert.Equal("NodeId", c.thisNodeIdentifier)
	fao.n = &models.Node{NodeAllOf0: models.NodeAllOf0{Meta: &models.ObjMeta{ID: "THAT-NODE"}, NodeIdentifier: "NodeId"}}
	c.runBody(nil)
	assert.Equal(3, c.runCount)
	assert.EqualValues("THIS-NODE", c.thisNodeID)

	tl.Flush()
	fc := &fake.Client{}

	// test listing and dispatch invocation
	tl.Logger().Infof("case: dispatch")
	tl.Flush()
	fc = &fake.Client{}
	c.oCrud = fc
	fc.RetLsSRObj = &mcsr.StorageRequestListOK{Payload: []*models.StorageRequest{}}
	fAS.RetIsReady = true
	c.runBody(nil)
	lParams := mcsr.NewStorageRequestListParams()
	lParams.NodeID = swag.String(string(c.thisNodeID))
	lParams.IsTerminated = swag.Bool(false)
	assert.Equal(lParams, fc.InLsSRObj)

	// validate watcher patterns
	wa := c.getWatcherArgs()
	assert.NotNil(wa)
	assert.Len(wa.Matchers, 2)

	m := wa.Matchers[0]
	assert.NotEmpty(m.MethodPattern)
	tl.Logger().Debugf("0.MethodPattern: %s", m.MethodPattern)
	re := regexp.MustCompile(m.MethodPattern)
	assert.True(re.MatchString("POST"))
	assert.NotEmpty(m.URIPattern)
	tl.Logger().Debugf("0.URIPattern: %s", m.URIPattern)
	re = regexp.MustCompile(m.URIPattern)
	assert.True(re.MatchString("/storage-requests"))
	assert.False(re.MatchString("/storage-requests/"))
	assert.Empty(m.ScopePattern)

	m = wa.Matchers[1]
	assert.NotEmpty(m.MethodPattern)
	tl.Logger().Debugf("2.MethodPattern: %s", m.MethodPattern)
	re = regexp.MustCompile(m.MethodPattern)
	assert.True(re.MatchString("PATCH"))
	assert.NotEmpty(m.URIPattern)
	tl.Logger().Debugf("2.URIPattern: %s", m.URIPattern)
	re = regexp.MustCompile(m.URIPattern)
	assert.True(re.MatchString("/storage-requests/id"))
	assert.NotEmpty(m.ScopePattern)
	tl.Logger().Debugf("2.ScopePattern: %s", m.ScopePattern)
	re = regexp.MustCompile(m.ScopePattern)
	assert.True(re.MatchString("foo:bar storageRequestState:FORMATTING abc:123"))
	assert.True(re.MatchString("foo:bar storageRequestState:USING abc:123"))
	assert.False(re.MatchString("foo:bar storageRequestState:PROVISIONING abc:123"))
}
