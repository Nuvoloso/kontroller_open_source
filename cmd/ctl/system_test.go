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
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/protection_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/system"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestSystemMakeRecord(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	ac := &systemCmd{}
	o := &models.System{
		SystemAllOf0: models.SystemAllOf0{
			Meta: &models.ObjMeta{
				ID:           "system-1",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
			Service: &models.NuvoService{
				NuvoServiceAllOf0: models.NuvoServiceAllOf0{
					ServiceAttributes: map[string]models.ValueType{
						"once":   models.ValueType{Kind: "STRING", Value: "fe"},
						"upon":   models.ValueType{Kind: "STRING", Value: "fi"},
						"a time": models.ValueType{Kind: "STRING", Value: "fo"},
					},
					ServiceVersion: "system-version-1",
				},
				ServiceState: models.ServiceState{
					State: "RUNNING",
				},
			},
		},
		SystemMutable: models.SystemMutable{
			Name:        "nuvo",
			Description: "system object",
		},
	}
	rec := ac.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(systemHeaders))
	assert.Equal("system-1", rec[hID])
	assert.Equal("1", rec[hVersion])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeCreated])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeModified])
	assert.Equal("system-version-1", rec[hSystemVersion])
	assert.Equal("RUNNING", rec[hControllerState])
	assert.Equal("nuvo", rec[hName])
	assert.Equal("system object", rec[hDescription])
	// note sorted order of attribute name
	al := "a time[S]: fo\nonce[S]: fe\nupon[S]: fi"
	assert.Equal(al, rec[hSystemAttributes])
}

