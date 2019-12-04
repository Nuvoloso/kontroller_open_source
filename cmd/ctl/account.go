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
	"sort"
	"strings"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
)

func init() {
	initAccount()
}

func initAccount() {
	cmd, _ := parser.AddCommand("account", "Account object commands", "Account object subcommands", &accountCmd{})
	cmd.Aliases = []string{"accounts"}
	cmd.AddCommand("list", "List accounts", "List or search for Account objects.", &accountListCmd{})
	cmd.AddCommand("create", "Create an account", "Create an Account object", &accountCreateCmd{})
	cmd.AddCommand("delete", "Delete an account", "Delete an Account object", &accountDeleteCmd{})
	cmd.AddCommand("modify", "Modify an account", "Modify an Account object.", &accountModifyCmd{})
	cmd.AddCommand("reset-secret", "Reset an account secret", "Reset an Account secret in a specified scope.", &accountResetSecretCmd{})
	cmd.AddCommand("show-columns", "Show account table columns", "Show names of columns used in table format", &showColsCmd{columns: accountHeaders})
}

type accountCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string

	cacheHelper
}

// account record keys/headers and their description
var accountHeaders = map[string]string{
	hID:                       dID,
	hTimeCreated:              dTimeCreated,
	hTimeModified:             dTimeModified,
	hVersion:                  dVersion,
	hName:                     dName,
	hDescription:              dDescription,
	hDisabled:                 "Indicates if the account is disabled.",
	hTenant:                   "The encompassing tenant account for the account.",
	hAccountRoles:             "A list of roles that can be applied to users of the account.",
	hUserRoles:                "A mapping of user to authorization roles.",
	hSnapshotManagementPolicy: "Snapshot management policy",
	hSnapshotCatalogPolicy:    "Snapshot catalog policy",
	hVSRManagementPolicy:      "VSR management policy",
	hTags:                     dTags,
}

var accountDefaultHeaders = []string{hTenant, hName, hDescription, hUserRoles, hTags, hDisabled}

// makeRecord creates a map of properties
func (c *accountCmd) makeRecord(o *models.Account) map[string]string {
	tenantAccountName := string(o.TenantAccountID)
	if o.TenantAccountID != "" {
		if a, ok := c.accounts[string(o.TenantAccountID)]; ok {
			tenantAccountName = a.name
		}
	}
	roles := make([]string, 0, len(o.AccountRoles))
	for _, r := range o.AccountRoles {
		rn, ok := c.roles[string(r)]
		if !ok {
			rn = string(r)
		}
		roles = append(roles, rn)
	}
	users := make([]string, 0, len(o.UserRoles))
	for k, ur := range o.UserRoles {
		un, ok := c.users[k]
		if !ok {
			un = k
		}
		rn, ok := c.roles[string(ur.RoleID)]
		if !ok {
			rn = string(ur.RoleID)
		}
		disabled := ""
		if ur.Disabled {
			disabled = "(D)"
		}
		users = append(users, fmt.Sprintf("%s:%s%s", un, rn, disabled))
	}
	sort.Strings(users)
	vdrStr := o.SnapshotManagementPolicy.VolumeDataRetentionOnDelete
	if vdrStr == "" {
		vdrStr = "N/A"
	}
	var smpBuf, scpBuf, vsrpBuf strings.Builder
	fmt.Fprintf(&smpBuf, "Inherited: %v", o.SnapshotManagementPolicy.Inherited)
	fmt.Fprintf(&smpBuf, "\nDeleteLast: %v", o.SnapshotManagementPolicy.DeleteLast)
	fmt.Fprintf(&smpBuf, "\nDeleteVolumeWithLast: %v", o.SnapshotManagementPolicy.DeleteVolumeWithLast)
	fmt.Fprintf(&smpBuf, "\nDisableSnapshotCreation: %v", o.SnapshotManagementPolicy.DisableSnapshotCreation)
	fmt.Fprintf(&smpBuf, "\nNoDelete: %v", o.SnapshotManagementPolicy.NoDelete)
	fmt.Fprintf(&smpBuf, "\nRetentionDurationSeconds: %d", swag.Int32Value(o.SnapshotManagementPolicy.RetentionDurationSeconds))
	fmt.Fprintf(&smpBuf, "\nVolumeDataRetentionOnDelete: %s", vdrStr)

	if o.SnapshotCatalogPolicy != nil {
		fmt.Fprintf(&scpBuf, "Inherited: %v", o.SnapshotCatalogPolicy.Inherited)
		fmt.Fprintf(&scpBuf, "\nCspDomainID: %s", o.SnapshotCatalogPolicy.CspDomainID)
		fmt.Fprintf(&scpBuf, "\nProtectionDomainID: %s", o.SnapshotCatalogPolicy.ProtectionDomainID)
	}

	fmt.Fprintf(&vsrpBuf, "Inherited: %v", o.VsrManagementPolicy.Inherited)
	fmt.Fprintf(&vsrpBuf, "\nNoDelete: %v", o.VsrManagementPolicy.NoDelete)
	fmt.Fprintf(&vsrpBuf, "\nRetentionDurationSeconds: %d", swag.Int32Value(o.VsrManagementPolicy.RetentionDurationSeconds))
	return map[string]string{
		hID:                       string(o.Meta.ID),
		hTimeCreated:              time.Time(o.Meta.TimeCreated).Format(time.RFC3339),
		hTimeModified:             time.Time(o.Meta.TimeModified).Format(time.RFC3339),
		hVersion:                  fmt.Sprintf("%d", o.Meta.Version),
		hName:                     string(o.Name),
		hDescription:              string(o.Description),
		hDisabled:                 fmt.Sprintf("%v", o.Disabled),
		hTenant:                   tenantAccountName,
		hAccountRoles:             strings.Join(roles, "\n"),
		hUserRoles:                strings.Join(users, "\n"),
		hSnapshotManagementPolicy: smpBuf.String(),
		hSnapshotCatalogPolicy:    scpBuf.String(),
		hVSRManagementPolicy:      vsrpBuf.String(),
		hTags:                     strings.Join(o.Tags, ", "),
	}
}

