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


package mongods

import (
	"reflect"
	"time"

	M "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	uuid "github.com/satori/go.uuid"
)

// This file contains additional converters to morph model objects to datastore objects

// ToModel converts a datastore object to a model object
func (o *StringList) ToModel() *M.ObjTags {
	mObj := make(M.ObjTags, len(*o))
	for i, s := range *o {
		mObj[i] = s
	}
	return &mObj
}

// FromModel converts a model object to a datastore object
func (o *StringList) FromModel(mObj *M.ObjTags) {
	for i, s := range *mObj {
		(*o)[i] = s
	}
}

// ToModel converts a datastore object to a model object
func (o *StringValueMap) ToModel() map[string]M.ValueType {
	svm := make(map[string]M.ValueType, len(*o))
	for k, v := range *o {
		svm[k] = M.ValueType{Kind: v.Kind, Value: v.Value}
	}
	return svm
}

// FromModel converts a model object to a datastore object
func (o *StringValueMap) FromModel(mObj map[string]M.ValueType) {
	for k, v := range mObj {
		(*o)[k] = ValueType{Kind: v.Kind, Value: v.Value}
	}
}

// ToModel converts a datastore object to a model object
func (o *RestrictedStringValueMap) ToModel() map[string]M.RestrictedValueType {
	svm := make(map[string]M.RestrictedValueType, len(*o))
	for k, v := range *o {
		svm[k] = M.RestrictedValueType{
			ValueType:                 M.ValueType{Kind: v.Kind, Value: v.Value},
			RestrictedValueTypeAllOf1: M.RestrictedValueTypeAllOf1{Immutable: v.Immutable},
		}
	}
	return svm
}

// FromModel converts a model object to a datastore object
func (o *RestrictedStringValueMap) FromModel(mObj map[string]M.RestrictedValueType) {
	for k, v := range mObj {
		(*o)[k] = RestrictedValueType{Kind: v.Kind, Value: v.Value, Immutable: v.Immutable}
	}
}

// ToModel converts a datastore object to a model object
func (o *AuthRoleMap) ToModel() map[string]M.AuthRole {
	mObj := make(map[string]M.AuthRole, len(*o))
	for k, v := range *o {
		mObj[k] = M.AuthRole{
			Disabled: v.Disabled,
			RoleID:   M.ObjIDMutable(v.RoleID),
		}
	}
	return mObj
}

// FromModel converts a model object to a datastore object
func (o *AuthRoleMap) FromModel(mObj map[string]M.AuthRole) {
	for k, v := range mObj {
		(*o)[k] = AuthRole{
			Disabled: v.Disabled,
			RoleID:   string(v.RoleID),
		}
	}
}

// ToModel converts a datastore object to a model object
func (o *CapabilityMap) ToModel() map[string]bool {
	mObj := make(map[string]bool, len(*o))
	for k, v := range *o {
		mObj[k] = v
	}
	return mObj
}

// FromModel converts a model object to a datastore object
func (o *CapabilityMap) FromModel(mObj map[string]bool) {
	for k, v := range mObj {
		(*o)[k] = v
	}
}

// ToModel converts a datastore object to a model object
func (o *UserPasswordPolicy) ToModel() *M.SystemMutableUserPasswordPolicy {
	return &M.SystemMutableUserPasswordPolicy{
		MinLength: o.MinLength,
	}
}

// FromModel converts a model object to a datastore object
func (o *UserPasswordPolicy) FromModel(mObj *M.SystemMutableUserPasswordPolicy) {
	o.MinLength = mObj.MinLength
}

// ToModel converts a datastore object to a model object
func (o *ObjIDList) ToModel() *[]M.ObjIDMutable {
	mObj := make([]M.ObjIDMutable, len(*o))
	for i, s := range *o {
		mObj[i] = M.ObjIDMutable(s)
	}
	return &mObj
}

// FromModel converts a model object to a datastore object
func (o *ObjIDList) FromModel(mObj *[]M.ObjIDMutable) {
	for i, s := range *mObj {
		(*o)[i] = string(s)
	}
}

// ObjMeta is not directly represented in the database.

// ToModel converts a datastore object to a model object
// The objType is required because it is not stored in the datastore
func (o *ObjMeta) ToModel(objType string) *M.ObjMeta {
	return &M.ObjMeta{
		ID:           M.ObjID(o.MetaObjID),
		Version:      M.ObjVersion(o.MetaVersion),
		ObjType:      objType,
		TimeCreated:  strfmt.DateTime(o.MetaTimeCreated),
		TimeModified: strfmt.DateTime(o.MetaTimeModified),
	}
}

// FromModel converts a model object to a datastore object
// As a special case to handle incoming modification requests, if the TimeModified IsZero() then current time is set instead
func (o *ObjMeta) FromModel(mObj *M.ObjMeta) {
	if mObj.ID != "" {
		o.MetaObjID = string(mObj.ID)
		o.MetaVersion = int32(mObj.Version)
		o.MetaTimeCreated = time.Time(mObj.TimeCreated)
		o.MetaTimeModified = time.Time(mObj.TimeModified)
		if o.MetaTimeModified.IsZero() {
			o.MetaTimeModified = time.Now() // modified
		}
	} else {
		// creation
		o.MetaObjID = uuid.NewV4().String()
		o.MetaVersion = 1
		o.MetaTimeCreated = time.Now()
		o.MetaTimeModified = o.MetaTimeCreated // initial constraint: timeCreated == timeModified
	}
}

