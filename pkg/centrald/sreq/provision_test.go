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
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestProvisioningSteps(t *testing.T) {
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
	sr := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:      "SR-1",
				Version: 1,
			},
		},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			PoolID:              "PROV-1",
			CompleteByTime:      strfmt.DateTime(now.Add(time.Hour)),
			CspStorageType:      "Amazon gp2",
			RequestedOperations: []string{"PROVISION", "ATTACH"},
			MinSizeBytes:        swag.Int64(1000),
			ShareableStorage:    true,
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
				StorageRequestState: "PROVISIONING",
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
			AvailableBytes:   swag.Int64(1000),
			ShareableStorage: true,
			StorageState: &models.StorageStateMutable{
				AttachmentState:  "DETACHED",
				ProvisionedState: "PROVISIONING",
			},
		},
	}
	// the provisionOp
	op := &provisionOp{
		c: c,
		rhs: &requestHandlerState{
			c:         c,
			Request:   sr,
			CSPDomain: dom,
			Cluster:   cluster,
		},
	}
	actualSizeBytes := int64(1024)

	//  ***************************** getInitialState
	op.rhs.TimedOut = true
	assert.Equal(ProvTimedOut, op.getInitialState(ctx))
	assert.False(op.idsWerePresent)
	op.rhs.TimedOut = false

	op.rhs.InError = true
	assert.Equal(ProvDone, op.getInitialState(ctx))
	assert.False(op.idsWerePresent)
	op.rhs.InError = false

	op.rhs.Storage = s
	op.rhs.Pool = sp
	assert.Zero(op.sizeBytes)
	assert.Equal(ProvSetIds, op.getInitialState(ctx))
	assert.NotZero(op.sizeBytes)
	assert.False(op.idsWerePresent)

	op.rhs.Request.PoolID = s.PoolID
	op.rhs.Request.StorageID = models.ObjIDMutable(s.Meta.ID)
	assert.Equal(ProvSetStorageProvisioned, op.getInitialState(ctx))
	assert.True(op.idsWerePresent)

	op.idsWerePresent = false // reset
	op.sizeBytes = 0          // reset
	op.rhs.Request.StorageRequestState = "REMOVING_TAG"
	assert.Equal(ProvRemoveTag, op.getInitialState(ctx))
	assert.False(op.idsWerePresent)
	assert.Zero(op.sizeBytes)
	op.rhs.Request.StorageRequestState = "PROVISIONING" // reset

	s.StorageState.ProvisionedState = "PROVISIONED"
	assert.Equal(ProvDone, op.getInitialState(ctx))
	assert.True(op.idsWerePresent)
	assert.NotZero(op.sizeBytes)
	s.StorageState.ProvisionedState = "PROVISIONING" // reset
	op.rhs.Storage = nil                             // reset
	op.rhs.Pool = nil                                // reset
	op.rhs.Request.PoolID = ""                       // reset
	op.rhs.Request.StorageID = ""                    // reset
	op.idsWerePresent = false                        // reset
	op.sizeBytes = 0                                 // reset

	assert.Equal(ProvGetCapacity, op.getInitialState(ctx))
	assert.False(op.idsWerePresent)

	//  ***************************** getCapacity
	assert.Nil(op.pool)
	assert.Zero(op.sizeBytes)
	// failure in sp get domain client
	tl.Logger().Info("Case: getCapacity: fail in GetDomainClient")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	appCSP := appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC := mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(nil, fmt.Errorf("failed in domain client"))
	op.c.App.AppCSP = appCSP
	fc.InSRUpdaterID = ""
	op.getCapacity(nil)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Nil(op.pool)
	assert.Zero(op.sizeBytes)
	assert.Regexp("Domain client failure", sr.RequestMessages[0].Message)
	assert.EqualValues(sr.Meta.ID, fc.InSRUpdaterID)
	tl.Flush()
	mockCtrl.Finish()
	// failure in volume size
	tl.Logger().Info("Case: getCapacity: fail in VolumeSize")
	op.sizeBytes = 0                                   // reset
	op.rhs.RetryLater = false                          // reset
	op.rhs.InError = false                             // reset
	sr.RequestMessages = []*models.TimestampedString{} // reset
	mockCtrl = gomock.NewController(t)
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(cspDC, nil)
	cspDC.EXPECT().VolumeSize(nil, models.CspStorageType(sr.CspStorageType), swag.Int64Value(sr.MinSizeBytes)).Return(int64(0), fmt.Errorf("failed in volume size"))
	op.c.App.AppCSP = appCSP
	op.getCapacity(nil)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Nil(op.pool)
	assert.Zero(op.sizeBytes)
	assert.Regexp("failed in volume size", sr.RequestMessages[0].Message)
	tl.Flush()
	mockCtrl.Finish()
	// failure in pool fetch
	tl.Logger().Info("Case: fail to fetch pool object")
	fc.InPoolFetchID = ""
	fc.RetPoolFetchObj = nil
	fc.RetPoolFetchErr = fmt.Errorf("pool-fetch-error")
	op.sizeBytes = 0                                   // reset
	op.rhs.RetryLater = false                          // reset
	op.rhs.InError = false                             // reset
	sr.RequestMessages = []*models.TimestampedString{} // reset
	sr.StorageRequestState = "PROVISIONING"            // reset
	mockCtrl = gomock.NewController(t)
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(cspDC, nil)
	cspDC.EXPECT().VolumeSize(nil, models.CspStorageType(sr.CspStorageType), swag.Int64Value(sr.MinSizeBytes)).Return(actualSizeBytes, nil)
	op.c.App.AppCSP = appCSP
	op.getCapacity(nil)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.EqualValues(op.rhs.Request.PoolID, fc.InPoolFetchID)
	assert.Nil(op.pool)
	assert.Len(sr.RequestMessages, 1)
	assert.Regexp("pool-fetch-error", sr.RequestMessages[0].Message)
	tl.Flush()
	// success
	tl.Logger().Info("Case: getCapacity: success")
	op.sizeBytes = 0                                   // reset
	op.rhs.RetryLater = false                          // reset
	op.rhs.InError = false                             // reset
	sr.RequestMessages = []*models.TimestampedString{} // reset
	sr.StorageRequestState = "PROVISIONING"            // reset
	mockCtrl = gomock.NewController(t)
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(cspDC, nil)
	cspDC.EXPECT().VolumeSize(nil, models.CspStorageType(sr.CspStorageType), swag.Int64Value(sr.MinSizeBytes)).Return(actualSizeBytes, nil)
	op.c.App.AppCSP = appCSP
	fc.InPoolFetchID = ""
	fc.RetPoolFetchObj = sp
	fc.RetPoolFetchErr = nil
	op.getCapacity(nil)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(sp, op.pool)
	assert.Equal(actualSizeBytes, op.sizeBytes)
	assert.Len(sr.RequestMessages, 1)
	assert.Regexp("Provisioned 1024 bytes .*PROV-1", sr.RequestMessages[0].Message)
	tl.Flush()

	//  ***************************** createStorage
	// failure
	op.rhs.RetryLater = false                          // reset
	op.rhs.InError = false                             // reset
	sr.RequestMessages = []*models.TimestampedString{} // reset
	assert.Empty(op.rhs.Storage)
	fc.RetCSObj = nil
	fc.RetCSErr = fmt.Errorf("create storage error")
	op.createStorage(ctx)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Empty(op.rhs.Storage)
	assert.Regexp("Failed to create Storage object", sr.RequestMessages[0].Message)
	// inject fail error
	c.rei.SetProperty("provision-fail-storage-create", &rei.Property{BoolValue: true})
	op.rhs.RetryLater = false                          // reset
	op.rhs.InError = false                             // reset
	sr.RequestMessages = []*models.TimestampedString{} // reset
	op.createStorage(ctx)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Empty(op.rhs.Storage)
	assert.Regexp("Failed to create Storage object.*provision-fail-storage-create", sr.RequestMessages[0].Message)
	// inject blocking error
	c.rei.SetProperty("provision-block-storage-create", &rei.Property{BoolValue: true})
	op.rhs.RetryLater = false                          // reset
	op.rhs.InError = false                             // reset
	sr.RequestMessages = []*models.TimestampedString{} // reset
	op.createStorage(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Empty(op.rhs.Storage)
	assert.Regexp("CreateStorage:.*provision-block-storage-create", sr.RequestMessages[0].Message)
	// success
	op.rhs.RetryLater = false                          // reset
	op.rhs.InError = false                             // reset
	sr.RequestMessages = []*models.TimestampedString{} // reset
	fc.RetCSObj = s
	fc.RetCSErr = nil
	fc.InCSobj = nil
	op.createStorage(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(s, op.rhs.Storage)
	assert.NotNil(fc.InCSobj)
	expS := &models.Storage{}
	expS.SizeBytes = swag.Int64(op.sizeBytes)
	expS.CspStorageType = s.CspStorageType
	expS.PoolID = s.PoolID
	expS.AvailableBytes = swag.Int64(op.sizeBytes)
	expS.ShareableStorage = s.ShareableStorage
	expS.StorageState = s.StorageState
	assert.Equal(expS, fc.InCSobj)

	// ***************************** setIds
	srCopy := &models.StorageRequest{}
	testutils.Clone(sr, srCopy)
	op.rhs.Request = srCopy
	assert.Empty(op.rhs.Request.PoolID)
	assert.Empty(op.rhs.Request.StorageID)
	assert.NotNil(op.rhs.Storage)
	assert.NotEmpty(op.rhs.Storage.PoolID)
	assert.Nil(op.rhs.Pool)
	// failure case
	fc.InSRUpdaterItems = nil
	fc.RetSRUpdaterObj = nil
	fc.RetSRUpdaterUpdateErr = fmt.Errorf("update storage request error")
	op.setIds(ctx)
	assert.True(op.rhs.InError)
	assert.Equal([]string{"storageRequestState", "requestMessages", "storageId"}, fc.InSRUpdaterItems.Set)
	// success
	op.rhs.RetryLater = false // reset
	op.rhs.InError = false    // reset
	op.rhs.Request = srCopy   // reset
	fc.InSRUpdaterItems = nil
	fc.RetSRUpdaterUpdateErr = nil
	fc.ModSRUpdaterObj = nil
	op.setIds(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Nil(op.pool) // cleared
	assert.Equal([]string{"storageRequestState", "requestMessages", "storageId"}, fc.InSRUpdaterItems.Set)
	assert.Equal(srCopy, op.rhs.Request) // updated obj in po
	assert.Equal(srCopy, fc.ModSRUpdaterObj)
	op.rhs.Request = sr // put the original obj back

	// ***************************** createCSPVolumeSetStorageProvisioned (!idsWerePresent)
	op.idsWerePresent = false
	assert.Nil(op.pool)        // cleared
	assert.NotNil(op.rhs.Pool) // set
	// test GetDomainClient failure
	op.rhs.RetryLater = false                          // reset
	op.rhs.InError = false                             // reset
	sr.RequestMessages = []*models.TimestampedString{} // reset
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(nil, fmt.Errorf("get domain client error"))
	assert.False(op.rhs.RetryLater)
	op.c.App.AppCSP = appCSP
	fc.InSRUpdaterID = ""
	op.createCSPVolumeSetStorageProvisioned(ctx)
	assert.True(op.rhs.RetryLater)
	assert.Regexp("Domain client failure", sr.RequestMessages[0].Message)
	assert.EqualValues(sr.Meta.ID, fc.InSRUpdaterID)
	// volume create failure
	op.rhs.RetryLater = false                          // reset
	op.rhs.InError = false                             // reset
	sr.RequestMessages = []*models.TimestampedString{} // reset
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(cspDC, nil)
	vca := &csp.VolumeCreateArgs{
		StorageTypeName: models.CspStorageType(op.rhs.Request.CspStorageType),
		SizeBytes:       op.sizeBytes,
		Tags: []string{
			com.VolTagSystem + ":" + op.c.systemID,
			com.VolTagStorageID + ":" + string(op.rhs.Storage.Meta.ID),
			com.VolTagPoolID + ":" + string(op.rhs.Pool.Meta.ID),
			com.VolTagStorageRequestID + ":" + string(op.rhs.Request.Meta.ID),
		},
		ProvisioningAttributes: op.rhs.Cluster.ClusterAttributes,
	}
	cspDC.EXPECT().VolumeCreate(ctx, vca).Return(nil, fmt.Errorf("volume create error"))
	op.c.App.AppCSP = appCSP
	fc.InSRUpdaterID = ""
	op.createCSPVolumeSetStorageProvisioned(ctx)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Regexp("^Error: Failed to create CSP volume", sr.RequestMessages[0].Message)
	assert.Empty(fc.InSRUpdaterID)
	// inject error
	c.rei.SetProperty("provision-fail-csp-volume-create", &rei.Property{BoolValue: true})
	op.rhs.RetryLater = false                          // reset
	op.rhs.InError = false                             // reset
	sr.RequestMessages = []*models.TimestampedString{} // reset
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(cspDC, nil)
	op.c.App.AppCSP = appCSP
	fc.InSRUpdaterID = ""
	op.createCSPVolumeSetStorageProvisioned(ctx)
	assert.True(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Regexp("^Error: Failed to create CSP volume.*provision-fail-csp-volume-create", sr.RequestMessages[0].Message)
	assert.Empty(fc.InSRUpdaterID)
	// storage object update failure
	op.rhs.RetryLater = false                          // reset
	op.rhs.InError = false                             // reset
	sr.RequestMessages = []*models.TimestampedString{} // reset
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(cspDC, nil)
	vol := &csp.Volume{
		CSPDomainType: dom.CspDomainType,
		Identifier:    "vol-1",
	}
	cspDC.EXPECT().VolumeCreate(ctx, vca).Return(vol, nil)
	fc.RetUSObj = nil
	fc.RetUSErr = fmt.Errorf("storage update error")
	op.c.App.AppCSP = appCSP
	op.createCSPVolumeSetStorageProvisioned(ctx)
	assert.True(op.rhs.InError)
	assert.Regexp("Created volume", sr.RequestMessages[0].Message)
	assert.Regexp("Failed to update Storage state", sr.RequestMessages[1].Message)
	// success case
	op.rhs.RetryLater = false                          // reset
	op.rhs.InError = false                             // reset
	sr.RequestMessages = []*models.TimestampedString{} // reset
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(cspDC, nil)
	cspDC.EXPECT().VolumeCreate(ctx, vca).Return(vol, nil)
	newS := &models.Storage{}
	*newS = *s // shallow copy
	newS.Meta = &models.ObjMeta{}
	*newS.Meta = *s.Meta // shallow copy
	newS.Meta.Version++
	fc.RetUSObj = newS
	fc.RetUSErr = nil
	op.c.App.AppCSP = appCSP
	op.createCSPVolumeSetStorageProvisioned(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(newS, op.rhs.Storage) // updated object in po
	assert.Equal(newS.Meta.Version, op.rhs.Storage.Meta.Version)
	op.rhs.Storage = s // put the original object back
	assert.Equal("PROVISIONED", s.StorageState.ProvisionedState)
	assert.Equal(vol.Identifier, s.StorageIdentifier)
	assert.Regexp("ProvisionedState change: PROVISIONING.*PROVISIONED", s.StorageState.Messages[0].Message)

	// ***************************** createCSPVolumeSetStorageProvisioned (idsWerePresent)
	assert.Nil(op.pool)        // cleared
	assert.NotNil(op.rhs.Pool) // set
	op.idsWerePresent = true
	op.rhs.Storage = s
	op.rhs.Request.PoolID = s.PoolID
	op.rhs.Request.StorageID = models.ObjIDMutable(s.Meta.ID)
	// volume list finds a provisioned volume
	op.rhs.RetryLater = false                               // reset
	op.rhs.InError = false                                  // reset
	sr.RequestMessages = []*models.TimestampedString{}      // reset
	s.StorageIdentifier = ""                                // reset
	s.StorageState.ProvisionedState = "PROVISIONING"        // reset
	s.StorageState.Messages = []*models.TimestampedString{} // reset
	vla := &csp.VolumeListArgs{
		StorageTypeName:        models.CspStorageType(s.CspStorageType),
		Tags:                   vca.Tags,
		ProvisioningAttributes: op.rhs.Cluster.ClusterAttributes,
	}
	vol.ProvisioningState = csp.VolumeProvisioningProvisioned
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(cspDC, nil)
	cspDC.EXPECT().VolumeList(ctx, vla).Return([]*csp.Volume{vol}, nil)
	fc.RetUSObj = newS
	fc.RetUSErr = nil
	op.c.App.AppCSP = appCSP
	op.createCSPVolumeSetStorageProvisioned(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(newS, op.rhs.Storage) // updated object in po
	assert.Equal(newS.Meta.Version, op.rhs.Storage.Meta.Version)
	op.rhs.Storage = s // put the original object back
	assert.Equal("PROVISIONED", s.StorageState.ProvisionedState)
	assert.Equal(vol.Identifier, s.StorageIdentifier)
	assert.Regexp("ProvisionedState change: PROVISIONING.*PROVISIONED", s.StorageState.Messages[0].Message)
	assert.Regexp("Created volume", sr.RequestMessages[0].Message)
	// volume list finds no provisioned volume - create called
	vol.ProvisioningState = csp.VolumeProvisioningProvisioning // reset
	sr.RequestMessages = []*models.TimestampedString{}         // reset
	s.StorageIdentifier = ""                                   // reset
	s.StorageState.ProvisionedState = "PROVISIONING"           // reset
	s.StorageState.Messages = []*models.TimestampedString{}    // reset
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(cspDC, nil)
	cspDC.EXPECT().VolumeList(ctx, vla).Return([]*csp.Volume{vol}, nil)
	cspDC.EXPECT().VolumeCreate(ctx, vca).Return(vol, nil)
	fc.RetUSObj = newS
	fc.RetUSErr = nil
	op.c.App.AppCSP = appCSP
	op.createCSPVolumeSetStorageProvisioned(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(newS, op.rhs.Storage) // updated object in po
	assert.Equal(newS.Meta.Version, op.rhs.Storage.Meta.Version)
	op.rhs.Storage = s // put the original object back
	assert.Equal("PROVISIONED", s.StorageState.ProvisionedState)
	assert.Equal(vol.Identifier, s.StorageIdentifier)
	assert.Regexp("ProvisionedState change: PROVISIONING.*PROVISIONED", s.StorageState.Messages[0].Message)
	// volume list fails - create called
	vol.ProvisioningState = csp.VolumeProvisioningProvisioning // reset
	sr.RequestMessages = []*models.TimestampedString{}         // reset
	s.StorageIdentifier = ""                                   // reset
	s.StorageState.ProvisionedState = "PROVISIONING"           // reset
	s.StorageState.Messages = []*models.TimestampedString{}    // reset
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(cspDC, nil)
	cspDC.EXPECT().VolumeList(ctx, vla).Return(nil, fmt.Errorf("volume list error"))
	cspDC.EXPECT().VolumeCreate(ctx, vca).Return(vol, nil)
	fc.RetUSObj = newS
	fc.RetUSErr = nil
	op.c.App.AppCSP = appCSP
	op.createCSPVolumeSetStorageProvisioned(ctx)
	assert.False(op.rhs.InError)
	assert.False(op.rhs.RetryLater)
	assert.Equal(newS, op.rhs.Storage) // updated object in po
	assert.Equal(newS.Meta.Version, op.rhs.Storage.Meta.Version)
	op.rhs.Storage = s // put the original object back
	assert.Equal("PROVISIONED", s.StorageState.ProvisionedState)
	assert.Equal(vol.Identifier, s.StorageIdentifier)
	assert.Regexp("ProvisionedState change: PROVISIONING.*PROVISIONED", s.StorageState.Messages[0].Message)

	// ***************************** removeCSPVolumeTag

	// GetDomainClient failure, RetryLater
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(nil, fmt.Errorf("get domain client error"))
	fc.InSRUpdaterID = ""
	sr.RequestMessages = []*models.TimestampedString{} // reset
	op.rhs.InError = false
	op.rhs.RetryLater = false
	op.c.App.AppCSP = appCSP
	op.removeCSPVolumeTag(ctx)
	assert.False(op.rhs.InError)
	assert.True(op.rhs.RetryLater)
	assert.Regexp("Domain client failure", sr.RequestMessages[0].Message)
	assert.EqualValues(sr.Meta.ID, fc.InSRUpdaterID)

	// CSP error ignored
	mockCtrl.Finish()
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	vta := &csp.VolumeTagArgs{
		VolumeIdentifier: vol.Identifier,
		Tags: []string{
			com.VolTagStorageRequestID + ":" + string(op.rhs.Request.Meta.ID),
		},
	}
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(cspDC, nil)
	cspDC.EXPECT().VolumeTagsDelete(ctx, vta).Return(nil, fmt.Errorf("vta error"))
	sr.RequestMessages = []*models.TimestampedString{} // reset
	op.c.App.AppCSP = appCSP
	op.removeCSPVolumeTag(ctx)
	assert.Empty(sr.RequestMessages)
	assert.Equal(1, tl.CountPattern("Ignoring VolumeTagsDelete error: vta error"))

	mockCtrl.Finish()
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(op.rhs.CSPDomain).Return(cspDC, nil)
	cspDC.EXPECT().VolumeTagsDelete(ctx, vta).Return(vol, nil)
	op.c.App.AppCSP = appCSP
	op.removeCSPVolumeTag(ctx)

	// done with mock
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)

	//  ***************************** timedOutCleanup
	// no errors
	op.rhs.Storage = s
	op.sizeBytes = 0
	op.rhs.Request.PoolID = s.PoolID
	fc.RetDSErr = nil
	op.timedOutCleanup(ctx)
	assert.Nil(op.rhs.Storage)
	assert.Zero(op.sizeBytes)
	// fail to delete storage
	op.rhs.Storage = s
	op.sizeBytes = 0
	op.rhs.Request.PoolID = s.PoolID
	fc.RetDSErr = fmt.Errorf("delete storage error")
	op.timedOutCleanup(ctx)
	assert.Equal(s, op.rhs.Storage)
	assert.Zero(op.sizeBytes)
}

func TestProvisionStorage(t *testing.T) {
	assert := assert.New(t)

	op := &fakeProvisionOps{}
	op.ops = op
	op.rhs = &requestHandlerState{}
	op.retGIS = ProvGetCapacity
	expCalled := []string{"GIS", "GC", "CS", "SI", "VSS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakeProvisionOps{}
	op.ops = op
	op.rhs = &requestHandlerState{}
	op.retGIS = ProvTimedOut
	expCalled = []string{"GIS", "TOC"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakeProvisionOps{}
	op.ops = op
	op.rhs = &requestHandlerState{}
	op.retGIS = ProvDone
	expCalled = []string{"GIS"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// call the real handler with an error

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
	}
	c.ProvisionStorage(nil, rhs)
}

func TestRemoveTag(t *testing.T) {
	assert := assert.New(t)

	op := &fakeProvisionOps{}
	op.ops = op
	op.rhs = &requestHandlerState{}
	op.retGIS = ProvRemoveTag
	expCalled := []string{"GIS", "RT"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	op = &fakeProvisionOps{}
	op.ops = op
	op.rhs = &requestHandlerState{}
	op.retGIS = ProvTimedOut
	expCalled = []string{"GIS", "TOC"}
	op.run(nil)
	assert.Equal(expCalled, op.called)

	// call the real handler with an error

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
	}
	c.RemoveTag(nil, rhs)
}

type fakeProvisionOps struct {
	provisionOp
	called []string
	retGIS provSubState
}

func (op *fakeProvisionOps) getInitialState(ctx context.Context) provSubState {
	op.called = append(op.called, "GIS")
	return op.retGIS
}

func (op *fakeProvisionOps) getCapacity(ctx context.Context) {
	op.called = append(op.called, "GC")
}

func (op *fakeProvisionOps) createStorage(ctx context.Context) {
	op.called = append(op.called, "CS")
}

func (op *fakeProvisionOps) setIds(ctx context.Context) {
	op.called = append(op.called, "SI")
}

func (op *fakeProvisionOps) createCSPVolumeSetStorageProvisioned(ctx context.Context) {
	op.called = append(op.called, "VSS")
}

func (op *fakeProvisionOps) removeCSPVolumeTag(ctx context.Context) {
	op.called = append(op.called, "RT")
}

func (op *fakeProvisionOps) timedOutCleanup(ctx context.Context) {
	op.called = append(op.called, "TOC")
}
