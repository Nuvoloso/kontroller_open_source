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
	"io"
	"os"
	"sort"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/csp/azure"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/jessevdk/go-flags"
	"github.com/olekukonko/tablewriter"
	"github.com/op/go-logging"
)

// Appname is set during build
var Appname string

// AppCtx contains common top-level options and state
type AppCtx struct {
	DebugAPI    bool `long:"debug" description:"Enable debug of the API"`
	Csp         csp.CloudServiceProvider
	Azure       *azure.CSP
	AzureClient *azure.Client
	Emitter
}

// CredFlags contain flags required for authentication
type CredFlags struct {
	ClientID     string `short:"u" long:"client-id" env:"AZURE_CLIENT_ID" required:"yes" description:"The service principal identifier"`
	ClientSecret string `short:"p" long:"client-secret" env:"AZURE_CLIENT_SECRET" required:"yes" description:"The service principal password"`
	TenantID     string `short:"t" long:"tenant" env:"AZURE_TENANT_ID" required:"yes" description:"The directory identifier"`
}

// ProcessCredFlags processes credential flags and sets the values in the attrs map.
func (f *CredFlags) ProcessCredFlags(attrs map[string]models.ValueType) {
	attrs[azure.AttrClientID] = models.ValueType{Kind: "STRING", Value: f.ClientID}
	attrs[azure.AttrClientSecret] = models.ValueType{Kind: "SECRET", Value: f.ClientSecret}
	attrs[azure.AttrTenantID] = models.ValueType{Kind: "STRING", Value: f.TenantID}
	return
}

// ClientFlags are the flags required for the client object
type ClientFlags struct {
	CredFlags
	SubscriptionID string `short:"s" long:"subscription-id" env:"AZURE_SUBSCRIPTION_ID" required:"yes" description:"The subscription identifier"`
	Location       string `short:"l" long:"location" env:"AZURE_LOCATION" description:"The location (optional)"`
	Zone           string `short:"z" long:"zone" env:"AZURE_ZONE" description:"The zone (optional)"`

	cachedAuthorizer autorest.Authorizer
}

// ProcessClientFlags processes client flags and sets the values in the attrs map.
func (f *ClientFlags) ProcessClientFlags(attrs map[string]models.ValueType) {
	f.ProcessCredFlags(attrs)
	attrs[azure.AttrSubscriptionID] = models.ValueType{Kind: "STRING", Value: f.SubscriptionID}
	if f.Location != "" {
		attrs[azure.AttrLocation] = models.ValueType{Kind: "STRING", Value: f.Location}
	}
	if f.Zone != "" {
		attrs[azure.AttrZone] = models.ValueType{Kind: "STRING", Value: f.Zone}
	}
}

// GetAuthorizer returns an authorizer
func (f *ClientFlags) GetAuthorizer() (autorest.Authorizer, error) {
	if f.cachedAuthorizer != nil {
		return f.cachedAuthorizer, nil
	}
	attrs := make(map[string]models.ValueType)
	f.ProcessClientFlags(attrs)
	ccc, err := appCtx.Azure.AzGetClientCredentialsConfig(attrs, true)
	if err == nil {
		f.cachedAuthorizer, err = ccc.Authorizer()
	}
	return f.cachedAuthorizer, err
}

// DisksClient returns its namesake
func (f *ClientFlags) DisksClient() (compute.DisksClient, error) {
	a, err := f.GetAuthorizer()
	if err == nil {
		cl := compute.NewDisksClient(f.SubscriptionID)
		cl.Authorizer = a
		return cl, nil
	}
	return compute.DisksClient{}, err
}

// GroupsClient returns its namesake
func (f *ClientFlags) GroupsClient() (resources.GroupsClient, error) {
	a, err := f.GetAuthorizer()
	if err == nil {
		cl := resources.NewGroupsClient(f.SubscriptionID)
		cl.Authorizer = a
		return cl, nil
	}
	return resources.GroupsClient{}, err
}

