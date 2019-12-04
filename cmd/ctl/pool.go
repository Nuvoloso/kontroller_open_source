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
	"strings"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/pool"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
)

func init() {
	initPool()
}

func initPool() {
	cmd, _ := parser.AddCommand("pool", "Pool object commands", "Pool object subcommands", &poolCmd{})
	cmd.Aliases = []string{"pools"}
	cmd.AddCommand("list", "List Pools", "List or search for Pool objects.", &poolListCmd{})
	cmd.AddCommand("get", "Get a Pool", "Get a Pool object", &poolFetchCmd{})
	cmd.AddCommand("show-columns", "Show Pool table columns", "Show names of columns used in table format", &showColsCmd{columns: poolHeaders})
}

type poolCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string

	cacheHelper
}

// pool record keys/headers and their description
var poolHeaders = map[string]string{
	hID:                   dID,
	hTags:                 dTags,
	hCspDomain:            dCspDomain, // pseudo-column
	hCSPStorageType:       dCspStorageType,
	hAccessibilityScope:   dAccessibilityScope,
	hAccessibilityScopeID: "The accessibility scope object ID.",
	hAccount:              dAccount,
	hAuthorizedAccount:    "The authorized account",
	hCluster:              "The associated cluster",
}

var poolDefaultHeaders = []string{hID, hCspDomain, hCluster, hAuthorizedAccount, hCSPStorageType}

// makeRecord creates a map of properties
func (c *poolCmd) makeRecord(o *models.Pool) map[string]string {
	cspDomain := string(o.CspDomainID)
	if cn, ok := c.cspDomains[cspDomain]; ok { // assumes map present
		cspDomain = cn
	}
	clusterName := string(o.ClusterID)
	if cln, ok := c.clusters[clusterName]; ok { // assumes map present
		clusterName = cln.name
	}
	accountName := string(o.AccountID)
	if an, ok := c.accounts[accountName]; ok { // assumes map present
		accountName = an.name
	}
	authAccountName := string(o.AuthorizedAccountID)
	if an, ok := c.accounts[authAccountName]; ok { // assumes map present
		authAccountName = an.name
	}
	return map[string]string{
		hID:                   string(o.Meta.ID),
		hSystemTags:           strings.Join(o.SystemTags, ", "),
		hAccessibilityScope:   string(o.StorageAccessibility.AccessibilityScope),
		hAccessibilityScopeID: string(o.StorageAccessibility.AccessibilityScopeObjID),
		hCSPStorageType:       o.CspStorageType,
		hCspDomain:            cspDomain,
		hCluster:              clusterName,
		hAccount:              accountName,
		hAuthorizedAccount:    authAccountName,
	}
}

func (c *poolCmd) validateColumns(columns string) error {
	var err error
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(poolHeaders), poolDefaultHeaders)
	return err
}

func (c *poolCmd) Emit(data []*models.Pool) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.EmitJSON(data)
	case "yaml":
		return appCtx.EmitYAML(data)
	}
	c.cacheAccounts()
	c.cacheCSPDomains()
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

