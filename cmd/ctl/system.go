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
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/system"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
)

func init() {
	initSystem()
}

func initSystem() {
	cmd, _ := parser.AddCommand("system", "System object commands", "System object subcommands", &systemCmd{})
	cmd.Aliases = []string{"sys"}
	cmd.AddCommand("get", "Get the System object", "Get information about the installed Nuvoloso system.", &systemGetCmd{})
	cmd.AddCommand("modify", "Modify the System object", "Modify the System object.", &systemModifyCmd{})
	cmd.AddCommand("show-columns", "Show System table columns", "Show names of columns used in table format", &showColsCmd{columns: systemHeaders})
	cmd.AddCommand("get-hostname", "Get the System hostname", "Get the System hostname. Will be derived from system object if exists else from the Kubernetes load balancer service.", &systemGetHostnameCmd{})
}

type systemCmd struct {
	tableCols []string

	cacheHelper
}

// system record keys/headers and their description
var systemHeaders = map[string]string{
	hID:               dID,
	hTimeCreated:      dTimeCreated,
	hTimeModified:     dTimeModified,
	hVersion:          dVersion,
	hName:             dName,
	hDescription:      dDescription,
	hSystemAttributes: "Attributes describing the system.",
	hControllerState:  dControllerState,
	hSystemVersion:    "The version of the system software.",
}

var systemDefaultHeaders = []string{hName, hID, hDescription, hControllerState, hSystemVersion}

// makeRecord creates a map of properties
func (c *systemCmd) makeRecord(o *models.System) map[string]string {
	attrList := []string{}
	controllerState := "UNKNOWN"
	systemVersion := "UNKNOWN"
	if o.Service != nil {
		if o.Service.ServiceAttributes != nil {
			attrs := make([]string, 0, len(o.Service.ServiceAttributes))
			for a := range o.Service.ServiceAttributes {
				attrs = append(attrs, a)
			}
			sort.Strings(attrs)
			for _, a := range attrs {
				v := o.Service.ServiceAttributes[a]
				s := a + "[" + v.Kind[0:1] + "]: " + v.Value
				attrList = append(attrList, s)
			}
		}
		if o.Service.State != "" {
			controllerState = o.Service.State
		}
		systemVersion = o.Service.ServiceVersion
	}

	return map[string]string{
		hID:               string(o.Meta.ID),
		hTimeCreated:      time.Time(o.Meta.TimeCreated).Format(time.RFC3339),
		hTimeModified:     time.Time(o.Meta.TimeModified).Format(time.RFC3339),
		hVersion:          fmt.Sprintf("%d", o.Meta.Version),
		hName:             string(o.Name),
		hDescription:      string(o.Description),
		hSystemAttributes: strings.Join(attrList, "\n"),
		hControllerState:  controllerState,
		hSystemVersion:    systemVersion,
	}
}

func (c *systemCmd) validateColumns(columns string) error {
	if matched, _ := regexp.MatchString("^\\s*$", columns); matched {
		c.tableCols = systemDefaultHeaders
	} else {
		c.tableCols = strings.Split(columns, ",")
		for _, col := range c.tableCols {
			if _, ok := systemHeaders[col]; !ok {
				return fmt.Errorf("invalid column \"%s\"", col)
			}
		}
	}
	return nil
}

func (c *systemCmd) Emit(data *models.System) error {
	switch appCtx.OutputFormat {
	case "json":
		return appCtx.EmitJSON(data)
	case "yaml":
		return appCtx.EmitYAML(data)
	}
	rows := make([][]string, 1)
	rec := c.makeRecord(data)
	row := make([]string, len(c.tableCols))
	for j, h := range c.tableCols {
		row[j] = rec[h]
	}
	rows[0] = row
	return appCtx.EmitTable(c.tableCols, rows, nil)
}

