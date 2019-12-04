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


package azuresdk

import (
	"context"

	// NOTE: Using "latest" for now
	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/storage/mgmt/storage"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure/auth"
)

// API is an interface over the AWS SDK
type API interface {
	// Add factory methods as needed
	NewClientCredentialsConfig(clientID string, clientSecret string, tenantID string) CCC
	NewAccountsClient(subscriptionID string, authorizer autorest.Authorizer) AC
	NewBlobContainersClient(subscriptionID string, authorizer autorest.Authorizer) BCC
	NewDisksClient(subscriptionID string, authorizer autorest.Authorizer) DC
	NewGroupsClient(subscriptionID string, authorizer autorest.Authorizer) GC
	NewVirtualMachinesClient(subscriptionID string, authorizer autorest.Authorizer) VMC
}

// CCC provides desired auth.ClientCredentialsConfig methods
type CCC interface {
	Authorizer() (autorest.Authorizer, error)
	ServicePrincipalToken() (SPT, error)
}

// SPT provides desired ServicePrincipalToken methods
type SPT interface {
	Refresh() error
}

// GC is an interface for the GroupsClient methods
type GC interface {
	Get(ctx context.Context, groupName string) (resources.Group, error)
}

// AC is an interface for service Account client methods
type AC interface {
	Get(ctx context.Context, groupName string, accountName string) (storage.Account, error)
	ListKeys(ctx context.Context, groupName string, accountName string) (storage.AccountListKeysResult, error)
}

// BCC is an interface for storage.BlobContainer client methods
type BCC interface {
	Create(ctx context.Context, groupName string, accountName string, containerName string, params storage.BlobContainer) (storage.BlobContainer, error)
}

// DC is an interface for compute.DisksClient methods
type DC interface {
	CreateOrUpdate(ctx context.Context, groupName, diskName string, disk compute.Disk) (DCCreateOrUpdateFuture, error)
	Delete(ctx context.Context, groupName, diskName string) (DCDisksDeleteFuture, error)
	Get(ctx context.Context, groupName, diskName string) (compute.Disk, error)
	ListByResourceGroupComplete(ctx context.Context, groupName string) (DCListIterator, error)
	Update(ctx context.Context, groupName, diskName string, disk compute.DiskUpdate) (DCDisksUpdateFuture, error)
}

// DCCreateOrUpdateFuture is returned by DC.CreateOrUpdate
type DCCreateOrUpdateFuture interface {
	WaitForCompletionRef(ctx context.Context, dc DC) error
	Result(dc DC) (compute.Disk, error)
}

// DCDisksDeleteFuture is returned by DC.Delete
type DCDisksDeleteFuture interface {
	WaitForCompletionRef(ctx context.Context, dc DC) error
	Result(dc DC) (autorest.Response, error)
}

// DCListIterator is returned by DC.ListComplete
type DCListIterator interface {
	NotDone() bool
	NextWithContext(ctx context.Context) error
	Value() compute.Disk
}

// DCDisksUpdateFuture is returned by DC.Update
type DCDisksUpdateFuture interface {
	WaitForCompletionRef(ctx context.Context, client DC) error
	Result(client DC) (compute.Disk, error)
}

// VMC is an interface for compute.VirtualMachinesClient methods
type VMC interface {
	CreateOrUpdate(ctx context.Context, groupName string, vmName string, vm compute.VirtualMachine) (VMCreateOrUpdateFuture, error)
	Get(ctx context.Context, groupName string, vmName string, expand compute.InstanceViewTypes) (compute.VirtualMachine, error)
}

// VMCreateOrUpdateFuture is returned by VMC.CreateOrUpdate
type VMCreateOrUpdateFuture interface {
	WaitForCompletionRef(ctx context.Context, client VMC) error
	Result(client VMC) (compute.VirtualMachine, error)
}

////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////

// SDK satisfies the API interface
type SDK struct{}

var _ = API(&SDK{})

// New returns the real implementation of the Azure API
func New() API {
	return &SDK{}
}

// NewClientCredentialsConfig wraps auth.NewClientCredentialsConfig and returns an interface
func (c *SDK) NewClientCredentialsConfig(clientID string, clientSecret string, tenantID string) CCC {
	rc := &ClientCredentialsConfig{}
	rc.CCC = auth.NewClientCredentialsConfig(clientID, clientSecret, tenantID)
	return rc
}

// NewAccountsClient wraps storage.NewAccountsClient
func (c *SDK) NewAccountsClient(subscriptionID string, authorizer autorest.Authorizer) AC {
	client := storage.NewAccountsClient(subscriptionID)
	client.Authorizer = authorizer
	return &AccountsClient{C: client}
}

// NewBlobContainersClient wraps storage.NewBlobContainersClient
func (c *SDK) NewBlobContainersClient(subscriptionID string, authorizer autorest.Authorizer) BCC {
	client := storage.NewBlobContainersClient(subscriptionID)
	client.Authorizer = authorizer
	return &BlobContainersClient{C: client}
}

