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
	initEvent()
}

func initEvent() {
	cmd, _ := parser.AddCommand("event", "Event commands", "Event Subcommands", &eventCmd{})
	cmd.Aliases = []string{"ev"}
	ci, _ := cmd.AddCommand("create-incident", "Create an incident", "Create an incident in a pod", &eventCreateIncidentCmd{})
	ci.Aliases = []string{"ci"}
	cc, _ := cmd.AddCommand("create-condition", "Create a condition", "Create an condition in a pod", &eventCreateConditionCmd{})
	cc.Aliases = []string{"cc"}
}

type eventCmd struct {
	tableCols []string
}

const (
	hEMessage       = "Event Message"
	hEName          = "Event Name"
	hEType          = "Event Type"
	hEReason        = "Event Reason"
	hELastTimeStamp = "Last Timestamp"
)

var eventHeaders = map[string]string{
	hEName:          "event name",
	hEMessage:       "event message",
	hEType:          "event type",
	hEReason:        "event reason",
	hELastTimeStamp: "last timestamp",
}

var eventDefaultHeaders = []string{hEName, hEType, hEReason, hEMessage, hELastTimeStamp}

func (c *eventCmd) makeRecord(o *cluster.EventObj) map[string]string {
	return map[string]string{
		hEName:          o.Name,
		hEReason:        o.Reason,
		hEMessage:       o.Message,
		hEType:          o.Type,
		hELastTimeStamp: o.LastTimeStamp,
	}
}

func (c *eventCmd) validateColumns(columns string) error {
	if matched, _ := regexp.MatchString("^\\s*$", columns); matched {
		c.tableCols = eventDefaultHeaders
	} else {
		c.tableCols = strings.Split(columns, ",")
		for _, col := range c.tableCols {
			if _, ok := eventHeaders[col]; !ok {
				return fmt.Errorf("invalid column \"%s\"", col)
			}
		}
	}
	return nil
}

func (c *eventCmd) Emit(data []*cluster.EventObj) error {
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

type eventCreateIncidentCmd struct {
	Severity int    `short:"s" long:"severity" description:"Specify the Event Severity.  0: Normal, 1: Warning, 2: Fatal" default:"0"`
	Message  string `short:"m" long:"message" description:"Specify the Event message" required:"yes"`
	Columns  string `long:"columns" description:"Comma separated list of column names"`
	eventCmd
}

func (c *eventCreateIncidentCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if c.Severity > 2 || c.Severity < 0 {
		return fmt.Errorf("Severity option out of range: %d", c.Severity)
	}
	inc := &cluster.Incident{
		Severity: cluster.IncidentSeverity(c.Severity),
		Message:  c.Message,
	}
	res, err := appCtx.K8sClient.RecordIncident(appCtx.ctx, inc)
	if err != nil {
		return err
	}
	return c.Emit([]*cluster.EventObj{res})
}

type eventCreateConditionCmd struct {
	Status  int    `short:"s" long:"status" description:"Specify the Event Status.  1:ServiceReady, 2:ServiceNotReady" default:"0"`
	Columns string `long:"columns" description:"Comma separated list of column names"`
	eventCmd
}

func (c *eventCreateConditionCmd) Execute(args []string) error {
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if c.Status > 1 || c.Status < 0 {
		return fmt.Errorf("Status option out of range: %d", c.Status)
	}
	con := &cluster.Condition{
		Status: cluster.ConditionStatus(c.Status),
	}
	res, err := appCtx.K8sClient.RecordCondition(appCtx.ctx, con)
	if err != nil {
		return err
	}
	return c.Emit([]*cluster.EventObj{res})
}
