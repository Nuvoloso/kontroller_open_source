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
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/node"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
	"github.com/olekukonko/tablewriter"
)

func init() {
	initNode()
}

func initNode() {
	cmd, _ := parser.AddCommand("node", "Node object commands", "Node object subcommands", &nodeCmd{})
	cmd.Aliases = []string{"nodes"}
	cmd.AddCommand("list", "List Nodes", "List or search for Node objects.", &nodeListCmd{})
	cmd.AddCommand("delete", "Delete a Node", "Delete a Node object.", &nodeDeleteCmd{})
	cmd.AddCommand("get", "Get a Node", "Get a Node object.", &nodeFetchCmd{})
	cmd.AddCommand("show-columns", "Show Node table columns", "Show names of columns used in table format.", &showColsCmd{columns: nodeHeaders})
}

type nodeCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string

	cacheHelper
}

// node record keys/headers and their description
var nodeHeaders = map[string]string{
	hID:              dID,
	hTimeCreated:     dTimeCreated,
	hTimeModified:    dTimeModified,
	hVersion:         dVersion,
	hName:            dName + " Node names are not unique across clusters.",
	hDescription:     dDescription,
	hAccount:         dAccount,
	hTags:            dTags,
	hNodeIdentifier:  dNodeIdentifier,
	hNodeAttributes:  "Attributes describing the node.",
	hTotalCache:      "Total cache size on node",
	hAvailableCache:  "Available cache size on node",
	hLocalStorage:    "Information on ephemeral, node-scoped storage devices",
	hCspDomain:       dCspDomain,       // pseudo-column
	hCluster:         dCluster,         // pseudo-column
	hControllerState: dControllerState, // pseudo-column
	hServiceIP:       dServiceIP,       // pseudo-column
	hServiceLocator:  dServiceLocator,  // pseudo-column
	hNodeState:       "The node state", // pseudo-column
}

var nodeDefaultHeaders = []string{hName, hNodeIdentifier, hCluster, hCspDomain, hServiceLocator, hNodeState}

// makeRecord creates a map of properties
func (c *nodeCmd) makeRecord(o *models.Node) map[string]string {
	var domainID string
	cluster := string(o.ClusterID)
	if cn, ok := c.clusters[cluster]; ok { // assumes map present
		cluster = cn.name
		domainID = cn.id
	}
	domain := domainID
	if dn, ok := c.cspDomains[domainID]; ok {
		domain = dn
	}
	account := string(o.AccountID)
	if a, ok := c.accounts[account]; ok {
		account = a.name
	}
	attrList := []string{}
	for _, a := range util.SortedStringKeys(o.NodeAttributes) {
		v := o.NodeAttributes[a]
		s := a + "[" + v.Kind[0:1] + "]: " + v.Value
		attrList = append(attrList, s)
	}
	localStorageList := []string{}
	for _, ls := range util.SortedStringKeys(o.LocalStorage) {
		v := o.LocalStorage[ls]
		s := fmt.Sprintf("%s [%s] %s: %s/%s", v.DeviceName, v.DeviceType, v.DeviceState, sizeToString(swag.Int64Value(v.UsableSizeBytes)), sizeToString(swag.Int64Value(v.SizeBytes)))
		localStorageList = append(localStorageList, s)
	}
	controllerState := "UNKNOWN"
	serviceIP := ""
	serviceLocator := ""
	if o.Service != nil {
		serviceIP = o.Service.ServiceIP
		serviceLocator = o.Service.ServiceLocator
		if o.Service.State != "" {
			controllerState = o.Service.State
		}
	}
	nodeState := o.State
	if o.State == common.ClusterStateManaged {
		nodeState = fmt.Sprintf("%s/%s", o.State, controllerState)
	}
	return map[string]string{
		hID:              string(o.Meta.ID),
		hTimeCreated:     time.Time(o.Meta.TimeCreated).Format(time.RFC3339),
		hTimeModified:    time.Time(o.Meta.TimeModified).Format(time.RFC3339),
		hVersion:         fmt.Sprintf("%d", o.Meta.Version),
		hName:            string(o.Name),
		hDescription:     string(o.Description),
		hAccount:         account,
		hTags:            strings.Join(o.Tags, ", "),
		hControllerState: controllerState,
		hNodeIdentifier:  o.NodeIdentifier,
		hNodeAttributes:  strings.Join(attrList, "\n"),
		hTotalCache:      sizeToString(swag.Int64Value(o.TotalCacheBytes)),
		hAvailableCache:  sizeToString(swag.Int64Value(o.AvailableCacheBytes)),
		hLocalStorage:    strings.Join(localStorageList, "\n"),
		hCspDomain:       domain,
		hCluster:         cluster,
		hServiceIP:       serviceIP,
		hServiceLocator:  serviceLocator,
		hNodeState:       nodeState,
	}
}

