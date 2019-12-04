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


package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/cluster"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/consistency_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/node"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/alecthomas/units"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestVolumeSeriesMakeRecord(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	cmd := &volumeSeriesCmd{}
	cmd.accounts = map[string]ctxIDName{
		"accountID": {"", "System"},
	}
	cmd.cspDomains = map[string]string{
		"cspDomainID": "MyAWS",
	}
	cmd.clusters = map[string]ctxIDName{
		"clusterID": ctxIDName{id: "cspDomainID", name: "MyCluster"},
	}
	cmd.consistencyGroups = map[string]ctxIDName{
		"cgID": ctxIDName{id: "accountID", name: "CG"},
	}
	cmd.nodes = map[string]ctxIDName{
		"nodeID": ctxIDName{id: "clusterID", name: "node1"},
	}
	cmd.servicePlans = map[string]string{
		"planID": "plan B",
	}
	o := &models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID:           "id",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
			RootParcelUUID: "uuid",
		},
		VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
			AccountID: "accountID",
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				BoundClusterID:   "clusterID",
				ConfiguredNodeID: "nodeID",
				CapacityAllocations: map[string]models.CapacityAllocation{
					"poolID": {ConsumedBytes: swag.Int64(1073741824), ReservedBytes: swag.Int64(107374182400)},
				},
				Mounts: []*models.Mount{
					{
						SnapIdentifier:    "HEAD",
						MountedNodeID:     "nodeID",
						MountedNodeDevice: "/dev/a",
						MountMode:         "READ_WRITE",
						MountState:        "MOUNTED",
						MountTime:         strfmt.DateTime(now),
					},
				},
				RootStorageID: "storageID",
				StorageParcels: map[string]models.ParcelAllocation{
					"storageID": {SizeBytes: swag.Int64(1073741824)},
				},
				VolumeSeriesState: "IN_USE",
				CacheAllocations: map[string]models.CacheAllocation{
					"nodeId1": {
						AllocatedSizeBytes: swag.Int64(1),
						RequestedSizeBytes: swag.Int64(2),
					},
					"nodeId2": {
						AllocatedSizeBytes: swag.Int64(22),
						RequestedSizeBytes: swag.Int64(22),
					},
					"nodeId3": {
						AllocatedSizeBytes: swag.Int64(3072),
						RequestedSizeBytes: swag.Int64(4096),
					},
				},
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				Description:        "desc",
				Name:               "vol1",
				ConsistencyGroupID: "cgID",
				ServicePlanID:      "planID",
				SizeBytes:          swag.Int64(100 * int64(units.GiB)),
				SpaAdditionalBytes: swag.Int64(int64(units.GiB)),
				Tags:               models.ObjTags{"tag1", "tag2", "tag3"},
				ClusterDescriptor:  models.ClusterDescriptor{com.ClusterDescriptorPVName: models.ValueType{Kind: "STRING", Value: "pvname"}},
			},
		},
	}
	rec := cmd.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(volumeSeriesHeaders))
	assert.Equal("uuid", rec[hRootParcelUUID])
	assert.Equal("System", rec[hAccount])
	assert.Equal("MyCluster", rec[hBoundCluster])
	assert.Equal("node1:/dev/a", rec[hMounts])
	// assert.Equal("TBD", rec[hCapacityAllocations])
	// assert.Equal("TBD", rec[hStorageParcels])
	assert.Equal("storageID", rec[hRootStorageID])
	assert.Equal("id", rec[hID])
	assert.Equal("vol1", rec[hName])
	assert.Equal("desc", rec[hDescription])
	assert.Equal("IN_USE", rec[hState])
	assert.Equal("tag1, tag2, tag3", rec[hTags])
	assert.Equal("CG", rec[hConsistencyGroup])
	assert.Equal("plan B", rec[hServicePlan])
	assert.Equal("100GiB*", rec[hSizeBytes])
	assert.Equal("101GiB*", rec[hAdjustedSizeBytes])
	assert.Equal("node1", rec[hHeadNode])
	assert.Equal("node1", rec[hConfiguredNode])
	ca := "nodeId1: 1B/2B\nnodeId2: 22B/22B\nnodeId3: 3KiB/4KiB"
	assert.Equal(ca, rec[hCacheAllocations])
	assert.Equal("pvname", rec[hPersistentVolumeName])

	// single node cache case
	o.CacheAllocations = map[string]models.CacheAllocation{
		"nodeId1": {
			AllocatedSizeBytes: swag.Int64(1),
			RequestedSizeBytes: swag.Int64(2),
		},
	}
	rec = cmd.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(volumeSeriesHeaders))
	assert.Equal("1B/2B", rec[hCacheAllocations])

	// cache routines are singletons
	assert.NoError(cmd.loadDCNCaches())
	assert.NoError(cmd.cacheConsistencyGroups())

	// repeat without the maps, different snap, mode and state, no spaAdditionalSize
	cmd.accounts = map[string]ctxIDName{}
	cmd.cspDomains = map[string]string{}
	cmd.clusters = map[string]ctxIDName{}
	cmd.consistencyGroups = map[string]ctxIDName{}
	cmd.nodes = map[string]ctxIDName{}
	cmd.servicePlans = map[string]string{}
	o.Mounts[0].MountMode = com.VolMountModeRO
	o.Mounts[0].MountState = com.VolMountStateMounting
	o.Mounts[0].SnapIdentifier = "1"
	o.SpaAdditionalBytes = swag.Int64(0)
	rec = cmd.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(volumeSeriesHeaders))
	assert.Equal("accountID", rec[hAccount])
	assert.Equal("cgID", rec[hConsistencyGroup])
	assert.Equal("clusterID", rec[hBoundCluster])
	assert.Equal("1:nodeID:/dev/a[ro](mounting)", rec[hMounts])
	assert.Equal("planID", rec[hServicePlan])
	assert.Equal("nodeID", rec[hConfiguredNode])
	assert.Equal("100GiB", rec[hSizeBytes])
	assert.Equal("100GiB", rec[hAdjustedSizeBytes])

	// unmounting state
	o.Mounts[0].MountMode = com.VolMountModeRW
	o.Mounts[0].MountState = com.VolMountStateUnmounting
	o.Mounts[0].SnapIdentifier = com.VolMountHeadIdentifier
	rec = cmd.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(volumeSeriesHeaders))
	assert.Equal("nodeID:/dev/a(unmounting)", rec[hMounts])

	// error state
	o.Mounts[0].MountMode = com.VolMountModeRW
	o.Mounts[0].MountState = com.VolMountStateError
	o.Mounts[0].SnapIdentifier = com.VolMountHeadIdentifier
	rec = cmd.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(volumeSeriesHeaders))
	assert.Equal("nodeID:/dev/a(error)", rec[hMounts])
}