func TestSystemGet(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	params := system.NewSystemFetchParams()
	res := &system.SystemFetchOK{
		Payload: &models.System{
			SystemAllOf0: models.SystemAllOf0{
				Meta: &models.ObjMeta{
					ID:           "id1",
					Version:      1,
					TimeCreated:  strfmt.DateTime(now),
					TimeModified: strfmt.DateTime(now),
				},
				Service: &models.NuvoService{
					NuvoServiceAllOf0: models.NuvoServiceAllOf0{
						ServiceVersion: "system-version-1",
					},
				},
			},
			SystemMutable: models.SystemMutable{
				Name:        models.ObjName("nuvo"),
				Description: models.ObjDescription("nuvo system"),
			},
		},
	}

	// get, all defaults
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockSystemClient(mockCtrl)
	mAPI.EXPECT().System().Return(cOps)
	cOps.EXPECT().SystemFetch(params).Return(res, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSystem()
	err := parseAndRun([]string{"system", "get"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(systemDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, 1)

	// get default, json (alias)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockSystemClient(mockCtrl)
	mAPI.EXPECT().System().Return(cOps)
	cOps.EXPECT().SystemFetch(params).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSystem()
	err = parseAndRun([]string{"sys", "get", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(res.Payload, te.jsonData)

	// get default, yaml
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockSystemClient(mockCtrl)
	mAPI.EXPECT().System().Return(cOps)
	cOps.EXPECT().SystemFetch(params).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSystem()
	err = parseAndRun([]string{"system", "get", "-o", "yaml"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(res.Payload, te.yamlData)

	// get with columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockSystemClient(mockCtrl)
	mAPI.EXPECT().System().Return(cOps)
	cOps.EXPECT().SystemFetch(params).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSystem()
	err = parseAndRun([]string{"system", "get", "--columns", "Name,TimeCreated"})
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
	initSystem()
	err = parseAndRun([]string{"system", "get", "--columns", "Name,Foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// get, API failure model error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockSystemClient(mockCtrl)
	mAPI.EXPECT().System().Return(cOps)
	apiErr := &system.SystemFetchDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	cOps.EXPECT().SystemFetch(params).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSystem()
	err = parseAndRun([]string{"system", "get"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// get, API failure arbitrary error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockSystemClient(mockCtrl)
	mAPI.EXPECT().System().Return(cOps)
	otherErr := fmt.Errorf("OTHER ERROR")
	cOps.EXPECT().SystemFetch(params).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSystem()
	err = parseAndRun([]string{"system", "get"})
	assert.NotNil(err)
	assert.Equal(otherErr, err)

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
	initSystem()
	err = parseAndRun([]string{"system", "get", "-A", "System"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSystem()
	err = parseAndRun([]string{"system", "get", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestSystemModify(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	lParams := system.NewSystemFetchParams()
	lRes := &system.SystemFetchOK{
		Payload: &models.System{
			SystemAllOf0: models.SystemAllOf0{
				Meta: &models.ObjMeta{
					ID:           "id1",
					Version:      1,
					TimeCreated:  strfmt.DateTime(now),
					TimeModified: strfmt.DateTime(now),
				},
				Service: &models.NuvoService{
					NuvoServiceAllOf0: models.NuvoServiceAllOf0{
						ServiceVersion: "system-version-1",
					},
				},
			},
			SystemMutable: models.SystemMutable{
				Name:        models.ObjName("nuvo"),
				Description: models.ObjDescription("nuvo system"),
				ClusterUsagePolicy: &models.ClusterUsagePolicy{
					Inherited:          false,
					AccountSecretScope: "CLUSTER",
				},
				ManagementHostCName: "cname",
			},
		},
	}
	pdRes := &protection_domain.ProtectionDomainListOK{
		Payload: []*models.ProtectionDomain{
			&models.ProtectionDomain{
				ProtectionDomainAllOf0: models.ProtectionDomainAllOf0{
					Meta: &models.ObjMeta{
						ID: "newPdID",
					},
				},
				ProtectionDomainCreateOnce: models.ProtectionDomainCreateOnce{},
				ProtectionDomainMutable: models.ProtectionDomainMutable{
					Name: "newPdName",
				},
			},
		},
	}
	domRes := &csp_domain.CspDomainListOK{
		Payload: []*models.CSPDomain{
			&models.CSPDomain{
				CSPDomainAllOf0: models.CSPDomainAllOf0{
					Meta: &models.ObjMeta{
						ID: "newDomID",
					},
				},
				CSPDomainMutable: models.CSPDomainMutable{
					Name: "newDomName",
				},
			},
		},
	}

	// Name and description and managementHostname
	t.Log("Name, description, userPasswordPolicy and managementHostname")
	mockCtrl := gomock.NewController(t)
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockSystemClient(mockCtrl)
	fm := mockmgmtclient.NewSystemMatcher(t, lParams)
	cOps.EXPECT().SystemFetch(fm).Return(lRes, nil)
	uParams := system.NewSystemUpdateParams()
	uParams.Payload = &models.SystemMutable{
		Name:                "NewName",
		Description:         "new description",
		ManagementHostCName: "new cname",
		UserPasswordPolicy:  &models.SystemMutableUserPasswordPolicy{MinLength: 12},
	}
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"name", "description", "managementHostCName", "userPasswordPolicy.minLength"}
	um := mockmgmtclient.NewSystemMatcher(t, uParams)
	uRet := system.NewSystemUpdateOK()
	assert.NotNil(lRes.Payload)
	uRet.Payload = lRes.Payload
	cOps.EXPECT().SystemUpdate(um).Return(uRet, nil)
	mAPI.EXPECT().System().Return(cOps).MinTimes(1)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSystem()
	err := parseAndRun([]string{"system", "modify", "-N", "NewName", "-d", "new description",
		"-M", "new cname", "--user-password-min-length=12", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(uRet.Payload, te.jsonData)
	mockCtrl.Finish()

	// CUP modifications
	t.Log("ClusterUsagePolicy, set description and managementHostCName to empty")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockSystemClient(mockCtrl)
	fm = mockmgmtclient.NewSystemMatcher(t, lParams)
	cOps.EXPECT().SystemFetch(fm).Return(lRes, nil)
	uParams = system.NewSystemUpdateParams()
	uParams.Payload = &models.SystemMutable{
		ClusterUsagePolicy: &models.ClusterUsagePolicy{
			Inherited:                   false,
			AccountSecretScope:          "GLOBAL",
			ConsistencyGroupName:        "${cluster.name}",
			VolumeDataRetentionOnDelete: "RETAIN",
		},
	}
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"clusterUsagePolicy", "description", "managementHostCName"}
	um = mockmgmtclient.NewSystemMatcher(t, uParams)
	uRet = system.NewSystemUpdateOK()
	assert.NotNil(lRes.Payload)
	uRet.Payload = lRes.Payload
	cOps.EXPECT().SystemUpdate(um).Return(uRet, nil)
	mAPI.EXPECT().System().Return(cOps).MinTimes(1)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSystem()
	err = parseAndRun([]string{"system", "modify", "--cup-account-secret-scope", "GLOBAL",
		"--cup-consistency-group-name", "${cluster.name}",
		"--cup-data-retention-on-delete", "RETAIN", "-V", "1", "-M", " ", "-d", " ", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(uRet.Payload, te.jsonData)
	mockCtrl.Finish()

	// SMP modifications: all flags to false + set Retention, SCP policy changes with names
	t.Log("SnapshotManagementPolicy: all flags to false + set Retention, SCP policy changes with names")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	lRes.Payload.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{
		DisableSnapshotCreation:  true,
		DeleteLast:               true,
		DeleteVolumeWithLast:     true,
		NoDelete:                 true,
		RetentionDurationSeconds: swag.Int32(0),
	}
	lRes.Payload.SnapshotCatalogPolicy = &models.SnapshotCatalogPolicy{
		CspDomainID:        "domID",
		ProtectionDomainID: "pdID",
	}
	uParams = system.NewSystemUpdateParams()
	uParams.Payload = &models.SystemMutable{
		SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
			DisableSnapshotCreation:  false,
			DeleteLast:               false,
			DeleteVolumeWithLast:     false,
			NoDelete:                 false,
			RetentionDurationSeconds: swag.Int32(7 * 24 * 60 * 60),
		},
		SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
			CspDomainID:        "newDomID",
			ProtectionDomainID: "newPdID",
		},
	}
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"snapshotManagementPolicy", "snapshotCatalogPolicy"}
	um = mockmgmtclient.NewSystemMatcher(t, uParams)
	uRet = system.NewSystemUpdateOK()
	assert.NotNil(lRes.Payload)
	uRet.Payload = lRes.Payload
	cOps = mockmgmtclient.NewMockSystemClient(mockCtrl)
	fm = mockmgmtclient.NewSystemMatcher(t, lParams)
	cOps.EXPECT().SystemFetch(fm).Return(lRes, nil)
	cOps.EXPECT().SystemUpdate(um).Return(uRet, nil)
	mAPI.EXPECT().System().Return(cOps).MinTimes(1)
	pdm := mockmgmtclient.NewProtectionDomainMatcher(t, protection_domain.NewProtectionDomainListParams())
	pdOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
	mAPI.EXPECT().ProtectionDomain().Return(pdOps).MinTimes(1)
	pdOps.EXPECT().ProtectionDomainList(pdm).Return(pdRes, nil).MinTimes(1)
	dm := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	dOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(dm).Return(domRes, nil).MinTimes(1)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSystem()
	err = parseAndRun([]string{"system", "modify", "-V", "1", "-o", "json",
		"--scp-domain", "newDomName", "--scp-protection-domain", "newPdName",
		"--smp-enable-snapshots", "--smp-delete-snapshots", "--smp-no-delete-last-snapshot", "--smp-no-delete-vol-with-last", "--smp-retention-days", "7"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(uRet.Payload, te.jsonData)
	mockCtrl.Finish()

	// SMP modifications: all flags to true, VSR management policy: set retention period
	t.Log("SnapshotManagementPolicy: all flags to true, VSR management policy: set retention period")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	lRes.Payload.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{
		DisableSnapshotCreation: false,
		DeleteLast:              false,
		DeleteVolumeWithLast:    false,
		NoDelete:                false,
	}
	lRes.Payload.VsrManagementPolicy = &models.VsrManagementPolicy{
		RetentionDurationSeconds: swag.Int32(100),
	}
	cOps = mockmgmtclient.NewMockSystemClient(mockCtrl)
	fm = mockmgmtclient.NewSystemMatcher(t, lParams)
	cOps.EXPECT().SystemFetch(fm).Return(lRes, nil)
	uParams = system.NewSystemUpdateParams()
	uParams.Payload = &models.SystemMutable{
		SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
			DisableSnapshotCreation: true,
			DeleteLast:              true,
			DeleteVolumeWithLast:    true,
			NoDelete:                true,
		},
		VsrManagementPolicy: &models.VsrManagementPolicy{
			RetentionDurationSeconds: swag.Int32(7 * 24 * 60 * 60),
		},
	}
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"snapshotManagementPolicy", "vsrManagementPolicy"}
	um = mockmgmtclient.NewSystemMatcher(t, uParams)
	uRet = system.NewSystemUpdateOK()
	assert.NotNil(lRes.Payload)
	uRet.Payload = lRes.Payload
	cOps.EXPECT().SystemUpdate(um).Return(uRet, nil)
	mAPI.EXPECT().System().Return(cOps).MinTimes(1)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSystem()
	t.Logf("uParams: %v", uParams)
	err = parseAndRun([]string{"system", "modify", "-V", "1", "-o", "json",
		"--smp-disable-snapshots", "--smp-no-delete-snapshots", "--smp-delete-last-snapshot", "--smp-delete-vol-with-last", "--vsrp-retention-days", "7"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(uRet.Payload, te.jsonData)
	mockCtrl.Finish()

	// init context failure
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
	initSystem()
	err = parseAndRun([]string{"system", "modify", "-A", "System"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// other error cases
	updateErr := system.NewSystemUpdateDefault(400)
	updateErr.Payload = &models.Error{Code: 400, Message: swag.String("update error")}
	otherErr := fmt.Errorf("other error")
	mNotNil := gomock.Not(gomock.Nil)
	errTCs := []struct {
		name      string
		args      []string
		re        string
		noSys     bool
		fetchErr  error
		updateErr error
		noMock    bool
	}{
		{
			name: "No modifications",
			args: []string{"system", "modify", "-V", "1", "-o", "json"},
			re:   "No modifications",
		},
		{
			name:     "Fetch error",
			args:     []string{"system", "modify", "-N", "NewName", "-V", "1", "-o", "json"},
			re:       "other error",
			fetchErr: otherErr,
		},
		{
			name:      "Update default error",
			args:      []string{"system", "modify", "-N", "NewName", "-V", "1", "-o", "json"},
			re:        "update error",
			updateErr: updateErr,
		},
		{
			name:      "Update other error",
			args:      []string{"system", "modify", "-N", "NewName", "-V", "1", "-o", "json"},
			re:        "other error",
			updateErr: otherErr,
		},
		{
			name:   "invalid columns",
			args:   []string{"system", "modify", "--columns", "ID,foo"},
			re:     "invalid column",
			noMock: true,
		},
		{
			name: "enable-snapshots/disable-snapshots",
			args: []string{"system", "modify", "--smp-enable-snapshots", "--smp-disable-snapshots", "-o", "json"},
			re:   "do not specify 'smp-enable-snapshots' and 'smp-disable-snapshots' or 'smp-delete-snapshots' and 'smp-no-delete-snapshots' together",
		},
		{
			name:   "no-delete-snapshots/smp-retention-days",
			args:   []string{"system", "modify", "--smp-no-delete-snapshots", "--smp-retention-days", "7", "-o", "json"},
			re:     "do not specify 'smp-no-delete-snapshots' and 'smp-retention-days' together",
			noMock: true,
		},
	}
	for _, tc := range errTCs {
		t.Logf("case: %s", tc.name)
		if !tc.noMock {
			mockCtrl.Finish()
			mockCtrl = gomock.NewController(t)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			if !tc.noSys {
				cOps = mockmgmtclient.NewMockSystemClient(mockCtrl)
				mAPI.EXPECT().System().Return(cOps).MinTimes(1)
				fm = mockmgmtclient.NewSystemMatcher(t, lParams)
				if tc.fetchErr != nil {
					cOps.EXPECT().SystemFetch(fm).Return(nil, tc.fetchErr)
				} else {
					cOps.EXPECT().SystemFetch(fm).Return(lRes, nil)
					if tc.updateErr != nil {
						cOps.EXPECT().SystemUpdate(mNotNil).Return(nil, tc.updateErr)
					}
				}
			}
			appCtx.API = mAPI
		}
		te = &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initSystem()
		err = parseAndRun(tc.args)
		assert.NotNil(err)
		assert.Nil(te.jsonData)
		assert.Regexp(tc.re, err.Error())
		appCtx.Account, appCtx.AccountID = "", ""
	}

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSystem()
	err = parseAndRun([]string{"system", "modify", "-N", "NewName", "o", "json"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestSystemHostnameGet(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	params := system.NewSystemHostnameFetchParams()
	res := &system.SystemHostnameFetchOK{
		Payload: "hostname", // newline will be
	}

	outFile := "./hostname"
	defer func() { os.Remove(outFile) }()
	// get, all defaults
	t.Log("case: fetch hostname")
	mockCtrl := gomock.NewController(t)
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockSystemClient(mockCtrl)
	mAPI.EXPECT().System().Return(cOps)
	cOps.EXPECT().SystemHostnameFetch(params).Return(res, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSystem()
	err := parseAndRun([]string{"system", "get-hostname", "-O", "./hostname"})
	assert.Nil(err)
	resBytes, err := ioutil.ReadFile(outFile)
	assert.Nil(err)
	assert.Equal(res.Payload+"\n", string(resBytes)) // newline added when returned
	mockCtrl.Finish()
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("case: fail hostname fetch, use stdout")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockSystemClient(mockCtrl)
	hnfErr := system.NewSystemHostnameFetchDefault(400)
	hnfErr.Payload = &models.Error{Code: 400, Message: swag.String("hnfErr")}
	mAPI.EXPECT().System().Return(cOps)
	cOps.EXPECT().SystemHostnameFetch(params).Return(nil, hnfErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSystem()
	err = parseAndRun([]string{"system", "get-hostname"})
	assert.NotNil(err)
	assert.Regexp("hnfErr", err)
	mockCtrl.Finish()
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("case: invalid output file")
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSystem()
	err = parseAndRun([]string{"system", "get-hostname", "-O", "./bad/file"})
	assert.NotNil(err)
	assert.Regexp("no such file or directory", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

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
	initSystem()
	err = parseAndRun([]string{"system", "get-hostname", "-A", "System"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""
	mockCtrl.Finish()

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initSystem()
	err = parseAndRun([]string{"system", "get-hostname", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}
