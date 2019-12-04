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

	mcp "github.com/Nuvoloso/kontroller/pkg/autogen/client/pool"
	mcs "github.com/Nuvoloso/kontroller/pkg/autogen/client/storage"
	mcsr "github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	appmock "github.com/Nuvoloso/kontroller/pkg/centrald/mock"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	mockcsp "github.com/Nuvoloso/kontroller/pkg/csp/mock"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestMakeProvisioningStorageUnprovisioned(t *testing.T) {
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

	// success case
	fc.InLsSObj = nil
	fc.RetLsSOk = &mcs.StorageListOK{
		Payload: []*models.Storage{
			&models.Storage{
				StorageAllOf0: models.StorageAllOf0{
					Meta: &models.ObjMeta{
						ID: "STORAGE-1",
					},
				},
				StorageMutable: models.StorageMutable{
					StorageState: &models.StorageStateMutable{
						ProvisionedState: "PROVISIONING",
					},
				},
			},
		},
	}
	fc.RetLsSRObj = &mcsr.StorageRequestListOK{
		Payload: []*models.StorageRequest{
			&models.StorageRequest{
				StorageRequestAllOf0: models.StorageRequestAllOf0{
					Meta: &models.ObjMeta{
						ID: "SR-1",
					},
				},
				StorageRequestCreateOnce: models.StorageRequestCreateOnce{
					PoolID:              "poolID",
					RequestedOperations: []string{"PROVISION", "ATTACH"},
					MinSizeBytes:        swag.Int64(1000),
				},
				StorageRequestMutable: models.StorageRequestMutable{
					StorageRequestCreateMutable: models.StorageRequestCreateMutable{
						StorageID: "STORAGE-1",
					},
				},
			},
		},
	}
	expLsParam := mcs.NewStorageListParams()
	expLsParam.ProvisionedState = swag.String(com.StgProvisionedStateProvisioning)
	expLsSR := mcsr.NewStorageRequestListParams()
	expLsSR.StorageID = swag.String("STORAGE-1")
	expLsSR.IsTerminated = swag.Bool(false)
	fc.RetDSErr = nil
	fc.InLsSRObj = nil
	fc.RetLsSRErr = nil
	fc.RetUSObj = fc.RetLsSOk.Payload[0]
	fc.RetUSErr = nil
	fc.InSRUpdaterItems = nil
	fc.RetSRUpdaterUpdateErr = nil
	assert.EqualValues("STORAGE-1", fc.RetLsSRObj.Payload[0].StorageID)
	err := c.makeProvisioningStorageUnprovisioned(ctx)
	assert.NoError(err)
	assert.Equal(expLsParam, fc.InLsSObj)
	assert.Equal(expLsSR, fc.InLsSRObj)
	assert.Equal("UNPROVISIONED", fc.RetLsSOk.Payload[0].StorageState.ProvisionedState)
	assert.EqualValues("", fc.RetLsSRObj.Payload[0].StorageID)
	fc.RetLsSRObj.Payload[0].StorageID = "STORAGE-1" // undo
	assert.Equal([]string{"requestMessages", "storageId"}, fc.InSRUpdaterItems.Set)

	// error while updating Storage
	fc.InLsSObj = nil
	fc.InLsSRObj = nil
	fc.RetUSObj = nil
	fc.RetUSErr = fmt.Errorf("storage update error")
	err = c.makeProvisioningStorageUnprovisioned(ctx)
	assert.Error(err)
	assert.Regexp("storage update error", err.Error())
	assert.Equal(expLsParam, fc.InLsSObj)
	assert.Equal(expLsSR, fc.InLsSRObj)
	assert.Equal(fc.RetUSErr, err)

	// error updating storage request, test not setting poolId
	fc.RetLsSRObj.Payload[0].PoolID = "PROV-1"
	fc.InLsSObj = nil
	fc.InLsSRObj = nil
	fc.InSRUpdaterItems = nil
	fc.RetSRUpdaterUpdateErr = fmt.Errorf("storage request update error")
	err = c.makeProvisioningStorageUnprovisioned(ctx)
	assert.Error(err)
	assert.Regexp("storage request update error", err.Error())
	assert.Equal(expLsParam, fc.InLsSObj)
	assert.Equal(expLsSR, fc.InLsSRObj)
	assert.Equal([]string{"requestMessages", "storageId"}, fc.InSRUpdaterItems.Set)

	// error in storage request list
	fc.InLsSObj = nil
	fc.InLsSRObj = nil
	fc.RetLsSRErr = fmt.Errorf("storage request list error")
	err = c.makeProvisioningStorageUnprovisioned(ctx)
	assert.Error(err)
	assert.Regexp("storage request list error", err.Error())
	assert.Equal(expLsParam, fc.InLsSObj)
	assert.Equal(expLsSR, fc.InLsSRObj)

	// error in storage listing API
	fc.RetLsSErr = fmt.Errorf("storage list error")
	err = c.makeProvisioningStorageUnprovisioned(ctx)
	assert.Error(err)
	assert.Regexp("storage list error", err.Error())
	assert.Equal(fc.RetLsSErr, err)
}

