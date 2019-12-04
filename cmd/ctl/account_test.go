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
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/protection_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/role"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/user"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/Nuvoloso/kontroller/pkg/common"
	mockmgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient/mock"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestParseUserRoles(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()

	c := &accountCmd{}
	ret, err := c.parseUserRoles(nil)
	assert.NoError(err)
	assert.Empty(ret)

	c.roles = map[string]string{}
	c.users = map[string]string{}
	flags := map[string]string{"user1": "role1"}
	ret, err = c.parseUserRoles(flags)
	assert.Empty(ret)
	if assert.Error(err) {
		assert.Equal("User \"user1\" not found", err.Error())
	}

	c.users["uid1"] = "user1"
	ret, err = c.parseUserRoles(flags)
	assert.Empty(ret)
	if assert.Error(err) {
		assert.Equal("Role \"role1\" not found", err.Error())
	}

	c.roles["rid1"] = "role1"
	ret, err = c.parseUserRoles(flags)
	assert.NoError(err)
	exp := map[string]models.AuthRole{
		"uid1": {Disabled: false, RoleID: "rid1"},
	}
	assert.Equal(exp, ret)

	flags["user1"] = "role1:disabled"
	ret, err = c.parseUserRoles(flags)
	assert.NoError(err)
	exp["uid1"] = models.AuthRole{Disabled: true, RoleID: "rid1"}
	assert.Equal(exp, ret)

	flags["user1"] = "role1:notdisabled"
	ret, err = c.parseUserRoles(flags)
	assert.NoError(err)
	exp["uid1"] = models.AuthRole{Disabled: false, RoleID: "rid1"}
	assert.Equal(exp, ret)
}

func TestAccountMakeRecord(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	ac := &accountCmd{}
	ac.accounts = map[string]ctxIDName{
		"id":  {"id2", "account2"},
		"id2": {"", "nuvoloso"},
	}
	ac.roles = map[string]string{
		"rid1": centrald.SystemAdminRole,
		"rid2": "SomeRole",
	}
	ac.users = map[string]string{
		"uid1": centrald.SystemUser,
		"uid2": "otherUser",
	}
	o := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{
				ID:           "id",
				Version:      1,
				TimeCreated:  strfmt.DateTime(now),
				TimeModified: strfmt.DateTime(now),
			},
			AccountRoles: []models.ObjIDMutable{
				"rid1", "rid2",
			},
			TenantAccountID: "id2",
		},
		AccountMutable: models.AccountMutable{
			Name:        "account2",
			Description: "account2 object",
			Disabled:    true,
			Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
			UserRoles: map[string]models.AuthRole{
				"uid1": models.AuthRole{RoleID: "rid1"},
				"uid2": models.AuthRole{RoleID: "rid2", Disabled: true},
			},
			SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
				DisableSnapshotCreation:  true,
				RetentionDurationSeconds: swag.Int32(99),
				Inherited:                true,
			},
			SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
				CspDomainID:        "domID",
				ProtectionDomainID: "pdID",
				Inherited:          false,
			},
			VsrManagementPolicy: &models.VsrManagementPolicy{
				RetentionDurationSeconds: swag.Int32(100),
				Inherited:                false,
			},
		},
	}
	rec := ac.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(accountHeaders))
	assert.Equal("id", rec[hID])
	assert.Equal("1", rec[hVersion])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeCreated])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeModified])
	assert.Equal("account2", rec[hName])
	assert.Equal("account2 object", rec[hDescription])
	assert.Equal("nuvoloso", rec[hTenant])
	assert.Equal("System Admin\nSomeRole", rec[hAccountRoles])
	assert.Equal("true", rec[hDisabled])
	assert.Equal("tag1, tag2, tag3", rec[hTags])
	// note sorted order of user name
	ur := centrald.SystemUser + ":" + centrald.SystemAdminRole + "\notherUser:SomeRole(D)"
	assert.Equal(ur, rec[hUserRoles])
	smpDetails := "Inherited: true\nDeleteLast: false\nDeleteVolumeWithLast: false\nDisableSnapshotCreation: true\nNoDelete: false\nRetentionDurationSeconds: 99\nVolumeDataRetentionOnDelete: N/A"
	assert.Equal(smpDetails, rec[hSnapshotManagementPolicy])
	scpDetails := "Inherited: false\nCspDomainID: domID\nProtectionDomainID: pdID"
	assert.Equal(scpDetails, rec[hSnapshotCatalogPolicy])
	vsrpDetails := "Inherited: false\nNoDelete: false\nRetentionDurationSeconds: 100"
	assert.Equal(vsrpDetails, rec[hVSRManagementPolicy])

	// cache routines are singletons
	err := ac.cacheRoles()
	assert.Nil(err)
	err = ac.cacheUsers()
	assert.Nil(err)

	// no tenant or users
	o2 := &models.Account{}
	testutils.Clone(o, o2)
	o2.TenantAccountID = ""
	o2.UserRoles = map[string]models.AuthRole{}
	rec = ac.makeRecord(o2)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(accountHeaders))
	assert.Equal("id", rec[hID])
	assert.Equal("1", rec[hVersion])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeCreated])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeModified])
	assert.Equal("account2", rec[hName])
	assert.Equal("account2 object", rec[hDescription])
	assert.Empty(rec[hTenant])
	assert.Equal("System Admin\nSomeRole", rec[hAccountRoles])
	assert.Equal("true", rec[hDisabled])
	assert.Equal("tag1, tag2, tag3", rec[hTags])
	assert.Empty(rec[hUserRoles])

	// no caches
	ac.accounts = map[string]ctxIDName{}
	ac.roles = map[string]string{}
	ac.users = map[string]string{}
	rec = ac.makeRecord(o)
	t.Log(rec)
	assert.NotNil(rec)
	assert.Len(rec, len(accountHeaders))
	assert.Equal("id", rec[hID])
	assert.Equal("1", rec[hVersion])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeCreated])
	assert.Equal(now.Format(time.RFC3339), rec[hTimeModified])
	assert.Equal("account2", rec[hName])
	assert.Equal("account2 object", rec[hDescription])
	assert.Equal("id2", rec[hTenant])
	assert.Equal("rid1\nrid2", rec[hAccountRoles])
	assert.Equal("true", rec[hDisabled])
	assert.Equal("tag1, tag2, tag3", rec[hTags])
	ur = "uid1:rid1" + "\nuid2:rid2(D)"
	assert.Equal(ur, rec[hUserRoles])
}

