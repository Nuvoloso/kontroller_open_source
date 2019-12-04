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
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/cluster"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
	"github.com/olekukonko/tablewriter"
)

func init() {
	initCluster()
}

func initCluster() {
	cmd, _ := parser.AddCommand("cluster", "Cluster object commands", "Cluster object subcommands", &clusterCmd{})
	cmd.Aliases = []string{"clusters"}
	cmd.AddCommand("create", "Create a Cluster", "Create a Cluster object", &clusterCreateCmd{})
	cmd.AddCommand("get", "Get a Cluster", "Get a Cluster object.", &clusterFetchCmd{})
	cmd.AddCommand("list", "List Clusters", "List or search for Cluster objects.", &clusterListCmd{})
	cmd.AddCommand("delete", "Delete a Cluster", "Delete a Cluster object.", &clusterDeleteCmd{})
	cmd.AddCommand("modify", "Modify a Cluster", "Modify a Cluster object.  Use a whitespace-only string to clear trimmed properties.", &clusterModifyCmd{})
	cmd.AddCommand("get-secret", "Download an account secret", "Download an account secret for a cluster.", &clusterSecretFetchCmd{})
	cmd.AddCommand("show-columns", "Show Cluster table columns", "Show names of columns used in table format.", &showColsCmd{columns: clusterHeaders})
	cmd.AddCommand("get-deployment", "Managed cluster deployment", "Download the deployment configuration to a manage a cluster", &clusterGetDeploymentCmd{})
}

type clusterCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string

	cacheHelper
}

// cluster record keys/headers and their description
var clusterHeaders = map[string]string{
	hID:                 dID,
	hTimeCreated:        dTimeCreated,
	hTimeModified:       dTimeModified,
	hVersion:            dVersion,
	hName:               dName + " It is unique within the CSP domain.",
	hDescription:        dDescription,
	hAccount:            dAccount,
	hAuthorizedAccounts: dAuthorizedAccounts,
	hTags:               dTags,
	hClusterAttributes:  "Attributes describing the cluster.",
	hClusterIdentifier:  dClusterIdentifier,
	hClusterType:        "The type of cluster.",
	hCspDomain:          dCspDomain, // pseudo-column
	hClusterVersion:     "The version of cluster software.",
	hControllerState:    dControllerState,                  // pseudo-column
	hServiceIP:          dServiceIP,                        // pseudo-column
	hServiceLocator:     dServiceLocator,                   // pseudo-column
	hResourceState:      "The state of cluster resources.", // pseudo-column
	hClusterState:       "The cluster state",               // pseudo-column
}

var clusterDefaultHeaders = []string{hName, hCspDomain, hAuthorizedAccounts, hClusterState, hResourceState}