func (c *poolCmd) list(params *pool.PoolListParams) ([]*models.Pool, error) {
	res, err := appCtx.API.Pool().PoolList(params)
	if err != nil {
		if e, ok := err.(*pool.PoolListDefault); ok && e.Payload.Message != nil {
			return nil, fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res.Payload, nil
}

type poolListCmd struct {
	DomainName              string `short:"D" long:"domain" description:"Name of a cloud service provider domain"`
	ClusterName             string `short:"C" long:"cluster-name" description:"Name of a cluster"`
	AuthorizedAccountName   string `short:"Z" long:"authorized-account" description:"Name of the authorized account"`
	OwnerAuthorized         bool   `long:"owner-auth" description:"Match a pool where the owner (the context account) is also the authorized account"`
	StorageType             string `short:"T" long:"storage-type" description:"The CSP name for a type of storage"`
	AccessibilityScope      string `short:"a" long:"scope" description:"The accessibility scope." choice:"NODE" choice:"CSPDOMAIN"`
	AccessibilityScopeObjID string `long:"scope-object-id" description:"ID of the accessibility scope object."`
	Columns                 string `short:"c" long:"columns" description:"Comma separated list of column names"`
	Follow                  bool   `short:"f" long:"follow-changes" description:"Monitor the system and repeat the command as relevant changes occur."`
	FollowNoClear           bool   `long:"follow-no-clear" description:"Do not clear the terminal (the default) when using follow-changes"`

	mustClearTerm bool
	params        *pool.PoolListParams
	runCount      int
	lastLen       int
	poolCmd
	remainingArgsCatcher
}

func (c *poolListCmd) Execute(args []string) error {
	var err error
	if err = c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	if c.ClusterName != "" && c.DomainName == "" {
		return fmt.Errorf("cluster-name requires domain")
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	if c.AuthorizedAccountName != "" && c.OwnerAuthorized {
		return fmt.Errorf("do not specify --authorized-account and --owner-auth together")
	}
	if c.OwnerAuthorized && appCtx.Account == "" {
		return fmt.Errorf("--owner-auth requires --account")
	}
	if err = c.cacheCSPDomains(); err == nil {
		err = c.cacheClusters()
	}
	if err != nil {
		return err
	}
	params := pool.NewPoolListParams()
	var domainID, clusterID string
	if c.DomainName != "" {
		for id, n := range c.cspDomains {
			if n == c.DomainName {
				domainID = id
			}
		}
		if domainID == "" {
			return fmt.Errorf("CSPDomain '%s' not found", c.DomainName)
		}
		params.CspDomainID = swag.String(domainID)
	}
	if c.ClusterName != "" {
		for id, cn := range c.clusters {
			if cn.id == string(domainID) && cn.name == c.ClusterName {
				clusterID = id
			}
		}
		if clusterID == "" {
			return fmt.Errorf("cluster '%s' not found in domain '%s'", c.ClusterName, c.DomainName)
		}
		params.ClusterID = swag.String(clusterID)
	}
	if c.StorageType != "" {
		params.CspStorageType = &c.StorageType
	}
	if c.AuthorizedAccountName != "" {
		id, err := c.validateAccount(c.AuthorizedAccountName, "authorized")
		if err != nil {
			return err
		}
		params.AuthorizedAccountID = swag.String(id)
	} else if c.OwnerAuthorized {
		params.AuthorizedAccountID = &appCtx.AccountID
	}
	if c.AccessibilityScope != "" {
		params.AccessibilityScope = &c.AccessibilityScope
	}
	if c.AccessibilityScopeObjID != "" {
		params.AccessibilityScopeObjID = &c.AccessibilityScopeObjID
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

var poolListCmdRunCacheThreshold = 1

func (c *poolListCmd) run() error {
	var err error
	var res []*models.Pool
	if res, err = c.list(c.params); err != nil {
		return err
	}
	if c.mustClearTerm {
		appCtx.ClearTerminal()
	}
	if c.runCount > poolListCmdRunCacheThreshold && c.lastLen < len(res) {
		// refresh caches when new objects added
		c.cspDomains = nil
		c.clusters = nil
		c.accounts = nil
	}
	c.lastLen = len(res)
	return c.Emit(res)
}

// ChangeDetected is part of the ChangeDetector interface
func (c *poolListCmd) ChangeDetected() error {
	c.runCount++
	return c.run()
}

// WatcherArgs is part of the ChangeDetector interface
func (c *poolListCmd) WatcherArgs() *models.CrudWatcherCreateArgs {
	cm := &models.CrudMatcher{
		URIPattern: "/pools/?",
	}
	var scopeB bytes.Buffer
	if c.params.AuthorizedAccountID != nil {
		fmt.Fprintf(&scopeB, ".*authorizedAccountId:%s", *c.params.AuthorizedAccountID)
	}
	if c.params.ClusterID != nil {
		fmt.Fprintf(&scopeB, ".*clusterId:%s", *c.params.ClusterID)
	}
	if c.params.CspDomainID != nil {
		fmt.Fprintf(&scopeB, ".*cspDomainId:%s", *c.params.CspDomainID)
	}
	if scopeB.Len() > 0 {
		cm.ScopePattern = scopeB.String()
	}
	return &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{cm},
	}
}

type poolFetchCmd struct {
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`

	poolCmd
	requiredIDRemainingArgsCatcher
}

func (c *poolFetchCmd) Execute(args []string) error {
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
	fParams := pool.NewPoolFetchParams()
	fParams.ID = string(c.ID)
	res, err := appCtx.API.Pool().PoolFetch(fParams)
	if err != nil {
		if e, ok := err.(*pool.PoolFetchDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.Pool{res.Payload})
}
