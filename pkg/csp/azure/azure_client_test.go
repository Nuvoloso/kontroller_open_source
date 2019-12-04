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
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/storage/mgmt/storage"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockaz "github.com/Nuvoloso/kontroller/pkg/azuresdk/mock"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

type azureFakeClientInitializer struct {
	err error
}

func (fci *azureFakeClientInitializer) clientInit(cl *Client) error {
	return fci.err
}

func TestClientInit(t *testing.T) {
	assert := assert.New(t)

	csp, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(csp)
	azureCSP, ok := csp.(*CSP)
	assert.True(ok)
	assert.Equal(azureCSP, azureCSP.clInit) // self-ref

	azureCl := &Client{
		csp: azureCSP,
	}

	err = azureCSP.clientInit(azureCl)
	assert.NotNil(err)

	// real Client calls
	rgName := "resourceGroup"
	dObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID: "CSP-DOMAIN-1",
			},
			CspDomainType: models.CspDomainTypeMutable(CSPDomainType),
			CspDomainAttributes: map[string]models.ValueType{
				AttrResourceGroupName:  models.ValueType{Kind: "STRING", Value: rgName},
				AttrStorageAccountName: models.ValueType{Kind: "STRING", Value: "saName"},
				AttrSubscriptionID:     models.ValueType{Kind: "STRING", Value: "subid"},
				AttrClientID:           models.ValueType{Kind: "STRING", Value: "clientid"},
				AttrClientSecret:       models.ValueType{Kind: "SECRET", Value: "clientsecret"},
				AttrTenantID:           models.ValueType{Kind: "STRING", Value: "tenantid"},
			},
		},
		CSPDomainMutable: models.CSPDomainMutable{},
	}
	cl, err := azureCSP.Client(dObj)
	assert.NoError(err)
	azureCl, ok = cl.(*Client)
	assert.True(ok)
	assert.Equal(azureCSP, azureCl.csp)
	assert.Equal(time.Second*azureClientDefaultTimeoutSecs, azureCl.Timeout)
	assert.Equal(dObj.CspDomainAttributes, map[string]models.ValueType(azureCl.attrs))
	assert.Equal(dObj.Meta.ID, cl.ID())
	assert.Equal(dObj.CspDomainType, cl.Type())

	// resource group name from cluster over domain
	rgn := azureCl.getResourceGroup(nil)
	assert.Equal(rgName, rgn)
	rgn = azureCl.getResourceGroup(map[string]models.ValueType{
		ImdResourceGroupName: models.ValueType{Kind: "STRING", Value: "imd-rg-name"}})
	assert.Equal("imd-rg-name", rgn)

	// timeout can be set
	cl.SetTimeout(azureClientDefaultTimeoutSecs + 1)
	assert.EqualValues((azureClientDefaultTimeoutSecs+1)*time.Second, azureCl.Timeout)
	cl.SetTimeout(-1)
	assert.EqualValues(azureClientDefaultTimeoutSecs*time.Second, azureCl.Timeout)

	// try client construction
	assert.Nil(azureCl.disksClient)
	dc := azureCl.newDisksClient()
	assert.NotNil(dc)
	assert.Equal(dc, azureCl.disksClient)
	assert.Equal(azureCl.disksClient, azureCl.newDisksClient())

	assert.Nil(azureCl.vmClient)
	vmc := azureCl.newVMClient()
	assert.NotNil(vmc)
	assert.Equal(vmc, azureCl.vmClient)
	assert.Equal(azureCl.vmClient, azureCl.newVMClient())

	// doWithRetry
	cnt := 0
	opErr := fmt.Errorf("op-error")
	op := func() (bool, error) {
		cnt++
		if cnt == 2 {
			return false, opErr
		}
		return true, nil
	}
	err = azureCl.doWithRetry(op)
	assert.Error(err)
	assert.Equal(opErr, err)

	// clientInit fails
	delete(dObj.CspDomainAttributes, AttrTenantID)
	cl, err = azureCSP.Client(dObj)
	assert.Error(err)
	assert.Nil(cl)

	// invalid domain type
	dObj.CspDomainType = "badtype"
	cl, err = azureCSP.Client(dObj)
	assert.Error(err)
	assert.Nil(cl)
}