// CopyToObject uses reflection to set its properties in any object assuming the embedded field names are identical.
func (o *ObjMeta) CopyToObject(obj interface{}) {
	if reflect.TypeOf(obj).Kind() != reflect.Ptr {
		panic("Must pass obj by reference")
	}
	oT := reflect.TypeOf(*o)
	oV := reflect.ValueOf(*o)
	objV := reflect.Indirect(reflect.ValueOf(obj))
	for i := 0; i < oT.NumField(); i++ {
		oF := oT.Field(i)
		objV.FieldByName(oF.Name).Set(oV.FieldByName(oF.Name))
	}
}

// CopyFromObject uses reflection to copy its properties from any object assuming the embedded field names are identical.
func (o *ObjMeta) CopyFromObject(obj interface{}) {
	oT := reflect.TypeOf(*o)
	oV := reflect.Indirect(reflect.ValueOf(o)) // ValueOf(*o) is not directly addressable
	var objV reflect.Value
	if reflect.TypeOf(obj).Kind() == reflect.Ptr {
		objV = reflect.Indirect(reflect.ValueOf(obj))
	} else {
		objV = reflect.ValueOf(obj)
	}
	for i := 0; i < oT.NumField(); i++ {
		oF := oT.Field(i)
		oV.FieldByName(oF.Name).Set(objV.FieldByName(oF.Name))
	}
}

// ToModel converts a datastore object to a model object
func (o *TimestampedString) ToModel() *M.TimestampedString {
	return &M.TimestampedString{
		Message: o.Message,
		Time:    strfmt.DateTime(o.Time),
	}
}

// FromModel converts a model object to a datastore object
func (o *TimestampedString) FromModel(mObj *M.TimestampedString) {
	o.Message = mObj.Message
	o.Time = time.Time(mObj.Time)
}

// ToModel converts a datastore object to a model object
func (o *ServiceState) ToModel() *M.ServiceState {
	return &M.ServiceState{
		HeartbeatPeriodSecs: o.HeartbeatPeriodSecs,
		HeartbeatTime:       strfmt.DateTime(o.HeartbeatTime),
		State:               o.State,
	}
}

// FromModel converts a model object to a datastore object
func (o *ServiceState) FromModel(mObj *M.ServiceState) {
	o.HeartbeatPeriodSecs = mObj.HeartbeatPeriodSecs
	o.HeartbeatTime = time.Time(mObj.HeartbeatTime)
	o.State = mObj.State
	if o.State == "" {
		o.State = "UNKNOWN"
	}
}

// ToModel converts a datastore object to a model object
func (o *NuvoService) ToModel() *M.NuvoService {
	messages := make([]*M.TimestampedString, len(o.Messages))
	for i, msg := range o.Messages {
		messages[i] = msg.ToModel()
	}
	ns := &M.NuvoService{
		NuvoServiceAllOf0: M.NuvoServiceAllOf0{
			Messages:          messages,
			ServiceType:       o.ServiceType,
			ServiceVersion:    o.ServiceVersion,
			ServiceIP:         o.ServiceIP,
			ServiceLocator:    o.ServiceLocator,
			ServiceAttributes: o.ServiceAttributes.ToModel(),
		},
	}
	ns.ServiceState = *(&o.ServiceState).ToModel()
	return ns
}

// FromModel converts a model object to a datastore object
func (o *NuvoService) FromModel(mObj *M.NuvoService) {
	o.Messages = make([]TimestampedString, len(mObj.Messages))
	for i, msg := range mObj.Messages {
		o.Messages[i].FromModel(msg)
	}
	o.ServiceType = mObj.ServiceType
	o.ServiceVersion = mObj.ServiceVersion
	o.ServiceIP = mObj.ServiceIP
	o.ServiceLocator = mObj.ServiceLocator
	o.ServiceAttributes = make(StringValueMap, len(mObj.ServiceAttributes))
	o.ServiceAttributes.FromModel(mObj.ServiceAttributes)
	ss := &mObj.ServiceState
	o.ServiceState.FromModel(ss)
}

// ToModel converts a datastore object to a model object
func (o *Identity) ToModel() *M.Identity {
	return &M.Identity{
		AccountID:       M.ObjIDMutable(o.AccountID),
		TenantAccountID: M.ObjID(o.TenantAccountID),
		UserID:          M.ObjIDMutable(o.UserID),
	}
}

// FromModel converts a model object to a datastore object
func (o *Identity) FromModel(mObj *M.Identity) {
	o.AccountID = string(mObj.AccountID)
	o.TenantAccountID = string(mObj.TenantAccountID)
	o.UserID = string(mObj.UserID)
}

// ToModel converts a datastore object to a model object
func (o *IoPattern) ToModel() *M.IoPattern {
	return &M.IoPattern{
		Name:            string(o.Name),
		MinSizeBytesAvg: swag.Int32(o.MinSizeBytesAvg),
		MaxSizeBytesAvg: swag.Int32(o.MaxSizeBytesAvg),
	}
}

