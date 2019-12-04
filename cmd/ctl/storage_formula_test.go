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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_formula"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestStorageFormulaMakeRecord(t *testing.T) {
	assert := assert.New(t)

	ac := &storageFormulaCmd{}
	o := &models.StorageFormula{
		Name:        "9ms-rand-rw",
		Description: "9ms, Random, Read/Write",
		IoProfile: &models.IoProfile{
			IoPattern:    &models.IoPattern{Name: "random", MinSizeBytesAvg: swag.Int32(0), MaxSizeBytesAvg: swag.Int32(16384)},
			ReadWriteMix: &models.ReadWriteMix{Name: "read-write", MinReadPercent: swag.Int32(30), MaxReadPercent: swag.Int32(70)},
		},
		SscList: models.SscListMutable{
			"Response Time Average": {Kind: "DURATION", Value: "9ms"},
			"Availability":          {Kind: "PERCENTAGE", Value: "99.999%"},
		},
		StorageComponent: map[string]models.StorageFormulaTypeElement{
			"Amazon io1": {Percentage: swag.Int32(50)},
			"Amazon gp2": {Percentage: swag.Int32(50)},
		},
		CacheComponent: map[string]models.StorageFormulaTypeElement{
			"Amazon SSD": {Percentage: swag.Int32(20)},
			"Amazon HDD": {Percentage: swag.Int32(0)},
		},
		StorageLayout: "mirrored",
	}
	rec := ac.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(storageFormulaHeaders))
	assert.EqualValues(o.Name, rec[hName])
	assert.EqualValues(o.Description, rec[hDescription])
	assert.EqualValues("Amazon SSD:20%", rec[hCacheComponent])
	assert.EqualValues("Amazon gp2:50%\nAmazon io1:50%", rec[hStorageComponent])
	assert.EqualValues("random, read-write", rec[hIOProfile])
	assert.EqualValues("Availability:99.999%\nResponse Time Average:9ms", rec[hSscList])
	assert.EqualValues(o.StorageLayout, rec[hStorageLayout])
}

func TestStorageFormulaList(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	res := &storage_formula.StorageFormulaListOK{
		Payload: []*models.StorageFormula{
			{
				Description: "8ms, Random, Read/Write",
				Name:        "8ms-rand-rw",
				IoProfile: &models.IoProfile{
					IoPattern:    &models.IoPattern{Name: "random", MinSizeBytesAvg: swag.Int32(0), MaxSizeBytesAvg: swag.Int32(16384)},
					ReadWriteMix: &models.ReadWriteMix{Name: "read-write", MinReadPercent: swag.Int32(30), MaxReadPercent: swag.Int32(70)},
				},
				SscList: models.SscListMutable{
					"Response Time Average": {Kind: "DURATION", Value: "8ms"},
					"Availability":          {Kind: "PERCENTAGE", Value: "99.999%"},
				},
				StorageComponent: map[string]models.StorageFormulaTypeElement{
					"Amazon gp2": {Percentage: swag.Int32(100)},
				},
				CacheComponent: map[string]models.StorageFormulaTypeElement{
					"Amazon SSD": {Percentage: swag.Int32(20)},
				},
				StorageLayout: "mirrored",
			},
			{
				Description: "50ms, Streaming, Write Mostly",
				Name:        "50ms-stream-wm",
				IoProfile: &models.IoProfile{
					IoPattern:    &models.IoPattern{Name: "streaming", MinSizeBytesAvg: swag.Int32(262144), MaxSizeBytesAvg: swag.Int32(41944304)},
					ReadWriteMix: &models.ReadWriteMix{Name: "write-mostly", MinReadPercent: swag.Int32(0), MaxReadPercent: swag.Int32(30)},
				},
				SscList: models.SscListMutable{
					"Response Time Average": {Kind: "DURATION", Value: "100ms"},
					"Availability":          {Kind: "PERCENTAGE", Value: "99.9%"},
				},
				StorageComponent: map[string]models.StorageFormulaTypeElement{
					"Amazon gp2": {Percentage: swag.Int32(10)},
					"Amazon S3":  {Percentage: swag.Int32(90)},
				},
				CacheComponent: map[string]models.StorageFormulaTypeElement{
					"Amazon HDD": {Percentage: swag.Int32(10)},
				},
				StorageLayout: "standalone",
			},
		},
	}

	// list, with name
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mock.NewMockAPI(mockCtrl)
	cOps := mock.NewMockStorageFormulaClient(mockCtrl)
	mAPI.EXPECT().StorageFormula().Return(cOps)
	m := mock.NewStorageFormulaMatcher(t, storage_formula.NewStorageFormulaListParams().WithName(swag.String("8ms-rand-rw")))
	cOps.EXPECT().StorageFormulaList(m).Return(res, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageFormula()
	err := parseAndRun([]string{"storage-formulas", "list", "-n", "8ms-rand-rw"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(storageFormulaDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list default, json
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mock.NewMockAPI(mockCtrl)
	cOps = mock.NewMockStorageFormulaClient(mockCtrl)
	mAPI.EXPECT().StorageFormula().Return(cOps)
	m = mock.NewStorageFormulaMatcher(t, storage_formula.NewStorageFormulaListParams())
	cOps.EXPECT().StorageFormulaList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageFormula()
	err = parseAndRun([]string{"storage-formula", "list", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(res.Payload, te.jsonData)

	// list default, yaml
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mock.NewMockAPI(mockCtrl)
	cOps = mock.NewMockStorageFormulaClient(mockCtrl)
	mAPI.EXPECT().StorageFormula().Return(cOps)
	cOps.EXPECT().StorageFormulaList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageFormula()
	err = parseAndRun([]string{"sf", "list", "-o", "yaml"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(res.Payload, te.yamlData)

	// list with columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mock.NewMockAPI(mockCtrl)
	cOps = mock.NewMockStorageFormulaClient(mockCtrl)
	mAPI.EXPECT().StorageFormula().Return(cOps)
	cOps.EXPECT().StorageFormulaList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageFormula()
	err = parseAndRun([]string{"storage-formulas", "list", "--columns", "Name,Storage"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, 2)
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list with invalid columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mock.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageFormula()
	err = parseAndRun([]string{"storage-formula", "list", "--columns", "Name,Foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// list, API failure model error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mock.NewMockAPI(mockCtrl)
	cOps = mock.NewMockStorageFormulaClient(mockCtrl)
	mAPI.EXPECT().StorageFormula().Return(cOps)
	apiErr := &storage_formula.StorageFormulaListDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	cOps.EXPECT().StorageFormulaList(m).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageFormula()
	err = parseAndRun([]string{"sf", "list"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// list, API failure arbitrary error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mock.NewMockAPI(mockCtrl)
	cOps = mock.NewMockStorageFormulaClient(mockCtrl)
	mAPI.EXPECT().StorageFormula().Return(cOps)
	otherErr := fmt.Errorf("OTHER ERROR")
	cOps.EXPECT().StorageFormulaList(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageFormula()
	err = parseAndRun([]string{"sf", "list"})
	assert.NotNil(err)
	assert.Equal(otherErr, err)

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initStorageFormula()
	err = parseAndRun([]string{"storage-formula", "list", "o", "json"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}
