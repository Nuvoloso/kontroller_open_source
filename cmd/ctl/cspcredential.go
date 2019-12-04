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
	"sort"
	"strings"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/csp_credential"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/jessevdk/go-flags"
)

func init() {
	initCspCredential()
}

func initCspCredential() {
	cmd, _ := parser.AddCommand("csp-credential", "CSPCredential object commands", "CSPCredential object subcommands", &cspCredentialCmd{})
	cmd.Aliases = []string{"csp-credentials", "credential", "cred"}
	cmd.AddCommand("list", "List CSPCredentials", "List or search for CSPCredential objects", &cspCredentialListCmd{})
	cmd.AddCommand("create", "Create a CSPCredential", "Create a CSPCredential object", &cspCredentialCreateCmd{})
	cmd.AddCommand("delete", "Delete a CSPCredential", "Delete a CSPCredential object", &cspCredentialDeleteCmd{})
	cmd.AddCommand("modify", "Modify a CSPCredential", "Modify a CSPCredential object. Use a whitespace-only string to clear trimmed properties", &cspCredentialModifyCmd{})
	c, _ := cmd.AddCommand("metadata", "Show CSPCredential metadata", "Show metadata on a CSPCredential type", &cspCredentialMetadataCmd{})
	c.Aliases = []string{"md"}
	cmd.AddCommand("show-columns", "Show CSPCredential table columns", "Show names of columns used in table format", &showColsCmd{columns: cspCredentialHeaders})
}

type cspCredentialCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string

	cacheHelper
}

// cspCredential record keys/headers and their description
var cspCredentialHeaders = map[string]string{
	hID:                   dID,
	hTimeCreated:          dTimeCreated,
	hTimeModified:         dTimeModified,
	hVersion:              dVersion,
	hName:                 dName,
	hDescription:          dDescription,
	hAccount:              dAccount,
	hTags:                 dTags,
	hCspDomainType:        "The type of Cloud Service Provider.",
	hCredentialAttributes: "Attributes describing the Cloud Service Provider domain.",
}

var cspCredentialDefaultHeaders = []string{hName, hDescription, hCspDomainType, hCredentialAttributes, hTags}

// makeRecord creates a map of properties
func (c *cspCredentialCmd) makeRecord(o *models.CSPCredential) map[string]string {
	attrs := make([]string, 0, len(o.CredentialAttributes))
	for a := range o.CredentialAttributes {
		attrs = append(attrs, a)
	}
	account := string(o.AccountID)
	if a, ok := c.accounts[account]; ok {
		account = a.name
	}
	sort.Strings(attrs)
	attrList := []string{}
	for _, a := range attrs {
		v := o.CredentialAttributes[a]
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
		hID:                   string(o.Meta.ID),
		hTimeCreated:          time.Time(o.Meta.TimeCreated).Format(time.RFC3339),
		hTimeModified:         time.Time(o.Meta.TimeModified).Format(time.RFC3339),
		hVersion:              fmt.Sprintf("%d", o.Meta.Version),
		hName:                 string(o.Name),
		hDescription:          string(o.Description),
		hAccount:              account,
		hCspDomainType:        string(o.CspDomainType),
		hCredentialAttributes: strings.Join(attrList, "\n"),
		hTags:                 strings.Join(o.Tags, ", "),
	}
}

func (c *cspCredentialCmd) validateColumns(columns string) (err error) {
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(cspCredentialHeaders), cspCredentialDefaultHeaders)
	return err
}