// VirtualMachinesClient returns its namesake
func (f *ClientFlags) VirtualMachinesClient() (compute.VirtualMachinesClient, error) {
	a, err := f.GetAuthorizer()
	if err == nil {
		cl := compute.NewVirtualMachinesClient(f.SubscriptionID)
		cl.Authorizer = a
		return cl, nil
	}
	return compute.VirtualMachinesClient{}, err
}

// CSPClientFlags are required for CSP access
type CSPClientFlags struct {
	ClientFlags
	GroupName      string `short:"g" long:"group" env:"AZURE_RESOURCE_GROUP" required:"yes" description:"The name of the group"`
	StorageAccount string `short:"A" long:"storage-account" env:"AZURE_STORAGE_ACCOUNT" required:"yes" description:"The storage account"`
}

// ProcessCSPClientFlags creates a CSP client
func (f *CSPClientFlags) ProcessCSPClientFlags() error {
	attrs := make(map[string]models.ValueType)
	attrs[azure.AttrResourceGroupName] = models.ValueType{Kind: "STRING", Value: f.GroupName}
	attrs[azure.AttrStorageAccountName] = models.ValueType{Kind: "STRING", Value: f.StorageAccount}
	f.ProcessClientFlags(attrs)
	if err := appCtx.ClientInit(attrs); err != nil {
		return err
	}
	return nil
}

// CSPInit gets the CSP object
func (ac *AppCtx) CSPInit() {
	// initialize the CSP
	if ac.Csp == nil {
		c, err := csp.NewCloudServiceProvider(azure.CSPDomainType)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
			os.Exit(1)
		}
		ac.Csp = c
		azureCsp, ok := c.(*azure.CSP)
		if !ok {
			fmt.Fprintf(os.Stderr, "Error: AWS csp conversion failed\n")
			os.Exit(1)
		}
		ac.Azure = azureCsp
		if ac.DebugAPI {
			log := logging.MustGetLogger("example")
			ac.Azure.SetDebugLogger(log)
		}
	}
}

// parseColumns parses a list of column names.
// - it validates names in a case insensitive manner.  All column names are
//   assumed to be distinct in a case insensitive manner.
// - specified column names replace the default unless the default is implied (see below)
//   in which case the specified columns are added in their relative position to the
//   default columns
// - it accepts "+name" or "-name" as adjustments to the default columns which are
//   assumed to be appended at the end.
// - it accepts '*' as the position of the default columns
// - it skips empty column specifications (i.e. a comma without a leading token)
// - leading and trailing space in a column name is trimmed for matching purposes
func (ac *AppCtx) parseColumns(colString string, validCols []string, defaultCols []string) ([]string, error) {
	lcCols := map[string]string{}
	for _, c := range validCols {
		lc := strings.ToLower(c)
		lc = strings.TrimSpace(lc)
		lcCols[lc] = c
	}
	mustAddDefaults := false
	beforeDefault := []string{}
	afterDefault := []string{}
	newCols := &beforeDefault
	removeDefault := map[string]struct{}{}
	seenStar := false
	for _, token := range strings.Split(colString, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if token == "*" {
			if seenStar {
				return nil, fmt.Errorf("duplicate use of *")
			}
			seenStar = true
			newCols = &afterDefault
			mustAddDefaults = true
			continue
		}
		mustRemove := false
		if strings.HasPrefix(token, "+") {
			mustAddDefaults = true
			token = strings.TrimPrefix(token, "+")
		} else if strings.HasPrefix(token, "-") {
			mustAddDefaults = true
			token = strings.TrimPrefix(token, "-")
			mustRemove = true
		}
		if c, ok := lcCols[strings.ToLower(token)]; ok {
			if mustRemove {
				removeDefault[c] = struct{}{}
			} else {
				*newCols = append(*newCols, c)
			}
		} else {
			return nil, fmt.Errorf("invalid column \"%s\"", token)
		}
	}
	cols := beforeDefault
	if mustAddDefaults {
		for _, c := range defaultCols {
			if _, has := removeDefault[c]; has {
				continue
			}
			if !util.Contains(cols, c) {
				cols = append(cols, c)
			}
		}
	}
	for _, c := range afterDefault {
		if !util.Contains(cols, c) {
			cols = append(cols, c)
		}
	}
	if len(cols) == 0 {
		cols = defaultCols
	}
	return cols, nil
}

