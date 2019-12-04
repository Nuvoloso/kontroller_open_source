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

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/service_plan_allocation"
	vs "github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	vsr "github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series_request"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/alecthomas/units"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/olekukonko/tablewriter"
)

func init() {
	initVolumeSeriesRequest()
}

func initVolumeSeriesRequest() {
	cmd, _ := parser.AddCommand("volume-series-request", "VolumeSeriesRequest object commands", "VolumeSeriesRequest object subcommands", &volumeSeriesRequestCmd{})
	cmd.Aliases = []string{"volume-series-requests", "vr", "vsr"}
	cmd.AddCommand("list", "List VolumeSeriesRequests", "List or search for VolumeSeriesRequest objects.", &volumeSeriesRequestListCmd{})
	cmd.AddCommand("create", "Create a VolumeSeriesRequest", "Create a VolumeSeriesRequest object.", &volumeSeriesRequestCreateCmd{})
	cmd.AddCommand("delete", "Delete a VolumeSeriesRequest", "Delete a VolumeSeriesRequest object.", &volumeSeriesRequestDeleteCmd{})
	cmd.AddCommand("get", "Get a VolumeSeriesRequest", "Get a VolumeSeriesRequest object.", &volumeSeriesRequestFetchCmd{})
	cmd.AddCommand("cancel", "Cancel a VolumeSeriesRequest", "Cancel a VolumeSeriesRequest object.", &volumeSeriesRequestCancelCmd{})
	cmd.AddCommand("show-columns", "Show VolumeSeriesRequest table columns", "Show names of columns used in table format.", &showColsCmd{columns: volumeSeriesRequestHeaders})
}

type volumeSeriesRequestCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string
	activeReq    bool

	cacheHelper
}

// volumeSeriesRequest record keys/headers and their description
var volumeSeriesRequestHeaders = map[string]string{
	hCluster:             dCluster, // pseudo-column
	hCompleteBy:          "The completion deadline.",
	hID:                  dID,
	hNode:                dNodeName, // pseudo-column
	hRequestState:        "The request state.",
	hRequestedOperations: "The operations.",
	hTimeCreated:         dTimeCreated,
	hTimeModified:        dTimeModified,
	hVsID:                "The VolumeSeries object ID.",
	hTimeDuration:        "The total time taken on completion",
	hObjectID:            "The object concerned",
	hProgress:            "The percent complete",
}

var volumeSeriesRequestDefaultHeaders = []string{hID, hTimeCreated, hTimeDuration, hProgress, hRequestState, hRequestedOperations, hObjectID}

var terminalVolumeSeriesRequestStates = []string{
	"SUCCEEDED",
	"FAILED",
	"CANCELED",
}

// makeRecord creates a map of properties
func (c *volumeSeriesRequestCmd) makeRecord(o *models.VolumeSeriesRequest, now time.Time) map[string]string {
	var cluster string
	if cn, ok := c.clusters[string(o.ClusterID)]; ok { // assume map
		cluster = cn.name
	} else {
		cluster = string(o.ClusterID)
	}
	var node string
	if cn, ok := c.nodes[string(o.NodeID)]; ok { // assume map
		node = cn.name
	} else {
		node = string(o.NodeID)
	}
	obj := string(o.VolumeSeriesID)
	if util.Contains(o.RequestedOperations, common.VolReqOpCGCreateSnapshot) {
		obj = string(o.ConsistencyGroupID)
	}
	if util.Contains(o.RequestedOperations, common.VolReqOpAllocateCapacity) {
		obj = string(o.ServicePlanAllocationID)
	}
	pct := ""
	if o.Progress != nil {
		pct = fmt.Sprintf("%d%%", swag.Int32Value(o.Progress.PercentComplete))
	}
	var tDur string
	if util.Contains(terminalVolumeSeriesRequestStates, o.VolumeSeriesRequestState) {
		tDur = fmt.Sprintf("%s", time.Time(o.Meta.TimeModified).Sub(time.Time(o.Meta.TimeCreated)).Round(time.Millisecond))
		pct = ""
	} else {
		c.activeReq = true
		tDur = fmt.Sprintf("%s", now.Sub(time.Time(o.Meta.TimeCreated)).Round(time.Second))
	}
	return map[string]string{
		hCluster:             cluster,
		hCompleteBy:          time.Time(o.CompleteByTime).Format(time.RFC3339),
		hID:                  string(o.Meta.ID),
		hNode:                node,
		hRequestState:        o.VolumeSeriesRequestState,
		hRequestedOperations: strings.Join(o.RequestedOperations, ", "),
		hTimeCreated:         time.Time(o.Meta.TimeCreated).Format(time.RFC3339),
		hTimeModified:        time.Time(o.Meta.TimeModified).Format(time.RFC3339),
		hVsID:                string(o.VolumeSeriesID),
		hTimeDuration:        tDur,
		hObjectID:            obj,
		hProgress:            pct,
	}
}

