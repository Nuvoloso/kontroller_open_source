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
	"sort"
	"strings"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/client/snapshot"
	vs "github.com/Nuvoloso/kontroller/pkg/autogen/client/volume_series"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
)

func init() {
	initSnapshot()
}

func initSnapshot() {
	cmd, _ := parser.AddCommand("snapshot", "Snapshot object commands", "Snapshot object subcommands", &snapshotCmd{})
	cmd.Aliases = []string{"snap", "snapshots", "sn"}
	cmd.AddCommand("list", "List Snapshots", "List or search for Snapshot objects.", &snapshotListCmd{})
	cmd.AddCommand("get", "Get a Snapshot", "Get a Snapshot object.", &snapshotFetchCmd{})
	cmd.AddCommand("modify", "Modify a Snapshot", "Modify a Snapshot object. Use a whitespace-only string to clear trimmed properties.", &snapshotModifyCmd{})
	cmd.AddCommand("show-columns", "Show Snapshot table columns", "Show names of columns used in table format", &showColsCmd{columns: snapshotHeaders})
}

type snapshotCmd struct {
	OutputFormat string `hidden:"1" short:"o" long:"output" description:"Output format control" choice:"json" choice:"table" choice:"yaml" default:"table"`
	tableCols    []string

	cacheHelper
}

// snapshot record keys/headers and their description
var snapshotHeaders = map[string]string{
	hID:               dID,
	hVersion:          dVersion,
	hAccount:          dAccount,
	hTenant:           "The encompassing tenant account for the account.",
	hPitIdentifier:    dPitIdentifier,
	hSnapIdentifier:   dSnapIdentifier,
	hSnapTime:         "The time at which the PiT was made",
	hVolumeSeries:     "The VolumeSeries object name for which the snapshot is created.",
	hConsistencyGroup: "The consistency group to which the volume series belongs.",
	hSizeBytes:        "The size in bytes.",
	hDeleteAfterTime:  "The time after which the snapshot can be purged.",
	hLocations:        "A mapping of CSPDomainID to SnapshotLocation.",
	hSystemTags:       "Arbitrary system tags string used for searching.",
	hTags:             dTags,
}

var snapshotDefaultHeaders = []string{hID, hSnapTime, hVolumeSeries, hConsistencyGroup, hSizeBytes, hLocations}

// makeRecord creates a map of properties
func (c *snapshotCmd) makeRecord(o *models.Snapshot) map[string]string {
	var account, cgName, sb, vsName string
	if a, ok := c.accounts[string(o.AccountID)]; !ok {
		account = string(o.AccountID)
	} else {
		account = a.name
	}
	tenantAccountName := string(o.TenantAccountID)
	if o.TenantAccountID != "" {
		if a, ok := c.accounts[string(o.TenantAccountID)]; ok {
			tenantAccountName = a.name
		}
	}
	if vs, ok := c.volumeSeries[string(o.VolumeSeriesID)]; ok {
		vsName = vs.name
	} else {
		vsName = string(o.VolumeSeriesID)
	}
	if swag.Int64Value(&o.SizeBytes) != 0 {
		sb = sizeToString(swag.Int64Value(&o.SizeBytes))
	}
	if cg, ok := c.consistencyGroups[string(o.ConsistencyGroupID)]; ok {
		cgName = cg.name
	} else {
		cgName = string(o.ConsistencyGroupID)
	}
	locations := make([]string, 0, len(o.Locations))
	for cspDomainID := range o.Locations {
		scpDomainName := cspDomainID // fall back on the ID if name is not found
		for id, name := range c.cspDomains {
			if id == cspDomainID {
				scpDomainName = name
			}
		}
		locations = append(locations, scpDomainName)
	}
	sort.Strings(locations)
	return map[string]string{
		hID:               string(o.Meta.ID),
		hVersion:          fmt.Sprintf("%d", o.Meta.Version),
		hAccount:          account,
		hTenant:           tenantAccountName,
		hPitIdentifier:    string(o.PitIdentifier),
		hSnapIdentifier:   string(o.SnapIdentifier),
		hSnapTime:         time.Time(o.SnapTime).Format(time.RFC3339),
		hVolumeSeries:     vsName,
		hConsistencyGroup: cgName,
		hSizeBytes:        sb,
		hDeleteAfterTime:  time.Time(o.DeleteAfterTime).Format(time.RFC3339),
		hLocations:        strings.Join(locations, ", "),
		hSystemTags:       strings.Join(o.SystemTags, ", "),
		hTags:             strings.Join(o.Tags, ", "),
	}
}

