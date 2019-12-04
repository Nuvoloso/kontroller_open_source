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
	"reflect"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/slo"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

// Matcher for SLO
type mockSloMatchCtx int

const (
	mockSloListParam mockSloMatchCtx = iota
)

type mockSloMatcher struct {
	t         *testing.T
	ctx       mockSloMatchCtx
	listParam *slo.SloListParams
}

func newSloMatcher(t *testing.T, ctx mockSloMatchCtx) *mockSloMatcher {
	return &mockSloMatcher{t: t, ctx: ctx}
}

// Matcher ends the chain
func (o *mockSloMatcher) Matcher() gomock.Matcher {
	return o
}

// Matches is from gomock.Matcher
func (o *mockSloMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	switch o.ctx {
	case mockSloListParam:
		p := x.(*slo.SloListParams)
		assert.EqualValues(o.listParam, p)
		return reflect.DeepEqual(o.listParam, p)
	}
	return false
}

// String is from gomock.Matcher
func (o *mockSloMatcher) String() string {
	switch o.ctx {
	case mockSloListParam:
		return "matches on list"
	}
	return "unknown context"
}

func TestSloMakeRecord(t *testing.T) {
	assert := assert.New(t)

	ac := &sloCmd{}
	o := &models.SLO{
		Name:        "MySLO",
		Description: "A Test SLO",
		Choices: []*models.RestrictedValueType{
			{
				ValueType:                 models.ValueType{Kind: "DURATION", Value: "5ms"},
				RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: true},
			},
			{
				ValueType:                 models.ValueType{Kind: "DURATION", Value: "10ms"},
				RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: true},
			},
			{
				ValueType:                 models.ValueType{Kind: "DURATION", Value: "50ms"},
				RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: true},
			},
		},
	}
	rec := ac.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(sloHeaders))
	assert.Equal("MySLO", rec[hName])
	assert.Equal("A Test SLO", rec[hDescription])
	assert.Equal("DURATION", rec[hUnitDimension])
	assert.Equal("5ms, 10ms, 50ms", rec[hChoices])
}

func TestSloList(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	params := slo.NewSloListParams()
	params.NamePattern = swag.String("")
	m := newSloMatcher(t, mockSloListParam)
	m.listParam = params
	res := &slo.SloListOK{
		Payload: []*models.SLO{
			&models.SLO{
				Name:        models.ObjName("Security"),
				Description: models.ObjDescription("Security level in terms of data encryption"),
				Choices: []*models.RestrictedValueType{
					{
						ValueType:                 models.ValueType{Kind: "STRING", Value: "High Encryption"},
						RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: true},
					},
					{
						ValueType:                 models.ValueType{Kind: "STRING", Value: "No Encryption"},
						RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: true},
					},
				},
			},
			&models.SLO{
				Name:        models.ObjName("Response Time Average"),
				Description: models.ObjDescription("Maximum response time target for the storage, in milliseconds"),
				Choices: []*models.RestrictedValueType{
					{
						ValueType:                 models.ValueType{Kind: "DURATION", Value: "1ms"},
						RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: true},
					},
					{
						ValueType:                 models.ValueType{Kind: "DURATION", Value: "5ms"},
						RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: true},
					},
					{
						ValueType:                 models.ValueType{Kind: "DURATION", Value: "10ms"},
						RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: true},
					},
				},
			},
		},
	}

	// list, all defaults
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockSLOClient(mockCtrl)
	mAPI.EXPECT().Slo().Return(cOps)
	cOps.EXPECT().SloList(m.Matcher()).Return(res, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSlo()
	err := parseAndRun([]string{"slo", "list"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(sloDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list default, json
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockSLOClient(mockCtrl)
	mAPI.EXPECT().Slo().Return(cOps)
	cOps.EXPECT().SloList(m.Matcher()).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSlo()
	err = parseAndRun([]string{"slo", "list", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(res.Payload, te.jsonData)

	// list default, yaml
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockSLOClient(mockCtrl)
	mAPI.EXPECT().Slo().Return(cOps)
	cOps.EXPECT().SloList(m.Matcher()).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSlo()
	err = parseAndRun([]string{"slo", "list", "-o", "yaml"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(res.Payload, te.yamlData)

	// list with columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockSLOClient(mockCtrl)
	mAPI.EXPECT().Slo().Return(cOps)
	cOps.EXPECT().SloList(m.Matcher()).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSlo()
	err = parseAndRun([]string{"slo", "list", "--columns", "Name,Choices"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, 2)
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list with invalid columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSlo()
	err = parseAndRun([]string{"slo", "list", "-c", "Name,Foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// list, API failure model error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockSLOClient(mockCtrl)
	mAPI.EXPECT().Slo().Return(cOps)
	apiErr := &slo.SloListDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	cOps.EXPECT().SloList(m.Matcher()).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSlo()
	err = parseAndRun([]string{"slo", "list"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// list, API failure arbitrary error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockSLOClient(mockCtrl)
	mAPI.EXPECT().Slo().Return(cOps)
	otherErr := fmt.Errorf("OTHER ERROR")
	cOps.EXPECT().SloList(m.Matcher()).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSlo()
	err = parseAndRun([]string{"slo", "list"})
	assert.NotNil(err)
	assert.Equal(otherErr, err)

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSlo()
	err = parseAndRun([]string{"slo", "list", "o", "json"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}
