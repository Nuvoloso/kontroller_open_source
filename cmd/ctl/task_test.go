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
	"os"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/task"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	flags "github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestTaskMakeRecord(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	ac := &taskCmd{}
	o := &models.Task{
		TaskAllOf0: models.TaskAllOf0{
			Meta: &models.ObjMeta{
				ID:           "task-1",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
			Progress: &models.Progress{
				PercentComplete: swag.Int32(20),
			},
			State: "ACTIVE",
		},
		TaskCreateOnce: models.TaskCreateOnce{
			Operation: "OP",
		},
	}
	rec := ac.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(taskHeaders))
	assert.Equal("task-1", rec[hID])
	assert.Equal("OP", rec[hOperation])
	assert.Equal("ACTIVE", rec[hState])
	assert.Equal("20%", rec[hProgress])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeCreated])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeModified])
}

func TestTaskGet(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	params := task.NewTaskFetchParams()
	params.ID = "id"
	tM := mockmgmtclient.NewTaskMatcher(t, params)
	res := &task.TaskFetchOK{
		Payload: &models.Task{
			TaskAllOf0: models.TaskAllOf0{
				Meta: &models.ObjMeta{
					ID:           models.ObjID(params.ID),
					Version:      1,
					TimeCreated:  strfmt.DateTime(now),
					TimeModified: strfmt.DateTime(now),
				},
				Progress: &models.Progress{
					PercentComplete: swag.Int32(20),
				},
				State: "ACTIVE",
			},
			TaskCreateOnce: models.TaskCreateOnce{
				Operation: "OP",
			},
		},
	}

	// get, all defaults
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockTaskClient(mockCtrl)
	mAPI.EXPECT().Task().Return(cOps)
	cOps.EXPECT().TaskFetch(tM).Return(res, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initTask()
	err := parseAndRun([]string{"task", "get", "id"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(taskDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, 1)

	// get default, json (alias)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockTaskClient(mockCtrl)
	mAPI.EXPECT().Task().Return(cOps)
	cOps.EXPECT().TaskFetch(tM).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initTask()
	err = parseAndRun([]string{"task", "get", "--id", "id", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.Task{res.Payload}, te.jsonData)

	// get default, yaml
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockTaskClient(mockCtrl)
	mAPI.EXPECT().Task().Return(cOps)
	cOps.EXPECT().TaskFetch(tM).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initTask()
	err = parseAndRun([]string{"task", "get", "--id", "id", "-o", "yaml"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues([]*models.Task{res.Payload}, te.yamlData)

	// get with columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockTaskClient(mockCtrl)
	mAPI.EXPECT().Task().Return(cOps)
	cOps.EXPECT().TaskFetch(tM).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initTask()
	err = parseAndRun([]string{"task", "get", "--id", "id", "--columns", "Operation,TimeCreated"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, 2)
	t.Log(te.tableData)
	assert.Len(te.tableData, 1)

	// get with invalid columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initTask()
	err = parseAndRun([]string{"task", "get", "--id", "id", "--columns", "Name,Foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// get, API failure model error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockTaskClient(mockCtrl)
	mAPI.EXPECT().Task().Return(cOps)
	apiErr := &task.TaskFetchDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	cOps.EXPECT().TaskFetch(tM).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initTask()
	err = parseAndRun([]string{"task", "get", "--id", "id"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// get, API failure arbitrary error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockTaskClient(mockCtrl)
	mAPI.EXPECT().Task().Return(cOps)
	otherErr := fmt.Errorf("OTHER ERROR")
	cOps.EXPECT().TaskFetch(tM).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initTask()
	err = parseAndRun([]string{"task", "get", "--id", "id"})
	assert.NotNil(err)
	assert.Equal(otherErr, err)

	mockCtrl.Finish()
	t.Log("init context failure")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "Task").Return("", fmt.Errorf("ctx error"))
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initTask()
	err = parseAndRun([]string{"task", "get", "--id", "id", "-A", "Task"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("expected id")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initTask()
	err = parseAndRun([]string{"task", "get"})
	assert.NotNil(err)
	assert.Regexp("expected --id flag or a single identifier argument", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestTaskList(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	params := task.NewTaskListParams()
	tM := mockmgmtclient.NewTaskMatcher(t, params)
	res := &task.TaskListOK{
		Payload: []*models.Task{
			&models.Task{
				TaskAllOf0: models.TaskAllOf0{
					Meta: &models.ObjMeta{
						ID:           "ID-1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
					Progress: &models.Progress{
						PercentComplete: swag.Int32(20),
					},
					State: "ACTIVE",
				},
				TaskCreateOnce: models.TaskCreateOnce{
					Operation: "OP",
				},
			},
		},
	}

	// list, all defaults
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockTaskClient(mockCtrl)
	mAPI.EXPECT().Task().Return(cOps)
	cOps.EXPECT().TaskList(tM).Return(res, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initTask()
	err := parseAndRun([]string{"task", "list"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(taskDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list default, json
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockTaskClient(mockCtrl)
	mAPI.EXPECT().Task().Return(cOps)
	cOps.EXPECT().TaskList(tM).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initTask()
	err = parseAndRun([]string{"task", "list", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(res.Payload, te.jsonData)

	// list default, yaml
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockTaskClient(mockCtrl)
	mAPI.EXPECT().Task().Return(cOps)
	cOps.EXPECT().TaskList(tM).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initTask()
	err = parseAndRun([]string{"task", "list", "-o", "yaml"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(res.Payload, te.yamlData)

	// list with columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockTaskClient(mockCtrl)
	mAPI.EXPECT().Task().Return(cOps)
	cOps.EXPECT().TaskList(tM).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initTask()
	err = parseAndRun([]string{"task", "list", "--columns", "Operation,ID,State"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, 3)
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list with follow
	fw := &fakeWatcher{t: t}
	appCtx.watcher = fw
	fw.CallCBCount = 1
	fos := &fakeOSExecutor{}
	appCtx.osExec = fos
	mockCtrl.Finish()
	params.Operation = swag.String("OP")
	tM = mockmgmtclient.NewTaskMatcher(t, params)
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockTaskClient(mockCtrl)
	mAPI.EXPECT().Task().Return(cOps)
	cOps.EXPECT().TaskList(tM).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initTask()
	err = parseAndRun([]string{"task", "list", "-f", "-O", "OP"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(taskDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))
	expWArgs := &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{
			&models.CrudMatcher{
				URIPattern:   "/tasks/?",
				ScopePattern: ".*operation:OP",
			},
		},
	}
	assert.NotNil(fw.InArgs)
	assert.Equal(expWArgs, fw.InArgs)
	assert.NotNil(fw.InCB)
	assert.Equal(1, fw.NumCalls)
	assert.NoError(fw.CBErr)
	params.Operation = nil

	// list with invalid columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initTask()
	err = parseAndRun([]string{"task", "list", "-c", "Name,Foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// list, API failure model error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockTaskClient(mockCtrl)
	mAPI.EXPECT().Task().Return(cOps)
	apiErr := &task.TaskListDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	cOps.EXPECT().TaskList(tM).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initTask()
	err = parseAndRun([]string{"task", "list"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// list, API failure arbitrary error
	mockCtrl.Finish()
	params.Operation = swag.String("OP")
	tM = mockmgmtclient.NewTaskMatcher(t, params)
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockTaskClient(mockCtrl)
	mAPI.EXPECT().Task().Return(cOps)
	otherErr := fmt.Errorf("OTHER ERROR")
	cOps.EXPECT().TaskList(tM).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initTask()
	err = parseAndRun([]string{"task", "list", "-O", "OP"})
	assert.NotNil(err)
	assert.Equal(otherErr, err)

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initTask()
	err = parseAndRun([]string{"task", "list", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestTaskCancel(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	taskToCancel := "T-1"
	cParams := task.NewTaskCancelParams()
	cParams.ID = taskToCancel
	tM := mockmgmtclient.NewTaskMatcher(t, cParams)

	now := time.Now()
	cRet := &task.TaskCancelOK{
		Payload: &models.Task{
			TaskAllOf0: models.TaskAllOf0{
				Meta: &models.ObjMeta{
					ID:           "T-1",
					Version:      1,
					TimeCreated:  strfmt.DateTime(now),
					TimeModified: strfmt.DateTime(now),
				},
				Progress: &models.Progress{
					PercentComplete: swag.Int32(20),
				},
				State: "ACTIVE",
			},
			TaskCreateOnce: models.TaskCreateOnce{
				Operation: "OP",
			},
		},
	}

	// cancel
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockTaskClient(mockCtrl)
	mAPI.EXPECT().Task().Return(cOps).MinTimes(1)
	cOps.EXPECT().TaskCancel(tM).Return(cRet, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initTask()
	err := parseAndRun([]string{"task", "cancel", taskToCancel, "--confirm"})
	assert.Nil(err)

	// cancel, --confirm not specified
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initTask()
	err = parseAndRun([]string{"task", "cancel", "--id", taskToCancel})
	assert.NotNil(err)
	assert.Regexp("--confirm", err.Error())

	// cancel, API model error
	apiErr := &task.TaskCancelDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockTaskClient(mockCtrl)
	mAPI.EXPECT().Task().Return(cOps).MinTimes(1)
	cOps.EXPECT().TaskCancel(tM).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initTask()
	err = parseAndRun([]string{"task", "cancel", "--id", taskToCancel, "--confirm"})

	mockCtrl.Finish()
	t.Log("init context failure")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "System").Return("", fmt.Errorf("ctx error"))
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initTask()
	err = parseAndRun([]string{"task", "cancel", "-A", "System", "--id", taskToCancel, "--confirm"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// invalid columns
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initTask()
	err = parseAndRun([]string{"task", "cancel", "--id", taskToCancel, "--confirm", "--columns", "foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	t.Log("expected id")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initTask()
	err = parseAndRun([]string{"task", "cancel"})
	assert.NotNil(err)
	assert.Regexp("expected --id flag or a single identifier argument", err)
	appCtx.Account, appCtx.AccountID = "", ""
}