func (c *accountCmd) validateColumns(columns string) error {
	var err error
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(accountHeaders), accountDefaultHeaders)
	return err
}

func (c *accountCmd) Emit(data []*models.Account) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.EmitJSON(data)
	case "yaml":
		return appCtx.EmitYAML(data)
	}
	c.cacheAccounts()
	c.cacheRoles()
	c.cacheUsers()
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

func (c *accountCmd) list(params *account.AccountListParams) ([]*models.Account, error) {
	if params == nil {
		params = account.NewAccountListParams()
	}
	res, err := appCtx.API.Account().AccountList(params)
	if err != nil {
		if e, ok := err.(*account.AccountListDefault); ok && e.Payload.Message != nil {
			return nil, fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res.Payload, nil
}

func (c *accountCmd) parseUserRoles(userRolesFlags map[string]string) (map[string]models.AuthRole, error) {
	if len(userRolesFlags) > 0 {
		if err := c.cacheRoles(); err != nil {
			return nil, err
		}
		if err := c.cacheUsers(); err != nil {
			return nil, err
		}
	}
	userRoles := map[string]models.AuthRole{}
	for u, r := range userRolesFlags {
		rd := strings.SplitN(r, ":", 2)
		uid := ""
		for k, v := range c.users {
			if v == u {
				uid = k
				break
			}
		}
		if uid == "" {
			return nil, fmt.Errorf("User \"%s\" not found", u)
		}
		rid := ""
		for k, v := range c.roles {
			if strings.EqualFold(v, rd[0]) {
				rid = k
				break
			}
		}
		if rid == "" {
			return nil, fmt.Errorf("Role \"%s\" not found", r)
		}
		disabled := false
		if len(rd) > 1 && rd[1] == "disabled" {
			disabled = true
		}
		userRoles[uid] = models.AuthRole{RoleID: models.ObjIDMutable(rid), Disabled: disabled}
	}
	return userRoles, nil
}

type accountCreateCmd struct {
	Name        string            `short:"n" long:"name" description:"An account name" required:"yes"`
	Description string            `short:"d" long:"description" description:"The purpose of the account"`
	Disabled    bool              `long:"disabled" description:"The account is disabled if true"`
	Tags        []string          `short:"t" long:"tag" description:"A tag value; repeat as needed"`
	UserRoles   map[string]string `short:"u" long:"user-roles" description:"A mapping of user name to a role name and optional disabled flag in the form user:role:disabled. Repeat as needed"`
	Columns     string            `short:"c" long:"columns" description:"Comma separated list of column names"`

	accountCmd
	remainingArgsCatcher
}

func (c *accountCreateCmd) Execute(args []string) error {
	if err := c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := appCtx.InitContextAccount(); err != nil {
		return err
	}
	params := account.NewAccountCreateParams().WithPayload(&models.Account{})
	if appCtx.Account != common.SystemAccount {
		params.Payload.TenantAccountID = models.ObjIDMutable(appCtx.AccountID)
	}
	params.Payload.Name = models.ObjName(c.Name)
	params.Payload.Description = models.ObjDescription(c.Description)
	params.Payload.Disabled = c.Disabled
	params.Payload.Tags = c.Tags
	userRoles, err := c.parseUserRoles(c.UserRoles)
	if err != nil {
		return err
	}
	params.Payload.UserRoles = userRoles
	res, err := appCtx.API.Account().AccountCreate(params)
	if err != nil {
		if e, ok := err.(*account.AccountCreateDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.Account{res.Payload})
}

type accountListCmd struct {
	Name       string   `short:"n" long:"name" description:"An account name"`
	DomainName string   `short:"D" long:"domain-name" description:"Name of a cloud service provider domain that is referenced by the account"`
	Tags       []string `short:"t" long:"tag" description:"A tag value; repeat as needed"`
	//All  bool     `short:"a" long:"show-all" description:"include disabled accounts"`
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`

	accountCmd
	remainingArgsCatcher
}

func (c *accountListCmd) Execute(args []string) error {
	var err error
	if err = c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	params := account.NewAccountListParams()
	if appCtx.Account != common.SystemAccount && appCtx.AccountID != "" && c.Name != "" {
		params.TenantAccountID = &appCtx.AccountID
	}
	if c.DomainName != "" {
		domID, err := c.validateDomainName(c.DomainName)
		if err != nil {
			return err
		}
		params.CspDomainID = &domID
	}
	if c.Name != "" {
		params.Name = &c.Name
	}
	params.Tags = c.Tags
	var res []*models.Account
	if res, err = c.list(params); err != nil {
		return err
	}
	return c.Emit(res)
}

type accountDeleteCmd struct {
	Name    string `short:"n" long:"name" description:"An account name" required:"yes"`
	Confirm bool   `long:"confirm" description:"Confirm the deletion of the account"`

	accountCmd
	remainingArgsCatcher
}

func (c *accountDeleteCmd) Execute(args []string) error {
	if err := c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if !c.Confirm {
		return fmt.Errorf("specify --confirm to delete the \"%s\" account", c.Name)
	}
	if err := appCtx.InitContextAccount(); err != nil {
		return err
	}
	id, err := c.validateAccount(c.Name, "")
	if err != nil {
		return err
	}
	dParams := account.NewAccountDeleteParams().WithID(id)
	if _, err = appCtx.API.Account().AccountDelete(dParams); err != nil {
		if e, ok := err.(*account.AccountDeleteDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return nil
}

type accountModifyCmd struct {
	Name            string            `short:"n" long:"name" description:"An account name. If not specified, the context account itself is modified"`
	NewName         string            `short:"N" long:"new-name" description:"The new name for the object"`
	Description     string            `short:"d" long:"description" description:"The new description; leading and trailing whitespace will be stripped"`
	Disable         bool              `long:"disable" description:"The account is disabled if set"`
	Enable          bool              `long:"enable" description:"The account is enabled if set"`
	UserRoles       map[string]string `short:"u" long:"user-roles" description:"A mapping of user name to a role name and optional disabled flag in the form user:role:disabled. Repeat as needed"`
	UserRolesAction string            `long:"user-roles-action" description:"Specifies how to process user-roles" choice:"APPEND" choice:"REMOVE" choice:"SET" default:"APPEND"`
	Version         int32             `short:"V" long:"version" description:"Enforce update of the specified version of the object"`
	SMPFlagsWithInheritance
	SCPFlagsWithInheritance
	VSRPFlagsWithInheritance
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`

	accountCmd
	remainingArgsCatcher
}

func (c *accountModifyCmd) Execute(args []string) error {
	var err error
	if err = c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err = c.SmpValidateInheritableFlags(); err != nil {
		return err
	}
	if err = c.ScpValidateInheritableFlags(); err != nil {
		return err
	}
	if err = c.VsrpValidateInheritableFlags(); err != nil {
		return err
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	accountID := ""
	if c.Name == "" {
		if appCtx.AccountID == "" {
			return fmt.Errorf("Neither of the --account or --name flags were specified")
		}
		accountID = appCtx.AccountID
	} else if accountID, err = c.validateAccount(c.Name, ""); err != nil {
		return err
	}
	nChg := 0
	uParams := account.NewAccountUpdateParams().WithPayload(&models.AccountMutable{})
	if c.NewName != "" {
		uParams.Payload.Name = models.ObjName(c.NewName)
		uParams.Set = append(uParams.Set, "name")
		nChg++
	}
	ts := strings.TrimSpace(c.Description)
	if c.Description != "" {
		uParams.Payload.Description = models.ObjDescription(ts)
		uParams.Set = append(uParams.Set, "description")
		nChg++
	}
	if c.Disable || c.Enable {
		if c.Disable && c.Enable {
			return fmt.Errorf("do not specify 'enable' and 'disable' together")
		}
		uParams.Payload.Disabled = c.Disable
		uParams.Set = append(uParams.Set, "disabled")
		nChg++
	}

	// note: since on account fetch global system policies for snapshot and VSR management are used, in case of any of them not explicitly set for the account object
	// the global settings will be applied here as part of the modifications
	fRes, err := appCtx.API.Account().AccountFetch(account.NewAccountFetchParams().WithID(accountID))
	if err != nil {
		return err
	}
	accountObj := fRes.Payload
	uParams.Version = swag.Int32(int32(accountObj.Meta.Version))
	if c.SmpProcessInheritableFlags(accountObj.SnapshotManagementPolicy) {
		uParams.Payload.SnapshotManagementPolicy = accountObj.SnapshotManagementPolicy
		uParams.Set = append(uParams.Set, "snapshotManagementPolicy")
		nChg++
	}
	if c.SCPFlags.ScpProtectionDomain != "" && c.SCPFlags.ScpProtectionDomainID == "" {
		if pdID, err := c.validateProtectionDomainName(c.SCPFlags.ScpProtectionDomain); err == nil {
			c.SCPFlags.ScpProtectionDomainID = pdID
		}
	}
	if c.SCPFlags.ScpCspDomain != "" && c.SCPFlags.ScpCspDomainID == "" {
		if domID, err := c.validateDomainName(c.SCPFlags.ScpCspDomain); err == nil {
			c.SCPFlags.ScpCspDomainID = domID
		}
	}
	if c.ScpProcessInheritableFlags(accountObj.SnapshotCatalogPolicy) {
		uParams.Payload.SnapshotCatalogPolicy = accountObj.SnapshotCatalogPolicy
		uParams.Set = append(uParams.Set, "snapshotCatalogPolicy")
		nChg++
	}
	if c.VsrpProcessInheritableFlags(accountObj.VsrManagementPolicy) {
		uParams.Payload.VsrManagementPolicy = accountObj.VsrManagementPolicy
		uParams.Set = append(uParams.Set, "vsrManagementPolicy")
		nChg++
	}

	if len(c.UserRoles) > 0 || c.UserRolesAction == "SET" {
		userRoles, err := c.parseUserRoles(c.UserRoles)
		if err != nil {
			return err
		}
		uParams.Payload.UserRoles = userRoles
		switch c.UserRolesAction {
		case "APPEND":
			uParams.Append = append(uParams.Append, "userRoles")
		case "SET":
			uParams.Set = append(uParams.Set, "userRoles")
		case "REMOVE":
			uParams.Remove = append(uParams.Remove, "userRoles")
		}
		nChg++
	}
	if nChg == 0 {
		return fmt.Errorf("No modifications specified")
	}
	uParams.ID = accountID
	if c.Version != 0 {
		uParams.Version = &c.Version
	}
	var uRes *account.AccountUpdateOK
	if uRes, err = appCtx.API.Account().AccountUpdate(uParams); err != nil {
		if e, ok := err.(*account.AccountUpdateDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.Account{uRes.Payload})
}

type accountResetSecretCmd struct {
	Name               string `short:"n" long:"name" description:"An account name. If not specified, the context account itself is used"`
	AccountSecretScope string `short:"S" long:"account-secret-scope" description:"Set the scope for account secrets in a local cluster usage policy" choice:"CLUSTER" choice:"CSPDOMAIN" choice:"GLOBAL" required:"yes"`
	Recursive          bool   `short:"r" long:"recursive" description:"Apply the operation recursively within the specified scope"`
	DomainName         string `short:"D" long:"domain-name" description:"Name of a cloud service provider domain"`
	ClusterName        string `short:"C" long:"cluster-name" description:"Name of a cluster"`

	accountCmd
	remainingArgsCatcher
}

func (c *accountResetSecretCmd) Execute(args []string) error {
	var err error
	if err = c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	accountID := ""
	if c.Name == "" {
		if appCtx.AccountID == "" {
			return fmt.Errorf("Neither of the --account or --name flags were specified")
		}
		accountID = appCtx.AccountID
	} else if accountID, err = c.validateAccount(c.Name, ""); err != nil {
		return err
	}
	var domainID, clusterID string
	if c.AccountSecretScope != common.AccountSecretScopeGlobal {
		domainID, clusterID, _, err = c.validateDomainClusterNodeNames(c.DomainName, c.ClusterName, "")
		if err != nil {
			return err
		}
	}
	params := account.NewAccountSecretResetParams()
	params.ID = accountID
	params.AccountSecretScope = c.AccountSecretScope
	if c.Recursive {
		params.Recursive = swag.Bool(c.Recursive)
	}
	if c.AccountSecretScope == common.AccountSecretScopeCspDomain {
		if domainID == "" {
			return fmt.Errorf("missing domain-name")
		}
		params.CspDomainID = swag.String(domainID)
	}
	if c.AccountSecretScope == common.AccountSecretScopeCluster {
		if clusterID == "" {
			return fmt.Errorf("missing cluster-name")
		}
		params.ClusterID = swag.String(clusterID)
	}
	if _, err = appCtx.API.Account().AccountSecretReset(params); err != nil {
		if e, ok := err.(*account.AccountSecretResetDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return nil
}
