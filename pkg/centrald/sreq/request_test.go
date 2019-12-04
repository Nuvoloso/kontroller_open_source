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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/cluster"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/pool"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
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
	ProvIds       []string
	AttIds        []string
	DetIds        []string
	UndoDetIds    []string
	RelIds        []string
	ReAttIds      []string
	RemoveTagIds  []string
	signalled     bool
	resetSignal   bool
	mux           sync.Mutex
	cond          *sync.Cond
}

var _ = requestHandlers(&fakeRequestHandlers{})

func newFakeRequestHandlers() *fakeRequestHandlers {
	fa := &fakeRequestHandlers{}
	fa.Transitions = make([]string, 0)
	fa.ProvIds = make([]string, 0)
	fa.AttIds = make([]string, 0)
	fa.DetIds = make([]string, 0)
	fa.RelIds = make([]string, 0)
	fa.ReAttIds = make([]string, 0)
	fa.RemoveTagIds = make([]string, 0)
	fa.cond = sync.NewCond(&fa.mux)
	return fa
}

func (fa *fakeRequestHandlers) common(ctx context.Context, rhs *requestHandlerState, thisState string) {
	fa.mux.Lock()
	if fa.WaitForSignal {
		if fa.resetSignal {
			fa.signalled = false
		}
		for !fa.signalled {
			fa.BlockCount++
			fa.cond.Wait()
		}
	}
	fa.Count++
	fa.Transitions = append(fa.Transitions, thisState)
	switch thisState {
	case "PROVISIONING":
		fa.ProvIds = append(fa.ProvIds, string(rhs.Request.Meta.ID))
	case "ATTACHING":
		fa.AttIds = append(fa.AttIds, string(rhs.Request.Meta.ID))
	case "DETACHING":
		fa.DetIds = append(fa.DetIds, string(rhs.Request.Meta.ID))
	case "UNDO_DETACHING":
		fa.UndoDetIds = append(fa.UndoDetIds, string(rhs.Request.Meta.ID))
	case "RELEASING":
		fa.RelIds = append(fa.RelIds, string(rhs.Request.Meta.ID))
	case "REATTACHING":
		fa.ReAttIds = append(fa.ReAttIds, string(rhs.Request.Meta.ID))
	case "REMOVING_TAG":
		fa.RemoveTagIds = append(fa.RemoveTagIds, string(rhs.Request.Meta.ID))
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
	sort.Strings(fa.ProvIds)
	sort.Strings(fa.AttIds)
	sort.Strings(fa.DetIds)
	sort.Strings(fa.RelIds)
}

func (fa *fakeRequestHandlers) Signal() {
	fa.mux.Lock()
	fa.signalled = true
	fa.cond.Broadcast()
	fa.mux.Unlock()
}

func (fa *fakeRequestHandlers) SignalOnce() {
	fa.resetSignal = true
	fa.Signal()
}

func (fa *fakeRequestHandlers) ProvisionStorage(ctx context.Context, rhs *requestHandlerState) {
	fa.common(ctx, rhs, "PROVISIONING")
}

func (fa *fakeRequestHandlers) AttachStorage(ctx context.Context, rhs *requestHandlerState) {
	fa.common(ctx, rhs, "ATTACHING")
}

func (fa *fakeRequestHandlers) DetachStorage(ctx context.Context, rhs *requestHandlerState) {
	fa.common(ctx, rhs, "DETACHING")
}

func (fa *fakeRequestHandlers) UndoDetachStorage(ctx context.Context, rhs *requestHandlerState) {
	fa.common(ctx, rhs, "UNDO_DETACHING")
}

func (fa *fakeRequestHandlers) ReleaseStorage(ctx context.Context, rhs *requestHandlerState) {
	fa.common(ctx, rhs, "RELEASING")
}

func (fa *fakeRequestHandlers) ReattachSwitchNodes(ctx context.Context, rhs *requestHandlerState) {
	fa.common(ctx, rhs, "REATTACHING")
}

func (fa *fakeRequestHandlers) RemoveTag(ctx context.Context, rhs *requestHandlerState) {
	fa.common(ctx, rhs, "REMOVING_TAG")
}

// a custom matcher that only matches on the SR ID, and only returns true/false, no asserts
type mockStorageRequestUpdateMatcher struct {
	id string
}

func newStorageRequestUpdateMatcher(id models.ObjID) *mockStorageRequestUpdateMatcher {
	return &mockStorageRequestUpdateMatcher{id: string(id)}
}

// Matches is from gomock.Matcher
func (o *mockStorageRequestUpdateMatcher) Matches(x interface{}) bool {
	p, ok := x.(*storage_request.StorageRequestUpdateParams)
	return ok && p.ID == o.id
}

// String is from gomock.Matcher
func (o *mockStorageRequestUpdateMatcher) String() string {
	return "update.ID matches " + o.id
}

func TestDispatchRequest(t *testing.T) {
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

	resD := &csp_domain.CspDomainFetchOK{
		Payload: &models.CSPDomain{
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
		},
	}
	resSP := &pool.PoolFetchOK{
		Payload: &models.Pool{
			PoolAllOf0: models.PoolAllOf0{
				Meta: &models.ObjMeta{
					ID:      "POOL-1",
					Version: 1,
				},
			},
		},
	}
	resC := &cluster.ClusterFetchOK{
		Payload: &models.Cluster{
			ClusterAllOf0: models.ClusterAllOf0{
				Meta: &models.ObjMeta{
					ID:      "CLUSTER-1",
					Version: 1,
				},
			},
		},
	}
	resS := &storage.StorageFetchOK{
		Payload: &models.Storage{
			StorageAllOf0: models.StorageAllOf0{
				Meta: &models.ObjMeta{
					ID:      "STORAGE-1",
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
					AttachmentState:    "ATTACHED", // Note: state relevant for FORMAT/USE cases
					ProvisionedState:   "PROVISIONED",
				},
			},
		},
	}
	resSR := &storage_request.StorageRequestUpdateOK{
		Payload: &models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID:      "fb213ad9-4b9b-44ca-adf0-968babf7bb2c",
					Version: 1,
				},
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "NEW",
				},
			},
		},
	}

	cbTime := time.Now().Add(time.Hour * 1)
	srs := []*models.StorageRequest{
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "prov-only",
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				CspDomainID:         "CSP-DOMAIN-1",
				PoolID:              "POOL-1",
				ClusterID:           "CLUSTER-1",
				RequestedOperations: []string{"PROVISION"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "PROVISIONING",
				},
				StorageRequestCreateMutable: models.StorageRequestCreateMutable{
					StorageID: "STORAGE-1", // has storage - recovery case
				},
			},
		},
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "att-only",
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				CspDomainID:         "CSP-DOMAIN-1",
				RequestedOperations: []string{"ATTACH"},
				ClusterID:           "CLUSTER-1",
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "ATTACHING",
				},
				StorageRequestCreateMutable: models.StorageRequestCreateMutable{
					NodeID:    "NODE-1",
					StorageID: "STORAGE-1",
				},
			},
		},
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "det-only",
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				CspDomainID:         "CSP-DOMAIN-1",
				RequestedOperations: []string{"DETACH"},
				ClusterID:           "CLUSTER-1",
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "DETACHING",
				},
				StorageRequestCreateMutable: models.StorageRequestCreateMutable{
					StorageID: "STORAGE-1",
				},
			},
		},
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "rel-only",
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				CspDomainID:         "CSP-DOMAIN-1",
				RequestedOperations: []string{"RELEASE"},
				ClusterID:           "CLUSTER-1",
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "RELEASING",
				},
				StorageRequestCreateMutable: models.StorageRequestCreateMutable{
					StorageID: "STORAGE-1",
				},
			},
		},
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "undo-prov-only",
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				CspDomainID:         "CSP-DOMAIN-1",
				PoolID:              "POOL-1",
				RequestedOperations: []string{"PROVISION"},
				ClusterID:           "CLUSTER-1",
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "UNDO_PROVISIONING",
				},
				StorageRequestCreateMutable: models.StorageRequestCreateMutable{
					StorageID: "STORAGE-1", // has storage - gets released
				},
			},
		},
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "undo-att-only",
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				CspDomainID:         "CSP-DOMAIN-1",
				RequestedOperations: []string{"ATTACH"},
				ClusterID:           "CLUSTER-1",
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "UNDO_ATTACHING",
				},
				StorageRequestCreateMutable: models.StorageRequestCreateMutable{
					NodeID:    "NODE-1",
					StorageID: "STORAGE-1",
				},
			},
		},
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "undo-det-only",
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				CspDomainID:         "CSP-DOMAIN-1",
				RequestedOperations: []string{"DETACH"},
				ClusterID:           "CLUSTER-1",
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "UNDO_DETACHING",
				},
				StorageRequestCreateMutable: models.StorageRequestCreateMutable{
					StorageID: "STORAGE-1",
				},
			},
		},
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "format-intended-for-agentd",
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				CspDomainID:         "CSP-DOMAIN-1",
				PoolID:              "POOL-1",
				RequestedOperations: []string{"PROVISION", "ATTACH", "FORMAT"},
				ClusterID:           "CLUSTER-1",
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "ATTACHING",
				},
				StorageRequestCreateMutable: models.StorageRequestCreateMutable{
					NodeID:    "NODE-1",
					StorageID: "STORAGE-1", // has attached storage - recovery case before agentd picks it up
				},
			},
		},
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "use-intended-for-agentd",
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				CspDomainID:         "CSP-DOMAIN-1",
				PoolID:              "POOL-1",
				RequestedOperations: []string{"PROVISION", "ATTACH", "USE"},
				ClusterID:           "CLUSTER-1",
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "ATTACHING", // attach must detect that storage it attached and transition
				},
				StorageRequestCreateMutable: models.StorageRequestCreateMutable{
					NodeID:    "NODE-1",
					StorageID: "STORAGE-1", // has attached storage - recovery case before agentd picks it up
				},
			},
		},
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "error-because-fail-on-storage-load",
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				CspDomainID:         "CSP-DOMAIN-1",
				RequestedOperations: []string{"ATTACH"},
				ClusterID:           "CLUSTER-1",
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "ATTACHING",
				},
				StorageRequestCreateMutable: models.StorageRequestCreateMutable{
					StorageID: "STORAGE-FAIL",
				},
			},
		},
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "dispatched-in-spite-of-fail-on-storage-load",
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				CspDomainID:         "CSP-DOMAIN-1",
				RequestedOperations: []string{"RELEASE"},
				ClusterID:           "CLUSTER-1",
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "RELEASING",
				},
				StorageRequestCreateMutable: models.StorageRequestCreateMutable{
					StorageID: "STORAGE-FAIL",
				},
			},
		},
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "error-on-csp-domain-load-error",
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				CspDomainID:         "CSP-DOMAIN-FAIL",
				PoolID:              "POOL-1",
				RequestedOperations: []string{"PROVISION"},
				ClusterID:           "CLUSTER-1",
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "PROVISIONING",
				},
			},
		},
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "format-being-processed-by-agentd",
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				RequestedOperations: []string{"FORMAT"},
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "FORMATTING",
				},
			},
		},
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "use-being-processed-by-agentd",
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				RequestedOperations: []string{"USE"},
				ClusterID:           "CLUSTER-1",
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "USING",
				},
			},
		},
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "new-format-processed-by-agentd",
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				RequestedOperations: []string{"FORMAT"},
				ClusterID:           "CLUSTER-1",
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "NEW",
				},
			},
		},
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "new-use-processed-by-agentd",
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				RequestedOperations: []string{"USE"},
				ClusterID:           "CLUSTER-1",
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "NEW",
				},
			},
		},
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "new-close-processed-by-agentd",
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				RequestedOperations: []string{"CLOSE"},
				ClusterID:           "CLUSTER-1",
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "NEW",
				},
			},
		},
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "close-being-processed-by-agentd",
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				RequestedOperations: []string{"CLOSE"},
				ClusterID:           "CLUSTER-1",
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "CLOSING",
				},
			},
		},
	}

	expProvIds := []string{ // sorted
		"prov-only",
	}
	expAttIds := []string{ // sorted
		"att-only", "format-intended-for-agentd", "use-intended-for-agentd",
	}
	expDetIds := []string{ // sorted (call to mock SR Update for error request changes ID to that of retSR)
		"det-only",
		"fb213ad9-4b9b-44ca-adf0-968babf7bb2c",
		"undo-att-only",
	}
	expRelIds := []string{ // sorted (call to mock SR Update for error request changes ID to that of retSR)
		"dispatched-in-spite-of-fail-on-storage-load",
		"fb213ad9-4b9b-44ca-adf0-968babf7bb2c",
		"rel-only",
		"undo-prov-only",
	}
	expRemoveTagIds := []string{
		"prov-only",
	}
	expUndoDetIds := []string{"undo-det-only"}
	isErrReq := map[string]struct{}{
		"error-on-csp-domain-load-error":     struct{}{},
		"error-because-fail-on-storage-load": struct{}{},
	}
	errCount := len(isErrReq)
	uniqueIds := map[string]struct{}{} // for fake handler
	for _, x := range expProvIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expAttIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expDetIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expRelIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expUndoDetIds {
		uniqueIds[x] = struct{}{}
	}
	idCount := len(uniqueIds) + errCount - 1
	firstDispatchCount := idCount
	tl.Logger().Debugf("** StorageRequest #tcs: %d", len(srs))
	tl.Logger().Debugf("** StorageRequest #unique ids: %d", len(uniqueIds))
	tl.Logger().Debugf("** StorageRequest idCount: %d", idCount)
	tl.Logger().Debugf("** StorageRequest errCount: %d", errCount)
	tl.Logger().Debugf("** StorageRequest opCount: %d", firstDispatchCount)

	fa := newFakeRequestHandlers()
	fa.WaitForSignal = true
	c.requestHandlers = fa
	assert.Empty(c.activeRequests)
	assert.Empty(c.doneRequests)

	// mix of success and non-database errors
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	dOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	mD := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainFetchParams().WithID(string(resD.Payload.Meta.ID)))
	dop1 := dOps.EXPECT().CspDomainFetch(mD).Return(resD, nil).Times(1)
	mDFail := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainFetchParams().WithID("CSP-DOMAIN-FAIL"))
	dOps.EXPECT().CspDomainFetch(mDFail).Return(nil, fmt.Errorf("csp domain load error")).After(dop1)
	sOps := mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(sOps).AnyTimes()
	mS := mockmgmtclient.NewStorageMatcher(t, storage.NewStorageFetchParams().WithID(string(resS.Payload.Meta.ID)))
	op1 := sOps.EXPECT().StorageFetch(mS).Return(resS, nil).Times(9)
	mSFail := mockmgmtclient.NewStorageMatcher(t, storage.NewStorageFetchParams().WithID("STORAGE-FAIL"))
	sOps.EXPECT().StorageFetch(mSFail).Return(nil, fmt.Errorf("storage load error")).Times(2).After(op1)
	spOps := mockmgmtclient.NewMockPoolClient(mockCtrl)
	mAPI.EXPECT().Pool().Return(spOps).Times(1) // gets cached
	mSP := mockmgmtclient.NewPoolMatcher(t, pool.NewPoolFetchParams().WithID(string(resSP.Payload.Meta.ID)))
	spOps.EXPECT().PoolFetch(mSP).Return(resSP, nil).Times(1) // gets cached
	cOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(cOps).Times(1) // gets cached
	mC := mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterFetchParams().WithID(string(resC.Payload.Meta.ID)))
	cOps.EXPECT().ClusterFetch(mC).Return(resC, nil).Times(1) // gets cached
	// only the error requests change state (to UNDO states)
	srOps := mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).MinTimes(errCount)
	srOps.EXPECT().StorageRequestUpdate(gomock.Not(gomock.Nil())).Return(resSR, nil).MinTimes(errCount)
	tl.Logger().Debug("** calling dispatchRequests")
	tl.Flush()
	cnt := c.dispatchRequests(nil, srs)
	tl.Flush()
	assert.Equal(firstDispatchCount, cnt)
	tl.Logger().Debug("** Waiting for handlers to be dispatched")
	tl.Flush()

	// wait until all requests block in the fake handlers
	for fa.BlockCount < idCount {
		time.Sleep(time.Millisecond * 1)
	}
	tl.Logger().Debug("** All requests are blocked")
	assert.Equal(fa.BlockCount, len(c.activeRequests))
	assert.Empty(c.doneRequests)
	tl.Flush()

	// reissue the non-error requests again - they should all be ignored
	mockCtrl.Finish()
	srs2 := []*models.StorageRequest{}
	for _, sr := range srs {
		if _, ok := isErrReq[string(sr.Meta.ID)]; ok {
			continue
		}
		srs2 = append(srs2, sr)
	}
	assert.Len(srs2, len(srs)-errCount)
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	tl.Logger().Debug("** Calling dispatchRequests again on non-error requests")
	tl.Flush()
	cnt = c.dispatchRequests(nil, srs2)
	assert.Equal(0, cnt)

	// wake up handlers and wait for completion
	tl.Flush()
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).MinTimes(4)
	srm := newStorageRequestUpdateMatcher("prov-only")
	resProvSR := &storage_request.StorageRequestUpdateOK{Payload: srs[0]}
	srOps.EXPECT().StorageRequestUpdate(srm).Return(resProvSR, nil).MinTimes(2)
	srOps.EXPECT().StorageRequestUpdate(gomock.Not(srm)).Return(resSR, nil).MinTimes(3)
	fa.Signal()
	for len(c.activeRequests) > 0 {
		time.Sleep(time.Millisecond * 1)
	}
	assert.Equal(idCount+1, fa.Count)
	assert.Len(c.doneRequests, idCount)
	fa.Sort()
	assert.Equal(expProvIds, fa.ProvIds)
	assert.Equal(expAttIds, fa.AttIds)
	assert.Equal(expDetIds, fa.DetIds)
	assert.Equal(expRelIds, fa.RelIds)
	assert.Equal(expRemoveTagIds, fa.RemoveTagIds)
	assert.Equal(expUndoDetIds, fa.UndoDetIds)

	// case: dispatch of multi-operation for centrald and agentd
	ftSRS := []*models.StorageRequest{
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "provision-att-format-use",
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				CspDomainID:         "CSP-DOMAIN-1",
				CspStorageType:      "STORAGE-TYPE",
				PoolID:              "POOL-1",
				RequestedOperations: []string{"PROVISION", "ATTACH", "FORMAT", "USE"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestCreateMutable: models.StorageRequestCreateMutable{
					NodeID: "NODE-1",
				},
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "PROVISIONING", // avoid a crud call
				},
			},
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainFetch(mD).Return(resD, nil).Times(1)
	tl.Logger().Debug("** Calling dispatchRequests: PROVISION+ATTACH+FORMAT+USE")
	tl.Flush()
	fa = newFakeRequestHandlers()
	fa.WaitForSignal = true
	c.requestHandlers = fa
	assert.Equal(0, len(c.activeRequests))
	cnt = c.dispatchRequests(nil, ftSRS)
	assert.Equal(1, cnt)
	assert.Equal(1, len(c.activeRequests))
	for fa.BlockCount < 1 {
		time.Sleep(time.Millisecond * 1)
	}
	tl.Logger().Debug("** ftSR request is blocked")

	// case: dispatch of PROVISION with poolId set.
	spSRs := []*models.StorageRequest{
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "provision-with-sp-id",
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				CspDomainID:         "CSP-DOMAIN-1",
				CspStorageType:      "STORAGE-TYPE",
				PoolID:              "POOL-1",
				RequestedOperations: []string{"PROVISION"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestCreateMutable: models.StorageRequestCreateMutable{
					NodeID: "NODE-1",
				},
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "PROVISIONING", // avoid a crud call
				},
			},
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainFetch(mD).Return(resD, nil).Times(1)
	tl.Logger().Debug("** Calling dispatchRequests: spID")
	tl.Flush()
	fa = newFakeRequestHandlers()
	fa.WaitForSignal = true
	c.requestHandlers = fa
	c.activeRequests = map[models.ObjID]*requestHandlerState{}
	cnt = c.dispatchRequests(nil, spSRs)
	assert.Equal(1, cnt)
	assert.Equal(1, len(c.activeRequests))
	for fa.BlockCount < 1 {
		time.Sleep(time.Millisecond * 1)
	}
	tl.Logger().Debug("** spSRs request is blocked")

	// Database error cases
	srs = []*models.StorageRequest{
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID:      "prov-only",
					Version: 3,
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				CspDomainID:         "CSP-DOMAIN-1",
				PoolID:              "POOL-1",
				RequestedOperations: []string{"PROVISION"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "PROVISIONING",
				},
				StorageRequestCreateMutable: models.StorageRequestCreateMutable{
					StorageID: "STORAGE-1", // has storage - recovery case
				},
			},
		},
		&models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID:      "old-request",
					Version: 3,
				},
			},
		},
	}

	// csp domain load db error, test doneRequests behavior
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	mD.FetchParam.ID = "CSP-DOMAIN-1"
	dOps.EXPECT().CspDomainFetch(mD).Return(nil, fake.TransientErr).Times(1)
	tl.Logger().Debug("** Calling dispatchRequests: csp domain db error")
	tl.Flush()
	c.doneRequests = make(map[models.ObjID]models.ObjVersion)
	c.doneRequests["old-request"] = 4
	c.doneRequests["prov-only"] = 3
	cnt = c.dispatchRequests(nil, srs)
	assert.Equal(0, cnt)

	// storage load db error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainFetch(mD).Return(resD, nil).Times(1)
	sOps = mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(sOps).MinTimes(1)
	mS.FetchParam.ID = "STORAGE-1"
	sOps.EXPECT().StorageFetch(mS).Return(nil, fake.TransientErr).Times(1)
	tl.Logger().Debug("** Calling dispatchRequests: storage db error")
	tl.Flush()
	cnt = c.dispatchRequests(nil, srs)
	assert.Equal(0, cnt)

	// pool load db error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	dOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainFetch(mD).Return(resD, nil).Times(1)
	sOps = mockmgmtclient.NewMockStorageClient(mockCtrl)
	mAPI.EXPECT().Storage().Return(sOps).MinTimes(1)
	sOps.EXPECT().StorageFetch(mS).Return(resS, nil).Times(1)
	spOps = mockmgmtclient.NewMockPoolClient(mockCtrl)
	mAPI.EXPECT().Pool().Return(spOps).Times(1)
	mSP.FetchParam.ID = "POOL-1"
	spOps.EXPECT().PoolFetch(mSP).Return(nil, fake.TransientErr).Times(1)
	tl.Logger().Debug("** Calling dispatchRequests: pool db error")
	tl.Flush()
	cnt = c.dispatchRequests(nil, srs)
	assert.Equal(0, cnt)

	// log error messages - adjust if wording changes
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cntBeingProcessed := 0
	cntDone := 0
	cntSkippingDB := 0
	cntSkippingAgentdProcessing := 0
	cntAgentdIntended := 0
	tl.Iterate(func(i uint64, s string) {
		if m, err := regexp.MatchString("already being processed", s); err == nil && m {
			cntBeingProcessed++
		}
		if m, err := regexp.MatchString("processing is already done", s); err == nil && m {
			cntDone++
		}
		if m, err := regexp.MatchString("Skipping StorageRequest.*transient error", s); err == nil && m {
			cntSkippingDB++
		}
		if m, err := regexp.MatchString("Skipping StorageRequest.*processed by agentd", s); err == nil && m {
			cntSkippingAgentdProcessing++
		}
		if m, err := regexp.MatchString("intended-for-agentd: State change", s); err == nil && m {
			cntAgentdIntended++
		}
	})
	assert.Equal(idCount-errCount, cntBeingProcessed)
	assert.Equal(3, cntSkippingDB)
	assert.Equal(3, cntDone)
	assert.Equal(12, cntSkippingAgentdProcessing)
	assert.Equal(2, cntAgentdIntended)
	tl.Flush()
}

