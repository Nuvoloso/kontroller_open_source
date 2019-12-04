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


package gc

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/gcsdk"
	"github.com/Nuvoloso/kontroller/pkg/gcsdk/mock"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/docker/go-units"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

var operationResponse = compute.Operation{
	Id:            6917821783428586027,
	InsertTime:    "2017-11-22T12:57:40.084-08:00",
	Name:          "operation-1511384259910-55e9891ee5970-33fdc63d-4bee6b10",
	OperationType: "delete",
	Progress:      0,
	SelfLink:      "https://www.googleapis.com/compute/v1/projects/wk-dev/zones/us-central1-f/operations/operation-1511384259910-55e9891ee5970-33fdc63d-4bee6b10",
	Status:        "DONE",
	TargetId:      7642006033207418043,
	TargetLink:    "https://www.googleapis.com/compute/v1/projects/wk-dev/zones/us-central1-f/instances/permissionlogs-yuanwang-1-11221246-d0b6-harness-p548",
	Zone:          "https://www.googleapis.com/compute/v1/projects/wk-dev/zones/us-central1-f",
}

var creds = "{}"
var zone = "zone"
var projectID = "projectID"
var gcAttrs = map[string]models.ValueType{
	AttrCred: models.ValueType{Kind: "SECRET", Value: creds},
	AttrZone: models.ValueType{Kind: "STRING", Value: zone},
}

func TestVolumeIdentifierCreate(t *testing.T) {
	assert := assert.New(t)

	assert.Equal("gce:something", VolumeIdentifierCreate(ServiceGCE, "something"))
	assert.Equal("gcs:something", VolumeIdentifierCreate(ServiceGCS, "something"))
}

func TestVolumeIdentifierParser(t *testing.T) {
	assert := assert.New(t)

	// test the identifer parser
	s0, s1, err := VolumeIdentifierParse(ServiceGCE + ":something")
	assert.NoError(err)
	assert.Equal(ServiceGCE, s0)
	assert.Equal("something", s1)
	s0, s1, err = VolumeIdentifierParse(ServiceGCS + ":else:")
	assert.NoError(err)
	assert.Equal(ServiceGCS, s0)
	assert.Equal("else:", s1)
	s0, s1, err = VolumeIdentifierParse("foobar")
	assert.Regexp("invalid volume identifier", err)
	s0, s1, err = VolumeIdentifierParse(ServiceGCE)
	assert.NoError(err)
	assert.Equal(ServiceGCE, s0)
	assert.Empty(s1)
}

func TestVolumeAttach(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()

	nid := "n-1"
	vid := "v-1"
	vaa := &csp.VolumeAttachArgs{
		VolumeIdentifier: VolumeIdentifierCreate(ServiceGCE, vid),
		NodeIdentifier:   nid,
	}

	gcCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(gcCSP)
	cGC, ok := gcCSP.(*CSP)
	assert.True(ok)
	gcCl := &Client{
		csp:       cGC,
		api:       gcsdk.New(),
		attrs:     gcAttrs,
		projectID: projectID,
	}
	gcCl.vr = gcCl

	deviceName := "nuvoloso-" + vid
	disk := &compute.AttachedDisk{
		DeviceName: deviceName,
		Source:     fmt.Sprintf(diskSourceURL, projectID, gcAttrs[AttrZone].Value, deviceName),
	}

	// NewComputeService fails
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	api.EXPECT().NewComputeService(ctx, option.WithCredentialsJSON([]byte("{}"))).Return(nil, errors.New("error 1"))
	vol, err := gcCl.VolumeAttach(ctx, vaa)
	assert.Regexp("error 1", err)
	assert.Error(errors.Unwrap(err))
	assert.Nil(vol)
	mockCtrl.Finish()

	// AttachDisk.Do fails
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc := mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	instancesSvc := mock.NewMockInstancesService(mockCtrl)
	svc.EXPECT().Instances().Return(instancesSvc)
	call := mock.NewMockInstancesAttachDiskCall(mockCtrl)
	instancesSvc.EXPECT().AttachDisk(projectID, zone, nid, disk).Return(call)
	call.EXPECT().Context(ctx).Return(call)
	call.EXPECT().Do().Return(nil, errors.New("error 2"))
	vol, err = gcCl.VolumeAttach(ctx, vaa)
	assert.Regexp("error 2", err)
	assert.Nil(vol)
	mockCtrl.Finish()

	// gceVolumeGet fails
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc = mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	fvc := &fakeVolumeRetriever{retErr: errors.New("error 3")}
	gcCl.vr = fvc
	instancesSvc = mock.NewMockInstancesService(mockCtrl)
	svc.EXPECT().Instances().Return(instancesSvc)
	call = mock.NewMockInstancesAttachDiskCall(mockCtrl)
	instancesSvc.EXPECT().AttachDisk(projectID, zone, nid, disk).Return(call)
	call.EXPECT().Context(ctx).Return(call)
	call.EXPECT().Do().Return(&compute.Operation{Name: "operation-1"}, nil)
	zoneSvc := mock.NewMockZoneOperationsService(mockCtrl)
	svc.EXPECT().ZoneOperations().Return(zoneSvc)
	zoneCall := mock.NewMockZoneOperationsGetCall(mockCtrl)
	zoneSvc.EXPECT().Get(projectID, zone, "operation-1").Return(zoneCall)
	zoneCall.EXPECT().Context(ctx).Return(zoneCall)
	zoneCall.EXPECT().Do().Return(&compute.Operation{Status: "DONE"}, nil)
	vol, err = gcCl.VolumeAttach(ctx, vaa)
	assert.Equal(vid, fvc.inName)
	assert.Regexp("error 3", err)
	assert.Nil(vol)
	mockCtrl.Finish()

	// success
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc = mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	fvc = &fakeVolumeRetriever{retVolume: &csp.Volume{Identifier: vaa.VolumeIdentifier}}
	gcCl.vr = fvc
	instancesSvc = mock.NewMockInstancesService(mockCtrl)
	svc.EXPECT().Instances().Return(instancesSvc)
	call = mock.NewMockInstancesAttachDiskCall(mockCtrl)
	instancesSvc.EXPECT().AttachDisk(projectID, zone, nid, disk).Return(call)
	call.EXPECT().Context(ctx).Return(call)
	call.EXPECT().Do().Return(&compute.Operation{Name: "operation-1"}, nil)
	zoneSvc = mock.NewMockZoneOperationsService(mockCtrl)
	svc.EXPECT().ZoneOperations().Return(zoneSvc)
	zoneCall = mock.NewMockZoneOperationsGetCall(mockCtrl)
	zoneSvc.EXPECT().Get(projectID, zone, "operation-1").Return(zoneCall)
	zoneCall.EXPECT().Context(ctx).Return(zoneCall)
	zoneCall.EXPECT().Do().Return(&compute.Operation{Status: "DONE"}, nil)
	vol, err = gcCl.VolumeAttach(ctx, vaa)
	assert.Equal(vid, fvc.inName)
	assert.NoError(err)
	assert.True(vol == fvc.retVolume)
	mockCtrl.Finish()

	// waitForOperation: operation completes with status DONE but with error
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc = mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	fvc = &fakeVolumeRetriever{retVolume: &csp.Volume{Identifier: vaa.VolumeIdentifier}}
	gcCl.vr = fvc
	instancesSvc = mock.NewMockInstancesService(mockCtrl)
	svc.EXPECT().Instances().Return(instancesSvc)
	call = mock.NewMockInstancesAttachDiskCall(mockCtrl)
	instancesSvc.EXPECT().AttachDisk(projectID, zone, nid, disk).Return(call)
	call.EXPECT().Context(ctx).Return(call)
	call.EXPECT().Do().Return(&compute.Operation{Name: "operation-1"}, nil)
	zoneSvc = mock.NewMockZoneOperationsService(mockCtrl)
	svc.EXPECT().ZoneOperations().Return(zoneSvc)
	zoneCall = mock.NewMockZoneOperationsGetCall(mockCtrl)
	zoneSvc.EXPECT().Get(projectID, zone, "operation-1").Return(zoneCall)
	zoneCall.EXPECT().Context(ctx).Return(zoneCall)
	zoneCall.EXPECT().Do().Return(&compute.Operation{Status: "DONE", HttpErrorStatusCode: 404, HttpErrorMessage: "Resource not found", Error: &compute.OperationError{}}, nil)
	vol, err = gcCl.VolumeAttach(ctx, vaa)
	assert.Error(err)
	assert.Regexp("operation failure: code: 404", err)
	mockCtrl.Finish()

	// waitForOperation fails
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc = mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	fvc = &fakeVolumeRetriever{retVolume: &csp.Volume{Identifier: vaa.VolumeIdentifier}}
	gcCl.vr = fvc
	instancesSvc = mock.NewMockInstancesService(mockCtrl)
	svc.EXPECT().Instances().Return(instancesSvc)
	call = mock.NewMockInstancesAttachDiskCall(mockCtrl)
	instancesSvc.EXPECT().AttachDisk(projectID, zone, nid, disk).Return(call)
	call.EXPECT().Context(ctx).Return(call)
	call.EXPECT().Do().Return(&compute.Operation{Name: "operation-1"}, nil)
	zoneSvc = mock.NewMockZoneOperationsService(mockCtrl)
	svc.EXPECT().ZoneOperations().Return(zoneSvc)
	zoneCall = mock.NewMockZoneOperationsGetCall(mockCtrl)
	zoneSvc.EXPECT().Get(projectID, zone, "operation-1").Return(zoneCall)
	zoneCall.EXPECT().Context(ctx).Return(zoneCall)
	zoneCall.EXPECT().Do().Return(nil, errors.New("error 3"))
	vol, err = gcCl.VolumeAttach(ctx, vaa)
	assert.Error(err)
	assert.Nil(vol)
	mockCtrl.Finish()

	// waitForOperation needs to re-check for status
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc = mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	fvc = &fakeVolumeRetriever{retVolume: &csp.Volume{Identifier: vaa.VolumeIdentifier}}
	gcCl.vr = fvc
	instancesSvc = mock.NewMockInstancesService(mockCtrl)
	svc.EXPECT().Instances().Return(instancesSvc)
	call = mock.NewMockInstancesAttachDiskCall(mockCtrl)
	instancesSvc.EXPECT().AttachDisk(projectID, zone, nid, disk).Return(call)
	call.EXPECT().Context(ctx).Return(call)
	call.EXPECT().Do().Return(&compute.Operation{Name: "operation-1"}, nil)
	zoneSvc = mock.NewMockZoneOperationsService(mockCtrl)
	svc.EXPECT().ZoneOperations().Return(zoneSvc).MinTimes(2)
	zoneCall = mock.NewMockZoneOperationsGetCall(mockCtrl)
	zoneSvc.EXPECT().Get(projectID, zone, "operation-1").Return(zoneCall).MinTimes(2)
	zoneCall.EXPECT().Context(ctx).Return(zoneCall).MinTimes(2)
	firstCall := zoneCall.EXPECT().Do().Return(&compute.Operation{Status: "PENDING"}, nil)
	zoneCall.EXPECT().Do().Return(&compute.Operation{Status: "DONE"}, nil).After(firstCall)
	vol, err = gcCl.VolumeAttach(ctx, vaa)
	assert.Equal(vid, fvc.inName)
	assert.NoError(err)
	assert.True(vol == fvc.retVolume)

	// invalid storage type
	vaa.VolumeIdentifier = "not the volume identifier you are looking for"
	vol, err = gcCl.VolumeAttach(ctx, vaa)
	assert.Nil(vol)
	assert.Regexp("storage type currently unsupported", err)
}

