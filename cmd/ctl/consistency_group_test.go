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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/application_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/consistency_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestConsistencyGroupMakeRecord(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	cmd := &consistencyGroupCmd{}
	cmd.accounts = map[string]ctxIDName{
		"accountID": {"", "System"},
	}
	cmd.applicationGroups = map[string]ctxIDName{
		"agID": ctxIDName{id: "accountID", name: "AG"},
	}
	o := &models.ConsistencyGroup{
		ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
			Meta: &models.ObjMeta{
				ID:           "id",
				Version:      2,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
		},
		ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
			AccountID: "accountID",
		},
		ConsistencyGroupMutable: models.ConsistencyGroupMutable{
			Description:         "desc",
			Name:                "cg1",
			ApplicationGroupIds: []models.ObjIDMutable{"agID"},
			Tags:                models.ObjTags{"tag1", "tag2", "tag3"},
			SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
				DisableSnapshotCreation:  true,
				RetentionDurationSeconds: swag.Int32(99),
				Inherited:                true,
			},
		},
	}
	rec := cmd.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(consistencyGroupHeaders))
	assert.Equal("id", rec[hID])
	assert.Equal("2", rec[hVersion])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeCreated])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeModified])
	assert.Equal("System", rec[hAccount])
	assert.Equal("cg1", rec[hName])
	assert.Equal("desc", rec[hDescription])
	assert.Equal("AG", rec[hApplicationGroups])
	assert.Equal("tag1, tag2, tag3", rec[hTags])
	smpDetails := "Inherited: true\nDeleteLast: false\nDeleteVolumeWithLast: false\nDisableSnapshotCreation: true\nNoDelete: false\nRetentionDurationSeconds: 99\nVolumeDataRetentionOnDelete: N/A"
	assert.Equal(smpDetails, rec[hSnapshotManagementPolicy])

	// cache routines are singletons
	assert.NoError(cmd.cacheApplicationGroups())

	// repeat without the map
	cmd.accounts = map[string]ctxIDName{}
	cmd.applicationGroups = map[string]ctxIDName{}
	rec = cmd.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(consistencyGroupHeaders))
	assert.Equal("accountID", rec[hAccount])
	assert.Equal("agID", rec[hApplicationGroups])
}