func TestValidate(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()

	csp, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(csp)
	azureCSP, ok := csp.(*CSP)
	assert.True(ok)
	assert.NotNil(azureCSP)

	azureCl := &Client{
		csp: azureCSP,
	}

	// real Client calls
	dObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID: "CSP-DOMAIN-1",
			},
			CspDomainType: models.CspDomainTypeMutable(CSPDomainType),
			CspDomainAttributes: map[string]models.ValueType{
				AttrResourceGroupName:  models.ValueType{Kind: "STRING", Value: "rgName"},
				AttrStorageAccountName: models.ValueType{Kind: "STRING", Value: "saName"},
				AttrSubscriptionID:     models.ValueType{Kind: "STRING", Value: "subid"},
				AttrClientID:           models.ValueType{Kind: "STRING", Value: "clientid"},
				AttrClientSecret:       models.ValueType{Kind: "SECRET", Value: "clientsecret"},
				AttrTenantID:           models.ValueType{Kind: "STRING", Value: "tenantid"},
			},
		},
		CSPDomainMutable: models.CSPDomainMutable{},
	}
	dc, err := azureCSP.Client(dObj)
	assert.NoError(err)
	azureCl, ok = dc.(*Client)
	err = azureCl.Validate(ctx)
	assert.Nil(err)

	// missing parameters
	delete(azureCl.attrs, AttrSubscriptionID)
	err = azureCl.Validate(ctx)
	assert.Error(err)
}