// See CUM-1939
func TestDispatchCUM1939Race(t *testing.T) {
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

	domObj := &models.CSPDomain{
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
	fc.RetLDObj = domObj
	storageObj := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta: &models.ObjMeta{
				ID:      "STORAGE-1",
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
				AttachmentState:    "ATTACHED", // Note: state relevant for FORMAT/USE cases
				ProvisionedState:   "PROVISIONED",
			},
		},
	}
	fc.StorageFetchRetObj = storageObj

	cbTime := time.Now().Add(time.Hour * 1)

	// set up the fake handler
	fa := newFakeRequestHandlers()
	fa.WaitForSignal = true
	c.requestHandlers = fa
	assert.Empty(c.activeRequests)
	assert.Empty(c.doneRequests)

	// @T0 the request is dispatched
	t.Log("@T0")
	srT0 := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:      "SR",
				Version: 1,
			},
		},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			CspDomainID:         "CSP-DOMAIN-1",
			PoolID:              "POOL-1",
			RequestedOperations: []string{"PROVISION", "ATTACH", "FORMAT", "USE"},
			CompleteByTime:      strfmt.DateTime(cbTime),
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
				StorageRequestState: "NEW",
			},
			StorageRequestCreateMutable: models.StorageRequestCreateMutable{
				StorageID: "STORAGE-1", // has storage - recovery case
			},
		},
	}
	fc.SRUpdaterClone = true // clone and bump version in updater
	assert.Equal(1, c.dispatchRequests(nil, []*models.StorageRequest{srT0}))
	assert.Equal(1, len(c.activeRequests))

	// wait until the request blocks in the fake handler
	for fa.BlockCount < 1 {
		time.Sleep(time.Millisecond * 1)
	}
	tl.Logger().Debug("** All requests are blocked")
	assert.Equal(fa.BlockCount, len(c.activeRequests))
	assert.Empty(c.doneRequests)
	assert.Equal("PROVISIONING", c.activeRequests["SR"].Request.StorageRequestState)
	tl.Flush()

	// advance to ATTACHING
	t.Log("@T0+")
	fa.SignalOnce()
	for fa.BlockCount < 2 {
		time.Sleep(time.Millisecond * 1)
	}
	assert.Equal("SR", fa.ProvIds[0])
	assert.Equal("ATTACHING", fc.SRUpdaterClonedObj.StorageRequestState)
	assert.EqualValues(3, fc.SRUpdaterClonedObj.Meta.Version)
	assert.Empty(fa.AttIds)
	assert.Equal(1, len(c.activeRequests))
	assert.Equal("ATTACHING", c.activeRequests["SR"].Request.StorageRequestState) // fails in cum-1939
	assert.EqualValues(3, c.activeRequests["SR"].Request.Meta.Version)            // fails in cum-1939

	// @T1 list is called again and will contain this version of the storage request
	var srT1 *models.StorageRequest
	testutils.Clone(fc.SRUpdaterClonedObj, &srT1)

	// @T2 thread dispatched at T0 completes
	t.Log("@T2")
	fa.Signal()
	for len(c.activeRequests) > 0 {
		time.Sleep(time.Millisecond * 1)
	}
	assert.Equal("SR", fa.AttIds[0])
	assert.Equal("FORMATTING", fc.SRUpdaterClonedObj.StorageRequestState)
	assert.EqualValues(4, fc.SRUpdaterClonedObj.Meta.Version)
	assert.NotEmpty(c.doneRequests)
	assert.EqualValues(4, c.doneRequests["SR"]) // fails in cum-1939

	// @T3 dispatch of T1 list done
	t.Log("@T3")
	assert.Equal(0, c.dispatchRequests(nil, []*models.StorageRequest{srT1})) // fails in cum-1939
	assert.Equal(0, len(c.activeRequests))                                   // fails in cum-1939
}

