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


package vra

import (
	"fmt"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/go-openapi/swag"
)

// SupportedVolumeSeriesRequestStates returns the list of VolumeSeriesRequest states
func SupportedVolumeSeriesRequestStates() []string {
	return util.StringKeys(stateData)
}

// SupportedVolumeSeriesRequestOperations returns the list of VolumeSeriesRequest operations
func SupportedVolumeSeriesRequestOperations() []string {
	return util.StringKeys(initialProcessOfOperation)
}

// ValidateVolumeSeriesRequestState verifies a volumeSeriesRequest state
func ValidateVolumeSeriesRequestState(state string) bool {
	_, ok := stateData[state]
	return ok
}

var terminalVolumeSeriesRequestStates = []string{
	com.VolReqStateSucceeded,
	com.VolReqStateFailed,
	com.VolReqStateCanceled,
}

// TerminalVolumeSeriesRequestStates returns a list of terminal volumeSeriesRequest states
func TerminalVolumeSeriesRequestStates() []string {
	return terminalVolumeSeriesRequestStates
}

// VolumeSeriesRequestStateIsTerminated verifies if a volumeSeriesRequest state is a terminal state
func VolumeSeriesRequestStateIsTerminated(state string) bool {
	return util.Contains(terminalVolumeSeriesRequestStates, state)
}

var configuredVolumeSeriesStates = []string{
	com.VolStateConfigured,
	com.VolStateInUse,
}

// VolumeSeriesIsConfigured returns true if a volume is configured
func VolumeSeriesIsConfigured(state string) bool {
	return util.Contains(configuredVolumeSeriesStates, state)
}

// VolumeSeriesIsProvisioned returns true if a volume is provisioned
func VolumeSeriesIsProvisioned(state string) bool {
	return state == com.VolStateProvisioned || VolumeSeriesIsConfigured(state)
}

// VolumeSeriesIsBound returns true if a volume is bound to a cluster
func VolumeSeriesIsBound(state string) bool {
	return state == com.VolStateBound || VolumeSeriesIsProvisioned(state)
}

// VolumeSeriesIsPublished returns true if a volume is published
func VolumeSeriesIsPublished(vs *models.VolumeSeries) bool {
	return len(vs.ClusterDescriptor) != 0 && VolumeSeriesIsBound(vs.VolumeSeriesState)
}

// VolumeSeriesFsIsAttached returns true if the filesystem on the volume media is attached
func VolumeSeriesFsIsAttached(vs *models.VolumeSeries) bool {
	if vs.VolumeSeriesState != com.VolStateInUse {
		return false
	}
	stag := util.NewTagList(vs.SystemTags)
	_, ok := stag.Get(com.SystemTagVolumeFsAttached)
	return ok
}

// VolumeSeriesHeadIsMounted returns true if the vs head is mounted
func VolumeSeriesHeadIsMounted(vs *models.VolumeSeries) bool {
	for _, m := range vs.Mounts {
		if m.MountState == com.VolMountStateMounted { // IN_USE does not guarantee MOUNTED
			if m.SnapIdentifier == com.VolMountHeadIdentifier {
				return true // head node
			}
		}
	}
	return false
}

// VolumeSeriesCacheIsRequested returns true if the vs head is mounted and cache was requested (but may not have been allocated)
func VolumeSeriesCacheIsRequested(vs *models.VolumeSeries) bool {
	if VolumeSeriesHeadIsMounted(vs) {
		for _, a := range vs.CacheAllocations {
			if swag.Int64Value(a.RequestedSizeBytes) > 0 {
				return true
			}
		}
	}
	return false
}

// AnimatorProcess identifies the type of process that performs a state
type AnimatorProcess int

// AnimatorProcess values
const (
	ApUnknown AnimatorProcess = iota // in StateData this is used where the process cannot be determined statically
	ApAgentd
	ApCentrald
	ApClusterd
)

func (ap AnimatorProcess) String() string {
	switch ap {
	case ApAgentd:
		return "agentd"
	case ApCentrald:
		return "centrald"
	case ApClusterd:
		return "clusterd"
	}
	return "unknown"
}