// FromModel converts a model object to a datastore object
func (o *IoPattern) FromModel(mObj *M.IoPattern) {
	o.Name = mObj.Name
	o.MinSizeBytesAvg = swag.Int32Value(mObj.MinSizeBytesAvg)
	o.MaxSizeBytesAvg = swag.Int32Value(mObj.MaxSizeBytesAvg)
}

// ToModel converts a datastore object to a model object
func (o *ReadWriteMix) ToModel() *M.ReadWriteMix {
	return &M.ReadWriteMix{
		Name:           string(o.Name),
		MinReadPercent: swag.Int32(o.MinReadPercent),
		MaxReadPercent: swag.Int32(o.MaxReadPercent),
	}
}

// FromModel converts a model object to a datastore object
func (o *ReadWriteMix) FromModel(mObj *M.ReadWriteMix) {
	o.Name = mObj.Name
	o.MinReadPercent = swag.Int32Value(mObj.MinReadPercent)
	o.MaxReadPercent = swag.Int32Value(mObj.MaxReadPercent)
}

// ToModel converts a datastore object to a model object
func (o *IoProfile) ToModel() *M.IoProfile {
	return &M.IoProfile{
		IoPattern:    o.IoPattern.ToModel(),
		ReadWriteMix: o.ReadWriteMix.ToModel(),
	}
}

// FromModel converts a model object to a datastore object
func (o *IoProfile) FromModel(mObj *M.IoProfile) {
	o.IoPattern.FromModel(mObj.IoPattern)
	o.ReadWriteMix.FromModel(mObj.ReadWriteMix)
}

// ToModel converts a datastore object to a model object
func (o *ProvisioningUnit) ToModel() *M.ProvisioningUnit {
	return &M.ProvisioningUnit{
		IOPS:       swag.Int64(o.IOPS),
		Throughput: swag.Int64(o.Throughput),
	}
}

// FromModel converts a model object to a datastore object
func (o *ProvisioningUnit) FromModel(mObj *M.ProvisioningUnit) {
	o.IOPS = swag.Int64Value(mObj.IOPS)
	o.Throughput = swag.Int64Value(mObj.Throughput)
}

// ToModel converts a datastore object to a model object
func (o *VolumeSeriesMinMaxSize) ToModel() *M.VolumeSeriesMinMaxSize {
	return &M.VolumeSeriesMinMaxSize{
		MinSizeBytes: swag.Int64(o.MinSizeBytes),
		MaxSizeBytes: swag.Int64(o.MaxSizeBytes),
	}
}

// FromModel converts a model object to a datastore object
func (o *VolumeSeriesMinMaxSize) FromModel(mObj *M.VolumeSeriesMinMaxSize) {
	o.MinSizeBytes = swag.Int64Value(mObj.MinSizeBytes)
	o.MaxSizeBytes = swag.Int64Value(mObj.MaxSizeBytes)
}

// ToModel converts a datastore object to a model object
func (o *StorageTypeReservationMap) ToModel() map[string]M.StorageTypeReservation {
	pam := make(map[string]M.StorageTypeReservation, len(*o))
	for k, v := range *o {
		pam[k] = M.StorageTypeReservation{NumMirrors: v.NumMirrors, SizeBytes: swag.Int64(v.SizeBytes)}
	}
	return pam
}

// FromModel converts a model object to a datastore object
func (o *StorageTypeReservationMap) FromModel(mObj map[string]M.StorageTypeReservation) {
	for k, v := range mObj {
		(*o)[k] = StorageTypeReservation{NumMirrors: v.NumMirrors, SizeBytes: swag.Int64Value(v.SizeBytes)}
	}
}

// ToModel converts a datastore object to a model object
func (o *CapacityReservationPlan) ToModel() *M.CapacityReservationPlan {
	crm := make(map[string]M.StorageTypeReservation, 0)
	if o.StorageTypeReservations != nil {
		crm = o.StorageTypeReservations.ToModel()
	}
	return &M.CapacityReservationPlan{
		StorageTypeReservations: crm,
	}
}

// FromModel converts a model object to a datastore object
func (o *CapacityReservationPlan) FromModel(mObj *M.CapacityReservationPlan) {
	if o.StorageTypeReservations == nil {
		o.StorageTypeReservations = make(StorageTypeReservationMap, len(mObj.StorageTypeReservations))
	}
	o.StorageTypeReservations.FromModel(mObj.StorageTypeReservations)
}

// ToModel converts a datastore object to a model object
func (o *PoolReservationMap) ToModel() map[string]M.PoolReservation {
	pam := make(map[string]M.PoolReservation, len(*o))
	for k, v := range *o {
		pam[k] = M.PoolReservation{SizeBytes: swag.Int64(v.SizeBytes), NumMirrors: v.NumMirrors}
	}
	return pam
}

// FromModel converts a model object to a datastore object
func (o *PoolReservationMap) FromModel(mObj map[string]M.PoolReservation) {
	for k, v := range mObj {
		(*o)[k] = PoolReservation{SizeBytes: swag.Int64Value(v.SizeBytes), NumMirrors: v.NumMirrors}
	}
}

