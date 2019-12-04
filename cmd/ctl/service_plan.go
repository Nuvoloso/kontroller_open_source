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
	"strconv"
	"strings"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
)

func init() {
	initServicePlan()
}

func initServicePlan() {
	cmd, _ := parser.AddCommand("service-plan", "ServicePlan object commands", "ServicePlan object subcommands", &servicePlanCmd{})
	cmd.Aliases = []string{"service-plans", "plan", "plans", "sp", "sps"}
	cmd.AddCommand("list", "List ServicePlans", "List or search for ServicePlan objects.", &servicePlanListCmd{})
	cmd.AddCommand("modify", "Modify a ServicePlan", "Modify a ServicePlan object.", &servicePlanModifyCmd{})
	cmd.AddCommand("show-columns", "Show ServicePlan table columns", "Show names of columns used in table format", &showColsCmd{columns: servicePlanHeaders})
}

type servicePlanCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string

	cacheHelper
}

// servicePlan record keys/headers and their description
var servicePlanHeaders = map[string]string{
	hID:                dID,
	hName:              dName,
	hDescription:       dDescription,
	hAccounts:          "The accounts authorized to use the service plan.",
	hSourceServicePlan: "The service plan from which the service plan was cloned.",
	hState:             "The state of the service plan.",
	hSLO:               "The service level objectives.",
	hIOProfile:         "The type of IO.",
	hProvisioningUnit:  "The performance/capacity ratio for a service plan",
	hVSMinSize:         "The minimum volume size for a service plan",
	hVSMaxSize:         "The maximum volume size for a service plan",
	hTags:              dTags,
}

var servicePlanDefaultHeaders = []string{hName, hIOProfile, hProvisioningUnit, hVSMinSize, hVSMaxSize}

// makeRecord creates a map of properties
func (c *servicePlanCmd) makeRecord(o *models.ServicePlan) map[string]string {
	accounts := make([]string, 0, len(o.Accounts))
	for _, a := range o.Accounts {
		var account string
		if ca, ok := c.accounts[string(a)]; ok { // assumes map present
			account = ca.name
		} else {
			account = string(a)
		}
		accounts = append(accounts, account)
	}
	var sourceSP string
	if cn, ok := c.servicePlans[string(o.SourceServicePlanID)]; ok { // assumes map present
		sourceSP = cn
	} else {
		sourceSP = string(o.SourceServicePlanID)
	}
	slos := make([]string, 0, len(o.Slos))
	for n, rvt := range o.Slos {
		slos = append(slos, fmt.Sprintf("%s:%s", n, rvt.Value))
	}
	iop := o.IoProfile
	catS := make([]string, 0, 2)
	catS = append(catS, fmt.Sprintf("%s(avgSz:%s-%s)",
		iop.IoPattern.Name, sizeToString(int64(*iop.IoPattern.MinSizeBytesAvg)), sizeToString(int64(*iop.IoPattern.MaxSizeBytesAvg))))
	catS = append(catS, fmt.Sprintf("%s(read:%d-%d%%)",
		iop.ReadWriteMix.Name, *iop.ReadWriteMix.MinReadPercent, *iop.ReadWriteMix.MaxReadPercent))
	iopS := strings.Join(catS, ", ")
	pUnitS := "Service Plan missing Provisioning Unit"
	if *o.ProvisioningUnit.IOPS != 0 {
		pUnitS = strconv.FormatInt(*o.ProvisioningUnit.IOPS, 10) + "IOPS/GiB"
	} else if *o.ProvisioningUnit.Throughput != 0 {
		pUnitS = sizeToString(*o.ProvisioningUnit.Throughput) + "/s/GiB"
	}
	vsMin := sizeToString(*o.VolumeSeriesMinMaxSize.MinSizeBytes)
	vsMax := sizeToString(*o.VolumeSeriesMinMaxSize.MaxSizeBytes)
	return map[string]string{
		hID:                string(o.Meta.ID),
		hName:              string(o.Name),
		hDescription:       string(o.Description),
		hAccounts:          strings.Join(accounts, ", "),
		hSourceServicePlan: sourceSP,
		hState:             o.State,
		hTags:              strings.Join(o.Tags, ", "),
		hSLO:               strings.Join(slos, ", "),
		hIOProfile:         iopS,
		hProvisioningUnit:  pUnitS,
		hVSMinSize:         vsMin,
		hVSMaxSize:         vsMax,
	}
}

func (c *servicePlanCmd) validateColumns(columns string) error {
	var err error
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(servicePlanHeaders), servicePlanDefaultHeaders)
	return err
}

func (c *servicePlanCmd) Emit(data []*models.ServicePlan) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.EmitJSON(data)
	case "yaml":
		return appCtx.EmitYAML(data)
	}
	c.cacheAccounts()
	c.cacheServicePlans()
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