func TestVolumeCreate(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()

	// insert fake storage types to force failure
	numST := len(gcCspStorageTypes)
	cspSTFooService := models.CspStorageType("Fake ST")
	fooServiceST := &models.CSPStorageType{
		CspDomainType:          CSPDomainType,
		Description:            "Fake ST",
		Name:                   cspSTFooService,
		MinAllocationSizeBytes: swag.Int64(4 * units.GiB),
		MaxAllocationSizeBytes: swag.Int64(16 * units.TiB),
		AccessibilityScope:     "CSPDOMAIN",
		CspStorageTypeAttributes: map[string]models.ValueType{
			PAService:    models.ValueType{Kind: "STRING", Value: "FooService"},
			PAVolumeType: models.ValueType{Kind: "STRING", Value: "blah"},
		},
	}
	gcCspStorageTypes = append(gcCspStorageTypes, fooServiceST)
	cspSTNoService := models.CspStorageType("No svc ST")
	noServiceST := &models.CSPStorageType{
		CspDomainType:          CSPDomainType,
		Description:            "not dynamically provisionable storage",
		Name:                   cspSTNoService,
		MinAllocationSizeBytes: swag.Int64(4 * units.GiB),
		MaxAllocationSizeBytes: swag.Int64(16 * units.TiB),
		AccessibilityScope:     "CSPDOMAIN",
		CspStorageTypeAttributes: map[string]models.ValueType{
			PAVolumeType: models.ValueType{Kind: "STRING", Value: "blah"},
		},
	}
	gcCspStorageTypes = append(gcCspStorageTypes, noServiceST)
	defer func() {
		gcCspStorageTypes = gcCspStorageTypes[:numST]
		createSTMap()
	}()
	createSTMap()

	gcCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(gcCSP)
	cGC, ok := gcCSP.(*CSP)
	assert.True(ok)

	gcCl := &Client{
		csp:       cGC,
		api:       gcsdk.New(),
		attrs:     gcAttrs,
		projectID: projectID,
	}
	gcCl.vr = gcCl

	// unsupported storage type
	vca := &csp.VolumeCreateArgs{
		StorageTypeName: cspSTFooService,
	}
	vol, err := gcCl.VolumeCreate(nil, vca)
	assert.Regexp("storage type currently unsupported", err)
	assert.Nil(vol)

	// not dynamically provisionable storage type
	vca.StorageTypeName = cspSTNoService
	vol, err = gcCl.VolumeCreate(nil, vca)
	assert.Regexp("storage type is not dynamically provisionable", err)
	assert.Nil(vol)

	// invalid storage type
	vca.StorageTypeName = "fooBarType"
	vol, err = gcCl.VolumeCreate(nil, vca)
	assert.Regexp("invalid storage type for CSPDomain", err)
	assert.Nil(vol)

	// valid storage type
	vca.StorageTypeName = "GCE pd-ssd"
	diskName := nuvoNamePrefix + "stg-1"
	volumeType := fmt.Sprintf(volTypeURL, projectID, gcAttrs[AttrZone].Value, "pd-ssd")
	disk := &compute.Disk{
		Name:   diskName,
		SizeGb: util.RoundUpBytes(vca.SizeBytes, units.GiB) / units.GiB,
		Type:   volumeType,
	}

	// NewComputeService fails
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	api.EXPECT().NewComputeService(ctx, option.WithCredentialsJSON([]byte("{}"))).Return(nil, errors.New("error 1"))
	vol, err = gcCl.VolumeCreate(ctx, vca)
	assert.Regexp("error 1", err)
	assert.Error(errors.Unwrap(err))
	assert.Nil(vol)
	mockCtrl.Finish()

	// Insert.Do fails with StorageID tag present
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc := mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	disksSvc := mock.NewMockDisksService(mockCtrl)
	svc.EXPECT().Disks().Return(disksSvc)
	call := mock.NewMockDisksInsertCall(mockCtrl)
	disksSvc.EXPECT().Insert(projectID, zone, gomock.Any()).Return(call)
	call.EXPECT().Context(ctx).Return(call)
	call.EXPECT().Do().Return(nil, errors.New("error 2"))
	vol, err = gcCl.VolumeCreate(ctx, vca)
	assert.Regexp("error 2", err)
	assert.Nil(vol)
	mockCtrl.Finish()

	// gceVolumeGet fails
	vca.Tags = []string{com.VolTagStorageID + ":stg-1"}
	disk.Labels = gceLabelsFromModel(vca.Tags)
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc = mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	fvc := &fakeVolumeRetriever{retErr: errors.New("error 3")}
	gcCl.vr = fvc
	disksSvc = mock.NewMockDisksService(mockCtrl)
	svc.EXPECT().Disks().Return(disksSvc)
	call = mock.NewMockDisksInsertCall(mockCtrl)
	disksSvc.EXPECT().Insert(projectID, zone, disk).Return(call)
	call.EXPECT().Context(ctx).Return(call)
	call.EXPECT().Do().Return(&compute.Operation{Name: "operation-1", TargetId: uint64(12345)}, nil)
	zoneSvc := mock.NewMockZoneOperationsService(mockCtrl)
	svc.EXPECT().ZoneOperations().Return(zoneSvc)
	zoneCall := mock.NewMockZoneOperationsGetCall(mockCtrl)
	zoneSvc.EXPECT().Get(projectID, zone, "operation-1").Return(zoneCall)
	zoneCall.EXPECT().Context(ctx).Return(zoneCall)
	zoneCall.EXPECT().Do().Return(&compute.Operation{Status: "DONE"}, nil)
	vol, err = gcCl.VolumeCreate(ctx, vca)
	assert.Equal(diskName, fvc.inName)
	assert.Regexp("error 3", err)
	assert.Nil(vol)
	mockCtrl.Finish()

	// success
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc = mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	fvc = &fakeVolumeRetriever{retVolume: &csp.Volume{Identifier: diskName}}
	gcCl.vr = fvc
	disksSvc = mock.NewMockDisksService(mockCtrl)
	svc.EXPECT().Disks().Return(disksSvc)
	call = mock.NewMockDisksInsertCall(mockCtrl)
	disksSvc.EXPECT().Insert(projectID, zone, disk).Return(call)
	call.EXPECT().Context(ctx).Return(call)
	call.EXPECT().Do().Return(&compute.Operation{Name: "operation-1", TargetId: uint64(12345)}, nil)
	zoneSvc = mock.NewMockZoneOperationsService(mockCtrl)
	svc.EXPECT().ZoneOperations().Return(zoneSvc)
	zoneCall = mock.NewMockZoneOperationsGetCall(mockCtrl)
	zoneSvc.EXPECT().Get(projectID, zone, "operation-1").Return(zoneCall)
	zoneCall.EXPECT().Context(ctx).Return(zoneCall)
	zoneCall.EXPECT().Do().Return(&compute.Operation{Status: "DONE"}, nil)
	vol, err = gcCl.VolumeCreate(ctx, vca)
	assert.Equal(diskName, fvc.inName)
	assert.NoError(err)
	assert.True(vol == fvc.retVolume)
	mockCtrl.Finish()

	// waitForOperation fails
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc = mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	fvc = &fakeVolumeRetriever{retVolume: &csp.Volume{Identifier: diskName}}
	gcCl.vr = fvc
	disksSvc = mock.NewMockDisksService(mockCtrl)
	svc.EXPECT().Disks().Return(disksSvc)
	call = mock.NewMockDisksInsertCall(mockCtrl)
	disksSvc.EXPECT().Insert(projectID, zone, disk).Return(call)
	call.EXPECT().Context(ctx).Return(call)
	call.EXPECT().Do().Return(&compute.Operation{Name: "operation-1"}, nil)
	zoneSvc = mock.NewMockZoneOperationsService(mockCtrl)
	svc.EXPECT().ZoneOperations().Return(zoneSvc)
	zoneCall = mock.NewMockZoneOperationsGetCall(mockCtrl)
	zoneSvc.EXPECT().Get(projectID, zone, "operation-1").Return(zoneCall)
	zoneCall.EXPECT().Context(ctx).Return(zoneCall)
	zoneCall.EXPECT().Do().Return(nil, errors.New("error 3"))
	vol, err = gcCl.VolumeCreate(ctx, vca)
	assert.Error(err)
	assert.Nil(vol)
}

