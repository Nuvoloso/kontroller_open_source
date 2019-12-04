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


package azure

import (
	"context"
	"fmt"
	"testing"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockaz "github.com/Nuvoloso/kontroller/pkg/azuresdk/mock"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/alecthomas/units"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestAzureTagConversion(t *testing.T) {
	assert := assert.New(t)

	expVolTags := []string{
		"tag1:val1",
		"tag2",
		"tag3:val3",
	}
	expDiskTags := map[string]*string{
		"tag1": to.StringPtr("val1"),
		"tag2": to.StringPtr(""),
		"tag3": to.StringPtr("val3"),
	}

	assert.Equal(expDiskTags, azureVolToDiskTags(expVolTags))
	assert.Equal(expDiskTags, azureVolToDiskTags([]string{ // unsorted
		"tag2",
		"tag3:val3",
		"tag1:val1",
	}))
	assert.Nil(azureVolToDiskTags([]string{}))
	assert.Nil(azureVolToDiskTags(nil))

	assert.Equal(expVolTags, azureDiskToVolTags(expDiskTags))
	assert.Equal([]string{}, azureDiskToVolTags(map[string]*string{}))
	assert.Equal([]string{}, azureDiskToVolTags(nil))
}

func TestAzureVolumeFetch(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()

	azureCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(azureCSP)
	cAzure, ok := azureCSP.(*CSP)
	assert.True(ok)

	groupName := "resourceGroup"
	azureCl := &Client{
		csp: cAzure,
	}
	azureCl.attrs = make(map[string]models.ValueType)
	azureCl.attrs[AttrResourceGroupName] = models.ValueType{Kind: "STRING", Value: groupName}

	diskName := "nuvoloso-disk-XXXX"
	vfa := &csp.VolumeFetchArgs{
		VolumeIdentifier: diskName,
	}

	vmName := "aks-nodepool1-19665317-0"
	vm := compute.VirtualMachine{
		Name:                     to.StringPtr(vmName),
		VirtualMachineProperties: &compute.VirtualMachineProperties{},
		ID:                       to.StringPtr(fmt.Sprintf("/some/uri/to/%s", vmName)),
	}

	diskType, cspST := CspStorageTypeToDiskType(azureCspStorageTypes[0].Name)
	volSize := 1
	volSizeBytes := int64(volSize * int(units.GiB))
	expDisk := compute.Disk{
		Name:     to.StringPtr(diskName),
		Location: to.StringPtr("location"),
		Sku: &compute.DiskSku{
			Name: compute.DiskStorageAccountTypes(diskType),
		},
		DiskProperties: &compute.DiskProperties{
			CreationData: &compute.CreationData{
				CreateOption: compute.Empty,
			},
			DiskSizeGB:    to.Int32Ptr(int32(volSize)),
			DiskSizeBytes: to.Int64Ptr(volSizeBytes),
		},
		ID:        to.StringPtr(fmt.Sprintf("/some/url/to/%s", diskName)),
		ManagedBy: vm.ID,
		Tags: map[string]*string{
			"tag1": to.StringPtr("val1"),
		},
	}
	lun5 := int32(5)
	vm.VirtualMachineProperties.StorageProfile = &compute.StorageProfile{}
	vm.VirtualMachineProperties.StorageProfile.DataDisks = &[]compute.DataDisk{
		compute.DataDisk{
			Name:         expDisk.Name,
			Lun:          to.Int32Ptr(lun5),
			CreateOption: compute.DiskCreateOptionTypesAttach,
			DiskSizeGB:   expDisk.DiskSizeGB,
			ManagedDisk: &compute.ManagedDiskParameters{
				ID: expDisk.ID,
			},
		},
	}
	expVol := azureDiskToVolume(&expDisk, &vm)
	assert.NotNil(expVol)
	assert.EqualValues(CSPDomainType, expVol.CSPDomainType)
	assert.Equal(cspST.Name, expVol.StorageTypeName)
	assert.Equal(diskName, expVol.Identifier)
	assert.Equal(diskType, expVol.Type)
	assert.Equal(volSizeBytes, expVol.SizeBytes)
	assert.Equal(csp.VolumeProvisioningProvisioned, expVol.ProvisioningState)
	assert.Len(expVol.Attachments, 1)
	assert.Equal(vmName, expVol.Attachments[0].NodeIdentifier)
	assert.Equal(azureVMLunToDeviceName(&lun5), expVol.Attachments[0].Device)
	assert.Regexp("/dev/disk/azure/scsi1/lun5", expVol.Attachments[0].Device)
	assert.Equal(csp.VolumeAttachmentAttached, expVol.Attachments[0].State)
	assert.Len(expVol.Tags, 1)
	assert.Equal("tag1:val1", expVol.Tags[0])

	expVolNoVM := azureDiskToVolume(&expDisk, nil)
	assert.NotNil(expVolNoVM)
	assert.EqualValues(CSPDomainType, expVolNoVM.CSPDomainType)
	assert.Equal(cspST.Name, expVolNoVM.StorageTypeName)
	assert.Equal(diskName, expVolNoVM.Identifier)
	assert.Equal(diskType, expVolNoVM.Type)
	assert.Equal(volSizeBytes, expVolNoVM.SizeBytes)
	assert.Equal(csp.VolumeProvisioningProvisioned, expVolNoVM.ProvisioningState)
	assert.Len(expVolNoVM.Attachments, 0)

	t.Log("get api error")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mDC := mockaz.NewMockDC(mockCtrl)
	retErr := fmt.Errorf("api-err")
	mDC.EXPECT().Get(ctx, groupName, diskName).Return(compute.Disk{}, retErr)
	azureCl.disksClient = mDC
	vol, err := azureCl.VolumeFetch(ctx, vfa)
	assert.Error(err)
	assert.Equal(retErr, err)
	assert.Nil(vol)

	t.Log("get vmc error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mDC = mockaz.NewMockDC(mockCtrl)
	mVMC := mockaz.NewMockVMC(mockCtrl)
	mDC.EXPECT().Get(ctx, groupName, diskName).Return(expDisk, nil)
	mVMC.EXPECT().Get(ctx, groupName, vmName, compute.InstanceView).Return(compute.VirtualMachine{}, retErr)
	azureCl.disksClient = mDC
	azureCl.vmClient = mVMC
	vol, err = azureCl.VolumeFetch(ctx, vfa)
	assert.NoError(err)
	assert.Equal(expVolNoVM, vol)

	t.Log("get success")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mDC = mockaz.NewMockDC(mockCtrl)
	mVMC = mockaz.NewMockVMC(mockCtrl)
	mDC.EXPECT().Get(ctx, groupName, diskName).Return(expDisk, nil)
	mVMC.EXPECT().Get(ctx, groupName, vmName, compute.InstanceView).Return(vm, nil)
	azureCl.disksClient = mDC
	azureCl.vmClient = mVMC
	vol, err = azureCl.VolumeFetch(ctx, vfa)
	assert.NoError(err)
	assert.Equal(expVol, vol)
}

func TestAzureVolumeList(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()
	azureCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(azureCSP)
	cAzure, ok := azureCSP.(*CSP)
	assert.True(ok)

	groupName := "resourceGroup"
	azureCl := &Client{
		csp: cAzure,
	}
	azureCl.attrs = make(map[string]models.ValueType)
	azureCl.attrs[AttrResourceGroupName] = models.ValueType{Kind: "STRING", Value: groupName}

	vmName := "aks-nodepool1-19665317-0"
	vm := compute.VirtualMachine{
		Name:                     to.StringPtr(vmName),
		VirtualMachineProperties: &compute.VirtualMachineProperties{},
		ID:                       to.StringPtr(fmt.Sprintf("/some/uri/to/%s", vmName)),
	}

	diskName1 := "nuvoloso-disk-XXXX1"
	diskName2 := "nuvoloso-disk-XXXX2"
	diskType1, cspST1 := CspStorageTypeToDiskType(azureCspStorageTypes[0].Name)
	diskType2, cspST2 := CspStorageTypeToDiskType(azureCspStorageTypes[1].Name)
	volSize := 1
	volSizeBytes := int64(volSize * int(units.GiB))
	expDisk1 := compute.Disk{
		Name:     to.StringPtr(diskName1),
		Location: to.StringPtr("location"),
		Sku: &compute.DiskSku{
			Name: compute.DiskStorageAccountTypes(diskType1),
		},
		DiskProperties: &compute.DiskProperties{
			CreationData: &compute.CreationData{
				CreateOption: compute.Empty,
			},
			DiskSizeGB:    to.Int32Ptr(int32(volSize)),
			DiskSizeBytes: to.Int64Ptr(volSizeBytes),
		},
		ID:        to.StringPtr(fmt.Sprintf("/some/url/to/%s", diskName1)),
		ManagedBy: vm.ID,
		Tags: map[string]*string{
			"tag1": to.StringPtr("val1"),
			"tag2": to.StringPtr("val2-1"),
		},
	}
	expDisk2 := *cloneDisk(&expDisk1)
	assert.Equal(expDisk1, expDisk2) // validate cloning
	expDisk2.Name = to.StringPtr(diskName2)
	expDisk2.ID = to.StringPtr(fmt.Sprintf("/some/url/to/%s", diskName2))
	expDisk2.Sku.Name = compute.DiskStorageAccountTypes(diskType2)
	expDisk2.Tags["tag2"] = to.StringPtr("val2-2")
	expDisk2.Tags["tag3"] = to.StringPtr("val3")

	lun3 := int32(3)
	lun5 := int32(5)
	vm.VirtualMachineProperties.StorageProfile = &compute.StorageProfile{}
	vm.VirtualMachineProperties.StorageProfile.DataDisks = &[]compute.DataDisk{
		compute.DataDisk{
			Name:         expDisk1.Name,
			Lun:          to.Int32Ptr(lun5),
			CreateOption: compute.DiskCreateOptionTypesAttach,
			DiskSizeGB:   expDisk1.DiskSizeGB,
			ManagedDisk: &compute.ManagedDiskParameters{
				ID: expDisk1.ID,
			},
		},
		compute.DataDisk{
			Name:         expDisk2.Name,
			Lun:          to.Int32Ptr(lun3),
			CreateOption: compute.DiskCreateOptionTypesAttach,
			DiskSizeGB:   expDisk2.DiskSizeGB,
			ManagedDisk: &compute.ManagedDiskParameters{
				ID: expDisk2.ID,
			},
		},
	}

	expVol1 := azureDiskToVolume(&expDisk1, &vm)
	expVol2 := azureDiskToVolume(&expDisk2, &vm)

	vla := &csp.VolumeListArgs{}

	t.Log("list api error")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mDC := mockaz.NewMockDC(mockCtrl)
	mVMC := mockaz.NewMockVMC(mockCtrl)
	retErr := fmt.Errorf("api-err")
	mDC.EXPECT().ListByResourceGroupComplete(ctx, groupName).Return(nil, retErr)
	azureCl.disksClient = mDC
	azureCl.vmClient = mVMC
	vl, err := azureCl.VolumeList(ctx, vla)
	assert.Error(err)
	assert.Equal(retErr, err)
	assert.Nil(vl)

	t.Log("list iter error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mDC = mockaz.NewMockDC(mockCtrl)
	mVMC = mockaz.NewMockVMC(mockCtrl)
	mLF := mockaz.NewMockDCListIterator(mockCtrl)
	mLF.EXPECT().NotDone().Return(true)
	mLF.EXPECT().Value().Return(expDisk1)
	mLF.EXPECT().NextWithContext(ctx).Return(retErr)
	mDC.EXPECT().ListByResourceGroupComplete(ctx, groupName).Return(mLF, nil)
	mVMC.EXPECT().Get(ctx, groupName, vmName, compute.InstanceView).Return(vm, nil)
	azureCl.disksClient = mDC
	azureCl.vmClient = mVMC
	vl, err = azureCl.VolumeList(ctx, vla)
	assert.Error(err)
	assert.Equal(retErr, err)
	assert.Nil(vl)

	t.Log("list success")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mDC = mockaz.NewMockDC(mockCtrl)
	mVMC = mockaz.NewMockVMC(mockCtrl)
	mLF = mockaz.NewMockDCListIterator(mockCtrl)
	mLF.EXPECT().NotDone().Return(true).Times(2)
	mLF.EXPECT().NotDone().Return(false)
	mLF.EXPECT().Value().Return(expDisk1)
	mVMC.EXPECT().Get(ctx, groupName, vmName, compute.InstanceView).Return(vm, nil)
	mLF.EXPECT().Value().Return(expDisk2)
	mLF.EXPECT().NextWithContext(ctx).Return(nil).Times(2)
	mDC.EXPECT().ListByResourceGroupComplete(ctx, groupName).Return(mLF, nil)
	azureCl.disksClient = mDC
	azureCl.vmClient = mVMC
	vl, err = azureCl.VolumeList(ctx, vla)
	assert.NoError(err)
	assert.Equal([]*csp.Volume{expVol1, expVol2}, vl)

	t.Log("list filter by type")
	mockCtrl.Finish()
	vla.StorageTypeName = cspST2.Name
	mockCtrl = gomock.NewController(t)
	mDC = mockaz.NewMockDC(mockCtrl)
	mVMC = mockaz.NewMockVMC(mockCtrl)
	mLF = mockaz.NewMockDCListIterator(mockCtrl)
	mLF.EXPECT().NotDone().Return(true).Times(2)
	mLF.EXPECT().NotDone().Return(false)
	mLF.EXPECT().Value().Return(expDisk1)
	mVMC.EXPECT().Get(ctx, groupName, vmName, compute.InstanceView).Return(vm, nil)
	mLF.EXPECT().Value().Return(expDisk2)
	mLF.EXPECT().NextWithContext(ctx).Return(nil).Times(2)
	mDC.EXPECT().ListByResourceGroupComplete(ctx, groupName).Return(mLF, nil)
	azureCl.disksClient = mDC
	azureCl.vmClient = mVMC
	vl, err = azureCl.VolumeList(ctx, vla)
	assert.NoError(err)
	assert.Equal([]*csp.Volume{expVol2}, vl)
	for _, vol := range vl {
		assert.NotEqual(cspST1.Name, vol.StorageTypeName)
		assert.Equal(cspST2.Name, vol.StorageTypeName)
	}

	vla.StorageTypeName = ""
	tagTCs := []struct {
		filter []string
		exp    []*csp.Volume
	}{
		{[]string{"tag2:val2-1"}, []*csp.Volume{expVol1}},
		{[]string{"tag2:val2-2"}, []*csp.Volume{expVol2}},
		{[]string{"tag3:val3"}, []*csp.Volume{expVol2}},
		{[]string{"tag1:val1"}, []*csp.Volume{expVol1, expVol2}},
		{[]string{"tag1:val1", "tag3:val3"}, []*csp.Volume{expVol2}},
		{[]string{}, []*csp.Volume{expVol1, expVol2}},
		{nil, []*csp.Volume{expVol1, expVol2}},
		{[]string{"tag1:val1", "tag3:val4"}, []*csp.Volume{}},
	}
	for i, tc := range tagTCs {
		t.Log("list filter by tags:", i, tc.filter)
		vla.Tags = tc.filter
		mockCtrl.Finish()
		mockCtrl = gomock.NewController(t)
		mDC = mockaz.NewMockDC(mockCtrl)
		mVMC = mockaz.NewMockVMC(mockCtrl)
		mLF = mockaz.NewMockDCListIterator(mockCtrl)
		mLF.EXPECT().NotDone().Return(true).Times(2)
		mLF.EXPECT().NotDone().Return(false)
		mLF.EXPECT().Value().Return(expDisk1)
		if len(tc.exp) > 0 {
			mVMC.EXPECT().Get(ctx, groupName, vmName, compute.InstanceView).Return(vm, nil)
		}
		mLF.EXPECT().Value().Return(expDisk2)
		mLF.EXPECT().NextWithContext(ctx).Return(nil).Times(2)
		mDC.EXPECT().ListByResourceGroupComplete(ctx, groupName).Return(mLF, nil)
		azureCl.disksClient = mDC
		azureCl.vmClient = mVMC
		vl, err = azureCl.VolumeList(ctx, vla)
		assert.NoError(err)
		assert.Equal(tc.exp, vl, "case %d", i)
	}
}

func TestAzureVolumeCreate(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()
	azureCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(azureCSP)
	cAzure, ok := azureCSP.(*CSP)
	assert.True(ok)

	groupName := "resourceGroup"
	location := "location"
	azureCl := &Client{
		csp: cAzure,
	}
	azureCl.attrs = make(map[string]models.ValueType)
	azureCl.attrs[AttrResourceGroupName] = models.ValueType{Kind: "STRING", Value: groupName}
	azureCl.attrs[AttrLocation] = models.ValueType{Kind: "STRING", Value: location}

	vca := &csp.VolumeCreateArgs{}
	vol, err := azureCl.VolumeCreate(ctx, vca)
	assert.Nil(vol)
	assert.NotNil(err)

	// invalid storage type
	vca.StorageTypeName = "foo"
	vol, err = azureCl.VolumeCreate(ctx, vca)
	assert.Nil(vol)
	assert.Error(err)
	assert.Regexp("invalid storage type", err)

	savedUUIDGen := uuidGenerator
	defer func() {
		uuidGenerator = savedUUIDGen
	}()
	uuidGenerator = func() string { return "UUID" }
	diskName := newResourceName("disk")
	assert.Equal("nuvoloso-disk-UUID", diskName)

	// setup for mocking
	vca.StorageTypeName = azureCspStorageTypes[0].Name
	vca.SizeBytes = int64(units.GiB) - 1
	vca.Tags = []string{
		"tag1:val1",
	}
	diskType, _ := CspStorageTypeToDiskType(vca.StorageTypeName)
	volSize := 1
	expDisk := compute.Disk{
		Location: to.StringPtr(location),
		Sku: &compute.DiskSku{
			Name: compute.DiskStorageAccountTypes(diskType),
		},
		DiskProperties: &compute.DiskProperties{
			CreationData: &compute.CreationData{
				CreateOption: compute.Empty,
			},
			DiskSizeGB: to.Int32Ptr(int32(volSize)),
		},
		Tags: map[string]*string{
			"tag1": to.StringPtr("val1"),
		},
	}
	expVol := azureDiskToVolume(&expDisk, nil)

	t.Log("create api error")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mDC := mockaz.NewMockDC(mockCtrl)
	retErr := fmt.Errorf("api-err")
	diskM := mockaz.NewDiskMatcher(t, expDisk)
	mDC.EXPECT().CreateOrUpdate(ctx, groupName, diskName, diskM).Return(nil, retErr)
	azureCl.disksClient = mDC
	vol, err = azureCl.VolumeCreate(ctx, vca)
	assert.Error(err)
	assert.Equal(retErr, err)
	assert.Nil(vol)

	t.Log("create future wait error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mDF := mockaz.NewMockDCCreateOrUpdateFuture(mockCtrl)
	mDC = mockaz.NewMockDC(mockCtrl)
	mDF.EXPECT().WaitForCompletionRef(ctx, mDC).Return(retErr)
	mDC.EXPECT().CreateOrUpdate(ctx, groupName, diskName, diskM).Return(mDF, nil)
	azureCl.disksClient = mDC
	vol, err = azureCl.VolumeCreate(ctx, vca)
	assert.Error(err)
	assert.Equal(retErr, err)
	assert.Nil(vol)

	t.Log("create future result error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mDF = mockaz.NewMockDCCreateOrUpdateFuture(mockCtrl)
	mDC = mockaz.NewMockDC(mockCtrl)
	mDF.EXPECT().WaitForCompletionRef(ctx, mDC).Return(nil)
	mDF.EXPECT().Result(mDC).Return(compute.Disk{}, retErr)
	mDC.EXPECT().CreateOrUpdate(ctx, groupName, diskName, diskM).Return(mDF, nil)
	azureCl.disksClient = mDC
	vol, err = azureCl.VolumeCreate(ctx, vca)
	assert.Error(err)
	assert.Equal(retErr, err)
	assert.Nil(vol)

	t.Log("create success")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mDF = mockaz.NewMockDCCreateOrUpdateFuture(mockCtrl)
	mDC = mockaz.NewMockDC(mockCtrl)
	mDF.EXPECT().WaitForCompletionRef(ctx, mDC).Return(nil)
	mDF.EXPECT().Result(mDC).Return(expDisk, nil)
	mDC.EXPECT().CreateOrUpdate(ctx, groupName, diskName, diskM).Return(mDF, nil)
	azureCl.disksClient = mDC
	vol, err = azureCl.VolumeCreate(ctx, vca)
	assert.NoError(err)
	assert.Equal(expVol, vol)
}

func TestAzureVolumeTagSet(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()
	azureCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(azureCSP)
	cAzure, ok := azureCSP.(*CSP)
	assert.True(ok)

	azureCl := &Client{
		csp: cAzure,
	}
	vta := &csp.VolumeTagArgs{}
	vol, err := azureCl.VolumeTagsSet(ctx, vta)
	assert.Nil(vol)
	assert.NotNil(err)
}

func TestAzureVolumeTagDelete(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()
	azureCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(azureCSP)
	cAzure, ok := azureCSP.(*CSP)
	assert.True(ok)

	azureCl := &Client{
		csp: cAzure,
	}
	vta := &csp.VolumeTagArgs{}
	vol, err := azureCl.VolumeTagsDelete(ctx, vta)
	assert.Nil(vol)
	assert.Nil(err) // TBD: no-op for now
}

func TestAzureVolumeSize(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()
	cspSTFooService := models.CspStorageType("Azure Fake ST")
	azureCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(azureCSP)
	cAzure, ok := azureCSP.(*CSP)
	assert.True(ok)

	azureCl := &Client{
		csp: cAzure,
	}
	sizeBytes, err := azureCl.VolumeSize(ctx, cspSTFooService, 1)
	assert.Zero(sizeBytes)
	assert.NotNil(err)

	stN := azureCspStorageTypes[0].Name
	stMinSizeBytes := swag.Int64Value(azureCspStorageTypes[0].MinAllocationSizeBytes)
	stMaxSizeBytes := swag.Int64Value(azureCspStorageTypes[0].MaxAllocationSizeBytes)

	sizeBytes, err = azureCl.VolumeSize(ctx, stN, -1)
	assert.Error(err)
	assert.Zero(sizeBytes)

	sizeBytes, err = azureCl.VolumeSize(ctx, stN, 0)
	assert.NoError(err)
	assert.Equal(stMinSizeBytes, sizeBytes)

	sizeBytes, err = azureCl.VolumeSize(ctx, stN, stMaxSizeBytes+1)
	assert.Error(err)

	sizeBytes, err = azureCl.VolumeSize(ctx, stN, 2*stMinSizeBytes+1)
	assert.NoError(err)
	assert.Equal(3*stMinSizeBytes, sizeBytes)
}

func TestAzureVolumeAttach(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()
	azureCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(azureCSP)
	cAzure, ok := azureCSP.(*CSP)
	assert.True(ok)

	groupName := "resourceGroup"
	azureCl := &Client{
		csp: cAzure,
	}
	azureCl.attrs = make(map[string]models.ValueType)
	azureCl.attrs[AttrResourceGroupName] = models.ValueType{Kind: "STRING", Value: groupName}

	diskName := "nuvoloso-disk-XXXX"
	diskType, cspST := CspStorageTypeToDiskType(azureCspStorageTypes[0].Name)
	volSize := 1
	volSizeBytes := int64(volSize * int(units.GiB))
	expDisk := compute.Disk{ // TBD: handle before/after
		Name:     to.StringPtr(diskName),
		ID:       to.StringPtr("ID"),
		Location: to.StringPtr("location"),
		Sku: &compute.DiskSku{
			Name: compute.DiskStorageAccountTypes(diskType),
		},
		DiskProperties: &compute.DiskProperties{
			CreationData: &compute.CreationData{
				CreateOption: compute.Empty,
			},
			DiskSizeGB:    to.Int32Ptr(int32(volSize)),
			DiskSizeBytes: to.Int64Ptr(volSizeBytes),
		},
		// TBD: Handle tags
	}

	lun0 := int32(0) // unlike swag, to requires a variable
	lun1 := int32(1)
	dl0 := []compute.DataDisk{
		compute.DataDisk{
			Name: to.StringPtr("disk-at-lun0"),
			Lun:  to.Int32Ptr(lun0), // used
		},
	}
	dl1 := []compute.DataDisk{
		dl0[0],
		compute.DataDisk{
			Name:         expDisk.Name,
			Lun:          to.Int32Ptr(lun1),
			CreateOption: compute.DiskCreateOptionTypesAttach,
			DiskSizeGB:   expDisk.DiskSizeGB,
			ManagedDisk: &compute.ManagedDiskParameters{
				ID: expDisk.ID,
			},
		},
	}
	dlMax := []compute.DataDisk{}
	for i := 0; i < MaxLunsPerVM; i++ {
		dlMax = append(dlMax, compute.DataDisk{
			Name: to.StringPtr(fmt.Sprintf("disk-%d", i)),
			Lun:  to.Int32Ptr(int32(i)),
		})
	}

	vmName := "aks-nodepool1-19665317-0"
	vmB := compute.VirtualMachine{
		Name:                     to.StringPtr(vmName),
		VirtualMachineProperties: &compute.VirtualMachineProperties{},
		ID:                       to.StringPtr(fmt.Sprintf("/some/uri/to/%s", vmName)),
	}
	vmB.VirtualMachineProperties.StorageProfile = &compute.StorageProfile{}
	vmB.VirtualMachineProperties.StorageProfile.DataDisks = &dl0
	assert.Equal(vmName, azureDiskManagedByToVMName(vmB.ID))

	vmA := *cloneVM(&vmB)
	assert.Equal(vmB, vmA) // validate cloning worked
	vmA.StorageProfile.DataDisks = &dl1
	assert.NotEqual(vmB, vmA)
	vmMax := *cloneVM(&vmB)
	vmMax.StorageProfile.DataDisks = &dlMax
	assert.NotEqual(vmB, vmMax)

	expDiskA := *cloneDisk(&expDisk)
	assert.Equal(expDisk, expDiskA) // validate cloning worked
	expDiskA.ManagedBy = vmB.ID

	expVol := azureDiskToVolume(&expDiskA, &vmA)
	assert.NotNil(expVol)
	assert.EqualValues(CSPDomainType, expVol.CSPDomainType)
	assert.Equal(cspST.Name, expVol.StorageTypeName)
	assert.Equal(diskName, expVol.Identifier)
	assert.Equal(diskType, expVol.Type)
	assert.Equal(volSizeBytes, expVol.SizeBytes)
	assert.Equal(csp.VolumeProvisioningProvisioned, expVol.ProvisioningState)
	assert.Len(expVol.Attachments, 1)
	assert.Equal(vmName, expVol.Attachments[0].NodeIdentifier)
	assert.Equal(azureVMLunToDeviceName(&lun1), expVol.Attachments[0].Device)
	assert.Regexp("/dev/disk/azure/scsi1/lun1", expVol.Attachments[0].Device)
	assert.Equal(csp.VolumeAttachmentAttached, expVol.Attachments[0].State)

	vaa := &csp.VolumeAttachArgs{
		VolumeIdentifier: diskName,
		NodeIdentifier:   vmName,
	}

	t.Log("attach dc.get error")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mDC := mockaz.NewMockDC(mockCtrl)
	mVMC := mockaz.NewMockVMC(mockCtrl)
	retErr := fmt.Errorf("api-err")
	mDC.EXPECT().Get(ctx, groupName, diskName).Return(compute.Disk{}, retErr)
	azureCl.disksClient = mDC
	azureCl.vmClient = mVMC
	vol, err := azureCl.VolumeAttach(ctx, vaa)
	assert.Error(err)
	assert.Equal(retErr, err)
	assert.Nil(vol)

	t.Log("attach vmc.get err")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mDC = mockaz.NewMockDC(mockCtrl)
	mVMC = mockaz.NewMockVMC(mockCtrl)
	mDC.EXPECT().Get(ctx, groupName, diskName).Return(expDisk, nil)
	mVMC.EXPECT().Get(ctx, groupName, vmName, compute.InstanceView).Return(compute.VirtualMachine{}, retErr)
	azureCl.disksClient = mDC
	azureCl.vmClient = mVMC
	vol, err = azureCl.VolumeAttach(ctx, vaa)
	assert.Error(err)
	assert.Equal(retErr, err)
	assert.Nil(vol)

	t.Log("attach no more luns err")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mDC = mockaz.NewMockDC(mockCtrl)
	mVMC = mockaz.NewMockVMC(mockCtrl)
	mDC.EXPECT().Get(ctx, groupName, diskName).Return(expDisk, nil)
	mVMC.EXPECT().Get(ctx, groupName, vmName, compute.InstanceView).Return(vmMax, nil)
	azureCl.disksClient = mDC
	azureCl.vmClient = mVMC
	vol, err = azureCl.VolumeAttach(ctx, vaa)
	assert.Error(err)
	assert.Regexp("no more LUNs", err)
	assert.Nil(vol)

	t.Log("attach vmc.update err")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mDC = mockaz.NewMockDC(mockCtrl)
	mVMC = mockaz.NewMockVMC(mockCtrl)
	mDC.EXPECT().Get(ctx, groupName, diskName).Return(expDisk, nil)
	mVMC.EXPECT().Get(ctx, groupName, vmName, compute.InstanceView).Return(*cloneVM(&vmB), nil)
	vmM := mockaz.NewVirtualMachineMatcher(t, vmA)
	mVMC.EXPECT().CreateOrUpdate(ctx, groupName, vmName, vmM).Return(nil, retErr)
	azureCl.disksClient = mDC
	azureCl.vmClient = mVMC
	vol, err = azureCl.VolumeAttach(ctx, vaa)
	assert.Error(err)
	assert.Equal(retErr, err)
	assert.Nil(vol)

	t.Log("attach vmc.update future wait err")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mDC = mockaz.NewMockDC(mockCtrl)
	mVMC = mockaz.NewMockVMC(mockCtrl)
	mVMF := mockaz.NewMockVMCreateOrUpdateFuture(mockCtrl)
	mDC.EXPECT().Get(ctx, groupName, diskName).Return(expDisk, nil)
	mVMC.EXPECT().Get(ctx, groupName, vmName, compute.InstanceView).Return(*cloneVM(&vmB), nil)
	vmM = mockaz.NewVirtualMachineMatcher(t, vmA)
	mVMC.EXPECT().CreateOrUpdate(ctx, groupName, vmName, vmM).Return(mVMF, nil)
	mVMF.EXPECT().WaitForCompletionRef(ctx, mVMC).Return(retErr)
	azureCl.disksClient = mDC
	azureCl.vmClient = mVMC
	vol, err = azureCl.VolumeAttach(ctx, vaa)
	assert.Error(err)
	assert.Equal(retErr, err)
	assert.Nil(vol)

	t.Log("attach vmc.update future result err")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mDC = mockaz.NewMockDC(mockCtrl)
	mVMC = mockaz.NewMockVMC(mockCtrl)
	mVMF = mockaz.NewMockVMCreateOrUpdateFuture(mockCtrl)
	mDC.EXPECT().Get(ctx, groupName, diskName).Return(expDisk, nil)
	mVMC.EXPECT().Get(ctx, groupName, vmName, compute.InstanceView).Return(*cloneVM(&vmB), nil)
	vmM = mockaz.NewVirtualMachineMatcher(t, vmA)
	mVMC.EXPECT().CreateOrUpdate(ctx, groupName, vmName, vmM).Return(mVMF, nil)
	mVMF.EXPECT().WaitForCompletionRef(ctx, mVMC).Return(nil)
	mVMF.EXPECT().Result(mVMC).Return(vmA, retErr)
	azureCl.disksClient = mDC
	azureCl.vmClient = mVMC
	vol, err = azureCl.VolumeAttach(ctx, vaa)
	assert.Error(err)
	assert.Equal(retErr, err)
	assert.Nil(vol)

	t.Log("attach success but get fails")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mDC = mockaz.NewMockDC(mockCtrl)
	mVMC = mockaz.NewMockVMC(mockCtrl)
	mVMF = mockaz.NewMockVMCreateOrUpdateFuture(mockCtrl)
	mDC.EXPECT().Get(ctx, groupName, diskName).Return(expDisk, nil)
	mVMC.EXPECT().Get(ctx, groupName, vmName, compute.InstanceView).Return(*cloneVM(&vmB), nil)
	vmM = mockaz.NewVirtualMachineMatcher(t, vmA)
	mVMC.EXPECT().CreateOrUpdate(ctx, groupName, vmName, vmM).Return(mVMF, nil)
	mVMF.EXPECT().WaitForCompletionRef(ctx, mVMC).Return(nil)
	mVMF.EXPECT().Result(mVMC).Return(vmA, nil)
	mDC.EXPECT().Get(ctx, groupName, diskName).Return(compute.Disk{}, retErr)
	azureCl.disksClient = mDC
	azureCl.vmClient = mVMC
	vol, err = azureCl.VolumeAttach(ctx, vaa)
	assert.Error(err)
	assert.Equal(retErr, err)
	assert.Nil(vol)

	t.Log("attach success")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mDC = mockaz.NewMockDC(mockCtrl)
	mVMC = mockaz.NewMockVMC(mockCtrl)
	mVMF = mockaz.NewMockVMCreateOrUpdateFuture(mockCtrl)
	mDC.EXPECT().Get(ctx, groupName, diskName).Return(expDisk, nil)
	mVMC.EXPECT().Get(ctx, groupName, vmName, compute.InstanceView).Return(*cloneVM(&vmB), nil)
	vmM = mockaz.NewVirtualMachineMatcher(t, vmA)
	mVMC.EXPECT().CreateOrUpdate(ctx, groupName, vmName, vmM).Return(mVMF, nil)
	mVMF.EXPECT().WaitForCompletionRef(ctx, mVMC).Return(nil)
	mVMF.EXPECT().Result(mVMC).Return(vmA, nil)
	mDC.EXPECT().Get(ctx, groupName, diskName).Return(expDiskA, nil)
	azureCl.disksClient = mDC
	azureCl.vmClient = mVMC
	vol, err = azureCl.VolumeAttach(ctx, vaa)
	assert.NoError(err)
	assert.Equal(expVol, vol)
}

func TestAzureVolumeDelete(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()
	azureCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(azureCSP)
	cAzure, ok := azureCSP.(*CSP)
	assert.True(ok)

	groupName := "resourceGroup"
	azureCl := &Client{
		csp: cAzure,
	}
	azureCl.attrs = make(map[string]models.ValueType)
	azureCl.attrs[AttrResourceGroupName] = models.ValueType{Kind: "STRING", Value: groupName}

	t.Log("delete api error")
	diskName := "nuvoloso-disk-XXXX"
	vda := &csp.VolumeDeleteArgs{
		VolumeIdentifier: diskName,
	}

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mDC := mockaz.NewMockDC(mockCtrl)
	retErr := fmt.Errorf("api-err")
	mDC.EXPECT().Delete(ctx, groupName, diskName).Return(nil, retErr)
	azureCl.disksClient = mDC
	err = azureCl.VolumeDelete(ctx, vda)
	assert.Error(err)
	assert.Equal(retErr, err)

	t.Log("create future wait error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mDF := mockaz.NewMockDCDisksDeleteFuture(mockCtrl)
	mDC = mockaz.NewMockDC(mockCtrl)
	mDF.EXPECT().WaitForCompletionRef(ctx, mDC).Return(retErr)
	mDC.EXPECT().Delete(ctx, groupName, diskName).Return(mDF, nil)
	azureCl.disksClient = mDC
	err = azureCl.VolumeDelete(ctx, vda)
	assert.Error(err)
	assert.Equal(retErr, err)

	t.Log("create future result error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mDF = mockaz.NewMockDCDisksDeleteFuture(mockCtrl)
	mDC = mockaz.NewMockDC(mockCtrl)
	mDF.EXPECT().WaitForCompletionRef(ctx, mDC).Return(nil)
	mDF.EXPECT().Result(mDC).Return(autorest.Response{}, retErr)
	mDC.EXPECT().Delete(ctx, groupName, diskName).Return(mDF, nil)
	azureCl.disksClient = mDC
	err = azureCl.VolumeDelete(ctx, vda)
	assert.Error(err)
	assert.Equal(retErr, err)

	t.Log("create success")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mDF = mockaz.NewMockDCDisksDeleteFuture(mockCtrl)
	mDC = mockaz.NewMockDC(mockCtrl)
	mDF.EXPECT().WaitForCompletionRef(ctx, mDC).Return(nil)
	mDF.EXPECT().Result(mDC).Return(autorest.Response{}, nil)
	mDC.EXPECT().Delete(ctx, groupName, diskName).Return(mDF, nil)
	azureCl.disksClient = mDC
	err = azureCl.VolumeDelete(ctx, vda)
	assert.NoError(err)
}

func TestAzureVolumeDetach(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()
	azureCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(azureCSP)
	cAzure, ok := azureCSP.(*CSP)
	assert.True(ok)

	groupName := "resourceGroup"
	azureCl := &Client{
		csp: cAzure,
	}
	azureCl.attrs = make(map[string]models.ValueType)
	azureCl.attrs[AttrResourceGroupName] = models.ValueType{Kind: "STRING", Value: groupName}

	diskName := "nuvoloso-disk-XXXX"
	diskType, _ := CspStorageTypeToDiskType(azureCspStorageTypes[0].Name)
	volSize := 1
	expDisk := compute.Disk{ // fetched after detached so not managed
		Name:     to.StringPtr(diskName),
		ID:       to.StringPtr("ID"),
		Location: to.StringPtr("location"),
		Sku: &compute.DiskSku{
			Name: compute.DiskStorageAccountTypes(diskType),
		},
		DiskProperties: &compute.DiskProperties{
			CreationData: &compute.CreationData{
				CreateOption: compute.Empty,
			},
			DiskSizeGB: to.Int32Ptr(int32(volSize)),
		},
		// TBD: Handle tags
	}
	expVol := azureDiskToVolume(&expDisk, nil)
	assert.NotNil(expVol)

	lun0 := int32(0) // unlike swag, to requires a variable
	lun1 := int32(1)
	lun2 := int32(2)
	dl0 := []compute.DataDisk{
		compute.DataDisk{
			Name: to.StringPtr("disk-at-lun0"),
			Lun:  to.Int32Ptr(lun0), // used
		},
		compute.DataDisk{
			Name:         expDisk.Name,
			Lun:          to.Int32Ptr(lun1),
			CreateOption: compute.DiskCreateOptionTypesAttach,
			DiskSizeGB:   expDisk.DiskSizeGB,
			ManagedDisk: &compute.ManagedDiskParameters{
				ID: expDisk.ID,
			},
		},
		compute.DataDisk{
			Name: to.StringPtr("disk-at-lun2"),
			Lun:  to.Int32Ptr(lun2), // used
		},
	}
	dl1 := []compute.DataDisk{
		dl0[0],
		dl0[2],
	}
	vmName := "aks-nodepool1-19665317-0"
	vmB := compute.VirtualMachine{
		Name:                     to.StringPtr(vmName),
		VirtualMachineProperties: &compute.VirtualMachineProperties{},
	}
	vmB.VirtualMachineProperties.StorageProfile = &compute.StorageProfile{}
	vmB.VirtualMachineProperties.StorageProfile.DataDisks = &dl0
	vmA := *cloneVM(&vmB)
	assert.Equal(vmB, vmA) // catch props that don't get cloned
	vmA.StorageProfile.DataDisks = &dl1
	assert.NotEqual(vmB, vmA)

	vda := &csp.VolumeDetachArgs{
		VolumeIdentifier: diskName,
		NodeIdentifier:   vmName,
	}

	t.Log("detach vmc.get error")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mDC := mockaz.NewMockDC(mockCtrl)
	mVMC := mockaz.NewMockVMC(mockCtrl)
	retErr := fmt.Errorf("api-err")
	mVMC.EXPECT().Get(ctx, groupName, vmName, compute.InstanceView).Return(compute.VirtualMachine{}, retErr)
	azureCl.disksClient = mDC
	azureCl.vmClient = mVMC
	vol, err := azureCl.VolumeDetach(ctx, vda)
	assert.Error(err)
	assert.Equal(retErr, err)
	assert.Nil(vol)

	t.Log("detach not found")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mDC = mockaz.NewMockDC(mockCtrl)
	mVMC = mockaz.NewMockVMC(mockCtrl)
	mVMC.EXPECT().Get(ctx, groupName, vmName, compute.InstanceView).Return(*cloneVM(&vmA), nil)
	azureCl.disksClient = mDC
	azureCl.vmClient = mVMC
	vol, err = azureCl.VolumeDetach(ctx, vda)
	assert.Equal(csp.ErrorVolumeNotAttached, err)
	assert.Nil(vol)

	t.Log("detach vmc.update err")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mDC = mockaz.NewMockDC(mockCtrl)
	mVMC = mockaz.NewMockVMC(mockCtrl)
	mVMC.EXPECT().Get(ctx, groupName, vmName, compute.InstanceView).Return(*cloneVM(&vmB), nil)
	vmM := mockaz.NewVirtualMachineMatcher(t, vmA)
	mVMC.EXPECT().CreateOrUpdate(ctx, groupName, vmName, vmM).Return(nil, retErr)
	azureCl.disksClient = mDC
	azureCl.vmClient = mVMC
	vol, err = azureCl.VolumeDetach(ctx, vda)
	assert.Error(err)
	assert.Equal(retErr, err)
	assert.Nil(vol)

	t.Log("detach vmc.update future wait err")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mDC = mockaz.NewMockDC(mockCtrl)
	mVMC = mockaz.NewMockVMC(mockCtrl)
	mVMF := mockaz.NewMockVMCreateOrUpdateFuture(mockCtrl)
	mVMC.EXPECT().Get(ctx, groupName, vmName, compute.InstanceView).Return(*cloneVM(&vmB), nil)
	vmM = mockaz.NewVirtualMachineMatcher(t, vmA)
	mVMC.EXPECT().CreateOrUpdate(ctx, groupName, vmName, vmM).Return(mVMF, nil)
	mVMF.EXPECT().WaitForCompletionRef(ctx, mVMC).Return(retErr)
	azureCl.disksClient = mDC
	azureCl.vmClient = mVMC
	vol, err = azureCl.VolumeDetach(ctx, vda)
	assert.Error(err)
	assert.Equal(retErr, err)
	assert.Nil(vol)

	t.Log("detach vmc.update future result err")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mDC = mockaz.NewMockDC(mockCtrl)
	mVMC = mockaz.NewMockVMC(mockCtrl)
	mVMF = mockaz.NewMockVMCreateOrUpdateFuture(mockCtrl)
	mVMC.EXPECT().Get(ctx, groupName, vmName, compute.InstanceView).Return(*cloneVM(&vmB), nil)
	vmM = mockaz.NewVirtualMachineMatcher(t, vmA)
	mVMC.EXPECT().CreateOrUpdate(ctx, groupName, vmName, vmM).Return(mVMF, nil)
	mVMF.EXPECT().WaitForCompletionRef(ctx, mVMC).Return(nil)
	mVMF.EXPECT().Result(mVMC).Return(vmA, retErr)
	azureCl.disksClient = mDC
	azureCl.vmClient = mVMC
	vol, err = azureCl.VolumeDetach(ctx, vda)
	assert.Error(err)
	assert.Equal(retErr, err)
	assert.Nil(vol)

	t.Log("detach dc.get err")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mDC = mockaz.NewMockDC(mockCtrl)
	mVMC = mockaz.NewMockVMC(mockCtrl)
	mVMF = mockaz.NewMockVMCreateOrUpdateFuture(mockCtrl)
	mVMC.EXPECT().Get(ctx, groupName, vmName, compute.InstanceView).Return(*cloneVM(&vmB), nil)
	vmM = mockaz.NewVirtualMachineMatcher(t, vmA)
	mVMC.EXPECT().CreateOrUpdate(ctx, groupName, vmName, vmM).Return(mVMF, nil)
	mVMF.EXPECT().WaitForCompletionRef(ctx, mVMC).Return(nil)
	mVMF.EXPECT().Result(mVMC).Return(vmA, nil)
	mDC.EXPECT().Get(ctx, groupName, diskName).Return(compute.Disk{}, retErr)
	azureCl.disksClient = mDC
	azureCl.vmClient = mVMC
	vol, err = azureCl.VolumeDetach(ctx, vda)
	assert.Error(err)
	assert.Equal(retErr, err)
	assert.Nil(vol)

	t.Log("detach success")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mDC = mockaz.NewMockDC(mockCtrl)
	mVMC = mockaz.NewMockVMC(mockCtrl)
	mVMF = mockaz.NewMockVMCreateOrUpdateFuture(mockCtrl)
	mVMC.EXPECT().Get(ctx, groupName, vmName, compute.InstanceView).Return(*cloneVM(&vmB), nil)
	vmM = mockaz.NewVirtualMachineMatcher(t, vmA)
	mVMC.EXPECT().CreateOrUpdate(ctx, groupName, vmName, vmM).Return(mVMF, nil)
	mVMF.EXPECT().WaitForCompletionRef(ctx, mVMC).Return(nil)
	mVMF.EXPECT().Result(mVMC).Return(vmA, nil)
	mDC.EXPECT().Get(ctx, groupName, diskName).Return(expDisk, nil)
	azureCl.disksClient = mDC
	azureCl.vmClient = mVMC
	vol, err = azureCl.VolumeDetach(ctx, vda)
	assert.NoError(err)
	assert.Equal(expVol, vol)
}

func cloneVM(s *compute.VirtualMachine) *compute.VirtualMachine {
	var n *compute.VirtualMachine
	testutils.Clone(&s, &n)
	// luns not getting cloned
	sDisks := *s.StorageProfile.DataDisks
	nDisks := *n.StorageProfile.DataDisks
	for i, disk := range sDisks {
		if disk.Lun != nil {
			nDisks[i].Lun = disk.Lun
		}
	}
	return n
}

func cloneDisk(s *compute.Disk) *compute.Disk {
	var n *compute.Disk
	testutils.Clone(&s, &n)
	return n
}
