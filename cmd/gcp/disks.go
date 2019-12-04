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
	"regexp"
	"strings"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/csp/gc"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/docker/go-units"
)

func init() {
	initDisks()
}

// default instance metadata timeout is 30 seconds, much longer than necessary
const shortIMDTimeoutSecs = 5

func initDisks() {
	cmd, _ := parser.AddCommand("disks", "GCE disk commands", "GCE disk subcommands", &disksCmd{})
	cmd.Aliases = []string{"disk"}
	cmd.AddCommand("attach", "Attach disk", "Attach an GCE disk to an instance", &disksAttachCmd{})
	cmd.AddCommand("create", "Create disk", "Create an GCE disk", &disksCreateCmd{})
	cmd.AddCommand("delete", "Delete a disk", "Delete an GCE disk", &disksDeleteCmd{})
	cmd.AddCommand("detach", "Detach disk", "Detach an GCE disk from an instance", &disksDetachCmd{})
	cmd.AddCommand("get", "Lookup disk", "Lookup an GCE disk", &disksFetchCmd{})
	cmd.AddCommand("list", "List disks", "List GCE disks", &disksListCmd{})
	c, _ := cmd.AddCommand("set-tags", "Set tags on a disk", "Set or modify GCE disk tags", &disksSetTagsCmd{})
	c.Aliases = []string{"tag-set"}
	c, _ = cmd.AddCommand("remove-tags", "Remove tags on a disk", "Remove GCE disk tags", &disksRmTagsCmd{})
	c.Aliases = []string{"tag-rm"}
}

type disksCmd struct {
	tableCols []string
}

const (
	hDiskID                = "Disk Identifier"
	hDiskType              = "Type"
	hDiskSize              = "Size"
	hDiskProvisioningState = "State"
	hDiskAttachment        = "Attachment"
	hTags                  = "Tags"
)

var disksHeaders = map[string]string{
	hDiskID:                "disk identifier",
	hDiskType:              "disk type",
	hDiskSize:              "disk size",
	hDiskProvisioningState: "disk provisioning state",
	hDiskAttachment:        "attachment data",
	hTags:                  "tags",
}

var disksDefaultHeaders = []string{hDiskID, hDiskType, hDiskSize, hDiskProvisioningState, hDiskAttachment, hTags}

func (c *disksCmd) makeRecord(o *csp.Volume) map[string]string {
	att := make([]string, 0, len(o.Attachments))
	for _, a := range o.Attachments {
		s := fmt.Sprintf("%s:%s:%s", a.NodeIdentifier, a.Device, a.State.String())
		att = append(att, s)
	}
	return map[string]string{
		hDiskID:                o.Identifier,
		hDiskType:              o.Type,
		hDiskSize:              util.SizeBytesToString(o.SizeBytes),
		hDiskProvisioningState: o.ProvisioningState.String(),
		hDiskAttachment:        strings.Join(att, " "),
		hTags:                  strings.Join(o.Tags, ", "),
	}
}

func (c *disksCmd) validateColumns(columns string) error {
	if matched, _ := regexp.MatchString("^\\s*$", columns); matched {
		c.tableCols = disksDefaultHeaders
	} else {
		c.tableCols = strings.Split(columns, ",")
		for _, col := range c.tableCols {
			if _, ok := disksHeaders[col]; !ok {
				return fmt.Errorf("invalid column \"%s\"", col)
			}
		}
	}
	return nil
}

func (c *disksCmd) Emit(data []*csp.Volume) error {
	switch appCtx.OutputFormat {
	case "json":
		return appCtx.EmitJSON(data)
	case "yaml":
		return appCtx.EmitYAML(data)
	}
	rows := make([][]string, len(data))
	for i, o := range data {
		rec := c.makeRecord(o)
		row := make([]string, len(c.tableCols))
		for j, h := range c.tableCols {
			row[j] = rec[h]
		}
		rows[i] = row
	}
	return appCtx.EmitTable(c.tableCols, rows, nil)
}