func TestVolumeDelete(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()

	vid := "v-1"
	vda := &csp.VolumeDeleteArgs{
		VolumeIdentifier: VolumeIdentifierCreate(ServiceGCE, vid),
	}

	gcCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(gcCSP)
	cGC, ok := gcCSP.(*CSP)
	assert.True(ok)

	gcCl := &Client{
		csp:       cGC,
		api:       gcsdk.New(),
		attrs:     gcAttrs,
		projectID: projectID,
	}
	gcCl.vr = gcCl

	// getComputeService service
	err = gcCl.VolumeDelete(nil, vda)
	assert.Regexp("missing 'type' field in credentials", err)

	// Disks delete fails
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc := mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	disksSvc := mock.NewMockDisksService(mockCtrl)
	svc.EXPECT().Disks().Return(disksSvc)
	call := mock.NewMockDisksDeleteCall(mockCtrl)
	disksSvc.EXPECT().Delete(projectID, zone, vid).Return(call)
	call.EXPECT().Context(ctx).Return(call)
	call.EXPECT().Do().Return(nil, errors.New("error 1"))
	err = gcCl.VolumeDelete(ctx, vda)
	assert.Regexp("error 1", err)
	mockCtrl.Finish()

	// success
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc = mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	disksSvc = mock.NewMockDisksService(mockCtrl)
	svc.EXPECT().Disks().Return(disksSvc)
	call = mock.NewMockDisksDeleteCall(mockCtrl)
	disksSvc.EXPECT().Delete(projectID, zone, vid).Return(call)
	call.EXPECT().Context(ctx).Return(call)
	call.EXPECT().Do().Return(&compute.Operation{Name: "operation-1"}, nil)
	zoneSvc := mock.NewMockZoneOperationsService(mockCtrl)
	svc.EXPECT().ZoneOperations().Return(zoneSvc)
	zoneCall := mock.NewMockZoneOperationsGetCall(mockCtrl)
	zoneSvc.EXPECT().Get(projectID, zone, "operation-1").Return(zoneCall)
	zoneCall.EXPECT().Context(ctx).Return(zoneCall)
	zoneCall.EXPECT().Do().Return(&compute.Operation{Status: "DONE"}, nil)
	err = gcCl.VolumeDelete(ctx, vda)
	assert.NoError(err)
	mockCtrl.Finish()

	// waitForOperation fails to get operation data
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc = mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	disksSvc = mock.NewMockDisksService(mockCtrl)
	svc.EXPECT().Disks().Return(disksSvc)
	call = mock.NewMockDisksDeleteCall(mockCtrl)
	disksSvc.EXPECT().Delete(projectID, zone, vid).Return(call)
	call.EXPECT().Context(ctx).Return(call)
	call.EXPECT().Do().Return(&compute.Operation{Name: "operation-1"}, nil)
	zoneSvc = mock.NewMockZoneOperationsService(mockCtrl)
	svc.EXPECT().ZoneOperations().Return(zoneSvc)
	zoneCall = mock.NewMockZoneOperationsGetCall(mockCtrl)
	zoneSvc.EXPECT().Get(projectID, zone, "operation-1").Return(zoneCall)
	zoneCall.EXPECT().Context(ctx).Return(zoneCall)
	zoneCall.EXPECT().Do().Return(nil, errors.New("error 2"))
	err = gcCl.VolumeDelete(ctx, vda)
	assert.Error(err)

	// invalid storage type
	vda.VolumeIdentifier = "not the volume identifier you are looking for"
	err = gcCl.VolumeDelete(ctx, vda)
	assert.Regexp("invalid volume identifier", err)
}

