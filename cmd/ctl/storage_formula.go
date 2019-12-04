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
	"sort"
	"strings"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_formula"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/go-openapi/swag"
	"github.com/olekukonko/tablewriter"
)

func init() {
	initStorageFormula()
}

func initStorageFormula() {
	cmd, _ := parser.AddCommand("storage-formula", "StorageFormula object commands", "StorageFormula object subcommands", &storageFormulaCmd{})
	cmd.Aliases = []string{"sf", "storage-formulas"}
	cmd.AddCommand("list", "List StorageFormulas", "List or search for StorageFormula objects.", &storageFormulaListCmd{})
	cmd.AddCommand("show-columns", "Show StorageFormula table columns", "Show names of columns used in table format", &showColsCmd{columns: storageFormulaHeaders})
}

type storageFormulaCmd struct {
	tableCols []string
}

// storageFormula record keys/headers and their description
var storageFormulaHeaders = map[string]string{
	hName:             dName,
	hDescription:      dDescription,
	hCacheComponent:   "The CSPStorageTypes and percentages for the cache.",
	hIOProfile:        "The type of IO.",
	hSscList:          "The storage service capabilities.",
	hStorageComponent: "The CSPStorageTypes and percentages for the primary storage.",
	hStorageLayout:    "The layout to use for the parcels.",
}

var storageFormulaDefaultHeaders = []string{hName, hDescription, hStorageComponent, hCacheComponent, hIOProfile, hSscList, hStorageLayout}

// makeRecord creates a map of properties
func (c *storageFormulaCmd) makeRecord(o *models.StorageFormula) map[string]string {
	cList := []string{}
	for k, v := range o.CacheComponent {
		if p := swag.Int32Value(v.Percentage); p > 0 {
			cList = append(cList, fmt.Sprintf("%s:%d%%", k, p))
		}
	}
	sort.Strings(cList)
	iopList := []string{}
	for _, s := range []string{o.IoProfile.IoPattern.Name, o.IoProfile.ReadWriteMix.Name} {
		if s != "" {
			iopList = append(iopList, s)
		}
	}
	sscList := []string{}
	for k, v := range o.SscList {
		sscList = append(sscList, fmt.Sprintf("%s:%s", k, v.Value))
	}
	sort.Strings(sscList)
	sList := []string{}
	for k, v := range o.StorageComponent {
		sList = append(sList, fmt.Sprintf("%s:%d%%", k, swag.Int32Value(v.Percentage)))
	}
	sort.Strings(sList)
	return map[string]string{
		hName:             string(o.Name),
		hDescription:      string(o.Description),
		hCacheComponent:   strings.Join(cList, "\n"),
		hIOProfile:        strings.Join(iopList, ", "),
		hSscList:          strings.Join(sscList, "\n"),
		hStorageComponent: strings.Join(sList, "\n"),
		hStorageLayout:    string(o.StorageLayout),
	}
}

func (c *storageFormulaCmd) validateColumns(columns string) error {
	if matched, _ := regexp.MatchString("^\\s*$", columns); matched {
		c.tableCols = storageFormulaDefaultHeaders
	} else {
		c.tableCols = strings.Split(columns, ",")
		for _, col := range c.tableCols {
			if _, ok := storageFormulaHeaders[col]; !ok {
				return fmt.Errorf("invalid column \"%s\"", col)
			}
		}
	}
	return nil
}

func (c *storageFormulaCmd) Emit(data []*models.StorageFormula) error {
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
	cT := func(t *tablewriter.Table) {
		t.SetColWidth(17)
	}
	return appCtx.EmitTable(c.tableCols, rows, cT)
}

func (c *storageFormulaCmd) list(params *storage_formula.StorageFormulaListParams) ([]*models.StorageFormula, error) {
	res, err := appCtx.API.StorageFormula().StorageFormulaList(params)
	if err != nil {
		if e, ok := err.(*storage_formula.StorageFormulaListDefault); ok && e.Payload.Message != nil {
			return nil, fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res.Payload, nil
}

type storageFormulaListCmd struct {
	Name    string `short:"n" long:"name" description:"Name of a single storage formula"`
	Columns string `long:"columns" description:"Comma separated list of column names"`

	storageFormulaCmd
	remainingArgsCatcher
}

func (c *storageFormulaListCmd) Execute(args []string) error {
	var err error
	if err = c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	params := storage_formula.NewStorageFormulaListParams()
	if c.Name != "" {
		params.Name = &c.Name
	}
	var res []*models.StorageFormula
	if res, err = c.list(params); err != nil {
		return err
	}
	return c.Emit(res)
}
