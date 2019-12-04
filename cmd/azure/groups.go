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

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
)

func init() {
	cmd, _ := parser.AddCommand("group", "Resource group object commands", "Resource group object subcommands", &groupCmd{})
	cmd.AddCommand("list", "List groups", "List groups in the subscription", &groupListCmd{})
	cmd.AddCommand("show-columns", "Show group table columns", "Show names of columns used in table format", &showColsCmd{columns: groupHeaders})
	cmd.AddCommand("create", "Create group", "Create a new group for the subscription", &groupCreateCmd{})
}

var groupHeaders = map[string]string{
	hID:        dID,
	hLocation:  dLocation,
	hManagedBy: dManagedBy,
	hName:      dName,
	hTags:      dTags,
	hType:      dType,
}

var groupDefaultHeaders = []string{hName, hLocation}

type groupCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string
}

func (c *groupCmd) validateColumns(columns string) error {
	var err error
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(groupHeaders), groupDefaultHeaders)
	return err
}

func (c *groupCmd) makeRecord(group resources.Group) map[string]string {
	var tb bytes.Buffer
	for _, k := range util.SortedStringKeys(group.Tags) {
		tag := swag.StringValue(group.Tags[k])
		fmt.Fprintf(&tb, "%s:%s ", k, tag)
	}
	tags := ""
	if tb.Len() > 0 {
		tags = tb.String()
	}
	return map[string]string{
		hID:        swag.StringValue(group.ID),
		hName:      swag.StringValue(group.Name),
		hLocation:  swag.StringValue(group.Location),
		hManagedBy: swag.StringValue(group.ManagedBy),
		hType:      swag.StringValue(group.Type),
		hTags:      tags,
	}
}

func (c *groupCmd) Emit(group []resources.Group) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.Emitter.EmitJSON(group)
	case "yaml":
		return appCtx.Emitter.EmitYAML(group)
	}
	rows := make([][]string, 0, len(group))
	for _, d := range group {
		rec := c.makeRecord(d)
		row := make([]string, len(c.tableCols))
		for j, h := range c.tableCols {
			row[j] = rec[h]
		}
		rows = append(rows, row)
	}
	return appCtx.Emitter.EmitTable(c.tableCols, rows, nil)
}

type groupListCmd struct {
	ClientFlags
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`

	groupCmd
}

func (c *groupListCmd) Execute(args []string) error {
	var err error
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	gc, err := c.GroupsClient()
	if err != nil {
		return err
	}
	ctx := context.Background()
	var iter resources.GroupListResultIterator
	iter, err = gc.ListComplete(ctx, "", nil)
	if err == nil {
		var groups []resources.Group
		for ; iter.NotDone(); iter.NextWithContext(ctx) {
			groups = append(groups, iter.Value())
		}
		return c.Emit(groups)
	}

	return err
}

type groupCreateCmd struct {
	ClientFlags
	GroupName string `short:"g" long:"group" required:"yes" description:"The name of the group"`
	Columns   string `short:"c" long:"columns" description:"Comma separated list of column names"`

	groupCmd
}

func (c *groupCreateCmd) Execute(args []string) error {
	var err error
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	gc, err := c.GroupsClient()
	if err != nil {
		return err
	}
	ctx := context.Background()
	var groupParams resources.Group
	groupParams.Location = swag.String(c.Location)
	group, err := gc.CreateOrUpdate(ctx, c.GroupName, groupParams)
	if err == nil {
		return c.Emit([]resources.Group{group})
	}
	return err
}
