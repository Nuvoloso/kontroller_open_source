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
	"strings"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/application_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/util"
)

func init() {
	initApplicationGroup()
}

func initApplicationGroup() {
	cmd, _ := parser.AddCommand("application-group", "ApplicationGroup object commands", "ApplicationGroup object subcommands", &applicationGroupCmd{})
	cmd.Aliases = []string{"application-groups", "ag", "ags"}
	cmd.AddCommand("list", "List application groups", "List ApplicationGroups.", &applicationGroupListCmd{})
	cmd.AddCommand("show-columns", "Show applicationGroup table columns", "Show names of columns used in table format", &showColsCmd{columns: applicationGroupHeaders})
}

type applicationGroupCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string

	cacheHelper
}

// applicationGroup record keys/headers and their description
var applicationGroupHeaders = map[string]string{
	hID:           dID,
	hTimeCreated:  dTimeCreated,
	hTimeModified: dTimeModified,
	hVersion:      dVersion,
	hAccount:      dAccount,
	hName:         dName,
	hDescription:  dDescription,
	hTags:         dTags,
}

var applicationGroupDefaultHeaders = []string{hName, hAccount, hTags}

// makeRecord creates a map of properties
func (c *applicationGroupCmd) makeRecord(o *models.ApplicationGroup) map[string]string {
	var account string
	if a, ok := c.accounts[string(o.AccountID)]; !ok {
		account = string(o.AccountID)
	} else {
		account = a.name
	}
	return map[string]string{
		hID:           string(o.Meta.ID),
		hTimeCreated:  time.Time(o.Meta.TimeCreated).Format(time.RFC3339),
		hTimeModified: time.Time(o.Meta.TimeModified).Format(time.RFC3339),
		hVersion:      fmt.Sprintf("%d", o.Meta.Version),
		hName:         string(o.Name),
		hDescription:  string(o.Description),
		hAccount:      account,
		hTags:         strings.Join(o.Tags, ", "),
	}
}

func (c *applicationGroupCmd) validateColumns(columns string) error {
	var err error
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(applicationGroupHeaders), applicationGroupDefaultHeaders)
	return err
}

func (c *applicationGroupCmd) Emit(data []*models.ApplicationGroup) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.EmitJSON(data)
	case "yaml":
		return appCtx.EmitYAML(data)
	}
	c.cacheAccounts()
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

func (c *applicationGroupCmd) list(params *application_group.ApplicationGroupListParams) ([]*models.ApplicationGroup, error) {
	if params == nil {
		params = application_group.NewApplicationGroupListParams()
	}
	res, err := appCtx.API.ApplicationGroup().ApplicationGroupList(params)
	if err != nil {
		if e, ok := err.(*application_group.ApplicationGroupListDefault); ok && e.Payload.Message != nil {
			return nil, fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res.Payload, nil
}

type applicationGroupListCmd struct {
	Name      string   `short:"n" long:"name" description:"An applicationGroup name"`
	Tags      []string `short:"t" long:"tag" description:"A tag value; repeat as needed"`
	OwnedOnly bool     `long:"owned-only" description:"Retrieve only applicationGroups owned by the account, otherwise, if a tenant admin, retrieve applicationGroups owned by the tenant or any subordinate account. A no-op for non-tenant accounts"`
	Columns   string   `short:"c" long:"columns" description:"Comma separated list of column names"`

	applicationGroupCmd
	remainingArgsCatcher
}

func (c *applicationGroupListCmd) Execute(args []string) error {
	var err error
	if err = c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	params := application_group.NewApplicationGroupListParams()
	if c.Name != "" {
		params.Name = &c.Name
	}
	if appCtx.AccountID != "" && c.OwnedOnly {
		params.AccountID = &appCtx.AccountID
	}
	params.Tags = c.Tags
	var res []*models.ApplicationGroup
	if res, err = c.list(params); err != nil {
		return err
	}
	return c.Emit(res)
}
