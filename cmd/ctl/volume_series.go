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
	"os"
	"strings"

	vs "github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/alecthomas/units"
	"github.com/go-openapi/swag"
)

func init() {
	initVolumeSeries()
}

func initVolumeSeries() {
	cmd, _ := parser.AddCommand("volume-series", "VolumeSeries object commands", "VolumeSeries object subcommands", &volumeSeriesCmd{})
	cmd.Aliases = []string{"vol", "volume", "volumes", "vs"}
	cmd.AddCommand("get", "Get a VolumeSeries", "Get a VolumeSeries object.", &volumeSeriesFetchCmd{})
	cmd.AddCommand("list", "List VolumeSeries", "List or search for VolumeSeries objects.", &volumeSeriesListCmd{})
	cmd.AddCommand("modify", "Modify a VolumeSeries", "Modify a VolumeSeries object.  Use a whitespace-only string to clear trimmed properties.", &volumeSeriesModifyCmd{})
	cmd.AddCommand("get-pv", "Persistent Volume specification", "Download the persistent volume specification to mount on a node", &volumeSeriesGetPV{})
	cmd.AddCommand("show-columns", "Show VolumeSeries table columns", "Show names of columns used in table format", &showColsCmd{columns: volumeSeriesHeaders})
}

type volumeSeriesCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string

	cacheHelper
}

// volumeSeries record keys/headers and their description
var volumeSeriesHeaders = map[string]string{
	hRootParcelUUID: "The UUID of the root parcel of the volume series object.",
	hAccount:        dAccount,
	hBoundCluster:   "The cluster to which the volume series is bound.",
	hMounts:         "The nodes and devices where the volume series is mounted.",
	// TBD hCapacityAllocations: "The reserved and consume capacities for the volume series, by pool.",
	// TBD hStorageParcels:      "The set of storage object parcels used by the volume series.",
	hRootStorageID:        "The storage object containing the root parcel of the volume series.",
	hID:                   dID,
	hName:                 dName,
	hDescription:          dDescription,
	hState:                "The state of the volume series object.",
	hTags:                 dTags,
	hConsistencyGroup:     "The consistency group to which the volume series belongs.",
	hServicePlan:          "The service plan for the volume series.",
	hSizeBytes:            "The size in bytes. A '*' is appended if the adjusted size is different.",
	hAdjustedSizeBytes:    "The size in bytes adjusted with additional SPA capacity. A '*' is appended if adjusted.",
	hHeadNode:             dHeadNode,
	hConfiguredNode:       dConfiguredNode,
	hCacheAllocations:     "The cache allocations.",
	hPersistentVolumeName: "The name of volume in the cluster.",
}

var volumeSeriesDefaultHeaders = []string{hName, hAccount, hState, hServicePlan, hAdjustedSizeBytes, hBoundCluster, hConfiguredNode, hConsistencyGroup}

