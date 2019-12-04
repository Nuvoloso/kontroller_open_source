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
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/op/go-logging"
	"github.com/stretchr/testify/assert"
)

type fakeRequestHandlers struct {
	t                     *testing.T
	log                   *logging.Logger
	Count                 int
	CtxCount              int
	NoIDCtxCount          int
	BadCtxCount           int
	BlockCount            int
	Transitions           []string
	RetryOnState          string
	FailOnStates          []string
	FailOnIDState         map[string]string // id -> state
	CancelOnState         string
	AbortOnState          string
	PanicOnState          string
	WaitForSignal         bool
	ACIds                 []string
	AttachFsIds           []string
	BindIds               []string
	CfgIds                []string
	ChangeCapacityIds     []string
	ChooseNodeIds         []string
	CreateIds             []string
	DelSPAIds             []string
	ExpIds                []string
	PlaceIds              []string
	PlaceReattachIds      []string
	PublishIds            []string
	PublishServicePlanIds []string
	ReallocCacheIds       []string
	RenameIds             []string
	ResizeCacheIds        []string
	SizeIds               []string
	UndoACIds             []string
	UndoAttachFsIds       []string
	UndoBindIds           []string
	UndoCfgIds            []string
	UndoChangeCapacityIds []string
	UndoCreateFSnapIds    []string
	UndoCreateIds         []string
	UndoExpIds            []string
	UndoPlaceIds          []string
	UndoPublishIds        []string
	UndoReallocCacheIds   []string
	UndoRenameIds         []string
	UndoResizeCacheIds    []string
	UndoSizeIds           []string
	UndoVolSnapRestoreIds []string
	CgSnapCreateIds       map[string][]string // state -> arrays of ids
	NodeDeleteIds         map[string][]string // state -> arrays of ids
	VolDetachIds          map[string][]string // state -> arrays of ids
	VolSnapCreateIds      map[string][]string // state -> arrays of ids
	CreateFSnapIds        map[string][]string // state -> arrays of ids
	VolSnapRestoreIds     map[string][]string // state -> arrays of ids
	signalled             bool
	vsIDIncorrect         bool
	spaIDIncorrect        bool
	mux                   sync.Mutex
	cond                  *sync.Cond
}

var _ = AllocateCapacityHandlers(&fakeRequestHandlers{})
var _ = AttachFsHandlers(&fakeRequestHandlers{})
var _ = CreateHandlers(&fakeRequestHandlers{})
var _ = CreateFromSnapshotHandlers(&fakeRequestHandlers{})
var _ = BindHandlers(&fakeRequestHandlers{})
var _ = ChangeCapacityHandlers(&fakeRequestHandlers{})
var _ = PublishHandlers(&fakeRequestHandlers{})
var _ = PublishServicePlanHandlers(&fakeRequestHandlers{})
var _ = AllocationHandlers(&fakeRequestHandlers{})
var _ = MountHandlers(&fakeRequestHandlers{})
var _ = NodeDeleteHandlers(&fakeRequestHandlers{})
var _ = RenameHandlers(&fakeRequestHandlers{})
var _ = CGSnapshotCreateHandlers(&fakeRequestHandlers{})
var _ = VolumeDetachHandlers(&fakeRequestHandlers{})
var _ = VolSnapshotCreateHandlers(&fakeRequestHandlers{})
var _ = VolSnapshotRestoreHandlers(&fakeRequestHandlers{})

func newFakeRequestHandlers(t *testing.T, log *logging.Logger) *fakeRequestHandlers {
	fh := &fakeRequestHandlers{}
	fh.t = t
	fh.log = log
	fh.Transitions = make([]string, 0)
	fh.ACIds = make([]string, 0)
	fh.AttachFsIds = make([]string, 0)
	fh.BindIds = make([]string, 0)
	fh.CfgIds = make([]string, 0)
	fh.ChangeCapacityIds = make([]string, 0)
	fh.ChooseNodeIds = make([]string, 0)
	fh.CreateIds = make([]string, 0)
	fh.DelSPAIds = make([]string, 0)
	fh.ExpIds = make([]string, 0)
	fh.PlaceIds = make([]string, 0)
	fh.PlaceReattachIds = make([]string, 0)
	fh.PublishIds = make([]string, 0)
	fh.PublishServicePlanIds = make([]string, 0)
	fh.ReallocCacheIds = make([]string, 0)
	fh.RenameIds = make([]string, 0)
	fh.ResizeCacheIds = make([]string, 0)
	fh.SizeIds = make([]string, 0)
	fh.UndoACIds = make([]string, 0)
	fh.UndoAttachFsIds = make([]string, 0)
	fh.UndoBindIds = make([]string, 0)
	fh.UndoCfgIds = make([]string, 0)
	fh.UndoChangeCapacityIds = make([]string, 0)
	fh.UndoCreateFSnapIds = make([]string, 0)
	fh.UndoCreateIds = make([]string, 0)
	fh.UndoExpIds = make([]string, 0)
	fh.UndoPlaceIds = make([]string, 0)
	fh.UndoPublishIds = make([]string, 0)
	fh.UndoReallocCacheIds = make([]string, 0)
	fh.UndoRenameIds = make([]string, 0)
	fh.UndoResizeCacheIds = make([]string, 0)
	fh.UndoSizeIds = make([]string, 0)
	fh.CgSnapCreateIds = make(map[string][]string, 0)
	fh.NodeDeleteIds = make(map[string][]string, 0)
	fh.VolDetachIds = make(map[string][]string, 0)
	fh.VolSnapCreateIds = make(map[string][]string, 0)
	fh.CreateFSnapIds = make(map[string][]string, 0)
	fh.VolSnapRestoreIds = make(map[string][]string, 0)
	fh.UndoVolSnapRestoreIds = make([]string, 0)
	fh.FailOnIDState = make(map[string]string)
	fh.cond = sync.NewCond(&fh.mux)
	return fh
}

func (fh *fakeRequestHandlers) common(ctx context.Context, rhs *RequestHandlerState, thisHandler string) {
	fh.mux.Lock()
	if fh.WaitForSignal {
		for !fh.signalled {
			fh.BlockCount++
			fh.cond.Wait()
		}
	}
	fh.Count++
	if ctx == nil {
		fh.BadCtxCount++
	} else if cnv := ctx.Value(mgmtclient.AuthIdentityKey{}); cnv == nil {
		fh.NoIDCtxCount++
	} else if identity, ok := cnv.(*models.Identity); !ok {
		//fh.BadCtxCount++
	} else if identity != rhs.Request.Creator {
		//fh.BadCtxCount++
	} else {
		fh.CtxCount++
	}
	fh.Transitions = append(fh.Transitions, thisHandler)
	fh.log.Infof("** fh: %s %s %s", rhs.Request.Meta.ID, thisHandler, rhs.Request.VolumeSeriesRequestState)
	switch thisHandler { // Use State names
	case "ALLOCATING_CAPACITY":
		fh.ACIds = append(fh.ACIds, string(rhs.Request.Meta.ID))
	case "ATTACHING_FS":
		fh.AttachFsIds = append(fh.AttachFsIds, string(rhs.Request.Meta.ID))
	case "BINDING":
		fh.BindIds = append(fh.BindIds, string(rhs.Request.Meta.ID))
	case "CG_SNAPSHOT_CREATE":
		fh.visitedCgSnapCreateState(rhs)
	case "CHANGING_CAPACITY":
		fh.ChangeCapacityIds = append(fh.ChangeCapacityIds, string(rhs.Request.Meta.ID))
	case "CHOOSING_NODE":
		fh.ChooseNodeIds = append(fh.ChooseNodeIds, string(rhs.Request.Meta.ID))
	case "CREATING":
		fh.CreateIds = append(fh.CreateIds, string(rhs.Request.Meta.ID))
	case "CREATING_FROM_SNAPSHOT":
		fh.visitedCreatingFromSnapshotState(rhs)
	case "DELETING_SPA":
		fh.DelSPAIds = append(fh.DelSPAIds, string(rhs.Request.Meta.ID))
	case "NODE_DELETE":
		fh.visitedNodeDeleteState(rhs)
	case "PLACEMENT":
		fh.PlaceIds = append(fh.PlaceIds, string(rhs.Request.Meta.ID))
	case "PLACEMENT_REATTACH":
		fh.PlaceReattachIds = append(fh.PlaceReattachIds, string(rhs.Request.Meta.ID))
	case "PUBLISHING":
		fh.PublishIds = append(fh.PublishIds, string(rhs.Request.Meta.ID))
	case "PUBLISHING_SERVICE_PLAN":
		fh.PublishServicePlanIds = append(fh.PublishServicePlanIds, string(rhs.Request.Meta.ID))
	case "REALLOCATING_CACHE":
		fh.ReallocCacheIds = append(fh.ReallocCacheIds, string(rhs.Request.Meta.ID))
	case "RENAMING":
		fh.RenameIds = append(fh.RenameIds, string(rhs.Request.Meta.ID))
	case "RESIZING_CACHE":
		fh.ResizeCacheIds = append(fh.ResizeCacheIds, string(rhs.Request.Meta.ID))
	case "SIZING":
		fh.SizeIds = append(fh.SizeIds, string(rhs.Request.Meta.ID))
	case "VOL_DETACH":
		fh.visitedVolDetachState(rhs)
	case "VOL_SNAPSHOT_CREATE":
		fh.visitedVolSnapCreateState(rhs)
	case "VOL_SNAPSHOT_RESTORE":
		fh.visitedVolSnapRestoreState(rhs)
	case "VOLUME_CONFIG":
		fh.CfgIds = append(fh.CfgIds, string(rhs.Request.Meta.ID))
	case "VOLUME_EXPORT":
		fh.ExpIds = append(fh.ExpIds, string(rhs.Request.Meta.ID))
	case "UNDO_ALLOCATING_CAPACITY":
		fh.UndoACIds = append(fh.UndoACIds, string(rhs.Request.Meta.ID))
	case "UNDO_ATTACHING_FS":
		fh.UndoAttachFsIds = append(fh.UndoAttachFsIds, string(rhs.Request.Meta.ID))
	case "UNDO_BINDING":
		fh.UndoBindIds = append(fh.UndoBindIds, string(rhs.Request.Meta.ID))
	case "UNDO_CG_SNAPSHOT_CREATE":
		fh.visitedCgSnapCreateState(rhs)
	case "UNDO_CHANGING_CAPACITY":
		fh.UndoChangeCapacityIds = append(fh.UndoChangeCapacityIds, string(rhs.Request.Meta.ID))
	case "UNDO_CREATING":
		fh.UndoCreateIds = append(fh.UndoCreateIds, string(rhs.Request.Meta.ID))
	case "UNDO_CREATING_FROM_SNAPSHOT":
		fh.UndoCreateFSnapIds = append(fh.UndoCreateFSnapIds, string(rhs.Request.Meta.ID))
	case "UNDO_PLACEMENT":
		fh.UndoPlaceIds = append(fh.UndoPlaceIds, string(rhs.Request.Meta.ID))
	case "UNDO_PUBLISHING":
		fh.UndoPublishIds = append(fh.UndoPublishIds, string(rhs.Request.Meta.ID))
	case "UNDO_REALLOCATING_CACHE":
		fh.UndoReallocCacheIds = append(fh.UndoReallocCacheIds, string(rhs.Request.Meta.ID))
	case "UNDO_RENAMING":
		fh.UndoRenameIds = append(fh.UndoRenameIds, string(rhs.Request.Meta.ID))
	case "UNDO_RESIZING_CACHE":
		fh.UndoResizeCacheIds = append(fh.UndoResizeCacheIds, string(rhs.Request.Meta.ID))
	case "UNDO_SIZING":
		fh.UndoSizeIds = append(fh.UndoSizeIds, string(rhs.Request.Meta.ID))
	case "UNDO_VOL_SNAPSHOT_CREATE":
		fh.visitedVolSnapCreateState(rhs)
	case "UNDO_VOL_SNAPSHOT_RESTORE":
		fh.UndoVolSnapRestoreIds = append(fh.UndoVolSnapRestoreIds, string(rhs.Request.Meta.ID))
	case "UNDO_VOLUME_EXPORT":
		fh.UndoExpIds = append(fh.UndoExpIds, string(rhs.Request.Meta.ID))
	case "UNDO_VOLUME_CONFIG":
		fh.UndoCfgIds = append(fh.UndoCfgIds, string(rhs.Request.Meta.ID))
	default:
		panic(fmt.Sprintf("Not handling %s", thisHandler))
	}
	if rhs.Request.ServicePlanAllocationID != rhs.originalSpaID {
		fh.spaIDIncorrect = true
	}
	if rhs.Request.VolumeSeriesID != rhs.originalVsID {
		fh.vsIDIncorrect = true
	}
	if fh.RetryOnState == rhs.Request.VolumeSeriesRequestState {
		fh.log.Infof("** fh: %s setting RetryLater IN %s", rhs.Request.Meta.ID, rhs.Request.VolumeSeriesRequestState)
		rhs.RetryLater = true
	}
	if util.Contains(fh.FailOnStates, rhs.Request.VolumeSeriesRequestState) {
		fh.log.Infof("** fh: %s setting InError IN %s", rhs.Request.Meta.ID, rhs.Request.VolumeSeriesRequestState)
		rhs.InError = true
	}
	if state, exists := fh.FailOnIDState[string(rhs.Request.Meta.ID)]; exists && state == rhs.Request.VolumeSeriesRequestState {
		fh.log.Infof("** fh: %s setting InError IN %s", rhs.Request.Meta.ID, rhs.Request.VolumeSeriesRequestState)
		rhs.InError = true
	}
	if fh.CancelOnState == rhs.Request.VolumeSeriesRequestState {
		fh.log.Infof("** fh: %s setting Canceling IN %s", rhs.Request.Meta.ID, rhs.Request.VolumeSeriesRequestState)
		rhs.Canceling = true
	}
	if fh.AbortOnState == rhs.Request.VolumeSeriesRequestState {
		fh.log.Infof("** fh: %s setting AbortUndo IN %s", rhs.Request.Meta.ID, rhs.Request.VolumeSeriesRequestState)
		rhs.AbortUndo = true
	}
	if fh.PanicOnState == rhs.Request.VolumeSeriesRequestState {
		fh.log.Infof("** fh: %s panicking IN %s", rhs.Request.Meta.ID, rhs.Request.VolumeSeriesRequestState)
		panic(rhs.Request.VolumeSeriesRequestState)
	}
	fh.mux.Unlock()
}

