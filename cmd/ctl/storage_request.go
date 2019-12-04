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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/alecthomas/units"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/olekukonko/tablewriter"
)

func init() {
	initStorageRequest()
}

func initStorageRequest() {
	cmd, _ := parser.AddCommand("storage-request", "StorageRequest object commands", "StorageRequest object subcommands", &storageRequestCmd{})
	cmd.Aliases = []string{"storage-requests", "sr"}
	cmd.AddCommand("list", "List StorageRequests", "List or search for StorageRequest objects.", &storageRequestListCmd{})
	cmd.AddCommand("create", "Create a StorageRequest", "Create a StorageRequest object.", &storageRequestCreateCmd{})
	cmd.AddCommand("delete", "Delete a StorageRequest", "Delete a StorageRequest object.", &storageRequestDeleteCmd{})
	cmd.AddCommand("get", "Get a StorageRequest", "Get a StorageRequest object.", &storageRequestFetchCmd{})
	cmd.AddCommand("show-columns", "Show StorageRequest table columns", "Show names of columns used in table format", &showColsCmd{columns: storageRequestHeaders})
}

type storageRequestCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string
	activeReq    bool

	cacheHelper
}

// storageRequest record keys/headers and their description
var storageRequestHeaders = map[string]string{
	hAccount:             dAccount,
	hCSPStorageType:      dCspStorageType,
	hCluster:             dCluster, // pseudo-column
	hCompleteBy:          "The completion deadline.",
	hCspDomain:           dCspDomain, // pseudo-column
	hID:                  dID,
	hMinSizeBytes:        "minimum capacity",
	hParcelSizeBytes:     "The device parcel size",
	hNode:                dNodeName, // pseudo-column
	hRequestState:        "request state",
	hRequestedOperations: "operations",
	hStorageID:           "storage object",
	hPool:                "The pool.", // pseudo-column
	hTimeCreated:         dTimeCreated,
	hTimeModified:        dTimeModified,
	hTimeDuration:        "The total time taken on completion",
}

var storageRequestDefaultHeaders = []string{hID, hTimeCreated, hTimeDuration, hRequestState, hRequestedOperations, hCSPStorageType, hCspDomain, hCluster, hNode, hStorageID, hMinSizeBytes}

// makeRecord creates a map of properties
func (c *storageRequestCmd) makeRecord(o *models.StorageRequest, now time.Time) map[string]string {
	var ok bool
	var account, cspDomain, cluster, node string
	if a, ok := c.accounts[string(o.AccountID)]; !ok {
		account = string(o.AccountID)
	} else {
		account = a.name
	}
	if cspDomain, ok = c.cspDomains[string(o.CspDomainID)]; !ok {
		cspDomain = string(o.CspDomainID)
	}
	if cn, ok := c.clusters[string(o.ClusterID)]; ok {
		cluster = cn.name
	} else {
		cluster = string(o.ClusterID)
	}
	if cn, ok := c.nodes[string(o.NodeID)]; ok {
		node = cn.name
	} else {
		node = string(o.NodeID)
	}
	pool := string(o.PoolID)
	var msb string
	if swag.Int64Value(o.MinSizeBytes) != 0 {
		msb = sizeToString(swag.Int64Value(o.MinSizeBytes))
	}
	var psb string
	if swag.Int64Value(o.ParcelSizeBytes) != 0 {
		psb = sizeToString(swag.Int64Value(o.ParcelSizeBytes))
	}
	var tDur string
	if util.Contains([]string{common.StgReqStateSucceeded, common.StgReqStateFailed}, o.StorageRequestState) {
		tDur = fmt.Sprintf("%s", time.Time(o.Meta.TimeModified).Sub(time.Time(o.Meta.TimeCreated)).Round(time.Millisecond))
	} else {
		c.activeReq = true
		tDur = fmt.Sprintf("%s", now.Sub(time.Time(o.Meta.TimeCreated)).Round(time.Second))
	}
	return map[string]string{
		hAccount:             account,
		hCSPStorageType:      o.CspStorageType,
		hCluster:             cluster,
		hCompleteBy:          o.CompleteByTime.String(),
		hCspDomain:           cspDomain,
		hID:                  string(o.Meta.ID),
		hMinSizeBytes:        msb,
		hParcelSizeBytes:     psb,
		hNode:                node,
		hRequestState:        o.StorageRequestState,
		hRequestedOperations: strings.Join(o.RequestedOperations, ", "),
		hStorageID:           string(o.StorageID),
		hPool:                pool,
		hTimeCreated:         time.Time(o.Meta.TimeCreated).Format(time.RFC3339),
		hTimeModified:        time.Time(o.Meta.TimeModified).Format(time.RFC3339),
		hTimeDuration:        tDur,
	}
}

