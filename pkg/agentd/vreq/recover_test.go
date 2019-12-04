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

	"github.com/Nuvoloso/kontroller/pkg/agentd"
	fa "github.com/Nuvoloso/kontroller/pkg/agentd/fake"
	fakeState "github.com/Nuvoloso/kontroller/pkg/agentd/state/fake"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	fev "github.com/Nuvoloso/kontroller/pkg/crude/fake"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	mockNuvo "github.com/Nuvoloso/kontroller/pkg/nuvoapi/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestRecoverVolumesInUse(t *testing.T) {
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
	fao := &fa.AppObjects{}
	fao.Node = &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "THIS-NODE",
			},
			NodeIdentifier: "this-node-identifier",
		},
	}
	app.AppObjects = fao

	c := newComponent()
	c.Init(app)
	thisNodeID := models.ObjIDMutable(fao.Node.Meta.ID)
	notThisNodeID := models.ObjIDMutable("OTHER-NODE")
	c.thisNodeID = thisNodeID
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc
	ctx := context.Background()

	vsObj1 := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: "VS-1",
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				VolumeSeriesState:    "IN_USE",
				NuvoVolumeIdentifier: "nuvo-vol-1",
				Mounts: []*models.Mount{
					&models.Mount{
						MountedNodeID:  thisNodeID,
						SnapIdentifier: "HEAD",
						PitIdentifier:  "",
					},
					&models.Mount{
						MountedNodeID:     thisNodeID,
						SnapIdentifier:    "snap-1",
						PitIdentifier:     "pit-1",
						MountedNodeDevice: "dev-1",
					},
					&models.Mount{
						MountedNodeID:  notThisNodeID,
						SnapIdentifier: "snap-n2",
					},
				},
			},
		},
	}
	vsObj2 := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: "VS-2",
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				VolumeSeriesState:    "IN_USE",
				NuvoVolumeIdentifier: "nuvo-vol-2",
				Mounts: []*models.Mount{
					&models.Mount{
						MountedNodeID:  notThisNodeID,
						SnapIdentifier: "HEAD",
						PitIdentifier:  "",
					},
					&models.Mount{
						MountedNodeID:  notThisNodeID,
						SnapIdentifier: "snap-1",
					},
					&models.Mount{
						MountedNodeID:     thisNodeID,
						SnapIdentifier:    "snap-2",
						PitIdentifier:     "pit-2",
						MountedNodeDevice: "dev-2",
					},
				},
			},
		},
	}
	vsObj3 := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: "VS-3",
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				VolumeSeriesState:    "IN_USE",
				NuvoVolumeIdentifier: "nuvo-vol-3",
				Mounts: []*models.Mount{
					&models.Mount{
						MountedNodeID:     thisNodeID,
						SnapIdentifier:    "snap-3",
						PitIdentifier:     "pit-3",
						MountedNodeDevice: "dev-3",
					},
				},
			},
		},
	}
	vsSkip := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: "VS-SKIP",
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				VolumeSeriesState:    "CONFIGURED",
				NuvoVolumeIdentifier: "nuvo-vol-skip",
			},
		},
	}

	var err error

	tl.Logger().Info("case: local HEAD added, remote HEAD skipped, nuvo error")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	nvAPI := mockNuvo.NewMockNuvoVM(mockCtrl)
	c.App.NuvoAPI = nvAPI
	nvAPI.EXPECT().UnexportLun(vsObj1.NuvoVolumeIdentifier, "pit-1", "dev-1").Return(fmt.Errorf("unexport-error"))
	fc.InLsVObj = nil
	lsVParams := volume_series.NewVolumeSeriesListParams()
	lsVParams.ConfiguredNodeID = swag.String(string(thisNodeID))
	fc.RetLsVOk = &volume_series.VolumeSeriesListOK{
		Payload: []*models.VolumeSeries{
			vsClone(vsObj1),
		},
	}
	fc.RetLsVErr = nil
	lunList, err := c.RecoverVolumesInUse(ctx)
	assert.Error(err)
	assert.Regexp("unexport-error", err)
	assert.Nil(lunList)
	assert.Equal(lsVParams, fc.InLsVObj)
	mockCtrl.Finish()

	tl.Logger().Info("case: local HEAD added, remote HEAD skipped, updater error")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.App.NuvoAPI = nvAPI
	nvAPI.EXPECT().UnexportLun(vsObj1.NuvoVolumeIdentifier, "pit-1", "dev-1").Return(nil)
	fc.RetLsVOk = &volume_series.VolumeSeriesListOK{
		Payload: []*models.VolumeSeries{
			vsClone(vsObj1),
		},
	}
	fc.RetLsVErr = nil
	fc.InVSUpdaterID = ""
	fc.InVSUpdaterItems = nil
	fc.RetVSUpdaterErr = fmt.Errorf("updater-error")
	assert.Len(c.App.GetLUNs(), 0)
	lunList, err = c.RecoverVolumesInUse(ctx)
	assert.Error(err)
	assert.Regexp("updater-error", err)
	assert.Nil(lunList)
	assert.Equal(string(vsObj1.Meta.ID), fc.InVSUpdaterID)
	assert.Equal([]string{"messages", "mounts", "volumeSeriesState", "configuredNodeId"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Append)
	assert.Nil(fc.InVSUpdaterItems.Remove)
	mockCtrl.Finish()

	tl.Logger().Info("case: local HEAD added")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.App.NuvoAPI = nvAPI
	nvAPI.EXPECT().UnexportLun(vsObj1.NuvoVolumeIdentifier, "pit-1", "dev-1").Return(nil)
	fc.RetLsVOk = &volume_series.VolumeSeriesListOK{
		Payload: []*models.VolumeSeries{
			vsClone(vsObj1),
		},
	}
	fc.RetLsVErr = nil
	fc.InVSUpdaterID = ""
	fc.InVSUpdaterItems = nil
	fc.RetVSUpdaterErr = nil
	fc.FetchVSUpdaterObj = fc.RetLsVOk.Payload[0]
	lunList, err = c.RecoverVolumesInUse(ctx)
	assert.NoError(err)
	assert.Len(lunList, 1)
	assert.EqualValues(vsObj1.Meta.ID, lunList[0].VolumeSeriesID)
	assert.EqualValues("HEAD", lunList[0].SnapIdentifier)
	assert.Equal("IN_USE", fc.ModVSUpdaterObj2.VolumeSeriesState)
	mockCtrl.Finish()

	tl.Logger().Info("case: remote HEAD skipped")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.App.NuvoAPI = nvAPI
	nvAPI.EXPECT().UnexportLun(vsObj2.NuvoVolumeIdentifier, "pit-2", "dev-2").Return(nil)
	fc.RetLsVOk = &volume_series.VolumeSeriesListOK{
		Payload: []*models.VolumeSeries{
			vsClone(vsObj2),
		},
	}
	fc.RetLsVErr = nil
	fc.InVSUpdaterID = ""
	fc.InVSUpdaterItems = nil
	fc.RetVSUpdaterErr = nil
	fc.FetchVSUpdaterObj = fc.RetLsVOk.Payload[0]
	lunList, err = c.RecoverVolumesInUse(ctx)
	assert.NoError(err)
	assert.Len(lunList, 0)
	assert.Equal("IN_USE", fc.ModVSUpdaterObj2.VolumeSeriesState)
	mockCtrl.Finish()

	tl.Logger().Info("case: local SNAP LUNs only - transition to CONFIGURED")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.App.NuvoAPI = nvAPI
	nvAPI.EXPECT().UnexportLun(vsObj3.NuvoVolumeIdentifier, "pit-3", "dev-3").Return(nil)
	fc.RetLsVOk = &volume_series.VolumeSeriesListOK{
		Payload: []*models.VolumeSeries{
			vsClone(vsObj3),
		},
	}
	fc.RetLsVErr = nil
	fc.InVSUpdaterID = ""
	fc.InVSUpdaterItems = nil
	fc.RetVSUpdaterErr = nil
	fc.FetchVSUpdaterObj = fc.RetLsVOk.Payload[0]
	lunList, err = c.RecoverVolumesInUse(ctx)
	assert.NoError(err)
	assert.Len(lunList, 0)
	assert.Equal("CONFIGURED", fc.ModVSUpdaterObj2.VolumeSeriesState)
	assert.Len(fc.ModVSUpdaterObj2.Messages, 1)
	assert.Regexp("IN_USE ⇒ CONFIGURED", fc.ModVSUpdaterObj2.Messages[0].Message)
	tl.Flush()

	tl.Logger().Info("case: Skip CONFIGURED")
	tl.Flush()
	fc.RetLsVOk = &volume_series.VolumeSeriesListOK{
		Payload: []*models.VolumeSeries{
			vsClone(vsSkip),
		},
	}
	fc.RetLsVErr = nil
	fc.InVSUpdaterID = ""
	fc.InVSUpdaterItems = nil
	fc.RetVSUpdaterErr = nil
	fc.FetchVSUpdaterObj = fc.RetLsVOk.Payload[0]
	lunList, err = c.RecoverVolumesInUse(ctx)
	assert.NoError(err)
	assert.Len(lunList, 0)

	tl.Logger().Info("case: list error")
	tl.Flush()
	fc.RetLsVOk = nil
	fc.RetLsVErr = fmt.Errorf("vs-list-error")
	lunList, err = c.RecoverVolumesInUse(ctx)
	assert.Error(err)
	assert.Regexp("vs-list-error", err)
	assert.Nil(lunList)
}

