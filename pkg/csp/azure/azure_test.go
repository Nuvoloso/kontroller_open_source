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
	"fmt"
	"sort"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
)

func TestAzureAttributes(t *testing.T) {
	assert := assert.New(t)

	c, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(c)

	attrs := c.Attributes()
	assert.NotNil(attrs)
	assert.Equal(azureAttributes, csp.AttributeDescriptionMap(attrs))

	attrs = c.CredentialAttributes()
	assert.NotNil(attrs)
	assert.Equal(credentialAttributesNames, util.SortedStringKeys(attrs))

	attrs = c.DomainAttributes()
	assert.NotNil(attrs)
	sort.Strings(domainAttributesNames)
	assert.Equal(domainAttributesNames, util.SortedStringKeys(attrs))
}

func TestLocalInstanceDeviceName(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	c, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(c)
	c.SetDebugLogger(tl.Logger())
	azureCSP, ok := c.(*CSP)
	assert.True(ok)
	assert.NotNil(azureCSP)
	assert.NotNil(azureCSP.Log)
	assert.NotPanics(func() {
		azureCSP.dbg("test1")
		azureCSP.dbgF("test2 %s", "foo")
	})

	name, err := c.LocalInstanceDeviceName("somevolID", "somedevice")
	assert.NotNil(err)
	assert.Empty(name)
}

func TestAzureStorageTypes(t *testing.T) {
	assert := assert.New(t)
	c, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(c)

	sTs := c.SupportedCspStorageTypes()
	assert.NotNil(sTs)
	assert.Len(azureCspStorageTypes, len(sTs))
	assert.Equal(azureCspStorageTypes, sTs)

	devType, err := c.GetDeviceTypeByCspStorageType("someStorageType")
	assert.Empty(devType)
	assert.NotNil(err)

	for _, cspST := range azureCspStorageTypes {
		devType, err = c.GetDeviceTypeByCspStorageType(cspST.Name)
		assert.NoError(err)
		assert.Equal(cspST.DeviceType, devType)
	}
}

func TestAzureStorageFormulas(t *testing.T) {
	assert := assert.New(t)
	c, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(c)

	sSF := c.SupportedStorageFormulas()
	assert.NotNil(sSF)
	assert.Len(azureStorageFormulas, len(sSF))
	assert.Equal(azureStorageFormulas, sSF)
}

func TestTransferRate(t *testing.T) {
	assert := assert.New(t)
	c, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(c)
	assert.Equal(int32(BLOBUploadTransferRate), c.ProtectionStoreUploadTransferRate())
}

func TestValidBucketName(t *testing.T) {
	assert := assert.New(t)
	res := CompatibleBucketName("hello_hi")
	assert.False(res)

	res = CompatibleBucketName("nuvoloso-")
	assert.False(res)

	res = CompatibleBucketName("nuvoloso-2fc37d6d-dd0d-4aef-b3f5-4cfef21c570c")
	assert.True(res)
}

func TestSanitizedAttributes(t *testing.T) {
	assert := assert.New(t)
	c, err := csp.NewCloudServiceProvider(CSPDomainType)
	domainAttrs := map[string]models.ValueType{}
	saveUUIDGen := uuidGenerator
	defer func() { uuidGenerator = saveUUIDGen }()
	fakeUUID := "a17e6b01-e679-4ea8-9cae-0f4be22d67a7"
	uuidGenerator = func() string { return fakeUUID }

	// Empty attributes
	attr, err := c.SanitizedAttributes(domainAttrs)
	assert.Nil(err)
	assert.NotNil(attr)
	assert.Equal("nuvoloso-a17e6b01-e679-4ea8-9cae-0f4be22d67a7", attr[AttrPStoreBlobContainerName].Value)

	// success cases
	tcs := []string{"", "bucketname", "bucket-name", "123-bucket", "123", "aaaaaaaaaabbbbbbbbbbccccccccccddddddddddeeeeeeeeeeffffffffffggg"}
	var retVal string
	for _, tc := range tcs {
		switch tc {
		case "":
			retVal = fmt.Sprintf(protectionStoreNameFormat, fakeUUID)
		default:
			domainAttrs[AttrPStoreBlobContainerName] = models.ValueType{Kind: "STRING", Value: tc}
			retVal = tc
		}
		attr, err := c.SanitizedAttributes(domainAttrs)
		assert.Nil(err)
		assert.NotNil(attr)
		assert.Equal(retVal, attr[AttrPStoreBlobContainerName].Value)
	}

	// failure cases
	tcs = []string{"12", "dot..dottest", "bucket.name", "192.168.5.4", "hyphen-.dottest", "dot.-hyphentest", "aaaaaaaaaabbbbbbbbbbccccccccccddddddddddeeeeeeeeeeffffffffffgggg"}
	for _, tc := range tcs {
		domainAttrs[AttrPStoreBlobContainerName] = models.ValueType{Kind: "STRING", Value: tc}
		attr, err := c.SanitizedAttributes(domainAttrs)
		assert.NotNil(err)
		assert.Nil(attr)
		assert.Equal(azureInvalidBucketNameMsg, err.Error())
	}

	// invalid type
	domainAttrs[AttrPStoreBlobContainerName] = models.ValueType{Kind: "NOTSTRING", Value: "randomValue"}
	attr, err = c.SanitizedAttributes(domainAttrs)
	assert.NotNil(err)
	assert.Nil(attr)
	assert.Equal("Persistent Store Bucket [BLOB Container] name must be a string", err.Error())
}

func TestUUIDGenerator(t *testing.T) {
	assert := assert.New(t)

	uuidS := uuidGenerator()
	assert.NotEmpty(uuidS)
	uuidV, err := uuid.FromString(uuidS)
	assert.NoError(err)
	assert.Equal(uuidS, uuidV.String())
}
