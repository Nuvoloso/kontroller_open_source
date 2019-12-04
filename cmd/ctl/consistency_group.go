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
	"strings"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/consistency_group"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
)

func init() {
	initConsistencyGroup()
}

func initConsistencyGroup() {
	cmd, _ := parser.AddCommand("consistency-group", "ConsistencyGroup object commands", "ConsistencyGroup object subcommands", &consistencyGroupCmd{})
	cmd.Aliases = []string{"consistency-groups", "cg", "cgs"}
	cmd.AddCommand("list", "List consistency groups", "List ConsistencyGroups.", &consistencyGroupListCmd{})
	cmd.AddCommand("modify", "Modify consistency group", "Modify consistency group object.", &consistencyGroupModifyCmd{})
	cmd.AddCommand("show-columns", "Show consistencyGroup table columns", "Show names of columns used in table format", &showColsCmd{columns: consistencyGroupHeaders})
}

type consistencyGroupCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string

	cacheHelper
}

// consistencyGroup record keys/headers and their description
var consistencyGroupHeaders = map[string]string{
	hID:                       dID,
	hTimeCreated:              dTimeCreated,
	hTimeModified:             dTimeModified,
	hVersion:                  dVersion,
	hAccount:                  dAccount,
	hApplicationGroups:        "The application groups to which the consistency group belongs.",
	hName:                     dName,
	hDescription:              dDescription,
	hSnapshotManagementPolicy: "Snapshot management policy",
	hTags:                     dTags,
}

var consistencyGroupDefaultHeaders = []string{hName, hAccount, hApplicationGroups, hTags}

// makeRecord creates a map of properties
func (c *consistencyGroupCmd) makeRecord(o *models.ConsistencyGroup) map[string]string {
	var account string
	if a, ok := c.accounts[string(o.AccountID)]; !ok {
		account = string(o.AccountID)
	} else {
		account = a.name
	}
	ags := []string{}
	for _, id := range o.ApplicationGroupIds {
		ag := string(id)
		if an, ok := c.applicationGroups[ag]; ok {
			ag = an.name
		}
		ags = append(ags, ag)
	}
	vdrStr := o.SnapshotManagementPolicy.VolumeDataRetentionOnDelete
	if vdrStr == "" {
		vdrStr = "N/A"
	}
	var smpBuf strings.Builder
	fmt.Fprintf(&smpBuf, "Inherited: %v", o.SnapshotManagementPolicy.Inherited)
	fmt.Fprintf(&smpBuf, "\nDeleteLast: %v", o.SnapshotManagementPolicy.DeleteLast)
	fmt.Fprintf(&smpBuf, "\nDeleteVolumeWithLast: %v", o.SnapshotManagementPolicy.DeleteVolumeWithLast)
	fmt.Fprintf(&smpBuf, "\nDisableSnapshotCreation: %v", o.SnapshotManagementPolicy.DisableSnapshotCreation)
	fmt.Fprintf(&smpBuf, "\nNoDelete: %v", o.SnapshotManagementPolicy.NoDelete)
	fmt.Fprintf(&smpBuf, "\nRetentionDurationSeconds: %d", swag.Int32Value(o.SnapshotManagementPolicy.RetentionDurationSeconds))
	fmt.Fprintf(&smpBuf, "\nVolumeDataRetentionOnDelete: %s", vdrStr)
	return map[string]string{
		hID:                       string(o.Meta.ID),
		hTimeCreated:              time.Time(o.Meta.TimeCreated).Format(time.RFC3339),
		hTimeModified:             time.Time(o.Meta.TimeModified).Format(time.RFC3339),
		hVersion:                  fmt.Sprintf("%d", o.Meta.Version),
		hName:                     string(o.Name),
		hDescription:              string(o.Description),
		hAccount:                  account,
		hApplicationGroups:        strings.Join(ags, ", "),
		hSnapshotManagementPolicy: smpBuf.String(),
		hTags:                     strings.Join(o.Tags, ", "),
	}
}

func (c *consistencyGroupCmd) validateColumns(columns string) error {
	var err error
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(consistencyGroupHeaders), consistencyGroupDefaultHeaders)
	return err
}

func (c *consistencyGroupCmd) Emit(data []*models.ConsistencyGroup) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.EmitJSON(data)
	case "yaml":
		return appCtx.EmitYAML(data)
	}
	c.cacheAccounts()
	c.cacheApplicationGroups()
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