func TestCheckNuvoVolumes(t *testing.T) {
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
	fao := &fa.AppObjects{}
	app.AppObjects = fao
	fso := fakeState.NewFakeNodeState()
	app.StateOps = fso

	c := newComponent()
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc
	ctx := context.Background()
	c.thisNodeID = models.ObjIDMutable("someID")

	// NuvoApi List Vols returns volumes
	tl.Logger().Info("case: NuvoApi List Vols returns volumes")
	tl.Flush()
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	nvAPI := mockNuvo.NewMockNuvoVM(mockCtrl)
	c.App.NuvoAPI = nvAPI
	nvAPI.EXPECT().ListVols().Return([]string{"vol1"}, nil)
	err := c.CheckNuvoVolumes(ctx)
	assert.Nil(err)
	assert.Equal(1, tl.CountPattern("NUVOAPI ListVols.* called"))
	assert.Equal(1, tl.CountPattern("NUVOAPI ListVols.*:"))
	mockCtrl.Finish()

	// Nuvo ListVols returns empty list, NoVolumesInUse called, no error
	tl.Logger().Info("case: NoVolumesInUse called, no error")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.App.NuvoAPI = nvAPI
	nvAPI.EXPECT().ListVols().Return([]string{}, nil)
	fc.RetLsVOk = &volume_series.VolumeSeriesListOK{Payload: []*models.VolumeSeries{}}
	fc.RetLsVErr = nil
	err = c.CheckNuvoVolumes(ctx)
	assert.Nil(err)
	assert.Equal(1, tl.CountPattern("NUVOAPI ListVols.* called"))
	assert.Equal(1, tl.CountPattern("NUVOAPI ListVols.*:"))
	mockCtrl.Finish()

	// Nuvo ListVols error, NoVolumesInUse called, returns Error
	tl.Logger().Info("case: NoVolumesInUse called, no error")
	tl.Flush()
	mockCtrl = gomock.NewController(t)
	nvAPI = mockNuvo.NewMockNuvoVM(mockCtrl)
	c.App.NuvoAPI = nvAPI
	nvAPI.EXPECT().ListVols().Return(nil, fmt.Errorf("ignored error"))
	fc.RetLsVOk = nil
	fc.RetLsVErr = fmt.Errorf("vs-list-error")
	err = c.CheckNuvoVolumes(ctx)
	assert.NotNil(err)
	assert.Equal(1, tl.CountPattern("NUVOAPI ListVols.* called"))
	assert.Equal(1, tl.CountPattern("NUVOAPI ListVols.*: ignored error"))
	assert.Equal("vs-list-error", err.Error())
}