func (c *nodeCmd) validateColumns(columns string) error {
	var err error
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(nodeHeaders), nodeDefaultHeaders)
	return err
}

func (c *nodeCmd) Emit(data []*models.Node) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.EmitJSON(data)
	case "yaml":
		return appCtx.EmitYAML(data)
	}
	c.cacheAccounts()
	c.cacheCSPDomains()
	c.cacheClusters()
	lsCol := -1
	lsWidth := 0
	max := func(x, y int) int {
		if x > y {
			return x
		}
		return y
	}
	for i, cn := range c.tableCols {
		if cn == hLocalStorage {
			lsCol = i
			break
		}
	}
	rows := make([][]string, len(data))
	for i, o := range data {
		rec := c.makeRecord(o)
		row := make([]string, len(c.tableCols))
		for j, h := range c.tableCols {
			row[j] = rec[h]
		}
		if lsCol != -1 {
			ls := row[lsCol]
			lines := strings.Split(ls, "\n")
			for _, l := range lines {
				lsWidth = max(lsWidth, len(l))
			}
		}
		rows[i] = row
	}
	cT := func(t *tablewriter.Table) {
		if lsCol != -1 {
			t.SetColMinWidth(lsCol, lsWidth)
		}
	}
	return appCtx.EmitTable(c.tableCols, rows, cT)
}

