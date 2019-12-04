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


package azure

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2019-04-01/storage"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/azuresdk"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/go-openapi/swag"
)

// Client implements the DomainClient for the Azure domain
type Client struct {
	csp        *CSP
	Timeout    time.Duration
	domID      models.ObjID
	attrs      map[string]models.ValueType
	authorizer autorest.Authorizer
	client     azuresdk.API
	// cached clients
	cacheMux    sync.Mutex
	disksClient azuresdk.DC
	vmClient    azuresdk.VMC
}

// Client returns a DomainClient
func (c *CSP) Client(dObj *models.CSPDomain) (csp.DomainClient, error) {
	if string(dObj.CspDomainType) != CSPDomainType {
		return nil, fmt.Errorf("invalid DomainType")
	}
	id := models.ObjID("")
	if dObj.Meta != nil {
		id = dObj.Meta.ID
	}
	cl := &Client{
		csp:     c,
		attrs:   dObj.CspDomainAttributes,
		domID:   id,
		Timeout: time.Second * azureClientDefaultTimeoutSecs,
		client:  azuresdk.New(),
	}
	if err := c.clInit.clientInit(cl); err != nil {
		return nil, err
	}
	return cl, nil
}

// clientInit initializes an Azure Client
func (c *CSP) clientInit(cl *Client) error {
	ccc, err := c.AzGetClientCredentialsConfig(cl.attrs, true)
	if err == nil {
		var authorizer autorest.Authorizer
		authorizer, err = ccc.Authorizer()
		if err == nil {
			cl.authorizer = authorizer
		}
	}
	return err
}

// Type returns the CSPDomain type
func (cl *Client) Type() models.CspDomainTypeMutable {
	return cl.csp.Type()
}

// ID returns the domain object ID
func (cl *Client) ID() models.ObjID {
	return cl.domID
}

// SetTimeout sets the client timeout
func (cl *Client) SetTimeout(secs int) {
	if secs <= 0 {
		secs = azureClientDefaultTimeoutSecs
	}
	cl.Timeout = time.Duration(secs) * time.Second
}

// Validate determines if the credentials and zone are valid, returning nil on success
func (cl *Client) Validate(ctx context.Context) error {
	if err := azureAttributes.ValidateExpected(domainAttributesNames, cl.attrs, true, true); err != nil {
		return err
	}
	// TBD: need to validate zone and region
	return nil
}

// CreateProtectionStore creates an Azure BLOB bucket used for storing snapshots
func (cl *Client) CreateProtectionStore(ctx context.Context) (map[string]models.ValueType, error) {
	subID := cl.attrs[AttrSubscriptionID].Value
	resourceGroupName := cl.attrs[AttrResourceGroupName].Value
	if err := cl.validateResourceGroup(ctx, subID, resourceGroupName); err != nil {
		return nil, fmt.Errorf("invalid resource group: %w", err)
	}
	storageAccountName := cl.attrs[AttrStorageAccountName].Value
	if err := cl.validateStorageAccount(ctx, subID, resourceGroupName, storageAccountName); err != nil {
		return nil, fmt.Errorf("invalid storage account: %w", err)
	}
	blobContainerName := cl.attrs[AttrPStoreBlobContainerName].Value
	if err := cl.createBlobContainer(ctx, subID, resourceGroupName, storageAccountName, blobContainerName); err != nil {
		return nil, fmt.Errorf("blob container create error: %w", err)
	}
	return cl.attrs, nil
}

func (cl *Client) validateResourceGroup(ctx context.Context, subID string, groupName string) error {
	gc := cl.client.NewGroupsClient(subID, cl.authorizer)
	resourceGroup, err := gc.Get(ctx, groupName)
	if err != nil {
		return err
	}
	if val, ok := cl.attrs[AttrLocation]; ok && val.Value != "" && val.Value != swag.StringValue(resourceGroup.Location) {
		return fmt.Errorf("location mismatch")
	}
	cl.attrs[AttrLocation] = models.ValueType{Kind: "STRING", Value: swag.StringValue(resourceGroup.Location)}
	return err
}

