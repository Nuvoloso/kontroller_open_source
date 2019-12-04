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
	"net/http"
	"regexp"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	mockCSP "github.com/Nuvoloso/kontroller/pkg/csp/mock"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

type fakeRequestHandlers struct {
	Count         int
	BlockCount    int
	Transitions   []string
	RetryOnState  string
	FailOnState   string
	WaitForSignal bool
	FmtIds        []string
	UseIds        []string
	CloseIds      []string
	signalled     bool
	mux           sync.Mutex
	cond          *sync.Cond
}

var _ = requestHandlers(&fakeRequestHandlers{})

func newFakeRequestHandlers() *fakeRequestHandlers {
	fa := &fakeRequestHandlers{}
	fa.Transitions = make([]string, 0)
	fa.FmtIds = make([]string, 0)
	fa.UseIds = make([]string, 0)
	fa.CloseIds = make([]string, 0)
	fa.cond = sync.NewCond(&fa.mux)
	return fa
}

func (fa *fakeRequestHandlers) common(ctx context.Context, rhs *requestHandlerState, thisState string) {
	fa.mux.Lock()
	if fa.WaitForSignal {
		for !fa.signalled {
			fa.BlockCount++
			fa.cond.Wait()
		}
	}
	fa.Count++
	fa.Transitions = append(fa.Transitions, thisState)
	switch thisState {
	case "FORMATTING":
		fa.FmtIds = append(fa.FmtIds, string(rhs.Request.Meta.ID))
	case "USING":
		fa.UseIds = append(fa.UseIds, string(rhs.Request.Meta.ID))
	case "CLOSING":
		fa.CloseIds = append(fa.CloseIds, string(rhs.Request.Meta.ID))
	case "DETACHING":
		rhs.RetryLater = true
	}
	if fa.RetryOnState == thisState {
		rhs.RetryLater = true
	}
	if fa.FailOnState == thisState {
		rhs.InError = true
	}
	fa.mux.Unlock()
}

func (fa *fakeRequestHandlers) Sort() {
	sort.Strings(fa.FmtIds)
	sort.Strings(fa.UseIds)
}

func (fa *fakeRequestHandlers) Signal() {
	fa.mux.Lock()
	fa.signalled = true
	fa.cond.Broadcast()
	fa.mux.Unlock()
}

func (fa *fakeRequestHandlers) FormatStorage(ctx context.Context, rhs *requestHandlerState) {
	fa.common(ctx, rhs, "FORMATTING")
}

func (fa *fakeRequestHandlers) UseStorage(ctx context.Context, rhs *requestHandlerState) {
	fa.common(ctx, rhs, "USING")
}

func (fa *fakeRequestHandlers) CloseStorage(ctx context.Context, rhs *requestHandlerState) {
	fa.common(ctx, rhs, "CLOSING")
}