func (c *nodeCmd) list(params *node.NodeListParams) ([]*models.Node, error) {
	if params == nil {
		params = node.NewNodeListParams()
	}
	res, err := appCtx.API.Node().NodeList(params)
	if err != nil {
		if e, ok := err.(*node.NodeListDefault); ok && e.Payload.Message != nil {
			return nil, fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res.Payload, nil
}

type nodeListCmd struct {
	Name           string         `short:"n" long:"name" description:"A Node object name in a specified cluster; alternatively specify the node identifier"`
	Tags           []string       `short:"t" long:"tag" description:"A tag value; repeat as needed"`
	DomainName     string         `short:"D" long:"domain" description:"Name of a cloud service provider domain"`
	ClusterName    string         `short:"C" long:"cluster-name" description:"Name of a cluster in a specified domain"`
	NodeIdentifier string         `short:"I" long:"node-identifier" description:"The unique identifier of the node; alternatively specify the node name"`
	SvcHbTimeGE    util.Timestamp `long:"svc-hb-time-ge" description:"Lower bound of service heartbeat time to list, specified either as a duration or an absolute RFC3339 time"`
	SvcHbTimeLE    util.Timestamp `long:"svc-hb-time-le" description:"Upper bound of service heartbeat time to list, specified either as a duration or an absolute RFC3339 time"`
	SvcStateEQ     string         `long:"svc-state-eq" description:"Service state to match"`
	SvcStateNE     string         `long:"svc-state-ne" description:"Service state to not match. Overrides the svc-state-eq flag"`
	Columns        string         `short:"c" long:"columns" description:"Comma separated list of column names"`
	Follow         bool           `short:"f" long:"follow-changes" description:"Monitor the system and repeat the command as relevant changes occur"`
	FollowNoClear  bool           `long:"follow-no-clear" description:"Do not clear the terminal (the default) when using follow-changes"`

	mustClearTerm bool
	params        *node.NodeListParams
	runCount      int
	lastLen       int
	nodeCmd
	optionalNidsRemainingArgsCatcher
}

func (c *nodeListCmd) Execute(args []string) error {
	var err error
	if err = c.verifyOptionalNidsAndNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	c.skipNodeCache = true
	_, clusterID, _, err := c.validateDomainClusterNodeNames(c.DomainName, c.ClusterName, "")
	if err != nil {
		return err
	}
	params := node.NewNodeListParams()
	params.Name = &c.Name
	params.Tags = c.Tags
	if c.NodeIdentifier != "" {
		params.NodeIdentifier = &c.NodeIdentifier
	}
	if c.ClusterName != "" {
		params.ClusterID = &clusterID
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
	if len(c.Nids) > 0 {
		params.NodeIds = c.Nids
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

var nodeListCmdRunCacheThreshold = 1

func (c *nodeListCmd) run() error {
	var err error
	var res []*models.Node
	if res, err = c.list(c.params); err != nil {
		return err
	}
	if c.mustClearTerm {
		appCtx.ClearTerminal()
	}
	if c.runCount > nodeListCmdRunCacheThreshold && c.lastLen < len(res) {
		// refresh caches when new objects added
		c.cspDomains = nil
		c.clusters = nil
	}
	c.lastLen = len(res)
	return c.Emit(res)
}

// ChangeDetected is part of the ChangeDetector interface
func (c *nodeListCmd) ChangeDetected() error {
	c.runCount++
	return c.run()
}

// WatcherArgs is part of the ChangeDetector interface
func (c *nodeListCmd) WatcherArgs() *models.CrudWatcherCreateArgs {
	cm := &models.CrudMatcher{
		URIPattern: "/nodes/?",
	}
	var scopeB bytes.Buffer
	if c.params.ClusterID != nil {
		fmt.Fprintf(&scopeB, ".*clusterId:%s", *c.params.ClusterID)
	}
	if scopeB.Len() > 0 {
		cm.ScopePattern = scopeB.String()
	}
	return &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{cm},
	}
}

type nodeDeleteCmd struct {
	Name        string `short:"n" long:"name" description:"A Node object name in a specified cluster" required:"yes"`
	DomainName  string `short:"D" long:"domain" description:"Name of a cloud service provider domain" required:"yes"`
	ClusterName string `short:"C" long:"cluster-name" description:"Name of a cluster in a specified domain" required:"yes"`
	Confirm     bool   `long:"confirm" description:"Confirm the deletion of the object"`

	nodeCmd
	remainingArgsCatcher
}

func (c *nodeDeleteCmd) Execute(args []string) error {
	if err := c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if !c.Confirm {
		return fmt.Errorf("specify --confirm to delete the \"%s\" Node object", c.Name)
	}
	if err := appCtx.InitContextAccount(); err != nil {
		return err
	}
	_, _, nodeID, err := c.validateDomainClusterNodeNames(c.DomainName, c.ClusterName, c.Name)
	if err != nil {
		return err
	}
	dParams := node.NewNodeDeleteParams()
	dParams.ID = nodeID
	if _, err = appCtx.API.Node().NodeDelete(dParams); err != nil {
		if e, ok := err.(*node.NodeDeleteDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return nil
}

type nodeFetchCmd struct {
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`

	nodeCmd
	requiredIDRemainingArgsCatcher
}

func (c *nodeFetchCmd) Execute(args []string) error {
	var err error
	if err = c.verifyRequiredIDAndNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := appCtx.InitContextAccount(); err != nil {
		return err
	}
	fParams := node.NewNodeFetchParams()
	fParams.ID = string(c.ID)
	res, err := appCtx.API.Node().NodeFetch(fParams)
	if err != nil {
		if e, ok := err.(*node.NodeFetchDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.Node{res.Payload})
}
