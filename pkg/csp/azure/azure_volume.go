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
	"sort"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/azuresdk"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/alecthomas/units"
)

// VolumeAttach attaches an Azure CSP volume
func (cl *Client) VolumeAttach(ctx context.Context, vaa *csp.VolumeAttachArgs) (*csp.Volume, error) {
	groupName := cl.getResourceGroup(vaa.ProvisioningAttributes)
	dc := cl.newDisksClient()
	vmc := cl.newVMClient()
	var d compute.Disk
	var vm compute.VirtualMachine
	// TBD: Handle VM Scaleable Sets
	op := func() (bool, error) {
		var err error
		cl.csp.dbgF("AZURE fetching disk[%s] in group[%s]", vaa.VolumeIdentifier, groupName)
		d, err = dc.Get(ctx, groupName, vaa.VolumeIdentifier)
		if err != nil {
			return false, err
		}
		cl.csp.dbgF("AZURE fetching node[%s] in group[%s]", vaa.NodeIdentifier, groupName)
		vm, err = vmc.Get(ctx, groupName, vaa.NodeIdentifier, compute.InstanceView)
		if err == nil {
			// TBD: handle case of disk already attached to
			//  - this host: error or consider done? if latter must get lun
			//  - other host
			disks := *vm.StorageProfile.DataDisks
			// Find the next available lun (adapted from K8s)
			used := make([]bool, MaxLunsPerVM)
			for _, disk := range disks {
				if disk.Lun != nil {
					used[*disk.Lun] = true
				}
			}
			lun := int32(-1)
			for k, v := range used {
				if !v {
					lun = int32(k)
					break
				}
			}
			if lun == -1 {
				return false, fmt.Errorf("no more LUNs available")
			}
			cl.csp.dbgF("AZURE attaching disk[%s] to node[%s] in group[%s]: lun[%d]", vaa.VolumeIdentifier, vaa.NodeIdentifier, groupName, lun)
			dd := compute.DataDisk{
				Name:         d.Name,
				Lun:          to.Int32Ptr(lun),
				CreateOption: compute.DiskCreateOptionTypesAttach,
				DiskSizeGB:   d.DiskSizeGB,
				ManagedDisk: &compute.ManagedDiskParameters{
					ID: d.ID,
				},
			}
			disks = append(disks, dd)
			vm.StorageProfile.DataDisks = &disks
			var future azuresdk.VMCreateOrUpdateFuture
			future, err = vmc.CreateOrUpdate(ctx, groupName, vaa.NodeIdentifier, vm)
			if err == nil {
				err = future.WaitForCompletionRef(ctx, vmc)
				if err == nil {
					vm, err = future.Result(vmc)
					if err == nil {
						return false, nil
					}
				}
			}
		}
		return false, err // TBD: retry cases
	}
	if err := cl.doWithRetry(op); err != nil {
		return nil, err
	}
	// refresh the disk
	cl.csp.dbgF("AZURE fetching disk[%s] in group[%s]", vaa.VolumeIdentifier, groupName)
	d, err := dc.Get(ctx, groupName, vaa.VolumeIdentifier)
	if err != nil {
		return nil, err
	}
	return azureDiskToVolume(&d, &vm), nil
}

// VolumeCreate creates an Azure CSP volume
func (cl *Client) VolumeCreate(ctx context.Context, vca *csp.VolumeCreateArgs) (*csp.Volume, error) {
	diskType, cspStorageType := CspStorageTypeToDiskType(vca.StorageTypeName)
	if cspStorageType == nil {
		return nil, fmt.Errorf("invalid storage type for CSPDomain")
	}
	location := cl.attrs[AttrLocation].Value
	groupName := cl.getResourceGroup(vca.ProvisioningAttributes)
	diskName := newResourceName("disk")
	_, volSize := azureDiskSize(vca.SizeBytes)
	dc := cl.newDisksClient()
	d := compute.Disk{
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
		Tags: azureVolToDiskTags(vca.Tags),
	}
	op := func() (bool, error) {
		cl.csp.dbgF("AZURE creating disk[%s] in group[%s] (%s,%dGiB)", diskName, groupName, diskType, volSize)
		future, err := dc.CreateOrUpdate(ctx, groupName, diskName, d)
		if err == nil {
			err = future.WaitForCompletionRef(ctx, dc)
			if err == nil {
				d, err = future.Result(dc)
			}
		}
		return false, err // TBD: determine retry cases
	}
	if err := cl.doWithRetry(op); err != nil {
		return nil, err
	}
	return azureDiskToVolume(&d, nil), nil
}

