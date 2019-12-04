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


package gcsdk

import (
	"context"

	"cloud.google.com/go/storage"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// API is an interface over the GC SDK
type API interface {
	// Add methods as needed

	NewComputeService(ctx context.Context, opts ...option.ClientOption) (ComputeService, error)
	NewStorageClient(ctx context.Context, opts ...option.ClientOption) (StorageClient, error)
}

// ComputeService is an interface over compute.Service
type ComputeService interface {
	Disks() DisksService
	Instances() InstancesService
	ZoneOperations() ZoneOperationsService
	Zones() ZonesService
}

// DisksService is an interface over compute.DisksService
type DisksService interface {
	Delete(project string, zone string, disk string) DisksDeleteCall
	Get(project string, zone string, disk string) DisksGetCall
	Insert(project string, zone string, disk *compute.Disk) DisksInsertCall
	List(project string, zone string) DisksListCall
	SetLabels(project string, zone string, vid string, labelsRequest *compute.ZoneSetLabelsRequest) DisksSetLabelsCall
}

// DisksDeleteCall is an interface over compute.DisksDeleteCall
type DisksDeleteCall interface {
	Context(ctx context.Context) DisksDeleteCall
	Do(opts ...googleapi.CallOption) (*compute.Operation, error)
}

// DisksGetCall is an interface over compute.DisksGetCall
type DisksGetCall interface {
	Context(ctx context.Context) DisksGetCall
	Do(opts ...googleapi.CallOption) (*compute.Disk, error)
}

// DisksInsertCall is an interface over compute.DisksInsertCall
type DisksInsertCall interface {
	Context(ctx context.Context) DisksInsertCall
	Do(opts ...googleapi.CallOption) (*compute.Operation, error)
}

// DisksListCall is an interface over compute.DisksListCall
type DisksListCall interface {
	Filter(filter string) DisksListCall
	Pages(ctx context.Context, f func(*compute.DiskList) error) error
}

// DisksSetLabelsCall is an interface over compute.DisksSetLabelsCall
type DisksSetLabelsCall interface {
	Context(ctx context.Context) DisksSetLabelsCall
	Do(opts ...googleapi.CallOption) (*compute.Operation, error)
}

// InstancesService is an interface over compute.InstancesService
type InstancesService interface {
	AttachDisk(project string, zone string, instance string, attacheddisk *compute.AttachedDisk) InstancesAttachDiskCall
	DetachDisk(project string, zone string, instance string, deviceName string) InstancesDetachDiskCall
}

// InstancesAttachDiskCall is an interface over compute.InstancesAttachDiskCall
type InstancesAttachDiskCall interface {
	Context(ctx context.Context) InstancesAttachDiskCall
	Do(opts ...googleapi.CallOption) (*compute.Operation, error)
	ForceAttach(forceAttach bool) InstancesAttachDiskCall
}

// InstancesDetachDiskCall is an interface over compute.InstancesDetachDiskCall
type InstancesDetachDiskCall interface {
	Context(ctx context.Context) InstancesDetachDiskCall
	Do(opts ...googleapi.CallOption) (*compute.Operation, error)
}

// ZoneOperationsService is an interface over compute.ZoneOperationsService
type ZoneOperationsService interface {
	Get(project string, zone string, operation string) ZoneOperationsGetCall
}

// ZoneOperationsGetCall is an interface over compute.ZoneOperationsGetCall
type ZoneOperationsGetCall interface {
	Context(ctx context.Context) ZoneOperationsGetCall
	Do(opts ...googleapi.CallOption) (*compute.Operation, error)
}

// ZonesService is an interface over compute.ZonesService
type ZonesService interface {
	Get(project string, zone string) ZonesGetCall
}

// ZonesGetCall is an interface over compute.ZonesGetCall
type ZonesGetCall interface {
	Context(ctx context.Context) ZonesGetCall
	Do(opts ...googleapi.CallOption) (*compute.Zone, error)
}

// StorageClient is an interface over storage.Client
type StorageClient interface {
	Bucket(name string) BucketHandle
}

// BucketHandle is an interface over storage.BucketHandle
type BucketHandle interface {
	Create(ctx context.Context, projectID string, attrs *storage.BucketAttrs) (err error)
}

// SDK satisfies Client.API
type sdk struct{}

var _ = API(&sdk{})

type service struct {
	s *compute.Service
}

var _ = ComputeService(&service{})

type disksService struct {
	d *compute.DisksService
}

var _ = DisksService(&disksService{})

type disksDeleteCall struct {
	c *compute.DisksDeleteCall
}

var _ = DisksDeleteCall(&disksDeleteCall{})

type disksGetCall struct {
	c *compute.DisksGetCall
}

var _ = DisksGetCall(&disksGetCall{})

type disksInsertCall struct {
	c *compute.DisksInsertCall
}

var _ = DisksInsertCall(&disksInsertCall{})

type disksListCall struct {
	c *compute.DisksListCall
}

var _ = DisksListCall(&disksListCall{})

type disksSetLabelsCall struct {
	c *compute.DisksSetLabelsCall
}

var _ = DisksSetLabelsCall(&disksSetLabelsCall{})

type instancesService struct {
	i *compute.InstancesService
}

var _ = InstancesService(&instancesService{})

type instancesAttachDiskCall struct {
	c *compute.InstancesAttachDiskCall
}

var _ = InstancesAttachDiskCall(&instancesAttachDiskCall{})

type instancesDetachDiskCall struct {
	c *compute.InstancesDetachDiskCall
}

var _ = InstancesDetachDiskCall(&instancesDetachDiskCall{})

type zoneOperationsService struct {
	z *compute.ZoneOperationsService
}

var _ = ZoneOperationsService(&zoneOperationsService{})

type zoneOperationsGetCall struct {
	c *compute.ZoneOperationsGetCall
}

var _ = ZoneOperationsGetCall(&zoneOperationsGetCall{})

type zonesService struct {
	z *compute.ZonesService
}

var _ = ZonesService(&zonesService{})

type zonesGetCall struct {
	c *compute.ZonesGetCall
}

var _ = ZonesGetCall(&zonesGetCall{})

type storageClient struct {
	c *storage.Client
}

type bucketHandle struct {
	b *storage.BucketHandle
}

// New returns the real implementation of API over the GC SDK
func New() API {
	return &sdk{}
}

// NewComputeService wraps compute.NewService
func (c *sdk) NewComputeService(ctx context.Context, opts ...option.ClientOption) (ComputeService, error) {
	ret := &service{}
	var err error
	if ret.s, err = compute.NewService(ctx, opts...); err != nil {
		ret = nil
	}
	return ret, err
}

func (cs *service) Disks() DisksService {
	return &disksService{d: cs.s.Disks}
}

func (d *disksService) Delete(project string, zone string, disk string) DisksDeleteCall {
	return &disksDeleteCall{c: d.d.Delete(project, zone, disk)}
}

func (c *disksDeleteCall) Context(ctx context.Context) DisksDeleteCall {
	c.c.Context(ctx) // returns itself
	return c
}

func (c *disksDeleteCall) Do(opts ...googleapi.CallOption) (*compute.Operation, error) {
	return c.c.Do(opts...)
}

func (d *disksService) Get(project string, zone string, disk string) DisksGetCall {
	return &disksGetCall{c: d.d.Get(project, zone, disk)}
}

func (c *disksGetCall) Context(ctx context.Context) DisksGetCall {
	c.c.Context(ctx) // returns itself
	return c
}

func (c *disksGetCall) Do(opts ...googleapi.CallOption) (*compute.Disk, error) {
	return c.c.Do(opts...)
}

func (d *disksService) Insert(project string, zone string, disk *compute.Disk) DisksInsertCall {
	return &disksInsertCall{c: d.d.Insert(project, zone, disk)}
}

func (c *disksInsertCall) Context(ctx context.Context) DisksInsertCall {
	c.c.Context(ctx) // returns itself
	return c
}

func (c *disksInsertCall) Do(opts ...googleapi.CallOption) (*compute.Operation, error) {
	return c.c.Do(opts...)
}

func (d *disksService) List(project string, zone string) DisksListCall {
	return &disksListCall{c: d.d.List(project, zone)}
}

func (c *disksListCall) Filter(filter string) DisksListCall {
	c.c.Filter(filter) // returns itself
	return c
}

func (c *disksListCall) Pages(ctx context.Context, f func(*compute.DiskList) error) error {
	return c.c.Pages(ctx, f)
}

func (d *disksService) SetLabels(project string, zone string, vid string, labelsRequest *compute.ZoneSetLabelsRequest) DisksSetLabelsCall {
	return &disksSetLabelsCall{c: d.d.SetLabels(project, zone, vid, labelsRequest)}
}

func (c *disksSetLabelsCall) Context(ctx context.Context) DisksSetLabelsCall {
	c.c.Context(ctx) // returns itself
	return c
}

func (c *disksSetLabelsCall) Do(opts ...googleapi.CallOption) (*compute.Operation, error) {
	return c.c.Do(opts...)
}

func (cs *service) Instances() InstancesService {
	return &instancesService{i: cs.s.Instances}
}

func (i *instancesService) AttachDisk(project string, zone string, instance string, attacheddisk *compute.AttachedDisk) InstancesAttachDiskCall {
	return &instancesAttachDiskCall{c: i.i.AttachDisk(project, zone, instance, attacheddisk)}
}

func (c *instancesAttachDiskCall) Context(ctx context.Context) InstancesAttachDiskCall {
	c.c.Context(ctx) // returns itself
	return c
}

func (c *instancesAttachDiskCall) Do(opts ...googleapi.CallOption) (*compute.Operation, error) {
	return c.c.Do(opts...)
}

func (c *instancesAttachDiskCall) ForceAttach(forceAttach bool) InstancesAttachDiskCall {
	c.c.ForceAttach(forceAttach) // returns itself
	return c
}

func (i *instancesService) DetachDisk(project string, zone string, instance string, deviceName string) InstancesDetachDiskCall {
	return &instancesDetachDiskCall{c: i.i.DetachDisk(project, zone, instance, deviceName)}
}

func (c *instancesDetachDiskCall) Context(ctx context.Context) InstancesDetachDiskCall {
	c.c.Context(ctx) // returns itself
	return c
}

func (c *instancesDetachDiskCall) Do(opts ...googleapi.CallOption) (*compute.Operation, error) {
	return c.c.Do(opts...)
}

func (cs *service) ZoneOperations() ZoneOperationsService {
	return &zoneOperationsService{z: cs.s.ZoneOperations}
}

func (zs *zoneOperationsService) Get(project string, zone string, operation string) ZoneOperationsGetCall {
	return &zoneOperationsGetCall{c: zs.z.Get(project, zone, operation)}
}

func (c *zoneOperationsGetCall) Context(ctx context.Context) ZoneOperationsGetCall {
	c.c.Context(ctx) // returns itself
	return c
}

func (c *zoneOperationsGetCall) Do(opts ...googleapi.CallOption) (*compute.Operation, error) {
	return c.c.Do(opts...)
}

func (cs *service) Zones() ZonesService {
	return &zonesService{z: cs.s.Zones}
}

func (zs *zonesService) Get(project string, zone string) ZonesGetCall {
	return &zonesGetCall{c: zs.z.Get(project, zone)}
}

func (c *zonesGetCall) Context(ctx context.Context) ZonesGetCall {
	c.c.Context(ctx) // returns itself
	return c
}

func (c *zonesGetCall) Do(opts ...googleapi.CallOption) (*compute.Zone, error) {
	return c.c.Do(opts...)
}

// NewStorageClient wraps storage.NewClient
func (c *sdk) NewStorageClient(ctx context.Context, opts ...option.ClientOption) (StorageClient, error) {
	ret := &storageClient{}
	var err error
	if ret.c, err = storage.NewClient(ctx, opts...); err != nil {
		ret = nil
	}
	return ret, err
}

func (c *storageClient) Bucket(name string) BucketHandle {
	return &bucketHandle{b: c.c.Bucket(name)}
}

func (b *bucketHandle) Create(ctx context.Context, projectID string, attrs *storage.BucketAttrs) (err error) {
	return b.b.Create(ctx, projectID, attrs)
}
