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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/storage"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/alecthomas/units"
	"github.com/go-openapi/swag"
)

func init() {
	initStorage()
}

func initStorage() {
	cmd, _ := parser.AddCommand("storage", "Storage object commands", "Storage object subcommands", &storageCmd{})
	cmd.Aliases = []string{"storage"}
	cmd.AddCommand("get", "Get a Storage object", "Get a Storage object.", &storageFetchCmd{})
	cmd.AddCommand("list", "List Storage", "List or search for Storage objects.", &storageListCmd{})
	cmd.AddCommand("delete", "Delete Storage", "Delete a Storage object.", &storageDeleteCmd{})
	cmd.AddCommand("show-columns", "Show Storage table columns", "Show names of columns used in table format", &showColsCmd{columns: storageHeaders})
}

type storageCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string

	cacheHelper
}

// storage record keys/headers and their description
var storageHeaders = map[string]string{
	hAccessibilityScope:   dAccessibilityScope,
	hAccessibilityScopeID: "The accessibility scope object ID.",
	hAccount:              dAccount,
	hAttachedCluster:      dCluster,                                     // pseudo-column
	hAttachedNode:         "The node to which the storage is attached.", // pseudo-column
	hAttachmentState:      "The node attachment state.",
	hAvailableBytes:       "The available size in bytes. (aggregation: sum)",
	hCSPStorageType:       dCspStorageType,
	hCspDomain:            dCspDomain, // pseudo-column
	hDeviceState:          "The Nuvoloso storage layer device state for this storage object.",
	hID:                   dID,
	hMediaState:           "The media state for the contents of this storage object.",
	hNodeDevice:           "The OS device name.",
	hParcelSizeBytes:      "The parcel size in bytes.",
	hProvisionedState:     "The provisioning state.",
	hSizeBytes:            "The size in bytes. (aggregation: sum)",
	hStorageIdentifier:    "The CSP identifier for the storage object.",
	hPool:                 "The pool.", // pseudo-column
	hTotalParcelCount:     "The number of parcels",
}

var storageDefaultHeaders = []string{hID, hCSPStorageType, hProvisionedState, hAttachmentState, hDeviceState, hAvailableBytes, hSizeBytes, hNode, hNodeDevice, hStorageIdentifier}

// aggregation fields
var storageAgFields = agFieldMap{
	hSizeBytes:      agField{"sizeBytes", true},
	hAvailableBytes: agField{"availableBytes", true},
}

// makeRecord creates a map of properties
func (c *storageCmd) makeRecord(o *models.Storage) map[string]string {
	var ok bool
	var account, cspDomain string
	if a, ok := c.accounts[string(o.AccountID)]; !ok {
		account = string(o.AccountID)
	} else {
		account = a.name
	}
	if cspDomain, ok = c.cspDomains[string(o.CspDomainID)]; !ok { // assume map
		cspDomain = string(o.CspDomainID)
	}
	var cluster, node, aSt, dSt, mSt, pSt, nDev string
	if o.StorageState != nil {
		aSt = o.StorageState.AttachmentState
		dSt = o.StorageState.DeviceState
		mSt = o.StorageState.MediaState
		pSt = o.StorageState.ProvisionedState
		nDev = o.StorageState.AttachedNodeDevice
		if cl, ok := c.clusters[string(o.ClusterID)]; ok { // assume map
			cluster = cl.name
		} else {
			cluster = string(o.ClusterID)
		}
		if cn, ok := c.nodes[string(o.StorageState.AttachedNodeID)]; ok { // assume map
			node = cn.name
		} else {
			node = string(o.StorageState.AttachedNodeID)
		}
	}
	var sb, ab, pb, tpc string
	if swag.Int64Value(o.SizeBytes) != 0 {
		sb = sizeToString(swag.Int64Value(o.SizeBytes))
	}
	if swag.Int64Value(o.AvailableBytes) != 0 {
		ab = sizeToString(swag.Int64Value(o.AvailableBytes))
	}
	if swag.Int64Value(o.ParcelSizeBytes) != 0 {
		pb = sizeToString(swag.Int64Value(o.ParcelSizeBytes))
	}
	if swag.Int64Value(o.TotalParcelCount) != 0 {
		tpc = fmt.Sprintf("%d", swag.Int64Value(o.TotalParcelCount))
	}
	return map[string]string{
		hAccessibilityScope:   string(o.StorageAccessibility.AccessibilityScope),
		hAccessibilityScopeID: string(o.StorageAccessibility.AccessibilityScopeObjID),
		hAccount:              account,
		hCSPStorageType:       string(o.CspStorageType),
		hAttachedCluster:      cluster,
		hCspDomain:            cspDomain,
		hID:                   string(o.Meta.ID),
		hSizeBytes:            sb,
		hAvailableBytes:       ab,
		hParcelSizeBytes:      pb,
		hAttachedNode:         node,
		hPool:                 string(o.PoolID),
		hNodeDevice:           nDev,
		hAttachmentState:      aSt,
		hDeviceState:          dSt,
		hMediaState:           mSt,
		hProvisionedState:     pSt,
		hStorageIdentifier:    o.StorageIdentifier,
		hTotalParcelCount:     tpc,
	}
}