func (fh *fakeRequestHandlers) visitedCgSnapCreateState(rhs *RequestHandlerState) {
	state := rhs.Request.VolumeSeriesRequestState
	if _, exists := fh.CgSnapCreateIds[state]; !exists {
		fh.CgSnapCreateIds[state] = []string{}
	}
	l := fh.CgSnapCreateIds[state]
	l = append(l, string(rhs.Request.Meta.ID))
	fh.CgSnapCreateIds[state] = l
}

func (fh *fakeRequestHandlers) visitedNodeDeleteState(rhs *RequestHandlerState) {
	state := rhs.Request.VolumeSeriesRequestState
	if _, exists := fh.NodeDeleteIds[state]; !exists {
		fh.NodeDeleteIds[state] = []string{}
	}
	l := fh.NodeDeleteIds[state]
	l = append(l, string(rhs.Request.Meta.ID))
	fh.NodeDeleteIds[state] = l
}

func (fh *fakeRequestHandlers) visitedVolDetachState(rhs *RequestHandlerState) {
	state := rhs.Request.VolumeSeriesRequestState
	if _, exists := fh.VolDetachIds[state]; !exists {
		fh.VolDetachIds[state] = []string{}
	}
	l := fh.VolDetachIds[state]
	l = append(l, string(rhs.Request.Meta.ID))
	fh.VolDetachIds[state] = l
}

func (fh *fakeRequestHandlers) visitedVolSnapCreateState(rhs *RequestHandlerState) {
	state := rhs.Request.VolumeSeriesRequestState
	if _, exists := fh.VolSnapCreateIds[state]; !exists {
		fh.VolSnapCreateIds[state] = []string{}
	}
	l := fh.VolSnapCreateIds[state]
	l = append(l, string(rhs.Request.Meta.ID))
	fh.VolSnapCreateIds[state] = l
}

func (fh *fakeRequestHandlers) visitedCreatingFromSnapshotState(rhs *RequestHandlerState) {
	state := rhs.Request.VolumeSeriesRequestState
	if _, exists := fh.CreateFSnapIds[state]; !exists {
		fh.CreateFSnapIds[state] = []string{}
	}
	l := fh.CreateFSnapIds[state]
	l = append(l, string(rhs.Request.Meta.ID))
	fh.CreateFSnapIds[state] = l
}

func (fh *fakeRequestHandlers) visitedVolSnapRestoreState(rhs *RequestHandlerState) {
	state := rhs.Request.VolumeSeriesRequestState
	if _, exists := fh.VolSnapRestoreIds[state]; !exists {
		fh.VolSnapRestoreIds[state] = []string{}
	}
	l := fh.VolSnapRestoreIds[state]
	l = append(l, string(rhs.Request.Meta.ID))
	fh.VolSnapRestoreIds[state] = l
}

func (fh *fakeRequestHandlers) Signal() {
	fh.mux.Lock()
	fh.signalled = true
	fh.cond.Broadcast()
	fh.mux.Unlock()
}

func (fh *fakeRequestHandlers) AllocateCapacity(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "ALLOCATING_CAPACITY")
}

func (fh *fakeRequestHandlers) AttachFs(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "ATTACHING_FS")
}

func (fh *fakeRequestHandlers) Bind(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "BINDING")
}

func (fh *fakeRequestHandlers) CGSnapshotCreate(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "CG_SNAPSHOT_CREATE")
}

func (fh *fakeRequestHandlers) ChangeCapacity(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "CHANGING_CAPACITY")
}

func (fh *fakeRequestHandlers) ChooseNode(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "CHOOSING_NODE")
}

func (fh *fakeRequestHandlers) Create(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "CREATING")
}

func (fh *fakeRequestHandlers) CreateFromSnapshot(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "CREATING_FROM_SNAPSHOT")
}

func (fh *fakeRequestHandlers) Configure(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "VOLUME_CONFIG")
}

func (fh *fakeRequestHandlers) Export(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "VOLUME_EXPORT")
}

func (fh *fakeRequestHandlers) NodeDelete(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "NODE_DELETE")
}

func (fh *fakeRequestHandlers) Place(ctx context.Context, rhs *RequestHandlerState) {
	if rhs.HasDelete || rhs.HasUnbind {
		fh.common(ctx, rhs, "PLACEMENT_REATTACH")
	} else {
		fh.common(ctx, rhs, "PLACEMENT")
	}
}

func (fh *fakeRequestHandlers) Publish(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "PUBLISHING")
}

func (fh *fakeRequestHandlers) PublishServicePlan(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "PUBLISHING_SERVICE_PLAN")
}

func (fh *fakeRequestHandlers) ReallocateCache(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "REALLOCATING_CACHE")
}

func (fh *fakeRequestHandlers) Rename(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "RENAMING")
}

func (fh *fakeRequestHandlers) ResizeCache(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "RESIZING_CACHE")
}

func (fh *fakeRequestHandlers) Size(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "SIZING")
}

func (fh *fakeRequestHandlers) VolDetach(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "VOL_DETACH")
}

func (fh *fakeRequestHandlers) VolSnapshotCreate(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "VOL_SNAPSHOT_CREATE")
}

func (fh *fakeRequestHandlers) VolSnapshotRestore(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "VOL_SNAPSHOT_RESTORE")
}

func (fh *fakeRequestHandlers) UndoAllocateCapacity(ctx context.Context, rhs *RequestHandlerState) {
	if rhs.HasDeleteSPA {
		fh.common(ctx, rhs, "DELETING_SPA")
	} else {
		fh.common(ctx, rhs, "UNDO_ALLOCATING_CAPACITY")
	}
}

func (fh *fakeRequestHandlers) UndoAttachFs(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "UNDO_ATTACHING_FS")
}

func (fh *fakeRequestHandlers) UndoBind(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "UNDO_BINDING")
}

func (fh *fakeRequestHandlers) UndoCGSnapshotCreate(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "UNDO_CG_SNAPSHOT_CREATE")
}

func (fh *fakeRequestHandlers) UndoChangeCapacity(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "UNDO_CHANGING_CAPACITY")
}

func (fh *fakeRequestHandlers) UndoConfigure(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "UNDO_VOLUME_CONFIG")
}

func (fh *fakeRequestHandlers) UndoCreate(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "UNDO_CREATING")
}

func (fh *fakeRequestHandlers) UndoCreateFromSnapshot(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "UNDO_CREATING_FROM_SNAPSHOT")
}

func (fh *fakeRequestHandlers) UndoExport(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "UNDO_VOLUME_EXPORT")
}

func (fh *fakeRequestHandlers) UndoPlace(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "UNDO_PLACEMENT")
}

func (fh *fakeRequestHandlers) UndoPublish(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "UNDO_PUBLISHING")
}

func (fh *fakeRequestHandlers) UndoReallocateCache(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "UNDO_REALLOCATING_CACHE")
}

func (fh *fakeRequestHandlers) UndoRename(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "UNDO_RENAMING")
}

func (fh *fakeRequestHandlers) UndoResizeCache(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "UNDO_RESIZING_CACHE")
}

func (fh *fakeRequestHandlers) UndoSize(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "UNDO_SIZING")
}

func (fh *fakeRequestHandlers) UndoVolSnapshotCreate(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "UNDO_VOL_SNAPSHOT_CREATE")
}

func (fh *fakeRequestHandlers) UndoVolSnapshotRestore(ctx context.Context, rhs *RequestHandlerState) {
	fh.common(ctx, rhs, "UNDO_VOL_SNAPSHOT_RESTORE")
}

// a custom matcher that only matches on the VS ID, and only returns true/false, no asserts
type mockVolumeSeriesFetchMatcher struct {
	id string
}

func newVolumeSeriesFetchMatcher(id models.ObjID) *mockVolumeSeriesFetchMatcher {
	return &mockVolumeSeriesFetchMatcher{id: string(id)}
}

// Matches is from gomock.Matcher
func (o *mockVolumeSeriesFetchMatcher) Matches(x interface{}) bool {
	p, ok := x.(*volume_series.VolumeSeriesFetchParams)
	return ok && p.ID == o.id
}

// String is from gomock.Matcher
func (o *mockVolumeSeriesFetchMatcher) String() string {
	return "fetch.ID matches " + o.id
}

// a custom matcher that only matches on the VSR ID, and only returns true/false, no asserts
type mockVolumeSeriesRequestUpdateMatcher struct {
	id string
}

func newVolumeSeriesRequestUpdateMatcher(id models.ObjID) *mockVolumeSeriesRequestUpdateMatcher {
	return &mockVolumeSeriesRequestUpdateMatcher{id: string(id)}
}