// ToModel converts a datastore object to a model object
func (o *CapacityReservationResult) ToModel() *M.CapacityReservationResult {
	crm := make(map[string]M.PoolReservation, 0)
	if o.CurrentReservations != nil {
		crm = o.CurrentReservations.ToModel()
	}
	drm := make(map[string]M.PoolReservation, 0)
	if o.DesiredReservations != nil {
		drm = o.DesiredReservations.ToModel()
	}
	return &M.CapacityReservationResult{
		CurrentReservations: crm,
		DesiredReservations: drm,
	}
}

// FromModel converts a model object to a datastore object
func (o *CapacityReservationResult) FromModel(mObj *M.CapacityReservationResult) {
	if o.CurrentReservations == nil {
		o.CurrentReservations = make(PoolReservationMap, len(mObj.CurrentReservations))
	}
	o.CurrentReservations.FromModel(mObj.CurrentReservations)
	if o.DesiredReservations == nil {
		o.DesiredReservations = make(PoolReservationMap, len(mObj.DesiredReservations))
	}
	o.DesiredReservations.FromModel(mObj.DesiredReservations)
}

// ToModel converts a datastore object to a model object
func (o *NodeStorageDeviceMap) ToModel() map[string]M.NodeStorageDevice {
	sdm := make(map[string]M.NodeStorageDevice, len(*o))
	for k, v := range *o {
		sdm[k] = M.NodeStorageDevice{
			DeviceName:      v.DeviceName,
			DeviceState:     v.DeviceState,
			DeviceType:      v.DeviceType,
			SizeBytes:       swag.Int64(v.SizeBytes),
			UsableSizeBytes: swag.Int64(v.UsableSizeBytes),
		}
	}
	return sdm
}

// FromModel converts a model object to a datastore object
func (o *NodeStorageDeviceMap) FromModel(mObj map[string]M.NodeStorageDevice) {
	for k, v := range mObj {
		(*o)[k] = NodeStorageDevice{
			DeviceName:      v.DeviceName,
			DeviceState:     v.DeviceState,
			DeviceType:      v.DeviceType,
			SizeBytes:       swag.Int64Value(v.SizeBytes),
			UsableSizeBytes: swag.Int64Value(v.UsableSizeBytes),
		}
	}
}

// ToModel converts a datastore object to a model object
func (o *StorageAccessibility) ToModel() *M.StorageAccessibilityMutable {
	return &M.StorageAccessibilityMutable{
		AccessibilityScope:      M.StorageAccessibilityScopeType(o.AccessibilityScope),
		AccessibilityScopeObjID: M.ObjIDMutable(o.AccessibilityScopeObjID),
	}
}

// FromModel converts a model object to a datastore object
func (o *StorageAccessibility) FromModel(mObj *M.StorageAccessibilityMutable) {
	o.AccessibilityScope = string(mObj.AccessibilityScope)
	o.AccessibilityScopeObjID = string(mObj.AccessibilityScopeObjID)
}

// ToModel converts a datastore object to a model object
func (o *StorageParcelMap) ToModel() map[string]M.StorageParcelElement {
	spm := make(map[string]M.StorageParcelElement, len(*o))
	for k, v := range *o {
		spm[k] = M.StorageParcelElement{
			SizeBytes:              swag.Int64(v.SizeBytes),
			ShareableStorage:       v.ShareableStorage,
			ProvMinSizeBytes:       swag.Int64(v.ProvMinSizeBytes),
			ProvParcelSizeBytes:    swag.Int64(v.ProvParcelSizeBytes),
			ProvRemainingSizeBytes: swag.Int64(v.ProvRemainingSizeBytes),
			ProvNodeID:             v.ProvNodeID,
			ProvStorageRequestID:   v.ProvStorageRequestID,
		}
	}
	return spm
}

// FromModel converts a model object to a datastore object
func (o *StorageParcelMap) FromModel(mObj map[string]M.StorageParcelElement) {
	for k, v := range mObj {
		(*o)[k] = StorageParcelElement{
			SizeBytes:              swag.Int64Value(v.SizeBytes),
			ShareableStorage:       v.ShareableStorage,
			ProvMinSizeBytes:       swag.Int64Value(v.ProvMinSizeBytes),
			ProvParcelSizeBytes:    swag.Int64Value(v.ProvParcelSizeBytes),
			ProvRemainingSizeBytes: swag.Int64Value(v.ProvRemainingSizeBytes),
			ProvNodeID:             v.ProvNodeID,
			ProvStorageRequestID:   v.ProvStorageRequestID,
		}
	}
}

// ToModel converts a datastore object to a model object
func (o *StorageElement) ToModel() *M.StoragePlanStorageElement {
	return &M.StoragePlanStorageElement{
		Intent:         o.Intent,
		SizeBytes:      swag.Int64(o.SizeBytes),
		StorageParcels: o.StorageParcels.ToModel(),
		PoolID:         M.ObjIDMutable(o.PoolID),
	}
}

// FromModel converts a model object to a datastore object
func (o *StorageElement) FromModel(mObj *M.StoragePlanStorageElement) {
	o.Intent = mObj.Intent
	o.SizeBytes = swag.Int64Value(mObj.SizeBytes)
	if o.StorageParcels == nil {
		o.StorageParcels = make(StorageParcelMap, len(mObj.StorageParcels))
	}
	o.StorageParcels.FromModel(mObj.StorageParcels)
	o.PoolID = string(mObj.PoolID)
}