func TestVolumeDetach(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()

	nid := "n-1"
	nd := "nd"
	vid := "v-1"
	vda := &csp.VolumeDetachArgs{
		VolumeIdentifier: VolumeIdentifierCreate(ServiceGCE, vid),
		NodeIdentifier:   nid,
		NodeDevice:       nd,
	}

	gcCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(gcCSP)
	cGC, ok := gcCSP.(*CSP)
	assert.True(ok)

	gcCl := &Client{
		csp:       cGC,
		api:       gcsdk.New(),
		attrs:     gcAttrs,
		projectID: projectID,
	}
	gcCl.vr = gcCl

	// NewComputeService fails
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	api.EXPECT().NewComputeService(ctx, option.WithCredentialsJSON([]byte("{}"))).Return(nil, errors.New("error 1"))
	vol, err := gcCl.VolumeDetach(ctx, vda)
	assert.Regexp("error 1", err)
	assert.Error(errors.Unwrap(err))
	assert.Nil(vol)
	mockCtrl.Finish()

	// DetachDisk.Do fails
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc := mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	instancesSvc := mock.NewMockInstancesService(mockCtrl)
	svc.EXPECT().Instances().Return(instancesSvc)
	call := mock.NewMockInstancesDetachDiskCall(mockCtrl)
	instancesSvc.EXPECT().DetachDisk(projectID, zone, nid, vid).Return(call)
	call.EXPECT().Context(ctx).Return(call)
	call.EXPECT().Do().Return(nil, errors.New("error 2"))
	vol, err = gcCl.VolumeDetach(ctx, vda)
	assert.Regexp("error 2", err)
	assert.Nil(vol)
	mockCtrl.Finish()

	// gceVolumeGet fails
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc = mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	fvc := &fakeVolumeRetriever{retErr: errors.New("error 3")}
	gcCl.vr = fvc
	instancesSvc = mock.NewMockInstancesService(mockCtrl)
	svc.EXPECT().Instances().Return(instancesSvc)
	call = mock.NewMockInstancesDetachDiskCall(mockCtrl)
	instancesSvc.EXPECT().DetachDisk(projectID, zone, nid, vid).Return(call)
	call.EXPECT().Context(ctx).Return(call)
	call.EXPECT().Do().Return(&compute.Operation{Name: "operation-1"}, nil)
	zoneSvc := mock.NewMockZoneOperationsService(mockCtrl)
	svc.EXPECT().ZoneOperations().Return(zoneSvc)
	zoneCall := mock.NewMockZoneOperationsGetCall(mockCtrl)
	zoneSvc.EXPECT().Get(projectID, zone, "operation-1").Return(zoneCall)
	zoneCall.EXPECT().Context(ctx).Return(zoneCall)
	zoneCall.EXPECT().Do().Return(&compute.Operation{Status: "DONE"}, nil)
	vol, err = gcCl.VolumeDetach(ctx, vda)
	assert.Equal(vid, fvc.inName)
	assert.Regexp("error 3", err)
	assert.Nil(vol)
	mockCtrl.Finish()

	// success
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc = mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	fvc = &fakeVolumeRetriever{retVolume: &csp.Volume{Identifier: vda.VolumeIdentifier}}
	gcCl.vr = fvc
	instancesSvc = mock.NewMockInstancesService(mockCtrl)
	svc.EXPECT().Instances().Return(instancesSvc)
	call = mock.NewMockInstancesDetachDiskCall(mockCtrl)
	instancesSvc.EXPECT().DetachDisk(projectID, zone, nid, vid).Return(call)
	call.EXPECT().Context(ctx).Return(call)
	call.EXPECT().Do().Return(&compute.Operation{Name: "operation-1"}, nil)
	zoneSvc = mock.NewMockZoneOperationsService(mockCtrl)
	svc.EXPECT().ZoneOperations().Return(zoneSvc)
	zoneCall = mock.NewMockZoneOperationsGetCall(mockCtrl)
	zoneSvc.EXPECT().Get(projectID, zone, "operation-1").Return(zoneCall)
	zoneCall.EXPECT().Context(ctx).Return(zoneCall)
	zoneCall.EXPECT().Do().Return(&compute.Operation{Status: "DONE"}, nil)
	vol, err = gcCl.VolumeDetach(ctx, vda)
	assert.Equal(vid, fvc.inName)
	assert.NoError(err)
	assert.True(vol == fvc.retVolume)
	mockCtrl.Finish()

	// waitForOperation fails, force is ignored
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc = mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	fvc = &fakeVolumeRetriever{retVolume: &csp.Volume{Identifier: vda.VolumeIdentifier}}
	gcCl.vr = fvc
	instancesSvc = mock.NewMockInstancesService(mockCtrl)
	svc.EXPECT().Instances().Return(instancesSvc)
	call = mock.NewMockInstancesDetachDiskCall(mockCtrl)
	instancesSvc.EXPECT().DetachDisk(projectID, zone, nid, vid).Return(call)
	call.EXPECT().Context(ctx).Return(call)
	call.EXPECT().Do().Return(&compute.Operation{Name: "operation-1"}, nil)
	zoneSvc = mock.NewMockZoneOperationsService(mockCtrl)
	svc.EXPECT().ZoneOperations().Return(zoneSvc)
	zoneCall = mock.NewMockZoneOperationsGetCall(mockCtrl)
	zoneSvc.EXPECT().Get(projectID, zone, "operation-1").Return(zoneCall)
	zoneCall.EXPECT().Context(ctx).Return(zoneCall)
	zoneCall.EXPECT().Do().Return(nil, errors.New("error 3"))
	vda.Force = true
	vol, err = gcCl.VolumeDetach(ctx, vda)
	assert.Regexp("error 3", err)
	assert.Nil(vol)

	// invalid storage type
	vda.VolumeIdentifier = "not the volume identifier you are looking for"
	vol, err = gcCl.VolumeDetach(ctx, vda)
	assert.Regexp("storage type currently unsupported", err)
	assert.Nil(vol)
}

