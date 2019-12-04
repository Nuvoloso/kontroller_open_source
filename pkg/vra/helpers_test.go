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
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/crude"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/pstore"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestGetFirstErrorMessage(t *testing.T) {
	assert := assert.New(t)

	assert.Empty(GetFirstErrorMessage(nil))

	ml := []*models.TimestampedString{}
	assert.Empty(GetFirstErrorMessage(ml))

	now := strfmt.DateTime(time.Now())
	ml = append(ml, &models.TimestampedString{Message: "not an Error", Time: now})
	assert.Empty(GetFirstErrorMessage(ml))

	ml = append(ml, &models.TimestampedString{Message: "Error: this is an error", Time: now})
	assert.Equal("Error: this is an error", GetFirstErrorMessage(ml))

	ml = append(ml, &models.TimestampedString{Message: "Error: this is another error", Time: now})
	assert.Equal("Error: this is an error", GetFirstErrorMessage(ml))
}

func TestSetVolumeState(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	ops := &fakeOps{}
	a := NewAnimator(0, tl.Logger(), ops)
	fc := &fake.Client{}
	a.OCrud = fc

	vs := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID:      "vs1",
				Version: 1,
			},
			RootParcelUUID: "uuid",
		},
		VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
			AccountID: "account1",
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				VolumeSeriesState: com.VolStateUnbound,
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				SizeBytes: swag.Int64(10737418240),
			},
		},
	}
	vr := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "vr1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			RequestedOperations: []string{"BIND", "MOUNT"},
			CompleteByTime:      strfmt.DateTime(time.Now().Add(time.Hour)),
			ClusterID:           "cluster1",
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "BINDING",
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				VolumeSeriesID: "vs1",
			},
		},
	}
	rhs := &RequestHandlerState{
		A:            a,
		Request:      vr,
		VolumeSeries: vs,
		HasCreate:    util.Contains(vr.RequestedOperations, com.VolReqOpCreate),
		HasBind:      util.Contains(vr.RequestedOperations, com.VolReqOpBind),
		HasMount:     util.Contains(vr.RequestedOperations, com.VolReqOpMount),
		Canceling:    vr.CancelRequested,
	}
	rhs.VSUpdater = rhs

	fc.RetVSUpdaterUpdateErr = fmt.Errorf("update-volume-error")
	assert.Equal("UNBOUND", vs.VolumeSeriesState)
	assert.Len(vs.Messages, 0)
	assert.Len(rhs.Request.RequestMessages, 0)
	rhs.SetVolumeState(nil, "BOUND")
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	assert.Equal("BOUND", vs.VolumeSeriesState)
	assert.Len(vs.Messages, 1)
	assert.Regexp("⇒ BOUND", vs.Messages[0].Message)
	assert.Len(rhs.Request.RequestMessages, 1)
	assert.Regexp("update-volume-error", rhs.Request.RequestMessages[0].Message)
	assert.Equal([]string{"volumeSeriesState", "messages"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Append)
	assert.Nil(fc.InVSUpdaterItems.Remove)

	// plan-only, no update
	fc.RetVSUpdaterUpdateErr = nil
	fc.InVSUpdaterItems = nil
	vr.PlanOnly = swag.Bool(true)
	rhs.RetryLater = false
	rhs.InError = false
	rhs.SetVolumeState(nil, "BOUND")
	assert.Nil(fc.InVSUpdaterItems)

	// no state change, skip update
	fc.InVSUpdaterItems = nil
	vr.PlanOnly = nil
	vs.VolumeSeriesState = "BOUND"
	rhs.RetryLater = false
	rhs.InError = false
	rhs.SetVolumeState(nil, "BOUND")
	assert.Equal([]string{"volumeSeriesState", "messages"}, fc.InVSUpdaterItems.Set)
	assert.Regexp(errVSUpdateRetryAborted, fc.ModVSUpdaterErr)

	// state change
	fc.InVSUpdaterItems = nil
	rhs.RetryLater = false
	rhs.InError = false
	rhs.Request.RequestMessages = []*models.TimestampedString{}
	vs.VolumeSeriesState = "UNBOUND"
	vs.Messages = []*models.TimestampedString{}
	rhs.SetVolumeState(nil, "BOUND")
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Equal("BOUND", vs.VolumeSeriesState)
	assert.Len(vs.Messages, 1)
	assert.Regexp("⇒ BOUND", vs.Messages[0].Message)
	assert.Len(rhs.Request.RequestMessages, 0)
	assert.Equal([]string{"volumeSeriesState", "messages"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Append)
	assert.Nil(fc.InVSUpdaterItems.Remove)
	assert.Equal(vs, rhs.VolumeSeries)
	tl.Flush()
}

func TestSetVolumeSystemTags(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	ops := &fakeOps{}
	a := NewAnimator(0, tl.Logger(), ops)
	fc := &fake.Client{}
	a.OCrud = fc

	vs := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID:      "vs",
				Version: 1,
			},
			RootParcelUUID: "uuid",
		},
		VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
			AccountID: "account1",
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				VolumeSeriesState: com.VolStateUnbound,
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				SizeBytes:  swag.Int64(10737418240),
				SystemTags: []string{"existingKey1:ev1", "existingKey2:ev2"},
			},
		},
	}
	vr := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "vr1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			RequestedOperations: []string{"BIND", "MOUNT"},
			CompleteByTime:      strfmt.DateTime(time.Now().Add(time.Hour)),
			ClusterID:           "cluster1",
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "BINDING",
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				VolumeSeriesID: "vs1",
			},
		},
	}
	rhs := &RequestHandlerState{
		A:            a,
		Request:      vr,
		VolumeSeries: vs,
		HasCreate:    util.Contains(vr.RequestedOperations, com.VolReqOpCreate),
		HasBind:      util.Contains(vr.RequestedOperations, com.VolReqOpBind),
		HasMount:     util.Contains(vr.RequestedOperations, com.VolReqOpMount),
		Canceling:    vr.CancelRequested,
	}
	rhs.VSUpdater = rhs

	fc.RetVSUpdaterUpdateErr = fmt.Errorf("update-volume-error")
	nT := len(vs.SystemTags)
	rhs.SetVolumeSystemTags(nil, "somekey:somevalue")
	assert.Len(vs.SystemTags, nT)
	assert.Len(rhs.Request.RequestMessages, 1)
	assert.Regexp("update-volume-error", rhs.Request.RequestMessages[0].Message)
	assert.Equal(1, tl.CountPattern("update-volume-error"))

	fc.RetVSUpdaterUpdateErr = nil
	nT = len(vs.SystemTags)
	rhs.SetVolumeSystemTags(nil, "somekey:somevalue", "anotherkey")
	assert.Len(rhs.VolumeSeries.SystemTags, nT+2)
	sTag := util.NewTagList(vs.SystemTags)
	val, ok := sTag.Get("somekey")
	assert.True(ok)
	assert.Equal("somevalue", val)
	val, ok = sTag.Get("anotherkey")
	assert.True(ok)
	assert.Equal("", val)

	fc.RetVSUpdaterObj = nil
	fc.RetVSUpdaterErr = nil
	nT = len(vs.SystemTags)
	sTag = util.NewTagList(vs.SystemTags)
	val, ok = sTag.Get("somekey")
	assert.True(ok)
	rhs.SetVolumeSystemTags(nil, "somekey:"+val+"2")
	assert.Equal([]string{"systemTags"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Append)
	assert.Nil(fc.InVSUpdaterItems.Remove)
	assert.Len(rhs.VolumeSeries.SystemTags, nT)
	sTag = util.NewTagList(vs.SystemTags)
	newVal, ok := sTag.Get("somekey")
	assert.True(ok)
	assert.NotEqual(val, newVal)
	assert.Equal("somevalue2", newVal)

	fc.RetVSUpdaterObj = nil
	fc.RetVSUpdaterErr = nil
	nT = len(vs.SystemTags)
	val, ok = sTag.Get("somekey")
	assert.True(ok)
	rhs.RemoveVolumeSystemTags(nil, "somekey:somevalue")
	assert.Equal([]string{"systemTags"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Append)
	assert.Nil(fc.InVSUpdaterItems.Remove)
	assert.Len(rhs.VolumeSeries.SystemTags, nT-1)
	sTag = util.NewTagList(vs.SystemTags)
	_, ok = sTag.Get("somekey")
	assert.False(ok)

	fc.RetVSUpdaterUpdateErr = nil
	fc.InVSUpdaterItems = nil
	vr.PlanOnly = swag.Bool(true)
	rhs.RetryLater = false
	rhs.InError = false
	rhs.SetVolumeSystemTags(nil, "somekey:somevalue")
	assert.Nil(fc.InVSUpdaterItems)

	fc.RetVSUpdaterUpdateErr = nil
	fc.InVSUpdaterItems = nil
	vr.PlanOnly = swag.Bool(true)
	rhs.RetryLater = false
	rhs.InError = false
	rhs.RemoveVolumeSystemTags(nil, "somekey:somevalue")
	assert.Nil(fc.InVSUpdaterItems)
}

func TestStash(t *testing.T) {
	assert := assert.New(t)

	type keyType struct{}
	type valueType struct {
		i int
	}
	value := valueType{4}

	rhs := &RequestHandlerState{}
	assert.Nil(rhs.Stash)
	v1 := rhs.StashGet(keyType{})
	assert.Nil(v1)
	assert.Nil(rhs.Stash)

	rhs.StashSet(keyType{}, &value)
	v1 = rhs.StashGet(keyType{})
	assert.NotNil(v1)
	v, ok := v1.(*valueType)
	assert.True(ok)
	assert.Equal(value, *v)
}

func TestCopyProgress(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	ops := &fakeOps{}
	a := NewAnimator(0, tl.Logger(), ops)
	fc := &fake.Client{}
	a.OCrud = fc

	vs := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID:      "vs1",
				Version: 1,
			},
		},
	}
	vr := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "vr1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			RequestedOperations: []string{"VOL_SNAPSHOT_CREATE"},
		},
	}
	rhs := &RequestHandlerState{
		A:            a,
		Request:      vr,
		VolumeSeries: vs,
	}

	assert.Nil(rhs.Request.Progress)

	fc.RetVSUpdaterErr = fmt.Errorf("fake-error")
	cpr := pstore.CopyProgressReport{
		TotalBytes:       1000,
		OffsetBytes:      500,
		TransferredBytes: 300,
		PercentComplete:  34.0,
	}
	tB := time.Now()
	rhs.CopyProgress(nil, cpr)
	tA := time.Now()
	sp := rhs.Request.Progress
	assert.NotNil(sp)
	assert.Equal(cpr.TotalBytes, sp.TotalBytes)
	assert.Equal(cpr.OffsetBytes, sp.OffsetBytes)
	assert.Equal(cpr.TransferredBytes, sp.TransferredBytes)
	assert.Equal(int32(cpr.PercentComplete), swag.Int32Value(sp.PercentComplete))
	assert.WithinDuration(tA, time.Time(sp.Timestamp), tA.Sub(tB))
	assert.EqualValues(rhs.Request.Meta.ID, fc.InVSRUpdaterID)
	assert.Equal([]string{"progress"}, fc.InVSRUpdaterItems.Set)
	assert.Nil(fc.InVSRUpdaterItems.Append)
	assert.Nil(fc.InVSRUpdaterItems.Remove)

	cpr.PercentComplete = -1
	rhs.CopyProgress(nil, cpr)
	sp = rhs.Request.Progress
	assert.Equal(int32(0), swag.Int32Value(sp.PercentComplete))

	cpr.PercentComplete = 100
	rhs.CopyProgress(nil, cpr)
	sp = rhs.Request.Progress
	assert.Equal(int32(99), swag.Int32Value(sp.PercentComplete))
}