// ToModel converts a datastore object to a model object
func (o *StoragePlan) ToModel() *M.StoragePlan {
	hints := make(map[string]M.ValueType, 0)
	if o.PlacementHints != nil {
		hints = o.PlacementHints.ToModel()
	}
	elements := make([]*M.StoragePlanStorageElement, len(o.StorageElements))
	for i, elem := range o.StorageElements {
		elements[i] = elem.ToModel()
	}
	return &M.StoragePlan{
		StorageLayout:   M.StorageLayout(o.StorageLayout),
		LayoutAlgorithm: o.LayoutAlgorithm,
		PlacementHints:  hints,
		StorageElements: elements,
	}
}

// FromModel converts a model object to a datastore object
func (o *StoragePlan) FromModel(mObj *M.StoragePlan) {
	o.StorageLayout = string(mObj.StorageLayout)
	o.LayoutAlgorithm = mObj.LayoutAlgorithm
	if o.PlacementHints == nil {
		o.PlacementHints = make(StringValueMap, len(mObj.PlacementHints))
	}
	o.PlacementHints.FromModel(mObj.PlacementHints)
	o.StorageElements = make([]StorageElement, len(mObj.StorageElements))
	for i, elem := range mObj.StorageElements {
		o.StorageElements[i].FromModel(elem)
	}
}

// ToModel converts a datastore object to a model object
func (o *StorageState) ToModel() *M.StorageStateMutable {
	messages := make([]*M.TimestampedString, len(o.Messages))
	for i, msg := range o.Messages {
		messages[i] = msg.ToModel()
	}
	return &M.StorageStateMutable{
		AttachedNodeDevice: o.AttachedNodeDevice,
		AttachedNodeID:     M.ObjIDMutable(o.AttachedNodeID),
		AttachmentState:    o.AttachmentState,
		DeviceState:        o.DeviceState,
		MediaState:         o.MediaState,
		Messages:           messages,
		ProvisionedState:   o.ProvisionedState,
	}
}

// FromModel converts a model object to a datastore object
func (o *StorageState) FromModel(mObj *M.StorageStateMutable) {
	o.AttachedNodeDevice = mObj.AttachedNodeDevice
	o.AttachedNodeID = string(mObj.AttachedNodeID)
	o.AttachmentState = mObj.AttachmentState
	if o.AttachmentState == "" {
		o.AttachmentState = centrald.DefaultStorageAttachmentState
	}
	o.DeviceState = mObj.DeviceState
	if o.DeviceState == "" {
		o.DeviceState = centrald.DefaultStorageDeviceState
	}
	o.MediaState = mObj.MediaState
	if o.MediaState == "" {
		o.MediaState = centrald.DefaultStorageMediaState
	}
	o.Messages = make([]TimestampedString, len(mObj.Messages))
	for i, msg := range mObj.Messages {
		o.Messages[i].FromModel(msg)
	}
	o.ProvisionedState = mObj.ProvisionedState
	if o.ProvisionedState == "" {
		o.ProvisionedState = centrald.DefaultStorageProvisionedState
	}
}

// ToModel converts a datastore object to a model object
func (o *CacheAllocationMap) ToModel() map[string]M.CacheAllocation {
	cam := make(map[string]M.CacheAllocation, len(*o))
	for k, v := range *o {
		cam[k] = M.CacheAllocation{
			AllocatedSizeBytes: swag.Int64(v.AllocatedSizeBytes),
			RequestedSizeBytes: swag.Int64(v.RequestedSizeBytes),
		}
	}
	return cam
}

// FromModel converts a model object to a datastore object
func (o *CacheAllocationMap) FromModel(mObj map[string]M.CacheAllocation) {
	for k, v := range mObj {
		(*o)[k] = CacheAllocation{
			AllocatedSizeBytes: swag.Int64Value(v.AllocatedSizeBytes),
			RequestedSizeBytes: swag.Int64Value(v.RequestedSizeBytes),
		}
	}
}

// ToModel converts a datastore object to a model object
func (o *CapacityAllocationMap) ToModel() map[string]M.CapacityAllocation {
	cam := make(map[string]M.CapacityAllocation, len(*o))
	for k, v := range *o {
		cam[k] = M.CapacityAllocation{
			ReservedBytes: swag.Int64(v.ReservedBytes),
			ConsumedBytes: swag.Int64(v.ConsumedBytes),
		}
	}
	return cam
}

// FromModel converts a model object to a datastore object
func (o *CapacityAllocationMap) FromModel(mObj map[string]M.CapacityAllocation) {
	for k, v := range mObj {
		(*o)[k] = CapacityAllocation{
			ReservedBytes: swag.Int64Value(v.ReservedBytes),
			ConsumedBytes: swag.Int64Value(v.ConsumedBytes),
		}
	}
}

// ToModel converts a datastore object to a model object
func (o *ParcelAllocationMap) ToModel() map[string]M.ParcelAllocation {
	pam := make(map[string]M.ParcelAllocation, len(*o))
	for k, v := range *o {
		pam[k] = M.ParcelAllocation{SizeBytes: swag.Int64(v.SizeBytes)}
	}
	return pam
}

// FromModel converts a model object to a datastore object
func (o *ParcelAllocationMap) FromModel(mObj map[string]M.ParcelAllocation) {
	for k, v := range mObj {
		(*o)[k] = ParcelAllocation{SizeBytes: swag.Int64Value(v.SizeBytes)}
	}
}

