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
	"errors"
	"fmt"
	"net/http"
	"testing"

	client_pool "github.com/Nuvoloso/kontroller/pkg/autogen/client/pool"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan_allocation"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	fhk "github.com/Nuvoloso/kontroller/pkg/housekeeping/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/alecthomas/units"
	"github.com/go-openapi/swag"
	"github.com/stretchr/testify/assert"
)

func TestAllocateCapacitySteps(t *testing.T) {
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
				Name:               "MyCluster",
				AuthorizedAccounts: []models.ObjIDMutable{"SystemID"},
			},
		},
	}
	domain := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta:          &models.ObjMeta{ID: "CSP-DOMAIN-1"},
			CspDomainType: "AWS",
		},
		CSPDomainMutable: models.CSPDomainMutable{
			AuthorizedAccounts: []models.ObjIDMutable{"SystemID"},
		},
	}
	vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VSR-1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			RequestedOperations: []string{"ALLOCATE_CAPACITY"},
			ServicePlanAllocationCreateSpec: &models.ServicePlanAllocationCreateArgs{
				ServicePlanAllocationCreateOnce: models.ServicePlanAllocationCreateOnce{
					AccountID:           "OWNER-ACCOUNT",
					AuthorizedAccountID: "AUTHORIZED-ACCOUNT",
					ClusterID:           "CLUSTER-1",
					ServicePlanID:       "SERVICE-PLAN-1",
				},
			},
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "ALLOCATING_CAPACITY",
			},
		},
	}
	sfs := []*models.StorageFormula{
		{
			Description: "8ms, Random, Read/Write",
			Name:        "8ms-rand-rw",
			IoProfile: &models.IoProfile{
				IoPattern:    &models.IoPattern{Name: "random", MinSizeBytesAvg: swag.Int32(0), MaxSizeBytesAvg: swag.Int32(16384)},
				ReadWriteMix: &models.ReadWriteMix{Name: "read-write", MinReadPercent: swag.Int32(30), MaxReadPercent: swag.Int32(70)},
			},
			SscList: models.SscListMutable{
				"Response Time Average": {Kind: "DURATION", Value: "8ms"},
				"Response Time Maximum": {Kind: "DURATION", Value: "50ms"},
				"Availability":          {Kind: "PERCENTAGE", Value: "99.999%"},
			},
			StorageComponent: map[string]models.StorageFormulaTypeElement{
				"Amazon gp2": {Percentage: swag.Int32(100)},
			},
			CacheComponent: map[string]models.StorageFormulaTypeElement{
				"Amazon SSD": {Percentage: swag.Int32(20)},
			},
			StorageLayout: "mirrored",
		},
	}
	crpObj := &models.CapacityReservationPlan{
		StorageTypeReservations: map[string]models.StorageTypeReservation{
			"Amazon gp2": models.StorageTypeReservation{
				NumMirrors: 1,
				SizeBytes:  swag.Int64(int64(1 * units.GiB)),
			},
		},
	}
	crp := crpClone(crpObj)
	spaObj := &models.ServicePlanAllocation{
		ServicePlanAllocationAllOf0: models.ServicePlanAllocationAllOf0{
			Meta: &models.ObjMeta{ID: "SPA-1"},
		},
		ServicePlanAllocationCreateOnce: models.ServicePlanAllocationCreateOnce{
			AccountID:           "OWNER-ACCOUNT",
			AuthorizedAccountID: "AUTHORIZED-ACCOUNT",
			ClusterID:           "CLUSTER-1",
			ServicePlanID:       "SERVICE-PLAN-1",
		},
	}
	pools := []*models.Pool{
		&models.Pool{ // empty
			PoolAllOf0: models.PoolAllOf0{
				Meta: &models.ObjMeta{ID: "POOL-AA-CL-Amazon GP2-1"},
			},
			PoolCreateOnce: models.PoolCreateOnce{
				CspStorageType: "Amazon gp2",
			},
		},
		&models.Pool{ // not empty
			PoolAllOf0: models.PoolAllOf0{
				Meta: &models.ObjMeta{ID: "POOL-AA-CL-Amazon io-1"},
			},
			PoolCreateOnce: models.PoolCreateOnce{
				CspStorageType: "Amazon io1",
			},
		},
		&models.Pool{
			PoolAllOf0: models.PoolAllOf0{
				Meta: &models.ObjMeta{ID: "POOL-AA-CL-Amazon GP2-2"}, // 2nd of same type
			},
			PoolCreateOnce: models.PoolCreateOnce{
				CspStorageType: "Amazon gp2",
			},
		},
	}
	stObj := app.GetCspStorageType("Amazon gp2")
	assert.NotNil(stObj)
	sTagCreator := fmt.Sprintf("%s:%s", com.SystemTagVsrCreator, vsrObj.Meta.ID)
	sTagDeleting := fmt.Sprintf("%s:%s", com.SystemTagVsrDeleting, vsrObj.Meta.ID)
	sTagOp := fmt.Sprintf("%s:%s", com.SystemTagVsrOperator, vsrObj.Meta.ID)
	vsrCreateSpaSysTag := fmt.Sprintf("%s:%s", com.SystemTagVsrCreateSPA, vsrObj.Meta.ID)
	spObj := &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			Meta: &models.ObjMeta{
				ID: "SERVICE-PLAN-1",
			},
			IoProfile: &models.IoProfile{
				IoPattern:    &models.IoPattern{Name: "sequential", MinSizeBytesAvg: swag.Int32(16384), MaxSizeBytesAvg: swag.Int32(262144)},
				ReadWriteMix: &models.ReadWriteMix{Name: "read-write", MinReadPercent: swag.Int32(30), MaxReadPercent: swag.Int32(70)},
			},
			ProvisioningUnit: &models.ProvisioningUnit{
				IOPS:       swag.Int64(22),
				Throughput: swag.Int64(0),
			},
			VolumeSeriesMinMaxSize: &models.VolumeSeriesMinMaxSize{
				MinSizeBytes: swag.Int64(1 * int64(units.GiB)),
				MaxSizeBytes: swag.Int64(64 * int64(units.TiB)),
			},
			State:               "PUBLISHED",
			SourceServicePlanID: "SERVICE-PLAN-0",
		},
		ServicePlanMutable: models.ServicePlanMutable{
			Name:        "Service Plan Name",
			Description: "Service Plan Description",
			Accounts:    []models.ObjIDMutable{"SystemID"},
			Slos: models.SloListMutable{
				"Availability": models.RestrictedValueType{
					ValueType: models.ValueType{
						Kind:  "STRING",
						Value: "99.999%",
					},
					RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{
						Immutable: true,
					},
				},
			},
			Tags: []string{"tag1", "tag2"},
		},
	}

	var op *acOp
	var vsr *models.VolumeSeriesRequest

	//  ***************************** sTagCreator, sTagDeleting and sTagOp
	vsr = vsrClone(vsrObj)
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	assert.Equal(sTagCreator, op.sTagCreator())
	assert.Equal(sTagOp, op.sTagOp())

	//  ***************************** getInitialState

	// UNDO_ALLOCATING_CAPACITY cases
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "UNDO_ALLOCATING_CAPACITY"

	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.rhs.TimedOut = true
	op.rhs.InError = true
	assert.Equal(ACUndoFindSPA, op.getInitialState(ctx))
	assert.True(op.inError)
	assert.False(op.rhs.InError) // cleared
	assert.True(op.rhs.TimedOut) // unmodified
	assert.False(op.planOnly)

	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.rhs.Request.PlanOnly = swag.Bool(true)
	op.rhs.TimedOut = true
	op.rhs.InError = true
	assert.Equal(ACUndoDone, op.getInitialState(ctx))
	assert.True(op.rhs.InError)  // unmodified
	assert.True(op.rhs.TimedOut) // unmodified
	assert.True(op.planOnly)

	// DELETE_SPA cases
	vsr = vsrClone(vsrObj)
	vsr.RequestedOperations[0] = "DELETE_SPA"
	vsr.VolumeSeriesRequestState = "DELETING_SPA"
	vsr.ServicePlanAllocationCreateSpec.TotalCapacityBytes = swag.Int64(0)

	// TimedOut set on entry, planOnly picked up
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.rhs.Request.PlanOnly = swag.Bool(true)
	op.rhs.TimedOut = true
	assert.Equal(ACError, op.getInitialState(ctx))
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Invalid invocation of DELETE_SPA", op.rhs.Request.RequestMessages[0].Message)
	assert.True(op.planOnly)

	// InError set on entry
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.rhs.InError = true
	assert.Equal(ACError, op.getInitialState(ctx))
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Invalid invocation of DELETE_SPA", op.rhs.Request.RequestMessages[0].Message)
	assert.False(op.planOnly)

	// success, planning not done, planOnly picked up
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.rhs.Request.PlanOnly = swag.Bool(true)
	assert.Equal(ACLoadObjects, op.getInitialState(ctx))
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.False(op.planningDone)
	assert.True(op.planOnly)
	tl.Flush()

	// ALLOCATING_CAPACITY cases
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "ALLOCATING_CAPACITY"

	// TimedOut set on entry, planOnly picked up
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.rhs.Request.PlanOnly = swag.Bool(true)
	op.rhs.TimedOut = true
	assert.Equal(ACError, op.getInitialState(ctx))
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Invalid invocation of ALLOCATE_CAPACITY", op.rhs.Request.RequestMessages[0].Message)
	assert.True(op.planOnly)

	// InError set on entry
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.rhs.InError = true
	assert.Equal(ACError, op.getInitialState(ctx))
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Invalid invocation of ALLOCATE_CAPACITY", op.rhs.Request.RequestMessages[0].Message)
	assert.False(op.planOnly)

	// success, planning not done, planOnly picked up
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.rhs.Request.PlanOnly = swag.Bool(true)
	assert.Equal(ACLoadObjects, op.getInitialState(ctx))
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.False(op.planningDone)
	assert.True(op.planOnly)
	tl.Flush()

	// success, planning done
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.rhs.Request.CapacityReservationPlan = crp
	op.rhs.Request.ServicePlanAllocationCreateSpec.StorageFormula = sfs[0].Name
	assert.Equal(ACLoadObjects, op.getInitialState(ctx))
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.True(op.planningDone)
	assert.False(op.planOnly)
	tl.Flush()

	//  ***************************** loadObjects
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "ALLOCATING_CAPACITY"

	fc.RetLClErr = fmt.Errorf("cluster fetch error")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.loadObjects(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Equal("CLUSTER-1", fc.InLClID)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("cluster fetch error", op.rhs.Request.RequestMessages[0].Message)

	fc.RetLClErr = nil
	fc.RetLClObj = cl
	fc.InLClID = ""
	fc.RetLDErr = fmt.Errorf("domain fetch error")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.loadObjects(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Equal("CLUSTER-1", fc.InLClID)
	assert.Equal("CSP-DOMAIN-1", fc.InLDid)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("domain fetch error", op.rhs.Request.RequestMessages[0].Message)
	assert.Nil(op.cluster)
	assert.Nil(op.domain)

	fc.RetLDErr = nil
	fc.RetLDObj = domain
	fc.RetSvPFetchErr = fmt.Errorf("service plan fetch error")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.loadObjects(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Equal("CLUSTER-1", fc.InLClID)
	assert.Equal("CSP-DOMAIN-1", fc.InLDid)
	assert.Equal("SERVICE-PLAN-1", fc.InSvPFetchID)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("service plan fetch error", op.rhs.Request.RequestMessages[0].Message)
	assert.Nil(op.sp)

	fc.RetSvPFetchErr = nil
	fc.RetSvPFetchObj = spObj
	fc.InLDid = ""
	fc.InLClID = ""
	fc.InSvPFetchID = ""
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.loadObjects(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.Equal("CLUSTER-1", fc.InLClID)
	assert.Equal("CSP-DOMAIN-1", fc.InLDid)
	assert.Equal("SERVICE-PLAN-1", fc.InSvPFetchID)
	assert.Equal(cl, op.cluster)
	assert.Equal(domain, op.domain)
	assert.Equal(spObj, op.sp)

	tl.Flush()

	//  ***************************** updateRequest, clearSpaIDInRequest
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "ALLOCATING_CAPACITY"

	fc.RetVSRUpdaterErr = fmt.Errorf("vsr update error")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.rhs.Request.ServicePlanAllocationID = "SPA-1"
	op.clearSpaIDInRequest(ctx) // calls updateRequest
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Update error.*vsr update error", op.rhs.Request.RequestMessages[0].Message)
	assert.Empty(op.rhs.Request.ServicePlanAllocationID)

	//  ***************************** waitForLock
	c.reservationCS.Drain() // force closure
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.waitForLock(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("reservation lock error", op.rhs.Request.RequestMessages[0].Message)
	assert.Nil(op.reservationCST)

	c.reservationCS = util.NewCriticalSectionGuard()
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.waitForLock(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(op.reservationCST)
	assert.Equal([]string{"VSR-1"}, c.reservationCS.Status())
	tl.Flush()

	//  ***************************** selectStorageFormula
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "ALLOCATING_CAPACITY"

	sfc.RetSFfSPErr = fmt.Errorf("sff sp error")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.selectStorageFormula(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("storage formula error.*sff sp error", op.rhs.Request.RequestMessages[0].Message)

	sfc.RetSFfSPErr = nil
	sfc.RetSFfSPObj = []*models.StorageFormula{}
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.selectStorageFormula(ctx)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("no applicable storage formula found", op.rhs.Request.RequestMessages[0].Message)

	sfc.RetSFfSPObj = sfs
	sfc.RetSFfDErr = fmt.Errorf("SFfD error")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.selectStorageFormula(ctx)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("unable to select storage formula.*SFfD error", op.rhs.Request.RequestMessages[0].Message)

	sfc.RetSFfDErr = nil
	sfc.RetSFfDf = sfs[0]
	sfc.RetCRPObj = crp
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	assert.Nil(op.rhs.Request.CapacityReservationPlan)
	assert.Empty(op.rhs.Request.StorageFormula)
	op.selectStorageFormula(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(op.rhs.Request.CapacityReservationPlan)
	assert.Equal(crp, op.rhs.Request.CapacityReservationPlan)
	assert.Equal(sfs[0].Name, op.rhs.Request.StorageFormula)

	//  ***************************** findSPA
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "ALLOCATING_CAPACITY"

	// list error
	fc.RetSPAListErr = fmt.Errorf("spa-list-error")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.findSPA(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("ServicePlanAllocation list error.*spa-list-error", op.rhs.Request.RequestMessages[0].Message)

	// no error, not found
	fc.RetSPAListErr = nil
	fc.RetSPAListOK = &service_plan_allocation.ServicePlanAllocationListOK{}
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.findSPA(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Nil(op.spa)

	// found
	fc.RetSPAListErr = nil
	fc.RetSPAListOK = &service_plan_allocation.ServicePlanAllocationListOK{
		Payload: []*models.ServicePlanAllocation{spaObj},
	}
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.findSPA(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(spaObj, op.spa)
	assert.EqualValues(op.rhs.Request.ServicePlanAllocationID, op.spa.Meta.ID)

	//  ***************************** validateVSRAgainstSPA / includes vsrIsActive
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "ALLOCATING_CAPACITY"

	// success, already claimed by this vsr
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.spa = spaClone(spaObj)
	op.spa.SystemTags = []string{"foo:bar", sTagOp}
	op.validateVSRAgainstSPA(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	// in use by active vsr
	fc.RetVRObj = &models.VolumeSeriesRequest{}
	fc.RetVRObj.VolumeSeriesRequestState = "NOT-TERMINATED"
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.spa = spaClone(spaObj)
	op.spa.SystemTags = []string{"foo:bar", sTagOp + "www"}
	expLockVsrID := string(vsr.Meta.ID) + "www"
	op.validateVSRAgainstSPA(ctx)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("being processed by request.*www", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal(expLockVsrID, fc.InVRFetchID)

	// failed to fetch lock vsr
	fc.RetVRObj = nil
	fc.RetVRErr = fmt.Errorf("some error")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.spa = spaClone(spaObj)
	op.spa.SystemTags = []string{"foo:bar", sTagOp + "xxx"}
	expLockVsrID = string(vsr.Meta.ID) + "xxx"
	op.validateVSRAgainstSPA(ctx)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("being processed by request.*xxx", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal(expLockVsrID, fc.InVRFetchID)

	// fail size check
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.spa = spaClone(spaObj)
	op.rhs.Request.ServicePlanAllocationCreateSpec.TotalCapacityBytes = swag.Int64(1000)
	op.spa.TotalCapacityBytes = swag.Int64(5000)
	op.spa.ReservableCapacityBytes = swag.Int64(999)
	op.validateVSRAgainstSPA(ctx)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Invalid totalCapacityBytes.*4001.*already reserved", op.rhs.Request.RequestMessages[0].Message)

	// success (lock not stale)
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.spa = spaClone(spaObj)
	op.validateVSRAgainstSPA(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	// success: stale lock (vsr terminated)
	fc.RetVRObj = &models.VolumeSeriesRequest{}
	fc.RetVRObj.VolumeSeriesRequestState = "SUCCEEDED"
	fc.RetVRErr = nil
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.spa = spaClone(spaObj)
	op.spa.SystemTags = []string{"foo:bar", sTagOp + "yyy"}
	expLockVsrID = string(vsr.Meta.ID) + "yyy"
	op.validateVSRAgainstSPA(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Ignoring stale lock.*yyy", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal(expLockVsrID, fc.InVRFetchID)

	// success: stale lock (vsr not found)
	fc.RetVRObj = nil
	fc.RetVRErr = crud.NewError(&models.Error{Code: http.StatusNotFound, Message: swag.String(com.ErrorNotFound)})
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.spa = spaClone(spaObj)
	op.spa.SystemTags = []string{"foo:bar", sTagOp + "zzz"}
	expLockVsrID = string(vsr.Meta.ID) + "zzz"
	op.validateVSRAgainstSPA(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Ignoring stale lock.*zzz", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal(expLockVsrID, fc.InVRFetchID)

	//  ***************************** markSPAInUse
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "ALLOCATING_CAPACITY"

	// already claimed by this vsr
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.spa = spaClone(spaObj)
	op.spa.SystemTags = []string{"foo:bar", sTagOp}
	op.markSPAInUse(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	// plan only
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.planOnly = true
	op.spa = spaClone(spaObj)
	op.markSPAInUse(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Plan:", op.rhs.Request.RequestMessages[0].Message)

	// update error
	fc.RetSPAUpdaterErr = fmt.Errorf("spa-update-error")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.spa = spaClone(spaObj)
	op.markSPAInUse(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("ServicePlanAllocation update error.*spa-update-error", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal(ctx, fc.InSPAUpdaterCtx)
	assert.EqualValues(op.spa.Meta.ID, fc.InSPAUpdaterID)
	assert.Equal([]string{"systemTags"}, fc.InSPAUpdaterItems.Set)
	assert.Nil(fc.InSPAUpdaterItems.Append)
	assert.Nil(fc.InSPAUpdaterItems.Remove)

	// update success, force fetch 2nd call to verify updater
	tl.Flush()
	oU := spaClone(spaObj)
	oU.Tags = []string{"user:tag"}             // background change
	oU.SystemTags = []string{sTagOp + "STALE"} // stale lock replaced
	fc.FetchSPAUpdaterObj = spaClone(oU)       // 2nd callback object
	oU.SystemTags = []string{sTagOp}           // final updated
	fc.RetSPAUpdaterErr = nil
	fc.ModSPAUpdaterObj = nil
	fc.ForceFetchSPAUpdater = true
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.spa = spaClone(spaObj) // current
	op.markSPAInUse(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(ctx, fc.InSPAUpdaterCtx)
	assert.EqualValues(op.spa.Meta.ID, fc.InSPAUpdaterID)
	assert.Equal([]string{"systemTags"}, fc.InSPAUpdaterItems.Set)
	assert.EqualValues(fc.ModSPAUpdaterObj2, op.spa)
	assert.EqualValues(oU, fc.ModSPAUpdaterObj2)

	// update success for DELETE_SPA
	tl.Flush()
	vsr = vsrClone(vsrObj)
	vsr.RequestedOperations = []string{"DELETE_SPA"}
	vsr.VolumeSeriesRequestState = "DELETING_SPA"
	fc.FetchSPAUpdaterObj = nil
	fc.ModSPAUpdaterObj = nil
	fc.ForceFetchSPAUpdater = false
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsr, HasDeleteSPA: true}}
	op.spa = spaClone(spaObj) // current
	op.markSPAInUse(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(ctx, fc.InSPAUpdaterCtx)
	assert.EqualValues(op.spa.Meta.ID, fc.InSPAUpdaterID)
	assert.Equal([]string{"systemTags"}, fc.InSPAUpdaterItems.Set)
	assert.EqualValues([]string{sTagOp, sTagDeleting}, fc.ModSPAUpdaterObj.SystemTags)
	assert.EqualValues(fc.ModSPAUpdaterObj, op.spa)

	//  ***************************** createSPA
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "ALLOCATING_CAPACITY"

	// error
	fc.RetSPACreateErr = fmt.Errorf("spa-create-error")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.createSPA(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("ServicePlanAllocation create error.*spa-create-error", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal(ctx, fc.InSPACreateCtx)

	// success, ensure correct fields set/overridden
	fc.RetSPACreateErr = nil
	fc.RetSPACreateObj = spaClone(spaObj)
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.rhs.Request.StorageFormula = "formula1"
	ca := op.rhs.Request.ServicePlanAllocationCreateSpec
	ca.Messages = []*models.TimestampedString{&models.TimestampedString{}}                                    // discarded
	ca.ReservationState = "OK"                                                                                // discarded
	ca.StorageFormula = "foobar"                                                                              // kept
	ca.StorageReservations = map[string]models.StorageTypeReservation{"foo": models.StorageTypeReservation{}} // discarded
	ca.SystemTags = []string{"foo:bar"}                                                                       // replaced
	ca.Tags = []string{"tag1:v1"}                                                                             // kept
	ca.TotalCapacityBytes = swag.Int64(1000)                                                                  // discarded
	op.createSPA(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(ctx, fc.InSPACreateCtx)
	assert.Equal(fc.RetSPACreateObj, op.spa)
	co := fc.InSPACreateObj
	assert.NotNil(co)
	assert.Nil(co.Messages)
	assert.Empty(co.ReservationState)
	assert.EqualValues("formula1", co.StorageFormula)
	assert.Nil(co.StorageReservations)
	assert.EqualValues([]string{sTagCreator, sTagOp}, co.SystemTags)
	assert.EqualValues([]string{vsrCreateSpaSysTag}, op.rhs.Request.SystemTags)
	assert.EqualValues([]string{"tag1:v1"}, co.Tags)
	assert.Equal(int64(0), swag.Int64Value(co.TotalCapacityBytes))
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Created ServicePlanAllocation", op.rhs.Request.RequestMessages[0].Message)
	assert.EqualValues(op.rhs.Request.ServicePlanAllocationID, op.spa.Meta.ID)

	// plan only
	fc.RetSPACreateObj = nil
	fc.InSPACreateObj = nil
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.planOnly = true
	op.rhs.Request.StorageFormula = "formula1"
	ca = op.rhs.Request.ServicePlanAllocationCreateSpec
	ca.Messages = []*models.TimestampedString{&models.TimestampedString{}}                                    // discarded
	ca.ReservationState = "OK"                                                                                // discarded
	ca.StorageFormula = "foobar"                                                                              // kept
	ca.StorageReservations = map[string]models.StorageTypeReservation{"foo": models.StorageTypeReservation{}} // discarded
	ca.SystemTags = []string{"foo:bar"}                                                                       // replaced
	ca.Tags = []string{"tag1:v1"}                                                                             // kept
	ca.TotalCapacityBytes = swag.Int64(1000)                                                                  // discarded
	op.createSPA(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Nil(fc.InSPACreateObj)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Plan:", op.rhs.Request.RequestMessages[0].Message)
	assert.NotNil(op.spa)

	//  ***************************** finalizeSPA
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "ALLOCATING_CAPACITY"
	crr := &models.CapacityReservationResult{
		CurrentReservations: map[string]models.PoolReservation{},
		DesiredReservations: map[string]models.PoolReservation{},
	}
	crr.DesiredReservations["POOL-AA-CL-Amazon GP2-2"] = models.PoolReservation{NumMirrors: 2, SizeBytes: swag.Int64(2000)}
	vsr.CapacityReservationResult = crr

	// update error
	fc.RetSPAUpdaterErr = fmt.Errorf("spa-update-error")
	fc.InSPAUpdaterItems = nil
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.spa = spaClone(spaObj)
	op.finalizeSPA(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("ServicePlanAllocation update error.*spa-update-error", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal(ctx, fc.InSPAUpdaterCtx)
	assert.EqualValues(op.spa.Meta.ID, fc.InSPAUpdaterID)
	assert.Equal([]string{"storageReservations", "totalCapacityBytes"}, fc.InSPAUpdaterItems.Set)
	assert.Equal([]string{"systemTags"}, fc.InSPAUpdaterItems.Remove)

	// inject error
	tl.Flush()
	c.rei.SetProperty("ac-fail-in-spa-finalize", &rei.Property{BoolValue: true})
	fc.RetSPAUpdaterErr = nil
	fc.InSPAUpdaterItems = nil
	fc.ForceFetchSPAUpdater = false
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.spa = spaClone(spaObj)
	op.finalizeSPA(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("ServicePlanAllocation update error.*ac-fail-in-spa-finalize", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal(ctx, fc.InSPAUpdaterCtx)
	assert.Empty(op.rhs.Request.ServicePlanAllocationID)
	assert.EqualValues(op.spa.Meta.ID, fc.InSPAUpdaterID)
	assert.Equal([]string{"storageReservations", "totalCapacityBytes"}, fc.InSPAUpdaterItems.Set)
	assert.Equal([]string{"systemTags"}, fc.InSPAUpdaterItems.Remove)
	tl.Flush()

	// success, force fetch 2nd call, not creator
	oU = spaClone(spaObj)
	oU.SystemTags = []string{sTagOp}
	oU.Tags = []string{"user:tag"}       // background change
	fc.FetchSPAUpdaterObj = spaClone(oU) // 2nd callback object
	fc.RetSPAUpdaterErr = nil
	fc.InSPAUpdaterItems = nil
	fc.ForceFetchSPAUpdater = true
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.rhs.Request.ServicePlanAllocationCreateSpec.TotalCapacityBytes = swag.Int64(1001)
	op.rhs.Request.ServicePlanAllocationID = ""
	oI := spaClone(spaObj)
	oI.ReservableCapacityBytes = swag.Int64(1000)
	oI.StorageReservations = map[string]models.StorageTypeReservation{}
	oI.TotalCapacityBytes = swag.Int64(1001)
	oI.SystemTags = []string{op.sTagOp()} // no creator tag
	op.spa = oI
	op.finalizeSPA(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(ctx, fc.InSPAUpdaterCtx)
	assert.EqualValues(op.spa.Meta.ID, fc.InSPAUpdaterID)
	assert.EqualValues(op.spa.Meta.ID, op.rhs.Request.ServicePlanAllocationID)
	assert.Equal([]string{"storageReservations", "totalCapacityBytes"}, fc.InSPAUpdaterItems.Set)
	assert.Equal([]string{"systemTags"}, fc.InSPAUpdaterItems.Remove)
	assert.Nil(fc.InSPAUpdaterItems.Append)
	assert.EqualValues([]string{sTagOp}, fc.ModSPAUpdaterObj.SystemTags)
	uo := fc.ModSPAUpdaterObj2
	assert.EqualValues(uo, op.spa)
	assert.EqualValues([]string{sTagOp}, uo.SystemTags)
	assert.EqualValues([]string{"user:tag"}, uo.Tags)
	assert.NotNil(uo.StorageReservations)
	assert.NotNil(uo.TotalCapacityBytes)
	assert.EqualValues(swag.Int64Value(oI.TotalCapacityBytes), swag.Int64Value(uo.TotalCapacityBytes))
	assert.Len(uo.StorageReservations, 1)
	for spID, spr := range crr.DesiredReservations {
		str, ok := uo.StorageReservations[spID]
		assert.True(ok)
		assert.Equal(spr.NumMirrors, str.NumMirrors)
		assert.Equal(swag.Int64Value(spr.SizeBytes), swag.Int64Value(str.SizeBytes))
	}

	//  ***************************** releaseSPA
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "ALLOCATING_CAPACITY"

	// update error
	fc.RetSPAUpdaterErr = fmt.Errorf("spa-update-error")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.spa = spaClone(spaObj)
	op.releaseSPA(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("ServicePlanAllocation update error.*spa-update-error", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal(ctx, fc.InSPAUpdaterCtx)
	assert.EqualValues(op.spa.Meta.ID, fc.InSPAUpdaterID)
	assert.Equal([]string{"systemTags"}, fc.InSPAUpdaterItems.Remove)

	// update success, force fetch 2nd call to verify updater
	tl.Flush()
	oU = spaClone(spaObj)
	oU.Tags = []string{"user:tag"}       // background change
	fc.FetchSPAUpdaterObj = spaClone(oU) // 2nd callback object
	oU.SystemTags = []string{sTagOp}     // final updated
	fc.RetSPAUpdaterErr = nil
	fc.ModSPAUpdaterObj = nil
	fc.ForceFetchSPAUpdater = true
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.spa = spaClone(spaObj) // current
	op.releaseSPA(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(ctx, fc.InSPAUpdaterCtx)
	assert.EqualValues(op.spa.Meta.ID, fc.InSPAUpdaterID)
	assert.Equal([]string{"systemTags"}, fc.InSPAUpdaterItems.Remove)
	assert.EqualValues(fc.ModSPAUpdaterObj2, op.spa)
	assert.EqualValues(oU, fc.ModSPAUpdaterObj2)

	//  ***************************** deleteSPA
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "ALLOCATING_CAPACITY"

	// delete error
	fc.RetSPADeleteErr = fmt.Errorf("spa-delete-error")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.spa = spaClone(spaObj)
	op.deleteSPA(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 2)
	assert.Regexp("Deleting ServicePlanAllocation", op.rhs.Request.RequestMessages[0].Message)
	assert.Regexp("ServicePlanAllocation delete error.*spa-delete-error", op.rhs.Request.RequestMessages[1].Message)
	assert.Equal(ctx, fc.InSPADeleteCtx)
	assert.EqualValues(op.spa.Meta.ID, fc.InSPADeleteID)
	assert.NotNil(op.spa)

	// success
	fc.InSPADeleteID = ""
	fc.RetSPADeleteErr = nil
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.spa = spaClone(spaObj)
	op.deleteSPA(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Nil(op.spa)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Deleting ServicePlanAllocation", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal(ctx, fc.InSPADeleteCtx)
	assert.EqualValues(spaObj.Meta.ID, fc.InSPADeleteID)

	//  ***************************** findPools
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "ALLOCATING_CAPACITY"
	vsr.CapacityReservationPlan = crpClone(crpObj)

	// list error
	tl.Flush()
	tl.Logger().Info("*** findPool case: list error")
	fc.RetPoolListErr = fmt.Errorf("pool-list-error")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}, spa: spaClone(spaObj)}
	assert.Nil(op.pools)
	op.findPools(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Pool list error.*pool-list-error", op.rhs.Request.RequestMessages[0].Message)
	assert.NotNil(op.pools)
	assert.Len(op.pools, len(vsr.CapacityReservationPlan.StorageTypeReservations))

	// no error, none found
	tl.Flush()
	tl.Logger().Info("*** findPool case: none found")
	fc.RetPoolListErr = nil
	fc.RetPoolListObj = &client_pool.PoolListOK{}
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}, spa: spaClone(spaObj)}
	assert.Nil(op.pools)
	op.findPools(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(op.pools)
	assert.Len(op.pools, len(vsr.CapacityReservationPlan.StorageTypeReservations))
	cs := op.rhs.Request.ServicePlanAllocationCreateSpec
	assert.EqualValues(cs.AuthorizedAccountID, swag.StringValue(fc.InPoolListObj.AuthorizedAccountID))
	assert.EqualValues(cs.ClusterID, swag.StringValue(fc.InPoolListObj.ClusterID))
	assert.Nil(fc.InPoolListObj.ServicePlanAllocationID)
	assert.Nil(fc.InPoolListObj.CspStorageType)
	assert.Len(op.rhs.Request.RequestMessages, 0)

	// no error, multiple found. Spa has no previous storageReservation
	tl.Flush()
	tl.Logger().Info("*** findPool case: spa has no previous storageReservation")
	fc.RetPoolListErr = nil
	fc.RetPoolListObj = &client_pool.PoolListOK{Payload: pools}
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}, spa: spaClone(spaObj)}
	assert.Empty(op.spa.StorageReservations)
	assert.Nil(op.pools)
	op.findPools(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(op.pools)
	assert.Len(op.pools, len(op.rhs.Request.CapacityReservationPlan.StorageTypeReservations))
	assert.Len(op.pools, 1)
	assert.EqualValues("POOL-AA-CL-Amazon GP2-1", op.pools["Amazon gp2"].Meta.ID)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("existing pool.*Amazon gp2.*GP2-1", op.rhs.Request.RequestMessages[0].Message)

	// no error, multiple found. Spa has previous storageReservation and new CRP overlaps
	tl.Flush()
	tl.Logger().Info("*** findPool case: spa has previous storageReservation + CRP overlap")
	fc.RetPoolListErr = nil
	fc.RetPoolListObj = &client_pool.PoolListOK{Payload: pools}
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}, spa: spaClone(spaObj)}
	op.spa.StorageReservations = make(map[string]models.StorageTypeReservation)
	op.spa.StorageReservations["POOL-AA-CL-Amazon GP2-2"] = models.StorageTypeReservation{}
	op.spa.StorageReservations["POOL-AA-CL-Amazon io-1"] = models.StorageTypeReservation{}
	assert.Nil(op.pools)
	op.findPools(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(op.pools)
	assert.Len(op.pools, len(op.rhs.Request.CapacityReservationPlan.StorageTypeReservations))
	assert.Len(op.pools, 1)
	assert.EqualValues("POOL-AA-CL-Amazon GP2-2", op.pools["Amazon gp2"].Meta.ID) // 2 not 1
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("previously used pool.*Amazon gp2.*GP2-2", op.rhs.Request.RequestMessages[0].Message)

	// no error, multiple found. Spa has previous storageReservation and new CRP does not overlap
	tl.Flush()
	tl.Logger().Info("*** findPool case: spa has previous storageReservation + no CRP overlap")
	fc.RetPoolListErr = nil
	fc.RetPoolListObj = &client_pool.PoolListOK{Payload: pools}
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}, spa: spaClone(spaObj)}
	op.spa.StorageReservations = make(map[string]models.StorageTypeReservation)
	op.spa.StorageReservations["Some-pool-id"] = models.StorageTypeReservation{}
	op.rhs.Request.CapacityReservationPlan.StorageTypeReservations["Amazon io1"] = models.StorageTypeReservation{}
	assert.Nil(op.pools)
	op.findPools(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(op.pools)
	assert.Len(op.pools, len(op.rhs.Request.CapacityReservationPlan.StorageTypeReservations))
	assert.Len(op.pools, 2)
	assert.Contains(op.pools, "Amazon gp2")
	assert.EqualValues("POOL-AA-CL-Amazon GP2-1", op.pools["Amazon gp2"].Meta.ID)
	assert.Contains(op.pools, "Amazon io1")
	assert.EqualValues("POOL-AA-CL-Amazon io-1", op.pools["Amazon io1"].Meta.ID)
	assert.Len(op.rhs.Request.RequestMessages, 2)
	assert.Regexp("existing pool", op.rhs.Request.RequestMessages[0].Message)
	assert.Regexp("existing pool", op.rhs.Request.RequestMessages[1].Message)

	// no error, multiple found. Spa has previous storageReservation but CRR has references
	tl.Flush()
	tl.Logger().Info("*** findPool case: spa has previous storageReservation + CRR present")
	fc.RetPoolListErr = nil
	fc.RetPoolListObj = &client_pool.PoolListOK{Payload: pools}
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}, spa: spaClone(spaObj)}
	op.spa.StorageReservations = make(map[string]models.StorageTypeReservation)
	op.spa.StorageReservations["POOL-AA-CL-Amazon io-1"] = models.StorageTypeReservation{}
	op.rhs.Request.CapacityReservationPlan.StorageTypeReservations["Amazon io1"] = models.StorageTypeReservation{}
	crr = &models.CapacityReservationResult{
		CurrentReservations: map[string]models.PoolReservation{},
		DesiredReservations: map[string]models.PoolReservation{},
	}
	crr.DesiredReservations["POOL-AA-CL-Amazon GP2-2"] = models.PoolReservation{}
	op.rhs.Request.CapacityReservationResult = crr
	assert.Nil(op.pools)
	op.findPools(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(op.pools)
	assert.Len(op.pools, 2)
	assert.Contains(op.pools, "Amazon gp2")
	assert.EqualValues("POOL-AA-CL-Amazon GP2-2", op.pools["Amazon gp2"].Meta.ID)
	assert.Contains(op.pools, "Amazon io1")
	assert.EqualValues("POOL-AA-CL-Amazon io-1", op.pools["Amazon io1"].Meta.ID)
	assert.Len(op.rhs.Request.RequestMessages, 2)
	assert.Regexp("previously used pool.*io-1", op.rhs.Request.RequestMessages[0].Message)
	assert.Regexp("planned pool.*Amazon gp2.*GP2-2", op.rhs.Request.RequestMessages[1].Message)

	//  ***************************** createPools
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "ALLOCATING_CAPACITY"

	// error
	tl.Flush()
	tl.Logger().Info("*** createPools case: error")
	fc.RetPoolCreateErr = fmt.Errorf("pool-create-error")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}, cluster: cl, domain: domain}
	op.pools = make(map[string]*models.Pool)
	op.pools["Amazon gp2"] = nil
	op.createPools(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Pool create error.*pool-create-error", op.rhs.Request.RequestMessages[0].Message)

	// plan only
	tl.Flush()
	tl.Logger().Info("*** createPools case: plan only")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}, cluster: cl, domain: domain}
	op.planOnly = true
	op.pools = make(map[string]*models.Pool)
	op.pools["Amazon gp2"] = nil
	op.pools["Amazon io1"] = nil
	op.createPools(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 2)
	assert.Regexp("Plan:", op.rhs.Request.RequestMessages[0].Message)
	assert.Regexp("Plan:", op.rhs.Request.RequestMessages[1].Message)
	assert.Nil(op.pools["Amazon gp2"])
	assert.Nil(op.pools["Amazon io1"])

	// success, ensure correct fields set
	tl.Flush()
	tl.Logger().Info("*** createPools case: error")
	fc.RetPoolCreateErr = nil
	fc.RetPoolCreateObj = pools[0]
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}, cluster: cl, domain: domain}
	op.pools = make(map[string]*models.Pool)
	op.pools["Amazon gp2"] = nil
	op.pools["Amazon io1"] = pools[1]
	op.createPools(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(pools[0], op.pools["Amazon gp2"])
	cs = op.rhs.Request.ServicePlanAllocationCreateSpec
	assert.Equal(ctx, fc.InPoolCreateCtx)
	assert.Equal(cs.AccountID, fc.InPoolCreateObj.AccountID)
	assert.Equal(cs.AuthorizedAccountID, fc.InPoolCreateObj.AuthorizedAccountID)
	assert.Equal(cs.ClusterID, fc.InPoolCreateObj.ClusterID)
	assert.Equal(cl.CspDomainID, fc.InPoolCreateObj.CspDomainID)
	assert.Equal(cs.AccountID, fc.InPoolCreateObj.AccountID)
	assert.Equal("Amazon gp2", fc.InPoolCreateObj.CspStorageType)
	sa := &models.StorageAccessibilityMutable{
		AccessibilityScope:      "CSPDOMAIN",
		AccessibilityScopeObjID: cl.CspDomainID,
	}
	assert.Equal(sa, fc.InPoolCreateObj.StorageAccessibility)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Created pool.*POOL-AA-CL-Amazon GP2-1.*Amazon gp2", op.rhs.Request.RequestMessages[0].Message)

	//  ***************************** removeEmptyPoolReferences/ deletePools
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "ALLOCATING_CAPACITY"

	tl.Logger().Info("*** removeEmptyPoolReferences case: update error")
	tl.Flush()
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}, cluster: cl, domain: domain}
	op.pools = make(map[string]*models.Pool)
	assert.Len(pools[0].ServicePlanReservations, 0) // zero size
	op.pools["Amazon gp2"] = pools[0]
	op.pools["Amazon io1"] = nil
	crr = &models.CapacityReservationResult{
		CurrentReservations: map[string]models.PoolReservation{},
		DesiredReservations: map[string]models.PoolReservation{},
	}
	poolID := string(pools[0].Meta.ID)
	crr.CurrentReservations[poolID] = models.PoolReservation{}
	crr.DesiredReservations[poolID] = models.PoolReservation{}
	op.rhs.Request.CapacityReservationResult = crr
	fc.RetVSRUpdaterErr = fmt.Errorf("update-error")
	fc.RetVSRUpdaterObj = nil
	op.removeEmptyPoolReferences(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Removed references to pool.*POOL-AA-CL-Amazon GP2-1", op.rhs.Request.RequestMessages[0].Message)
	assert.NotContains(crr.CurrentReservations, poolID)
	assert.NotContains(crr.DesiredReservations, poolID)

	tl.Logger().Info("*** removeEmptyPoolReferences case: no error")
	tl.Flush()
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}, cluster: cl, domain: domain}
	op.pools = make(map[string]*models.Pool)
	assert.Len(pools[0].ServicePlanReservations, 0) // zero size
	op.pools["Amazon gp2"] = pools[0]
	op.pools["Amazon io1"] = nil
	crr = &models.CapacityReservationResult{
		CurrentReservations: map[string]models.PoolReservation{},
		DesiredReservations: map[string]models.PoolReservation{},
	}
	poolID = string(pools[0].Meta.ID)
	crr.CurrentReservations[poolID] = models.PoolReservation{}
	crr.DesiredReservations[poolID] = models.PoolReservation{}
	op.rhs.Request.CapacityReservationResult = crr
	fc.RetVSRUpdaterErr = nil
	fc.RetVSRUpdaterObj = op.rhs.Request
	op.removeEmptyPoolReferences(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Removed references to pool.*POOL-AA-CL-Amazon GP2-1", op.rhs.Request.RequestMessages[0].Message)
	assert.NotContains(crr.CurrentReservations, poolID)
	assert.NotContains(crr.DesiredReservations, poolID)

	tl.Logger().Info("*** removeEmptyPoolReferences case: no empty pool")
	tl.Flush()
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}, cluster: cl, domain: domain}
	assert.Len(pools[0].ServicePlanReservations, 0) // zero size
	op.pools = make(map[string]*models.Pool)
	var aPool *models.Pool
	testutils.Clone(pools[0], &aPool)
	aPool.ServicePlanReservations = make(map[string]models.StorageTypeReservation)
	aPool.ServicePlanReservations["some-spa"] = models.StorageTypeReservation{} // no longer zero sized
	op.pools["Amazon gp2"] = aPool
	op.pools["Amazon io1"] = nil
	crr = &models.CapacityReservationResult{
		CurrentReservations: map[string]models.PoolReservation{},
		DesiredReservations: map[string]models.PoolReservation{},
	}
	poolID = string(pools[0].Meta.ID)
	crr.CurrentReservations[poolID] = models.PoolReservation{}
	crr.DesiredReservations[poolID] = models.PoolReservation{}
	op.rhs.Request.CapacityReservationResult = crr
	fc.RetVSRUpdaterErr = nil
	fc.RetVSRUpdaterObj = op.rhs.Request
	op.removeEmptyPoolReferences(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 0)
	assert.Contains(crr.CurrentReservations, poolID)
	assert.Contains(crr.DesiredReservations, poolID)

	tl.Logger().Info("*** deletePools case: error")
	tl.Flush()
	fc.RetPoolDeleteErr = fmt.Errorf("pool-delete-error")
	fc.InPoolDeleteObj = ""
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}, cluster: cl, domain: domain}
	assert.Len(pools[0].ServicePlanReservations, 0) // zero size
	op.pools = make(map[string]*models.Pool)
	op.pools["Amazon gp2"] = pools[0]
	op.pools["Amazon io1"] = nil
	op.rhs.Request.CapacityReservationResult = crr
	op.deletePools(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Equal(ctx, fc.InPoolDeleteCtx)
	assert.EqualValues(pools[0].Meta.ID, fc.InPoolDeleteObj)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Deleting pool.*POOL-AA-CL-Amazon GP2-1", op.rhs.Request.RequestMessages[0].Message)
	assert.Contains(op.pools, "Amazon gp2")

	tl.Flush()
	tl.Logger().Info("*** deletePools case: empty pools deleted")
	fc.RetPoolDeleteErr = nil
	fc.InPoolDeleteObj = ""
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}, cluster: cl, domain: domain}
	assert.Len(pools[0].ServicePlanReservations, 0) // zero size
	op.pools = make(map[string]*models.Pool)
	op.pools["Amazon gp2"] = pools[0]
	op.pools["Amazon io1"] = pools[1] // non-zero
	pools[1].ServicePlanReservations = map[string]models.StorageTypeReservation{
		"some-spa": {NumMirrors: 1, SizeBytes: swag.Int64(1024)},
	}
	op.deletePools(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(ctx, fc.InPoolDeleteCtx)
	assert.EqualValues(pools[0].Meta.ID, fc.InPoolDeleteObj)
	assert.Nil(op.pools["Amazon gp2"])
	assert.Equal(pools[1], op.pools["Amazon io1"])
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Deleting pool.*POOL-AA-CL-Amazon GP2-1", op.rhs.Request.RequestMessages[0].Message)

	//  ***************************** initializeCRR
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "ALLOCATING_CAPACITY"

	tl.Flush()
	tl.Logger().Info("*** initializeCRR")
	crp = &models.CapacityReservationPlan{
		StorageTypeReservations: map[string]models.StorageTypeReservation{
			"st1": models.StorageTypeReservation{
				NumMirrors: 1,
				SizeBytes:  swag.Int64(int64(1 * units.GiB)),
			},
			"st2": models.StorageTypeReservation{
				NumMirrors: 1,
				SizeBytes:  swag.Int64(int64(2 * units.GiB)),
			},
			"st3": models.StorageTypeReservation{
				NumMirrors: 1,
				SizeBytes:  swag.Int64(int64(3 * units.GiB)),
			},
		},
	}
	expCRR := &models.CapacityReservationResult{ // expected CRR
		CurrentReservations: map[string]models.PoolReservation{},
		DesiredReservations: map[string]models.PoolReservation{},
	}
	for cst, str := range crp.StorageTypeReservations {
		expCRR.DesiredReservations[cst+"ID"] = models.PoolReservation{
			SizeBytes:  str.SizeBytes,
			NumMirrors: str.NumMirrors,
		}
	}
	makePool := func(cst string, prevM int32, prevCap int64) *models.Pool {
		p := &models.Pool{}
		p.Meta = &models.ObjMeta{ID: models.ObjID(cst + "ID")}
		p.CspStorageType = cst
		if prevCap > 0 {
			p.ServicePlanReservations = make(map[string]models.StorageTypeReservation)
			p.ServicePlanReservations[string(spaObj.Meta.ID)] = models.StorageTypeReservation{
				SizeBytes:  swag.Int64(prevCap),
				NumMirrors: prevM,
			}
			expCRR.CurrentReservations[cst+"ID"] = models.PoolReservation{
				SizeBytes:  swag.Int64(prevCap),
				NumMirrors: prevM,
			}
		} else {
			expCRR.CurrentReservations[cst+"ID"] = models.PoolReservation{NumMirrors: 1}
		}
		return p
	}
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}, cluster: cl, domain: domain}
	op.spa = spaClone(spaObj)
	op.rhs.Request.CapacityReservationPlan = crp
	op.pools = make(map[string]*models.Pool)
	op.pools["st1"] = makePool("st1", 1, 1000)
	op.pools["st2"] = makePool("st2", 0, 0)
	op.pools["st3"] = makePool("st3", 1, 3000)
	op.initializeCRR(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	crr = op.rhs.Request.CapacityReservationResult
	assert.NotNil(crr)
	assert.Equal(expCRR, crr)
	assert.NoError(crr.Validate(nil))

	//  ***************************** setPoolCapacity
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "ALLOCATING_CAPACITY"

	crr = &models.CapacityReservationResult{
		CurrentReservations: map[string]models.PoolReservation{},
		DesiredReservations: map[string]models.PoolReservation{},
	}
	crr.CurrentReservations["POOL-AA-CL-Amazon io-1"] = models.PoolReservation{NumMirrors: 1, SizeBytes: swag.Int64(1000)}
	crr.DesiredReservations["POOL-AA-CL-Amazon GP2-2"] = models.PoolReservation{NumMirrors: 2, SizeBytes: swag.Int64(2000)}

	// add capacity
	tl.Flush()
	tl.Logger().Info("*** case: addCapacityToPools")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}, cluster: cl, domain: domain}
	op.spa = spaClone(spaObj)
	op.pools = make(map[string]*models.Pool)
	op.pools["Amazon gp2"] = pools[2]
	op.pools["Amazon io1"] = pools[1]
	op.rhs.Request.CapacityReservationResult = crr
	fops := &fakeACOps{}
	op.ops = fops
	op.addCapacityToPools(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(fops.scaArgs)
	expSCA := &SetCapacityArgs{
		ServicePlanAllocationID: string(op.spa.Meta.ID),
		Capacity:                &models.PoolReservation{NumMirrors: 2, SizeBytes: swag.Int64(2000)},
		Pool:                    pools[2],
	}
	assert.Equal(expSCA, fops.scaArgs)
	assert.Nil(op.pools["Amazon gp2"]) // nil fake return pool
	assert.NotNil(op.pools["Amazon io1"])
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Set capacity.*POOL-AA-CL-Amazon GP2-2.*2, 2KB", op.rhs.Request.RequestMessages[0].Message)

	// remove capacity (CRR present)
	tl.Flush()
	tl.Logger().Info("*** case: removeCapacityFromPools (CRR present)")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}, cluster: cl, domain: domain}
	op.spa = spaClone(spaObj)
	op.pools = make(map[string]*models.Pool)
	op.pools["Amazon gp2"] = pools[2]
	op.pools["Amazon io1"] = pools[1]
	op.rhs.Request.CapacityReservationResult = crr
	fops = &fakeACOps{}
	op.ops = fops
	op.removeCapacityFromPools(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.NotNil(fops.scaArgs)
	expSCA = &SetCapacityArgs{
		ServicePlanAllocationID: string(op.spa.Meta.ID),
		Capacity:                &models.PoolReservation{NumMirrors: 1, SizeBytes: swag.Int64(1000)},
		Pool:                    pools[1],
	}
	assert.Equal(expSCA, fops.scaArgs)
	assert.NotNil(op.pools["Amazon gp2"])
	assert.Nil(op.pools["Amazon io1"]) // nil fake return pool
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Set capacity.*POOL-AA-CL-Amazon io-1.*1, 1KB", op.rhs.Request.RequestMessages[0].Message)

	// remove capacity (CRR empty)
	tl.Flush()
	tl.Logger().Info("*** case: removeCapacityFromPools (CRR empty)")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}, cluster: cl, domain: domain}
	op.spa = spaClone(spaObj)
	op.pools = make(map[string]*models.Pool)
	op.pools["Amazon gp2"] = pools[2]
	op.pools["Amazon io1"] = pools[1]
	op.rhs.Request.CapacityReservationResult = &models.CapacityReservationResult{}
	fops = &fakeACOps{}
	op.ops = fops
	op.removeCapacityFromPools(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Nil(fops.scaArgs)
	assert.NotNil(op.pools["Amazon gp2"])
	assert.NotNil(op.pools["Amazon io1"])

	// remove capacity (CRR nil)
	tl.Flush()
	tl.Logger().Info("*** case: removeCapacityFromPools (CRR nil)")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}, cluster: cl, domain: domain}
	op.spa = spaClone(spaObj)
	op.pools = make(map[string]*models.Pool)
	op.pools["Amazon gp2"] = pools[2]
	op.pools["Amazon io1"] = pools[1]
	op.rhs.Request.CapacityReservationResult = nil
	fops = &fakeACOps{}
	op.ops = fops
	op.removeCapacityFromPools(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Nil(fops.scaArgs)
	assert.NotNil(op.pools["Amazon gp2"])
	assert.NotNil(op.pools["Amazon io1"])

	// error
	tl.Logger().Info("*** case: setPoolCapacity error")
	tl.Flush()
	fc.RetVSRUpdaterErr = fmt.Errorf("vsr update error")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}, cluster: cl, domain: domain}
	op.spa = spaClone(spaObj)
	op.pools = make(map[string]*models.Pool)
	op.pools["Amazon gp2"] = pools[2]
	op.pools["Amazon io1"] = pools[1]
	op.rhs.Request.CapacityReservationResult = crr
	fops = &fakeACOps{}
	op.ops = fops
	fops.retScaErr = fmt.Errorf("set-pool-capacity-for-spa-error")
	op.setPoolCapacity(ctx, crr.CurrentReservations)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.NotNil(fops.scaArgs)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Error updating pool.*set-pool-capacity-for-spa-error", op.rhs.Request.RequestMessages[0].Message)
	assert.NotNil(op.pools["Amazon gp2"])
	assert.NotNil(op.pools["Amazon io1"])

	// ***************************** removeStorageReservations
	vsr = vsrClone(vsrObj)
	vsr.RequestedOperations = []string{"DELETE_SPA"}
	vsr.VolumeSeriesRequestState = "DELETING_SPA"

	// already removed
	tl.Logger().Info("*** case: removeStorageReservations already removed")
	tl.Flush()
	fc.InSPAUpdaterItems = nil
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.spa = spaClone(spaObj) // current
	op.removeStorageReservations(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(ctx, fc.InSPAUpdaterCtx)
	assert.Nil(fc.InSPAUpdaterItems)

	// update error
	tl.Logger().Info("*** case: removeStorageReservations update error")
	tl.Flush()
	fc.InSPAUpdaterID = ""
	fc.RetSPAUpdaterErr = errors.New("update-err")
	fc.RetSPAUpdaterObj = nil
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.spa = spaClone(spaObj) // current
	op.spa.StorageReservations = make(map[string]models.StorageTypeReservation)
	op.spa.StorageReservations["POOL-AA-CL-Amazon io-1"] = models.StorageTypeReservation{}
	op.spa.TotalCapacityBytes = swag.Int64(1)
	op.removeStorageReservations(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Equal(ctx, fc.InSPAUpdaterCtx)
	assert.EqualValues(op.spa.Meta.ID, fc.InSPAUpdaterID)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("ServicePlanAllocation update error.*update-err", op.rhs.Request.RequestMessages[0].Message)

	// inject error
	tl.Logger().Info("*** case: removeStorageReservations rei error")
	tl.Flush()
	c.rei.SetProperty("ac-fail-in-spa-remove-res", &rei.Property{BoolValue: true})
	fc.RetSPAUpdaterErr = nil
	fc.InSPAUpdaterItems = nil
	fc.ForceFetchSPAUpdater = false
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.spa = spaClone(spaObj)
	op.spa.StorageReservations = make(map[string]models.StorageTypeReservation)
	op.spa.StorageReservations["POOL-AA-CL-Amazon io-1"] = models.StorageTypeReservation{}
	op.spa.TotalCapacityBytes = swag.Int64(1)
	op.removeStorageReservations(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("ServicePlanAllocation update error.*ac-fail-in-spa-remove-res", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal(ctx, fc.InSPAUpdaterCtx)
	assert.EqualValues(op.spa.Meta.ID, fc.InSPAUpdaterID)
	assert.Equal([]string{"storageReservations", "totalCapacityBytes"}, fc.InSPAUpdaterItems.Set)

	// update success, force fetch 2nd call to verify updater
	tl.Logger().Info("*** case: removeStorageReservations success")
	tl.Flush()
	oU = spaClone(spaObj)
	oU.Tags = []string{"user:tag"}       // background change
	fc.FetchSPAUpdaterObj = spaClone(oU) // 2nd callback object
	oU.StorageReservations = nil         // final update
	oU.TotalCapacityBytes = swag.Int64(0)
	fc.RetSPAUpdaterErr = nil
	fc.ModSPAUpdaterObj = nil
	fc.ForceFetchSPAUpdater = true
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.spa = spaClone(spaObj) // current
	op.spa.StorageReservations = make(map[string]models.StorageTypeReservation)
	op.spa.StorageReservations["POOL-AA-CL-Amazon io-1"] = models.StorageTypeReservation{}
	op.spa.TotalCapacityBytes = swag.Int64(1)
	op.removeStorageReservations(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(ctx, fc.InSPAUpdaterCtx)
	assert.EqualValues(op.spa.Meta.ID, fc.InSPAUpdaterID)
	assert.Equal([]string{"storageReservations", "totalCapacityBytes"}, fc.InSPAUpdaterItems.Set)
	assert.EqualValues(fc.ModSPAUpdaterObj2, op.spa)
	assert.EqualValues(oU, fc.ModSPAUpdaterObj2)

	// ***************************** updateServicePlan
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "ALLOCATING_CAPACITY"
	sp := spClone(spObj)
	authAccount := models.ObjIDMutable("AUTH-ACCOUNT-ID")

	// addAccountToServicePlan success
	tl.Flush()
	tl.Logger().Info("*** case: addAccountToServicePlan success")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.domain = dClone(domain)
	op.cluster = clClone(cl)
	op.sp = sp
	op.rhs.Request.ServicePlanAllocationCreateSpec.AuthorizedAccountID = authAccount
	fc.RetSvPUpdaterErr = nil
	fc.InSvPUpdaterItems = nil
	fc.ForceFetchSvPUpdater = false
	op.addAccountToSDC(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(ctx, fc.InSvPUpdaterCtx)
	assert.EqualValues(op.sp.Meta.ID, models.ObjIDMutable(fc.InSvPUpdaterID))
	assert.Equal([]string{"accounts"}, fc.InSvPUpdaterItems.Append)
	assert.Nil(fc.InSvPUpdaterItems.Set)
	assert.Nil(fc.InSvPUpdaterItems.Remove)
	uoSP := fc.ModSvPUpdaterObj
	assert.EqualValues(uoSP, op.sp)
	assert.Contains(uoSP.Accounts, authAccount)
	assert.Regexp(fmt.Sprintf("Account .%s. authorized for ServicePlan .%s", authAccount, sp.Meta.ID), op.rhs.Request.RequestMessages[0].Message)

	// addAccountToServicePlan already exists
	sp2 := spClone(spObj)
	tl.Flush()
	tl.Logger().Info("*** case: addAccountToServicePlan already exists")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.domain = dClone(domain)
	op.cluster = clClone(cl)
	op.sp = sp2
	op.sp.Accounts = append(op.sp.Accounts, authAccount)
	op.rhs.Request.ServicePlanAllocationCreateSpec.AuthorizedAccountID = authAccount
	fc.InSvPUpdaterItems = nil
	op.addAccountToSDC(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Nil(fc.InSvPUpdaterItems)
	assert.Regexp("Account .*already authorized for service plan", op.rhs.Request.RequestMessages[0].Message)

	// plan only
	tl.Flush()
	tl.Logger().Info("*** case: addAccountToServicePlan planOnly")
	fc.RetSvPUpdaterObj = nil
	fc.ModSvPUpdaterObj = nil
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.planOnly = true
	op.rhs.Request.ServicePlanAllocationCreateSpec.AuthorizedAccountID = authAccount
	op.addAccountToSDC(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Nil(fc.ModSvPUpdaterObj)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Plan:", op.rhs.Request.RequestMessages[0].Message)
	op.planOnly = false //clean up

	// addAccountToServicePlan update error
	tl.Flush()
	tl.Logger().Info("*** case: addAccountToServicePlan update error")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.domain = dClone(domain)
	op.cluster = clClone(cl)
	sp = spClone(spObj)
	op.sp = sp
	op.rhs.Request.ServicePlanAllocationCreateSpec.AuthorizedAccountID = authAccount
	fc.RetSvPUpdaterErr = fmt.Errorf("sp-update-error")
	fc.InSvPUpdaterItems = nil
	op.addAccountToSDC(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 3)
	assert.Regexp("ServicePlan update error.*sp-update-error", op.rhs.Request.RequestMessages[0].Message)
	assert.Equal(ctx, fc.InSvPUpdaterCtx)
	assert.Equal([]string{"accounts"}, fc.InSvPUpdaterItems.Append)

	// removeAccountFromServicePlan success
	tl.Flush()
	tl.Logger().Info("*** case: removeAccountFromServicePlan success")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.domain = dClone(domain)
	op.cluster = clClone(cl)
	op.sp = sp
	op.sp.Accounts = append(op.sp.Accounts, authAccount)
	op.rhs.Request.ServicePlanAllocationCreateSpec.AuthorizedAccountID = authAccount
	fc.RetSvPUpdaterErr = nil
	fc.InSvPUpdaterItems = nil
	fc.ForceFetchSvPUpdater = false
	op.removeAccountFromSDC(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(ctx, fc.InSvPUpdaterCtx)
	assert.EqualValues(op.sp.Meta.ID, models.ObjIDMutable(fc.InSvPUpdaterID))
	assert.Equal([]string{"accounts"}, fc.InSvPUpdaterItems.Remove)
	assert.Nil(fc.InSvPUpdaterItems.Set)
	assert.Nil(fc.InSvPUpdaterItems.Append)
	uoSP = fc.ModSvPUpdaterObj
	assert.EqualValues(uoSP, op.sp)
	assert.Contains(uoSP.Accounts, authAccount)
	assert.Regexp(fmt.Sprintf("Account .%s. removed from ServicePlan .%s", authAccount, sp.Meta.ID), op.rhs.Request.RequestMessages[0].Message)

	// removeAccountFromServicePlan already deleted
	tl.Flush()
	tl.Logger().Info("*** case: removeAccountFromServicePlan already deleted")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.domain = dClone(domain)
	op.cluster = clClone(cl)
	op.sp = sp
	op.sp.Accounts = []models.ObjIDMutable{}
	op.rhs.Request.ServicePlanAllocationCreateSpec.AuthorizedAccountID = authAccount
	op.removeAccountFromSDC(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 3)
	assert.Regexp(".*already removed from ServicePlan", op.rhs.Request.RequestMessages[0].Message)

	// plan only
	tl.Flush()
	tl.Logger().Info("*** case: removeAccountFromSDC planOnly")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}, planOnly: true}
	op.rhs.Request.ServicePlanAllocationCreateSpec.AuthorizedAccountID = authAccount
	op.removeAccountFromSDC(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 1)
	assert.Regexp("Plan: Remove account.*"+authAccount, op.rhs.Request.RequestMessages[0].Message)

	// addAccountToCspDomain success
	tl.Flush()
	tl.Logger().Info("*** case: addAccountToCspDomain success")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.domain = dClone(domain)
	op.cluster = clClone(cl)
	op.sp = spClone(sp)
	op.rhs.Request.ServicePlanAllocationCreateSpec.AuthorizedAccountID = authAccount
	fc.RetCdUpdaterErr = nil
	fc.InCdUpdaterItems = nil
	fc.ForceFetchCdUpdater = false
	op.addAccountToSDC(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(ctx, fc.InCdUpdaterCtx)
	assert.EqualValues(op.domain.Meta.ID, models.ObjIDMutable(fc.InCdUpdaterID))
	assert.Equal([]string{"authorizedAccounts"}, fc.InCdUpdaterItems.Append)
	assert.Nil(fc.InCdUpdaterItems.Set)
	assert.Nil(fc.InCdUpdaterItems.Remove)
	uoCD := fc.ModCdUpdaterObj
	assert.EqualValues(uoCD, op.domain)
	assert.Contains(uoCD.AuthorizedAccounts, authAccount)
	assert.Regexp(fmt.Sprintf("Account .%s. authorized for Domain .%s", authAccount, domain.Meta.ID), op.rhs.Request.RequestMessages[1].Message)

	// addAccountToCspDomain already exists
	tl.Flush()
	tl.Logger().Info("*** case: addAccountToCspDomain already exists")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.cluster = clClone(cl)
	op.sp = spClone(sp)
	op.domain = dClone(domain)
	op.domain.AuthorizedAccounts = append(op.domain.AuthorizedAccounts, authAccount)
	op.rhs.Request.ServicePlanAllocationCreateSpec.AuthorizedAccountID = authAccount
	fc.InCdUpdaterItems = nil
	op.addAccountToSDC(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Nil(fc.InCdUpdaterItems)
	assert.Regexp("Account .*already authorized for Domain", op.rhs.Request.RequestMessages[1].Message)

	// addAccountToCspDomain update error
	tl.Flush()
	tl.Logger().Info("*** case: addAccountToCspDomain update error")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.cluster = clClone(cl)
	op.sp = spClone(spObj)
	op.domain = dClone(domain)
	op.rhs.Request.ServicePlanAllocationCreateSpec.AuthorizedAccountID = authAccount
	fc.RetCdUpdaterErr = fmt.Errorf("cd-update-error")
	fc.InCdUpdaterItems = nil
	op.addAccountToSDC(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 3)
	assert.Regexp("Domain update error.*cd-update-error", op.rhs.Request.RequestMessages[1].Message)
	assert.Equal(ctx, fc.InCdUpdaterCtx)
	assert.Equal([]string{"authorizedAccounts"}, fc.InCdUpdaterItems.Append)

	// removeAccountFromCspDomain success
	tl.Flush()
	tl.Logger().Info("*** case: removeAccountFromCspDomain success")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.cluster = clClone(cl)
	op.sp = spClone(spObj)
	op.domain = dClone(domain)
	op.domain.AuthorizedAccounts = append(op.domain.AuthorizedAccounts, authAccount)
	op.rhs.Request.ServicePlanAllocationCreateSpec.AuthorizedAccountID = authAccount
	fc.RetCdUpdaterErr = nil
	fc.InCdUpdaterItems = nil
	fc.ForceFetchCdUpdater = false
	op.removeAccountFromSDC(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(ctx, fc.InCdUpdaterCtx)
	assert.EqualValues(op.domain.Meta.ID, models.ObjIDMutable(fc.InCdUpdaterID))
	assert.Equal([]string{"authorizedAccounts"}, fc.InCdUpdaterItems.Remove)
	assert.Nil(fc.InCdUpdaterItems.Set)
	assert.Nil(fc.InCdUpdaterItems.Append)
	uoCD = fc.ModCdUpdaterObj
	assert.EqualValues(uoCD, op.domain)
	assert.Contains(uoCD.AuthorizedAccounts, authAccount)
	assert.Regexp(fmt.Sprintf("Account .%s. removed from Domain .%s", authAccount, domain.Meta.ID), op.rhs.Request.RequestMessages[1].Message)

	// removeAccountFromCspDomain already deleted
	tl.Flush()
	tl.Logger().Info("*** case: removeAccountFromCspDomain already deleted")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}, cluster: cl, domain: domain, sp: spObj}
	op.domain = domain
	op.domain.AuthorizedAccounts = []models.ObjIDMutable{}
	op.rhs.Request.ServicePlanAllocationCreateSpec.AuthorizedAccountID = authAccount
	op.removeAccountFromSDC(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 3)
	assert.Regexp(".*already removed from Domain", op.rhs.Request.RequestMessages[1].Message)

	/////////

	// addAccountToCluster success
	tl.Flush()
	tl.Logger().Info("*** case: addAccountToCluster success")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.domain = dClone(domain)
	op.cluster = clClone(cl)
	op.sp = spClone(sp)
	op.rhs.Request.ServicePlanAllocationCreateSpec.AuthorizedAccountID = authAccount
	fc.RetClUpdaterErr = nil
	fc.InClUpdaterItems = nil
	fc.ForceFetchClUpdater = false
	op.addAccountToSDC(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(ctx, fc.InClUpdaterCtx)
	assert.EqualValues(op.cluster.Meta.ID, models.ObjIDMutable(fc.InClUpdaterID))
	assert.Equal([]string{"authorizedAccounts"}, fc.InClUpdaterItems.Append)
	assert.Nil(fc.InClUpdaterItems.Set)
	assert.Nil(fc.InClUpdaterItems.Remove)
	uoCL := fc.ModClUpdaterObj
	assert.EqualValues(uoCL, op.cluster)
	assert.Contains(uoCL.AuthorizedAccounts, authAccount)
	assert.Regexp(fmt.Sprintf("Account .%s. authorized for Cluster .%s", authAccount, cl.Meta.ID), op.rhs.Request.RequestMessages[2].Message)

	// addAccountToCluster already exists
	tl.Flush()
	tl.Logger().Info("*** case: addAccountToCluster already exists")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.cluster = clClone(cl)
	op.sp = spClone(sp)
	op.domain = dClone(domain)
	op.cluster.AuthorizedAccounts = append(op.cluster.AuthorizedAccounts, authAccount)
	op.rhs.Request.ServicePlanAllocationCreateSpec.AuthorizedAccountID = authAccount
	fc.InClUpdaterItems = nil
	op.addAccountToSDC(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Nil(fc.InClUpdaterItems)
	assert.Regexp("Account .*already authorized for Cluster", op.rhs.Request.RequestMessages[2].Message)

	// addAccountToCluster update error
	tl.Flush()
	tl.Logger().Info("*** case: addAccountToCluster update error")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.cluster = clClone(cl)
	op.sp = spClone(spObj)
	op.domain = dClone(domain)
	op.rhs.Request.ServicePlanAllocationCreateSpec.AuthorizedAccountID = authAccount
	fc.RetClUpdaterErr = fmt.Errorf("cl-update-error")
	fc.InClUpdaterItems = nil
	op.addAccountToSDC(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 3)
	assert.Regexp("Cluster update error.*cl-update-error", op.rhs.Request.RequestMessages[2].Message)
	assert.Equal(ctx, fc.InClUpdaterCtx)
	assert.Equal([]string{"authorizedAccounts"}, fc.InClUpdaterItems.Append)

	// removeAccountFromCluster success
	tl.Flush()
	tl.Logger().Info("*** case: removeAccountFromCluster success")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	op.cluster = clClone(cl)
	op.sp = spClone(spObj)
	op.domain = dClone(domain)
	op.cluster.AuthorizedAccounts = append(op.cluster.AuthorizedAccounts, authAccount)
	op.rhs.Request.ServicePlanAllocationCreateSpec.AuthorizedAccountID = authAccount
	fc.RetClUpdaterErr = nil
	fc.InClUpdaterItems = nil
	fc.ForceFetchClUpdater = false
	op.removeAccountFromSDC(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(ctx, fc.InClUpdaterCtx)
	assert.EqualValues(op.cluster.Meta.ID, models.ObjIDMutable(fc.InClUpdaterID))
	assert.Equal([]string{"authorizedAccounts"}, fc.InClUpdaterItems.Remove)
	assert.Nil(fc.InClUpdaterItems.Set)
	assert.Nil(fc.InClUpdaterItems.Append)
	uoCL = fc.ModClUpdaterObj
	assert.EqualValues(uoCL, op.cluster)
	assert.Contains(uoCL.AuthorizedAccounts, authAccount)
	assert.Regexp(fmt.Sprintf("Account .%s. removed from Cluster .%s", authAccount, cl.Meta.ID), op.rhs.Request.RequestMessages[2].Message)

	// removeAccountFromCluster already deleted
	tl.Flush()
	tl.Logger().Info("*** case: removeAccountFromCluster already deleted")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}, cluster: cl, domain: domain, sp: spObj}
	op.cluster.AuthorizedAccounts = []models.ObjIDMutable{}
	op.rhs.Request.ServicePlanAllocationCreateSpec.AuthorizedAccountID = authAccount
	op.removeAccountFromSDC(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Len(op.rhs.Request.RequestMessages, 3)
	assert.Regexp(".*already removed from Cluster", op.rhs.Request.RequestMessages[2].Message)

	// ******************** setPoolCapacityForSPA

	stN := "Amazon gp2"
	poolObj := &models.Pool{}
	poolObj.Meta = &models.ObjMeta{ID: "SPA-1"}
	poolObj.CspStorageType = stN
	poolObj.ServicePlanReservations = map[string]models.StorageTypeReservation{
		"SPA-1": models.StorageTypeReservation{NumMirrors: 1, SizeBytes: swag.Int64(1000)},
		"SPA-2": models.StorageTypeReservation{NumMirrors: 1, SizeBytes: swag.Int64(2000)},
	}
	var pRet *models.Pool
	var err error

	// reservation present, no change
	tl.Logger().Info("case: setPoolCapacityForSPA no change")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	p := poolClone(poolObj)
	str, ok := p.ServicePlanReservations["SPA-1"]
	assert.True(ok)
	sca := &SetCapacityArgs{
		ServicePlanAllocationID: "SPA-1",
		Capacity:                &models.PoolReservation{NumMirrors: str.NumMirrors, SizeBytes: str.SizeBytes},
		Pool:                    p,
	}
	pRet, err = op.setPoolCapacityForSPA(ctx, sca)
	assert.NoError(err)
	assert.NotNil(pRet)
	assert.Equal(p, pRet)

	// reservation inserted, tcb changed
	tl.Logger().Info("case: setPoolCapacityForSPA success reservation inserted")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	p = poolClone(poolObj)
	strSPA1, ok := p.ServicePlanReservations["SPA-1"]
	assert.True(ok)
	p.ServicePlanReservations = nil
	sca = &SetCapacityArgs{
		ServicePlanAllocationID: "SPA-1",
		Capacity:                &models.PoolReservation{NumMirrors: strSPA1.NumMirrors, SizeBytes: strSPA1.SizeBytes},
		Pool:                    p,
	}
	pRet, err = op.setPoolCapacityForSPA(ctx, sca)
	assert.NoError(err)
	assert.NotNil(pRet)
	assert.Equal(pRet, fc.ModPoolUpdaterObj)
	assert.Len(pRet.ServicePlanReservations, 1)
	str, ok = pRet.ServicePlanReservations["SPA-1"]
	assert.True(ok)
	assert.Equal(strSPA1, str)

	// reservation changed
	tl.Logger().Info("case: setPoolCapacityForSPA reservation changed")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	p = poolClone(poolObj)
	strSPA2, ok := p.ServicePlanReservations["SPA-2"]
	assert.True(ok)
	sca = &SetCapacityArgs{
		ServicePlanAllocationID: "SPA-1",
		Capacity:                &models.PoolReservation{NumMirrors: strSPA1.NumMirrors, SizeBytes: swag.Int64(4000)},
		Pool:                    p,
	}
	pRet, err = op.setPoolCapacityForSPA(ctx, sca)
	assert.NoError(err)
	assert.NotNil(pRet)
	assert.Equal(pRet, fc.ModPoolUpdaterObj)
	assert.Len(pRet.ServicePlanReservations, 2)
	str, ok = pRet.ServicePlanReservations["SPA-1"]
	assert.True(ok)
	assert.Equal(int32(1), str.NumMirrors)
	assert.Equal(int64(4000), *str.SizeBytes)
	str, ok = pRet.ServicePlanReservations["SPA-2"]
	assert.True(ok)
	assert.Equal(strSPA2, str)

	// reservation removed (num mirrors 0)
	tl.Logger().Info("case: setPoolCapacityForSPA reservation removed (#M=0)")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	p = poolClone(poolObj)
	sca = &SetCapacityArgs{
		ServicePlanAllocationID: "SPA-1",
		Capacity:                &models.PoolReservation{NumMirrors: 0, SizeBytes: swag.Int64(1)},
		Pool:                    p,
	}
	pRet, err = op.setPoolCapacityForSPA(ctx, sca)
	assert.NoError(err)
	assert.NotNil(pRet)
	assert.Equal(pRet, fc.ModPoolUpdaterObj)
	assert.Len(pRet.ServicePlanReservations, 1)
	_, ok = pRet.ServicePlanReservations["SPA-1"]
	assert.False(ok)
	str, ok = pRet.ServicePlanReservations["SPA-2"]
	assert.True(ok)
	assert.Equal(strSPA2, str)

	// reservation removed (size 0)
	tl.Logger().Info("case: setPoolCapacityForSPA reservation removed (Sz=0)")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	p = poolClone(poolObj)
	sca = &SetCapacityArgs{
		ServicePlanAllocationID: "SPA-1",
		Capacity:                &models.PoolReservation{NumMirrors: 1, SizeBytes: swag.Int64(0)},
		Pool:                    p,
	}
	pRet, err = op.setPoolCapacityForSPA(ctx, sca)
	assert.NoError(err)
	assert.NotNil(pRet)
	assert.Equal(pRet, fc.ModPoolUpdaterObj)
	assert.Len(pRet.ServicePlanReservations, 1)
	_, ok = pRet.ServicePlanReservations["SPA-1"]
	assert.False(ok)
	str, ok = pRet.ServicePlanReservations["SPA-2"]
	assert.True(ok)
	assert.Equal(strSPA2, str)

	// no reservation (num mirrors 0)
	tl.Logger().Info("case:setPoolCapacityForSPA  no reservation (#M=0)")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	fc.ModPoolUpdaterObj = nil
	p = poolClone(poolObj)
	delete(p.ServicePlanReservations, "SPA-1")
	sca = &SetCapacityArgs{
		ServicePlanAllocationID: "SPA-1",
		Capacity:                &models.PoolReservation{NumMirrors: 0, SizeBytes: swag.Int64(1)},
		Pool:                    p,
	}
	pRet, err = op.setPoolCapacityForSPA(ctx, sca)
	assert.NoError(err)
	assert.NotNil(pRet)
	assert.Nil(fc.ModPoolUpdaterObj)
	assert.Equal(p, pRet)
	assert.Len(pRet.ServicePlanReservations, 1)
	_, ok = pRet.ServicePlanReservations["SPA-1"]
	assert.False(ok)
	str, ok = pRet.ServicePlanReservations["SPA-2"]
	assert.True(ok)
	assert.Equal(strSPA2, str)

	// no reservation (size 0)
	tl.Logger().Info("case: setPoolCapacityForSPA no reservation (Sz=0)")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	fc.ModPoolUpdaterObj = nil
	p = poolClone(poolObj)
	delete(p.ServicePlanReservations, "SPA-1")
	sca = &SetCapacityArgs{
		ServicePlanAllocationID: "SPA-1",
		Capacity:                &models.PoolReservation{NumMirrors: 1, SizeBytes: swag.Int64(0)},
		Pool:                    p,
	}
	pRet, err = op.setPoolCapacityForSPA(ctx, sca)
	assert.NoError(err)
	assert.NotNil(pRet)
	assert.Nil(fc.ModPoolUpdaterObj)
	assert.Equal(p, pRet)
	assert.Len(pRet.ServicePlanReservations, 1)
	_, ok = pRet.ServicePlanReservations["SPA-1"]
	assert.False(ok)
	str, ok = pRet.ServicePlanReservations["SPA-2"]
	assert.True(ok)
	assert.Equal(strSPA2, str)

	// update error
	tl.Logger().Info("case: setPoolCapacityForSPA update error")
	op = &acOp{c: c, rhs: &vra.RequestHandlerState{A: c.Animator, Request: vsrClone(vsr)}}
	fc.RetPoolUpdaterErr = fmt.Errorf("sp-update-err")
	p = poolClone(poolObj)
	str, ok = p.ServicePlanReservations["SPA-1"]
	assert.True(ok)
	p.ServicePlanReservations = nil
	sca = &SetCapacityArgs{
		ServicePlanAllocationID: "SPA-1",
		Capacity:                &models.PoolReservation{NumMirrors: str.NumMirrors, SizeBytes: str.SizeBytes},
		Pool:                    p,
	}
	pRet, err = op.setPoolCapacityForSPA(ctx, sca)
	assert.Error(err)
	assert.Regexp("sp-update-err", err)
	assert.Nil(pRet)
}

func TestAllocateCapacity(t *testing.T) {
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
	domain := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta:          &models.ObjMeta{ID: "CSP-DOMAIN-1"},
			CspDomainType: "AWS",
		},
	}
	vsrObj := &models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "VSR-1",
			},
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			RequestedOperations: []string{"ALLOCATE_CAPACITY"},
			ServicePlanAllocationCreateSpec: &models.ServicePlanAllocationCreateArgs{
				ServicePlanAllocationCreateOnce: models.ServicePlanAllocationCreateOnce{
					AccountID:           "OWNER-ACCOUNT",
					AuthorizedAccountID: "AUTHORIZED-ACCOUNT",
					ClusterID:           "CLUSTER-1",
					ServicePlanID:       "SERVICE-PLAN-1",
				},
			},
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				VolumeSeriesRequestState: "ALLOCATING_CAPACITY",
			},
		},
	}
	spaObj := &models.ServicePlanAllocation{}

	// test ALLOCATE_CAPACITY transition sequences
	tl.Logger().Info("TESTING FORWARD STATE TRANSITIONS")
	tl.Flush()
	vsr := vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "ALLOCATING_CAPACITY"

	// found, validation failure
	tl.Logger().Info("Case: found, validation failure")
	tl.Flush()
	op := &fakeACOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vsrClone(vsr)}
	op.retGIS = ACLoadObjects
	op.retFindObj = spaObj
	op.retValidateSPAInErr = true
	expCalled := []string{"GIS", "LO", "WFL", "FSPA", "VSPA"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// found, validation ok, planning done, claim error
	tl.Logger().Info("Case: found, validation ok, planning done, claim error")
	tl.Flush()
	op = &fakeACOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vsrClone(vsr)}
	op.retGIS = ACLoadObjects
	op.gisPlanningDone = true
	op.retFindObj = spaObj
	op.retMarkSPALater = true
	expCalled = []string{"GIS", "LO", "WFL", "FSPA", "VSPA", "MSPA"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// found, validation ok, planning done, plan only
	tl.Logger().Info("Case: found, validation ok, planning done, update ok, plan only")
	tl.Flush()
	op = &fakeACOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vsrClone(vsr)}
	op.retGIS = ACLoadObjects
	op.gisPlanningDone = true
	op.gisPlanOnly = true
	op.retFindObj = spaObj
	expCalled = []string{"GIS", "LO", "WFL", "FSPA", "VSPA", "MSPA", "FP", "CP"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// found, validation ok, planning done, !plan only, update ok
	tl.Logger().Info("Case: found, validation ok, planning done, update ok")
	tl.Flush()
	op = &fakeACOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vsrClone(vsr)}
	op.retGIS = ACLoadObjects
	op.gisPlanningDone = true
	op.retFindObj = spaObj
	expCalled = []string{"GIS", "LO", "WFL", "FSPA", "VSPA", "MSPA", "FP", "CP", "CRR", "UR", "ACP", "AASDC", "FIN"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// found, validation ok, update ok, planning not done, plan only
	tl.Logger().Info("Case: found, validation ok, update ok, planning not done, plan only")
	tl.Flush()
	op = &fakeACOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vsrClone(vsr)}
	op.retGIS = ACLoadObjects
	op.gisPlanOnly = true
	op.retFindObj = spaObj
	expCalled = []string{"GIS", "LO", "WFL", "FSPA", "VSPA", "SSF", "UR", "MSPA", "FP", "CP"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// found, validation ok, update ok, planning not done, !plan only
	tl.Logger().Info("Case: found, validation ok, update ok, planning not done")
	tl.Flush()
	op = &fakeACOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vsrClone(vsr)}
	op.retGIS = ACLoadObjects
	op.retFindObj = spaObj
	expCalled = []string{"GIS", "LO", "WFL", "FSPA", "VSPA", "SSF", "UR", "MSPA", "FP", "CP", "CRR", "UR", "ACP", "AASDC", "FIN"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// found, validation ok, update ok, planning not done, !plan only, skip crr
	tl.Logger().Info("Case: found, validation ok, update ok, planning not done")
	tl.Flush()
	op = &fakeACOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vsrClone(vsr)}
	crr := &models.CapacityReservationResult{
		CurrentReservations: map[string]models.PoolReservation{},
		DesiredReservations: map[string]models.PoolReservation{},
	}
	crr.DesiredReservations["POOL-AA-CL-Amazon GP2-2"] = models.PoolReservation{NumMirrors: 2, SizeBytes: swag.Int64(2000)}
	op.rhs.Request.CapacityReservationResult = crr
	op.retGIS = ACLoadObjects
	op.retFindObj = spaObj
	expCalled = []string{"GIS", "LO", "WFL", "FSPA", "VSPA", "SSF", "UR", "MSPA", "FP", "CP", "UR", "ACP", "AASDC", "FIN"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// found, validation ok, update ok, planning not done, !plan only, skip crr
	tl.Logger().Info("Case: found, validation ok, update ok, planning not done")
	tl.Flush()
	op = &fakeACOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vsrClone(vsr)}
	crr = &models.CapacityReservationResult{
		CurrentReservations: map[string]models.PoolReservation{},
		DesiredReservations: map[string]models.PoolReservation{},
	}
	op.rhs.Request.CapacityReservationResult = crr
	op.retGIS = ACLoadObjects
	op.retFindObj = spaObj
	expCalled = []string{"GIS", "LO", "WFL", "FSPA", "VSPA", "SSF", "UR", "MSPA", "FP", "CP", "CRR", "UR", "ACP", "AASDC", "FIN"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// not found, planning  done, create error
	tl.Logger().Info("Case: not found, planning  done, create error")
	tl.Flush()
	op = &fakeACOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vsrClone(vsr)}
	op.retGIS = ACLoadObjects
	op.gisPlanningDone = true
	op.retCreateSPALater = true
	expCalled = []string{"GIS", "LO", "WFL", "FSPA", "CSPA"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// not found, planning  done, create
	tl.Logger().Info("Case: not found, planning  done, create")
	tl.Flush()
	op = &fakeACOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vsrClone(vsr)}
	op.retGIS = ACLoadObjects
	op.gisPlanningDone = true
	expCalled = []string{"GIS", "LO", "WFL", "FSPA", "CSPA", "FP", "CP", "CRR", "UR", "ACP", "AASDC", "FIN"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// not found, planning not done, create spa
	tl.Logger().Info("Case: not found, planning not done, create")
	tl.Flush()
	op = &fakeACOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vsrClone(vsr)}
	op.retGIS = ACLoadObjects
	expCalled = []string{"GIS", "LO", "WFL", "FSPA", "SSF", "UR", "CSPA", "FP", "CP", "CRR", "UR", "ACP", "AASDC", "FIN"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// test UNDO_ALLOCATE_CAPACITY transition sequences
	tl.Logger().Info("TESTING UNDO STATE TRANSITIONS")
	tl.Flush()
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "UNDO_ALLOCATING_CAPACITY"

	// plan only
	tl.Logger().Info("Case: UNDO plan only")
	tl.Flush()
	op = &fakeACOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vsrClone(vsr)}
	op.rhs.Request.PlanOnly = swag.Bool(true)
	op.retGIS = ACUndoDone
	op.gisPlanOnly = true
	expCalled = []string{"GIS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// not found
	tl.Logger().Info("Case: UNDO not found")
	tl.Flush()
	op = &fakeACOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vsrClone(vsr)}
	op.retGIS = ACUndoFindSPA
	expCalled = []string{"GIS", "FSPA"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// found, not owned
	tl.Logger().Info("Case: UNDO found, not owned")
	tl.Flush()
	op = &fakeACOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vsrClone(vsr)}
	op.retGIS = ACUndoFindSPA
	op.retFindObj = spaClone(spaObj)
	op.retFindObj.SystemTags = []string{}
	expCalled = []string{"GIS", "FSPA"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// found, owned, not created
	tl.Logger().Info("Case: UNDO found, owned, not created")
	tl.Flush()
	op = &fakeACOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vsrClone(vsr)}
	op.retGIS = ACUndoFindSPA
	op.retFindObj = spaClone(spaObj)
	op.retFindObj.SystemTags = []string{op.sTagOp()}
	expCalled = []string{"GIS", "FSPA", "LO", "WFL", "FP", "RCP", "RPR", "DP", "CSIR", "REL"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// found, owned, created
	tl.Logger().Info("Case: UNDO found, owned, created")
	tl.Flush()
	op = &fakeACOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vsrClone(vsr)}
	op.rhs.Request.SystemTags = append(op.rhs.Request.SystemTags, op.vsrCreateSpaSysTag())
	op.retGIS = ACUndoFindSPA
	op.retFindObj = spaClone(spaObj)
	op.retFindObj.SystemTags = []string{op.sTagOp(), op.sTagCreator()}
	expCalled = []string{"GIS", "FSPA", "LO", "WFL", "FP", "RCP", "RPR", "DP", "CSIR", "DEL", "RASDC"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// found, not owned, created
	tl.Logger().Info("Case: UNDO found, not owned, created")
	tl.Flush()
	op = &fakeACOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vsrClone(vsr)}
	op.rhs.Request.SystemTags = append(op.rhs.Request.SystemTags, op.vsrCreateSpaSysTag())
	op.retGIS = ACUndoFindSPA
	op.retFindObj = spaClone(spaObj)
	op.retFindObj.SystemTags = []string{op.sTagCreator()}
	expCalled = []string{"GIS", "FSPA", "LO", "WFL", "FP", "RCP", "RPR", "DP", "CSIR", "DEL", "RASDC"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// test DELETE_SPA transition sequences
	tl.Logger().Info("TESTING DELETE_SPA STATE TRANSITIONS")
	tl.Flush()
	vsr = vsrClone(vsrObj)
	vsr.RequestedOperations = []string{"DELETE_SPA"}
	vsr.VolumeSeriesRequestState = "DELETING_SPA"

	// plan only
	tl.Logger().Info("Case: DELETE_SPA plan only")
	tl.Flush()
	op = &fakeACOps{}
	op.c = c
	op.ops = op
	op.retFindObj = spaClone(spaObj)
	op.rhs = &vra.RequestHandlerState{Request: vsrClone(vsr), HasDeleteSPA: true}
	op.rhs.Request.PlanOnly = swag.Bool(true)
	op.retGIS = ACLoadObjects
	op.gisPlanOnly = true
	expCalled = []string{"GIS", "LO", "WFL", "FSPA", "VSPA", "SSF", "UR", "MSPA", "FP", "RASDC"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// not found
	tl.Logger().Info("Case: DELETE_SPA not found")
	tl.Flush()
	op = &fakeACOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vsrClone(vsr), HasDeleteSPA: true}
	op.retGIS = ACLoadObjects
	expCalled = []string{"GIS", "LO", "WFL", "FSPA", "RASDC"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// found, validation failure
	tl.Logger().Info("Case: found, validation failure")
	tl.Flush()
	op = &fakeACOps{}
	op.c = c
	op.ops = op
	op.rhs = &vra.RequestHandlerState{Request: vsrClone(vsr), HasDeleteSPA: true}
	op.retGIS = ACLoadObjects
	op.retFindObj = spaClone(spaObj)
	op.retValidateSPAInErr = true
	expCalled = []string{"GIS", "LO", "WFL", "FSPA", "VSPA"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// found, planning done
	op = &fakeACOps{}
	op.c = c
	op.ops = op
	op.retFindObj = spaClone(spaObj)
	op.rhs = &vra.RequestHandlerState{Request: vsrClone(vsr), HasDeleteSPA: true}
	op.gisPlanningDone = true
	op.retGIS = ACLoadObjects
	expCalled = []string{"GIS", "LO", "WFL", "FSPA", "VSPA", "MSPA", "FP", "CP", "CRR", "UR", "ACP", "RSR", "RPR", "DP", "CSIR", "DEL", "RASDC"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// found, planning needed
	op = &fakeACOps{}
	op.c = c
	op.ops = op
	op.retFindObj = spaClone(spaObj)
	op.rhs = &vra.RequestHandlerState{Request: vsrClone(vsr), HasDeleteSPA: true}
	op.retGIS = ACLoadObjects
	expCalled = []string{"GIS", "LO", "WFL", "FSPA", "VSPA", "SSF", "UR", "MSPA", "FP", "CP", "CRR", "UR", "ACP", "RSR", "RPR", "DP", "CSIR", "DEL", "RASDC"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// call the real handler and fail after entering the CS; check release of the CST
	ctx := context.Background()
	vsr = vsrClone(vsrObj)
	rhs := &vra.RequestHandlerState{
		A:       c.Animator,
		Request: vsr,
	}
	tl.Logger().Info("CALL REAL ALLOCATE_CAPACITY")
	tl.Flush()
	assert.Empty(op.c.reservationCS.Status())
	assert.Equal(0, op.c.reservationCS.Used)
	fc.RetSPAListErr = fmt.Errorf("spa-list-error")
	fc.RetLDObj = domain
	fc.RetLClObj = cl
	c.AllocateCapacity(ctx, rhs)
	assert.Empty(op.c.reservationCS.Status())
	assert.Equal(1, op.c.reservationCS.Used)

	// call the real handler for DELETE_SPA
	tl.Logger().Info("CALL REAL DELETE_SPA")
	tl.Flush()
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "DELETING_SPA"
	rhs = &vra.RequestHandlerState{
		A:            c.Animator,
		InError:      true,
		TimedOut:     false,
		HasDeleteSPA: true,
		Request:      vsr,
	}
	fc.RetSPAListErr = fmt.Errorf("spa-list-error")
	c.UndoAllocateCapacity(ctx, rhs)

	// call the real undo handler
	tl.Logger().Info("CALL REAL UNDO_ALLOCATE_CAPACITY")
	tl.Flush()
	vsr = vsrClone(vsrObj)
	vsr.VolumeSeriesRequestState = "UNDO_ALLOCATING_CAPACITY"
	rhs = &vra.RequestHandlerState{
		A:        c.Animator,
		InError:  true,
		TimedOut: false,
		Request:  vsr,
	}
	fc.RetSPAListErr = fmt.Errorf("spa-list-error")
	c.UndoAllocateCapacity(ctx, rhs)

	// check state strings exist up to ACError
	var ss acSubState
	for ss = ACLoadObjects; ss < ACNoOp; ss++ {
		s := ss.String()
		tl.Logger().Debugf("Testing %d %s", ss, s)
		assert.Regexp("^AC", s)
	}
	assert.Regexp("^acSubState", ss.String())
}

type fakeACOps struct {
	acOp
	called              []string
	retGIS              acSubState
	gisPlanningDone     bool
	gisPlanOnly         bool
	retFindObj          *models.ServicePlanAllocation
	retValidateSPAInErr bool
	retMarkSPALater     bool
	retCreateSPALater   bool
	scaArgs             *SetCapacityArgs
	retScaPool          *models.Pool
	retScaErr           error
}

func (op *fakeACOps) getInitialState(ctx context.Context) acSubState {
	op.called = append(op.called, "GIS")
	op.planningDone = op.gisPlanningDone
	op.planOnly = op.gisPlanOnly
	return op.retGIS
}

func (op *fakeACOps) loadObjects(ctx context.Context) {
	op.called = append(op.called, "LO")
}

func (op *fakeACOps) updateRequest(ctx context.Context) {
	op.called = append(op.called, "UR")
}

func (op *fakeACOps) waitForLock(ctx context.Context) {
	op.called = append(op.called, "WFL")
}

func (op *fakeACOps) selectStorageFormula(ctx context.Context) {
	op.called = append(op.called, "SSF")
}

func (op *fakeACOps) findSPA(ctx context.Context) {
	op.called = append(op.called, "FSPA")
	op.spa = op.retFindObj
}

func (op *fakeACOps) validateVSRAgainstSPA(ctx context.Context) {
	op.called = append(op.called, "VSPA")
	op.rhs.InError = op.retValidateSPAInErr
}

func (op *fakeACOps) markSPAInUse(ctx context.Context) {
	op.called = append(op.called, "MSPA")
	op.rhs.RetryLater = op.retMarkSPALater
}

func (op *fakeACOps) createSPA(ctx context.Context) {
	op.called = append(op.called, "CSPA")
	op.rhs.RetryLater = op.retCreateSPALater
}

func (op *fakeACOps) finalizeSPA(ctx context.Context) {
	op.called = append(op.called, "FIN")
}

func (op *fakeACOps) releaseSPA(ctx context.Context) {
	op.called = append(op.called, "REL")
}

func (op *fakeACOps) deleteSPA(ctx context.Context) {
	op.called = append(op.called, "DEL")
}

func (op *fakeACOps) findPools(ctx context.Context) {
	op.called = append(op.called, "FP")
}

func (op *fakeACOps) createPools(ctx context.Context) {
	op.called = append(op.called, "CP")
}

func (op *fakeACOps) deletePools(ctx context.Context) {
	op.called = append(op.called, "DP")
}

func (op *fakeACOps) initializeCRR(ctx context.Context) {
	op.called = append(op.called, "CRR")
}

func (op *fakeACOps) addCapacityToPools(ctx context.Context) {
	op.called = append(op.called, "ACP")
}

func (op *fakeACOps) removeCapacityFromPools(ctx context.Context) {
	op.called = append(op.called, "RCP")
}

func (op *fakeACOps) removeEmptyPoolReferences(ctx context.Context) {
	op.called = append(op.called, "RPR")
}

func (op *fakeACOps) removeStorageReservations(ctx context.Context) {
	op.called = append(op.called, "RSR")
}

func (op *fakeACOps) addAccountToSDC(ctx context.Context) {
	op.called = append(op.called, "AASDC")
}

func (op *fakeACOps) removeAccountFromSDC(ctx context.Context) {
	op.called = append(op.called, "RASDC")
}

func (op *fakeACOps) clearSpaIDInRequest(ctx context.Context) {
	op.called = append(op.called, "CSIR")
}

func (op *fakeACOps) setPoolCapacityForSPA(ctx context.Context, args *SetCapacityArgs) (*models.Pool, error) {
	op.scaArgs = args
	return op.retScaPool, op.retScaErr
}
