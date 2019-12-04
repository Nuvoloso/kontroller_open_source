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


package vreq

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/crud"
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/Nuvoloso/kontroller/pkg/vra"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	uuid "github.com/satori/go.uuid"
)

// configureSubState represents VOLUME_CONFIG sub-states.
// There is at most one database update in each sub-state.
type configureSubState int

// configureSubState values
const (
	// Query for all objects needed to complete the operation
	ConfigureStart configureSubState = iota
	// Tell Storelandia the locations of all devices to be used by this volume
	ConfigureSetNewDeviceLocations
	// Set the rootStorageID of the volume series
	ConfigureSetRootStorageID
	// On Restart, query for objects needed to continue and then ConfigureCreateLogVol
	ConfigureRestartAndCreate
	// Create a new LogVol
	ConfigureCreateLogVol
	// This VSR created the LogVol, but it might have been closed if Storelandia restarted
	ConfigureRestartAndOpenCreated
	// Open a LogVol that this VSR created before restart, then ConfigureAllocParcels
	ConfigureOpenCreatedLogVol
	// Make nuvo api AllocParcels calls for all parcels listed in the VolumeSeries.StorageParcels
	ConfigureAllocParcels
	// Query for all objects needed to complete configuring an existing volume
	ConfigureStartExisting
	// Tell Storelandia the locations of all devices used by this existing volume
	ConfigureSetExistingDeviceLocations
	// Open an existing LogVol (requires no new parcel allocation)
	ConfigureOpenLogVol
	// Set the VolumeSeries to CONFIGURED
	ConfigureSetConfigured
	// Return
	ConfigureDone
	// Set the VolumeSeries to PROVISIONED
	ConfigureSetProvisioned
	// Close the LogVol that was just opened
	ConfigureCloseLogVol
	// Change the state of the VolumeSeries to DELETING
	ConfigureSetDeleting
	// Close the LogVol (if present) so it can be deleted
	ConfigureCloseDeleteLogVol
	// Reload configuration information prior to delete in case not known to Nuvo
	ConfigurePreDeleteReloadConfig
	// Reconfigure device locations prior to delete in case not known to Nuvo
	ConfigurePreDeleteSetDeviceLocations
	// Delete the LogVol for this VolumeSeries
	ConfigureDeleteLogVol
	// Reset the rootStorageID of the volume series, also clear systemTags set by this handler
	ConfigureResetRootStorageID
	// Return
	ConfigureUndoDone
	// NoOp
	ConfigureNoOp
)

func (ss configureSubState) String() string {
	switch ss {
	case ConfigureStart:
		return "ConfigureStart"
	case ConfigureSetNewDeviceLocations:
		return "ConfigureSetNewDeviceLocations"
	case ConfigureSetRootStorageID:
		return "ConfigureSetRootStorageID"
	case ConfigureRestartAndCreate:
		return "ConfigureRestartAndCreate"
	case ConfigureCreateLogVol:
		return "ConfigureCreateLogVol"
	case ConfigureRestartAndOpenCreated:
		return "ConfigureRestartAndOpenCreated"
	case ConfigureOpenCreatedLogVol:
		return "ConfigureOpenCreatedLogVol"
	case ConfigureAllocParcels:
		return "ConfigureAllocParcels"
	case ConfigureStartExisting:
		return "ConfigureStartExisting"
	case ConfigureSetExistingDeviceLocations:
		return "ConfigureSetExistingDeviceLocations"
	case ConfigureOpenLogVol:
		return "ConfigureOpenLogVol"
	case ConfigureSetConfigured:
		return "ConfigureSetConfigured"
	case ConfigureDone:
		return "ConfigureDone"
	case ConfigureSetProvisioned:
		return "ConfigureSetProvisioned"
	case ConfigureCloseLogVol:
		return "ConfigureCloseLogVol"
	case ConfigureSetDeleting:
		return "ConfigureSetDeleting"
	case ConfigureCloseDeleteLogVol:
		return "ConfigureCloseDeleteLogVol"
	case ConfigurePreDeleteReloadConfig:
		return "ConfigurePreDeleteReloadConfig"
	case ConfigurePreDeleteSetDeviceLocations:
		return "ConfigurePreDeleteSetDeviceLocations"
	case ConfigureDeleteLogVol:
		return "ConfigureDeleteLogVol"
	case ConfigureResetRootStorageID:
		return "ConfigureResetRootStorageID"
	case ConfigureUndoDone:
		return "ConfigureUndoDone"
	}
	return fmt.Sprintf("configureSubState(%d)", ss)
}