// VolumeDelete deletes an Azure CSP volume
func (cl *Client) VolumeDelete(ctx context.Context, vda *csp.VolumeDeleteArgs) error {
	groupName := cl.getResourceGroup(vda.ProvisioningAttributes)
	dc := cl.newDisksClient()
	op := func() (bool, error) {
		cl.csp.dbgF("AZURE deleting disk[%s] in group[%s]", vda.VolumeIdentifier, groupName)
		future, err := dc.Delete(ctx, groupName, vda.VolumeIdentifier)
		if err == nil {
			err = future.WaitForCompletionRef(ctx, dc)
			if err == nil {
				_, err = future.Result(dc)
			}
		}
		return false, err // TBD: determine retry cases
	}
	return cl.doWithRetry(op)
}

// VolumeDetach detaches an Azure CSP volume
func (cl *Client) VolumeDetach(ctx context.Context, vda *csp.VolumeDetachArgs) (*csp.Volume, error) {
	groupName := cl.getResourceGroup(vda.ProvisioningAttributes)
	vmc := cl.newVMClient()
	found := false
	var vm compute.VirtualMachine
	// TBD: Handle VM Scaleable Sets
	op := func() (bool, error) {
		cl.csp.dbgF("AZURE fetching node[%s] in group[%s]", vda.NodeIdentifier, groupName)
		var err error
		vm, err = vmc.Get(ctx, groupName, vda.NodeIdentifier, compute.InstanceView)
		if err == nil {
			disks := *vm.StorageProfile.DataDisks
			lun := int32(-1)
			for i, d := range disks {
				if d.Name != nil && *d.Name == vda.VolumeIdentifier {
					found = true
					lun = to.Int32(d.Lun)
					disks = append(disks[:i], disks[i+1:]...)
				}
			}
			if !found {
				cl.csp.dbgF("AZURE disk[%s] already detached from node[%s] group[%s]", vda.VolumeIdentifier, vda.NodeIdentifier, groupName)
				return false, csp.ErrorVolumeNotAttached
			}
			vm.StorageProfile.DataDisks = &disks
			cl.csp.dbgF("AZURE detaching disk[%s] from node[%s] releasing lun[%d]", vda.VolumeIdentifier, vda.NodeIdentifier, lun)
			var future azuresdk.VMCreateOrUpdateFuture
			future, err = vmc.CreateOrUpdate(ctx, groupName, vda.NodeIdentifier, vm)
			if err == nil {
				err = future.WaitForCompletionRef(ctx, vmc)
				if err == nil {
					vm, err = future.Result(vmc)
					if err == nil {
						return false, nil
					}
				}
			}
		}
		return false, err // TBD: retry cases
	}
	if err := cl.doWithRetry(op); err != nil {
		return nil, err
	}
	cl.csp.dbgF("AZURE fetching disk[%s] in group[%s]", vda.VolumeIdentifier, groupName)
	dc := cl.newDisksClient()
	d, err := dc.Get(ctx, groupName, vda.VolumeIdentifier)
	if err != nil {
		return nil, err
	}
	return azureDiskToVolume(&d, nil), nil
}