// ToModel converts a datastore object to a model object
func (o *Mount) ToModel() *M.Mount {
	return &M.Mount{
		SnapIdentifier:    o.SnapIdentifier,
		PitIdentifier:     o.PitIdentifier,
		MountedNodeID:     M.ObjIDMutable(o.MountedNodeID),
		MountedNodeDevice: o.MountedNodeDevice,
		MountMode:         o.MountMode,
		MountState:        o.MountState,
		MountTime:         strfmt.DateTime(o.MountTime),
	}
}

// FromModel converts a model object to a datastore object
func (o *Mount) FromModel(mObj *M.Mount) {
	o.SnapIdentifier = mObj.SnapIdentifier
	o.PitIdentifier = mObj.PitIdentifier
	o.MountedNodeID = string(mObj.MountedNodeID)
	o.MountedNodeDevice = mObj.MountedNodeDevice
	o.MountMode = mObj.MountMode
	o.MountState = mObj.MountState
	o.MountTime = time.Time(mObj.MountTime)
}

// ToModel converts a datastore object to a model object
// ATYPICAL: A nil is returned if the timestamp is zero.
func (o *Progress) ToModel() *M.Progress {
	if o.Timestamp.IsZero() {
		return nil
	}
	pc := swag.Int32(o.PercentComplete)
	if o.PercentComplete == 0 {
		pc = nil
	}
	return &M.Progress{
		OffsetBytes:      o.OffsetBytes,
		PercentComplete:  pc,
		Timestamp:        strfmt.DateTime(o.Timestamp),
		TotalBytes:       o.TotalBytes,
		TransferredBytes: o.TransferredBytes,
	}
}

// FromModel converts a model object to a datastore object
// ATYPICAL: model may be nil.
func (o *Progress) FromModel(mObj *M.Progress) {
	if mObj == nil {
		mObj = &M.Progress{}
	}
	o.OffsetBytes = mObj.OffsetBytes
	o.PercentComplete = swag.Int32Value(mObj.PercentComplete)
	o.Timestamp = time.Time(mObj.Timestamp)
	o.TotalBytes = mObj.TotalBytes
	o.TransferredBytes = mObj.TransferredBytes
}

// ToModel converts a datastore object to a model object
func (o *SnapshotLocation) ToModel() *M.SnapshotLocation {
	return &M.SnapshotLocation{
		CreationTime: strfmt.DateTime(o.CreationTime),
		CspDomainID:  M.ObjIDMutable(o.CspDomainID),
	}
}

// FromModel converts a model object to a datastore object
func (o *SnapshotLocation) FromModel(mObj *M.SnapshotLocation) {
	o.CreationTime = time.Time(mObj.CreationTime)
	o.CspDomainID = string(mObj.CspDomainID)
}

// ToModel converts a datastore object to a model object
func (o *SnapshotLocationMap) ToModel() map[string]M.SnapshotLocation {
	mObj := make(map[string]M.SnapshotLocation, len(*o))
	for k, v := range *o {
		sl := *v.ToModel()
		sl.CspDomainID = M.ObjIDMutable(k)
		mObj[k] = sl
	}
	return mObj
}

// FromModel converts a model object to a datastore object
func (o *SnapshotLocationMap) FromModel(mObj map[string]M.SnapshotLocation) {
	for k, v := range mObj {
		sl := &SnapshotLocation{}
		sl.FromModel(&v)
		sl.CspDomainID = k
		(*o)[k] = *sl
	}
}

// ToModel converts a datastore object to a model object
func (o *SnapshotData) ToModel() M.SnapshotData {
	if o.Locations == nil {
		o.Locations = make([]SnapshotLocation, 0)
	}
	locations := make([]*M.SnapshotLocation, len(o.Locations))
	for i, l := range o.Locations {
		locations[i] = l.ToModel()
	}
	return M.SnapshotData{
		ConsistencyGroupID: M.ObjIDMutable(o.ConsistencyGroupID),
		DeleteAfterTime:    strfmt.DateTime(o.DeleteAfterTime),
		Locations:          locations,
		PitIdentifier:      o.PitIdentifier,
		ProtectionDomainID: M.ObjIDMutable(o.ProtectionDomainID),
		SizeBytes:          swag.Int64(o.SizeBytes),
		SnapIdentifier:     o.SnapIdentifier,
		SnapTime:           strfmt.DateTime(o.SnapTime),
		VolumeSeriesID:     M.ObjIDMutable(o.VolumeSeriesID),
	}
}

// FromModel converts a model object to a datastore object
func (o *SnapshotData) FromModel(mObj *M.SnapshotData) {
	o.ConsistencyGroupID = string(mObj.ConsistencyGroupID)
	o.DeleteAfterTime = time.Time(mObj.DeleteAfterTime)
	if mObj.Locations == nil {
		mObj.Locations = make([]*M.SnapshotLocation, 0)
	}
	o.Locations = make([]SnapshotLocation, len(mObj.Locations))
	for i, l := range mObj.Locations {
		o.Locations[i].FromModel(l)
	}
	o.PitIdentifier = mObj.PitIdentifier
	o.ProtectionDomainID = string(mObj.ProtectionDomainID)
	o.SizeBytes = swag.Int64Value(mObj.SizeBytes)
	o.SnapIdentifier = mObj.SnapIdentifier
	o.SnapTime = time.Time(mObj.SnapTime)
	o.VolumeSeriesID = string(mObj.VolumeSeriesID)
}