func (c *storageRequestCmd) validateColumns(columns string) error {
	var err error
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(storageRequestHeaders), storageRequestDefaultHeaders)
	return err
}

func (c *storageRequestCmd) Emit(data []*models.StorageRequest) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.EmitJSON(data)
	case "yaml":
		return appCtx.EmitYAML(data)
	}
	c.cacheAccounts()
	c.cacheCSPDomains()
	c.cacheClusters()
	c.cacheNodes()
	now := time.Now()
	rows := make([][]string, len(data))
	for i, o := range data {
		rec := c.makeRecord(o, now)
		row := make([]string, len(c.tableCols))
		for j, h := range c.tableCols {
			row[j] = rec[h]
		}
		rows[i] = row
	}
	cT := func(t *tablewriter.Table) {
		t.SetColWidth(18)
	}
	return appCtx.EmitTable(c.tableCols, rows, cT)
}

func (c *storageRequestCmd) list(params *storage_request.StorageRequestListParams) ([]*models.StorageRequest, error) {
	res, err := appCtx.API.StorageRequest().StorageRequestList(params)
	if err != nil {
		if e, ok := err.(*storage_request.StorageRequestListDefault); ok && e.Payload.Message != nil {
			return nil, fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res.Payload, nil
}

type storageRequestListCmd struct {
	DomainName    string         `short:"D" long:"domain" description:"Name of a cloud service provider domain"`
	ClusterName   string         `short:"C" long:"cluster-name" description:"Name of a cluster in a specified domain"`
	NodeName      string         `short:"N" long:"node-name" description:"The name of a node in a specified cluster"`
	NodeID        string         `long:"node-id" description:"A Node object identifier. Do not specify with node-name"`
	PoolID        string         `short:"P" long:"pool-id" description:"The ID of a pool"`
	State         string         `long:"state" description:"The request state"`
	StorageID     string         `short:"S" long:"storage-id" description:"The identifier of a storage object"`
	Recent        util.Timestamp `short:"R" long:"active-or-modified-ge" description:"Active or recently modified requests. Specify the modification duration or an absolute RFC3339 time"`
	IsTerminated  bool           `long:"terminated" description:"Return only storage requests that have terminated"`
	NotTerminated bool           `long:"active" description:"Return only storage requests that have not terminated"`
	VSRId         string         `long:"volume-series-request-id" description:"Return storage requests that have a claim from the specified VolumeSeriesRequest object"`
	Columns       string         `short:"c" long:"columns" description:"Comma separated list of column names"`
	Follow        bool           `short:"f" long:"follow-changes" description:"Monitor the system and repeat the command as relevant changes occur"`
	FollowNoClear bool           `long:"follow-no-clear" description:"Do not clear the terminal (the default) when using follow-changes"`

	mustClearTerm bool
	params        *storage_request.StorageRequestListParams
	res           []*models.StorageRequest
	runCount      int
	lastLen       int
	storageRequestCmd
	remainingArgsCatcher
}

func (c *storageRequestListCmd) Execute(args []string) error {
	if err := c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if c.IsTerminated && c.NotTerminated {
		return fmt.Errorf("do not specify 'terminated' and 'active' together")
	}
	if err := appCtx.InitContextAccount(); err != nil {
		return err
	}
	if c.NodeName != "" && c.NodeID != "" {
		return fmt.Errorf("do not specify 'node-name' and 'node-id' together")
	}
	domainID, clusterID, nodeID, err := c.validateDomainClusterNodeNames(c.DomainName, c.ClusterName, c.NodeName)
	if err != nil {
		return err
	}
	params := storage_request.NewStorageRequestListParams()
	if c.State != "" {
		params.StorageRequestState = &c.State
	}
	if domainID != "" {
		params.CspDomainID = &domainID
	}
	if clusterID != "" {
		params.ClusterID = &clusterID
	}
	if c.NodeID != "" {
		nodeID = c.NodeID
	}
	if nodeID != "" {
		params.NodeID = &nodeID
	}
	if c.PoolID != "" {
		params.PoolID = swag.String(c.PoolID)
	}
	if c.StorageID != "" {
		params.StorageID = &c.StorageID
	}
	if c.IsTerminated || c.NotTerminated {
		params.IsTerminated = &c.IsTerminated
	}
	if c.VSRId != "" {
		params.VolumeSeriesRequestID = &c.VSRId
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

var storageRequestListCmdRunCacheThreshold = 1

func (c *storageRequestListCmd) run() error {
	var err error
	if c.Recent.Specified() {
		dt := c.Recent.Value()
		c.params.ActiveOrTimeModifiedGE = &dt
	}
	if c.res, err = c.list(c.params); err != nil {
		return err
	}
	c.activeReq = false
	return c.display()
}

func (c *storageRequestListCmd) display() error {
	if c.mustClearTerm {
		appCtx.ClearTerminal()
	}
	if c.runCount > storageRequestListCmdRunCacheThreshold && c.lastLen < len(c.res) {
		// refresh caches when new objects added
		c.cspDomains = nil
		c.clusters = nil
		c.nodes = nil
	}
	c.lastLen = len(c.res)
	return c.Emit(c.res)
}

// ChangeDetected is part of the ChangeDetector interface
func (c *storageRequestListCmd) ChangeDetected() error {
	c.runCount++
	return c.run()
}

// NoChangeTick is part of the ChangeDetectorWithTicker interface
func (c *storageRequestListCmd) NoChangeTick() error {
	if c.activeReq {
		return c.display()
	}
	return nil
}

// NoChangeTickerPeriod is part of the ChangeDetectorWithTicker interface
func (c *storageRequestListCmd) NoChangeTickerPeriod() time.Duration {
	return time.Second
}

// WatcherArgs is part of the ChangeDetector interface
func (c *storageRequestListCmd) WatcherArgs() *models.CrudWatcherCreateArgs {
	cm := &models.CrudMatcher{
		URIPattern: "/storage-requests/?",
	}
	var scopeB bytes.Buffer
	if c.params.ClusterID != nil {
		fmt.Fprintf(&scopeB, ".*clusterId:%s", *c.params.ClusterID)
	}
	if c.params.NodeID != nil {
		fmt.Fprintf(&scopeB, ".*nodeId:%s", *c.params.NodeID)
	}
	if c.params.PoolID != nil {
		fmt.Fprintf(&scopeB, ".*poolId:%s", *c.params.PoolID)
	}
	if c.params.StorageRequestState != nil {
		fmt.Fprintf(&scopeB, ".*storageRequestState:%s", *c.params.StorageRequestState)
	}
	if scopeB.Len() > 0 {
		cm.ScopePattern = scopeB.String()
	}
	return &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{cm},
	}
}

type storageRequestCreateCmd struct {
	RequestedOperations []string      `short:"O" long:"operations" description:"Requested operations. Repeat as needed." choice:"PROVISION" choice:"ATTACH" choice:"FORMAT" choice:"USE" choice:"CLOSE" choice:"DETACH" choice:"RELEASE" choice:"REATTACH" required:"yes"`
	MinSizeBytes        string        `short:"b" long:"min-size-bytes" description:"The minimum number of bytes that may be provisioned. Specify a suffix of B, GB, GiB, etc.  Required for PROVISION."`
	ParcelSizeBytes     string        `short:"p" long:"parcel-size-bytes" description:"The device parcel size. Specify a suffix of B, GB, GiB, etc.  Required for FORMAT."`
	DomainName          string        `short:"D" long:"domain" description:"Name of a cloud service provider domain. Required if the operation is ATTACH."`
	PoolID              string        `short:"P" long:"pool-id" description:"The ID of a pool. Required for PROVISION if not searching."`
	ClusterName         string        `short:"C" long:"cluster-name" description:"Name of a cluster. Required if the operation is ATTACH."`
	NodeName            string        `short:"N" long:"node-name" description:"The name of a node in a specified cluster. Required if the operations is ATTACH or REATTACH."`
	StorageID           string        `short:"S" long:"storage-id" description:"The identifier of a storage object. Required for ATTACH, DETACH, REATTACH, RELEASE and CLOSE unless PROVISION also present."`
	SystemTags          []string      `long:"system-tag" description:"A system tag value; repeat as needed"`
	CompleteBy          time.Duration `short:"x" long:"complete-by" description:"The deadline for completion." default:"30m"`
	Columns             string        `short:"c" long:"columns" description:"Comma separated list of column names"`

	storageRequestCmd
	remainingArgsCatcher
}

func (c *storageRequestCreateCmd) Execute(args []string) error {
	var err error
	if err = c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	var msb int64
	if c.MinSizeBytes != "" {
		msb, err = units.ParseStrictBytes(c.MinSizeBytes)
		if err != nil {
			return err
		}
	}
	var psb int64
	if c.ParcelSizeBytes != "" {
		psb, err = units.ParseStrictBytes(c.ParcelSizeBytes)
		if err != nil {
			return err
		}
	}
	if err := appCtx.InitContextAccount(); err != nil {
		return err
	}
	_, _, nodeID, err := c.validateDomainClusterNodeNames(c.DomainName, c.ClusterName, c.NodeName)
	if err != nil {
		return err
	}
	req := &models.StorageRequestCreateArgs{}
	req.RequestedOperations = c.RequestedOperations
	if c.PoolID != "" {
		req.PoolID = models.ObjIDMutable(c.PoolID)
	}
	if nodeID != "" {
		if c.RequestedOperations[0] == "REATTACH" {
			req.ReattachNodeID = models.ObjIDMutable(nodeID)
		} else {
			req.NodeID = models.ObjIDMutable(nodeID)
		}
	}
	if c.MinSizeBytes != "" {
		req.MinSizeBytes = &msb
	}
	if c.ParcelSizeBytes != "" {
		req.ParcelSizeBytes = &psb
	}
	if c.StorageID != "" {
		req.StorageID = models.ObjIDMutable(c.StorageID)
	}
	req.SystemTags = c.SystemTags
	req.CompleteByTime = strfmt.DateTime(time.Now().Add(c.CompleteBy))
	params := storage_request.NewStorageRequestCreateParams().WithPayload(req)
	res, err := appCtx.API.StorageRequest().StorageRequestCreate(params)
	if err != nil {
		if e, ok := err.(*storage_request.StorageRequestCreateDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.StorageRequest{res.Payload})
}

type storageRequestDeleteCmd struct {
	Confirm bool `long:"confirm" description:"Confirm the deletion of the object"`

	storageRequestCmd
	requiredIDRemainingArgsCatcher
}

func (c *storageRequestDeleteCmd) Execute(args []string) error {
	if err := c.verifyRequiredIDAndNoRemainingArgs(); err != nil {
		return err
	}
	if !c.Confirm {
		return fmt.Errorf("specify --confirm to delete the \"%s\" StorageRequest object", c.ID)
	}
	if err := appCtx.InitContextAccount(); err != nil {
		return err
	}
	dParams := storage_request.NewStorageRequestDeleteParams()
	dParams.ID = string(c.ID)
	if _, err := appCtx.API.StorageRequest().StorageRequestDelete(dParams); err != nil {
		if e, ok := err.(*storage_request.StorageRequestDeleteDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return nil
}

type storageRequestFetchCmd struct {
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`

	storageRequestCmd
	requiredIDRemainingArgsCatcher
}

func (c *storageRequestFetchCmd) Execute(args []string) error {
	if err := c.verifyRequiredIDAndNoRemainingArgs(); err != nil {
		return err
	}
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := appCtx.InitContextAccount(); err != nil {
		return err
	}
	fParams := storage_request.NewStorageRequestFetchParams()
	fParams.ID = string(c.ID)
	res, err := appCtx.API.StorageRequest().StorageRequestFetch(fParams)
	if err != nil {
		if e, ok := err.(*storage_request.StorageRequestFetchDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.StorageRequest{res.Payload})
}