func TestVolumeSeriesGet(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	reqToFetch := "VS-1"
	fParams := volume_series.NewVolumeSeriesFetchParams()
	fParams.ID = reqToFetch

	now := time.Now()
	fRet := &volume_series.VolumeSeriesFetchOK{
		Payload: &models.VolumeSeries{
			VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
				Meta: &models.ObjMeta{
					ID:           "VS-1",
					Version:      1,
					TimeCreated:  strfmt.DateTime(now),
					TimeModified: strfmt.DateTime(now),
				},
				RootParcelUUID: "uuid",
			},
			VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
				AccountID: "System",
			},
			VolumeSeriesMutable: models.VolumeSeriesMutable{
				VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
					VolumeSeriesState: "UNBOUND",
				},
				VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
					Name:               "MyVS-1",
					Description:        "new volume series",
					ConsistencyGroupID: "c-g-1",
					ServicePlanID:      "planID",
					SizeBytes:          swag.Int64(1073741824),
					Tags:               []string{"tag1:value1"},
					ClusterDescriptor: map[string]models.ValueType{
						"cdKey": models.ValueType{Kind: "STRING", Value: "Key value\n"},
					},
				},
			},
		},
	}

	outFile := "./vs-descriptor"
	defer func() { os.Remove(outFile) }()

	// fetch
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes, loadPools, loadAccounts, loadCGs, loadPlans)
	cOps := mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(cOps).MinTimes(1)
	mF := mockmgmtclient.NewVolumeSeriesMatcher(t, fParams)
	cOps.EXPECT().VolumeSeriesFetch(mF).Return(fRet, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err := parseAndRun([]string{"volume-series", "get", reqToFetch})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(volumeSeriesDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, 1)

	// emit cluster descriptor key
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(cOps).MinTimes(1)
	mF = mockmgmtclient.NewVolumeSeriesMatcher(t, fParams)
	cOps.EXPECT().VolumeSeriesFetch(mF).Return(fRet, nil)
	appCtx.API = mAPI
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume-series", "get", "--id", reqToFetch, "-K", "cdKey"})
	assert.Nil(err)
	resBytes, err := ioutil.ReadFile(outFile)
	assert.Nil(err)
	assert.Equal(fRet.Payload.ClusterDescriptor["cdKey"].Value, string(resBytes))

	// fail on invalid cluster descriptor key, use stdout
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(cOps).MinTimes(1)
	mF = mockmgmtclient.NewVolumeSeriesMatcher(t, fParams)
	cOps.EXPECT().VolumeSeriesFetch(mF).Return(fRet, nil)
	appCtx.API = mAPI
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume-series", "get", "--id", reqToFetch, "-K", "fooKey", "-O", "-"})
	assert.NotNil(err)
	assert.Regexp("fooKey", err)

	// fail on bad output file
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume-series", "get", "--id", reqToFetch, "-K", "cdKey", "-O", "./bad/outputFile"})
	assert.NotNil(err)
	assert.Regexp("no such file or directory", err)

	// apiError
	apiErr := &volume_series.VolumeSeriesFetchDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(cOps).MinTimes(1)
	mF = mockmgmtclient.NewVolumeSeriesMatcher(t, fParams)
	cOps.EXPECT().VolumeSeriesFetch(mF).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume-series", "get", "--id", reqToFetch})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	mockCtrl.Finish()
	t.Log("init context failure")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "System").Return("", fmt.Errorf("ctx error"))
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume-series", "get", "-A", "System", "--id", reqToFetch})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// invalid columns
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume-series", "get", "--id", reqToFetch, "--columns", "foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume-series", "get", "--id", reqToFetch, "-c", "foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	t.Log("expected id")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume-series", "get"})
	assert.NotNil(err)
	assert.Regexp("expected --id flag or a single identifier argument", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestVolumeSeriesList(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	res := &volume_series.VolumeSeriesListOK{
		Payload: []*models.VolumeSeries{
			&models.VolumeSeries{
				VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
					RootParcelUUID: "uuid1",
				},
				VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
					AccountID: "accountID",
				},
				VolumeSeriesMutable: models.VolumeSeriesMutable{
					VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
						BoundClusterID:   "clusterID",
						ConfiguredNodeID: "nodeID-1",
						CapacityAllocations: map[string]models.CapacityAllocation{
							"poolID": {ConsumedBytes: swag.Int64(1073741824), ReservedBytes: swag.Int64(107374182400)},
						},
						Mounts: []*models.Mount{
							{
								SnapIdentifier:    "HEAD",
								MountedNodeID:     "nodeID-1",
								MountedNodeDevice: "/dev/a",
								MountMode:         "READ_WRITE",
								MountState:        "MOUNTED",
								MountTime:         strfmt.DateTime(now),
							},
						},
						RootStorageID: "storageID",
						StorageParcels: map[string]models.ParcelAllocation{
							"storageID": {SizeBytes: swag.Int64(1073741824)},
						},
						VolumeSeriesState: "IN_USE",
					},
					VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
						Description:        "desc",
						Name:               "vol1",
						ConsistencyGroupID: "c-g-1",
						ServicePlanID:      "plan-1",
						SizeBytes:          swag.Int64(107374182400),
						Tags:               models.ObjTags{"tag1", "tag2", "tag3"},
					},
				},
			},
			&models.VolumeSeries{
				VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id2",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
					RootParcelUUID: "uuid2",
				},
				VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
					AccountID: "accountID",
				},
				VolumeSeriesMutable: models.VolumeSeriesMutable{
					VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
						BoundClusterID:   "clusterID",
						ConfiguredNodeID: "nodeID-1",
						CapacityAllocations: map[string]models.CapacityAllocation{
							"poolID": {ConsumedBytes: swag.Int64(1073741824), ReservedBytes: swag.Int64(107374182400)},
						},
						Mounts: []*models.Mount{
							{
								SnapIdentifier:    "HEAD",
								MountedNodeID:     "nodeID-1",
								MountedNodeDevice: "/dev/b",
								MountMode:         "READ_WRITE",
								MountState:        "MOUNTED",
								MountTime:         strfmt.DateTime(now),
							},
						},
						RootStorageID: "storageID",
						StorageParcels: map[string]models.ParcelAllocation{
							"storageID": {SizeBytes: swag.Int64(1073741824)},
						},
						VolumeSeriesState: "IN_USE",
					},
					VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
						Description:        "desc2",
						Name:               "vol2",
						ConsistencyGroupID: "c-g-1",
						ServicePlanID:      "plan-1",
						SizeBytes:          swag.Int64(107374182400),
						Tags:               models.ObjTags{"tag1", "tag2", "tag3"},
					},
				},
			},
		},
	}

	t.Log("case: list, all defaults")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes, loadPools, loadAccounts, loadCGs, loadPlans)
	params := volume_series.NewVolumeSeriesListParams()
	m := mockmgmtclient.NewVolumeSeriesMatcher(t, params)
	cOps := mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(cOps)
	cOps.EXPECT().VolumeSeriesList(m).Return(res, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err := parseAndRun([]string{"volumes", "list"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(volumeSeriesDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: list, json, most options")
	mAPI = cacheLoadHelper(t, mockCtrl, loadContext, loadDomains, loadClusters, loadNodes, loadPools, loadCGs, loadPlans)
	params = volume_series.NewVolumeSeriesListParams()
	params.AccountID = swag.String("accountID")
	params.NamePattern = swag.String("vol")
	params.Tags = []string{"tag2"}
	params.BoundCspDomainID = swag.String("CSP-DOMAIN-1")
	params.BoundClusterID = swag.String("clusterID")
	params.MountedNodeID = swag.String("nodeID-1")
	params.ConfiguredNodeID = swag.String("nodeID-1")
	params.StorageID = swag.String("storageID")
	params.PoolID = swag.String("poolID")
	params.ServicePlanID = swag.String("plan-1")
	params.ConsistencyGroupID = swag.String("c-g-1")
	params.VolumeSeriesState = []string{"CONFIGURED", "BOUND"}
	params.VolumeSeriesStateNot = []string{"UNBOUND"}
	m = mockmgmtclient.NewVolumeSeriesMatcher(t, params)
	cOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(cOps)
	cOps.EXPECT().VolumeSeriesList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume-series", "list", "-D", "domainName", "-C", "clusterName",
		"-N", "nodeName", "--pool-id", "poolID", "-A", "System", "--owned-only", "--service-plan", "plan B",
		"-n", "vol", "--storage-id", "storageID", "--consistency-group", "CG", "-t", "tag2",
		"--state=CONFIGURED", "--state=BOUND", "--state=!UNBOUND", "--configured-node-name", "nodeName",
		"-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(res.Payload, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: list, yaml")
	mAPI = cacheLoadHelper(t, mockCtrl, loadContext, loadDomains, loadClusters, loadNodes)
	params = volume_series.NewVolumeSeriesListParams()
	m = mockmgmtclient.NewVolumeSeriesMatcher(t, params)
	cOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(cOps)
	cOps.EXPECT().VolumeSeriesList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volumes", "list", "-A", "System", "-o", "yaml"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(res.Payload, te.yamlData)
	appCtx.Account, appCtx.AccountID = "", ""

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: list with follow")
	fw := &fakeWatcher{t: t}
	appCtx.watcher = fw
	fw.CallCBCount = 1
	fos := &fakeOSExecutor{}
	appCtx.osExec = fos
	volumeSeriesListCmdRunCacheThreshold = 0 // force a cache refresh
	defer func() { volumeSeriesListCmdRunCacheThreshold = 1 }()
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes, loadPools, loadAccounts, loadCGs, loadPlans)
	params = volume_series.NewVolumeSeriesListParams()
	m = mockmgmtclient.NewVolumeSeriesMatcher(t, params)
	cOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(cOps)
	cOps.EXPECT().VolumeSeriesList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volumes", "list", "-f"})
	assert.Nil(err)
	assert.Len(te.tableHeaders, len(volumeSeriesDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))
	expWArgs := &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{
				URIPattern: "/volume-series/?",
			},
		},
	}
	assert.NotNil(fw.InArgs)
	assert.Equal(expWArgs, fw.InArgs)
	assert.NotNil(fw.InCB)
	assert.Equal(1, fw.NumCalls)
	assert.NoError(fw.CBErr)

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: list with columns")
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes, loadPools, loadAccounts, loadCGs, loadPlans)
	params = volume_series.NewVolumeSeriesListParams()
	m = mockmgmtclient.NewVolumeSeriesMatcher(t, params)
	cOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(cOps)
	cOps.EXPECT().VolumeSeriesList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volumes", "list", "--columns", "ID,RootStorage"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, 2)
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: list with invalid columns")
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volumes", "list", "-c", "ID,Foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	mockCtrl.Finish()
	t.Log("list, API failure model error")
	apiErr := &volume_series.VolumeSeriesListDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
	params = volume_series.NewVolumeSeriesListParams()
	m = mockmgmtclient.NewVolumeSeriesMatcher(t, params)
	cOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(cOps)
	cOps.EXPECT().VolumeSeriesList(m).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume-series", "list"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	mockCtrl.Finish()
	t.Log("list, API failure arbitrary error")
	otherErr := fmt.Errorf("OTHER ERROR")
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
	params = volume_series.NewVolumeSeriesListParams()
	m = mockmgmtclient.NewVolumeSeriesMatcher(t, params)
	cOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(cOps)
	cOps.EXPECT().VolumeSeriesList(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume-series", "list"})
	assert.NotNil(err)
	assert.Equal(otherErr.Error(), err.Error())

	mockCtrl.Finish()
	t.Log("cspdomain list error")
	cspDomainListErr := fmt.Errorf("cspDomainListError")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	dm := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	dOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps)
	dOps.EXPECT().CspDomainList(dm).Return(nil, cspDomainListErr).MinTimes(1)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume-series", "list", "-D", "domain"})
	assert.NotNil(err)
	assert.Equal(cspDomainListErr.Error(), err.Error())

	// cluster list error
	mockCtrl.Finish()
	t.Log("cluster list error")
	clusterListErr := fmt.Errorf("clusterListErr")
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains)
	cm := mockmgmtclient.NewClusterMatcher(t, cluster.NewClusterListParams())
	clOps := mockmgmtclient.NewMockClusterClient(mockCtrl)
	mAPI.EXPECT().Cluster().Return(clOps)
	clOps.EXPECT().ClusterList(cm).Return(nil, clusterListErr).MinTimes(1)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volumes", "list", "-D", "domain"})
	assert.NotNil(err)
	assert.Equal(clusterListErr.Error(), err.Error())

	mockCtrl.Finish()
	t.Log("node list error")
	nodeListErr := fmt.Errorf("nodeListErr")
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters)
	nm := mockmgmtclient.NewNodeMatcher(t, node.NewNodeListParams())
	nOps := mockmgmtclient.NewMockNodeClient(mockCtrl)
	mAPI.EXPECT().Node().Return(nOps)
	nOps.EXPECT().NodeList(nm).Return(nil, nodeListErr).MinTimes(1)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volumes", "list", "-D", "domain"})
	assert.NotNil(err)
	assert.Equal(nodeListErr.Error(), err.Error())

	mockCtrl.Finish()
	t.Log("service plan list error")
	pListErr := fmt.Errorf("pListErr")
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
	ppm := mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	pOps := mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(pOps)
	pOps.EXPECT().ServicePlanList(ppm).Return(nil, pListErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volumes", "list", "--service-plan=planner"})
	assert.NotNil(err)
	assert.Equal(pListErr.Error(), err.Error())

	mockCtrl.Finish()
	t.Log("consistency group list error")
	cgListErr := fmt.Errorf("cgListErr")
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadContext, loadDomains, loadClusters, loadNodes)
	cgm := mockmgmtclient.NewConsistencyGroupMatcher(t, consistency_group.NewConsistencyGroupListParams())
	cgOps := mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cgOps)
	cgOps.EXPECT().ConsistencyGroupList(cgm).Return(nil, cgListErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume-series", "list", "-A", "System", "--consistency-group", "CG"})
	assert.NotNil(err)
	assert.Equal(cgListErr.Error(), err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	mockCtrl.Finish()
	t.Log("init context failure")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "System").Return("", fmt.Errorf("ctx error"))
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume-series", "list", "-A", "System"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// not found cases
	notFoundTCs := []struct {
		args    []string
		pat     string
		cacheIt []loadCache // defaults to load all caches
	}{
		{
			args:    []string{"volumes", "list", "-D", "foo"},
			pat:     "csp domain.*not found",
			cacheIt: []loadCache{loadDomains, loadClusters, loadNodes},
		},
		{
			args:    []string{"volumes", "list", "-D", "domainName", "-C", "foo"},
			pat:     "cluster.*not found",
			cacheIt: []loadCache{loadDomains, loadClusters, loadNodes},
		},
		{
			args:    []string{"volumes", "list", "-D", "domainName", "-C", "clusterName", "-N", "foo"},
			pat:     "node.*not found",
			cacheIt: []loadCache{loadDomains, loadClusters, loadNodes},
		},
		{
			args:    []string{"volumes", "list", "--service-plan", "foo"},
			pat:     "service plan.*not found",
			cacheIt: []loadCache{loadDomains, loadClusters, loadNodes, loadPlans},
		},
		{
			args:    []string{"volumes", "list", "-A", "System", "--consistency-group", "foo"},
			pat:     "consistency group.*not found",
			cacheIt: []loadCache{loadContext, loadDomains, loadClusters, loadNodes, loadCGs},
		},
	}
	for _, tc := range notFoundTCs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		t.Log(tc.args)
		mAPI = cacheLoadHelper(t, mockCtrl, tc.cacheIt...)
		appCtx.API = mAPI
		te = &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initVolumeSeries()
		err = parseAndRun(tc.args)
		assert.NotNil(err)
		assert.Regexp(tc.pat, err.Error())
		appCtx.Account, appCtx.AccountID = "", ""
	}

	// invalid flag combinations
	invalidFlagTCs := []struct {
		args []string
		pat  string
	}{
		{
			args: []string{"volumes", "list", "-N", "nodeName"},
			pat:  "node-name requires cluster-name and domain",
		},
		{
			args: []string{"volumes", "list", "-N", "nodeName", "-C", "clusterName"},
			pat:  "cluster-name requires domain",
		},
	}
	for _, tc := range invalidFlagTCs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		t.Log(tc.args)
		mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
		appCtx.API = mAPI
		te = &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initVolumeSeries()
		err = parseAndRun(tc.args)
		assert.NotNil(err)
		assert.Regexp(tc.pat, err.Error())
	}

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volumes", "list", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestVolumeSeriesModify(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	res := &volume_series.VolumeSeriesListOK{
		Payload: []*models.VolumeSeries{
			&models.VolumeSeries{
				VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
					RootParcelUUID: "uuid1",
				},
				VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
					AccountID: "accountID",
				},
				VolumeSeriesMutable: models.VolumeSeriesMutable{
					VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
						BoundClusterID: "clusterID",
						CapacityAllocations: map[string]models.CapacityAllocation{
							"poolID": {ConsumedBytes: swag.Int64(1073741824), ReservedBytes: swag.Int64(107374182400)},
						},
						Mounts: []*models.Mount{
							{
								SnapIdentifier:    "HEAD",
								MountedNodeID:     "nodeID-1",
								MountedNodeDevice: "/dev/a",
								MountMode:         "READ_WRITE",
								MountState:        "MOUNTED",
								MountTime:         strfmt.DateTime(now),
							},
						},
						RootStorageID: "storageID",
						StorageParcels: map[string]models.ParcelAllocation{
							"storageID": {SizeBytes: swag.Int64(1073741824)},
						},
						VolumeSeriesState: "IN_USE",
					},
					VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
						Description:        "desc",
						Name:               "vol1",
						ConsistencyGroupID: "c-g-1",
						ServicePlanID:      "plan-1",
						SizeBytes:          swag.Int64(107374182400),
						Tags:               models.ObjTags{"tag1", "tag2", "tag3"},
					},
				},
			},
		},
	}
	lParams := volume_series.NewVolumeSeriesListParams()
	lParams.Name = swag.String(string(res.Payload[0].Name))
	lParams.AccountID = swag.String(string(res.Payload[0].AccountID))
	m := mockmgmtclient.NewVolumeSeriesMatcher(t, lParams)

	t.Log("case: update with APPEND")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := cacheLoadHelper(t, mockCtrl, loadContext)
	vOps := mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vOps).MinTimes(1)
	vOps.EXPECT().VolumeSeriesList(m).Return(res, nil)
	uParams := volume_series.NewVolumeSeriesUpdateParams()
	uParams.Payload = &models.VolumeSeriesMutable{
		VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
			Description: "new Description",
			Tags:        []string{"tag1", "tag2"},
		},
	}
	uParams.ID = string(res.Payload[0].Meta.ID)
	uParams.Set = []string{"description"}
	uParams.Append = []string{"tags"}
	um := mockmgmtclient.NewVolumeSeriesMatcher(t, uParams)
	uRet := volume_series.NewVolumeSeriesUpdateOK()
	uRet.Payload = res.Payload[0]
	vOps.EXPECT().VolumeSeriesUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err := parseAndRun([]string{"volume", "modify", "-A", "System", "-n", *lParams.Name,
		"-d", " new Description ", "-t", "tag1", "-t", "tag2", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.VolumeSeries{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: update with SET")
	mAPI = cacheLoadHelper(t, mockCtrl, loadContext, loadCGs)
	vOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vOps).MinTimes(1)
	vOps.EXPECT().VolumeSeriesList(m).Return(res, nil)
	uParams = volume_series.NewVolumeSeriesUpdateParams()
	uParams.Payload = &models.VolumeSeriesMutable{
		VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
			Description:        "new Description",
			ConsistencyGroupID: "c-g-1",
			Tags:               []string{"tag1", "tag2"},
		},
	}
	uParams.ID = string(res.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"description", "consistencyGroupId", "tags"}
	um = mockmgmtclient.NewVolumeSeriesMatcher(t, uParams)
	uRet = volume_series.NewVolumeSeriesUpdateOK()
	uRet.Payload = res.Payload[0]
	vOps.EXPECT().VolumeSeriesUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume", "modify", "-A", "System", "-n", *lParams.Name,
		"-d", " new Description ", "--consistency-group", "CG",
		"-t", "tag1", "-t", "tag2", "--tag-action", "SET", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.VolumeSeries{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: update with REMOVE")
	mAPI = cacheLoadHelper(t, mockCtrl, loadContext)
	vOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vOps).MinTimes(1)
	vOps.EXPECT().VolumeSeriesList(m).Return(res, nil)
	uParams = volume_series.NewVolumeSeriesUpdateParams()
	uParams.Payload = &models.VolumeSeriesMutable{
		VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
			Tags: []string{"tag1", "tag2"},
		},
	}
	uParams.ID = string(res.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Remove = []string{"tags"}
	um = mockmgmtclient.NewVolumeSeriesMatcher(t, uParams)
	uRet = volume_series.NewVolumeSeriesUpdateOK()
	uRet.Payload = res.Payload[0]
	vOps.EXPECT().VolumeSeriesUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume", "modify", "-A", "System", "-n", *lParams.Name,
		"-t", "tag1", "-t", "tag2", "--tag-action", "REMOVE", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.VolumeSeries{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: update with empty SET")
	mAPI = cacheLoadHelper(t, mockCtrl, loadContext)
	vOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(vOps).MinTimes(1)
	vOps.EXPECT().VolumeSeriesList(m).Return(res, nil)
	uParams = volume_series.NewVolumeSeriesUpdateParams()
	uParams.Payload = &models.VolumeSeriesMutable{
		VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
			Description: "new Description",
		},
	}
	uParams.ID = string(res.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"description", "tags"}
	um = mockmgmtclient.NewVolumeSeriesMatcher(t, uParams)
	uRet = volume_series.NewVolumeSeriesUpdateOK()
	uRet.Payload = res.Payload[0]
	vOps.EXPECT().VolumeSeriesUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume", "modify", "-A", "System", "-n", *lParams.Name,
		"-d", " new Description ",
		"--tag-action", "SET", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.VolumeSeries{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	mockCtrl.Finish()
	t.Log("init context failure")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "System").Return("", fmt.Errorf("ctx error"))
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume-series", "modify", "-A", "System", "-n", *lParams.Name})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// other error cases
	updateErr := volume_series.NewVolumeSeriesUpdateDefault(400)
	updateErr.Payload = &models.Error{Code: 400, Message: swag.String("update error")}
	otherErr := fmt.Errorf("other error")
	mNotNil := gomock.Not(gomock.Nil)
	errTCs := []struct {
		name      string
		args      []string
		re        string
		clErr     error
		clRC      *volume_series.VolumeSeriesListOK
		updateErr error
		needCG    bool
		noCL      bool
		noCtx     bool
	}{
		{
			name: "No modifications",
			args: []string{"volume", "modify", "-A", "System", "-n", *lParams.Name, "-V", "1", "-o", "json"},
			re:   "No modifications",
		},
		{
			name:      "Update default error",
			args:      []string{"volume", "modify", "-A", "System", "-n", *lParams.Name, "-d", "NewDesc", "-V", "1", "-o", "json"},
			re:        "update error",
			updateErr: updateErr,
		},
		{
			name:      "Update other error",
			args:      []string{"volume", "modify", "-A", "System", "-n", *lParams.Name, "-d", "NewDesc", "-V", "1", "-o", "json"},
			re:        "other error",
			updateErr: otherErr,
		},
		{
			name:   "CG not found",
			args:   []string{"volume", "modify", "-A", "System", "-n", *lParams.Name, "--consistency-group=foo", "-V", "1", "-o", "json"},
			re:     "consistency group.*not found",
			needCG: true,
			noCL:   true,
		},
		{
			name:  "invalid columns",
			args:  []string{"volume", "modify", "-A", "System", "-n", *lParams.Name, "--columns", "ID,foo"},
			re:    "invalid column",
			noCL:  true,
			noCtx: true,
		},
		{
			name:  "invalid columns2",
			args:  []string{"volume", "modify", "-A", "System", "-n", *lParams.Name, "-c", "ID,foo"},
			re:    "invalid column",
			noCL:  true,
			noCtx: true,
		},
		{
			name: "volume series not found",
			args: []string{"volume", "modify", "-A", "System", "-n", *lParams.Name, "-d", "NewDesc", "-V", "1", "-o", "json"},
			re:   "volume series.*not found",
			clRC: &volume_series.VolumeSeriesListOK{Payload: []*models.VolumeSeries{}},
		},
		{
			name:  "volume series list error",
			args:  []string{"volume", "modify", "-A", "System", "-n", *lParams.Name, "-d", "NewDesc", "-V", "1", "-o", "json"},
			re:    "other error",
			clErr: otherErr,
		},
	}
	for _, tc := range errTCs {
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		if tc.needCG {
			mAPI = cacheLoadHelper(t, mockCtrl, loadContext, loadCGs)
		} else if !tc.noCtx {
			mAPI = cacheLoadHelper(t, mockCtrl, loadContext)
		}
		if !tc.noCL {
			vOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
			mAPI.EXPECT().VolumeSeries().Return(vOps).MinTimes(1)
			if tc.clErr != nil {
				vOps.EXPECT().VolumeSeriesList(m).Return(nil, tc.clErr)
			} else {
				if tc.clRC == nil {
					tc.clRC = res
				}
				vOps.EXPECT().VolumeSeriesList(m).Return(tc.clRC, nil)
			}
			if tc.updateErr != nil {
				vOps.EXPECT().VolumeSeriesUpdate(mNotNil).Return(nil, tc.updateErr)
			}
		}
		appCtx.API = mAPI
		t.Logf("case: %s", tc.name)
		te = &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initVolumeSeries()
		err = parseAndRun(tc.args)
		assert.NotNil(err)
		assert.Nil(te.jsonData)
		assert.Regexp(tc.re, err.Error())
		appCtx.Account, appCtx.AccountID = "", ""
	}

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume", "modify", "-A", "System", "-n", *lParams.Name,
		"-d", " new Description ", "-t", "tag1", "-t", "tag2", "o", "json"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestVolumeSeriesGetPV(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	fParams := volume_series.NewVolumeSeriesPVSpecFetchParams()
	fParams.ID = "vsID"
	fParams.ClusterVersion = swag.String("")
	// PV fetch
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(cOps).MinTimes(1)
	fRet := volume_series.NewVolumeSeriesPVSpecFetchOK()
	fRet.Payload = &models.VolumeSeriesPVSpecFetchOKBody{}
	fRet.Payload.PvSpec = "pv yaml"
	fRet.Payload.Format = "yaml"
	cOps.EXPECT().VolumeSeriesPVSpecFetch(fParams).Return(fRet, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	outFile := "./pv.yaml"
	defer func() { os.Remove(outFile) }()
	err := parseAndRun([]string{"volume-series", "get-pv", fParams.ID})
	assert.Nil(err)
	resBytes, err := ioutil.ReadFile(outFile)
	assert.Nil(err)
	assert.Equal(fRet.Payload.PvSpec, string(resBytes))

	// apiError
	apiErr := &volume_series.VolumeSeriesPVSpecFetchDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	fParams.Capacity = swag.Int64(10737418240)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(cOps).MinTimes(1)
	cOps.EXPECT().VolumeSeriesPVSpecFetch(fParams).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume-series", "get-pv", "--id", fParams.ID, "-O", "-", "--capacity", "10GiB"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())
	fParams.Capacity = nil // reset

	// other error
	otherErr := fmt.Errorf("other error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockVolumeSeriesClient(mockCtrl)
	mAPI.EXPECT().VolumeSeries().Return(cOps).MinTimes(1)
	cOps.EXPECT().VolumeSeriesPVSpecFetch(fParams).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume-series", "get-pv", "--id", fParams.ID})
	assert.NotNil(err)
	assert.Equal("other error", err.Error())

	mockCtrl.Finish()
	t.Log("init context failure")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "System").Return("", fmt.Errorf("ctx error"))
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume-series", "get-pv", "-A", "System", "--id", fParams.ID})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// invalid output file
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume-series", "get-pv", "--id", fParams.ID,
		"-O", "./bad/file"})
	assert.NotNil(err)
	assert.Regexp("no such file or directory", err.Error())

	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume-series", "get-pv", "--id", fParams.ID,
		"--capacity", "10GOOBERS"})
	assert.NotNil(err)
	assert.Regexp("units: unknown unit .*", err.Error())

	t.Log("expected id")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initVolumeSeries()
	err = parseAndRun([]string{"volume-series", "get-pv"})
	assert.NotNil(err)
	assert.Regexp("expected --id flag or a single identifier argument", err)
	appCtx.Account, appCtx.AccountID = "", ""
}
