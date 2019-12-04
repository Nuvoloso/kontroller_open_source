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
	initController()
}

func initController() {
	cmd, _ := parser.AddCommand("controller", "Controller commands", "Controller Subcommands", &controllerCmd{})
	cmd.AddCommand("fetch", "Fetch a Controller", "Fetch a particular controller in a namespace", &controllerFetchCmd{})
}

type controllerCmd struct {
	tableCols []string
}

const (
	hCKind        = "Kind"
	hCName        = "Name"
	hCNamespace   = "Namespace"
	hCServiceName = "Controller Name"
	hCReplicas    = "Replicas"
)

var controllerHeaders = map[string]string{
	hCKind:        "controller kind",
	hCName:        "controller name",
	hCNamespace:   "controller namespace",
	hCServiceName: "controller name",
	hCReplicas:    "replicas",
}

var controllerDefaultHeaders = []string{hCKind, hCName, hCNamespace, hCServiceName, hCReplicas}

func (c *controllerCmd) makeRecord(o *cluster.ControllerObj) map[string]string {
	return map[string]string{
		hCKind:        o.Kind,
		hCName:        o.Name,
		hCNamespace:   o.Namespace,
		hCServiceName: o.ServiceName,
		hCReplicas:    fmt.Sprintf("%d", o.Replicas),
	}
}

func (c *controllerCmd) validateColumns(columns string) error {
	if matched, _ := regexp.MatchString("^\\s*$", columns); matched {
		c.tableCols = controllerDefaultHeaders
	} else {
		c.tableCols = strings.Split(columns, ",")
		for _, col := range c.tableCols {
			if _, ok := controllerHeaders[col]; !ok {
				return fmt.Errorf("invalid column \"%s\"", col)
			}
		}
	}
	return nil
}

func (c *controllerCmd) Emit(data []*cluster.ControllerObj) error {
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

type controllerFetchCmd struct {
	Kind      string `short:"k" long:"kind" description:"Specify the Controller kind" required:"yes"`
	Name      string `short:"c" long:"name" description:"Specify the Controller name" required:"yes"`
	Namespace string `short:"n" long:"namespace" description:"Specify the Namespace of the Controller" default:"default"`
	Columns   string `long:"columns" description:"Comma separated list of column names"`
	controllerCmd
}

func (c *controllerFetchCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	pfa := &cluster.ControllerFetchArgs{
		Kind:      c.Kind,
		Name:      c.Name,
		Namespace: c.Namespace,
	}
	res, err := appCtx.K8sClient.ControllerFetch(appCtx.ctx, pfa)
	if err != nil {
		return err
	}
	return c.Emit([]*cluster.ControllerObj{res})
}
