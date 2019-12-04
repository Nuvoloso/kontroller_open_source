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
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_formula"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/clusterd"
	fakeState "github.com/Nuvoloso/kontroller/pkg/clusterd/state/fake"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	mockcsp "github.com/Nuvoloso/kontroller/pkg/csp/mock"
	"github.com/Nuvoloso/kontroller/pkg/layout"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	fvra "github.com/Nuvoloso/kontroller/pkg/vra/fake"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestSizeSteps(t *testing.T) {
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
	fso := fakeState.NewFakeClusterState()
	app.StateOps = fso

	c := newComponent()
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc
	ctx := context.Background()

	// Invoke the steps in order (success cases last to fall through)
	now := time.Now()
	vr := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:      "VR-1",
				Version: 1,
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			ClusterID:           "cl1",
			CompleteByTime:      strfmt.DateTime(now),
			RequestedOperations: []string{"MOUNT"},
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				CapacityReservationPlan:  &models.CapacityReservationPlan{},
				RequestMessages:          []*models.TimestampedString{},
				StoragePlan:              &models.StoragePlan{},
				VolumeSeriesRequestState: "NEW",
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID:         "node1",
				VolumeSeriesID: "VS-1",
			},
		},
	}
	v := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID:      "VS-1",
				Version: 1,
			},
			RootParcelUUID: "uuid",
		},
		VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
			AccountID: "account1",
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				BoundClusterID:          "cl-1",
				ServicePlanAllocationID: "spa-1",
				CapacityAllocations: map[string]models.CapacityAllocation{
					"prov-1": {ReservedBytes: swag.Int64(2147483648)},
					"prov-2": {ReservedBytes: swag.Int64(0)},
					"prov-3": {ReservedBytes: swag.Int64(1073741824)},
				},
				Messages:          []*models.TimestampedString{},
				VolumeSeriesState: "BOUND",
				Mounts:            []*models.Mount{},
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				Name:               "name",
				Description:        "description",
				SizeBytes:          swag.Int64(3221225472),
				ServicePlanID:      "plan1",
				SpaAdditionalBytes: swag.Int64(1073741824),
			},
		},
	}
	spa := &models.ServicePlanAllocation{
		ServicePlanAllocationMutable: models.ServicePlanAllocationMutable{
			ServicePlanAllocationCreateMutable: models.ServicePlanAllocationCreateMutable{
				StorageFormula: "formula-e",
			},
		},
	}
	sfOK := &storage_formula.StorageFormulaListOK{
		Payload: []*models.StorageFormula{
			&models.StorageFormula{
				CacheComponent: map[string]models.StorageFormulaTypeElement{"type": {Percentage: swag.Int32(0)}},
				Name:           "formula-e",
				StorageLayout:  models.StorageLayoutStandalone,
			},
		},
	}
	sts := []*models.CSPStorageType{
		&models.CSPStorageType{
			Name: "Amazon HDD",
			CspStorageTypeAttributes: map[string]models.ValueType{
				csp.CSPEphemeralStorageType: {Kind: "STRING", Value: csp.EphemeralTypeHDD},
			},
		},
	}
	rhs := &vra.RequestHandlerState{
		A:            c.Animator,
		HasMount:     true,
		Request:      vr,
		VolumeSeries: v,
	}
	op := &sizeOp{
		c:   c,
		rhs: rhs,
	}
	layoutAlgorithm, err := layout.FindAlgorithm(layout.AlgorithmStandaloneNetworkedShared)
	assert.NoError(err)
	assert.NotNil(layoutAlgorithm)

	// ***************************** getInitialState
	op.rhs.TimedOut = true
	assert.Equal(SizeCleanup, op.getInitialState(ctx))
	op.rhs.TimedOut = false

	op.rhs.Canceling = true
	assert.Equal(SizeDone, op.getInitialState(ctx))
	op.rhs.Canceling = false

	op.rhs.InError = true
	assert.Equal(SizeDone, op.getInitialState(ctx))
	op.rhs.InError = false

	c.rei.SetProperty("size-block-on-start", &rei.Property{BoolValue: true})
	assert.Equal(SizeDone, op.getInitialState(ctx))
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	op.rhs.RetryLater = false

	v.VolumeSeriesState = "PROVISIONED"
	assert.Equal(SizeSetStoragePlan, op.getInitialState(ctx))
	v.VolumeSeriesState = "BOUND" // reset

	assert.Equal(SizeSetStoragePlan, op.getInitialState(ctx))

	vr.StoragePlan = nil
	assert.Equal(SizeSetStoragePlan, op.getInitialState(ctx))

	vr.StoragePlan = &models.StoragePlan{
		StorageLayout: "random",
		StorageElements: []*models.StoragePlanStorageElement{
			&models.StoragePlanStorageElement{},
		},
	}
	assert.Equal(SizeDone, op.getInitialState(ctx))

	vr.VolumeSeriesRequestState = com.VolReqStateUndoResizingCache
	v.VolumeSeriesState = "CONFIGURED"
	op.rhs.InError = true
	op.inError = false
	assert.Equal(SizeUndoDone, op.getInitialState(ctx))
	assert.False(op.inError)
	assert.True(op.rhs.InError)
	v.VolumeSeriesState = "BOUND" // reset

	vr.VolumeSeriesRequestState = com.VolReqStateUndoSizing
	op.rhs.InError = true
	op.inError = false
	assert.Equal(SizeCleanup, op.getInitialState(ctx))
	assert.True(op.inError)
	assert.False(op.rhs.InError)

	v.VolumeSeriesState = "PROVISIONED"
	assert.Equal(SizeUndoDone, op.getInitialState(ctx))
	assert.Equal("PROVISIONED", v.VolumeSeriesState)
	v.VolumeSeriesState = "BOUND" // reset

	op.rhs.HasDelete = true
	assert.Equal(SizeCleanup, op.getInitialState(ctx))
	op.rhs.HasDelete = false

	vr.PlanOnly = swag.Bool(true)
	assert.Equal(SizeUndoDone, op.getInitialState(ctx))
	vr.PlanOnly = nil

	sp := vr.StoragePlan
	vr.StoragePlan = &models.StoragePlan{}
	op.rhs.HasDelete = true
	assert.Equal(SizeUndoDone, op.getInitialState(ctx))
	op.rhs.HasDelete = false

	v.VolumeSeriesState = "BOUND"
	assert.Equal(SizeUndoDone, op.getInitialState(ctx))
	op.inError = false
	vr.StoragePlan = sp
	vr.VolumeSeriesRequestState = com.VolReqStateSizing

	// ***************************** setStoragePlan
	// failure, spa fetch error
	op.rhs.InError = false                             // reset
	op.rhs.RetryLater = false                          // reset
	vr.RequestMessages = []*models.TimestampedString{} // reset
	op.rhs.VolumeSeries = v
	fc.RetSPAFetchErr = fmt.Errorf("spa fetch error")
	fc.RetSPAFetchObj = nil
	fc.InVSRUpdaterItems = nil
	fc.RetVSRUpdaterObj = vr
	fc.RetVSRUpdaterErr = nil
	op.setStoragePlan(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(vr.RequestMessages, 1)
	assert.NotNil(fc.InVSRUpdaterID)

	// failure, formula fetch db error with CHANGE_CAPACITY
	op.rhs.HasChangeCapacity = true
	op.rhs.InError = false                             // reset
	op.rhs.RetryLater = false                          // reset
	vr.RequestMessages = []*models.TimestampedString{} // reset
	op.rhs.VolumeSeries = v
	fc.RetLsSFErr = fmt.Errorf("storage formula db error")
	fc.RetLsSFOk = nil
	fc.InVSRUpdaterItems = nil
	fc.RetVSRUpdaterObj = vr
	fc.RetVSRUpdaterErr = nil
	op.setStoragePlan(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(vr.RequestMessages, 1)
	assert.NotNil(fc.InVSRUpdaterID)
	op.rhs.HasChangeCapacity = false // reset

	// failure, formula fetch db error
	op.rhs.InError = false                             // reset
	op.rhs.RetryLater = false                          // reset
	vr.RequestMessages = []*models.TimestampedString{} // reset
	op.rhs.VolumeSeries = v
	fc.RetLsSFErr = fmt.Errorf("storage formula db error")
	fc.RetLsSFOk = nil
	fc.RetSPAFetchErr = nil
	fc.RetSPAFetchObj = spa
	fc.InVSRUpdaterItems = nil
	fc.RetVSRUpdaterObj = vr
	fc.RetVSRUpdaterErr = nil
	op.setStoragePlan(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(vr.RequestMessages, 1)
	assert.NotNil(fc.InVSRUpdaterID)

	// failure, formula fetch not found (also cover planOnly w/o BIND or CHANGE_CAPACITY)
	op.rhs.InError = false                             // reset
	op.rhs.RetryLater = false                          // reset
	vr.RequestMessages = []*models.TimestampedString{} // reset
	vr.PlanOnly = swag.Bool(true)
	vr.StorageFormula = "complex"
	fc.InLsSFObj = nil
	fc.RetLsSFErr = nil
	fc.RetLsSFOk = &storage_formula.StorageFormulaListOK{}
	fc.InVSRUpdaterItems = nil
	op.setStoragePlan(ctx)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(vr.RequestMessages, 1)
	assert.EqualValues(spa.StorageFormula, swag.StringValue(fc.InLsSFObj.Name))
	assert.Nil(fc.InVSRUpdaterItems)

	// failure, layout algorithm recorded in VS is wrong
	vr.PlanOnly = swag.Bool(false)                     // reset
	op.rhs.InError = false                             // reset
	vr.StoragePlan = nil                               // reset
	vr.RequestMessages = []*models.TimestampedString{} // reset
	v.LifecycleManagementData = &models.LifecycleManagementData{
		LayoutAlgorithm: layoutAlgorithm.Name + "foo",
	}
	op.rhs.VolumeSeries = v
	fc.RetLsSFOk = sfOK
	op.setStoragePlan(ctx)
	assert.True(op.rhs.InError)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Invalid layout algorithm.*recorded in VolumeSeries", vr.RequestMessages[0].Message)

	// success with recorded layout algorithm
	vr.PlanOnly = swag.Bool(false)                     // reset
	op.rhs.InError = false                             // reset
	vr.StoragePlan = nil                               // reset
	vr.RequestMessages = []*models.TimestampedString{} // reset
	v.LifecycleManagementData = &models.LifecycleManagementData{
		LayoutAlgorithm: layoutAlgorithm.Name,
	}
	op.rhs.VolumeSeries = v
	fc.InVSRUpdaterItems = nil
	fc.RetVSRUpdaterObj = nil
	fc.RetVSRUpdaterErr = nil
	op.setStoragePlan(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(vr.RequestMessages, 0)
	assert.NotNil(fc.InVSRUpdaterItems)
	assert.NotNil(fc.ModVSRUpdaterObj)
	assert.NotNil(fc.ModVSRUpdaterObj.StoragePlan)
	assert.Equal(layoutAlgorithm.Name, fc.ModVSRUpdaterObj.StoragePlan.LayoutAlgorithm)
	assert.Equal(layoutAlgorithm.StorageLayout, fc.ModVSRUpdaterObj.StoragePlan.StorageLayout)

	// failure, no layout algorithm found
	vr.PlanOnly = swag.Bool(false)                     // reset
	op.rhs.InError = false                             // reset
	vr.StoragePlan = nil                               // reset
	vr.RequestMessages = []*models.TimestampedString{} // reset
	v.LifecycleManagementData = nil                    // reset
	op.rhs.VolumeSeries = v
	fso.RetFPAalgo = nil
	op.setStoragePlan(ctx)
	assert.True(op.rhs.InError)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("No placement algorithm for layout", vr.RequestMessages[0].Message)
	assert.Equal(models.StorageLayoutStandalone, fso.InFPAlayout) // TBD: from formula

	fso.RetFPAalgo = layoutAlgorithm // now succeed

	// success with finding layout algorithm, update failure
	vr.PlanOnly = swag.Bool(false)                     // reset
	op.rhs.InError = false                             // reset
	vr.StoragePlan = nil                               // reset
	vr.RequestMessages = []*models.TimestampedString{} // reset
	op.rhs.VolumeSeries = v
	fc.InVSRUpdaterItems = nil
	fc.RetVSRUpdaterObj = nil
	fc.RetVSRUpdaterErr = fmt.Errorf("db update error")
	op.setStoragePlan(ctx)
	assert.True(op.rhs.InError)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("Failed to update.*db update error", vr.RequestMessages[0].Message)
	assert.NotNil(fc.InVSRUpdaterItems)

	// success with finding layout algorithm
	op.rhs.InError = false                             // reset
	vr.StoragePlan = nil                               // reset
	vr.RequestMessages = []*models.TimestampedString{} // reset
	op.rhs.VolumeSeries = v
	fc.InLsSFObj = nil
	fc.RetLsSFErr = nil
	fc.RetLsSFOk = sfOK
	newVr := &models.VolumeSeriesRequest{}
	*newVr = *vr // shallow copy
	newVr.Meta = &models.ObjMeta{}
	*newVr.Meta = *vr.Meta // shallow copy
	newVr.Meta.Version++
	fc.InUVItems = nil
	fc.InUVRItems = nil
	fc.InUVRCount = 0
	assert.NotEqual(newVr.Meta.Version, vr.Meta.Version)
	fc.RetVSRUpdaterObj = newVr
	fc.RetVSRUpdaterErr = nil
	op.setStoragePlan(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(newVr, op.rhs.Request) // updated obj in po
	assert.Equal(newVr.Meta.Version, op.rhs.Request.Meta.Version)
	assert.Empty(vr.RequestMessages)
	assert.EqualValues("formula-e", swag.StringValue(fc.InLsSFObj.Name))
	assert.Nil(fc.InUVItems)
	assert.NotNil(fc.InVSRUpdaterItems)
	assert.NotNil(vr.StoragePlan)
	assert.EqualValues(sfOK.Payload[0].StorageLayout, vr.StoragePlan.StorageLayout)
	assert.Equal(layoutAlgorithm.Name, vr.StoragePlan.LayoutAlgorithm)
	assert.Empty(vr.StoragePlan.PlacementHints)
	assert.Len(vr.StoragePlan.StorageElements, 2)
	first := 0
	other := 1
	if string(vr.StoragePlan.StorageElements[0].PoolID) == "prov-3" {
		first = 1
		other = 0
	}
	assert.Equal(com.VolReqStgElemIntentData, vr.StoragePlan.StorageElements[first].Intent)
	assert.EqualValues(2147483648, swag.Int64Value(vr.StoragePlan.StorageElements[first].SizeBytes))
	assert.Len(vr.StoragePlan.StorageElements[first].StorageParcels, 1)
	assert.EqualValues(2147483648, swag.Int64Value(vr.StoragePlan.StorageElements[first].StorageParcels[com.VolReqStgPseudoParcelKey].SizeBytes))
	assert.EqualValues("prov-1", vr.StoragePlan.StorageElements[first].PoolID)
	assert.Equal(com.VolReqStgElemIntentData, vr.StoragePlan.StorageElements[other].Intent)
	assert.EqualValues(1073741824, swag.Int64Value(vr.StoragePlan.StorageElements[other].SizeBytes))
	assert.Len(vr.StoragePlan.StorageElements[first].StorageParcels, 1)
	assert.EqualValues(1073741824, swag.Int64Value(vr.StoragePlan.StorageElements[other].StorageParcels[com.VolReqStgPseudoParcelKey].SizeBytes))
	assert.EqualValues("prov-3", vr.StoragePlan.StorageElements[other].PoolID)
	op.rhs.Request = vr // put the original obj back

	// success, provisioned
	vr.StoragePlan = nil                               // reset
	vr.RequestMessages = []*models.TimestampedString{} // reset
	v.StorageParcels = map[string]models.ParcelAllocation{
		"1": {SizeBytes: swag.Int64(2147483648)},
		"2": {SizeBytes: swag.Int64(2147483648)},
	}
	v.VolumeSeriesState = "PROVISIONED"
	op.setStoragePlan(ctx)
	assert.Nil(vr.StoragePlan)
	assert.Nil(fc.InUVItems)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(vr, op.rhs.Request) // original obj in op
	v.StorageParcels = nil           // reset
	v.VolumeSeriesState = "BOUND"    // reset
	op.rhs.Request = vr              // put the original obj back

	// success with cache, plan only (also cover ignored storageLayout code path)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cSP := mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cSP.EXPECT().SupportedCspStorageTypes().Return(sts).Times(3)
	app.CSP = cSP
	vr.StoragePlan = nil                               // reset
	vr.RequestMessages = []*models.TimestampedString{} // reset
	vr.PlanOnly = swag.Bool(true)
	vr.CapacityReservationResult = &models.CapacityReservationResult{
		DesiredReservations: map[string]models.PoolReservation{
			"prov-1": {SizeBytes: swag.Int64(2147483648)},
			"prov-2": {SizeBytes: swag.Int64(0)},
			"prov-3": {SizeBytes: swag.Int64(1073741824)},
		},
	}
	vr.StorageFormula = "complex"
	v.CapacityAllocations = map[string]models.CapacityAllocation{}
	sfOK.Payload[0].CacheComponent["type"] = models.StorageFormulaTypeElement{Percentage: swag.Int32(10)}
	sfOK.Payload[0].StorageLayout = models.StorageLayoutMirrored
	fc.InLsSFObj = nil
	fc.InVSRUpdaterItems = nil
	op.rhs.HasBind = true
	op.setStoragePlan(ctx)
	assert.Nil(fc.InUVItems)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(newVr, op.rhs.Request) // updated obj in po
	assert.Equal(newVr.Meta.Version, op.rhs.Request.Meta.Version)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("ignored .* storageLayout ", vr.RequestMessages[0].Message)
	assert.EqualValues("complex", swag.StringValue(fc.InLsSFObj.Name))
	assert.Nil(fc.InUVItems)
	assert.NotNil(fc.InVSRUpdaterItems)
	assert.NotNil(vr.StoragePlan)
	assert.EqualValues(models.StorageLayoutStandalone, vr.StoragePlan.StorageLayout)
	assert.Empty(vr.StoragePlan.PlacementHints)
	assert.Len(vr.StoragePlan.StorageElements, 3)
	first = 0
	other = 1
	if string(vr.StoragePlan.StorageElements[0].PoolID) == "prov-3" {
		first = 1
		other = 0
	}
	assert.Equal(com.VolReqStgElemIntentData, vr.StoragePlan.StorageElements[first].Intent)
	assert.EqualValues(2147483648, swag.Int64Value(vr.StoragePlan.StorageElements[first].SizeBytes))
	assert.Len(vr.StoragePlan.StorageElements[first].StorageParcels, 1)
	assert.EqualValues(2147483648, swag.Int64Value(vr.StoragePlan.StorageElements[first].StorageParcels[com.VolReqStgPseudoParcelKey].SizeBytes))
	assert.EqualValues("prov-1", vr.StoragePlan.StorageElements[first].PoolID)
	assert.Equal(com.VolReqStgElemIntentData, vr.StoragePlan.StorageElements[other].Intent)
	assert.EqualValues(1073741824, swag.Int64Value(vr.StoragePlan.StorageElements[other].SizeBytes))
	assert.Len(vr.StoragePlan.StorageElements[first].StorageParcels, 1)
	assert.EqualValues(1073741824, swag.Int64Value(vr.StoragePlan.StorageElements[other].StorageParcels[com.VolReqStgPseudoParcelKey].SizeBytes))
	assert.EqualValues("prov-3", vr.StoragePlan.StorageElements[other].PoolID)
	expCache := &models.StoragePlanStorageElement{
		Intent:    "CACHE",
		SizeBytes: swag.Int64(429496730), // 10*(3221225472+1073741824) + 99) / 100
		StorageParcels: map[string]models.StorageParcelElement{
			"SSD": {ProvMinSizeBytes: swag.Int64(0)},
		},
	}
	assert.Equal(expCache, vr.StoragePlan.StorageElements[2])
	op.rhs.Request = vr    // put the original obj back
	op.rhs.HasBind = false // reset

	// success, plan only, CHANGE_CAPACITY
	vr.StoragePlan = nil                               // reset
	vr.RequestMessages = []*models.TimestampedString{} // reset
	vr.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{}
	v.VolumeSeriesState = "CONFIGURED"
	op.rhs.HasChangeCapacity = true
	op.setStoragePlan(ctx)
	if assert.NotNil(vr.StoragePlan) && assert.Len(vr.StoragePlan.StorageElements, 1) {
		expCache.SizeBytes = swag.Int64(322122548) // 10*(3221225472) + 99) / 100, SpaSizeBytes is taken from VSR, not VS
		assert.Equal(expCache, vr.StoragePlan.StorageElements[0])
	}
	assert.Nil(fc.InUVItems)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(newVr, op.rhs.Request) // updated obj in po
	assert.Equal(newVr.Meta.Version, op.rhs.Request.Meta.Version)
	v.StorageParcels = nil           // reset
	v.VolumeSeriesState = "BOUND"    // reset
	op.rhs.Request = vr              // put the original obj back
	op.rhs.HasChangeCapacity = false // reset

	// success, cache type found, HasVolSnapshotRestore ignores cache
	vr.StoragePlan = nil                               // reset
	vr.RequestMessages = []*models.TimestampedString{} // reset
	v.StorageParcels = map[string]models.ParcelAllocation{
		"1": {SizeBytes: swag.Int64(2147483648)},
		"2": {SizeBytes: swag.Int64(2147483648)},
	}
	v.VolumeSeriesState = "PROVISIONED"
	sfOK.Payload[0].CacheComponent["type"] = models.StorageFormulaTypeElement{Percentage: swag.Int32(0)} // reset
	sfOK.Payload[0].CacheComponent["Amazon HDD"] = models.StorageFormulaTypeElement{Percentage: swag.Int32(10)}
	fc.InVSRUpdaterItems = nil
	op.rhs.HasVolSnapshotRestore = true
	op.setStoragePlan(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Nil(fc.InVSRUpdaterItems)
	assert.Nil(vr.StoragePlan)
	assert.Len(vr.RequestMessages, 1)
	assert.Regexp("ignored .* cache ", vr.RequestMessages[0].Message)
	v.VolumeSeriesState = "BOUND"        // reset
	op.rhs.HasVolSnapshotRestore = false // reset
	op.rhs.Request = vr                  // put the original obj back

	// success, provisioned, cache type found, no SPA bytes
	vr.StoragePlan = nil                               // reset
	vr.RequestMessages = []*models.TimestampedString{} // reset
	v.StorageParcels = map[string]models.ParcelAllocation{
		"1": {SizeBytes: swag.Int64(2147483648)},
		"2": {SizeBytes: swag.Int64(2147483648)},
	}
	v.SpaAdditionalBytes = nil
	v.VolumeSeriesState = "PROVISIONED"
	sfOK.Payload[0].CacheComponent["type"] = models.StorageFormulaTypeElement{Percentage: swag.Int32(0)} // reset
	sfOK.Payload[0].CacheComponent["Amazon HDD"] = models.StorageFormulaTypeElement{Percentage: swag.Int32(10)}
	fc.InVSRUpdaterItems = nil
	op.setStoragePlan(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(fc.InVSRUpdaterItems)
	assert.NotNil(vr.StoragePlan)
	assert.Empty(vr.StoragePlan.StorageLayout)
	assert.Empty(vr.StoragePlan.PlacementHints)
	assert.Len(vr.StoragePlan.StorageElements, 1)
	expCache = &models.StoragePlanStorageElement{
		Intent:    "CACHE",
		SizeBytes: swag.Int64(322122548), // 10*(3221225472+0) + 99) / 100
		StorageParcels: map[string]models.StorageParcelElement{
			"HDD": {ProvMinSizeBytes: swag.Int64(0)},
		},
	}
	v.VolumeSeriesState = "BOUND"                                 // reset
	vr.StoragePlan.StorageLayout = models.StorageLayoutStandalone // reset
	op.rhs.Request = vr                                           // put the original obj back

	// ***************************** cleanup
	assert.NotEmpty(vr.StoragePlan.StorageLayout)
	assert.NotEmpty(vr.StoragePlan.StorageElements)
	op.cleanup(ctx)
	assert.Empty(vr.StoragePlan.StorageLayout)
	assert.Empty(vr.StoragePlan.PlacementHints)
	assert.Empty(vr.StoragePlan.StorageElements)
}

func TestSize(t *testing.T) {
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
	c := newComponent()
	c.Init(app)

	vr := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VR-1",
			},
		},
	}

	op := &fakeSizingOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = SizeSetStoragePlan
	expCalled := []string{"GIS", "SSP"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakeSizingOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = SizeCleanup
	expCalled = []string{"GIS", "TOC"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakeSizingOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr}
	op.retGIS = SizeDone
	expCalled = []string{"GIS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// call the real handler with an error

	v := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: "VS-1",
			},
			RootParcelUUID: "uuid",
		},
	}
	vr = &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VR-1",
			},
		},
	}
	vr.VolumeSeriesID = models.ObjIDMutable(v.Meta.ID)
	fc := &fake.Client{}
	fvu := &fvra.VolumeUpdater{}
	c.oCrud = fc
	c.Animator.OCrud = fc
	rhs := &vra.RequestHandlerState{
		A:            c.Animator,
		InError:      true,
		Request:      vr,
		VolumeSeries: v,
	}
	rhs.VSUpdater = fvu
	c.Size(nil, rhs)
	assert.Empty(fc.InLsSFObj)

	// call real handler in undo
	vr.StoragePlan = &models.StoragePlan{
		StorageLayout: "random",
		PlacementHints: map[string]models.ValueType{
			"one": {Kind: "STRING", Value: "string"},
		},
		StorageElements: []*models.StoragePlanStorageElement{
			&models.StoragePlanStorageElement{},
		},
	}
	vr.VolumeSeriesRequestState = com.VolReqStateUndoSizing
	rhs.InError = true
	c.UndoSize(nil, rhs)
	assert.True(rhs.InError)
	assert.Empty(vr.StoragePlan.StorageLayout)
	assert.Empty(vr.StoragePlan.PlacementHints)
	assert.Empty(vr.StoragePlan.StorageElements)
	assert.Empty(fvu.InSVSState)

	// check place state strings exist up to SizeNoOp
	var ss sizeSubState
	for ss = SizeSetStoragePlan; ss < SizeNoOp; ss++ {
		s := ss.String()
		tl.Logger().Debugf("Testing %d %s", ss, s)
		assert.Regexp("^Size", s)
	}
	assert.Regexp("^sizeSubState", ss.String())
}

