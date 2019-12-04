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
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	appmock "github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	mockcsp "github.com/Nuvoloso/kontroller/pkg/csp/mock"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestReleaseSteps(t *testing.T) {
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
	fc := &fake.Client{}
	c.oCrud = fc
	c.systemID = "SYSTEM"
	ctx := context.Background()

	// Invoke the provisioning steps in order (success cases last to fall through)
	now := time.Now()
	s := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta: &models.ObjMeta{
				ID:      "STORAGE-1",
				Version: 1,
			},
			CspDomainID:    "CSP-DOMAIN-1",
			CspStorageType: "Amazon gp2",
			SizeBytes:      swag.Int64(1000),
			StorageAccessibility: &models.StorageAccessibility{
				StorageAccessibilityMutable: models.StorageAccessibilityMutable{
					AccessibilityScope:      "CSPDOMAIN",
					AccessibilityScopeObjID: "CSP-DOMAIN-1",
				},
			},
			PoolID: "PROV-1",
		},
		StorageMutable: models.StorageMutable{
			AvailableBytes: swag.Int64(1000),
			StorageState: &models.StorageStateMutable{
				AttachmentState:  "DETACHED",
				ProvisionedState: "PROVISIONED",
			},
		},
	}
	sr := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:      "SR-1",
				Version: 1,
			},
		},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			CompleteByTime:      strfmt.DateTime(now.Add(time.Hour)),
			PoolID:              "PROV-1",
			RequestedOperations: []string{"PROVISION", "ATTACH"},
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
				StorageRequestState: "RELEASING",
			},
			StorageRequestCreateMutable: models.StorageRequestCreateMutable{
				StorageID: models.ObjIDMutable(s.Meta.ID),
			},
		},
	}
	sp := &models.Pool{
		PoolAllOf0: models.PoolAllOf0{
			Meta: &models.ObjMeta{
				ID:      "PROV-1",
				Version: 1,
			},
		},
	}
	cluster := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID:      "CLUSTER-1",
				Version: 1,
			},
		},
		ClusterMutable: models.ClusterMutable{
			ClusterCreateMutable: models.ClusterCreateMutable{
				ClusterAttributes: map[string]models.ValueType{},
			},
		},
	}
	dom := &models.CSPDomain{
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

	// the release op
	op := &releaseOp{
		c: c,
		rhs: &requestHandlerState{
			c:         c,
			Request:   sr,
			CSPDomain: dom,
			Storage:   s,
			Pool:      sp,
			Cluster:   cluster,
		},
	}

	//  ***************************** getInitialState
	op.rhs.TimedOut = true
	assert.Equal(RelDone, op.getInitialState(ctx))
	op.rhs.TimedOut = false // reset

	op.rhs.InError = true
	assert.Equal(RelDone, op.getInitialState(ctx))
	op.rhs.InError = false // reset

	// no storage set
	op.rhs.InError = false    // reset
	op.rhs.TimedOut = false   // reset
	op.rhs.RetryLater = false // reset
	op.rhs.Storage = nil
	assert.Equal(RelDone, op.getInitialState(ctx))
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)

	// storage set, in UNPROVISIONING
	op.rhs.InError = false    // reset
	op.rhs.TimedOut = false   // reset
	op.rhs.RetryLater = false // reset
	op.rhs.Storage = s        // reset
	s.StorageState.ProvisionedState = com.StgProvisionedStateUnprovisioning
	assert.Equal(RelDeleteStorage, op.getInitialState(ctx))
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	// storage set, not UNPROVISIONING
	op.rhs.InError = false                                               // reset
	op.rhs.TimedOut = false                                              // reset
	op.rhs.RetryLater = false                                            // reset
	op.rhs.Storage = s                                                   // reset
	s.StorageState.ProvisionedState = com.StgProvisionedStateProvisioned // reset
	assert.Equal(RelSetStorageState, op.getInitialState(ctx))
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)

	// UNDO_PROVISIONING takes normal path for PROVISIONED storage
	op.rhs.InError = true
	sr.StorageRequestState = "UNDO_PROVISIONING"
	assert.Equal(RelSetStorageState, op.getInitialState(ctx))
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.True(op.wasInError)
	sr.StorageRequestState = "RELEASING" // reset
	op.wasInError = false                // reset

	//  ***************************** setStorageState

	// fail to update storage state
	op.rhs.RetryLater = false                                 // reset
	sr.RequestMessages = make([]*models.TimestampedString, 0) // reset
	fc.RetPoolFetchErr = nil
	fc.RetPoolFetchObj = sp
	fc.RetUSObj = nil
	fc.RetUSErr = fmt.Errorf("update error")
	fc.InSRUpdaterID = ""
	op.setStorageState(ctx)
	assert.True(op.rhs.RetryLater)
	assert.Regexp("ProvisionedState change.* "+com.StgProvisionedStateUnprovisioning, s.StorageState.Messages[0].Message)
	assert.Regexp("Failed to update Storage state", sr.RequestMessages[0].Message)
	assert.EqualValues(sr.Meta.ID, fc.InSRUpdaterID)

	// success: fail to tag volumes but updated storage state
	op.rhs.RetryLater = false                                            // reset
	sr.RequestMessages = make([]*models.TimestampedString, 0)            // reset
	s.StorageState.ProvisionedState = com.StgProvisionedStateProvisioned // reset
	s.StorageState.Messages = make([]*models.TimestampedString, 0)       // reset
	newSObj := &models.Storage{}
	fc.RetUSObj = newSObj
	fc.RetUSErr = nil
	op.setStorageState(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.True(s.StorageState.ProvisionedState == com.StgProvisionedStateUnprovisioning)
	assert.Regexp("ProvisionedState change.* "+com.StgProvisionedStateUnprovisioning, s.StorageState.Messages[0].Message)
	assert.Equal(newSObj, op.rhs.Storage)

	//  ***************************** deleteStorage

	// fail to get domain client
	s.StorageIdentifier = "v-123"
	op.rhs.Storage = s                                        // reset
	op.rhs.RetryLater = false                                 // reset
	sr.RequestMessages = make([]*models.TimestampedString, 0) // reset
	fc.RetPoolFetchErr = nil
	fc.RetPoolFetchObj = sp
	fc.InSRUpdaterID = ""
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	appCSP := appmock.NewMockAppCloudServiceProvider(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(nil, fmt.Errorf("get domain client error"))
	op.c.App.AppCSP = appCSP
	op.deleteStorage(ctx)
	assert.True(op.rhs.RetryLater)
	assert.Regexp("Domain client failure deleting volume", sr.RequestMessages[0].Message)
	assert.EqualValues(sr.Meta.ID, fc.InSRUpdaterID)

	// fail to delete CSP volume
	op.rhs.RetryLater = false                                 // reset
	sr.RequestMessages = make([]*models.TimestampedString, 0) // reset
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC := mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(cspDC, nil)
	vda := &csp.VolumeDeleteArgs{
		VolumeIdentifier:       op.rhs.Storage.StorageIdentifier,
		ProvisioningAttributes: op.rhs.Cluster.ClusterAttributes,
	}
	cspDC.EXPECT().VolumeDelete(ctx, vda).Return(fmt.Errorf("delete error"))
	op.c.App.AppCSP = appCSP
	fc.InSRUpdaterID = ""
	op.deleteStorage(ctx)
	assert.True(op.rhs.RetryLater)
	assert.Regexp("Failed to delete CSP volume", sr.RequestMessages[0].Message)
	assert.EqualValues(sr.Meta.ID, fc.InSRUpdaterID)

	// failed to delete CSP volume with not-found error, failed to delete Storage
	op.rhs.RetryLater = false                                 // reset
	op.rhs.InError = false                                    // reset
	sr.RequestMessages = make([]*models.TimestampedString, 0) // reset
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(cspDC, nil)
	cspDC.EXPECT().VolumeDelete(ctx, vda).Return(csp.ErrorVolumeNotFound)
	op.c.App.AppCSP = appCSP
	fc.RetDSErr = fmt.Errorf("storage delete error")
	fc.InSRUpdaterID = ""
	op.deleteStorage(ctx)
	assert.True(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("Failed to delete Storage", sr.RequestMessages[0].Message)
	assert.EqualValues(sr.Meta.ID, fc.InSRUpdaterID)

	// success, csp volume and storage deleted
	op.rhs.RetryLater = false                                 // reset
	op.rhs.InError = false                                    // reset
	sr.RequestMessages = make([]*models.TimestampedString, 0) // reset
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(cspDC, nil)
	cspDC.EXPECT().VolumeDelete(ctx, vda).Return(nil)
	op.c.App.AppCSP = appCSP
	fc.RetDSErr = nil
	op.deleteStorage(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("Deleted Storage", sr.RequestMessages[0].Message)
	tl.Flush()

	// success: no csp volume to delete, storage deleted
	s.StorageIdentifier = ""
	op.rhs.RetryLater = false                                 // reset
	op.rhs.InError = false                                    // reset
	sr.RequestMessages = make([]*models.TimestampedString, 0) // reset
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	op.c.App.AppCSP = appCSP
	op.deleteStorage(ctx)
	assert.False(op.rhs.RetryLater)
	assert.False(op.rhs.InError)
	assert.Regexp("Deleted Storage", sr.RequestMessages[0].Message)
	assert.Equal(1, tl.CountPattern("no CSP volume to delete"))
}

func TestReleaseStorage(t *testing.T) {
	assert := assert.New(t)

	sr := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "SR-1",
			},
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
				StorageRequestState: "RELEASING",
			},
			StorageRequestCreateMutable: models.StorageRequestCreateMutable{
				StorageID: models.ObjIDMutable("STORAGE-1"),
			},
		},
	}
	op := &fakeReleaseOps{}
	op.ops = op
	op.rhs = &requestHandlerState{Request: sr, Pool: &models.Pool{}}
	op.retGIS = RelSetStorageState
	expCalled := []string{"GIS", "SSS", "DS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.NotEmpty(sr.StorageID)         // not cleared before return
	assert.True(op.rhs.DoNotSetStorageID) // will not update storageId
	assert.Nil(op.rhs.Pool)               // cleared before return

	op = &fakeReleaseOps{}
	op.ops = op
	op.rhs = &requestHandlerState{Request: sr}
	op.retGIS = RelDeleteStorage
	expCalled = []string{"GIS", "DS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakeReleaseOps{}
	op.ops = op
	op.rhs = &requestHandlerState{Request: sr}
	op.retGIS = RelDone
	expCalled = []string{"GIS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// ReleaseStorage called to undo successful ProvisionStorage
	op = &fakeReleaseOps{}
	op.ops = op
	op.rhs = &requestHandlerState{Request: sr}
	op.retGIS = RelSetStorageState
	op.wasInError = true
	expCalled = []string{"GIS", "SSS", "DS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)
	assert.True(op.rhs.InError)

	// call the real pool with an error
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
	rhs := &requestHandlerState{
		InError: true,
		Request: sr,
	}
	c.ReleaseStorage(nil, rhs)
}

type fakeReleaseOps struct {
	releaseOp
	called []string
	retGIS releaseSubState
}

func (op *fakeReleaseOps) getInitialState(ctx context.Context) releaseSubState {
	op.called = append(op.called, "GIS")
	return op.retGIS
}
func (op *fakeReleaseOps) setStorageState(ctx context.Context) {
	op.called = append(op.called, "SSS")
}
func (op *fakeReleaseOps) deleteStorage(ctx context.Context) {
	op.called = append(op.called, "DS")
}
