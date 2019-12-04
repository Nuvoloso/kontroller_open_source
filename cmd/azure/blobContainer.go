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

	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2019-04-01/storage"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
)

func init() {
	cmd, _ := parser.AddCommand("container", "Container commands", "Container object subcommands", &blobConCmd{})
	cmd.AddCommand("show-columns", "Show account table columns", "Show names of columns used in table format", &showColsCmd{columns: blobContainerHeaders})
	cmd.AddCommand("list", "List containers", "List the containers in a storage account", &blobConListCmd{})
	cmd.AddCommand("create", "Create a container", "Create a container in a storage account", &blobConCreateCmd{})
}

var blobContainerHeaders = map[string]string{
	hName: dName,
	hID:   dID,
	hType: dType,
}
var blobContainerDefaultHeaders = []string{hName}

type blobConCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string
}

func (c *blobConCmd) validateColumns(columns string) error {
	var err error
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(blobContainerHeaders), blobContainerDefaultHeaders)
	return err
}

func (c *blobConCmd) makeRecord(blobCon storage.BlobContainer) map[string]string {
	return map[string]string{
		hName: swag.StringValue(blobCon.Name),
		hID:   swag.StringValue(blobCon.ID),
		hType: swag.StringValue(blobCon.Type),
	}
}

func (c *blobConCmd) Emit(blobCons []storage.BlobContainer) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.Emitter.EmitJSON(blobCons)
	case "yaml":
		return appCtx.Emitter.EmitYAML(blobCons)
	}
	rows := make([][]string, 0, len(blobCons))
	for _, d := range blobCons {
		rec := c.makeRecord(d)
		row := make([]string, len(c.tableCols))
		for j, h := range c.tableCols {
			row[j] = rec[h]
		}
		rows = append(rows, row)
	}
	return appCtx.Emitter.EmitTable(c.tableCols, rows, nil)
}

type blobConListCmd struct {
	ClientFlags
	AccountName string `short:"a" long:"account" required:"yes" description:"The name of the account"`
	GroupName   string `short:"g" long:"group" env:"AZURE_RESOURCE_GROUP" required:"yes" description:"The name of the group"`
	Columns     string `short:"c" long:"columns" description:"Comma separated list of column names"`

	blobConCmd
}

func (c *blobConListCmd) Execute(args []string) error {
	var err error
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	bc := storage.NewBlobContainersClient(c.SubscriptionID)
	bc.Authorizer, err = c.GetAuthorizer()
	if err != nil {
		return err
	}
	ctx := context.Background()
	result, err := bc.ListComplete(ctx, c.GroupName, c.AccountName, "", "", "")
	if err == nil {
		var blobCon []storage.BlobContainer
		for ; result.NotDone(); result.NextWithContext(ctx) {
			blobCon = append(blobCon, convertFromListItemToBlob(result.Value()))
		}
		return c.Emit(blobCon)
	}
	return err
}

func convertFromListItemToBlob(item storage.ListContainerItem) storage.BlobContainer {
	var result storage.BlobContainer
	result.ContainerProperties = item.ContainerProperties
	result.Etag = item.Etag
	result.ID = item.ID
	result.Name = item.Name
	result.Type = item.Type
	return result
}

type blobConCreateCmd struct {
	ClientFlags
	AccountName   string `short:"a" long:"account" required:"yes" description:"The name of the account"`
	GroupName     string `short:"g" long:"group" env:"AZURE_RESOURCE_GROUP" required:"yes" description:"The name of the group"`
	ContainerName string `short:"n" long:"containerName" required:"yes" description:"The name of the container"`

	blobConCmd
}

func (c *blobConCreateCmd) Execute(args []string) error {
	var err error
	bc := storage.NewBlobContainersClient(c.SubscriptionID)
	bc.Authorizer, err = c.GetAuthorizer()
	if err != nil {
		return err
	}
	ctx := context.Background()
	blobParams := storage.BlobContainer{}
	result, err := bc.Create(ctx, c.GroupName, c.AccountName, c.ContainerName, blobParams)
	if err == nil {
		fmt.Printf("Blob container %s created\n", swag.StringValue(result.Name))
		return nil
	}
	return err
}