func TestAccountList(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	m := mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	q := mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	res := &account.AccountListOK{
		Payload: []*models.Account{
			&models.Account{
				AccountAllOf0: models.AccountAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				AccountMutable: models.AccountMutable{
					Name:        "account1",
					Description: "account1 object",
					Disabled:    true,
					Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
					UserRoles: map[string]models.AuthRole{
						"uid1": models.AuthRole{RoleID: "rid1"},
						"uid2": models.AuthRole{RoleID: "rid2"},
					},
					SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
						DisableSnapshotCreation:  true,
						RetentionDurationSeconds: swag.Int32(99),
						Inherited:                true,
					},
					SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
						CspDomainID:        "domID",
						ProtectionDomainID: "pdID",
						Inherited:          false,
					},
					VsrManagementPolicy: &models.VsrManagementPolicy{
						RetentionDurationSeconds: swag.Int32(100),
						Inherited:                false,
					},
				},
			},
			&models.Account{
				AccountAllOf0: models.AccountAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id2",
						Version:      2,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				AccountMutable: models.AccountMutable{
					Name:        "account2",
					Description: "account2 object",
					Disabled:    true,
					Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
					UserRoles: map[string]models.AuthRole{
						"uid1": models.AuthRole{RoleID: "rid1"},
						"uid2": models.AuthRole{RoleID: "rid2"},
					},
					SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
						DisableSnapshotCreation:  true,
						RetentionDurationSeconds: swag.Int32(99),
						Inherited:                true,
					},
					SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
						CspDomainID:        "domID",
						ProtectionDomainID: "pdID",
						Inherited:          false,
					},
					VsrManagementPolicy: &models.VsrManagementPolicy{
						RetentionDurationSeconds: swag.Int32(100),
						Inherited:                false,
					},
				},
			},
		},
	}
	r := mockmgmtclient.NewRoleMatcher(t, role.NewRoleListParams())
	rRes := &role.RoleListOK{
		Payload: []*models.Role{
			&models.Role{
				RoleAllOf0: models.RoleAllOf0{
					Meta: &models.ObjMeta{
						ID: "rid1",
					},
				},
				RoleMutable: models.RoleMutable{
					Name: centrald.SystemAdminRole,
				},
			},
		},
	}
	u := mockmgmtclient.NewUserMatcher(t, user.NewUserListParams())
	uRes := &user.UserListOK{
		Payload: []*models.User{
			&models.User{
				UserAllOf0: models.UserAllOf0{
					Meta: &models.ObjMeta{
						ID: "uid2",
					},
				},
				UserMutable: models.UserMutable{
					AuthIdentifier: "other",
				},
			},
		},
	}

	// list, all defaults
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps).MinTimes(1)
	m1 := cOps.EXPECT().AccountList(m).Return(res, nil)
	cOps.EXPECT().AccountList(q).Return(res, nil).After(m1)
	rOps := mockmgmtclient.NewMockRoleClient(mockCtrl)
	mAPI.EXPECT().Role().Return(rOps)
	rOps.EXPECT().RoleList(r).Return(rRes, nil)
	uOps := mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(uOps)
	uOps.EXPECT().UserList(u).Return(uRes, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err := parseAndRun([]string{"account", "list"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, len(accountDefaultHeaders))
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list default, json
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps)
	cOps.EXPECT().AccountList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "list", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues(res.Payload, te.jsonData)

	// list with filters, yaml
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps).MinTimes(1)
	m2 := mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	m2.ListParam.Name = swag.String("System")
	m2.ListParam.Tags = []string{"tag1", "tag2"}
	cOps.EXPECT().AccountList(m2).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "list", "-o", "yaml", "-n", "System", "-t", "tag1", "-t", "tag2"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(res.Payload, te.yamlData)

	// list tenant context, yaml
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "account1").Return("id1", nil)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps)
	cOps.EXPECT().AccountList(m).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "list", "-o", "yaml", "-A", "account1"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(res.Payload, te.yamlData)
	appCtx.Account, appCtx.AccountID = "", ""

	// list tenant with subordinate filter, yaml
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "account1").Return("id1", nil)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps)
	m2 = mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	m2.ListParam.TenantAccountID = swag.String("id1")
	m2.ListParam.Name = swag.String("account2")
	cOps.EXPECT().AccountList(m2).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "list", "-o", "yaml", "-A", "account1", "-n", "account2"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(res.Payload, te.yamlData)
	appCtx.Account, appCtx.AccountID = "", ""

	// list with columns, Role & Account List errors ignored
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps).MinTimes(1)
	m1 = cOps.EXPECT().AccountList(m).Return(res, nil)
	cOps.EXPECT().AccountList(q).Return(nil, centrald.ErrorDbError).After(m1)
	rOps = mockmgmtclient.NewMockRoleClient(mockCtrl)
	mAPI.EXPECT().Role().Return(rOps)
	rOps.EXPECT().RoleList(r).Return(nil, centrald.ErrorDbError)
	uOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(uOps)
	uOps.EXPECT().UserList(u).Return(nil, centrald.ErrorDbError)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "list", "--columns", "Name,ID"})
	assert.Nil(err)
	t.Log(te.tableHeaders)
	assert.Len(te.tableHeaders, 2)
	t.Log(te.tableData)
	assert.Len(te.tableData, len(res.Payload))

	// list with csp domain
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cdOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdM := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	cdOps.EXPECT().CspDomainList(cdM).Return(resCspDomains, nil)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps)
	m3 := mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	m3.ListParam.CspDomainID = swag.String("CSP-DOMAIN-1")
	cOps.EXPECT().AccountList(m3).Return(res, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "list", "-o", "yaml", "-D", "domainName"})
	assert.Nil(err)
	assert.NotNil(te.yamlData)
	assert.EqualValues(res.Payload, te.yamlData)
	appCtx.Account, appCtx.AccountID = "", ""

	// list with invalid csp domain
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cdOps = mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(cdOps)
	cdM = mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	cdOps.EXPECT().CspDomainList(cdM).Return(resCspDomains, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "list", "-o", "yaml", "-D", "domainNameFOO"})
	assert.NotNil(err)
	appCtx.Account, appCtx.AccountID = "", ""

	// list with invalid columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "list", "-c", "Name,ID,Foo"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// list, API failure model error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps)
	apiErr := &account.AccountListDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	cOps.EXPECT().AccountList(m).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "list"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// list, API failure arbitrary error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps)
	otherErr := fmt.Errorf("OTHER ERROR")
	cOps.EXPECT().AccountList(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "list"})
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
	initAccount()
	err = parseAndRun([]string{"account", "list", "-A", "System"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "list", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestAccountCreate(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	params := account.NewAccountCreateParams()
	params.Payload = &models.Account{
		AccountMutable: models.AccountMutable{
			Name:        models.ObjName("name"),
			Description: models.ObjDescription("description"),
			Tags:        []string{"tag1", "tag2"},
			UserRoles: map[string]models.AuthRole{
				"uid1": models.AuthRole{RoleID: "rid1"},
				"uid2": models.AuthRole{RoleID: "rid1"},
			},
		},
	}
	res := &account.AccountCreateCreated{
		Payload: params.Payload,
	}
	m := mockmgmtclient.NewAccountMatcher(t, params)
	r := mockmgmtclient.NewRoleMatcher(t, role.NewRoleListParams())
	rRes := &role.RoleListOK{
		Payload: []*models.Role{
			&models.Role{
				RoleAllOf0: models.RoleAllOf0{
					Meta: &models.ObjMeta{
						ID: "rid1",
					},
				},
				RoleMutable: models.RoleMutable{
					Name: centrald.SystemAdminRole,
				},
			},
		},
	}
	u := mockmgmtclient.NewUserMatcher(t, user.NewUserListParams())
	uRes := &user.UserListOK{
		Payload: []*models.User{
			&models.User{
				UserAllOf0: models.UserAllOf0{
					Meta: &models.ObjMeta{
						ID: "uid1",
					},
				},
				UserMutable: models.UserMutable{
					AuthIdentifier: "user1",
				},
			},
			&models.User{
				UserAllOf0: models.UserAllOf0{
					Meta: &models.ObjMeta{
						ID: "uid2",
					},
				},
				UserMutable: models.UserMutable{
					AuthIdentifier: "user2",
				},
			},
		},
	}

	// create
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps)
	cOps.EXPECT().AccountCreate(m).Return(res, nil)
	rOps := mockmgmtclient.NewMockRoleClient(mockCtrl)
	mAPI.EXPECT().Role().Return(rOps)
	rOps.EXPECT().RoleList(r).Return(rRes, nil)
	uOps := mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(uOps)
	uOps.EXPECT().UserList(u).Return(uRes, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err := parseAndRun([]string{"account", "create", "-n", string(params.Payload.Name),
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-u", "user1:" + centrald.SystemAdminRole, "-u", "user2:" + centrald.SystemAdminRole,
		"-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.Account{params.Payload}, te.jsonData)

	// create, invalid columns
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "create", "-n", string(params.Payload.Name),
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-u", "user1:" + centrald.SystemAdminRole, "-u", "user2:" + centrald.SystemAdminRole,
		"--columns", "Name,ID,FOO"})
	assert.NotNil(err)
	assert.Regexp("invalid column", err)

	// create, API model error, specify tenant account
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "nuvoloso").Return("aid1", nil)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps).MinTimes(1)
	params.Payload.TenantAccountID = "aid1"
	apiErr := &account.AccountCreateDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	cOps.EXPECT().AccountCreate(m).Return(nil, apiErr)
	rOps = mockmgmtclient.NewMockRoleClient(mockCtrl)
	mAPI.EXPECT().Role().Return(rOps)
	rOps.EXPECT().RoleList(r).Return(rRes, nil)
	uOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(uOps)
	uOps.EXPECT().UserList(u).Return(uRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "create", "-A", "nuvoloso", "-n", string(params.Payload.Name),
		"-c", "name, ID",
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-u", "user1:" + centrald.SystemAdminRole, "-u", "user2:" + centrald.SystemAdminRole})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())
	params.Payload.TenantAccountID = ""
	appCtx.Account, appCtx.AccountID = "", ""

	// create, API failure arbitrary error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps)
	otherErr := fmt.Errorf("API ERROR")
	cOps.EXPECT().AccountCreate(m).Return(nil, otherErr)
	rOps = mockmgmtclient.NewMockRoleClient(mockCtrl)
	mAPI.EXPECT().Role().Return(rOps)
	rOps.EXPECT().RoleList(r).Return(rRes, nil)
	uOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(uOps)
	uOps.EXPECT().UserList(u).Return(uRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "create", "-n", string(params.Payload.Name),
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-u", "user1:" + centrald.SystemAdminRole, "-u", "user2:" + centrald.SystemAdminRole})
	assert.NotNil(err)
	assert.Equal(otherErr, err)

	mockCtrl.Finish()
	t.Log("init context failure")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "Tenant").Return("", fmt.Errorf("ctx error"))
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "create", "-A", "Tenant", "-n", string(params.Payload.Name)})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// user not found
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	rOps = mockmgmtclient.NewMockRoleClient(mockCtrl)
	mAPI.EXPECT().Role().Return(rOps)
	rOps.EXPECT().RoleList(r).Return(rRes, nil)
	uOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(uOps)
	uOps.EXPECT().UserList(u).Return(uRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "create", "-n", string(params.Payload.Name),
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-u", "user1:" + centrald.SystemAdminRole, "-u", "user3:" + centrald.SystemAdminRole})
	assert.NotNil(err)
	assert.Equal("User \"user3\" not found", err.Error())

	// role not found
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	rOps = mockmgmtclient.NewMockRoleClient(mockCtrl)
	mAPI.EXPECT().Role().Return(rOps)
	rOps.EXPECT().RoleList(r).Return(rRes, nil)
	uOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(uOps)
	uOps.EXPECT().UserList(u).Return(uRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "create", "-n", string(params.Payload.Name),
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-u", "user1:" + centrald.SystemAdminRole, "-u", "user2:other"})
	assert.NotNil(err)
	assert.Equal("Role \"other\" not found", err.Error())

	// role list failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	rOps = mockmgmtclient.NewMockRoleClient(mockCtrl)
	mAPI.EXPECT().Role().Return(rOps)
	rOps.EXPECT().RoleList(r).Return(nil, centrald.ErrorDbError)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "create", "-n", string(params.Payload.Name),
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-u", "user1:" + centrald.SystemAdminRole, "-u", "user2:other"})
	assert.Error(err)

	// user list failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	rOps = mockmgmtclient.NewMockRoleClient(mockCtrl)
	mAPI.EXPECT().Role().Return(rOps)
	rOps.EXPECT().RoleList(r).Return(rRes, nil)
	uOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(uOps)
	uOps.EXPECT().UserList(u).Return(nil, centrald.ErrorDbError)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "create", "-n", string(params.Payload.Name),
		"-d", string(params.Payload.Description), "-t", "tag1", "-t", "tag2",
		"-u", "user1:" + centrald.SystemAdminRole, "-u", "user2:other"})
	assert.Error(err)

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "create", "-n", string(params.Payload.Name), "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestAccountDelete(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	lRes := &account.AccountListOK{
		Payload: []*models.Account{
			&models.Account{
				AccountAllOf0: models.AccountAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				AccountMutable: models.AccountMutable{
					Name:        "accountName",
					Description: "account1 object",
					Disabled:    true,
					Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
					UserRoles: map[string]models.AuthRole{
						"uid1": models.AuthRole{RoleID: "rid1"},
						"uid2": models.AuthRole{RoleID: "rid2"},
					},
				},
			},
		},
	}
	m := mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	dParams := account.NewAccountDeleteParams()
	dParams.ID = string(lRes.Payload[0].Meta.ID)
	dRet := &account.AccountDeleteNoContent{}

	// delete
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps).MinTimes(1)
	cOps.EXPECT().AccountList(m).Return(lRes, nil)
	cOps.EXPECT().AccountDelete(dParams).Return(dRet, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err := parseAndRun([]string{"account", "delete", "-n", string(lRes.Payload[0].Name), "--confirm"})
	assert.Nil(err)

	// delete, --confirm not specified
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "delete", "-n", string(lRes.Payload[0].Name)})
	assert.NotNil(err)
	assert.Regexp("--confirm", err.Error())

	// delete, not found
	emptyRes := &account.AccountListOK{
		Payload: []*models.Account{},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps).MinTimes(1)
	cOps.EXPECT().AccountList(m).Return(emptyRes, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "delete", "-n", string(lRes.Payload[0].Name), "--confirm"})
	assert.NotNil(err)
	assert.Regexp("not found", err.Error())

	// delete, list error
	otherErr := fmt.Errorf("other error")
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps).MinTimes(1)
	cOps.EXPECT().AccountList(m).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "delete", "-n", string(lRes.Payload[0].Name), "--confirm"})
	assert.NotNil(err)
	assert.Regexp("other error", err.Error())

	// delete, API model error
	apiErr := &account.AccountDeleteDefault{
		Payload: &models.Error{
			Code:    400,
			Message: swag.String("api error"),
		},
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps).MinTimes(1)
	cOps.EXPECT().AccountList(m).Return(lRes, nil)
	cOps.EXPECT().AccountDelete(dParams).Return(nil, apiErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "delete", "-n", string(lRes.Payload[0].Name), "--confirm"})
	assert.NotNil(err)
	assert.Equal(*apiErr.Payload.Message, err.Error())

	// delete, API arbitrary error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps).MinTimes(1)
	cOps.EXPECT().AccountList(m).Return(lRes, nil)
	cOps.EXPECT().AccountDelete(dParams).Return(nil, otherErr)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "delete", "-n", string(lRes.Payload[0].Name), "--confirm"})
	assert.NotNil(err)
	assert.Regexp("other error", err.Error())

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
	initAccount()
	err = parseAndRun([]string{"account", "delete", "-A", "System", "-n", string(lRes.Payload[0].Name), "--confirm"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "delete", "-n", string(lRes.Payload[0].Name), "confirm"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestAccountModify(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil

	now := time.Now()
	lRes := &account.AccountListOK{
		Payload: []*models.Account{
			&models.Account{
				AccountAllOf0: models.AccountAllOf0{
					Meta: &models.ObjMeta{
						ID:           "id1",
						Version:      1,
						TimeCreated:  strfmt.DateTime(now),
						TimeModified: strfmt.DateTime(now),
					},
				},
				AccountMutable: models.AccountMutable{
					Name:        "accountName",
					Description: "account1 object",
					Disabled:    true,
					Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
					UserRoles: map[string]models.AuthRole{
						"uid1": models.AuthRole{RoleID: "rid1"},
						"uid2": models.AuthRole{RoleID: "rid2"},
					},
					SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
						DisableSnapshotCreation:  true,
						RetentionDurationSeconds: swag.Int32(99),
						Inherited:                true,
					},
					SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
						CspDomainID:        "domID",
						ProtectionDomainID: "pdID",
						Inherited:          false,
					},
					VsrManagementPolicy: &models.VsrManagementPolicy{
						RetentionDurationSeconds: swag.Int32(100),
						NoDelete:                 false,
						Inherited:                true,
					},
				},
			},
		},
	}
	lParams := account.NewAccountListParams()
	m := mockmgmtclient.NewAccountMatcher(t, lParams)
	r := mockmgmtclient.NewRoleMatcher(t, role.NewRoleListParams())
	rRes := &role.RoleListOK{
		Payload: []*models.Role{
			&models.Role{
				RoleAllOf0: models.RoleAllOf0{
					Meta: &models.ObjMeta{
						ID: "rid1",
					},
				},
				RoleMutable: models.RoleMutable{
					Name: "someRole",
				},
			},
		},
	}
	u := mockmgmtclient.NewUserMatcher(t, user.NewUserListParams())
	uRes := &user.UserListOK{
		Payload: []*models.User{
			&models.User{
				UserAllOf0: models.UserAllOf0{
					Meta: &models.ObjMeta{
						ID: "uid1",
					},
				},
				UserMutable: models.UserMutable{
					AuthIdentifier: "user1",
				},
			},
			&models.User{
				UserAllOf0: models.UserAllOf0{
					Meta: &models.ObjMeta{
						ID: "uid2",
					},
				},
				UserMutable: models.UserMutable{
					AuthIdentifier: "user2",
				},
			},
		},
	}
	aRes := &account.AccountFetchOK{
		Payload: &models.Account{
			AccountAllOf0: models.AccountAllOf0{
				Meta: &models.ObjMeta{
					ID:           "id1",
					Version:      1,
					TimeCreated:  strfmt.DateTime(now),
					TimeModified: strfmt.DateTime(now),
				},
			},
			AccountMutable: models.AccountMutable{
				Name: "accountName",
				SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
					DisableSnapshotCreation:  true,
					RetentionDurationSeconds: swag.Int32(99),
					Inherited:                true,
				},
				SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
					CspDomainID:        "domID",
					ProtectionDomainID: "pdID",
					Inherited:          false,
				},
				VsrManagementPolicy: &models.VsrManagementPolicy{
					RetentionDurationSeconds: swag.Int32(100),
					NoDelete:                 false,
					Inherited:                true,
				},
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

	afParams := account.NewAccountFetchParams()
	afParams.ID = "id1"
	// update (APPEND)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	cOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps).MinTimes(1)
	cOps.EXPECT().AccountList(m).Return(lRes, nil)
	rOps := mockmgmtclient.NewMockRoleClient(mockCtrl)
	mAPI.EXPECT().Role().Return(rOps)
	rOps.EXPECT().RoleList(r).Return(rRes, nil)
	uOps := mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(uOps)
	uOps.EXPECT().UserList(u).Return(uRes, nil)
	uParams := account.NewAccountUpdateParams()
	uParams.Payload = &models.AccountMutable{
		Name:        "newName",
		Description: "new Description",
		UserRoles:   map[string]models.AuthRole{"uid1": {RoleID: "rid1"}},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"name", "description"}
	uParams.Append = []string{"userRoles"}
	um := mockmgmtclient.NewAccountMatcher(t, uParams)
	uRet := account.NewAccountUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().AccountFetch(afParams).Return(aRes, nil)
	cOps.EXPECT().AccountUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te := &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err := parseAndRun([]string{"account", "modify", "-n", string(lRes.Payload[0].Name),
		"-N", "newName", "-d", " new Description ", "-u", "user1:somerole", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.Account{uRet.Payload}, te.jsonData)

	// same but with SET, inherited Snapshot policy all flags to false + set Retention
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	aRes.Payload.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{ // Inherited = false
		DisableSnapshotCreation:  true,
		DeleteLast:               true,
		DeleteVolumeWithLast:     true,
		NoDelete:                 true,
		RetentionDurationSeconds: swag.Int32(0),
		Inherited:                true,
	}
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "accountName").Return("id1", nil)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps).MinTimes(1)
	cOps.EXPECT().AccountFetch(afParams).Return(aRes, nil)
	rOps = mockmgmtclient.NewMockRoleClient(mockCtrl)
	mAPI.EXPECT().Role().Return(rOps)
	rOps.EXPECT().RoleList(r).Return(rRes, nil)
	uOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(uOps)
	uOps.EXPECT().UserList(u).Return(uRes, nil)
	uParams = account.NewAccountUpdateParams()
	uParams.Payload = &models.AccountMutable{
		Name:      "newName",
		Disabled:  false,
		UserRoles: map[string]models.AuthRole{"uid1": {RoleID: "rid1", Disabled: true}},
		SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
			DisableSnapshotCreation:  false,
			DeleteLast:               false,
			DeleteVolumeWithLast:     false,
			NoDelete:                 false,
			RetentionDurationSeconds: swag.Int32(7 * 24 * 60 * 60),
			Inherited:                false,
		},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"name", "disabled", "userRoles", "snapshotManagementPolicy"}
	um = mockmgmtclient.NewAccountMatcher(t, uParams)
	uRet = account.NewAccountUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().AccountUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "modify", "-A", string(lRes.Payload[0].Name),
		"-N", "newName", "--enable", "-u", "user1:someRole:disabled",
		"--smp-enable-snapshots", "--smp-delete-snapshots", "--smp-no-delete-last-snapshot", "--smp-no-delete-vol-with-last", "--smp-retention-days", "7",
		"--user-roles-action=SET", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.Account{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// same but with SET, non-inherited Snapshot policy all flags to false + set Retention
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	aRes.Payload.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{ // Inherited = false
		DisableSnapshotCreation:  true,
		DeleteLast:               true,
		DeleteVolumeWithLast:     true,
		NoDelete:                 true,
		RetentionDurationSeconds: swag.Int32(0),
	}
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "accountName").Return("id1", nil)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps).MinTimes(1)
	cOps.EXPECT().AccountFetch(afParams).Return(aRes, nil)
	rOps = mockmgmtclient.NewMockRoleClient(mockCtrl)
	mAPI.EXPECT().Role().Return(rOps)
	rOps.EXPECT().RoleList(r).Return(rRes, nil)
	uOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(uOps)
	uOps.EXPECT().UserList(u).Return(uRes, nil)
	uParams = account.NewAccountUpdateParams()
	uParams.Payload = &models.AccountMutable{
		Name:      "newName",
		Disabled:  false,
		UserRoles: map[string]models.AuthRole{"uid1": {RoleID: "rid1", Disabled: true}},
		SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
			DisableSnapshotCreation:  false,
			DeleteLast:               false,
			DeleteVolumeWithLast:     false,
			NoDelete:                 false,
			RetentionDurationSeconds: swag.Int32(7 * 24 * 60 * 60),
		},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"name", "disabled", "userRoles", "snapshotManagementPolicy"}
	um = mockmgmtclient.NewAccountMatcher(t, uParams)
	uRet = account.NewAccountUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().AccountUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "modify", "-A", string(lRes.Payload[0].Name),
		"-N", "newName", "--enable", "-u", "user1:someRole:disabled",
		"--smp-enable-snapshots", "--smp-delete-snapshots", "--smp-no-delete-last-snapshot", "--smp-no-delete-vol-with-last", "--smp-retention-days", "7",
		"--user-roles-action=SET", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.Account{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// Snapshot management policy all flags to true, catalog policy changes (with IDs)
	aRes.Payload.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{
		DisableSnapshotCreation: false,
		DeleteLast:              false,
		DeleteVolumeWithLast:    false,
		NoDelete:                false,
	}
	aRes.Payload.SnapshotCatalogPolicy = &models.SnapshotCatalogPolicy{
		CspDomainID:        "domID",
		ProtectionDomainID: "pdID",
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "accountName").Return("id1", nil)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps).MinTimes(2)
	cOps.EXPECT().AccountFetch(afParams).Return(aRes, nil)
	rOps = mockmgmtclient.NewMockRoleClient(mockCtrl)
	mAPI.EXPECT().Role().Return(rOps)
	rOps.EXPECT().RoleList(r).Return(rRes, nil)
	uOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(uOps)
	uOps.EXPECT().UserList(u).Return(uRes, nil)
	uParams = account.NewAccountUpdateParams()
	uParams.Payload = &models.AccountMutable{
		UserRoles: map[string]models.AuthRole{"uid1": {RoleID: "rid1", Disabled: true}},
		SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
			DisableSnapshotCreation: true,
			DeleteLast:              true,
			DeleteVolumeWithLast:    true,
			NoDelete:                true,
		},
		SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
			CspDomainID:        "newDomID",
			ProtectionDomainID: "newPdID",
		},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"userRoles", "snapshotManagementPolicy", "snapshotCatalogPolicy"}
	um = mockmgmtclient.NewAccountMatcher(t, uParams)
	uRet = account.NewAccountUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().AccountUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "modify", "-A", string(lRes.Payload[0].Name),
		"--smp-disable-snapshots", "--smp-no-delete-snapshots", "--smp-delete-last-snapshot", "--smp-delete-vol-with-last",
		"--scp-domain-id", "newDomID", "--scp-protection-domain-id", "newPdID",
		"-u", "user1:someRole:disabled", "--user-roles-action=SET", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.Account{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// enable snapshots for non-inherited snapshot management policy
	aRes.Payload.SnapshotManagementPolicy = &models.SnapshotManagementPolicy{
		DisableSnapshotCreation: false,
		Inherited:               false,
	}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "accountName").Return("id1", nil)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps).MinTimes(2)
	cOps.EXPECT().AccountFetch(afParams).Return(aRes, nil)
	rOps = mockmgmtclient.NewMockRoleClient(mockCtrl)
	mAPI.EXPECT().Role().Return(rOps)
	rOps.EXPECT().RoleList(r).Return(rRes, nil)
	uOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(uOps)
	uOps.EXPECT().UserList(u).Return(uRes, nil)
	uParams = account.NewAccountUpdateParams()
	uParams.Payload = &models.AccountMutable{
		UserRoles: map[string]models.AuthRole{"uid1": {RoleID: "rid1", Disabled: true}},
		SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
			DisableSnapshotCreation: true,
			Inherited:               false,
		},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"userRoles", "snapshotManagementPolicy"}
	um = mockmgmtclient.NewAccountMatcher(t, uParams)
	uRet = account.NewAccountUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().AccountUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "modify", "-A", string(lRes.Payload[0].Name),
		"--smp-disable-snapshots", "-u", "user1:someRole:disabled",
		"--user-roles-action=SET", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.Account{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// use system snapshot and VSR management policies
	aRes.Payload.SnapshotManagementPolicy.Inherited = false
	aRes.Payload.SnapshotCatalogPolicy.Inherited = false
	aRes.Payload.VsrManagementPolicy.Inherited = false
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "accountName").Return("id1", nil)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps).MinTimes(2)
	cOps.EXPECT().AccountFetch(afParams).Return(aRes, nil)
	uParams = account.NewAccountUpdateParams()
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"snapshotManagementPolicy", "snapshotCatalogPolicy", "vsrManagementPolicy"}
	uParams.Payload = &models.AccountMutable{
		SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
			Inherited: true,
		},
		SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
			Inherited: true,
		},
		VsrManagementPolicy: &models.VsrManagementPolicy{
			Inherited: true,
		},
	}
	um = mockmgmtclient.NewAccountMatcher(t, uParams)
	uRet = account.NewAccountUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().AccountUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "modify", "-A", string(lRes.Payload[0].Name),
		"--vsrp-inherit", "--smp-inherit", "--scp-inherit", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.Account{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// inherit already inherited policies
	aRes.Payload.SnapshotManagementPolicy.Inherited = true
	aRes.Payload.SnapshotCatalogPolicy.Inherited = true
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "accountName").Return("id1", nil)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps).MinTimes(1)
	cOps.EXPECT().AccountFetch(afParams).Return(aRes, nil)
	uParams = account.NewAccountUpdateParams()
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"snapshotManagementPolicy", "snapshotCatalogPolicy"}
	uParams.Payload = &models.AccountMutable{
		SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
			Inherited: true,
		},
		SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
			Inherited: true,
		},
	}
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "modify", "-A", string(lRes.Payload[0].Name),
		"--vsrp-inherit", "--smp-inherit", "--scp-inherit", "-V", "1", "-o", "json"})
	assert.NotNil(err)
	assert.Equal("No modifications specified", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// set VSR retention period for VSR management policy, snapshot catalog policy changes (with names)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps = mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "accountName").Return("id1", nil)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps).MinTimes(2)
	cOps.EXPECT().AccountFetch(afParams).Return(aRes, nil)
	pdm := mockmgmtclient.NewProtectionDomainMatcher(t, protection_domain.NewProtectionDomainListParams())
	pdOps := mockmgmtclient.NewMockProtectionDomainClient(mockCtrl)
	mAPI.EXPECT().ProtectionDomain().Return(pdOps).MinTimes(1)
	pdOps.EXPECT().ProtectionDomainList(pdm).Return(pdRes, nil).MinTimes(1)
	dm := mockmgmtclient.NewCspDomainMatcher(t, csp_domain.NewCspDomainListParams())
	dOps := mockmgmtclient.NewMockCSPDomainClient(mockCtrl)
	mAPI.EXPECT().CspDomain().Return(dOps).MinTimes(1)
	dOps.EXPECT().CspDomainList(dm).Return(domRes, nil).MinTimes(1)
	uParams = account.NewAccountUpdateParams()
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"vsrManagementPolicy", "snapshotCatalogPolicy"}
	uParams.Payload = &models.AccountMutable{
		VsrManagementPolicy: &models.VsrManagementPolicy{
			NoDelete:                 false,
			RetentionDurationSeconds: swag.Int32(7 * 24 * 60 * 60),
			Inherited:                false,
		},
		SnapshotCatalogPolicy: &models.SnapshotCatalogPolicy{
			CspDomainID:        "newDomID",
			ProtectionDomainID: "newPdID",
		},
	}
	um = mockmgmtclient.NewAccountMatcher(t, uParams)
	uRet = account.NewAccountUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().AccountUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "modify", "-A", string(lRes.Payload[0].Name),
		"--scp-domain", "newDomName", "--scp-protection-domain", "newPdName",
		"--vsrp-retention-days", "7", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.Account{uRet.Payload}, te.jsonData)
	appCtx.Account, appCtx.AccountID = "", ""

	// same but with REMOVE
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps).MinTimes(1)
	cOps.EXPECT().AccountList(m).Return(lRes, nil)
	cOps.EXPECT().AccountFetch(afParams).Return(aRes, nil)
	rOps = mockmgmtclient.NewMockRoleClient(mockCtrl)
	mAPI.EXPECT().Role().Return(rOps)
	rOps.EXPECT().RoleList(r).Return(rRes, nil)
	uOps = mockmgmtclient.NewMockUserClient(mockCtrl)
	mAPI.EXPECT().User().Return(uOps)
	uOps.EXPECT().UserList(u).Return(uRes, nil)
	uParams = account.NewAccountUpdateParams()
	uParams.Payload = &models.AccountMutable{
		Name:      "newName",
		Disabled:  true,
		UserRoles: map[string]models.AuthRole{"uid1": {RoleID: "rid1"}},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"name", "disabled"}
	uParams.Remove = []string{"userRoles"}
	um = mockmgmtclient.NewAccountMatcher(t, uParams)
	uRet = account.NewAccountUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().AccountUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "modify", "-n", string(lRes.Payload[0].Name),
		"-N", "newName", "--disable", "-u", "user1:someRole",
		"--user-roles-action", "REMOVE", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.Account{uRet.Payload}, te.jsonData)

	// same but empty SET
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(cOps).MinTimes(1)
	cOps.EXPECT().AccountList(m).Return(lRes, nil)
	cOps.EXPECT().AccountFetch(afParams).Return(aRes, nil)
	uParams = account.NewAccountUpdateParams()
	uParams.Payload = &models.AccountMutable{
		Name:        "newName",
		Description: "new Description",
		UserRoles:   map[string]models.AuthRole{},
	}
	uParams.ID = string(lRes.Payload[0].Meta.ID)
	uParams.Version = swag.Int32(1)
	uParams.Set = []string{"name", "description", "userRoles"}
	um = mockmgmtclient.NewAccountMatcher(t, uParams)
	uRet = account.NewAccountUpdateOK()
	uRet.Payload = lRes.Payload[0]
	cOps.EXPECT().AccountUpdate(um).Return(uRet, nil)
	appCtx.API = mAPI
	te = &TestEmitter{}
	appCtx.Emitter = te
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "modify", "-n", string(lRes.Payload[0].Name),
		"-N", "newName", "-d", " new Description ",
		"--user-roles-action", "SET", "-V", "1", "-o", "json"})
	assert.Nil(err)
	assert.NotNil(te.jsonData)
	assert.EqualValues([]*models.Account{uRet.Payload}, te.jsonData)

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
	initAccount()
	err = parseAndRun([]string{"account", "modify", "-A", "System", "-n", string(lRes.Payload[0].Name)})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())
	appCtx.Account, appCtx.AccountID = "", ""

	// other error cases
	updateErr := account.NewAccountUpdateDefault(400)
	updateErr.Payload = &models.Error{Code: 400, Message: swag.String("update error")}
	otherErr := fmt.Errorf("other error")
	mNotNil := gomock.Not(gomock.Nil)
	errTCs := []struct {
		name      string
		args      []string
		re        string
		alErr     error
		alRC      *account.AccountListOK
		alRC2     *account.AccountFetchOK
		alErr2    error
		sfErr     error
		updateErr error
		roleErr   error
		noMock    bool
		noAL      bool
		mockAF    bool
	}{
		{
			name:   "No modifications",
			args:   []string{"account", "modify", "-n", string(lRes.Payload[0].Name), "-V", "1", "-o", "json"},
			re:     "No modifications",
			mockAF: true,
		},
		{
			name:      "Update default error",
			args:      []string{"account", "modify", "-n", string(lRes.Payload[0].Name), "-N", "NewName", "-V", "1", "-o", "json"},
			re:        "update error",
			updateErr: updateErr,
			mockAF:    true,
		},
		{
			name:      "Update other error",
			args:      []string{"account", "modify", "-n", string(lRes.Payload[0].Name), "-N", "NewName", "-V", "1", "-o", "json"},
			re:        "other error",
			updateErr: otherErr,
			mockAF:    true,
		},
		{
			name:   "invalid columns",
			args:   []string{"account", "modify", "-n", string(lRes.Payload[0].Name), "--columns", "ID,foo"},
			re:     "invalid column",
			noMock: true,
			mockAF: true,
		},
		{
			name: "account not found",
			args: []string{"account", "modify", "-n", string(lRes.Payload[0].Name), "-N", "NewName", "-V", "1", "-o", "json"},
			re:   "account.*not found",
			alRC: &account.AccountListOK{Payload: []*models.Account{}},
		},
		{
			name:  "account list error",
			args:  []string{"account", "modify", "-n", string(lRes.Payload[0].Name), "-N", "NewName", "-V", "1", "-o", "json"},
			re:    "other error",
			alErr: otherErr,
		},
		{
			name:    "role list error",
			args:    []string{"account", "modify", "-n", string(lRes.Payload[0].Name), "-N", "NewName", "-u", "u:r", "-o", "json"},
			re:      "other error",
			roleErr: otherErr,
			mockAF:  true,
		},
		{
			name: "enable/disable",
			args: []string{"account", "modify", "-n", string(lRes.Payload[0].Name), "--enable", "--disable", "-o", "json"},
			re:   "enable.*disable.*together",
		},
		{
			name: "no name",
			args: []string{"account", "modify", "--new-name=NEW", "-o", "json"},
			re:   "Neither.*account.*name ",
			noAL: true,
		},
		{
			name: "enable-snapshots/disable-snapshots",
			args: []string{"account", "modify", "-n", string(lRes.Payload[0].Name), "--smp-enable-snapshots", "--smp-disable-snapshots", "-o", "json"},
			re:   "do not specify 'smp-enable-snapshots' and 'smp-disable-snapshots' or 'smp-delete-snapshots' and 'smp-no-delete-snapshots' together",
			noAL: true,
		},
		{
			name: "no-delete-snapshots/smp-retention-days",
			args: []string{"account", "modify", "-n", string(lRes.Payload[0].Name), "--smp-no-delete-snapshots", "--smp-retention-days", "7", "-o", "json"},
			re:   "do not specify 'smp-no-delete-snapshots' and 'smp-retention-days' together",
			noAL: true,
		},
		{
			name: "use system snapshot policy together with its modifications",
			args: []string{"account", "modify", "-n", string(lRes.Payload[0].Name), "--smp-inherit", "--smp-disable-snapshots", "-o", "json"},
			re:   "do not specify.*together with modifications",
			noAL: true,
		},
		{
			name: "use system VSR policy together with its modifications",
			args: []string{"account", "modify", "-n", string(lRes.Payload[0].Name), "--vsrp-inherit", "--vsrp-retention-days", "7", "-o", "json"},
			re:   "do not specify.*together with modifications",
			noAL: true,
		},
		{
			name: "use system snapshot catalog policy together with its modifications",
			args: []string{"account", "modify", "-n", string(lRes.Payload[0].Name), "--scp-inherit", "--scp-domain", "new_cspdomain", "-o", "json"},
			re:   "do not specify.*together with modifications",
			noAL: true,
		},
		{
			name:   "enable-snapshots-afetch-error",
			args:   []string{"account", "modify", "-n", string(lRes.Payload[0].Name), "--smp-enable-snapshots", "-o", "json"},
			re:     "other error",
			alErr2: otherErr,
		},
	}
	for _, tc := range errTCs {
		t.Logf("case: %s", tc.name)
		if !tc.noMock {
			mockCtrl.Finish()
			mockCtrl = gomock.NewController(t)
			mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
			if !tc.noAL {
				numAccountListCalls := 1
				if tc.alErr2 != nil {
					numAccountListCalls = 2
				}
				cOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
				mAPI.EXPECT().Account().Return(cOps).MinTimes(numAccountListCalls)
				if tc.alErr != nil {
					cOps.EXPECT().AccountList(m).Return(nil, tc.alErr)
				} else {
					if tc.alRC == nil {
						tc.alRC = lRes
					}
					cOps.EXPECT().AccountList(m).Return(tc.alRC, nil)
				}
				if tc.updateErr != nil {
					cOps.EXPECT().AccountUpdate(mNotNil).Return(nil, tc.updateErr)
				}
				if tc.alErr2 != nil {
					cOps.EXPECT().AccountFetch(afParams).Return(nil, tc.alErr2)
				} else if tc.alRC2 != nil {
					cOps.EXPECT().AccountFetch(afParams).Return(tc.alRC2, nil)
				} else if tc.mockAF {
					cOps.EXPECT().AccountFetch(afParams).Return(aRes, nil)
				}
			}
			if tc.roleErr != nil {
				rOps = mockmgmtclient.NewMockRoleClient(mockCtrl)
				mAPI.EXPECT().Role().Return(rOps)
				rOps.EXPECT().RoleList(r).Return(nil, tc.roleErr)
			}
			appCtx.API = mAPI
		}
		te = &TestEmitter{}
		appCtx.Emitter = te
		parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
		initParser()
		initAccount()
		err = parseAndRun(tc.args)
		assert.NotNil(err)
		assert.Nil(te.jsonData)
		assert.Regexp(tc.re, err.Error())
	}

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "modify", "-A", string(lRes.Payload[0].Name),
		"-N", "newName", "--enable", "-u", "user1:someRole:disabled",
		"--smp-enable-snapshots", "--smp-delete-snapshots", "--smp-no-delete-last-snapshot", "--smp-no-delete-vol-with-last", "--smp-retention-days", "7",
		"--user-roles-action=SET", "-V", "1", "o", "json"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}

