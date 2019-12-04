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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/account"
	"github.com/Nuvoloso/kontroller/pkg/autogen/client/protection_domain"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
)

func init() {
	initProtectionDomain()
}

func initProtectionDomain() {
	cmd, _ := parser.AddCommand("protection-domain", "ProtectionDomain object commands", "ProtectionDomain object subcommands", &protectionDomainCmd{})
	cmd.Aliases = []string{"pd"}
	cmd.AddCommand("create", "Create a ProtectionDomain", "Create a ProtectionDomain object", &protectionDomainCreateCmd{})
	cmd.AddCommand("delete", "Delete a ProtectionDomain", "Delete a ProtectionDomain object", &protectionDomainDeleteCmd{})
	cmd.AddCommand("get", "Get a ProtectionDomain", "Get a ProtectionDomain object", &protectionDomainFetchCmd{})
	cmd.AddCommand("list", "List ProtectionDomains", "List or search for ProtectionDomain objects.", &protectionDomainListCmd{})
	cmd.AddCommand("modify", "Modify a ProtectionDomain", "Modify a ProtectionDomain object.", &protectionDomainModifyCmd{})

	cmd.AddCommand("clear", "Clear a ProtectionDomain association in the Account object", "Remove a ProtectionDomain association in the Account object.", &protectionDomainClearCmd{})
	cmd.AddCommand("set", "Set a ProtectionDomain in the Account object", "Associate a ProtectionDomain with a CSPDomain in the Account object.", &protectionDomainSetCmd{})
	cmd.AddCommand("show", "Show ProtectionDomains in the Account object", "Show the Account object ProtectionDomain associations.", &protectionDomainShowCmd{})

	c, _ := cmd.AddCommand("metadata", "Show ProtectionDomain metadata", "Show metadata on a ProtectionDomain encryption choices", &protectionDomainMetadataCmd{})
	c.Aliases = []string{"md"}
	cmd.AddCommand("show-columns", "Show ProtectionDomain table columns", "Show names of columns used in table format", &showColsCmd{columns: protectionDomainHeaders})
}

type protectionDomainCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string

	cacheHelper
}

// protectionDomain record keys/headers and their description
var protectionDomainHeaders = map[string]string{
	hID:           dID,
	hTags:         dTags,
	hAccount:      dAccount,
	hName:         dName,
	hDescription:  dDescription,
	hTimeCreated:  dTimeCreated,
	hTimeModified: dTimeModified,
}

var protectionDomainDefaultHeaders = []string{hAccount, hName, hDescription}

// makeRecord creates a map of properties
func (c *protectionDomainCmd) makeRecord(o *models.ProtectionDomain) map[string]string {
	account := string(o.AccountID)
	if a, ok := c.accounts[account]; ok {
		account = a.name
	}
	return map[string]string{
		hID:           string(o.Meta.ID),
		hTimeCreated:  time.Time(o.Meta.TimeCreated).Format(time.RFC3339),
		hTimeModified: time.Time(o.Meta.TimeModified).Format(time.RFC3339),
		hName:         string(o.Name),
		hDescription:  string(o.Description),
		hAccount:      account,
		hTags:         strings.Join(o.Tags, ", "),
	}
}

func (c *protectionDomainCmd) validateColumns(columns string) (err error) {
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(protectionDomainHeaders), protectionDomainDefaultHeaders)
	return err
}

