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


package vreq

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	"github.com/Nuvoloso/kontroller/pkg/clusterd/state"
	fakeState "github.com/Nuvoloso/kontroller/pkg/clusterd/state/fake"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	mockcsp "github.com/Nuvoloso/kontroller/pkg/csp/mock"
	"github.com/Nuvoloso/kontroller/pkg/layout"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/docker/go-units"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestPlaceSteps(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	app.CSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	app.StateGuard = util.NewCriticalSectionGuard()
	assert.NotNil(app.StateGuard)
	fso := fakeState.NewFakeClusterState()
	app.StateOps = fso
	app.ClusterID = "CLUSTER-1"

	c := newComponent()
	c.Init(app)
	assert.NotNil(c.Log)
	assert.Equal(app.Log, c.Log)
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc
	ctx := context.Background()

	vsObj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID:      "VS-1",
				Version: 1,
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				VolumeSeriesState: "NEW",
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				SizeBytes: swag.Int64(int64(1 * units.GiB)),
			},
		},
	}
	laSNS, err := layout.FindAlgorithm(layout.AlgorithmStandaloneNetworkedShared)
	assert.NoError(err)
	assert.NotNil(laSNS)
	laSLU, err := layout.FindAlgorithm(layout.AlgorithmStandaloneLocalUnshared)
	assert.NoError(err)
	assert.NotNil(laSLU)
	vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VSR-1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			ClusterID: "CLUSTER-1",
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "PLACEMENT",
				StoragePlan: &models.StoragePlan{
					StorageLayout:   laSNS.StorageLayout,
					LayoutAlgorithm: laSNS.Name,
					StorageElements: []*models.StoragePlanStorageElement{
						&models.StoragePlanStorageElement{
							Intent:    "CACHE",
							SizeBytes: swag.Int64(int64(100 * units.MiB)),
							StorageParcels: map[string]models.StorageParcelElement{
								"SSD": models.StorageParcelElement{
									ProvMinSizeBytes: swag.Int64(0),
								},
							},
						},
						&models.StoragePlanStorageElement{
							Intent:    "DATA",
							SizeBytes: swag.Int64(int64(10 * units.GiB)),
							PoolID:    "SP-1",
							StorageParcels: map[string]models.StorageParcelElement{
								"STORAGE": models.StorageParcelElement{
									SizeBytes: swag.Int64(int64(10 * units.GiB)),
								},
							},
						},
						&models.StoragePlanStorageElement{
							Intent:    "DATA",
							SizeBytes: swag.Int64(int64(50 * units.GiB)),
							PoolID:    "SP-2",
							StorageParcels: map[string]models.StorageParcelElement{
								"STORAGE": models.StorageParcelElement{
									SizeBytes: swag.Int64(int64(50 * units.GiB)),
								},
							},
						},
					},
				},
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID:         "NODE-1",
				VolumeSeriesID: "VS-1",
			},
		},
	}
	vs := vsClone(vsObj)
	vsr := vsrClone(vsrObj)
	vsrTag := fmt.Sprintf("%s:%s", com.SystemTagVsrPlacement, vsr.Meta.ID)

	rhsObj := &vra.RequestHandlerState{
		A:            c.Animator,
		Request:      vsr,
		VolumeSeries: vs,
	}
	op := &placeOp{
		c:   c,
		rhs: rhsObj,
	}

	//  ***************************** getInitialState

	// UNDO_PLACEMENT cases
	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.inError = false
	op.rhs.InError = true
	op.rhs.Request.PlanOnly = swag.Bool(true)
	op.rhs.Request.VolumeSeriesRequestState = com.VolReqStateUndoPlacement
	assert.Equal(PlaceUndoDone, op.getInitialState(ctx))
	assert.True(op.inError)
	assert.False(op.rhs.InError)

	op.inError = false
	op.rhs.InError = true
	op.rhs.Request.PlanOnly = nil
	assert.Equal(PlaceUndoStart, op.getInitialState(ctx))
	assert.True(op.inError)
	assert.False(op.rhs.InError)

	vs.VolumeSeriesState = "BOUND" // reset
	vs.SystemTags = []string{}     // reset

	op.inError = false
	op.rhs.InError = false
	assert.Equal(PlaceUndoStart, op.getInitialState(ctx))
	assert.False(op.inError)

	tl.Flush()

	// HasDelete cases
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.Request.VolumeSeriesRequestState = com.VolReqStateUndoPlacement
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = "DELETING"
	vs.SystemTags = []string{}
	op.rhs.VolumeSeries = vs
	op.rhs.HasDelete = true

	op.inError = false
	op.rhs.InError = false
	vsr.SystemTags = []string{"vsr.placement:DELETING"}
	assert.Equal(PlaceUndoStart, op.getInitialState(ctx))
	assert.False(op.inError)

	op.inError = false
	op.rhs.InError = false
	vsr.SystemTags = []string{}
	assert.Equal(PlaceDeleteStart, op.getInitialState(ctx))
	assert.False(op.inError)

	op.inError = false
	op.rhs.InError = false
	vsr.StoragePlan.StorageElements = nil
	assert.Equal(PlaceDeleteStart, op.getInitialState(ctx))
	assert.False(op.inError)

	op.inError = false
	op.rhs.InError = false
	vsr.StoragePlan = nil
	vsr.SystemTags = []string{"vsr.placement:DELETING"}
	assert.Equal(PlaceDeleteStart, op.getInitialState(ctx))
	assert.False(op.inError)

	op.rhs.HasDelete = false

	// HasUnbind cases
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.Request.VolumeSeriesRequestState = com.VolReqStateUndoPlacement
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = "PROVISIONED"
	vs.SystemTags = []string{}
	op.rhs.VolumeSeries = vs
	op.rhs.HasUnbind = true

	op.inError = false
	op.rhs.InError = false
	vsr.SystemTags = []string{"vsr.placement:DELETING"}
	assert.Equal(PlaceUndoStart, op.getInitialState(ctx))
	assert.False(op.inError)

	op.inError = false
	op.rhs.InError = false
	vsr.SystemTags = []string{}
	assert.Equal(PlaceDeleteStart, op.getInitialState(ctx))
	assert.False(op.inError)

	op.inError = false
	op.rhs.InError = false
	vsr.StoragePlan.StorageElements = nil
	assert.Equal(PlaceDeleteStart, op.getInitialState(ctx))
	assert.False(op.inError)

	op.inError = false
	op.rhs.InError = false
	vsr.StoragePlan = nil
	vsr.SystemTags = []string{"vsr.placement:DELETING"}
	assert.Equal(PlaceDeleteStart, op.getInitialState(ctx))
	assert.False(op.inError)

	op.rhs.HasUnbind = false

	// PLACEMENT cases
	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.rhs.Request.PlanOnly = swag.Bool(true)
	delete(op.rhs.Request.StoragePlan.StorageElements[1].StorageParcels, "STORAGE")
	_, has := op.rhs.Request.StoragePlan.StorageElements[1].StorageParcels["STORAGE"]
	assert.False(has)
	assert.Equal(PlaceStartExecuting, op.getInitialState(ctx))

	// provisioned, no lifecycle data
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = "PROVISIONED"
	vs.LifecycleManagementData = nil
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.rhs.RetryLater = false
	op.rhs.InError = false
	assert.Equal(PlaceError, op.getInitialState(ctx))
	tl.Flush()
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Invalid layout algorithm", op.rhs.Request.RequestMessages[0].Message)

	// provisioned, algorithm not found
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = "PROVISIONED"
	vs.LifecycleManagementData = &models.LifecycleManagementData{LayoutAlgorithm: laSNS.Name + "foo"}
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.rhs.RetryLater = false
	op.rhs.InError = false
	assert.Equal(PlaceError, op.getInitialState(ctx))
	tl.Flush()
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Invalid layout algorithm", op.rhs.Request.RequestMessages[0].Message)

	// provisioned, not local unshared
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = "PROVISIONED"
	vs.LifecycleManagementData = &models.LifecycleManagementData{LayoutAlgorithm: laSNS.Name}
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.rhs.RetryLater = false
	op.rhs.InError = false
	assert.Equal(PlaceDone, op.getInitialState(ctx))
	tl.Flush()
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	// provisioned, local unshared, planning not done
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = "PROVISIONED"
	vs.LifecycleManagementData = &models.LifecycleManagementData{LayoutAlgorithm: laSLU.Name}
	vsr = vsrClone(vsrObj)
	vsr.StoragePlan.StorageElements = []*models.StoragePlanStorageElement{}
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.rhs.RetryLater = false
	op.rhs.InError = false
	assert.Equal(PlaceReattachStartPlanning, op.getInitialState(ctx))
	tl.Flush()
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	// provisioned, local unshared, planning done
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = "PROVISIONED"
	vs.LifecycleManagementData = &models.LifecycleManagementData{LayoutAlgorithm: laSLU.Name}
	vsr = vsrClone(vsrObj)
	vsr.StoragePlan.StorageElements = []*models.StoragePlanStorageElement{&models.StoragePlanStorageElement{Intent: com.VolReqStgElemIntentData}}
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.rhs.RetryLater = false
	op.rhs.InError = false
	assert.Equal(PlaceReattachExecute, op.getInitialState(ctx))
	tl.Flush()
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	op.rID, op.vsrTag = "", ""
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.rhs.Request.PlanOnly = swag.Bool(true)
	_, has = op.rhs.Request.StoragePlan.StorageElements[1].StorageParcels["STORAGE"]
	assert.True(has)
	assert.Equal(PlaceStartPlanning, op.getInitialState(ctx))
	tl.Flush()
	assert.Equal(vsrTag, op.vsrTag)
	assert.Equal("VSR-1", op.rID)

	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.rhs.Request.PlanOnly = swag.Bool(true)
	op.rhs.Request.StoragePlan.StorageElements = []*models.StoragePlanStorageElement{}
	assert.Equal(PlaceDone, op.getInitialState(ctx))
	assert.True(op.rhs.InError)

	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.rhs.Request.PlanOnly = swag.Bool(true)
	op.rhs.Request.StoragePlan = nil
	assert.Equal(PlaceDone, op.getInitialState(ctx))
	assert.True(op.rhs.InError)
	op.rhs.InError = false // reset

	//  ***************************** waitForLock
	app.StateGuard.Drain() // force closure
	assert.Nil(op.stateCST)
	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.waitForLock(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("state lock error", op.rhs.Request.RequestMessages[0].Message)
	assert.Nil(op.stateCST)

	app.StateGuard = util.NewCriticalSectionGuard()
	op.rhs.Request.RequestMessages = []*models.TimestampedString{}
	op.rhs.RetryLater = false
	op.rhs.InError = false
	op.waitForLock(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(op.stateCST)
	assert.Equal([]string{"VSR-1"}, app.StateGuard.Status())
	tl.Flush()

	op.waitForLock(ctx) // is re-entrant - won't block

	//  ***************************** selectStorage

	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	vsr.StoragePlan.LayoutAlgorithm = ""
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	assert.Nil(op.selStg)
	op.selectStorage(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Layout algorithm.*not found", op.rhs.Request.RequestMessages[0].Message)
	tl.Flush()

	fso.RetSSErr = fmt.Errorf("select-storage-error")
	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	assert.Nil(op.selStg)
	op.selectStorage(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("select-storage-error", op.rhs.Request.RequestMessages[0].Message)
	assert.NotNil(fso.InSSArgs)
	assert.Equal(vsr, fso.InSSArgs.VSR)
	assert.Equal(vs, fso.InSSArgs.VS)
	assert.Equal(laSNS, fso.InSSArgs.LA)
	tl.Flush()

	resSS := &state.SelectStorageResponse{
		Elements: []state.ElementStorage{
			{}, // CACHE gets empty ElementStorage
			{
				NumItems:        1,
				ParcelSizeBytes: int64(500 * units.MiB),
				Items: []state.StorageItem{
					{SizeBytes: int64(10 * units.GiB), ShareableStorage: true, NumParcels: 10, PoolID: "SP-1", MinSizeBytes: int64(100 * units.GiB), RemainingSizeBytes: 1000, NodeID: "NODE-1"},
				},
			},
			{
				NumItems:        2,
				ParcelSizeBytes: int64(500 * units.MiB),
				Items: []state.StorageItem{
					{
						SizeBytes:  int64(20 * units.GiB),
						NumParcels: 20,
						Storage: &models.Storage{
							StorageAllOf0: models.StorageAllOf0{
								Meta: &models.ObjMeta{ID: "S-1-0"},
							},
							StorageMutable: models.StorageMutable{
								StorageState: &models.StorageStateMutable{},
							},
						},
						PoolID:           "SP-2",
						ShareableStorage: true,
					},
					{SizeBytes: int64(30 * units.GiB), StorageRequestID: "SR-ACTIVE", NumParcels: 30, PoolID: "SP-2", MinSizeBytes: int64(100 * units.GiB), RemainingSizeBytes: 0, NodeID: "NODE-2"},
				},
			},
		},
	}
	resSS.LA, err = layout.FindAlgorithm(layout.AlgorithmStandaloneNetworkedShared)
	assert.NoError(err)
	assert.NotNil(resSS.LA)
	fso.RetSSErr = nil
	fso.RetSSResp = resSS
	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.rhs.RetryLater = false
	assert.Nil(op.selStg)
	op.selectStorage(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.Equal(resSS, op.selStg)
	tl.Flush()

	//  ***************************** tagObjects

	t.Log("tagObjects: planOnly")
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	vsr.PlanOnly = swag.Bool(true)
	op.tagObjects(nil)
	assert.False(op.rhs.RetryLater)
	assert.Empty(op.rhs.Request.RequestMessages)
	vsr.PlanOnly = nil

	t.Log("tagObjects: storage not present, volume not being deleted, volume updater error")
	vs = vsClone(vsObj)
	op.rhs.HasDelete = false
	op.rhs.VolumeSeries = vs
	vs.StorageParcels = map[string]models.ParcelAllocation{
		"S-1-0": models.ParcelAllocation{SizeBytes: swag.Int64(int64(100 * units.GiB))},
	}
	fc.RetUSErr = nil
	fc.PassThroughUSObj = true
	fc.InUSitems = nil
	fc.RetVSUpdaterErr = fmt.Errorf("updater-error")
	fc.InVSUpdaterItems = nil
	op.tagObjects(nil)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Equal([]string{"messages", "volumeSeriesState"}, fc.InVSUpdaterItems.Set)
	assert.Equal([]string{"systemTags"}, fc.InVSUpdaterItems.Append)
	assert.Empty(fc.InVSUpdaterItems.Remove)
	assert.Len(op.rhs.Request.RequestMessages, 2)
	assert.Regexp("Storage.*not present", op.rhs.Request.RequestMessages[0].Message)
	assert.Regexp("Failed to update VolumeSeries", op.rhs.Request.RequestMessages[1].Message)
	op.rhs.RetryLater = false
	fc.RetVSUpdaterErr = nil

	t.Log("tagObjects: storage updater error")
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	fso.CallRealLookupStorage = true
	fso.Storage["S-1-0"] = &models.Storage{
		StorageMutable: models.StorageMutable{
			AvailableBytes: swag.Int64(0),
			StorageState:   &models.StorageStateMutable{},
		},
	}
	fc.PassThroughUSObj = false
	fc.RetUSErr = fmt.Errorf("storage-update-error")
	op.tagObjects(nil)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("S-1-0.*storage-update-error", op.rhs.Request.RequestMessages[0].Message)
	op.rhs.RetryLater = false
	tl.Flush()

	t.Log("tagObjects: already tagged")
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	fc.PassThroughUSObj = true
	fc.RetUSErr = nil
	op.tagObjects(nil)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Tagged VolumeSeries", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal(1, tl.CountPattern("already tagged"))

	t.Log("tagObjects: attached storage updated, volume being deleted, no state change")
	vsr = vsrClone(vsrObj)
	vs.SystemTags = []string{}
	vs.VolumeSeriesState = com.VolStateDeleting
	op.rhs.HasDelete = true
	op.rhs.Request = vsr
	fso.Storage["S-1-0"] = &models.Storage{
		StorageMutable: models.StorageMutable{
			AvailableBytes: swag.Int64(0),
			StorageState:   &models.StorageStateMutable{},
		},
	}
	fc.InVSUpdaterItems = nil
	op.tagObjects(nil)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal([]string{"messages", "volumeSeriesState"}, fc.InVSUpdaterItems.Set)
	assert.Equal([]string{"systemTags"}, fc.InVSUpdaterItems.Append)
	assert.Empty(fc.InVSUpdaterItems.Remove)
	assert.EqualValues([]string{op.vsrTag}, fso.Storage["S-1-0"].SystemTags)
	assert.EqualValues([]string{op.vsrTag}, vs.SystemTags)
	assert.Len(op.rhs.Request.RequestMessages, 2)
	assert.Regexp("Tagged Storage", op.rhs.Request.RequestMessages[0].Message)
	assert.Regexp("Tagged VolumeSeries", op.rhs.Request.RequestMessages[1].Message)
	delete(fso.Storage, "S-1-0")

	t.Log("tagObjects: detached storage updated, volume being deleted, state change")
	vsr = vsrClone(vsrObj)
	vs.SystemTags = []string{}
	vs.VolumeSeriesState = com.VolStateProvisioned
	op.rhs.HasDelete = true
	op.rhs.Request = vsr
	fso.DetachedStorage["S-1-0"] = &models.Storage{
		StorageMutable: models.StorageMutable{
			AvailableBytes: swag.Int64(0),
			StorageState:   &models.StorageStateMutable{},
		},
	}
	fc.InVSUpdaterItems = nil
	op.tagObjects(nil)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal([]string{"messages", "volumeSeriesState"}, fc.InVSUpdaterItems.Set)
	assert.Equal([]string{"systemTags"}, fc.InVSUpdaterItems.Append)
	assert.Empty(fc.InVSUpdaterItems.Remove)
	assert.EqualValues([]string{op.vsrTag}, fso.DetachedStorage["S-1-0"].SystemTags)
	assert.EqualValues([]string{op.vsrTag}, vs.SystemTags)
	assert.Len(op.rhs.Request.RequestMessages, 2)
	assert.Regexp("Tagged Storage", op.rhs.Request.RequestMessages[0].Message)
	assert.Regexp("Tagged VolumeSeries", op.rhs.Request.RequestMessages[1].Message)
	assert.Regexp("set state to "+com.VolStateDeleting, op.rhs.Request.RequestMessages[1].Message)
	assert.Equal(com.VolStateDeleting, op.rhs.VolumeSeries.VolumeSeriesState)
	assert.Len(op.rhs.VolumeSeries.Messages, 1)
	assert.Regexp(com.VolStateDeleting, op.rhs.VolumeSeries.Messages[0].Message)
	delete(fso.DetachedStorage, "S-1-0")

	fc.PassThroughUSObj = false
	fso.CallRealLookupStorage = false
	op.rhs.HasDelete = false

	//  ***************************** populatePlan
	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.selStg = resSS
	op.populatePlan(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Nil(op.selStg)
	assert.True(op.mustUpdateStoragePlan)
	expParcelMap0 := map[string]models.StorageParcelElement{
		"SSD": models.StorageParcelElement{ProvMinSizeBytes: swag.Int64(0)},
	}
	expParcelMap1 := map[string]models.StorageParcelElement{
		"STORAGE-1-0": models.StorageParcelElement{SizeBytes: swag.Int64(int64(10 * units.GiB)), ShareableStorage: true, ProvMinSizeBytes: swag.Int64(int64(100 * units.GiB)), ProvParcelSizeBytes: swag.Int64(int64(500 * units.MiB)), ProvRemainingSizeBytes: swag.Int64(1000), ProvNodeID: "NODE-1"},
	}
	expParcelMap2 := map[string]models.StorageParcelElement{
		"S-1-0":       models.StorageParcelElement{SizeBytes: swag.Int64(int64(20 * units.GiB)), ShareableStorage: true},
		"STORAGE-2-1": models.StorageParcelElement{SizeBytes: swag.Int64(int64(30 * units.GiB)), ProvMinSizeBytes: swag.Int64(int64(100 * units.GiB)), ProvParcelSizeBytes: swag.Int64(int64(500 * units.MiB)), ProvRemainingSizeBytes: swag.Int64(0), ProvNodeID: "NODE-2", ProvStorageRequestID: "SR-ACTIVE"},
	}
	assert.Len(vsr.StoragePlan.StorageElements, 3)
	assert.EqualValues(expParcelMap0, vsr.StoragePlan.StorageElements[0].StorageParcels)
	assert.EqualValues(expParcelMap1, vsr.StoragePlan.StorageElements[1].StorageParcels)
	assert.EqualValues(expParcelMap2, vsr.StoragePlan.StorageElements[2].StorageParcels)
	tl.Flush()

	//  ***************************** populateDeletionPlan
	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	vs.CapacityAllocations = map[string]models.CapacityAllocation{
		"SP-1": models.CapacityAllocation{
			ReservedBytes: swag.Int64(int64(200 * units.GiB)),
			ConsumedBytes: swag.Int64(int64(200 * units.GiB)),
		},
	}
	vs.StorageParcels = map[string]models.ParcelAllocation{
		"S-1-0": models.ParcelAllocation{SizeBytes: swag.Int64(int64(100 * units.GiB))},
		"S-2-0": models.ParcelAllocation{SizeBytes: swag.Int64(int64(100 * units.GiB))},
	}
	vsr.StoragePlan = nil
	fso.Storage["S-1-0"] = &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta: &models.ObjMeta{
				ID: "S-1-0",
			},
			PoolID: "SP-1",
		},
		StorageMutable: models.StorageMutable{
			AvailableBytes: swag.Int64(0),
			StorageState:   &models.StorageStateMutable{},
		},
	}
	fso.Storage["S-1-1"] = &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta: &models.ObjMeta{
				ID: "S-1-1",
			},
			PoolID: "SP-2",
		},
		StorageMutable: models.StorageMutable{
			AvailableBytes: swag.Int64(0),
			StorageState:   &models.StorageStateMutable{},
		},
	}
	fso.DetachedStorage["S-2-0"] = &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta: &models.ObjMeta{
				ID: "S-2-0",
			},
			PoolID: "SP-1",
		},
		StorageMutable: models.StorageMutable{
			AvailableBytes: swag.Int64(0),
			StorageState:   &models.StorageStateMutable{},
		},
	}
	fso.CallRealFindAllStorage = true
	assert.True(fso == op.c.App.StateOps)
	assert.Len(op.c.App.StateOps.CS().Storage, 2)
	op.mustUpdateStoragePlan = false
	op.populateDeletionPlan(nil)
	assert.True(op.mustUpdateStoragePlan)
	if assert.NotNil(vsr.StoragePlan) {
		assert.Len(vsr.StoragePlan.StorageElements, 1)
		assert.EqualValues("SP-1", vsr.StoragePlan.StorageElements[0].PoolID)
		assert.Len(vsr.StoragePlan.StorageElements[0].StorageParcels, 2)
		for _, spe := range vsr.StoragePlan.StorageElements[0].StorageParcels {
			assert.Equal(int64(100*units.GiB), swag.Int64Value(spe.SizeBytes))
		}
	}
	delete(fso.Storage, "S-1-0")
	delete(fso.Storage, "S-1-1")
	delete(fso.DetachedStorage, "S-1-0")
	fso.CallRealFindAllStorage = false
	tl.Flush()

	//  ***************************** updatePlan
	fc.RetVSRUpdaterErr = fmt.Errorf("volume-series-update-error")
	fc.RetVSRUpdaterObj = nil
	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.mustUpdateStoragePlan = true
	op.updatePlan(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.True(op.mustUpdateStoragePlan)

	op.mustUpdateStoragePlan = false
	op.rhs.RetryLater = false
	op.updatePlan(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	fc.RetVSRUpdaterErr = nil
	fc.RetVSRUpdaterObj = nil
	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.rhs.RetryLater = false
	op.mustUpdateStoragePlan = true
	op.updatePlan(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.False(op.mustUpdateStoragePlan)
	assert.NotNil(fc.ModVSRUpdaterObj)
	assert.Equal(vsr, fc.ModVSRUpdaterObj)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Storage plan updated", op.rhs.Request.RequestMessages[0].Message)
	tl.Flush()

	//  ***************************** fetchClaims
	fso.InFCByR = ""
	fso.RetFCByRObj = []*state.ClaimData{}
	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.rhs.RetryLater = false
	op.fetchClaims(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.EqualValues(op.rhs.Request.Meta.ID, fso.InFCByR)
	tl.Flush()

	//  ***************************** handleCompletedStorageRequests
	spe00 := models.StorageParcelElement{SizeBytes: swag.Int64(int64(10 * units.GiB)), ProvMinSizeBytes: swag.Int64(int64(100 * units.GiB)), ProvParcelSizeBytes: swag.Int64(int64(500 * units.MiB)), ProvRemainingSizeBytes: swag.Int64(0), ProvNodeID: "NODE-1"}
	spe10 := models.StorageParcelElement{SizeBytes: swag.Int64(int64(20 * units.GiB))}
	spe11 := models.StorageParcelElement{SizeBytes: swag.Int64(int64(30 * units.GiB)), ProvMinSizeBytes: swag.Int64(int64(100 * units.GiB)), ProvParcelSizeBytes: swag.Int64(int64(500 * units.MiB)), ProvRemainingSizeBytes: swag.Int64(1000), ProvNodeID: "NODE-2"}
	vsrObj.StoragePlan = &models.StoragePlan{
		StorageElements: []*models.StoragePlanStorageElement{
			&models.StoragePlanStorageElement{
				Intent:    "DATA",
				SizeBytes: swag.Int64(int64(10 * units.GiB)),
				PoolID:    "SP-1",
				StorageParcels: map[string]models.StorageParcelElement{
					"STORAGE-0-0": spe00,
				},
			},
			&models.StoragePlanStorageElement{
				Intent:    "DATA",
				SizeBytes: swag.Int64(int64(50 * units.GiB)),
				PoolID:    "SP-2",
				StorageParcels: map[string]models.StorageParcelElement{
					"S-1-0":       spe10,
					"STORAGE-1-1": spe11,
				},
			},
			&models.StoragePlanStorageElement{
				Intent:    "CACHE",
				SizeBytes: swag.Int64(int64(100 * units.MiB)),
				StorageParcels: map[string]models.StorageParcelElement{
					"SSD": models.StorageParcelElement{
						ProvMinSizeBytes: swag.Int64(0),
					},
				},
			},
		},
	}
	sr00 := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{ID: "SR-0-0"},
		},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			PoolID: "SP-1",
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
				StorageRequestState: "SUCCEEDED",
			},
			StorageRequestCreateMutable: models.StorageRequestCreateMutable{
				NodeID:    "NODE-1",
				StorageID: "S-0-0",
			},
		},
	}
	sr00f := srClone(sr00)
	sr00f.StorageRequestState = com.StgReqStateFailed // Failed variant of sr00
	sr11 := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{ID: "SR-1-1"},
		},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			PoolID: "SP-2",
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
				StorageRequestState: "SUCCEEDED",
			},
			StorageRequestCreateMutable: models.StorageRequestCreateMutable{
				NodeID:    "NODE-2",
				StorageID: "S-1-1",
			},
		},
	}
	clD := []*state.ClaimData{
		{StorageRequest: sr00, PoolID: "SP-1", NodeID: "NODE-1", RemainingBytes: 0, VSRClaims: []state.VSRClaim{{RequestID: "VSR-1", SizeBytes: int64(10 * units.GiB), Annotation: "STORAGE-0-0"}}},
		{StorageRequest: sr11, PoolID: "SP-2", NodeID: "NODE-2", RemainingBytes: 0, VSRClaims: []state.VSRClaim{{RequestID: "VSR-1", SizeBytes: int64(30 * units.GiB), Annotation: "STORAGE-1-1"}}},
	}

	// case: rei
	tl.Logger().Info("case: rei")
	tl.Flush()
	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	testutils.Clone(clD, &op.claims)
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.c.rei.SetProperty("sr-race", &rei.Property{BoolValue: true})
	op.handleCompletedStorageRequests(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	op.rhs.RetryLater = false // reset

	// case: success + not-ready
	tl.Logger().Info("case: handleCompletedStorageRequests 1 success, 1 not ready")
	tl.Flush()
	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	testutils.Clone(clD, &op.claims)
	op.claims[1].StorageRequest.StorageRequestState = "ATTACHING" // not ready
	assert.Len(vsr.StoragePlan.StorageElements[0].StorageParcels, 1)
	assert.Len(vsr.StoragePlan.StorageElements[1].StorageParcels, 2)
	op.activeSRs = false
	op.mustUpdateStoragePlan = false
	op.handleCompletedStorageRequests(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.True(op.activeSRs)
	assert.True(op.mustUpdateStoragePlan)
	assert.Len(vsr.StoragePlan.StorageElements[0].StorageParcels, 1)
	assert.Len(vsr.StoragePlan.StorageElements[1].StorageParcels, 2)
	spe, has := vsr.StoragePlan.StorageElements[0].StorageParcels["S-0-0"]
	assert.True(has)
	assert.Equal(spe00, spe)
	_, has = vsr.StoragePlan.StorageElements[0].StorageParcels["STORAGE-0-0"]
	assert.False(has)
	spe, has = vsr.StoragePlan.StorageElements[1].StorageParcels["S-1-0"]
	assert.True(has)
	spe, has = vsr.StoragePlan.StorageElements[1].StorageParcels["STORAGE-1-1"]
	assert.True(has)
	assert.Equal(spe11, spe)
	vsrC, _ := op.claims.FindVSRClaimByAnnotation(op.rID, "STORAGE-0-0")
	assert.Nil(vsrC)
	vsrC, _ = op.claims.FindVSRClaimByAnnotation(op.rID, "STORAGE-1-1")
	assert.NotNil(vsrC)

	// case: failure
	tl.Logger().Info("case: handleCompletedStorageRequests failure")
	tl.Flush()
	op.claims[1].StorageRequest.StorageRequestState = "FAILED"
	op.activeSRs = false
	op.mustUpdateStoragePlan = false
	op.handleCompletedStorageRequests(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("StorageRequest.*failed", op.rhs.Request.RequestMessages[0].Message)
	vsrC, _ = op.claims.FindVSRClaimByAnnotation(op.rID, "STORAGE-1-1") // claim removed on failure
	assert.Nil(vsrC)

	// case: failure
	tl.Logger().Info("case: handleCompletedStorageRequests failure with message")
	tl.Flush()
	testutils.Clone(clD, &op.claims)
	op.claims[1].StorageRequest.StorageRequestState = "FAILED"
	op.claims[1].StorageRequest.RequestMessages = []*models.TimestampedString{{Message: "Error: csp error"}}
	op.rhs.Request.RequestMessages = nil
	op.rhs.InError = false
	op.activeSRs = false
	op.mustUpdateStoragePlan = false
	op.handleCompletedStorageRequests(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Error: STORAGE-1-1 StorageRequest.*csp error", op.rhs.Request.RequestMessages[0].Message)
	vsrC, _ = op.claims.FindVSRClaimByAnnotation(op.rID, "STORAGE-1-1") // claim removed on failure
	assert.Nil(vsrC)

	// case: no change
	op.mustUpdateStoragePlan = false

	// case: success
	tl.Logger().Info("case: handleCompletedStorageRequests 2 success")
	tl.Flush()
	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.rhs.InError = false
	testutils.Clone(clD, &op.claims)
	assert.Len(vsr.StoragePlan.StorageElements[0].StorageParcels, 1)
	assert.Len(vsr.StoragePlan.StorageElements[1].StorageParcels, 2)
	op.handleCompletedStorageRequests(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.False(op.activeSRs)
	assert.Len(vsr.StoragePlan.StorageElements[0].StorageParcels, 1)
	assert.Len(vsr.StoragePlan.StorageElements[1].StorageParcels, 2)
	spe, has = vsr.StoragePlan.StorageElements[0].StorageParcels["S-0-0"]
	assert.True(has)
	assert.Equal(spe00, spe)
	spe, has = vsr.StoragePlan.StorageElements[1].StorageParcels["S-1-0"]
	assert.True(has)
	assert.Equal(spe10, spe)
	spe, has = vsr.StoragePlan.StorageElements[1].StorageParcels["S-1-1"]
	assert.True(has)
	assert.Equal(spe11, spe)
	vsrC, _ = op.claims.FindVSRClaimByAnnotation(op.rID, "STORAGE-0-0")
	assert.Nil(vsrC)
	vsrC, _ = op.claims.FindVSRClaimByAnnotation(op.rID, "STORAGE-1-1")
	assert.Nil(vsrC)

	//  ***************************** claimFromExistingStorageRequests
	tl.Logger().Info("case: Reference to non-existent SR causes restart")
	tl.Flush()
	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	vsr.StoragePlan = &models.StoragePlan{
		StorageElements: []*models.StoragePlanStorageElement{
			&models.StoragePlanStorageElement{
				SizeBytes: swag.Int64(int64(10 * units.GiB)),
				PoolID:    "SP-1",
				StorageParcels: map[string]models.StorageParcelElement{
					"STORAGE-0-0": models.StorageParcelElement{
						ProvStorageRequestID:   "INVALID-SR",
						SizeBytes:              swag.Int64(int64(10 * units.GiB)),
						ProvMinSizeBytes:       swag.Int64(int64(100 * units.GiB)),
						ProvParcelSizeBytes:    swag.Int64(int64(500 * units.MiB)),
						ProvRemainingSizeBytes: swag.Int64(0),
						ProvNodeID:             "NODE-1",
					},
				},
			},
		},
	}
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.mustRestart = false
	fso.RetFCBySRObj = nil
	op.activeSRs = false
	op.mustUpdateRHS = false
	op.claimFromExistingStorageRequests(ctx)
	assert.True(op.mustRestart)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.False(op.activeSRs)
	assert.False(op.mustUpdateRHS)
	assert.Equal("INVALID-SR", fso.InFCBySRid)
	assert.Regexp("INVALID-SR", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Info("case: Skip failed SR")
	tl.Flush()
	vsr = vsrClone(vsrObj)
	vsr.StoragePlan = &models.StoragePlan{
		StorageElements: []*models.StoragePlanStorageElement{
			&models.StoragePlanStorageElement{
				SizeBytes: swag.Int64(int64(10 * units.GiB)),
				PoolID:    "SP-1",
				StorageParcels: map[string]models.StorageParcelElement{
					"STORAGE-0-0": models.StorageParcelElement{
						ProvStorageRequestID:   "SR-0-0",
						SizeBytes:              swag.Int64(int64(10 * units.GiB)),
						ProvMinSizeBytes:       swag.Int64(int64(100 * units.GiB)),
						ProvParcelSizeBytes:    swag.Int64(int64(500 * units.MiB)),
						ProvRemainingSizeBytes: swag.Int64(0),
						ProvNodeID:             "NODE-1",
					},
				},
			},
		},
	}
	cdSR00f := &state.ClaimData{
		StorageRequest: sr00f, // FAILED SR
		PoolID:         "SP-1",
		NodeID:         "NODE-1",
		RemainingBytes: int64(10 * units.GiB),
		VSRClaims:      []state.VSRClaim{{RequestID: "OTHER-VSR", SizeBytes: int64(100 * units.GiB), Annotation: "STORAGE-2-0"}},
	}
	fso.RetFCBySRObj = cdSR00f
	fso.InACsr = nil
	op.rhs.Request = vsr
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.mustRestart = false
	op.claimFromExistingStorageRequests(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Nil(fso.InACsr)
	assert.Equal(1, tl.CountPattern("Skipping failed SR"))

	tl.Logger().Info("case: AddClaim fails")
	tl.Flush()
	vsr = vsrClone(vsrObj)
	vsr.StoragePlan = &models.StoragePlan{
		StorageElements: []*models.StoragePlanStorageElement{
			&models.StoragePlanStorageElement{
				Intent:    "DATA",
				SizeBytes: swag.Int64(int64(10 * units.GiB)),
				PoolID:    "SP-1",
				StorageParcels: map[string]models.StorageParcelElement{
					"STORAGE-0-0": models.StorageParcelElement{
						ProvStorageRequestID:   "SR-0-0",
						SizeBytes:              swag.Int64(int64(10 * units.GiB)),
						ProvMinSizeBytes:       swag.Int64(int64(100 * units.GiB)),
						ProvParcelSizeBytes:    swag.Int64(int64(500 * units.MiB)),
						ProvRemainingSizeBytes: swag.Int64(0),
						ProvNodeID:             "NODE-1",
					},
				},
			},
			&models.StoragePlanStorageElement{
				Intent:    "CACHE",
				SizeBytes: swag.Int64(int64(100 * units.MiB)),
				StorageParcels: map[string]models.StorageParcelElement{
					"SSD": models.StorageParcelElement{
						ProvMinSizeBytes: swag.Int64(0),
					},
				},
			},
		},
	}
	cdSR00 := &state.ClaimData{
		StorageRequest: sr00,
		PoolID:         "SP-1",
		NodeID:         "NODE-1",
		RemainingBytes: int64(10 * units.GiB),
		VSRClaims:      []state.VSRClaim{{RequestID: "OTHER-VSR", SizeBytes: int64(100 * units.GiB), Annotation: "STORAGE-2-0"}},
	}
	fso.RetFCBySRObj = cdSR00
	fso.InACsr = nil
	fso.InACcl = nil
	fso.RetACErr = fmt.Errorf("add-claim-error")
	op.rhs.Request = vsr
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.mustRestart = false
	op.claimFromExistingStorageRequests(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.False(op.activeSRs)
	assert.False(op.mustUpdateRHS)
	assert.False(op.mustRestart)
	assert.Regexp("AddClaim.*add-claim-error", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal("SR-0-0", fso.InFCBySRid)
	assert.Equal(sr00, fso.InACsr)
	assert.Equal(&state.VSRClaim{RequestID: op.rID, SizeBytes: int64(10 * units.GiB), Annotation: "STORAGE-0-0"}, fso.InACcl)

	tl.Logger().Info("case: AddClaim succeeds")
	tl.Flush()
	fso.InACsr = nil
	fso.InACcl = nil
	fso.RetACErr = nil
	fso.InFCByR = ""
	fso.RetFCByRObj = state.ClaimList{
		&state.ClaimData{
			StorageRequest: sr00,
			PoolID:         "SP-1",
			NodeID:         "NODE-1",
			RemainingBytes: int64(10 * units.GiB),
			VSRClaims: []state.VSRClaim{
				{RequestID: "OTHER-VSR", SizeBytes: int64(100 * units.GiB), Annotation: "STORAGE-2-0"},
				{RequestID: op.rID, SizeBytes: int64(10 * units.GiB), Annotation: "STORAGE-0-0"},
			},
		},
	}
	op.rhs.Request = vsr
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.claims = nil
	op.claimFromExistingStorageRequests(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.True(op.activeSRs)
	assert.True(op.mustUpdateRHS)
	assert.False(op.mustRestart)
	assert.Equal(op.rID, fso.InFCByR)
	assert.EqualValues(fso.RetFCByRObj, op.claims)
	assert.Equal("SR-0-0", fso.InFCBySRid)
	assert.Equal(sr00, fso.InACsr)
	assert.Equal(&state.VSRClaim{RequestID: op.rID, SizeBytes: int64(10 * units.GiB), Annotation: "STORAGE-0-0"}, fso.InACcl)

	tl.Logger().Info("case: claims not added again")
	tl.Flush()
	fso.InFCByR = ""
	fso.InACsr = nil
	fso.InACcl = nil
	assert.NotNil(op.claims.FindVSRClaimByAnnotation(op.rID, "STORAGE-0-0"))
	op.activeSRs = false
	op.mustUpdateRHS = false
	op.claimFromExistingStorageRequests(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.False(op.activeSRs)
	assert.False(op.mustUpdateRHS)
	assert.False(op.mustRestart)
	assert.Equal("", fso.InFCByR)
	assert.Nil(fso.InACsr)
	assert.Nil(fso.InACcl)

	//  ***************************** recomputeStoragePlan
	tl.Logger().Info("case: must restart")
	tl.Flush()
	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.InError = false
	op.rhs.RetryLater = false
	assert.Equal(com.VolReqStatePlacement, op.rhs.Request.VolumeSeriesRequestState)
	op.recomputeStoragePlan(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(com.VolReqStateSizing, op.rhs.Request.VolumeSeriesRequestState)
	assert.Regexp("Recomputing storagePlan", op.rhs.Request.RequestMessages[0].Message)
	assert.Regexp("PLACEMENT ⇒ SIZING", op.rhs.Request.RequestMessages[1].Message)
	assert.NotNil(op.rhs.Request.StoragePlan)
	assert.Empty(op.rhs.Request.StoragePlan.StorageElements)

	//  ***************************** issueNewStorageRequests
	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	assert.Len(vsr.StoragePlan.StorageElements, 3)
	vsr.StoragePlan.StorageElements = []*models.StoragePlanStorageElement{
		vsr.StoragePlan.StorageElements[0], // remove 2nd element
		vsr.StoragePlan.StorageElements[2], // keep cache element
	}
	assert.Len(vsr.StoragePlan.StorageElements, 2)
	assert.Len(vsr.StoragePlan.StorageElements[0].StorageParcels, 1)
	spe, has = vsr.StoragePlan.StorageElements[0].StorageParcels["STORAGE-0-0"]
	assert.True(has)
	assert.Equal(spe00, spe)
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.claims = []*state.ClaimData{} // no claims at this time

	tl.Logger().Info("case: issueNewStorageRequests SR create error")
	tl.Flush()
	fc.InSRCArgs = nil
	fc.PassThroughSRCObj = false
	fc.RetSRCErr = fmt.Errorf("storage-request-create-error")
	op.activeSRs = false
	op.mustUpdateRHS = false
	op.issueNewStorageRequests(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.False(op.activeSRs)
	assert.False(op.mustUpdateRHS)

	tl.Logger().Info("case: issueNewStorageRequests single element to validate content")
	tl.Flush()
	fc.InSRCArgs = nil
	fc.PassThroughSRCObj = true
	fso.InTSRsr = nil
	op.rhs.RetryLater = false
	op.issueNewStorageRequests(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.True(op.activeSRs)
	assert.True(op.mustUpdateRHS)
	// check the SR
	assert.NotNil(fc.InSRCArgs)
	assert.NotNil(fso.InTSRsr)
	assert.Equal(fc.InSRCArgs, fso.InTSRsr) // because fc.PassThroughSRCObj is set
	sr := fso.InTSRsr
	assert.Equal([]string{"PROVISION", "ATTACH", "FORMAT", "USE"}, sr.RequestedOperations)
	assert.Equal(vsr.CompleteByTime, sr.CompleteByTime)
	assert.EqualValues(spe00.ProvNodeID, sr.NodeID)
	assert.Equal(*spe00.ProvMinSizeBytes, *sr.MinSizeBytes)
	assert.Equal(*spe00.ProvParcelSizeBytes, *sr.ParcelSizeBytes)
	assert.Equal(vsr.StoragePlan.StorageElements[0].PoolID, sr.PoolID)
	assert.NotEmpty(sr.SystemTags)
	assert.Len(sr.SystemTags, 1)
	assert.Equal(vsrTag, sr.SystemTags[0])
	// check the claim in the SR
	assert.NotNil(sr.VolumeSeriesRequestClaims)
	vse, ok := sr.VolumeSeriesRequestClaims.Claims[string(vsr.Meta.ID)]
	assert.True(ok)
	assert.Equal(spe00.SizeBytes, vse.SizeBytes)
	assert.Equal("STORAGE-0-0", vse.Annotation)
	assert.Equal(spe00.ProvRemainingSizeBytes, sr.VolumeSeriesRequestClaims.RemainingBytes)

	tl.Logger().Info("case: issueNewStorageRequests multiple elements")
	tl.Flush()
	fso.InTSRsr = nil
	vsr = vsrClone(vsrObj)
	assert.Len(vsr.StoragePlan.StorageElements, 3)
	op.rhs.Request = vsr
	op.activeSRs = false
	op.mustUpdateRHS = false
	op.issueNewStorageRequests(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.True(op.activeSRs)
	assert.True(op.mustUpdateRHS)
	// last SR viewable
	assert.NotNil(fc.InSRCArgs)
	assert.NotNil(fso.InTSRsr)
	assert.Equal(fc.InSRCArgs, fso.InTSRsr) // because fc.PassThroughSRCObj is set
	sr = fso.InTSRsr
	assert.Equal([]string{"PROVISION", "ATTACH", "FORMAT", "USE"}, sr.RequestedOperations)
	assert.Equal(vsr.CompleteByTime, sr.CompleteByTime)
	assert.EqualValues(spe11.ProvNodeID, sr.NodeID)
	assert.Equal(*spe11.ProvMinSizeBytes, *sr.MinSizeBytes)
	assert.Equal(*spe11.ProvParcelSizeBytes, *sr.ParcelSizeBytes)
	assert.Equal(vsr.StoragePlan.StorageElements[1].PoolID, sr.PoolID)
	assert.NotEmpty(sr.SystemTags)
	assert.Len(sr.SystemTags, 1)
	assert.Equal(vsrTag, sr.SystemTags[0])
	// check the claim in the SR
	assert.NotNil(sr.VolumeSeriesRequestClaims)
	vse, ok = sr.VolumeSeriesRequestClaims.Claims[string(vsr.Meta.ID)]
	assert.True(ok)
	assert.Equal(spe11.SizeBytes, vse.SizeBytes)
	assert.Equal("STORAGE-1-1", vse.Annotation)
	assert.Equal(spe11.ProvRemainingSizeBytes, sr.VolumeSeriesRequestClaims.RemainingBytes)

	//  ***************************** allocateStorageCapacity
	tl.Logger().Info("case: allocateStorageCapacity storage not in state")
	tl.Flush()
	vsr = vsrClone(vsrObj) // one Storage object
	op.rhs.Request = vsr
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.mustUpdateRHS = false
	op.allocateStorageCapacity(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Storage.*not present", op.rhs.Request.RequestMessages[0].Message)
	assert.False(op.mustUpdateRHS)

	tl.Logger().Info("case: allocateStorageCapacity storage has insufficient capacity")
	tl.Flush()
	fso.Storage["S-1-0"] = &models.Storage{
		StorageMutable: models.StorageMutable{
			AvailableBytes: swag.Int64(0),
			StorageState:   &models.StorageStateMutable{},
		},
	}
	vsr = vsrClone(vsrObj) // one Storage object
	op.rhs.Request = vsr
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.allocateStorageCapacity(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Storage.*Insufficient capacity", op.rhs.Request.RequestMessages[0].Message)
	assert.False(op.mustUpdateRHS)

	tl.Logger().Info("case: allocateStorageCapacity storage update error")
	tl.Flush()
	fso.Storage["S-1-0"] = &models.Storage{
		StorageMutable: models.StorageMutable{
			AvailableBytes: spe10.SizeBytes,
			StorageState:   &models.StorageStateMutable{},
		},
	}
	fc.InUSitems = nil
	fc.RetUSErr = fmt.Errorf("storage-update-error")
	vsr = vsrClone(vsrObj) // one Storage object
	op.rhs.Request = vsr
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.allocateStorageCapacity(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("S-1-0.*storage-update-error", op.rhs.Request.RequestMessages[0].Message)
	assert.NotNil(fc.InUSitems)
	assert.Equal([]string{"availableBytes", "storageState"}, fc.InUSitems.Set)
	assert.Equal([]string{"systemTags"}, fc.InUSitems.Append)
	assert.Empty(fc.InUSitems.Remove)
	assert.False(op.mustUpdateRHS)

	tl.Logger().Info("case: allocateStorageCapacity storage update")
	tl.Flush()
	fc.RetUSErr = nil
	fc.PassThroughUSObj = true
	fc.InUSitems = nil
	fso.Storage["S-1-0"] = &models.Storage{
		StorageMutable: models.StorageMutable{
			AvailableBytes: spe10.SizeBytes,
			StorageState:   &models.StorageStateMutable{},
		},
	}
	vsr = vsrClone(vsrObj) // one Storage object
	assert.NotEmpty(op.rhs.Request.StoragePlan.StorageElements)
	for _, el := range vsr.StoragePlan.StorageElements {
		spe, found := el.StorageParcels["S-1-0"]
		if found {
			spe.ShareableStorage = true
			el.StorageParcels["S-1-0"] = spe
			break
		}
	}
	found := false
	for _, el := range vsr.StoragePlan.StorageElements {
		if spe, present := el.StorageParcels["S-1-0"]; present {
			found = true
			assert.True(spe.ShareableStorage)
		}
	}
	assert.True(found)
	op.rhs.Request = vsr
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.allocateStorageCapacity(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Allocated.*from Storage", op.rhs.Request.RequestMessages[0].Message)
	sObj := fso.Storage["S-1-0"]
	assert.EqualValues(0, swag.Int64Value(sObj.AvailableBytes))
	assert.NotNil(sObj.SystemTags)
	assert.Len(sObj.SystemTags, 1)
	assert.Len(sObj.StorageState.Messages, 1)
	assert.Regexp("Allocated", sObj.StorageState.Messages[0].Message)
	assert.Equal(vsrTag, sObj.SystemTags[0])
	assert.NotNil(fc.InUSitems)
	assert.Equal([]string{"availableBytes", "storageState", "shareableStorage"}, fc.InUSitems.Set)
	assert.Equal([]string{"systemTags"}, fc.InUSitems.Append)
	assert.Empty(fc.InUSitems.Remove)
	assert.True(op.mustUpdateRHS)

	tl.Logger().Info("case: allocateStorageCapacity storage update reentrant")
	tl.Flush()
	op.mustUpdateRHS = false
	op.allocateStorageCapacity(ctx)
	assert.Equal(1, tl.CountPattern("already processed Storage"))
	assert.False(op.mustUpdateRHS)

	//  ***************************** waitForStorageRequests
	tl.Logger().Info("case: waitForStorageRequests no active SRs")
	tl.Flush()
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.activeSRs = false
	assert.Equal("PLACEMENT", vsr.VolumeSeriesRequestState)
	op.waitForStorageRequests(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal("PLACEMENT", vsr.VolumeSeriesRequestState)

	tl.Logger().Info("case: waitForStorageRequests active SRs no mod")
	tl.Flush()
	fc.InUVRObj = nil
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.activeSRs = true
	op.mustUpdateRHS = false
	assert.Equal("PLACEMENT", vsr.VolumeSeriesRequestState)
	op.waitForStorageRequests(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal("STORAGE_WAIT", vsr.VolumeSeriesRequestState)
	assert.Nil(fc.InUVRObj)

	tl.Logger().Info("case: waitForStorageRequests active SRs with mod and update error")
	tl.Flush()
	fc.RetVSRUpdaterObj = nil
	fc.RetVSRUpdaterErr = nil
	fc.RetVSRUpdaterUpdateErr = fmt.Errorf("vsr-update-error")
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.activeSRs = true
	op.mustUpdateRHS = true
	assert.Equal("PLACEMENT", vsr.VolumeSeriesRequestState)
	op.waitForStorageRequests(ctx)
	assert.False(op.rhs.RetryLater)
	assert.True(op.rhs.InError)
	assert.Equal("STORAGE_WAIT", vsr.VolumeSeriesRequestState)
	assert.NotNil(fc.ModVSRUpdaterObj)

	tl.Logger().Info("case: waitForStorageRequests active SRs with mod no update error")
	tl.Flush()
	fc.RetVSRUpdaterObj = nil
	fc.RetVSRUpdaterErr = nil
	fc.RetVSRUpdaterUpdateErr = nil
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.activeSRs = true
	op.mustUpdateRHS = true
	assert.Equal("PLACEMENT", vsr.VolumeSeriesRequestState)
	op.waitForStorageRequests(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal("STORAGE_WAIT", vsr.VolumeSeriesRequestState)
	assert.NotNil(fc.ModVSRUpdaterObj)

	//  ***************************** assignStorageInVolumeSeries
	s00 := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta:   &models.ObjMeta{ID: "S-0-0"},
			PoolID: "SP-1",
		},
		StorageMutable: models.StorageMutable{
			AvailableBytes:   swag.Int64(500 * units.GiB),
			StorageState:     &models.StorageStateMutable{},
			ShareableStorage: true,
			TotalParcelCount: swag.Int64(600),
			ParcelSizeBytes:  swag.Int64(units.GiB),
		},
	}
	s10 := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta:   &models.ObjMeta{ID: "S-1-0"},
			PoolID: "SP-2",
		},
		StorageMutable: models.StorageMutable{
			AvailableBytes:   swag.Int64(500 * units.GiB),
			StorageState:     &models.StorageStateMutable{},
			ShareableStorage: true,
			TotalParcelCount: swag.Int64(600),
			ParcelSizeBytes:  swag.Int64(units.GiB),
		},
	}
	s11 := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta:   &models.ObjMeta{ID: "S-1-1"},
			PoolID: "SP-2",
		},
		StorageMutable: models.StorageMutable{
			AvailableBytes:   swag.Int64(500 * units.GiB),
			StorageState:     &models.StorageStateMutable{},
			ShareableStorage: true,
			TotalParcelCount: swag.Int64(600),
			ParcelSizeBytes:  swag.Int64(units.GiB),
		},
	}
	s20 := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta:   &models.ObjMeta{ID: "S-2-0"},
			PoolID: "SP-1",
		},
		StorageMutable: models.StorageMutable{
			AvailableBytes:   swag.Int64(500 * units.GiB),
			StorageState:     &models.StorageStateMutable{},
			ShareableStorage: true,
			TotalParcelCount: swag.Int64(600),
			ParcelSizeBytes:  swag.Int64(units.GiB),
		},
	}
	sObjects := []*models.Storage{s00, s10, s11, s20}
	fsoResetStorage := func() {
		fso.Storage = make(map[string]*models.Storage)
		for _, sObj := range sObjects {
			fso.Storage[string(sObj.Meta.ID)] = sClone(sObj)
		}
		fso.DetachedStorage = make(map[string]*models.Storage)
	}
	fsoResetStorage()
	spe20 := models.StorageParcelElement{SizeBytes: swag.Int64(int64(90 * units.GiB))}
	vsrObj.StoragePlan = &models.StoragePlan{
		StorageElements: []*models.StoragePlanStorageElement{
			&models.StoragePlanStorageElement{
				Intent:    "DATA",
				SizeBytes: swag.Int64(int64(10 * units.GiB)),
				PoolID:    "SP-1",
				StorageParcels: map[string]models.StorageParcelElement{
					"S-0-0": spe00,
				},
			},
			&models.StoragePlanStorageElement{
				Intent:    "DATA",
				SizeBytes: swag.Int64(int64(50 * units.GiB)),
				PoolID:    "SP-2",
				StorageParcels: map[string]models.StorageParcelElement{
					"S-1-0": spe10,
					"S-1-1": spe11,
				},
			},
			&models.StoragePlanStorageElement{
				Intent:    "DATA",
				SizeBytes: swag.Int64(int64(10 * units.GiB)),
				PoolID:    "SP-1",
				StorageParcels: map[string]models.StorageParcelElement{
					"S-2-0": spe20,
				},
			},
			&models.StoragePlanStorageElement{
				Intent:    "CACHE",
				SizeBytes: swag.Int64(int64(100 * units.MiB)),
				StorageParcels: map[string]models.StorageParcelElement{
					"SSD": models.StorageParcelElement{
						ProvMinSizeBytes: swag.Int64(0),
					},
				},
			},
		},
	}
	vsObj.CapacityAllocations = map[string]models.CapacityAllocation{
		"SP-1": models.CapacityAllocation{
			ReservedBytes: swag.Int64(int64(100 * units.GiB)),
		},
		"SP-2": models.CapacityAllocation{
			ReservedBytes: swag.Int64(int64(100 * units.GiB)),
			ConsumedBytes: swag.Int64(int64(10 * units.GiB)),
		},
	}
	expCapBySP := map[string]int64{
		"SP-1": int64(100 * units.GiB),
		"SP-2": int64(50 * units.GiB),
	}
	expCapByS := map[string]int64{
		"S-0-0": int64(10 * units.GiB),
		"S-1-0": int64(20 * units.GiB),
		"S-1-1": int64(30 * units.GiB),
		"S-2-0": int64(90 * units.GiB),
	}
	expVsSP := map[string]models.ParcelAllocation{
		"S-0-0": models.ParcelAllocation{SizeBytes: swag.Int64(int64(10 * units.GiB))},
		"S-1-0": models.ParcelAllocation{SizeBytes: swag.Int64(int64(20 * units.GiB))},
		"S-1-1": models.ParcelAllocation{SizeBytes: swag.Int64(int64(30 * units.GiB))},
		"S-2-0": models.ParcelAllocation{SizeBytes: swag.Int64(int64(90 * units.GiB))},
	}
	expVsCA := map[string]models.CapacityAllocation{
		"SP-1": models.CapacityAllocation{
			ReservedBytes: swag.Int64(int64(100 * units.GiB)),
			ConsumedBytes: swag.Int64(int64(100 * units.GiB)),
		},
		"SP-2": models.CapacityAllocation{
			ReservedBytes: swag.Int64(int64(100 * units.GiB)),
			ConsumedBytes: swag.Int64(int64(60 * units.GiB)),
		},
	}

	tl.Logger().Info("case: assignStorageInVolumeSeries REI block")
	c.rei.SetProperty("placement-block-on-vs-update", &rei.Property{StringValue: string(vsObj.Meta.ID)})
	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	op.rhs.VolumeSeries = vs
	op.rhs.Request = vsr
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.assignStorageInVolumeSeries(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	tl.Logger().Info("case: assignStorageInVolumeSeries update error")
	tl.Flush()
	fc.RetVSUpdaterErr = fmt.Errorf("vs-update-err")
	fc.InVSUpdaterItems = nil
	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	op.rhs.VolumeSeries = vs
	op.rhs.Request = vsr
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.assignStorageInVolumeSeries(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(expCapBySP, op.capByP)
	assert.Equal(expCapByS, op.capByS)

	tl.Logger().Info("case: assignStorageInVolumeSeries (no previous parcels)")
	tl.Flush()
	fc.RetVSUpdaterErr = nil
	fc.InVSUpdaterItems = nil
	vs = vsClone(vsObj)
	vs.LifecycleManagementData = nil
	vs.VolumeSeriesState = com.VolStateBound
	vsr = vsrClone(vsrObj)
	vsr.StoragePlan.LayoutAlgorithm = laSNS.Name
	op.rhs.VolumeSeries = vs
	op.rhs.Request = vsr
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.capByS = nil
	op.capByP = nil
	op.assignStorageInVolumeSeries(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(com.VolStateProvisioned, op.rhs.VolumeSeries.VolumeSeriesState)
	assert.Equal(expCapBySP, op.capByP)
	assert.Equal(expCapByS, op.capByS)
	assert.EqualValues(expVsSP, op.rhs.VolumeSeries.StorageParcels)
	assert.EqualValues(expVsCA, op.rhs.VolumeSeries.CapacityAllocations)
	assert.EqualValues([]string{op.vsrTag}, op.rhs.VolumeSeries.SystemTags)
	assert.Equal(&models.LifecycleManagementData{LayoutAlgorithm: laSNS.Name}, op.rhs.VolumeSeries.LifecycleManagementData)
	assert.Equal([]string{"capacityAllocations", "storageParcels", "messages", "lifecycleManagementData", "volumeSeriesState"}, fc.InVSUpdaterItems.Set)
	assert.Equal([]string{"systemTags"}, fc.InVSUpdaterItems.Append)
	assert.Nil(fc.InVSUpdaterItems.Remove)
	capAddedMsg := "VolumeSeries capacity added:\n" +
		" capacityAllocations:\n" +
		"  - SP[SP-1] consumedBytes=100GiB (+100GiB)\n" +
		"  - SP[SP-2] consumedBytes=60GiB (+50GiB)\n" +
		" storageParcels:\n" +
		"  - S[S-0-0] sizeBytes=10GiB (+10GiB) SP[SP-1]\n" +
		"  - S[S-1-0] sizeBytes=20GiB (+20GiB) SP[SP-2]\n" +
		"  - S[S-1-1] sizeBytes=30GiB (+30GiB) SP[SP-2]\n" +
		"  - S[S-2-0] sizeBytes=90GiB (+90GiB) SP[SP-1]"
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Equal(capAddedMsg, op.rhs.Request.RequestMessages[0].Message)
	assert.Len(vs.Messages, 2)
	assert.Equal(capAddedMsg, vs.Messages[0].Message)
	assert.Regexp(com.VolStateProvisioned, vs.Messages[1].Message)

	tl.Logger().Info("case: assignStorageInVolumeSeries (with previous parcels)")
	tl.Flush()
	fc.InVSUpdaterItems = nil
	vsObj.StorageParcels = map[string]models.ParcelAllocation{
		"S-1-0": models.ParcelAllocation{SizeBytes: swag.Int64(int64(100 * units.GiB))},
	}
	expVsSP["S-1-0"] = models.ParcelAllocation{SizeBytes: swag.Int64(int64(120 * units.GiB))}
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = com.VolStateConfigured // isProvisioned
	vsr = vsrClone(vsrObj)
	op.rhs.VolumeSeries = vs
	op.rhs.Request = vsr
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.capByS = nil
	op.capByP = nil
	op.assignStorageInVolumeSeries(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal([]string{"capacityAllocations", "storageParcels", "messages", "lifecycleManagementData", "volumeSeriesState"}, fc.InVSUpdaterItems.Set)
	assert.Equal([]string{"systemTags"}, fc.InVSUpdaterItems.Append)
	assert.Equal(com.VolStateConfigured, op.rhs.VolumeSeries.VolumeSeriesState)
	assert.Equal(expCapBySP, op.capByP)
	assert.Equal(expCapByS, op.capByS)
	assert.EqualValues(expVsSP, op.rhs.VolumeSeries.StorageParcels)
	assert.EqualValues(expVsCA, op.rhs.VolumeSeries.CapacityAllocations)
	assert.EqualValues([]string{op.vsrTag}, op.rhs.VolumeSeries.SystemTags)
	capAddedMsg = "VolumeSeries capacity added:\n" +
		" capacityAllocations:\n" +
		"  - SP[SP-1] consumedBytes=100GiB (+100GiB)\n" +
		"  - SP[SP-2] consumedBytes=60GiB (+50GiB)\n" +
		" storageParcels:\n" +
		"  - S[S-0-0] sizeBytes=10GiB (+10GiB) SP[SP-1]\n" +
		"  - S[S-1-0] sizeBytes=120GiB (+20GiB) SP[SP-2]\n" +
		"  - S[S-1-1] sizeBytes=30GiB (+30GiB) SP[SP-2]\n" +
		"  - S[S-2-0] sizeBytes=90GiB (+90GiB) SP[SP-1]"
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Equal(capAddedMsg, op.rhs.Request.RequestMessages[0].Message)
	assert.Len(vs.Messages, 1)
	assert.Equal(capAddedMsg, vs.Messages[0].Message)

	tl.Logger().Info("case: assignStorageInVolumeSeries reentrant")
	tl.Flush()
	fc.InVSUpdaterItems = nil
	fc.RetVSUpdaterErr = fmt.Errorf("vs-update-err")
	op.capByS = nil
	op.capByP = nil
	op.assignStorageInVolumeSeries(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Nil(op.capByS)
	assert.Nil(op.capByP)
	assert.Nil(fc.InVSUpdaterItems)

	//  ***************************** removeStorageFromVolumeSeries
	// Note: vsrObj contains an updated storagePlan with provisioned storage.
	vsObj.SystemTags = []string{vsrTag}
	fso.CallRealLookupStorage = true

	tl.Logger().Info("case: removeStorageFromVolumeSeries update error")
	tl.Flush()
	fc.RetVSUpdaterErr = fmt.Errorf("vs-update-err")
	fc.InVSUpdaterItems = nil
	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	op.rhs.VolumeSeries = vs
	op.rhs.Request = vsr
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.capByS = nil
	op.capByP = nil
	op.removeStorageFromVolumeSeries(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(expCapBySP, op.capByP)
	assert.Equal(expCapByS, op.capByS)
	assert.Equal([]string{"capacityAllocations", "storageParcels", "messages", "volumeSeriesState", "lifecycleManagementData"}, fc.InVSUpdaterItems.Set)
	assert.Equal([]string{"systemTags"}, fc.InVSUpdaterItems.Remove)
	assert.Nil(fc.InVSUpdaterItems.Append)

	tl.Logger().Info("case: removeStorageFromVolumeSeries (with previous parcels)")
	tl.Flush()
	vsObj.CapacityAllocations = map[string]models.CapacityAllocation{
		"SP-1": models.CapacityAllocation{
			ReservedBytes: swag.Int64(int64(100 * units.GiB)),
			ConsumedBytes: swag.Int64(int64(100 * units.GiB)),
		},
		"SP-2": models.CapacityAllocation{
			ReservedBytes: swag.Int64(int64(100 * units.GiB)),
			ConsumedBytes: swag.Int64(int64(60 * units.GiB)),
		},
	}
	vsObj.StorageParcels = map[string]models.ParcelAllocation{
		"S-0-0": models.ParcelAllocation{SizeBytes: swag.Int64(int64(10 * units.GiB))},
		"S-1-0": models.ParcelAllocation{SizeBytes: swag.Int64(int64(120 * units.GiB))},
		"S-1-1": models.ParcelAllocation{SizeBytes: swag.Int64(int64(30 * units.GiB))},
		"S-2-0": models.ParcelAllocation{SizeBytes: swag.Int64(int64(90 * units.GiB))},
	}
	vsAfterUndo := vsClone(vsObj)
	vsAfterUndo.SystemTags = []string{}
	vsAfterUndo.CapacityAllocations = map[string]models.CapacityAllocation{
		"SP-1": models.CapacityAllocation{
			ReservedBytes: swag.Int64(int64(100 * units.GiB)),
		},
		"SP-2": models.CapacityAllocation{
			ReservedBytes: swag.Int64(int64(100 * units.GiB)),
			ConsumedBytes: swag.Int64(int64(10 * units.GiB)),
		},
	}
	vsAfterUndo.StorageParcels = map[string]models.ParcelAllocation{
		"S-1-0": models.ParcelAllocation{SizeBytes: swag.Int64(int64(100 * units.GiB))},
	}
	fc.RetVSUpdaterErr = nil
	fc.InVSUpdaterItems = nil
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = "BOUND"
	vs.LifecycleManagementData = &models.LifecycleManagementData{LayoutAlgorithm: laSNS.Name}
	vsr = vsrClone(vsrObj)
	op.rhs.VolumeSeries = vs
	op.rhs.Request = vsr
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.capByS = nil
	op.capByP = nil
	op.removeStorageFromVolumeSeries(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(expCapBySP, op.capByP)
	assert.Equal(expCapByS, op.capByS)
	assert.Equal(vsAfterUndo.CapacityAllocations, op.rhs.VolumeSeries.CapacityAllocations)
	assert.Equal(vsAfterUndo.StorageParcels, op.rhs.VolumeSeries.StorageParcels)
	capRmMsg := "VolumeSeries capacity removed:\n" +
		" capacityAllocations:\n" +
		"  - SP[SP-1] consumedBytes=0B (-100GiB)\n" +
		"  - SP[SP-2] consumedBytes=10GiB (-50GiB)\n" +
		" storageParcels:\n" +
		"  - S[S-0-0] sizeBytes=0B (-10GiB) SP[SP-1]\n" +
		"  - S[S-1-0] sizeBytes=100GiB (-20GiB) SP[SP-2]\n" +
		"  - S[S-1-1] sizeBytes=0B (-30GiB) SP[SP-2]\n" +
		"  - S[S-2-0] sizeBytes=0B (-90GiB) SP[SP-1]"
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Equal(capRmMsg, op.rhs.Request.RequestMessages[0].Message)
	assert.Len(vs.Messages, 1)
	assert.Equal(capRmMsg, vs.Messages[0].Message)
	assert.Equal("", fc.ModVSUpdaterObj.LifecycleManagementData.LayoutAlgorithm)

	tl.Logger().Info("case: removeStorageFromVolumeSeries (no previous parcels)")
	tl.Flush()
	vsObj.CapacityAllocations = map[string]models.CapacityAllocation{
		"SP-1": models.CapacityAllocation{
			ReservedBytes: swag.Int64(int64(100 * units.GiB)),
			ConsumedBytes: swag.Int64(int64(100 * units.GiB)),
		},
		"SP-2": models.CapacityAllocation{
			ReservedBytes: swag.Int64(int64(100 * units.GiB)),
			ConsumedBytes: swag.Int64(int64(60 * units.GiB)),
		},
	}
	vsObj.StorageParcels = map[string]models.ParcelAllocation{
		"S-0-0": models.ParcelAllocation{SizeBytes: swag.Int64(int64(10 * units.GiB))},
		"S-1-0": models.ParcelAllocation{SizeBytes: swag.Int64(int64(20 * units.GiB))},
		"S-1-1": models.ParcelAllocation{SizeBytes: swag.Int64(int64(30 * units.GiB))},
		"S-2-0": models.ParcelAllocation{SizeBytes: swag.Int64(int64(90 * units.GiB))},
	}
	vsAfterUndo = vsClone(vsObj)
	vsAfterUndo.SystemTags = []string{}
	vsAfterUndo.CapacityAllocations = map[string]models.CapacityAllocation{
		"SP-1": models.CapacityAllocation{
			ReservedBytes: swag.Int64(int64(100 * units.GiB)),
		},
		"SP-2": models.CapacityAllocation{
			ReservedBytes: swag.Int64(int64(100 * units.GiB)),
			ConsumedBytes: swag.Int64(int64(10 * units.GiB)),
		},
	}
	vsAfterUndo.StorageParcels = map[string]models.ParcelAllocation{}
	fc.InVSUpdaterItems = nil
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = "CONFIGURED"
	vsr = vsrClone(vsrObj)
	op.rhs.VolumeSeries = vs
	op.rhs.Request = vsr
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.capByS = nil
	op.capByP = nil
	op.removeStorageFromVolumeSeries(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(expCapBySP, op.capByP)
	assert.Equal(expCapByS, op.capByS)
	assert.Equal(vsAfterUndo.CapacityAllocations, op.rhs.VolumeSeries.CapacityAllocations)
	assert.Equal(vsAfterUndo.StorageParcels, op.rhs.VolumeSeries.StorageParcels)
	capRmMsg = "VolumeSeries capacity removed:\n" +
		" capacityAllocations:\n" +
		"  - SP[SP-1] consumedBytes=0B (-100GiB)\n" +
		"  - SP[SP-2] consumedBytes=10GiB (-50GiB)\n" +
		" storageParcels:\n" +
		"  - S[S-0-0] sizeBytes=0B (-10GiB) SP[SP-1]\n" +
		"  - S[S-1-0] sizeBytes=0B (-20GiB) SP[SP-2]\n" +
		"  - S[S-1-1] sizeBytes=0B (-30GiB) SP[SP-2]\n" +
		"  - S[S-2-0] sizeBytes=0B (-90GiB) SP[SP-1]"
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Equal(capRmMsg, op.rhs.Request.RequestMessages[0].Message)
	assert.Len(vs.Messages, 2)
	assert.Equal(capRmMsg, vs.Messages[0].Message)
	assert.Equal("State change CONFIGURED ⇒ BOUND", vs.Messages[1].Message)

	tl.Logger().Info("case: removeStorageFromVolumeSeries reentrant")
	tl.Flush()
	fc.InVSUpdaterItems = nil
	fc.RetVSUpdaterErr = fmt.Errorf("vs-update-err")
	vs = vsClone(vsObj)
	vs.SystemTags = []string{}
	vsr = vsrClone(vsrObj)
	op.rhs.VolumeSeries = vs
	op.rhs.Request = vsr
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.capByS = nil
	op.capByP = nil
	assert.Empty(vs.SystemTags)
	op.removeStorageFromVolumeSeries(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Nil(op.capByS)
	assert.Nil(op.capByP)
	assert.Nil(fc.InVSUpdaterItems)
	fso.CallRealLookupStorage = false

	//  ***************************** releaseStorageCapacity
	tl.Logger().Info("case: releaseStorageCapacity storage not in state")
	tl.Flush()
	fso.CallRealLookupStorage = true
	fsoResetStorage()
	sCnt := 0
	for _, el := range vsrObj.StoragePlan.StorageElements {
		if el.Intent == com.VolReqStgElemIntentCache {
			continue
		}
		for k := range el.StorageParcels {
			if !strings.HasPrefix(k, com.VolReqStgPseudoParcelKey) {
				sCnt++
			}
		}
	}
	assert.Equal(4, sCnt)
	sStash := fso.Storage
	fso.Storage = map[string]*models.Storage{}
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.mustUpdateRHS = false
	op.releaseStorageCapacity(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.False(op.mustUpdateRHS)
	assert.Equal(sCnt, tl.CountPattern("Storage.*not present"))

	tl.Logger().Info("case: releaseStorageCapacity storage already processed")
	tl.Flush()
	fso.Storage = sStash
	for _, sObj := range fso.Storage {
		sObj.SystemTags = []string{}
	}
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.mustUpdateRHS = false
	op.releaseStorageCapacity(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.False(op.mustUpdateRHS)
	assert.Equal(sCnt, tl.CountPattern("already processed Storage"))

	tl.Logger().Info("case: releaseStorageCapacity storage update error")
	tl.Flush()
	fsoResetStorage()
	for _, sObj := range fso.Storage {
		sObj.SystemTags = []string{vsrTag}
	}
	fc.PassThroughUSObj = false
	fc.RetUSErr = fmt.Errorf("storage-update-error")
	fc.InUSitems = nil
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.mustUpdateRHS = false
	op.releaseStorageCapacity(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.False(op.mustUpdateRHS)
	assert.Regexp("S-0-0.*storage-update-error", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal([]string{"availableBytes", "storageState"}, fc.InUSitems.Set)
	assert.Equal([]string{"systemTags"}, fc.InUSitems.Remove)
	assert.Empty(fc.InUSitems.Append)
	assert.NotNil(fc.InUSObj)
	assert.Len(fc.InUSObj.StorageState.Messages, 1)
	assert.Regexp("Returned", fc.InUSObj.StorageState.Messages[0].Message)

	tl.Logger().Info("case: releaseStorageCapacity storage update (shareable)")
	tl.Flush()
	fsoResetStorage()
	sSz := map[string]int64{}
	for k, sObj := range fso.Storage {
		sObj.SystemTags = []string{vsrTag}
		sObj.ShareableStorage = true
		sSz[k] = swag.Int64Value(sObj.AvailableBytes)
	}
	assert.Empty(fso.DetachedStorage)
	for k, v := range fso.Storage { // move 1 to Detached
		fso.DetachedStorage[k] = v
		delete(fso.Storage, k)
		break
	}
	fc.PassThroughUSObj = true
	fc.RetUSErr = nil
	fc.InUSitems = nil
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.mustUpdateRHS = false
	op.releaseStorageCapacity(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 4)
	assert.True(op.mustUpdateRHS)
	for k, sObj := range fso.Storage {
		assert.Truef(swag.Int64Value(sObj.AvailableBytes) > sSz[k], "case: [%s] %d %d", k, swag.Int64Value(sObj.AvailableBytes), sSz[k])
	}
	for k, sObj := range fso.DetachedStorage {
		assert.Truef(swag.Int64Value(sObj.AvailableBytes) > sSz[k], "case: [%s] %d %d", k, swag.Int64Value(sObj.AvailableBytes), sSz[k])
	}

	tl.Logger().Info("case: releaseStorageCapacity storage update (not shareable)")
	tl.Flush()
	fsoResetStorage()
	for k, sObj := range fso.Storage {
		sObj.SystemTags = []string{vsrTag}
		sObj.ShareableStorage = false
		sSz[k] = swag.Int64Value(sObj.TotalParcelCount) * swag.Int64Value(sObj.ParcelSizeBytes)
	}
	fc.PassThroughUSObj = true
	fc.RetUSErr = nil
	fc.InUSitems = nil
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.mustUpdateRHS = false
	op.releaseStorageCapacity(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Len(op.rhs.Request.RequestMessages, 4)
	assert.True(op.mustUpdateRHS)
	for k, sObj := range fso.Storage {
		assert.Truef(swag.Int64Value(sObj.AvailableBytes) == sSz[k], "case: [%s] %d %d", k, swag.Int64Value(sObj.AvailableBytes), sSz[k])
	}
	fso.CallRealLookupStorage = false

	//  ***************************** removeClaims
	tl.Logger().Info("case: removeClaims")
	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	op.rhs.InError = false
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.removeClaims(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Equal(op.rID, fso.InRCReq)
	tl.Flush()

	// /////////////////////////////////////////////////////////////////
	// REATTACH (only) steps
	// /////////////////////////////////////////////////////////////////
	raS1 := &models.Storage{}
	raS1.Meta = &models.ObjMeta{ID: "RA-S1"}
	raS1.StorageState = &models.StorageStateMutable{AttachedNodeID: "NODE-1"}
	raS1.PoolID = "POOL-1"
	raS2 := &models.Storage{}
	raS2.Meta = &models.ObjMeta{ID: "RA-S2"}
	raS2.StorageState = &models.StorageStateMutable{AttachedNodeID: "NODE-2"}
	raS2.PoolID = "POOL-2"

	makeRaStoragePlan := func() *models.StoragePlan {
		sp := &models.StoragePlan{
			StorageElements: []*models.StoragePlanStorageElement{
				&models.StoragePlanStorageElement{
					Intent: "CACHE",
				},
			},
		}
		for _, sObj := range []*models.Storage{raS1, raS2} {
			se := &models.StoragePlanStorageElement{
				Intent:         com.VolReqStgElemIntentData,
				PoolID:         sObj.PoolID,
				StorageParcels: map[string]models.StorageParcelElement{},
			}
			sp.StorageElements = append(sp.StorageElements, se)
			se.StorageParcels[string(sObj.Meta.ID)] = models.StorageParcelElement{
				ProvNodeID: string(sObj.StorageState.AttachedNodeID),
			}
		}
		for i, se := range sp.StorageElements {
			t.Logf("StoragePlan[%d]: %#v", i, se)
		}
		return sp
	}
	raStoragePlan := makeRaStoragePlan()

	//  ***************************** issueNewReattachStorageRequests

	// fail to create StorageRequest
	t.Log("issueNewReattachStorageRequests: case fail to create storage request")
	fc.PassThroughSRCObj = false
	fc.InSRCArgs = nil
	fc.RetSRCErr = fmt.Errorf("storage-request-create")
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = "PROVISIONED"
	vs.LifecycleManagementData = &models.LifecycleManagementData{LayoutAlgorithm: laSLU.Name}
	vs.StorageParcels = map[string]models.ParcelAllocation{}
	vs.StorageParcels["RA-S1"] = models.ParcelAllocation{}
	vs.StorageParcels["RA-S2"] = models.ParcelAllocation{}
	vsr = vsrClone(vsrObj)
	vsr.StoragePlan = raStoragePlan
	vsr.NodeID = raS1.StorageState.AttachedNodeID
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.rhs.RetryLater = false
	op.rhs.InError = false
	op.activeSRs = false
	op.mustUpdateRHS = false
	op.issueNewReattachStorageRequests(ctx)
	tl.Flush()
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.False(op.activeSRs)
	assert.False(op.mustUpdateRHS)
	assert.NotNil(fc.InSRCArgs)
	assert.EqualValues(raS2.Meta.ID, fc.InSRCArgs.StorageID) // RA-S1 skipped
	assert.Len(op.rhs.Request.RequestMessages, 2)
	assert.Regexp("Reattaching Storage", op.rhs.Request.RequestMessages[0].Message)
	assert.Regexp("StorageRequest error", op.rhs.Request.RequestMessages[1].Message)

	// success
	t.Log("issueNewReattachStorageRequests: create storage request")
	fc.PassThroughSRCObj = true
	fc.InSRCArgs = nil
	fc.RetSRCErr = nil
	fso.InTSRsr = nil
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = "PROVISIONED"
	vs.LifecycleManagementData = &models.LifecycleManagementData{LayoutAlgorithm: laSLU.Name}
	vs.StorageParcels = map[string]models.ParcelAllocation{}
	vs.StorageParcels["RA-S1"] = models.ParcelAllocation{}
	vs.StorageParcels["RA-S2"] = models.ParcelAllocation{}
	vsr = vsrClone(vsrObj)
	vsr.StoragePlan = raStoragePlan
	vsr.NodeID = raS1.StorageState.AttachedNodeID
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.rhs.RetryLater = false
	op.rhs.InError = false
	op.activeSRs = false
	op.mustUpdateRHS = false
	op.issueNewReattachStorageRequests(ctx)
	tl.Flush()
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.True(op.activeSRs)
	assert.True(op.mustUpdateRHS)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Reattaching Storage", op.rhs.Request.RequestMessages[0].Message)
	assert.NotNil(fc.InSRCArgs)
	assert.EqualValues(raS2.Meta.ID, fc.InSRCArgs.StorageID) // RA-S1 skipped
	assert.NotNil(fso.InTSRsr)
	assert.Equal(op.rhs.Request.CompleteByTime, fso.InTSRsr.CompleteByTime)
	assert.Equal([]string{com.StgReqOpReattach}, fso.InTSRsr.RequestedOperations)
	assert.Equal(op.rhs.Request.NodeID, fso.InTSRsr.ReattachNodeID)
	assert.EqualValues(raS2.Meta.ID, fso.InTSRsr.StorageID)
	assert.EqualValues([]string{op.vsrTag}, fso.InTSRsr.SystemTags)
	assert.NotNil(fso.InTSRsr.VolumeSeriesRequestClaims)
	assert.Nil(fso.InTSRsr.VolumeSeriesRequestClaims.RemainingBytes)
	assert.NotEmpty(fso.InTSRsr.VolumeSeriesRequestClaims.Claims)
	assert.Contains(fso.InTSRsr.VolumeSeriesRequestClaims.Claims, op.rID)
	assert.EqualValues(raS2.Meta.ID, fso.InTSRsr.VolumeSeriesRequestClaims.Claims[op.rID].Annotation)

	//  ***************************** populateReattachPlan

	// fail: storage not found in either of attached or detached caches
	t.Log("populateReattachPlan case: storage not in attached or detached caches")
	fso.CallRealLookupStorage = true
	fso.Storage = nil
	fso.DetachedStorage = nil
	fso.InLSid = ""
	fso.RetLSObj = nil
	assert.NotEmpty(app.ClusterID)
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = "PROVISIONED"
	vs.LifecycleManagementData = &models.LifecycleManagementData{LayoutAlgorithm: laSLU.Name}
	vs.StorageParcels = map[string]models.ParcelAllocation{}
	vs.StorageParcels["RA-S1"] = models.ParcelAllocation{} // one element only to avoid key order issues
	vsr = vsrClone(vsrObj)
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.rhs.RetryLater = false
	op.rhs.InError = false
	op.mustUpdateStoragePlan = false
	op.populateReattachPlan(ctx)
	tl.Flush()
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.False(op.mustUpdateStoragePlan)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Storage.*not accessible", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal("RA-S1", fso.InLSid)

	// succeed
	t.Log("populateReattachPlan case: success (one in cache, one attachable)")
	fso.CallRealLookupStorage = true
	fso.Storage = map[string]*models.Storage{}
	fso.DetachedStorage = map[string]*models.Storage{}
	fso.Storage["RA-S1"] = raS1
	fso.DetachedStorage["RA-S2"] = sClone(raS2)
	func() {
		sObj := fso.DetachedStorage["RA-S2"]
		sObj.StorageState = &models.StorageStateMutable{}
		sObj.StorageState.AttachmentState = com.StgAttachmentStateDetached
	}()
	assert.True(fso == op.c.App.StateOps)
	assert.Len(op.c.App.StateOps.CS().Storage, 1)
	assert.Len(op.c.App.StateOps.CS().DetachedStorage, 1)
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = "PROVISIONED"
	vs.LifecycleManagementData = &models.LifecycleManagementData{LayoutAlgorithm: laSLU.Name}
	vs.StorageParcels = map[string]models.ParcelAllocation{}
	vs.StorageParcels["RA-S1"] = models.ParcelAllocation{}
	vs.StorageParcels["RA-S2"] = models.ParcelAllocation{}
	vsr = vsrClone(vsrObj)
	vsr.StoragePlan = &models.StoragePlan{
		StorageElements: []*models.StoragePlanStorageElement{
			&models.StoragePlanStorageElement{
				Intent: "CACHE",
			},
		},
	}
	op.rhs.Request = vsr
	op.rhs.VolumeSeries = vs
	op.rhs.RetryLater = false
	op.rhs.InError = false
	op.mustUpdateStoragePlan = false
	op.populateReattachPlan(ctx)
	tl.Flush()
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.True(op.mustUpdateStoragePlan)
	assert.Len(vsr.StoragePlan.StorageElements, 3)
	assert.Equal("CACHE", vsr.StoragePlan.StorageElements[0].Intent)
	for _, s := range []*models.Storage{raS1, raS2} {
		found := false
		for _, se := range vsr.StoragePlan.StorageElements {
			if spe, has := se.StorageParcels[string(s.Meta.ID)]; has {
				found = true
				expNodeID := ""
				if string(s.Meta.ID) == "RA-S1" {
					expNodeID = string(s.StorageState.AttachedNodeID)
				}
				assert.Equal(expNodeID, spe.ProvNodeID, "spe[%s]", s.Meta.ID)
				assert.Equal("DATA", se.Intent)
				assert.Equal(s.PoolID, se.PoolID)
				break
			}
		}
		assert.Truef(found, "%s", s.Meta.ID)
	}
	fso.CallRealLookupStorage = false

	//  ***************************** handleCompletedReattachStorageRequests

	raStoragePlan = makeRaStoragePlan()
	raSR := &models.StorageRequest{}
	raSR.Meta = &models.ObjMeta{ID: "RA-S2-SR"}
	raClD := []*state.ClaimData{
		{StorageRequest: raSR, VSRClaims: []state.VSRClaim{{RequestID: op.rID, Annotation: string(raS2.Meta.ID)}}},
	}
	testutils.Clone(raClD, &op.claims)
	raVC, raCD := op.claims.FindVSRClaimByAnnotation(op.rID, string(raS2.Meta.ID))
	assert.NotNil(raVC)
	assert.NotNil(raCD)
	assert.Equal(raSR, raCD.StorageRequest)
	raVC, raCD = op.claims.FindVSRClaimByAnnotation(op.rID, string(raS1.Meta.ID))
	assert.Nil(raVC)
	assert.Nil(raCD)

	t.Log("handleCompletedReattachStorageRequests case: SR active")
	testutils.Clone(raClD, &op.claims)
	op.claims[0].StorageRequest.StorageRequestState = "ATTACHING" // active
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = "PROVISIONED"
	vs.LifecycleManagementData = &models.LifecycleManagementData{LayoutAlgorithm: laSLU.Name}
	vs.StorageParcels = map[string]models.ParcelAllocation{}
	vs.StorageParcels["RA-S1"] = models.ParcelAllocation{}
	vs.StorageParcels["RA-S2"] = models.ParcelAllocation{}
	op.rhs.VolumeSeries = vs
	vsr = vsrClone(vsrObj)
	vsr.StoragePlan = makeRaStoragePlan() // a copy
	vsr.NodeID = raS1.StorageState.AttachedNodeID
	op.rhs.Request = vsr
	op.rhs.RetryLater = false
	op.rhs.InError = false
	op.activeSRs = false
	op.mustUpdateStoragePlan = false
	op.handleCompletedReattachStorageRequests(ctx)
	tl.Flush()
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.True(op.activeSRs)
	assert.False(op.mustUpdateStoragePlan)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.Equal(raStoragePlan, vsr.StoragePlan) // unchanged

	t.Log("handleCompletedReattachStorageRequests case: SR failed")
	testutils.Clone(raClD, &op.claims)
	op.claims[0].StorageRequest.StorageRequestState = "FAILED"
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = "PROVISIONED"
	vs.LifecycleManagementData = &models.LifecycleManagementData{LayoutAlgorithm: laSLU.Name}
	vs.StorageParcels = map[string]models.ParcelAllocation{}
	vs.StorageParcels["RA-S1"] = models.ParcelAllocation{}
	vs.StorageParcels["RA-S2"] = models.ParcelAllocation{}
	op.rhs.VolumeSeries = vs
	vsr = vsrClone(vsrObj)
	vsr.StoragePlan = makeRaStoragePlan() // a copy
	vsr.NodeID = raS1.StorageState.AttachedNodeID
	op.rhs.Request = vsr
	op.rhs.RetryLater = false
	op.rhs.InError = false
	op.activeSRs = false
	op.mustUpdateStoragePlan = false
	op.handleCompletedReattachStorageRequests(ctx)
	tl.Flush()
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.False(op.activeSRs)
	assert.False(op.mustUpdateStoragePlan)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("REATTACH.*failed", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal(raStoragePlan, vsr.StoragePlan) // unchanged

	t.Log("handleCompletedReattachStorageRequests case: SR succeeded")
	testutils.Clone(raClD, &op.claims)
	op.claims[0].StorageRequest.StorageRequestState = "SUCCEEDED"
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = "PROVISIONED"
	vs.LifecycleManagementData = &models.LifecycleManagementData{LayoutAlgorithm: laSLU.Name}
	vs.StorageParcels = map[string]models.ParcelAllocation{}
	vs.StorageParcels["RA-S1"] = models.ParcelAllocation{}
	vs.StorageParcels["RA-S2"] = models.ParcelAllocation{}
	op.rhs.VolumeSeries = vs
	vsr = vsrClone(vsrObj)
	vsr.StoragePlan = makeRaStoragePlan() // a copy
	vsr.NodeID = raS1.StorageState.AttachedNodeID
	op.rhs.Request = vsr
	op.rhs.RetryLater = false
	op.rhs.InError = false
	op.activeSRs = false
	op.mustUpdateStoragePlan = false
	op.handleCompletedReattachStorageRequests(ctx)
	tl.Flush()
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.False(op.activeSRs)
	assert.True(op.mustUpdateStoragePlan)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Reattached Storage", op.rhs.Request.RequestMessages[0].Message)
	assert.NotEqual(raStoragePlan, vsr.StoragePlan) // modified
	seS2 := raStoragePlan.StorageElements[2].StorageParcels[string(raS2.Meta.ID)]
	assert.EqualValues(raS2.StorageState.AttachedNodeID, seS2.ProvNodeID)
	seS2.ProvNodeID = string(op.rhs.Request.NodeID)
	seS2New := vsr.StoragePlan.StorageElements[2].StorageParcels[string(raS2.Meta.ID)]
	assert.Equal(seS2, seS2New)
}

func TestPlace(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	app := &clusterd.AppCtx{
		AppArgs: clusterd.AppArgs{
			Log: tl.Logger(),
		},
	}
	evM := fev.NewFakeEventManager()
	app.CrudeOps = evM
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	app.CSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	app.StateGuard = util.NewCriticalSectionGuard()
	assert.NotNil(app.StateGuard)
	fso := fakeState.NewFakeClusterState()
	app.StateOps = fso

	c := newComponent()
	c.Init(app)
	assert.NotNil(c.Log)
	assert.Equal(app.Log, c.Log)

	vr := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VR-1",
			},
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "PLACEMENT",
				StoragePlan: &models.StoragePlan{
					StorageElements: []*models.StoragePlanStorageElement{
						&models.StoragePlanStorageElement{
							SizeBytes: swag.Int64(int64(10 * units.GiB)),
							PoolID:    "SP-1",
							StorageParcels: map[string]models.StorageParcelElement{
								"STORAGE": models.StorageParcelElement{
									SizeBytes: swag.Int64(int64(10 * units.GiB)),
								},
							},
						},
					},
				},
			},
		},
	}

	// test PLACE transition sequences
	op := &fakePlaceOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.rhs.Request.PlanOnly = swag.Bool(false)
	op.retGIS = PlaceDone
	expCalled := []string{"GIS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)
	op.rhs.Request.PlanOnly = nil

	op = &fakePlaceOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.rhs.Request.PlanOnly = swag.Bool(true)
	op.retGIS = PlaceStartPlanning
	expCalled = []string{"GIS", "WFL", "SS", "PP", "UP"}
	op.run(nil)
	assert.Equal(expCalled, op.called)
	op.rhs.Request.PlanOnly = nil

	op = &fakePlaceOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = PlaceStartPlanning
	op.upCancelCountDown = 1
	expCalled = []string{"GIS", "WFL", "SS", "PP", "UP-C"}
	assert.False(op.rhs.Canceling)
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.True(op.rhs.Canceling)

	op = &fakePlaceOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = PlaceStartPlanning
	expCalled = []string{"GIS", "WFL", "SS", "PP", "UP", "FC", "CSR", "UP", "ESR", "NSR", "ASC", "WSR", "ASV"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakePlaceOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = PlaceStartPlanning
	op.setRestartESR = true
	expCalled = []string{"GIS", "WFL", "SS", "PP", "UP", "FC", "CSR", "UP", "ESR", "RSP"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakePlaceOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = PlaceStartPlanning
	op.upCancelCountDown = 2
	expCalled = []string{"GIS", "WFL", "SS", "PP", "UP", "FC", "CSR", "UP-C"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakePlaceOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = PlaceStartExecuting
	expCalled = []string{"GIS", "WFL", "FC", "CSR", "UP", "ESR", "NSR", "ASC", "WSR", "ASV"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakePlaceOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = PlaceStartExecuting
	op.setRestartESR = true
	expCalled = []string{"GIS", "WFL", "FC", "CSR", "UP", "ESR", "RSP"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakePlaceOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = PlaceStartExecuting
	op.upCancelCountDown = 1
	expCalled = []string{"GIS", "WFL", "FC", "CSR", "UP-C"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakePlaceOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = PlaceUndoStart
	op.upCancelCountDown = 1
	expCalled = []string{"GIS", "WFL", "RSV", "RSC", "RC"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakePlaceOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = PlaceDeleteStart
	expCalled = []string{"GIS", "WFL", "TO", "PDP", "UP", "RSV", "RSC", "RC"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakePlaceOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.rhs.Request.PlanOnly = swag.Bool(true)
	op.retGIS = PlaceDeleteStart
	expCalled = []string{"GIS", "WFL", "TO", "PDP", "UP"}
	op.run(nil)
	assert.Equal(expCalled, op.called)
	op.rhs.Request.PlanOnly = nil

	op = &fakePlaceOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = PlaceUndoDone
	op.upCancelCountDown = 1
	expCalled = []string{"GIS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// re-attachment use cases
	op = &fakePlaceOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.rhs.Request.PlanOnly = swag.Bool(true)
	op.retGIS = PlaceReattachStartPlanning
	expCalled = []string{"GIS", "WFL", "R-PP", "UP"}
	op.run(nil)
	assert.Equal(expCalled, op.called)
	op.rhs.Request.PlanOnly = nil

	op = &fakePlaceOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = PlaceReattachStartPlanning
	op.upCancelCountDown = 1
	expCalled = []string{"GIS", "WFL", "R-PP", "UP-C"}
	assert.False(op.rhs.Canceling)
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.True(op.rhs.Canceling)

	op = &fakePlaceOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = PlaceReattachStartPlanning
	expCalled = []string{"GIS", "WFL", "R-PP", "UP", "FC", "R-CSR", "UP", "R-NSR", "WSR"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakePlaceOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = PlaceReattachStartPlanning
	op.upCancelCountDown = 2
	expCalled = []string{"GIS", "WFL", "R-PP", "UP", "FC", "R-CSR", "UP-C"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakePlaceOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = PlaceReattachExecute
	expCalled = []string{"GIS", "WFL", "FC", "R-CSR", "UP", "R-NSR", "WSR"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// call the real handler and ensure that it grabs the lock
	v := &models.VolumeSeries{}
	v.VolumeSeriesState = "BOUND"
	rhs := &vra.RequestHandlerState{
		A:            c.Animator,
		InError:      false,
		TimedOut:     false,
		Request:      vr,
		VolumeSeries: v,
	}
	fso.RetSSErr = fmt.Errorf("select-storage-error")
	assert.Empty(app.StateGuard.Status())
	assert.Equal(0, app.StateGuard.Used)
	c.Place(nil, rhs)
	assert.Empty(app.StateGuard.Status())
	assert.Equal(1, app.StateGuard.Used)

	// really test undo handler, including path to restore rhs.InError
	rhs.InError = true
	rhs.RetryLater = false
	rhs.Request = &models.VolumeSeriesRequest{}
	rhs.Request.VolumeSeriesRequestState = com.VolReqStateUndoPlacement
	rhs.Request.PlanOnly = swag.Bool(true)
	rhs.Request.Meta = &models.ObjMeta{}
	c.UndoPlace(nil, rhs)
	assert.False(rhs.RetryLater)

	// check place state strings exist up to PlaceNoOp
	var pss placeSubState
	for pss = PlaceStartPlanning; pss < PlaceNoOp; pss++ {
		s := pss.String()
		tl.Logger().Debugf("Testing %d %s", pss, s)
		assert.Regexp("^Place", s)
	}
	assert.Regexp("^placeSubState", pss.String())
}

type fakePlaceOps struct {
	placeOp
	wflCalled         bool
	upCancelCountDown int
	called            []string
	retGIS            placeSubState
	setRestartESR     bool
}

func (op *fakePlaceOps) getInitialState(ctx context.Context) placeSubState {
	op.called = append(op.called, "GIS")
	return op.retGIS
}
func (op *fakePlaceOps) waitForLock(ctx context.Context) {
	if !op.wflCalled {
		op.called = append(op.called, "WFL")
		op.wflCalled = true
	}
}
func (op *fakePlaceOps) selectStorage(ctx context.Context) {
	op.called = append(op.called, "SS")
}
func (op *fakePlaceOps) tagObjects(ctx context.Context) {
	op.called = append(op.called, "TO")
}
func (op *fakePlaceOps) populatePlan(ctx context.Context) {
	op.called = append(op.called, "PP")
}
func (op *fakePlaceOps) populateDeletionPlan(ctx context.Context) {
	op.called = append(op.called, "PDP")
}
func (op *fakePlaceOps) populateReattachPlan(ctx context.Context) {
	op.called = append(op.called, "R-PP")
}
func (op *fakePlaceOps) updatePlan(ctx context.Context) {
	if op.upCancelCountDown > 0 {
		op.upCancelCountDown--
		if op.upCancelCountDown == 0 {
			op.rhs.Canceling = true
			op.called = append(op.called, "UP-C")
			return
		}
	}
	op.called = append(op.called, "UP")
}
func (op *fakePlaceOps) fetchClaims(ctx context.Context) {
	op.called = append(op.called, "FC")
}
func (op *fakePlaceOps) handleCompletedReattachStorageRequests(ctx context.Context) {
	op.called = append(op.called, "R-CSR")
}
func (op *fakePlaceOps) handleCompletedStorageRequests(ctx context.Context) {
	op.called = append(op.called, "CSR")
}
func (op *fakePlaceOps) claimFromExistingStorageRequests(ctx context.Context) {
	op.called = append(op.called, "ESR")
	if op.setRestartESR {
		op.mustRestart = true
	}
}
func (op *fakePlaceOps) issueNewReattachStorageRequests(ctx context.Context) {
	op.called = append(op.called, "R-NSR")
}
func (op *fakePlaceOps) issueNewStorageRequests(ctx context.Context) {
	op.called = append(op.called, "NSR")
}
func (op *fakePlaceOps) removeClaims(ctx context.Context) {
	op.called = append(op.called, "RC")
}
func (op *fakePlaceOps) allocateStorageCapacity(ctx context.Context) {
	op.called = append(op.called, "ASC")
}
func (op *fakePlaceOps) releaseStorageCapacity(ctx context.Context) {
	op.called = append(op.called, "RSC")
}
func (op *fakePlaceOps) waitForStorageRequests(ctx context.Context) {
	op.called = append(op.called, "WSR")
}
func (op *fakePlaceOps) assignStorageInVolumeSeries(ctx context.Context) {
	op.called = append(op.called, "ASV")
}
func (op *fakePlaceOps) removeStorageFromVolumeSeries(ctx context.Context) {
	op.called = append(op.called, "RSV")
}
func (op *fakePlaceOps) recomputeStoragePlan(ctx context.Context) {
	op.called = append(op.called, "RSP")
}