// NewDisksClient wraps compute.NewDisksClient and returns an interface
func (c *SDK) NewDisksClient(subscriptionID string, authorizer autorest.Authorizer) DC {
	cl := compute.NewDisksClient(subscriptionID)
	cl.Authorizer = authorizer
	return &DisksClient{C: cl}
}

// NewGroupsClient wraps resources.NewGroupsClient
func (c *SDK) NewGroupsClient(subscriptionID string, authorizer autorest.Authorizer) GC {
	client := resources.NewGroupsClient(subscriptionID)
	client.Authorizer = authorizer
	return &GroupsClient{C: client}
}

// NewVirtualMachinesClient wraps compute.NewVirtualMachinesClient and returns an interface
func (c *SDK) NewVirtualMachinesClient(subscriptionID string, authorizer autorest.Authorizer) VMC {
	cl := compute.NewVirtualMachinesClient(subscriptionID)
	cl.Authorizer = authorizer
	return &VirtualMachinesClient{C: cl}
}

////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////

// ClientCredentialsConfig wraps its namesake
type ClientCredentialsConfig struct {
	CCC auth.ClientCredentialsConfig
}

// Authorizer invokes its namesake
func (o *ClientCredentialsConfig) Authorizer() (autorest.Authorizer, error) {
	return o.CCC.Authorizer()
}

// ServicePrincipalToken invokes its namesake
func (o *ClientCredentialsConfig) ServicePrincipalToken() (SPT, error) {
	spt, err := o.CCC.ServicePrincipalToken()
	if err == nil {
		return &ServicePrincipalToken{SPT: spt}, nil
	}
	return nil, err
}

////////////////////////////////////////////////////////////////////////////////////////////////////

// ServicePrincipalToken wraps the real ServicePrincipalToken
type ServicePrincipalToken struct {
	SPT *adal.ServicePrincipalToken
}

// Refresh invokes its namesake
func (o *ServicePrincipalToken) Refresh() error {
	return o.SPT.Refresh()
}

////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////

// GroupsClient satisfies the GC interface
type GroupsClient struct {
	C resources.GroupsClient
}

// Get implements the group Get call
func (g *GroupsClient) Get(ctx context.Context, resourceGroupName string) (resources.Group, error) {
	return g.C.Get(ctx, resourceGroupName)
}

////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////

// AccountsClient satisfies the AC interface
type AccountsClient struct {
	C storage.AccountsClient
}

// Get implements the account Get
func (a *AccountsClient) Get(ctx context.Context, groupName string, accountName string) (storage.Account, error) {
	return a.C.GetProperties(ctx, groupName, accountName, "")
}

// ListKeys implements the function of listing keys for a particular account
func (a *AccountsClient) ListKeys(ctx context.Context, groupName string, accountName string) (storage.AccountListKeysResult, error) {
	return a.C.ListKeys(ctx, groupName, accountName)
}

////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////

// BlobContainersClient satisfies the BCC interface
type BlobContainersClient struct {
	C storage.BlobContainersClient
}

// Create implements the BlobContainer Create
func (b *BlobContainersClient) Create(ctx context.Context, groupName string, accountName string, containerName string, params storage.BlobContainer) (storage.BlobContainer, error) {
	return b.C.Create(ctx, groupName, accountName, containerName, params)
}

////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////

// DisksClient satisfies the DC interface
type DisksClient struct {
	C compute.DisksClient
}

// CreateOrUpdate wraps compute.DisksClient.CreateOrUpdate
func (d *DisksClient) CreateOrUpdate(ctx context.Context, groupName, diskName string, disk compute.Disk) (DCCreateOrUpdateFuture, error) {
	future, err := d.C.CreateOrUpdate(ctx, groupName, diskName, disk)
	return &DisksCreateOrUpdateFuture{F: future}, err
}

// Delete wraps compute.Delete
func (d *DisksClient) Delete(ctx context.Context, groupName, diskName string) (DCDisksDeleteFuture, error) {
	future, err := d.C.Delete(ctx, groupName, diskName)
	return &DisksDeleteFuture{F: future}, err
}

// Get invokes its namesake
func (d *DisksClient) Get(ctx context.Context, groupName, diskName string) (compute.Disk, error) {
	return d.C.Get(ctx, groupName, diskName)
}

// ListByResourceGroupComplete invokes its namesake
func (d *DisksClient) ListByResourceGroupComplete(ctx context.Context, groupName string) (DCListIterator, error) {
	iter, err := d.C.ListByResourceGroupComplete(ctx, groupName)
	return &DiskListIterator{I: iter}, err
}

// Update invokes its namesake
func (d *DisksClient) Update(ctx context.Context, groupName, diskName string, disk compute.DiskUpdate) (DCDisksUpdateFuture, error) {
	future, err := d.C.Update(ctx, groupName, diskName, disk)
	return &DisksUpdateFuture{F: future}, err

}