// ToModel converts a datastore object to a model object
func (o *SnapshotMap) ToModel() map[string]M.SnapshotData {
	snaps := make(map[string]M.SnapshotData, len(*o))
	for k, v := range *o {
		snaps[k] = v.ToModel()
	}
	return snaps
}

// FromModel converts a model object to a datastore object
func (o *SnapshotMap) FromModel(mObj map[string]M.SnapshotData) {
	for k, v := range mObj {
		s := SnapshotData{}
		s.FromModel(&v)
		(*o)[k] = s
	}
}

// ToModel converts a datastore object to a model object.
// ATYPICAL: The model value can be returned as nil.  We use the Inherited field
// as an existential indicator.
func (o *SnapshotCatalogPolicy) ToModel() *M.SnapshotCatalogPolicy {
	if o.Inherited {
		return nil
	}
	return &M.SnapshotCatalogPolicy{
		CspDomainID:        M.ObjIDMutable(o.CspDomainID),
		ProtectionDomainID: M.ObjIDMutable(o.ProtectionDomainID),
		Inherited:          o.Inherited,
	}
}

// FromModel converts a model object to a datastore object
// ATYPICAL: model may be nil.
func (o *SnapshotCatalogPolicy) FromModel(mObj *M.SnapshotCatalogPolicy) {
	if mObj == nil {
		mObj = &M.SnapshotCatalogPolicy{Inherited: true}
	}
	o.Inherited = mObj.Inherited
	if o.Inherited { // must ensure values are empty
		o.CspDomainID = ""
		o.ProtectionDomainID = ""
	} else {
		o.CspDomainID = string(mObj.CspDomainID)
		o.ProtectionDomainID = string(mObj.ProtectionDomainID)
	}
}

// ToModel converts a datastore object to a model object.
// ATYPICAL: The model value can be returned as nil.  We use the Inherited field
// as an existential indicator.
func (o *SnapshotManagementPolicy) ToModel() *M.SnapshotManagementPolicy {
	if o.Inherited {
		return nil
	}
	return &M.SnapshotManagementPolicy{
		DeleteLast:                  o.DeleteLast,
		DeleteVolumeWithLast:        o.DeleteVolumeWithLast,
		DisableSnapshotCreation:     o.DisableSnapshotCreation,
		NoDelete:                    o.NoDelete,
		RetentionDurationSeconds:    swag.Int32(o.RetentionDurationSeconds),
		VolumeDataRetentionOnDelete: o.VolumeDataRetentionOnDelete,
		Inherited:                   o.Inherited,
	}
}

// FromModel converts a model object to a datastore object
// ATYPICAL: model may be nil.
func (o *SnapshotManagementPolicy) FromModel(mObj *M.SnapshotManagementPolicy) {
	if mObj == nil {
		mObj = &M.SnapshotManagementPolicy{Inherited: true}
	}
	o.DeleteLast = mObj.DeleteLast
	o.DeleteVolumeWithLast = mObj.DeleteVolumeWithLast
	o.DisableSnapshotCreation = mObj.DisableSnapshotCreation
	o.NoDelete = mObj.NoDelete
	o.RetentionDurationSeconds = swag.Int32Value(mObj.RetentionDurationSeconds)
	o.VolumeDataRetentionOnDelete = mObj.VolumeDataRetentionOnDelete
	o.Inherited = mObj.Inherited
}

// ToModel converts a datastore object to a model object
func (o *LifecycleManagementData) ToModel() *M.LifecycleManagementData {
	return &M.LifecycleManagementData{
		EstimatedSizeBytes:        o.EstimatedSizeBytes,
		FinalSnapshotNeeded:       o.FinalSnapshotNeeded,
		GenUUID:                   o.GenUUID,
		WriteIOCount:              o.WriteIOCount,
		LastSnapTime:              strfmt.DateTime(o.LastSnapTime),
		LastUploadTime:            strfmt.DateTime(o.LastUploadTime),
		LastUploadSizeBytes:       o.LastUploadSizeBytes,
		LastUploadTransferRateBPS: o.LastUploadTransferRateBPS,
		NextSnapshotTime:          strfmt.DateTime(o.NextSnapshotTime),
		SizeEstimateRatio:         o.SizeEstimateRatio,
		LayoutAlgorithm:           o.LayoutAlgorithm,
	}
}

// FromModel converts a model object to a datastore object
// ATYPICAL: model may be nil.
func (o *LifecycleManagementData) FromModel(mObj *M.LifecycleManagementData) {
	if mObj == nil {
		mObj = &M.LifecycleManagementData{}
	}
	o.EstimatedSizeBytes = mObj.EstimatedSizeBytes
	o.FinalSnapshotNeeded = mObj.FinalSnapshotNeeded
	o.GenUUID = mObj.GenUUID
	o.WriteIOCount = mObj.WriteIOCount
	o.LastSnapTime = time.Time(mObj.LastSnapTime)
	o.LastUploadTime = time.Time(mObj.LastUploadTime)
	o.LastUploadSizeBytes = mObj.LastUploadSizeBytes
	o.LastUploadTransferRateBPS = mObj.LastUploadTransferRateBPS
	o.NextSnapshotTime = time.Time(mObj.NextSnapshotTime)
	o.SizeEstimateRatio = mObj.SizeEstimateRatio
	o.LayoutAlgorithm = mObj.LayoutAlgorithm
}

