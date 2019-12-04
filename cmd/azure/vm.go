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
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/docker/go-units"
	"github.com/go-openapi/swag"
)

func init() {
	cmd, _ := parser.AddCommand("vm", "Virtual Machine object commands", "VM object subcommands", &vmCmd{})
	cmd.AddCommand("list", "List VMs", "List the virtual machines in a resource group", &vmListCmd{})
	cmd.AddCommand("attach", "Attach", "Attach a disk to a virtual machine", &vmAttachDiskCmd{})
	cmd.AddCommand("detach", "Detach", "Detach a disk from a virtual machine", &vmDetachDiskCmd{})
	cmd.AddCommand("show-columns", "Show vm table columns", "Show names of columns used in table format", &showColsCmd{columns: vmHeaders})
}

var vmHeaders = map[string]string{
	hName:         dName,
	hType:         dType,
	hLocation:     dLocation,
	hVMSize:       dVMSize,
	hOSDiskSize:   dOSDiskSize,
	hOSType:       dOSType,
	hOSImageSKU:   dOSImageSKU,
	hNumDataDisks: dNumDataDisks,
}

var vmDefaultHeaders = []string{hName, hVMSize, hOSType, hOSImageSKU, hOSDiskSize, hNumDataDisks}

type vmCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string
}

func (c *vmCmd) validateColumns(columns string) error {
	var err error
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(vmHeaders), vmDefaultHeaders)
	return err
}

func (c *vmCmd) makeRecord(vm compute.VirtualMachine) map[string]string {
	osDiskSize := util.SizeBytes(int64(swag.Int32Value(vm.StorageProfile.OsDisk.DiskSizeGB)) * units.GiB)
	numDataDisks := ""
	if vm.StorageProfile.DataDisks != nil {
		numDataDisks = fmt.Sprintf("%d", len(*vm.StorageProfile.DataDisks))
	}
	return map[string]string{
		hName:         swag.StringValue(vm.Name),
		hType:         swag.StringValue(vm.Type),
		hLocation:     swag.StringValue(vm.Location),
		hVMSize:       string(vm.HardwareProfile.VMSize),
		hOSType:       string(vm.StorageProfile.OsDisk.OsType),
		hOSImageSKU:   swag.StringValue(vm.StorageProfile.ImageReference.Sku),
		hOSDiskSize:   osDiskSize.String(),
		hNumDataDisks: numDataDisks,
	}
}

func (c *vmCmd) Emit(vms []compute.VirtualMachine) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.Emitter.EmitJSON(vms)
	case "yaml":
		return appCtx.Emitter.EmitYAML(vms)
	}
	rows := make([][]string, 0, len(vms))
	for _, item := range vms {
		rec := c.makeRecord(item)
		row := make([]string, len(c.tableCols))
		for j, h := range c.tableCols {
			row[j] = rec[h]
		}
		rows = append(rows, row)
	}
	return appCtx.Emitter.EmitTable(c.tableCols, rows, nil)
}

type vmListCmd struct {
	ClientFlags
	GroupName string `short:"g" long:"group" env:"AZURE_RESOURCE_GROUP" required:"yes" description:"The name of the group"`
	Columns   string `short:"c" long:"columns" description:"Comma separated list of column names"`

	vmCmd
}

func (c *vmListCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	vmc, err := c.VirtualMachinesClient()
	if err == nil {
		ctx := context.Background()
		var iter compute.VirtualMachineListResultIterator
		iter, err = vmc.ListComplete(ctx, c.GroupName)
		if err == nil {
			var vms []compute.VirtualMachine
			for ; iter.NotDone(); iter.NextWithContext(ctx) {
				vms = append(vms, iter.Value())
			}
			return c.Emit(vms)
		}
	}
	return err
}