func TestCreateProtectionStore(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()

	csp, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(csp)
	azureCSP, ok := csp.(*CSP)
	assert.True(ok)
	assert.NotNil(azureCSP)

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cl := mockaz.NewMockAPI(mockCtrl)
	gcl := mockaz.NewMockGC(mockCtrl)
	acl := mockaz.NewMockAC(mockCtrl)
	bccl := mockaz.NewMockBCC(mockCtrl)
	azureCl := &Client{
		csp: azureCSP,
	}

	subID := "nuvolosoSubID"
	groupName := "nuvolosoGroup"
	accountName := "storageAccountName"
	blobName := "blobName"
	location := "uswest"
	groupRes := resources.Group{
		Location: swag.String(location),
	}
	key1 := "key1"
	keys := []storage.AccountKey{
		storage.AccountKey{
			Value: swag.String(key1),
		},
	}
	keyList := storage.AccountListKeysResult{
		Keys: &keys,
	}
	blobParams := storage.BlobContainer{}
	blobRes := storage.BlobContainer{}

	azureCl.client = cl
	azureCl.attrs = make(map[string]models.ValueType)
	azureCl.attrs[AttrSubscriptionID] = models.ValueType{Kind: "STRING", Value: subID}
	azureCl.attrs[AttrResourceGroupName] = models.ValueType{Kind: "STRING", Value: groupName}
	azureCl.attrs[AttrStorageAccountName] = models.ValueType{Kind: "STRING", Value: accountName}
	azureCl.attrs[AttrPStoreBlobContainerName] = models.ValueType{Kind: "STRING", Value: blobName}

	// success
	cl.EXPECT().NewGroupsClient(subID, gomock.Any()).Return(gcl)
	gcl.EXPECT().Get(ctx, groupName).Return(groupRes, nil)
	cl.EXPECT().NewAccountsClient(subID, gomock.Any()).Return(acl)
	acl.EXPECT().Get(ctx, groupName, accountName).Return(storage.Account{}, nil)
	acl.EXPECT().ListKeys(ctx, groupName, accountName).Return(keyList, nil)
	cl.EXPECT().NewBlobContainersClient(subID, gomock.Any()).Return(bccl)
	bccl.EXPECT().Create(ctx, groupName, accountName, blobName, blobParams).Return(blobRes, nil)
	_, ok = azureCl.attrs[AttrLocation]
	assert.False(ok)
	attrMap, err := azureCl.CreateProtectionStore(ctx)
	assert.Nil(err)
	assert.NotNil(attrMap)
	loc := attrMap[AttrLocation].Value
	assert.Equal(location, loc)

	// failure to create blob container
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaz.NewMockAPI(mockCtrl)
	gcl = mockaz.NewMockGC(mockCtrl)
	acl = mockaz.NewMockAC(mockCtrl)
	bccl = mockaz.NewMockBCC(mockCtrl)
	azureCl.client = cl
	cl.EXPECT().NewGroupsClient(subID, gomock.Any()).Return(gcl)
	gcl.EXPECT().Get(ctx, groupName).Return(groupRes, nil)
	cl.EXPECT().NewAccountsClient(subID, gomock.Any()).Return(acl)
	acl.EXPECT().Get(ctx, groupName, accountName).Return(storage.Account{}, nil)
	acl.EXPECT().ListKeys(ctx, groupName, accountName).Return(keyList, nil)
	cl.EXPECT().NewBlobContainersClient(subID, gomock.Any()).Return(bccl)
	bccl.EXPECT().Create(ctx, groupName, accountName, blobName, blobParams).Return(blobRes, fmt.Errorf("blob create failure"))
	attrMap, err = azureCl.CreateProtectionStore(ctx)
	assert.Error(err)
	assert.Regexp("blob create failure", err.Error())

	// no keys available, key list empty
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaz.NewMockAPI(mockCtrl)
	gcl = mockaz.NewMockGC(mockCtrl)
	acl = mockaz.NewMockAC(mockCtrl)
	bccl = mockaz.NewMockBCC(mockCtrl)
	azureCl.client = cl
	cl.EXPECT().NewGroupsClient(subID, gomock.Any()).Return(gcl)
	gcl.EXPECT().Get(ctx, groupName).Return(groupRes, nil)
	cl.EXPECT().NewAccountsClient(subID, gomock.Any()).Return(acl)
	acl.EXPECT().Get(ctx, groupName, accountName).Return(storage.Account{}, nil)
	acl.EXPECT().ListKeys(ctx, groupName, accountName).Return(storage.AccountListKeysResult{}, nil)
	attrMap, err = azureCl.CreateProtectionStore(ctx)
	assert.Error(err)
	assert.Regexp("Keys unavailable for Azure storage account", err.Error())

	// no keys available
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaz.NewMockAPI(mockCtrl)
	gcl = mockaz.NewMockGC(mockCtrl)
	acl = mockaz.NewMockAC(mockCtrl)
	bccl = mockaz.NewMockBCC(mockCtrl)
	azureCl.client = cl
	cl.EXPECT().NewGroupsClient(subID, gomock.Any()).Return(gcl)
	gcl.EXPECT().Get(ctx, groupName).Return(groupRes, nil)
	cl.EXPECT().NewAccountsClient(subID, gomock.Any()).Return(acl)
	acl.EXPECT().Get(ctx, groupName, accountName).Return(storage.Account{}, nil)
	acl.EXPECT().ListKeys(ctx, groupName, accountName).Return(storage.AccountListKeysResult{Keys: &[]storage.AccountKey{}}, nil)
	attrMap, err = azureCl.CreateProtectionStore(ctx)
	assert.Error(err)
	assert.Regexp("Keys unavailable for Azure storage account", err.Error())

	// list keys error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaz.NewMockAPI(mockCtrl)
	gcl = mockaz.NewMockGC(mockCtrl)
	acl = mockaz.NewMockAC(mockCtrl)
	bccl = mockaz.NewMockBCC(mockCtrl)
	azureCl.client = cl
	cl.EXPECT().NewGroupsClient(subID, gomock.Any()).Return(gcl)
	gcl.EXPECT().Get(ctx, groupName).Return(groupRes, nil)
	cl.EXPECT().NewAccountsClient(subID, gomock.Any()).Return(acl)
	acl.EXPECT().Get(ctx, groupName, accountName).Return(storage.Account{}, nil)
	acl.EXPECT().ListKeys(ctx, groupName, accountName).Return(storage.AccountListKeysResult{Keys: &[]storage.AccountKey{}}, fmt.Errorf("list key error"))
	attrMap, err = azureCl.CreateProtectionStore(ctx)
	assert.Error(err)
	assert.Regexp("list key error", err.Error())

	// get account error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaz.NewMockAPI(mockCtrl)
	gcl = mockaz.NewMockGC(mockCtrl)
	acl = mockaz.NewMockAC(mockCtrl)
	bccl = mockaz.NewMockBCC(mockCtrl)
	azureCl.client = cl
	cl.EXPECT().NewGroupsClient(subID, gomock.Any()).Return(gcl)
	gcl.EXPECT().Get(ctx, groupName).Return(groupRes, nil)
	cl.EXPECT().NewAccountsClient(subID, gomock.Any()).Return(acl)
	acl.EXPECT().Get(ctx, groupName, accountName).Return(storage.Account{}, fmt.Errorf("get account error"))
	attrMap, err = azureCl.CreateProtectionStore(ctx)
	assert.Error(err)
	assert.Regexp("get account error", err.Error())

	// get group error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaz.NewMockAPI(mockCtrl)
	gcl = mockaz.NewMockGC(mockCtrl)
	acl = mockaz.NewMockAC(mockCtrl)
	bccl = mockaz.NewMockBCC(mockCtrl)
	azureCl.client = cl
	cl.EXPECT().NewGroupsClient(subID, gomock.Any()).Return(gcl)
	gcl.EXPECT().Get(ctx, groupName).Return(groupRes, fmt.Errorf("get group error"))
	attrMap, err = azureCl.CreateProtectionStore(ctx)
	assert.Error(err)
	assert.Regexp("get group error", err.Error())

	// location mismatch
	azureCl.attrs[AttrLocation] = models.ValueType{Kind: "STRING", Value: "bad location"}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaz.NewMockAPI(mockCtrl)
	gcl = mockaz.NewMockGC(mockCtrl)
	azureCl.client = cl
	cl.EXPECT().NewGroupsClient(subID, gomock.Any()).Return(gcl)
	gcl.EXPECT().Get(ctx, groupName).Return(groupRes, nil)
	attrMap, err = azureCl.CreateProtectionStore(ctx)
	assert.Error(err)
	assert.Regexp("location mismatch", err.Error())

	// location empty
	azureCl.attrs[AttrLocation] = models.ValueType{Kind: "STRING", Value: ""}
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaz.NewMockAPI(mockCtrl)
	gcl = mockaz.NewMockGC(mockCtrl)
	azureCl.client = cl
	cl.EXPECT().NewGroupsClient(subID, gomock.Any()).Return(gcl)
	gcl.EXPECT().Get(ctx, groupName).Return(groupRes, nil)
	err = azureCl.validateResourceGroup(ctx, subID, groupName)
	assert.NoError(err)
	assert.Equal(location, azureCl.attrs[AttrLocation].Value)
}

