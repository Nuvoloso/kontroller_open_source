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
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/centrald"
	"github.com/alecthomas/units"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
)

func TestStringValueMap(t *testing.T) {
	assert := assert.New(t)

	// Test datastore conversion to and from model
	o1d := StringValueMap{
		"property1": ValueType{Kind: "STRING", Value: "string1"},
		"property2": ValueType{Kind: "INT", Value: "4"},
		"property3": ValueType{Kind: "String", Value: "5"},
		"property4": ValueType{Kind: "SECRET", Value: "something to be kept secret"},
	}

	// convert to model
	o1m := o1d.ToModel()
	assert.Len(o1d, 4)
	assert.Len(o1m, len(o1d))
	for p, vm := range o1m {
		assert.Contains(o1d, p)
		assert.Equal(o1d[p], ValueType(vm))
	}
	// convert back
	o2d := StringValueMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// retest with empty map
	o1d = StringValueMap{}
	assert.Len(o1d, 0)
	o1m = o1d.ToModel()
	assert.Len(o1m, 0)
	o2d = StringValueMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)
	assert.Len(o2d, 0)

	// Test model conversion to and from datastore
	o3m := map[string]models.ValueType{
		"property1": {Kind: "INT", Value: "2"},
		"property2": {Kind: "STRING", Value: "string2"},
		"property3": {Kind: "SECRET", Value: "something to be kept secret"},
	}
	// convert to datastore
	o3d := StringValueMap{}
	o3d.FromModel(o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o4m, o3m)
}

func TestRestrictedStringValueMap(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, RestrictedValueType{}, models.RestrictedValueType{}, fMap)

	// Test datastore conversion to and from model
	o1d := RestrictedStringValueMap{
		"property1": RestrictedValueType{Kind: "STRING", Value: "string1", Immutable: true},
		"property2": RestrictedValueType{Kind: "INT", Value: "4", Immutable: false},
		"property3": RestrictedValueType{Kind: "String", Value: "5"},
		"property4": RestrictedValueType{Kind: "SECRET", Value: "something to be kept secret"},
	}

	// convert to model
	o1m := o1d.ToModel()
	assert.Len(o1d, 4)
	assert.Len(o1m, len(o1d))
	for p, vm := range o1m {
		assert.Contains(o1d, p)
		assert.Equal(o1d[p].Kind, vm.Kind)
		assert.Equal(o1d[p].Value, vm.Value)
		assert.Equal(o1d[p].Immutable, vm.Immutable)
	}
	// convert back
	o2d := RestrictedStringValueMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// retest with empty map
	o1d = RestrictedStringValueMap{}
	assert.Len(o1d, 0)
	o1m = o1d.ToModel()
	assert.Len(o1m, 0)
	o2d = RestrictedStringValueMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)
	assert.Len(o2d, 0)

	// Test model conversion to and from datastore
	o3m := map[string]models.RestrictedValueType{
		"property1": {
			ValueType:                 models.ValueType{Kind: "INT", Value: "2"},
			RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: true},
		},
		"property2": {ValueType: models.ValueType{Kind: "STRING", Value: "string2"}},
		"property3": {ValueType: models.ValueType{Kind: "SECRET", Value: "something to be kept secret"}},
	}
	// convert to datastore
	o3d := RestrictedStringValueMap{}
	o3d.FromModel(o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o4m, o3m)
}

func TestAuthRoleMap(t *testing.T) {
	assert := assert.New(t)

	// Test datastore conversion to and from model
	o1d := AuthRoleMap{
		"user1": AuthRole{RoleID: "role1", Disabled: true},
		"user2": AuthRole{RoleID: "role2", Disabled: false},
		"user3": AuthRole{},
	}
	// convert to model
	o1m := o1d.ToModel()
	assert.Len(o1d, 3)
	assert.Len(o1m, len(o1d))
	for u, r := range o1m {
		assert.Contains(o1d, u)
		assert.Equal(o1d[u].Disabled, r.Disabled)
		assert.EqualValues(o1d[u].RoleID, r.RoleID)
	}
	// convert back
	o2d := AuthRoleMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// retest with empty map
	o1d = AuthRoleMap{}
	assert.Len(o1d, 0)
	o1m = o1d.ToModel()
	assert.Len(o1m, 0)
	o2d = AuthRoleMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)
	assert.Len(o2d, 0)

	// Test model conversion to and from datastore
	o3m := map[string]models.AuthRole{
		"User1": models.AuthRole{RoleID: "Role1", Disabled: true},
		"User2": models.AuthRole{RoleID: "Role2", Disabled: false},
		"User3": models.AuthRole{},
	}
	// convert to datastore
	o3d := AuthRoleMap{}
	o3d.FromModel(o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o4m, o3m)
}

func TestObjIdList(t *testing.T) {
	assert := assert.New(t)

	// Test datastore conversion to and from model
	o1d := ObjIDList{"objID1", "objID2", "objID3"}
	// convert to model
	o1m := o1d.ToModel()
	assert.Len(o1d, 3)
	assert.Len(*o1m, len(o1d))
	for u, r := range *o1m {
		assert.Equal(o1d[u], string(r))
	}
	// convert back
	o2d := make(ObjIDList, len(o1d))
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// retest with empty list
	o1d = ObjIDList{}
	assert.Len(o1d, 0)
	o1m = o1d.ToModel()
	assert.Len(*o1m, 0)
	o2d = ObjIDList{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)
	assert.Len(o2d, 0)

	// Test model conversion to and from datastore
	o3m := []models.ObjIDMutable{"objID1", "objID2", "objID3"}
	// convert to datastore
	o3d := make(ObjIDList, len(o3m))
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(*o4m, o3m)
}

func TestConvertObjMeta(t *testing.T) {
	assert := assert.New(t)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := ObjMeta{
		MetaObjID:        uuid.NewV4().String(),
		MetaVersion:      1,
		MetaTimeCreated:  now,
		MetaTimeModified: now,
	}
	// convert to model
	o1m := o1d.ToModel("ObjMeta")
	assert.Equal("ObjMeta", o1m.ObjType)
	// convert back
	o2d := ObjMeta{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d, "Datastore to model conversion is reversible")

	// Test model conversion to and from datastore
	o3m := models.ObjMeta{
		ID:          o1m.ID,
		Version:     o1m.Version,
		ObjType:     "ObjMeta",
		TimeCreated: o1m.TimeCreated,
	}
	assert.Equal(o3m.ObjType, "ObjMeta")
	assert.True(time.Time(o3m.TimeModified).IsZero(), "Unset TimeModified == IsZero()")
	// convert to datastore
	o3d := ObjMeta{}
	o3d.FromModel(&o3m)
	assert.True(time.Time(o3m.TimeModified).Before(o3d.MetaTimeModified), "TimeModified is now set")
	assert.True(time.Time(o3d.MetaTimeModified).After(now), "TimeModified set from current time")
	// convert back
	o4m := o3d.ToModel("ObjMeta")
	assert.Equal(o3m.ID, o4m.ID)
	assert.Equal(o3m.TimeCreated, o4m.TimeCreated)
	assert.True(time.Time(o3d.MetaTimeModified).Equal(time.Time(o4m.TimeModified)))

	// Test typical creation case where Meta not known
	o5m := models.ObjMeta{}
	o5d := ObjMeta{}
	o5d.FromModel(&o5m)
	assert.True(string(o5d.MetaObjID) != "")
	assert.Equal(int32(1), o5d.MetaVersion)
	o6m := o5d.ToModel("ObjMeta")
	assert.Equal(o6m.TimeCreated, o6m.TimeModified) // modified set to create time
	assert.Equal(models.ObjVersion(1), o6m.Version)

	// test assignment too and from arbitrary objects with embedded ObjMeta
	type tGood1 struct {
		ObjMeta
		s string
		i int
	}
	o1 := tGood1{}
	assert.Panics(func() { o1d.CopyToObject(o1) }) // pass by value
	assert.NotPanics(func() { o1d.CopyToObject(&o1) })
	assert.Equal(o1d.MetaObjID, o1.MetaObjID)
	assert.Equal(o1d.MetaVersion, o1.MetaVersion)
	assert.Equal(o1d.MetaTimeCreated, o1.MetaTimeCreated)
	assert.Equal(o1d.MetaTimeModified, o1.MetaTimeModified)
	o7d := ObjMeta{}
	assert.NotPanics(func() { o7d.CopyFromObject(o1) })
	assert.Equal(o1d, o7d)
	o7d = ObjMeta{}
	assert.NotPanics(func() { o7d.CopyFromObject(&o1) })
	assert.Equal(o1d, o7d)
}

