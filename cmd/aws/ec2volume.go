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
	"github.com/Nuvoloso/kontroller/pkg/csp/aws"
	"github.com/alecthomas/units"
)

func init() {
	initEC2Volume()
}

// default instance metadata timeout is 30 seconds, much longer than necessary
const shortIMDTimeoutSecs = 5

func initEC2Volume() {
	cmd, _ := parser.AddCommand("ec2volume", "EC2 Volume commands", "EC2 Volume subcommands", &ec2VolumeCmd{})
	cmd.Aliases = []string{"ecv", "volume"}
	cmd.AddCommand("attach", "Attach volume", "Attach an EC2 volume to an instance", &ec2VolumeAttachCmd{})
	cmd.AddCommand("create", "Create volume", "Create an EC2 volume", &ec2VolumeCreateCmd{})
	cmd.AddCommand("delete", "Delete a volume", "Delete an EC2 volume", &ec2VolumeDeleteCmd{})
	cmd.AddCommand("detach", "Detach volume", "Detach an EC2 volume from an instance", &ec2VolumeDetachCmd{})
	cmd.AddCommand("get", "Lookup volume", "Lookup an EC2 volume", &ec2VolumeFetchCmd{})
	cmd.AddCommand("list", "List volumes", "List EC2 volumes", &ec2VolumeListCmd{})
	c, _ := cmd.AddCommand("set-tags", "Set tags on a volume", "Set or modify EC2 volume tags", &ec2VolumeSetTagsCmd{})
	c.Aliases = []string{"tag-set"}
	c, _ = cmd.AddCommand("remove-tags", "Remove tags on a volume", "Remove EC2 volume tags", &ec2VolumeRmTagsCmd{})
	c.Aliases = []string{"tag-rm"}
}

type ec2VolumeCmd struct {
	tableCols []string
}

const (
	hVolumeID                = "Volume Identifier"
	hVolumeType              = "Type"
	hVolumeSize              = "Size"
	hVolumeProvisioningState = "State"
	hVolumeAttachment        = "Attachment"
	hTags                    = "Tags"
)

var ec2VolumeHeaders = map[string]string{
	hVolumeID:                "volume identifier",
	hVolumeType:              "volume type",
	hVolumeSize:              "volume size",
	hVolumeProvisioningState: "volume provisioning state",
	hVolumeAttachment:        "attachment data",
	hTags:                    "tags",
}

var ec2VolumeDefaultHeaders = []string{hVolumeID, hVolumeType, hVolumeSize, hVolumeProvisioningState, hVolumeAttachment, hTags}

func (c *ec2VolumeCmd) makeRecord(o *csp.Volume) map[string]string {
	att := make([]string, 0, len(o.Attachments))
	for _, a := range o.Attachments {
		s := fmt.Sprintf("%s:%s:%s", a.NodeIdentifier, a.Device, a.State.String())
		att = append(att, s)
	}
	return map[string]string{
		hVolumeID:                o.Identifier,
		hVolumeType:              o.Type,
		hVolumeSize:              fmt.Sprintf("%d", o.SizeBytes),
		hVolumeProvisioningState: o.ProvisioningState.String(),
		hVolumeAttachment:        strings.Join(att, " "),
		hTags:                    strings.Join(o.Tags, ", "),
	}
}