func TestRequestHandlerStateHelpers(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM

	c := &SRComp{}
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc

	now := time.Now()
	o := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "STORAGE-REQ-1",
			},
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
				StorageRequestState: "NEW",
			},
		},
	}

	rhs := &requestHandlerState{
		c:       c,
		Request: o,
	}
	assert.Len(o.RequestMessages, 0)

	// test setting a message
	m := rhs.setRequestMessage("msg %d", 1)
	assert.Equal("msg 1", m)
	assert.Len(o.RequestMessages, 1)
	assert.Equal(m, o.RequestMessages[0].Message)
	assert.True(now.Before(time.Time(o.RequestMessages[0].Time)))

	// test setting the state
	assert.Equal("NEW", o.StorageRequestState)
	chg := rhs.setRequestState("NEW")
	assert.False(chg)
	chg = rhs.setRequestState("FORMATTING")
	assert.True(chg)
	assert.Equal("FORMATTING", o.StorageRequestState)
	assert.Len(o.RequestMessages, 2)
	assert.Regexp("NEW .* FORMATTING", o.RequestMessages[1].Message)
	cntN2P := 0
	tl.Iterate(func(i uint64, s string) {
		if m, err := regexp.MatchString(string(o.Meta.ID)+".* NEW .* FORMATTING", s); err == nil && m {
			cntN2P++
		}
	})
	assert.Equal(1, cntN2P)

	// test updating state (no change)
	assert.Len(o.RequestMessages, 2)
	assert.Equal("FORMATTING", o.StorageRequestState)
	err := rhs.setAndUpdateRequestState(nil, "FORMATTING")
	assert.Nil(err)
	assert.Len(o.RequestMessages, 2)
	assert.Equal("FORMATTING", o.StorageRequestState)

	// test setting error
	assert.False(rhs.InError)
	msg := rhs.setRequestError("request error")
	assert.Len(o.RequestMessages, 3)
	assert.True(rhs.InError)
	assert.Regexp("request error", msg)
	assert.Regexp("request error", o.RequestMessages[2].Message)
	cntRequestErr := 0
	tl.Iterate(func(i uint64, s string) {
		if m, err := regexp.MatchString("request error", s); err == nil && m {
			cntRequestErr++
		}
	})
	assert.Equal(1, cntRequestErr)

	// success case, rhs object updated in-place then replaced by return value
	oM := make([]*models.TimestampedString, len(o.RequestMessages))
	copy(oM, o.RequestMessages)
	oState := o.StorageRequestState
	fc.RetSRUpdatedObj = &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:      o.Meta.ID,
				Version: o.Meta.Version + 1,
			},
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
				StorageRequestState: o.StorageRequestState,
				RequestMessages:     o.RequestMessages,
			},
		},
	}
	assert.Equal(o, rhs.Request)
	assert.Equal("FORMATTING", o.StorageRequestState)
	err = rhs.setAndUpdateRequestState(nil, "USING")
	assert.Nil(err)
	assert.Equal(o, fc.ModSRUpdaterObj)
	assert.EqualValues(0, fc.InSRUpdaterItems.Version)
	assert.Equal([]string{"storageRequestState", "requestMessages"}, fc.InSRUpdaterItems.Set)
	assert.Nil(fc.InSRUpdaterItems.Append)
	assert.Nil(fc.InSRUpdaterItems.Remove)
	assert.Equal(fc.RetSRUpdatedObj, rhs.Request)
	assert.NotEqual(o, rhs.Request)
	assert.Equal("USING", o.StorageRequestState)
	assert.True(len(rhs.Request.RequestMessages) < len(o.RequestMessages))
	assert.True(rhs.Request.Meta.Version > o.Meta.Version)
	o.RequestMessages = oM         // restore previous
	o.StorageRequestState = oState // restore previous

	// failure case when updating state - object not updated
	rhs.Request = o
	assert.NotEqual("USING", o.StorageRequestState)
	fc.RetSRUpdatedObj = nil
	fc.RetSRUpdaterUpdateErr = fmt.Errorf("update error")
	err = rhs.setAndUpdateRequestState(nil, "USING")
	assert.NotNil(err)
	assert.Regexp("update error", err.Error())
	assert.Equal("USING", o.StorageRequestState)

	// test setting message and updating state
	// failure case - message set but object not updated
	o = &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "SR1",
			},
		},
	}
	newO := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:      "SR1",
				Version: 2,
			},
		},
	}
	rhs.Request = o
	fc.InSRUpdaterItems = nil
	fc.RetSRUpdatedObj = nil
	fc.RetSRUpdaterUpdateErr = fmt.Errorf("update error")
	msg, err = rhs.setAndUpdateRequestMessage(nil, "msg %s", "1")
	assert.Error(err)
	assert.Equal("msg 1", msg)
	assert.Equal(o, rhs.Request)
	assert.Equal(o.RequestMessages[0].Message, msg)
	assert.Equal([]string{"storageRequestState", "requestMessages"}, fc.InSRUpdaterItems.Set)
	assert.Nil(fc.InSRUpdaterItems.Append)
	assert.Nil(fc.InSRUpdaterItems.Remove)
	// success case - message and object updated
	rhs.Request = o
	fc.InSRUpdaterItems = nil
	fc.RetSRUpdatedObj = newO
	fc.RetSRUpdaterUpdateErr = nil
	msg, err = rhs.setAndUpdateRequestMessageDistinct(nil, "msg %s", "2")
	assert.Nil(err)
	assert.Equal("msg 2", msg)
	assert.Equal(newO, rhs.Request)
	assert.Len(o.RequestMessages, 2)
	assert.Equal(o.RequestMessages[1].Message, msg)
	rhs.Request.RequestMessages = o.RequestMessages
	msg, err = rhs.setAndUpdateRequestMessageDistinct(nil, "msg %s", "2")
	assert.Nil(err)
	assert.Equal("msg 2", msg)
	assert.Equal(newO, rhs.Request)
	assert.Len(o.RequestMessages, 2)

	// attempt predicates
	for st := range stateOrder {
		rhs.Request.StorageRequestState = st
		switch st {
		case com.StgReqStateNew:
			assert.True(rhs.attemptFormat())
			assert.True(rhs.attemptUse())
		case com.StgReqStateFormatting:
			assert.True(rhs.attemptFormat())
			assert.True(rhs.attemptUse())
		case com.StgReqStateUsing:
			assert.False(rhs.attemptFormat())
			assert.True(rhs.attemptUse())
		case com.StgReqStateClosing:
			assert.True(rhs.attemptClose())
		case com.StgReqStateSucceeded:
			assert.False(rhs.attemptFormat())
			assert.False(rhs.attemptUse())
		case com.StgReqStateFailed:
			assert.False(rhs.attemptFormat())
			assert.False(rhs.attemptUse())
		}
	}
}