func TestTimestampedString(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, TimestampedString{}, models.TimestampedString{}, fMap)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := TimestampedString{
		Message: "a message",
		Time:    now,
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := TimestampedString{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.TimestampedString{
		Message: "another message",
		Time:    o1m.Time,
	}
	// convert to datastore
	o3d := TimestampedString{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o3m, *o4m)
}

func TestServiceState(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, ServiceState{}, models.ServiceState{}, fMap)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := ServiceState{
		HeartbeatPeriodSecs: 90,
		HeartbeatTime:       now,
		State:               "UNKNOWN",
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := ServiceState{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.ServiceState{
		HeartbeatPeriodSecs: 300,
		HeartbeatTime:       o1m.HeartbeatTime,
		State:               "STARTING",
	}
	// convert to datastore
	o3d := ServiceState{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o3m, *o4m)
}

func TestNuvoService(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, NuvoService{}, models.NuvoService{}, fMap)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := NuvoService{
		ServiceType:    "Clusterd",
		ServiceVersion: "1.1.1",
		ServiceIP:      "100.96.5.13",
		ServiceLocator: "service-locator",
		ServiceState: ServiceState{
			HeartbeatPeriodSecs: 90,
			HeartbeatTime:       now,
			State:               "UNKNOWN",
		},
		ServiceAttributes: StringValueMap{
			"attr1": ValueType{Kind: "INT", Value: "1"},
			"attr2": ValueType{Kind: "STRING", Value: "a string"},
		},
		Messages: []TimestampedString{
			TimestampedString{Message: "message1", Time: now},
			TimestampedString{Message: "message2", Time: now},
			TimestampedString{Message: "message3", Time: now},
		},
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := NuvoService{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.NuvoService{
		NuvoServiceAllOf0: models.NuvoServiceAllOf0{
			ServiceType:       "Agentd",
			ServiceVersion:    "1.2.3",
			ServiceIP:         "1.2.3.4",
			ServiceLocator:    "service-pod",
			ServiceAttributes: map[string]models.ValueType{},
			Messages:          []*models.TimestampedString{},
		},
		ServiceState: models.ServiceState{
			HeartbeatPeriodSecs: 300,
			HeartbeatTime:       o1m.ServiceState.HeartbeatTime,
			State:               "STARTING",
		},
	}
	// convert to datastore
	o3d := NuvoService{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o3m, *o4m)

	// convert model with no ServiceState or ServiceAttributes set to datastore
	o5m := models.NuvoService{
		NuvoServiceAllOf0: models.NuvoServiceAllOf0{
			ServiceType:    "Agentd",
			ServiceVersion: "1.2.3",
			ServiceIP:      "8.7.6.5",
			ServiceLocator: "pod-name",
		},
	}
	o5d := NuvoService{}
	o5d.FromModel(&o5m)
	assert.Equal(o5d.State, "UNKNOWN")
	assert.Len(o5d.ServiceAttributes, 0)
}

func TestIdentity(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	cmpObjToModel(t, Identity{}, models.Identity{}, nil)

	// Test datastore conversion to and from model
	o1d := Identity{
		AccountID:       "aid1",
		TenantAccountID: "tid1",
		UserID:          "uid1",
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := Identity{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.Identity{
		AccountID:       "aid1",
		TenantAccountID: "tid1",
		UserID:          "uid1",
	}
	// convert to datastore
	o3d := Identity{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o3m, *o4m)
}

func TestIoProfile(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, IoProfile{}, models.IoProfile{}, fMap)

	// Test datastore conversion to and from model
	o1d := IoProfile{
		IoPattern:    IoPattern{Name: "random", MinSizeBytesAvg: 0, MaxSizeBytesAvg: 16384},
		ReadWriteMix: ReadWriteMix{Name: "read-write", MinReadPercent: 30, MaxReadPercent: 70},
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := IoProfile{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.IoProfile{
		IoPattern:    &models.IoPattern{Name: "sequential", MinSizeBytesAvg: swag.Int32(16384), MaxSizeBytesAvg: swag.Int32(262144)},
		ReadWriteMix: &models.ReadWriteMix{Name: "read-write", MinReadPercent: swag.Int32(30), MaxReadPercent: swag.Int32(70)},
	}
	// convert to datastore
	o3d := IoProfile{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o3m, *o4m)
}

func TestProvisioningUnit(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, ProvisioningUnit{}, models.ProvisioningUnit{}, fMap)

	// Test datastore conversion to and from model
	o1d := ProvisioningUnit{
		IOPS:       50,
		Throughput: 0,
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := ProvisioningUnit{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	o3m := models.ProvisioningUnit{
		IOPS:       swag.Int64(0),
		Throughput: swag.Int64(22 * int64(units.MB)),
	}
	// convert to datastore
	o3d := ProvisioningUnit{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o3m, *o4m)
}

func TestVolumeSeriesMinMaxSize(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, VolumeSeriesMinMaxSize{}, models.VolumeSeriesMinMaxSize{}, fMap)

	// Test datastore conversion to and from model
	o1d := VolumeSeriesMinMaxSize{
		MinSizeBytes: 10000000,
		MaxSizeBytes: 1000000000000,
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := VolumeSeriesMinMaxSize{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	o3m := models.VolumeSeriesMinMaxSize{
		MinSizeBytes: swag.Int64(0),
		MaxSizeBytes: swag.Int64(64 * int64(units.TiB)),
	}
	// convert to datastore
	o3d := VolumeSeriesMinMaxSize{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o3m, *o4m)
}

func TestStorageTypeReservationMap(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, StorageTypeReservation{}, models.StorageTypeReservation{}, fMap)

	// Test datastore conversion to and from model
	o1d := StorageTypeReservationMap{
		"storageType1": {NumMirrors: 1, SizeBytes: 1},
		"storageType2": {NumMirrors: 2, SizeBytes: 22},
		"storageType3": {NumMirrors: 3, SizeBytes: 333},
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := StorageTypeReservationMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// retest with empty map
	o1d = StorageTypeReservationMap{}
	assert.Len(o1d, 0)
	o1m = o1d.ToModel()
	assert.Len(o1m, 0)
	o2d = StorageTypeReservationMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)
	assert.Len(o2d, 0)

	// Test model conversion to and from datastore
	o3m := map[string]models.StorageTypeReservation{
		"storageType1": {NumMirrors: 1, SizeBytes: swag.Int64(1)},
		"storageType2": {NumMirrors: 2, SizeBytes: swag.Int64(22)},
		"storageType3": {NumMirrors: 3, SizeBytes: swag.Int64(333)},
	}
	// convert to datastore
	o3d := StorageTypeReservationMap{}
	o3d.FromModel(o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o4m, o3m)
}

func TestCapacityReservationPlan(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, CapacityReservationPlan{}, models.CapacityReservationPlan{}, fMap)

	// Test datastore conversion to and from model
	o1d := CapacityReservationPlan{
		StorageTypeReservations: StorageTypeReservationMap{
			"storageType1": {NumMirrors: 1, SizeBytes: 1},
			"storageType2": {NumMirrors: 2, SizeBytes: 22},
			"storageType3": {NumMirrors: 3, SizeBytes: 333},
		},
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := CapacityReservationPlan{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.CapacityReservationPlan{
		StorageTypeReservations: map[string]models.StorageTypeReservation{
			"storageType1": {NumMirrors: 1, SizeBytes: swag.Int64(1)},
			"storageType2": {NumMirrors: 2, SizeBytes: swag.Int64(22)},
			"storageType3": {NumMirrors: 3, SizeBytes: swag.Int64(333)},
		},
	}
	// convert to datastore
	o3d := CapacityReservationPlan{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o3m, *o4m)
}

func TestPoolReservationMap(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, PoolReservation{}, models.PoolReservation{}, fMap)

	// Test datastore conversion to and from model
	o1d := PoolReservationMap{
		"poolId1": {NumMirrors: 1, SizeBytes: 1},
		"poolId2": {NumMirrors: 2, SizeBytes: 22},
		"poolId3": {NumMirrors: 3, SizeBytes: 333},
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := PoolReservationMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// retest with empty map
	o1d = PoolReservationMap{}
	assert.Len(o1d, 0)
	o1m = o1d.ToModel()
	assert.Len(o1m, 0)
	o2d = PoolReservationMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)
	assert.Len(o2d, 0)

	// Test model conversion to and from datastore
	o3m := map[string]models.PoolReservation{
		"poolId1": {NumMirrors: 1, SizeBytes: swag.Int64(1)},
		"poolId2": {NumMirrors: 2, SizeBytes: swag.Int64(22)},
		"poolId3": {NumMirrors: 3, SizeBytes: swag.Int64(333)},
	}
	// convert to datastore
	o3d := PoolReservationMap{}
	o3d.FromModel(o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o4m, o3m)
}

func TestCapacityReservationResult(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, CapacityReservationResult{}, models.CapacityReservationResult{}, fMap)

	// Test datastore conversion to and from model
	o1d := CapacityReservationResult{
		CurrentReservations: PoolReservationMap{
			"poolId1": {NumMirrors: 1, SizeBytes: 1},
			"poolId2": {NumMirrors: 2, SizeBytes: 22},
			"poolId3": {NumMirrors: 3, SizeBytes: 333},
		},
		DesiredReservations: PoolReservationMap{
			"poolId1": {NumMirrors: 1, SizeBytes: 10},
			"poolId2": {NumMirrors: 2, SizeBytes: 220},
			"poolId3": {NumMirrors: 3, SizeBytes: 3330},
		},
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := CapacityReservationResult{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.CapacityReservationResult{
		CurrentReservations: map[string]models.PoolReservation{
			"poolId1": {NumMirrors: 1, SizeBytes: swag.Int64(1)},
			"poolId2": {NumMirrors: 2, SizeBytes: swag.Int64(22)},
			"poolId3": {NumMirrors: 3, SizeBytes: swag.Int64(333)},
		},
		DesiredReservations: map[string]models.PoolReservation{
			"poolId1": {NumMirrors: 1, SizeBytes: swag.Int64(10)},
			"poolId2": {NumMirrors: 2, SizeBytes: swag.Int64(220)},
			"poolId3": {NumMirrors: 3, SizeBytes: swag.Int64(3330)},
		},
	}
	// convert to datastore
	o3d := CapacityReservationResult{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o3m, *o4m)
}

func TestNodeStorageDeviceMap(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, NodeStorageDevice{}, models.NodeStorageDevice{}, fMap)

	// Test datastore conversion to and from model
	o1d := NodeStorageDeviceMap{
		"uuid1": NodeStorageDevice{DeviceName: "d1", DeviceState: "s1", DeviceType: "t1", SizeBytes: 11, UsableSizeBytes: 10},
		"uuid2": NodeStorageDevice{DeviceName: "d2", DeviceState: "s2", DeviceType: "t3", SizeBytes: 22, UsableSizeBytes: 20},
		"uuid3": NodeStorageDevice{DeviceName: "d3", DeviceState: "s2", DeviceType: "t3", SizeBytes: 333, UsableSizeBytes: 300},
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := NodeStorageDeviceMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// retest with empty map
	o1d = NodeStorageDeviceMap{}
	assert.Len(o1d, 0)
	o1m = o1d.ToModel()
	assert.Len(o1m, 0)
	o2d = NodeStorageDeviceMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)
	assert.Len(o2d, 0)

	// Test model conversion to and from datastore
	o3m := map[string]models.NodeStorageDevice{
		"uuid1": models.NodeStorageDevice{DeviceName: "d1", DeviceState: "s1", DeviceType: "t1", SizeBytes: swag.Int64(11), UsableSizeBytes: swag.Int64(10)},
		"uuid2": models.NodeStorageDevice{DeviceName: "d2", DeviceState: "s2", DeviceType: "t2", SizeBytes: swag.Int64(22), UsableSizeBytes: swag.Int64(20)},
		"uuid3": models.NodeStorageDevice{DeviceName: "d3", DeviceState: "s3", DeviceType: "t3", SizeBytes: swag.Int64(333), UsableSizeBytes: swag.Int64(300)},
	}
	// convert to datastore
	o3d := NodeStorageDeviceMap{}
	o3d.FromModel(o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o4m, o3m)
}

func TestStorageAccessibility(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, StorageAccessibility{}, models.StorageAccessibility{}, fMap)

	// Test datastore conversion to and from model
	o1d := StorageAccessibility{
		AccessibilityScope:      "NODE",
		AccessibilityScopeObjID: "c47d4b94-8298-4b42-b421-240917cfad75",
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := StorageAccessibility{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.StorageAccessibilityMutable{
		AccessibilityScope:      "CSPDOMAIN",
		AccessibilityScopeObjID: "080903d3-20d9-44f0-a949-28be5f8f3a1d",
	}
	// convert to datastore
	o3d := StorageAccessibility{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o3m, *o4m)
}

func TestStorageElement(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, StorageElement{}, models.StoragePlanStorageElement{}, fMap)
	cmpObjToModel(t, StorageParcelElement{}, models.StorageParcelElement{}, fMap)

	// Test datastore conversion to and from model
	o1d := StorageElement{
		Intent:    "DATA",
		SizeBytes: 10101010,
		StorageParcels: StorageParcelMap{
			"storageId1": StorageParcelElement{SizeBytes: 1, ShareableStorage: true, ProvMinSizeBytes: 2, ProvParcelSizeBytes: 3, ProvRemainingSizeBytes: 4, ProvNodeID: "NODE-1", ProvStorageRequestID: "SR-1"},
			"storageId2": StorageParcelElement{SizeBytes: 22, ProvMinSizeBytes: 23, ProvParcelSizeBytes: 24, ProvRemainingSizeBytes: 0, ProvNodeID: "NODE-2", ProvStorageRequestID: "SR-2"},
			"storageId3": StorageParcelElement{SizeBytes: 333},
		},
		PoolID: "provider1",
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := StorageElement{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.StoragePlanStorageElement{
		Intent:    "DATA",
		SizeBytes: swag.Int64(1010101),
		StorageParcels: map[string]models.StorageParcelElement{
			"storageId1": models.StorageParcelElement{SizeBytes: swag.Int64(1), ShareableStorage: true, ProvMinSizeBytes: swag.Int64(2), ProvParcelSizeBytes: swag.Int64(3), ProvRemainingSizeBytes: swag.Int64(4), ProvNodeID: "NODE-1", ProvStorageRequestID: "SR-1"},
			"storageId2": models.StorageParcelElement{SizeBytes: swag.Int64(22), ProvMinSizeBytes: swag.Int64(2), ProvParcelSizeBytes: swag.Int64(3), ProvRemainingSizeBytes: swag.Int64(4), ProvNodeID: "NODE-2", ProvStorageRequestID: "SR-2"},
			"storageId3": models.StorageParcelElement{SizeBytes: swag.Int64(333), ProvMinSizeBytes: swag.Int64(2), ProvParcelSizeBytes: swag.Int64(3), ProvRemainingSizeBytes: swag.Int64(4)},
		},
		PoolID: "provider1",
	}
	// convert to datastore
	o3d := StorageElement{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o3m, *o4m)
}

func TestStoragePlan(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, StoragePlan{}, models.StoragePlan{}, fMap)

	// Test datastore conversion to and from model
	o1d := StoragePlan{
		StorageLayout:   "standalone",
		LayoutAlgorithm: "TBD",
		PlacementHints: StringValueMap{
			"tbd":        {Kind: "STRING", Value: "string1"},
			"notdefined": {Kind: "STRING", Value: "string2"},
		},
		StorageElements: []StorageElement{
			{
				Intent:    "DATA",
				SizeBytes: 10101010,
				StorageParcels: StorageParcelMap{
					"storageId1": StorageParcelElement{SizeBytes: 1},
					"storageId2": StorageParcelElement{SizeBytes: 22},
					"storageId3": StorageParcelElement{SizeBytes: 333},
				},
				PoolID: "provider1",
			}, {
				Intent:    "DATA",
				SizeBytes: 10101010,
				StorageParcels: StorageParcelMap{
					"storageId4": StorageParcelElement{SizeBytes: 1},
					"storageId5": StorageParcelElement{SizeBytes: 22},
					"storageId6": StorageParcelElement{SizeBytes: 333},
				},
				PoolID: "provider2",
			},
		},
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := StoragePlan{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.StoragePlan{
		StorageLayout:   "mirrored",
		LayoutAlgorithm: "TBD",
		PlacementHints: map[string]models.ValueType{
			"property1": {Kind: "STRING", Value: "string3"},
			"property2": {Kind: "STRING", Value: "string4"},
		},
		StorageElements: []*models.StoragePlanStorageElement{
			{
				Intent:    "DATA",
				SizeBytes: swag.Int64(1010101),
				StorageParcels: map[string]models.StorageParcelElement{
					"storageId1": models.StorageParcelElement{SizeBytes: swag.Int64(1), ProvMinSizeBytes: swag.Int64(2), ProvParcelSizeBytes: swag.Int64(3), ProvRemainingSizeBytes: swag.Int64(4)},
					"storageId2": models.StorageParcelElement{SizeBytes: swag.Int64(22), ProvMinSizeBytes: swag.Int64(2), ProvParcelSizeBytes: swag.Int64(3), ProvRemainingSizeBytes: swag.Int64(4)},
					"storageId3": models.StorageParcelElement{SizeBytes: swag.Int64(333), ProvMinSizeBytes: swag.Int64(2), ProvParcelSizeBytes: swag.Int64(3), ProvRemainingSizeBytes: swag.Int64(4)},
				},
				PoolID: "provider1",
			}, {
				Intent:    "DATA",
				SizeBytes: swag.Int64(1010101),
				StorageParcels: map[string]models.StorageParcelElement{
					"storageId4": models.StorageParcelElement{SizeBytes: swag.Int64(1), ProvMinSizeBytes: swag.Int64(2), ProvParcelSizeBytes: swag.Int64(3), ProvRemainingSizeBytes: swag.Int64(4)},
					"storageId5": models.StorageParcelElement{SizeBytes: swag.Int64(22), ProvMinSizeBytes: swag.Int64(2), ProvParcelSizeBytes: swag.Int64(3), ProvRemainingSizeBytes: swag.Int64(4)},
					"storageId6": models.StorageParcelElement{SizeBytes: swag.Int64(333), ProvMinSizeBytes: swag.Int64(2), ProvParcelSizeBytes: swag.Int64(3), ProvRemainingSizeBytes: swag.Int64(4)},
				},
				PoolID: "provider2",
			},
		},
	}
	// convert to datastore
	o3d := StoragePlan{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o3m, *o4m)
}

func TestStorageState(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, StorageState{}, models.StorageState{}, fMap)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := StorageState{
		AttachedNodeDevice: "abc",
		AttachedNodeID:     "def",
		AttachmentState:    "ATTACHING",
		Messages: []TimestampedString{
			TimestampedString{Message: "message1", Time: now},
			TimestampedString{Message: "message2", Time: now},
			TimestampedString{Message: "message3", Time: now},
		},
		DeviceState:      "UNUSED",
		MediaState:       "FORMATTED",
		ProvisionedState: "PROVISIONED",
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := StorageState{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.StorageStateMutable{
		AttachedNodeDevice: "abc",
		AttachedNodeID:     "def",
		AttachmentState:    centrald.DefaultStorageAttachmentState,
		DeviceState:        centrald.DefaultStorageDeviceState,
		MediaState:         centrald.DefaultStorageMediaState,
		Messages:           []*models.TimestampedString{},
		ProvisionedState:   centrald.DefaultStorageProvisionedState,
	}
	// convert to datastore
	o3d := StorageState{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o3m, *o4m)

	// convert model with no states or Messages set to datastore
	o5m := models.StorageStateMutable{}
	o5d := StorageState{}
	o5d.FromModel(&o5m)
	assert.Equal(o5d.AttachmentState, centrald.DefaultStorageAttachmentState)
	assert.Equal(o5d.DeviceState, centrald.DefaultStorageDeviceState)
	assert.Equal(o5d.MediaState, centrald.DefaultStorageMediaState)
	assert.Equal(o5d.ProvisionedState, centrald.DefaultStorageProvisionedState)
	assert.Len(o5d.Messages, 0)
}

func TestCacheAllocationMap(t *testing.T) {
	assert := assert.New(t)

	// Test datastore conversion to and from model
	o1d := CacheAllocationMap{
		"nodeId1": {
			AllocatedSizeBytes: 1,
			RequestedSizeBytes: 2,
		},
		"nodeId2": {
			AllocatedSizeBytes: 22,
			RequestedSizeBytes: 22,
		},
		"nodeId3": {
			AllocatedSizeBytes: 333,
			RequestedSizeBytes: 4444,
		},
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := CacheAllocationMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// retest with empty map
	o1d = CacheAllocationMap{}
	assert.Len(o1d, 0)
	o1m = o1d.ToModel()
	assert.Len(o1m, 0)
	o2d = CacheAllocationMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)
	assert.Len(o2d, 0)

	// Test model conversion to and from datastore
	o3m := map[string]models.CacheAllocation{
		"nodeId1": {
			AllocatedSizeBytes: swag.Int64(1),
			RequestedSizeBytes: swag.Int64(2),
		},
		"nodeId2": {
			AllocatedSizeBytes: swag.Int64(22),
			RequestedSizeBytes: swag.Int64(22),
		},
		"nodeId3": {
			AllocatedSizeBytes: swag.Int64(333),
			RequestedSizeBytes: swag.Int64(444),
		},
	}
	// convert to datastore
	o3d := CacheAllocationMap{}
	o3d.FromModel(o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o4m, o3m)
}

func TestCapacityAllocationMap(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, CapacityAllocation{}, models.CapacityAllocation{}, fMap)

	// Test datastore conversion to and from model
	o1d := CapacityAllocationMap{
		"provId1": CapacityAllocation{ReservedBytes: 2, ConsumedBytes: 1},
		"provId2": CapacityAllocation{ReservedBytes: 33, ConsumedBytes: 22},
		"provId3": CapacityAllocation{ReservedBytes: 444, ConsumedBytes: 333},
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := CapacityAllocationMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// retest with empty map
	o1d = CapacityAllocationMap{}
	assert.Len(o1d, 0)
	o1m = o1d.ToModel()
	assert.Len(o1m, 0)
	o2d = CapacityAllocationMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)
	assert.Len(o2d, 0)

	// Test model conversion to and from datastore
	o3m := map[string]models.CapacityAllocation{
		"provId1": models.CapacityAllocation{ReservedBytes: swag.Int64(2), ConsumedBytes: swag.Int64(1)},
		"provId2": models.CapacityAllocation{ReservedBytes: swag.Int64(33), ConsumedBytes: swag.Int64(22)},
		"provId3": models.CapacityAllocation{ReservedBytes: swag.Int64(444), ConsumedBytes: swag.Int64(333)},
	}
	// convert to datastore
	o3d := CapacityAllocationMap{}
	o3d.FromModel(o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o4m, o3m)
}

func TestParcelAllocationMap(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, ParcelAllocation{}, models.ParcelAllocation{}, fMap)

	// Test datastore conversion to and from model
	o1d := ParcelAllocationMap{
		"storageId1": ParcelAllocation{SizeBytes: 1},
		"storageId2": ParcelAllocation{SizeBytes: 22},
		"storageId3": ParcelAllocation{SizeBytes: 333},
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := ParcelAllocationMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// retest with empty map
	o1d = ParcelAllocationMap{}
	assert.Len(o1d, 0)
	o1m = o1d.ToModel()
	assert.Len(o1m, 0)
	o2d = ParcelAllocationMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)
	assert.Len(o2d, 0)

	// Test model conversion to and from datastore
	o3m := map[string]models.ParcelAllocation{
		"storageId1": models.ParcelAllocation{SizeBytes: swag.Int64(1)},
		"storageId2": models.ParcelAllocation{SizeBytes: swag.Int64(22)},
		"storageId3": models.ParcelAllocation{SizeBytes: swag.Int64(333)},
	}
	// convert to datastore
	o3d := ParcelAllocationMap{}
	o3d.FromModel(o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o4m, o3m)
}

func TestMount(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, Mount{}, models.Mount{}, fMap)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := Mount{
		SnapIdentifier:    "HEAD",
		PitIdentifier:     "",
		MountedNodeID:     "nodeId1",
		MountedNodeDevice: "/dev/abc",
		MountMode:         "READ_WRITE",
		MountState:        "MOUNTED",
		MountTime:         now,
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := Mount{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.Mount{
		SnapIdentifier:    "22",
		PitIdentifier:     "pit-22",
		MountedNodeID:     "nodeId1",
		MountedNodeDevice: "/dev/abc",
		MountMode:         "READ_WRITE",
		MountState:        "MOUNTED",
		MountTime:         o1m.MountTime,
	}
	// convert to datastore
	o3d := Mount{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o3m, *o4m)
}

func TestProgress(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, Progress{}, models.Progress{}, fMap)

	// ATYPICAL converter - do not copy

	// Test datastore conversion to and from model (existential property set)
	o1d := Progress{
		OffsetBytes:      1000,
		PercentComplete:  32,
		Timestamp:        time.Now().Round(0),
		TotalBytes:       2000,
		TransferredBytes: 500,
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := Progress{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// retest with existential value set
	o1d = Progress{Timestamp: time.Now().Round(0)}
	o1m = o1d.ToModel()
	o2d = Progress{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore (existential value set)
	o3m := &models.Progress{Timestamp: strfmt.DateTime(time.Now())}
	// convert to datastore
	o3d := Progress{}
	o3d.FromModel(o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o4m, o3m)

	// existential value not set => nil model
	o5d := Progress{}
	o5m := o5d.ToModel()
	assert.Nil(o5m)

	// nil model => empty
	o6d := Progress{}
	o6d.FromModel(nil)
	assert.Equal(Progress{}, o6d)
}

func TestSnapshotData(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, SnapshotData{}, models.SnapshotData{}, fMap)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := SnapshotData{
		ConsistencyGroupID: "cg-1",
		DeleteAfterTime:    now.Add(90 * 24 * time.Hour),
		Locations: []SnapshotLocation{
			{
				CreationTime: now,
				CspDomainID:  "csp-1",
			},
			{
				CreationTime: now.Add(10 * time.Minute),
				CspDomainID:  "csp-2",
			},
		},
		PitIdentifier:      "pit-1",
		ProtectionDomainID: "pd-1",
		SizeBytes:          1000,
		SnapIdentifier:     "snap-1",
		SnapTime:           now,
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := SnapshotData{}
	o2d.FromModel(&o1m)
	assert.Equal(o1d, o2d)

	// retest with empty
	o1d = SnapshotData{}
	o1m = o1d.ToModel()
	o2d = SnapshotData{}
	o2d.FromModel(&o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.SnapshotData{}
	// convert to datastore
	o3d := SnapshotData{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(swag.Int64Value(o4m.SizeBytes), swag.Int64Value(o3m.SizeBytes))
	o3m.SizeBytes = o4m.SizeBytes
	assert.Equal(o4m, o3m)
}

func TestSnapshotMap(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, SnapshotData{}, models.SnapshotData{}, fMap)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := SnapshotMap{
		"1": SnapshotData{SnapTime: now.Add(-2 * time.Hour), Locations: []SnapshotLocation{}},
		"2": SnapshotData{SnapTime: now.Add(-1 * time.Hour), Locations: []SnapshotLocation{}},
		"3": SnapshotData{SnapTime: now, Locations: []SnapshotLocation{}},
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := SnapshotMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// retest with empty map
	o1d = SnapshotMap{}
	assert.Len(o1d, 0)
	o1m = o1d.ToModel()
	assert.Len(o1m, 0)
	o2d = SnapshotMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)
	assert.Len(o2d, 0)

	// Test model conversion to and from datastore
	o3m := map[string]models.SnapshotData{
		"1": models.SnapshotData{SnapTime: strfmt.DateTime(now.Add(-2 * time.Hour)), SizeBytes: swag.Int64(1000), Locations: []*models.SnapshotLocation{}},
		"2": models.SnapshotData{SnapTime: strfmt.DateTime(now.Add(-1 * time.Hour)), SizeBytes: swag.Int64(2000), Locations: []*models.SnapshotLocation{}},
		"3": models.SnapshotData{SnapTime: strfmt.DateTime(now.Add(time.Hour)), SizeBytes: swag.Int64(3000), Locations: []*models.SnapshotLocation{}},
	}
	// convert to datastore
	o3d := SnapshotMap{}
	o3d.FromModel(o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o4m, o3m)
}

func TestConvertSnapshotCatalogPolicy(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, SnapshotCatalogPolicy{}, models.SnapshotCatalogPolicy{}, fMap)

	// ATYPICAL converter - do not copy

	// Test datastore conversion to and from model (existential property set)
	o1d := SnapshotCatalogPolicy{
		CspDomainID:        "csp-1",
		ProtectionDomainID: "pd-1",
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := SnapshotCatalogPolicy{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// retest with existential value set
	o1d = SnapshotCatalogPolicy{Inherited: false}
	o1m = o1d.ToModel()
	o2d = SnapshotCatalogPolicy{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore (existential value set)
	o3m := &models.SnapshotCatalogPolicy{Inherited: false}
	// convert to datastore
	o3d := SnapshotCatalogPolicy{}
	o3d.FromModel(o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o4m, o3m)

	// existential value not set => nil model
	o5d := SnapshotCatalogPolicy{Inherited: true}
	o5m := o5d.ToModel()
	assert.Nil(o5m)

	// nil model => empty (except for Inherited)
	o6d := SnapshotCatalogPolicy{}
	o6d.FromModel(nil)
	assert.Equal(SnapshotCatalogPolicy{Inherited: true}, o6d)

	// if existential value not set then model is cleared of other values
	o7d := SnapshotCatalogPolicy{Inherited: true, CspDomainID: "x", ProtectionDomainID: "y"}
	o7m := o7d.ToModel()
	o8d := SnapshotCatalogPolicy{Inherited: true}
	o8m := o8d.ToModel()
	assert.Equal(o8m, o7m)
}

func TestSnapshotManagementPolicy(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, SnapshotManagementPolicy{}, models.SnapshotManagementPolicy{}, fMap)

	// ATYPICAL converter - do not copy

	// Test datastore conversion to and from model (existential property set)
	o1d := SnapshotManagementPolicy{
		DeleteLast:                  true,
		DeleteVolumeWithLast:        false,
		DisableSnapshotCreation:     true,
		NoDelete:                    true,
		RetentionDurationSeconds:    999999,
		VolumeDataRetentionOnDelete: "",
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := SnapshotManagementPolicy{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// retest with existential value set
	o1d = SnapshotManagementPolicy{Inherited: false}
	o1m = o1d.ToModel()
	o2d = SnapshotManagementPolicy{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore (existential value set)
	o3m := &models.SnapshotManagementPolicy{Inherited: false}
	// convert to datastore
	o3d := SnapshotManagementPolicy{}
	o3d.FromModel(o3m)
	assert.Equal(int32(0), o3d.RetentionDurationSeconds)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(int32(0), swag.Int32Value(o4m.RetentionDurationSeconds))

	// existential value not set => nil model
	o5d := SnapshotManagementPolicy{Inherited: true}
	o5m := o5d.ToModel()
	assert.Nil(o5m)

	// nil model => empty
	o6d := SnapshotManagementPolicy{}
	o6d.FromModel(nil)
	assert.Equal(SnapshotManagementPolicy{Inherited: true}, o6d)
}

func TestLifecycleManagementData(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, LifecycleManagementData{}, models.LifecycleManagementData{}, fMap)

	// Test datastore conversion to and from model
	o1d := LifecycleManagementData{
		EstimatedSizeBytes:        32 * 1024 * 1024,
		FinalSnapshotNeeded:       true,
		GenUUID:                   "uuid",
		WriteIOCount:              103100,
		LastUploadTime:            time.Now(),
		LastSnapTime:              time.Now().Add(-2 * time.Hour),
		LastUploadSizeBytes:       27 * 1024 * 1024,
		LastUploadTransferRateBPS: 100000,
		NextSnapshotTime:          time.Now().Add(4 * time.Hour),
		SizeEstimateRatio:         1.02,
		LayoutAlgorithm:           "layoutAlgorithm",
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := LifecycleManagementData{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// retest with empty
	o1d = LifecycleManagementData{}
	o1m = o1d.ToModel()
	o2d = LifecycleManagementData{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := &models.LifecycleManagementData{}
	// convert to datastore
	o3d := LifecycleManagementData{}
	o3d.FromModel(o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o4m, o3m)

	// nil model => empty
	o5d := LifecycleManagementData{}
	o5d.FromModel(nil)
	assert.Equal(LifecycleManagementData{}, o5d)
}

func TestSyncPeerMap(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, SyncPeer{}, models.SyncPeer{}, fMap)

	// Test datastore conversion to and from model
	o1d := SyncPeerMap{
		"1": SyncPeer{ID: "id1", State: "S1", GenCounter: 1},
		"2": SyncPeer{ID: "id2", State: "S2", GenCounter: 1, Annotation: "note"},
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := SyncPeerMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// retest with empty map
	o1d = SyncPeerMap{}
	assert.Len(o1d, 0)
	o1m = o1d.ToModel()
	assert.Len(o1m, 0)
	o2d = SyncPeerMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)
	assert.Len(o2d, 0)

	// Test model conversion to and from datastore
	o3m := map[string]models.SyncPeer{
		"1": models.SyncPeer{ID: "id1", State: "S1", Annotation: "noted"},
		"2": models.SyncPeer{ID: "id2", State: "S2"},
	}
	// convert to datastore
	o3d := SyncPeerMap{}
	o3d.FromModel(o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o4m, o3m)
}

func TestConvertAccount(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{
		"Meta": "ObjMeta",
	}
	cmpObjToModel(t, Account{}, models.Account{}, fMap)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := Account{
		ObjMeta: ObjMeta{
			MetaObjID:        uuid.NewV4().String(),
			MetaVersion:      1,
			MetaTimeCreated:  now,
			MetaTimeModified: now,
		},
		Name:            "account1",
		Description:     "account1 object",
		TenantAccountID: "tenantId1",
		Disabled:        false,
		AccountRoles:    []string{"role1", "role2"},
		UserRoles: AuthRoleMap{
			centrald.SystemUser: AuthRole{RoleID: "role1", Disabled: false},
		},
		Messages: []TimestampedString{{Message: "msg", Time: now}},
		Secrets:  map[string]string{},
		// No Tags specified in datastore object
	}
	// convert to model
	o1m := o1d.ToModel()
	assert.Equal("Account", o1m.Meta.ObjType, "ObjType is set")
	assert.Len(o1d.Tags, 0) // created as a side effect
	assert.NotNil(o1d.Tags)
	assert.Len(o1m.Tags, 0)
	assert.NotNil(o1m.SnapshotManagementPolicy)
	assert.Nil(o1m.ProtectionDomains)
	// convert back
	o2d := Account{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{
				ID:          o1m.Meta.ID,
				Version:     o1m.Meta.Version,
				ObjType:     "Account",
				TimeCreated: o1m.Meta.TimeCreated,
			},
			AccountRoles:    []models.ObjIDMutable{"role1", "role2"},
			TenantAccountID: "tenantID1",
		},
		AccountMutable: models.AccountMutable{
			Name:        "account2",
			Description: "account2 object",
			Disabled:    true,
			Messages:    []*models.TimestampedString{&models.TimestampedString{Message: "msg"}},
			Tags:        models.ObjTags{"tag1", "tag2", "tag3"},
			UserRoles: map[string]models.AuthRole{
				centrald.SystemUser: models.AuthRole{RoleID: "role1"},
				"otherUser":         models.AuthRole{RoleID: "role2", Disabled: true},
			},
			SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
				RetentionDurationSeconds: swag.Int32(10),
			},
			ProtectionDomains: map[string]models.ObjIDMutable{
				"DEFAULT": "PD-1",
			},
		},
	}
	// convert to datastore
	o3d := Account{}
	o3d.FromModel(&o3m)
	assert.Nil(o3d.Secrets)
	// convert back
	o4m := o3d.ToModel()
	assert.NotEqual(time.Time(o3m.Meta.TimeModified), time.Time(o4m.Meta.TimeModified))
	o3m.Meta.TimeModified = o4m.Meta.TimeModified
	assert.Equal(o3m, *o4m)

	// Test model without arrays conversion to datastore
	o3m.Meta = nil
	o3m.Tags = nil
	o3m.Messages = nil
	o3m.AccountRoles = nil
	o3m.UserRoles = nil
	o4d := Account{}
	o4d.FromModel(&o3m)
	assert.NotEmpty(o4d.ObjMeta.MetaVersion)
	assert.EqualValues(1, o4d.ObjMeta.MetaVersion)
	assert.NotNil(o4d.AccountRoles, "AccountRoles always set in datastore")
	assert.NotNil(o4d.UserRoles, "UserRoles always set in datastore")
	assert.NotNil(o4d.Messages, "Messages always set in datastore")
	assert.NotNil(o4d.Tags, "Tags always set in datastore")
}

func TestConvertApplicationGroup(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{
		"Meta": "ObjMeta",
	}
	cmpObjToModel(t, ApplicationGroup{}, models.ApplicationGroup{}, fMap)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := ApplicationGroup{
		ObjMeta: ObjMeta{
			MetaObjID:        uuid.NewV4().String(),
			MetaVersion:      1,
			MetaTimeCreated:  now,
			MetaTimeModified: now,
		},
		AccountID:       "account1",
		TenantAccountID: "tenant1",
		Name:            "vs1",
		Description:     "vs1 object",
		// No Tags specified in datastore object
	}

	assert.True(o1d.Tags == nil)
	assert.True(o1d.SystemTags == nil)
	// convert to model
	o1m := o1d.ToModel()
	assert.Equal("ApplicationGroup", o1m.Meta.ObjType, "ObjType is set")
	assert.Len(o1d.Tags, 0) // created as a side effect
	assert.Len(o1m.Tags, 0)
	assert.Len(o1d.SystemTags, 0) // created as a side effect
	assert.Len(o1m.SystemTags, 0)
	// convert back
	o2d := ApplicationGroup{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)
	assert.True(o2d.Tags != nil)
	assert.True(o2d.SystemTags != nil)

	// Test model conversion to and from datastore
	o3m := models.ApplicationGroup{
		ApplicationGroupAllOf0: models.ApplicationGroupAllOf0{
			Meta: &models.ObjMeta{
				ID:          o1m.Meta.ID,
				Version:     o1m.Meta.Version,
				ObjType:     "ApplicationGroup",
				TimeCreated: o1m.Meta.TimeCreated,
			},
		},
		ApplicationGroupCreateOnce: models.ApplicationGroupCreateOnce{
			AccountID:       "account1",
			TenantAccountID: "tenant1",
		},
		ApplicationGroupMutable: models.ApplicationGroupMutable{
			Name:        o1m.Name,
			Description: o1m.Description,
			Tags:        []string{"tag1", "tag2"},
			SystemTags:  []string{"stag1", "stag2"},
		},
	}
	// convert to datastore
	o3d := ApplicationGroup{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.NotEqual(time.Time(o3m.Meta.TimeModified), time.Time(o4m.Meta.TimeModified))
	o3m.Meta.TimeModified = o4m.Meta.TimeModified
	assert.Equal(o3m, *o4m)

	// Test model without Tags, et al conversion to datastore
	o3m.Tags = nil
	o3m.SystemTags = nil
	o4d := ApplicationGroup{}
	o4d.FromModel(&o3m)
	assert.Len(o4d.Tags, 0, "Tags always set in datastore")
	assert.Len(o4d.SystemTags, 0, "SystemTags always set in datastore")
}

func TestConvertCluster(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{
		"Meta": "ObjMeta",
	}
	cmpObjToModel(t, Cluster{}, models.Cluster{}, fMap)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := Cluster{
		ObjMeta: ObjMeta{
			MetaObjID:        uuid.NewV4().String(),
			MetaVersion:      1,
			MetaTimeCreated:  now,
			MetaTimeModified: now,
		},
		Name:        "cluster1",
		Description: "cluster1 object",
		// No Tags specified in datastore object
		ClusterAttributes: StringValueMap{
			"attr1": ValueType{Kind: "INT", Value: "1"},
			"attr2": ValueType{Kind: "STRING", Value: "a string"},
		},
		ClusterVersion: "1.7",
		Service: NuvoService{
			ServiceType:    "Clusterd",
			ServiceVersion: "1.1.1",
			ServiceState: ServiceState{
				HeartbeatPeriodSecs: 90,
				HeartbeatTime:       now,
				State:               "UNKNOWN",
			},
			ServiceAttributes: StringValueMap{
				"attr1": ValueType{Kind: "INT", Value: "1"},
				"attr2": ValueType{Kind: "STRING", Value: "a string"},
			},
			Messages: []TimestampedString{
				TimestampedString{Message: "message1", Time: now},
				TimestampedString{Message: "message2", Time: now},
				TimestampedString{Message: "message3", Time: now},
			},
		},
		ClusterIdentifier: "myCluster",
		ClusterType:       "Kubernetes",
		CspDomainID:       "cspDomainId",
		AccountID:         "systemId",
		ClusterUsagePolicy: ClusterUsagePolicy{
			AccountSecretScope: "CLUSTER",
		},
		State: "ACTIVE",
		Messages: []TimestampedString{
			{Message: "message1", Time: now},
			{Message: "message2", Time: now},
			{Message: "message3", Time: now},
		},
	}
	// convert to model
	o1m := o1d.ToModel()
	assert.Equal("Cluster", o1m.Meta.ObjType, "ObjType is set")
	assert.Len(o1d.Tags, 0) // created as a side effect
	assert.Len(o1m.Tags, 0)
	// convert back
	o2d := Cluster{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID:          o1m.Meta.ID,
				Version:     o1m.Meta.Version,
				ObjType:     "Cluster",
				TimeCreated: o1m.Meta.TimeCreated,
			},
		},
		ClusterMutable: models.ClusterMutable{
			ClusterCreateMutable: models.ClusterCreateMutable{
				Name:               "cluster2",
				Description:        "cluster2 object",
				Tags:               models.ObjTags{"tag1", "tag2", "tag3"},
				ClusterAttributes:  o1m.ClusterAttributes,
				AuthorizedAccounts: []models.ObjIDMutable{"accountId1"},
				ClusterIdentifier:  "myCluster2",
				State:              "ACTIVE",
				Messages:           []*models.TimestampedString{&models.TimestampedString{Message: "msg"}},
			},
			ClusterMutableAllOf0: models.ClusterMutableAllOf0{
				Service:        o1m.Service,
				ClusterVersion: o1m.ClusterVersion,
			},
		},
		ClusterCreateOnce: models.ClusterCreateOnce{
			AccountID:   "systemId",
			ClusterType: "Kubernetes",
			CspDomainID: "cspDomainId",
		},
	}
	// convert to datastore (existential properties not set)
	o3d := Cluster{}
	o3d.FromModel(&o3m)
	assert.True(o3d.ClusterUsagePolicy.Inherited)
	// convert back
	o4m := o3d.ToModel()
	assert.NotEqual(time.Time(o3m.Meta.TimeModified), time.Time(o4m.Meta.TimeModified))
	o3m.Meta.TimeModified = o4m.Meta.TimeModified
	assert.Nil(o4m.ClusterUsagePolicy)
	assert.Equal(o3m, *o4m)

	// Test model with nil pointer types conversion to datastore
	o3m.Meta = nil
	o3m.Tags = nil
	o3m.ClusterAttributes = nil
	o3m.Service = nil
	o3m.AuthorizedAccounts = nil
	o3m.ClusterUsagePolicy = nil
	o4d := Cluster{}
	o4d.FromModel(&o3m)
	assert.NotEmpty(o4d.ObjMeta.MetaVersion)
	assert.EqualValues(1, o4d.ObjMeta.MetaVersion)
	assert.Len(o4d.Tags, 0)
	assert.Len(o4d.ClusterAttributes, 0)
	assert.NotNil(o4d.Service)
	assert.NotNil(o4d.AuthorizedAccounts)
	assert.Len(o4d.AuthorizedAccounts, 0)
	assert.True(o4d.ClusterUsagePolicy.Inherited)

	// convert empty Cluster to model
	o5d := Cluster{}
	o5m := o5d.ToModel()
	assert.NotNil(o5m.AuthorizedAccounts)
	assert.NotNil(o5m.ClusterAttributes)
	assert.NotNil(o5m.Tags)
	assert.NotNil(o5m.Service)
	assert.NotNil(o5m.Service.ServiceAttributes)
	assert.NotNil(o5m.Service.ServiceState)
	assert.NotNil(o5m.Service.Messages)
	assert.NotNil(o5m.ClusterUsagePolicy)
	assert.Equal(&models.ClusterUsagePolicy{}, o5m.ClusterUsagePolicy)
}

func TestConvertClusterUsagePolicy(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, ClusterUsagePolicy{}, models.ClusterUsagePolicy{}, fMap)

	// ATYPICAL converter - do not copy

	// Test datastore conversion to and from model (existential property set)
	o1d := ClusterUsagePolicy{
		AccountSecretScope:          "GLOBAL",
		ConsistencyGroupName:        "${k8sPod.name}",
		VolumeDataRetentionOnDelete: "DELETE",
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := ClusterUsagePolicy{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// retest with existential value set
	o1d = ClusterUsagePolicy{Inherited: false}
	o1m = o1d.ToModel()
	o2d = ClusterUsagePolicy{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore (existential value set)
	o3m := &models.ClusterUsagePolicy{Inherited: false}
	// convert to datastore
	o3d := ClusterUsagePolicy{}
	o3d.FromModel(o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o4m, o3m)

	// existential value not set => nil model
	o5d := ClusterUsagePolicy{Inherited: true}
	o5m := o5d.ToModel()
	assert.Nil(o5m)

	// nil model => empty (except for Inherited)
	o6d := ClusterUsagePolicy{}
	o6d.FromModel(nil)
	assert.Equal(ClusterUsagePolicy{Inherited: true}, o6d)
}

func TestConvertConsistencyGroup(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{
		"Meta": "ObjMeta",
	}
	cmpObjToModel(t, ConsistencyGroup{}, models.ConsistencyGroup{}, fMap)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := ConsistencyGroup{
		ObjMeta: ObjMeta{
			MetaObjID:        uuid.NewV4().String(),
			MetaVersion:      1,
			MetaTimeCreated:  now,
			MetaTimeModified: now,
		},
		AccountID:           "account1",
		TenantAccountID:     "tenant1",
		Name:                "vs1",
		Description:         "vs1 object",
		ApplicationGroupIds: ObjIDList{"app"},
		// No Tags specified in datastore object
	}

	assert.True(o1d.Tags == nil)
	assert.True(o1d.SystemTags == nil)
	// convert to model
	o1m := o1d.ToModel()
	assert.Equal("ConsistencyGroup", o1m.Meta.ObjType, "ObjType is set")
	assert.Len(o1d.Tags, 0) // created as a side effect
	assert.Len(o1m.Tags, 0)
	assert.Len(o1d.SystemTags, 0) // created as a side effect
	assert.Len(o1m.SystemTags, 0)
	assert.NotNil(o1m.SnapshotManagementPolicy)
	// convert back
	o2d := ConsistencyGroup{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)
	assert.True(o2d.Tags != nil)
	assert.True(o2d.SystemTags != nil)

	// Test model conversion to and from datastore
	o3m := models.ConsistencyGroup{
		ConsistencyGroupAllOf0: models.ConsistencyGroupAllOf0{
			Meta: &models.ObjMeta{
				ID:          o1m.Meta.ID,
				Version:     o1m.Meta.Version,
				ObjType:     "ConsistencyGroup",
				TimeCreated: o1m.Meta.TimeCreated,
			},
		},
		ConsistencyGroupCreateOnce: models.ConsistencyGroupCreateOnce{
			AccountID:       "account1",
			TenantAccountID: "tenant1",
		},
		ConsistencyGroupMutable: models.ConsistencyGroupMutable{
			Name:                o1m.Name,
			Description:         o1m.Description,
			ApplicationGroupIds: []models.ObjIDMutable{"app1", "app2"},
			SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
				RetentionDurationSeconds: swag.Int32(10),
			},
			Tags:       []string{"tag1", "tag2"},
			SystemTags: []string{"stag1", "stag2"},
		},
	}
	// convert to datastore
	o3d := ConsistencyGroup{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.NotEqual(time.Time(o3m.Meta.TimeModified), time.Time(o4m.Meta.TimeModified))
	o3m.Meta.TimeModified = o4m.Meta.TimeModified
	assert.Equal(o3m, *o4m)

	// Test model without Tags, et al conversion to datastore
	o3m.ApplicationGroupIds = nil
	o3m.Tags = nil
	o3m.SystemTags = nil
	o4d := ConsistencyGroup{}
	o4d.FromModel(&o3m)
	assert.NotNil(o4d.ApplicationGroupIds, "ApplicationGroupIds always set in datastore")
	assert.NotNil(o4d.Tags, "Tags always set in datastore")
	assert.NotNil(o4d.SystemTags, "SystemTags always set in datastore")
}

func TestConvertCSPDomain(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{
		"Meta": "ObjMeta",
	}
	cmpObjToModel(t, CspDomain{}, models.CSPDomain{}, fMap)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := CspDomain{
		ObjMeta: ObjMeta{
			MetaObjID:        uuid.NewV4().String(),
			MetaVersion:      1,
			MetaTimeCreated:  now,
			MetaTimeModified: now,
		},
		CspDomainType:  "AWS",
		AccountID:      "accountId1",
		Name:           "cspDomain1",
		Description:    "cspDomain1 object",
		ManagementHost: "f.q.d.n",
		CspDomainAttributes: StringValueMap{
			"cred": ValueType{Kind: "SECRET", Value: "secret"},
			"id":   ValueType{Kind: "STRING", Value: "user"},
		},
		ClusterUsagePolicy: ClusterUsagePolicy{
			AccountSecretScope: "CSPDOMAIN",
		},
		CspCredentialID: "cspCredID",
		StorageCosts: StorageCostMap{
			"st1": StorageCost{CostPerGiB: 0.003},
			"st2": StorageCost{CostPerGiB: 23.9},
			"st3": StorageCost{},
		},
		// No Tags specified in datastore object
	}

	// convert to model
	o1m := o1d.ToModel()
	assert.Equal("CspDomain", o1m.Meta.ObjType, "ObjType is set")
	assert.Len(o1d.Tags, 0) // created as a side effect
	assert.Len(o1m.Tags, 0)
	// convert back
	o2d := CspDomain{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID:          o1m.Meta.ID,
				Version:     o1m.Meta.Version,
				ObjType:     "CspDomain",
				TimeCreated: o1m.Meta.TimeCreated,
			},
			CspDomainType: "AWS",
			AccountID:     "accountId1",
			CspDomainAttributes: map[string]models.ValueType{
				"cred": models.ValueType{Kind: "SECRET", Value: "secret"},
				"id":   models.ValueType{Kind: "STRING", Value: "user"},
			},
		},
		CSPDomainMutable: models.CSPDomainMutable{
			Name:               "cspDomain1",
			Description:        "cspDomain1 object",
			ManagementHost:     "1.2.3.4",
			Tags:               models.ObjTags{"tag1", "tag2", "tag3"},
			AuthorizedAccounts: []models.ObjIDMutable{"a-aid1", "a-aid2"},
			CspCredentialID:    "cspCredId",
			StorageCosts: map[string]models.StorageCost{
				"st1": models.StorageCost{CostPerGiB: 0.003},
				"st2": models.StorageCost{CostPerGiB: 23.9},
				"st3": models.StorageCost{},
			},
		},
	}

	// convert to datastore (existential properties not set)
	o3d := CspDomain{}
	o3d.FromModel(&o3m)
	assert.True(o3d.ClusterUsagePolicy.Inherited)
	// convert back
	o4m := o3d.ToModel()
	assert.NotEqual(time.Time(o3m.Meta.TimeModified), time.Time(o4m.Meta.TimeModified))
	o3m.Meta.TimeModified = o4m.Meta.TimeModified
	assert.Nil(o4m.ClusterUsagePolicy)
	assert.Equal(o3m, *o4m)

	// Test model without Tags or CSPDomainAttributes conversion to datastore
	o3m.Tags = nil
	o3m.CspDomainAttributes = nil
	o3m.AuthorizedAccounts = nil
	o3m.ClusterUsagePolicy = nil
	o4d := CspDomain{}
	o4d.FromModel(&o3m)
	assert.Len(o4d.Tags, 0, "Tags always set in datastore")
	assert.Len(o4d.CspDomainAttributes, 0, "CspDomainAttributes always set in datastore")
	assert.NotNil(o4d.AuthorizedAccounts)
	assert.Len(o4d.AuthorizedAccounts, 0)
	assert.True(o4d.ClusterUsagePolicy.Inherited)
}

func TestConvertNode(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{
		"Meta": "ObjMeta",
	}
	cmpObjToModel(t, Node{}, models.Node{}, fMap)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := Node{
		ObjMeta: ObjMeta{
			MetaObjID:        uuid.NewV4().String(),
			MetaVersion:      1,
			MetaTimeCreated:  now,
			MetaTimeModified: now,
		},
		AccountID:   "aid1",
		ClusterID:   "clusterId",
		Name:        "node1",
		Description: "node1 object",
		LocalStorage: NodeStorageDeviceMap{
			"id1": {DeviceName: "n1", DeviceState: "s1", DeviceType: "t1", SizeBytes: 11, UsableSizeBytes: 10},
			"id2": {DeviceName: "n2", DeviceState: "s2", DeviceType: "t2", SizeBytes: 22, UsableSizeBytes: 20},
		},
		AvailableCacheBytes: 1,
		CacheUnitSizeBytes:  1,
		TotalCacheBytes:     2,
		NodeIdentifier:      "127.0.0.1",
		NodeAttributes: StringValueMap{
			"attr1": ValueType{Kind: "INT", Value: "1"},
			"attr2": ValueType{Kind: "STRING", Value: "a string"},
		},
		Service: NuvoService{
			ServiceType:    "Agentd",
			ServiceVersion: "1.2.1",
			ServiceState: ServiceState{
				HeartbeatPeriodSecs: 90,
				HeartbeatTime:       now,
				State:               "UNKNOWN",
			},
			ServiceAttributes: StringValueMap{
				"attr1": ValueType{Kind: "INT", Value: "1"},
				"attr2": ValueType{Kind: "STRING", Value: "a string"},
			},
			Messages: []TimestampedString{
				TimestampedString{Message: "message1", Time: now},
				TimestampedString{Message: "message2", Time: now},
				TimestampedString{Message: "message3", Time: now},
			},
		},
		State: "MANAGED",
		// No Tags specified in datastore object
	}
	// convert to model
	o1m := o1d.ToModel()
	assert.Equal("Node", o1m.Meta.ObjType, "ObjType is set")
	assert.Len(o1d.Tags, 0) // created as a side effect
	assert.Len(o1m.Tags, 0)
	// convert back
	o2d := Node{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.Node{
		NodeAllOf0: models.NodeAllOf0{
			Meta: &models.ObjMeta{
				ID:          o1m.Meta.ID,
				Version:     o1m.Meta.Version,
				ObjType:     "Node",
				TimeCreated: o1m.Meta.TimeCreated,
			},
			AccountID:      "aid1",
			ClusterID:      "clusterId",
			NodeIdentifier: "127.0.0.1",
		},
		NodeMutable: models.NodeMutable{
			Name:        "node2",
			Description: "node2 object",
			LocalStorage: map[string]models.NodeStorageDevice{
				"id1": {DeviceName: "n1", DeviceState: "s1", DeviceType: "t1", SizeBytes: swag.Int64(11), UsableSizeBytes: swag.Int64(10)},
				"id2": {DeviceName: "n2", DeviceState: "s2", DeviceType: "t2", SizeBytes: swag.Int64(22), UsableSizeBytes: swag.Int64(20)},
			},
			AvailableCacheBytes: swag.Int64(1),
			CacheUnitSizeBytes:  swag.Int64(2),
			TotalCacheBytes:     swag.Int64(2),
			Tags:                models.ObjTags{"tag1", "tag2", "tag3"},
			Service:             o1m.Service,
			State:               "TEAR_DOWN",
			NodeAttributes:      o1m.NodeAttributes,
		},
	}
	// convert to datastore
	o3d := Node{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.NotEqual(time.Time(o3m.Meta.TimeModified), time.Time(o4m.Meta.TimeModified))
	o3m.Meta.TimeModified = o4m.Meta.TimeModified
	assert.Equal(o3m, *o4m)

	// Test model with nil pointer types conversion to datastore
	o3m.Tags = nil
	o3m.LocalStorage = nil
	o3m.NodeAttributes = nil
	o3m.Service = nil
	o4d := Node{}
	o4d.FromModel(&o3m)
	assert.NotNil(o4d.Tags)
	assert.Empty(o4d.Tags)
	assert.NotNil(o4d.LocalStorage)
	assert.Empty(o4d.LocalStorage)
	assert.NotNil(o4d.NodeAttributes)
	assert.Empty(o4d.NodeAttributes)
	assert.NotNil(o4d.Service)

	// convert empty Node to model
	o5d := Node{}
	o5m := o5d.ToModel()
	assert.NotNil(o5m.LocalStorage)
	assert.NotNil(o5m.NodeAttributes)
	assert.NotNil(o5m.Tags)
	assert.NotNil(o5m.Service)
	assert.NotNil(o5m.Service.ServiceAttributes)
	assert.NotNil(o5m.Service.ServiceState)
	assert.NotNil(o5m.Service.Messages)
}

func TestConvertPool(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{
		"Meta": "ObjMeta",
	}
	cmpObjToModel(t, Pool{}, models.Pool{}, fMap)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := Pool{
		ObjMeta: ObjMeta{
			MetaObjID:        uuid.NewV4().String(),
			MetaVersion:      1,
			MetaTimeCreated:  now,
			MetaTimeModified: now,
		},
		// No Tags specified in datastore object
		AccountID:           "accountId",
		AuthorizedAccountID: "authorizedAccountId",
		ClusterID:           "clusterId",
		CspDomainID:         "cspDomainId",
		CspStorageType:      "EBS-GP2",
		StorageAccessibility: StorageAccessibility{
			AccessibilityScope:      "CSPDOMAIN",
			AccessibilityScopeObjID: "cspDomainId",
		},
		ServicePlanReservations: StorageTypeReservationMap{
			"spa1": StorageTypeReservation{NumMirrors: 1, SizeBytes: 1000},
		},
		// No Tags specified in datastore object
	}
	assert.True(o1d.SystemTags == nil)
	// convert to model
	o1m := o1d.ToModel()
	assert.Equal("Pool", o1m.Meta.ObjType, "ObjType is set")
	assert.Len(o1d.SystemTags, 0) // created as a side effect
	assert.Len(o1m.SystemTags, 0)
	// convert back
	o2d := Pool{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.Pool{
		PoolAllOf0: models.PoolAllOf0{
			Meta: &models.ObjMeta{
				ID:          o1m.Meta.ID,
				Version:     o1m.Meta.Version,
				ObjType:     "Pool",
				TimeCreated: o1m.Meta.TimeCreated,
			},
		},
		PoolMutable: models.PoolMutable{
			PoolCreateMutable: models.PoolCreateMutable{
				SystemTags: []string{"stag1", "stag2"},
			},
			PoolMutableAllOf0: models.PoolMutableAllOf0{
				ServicePlanReservations: o1m.ServicePlanReservations,
			},
		},
		PoolCreateOnce: models.PoolCreateOnce{
			AccountID:           o1m.AccountID,
			AuthorizedAccountID: o1m.AuthorizedAccountID,
			ClusterID:           o1m.ClusterID,
			CspDomainID:         o1m.CspDomainID,
			CspStorageType:      o1m.CspStorageType,
			StorageAccessibility: &models.StorageAccessibilityMutable{
				AccessibilityScope:      o1m.StorageAccessibility.AccessibilityScope,
				AccessibilityScopeObjID: o1m.StorageAccessibility.AccessibilityScopeObjID,
			},
		},
	}
	// convert to datastore
	o3d := Pool{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.NotEqual(time.Time(o3m.Meta.TimeModified), time.Time(o4m.Meta.TimeModified))
	o3m.Meta.TimeModified = o4m.Meta.TimeModified
	assert.Equal(o3m, *o4m)

	// Test model with nil pointer types conversion to datastore
	o3m.StorageAccessibility = nil
	o3m.SystemTags = nil
	o3m.ServicePlanReservations = nil
	o4d := Pool{}
	o4d.FromModel(&o3m)
	assert.Len(o4d.SystemTags, 0, "SystemTags always set in datastore")
	assert.Len(o4d.ServicePlanReservations, 0)

	// convert empty Pool to model
	o5d := Pool{}
	o5m := o5d.ToModel()
	assert.NotNil(o5m.ServicePlanReservations)
	assert.NotNil(o5m.SystemTags)
}

func TestConvertProtectionDomain(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{
		"Meta": "ObjMeta",
	}
	cmpObjToModel(t, ProtectionDomain{}, models.ProtectionDomain{}, fMap)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := ProtectionDomain{
		ObjMeta: ObjMeta{
			MetaObjID:        uuid.NewV4().String(),
			MetaVersion:      1,
			MetaTimeCreated:  now,
			MetaTimeModified: now,
		},
		AccountID:            "accountId",
		EncryptionAlgorithm:  "AES-192",
		EncryptionPassphrase: "my pass phrase",
		Name:                 "MyPD",
		Description:          "My PD",
	}
	assert.True(o1d.SystemTags == nil)
	// convert to model
	o1m := o1d.ToModel()
	assert.Equal("ProtectionDomain", o1m.Meta.ObjType, "ObjType is set")
	assert.Len(o1d.SystemTags, 0) // created as a side effect
	assert.Len(o1m.SystemTags, 0)
	assert.Len(o1d.Tags, 0) // created as a side effect
	assert.Len(o1m.Tags, 0)
	// convert back
	o2d := ProtectionDomain{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.ProtectionDomain{
		ProtectionDomainAllOf0: models.ProtectionDomainAllOf0{
			Meta: &models.ObjMeta{
				ID:          o1m.Meta.ID,
				Version:     o1m.Meta.Version,
				ObjType:     "ProtectionDomain",
				TimeCreated: o1m.Meta.TimeCreated,
			},
		},
		ProtectionDomainMutable: models.ProtectionDomainMutable{
			Name:        "pd1",
			Description: "protection domain 1",
			SystemTags:  []string{"stag1"},
			Tags:        []string{"tag1"},
		},
		ProtectionDomainCreateOnce: models.ProtectionDomainCreateOnce{
			AccountID:            o1m.AccountID,
			EncryptionAlgorithm:  "AES-256",
			EncryptionPassphrase: &models.ValueType{Kind: "SECRET", Value: "thequickbrownfoxjumpsoverthelazydog"},
		},
	}
	// convert to datastore
	o3d := ProtectionDomain{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.NotEqual(time.Time(o3m.Meta.TimeModified), time.Time(o4m.Meta.TimeModified))
	o3m.Meta.TimeModified = o4m.Meta.TimeModified
	assert.Equal(o3m, *o4m)

	// convert empty ProtectionDomain to model
	o5d := ProtectionDomain{}
	o5m := o5d.ToModel()
	assert.NotNil(o5m.SystemTags)
	assert.NotNil(o5m.Tags)

	// handle nil pointers
	o6m := models.ProtectionDomain{}
	o6d := &ProtectionDomain{}
	o6d.FromModel(&o6m)
}

func TestConvertRole(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{
		"Meta": "ObjMeta",
	}
	cmpObjToModel(t, Role{}, models.Role{}, fMap)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := Role{
		ObjMeta: ObjMeta{
			MetaObjID:        uuid.NewV4().String(),
			MetaVersion:      1,
			MetaTimeCreated:  now,
			MetaTimeModified: now,
		},
		Name: "peon",
		Capabilities: CapabilityMap{
			"cap1": true,
			"cap2": false,
		},
	}
	// convert to model
	o1m := o1d.ToModel()
	assert.Equal("Role", o1m.Meta.ObjType, "ObjType is set")
	// convert back
	o2d := Role{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.Role{
		RoleAllOf0: models.RoleAllOf0{
			Meta: &models.ObjMeta{
				ID:          o1m.Meta.ID,
				Version:     o1m.Meta.Version,
				ObjType:     "Role",
				TimeCreated: o1m.Meta.TimeCreated,
			},
		},
		RoleMutable: models.RoleMutable{
			Name:         o1m.Name,
			Capabilities: o1m.Capabilities,
		},
	}
	// convert to datastore
	o3d := Role{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.NotEqual(time.Time(o3m.Meta.TimeModified), time.Time(o4m.Meta.TimeModified))
	o3m.Meta.TimeModified = o4m.Meta.TimeModified
	assert.Equal(o3m, *o4m)

	// convert model with nil values to object
	o3m.Meta = nil
	o3m.Capabilities = nil
	o4d := Role{}
	o4d.FromModel(&o3m)
	assert.NotEmpty(o4d.ObjMeta.MetaObjID)
	assert.EqualValues(1, o4d.ObjMeta.MetaVersion)
	assert.NotNil(o4d.Capabilities)
	assert.Len(o4d.Capabilities, 0)

	// convert empty object to model
	o5d := Role{}
	o5m := o5d.ToModel()
	assert.NotNil(o5d.Capabilities)
	assert.NotNil(o5m.Capabilities)
}

func TestConvertServicePlan(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{
		"Meta": "ObjMeta",
	}
	cmpObjToModel(t, ServicePlan{}, models.ServicePlan{}, fMap)
	assert.True(true)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := ServicePlan{
		ObjMeta: ObjMeta{
			MetaObjID:        uuid.NewV4().String(),
			MetaVersion:      1,
			MetaTimeCreated:  now,
			MetaTimeModified: now,
		},
		Name:                "spName1",
		Description:         "service plan object",
		SourceServicePlanID: "sourceServicePlanID",
		State:               "UNPUBLISHED",
		IoProfile: IoProfile{
			IoPattern:    IoPattern{Name: "sequential", MinSizeBytesAvg: 16384, MaxSizeBytesAvg: 262144},
			ReadWriteMix: ReadWriteMix{Name: "read-write", MinReadPercent: 30, MaxReadPercent: 70},
		},
		ProvisioningUnit:       ProvisioningUnit{IOPS: 0, Throughput: 4000000},
		VolumeSeriesMinMaxSize: VolumeSeriesMinMaxSize{MinSizeBytes: 100000000, MaxSizeBytes: 10000000000},
		Accounts:               ObjIDList{"account1", "account2"},
		Slos: RestrictedStringValueMap{
			"slo1": {Kind: "INT", Value: "1", Immutable: true},
			"slo2": {Kind: "STRING", Value: "a string", Immutable: false},
		},
		// No Tags specified in datastore object
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := ServicePlan{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			Meta: &models.ObjMeta{
				ID:          o1m.Meta.ID,
				Version:     o1m.Meta.Version,
				ObjType:     "ServicePlan",
				TimeCreated: o1m.Meta.TimeCreated,
			},
			SourceServicePlanID:    o1m.SourceServicePlanID,
			State:                  o1m.State,
			IoProfile:              o1m.IoProfile,
			ProvisioningUnit:       o1m.ProvisioningUnit,
			VolumeSeriesMinMaxSize: o1m.VolumeSeriesMinMaxSize,
		},
		ServicePlanMutable: models.ServicePlanMutable{
			Name:        o1m.Name,
			Description: o1m.Description,
			Accounts:    []models.ObjIDMutable{"account1", "account2"},
			Slos: models.SloListMutable{
				"slo1": {
					ValueType:                 models.ValueType{Kind: "INT", Value: "1"},
					RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: false},
				},
				"slo2": {
					ValueType:                 models.ValueType{Kind: "STRING", Value: "a string"},
					RestrictedValueTypeAllOf1: models.RestrictedValueTypeAllOf1{Immutable: false},
				},
			},
			Tags: models.ObjTags{"tag1", "tag2", "tag3"},
		},
	}
	// convert to datastore
	o3d := ServicePlan{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.NotEqual(time.Time(o3m.Meta.TimeModified), time.Time(o4m.Meta.TimeModified))
	o3m.Meta.TimeModified = o4m.Meta.TimeModified
	assert.Equal(o3m, *o4m)

	// Test model with nil pointer types conversion to datastore
	o3m.Tags = nil
	o3m.Slos = nil
	o3m.Accounts = nil
	o4d := ServicePlan{}
	o4d.FromModel(&o3m)
	assert.Len(o4d.Tags, 0)
	assert.Len(o4d.Slos, 0)
	assert.Len(o4d.Accounts, 0)

	// convert empty ServicePlan to model
	o5d := ServicePlan{}
	o5m := o5d.ToModel()
	assert.NotNil(o5m.Tags)
	assert.NotNil(o5m.Accounts)
	assert.NotNil(o5m.Slos)
	assert.NotNil(o5m.IoProfile)
	assert.NotNil(o5m.ProvisioningUnit)
	assert.NotNil(o5m.VolumeSeriesMinMaxSize)
}

func TestConvertServicePlanAllocation(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{
		"Meta": "ObjMeta",
	}
	cmpObjToModel(t, ServicePlanAllocation{}, models.ServicePlanAllocation{}, fMap)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := ServicePlanAllocation{
		ObjMeta: ObjMeta{
			MetaObjID:        uuid.NewV4().String(),
			MetaVersion:      1,
			MetaTimeCreated:  now,
			MetaTimeModified: now,
		},
		CspDomainID:             "cspDomainId",
		ReservableCapacityBytes: 1000000000,
		ChargedCostPerGiB:       0.25,
		ClusterDescriptor: StringValueMap{
			"k8sPvcYaml": ValueType{Kind: "STRING", Value: "some string"},
		},
		ServicePlanAllocationCreateOnce: ServicePlanAllocationCreateOnce{
			AccountID:           "ownerAccountId",
			AuthorizedAccountID: "authorizedAccountId",
			ClusterID:           "clusterId",
			ServicePlanID:       "servicePlanId",
		},
		ServicePlanAllocationCreateMutable: ServicePlanAllocationCreateMutable{
			Messages: []TimestampedString{
				TimestampedString{Message: "message1", Time: now},
				TimestampedString{Message: "message2", Time: now},
				TimestampedString{Message: "message3", Time: now},
			},
			ProvisioningHints: StringValueMap{
				"attr1": ValueType{Kind: "INT", Value: "1"},
				"attr2": ValueType{Kind: "STRING", Value: "a string"},
				"attr3": ValueType{Kind: "SECRET", Value: "a secret string"},
			},
			ReservationState: "UNKNOWN",
			StorageFormula:   "Formula1",
			StorageReservations: StorageTypeReservationMap{
				"prov1": {NumMirrors: 1, SizeBytes: 1},
				"prov2": {NumMirrors: 2, SizeBytes: 22},
				"prov3": {NumMirrors: 3, SizeBytes: 333},
			},
			TotalCapacityBytes: 200000000000,
			// No Tags specified in datastore object
			// No SystemTags specified in datastore object
		},
	}
	assert.True(o1d.Tags == nil)
	assert.True(o1d.SystemTags == nil)
	// convert to model
	o1m := o1d.ToModel()
	assert.Equal("ServicePlanAllocation", o1m.Meta.ObjType, "ObjType is set")
	assert.Len(o1d.Tags, 0) // created as a side effect
	assert.Len(o1m.Tags, 0)
	assert.Len(o1d.SystemTags, 0) // created as a side effect
	assert.Len(o1m.SystemTags, 0)
	// convert back
	o2d := ServicePlanAllocation{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.ServicePlanAllocation{
		ServicePlanAllocationAllOf0: models.ServicePlanAllocationAllOf0{
			Meta: &models.ObjMeta{
				ID:          o1m.Meta.ID,
				Version:     o1m.Meta.Version,
				ObjType:     "ServicePlanAllocation",
				TimeCreated: o1m.Meta.TimeCreated,
			},
			CspDomainID: o1m.CspDomainID,
		},
		ServicePlanAllocationCreateOnce: models.ServicePlanAllocationCreateOnce{
			AccountID:           "ownerAccountId",
			AuthorizedAccountID: "authorizedAccountId",
			ClusterID:           "clusterId",
			ServicePlanID:       "servicePlanId",
		},
		ServicePlanAllocationMutable: models.ServicePlanAllocationMutable{
			ServicePlanAllocationMutableAllOf0: models.ServicePlanAllocationMutableAllOf0{
				ReservableCapacityBytes: o1m.ReservableCapacityBytes,
				ChargedCostPerGiB:       o1m.ChargedCostPerGiB,
				ClusterDescriptor:       o1m.ClusterDescriptor,
			},
			ServicePlanAllocationCreateMutable: models.ServicePlanAllocationCreateMutable{
				Messages:            o1m.Messages,
				ProvisioningHints:   o1m.ProvisioningHints,
				ReservationState:    "UNKNOWN",
				StorageFormula:      "Formula1",
				StorageReservations: o1m.StorageReservations,
				SystemTags:          []string{"stag1", "stag2"},
				Tags:                models.ObjTags{"tag1", "tag2", "tag3"},
				TotalCapacityBytes:  o1m.TotalCapacityBytes,
			},
		},
	}
	// convert to datastore
	o3d := ServicePlanAllocation{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.NotEqual(time.Time(o3m.Meta.TimeModified), time.Time(o4m.Meta.TimeModified))
	o3m.Meta.TimeModified = o4m.Meta.TimeModified
	assert.Equal(o3m, *o4m)

	// Test model with nil pointer types conversion to datastore
	o3m.Meta = nil
	o3m.Tags = nil
	o3m.SystemTags = nil
	o3m.ProvisioningHints = nil
	o3m.StorageReservations = nil
	o3m.ReservableCapacityBytes = nil
	o3m.TotalCapacityBytes = nil
	o3m.ClusterDescriptor = nil
	o4d := ServicePlanAllocation{}
	o4d.FromModel(&o3m)
	assert.Len(o4d.Tags, 0)
	assert.Len(o4d.SystemTags, 0, "SystemTags always set in datastore")
	assert.Len(o4d.StorageReservations, 0)
	assert.Len(o4d.StorageReservations, 0)
	assert.Zero(o4d.ReservableCapacityBytes)
	assert.Zero(o4d.TotalCapacityBytes)
	assert.Len(o4d.ClusterDescriptor, 0)

	// convert empty ServicePlanAllocation to model
	o5d := ServicePlanAllocation{}
	o5m := o5d.ToModel()
	assert.NotNil(o5m.ProvisioningHints)
	assert.NotNil(o5m.StorageReservations)
	assert.NotNil(o5m.Tags)
	assert.NotNil(o5m.ReservableCapacityBytes)
	assert.NotNil(o5m.ChargedCostPerGiB)
	assert.NotNil(o5m.TotalCapacityBytes)
	assert.Equal(centrald.DefaultServicePlanAllocationReservationState, o5m.ReservationState)
	assert.NotNil(o5m.Messages)
	assert.NotNil(o5m.SystemTags)
	assert.Nil(o5m.ClusterDescriptor)
}

func TestConvertSnapshot(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{
		"Meta": "ObjMeta",
	}
	cmpObjToModel(t, Snapshot{}, models.Snapshot{}, fMap)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := Snapshot{
		ObjMeta: ObjMeta{
			MetaObjID:        uuid.NewV4().String(),
			MetaVersion:      1,
			MetaTimeCreated:  now,
			MetaTimeModified: now,
		},
		AccountID:          "account1",
		ConsistencyGroupID: "cg1",
		PitIdentifier:      "pit1",
		ProtectionDomainID: "pd-1",
		SizeBytes:          12345,
		SnapIdentifier:     "HEAD",
		SnapTime:           now,
		TenantAccountID:    "tenacc1",
		VolumeSeriesID:     "vs1",
		DeleteAfterTime:    now.Add(90 * 24 * time.Hour),
		Locations: SnapshotLocationMap{
			"csp-1": SnapshotLocation{CreationTime: now, CspDomainID: "csp-1"},
			"csp-2": SnapshotLocation{CreationTime: now.Add(-2 * time.Hour), CspDomainID: "csp-2"},
			"csp-3": SnapshotLocation{CreationTime: now.Add(-3 * time.Hour), CspDomainID: "csp-3"},
		},
		Messages: []TimestampedString{
			{Message: "message1", Time: now},
			{Message: "message2", Time: now},
			{Message: "message3", Time: now},
		},
		// No Tags specified in datastore object
		// No SystemTags specified in datastore object
	}

	// convert to model
	o1m := o1d.ToModel()
	assert.Equal("Snapshot", o1m.Meta.ObjType, "ObjType is set")
	assert.Len(o1d.SystemTags, 0) // created as a side effect
	assert.NotNil(o1d.SystemTags)
	assert.Len(o1m.SystemTags, 0)
	assert.Len(o1d.Tags, 0) // created as a side effect
	assert.NotNil(o1d.Tags)
	assert.Len(o1m.Tags, 0)
	// convert back
	o2d := Snapshot{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.Snapshot{
		SnapshotAllOf0: models.SnapshotAllOf0{
			Meta: &models.ObjMeta{
				ID:          o1m.Meta.ID,
				Version:     o1m.Meta.Version,
				ObjType:     "Snapshot",
				TimeCreated: o1m.Meta.TimeCreated,
			},
			AccountID:          "account1",
			ConsistencyGroupID: "cg1",
			PitIdentifier:      "pit1",
			ProtectionDomainID: "pd-1",
			SizeBytes:          12345,
			SnapIdentifier:     "HEAD",
			SnapTime:           strfmt.DateTime(now),
			TenantAccountID:    "tenacc1",
			VolumeSeriesID:     "vs1",
		},
		SnapshotMutable: models.SnapshotMutable{
			DeleteAfterTime: strfmt.DateTime(now.Add(90 * 24 * time.Hour)),
			Locations: map[string]models.SnapshotLocation{
				"csp-1": models.SnapshotLocation{CreationTime: strfmt.DateTime(now), CspDomainID: "csp-1"},
				"csp-2": models.SnapshotLocation{CreationTime: strfmt.DateTime(now.Add(-2 * time.Hour)), CspDomainID: "csp-2"},
				"csp-3": models.SnapshotLocation{CreationTime: strfmt.DateTime(now.Add(-3 * time.Hour)), CspDomainID: "csp-3"},
			},
			Messages:   []*models.TimestampedString{&models.TimestampedString{Message: "msg"}},
			SystemTags: models.ObjTags{"stag1", "stag2"},
			Tags:       models.ObjTags{"tag1", "tag2"},
		},
	}

	// convert to datastore
	o3d := Snapshot{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.NotEqual(time.Time(o3m.Meta.TimeModified), time.Time(o4m.Meta.TimeModified))
	o3m.Meta.TimeModified = o4m.Meta.TimeModified
	assert.Equal(o3m, *o4m)

	// Test model without arrays conversion to datastore
	o3m.Meta = nil
	o3m.Locations = nil
	o3m.Messages = nil
	o3m.SystemTags = nil
	o3m.Tags = nil
	o4d := Snapshot{}
	o4d.FromModel(&o3m)
	assert.NotEmpty(o4d.ObjMeta.MetaObjID)
	assert.EqualValues(1, o4d.ObjMeta.MetaVersion)
	assert.Len(o4d.Locations, 0, "Locations set in datastore")
	assert.Len(o4d.Messages, 0, "Messages set in datastore")
	assert.Len(o4d.SystemTags, 0, "SystemTags always set in datastore")
	assert.Len(o4d.Tags, 0, "Tags always set in datastore")

	// convert empty VolumeSeriesRequest to model
	o5d := Snapshot{}
	o5m := o5d.ToModel()
	assert.NotNil(o5m.Locations)
	assert.NotNil(o5m.Messages)
	assert.NotNil(o5m.SystemTags)
	assert.NotNil(o5m.Tags)
}

func TestConvertStorage(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{
		"Meta": "ObjMeta",
	}
	cmpObjToModel(t, Storage{}, models.Storage{}, fMap)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := Storage{
		ObjMeta: ObjMeta{
			MetaObjID:        uuid.NewV4().String(),
			MetaVersion:      1,
			MetaTimeCreated:  now,
			MetaTimeModified: now,
		},
		AccountID:       "accountId",
		TenantAccountID: "tenantId",
		CspDomainID:     "cspDomainId",
		CspStorageType:  "Amazon gp2",
		StorageAccessibility: StorageAccessibility{
			AccessibilityScope:      "CSPDOMAIN",
			AccessibilityScopeObjID: "cspDomainId",
		},
		StorageIdentifier: "xxx",
		PoolID:            "poolID",
		ClusterID:         "xyz",
		SizeBytes:         int64(units.Tebibyte),
		AvailableBytes:    int64(units.Gibibyte),
		ParcelSizeBytes:   int64(units.Mebibyte),
		TotalParcelCount:  int64(987654),
		StorageState: StorageState{
			AttachedNodeDevice: "abc",
			AttachedNodeID:     "def",
			AttachmentState:    "ATTACHED",
			DeviceState:        "FORMATTING",
			MediaState:         "UNFORMATTED",
			Messages: []TimestampedString{
				TimestampedString{Message: "message1", Time: now},
				TimestampedString{Message: "message2", Time: now},
				TimestampedString{Message: "message3", Time: now},
			},
			ProvisionedState: "PROVISIONED",
		},
		ShareableStorage: true,
		// No Tags specified in datastore object
	}
	assert.True(o1d.SystemTags == nil)
	// convert to model
	o1m := o1d.ToModel()
	assert.Equal("Storage", o1m.Meta.ObjType, "ObjType is set")
	assert.Len(o1d.SystemTags, 0) // created as a side effect
	assert.Len(o1m.SystemTags, 0)
	// convert back
	o2d := Storage{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)
	assert.True(o2d.SystemTags != nil)

	// Test model conversion to and from datastore
	o3m := models.Storage{
		StorageAllOf0: models.StorageAllOf0{
			Meta: &models.ObjMeta{
				ID:          o1m.Meta.ID,
				Version:     o1m.Meta.Version,
				ObjType:     "Storage",
				TimeCreated: o1m.Meta.TimeCreated,
			},
			AccountID:       o1m.AccountID,
			TenantAccountID: o1m.TenantAccountID,
			CspDomainID:     o1m.CspDomainID,
			CspStorageType:  o1m.CspStorageType,
			SizeBytes:       o1m.SizeBytes,
			StorageAccessibility: &models.StorageAccessibility{
				StorageAccessibilityMutable: models.StorageAccessibilityMutable{
					AccessibilityScope:      o1m.StorageAccessibility.AccessibilityScope,
					AccessibilityScopeObjID: o1m.StorageAccessibility.AccessibilityScopeObjID,
				},
			},
			PoolID:    o1m.PoolID,
			ClusterID: o1m.ClusterID,
		},
		StorageMutable: models.StorageMutable{
			AvailableBytes:    o1m.AvailableBytes,
			ParcelSizeBytes:   o1m.ParcelSizeBytes,
			TotalParcelCount:  o1m.TotalParcelCount,
			ShareableStorage:  o1m.ShareableStorage,
			StorageIdentifier: o1m.StorageIdentifier,
			StorageState:      o1m.StorageState,
			SystemTags:        []string{"stag1", "stag2"},
		},
	}
	// convert to datastore
	o3d := Storage{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.NotEqual(time.Time(o3m.Meta.TimeModified), time.Time(o4m.Meta.TimeModified))
	o3m.Meta.TimeModified = o4m.Meta.TimeModified
	assert.Equal(o3m, *o4m)

	// Test model with nil pointer types conversion to datastore
	o3m.StorageAccessibility = nil
	o3m.StorageState = nil
	o3m.AvailableBytes = nil
	o3m.SizeBytes = nil
	o3m.ParcelSizeBytes = nil
	o3m.TotalParcelCount = nil
	o3m.SystemTags = nil
	o4d := Storage{}
	o4d.FromModel(&o3m)
	assert.NotNil(o4d.StorageAccessibility)
	assert.NotNil(o4d.StorageState)
	assert.Equal(centrald.DefaultStorageAttachmentState, o4d.StorageState.AttachmentState)
	assert.Equal(centrald.DefaultStorageDeviceState, o4d.StorageState.DeviceState)
	assert.Equal(centrald.DefaultStorageMediaState, o4d.StorageState.MediaState)
	assert.Equal(centrald.DefaultStorageProvisionedState, o4d.StorageState.ProvisionedState)
	assert.Zero(o4d.AvailableBytes)
	assert.Zero(o4d.SizeBytes)
	assert.Zero(o4d.ParcelSizeBytes)
	assert.Zero(o4d.TotalParcelCount)
	assert.Len(o4d.SystemTags, 0, "SystemTags always set in datastore")

	// convert empty Storage to model
	o5d := Storage{}
	o5m := o5d.ToModel()
	assert.NotNil(o5m.AvailableBytes)
	assert.NotNil(o5m.ParcelSizeBytes)
	assert.NotNil(o5m.TotalParcelCount)
	assert.NotNil(o5m.SizeBytes)
	assert.NotNil(o5m.StorageAccessibility)
	assert.NotNil(o5m.StorageState)
	assert.NotNil(o5m.StorageState.Messages)
	assert.NotNil(o5m.SystemTags)
}

func TestConvertStorageCostMap(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, StorageCost{}, models.StorageCost{}, fMap)

	// Test datastore conversion to and from model
	o1d := StorageCostMap{
		"st1": StorageCost{CostPerGiB: 0.003},
		"st2": StorageCost{CostPerGiB: 23.9},
		"st3": StorageCost{},
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := StorageCostMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// retest with empty map
	o1d = StorageCostMap{}
	assert.Len(o1d, 0)
	o1m = o1d.ToModel()
	assert.Len(o1m, 0)
	o2d = StorageCostMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)
	assert.Len(o2d, 0)

	// Test model conversion to and from datastore
	o3m := map[string]models.StorageCost{
		"st1": models.StorageCost{CostPerGiB: 0.003},
		"st2": models.StorageCost{CostPerGiB: 23.9},
		"st3": models.StorageCost{},
	}
	// convert to datastore
	o3d := StorageCostMap{}
	o3d.FromModel(o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o4m, o3m)
}
func TestConvertStorageRequest(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{
		"Meta":       "ObjMeta",
		"Terminated": "", // not persisted
	}
	cmpObjToModel(t, StorageRequest{}, models.StorageRequest{}, fMap)

	// Test datastore conversion to and from model
	now := time.Now()
	pTime := now.Add(time.Hour)
	o1d := StorageRequest{
		ObjMeta: ObjMeta{
			MetaObjID:        uuid.NewV4().String(),
			MetaVersion:      1,
			MetaTimeCreated:  now,
			MetaTimeModified: now,
		},
		RequestedOperations: []string{"PROVISION", "RELEASE"},
		CspStorageType:      "Amazon gp2",
		AccountID:           "aid1",
		TenantAccountID:     "tid1",
		NodeID:              "nodeID",
		ReattachNodeID:      "reattachNodeID",
		CspDomainID:         "cspDomainID",
		ClusterID:           "clusterID",
		MinSizeBytes:        int64(units.Gibibyte),
		CompleteByTime:      pTime,
		StorageID:           "storageID",
		StorageRequestState: centrald.DefaultStorageRequestState,
		PoolID:              "poolID",
		ParcelSizeBytes:     1000,
		ShareableStorage:    true,
		RequestMessages: []TimestampedString{
			{Message: "message1", Time: now},
			{Message: "message2", Time: now},
			{Message: "message3", Time: now},
		},
		VolumeSeriesRequestClaims: VsrClaim{
			RemainingBytes: 999,
			Claims: map[string]VsrClaimElement{
				"vsrID1": VsrClaimElement{SizeBytes: 1, Annotation: "S-0-1"},
				"vsrID2": VsrClaimElement{SizeBytes: 22, Annotation: "S-0-2"},
				"vsrID3": VsrClaimElement{SizeBytes: 333, Annotation: "S-0-3"},
			},
		},
		// No Tags specified in datastore object
	}
	assert.True(o1d.SystemTags == nil)
	// convert to model
	o1m := o1d.ToModel()
	assert.Equal("StorageRequest", o1m.Meta.ObjType, "ObjType is set")
	assert.Len(o1d.SystemTags, 0) // created as a side effect
	assert.Len(o1m.SystemTags, 0)
	// convert back
	o2d := StorageRequest{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)
	assert.True(o2d.SystemTags != nil)

	// Test model conversion to and from datastore
	o3m := models.StorageRequest{
		StorageRequestAllOf0: models.StorageRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:          o1m.Meta.ID,
				Version:     o1m.Meta.Version,
				ObjType:     "StorageRequest",
				TimeCreated: o1m.Meta.TimeCreated,
			},
		},
		StorageRequestMutable: models.StorageRequestMutable{
			StorageRequestMutableAllOf0: models.StorageRequestMutableAllOf0{
				RequestMessages:     o1m.RequestMessages,
				StorageRequestState: "SUCCEEDED",
			},
			StorageRequestCreateMutable: models.StorageRequestCreateMutable{
				NodeID:     o1m.NodeID,
				StorageID:  o1m.StorageID,
				SystemTags: []string{"stag1", "stag2"},
				VolumeSeriesRequestClaims: &models.VsrClaim{
					RemainingBytes: swag.Int64(9999),
					Claims: map[string]models.VsrClaimElement{
						"vsrID1": models.VsrClaimElement{SizeBytes: swag.Int64(1), Annotation: "S-1-1"},
						"vsrID2": models.VsrClaimElement{SizeBytes: swag.Int64(22), Annotation: "S-1-2"},
						"vsrID3": models.VsrClaimElement{SizeBytes: swag.Int64(333), Annotation: "S-1-3"},
					},
				},
			},
		},
		StorageRequestCreateOnce: models.StorageRequestCreateOnce{
			AccountID:           o1m.AccountID,
			TenantAccountID:     o1m.TenantAccountID,
			CspDomainID:         o1m.CspDomainID,
			ClusterID:           o1m.ClusterID,
			PoolID:              o1m.PoolID,
			ReattachNodeID:      o1m.ReattachNodeID,
			CspStorageType:      o1m.CspStorageType,
			MinSizeBytes:        o1m.MinSizeBytes,
			CompleteByTime:      o1m.CompleteByTime,
			RequestedOperations: o1m.RequestedOperations,
			ParcelSizeBytes:     o1m.ParcelSizeBytes,
			ShareableStorage:    o1m.ShareableStorage,
		},
	}
	// convert to datastore
	o3d := StorageRequest{}
	o3d.FromModel(&o3m)
	assert.True(o3d.Terminated)
	// convert back
	o4m := o3d.ToModel()
	assert.NotEqual(time.Time(o3m.Meta.TimeModified), time.Time(o4m.Meta.TimeModified))
	o3m.Meta.TimeModified = o4m.Meta.TimeModified
	assert.Equal(o3m, *o4m)

	// Test model with nil pointer types conversion to datastore, failed state
	o3m.Meta = nil
	o3m.RequestMessages = nil
	o3m.MinSizeBytes = nil
	o3m.RequestedOperations = nil
	o3m.SystemTags = nil
	o3m.StorageRequestState = "FAILED"
	o4d := StorageRequest{}
	o4d.FromModel(&o3m)
	assert.Len(o4d.RequestMessages, 0)
	assert.Zero(o4d.MinSizeBytes)
	assert.Len(o4d.RequestedOperations, 0)
	assert.Len(o4d.SystemTags, 0, "SystemTags always set in datastore")
	assert.True(o3d.Terminated)

	// convert empty StorageRequest to model
	o5d := StorageRequest{}
	o5m := o5d.ToModel()
	assert.NotNil(o5m.RequestMessages)
	assert.NotNil(o5m.MinSizeBytes)
	assert.NotNil(o5m.RequestedOperations)
	assert.NotNil(o5m.SystemTags)
}

func TestConvertSystem(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{
		"Meta":    "ObjMeta",
		"Service": "", // not persisted
	}
	cmpObjToModel(t, System{}, models.System{}, fMap)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := System{
		ObjMeta: ObjMeta{
			MetaObjID:        uuid.NewV4().String(),
			MetaVersion:      1,
			MetaTimeCreated:  now,
			MetaTimeModified: now,
		},
		Name:        "name",
		Description: "description",
		SnapshotManagementPolicy: SnapshotManagementPolicy{
			RetentionDurationSeconds: 1,
		},
		VsrManagementPolicy: VsrManagementPolicy{
			RetentionDurationSeconds: 1,
		},
		ClusterUsagePolicy: ClusterUsagePolicy{
			AccountSecretScope: "CSPDOMAIN",
		},
		UserPasswordPolicy: UserPasswordPolicy{
			MinLength: 5,
		},
		// No SystemTags specified in datastore object
	}
	assert.Nil(o1d.SystemTags)
	// convert to model
	o1m := o1d.ToModel()
	assert.Equal("System", o1m.Meta.ObjType, "ObjType is set")
	assert.Len(o1d.SystemTags, 0) // created as a side effect
	assert.NotNil(o1d.SystemTags)
	assert.Len(o1m.SystemTags, 0)
	// convert back
	o2d := System{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)
	assert.NotNil(o2d.SystemTags)

	// Test model conversion to and from datastore
	o3m := models.System{
		SystemAllOf0: models.SystemAllOf0{
			Meta: &models.ObjMeta{
				ID:          o1m.Meta.ID,
				Version:     o1m.Meta.Version,
				ObjType:     "System",
				TimeCreated: o1m.Meta.TimeCreated,
			},
			Service: &models.NuvoService{
				NuvoServiceAllOf0: models.NuvoServiceAllOf0{
					ServiceAttributes: map[string]models.ValueType{},
					Messages:          []*models.TimestampedString{},
				},
			},
		},
		SystemMutable: models.SystemMutable{
			Name:        o1m.Name,
			Description: o1m.Description,
			SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
				RetentionDurationSeconds: swag.Int32(2),
				NoDelete:                 true,
			},
			VsrManagementPolicy: &models.VsrManagementPolicy{
				RetentionDurationSeconds: swag.Int32(5),
				NoDelete:                 true,
			},
			ClusterUsagePolicy: &models.ClusterUsagePolicy{
				AccountSecretScope: "CLUSTER",
			},
			UserPasswordPolicy: &models.SystemMutableUserPasswordPolicy{
				MinLength: 5,
			},
			SystemTags: []string{"stag1", "stag2"},
		},
	}
	// convert to datastore
	o3d := System{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.NotEqual(time.Time(o3m.Meta.TimeModified), time.Time(o4m.Meta.TimeModified))
	o3m.Meta.TimeModified = o4m.Meta.TimeModified
	assert.Equal(o3m, *o4m)

	// convert model with nil values to object
	o3m.Meta = nil
	o3m.SystemTags = nil
	o4d := System{}
	o4d.FromModel(&o3m)
	assert.NotEmpty(o4d.ObjMeta.MetaObjID)
	assert.EqualValues(1, o4d.ObjMeta.MetaVersion)
	assert.NotNil(o4d.SystemTags, "SystemTags always set in datastore")

	// Test model conversion to and from datastore
	o5m := models.System{
		SystemAllOf0: models.SystemAllOf0{
			Meta: &models.ObjMeta{
				ID:          o1m.Meta.ID,
				Version:     o1m.Meta.Version,
				ObjType:     "System",
				TimeCreated: o1m.Meta.TimeCreated,
			},
			Service: &models.NuvoService{
				NuvoServiceAllOf0: models.NuvoServiceAllOf0{
					ServiceAttributes: map[string]models.ValueType{},
				},
			},
		},
		SystemMutable: models.SystemMutable{
			Name:        o1m.Name,
			Description: o1m.Description,
			SnapshotManagementPolicy: &models.SnapshotManagementPolicy{
				RetentionDurationSeconds: swag.Int32(2),
				NoDelete:                 true,
			},
			VsrManagementPolicy: &models.VsrManagementPolicy{
				RetentionDurationSeconds: swag.Int32(0),
				NoDelete:                 false,
			},
			SystemTags: []string{"stag1", "stag2"},
		},
	}
	// convert to datastore
	o5d := System{}
	o5d.FromModel(&o5m)
	// convert back
	o6m := o5d.ToModel()
	assert.NotNil(o6m.SnapshotManagementPolicy)
	assert.NotNil(o6m.VsrManagementPolicy)
	assert.NotNil(o6m.UserPasswordPolicy)
}

func TestConvertUser(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{
		"Meta":         "ObjMeta",
		"AccountRoles": "",
	}
	cmpObjToModel(t, User{}, models.User{}, fMap)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := User{
		ObjMeta: ObjMeta{
			MetaObjID:        uuid.NewV4().String(),
			MetaVersion:      1,
			MetaTimeCreated:  now,
			MetaTimeModified: now,
		},
		AuthIdentifier: "ann.user@nuvoloso.com",
		Disabled:       true,
		Password:       "pw",
		Profile: StringValueMap{
			"userName": ValueType{Kind: "STRING", Value: "ann user"},
			"attr2":    ValueType{Kind: "INT", Value: "1"},
		},
	}
	// convert to model
	o1m := o1d.ToModel()
	assert.Equal("User", o1m.Meta.ObjType, "ObjType is set")
	assert.Empty(o1m.Password)
	o1m.Password = "pw"
	// convert back
	o2d := User{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.User{
		UserAllOf0: models.UserAllOf0{
			Meta: &models.ObjMeta{
				ID:          o1m.Meta.ID,
				Version:     o1m.Meta.Version,
				ObjType:     "User",
				TimeCreated: o1m.Meta.TimeCreated,
			},
		},
		UserMutable: models.UserMutable{
			AuthIdentifier: o1m.AuthIdentifier,
			Disabled:       o1m.Disabled,
			Profile:        o1m.Profile,
		},
	}
	// convert to datastore
	o3d := User{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.NotEqual(time.Time(o3m.Meta.TimeModified), time.Time(o4m.Meta.TimeModified))
	o3m.Meta.TimeModified = o4m.Meta.TimeModified
	assert.Equal(o3m, *o4m)

	// convert model with nil values to object
	o3m.Meta = nil
	o3m.Profile = nil
	o4d := User{}
	o4d.FromModel(&o3m)
	assert.NotEmpty(o4d.ObjMeta.MetaObjID)
	assert.EqualValues(1, o4d.ObjMeta.MetaVersion)
	assert.NotNil(o4d.Profile)
	assert.Len(o4d.Profile, 0)

	// convert empty object to model
	o5d := User{}
	o5m := o5d.ToModel()
	assert.NotNil(o5d.Profile)
	assert.NotNil(o5m.Profile)
}

func TestConvertVolumeSeries(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{
		"Meta": "ObjMeta",
	}
	cmpObjToModel(t, VolumeSeries{}, models.VolumeSeries{}, fMap)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := VolumeSeries{
		ObjMeta: ObjMeta{
			MetaObjID:        uuid.NewV4().String(),
			MetaVersion:      1,
			MetaTimeCreated:  now,
			MetaTimeModified: now,
		},
		VolumeSeriesCreateOnceFields: VolumeSeriesCreateOnceFields{
			AccountID:       "account1",
			TenantAccountID: "tenant1",
		},
		VolumeSeriesCreateMutableFields: VolumeSeriesCreateMutableFields{
			Name:               "vs1",
			Description:        "vs1 object",
			SizeBytes:          int64(units.Gibibyte),
			SpaAdditionalBytes: int64(units.Mebibyte),
			ConsistencyGroupID: "con",
			ServicePlanID:      "servicePlan1",
			ClusterDescriptor: StringValueMap{
				"k8sPvcYaml": ValueType{Kind: "STRING", Value: "some string"},
			},
			// No Tags specified in datastore object
		},
		VolumeSeriesState:       "BOUND",
		BoundClusterID:          "cl1",
		BoundCspDomainID:        "csp1",
		ConfiguredNodeID:        "node1",
		RootStorageID:           "st1",
		NuvoVolumeIdentifier:    "nuvoVol1",
		RootParcelUUID:          "uuid",
		ServicePlanAllocationID: "ServicePlanAllocation1",
		Messages: []TimestampedString{
			{Message: "message1", Time: now},
			{Message: "message2", Time: now},
			{Message: "message3", Time: now},
		},
		CacheAllocations: CacheAllocationMap{
			"nodeId1": {
				AllocatedSizeBytes: 1,
				RequestedSizeBytes: 2,
			},
			"nodeId2": {
				AllocatedSizeBytes: 22,
				RequestedSizeBytes: 22,
			},
			"nodeId3": {
				AllocatedSizeBytes: 333,
				RequestedSizeBytes: 4444,
			},
		},
		CapacityAllocations: CapacityAllocationMap{
			"prov1": CapacityAllocation{ReservedBytes: 22, ConsumedBytes: 11},
			"prov2": CapacityAllocation{ReservedBytes: 333, ConsumedBytes: 222},
			"prov3": CapacityAllocation{ReservedBytes: 4444, ConsumedBytes: 3333},
		},
		StorageParcels: ParcelAllocationMap{
			"storageId1": ParcelAllocation{SizeBytes: 1},
			"storageId2": ParcelAllocation{SizeBytes: 22},
			"storageId3": ParcelAllocation{SizeBytes: 333},
		},
		Mounts: []Mount{
			{
				SnapIdentifier:    "HEAD",
				MountedNodeDevice: "/dev/abc",
				MountedNodeID:     "node1",
				MountMode:         "READ_WRITE",
				MountState:        "MOUNTED",
				MountTime:         now,
			}, {
				SnapIdentifier:    "2",
				MountedNodeDevice: "/dev/def",
				MountedNodeID:     "node2",
				MountMode:         "READ_ONLY",
				MountState:        "MOUNTED",
				MountTime:         now,
			},
		},
	}

	assert.Nil(o1d.Tags)
	assert.Nil(o1d.SystemTags)
	// convert to model
	o1m := o1d.ToModel()
	assert.Equal("VolumeSeries", o1m.Meta.ObjType, "ObjType is set")
	assert.Len(o1d.Tags, 0) // created as a side effect
	assert.Len(o1m.Tags, 0)
	assert.Len(o1d.SystemTags, 0) // created as a side effect
	assert.Len(o1m.SystemTags, 0)
	assert.NotNil(o1m.LifecycleManagementData)
	// convert back
	o2d := VolumeSeries{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)
	assert.True(o2d.Tags != nil)
	assert.True(o2d.SystemTags != nil)

	// Test model conversion to and from datastore
	o3m := models.VolumeSeries{
		VolumeSeriesAllOf0: models.VolumeSeriesAllOf0{
			Meta: &models.ObjMeta{
				ID:          o1m.Meta.ID,
				Version:     o1m.Meta.Version,
				ObjType:     "VolumeSeries",
				TimeCreated: o1m.Meta.TimeCreated,
			},
			RootParcelUUID: "uuid",
		},
		VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
			AccountID:       "account1",
			TenantAccountID: "tenant1",
		},
		VolumeSeriesMutable: models.VolumeSeriesMutable{
			VolumeSeriesMutableAllOf0: models.VolumeSeriesMutableAllOf0{
				BoundClusterID:          o1m.BoundClusterID,
				BoundCspDomainID:        o1m.BoundCspDomainID,
				ConfiguredNodeID:        o1m.ConfiguredNodeID,
				RootStorageID:           o1m.RootStorageID,
				NuvoVolumeIdentifier:    o1m.NuvoVolumeIdentifier,
				ServicePlanAllocationID: o1m.ServicePlanAllocationID,
				Messages:                o1m.Messages,
				VolumeSeriesState:       o1m.VolumeSeriesState,
				CacheAllocations: map[string]models.CacheAllocation{
					"nodeId1": {
						AllocatedSizeBytes: swag.Int64(1),
						RequestedSizeBytes: swag.Int64(2),
					},
					"nodeId2": {
						AllocatedSizeBytes: swag.Int64(22),
						RequestedSizeBytes: swag.Int64(22),
					},
					"nodeId3": {
						AllocatedSizeBytes: swag.Int64(333),
						RequestedSizeBytes: swag.Int64(444),
					},
				},
				CapacityAllocations: map[string]models.CapacityAllocation{
					"prov1": {ConsumedBytes: swag.Int64(111), ReservedBytes: swag.Int64(1111)},
					"prov2": {ConsumedBytes: swag.Int64(222), ReservedBytes: swag.Int64(2222)},
					"prov3": {ConsumedBytes: swag.Int64(333), ReservedBytes: swag.Int64(333)},
				},
				StorageParcels: map[string]models.ParcelAllocation{
					"storageId1": models.ParcelAllocation{SizeBytes: swag.Int64(1)},
					"storageId2": models.ParcelAllocation{SizeBytes: swag.Int64(22)},
					"storageId3": models.ParcelAllocation{SizeBytes: swag.Int64(333)},
				},
				Mounts: []*models.Mount{
					&models.Mount{
						SnapIdentifier:    "HEAD",
						MountedNodeDevice: "/dev/abc",
						MountedNodeID:     "node1",
						MountMode:         "READ_WRITE",
						MountState:        "MOUNTED",
						MountTime:         strfmt.DateTime(now),
					},
					&models.Mount{
						SnapIdentifier:    "2",
						MountedNodeDevice: "/dev/def",
						MountedNodeID:     "node2",
						MountMode:         "READ_ONLY",
						MountState:        "MOUNTED",
						MountTime:         strfmt.DateTime(now),
					},
				},
			},
			VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
				Name:               o1m.Name,
				Description:        o1m.Description,
				SizeBytes:          o1m.SizeBytes,
				SpaAdditionalBytes: o1m.SpaAdditionalBytes,
				ConsistencyGroupID: o1m.ConsistencyGroupID,
				ServicePlanID:      o1m.ServicePlanID,
				LifecycleManagementData: &models.LifecycleManagementData{
					NextSnapshotTime: strfmt.DateTime(now),
				},
				Tags:              []string{"tag1", "tag2"},
				SystemTags:        []string{"stag1", "stag2"},
				ClusterDescriptor: o1m.ClusterDescriptor,
			},
		},
	}
	// convert to datastore
	o3d := VolumeSeries{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.NotEqual(time.Time(o3m.Meta.TimeModified), time.Time(o4m.Meta.TimeModified))
	o3m.Meta.TimeModified = o4m.Meta.TimeModified
	assert.Equal(o3m, *o4m)

	// Test model without Tags, et al conversion to datastore
	o3m.Messages = nil
	o3m.CacheAllocations = nil
	o3m.CapacityAllocations = nil
	o3m.StorageParcels = nil
	o3m.Mounts = nil
	o3m.Tags = nil
	o3m.SystemTags = nil
	o4d := VolumeSeries{}
	o4d.FromModel(&o3m)
	assert.Len(o4d.Messages, 0, "Messages set in datastore")
	assert.Len(o4d.CacheAllocations, 0, "CacheAllocations set in datastore")
	assert.Len(o4d.CapacityAllocations, 0, "CapacityAllocations set in datastore")
	assert.Len(o4d.StorageParcels, 0, "StorageParcels set in datastore")
	assert.Len(o4d.Mounts, 0, "Mounts set in datastore")
	assert.Len(o4d.Tags, 0, "Tags always set in datastore")
	assert.Len(o4d.SystemTags, 0, "SystemTags always set in datastore")
}

func TestConvertVolumeSeriesRequest(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{
		"Meta":       "ObjMeta",
		"Terminated": "", // not persisted
	}
	cmpObjToModel(t, VolumeSeriesRequest{}, models.VolumeSeriesRequest{}, fMap)

	// Test datastore conversion to and from model
	now := time.Now()
	o1d := VolumeSeriesRequest{
		ObjMeta: ObjMeta{
			MetaObjID:        uuid.NewV4().String(),
			MetaVersion:      1,
			MetaTimeCreated:  now,
			MetaTimeModified: now,
		},
		Creator: Identity{
			AccountID:       "aid1",
			TenantAccountID: "tid1",
			UserID:          "uid1",
		},
		RequestedOperations:     []string{},
		CancelRequested:         false,
		PlanOnly:                false,
		ApplicationGroupIds:     ObjIDList{"ag1", "ag2"},
		ClusterID:               "cl1",
		ConsistencyGroupID:      "cg1",
		NodeID:                  "i-1",
		MountedNodeDevice:       "/dev/abc",
		ProtectionDomainID:      "pd-1",
		VolumeSeriesID:          "vs1",
		SnapIdentifier:          "HEAD",
		SyncCoordinatorID:       "sci-1",
		SyncPeers:               map[string]SyncPeer{"p1": SyncPeer{ID: "id1", State: "S1"}},
		ServicePlanAllocationID: "spa1",
		CompleteByTime:          now,
		ServicePlanAllocationCreateSpec: ServicePlanAllocationCreateSpec{
			ServicePlanAllocationCreateOnce: ServicePlanAllocationCreateOnce{
				AccountID:           "ownerAccountId",
				AuthorizedAccountID: "authorizedAccountId",
				ClusterID:           "clusterId",
				ServicePlanID:       "servicePlanId",
			},
			ServicePlanAllocationCreateMutable: ServicePlanAllocationCreateMutable{
				Messages: []TimestampedString{
					TimestampedString{Message: "message1", Time: now},
					TimestampedString{Message: "message2", Time: now},
					TimestampedString{Message: "message3", Time: now},
				},
				ProvisioningHints: StringValueMap{
					"attr1": ValueType{Kind: "INT", Value: "1"},
					"attr2": ValueType{Kind: "STRING", Value: "a string"},
					"attr3": ValueType{Kind: "SECRET", Value: "a secret string"},
				},
				ReservationState: "UNKNOWN",
				StorageFormula:   "Formula1",
				StorageReservations: StorageTypeReservationMap{
					"prov1": {SizeBytes: 1},
					"prov2": {SizeBytes: 22},
					"prov3": {SizeBytes: 333},
				},
				TotalCapacityBytes: 200000000000,
				SystemTags:         []string{"stag1"},
			},
		},
		VolumeSeriesCreateSpec: VolumeSeriesCreateSpec{
			VolumeSeriesCreateOnceFields: VolumeSeriesCreateOnceFields{
				AccountID:       "account1",
				TenantAccountID: "tenant1",
			},
			VolumeSeriesCreateMutableFields: VolumeSeriesCreateMutableFields{
				Name:               "vs1",
				Description:        "vs1 object",
				SizeBytes:          int64(units.Gibibyte),
				SpaAdditionalBytes: int64(units.Mebibyte),
				ConsistencyGroupID: "con",
				ServicePlanID:      "servicePlan1",
			},
		},
		Snapshot: SnapshotData{
			PitIdentifier: "pit0",
			SizeBytes:     1000,
		},
		SnapshotID: "snapshot-1",
		Progress: Progress{
			OffsetBytes:      1000,
			PercentComplete:  32,
			Timestamp:        time.Now().Round(0),
			TotalBytes:       2000,
			TransferredBytes: 500,
		},
		StorageFormula: "C12H22O11",
		CapacityReservationPlan: CapacityReservationPlan{
			StorageTypeReservations: StorageTypeReservationMap{
				"storageType1": {NumMirrors: 1, SizeBytes: 1},
				"storageType2": {NumMirrors: 2, SizeBytes: 22},
			},
		},
		CapacityReservationResult: CapacityReservationResult{
			CurrentReservations: PoolReservationMap{
				"poolId1": {SizeBytes: 1},
				"poolId2": {SizeBytes: 22},
				"poolId3": {SizeBytes: 333},
			},
			DesiredReservations: PoolReservationMap{
				"poolId1": {SizeBytes: 10},
				"poolId2": {SizeBytes: 220},
				"poolId3": {SizeBytes: 3330},
			},
		},
		StoragePlan: StoragePlan{
			LayoutAlgorithm: "linear",
			PlacementHints: StringValueMap{
				"tbd":        {Kind: "STRING", Value: "string1"},
				"notdefined": {Kind: "STRING", Value: "string2"},
			},
			StorageElements: []StorageElement{
				{
					Intent:    "DATA",
					SizeBytes: 10101010,
					StorageParcels: StorageParcelMap{
						"storageId1": StorageParcelElement{SizeBytes: 1},
						"storageId2": StorageParcelElement{SizeBytes: 22},
						"storageId3": StorageParcelElement{SizeBytes: 333},
					},
					PoolID: "provider1",
				}, {
					Intent:    "DATA",
					SizeBytes: 10101010,
					StorageParcels: StorageParcelMap{
						"storageId4": StorageParcelElement{SizeBytes: 1},
						"storageId5": StorageParcelElement{SizeBytes: 22},
						"storageId6": StorageParcelElement{SizeBytes: 333},
					},
					PoolID: "provider2",
				},
			},
		},
		VolumeSeriesRequestState: "BINDING",
		RequestMessages: []TimestampedString{
			{Message: "message1", Time: now},
			{Message: "message2", Time: now},
			{Message: "message3", Time: now},
		},
		FsType:     "ext4",
		DriverType: "flex",
		TargetPath: "targetPath",
		ReadOnly:   true,
		// No Tags specified in datastore object
	}

	assert.True(o1d.VolumeSeriesCreateSpec.Tags == nil)
	assert.True(o1d.VolumeSeriesCreateSpec.SystemTags == nil)
	assert.True(o1d.SystemTags == nil)
	// convert to model
	o1m := o1d.ToModel()
	assert.Equal("VolumeSeriesRequest", o1m.Meta.ObjType, "ObjType is set")
	assert.Len(o1d.VolumeSeriesCreateSpec.Tags, 0) // created as a side effect
	assert.Len(o1m.VolumeSeriesCreateSpec.Tags, 0)
	assert.Len(o1d.VolumeSeriesCreateSpec.SystemTags, 0) // created as a side effect
	assert.Len(o1m.VolumeSeriesCreateSpec.SystemTags, 0)
	assert.Len(o1d.SystemTags, 0) // created as a side effect
	assert.Len(o1m.SystemTags, 0)
	assert.NotNil(o1m.LifecycleManagementData)
	// convert back
	o2d := VolumeSeriesRequest{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)
	assert.True(o2d.VolumeSeriesCreateSpec.Tags != nil)
	assert.True(o2d.VolumeSeriesCreateSpec.SystemTags != nil)
	assert.True(o2d.SystemTags != nil)

	// Test model conversion to and from datastore
	o3m := models.VolumeSeriesRequest{
		VolumeSeriesRequestAllOf0: models.VolumeSeriesRequestAllOf0{
			Meta: &models.ObjMeta{
				ID:          o1m.Meta.ID,
				Version:     o1m.Meta.Version,
				ObjType:     "VolumeSeriesRequest",
				TimeCreated: o1m.Meta.TimeCreated,
			},
			CancelRequested: true,
		},
		VolumeSeriesRequestCreateOnce: models.VolumeSeriesRequestCreateOnce{
			ApplicationGroupIds: []models.ObjIDMutable{"ag1", "ag2"},
			ClusterID:           "cl1",
			CompleteByTime:      strfmt.DateTime(now),
			Creator: &models.Identity{
				AccountID:       "aid1",
				TenantAccountID: "tid1",
				UserID:          "uid1",
			},
			PlanOnly:            swag.Bool(true),
			ProtectionDomainID:  "pd-1",
			SnapIdentifier:      "HEAD",
			RequestedOperations: []string{"CREATE", "BIND"},
			SnapshotID:          "snapshot-2",
			ServicePlanAllocationCreateSpec: &models.ServicePlanAllocationCreateArgs{
				ServicePlanAllocationCreateOnce: models.ServicePlanAllocationCreateOnce{
					AccountID:           "ownerAccountId",
					AuthorizedAccountID: "authorizedAccountId",
					ClusterID:           "clusterId",
					ServicePlanID:       "servicePlanId",
				},
				ServicePlanAllocationCreateMutable: models.ServicePlanAllocationCreateMutable{
					Messages:            o1m.ServicePlanAllocationCreateSpec.Messages,
					ProvisioningHints:   o1m.ServicePlanAllocationCreateSpec.ProvisioningHints,
					ReservationState:    "UNKNOWN",
					StorageFormula:      "Formula1",
					StorageReservations: o1m.ServicePlanAllocationCreateSpec.StorageReservations,
					SystemTags:          []string{"stag1", "stag2"},
					Tags:                models.ObjTags{"tag1", "tag2", "tag3"},
					TotalCapacityBytes:  o1m.ServicePlanAllocationCreateSpec.TotalCapacityBytes,
				},
			},
			VolumeSeriesCreateSpec: &models.VolumeSeriesCreateArgs{
				VolumeSeriesCreateOnce: models.VolumeSeriesCreateOnce{
					AccountID:       "account1",
					TenantAccountID: "tenant1",
				},
				VolumeSeriesCreateMutable: models.VolumeSeriesCreateMutable{
					Name:        o1m.VolumeSeriesCreateSpec.Name,
					Description: o1m.VolumeSeriesCreateSpec.Description,
					// SizeBytes and SpaAdditionalBytes unset
					ConsistencyGroupID: o1m.VolumeSeriesCreateSpec.ConsistencyGroupID,
					ServicePlanID:      o1m.VolumeSeriesCreateSpec.ServicePlanID,
					Tags:               []string{"tag1", "tag2"},
					SystemTags:         []string{"vs-stag1"},
					LifecycleManagementData: &models.LifecycleManagementData{
						NextSnapshotTime: strfmt.DateTime(now),
					},
				},
			},
			FsType:     "ext4",
			DriverType: "flex",
			TargetPath: "targetPath",
		},
		VolumeSeriesRequestMutable: models.VolumeSeriesRequestMutable{
			VolumeSeriesRequestMutableAllOf0: models.VolumeSeriesRequestMutableAllOf0{
				CapacityReservationPlan: &models.CapacityReservationPlan{
					StorageTypeReservations: map[string]models.StorageTypeReservation{
						"storageType1": {NumMirrors: 1, SizeBytes: swag.Int64(1)},
						"storageType2": {NumMirrors: 2, SizeBytes: swag.Int64(22)},
						"storageType3": {NumMirrors: 3, SizeBytes: swag.Int64(333)},
					},
				},
				CapacityReservationResult: &models.CapacityReservationResult{
					CurrentReservations: map[string]models.PoolReservation{
						"poolId1": {SizeBytes: swag.Int64(1)},
						"poolId2": {SizeBytes: swag.Int64(22)},
						"poolId3": {SizeBytes: swag.Int64(333)},
					},
					DesiredReservations: map[string]models.PoolReservation{
						"poolId1": {SizeBytes: swag.Int64(10)},
						"poolId2": {SizeBytes: swag.Int64(220)},
						"poolId3": {SizeBytes: swag.Int64(3330)},
					},
				},
				MountedNodeDevice: "/dev/abc",
				RequestMessages:   o1m.RequestMessages,
				LifecycleManagementData: &models.LifecycleManagementData{
					NextSnapshotTime: strfmt.DateTime(now),
				},
				StorageFormula: "formula E",
				StoragePlan: &models.StoragePlan{
					LayoutAlgorithm: "TBD",
					PlacementHints: map[string]models.ValueType{
						"property1": {Kind: "STRING", Value: "string3"},
						"property2": {Kind: "STRING", Value: "string4"},
					},
					StorageElements: []*models.StoragePlanStorageElement{
						{
							Intent:    "DATA",
							SizeBytes: swag.Int64(1010101),
							StorageParcels: map[string]models.StorageParcelElement{
								"storageId1": models.StorageParcelElement{SizeBytes: swag.Int64(1), ProvMinSizeBytes: swag.Int64(2), ProvParcelSizeBytes: swag.Int64(3), ProvRemainingSizeBytes: swag.Int64(4)},
								"storageId2": models.StorageParcelElement{SizeBytes: swag.Int64(22), ProvMinSizeBytes: swag.Int64(2), ProvParcelSizeBytes: swag.Int64(3), ProvRemainingSizeBytes: swag.Int64(4)},
								"storageId3": models.StorageParcelElement{SizeBytes: swag.Int64(333), ProvMinSizeBytes: swag.Int64(2), ProvParcelSizeBytes: swag.Int64(3), ProvRemainingSizeBytes: swag.Int64(4)},
							},
							PoolID: "provider1",
						}, {
							Intent:    "DATA",
							SizeBytes: swag.Int64(1010101),
							StorageParcels: map[string]models.StorageParcelElement{
								"storageId4": models.StorageParcelElement{SizeBytes: swag.Int64(1), ProvMinSizeBytes: swag.Int64(2), ProvParcelSizeBytes: swag.Int64(3), ProvRemainingSizeBytes: swag.Int64(4)},
								"storageId5": models.StorageParcelElement{SizeBytes: swag.Int64(22), ProvMinSizeBytes: swag.Int64(2), ProvParcelSizeBytes: swag.Int64(3), ProvRemainingSizeBytes: swag.Int64(4)},
								"storageId6": models.StorageParcelElement{SizeBytes: swag.Int64(333), ProvMinSizeBytes: swag.Int64(2), ProvParcelSizeBytes: swag.Int64(3), ProvRemainingSizeBytes: swag.Int64(4)},
							},
							PoolID: "provider2",
						},
					},
				},
				VolumeSeriesRequestState: "CANCELED",
			},
			VolumeSeriesRequestCreateMutable: models.VolumeSeriesRequestCreateMutable{
				NodeID:                  "node1",
				ServicePlanAllocationID: "spa1",
				VolumeSeriesID:          "vs1",
				Snapshot: &models.SnapshotData{
					PitIdentifier: "pit0",
					SizeBytes:     swag.Int64(1000),
				},
				SystemTags: []string{"stag1", "stag2", "stag3"},
			},
		},
	}
	// convert to datastore
	o3d := VolumeSeriesRequest{}
	assert.Nil(o3m.SyncPeers)
	assert.Nil(o3m.VolumeSeriesCreateSpec.ClusterDescriptor)
	o3d.FromModel(&o3m)
	assert.NotNil(o3d.SyncPeers)
	assert.True(o3d.Terminated)
	// convert back
	o4m := o3d.ToModel()
	assert.NotEqual(time.Time(o3m.Meta.TimeModified), time.Time(o4m.Meta.TimeModified))
	o3m.Meta.TimeModified = o4m.Meta.TimeModified
	assert.NotNil(o4m.SyncPeers)
	o3m.SyncPeers = make(map[string]models.SyncPeer)
	assert.Nil(o4m.VolumeSeriesCreateSpec.SizeBytes)          // specificity preserved
	assert.Nil(o4m.VolumeSeriesCreateSpec.SpaAdditionalBytes) // specificity preserved
	o3m.VolumeSeriesCreateSpec.SpaAdditionalBytes = o4m.VolumeSeriesCreateSpec.SpaAdditionalBytes
	assert.Nil(o4m.VolumeSeriesCreateSpec.ClusterDescriptor)
	o3m.VolumeSeriesCreateSpec.ClusterDescriptor = nil
	assert.Equal(o3m, *o4m)

	// Test model without Tags, et al conversion to datastore (also succeeded)
	o3m.Meta = nil
	o3m.ApplicationGroupIds = nil
	o3m.Creator = nil
	o3m.RequestMessages = nil
	o3m.CapacityReservationPlan = nil
	o3m.CapacityReservationResult = nil
	o3m.StoragePlan = nil
	o3m.VolumeSeriesCreateSpec = nil
	o3m.Snapshot = nil
	o3m.LifecycleManagementData = nil
	o3m.SystemTags = nil
	o3m.VolumeSeriesRequestState = "SUCCEEDED"
	o4d := VolumeSeriesRequest{}
	o4d.FromModel(&o3m)
	assert.NotEmpty(o4d.ObjMeta.MetaObjID)
	assert.EqualValues(1, o4d.ObjMeta.MetaVersion)
	assert.NotNil(o4d.ApplicationGroupIds, "ApplicationGroupIds set in datastore")
	assert.NotNil(o4d.Creator, "Creator set in datastore")
	assert.NotNil(o4d.CapacityReservationPlan.StorageTypeReservations, "CapacityReservationPlan set in datastore")
	assert.NotNil(o4d.CapacityReservationResult.CurrentReservations, "CapacityReservationResult.CurrentReservations set in datastore")
	assert.NotNil(o4d.CapacityReservationResult.DesiredReservations, "CapacityReservationResult.DesiredReservations set in datastore")
	assert.NotNil(o4d.StoragePlan.PlacementHints, "StoragePlan set in datastore")
	assert.NotNil(o4d.StoragePlan.StorageElements, "StoragePlan set in datastore")
	assert.NotNil(o4d.VolumeSeriesCreateSpec.Tags, "Tags always set in datastore")
	assert.NotNil(o4d.VolumeSeriesCreateSpec.SystemTags, "SystemTags always set in datastore")
	assert.NotNil(o4d.SystemTags, "SystemTags always set in datastore")
	assert.True(o4d.Terminated)

	// convert empty VolumeSeriesRequest to model
	o5d := VolumeSeriesRequest{}
	o5m := o5d.ToModel()
	assert.NotNil(o5m.ApplicationGroupIds)
	assert.NotNil(o5m.Creator)
	assert.NotNil(o5m.RequestMessages)
	assert.NotNil(o5m.RequestedOperations)
	assert.NotNil(o5m.CapacityReservationPlan)
	assert.NotNil(o5m.StoragePlan)
	if assert.NotNil(o5m.VolumeSeriesCreateSpec) {
		assert.NotNil(o5m.VolumeSeriesCreateSpec.SizeBytes)
		assert.NotNil(o5m.VolumeSeriesCreateSpec.SpaAdditionalBytes)
		assert.NotNil(o5m.VolumeSeriesCreateSpec.Tags)
		assert.NotNil(o5m.VolumeSeriesCreateSpec.SystemTags)
	}
	assert.NotNil(o5m.SystemTags)
	assert.NotNil(o5m.Snapshot)
	assert.NotNil(o5m.Snapshot.Locations)
}

func TestVsrClaimMap(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, VsrClaimElement{}, models.VsrClaimElement{}, fMap)

	// Test datastore conversion to and from model
	o1d := VsrClaimMap{
		"vsrID1": VsrClaimElement{SizeBytes: 1, Annotation: "S-0-1"},
		"vsrID2": VsrClaimElement{SizeBytes: 22, Annotation: "S-0-2"},
		"vsrID3": VsrClaimElement{SizeBytes: 333, Annotation: "S-0-3"},
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := VsrClaimMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// retest with empty map
	o1d = VsrClaimMap{}
	assert.Len(o1d, 0)
	o1m = o1d.ToModel()
	assert.Len(o1m, 0)
	o2d = VsrClaimMap{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)
	assert.Len(o2d, 0)

	// Test model conversion to and from datastore
	o3m := map[string]models.VsrClaimElement{
		"vsrID1": models.VsrClaimElement{SizeBytes: swag.Int64(1), Annotation: "S-1-1"},
		"vsrID2": models.VsrClaimElement{SizeBytes: swag.Int64(22), Annotation: "S-1-2"},
		"vsrID3": models.VsrClaimElement{SizeBytes: swag.Int64(333), Annotation: "S-1-3"},
	}
	// convert to datastore
	o3d := VsrClaimMap{}
	o3d.FromModel(o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o4m, o3m)
}

func TestVsrClaim(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, VsrClaim{}, models.VsrClaim{}, fMap)

	// Test datastore conversion to and from model
	o1d := VsrClaim{
		RemainingBytes: 999,
		Claims: map[string]VsrClaimElement{
			"vsrID1": VsrClaimElement{SizeBytes: 1, Annotation: "S-0-1"},
			"vsrID2": VsrClaimElement{SizeBytes: 22, Annotation: "S-0-2"},
			"vsrID3": VsrClaimElement{SizeBytes: 333, Annotation: "S-0-3"},
		},
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := VsrClaim{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore
	o3m := models.VsrClaim{
		RemainingBytes: swag.Int64(9999),
		Claims: map[string]models.VsrClaimElement{
			"vsrID1": models.VsrClaimElement{SizeBytes: swag.Int64(1), Annotation: "S-1-1"},
			"vsrID2": models.VsrClaimElement{SizeBytes: swag.Int64(22), Annotation: "S-1-2"},
			"vsrID3": models.VsrClaimElement{SizeBytes: swag.Int64(333), Annotation: "S-1-3"},
		},
	}
	// convert to datastore
	o3d := VsrClaim{}
	o3d.FromModel(&o3m)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(o3m, *o4m)
}

func TestVsrManagementPolicy(t *testing.T) {
	assert := assert.New(t)

	// Test that the object covers the model
	fMap := map[string]string{}
	cmpObjToModel(t, VsrManagementPolicy{}, models.VsrManagementPolicy{}, fMap)

	// ATYPICAL converter - do not copy

	// Test datastore conversion to and from model (existential property set)
	o1d := VsrManagementPolicy{
		NoDelete:                 true,
		RetentionDurationSeconds: 999999,
	}
	// convert to model
	o1m := o1d.ToModel()
	// convert back
	o2d := VsrManagementPolicy{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// retest with existential value set
	o1d = VsrManagementPolicy{Inherited: false}
	o1m = o1d.ToModel()
	o2d = VsrManagementPolicy{}
	o2d.FromModel(o1m)
	assert.Equal(o1d, o2d)

	// Test model conversion to and from datastore (existential value set)
	o3m := &models.VsrManagementPolicy{Inherited: false}
	// convert to datastore
	o3d := VsrManagementPolicy{}
	o3d.FromModel(o3m)
	assert.Equal(int32(0), o3d.RetentionDurationSeconds)
	// convert back
	o4m := o3d.ToModel()
	assert.Equal(int32(0), swag.Int32Value(o4m.RetentionDurationSeconds))

	// existential value not set => nil model
	o5d := VsrManagementPolicy{Inherited: true}
	o5m := o5d.ToModel()
	assert.Nil(o5m)

	// nil model => empty (except for Inherited)
	o6d := VsrManagementPolicy{}
	o6d.FromModel(nil)
	assert.Equal(VsrManagementPolicy{Inherited: true}, o6d)
}