func TestVolumeFetch(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()

	vid := "v-1"
	vfa := &csp.VolumeFetchArgs{
		VolumeIdentifier: VolumeIdentifierCreate(ServiceGCE, vid),
	}

	gcCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(gcCSP)
	cGC, ok := gcCSP.(*CSP)
	assert.True(ok)

	gcCl := &Client{
		csp:       cGC,
		api:       gcsdk.New(),
		attrs:     gcAttrs,
		projectID: projectID,
	}
	gcCl.vr = gcCl

	// NewComputeService fails
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	api.EXPECT().NewComputeService(ctx, option.WithCredentialsJSON([]byte("{}"))).Return(nil, errors.New("error 1"))
	vol, err := gcCl.VolumeFetch(ctx, vfa)
	assert.Regexp("error 1", err)
	assert.Error(errors.Unwrap(err))
	assert.Nil(vol)
	mockCtrl.Finish()

	// gceVolumeGet fails
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc := mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	fvc := &fakeVolumeRetriever{retErr: errors.New("error 3")}
	gcCl.vr = fvc
	vol, err = gcCl.VolumeFetch(ctx, vfa)
	assert.Equal(vid, fvc.inName)
	assert.Regexp("error 3", err)
	assert.Nil(vol)
	mockCtrl.Finish()

	// success
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc = mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	fvc = &fakeVolumeRetriever{retVolume: &csp.Volume{Identifier: vfa.VolumeIdentifier}}
	gcCl.vr = fvc
	vol, err = gcCl.VolumeFetch(ctx, vfa)
	assert.Equal(vid, fvc.inName)
	assert.NoError(err)
	assert.True(fvc.retVolume == vol)

	// invalid storage type
	vfa.VolumeIdentifier = "not the volume identifier you are looking for"
	vol, err = gcCl.VolumeFetch(ctx, vfa)
	assert.Regexp("invalid volume identifier", err)
	assert.Nil(vol)
}

func TestVolumeList(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()

	gcCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(gcCSP)
	cGC, ok := gcCSP.(*CSP)
	assert.True(ok)

	gcCl := &Client{
		csp:       cGC,
		api:       gcsdk.New(),
		attrs:     gcAttrs,
		projectID: projectID,
	}
	gcCl.vr = gcCl

	vla := &csp.VolumeListArgs{
		StorageTypeName: "GCE pd-ssd",
		Tags:            []string{"label1:label1-value", "label2", "label3:label3-value"},
	}

	// NewComputeService fails
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	api.EXPECT().NewComputeService(ctx, option.WithCredentialsJSON([]byte("{}"))).Return(nil, errors.New("error 1"))
	vols, err := gcCl.VolumeList(ctx, vla)
	assert.Regexp("error 1", err)
	assert.Error(errors.Unwrap(err))
	assert.Nil(vols)
	mockCtrl.Finish()

	diskList := []*compute.Disk{
		&compute.Disk{},
	}
	// success
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc := mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	fvc := &fakeVolumeRetriever{retVolume: &csp.Volume{Identifier: "vid"}}
	gcCl.vr = fvc
	disksSvc := mock.NewMockDisksService(mockCtrl)
	svc.EXPECT().Disks().Return(disksSvc)
	call := mock.NewMockDisksListCall(mockCtrl)
	disksSvc.EXPECT().List(projectID, zone).Return(call)
	call.EXPECT().Filter(`type="https://www.googleapis.com/compute/v1/projects/projectID/zones/zone/diskTypes/pd-ssd" AND labels.label1="label1-value" AND labels.label2:* AND labels.label3="label3-value"`).Return(call)
	m := newDiskPagesMatcher(&compute.DiskList{Items: diskList})
	call.EXPECT().Pages(ctx, m).Return(nil)
	vols, err = gcCl.VolumeList(ctx, vla)
	assert.NoError(err)
	mockCtrl.Finish()

	// Disks.List fails
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc = mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	disksSvc = mock.NewMockDisksService(mockCtrl)
	svc.EXPECT().Disks().Return(disksSvc)
	call = mock.NewMockDisksListCall(mockCtrl)
	disksSvc.EXPECT().List(projectID, zone).Return(call)
	call.EXPECT().Filter(`type="https://www.googleapis.com/compute/v1/projects/projectID/zones/zone/diskTypes/pd-ssd" AND labels.label1="label1-value" AND labels.label2:* AND labels.label3="label3-value"`).Return(call)
	call.EXPECT().Pages(ctx, gomock.Not(gomock.Nil())).Return(errors.New("error 1"))
	vols, err = gcCl.VolumeList(ctx, vla)
	assert.Regexp("error 1", err)
	assert.Nil(vols)

	// invalid storage type in filter
	vla.StorageTypeName = "invalid storage type"
	service, st, obj := StorageTypeToServiceVolumeType(vla.StorageTypeName)
	assert.Empty(service)
	assert.Empty(st)
	assert.Nil(obj)
	vols, err = gcCl.VolumeList(ctx, vla)
	assert.Regexp("invalid storage type", err)
	assert.Nil(vols)

	// valid storage type, but wrong service
	vla.StorageTypeName = "GCE local-ssd"
	service, st, obj = StorageTypeToServiceVolumeType(vla.StorageTypeName)
	assert.Empty(service)
	assert.Empty(st)
	assert.NotNil(obj)
	vols, err = gcCl.VolumeList(ctx, vla)
	assert.Regexp("invalid storage type", err)
	assert.Nil(vols)
}