func (c *protectionDomainCmd) Emit(data []*models.ProtectionDomain) error {
	// TBD maybe need to handle Kind: SECRET in attributes here
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

func (c *protectionDomainCmd) list(params *protection_domain.ProtectionDomainListParams) ([]*models.ProtectionDomain, error) {
	if params == nil {
		params = protection_domain.NewProtectionDomainListParams()
	}
	res, err := appCtx.API.ProtectionDomain().ProtectionDomainList(params)
	if err != nil {
		if e, ok := err.(*protection_domain.ProtectionDomainListDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res.Payload, nil
}

var accountPDHeaders = []string{hCspDomain, hProtectionDomain}

func (c *protectionDomainCmd) EmitAccountPD(aObj *models.Account) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.EmitJSON(aObj)
	case "yaml":
		return appCtx.EmitYAML(aObj)
	}
	c.cacheCSPDomains()
	c.cacheProtectionDomains()
	invPD := map[string]string{}
	for cspID, pdID := range aObj.ProtectionDomains {
		cspName := ""
		if cspID != common.ProtectionStoreDefaultKey {
			cspName = cspID
			if n, found := c.cspDomains[cspID]; found {
				cspName = n
			}
		}
		pdName := string(pdID)
		if o, found := c.protectionDomains[string(pdID)]; found {
			pdName = string(o.Name)
		}
		invPD[cspName] = pdName
	}
	rows := make([][]string, 0, len(aObj.ProtectionDomains))
	for _, cspName := range util.SortedStringKeys(invPD) {
		pdName := invPD[cspName]
		row := []string{cspName, pdName}
		rows = append(rows, row)
	}
	return appCtx.EmitTable(accountPDHeaders, rows, nil)
}

type protectionDomainCreateCmd struct {
	Name                string   `short:"n" long:"name" description:"The name of the protection domain" required:"yes"`
	Description         string   `short:"d" long:"description" description:"The purpose of this protection domain; leading and trailing whitespace will be stripped"`
	EncryptionAlgorithm string   `short:"E" long:"encryption-algorithm" description:"The name of an encryption algorithm" default:"AES-256"`
	PassPhrase          string   `short:"p" long:"pass-phrase" default-mask:"-" description:"The passphrase; it will be read from stdin if not specified; leading and trailing whitespace will be stripped"`
	Tags                []string `short:"t" long:"tag" description:"A tag value; repeat as needed"`
	Columns             string   `short:"c" long:"columns" description:"Comma separated list of column names"`

	protectionDomainCmd
	remainingArgsCatcher
}

func (c *protectionDomainCreateCmd) Execute(args []string) error {
	if err := c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := appCtx.InitContextAccount(); err != nil {
		return err
	}
	params := protection_domain.NewProtectionDomainCreateParams()
	params.Payload = &models.ProtectionDomainCreateArgs{}
	params.Payload.AccountID = models.ObjIDMutable(appCtx.AccountID)
	params.Payload.Name = models.ObjName(c.Name)
	params.Payload.Description = models.ObjDescription(strings.TrimSpace(c.Description))
	params.Payload.Tags = c.Tags
	params.Payload.EncryptionAlgorithm = c.EncryptionAlgorithm
	if c.PassPhrase == "" && c.EncryptionAlgorithm != common.EncryptionNone {
		pw, err := paranoidPasswordPrompt("Passphrase")
		if err != nil {
			return err
		}
		c.PassPhrase = pw
	}
	params.Payload.EncryptionPassphrase = &models.ValueType{Kind: common.ValueTypeSecret, Value: strings.TrimSpace(c.PassPhrase)}
	res, err := appCtx.API.ProtectionDomain().ProtectionDomainCreate(params)
	if err != nil {
		if e, ok := err.(*protection_domain.ProtectionDomainCreateDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.ProtectionDomain{res.Payload})
}

type protectionDomainDeleteCmd struct {
	protectionDomainFetchCmdRaw
	Confirm bool `long:"confirm" description:"Confirm the deletion of the object"`
	protectionDomainCmd
}

func (c *protectionDomainDeleteCmd) Execute(args []string) error {
	var err error
	if err = c.rawVerify(); err != nil {
		return err
	}
	if !c.Confirm {
		return fmt.Errorf("specify --confirm to delete the \"%s\" ProtectionDomain object", c.Name)
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	obj, err := c.rawFetch()
	if err != nil {
		return err
	}
	dParams := protection_domain.NewProtectionDomainDeleteParams()
	dParams.ID = string(obj.Meta.ID)
	if _, err = appCtx.API.ProtectionDomain().ProtectionDomainDelete(dParams); err != nil {
		if e, ok := err.(*protection_domain.ProtectionDomainDeleteDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return nil
}

// protectionDomainFetchCmdRaw provides low-level fetch by name or id
type protectionDomainFetchCmdRaw struct {
	Name string `short:"n" long:"name" description:"The name of the protection domain. Required if id not specified"`
	optionalIDRemainingArgsCatcher
	rawCache *cacheHelper // used if set and primed
}

func (c *protectionDomainFetchCmdRaw) rawVerify() error {
	return c.verifyOptionalIDAndNoRemainingArgs()
}

func (c *protectionDomainFetchCmdRaw) rawFetch() (*models.ProtectionDomain, error) {
	if c.ID != "" {
		if c.rawCache != nil && c.rawCache.protectionDomains != nil { // use the cache
			return c.rawCache.lookupProtectionDomainByID(c.ID)
		}
		fParams := protection_domain.NewProtectionDomainFetchParams().WithID(c.ID)
		res, err := appCtx.API.ProtectionDomain().ProtectionDomainFetch(fParams)
		if err != nil {
			if e, ok := err.(*protection_domain.ProtectionDomainFetchDefault); ok && e.Payload.Message != nil {
				err = fmt.Errorf("%s", *e.Payload.Message)
			}
			return nil, err
		}
		return res.Payload, nil
	} else if c.Name != "" {
		if c.rawCache != nil && c.rawCache.protectionDomains != nil { // use the cache
			return c.rawCache.lookupProtectionDomainByName(c.Name)
		}
		cmd := &protectionDomainCmd{}
		lParams := protection_domain.NewProtectionDomainListParams()
		lParams.Name = swag.String(c.Name)
		pdl, err := cmd.list(lParams)
		if err == nil {
			if len(pdl) == 1 {
				return pdl[0], nil
			}
			err = fmt.Errorf("not found")
		}
		return nil, err
	} else {
		return nil, fmt.Errorf("either name or id should be specified")
	}
}

type protectionDomainFetchCmd struct {
	protectionDomainFetchCmdRaw
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`
	protectionDomainCmd
}

func (c *protectionDomainFetchCmd) Execute(args []string) error {
	var err error
	if err = c.rawVerify(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	obj, err := c.rawFetch()
	if err != nil {
		return err
	}
	return c.Emit([]*models.ProtectionDomain{obj})
}

type protectionDomainListCmd struct {
	NamePattern   string   `short:"n" long:"name" description:"Regex to match names"`
	AccountName   string   `long:"account-name" description:"An account name. If not specified, the context account itself is used"`
	Tags          []string `short:"t" long:"tag" description:"A tag value; repeat as needed"`
	Columns       string   `short:"c" long:"columns" description:"Comma separated list of column names"`
	Follow        bool     `short:"f" long:"follow-changes" description:"Monitor the system and repeat the command as relevant changes occur."`
	FollowNoClear bool     `long:"follow-no-clear" description:"Do not clear the terminal (the default) when using follow-changes"`

	mustClearTerm bool
	params        *protection_domain.ProtectionDomainListParams
	runCount      int
	lastLen       int

	protectionDomainCmd
	remainingArgsCatcher
}

func (c *protectionDomainListCmd) Execute(args []string) error {
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
	accountID := ""
	if c.AccountName == "" {
		accountID = appCtx.AccountID // may be empty
	} else if accountID, err = c.validateAccount(c.AccountName, ""); err != nil {
		return err
	}
	nParams := 0
	params := protection_domain.NewProtectionDomainListParams()
	if accountID != "" {
		params.AccountID = swag.String(accountID)
		nParams++
	}
	if c.NamePattern != "" {
		params.NamePattern = swag.String(c.NamePattern)
		nParams++
	}
	if len(c.Tags) > 0 {
		nParams++
		params.Tags = c.Tags
	}
	if nParams > 0 {
		c.params = params
	}
	if c.Follow {
		if !c.FollowNoClear {
			c.mustClearTerm = true
		}
		return appCtx.WatchForChange(c)
	}
	return c.run()
}

func (c *protectionDomainListCmd) run() error {
	var err error
	var res []*models.ProtectionDomain
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
func (c *protectionDomainListCmd) ChangeDetected() error {
	c.runCount++
	return c.run()
}

// WatcherArgs is part of the ChangeDetector interface
func (c *protectionDomainListCmd) WatcherArgs() *models.CrudWatcherCreateArgs {
	cm := &models.CrudMatcher{
		URIPattern: "/protection-domains/?",
	}
	// there are no scope properties currently defined
	return &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{cm},
	}
}

type protectionDomainModifyCmd struct {
	protectionDomainFetchCmdRaw
	NewName     string   `short:"N" long:"new-name" description:"The new name for the object"`
	Description string   `short:"d" long:"description" description:"The new description; leading and trailing whitespace will be stripped"`
	Tags        []string `short:"t" long:"tag" description:"A tag value; repeat as needed"`
	TagsAction  string   `long:"tag-action" description:"Specifies how to process tag values" choice:"APPEND" choice:"REMOVE" choice:"SET" default:"APPEND"`
	Columns     string   `short:"c" long:"columns" description:"Comma separated list of column names"`

	protectionDomainCmd
}

func (c *protectionDomainModifyCmd) Execute(args []string) error {
	var err error
	if err = c.rawVerify(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	uParams := protection_domain.NewProtectionDomainUpdateParams().WithPayload(&models.ProtectionDomainMutable{})
	nChg := 0
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
	pdToUpdate, err := c.rawFetch()
	if err != nil {
		return err
	}
	uParams.ID = string(pdToUpdate.Meta.ID)
	var res *protection_domain.ProtectionDomainUpdateOK
	if res, err = appCtx.API.ProtectionDomain().ProtectionDomainUpdate(uParams); err != nil {
		if e, ok := err.(*protection_domain.ProtectionDomainUpdateDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.ProtectionDomain{res.Payload})
}

type protectionDomainClearCmd struct {
	DomainName string `short:"D" long:"domain" description:"Name of a cloud service provider domain representing the protection store whose direct association to a protection domain is to be cleared. If not specified then the DEFAULT protection domain setting will be removed from the Account"`
	Version    int32  `short:"V" long:"version" description:"Enforce update of the specified version of the Account object"`

	protectionDomainCmd
	remainingArgsCatcher
}

func (c *protectionDomainClearCmd) Execute(args []string) error {
	var err error
	if err = c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	if appCtx.AccountID == "" {
		return fmt.Errorf("The --account flag was not specified")
	}
	domID := ""
	if c.DomainName != "" {
		domID, err = c.validateDomainName(c.DomainName)
		if err != nil {
			return err
		}
	}
	// Note: this is not a ProtectionDomain method
	params := account.NewAccountProtectionDomainClearParams()
	params.ID = appCtx.AccountID
	if domID != "" {
		params.CspDomainID = &domID
	}
	if c.Version > 0 {
		params.Version = &c.Version
	}
	res, err := appCtx.API.Account().AccountProtectionDomainClear(params)
	if err != nil {
		if e, ok := err.(*account.AccountProtectionDomainClearDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.EmitAccountPD(res.Payload)
}

type protectionDomainSetCmd struct {
	protectionDomainFetchCmdRaw
	DomainName string `short:"D" long:"domain" description:"Name of a cloud service provider domain representing a protection store. If not specified then the protection domain will apply to all protection stores in the Account"`
	Version    int32  `short:"V" long:"version" description:"Enforce update of the specified version of the Account object"`

	protectionDomainCmd
	remainingArgsCatcher
}

func (c *protectionDomainSetCmd) Execute(args []string) error {
	var err error
	if err = c.rawVerify(); err != nil {
		return err
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	if appCtx.AccountID == "" {
		return fmt.Errorf("The --account flag was not specified")
	}
	// prime the cache as it is used for table output
	if err = c.cacheProtectionDomains(); err != nil {
		return err
	}
	c.rawCache = &c.cacheHelper
	pdObj, err := c.rawFetch()
	if err != nil {
		return err
	}
	domID := ""
	if c.DomainName != "" {
		domID, err = c.validateDomainName(c.DomainName)
		if err != nil {
			return err
		}
	}
	// Note: this is not a ProtectionDomain method
	params := account.NewAccountProtectionDomainSetParams()
	params.ID = appCtx.AccountID
	params.ProtectionDomainID = string(pdObj.Meta.ID)
	if domID != "" {
		params.CspDomainID = &domID
	}
	if c.Version > 0 {
		params.Version = &c.Version
	}
	res, err := appCtx.API.Account().AccountProtectionDomainSet(params)
	if err != nil {
		if e, ok := err.(*account.AccountProtectionDomainSetDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.EmitAccountPD(res.Payload)
}

type protectionDomainShowCmd struct {
	protectionDomainCmd
	remainingArgsCatcher
}

func (c *protectionDomainShowCmd) Execute(args []string) error {
	var err error
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	if appCtx.AccountID == "" {
		return fmt.Errorf("The --account flag was not specified")
	}
	if err = c.cacheAccounts(); err == nil {
		if err = c.cacheCSPDomains(); err == nil {
			err = c.cacheProtectionDomains()
		}
	}
	if err != nil {
		return err
	}
	aObj := c.accountObjs[appCtx.AccountID] // or panic
	return c.EmitAccountPD(aObj)
}

type protectionDomainMetadataCmd struct {
	protectionDomainCmd
	remainingArgsCatcher
}

var protectionDomainMetadataCols = []string{"Encryption", "Description", "Min Pass Len"}

func (c *protectionDomainMetadataCmd) Execute(args []string) error {
	var err error
	if err := c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	params := protection_domain.NewProtectionDomainMetadataParams()
	res, err := appCtx.API.ProtectionDomain().ProtectionDomainMetadata(params)
	if err != nil {
		if e, ok := err.(*protection_domain.ProtectionDomainMetadataDefault); ok && e.Payload.Message != nil {
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
	rows := make([][]string, 0, len(res.Payload))
	for _, pdm := range res.Payload {
		row := []string{
			pdm.EncryptionAlgorithm,
			pdm.Description,
			fmt.Sprintf("%d", pdm.MinPassphraseLength),
		}
		rows = append(rows, row)
	}
	return appCtx.EmitTable(protectionDomainMetadataCols, rows, nil)
}