// well-known error messages returned by Storelandia
// TBD move these to a common place, or prefer to replace with a better nuvoapi Error interface
const (
	errDeviceLocationAlreadySet = "ERROR: File exists"
	errNodeLocationAlreadySet   = "ERROR: File exists"
	errVolumeAlreadyExists      = "ERROR: File exists"
	errVolumeAlreadyOpen        = "ERROR: File exists"
	errVolumeAlreadyClosed      = "ERROR: No such volume" // prefix
	errVolumeDoesNotExist       = "ERROR: No such file or directory"
)

type configureOperators interface {
	getInitialState(ctx context.Context) configureSubState
	queryConfigDB(ctx context.Context)
	setDeviceLocations(ctx context.Context)
	setRootStorageID(ctx context.Context)
	createLogVol(ctx context.Context)
	openLogVol(ctx context.Context)
	allocParcels(ctx context.Context)
	setVolumeState(ctx context.Context, state string)
	closeLogVol(ctx context.Context)
	deleteLogVol(ctx context.Context)
	resetRootStorageID(ctx context.Context)
}

type nodeLocation struct {
	nodeIPAddr string
}

type parcelData struct {
	parcelCount uint64
	nodeUUID    string
}

type configureOp struct {
	c                *Component
	rhs              *vra.RequestHandlerState
	ops              configureOperators
	storageParcels   map[string]*parcelData
	nodeLocations    map[string]*nodeLocation
	logVolCreatorTag string // tag set if this VSR should create the LogVol
	logVolCreatedTag string // tag set if LogVol creation is known to have succeeded
	alreadyOpen      bool   // communication between createLogVol and openLogVol
	inError          bool
}

// vra.AllocationHandlers methods

