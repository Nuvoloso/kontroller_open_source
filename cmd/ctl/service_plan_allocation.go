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
	"os"
	"strings"

	spa "github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan_allocation"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
)

func init() {
	initServicePlanAllocation()
}

func initServicePlanAllocation() {
	cmd, _ := parser.AddCommand("service-plan-allocation", "ServicePlanAllocation object commands", "ServicePlanAllocation object subcommands", &servicePlanAllocationCmd{})
	cmd.Aliases = []string{"service-plan-allocations", "spa", "spas"}
	cmd.AddCommand("get", "Get a ServicePlanAllocation", "Get a ServicePlanAllocation object.", &servicePlanAllocationFetchCmd{})
	cmd.AddCommand("list", "List ServicePlanAllocations", "List or search for ServicePlanAllocation objects.", &servicePlanAllocationListCmd{})
	cmd.AddCommand("modify", "Modify a ServicePlanAllocation", "Modify a ServicePlanAllocation object.", &servicePlanAllocationModifyCmd{})
	cpCmd, _ := cmd.AddCommand("customize-provisioning", "Customize provisioning from a ServicePlanAllocation", "Customize dynamic provisioning from a ServicePlanAllocation object.", &servicePlanAllocationCustomizeProvisioningCmd{})
	cpCmd.Aliases = []string{"customize"}
	cmd.AddCommand("show-columns", "Show ServicePlanAllocation table columns", "Show names of columns used in table format.", &showColsCmd{columns: servicePlanAllocationHeaders})
}

type servicePlanAllocationCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string
	cacheHelper
}

var servicePlanAllocationHeaders = map[string]string{
	hID:                      dID,
	hCluster:                 dCluster,
	hCspDomain:               dCspDomain,
	hStorageFormula:          "The Name of the storage formula.",
	hAccount:                 dAccount,
	hAuthorizedAccount:       "The account that is authorized to use the object.",
	hTotalCapacityBytes:      "The maximum number of bytes of storage that may allocated.",
	hReservableCapacityBytes: "The number of bytes of storage that can be reserved.",
	hServicePlan:             "The name of the service plan.",
	hState:                   "The reservation state reflects the ability to reserve additional capacity.",
	hTags:                    dTags,
}

var servicePlanAllocationDefaultHeaders = []string{hCspDomain, hCluster, hAuthorizedAccount, hState, hServicePlan, hReservableCapacityBytes, hTotalCapacityBytes}

// makeRecord creates a map of properties
func (c *servicePlanAllocationCmd) makeRecord(o *models.ServicePlanAllocation) map[string]string {
	var account, authAccount, cluster, domain, servicePlan string
	domain = string(o.CspDomainID)
	if dn, ok := c.cspDomains[string(o.CspDomainID)]; ok { //assumes map present
		domain = dn
	}
	if aac, ok := c.accounts[string(o.AuthorizedAccountID)]; ok {
		authAccount = aac.name
	} else {
		authAccount = string(o.AuthorizedAccountID)
	}
	if ac, ok := c.accounts[string(o.AccountID)]; ok {
		account = ac.name
	} else {
		account = string(o.AccountID)
	}
	if cl, ok := c.clusters[string(o.ClusterID)]; ok {
		cluster = cl.name
	} else {
		cluster = string(o.ClusterID)
	}
	if sp, ok := c.servicePlans[string(o.ServicePlanID)]; ok {
		servicePlan = sp
	} else {
		servicePlan = string(o.ServicePlanID)
	}
	return map[string]string{
		hID:                      string(o.Meta.ID),
		hCluster:                 cluster,
		hCspDomain:               domain,
		hAccount:                 account,
		hStorageFormula:          string(o.StorageFormula),
		hAuthorizedAccount:       authAccount,
		hTotalCapacityBytes:      sizeToString(*o.TotalCapacityBytes),
		hReservableCapacityBytes: sizeToString(*o.ReservableCapacityBytes),
		hServicePlan:             servicePlan,
		hState:                   o.ReservationState,
		hTags:                    strings.Join(o.Tags, ", "),
	}
}