func TestRequestHandlerStateHelpers(t *testing.T) {
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

	c := &Component{}
	c.Init(app)

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
	chg = rhs.setRequestState("PROVISIONING")
	assert.True(chg)
	assert.Equal("PROVISIONING", o.StorageRequestState)
	assert.Len(o.RequestMessages, 2)
	assert.Regexp("NEW .* PROVISIONING", o.RequestMessages[1].Message)
	cntN2P := 0
	tl.Iterate(func(i uint64, s string) {
		if m, err := regexp.MatchString(string(o.Meta.ID)+".* NEW .* PROVISIONING", s); err == nil && m {
			cntN2P++
		}
	})
	assert.Equal(1, cntN2P)

	// test updating state (no change)
	assert.Len(o.RequestMessages, 2)
	assert.Equal("PROVISIONING", o.StorageRequestState)
	err := rhs.setAndUpdateRequestState(nil, "PROVISIONING")
	assert.Nil(err)
	assert.Len(o.RequestMessages, 2)
	assert.Equal("PROVISIONING", o.StorageRequestState)

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

	// success case, rhs object updated
	oM := make([]*models.TimestampedString, len(o.RequestMessages))
	copy(oM, o.RequestMessages)
	oState := o.StorageRequestState
	rhs.setRequestState("ATTACH") // create the state change message
	resSR := &storage_request.StorageRequestUpdateOK{
		Payload: &models.StorageRequest{
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
		},
	}
	o.RequestMessages = oM // restore previous state
	o.StorageRequestState = oState

	// update with nodeId change
	rhs.SetNodeID = true
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	srOps := mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).MinTimes(1)
	mSR := mockmgmtclient.NewStorageRequestMatcher(t, storage_request.NewStorageRequestUpdateParams().WithID(string(resSR.Payload.Meta.ID)))
	mSR.ZeroMsgTime = true
	mSR.UpdateParam.Payload = &resSR.Payload.StorageRequestMutable
	mSR.UpdateParam.Set = []string{"storageRequestState", "requestMessages", "storageId", "nodeId"}
	srOps.EXPECT().StorageRequestUpdate(mSR).Return(resSR, nil).MinTimes(1)
	err = rhs.setAndUpdateRequestState(nil, "ATTACH")
	assert.Nil(err)
	assert.Equal(resSR.Payload, rhs.Request)
	assert.NotEqual(resSR.Payload, o)
	assert.False(rhs.SetNodeID)

	// failure case when updating state - object not updated.
	rhs.Request = o
	rhs.DoNotSetStorageID = true
	assert.NotEqual("DETACH", o.StorageRequestState)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).MinTimes(1)
	mSRnoSP := mockmgmtclient.NewStorageRequestMatcher(t, storage_request.NewStorageRequestUpdateParams().WithID(string(o.Meta.ID)))
	mSRnoSP.ZeroMsgTime = true
	mSRnoSP.UpdateParam.Payload = &o.StorageRequestMutable
	mSRnoSP.UpdateParam.Set = []string{"storageRequestState", "requestMessages"}
	srOps.EXPECT().StorageRequestUpdate(mSRnoSP).Return(nil, fmt.Errorf("update error")).MinTimes(1)
	err = rhs.setAndUpdateRequestState(nil, "DETACH")
	assert.NotNil(err)
	assert.Regexp("update error", err.Error())
	assert.Equal("DETACH", o.StorageRequestState)

	// test setting message and updating state (use crud)
	// failure case - message set but object not updated
	fc := &fake.Client{}
	c.oCrud = fc
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
	rhs.DoNotSetStorageID = true
	fc.InSRUpdaterItems = nil
	fc.RetSRUpdatedObj = newO
	fc.RetSRUpdaterUpdateErr = fmt.Errorf("update error")
	msg, err = rhs.setAndUpdateRequestMessage(nil, "msg %s", "1")
	assert.Error(err)
	assert.Equal("msg 1", msg)
	assert.Equal(o, rhs.Request)
	assert.Equal(o.RequestMessages[0].Message, msg)
	assert.Equal([]string{"storageRequestState", "requestMessages"}, fc.InSRUpdaterItems.Set)
	// success case - message and object updated
	rhs.DoNotSetStorageID = false
	rhs.Request = o
	fc.InSRUpdaterItems = nil
	fc.RetSRUpdatedObj = newO
	fc.RetSRUpdaterUpdateErr = nil
	msg, err = rhs.setAndUpdateRequestMessageDistinct(nil, "msg %s", "2")
	assert.Nil(err)
	assert.Equal("msg 2", msg)
	assert.Equal(newO, rhs.Request)
	assert.Equal(o.RequestMessages[1].Message, msg)
	assert.Equal([]string{"storageRequestState", "requestMessages", "storageId"}, fc.InSRUpdaterItems.Set)
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
			assert.True(rhs.attemptProvision())
			assert.True(rhs.attemptAttach())
			assert.True(rhs.attemptDetach())
			assert.True(rhs.attemptRelease())
		case com.StgReqStateCapacityWait:
			assert.True(rhs.attemptProvision())
			assert.True(rhs.attemptAttach())
			assert.True(rhs.attemptDetach())
			assert.True(rhs.attemptRelease())
		case com.StgReqStateProvisioning:
			assert.True(rhs.attemptProvision())
			assert.True(rhs.attemptAttach())
			assert.True(rhs.attemptDetach())
			assert.True(rhs.attemptRelease())
		case com.StgReqStateAttaching:
			assert.False(rhs.attemptProvision())
			assert.True(rhs.attemptAttach())
			assert.True(rhs.attemptDetach())
			assert.True(rhs.attemptRelease())
		case com.StgReqStateDetaching:
			assert.False(rhs.attemptProvision())
			assert.False(rhs.attemptAttach())
			assert.True(rhs.attemptDetach())
			assert.True(rhs.attemptRelease())
		case com.StgReqStateReleasing:
			assert.False(rhs.attemptProvision())
			assert.False(rhs.attemptAttach())
			assert.False(rhs.attemptDetach())
			assert.True(rhs.attemptRelease())
		case com.StgReqStateSucceeded:
			assert.False(rhs.attemptProvision())
			assert.False(rhs.attemptAttach())
			assert.False(rhs.attemptDetach())
			assert.False(rhs.attemptRelease())
		case com.StgReqStateFailed:
			assert.False(rhs.attemptProvision())
			assert.False(rhs.attemptAttach())
			assert.False(rhs.attemptDetach())
			assert.False(rhs.attemptRelease())
		}
	}
}