func TestDispatchRequest(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	cbTime := strfmt.DateTime(time.Now().Add(time.Hour * 1))

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	domClient := mockCSP.NewMockDomainClient(mockCtrl)
	app.CSPClient = domClient

	// Test various skip cases
	skipTCs := []struct {
		ID        string
		ops       []string
		state     string
		pat       string
		storageID string
	}{
		{
			ID:    "not-supported-op",
			ops:   []string{"DETACH"},
			state: "anything",
			pat:   "Skipping.*not-supported-op",
		},
		{
			ID:    "not-ready-1",
			ops:   []string{"PROVISION", "FORMAT"},
			state: "ATTACHING",
			pat:   "Skipping.*not-ready-1",
		},
		{
			ID:    "not-ready-2",
			ops:   []string{"ATTACH", "USE"},
			state: "NEW",
			pat:   "Skipping.*not-ready-2",
		},
		{
			ID:        "db-error",
			ops:       []string{"PROVISION", "ATTACH", "FORMAT", "USE"},
			state:     "FORMATTING",
			pat:       "Skipping.*db-error because of a transient error",
			storageID: "STORAGE-1",
		},
		{
			ID:        "centrald-op",
			ops:       []string{"CLOSE", "DETACH", "RELEASE"},
			state:     "CLOSED",
			pat:       "Skipping.*centrald-op processed by centrald",
			storageID: "STORAGE-1",
		},
		{
			ID:  "in-progress",
			ops: []string{},
			pat: "in-progress already being processed",
		},
		{
			ID:  "old-request",
			ops: []string{},
			pat: "old-request processing is already done",
		},
	}
	tl.Logger().Info("** DISPATCH skip cases")
	c := &SRComp{}
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc
	assert.Empty(c.activeRequests)
	assert.Empty(c.doneRequests)
	c.activeRequests["in-progress"] = &requestHandlerState{}
	c.doneRequests["db-error"] = 3
	c.doneRequests["old-request"] = 4
	fc.StorageFetchRetErr = fake.TransientErr
	fc.StorageFetchRetObj = nil
	for _, tc := range skipTCs {
		tl.Flush()
		sr := &models.StorageRequest{}
		sr.Meta = &models.ObjMeta{
			ID:      models.ObjID(tc.ID),
			Version: 3,
		}
		sr.RequestedOperations = tc.ops
		sr.StorageRequestState = tc.state
		sr.StorageID = models.ObjIDMutable(tc.storageID)
		sr.CompleteByTime = cbTime
		cnt := c.dispatchRequests(nil, []*models.StorageRequest{sr})
		assert.Equal(0, cnt)
		assert.Equal(1, tl.CountPattern(tc.pat))
		if tc.storageID != "" {
			assert.Equal(tc.storageID, fc.StorageFetchID)
		}
	}

	// dispatch simple storage failure cases
	dispatchStorageSimpleErrTCs := []struct {
		ID        string
		ops       []string
		state     string
		pat       string
		storageID string
	}{
		{
			ID:    "missing-storage-id",
			ops:   []string{"FORMAT"},
			state: "NEW",
			pat:   "storageId not set in.*missing-storage-id",
		},
		{
			ID:        "storage-id-not-found",
			ops:       []string{"FORMAT"},
			state:     "NEW",
			storageID: "fake-storage-id",
			pat:       "Failing StorageRequest storage-id-not-found.*storage fetch error",
		},
	}
	tl.Logger().Info("** DISPATCH storage simple error cases")
	tl.Flush()
	c = &SRComp{}
	c.Init(app)
	fc = &fake.Client{}
	c.oCrud = fc
	fc.StorageFetchRetErr = fmt.Errorf("storage fetch error")
	fa := newFakeRequestHandlers()
	c.requestHandlers = fa
	fa.WaitForSignal = true // non-error cases would block
	srs := make([]*models.StorageRequest, 0, len(dispatchStorageSimpleErrTCs))
	for _, tc := range dispatchStorageSimpleErrTCs {
		sr := &models.StorageRequest{}
		sr.Meta = &models.ObjMeta{
			ID: models.ObjID(tc.ID),
		}
		sr.RequestedOperations = tc.ops
		sr.StorageRequestState = tc.state
		sr.StorageID = models.ObjIDMutable(tc.storageID)
		sr.CompleteByTime = cbTime
		srs = append(srs, sr)
	}
	cnt := c.dispatchRequests(nil, srs)
	assert.Equal(len(dispatchStorageSimpleErrTCs), cnt)
	for _, tc := range dispatchStorageSimpleErrTCs {
		tl.Logger().Debugf("** checking pattern /%s/", tc.pat)
		assert.True(tl.CountPattern(tc.pat) >= 1)
	}
	assert.Equal(0, tl.CountPattern("Timed out"))
	// wait for these requests to be done - all are in error
	for len(c.activeRequests) != 0 {
		time.Sleep(1 * time.Millisecond)
	}

	// dispatch storage state invalid failure cases
	dispatchStorageStateErrTCs := []struct {
		ID string
		ss *models.StorageStateMutable
	}{
		{
			ID: "storage-state-not-set",
		},
		{
			ID: "storage-state-not-attached",
			ss: &models.StorageStateMutable{
				AttachmentState: "NotValid",
			},
		},
		{
			ID: "storage-state-wrong-node",
			ss: &models.StorageStateMutable{
				AttachmentState: "ATTACHED",
				AttachedNodeID:  "NotTHIS-NODE",
			},
		},
	}
	tl.Logger().Info("** DISPATCH storage state error cases")
	c = &SRComp{}
	c.Init(app)
	c.thisNodeID = "THIS-NODE"
	fc = &fake.Client{}
	c.oCrud = fc
	fa = newFakeRequestHandlers()
	c.requestHandlers = fa
	fa.WaitForSignal = true // non-error cases would block
	for _, tc := range dispatchStorageStateErrTCs {
		tl.Flush()
		sr := &models.StorageRequest{}
		sr.Meta = &models.ObjMeta{
			ID: models.ObjID(tc.ID),
		}
		sr.RequestedOperations = []string{"FORMAT"}
		sr.StorageRequestState = "NEW"
		sr.StorageID = "STORAGE-1"
		sr.CompleteByTime = cbTime
		srs = append(srs, sr)
		sObj := &models.Storage{StorageAllOf0: models.StorageAllOf0{Meta: &models.ObjMeta{ID: "STORAGE-1"}}}
		sObj.StorageState = tc.ss
		fc.StorageFetchRetObj = sObj
		cnt = c.dispatchRequests(nil, []*models.StorageRequest{sr})
		assert.Equal(1, cnt)
		pat := fmt.Sprintf("%s storageState invalid", tc.ID)
		tl.Logger().Debugf("** checking pattern /%s/", pat)
		assert.True(tl.CountPattern(pat) >= 1)
		assert.Equal(0, tl.CountPattern("Timed out"))
		// wait for the requests to be done - all are in error
		for len(c.activeRequests) != 0 {
			time.Sleep(1 * time.Millisecond)
		}
	}

	// dispatch success cases
	dispatchTCs := []struct {
		ID    string
		ops   []string
		state string
	}{
		{
			ID:    "new-format",
			ops:   []string{"FORMAT"},
			state: "NEW",
		},
		{
			ID:    "new-use",
			ops:   []string{"USE"},
			state: "NEW",
		},
		{
			ID:    "new-close",
			ops:   []string{"CLOSE"},
			state: "NEW",
		},
		{
			ID:    "format-use",
			ops:   []string{"FORMAT", "USE"},
			state: "NEW",
		},
		{
			ID:    "provision-attach-format-use",
			ops:   []string{"PROVISION", "ATTACH", "FORMAT", "USE"},
			state: "FORMATTING",
		},
		{
			ID:    "format-restart",
			ops:   []string{"FORMAT", "USE"},
			state: "FORMATTING",
		},
		{
			ID:    "use-restart",
			ops:   []string{"FORMAT", "USE"},
			state: "USING",
		},
		{
			ID:    "timed-out",
			ops:   []string{"USE"},
			state: "NEW",
		},
	}
	expFormatIds := []string{ // sorted
		"format-restart",
		"format-use",
		"new-format",
		"provision-attach-format-use",
	}
	expUseIds := []string{ // sorted
		"format-restart",
		"format-use",
		"new-use",
		"provision-attach-format-use",
		"use-restart",
	}
	expCloseIds := []string{
		"new-close",
	}
	tl.Logger().Info("** DISPATCH success cases")
	tl.Flush()
	c = &SRComp{}
	c.Init(app)
	c.thisNodeID = "NODE-1"
	fc = &fake.Client{}
	c.oCrud = fc
	fc.StorageFetchRetObj = &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta: &models.ObjMeta{
				ID:      "STORAGE-1",
				Version: 1,
			},
		},
		StorageMutable: models.StorageMutable{
			StorageIdentifier: "csp:identifier",
			StorageState: &models.StorageStateMutable{
				AttachedNodeDevice: "/dev/xvb0",
				AttachedNodeID:     "NODE-1",
				AttachmentState:    "ATTACHED",
				DeviceState:        "UNUSED",
			},
		},
	}
	fc.PassThroughUSRObj = true // return updated SR
	fa = newFakeRequestHandlers()
	c.requestHandlers = fa
	fa.WaitForSignal = true
	srs = make([]*models.StorageRequest, 0, len(dispatchTCs))
	for _, tc := range dispatchTCs {
		sr := &models.StorageRequest{}
		sr.Meta = &models.ObjMeta{
			ID: models.ObjID(tc.ID),
		}
		sr.RequestedOperations = tc.ops
		sr.StorageRequestState = tc.state
		sr.StorageID = models.ObjIDMutable("STORAGE-1")
		if tc.ID != "timed-out" {
			sr.CompleteByTime = cbTime
		}
		srs = append(srs, sr)
	}
	cnt = c.dispatchRequests(nil, srs)
	assert.Equal(len(dispatchTCs), cnt)
	// wait until all are blocked or failed (done)
	for fa.BlockCount != cnt-1 && len(c.doneRequests) == 0 {
		time.Sleep(1 * time.Millisecond)
	}
	assert.Len(c.doneRequests, 1)
	assert.True(func() bool { _, ok := c.doneRequests["timed-out"]; return ok }())
	fa.WaitForSignal = false
	fa.Signal()
	// wait until all are done
	for len(c.activeRequests) != 0 {
		time.Sleep(1 * time.Millisecond)
	}
	assert.Len(c.doneRequests, cnt)
	fa.Sort()
	assert.Equal(expFormatIds, fa.FmtIds)
	assert.Equal(expUseIds, fa.UseIds)
	assert.Equal(expCloseIds, fa.CloseIds)
	for _, tc := range dispatchTCs {
		pat := fmt.Sprintf("Processing StorageRequest %s", tc.ID)
		tl.Logger().Debugf("** checking pattern /%s/", pat) // duh, pattern here too!
		assert.Equal(2, tl.CountPattern(pat))
	}
	assert.Equal(1, tl.CountPattern("Timed out"))
}