func (c *volumeSeriesRequestCmd) validateColumns(columns string) error {
	var err error
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(volumeSeriesRequestHeaders), volumeSeriesRequestDefaultHeaders)
	return err
}

func (c *volumeSeriesRequestCmd) Emit(data []*models.VolumeSeriesRequest) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.EmitJSON(data)
	case "yaml":
		return appCtx.EmitYAML(data)
	}
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

func (c *volumeSeriesRequestCmd) list(params *vsr.VolumeSeriesRequestListParams) ([]*models.VolumeSeriesRequest, error) {
	res, err := appCtx.API.VolumeSeriesRequest().VolumeSeriesRequestList(params)
	if err != nil {
		if e, ok := err.(*vsr.VolumeSeriesRequestListDefault); ok && e.Payload.Message != nil {
			return nil, fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res.Payload, nil
}

func (c *volumeSeriesRequestCmd) lookupVolumeSeries(name, accountName, accountID string) (*models.VolumeSeries, error) {
	vslParams := vs.NewVolumeSeriesListParams()
	vslParams.AccountID = swag.String(accountID)
	vslParams.Name = swag.String(name)
	vsCmd := &volumeSeriesCmd{}
	ret, err := vsCmd.list(vslParams)
	if err == nil {
		if len(ret) == 1 {
			return ret[0], nil
		}
		err = fmt.Errorf("volume series '%s' in account '%s' name not found", name, accountName)
	}
	return nil, err
}

type volumeSeriesRequestListCmd struct {
	RequestedOperations []string       `short:"O" long:"operation" description:"Operations to match, prefix with ! to negate (e.g. !MOUNT); repeat as needed"`
	DomainName          string         `short:"D" long:"domain" description:"Name of a cloud service provider domain"`
	ClusterName         string         `short:"C" long:"cluster-name" description:"Name of a cluster in a specified domain"`
	NodeName            string         `short:"N" long:"node-name" description:"The name of a node in a specified cluster"`
	NodeID              string         `long:"node-id" description:"A Node object identifier. Do not specify with node-name"`
	State               []string       `short:"s" long:"state" description:"A request state. Repeat flag for multiple states"`
	Name                string         `short:"n" long:"name" description:"A VolumeSeries name"`
	PoolID              string         `long:"pool-id" description:"The id of a pool"`
	ProtectionDomain    string         `long:"protection-domain" description:"The name of a ProtectionDomain object owned by the account in context, if protection-domain-id is not specified"`
	ProtectionDomainID  string         `long:"protection-domain-id" description:"The identifier of a ProtectionDomain object"`
	SnapshotID          string         `short:"S" long:"snapshot-id" description:"A Snapshot ID"`
	VolumeSeriesID      string         `short:"V" long:"volume-series-id" description:"The VolumeSeries ID. Alternative to VolumeSeries name and account"`
	Recent              util.Timestamp `short:"R" long:"active-or-modified-ge" description:"Active or recently modified requests. Specify the modification duration or an absolute RFC3339 time"`
	IsTerminated        bool           `long:"terminated" description:"Return only storage requests that have terminated"`
	NotTerminated       bool           `long:"active" description:"Return only storage requests that have not terminated"`
	Columns             string         `short:"c" long:"columns" description:"Comma separated list of column names"`
	Follow              bool           `short:"f" long:"follow-changes" description:"Monitor the system and repeat the command as relevant changes occur"`
	FollowNoClear       bool           `long:"follow-no-clear" description:"Do not clear the terminal (the default) when using follow-changes"`

	mustClearTerm bool
	params        *vsr.VolumeSeriesRequestListParams
	res           []*models.VolumeSeriesRequest
	volumeSeriesRequestCmd
	remainingArgsCatcher
}

func (c *volumeSeriesRequestListCmd) Execute(args []string) error {
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
	if c.Name != "" && appCtx.AccountID == "" {
		return fmt.Errorf("volume series name requires account name")
	}
	if c.IsTerminated && c.NotTerminated {
		return fmt.Errorf("do not specify 'terminated' and 'active' together")
	}
	if c.NodeName != "" && c.NodeID != "" {
		return fmt.Errorf("do not specify 'node-name' and 'node-id' together")
	}
	_, clusterID, nodeID, err := c.validateDomainClusterNodeNames(c.DomainName, c.ClusterName, c.NodeName)
	if err != nil {
		return err
	}
	volumeSeriesID := ""
	if c.Name != "" {
		vsObj, err := c.lookupVolumeSeries(c.Name, appCtx.Account, appCtx.AccountID)
		if err != nil {
			return err
		}
		volumeSeriesID = string(vsObj.Meta.ID)
	} else if c.VolumeSeriesID != "" {
		volumeSeriesID = c.VolumeSeriesID
	}
	params := vsr.NewVolumeSeriesRequestListParams()
	if len(c.State) > 0 {
		params.VolumeSeriesRequestState = c.State
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
		params.PoolID = &c.PoolID
	}
	var pdID string
	if c.ProtectionDomainID != "" {
		pdID = c.ProtectionDomainID
	} else if c.ProtectionDomain != "" {
		pdID, err = c.validateProtectionDomainName(c.ProtectionDomain)
		if err != nil {
			return err
		}
	}
	if pdID != "" {
		params.ProtectionDomainID = &pdID
	}
	if c.SnapshotID != "" {
		params.SnapshotID = &c.SnapshotID
	}
	if volumeSeriesID != "" {
		params.VolumeSeriesID = swag.String(volumeSeriesID)
	}
	if c.IsTerminated || c.NotTerminated {
		params.IsTerminated = &c.IsTerminated
	}
	for _, op := range c.RequestedOperations {
		if strings.HasPrefix(op, "!") {
			params.RequestedOperationsNot = append(params.RequestedOperationsNot, strings.TrimPrefix(op, "!"))
		} else {
			params.RequestedOperations = append(params.RequestedOperations, op)
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

func (c *volumeSeriesRequestListCmd) run() error {
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

func (c *volumeSeriesRequestListCmd) display() error {
	if c.mustClearTerm {
		appCtx.ClearTerminal()
	}
	return c.Emit(c.res)
}

// ChangeDetected is part of the ChangeDetector interface
func (c *volumeSeriesRequestListCmd) ChangeDetected() error {
	return c.run()
}

// NoChangeTick is part of the ChangeDetectorWithTicker interface
func (c *volumeSeriesRequestListCmd) NoChangeTick() error {
	if c.activeReq {
		return c.display()
	}
	return nil
}

// NoChangeTickerPeriod is part of the ChangeDetectorWithTicker interface
func (c *volumeSeriesRequestListCmd) NoChangeTickerPeriod() time.Duration {
	return time.Second
}

// WatcherArgs is part of the ChangeDetector interface
func (c *volumeSeriesRequestListCmd) WatcherArgs() *models.CrudWatcherCreateArgs {
	cm := &models.CrudMatcher{
		URIPattern: "/volume-series-requests/?",
	}
	var scopeB bytes.Buffer
	if c.params.ClusterID != nil {
		fmt.Fprintf(&scopeB, ".*clusterId:%s", *c.params.ClusterID)
	}
	if c.params.NodeID != nil {
		fmt.Fprintf(&scopeB, ".*nodeId:%s", *c.params.NodeID)
	}
	if c.params.VolumeSeriesID != nil {
		fmt.Fprintf(&scopeB, ".*volumeSeriesId:%s", *c.params.VolumeSeriesID)
	}
	if scopeB.Len() > 0 {
		cm.ScopePattern = scopeB.String()
	}
	return &models.CrudWatcherCreateArgs{
		Matchers: []*models.CrudMatcher{cm},
	}
}

var vsrOps = []string{ // note: keep RequestedOperations description in sync
	"ALLOCATE_CAPACITY",
	"ATTACH_FS",
	"BIND",
	"CG_SNAPSHOT_CREATE",
	"CHANGE_CAPACITY",
	"CONFIGURE",
	"CREATE",
	"CREATE_FROM_SNAPSHOT",
	"DELETE",
	"DELETE_SPA",
	"DETACH_FS",
	"MOUNT",
	"NODE_DELETE",
	"PUBLISH",
	"UNPUBLISH",
	"RENAME",
	"UNBIND",
	"UNMOUNT",
	"VOL_DETACH",
	"VOL_SNAPSHOT_CREATE",
	"VOL_SNAPSHOT_RESTORE",
}

type volumeSeriesRequestCreateCmd struct {
	RequestedOperations []string      `short:"O" long:"operation" description:"Requested operation: ALLOCATE_CAPACITY, ATTACH_FS, BIND, CG_SNAPSHOT_CREATE, CHANGE_CAPACITY, CONFIGURE, CREATE, CREATE_FROM_SNAPSHOT, DELETE, DELETE_SPA, DETACH_FS, MOUNT, NODE_DELETE, PUBLISH, RENAME, UNBIND, UNMOUNT, UNPUBLISH, VOL_DETACH, VOL_SNAPSHOT_CREATE, VOL_SNAPSHOT_RESTORE. Repeat flag for multiple operations" required:"yes"`
	PlanOnly            bool          `long:"plan-only" description:"Plan but do not execute operations"`
	CompleteBy          time.Duration `short:"x" long:"complete-by" description:"The deadline for completion" default:"30m"`

	DomainName  string `short:"D" long:"domain" description:"Name of a cloud service provider domain. Required if the operation is CREATE_FROM_SNAPSHOT, BIND, CONFIGURE, CG_SNAPSHOT_CREATE, PUBLISH, UNPUBLISH, MOUNT, UNMOUNT, ALLOCATE_CAPACITY, DELETE_SPA or DELETE"`
	ClusterName string `short:"C" long:"cluster-name" description:"Name of a cluster. Required if the operation is CREATE_FROM_SNAPSHOT, BIND, CONFIGURE, CG_SNAPSHOT_CREATE, PUBLISH, UNPUBLISH, MOUNT, UNMOUNT, ALLOCATE_CAPACITY, DELETE_SPA or DELETE"`
	NodeName    string `short:"N" long:"node-name" description:"The name of a node in a specified cluster. Required for CONFIGURE, MOUNT, NODE_DELETE, VOL_DETACH and CREATE_FROM_SNAPSHOT"`

	Name                  string   `short:"n" long:"name" description:"The VolumeSeries name. Required for CHANGE_CAPACITY, CONFIGURE, CREATE, ATTACH_FS, BIND, PUBLISH, DETACH_FS, UNPUBLISH, MOUNT, NODE_DELETE, UNBIND, UNMOUNT, DELETE, VOL_DETACH, VOL_SNAPSHOT_CREATE or VOL_SNAPSHOT_RESTORE. Required for CREATE_FROM_SNAPSHOT only if not specifying SnapshotID."`
	AuthorizedAccountName string   `short:"Z" long:"authorized-account" description:"Name of the authorized account. Optional for ALLOCATE_CAPACITY and DELETE_SPA. If not specified the context account is used"`
	VolumeSeriesID        string   `short:"V" long:"volume-series-id" description:"The VolumeSeries ID. Alternative to VolumeSeries name and account"`
	Description           string   `short:"d" long:"description" description:"The VolumeSeries description. Optional for CREATE"`
	ServicePlan           string   `short:"P" long:"service-plan" description:"The name of a service plan. Required for CREATE, ALLOCATE_CAPACITY and DELETE_SPA"`
	SizeBytes             string   `short:"b" long:"size-bytes" description:"The size of the VolumeSeries or ServicePlanAllocation. Specify a suffix of B, GB, GiB, etc.  Required for CREATE, ALLOCATE_CAPACITY and VOL_SNAPSHOT_RESTORE. Optional for CHANGE_CAPACITY"`
	SpaAdditionalBytes    string   `long:"spa-additional-bytes" description:"Additional capacity to obtain from the ServicePlanAllocation. Specify a suffix of B, GB, GiB, etc.  Optional for CHANGE_CAPACITY"`
	ApplicationGroups     []string `long:"application-group" description:"The name of a application group. Repeat as needed. Optional for CREATE"`
	ConsistencyGroup      string   `long:"consistency-group" description:"The name of a consistency group. Optional for CREATE, required for CG_SNAPSHOT_CREATE"`
	Tags                  []string `short:"t" long:"tag" description:"A tag value; repeat as needed. Optional for CREATE"`
	NewName               string   `long:"new-name" description:"The new VolumeSeries name. Required for RENAME and CREATE_FROM_SNAPSHOT"`
	FsType                string   `short:"F" long:"fs-type" description:"The file system type for a VolumeSeries. Options: ext4, xfs. Optional for PUBLISH, required for ATTACH_FS."`
	DriverType            string   `long:"driver-type" description:"The volume driver type. Options: flex, csi. Optional for PUBLISH."`
	TargetPath            string   `long:"target-path" description:"Filesystem mount point. Required for ATTACH_FS and DETACH_FS."`
	ReadOnly              bool     `long:"read-only" description:"Write access is not permitted. Optional for ATTACH_FS."`
	SnapshotID            string   `short:"S" long:"snapshot-id" description:"The Snapshot ID. Required for CREATE_FROM_SNAPSHOT and VOL_SNAPSHOT_RESTORE."`
	SyncCoordinatorID     string   `long:"sync-coordinator-id" description:"The identity of the co-ordinating NODE_DELETE VSR. Required for VOL_DETACH"`

	SnapIdentifier string   `long:"snap-identifier" description:"Deprecated. The snapshot identifier concerned. Required for CREATE_FROM_SNAPSHOT"`
	SnapVolumeName string   `long:"snap-volume-name" description:"Deprecated. The snapshot volume name if different from 'Name'. May be required for VOL_SNAPSHOT_RESTORE"`
	SnapVolumeID   string   `long:"snap-volume-id" description:"Deprecated. The snapshot volume ID. Alternative to SnapVolumeName"`
	SnapLocation   []string `long:"snap-location" description:"Deprecated. The name of a CSPDomain object that identifies a protection store containing the snapshot. Required for VOL_SNAPSHOT_RESTORE. Repeat as needed"`
	SnapPitID      string   `long:"snap-pit-identifier" description:"Deprecated. The PiT identifier of the snapshot. Required for VOL_SNAPSHOT_RESTORE"`

	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`

	volumeSeriesRequestCmd
	remainingArgsCatcher
}

func (c *volumeSeriesRequestCreateCmd) validateOperations() error {
	for _, op := range c.RequestedOperations {
		if !util.Contains(vsrOps, op) {
			return fmt.Errorf("invalid operation %s. Choices: %s", op, strings.Join(vsrOps, ", "))
		}
	}
	return nil
}

func (c *volumeSeriesRequestCreateCmd) Execute(args []string) error {
	var err error
	if err := c.verifyNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateOperations(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	if c.Name != "" && appCtx.AccountID == "" {
		return fmt.Errorf("volume series name requires account name")
	}
	// The service validates the right combination of parameters, so focus only on sufficient arguments to assemble the request.
	var sb int64
	if c.SizeBytes != "" {
		sb, err = units.ParseStrictBytes(c.SizeBytes)
		if err != nil {
			return err
		}
	}
	var sab int64
	if c.SpaAdditionalBytes != "" {
		sab, err = units.ParseStrictBytes(c.SpaAdditionalBytes)
		if err != nil {
			return err
		}
	}
	_, clusterID, nodeID, err := c.validateDomainClusterNodeNames(c.DomainName, c.ClusterName, c.NodeName)
	if err != nil {
		return err
	}
	var authAccountID, cgID string
	agIds := []models.ObjIDMutable{}
	if appCtx.Account != "" || c.ConsistencyGroup != "" || len(c.ApplicationGroups) > 0 {
		agIds, cgID, err = c.validateApplicationConsistencyGroupNames(c.ApplicationGroups, c.ConsistencyGroup)
		if err != nil {
			return err
		}
	}
	if c.AuthorizedAccountName != "" {
		if authAccountID, err = c.validateAccount(c.AuthorizedAccountName, ""); err != nil {
			return err
		}
	} else if appCtx.AccountID != "" {
		authAccountID = appCtx.AccountID
	}
	servicePlanID := ""
	if c.ServicePlan != "" {
		if err = c.cacheServicePlans(); err != nil {
			return err
		}
		for id, n := range c.servicePlans {
			if n == c.ServicePlan {
				servicePlanID = id
			}
		}
		if servicePlanID == "" {
			return fmt.Errorf("service plan '%s' not found", c.ServicePlan)
		}
	}
	volumeSeriesID := ""
	if c.Name != "" && appCtx.AccountID != "" && !util.Contains(c.RequestedOperations, "CREATE") {
		vsObj, err := c.lookupVolumeSeries(c.Name, appCtx.Account, appCtx.AccountID)
		if err != nil {
			return err
		}
		volumeSeriesID = string(vsObj.Meta.ID)
	} else if c.VolumeSeriesID != "" {
		volumeSeriesID = c.VolumeSeriesID
	}
	snapVolumeID := ""
	if c.SnapVolumeName != "" && appCtx.AccountID != "" && !util.Contains(c.RequestedOperations, "CREATE") {
		snapVsObj, err := c.lookupVolumeSeries(c.SnapVolumeName, appCtx.Account, appCtx.AccountID)
		if err != nil {
			return err
		}
		snapVolumeID = string(snapVsObj.Meta.ID)
	} else if c.SnapVolumeID != "" {
		snapVolumeID = c.SnapVolumeID
	}
	req := &models.VolumeSeriesRequestCreateArgs{}
	req.VolumeSeriesCreateSpec = &models.VolumeSeriesCreateArgs{}
	req.ServicePlanAllocationCreateSpec = &models.ServicePlanAllocationCreateArgs{}
	req.Snapshot = &models.SnapshotData{}
	if clusterID != "" {
		req.ClusterID = models.ObjIDMutable(clusterID)
		req.ServicePlanAllocationCreateSpec.ClusterID = models.ObjIDMutable(clusterID)
	}
	req.CompleteByTime = strfmt.DateTime(time.Now().Add(c.CompleteBy))
	if nodeID != "" {
		req.NodeID = models.ObjIDMutable(nodeID)
	}
	if c.PlanOnly {
		req.PlanOnly = swag.Bool(c.PlanOnly)
	}
	req.RequestedOperations = c.RequestedOperations
	if appCtx.AccountID != "" {
		req.VolumeSeriesCreateSpec.AccountID = models.ObjIDMutable(appCtx.AccountID)
		req.ServicePlanAllocationCreateSpec.AccountID = models.ObjIDMutable(appCtx.AccountID)
	}
	if authAccountID != "" {
		req.ServicePlanAllocationCreateSpec.AuthorizedAccountID = models.ObjIDMutable(authAccountID)
	}
	if len(agIds) > 0 {
		req.ApplicationGroupIds = agIds
	}
	if cgID != "" {
		req.VolumeSeriesCreateSpec.ConsistencyGroupID = models.ObjIDMutable(cgID)
		req.ConsistencyGroupID = models.ObjIDMutable(cgID)
	}
	req.VolumeSeriesCreateSpec.Description = models.ObjDescription(c.Description)
	if util.Contains(c.RequestedOperations, "CREATE") {
		req.VolumeSeriesCreateSpec.Name = models.ObjName(c.Name)
	} else if util.Contains(c.RequestedOperations, "RENAME") || util.Contains(c.RequestedOperations, "CREATE_FROM_SNAPSHOT") {
		req.VolumeSeriesCreateSpec.Name = models.ObjName(c.NewName)
	}
	if servicePlanID != "" {
		req.VolumeSeriesCreateSpec.ServicePlanID = models.ObjIDMutable(servicePlanID)
		req.ServicePlanAllocationCreateSpec.ServicePlanID = models.ObjIDMutable(servicePlanID)
	}
	if c.SizeBytes != "" {
		req.VolumeSeriesCreateSpec.SizeBytes = &sb
		req.ServicePlanAllocationCreateSpec.TotalCapacityBytes = &sb
		req.Snapshot.SizeBytes = &sb
	}
	if c.SpaAdditionalBytes != "" {
		req.VolumeSeriesCreateSpec.SpaAdditionalBytes = &sab
	}
	req.VolumeSeriesCreateSpec.Tags = models.ObjTags(c.Tags)
	req.ServicePlanAllocationCreateSpec.Tags = models.ObjTags(c.Tags)
	if volumeSeriesID != "" {
		req.VolumeSeriesID = models.ObjIDMutable(volumeSeriesID)
		req.Snapshot.VolumeSeriesID = models.ObjIDMutable(volumeSeriesID)
	}
	if c.SnapIdentifier != "" {
		req.SnapIdentifier = c.SnapIdentifier
		req.Snapshot.SnapIdentifier = c.SnapIdentifier
	}
	if snapVolumeID != "" {
		req.Snapshot.VolumeSeriesID = models.ObjIDMutable(snapVolumeID)
	}
	if c.SnapPitID != "" {
		req.Snapshot.PitIdentifier = c.SnapPitID
	}
	if len(c.SnapLocation) > 0 {
		locs := make([]*models.SnapshotLocation, 0, len(c.SnapLocation))
		for _, l := range c.SnapLocation {
			var loc *models.SnapshotLocation
			for id, name := range c.cspDomains {
				if l == name {
					loc = &models.SnapshotLocation{CspDomainID: models.ObjIDMutable(id)}
					break
				}
			}
			if loc == nil {
				return fmt.Errorf("snapshot location '%s' not found", l)
			}
			locs = append(locs, loc)
		}
		req.Snapshot.Locations = locs
	}
	if !(util.Contains(c.RequestedOperations, "CHANGE_CAPACITY") || util.Contains(c.RequestedOperations, "CREATE") || util.Contains(c.RequestedOperations, "CREATE_FROM_SNAPSHOT") || util.Contains(c.RequestedOperations, "RENAME")) {
		req.VolumeSeriesCreateSpec = nil
	}
	if !util.Contains(c.RequestedOperations, "ALLOCATE_CAPACITY") {
		req.ServicePlanAllocationCreateSpec = nil
	} else {
		req.ClusterID = ""
	}
	if util.Contains(c.RequestedOperations, "DELETE_SPA") {
		spaParams := service_plan_allocation.NewServicePlanAllocationListParams()
		spaParams.ClusterID = &clusterID
		spaParams.AuthorizedAccountID = &authAccountID
		spaParams.ServicePlanID = &servicePlanID
		spaCmd := &servicePlanAllocationCmd{}
		ret, err := spaCmd.list(spaParams)
		if err != nil {
			return err
		} else if len(ret) != 1 {
			return fmt.Errorf("service plan allocation not found for the specified cluster, authorized account and service plan")
		}
		req.ServicePlanAllocationID = models.ObjIDMutable(ret[0].Meta.ID)
	}
	if util.Contains(c.RequestedOperations, "CREATE_FROM_SNAPSHOT") || util.Contains(c.RequestedOperations, "VOL_SNAPSHOT_RESTORE") {
		// TBD Fetch snapshot object
		if util.Contains(c.RequestedOperations, "CREATE_FROM_SNAPSHOT") {
			req.VolumeSeriesID = ""
			req.Snapshot.SizeBytes = nil
		}
		req.SnapIdentifier = ""
		if c.SnapshotID != "" {
			req.Snapshot = nil
			req.SnapshotID = models.ObjIDMutable(c.SnapshotID)
		}
	} else {
		req.Snapshot = nil
	}
	if util.Contains(c.RequestedOperations, "PUBLISH") {
		req.FsType = c.FsType
		req.DriverType = c.DriverType
	}
	if util.Contains(c.RequestedOperations, "ATTACH_FS") {
		req.FsType = c.FsType
		req.TargetPath = c.TargetPath
		req.ReadOnly = c.ReadOnly
	}
	if util.Contains(c.RequestedOperations, "DETACH_FS") {
		req.TargetPath = c.TargetPath
	}
	params := vsr.NewVolumeSeriesRequestCreateParams().WithPayload(req)
	res, err := appCtx.API.VolumeSeriesRequest().VolumeSeriesRequestCreate(params)
	if err != nil {
		if e, ok := err.(*vsr.VolumeSeriesRequestCreateDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.VolumeSeriesRequest{res.Payload})
}

type volumeSeriesRequestDeleteCmd struct {
	Confirm bool `long:"confirm" description:"Confirm the deletion of the object"`

	volumeSeriesRequestCmd
	requiredIDRemainingArgsCatcher
}

func (c *volumeSeriesRequestDeleteCmd) Execute(args []string) error {
	if err := c.verifyRequiredIDAndNoRemainingArgs(); err != nil {
		return err
	}
	if !c.Confirm {
		return fmt.Errorf("specify --confirm to delete the \"%s\" VolumeSeriesRequest object", c.ID)
	}
	if err := appCtx.InitContextAccount(); err != nil {
		return err
	}
	dParams := vsr.NewVolumeSeriesRequestDeleteParams()
	dParams.ID = string(c.ID)
	if _, err := appCtx.API.VolumeSeriesRequest().VolumeSeriesRequestDelete(dParams); err != nil {
		if e, ok := err.(*vsr.VolumeSeriesRequestDeleteDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return nil
}

type volumeSeriesRequestFetchCmd struct {
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`

	volumeSeriesRequestCmd
	requiredIDRemainingArgsCatcher
}

func (c *volumeSeriesRequestFetchCmd) Execute(args []string) error {
	var err error
	if err := c.verifyRequiredIDAndNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err := appCtx.InitContextAccount(); err != nil {
		return err
	}
	fParams := vsr.NewVolumeSeriesRequestFetchParams()
	fParams.ID = string(c.ID)
	res, err := appCtx.API.VolumeSeriesRequest().VolumeSeriesRequestFetch(fParams)
	if err != nil {
		if e, ok := err.(*vsr.VolumeSeriesRequestFetchDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.VolumeSeriesRequest{res.Payload})
}

type volumeSeriesRequestCancelCmd struct {
	Confirm bool `long:"confirm" description:"Confirm the cancellation of the request"`

	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`

	volumeSeriesRequestCmd
	requiredIDRemainingArgsCatcher
}

func (c *volumeSeriesRequestCancelCmd) Execute(args []string) error {
	if err := c.verifyRequiredIDAndNoRemainingArgs(); err != nil {
		return err
	}
	if err := c.validateColumns(c.Columns); err != nil {
		return err
	}
	if !c.Confirm {
		return fmt.Errorf("specify --confirm to cancel the \"%s\" VolumeSeriesRequest object", c.ID)
	}
	if err := appCtx.InitContextAccount(); err != nil {
		return err
	}
	cParams := vsr.NewVolumeSeriesRequestCancelParams()
	cParams.ID = string(c.ID)
	res, err := appCtx.API.VolumeSeriesRequest().VolumeSeriesRequestCancel(cParams)
	if err != nil {
		if e, ok := err.(*vsr.VolumeSeriesRequestCancelDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.VolumeSeriesRequest{res.Payload})
}