// StateInfo contains information on a request state
type StateInfo struct {
	// The order is used to ensure that we do not repeat a state on restart
	Order int
	// The process in which the state is animated
	Process AnimatorProcess
	// Some states have UNDO counterparts
	Undo string
	// internal fields
	name string // set via an init function
}

// Name returns the state name
func (si StateInfo) Name() string {
	return si.name
}

// GetProcess returns the process animating the state or ApUnknown
func (si StateInfo) GetProcess(firstOp string) AnimatorProcess {
	if si.name == com.VolReqStateNew {
		ap, found := initialProcessOfOperation[firstOp]
		if !found {
			return ApUnknown
		}
		return ap
	}
	return si.Process
}

// IsNormal returns true if the state is a normal state
func (si StateInfo) IsNormal() bool {
	return si.Order < soMinUndoStateOrder
}

// IsUndo returns true if the state is an undo state
func (si StateInfo) IsUndo() bool {
	return si.Order >= soMinUndoStateOrder && si.Order < soTerminationStatesOrder
}

// IsTerminal returns true if the state is a terminal state
func (si StateInfo) IsTerminal() bool {
	return si.Order >= soTerminationStatesOrder
}

// StateDataMap contains ordering, undo and process information by state.
// The NEW state is special in that the process can only be determined based on runtime operations;
// likewise the termination states have no specific process.
// All other states have a 1:1 relationship with a specific process type.
// (Note that a single process may implement the same state with more than one interface, depending
// on the runtime operations involved).
type StateDataMap map[string]StateInfo

// Order returns the order of a given state or -1 if not in the map
// Note multiple states may have the same order.
func (sdm StateDataMap) Order(state string) int {
	sd, found := sdm[state]
	if !found {
		return -1
	}
	return sd.Order
}

// Undo returns the "undo" state for a given state, if any
func (sdm StateDataMap) Undo(state string) string {
	sd, found := sdm[state]
	if !found {
		return ""
	}
	return sd.Undo
}

// Process returns the process for a state/first-operation combination or ApUnknown
// The fist operation is used only if the state is NEW.
func (sdm StateDataMap) Process(state string, firstOp string) AnimatorProcess {
	sd, found := sdm[state]
	if !found {
		return ApUnknown
	}
	return sd.GetProcess(firstOp)
}

// initialProcessOfOperation map
var initialProcessOfOperation = map[string]AnimatorProcess{
	com.VolReqOpAllocateCapacity:   ApCentrald,
	com.VolReqOpAttachFs:           ApAgentd,
	com.VolReqOpBind:               ApCentrald,
	com.VolReqOpCGCreateSnapshot:   ApClusterd,
	com.VolReqOpChangeCapacity:     ApCentrald,
	com.VolReqOpConfigure:          ApClusterd,
	com.VolReqOpCreate:             ApCentrald,
	com.VolReqOpCreateFromSnapshot: ApClusterd,
	com.VolReqOpDelete:             ApCentrald,
	com.VolReqOpDeleteSPA:          ApCentrald,
	com.VolReqOpDetachFs:           ApAgentd,
	com.VolReqOpMount:              ApClusterd,
	com.VolReqOpNodeDelete:         ApCentrald,
	com.VolReqOpPublish:            ApClusterd,
	com.VolReqOpRename:             ApCentrald,
	com.VolReqOpUnbind:             ApCentrald,
	com.VolReqOpUnmount:            ApAgentd,
	com.VolReqOpUnpublish:          ApClusterd,
	com.VolReqOpVolCreateSnapshot:  ApAgentd,
	com.VolReqOpVolDetach:          ApCentrald,
	com.VolReqOpVolRestoreSnapshot: ApAgentd,
}

const (
	soMinUndoStateOrder      = 501
	soTerminationStatesOrder = 999
)