func (c *cspCredentialCmd) Emit(data []*models.CSPCredential) error {
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

func (c *cspCredentialCmd) list(params *csp_credential.CspCredentialListParams) ([]*models.CSPCredential, error) {
	if params == nil {
		params = csp_credential.NewCspCredentialListParams()
	}
	res, err := appCtx.API.CspCredential().CspCredentialList(params)
	if err != nil {
		if e, ok := err.(*csp_credential.CspCredentialListDefault); ok && e.Payload.Message != nil {
			return nil, fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res.Payload, nil
}

func (c *cspCredentialCmd) getCspCredentialMetadata(domType string) (*models.CSPCredentialMetadata, []string, error) {
	params := csp_credential.NewCspCredentialMetadataParams()
	params.CspDomainType = domType
	res, err := appCtx.API.CspCredential().CspCredentialMetadata(params)
	if err != nil {
		return nil, nil, err
	}
	attrNames := make([]string, 0, len(res.Payload.AttributeMetadata))
	attrKinds := make(map[string]string)
	for attrName, descriptor := range res.Payload.AttributeMetadata {
		attrNames = append(attrNames, attrName)
		attrKinds[attrName] = descriptor.Kind
	}
	if c.cspDomainAttrKinds == nil { // cache if not yet done so
		c.cacheCspDomainAttrKinds(attrKinds)
	}
	return res.Payload, attrNames, nil
}

func (c *cspCredentialCmd) convertAttrs(domType string, attrs map[string]string, credFile string) (map[string]models.ValueType, error) {
	if credFile == "" && len(attrs) == 0 {
		return nil, fmt.Errorf("Attributes are required for CSPCredential object of domain type %s", domType)
	}
	if c.cspDomainAttrKinds == nil { // cache if not yet done so
		if _, _, err := c.getCspCredentialMetadata(domType); err != nil {
			return nil, err
		}
	}
	if credFile != "" {
		if len(c.cspDomainAttrKinds) != 1 {
			return nil, fmt.Errorf("Unexpected attributes for domain type %s", domType)
		}
		content, err := ioutil.ReadFile(credFile)
		if err != nil {
			return nil, err
		}
		for k := range c.cspDomainAttrKinds {
			attrs[k] = string(content)
		}
	}
	dAttrs := make(map[string]models.ValueType, len(attrs))
	for attrName, value := range attrs {
		attrKind, ok := c.cspDomainAttrKinds[attrName]
		if !ok {
			attrKind = "STRING"
		}
		dAttrs[attrName] = models.ValueType{Kind: attrKind, Value: value}
	}
	return dAttrs, nil
}

type cspCredentialCreateCmd struct {
	Name        string            `short:"n" long:"name" description:"The name of the cloud service provider credential" required:"yes"`
	Description string            `short:"d" long:"description" description:"The purpose of this cloud service provider credential"`
	Tags        []string          `short:"t" long:"tag" description:"A tag value; repeat as needed"`
	Columns     string            `short:"c" long:"columns" description:"Comma separated list of column names"`
	DomainType  string            `short:"T" long:"domain-type" description:"Type of cloud service provider" choice:"AWS" choice:"Azure" choice:"GCP" default:"AWS"`
	Attrs       map[string]string `short:"a" long:"attribute" description:"An attribute name:value pair. Repeat as necessary. Depending on the domain type, attributes may be provided using the credentials file, e.g. case of Google Cloud"`
	CredFile    flags.Filename    `short:"F" long:"cred-file" description:"Path to the file containing a JSON with credentials"`

	cspCredentialCmd
	remainingArgsCatcher
}

func (c *cspCredentialCreateCmd) Execute(args []string) error {
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
	var credAttr map[string]models.ValueType
	if credAttr, err = c.convertAttrs(c.DomainType, c.Attrs, string(c.CredFile)); err != nil {
		return err
	}
	params := csp_credential.NewCspCredentialCreateParams()
	params.Payload = &models.CSPCredential{}
	params.Payload.AccountID = models.ObjIDMutable(appCtx.AccountID)
	params.Payload.Name = models.ObjName(c.Name)
	params.Payload.Description = models.ObjDescription(c.Description)
	params.Payload.Tags = c.Tags
	params.Payload.CspDomainType = models.CspDomainTypeMutable(c.DomainType)
	params.Payload.CredentialAttributes = credAttr
	res, err := appCtx.API.CspCredential().CspCredentialCreate(params)
	if err != nil {
		if e, ok := err.(*csp_credential.CspCredentialCreateDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.CSPCredential{res.Payload})
}

type cspCredentialListCmd struct {
	Name       string   `short:"n" long:"name" description:"A CSPCredential object name"`
	Tags       []string `short:"t" long:"tag" description:"A tag value; repeat as needed"`
	DomainType string   `short:"T" long:"domain-type" description:"Type of cloud service provider"`
	Columns    string   `short:"c" long:"columns" description:"Comma separated list of column names"`

	cspCredentialCmd
	remainingArgsCatcher
}

func (c *cspCredentialListCmd) Execute(args []string) error {
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
	params := csp_credential.NewCspCredentialListParams()
	params.Name = &c.Name
	params.Tags = c.Tags
	params.CspDomainType = &c.DomainType
	var res []*models.CSPCredential
	if res, err = c.list(params); err != nil {
		return err
	}
	return c.Emit(res)
}

type cspCredentialDeleteCmd struct {
	Name    string `short:"n" long:"name" description:"A CSPCredential name" required:"yes"`
	Confirm bool   `long:"confirm" description:"Confirm the deletion of the object"`

	cspCredentialCmd
	remainingArgsCatcher
}

func (c *cspCredentialDeleteCmd) Execute(args []string) error {
	if err := c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if !c.Confirm {
		return fmt.Errorf("specify --confirm to delete the \"%s\" CSPCredential object", c.Name)
	}
	if err := appCtx.InitContextAccount(); err != nil {
		return err
	}
	lParams := csp_credential.NewCspCredentialListParams()
	lParams.Name = &c.Name
	if appCtx.AccountID != "" {
		lParams.AccountID = &appCtx.AccountID
	}
	var lRes []*models.CSPCredential
	var err error
	if lRes, err = c.list(lParams); err != nil {
		return err
	}
	if len(lRes) != 1 {
		return fmt.Errorf("CSPCredential object \"%s\" owned by \"%s\" not found", c.Name, appCtx.Account)
	}
	dParams := csp_credential.NewCspCredentialDeleteParams()
	dParams.ID = string(lRes[0].Meta.ID)
	if _, err = appCtx.API.CspCredential().CspCredentialDelete(dParams); err != nil {
		if e, ok := err.(*csp_credential.CspCredentialDeleteDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return nil
}

type cspCredentialModifyCmd struct {
	Name        string            `short:"n" long:"name" description:"The name of the CSPCredential object to be modified" required:"yes"`
	NewName     string            `short:"N" long:"new-name" description:"The new name for the object"`
	Description string            `short:"d" long:"description" description:"The new description; leading and trailing whitespace will be stripped"`
	Attrs       map[string]string `short:"a" long:"attribute" description:"An attribute name:value pair; repeat as necessary, subject to the value of attribute-action"`
	AttrsAction string            `long:"attribute-action" description:"Specifies how to process attribute values" choice:"APPEND" choice:"REMOVE" choice:"SET" default:"APPEND"`
	Tags        []string          `short:"t" long:"tag" description:"A tag value; repeat as needed, subject to the value of tag-action"`
	TagsAction  string            `long:"tag-action" description:"Specifies how to process tag values" choice:"APPEND" choice:"REMOVE" choice:"SET" default:"APPEND"`
	Version     int32             `short:"V" long:"version" description:"Enforce update of the specified version of the object"`
	Columns     string            `short:"c" long:"columns" description:"Comma separated list of column names"`
	cspCredentialCmd
	remainingArgsCatcher
}

func (c *cspCredentialModifyCmd) Execute(args []string) error {
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
	lParams := csp_credential.NewCspCredentialListParams()
	lParams.Name = &c.Name
	if appCtx.AccountID != "" {
		lParams.AccountID = &appCtx.AccountID
	}
	var lRes []*models.CSPCredential
	if lRes, err = c.list(lParams); err != nil {
		return err
	}
	if len(lRes) != 1 {
		return fmt.Errorf("CSPCredential object \"%s\" owned by \"%s\" not found", c.Name, appCtx.Account)
	}
	nChg := 0
	uParams := csp_credential.NewCspCredentialUpdateParams()
	uParams.Payload = &models.CSPCredentialMutable{}
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
	if len(c.Attrs) != 0 {
		var credAttr map[string]models.ValueType
		if credAttr, err = c.convertAttrs(string(lRes[0].CspDomainType), c.Attrs, ""); err != nil {
			return err
		}
		uParams.Payload.CredentialAttributes = credAttr
		switch c.AttrsAction {
		case "APPEND":
			uParams.Append = append(uParams.Append, "credentialAttributes")
		case "SET":
			uParams.Set = append(uParams.Set, "credentialAttributes")
		case "REMOVE":
			uParams.Remove = append(uParams.Remove, "credentialAttributes")
		}
		nChg++
	} else if c.AttrsAction == "SET" {
		uParams.Set = append(uParams.Set, "credentialAttributes")
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
	if nChg == 0 {
		return fmt.Errorf("No modifications specified")
	}
	uParams.ID = string(lRes[0].Meta.ID)
	if c.Version != 0 {
		uParams.Version = &c.Version
	}
	var res *csp_credential.CspCredentialUpdateOK
	if res, err = appCtx.API.CspCredential().CspCredentialUpdate(uParams); err != nil {
		if e, ok := err.(*csp_credential.CspCredentialUpdateDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.CSPCredential{res.Payload})
}

type cspCredentialMetadataCmd struct {
	DomainType string `short:"T" long:"domain-type" description:"Type of cloud service provider" choice:"AWS" choice:"Azure" choice:"GCP" default:"AWS"`
	cspCredentialCmd
	remainingArgsCatcher
}

var cspCredentialMetadataCols = []string{"AttributeName", "Kind", "Description", "Required"}

func (c *cspCredentialMetadataCmd) Execute(args []string) error {
	if err := c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	md, attrNames, err := c.getCspCredentialMetadata(c.DomainType)
	if err != nil {
		if e, ok := err.(*csp_credential.CspCredentialMetadataDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	switch c.OutputFormat {
	case "json":
		return appCtx.EmitJSON(md)
	case "yaml":
		return appCtx.EmitYAML(md)
	}

	sort.Strings(attrNames)
	rows := make([][]string, 0, len(attrNames))
	for _, n := range attrNames {
		d := md.AttributeMetadata[n]
		row := []string{
			n,
			d.Kind,
			d.Description,
			fmt.Sprintf("%v", !d.Optional),
		}
		rows = append(rows, row)
	}
	return appCtx.EmitTable(cspCredentialMetadataCols, rows, nil)
}