func TestResizeCache(t *testing.T) {
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
	c := newComponent()
	c.Init(app)

	vr := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VR-1",
			},
		},
	}

	op := &fakeSizingOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr, HasChangeCapacity: true}
	op.retGIS = SizeSetStoragePlan
	expCalled := []string{"GIS", "SSP"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakeSizingOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr, HasChangeCapacity: true}
	op.retGIS = SizeCleanup
	expCalled = []string{"GIS", "TOC"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakeSizingOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vr, HasChangeCapacity: true}
	op.retGIS = SizeDone
	expCalled = []string{"GIS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// call the real handler with an error

	v := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: "VS-1",
			},
			RootParcelUUID: "uuid",
		},
	}
	v.VolumeSeriesState = "PROVISIONED"
	vr = &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VR-1",
			},
		},
	}
	vr.VolumeSeriesID = models.ObjIDMutable(v.Meta.ID)
	fc := &fake.Client{}
	fvu := &fvra.VolumeUpdater{}
	c.oCrud = fc
	c.Animator.OCrud = fc
	rhs := &vra.RequestHandlerState{
		A:                 c.Animator,
		HasChangeCapacity: true,
		InError:           true,
		Request:           vr,
		VolumeSeries:      v,
	}
	rhs.VSUpdater = fvu
	c.ResizeCache(nil, rhs)
	assert.Empty(fc.InLsSFObj)

	// call real handler in undo
	vr.StoragePlan = &models.StoragePlan{
		StorageElements: []*models.StoragePlanStorageElement{
			&models.StoragePlanStorageElement{Intent: "CACHE"},
		},
	}
	vr.VolumeSeriesRequestState = com.VolReqStateUndoResizingCache
	rhs.InError = true
	c.UndoResizeCache(nil, rhs)
	assert.True(rhs.InError)
	assert.Empty(vr.StoragePlan.StorageLayout)
	assert.Empty(vr.StoragePlan.PlacementHints)
	assert.Len(vr.StoragePlan.StorageElements, 1)
	assert.Empty(fvu.InSVSState)
}

type fakeSizingOps struct {
	sizeOp
	called []string
	retGIS sizeSubState
}

func (op *fakeSizingOps) getInitialState(ctx context.Context) sizeSubState {
	op.called = append(op.called, "GIS")
	return op.retGIS
}

func (op *fakeSizingOps) setStoragePlan(ctx context.Context) {
	op.called = append(op.called, "SSP")
}

func (op *fakeSizingOps) cleanup(ctx context.Context) {
	op.called = append(op.called, "TOC")
}