func TestDeleteUnprovisionedStorage(t *testing.T) {
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

	// success case
	fc.InLsSObj = nil
	fc.RetLsSOk = &mcs.StorageListOK{
		Payload: []*models.Storage{
			&models.Storage{
				StorageAllOf0: models.StorageAllOf0{
					Meta: &models.ObjMeta{
						ID: "STORAGE-1",
					},
				},
				StorageMutable: models.StorageMutable{
					StorageState: &models.StorageStateMutable{
						ProvisionedState: "UNPROVISIONED",
					},
				},
			},
		},
	}
	expLsParam := mcs.NewStorageListParams()
	expLsParam.ProvisionedState = swag.String(com.StgProvisionedStateUnprovisioned)
	fc.InDSObj = ""
	fc.RetDSErr = nil
	err := c.deleteUnprovisionedStorage(ctx)
	assert.NoError(err)
	assert.Equal(expLsParam, fc.InLsSObj)
	assert.EqualValues(fc.InDSObj, fc.RetLsSOk.Payload[0].Meta.ID)

	// error while deleting
	fc.InLsSObj = nil
	fc.InDSObj = ""
	fc.RetDSErr = fmt.Errorf("deletion error")
	err = c.deleteUnprovisionedStorage(ctx)
	assert.Error(err)
	assert.Equal(expLsParam, fc.InLsSObj)
	assert.Equal(fc.RetDSErr, err)
	assert.EqualValues(fc.InDSObj, fc.RetLsSOk.Payload[0].Meta.ID)

	// error in listing API
	fc.RetLsSErr = fmt.Errorf("listing error")
	fc.InDSObj = ""
	err = c.deleteUnprovisionedStorage(ctx)
	assert.Error(err)
	assert.Equal(fc.RetLsSErr, err)
	assert.Empty(fc.InDSObj)
}