func (c *systemCmd) get(params *system.SystemFetchParams) (*models.System, error) {
	res, err := appCtx.API.System().SystemFetch(params)
	if err != nil {
		if e, ok := err.(*system.SystemFetchDefault); ok && e.Payload.Message != nil {
			return nil, fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res.Payload, nil
}

type systemGetCmd struct {
	Columns string `long:"columns" description:"Comma separated list of column names"`

	systemCmd
	remainingArgsCatcher
}

func (c *systemGetCmd) Execute(args []string) error {
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
	params := system.NewSystemFetchParams()
	var res *models.System
	if res, err = c.get(params); err != nil {
		return err
	}
	return c.Emit(res)
}

type systemModifyCmd struct {
	NewName     string `short:"N" long:"new-name" description:"The new name for the object"`
	Description string `short:"d" long:"description" description:"The new description; leading and trailing whitespace will be stripped; set space to clear"`
	CUPFlags
	SMPFlags
	SCPFlags
	VSRPFlags
	UserPasswordMinLength int32  `long:"user-password-min-length" description:"The minimum number of characters required in a user's password"`
	ManagementHostCName   string `short:"M" long:"management-host-cname" description:"The management host dns cname; leading and trailing whitespace will be stripped; set space to clear"`

	Version int32  `short:"V" long:"version" description:"Enforce update of the specified version of the object"`
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`

	systemCmd
	remainingArgsCatcher
}

func (c *systemModifyCmd) Execute(args []string) error {
	var err error
	if err = c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err = c.SmpValidateFlags(); err != nil {
		return err
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	params := system.NewSystemFetchParams()
	var lRes *models.System
	if lRes, err = c.get(params); err != nil {
		return err
	}

	nChg := 0
	uParams := system.NewSystemUpdateParams().WithPayload(&models.SystemMutable{})
	if c.NewName != "" {
		uParams.Payload.Name = models.ObjName(c.NewName)
		uParams.Set = append(uParams.Set, "name")
		nChg++
	}
	if c.Description != "" {
		uParams.Payload.Description = models.ObjDescription(strings.TrimSpace(c.Description))
		uParams.Set = append(uParams.Set, "description")
		nChg++
	}
	if c.CupProcessLocalFlags(lRes.ClusterUsagePolicy) {
		nChg++
		uParams.Set = append(uParams.Set, "clusterUsagePolicy")
		uParams.Payload.ClusterUsagePolicy = lRes.ClusterUsagePolicy
	}
	if c.SmpProcessLocalFlags(lRes.SnapshotManagementPolicy) {
		uParams.Payload.SnapshotManagementPolicy = lRes.SnapshotManagementPolicy
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
	if c.ScpProcessLocalFlags(lRes.SnapshotCatalogPolicy) {
		uParams.Payload.SnapshotCatalogPolicy = lRes.SnapshotCatalogPolicy
		uParams.Set = append(uParams.Set, "snapshotCatalogPolicy")
		nChg++
	}
	if c.VsrpProcessLocalFlags(lRes.VsrManagementPolicy) {
		uParams.Payload.VsrManagementPolicy = lRes.VsrManagementPolicy
		uParams.Set = append(uParams.Set, "vsrManagementPolicy")
		nChg++
	}
	if c.UserPasswordMinLength != 0 {
		uParams.Payload.UserPasswordPolicy = &models.SystemMutableUserPasswordPolicy{MinLength: c.UserPasswordMinLength}
		uParams.Set = append(uParams.Set, "userPasswordPolicy.minLength")
		nChg++
	}
	if c.ManagementHostCName != "" {
		uParams.Payload.ManagementHostCName = strings.TrimSpace(c.ManagementHostCName)
		uParams.Set = append(uParams.Set, "managementHostCName")
		nChg++
	}
	if nChg == 0 {
		return fmt.Errorf("No modifications specified")
	}
	if c.Version != 0 {
		uParams.Version = &c.Version
	}
	var res *system.SystemUpdateOK
	if res, err = appCtx.API.System().SystemUpdate(uParams); err != nil {
		if e, ok := err.(*system.SystemUpdateDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit(res.Payload)
}

type systemGetHostnameCmd struct {
	OutputFile string `short:"O" long:"output-file" description:"The name of the output file. Use - for stdout" default:"-"`

	systemCmd
	remainingArgsCatcher
}

func (c *systemGetHostnameCmd) Execute(args []string) error {
	var err error
	if err = c.verifyNoRemainingArgs(); err != nil {
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
	params := system.NewSystemHostnameFetchParams()
	var res *system.SystemHostnameFetchOK
	if res, err = appCtx.API.System().SystemHostnameFetch(params); err != nil {
		if e, ok := err.(*system.SystemHostnameFetchDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	oF.WriteString(res.Payload)
	oF.WriteString("\n")
	return nil
}