func TestRequestAnimator(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &agentd.AppCtx{
		AppArgs: agentd.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM

	pastCompleteBy := strfmt.DateTime(time.Now().Add(time.Hour * -1))
	futureCompleteBy := strfmt.DateTime(time.Now().Add(time.Hour * 1))

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	domClient := mockCSP.NewMockDomainClient(mockCtrl)
	app.CSPClient = domClient

	tl.Logger().Info("case: RHS InError on entry for FORMAT")
	c := &SRComp{}
	c.Init(app)
	c.thisNodeID = "NODE-1"
	fc := &fake.Client{}
	c.oCrud = fc
	fa := newFakeRequestHandlers()
	c.requestHandlers = fa
	done := false
	rhs := &requestHandlerState{
		c:         c,
		Request:   &models.StorageRequest{},
		HasFormat: true,
		InError:   true,
		Storage: &models.Storage{
			StorageMutable: models.StorageMutable{
				StorageState: &models.StorageStateMutable{
					DeviceState: "FORMATTING",
					Messages:    []*models.TimestampedString{},
				},
			},
		},
		Done: func() { done = true },
	}
	rhs.Request.Meta = &models.ObjMeta{ID: "SR-1"}
	rhs.Request.CompleteByTime = futureCompleteBy
	rhs.Request.RequestedOperations = []string{"FORMAT"}
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal("FAILED", rhs.Request.StorageRequestState)
	assert.Regexp("⇒ FAILED", rhs.Request.RequestMessages[0].Message)
	assert.Equal([]string{}, fa.Transitions)
	assert.NotNil(fc.InUSitems)
	assert.Equal([]string{"storageState"}, fc.InUSitems.Set)
	assert.Empty(fc.InUSitems.Append)
	assert.Empty(fc.InUSitems.Remove)
	assert.Equal("UNUSED", rhs.Storage.StorageState.DeviceState)

	tl.Logger().Info("case: RHS InError on entry for CLOSE")
	done = false
	rhs = &requestHandlerState{
		c:        c,
		Request:  &models.StorageRequest{},
		HasClose: true,
		InError:  true,
		Storage: &models.Storage{
			StorageMutable: models.StorageMutable{
				StorageState: &models.StorageStateMutable{
					DeviceState: "CLOSING",
					Messages:    []*models.TimestampedString{},
				},
			},
		},
		Done: func() { done = true },
	}
	rhs.Request.Meta = &models.ObjMeta{ID: "SR-1"}
	rhs.Request.CompleteByTime = futureCompleteBy
	rhs.Request.RequestedOperations = []string{"CLOSE"}
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal("FAILED", rhs.Request.StorageRequestState)
	assert.Regexp("⇒ FAILED", rhs.Request.RequestMessages[0].Message)
	assert.Equal([]string{}, fa.Transitions)
	assert.NotNil(fc.InUSitems)
	assert.Equal([]string{"storageState"}, fc.InUSitems.Set)
	assert.Empty(fc.InUSitems.Append)
	assert.Empty(fc.InUSitems.Remove)
	assert.Equal("OPEN", rhs.Storage.StorageState.DeviceState)

	//
	tl.Logger().Info("case: RHS time out on entry")
	rhs = &requestHandlerState{
		c:         c,
		Request:   &models.StorageRequest{},
		HasFormat: true,
		Storage: &models.Storage{
			StorageMutable: models.StorageMutable{
				StorageState: &models.StorageStateMutable{
					DeviceState: "FORMATTING",
					Messages:    []*models.TimestampedString{},
				},
			},
		},
		Done: func() { done = true },
	}
	rhs.Request.Meta = &models.ObjMeta{ID: "SR-1"}
	rhs.Request.CompleteByTime = pastCompleteBy
	rhs.Request.RequestedOperations = []string{"FORMAT"}
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	done = false
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.True(rhs.TimedOut)
	assert.Equal("FAILED", rhs.Request.StorageRequestState)
	assert.Regexp("Timed out", rhs.Request.RequestMessages[0].Message)
	assert.Regexp("⇒ FAILED", rhs.Request.RequestMessages[1].Message)
	assert.Equal([]string{}, fa.Transitions)
	assert.NotNil(fc.InUSitems)
	assert.Equal([]string{"storageState"}, fc.InUSitems.Set)
	assert.Empty(fc.InUSitems.Append)
	assert.Empty(fc.InUSitems.Remove)
	assert.Equal("UNUSED", rhs.Storage.StorageState.DeviceState)

	//
	tl.Logger().Info("case: RHS FORMAT+USE via NEW")
	rhs = &requestHandlerState{
		c:         c,
		Request:   &models.StorageRequest{},
		HasFormat: true,
		HasUse:    true,
		Done:      func() { done = true },
	}
	rhs.Request.Meta = &models.ObjMeta{ID: "SR-1"}
	rhs.Request.CompleteByTime = futureCompleteBy
	rhs.Request.RequestedOperations = []string{"FORMAT", "USE"}
	rhs.Request.StorageRequestState = "NEW"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	done = false
	fa.Transitions = []string{}
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal("SUCCEEDED", rhs.Request.StorageRequestState)
	assert.Regexp("NEW ⇒ FORMATTING", rhs.Request.RequestMessages[0].Message)
	assert.Regexp("FORMATTING ⇒ USING", rhs.Request.RequestMessages[1].Message)
	assert.Regexp("USING ⇒ SUCCEEDED", rhs.Request.RequestMessages[2].Message)
	assert.Equal([]string{"FORMATTING", "USING"}, fa.Transitions)

	//
	tl.Logger().Info("case: RHS PROVISION+ATTACH+FORMAT+USE via FORMATTING")
	rhs = &requestHandlerState{
		c:       c,
		Request: &models.StorageRequest{},
		Storage: &models.Storage{
			StorageMutable: models.StorageMutable{
				StorageIdentifier: "csp:identifier",
				StorageState: &models.StorageStateMutable{
					DeviceState: "UNUSED",
					Messages:    []*models.TimestampedString{},
				},
			},
		},
		HasFormat:    true,
		HasUse:       true,
		HasProvision: true,
		HasAttach:    true,
		Done:         func() { done = true },
	}
	rhs.Request.Meta = &models.ObjMeta{ID: "SR-1"}
	rhs.Request.CompleteByTime = futureCompleteBy
	rhs.Request.RequestedOperations = []string{"PROVISION", "ATTACH", "FORMAT", "USE"}
	rhs.Request.StorageRequestState = "FORMATTING"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	done = false
	fa.Transitions = []string{}
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.True(rhs.RetryLater)
	assert.Equal("REMOVING_TAG", rhs.Request.StorageRequestState)
	assert.Regexp("FORMATTING ⇒ USING", rhs.Request.RequestMessages[0].Message)
	assert.Regexp("USING ⇒ REMOVING_TAG", rhs.Request.RequestMessages[1].Message)
	assert.Equal([]string{"FORMATTING", "USING"}, fa.Transitions)
	tl.Flush()

	//
	tl.Logger().Info("case: RHS FORMAT+USE via USING")
	rhs = &requestHandlerState{
		c:         c,
		Request:   &models.StorageRequest{},
		HasFormat: true,
		HasUse:    true,
		Done:      func() { done = true },
	}
	rhs.Request.Meta = &models.ObjMeta{ID: "SR-1"}
	rhs.Request.CompleteByTime = futureCompleteBy
	rhs.Request.RequestedOperations = []string{"FORMAT", "USE"}
	rhs.Request.StorageRequestState = "USING"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	done = false
	fa.Transitions = []string{}
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal("SUCCEEDED", rhs.Request.StorageRequestState)
	assert.Regexp("USING ⇒ SUCCEEDED", rhs.Request.RequestMessages[0].Message)
	assert.Equal([]string{"USING"}, fa.Transitions)

	//
	tl.Logger().Info("case: RHS FORMAT+USE fail in USING")
	rhs = &requestHandlerState{
		c:         c,
		Request:   &models.StorageRequest{},
		HasFormat: true,
		HasUse:    true,
		Done:      func() { done = true },
	}
	rhs.Request.Meta = &models.ObjMeta{ID: "SR-1"}
	rhs.Request.CompleteByTime = futureCompleteBy
	rhs.Request.RequestedOperations = []string{"FORMAT", "USE"}
	rhs.Request.StorageRequestState = "FORMATTING"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	done = false
	fa.FailOnState = "USING"
	fa.Transitions = []string{}
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.True(rhs.InError)
	assert.Equal("FAILED", rhs.Request.StorageRequestState)
	assert.Regexp("FORMATTING ⇒ USING", rhs.Request.RequestMessages[0].Message)
	assert.Regexp("USING ⇒ FAILED", rhs.Request.RequestMessages[1].Message)
	assert.Equal([]string{"FORMATTING", "USING"}, fa.Transitions)

	//
	tl.Logger().Info("case: RHS FORMAT+USE retry in FORMATTING")
	rhs = &requestHandlerState{
		c:         c,
		Request:   &models.StorageRequest{},
		HasFormat: true,
		HasUse:    true,
		Done:      func() { done = true },
	}
	rhs.Request.Meta = &models.ObjMeta{ID: "SR-1"}
	rhs.Request.CompleteByTime = futureCompleteBy
	rhs.Request.RequestedOperations = []string{"FORMAT", "USE"}
	rhs.Request.StorageRequestState = "FORMATTING"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	done = false
	fa.FailOnState = ""
	fa.RetryOnState = "FORMATTING"
	fa.Transitions = []string{}
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.True(rhs.RetryLater)
	assert.Equal("FORMATTING", rhs.Request.StorageRequestState)
	assert.Empty(rhs.Request.RequestMessages)
	assert.Equal([]string{"FORMATTING"}, fa.Transitions)

	tl.Logger().Info("case: RHS FORMAT+USE transient failure to update state before USING")
	rhs = &requestHandlerState{
		c:         c,
		Request:   &models.StorageRequest{},
		HasFormat: true,
		HasUse:    true,
		Done:      func() { done = true },
	}
	rhs.Request.Meta = &models.ObjMeta{ID: "SR-1"}
	rhs.Request.CompleteByTime = futureCompleteBy
	rhs.Request.RequestedOperations = []string{"FORMAT", "USE"}
	rhs.Request.StorageRequestState = "FORMATTING"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	done = false
	fa.FailOnState = ""
	fa.RetryOnState = ""
	fa.Transitions = []string{}
	fc.RetSRUpdaterUpdateErr = fmt.Errorf("storage request update error")
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.True(rhs.RetryLater)
	assert.Equal("USING", rhs.Request.StorageRequestState)
	if assert.Len(rhs.Request.RequestMessages, 1) {
		assert.Regexp("FORMATTING ⇒ USING", rhs.Request.RequestMessages[0].Message)
	}
	assert.Equal([]string{"FORMATTING"}, fa.Transitions)

	tl.Logger().Info("case: RHS FORMAT+USE permanant failure to update state before USING")
	rhs = &requestHandlerState{
		c:         c,
		Request:   &models.StorageRequest{},
		HasFormat: true,
		HasUse:    true,
		Done:      func() { done = true },
	}
	rhs.Request.Meta = &models.ObjMeta{ID: "SR-1"}
	rhs.Request.CompleteByTime = futureCompleteBy
	rhs.Request.RequestedOperations = []string{"FORMAT", "USE"}
	rhs.Request.StorageRequestState = "FORMATTING"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	done = false
	fa.FailOnState = ""
	fa.RetryOnState = ""
	fa.Transitions = []string{}
	expErr := &crud.Error{Payload: models.Error{Code: http.StatusConflict, Message: swag.String("permanent")}}
	fc.RetSRUpdaterUpdateErr = expErr
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.True(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Equal("FAILED", rhs.Request.StorageRequestState)
	if assert.Len(rhs.Request.RequestMessages, 3) {
		assert.Regexp("FORMATTING ⇒ USING", rhs.Request.RequestMessages[0].Message)
		assert.Regexp("Error: permanent", rhs.Request.RequestMessages[1].Message)
		assert.Regexp("USING ⇒ FAILED", rhs.Request.RequestMessages[2].Message)
	}
	assert.Equal([]string{"FORMATTING"}, fa.Transitions)

	tl.Logger().Info("case: RHS ATTACH+FORMAT+USE, failure in USE ⇒ ATTACHING")
	rhs = &requestHandlerState{
		c:         c,
		Request:   &models.StorageRequest{},
		HasAttach: true,
		HasFormat: true,
		HasUse:    true,
		Done:      func() { done = true },
	}
	rhs.Request.Meta = &models.ObjMeta{ID: "SR-1"}
	rhs.Request.CompleteByTime = futureCompleteBy
	rhs.Request.RequestedOperations = []string{"ATTACH", "FORMAT", "USE"}
	rhs.Request.StorageRequestState = "FORMATTING"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	done = false
	fa.FailOnState = "USING"
	fa.RetryOnState = ""
	fa.Transitions = []string{}
	fc.RetSRUpdaterUpdateErr = nil
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.True(rhs.RetryLater)
	assert.Equal("UNDO_ATTACHING", rhs.Request.StorageRequestState)
	if assert.Len(rhs.Request.RequestMessages, 2) {
		assert.Regexp("FORMATTING ⇒ USING", rhs.Request.RequestMessages[0].Message)
		assert.Regexp("USING ⇒ UNDO_ATTACHING", rhs.Request.RequestMessages[1].Message)
	}
	assert.Equal([]string{"FORMATTING", "USING"}, fa.Transitions)

	tl.Logger().Info("case: RHS ATTACH+FORMAT+USE, failure in USE, SR updates fail")
	rhs = &requestHandlerState{
		c:         c,
		Request:   &models.StorageRequest{},
		HasAttach: true,
		HasFormat: true,
		HasUse:    true,
		Done:      func() { done = true },
	}
	rhs.Request.Meta = &models.ObjMeta{ID: "SR-1"}
	rhs.Request.CompleteByTime = futureCompleteBy
	rhs.Request.RequestedOperations = []string{"ATTACH", "FORMAT", "USE"}
	rhs.Request.StorageRequestState = "FORMATTING"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	done = false
	fa.FailOnState = "USING"
	fa.RetryOnState = ""
	fa.Transitions = []string{}
	fc.RetSRUpdaterUpdateErr = expErr
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.True(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Equal("FAILED", rhs.Request.StorageRequestState)
	if assert.Len(rhs.Request.RequestMessages, 5) {
		assert.Regexp("FORMATTING ⇒ USING", rhs.Request.RequestMessages[0].Message)
		assert.Regexp("Error: permanent", rhs.Request.RequestMessages[1].Message)
		assert.Regexp("USING ⇒ UNDO_ATTACHING", rhs.Request.RequestMessages[2].Message)
		assert.Regexp("Error: permanent", rhs.Request.RequestMessages[3].Message)
		assert.Regexp("UNDO_ATTACHING ⇒ FAILED", rhs.Request.RequestMessages[4].Message)
	}
	assert.Equal([]string{"FORMATTING"}, fa.Transitions)

	tl.Logger().Info("case: RHS CLOSE via NEW")
	rhs = &requestHandlerState{
		c:        c,
		Request:  &models.StorageRequest{},
		HasClose: true,
		Done:     func() { done = true },
	}
	rhs.Request.Meta = &models.ObjMeta{ID: "SR-1"}
	rhs.Request.CompleteByTime = futureCompleteBy
	rhs.Request.RequestedOperations = []string{"CLOSE"}
	rhs.Request.StorageRequestState = "NEW"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	done = false
	fa.Transitions = []string{}
	fc.RetSRUpdaterUpdateErr = nil
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal("SUCCEEDED", rhs.Request.StorageRequestState)
	assert.Regexp("NEW ⇒ CLOSING", rhs.Request.RequestMessages[0].Message)
	assert.Regexp("CLOSING ⇒ SUCCEEDED", rhs.Request.RequestMessages[1].Message)
	assert.Equal([]string{"CLOSING"}, fa.Transitions)

	//
	tl.Logger().Info("case: RHS CLOSE+DETACH+RELEASE via CLOSING")
	rhs = &requestHandlerState{
		c:       c,
		Request: &models.StorageRequest{},
		Storage: &models.Storage{
			StorageMutable: models.StorageMutable{
				StorageIdentifier: "csp:identifier",
				StorageState: &models.StorageStateMutable{
					DeviceState: "USED",
					Messages:    []*models.TimestampedString{},
				},
			},
		},
		HasClose:   true,
		HasDetach:  true,
		HasRelease: true,
		Done:       func() { done = true },
	}
	rhs.Request.Meta = &models.ObjMeta{ID: "SR-1"}
	rhs.Request.CompleteByTime = futureCompleteBy
	rhs.Request.RequestedOperations = []string{"CLOSE", "DETACH", "RELEASE"}
	rhs.Request.StorageRequestState = "CLOSING"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	done = false
	fa.Transitions = []string{}
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal("DETACHING", rhs.Request.StorageRequestState)
	assert.Regexp("CLOSING ⇒ DETACHING", rhs.Request.RequestMessages[0].Message)
	assert.Equal([]string{"CLOSING"}, fa.Transitions)
	tl.Flush()
}
