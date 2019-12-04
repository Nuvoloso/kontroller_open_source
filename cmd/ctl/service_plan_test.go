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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/alecthomas/units"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestServicePlanMakeRecord(t *testing.T) {
	assert := assert.New(t)
	cmd := &servicePlanCmd{}
	cmd.accounts = map[string]ctxIDName{
		"SystemID": {"", "System"},
	}
	cmd.servicePlans = map[string]string{
		"SERVICE-PLAN-0": "service-plan-0",
	}
	o := &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			Meta: &models.ObjMeta{
				ID: "SERVICE-PLAN-1",
			},
			IoProfile: &models.IoProfile{
				IoPattern:    &models.IoPattern{Name: "sequential", MinSizeBytesAvg: swag.Int32(16384), MaxSizeBytesAvg: swag.Int32(262144)},
				ReadWriteMix: &models.ReadWriteMix{Name: "read-write", MinReadPercent: swag.Int32(30), MaxReadPercent: swag.Int32(70)},
			},
			ProvisioningUnit: &models.ProvisioningUnit{
				IOPS:       swag.Int64(0),
				Throughput: swag.Int64(94 * int64(units.MB)),
			},
			VolumeSeriesMinMaxSize: &models.VolumeSeriesMinMaxSize{
				MinSizeBytes: swag.Int64(1 * int64(units.GiB)),
				MaxSizeBytes: swag.Int64(64 * int64(units.TiB)),
			},
			State:               "PUBLISHED",
			SourceServicePlanID: "SERVICE-PLAN-0",
		},
		ServicePlanMutable: models.ServicePlanMutable{
			Name:        "Service Plan Name",
			Description: "Service Plan Description",
			Accounts:    []models.ObjIDMutable{"SystemID"},
			Slos: models.SloListMutable{
				"Availability": models.RestrictedValueType{
					ValueType: models.ValueType{
						Kind:  "STRING",
						Value: "99.999%",
					},
					RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{
						Immutable: true,
					},
				},
			},
			Tags: []string{"tag1", "tag2"},
		},
	}
	rec := cmd.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(servicePlanHeaders))
	assert.EqualValues(o.Meta.ID, rec[hID])
	assert.EqualValues(o.Name, rec[hName])
	assert.EqualValues(o.Description, rec[hDescription])
	assert.Equal("System", rec[hAccounts])
	assert.Equal("service-plan-0", rec[hSourceServicePlan])
	assert.Equal(o.State, rec[hState])
	assert.Equal("Availability:99.999%", rec[hSLO])
	assert.Equal("sequential(avgSz:16KiB-256KiB), read-write(read:30-70%)", rec[hIOProfile])
	assert.Equal("94MB/s/GiB", rec[hProvisioningUnit])
	assert.Equal("1GiB", rec[hVSMinSize])
	assert.Equal("64TiB", rec[hVSMaxSize])
	assert.Equal("tag1, tag2", rec[hTags])

	// Testing SP Provisioning Unit as IOPS
	o = &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			Meta: &models.ObjMeta{
				ID: "SERVICE-PLAN-1",
			},
			IoProfile: &models.IoProfile{
				IoPattern:    &models.IoPattern{Name: "sequential", MinSizeBytesAvg: swag.Int32(16384), MaxSizeBytesAvg: swag.Int32(262144)},
				ReadWriteMix: &models.ReadWriteMix{Name: "read-write", MinReadPercent: swag.Int32(30), MaxReadPercent: swag.Int32(70)},
			},
			ProvisioningUnit: &models.ProvisioningUnit{
				IOPS:       swag.Int64(22),
				Throughput: swag.Int64(0),
			},
			VolumeSeriesMinMaxSize: &models.VolumeSeriesMinMaxSize{
				MinSizeBytes: swag.Int64(1 * int64(units.GiB)),
				MaxSizeBytes: swag.Int64(64 * int64(units.TiB)),
			},
			State:               "PUBLISHED",
			SourceServicePlanID: "SERVICE-PLAN-0",
		},
		ServicePlanMutable: models.ServicePlanMutable{
			Name:        "Service Plan Name",
			Description: "Service Plan Description",
			Accounts:    []models.ObjIDMutable{"SystemID"},
			Slos: models.SloListMutable{
				"Availability": models.RestrictedValueType{
					ValueType: models.ValueType{
						Kind:  "STRING",
						Value: "99.999%",
					},
					RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{
						Immutable: true,
					},
				},
			},
			Tags: []string{"tag1", "tag2"},
		},
	}
	rec = cmd.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(servicePlanHeaders))
	assert.EqualValues(o.Meta.ID, rec[hID])
	assert.EqualValues(o.Name, rec[hName])
	assert.EqualValues(o.Description, rec[hDescription])
	assert.Equal("System", rec[hAccounts])
	assert.Equal("service-plan-0", rec[hSourceServicePlan])
	assert.Equal(o.State, rec[hState])
	assert.Equal("Availability:99.999%", rec[hSLO])
	assert.Equal("sequential(avgSz:16KiB-256KiB), read-write(read:30-70%)", rec[hIOProfile])
	assert.Equal("22IOPS/GiB", rec[hProvisioningUnit])
	assert.Equal("1GiB", rec[hVSMinSize])
	assert.Equal("64TiB", rec[hVSMaxSize])
	assert.Equal("tag1, tag2", rec[hTags])

	// cacheAccounts is a singleton
	err := cmd.cacheAccounts()
	assert.Nil(err)

	// cacheServicePlans is a singleton
	err = cmd.cacheServicePlans()
	assert.Nil(err)

	// ids not in maps
	o.SourceServicePlanID = models.ObjIDMutable("FOO")
	cmd.accounts = map[string]ctxIDName{}
	rec = cmd.makeRecord(o)
	t.Log(rec)
	assert.Equal("SystemID", rec[hAccounts])
	assert.Equal("FOO", rec[hSourceServicePlan])
}

