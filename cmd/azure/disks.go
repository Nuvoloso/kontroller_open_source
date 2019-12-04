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
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
)

func init() {
	cmd, _ := parser.AddCommand("disk", "Disk object commands", "Disk object subcommands", &diskCmd{})
	cmd.AddCommand("list", "List disks", "List disks in the subscription", &diskListCmd{})
	cmd.AddCommand("create", "Create a disk", "Create a disk", &diskCreateCmd{})
	cmd.AddCommand("delete", "Delete a disk", "Delete a disk", &diskDeleteCmd{})
	cmd.AddCommand("show-columns", "Show disk table columns", "Show names of columns used in table format", &showColsCmd{columns: diskHeaders})
}

var diskHeaders = map[string]string{
	hID:        dID,
	hLocation:  dLocation,
	hManagedBy: dManagedBy,
	hName:      dName,
	hSKU:       dSKU,
	hSize:      dSize,
	hTags:      dTags,
	hType:      dType,
	hZones:     dZones,
}

var diskDefaultHeaders = []string{hName, hSKU, hSize, hManagedBy}

type diskCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string
}

func (c *diskCmd) validateColumns(columns string) error {
	var err error
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(diskHeaders), diskDefaultHeaders)
	return err
}

func (c *diskCmd) makeRecord(disk compute.Disk) map[string]string {
	sku := ""
	if disk.Sku != nil {
		sku = fmt.Sprintf("%s/%s", disk.Sku.Name, swag.StringValue(disk.Sku.Tier))
	}
	size := ""
	if disk.DiskProperties != nil {
		dp := disk.DiskProperties
		size = fmt.Sprintf("%d", swag.Int32Value(dp.DiskSizeGB))
	}
	var tb bytes.Buffer
	for _, k := range util.SortedStringKeys(disk.Tags) {
		tag := swag.StringValue(disk.Tags[k])
		fmt.Fprintf(&tb, "%s:%s ", k, tag)
	}
	tags := ""
	if tb.Len() > 0 {
		tags = tb.String()
	}
	mb := to.String(disk.ManagedBy)
	if mb != "" {
		if li := strings.LastIndex(mb, "/"); li > 0 {
			mb = mb[li+1 : len(mb)]
		}
	}
	return map[string]string{
		hID:        swag.StringValue(disk.ID),
		hName:      swag.StringValue(disk.Name),
		hLocation:  swag.StringValue(disk.Location),
		hManagedBy: mb,
		hType:      swag.StringValue(disk.Type),
		hSKU:       sku,
		hSize:      size,
		hTags:      tags,
	}
}

func (c *diskCmd) Emit(diskList []compute.Disk) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.Emitter.EmitJSON(diskList)
	case "yaml":
		return appCtx.Emitter.EmitYAML(diskList)
	}
	rows := make([][]string, 0, len(diskList))
	for _, d := range diskList {
		rec := c.makeRecord(d)
		row := make([]string, len(c.tableCols))
		for j, h := range c.tableCols {
			row[j] = rec[h]
		}
		rows = append(rows, row)
	}
	return appCtx.Emitter.EmitTable(c.tableCols, rows, nil)
}

type diskListCmd struct {
	ClientFlags
	GroupName string `short:"g" long:"group" env:"AZURE_RESOURCE_GROUP" required:"yes" description:"The name of the group"`
	Columns   string `short:"c" long:"columns" description:"Comma separated list of column names"`

	diskCmd
}

func (c *diskListCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	dc, err := c.DisksClient()
	if err == nil {
		ctx := context.Background()
		var iter compute.DiskListIterator
		iter, err = dc.ListByResourceGroupComplete(ctx, c.GroupName)
		if err == nil {
			var diskList []compute.Disk
			for err == nil && iter.NotDone() {
				diskList = append(diskList, iter.Value())
				err = iter.NextWithContext(ctx)
			}
			if err == nil {
				return c.Emit(diskList)
			}
		}
	}
	return err
}

type diskCreateCmd struct {
	ClientFlags
	GroupName string `short:"g" long:"group" env:"AZURE_RESOURCE_GROUP" required:"yes" description:"The name of the group"`
	DiskName  string `short:"n" long:"disk-name" description:"Globally unique name for the disk" required:"yes"`
	SizeGiB   int32  `short:"b" long:"size-gib" description:"Size in GiB" default:"1"`
	DiskType  string `short:"T" long:"disk-type" description:"The disk type. e.g. 'Standard_LRS', 'Premium_LRS', 'StandardSSD_LRS', 'UltraSSD_LRS'" required:"yes"`
	Columns   string `short:"c" long:"columns" description:"Comma separated list of column names"`

	diskCmd
}

func (c *diskCreateCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	ctx := context.Background()
	var g resources.Group
	gc, err := c.GroupsClient()
	if err == nil {
		g, err = gc.Get(ctx, c.GroupName)
		if err != nil {
			return err
		}
	}
	dc, err := c.DisksClient()
	if err == nil {
		d := compute.Disk{
			Location: g.Location,
			Sku: &compute.DiskSku{
				Name: compute.DiskStorageAccountTypes(c.DiskType),
			},
			DiskProperties: &compute.DiskProperties{
				CreationData: &compute.CreationData{
					CreateOption: compute.Empty,
				},
				DiskSizeGB: &c.SizeGiB,
			},
		}
		var future compute.DisksCreateOrUpdateFuture
		future, err = dc.CreateOrUpdate(ctx, c.GroupName, c.DiskName, d)
		if err == nil {
			err = future.WaitForCompletionRef(ctx, dc.Client)
			if err == nil {
				d, err = future.Result(dc)
				if err == nil {
					return c.Emit([]compute.Disk{d})
				}
			}
		}
	}
	return nil
}

type diskDeleteCmd struct {
	ClientFlags
	GroupName string `short:"g" long:"group" env:"AZURE_RESOURCE_GROUP" required:"yes" description:"The name of the group"`
	DiskName  string `short:"n" long:"disk-name" description:"Globally unique name for the disk" required:"yes"`

	diskCmd
}

func (c *diskDeleteCmd) Execute(args []string) error {
	ctx := context.Background()
	dc, err := c.DisksClient()
	if err == nil {
		var future compute.DisksDeleteFuture
		future, err = dc.Delete(ctx, c.GroupName, c.DiskName)
		if err == nil {
			err = future.WaitForCompletionRef(ctx, dc.Client)
			if err == nil {
				_, err = future.Result(dc)
			}
		}
	}
	return nil
}