func (c *snapshotCmd) validateColumns(columns string) error {
	var err error
	c.tableCols, err = appCtx.parseColumns(columns, util.StringKeys(snapshotHeaders), snapshotDefaultHeaders)
	return err
}

func (c *snapshotCmd) Emit(data []*models.Snapshot) error {
	switch c.OutputFormat {
	case "json":
		return appCtx.EmitJSON(data)
	case "yaml":
		return appCtx.EmitYAML(data)
	}
	c.cacheAccounts()
	c.cacheConsistencyGroups()
	c.cacheCSPDomains()
	rows := make([][]string, len(data))
	for i, o := range data {
		c.cacheVolumeSeries(string(o.VolumeSeriesID), "", "")
		rec := c.makeRecord(o)
		row := make([]string, len(c.tableCols))
		for j, h := range c.tableCols {
			row[j] = rec[h]
		}
		rows[i] = row
	}
	return appCtx.EmitTable(c.tableCols, rows, nil)
}

func (c *snapshotCmd) list(params *snapshot.SnapshotListParams) ([]*models.Snapshot, error) {
	res, err := appCtx.API.Snapshot().SnapshotList(params)
	if err != nil {
		if e, ok := err.(*snapshot.SnapshotListDefault); ok && e.Payload.Message != nil {
			return nil, fmt.Errorf("%s", *e.Payload.Message)
		}
		return nil, err
	}
	return res.Payload, nil
}

type snapshotFetchCmd struct {
	Columns string `short:"c" long:"columns" description:"Comma separated list of column names"`

	snapshotCmd
	requiredIDRemainingArgsCatcher
}

