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
	"bytes"
	"fmt"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/task"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
)

func init() {
	initTask()
}

func initTask() {
	cmd, _ := parser.AddCommand("task", "Task object commands", "Task object subcommands", &taskCmd{})
	cmd.AddCommand("list", "List Tasks", "List or search for Task objects.", &taskListCmd{})
	cmd.AddCommand("get", "Get a Task object", "Get information about a task.", &taskGetCmd{})
	cmd.AddCommand("cancel", "Cancel a Task", "Cancel a Task object.", &taskCancelCmd{})
	cmd.AddCommand("show-columns", "Show Task table columns", "Show names of columns used in table format", &showColsCmd{columns: taskHeaders})
}

type taskCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string
}

// task record keys/headers and their description
var taskHeaders = map[string]string{
	hID:           dID,
	hTimeCreated:  dTimeCreated,
	hTimeModified: dTimeModified,
	hOperation:    "The name of the operation",
	hState:        "The task state",
	hProgress:     "The percent complete",
}

var taskDefaultHeaders = []string{hID, hTimeModified, hState, hProgress}

// makeRecord creates a map of properties
func (c *taskCmd) makeRecord(o *models.Task) map[string]string {
	pct := ""
	if o.Progress != nil {
		pct = fmt.Sprintf("%d%%", swag.Int32Value(o.Progress.PercentComplete))
	}
	return map[string]string{
		hID:           string(o.Meta.ID),
		hTimeCreated:  time.Time(o.Meta.TimeCreated).Format(time.RFC3339),
		hTimeModified: time.Time(o.Meta.TimeModified).Format(time.RFC3339),
		hOperation:    o.Operation,
		hState:        o.State,
		hProgress:     pct,
	}
}

func (c *taskCmd) validateColumns(columns string) error {
	var err error
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(taskHeaders), taskDefaultHeaders)
	return err

}

func (c *taskCmd) Emit(data []*models.Task) error {
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

func (c *taskCmd) get(params *task.TaskFetchParams) (*models.Task, error) {
	res, err := appCtx.API.Task().TaskFetch(params)
	if err != nil {
		if e, ok := err.(*task.TaskFetchDefault); ok && e.Payload.Message != nil {
			return nil, fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res.Payload, nil
}

type taskGetCmd struct {
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`

	taskCmd
	requiredIDRemainingArgsCatcher
}

func (c *taskGetCmd) Execute(args []string) error {
	var err error
	if err = c.verifyRequiredIDAndNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	params := task.NewTaskFetchParams()
	params.ID = c.ID
	var tObj *models.Task
	if tObj, err = c.get(params); err != nil {
		return err
	}
	return c.Emit([]*models.Task{tObj})
}

func (c *taskCmd) list(params *task.TaskListParams) ([]*models.Task, error) {
	res, err := appCtx.API.Task().TaskList(params)
	if err != nil {
		if e, ok := err.(*task.TaskListDefault); ok && e.Payload.Message != nil {
			return nil, fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res.Payload, nil
}

type taskListCmd struct {
	Operation     string `short:"O" long:"operation" description:"An operation name"`
	Columns       string `short:"c" long:"columns" description:"Comma separated list of column names"`
	Follow        bool   `short:"f" long:"follow-changes" description:"Monitor the system and repeat the command as relevant changes occur"`
	FollowNoClear bool   `long:"follow-no-clear" description:"Do not clear the terminal (the default) when using follow-changes"`

	mustClearTerm bool
	params        *task.TaskListParams
	res           []*models.Task

	taskCmd
	remainingArgsCatcher
}

func (c *taskListCmd) Execute(args []string) error {
	var err error
	if err = c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	params := task.NewTaskListParams()
	if c.Operation != "" {
		params.Operation = &c.Operation
	}
	c.params = params
	if c.Follow {
		if !c.FollowNoClear {
			c.mustClearTerm = true
		}
		return appCtx.WatchForChange(c)
	}
	return c.run()
}

func (c *taskListCmd) run() error {
	var err error
	if c.res, err = c.list(c.params); err != nil {
		return err
	}
	// TBD: work on cache refresh on demand
	return c.display()
}

func (c *taskListCmd) display() error {
	if c.mustClearTerm {
		appCtx.ClearTerminal()
	}
	return c.Emit(c.res)
}

// ChangeDetected is part of the ChangeDetector interface
func (c *taskListCmd) ChangeDetected() error {
	return c.run()
}

// WatcherArgs is part of the ChangeDetector interface
func (c *taskListCmd) WatcherArgs() *models.CrudWatcherCreateArgs {
	cm := &models.CrudMatcher{
		URIPattern: "/tasks/?",
	}
	var scopeB bytes.Buffer
	if c.params.Operation != nil {
		fmt.Fprintf(&scopeB, ".*operation:%s", *c.params.Operation)
	}
	if scopeB.Len() > 0 {
		cm.ScopePattern = scopeB.String()
	}
	return &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{cm},
	}
}

type taskCancelCmd struct {
	Confirm bool `long:"confirm" description:"Confirm the cancellation of the task"`

	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`

	taskCmd
	requiredIDRemainingArgsCatcher
}

func (c *taskCancelCmd) Execute(args []string) error {
	if err := c.verifyRequiredIDAndNoRemainingArgs(); err != nil {
		return err
	}
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if !c.Confirm {
		return fmt.Errorf("specify --confirm to cancel the \"%s\" Task object", c.ID)
	}
	if err := appCtx.InitContextAccount(); err != nil {
		return err
	}
	cParams := task.NewTaskCancelParams()
	cParams.ID = string(c.ID)
	res, err := appCtx.API.Task().TaskCancel(cParams)
	if err != nil {
		if e, ok := err.(*task.TaskCancelDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.Task{res.Payload})
}
