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
	"sort"
	"strings"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_credential"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
)

func init() {
	initCspDomain()
}

func initCspDomain() {
	cmd, _ := parser.AddCommand("csp-domain", "CSPDomain object commands", "CSPDomain object subcommands", &cspDomainCmd{})
	cmd.Aliases = []string{"csp-domains", "domain", "dom"}
	cmd.AddCommand("get", "Get a CSPDomain", "Get a CSPDomain object.", &cspDomainFetchCmd{})
	cmd.AddCommand("list", "List CSPDomains", "List or search for CSPDomain objects", &cspDomainListCmd{})
	cmd.AddCommand("create", "Create a CSPDomain", "Create a CSPDomain object", &cspDomainCreateCmd{})
	cmd.AddCommand("delete", "Delete a CSPDomain", "Delete a CSPDomain object", &cspDomainDeleteCmd{})
	cmd.AddCommand("modify", "Modify a CSPDomain", "Modify a CSPDomain object. Use a whitespace-only string to clear trimmed properties", &cspDomainModifyCmd{})
	cmd.AddCommand("get-deployment", "Managed cluster deployment", "Download the deployment configuration to a manage a cluster in this domain", &cspDomainDeployFetchCmd{})
	c, _ := cmd.AddCommand("metadata", "Show CSPDomain metadata", "Show metadata on a CSPDomain type", &cspDomainMetadataCmd{})
	c.Aliases = []string{"md"}
	c, _ = cmd.AddCommand("service-plan-cost", "Compute the cost of a service plan", "Compute the cost of a service plan based on storage costs in the domain", &cspDomainServicePlanCostCmd{})
	c.Aliases = []string{"sp-cost"}
	cmd.AddCommand("show-columns", "Show CSPDomain table columns", "Show names of columns used in table format", &showColsCmd{columns: cspDomainHeaders})
}

type cspDomainCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string

	cacheHelper
}

// cspDomain record keys/headers and their description
var cspDomainHeaders = map[string]string{
	hID:                  dID,
	hTimeCreated:         dTimeCreated,
	hTimeModified:        dTimeModified,
	hVersion:             dVersion,
	hName:                dName,
	hDescription:         dDescription,
	hAccount:             dAccount,
	hAuthorizedAccounts:  dAuthorizedAccounts,
	hTags:                dTags,
	hCspDomainType:       "The type of Cloud Service Provider.",
	hManagementHost:      "Hostname or IPv4 address of the Nuvoloso management service.",
	hCspDomainAttributes: "Attributes describing the Cloud Service Provider domain.",
	hCspCredential:       "The CspCredential object name.",
}

var cspDomainDefaultHeaders = []string{hName, hManagementHost, hAuthorizedAccounts}

// makeRecord creates a map of properties
func (c *cspDomainCmd) makeRecord(o *models.CSPDomain) map[string]string {
	attrs := make([]string, 0, len(o.CspDomainAttributes))
	for a := range o.CspDomainAttributes {
		attrs = append(attrs, a)
	}
	account := string(o.AccountID)
	if a, ok := c.accounts[account]; ok {
		account = a.name
	}
	authorized := make([]string, 0, len(o.AuthorizedAccounts))
	for _, aid := range o.AuthorizedAccounts {
		aVal := string(aid)
		if a, ok := c.accounts[aVal]; ok {
			aVal = a.name
		}
		authorized = append(authorized, aVal)
	}
	var credName string
	if cred, ok := c.cspCredentials[string(o.CspCredentialID)]; ok {
		credName = cred
	} else {
		credName = string(o.CspCredentialID)
	}
	sort.Strings(attrs)
	attrList := []string{}
	for _, a := range attrs {
		v := o.CspDomainAttributes[a]
		kind := v.Kind[0:1]
		value := v.Value
		if v.Kind == "SECRET" {
			kind = "E"
			value = "***"
		}
		s := a + "[" + kind + "]: " + value
		attrList = append(attrList, s)
	}
	return map[string]string{
		hID:                  string(o.Meta.ID),
		hTimeCreated:         time.Time(o.Meta.TimeCreated).Format(time.RFC3339),
		hTimeModified:        time.Time(o.Meta.TimeModified).Format(time.RFC3339),
		hVersion:             fmt.Sprintf("%d", o.Meta.Version),
		hName:                string(o.Name),
		hDescription:         string(o.Description),
		hAccount:             account,
		hAuthorizedAccounts:  strings.Join(authorized, ", "),
		hCspDomainType:       string(o.CspDomainType),
		hManagementHost:      o.ManagementHost,
		hCspDomainAttributes: strings.Join(attrList, "\n"),
		hCspCredential:       credName,
		hTags:                strings.Join(o.Tags, ", "),
	}
}

