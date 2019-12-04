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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/audit_log"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
)

func init() {
	initAuditLog()
}

func initAuditLog() {
	cmd, _ := parser.AddCommand("audit-log", "Audit Log Records commands", "Audit Log Records subcommands", &auditLogCmd{})
	cmd.Aliases = []string{"audit", "audit-log-records", "al"}
	ann, _ := cmd.AddCommand("annotate", "Create an Annotation", "Annotate a previous Audit Log Record.", &auditLogAnnotateCmd{})
	ann.Aliases = []string{"create", "ann"}
	cmd.AddCommand("list", "List Audit Log Records", "List or search for Audit Log Records.", &auditLogListCmd{})
	cmd.AddCommand("show-columns", "Show Audit Log table columns", "Show names of columns used in table format", &showColsCmd{columns: auditLogHeaders})
}

type auditLogCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string

	cacheHelper
}

// audit_log.record keys/headers and their description
var auditLogHeaders = map[string]string{
	hRecordNum:      "The record sequence number.",
	hParentNum:      "The sequence number of the parent record.",
	hTimestamp:      "The time the record was logged.",
	hClassification: "The classification of the record.",
	hObjectType:     "The type of the object affected.",
	hID:             dID,
	hName:           dName,
	hAction:         "The action performed.",
	hTenant:         "The encompassing tenant account for the account.",
	hAccountID:      "The ID of the account that performed the action.",
	hAccount:        "The name of the account that performed the action.",
	hUserID:         "The ID of the user that performed the action.",
	hAuthIdentifier: "The authentication identifier of the user that performed the action.",
	hError:          "Indicates if the record logs an error.",
	hMessage:        "A message.",
}

var auditLogDefaultHeaders = []string{hTimestamp, hClassification, hObjectType, hAction, hName, hAccount, hAuthIdentifier, hError, hMessage}

// makeRecord creates a map of properties
func (c *auditLogCmd) makeRecord(o *models.AuditLogRecord) map[string]string {
	tenant := string(o.TenantAccountID)
	if a, ok := c.accounts[tenant]; ok {
		tenant = a.name
	}
	eValue := ""
	if o.Error {
		eValue = "E"
	}
	return map[string]string{
		hRecordNum:      fmt.Sprintf("%d", o.RecordNum),
		hParentNum:      fmt.Sprintf("%d", o.ParentNum),
		hTimestamp:      time.Time(o.Timestamp).Format(time.RFC3339),
		hClassification: o.Classification,
		hObjectType:     o.ObjectType,
		hID:             string(o.ObjectID),
		hName:           o.Name,
		hAction:         o.Action,
		hTenant:         tenant,
		hAccountID:      string(o.AccountID),
		hAccount:        o.AccountName,
		hUserID:         string(o.UserID),
		hAuthIdentifier: o.AuthIdentifier,
		hError:          eValue,
		hMessage:        o.Message,
	}
}

func (c *auditLogCmd) validateColumns(columns string) error {
	var err error
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(auditLogHeaders), auditLogDefaultHeaders)
	return err
}

func (c *auditLogCmd) Emit(data []*models.AuditLogRecord) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.EmitJSON(data)
	case "yaml":
		return appCtx.EmitYAML(data)
	}
	c.cacheAccounts()
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

