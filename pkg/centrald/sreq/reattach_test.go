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
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestReattachAnimator(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log: tl.Logger(),
		},
	}
	app.AppCSP = app
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM

	c := &Component{}
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc

	now := time.Now()
	cbTime := now.Add(time.Hour * 1)
	rhs := &requestHandlerState{}
	rhs.c = c
	rhs.Request = &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:      "SR-1",
				Version: 1,
			},
		},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			RequestedOperations: []string{"REATTACH"},
			CompleteByTime:      strfmt.DateTime(cbTime),
			CspDomainID:         "CSP-DOMAIN-1",
			ReattachNodeID:      "NODE-2",
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestCreateMutable: models.StorageRequestCreateMutable{
				StorageID: "STORAGE-1",
			},
		},
	}
	rhs.Storage = &models.Storage{}
	rhs.Storage.Meta = &models.ObjMeta{ID: "STORAGE-1"}

	// test reattachGetStates
	tcs := []struct {
		ss      *models.StorageStateMutable
		states  []string
		setNode bool
	}{
		{
			ss:     &models.StorageStateMutable{AttachedNodeID: "NODE-2", AttachmentState: "ATTACHED"},
			states: []string{"USING"},
		},
		{
			ss:     &models.StorageStateMutable{AttachedNodeID: "NODE-1", AttachmentState: "ATTACHED", DeviceState: "OPEN"},
			states: []string{"CLOSING"},
		},
		{
			ss:     &models.StorageStateMutable{AttachedNodeID: "NODE-1", AttachmentState: "ATTACHED", DeviceState: "CLOSING"},
			states: []string{"CLOSING"},
		},
		{
			ss:     &models.StorageStateMutable{AttachedNodeID: "NODE-1", AttachmentState: "ATTACHED", DeviceState: "ERROR"},
			states: []string{"CLOSING"},
		},
		{
			ss:     &models.StorageStateMutable{AttachedNodeID: "NODE-1", AttachmentState: "ATTACHED", DeviceState: "UNUSED"},
			states: []string{"DETACHING", "REATTACHING", "ATTACHING", "USING"},
		},
		{
			ss:     &models.StorageStateMutable{AttachedNodeID: "NODE-1", AttachmentState: "DETACHING", DeviceState: "UNUSED"},
			states: []string{"REATTACHING", "ATTACHING", "USING"},
		},
		{
			ss:     &models.StorageStateMutable{AttachedNodeID: "", AttachmentState: "DETACHED", DeviceState: "UNUSED"},
			states: []string{"REATTACHING", "ATTACHING", "USING"},
		},
		{
			ss:     &models.StorageStateMutable{AttachedNodeID: "", AttachmentState: "ERROR", DeviceState: "UNUSED"},
			states: []string{"REATTACHING", "ATTACHING", "USING"},
		},
		{
			ss:     &models.StorageStateMutable{AttachedNodeID: "", AttachmentState: "", DeviceState: "UNUSED"},
			states: []string{"REATTACHING", "ATTACHING", "USING"},
		},
		{
			ss:      &models.StorageStateMutable{AttachedNodeID: "NODE-2", AttachmentState: "DETACHED", DeviceState: "UNUSED"},
			states:  []string{"ATTACHING", "USING"},
			setNode: true,
		},
	}
	for i, tc := range tcs {
		t.Log("case: REATTACH", i)
		rhs.Storage.StorageState = tc.ss
		if tc.setNode {
			rhs.Request.NodeID = rhs.Request.ReattachNodeID
		} else {
			rhs.Request.NodeID = "NODE-1"
		}
		states := c.reattachGetStates(rhs)
		assert.Equal(tc.states, states, "case %d", i)
		tl.Flush()
	}

	// animator success ("DETACHING", "REATTACHING", "ATTACHING", "USING")
	t.Log("case: REATTACH success")
	rhs.HasReattach = true
	rhs.Request.StorageRequestState = "DETACHING"
	rhs.Storage.StorageState = &models.StorageStateMutable{AttachedNodeID: "NODE-1", AttachmentState: "ATTACHED", DeviceState: "UNUSED"}
	requestDone := false
	rhs.Done = func() { requestDone = true }
	ctx := context.Background()
	fa := newFakeRequestHandlers()
	c.requestHandlers = fa
	c.requestAnimator(ctx, rhs)
	tl.Flush()
	assert.True(requestDone)
	assert.NotEmpty(fa.DetIds)
	assert.NotEmpty(fa.ReAttIds)
	assert.NotEmpty(fa.AttIds)
	assert.Empty(fa.ProvIds)
	assert.Empty(fa.RelIds)
	assert.Equal([]string{"DETACHING", "REATTACHING", "ATTACHING"}, fa.Transitions)

	// animator ATTACHING fails, no undo
	t.Log("case: REATTACH fail in ATTACHING")
	requestDone = false
	rhs.RetryLater = false
	rhs.InError = false
	rhs.TimedOut = false
	fa = newFakeRequestHandlers()
	fa.FailOnState = "ATTACHING"
	c.requestHandlers = fa
	c.requestAnimator(ctx, rhs)
	tl.Flush()
	assert.True(requestDone)
	assert.NotEmpty(fa.DetIds)
	assert.NotEmpty(fa.ReAttIds)
	assert.NotEmpty(fa.AttIds)
	assert.Empty(fa.ProvIds)
	assert.Empty(fa.RelIds)
	assert.Empty(fa.UndoDetIds)
	assert.Equal([]string{"DETACHING", "REATTACHING", "ATTACHING"}, fa.Transitions)

	// animator DETACHING fails, no undo
	t.Log("case: REATTACH fail in DETACHING")
	requestDone = false
	rhs.RetryLater = false
	rhs.InError = false
	rhs.TimedOut = false
	fa = newFakeRequestHandlers()
	fa.FailOnState = "DETACHING"
	c.requestHandlers = fa
	c.requestAnimator(ctx, rhs)
	tl.Flush()
	assert.True(requestDone)
	assert.NotEmpty(fa.DetIds)
	assert.Empty(fa.ReAttIds)
	assert.Empty(fa.AttIds)
	assert.Empty(fa.ProvIds)
	assert.Empty(fa.RelIds)
	assert.Empty(fa.UndoDetIds)
	assert.Equal([]string{"DETACHING"}, fa.Transitions)

	// test the handler
	t.Log("case: ReattachSwitchNodes")
	assert.EqualValues("NODE-2", rhs.Request.ReattachNodeID)
	rhs.Request.NodeID = "NODE-1"
	rhs.SetNodeID = false
	rhs.RetryLater = false
	rhs.InError = false
	rhs.TimedOut = false
	c.ReattachSwitchNodes(ctx, rhs)
	assert.False(rhs.RetryLater)
	assert.False(rhs.InError)
	assert.False(rhs.TimedOut)
	assert.Equal(rhs.Request.ReattachNodeID, rhs.Request.NodeID)
	assert.EqualValues("NODE-2", rhs.Request.NodeID)
	assert.True(rhs.SetNodeID)
}