func TestServicePlanList(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	resAccounts := &account.AccountListOK{
		Payload: []*models.Account{
			&models.Account{
				AccountAllOf0: models.AccountAllOf0{
					Meta: &models.ObjMeta{
						ID: "SystemID",
					},
				},
				AccountMutable: models.AccountMutable{
					Name: "System",
				},
			},
			&models.Account{
				AccountAllOf0: models.AccountAllOf0{
					Meta: &models.ObjMeta{
						ID: "authAccountID",
					},
				},
				AccountMutable: models.AccountMutable{
					Name: "AuthAccount",
				},
			},
		},
	}
	resSP := &service_plan.ServicePlanListOK{
		Payload: []*models.ServicePlan{
			&models.ServicePlan{
				ServicePlanAllOf0: models.ServicePlanAllOf0{
					Meta: &models.ObjMeta{
						ID: "SERVICE-PLAN-1",
					},
				},
				ServicePlanMutable: models.ServicePlanMutable{
					Name: "Service Plan Name",
				},
			},
			&models.ServicePlan{
				ServicePlanAllOf0: models.ServicePlanAllOf0{
					Meta: &models.ObjMeta{
						ID: "SERVICE-PLAN-0",
					},
				},
				ServicePlanMutable: models.ServicePlanMutable{
					Name: "service-plan-0",
				},
			},
		},
	}
	res := &service_plan.ServicePlanListOK{
		Payload: []*models.ServicePlan{
			&models.ServicePlan{
				ServicePlanAllOf0: models.ServicePlanAllOf0{
					Meta: &models.ObjMeta{
						ID: "SERVICE-PLAN-1",
					},
					IoProfile: &models.IoProfile{
						IoPattern:    &models.IoPattern{Name: "sequential", MinSizeBytesAvg: swag.Int32(16384), MaxSizeBytesAvg: swag.Int32(262144)},
						ReadWriteMix: &models.ReadWriteMix{Name: "read-write", MinReadPercent: swag.Int32(30), MaxReadPercent: swag.Int32(70)},
					},
					ProvisioningUnit: &models.ProvisioningUnit{
						IOPS:       swag.Int64(0),
						Throughput: swag.Int64(94 * int64(units.MB)),
					},
					VolumeSeriesMinMaxSize: &models.VolumeSeriesMinMaxSize{
						MinSizeBytes: swag.Int64(1 * int64(units.GiB)),
						MaxSizeBytes: swag.Int64(64 * int64(units.TiB)),
					},
					State:               "PUBLISHED",
					SourceServicePlanID: "SERVICE-PLAN-0",
				},
				ServicePlanMutable: models.ServicePlanMutable{
					Name:        "Service Plan Name",
					Description: "Service Plan Description",
					Accounts:    []models.ObjIDMutable{"SystemID"},
					Slos: models.SloListMutable{
						"Availability": models.RestrictedValueType{
							ValueType: models.ValueType{
								Kind:  "STRING",
								Value: "99.999%",
							},
							RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{
								Immutable: true,
							},
						},
					},
					Tags: []string{"tag1", "tag2"},
				},
			},
		},
	}

	// list, all defaults
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	am := mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	aOps.EXPECT().AccountList(am).Return(resAccounts, nil)
	mAPI.EXPECT().Account().Return(aOps)
	cOps := mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(cOps).MinTimes(1)
	m := mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	first := cOps.EXPECT().ServicePlanList(m).Return(res, nil)
	cOps.EXPECT().ServicePlanList(m).Return(resSP, nil).After(first)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlan()
	err := parseAndRun([]string{"service-plans", "list"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(servicePlanDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list with name, json
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("list with name, json")
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(cOps)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "System").Return("accountID", nil)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	aOps.EXPECT().AccountList(am).Return(resAccounts, nil)
	mAPI.EXPECT().Account().Return(aOps)
	params := service_plan.NewServicePlanListParams()
	params.Name = swag.String("name")
	params.AuthorizedAccountID = swag.String("authAccountID")
	m = mockmgmtclient.NewServicePlanMatcher(t, params)
	cOps.EXPECT().ServicePlanList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlan()
	err = parseAndRun([]string{"plans", "list", "-A", "System", "-Z", "AuthAccount", "-n", "name", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(res.Payload, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// list with source, yaml
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("list with source, yaml")
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "AuthAccount").Return("authAccountID", nil)
	cOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(cOps).MinTimes(1)
	m = mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	first = cOps.EXPECT().ServicePlanList(m).Return(resSP, nil)
	params = service_plan.NewServicePlanListParams()
	params.SourceServicePlanID = swag.String("SERVICE-PLAN-0")
	params.AuthorizedAccountID = swag.String("authAccountID")
	m = mockmgmtclient.NewServicePlanMatcher(t, params)
	cOps.EXPECT().ServicePlanList(m).Return(res, nil).After(first)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlan()
	err = parseAndRun([]string{"service-plan", "list", "-A", "AuthAccount", "--owner-auth", "-s", "service-plan-0", "-o", "yaml"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(res.Payload, te.yamlData)
	appCtx.Account, appCtx.AccountID = "", ""

	// list with columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("list with columns")
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	aOps.EXPECT().AccountList(am).Return(resAccounts, nil)
	mAPI.EXPECT().Account().Return(aOps)
	cOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(cOps).MinTimes(1)
	m = mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	first = cOps.EXPECT().ServicePlanList(m).Return(res, nil)
	cOps.EXPECT().ServicePlanList(m).Return(resSP, nil).After(first)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlan()
	err = parseAndRun([]string{"sp", "list", "--columns", "Name,State"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, 2)
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list with invalid columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("list with invalid columns")
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlan()
	err = parseAndRun([]string{"sps", "list", "-c", "Name,Foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// list with source-service-plan, caching fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("list with source-service-plan, caching fails")
	lErr := fmt.Errorf("LIST FAILURE")
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(cOps).MinTimes(1)
	m = mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	cOps.EXPECT().ServicePlanList(m).Return(nil, lErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlan()
	err = parseAndRun([]string{"service-plan", "list", "-s", "FOO"})
	assert.NotNil(err)
	assert.Equal(lErr.Error(), err.Error())

	// list with invalid source-service-plan
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("list with invalid source-service-plan")
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(cOps).MinTimes(1)
	m = mockmgmtclient.NewServicePlanMatcher(t, service_plan.NewServicePlanListParams())
	cOps.EXPECT().ServicePlanList(m).Return(resSP, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlan()
	err = parseAndRun([]string{"service-plan", "list", "-s", "FOO"})
	assert.NotNil(err)
	assert.Regexp("service plan.*not found", err.Error())

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("list with invalid authorized account")
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	aOps.EXPECT().AccountList(am).Return(resAccounts, nil)
	mAPI.EXPECT().Account().Return(aOps)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlan()
	err = parseAndRun([]string{"service-plan", "list", "--auth-account-name", "FOO"})
	assert.NotNil(err)
	assert.Regexp("authorized account.*not found", err.Error())

	// list succeeds even if caching fails (no filter, show columns that prefer mapping)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("list succeeds even if caching fails")
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	aOps.EXPECT().AccountList(am).Return(nil, lErr)
	mAPI.EXPECT().Account().Return(aOps)
	cOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(cOps).MinTimes(1)
	first = cOps.EXPECT().ServicePlanList(m).Return(res, nil)
	cOps.EXPECT().ServicePlanList(m).Return(nil, lErr).After(first)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlan()
	err = parseAndRun([]string{"service-plans", "list", "--columns", "Accounts,SourceServicePlan"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, 2)
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))
	assert.NotContains(te.tableData[0], "System")
	assert.NotContains(te.tableData[0], "service-plan-0")
	assert.Contains(te.tableData[0], "SystemID")
	assert.Contains(te.tableData[0], "SERVICE-PLAN-0")

	// list, API failure model error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("list, API failure model error")
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(cOps)
	apiErr := &service_plan.ServicePlanListDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	cOps.EXPECT().ServicePlanList(m).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlan()
	err = parseAndRun([]string{"service-plan", "list"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// list, API failure arbitrary error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("list, API failure arbitrary error")
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(cOps)
	otherErr := fmt.Errorf("OTHER ERROR")
	cOps.EXPECT().ServicePlanList(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlan()
	err = parseAndRun([]string{"service-plan", "list"})
	assert.NotNil(err)
	assert.Equal(otherErr, err)

	mockCtrl.Finish()
	t.Log("init context failure")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "System").Return("", fmt.Errorf("ctx error"))
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlan()
	err = parseAndRun([]string{"sp", "list", "-A", "System"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlan()
	err = parseAndRun([]string{"sp", "list", "-Z", "auth", "--owner-auth"})
	assert.Error(err)
	assert.Regexp("do not specify --auth-account-name and --owner-auth together", err)

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlan()
	err = parseAndRun([]string{"sp", "list", "--owner-auth"})
	assert.Error(err)
	assert.Regexp("owner-auth requires --account", err)

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlan()
	err = parseAndRun([]string{"plans", "list", "o", "json"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestServicePlanModify(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	res := &service_plan.ServicePlanListOK{
		Payload: []*models.ServicePlan{
			&models.ServicePlan{
				ServicePlanAllOf0: models.ServicePlanAllOf0{
					Meta: &models.ObjMeta{
						ID: "SERVICE-PLAN-1",
					},
					State:               "PUBLISHED",
					SourceServicePlanID: "SERVICE-PLAN-0",
				},
				ServicePlanMutable: models.ServicePlanMutable{
					Name:        "Service Plan Name",
					Description: "Service Plan Description",
					Accounts:    []models.ObjIDMutable{"SystemID"},
				},
			},
		},
	}
	aM := mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	resAccounts := &account.AccountListOK{
		Payload: []*models.Account{
			&models.Account{
				AccountAllOf0: models.AccountAllOf0{
					Meta: &models.ObjMeta{
						ID: "SystemID",
					},
				},
				AccountMutable: models.AccountMutable{
					Name: "System",
				},
			},
			&models.Account{
				AccountAllOf0: models.AccountAllOf0{
					Meta: &models.ObjMeta{
						ID: "otherID1",
					},
				},
				AccountMutable: models.AccountMutable{
					Name: "Other",
				},
			},
			&models.Account{
				AccountAllOf0: models.AccountAllOf0{
					Meta: &models.ObjMeta{
						ID: "otherID2",
					},
				},
				AccountMutable: models.AccountMutable{
					Name: "Another",
				},
			},
		},
	}
	lParams := service_plan.NewServicePlanListParams().WithName(swag.String(string(res.Payload[0].Name)))
	m := mockmgmtclient.NewServicePlanMatcher(t, lParams)

	// update (APPEND)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	pOps := mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(pOps).MinTimes(1)
	pOps.EXPECT().ServicePlanList(m).Return(res, nil)
	uParams := service_plan.NewServicePlanUpdateParams()
	uParams.Payload = &models.ServicePlanMutable{
		Accounts: []models.ObjIDMutable{"otherID2", "otherID1"},
	}
	uParams.ID = string(res.Payload[0].Meta.ID)
	uParams.Append = []string{"accounts"}
	um := mockmgmtclient.NewServicePlanMatcher(t, uParams)
	uRet := service_plan.NewServicePlanUpdateOK()
	uRet.Payload = res.Payload[0]
	pOps.EXPECT().ServicePlanUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlan()
	err := parseAndRun([]string{"plan", "modify", "-n", *lParams.Name,
		"-Z", "Another", "-Z", "Other", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.ServicePlan{uRet.Payload}, te.jsonData)

	// same but with SET, owner-auth
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "System").Return("SystemID", nil)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	pOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(pOps).MinTimes(1)
	pOps.EXPECT().ServicePlanList(m).Return(res, nil)
	uParams = service_plan.NewServicePlanUpdateParams()
	uParams.Payload = &models.ServicePlanMutable{
		Accounts: []models.ObjIDMutable{"SystemID", "otherID1", "otherID2"},
	}
	uParams.ID = string(res.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"accounts"}
	um = mockmgmtclient.NewServicePlanMatcher(t, uParams)
	uRet = service_plan.NewServicePlanUpdateOK()
	uRet.Payload = res.Payload[0]
	pOps.EXPECT().ServicePlanUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlan()
	err = parseAndRun([]string{"service-plan", "-A", "System", "modify", "-n", *lParams.Name,
		"-Z", "Other", "-Z", "Another", "--owner-auth", "--accounts-action", "SET", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.ServicePlan{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// same but with REMOVE
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	aOps.EXPECT().AccountList(aM).Return(resAccounts, nil)
	pOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(pOps).MinTimes(1)
	pOps.EXPECT().ServicePlanList(m).Return(res, nil)
	uParams = service_plan.NewServicePlanUpdateParams()
	uParams.Payload = &models.ServicePlanMutable{
		Accounts: []models.ObjIDMutable{"otherID1", "otherID2"},
	}
	uParams.ID = string(res.Payload[0].Meta.ID)
	uParams.Remove = []string{"accounts"}
	um = mockmgmtclient.NewServicePlanMatcher(t, uParams)
	uRet = service_plan.NewServicePlanUpdateOK()
	uRet.Payload = res.Payload[0]
	pOps.EXPECT().ServicePlanUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlan()
	err = parseAndRun([]string{"plan", "modify", "-n", *lParams.Name,
		"-Z", "Other", "-Z", "Another", "--accounts-action", "REMOVE", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.ServicePlan{uRet.Payload}, te.jsonData)

	// same but empty SET
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	pOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
	mAPI.EXPECT().ServicePlan().Return(pOps).MinTimes(1)
	pOps.EXPECT().ServicePlanList(m).Return(res, nil)
	uParams = service_plan.NewServicePlanUpdateParams()
	uParams.Payload = &models.ServicePlanMutable{}
	uParams.ID = string(res.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"accounts"}
	um = mockmgmtclient.NewServicePlanMatcher(t, uParams)
	uRet = service_plan.NewServicePlanUpdateOK()
	uRet.Payload = res.Payload[0]
	pOps.EXPECT().ServicePlanUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlan()
	err = parseAndRun([]string{"plan", "modify", "-n", *lParams.Name,
		"--accounts-action", "SET", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.ServicePlan{uRet.Payload}, te.jsonData)

	mockCtrl.Finish()
	t.Log("init context failure")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "System").Return("", fmt.Errorf("ctx error"))
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlan()
	err = parseAndRun([]string{"sp", "modify", "-A", "System", "-n", *lParams.Name})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// other error cases
	updateErr := service_plan.NewServicePlanUpdateDefault(400)
	updateErr.Payload = &models.Error{Code: 400, Message: swag.String("update error")}
	otherErr := fmt.Errorf("other error")
	errTCs := []struct {
		name      string
		args      []string
		re        string
		clErr     error
		clRC      *service_plan.ServicePlanListOK
		alErr     error
		alRC      *account.AccountListOK
		updateErr error
		noMock    bool
		noAL      bool
	}{
		{
			name: "No modifications",
			args: []string{"plan", "modify", "-n", *lParams.Name, "-V", "1", "-o", "json"},
			re:   "No modifications",
			noAL: true,
		},
		{
			name:      "Update default error",
			args:      []string{"plan", "modify", "-n", *lParams.Name, "-Z", "Other", "-V", "1", "-o", "json"},
			re:        "update error",
			updateErr: updateErr,
		},
		{
			name:      "Update other error",
			args:      []string{"plan", "modify", "-n", *lParams.Name, "-Z", "Other", "-V", "1", "-o", "json"},
			re:        "other error",
			updateErr: otherErr,
		},
		{
			name:   "invalid columns",
			args:   []string{"plan", "modify", "-n", *lParams.Name, "--columns", "ID,foo"},
			re:     "invalid column",
			noMock: true,
		},
		{
			name:   "invalid columns2",
			args:   []string{"plan", "modify", "-n", *lParams.Name, "-c", "ID,foo"},
			re:     "invalid column",
			noMock: true,
		},
		{
			name: "plan not found",
			args: []string{"plan", "modify", "-n", *lParams.Name, "-Z", "Other", "-V", "1", "-o", "json"},
			re:   "ServicePlan.*not found",
			clRC: &service_plan.ServicePlanListOK{Payload: []*models.ServicePlan{}},
			noAL: true,
		},
		{
			name:  "plan list error",
			args:  []string{"plan", "modify", "-n", *lParams.Name, "-Z", "Other", "-V", "1", "-o", "json"},
			re:    "other error",
			clErr: otherErr,
			noAL:  true,
		},
		{
			name: "account not found",
			args: []string{"plan", "modify", "-n", *lParams.Name, "-Z", "Other", "-V", "1", "-o", "json"},
			re:   "account.*not found",
			alRC: &account.AccountListOK{Payload: []*models.Account{}},
		},
		{
			name:  "account list error",
			args:  []string{"plan", "modify", "-n", *lParams.Name, "-Z", "Other", "-V", "1", "-o", "json"},
			re:    "other error",
			alErr: otherErr,
		},
		{
			name:   "bad owner-auth",
			args:   []string{"plan", "modify", "-n", *lParams.Name, "--owner-auth"},
			re:     "owner-auth requires --account",
			noMock: true,
		},
	}
	for _, tc := range errTCs {
		t.Logf("case: %s", tc.name)
		if !tc.noMock {
			mockCtrl.Finish()
			mockCtrl = gomock.NewController(t)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			pOps = mockmgmtclient.NewMockServicePlanClient(mockCtrl)
			mAPI.EXPECT().ServicePlan().Return(pOps).MinTimes(1)
			if tc.clErr != nil {
				pOps.EXPECT().ServicePlanList(m).Return(nil, tc.clErr)
			} else {
				if tc.clRC == nil {
					tc.clRC = res
				}
				pOps.EXPECT().ServicePlanList(m).Return(tc.clRC, nil)
			}
			if tc.updateErr != nil {
				pOps.EXPECT().ServicePlanUpdate(gomock.Not(gomock.Nil)).Return(nil, tc.updateErr)
			}
			if !tc.noAL {
				aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
				mAPI.EXPECT().Account().Return(aOps)
				if tc.alErr != nil {
					aOps.EXPECT().AccountList(aM).Return(nil, tc.alErr)
				} else {
					if tc.alRC == nil {
						tc.alRC = resAccounts
					}
					aOps.EXPECT().AccountList(aM).Return(tc.alRC, nil)
				}
			}
			appCtx.API = mAPI
		}
		te = &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initServicePlan()
		err = parseAndRun(tc.args)
		assert.NotNil(err)
		assert.Nil(te.jsonData)
		assert.Regexp(tc.re, err.Error())
	}

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initServicePlan()
	err = parseAndRun([]string{"plan", "modify", "-n", *lParams.Name, "o", "json"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}