// Matches is from gomock.Matcher
func (o *mockVolumeSeriesRequestUpdateMatcher) Matches(x interface{}) bool {
	p, ok := x.(*volume_series_request.VolumeSeriesRequestUpdateParams)
	return ok && p.ID == o.id
}

// String is from gomock.Matcher
func (o *mockVolumeSeriesRequestUpdateMatcher) String() string {
	return "update.ID matches " + o.id
}

func TestDispatchRequests(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	resV := &volume_series.VolumeSeriesFetchOK{
		Payload: &models.VolumeSeries{
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
					BoundClusterID: "cluster1",
					CacheAllocations: map[string]models.CacheAllocation{
						"node1": models.CacheAllocation{
							RequestedSizeBytes: swag.Int64(1),
						},
					},
					ConfiguredNodeID: "node1",
					Mounts: []*models.Mount{
						&models.Mount{SnapIdentifier: "HEAD", MountedNodeID: "node1", MountState: "MOUNTED"},
					},
					StorageParcels: map[string]models.ParcelAllocation{
						"st1": models.ParcelAllocation{SizeBytes: swag.Int64(1)},
					},
					VolumeSeriesState: com.VolStateProvisioned,
				},
				VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
					SizeBytes:         swag.Int64(10737418240),
					ClusterDescriptor: models.ClusterDescriptor{"key": models.ValueType{Kind: "STRING", Value: "value"}},
				},
			},
		},
	}
	resV2 := &volume_series.VolumeSeriesFetchOK{
		Payload: &models.VolumeSeries{
			VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
				Meta: &models.ObjMeta{
					ID:      "vs2",
					Version: 1,
				},
				RootParcelUUID: "uuid",
			},
			VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
				AccountID: "account1",
			},
			VolumeSeriesMutable: models.VolumeSeriesMutable{
				VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
					BoundClusterID:    "cluster1",
					VolumeSeriesState: com.VolStateBound,
				},
				VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
					SizeBytes: swag.Int64(10737418240),
				},
			},
		},
	}
	resV3 := &volume_series.VolumeSeriesFetchOK{
		Payload: &models.VolumeSeries{
			VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
				Meta: &models.ObjMeta{
					ID:      "vs3",
					Version: 1,
				},
				RootParcelUUID: "uuid",
			},
			VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
				AccountID: "account1",
			},
			VolumeSeriesMutable: models.VolumeSeriesMutable{
				VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
					BoundClusterID: "cluster1",
					RootStorageID:  "st1",
					StorageParcels: map[string]models.ParcelAllocation{
						"st1": models.ParcelAllocation{SizeBytes: swag.Int64(1)},
					},
					VolumeSeriesState: com.VolStateDeleting,
				},
				VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
					SizeBytes:         swag.Int64(10737418240),
					ClusterDescriptor: models.ClusterDescriptor{"key": models.ValueType{Kind: "STRING", Value: "value"}},
				},
			},
		},
	}
	resV4 := &volume_series.VolumeSeriesFetchOK{
		Payload: &models.VolumeSeries{
			VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
				Meta: &models.ObjMeta{
					ID:      "vs4",
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
		},
	}

	cbTime := time.Now().Add(time.Hour * 1)
	vrs := []*models.VolumeSeriesRequest{
		&models.VolumeSeriesRequest{ // [0]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "create-only",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"CREATE"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "CREATING",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs1",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [1]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "bind-only",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"BIND"},
				CompleteByTime:      strfmt.DateTime(cbTime),
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
		},
		&models.VolumeSeriesRequest{ // [2]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "bind-only-in-capacity-wait",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"BIND"},
				CompleteByTime:      strfmt.DateTime(cbTime),
				ClusterID:           "cluster1",
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "CAPACITY_WAIT",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs1",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [3]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "mount-only",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"MOUNT"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "SIZING",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					NodeID:         "node1",
					VolumeSeriesID: "vs1",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [4]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "mount-only-in-storage-wait",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"MOUNT"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "STORAGE_WAIT",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					NodeID:         "node1",
					VolumeSeriesID: "vs1",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [5]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "delete-configured",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"DELETE"},
				CompleteByTime:      strfmt.DateTime(cbTime),
				ClusterID:           "cluster-1",
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					NodeID:         "node-1",
					VolumeSeriesID: "vs3",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [6]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "delete-provisioned",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"DELETE"},
				CompleteByTime:      strfmt.DateTime(cbTime),
				ClusterID:           "cluster-1",
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs1",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [7]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "delete-unbound",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"DELETE"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs4",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [8]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "unmount-only",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"UNMOUNT"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "UNDO_VOLUME_EXPORT",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs1",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [9]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "rename-only",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"RENAME"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "RENAMING",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs1",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [10]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "error-because-fail-on-volume-series-load",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"BIND"},
				CompleteByTime:      strfmt.DateTime(cbTime),
				ClusterID:           "cluster1",
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "BINDING",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs-FAIL",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [11]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "allocate-capacity-with-undo",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations:             []string{"ALLOCATE_CAPACITY"},
				CompleteByTime:                  strfmt.DateTime(cbTime),
				ServicePlanAllocationCreateSpec: &models.ServicePlanAllocationCreateArgs{},
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [12]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "allocate-capacity-undo-only",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations:             []string{"ALLOCATE_CAPACITY"},
				CompleteByTime:                  strfmt.DateTime(cbTime),
				ServicePlanAllocationCreateSpec: &models.ServicePlanAllocationCreateArgs{},
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "UNDO_ALLOCATING_CAPACITY",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [13]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "cg-snapshot-no-undo",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"CG_SNAPSHOT_CREATE"},
				CompleteByTime:      strfmt.DateTime(cbTime),
				ClusterID:           "cl-1",
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					ConsistencyGroupID: "cg-1",
				},
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [14]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "cg-snapshot-with-undo",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"CG_SNAPSHOT_CREATE"},
				CompleteByTime:      strfmt.DateTime(cbTime),
				ClusterID:           "cl-1",
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					ConsistencyGroupID: "cg-1",
				},
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [15]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "cg-snapshot-undo-only",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"CG_SNAPSHOT_CREATE"},
				CompleteByTime:      strfmt.DateTime(cbTime),
				ClusterID:           "cl-1",
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					ConsistencyGroupID: "cg-1",
				},
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "UNDO_CG_SNAPSHOT_VOLUMES",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [16]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "vol-snapshot-no-undo",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"VOL_SNAPSHOT_CREATE"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs1",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [17]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "vol-snapshot-with-undo",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"VOL_SNAPSHOT_CREATE"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs1",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [18]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "vol-snapshot-undo-only",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"VOL_SNAPSHOT_CREATE"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "UNDO_SNAPSHOT_UPLOAD_DONE",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs1",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [19]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "create-from-snap-only",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"CREATE_FROM_SNAPSHOT"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs1",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [20]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "undo-create-from-snap",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"CREATE_FROM_SNAPSHOT"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "UNDO_CREATING_FROM_SNAPSHOT",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs1",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [21]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "vol-snapshot-restore",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"CREATE", "BIND", "MOUNT", "VOL_SNAPSHOT_RESTORE"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs1",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [22]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "undo-vol-snapshot-restore",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"CREATE", "BIND", "MOUNT", "VOL_SNAPSHOT_RESTORE"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "UNDO_SNAPSHOT_RESTORE",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs1",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [23]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "delete-spa",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"DELETE_SPA"},
				CompleteByTime:      strfmt.DateTime(cbTime),
				Creator: &models.Identity{
					AccountID: "aid1",
				},
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					ServicePlanAllocationID: "spa1",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [24]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "delete-bound",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"DELETE"},
				CompleteByTime:      strfmt.DateTime(cbTime),
				ClusterID:           "cluster-1",
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs2",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [25]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "configure-no-undo",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"CONFIGURE"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs2",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [26]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "configure-undo-only",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"CONFIGURE"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "UNDO_VOLUME_CONFIG",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs2",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [27]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "change-capacity-with-undo",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"CHANGE_CAPACITY"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs2",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [28]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "change-capacity-undo-only",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"CHANGE_CAPACITY"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "UNDO_CHANGING_CAPACITY",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs2",
				},
			},
		},

		&models.VolumeSeriesRequest{ // [29]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "change-capacity-with-wait",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"CHANGE_CAPACITY"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "CAPACITY_WAIT",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs2",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [30]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "publish-with-undo",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"PUBLISH"},
				CompleteByTime:      strfmt.DateTime(cbTime),
				ClusterID:           "cluster1",
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs2",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [31]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "publish-undo-only",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"PUBLISH"},
				CompleteByTime:      strfmt.DateTime(cbTime),
				ClusterID:           "cluster1",
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "UNDO_PUBLISHING",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs2",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [32]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "unpublish-only",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"UNPUBLISH"},
				CompleteByTime:      strfmt.DateTime(cbTime),
				ClusterID:           "cluster1",
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs2",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [33]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "attach-fs-with-undo",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"ATTACH_FS"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs2",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [34]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "attach-fs-undo-only",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"ATTACH_FS"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "UNDO_ATTACHING_FS",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs2",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [35]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "detach-fs",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"DETACH_FS"},
				CompleteByTime:      strfmt.DateTime(cbTime),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs2",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [36]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "unpublish-delete-configured",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"UNPUBLISH", "DELETE"}, // UNPUBLISH is redundant but allowed, only one unpublish state
				CompleteByTime:      strfmt.DateTime(cbTime),
				ClusterID:           "cluster-1",
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					NodeID:         "node-1",
					VolumeSeriesID: "vs3",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [37]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "change-capacity-without-cache",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"CHANGE_CAPACITY"},
				CompleteByTime:      strfmt.DateTime(cbTime),
				ClusterID:           "cluster-1",
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					NodeID:         "node-1",
					VolumeSeriesID: "vs3",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [38]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "change-capacity-with-cache",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"CHANGE_CAPACITY"},
				CompleteByTime:      strfmt.DateTime(cbTime),
				ClusterID:           "cluster-1",
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					NodeID:         "node-1",
					VolumeSeriesID: "vs1",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [39]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "change-capacity-undo-cache",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"CHANGE_CAPACITY"},
				CompleteByTime:      strfmt.DateTime(cbTime),
				ClusterID:           "cluster-1",
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "UNDO_REALLOCATING_CACHE",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					NodeID:         "node-1",
					VolumeSeriesID: "vs1",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [40]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "unbind-new",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"UNBIND"},
				CompleteByTime:      strfmt.DateTime(cbTime),
				ClusterID:           "cluster-1",
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					NodeID:         "node-1",
					VolumeSeriesID: "vs3",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [41] this case tickles undoOnly()
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "unbind-continued",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"UNBIND"},
				CompleteByTime:      strfmt.DateTime(cbTime),
				ClusterID:           "cluster-1",
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "UNDO_BINDING",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					NodeID:         "node-1",
					VolumeSeriesID: "vs3",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [42] note: ATTACHING_FS fails so undo path traversed
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "vol-restore-on-mount",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"MOUNT", "VOL_SNAPSHOT_RESTORE", "ATTACH_FS"},
				CompleteByTime:      strfmt.DateTime(cbTime),
				ClusterID:           "cluster-1",
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					NodeID:         "node-1",
					VolumeSeriesID: "vs2",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [43]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "node-delete",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"NODE_DELETE"},
				CompleteByTime:      strfmt.DateTime(cbTime),
				ClusterID:           "cluster-1",
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					NodeID: "node-1",
				},
			},
		},
		&models.VolumeSeriesRequest{ // [44]
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "vol-detach",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"VOL_DETACH"},
				CompleteByTime:      strfmt.DateTime(cbTime),
				ClusterID:           "cluster-1",
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					NodeID:         "node-1",
					VolumeSeriesID: "vs2",
				},
			},
		},
	}

	expACIds := []string{
		"allocate-capacity-with-undo",
	}
	expAttachFsIds := []string{
		"attach-fs-with-undo",
		"vol-restore-on-mount",
	}
	expBindIds := []string{ // sorted
		"bind-only",
		"bind-only-in-capacity-wait",
		"vol-snapshot-restore",
	}
	expCfgIds := []string{ //sorted
		"configure-no-undo",
		"mount-only",
		"mount-only-in-storage-wait",
		"vol-restore-on-mount",
		"vol-snapshot-restore",
	}
	expCGssIds := []string{
		"cg-snapshot-no-undo",
		"cg-snapshot-with-undo",
	}
	expChgCapIds := []string{ // sorted
		"change-capacity-with-cache",
		"change-capacity-with-undo",
		"change-capacity-with-wait",
		"change-capacity-without-cache",
	}
	expChooseNodeIds := []string{
		"delete-configured",
		"unbind-new",
		"unpublish-delete-configured",
	}
	expCreateIds := []string{ // sorted
		"create-only",
		"vol-snapshot-restore",
	}
	expCreateFSnapIds := []string{ // sorted
		"create-from-snap-only",
	}
	expExpIds := []string{ //sorted
		"mount-only",
		"mount-only-in-storage-wait",
		"vol-restore-on-mount",
		"vol-snapshot-restore",
	}
	expDelSPAIds := []string{
		"delete-spa",
	}
	expNDssIds := []string{ // sorted
		"node-delete",
	}
	expPlaceIds := []string{ // sorted
		"configure-no-undo",
		"mount-only",
		"mount-only-in-storage-wait",
		"vol-restore-on-mount",
		"vol-snapshot-restore",
	}
	expPlaceReattachIds := []string{
		"delete-configured",
		"unbind-new",
		"unpublish-delete-configured",
	}
	expPSPIds := []string{
		"allocate-capacity-with-undo",
	}
	expPublishIds := []string{ // sorted
		"publish-with-undo",
	}
	expPublishServicePlanIds := []string{ // sorted
		"allocate-capacity-with-undo",
	}
	expReallocCacheIds := []string{ // sorted
		"change-capacity-with-cache",
	}
	expRenameIds := []string{
		"rename-only",
	}
	expResizeCacheIds := []string{ // sorted
		"change-capacity-with-cache",
	}
	expSizeIds := []string{
		"configure-no-undo",
		"mount-only",
		"vol-restore-on-mount",
		"vol-snapshot-restore",
	}
	expVDssIds := []string{ // sorted
		"vol-detach",
	}
	expVolSnapRestoreIds := []string{ // sorted
		"vol-restore-on-mount",
		"vol-snapshot-restore",
	}
	expVSCIds := []string{
		"vol-snapshot-no-undo",
		"vol-snapshot-with-undo",
	}
	expUndoACIds := []string{ // sorted
		"allocate-capacity-undo-only",
		"allocate-capacity-with-undo",
	}
	expUndoAttachFsIds := []string{
		"attach-fs-undo-only",
		"attach-fs-with-undo",
		"detach-fs",
		"vol-restore-on-mount",
	}
	expUndoBindIds := []string{ // sorted
		"delete-bound",
		"delete-configured",
		"delete-provisioned",
		"delete-unbound",
		"unbind-continued",
		"unbind-new",
		"undo-vol-snapshot-restore",
		"unpublish-delete-configured",
	}
	expUndoCGssIds := []string{ // sorted
		"cg-snapshot-undo-only",
	}
	expUndoChgCapIds := []string{ // sorted
		"change-capacity-undo-cache",
		"change-capacity-undo-only",
		"change-capacity-with-undo",
		"change-capacity-with-wait",
	}
	expUndoConfigIds := []string{
		"configure-undo-only",
		"delete-configured",
		"unbind-new",
		"undo-vol-snapshot-restore",
		"unpublish-delete-configured",
		"vol-restore-on-mount",
	}
	expUndoCreateIds := []string{ // sorted
		"delete-bound",
		"delete-configured",
		"delete-provisioned",
		"delete-unbound",
		"undo-vol-snapshot-restore",
		"unpublish-delete-configured",
	}
	expUndoCreateFSnapIds := []string{ // sorted
		"undo-create-from-snap",
	}
	expUndoExportIds := []string{
		"undo-vol-snapshot-restore",
		"unmount-only",
		"vol-restore-on-mount",
	}
	expUndoPlaceIds := []string{ // sorted
		"delete-configured",
		"delete-provisioned",
		"unbind-new",
		"undo-vol-snapshot-restore",
		"unpublish-delete-configured",
		"vol-restore-on-mount",
	}
	expUndoPublishIds := []string{ // sorted
		"delete-configured",
		"delete-provisioned",
		"publish-undo-only",
		"publish-with-undo",
		"unbind-new",
		"unpublish-delete-configured",
		"unpublish-only",
	}
	expUndoReallocCacheIds := []string{ // sorted
		"change-capacity-undo-cache",
	}
	expUndoRenameIds := []string{
		"rename-only",
	}
	expUndoResizeCacheIds := []string{ // sorted
		"change-capacity-undo-cache",
	}
	expUndoSizeIds := []string{ // sorted
		"delete-configured",
		"delete-provisioned",
		"unbind-new",
		"undo-vol-snapshot-restore",
		"unpublish-delete-configured",
		"vol-restore-on-mount",
	}
	expUndoVolSnapRestoreIds := []string{ // sorted
		"undo-vol-snapshot-restore",
		"vol-restore-on-mount",
	}
	expUndoVSCIds := []string{ // sorted
		"vol-snapshot-undo-only",
		"vol-snapshot-with-undo",
	}
	expErrIds := []string{
		"error-because-fail-on-volume-series-load",
	}
	expUndoBindIdsWithErr := append(expUndoBindIds, expErrIds...)
	sort.Strings(expUndoBindIdsWithErr)
	errCount := len(expErrIds)
	uniqueIds := map[string]struct{}{} // for fake handler
	for _, x := range expACIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expAttachFsIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expBindIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expCfgIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expCGssIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expChgCapIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expChooseNodeIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expCreateIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expCreateFSnapIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expDelSPAIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expNDssIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expPlaceIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expPlaceReattachIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expPublishIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expPSPIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expReallocCacheIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expRenameIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expResizeCacheIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expVDssIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expVolSnapRestoreIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expVSCIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expUndoACIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expUndoAttachFsIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expUndoBindIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expUndoCGssIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expUndoChgCapIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expUndoConfigIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expUndoCreateIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expUndoCreateFSnapIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expUndoExportIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expUndoPlaceIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expUndoPublishIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expUndoReallocCacheIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expUndoResizeCacheIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expUndoSizeIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expUndoVolSnapRestoreIds {
		uniqueIds[x] = struct{}{}
	}
	for _, x := range expUndoVSCIds {
		uniqueIds[x] = struct{}{}
	}
	// note: exactly 1 error case at [10] even though we keep an errCount
	idCount := len(uniqueIds) // does not count the error cases
	firstDispatchCount := idCount + errCount
	vs1Count := 0
	vs2Count := 0
	vs3Count := 0
	vs4Count := 0
	vsErrIdx := 0
	for i, vr := range vrs {
		switch vr.VolumeSeriesID {
		case "vs1":
			vs1Count++
		case "vs2":
			vs2Count++
		case "vs3":
			vs3Count++
		case "vs4":
			vs4Count++
		case "vs-FAIL":
			vsErrIdx = i
		}
	}
	tl.Logger().Debugf("** VolumeSeriesRequest #tcs: %d", len(vrs))
	tl.Logger().Debugf("** VolumeSeriesRequest #unique ids: %d", len(uniqueIds))
	tl.Logger().Debugf("** VolumeSeriesRequest idCount: %d", idCount)
	tl.Logger().Debugf("** VolumeSeriesRequest errCount: %d", errCount)
	tl.Logger().Debugf("** VolumeSeriesRequest opCount: %d", firstDispatchCount)
	tl.Logger().Debugf("** VolumeSeriesRequest vs1Count: %d", vs1Count)
	tl.Logger().Debugf("** VolumeSeriesRequest vs2Count: %d", vs2Count)
	tl.Logger().Debugf("** VolumeSeriesRequest vs3Count: %d", vs3Count)
	tl.Logger().Debugf("** VolumeSeriesRequest vs4Count: %d", vs4Count)
	tl.Logger().Debugf("** VolumeSeriesRequest vsErrIdx: %d", vsErrIdx)
	tl.Flush()

	fh := newFakeRequestHandlers(t, tl.Logger())
	fh.WaitForSignal = true
	fh.FailOnStates = []string{"RENAMING", "PUBLISHING_SERVICE_PLAN", "PUBLISHING", "ATTACHING_FS"}
	fh.FailOnIDState["change-capacity-with-undo"] = "CHANGING_CAPACITY"
	fh.FailOnIDState["change-capacity-with-wait"] = "CHANGING_CAPACITY"
	fh.FailOnIDState["cg-snapshot-with-undo"] = "CG_SNAPSHOT_WAIT"
	fh.FailOnIDState["vol-snapshot-with-undo"] = "SNAPSHOT_UPLOAD_DONE"
	ops := &fakeOps{}
	ctx := context.Background()

	// mix of success and non-database errors
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	a := NewAnimator(0, tl.Logger(), ops)
	assert.Empty(a.activeRequests)
	assert.Empty(a.doneRequests)
	a.allocateCapacityHandlers = fh
	a.allocationHandlers = fh
	a.attachFsHandlers = fh
	a.bindHandlers = fh
	a.cgSnapshotCreateHandlers = fh
	a.changeCapacityHandlers = fh
	a.createFromSnapshotHandlers = fh
	a.createHandlers = fh
	a.mountHandlers = fh
	a.nodeDeleteHandlers = fh
	a.publishHandlers = fh
	a.publishServicePlanHandlers = fh
	a.renameHandlers = fh
	a.volDetachHandlers = fh
	a.volSnapshotCreateHandlers = fh
	a.volSnapshotRestoreHandlers = fh
	a.OCrud = crud.NewClient(mAPI, a.Log)
	vOps := mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vOps).MinTimes(1)
	vOps.EXPECT().VolumeSeriesFetch(newVolumeSeriesFetchMatcher(resV.Payload.Meta.ID)).Return(resV, nil).Times(vs1Count)
	vOps.EXPECT().VolumeSeriesFetch(newVolumeSeriesFetchMatcher(resV2.Payload.Meta.ID)).Return(resV2, nil).Times(vs2Count)
	vOps.EXPECT().VolumeSeriesFetch(newVolumeSeriesFetchMatcher(resV3.Payload.Meta.ID)).Return(resV3, nil).Times(vs3Count)
	vOps.EXPECT().VolumeSeriesFetch(newVolumeSeriesFetchMatcher(resV4.Payload.Meta.ID)).Return(resV4, nil).Times(vs4Count)
	vOps.EXPECT().VolumeSeriesFetch(newVolumeSeriesFetchMatcher("vs-FAIL")).Return(nil, fmt.Errorf("volume series load error")).Times(errCount)

	vrOps := mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(vrOps).MinTimes(1)
	for _, vr := range vrs {
		if vr.VolumeSeriesRequestState == "NEW" || vr.VolumeSeriesID == "vs-FAIL" {
			resVR := &volume_series_request.VolumeSeriesRequestUpdateOK{Payload: vr}
			vrOps.EXPECT().VolumeSeriesRequestUpdate(newVolumeSeriesRequestUpdateMatcher(vr.Meta.ID)).Return(resVR, nil)
		}
	}
	tl.Logger().Debug("** calling dispatchRequests")
	tl.Flush()
	cnt := a.DispatchRequests(ctx, vrs)
	tl.Flush()
	assert.Equal(firstDispatchCount, cnt)
	tl.Logger().Debug("** Waiting for handlers to be dispatched")
	tl.Flush()

	// wait until all requests block in the fake handlers
	for fh.BlockCount < idCount+errCount {
		time.Sleep(time.Millisecond * 1)
	}
	tl.Logger().Debugf("** All requests are blocked: %d", len(a.activeRequests))
	assert.Equal(fh.BlockCount, len(a.activeRequests))
	assert.Empty(a.doneRequests)
	tl.Flush()

	// reissue the non-error requests - they should all be ignored
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	vrs2 := []*models.VolumeSeriesRequest{}
	for _, vr := range vrs {
		if util.Contains(expErrIds, string(vr.Meta.ID)) {
			continue
		}
		vrs2 = append(vrs2, vr)
	}
	assert.Len(vrs2, len(vrs)-errCount)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	a.OCrud = crud.NewClient(mAPI, a.Log)
	tl.Logger().Debug("** Calling dispatchRequests again on non-error requests")
	tl.Flush()
	cnt = a.DispatchRequests(ctx, vrs2)
	assert.Zero(cnt)
	assert.Empty(a.doneRequests)
	tl.Flush()

	// wake up handlers and wait for completion
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	a.OCrud = crud.NewClient(mAPI, a.Log)
	vrOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(vrOps).MinTimes(1)
	for _, vr := range vrs {
		resVR := &volume_series_request.VolumeSeriesRequestUpdateOK{Payload: vr}
		vrOps.EXPECT().VolumeSeriesRequestUpdate(newVolumeSeriesRequestUpdateMatcher(vr.Meta.ID)).Return(resVR, nil).MinTimes(1)
	}
	fh.Signal()
	for len(a.activeRequests) > 0 {
		time.Sleep(time.Millisecond * 1)
	}
	assert.Len(a.doneRequests, idCount+errCount)
	assert.Equal(148+errCount, fh.Count)      // mount:7, unmount:1, delete:21, rename:3, allocate_capacity:5, delete_spa:1, cg-snapshot:8, vol-snapshot:25, create-from-snapshot:3, vol-snapshot-restore:28, configure:4, change_capacity:12, publish:8, attach-fs:4, unbind:8, node-delete:7, vol-detach:3
	assert.Equal(fh.Count-1, fh.NoIDCtxCount) // delete-spa specifies a creator
	assert.Equal(1, fh.CtxCount)
	assert.Zero(fh.BadCtxCount)
	sort.Strings(fh.ACIds)
	sort.Strings(fh.AttachFsIds)
	sort.Strings(fh.BindIds)
	sort.Strings(fh.CfgIds)
	sort.Strings(fh.ChangeCapacityIds)
	sort.Strings(fh.ChooseNodeIds)
	sort.Strings(fh.CreateIds)
	sort.Strings(fh.DelSPAIds)
	sort.Strings(fh.ExpIds)
	sort.Strings(fh.PlaceIds)
	sort.Strings(fh.PlaceReattachIds)
	sort.Strings(fh.PublishIds)
	sort.Strings(fh.PublishServicePlanIds)
	sort.Strings(fh.ReallocCacheIds)
	sort.Strings(fh.RenameIds)
	sort.Strings(fh.ResizeCacheIds)
	sort.Strings(fh.SizeIds)
	sort.Strings(fh.UndoACIds)
	sort.Strings(fh.UndoAttachFsIds)
	sort.Strings(fh.UndoBindIds)
	sort.Strings(fh.UndoCfgIds)
	sort.Strings(fh.UndoChangeCapacityIds)
	sort.Strings(fh.UndoCreateFSnapIds)
	sort.Strings(fh.UndoCreateIds)
	sort.Strings(fh.UndoExpIds)
	sort.Strings(fh.UndoPlaceIds)
	sort.Strings(fh.UndoPublishIds)
	sort.Strings(fh.UndoReallocCacheIds)
	sort.Strings(fh.UndoRenameIds)
	sort.Strings(fh.UndoResizeCacheIds)
	sort.Strings(fh.UndoSizeIds)
	sort.Strings(fh.UndoVolSnapRestoreIds)
	assert.Equal(expACIds, fh.ACIds)
	assert.Equal(expAttachFsIds, fh.AttachFsIds)
	assert.Equal(expBindIds, fh.BindIds)
	assert.Equal(expCfgIds, fh.CfgIds)
	assert.Equal(expChgCapIds, fh.ChangeCapacityIds)
	assert.Equal(expChooseNodeIds, fh.ChooseNodeIds)
	assert.Equal(expCreateIds, fh.CreateIds)
	assert.Equal(expDelSPAIds, fh.DelSPAIds)
	assert.Equal(expExpIds, fh.ExpIds)
	assert.Equal(expPlaceIds, fh.PlaceIds)
	assert.Equal(expPlaceReattachIds, fh.PlaceReattachIds)
	assert.Equal(expPublishIds, fh.PublishIds)
	assert.Equal(expPublishServicePlanIds, fh.PublishServicePlanIds)
	assert.Equal(expReallocCacheIds, fh.ReallocCacheIds)
	assert.Equal(expRenameIds, fh.RenameIds)
	assert.Equal(expResizeCacheIds, fh.ResizeCacheIds)
	assert.Equal(expSizeIds, fh.SizeIds)
	assert.Equal(expUndoACIds, fh.UndoACIds)
	assert.Equal(expUndoAttachFsIds, fh.UndoAttachFsIds)
	assert.Equal(expUndoBindIdsWithErr, fh.UndoBindIds) // the one error added here
	assert.Equal(expUndoChgCapIds, fh.UndoChangeCapacityIds)
	assert.Equal(expUndoConfigIds, fh.UndoCfgIds)
	assert.Equal(expUndoCreateFSnapIds, fh.UndoCreateFSnapIds)
	assert.Equal(expUndoCreateIds, fh.UndoCreateIds)
	assert.Equal(expUndoExportIds, fh.UndoExpIds)
	assert.Equal(expUndoPlaceIds, fh.UndoPlaceIds)
	assert.Equal(expUndoPublishIds, fh.UndoPublishIds)
	assert.Equal(expUndoReallocCacheIds, fh.UndoReallocCacheIds)
	assert.Equal(expUndoRenameIds, fh.UndoRenameIds)
	assert.Equal(expUndoResizeCacheIds, fh.UndoResizeCacheIds)
	assert.Equal(expUndoSizeIds, fh.UndoSizeIds)
	assert.Equal(expUndoVolSnapRestoreIds, fh.UndoVolSnapRestoreIds)
	assert.False(fh.spaIDIncorrect)
	assert.False(fh.vsIDIncorrect)
	// cg snapshot create data in map of state ⇒ ids
	expCgSCIds := map[string][]string{
		"CG_SNAPSHOT_VOLUMES":      []string{"cg-snapshot-no-undo", "cg-snapshot-with-undo"},
		"CG_SNAPSHOT_WAIT":         []string{"cg-snapshot-no-undo", "cg-snapshot-with-undo"},
		"CG_SNAPSHOT_FINALIZE":     []string{"cg-snapshot-no-undo"},
		"UNDO_CG_SNAPSHOT_VOLUMES": []string{"cg-snapshot-undo-only", "cg-snapshot-with-undo"},
	}
	for _, v := range fh.CgSnapCreateIds {
		sort.Strings(v)
	}
	assert.Equal(expCgSCIds, fh.CgSnapCreateIds)
	// node delete data in map of state ⇒ ids
	expNodeDeleteIds := map[string][]string{
		"DRAINING_REQUESTS":  []string{"node-delete"},
		"CANCELING_REQUESTS": []string{"node-delete"},
		"DETACHING_VOLUMES":  []string{"node-delete"},
		"DETACHING_STORAGE":  []string{"node-delete"},
		"VOLUME_DETACH_WAIT": []string{"node-delete"},
		"VOLUME_DETACHED":    []string{"node-delete"},
		"DELETING_NODE":      []string{"node-delete"},
	}
	for _, v := range fh.NodeDeleteIds {
		sort.Strings(v)
	}
	assert.Equal(expNodeDeleteIds, fh.NodeDeleteIds)
	// vol detach data in map of state ⇒ ids
	expVolDetachIds := map[string][]string{
		"VOLUME_DETACH_WAIT": []string{"vol-detach"},
		"VOLUME_DETACHING":   []string{"vol-detach"},
		"VOLUME_DETACHED":    []string{"vol-detach"},
	}
	for _, v := range fh.VolDetachIds {
		sort.Strings(v)
	}
	assert.Equal(expVolDetachIds, fh.VolDetachIds)
	// vol snapshot create data in map of state ⇒ ids
	expVolSCIds := map[string][]string{
		"PAUSING_IO":                []string{"vol-snapshot-no-undo", "vol-snapshot-with-undo"},
		"PAUSED_IO":                 []string{"vol-snapshot-no-undo", "vol-snapshot-with-undo"},
		"CREATING_PIT":              []string{"vol-snapshot-no-undo", "vol-snapshot-with-undo"},
		"CREATED_PIT":               []string{"vol-snapshot-no-undo", "vol-snapshot-with-undo"},
		"SNAPSHOT_UPLOADING":        []string{"vol-snapshot-no-undo", "vol-snapshot-with-undo"},
		"SNAPSHOT_UPLOAD_DONE":      []string{"vol-snapshot-no-undo", "vol-snapshot-with-undo"},
		"FINALIZING_SNAPSHOT":       []string{"vol-snapshot-no-undo"},
		"UNDO_PAUSING_IO":           []string{"vol-snapshot-undo-only", "vol-snapshot-with-undo"},
		"UNDO_PAUSED_IO":            []string{"vol-snapshot-undo-only", "vol-snapshot-with-undo"},
		"UNDO_CREATING_PIT":         []string{"vol-snapshot-undo-only", "vol-snapshot-with-undo"},
		"UNDO_CREATED_PIT":          []string{"vol-snapshot-undo-only", "vol-snapshot-with-undo"},
		"UNDO_SNAPSHOT_UPLOADING":   []string{"vol-snapshot-undo-only", "vol-snapshot-with-undo"},
		"UNDO_SNAPSHOT_UPLOAD_DONE": []string{"vol-snapshot-undo-only", "vol-snapshot-with-undo"},
	}
	for _, v := range fh.VolSnapCreateIds {
		sort.Strings(v)
	}
	assert.Equal(expVolSCIds, fh.VolSnapCreateIds)
	t.Log(fh.VolSnapCreateIds)
	tl.Flush()
	// vol snapshot restore data in map of state ⇒ ids
	expVolSRIds := map[string][]string{
		"SNAPSHOT_RESTORE":          []string{"vol-restore-on-mount", "vol-snapshot-restore"},
		"SNAPSHOT_RESTORE_FINALIZE": []string{"vol-restore-on-mount", "vol-snapshot-restore"},
	}
	for _, v := range fh.VolSnapRestoreIds {
		sort.Strings(v)
	}
	assert.Equal(expVolSRIds, fh.VolSnapRestoreIds)
	t.Log(fh.VolSnapRestoreIds)
	tl.Flush()
	// create from snapshot data in map of state ⇒ ids
	expCFSIds := map[string][]string{
		"CREATING_FROM_SNAPSHOT": []string{"create-from-snap-only"},
		"SNAPSHOT_RESTORE_DONE":  []string{"create-from-snap-only"},
	}
	for _, v := range fh.CreateFSnapIds {
		sort.Strings(v)
	}
	assert.Equal(expCFSIds, fh.CreateFSnapIds)
	t.Log(fh.CreateFSnapIds)
	tl.Flush()

	// skip dispatch and db error on volume series load
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("test dispatch and db error on volume series load")
	a.doneRequests = make(map[models.ObjID]models.ObjVersion)
	a.doneRequests["old-request"] = 4
	a.doneRequests["error-db-error"] = 3
	vrs2 = []*models.VolumeSeriesRequest{
		&models.VolumeSeriesRequest{
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "skip-me",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"MOUNT"},
				CompleteByTime:      strfmt.DateTime(time.Now().Add(-1 * time.Minute)),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs3",
				},
			},
		},
		&models.VolumeSeriesRequest{
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID:      "old-request",
					Version: 3,
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"MOUNT"},
				CompleteByTime:      strfmt.DateTime(time.Now().Add(-1 * time.Minute)),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "PLACEMENT",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs2",
				},
			},
		},
		&models.VolumeSeriesRequest{
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID:      "error-db-error",
					Version: 3,
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"CREATE"},
				CompleteByTime:      strfmt.DateTime(time.Now().Add(-1 * time.Minute)),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "NEW",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs1",
				},
			},
		},
	}
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	a.OCrud = crud.NewClient(mAPI, a.Log)
	vOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vOps)
	mV := mockmgmtclient.NewVolumeSeriesMatcher(t, volume_series.NewVolumeSeriesFetchParams().WithID(string(resV.Payload.Meta.ID)))
	retErr := &volume_series.VolumeSeriesFetchDefault{Payload: &models.Error{Code: 500, Message: swag.String(com.ErrorDbError)}}
	vOps.EXPECT().VolumeSeriesFetch(mV).Return(nil, retErr)
	cnt = a.DispatchRequests(ctx, vrs2)
	assert.Zero(cnt)
	tl.Flush()
	mockCtrl.Finish()

	t.Log("case: panic and recovery")
	mockCtrl = gomock.NewController(t)
	a.doneRequests = make(map[models.ObjID]models.ObjVersion)
	vrs2 = []*models.VolumeSeriesRequest{
		&models.VolumeSeriesRequest{
			VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
				Meta: &models.ObjMeta{
					ID: "panicID",
				},
			},
			VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
				RequestedOperations: []string{"MOUNT"},
				CompleteByTime:      strfmt.DateTime(time.Now().Add(3 * time.Minute)),
			},
			VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
				VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
					VolumeSeriesRequestState: "VOLUME_CONFIG",
				},
				VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
					VolumeSeriesID: "vs1",
				},
			},
		},
	}
	vr := vrs2[0]
	fh.PanicOnState = com.VolReqStateVolumeExport
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	a.OCrud = crud.NewClient(mAPI, a.Log)
	vrOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(vrOps).MinTimes(1)
	resVR := &volume_series_request.VolumeSeriesRequestUpdateOK{Payload: vr}
	vrOps.EXPECT().VolumeSeriesRequestUpdate(newVolumeSeriesRequestUpdateMatcher("panicID")).Return(resVR, nil).Times(2)
	vOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vOps)
	vOps.EXPECT().VolumeSeriesFetch(newVolumeSeriesFetchMatcher(resV.Payload.Meta.ID)).Return(resV, nil)
	assert.Empty(a.activeRequests)
	cnt = a.DispatchRequests(ctx, vrs2)
	assert.Equal(1, cnt)
	for len(a.activeRequests) > 0 {
		time.Sleep(time.Millisecond * 1)
	}
	_, ok := a.doneRequests["panicID"]
	assert.True(ok)
	if assert.Len(vr.RequestMessages, 3) {
		assert.Regexp("VOLUME_CONFIG ⇒ VOLUME_EXPORT", vr.RequestMessages[0].Message)
		assert.Regexp("^Panic occurred: VOLUME_EXPORT", vr.RequestMessages[1].Message)
		assert.Regexp("VOLUME_EXPORT ⇒ FAILED", vr.RequestMessages[2].Message)
	}
	assert.Equal(1, tl.CountPattern("PANIC: VOLUME_EXPORT"))
}