func TestReattachDispatch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &centrald.AppCtx{
		AppArgs: centrald.AppArgs{
			Log: tl.Logger(),
		},
	}
	app.AppCSP = app
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM

	c := &Component{}
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc
	fa := newFakeRequestHandlers()
	c.requestHandlers = fa

	now := time.Now()
	cbTime := now.Add(time.Hour * 1)
	ctx := context.Background()

	srObj := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:      "SR-R1",
				Version: 1,
			},
		},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			RequestedOperations: []string{"REATTACH"},
			CompleteByTime:      strfmt.DateTime(cbTime),
			CspDomainID:         "CSP-DOMAIN-1",
			ReattachNodeID:      "NODE-2",
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestCreateMutable: models.StorageRequestCreateMutable{
				StorageID: "STORAGE-R1",
			},
		},
	}

	// dispatch cases
	fc.RetLDObj = &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID:      "CSP-DOMAIN-1",
				Version: 1,
			},
			CspDomainType: "AWS",
		},
		CSPDomainMutable: models.CSPDomainMutable{
			Name: "MyCSPDomain",
		},
	}
	fc.RetPoolFetchObj = &models.Pool{
		PoolAllOf0: models.PoolAllOf0{
			Meta: &models.ObjMeta{
				ID:      "POOL-1",
				Version: 1,
			},
		},
	}
	fc.StorageFetchRetObj = &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta: &models.ObjMeta{
				ID:      "STORAGE-R1",
				Version: 1,
			},
			CspDomainID:    "CSP-DOMAIN-1",
			CspStorageType: "Amazon gp2",
			SizeBytes:      swag.Int64(107374182400),
			StorageAccessibility: &models.StorageAccessibility{
				StorageAccessibilityMutable: models.StorageAccessibilityMutable{
					AccessibilityScope:      "CSPDOMAIN",
					AccessibilityScopeObjID: "CSP-DOMAIN-1",
				},
			},
			PoolID: "POOL-1",
		},
		StorageMutable: models.StorageMutable{
			AvailableBytes:    swag.Int64(10737418240),
			StorageIdentifier: "vol:volume-1",
		},
	}
	ssNoProgress := &models.StorageStateMutable{
		AttachedNodeDevice: "/dev/xvb0",
		AttachedNodeID:     "NODE-1",
		AttachmentState:    "ATTACHED",
		DeviceState:        "OPEN",
		ProvisionedState:   "PROVISIONED",
	}

	c.mux.Lock()
	c.activeRequests = map[models.ObjID]*requestHandlerState{}
	c.mux.Unlock()

	var srCopy *models.StorageRequest
	notNewDispatchCases := []string{com.StgReqStateDetaching, com.StgReqStateReattaching, com.StgReqStateAttaching}
	for _, state := range notNewDispatchCases {
		t.Log("case: dispatch", state)
		srObj.StorageRequestState = state
		srObj.Meta.ID = models.ObjID("SR-" + state) // give each a distinct id to be run in parallel
		srCopy = nil                                // don't let Clone reuse the top-level object
		testutils.Clone(srObj, &srCopy)             // each parallel dispatched SR must be a distinct object
		fc.StorageFetchRetObj.StorageState = ssNoProgress
		cnt := c.dispatchRequests(ctx, []*models.StorageRequest{srCopy})
		assert.Equal(1, cnt)
		tl.Flush()
	}
	for len(c.activeRequests) > 0 {
		time.Sleep(5 * time.Millisecond)
	}

	newDispatchCases := []*models.StorageStateMutable{ // partial progress
		{
			AttachedNodeDevice: "/dev/xvb0",
			AttachedNodeID:     "NODE-1",
			AttachmentState:    "ATTACHED",
			DeviceState:        "UNUSED",
			ProvisionedState:   "PROVISIONED",
		},
		{
			AttachmentState:  "DETACHED",
			DeviceState:      "UNUSED",
			ProvisionedState: "PROVISIONED",
		},
	}
	for _, ss := range newDispatchCases {
		srObj.StorageRequestState = com.StgReqStateNew
		srObj.Meta.ID = models.ObjID("SR-NEW" + ss.AttachmentState + "-" + ss.DeviceState)
		srCopy = nil // don't let Clone reuse the top-level object
		testutils.Clone(srObj, &srCopy)
		fc.StorageFetchRetObj.StorageState = ss
		cnt := c.dispatchRequests(ctx, []*models.StorageRequest{srCopy})
		assert.Equal(1, cnt, "case: New %s", srObj.Meta.ID)
		tl.Flush()
	}

	// no-dispatch cases
	noDispatchStates := []string{
		com.StgReqStateClosing,
		com.StgReqStateNew, // Storage does not display partial progress
		com.StgReqStateProvisioning,
		com.StgReqStateReleasing,
		com.StgReqStateUsing,
	}
	fc.StorageFetchRetObj.StorageState = ssNoProgress
	for _, state := range noDispatchStates {
		t.Log("case: no dispatch", state)
		srObj.StorageRequestState = state
		srObj.Meta.ID = models.ObjID("SR-ND-" + state) // give each a distinct id
		srCopy = nil                                   // don't let Clone reuse the top-level object
		testutils.Clone(srObj, &srCopy)
		cnt := c.dispatchRequests(ctx, []*models.StorageRequest{srCopy})
		assert.Equal(0, cnt, "case: New %s", srObj.Meta.ID)
		tl.Flush()
	}

	// invalid invocation of dispatchNewReattachSR
	srObj.StorageRequestState = com.StgReqStateClosing
	assert.PanicsWithValue("invalid invocation of dispatchNewReattachSR", func() { c.dispatchNewReattachSR(&requestHandlerState{Request: srObj}) })
}
