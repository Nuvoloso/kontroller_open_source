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

	"github.com/Nuvoloso/kontroller/pkg/cluster"
)

func init() {
	initNode()
}

func initNode() {
	cmd, _ := parser.AddCommand("node", "Node commands", "Node Subcommands", &nodeCmd{})
	cmd.AddCommand("fetch", "Fetch a Node", "Fetch a particular node", &nodeFetchCmd{})
}

type nodeCmd struct {
	tableCols []string
}

const (
	hNodeName = "Node Name"
	hNodeUID  = "Node uid"
)

var nodeHeaders = map[string]string{
	hNodeName: "node name",
	hNodeUID:  "node uid",
}

var nodeDefaultHeaders = []string{hNodeName, hNodeUID}

func (c *nodeCmd) makeRecord(o *cluster.NodeObj) map[string]string {
	return map[string]string{
		hNodeName: o.Name,
	}
}

func (c *nodeCmd) validateColumns(columns string) error {
	if matched, _ := regexp.MatchString("^\\s*$", columns); matched {
		c.tableCols = nodeDefaultHeaders
	} else {
		c.tableCols = strings.Split(columns, ",")
		for _, col := range c.tableCols {
			if _, ok := nodeHeaders[col]; !ok {
				return fmt.Errorf("invalid column \"%s\"", col)
			}
		}
	}
	return nil
}

func (c *nodeCmd) Emit(data []*cluster.NodeObj) error {
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

type nodeFetchCmd struct {
	Name    string `short:"N" long:"node-name" description:"Specify the Node name" required:"yes"`
	Columns string `long:"columns" description:"Comma separated list of column names"`
	nodeCmd
}

func (c *nodeFetchCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	nfa := &cluster.NodeFetchArgs{
		Name: c.Name,
	}
	res, err := appCtx.K8sClient.NodeFetch(appCtx.ctx, nfa)
	if err != nil {
		return err
	}
	return c.Emit([]*cluster.NodeObj{res})
}