func (c *cspDomainCmd) validateColumns(columns string) (err error) {
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(cspDomainHeaders), cspDomainDefaultHeaders)
	return err
}

func (c *cspDomainCmd) Emit(data []*models.CSPDomain) error {
	// TBD maybe need to handle Kind: SECRET in attributes here
	switch c.OutputFormat {
	case "json":
		return appCtx.EmitJSON(data)
	case "yaml":
		return appCtx.EmitYAML(data)
	}
	c.cacheAccounts()
	c.cacheCspCredentials("", "")
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

func (c *cspDomainCmd) list(params *csp_domain.CspDomainListParams) ([]*models.CSPDomain, error) {
	if params == nil {
		params = csp_domain.NewCspDomainListParams()
	}
	res, err := appCtx.API.CspDomain().CspDomainList(params)
	if err != nil {
		if e, ok := err.(*csp_domain.CspDomainListDefault); ok && e.Payload.Message != nil {
			return nil, fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res.Payload, nil
}

func (c *cspDomainCreateCmd) lookupCspCredentialID(name, domType string) (models.ObjID, error) {
	lParams := csp_credential.NewCspCredentialListParams()
	lParams.Name = swag.String(name)
	lParams.CspDomainType = swag.String(domType)
	credCmd := &cspCredentialCmd{}
	ret, err := credCmd.list(lParams)
	if err == nil {
		if len(ret) == 1 {
			c.cacheCspCredentials(string(ret[0].Meta.ID), name)
			return ret[0].Meta.ID, nil
		}
	}
	return "", err
}

type cspDomainCreateCmd struct {
	Name              string             `short:"n" long:"name" description:"The name of the cloud service provider domain" required:"yes"`
	Description       string             `short:"d" long:"description" description:"The purpose of this cloud service provider domain"`
	Tags              []string           `short:"t" long:"tag" description:"A tag value; repeat as needed"`
	Columns           string             `short:"c" long:"columns" description:"Comma separated list of column names"`
	DomainType        string             `short:"T" long:"domain-type" description:"Type of cloud service provider" choice:"AWS" choice:"Azure" choice:"GCP" default:"AWS"`
	ManagementHost    string             `short:"H" long:"management-host" description:"Name or address of the Nuvoloso management host usable from within the CSP domain"`
	Attrs             map[string]string  `short:"a" long:"attribute" description:"An attribute name:value pair. Repeat as necessary" required:"yes"`
	CspCredentialName string             `long:"cred-name" description:"The CspCredential name"`
	CspCredentialID   string             `long:"cred-id" description:"The CspCredential ID. Alternative to CspCredentialName"`
	StorageCosts      map[string]float64 `long:"cost" description:"Specify the cost of storage in the form 'Storage Type:Number'; repeat as necessary"`

	cspDomainCmd
	remainingArgsCatcher
}

func (c *cspDomainCreateCmd) Execute(args []string) error {
	if err := c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := appCtx.InitContextAccount(); err != nil {
		return err
	}
	// TBD: Optional initially - will be required after transition
	// if c.CspCredentialName == "" && c.CspCredentialID == "" {
	// 	return fmt.Errorf("CSPDomain requires CspCredential name or CspCredential ID")
	// }
	params := csp_domain.NewCspDomainCreateParams()
	params.Payload = &models.CSPDomain{}
	params.Payload.AccountID = models.ObjIDMutable(appCtx.AccountID)
	params.Payload.Name = models.ObjName(c.Name)
	params.Payload.Description = models.ObjDescription(c.Description)
	params.Payload.Tags = c.Tags
	params.Payload.CspDomainType = models.CspDomainTypeMutable(c.DomainType)
	params.Payload.ManagementHost = c.ManagementHost
	params.Payload.CspDomainAttributes = convertAttrs(c.Attrs)
	if c.CspCredentialID != "" {
		params.Payload.CspCredentialID = models.ObjIDMutable(c.CspCredentialID)
	} else if c.CspCredentialName != "" {
		cspCredentialID, err := c.lookupCspCredentialID(c.CspCredentialName, c.DomainType)
		if err != nil {
			return err
		}
		params.Payload.CspCredentialID = models.ObjIDMutable(cspCredentialID)
	}
	if len(c.StorageCosts) > 0 {
		costs := map[string]models.StorageCost{}
		for st, c := range c.StorageCosts {
			costs[st] = models.StorageCost{CostPerGiB: c}
		}
		params.Payload.StorageCosts = costs
	}
	res, err := appCtx.API.CspDomain().CspDomainCreate(params)
	if err != nil {
		if e, ok := err.(*csp_domain.CspDomainCreateDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.CSPDomain{res.Payload})
}

type cspDomainListCmd struct {
	Name              string   `short:"n" long:"name" description:"A CSPDomain object name"`
	AuthorizedAccount string   `short:"Z" long:"authorized-account" description:"Name of an authorized subordinate account"`
	OwnerAuthorized   bool     `long:"owner-auth" description:"Match a CSPDomain where the owner (the context account) is also the authorized account"`
	Tags              []string `short:"t" long:"tag" description:"A tag value; repeat as needed"`
	DomainType        string   `short:"T" long:"domain-type" description:"Type of cloud service provider"`
	Columns           string   `short:"c" long:"columns" description:"Comma separated list of column names"`
	Follow            bool     `short:"f" long:"follow-changes" description:"Monitor the system and repeat the command as relevant changes occur."`
	FollowNoClear     bool     `long:"follow-no-clear" description:"Do not clear the terminal (the default) when using follow-changes"`

	mustClearTerm bool
	params        *csp_domain.CspDomainListParams
	runCount      int
	lastLen       int

	cspDomainCmd
	remainingArgsCatcher
}

func (c *cspDomainListCmd) Execute(args []string) error {
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
	if c.AuthorizedAccount != "" && c.OwnerAuthorized {
		return fmt.Errorf("do not specify --authorized-account and --owner-auth together")
	}
	if c.OwnerAuthorized && appCtx.Account == "" {
		return fmt.Errorf("--owner-auth requires --account")
	}
	params := csp_domain.NewCspDomainListParams()
	params.Name = &c.Name
	if c.AuthorizedAccount != "" {
		id, err := c.validateAccount(c.AuthorizedAccount, "authorized")
		if err != nil {
			return err
		}
		params.AuthorizedAccountID = &id
	} else if c.OwnerAuthorized {
		params.AuthorizedAccountID = &appCtx.AccountID
	}
	params.Tags = c.Tags
	params.CspDomainType = &c.DomainType
	c.params = params
	if c.Follow {
		if !c.FollowNoClear {
			c.mustClearTerm = true
		}
		return appCtx.WatchForChange(c)
	}
	return c.run()
}

func (c *cspDomainListCmd) run() error {
	var err error
	var res []*models.CSPDomain
	if res, err = c.list(c.params); err != nil {
		return err
	}
	if c.mustClearTerm {
		appCtx.ClearTerminal()
	}
	c.lastLen = len(res)
	return c.Emit(res)
}

// ChangeDetected is part of the ChangeDetector interface
func (c *cspDomainListCmd) ChangeDetected() error {
	c.runCount++
	return c.run()
}

// WatcherArgs is part of the ChangeDetector interface
func (c *cspDomainListCmd) WatcherArgs() *models.CrudWatcherCreateArgs {
	cm := &models.CrudMatcher{
		URIPattern: "/csp-domains/?",
	}
	// there are no scope properties currently defined
	return &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{cm},
	}
}

type cspDomainFetchCmd struct {
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`

	cspDomainCmd
	requiredIDRemainingArgsCatcher
}

func (c *cspDomainFetchCmd) Execute(args []string) error {
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
	fParams := csp_domain.NewCspDomainFetchParams().WithID(c.ID)
	res, err := appCtx.API.CspDomain().CspDomainFetch(fParams)
	if err != nil {
		if e, ok := err.(*csp_domain.CspDomainFetchDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.CSPDomain{res.Payload})
}

type cspDomainDeleteCmd struct {
	Name    string `short:"n" long:"name" description:"A CSPDomain name" required:"yes"`
	Confirm bool   `long:"confirm" description:"Confirm the deletion of the object"`

	cspDomainCmd
	remainingArgsCatcher
}

func (c *cspDomainDeleteCmd) Execute(args []string) error {
	if err := c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if !c.Confirm {
		return fmt.Errorf("specify --confirm to delete the \"%s\" CSPDomain object", c.Name)
	}
	if err := appCtx.InitContextAccount(); err != nil {
		return err
	}
	lParams := csp_domain.NewCspDomainListParams()
	lParams.Name = &c.Name
	var lRes []*models.CSPDomain
	var err error
	if lRes, err = c.list(lParams); err != nil {
		return err
	}
	if len(lRes) != 1 {
		return fmt.Errorf("CSPDomain object \"%s\" not found", c.Name)
	}
	dParams := csp_domain.NewCspDomainDeleteParams()
	dParams.ID = string(lRes[0].Meta.ID)
	if _, err = appCtx.API.CspDomain().CspDomainDelete(dParams); err != nil {
		if e, ok := err.(*csp_domain.CspDomainDeleteDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return nil
}

type cspDomainModifyCmd struct {
	Name                     string             `short:"n" long:"name" description:"The name of the CSPDomain object to be modified" required:"yes"`
	NewName                  string             `short:"N" long:"new-name" description:"The new name for the object"`
	ManagementHost           string             `short:"H" long:"management-host" description:"The new management host value; leading and trailing whitespace will be stripped"`
	Description              string             `short:"d" long:"description" description:"The new description; leading and trailing whitespace will be stripped"`
	AuthorizedAccounts       []string           `short:"Z" long:"authorized-account" description:"Name of an authorized account. Repeat as needed. Subject to the value of accounts-action"`
	AuthorizedAccountsAction string             `long:"authorized-accounts-action" description:"Specifies how to process authorized-account values" choice:"APPEND" choice:"REMOVE" choice:"SET" default:"APPEND"`
	OwnerAuthorized          bool               `long:"owner-auth" description:"Authorize the owner (the context account). Subject to the value of authorized-accounts-action"`
	StorageCosts             map[string]float64 `long:"cost" description:"Specify the cost of storage in the form 'Storage Type:Number'; repeat as necessary, subject to the value of cost-action"`
	StorageCostsAction       string             `long:"cost-action" description:"Specifies how to process cost values" choice:"APPEND" choice:"REMOVE" choice:"SET" default:"APPEND"`
	CUPFlagsWithInheritance
	Tags            []string `short:"t" long:"tag" description:"A tag value; repeat as needed, subject to the value of tag-action"`
	TagsAction      string   `long:"tag-action" description:"Specifies how to process tag values" choice:"APPEND" choice:"REMOVE" choice:"SET" default:"APPEND"`
	CspCredentialID string   `long:"cred-id" description:"The new CspCredential ID"`
	Version         int32    `short:"V" long:"version" description:"Enforce update of the specified version of the object"`
	Columns         string   `short:"c" long:"columns" description:"Comma separated list of column names"`
	cspDomainCmd
	remainingArgsCatcher
}

func (c *cspDomainModifyCmd) Execute(args []string) error {
	var err error
	if err := c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	if c.OwnerAuthorized && appCtx.Account == "" {
		return fmt.Errorf("--owner-auth requires --account")
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	if err = c.CupValidateInheritableFlags(); err != nil {
		return err
	}
	lParams := csp_domain.NewCspDomainListParams()
	lParams.Name = &c.Name
	var lRes []*models.CSPDomain
	if lRes, err = c.list(lParams); err != nil {
		return err
	}
	if len(lRes) != 1 {
		return fmt.Errorf("CSPDomain object \"%s\" not found", c.Name)
	}
	nChg := 0
	uParams := csp_domain.NewCspDomainUpdateParams()
	uParams.Payload = &models.CSPDomainMutable{}
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
	ts = strings.TrimSpace(c.ManagementHost)
	if c.ManagementHost != "" {
		uParams.Payload.ManagementHost = ts
		uParams.Set = append(uParams.Set, "managementHost")
		nChg++
	}
	if len(c.AuthorizedAccounts) != 0 || c.OwnerAuthorized || c.AuthorizedAccountsAction == "SET" {
		if c.OwnerAuthorized {
			uParams.Payload.AuthorizedAccounts = append(uParams.Payload.AuthorizedAccounts, models.ObjIDMutable(appCtx.AccountID))
		}
		for _, account := range c.AuthorizedAccounts {
			id, err := c.validateAccount(account, "authorized")
			if err != nil {
				return err
			}
			uParams.Payload.AuthorizedAccounts = append(uParams.Payload.AuthorizedAccounts, models.ObjIDMutable(id))
		}
		switch c.AuthorizedAccountsAction {
		case "APPEND":
			uParams.Append = append(uParams.Append, "authorizedAccounts")
		case "SET":
			uParams.Set = append(uParams.Set, "authorizedAccounts")
		case "REMOVE":
			uParams.Remove = append(uParams.Remove, "authorizedAccounts")
		}
		nChg++
	}
	if len(c.StorageCosts) != 0 || c.StorageCostsAction == "SET" {
		costs := map[string]models.StorageCost{}
		for st, c := range c.StorageCosts {
			costs[st] = models.StorageCost{CostPerGiB: c}
		}
		uParams.Payload.StorageCosts = costs
		switch c.StorageCostsAction {
		case "APPEND":
			uParams.Append = append(uParams.Append, "storageCosts")
		case "SET":
			uParams.Set = append(uParams.Set, "storageCosts")
		case "REMOVE":
			uParams.Remove = append(uParams.Remove, "storageCosts")
		}
		nChg++
	}
	if len(c.Tags) != 0 {
		uParams.Payload.Tags = c.Tags
		switch c.TagsAction {
		case "APPEND":
			uParams.Append = append(uParams.Append, "tags")
		case "SET":
			uParams.Set = append(uParams.Set, "tags")
		case "REMOVE":
			uParams.Remove = append(uParams.Remove, "tags")
		}
		nChg++
	} else if c.TagsAction == "SET" {
		uParams.Set = append(uParams.Set, "tags")
	}
	if c.CupProcessInheritableFlags(lRes[0].ClusterUsagePolicy) {
		nChg++
		uParams.Set = append(uParams.Set, "clusterUsagePolicy")
		uParams.Payload.ClusterUsagePolicy = lRes[0].ClusterUsagePolicy
	}
	ts = strings.TrimSpace(c.CspCredentialID)
	if c.CspCredentialID != "" {
		uParams.Payload.CspCredentialID = models.ObjIDMutable(c.CspCredentialID)
		uParams.Set = append(uParams.Set, "cspCredentialId")
		nChg++
	}
	if nChg == 0 {
		return fmt.Errorf("No modifications specified")
	}
	uParams.ID = string(lRes[0].Meta.ID)
	if c.Version != 0 {
		uParams.Version = &c.Version
	}
	var res *csp_domain.CspDomainUpdateOK
	if res, err = appCtx.API.CspDomain().CspDomainUpdate(uParams); err != nil {
		if e, ok := err.(*csp_domain.CspDomainUpdateDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.CSPDomain{res.Payload})
}

type cspDomainDeployFetchCmd struct {
	Name        string `short:"n" long:"name" description:"A CSPDomain name" required:"yes"`
	OutputFile  string `short:"O" long:"output-file" description:"The name of the output file. Use - for stdout" default:"nuvo-cluster.yaml"`
	ClusterType string `long:"cluster-type" description:"Type of cluster" default:"kubernetes"`
	ClusterName string `long:"cluster-name" description:"Name for the new cluster"`
	Force       bool   `long:"force" description:"Obtains a copy of a previously issued deployment configuration for an existing cluster object."`
	cspDomainCmd
	remainingArgsCatcher
}

func (c *cspDomainDeployFetchCmd) Execute(args []string) error {
	var err error
	if err := c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	var oF *os.File
	if c.OutputFile == "-" {
		oF = os.Stdout
	} else {
		oF, err = os.Create(c.OutputFile)
		if err != nil {
			return err
		}
		defer oF.Close()
	}
	lParams := csp_domain.NewCspDomainListParams()
	lParams.Name = &c.Name
	var lRes []*models.CSPDomain
	if lRes, err = c.list(lParams); err != nil {
		return err
	}
	if len(lRes) != 1 {
		return fmt.Errorf("CSPDomain object \"%s\" not found", c.Name)
	}
	var ret *csp_domain.CspDomainDeploymentFetchOK
	dfParams := csp_domain.NewCspDomainDeploymentFetchParams()
	dfParams.ID = string(lRes[0].Meta.ID)
	dfParams.ClusterType = &c.ClusterType
	if c.ClusterName != "" {
		dfParams.Name = &c.ClusterName
	}
	dfParams.Force = &c.Force
	if ret, err = appCtx.API.CspDomain().CspDomainDeploymentFetch(dfParams); err != nil {
		if e, ok := err.(*csp_domain.CspDomainDeploymentFetchDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	oF.WriteString(ret.Payload.Deployment)
	return nil
}

type cspDomainMetadataCmd struct {
	DomainType string `short:"T" long:"domain-type" description:"Type of cloud service provider" choice:"AWS" choice:"Azure" choice:"GCP" default:"AWS"`
	cspDomainCmd
	remainingArgsCatcher
}

var cspDomainMetadataCols = []string{"AttributeName", "Kind", "Description", "Required"}

func (c *cspDomainMetadataCmd) Execute(args []string) error {
	var err error
	if err := c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	params := csp_domain.NewCspDomainMetadataParams()
	params.CspDomainType = c.DomainType
	res, err := appCtx.API.CspDomain().CspDomainMetadata(params)
	if err != nil {
		if e, ok := err.(*csp_domain.CspDomainMetadataDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	switch c.OutputFormat {
	case "json":
		return appCtx.EmitJSON(res.Payload)
	case "yaml":
		return appCtx.EmitYAML(res.Payload)
	}
	names := make([]string, 0, len(res.Payload.AttributeMetadata))
	for n := range res.Payload.AttributeMetadata {
		names = append(names, n)
	}
	sort.Strings(names)
	rows := make([][]string, 0, len(names))
	for _, n := range names {
		d := res.Payload.AttributeMetadata[n]
		row := []string{
			n,
			d.Kind,
			d.Description,
			fmt.Sprintf("%v", !d.Optional),
		}
		rows = append(rows, row)
	}
	return appCtx.EmitTable(cspDomainMetadataCols, rows, nil)
}

type cspDomainServicePlanCostCmd struct {
	Name        string `short:"n" long:"name" description:"A CSPDomain name" required:"yes"`
	ServicePlan string `short:"P" long:"service-plan" description:"The name of a service plan." required:"yes"`

	cspDomainCmd
	remainingArgsCatcher
}

func (c *cspDomainServicePlanCostCmd) Execute(args []string) error {
	var err error
	if err := c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	lParams := csp_domain.NewCspDomainListParams()
	lParams.Name = &c.Name
	var lRes []*models.CSPDomain
	if lRes, err = c.list(lParams); err != nil {
		return err
	}
	if len(lRes) != 1 {
		return fmt.Errorf("CSPDomain object \"%s\" not found", c.Name)
	}
	servicePlanID := ""
	if err = c.cacheServicePlans(); err != nil {
		return err
	}
	for id, n := range c.servicePlans {
		if n == c.ServicePlan {
			servicePlanID = id
		}
	}
	if servicePlanID == "" {
		return fmt.Errorf("service plan '%s' not found", c.ServicePlan)
	}
	var ret *csp_domain.CspDomainServicePlanCostOK
	params := csp_domain.NewCspDomainServicePlanCostParams()
	params.ID = string(lRes[0].Meta.ID)
	params.ServicePlanID = servicePlanID
	if ret, err = appCtx.API.CspDomain().CspDomainServicePlanCost(params); err != nil {
		if e, ok := err.(*csp_domain.CspDomainServicePlanCostDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit(ret.Payload)
}

var cspDomainServicePlanCostHeaders = []string{"Cost/GiB", "Cost (no cache)/GiB"}

// Emit is overridden for this command
func (c *cspDomainServicePlanCostCmd) Emit(cost *models.ServicePlanCost) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.EmitJSON(cost)
	case "yaml":
		return appCtx.EmitYAML(cost)
	}
	rows := [][]string{
		{fmt.Sprintf("%g", cost.CostPerGiB), fmt.Sprintf("%g", cost.CostPerGiBWithoutCache)},
	}
	return appCtx.EmitTable(cspDomainServicePlanCostHeaders, rows, nil)
}