// VolumeFetch returns information on an Azure CSP volume
func (cl *Client) VolumeFetch(ctx context.Context, vfa *csp.VolumeFetchArgs) (*csp.Volume, error) {
	groupName := cl.getResourceGroup(vfa.ProvisioningAttributes)
	dc := cl.newDisksClient()
	cl.csp.dbgF("AZURE fetching disk[%s] in group[%s]", vfa.VolumeIdentifier, groupName)
	d, err := dc.Get(ctx, groupName, vfa.VolumeIdentifier)
	if err != nil {
		return nil, err
	}
	return azureDiskToVolume(&d, cl.attachedVM(ctx, groupName, &d)), nil
}

// VolumeList performs a search for Azure CSP volumes (constrained by resource group)
func (cl *Client) VolumeList(ctx context.Context, vla *csp.VolumeListArgs) ([]*csp.Volume, error) {
	groupName := cl.getResourceGroup(vla.ProvisioningAttributes)
	skuName := ""
	if vla.StorageTypeName != "" {
		skuName, _ = CspStorageTypeToDiskType(vla.StorageTypeName)
	}
	var tl *util.TagList
	if len(vla.Tags) > 0 {
		tl = util.NewTagList(vla.Tags)
	}
	cl.csp.dbgF("AZURE listing disks in group[%s]", groupName)
	dc := cl.newDisksClient()
	vmCache := map[string]*compute.VirtualMachine{}
	iter, err := dc.ListByResourceGroupComplete(ctx, groupName)
	if err == nil {
		volList := []*csp.Volume{}
	nextDisk:
		for ; err == nil && iter.NotDone(); err = iter.NextWithContext(ctx) {
			d := iter.Value()
			if skuName != "" && (d.Sku == nil || skuName != string(d.Sku.Name)) {
				continue // storage type mismatch
			}
			if tl != nil {
				dt := util.NewTagList(azureDiskToVolTags(d.Tags))
				for _, k := range tl.Keys(nil) {
					tv, _ := tl.Get(k)
					if dv, found := dt.Get(k); !found || dv != tv {
						continue nextDisk // tag mismatch
					}
				}
			}
			var pVM *compute.VirtualMachine
			if d.ManagedBy != nil {
				vmName := azureDiskManagedByToVMName(d.ManagedBy)
				if vm, ok := vmCache[vmName]; ok {
					pVM = vm
				} else if vm := cl.attachedVM(ctx, groupName, &d); vm != nil {
					vmCache[vmName] = vm
					pVM = vm
				}
			}
			volList = append(volList, azureDiskToVolume(&d, pVM))
		}
		if err == nil {
			return volList, nil
		}
	}
	return nil, err
}

func (cl *Client) attachedVM(ctx context.Context, groupName string, pDisk *compute.Disk) *compute.VirtualMachine {
	if pDisk.ManagedBy != nil {
		vmName := azureDiskManagedByToVMName(pDisk.ManagedBy)
		cl.csp.dbgF("AZURE fetching VM[%s] in group[%s]", vmName, groupName)
		vmc := cl.newVMClient()
		vm, err := vmc.Get(ctx, groupName, vmName, compute.InstanceView)
		if err == nil {
			return &vm
		}
		cl.csp.dbgF("AZURE unable to fetch VM[%s] for disk[%s] in group[%s] MB=%s: %s", vmName, to.String(pDisk.Name), groupName, to.String(pDisk.ManagedBy), err.Error())
	}
	return nil
}

// VolumeTagsDelete removes tags on Azure CSP volumes
func (cl *Client) VolumeTagsDelete(ctx context.Context, vta *csp.VolumeTagArgs) (*csp.Volume, error) {
	// TBD: fill this in
	return nil, nil // fake success
}

// VolumeTagsSet creates or modifies tags on Azure CSP volumes
func (cl *Client) VolumeTagsSet(ctx context.Context, vta *csp.VolumeTagArgs) (*csp.Volume, error) {
	// TBD: fill this in
	return nil, fmt.Errorf("not implemented")
}

