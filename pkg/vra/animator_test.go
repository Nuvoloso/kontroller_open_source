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


package vra

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

type fakeOps struct {
	vrList        []*models.VolumeSeriesRequest
	bodyCount     int
	dispatchCount int
	skipCount     int
}

func (o *fakeOps) RunBody(ctx context.Context) []*models.VolumeSeriesRequest {
	o.bodyCount++
	return o.vrList
}

func (o *fakeOps) ShouldDispatch(ctx context.Context, rhs *RequestHandlerState) bool {
	if rhs.Request.Meta.ID[0:5] == "skip-" {
		o.skipCount++
		return false
	}
	o.dispatchCount++
	return true
}

var _ = Ops(&fakeOps{})

func TestNewAnimator(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	logger := tl.Logger()
	retryInterval := time.Second * 20
	stopPeriod := StopPeriodDefault + time.Second*10
	ops := &fakeOps{}
	fa := newFakeRequestHandlers(t, tl.Logger())
	items := &crud.Updates{Set: []string{"volumeSeriesRequestState", "requestMessages"}}

	a := NewAnimator(retryInterval, logger, ops)
	assert.Equal(retryInterval, a.RetryInterval)
	assert.Equal(logger, a.Log)
	assert.Equal(ops, a.ops)
	assert.NotNil(a.allocateCapacityHandlers)
	assert.NotNil(a.allocationHandlers)
	assert.NotNil(a.attachFsHandlers)
	assert.NotNil(a.bindHandlers)
	assert.NotNil(a.cgSnapshotCreateHandlers)
	assert.NotNil(a.createHandlers)
	assert.NotNil(a.mountHandlers)
	assert.NotNil(a.nodeDeleteHandlers)
	assert.NotNil(a.renameHandlers)
	assert.NotNil(a.publishHandlers)
	assert.NotNil(a.publishServicePlanHandlers)
	assert.NotNil(a.volDetachHandlers)
	assert.NotNil(a.volSnapshotCreateHandlers)
	assert.NotNil(a.volSnapshotRestoreHandlers)
	assert.NotNil(a.activeRequests)
	assert.NotNil(a.doneRequests)
	assert.Equal(items, a.oItems)

	// test merge check
	a.oItems.Set = []string{}
	assert.Panics(func() { a.mergeSetItems([]string{"1", "2", "syncPeers", "3"}) })

	// test the merge functionality
	a.oItems.Set = []string{}
	a.mergeSetItems([]string{"1", "2", "3", "5", "7"})
	a.mergeSetItems([]string{"1", "2", "7", "11"})
	a.mergeSetItems([]string{"13", "2", "17"})
	assert.Len(a.oItems.Set, 8)
	for _, v := range []string{"1", "2", "3", "5", "7", "11", "13", "17"} {
		assert.True(util.Contains(a.oItems.Set, v), v)
	}

	// test individual "With" methods
	expItems := &crud.Updates{Set: []string{"volumeSeriesRequestState", "requestMessages", "capacityReservationPlan", "capacityReservationResult", "servicePlanAllocationId", "storageFormula", "systemTags"}}
	a = NewAnimator(retryInterval, logger, ops).WithAllocateCapacityHandlers(fa)
	assert.Equal(fa, a.allocateCapacityHandlers)
	assert.Equal(expItems, a.oItems)

	expItems = &crud.Updates{Set: []string{"volumeSeriesRequestState", "requestMessages"}}
	a = NewAnimator(retryInterval, logger, ops).WithAttachFsHandlers(fa)
	assert.Equal(fa, a.attachFsHandlers)
	assert.Equal(expItems, a.oItems)

	expItems = &crud.Updates{Set: []string{"volumeSeriesRequestState", "requestMessages"}}
	a = NewAnimator(retryInterval, logger, ops).WithCreateHandlers(fa)
	assert.Equal(fa, a.createHandlers)
	assert.Equal(expItems, a.oItems)

	expItems = &crud.Updates{Set: []string{"volumeSeriesRequestState", "requestMessages", "systemTags"}}
	a = NewAnimator(retryInterval, logger, ops).WithCreateFromSnapshotHandlers(fa)
	assert.Equal(fa, a.createFromSnapshotHandlers)
	assert.Equal(expItems, a.oItems)

	expItems = &crud.Updates{Set: []string{"volumeSeriesRequestState", "requestMessages", "capacityReservationResult", "servicePlanAllocationId", "storageFormula", "systemTags"}}
	a = NewAnimator(retryInterval, logger, ops).WithBindHandlers(fa)
	assert.Equal(fa, a.bindHandlers)
	assert.Equal(expItems, a.oItems)

	expItems = &crud.Updates{Set: []string{"volumeSeriesRequestState", "requestMessages", "capacityReservationResult", "systemTags"}}
	a = NewAnimator(retryInterval, logger, ops).WithChangeCapacityHandlers(fa)
	assert.Equal(fa, a.changeCapacityHandlers)
	assert.Equal(expItems, a.oItems)

	expItems = &crud.Updates{Set: []string{"volumeSeriesRequestState", "requestMessages", "storagePlan"}}
	a = NewAnimator(retryInterval, logger, ops).WithAllocationHandlers(fa)
	assert.Equal(fa, a.allocationHandlers)
	assert.Equal(expItems, a.oItems)

	expItems = &crud.Updates{Set: []string{"volumeSeriesRequestState", "requestMessages", "mountedNodeDevice", "systemTags"}}
	a = NewAnimator(retryInterval, logger, ops).WithMountHandlers(fa)
	assert.Equal(fa, a.mountHandlers)
	assert.Equal(expItems, a.oItems)

	expItems = &crud.Updates{Set: []string{"volumeSeriesRequestState", "requestMessages", "systemTags"}}
	a = NewAnimator(retryInterval, logger, ops).WithNodeDeleteHandlers(fa)
	assert.Equal(fa, a.nodeDeleteHandlers)
	assert.Equal(expItems, a.oItems)

	expItems = &crud.Updates{Set: []string{"volumeSeriesRequestState", "requestMessages", "systemTags"}}
	a = NewAnimator(retryInterval, logger, ops).WithRenameHandlers(fa)
	assert.Equal(fa, a.renameHandlers)
	assert.Equal(expItems, a.oItems)

	expItems = &crud.Updates{Set: []string{"volumeSeriesRequestState", "requestMessages"}}
	a = NewAnimator(retryInterval, logger, ops).WithCGSnapshotCreateHandlers(fa)
	assert.Equal(fa, a.cgSnapshotCreateHandlers)
	assert.Equal(expItems, a.oItems)

	expItems = &crud.Updates{Set: []string{"volumeSeriesRequestState", "requestMessages", "systemTags"}}
	a = NewAnimator(retryInterval, logger, ops).WithVolumeDetachHandlers(fa)
	assert.Equal(fa, a.volDetachHandlers)
	assert.Equal(expItems, a.oItems)

	expItems = &crud.Updates{Set: []string{"volumeSeriesRequestState", "requestMessages", "mountedNodeDevice", "snapshot", "lifecycleManagementData", "systemTags"}}
	a = NewAnimator(retryInterval, logger, ops).WithVolSnapshotCreateHandlers(fa)
	assert.Equal(fa, a.volSnapshotCreateHandlers)
	assert.Equal(expItems, a.oItems)

	expItems = &crud.Updates{Set: []string{"volumeSeriesRequestState", "requestMessages", "systemTags"}}
	a = NewAnimator(retryInterval, logger, ops).WithVolSnapshotRestoreHandlers(fa)
	assert.Equal(fa, a.volSnapshotRestoreHandlers)
	assert.Equal(expItems, a.oItems)

	expItems = &crud.Updates{Set: []string{"volumeSeriesRequestState", "requestMessages"}}
	a = NewAnimator(retryInterval, logger, ops).WithPublishServicePlanHandlers(fa)
	assert.Equal(fa, a.publishServicePlanHandlers)
	assert.Equal(expItems, a.oItems)

	expItems = &crud.Updates{Set: []string{"volumeSeriesRequestState", "requestMessages"}}
	a = NewAnimator(retryInterval, logger, ops).WithPublishHandlers(fa)
	assert.Equal(fa, a.publishHandlers)
	assert.Equal(expItems, a.oItems)

	assert.NotEqual(StopPeriodDefault, stopPeriod)
	assert.Equal(StopPeriodDefault, a.StopPeriod)
	a.WithStopPeriod(stopPeriod)
	assert.Equal(stopPeriod, a.StopPeriod)

	// test overlapping "With"
	expItems = &crud.Updates{Set: []string{"volumeSeriesRequestState", "requestMessages", "capacityReservationPlan", "capacityReservationResult", "servicePlanAllocationId", "storageFormula", "systemTags"}}
	a = NewAnimator(retryInterval, logger, ops).WithAllocateCapacityHandlers(fa).WithBindHandlers(fa)
	assert.Equal(fa, a.allocateCapacityHandlers)
	assert.Equal(fa, a.bindHandlers)
	assert.Equal(expItems, a.oItems)

	expItems = &crud.Updates{Set: []string{"volumeSeriesRequestState", "requestMessages", "capacityReservationResult", "servicePlanAllocationId", "storageFormula", "systemTags", "capacityReservationPlan"}}
	a = NewAnimator(retryInterval, logger, ops).WithBindHandlers(fa).WithAllocateCapacityHandlers(fa)
	assert.Equal(fa, a.bindHandlers)
	assert.Equal(fa, a.allocateCapacityHandlers)
	assert.Equal(expItems, a.oItems)

	// Notify does not panic before start
	nc := a.NotifyCount
	assert.NotPanics(func() { a.Notify() })
	assert.Equal(nc+1, a.NotifyCount)
}

