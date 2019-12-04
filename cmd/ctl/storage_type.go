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
	"strconv"
	"strings"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_storage_type"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/go-openapi/swag"
)

func init() {
	initCspStorageType()
}

func initCspStorageType() {
	cmd, _ := parser.AddCommand("csp-storage-type", "CSPStorageType object commands", "CSPStorageType object subcommands", &cspStorageTypeCmd{})
	cmd.Aliases = []string{"storage-type", "storage-types", "csp-storage-types"}
	cmd.AddCommand("list", "List CSPStorageTypes", "List or search for CSPStorageType objects.", &cspStorageTypeListCmd{})
	cmd.AddCommand("show-columns", "Show CSPStorageType table columns", "Show names of columns used in table format", &showColsCmd{columns: cspStorageTypeHeaders})
}

type cspStorageTypeCmd struct {
	tableCols []string
}

// cspStorageType record keys/headers and their description
var cspStorageTypeHeaders = map[string]string{
	hName:                             dName,
	hDescription:                      dDescription,
	hCspDomainType:                    dCspDomainType,
	hMaxAllocationSizeBytes:           "The maximum allocation size in bytes.",
	hMinAllocationSizeBytes:           "The minimum allocation size in bytes.",
	hPreferredAllocationSizeBytes:     "The preferred device allocation size in bytes.",
	hPreferredAllocationUnitSizeBytes: "The preferred device allocation unit size in bytes.",
	hProvisioningUnit:                 "The performance/capacity ratio for a storage element.",
	hParcelSizeBytes:                  "The parcel size for a storage element.",
	hAccessibilityScope:               "The accessibility scope.",
}

var cspStorageTypeDefaultHeaders = []string{hName, hDescription, hAccessibilityScope, hProvisioningUnit, hPreferredAllocationSizeBytes, hMinAllocationSizeBytes, hMaxAllocationSizeBytes, hPreferredAllocationUnitSizeBytes, hParcelSizeBytes}

// makeRecord creates a map of properties
func (c *cspStorageTypeCmd) makeRecord(o *models.CSPStorageType) map[string]string {
	pUnitS := "CSPStorageType missing Provisioning Unit"
	if *o.ProvisioningUnit.IOPS != 0 {
		pUnitS = strconv.FormatInt(*o.ProvisioningUnit.IOPS, 10) + "IOPS/GiB"
	} else if *o.ProvisioningUnit.Throughput != 0 {
		pUnitS = sizeToString(*o.ProvisioningUnit.Throughput) + "/s/GiB"
	}
	return map[string]string{
		hName:                             string(o.Name),
		hDescription:                      string(o.Description),
		hCspDomainType:                    string(o.CspDomainType),
		hMaxAllocationSizeBytes:           sizeToString(swag.Int64Value(o.MaxAllocationSizeBytes)),
		hMinAllocationSizeBytes:           sizeToString(swag.Int64Value(o.MinAllocationSizeBytes)),
		hPreferredAllocationSizeBytes:     sizeToString(swag.Int64Value(o.PreferredAllocationSizeBytes)),
		hPreferredAllocationUnitSizeBytes: sizeToString(swag.Int64Value(o.PreferredAllocationUnitSizeBytes)),
		hProvisioningUnit:                 pUnitS,
		hParcelSizeBytes:                  sizeToString(swag.Int64Value(o.ParcelSizeBytes)),
		hAccessibilityScope:               string(o.AccessibilityScope),
	}
}

func (c *cspStorageTypeCmd) validateColumns(columns string) error {
	if matched, _ := regexp.MatchString("^\\s*$", columns); matched {
		c.tableCols = cspStorageTypeDefaultHeaders
	} else {
		c.tableCols = strings.Split(columns, ",")
		for _, col := range c.tableCols {
			if _, ok := cspStorageTypeHeaders[col]; !ok {
				return fmt.Errorf("invalid column \"%s\"", col)
			}
		}
	}
	return nil
}

func (c *cspStorageTypeCmd) Emit(data []*models.CSPStorageType) error {
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

func (c *cspStorageTypeCmd) list(params *csp_storage_type.CspStorageTypeListParams) ([]*models.CSPStorageType, error) {
	res, err := appCtx.API.CspStorageType().CspStorageTypeList(params)
	if err != nil {
		if e, ok := err.(*csp_storage_type.CspStorageTypeListDefault); ok && e.Payload.Message != nil {
			return nil, fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res.Payload, nil
}

type cspStorageTypeListCmd struct {
	DomainType string `short:"T" long:"domain-type" description:"Type of cloud service provider" choice:"AWS" choice:"Azure" choice:"GCP"`
	Columns    string `long:"columns" description:"Comma separated list of column names"`

	cspStorageTypeCmd
	remainingArgsCatcher
}

func (c *cspStorageTypeListCmd) Execute(args []string) error {
	var err error
	if err = c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	params := csp_storage_type.NewCspStorageTypeListParams()
	if c.DomainType != "" {
		params.CspDomainType = &c.DomainType
	}
	var res []*models.CSPStorageType
	if res, err = c.list(params); err != nil {
		return err
	}
	return c.Emit(res)
}