// makeRecord creates a map of properties
func (c *volumeSeriesCmd) makeRecord(o *models.VolumeSeries) map[string]string {
	var ok bool
	var account, cluster, cg, plan, sb, adjSz string
	if a, ok := c.accounts[string(o.AccountID)]; !ok {
		account = string(o.AccountID)
	} else {
		account = a.name
	}
	if cn, ok := c.clusters[string(o.BoundClusterID)]; ok { // assumes map present
		cluster = cn.name
	} else {
		cluster = string(o.BoundClusterID)
	}
	if plan, ok = c.servicePlans[string(o.ServicePlanID)]; !ok {
		plan = string(o.ServicePlanID)
	}
	if cn, ok := c.consistencyGroups[string(o.ConsistencyGroupID)]; ok {
		cg = cn.name
	} else {
		cg = string(o.ConsistencyGroupID)
	}
	configuredNode := string(o.ConfiguredNodeID)
	if nn, ok := c.nodes[configuredNode]; ok {
		configuredNode = nn.name
	}
	mounts := []string{}
	headNode := ""
	for _, m := range o.Mounts {
		var n string
		if nn, ok := c.nodes[string(m.MountedNodeID)]; ok {
			n = nn.name
		} else {
			n = string(m.MountedNodeID)
		}
		mode := "" // READ_WRITE
		if m.MountMode == com.VolMountModeRO {
			mode = "[ro]"
		}
		state := "" // MOUNTED
		if m.MountState == com.VolMountStateMounting {
			state = "(mounting)"
		} else if m.MountState == com.VolMountStateUnmounting {
			state = "(unmounting)"
		} else if m.MountState == com.VolMountStateError {
			state = "(error)"
		}
		snap := "" // HEAD
		if m.SnapIdentifier != com.VolMountHeadIdentifier {
			snap = m.SnapIdentifier + ":"
		} else {
			headNode = n
		}
		s := snap + n + ":" + m.MountedNodeDevice + mode + state
		mounts = append(mounts, s)
	}
	if swag.Int64Value(o.SizeBytes) != 0 {
		sb = sizeToString(swag.Int64Value(o.SizeBytes))
	}
	if swag.Int64Value(o.SpaAdditionalBytes) != 0 {
		adjSz = sizeToString(swag.Int64Value(o.SizeBytes)+swag.Int64Value(o.SpaAdditionalBytes)) + "*"
		sb += "*"
	} else {
		adjSz = sb
	}
	var cacheAllocations string
	cacheAllocationsList := []string{}
	for _, a := range util.SortedStringKeys(o.CacheAllocations) {
		v := o.CacheAllocations[a]
		asz := sizeToString(swag.Int64Value(v.AllocatedSizeBytes))
		rsz := sizeToString(swag.Int64Value(v.RequestedSizeBytes))
		if len(o.CacheAllocations) == 1 {
			cacheAllocations = asz + "/" + rsz
			break
		}
		s := a + ": " + asz + "/" + rsz
		cacheAllocationsList = append(cacheAllocationsList, s)
	}
	if cacheAllocations == "" {
		cacheAllocations = strings.Join(cacheAllocationsList, "\n")
	}
	var pvName string
	if o.ClusterDescriptor != nil {
		if val, ok := o.ClusterDescriptor[com.ClusterDescriptorPVName]; ok {
			pvName = val.Value
		}
	}
	return map[string]string{
		hRootParcelUUID:       o.RootParcelUUID,
		hAccount:              account,
		hBoundCluster:         cluster,
		hMounts:               strings.Join(mounts, "\n"),
		hRootStorageID:        string(o.RootStorageID),
		hID:                   string(o.Meta.ID),
		hName:                 string(o.Name),
		hDescription:          string(o.Description),
		hState:                o.VolumeSeriesState,
		hTags:                 strings.Join(o.Tags, ", "),
		hConsistencyGroup:     cg,
		hServicePlan:          plan,
		hSizeBytes:            sb,
		hAdjustedSizeBytes:    adjSz,
		hHeadNode:             headNode,
		hConfiguredNode:       configuredNode,
		hCacheAllocations:     cacheAllocations,
		hPersistentVolumeName: pvName,
	}
}

func (c *volumeSeriesCmd) validateColumns(columns string) error {
	var err error
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(volumeSeriesHeaders), volumeSeriesDefaultHeaders)
	return err
}

func (c *volumeSeriesCmd) Emit(data []*models.VolumeSeries) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.EmitJSON(data)
	case "yaml":
		return appCtx.EmitYAML(data)
	}
	c.loadDCNCaches()
	c.cacheAccounts()
	c.cacheConsistencyGroups()
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