var appCtx = &AppCtx{}
var parser = flags.NewParser(appCtx, flags.Default&^flags.PrintErrors)
var outputWriter io.Writer

func init() {
	outputWriter = os.Stdout
	initParser()
}

func initParser() {
	parser.ShortDescription = Appname
	parser.Usage = "[Application Options]"
	parser.LongDescription = "Nuvoloso development tool to exercise aspects of the AWS SDK"
	parser.AddCommand("validate-cred", "Validate credential", "Check that the credentials are valid.", &validateCredentialCmd{})
	parser.AddCommand("validate-client", "Validate client", "Check that the client is valid.", &validateClientCmd{})
}

func commandHandler(command flags.Commander, args []string) error {
	if command == nil {
		return nil
	}
	appCtx.CSPInit()
	return command.Execute(args)
}

func parseAndRun(args []string) error {
	parser.CommandHandler = commandHandler
	_, err := parser.ParseArgs(args)
	if err != nil {
		if e, ok := err.(*flags.Error); ok && e.Type == flags.ErrHelp {
			fmt.Fprint(outputWriter, err.Error())
			return nil
		}
		return fmt.Errorf("%s", err.Error())
	}
	return nil
}

func main() {
	appCtx.Emitter = &StdoutEmitter{}
	if err := parseAndRun(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s.\n", err.Error())
		os.Exit(1)
	}
	os.Exit(0)
}

type showColsCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`

	columns map[string]string
}

// Execute provides a common implementation to output table columns
func (c *showColsCmd) Execute(args []string) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.EmitJSON(c.columns)
	case "yaml":
		return appCtx.EmitYAML(c.columns)
	}
	cols := make([]string, 0, len(c.columns))
	for k := range c.columns {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	rows := make([][]string, len(cols))
	dLen := 0
	for i, col := range cols {
		row := make([]string, 2)
		row[0] = col
		d := c.columns[col]
		row[1] = d
		rows[i] = row
		if len(d) > dLen {
			dLen = len(d)
		}
	}
	cT := func(t *tablewriter.Table) {
		t.SetColWidth(dLen)
	}
	appCtx.EmitTable([]string{"Column Name", "Description"}, rows, cT)
	return nil
}

type validateCredentialCmd struct {
	CredFlags
}

func (c *validateCredentialCmd) Execute(args []string) error {
	attrs := make(map[string]models.ValueType)
	c.ProcessCredFlags(attrs)
	return appCtx.Azure.ValidateCredential(azure.CSPDomainType, attrs)
}

// ClientInit sets the Azure credential attributes
func (ac *AppCtx) ClientInit(attrs map[string]models.ValueType) error {
	// fake a CSPDomain object
	dObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID: "fakeDomain",
			},
			CspDomainType:       azure.CSPDomainType,
			CspDomainAttributes: attrs,
		},
	}
	if cl, err := ac.Azure.Client(dObj); err == nil {
		azureCl, ok := cl.(*azure.Client)
		if !ok {
			return fmt.Errorf("Azure client conversion failed")
		}
		ac.AzureClient = azureCl
	} else {
		return err
	}
	return nil
}

type validateClientCmd struct {
	ClientFlags
}

func (c *validateClientCmd) Execute(args []string) error {
	attrs := make(map[string]models.ValueType)
	c.ProcessClientFlags(attrs)
	if err := appCtx.ClientInit(attrs); err != nil {
		return err
	}
	if err := appCtx.AzureClient.Validate(nil); err != nil {
		return fmt.Errorf("Azure client validate failed: %s", err.Error())
	}
	fmt.Println("Credentials appear to be valid")
	return nil
}