func (c *servicePlanAllocationCmd) validateColumns(columns string) error {
	var err error
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(servicePlanAllocationHeaders), servicePlanAllocationDefaultHeaders)
	return err
}

func (c *servicePlanAllocationCmd) Emit(data []*models.ServicePlanAllocation) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.EmitJSON(data)
	case "yaml":
		return appCtx.EmitYAML(data)
	}
	c.cacheAccounts()
	c.cacheServicePlans()
	c.cacheClusters()
	c.cacheCSPDomains()
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

func (c *servicePlanAllocationCmd) list(params *spa.ServicePlanAllocationListParams) ([]*models.ServicePlanAllocation, error) {
	res, err := appCtx.API.ServicePlanAllocation().ServicePlanAllocationList(params)
	if err != nil {
		if e, ok := err.(*spa.ServicePlanAllocationListDefault); ok && e.Payload.Message != nil {
			return nil, fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res.Payload, nil
}

type servicePlanAllocationFetchCmd struct {
	DescriptorKey string `short:"K" long:"descriptor-key" description:"Emit the value of the named clusterDescriptor key to the output-file"`
	OutputFile    string `short:"O" long:"output-file" description:"The name of the output file. Use - for stdout" default:"spa-descriptor"`
	Columns       string `short:"c" long:"columns" description:"Comma separated list of column names"`

	servicePlanAllocationCmd
	requiredIDRemainingArgsCatcher
}

func (c *servicePlanAllocationFetchCmd) Execute(args []string) error {
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
	var oF *os.File
	if c.DescriptorKey != "" {
		if c.OutputFile == "-" {
			oF = os.Stdout
		} else {
			oF, err = os.Create(c.OutputFile)
			if err != nil {
				return err
			}
			defer oF.Close()
		}
	}
	fParams := spa.NewServicePlanAllocationFetchParams()
	fParams.ID = string(c.ID)
	res, err := appCtx.API.ServicePlanAllocation().ServicePlanAllocationFetch(fParams)
	if err != nil {
		if e, ok := err.(*spa.ServicePlanAllocationFetchDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	if c.DescriptorKey != "" {
		kv, found := res.Payload.ClusterDescriptor[c.DescriptorKey]
		if !found {
			return fmt.Errorf("clusterDescriptor key '%s' not found", c.DescriptorKey)
		}
		oF.WriteString(kv.Value)
		return nil
	}
	return c.Emit([]*models.ServicePlanAllocation{res.Payload})
}

type servicePlanAllocationListCmd struct {
	AuthorizedAccountName string   `short:"Z" long:"auth-account-name" description:"Name of the account authorized to use the SPA"`
	OwnerAuthorized       bool     `long:"owner-auth" description:"Match a SPA where the owner (the context account) is also the authorized account"`
	DomainName            string   `short:"D" long:"domain" description:"Name of a cloud service provider domain"`
	ClusterName           string   `short:"C" long:"cluster-name" description:"Name of a cluster in a specified domain"`
	ServicePlanName       string   `short:"P" long:"service-plan" description:"Name of the service plan associated with the SPA"`
	States                []string `long:"state" description:"The reservation state"`
	Tags                  []string `short:"t" long:"tag" description:"A tag value; repeat as needed"`
	StorageFormula        string   `short:"F" long:"storage-formula-name" description:"Name of a storage formula"`
	PoolID                string   `long:"pool-id" description:"The ID of a Pool object"`
	Columns               string   `short:"c" long:"columns" description:"Comma separated list of column names"`
	Follow                bool     `short:"f" long:"follow-changes" description:"Monitor the system and repeat the command as relevant changes occur"`
	FollowNoClear         bool     `long:"follow-no-clear" description:"Do not clear the terminal (the default) when using follow-changes"`

	mustClearTerm bool
	params        *spa.ServicePlanAllocationListParams
	runCount      int
	lastLen       int
	servicePlanAllocationCmd
	remainingArgsCatcher
}

func (c *servicePlanAllocationListCmd) Execute(args []string) error {
	if err := c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if c.AuthorizedAccountName != "" && c.OwnerAuthorized {
		return fmt.Errorf("do not specify --auth-account-name and --owner-auth together")
	}
	if c.OwnerAuthorized && appCtx.Account == "" {
		return fmt.Errorf("--owner-auth requires --account")
	}
	if err := appCtx.InitContextAccount(); err != nil {
		return err
	}
	domainID, clusterID, _, err := c.validateDomainClusterNodeNames(c.DomainName, c.ClusterName, "")
	if err != nil {
		return err
	}
	params := spa.NewServicePlanAllocationListParams()
	if domainID != "" {
		params.CspDomainID = &domainID
	}
	if clusterID != "" {
		params.ClusterID = &clusterID
	}
	for _, state := range c.States {
		if strings.HasPrefix(state, "!") {
			params.ReservationStateNot = append(params.ReservationStateNot, strings.TrimPrefix(state, "!"))
		} else {
			params.ReservationState = append(params.ReservationState, state)
		}
	}
	if c.StorageFormula != "" {
		params.StorageFormulaName = &c.StorageFormula
	}
	if c.AuthorizedAccountName != "" {
		authAccountID, err := c.validateAccount(c.AuthorizedAccountName, "authorized")
		if err != nil {
			return err
		}
		params.AuthorizedAccountID = &authAccountID
	} else if c.OwnerAuthorized {
		params.AuthorizedAccountID = &appCtx.AccountID
	}
	if c.ServicePlanName != "" {
		if err = c.cacheServicePlans(); err != nil {
			return err
		}
		for id, n := range c.servicePlans {
			if n == c.ServicePlanName {
				params.ServicePlanID = swag.String(id)
			}
		}
		if params.ServicePlanID == nil {
			return fmt.Errorf("service plan '%s' not found", c.ServicePlanName)
		}
	}
	if c.PoolID != "" {
		params.PoolID = &c.PoolID
	}
	params.Tags = c.Tags
	c.params = params
	if c.Follow {
		if !c.FollowNoClear {
			c.mustClearTerm = true
		}
		return appCtx.WatchForChange(c)
	}
	return c.run()
}

var servicePlanAllocationListCmdRunCacheThreshold = 1

func (c *servicePlanAllocationListCmd) run() error {
	var err error
	var res []*models.ServicePlanAllocation
	if res, err = c.list(c.params); err != nil {
		return err
	}
	if c.mustClearTerm {
		appCtx.ClearTerminal()
	}
	if c.runCount > servicePlanAllocationListCmdRunCacheThreshold && c.lastLen < len(res) {
		// refresh caches when new objects added
		c.cspDomains = nil
		c.clusters = nil
		c.accounts = nil
	}
	c.lastLen = len(res)
	return c.Emit(res)
}

// ChangeDetected is part of the ChangeDetector interface
func (c *servicePlanAllocationListCmd) ChangeDetected() error {
	c.runCount++
	return c.run()
}

// WatcherArgs is part of the ChangeDetector interface
func (c *servicePlanAllocationListCmd) WatcherArgs() *models.CrudWatcherCreateArgs {
	cm := &models.CrudMatcher{
		URIPattern: "/service-plan-allocations/?",
	}
	var scopeB bytes.Buffer
	if c.params.ClusterID != nil {
		fmt.Fprintf(&scopeB, ".*clusterID:%s", *c.params.ClusterID)
	}
	if c.params.CspDomainID != nil {
		fmt.Fprintf(&scopeB, ".*cspDomainID:%s", *c.params.CspDomainID)
	}
	if c.params.AuthorizedAccountID != nil {
		fmt.Fprintf(&scopeB, ".*authorizedAccountID:%s", *c.params.AuthorizedAccountID)
	}
	if c.params.ServicePlanID != nil {
		fmt.Fprintf(&scopeB, ".*servicePlanID:%s", *c.params.ServicePlanID)
	}
	if scopeB.Len() > 0 {
		cm.ScopePattern = scopeB.String()
	}
	return &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{cm},
	}
}

type servicePlanAllocationModifyCmd struct {
	AuthorizedAccountName string   `short:"Z" long:"authorized-account" description:"Name of the account authorized to use the SPA. If not specified, the context account is assumed"`
	DomainName            string   `short:"D" long:"domain" description:"Name of a cloud service provider domain" required:"yes"`
	ClusterName           string   `short:"C" long:"cluster-name" description:"Name of a cluster in a specified domain" required:"yes"`
	ServicePlanName       string   `short:"P" long:"service-plan-name" description:"Name of the service plan associated with the SPA" required:"yes"`
	DisableReservation    bool     `long:"disable-reservation" description:"Disable reservation in the SPA. Do not specify with enable-reservation"`
	EnableReservation     bool     `long:"enable-reservation" description:"Enable reservation in the SPA. Do not specify with disable-reservation"`
	Tags                  []string `short:"t" long:"tag" description:"A tag value; repeat as needed"`
	TagsAction            string   `long:"tag-action" description:"Specifies how to process tag values" choice:"APPEND" choice:"REMOVE" choice:"SET" default:"APPEND"`
	Version               int32    `short:"V" long:"version" description:"Enforce update of the specified version of the object"`
	Columns               string   `short:"c" long:"columns" description:"Comma separated list of column names"`
	servicePlanAllocationCmd
	remainingArgsCatcher
}

func (c *servicePlanAllocationModifyCmd) Execute(args []string) error {
	if err := c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if c.AuthorizedAccountName == "" && appCtx.Account == "" {
		return fmt.Errorf("Neither of the --account or --authorized-account flags were specified")
	}
	if c.DisableReservation && c.EnableReservation {
		return fmt.Errorf("do not specify 'enable-reservation' and 'disable-reservation' together")
	}
	if err := appCtx.InitContextAccount(); err != nil {
		return err
	}
	domainID, clusterID, _, err := c.validateDomainClusterNodeNames(c.DomainName, c.ClusterName, "")
	if err != nil {
		return err
	}
	authAccountID := appCtx.AccountID
	if c.AuthorizedAccountName != "" {
		if authAccountID, err = c.validateAccount(c.AuthorizedAccountName, "authorized"); err != nil {
			return err
		}
	}
	if err = c.cacheServicePlans(); err != nil {
		return err
	}
	var servicePlanID *string
	for id, n := range c.servicePlans {
		if n == c.ServicePlanName {
			servicePlanID = swag.String(id)
		}
	}
	if servicePlanID == nil {
		return fmt.Errorf("service plan '%s' not found", c.ServicePlanName)
	}
	lParams := spa.NewServicePlanAllocationListParams()
	lParams.CspDomainID = &domainID
	lParams.ClusterID = &clusterID
	lParams.AuthorizedAccountID = &authAccountID
	lParams.ServicePlanID = servicePlanID
	var lRes []*models.ServicePlanAllocation
	if lRes, err = c.list(lParams); err != nil {
		return err
	}
	if len(lRes) != 1 {
		return fmt.Errorf("ServicePlanAllocation object not found")
	}
	nChg := 0
	uParams := spa.NewServicePlanAllocationUpdateParams()
	uParams.ID = string(lRes[0].Meta.ID)
	uParams.Payload = &models.ServicePlanAllocationMutable{
		ServicePlanAllocationCreateMutable: models.ServicePlanAllocationCreateMutable{},
	}
	if c.EnableReservation || c.DisableReservation {
		changeState := true
		if c.EnableReservation {
			if lRes[0].ReservationState != common.SPAReservationStateDisabled {
				changeState = false // already in some enabled state, leave unchanged
			} else {
				uParams.Payload.ReservationState = common.SPAReservationStateUnknown
			}
		} else {
			uParams.Payload.ReservationState = common.SPAReservationStateDisabled
		}
		if changeState {
			uParams.Set = append(uParams.Set, "reservationState")
			nChg++
		}
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
	}
	if nChg == 0 {
		return fmt.Errorf("No modifications specified")
	}
	if c.Version != 0 {
		uParams.Version = &c.Version
	}
	var res *spa.ServicePlanAllocationUpdateOK
	if res, err = appCtx.API.ServicePlanAllocation().ServicePlanAllocationUpdate(uParams); err != nil {
		if e, ok := err.(*spa.ServicePlanAllocationUpdateDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.ServicePlanAllocation{res.Payload})

}

type servicePlanAllocationCustomizeProvisioningCmd struct {
	ApplicationGroupName        string   `long:"application-group-name" description:"Specify the application group name template"`
	ApplicationGroupDescription string   `long:"application-group-description" description:"Specify the application group description template"`
	ApplicationGroupTags        []string `long:"application-group-tag" description:"Specify a application group tag template. Repeat as necessary"`
	ConsistencyGroupName        string   `long:"consistency-group-name" description:"Specify the consistency group name template"`
	ConsistencyGroupDescription string   `long:"consistency-group-description" description:"Specify the consistency group description template"`
	ConsistencyGroupTags        []string `long:"consistency-group-tag" description:"Specify a consistency group tag template. Repeat as necessary"`
	VolumeSeriesTags            []string `long:"volume-series-tag" description:"Specify a volume series tag template. Repeat as necessary"`
	K8sName                     string   `long:"k8s-name" description:"The secret object name if a Kubernetes cluster"`
	K8sNamespace                string   `long:"k8s-namespace" description:"The secret object namespace if a Kubernetes cluster"`

	OutputFile string `short:"O" long:"output-file" description:"The name of the output file. Use - for stdout" default:"customized-secret.yaml"`

	servicePlanAllocationCmd
	requiredIDRemainingArgsCatcher
}

func (c *servicePlanAllocationCustomizeProvisioningCmd) Execute(args []string) error {
	var err error
	if err = c.verifyRequiredIDAndNoRemainingArgs(); err != nil {
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
	params := spa.NewServicePlanAllocationCustomizeProvisioningParams()
	params.ID = string(c.ID)
	if c.ApplicationGroupName != "" {
		params.ApplicationGroupName = swag.String(c.ApplicationGroupName)
	}
	if c.ApplicationGroupDescription != "" {
		params.ApplicationGroupDescription = swag.String(c.ApplicationGroupDescription)
	}
	if len(c.ApplicationGroupTags) > 0 {
		params.ApplicationGroupTag = c.ApplicationGroupTags
	}
	if c.ConsistencyGroupName != "" {
		params.ConsistencyGroupName = swag.String(c.ConsistencyGroupName)
	}
	if c.ConsistencyGroupDescription != "" {
		params.ConsistencyGroupDescription = swag.String(c.ConsistencyGroupDescription)
	}
	if len(c.ConsistencyGroupTags) > 0 {
		params.ConsistencyGroupTag = c.ConsistencyGroupTags
	}
	if len(c.VolumeSeriesTags) > 0 {
		params.VolumeSeriesTag = c.VolumeSeriesTags
	}
	if c.K8sName != "" {
		params.K8sName = swag.String(c.K8sName)
	}
	if c.K8sNamespace != "" {
		params.K8sNamespace = swag.String(c.K8sNamespace)
	}
	var ret *spa.ServicePlanAllocationCustomizeProvisioningOK
	if ret, err = appCtx.API.ServicePlanAllocation().ServicePlanAllocationCustomizeProvisioning(params); err != nil {
		if e, ok := err.(*spa.ServicePlanAllocationCustomizeProvisioningDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	oF.WriteString(ret.Payload.Value)
	return nil
}