func TestVolumeTags(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()

	gcCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(gcCSP)
	cGC, ok := gcCSP.(*CSP)
	assert.True(ok)

	gcCl := &Client{
		csp:       cGC,
		api:       gcsdk.New(),
		attrs:     gcAttrs,
		projectID: projectID,
	}

	vta := &csp.VolumeTagArgs{
		VolumeIdentifier: VolumeIdentifierCreate(ServiceGCE, "disk-1"),
	}

	// empty tags
	vta.Tags = []string{}
	assert.True(vta.Tags != nil)
	assert.Len(vta.Tags, 0)
	vol, err := gcCl.VolumeTagsDelete(nil, vta)
	assert.Regexp("no tags specified", err)
	assert.Nil(vol)
	vol, err = gcCl.VolumeTagsSet(nil, vta)
	assert.Regexp("no tags specified", err)
	assert.Nil(vol)

	vta.Tags = []string{"label1:label1-value", "label3:label3-value"}

	// NewComputeService fails
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	api.EXPECT().NewComputeService(ctx, option.WithCredentialsJSON([]byte("{}"))).Return(nil, errors.New("error 1"))
	vol, err = gcCl.VolumeTagsDelete(ctx, vta)
	assert.Regexp("error 1", err)
	assert.Error(errors.Unwrap(err))
	assert.Nil(vol)
	mockCtrl.Finish()

	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	api.EXPECT().NewComputeService(ctx, option.WithCredentialsJSON([]byte("{}"))).Return(nil, errors.New("error 1"))
	vol, err = gcCl.VolumeTagsSet(ctx, vta)
	assert.Regexp("error 1", err)
	assert.Error(errors.Unwrap(err))
	assert.Nil(vol)
	mockCtrl.Finish()

	// gceVolumeGet fails
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc := mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	fvc := &fakeVolumeRetriever{retErr: errors.New("error 2")}
	gcCl.vr = fvc
	vol, err = gcCl.VolumeTagsDelete(ctx, vta)
	assert.Regexp("error 2", err)
	assert.Nil(vol)
	vol, err = gcCl.VolumeTagsSet(ctx, vta)
	assert.Regexp("error 2", err)
	assert.Nil(vol)
	mockCtrl.Finish()

	labelFingerprint := "label-fingerprint"
	disk := &compute.Disk{
		Name:             "disk-1",
		Type:             "pd-ssd",
		SizeGb:           1,
		Labels:           map[string]string{"a": "b"},
		LabelFingerprint: labelFingerprint,
	}
	labelsRequest := &compute.ZoneSetLabelsRequest{
		LabelFingerprint: labelFingerprint,
		Labels:           map[string]string{"a": "b"},
	}

	// SetLabels fails
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc = mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	fvc = &fakeVolumeRetriever{retVolume: &csp.Volume{Identifier: vta.VolumeIdentifier, Raw: disk}}
	gcCl.vr = fvc
	disksSvc := mock.NewMockDisksService(mockCtrl)
	svc.EXPECT().Disks().Return(disksSvc)
	setLabelsCall := mock.NewMockDisksSetLabelsCall(mockCtrl)
	disksSvc.EXPECT().SetLabels(projectID, zone, "disk-1", labelsRequest).Return(setLabelsCall)
	setLabelsCall.EXPECT().Context(ctx).Return(setLabelsCall)
	setLabelsCall.EXPECT().Do().Return(nil, errors.New("error 3"))
	vol, err = gcCl.VolumeTagsDelete(ctx, vta)
	assert.Error(err)
	assert.Regexp("error 3", err)
	mockCtrl.Finish()

	// success for VolumeTagsDelete
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc = mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	fvc = &fakeVolumeRetriever{retVolume: &csp.Volume{Identifier: vta.VolumeIdentifier, Raw: disk}}
	gcCl.vr = fvc
	disksSvc = mock.NewMockDisksService(mockCtrl)
	svc.EXPECT().Disks().Return(disksSvc)
	setLabelsCall = mock.NewMockDisksSetLabelsCall(mockCtrl)
	disksSvc.EXPECT().SetLabels(projectID, zone, "disk-1", labelsRequest).Return(setLabelsCall)
	setLabelsCall.EXPECT().Context(ctx).Return(setLabelsCall)
	setLabelsCall.EXPECT().Do().Return(&compute.Operation{Status: "DONE"}, nil)
	zoneSvc := mock.NewMockZoneOperationsService(mockCtrl)
	svc.EXPECT().ZoneOperations().Return(zoneSvc)
	zoneCall := mock.NewMockZoneOperationsGetCall(mockCtrl)
	zoneSvc.EXPECT().Get(projectID, zone, gomock.Not(gomock.Nil())).Return(zoneCall)
	zoneCall.EXPECT().Context(ctx).Return(zoneCall)
	zoneCall.EXPECT().Do().Return(&compute.Operation{Status: "DONE"}, nil)
	vol, err = gcCl.VolumeTagsDelete(ctx, vta)
	assert.Equal(ctx, fvc.inCtx)
	assert.Equal("disk-1", fvc.inName)
	assert.EqualValues(vol.Raw.(*compute.Disk), fvc.retVolume.Raw.(*compute.Disk))
	assert.NoError(err)
	mockCtrl.Finish()

	// success for VolumeTagsSet
	disk.Labels = map[string]string{"a": "b", "label2": ""}
	labelsToSet := gceLabelsFromModel(vta.Tags)
	labelsToSet["a"] = "b"
	labelsToSet["label2"] = "label2-value"
	labelsRequest = &compute.ZoneSetLabelsRequest{
		LabelFingerprint: labelFingerprint,
		Labels:           labelsToSet,
	}
	vta.Tags = []string{"label1:label1-value", "label2:label2-value", "label3:label3-value"}

	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc = mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	fvc = &fakeVolumeRetriever{retVolume: &csp.Volume{Identifier: vta.VolumeIdentifier, Raw: disk}}
	gcCl.vr = fvc
	disksSvc = mock.NewMockDisksService(mockCtrl)
	svc.EXPECT().Disks().Return(disksSvc)
	setLabelsCall = mock.NewMockDisksSetLabelsCall(mockCtrl)
	disksSvc.EXPECT().SetLabels(projectID, zone, "disk-1", labelsRequest).Return(setLabelsCall)
	setLabelsCall.EXPECT().Context(ctx).Return(setLabelsCall)
	setLabelsCall.EXPECT().Do().Return(&compute.Operation{Status: "DONE"}, nil)
	zoneSvc = mock.NewMockZoneOperationsService(mockCtrl)
	svc.EXPECT().ZoneOperations().Return(zoneSvc)
	zoneCall = mock.NewMockZoneOperationsGetCall(mockCtrl)
	zoneSvc.EXPECT().Get(projectID, zone, gomock.Not(gomock.Nil())).Return(zoneCall)
	zoneCall.EXPECT().Context(ctx).Return(zoneCall)
	zoneCall.EXPECT().Do().Return(&compute.Operation{Status: "DONE"}, nil)
	vol, err = gcCl.VolumeTagsSet(ctx, vta)
	assert.Equal(ctx, fvc.inCtx)
	assert.Equal("disk-1", fvc.inName)
	assert.True(vol == fvc.retVolume)
	assert.NoError(err)
	mockCtrl.Finish()

	// retry after invalid fingerprint
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc = mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	fvc = &fakeVolumeRetriever{retVolume: &csp.Volume{Identifier: vta.VolumeIdentifier, Raw: disk}}
	gcCl.vr = fvc
	disksSvc = mock.NewMockDisksService(mockCtrl)
	svc.EXPECT().Disks().Return(disksSvc).MinTimes(2)
	setLabelsCall = mock.NewMockDisksSetLabelsCall(mockCtrl)
	disksSvc.EXPECT().SetLabels(projectID, zone, "disk-1", labelsRequest).Return(setLabelsCall).MinTimes(2)
	setLabelsCall.EXPECT().Context(ctx).Return(setLabelsCall).MinTimes(2)
	firstCall := setLabelsCall.EXPECT().Do().Return(nil, errors.New("googleapi: Error 412: Labels fingerprint either invalid or resource labels have changed, conditionNotMet"))
	setLabelsCall.EXPECT().Do().Return(&compute.Operation{Status: "DONE"}, nil).After(firstCall)
	zoneSvc = mock.NewMockZoneOperationsService(mockCtrl)
	svc.EXPECT().ZoneOperations().Return(zoneSvc)
	zoneCall = mock.NewMockZoneOperationsGetCall(mockCtrl)
	zoneSvc.EXPECT().Get(projectID, zone, gomock.Not(gomock.Nil())).Return(zoneCall)
	zoneCall.EXPECT().Context(ctx).Return(zoneCall)
	zoneCall.EXPECT().Do().Return(&compute.Operation{Status: "DONE"}, nil)
	vol, err = gcCl.VolumeTagsSet(ctx, vta)
	assert.Equal(ctx, fvc.inCtx)
	assert.True(fvc.inCnt >= 2) // retried
	assert.Equal("disk-1", fvc.inName)
	assert.True(vol == fvc.retVolume)
	assert.NoError(err)

	// invalid storage type
	vta.VolumeIdentifier = "not the volume identifier you are looking for"
	vol, err = gcCl.VolumeTagsDelete(ctx, vta)
	assert.Regexp("invalid volume identifier", err)
	assert.Nil(vol)
	vol, err = gcCl.VolumeTagsSet(ctx, vta)
	assert.Regexp("invalid volume identifier", err)
	assert.Nil(vol)
}

