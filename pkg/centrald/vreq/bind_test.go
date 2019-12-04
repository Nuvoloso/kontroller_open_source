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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/pool"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan_allocation"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	fhk "github.com/Nuvoloso/kontroller/pkg/housekeeping/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/alecthomas/units"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestBindSteps(t *testing.T) {
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
	sfc := &fakeSFC{}
	app.SFC = sfc
	fts := &fhk.TaskScheduler{}
	app.TaskScheduler = fts
	ctx := context.Background()
	c := &Component{}
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc

	cl := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID:      "CLUSTER-1",
				Version: 1,
			},
		},
		ClusterCreateOnce: models.ClusterCreateOnce{
			ClusterType: "kubernetes",
			CspDomainID: "CSP-DOMAIN-1",
		},
		ClusterMutable: models.ClusterMutable{
			ClusterCreateMutable: models.ClusterCreateMutable{
				Name: "MyCluster",
			},
		},
	}
	spaObj := &models.ServicePlanAllocation{
		ServicePlanAllocationAllOf0: models.ServicePlanAllocationAllOf0{
			Meta: &models.ObjMeta{ID: "SPA-1"},
		},
		ServicePlanAllocationCreateOnce: models.ServicePlanAllocationCreateOnce{
			AccountID:           "SPA-OWNER-ACCOUNT",
			AuthorizedAccountID: "VOLUME-OWNER-ACCOUNT",
			ClusterID:           "CLUSTER-1",
			ServicePlanID:       "SERVICE-PLAN-1",
		},
		ServicePlanAllocationMutable: models.ServicePlanAllocationMutable{
			ServicePlanAllocationCreateMutable: models.ServicePlanAllocationCreateMutable{
				StorageFormula: "FORMULA1",
				StorageReservations: map[string]models.StorageTypeReservation{
					"SP-1": models.StorageTypeReservation{},
					"SP-2": models.StorageTypeReservation{},
				},
			},
		},
	}
	spa := spaClone(spaObj)
	poolList := []*models.Pool{
		&models.Pool{
			PoolAllOf0: models.PoolAllOf0{
				Meta: &models.ObjMeta{ID: "SP-1"},
			},
			PoolCreateOnce: models.PoolCreateOnce{
				CspStorageType: "Amazon gp2",
			},
		},
		&models.Pool{
			PoolAllOf0: models.PoolAllOf0{
				Meta: &models.ObjMeta{ID: "SP-2"},
			},
			PoolCreateOnce: models.PoolCreateOnce{
				CspStorageType: "Amazon io1",
			},
		},
	}
	pools := poolListClone(poolList)
	vsObj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID:      "VS-1",
				Version: 1,
			},
		},
		VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
			AccountID: "VOLUME-OWNER-ACCOUNT",
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				VolumeSeriesState: "NEW",
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				SizeBytes:     swag.Int64(int64(1 * units.GiB)),
				ServicePlanID: "SERVICE-PLAN-1",
			},
		},
	}
	vs := vsClone(vsObj)
	vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VSR-1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			ClusterID: "CLUSTER-1",
			VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{
				VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
					SizeBytes:          swag.Int64(int64(10 * units.GiB)),
					SpaAdditionalBytes: swag.Int64(int64(units.GiB)),
				},
			},
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "BINDING",
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				VolumeSeriesID: "VS-1",
			},
		},
	}
	vsr := vsrClone(vsrObj)

	newOp := func() *bindOp {
		op := &bindOp{}
		op.c = c
		op.ops = op
		op.rhs = &vra.RequestHandlerState{A: c.Animator, VolumeSeries: vsClone(vs), Request: vsrClone(vsr)}
		op.classifyOp()
		assert.True(!op.isCCOp || op.ccHasSizeBytes || op.ccHasSpaAdditionalBytes)
		if !op.isCCOp {
			op.rhs.Request.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{} // at runtime present with auth properties
		} else {
			op.rhs.Request.ClusterID = ""
		}
		return op
	}

	// test helpers
	op := newOp()
	op.spa = spaClone(spaObj)
	ownerTag := "vsr.op:VSR-1"
	notOwnerTag := "vsr.op:OTHER-VSR"
	assert.Equal(ownerTag, op.sTagOp())
	assert.False(op.vsrSpaInUse())
	assert.False(op.vsrOwnsSpa())
	op.vsrClaimSpa()
	assert.True(op.vsrSpaInUse())
	assert.True(op.vsrOwnsSpa())
	assert.EqualValues([]string{ownerTag}, op.spa.SystemTags)
	op.vsrDisownSpa()
	assert.False(op.vsrSpaInUse())
	assert.False(op.vsrOwnsSpa())
	assert.EqualValues([]string{}, op.spa.SystemTags)
	op.spa.SystemTags = []string{"foo:bar", "something:else"}
	op.vsrClaimSpa()
	op.spa.SystemTags = append(op.spa.SystemTags, "one:more")
	assert.True(op.vsrSpaInUse())
	assert.True(op.vsrOwnsSpa())
	assert.EqualValues([]string{"foo:bar", "something:else", ownerTag, "one:more"}, op.spa.SystemTags)
	op.vsrDisownSpa()
	assert.False(op.vsrSpaInUse())
	assert.False(op.vsrOwnsSpa())
	assert.EqualValues([]string{"foo:bar", "something:else", "one:more"}, op.spa.SystemTags)
	op.spa.SystemTags = []string{notOwnerTag}
	assert.True(op.vsrSpaInUse())
	assert.False(op.vsrOwnsSpa())

	assert.False(op.isCCOp)
	assert.Equal("bind-foo", op.reiProp("foo"))
	assert.Equal(com.VolReqOpBind, op.opName)

	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "CHANGING_CAPACITY"
	op = newOp()
	assert.True(op.isCCOp)
	assert.Equal("chg-cap-foo", op.reiProp("foo"))
	assert.Equal(com.VolReqOpChangeCapacity, op.opName)

	// ****************************** bindRemoveTagRe
	matchTCS := []string{
		com.SystemTagVolClusterPrefix + "foo",
		com.SystemTagVolClusterPrefix + "foo.bar",
	}
	for _, k := range matchTCS {
		assert.True(bindRemoveTagRe.MatchString(k), "match %s", k)
	}
	notMatchTCS := []string{
		com.SystemTagVolClusterPrefix,
		"x" + com.SystemTagVolClusterPrefix,
		"a.b.c",
	}
	for _, k := range notMatchTCS {
		assert.False(bindRemoveTagRe.MatchString(k), "not match %s", k)
	}

	//  ***************************** getInitialState

	// UNDO_BIND cases
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "UNDO_BINDING"

	tl.Logger().Info("case: getInitialState undo spID not set, no delete")
	tl.Flush()
	op = newOp()
	op.rhs.TimedOut = true
	op.rhs.InError = true
	assert.Equal(BindUndoDone, op.getInitialState(ctx))
	assert.True(op.inError)
	assert.False(op.rhs.InError) // cleared
	assert.True(op.rhs.TimedOut) // unmodified

	tl.Logger().Info("case: getInitialState undo spID set, no delete")
	tl.Flush()
	op = newOp()
	op.rhs.TimedOut = true
	op.rhs.InError = true
	op.rhs.Request.ServicePlanAllocationID = "SPA-1"
	assert.Equal(BindUndoWaitForReservationLock, op.getInitialState(ctx))
	assert.True(op.inError)
	assert.False(op.rhs.InError) // cleared
	assert.True(op.rhs.TimedOut) // unmodified

	tl.Logger().Info("case: getInitialState undo, delete, SPA id in VS")
	tl.Flush()
	op = newOp()
	op.rhs.TimedOut = true
	op.rhs.InError = true
	op.rhs.HasDelete = true
	op.rhs.VolumeSeries.ServicePlanAllocationID = "SPA-1"
	assert.Equal(BindUndoWaitForReservationLock, op.getInitialState(ctx))
	assert.True(op.inError)
	assert.False(op.rhs.InError) // cleared
	assert.True(op.rhs.TimedOut) // unmodified
	assert.EqualValues("SPA-1", op.rhs.Request.ServicePlanAllocationID)

	tl.Logger().Info("case: getInitialState undo, delete, no SPA id in VS")
	tl.Flush()
	op = newOp()
	op.rhs.TimedOut = true
	op.rhs.InError = true
	op.rhs.HasDelete = true
	assert.Equal(BindUndoDone, op.getInitialState(ctx))
	assert.True(op.inError)
	assert.False(op.rhs.InError) // cleared
	assert.True(op.rhs.TimedOut) // unmodified

	tl.Logger().Info("case: getInitialState undo, unbind, SPA id in VS")
	tl.Flush()
	op = newOp()
	op.rhs.TimedOut = true
	op.rhs.InError = true
	op.rhs.HasUnbind = true
	op.rhs.VolumeSeries.ServicePlanAllocationID = "SPA-1"
	assert.Equal(BindUndoWaitForReservationLock, op.getInitialState(ctx))
	assert.True(op.inError)
	assert.False(op.rhs.InError) // cleared
	assert.True(op.rhs.TimedOut) // unmodified
	assert.EqualValues("SPA-1", op.rhs.Request.ServicePlanAllocationID)

	tl.Logger().Info("case: getInitialState undo, unbind, no SPA id in VS")
	tl.Flush()
	op = newOp()
	op.rhs.TimedOut = true
	op.rhs.InError = true
	op.rhs.HasUnbind = true
	assert.Equal(BindUndoDone, op.getInitialState(ctx))
	assert.True(op.inError)
	assert.False(op.rhs.InError) // cleared
	assert.True(op.rhs.TimedOut) // unmodified

	tl.Logger().Info("case: getInitialState UndoDone was waiting")
	tl.Flush()
	op = newOp()
	op.rhs.TimedOut = true
	op.rhs.Canceling = true
	op.rhs.InError = true
	sTag := util.NewTagList(op.rhs.Request.SystemTags)
	sTag.Set(com.SystemTagVsrSpaCapacityWait, "")
	op.rhs.Request.SystemTags = sTag.List()
	op.rhs.Request.ServicePlanAllocationID = "SPA-1"
	assert.Equal(BindUndoDone, op.getInitialState(ctx))
	assert.True(op.inError)
	assert.False(op.rhs.InError)  // cleared
	assert.True(op.rhs.TimedOut)  // unmodified
	assert.True(op.rhs.Canceling) // unmodified

	tl.Logger().Info("case: getInitialState UndoDone PlanOnly")
	tl.Flush()
	op = newOp()
	op.rhs.TimedOut = true
	op.rhs.InError = true
	op.rhs.Request.PlanOnly = swag.Bool(true)
	op.rhs.Request.ServicePlanAllocationID = "SPA-1"
	assert.Equal(BindUndoDone, op.getInitialState(ctx))
	assert.False(op.inError)
	assert.True(op.rhs.InError)  // unmodified
	assert.True(op.rhs.TimedOut) // unmodified

	// BIND cases
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "BINDING"

	tl.Logger().Info("case: getInitialState BindError TimedOut, PlanOnly")
	tl.Flush()
	op = newOp()
	op.rhs.TimedOut = true
	op.rhs.Request.PlanOnly = swag.Bool(true)
	assert.Equal(BindError, op.getInitialState(ctx))
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Invalid invocation of BIND", op.rhs.Request.RequestMessages[0].Message)
	assert.True(op.planOnly)

	tl.Logger().Info("case: getInitialState BindError InError")
	tl.Flush()
	op = newOp()
	op.rhs.TimedOut = true
	assert.Equal(BindError, op.getInitialState(ctx))
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Invalid invocation of BIND", op.rhs.Request.RequestMessages[0].Message)
	assert.False(op.planOnly)

	tl.Logger().Info("case: getInitialState BindLoadObjects")
	op = newOp()
	assert.Equal(BindLoadObjects, op.getInitialState(ctx))

	// UNDO_CHANGING_CAPACITY cases
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "UNDO_CHANGING_CAPACITY"

	tl.Logger().Info("case: getInitialState undo SPA id in VS")
	tl.Flush()
	op = newOp()
	op.rhs.TimedOut = true
	op.rhs.InError = true
	op.rhs.HasDelete = true
	op.rhs.VolumeSeries.ServicePlanAllocationID = "SPA-1"
	assert.Equal(BindUndoWaitForReservationLock, op.getInitialState(ctx))
	assert.True(op.inError)
	assert.False(op.rhs.InError) // cleared
	assert.True(op.rhs.TimedOut) // unmodified
	assert.EqualValues("SPA-1", op.rhs.Request.ServicePlanAllocationID)

	tl.Logger().Info("case: getInitialState undo no SPA id in VS")
	tl.Flush()
	op = newOp()
	op.rhs.TimedOut = true
	op.rhs.InError = true
	op.rhs.HasDelete = true
	assert.Equal(BindUndoDone, op.getInitialState(ctx))
	assert.True(op.inError)
	assert.False(op.rhs.InError) // cleared
	assert.True(op.rhs.TimedOut) // unmodified

	// CHANGING_CAPACITY cases
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "CHANGING_CAPACITY"

	tl.Logger().Info("case: CHANGE_CAPACITY getInitialState ")
	op = newOp()
	op.rhs.VolumeSeries.ServicePlanAllocationID = "SPA-1"
	assert.Equal(BindWaitForReservationLock, op.getInitialState(ctx))
	assert.EqualValues("SPA-1", op.rhs.Request.ServicePlanAllocationID)

	//  ***************************** loadObjects
	vsr = vsrClone(vsrObj)

	tl.Logger().Info("case: loadObjects cluster fetch error")
	tl.Flush()
	fc.RetLClErr = fmt.Errorf("cluster fetch error")
	op = newOp()
	op.loadObjects(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Equal("CLUSTER-1", fc.InLClID)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("cluster fetch error", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Info("case: loadObjects cluster fetch")
	tl.Flush()
	fc.RetLClErr = nil
	fc.RetLClObj = cl
	fc.InLClID = ""
	op = newOp()
	op.loadObjects(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.Equal("CLUSTER-1", fc.InLClID)
	assert.Equal(cl, op.cluster)

	//  ***************************** poolLoadAll
	vsr = vsrClone(vsrObj)
	spa = spaClone(spaObj)
	pools = poolListClone(poolList)

	tl.Logger().Info("case: poolLoadAll list error")
	tl.Flush()
	fc.InPoolListObj = nil
	fc.RetPoolListObj = nil
	fc.RetPoolListErr = fmt.Errorf("sp-list-error")
	op = newOp()
	op.spa = spa
	op.poolLoadAll(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.NotNil(fc.InPoolListObj)
	assert.Equal("SPA-1", swag.StringValue(fc.InPoolListObj.ServicePlanAllocationID))
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("sp-list-error", op.rhs.Request.RequestMessages[0].Message)
	assert.Nil(op.pools)

	tl.Logger().Info("case: poolLoadAll success (ignore extra pool)")
	tl.Flush()
	fc.InPoolListObj = nil
	newPool := &models.Pool{}
	newPool.Meta = &models.ObjMeta{ID: "EXTRA-POOL"}
	pools = append(pools, newPool)
	assert.NotEqual(len(pools), len(op.spa.StorageReservations))
	fc.RetPoolListObj = &pool.PoolListOK{Payload: pools}
	fc.RetPoolListErr = nil
	op = newOp()
	op.spa = spa
	op.poolLoadAll(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(op.pools)
	assert.NotEmpty(op.pools)
	assert.Len(op.pools, len(op.spa.StorageReservations))
	for poolID := range op.spa.StorageReservations {
		o, present := op.pools[poolID]
		assert.True(present)
		assert.NotNil(o)
		assert.EqualValues(poolID, o.Meta.ID)
	}

	tl.Logger().Info("case: poolLoadAll pool not found")
	tl.Flush()
	pools = poolListClone(poolList)
	pools = pools[1:]
	fc.RetPoolListObj = &pool.PoolListOK{Payload: pools}
	fc.RetPoolListErr = nil
	op = newOp()
	op.spa = spa
	op.poolLoadAll(ctx)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(fc.InPoolListObj)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("no longer associated", op.rhs.Request.RequestMessages[0].Message)
	assert.Nil(op.pools)

	//  ***************************** waitForLock
	vsr = vsrClone(vsrObj)

	tl.Logger().Info("case: waitForLock reservation lock error")
	tl.Flush()
	c.reservationCS.Drain() // force closure
	op = newOp()
	assert.Nil(op.reservationCST)
	op.waitForLock(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("reservation lock error", op.rhs.Request.RequestMessages[0].Message)
	assert.Nil(op.reservationCST)

	tl.Logger().Info("case: waitForLock reservation lock")
	tl.Flush()
	c.reservationCS = util.NewCriticalSectionGuard()
	op = newOp()
	op.waitForLock(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(op.reservationCST)
	assert.Equal([]string{"VSR-1"}, c.reservationCS.Status())
	tl.Flush()

	//  ***************************** spaSearch
	vsr = vsrClone(vsrObj)

	// list error
	tl.Logger().Info("case: spaSearch list error")
	tl.Flush()
	fc.InSPAListParams = nil
	fc.RetSPAListErr = fmt.Errorf("spa-list-error")
	op = newOp()
	op.cluster = cl
	op.spaSearch(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("ServicePlanAllocation list error.*spa-list-error", op.rhs.Request.RequestMessages[0].Message)
	assert.Nil(op.spa)
	assert.NotNil(fc.InSPAListParams)
	assert.EqualValues(vs.AccountID, swag.StringValue(fc.InSPAListParams.AuthorizedAccountID))
	assert.EqualValues(vs.ServicePlanID, swag.StringValue(fc.InSPAListParams.ServicePlanID))
	assert.EqualValues(cl.Meta.ID, swag.StringValue(fc.InSPAListParams.ClusterID))

	// no error, not found
	tl.Logger().Info("case: spaSearch not found")
	tl.Flush()
	fc.RetSPAListErr = nil
	fc.RetSPAListOK = &service_plan_allocation.ServicePlanAllocationListOK{}
	op = newOp()
	op.cluster = cl
	op.spaSearch(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Nil(op.spa)

	// found
	tl.Logger().Info("case: spaSearch found")
	tl.Flush()
	fc.RetSPAListErr = nil
	fc.RetSPAListOK = &service_plan_allocation.ServicePlanAllocationListOK{
		Payload: []*models.ServicePlanAllocation{spaObj},
	}
	op = newOp()
	op.cluster = cl
	op.spaSearch(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(spaObj, op.spa)

	//  ***************************** spaFetch
	vsr = vsrClone(vsrObj)
	spa = spaClone(spaObj)

	tl.Logger().Info("case: spaFetch not-found error")
	tl.Flush()
	fc.InSPAFetchID = ""
	fc.RetSPAFetchErr = centrald.ErrorNotFound
	op = newOp()
	op.rhs.Request.ServicePlanAllocationID = models.ObjIDMutable(spa.Meta.ID)
	op.spaFetch(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Nil(op.spa)
	assert.Equal("SPA-1", fc.InSPAFetchID)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("not found", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Info("case: spaFetch other error")
	tl.Flush()
	fc.InSPAFetchID = ""
	fc.RetSPAFetchErr = fmt.Errorf("spa fetch error")
	op = newOp()
	op.rhs.Request.ServicePlanAllocationID = models.ObjIDMutable(spa.Meta.ID)
	op.spaFetch(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Equal("SPA-1", fc.InSPAFetchID)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("spa fetch error", op.rhs.Request.RequestMessages[0].Message)
	assert.Nil(op.spa)

	tl.Logger().Info("case: spaFetch success")
	tl.Flush()
	fc.InSPAFetchID = ""
	fc.RetSPAFetchErr = nil
	fc.RetSPAFetchObj = spa
	op = newOp()
	op.rhs.Request.ServicePlanAllocationID = models.ObjIDMutable(spa.Meta.ID)
	op.spaFetch(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.Equal("SPA-1", fc.InSPAFetchID)
	assert.Equal(spa, op.spa)

	//  ***************************** spaCanClaimOwnership
	vsr = vsrClone(vsrObj)
	spa = spaClone(spaObj)

	tl.Logger().Info("case: spaCanClaimOwnership not-in-use")
	tl.Flush()
	spa.SystemTags = []string{}
	op = newOp()
	op.spa = spa
	op.spaCanClaimOwnership(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	tl.Logger().Info("case: spaCanClaimOwnership in-use, not owner")
	tl.Flush()
	spa.SystemTags = []string{notOwnerTag}
	op = newOp()
	op.spa = spa
	op.spaCanClaimOwnership(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)

	tl.Logger().Info("case: spaCanClaimOwnership in-use, is owner")
	tl.Flush()
	spa.SystemTags = []string{ownerTag}
	op = newOp()
	op.spa = spa
	op.spaCanClaimOwnership(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	//  ***************************** computeReservationPlan
	tl.Logger().Info("case: computeReservationPlan formula not found")
	tl.Flush()
	sfc.InFSFName = ""
	sfc.RetFSFObj = nil
	op = newOp()
	op.spa = spa
	op.computeReservationPlan(ctx)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Invalid storage formula", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal(op.spa.StorageFormula, sfc.InFSFName)
	assert.Nil(op.reservationPlan)
	assert.Zero(op.totalReservationBytes)
	assert.Zero(op.deltaReservationBytes)

	tl.Logger().Info("case: computeReservationPlan BIND ignores spaAdditionalBytes")
	tl.Flush()
	assert.EqualValues(852*units.MiB, *app.GetCspStorageType("Amazon gp2").ParcelSizeBytes) // tests are tuned for these values
	assert.EqualValues(1020*units.MiB, *app.GetCspStorageType("Amazon io1").ParcelSizeBytes)
	sfc.InFSFName = ""
	sfc.RetFSFObj = &models.StorageFormula{
		StorageComponent: map[string]models.StorageFormulaTypeElement{
			"Amazon gp2": {Percentage: swag.Int32(70), Overhead: swag.Int32(11)},
			"Amazon io1": {Percentage: swag.Int32(30)},
		},
	}
	vs.SizeBytes = swag.Int64(int64(10 * units.GiB))
	vs.SpaAdditionalBytes = swag.Int64(int64(units.GiB)) // will be ignored if set
	expPlan1 := map[string]models.StorageTypeReservation{
		"Amazon gp2": {NumMirrors: 1, SizeBytes: swag.Int64(int64(10 * 852 * units.MiB))}, // rounded up to parcel size
		"Amazon io1": {NumMirrors: 1, SizeBytes: swag.Int64(int64(4 * 1020 * units.MiB))},
	}
	sfc.RetCRPObj = &models.CapacityReservationPlan{
		StorageTypeReservations: expPlan1,
	}
	sfc.InCRPArgs = nil
	expTotal := swag.Int64Value(vs.SizeBytes)
	op = newOp()
	assert.False(op.isCCOp)
	assert.False(op.isUndoOp)
	op.spa = spa
	op.computeReservationPlan(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.Equal(op.spa.StorageFormula, sfc.InFSFName)
	assert.Equal(expPlan1, op.reservationPlan)
	assert.EqualValues(expTotal, op.totalReservationBytes)
	assert.EqualValues(expTotal, op.deltaReservationBytes)
	assert.NotNil(sfc.InCRPArgs)
	assert.Equal(swag.Int64Value(vs.SizeBytes), sfc.InCRPArgs.SizeBytes)
	assert.Equal(sfc.RetFSFObj, sfc.InCRPArgs.SF)

	tl.Logger().Info("case: computeReservationPlan UNDO_BIND, returns spaAdditionalBytes")
	tl.Flush()
	vs.SizeBytes = swag.Int64(int64(10 * units.GiB))                                     // current size
	sabTag := fmt.Sprintf("%s:%d", com.SystemTagVsrSpaAdditionalBytes, int64(units.GiB)) // already assigned, cleared in vs
	expTotal = swag.Int64Value(vs.SizeBytes) + swag.Int64Value(vs.SpaAdditionalBytes)
	vsr.SystemTags = []string{sabTag}
	vsr.VolumeSeriesRequestState = "UNDO_BINDING"
	sfc.RetCRPObj = &models.CapacityReservationPlan{
		StorageTypeReservations: expPlan1,
	}
	sfc.InCRPArgs = nil
	op = newOp()
	assert.False(op.isCCOp)
	assert.True(op.isUndoOp)
	op.spa = spa
	op.computeReservationPlan(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.Equal(op.spa.StorageFormula, sfc.InFSFName)
	assert.Equal(expPlan1, op.reservationPlan)
	assert.EqualValues(expTotal, op.totalReservationBytes)
	assert.EqualValues(expTotal, op.deltaReservationBytes)
	assert.Equal(swag.Int64Value(vs.SizeBytes), sfc.InCRPArgs.SizeBytes)
	assert.Equal(sfc.RetFSFObj, sfc.InCRPArgs.SF)

	tl.Logger().Info("case: computeReservationPlan CHANGE_CAPACITY (sizeBytes and spaAdditionalBytes)")
	tl.Flush()
	vs.SizeBytes = swag.Int64(int64(10 * units.GiB))     // current size
	vs.SpaAdditionalBytes = swag.Int64(int64(units.GiB)) // already assigned
	vsr.VolumeSeriesRequestState = com.VolReqStateChangingCapacity
	vsr.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{
		VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
			SizeBytes:          swag.Int64(int64(12 * units.GiB)), // Δ=2GiB
			SpaAdditionalBytes: swag.Int64(int64(2 * units.GiB)),  // Δ=1GiB
		},
	}
	expTotal = swag.Int64Value(vsr.VolumeSeriesCreateSpec.SizeBytes) + swag.Int64Value(vsr.VolumeSeriesCreateSpec.SpaAdditionalBytes)
	expDelta := expTotal - (swag.Int64Value(vs.SizeBytes) + swag.Int64Value(vs.SpaAdditionalBytes))
	expPlan2 := map[string]models.StorageTypeReservation{
		"Amazon gp2": {NumMirrors: 1, SizeBytes: swag.Int64(int64(12 * 852 * units.MiB))}, // rounded
		"Amazon io1": {NumMirrors: 1, SizeBytes: swag.Int64(int64(4 * 1020 * units.MiB))},
	}
	sfc.RetCRPObjL = []*models.CapacityReservationPlan{
		&models.CapacityReservationPlan{StorageTypeReservations: expPlan1},
		&models.CapacityReservationPlan{StorageTypeReservations: expPlan2},
	}
	sfc.InCRPArgsL = nil
	op = newOp()
	assert.True(op.isCCOp)
	assert.False(op.isUndoOp)
	op.spa = spa
	op.computeReservationPlan(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.Equal(op.spa.StorageFormula, sfc.InFSFName)
	assert.Equal(expPlan2, op.reservationPlan)
	assert.Equal(expTotal, op.totalReservationBytes)
	assert.Equal(expDelta, op.deltaReservationBytes)
	assert.Equal(swag.Int64Value(vs.SizeBytes), sfc.InCRPArgsL[0].SizeBytes)
	assert.Equal(sfc.RetFSFObj, sfc.InCRPArgsL[0].SF)
	assert.Equal(swag.Int64Value(vsr.VolumeSeriesCreateSpec.SizeBytes), sfc.InCRPArgsL[1].SizeBytes)
	assert.Equal(sfc.RetFSFObj, sfc.InCRPArgsL[1].SF)
	assert.Empty(sfc.RetCRPObjL)

	tl.Logger().Info("case: computeReservationPlan CHANGE_CAPACITY (no SizeBytes, reducing spaAdditionalBytes)")
	tl.Flush()
	vs.SizeBytes = swag.Int64(int64(10 * units.GiB))     // current size
	vs.SpaAdditionalBytes = swag.Int64(int64(units.GiB)) // already assigned
	vsr.VolumeSeriesRequestState = com.VolReqStateChangingCapacity
	vsr.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{
		VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
			SpaAdditionalBytes: swag.Int64(0), // Δ=-1GiB
		},
	}
	expTotal = swag.Int64Value(vs.SizeBytes) + swag.Int64Value(vsr.VolumeSeriesCreateSpec.SpaAdditionalBytes)
	expDelta = expTotal - (swag.Int64Value(vs.SizeBytes) + swag.Int64Value(vs.SpaAdditionalBytes))
	sfc.RetCRPObj = &models.CapacityReservationPlan{StorageTypeReservations: expPlan1}
	sfc.RetCRPObjL = nil
	sfc.InCRPArgs = nil
	op = newOp()
	assert.True(op.isCCOp)
	assert.False(op.isUndoOp)
	op.spa = spa
	curPlan := op.planFromFormula(sfc.RetFSFObj, int64(10*units.GiB)) // using same formula as above
	op.computeReservationPlan(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.Equal(op.spa.StorageFormula, sfc.InFSFName)
	assert.Equal(curPlan, op.reservationPlan)
	assert.Equal(expPlan1, op.reservationPlan)
	assert.Equal(expTotal, op.totalReservationBytes)
	assert.Equal(expDelta, op.deltaReservationBytes)
	assert.Equal(swag.Int64Value(vs.SizeBytes), sfc.InCRPArgs.SizeBytes)
	assert.Equal(sfc.RetFSFObj, sfc.InCRPArgs.SF)

	//  ***************************** spaCheckForCapacity
	vs = vsClone(vsObj)
	vsr = vsrClone(vsrObj)
	spa = spaClone(spaObj)

	tl.Logger().Info("case: spaCheckForCapacity not owner")
	tl.Flush()
	spa.SystemTags = []string{notOwnerTag}
	op = newOp()
	op.spa = spa
	op.spaCheckForCapacity(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.InError)
	assert.False(op.spaHasCapacity)

	tl.Logger().Info("case: spaCheckForCapacity, delta>=0, state ok, no capacity")
	tl.Flush()
	spa.SystemTags = []string{}
	spa.ReservationState = com.SPAReservationStateOk
	spa.ReservableCapacityBytes = swag.Int64(swag.Int64Value(vs.SizeBytes))
	op = newOp()
	op.spa = spa
	op.deltaReservationBytes = swag.Int64Value(vs.SizeBytes) + 1
	op.spaCheckForCapacity(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.InError)
	assert.False(op.spaHasCapacity)

	tl.Logger().Info("case: spaCheckForCapacity, delta>=0, state not ok, capacity")
	tl.Flush()
	spa.SystemTags = []string{}
	spa.ReservationState = com.SPAReservationStateDisabled
	spa.ReservableCapacityBytes = swag.Int64(swag.Int64Value(vs.SizeBytes) + 1)
	op = newOp()
	op.spa = spa
	op.deltaReservationBytes = swag.Int64Value(vs.SizeBytes)
	op.spaCheckForCapacity(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.InError)
	assert.False(op.spaHasCapacity)

	tl.Logger().Info("case: spaCheckForCapacity delta>=0, success")
	tl.Flush()
	spa.SystemTags = []string{}
	spa.ReservationState = com.SPAReservationStateOk
	spa.ReservableCapacityBytes = swag.Int64(int64(11 * units.GiB))
	op = newOp()
	op.spa = spa
	op.deltaReservationBytes = int64(11 * units.GiB)
	op.spaCheckForCapacity(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.InError)
	assert.True(op.spaHasCapacity)

	tl.Logger().Info("case: spaCheckForCapacity delta<0")
	tl.Flush()
	spa.SystemTags = []string{}
	spa.ReservationState = com.SPAReservationStateDisabled
	spa.ReservableCapacityBytes = swag.Int64(0)
	op = newOp()
	op.spa = spa
	op.deltaReservationBytes = -1 * swag.Int64Value(vs.SizeBytes)
	op.spaCheckForCapacity(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.InError)
	assert.True(op.spaHasCapacity)

	//  ***************************** spaReserveCapacity
	vsr = vsrClone(vsrObj)
	spa = spaClone(spaObj)
	vs = vsClone(vsObj)

	tl.Logger().Info("case: spaReserveCapacity planOnly")
	tl.Flush()
	fc.RetSPAUpdaterErr = fmt.Errorf("spa-update-error")
	op = newOp()
	op.planOnly = true
	op.spa = spa
	op.totalReservationBytes = 2 * swag.Int64Value(vs.SizeBytes)
	op.deltaReservationBytes = op.totalReservationBytes
	op.spaReserveCapacity(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Plan: Reserve 2GiB .*2GiB.* in SPA", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Info("case: spaReserveCapacity updater error")
	tl.Flush()
	fc.RetSPAUpdaterErr = fmt.Errorf("spa-update-error")
	op = newOp()
	op.spa = spa
	op.spaHasCapacity = true
	op.totalReservationBytes = swag.Int64Value(vs.SizeBytes)
	op.deltaReservationBytes = op.totalReservationBytes
	op.spaReserveCapacity(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("ServicePlanAllocation update error.*spa-update-error", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal(ctx, fc.InSPAUpdaterCtx)
	assert.EqualValues(op.spa.Meta.ID, fc.InSPAUpdaterID)
	assert.Equal([]string{"systemTags"}, fc.InSPAUpdaterItems.Append)
	assert.Equal([]string{"reservableCapacityBytes"}, fc.InSPAUpdaterItems.Set)
	assert.Nil(fc.InSPAUpdaterItems.Remove)
	assert.True(op.spaHasCapacity)

	tl.Logger().Info("case: spaReserveCapacity inject failure")
	tl.Flush()
	c.rei.SetProperty("bind-fail-in-reserve", &rei.Property{BoolValue: true})
	fc.RetSPAUpdaterErr = nil
	op = newOp()
	op.spa = spa
	op.spaHasCapacity = true
	op.totalReservationBytes = swag.Int64Value(vs.SizeBytes)
	op.deltaReservationBytes = op.totalReservationBytes
	op.spaReserveCapacity(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("ServicePlanAllocation update error.*bind-fail-in-reserve", op.rhs.Request.RequestMessages[0].Message)
	assert.True(op.spaHasCapacity)

	tl.Logger().Info("case: spaReserveCapacity inject spaChange")
	tl.Flush()
	c.rei.SetProperty("bind-spa-change-in-reserve", &rei.Property{BoolValue: true})
	fc.RetSPAUpdaterErr = nil
	op = newOp()
	op.spa = spa
	op.spaHasCapacity = true
	op.totalReservationBytes = swag.Int64Value(vs.SizeBytes)
	op.deltaReservationBytes = op.totalReservationBytes
	op.spaReserveCapacity(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 2)
	assert.Regexp("bind-spa-change-in-reserve", op.rhs.Request.RequestMessages[0].Message)
	assert.Regexp("ServicePlanAllocation update error.*state or capacity changed externally", op.rhs.Request.RequestMessages[1].Message)
	assert.False(op.spaHasCapacity)

	tl.Logger().Info("case: spaReserveCapacity spaChange (state)")
	tl.Flush()
	fc.RetSPAUpdaterErr = nil
	fc.RetSPAUpdaterObj = nil
	fc.ForceFetchSPAUpdater = true // force 2nd call
	fc.FetchSPAUpdaterObj = spaClone(spaObj)
	fc.FetchSPAUpdaterObj.ReservationState = com.SPAReservationStateDisabled
	op = newOp()
	op.spa = spa
	op.spaHasCapacity = true
	op.totalReservationBytes = swag.Int64Value(vs.SizeBytes)
	op.deltaReservationBytes = op.totalReservationBytes
	op.spaReserveCapacity(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("ServicePlanAllocation update error.*state or capacity changed externally", op.rhs.Request.RequestMessages[0].Message)
	assert.False(op.spaHasCapacity)

	tl.Logger().Info("case: spaReserveCapacity spaChange (rcb)")
	tl.Flush()
	fc.RetSPAUpdaterErr = nil
	fc.RetSPAUpdaterObj = nil
	fc.ForceFetchSPAUpdater = true // force 2nd call
	fc.FetchSPAUpdaterObj = spaClone(spaObj)
	fc.FetchSPAUpdaterObj.ReservationState = com.SPAReservationStateOk
	fc.FetchSPAUpdaterObj.ReservableCapacityBytes = swag.Int64(swag.Int64Value(vs.SizeBytes) - 1)
	op = newOp()
	op.spa = spa
	op.spaHasCapacity = true
	op.totalReservationBytes = swag.Int64Value(vs.SizeBytes)
	op.deltaReservationBytes = op.totalReservationBytes
	op.spaReserveCapacity(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("ServicePlanAllocation update error.*state or capacity changed externally", op.rhs.Request.RequestMessages[0].Message)
	assert.False(op.spaHasCapacity)

	tl.Logger().Info("case: spaReserveCapacity, delta>=0, success")
	tl.Flush()
	fc.RetSPAUpdaterErr = nil
	fc.RetSPAUpdaterObj = nil
	fc.ForceFetchSPAUpdater = false
	op = newOp()
	op.spa = spa
	op.spa.ReservationState = com.SPAReservationStateOk
	op.spa.ReservableCapacityBytes = swag.Int64(2*swag.Int64Value(vs.SizeBytes) + 1)
	op.spaHasCapacity = true
	op.totalReservationBytes = 2 * swag.Int64Value(vs.SizeBytes)
	op.deltaReservationBytes = op.totalReservationBytes
	op.spaReserveCapacity(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.True(op.spaHasCapacity)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.EqualValues([]string{ownerTag}, op.spa.SystemTags)
	assert.Equal(int64(1), swag.Int64Value(op.spa.ReservableCapacityBytes))
	assert.True(op.vsrOwnsSpa())

	tl.Logger().Info("case: spaReserveCapacity, delta<0, success")
	tl.Flush()
	fc.RetSPAUpdaterErr = nil
	fc.RetSPAUpdaterObj = nil
	fc.ForceFetchSPAUpdater = false
	op = newOp()
	op.spa = spa
	spa.SystemTags = nil
	assert.False(op.vsrOwnsSpa())
	op.spa.ReservationState = com.SPAReservationStateDisabled
	op.spa.ReservableCapacityBytes = swag.Int64(0)
	op.spaHasCapacity = true
	op.deltaReservationBytes = -1 * int64(units.GiB)
	op.spaReserveCapacity(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.True(op.spaHasCapacity)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.EqualValues([]string{ownerTag}, op.spa.SystemTags)
	assert.Equal(int64(1*units.GiB), swag.Int64Value(op.spa.ReservableCapacityBytes))
	assert.True(op.vsrOwnsSpa())

	tl.Logger().Info("case: spaReserveCapacity already updated")
	tl.Flush()
	fc.RetSPAUpdaterErr = fmt.Errorf("spa-update-error") // fail if called
	fc.RetSPAUpdaterObj = nil
	fc.ForceFetchSPAUpdater = false
	assert.True(op.vsrOwnsSpa()) // use same op
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.True(op.spaHasCapacity)
	op.spaReserveCapacity(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.True(op.spaHasCapacity)

	//  ***************************** spaReturnCapacity
	vsr = vsrClone(vsrObj)
	spa = spaClone(spaObj)
	vs = vsClone(vsObj)

	tl.Logger().Info("case: spaReturnCapacity updater error")
	tl.Flush()
	fc.RetSPAUpdaterErr = fmt.Errorf("spa-update-error")
	op = newOp()
	op.spa = spa
	op.deltaReservationBytes = swag.Int64Value(vs.SizeBytes)
	op.spaReturnCapacity(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("ServicePlanAllocation update error.*spa-update-error", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal(ctx, fc.InSPAUpdaterCtx)
	assert.EqualValues(op.spa.Meta.ID, fc.InSPAUpdaterID)
	assert.Equal([]string{"systemTags"}, fc.InSPAUpdaterItems.Remove)
	assert.Equal([]string{"reservableCapacityBytes"}, fc.InSPAUpdaterItems.Set)
	assert.Nil(fc.InSPAUpdaterItems.Append)

	tl.Logger().Info("case: spaReturnCapacity inject failure")
	tl.Flush()
	c.rei.SetProperty("bind-fail-in-return", &rei.Property{BoolValue: true})
	fc.RetSPAUpdaterErr = nil
	op = newOp()
	op.spa = spa
	op.deltaReservationBytes = swag.Int64Value(vs.SizeBytes)
	op.spaReturnCapacity(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("ServicePlanAllocation update error.*bind-fail-in-return", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Info("case: spaReturnCapacity delta>=0, success")
	tl.Flush()
	fc.RetSPAUpdaterErr = nil
	fc.RetSPAUpdaterObj = nil
	fc.ForceFetchSPAUpdater = false
	op = newOp()
	op.spa = spa
	op.spa.ReservableCapacityBytes = swag.Int64(1)
	op.deltaReservationBytes = 2 * swag.Int64Value(vs.SizeBytes)
	op.spaReturnCapacity(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.EqualValues([]string{ownerTag}, op.spa.SystemTags)
	assert.Equal(op.deltaReservationBytes+1, swag.Int64Value(op.spa.ReservableCapacityBytes))

	tl.Logger().Info("case: spaReturnCapacity delta<0, success")
	tl.Flush()
	fc.RetSPAUpdaterErr = nil
	fc.RetSPAUpdaterObj = nil
	fc.ForceFetchSPAUpdater = false
	op = newOp()
	op.spa = spa
	op.spa.ReservableCapacityBytes = swag.Int64(1000)
	op.deltaReservationBytes = -1000
	op.spaReturnCapacity(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.EqualValues([]string{ownerTag}, op.spa.SystemTags)
	assert.EqualValues(0, swag.Int64Value(op.spa.ReservableCapacityBytes))

	//  ***************************** spaSetOrClearInUse (via wrappers)
	vsr = vsrClone(vsrObj)
	spa = spaClone(spaObj)

	tl.Logger().Info("case: spaSetInUse planOnly")
	tl.Flush()
	fc.InSPAUpdaterID = ""
	fc.InSPAUpdaterItems = nil
	fc.RetSPAUpdaterErr = fmt.Errorf("spa-update-error")
	op = newOp()
	op.planOnly = true
	op.spa = spa
	op.spaSetInUse(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Plan: Mark ServicePlanAllocation object in-use", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Info("case: spaClearInUse planOnly")
	tl.Flush()
	fc.InSPAUpdaterID = ""
	fc.InSPAUpdaterItems = nil
	fc.RetSPAUpdaterErr = fmt.Errorf("spa-update-error")
	op = newOp()
	op.planOnly = true
	op.spa = spa
	op.spaClearInUse(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Plan: Mark ServicePlanAllocation object not in-use", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Info("case: spaSetInUse updater error")
	tl.Flush()
	fc.InSPAUpdaterID = ""
	fc.InSPAUpdaterItems = nil
	fc.RetSPAUpdaterErr = fmt.Errorf("spa-update-error")
	op = newOp()
	op.spa = spa
	op.spaSetInUse(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("ServicePlanAllocation update error.*spa-update-error", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Info("case: spaSetInUse inject failure")
	tl.Flush()
	c.rei.SetProperty("bind-fail-set-in-use", &rei.Property{BoolValue: true})
	fc.InSPAUpdaterID = ""
	fc.InSPAUpdaterItems = nil
	fc.RetSPAUpdaterErr = nil
	op = newOp()
	op.spa = spa
	op.spaSetInUse(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("ServicePlanAllocation update error.*bind-fail-set-in-use", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal(ctx, fc.InSPAUpdaterCtx)
	assert.EqualValues(op.spa.Meta.ID, fc.InSPAUpdaterID)
	assert.Equal([]string{"systemTags"}, fc.InSPAUpdaterItems.Append)
	assert.Nil(fc.InSPAUpdaterItems.Set)
	assert.Nil(fc.InSPAUpdaterItems.Remove)

	tl.Logger().Info("case: spaClearInUse inject failure")
	tl.Flush()
	c.rei.SetProperty("bind-fail-clear-in-use", &rei.Property{BoolValue: true})
	fc.InSPAUpdaterID = ""
	fc.InSPAUpdaterItems = nil
	fc.RetSPAUpdaterErr = nil
	op = newOp()
	op.spa = spa
	op.spaClearInUse(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("ServicePlanAllocation update error.*bind-fail-clear-in-use", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal(ctx, fc.InSPAUpdaterCtx)
	assert.EqualValues(op.spa.Meta.ID, fc.InSPAUpdaterID)
	assert.Equal([]string{"systemTags"}, fc.InSPAUpdaterItems.Remove)
	assert.Nil(fc.InSPAUpdaterItems.Set)
	assert.Nil(fc.InSPAUpdaterItems.Append)

	tl.Logger().Info("case: spaSetInUse success")
	tl.Flush()
	fc.InSPAUpdaterID = ""
	fc.InSPAUpdaterItems = nil
	op = newOp()
	op.spa = spa
	op.spaSetInUse(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.Equal(ctx, fc.InSPAUpdaterCtx)
	assert.EqualValues(op.spa.Meta.ID, fc.InSPAUpdaterID)
	assert.Equal([]string{"systemTags"}, fc.InSPAUpdaterItems.Append)
	assert.Nil(fc.InSPAUpdaterItems.Set)
	assert.Nil(fc.InSPAUpdaterItems.Remove)
	assert.EqualValues([]string{ownerTag}, op.spa.SystemTags)

	//  ***************************** vsrSetCapacityReservationResult
	vsr = vsrClone(vsrObj)
	spa = spaClone(spaObj)
	vs = vsClone(vsObj)
	pools = poolListClone(poolList)

	tl.Logger().Info("case: vsrSetCapacityReservationResult success")
	tl.Flush()
	sfc.InFSFName = ""
	sfc.RetFSFObj = &models.StorageFormula{
		StorageComponent: map[string]models.StorageFormulaTypeElement{
			"Amazon gp2": {Percentage: swag.Int32(70)}, // SP-1
			"Amazon io1": {Percentage: swag.Int32(30)}, // SP-2
		},
	}
	vs.CapacityAllocations = map[string]models.CapacityAllocation{}
	vs.CapacityAllocations["SP-1"] = models.CapacityAllocation{ReservedBytes: swag.Int64(1000), ConsumedBytes: swag.Int64(10)}
	vs.CapacityAllocations["SP-3"] = models.CapacityAllocation{ReservedBytes: swag.Int64(3000), ConsumedBytes: swag.Int64(30)}
	spa.StorageReservations = map[string]models.StorageTypeReservation{}
	spa.StorageReservations["SP-1"] = models.StorageTypeReservation{NumMirrors: 1}
	spa.StorageReservations["SP-2"] = models.StorageTypeReservation{NumMirrors: 2}
	expCRR := &models.CapacityReservationResult{ // expected CRR
		CurrentReservations: map[string]models.PoolReservation{},
		DesiredReservations: map[string]models.PoolReservation{},
	}
	expCRR.CurrentReservations["SP-1"] = models.PoolReservation{SizeBytes: swag.Int64(1000)}
	expCRR.CurrentReservations["SP-3"] = models.PoolReservation{SizeBytes: swag.Int64(3000)}
	expCRR.DesiredReservations["SP-1"] = models.PoolReservation{SizeBytes: swag.Int64(int64(8 * units.GiB))}
	expCRR.DesiredReservations["SP-2"] = models.PoolReservation{SizeBytes: swag.Int64(2 * int64(3*units.GiB))}
	op = newOp()
	op.spa = spa
	op.pools = make(map[string]*models.Pool)
	for _, p := range pools {
		op.pools[string(p.Meta.ID)] = p
	}
	op.reservationPlan = map[string]models.StorageTypeReservation{
		"Amazon gp2": {NumMirrors: 1, SizeBytes: swag.Int64(int64(8 * units.GiB))},
		"Amazon io1": {NumMirrors: 2, SizeBytes: swag.Int64(int64(3 * units.GiB))},
	}
	op.vsrSetCapacityReservationResult(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(op.rhs.Request.CapacityReservationResult)
	for _, k := range []string{"SP-1", "SP-2", "SP-3"} {
		crr := op.rhs.Request.CapacityReservationResult
		switch k {
		case "SP-1":
			assert.EqualValues(1000, swag.Int64Value(crr.CurrentReservations[k].SizeBytes))
			assert.EqualValues(swag.Int64Value(expCRR.DesiredReservations[k].SizeBytes), swag.Int64Value(crr.DesiredReservations[k].SizeBytes))
		case "SP-2":
			assert.EqualValues(swag.Int64Value(expCRR.DesiredReservations[k].SizeBytes), swag.Int64Value(crr.DesiredReservations[k].SizeBytes))
		case "SP-3":
			assert.EqualValues(3000, swag.Int64Value(crr.CurrentReservations[k].SizeBytes))
		}
	}

	//  ***************************** vsUpdate (via wrappers)
	vsr = vsrClone(vsrObj)
	spa = spaClone(spaObj)
	vs = vsClone(vsObj)

	tl.Logger().Info("case: vsSet planOnly")
	tl.Flush()
	fc.RetVSUpdaterErr = fmt.Errorf("vs-update-error")
	op = newOp()
	op.planOnly = true
	op.cluster = cl
	op.spa = spa
	op.vsSet(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Plan: Commit changes to VolumeSeries", op.rhs.Request.RequestMessages[0].Message)

	// BIND cases
	tl.Logger().Info("case: BIND vsSet updater error")
	tl.Flush()
	fc.InVSUpdaterCtx = nil
	fc.RetVSUpdaterErr = fmt.Errorf("vs-update-error")
	op = newOp()
	assert.False(op.isCCOp)
	op.cluster = cl
	op.spa = spa
	op.vsSet(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("VolumeSeries update error.*vs-update-error", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal(ctx, fc.InVSUpdaterCtx)

	tl.Logger().Info("case: BIND vsSet inject failure")
	tl.Flush()
	c.rei.SetProperty("bind-fail-in-vs-set-bound", &rei.Property{BoolValue: true})
	fc.RetVSUpdaterErr = nil
	fc.InVSUpdaterID = ""
	op = newOp()
	assert.False(op.isCCOp)
	op.cluster = cl
	op.spa = spa
	op.vsSet(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("VolumeSeries update error.*bind-fail-in-vs-set-bound", op.rhs.Request.RequestMessages[0].Message)
	assert.True(op.crrSetDesired)

	tl.Logger().Info("case: BIND vsSet success")
	tl.Flush()
	vs.SystemTags = []string{"tag.not.removed", com.SystemTagVolClusterPrefix + "tag.removed"}
	vs.CapacityAllocations = map[string]models.CapacityAllocation{}
	vs.CapacityAllocations["SP-1"] = models.CapacityAllocation{ReservedBytes: swag.Int64(1000), ConsumedBytes: swag.Int64(10)}
	vs.CapacityAllocations["SP-3"] = models.CapacityAllocation{ReservedBytes: swag.Int64(3000), ConsumedBytes: swag.Int64(30)}
	vs.SpaAdditionalBytes = swag.Int64(1000)
	vs.LifecycleManagementData = &models.LifecycleManagementData{}
	// match expCA values with expCRR from earlier
	expCA := map[string]models.CapacityAllocation{}
	expCA["SP-1"] = models.CapacityAllocation{ReservedBytes: swag.Int64(int64(8 * units.GiB)), ConsumedBytes: swag.Int64(10)}
	expCA["SP-2"] = models.CapacityAllocation{ReservedBytes: swag.Int64(2 * int64(3*units.GiB))}
	expCA["SP-3"] = models.CapacityAllocation{ReservedBytes: swag.Int64(3000), ConsumedBytes: swag.Int64(30)}
	fc.InVSUpdaterID = ""
	fc.ModVSUpdaterObj = nil
	op = newOp()
	assert.False(op.isCCOp)
	op.cluster = cl
	op.spa = spa
	op.rhs.Request.CapacityReservationResult = expCRR // from earlier
	op.vsSet(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.True(op.crrSetDesired)
	assert.Equal(ctx, fc.InVSUpdaterCtx)
	assert.Equal("VS-1", fc.InVSUpdaterID)
	assert.Nil(fc.InVSUpdaterItems.Append)
	assert.Equal([]string{"volumeSeriesState", "messages", "boundClusterId", "servicePlanAllocationId", "capacityAllocations", "systemTags", "spaAdditionalBytes", "lifecycleManagementData"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Remove)
	assert.NotNil(fc.ModVSUpdaterObj)
	vs = fc.ModVSUpdaterObj
	assert.Equal(com.VolStateBound, vs.VolumeSeriesState)
	assert.EqualValues(cl.Meta.ID, vs.BoundClusterID)
	assert.EqualValues(spa.Meta.ID, vs.ServicePlanAllocationID)
	assert.EqualValues(0, swag.Int64Value(vs.SpaAdditionalBytes))
	assert.NotEmpty(op.rhs.Request.CapacityReservationResult.DesiredReservations)
	numFound := 0
	for spID, spr := range op.rhs.Request.CapacityReservationResult.DesiredReservations {
		assert.Contains(vs.CapacityAllocations, spID)
		cap, ok := vs.CapacityAllocations[spID]
		assert.True(ok, spID)
		expCap, ok := expCA[spID]
		assert.True(ok, spID)
		assert.Equal(swag.Int64Value(expCap.ConsumedBytes), swag.Int64Value(cap.ConsumedBytes), spID)
		assert.Equal(swag.Int64Value(expCap.ReservedBytes), swag.Int64Value(cap.ReservedBytes), spID)
		assert.Equal(swag.Int64Value(spr.SizeBytes), swag.Int64Value(cap.ReservedBytes), spID)
		numFound++
	}
	assert.True(numFound <= len(op.rhs.Request.CapacityReservationResult.DesiredReservations))
	for spID, ca := range expCA {
		assert.Contains(vs.CapacityAllocations, spID)
		cap, ok := vs.CapacityAllocations[spID]
		assert.True(ok, spID)
		assert.Equal(swag.Int64Value(ca.ConsumedBytes), swag.Int64Value(cap.ConsumedBytes), spID)
		assert.Equal(swag.Int64Value(ca.ReservedBytes), swag.Int64Value(cap.ReservedBytes), spID)
	}
	assert.Len(vs.Messages, 1)
	assert.Regexp("State change", vs.Messages[0].Message)
	assert.EqualValues([]string{"tag.not.removed"}, vs.SystemTags)
	assert.Nil(vs.LifecycleManagementData)

	tl.Logger().Info("case: BIND vsSet already updated")
	tl.Flush()
	fc.RetVSUpdaterErr = fmt.Errorf("vs-update-error") // fail if called
	assert.Equal(com.VolStateBound, op.rhs.VolumeSeries.VolumeSeriesState)
	op.vsSet(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	tl.Logger().Info("case: UNDO_BINDING vsUndo inject failure")
	tl.Flush()
	vsr.VolumeSeriesRequestState = "UNDO_BINDING"
	c.rei.SetProperty("bind-fail-in-vs-set-unbound", &rei.Property{BoolValue: true})
	fc.RetVSUpdaterErr = nil
	fc.InVSUpdaterID = ""
	vs = vsClone(vsObj)
	op = newOp()
	assert.False(op.isCCOp)
	op.cluster = cl
	op.spa = spa
	op.rhs.VolumeSeries.VolumeSeriesState = com.VolStateBound
	op.vsUndo(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("VolumeSeries update error.*bind-fail-in-vs-set-unbound", op.rhs.Request.RequestMessages[0].Message)
	assert.False(op.crrSetDesired)

	tl.Logger().Info("case: UNDO_BINDING vsUndo success (UNBOUND)")
	tl.Flush()
	fc.InVSUpdaterID = ""
	fc.ModVSUpdaterObj = nil
	vs = vsClone(vsObj)
	vs.SpaAdditionalBytes = swag.Int64(1000)
	vs.LifecycleManagementData = &models.LifecycleManagementData{}
	op = newOp()
	assert.False(op.isCCOp)
	op.cluster = cl
	op.spa = spa
	op.rhs.VolumeSeries.VolumeSeriesState = com.VolStateBound
	op.rhs.Request.CapacityReservationResult = expCRR // from earlier
	op.vsUndo(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.False(op.crrSetDesired)
	assert.Equal(ctx, fc.InVSUpdaterCtx)
	assert.Equal("VS-1", fc.InVSUpdaterID)
	assert.Nil(fc.InVSUpdaterItems.Append)
	assert.Equal([]string{"volumeSeriesState", "messages", "boundClusterId", "servicePlanAllocationId", "capacityAllocations", "systemTags", "spaAdditionalBytes", "lifecycleManagementData"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Remove)
	assert.NotNil(fc.ModVSUpdaterObj)
	vs = fc.ModVSUpdaterObj
	assert.Equal(com.VolStateUnbound, vs.VolumeSeriesState)
	assert.Empty(vs.BoundClusterID)
	assert.Empty(vs.ServicePlanAllocationID)
	assert.Equal(0, len(vs.CapacityAllocations))
	assert.Len(vs.Messages, 1)
	assert.Regexp("State change", vs.Messages[0].Message)
	assert.EqualValues(0, swag.Int64Value(vs.SpaAdditionalBytes))
	assert.Nil(vs.LifecycleManagementData)

	tl.Logger().Info("case: UNDO_BINDING vsUndo success (UNBIND, with CRR)")
	tl.Flush()
	fc.InVSUpdaterID = ""
	fc.ModVSUpdaterObj = nil
	vs = vsClone(vsObj)
	vs.SpaAdditionalBytes = swag.Int64(1000)
	op = newOp()
	assert.False(op.isCCOp)
	op.cluster = cl
	op.spa = spa
	op.rhs.VolumeSeries.VolumeSeriesState = com.VolStateBound
	op.rhs.HasUnbind = true
	op.rhs.Request.CapacityReservationResult = expCRR // from earlier
	op.vsUndo(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.False(op.crrSetDesired)
	assert.Equal(ctx, fc.InVSUpdaterCtx)
	assert.Equal("VS-1", fc.InVSUpdaterID)
	assert.Nil(fc.InVSUpdaterItems.Append)
	assert.Equal([]string{"volumeSeriesState", "messages", "boundClusterId", "servicePlanAllocationId", "capacityAllocations", "systemTags", "spaAdditionalBytes", "lifecycleManagementData"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Remove)
	assert.NotNil(fc.ModVSUpdaterObj)
	vs = fc.ModVSUpdaterObj
	assert.Equal(com.VolStateUnbound, vs.VolumeSeriesState)
	assert.Empty(vs.BoundClusterID)
	assert.Empty(vs.ServicePlanAllocationID)
	assert.Equal(0, len(vs.CapacityAllocations))
	assert.Len(vs.Messages, 1)
	assert.Regexp("State change", vs.Messages[0].Message)
	assert.EqualValues(0, swag.Int64Value(vs.SpaAdditionalBytes))

	tl.Logger().Info("case: UNDO_BINDING vsUndo success (UNBIND, no CRR)")
	tl.Flush()
	fc.InVSUpdaterID = ""
	fc.ModVSUpdaterObj = nil
	vs = vsClone(vsObj)
	op = newOp()
	assert.False(op.isCCOp)
	op.cluster = cl
	op.spa = spa
	op.rhs.VolumeSeries.VolumeSeriesState = com.VolStateBound
	op.rhs.HasUnbind = true
	assert.Nil(op.rhs.Request.CapacityReservationResult)
	op.vsUndo(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(op.rhs.Request.CapacityReservationResult)
	assert.Empty(op.rhs.Request.CapacityReservationResult.CurrentReservations)
	assert.Nil(op.rhs.Request.CapacityReservationResult.DesiredReservations)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.False(op.crrSetDesired)
	assert.Equal(ctx, fc.InSPAUpdaterCtx)
	assert.Equal("VS-1", fc.InVSUpdaterID)
	assert.Nil(fc.InVSUpdaterItems.Append)
	assert.Equal([]string{"volumeSeriesState", "messages", "boundClusterId", "servicePlanAllocationId", "capacityAllocations", "systemTags", "spaAdditionalBytes", "lifecycleManagementData"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Remove)
	assert.NotNil(fc.ModVSUpdaterObj)
	vs = fc.ModVSUpdaterObj
	assert.Equal(com.VolStateUnbound, vs.VolumeSeriesState)
	assert.Empty(vs.BoundClusterID)
	assert.Empty(vs.ServicePlanAllocationID)
	assert.Empty(vs.CapacityAllocations)
	assert.Len(vs.Messages, 1)
	assert.Regexp("State change", vs.Messages[0].Message)

	tl.Logger().Info("case: UNDO_BINDING vsUndo success (DELETING, with CRR)")
	tl.Flush()
	fc.InVSUpdaterID = ""
	fc.ModVSUpdaterObj = nil
	vs = vsClone(vsObj)
	vs.SpaAdditionalBytes = swag.Int64(1000)
	op = newOp()
	assert.False(op.isCCOp)
	op.cluster = cl
	op.spa = spa
	op.rhs.VolumeSeries.VolumeSeriesState = com.VolStateBound
	op.rhs.HasDelete = true
	op.rhs.Request.CapacityReservationResult = expCRR // from earlier
	op.vsUndo(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.False(op.crrSetDesired)
	assert.Equal(ctx, fc.InVSUpdaterCtx)
	assert.Equal("VS-1", fc.InVSUpdaterID)
	assert.Nil(fc.InVSUpdaterItems.Append)
	assert.Equal([]string{"volumeSeriesState", "messages", "boundClusterId", "servicePlanAllocationId", "capacityAllocations", "systemTags", "spaAdditionalBytes", "lifecycleManagementData"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Remove)
	assert.NotNil(fc.ModVSUpdaterObj)
	vs = fc.ModVSUpdaterObj
	assert.Equal(com.VolStateDeleting, vs.VolumeSeriesState)
	assert.Empty(vs.BoundClusterID)
	assert.Empty(vs.ServicePlanAllocationID)
	assert.Equal(0, len(vs.CapacityAllocations))
	assert.Len(vs.Messages, 1)
	assert.Regexp("State change", vs.Messages[0].Message)
	assert.EqualValues(0, swag.Int64Value(vs.SpaAdditionalBytes))

	tl.Logger().Info("case: UNDO_BINDING vsUndo success (DELETING, no CRR)")
	tl.Flush()
	fc.InVSUpdaterID = ""
	fc.ModVSUpdaterObj = nil
	vs = vsClone(vsObj)
	op = newOp()
	assert.False(op.isCCOp)
	op.cluster = cl
	op.spa = spa
	op.rhs.VolumeSeries.VolumeSeriesState = com.VolStateBound
	op.rhs.HasDelete = true
	assert.Nil(op.rhs.Request.CapacityReservationResult)
	op.vsUndo(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(op.rhs.Request.CapacityReservationResult)
	assert.Empty(op.rhs.Request.CapacityReservationResult.CurrentReservations)
	assert.Nil(op.rhs.Request.CapacityReservationResult.DesiredReservations)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.False(op.crrSetDesired)
	assert.Equal(ctx, fc.InSPAUpdaterCtx)
	assert.Equal("VS-1", fc.InVSUpdaterID)
	assert.Nil(fc.InVSUpdaterItems.Append)
	assert.Equal([]string{"volumeSeriesState", "messages", "boundClusterId", "servicePlanAllocationId", "capacityAllocations", "systemTags", "spaAdditionalBytes", "lifecycleManagementData"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Remove)
	assert.NotNil(fc.ModVSUpdaterObj)
	vs = fc.ModVSUpdaterObj
	assert.Equal(com.VolStateDeleting, vs.VolumeSeriesState)
	assert.Empty(vs.BoundClusterID)
	assert.Empty(vs.ServicePlanAllocationID)
	assert.Empty(vs.CapacityAllocations)
	assert.Len(vs.Messages, 1)
	assert.Regexp("State change", vs.Messages[0].Message)

	tl.Logger().Info("case: UNDO_BINDING vsUndo already updated")
	tl.Flush()
	fc.RetVSUpdaterErr = fmt.Errorf("vs-update-error") // fail if called
	assert.Equal(com.VolStateDeleting, op.rhs.VolumeSeries.VolumeSeriesState)
	op.vsUndo(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	// UNDO_CHANGING_CAPACITY cases supplement UNDO_BINDING cases above without repeating all details
	vsr.VolumeSeriesRequestState = "UNDO_CHANGING_CAPACITY"
	vsr.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{
		VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
			SizeBytes:          swag.Int64(int64(12 * units.GiB)),
			SpaAdditionalBytes: swag.Int64(0),
		},
	}
	sTag = util.NewTagList(vsr.SystemTags)
	sTag.Set(com.SystemTagVsrOldSizeBytes, "10737418240")
	sTag.Set(com.SystemTagVsrOldSpaAdditionalBytes, "1073741824")
	vsr.SystemTags = sTag.List()

	tl.Logger().Info("case: UNDO_CHANGING_CAPACITY vsUndo success")
	tl.Flush()
	fc.InVSUpdaterID = ""
	fc.ModVSUpdaterObj = nil
	fc.RetVSUpdaterErr = nil
	vs = vsClone(vsObj)
	vs.BoundClusterID = models.ObjIDMutable(cl.Meta.ID)
	vs.ServicePlanAllocationID = models.ObjIDMutable(spa.Meta.ID)
	vs.SizeBytes = vsr.VolumeSeriesCreateSpec.SizeBytes
	vs.SpaAdditionalBytes = vsr.VolumeSeriesCreateSpec.SpaAdditionalBytes
	op = newOp()
	assert.True(op.isCCOp)
	op.cluster = cl
	op.spa = spa
	op.rhs.VolumeSeries.VolumeSeriesState = com.VolStateBound
	op.rhs.HasChangeCapacity = true
	assert.Nil(op.rhs.Request.CapacityReservationResult)
	op.vsUndo(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Nil(op.rhs.Request.CapacityReservationResult)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.False(op.crrSetDesired)
	assert.Equal(ctx, fc.InSPAUpdaterCtx)
	assert.Equal("VS-1", fc.InVSUpdaterID)
	assert.Nil(fc.InVSUpdaterItems.Append)
	assert.Equal([]string{"messages", "sizeBytes", "spaAdditionalBytes"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Remove)
	assert.NotNil(fc.ModVSUpdaterObj)
	vs = fc.ModVSUpdaterObj
	assert.Equal(com.VolStateBound, vs.VolumeSeriesState)
	assert.EqualValues(cl.Meta.ID, vs.BoundClusterID)
	assert.EqualValues(spa.Meta.ID, vs.ServicePlanAllocationID)
	if assert.Len(vs.Messages, 2) {
		assert.Regexp("sizeBytes change.*⇒ 10GiB", vs.Messages[0].Message)
		assert.Regexp("spaAdditionalBytes change.*⇒ 1GiB", vs.Messages[1].Message)
	}
	assert.EqualValues(10*units.GiB, swag.Int64Value(vs.SizeBytes))
	assert.EqualValues(1*units.GiB, swag.Int64Value(vs.SpaAdditionalBytes))

	tl.Logger().Info("case: UNDO_CHANGING_CAPACITY vsUndo not required")
	tl.Flush()
	fc.InVSUpdaterID = ""
	fc.ModVSUpdaterObj = nil
	fc.RetVSUpdaterErr = fmt.Errorf("vs-update-error") // fail if called
	sTag.Delete(com.SystemTagVsrOldSizeBytes)
	sTag.Delete(com.SystemTagVsrOldSpaAdditionalBytes)
	vsr.SystemTags = sTag.List()
	vs = vsClone(vsObj)
	vs.BoundClusterID = models.ObjIDMutable(cl.Meta.ID)
	vs.ServicePlanAllocationID = models.ObjIDMutable(spa.Meta.ID)
	vs.SizeBytes = vsr.VolumeSeriesCreateSpec.SizeBytes
	vs.SpaAdditionalBytes = vsr.VolumeSeriesCreateSpec.SpaAdditionalBytes
	op = newOp()
	assert.True(op.isCCOp)
	op.cluster = cl
	op.spa = spa
	op.rhs.VolumeSeries.VolumeSeriesState = com.VolStateBound
	op.rhs.HasChangeCapacity = true
	op.vsUndo(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	// CHANGING_CAPACITY cases supplement BIND cases above without repeating all details
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "CHANGING_CAPACITY"

	tl.Logger().Info("case: CHANGE_CAPACITY vsSet inject failure, no CRR")
	tl.Flush()
	c.rei.SetProperty("chg-cap-fail-in-vs-update", &rei.Property{BoolValue: true})
	fc.RetVSUpdaterErr = nil
	fc.InVSUpdaterID = ""
	op = newOp()
	assert.True(op.isCCOp)
	assert.True(op.ccHasSizeBytes)
	assert.True(op.ccHasSpaAdditionalBytes)
	op.rhs.Request.CapacityReservationResult = nil
	op.vsSet(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("VolumeSeries update error.*chg-cap-fail-in-vs-update", op.rhs.Request.RequestMessages[0].Message)
	assert.False(op.crrSetDesired)
	assert.Equal("VS-1", fc.InVSUpdaterID)
	assert.Nil(fc.InVSUpdaterItems.Append)
	assert.Equal([]string{"messages", "sizeBytes", "spaAdditionalBytes"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Remove)

	tl.Logger().Info("case: CHANGE_CAPACITY vsSet inject failure, empty CRR, no spaAdditionalBytes")
	tl.Flush()
	c.rei.SetProperty("chg-cap-fail-in-vs-update", &rei.Property{BoolValue: true})
	fc.RetVSUpdaterErr = nil
	fc.InVSUpdaterID = ""
	op = newOp()
	assert.True(op.isCCOp)
	op.rhs.Request.CapacityReservationResult = &models.CapacityReservationResult{}
	assert.True(op.ccHasSizeBytes)
	op.ccHasSpaAdditionalBytes = false
	op.vsSet(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("VolumeSeries update error.*chg-cap-fail-in-vs-update", op.rhs.Request.RequestMessages[0].Message)
	assert.False(op.crrSetDesired)
	assert.Equal("VS-1", fc.InVSUpdaterID)
	assert.Nil(fc.InVSUpdaterItems.Append)
	assert.Equal([]string{"messages", "sizeBytes"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Remove)

	tl.Logger().Info("case: CHANGE_CAPACITY vsSet inject failure, not empty CRR")
	tl.Flush()
	c.rei.SetProperty("chg-cap-fail-in-vs-update", &rei.Property{BoolValue: true})
	fc.RetVSUpdaterErr = nil
	fc.InVSUpdaterID = ""
	op = newOp()
	assert.True(op.isCCOp)
	op.rhs.Request.CapacityReservationResult = &models.CapacityReservationResult{
		DesiredReservations: map[string]models.PoolReservation{
			"something": models.PoolReservation{},
		},
	}
	assert.True(op.ccHasSizeBytes)
	assert.True(op.ccHasSpaAdditionalBytes)
	op.vsSet(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("VolumeSeries update error.*chg-cap-fail-in-vs-update", op.rhs.Request.RequestMessages[0].Message)
	assert.True(op.crrSetDesired) // logic tested in BIND
	assert.Equal("VS-1", fc.InVSUpdaterID)
	assert.Nil(fc.InVSUpdaterItems.Append)
	assert.Equal([]string{"messages", "sizeBytes", "capacityAllocations", "spaAdditionalBytes"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Remove)

	tl.Logger().Info("case: CHANGE_CAPACITY vsSet success, UNBOUND, sizeBytes and spaAdditionalBytes updated")
	tl.Flush()
	vs.VolumeSeriesState = com.VolStateUnbound
	vs.SizeBytes = swag.Int64(swag.Int64Value(vsr.VolumeSeriesCreateSpec.SizeBytes) - 1000)
	vs.SpaAdditionalBytes = swag.Int64(swag.Int64Value(vsr.VolumeSeriesCreateSpec.SpaAdditionalBytes) + 1000)
	vs.Messages = nil
	fc.InVSUpdaterID = ""
	fc.ModVSUpdaterObj = nil
	op = newOp()
	assert.True(op.isCCOp)
	assert.True(op.ccHasSizeBytes)
	assert.True(op.ccHasSpaAdditionalBytes)
	op.rhs.Request.CapacityReservationResult = nil
	op.vsSet(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.False(op.crrSetDesired)
	assert.Equal("VS-1", fc.InVSUpdaterID)
	assert.Nil(fc.InVSUpdaterItems.Append)
	assert.Equal([]string{"messages", "sizeBytes", "spaAdditionalBytes"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Remove)
	assert.NotNil(fc.ModVSUpdaterObj)
	assert.Equal(swag.Int64Value(op.rhs.Request.VolumeSeriesCreateSpec.SizeBytes), swag.Int64Value(fc.ModVSUpdaterObj.SizeBytes))
	assert.Equal(swag.Int64Value(op.rhs.Request.VolumeSeriesCreateSpec.SpaAdditionalBytes), swag.Int64Value(fc.ModVSUpdaterObj.SpaAdditionalBytes))
	assert.Len(fc.ModVSUpdaterObj.Messages, 2)
	assert.Regexp("sizeBytes change", fc.ModVSUpdaterObj.Messages[0].Message)
	assert.Regexp("spaAdditionalBytes change", fc.ModVSUpdaterObj.Messages[1].Message)

	//  ***************************** vsrSaveVSProps
	vsr = vsrClone(vsrObj)
	vs = vsClone(vsObj)
	vs.SpaAdditionalBytes = swag.Int64(int64(units.MiB))
	sabTag = fmt.Sprintf("%s:%d", com.SystemTagVsrSpaAdditionalBytes, units.MiB)

	tl.Logger().Info("case: vsrSaveVSProps planOnly")
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.vsrSaveVSProps(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Plan: Save VS spaAdditionalBytes in VSR", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Info("case: vsrSaveVSProps spaAdditionalBytes not saved")
	tl.Flush()
	fc.InVSRUpdaterItems = nil
	fc.ModVSRUpdaterObj = nil
	fc.RetVSRUpdaterErr = nil
	op = newOp()
	op.vsrSaveVSProps(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(fc.InVSRUpdaterItems)
	assert.Equal([]string{"systemTags"}, fc.InVSRUpdaterItems.Set)
	assert.Empty(fc.InVSRUpdaterItems.Append)
	assert.Empty(fc.InVSRUpdaterItems.Remove)
	assert.EqualValues([]string{sabTag}, fc.ModVSRUpdaterObj.SystemTags)
	assert.Equal(fc.ModVSRUpdaterObj, op.rhs.Request)

	tl.Logger().Info("case: vsrSaveVSProps spaAdditionalBytes already saved")
	tl.Flush()
	fc.InVSRUpdaterItems = nil
	fc.ModVSRUpdaterObj = nil
	fc.RetVSRUpdaterErr = fmt.Errorf("should-not-fail")
	op = newOp()
	op.rhs.Request.SystemTags = []string{sabTag}
	op.vsrSaveVSProps(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Nil(fc.InVSRUpdaterItems)

	//  ***************************** vsrSetCapacityWait
	vsr = vsrClone(vsrObj)

	tl.Logger().Info("case: vsrSetCapacityWait planOnly")
	tl.Flush()
	op = newOp()
	op.planOnly = true
	op.vsrSetCapacityWait(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Plan: Set VSR state to CAPACITY_WAIT", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Info("case: vsrSetCapacityWait success")
	tl.Flush()
	op = newOp()
	op.vsrSetCapacityWait(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Equal("CAPACITY_WAIT", op.rhs.Request.VolumeSeriesRequestState)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("State.*⇒ CAPACITY_WAIT", op.rhs.Request.RequestMessages[0].Message)
	sTag = util.NewTagList(op.rhs.Request.SystemTags)
	_, found := sTag.Get(com.SystemTagVsrSpaCapacityWait)
	assert.True(found)

	//  ***************************** vsrUpdate
	vsr = vsrClone(vsrObj)
	spa = spaClone(spaObj)

	tl.Logger().Info("case: vsrUpdate planOnly")
	tl.Flush()
	fc.ModVSRUpdaterObj = nil
	fc.RetVSRUpdaterErr = fmt.Errorf("vsr update error")
	op = newOp()
	op.planOnly = true
	op.spa = spa
	op.vsrUpdate(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Plan: Update VSR.*servicePlanAllocationId=.SPA-1. storageFormula=.FORMULA1", op.rhs.Request.RequestMessages[0].Message)

	tl.Logger().Info("case: vsrUpdate error")
	tl.Flush()
	fc.ModVSRUpdaterObj = nil
	fc.RetVSRUpdaterErr = fmt.Errorf("vsr update error")
	op = newOp()
	op.spa = spa
	sTag = util.NewTagList(op.rhs.Request.SystemTags)
	sTag.Set(com.SystemTagVsrSpaCapacityWait, "")
	op.rhs.Request.SystemTags = sTag.List()
	op.vsrUpdate(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Update error.*vsr update error", op.rhs.Request.RequestMessages[0].Message)
	assert.Nil(fc.RetVSRUpdaterObj)
	sTag = util.NewTagList(op.rhs.Request.SystemTags)
	_, found = sTag.Get(com.SystemTagVsrSpaCapacityWait)
	assert.False(found)

	tl.Logger().Info("case: success stashing sizes during CCop")
	tl.Flush()
	vsr.VolumeSeriesRequestState = com.VolReqStateChangingCapacity
	vsr.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{
		VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
			SizeBytes:          swag.Int64(int64(8 * units.GiB)),
			SpaAdditionalBytes: swag.Int64(0),
		},
	}
	op = newOp()
	op.spa = spa
	sTag = util.NewTagList(op.rhs.Request.SystemTags)
	sTag.Set(com.SystemTagVsrSpaCapacityWait, "")
	sTag.Delete(com.SystemTagVsrOldSizeBytes)
	sTag.Delete(com.SystemTagVsrOldSpaAdditionalBytes)
	op.rhs.Request.SystemTags = sTag.List()
	op.rhs.VolumeSeries.SizeBytes = swag.Int64(int64(10 * units.GiB))
	op.rhs.VolumeSeries.SpaAdditionalBytes = swag.Int64(int64(units.GiB))
	fc.RetVSRUpdaterObj = op.rhs.Request
	fc.RetVSRUpdaterErr = nil
	op.vsrUpdate(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Empty(op.rhs.Request.RequestMessages)
	assert.True(op.rhs.Request == fc.RetVSRUpdaterObj)
	sTag = util.NewTagList(op.rhs.Request.SystemTags)
	_, found = sTag.Get(com.SystemTagVsrSpaCapacityWait)
	assert.False(found)
	v, _ := sTag.Get(com.SystemTagVsrOldSizeBytes)
	assert.Equal("10737418240", v)
	v, _ = sTag.Get(com.SystemTagVsrOldSpaAdditionalBytes)
	assert.Equal("1073741824", v)
}

func TestBind(t *testing.T) {
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
	sfc := &fakeSFC{}
	app.SFC = sfc
	fts := &fhk.TaskScheduler{}
	app.TaskScheduler = fts
	ctx := context.Background()
	c := &Component{}
	c.Init(app)
	assert.NotNil(c.Log)
	assert.NotNil(c.reservationCS)
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc

	cl := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID:      "CLUSTER-1",
				Version: 1,
			},
		},
		ClusterCreateOnce: models.ClusterCreateOnce{
			ClusterType: "kubernetes",
			CspDomainID: "CSP-DOMAIN-1",
		},
		ClusterMutable: models.ClusterMutable{
			ClusterCreateMutable: models.ClusterCreateMutable{
				Name: "MyCluster",
			},
		},
	}
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
	vs := vsClone(vsObj)
	vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VR-1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{
				VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
					SizeBytes:          swag.Int64(int64(10 * units.GiB)),
					SpaAdditionalBytes: swag.Int64(int64(units.GiB)),
				},
			},
		},
	}
	vsr := vsrClone(vsrObj)
	spaObj := &models.ServicePlanAllocation{}

	newFakeBindOp := func() *fakeBindOps {
		op := &fakeBindOps{}
		op.c = c
		op.ops = op
		op.rhs = &vra.RequestHandlerState{A: c.Animator, VolumeSeries: vsClone(vs), Request: vsrClone(vsr)}
		op.tl = tl
		op.classifyOp()
		assert.True(!op.isCCOp || op.ccHasSizeBytes || op.ccHasSpaAdditionalBytes)
		if op.isCCOp {
			op.rhs.Request.ServicePlanAllocationID = "SPA-1" // default assumes a bound volume
		}
		return op
	}

	// test BIND transition sequences
	tl.Logger().Info("BIND FORWARD STATE TRANSITIONS")
	tl.Flush()
	vsr.VolumeSeriesRequestState = "BINDING"

	tl.Logger().Info("Case: success no retry")
	tl.Flush()
	op := newFakeBindOp()
	assert.False(op.isCCOp)
	assert.False(op.isUndoOp)
	op.retGIS = BindLoadObjects
	op.retSpaSearchObj = spaClone(spaObj)
	op.retCheckForCapacity = true
	expCalled := []string{"GIS", "LO", "WFL", "SEARCH", "CRP", "CFC", "PLA", "VSC", "UR", "SRsvC", "VS", "CIU"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: success on retry (spaId set)")
	tl.Flush()
	op = newFakeBindOp()
	assert.False(op.isCCOp)
	assert.False(op.isUndoOp)
	op.retGIS = BindLoadObjects
	op.rhs.Request.ServicePlanAllocationID = "SPA-1"
	op.retSpaFetchObj = spaClone(spaObj)
	op.retCanClaimOwnership = true
	op.retCheckForCapacity = true
	expCalled = []string{"GIS", "LO", "WFL", "SF", "CCO", "CRP", "CFC", "PLA", "VSC", "UR", "SRsvC", "VS", "CIU"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: spa not found first time")
	tl.Flush()
	op = newFakeBindOp()
	assert.False(op.isCCOp)
	assert.False(op.isUndoOp)
	op.retGIS = BindLoadObjects
	expCalled = []string{"GIS", "LO", "WFL", "SEARCH", "SCW"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: spa not found on retry (spaId set)")
	tl.Flush()
	op = newFakeBindOp()
	assert.False(op.isCCOp)
	assert.False(op.isUndoOp)
	op.retGIS = BindLoadObjects
	op.rhs.Request.ServicePlanAllocationID = "SPA-1"
	expCalled = []string{"GIS", "LO", "WFL", "SF", "SCW"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: spa fetch error on retry (spaId set)")
	tl.Flush()
	op = newFakeBindOp()
	assert.False(op.isCCOp)
	assert.False(op.isUndoOp)
	op.retGIS = BindLoadObjects
	op.rhs.Request.ServicePlanAllocationID = "SPA-1"
	op.retSpaFetchRetryLater = true
	expCalled = []string{"GIS", "LO", "WFL", "SF"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: spa has no capacity")
	tl.Flush()
	op = newFakeBindOp()
	assert.False(op.isCCOp)
	assert.False(op.isUndoOp)
	op.retGIS = BindLoadObjects
	op.retSpaSearchObj = spaClone(spaObj)
	expCalled = []string{"GIS", "LO", "WFL", "SEARCH", "CRP", "CFC", "SCW"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: storage formula lookup error")
	tl.Flush()
	op = newFakeBindOp()
	assert.False(op.isCCOp)
	assert.False(op.isUndoOp)
	op.retGIS = BindLoadObjects
	op.retSpaSearchObj = spaClone(spaObj)
	op.retComputeResPlanErr = true
	expCalled = []string{"GIS", "LO", "WFL", "SEARCH", "CRP"}
	op.run(nil)
	assert.Equal(expCalled, op.called)
	op.retComputeResPlanErr = false

	tl.Logger().Info("Case: spa loses capacity during reservation")
	tl.Flush()
	op = newFakeBindOp()
	assert.False(op.isCCOp)
	assert.False(op.isUndoOp)
	op.retGIS = BindLoadObjects
	op.retSpaSearchObj = spaClone(spaObj)
	op.retCheckForCapacity = true
	op.retSpaReserveNoCap = true
	op.spaHasCapacity = true
	expCalled = []string{"GIS", "LO", "WFL", "SEARCH", "CRP", "CFC", "PLA", "VSC", "UR", "SRsvC", "SCW"}
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.False(op.spaHasCapacity)

	// test UNDO_BIND transition sequences
	tl.Logger().Info("BIND UNDO STATE TRANSITIONS")
	tl.Flush()
	vsr.VolumeSeriesRequestState = "UNDO_BINDING"
	vs.VolumeSeriesState = "BOUND"

	tl.Logger().Info("Case: undo all")
	tl.Flush()
	op = newFakeBindOp()
	assert.False(op.isCCOp)
	assert.True(op.isUndoOp)
	op.rhs.Request.ServicePlanAllocationID = "SPA-1"
	op.retGIS = BindUndoWaitForReservationLock
	op.retSpaFetchObj = spaClone(spaObj)
	op.retCanClaimOwnership = true
	expCalled = []string{"GIS", "WFL", "SF", "CCO", "SIU", "SVsP", "VU", "CRP", "SRetC"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: undo done")
	tl.Flush()
	op = newFakeBindOp()
	assert.False(op.isCCOp)
	assert.True(op.isUndoOp)
	op.rhs.Request.ServicePlanAllocationID = "SPA-1"
	op.retGIS = BindUndoDone
	expCalled = []string{"GIS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// test CHANGE_CAPACITY transition sequences
	tl.Logger().Info("CHANGE_CAPACITY FORWARD STATE TRANSITIONS")
	tl.Flush()
	vsr.VolumeSeriesRequestState = "CHANGING_CAPACITY"

	tl.Logger().Info("Case: CHANGE_CAPACITY success no retry, volume IsBound")
	tl.Flush()
	op = newFakeBindOp()
	assert.True(op.isCCOp)
	assert.False(op.isUndoOp)
	assert.NotEmpty(op.rhs.Request.ServicePlanAllocationID)
	op.retGIS = BindWaitForReservationLock
	op.retSpaFetchObj = spaClone(spaObj)
	op.retCanClaimOwnership = true
	op.retCheckForCapacity = true
	expCalled = []string{"GIS", "WFL", "SF", "CCO", "CRP", "CFC", "PLA", "VSC", "UR", "SRsvC", "VS", "CIU"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: CHANGE_CAPACITY success no retry, volume Unbound")
	tl.Flush()
	op = newFakeBindOp()
	assert.True(op.isCCOp)
	assert.False(op.isUndoOp)
	op.rhs.Request.ServicePlanAllocationID = ""
	op.retGIS = BindWaitForReservationLock
	op.retSpaFetchObj = spaClone(spaObj)
	op.retCanClaimOwnership = true
	op.retCheckForCapacity = true
	expCalled = []string{"GIS", "WFL", "VS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// test CHANGE_CAPACITY transition sequences
	tl.Logger().Info("CHANGE_CAPACITY UNDO STATE TRANSITIONS")
	tl.Flush()
	vsr.VolumeSeriesRequestState = "UNDO_CHANGING_CAPACITY"

	tl.Logger().Info("Case: CHANGE_CAPACITY undo, volume IsBound")
	tl.Flush()
	op = newFakeBindOp()
	assert.True(op.isCCOp)
	assert.True(op.isUndoOp)
	assert.NotEmpty(op.rhs.Request.ServicePlanAllocationID)
	op.retGIS = BindUndoWaitForReservationLock
	op.retSpaFetchObj = spaClone(spaObj)
	op.retCanClaimOwnership = true
	op.retCheckForCapacity = true
	expCalled = []string{"GIS", "WFL", "SF", "CCO", "SIU", "SVsP", "VU", "CRP", "SRetC"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	tl.Logger().Info("Case: CHANGE_CAPACITY undo, volume Unbound")
	tl.Flush()
	op = newFakeBindOp()
	assert.True(op.isCCOp)
	assert.True(op.isUndoOp)
	op.rhs.Request.ServicePlanAllocationID = ""
	op.retGIS = BindUndoDone
	op.retSpaFetchObj = spaClone(spaObj)
	op.retCanClaimOwnership = true
	op.retCheckForCapacity = true
	expCalled = []string{"GIS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// Run real handlers
	tl.Logger().Info("CALL REAL BIND")
	tl.Flush()
	fc.RetLClObj = cl
	fc.RetSPAListErr = fmt.Errorf("spa-list-error")
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "BINDING"
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = com.VolStateUnbound
	rhs := &vra.RequestHandlerState{
		A:            c.Animator,
		Request:      vsr,
		VolumeSeries: vs,
	}
	assert.Empty(c.reservationCS.Status())
	assert.Equal(0, c.reservationCS.Used)
	c.Bind(ctx, rhs)
	assert.Empty(c.reservationCS.Status())
	assert.Equal(1, c.reservationCS.Used)

	tl.Logger().Info("CALL REAL UNDO_BIND")
	tl.Flush()
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = com.VolReqStateUndoBinding
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = com.VolStateInUse
	rhs = &vra.RequestHandlerState{
		A:            c.Animator,
		Request:      vsr,
		VolumeSeries: vs,
	}
	rhs.InError = true
	c.UndoBind(ctx, rhs)
	assert.True(rhs.InError)

	tl.Logger().Info("CALL REAL CHANGE_CAPACITY")
	tl.Flush()
	fc.RetSvPFetchErr = fmt.Errorf("spa-fetch-error")
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "CHANGING_CAPACITY"
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = com.VolStateBound
	rhs = &vra.RequestHandlerState{
		A:            c.Animator,
		Request:      vsr,
		VolumeSeries: vs,
	}
	assert.Empty(c.reservationCS.Status())
	assert.Equal(1, c.reservationCS.Used)
	c.ChangeCapacity(ctx, rhs)
	assert.Empty(c.reservationCS.Status())
	assert.Equal(2, c.reservationCS.Used)

	tl.Logger().Info("CALL REAL UNDO_CHANGE_CAPACITY")
	tl.Flush()
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = com.VolReqStateUndoChangingCapacity
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = com.VolStateUnbound
	rhs = &vra.RequestHandlerState{
		A:            c.Animator,
		Request:      vsr,
		VolumeSeries: vs,
	}
	rhs.InError = true
	c.UndoChangeCapacity(ctx, rhs)
	assert.True(rhs.InError)

	// check bind state strings exist up to BindError
	var bss bindSubState
	for bss = BindLoadObjects; bss < BindNoOp; bss++ {
		s := bss.String()
		tl.Logger().Debugf("Testing %d %s", bss, s)
		assert.Regexp("^Bind", s)
	}
	assert.Regexp("^bindSubState", bss.String())
}

type fakeBindOps struct {
	bindOp
	tl                    *testutils.TestLogger
	called                []string
	retGIS                bindSubState
	retSpaFetchObj        *models.ServicePlanAllocation
	retSpaFetchRetryLater bool
	retSpaReserveNoCap    bool
	retSpaSearchObj       *models.ServicePlanAllocation
	retComputeResPlanErr  bool
	retCheckForCapacity   bool
	retCanClaimOwnership  bool
}

func (op *fakeBindOps) getInitialState(ctx context.Context) bindSubState {
	op.called = append(op.called, "GIS")
	op.tl.Logger().Infof("*** GIS ⇒ %s\n", op.retGIS)
	return op.retGIS
}
func (op *fakeBindOps) loadObjects(ctx context.Context) {
	op.called = append(op.called, "LO")
}
func (op *fakeBindOps) poolLoadAll(ctx context.Context) {
	op.called = append(op.called, "PLA")
}
func (op *fakeBindOps) computeReservationPlan(ctx context.Context) {
	op.called = append(op.called, "CRP")
	if op.retComputeResPlanErr {
		op.rhs.InError = true
	}
}
func (op *fakeBindOps) spaCheckForCapacity(ctx context.Context) {
	op.called = append(op.called, "CFC")
	op.spaHasCapacity = op.retCheckForCapacity
}
func (op *fakeBindOps) spaCanClaimOwnership(ctx context.Context) {
	op.called = append(op.called, "CCO")
	if !op.retCanClaimOwnership {
		op.rhs.RetryLater = true
	}
}
func (op *fakeBindOps) spaClearInUse(ctx context.Context) {
	op.called = append(op.called, "CIU")
}
func (op *fakeBindOps) spaFetch(ctx context.Context) {
	op.called = append(op.called, "SF")
	op.spa = op.retSpaFetchObj
	op.rhs.RetryLater = op.retSpaFetchRetryLater
}
func (op *fakeBindOps) spaReserveCapacity(ctx context.Context) {
	op.called = append(op.called, "SRsvC")
	if op.retSpaReserveNoCap {
		op.spaHasCapacity = false
		op.rhs.RetryLater = false
	}
}
func (op *fakeBindOps) spaReturnCapacity(ctx context.Context) {
	op.called = append(op.called, "SRetC")
}
func (op *fakeBindOps) spaSearch(ctx context.Context) {
	op.called = append(op.called, "SEARCH")
	op.spa = op.retSpaSearchObj
}
func (op *fakeBindOps) spaSetInUse(ctx context.Context) {
	op.called = append(op.called, "SIU")
}
func (op *fakeBindOps) vsrSetCapacityReservationResult(ctx context.Context) {
	op.called = append(op.called, "VSC")
}
func (op *fakeBindOps) vsSet(ctx context.Context) {
	op.called = append(op.called, "VS")
}
func (op *fakeBindOps) vsUndo(ctx context.Context) {
	op.called = append(op.called, "VU")
}
func (op *fakeBindOps) vsrSaveVSProps(ctx context.Context) {
	op.called = append(op.called, "SVsP")
}
func (op *fakeBindOps) vsrSetCapacityWait(ctx context.Context) {
	op.called = append(op.called, "SCW")
}
func (op *fakeBindOps) vsrUpdate(ctx context.Context) {
	op.called = append(op.called, "UR")
}
func (op *fakeBindOps) waitForLock(ctx context.Context) {
	op.called = append(op.called, "WFL")
}