func TestValidateCredential(t *testing.T) {
	assert := assert.New(t)

	csp, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(csp)
	azureCSP, ok := csp.(*CSP)
	assert.True(ok)
	assert.NotNil(azureCSP)
	assert.NotNil(azureCSP.API)

	t.Log("invalid domain type")
	err = azureCSP.ValidateCredential("InvalidDomainType", map[string]models.ValueType{})
	assert.NotNil(err)

	t.Log("empty attributes")
	err = azureCSP.ValidateCredential(CSPDomainType, map[string]models.ValueType{})
	assert.NotNil(err)

	am := map[string]models.ValueType{}
	for i, n := range credentialAttributesNames {
		t.Log("Added attr", n)
		ad, ok := azureAttributes[n]
		assert.True(ok, "Attr %s", n)
		am[n] = models.ValueType{Kind: ad.Kind, Value: n}
		if i < len(credentialAttributesNames)-1 {
			err = azureCSP.ValidateCredential(CSPDomainType, am)
			assert.Error(err)
		}
	}

	api := azureCSP.API
	// mock the API to check the values of ccc
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	mAPI := mockaz.NewMockAPI(mockCtrl)
	mCCC := mockaz.NewMockCCC(mockCtrl)
	mSPT := mockaz.NewMockSPT(mockCtrl)
	azureCSP.API = mAPI
	mAPI.EXPECT().NewClientCredentialsConfig(AttrClientID, AttrClientSecret, AttrTenantID).Return(mCCC)
	mCCC.EXPECT().ServicePrincipalToken().Return(mSPT, nil)
	mSPT.EXPECT().Refresh().Return(nil)
	err = azureCSP.ValidateCredential(CSPDomainType, am)
	assert.NoError(err)
	mockCtrl.Finish()

	mockCtrl = gomock.NewController(t)
	mAPI = mockaz.NewMockAPI(mockCtrl)
	mCCC = mockaz.NewMockCCC(mockCtrl)
	mSPT = mockaz.NewMockSPT(mockCtrl)
	azureCSP.API = mAPI
	mAPI.EXPECT().NewClientCredentialsConfig(AttrClientID, AttrClientSecret, AttrTenantID).Return(mCCC)
	mCCC.EXPECT().ServicePrincipalToken().Return(mSPT, nil)
	mSPT.EXPECT().Refresh().Return(fmt.Errorf("bad spt"))
	err = azureCSP.ValidateCredential(CSPDomainType, am)
	assert.Error(err)

	// validate that the authorizer is really created
	azureCSP.API = api // restore
	aa, err := azureCSP.AzGetClientCredentialsConfig(am, false)
	assert.NoError(err)
	assert.NotNil(aa)
}