func (c *storageCmd) validateColumns(columns string) error {
	var err error
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(storageHeaders), storageDefaultHeaders)
	return err
}

func (c *storageCmd) Emit(data []*models.Storage) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.EmitJSON(data)
	case "yaml":
		return appCtx.EmitYAML(data)
	}
	c.loadDCNCaches()
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

func (c *storageCmd) list(params *storage.StorageListParams) (*storage.StorageListOK, error) {
	res, err := appCtx.API.Storage().StorageList(params)
	if err != nil {
		if e, ok := err.(*storage.StorageListDefault); ok && e.Payload.Message != nil {
			return nil, fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res, nil
}

type storageListCmd struct {
	DomainName              string   `short:"D" long:"domain" description:"Name of a cloud service provider domain"`
	ClusterName             string   `short:"C" long:"cluster-name" description:"Name of a cluster in a specified domain"`
	NodeName                string   `short:"N" long:"node-name" description:"The name of a node in a specified cluster"`
	PoolID                  string   `short:"P" long:"pool-id" description:"The id of a pool"`
	StorageType             string   `short:"T" long:"storage-type" description:"The CSP name for a type of storage"`
	AccessibilityScope      string   `short:"a" long:"scope" description:"The accessibility scope." choice:"NODE" choice:"CSPDOMAIN"`
	AccessibilityScopeObjID string   `long:"scope-object-id" description:"ID of the accessibility scope object."`
	BytesGE                 string   `long:"ge" description:"Select objects whose availableBytes value is greater than or equal to the specified value.  A suffix of B, GB, GiB, etc. is required."`
	BytesLT                 string   `long:"lt" description:"Select objects whose availableBytes value is less than the specified value.  A suffix of B, GB, GiB, etc. is required."`
	ProvisionedState        string   `long:"provisioned-state" description:"Selects objects whose provisionedState is equal to the specified value" choice:"UNPROVISIONED" choice:"PROVISIONING" choice:"PROVISIONED" choice:"UNPROVISIONING" choice:"ERROR"`
	AttachmentState         string   `long:"attachment-state" description:"Selects objects whose attachmentState is equal to the specified value" choice:"DETACHED" choice:"ATTACHED" choice:"DETACHING" choice:"ATTACHING" choice:"ERROR"`
	DeviceState             string   `long:"device-state" description:"Selects objects whose deviceState is equal to the specified value" choice:"UNUSED" choice:"FORMATTING" choice:"OPENING" choice:"OPEN" choice:"CLOSING" choice:"ERROR"`
	MediaState              string   `long:"media-state" description:"Selects objects whose mediaState is equal to the specified value" choice:"UNFORMATTED" choice:"FORMATTED"`
	Sum                     []string `long:"sum" description:"Sum specified fields"`
	IsProvisioned           bool     `long:"is-provisioned" description:"Selects objects whose provisionedState is PROVISIONED or UNPROVISIONING"`
	NotProvisioned          bool     `long:"not-provisioned" description:"Selects objects whose provisionedState is neither PROVISIONED nor UNPROVISIONING"`
	Columns                 string   `short:"c" long:"columns" description:"Comma separated list of column names"`
	Follow                  bool     `short:"f" long:"follow-changes" description:"Monitor the system and repeat the command as relevant changes occur."`
	FollowNoClear           bool     `long:"follow-no-clear" description:"Do not clear the terminal (the default) when using follow-changes"`

	mustClearTerm bool
	params        *storage.StorageListParams
	runCount      int
	lastLen       int

	storageCmd
	remainingArgsCatcher
}

func (c *storageListCmd) Execute(args []string) error {
	var err error
	if err = c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	var bge, blt int64
	if c.BytesGE != "" {
		bge, err = units.ParseStrictBytes(c.BytesGE)
		if err != nil {
			return err
		}
	}
	if c.BytesLT != "" {
		blt, err = units.ParseStrictBytes(c.BytesLT)
		if err != nil {
			return err
		}
	}
	params := storage.NewStorageListParams()
	if len(c.Sum) > 0 {
		if params.Sum, err = storageAgFields.validateSumFields(c.Sum); err != nil {
			return err
		}
		if c.Columns != "" {
			return fmt.Errorf("do not specify --columns when using --sum")
		}
	}
	if c.IsProvisioned {
		params.IsProvisioned = swag.Bool(true)
	}
	if c.NotProvisioned {
		if c.IsProvisioned {
			return fmt.Errorf("do not specify both --is-provisioned and --not-provisioned")
		}
		params.IsProvisioned = swag.Bool(false)
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	domainID, clusterID, nodeID, err := c.validateDomainClusterNodeNames(c.DomainName, c.ClusterName, c.NodeName)
	if err != nil {
		return err
	}
	if domainID != "" {
		params.CspDomainID = &domainID
	}
	if clusterID != "" {
		params.ClusterID = &clusterID
	}
	if nodeID != "" {
		params.AttachedNodeID = &nodeID
	}
	if c.PoolID != "" {
		params.PoolID = swag.String(c.PoolID)
	}
	if c.StorageType != "" {
		params.CspStorageType = &c.StorageType
	}
	if c.AccessibilityScope != "" {
		params.AccessibilityScope = &c.AccessibilityScope
	}
	if c.AccessibilityScopeObjID != "" {
		params.AccessibilityScopeObjID = &c.AccessibilityScopeObjID
	}
	if c.BytesGE != "" {
		params.AvailableBytesGE = swag.Int64(bge)
	}
	if c.BytesLT != "" {
		params.AvailableBytesLT = swag.Int64(blt)
	}
	if c.ProvisionedState != "" {
		params.ProvisionedState = swag.String(c.ProvisionedState)
	}
	if c.AttachmentState != "" {
		params.AttachmentState = swag.String(c.AttachmentState)
	}
	if c.DeviceState != "" {
		params.DeviceState = swag.String(c.DeviceState)
	}
	if c.MediaState != "" {
		params.MediaState = swag.String(c.MediaState)
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

var storageListCmdRunCacheThreshold = 1

func (c *storageListCmd) run() error {
	var err error
	var res *storage.StorageListOK
	if res, err = c.list(c.params); err != nil {
		return err
	}
	if c.mustClearTerm {
		appCtx.ClearTerminal()
	}
	if c.runCount > storageListCmdRunCacheThreshold && c.lastLen < len(res.Payload) {
		// refresh caches when new objects added
		c.cspDomains = nil
		c.clusters = nil
		c.accounts = nil
		c.nodes = nil
	}
	c.lastLen = len(res.Payload)
	if len(c.Sum) == 0 {
		return c.Emit(res.Payload)
	}
	appCtx.OutputFormat = c.OutputFormat
	return appCtx.EmitAggregation(storageAgFields, res)
}

// ChangeDetected is part of the ChangeDetector interface
func (c *storageListCmd) ChangeDetected() error {
	c.runCount++
	return c.run()
}

// WatcherArgs is part of the ChangeDetector interface
func (c *storageListCmd) WatcherArgs() *models.CrudWatcherCreateArgs {
	cm := &models.CrudMatcher{
		URIPattern: "/storage/?",
	}
	// there are no scope properties currently defined for storage
	return &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{cm},
	}
}

type storageDeleteCmd struct {
	Confirm bool `long:"confirm" description:"Confirm the deletion of the object"`

	storageCmd
	requiredIDRemainingArgsCatcher
}

func (c *storageDeleteCmd) Execute(args []string) error {
	if err := c.verifyRequiredIDAndNoRemainingArgs(); err != nil {
		return err
	}
	if !c.Confirm {
		return fmt.Errorf("specify --confirm to delete the \"%s\" Storage object", c.ID)
	}
	if err := appCtx.InitContextAccount(); err != nil {
		return err
	}
	dParams := storage.NewStorageDeleteParams().WithID(c.ID)
	if _, err := appCtx.API.Storage().StorageDelete(dParams); err != nil {
		if e, ok := err.(*storage.StorageDeleteDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return nil
}

type storageFetchCmd struct {
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`

	storageCmd
	requiredIDRemainingArgsCatcher
}

func (c *storageFetchCmd) Execute(args []string) error {
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
	fParams := storage.NewStorageFetchParams().WithID(c.ID)
	res, err := appCtx.API.Storage().StorageFetch(fParams)
	if err != nil {
		if e, ok := err.(*storage.StorageFetchDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.Storage{res.Payload})
}