func TestReleaseOrphanedCSPVolumes(t *testing.T) {
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

	dom := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID: "CSP-DOMAIN-1",
			},
			CspDomainType:       "AWS",
			CspDomainAttributes: map[string]models.ValueType{},
		},
		CSPDomainMutable: models.CSPDomainMutable{
			Name: "MyCSPDomain",
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
	sp := &models.Pool{
		PoolAllOf0: models.PoolAllOf0{
			Meta: &models.ObjMeta{
				ID: "Pool-1",
			},
		},
		PoolCreateOnce: models.PoolCreateOnce{
			CspDomainID:    "CSP-DOMAIN-1",
			CspStorageType: "Amazon gp2",
			ClusterID:      "CLUSTER-1",
		},
		PoolMutable: models.PoolMutable{
			PoolCreateMutable: models.PoolCreateMutable{},
		},
	}
	sr := &models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{
				ID: "SR-1",
			},
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
				StorageRequestState: "FORMATTING",
			},
		},
	}
	stg := &models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta: &models.ObjMeta{
				ID: "stg-1",
			},
		},
		StorageMutable: models.StorageMutable{
			StorageState: &models.StorageStateMutable{
				ProvisionedState: "PROVISIONING",
			},
		},
	}
	vols := []*csp.Volume{
		&csp.Volume{
			Identifier: "vol-1",
		},
		&csp.Volume{
			Identifier: "vol-2",
		},
	}

	// case: domain client failure
	tl.Logger().Info("case: domain client failure")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	appCSP := appmock.NewMockAppCloudServiceProvider(mockCtrl)
	appCSP.EXPECT().GetDomainClient(dom).Return(nil, fmt.Errorf("failed in domain client"))
	c.App.AppCSP = appCSP
	assert.NoError(c.releasePoolOrphanedCSPVolumes(ctx, sp, dom, cluster))
	assert.Equal(1, tl.CountPattern("CSPDomain.*client failure"))
	tl.Flush()

	// case: volume list failure
	tl.Logger().Info("case: CSP VolumeList failure")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC := mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(dom).Return(cspDC, nil)
	vla := &csp.VolumeListArgs{
		StorageTypeName: models.CspStorageType(sp.CspStorageType),
		Tags: []string{
			com.VolTagSystem + ":" + c.systemID,
			com.VolTagPoolID + ":" + string(sp.Meta.ID),
			com.VolTagStorageRequestID,
		},
		ProvisioningAttributes: cluster.ClusterAttributes,
	}
	cspDC.EXPECT().VolumeList(ctx, vla).Return(nil, fmt.Errorf("VolumeList error"))
	c.App.AppCSP = appCSP
	assert.NoError(c.releasePoolOrphanedCSPVolumes(ctx, sp, dom, cluster))
	assert.Equal(1, tl.CountPattern("Pool.*VolumeList error"))
	tl.Flush()

	// case: error in volume delete, cover no storageID tag in csp volume case
	tl.Logger().Info("case: CSP VolumeDelete failure")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(dom).Return(cspDC, nil)
	cspDC.EXPECT().VolumeList(ctx, vla).Return(vols, nil)
	vda0 := &csp.VolumeDeleteArgs{
		VolumeIdentifier:       vols[0].Identifier,
		ProvisioningAttributes: cluster.ClusterAttributes,
	}
	cspDC.EXPECT().VolumeDelete(ctx, vda0).Return(fmt.Errorf("VolumeDelete error"))
	vda1 := &csp.VolumeDeleteArgs{
		VolumeIdentifier:       vols[1].Identifier,
		ProvisioningAttributes: cluster.ClusterAttributes,
	}
	cspDC.EXPECT().VolumeDelete(ctx, vda1).Return(csp.ErrorVolumeNotFound) // ignored
	c.App.AppCSP = appCSP
	assert.NoError(c.releasePoolOrphanedCSPVolumes(ctx, sp, dom, cluster))
	assert.Equal(1, tl.CountPattern("Pool.*VolumeDelete error"))
	tl.Flush()

	// case: All Pools: error in listing Pool
	tl.Logger().Info("case: Pool list error")
	fc.RetPoolListErr = fmt.Errorf("Pool list error")
	fc.RetPoolListObj = nil
	fc.InPoolListObj = nil
	lParams := mcp.NewPoolListParams()
	err := c.releaseOrphanedCSPVolumes(ctx)
	assert.Error(err)
	assert.Regexp("Pool list error", err.Error())
	assert.Equal(lParams, fc.InPoolListObj)
	tl.Flush()

	// case: Error in loading CSPDomain
	tl.Logger().Info("case: CSPDomain fetch error")
	sps := []*models.Pool{
		&models.Pool{
			PoolAllOf0: models.PoolAllOf0{
				Meta: &models.ObjMeta{
					ID: "SPS-0",
				},
			},
			PoolCreateOnce: models.PoolCreateOnce{
				CspDomainID:    "CSP-DOMAIN-1",
				CspStorageType: "Amazon gp2",
			},
			PoolMutable: models.PoolMutable{
				PoolCreateMutable: models.PoolCreateMutable{},
			},
		},
		&models.Pool{
			PoolAllOf0: models.PoolAllOf0{
				Meta: &models.ObjMeta{
					ID: "SPS-1",
				},
			},
			PoolCreateOnce: models.PoolCreateOnce{
				CspDomainID:    "CSP-DOMAIN-1",
				CspStorageType: "Amazon gp2",
			},
			PoolMutable: models.PoolMutable{
				PoolCreateMutable: models.PoolCreateMutable{},
			},
		},
	}
	fc.RetPoolListErr = nil
	fc.RetPoolListObj = &mcp.PoolListOK{
		Payload: sps,
	}
	fc.RetLDErr = fmt.Errorf("CSPDomain fetch error")
	fc.RetLDObj = nil
	fc.InLDid = ""
	err = c.releaseOrphanedCSPVolumes(ctx)
	assert.Error(err)
	assert.Regexp("CSPDomain fetch error", err.Error())
	assert.Equal(lParams, fc.InPoolListObj)
	assert.EqualValues(dom.Meta.ID, fc.InLDid)
	tl.Flush()

	// case: Error in loading Cluster
	tl.Logger().Info("case: Cluster fetch error")
	fc.RetLDErr = nil
	fc.RetLDObj = dom
	fc.InLDid = ""
	fc.RetLClObj = nil
	fc.RetLClErr = fmt.Errorf("Cluster fetch error")
	err = c.releaseOrphanedCSPVolumes(ctx)
	assert.Error(err)
	assert.Regexp("Cluster fetch error", err.Error())
	assert.Equal(lParams, fc.InPoolListObj)
	assert.EqualValues(dom.Meta.ID, fc.InLDid)
	tl.Flush()

	// case: multi-sp release
	tl.Logger().Info("case: Multiple Pool CSP Volume release")
	fc.RetLDErr = nil
	fc.RetLDObj = dom
	fc.InLDid = ""
	fc.CntLD = 0
	fc.RetLClObj = cluster
	fc.RetLClErr = nil
	fc.CntLCl = 0
	fc.StorageFetchRetObj, fc.StorageFetchRetErr = stg, nil
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	appCSP = appmock.NewMockAppCloudServiceProvider(mockCtrl)
	cspDC = mockcsp.NewMockDomainClient(mockCtrl)
	appCSP.EXPECT().GetDomainClient(dom).Return(cspDC, nil).Times(2)
	vols[0].Tags = []string{com.VolTagStorageRequestID + ":SR-1", com.VolTagStorageID + ":stg-1"}
	vla0 := &csp.VolumeListArgs{
		StorageTypeName: models.CspStorageType(sp.CspStorageType),
		Tags: []string{
			com.VolTagSystem + ":" + c.systemID,
			com.VolTagPoolID + ":" + string(sps[0].Meta.ID),
			com.VolTagStorageRequestID,
		},
		ProvisioningAttributes: cluster.ClusterAttributes,
	}
	cspDC.EXPECT().VolumeList(ctx, vla0).Return([]*csp.Volume{vols[0]}, nil)
	vla1 := &csp.VolumeListArgs{
		StorageTypeName: models.CspStorageType(sp.CspStorageType),
		Tags: []string{
			com.VolTagSystem + ":" + c.systemID,
			com.VolTagPoolID + ":" + string(sps[1].Meta.ID),
			com.VolTagStorageRequestID,
		},
		ProvisioningAttributes: cluster.ClusterAttributes,
	}
	cspDC.EXPECT().VolumeList(ctx, vla1).Return([]*csp.Volume{vols[1]}, nil)
	vda0 = &csp.VolumeDeleteArgs{
		VolumeIdentifier:       vols[0].Identifier,
		ProvisioningAttributes: cluster.ClusterAttributes,
	}
	cspDC.EXPECT().VolumeDelete(ctx, vda0).Return(fmt.Errorf("VolumeDelete error"))
	vda1 = &csp.VolumeDeleteArgs{
		VolumeIdentifier:       vols[1].Identifier,
		ProvisioningAttributes: cluster.ClusterAttributes,
	}
	cspDC.EXPECT().VolumeDelete(ctx, vda1).Return(nil)
	c.App.AppCSP = appCSP
	err = c.releaseOrphanedCSPVolumes(ctx)
	assert.NoError(err)
	assert.Equal(lParams, fc.InPoolListObj)
	assert.EqualValues(dom.Meta.ID, fc.InLDid)
	assert.Equal("stg-1", fc.StorageFetchID)
	assert.Equal(1, fc.CntLD)  // domain cached
	assert.Equal(1, fc.CntLCl) // cluster cached
	assert.Equal(2, tl.CountPattern("Pool SPS-.: Found 1 "))
	tl.Flush()

	tl.Logger().Info("case: StorageFetch NotFound")
	appCSP.EXPECT().GetDomainClient(dom).Return(cspDC, nil)
	cspDC.EXPECT().VolumeList(ctx, vla0).Return([]*csp.Volume{vols[0]}, nil)
	fc.StorageFetchID, fc.StorageFetchRetObj, fc.StorageFetchRetErr = "", nil, fake.NotFoundErr
	cspDC.EXPECT().VolumeDelete(ctx, vda0).Return(nil)
	assert.NoError(c.releasePoolOrphanedCSPVolumes(ctx, sps[0], dom, cluster))
	assert.Equal(1, tl.CountPattern("Pool SPS-.: Found 1 "))
	tl.Flush()

	tl.Logger().Info("case: StorageFetch unknown error")
	fc.RetLDErr = nil
	fc.RetLDObj = dom
	fc.InLDid = ""
	fc.CntLD = 0
	fc.StorageFetchID, fc.StorageFetchRetObj, fc.StorageFetchRetErr = "", nil, fmt.Errorf("storageFetchErr")
	appCSP.EXPECT().GetDomainClient(dom).Return(cspDC, nil)
	cspDC.EXPECT().VolumeList(ctx, vla0).Return([]*csp.Volume{vols[0]}, nil)
	err = c.releaseOrphanedCSPVolumes(ctx)
	assert.Equal("stg-1", fc.StorageFetchID)
	assert.Regexp("storageFetchErr", err)
	tl.Flush()

	tl.Logger().Info("case: StorageFetch transient error")
	fc.RetLDErr = nil
	fc.RetLDObj = dom
	fc.InLDid = ""
	fc.CntLD = 0
	fc.StorageFetchID, fc.StorageFetchRetObj, fc.StorageFetchRetErr = "", nil, fake.TransientErr
	appCSP.EXPECT().GetDomainClient(dom).Return(cspDC, nil)
	cspDC.EXPECT().VolumeList(ctx, vla0).Return([]*csp.Volume{vols[0]}, nil)
	err = c.releaseOrphanedCSPVolumes(ctx)
	assert.Equal("stg-1", fc.StorageFetchID)
	assert.Regexp("transientErr", err)
	tl.Flush()

	tl.Logger().Info("case: storage PROVISIONED, SR in progress")
	stg.StorageState.ProvisionedState = "PROVISIONED"
	fc.RetLDErr = nil
	fc.RetLDObj = dom
	fc.InLDid = ""
	fc.CntLD = 0
	fc.StorageFetchID, fc.StorageFetchRetObj, fc.StorageFetchRetErr = "", stg, nil
	fc.RetSRFetchObj = sr
	appCSP.EXPECT().GetDomainClient(dom).Return(cspDC, nil).Times(2)
	cspDC.EXPECT().VolumeList(ctx, vla0).Return([]*csp.Volume{vols[0]}, nil)
	cspDC.EXPECT().VolumeList(ctx, vla1).Return([]*csp.Volume{}, nil) // cover empty pool
	assert.NoError(c.releaseOrphanedCSPVolumes(ctx))
	assert.Equal("stg-1", fc.StorageFetchID)
	assert.Equal("SR-1", fc.InSRFetchID)
	assert.Equal(2, tl.CountPattern("Pool SPS-.: Found 0 "))
	tl.Flush()

	tl.Logger().Info("case: storage PROVISIONED, SR not found")
	vta0 := &csp.VolumeTagArgs{
		VolumeIdentifier: vols[0].Identifier,
		Tags: []string{
			com.VolTagStorageRequestID + ":SR-1",
		},
		ProvisioningAttributes: cluster.ClusterAttributes,
	}
	appCSP.EXPECT().GetDomainClient(dom).Return(cspDC, nil)
	cspDC.EXPECT().VolumeList(ctx, vla0).Return([]*csp.Volume{vols[0]}, nil)
	fc.InSRFetchID, fc.RetSRFetchObj, fc.RetSRFetchErr = "", nil, fake.NotFoundErr
	cspDC.EXPECT().VolumeTagsDelete(ctx, vta0).Return(vols[0], nil)
	assert.NoError(c.releasePoolOrphanedCSPVolumes(ctx, sps[0], dom, cluster))
	assert.Equal("SR-1", fc.InSRFetchID)
	assert.Equal(1, tl.CountPattern("Pool SPS-.: Found 0 "))
	tl.Flush()

	tl.Logger().Info("case: storage PROVISIONED, SR succeeded, tag delete error ignored")
	sr.StorageRequestState = "SUCCEEDED"
	appCSP.EXPECT().GetDomainClient(dom).Return(cspDC, nil)
	cspDC.EXPECT().VolumeList(ctx, vla0).Return([]*csp.Volume{vols[0]}, nil)
	fc.InSRFetchID, fc.RetSRFetchObj, fc.RetSRFetchErr = "", sr, nil
	cspDC.EXPECT().VolumeTagsDelete(ctx, vta0).Return(nil, fmt.Errorf("vtdErr"))
	assert.NoError(c.releasePoolOrphanedCSPVolumes(ctx, sps[0], dom, cluster))
	assert.Equal("SR-1", fc.InSRFetchID)
	assert.Equal(1, tl.CountPattern("Pool SPS-.: Found 0 "))
	assert.Equal(1, tl.CountPattern("Ignoring VolumeTagsDelete"))
	tl.Flush()

	tl.Logger().Info("case: storage PROVISIONED, SR failed, tag delete error ignored")
	sr.StorageRequestState = "FAILED"
	appCSP.EXPECT().GetDomainClient(dom).Return(cspDC, nil)
	cspDC.EXPECT().VolumeList(ctx, vla0).Return([]*csp.Volume{vols[0]}, nil)
	fc.InSRFetchID, fc.RetSRFetchObj, fc.RetSRFetchErr = "", sr, nil
	cspDC.EXPECT().VolumeTagsDelete(ctx, vta0).Return(nil, fmt.Errorf("vtdErr"))
	assert.NoError(c.releasePoolOrphanedCSPVolumes(ctx, sps[0], dom, cluster))
	assert.Equal("SR-1", fc.InSRFetchID)
	assert.Equal(1, tl.CountPattern("Pool SPS-.: Found 0 "))
	assert.Equal(1, tl.CountPattern("Ignoring VolumeTagsDelete"))
	tl.Flush()

	tl.Logger().Info("case: storage PROVISIONED, SR unknown error")
	appCSP.EXPECT().GetDomainClient(dom).Return(cspDC, nil)
	cspDC.EXPECT().VolumeList(ctx, vla0).Return([]*csp.Volume{vols[0]}, nil)
	fc.InSRFetchID, fc.RetSRFetchObj, fc.RetSRFetchErr = "", nil, fmt.Errorf("srFetchErr")
	err = c.releasePoolOrphanedCSPVolumes(ctx, sps[0], dom, cluster)
	assert.Regexp("srFetchErr", err)
	assert.Equal("SR-1", fc.InSRFetchID)
	tl.Flush()

	tl.Logger().Info("case: storage PROVISIONED, SR transient error")
	appCSP.EXPECT().GetDomainClient(dom).Return(cspDC, nil)
	cspDC.EXPECT().VolumeList(ctx, vla0).Return([]*csp.Volume{vols[0]}, nil)
	fc.InSRFetchID, fc.RetSRFetchObj, fc.RetSRFetchErr = "", nil, fake.TransientErr
	err = c.releasePoolOrphanedCSPVolumes(ctx, sps[0], dom, cluster)
	assert.Regexp("transientErr", err)
	assert.Equal("SR-1", fc.InSRFetchID)
}
