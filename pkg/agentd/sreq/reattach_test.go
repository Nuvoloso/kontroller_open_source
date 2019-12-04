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

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
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
		ss       *models.StorageStateMutable
		states   []string
		destNode bool
		inError  bool
		inRetry  bool
	}{
		// not yet attached or different node
		{
			ss:      &models.StorageStateMutable{},
			inRetry: true,
		},
		{
			ss:      &models.StorageStateMutable{AttachedNodeID: "NODE-2", AttachmentState: "ATTACHED"},
			inRetry: true,
		},
		// dest node success cases
		{
			ss:       &models.StorageStateMutable{AttachedNodeID: "NODE-2", AttachmentState: "ATTACHED", DeviceState: "OPENING"},
			states:   []string{"USING"},
			destNode: true,
		},
		{
			ss:       &models.StorageStateMutable{AttachedNodeID: "NODE-2", AttachmentState: "ATTACHED", DeviceState: "UNUSED"},
			states:   []string{"USING"},
			destNode: true,
		},
		{
			ss:       &models.StorageStateMutable{AttachedNodeID: "NODE-2", AttachmentState: "ATTACHED", DeviceState: "ERROR"},
			states:   []string{"USING"},
			destNode: true,
		},
		{
			ss:       &models.StorageStateMutable{AttachedNodeID: "NODE-2", AttachmentState: "ATTACHED", DeviceState: "OPEN"},
			destNode: true,
		},
		// dest node failure cases
		{
			ss:       &models.StorageStateMutable{AttachedNodeID: "NODE-2", AttachmentState: "ATTACHED", DeviceState: "FORMATTING"},
			destNode: true,
			inError:  true,
		},
		{
			ss:       &models.StorageStateMutable{AttachedNodeID: "NODE-2", AttachmentState: "ATTACHED", DeviceState: "CLOSING"},
			destNode: true,
			inError:  true,
		},

		// original node success cases
		{
			ss:     &models.StorageStateMutable{AttachedNodeID: "NODE-1", AttachmentState: "ATTACHED", DeviceState: "UNUSED"},
			states: []string{"DETACHING"},
		},
		{
			ss:     &models.StorageStateMutable{AttachedNodeID: "NODE-1", AttachmentState: "ATTACHED", DeviceState: "OPEN"},
			states: []string{"CLOSING", "DETACHING"},
		},
		{
			ss:     &models.StorageStateMutable{AttachedNodeID: "NODE-1", AttachmentState: "ATTACHED", DeviceState: "CLOSING"},
			states: []string{"CLOSING", "DETACHING"},
		},
		{
			ss:     &models.StorageStateMutable{AttachedNodeID: "NODE-1", AttachmentState: "ATTACHED", DeviceState: "ERROR"},
			states: []string{"CLOSING", "DETACHING"},
		},
		// original node failure cases
		{
			ss:      &models.StorageStateMutable{AttachedNodeID: "NODE-1", AttachmentState: "ATTACHED"},
			inError: true,
		},
	}
	for i, tc := range tcs {
		t.Log("case: REATTACH", i)
		rhs.Storage.StorageState = tc.ss
		rhs.InError = false
		rhs.RetryLater = false
		if tc.destNode {
			c.thisNodeID = rhs.Request.ReattachNodeID
		} else { // originating node
			c.thisNodeID = "NODE-1"
		}
		states := c.reattachGetStates(rhs)
		assert.Equal(tc.states, states, "case %d", i)
		assert.Equal(tc.inError, rhs.InError, "case %d", i)
		assert.Equal(tc.inRetry, rhs.RetryLater, "case %d", i)
		tl.Flush()
	}

	// animator success (dest: "USING")
	t.Log("case: REATTACH dest: USING")
	rhs.HasReattach = true
	rhs.InError = false
	rhs.RetryLater = false
	rhs.Request.StorageRequestState = "USING"
	rhs.Storage.StorageState = &models.StorageStateMutable{AttachedNodeID: "NODE-2", AttachmentState: "ATTACHED", DeviceState: "UNUSED"}
	requestDone := false
	rhs.Done = func() { requestDone = true }
	ctx := context.Background()
	fa := newFakeRequestHandlers()
	c.requestHandlers = fa
	c.thisNodeID = "NODE-2"
	c.requestAnimator(ctx, rhs)
	tl.Flush()
	assert.True(requestDone)
	assert.NotEmpty(fa.UseIds)
	assert.Empty(fa.CloseIds)
	assert.Empty(fa.FmtIds)
	assert.Equal([]string{"USING"}, fa.Transitions)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Equal("SUCCEEDED", rhs.Request.StorageRequestState)

	// animator success (dest: device already in use)
	rhs.HasReattach = true
	rhs.InError = false
	rhs.RetryLater = false
	rhs.Request.StorageRequestState = "USING"
	rhs.Storage.StorageState = &models.StorageStateMutable{AttachedNodeID: "NODE-2", AttachmentState: "ATTACHED", DeviceState: "OPEN"}
	requestDone = false
	rhs.Done = func() { requestDone = true }
	fa = newFakeRequestHandlers()
	c.requestHandlers = fa
	c.thisNodeID = "NODE-2"
	c.requestAnimator(ctx, rhs)
	tl.Flush()
	assert.True(requestDone)
	assert.Empty(fa.UseIds)
	assert.Empty(fa.CloseIds)
	assert.Empty(fa.FmtIds)
	assert.Empty(fa.Transitions)
	assert.False(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Equal("SUCCEEDED", rhs.Request.StorageRequestState)

	// animator failure (dest: open fails, storage state not affected)
	rhs.HasReattach = true
	rhs.InError = false
	rhs.RetryLater = false
	rhs.Request.StorageRequestState = "USING"
	rhs.Storage.StorageState = &models.StorageStateMutable{AttachedNodeID: "NODE-2", AttachmentState: "ATTACHED", DeviceState: "UNUSED"}
	requestDone = false
	rhs.Done = func() { requestDone = true }
	fa = newFakeRequestHandlers()
	fa.FailOnState = "USING"
	c.requestHandlers = fa
	c.thisNodeID = "NODE-2"
	c.requestAnimator(ctx, rhs)
	tl.Flush()
	assert.True(requestDone)
	assert.NotEmpty(fa.UseIds)
	assert.Empty(fa.CloseIds)
	assert.Empty(fa.FmtIds)
	assert.Equal([]string{"USING"}, fa.Transitions)
	assert.True(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Equal("FAILED", rhs.Request.StorageRequestState)
	assert.Equal("UNUSED", rhs.Storage.StorageState.DeviceState)

	// animator success (origin: "CLOSING", "DETACHING")
	rhs.HasReattach = true
	rhs.InError = false
	rhs.RetryLater = false
	rhs.Request.StorageRequestState = "CLOSING"
	rhs.Storage.StorageState = &models.StorageStateMutable{AttachedNodeID: "NODE-1", AttachmentState: "ATTACHED", DeviceState: "OPEN"}
	requestDone = false
	rhs.Done = func() { requestDone = true }
	fa = newFakeRequestHandlers()
	c.requestHandlers = fa
	c.thisNodeID = "NODE-1"
	c.requestAnimator(ctx, rhs)
	tl.Flush()
	assert.True(requestDone)
	assert.Empty(fa.UseIds)
	assert.NotEmpty(fa.CloseIds)
	assert.Empty(fa.FmtIds)
	assert.Equal([]string{"CLOSING"}, fa.Transitions)
	assert.False(rhs.InError)
	assert.True(rhs.RetryLater)
	assert.Equal("DETACHING", rhs.Request.StorageRequestState)

	// animator failure (origin: "CLOSING", storage state not affected)
	rhs.HasReattach = true
	rhs.InError = false
	rhs.RetryLater = false
	rhs.Request.StorageRequestState = "CLOSING"
	rhs.Storage.StorageState = &models.StorageStateMutable{AttachedNodeID: "NODE-1", AttachmentState: "ATTACHED", DeviceState: "OPEN"}
	requestDone = false
	rhs.Done = func() { requestDone = true }
	fa = newFakeRequestHandlers()
	fa.FailOnState = "CLOSING"
	c.requestHandlers = fa
	c.thisNodeID = "NODE-1"
	c.requestAnimator(ctx, rhs)
	tl.Flush()
	assert.True(requestDone)
	assert.Empty(fa.UseIds)
	assert.NotEmpty(fa.CloseIds)
	assert.Empty(fa.FmtIds)
	assert.Equal([]string{"CLOSING"}, fa.Transitions)
	assert.True(rhs.InError)
	assert.False(rhs.RetryLater)
	assert.Equal("FAILED", rhs.Request.StorageRequestState)
	assert.Equal("OPEN", rhs.Storage.StorageState.DeviceState)
}

func TestReattachDispatch(t *testing.T) {
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

	// no-dispatch cases
	noDispatchStates := []string{
		com.StgReqStateAttaching,
		com.StgReqStateDetaching,
		com.StgReqStateReattaching,
	}
	for _, state := range noDispatchStates {
		t.Log("case: no dispatch", state)
		srObj.StorageRequestState = state
		cnt := c.dispatchRequests(ctx, []*models.StorageRequest{srObj})
		assert.Equal(0, cnt)
		tl.Flush()
	}

	// dispatch cases
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
			StorageState: &models.StorageStateMutable{
				AttachedNodeDevice: "/dev/xvb0",
				AttachedNodeID:     "NODE-1",
				AttachmentState:    "ATTACHED",
				ProvisionedState:   "PROVISIONED",
			},
		},
	}
	dispatchStates := []string{com.StgReqStateNew, com.StgReqStateClosing, com.StgReqStateUsing}
	for _, state := range dispatchStates {
		t.Log("case: dispatch", state)
		srObj.StorageRequestState = state
		srObj.Meta.ID = models.ObjID("SR-" + state) // give each a distinct id to be run in parallel
		cnt := c.dispatchRequests(ctx, []*models.StorageRequest{srObj})
		assert.Equal(1, cnt)
		tl.Flush()
	}
	for len(c.activeRequests) > 0 {
		time.Sleep(5 * time.Millisecond)
	}
}