func TestHelpers(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	vr := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:      "vr1",
				Version: 1,
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
				VolumeSeriesID:     "vs1",
				ConsistencyGroupID: "cg1",
			},
		},
	}
	resVR := &volume_series_request.VolumeSeriesRequestUpdateOK{
		Payload: vr,
	}
	fh := newFakeRequestHandlers(t, tl.Logger())
	fh.WaitForSignal = true
	ops := &fakeOps{}
	expMessagesLen := 0

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	a := NewAnimator(0, tl.Logger(), ops)
	assert.Empty(a.activeRequests)
	a.createHandlers = fh
	a.bindHandlers = fh
	a.allocationHandlers = fh
	a.mountHandlers = fh
	a.renameHandlers = fh
	a.OCrud = crud.NewClient(mAPI, a.Log)
	rhs := &RequestHandlerState{
		A:         a,
		Request:   vr,
		HasCreate: util.Contains(vr.RequestedOperations, com.VolReqOpCreate),
		HasBind:   util.Contains(vr.RequestedOperations, com.VolReqOpBind),
		HasMount:  util.Contains(vr.RequestedOperations, com.VolReqOpMount),
		Canceling: vr.CancelRequested,
	}
	assert.False(rhs.InError)
	assert.Empty(rhs.Request.RequestMessages)
	msgErr := rhs.SetRequestError("in error")
	assert.Equal("in error", msgErr)
	expMessagesLen++
	assert.Len(rhs.Request.RequestMessages, expMessagesLen)
	assert.True(rhs.InError)
	rhs.InError = false
	msgMsg := rhs.SetRequestMessage("random %s message", "fun")
	assert.Equal("random fun message", msgMsg)
	expMessagesLen++
	assert.Len(rhs.Request.RequestMessages, expMessagesLen)
	assert.Equal(1, tl.CountPattern(msgErr))
	assert.Equal(1, tl.CountPattern(msgMsg))
	msgMsg = rhs.SetRequestMessageDistinct("a %s message", "repeated")
	assert.Equal("a repeated message", msgMsg)
	msgMsg = rhs.SetRequestMessageDistinct("a %s message", "repeated")
	assert.Equal("a repeated message", msgMsg)
	expMessagesLen++
	assert.Len(rhs.Request.RequestMessages, expMessagesLen) // inserted once
	assert.Equal(2, tl.CountPattern(msgMsg))                // logged twice
	tl.Flush()

	// SetAndUpdateRequestMessageRepeated
	reqMsgCnt := len(rhs.Request.RequestMessages)
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	a.OCrud = crud.NewClient(mAPI, a.Log)
	vrOps := mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(vrOps).MinTimes(1)
	vrOps.EXPECT().VolumeSeriesRequestUpdate(gomock.Not(gomock.Nil())).Return(resVR, nil).MinTimes(2)
	rhs.SetAndUpdateRequestMessageRepeated(nil, "random %s message", "fun")
	assert.Len(rhs.Request.RequestMessages, reqMsgCnt)
	assert.Equal("random fun message (repeated 1)", rhs.Request.RequestMessages[1].Message)
	rhs.SetAndUpdateRequestMessageRepeated(nil, "random %s message", "fun")
	assert.Len(rhs.Request.RequestMessages, reqMsgCnt)
	assert.Equal("random fun message (repeated 2)", rhs.Request.RequestMessages[1].Message)
	expMessagesLen++
	rhs.SetAndUpdateRequestMessageRepeated(nil, "%s %s message", "brand", "new") // default behavior to append new msg doesn't change
	assert.Len(rhs.Request.RequestMessages, expMessagesLen)
	assert.Equal("brand new message", rhs.Request.RequestMessages[reqMsgCnt].Message)
	mockCtrl.Finish()

	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	a.OCrud = crud.NewClient(mAPI, a.Log)
	vrOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(vrOps).MinTimes(1)
	op := vrOps.EXPECT().VolumeSeriesRequestUpdate(gomock.Not(gomock.Nil())).Return(resVR, nil).Times(3)
	vrOps.EXPECT().VolumeSeriesRequestUpdate(gomock.Not(gomock.Nil())).Return(nil, fmt.Errorf("update error")).After(op)
	rhs.InError = false
	rhs.SetAndUpdateRequestError(nil, "err msg")
	assert.True(rhs.InError)

	expMessagesLen++
	assert.Len(rhs.Request.RequestMessages, expMessagesLen)
	rhs.InError = false
	rhs.SetAndUpdateRequestMessageDistinct(nil, "test msg %d", 1) // calls SetAndUpdateRequestMessage
	expMessagesLen++
	assert.Len(rhs.Request.RequestMessages, expMessagesLen)
	assert.Equal("test msg 1", rhs.Request.RequestMessages[expMessagesLen-1].Message)
	rhs.SetAndUpdateRequestMessageDistinct(nil, "test msg %d", 1)
	assert.Len(rhs.Request.RequestMessages, expMessagesLen)

	assert.False(rhs.SetRequestState("BINDING"))
	assert.Len(rhs.Request.RequestMessages, expMessagesLen)
	assert.True(rhs.SetRequestState("SIZING"))
	expMessagesLen++
	assert.Len(rhs.Request.RequestMessages, expMessagesLen)

	err := rhs.SetAndUpdateRequestState(nil, "SIZING")
	assert.NoError(err)
	assert.Len(rhs.Request.RequestMessages, expMessagesLen)

	err = rhs.SetAndUpdateRequestState(nil, "PLACEMENT")
	assert.NoError(err)
	expMessagesLen++
	assert.Len(rhs.Request.RequestMessages, expMessagesLen)

	err = rhs.SetAndUpdateRequestState(nil, "STORAGE_WAIT")
	assert.Error(err)
	expMessagesLen++
	assert.Len(rhs.Request.RequestMessages, expMessagesLen)
	tl.Flush()

	// volumeSeriesId handling
	fc := &fake.Client{}
	a.OCrud = fc
	rhs.originalVsID = rhs.Request.VolumeSeriesID
	rhs.originalCgID = rhs.Request.ConsistencyGroupID
	assert.NotEmpty(rhs.originalVsID)
	err = rhs.UpdateRequest(nil)
	assert.NotContains(fc.InVSRUpdaterItems.Set, "volumeSeriesId")
	assert.NotEmpty(rhs.originalVsID)
	assert.NotEmpty(rhs.Request.VolumeSeriesID)
	rhs.HasCreate = true
	err = rhs.UpdateRequest(nil)
	assert.NotContains(fc.InVSRUpdaterItems.Set, "volumeSeriesId")
	assert.NotEmpty(rhs.originalVsID)
	assert.NotEmpty(rhs.Request.VolumeSeriesID)
	rhs.originalVsID = ""
	rhs.originalCgID = ""
	err = rhs.UpdateRequest(nil)
	assert.Contains(fc.InVSRUpdaterItems.Set, "volumeSeriesId")
	rhs.HasCreate = false                 // reset
	a.OCrud = crud.NewClient(mAPI, a.Log) // reset
	assert.NotEmpty(rhs.originalVsID)
	assert.NotEmpty(rhs.Request.VolumeSeriesID)

	// servicePlanAllocationId handling
	fc = &fake.Client{}
	a.OCrud = fc
	rhs.Request.ServicePlanAllocationID = "spaID-1"
	rhs.originalSpaID = rhs.Request.ServicePlanAllocationID
	assert.NotEmpty(rhs.originalSpaID)
	err = rhs.UpdateRequest(nil)
	assert.NotContains(fc.InVSRUpdaterItems.Set, "servicePlanAllocationId")
	assert.NotEmpty(rhs.Request.ServicePlanAllocationID)
	assert.Equal(rhs.originalSpaID, rhs.Request.ServicePlanAllocationID)
	rhs.HasAllocateCapacity = true
	err = rhs.UpdateRequest(nil)
	assert.NotContains(fc.InVSRUpdaterItems.Set, "servicePlanAllocationId")
	assert.NotEmpty(rhs.Request.ServicePlanAllocationID)
	assert.Equal(rhs.originalSpaID, rhs.Request.ServicePlanAllocationID)
	rhs.originalSpaID = ""
	err = rhs.UpdateRequest(nil)
	assert.Contains(fc.InVSRUpdaterItems.Set, "servicePlanAllocationId")
	assert.NotEmpty(rhs.Request.ServicePlanAllocationID)
	assert.Equal(rhs.originalSpaID, rhs.Request.ServicePlanAllocationID)
	rhs.HasAllocateCapacity = false          // reset
	rhs.Request.ServicePlanAllocationID = "" // reset
	a.OCrud = crud.NewClient(mAPI, a.Log)    // reset

	// nodeId handling
	fc = &fake.Client{}
	a.OCrud = fc
	rhs.Request.NodeID = "node-id"
	rhs.originalNodeID = rhs.Request.NodeID
	assert.NotEmpty(rhs.originalNodeID)
	err = rhs.UpdateRequest(nil)
	assert.NotContains(fc.InVSRUpdaterItems.Set, "nodeId")
	assert.NotEmpty(rhs.originalNodeID)
	assert.NotEmpty(rhs.Request.NodeID)
	rhs.HasDelete = true
	err = rhs.UpdateRequest(nil)
	assert.NotContains(fc.InVSRUpdaterItems.Set, "nodeId")
	assert.NotEmpty(rhs.originalNodeID)
	assert.NotEmpty(rhs.Request.NodeID)
	rhs.originalNodeID = ""
	err = rhs.UpdateRequest(nil)
	assert.Contains(fc.InVSRUpdaterItems.Set, "nodeId")
	assert.NotEmpty(rhs.originalNodeID)
	assert.Equal(rhs.originalNodeID, rhs.Request.NodeID)
	rhs.HasDelete = false                 // reset
	rhs.Request.NodeID = ""               // reset
	a.OCrud = crud.NewClient(mAPI, a.Log) // reset

	tl.Logger().Debug("*** cancelRequested during update ***")
	vr2 := *vr
	vr2.CancelRequested = true
	resVR.Payload = &vr2
	fetchVR := &volume_series_request.VolumeSeriesRequestFetchOK{
		Payload: &vr2,
	}
	op = vrOps.EXPECT().VolumeSeriesRequestUpdate(gomock.Not(gomock.Nil())).Return(nil, fmt.Errorf(com.ErrorIDVerNotFound))
	mV := mockmgmtclient.NewVolumeSeriesRequestMatcher(t, volume_series_request.NewVolumeSeriesRequestFetchParams().WithID(string(vr2.Meta.ID)))
	vrOps.EXPECT().VolumeSeriesRequestFetch(mV).Return(fetchVR, nil)
	vrOps.EXPECT().VolumeSeriesRequestUpdate(gomock.Not(gomock.Nil())).Return(resVR, nil).MinTimes(1).After(op)
	assert.False(rhs.Canceling)
	assert.False(rhs.Request.CancelRequested)
	err = rhs.SetAndUpdateRequestState(nil, "PLACEMENT")
	assert.NoError(err)
	expMessagesLen++
	assert.Len(vr.RequestMessages, expMessagesLen)
	assert.True(rhs.Canceling)
	assert.True(rhs.Request.CancelRequested)
	tl.Flush()
	mockCtrl.Finish()

	tl.Logger().Debug("*** SyncPeer change during update @ earlier state ***")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	a.OCrud = crud.NewClient(mAPI, a.Log)
	vrOps = mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(vrOps).MinTimes(1)
	// first update fails
	vr.SyncPeers = nil
	rhs.Request = vr
	items := &crud.Updates{}
	items.Set = []string{"volumeSeriesRequestState"}
	updateParams1 := volume_series_request.NewVolumeSeriesRequestUpdateParams()
	updateParams1.ID = string(vr.Meta.ID)
	updateParams1.Payload = &vr.VolumeSeriesRequestMutable
	updateParams1.Set = items.Set
	updateParams1.Version = int32(vr.Meta.Version)
	mU1 := mockmgmtclient.NewVolumeSeriesRequestMatcher(t, updateParams1)
	op = vrOps.EXPECT().VolumeSeriesRequestUpdate(mU1).Return(nil, fmt.Errorf(com.ErrorIDVerNotFound))
	// fetch is done
	var fetchVSR *models.VolumeSeriesRequest
	testutils.Clone(vr, &fetchVSR)
	fetchVSR.Meta.Version = vr.Meta.Version + 2                             // version advanced
	fetchVSR.SyncPeers = map[string]models.SyncPeer{"vsID": {}}             // sync Peer updated
	fetchVSR.VolumeSeriesRequestState = vr.VolumeSeriesRequestState + "XXX" // earlier state
	fetchVR.Payload = fetchVSR
	mV = mockmgmtclient.NewVolumeSeriesRequestMatcher(t, volume_series_request.NewVolumeSeriesRequestFetchParams().WithID(string(fetchVSR.Meta.ID)))
	vrOps.EXPECT().VolumeSeriesRequestFetch(mV).Return(fetchVR, nil)
	// second update succeeds
	var updatedVSR *models.VolumeSeriesRequest
	testutils.Clone(vr, &updatedVSR) // copy of in memory vsr + changes for meta and syncPeers
	updatedVSR.Meta = fetchVSR.Meta
	updatedVSR.SyncPeers = fetchVSR.SyncPeers
	updateParams2 := volume_series_request.NewVolumeSeriesRequestUpdateParams()
	updateParams2.ID = string(updatedVSR.Meta.ID)
	updateParams2.Payload = &vr.VolumeSeriesRequestMutable // original VSR used as payload
	updateParams2.Set = items.Set
	updateParams2.Version = int32(updatedVSR.Meta.Version)
	mU2 := mockmgmtclient.NewVolumeSeriesRequestMatcher(t, updateParams2)
	// returned VSR
	var resultVSR *models.VolumeSeriesRequest
	testutils.Clone(updatedVSR, &resultVSR)
	resultVSR.Meta.Version++
	resVR.Payload = resultVSR
	vrOps.EXPECT().VolumeSeriesRequestUpdate(mU2).Return(resVR, nil).MinTimes(1).After(op)
	// use the low-level call
	err = rhs.UpdateRequestWithItems(nil, items)
	assert.NoError(err)
	// final state is the internal state
	assert.Equal(vr.VolumeSeriesRequestState, rhs.Request.VolumeSeriesRequestState)
	assert.NotEqual(fetchVSR.VolumeSeriesRequestState, rhs.Request.VolumeSeriesRequestState)
	// but version/syncPeers updated
	assert.Equal(resultVSR.Meta.Version, rhs.Request.Meta.Version)
	assert.Equal(fetchVSR.SyncPeers, rhs.Request.SyncPeers)
}