func (c *ec2VolumeCmd) validateColumns(columns string) error {
	if matched, _ := regexp.MatchString("^\\s*$", columns); matched {
		c.tableCols = ec2VolumeDefaultHeaders
	} else {
		c.tableCols = strings.Split(columns, ",")
		for _, col := range c.tableCols {
			if _, ok := ec2VolumeHeaders[col]; !ok {
				return fmt.Errorf("invalid column \"%s\"", col)
			}
		}
	}
	return nil
}
func (c *ec2VolumeCmd) Emit(data []*csp.Volume) error {
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

type ec2VolumeFetchCmd struct {
	ID      string `long:"id" required:"yes"`
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`
	ec2VolumeCmd
}

func (c *ec2VolumeFetchCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := appCtx.ClientInit(&appCtx.AwsCredFlags); err != nil {
		return err
	}
	volumeIdentifier := c.ID
	_, _, err := aws.VolumeIdentifierParse(c.ID)
	if err != nil {
		volumeIdentifier = aws.VolumeIdentifierCreate(aws.ServiceEC2, c.ID)
	}
	vfa := &csp.VolumeFetchArgs{
		VolumeIdentifier: volumeIdentifier,
	}
	v, err := appCtx.AwsClient.VolumeFetch(nil, vfa)
	if err != nil {
		return err
	}
	return c.Emit([]*csp.Volume{v})
}

type ec2VolumeCreateCmd struct {
	StorageType string   `short:"T" long:"storage-type" description:"Specify the CSPStorageType name" required:"yes"`
	Size        int64    `short:"s" long:"size-gib" description:"Specify the size in GiB." required:"yes"`
	Tags        []string `short:"t" long:"tag" description:"Specify a tag in the form 'key:value'. Repeat as needed."`
	Columns     string   `short:"c" long:"columns" description:"Comma separated list of column names"`
	ec2VolumeCmd
}

func (c *ec2VolumeCreateCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := appCtx.ClientInit(&appCtx.AwsCredFlags); err != nil {
		return err
	}
	vca := &csp.VolumeCreateArgs{
		StorageTypeName: models.CspStorageType(c.StorageType),
		SizeBytes:       c.Size * int64(units.GiB),
	}
	if len(c.Tags) > 0 {
		vca.Tags = c.Tags
	}
	v, err := appCtx.AwsClient.VolumeCreate(nil, vca)
	if err != nil {
		return err
	}
	return c.Emit([]*csp.Volume{v})
}

type ec2VolumeListCmd struct {
	StorageType string   `short:"T" long:"storage-type" description:"Specify the CSPStorageType name"`
	Tags        []string `short:"t" long:"tag" description:"Specify a tag in the form 'key:value' or just the key to search for existence. Repeat as needed"`
	Columns     string   `short:"c" long:"columns" description:"Comma separated list of column names"`
	ec2VolumeCmd
}

func (c *ec2VolumeListCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := appCtx.ClientInit(&appCtx.AwsCredFlags); err != nil {
		return err
	}
	vla := &csp.VolumeListArgs{
		StorageTypeName: models.CspStorageType(c.StorageType),
		Tags:            c.Tags,
	}

	res, err := appCtx.AwsClient.VolumeList(nil, vla)
	if err != nil {
		return err
	}
	imd, err := appCtx.Csp.LocalInstanceMetadata() // will fail if not running on EC2 instance
	if err == nil {
		nodeIdentifier := imd[csp.IMDInstanceName]
		// look up actual local attachments
		for _, vol := range res {
			for i, attached := range vol.Attachments {
				if attached.NodeIdentifier == nodeIdentifier {
					if attached.Device, err = appCtx.Csp.LocalInstanceDeviceName(vol.Identifier, attached.Device); err != nil {
						return err
					}
					vol.Attachments[i] = attached
				}
			}
		}
	}
	return c.Emit(res)
}

type ec2VolumeDeleteCmd struct {
	ID      string `long:"id" required:"yes"`
	Confirm bool   `long:"confirm" description:"Confirm the deletion. Required if dry-run is not set."`
	ec2VolumeCmd
}

func (c *ec2VolumeDeleteCmd) Execute(args []string) error {
	if err := appCtx.ClientInit(&appCtx.AwsCredFlags); err != nil {
		return err
	}
	volumeIdentifier := c.ID
	_, _, err := aws.VolumeIdentifierParse(c.ID)
	if err != nil {
		volumeIdentifier = aws.VolumeIdentifierCreate(aws.ServiceEC2, c.ID)
	}
	vda := &csp.VolumeDeleteArgs{
		VolumeIdentifier: volumeIdentifier,
	}
	if !c.Confirm {
		return fmt.Errorf("confirm not set")
	}
	err = appCtx.AwsClient.VolumeDelete(nil, vda)
	if err != nil {
		return err
	}
	return nil
}

type ec2VolumeAttachCmd struct {
	ID      string `long:"id" required:"yes"`
	NID     string `short:"I" long:"node-identifier" required:"yes"`
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`
	ec2VolumeCmd
}

func (c *ec2VolumeAttachCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := appCtx.ClientInit(&appCtx.AwsCredFlags); err != nil {
		return err
	}
	volumeIdentifier := c.ID
	_, _, err := aws.VolumeIdentifierParse(c.ID)
	if err != nil {
		volumeIdentifier = aws.VolumeIdentifierCreate(aws.ServiceEC2, c.ID)
	}
	vaa := &csp.VolumeAttachArgs{
		VolumeIdentifier: volumeIdentifier,
		NodeIdentifier:   c.NID,
	}
	v, err := appCtx.AwsClient.VolumeAttach(nil, vaa)
	if err != nil {
		return err
	}
	return c.Emit([]*csp.Volume{v})
}

type ec2VolumeDetachCmd struct {
	ID      string `long:"id" required:"yes"`
	NID     string `short:"I" long:"node-identifier"`
	Device  string `short:"d" long:"device-path" description:"The path where the volume is mounted."`
	Force   bool   `long:"force" description:"Force detach. This can corrupt data."`
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`
	ec2VolumeCmd
}

func (c *ec2VolumeDetachCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := appCtx.ClientInit(&appCtx.AwsCredFlags); err != nil {
		return err
	}
	volumeIdentifier := c.ID
	_, _, err := aws.VolumeIdentifierParse(c.ID)
	if err != nil {
		volumeIdentifier = aws.VolumeIdentifierCreate(aws.ServiceEC2, c.ID)
	}
	vda := &csp.VolumeDetachArgs{
		VolumeIdentifier: volumeIdentifier,
		NodeIdentifier:   strings.TrimSpace(c.NID),
		NodeDevice:       strings.TrimSpace(c.Device),
		Force:            c.Force,
	}
	v, err := appCtx.AwsClient.VolumeDetach(nil, vda)
	if err != nil {
		return err
	}
	return c.Emit([]*csp.Volume{v})
}

type ec2VolumeSetTagsCmd struct {
	ID      string   `long:"id" required:"yes"`
	Tags    []string `short:"t" long:"tag" description:"Specify a tag in the form 'key:value'. Repeat as needed."`
	Columns string   `short:"c" long:"columns" description:"Comma separated list of column names"`
	ec2VolumeCmd
}

func (c *ec2VolumeSetTagsCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := appCtx.ClientInit(&appCtx.AwsCredFlags); err != nil {
		return err
	}
	volumeIdentifier := c.ID
	_, _, err := aws.VolumeIdentifierParse(c.ID)
	if err != nil {
		volumeIdentifier = aws.VolumeIdentifierCreate(aws.ServiceEC2, c.ID)
	}
	vta := &csp.VolumeTagArgs{
		VolumeIdentifier: volumeIdentifier,
		Tags:             c.Tags,
	}
	v, err := appCtx.AwsClient.VolumeTagsSet(nil, vta)
	if err != nil {
		return err
	}
	return c.Emit([]*csp.Volume{v})
}

type ec2VolumeRmTagsCmd struct {
	ID      string   `long:"id" required:"yes"`
	Tags    []string `short:"t" long:"tag" description:"Specify a tag key in the form 'key' or 'key:value'. Repeat as needed."`
	Columns string   `short:"c" long:"columns" description:"Comma separated list of column names"`
	ec2VolumeCmd
}

func (c *ec2VolumeRmTagsCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := appCtx.ClientInit(&appCtx.AwsCredFlags); err != nil {
		return err
	}
	volumeIdentifier := c.ID
	_, _, err := aws.VolumeIdentifierParse(c.ID)
	if err != nil {
		volumeIdentifier = aws.VolumeIdentifierCreate(aws.ServiceEC2, c.ID)
	}
	vta := &csp.VolumeTagArgs{
		VolumeIdentifier: volumeIdentifier,
		Tags:             c.Tags,
	}
	v, err := appCtx.AwsClient.VolumeTagsDelete(nil, vta)
	if err != nil {
		return err
	}
	return c.Emit([]*csp.Volume{v})
}