func TestRequestWaiter(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	retVSR := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{ID: "vsr-1"},
		},
	}
	retSR := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{ID: "sr-1"},
		},
	}
	evM := fev.NewFakeEventManager()

	fI := &fakeRWInspectors{}

	// validate errors
	fc := &fake.Client{}
	wAList := []*RequestWaiterArgs{
		&RequestWaiterArgs{},
		&RequestWaiterArgs{VSR: retVSR},
		&RequestWaiterArgs{VSR: retVSR, CrudeOps: evM},
		&RequestWaiterArgs{VSR: retVSR, CrudeOps: evM, ClientOps: fc},
	}
	for _, wA := range wAList {
		assert.Panics(func() { NewRequestWaiter(wA) })
		assert.Error(wA.Validate(), "invalid arguments")
	}

	// validate success
	wA := &RequestWaiterArgs{VSR: retVSR, CrudeOps: evM, ClientOps: fc, Log: tl.Logger()}
	vsrW := NewRequestWaiter(wA)
	assert.NotNil(vsrW)
	assert.NotNil(vsrW.RW())
	assert.Equal(retVSR, vsrW.RW().vsr)
	assert.Equal(evM, vsrW.RW().crudeOps)
	assert.Equal(fc, vsrW.RW().clientOps)
	assert.Equal(tl.Logger(), vsrW.RW().log)
	assert.Equal(time.Duration(time.Second*5), vsrW.RW().waitInterval)

	// invalid invocations
	assert.Panics(func() { (&RWaiter{}).WaitForVSR(nil) })
	assert.Panics(func() { (&RWaiter{}).WaitForSR(nil) })

	// VSR completed
	retVSR.VolumeSeriesRequestState = com.VolReqStateSucceeded
	mW := &RWaiter{
		vsr:          retVSR,
		crudeOps:     evM,
		clientOps:    fc,
		log:          tl.Logger(),
		waitInterval: time.Duration(time.Millisecond * 10),
	}

	// invalid request with nil VSR
	mW.vsr = nil
	assert.Panics(func() { mW.WaitForVSR(nil) })

	// VSR completed
	mW.vsr = retVSR
	vsr, err := mW.WaitForVSR(nil)
	assert.Nil(err)
	assert.Equal(retVSR, vsr)

	// wait interval reached
	retVSR.VolumeSeriesRequestState = "NotTerminated"
	retVSR2 := &models.VolumeSeriesRequest{}
	testutils.Clone(retVSR, retVSR2)
	retVSR2.VolumeSeriesRequestState = com.VolReqStateSucceeded
	fc.RetVRObj = retVSR2
	vsr, err = mW.WaitForVSR(context.Background())
	assert.Nil(err)
	assert.Equal(retVSR2, vsr)
	assert.Equal(1, tl.CountPattern("RequestWaiter new interval"))
	tl.Flush()

	mW.waitInterval = time.Duration(time.Millisecond * 10)
	// wait interval reached but vsr lookup fails
	retVSR.VolumeSeriesRequestState = "NotTerminated"
	mW.vsr = retVSR
	fc.RetVRObj = nil
	fc.RetVRErr = fmt.Errorf("lookup error")
	vsr, err = mW.WaitForVSR(context.Background())
	assert.NotNil(err)
	assert.Regexp("lookup failed: lookup error", err.Error())
	assert.Equal(retVSR, vsr)
	assert.Equal(1, tl.CountPattern("RequestWaiter new interval"))
	assert.Equal(time.Duration(time.Second*30), mW.waitInterval)
	tl.Flush()

	// watcher notified
	vsr = nil
	err = nil
	retVSR.VolumeSeriesRequestState = "NotTerminated"
	mW.vsr = retVSR
	testutils.Clone(retVSR, retVSR2)
	retVSR2.VolumeSeriesRequestState = com.VolReqStateSucceeded
	fc.RetVRErr = nil
	fc.RetVRObj = retVSR2
	mW.clientOps = fc
	mW.waitInterval = time.Duration(time.Second * 5) // large enough to let the notify get called first
	ce := &crude.CrudEvent{Method: "PATCH", TrimmedURI: "/volume-series-requests/", Scope: crude.ScopeMap{}}
	c := mW.notifyCount
	mW.notifyChan = nil
	go func() {
		vsr, err = mW.WaitForVSR(context.Background())
	}()
	for mW.notifyChan == nil {
		time.Sleep(time.Millisecond * 1)
	}
	mW.CrudeNotify(crude.WatcherEvent, ce)
	for vsr == nil {
		time.Sleep(time.Millisecond * 1)
	}
	assert.Equal(c+1, mW.notifyCount)
	assert.Nil(err)
	assert.Equal(retVSR2, vsr)
	assert.Equal(1, tl.CountPattern("RequestWaiter notified"))
	tl.Flush()

	// context expired
	vsr = nil
	err = nil
	retVSR.VolumeSeriesRequestState = "NotTerminated"
	testutils.Clone(retVSR, retVSR2)
	retVSR2.VolumeSeriesRequestState = com.VolReqStateSucceeded
	mW.vsr = retVSR
	fc.RetVRErr = nil
	fc.RetVRObj = retVSR2
	mW.clientOps = fc
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Millisecond*50)
	defer cancel()
	vsr, err = mW.WaitForVSR(ctx)
	assert.NotNil(err)
	assert.Regexp("RequestWaiter context expired", err.Error())
	assert.Equal(retVSR, vsr)
	assert.Equal(1, tl.CountPattern(ErrRequestWaiterCtxExpired.Error()))
	tl.Flush()

	// inspector fails
	mW.vsrInspector = fI
	fI.RetCanWaitOnVSR = fmt.Errorf("vsr-inspector-breakout")
	mW.vsr = retVSR
	assert.False(mW.RequestStateIsTerminated())
	err = mW.waitForRequest(nil)
	assert.Error(err)
	assert.Equal(fI.RetCanWaitOnVSR, err)

	// test matcher for VSR
	wa := mW.getWatcherArgs()
	assert.NotNil(wa)
	assert.Len(wa.Matchers, 1)

	m := wa.Matchers[0]
	tl.Logger().Debugf("0.MethodPattern: %s", m.MethodPattern)
	re := regexp.MustCompile(m.MethodPattern)
	assert.True(re.MatchString("PATCH"))
	assert.NotEmpty(m.URIPattern)
	tl.Logger().Debugf("0.URIPattern: %s", m.URIPattern)
	re = regexp.MustCompile(m.URIPattern)
	assert.True(re.MatchString("/volume-series-requests/vsr-1?set=volumeSeriesRequestState&set=requestMessages&set=storagePlan&set=systemTags&version=9"))
	assert.True(re.MatchString("/volume-series-requests/vsr-1?set=requestMessages&set=volumeSeriesRequestState&set=storagePlan&set=systemTags&version=9"))
	assert.False(re.MatchString("/volume-series-requests"))
	assert.False(re.MatchString("/volume-series-requests/vsr-1?set=requestMessages"))
	assert.Empty(m.ScopePattern)

	retSR.StorageRequestState = com.StgReqStateSucceeded
	mW = &RWaiter{
		sr:           retSR,
		crudeOps:     evM,
		clientOps:    fc,
		log:          tl.Logger(),
		waitInterval: time.Duration(time.Millisecond * 10),
	}

	// invalid request with nil SR
	mW.sr = nil
	assert.Panics(func() { mW.WaitForSR(nil) })

	// SR completed
	mW.sr = retSR
	sr, err := mW.WaitForSR(nil)
	assert.Nil(err)
	assert.Equal(retSR, sr)

	// wait interval reached
	retSR.StorageRequestState = "NotTerminated"
	retSR2 := &models.StorageRequest{}
	testutils.Clone(retSR, retSR2)
	retSR2.StorageRequestState = com.StgReqStateSucceeded
	fc.RetSRFetchObj = retSR2
	sr, err = mW.WaitForSR(context.Background())
	assert.Nil(err)
	assert.Equal(retSR2, sr)
	assert.Equal(1, tl.CountPattern("RequestWaiter new interval"))
	tl.Flush()

	mW.waitInterval = time.Duration(time.Millisecond * 10)
	// wait interval reached but sr lookup fails
	retSR.StorageRequestState = "NotTerminated"
	mW.sr = retSR
	fc.RetSRFetchObj = nil
	fc.RetSRFetchErr = fmt.Errorf("lookup error")
	sr, err = mW.WaitForSR(context.Background())
	assert.NotNil(err)
	assert.Regexp("lookup failed: lookup error", err.Error())
	assert.Equal(retSR, sr)
	assert.Equal(1, tl.CountPattern("RequestWaiter new interval"))
	assert.Equal(time.Duration(time.Second*30), mW.waitInterval)
	tl.Flush()

	// watcher notified
	sr = nil
	err = nil
	retSR.StorageRequestState = "NotTerminated"
	mW.sr = retSR
	testutils.Clone(retSR, retSR2)
	retSR2.StorageRequestState = com.StgReqStateSucceeded
	fc.RetSRFetchErr = nil
	fc.RetSRFetchObj = retSR2
	mW.clientOps = fc
	mW.waitInterval = time.Duration(time.Second * 5) // large enough to let the notify get called first
	ce = &crude.CrudEvent{Method: "PATCH", TrimmedURI: "/storage-requests/", Scope: crude.ScopeMap{}}
	c = mW.notifyCount
	mW.notifyChan = nil
	go func() {
		sr, err = mW.WaitForSR(context.Background())
	}()
	for mW.notifyChan == nil {
		time.Sleep(time.Millisecond * 1)
	}
	mW.CrudeNotify(crude.WatcherEvent, ce)
	for sr == nil {
		time.Sleep(time.Millisecond * 1)
	}
	assert.Equal(c+1, mW.notifyCount)
	assert.Nil(err)
	assert.Equal(retSR2, sr)
	assert.Equal(1, tl.CountPattern("RequestWaiter notified"))
	tl.Flush()

	// context expired
	sr = nil
	err = nil
	retSR.StorageRequestState = "NotTerminated"
	testutils.Clone(retSR, retSR2)
	retSR2.StorageRequestState = com.StgReqStateSucceeded
	mW.sr = retSR
	fc.RetSRFetchErr = nil
	fc.RetSRFetchObj = retSR2
	mW.clientOps = fc
	ctx = context.Background()
	ctx, cancel = context.WithTimeout(ctx, time.Millisecond*50)
	defer cancel()
	sr, err = mW.WaitForSR(ctx)
	assert.NotNil(err)
	assert.Regexp("RequestWaiter context expired", err.Error())
	assert.Equal(retSR, sr)
	assert.Equal(1, tl.CountPattern(ErrRequestWaiterCtxExpired.Error()))
	tl.Flush()

	// inspector fails
	mW.srInspector = fI
	fI.RetCanWaitOnSR = fmt.Errorf("sr-inspector-breakout")
	mW.sr = retSR
	assert.False(mW.RequestStateIsTerminated())
	err = mW.waitForRequest(nil)
	assert.Error(err)
	assert.Equal(fI.RetCanWaitOnSR, err)

	// test matcher for SR
	mW.vsr = nil
	mW.sr = retSR
	wa = mW.getWatcherArgs()
	assert.NotNil(wa)
	assert.Len(wa.Matchers, 1)

	m = wa.Matchers[0]
	tl.Logger().Debugf("0.MethodPattern: %s", m.MethodPattern)
	re = regexp.MustCompile(m.MethodPattern)
	assert.True(re.MatchString("PATCH"))
	assert.NotEmpty(m.URIPattern)
	tl.Logger().Debugf("0.URIPattern: %s", m.URIPattern)
	re = regexp.MustCompile(m.URIPattern)
	assert.True(re.MatchString("/storage-requests/sr-1?set=storageRequestState&set=requestMessages&set=storageId&version=21"))
	assert.True(re.MatchString("/storage-requests/sr-1?set=requestMessages&set=storageRequestState&set=storageId&version=21"))
	assert.False(re.MatchString("/storage-requests"))
	assert.False(re.MatchString("/storage-requests/sr-1?set=requestMessages"))
	assert.Empty(m.ScopePattern)
}

type fakeRWInspectors struct {
	RetCanWaitOnVSR error
	RetCanWaitOnSR  error
}

func (i *fakeRWInspectors) CanWaitOnVSR(*models.VolumeSeriesRequest) error {
	return i.RetCanWaitOnVSR
}

func (i *fakeRWInspectors) CanWaitOnSR(*models.StorageRequest) error {
	return i.RetCanWaitOnSR
}

func TestHasten(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	mW := &RWaiter{}

	// using context that will expire before, new context should have same deadline
	inCtx, inCan := context.WithDeadline(context.Background(), time.Now())
	defer inCan()
	retCtx, _ := mW.HastenContextDeadline(inCtx, time.Second)
	inDL, _ := inCtx.Deadline()
	retDL, ok := retCtx.Deadline()
	assert.True(ok)
	assert.Equal(inDL, retDL)

	// using context that can be reduced
	inCtx, inCan = context.WithTimeout(context.Background(), 5*time.Minute)
	inDL, _ = inCtx.Deadline()
	defer inCan()
	retCtx, _ = mW.HastenContextDeadline(inCtx, time.Second)
	retDL, ok = retCtx.Deadline()
	assert.True(ok)
	assert.NotEqual(inDL, retDL)
}