func TestRequestAnimator(t *testing.T) {
	assert := assert.New(t)

	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	cbTime := time.Now().Add(time.Hour)
	vr := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "vr1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			RequestedOperations: []string{"BIND", "MOUNT"},
			CompleteByTime:      strfmt.DateTime(cbTime),
			ClusterID:           "cluster1",
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "NEW",
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				VolumeSeriesID: "vs1",
			},
		},
	}
	resVR := &volume_series_request.VolumeSeriesRequestUpdateOK{
		Payload: vr,
	}
	fh := newFakeRequestHandlers(t, tl.Logger())
	ops := &fakeOps{}

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	a := NewAnimator(0, tl.Logger(), ops)
	assert.Empty(a.activeRequests)
	a.createHandlers = fh
	a.bindHandlers = fh
	a.allocationHandlers = fh
	a.mountHandlers = fh
	a.renameHandlers = fh
	a.OCrud = crud.NewClient(mAPI, a.Log)
	rhs := &RequestHandlerState{
		A:         a,
		Request:   vr,
		HasCreate: util.Contains(vr.RequestedOperations, com.VolReqOpCreate),
		HasBind:   util.Contains(vr.RequestedOperations, com.VolReqOpBind),
		HasMount:  util.Contains(vr.RequestedOperations, com.VolReqOpMount),
		Canceling: vr.CancelRequested,
	}

	// multiple operations
	vrOps := mockmgmtclient.NewMockVolumeSeriesRequestClient(mockCtrl)
	mAPI.EXPECT().VolumeSeriesRequest().Return(vrOps).MinTimes(1)
	vrOps.EXPECT().VolumeSeriesRequestUpdate(gomock.Not(gomock.Nil())).Return(resVR, nil).Times(6)
	rhs.requestAnimator(nil)
	assert.Equal(5, fh.Count)
	assert.Equal("SUCCEEDED", vr.VolumeSeriesRequestState)
	tl.Flush()

	// update fails, RetryLater set
	fh.Count = 0
	vr.VolumeSeriesRequestState = "NEW"
	vrOps.EXPECT().VolumeSeriesRequestUpdate(gomock.Not(gomock.Nil())).Return(nil, fmt.Errorf("update failure"))
	assert.False(rhs.RetryLater)
	rhs.requestAnimator(nil)
	assert.Equal("BINDING", vr.VolumeSeriesRequestState)
	assert.Zero(fh.Count)
	assert.True(rhs.RetryLater)
	rhs.RetryLater = false
	tl.Flush()

	// timeout
	tl.Logger().Info("*** timed out (normal) ***")
	vr.CompleteByTime = strfmt.DateTime(time.Now().Add(-1 * time.Minute))
	vrOps.EXPECT().VolumeSeriesRequestUpdate(gomock.Not(gomock.Nil())).Return(resVR, nil).Times(2)
	rhs.requestAnimator(nil)
	assert.Equal(1, tl.CountPattern("Timed out"))
	assert.Equal(1, tl.CountPattern("BINDING ⇒ UNDO_BINDING"))
	assert.Equal(1, fh.Count) // undo states
	assert.True(rhs.TimedOut)
	rhs.TimedOut = false
	vr.CompleteByTime = strfmt.DateTime(cbTime)
	tl.Flush()

	tl.Logger().Info("*** canceled on entry ***")
	fh.Count = 0
	vr.RequestedOperations = []string{"CREATE", "BIND", "MOUNT"}
	vr.VolumeSeriesRequestState = "NEW"
	rhs.HasCreate = true
	rhs.Canceling = true
	vrOps.EXPECT().VolumeSeriesRequestUpdate(gomock.Not(gomock.Nil())).Return(resVR, nil).Times(2)
	rhs.requestAnimator(nil)
	assert.Equal(1, fh.Count) // undo states
	assert.Equal("CANCELED", vr.VolumeSeriesRequestState)
	assert.Equal(1, tl.CountPattern("NEW ⇒ UNDO_CREATING"))
	assert.Equal(1, tl.CountPattern("UNDO_CREATING ⇒ CANCELED"))
	tl.Flush()

	tl.Logger().Info("*** canceled detected on update ***")
	fh.Count = 0
	vr.VolumeSeriesRequestState = com.VolReqStateVolumeConfig
	rhs.Canceling = false
	rhs.AbortUndo = true // ignored when set before undo starts
	vr2 := *vr
	vr2.CancelRequested = true
	vr2.VolumeSeriesRequestState = com.VolReqStateVolumeExport // emulate successful update after fetch
	resVR2 := &volume_series_request.VolumeSeriesRequestUpdateOK{
		Payload: &vr2,
	}
	fetchVR2 := &volume_series_request.VolumeSeriesRequestFetchOK{
		Payload: &vr2,
	}
	op := vrOps.EXPECT().VolumeSeriesRequestUpdate(gomock.Not(gomock.Nil())).Return(nil, fmt.Errorf(com.ErrorIDVerNotFound))
	mV := mockmgmtclient.NewVolumeSeriesRequestMatcher(t, volume_series_request.NewVolumeSeriesRequestFetchParams().WithID(string(vr2.Meta.ID)))
	vrOps.EXPECT().VolumeSeriesRequestFetch(mV).Return(fetchVR2, nil)
	vrOps.EXPECT().VolumeSeriesRequestUpdate(gomock.Not(gomock.Nil())).Return(resVR2, nil).Times(7).After(op)
	rhs.requestAnimator(nil)
	assert.Equal(6, fh.Count) // undo states
	assert.False(rhs.AbortUndo)
	assert.True(rhs.Request == &vr2)
	assert.Equal("CANCELED", vr2.VolumeSeriesRequestState)
	assert.Equal(1, tl.CountPattern("VOLUME_EXPORT ⇒ UNDO_VOLUME_CONFIG"))
	assert.Equal(1, tl.CountPattern("UNDO_VOLUME_CONFIG ⇒ UNDO_PLACEMENT"))
	assert.Equal(1, tl.CountPattern("UNDO_PLACEMENT ⇒ UNDO_SIZING"))
	assert.Equal(1, tl.CountPattern("UNDO_SIZING ⇒ UNDO_BINDING"))
	assert.Equal(1, tl.CountPattern("UNDO_BINDING ⇒ UNDO_CREATING"))
	assert.Equal(1, tl.CountPattern("UNDO_CREATING ⇒ CANCELED"))
	tl.Flush()

	tl.Logger().Info("*** canceled in handler ***")
	fh.Count = 0
	fh.CancelOnState = com.VolReqStateVolumeExport
	vr.VolumeSeriesRequestState = com.VolReqStateVolumeConfig
	vr.CancelRequested = false
	rhs.Request = vr
	rhs.Canceling = false
	vrOps.EXPECT().VolumeSeriesRequestUpdate(gomock.Not(gomock.Nil())).Return(resVR, nil).Times(8)
	rhs.requestAnimator(nil)
	assert.Equal(8, fh.Count) // regular states and undo states
	assert.Equal("CANCELED", vr.VolumeSeriesRequestState)
	assert.Equal(1, tl.CountPattern("VOLUME_EXPORT ⇒ UNDO_VOLUME_EXPORT"))
	assert.Equal(1, tl.CountPattern("UNDO_VOLUME_EXPORT ⇒ UNDO_VOLUME_CONFIG"))
	assert.Equal(1, tl.CountPattern("UNDO_VOLUME_CONFIG ⇒ UNDO_PLACEMENT"))
	assert.Equal(1, tl.CountPattern("UNDO_PLACEMENT ⇒ UNDO_SIZING"))
	assert.Equal(1, tl.CountPattern("UNDO_SIZING ⇒ UNDO_BINDING"))
	assert.Equal(1, tl.CountPattern("UNDO_BINDING ⇒ UNDO_CREATING"))
	assert.Equal(1, tl.CountPattern("UNDO_CREATING ⇒ CANCELED"))
	tl.Flush()

	tl.Logger().Info("*** error in handler, db error during undo ***")
	fh.Count = 0
	fh.CancelOnState = ""
	fh.FailOnStates = []string{com.VolReqStateVolumeExport}
	vr.VolumeSeriesRequestState = com.VolReqStateVolumeExport
	vr.CancelRequested = false
	rhs.Canceling = false
	assert.False(rhs.InError)
	op = vrOps.EXPECT().VolumeSeriesRequestUpdate(gomock.Not(gomock.Nil())).Return(resVR, nil).Times(3)
	vrOps.EXPECT().VolumeSeriesRequestUpdate(gomock.Not(gomock.Nil())).Return(nil, fmt.Errorf(com.ErrorDbError)).After(op)
	rhs.requestAnimator(nil)
	assert.Equal(4, fh.Count) // regular states and undo states
	assert.Equal("UNDO_SIZING", vr.VolumeSeriesRequestState)
	assert.Equal(1, tl.CountPattern("VOLUME_EXPORT ⇒ UNDO_VOLUME_EXPORT"))
	assert.Equal(1, tl.CountPattern("UNDO_VOLUME_EXPORT ⇒ UNDO_VOLUME_CONFIG"))
	assert.Equal(1, tl.CountPattern("UNDO_VOLUME_CONFIG ⇒ UNDO_PLACEMENT"))
	assert.Equal(1, tl.CountPattern("UNDO_PLACEMENT ⇒ UNDO_SIZING"))
	assert.Zero(tl.CountPattern("UNDO_SIZING ⇒"))
	assert.True(rhs.RetryLater)
	tl.Flush()

	tl.Logger().Info("*** error in handler, undo aborted ***")
	fh.Count = 0
	fh.CancelOnState = ""
	fh.FailOnStates = []string{com.VolReqStateVolumeExport}
	fh.AbortOnState = com.VolReqStateUndoVolumeConfig
	vr.VolumeSeriesRequestState = com.VolReqStateVolumeExport
	vr.CancelRequested = false
	rhs.Canceling = false
	rhs.InError = false
	rhs.AbortUndo = false
	rhs.RetryLater = false
	vrOps.EXPECT().VolumeSeriesRequestUpdate(gomock.Not(gomock.Nil())).Return(resVR, nil).Times(3)
	rhs.requestAnimator(nil)
	assert.Equal(3, fh.Count) // regular states and undo states
	assert.Equal("FAILED", vr.VolumeSeriesRequestState)
	assert.Equal(1, tl.CountPattern("VOLUME_EXPORT ⇒ UNDO_VOLUME_EXPORT"))
	assert.Equal(1, tl.CountPattern("UNDO_VOLUME_EXPORT ⇒ UNDO_VOLUME_CONFIG"))
	assert.Equal(1, tl.CountPattern("UNDO_VOLUME_CONFIG ⇒ FAILED"))
	assert.Equal(1, tl.CountPattern("AbortUndo state:UNDO_VOLUME_CONFIG"))
	assert.True(rhs.AbortUndo)
	assert.True(rhs.InError)
	tl.Flush()

	tl.Logger().Info("*** undo on entry ***")
	fh.Count = 0
	fh.AbortOnState = ""
	fh.FailOnStates = []string{}
	vr.RequestedOperations = []string{"MOUNT"}
	vr.VolumeSeriesRequestState = com.VolReqStateUndoVolumeExport
	vr.CancelRequested = false
	rhs.Request = vr
	rhs.HasCreate = false
	rhs.HasBind = false
	rhs.InError = false
	rhs.Canceling = false
	rhs.RetryLater = false
	vrOps.EXPECT().VolumeSeriesRequestUpdate(gomock.Not(gomock.Nil())).Return(resVR, nil).Times(4)
	rhs.requestAnimator(nil)
	assert.Equal(4, fh.Count) // undo states
	assert.Equal("FAILED", vr.VolumeSeriesRequestState)
	assert.Zero(tl.CountPattern("⇒ UNDO_VOLUME_EXPORT"))
	assert.Equal(1, tl.CountPattern("UNDO_VOLUME_EXPORT ⇒ UNDO_VOLUME_CONFIG"))
	assert.Equal(1, tl.CountPattern("UNDO_VOLUME_CONFIG ⇒ UNDO_PLACEMENT"))
	assert.Equal(1, tl.CountPattern("UNDO_PLACEMENT ⇒ UNDO_SIZING"))
	assert.Equal(1, tl.CountPattern("UNDO_SIZING ⇒ FAILED"))
	assert.True(rhs.InError)
	assert.False(rhs.RetryLater)
	tl.Flush()

	tl.Logger().Info("*** undo on entry, subset of undo states ***")
	fh.Count = 0
	vr.VolumeSeriesRequestState = com.VolReqStateUndoPlacement
	rhs.Request = vr
	rhs.InError = false
	rhs.RetryLater = false
	vrOps.EXPECT().VolumeSeriesRequestUpdate(gomock.Not(gomock.Nil())).Return(resVR, nil).Times(2)
	rhs.requestAnimator(nil)
	assert.Equal(2, fh.Count) // undo states
	assert.Equal("FAILED", vr.VolumeSeriesRequestState)
	assert.Zero(tl.CountPattern("⇒ UNDO_PLACEMENT"))
	assert.Equal(1, tl.CountPattern("UNDO_PLACEMENT ⇒ UNDO_SIZING"))
	assert.Equal(1, tl.CountPattern("UNDO_SIZING ⇒ FAILED"))
	assert.True(rhs.InError)
	assert.False(rhs.RetryLater)
	tl.Flush()

	tl.Logger().Info("*** undo on entry, renaming ***")
	fh.Count = 0
	vr.VolumeSeriesRequestState = com.VolReqStateUndoRenaming
	rhs.Request = vr
	rhs.HasCreate = false
	rhs.HasBind = false
	rhs.HasMount = false
	rhs.HasDelete = false
	rhs.HasRename = true
	rhs.InError = false
	rhs.RetryLater = false
	vrOps.EXPECT().VolumeSeriesRequestUpdate(gomock.Not(gomock.Nil())).Return(resVR, nil)
	rhs.requestAnimator(nil)
	assert.Equal(1, fh.Count) // undo states
	assert.Equal("FAILED", vr.VolumeSeriesRequestState)
	assert.Zero(tl.CountPattern("⇒ UNDO_RENAMING"))
	assert.Equal(1, tl.CountPattern("UNDO_RENAMING ⇒ FAILED"))
	assert.True(rhs.InError)
	assert.False(rhs.RetryLater)

	tl.Logger().Info("*** timed out (CREATE_FROM_SNAPSHOT) ***")
	vr.RequestedOperations = []string{"CREATE_FROM_SNAPSHOT"}
	vr.VolumeSeriesRequestState = "CREATING_FROM_SNAPSHOT"
	vr.CompleteByTime = strfmt.DateTime(time.Now().Add(-1 * time.Minute))
	rhs = &RequestHandlerState{
		A:                     a,
		Request:               vr,
		HasCreateFromSnapshot: true,
	}
	tl.Flush()
	rhs.requestAnimator(nil)
	assert.Equal(0, tl.CountPattern("Timed out"))
	assert.Equal(0, tl.CountPattern("CREATE_FROM_SNAPSHOT ⇒ UNDO_CREATE_FROM_SNAPSHOT"))
	assert.Equal(1, fh.Count) // undo states
	assert.True(rhs.TimedOut)
	assert.True(rhs.suppressTimedOutMessage)
	rhs.TimedOut = false
	vr.CompleteByTime = strfmt.DateTime(cbTime)
}