func TestConsistencyGroupList(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	res := &consistency_group.ConsistencyGroupListOK{
		Payload: []*models.ConsistencyGroup{
			&models.ConsistencyGroup{
				ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
					AccountID: "accountID",
				},
				ConsistencyGroupMutable: models.ConsistencyGroupMutable{
					ApplicationGroupIds: []models.ObjIDMutable{"a-g-1", "a-g-2"},
					Description:         "desc",
					Name:                "cg1",
					Tags:                models.ObjTags{"tag1", "tag2", "tag3"},
					SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
						DisableSnapshotCreation:  true,
						RetentionDurationSeconds: swag.Int32(99),
						Inherited:                true,
					},
				},
			},
			&models.ConsistencyGroup{
				ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id2",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
					AccountID: "authAccountID",
				},
				ConsistencyGroupMutable: models.ConsistencyGroupMutable{
					ApplicationGroupIds: []models.ObjIDMutable{"a-g-1"},
					Description:         "desc2",
					Name:                "cg2",
					Tags:                models.ObjTags{"tag1", "tag2", "tag3"},
					SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
						DisableSnapshotCreation:  true,
						RetentionDurationSeconds: swag.Int32(99),
						Inherited:                true,
					},
				},
			},
		},
	}

	t.Log("case: list, all defaults")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := cacheLoadHelper(t, mockCtrl, loadAccounts, loadAGs)
	params := consistency_group.NewConsistencyGroupListParams()
	m := mockmgmtclient.NewConsistencyGroupMatcher(t, params)
	cOps := mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cOps)
	cOps.EXPECT().ConsistencyGroupList(m).Return(res, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initConsistencyGroup()
	err := parseAndRun([]string{"cgs", "list"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(consistencyGroupDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: list, json, most options")
	mAPI = cacheLoadHelper(t, mockCtrl, loadContext)
	params = consistency_group.NewConsistencyGroupListParams()
	params.AccountID = swag.String("accountID")
	params.Name = swag.String("cg")
	params.Tags = []string{"tag2"}
	m = mockmgmtclient.NewConsistencyGroupMatcher(t, params)
	cOps = mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cOps)
	cOps.EXPECT().ConsistencyGroupList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initConsistencyGroup()
	err = parseAndRun([]string{"consistency-group", "list", "-A", "System", "--owned-only", "-n", "cg", "-t", "tag2", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(res.Payload, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: list, yaml, remaining options")
	mAPI = cacheLoadHelper(t, mockCtrl, loadContext, loadAGs)
	params = consistency_group.NewConsistencyGroupListParams()
	params.ApplicationGroupID = swag.String("a-g-1")
	m = mockmgmtclient.NewConsistencyGroupMatcher(t, params)
	cOps = mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cOps)
	cOps.EXPECT().ConsistencyGroupList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initConsistencyGroup()
	err = parseAndRun([]string{"cgs", "list", "-A", "System", "--application-group=AG", "-o", "yaml"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(res.Payload, te.yamlData)
	appCtx.Account, appCtx.AccountID = "", ""

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: list with columns, account and AG list error ignored")
	agListErr := fmt.Errorf("agListErr")
	agm := mockmgmtclient.NewApplicationGroupMatcher(t, application_group.NewApplicationGroupListParams())
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps)
	apm := mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	aOps.EXPECT().AccountList(apm).Return(nil, fmt.Errorf("db error"))
	agOps := mockmgmtclient.NewMockApplicationGroupClient(mockCtrl)
	mAPI.EXPECT().ApplicationGroup().Return(agOps)
	agOps.EXPECT().ApplicationGroupList(agm).Return(nil, agListErr)
	params = consistency_group.NewConsistencyGroupListParams()
	m = mockmgmtclient.NewConsistencyGroupMatcher(t, params)
	cOps = mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cOps)
	cOps.EXPECT().ConsistencyGroupList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initConsistencyGroup()
	err = parseAndRun([]string{"consistency-group", "list", "--columns", "ID,Version,Name"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, 3)
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: list with invalid columns")
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initConsistencyGroup()
	err = parseAndRun([]string{"cgs", "list", "-c", "ID,Foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: list, API failure model error")
	apiErr := &consistency_group.ConsistencyGroupListDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	params = consistency_group.NewConsistencyGroupListParams()
	m = mockmgmtclient.NewConsistencyGroupMatcher(t, params)
	cOps = mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cOps)
	cOps.EXPECT().ConsistencyGroupList(m).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initConsistencyGroup()
	err = parseAndRun([]string{"consistency-group", "list"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: list, API failure arbitrary error")
	otherErr := fmt.Errorf("OTHER ERROR")
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	params = consistency_group.NewConsistencyGroupListParams()
	m = mockmgmtclient.NewConsistencyGroupMatcher(t, params)
	cOps = mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cOps)
	cOps.EXPECT().ConsistencyGroupList(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initConsistencyGroup()
	err = parseAndRun([]string{"consistency-group", "list"})
	assert.NotNil(err)
	assert.Equal(otherErr.Error(), err.Error())

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: account not found")
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "foo").Return("", fmt.Errorf("account not found"))
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initConsistencyGroup()
	err = parseAndRun([]string{"cg", "list", "-A", "foo"})
	assert.NotNil(err)
	assert.Regexp("account.*not found", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	mockCtrl.Finish()
	t.Log("application group list error")
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadContext)
	agOps = mockmgmtclient.NewMockApplicationGroupClient(mockCtrl)
	mAPI.EXPECT().ApplicationGroup().Return(agOps)
	agOps.EXPECT().ApplicationGroupList(agm).Return(nil, agListErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initConsistencyGroup()
	err = parseAndRun([]string{"cg", "list", "-A", "System", "--application-group=AG"})
	assert.Equal(agListErr.Error(), err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("application group not found")
	mAPI = cacheLoadHelper(t, mockCtrl, loadContext, loadAGs)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initConsistencyGroup()
	err = parseAndRun([]string{"cg", "list", "-A", "System", "--application-group", "foo"})
	assert.Regexp("application group.*not found", err)
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initConsistencyGroup()
	err = parseAndRun([]string{"cgs", "list", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestConsistencyGroupModify(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil
	now := time.Now()
	res := &consistency_group.ConsistencyGroupListOK{
		Payload: []*models.ConsistencyGroup{
			&models.ConsistencyGroup{
				ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
					AccountID: "accountID",
				},
				ConsistencyGroupMutable: models.ConsistencyGroupMutable{
					Description:         "desc",
					Name:                "cg1",
					ApplicationGroupIds: []models.ObjIDMutable{"a-g-1"},
					Tags:                models.ObjTags{"tag1", "tag2", "tag3"},
					SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
						DisableSnapshotCreation:  true,
						RetentionDurationSeconds: swag.Int32(99),
						Inherited:                true,
					},
				},
			},
		},
	}

	t.Log("case: APPEND lists")
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := cacheLoadHelper(t, mockCtrl, loadContext, loadAGs)
	args := []string{"cg", "modify", "-A", "System", "-n", "cg1", "--application-group=AG", "-t", "tag1", "-o", "json"}
	params := consistency_group.NewConsistencyGroupListParams()
	params.Name = swag.String(string(res.Payload[0].Name))
	params.AccountID = swag.String("accountID")
	cOps := mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cOps).MinTimes(2)
	cOps.EXPECT().ConsistencyGroupList(params).Return(res, nil)
	uParams := consistency_group.NewConsistencyGroupUpdateParams()
	uParams.ID = "id1"
	uParams.Payload = &models.ConsistencyGroupMutable{
		ApplicationGroupIds: []models.ObjIDMutable{"a-g-1"},
		Tags:                models.ObjTags{"tag1"},
	}
	uParams.Append = []string{"applicationGroupIds", "tags"}
	uParams.Version = swag.Int32(1)
	um := mockmgmtclient.NewConsistencyGroupMatcher(t, uParams)
	uRet := consistency_group.NewConsistencyGroupUpdateOK()
	uRet.Payload = res.Payload[0]
	cOps.EXPECT().ConsistencyGroupUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initConsistencyGroup()
	err := parseAndRun(args)
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.ConsistencyGroup{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: Disable Account snapshot policy, set all flags to false + set Retention in Consistency Group with SMP set, SET lists")
	res.Payload[0].SnapshotManagementPolicy = &models.SnapshotManagementPolicy{
		DisableSnapshotCreation:  true,
		DeleteLast:               true,
		DeleteVolumeWithLast:     true,
		NoDelete:                 true,
		RetentionDurationSeconds: swag.Int32(0),
		Inherited:                true,
	}
	mAPI = cacheLoadHelper(t, mockCtrl, loadContext, loadAGs)
	args = []string{"cg", "modify", "-A", "System", "-n", "cg1",
		"--smp-enable-snapshots", "--smp-delete-snapshots", "--smp-no-delete-last-snapshot", "--smp-no-delete-vol-with-last", "--smp-retention-days", "7",
		"--application-group=AG", "--application-group-action=SET",
		"-o", "json", "-V", "3"}
	cOps = mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cOps).MinTimes(2)
	cOps.EXPECT().ConsistencyGroupList(params).Return(res, nil)
	uParams = consistency_group.NewConsistencyGroupUpdateParams().WithID("id1")
	uParams.Payload = &models.ConsistencyGroupMutable{
		ApplicationGroupIds: []models.ObjIDMutable{"a-g-1"},
		SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
			DisableSnapshotCreation:  false,
			DeleteLast:               false,
			DeleteVolumeWithLast:     false,
			NoDelete:                 false,
			RetentionDurationSeconds: swag.Int32(7 * 24 * 60 * 60),
			Inherited:                false,
		},
	}
	uParams.Set = []string{"applicationGroupIds", "snapshotManagementPolicy"}
	uParams.Version = swag.Int32(3)
	um = mockmgmtclient.NewConsistencyGroupMatcher(t, uParams)
	uRet = consistency_group.NewConsistencyGroupUpdateOK()
	uRet.Payload = res.Payload[0]
	cOps.EXPECT().ConsistencyGroupUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initConsistencyGroup()
	err = parseAndRun(args)
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.ConsistencyGroup{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: Snapshot policy all flags to false + set Retention in Consistency Group with SMP set, SET lists")
	res.Payload[0].SnapshotManagementPolicy = &models.SnapshotManagementPolicy{ // Inherited = false
		DisableSnapshotCreation:     true,
		DeleteLast:                  true,
		DeleteVolumeWithLast:        true,
		NoDelete:                    true,
		RetentionDurationSeconds:    swag.Int32(0),
		VolumeDataRetentionOnDelete: "DELETE",
	}
	mAPI = cacheLoadHelper(t, mockCtrl, loadContext, loadAGs)
	args = []string{"cg", "modify", "-A", "System", "-n", "cg1",
		"--smp-enable-snapshots", "--smp-delete-snapshots", "--smp-no-delete-last-snapshot", "--smp-no-delete-vol-with-last", "--smp-retention-days", "7", "--smp-data-retention-on-delete", "RETAIN",
		"--application-group=AG", "--application-group-action=SET",
		"-t", "tag1", "-t", "tag2", "--tag-action=SET", "-o", "json", "-V", "3"}
	cOps = mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cOps).MinTimes(2)
	cOps.EXPECT().ConsistencyGroupList(params).Return(res, nil)
	uParams = consistency_group.NewConsistencyGroupUpdateParams().WithID("id1")
	uParams.Payload = &models.ConsistencyGroupMutable{
		ApplicationGroupIds: []models.ObjIDMutable{"a-g-1"},
		SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
			DisableSnapshotCreation:     false,
			DeleteLast:                  false,
			DeleteVolumeWithLast:        false,
			NoDelete:                    false,
			RetentionDurationSeconds:    swag.Int32(7 * 24 * 60 * 60),
			VolumeDataRetentionOnDelete: "RETAIN",
		},
		Tags: []string{"tag1", "tag2"},
	}
	uParams.Set = []string{"applicationGroupIds", "snapshotManagementPolicy", "tags"}
	uParams.Version = swag.Int32(3)
	um = mockmgmtclient.NewConsistencyGroupMatcher(t, uParams)
	uRet = consistency_group.NewConsistencyGroupUpdateOK()
	uRet.Payload = res.Payload[0]
	cOps.EXPECT().ConsistencyGroupUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initConsistencyGroup()
	err = parseAndRun(args)
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.ConsistencyGroup{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: Snapshot policy all flags to true")
	res.Payload[0].SnapshotManagementPolicy = &models.SnapshotManagementPolicy{ // Inherited = false
		DisableSnapshotCreation: false,
		DeleteLast:              false,
		DeleteVolumeWithLast:    false,
		NoDelete:                false,
	}
	mAPI = cacheLoadHelper(t, mockCtrl, loadContext)
	args = []string{"cg", "modify", "-A", "System", "-n", "cg1",
		"--smp-disable-snapshots", "--smp-no-delete-snapshots", "--smp-delete-last-snapshot", "--smp-delete-vol-with-last",
		"-o", "json", "-V", "3"}
	cOps = mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cOps).MinTimes(1)
	cOps.EXPECT().ConsistencyGroupList(params).Return(res, nil)
	uParams = consistency_group.NewConsistencyGroupUpdateParams().WithID("id1")
	uParams.Payload = &models.ConsistencyGroupMutable{
		SnapshotManagementPolicy: &models.SnapshotManagementPolicy{},
	}
	*uParams.Payload.SnapshotManagementPolicy = *res.Payload[0].SnapshotManagementPolicy
	uParams.Payload.SnapshotManagementPolicy.DisableSnapshotCreation = true
	uParams.Payload.SnapshotManagementPolicy.DeleteLast = true
	uParams.Payload.SnapshotManagementPolicy.DeleteVolumeWithLast = true
	uParams.Payload.SnapshotManagementPolicy.NoDelete = true
	uParams.Set = []string{"snapshotManagementPolicy"}
	uParams.Version = swag.Int32(3)
	um = mockmgmtclient.NewConsistencyGroupMatcher(t, uParams)
	uRet = consistency_group.NewConsistencyGroupUpdateOK()
	uRet.Payload = res.Payload[0]
	cOps.EXPECT().ConsistencyGroupUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initConsistencyGroup()
	err = parseAndRun(args)
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.ConsistencyGroup{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: Use parent Account Snapshot policy")
	res.Payload[0].SnapshotManagementPolicy = &models.SnapshotManagementPolicy{
		DisableSnapshotCreation:  true,
		RetentionDurationSeconds: swag.Int32(99),
		Inherited:                false,
	}
	mAPI = cacheLoadHelper(t, mockCtrl, loadContext)
	args = []string{"cg", "modify", "-A", "System", "-n", "cg1", "--smp-inherit", "-o", "json", "-V", "3"}
	cOps = mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cOps).MinTimes(2)
	cOps.EXPECT().ConsistencyGroupList(params).Return(res, nil)
	uParams = consistency_group.NewConsistencyGroupUpdateParams().WithID("id1")
	uParams.Payload = &models.ConsistencyGroupMutable{
		SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
			Inherited: true,
		},
	}
	uParams.Set = []string{"snapshotManagementPolicy"}
	uParams.Version = swag.Int32(3)
	um = mockmgmtclient.NewConsistencyGroupMatcher(t, uParams)
	uRet = consistency_group.NewConsistencyGroupUpdateOK()
	uRet.Payload = res.Payload[0]
	cOps.EXPECT().ConsistencyGroupUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initConsistencyGroup()
	err = parseAndRun(args)
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.ConsistencyGroup{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: REMOVE lists")
	mAPI = cacheLoadHelper(t, mockCtrl, loadContext, loadAGs)
	cOps = mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cOps).MinTimes(1)
	cOps.EXPECT().ConsistencyGroupList(params).Return(res, nil)
	uParams = consistency_group.NewConsistencyGroupUpdateParams()
	uParams.Payload = &models.ConsistencyGroupMutable{
		ApplicationGroupIds: []models.ObjIDMutable{"a-g-1"},
		Tags:                []string{"tag1", "tag2"},
	}
	uParams.ID = string(res.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Remove = []string{"applicationGroupIds", "tags"}
	um = mockmgmtclient.NewConsistencyGroupMatcher(t, uParams)
	uRet = consistency_group.NewConsistencyGroupUpdateOK()
	uRet.Payload = res.Payload[0]
	cOps.EXPECT().ConsistencyGroupUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initConsistencyGroup()
	err = parseAndRun([]string{"cg", "modify", "-A", "System", "-n", *params.Name,
		"--application-group=AG", "--application-group-action=REMOVE",
		"-t", "tag1", "-t", "tag2", "--tag-action", "REMOVE", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.ConsistencyGroup{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	t.Log("case: update with empty SET")
	mAPI = cacheLoadHelper(t, mockCtrl, loadContext)
	cOps = mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
	mAPI.EXPECT().ConsistencyGroup().Return(cOps).MinTimes(1)
	cOps.EXPECT().ConsistencyGroupList(params).Return(res, nil)
	uParams = consistency_group.NewConsistencyGroupUpdateParams().WithID("id1")
	uParams.Payload = &models.ConsistencyGroupMutable{
		ApplicationGroupIds: []models.ObjIDMutable{},
	}
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"applicationGroupIds", "tags"}
	um = mockmgmtclient.NewConsistencyGroupMatcher(t, uParams)
	uRet = consistency_group.NewConsistencyGroupUpdateOK()
	uRet.Payload = res.Payload[0]
	cOps.EXPECT().ConsistencyGroupUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initConsistencyGroup()
	err = parseAndRun([]string{"cg", "modify", "-A", "System", "-n", *params.Name,
		"--application-group-action=SET", "--tag-action", "SET", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.ConsistencyGroup{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

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
	initConsistencyGroup()
	err = parseAndRun([]string{"cg", "modify", "-A", "System", "-n", "cg1", "--smp-disable-snapshots", "-o", "json"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	params.AccountID = swag.String("")
	updateErr := consistency_group.NewConsistencyGroupUpdateDefault(400)
	updateErr.Payload = &models.Error{Code: 400, Message: swag.String("update error")}
	otherErr := fmt.Errorf("other error")
	mNotNil := gomock.Not(gomock.Nil)
	errTCs := []struct {
		name          string
		args          []string
		re            string
		noMock        bool
		updateErr     error
		cglRC         *consistency_group.ConsistencyGroupListOK
		cglErr        error
		useAccountSMP bool
	}{
		{
			name: "No modifications",
			args: []string{"cg", "modify", "-n", string(res.Payload[0].Name), "-o", "json"},
			re:   "No modifications",
		},
		{
			name:   "invalid columns",
			args:   []string{"cg", "modify", "-n", string(res.Payload[0].Name), "--columns", "ID,foo"},
			re:     "invalid column",
			noMock: true,
		},
		{
			name:  "cg not found",
			args:  []string{"cg", "modify", "-n", string(res.Payload[0].Name), "--smp-disable-snapshots", "-o", "json"},
			re:    "consistency.*not found",
			cglRC: &consistency_group.ConsistencyGroupListOK{Payload: []*models.ConsistencyGroup{}},
		},
		{
			name:   "cg list error",
			args:   []string{"cg", "modify", "-n", string(res.Payload[0].Name), "--smp-disable-snapshots", "-o", "json"},
			re:     "other error",
			cglErr: otherErr,
		},
		{
			name:      "Update default error",
			args:      []string{"cg", "modify", "-n", string(res.Payload[0].Name), "--smp-inherit", "-o", "json"},
			re:        "update error",
			updateErr: updateErr,
		},
		{
			name:      "Update other error",
			args:      []string{"cg", "modify", "-n", string(res.Payload[0].Name), "--smp-inherit", "-o", "json"},
			re:        "other error",
			updateErr: otherErr,
		},
		{
			name:   "enable-snapshots/disable-snapshots",
			args:   []string{"cg", "modify", "-n", string(res.Payload[0].Name), "--smp-enable-snapshots", "--smp-disable-snapshots", "-o", "json"},
			re:     "do not specify 'smp-enable-snapshots' and 'smp-disable-snapshots' or 'smp-delete-snapshots' and 'smp-no-delete-snapshots' together",
			noMock: true,
		},
		{
			name:   "no-delete-snapshots/smp-retention-days",
			args:   []string{"cg", "modify", "-n", string(res.Payload[0].Name), "--smp-no-delete-snapshots", "--smp-retention-days", "7", "-o", "json"},
			re:     "do not specify 'smp-no-delete-snapshots' and 'smp-retention-days' together",
			noMock: true,
		},
		{
			name:   "use account snapshot policy together with its modifications",
			args:   []string{"cg", "modify", "-n", string(res.Payload[0].Name), "--smp-inherit", "--smp-disable-snapshots", "-o", "json"},
			re:     "do not specify.*together with modifications",
			noMock: true,
		},
		{
			name:   "AG validate error",
			args:   []string{"cg", "modify", "-n", string(res.Payload[0].Name), "--application-group=foo", "-o", "json"},
			re:     "application-group requires account",
			noMock: true,
		},
	}
	for _, tc := range errTCs {
		inherited := false
		if tc.useAccountSMP {
			inherited = true
		}
		res.Payload[0].SnapshotManagementPolicy = &models.SnapshotManagementPolicy{
			DisableSnapshotCreation: false,
			NoDelete:                false,
			Inherited:               inherited,
		}
		t.Logf("case: %s", tc.name)
		if !tc.noMock {
			mockCtrl.Finish()
			mockCtrl = gomock.NewController(t)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			cOps = mockmgmtclient.NewMockConsistencyGroupClient(mockCtrl)
			if tc.cglErr != nil {
				cOps.EXPECT().ConsistencyGroupList(params).Return(nil, tc.cglErr)
			} else {
				if tc.cglRC == nil {
					tc.cglRC = res
				}
				cOps.EXPECT().ConsistencyGroupList(params).Return(tc.cglRC, nil)
			}
			if tc.updateErr != nil {
				cOps.EXPECT().ConsistencyGroupUpdate(mNotNil).Return(nil, tc.updateErr)
			}
			mAPI.EXPECT().ConsistencyGroup().Return(cOps).MinTimes(1)
			appCtx.API = mAPI
		}
		te = &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initConsistencyGroup()
		err = parseAndRun(tc.args)
		assert.NotNil(err)
		assert.Nil(te.jsonData)
		assert.Regexp(tc.re, err.Error())
	}

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initConsistencyGroup()
	args = []string{"cg", "modify", "-A", "System", "-n", "cg1", "--application-group=AG", "t", "tag1"}
	err = parseAndRun(args)
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}