func (c *consistencyGroupCmd) list(params *consistency_group.ConsistencyGroupListParams) ([]*models.ConsistencyGroup, error) {
	if params == nil {
		params = consistency_group.NewConsistencyGroupListParams()
	}
	res, err := appCtx.API.ConsistencyGroup().ConsistencyGroupList(params)
	if err != nil {
		if e, ok := err.(*consistency_group.ConsistencyGroupListDefault); ok && e.Payload.Message != nil {
			return nil, fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res.Payload, nil
}

type consistencyGroupListCmd struct {
	Name             string   `short:"n" long:"name" description:"An consistencyGroup name"`
	ApplicationGroup string   `long:"application-group" description:"The name of a application group"`
	Tags             []string `short:"t" long:"tag" description:"A tag value; repeat as needed"`
	OwnedOnly        bool     `long:"owned-only" description:"Retrieve only consistencyGroups owned by the account, otherwise, if a tenant admin, retrieve consistencyGroups owned by the tenant or any subordinate account. A no-op for non-tenant accounts"`
	Columns          string   `short:"c" long:"columns" description:"Comma separated list of column names"`

	consistencyGroupCmd
	remainingArgsCatcher
}

func (c *consistencyGroupListCmd) Execute(args []string) error {
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
	params := consistency_group.NewConsistencyGroupListParams()
	if c.Name != "" {
		params.Name = &c.Name
	}
	if appCtx.AccountID != "" && c.OwnedOnly {
		params.AccountID = &appCtx.AccountID
	}
	if c.ApplicationGroup != "" {
		agList := []string{c.ApplicationGroup}
		agIds, _, err := c.validateApplicationConsistencyGroupNames(agList, "")
		if err != nil {
			return err
		}
		params.ApplicationGroupID = swag.String(string(agIds[0]))
	}
	params.Tags = c.Tags
	var res []*models.ConsistencyGroup
	if res, err = c.list(params); err != nil {
		return err
	}
	return c.Emit(res)
}

type consistencyGroupModifyCmd struct {
	Name            string   `short:"n" long:"name" description:"A consistency group name" required:"yes"`
	AppGroups       []string `long:"application-group" description:"Name of an ApplicationGroup. Repeat as needed. Subject to the value of application-group-action"`
	AppGroupsAction string   `long:"application-group-action" description:"Specifies how to process application-group values" choice:"APPEND" choice:"REMOVE" choice:"SET" default:"APPEND"`
	SMPFlagsWithInheritance
	Tags       []string `short:"t" long:"tag" description:"A tag value; repeat as needed"`
	TagsAction string   `long:"tag-action" description:"Specifies how to process tag values" choice:"APPEND" choice:"REMOVE" choice:"SET" default:"APPEND"`
	Version    int32    `short:"V" long:"version" description:"Enforce update of the specified version of the object"`
	Columns    string   `short:"c" long:"columns" description:"Comma separated list of column names"`

	consistencyGroupCmd
	remainingArgsCatcher
}

func (c *consistencyGroupModifyCmd) Execute(args []string) error {
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
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	agIds := []models.ObjIDMutable{}
	if len(c.AppGroups) > 0 {
		agIds, _, err = c.validateApplicationConsistencyGroupNames(c.AppGroups, "")
		if err != nil {
			return err
		}
	}

	cgObjList, err := c.list(consistency_group.NewConsistencyGroupListParams().WithName(&c.Name).WithAccountID(&appCtx.AccountID))
	if err != nil {
		return err
	}
	if len(cgObjList) < 1 {
		return fmt.Errorf("consistency group '%s' not found", c.Name)
	}
	cgObj := cgObjList[0]
	nChg := 0

	uParams := consistency_group.NewConsistencyGroupUpdateParams().WithPayload(&models.ConsistencyGroupMutable{})
	if len(agIds) != 0 || c.AppGroupsAction == "SET" {
		uParams.Payload.ApplicationGroupIds = agIds
		switch c.AppGroupsAction {
		case "APPEND":
			uParams.Append = append(uParams.Append, "applicationGroupIds")
		case "SET":
			uParams.Set = append(uParams.Set, "applicationGroupIds")
		case "REMOVE":
			uParams.Remove = append(uParams.Remove, "applicationGroupIds")
		}
		nChg++
	}
	if c.SmpProcessInheritableFlags(cgObj.SnapshotManagementPolicy) {
		uParams.Payload.SnapshotManagementPolicy = cgObj.SnapshotManagementPolicy
		uParams.Set = append(uParams.Set, "snapshotManagementPolicy")
		nChg++
	}
	if len(c.Tags) != 0 || c.TagsAction == "SET" {
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
	} else {
		uParams.Version = swag.Int32(int32(cgObj.Meta.Version))
	}
	uParams.ID = string(cgObj.Meta.ID)
	var res *consistency_group.ConsistencyGroupUpdateOK
	if res, err = appCtx.API.ConsistencyGroup().ConsistencyGroupUpdate(uParams); err != nil {
		if e, ok := err.(*consistency_group.ConsistencyGroupUpdateDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.ConsistencyGroup{res.Payload})
}