// makeRecord creates a map of properties
func (c *clusterCmd) makeRecord(o *models.Cluster) map[string]string {
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
	cspDomain, ok := c.cspDomains[string(o.CspDomainID)]
	if !ok {
		cspDomain = string(o.CspDomainID)
	}
	attrs := make([]string, 0, len(o.ClusterAttributes))
	for a := range o.ClusterAttributes {
		attrs = append(attrs, a)
	}
	sort.Strings(attrs)
	attrList := []string{}
	for _, a := range attrs {
		v := o.ClusterAttributes[a]
		t := ""
		if len(v.Kind) > 0 {
			t = v.Kind[0:1]
		}
		s := a + "[" + t + "]: " + v.Value
		attrList = append(attrList, s)
	}
	controllerState := "UNKNOWN"
	serviceIP := ""
	serviceLocator := ""
	opState := ""
	if o.Service != nil {
		serviceIP = o.Service.ServiceIP
		serviceLocator = o.Service.ServiceLocator
		if o.Service.State != "" {
			controllerState = o.Service.State
		}
		if vt, found := o.Service.ServiceAttributes[common.ServiceAttrClusterResourceState]; found {
			opState = vt.Value
		}
	}
	clusterState := o.State
	if o.State == common.ClusterStateManaged {
		clusterState = fmt.Sprintf("%s/%s", o.State, controllerState)
	}
	return map[string]string{
		hID:                 string(o.Meta.ID),
		hTimeCreated:        time.Time(o.Meta.TimeCreated).Format(time.RFC3339),
		hTimeModified:       time.Time(o.Meta.TimeModified).Format(time.RFC3339),
		hVersion:            fmt.Sprintf("%d", o.Meta.Version),
		hName:               string(o.Name),
		hDescription:        string(o.Description),
		hAccount:            string(account),
		hAuthorizedAccounts: strings.Join(authorized, ", "),
		hTags:               strings.Join(o.Tags, ", "),
		hClusterVersion:     o.ClusterVersion,
		hControllerState:    controllerState,
		hClusterAttributes:  strings.Join(attrList, "\n"),
		hClusterIdentifier:  o.ClusterIdentifier,
		hClusterType:        o.ClusterType,
		hCspDomain:          cspDomain,
		hServiceIP:          serviceIP,
		hServiceLocator:     serviceLocator,
		hResourceState:      opState,
		hClusterState:       clusterState,
	}
}

func (c *clusterCmd) validateColumns(columns string) (err error) {
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(clusterHeaders), clusterDefaultHeaders)
	return err
}

func (c *clusterCmd) Emit(data []*models.Cluster) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.EmitJSON(data)
	case "yaml":
		return appCtx.EmitYAML(data)
	}
	c.cacheAccounts()
	c.cacheCSPDomains()
	// We need to insert pseudo-rows to properly display cluster state
	csCol := -1
	csWidth := 0
	max := func(x, y int) int {
		if x > y {
			return x
		}
		return y
	}
	for i, cn := range c.tableCols {
		if cn == hResourceState {
			csCol = i
			break
		}
	}
	reEmpty := regexp.MustCompile("^\\s*$")
	reStorage := regexp.MustCompile("^\\s*Storage")
	reStorageFieldSep := regexp.MustCompile("\\s+\\S+:")
	rows := make([][]string, 0, len(data)) // min length
	for _, o := range data {
		rec := c.makeRecord(o)
		row := make([]string, len(c.tableCols))
		for j, h := range c.tableCols {
			row[j] = rec[h]
		}
		if csCol != -1 {
			cs := row[csCol]
			lines := strings.Split(cs, "\n")
			lastL := len(lines) - 2
			for n, l := range lines {
				if reEmpty.MatchString(l) {
					continue
				}
				if reStorage.MatchString(l) {
					f := reStorageFieldSep.Split(l, -1)
					l = strings.Join([]string{f[0], f[2], f[11]}, " ")
				}
				csWidth = max(csWidth, len(l))
				if n > 0 { // skip the first
					row[csCol] = l
					if n < lastL {
						rows = append(rows, row)
						row = make([]string, len(c.tableCols))
					}
				}
			}
		}
		rows = append(rows, row)
	}
	cT := func(t *tablewriter.Table) {
		if csCol != -1 {
			t.SetColMinWidth(csCol, csWidth)
		}
	}
	return appCtx.EmitTable(c.tableCols, rows, cT)
}