// stateData contains ordering, undo and process information by state.
var stateData = StateDataMap{
	// sort by Order, ascending
	com.VolReqStateNew:                     {Order: 0},
	com.VolReqStateCreating:                {Order: 10, Process: ApCentrald, Undo: com.VolReqStateUndoCreating},
	com.VolReqStateCapacityWait:            {Order: 20, Process: ApCentrald},
	com.VolReqStateBinding:                 {Order: 20, Process: ApCentrald, Undo: com.VolReqStateUndoBinding},
	com.VolReqStatePublishing:              {Order: 30, Process: ApClusterd, Undo: com.VolReqStateUndoPublishing},
	com.VolReqStateChoosingNode:            {Order: 38, Process: ApClusterd},
	com.VolReqStateSizing:                  {Order: 40, Process: ApClusterd, Undo: com.VolReqStateUndoSizing},
	com.VolReqStateStorageWait:             {Order: 50, Process: ApClusterd},
	com.VolReqStatePlacement:               {Order: 50, Process: ApClusterd, Undo: com.VolReqStateUndoPlacement},
	com.VolReqStatePlacementReattach:       {Order: 50, Process: ApClusterd}, // like PLACEMENT, but only to re-attach storage with no undo path
	com.VolReqStateVolumeConfig:            {Order: 60, Process: ApAgentd, Undo: com.VolReqStateUndoVolumeConfig},
	com.VolReqStateVolumeExport:            {Order: 70, Process: ApAgentd, Undo: com.VolReqStateUndoVolumeExport},
	com.VolReqStateSnapshotRestore:         {Order: 80, Process: ApAgentd, Undo: com.VolReqStateUndoSnapshotRestore},
	com.VolReqStateSnapshotRestoreFinalize: {Order: 90, Process: ApAgentd},
	com.VolReqStateAttachingFs:             {Order: 95, Process: ApAgentd, Undo: com.VolReqStateUndoAttachingFs},
	com.VolReqStateChangingCapacity:        {Order: 100, Process: ApCentrald, Undo: com.VolReqStateUndoChangingCapacity},
	com.VolReqStateResizingCache:           {Order: 102, Process: ApClusterd, Undo: com.VolReqStateUndoResizingCache},
	com.VolReqStateReallocatingCache:       {Order: 104, Process: ApAgentd, Undo: com.VolReqStateUndoReallocatingCache},
	com.VolReqStateRenaming:                {Order: 110, Process: ApCentrald, Undo: com.VolReqStateUndoRenaming},
	com.VolReqStateCreatingFromSnapshot:    {Order: 130, Process: ApClusterd, Undo: com.VolReqStateUndoCreatingFromSnapshot},
	com.VolReqStateSnapshotRestoreDone:     {Order: 140, Process: ApClusterd},
	com.VolReqStateAllocatingCapacity:      {Order: 150, Process: ApCentrald, Undo: com.VolReqStateUndoAllocatingCapacity},
	com.VolReqStatePublishingServicePlan:   {Order: 151, Process: ApClusterd},
	com.VolReqStateCGSnapshotVolumes:       {Order: 170, Process: ApClusterd, Undo: com.VolReqStateUndoCGSnapshotVolumes},
	com.VolReqStateCGSnapshotWait:          {Order: 180, Process: ApClusterd, Undo: com.VolReqStateUndoCGSnapshotVolumes},
	com.VolReqStateCGSnapshotFinalize:      {Order: 190, Process: ApClusterd},
	com.VolReqStatePausingIO:               {Order: 250, Process: ApAgentd, Undo: com.VolReqStateUndoPausingIO},
	com.VolReqStatePausedIO:                {Order: 260, Process: ApAgentd, Undo: com.VolReqStateUndoPausedIO},
	com.VolReqStateCreatingPiT:             {Order: 270, Process: ApAgentd, Undo: com.VolReqStateUndoCreatingPiT},
	com.VolReqStateCreatedPiT:              {Order: 280, Process: ApAgentd, Undo: com.VolReqStateUndoCreatedPiT},
	com.VolReqStateSnapshotUploading:       {Order: 290, Process: ApAgentd, Undo: com.VolReqStateUndoSnapshotUploading},
	com.VolReqStateSnapshotUploadDone:      {Order: 300, Process: ApAgentd, Undo: com.VolReqStateUndoSnapshotUploadDone},
	com.VolReqStateFinalizingSnapshot:      {Order: 310, Process: ApAgentd},
	com.VolReqStateDrainingRequests:        {Order: 400, Process: ApCentrald}, // NODE_DELETE
	com.VolReqStateCancelingRequests:       {Order: 410, Process: ApCentrald}, // NODE_DELETE
	com.VolReqStateDetachingVolumes:        {Order: 420, Process: ApCentrald}, // NODE_DELETE
	com.VolReqStateDetachingStorage:        {Order: 430, Process: ApCentrald}, // NODE_DELETE
	com.VolReqStateVolumeDetachWait:        {Order: 440, Process: ApCentrald}, // NODE_DELETE & VOL_DETACH
	com.VolReqStateVolumeDetaching:         {Order: 450, Process: ApCentrald}, // VOL_DETACH
	com.VolReqStateVolumeDetached:          {Order: 460, Process: ApCentrald}, // NODE_DELETE & VOL_DETACH
	com.VolReqStateDeletingNode:            {Order: 470, Process: ApCentrald}, // NODE_DELETE

	// undo states >= soMinUndoStateOrder
	com.VolReqStateUndoAttachingFs:          {Order: 505, Process: ApAgentd},
	com.VolReqStateUndoSnapshotRestore:      {Order: 510, Process: ApAgentd},
	com.VolReqStateUndoVolumeExport:         {Order: 520, Process: ApAgentd},
	com.VolReqStateUndoVolumeConfig:         {Order: 530, Process: ApAgentd},
	com.VolReqStateUndoPlacement:            {Order: 540, Process: ApClusterd},
	com.VolReqStateUndoSizing:               {Order: 550, Process: ApClusterd},
	com.VolReqStateUndoPublishing:           {Order: 560, Process: ApClusterd},
	com.VolReqStateUndoBinding:              {Order: 570, Process: ApCentrald},
	com.VolReqStateUndoCreating:             {Order: 580, Process: ApCentrald},
	com.VolReqStateUndoReallocatingCache:    {Order: 586, Process: ApAgentd},
	com.VolReqStateUndoResizingCache:        {Order: 588, Process: ApClusterd},
	com.VolReqStateUndoChangingCapacity:     {Order: 590, Process: ApCentrald},
	com.VolReqStateUndoRenaming:             {Order: 600, Process: ApCentrald},
	com.VolReqStateUndoCreatingFromSnapshot: {Order: 610, Process: ApClusterd},
	com.VolReqStateUndoAllocatingCapacity:   {Order: 620, Process: ApCentrald},
	com.VolReqStateDeletingSPA:              {Order: 630, Process: ApCentrald}, // "pseudo" undo operation, executes the undo path for normal operation
	com.VolReqStateUndoCGSnapshotVolumes:    {Order: 730, Process: ApClusterd},
	com.VolReqStateUndoSnapshotUploadDone:   {Order: 740, Process: ApAgentd},
	com.VolReqStateUndoSnapshotUploading:    {Order: 750, Process: ApAgentd},
	com.VolReqStateUndoCreatedPiT:           {Order: 760, Process: ApAgentd},
	com.VolReqStateUndoCreatingPiT:          {Order: 770, Process: ApAgentd},
	com.VolReqStateUndoPausedIO:             {Order: 780, Process: ApAgentd},
	com.VolReqStateUndoPausingIO:            {Order: 790, Process: ApAgentd},

	// terminal states == soTerminationStatesOrder
	com.VolReqStateSucceeded: {Order: 999},
	com.VolReqStateFailed:    {Order: 999},
	com.VolReqStateCanceled:  {Order: 999},
}

func init() {
	for k, sd := range stateData {
		sd.name = k
		stateData[k] = sd
	}
}

// GetStateInfo returns the StateInfo structure for a given state.
// It panics if the state is not known.
func GetStateInfo(state string) StateInfo {
	sd, ok := stateData[state]
	if !ok {
		panic(fmt.Sprintf("GetStateInfo: unknown state '%s'", state))
	}
	return sd // returned by value
}

var validNodeDeleteLaunchStates = []string{
	com.NodeStateTearDown,
	com.NodeStateTimedOut,
}

// ValidNodeStatesForNodeDelete describes the node states where we can issue a node delete operation.
func ValidNodeStatesForNodeDelete() []string {
	return validNodeDeleteLaunchStates
}

var validNodeStatesForVolDetach = []string{
	com.NodeStateTearDown,
}

// ValidNodeStatesForVolDetach describes the node states where we can issue a vol detach operation
func ValidNodeStatesForVolDetach() []string {
	return validNodeStatesForVolDetach
}
