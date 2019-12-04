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
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/role"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/util"
)

func init() {
	initRole()
}

func initRole() {
	cmd, _ := parser.AddCommand("role", "Role object commands", "Role object subcommands", &roleCmd{})
	cmd.Aliases = []string{"roles"}
	cmd.AddCommand("list", "List roles", "List or search for Role objects.", &roleListCmd{})
	cmd.AddCommand("show-columns", "Show role table columns", "Show names of columns used in table format", &showColsCmd{columns: roleHeaders})
}

type roleCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string
}

// role record keys/headers and their description
var roleHeaders = map[string]string{
	hID:           dID,
	hTimeCreated:  dTimeCreated,
	hTimeModified: dTimeModified,
	hVersion:      dVersion,
	hName:         dName,
	// TBD Capabilities
}

var roleDefaultHeaders = []string{hName}

// makeRecord creates a map of properties
func (c *roleCmd) makeRecord(o *models.Role) map[string]string {
	return map[string]string{
		hID:           string(o.Meta.ID),
		hTimeCreated:  time.Time(o.Meta.TimeCreated).Format(time.RFC3339),
		hTimeModified: time.Time(o.Meta.TimeModified).Format(time.RFC3339),
		hVersion:      fmt.Sprintf("%d", o.Meta.Version),
		hName:         string(o.Name),
	}
}

func (c *roleCmd) validateColumns(columns string) error {
	var err error
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(roleHeaders), roleDefaultHeaders)
	return err
}

func (c *roleCmd) Emit(data []*models.Role) error {
	switch c.OutputFormat {
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

func (c *roleCmd) list(params *role.RoleListParams) ([]*models.Role, error) {
	if params == nil {
		params = role.NewRoleListParams()
	}
	res, err := appCtx.API.Role().RoleList(params)
	if err != nil {
		if e, ok := err.(*role.RoleListDefault); ok && e.Payload.Message != nil {
			return nil, fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res.Payload, nil
}

type roleListCmd struct {
	Name    string `short:"n" long:"name" description:"A role name"`
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`

	roleCmd
	remainingArgsCatcher
}

func (c *roleListCmd) Execute(args []string) error {
	var err error
	if err = c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	params := role.NewRoleListParams()
	params.Name = &c.Name
	var res []*models.Role
	if res, err = c.list(params); err != nil {
		return err
	}
	return c.Emit(res)
}
