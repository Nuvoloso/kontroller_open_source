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

	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/util"
)

func init() {
	initInstance()
}

func initInstance() {
	cmd, _ := parser.AddCommand("instances", "GCE Instance commands", "GCE Instance subcommands", &gceInstanceCmd{})
	cmd.Aliases = []string{"instance"}
	c, _ := cmd.AddCommand("list-cache-devices", "List cache devices", "List ephemeral devices that are eligible to be used as cache devices. Only works when run locally on a GCE instance", &gceInstanceCacheCmd{})
	c.Aliases = []string{"cache"}
}

type gceInstanceCmd struct {
	tableCols []string
}

const (
	hPath        = "Path"
	hDeviceType  = "Type"
	hDeviceSize  = "Size"
	hInitialized = "Init"
	hUsable      = "Use"
)

var gceInstanceHeaders = map[string]string{
	hPath:        "Device path",
	hDeviceType:  "Device type",
	hDeviceSize:  "Unformatted device size",
	hInitialized: "Device is pre-initialized for write performance",
	hUsable:      "Device can by used for caching",
}

var gceInstanceDefaultHeaders = []string{hPath, hDeviceType, hDeviceSize, hInitialized, hUsable}

func (c *gceInstanceCmd) makeRecord(o *csp.EphemeralDevice) map[string]string {
	init := ""
	if o.Initialized {
		init = "✔"
	}
	usable := ""
	if o.Usable {
		usable = "✔"
	}
	return map[string]string{
		hPath:        o.Path,
		hDeviceType:  o.Type,
		hDeviceSize:  util.SizeBytesToString(o.SizeBytes),
		hInitialized: init,
		hUsable:      usable,
	}
}

func (c *gceInstanceCmd) validateColumns(columns string) error {
	if matched, _ := regexp.MatchString("^\\s*$", columns); matched {
		c.tableCols = gceInstanceDefaultHeaders
	} else {
		c.tableCols = strings.Split(columns, ",")
		for _, col := range c.tableCols {
			if _, ok := gceInstanceHeaders[col]; !ok {
				return fmt.Errorf("invalid column \"%s\"", col)
			}
		}
	}
	return nil
}

func (c *gceInstanceCmd) Emit(data []*csp.EphemeralDevice) error {
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

type gceInstanceCacheCmd struct {
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`
	gceInstanceCmd
}

func (c *gceInstanceCacheCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := appCtx.ClientInit(&appCtx.CredFlags); err != nil {
		return err
	}
	devices, err := appCtx.GCPClient.LocalEphemeralDevices()
	if err != nil {
		return err
	}
	return c.Emit(devices)
}