type vmAttachDiskCmd struct {
	ClientFlags
	GroupName string `short:"g" long:"group" env:"AZURE_RESOURCE_GROUP" required:"yes" description:"The name of the group"`
	VMName    string `short:"n" long:"vm-name" description:"The name of the virtual machine" required:"yes"`
	DiskName  string `short:"d" long:"disk-name" description:"Globally unique name for the disk" required:"yes"`
	Columns   string `short:"c" long:"columns" description:"Comma separated list of column names"`

	vmCmd
}

func (c *vmAttachDiskCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	vmc, err := c.VirtualMachinesClient()
	if err == nil {
		ctx := context.Background()
		var dc compute.DisksClient
		var d compute.Disk
		dc, err = c.DisksClient()
		if err == nil {
			d, err = dc.Get(ctx, c.GroupName, c.DiskName)
		}
		if err != nil {
			return err
		}
		var vm compute.VirtualMachine
		vm, err = vmc.Get(ctx, c.GroupName, c.VMName, compute.InstanceView)
		if err == nil {
			disks := *vm.StorageProfile.DataDisks
			// Find the next available lun (adapted from K8s)
			used := make([]bool, 64) // 64 luns max
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
				return fmt.Errorf("no more LUNs available")
			}
			dd := compute.DataDisk{
				Name:         to.StringPtr(c.DiskName),
				Lun:          to.Int32Ptr(lun),
				CreateOption: compute.DiskCreateOptionTypesAttach,
				DiskSizeGB:   d.DiskSizeGB,
				ManagedDisk: &compute.ManagedDiskParameters{
					ID: d.ID,
				},
			}
			disks = append(disks, dd)
			vm.StorageProfile.DataDisks = &disks
			fmt.Printf("Attaching(%s, %s, %s: sz=%d lun=%d)\n", c.GroupName, c.VMName, c.DiskName, *d.DiskSizeGB, lun)
			var future compute.VirtualMachinesCreateOrUpdateFuture
			future, err = vmc.CreateOrUpdate(ctx, c.GroupName, c.VMName, vm)
			if err == nil {
				err = future.WaitForCompletionRef(ctx, vmc.Client)
				if err == nil {
					vm, err = future.Result(vmc)
					if err == nil {
						return c.Emit([]compute.VirtualMachine{vm})
					}
				}
			}
			return err
		}
	}
	return err
}

type vmDetachDiskCmd struct {
	ClientFlags
	GroupName string `short:"g" long:"group" env:"AZURE_RESOURCE_GROUP" required:"yes" description:"The name of the group"`
	VMName    string `short:"n" long:"vm-name" description:"The name of the virtual machine" required:"yes"`
	DiskName  string `short:"d" long:"disk-name" description:"Globally unique name for the disk" required:"yes"`
	Columns   string `short:"c" long:"columns" description:"Comma separated list of column names"`

	vmCmd
}

func (c *vmDetachDiskCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	vmc, err := c.VirtualMachinesClient()
	if err == nil {
		ctx := context.Background()
		vm, err := vmc.Get(ctx, c.GroupName, c.VMName, compute.InstanceView)
		if err == nil {
			disks := *vm.StorageProfile.DataDisks
			found := false
			for i, d := range disks {
				if d.Name != nil && *d.Name == c.DiskName {
					found = true
					disks = append(disks[:i], disks[i+1:]...)
				}
			}
			if !found {
				fmt.Printf("Disk not attached\n")
				return nil
			}
			vm.StorageProfile.DataDisks = &disks
			fmt.Printf("Detaching(%s, %s, %s)\n", c.GroupName, c.VMName, c.DiskName)
			var future compute.VirtualMachinesCreateOrUpdateFuture
			future, err = vmc.CreateOrUpdate(ctx, c.GroupName, c.VMName, vm)
			if err == nil {
				err = future.WaitForCompletionRef(ctx, vmc.Client)
				if err == nil {
					vm, err = future.Result(vmc)
					if err == nil {
						return c.Emit([]compute.VirtualMachine{vm})
					}
				}
			}
			return err
		}
	}
	return err
}