func (c *volumeSeriesCmd) list(params *vs.VolumeSeriesListParams) ([]*models.VolumeSeries, error) {
	if params == nil {
		params = vs.NewVolumeSeriesListParams()
	}
	res, err := appCtx.API.VolumeSeries().VolumeSeriesList(params)
	if err != nil {
		if e, ok := err.(*vs.VolumeSeriesListDefault); ok && e.Payload.Message != nil {
			return nil, fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res.Payload, nil
}

type volumeSeriesListCmd struct {
	NamePattern        string   `short:"n" long:"name" description:"Regex of matching volume series names"`
	ServicePlan        string   `short:"P" long:"service-plan" description:"The name of a service plan"`
	DomainName         string   `short:"D" long:"domain" description:"Name of a cloud service provider domain"`
	ClusterName        string   `short:"C" long:"cluster-name" description:"Name of a cluster in the specified domain"`
	NodeName           string   `short:"N" long:"mounted-node-name" description:"The name of a node in the specified cluster on which a volume is mounted"`
	ConfiguredNodeName string   `long:"configured-node-name" description:"The name of a node in the specified cluster on which a volume is configured"`
	PoolID             string   `long:"pool-id" description:"The id of a pool"`
	States             []string `long:"state" description:"A volume series state, prefix with ! to negate (e.g. !IN_USE); repeat as needed"`
	StorageID          string   `long:"storage-id" description:"ID of a storage object."`
	ConsistencyGroup   string   `long:"consistency-group" description:"The name of a consistency group"`
	OwnedOnly          bool     `long:"owned-only" description:"Retrieve only volume series owned by the account, otherwise, if a tenant admin, retrieve volume series owned by the tenant or any subordinate account. A no-op for non-tenant accounts"`
	Tags               []string `short:"t" long:"tag" description:"A tag value; repeat as needed"`
	Columns            string   `short:"c" long:"columns" description:"Comma separated list of column names"`
	Follow             bool     `short:"f" long:"follow-changes" description:"Monitor the system and repeat the command as relevant changes occur."`
	FollowNoClear      bool     `long:"follow-no-clear" description:"Do not clear the terminal (the default) when using follow-changes"`

	mustClearTerm bool
	params        *vs.VolumeSeriesListParams
	runCount      int
	lastLen       int

	volumeSeriesCmd
	remainingArgsCatcher
}

func (c *volumeSeriesListCmd) Execute(args []string) error {
	var err error
	if err := c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	domID, clusterID, nodeID, err := c.validateDomainClusterNodeNames(c.DomainName, c.ClusterName, c.NodeName)
	var configuredNodeID string
	if err == nil && c.ConfiguredNodeName != "" {
		_, _, configuredNodeID, err = c.validateDomainClusterNodeNames(c.DomainName, c.ClusterName, c.ConfiguredNodeName)
	}
	if err != nil {
		return err
	}
	_, cgID, err := c.validateApplicationConsistencyGroupNames(nil, c.ConsistencyGroup)
	if err != nil {
		return err
	}
	params := vs.NewVolumeSeriesListParams()
	if c.NamePattern != "" {
		params.NamePattern = &c.NamePattern
	}
	params.Tags = c.Tags
	if appCtx.AccountID != "" && c.OwnedOnly {
		params.AccountID = &appCtx.AccountID
	}
	if domID != "" {
		params.BoundCspDomainID = &domID
	}
	if clusterID != "" {
		params.BoundClusterID = &clusterID
	}
	if cgID != "" {
		params.ConsistencyGroupID = &cgID
	}
	if nodeID != "" {
		params.MountedNodeID = &nodeID
	}
	if configuredNodeID != "" {
		params.ConfiguredNodeID = &configuredNodeID
	}
	if c.StorageID != "" {
		params.StorageID = &c.StorageID
	}
	if c.PoolID != "" {
		params.PoolID = swag.String(c.PoolID)
	}
	if c.ServicePlan != "" {
		if err = c.cacheServicePlans(); err != nil {
			return err
		}
		for id, n := range c.servicePlans {
			if n == c.ServicePlan {
				params.ServicePlanID = swag.String(id)
			}
		}
		if params.ServicePlanID == nil {
			return fmt.Errorf("service plan '%s' not found", c.ServicePlan)
		}
	}
	for _, state := range c.States {
		if strings.HasPrefix(state, "!") {
			params.VolumeSeriesStateNot = append(params.VolumeSeriesStateNot, strings.TrimPrefix(state, "!"))
		} else {
			params.VolumeSeriesState = append(params.VolumeSeriesState, state)
		}
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

var volumeSeriesListCmdRunCacheThreshold = 1

func (c *volumeSeriesListCmd) run() error {
	var err error
	var res []*models.VolumeSeries
	if res, err = c.list(c.params); err != nil {
		return err
	}
	if c.mustClearTerm {
		appCtx.ClearTerminal()
	}
	if c.runCount > volumeSeriesListCmdRunCacheThreshold && c.lastLen < len(res) {
		// refresh caches when new objects added
		c.cspDomains = nil
		c.clusters = nil
		c.nodes = nil
		c.accounts = nil
		c.consistencyGroups = nil
	}
	c.lastLen = len(res)
	return c.Emit(res)
}

// ChangeDetected is part of the ChangeDetector interface
func (c *volumeSeriesListCmd) ChangeDetected() error {
	c.runCount++
	return c.run()
}

// WatcherArgs is part of the ChangeDetector interface
func (c *volumeSeriesListCmd) WatcherArgs() *models.CrudWatcherCreateArgs {
	cm := &models.CrudMatcher{
		URIPattern: "/volume-series/?",
	}
	// there are no scope properties currently defined for VS
	return &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{cm},
	}
}

type volumeSeriesFetchCmd struct {
	DescriptorKey string `short:"K" long:"descriptor-key" description:"Emit the value of the named clusterDescriptor key to the output-file"`
	OutputFile    string `short:"O" long:"output-file" description:"The name of the output file. Use - for stdout" default:"vs-descriptor"`
	Columns       string `short:"c" long:"columns" description:"Comma separated list of column names"`

	volumeSeriesCmd
	requiredIDRemainingArgsCatcher
}

func (c *volumeSeriesFetchCmd) Execute(args []string) error {
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
	var oF *os.File
	if c.DescriptorKey != "" {
		if c.OutputFile == "-" {
			oF = os.Stdout
		} else {
			oF, err = os.Create(c.OutputFile)
			if err != nil {
				return err
			}
			defer oF.Close()
		}
	}
	fParams := vs.NewVolumeSeriesFetchParams().WithID(c.ID)
	res, err := appCtx.API.VolumeSeries().VolumeSeriesFetch(fParams)
	if err != nil {
		if e, ok := err.(*vs.VolumeSeriesFetchDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	if c.DescriptorKey != "" {
		kv, found := res.Payload.ClusterDescriptor[c.DescriptorKey]
		if !found {
			return fmt.Errorf("clusterDescriptor key '%s' not found", c.DescriptorKey)
		}
		oF.WriteString(kv.Value)
		return nil
	}
	return c.Emit([]*models.VolumeSeries{res.Payload})
}

type volumeSeriesModifyCmd struct {
	Name             string   `short:"n" long:"name" description:"A VolumeSeries name" required:"yes"`
	ConsistencyGroup string   `long:"consistency-group" description:"The name of the new ConsistencyGroup"`
	Description      string   `short:"d" long:"description" description:"The new description; leading and trailing whitespace will be stripped"`
	Tags             []string `short:"t" long:"tag" description:"A tag value; repeat as needed"`
	TagsAction       string   `long:"tag-action" description:"Specifies how to process tag values" choice:"APPEND" choice:"REMOVE" choice:"SET" default:"APPEND"`
	Version          int32    `short:"V" long:"version" description:"Enforce update of the specified version of the object"`
	Columns          string   `short:"c" long:"columns" description:"Comma separated list of column names"`

	volumeSeriesCmd
	remainingArgsCatcher
}

func (c *volumeSeriesModifyCmd) Execute(args []string) error {
	if err := c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := appCtx.InitContextAccount(); err != nil {
		return err
	}
	_, cgID, err := c.validateApplicationConsistencyGroupNames(nil, c.ConsistencyGroup)
	if err != nil {
		return err
	}
	lRes, err := c.list(vs.NewVolumeSeriesListParams().WithName(&c.Name).WithAccountID(&appCtx.AccountID))
	if err != nil {
		return err
	} else if len(lRes) != 1 {
		return fmt.Errorf("volume series '%s' not found for account '%s'", c.Name, appCtx.Account)
	}

	nChg := 0
	uParams := vs.NewVolumeSeriesUpdateParams().WithPayload(&models.VolumeSeriesMutable{})
	ts := strings.TrimSpace(c.Description)
	if c.Description != "" {
		uParams.Payload.Description = models.ObjDescription(ts)
		uParams.Set = append(uParams.Set, "description")
		nChg++
	}
	if cgID != "" {
		uParams.Payload.ConsistencyGroupID = models.ObjIDMutable(cgID)
		uParams.Set = append(uParams.Set, "consistencyGroupId")
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
	uParams.ID = string(lRes[0].Meta.ID)
	if c.Version != 0 {
		uParams.Version = &c.Version
	}
	var res *vs.VolumeSeriesUpdateOK
	if res, err = appCtx.API.VolumeSeries().VolumeSeriesUpdate(uParams); err != nil {
		if e, ok := err.(*vs.VolumeSeriesUpdateDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.VolumeSeries{res.Payload})
}

type volumeSeriesGetPV struct {
	OutputFile     string `short:"O" long:"output-file" description:"The name of the output file. Use - for stdout" default:"pv.yaml"`
	FsType         string `long:"fstype" description:"The Filesystem type" default:"ext4"`
	ClusterVersion string `long:"clusterVersion" description:"The cluster version"`
	Capacity       string `long:"capacity" description:"The size of the Persistent Volume. Specify a suffix of B, GB, GiB, etc."`

	volumeSeriesCmd
	requiredIDRemainingArgsCatcher
}

func (c *volumeSeriesGetPV) Execute(args []string) error {
	var err error
	if err = c.verifyRequiredIDAndNoRemainingArgs(); err != nil {
		return err
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	var oF *os.File
	if c.OutputFile == "-" {
		oF = os.Stdout
	} else {
		oF, err = os.Create(c.OutputFile)
		if err != nil {
			return err
		}
		defer oF.Close()
	}
	fParams := vs.NewVolumeSeriesPVSpecFetchParams()
	fParams.ID = c.ID
	fParams.FsType = &c.FsType
	fParams.ClusterVersion = &c.ClusterVersion
	var cb int64
	if c.Capacity != "" {
		cb, err = units.ParseStrictBytes(c.Capacity)
		if err != nil {
			return err
		}
		fParams.Capacity = swag.Int64(cb)
	}
	//fmt.Println(fParams.ClusterVersion)
	var ret *vs.VolumeSeriesPVSpecFetchOK
	if ret, err = appCtx.API.VolumeSeries().VolumeSeriesPVSpecFetch(fParams); err != nil {
		if e, ok := err.(*vs.VolumeSeriesPVSpecFetchDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	oF.WriteString(ret.Payload.PvSpec)
	return nil
}