func (c *snapshotFetchCmd) Execute(args []string) error {
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
	fParams := snapshot.NewSnapshotFetchParams().WithID(c.ID)
	res, err := appCtx.API.Snapshot().SnapshotFetch(fParams)
	if err != nil {
		if e, ok := err.(*snapshot.SnapshotFetchDefault); ok && e.Payload.Message != nil {
			err = fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.Snapshot{res.Payload})
}

func (c *snapshotCmd) lookupAndCacheVolumeSeriesID(vsName, accountName, accountID string) (string, error) {
	vsCmd := &volumeSeriesCmd{}
	vsListRes, err := vsCmd.list(vs.NewVolumeSeriesListParams().WithName(&vsName).WithAccountID(&accountID))
	if err != nil {
		return "", err
	} else if len(vsListRes) != 1 {
		return "", fmt.Errorf("VolumeSeries '%s' not found for account '%s'", vsName, accountName)
	}
	c.cacheVolumeSeries(string(vsListRes[0].Meta.ID), string(vsListRes[0].Name), accountID)
	return string(vsListRes[0].Meta.ID), nil
}

type snapshotListCmd struct {
	OwnedOnly         bool           `long:"owned-only" description:"Retrieve only snapshots owned by the account, otherwise, if a tenant admin, retrieve snapshots owned by the tenant or any subordinate account. A no-op for non-tenant accounts"`
	ConsistencyGroup  string         `long:"consistency-group" description:"The name of a consistency group"`
	CountOnly         bool           `long:"count-only" description:"The number of items that would be returned"`
	DomainName        string         `short:"D" long:"domain" description:"Name of a cloud service provider domain"`
	DeleteAfterTimeLE util.Timestamp `long:"delete-after-time-le" description:"Upper bound of timestamp when the snapshot can be purged"`
	SnapIdentifier    string         `short:"S" long:"snap-id" description:"The identifier of a snapshot"`
	PitIdentifier     []string       `short:"P" long:"pit-id" description:"The identifier of a PiT. Repeat flag for multiple values"`
	ProtectionDomain   string         `long:"protection-domain" description:"The name of a ProtectionDomain object owned by the account in context, if protection-domain-id is not specified"`
	ProtectionDomainID string         `long:"protection-domain-id" description:"The identifier of a ProtectionDomain object"`
	Recent            util.Timestamp `short:"R" long:"recent-snapshots" description:"Recently created snapshots - only used with follow and overrides snap-time-ge. Specify the snapTime duration"`
	SnapTimeGE        util.Timestamp `long:"snap-time-ge" description:"Lower bound of snap time to list, specified either as a duration or an absolute RFC3339 time"`
	SnapTimeLE        util.Timestamp `long:"snap-time-le" description:"Upper bound of snap time to list, specified either as a duration or an absolute RFC3339 time"`
	SystemTags        []string       `long:"system-tag" description:"A system tag value; repeat as needed"`
	Tags              []string       `short:"t" long:"tag" description:"A tag value; repeat as needed"`
	VolumeSeries      string         `short:"V" long:"volume-series" description:"The VolumeSeries name"`
	SortAsc           []string       `long:"sort-asc" description:"Sort result in ascending order of the specified keys"`
	SortDesc          []string       `long:"sort-desc" description:"Sort result in descending order of the specified keys"`
	Skip              int32          `long:"skip" description:"The number of results to skip"`
	Limit             int32          `long:"limit" description:"Limit the number of results returned"`
	Columns           string         `short:"c" long:"columns" description:"Comma separated list of column names"`
	Follow            bool           `short:"f" long:"follow-changes" description:"Monitor the system and repeat the command as relevant changes occur"`
	FollowNoClear     bool           `long:"follow-no-clear" description:"Do not clear the terminal (the default) when using follow-changes"`
	mustClearTerm bool
	params        *snapshot.SnapshotListParams
	res           []*models.Snapshot

	snapshotCmd
	remainingArgsCatcher
}

func (c *snapshotListCmd) Execute(args []string) error {
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
	params := snapshot.NewSnapshotListParams()
	if appCtx.AccountID != "" && c.OwnedOnly {
		params.AccountID = &appCtx.AccountID
	}
	_, cgID, err := c.validateApplicationConsistencyGroupNames(nil, c.ConsistencyGroup)
	if err != nil {
		return err
	}
	if cgID != "" {
		params.ConsistencyGroupID = &cgID
	}
	if c.CountOnly {
		params.CountOnly = &c.CountOnly
	}
	var domainID string
	if c.DomainName != "" {
		if err = c.cacheCSPDomains(); err != nil {
			return err
		}
		for id, n := range c.cspDomains {
			if n == c.DomainName {
				domainID = id
			}
		}
		if domainID == "" {
			return fmt.Errorf("CSP domain '%s' not found", c.DomainName)
		}
	}
	if domainID != "" {
		params.CspDomainID = &domainID
	}
	if c.DeleteAfterTimeLE.Specified() {
		dt := c.DeleteAfterTimeLE.Value()
		params.DeleteAfterTimeLE = &dt
	}
	if c.SnapIdentifier != "" {
		params.SnapIdentifier = &c.SnapIdentifier
	}
	if c.SnapTimeGE.Specified() {
		dt := c.SnapTimeGE.Value()
		params.SnapTimeGE = &dt
	}
	if c.SnapTimeLE.Specified() {
		dt := c.SnapTimeLE.Value()
		params.SnapTimeLE = &dt
	}
	if len(c.PitIdentifier) > 0 {
		params.PitIdentifiers = c.PitIdentifier
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
	params.SystemTags = c.SystemTags
	params.Tags = c.Tags
	if c.VolumeSeries != "" {
		vsID, err := c.lookupAndCacheVolumeSeriesID(c.VolumeSeries, appCtx.Account, appCtx.AccountID)
		if err != nil {
			return err
		}
		params.VolumeSeriesID = &vsID
	}
	params.SortAsc = c.SortAsc
	params.SortDesc = c.SortDesc
	if c.Skip > 0 {
		params.Skip = &c.Skip
	}
	if c.Limit > 0 {
		params.Limit = &c.Limit
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

func (c *snapshotListCmd) run() error {
	var err error
	if c.Follow && c.Recent.Specified() {
		dt := c.Recent.Value()
		c.params.SnapTimeGE = &dt
	}
	if c.res, err = c.list(c.params); err != nil {
		return err
	}
	// TBD: work on cache refresh on demand
	return c.display()
}

func (c *snapshotListCmd) display() error {
	if c.mustClearTerm {
		appCtx.ClearTerminal()
	}
	return c.Emit(c.res)
}

// ChangeDetected is part of the ChangeDetector interface
func (c *snapshotListCmd) ChangeDetected() error {
	return c.run()
}

// WatcherArgs is part of the ChangeDetector interface
func (c *snapshotListCmd) WatcherArgs() *models.CrudWatcherCreateArgs {
	cm := &models.CrudMatcher{
		URIPattern: "/snapshots/?",
	}
	var scopeB bytes.Buffer
	if c.params.ConsistencyGroupID != nil {
		fmt.Fprintf(&scopeB, ".*consistencyGroupID:%s", *c.params.ConsistencyGroupID)
	}
	if c.params.CspDomainID != nil {
		fmt.Fprintf(&scopeB, ".*locations:.*%s", *c.params.CspDomainID)
	}
	if c.params.SnapIdentifier != nil {
		fmt.Fprintf(&scopeB, ".*snapIdentifier:%s", *c.params.SnapIdentifier)
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

// find Snapshot either by object ID if one is provided or by the other provided identifying args
func (c *snapshotModifyCmd) fetchSnapshotToUpdate() (*models.Snapshot, error) {
	if c.ID != "" {
		res, err := appCtx.API.Snapshot().SnapshotFetch(snapshot.NewSnapshotFetchParams().WithID(c.ID))
		if err != nil {
			if e, ok := err.(*snapshot.SnapshotFetchDefault); ok && e.Payload.Message != nil {
				return nil, fmt.Errorf("%s", *e.Payload.Message)
			}
			return nil, err
		}
		return res.Payload, nil
	} else if c.SnapIdentifier != "" && c.VolumeSeries != "" {
		vsID, err := c.lookupAndCacheVolumeSeriesID(c.VolumeSeries, appCtx.Account, appCtx.AccountID)
		if err != nil {
			return nil, err
		}
		_, cgID, err := c.validateApplicationConsistencyGroupNames(nil, c.ConsistencyGroup)
		if err != nil {
			return nil, err
		}
		lParams := snapshot.NewSnapshotListParams()
		lParams.SnapIdentifier = &c.SnapIdentifier
		lParams.VolumeSeriesID = &vsID
		lParams.ConsistencyGroupID = &cgID
		lRes, err := c.list(lParams)
		if err != nil {
			return nil, err
		}
		return lRes[0], nil
	}
	return nil, fmt.Errorf("Either Snapshot object ID or the pair of SnapIdentifier and VolumeSeries should be provided")
}

type snapshotModifyCmd struct {
	SnapIdentifier   string         `short:"S" long:"snap-id" description:"The identifier of a snapshot to be modified"`
	VolumeSeries     string         `long:"volume-series" description:"The VolumeSeries name of a snapshot to be modified"`
	ConsistencyGroup string         `long:"consistency-group" description:"The name of a consistency group of a snapshot to be modified"`
	DeleteAfterTime  util.Timestamp `long:"delete-after-time" description:"The time after which the snapshot can be purged"`
	Tags             []string       `short:"t" long:"tag" description:"A tag value; repeat as needed"`
	TagsAction       string         `long:"tag-action" description:"Specifies how to process tag values" choice:"APPEND" choice:"REMOVE" choice:"SET" default:"APPEND"`
	Columns          string         `short:"c" long:"columns" description:"Comma separated list of column names"`

	snapshotCmd
	optionalIDRemainingArgsCatcher
}

func (c *snapshotModifyCmd) Execute(args []string) error {
	var err error
	if err = c.verifyOptionalIDAndNoRemainingArgs(); err != nil {
		return err
	}
	if err = c.validateColumns(c.Columns); err != nil {
		return err
	}
	if err = appCtx.InitContextAccount(); err != nil {
		return err
	}
	nChg := 0
	uParams := snapshot.NewSnapshotUpdateParams().WithPayload(&models.SnapshotMutable{})
	if c.DeleteAfterTime.Specified() {
		uParams.Payload.DeleteAfterTime = c.DeleteAfterTime.Value()
		uParams.Set = append(uParams.Set, "deleteAfterTime")
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
	snapshotToUpdate, err := c.fetchSnapshotToUpdate()
	if err != nil {
		return err
	}
	uParams.ID = string(snapshotToUpdate.Meta.ID)
	var res *snapshot.SnapshotUpdateOK
	if res, err = appCtx.API.Snapshot().SnapshotUpdate(uParams); err != nil {
		if e, ok := err.(*snapshot.SnapshotUpdateDefault); ok && e.Payload.Message != nil {
			return fmt.Errorf("%s", *e.Payload.Message)
		}
		return err
	}
	return c.Emit([]*models.Snapshot{res.Payload})
}