func TestRequestAnimator(t *testing.T) {
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
	c := &Component{}
	c.Init(app)
	fa := newFakeRequestHandlers()
	c.requestHandlers = fa

	now := time.Now()
	cbTime := now.Add(time.Hour * 1)
	resSR := &storage_request.StorageRequestUpdateOK{
		Payload: &models.StorageRequest{
			StorageRequestAllOf0: models.StorageRequestAllOf0{
				Meta: &models.ObjMeta{
					ID:      "SR-1",
					Version: 1,
				},
			},
			StorageRequestCreateOnce: models.StorageRequestCreateOnce{
				CompleteByTime: strfmt.DateTime(cbTime),
				CspDomainID:    "CSP-DOMAIN-1",
			},
			StorageRequestMutable: models.StorageRequestMutable{
				StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
					StorageRequestState: "NEW",
				},
			},
		},
	}

	// Every primitive centrald operation, no failure
	// This is actually an invalid operation sequence but ok for UT
	tl.Logger().Info("case: all centrald ops, no failure")
	done := false
	rhs := &requestHandlerState{
		c:            c,
		Request:      resSR.Payload,
		HasProvision: true,
		HasAttach:    true,
		HasDetach:    true,
		HasRelease:   true,
		Done:         func() { done = true },
	}
	expTransitions := []string{"PROVISIONING", "ATTACHING", "DETACHING", "RELEASING"}
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	srOps := mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).Times(5)
	mNotNil := gomock.Not(gomock.Nil())
	srOps.EXPECT().StorageRequestUpdate(mNotNil).Return(resSR, nil).Times(5)
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal(expTransitions, fa.Transitions)
	assert.Equal("SUCCEEDED", rhs.Request.StorageRequestState)
	tl.Flush()

	// Every operation, retry on Attaching
	mockCtrl.Finish()
	tl.Logger().Info("case: retry on ATTACHING")
	fa = newFakeRequestHandlers()
	fa.RetryOnState = "ATTACHING"
	c.requestHandlers = fa
	rhs.Request.StorageRequestState = "NEW"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	expTransitions = []string{"PROVISIONING", "ATTACHING"}
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).Times(2)
	srOps.EXPECT().StorageRequestUpdate(mNotNil).Return(resSR, nil).Times(2)
	done = false
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal(expTransitions, fa.Transitions)
	assert.Equal("ATTACHING", rhs.Request.StorageRequestState)
	tl.Flush()

	// Provision, Attach, retry on Provision
	mockCtrl.Finish()
	tl.Logger().Info("case: provision, attach; retry on provision")
	rhs.HasProvision = true
	rhs.HasAttach = true
	rhs.HasDetach = false
	rhs.HasRelease = false
	rhs.InError = false
	rhs.RetryLater = false
	rhs.TimedOut = false
	fa = newFakeRequestHandlers()
	fa.RetryOnState = "PROVISIONING"
	c.requestHandlers = fa
	rhs.Request.StorageRequestState = "NEW"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	expTransitions = []string{"PROVISIONING"}
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).Times(1)
	srOps.EXPECT().StorageRequestUpdate(mNotNil).Return(resSR, nil).Times(1)
	done = false
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal(expTransitions, fa.Transitions)
	assert.Equal("PROVISIONING", rhs.Request.StorageRequestState)
	tl.Flush()

	// Provision, Attach, success with remove tag
	mockCtrl.Finish()
	tl.Logger().Info("case: provision, attach; retry on provision")
	rhs.HasProvision = true
	rhs.HasAttach = true
	rhs.HasDetach = false
	rhs.HasRelease = false
	rhs.InError = false
	rhs.RetryLater = false
	rhs.TimedOut = false
	fa = newFakeRequestHandlers()
	c.requestHandlers = fa
	rhs.Request.StorageRequestState = "NEW"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	expTransitions = []string{"PROVISIONING", "ATTACHING", "REMOVING_TAG"}
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).MinTimes(1)
	srOps.EXPECT().StorageRequestUpdate(mNotNil).Return(resSR, nil).MinTimes(1)
	done = false
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal(expTransitions, fa.Transitions)
	assert.Equal("SUCCEEDED", rhs.Request.StorageRequestState)
	tl.Flush()

	// Provision, Attach, fail on Provision
	mockCtrl.Finish()
	tl.Logger().Info("case: provision, attach; fail on provision")
	rhs.HasProvision = true
	rhs.HasAttach = true
	rhs.HasDetach = false
	rhs.HasRelease = false
	rhs.InError = false
	rhs.RetryLater = false
	rhs.TimedOut = false
	fa = newFakeRequestHandlers()
	fa.FailOnState = "PROVISIONING"
	c.requestHandlers = fa
	rhs.Request.StorageRequestState = "NEW"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	expTransitions = []string{"PROVISIONING", "RELEASING"}
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).Times(3)
	srOps.EXPECT().StorageRequestUpdate(mNotNil).Return(resSR, nil).Times(3)
	done = false
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal(expTransitions, fa.Transitions)
	assert.Equal("FAILED", rhs.Request.StorageRequestState)
	tl.Flush()

	// Detach, Release, fail on Releasing
	mockCtrl.Finish()
	tl.Logger().Info("case: NEW state, Detach, Release, fail on Releasing")
	rhs.HasProvision = false
	rhs.HasAttach = false
	rhs.HasDetach = true
	rhs.HasRelease = true
	rhs.InError = false
	rhs.RetryLater = false
	rhs.TimedOut = false
	fa = newFakeRequestHandlers()
	fa.FailOnState = "RELEASING"
	c.requestHandlers = fa
	rhs.Request.StorageRequestState = "NEW"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	expTransitions = []string{"DETACHING", "RELEASING", "UNDO_DETACHING"}
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).Times(4)
	srOps.EXPECT().StorageRequestUpdate(mNotNil).Return(resSR, nil).Times(4)
	done = false
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal(expTransitions, fa.Transitions)
	assert.Equal("FAILED", rhs.Request.StorageRequestState)
	tl.Flush()

	mockCtrl.Finish()
	tl.Logger().Info("case: RELEASING state, Detach, Release, fail on Releasing")
	rhs.HasProvision = false
	rhs.HasAttach = false
	rhs.HasDetach = true
	rhs.HasRelease = true
	rhs.InError = false
	rhs.RetryLater = false
	rhs.TimedOut = false
	fa = newFakeRequestHandlers()
	fa.FailOnState = "RELEASING"
	c.requestHandlers = fa
	rhs.Request.StorageRequestState = "RELEASING"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	expTransitions = []string{"RELEASING", "UNDO_DETACHING"}
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).Times(2)
	srOps.EXPECT().StorageRequestUpdate(mNotNil).Return(resSR, nil).Times(2)
	done = false
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal(expTransitions, fa.Transitions)
	assert.Equal("FAILED", rhs.Request.StorageRequestState)
	tl.Flush()

	mockCtrl.Finish()
	tl.Logger().Info("case: DETACHING state, Attach, Detach, InError on entry")
	rhs.HasProvision = false
	rhs.HasAttach = true
	rhs.HasDetach = true
	rhs.HasRelease = true
	rhs.InError = true
	rhs.RetryLater = false
	rhs.TimedOut = false
	fa = newFakeRequestHandlers()
	c.requestHandlers = fa
	rhs.Request.StorageRequestState = "DETACHING"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	expTransitions = []string{"UNDO_DETACHING", "DETACHING"}
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).Times(3)
	srOps.EXPECT().StorageRequestUpdate(mNotNil).Return(resSR, nil).Times(3)
	done = false
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal(expTransitions, fa.Transitions)
	assert.Equal("FAILED", rhs.Request.StorageRequestState)
	tl.Flush()

	mockCtrl.Finish()
	tl.Logger().Info("case: SR update failure during undo, RetryLater")
	rhs.HasProvision = false
	rhs.HasAttach = false
	rhs.HasDetach = true
	rhs.HasRelease = true
	rhs.InError = true
	rhs.RetryLater = false
	rhs.TimedOut = false
	fa = newFakeRequestHandlers()
	c.requestHandlers = fa
	rhs.Request.StorageRequestState = "RELEASING"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	expTransitions = []string{}
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps)
	srOps.EXPECT().StorageRequestUpdate(mNotNil).Return(nil, fmt.Errorf("db error"))
	done = false
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal(expTransitions, fa.Transitions)
	assert.Equal("UNDO_DETACHING", rhs.Request.StorageRequestState)
	assert.True(rhs.RetryLater)
	tl.Flush()

	mockCtrl.Finish()
	tl.Logger().Info("case: non-transient SR update failure during undo, Fail")
	rhs.HasProvision = false
	rhs.HasAttach = false
	rhs.HasDetach = true
	rhs.HasRelease = true
	rhs.InError = true
	rhs.RetryLater = false
	rhs.TimedOut = false
	fa = newFakeRequestHandlers()
	c.requestHandlers = fa
	rhs.Request.StorageRequestState = "RELEASING"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	expTransitions = []string{}
	expErr := &crud.Error{Payload: models.Error{Code: http.StatusConflict, Message: swag.String("permanent")}}
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).Times(2)
	prev := srOps.EXPECT().StorageRequestUpdate(mNotNil).Return(nil, expErr)
	srOps.EXPECT().StorageRequestUpdate(mNotNil).Return(resSR, nil).After(prev)
	done = false
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal(expTransitions, fa.Transitions)
	assert.Equal("FAILED", rhs.Request.StorageRequestState)
	assert.True(rhs.InError)
	assert.False(rhs.RetryLater)
	tl.Flush()

	// Fail on state transition, enter with capacity wait
	mockCtrl.Finish()
	tl.Logger().Info("case: enter with CAPACITY_WAIT, err on state change")
	rhs.HasProvision = true
	rhs.HasAttach = true
	rhs.HasDetach = true
	rhs.HasRelease = true
	rhs.InError = false
	rhs.RetryLater = false
	rhs.TimedOut = false
	fa = newFakeRequestHandlers()
	c.requestHandlers = fa
	rhs.Request.StorageRequestState = "CAPACITY_WAIT"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	expTransitions = []string{"PROVISIONING"}
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).Times(1)
	srOps.EXPECT().StorageRequestUpdate(mNotNil).Return(nil, fmt.Errorf("fail update")).Times(1)
	done = false
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal(expTransitions, fa.Transitions)
	assert.Equal("ATTACHING", rhs.Request.StorageRequestState)
	assert.True(rhs.RetryLater)
	tl.Flush()

	// Non-transient failure on state transition, enter with capacity wait
	mockCtrl.Finish()
	tl.Logger().Info("case: enter with CAPACITY_WAIT, err on state change")
	rhs.HasProvision = true
	rhs.HasAttach = true
	rhs.HasDetach = true
	rhs.HasRelease = true
	rhs.InError = false
	rhs.RetryLater = false
	rhs.TimedOut = false
	fa = newFakeRequestHandlers()
	c.requestHandlers = fa
	rhs.Request.StorageRequestState = "CAPACITY_WAIT"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	expTransitions = []string{"PROVISIONING", "RELEASING"}
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).Times(3)
	prev = srOps.EXPECT().StorageRequestUpdate(mNotNil).Return(nil, expErr)
	srOps.EXPECT().StorageRequestUpdate(mNotNil).Return(resSR, nil).After(prev).Times(2)
	done = false
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal(expTransitions, fa.Transitions)
	assert.Equal("FAILED", rhs.Request.StorageRequestState)
	assert.True(rhs.InError)
	assert.False(rhs.RetryLater)
	tl.Flush()

	// Timed out
	mockCtrl.Finish()
	tl.Logger().Info("case: enter with CAPACITY_WAIT, TimedOut")
	rhs.HasProvision = true
	rhs.HasAttach = true
	rhs.HasDetach = false
	rhs.HasRelease = false
	rhs.InError = false
	rhs.RetryLater = false
	rhs.TimedOut = false
	fa = newFakeRequestHandlers()
	c.requestHandlers = fa
	rhs.Request.StorageRequestState = "CAPACITY_WAIT"
	rhs.Request.CompleteByTime = strfmt.DateTime(now) // is in the past at this point
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	expTransitions = []string{"RELEASING"}
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).Times(2)
	srOps.EXPECT().StorageRequestUpdate(mNotNil).Return(resSR, nil).Times(2)
	done = false
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal(expTransitions, fa.Transitions)
	assert.Equal("FAILED", rhs.Request.StorageRequestState)
	assert.True(rhs.TimedOut)
	if assert.Len(rhs.Request.RequestMessages, 3) {
		assert.Regexp("Timed out", rhs.Request.RequestMessages[0].Message)
		assert.Regexp("CAPACITY_WAIT.*PROVISIONING", rhs.Request.RequestMessages[1].Message)
		assert.Regexp("PROVISIONING.*FAILED", rhs.Request.RequestMessages[2].Message)
	}
	tl.Flush()

	// test processing of request error when CSPDomain not set
	mockCtrl.Finish()
	tl.Logger().Info("case: CSPDomain loading error")
	rhs.HasProvision = true
	rhs.HasAttach = false
	rhs.HasDetach = false
	rhs.HasRelease = false
	rhs.InError = true
	rhs.RetryLater = false
	rhs.TimedOut = false
	rhs.Request.Meta.ID = "SR-CSPDomain-LOAD-ERROR"
	rhs.Request.StorageRequestState = "NEW" // anything but error
	rhs.Request.CompleteByTime = strfmt.DateTime(cbTime)
	rhs.Storage = nil
	rhs.CSPDomain = nil
	rhs.Pool = nil
	fa = newFakeRequestHandlers()
	c.requestHandlers = fa
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).Times(2)
	srOps.EXPECT().StorageRequestUpdate(mNotNil).Return(resSR, nil).Times(2)
	app.AppCSP = nil
	done = false
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal("FAILED", rhs.Request.StorageRequestState)
	tl.Flush()

	// test processing of request error when Storage not set
	mockCtrl.Finish()
	tl.Logger().Info("case: Storage loading error")
	rhs.HasProvision = true
	rhs.HasAttach = false
	rhs.HasDetach = false
	rhs.HasRelease = false
	rhs.InError = true
	rhs.RetryLater = false
	rhs.TimedOut = false
	rhs.Request.Meta.ID = "SR-STORAGE-LOAD-ERROR"
	rhs.Request.StorageRequestState = "NEW" // anything but error
	rhs.Request.CompleteByTime = strfmt.DateTime(cbTime)
	rhs.Storage = nil
	rhs.CSPDomain = &models.CSPDomain{}
	rhs.Pool = nil
	fa = newFakeRequestHandlers()
	c.requestHandlers = fa
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).Times(2)
	srOps.EXPECT().StorageRequestUpdate(mNotNil).Return(resSR, nil).Times(2)
	app.AppCSP = nil
	done = false
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal("FAILED", rhs.Request.StorageRequestState)
	tl.Flush()

	// test mix of centrald ops + format + use: ends with format
	done = false
	rhs = &requestHandlerState{
		c:            c,
		Request:      resSR.Payload,
		HasProvision: true,
		HasAttach:    true,
		HasFormat:    true,
		HasUse:       true,
		Done:         func() { done = true },
	}
	rhs.Request.Meta.ID = "SR-PROVISION-ATTACH-FORMAT-USE"
	rhs.Request.StorageRequestState = "NEW"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	fa = newFakeRequestHandlers()
	c.requestHandlers = fa
	expTransitions = []string{"PROVISIONING", "ATTACHING"}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).Times(3)
	mNotNil = gomock.Not(gomock.Nil())
	srOps.EXPECT().StorageRequestUpdate(mNotNil).Return(resSR, nil).Times(3)
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal(expTransitions, fa.Transitions)
	assert.Equal("FORMATTING", rhs.Request.StorageRequestState)
	assert.True(rhs.RetryLater)
	tl.Flush()

	// test same mix, executes only REMOVING_TAG
	done = false
	rhs = &requestHandlerState{
		c:            c,
		Request:      resSR.Payload,
		HasProvision: true,
		HasAttach:    true,
		HasFormat:    true,
		HasUse:       true,
		Done:         func() { done = true },
	}
	rhs.Request.StorageRequestState = "REMOVING_TAG"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	fa = newFakeRequestHandlers()
	c.requestHandlers = fa
	expTransitions = []string{"REMOVING_TAG"}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).MinTimes(1)
	mNotNil = gomock.Not(gomock.Nil())
	srOps.EXPECT().StorageRequestUpdate(mNotNil).Return(resSR, nil).MinTimes(1)
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal(expTransitions, fa.Transitions)
	assert.Equal("SUCCEEDED", rhs.Request.StorageRequestState)
	assert.False(rhs.RetryLater)
	tl.Flush()

	// test mix of centrald ops + use
	done = false
	rhs = &requestHandlerState{
		c:            c,
		Request:      resSR.Payload,
		HasProvision: false,
		HasAttach:    true,
		HasFormat:    false,
		HasUse:       true,
		Done:         func() { done = true },
	}
	rhs.Request.Meta.ID = "SR-ATTACH-USE"
	rhs.Request.StorageRequestState = "NEW"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	fa = newFakeRequestHandlers()
	c.requestHandlers = fa
	expTransitions = []string{"ATTACHING"}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).Times(2)
	mNotNil = gomock.Not(gomock.Nil())
	srOps.EXPECT().StorageRequestUpdate(mNotNil).Return(resSR, nil).Times(2)
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal(expTransitions, fa.Transitions)
	assert.Equal("USING", rhs.Request.StorageRequestState)
	assert.True(rhs.RetryLater)
	tl.Flush()

	// test mix of centrald ops + close
	done = false
	rhs = &requestHandlerState{
		c:          c,
		Request:    resSR.Payload,
		HasDetach:  true,
		HasRelease: true,
		HasClose:   true,
		Done:       func() { done = true },
	}
	rhs.Request.Meta.ID = "SR-CLOSE-DETACH-RELEASE"
	rhs.Request.StorageRequestState = "NEW"
	rhs.Request.RequestMessages = make([]*models.TimestampedString, 0)
	fa = newFakeRequestHandlers()
	c.requestHandlers = fa
	expTransitions = []string{}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	c.oCrud = crud.NewClient(mAPI, app.Log)
	srOps = mockmgmtclient.NewMockStorageRequestClient(mockCtrl)
	mAPI.EXPECT().StorageRequest().Return(srOps).Times(1)
	mNotNil = gomock.Not(gomock.Nil())
	srOps.EXPECT().StorageRequestUpdate(mNotNil).Return(resSR, nil).Times(1)
	c.requestAnimator(nil, rhs)
	assert.True(done)
	assert.Equal(expTransitions, fa.Transitions)
	assert.Equal("CLOSING", rhs.Request.StorageRequestState)
	assert.True(rhs.RetryLater)
	tl.Flush()
}