func TestNoVolumesInUse(t *testing.T) {
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
	fao := &fa.AppObjects{}
	app.AppObjects = fao
	fso := fakeState.NewFakeNodeState()
	app.StateOps = fso

	c := newComponent()
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc
	ctx := context.Background()

	vsUsedCacheSB := int64(1100)
	nodeTotalCacheBytes := int64(3000)
	nObj := &models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID: "THIS-NODE",
			},
			NodeIdentifier: "this-node-identifier",
		},
		NodeMutable: models.NodeMutable{
			Service: &models.NuvoService{
				ServiceState: models.ServiceState{
					State: "RUNNING",
				},
			},
			Name:        "THIS-NODE-NAME",
			Description: "node1 object",
			LocalStorage: map[string]models.NodeStorageDevice{
				"uuid1": models.NodeStorageDevice{
					DeviceName:      "d1",
					DeviceState:     "UNUSED",
					DeviceType:      "SSD",
					SizeBytes:       swag.Int64(1100),
					UsableSizeBytes: swag.Int64(1000),
				},
				"uuid2": models.NodeStorageDevice{
					DeviceName:      "d2",
					DeviceState:     "CACHE",
					DeviceType:      "SSD",
					SizeBytes:       swag.Int64(2200),
					UsableSizeBytes: swag.Int64(2000),
				},
			},
			AvailableCacheBytes: swag.Int64(nodeTotalCacheBytes - vsUsedCacheSB),
			TotalCacheBytes:     swag.Int64(nodeTotalCacheBytes),
		},
	}
	fao.Node = nObj
	thisNodeID := models.ObjIDMutable(fao.Node.Meta.ID)
	notThisNodeID := models.ObjIDMutable("OTHER-NODE")
	c.thisNodeID = thisNodeID

	vsObj := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID: "VS-1",
			},
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				VolumeSeriesState: "IN_USE",
				ConfiguredNodeID:  thisNodeID,
				StorageParcels: map[string]models.ParcelAllocation{
					"STG1": models.ParcelAllocation{},
					"STG2": models.ParcelAllocation{},
				},
				Mounts: []*models.Mount{
					&models.Mount{
						MountedNodeID:  thisNodeID,
						SnapIdentifier: "HEAD",
					},
					&models.Mount{
						MountedNodeID:  thisNodeID,
						SnapIdentifier: "snap-1",
					},
				},
				CacheAllocations: map[string]models.CacheAllocation{
					"THIS-NODE": {AllocatedSizeBytes: swag.Int64(vsUsedCacheSB)},
				},
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				SystemTags: []string{
					fmt.Sprintf("%s:/mnt", com.SystemTagVolumeFsAttached),
					fmt.Sprintf("%s:Something", com.SystemTagVolumeHeadStatSeries),
					fmt.Sprintf("%s:1", com.SystemTagVolumeHeadStatCount),
					"something:else",
				},
			},
		},
	}
	var vs *models.VolumeSeries
	var err error

	tl.Logger().Info("case: local mounts only")
	tl.Flush()
	vs = vsClone(vsObj)
	fc.InLsVObj = nil
	lsVParams := volume_series.NewVolumeSeriesListParams()
	lsVParams.ConfiguredNodeID = swag.String(string(thisNodeID))
	fc.RetLsVOk = &volume_series.VolumeSeriesListOK{Payload: []*models.VolumeSeries{vs}}
	fc.RetLsVErr = nil
	fc.FetchVSUpdaterObj = vs
	fc.InVSUpdaterID = ""
	fc.InVSUpdaterItems = nil
	stgIDs, err := c.NoVolumesInUse(ctx)
	assert.NoError(err)
	assert.NotNil(stgIDs)
	assert.Contains(stgIDs, "STG1")
	assert.Contains(stgIDs, "STG2")
	assert.Equal(lsVParams, fc.InLsVObj)
	assert.Equal(string(vs.Meta.ID), fc.InVSUpdaterID)
	assert.Equal([]string{"messages", "mounts", "volumeSeriesState", "configuredNodeId", "lifecycleManagementData", "systemTags"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Append)
	assert.Equal([]string{"cacheAllocations"}, fc.InVSUpdaterItems.Remove)
	assert.Equal(vs, fc.ModVSUpdaterObj2)
	assert.Len(vs.CacheAllocations, 1)
	assert.Nil(vs.CacheAllocations["THIS-NODE"].AllocatedSizeBytes)
	assert.Len(vs.Mounts, 0)
	assert.Len(vs.Messages, 3)
	assert.Regexp("crashed", vs.Messages[0].Message)
	assert.Regexp("crashed", vs.Messages[1].Message)
	assert.Regexp("IN_USE ⇒ PROVISIONED", vs.Messages[2].Message)
	assert.Empty(vs.ConfiguredNodeID)
	assert.NotNil(vs.LifecycleManagementData)
	assert.True(vs.LifecycleManagementData.FinalSnapshotNeeded)
	sTag := util.NewTagList(vs.SystemTags)
	_, found := sTag.Get(com.SystemTagVolumeLastHeadUnexport)
	assert.True(found)
	for _, tag := range []string{com.SystemTagVolumeHeadStatSeries, com.SystemTagVolumeHeadStatCount, com.SystemTagVolumeFsAttached} {
		_, found := sTag.Get(tag)
		assert.False(found)
	}

	tl.Logger().Info("case: local + remote mounts")
	tl.Flush()
	vs = vsClone(vsObj)
	vs.Mounts = append(vs.Mounts, &models.Mount{
		MountedNodeID:  notThisNodeID,
		SnapIdentifier: "snap-2",
	})
	assert.Len(vs.Mounts, 3)
	fc.RetLsVOk = &volume_series.VolumeSeriesListOK{Payload: []*models.VolumeSeries{vs}}
	fc.RetLsVErr = nil
	fc.FetchVSUpdaterObj = vs
	fc.InVSUpdaterID = ""
	fc.InVSUpdaterItems = nil
	stgIDs, err = c.NoVolumesInUse(ctx)
	assert.NoError(err)
	assert.Equal(string(vs.Meta.ID), fc.InVSUpdaterID)
	assert.Equal([]string{"messages", "mounts", "volumeSeriesState", "configuredNodeId", "lifecycleManagementData", "systemTags"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Append)
	assert.Equal([]string{"cacheAllocations"}, fc.InVSUpdaterItems.Remove)
	assert.Equal(vs, fc.ModVSUpdaterObj2)
	assert.Len(vs.CacheAllocations, 1)
	assert.Nil(vs.CacheAllocations["THIS-NODE"].AllocatedSizeBytes)
	assert.Len(vs.Mounts, 1)
	for i, m := range vs.Mounts {
		assert.NotEqual(c.thisNodeID, m.MountedNodeID, "mount[%d]", i)
	}
	assert.Len(vs.Messages, 2)
	assert.Regexp("crashed", vs.Messages[0].Message)
	assert.Regexp("crashed", vs.Messages[1].Message)
	assert.NotEmpty(vs.ConfiguredNodeID)
	assert.NotNil(vs.LifecycleManagementData)
	assert.True(vs.LifecycleManagementData.FinalSnapshotNeeded)
	sTag = util.NewTagList(vs.SystemTags)
	_, found = sTag.Get(com.SystemTagVolumeLastHeadUnexport)
	assert.True(found)

	tl.Logger().Info("case: remote mounts only")
	tl.Flush()
	vs = vsClone(vsObj)
	vs.Mounts = []*models.Mount{
		&models.Mount{
			MountedNodeID:  notThisNodeID,
			SnapIdentifier: "HEAD",
		},
		&models.Mount{
			MountedNodeID:  notThisNodeID,
			SnapIdentifier: "snap-1",
		},
	}
	vs.ConfiguredNodeID = notThisNodeID
	assert.Len(vs.Mounts, 2)
	fc.RetLsVOk = &volume_series.VolumeSeriesListOK{Payload: []*models.VolumeSeries{vs}}
	fc.FetchVSUpdaterObj = vs
	fc.InVSUpdaterID = ""
	fc.InVSUpdaterItems = nil
	stgIDs, err = c.NoVolumesInUse(ctx)
	assert.NoError(err)
	assert.Equal(string(vs.Meta.ID), fc.InVSUpdaterID)
	assert.Equal([]string{"messages", "mounts", "volumeSeriesState", "configuredNodeId", "lifecycleManagementData", "systemTags"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Append)
	assert.Nil(fc.InVSUpdaterItems.Remove)
	assert.Len(vs.Mounts, 2)
	for i, m := range vs.Mounts {
		assert.NotEqual(c.thisNodeID, m.MountedNodeID, "mount[%d]", i)
	}
	assert.Len(vs.Messages, 0)
	assert.NotEmpty(vs.ConfiguredNodeID)
	sTag = util.NewTagList(vs.SystemTags)
	_, found = sTag.Get(com.SystemTagVolumeLastHeadUnexport)
	assert.False(found)

	tl.Logger().Info("case: volume CONFIGURED")
	tl.Flush()
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = "CONFIGURED"
	vs.Mounts = nil
	vs.ConfiguredNodeID = thisNodeID
	fc.RetLsVOk = &volume_series.VolumeSeriesListOK{Payload: []*models.VolumeSeries{vs}}
	fc.FetchVSUpdaterObj = vs
	fc.InVSUpdaterID = ""
	fc.InVSUpdaterItems = nil
	stgIDs, err = c.NoVolumesInUse(ctx)
	assert.NoError(err)
	assert.Equal(string(vs.Meta.ID), fc.InVSUpdaterID)
	assert.Equal([]string{"messages", "mounts", "volumeSeriesState", "configuredNodeId", "lifecycleManagementData", "systemTags"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Append)
	assert.Equal([]string{"cacheAllocations"}, fc.InVSUpdaterItems.Remove)
	assert.Len(vs.CacheAllocations, 1)
	assert.Nil(vs.CacheAllocations["THIS-NODE"].AllocatedSizeBytes)
	assert.Len(vs.Mounts, 0)
	assert.Len(vs.Messages, 1)
	assert.Regexp("CONFIGURED ⇒ PROVISIONED", vs.Messages[0].Message)
	assert.Empty(vs.ConfiguredNodeID)
	sTag = util.NewTagList(vs.SystemTags)
	_, found = sTag.Get(com.SystemTagVolumeLastHeadUnexport)
	assert.False(found)

	tl.Logger().Info("case: volume CONFIGURED, failure to update node cache usage")
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = "CONFIGURED"
	vs.Mounts = nil
	vs.ConfiguredNodeID = thisNodeID
	fc.FetchVSUpdaterObj = vs
	fc.InVSUpdaterID = ""
	fc.InVSUpdaterItems = nil
	fso.RetUpdateNodeAvailableCacheErr = fmt.Errorf("Failure to update Node")
	stgIDs, err = c.NoVolumesInUse(ctx)
	assert.Error(err)
	assert.Equal(string(vs.Meta.ID), fc.InVSUpdaterID)
	assert.Equal([]string{"messages", "mounts", "volumeSeriesState", "configuredNodeId", "lifecycleManagementData", "systemTags"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Append)
	assert.Equal([]string{"cacheAllocations"}, fc.InVSUpdaterItems.Remove)
	assert.Len(vs.CacheAllocations, 1)
	assert.Nil(vs.CacheAllocations["THIS-NODE"].AllocatedSizeBytes)
	assert.Len(vs.Mounts, 0)
	assert.Len(vs.Messages, 0)
	assert.Empty(vs.ConfiguredNodeID)

	tl.Logger().Info("case: volume PROVISIONED")
	tl.Flush()
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = "PROVISIONED"
	vs.Mounts = nil
	vs.ConfiguredNodeID = ""
	fc.RetLsVOk = &volume_series.VolumeSeriesListOK{Payload: []*models.VolumeSeries{vs}}
	fc.FetchVSUpdaterObj = vs
	fc.InVSUpdaterID = ""
	fc.InVSUpdaterItems = nil
	stgIDs, err = c.NoVolumesInUse(ctx)
	assert.NoError(err)
	assert.Equal(string(vs.Meta.ID), fc.InVSUpdaterID)
	assert.Equal([]string{"messages", "mounts", "volumeSeriesState", "configuredNodeId", "lifecycleManagementData", "systemTags"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Append)
	assert.Nil(fc.InVSUpdaterItems.Remove)
	assert.Len(vs.Mounts, 0)
	assert.Len(vs.Messages, 0)
	assert.Empty(vs.ConfiguredNodeID)

	tl.Logger().Info("case: update error")
	tl.Flush()
	vs = vsClone(vsObj)
	vs.VolumeSeriesState = "PROVISIONED"
	vs.Mounts = nil
	vs.ConfiguredNodeID = ""
	fc.RetLsVOk = &volume_series.VolumeSeriesListOK{Payload: []*models.VolumeSeries{vs}}
	fc.FetchVSUpdaterObj = vs
	fc.InVSUpdaterID = ""
	fc.InVSUpdaterItems = nil
	fc.RetVSUpdaterErr = fmt.Errorf("updater-error")
	stgIDs, err = c.NoVolumesInUse(ctx)
	assert.Error(err)
	assert.Regexp("updater-error", err)
	assert.Equal(string(vs.Meta.ID), fc.InVSUpdaterID)
	assert.Equal([]string{"messages", "mounts", "volumeSeriesState", "configuredNodeId", "lifecycleManagementData", "systemTags"}, fc.InVSUpdaterItems.Set)
	assert.Nil(fc.InVSUpdaterItems.Append)
	assert.Nil(fc.InVSUpdaterItems.Remove)
	assert.Empty(vs.ConfiguredNodeID)

	tl.Logger().Info("case: list error")
	tl.Flush()
	vs = vsClone(vsObj)
	fc.RetLsVOk = nil
	fc.RetLsVErr = fmt.Errorf("vs-list-error")
	stgIDs, err = c.NoVolumesInUse(ctx)
	assert.Error(err)
	assert.Regexp("vs-list-error", err)
}

func TestVolumesRecovered(t *testing.T) {
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
	fao := &fa.AppObjects{}
	app.AppObjects = fao

	c := newComponent()
	c.Init(app)
	fc := &fake.Client{}
	c.oCrud = fc
	c.Animator.OCrud = fc

	c.VolumesRecovered()
	assert.Equal(1, c.Animator.NotifyCount)
}
