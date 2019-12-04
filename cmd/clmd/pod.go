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
	initPod()
}

func initPod() {
	cmd, _ := parser.AddCommand("pod", "Pod commands", "Pod Subcommands", &podCmd{})
	cmd.AddCommand("fetch", "Fetch a Pod", "Fetch a particular pod in a namespace", &podFetchCmd{})
	cmd.AddCommand("list", "List Pods", "Lists pods in a namespace matching a query", &podListCmd{})
}

type podCmd struct {
	tableCols []string
}

const (
	hPName           = "Pod Name"
	hPNamespace      = "Pod Namespace"
	hPControllerName = "Controller Name"
	hPControllerKind = "Controller Kind"
	hPControllerID   = "Controller ID"
)

var podHeaders = map[string]string{
	hPName:           "pod name",
	hPNamespace:      "pod namespace",
	hPControllerName: "controller name",
	hPControllerKind: "controller kind",
	hPControllerID:   "controller id",
}

var podDefaultHeaders = []string{hPName, hPNamespace, hPControllerName, hPControllerKind, hPControllerID}

func (c *podCmd) makeRecord(o *cluster.PodObj) map[string]string {
	return map[string]string{
		hPName:           o.Name,
		hPNamespace:      o.Namespace,
		hPControllerName: o.ControllerName,
		hPControllerKind: o.ControllerKind,
		hPControllerID:   o.ControllerID,
	}
}

func (c *podCmd) validateColumns(columns string) error {
	if matched, _ := regexp.MatchString("^\\s*$", columns); matched {
		c.tableCols = podDefaultHeaders
	} else {
		c.tableCols = strings.Split(columns, ",")
		for _, col := range c.tableCols {
			if _, ok := podHeaders[col]; !ok {
				return fmt.Errorf("invalid column \"%s\"", col)
			}
		}
	}
	return nil
}

func (c *podCmd) Emit(data []*cluster.PodObj) error {
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

type podFetchCmd struct {
	Name      string `short:"s" long:"name" description:"Specify the Pod name" required:"yes"`
	Namespace string `short:"n" long:"namespace" description:"Specify the Namespace of the Pod" required:"yes"`
	Columns   string `long:"columns" description:"Comma separated list of column names"`
	podCmd
}

func (c *podFetchCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	pfa := &cluster.PodFetchArgs{
		Name:      c.Name,
		Namespace: c.Namespace,
	}
	res, err := appCtx.K8sClient.PodFetch(appCtx.ctx, pfa)
	if err != nil {
		return err
	}
	return c.Emit([]*cluster.PodObj{res})
}

type podListCmd struct {
	Namespace string   `short:"n" long:"namespace" description:"Specify the Namespace of the Pod" required:"yes"`
	NodeName  string   `short:"N" long:"node-name" description:"Specify a node name"`
	Selectors []string `short:"l" long:"selector" description:"Selector (label query) to filter on, eg 'a=b', repeat as needed"`
	Columns   string   `long:"columns" description:"Comma separated list of column names"`
	podCmd
}

func (c *podListCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	pla := &cluster.PodListArgs{
		Namespace: c.Namespace,
		NodeName:  c.NodeName,
		Labels:    c.Selectors,
	}
	res, err := appCtx.K8sClient.PodList(appCtx.ctx, pla)
	if err != nil {
		return err
	}
	return c.Emit(res)
}