////////////////////////////////////////////////////////////////////////////////////////////////////

// DisksCreateOrUpdateFuture satisfies the DCCreateOrUpdateFuture interface
type DisksCreateOrUpdateFuture struct {
	F compute.DisksCreateOrUpdateFuture
}

// WaitForCompletionRef satisfies the DCCreateOrUpdateFuture interface
func (f *DisksCreateOrUpdateFuture) WaitForCompletionRef(ctx context.Context, client DC) error {
	cl := client.(*DisksClient)
	return f.F.WaitForCompletionRef(ctx, cl.C.Client)
}

// Result satisfies the DCCreateOrUpdateFuture interface
func (f *DisksCreateOrUpdateFuture) Result(client DC) (compute.Disk, error) {
	cl := client.(*DisksClient)
	return f.F.Result(cl.C)
}

////////////////////////////////////////////////////////////////////////////////////////////////////

// DisksDeleteFuture satisfies the DCDisksDeleteFuture interface
type DisksDeleteFuture struct {
	F compute.DisksDeleteFuture
}

// WaitForCompletionRef satisfies the DCDisksDeleteFuture interface
func (f *DisksDeleteFuture) WaitForCompletionRef(ctx context.Context, client DC) error {
	cl := client.(*DisksClient)
	return f.F.WaitForCompletionRef(ctx, cl.C.Client)
}

// Result satisfies the DCDisksDeleteFuture interface
func (f *DisksDeleteFuture) Result(client DC) (autorest.Response, error) {
	cl := client.(*DisksClient)
	return f.F.Result(cl.C)
}

////////////////////////////////////////////////////////////////////////////////////////////////////

// DiskListIterator satisfies the DCListIterator interface
type DiskListIterator struct {
	I compute.DiskListIterator
}

// NotDone wraps its namesake
func (i *DiskListIterator) NotDone() bool {
	return i.I.NotDone()
}

// NextWithContext wraps its namesake
func (i *DiskListIterator) NextWithContext(ctx context.Context) error {
	return i.I.NextWithContext(ctx)
}

// Value wraps its namesake
func (i *DiskListIterator) Value() compute.Disk {
	return i.I.Value()
}

////////////////////////////////////////////////////////////////////////////////////////////////////

// DisksUpdateFuture satisfies the DCDisksUpdateFuture interface
type DisksUpdateFuture struct {
	F compute.DisksUpdateFuture
}

// WaitForCompletionRef satisfies the DCDisksUpdateFuture interface
func (f *DisksUpdateFuture) WaitForCompletionRef(ctx context.Context, client DC) error {
	cl := client.(*DisksClient)
	return f.F.WaitForCompletionRef(ctx, cl.C.Client)
}

// Result satisfies the DCDisksUpdateFuture interface
func (f *DisksUpdateFuture) Result(client DC) (compute.Disk, error) {
	cl := client.(*DisksClient)
	return f.F.Result(cl.C)
}

////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////

// VirtualMachinesClient satisfies the VMC interface
type VirtualMachinesClient struct {
	C compute.VirtualMachinesClient
}

// CreateOrUpdate wraps compute.VirtualMachinesClient.CreateOrUpdate
func (v *VirtualMachinesClient) CreateOrUpdate(ctx context.Context, groupName string, vmName string, vm compute.VirtualMachine) (VMCreateOrUpdateFuture, error) {
	future, err := v.C.CreateOrUpdate(ctx, groupName, vmName, vm)
	return &VirtualMachinesCreateOrUpdateFuture{F: future}, err
}

// Get wraps compute.VirtualMachinesClient.Get
func (v *VirtualMachinesClient) Get(ctx context.Context, groupName string, vmName string, expand compute.InstanceViewTypes) (compute.VirtualMachine, error) {
	return v.C.Get(ctx, groupName, vmName, expand)
}

////////////////////////////////////////////////////////////////////////////////////////////////////

// VirtualMachinesCreateOrUpdateFuture satisfies the VMCreateOrUpdateFuture interface
type VirtualMachinesCreateOrUpdateFuture struct {
	F compute.VirtualMachinesCreateOrUpdateFuture
}

// WaitForCompletionRef satisfies the VMCreateOrUpdateFuture interface
func (f *VirtualMachinesCreateOrUpdateFuture) WaitForCompletionRef(ctx context.Context, client VMC) error {
	cl := client.(*VirtualMachinesClient)
	return f.F.WaitForCompletionRef(ctx, cl.C.Client)
}

// Result satisfies the VMCreateOrUpdateFuture interface
func (f *VirtualMachinesCreateOrUpdateFuture) Result(client VMC) (compute.VirtualMachine, error) {
	cl := client.(*VirtualMachinesClient)
	return f.F.Result(cl.C)
}