func TestAccountResetSecret(t *testing.T) {
	assert := assert.New(t)
	savedParser := parser
	defer func() {
		interactiveReader = os.Stdin
		outputWriter = os.Stdout
		parser = savedParser
	}()
	interactiveReader = nil
	defer func() {
		appCtx.Account, appCtx.AccountID = "", ""
	}()

	authAccountID := "authAccountID"
	lRes := &account.AccountListOK{
		Payload: []*models.Account{
			&models.Account{
				AccountAllOf0: models.AccountAllOf0{
					Meta: &models.ObjMeta{
						ID: "accountID",
					},
				},
				AccountMutable: models.AccountMutable{
					Name: "accountName",
				},
			},
			&models.Account{
				AccountAllOf0: models.AccountAllOf0{
					Meta: &models.ObjMeta{
						ID: "systemID",
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

	t.Log("case: GLOBAL")
	appCtx.Account, appCtx.AccountID = "", authAccountID
	params := account.NewAccountSecretResetParams()
	params.ID = authAccountID
	params.AccountSecretScope = common.AccountSecretScopeGlobal
	mASR := mockmgmtclient.NewAccountMatcher(t, params)
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockmgmtclient.NewMockAPI(mockCtrl)
	aOps := mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps).MinTimes(1)
	aOps.EXPECT().AccountSecretReset(mASR).Return(nil, nil)
	appCtx.API = mAPI
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err := parseAndRun([]string{"account", "reset-secret", "-S", "GLOBAL"})
	assert.Nil(err)
	mockCtrl.Finish()

	t.Log("case: GLOBAL, recursive, named account")
	appCtx.Account, appCtx.AccountID = "", ""
	params = account.NewAccountSecretResetParams()
	params.ID = string(lRes.Payload[0].Meta.ID)
	params.Recursive = swag.Bool(true)
	params.AccountSecretScope = common.AccountSecretScopeGlobal
	mASR = mockmgmtclient.NewAccountMatcher(t, params)
	mAL := mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps).MinTimes(1)
	aOps.EXPECT().AccountList(mAL).Return(lRes, nil).MinTimes(1)
	aOps.EXPECT().AccountSecretReset(mASR).Return(nil, nil).MinTimes(1)
	appCtx.API = mAPI
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "reset-secret", "-S", "GLOBAL", "-r", "-n", string(lRes.Payload[0].Name)})
	assert.Nil(err)
	mockCtrl.Finish()

	t.Log("case: CSPDOMAIN, recursive, apiError")
	appCtx.Account, appCtx.AccountID = "", authAccountID
	params = account.NewAccountSecretResetParams()
	params.ID = authAccountID
	params.Recursive = swag.Bool(true)
	params.AccountSecretScope = common.AccountSecretScopeCspDomain
	params.CspDomainID = swag.String(string(resCspDomains.Payload[0].Meta.ID))
	mASR = mockmgmtclient.NewAccountMatcher(t, params)
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps).MinTimes(1)
	apiErr := &account.AccountSecretResetDefault{
		Payload: &models.Error{Message: swag.String("secret-reset-error")},
	}
	aOps.EXPECT().AccountSecretReset(mASR).Return(nil, apiErr).MinTimes(1)
	appCtx.API = mAPI
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "reset-secret", "-S", "CSPDOMAIN", "-r", "-D", string(resCspDomains.Payload[0].Name)})
	assert.NotNil(err)
	assert.Regexp("secret-reset-error", err)
	mockCtrl.Finish()

	t.Log("case: CLUSTER")
	appCtx.Account, appCtx.AccountID = "", authAccountID
	params = account.NewAccountSecretResetParams()
	params.ID = authAccountID
	params.AccountSecretScope = common.AccountSecretScopeCluster
	params.ClusterID = swag.String(string(resClusters.Payload[0].Meta.ID))
	mASR = mockmgmtclient.NewAccountMatcher(t, params)
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps).MinTimes(1)
	aOps.EXPECT().AccountSecretReset(mASR).Return(nil, nil).MinTimes(1)
	appCtx.API = mAPI
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "reset-secret", "-S", "CLUSTER", "-D", string(resCspDomains.Payload[0].Name), "-C", string(resClusters.Payload[0].Name)})
	assert.Nil(err)
	mockCtrl.Finish()

	t.Log("case: CSPDOMAIN domain-name missing")
	appCtx.Account, appCtx.AccountID = "", authAccountID
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
	appCtx.API = mAPI
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "reset-secret", "-S", "CSPDOMAIN"})
	assert.NotNil(err)
	assert.Regexp("missing domain-name", err)
	mockCtrl.Finish()

	t.Log("case: CLUSTER cluster-name missing")
	appCtx.Account, appCtx.AccountID = "", authAccountID
	mockCtrl = gomock.NewController(t)
	mAPI = cacheLoadHelper(t, mockCtrl, loadDomains, loadClusters, loadNodes)
	appCtx.API = mAPI
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "reset-secret", "-S", "CLUSTER"})
	assert.NotNil(err)
	assert.Regexp("missing cluster-name", err)
	mockCtrl.Finish()

	t.Log("case: domain cache load error")
	appCtx.Account, appCtx.AccountID = "", authAccountID
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "reset-secret", "-S", "CLUSTER", "-C", "clusterName"})
	assert.NotNil(err)
	assert.Regexp("cluster-name requires domain", err)
	mockCtrl.Finish()

	t.Log("case: GLOBAL, invalid account")
	appCtx.Account, appCtx.AccountID = "", ""
	mAL = mockmgmtclient.NewAccountMatcher(t, account.NewAccountListParams())
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	aOps = mockmgmtclient.NewMockAccountClient(mockCtrl)
	mAPI.EXPECT().Account().Return(aOps).MinTimes(1)
	aOps.EXPECT().AccountList(mAL).Return(lRes, nil).MinTimes(1)
	appCtx.API = mAPI
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "reset-secret", "-S", "GLOBAL", "-r", "-n", "foo"})
	assert.NotNil(err)
	assert.Regexp("account.*not found", err)
	mockCtrl.Finish()

	t.Log("case: no auth identification")
	appCtx.Account, appCtx.AccountID = "", ""
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	appCtx.API = mAPI
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "reset-secret", "-S", "GLOBAL"})
	assert.NotNil(err)
	assert.Regexp("--account or --name", err)
	mockCtrl.Finish()

	t.Log("init context failure")
	mockCtrl = gomock.NewController(t)
	mAPI = mockmgmtclient.NewMockAPI(mockCtrl)
	authOps := mockmgmtclient.NewMockAuthenticationAPI(mockCtrl)
	mAPI.EXPECT().Authentication().Return(authOps)
	authOps.EXPECT().SetContextAccount("", "System").Return("", fmt.Errorf("ctx error"))
	appCtx.API = mAPI
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "reset-secret", "-A", "System", "-S", "GLOBAL"})
	assert.NotNil(err)
	assert.Equal("ctx error", err.Error())

	t.Log("unexpected arguments")
	parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
	initParser()
	initAccount()
	err = parseAndRun([]string{"account", "reset-secret", "-S", "GLOBAL", "foo"})
	assert.NotNil(err)
	assert.Regexp("unexpected arguments", err)
	appCtx.Account, appCtx.AccountID = "", ""
}