func (c *servicePlanCmd) list(params *service_plan.ServicePlanListParams) ([]*models.ServicePlan, error) {
	if params == nil {
		params = service_plan.NewServicePlanListParams()
	}
	res, err := appCtx.API.ServicePlan().ServicePlanList(params)
	if err != nil {
		if e, ok := err.(*service_plan.ServicePlanListDefault); ok && e.Payload.Message != nil {
			return nil, fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res.Payload, nil
}

type servicePlanListCmd struct {
	Name                  string   `short:"n" long:"name" description:"A service plan name"`
	AuthorizedAccountName string   `short:"Z" long:"auth-account-name" description:"Name of an account authorized to use the service plan"`
	OwnerAuthorized       bool     `long:"owner-auth" description:"Match a service plan where there owner (the context account) is also the authorized account"`
	Source                string   `short:"s" long:"source-service-plan" description:"A service plan name" hidden:"1"` // not currently useful
	Tags                  []string `short:"t" long:"tag" description:"A tag value; repeat as needed"`
	Columns               string   `short:"c" long:"columns" description:"Comma separated list of column names"`

	servicePlanCmd
	remainingArgsCatcher
}

func (c *servicePlanListCmd) Execute(args []string) error {
	var err error
	if err = c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	if c.AuthorizedAccountName != "" && c.OwnerAuthorized {
		return fmt.Errorf("do not specify --auth-account-name and --owner-auth together")
	}
	if c.OwnerAuthorized && appCtx.Account == "" {
		return fmt.Errorf("--owner-auth requires --account")
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	params := service_plan.NewServicePlanListParams()
	if c.AuthorizedAccountName != "" {
		authAccountID, err := c.validateAccount(c.AuthorizedAccountName, "authorized")
		if err != nil {
			return err
		}
		params.AuthorizedAccountID = &authAccountID
	} else if c.OwnerAuthorized {
		params.AuthorizedAccountID = &appCtx.AccountID
	}
	params.Tags = c.Tags
	if c.Name != "" {
		params.Name = &c.Name
	}
	if c.Source != "" {
		if err = c.cacheServicePlans(); err != nil {
			return err
		}
		for i, cn := range c.servicePlans {
			if cn == c.Source {
				params.SourceServicePlanID = swag.String(i)
				break
			}
		}
		if params.SourceServicePlanID == nil {
			return fmt.Errorf("Source service plan \"%s\" not found", c.Source)
		}
	}
	var res []*models.ServicePlan
	if res, err = c.list(params); err != nil {
		return err
	}
	return c.Emit(res)
}

type servicePlanModifyCmd struct {
	Name            string   `short:"n" long:"name" description:"The name of the ServicePlan object to be modified" required:"yes"`
	Accounts        []string `short:"Z" long:"authorized-account" description:"Name of an authorized account. Repeat as needed. Subject to the value of accounts-action"`
	AccountsAction  string   `long:"accounts-action" description:"Specifies how to process authorized-account values" choice:"APPEND" choice:"REMOVE" choice:"SET" default:"APPEND"`
	OwnerAuthorized bool     `long:"owner-auth" description:"Authorize the owner (the context account). Subject to the value of accounts-action"`
	Version         int32    `short:"V" long:"version" description:"Enforce update of the specified version of the object"`
	Columns         string   `short:"c" long:"columns" description:"Comma separated list of column names"`
	servicePlanCmd
	remainingArgsCatcher
}

func (c *servicePlanModifyCmd) Execute(args []string) error {
	var err error
	if err = c.verifyNoRemainingArgs(); err != nil {
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
	lParams := service_plan.NewServicePlanListParams().WithName(&c.Name)
	var lRes []*models.ServicePlan
	if lRes, err = c.list(lParams); err != nil {
		return err
	} else if len(lRes) != 1 {
		return fmt.Errorf("ServicePlan '%s' not found", c.Name)
	}
	nChg := 0
	uParams := service_plan.NewServicePlanUpdateParams().WithPayload(&models.ServicePlanMutable{})
	if len(c.Accounts) != 0 || c.OwnerAuthorized || c.AccountsAction == "SET" {
		if c.OwnerAuthorized {
			uParams.Payload.Accounts = append(uParams.Payload.Accounts, models.ObjIDMutable(appCtx.AccountID))
		}
		for _, account := range c.Accounts {
			id, err := c.validateAccount(account, "authorized")
			if err != nil {
				return err
			}
			uParams.Payload.Accounts = append(uParams.Payload.Accounts, models.ObjIDMutable(id))
		}
		switch c.AccountsAction {
		case "APPEND":
			uParams.Append = append(uParams.Append, "accounts")
		case "SET":
			uParams.Set = append(uParams.Set, "accounts")
		case "REMOVE":
			uParams.Remove = append(uParams.Remove, "accounts")
		}
		nChg++
	}
	if nChg == 0 {
		return fmt.Errorf("No modifications specified")
	}
	uParams.ID = string(lRes[0].Meta.ID)
	if c.Version != 0 {
		uParams.Version = &c.Version
	}
	var res *service_plan.ServicePlanUpdateOK
	if res, err = appCtx.API.ServicePlan().ServicePlanUpdate(uParams); err != nil {
		if e, ok := err.(*service_plan.ServicePlanUpdateDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.ServicePlan{res.Payload})
}
