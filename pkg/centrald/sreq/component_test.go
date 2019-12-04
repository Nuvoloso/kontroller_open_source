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

	mcp "github.com/Nuvoloso/kontroller/pkg/autogen/client/pool"
	mcs "github.com/Nuvoloso/kontroller/pkg/autogen/client/storage"
	mcsr "github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestPoolInit(t *testing.T) {
	assert := assert.New(t)
	args := Args{
		RetryInterval: time.Second * 20,
	}
	// This actually calls centrald.AppRegisterComponent but we can't intercept that
	c := PoolInit(&args)
	assert.Equal(args.RetryInterval, c.RetryInterval)

	// Notify() should not panic before Start()
	c.Notify()
}

func TestComponentMethods(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM

	// init fails to create a watcher
	ri := time.Duration(10 * time.Second)
	c := Component{
		Args: Args{RetryInterval: ri},
	}
	evM.RetWErr = fmt.Errorf("watch-error")
	c.Init(app)
	assert.Empty(c.watcherID)
	assert.EqualValues(ri, c.RetryInterval)
	assert.Equal(app, c.App)
	assert.Equal(app.Log, c.Log)
	assert.False(c.fatalError)

	// init creates a watcher
	c = Component{
		Args: Args{RetryInterval: ri},
	}
	evM.RetWErr = nil
	evM.RetWID = "watcher-id"
	c.Init(app)
	assert.Equal("watcher-id", c.watcherID)
	assert.EqualValues(WatcherRetryMultiplier*ri, c.RetryInterval)
	assert.Equal(app, c.App)
	assert.Equal(app.Log, c.Log)
	assert.False(c.fatalError)
	assert.NotNil(c.activeRequests)
	assert.NotNil(c.doneRequests)

	// change the interval for the UT
	utInterval := time.Millisecond * 10
	c.RetryInterval = utInterval
	assert.Equal(0, c.runCount)
	c.fatalError = true
	c.doneRequests["fakeID"] = 42
	c.Start() // invokes run/runBody asynchronously but won't do anything if c.fatalErr
	assert.True(c.wakeCount == 0)
	assert.True(c.notifyCount == 0)
	// notify is inherently racy so keep trying in a loop
	// if we break out of the loop it was detected! :-)
	for c.wakeCount == 0 {
		c.CrudeNotify(crude.WatcherEvent, &crude.CrudEvent{}) // uses Notify()
		time.Sleep(utInterval / 4)
	}

	// fake the stop channel so Stop() will time out
	_, cancel := context.WithCancel(context.Background())
	saveCancel := c.cancelRun
	c.cancelRun = cancel
	c.Stop()

	// now really stop
	c.cancelRun = saveCancel
	c.Stop()
	assert.Empty(c.doneRequests)
	foundWakeUp := 0
	foundTimedOut := 0
	foundStopping := 0
	foundStopped := 0
	tl.Iterate(func(i uint64, s string) {
		if res, err := regexp.MatchString("woken up", s); err == nil && res {
			foundWakeUp++
		}
		if res, err := regexp.MatchString("Timed out", s); err == nil && res {
			foundTimedOut++
		}
		if res, err := regexp.MatchString("Stopping", s); err == nil && res {
			foundStopping++
		}
		if res, err := regexp.MatchString("Stopped", s); err == nil && res {
			foundStopped++
		}
	})
	assert.True(foundWakeUp > 0)
	assert.Equal(2, foundStopping)
	assert.Equal(1, foundTimedOut)
	assert.Equal(2, foundStopped)

	// validate watcher patterns
	wa := c.getWatcherArgs()
	assert.NotNil(wa)
	assert.Len(wa.Matchers, 3)

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
	assert.Empty(m.MethodPattern)
	assert.NotEmpty(m.URIPattern)
	tl.Logger().Debugf("1.URIPattern: %s", m.URIPattern)
	re = regexp.MustCompile(m.URIPattern)
	assert.True(re.MatchString("/pools"))
	assert.True(re.MatchString("/pools/id"))
	assert.Empty(m.ScopePattern)

	m = wa.Matchers[2]
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
	assert.True(re.MatchString("foo:bar storageRequestState:REMOVING_TAG abc:123"))
	assert.True(re.MatchString("foo:bar storageRequestState:UNDO_PROVISIONING abc:123"))
	assert.True(re.MatchString("foo:bar storageRequestState:UNDO_ATTACHING abc:123"))
	assert.True(re.MatchString("foo:bar storageRequestState:UNDO_DETACHING abc:123"))
}

func TestComponentRunBody(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM

	c := Component{}
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc
	assert.Equal("", c.systemID)

	// Run body - error loading system obj
	fc.RetLSErr = fmt.Errorf("system load error")
	assert.Equal("", c.systemID)
	c.runBody(nil)
	assert.Equal("", c.systemID)

	// Run body - system object loads, startup logic cases
	fc = &fake.Client{}
	c.oCrud = fc
	assert.Equal("", c.systemID)
	fc.RetLSObj = &models.System{
		SystemAllOf0: models.SystemAllOf0{
			Meta: &models.ObjMeta{
				ID: "SYSTEM",
			},
		},
	}
	fc.RetPoolListErr = fmt.Errorf("pool list error")
	assert.False(c.startupCleanupDone)
	c.runBody(nil)
	assert.EqualValues(fc.RetLSObj.Meta.ID, c.systemID)
	assert.False(c.startupCleanupDone)
	fc.RetPoolListErr = nil
	fc.RetPoolListObj = &mcp.PoolListOK{
		Payload: make([]*models.Pool, 0),
	}
	fc.RetLsSErr = fmt.Errorf("storage list error")
	c.runBody(nil)
	assert.EqualValues(fc.RetLSObj.Meta.ID, c.systemID)
	assert.False(c.startupCleanupDone)
	fc.RetLsSErr = nil
	fc.RetLsSOk = &mcs.StorageListOK{
		Payload: make([]*models.Storage, 0),
	}
	fc.RetPoolListErr = fmt.Errorf("pool list error")
	c.runBody(nil)
	assert.False(c.startupCleanupDone)
	fc.RetPoolListErr = nil
	fc.RetPoolListObj = &mcp.PoolListOK{
		Payload: make([]*models.Pool, 0),
	}
	fc.InLsSRObj = nil
	fc.RetLsSRObj = &mcsr.StorageRequestListOK{
		Payload: make([]*models.StorageRequest, 0),
	}
	expLsSR := mcsr.NewStorageRequestListParams()
	expLsSR.IsTerminated = swag.Bool(false) // list active requests only
	c.runBody(nil)
	assert.True(c.startupCleanupDone)
	assert.Equal(expLsSR, fc.InLsSRObj)
}