func (c *clusterCmd) list(params *cluster.ClusterListParams) ([]*models.Cluster, error) {
	if params == nil {
		params = cluster.NewClusterListParams()
	}
	res, err := appCtx.API.Cluster().ClusterList(params)
	if err != nil {
		if e, ok := err.(*cluster.ClusterListDefault); ok && e.Payload.Message != nil {
			return nil, fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res.Payload, nil
}

type clusterCreateCmd struct {
	ClusterType        string   `short:"T" long:"cluster-type" description:"A kind of cluster software" default:"kubernetes"`
	DomainName         string   `short:"D" long:"domain-name" description:"Name of a cloud service provider domain" required:"yes"`
	AuthorizedAccounts []string `short:"Z" long:"authorized-account" description:"Name of an authorized account. Repeat as needed"`
	OwnerAuthorized    bool     `long:"owner-auth" description:"Authorize the owner (the context account)"`
	Description        string   `short:"d" long:"description" description:"The new description; leading and trailing whitespace will be stripped"`
	Name               string   `short:"n" long:"name" description:"A Cluster name within the specified cloud service provider domain" required:"yes"`
	Tags               []string `short:"t" long:"tag" description:"A tag value; repeat as needed"`
	Columns            string   `short:"c" long:"columns" description:"Comma separated list of column names"`

	clusterCmd
	remainingArgsCatcher
}

func (c *clusterCreateCmd) Execute(args []string) error {
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
	params := cluster.NewClusterCreateParams()
	params.Payload = &models.ClusterCreateArgs{}
	params.Payload.ClusterType = c.ClusterType
	if c.DomainName != "" {
		if err := c.cacheCSPDomains(); err != nil {
			return err
		}
		for i, n := range c.cspDomains {
			if n == c.DomainName {
				params.Payload.CspDomainID = models.ObjIDMutable(i)
				break
			}
		}
		if params.Payload.CspDomainID == "" {
			return fmt.Errorf("CSP domain \"%s\" not found", c.DomainName)
		}
	}
	if c.OwnerAuthorized {
		params.Payload.AuthorizedAccounts = append(params.Payload.AuthorizedAccounts, models.ObjIDMutable(appCtx.AccountID))
	}
	if len(c.AuthorizedAccounts) != 0 {
		for _, account := range c.AuthorizedAccounts {
			id, err := c.validateAccount(account, "authorized")
			if err != nil {
				return err
			}
			params.Payload.AuthorizedAccounts = append(params.Payload.AuthorizedAccounts, models.ObjIDMutable(id))
		}
	}
	params.Payload.Description = models.ObjDescription(c.Description)
	params.Payload.Name = models.ObjName(c.Name)
	params.Payload.Tags = c.Tags
	res, err := appCtx.API.Cluster().ClusterCreate(params)
	if err != nil {
		if e, ok := err.(*cluster.ClusterCreateDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.Cluster{res.Payload})
}

type clusterListCmd struct {
	Name              string         `short:"n" long:"name" description:"A Cluster object name"`
	AuthorizedAccount string         `short:"Z" long:"authorized-account" description:"Name of an authorized subordinate account"`
	OwnerAuthorized   bool           `long:"owner-auth" description:"Match a Cluster where the owner (the context account) is also the authorized account"`
	Tags              []string       `short:"t" long:"tag" description:"A tag value; repeat as needed"`
	DomainName        string         `short:"D" long:"domain-name" description:"Name of a cloud service provider domain"`
	ClusterIdentifier string         `short:"I" long:"cluster-identifier" description:"The unique identifier of the cluster"`
	SvcHbTimeGE       util.Timestamp `long:"svc-hb-time-ge" description:"Lower bound of service heartbeat time to list, specified either as a duration or an absolute RFC3339 time"`
	SvcHbTimeLE       util.Timestamp `long:"svc-hb-time-le" description:"Upper bound of service heartbeat time to list, specified either as a duration or an absolute RFC3339 time"`
	SvcStateEQ        string         `long:"svc-state-eq" description:"Service state to match"`
	SvcStateNE        string         `long:"svc-state-ne" description:"Service state to not match. Overrides the svc-state-eq flag"`
	Columns           string         `short:"c" long:"columns" description:"Comma separated list of column names"`
	Follow            bool           `short:"f" long:"follow-changes" description:"Monitor the system and repeat the command as relevant changes occur."`
	FollowNoClear     bool           `long:"follow-no-clear" description:"Do not clear the terminal (the default) when using follow-changes"`

	mustClearTerm bool
	params        *cluster.ClusterListParams
	runCount      int
	lastLen       int
	clusterCmd
	remainingArgsCatcher
}

func (c *clusterListCmd) Execute(args []string) error {
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
	params := cluster.NewClusterListParams()
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
	params.ClusterIdentifier = &c.ClusterIdentifier
	if c.DomainName != "" {
		if err := c.cacheCSPDomains(); err != nil {
			return err
		}
		for i, n := range c.cspDomains {
			if n == c.DomainName {
				params.CspDomainID = swag.String(i)
				break
			}
		}
		if params.CspDomainID == nil {
			return fmt.Errorf("CSP domain \"%s\" not found", c.DomainName)
		}
	}
	if c.SvcHbTimeGE.Specified() {
		dt := c.SvcHbTimeGE.Value()
		params.ServiceHeartbeatTimeGE = &dt
	}
	if c.SvcHbTimeLE.Specified() {
		dt := c.SvcHbTimeLE.Value()
		params.ServiceHeartbeatTimeLE = &dt
	}
	if c.SvcStateEQ != "" {
		params.ServiceStateEQ = &c.SvcStateEQ
	}
	if c.SvcStateNE != "" {
		params.ServiceStateNE = &c.SvcStateNE
	}
	c.params = params
	if c.Follow {
		if !c.FollowNoClear {
			c.mustClearTerm = true
		}
		return appCtx.WatchForChange(c)
	}
	return c.run()
}

var clusterListCmdRunCacheThreshold = 1

func (c *clusterListCmd) run() error {
	var err error
	var res []*models.Cluster
	if res, err = c.list(c.params); err != nil {
		return err
	}
	if c.mustClearTerm {
		appCtx.ClearTerminal()
	}
	if c.runCount > clusterListCmdRunCacheThreshold && c.lastLen < len(res) {
		// caches could get stale if new objects added
		c.cspDomains = nil
	}
	c.lastLen = len(res)
	return c.Emit(res)
}

// ChangeDetected is part of the ChangeDetector interface
func (c *clusterListCmd) ChangeDetected() error {
	c.runCount++
	return c.run()
}

// WatcherArgs is part of the ChangeDetector interface
func (c *clusterListCmd) WatcherArgs() *models.CrudWatcherCreateArgs {
	cm := &models.CrudMatcher{
		URIPattern: "/clusters/?",
	}
	// there are no scope properties currently defined
	return &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{cm},
	}
}

type clusterFetchCmd struct {
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`

	clusterCmd
	requiredIDRemainingArgsCatcher
}

func (c *clusterFetchCmd) Execute(args []string) error {
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
	fParams := cluster.NewClusterFetchParams().WithID(c.ID)
	res, err := appCtx.API.Cluster().ClusterFetch(fParams)
	if err != nil {
		if e, ok := err.(*cluster.ClusterFetchDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.Cluster{res.Payload})
}

type clusterDeleteCmd struct {
	Name       string `short:"n" long:"name" description:"A Cluster name within the specified cloud service provider domain" required:"yes"`
	DomainName string `short:"D" long:"domain-name" description:"Name of a cloud service provider domain" required:"yes"`
	Confirm    bool   `long:"confirm" description:"Confirm the deletion of the object"`

	clusterCmd
	remainingArgsCatcher
}

func (c *clusterDeleteCmd) Execute(args []string) error {
	if err := c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if !c.Confirm {
		return fmt.Errorf("specify --confirm to delete the \"%s\" Cluster object", c.Name)
	}
	var err error
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	if err = c.cacheCSPDomains(); err != nil {
		return err
	}
	lParams := cluster.NewClusterListParams()
	for i, n := range c.cspDomains {
		if n == c.DomainName {
			lParams.CspDomainID = swag.String(i)
			break
		}
	}
	if lParams.CspDomainID == nil {
		return fmt.Errorf("CSP domain \"%s\" not found", c.DomainName)
	}
	lParams.Name = &c.Name
	var lRes []*models.Cluster
	if lRes, err = c.list(lParams); err != nil {
		return err
	}
	if len(lRes) != 1 {
		return fmt.Errorf("Cluster object \"%s\" not found", c.Name)
	}
	dParams := cluster.NewClusterDeleteParams()
	dParams.ID = string(lRes[0].Meta.ID)
	if _, err = appCtx.API.Cluster().ClusterDelete(dParams); err != nil {
		if e, ok := err.(*cluster.ClusterDeleteDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return nil
}

type clusterModifyCmd struct {
	Name                     string   `short:"n" long:"name" description:"A Cluster name within the specified cloud service provider domain" required:"yes"`
	DomainName               string   `short:"D" long:"domain-name" description:"Name of a cloud service provider domain" required:"yes"`
	NewName                  string   `short:"N" long:"new-name" description:"The new name for the object"`
	Description              string   `short:"d" long:"description" description:"The new description; leading and trailing whitespace will be stripped"`
	AuthorizedAccounts       []string `short:"Z" long:"authorized-account" description:"Name of an authorized account. Repeat as needed. Subject to the value of authorized-accounts-action"`
	AuthorizedAccountsAction string   `long:"authorized-accounts-action" description:"Specifies how to process authorized-account values" choice:"APPEND" choice:"REMOVE" choice:"SET" default:"APPEND"`
	OwnerAuthorized          bool     `long:"owner-auth" description:"Authorize the owner (the context account). Subject to the value of authorized-accounts-action"`
	CUPFlagsWithInheritance
	Tags       []string `short:"t" long:"tag" description:"A tag value; repeat as needed"`
	TagsAction string   `long:"tag-action" description:"Specifies how to process tag values." choice:"APPEND" choice:"REMOVE" choice:"SET" default:"APPEND"`
	Version    int32    `short:"V" long:"version" description:"Enforce update of the specified version of the object"`
	Columns    string   `short:"c" long:"columns" description:"Comma separated list of column names"`

	clusterCmd
	remainingArgsCatcher
}

func (c *clusterModifyCmd) Execute(args []string) error {
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
	if err = c.CupValidateInheritableFlags(); err != nil {
		return err
	}
	lParams := cluster.NewClusterListParams()
	if err = c.cacheCSPDomains(); err != nil {
		return err
	}
	for i, n := range c.cspDomains {
		if n == c.DomainName {
			lParams.CspDomainID = swag.String(i)
			break
		}
	}
	if lParams.CspDomainID == nil {
		return fmt.Errorf("CSP domain \"%s\" not found", c.DomainName)
	}
	lParams.Name = &c.Name
	var lRes []*models.Cluster
	if lRes, err = c.list(lParams); err != nil {
		return err
	}
	if len(lRes) != 1 {
		return fmt.Errorf("Cluster object \"%s\" not found", c.Name)
	}

	nChg := 0
	uParams := cluster.NewClusterUpdateParams().WithPayload(&models.ClusterMutable{})
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
	if nChg == 0 {
		return fmt.Errorf("No modifications specified")
	}
	uParams.ID = string(lRes[0].Meta.ID)
	if c.Version != 0 {
		uParams.Version = &c.Version
	}
	var res *cluster.ClusterUpdateOK
	if res, err = appCtx.API.Cluster().ClusterUpdate(uParams); err != nil {
		if e, ok := err.(*cluster.ClusterUpdateDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.Cluster{res.Payload})
}

type clusterSecretFetchCmd struct {
	Name              string `short:"n" long:"name" description:"A Cluster name within the specified cloud service provider domain" required:"yes"`
	DomainName        string `short:"D" long:"domain-name" description:"Name of a cloud service provider domain" required:"yes"`
	AuthorizedAccount string `short:"Z" long:"authorized-account" description:"Name of the authorized account for which the secret is to be downloaded if not the context account"`
	K8sNamespace      string `long:"k8s-namespace" description:"The secret object namespace if a Kubernetes cluster"`
	OutputFile        string `short:"O" long:"output-file" description:"The name of the output file. Use - for stdout" default:"account-secret.yaml"`

	clusterCmd
	remainingArgsCatcher
}

func (c *clusterSecretFetchCmd) Execute(args []string) error {
	var err error
	if err = c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	var oF io.Writer
	if c.OutputFile == "-" {
		oF = outputWriter
	} else {
		f, err := os.Create(c.OutputFile)
		if err != nil {
			return err
		}
		defer f.Close()
		oF = f
	}
	lParams := cluster.NewClusterListParams()
	if err = c.cacheCSPDomains(); err != nil {
		return err
	}
	for i, n := range c.cspDomains {
		if n == c.DomainName {
			lParams.CspDomainID = swag.String(i)
			break
		}
	}
	if lParams.CspDomainID == nil {
		return fmt.Errorf("CSP domain \"%s\" not found", c.DomainName)
	}
	lParams.Name = &c.Name
	var lRes []*models.Cluster
	if lRes, err = c.list(lParams); err != nil {
		return err
	}
	if len(lRes) != 1 {
		return fmt.Errorf("Cluster object \"%s\" not found", c.Name)
	}
	var authAccountID string
	if c.AuthorizedAccount != "" {
		if authAccountID, err = c.validateAccount(c.AuthorizedAccount, "authorized"); err != nil {
			return err
		}
	} else {
		authAccountID = appCtx.AccountID
	}
	asfParams := cluster.NewClusterAccountSecretFetchParams()
	asfParams.ID = string(lRes[0].Meta.ID)
	asfParams.AuthorizedAccountID = authAccountID
	if c.K8sNamespace != "" {
		asfParams.K8sNamespace = swag.String(c.K8sNamespace)
	}
	var ret *cluster.ClusterAccountSecretFetchOK
	if ret, err = appCtx.API.Cluster().ClusterAccountSecretFetch(asfParams); err != nil {
		if e, ok := err.(*cluster.ClusterAccountSecretFetchDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	io.WriteString(oF, ret.Payload.Value)
	return nil
}

type clusterGetDeploymentCmd struct {
	Name                string `short:"n" long:"name" description:"A Cluster name within the specified cloud service provider domain" required:"yes"`
	DomainName          string `short:"D" long:"domain-name" description:"Name of a cloud service provider domain" required:"yes"`
	OutputFile          string `short:"O" long:"output-file" description:"The name of the output file. Use - for stdout" default:"nuvo-cluster.yaml"`
	OrchestratorType    string `short:"T" long:"orchestrator-type" description:"The type of orchestrator. Supported type is 'kubernetes'"`
	OrchestratorVersion string `short:"V" long:"orchestrator-version" description:"The version of the orchestrator. Default used for Kubernetes is '1.14.6'"`

	clusterCmd
	remainingArgsCatcher
}

func (c *clusterGetDeploymentCmd) Execute(args []string) error {
	var err error
	if err = c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	var clusterID string
	_, clusterID, _, err = c.validateDomainClusterNodeNames(c.DomainName, c.Name, "") // that will also validate that cluster is part of given domain
	if err != nil {
		return err
	}
	params := cluster.NewClusterOrchestratorGetDeploymentParams().WithID(clusterID).WithOrchestratorType(swag.String(c.OrchestratorType)).WithOrchestratorVersion(swag.String(c.OrchestratorVersion))
	res, err := appCtx.API.Cluster().ClusterOrchestratorGetDeployment(params)
	if err != nil {
		if e, ok := err.(*cluster.ClusterOrchestratorGetDeploymentDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	var oF io.Writer
	if c.OutputFile == "-" {
		oF = outputWriter
	} else {
		f, err := os.Create(c.OutputFile)
		if err != nil {
			return err
		}
		defer f.Close()
		oF = f
	}
	io.WriteString(oF, res.Payload.Deployment)
	return nil
}
