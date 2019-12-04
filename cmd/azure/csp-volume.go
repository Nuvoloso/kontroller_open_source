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
	"strings"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/alecthomas/units"
)

func init() {
	cspC, _ := parser.AddCommand("csp", "CSP commands", "CSP subcommands", &cspCmd{})

	cspV, _ := cspC.AddCommand("volume", "CSP volume commands", "CSP volume subcommans", &cspVolumeCmd{})
	cspV.Aliases = []string{"vol"}
	cspV.AddCommand("attach", "Attach a volume to a node", "Attach a volume", &cspVolumeAttach{})
	cspV.AddCommand("create", "Create a volume", "Create a volume", &cspVolumeCreate{})
	cspV.AddCommand("delete", "Delete a volume", "Delete a volume", &cspVolumeDelete{})
	cspV.AddCommand("detach", "Detach a volume from a node", "Detach a volume", &cspVolumeDetach{})
	cspV.AddCommand("list", "List volumes", "List volumes", &cspVolumeList{})
}

type cspCmd struct{}

var cspVolumeHeaders = map[string]string{
	hID:             dID,
	hCspStorageType: dCspStorageType,
	hType:           dType,
	hSize:           dSize,
	hNodeID:         dNodeID,
	hDevice:         dDevice,
	hTags:           dTags,
}

var cspVolumeDefaultHeaders = []string{hID, hCspStorageType, hSize, hNodeID, hDevice}

type cspVolumeCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string
}

func (c *cspVolumeCmd) validateColumns(columns string) error {
	var err error
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(cspVolumeHeaders), cspVolumeDefaultHeaders)
	return err
}

func (c *cspVolumeCmd) makeRecord(v *csp.Volume) map[string]string {
	size := util.SizeBytesToString(v.SizeBytes)
	rec := map[string]string{
		hID:             v.Identifier,
		hCspStorageType: string(v.StorageTypeName),
		hType:           v.Type,
		hSize:           size,
		hTags:           strings.Join(v.Tags, " "),
	}
	if len(v.Attachments) > 0 {
		rec[hNodeID] = v.Attachments[0].NodeIdentifier
		rec[hDevice] = v.Attachments[0].Device
	}
	return rec
}

func (c *cspVolumeCmd) Emit(vols []*csp.Volume) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.Emitter.EmitJSON(vols)
	case "yaml":
		return appCtx.Emitter.EmitYAML(vols)
	}
	rows := make([][]string, 0, len(vols))
	for _, d := range vols {
		rec := c.makeRecord(d)
		row := make([]string, len(c.tableCols))
		for j, h := range c.tableCols {
			row[j] = rec[h]
		}
		rows = append(rows, row)
	}
	return appCtx.Emitter.EmitTable(c.tableCols, rows, nil)
}

type cspVolumeCreate struct {
	CSPClientFlags
	SizeGiB     int    `short:"b" long:"size-gib" description:"Size in GiB" default:"1"`
	StorageType string `short:"S" long:"storage-type" description:"The CSP storage type. e.g. 'Azure Standard SSD', 'Azure Standard HDD', 'Azure Premium SSD', 'Azure Ultra SSD'" required:"yes"`
	Columns     string `short:"c" long:"columns" description:"Comma separated list of column names"`

	cspVolumeCmd
}

func (c *cspVolumeCreate) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := c.ProcessCSPClientFlags(); err != nil {
		return err
	}
	ctx := context.Background()
	cspArgs := &csp.VolumeCreateArgs{
		StorageTypeName: models.CspStorageType(c.StorageType),
		SizeBytes:       int64(c.SizeGiB * int(units.GiB)),
	}
	vol, err := appCtx.AzureClient.VolumeCreate(ctx, cspArgs)
	if err != nil {
		return err
	}
	return c.Emit([]*csp.Volume{vol})
}

type cspVolumeList struct {
	CSPClientFlags
	StorageType string   `short:"S" long:"storage-type" description:"The CSP storage type. e.g. 'Azure Standard SSD', 'Azure Standard HDD', 'Azure Premium SSD', 'Azure Ultra SSD'"`
	Tags        []string `short:"T" long:"tag" description:"Tag of the form K:V to match disks; repeat as needed"`
	Columns     string   `short:"c" long:"columns" description:"Comma separated list of column names"`

	cspVolumeCmd
}

func (c *cspVolumeList) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := c.ProcessCSPClientFlags(); err != nil {
		return err
	}
	ctx := context.Background()
	cspArgs := &csp.VolumeListArgs{}
	if c.StorageType != "" {
		cspArgs.StorageTypeName = models.CspStorageType(c.StorageType)
	}
	if len(c.Tags) > 0 {
		cspArgs.Tags = c.Tags
	}
	vols, err := appCtx.AzureClient.VolumeList(ctx, cspArgs)
	if err != nil {
		return err
	}
	return c.Emit(vols)
}

type cspVolumeDelete struct {
	CSPClientFlags
	VolID string `short:"I" long:"volume-id" required:"yes" description:"Identifier of volume to delete"`
}

func (c *cspVolumeDelete) Execute(args []string) error {
	if err := c.ProcessCSPClientFlags(); err != nil {
		return err
	}
	ctx := context.Background()
	cspArgs := &csp.VolumeDeleteArgs{
		VolumeIdentifier: c.VolID,
	}
	return appCtx.AzureClient.VolumeDelete(ctx, cspArgs)
}

type cspVolumeAttach struct {
	CSPClientFlags
	VolID   string `short:"I" long:"volume-id" required:"yes" description:"Identifier of volume"`
	NodeID  string `short:"N" long:"node-id" required:"yes" description:"Identifier of the node"`
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`

	cspVolumeCmd
}

func (c *cspVolumeAttach) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := c.ProcessCSPClientFlags(); err != nil {
		return err
	}
	ctx := context.Background()
	cspArgs := &csp.VolumeAttachArgs{
		VolumeIdentifier: c.VolID,
		NodeIdentifier:   c.NodeID,
	}
	vol, err := appCtx.AzureClient.VolumeAttach(ctx, cspArgs)
	if err != nil {
		return err
	}
	return c.Emit([]*csp.Volume{vol})
}

type cspVolumeDetach struct {
	CSPClientFlags
	VolID   string `short:"I" long:"volume-id" required:"yes" description:"Identifier of volume"`
	NodeID  string `short:"N" long:"node-id" required:"yes" description:"Identifier of the node"`
	Device  string `short:"D" long:"device" required:"yes" description:"The device name"`
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`

	cspVolumeCmd
}

func (c *cspVolumeDetach) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := c.ProcessCSPClientFlags(); err != nil {
		return err
	}
	ctx := context.Background()
	cspArgs := &csp.VolumeDetachArgs{
		VolumeIdentifier: c.VolID,
		NodeIdentifier:   c.NodeID,
		NodeDevice:       c.Device,
	}
	vol, err := appCtx.AzureClient.VolumeDetach(ctx, cspArgs)
	if err != nil {
		return err
	}
	return c.Emit([]*csp.Volume{vol})
}