func TestVolumeSize(t *testing.T) {
	assert := assert.New(t)

	// insert fake storage types to force failure
	numST := len(gcCspStorageTypes)
	cspSTFooService := models.CspStorageType("Fake ST")
	fooServiceST := &models.CSPStorageType{
		CspDomainType:          CSPDomainType,
		Description:            "Fake ST",
		Name:                   cspSTFooService,
		MinAllocationSizeBytes: swag.Int64(4 * units.GiB),
		MaxAllocationSizeBytes: swag.Int64(16 * units.TiB),
		AccessibilityScope:     "CSPDOMAIN",
		CspStorageTypeAttributes: map[string]models.ValueType{
			PAService:    models.ValueType{Kind: "STRING", Value: "FooService"},
			PAVolumeType: models.ValueType{Kind: "STRING", Value: "blah"},
		},
	}
	gcCspStorageTypes = append(gcCspStorageTypes, fooServiceST)
	cspSTNoService := models.CspStorageType("No svc ST")
	noServiceST := &models.CSPStorageType{
		CspDomainType:          CSPDomainType,
		Description:            "not dynamically provisionable storage",
		Name:                   cspSTNoService,
		MinAllocationSizeBytes: swag.Int64(4 * units.GiB),
		MaxAllocationSizeBytes: swag.Int64(16 * units.TiB),
		AccessibilityScope:     "CSPDOMAIN",
		CspStorageTypeAttributes: map[string]models.ValueType{
			PAVolumeType: models.ValueType{Kind: "STRING", Value: "blah"},
		},
	}
	gcCspStorageTypes = append(gcCspStorageTypes, noServiceST)
	volType := "fakeVT"
	fakeST := &models.CSPStorageType{
		CspDomainType:          CSPDomainType,
		Description:            "Fake ST with IOPS",
		Name:                   "Fake ST with IOPS",
		MinAllocationSizeBytes: swag.Int64(1 * units.GiB),
		MaxAllocationSizeBytes: swag.Int64(16 * units.TiB),
		AccessibilityScope:     "CSPDOMAIN",
		CspStorageTypeAttributes: map[string]models.ValueType{
			PAService:    models.ValueType{Kind: "STRING", Value: ServiceGCE},
			PAVolumeType: models.ValueType{Kind: "STRING", Value: volType},
			PAIopsGB:     models.ValueType{Kind: "INT", Value: "2"},
			PAIopsMin:    models.ValueType{Kind: "INT", Value: "50"},
			PAIopsMax:    models.ValueType{Kind: "INT", Value: "32000"},
		},
	}
	gcCspStorageTypes = append(gcCspStorageTypes, fakeST)
	defer func() {
		gcCspStorageTypes = gcCspStorageTypes[:numST]
		createSTMap()
	}()
	createSTMap()

	cspST := VolTypeToCSPStorageType(volType)
	assert.NotEqual("", cspST)
	assert.EqualValues("Fake ST with IOPS", cspST)
	svc, gcVolT, stObj := StorageTypeToServiceVolumeType(cspST)
	assert.Equal(ServiceGCE, svc)
	assert.EqualValues(volType, gcVolT)
	assert.Equal(fakeST, stObj)

	gcCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(gcCSP)
	_, ok := gcCSP.(*CSP)
	assert.True(ok)
	cGC, ok := gcCSP.(*CSP)
	assert.True(ok)
	gcCl := &Client{
		csp: cGC,
	}

	// unsupported storage type
	sizeBytes, err := gcCl.VolumeSize(nil, cspSTFooService, 1)
	assert.Regexp("storage type currently unsupported", err)
	assert.Zero(sizeBytes)

	// not dynamically provisionable storage type
	sizeBytes, err = gcCl.VolumeSize(nil, cspSTNoService, 1)
	assert.Regexp("storage type is not dynamically provisionable", err)
	assert.Zero(sizeBytes)

	// invalid storage type
	sizeBytes, err = gcCl.VolumeSize(nil, "fooBarType", 1)
	assert.Regexp("invalid storage type for CSPDomain", err)
	assert.Zero(sizeBytes)

	// invalid sizes
	sizeBytes, err = gcCl.VolumeSize(nil, "foo", -1)
	assert.Regexp("invalid allocation size", err)
	assert.Zero(sizeBytes)
	sizeBytes, err = gcCl.VolumeSize(nil, fooServiceST.Name, *fooServiceST.MaxAllocationSizeBytes+1)
	assert.Regexp("requested size exceeds the storage type maximum", err)
	assert.Zero(sizeBytes)

	// valid storage type
	sizeBytes, err = gcCl.VolumeSize(nil, "GCE pd-ssd", 1)
	assert.NoError(err)

	oneGib := int64(units.GiB)
	oneGB := int64(units.GB)
	sizeTCs := []struct{ req, actual int64 }{
		{0, oneGib},
		{oneGB, oneGib},
		{oneGB + 1, oneGib},
		{oneGB + (oneGib - oneGB - 1), oneGib},
		{oneGib, oneGib},
		{oneGib + 1, 2 * oneGib},
	}
	for _, tc := range sizeTCs {
		sizeBytes, err := gcCl.VolumeSize(nil, cspST, tc.req)
		assert.NoError(err)
		assert.Equal(tc.actual, sizeBytes)
	}
}