// ToModel converts a datastore object to a model object
func (o *SyncPeerMap) ToModel() map[string]M.SyncPeer {
	spm := make(map[string]M.SyncPeer, len(*o))
	for k, v := range *o {
		spm[k] = M.SyncPeer{
			ID:         M.ObjIDMutable(v.ID),
			State:      v.State,
			GenCounter: v.GenCounter,
			Annotation: v.Annotation,
		}
	}
	return spm
}

// FromModel converts a model object to a datastore object
func (o *SyncPeerMap) FromModel(mObj map[string]M.SyncPeer) {
	for k, v := range mObj {
		(*o)[k] = SyncPeer{
			ID:         string(v.ID),
			State:      v.State,
			GenCounter: v.GenCounter,
			Annotation: v.Annotation,
		}
	}
}

// ToModel converts a datastore object to a model object
func (o *VsrClaimMap) ToModel() map[string]M.VsrClaimElement {
	vcm := make(map[string]M.VsrClaimElement, len(*o))
	for k, v := range *o {
		vcm[k] = M.VsrClaimElement{
			SizeBytes:  swag.Int64(v.SizeBytes),
			Annotation: v.Annotation,
		}
	}
	return vcm
}

// FromModel converts a model object to a datastore object
func (o *VsrClaimMap) FromModel(mObj map[string]M.VsrClaimElement) {
	for k, v := range mObj {
		(*o)[k] = VsrClaimElement{
			SizeBytes:  swag.Int64Value(v.SizeBytes),
			Annotation: v.Annotation,
		}
	}
}

// ToModel converts a datastore object to a model object
func (o *VsrClaim) ToModel() *M.VsrClaim {
	return &M.VsrClaim{
		RemainingBytes: swag.Int64(o.RemainingBytes),
		Claims:         o.Claims.ToModel(),
	}
}

// FromModel converts a model object to a datastore object
func (o *VsrClaim) FromModel(mObj *M.VsrClaim) {
	if o.Claims == nil {
		o.Claims = make(map[string]VsrClaimElement, len(mObj.Claims))
	}
	o.Claims.FromModel(mObj.Claims)
	o.RemainingBytes = swag.Int64Value(mObj.RemainingBytes)
}

// ToModel converts a datastore object to a model object.
// ATYPICAL: The model value can be returned as nil.
func (o *VsrManagementPolicy) ToModel() *M.VsrManagementPolicy {
	if o.Inherited {
		return nil
	}
	return &M.VsrManagementPolicy{
		NoDelete:                 o.NoDelete,
		RetentionDurationSeconds: swag.Int32(o.RetentionDurationSeconds),
		Inherited:                o.Inherited,
	}
}

// FromModel converts a model object to a datastore object
// ATYPICAL: model may be nil.
func (o *VsrManagementPolicy) FromModel(mObj *M.VsrManagementPolicy) {
	if mObj == nil {
		mObj = &M.VsrManagementPolicy{Inherited: true}
	}
	o.NoDelete = mObj.NoDelete
	o.RetentionDurationSeconds = swag.Int32Value(mObj.RetentionDurationSeconds)
	o.Inherited = mObj.Inherited
}

// ToModel converts a datastore object to a model object.
// ATYPICAL: The model value can be returned as nil.
func (o *ClusterUsagePolicy) ToModel() *M.ClusterUsagePolicy {
	if o.Inherited {
		return nil
	}
	return &M.ClusterUsagePolicy{
		AccountSecretScope:          o.AccountSecretScope,
		ConsistencyGroupName:        o.ConsistencyGroupName,
		VolumeDataRetentionOnDelete: o.VolumeDataRetentionOnDelete,
		Inherited:                   o.Inherited,
	}
}

// FromModel converts a model object to a datastore object
// ATYPICAL: model may be nil.
func (o *ClusterUsagePolicy) FromModel(mObj *M.ClusterUsagePolicy) {
	if mObj == nil {
		mObj = &M.ClusterUsagePolicy{Inherited: true}
	}
	o.AccountSecretScope = mObj.AccountSecretScope
	o.ConsistencyGroupName = mObj.ConsistencyGroupName
	o.VolumeDataRetentionOnDelete = mObj.VolumeDataRetentionOnDelete
	o.Inherited = mObj.Inherited
}

// ToModel converts a datastore object to a model object
func (o *StorageCostMap) ToModel() map[string]M.StorageCost {
	spm := make(map[string]M.StorageCost, len(*o))
	for k, v := range *o {
		spm[k] = M.StorageCost{
			CostPerGiB: v.CostPerGiB,
		}
	}
	return spm
}

// FromModel converts a model object to a datastore object
func (o *StorageCostMap) FromModel(mObj map[string]M.StorageCost) {
	for k, v := range mObj {
		(*o)[k] = StorageCost{
			CostPerGiB: v.CostPerGiB,
		}
	}
}