type disksFetchCmd struct {
	ID      string `long:"id" required:"yes"`
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`
	disksCmd
}

func (c *disksFetchCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := appCtx.ClientInit(&appCtx.CredFlags); err != nil {
		return err
	}
	volumeIdentifier := c.ID
	_, _, err := gc.VolumeIdentifierParse(c.ID)
	if err != nil {
		volumeIdentifier = gc.VolumeIdentifierCreate(gc.ServiceGCE, c.ID)
	}
	vfa := &csp.VolumeFetchArgs{
		VolumeIdentifier: volumeIdentifier,
	}
	v, err := appCtx.GCPClient.VolumeFetch(nil, vfa)
	if err != nil {
		return err
	}
	return c.Emit([]*csp.Volume{v})
}

type disksCreateCmd struct {
	StorageType string   `short:"T" long:"storage-type" description:"Specify the CSPStorageType name" required:"yes"`
	Size        int64    `short:"s" long:"size-gib" description:"Specify the size in GiB." required:"yes"`
	Tags        []string `short:"t" long:"tag" description:"Specify a tag in the form 'key:value'. Repeat as needed."`
	Columns     string   `short:"c" long:"columns" description:"Comma separated list of column names"`
	disksCmd
}

func (c *disksCreateCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := appCtx.ClientInit(&appCtx.CredFlags); err != nil {
		return err
	}
	vca := &csp.VolumeCreateArgs{
		StorageTypeName: models.CspStorageType(c.StorageType),
		SizeBytes:       c.Size * units.GiB,
	}
	if len(c.Tags) > 0 {
		vca.Tags = c.Tags
	}
	v, err := appCtx.GCPClient.VolumeCreate(nil, vca)
	if err != nil {
		return err
	}
	return c.Emit([]*csp.Volume{v})
}

type disksListCmd struct {
	StorageType string   `short:"T" long:"storage-type" description:"Specify the CSPStorageType name"`
	Tags        []string `short:"t" long:"tag" description:"Specify a tag in the form 'key:value' or just the key to search for existence. Repeat as needed"`
	Columns     string   `short:"c" long:"columns" description:"Comma separated list of column names"`
	disksCmd
}

func (c *disksListCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := appCtx.ClientInit(&appCtx.CredFlags); err != nil {
		return err
	}
	vla := &csp.VolumeListArgs{
		StorageTypeName: models.CspStorageType(c.StorageType),
		Tags:            c.Tags,
	}

	res, err := appCtx.GCPClient.VolumeList(nil, vla)
	if err != nil {
		return err
	}
	return c.Emit(res)
}

type disksDeleteCmd struct {
	ID      string `long:"id" required:"yes"`
	Confirm bool   `long:"confirm" description:"Confirm the deletion. Required if dry-run is not set."`
	disksCmd
}

func (c *disksDeleteCmd) Execute(args []string) error {
	if err := appCtx.ClientInit(&appCtx.CredFlags); err != nil {
		return err
	}
	volumeIdentifier := c.ID
	_, _, err := gc.VolumeIdentifierParse(c.ID)
	if err != nil {
		volumeIdentifier = gc.VolumeIdentifierCreate(gc.ServiceGCE, c.ID)
	}
	vda := &csp.VolumeDeleteArgs{
		VolumeIdentifier: volumeIdentifier,
	}
	if !c.Confirm {
		return fmt.Errorf("confirm not set")
	}
	err = appCtx.GCPClient.VolumeDelete(nil, vda)
	if err != nil {
		return err
	}
	return nil
}

type disksAttachCmd struct {
	ID      string `long:"id" required:"yes"`
	NID     string `short:"I" long:"node-identifier" required:"yes"`
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`
	disksCmd
}

func (c *disksAttachCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := appCtx.ClientInit(&appCtx.CredFlags); err != nil {
		return err
	}
	volumeIdentifier := c.ID
	_, _, err := gc.VolumeIdentifierParse(c.ID)
	if err != nil {
		volumeIdentifier = gc.VolumeIdentifierCreate(gc.ServiceGCE, c.ID)
	}
	vaa := &csp.VolumeAttachArgs{
		VolumeIdentifier: volumeIdentifier,
		NodeIdentifier:   c.NID,
	}
	v, err := appCtx.GCPClient.VolumeAttach(nil, vaa)
	if err != nil {
		return err
	}
	return c.Emit([]*csp.Volume{v})
}

type disksDetachCmd struct {
	ID      string `long:"id" required:"yes"`
	NID     string `short:"I" long:"node-identifier" required:"yes"`
	Device  string `short:"d" long:"device-path" description:"The path where the disk is mounted."`
	Force   bool   `long:"force" description:"Force detach. This can corrupt data."`
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`
	disksCmd
}

func (c *disksDetachCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := appCtx.ClientInit(&appCtx.CredFlags); err != nil {
		return err
	}
	volumeIdentifier := c.ID
	_, _, err := gc.VolumeIdentifierParse(c.ID)
	if err != nil {
		volumeIdentifier = gc.VolumeIdentifierCreate(gc.ServiceGCE, c.ID)
	}
	vda := &csp.VolumeDetachArgs{
		VolumeIdentifier: volumeIdentifier,
		NodeIdentifier:   strings.TrimSpace(c.NID),
		NodeDevice:       strings.TrimSpace(c.Device),
		Force:            c.Force,
	}
	v, err := appCtx.GCPClient.VolumeDetach(nil, vda)
	if err != nil {
		return err
	}
	return c.Emit([]*csp.Volume{v})
}

type disksSetTagsCmd struct {
	ID      string   `long:"id" required:"yes"`
	Tags    []string `short:"t" long:"tag" description:"Specify a tag in the form 'key:value'. Repeat as needed."`
	Columns string   `short:"c" long:"columns" description:"Comma separated list of column names"`
	disksCmd
}

func (c *disksSetTagsCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := appCtx.ClientInit(&appCtx.CredFlags); err != nil {
		return err
	}
	volumeIdentifier := c.ID
	_, _, err := gc.VolumeIdentifierParse(c.ID)
	if err != nil {
		volumeIdentifier = gc.VolumeIdentifierCreate(gc.ServiceGCE, c.ID)
	}
	vta := &csp.VolumeTagArgs{
		VolumeIdentifier: volumeIdentifier,
		Tags:             c.Tags,
	}
	v, err := appCtx.GCPClient.VolumeTagsSet(nil, vta)
	if err != nil {
		return err
	}
	return c.Emit([]*csp.Volume{v})
}

type disksRmTagsCmd struct {
	ID      string   `long:"id" required:"yes"`
	Tags    []string `short:"t" long:"tag" description:"Specify a tag key in the form 'key' or 'key:value'. Repeat as needed."`
	Columns string   `short:"c" long:"columns" description:"Comma separated list of column names"`
	disksCmd
}

func (c *disksRmTagsCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := appCtx.ClientInit(&appCtx.CredFlags); err != nil {
		return err
	}
	volumeIdentifier := c.ID
	_, _, err := gc.VolumeIdentifierParse(c.ID)
	if err != nil {
		volumeIdentifier = gc.VolumeIdentifierCreate(gc.ServiceGCE, c.ID)
	}
	vta := &csp.VolumeTagArgs{
		VolumeIdentifier: volumeIdentifier,
		Tags:             c.Tags,
	}
	v, err := appCtx.GCPClient.VolumeTagsDelete(nil, vta)
	if err != nil {
		return err
	}
	return c.Emit([]*csp.Volume{v})
}