func TestGetComputeService(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()

	csp, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(csp)
	gcCSP, ok := csp.(*CSP)
	assert.True(ok)
	assert.NotNil(gcCSP)

	gcCl := &Client{
		csp:       gcCSP,
		api:       gcsdk.New(),
		attrs:     map[string]models.ValueType{},
		projectID: "test-proj-1",
	}

	// NewComputeService fails
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	api.EXPECT().NewComputeService(ctx, option.WithCredentialsJSON([]byte(""))).Return(nil, errors.New("error 1"))
	cs, err := gcCl.getComputeService(ctx)
	assert.Regexp("error 1", err)
	assert.Error(errors.Unwrap(err))
	assert.Nil(cs)

	// again with nil context, minimal serviceAccount
	serviceAccount := `{ "type": "service_account", "project_id": "test-proj-1" }`
	gcCl.attrs[AttrCred] = models.ValueType{Kind: "SECRET", Value: serviceAccount}
	api.EXPECT().NewComputeService(gomock.Not(gomock.Nil()), option.WithCredentialsJSON([]byte(serviceAccount))).Return(nil, errors.New("error 2"))
	cs, err = gcCl.getComputeService(nil)
	assert.Regexp("error 2", err)
	assert.Error(errors.Unwrap(err))
	assert.Nil(cs)

	// success
	svc := mock.NewMockComputeService(mockCtrl)
	api.EXPECT().NewComputeService(ctx, option.WithCredentialsJSON([]byte(serviceAccount))).Return(svc, nil)
	cs, err = gcCl.getComputeService(ctx)
	assert.NoError(err)
	assert.EqualValues(svc, cs)

	// re-call is a no-op
	assert.NotNil(gcCl.computeService)
	cs, err = gcCl.getComputeService(ctx)
	assert.NoError(err)
	assert.EqualValues(svc, cs)
}

func TestGceVolumeGet(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()

	cspObj, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(cspObj)
	gcCSP, ok := cspObj.(*CSP)
	assert.True(ok)
	assert.NotNil(gcCSP)

	gcCl := &Client{
		csp:       gcCSP,
		api:       gcsdk.New(),
		attrs:     gcAttrs,
		projectID: projectID,
	}

	// Do fails
	mockCtrl := gomock.NewController(t)
	api := mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc := mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	disksSvc := mock.NewMockDisksService(mockCtrl)
	svc.EXPECT().Disks().Return(disksSvc)
	call := mock.NewMockDisksGetCall(mockCtrl)
	disksSvc.EXPECT().Get(projectID, zone, "vid").Return(call)
	call.EXPECT().Context(ctx).Return(call)
	call.EXPECT().Do().Return(nil, errors.New("error 1"))
	v, err := gcCl.gceVolumeGet(ctx, "vid")
	assert.Nil(v)
	assert.Regexp("error 1", err)
	mockCtrl.Finish()

	// success
	disk := &compute.Disk{}
	mockCtrl = gomock.NewController(t)
	api = mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc = mock.NewMockComputeService(mockCtrl)
	gcCl.computeService = svc
	disksSvc = mock.NewMockDisksService(mockCtrl)
	svc.EXPECT().Disks().Return(disksSvc)
	call = mock.NewMockDisksGetCall(mockCtrl)
	disksSvc.EXPECT().Get(projectID, zone, "vid").Return(call)
	call.EXPECT().Context(ctx).Return(call)
	call.EXPECT().Do().Return(disk, nil)
	v, err = gcCl.gceVolumeGet(ctx, "vid")
	if assert.NotNil(v) {
		expV := &csp.Volume{
			CSPDomainType: "GCP",
			Identifier:    "gce:",
			Tags:          []string{},
			Attachments:   []csp.VolumeAttachment{},
			Raw:           disk,
		}
		assert.Equal(expV, v)
	}
	assert.NoError(err)
}

func TestGceDiskToVolume(t *testing.T) {
	assert := assert.New(t)

	// nil panics
	assert.Panics(func() { gceDiskToVolume(nil) })

	// empty object succeeds
	gceDisk := &compute.Disk{}
	ret := gceDiskToVolume(gceDisk)
	exp := &csp.Volume{
		CSPDomainType: "GCP",
		Identifier:    "gce:",
		Tags:          []string{},
		Attachments:   []csp.VolumeAttachment{},
		Raw:           gceDisk,
	}
	assert.Equal(exp, ret)

	gceDisk.Name = "name"
	gceDisk.Type = "pd-ssd"
	gceDisk.SizeGb = 3
	gceDisk.Labels = map[string]string{"a": "b"}
	exp.Identifier = "gce:name"
	exp.StorageTypeName = "GCE pd-ssd"
	exp.SizeBytes = 3 * units.GiB
	exp.Tags = []string{"a:b"}
	exp.Type = "pd-ssd"

	tcs := []struct {
		status string
		state  csp.VolumeProvisioningState
		users  []string
	}{
		{status: "CREATING", state: csp.VolumeProvisioningProvisioning, users: []string{"a"}},
		{status: "RESTORING", state: csp.VolumeProvisioningProvisioning, users: []string{"z/b/c/a"}},
		{status: "READY", state: csp.VolumeProvisioningProvisioned, users: []string{"a", "a/b/c/d"}},
		{status: "DELETING", state: csp.VolumeProvisioningUnprovisioning, users: []string{"a"}},
		{status: "FAILED", state: csp.VolumeProvisioningError, users: []string{"a"}},
	}
	for _, tc := range tcs {
		gceDisk.Status = tc.status
		gceDisk.Users = tc.users
		exp.ProvisioningState = tc.state
		exp.Attachments = []csp.VolumeAttachment{
			csp.VolumeAttachment{NodeIdentifier: "a", Device: "/dev/disk/by-id/google-name", State: csp.VolumeAttachmentAttached},
		}
		if len(tc.users) > 1 {
			exp.Attachments = append(exp.Attachments, csp.VolumeAttachment{NodeIdentifier: "d", Device: "/dev/disk/by-id/google-name", State: csp.VolumeAttachmentAttached})
		}
		ret = gceDiskToVolume(gceDisk)
		assert.Equal(exp, ret)
		gceDisk.Type = "project/disks/pd-standard"
		exp.Type = "pd-standard"
		exp.StorageTypeName = "GCE pd-standard"
	}
}

// a matcher for the Disk Pages(ctx, function) function that calls the function with the given page
type diskPagesMatcher struct {
	page   *compute.DiskList
	calls  int
	retErr error
}

func newDiskPagesMatcher(page *compute.DiskList) *diskPagesMatcher {
	return &diskPagesMatcher{page: page}
}

func (p *diskPagesMatcher) Matches(x interface{}) bool {
	p.calls++
	f, ok := x.(func(*compute.DiskList) error)
	if ok {
		p.retErr = f(p.page)
	}
	return ok
}

func (p *diskPagesMatcher) String() string {
	return fmt.Sprintf("diskPagesMatcher matches")
}

type fakeVolumeRetriever struct {
	inCtx     context.Context
	inName    string
	retVolume *csp.Volume
	retErr    error
	inCnt     int
}

func (fvc *fakeVolumeRetriever) gceVolumeGet(ctx context.Context, name string) (*csp.Volume, error) {
	fvc.inCtx, fvc.inName = ctx, name
	fvc.inCnt++
	return fvc.retVolume, fvc.retErr
}