func (cl *Client) validateStorageAccount(ctx context.Context, subID string, groupName string, storageAccountName string) error {
	ac := cl.client.NewAccountsClient(subID, cl.authorizer)
	_, err := ac.Get(ctx, groupName, storageAccountName)
	if err != nil {
		return err
	}
	// TBD: validate the account types (BLOB vs BLOCKBLOB)
	keyList, err := ac.ListKeys(ctx, groupName, storageAccountName)
	if err != nil {
		return err
	}
	if keyList.Keys == nil || len(*keyList.Keys) < 1 {
		return fmt.Errorf("Keys unavailable for Azure storage account %s", storageAccountName)
	}
	return nil
}

func (cl *Client) createBlobContainer(ctx context.Context, subID string, groupName string, accountName string, containerName string) error {
	var err error
	bc := cl.client.NewBlobContainersClient(subID, cl.authorizer)
	blobParams := storage.BlobContainer{}
	if _, err = bc.Create(ctx, groupName, accountName, containerName, blobParams); err != nil {
		return err
	}
	return nil
}

// newDisksClient returns a possibly cached disk client
func (cl *Client) newDisksClient() azuresdk.DC {
	cl.cacheMux.Lock()
	defer cl.cacheMux.Unlock()
	if cl.disksClient == nil {
		subID := cl.attrs[AttrSubscriptionID].Value
		cl.disksClient = cl.client.NewDisksClient(subID, cl.authorizer)
	}
	return cl.disksClient
}

// newVMClient returns a possibly cached vm client
func (cl *Client) newVMClient() azuresdk.VMC {
	cl.cacheMux.Lock()
	defer cl.cacheMux.Unlock()
	if cl.vmClient == nil {
		subID := cl.attrs[AttrSubscriptionID].Value
		cl.vmClient = cl.client.NewVirtualMachinesClient(subID, cl.authorizer)
	}
	return cl.vmClient
}

type retryableOperation func() (bool, error)

func (cl *Client) doWithRetry(op retryableOperation) error {
	var err error
	retry := true
	// TBD: implement some sort of backoff?
	for retry {
		retry, err = op()
	}
	return err
}

// getResourceGroup returns the resource group from provisioning
// attributes falling back to domain (client) attributes
func (cl *Client) getResourceGroup(provisioningAttributes map[string]models.ValueType) string {
	if len(provisioningAttributes) > 0 {
		if rgn, found := provisioningAttributes[ImdResourceGroupName]; found {
			return rgn.Value
		}
	}
	return cl.attrs[AttrResourceGroupName].Value
}

// ValidateCredential determines if the credentials are valid, returning nil on success
func (c *CSP) ValidateCredential(domType models.CspDomainTypeMutable, attrs map[string]models.ValueType) error {
	if string(domType) != CSPDomainType {
		return fmt.Errorf("invalid DomainType")
	}
	ccc, err := c.AzGetClientCredentialsConfig(attrs, false)
	if err == nil {
		var spt azuresdk.SPT
		spt, err = ccc.ServicePrincipalToken()
		if err == nil {
			err = spt.Refresh() // no network call made so far so use refresh to validate the credential
		}
	}
	return err
}

// AzGetClientCredentialsConfig returns a ClientCredentialsConfig object
func (c *CSP) AzGetClientCredentialsConfig(attrs map[string]models.ValueType, ignoreExtra bool) (azuresdk.CCC, error) {
	if err := azureAttributes.ValidateExpected(credentialAttributesNames, attrs, true, ignoreExtra); err != nil {
		return nil, err
	}
	return c.API.NewClientCredentialsConfig(attrs[AttrClientID].Value, attrs[AttrClientSecret].Value, attrs[AttrTenantID].Value), nil
}
