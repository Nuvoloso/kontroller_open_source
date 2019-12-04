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

	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2019-04-01/storage"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
)

func init() {
	cmd, _ := parser.AddCommand("account", "Storage account object commands", "Storage account object subcommands", &accountCmd{})
	cmd.AddCommand("list", "List accounts", "List accounts in the subscription", &accountListCmd{})
	cmd.AddCommand("show-columns", "Show account table columns", "Show names of columns used in table format", &showColsCmd{columns: accountHeaders})
	cmd.AddCommand("create", "Create storage account", "Create or update a storage account for a subscription", &accountCreateCmd{})
	cmd.AddCommand("keys", "Get storage account keys", "Get the keys for a storage account", &accountKeysCmd{})
	// delete
}

var accountHeaders = map[string]string{
	hID:            dID,
	hLocation:      dLocation,
	hName:          dName,
	hTags:          dTags,
	hType:          dType,
	hSKU:           dSKU,
	hResourceGroup: dResourceGroup,
}

var accountDefaultHeaders = []string{hName, hLocation, hSKU, hID}

type accountCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string
}

func (c *accountCmd) validateColumns(columns string) error {
	var err error
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(accountHeaders), accountDefaultHeaders)
	return err
}

func (c *accountCmd) makeRecord(account storage.Account) map[string]string {
	fmt.Printf("%#v\n", account)
	sku := ""
	if account.Sku != nil {
		sku = fmt.Sprintf("%s/%s", account.Sku.Name, account.Sku.Tier)
	}
	var tb bytes.Buffer
	for _, k := range util.SortedStringKeys(account.Tags) {
		tag := swag.StringValue(account.Tags[k])
		fmt.Fprintf(&tb, "%s:%s ", k, tag)
	}
	tags := ""
	if tb.Len() > 0 {
		tags = tb.String()
	}
	return map[string]string{
		hID:       swag.StringValue(account.ID),
		hName:     swag.StringValue(account.Name),
		hLocation: swag.StringValue(account.Location),
		hType:     swag.StringValue(account.Type),
		hSKU:      sku,
		hTags:     tags,
	}
}

func (c *accountCmd) Emit(accounts []storage.Account) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.Emitter.EmitJSON(accounts)
	case "yaml":
		return appCtx.Emitter.EmitYAML(accounts)
	}
	rows := make([][]string, 0, len(accounts))
	for _, d := range accounts {
		rec := c.makeRecord(d)
		row := make([]string, len(c.tableCols))
		for j, h := range c.tableCols {
			row[j] = rec[h]
		}
		rows = append(rows, row)
	}
	return appCtx.Emitter.EmitTable(c.tableCols, rows, nil)
}

type accountListCmd struct {
	ClientFlags
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`

	accountCmd
}

func (c *accountListCmd) Execute(args []string) error {
	var err error
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	ac := storage.NewAccountsClient(c.SubscriptionID)
	ac.Authorizer, err = c.GetAuthorizer()
	if err != nil {
		return err
	}
	ctx := context.Background()
	var iter storage.AccountListResultIterator
	iter, err = ac.ListComplete(ctx)
	if err == nil {
		var accounts []storage.Account
		for ; iter.NotDone(); iter.NextWithContext(ctx) {
			accounts = append(accounts, iter.Value())
		}
		return c.Emit((accounts))
	}
	return err
}

type accountCreateCmd struct {
	ClientFlags
	AccountName string `short:"a" long:"account" required:"yes" description:"The name of the account"`
	GroupName   string `short:"g" long:"group" required:"yes" description:"The name of the group"`
	SkuName     string `long:"sku" required:"no" default:"Standard_LRS" description:"The name of SKU"`
	AccessTier  string `long:"tier" required:"no" default:"Hot" description:"The type of access tier"`
	Kind        string `long:"kind" required:"no" default:"BlobStorage" description:"The kind of storage account"`

	accountCmd
}

func (c *accountCreateCmd) Execute(args []string) error {
	var err error
	ac := storage.NewAccountsClient(c.SubscriptionID)
	ac.Authorizer, err = c.GetAuthorizer()
	if err != nil {
		return err
	}
	ctx := context.Background()
	acParams := storage.AccountCreateParameters{
		Sku: &storage.Sku{
			Name: storage.SkuName(c.SkuName),
		},
		Kind:     storage.Kind(c.Kind),
		Location: swag.String(c.Location),
		// For BlobStorage we also require AccessTier (HOT/COLD) https://docs.microsoft.com/en-us/azure/storage/common/storage-account-overview#types-of-storage-accounts
		AccountPropertiesCreateParameters: &storage.AccountPropertiesCreateParameters{
			AccessTier: storage.AccessTier(c.AccessTier),
		},
	}
	result, err := ac.Create(ctx, c.GroupName, c.AccountName, acParams)
	if err == nil {
		err = result.WaitForCompletionRef(ctx, ac.Client)
		if err == nil {
			fmt.Printf("Storage Account %s created\n", c.AccountName)
			return nil
		}
	}
	return err
}

type accountKeysCmd struct {
	ClientFlags
	AccountName string `short:"a" long:"account" required:"yes" description:"The name of the account"`
	GroupName   string `short:"g" long:"group" required:"yes" description:"The name of the group"`

	accountCmd
}

func (c *accountKeysCmd) Execute(args []string) error {
	var err error
	ac := storage.NewAccountsClient(c.SubscriptionID)
	ac.Authorizer, err = c.GetAuthorizer()
	if err != nil {
		return err
	}
	ctx := context.Background()

	keyList, err := ac.ListKeys(ctx, c.GroupName, c.AccountName)
	if err == nil {
		keys := keyList.Keys
		for _, key := range *keys {
			fmt.Println(swag.StringValue(key.KeyName), "    ", swag.StringValue(key.Value))
		}
	}
	return nil
}