// VolumeSize returns the actual size that will be allocated for a volume.
// A zero sized request gets the minimum allocable
func (cl *Client) VolumeSize(ctx context.Context, stName models.CspStorageType, requestedSizeBytes int64) (int64, error) {
	if requestedSizeBytes < 0 {
		return 0, fmt.Errorf("invalid allocation size")
	}
	_, storageType := CspStorageTypeToDiskType(stName)
	if storageType == nil {
		return 0, fmt.Errorf("invalid storage type '%s' for Azure", stName)
	}
	if requestedSizeBytes < *storageType.MinAllocationSizeBytes {
		requestedSizeBytes = *storageType.MinAllocationSizeBytes
	} else if requestedSizeBytes > *storageType.MaxAllocationSizeBytes {
		return 0, fmt.Errorf("requested size exceeds the storage type maximum")
	}
	sb, _ := azureDiskSize(requestedSizeBytes)
	return sb, nil
}

// azureDiskToVolume creates a csp.Volume
// The VM is optional if the disk is not managed.
func azureDiskToVolume(d *compute.Disk, vm *compute.VirtualMachine) *csp.Volume {
	v := &csp.Volume{
		CSPDomainType:     CSPDomainType,
		StorageTypeName:   DiskTypeToCspStorageType(string(d.Sku.Name)),
		Identifier:        to.String(d.Name),
		Type:              string(d.Sku.Name),
		SizeBytes:         to.Int64(d.DiskProperties.DiskSizeBytes),
		ProvisioningState: csp.VolumeProvisioningProvisioned,
		Tags:              azureDiskToVolTags(d.Tags),
		Raw:               d,
	}
	// determine the node device (best effort)
	if vm != nil && d.ManagedBy != nil && to.String(d.ManagedBy) == to.String(vm.ID) && vm.StorageProfile != nil && vm.StorageProfile.DataDisks != nil {
		disks := *vm.StorageProfile.DataDisks
		for _, disk := range disks {
			if v.Identifier == to.String(disk.Name) && disk.Lun != nil {
				v.Attachments = []csp.VolumeAttachment{
					{
						NodeIdentifier: to.String(vm.Name),
						Device:         azureVMLunToDeviceName(disk.Lun),
						State:          csp.VolumeAttachmentAttached,
					},
				}
				break
			}
		}
	}
	return v
}

// azureVMLunToDeviceName returns the device name for a given LUN.
// See https://docs.microsoft.com/en-us/azure/virtual-machines/troubleshooting/troubleshoot-device-names-problems
func azureVMLunToDeviceName(pLUN *int32) string {
	return fmt.Sprintf("/dev/disk/azure/scsi1/lun%d", to.Int32(pLUN))
}

// azureDiskManagedByToVMName parses a ManagedBy field and extracts the VM name (which is the last component)
func azureDiskManagedByToVMName(pMB *string) string {
	mb := to.String(pMB)
	if li := strings.LastIndex(mb, "/"); li > 0 {
		mb = mb[li+1 : len(mb)]
	}
	return mb
}

// azureDiskSize converts a requested size into a supported
// size, returning sizeBytes and sizeGiB
func azureDiskSize(requestedSizeBytes int64) (int64, int64) {
	oneGib := int64(units.GiB)
	volSize := requestedSizeBytes / oneGib
	if volSize*oneGib < requestedSizeBytes {
		volSize++
	}
	return volSize * oneGib, volSize
}

func azureVolToDiskTags(volTags []string) map[string]*string {
	if len(volTags) == 0 {
		return nil
	}
	tl := util.NewTagList(volTags)
	dt := map[string]*string{}
	keys := tl.Keys(nil)
	sort.Strings(keys) // helps in UT, minimal overhead
	for _, k := range keys {
		v, _ := tl.Get(k)
		dt[k] = to.StringPtr(v)
	}
	return dt
}

func azureDiskToVolTags(dt map[string]*string) []string {
	tl := util.NewTagList(nil)
	for _, k := range util.SortedStringKeys(dt) { // helps in UT, minimal overhead
		vp := dt[k]
		tl.Set(k, to.String(vp))
	}
	return tl.List()
}