func (c *auditLogCmd) list(params *audit_log.AuditLogListParams) ([]*models.AuditLogRecord, error) {
	res, err := appCtx.API.AuditLog().AuditLogList(params)
	if err != nil {
		if e, ok := err.(*audit_log.AuditLogListDefault); ok && e.Payload.Message != nil {
			return nil, fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res.Payload, nil
}

type auditLogAnnotateCmd struct {
	Action  string `short:"a" long:"action" description:"The action performed. The value will be converted to lower case"`
	Error   bool   `long:"error" description:"The annotation reports an error"`
	Message string `short:"m" long:"message" description:"A free form message. Leading and trailing whitespace is ignored"`
	Name    string `short:"n" long:"name" description:"A name. Leading and trailing whitespace is ignored"`
	Parent  int32  `short:"p" long:"parent" description:"Record number of the parent record" required:"yes"`
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`

	auditLogCmd
	remainingArgsCatcher
}

func (c *auditLogAnnotateCmd) Execute(args []string) error {
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

	params := audit_log.NewAuditLogCreateParams().WithPayload(&models.AuditLogRecord{})
	params.Payload.Action = strings.TrimSpace(strings.ToLower(c.Action))
	params.Payload.Classification = com.AnnotationClass
	params.Payload.Error = c.Error
	params.Payload.Message = strings.TrimSpace(c.Message)
	params.Payload.Name = strings.TrimSpace(c.Name)
	params.Payload.ParentNum = c.Parent
	res, err := appCtx.API.AuditLog().AuditLogCreate(params)
	if err != nil {
		if e, ok := err.(*audit_log.AuditLogCreateDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.AuditLogRecord{res.Payload})
}

type auditLogListCmd struct {
	Type           string         `short:"T" long:"type" description:"An Audit Log Record object type, case insensitive. Object types are the same as the command groups, eg. account or volume-series, but not aliases and the dashes may be omitted"`
	Action         []string       `short:"a" long:"action" description:"The action performed, case insensitive. The actions can vary by object type, but most objects have 'create', 'delete' and 'update' actions. Repeat as needed"`
	Classification string         `short:"C" long:"classification" description:"Select only records with a single classification" choice:"audit" choice:"event" choice:"annotation"`
	Error          bool           `long:"error" description:"Select only records reporting errors only"`
	NoError        bool           `long:"no-error" description:"Filter out records reporting errors"`
	AccountOnly    bool           `long:"account-only" description:"Only retrieve records where the account matches the context account. Applies to system and tenant admins"`
	SubAccount     string         `short:"S" long:"subordinate-account" description:"Only retrieve records where the account matches the specified account that is subordinate to the context account. Applies to system and tenant admins"`
	Name           string         `short:"n" long:"name" description:"An object name"`
	Related        bool           `short:"r" long:"related" description:"Also retrieve all records related to the object specified in the --id option"`
	After          util.Timestamp `short:"R" long:"timestamp-ge" description:"The lower bound of timestamps to list, specified either as a duration or an absolute RFC3339 time"`
	Before         util.Timestamp `short:"B" long:"timestamp-le" description:"The upper bound of timestamps to list, specified either as a duration or an absolute RFC3339 time"`
	RecordNumMin   int32          `long:"record-num-ge" description:"The lower bound of record sequence numbers to list"`
	RecordNumMax   int32          `long:"record-num-le" description:"The upper bound of record sequence numbers to list"`
	Count          int32          `long:"count" description:"The maximum number of records to return"`
	User           string         `short:"u" long:"user" description:"Select only records of a single user, specified using their current auth identifier"`
	Columns        string         `short:"c" long:"columns" description:"Comma separated list of column names"`

	auditLogCmd
	optionalIDRemainingArgsCatcher
}

func (c *auditLogListCmd) Execute(args []string) error {
	var err error
	if err = c.verifyOptionalIDAndNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	params := audit_log.NewAuditLogListParams()
	if c.Type != "" {
		c.Type = strings.Replace(c.Type, "-", "", -1) // server side expects no dashes
		params.ObjectType = &c.Type
	}
	if c.Classification != "" {
		params.Classification = &c.Classification
	}
	if c.Error || c.NoError {
		if c.Error && c.NoError {
			return fmt.Errorf("do not specify 'error' and 'no-error' together")
		}
		params.Error = &c.Error
	}
	if c.Name != "" {
		params.Name = &c.Name
	}
	if c.ID != "" {
		params.ObjectID = &c.ID
	}
	if c.Related {
		params.Related = &c.Related
	}
	if c.After.Specified() {
		dt := c.After.Value()
		params.TimeStampGE = &dt
	}
	if c.Before.Specified() {
		dt := c.Before.Value()
		params.TimeStampLE = &dt
	}
	if c.RecordNumMin > 0 {
		params.RecordNumGE = &c.RecordNumMin
	}
	if c.RecordNumMax > 0 {
		params.RecordNumLE = &c.RecordNumMax
	}
	if c.Count > 0 {
		params.Count = &c.Count
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	if c.SubAccount != "" {
		if c.AccountOnly {
			return fmt.Errorf("do not specify 'account-only' and 'subordinate-account' together")
		}
		aID, err := c.validateAccount(c.SubAccount, "")
		if err != nil {
			return err
		}
		params.AccountID = &aID
	} else if c.AccountOnly {
		params.AccountID = &appCtx.AccountID
	}
	var res []*models.AuditLogRecord
	if res, err = c.list(params); err != nil {
		return err
	}
	return c.Emit(res)
}