// Configure implements the VOLUME_CONFIG state of a VolumeSeriesRequest. It uses several VolumeSeries properties
// and also queries for Storage and Node objects to support remote parcels.
// It will set the VolumeSeries.RootStorageID when it creates a new volume.
// No changes are made to the VolumeSeries or Storelandia when Request.Planonly is true.
func (c *Component) Configure(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &configureOp{
		c:              c,
		rhs:            rhs,
		storageParcels: make(map[string]*parcelData, 0),
		nodeLocations:  make(map[string]*nodeLocation, 0),
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// UndoConfigure implements the UNDO_VOLUME_CONFIG state of a VolumeSeriesRequest.
func (c *Component) UndoConfigure(ctx context.Context, rhs *vra.RequestHandlerState) {
	op := &configureOp{
		c:              c,
		rhs:            rhs,
		storageParcels: make(map[string]*parcelData, 0),
		nodeLocations:  make(map[string]*nodeLocation, 0),
	}
	op.ops = op // self-reference
	op.run(ctx)
}

// run executes the state machine
func (op *configureOp) run(ctx context.Context) {
	jumpToState := ConfigureNoOp
out:
	for ss := op.ops.getInitialState(ctx); !(op.rhs.InError || op.rhs.RetryLater); ss++ {
		if jumpToState != ConfigureNoOp {
			ss = jumpToState
			jumpToState = ConfigureNoOp
		}
		op.c.Log.Debugf("VolumeSeriesRequest %s: VOLUME_CONFIG %s", op.rhs.Request.Meta.ID, ss)
		switch ss {
		// VOLUME_CONFIG
		case ConfigureStart:
			op.ops.queryConfigDB(ctx)
		case ConfigureSetNewDeviceLocations:
			op.ops.setDeviceLocations(ctx)
		case ConfigureSetRootStorageID:
			op.ops.setRootStorageID(ctx)
			jumpToState = ConfigureCreateLogVol
		case ConfigureRestartAndCreate:
			op.ops.queryConfigDB(ctx)
		case ConfigureCreateLogVol:
			op.ops.createLogVol(ctx)
			jumpToState = ConfigureOpenCreatedLogVol
		case ConfigureRestartAndOpenCreated:
			op.ops.queryConfigDB(ctx)
		case ConfigureOpenCreatedLogVol:
			op.ops.openLogVol(ctx)
		case ConfigureAllocParcels:
			op.ops.allocParcels(ctx)
			jumpToState = ConfigureSetConfigured
		case ConfigureStartExisting:
			op.ops.queryConfigDB(ctx)
		case ConfigureSetExistingDeviceLocations:
			op.ops.setDeviceLocations(ctx)
		case ConfigureOpenLogVol:
			op.ops.openLogVol(ctx)
		case ConfigureSetConfigured:
			op.ops.setVolumeState(ctx, com.VolStateConfigured)
		// UNDO_VOLUME_CONFIG (undo path if !rhs.HasDelete)
		case ConfigureSetProvisioned:
			op.ops.setVolumeState(ctx, com.VolStateProvisioned)
		case ConfigureCloseLogVol:
			op.ops.closeLogVol(ctx)
			jumpToState = ConfigureUndoDone
		case ConfigureSetDeleting:
			op.ops.setVolumeState(ctx, com.VolStateDeleting)
		case ConfigureCloseDeleteLogVol:
			op.ops.closeLogVol(ctx)
		case ConfigurePreDeleteReloadConfig:
			op.ops.queryConfigDB(ctx)
		case ConfigurePreDeleteSetDeviceLocations:
			op.ops.setDeviceLocations(ctx)
			if op.rhs.InError {
				op.rhs.AbortUndo = true // ignored by the animator if not in undo
			}
		case ConfigureDeleteLogVol:
			op.ops.deleteLogVol(ctx)
		case ConfigureResetRootStorageID:
			op.ops.resetRootStorageID(ctx)
		default:
			break out
		}
	}
	op.rhs.InError = op.rhs.InError || op.inError
}

func (op *configureOp) getInitialState(ctx context.Context) configureSubState {
	if swag.BoolValue(op.rhs.Request.PlanOnly) {
		// TBD walk the steps, adding messages to the Request about what Configure would do
		return ConfigureDone
	}
	op.logVolCreatorTag = fmt.Sprintf("%s:%s", com.SystemTagVsrSetStorageID, op.rhs.Request.Meta.ID)
	op.logVolCreatedTag = fmt.Sprintf("%s:%s", com.SystemTagVsrCreatedLogVol, op.rhs.Request.Meta.ID)
	restoredTag := fmt.Sprintf("%s:%s", com.SystemTagVsrRestored, op.rhs.Request.Meta.ID) // volume committed if snapshot restored in this request
	vs := op.rhs.VolumeSeries
	if op.rhs.Request.VolumeSeriesRequestState == com.VolReqStateUndoVolumeConfig {
		op.inError = op.rhs.InError
		op.rhs.InError = false // allow state machine to run
		if !util.Contains(vs.SystemTags, restoredTag) {
			if util.Contains(vs.SystemTags, op.logVolCreatorTag) {
				return ConfigureCloseDeleteLogVol
			}
			if op.rhs.HasUnbind || op.rhs.HasDelete {
				if op.rhs.HasDelete && op.rhs.VolumeSeries.VolumeSeriesState != com.VolStateDeleting {
					return ConfigureSetDeleting
				}
				if vs.RootStorageID == "" {
					return ConfigureUndoDone
				}
				return ConfigureCloseDeleteLogVol
			}
		}
		if vs.RootStorageID == "" || vs.VolumeSeriesState == com.VolStateInUse {
			return ConfigureUndoDone
		}
		return ConfigureSetProvisioned
	}
	if vs.RootStorageID != "" {
		if util.Contains(vs.SystemTags, op.logVolCreatedTag) {
			return ConfigureRestartAndOpenCreated
		}
		if util.Contains(vs.SystemTags, op.logVolCreatorTag) {
			return ConfigureRestartAndCreate
		}
		if vra.VolumeSeriesIsConfigured(vs.VolumeSeriesState) {
			return ConfigureDone
		}
		return ConfigureStartExisting
	}
	return ConfigureStart
}

func (op *configureOp) queryConfigDB(ctx context.Context) {
	if err := op.c.rei.ErrOnBool("config-query-fail"); err != nil {
		op.rhs.SetAndUpdateRequestMessage(ctx, "Config query error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	for id, p := range op.rhs.VolumeSeries.StorageParcels {
		s, err := op.c.oCrud.StorageFetch(ctx, id)
		if err != nil {
			// assume transient error, update unlikely to be saved
			op.rhs.SetAndUpdateRequestMessage(ctx, "Storage %s load failure: %s", id, err.Error())
			op.rhs.RetryLater = true
			return
		}
		nodeUUID := string(s.StorageState.AttachedNodeID)
		// no rounding: assume planning steps set SizeBytes to an exact, positive multiple of ParcelSizeBytes
		op.storageParcels[id] = &parcelData{
			parcelCount: uint64(swag.Int64Value(p.SizeBytes) / swag.Int64Value(s.ParcelSizeBytes)),
			nodeUUID:    nodeUUID,
		}
		if _, found := op.nodeLocations[nodeUUID]; !found && nodeUUID != string(op.c.thisNodeID) {
			n, err := op.c.oCrud.NodeFetch(ctx, nodeUUID)
			if err != nil {
				// assume transient error, update unlikely to be saved
				op.rhs.SetAndUpdateRequestMessage(ctx, "Node %s load failure: %s", nodeUUID, err.Error())
				op.rhs.RetryLater = true
				return
			}
			op.nodeLocations[nodeUUID] = &nodeLocation{n.Service.ServiceIP}
		}
	}
}

func (op *configureOp) setDeviceLocations(ctx context.Context) {
	for uuid, n := range op.nodeLocations {
		op.c.Log.Debugf("VolumeSeriesRequest %s: NUVOAPI NodeLocation(%s, %s, %d)", op.rhs.Request.Meta.ID, uuid, n.nodeIPAddr, uint16(op.c.App.NuvoPort))
		err := op.c.App.NuvoAPI.NodeLocation(uuid, n.nodeIPAddr, uint16(op.c.App.NuvoPort))
		if err != nil {
			if err.Error() != errNodeLocationAlreadySet {
				op.rhs.SetRequestError("NUVOAPI NodeLocation [node=%s ip=%s port=%d] failed: %s", uuid, n.nodeIPAddr, op.c.App.NuvoPort, err.Error())
				return
			}
			op.c.Log.Debugf("VolumeSeriesRequest %s: NUVOAPI NodeLocation(%s, %s, %d) failed: %s", op.rhs.Request.Meta.ID, uuid, n.nodeIPAddr, uint16(op.c.App.NuvoPort), err.Error())
		}
	}

	for s, p := range op.storageParcels {
		op.c.Log.Debugf("VolumeSeriesRequest %s: NUVOAPI DeviceLocation(%s, %s)", op.rhs.Request.Meta.ID, s, p.nodeUUID)
		err := op.c.App.NuvoAPI.DeviceLocation(s, p.nodeUUID)
		if err != nil {
			if err.Error() != errDeviceLocationAlreadySet {
				op.rhs.SetRequestError("NUVOAPI DeviceLocation [storage=%s node=%s] failed: %s", s, p.nodeUUID, err.Error())
				return
			}
			op.c.Log.Debugf("VolumeSeriesRequest %s: NUVOAPI DeviceLocation(%s, %s) failed: %s", op.rhs.Request.Meta.ID, s, p.nodeUUID, err.Error())
		}
	}
}

func (op *configureOp) setRootStorageID(ctx context.Context) {
	// Use the storageID that is lexicographically highest for now. To support restart, the choice must be reproducible. Better algorithm is TBD
	rhs := op.rhs
	var rootStorageID string
	for k := range op.storageParcels {
		if k > rootStorageID {
			rootStorageID = k
		}
	}
	errVSUpdateRetryAborted := fmt.Errorf("rootStorageId is already set")
	modVS := func(vs *models.VolumeSeries) (*models.VolumeSeries, error) {
		if vs == nil {
			vs = rhs.VolumeSeries
		}
		if vs.RootStorageID != "" {
			return nil, errVSUpdateRetryAborted
		}
		vs.NuvoVolumeIdentifier = uuid.NewV4().String()
		vs.Messages = []*models.TimestampedString{
			&models.TimestampedString{
				Message: fmt.Sprintf("Set rootStorageId [%s] and nuvoVolumeIdentifier [%s]", rootStorageID, vs.NuvoVolumeIdentifier),
				Time:    strfmt.DateTime(time.Now()),
			},
		}
		vs.RootStorageID = models.ObjIDMutable(rootStorageID)
		vs.SystemTags = []string{op.logVolCreatorTag} // tag that this VSR is creating the LogVol
		return vs, nil
	}
	vsID := string(rhs.VolumeSeries.Meta.ID)
	items := &crud.Updates{Set: []string{"rootStorageId", "nuvoVolumeIdentifier"}, Append: []string{"messages", "systemTags"}}
	vs, err := rhs.A.OCrud.VolumeSeriesUpdater(ctx, vsID, modVS, items)
	if err != nil {
		if err == errVSUpdateRetryAborted {
			rhs.SetRequestError("%s", err.Error())
		} else {
			// assume transient error, update unlikely to be saved
			rhs.SetAndUpdateRequestMessage(ctx, "Failed to update VolumeSeries object [%s]: %s", vsID, err.Error())
			rhs.RetryLater = true
		}
		return
	}
	rhs.VolumeSeries = vs
	rhs.SetRequestMessage("Set rootStorageId [%s] and nuvoVolumeIdentifier [%s]", rootStorageID, vs.NuvoVolumeIdentifier)
}

func (op *configureOp) createLogVol(ctx context.Context) {
	rhs := op.rhs
	vsID := string(rhs.VolumeSeries.Meta.ID)
	rootID := string(rhs.VolumeSeries.RootStorageID)
	nuvoVolumeIdentifier := rhs.VolumeSeries.NuvoVolumeIdentifier
	rootParcelUUID := rhs.VolumeSeries.RootParcelUUID
	sizeBytes := uint64(swag.Int64Value(rhs.VolumeSeries.SizeBytes))
	op.c.Log.Debugf("VolumeSeriesRequest %s: VolumeSeries [%s] NUVOAPI CreateLogVol(%s, %s, %s, %d)", op.rhs.Request.Meta.ID, vsID, nuvoVolumeIdentifier, rootID, rootParcelUUID, sizeBytes)
	newParcelUUID, err := op.c.App.NuvoAPI.CreateLogVol(nuvoVolumeIdentifier, rootID, rootParcelUUID, sizeBytes)
	if err != nil {
		if err.Error() == errVolumeAlreadyExists {
			op.c.Log.Debugf("NUVOAPI CreateLogVol error: %s", err.Error())
			// let openLogVol try
		} else {
			rhs.SetRequestError("NUVOAPI CreateLogVol failed: %s", err.Error())
		}
		return
	}
	if newParcelUUID != rootParcelUUID {
		rhs.SetRequestError("NUVOAPI CreateLogVol returned a different rootParcelUUID: [%s] != new[%s]", rootParcelUUID, newParcelUUID)
		return
	}
	modVS := func(vs *models.VolumeSeries) (*models.VolumeSeries, error) {
		if vs == nil {
			vs = rhs.VolumeSeries
		}
		vs.Messages = []*models.TimestampedString{
			&models.TimestampedString{
				Message: "Successfully created the LogVol",
				Time:    strfmt.DateTime(time.Now()),
			},
		}
		vs.SystemTags = []string{op.logVolCreatedTag} // tag that this VSR has created the LogVol
		return vs, nil
	}
	items := &crud.Updates{Append: []string{"messages", "systemTags"}}
	vs, err := rhs.A.OCrud.VolumeSeriesUpdater(ctx, vsID, modVS, items)
	if err != nil {
		// assume transient error, update unlikely to be saved
		rhs.SetAndUpdateRequestMessage(ctx, "Failed to update VolumeSeries object [%s]: %s", vsID, err.Error())
		rhs.RetryLater = true
		return
	}
	rhs.VolumeSeries = vs
	rhs.SetRequestMessage("Created LogVol [%s storageID=%s parcelUUID=%s]", nuvoVolumeIdentifier, rootID, rootParcelUUID)
	op.alreadyOpen = true
}

func (op *configureOp) openLogVol(ctx context.Context) {
	if op.alreadyOpen {
		return
	}
	rootID := string(op.rhs.VolumeSeries.RootStorageID)
	nuvoVolumeIdentifier := op.rhs.VolumeSeries.NuvoVolumeIdentifier
	rootParcelUUID := op.rhs.VolumeSeries.RootParcelUUID
	op.c.Log.Debugf("VolumeSeriesRequest %s: VolumeSeries [%s] NUVOAPI OpenVol(%s, %s, %s, true)", op.rhs.Request.Meta.ID, op.rhs.VolumeSeries.Meta.ID, nuvoVolumeIdentifier, rootID, rootParcelUUID)
	err := op.c.App.NuvoAPI.OpenVol(nuvoVolumeIdentifier, rootID, rootParcelUUID, true)
	if err != nil && err.Error() != errVolumeAlreadyOpen {
		op.rhs.SetRequestError("NUVOAPI OpenVol error: %s", err.Error())
		return
	}
	op.rhs.SetRequestMessage("Opened LogVol [%s storageID=%s parcelUUID=%s]", nuvoVolumeIdentifier, rootID, rootParcelUUID)
}

func (op *configureOp) allocParcels(ctx context.Context) {
	vs := op.rhs.VolumeSeries
	nuvoVolumeIdentifier := op.rhs.VolumeSeries.NuvoVolumeIdentifier
	for s, p := range op.storageParcels {
		parcels := p.parcelCount
		if s == string(vs.RootStorageID) {
			if parcels == 0 {
				continue
			}
		}
		op.c.Log.Debugf("VolumeSeriesRequest %s: VolumeSeries [%s] NUVOAPI AllocParcels(%s, %s, %d)", op.rhs.Request.Meta.ID, vs.Meta.ID, nuvoVolumeIdentifier, s, parcels)
		err := op.c.App.NuvoAPI.AllocParcels(nuvoVolumeIdentifier, s, parcels)
		if err != nil {
			op.rhs.SetRequestError("NUVOAPI AllocParcels [%s storage=%s count=%d] failed: %s", nuvoVolumeIdentifier, s, p.parcelCount, err.Error())
			return
		}
	}
	op.rhs.SetRequestMessage("Allocated storage parcels")
}

func (op *configureOp) setVolumeState(ctx context.Context, newState string) {
	rhs := op.rhs
	setFields := []string{"volumeSeriesState", "configuredNodeId"}
	var nodeID models.ObjIDMutable
	if newState == com.VolStateConfigured {
		nodeID = rhs.Request.NodeID
		setFields = append(setFields, "systemTags")
	}
	errBreakout := fmt.Errorf("set-configure-breakout")
	modVS := func(vs *models.VolumeSeries) (*models.VolumeSeries, error) {
		if vs == nil {
			vs = rhs.VolumeSeries
		}
		if vs.VolumeSeriesState == newState && vs.ConfiguredNodeID == nodeID {
			return nil, errBreakout
		}
		if vs.VolumeSeriesState != newState {
			vs.Messages = []*models.TimestampedString{
				&models.TimestampedString{
					Message: fmt.Sprintf("State change %s ⇒ %s", vs.VolumeSeriesState, newState),
					Time:    strfmt.DateTime(time.Now()),
				},
			}
			if newState == com.VolStateConfigured {
				sTag := util.NewTagList(vs.SystemTags)
				sTag.Set(com.SystemTagVolumeLastConfiguredNode, string(nodeID))
				vs.SystemTags = sTag.List()
			}
			vs.VolumeSeriesState = newState
		}
		vs.ConfiguredNodeID = nodeID
		return vs, nil
	}
	vsID := string(rhs.VolumeSeries.Meta.ID)
	items := &crud.Updates{Set: setFields, Append: []string{"messages"}}
	vs, err := rhs.A.OCrud.VolumeSeriesUpdater(ctx, vsID, modVS, items)
	if err != nil && err != errBreakout {
		// assume transient error, update unlikely to be saved
		rhs.SetAndUpdateRequestMessage(ctx, "Failed to update VolumeSeries object [%s]: %s", vsID, err.Error())
		rhs.RetryLater = true
		return
	}
	if err == nil {
		rhs.VolumeSeries = vs
		rhs.SetRequestMessage("Set volume state to %s", newState)
	}
}

func (op *configureOp) closeLogVol(ctx context.Context) {
	var err error
	if err = op.c.rei.ErrOnBool("config-close-vol-nfe"); err != nil {
		op.rhs.SetRequestMessage("Config close-vol non-fatal error: %s", err.Error())
		err = nuvoapi.WrapError(fmt.Errorf(errVolumeAlreadyClosed))
	} else if err = op.c.rei.ErrOnBool("config-close-vol-error"); err != nil {
		op.rhs.SetRequestMessage("Config close-vol-error: %s", err.Error())
	} else if err = op.c.rei.ErrOnBool("block-close-log-vol"); err != nil {
		op.rhs.SetAndUpdateRequestMessageDistinct(ctx, "block-close-log-vol error: %s", err.Error())
		op.rhs.RetryLater = true
		return
	}
	nuvoVolumeIdentifier := op.rhs.VolumeSeries.NuvoVolumeIdentifier
	op.c.Log.Debugf("VolumeSeriesRequest %s: VolumeSeries [%s] NUVOAPI CloseVol(%s)", op.rhs.Request.Meta.ID, op.rhs.VolumeSeries.Meta.ID, nuvoVolumeIdentifier)
	if err == nil {
		err = op.c.App.NuvoAPI.CloseVol(nuvoVolumeIdentifier)
	}
	if err != nil {
		op.c.Log.Errorf("VolumeSeriesRequest %s: NUVOAPI CloseVol(%s) failed: %s", op.rhs.Request.Meta.ID, nuvoVolumeIdentifier, err.Error())
		if e, ok := err.(nuvoapi.ErrorInt); ok && e.NotInitialized() {
			op.c.Log.Errorf("VolumeSeriesRequest %s: NUVOAPI NOT INITIALIZED", op.rhs.Request.Meta.ID)
			op.rhs.RetryLater = true
			return
		} else if nuvoapi.ErrorIsTemporary(err) {
			op.rhs.SetRequestMessage("NUVOAPI temporary error: %s", err.Error())
			op.rhs.RetryLater = true
			return
		} else if !strings.HasPrefix(err.Error(), errVolumeAlreadyClosed) {
			op.rhs.AbortUndo = true // ignored by the animator if not in undo
			op.rhs.SetRequestError("NUVOAPI CloseVol failed: %s", err.Error())
			return
		}
	}
	op.rhs.SetRequestMessage("LogVol is closed")
}

func (op *configureOp) deleteLogVol(ctx context.Context) {
	var err error
	if err = op.c.rei.ErrOnBool("config-destroy-vol-nfe"); err != nil {
		op.rhs.SetRequestMessage("Config destroy-vol non-fatal error: %s", err.Error())
		err = nuvoapi.WrapError(fmt.Errorf(errVolumeDoesNotExist))
	} else if err = op.c.rei.ErrOnBool("config-destroy-vol-error"); err != nil {
		op.rhs.SetRequestMessage("Config destroy-vol-error: %s", err.Error())
	}
	nuvoVolumeIdentifier := op.rhs.VolumeSeries.NuvoVolumeIdentifier
	rootID := string(op.rhs.VolumeSeries.RootStorageID)
	rootParcelUUID := op.rhs.VolumeSeries.RootParcelUUID
	op.c.Log.Debugf("VolumeSeriesRequest %s: VolumeSeries [%s] NUVOAPI DestroyVol(%s, %s, %s, true)", op.rhs.Request.Meta.ID, op.rhs.VolumeSeries.Meta.ID, nuvoVolumeIdentifier, rootID, rootParcelUUID)
	if err == nil {
		err = op.c.App.NuvoAPI.DestroyVol(nuvoVolumeIdentifier, rootID, rootParcelUUID, true)
	}
	if err != nil {
		op.c.Log.Errorf("VolumeSeriesRequest %s: NUVOAPI DestroyVol(%s, %s, %s, true) failed: %s", op.rhs.Request.Meta.ID, nuvoVolumeIdentifier, rootID, rootParcelUUID, err.Error())
		if e, ok := err.(nuvoapi.ErrorInt); ok && e.NotInitialized() {
			op.c.Log.Errorf("VolumeSeriesRequest %s: NUVOAPI NOT INITIALIZED", op.rhs.Request.Meta.ID)
			op.rhs.RetryLater = true
			return
		} else if nuvoapi.ErrorIsTemporary(err) {
			op.rhs.SetRequestMessage("NUVOAPI temporary error: %s", err.Error())
			op.rhs.RetryLater = true
			return
		} else if err.Error() != errVolumeDoesNotExist {
			if op.rhs.VolumeSeries.VolumeSeriesState != com.VolStateDeleting {
				op.rhs.VSUpdater.SetVolumeState(ctx, com.VolStateProvisioned)
			}
			op.rhs.AbortUndo = true // ignored by the animator if not in undo
			op.rhs.SetRequestError("NUVOAPI DestroyVol failed: %s", err.Error())
			return
		}
	}
	op.rhs.SetRequestMessage("LogVol is deleted")
}

func (op *configureOp) resetRootStorageID(ctx context.Context) {
	rhs := op.rhs
	modVS := func(vs *models.VolumeSeries) (*models.VolumeSeries, error) {
		if vs == nil {
			vs = rhs.VolumeSeries
		}
		vs.Messages = []*models.TimestampedString{
			&models.TimestampedString{
				Message: "Cleared rootStorageId and nuvoVolumeIdentifier",
				Time:    strfmt.DateTime(time.Now()),
			},
		}
		vs.RootStorageID = ""
		vs.NuvoVolumeIdentifier = ""
		if vs.LifecycleManagementData != nil {
			vs.LifecycleManagementData.FinalSnapshotNeeded = false
		}
		vs.SystemTags = []string{op.logVolCreatorTag, op.logVolCreatedTag}
		return vs, nil
	}
	vsID := string(rhs.VolumeSeries.Meta.ID)
	items := &crud.Updates{Set: []string{"rootStorageId", "nuvoVolumeIdentifier"}, Append: []string{"messages"}, Remove: []string{"systemTags"}}
	if rhs.VolumeSeries.LifecycleManagementData != nil && rhs.VolumeSeries.LifecycleManagementData.FinalSnapshotNeeded {
		items.Set = append(items.Set, "lifecycleManagementData.finalSnapshotNeeded")
	}
	vs, err := rhs.A.OCrud.VolumeSeriesUpdater(ctx, vsID, modVS, items)
	if err != nil {
		// assume transient error, update unlikely to be saved
		rhs.SetAndUpdateRequestMessage(ctx, "Failed to update VolumeSeries object [%s]: %s", vsID, err.Error())
		rhs.RetryLater = true
		return
	}
	rhs.VolumeSeries = vs
	rhs.SetRequestMessage("Cleared rootStorageId and nuvoVolumeIdentifier")
}
