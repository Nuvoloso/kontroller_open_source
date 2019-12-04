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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/slo"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/util"
)

func init() {
	initSlo()
}

func initSlo() {
	cmd, _ := parser.AddCommand("slo", "SLO object commands", "SLO object subcommands", &sloCmd{})
	cmd.Aliases = []string{"slos"}
	cmd.AddCommand("list", "List SLOs", "List or search for SLO objects.", &sloListCmd{})
	cmd.AddCommand("show-columns", "Show SLO table columns", "Show names of columns used in table format", &showColsCmd{columns: sloHeaders})
}

type sloCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string
}

// slo record keys/headers and their description
var sloHeaders = map[string]string{
	hName:          dName,
	hDescription:   dDescription,
	hChoices:       "The choice of values.",
	hUnitDimension: "The unit in which the SLO is expressed.",
}

var sloDefaultHeaders = []string{hName, hDescription, hUnitDimension, hChoices}

// makeRecord creates a map of properties
func (c *sloCmd) makeRecord(o *models.SLO) map[string]string {
	strs := make([]string, len(o.Choices))
	for i, v := range o.Choices {
		strs[i] = v.ValueType.Value
	}

	return map[string]string{
		hName:          string(o.Name),
		hDescription:   string(o.Description),
		hUnitDimension: o.Choices[0].ValueType.Kind,
		hChoices:       strings.Join(strs, ", "),
	}
}

func (c *sloCmd) validateColumns(columns string) error {
	var err error
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(sloHeaders), sloDefaultHeaders)
	return err
}

func (c *sloCmd) Emit(data []*models.SLO) error {
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

func (c *sloCmd) list(params *slo.SloListParams) ([]*models.SLO, error) {
	res, err := appCtx.API.Slo().SloList(params)
	if err != nil {
		if e, ok := err.(*slo.SloListDefault); ok && e.Payload.Message != nil {
			return nil, fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res.Payload, nil
}

type sloListCmd struct {
	NamePattern string `short:"n" long:"name" description:"A SLO object name pattern"`
	Columns     string `short:"c" long:"columns" description:"Comma separated list of column names"`

	sloCmd
	remainingArgsCatcher
}

func (c *sloListCmd) Execute(args []string) error {
	var err error
	if err = c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	params := slo.NewSloListParams()
	params.NamePattern = &c.NamePattern
	var res []*models.SLO
	if res, err = c.list(params); err != nil {
		return err
	}
	return c.Emit(res)
}