func TestAnimatorMethods(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	ops := &fakeOps{}
	eM := &fev.Manager{}
	a := NewAnimator(0, tl.Logger(), ops)
	assert.Equal(RetryIntervalDefault, a.RetryInterval)

	// change the interval for the UT
	utInterval := time.Millisecond * 10
	a.RetryInterval = utInterval
	a.StopPeriod = utInterval
	assert.Equal(0, ops.bodyCount)
	crud := &crud.Client{Log: a.Log, ClientAPI: mAPI}
	a.doneRequests["fakeID"] = 42
	a.Start(crud, eM) // invokes run/RunBody asynchronously but won't do anything when fake RunBody returns an empty list
	assert.Equal(crud, a.OCrud)
	assert.Equal(eM, a.CrudeOps)
	assert.True(a.wakeCount == 0)
	// notify is inherently racy so keep trying in a loop
	// if we break out of the loop it was detected! :-)
	for a.wakeCount == 0 {
		a.CrudeNotify(crude.WatcherEvent, &crude.CrudEvent{}) // uses Notify()
		time.Sleep(utInterval / 4)
	}

	// verify RetryInterval timer wakes the run loop
	c := ops.bodyCount
	time.Sleep(utInterval * 2)
	assert.True(ops.bodyCount > c)

	// run loop should not waken via the RetryInterval for the following cases: set interval to a large duration and wait for the current interval to pass
	a.RetryInterval = time.Hour
	time.Sleep(utInterval * 2)

	// cover version handling in CrudeNotify (missing version already covered above)
	tl.Flush()
	ce := &crude.CrudEvent{Method: "PATCH", TrimmedURI: "/the/path/objId?set=x", Scope: crude.ScopeMap{}}
	ce.Scope[crude.ScopeMetaVersion] = "2"
	bc := ops.bodyCount
	c = a.NotifyCount
	a.CrudeNotify(crude.WatcherEvent, ce)
	assert.Equal(c+1, a.NotifyCount, "Notify should be called")
	// wait for loop to get past reseting doneRequests before next test
	for bc == ops.bodyCount {
		time.Sleep(utInterval)
	}

	a.doneRequests["objId"] = 3
	c = a.NotifyCount
	a.CrudeNotify(crude.WatcherEvent, ce)
	assert.Equal(c, a.NotifyCount, "Notify should be not called")
	assert.Equal(1, tl.CountPattern("objId is already done"))

	ce.Scope[crude.ScopeMetaVersion] = "invalidVersion"
	c = a.NotifyCount
	a.CrudeNotify(crude.WatcherEvent, ce)
	assert.Equal(c+1, a.NotifyCount, "Notify should be called")
	assert.Equal(1, tl.CountPattern("CrudeNotify invalid version"))
	tl.Flush()

	// fake the stop channel so Stop() will time out
	_, cancel := context.WithCancel(context.Background())
	saveCancel := a.cancelRun
	a.cancelRun = cancel
	a.Stop()

	// now really stop
	a.cancelRun = saveCancel
	a.Stop()
	assert.Empty(a.doneRequests)
	foundWakeUp := 0
	foundTimedOut := 0
	foundStopping := 0
	foundStopped := 0
	tl.Iterate(func(i uint64, s string) {
		if res, err := regexp.MatchString("awakened", s); err == nil && res {
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
}

func TestSkipProcessing(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	ops := &fakeOps{}
	a := NewAnimator(0, tl.Logger(), ops)
	assert.Zero(tl.CountPattern("."))

	assert.False(a.skipProcessing("objID", 1))
	a.doneRequests["objID"] = 2
	assert.True(a.skipProcessing("objID", 1))
	assert.Equal(1, tl.CountPattern("already done in this animator"))
	tl.Flush()

	a.doneRequests = make(map[models.ObjID]models.ObjVersion)
	a.activeRequests["objID"] = &RequestHandlerState{A: a}
	assert.True(a.skipProcessing("objID", 1))
	assert.Equal(1, tl.CountPattern("already being processed"))
	assert.Zero(tl.CountPattern("already done in this animator"))
}
