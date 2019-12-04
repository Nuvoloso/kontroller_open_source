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
	"net/http"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/azuresdk"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/op/go-logging"
	uuid "github.com/satori/go.uuid"
)

// Azure constants
const (
	AttrClientID                = "azure_client_id"
	AttrClientSecret            = "azure_client_secret"
	AttrLocation                = "azure_location"
	AttrSubscriptionID          = "azure_subscription_id"
	AttrTenantID                = "azure_tenant_id"
	AttrZone                    = "azure_zone"
	AttrResourceGroupName       = "azure_resource_group_name"
	AttrStorageAccountName      = "azure_storage_account_name"
	AttrPStoreBlobContainerName = "azure_protection_store_blob_container_name" // in azure these are referred to as BLOBContainers
	CSPDomainType               = "Azure"

	BLOBUploadTransferRate        = 0 // TBD: Need to figure out potential transfer rate
	azureClientDefaultTimeoutSecs = 30
	azureIMDDefaultTimeoutSecs    = 30
	protectionStoreNameFormat     = "nuvoloso-%s"

	azureInvalidBucketNameMsg = `- Container names must be unique within a Storage Account.
	- Container names must be lowercase
	- Container names can contian a-z, 0-9 and - characters
	- Container names must be at least 3 and no more than 63 characters long.`

	PADiskType = "azure_disk_type"

	newResourceNameFormat = "nuvoloso-%s-%s"
	MaxLunsPerVM          = 64

	ImdResourceGroupName = csp.IMDProvisioningPrefix + "AzureResourceGroupName"
	ImdLocation          = "AzureLocation"
	ImdOsType            = "AzureOsType"
	ImdTags              = "AzureTags"
	ImdVMSize            = "AzureVMSize"
)

// CSP is the Azure package cloud service provider abstraction
type CSP struct {
	Log        *logging.Logger
	IMDTimeout time.Duration
	clInit     azureClientInitializer
	exec       util.Exec
	// devCache   map[string]*InstanceDevice
	mux       sync.Mutex
	imdClient *http.Client
	API       azuresdk.API
}

type azureClientInitializer interface {
	clientInit(cl *Client) error
}

func init() {
	azureCSP := &CSP{exec: util.NewExec()}
	azureCSP.clInit = azureCSP // self-reference
	azureCSP.API = azuresdk.New()
	csp.RegisterCSP(azureCSP)
}

// // InstanceDevice includes details about a storage device on an instance
// type InstanceDevice struct {
// 	EphemeralDevice  bool
// 	VolumeIdentifier string
// }

// Type satisfies CloudServiceProvider
func (c *CSP) Type() models.CspDomainTypeMutable {
	return CSPDomainType
}

// SetDebugLogger satisfies CloudServiceProvider
func (c *CSP) SetDebugLogger(log *logging.Logger) {
	c.Log = log
}

func (c *CSP) dbg(args ...interface{}) {
	if c.Log != nil {
		c.Log.Debug(args...)
	}
}

func (c *CSP) dbgF(fmt string, args ...interface{}) {
	if c.Log != nil {
		c.Log.Debugf(fmt, args...)
	}
}

// Attribute descriptors
var azureAttributes = csp.AttributeDescriptionMap{
	AttrClientID:                models.AttributeDescriptor{Kind: common.ValueTypeString, Description: "The identifier of the service principal used for clusters in the domain"},
	AttrClientSecret:            models.AttributeDescriptor{Kind: common.ValueTypeSecret, Description: "The secret associated with the service principal"},
	AttrLocation:                models.AttributeDescriptor{Kind: common.ValueTypeString, Description: "The Azure location", Optional: true, Immutable: true},
	AttrSubscriptionID:          models.AttributeDescriptor{Kind: common.ValueTypeString, Description: "The Azure subscription identifier"},
	AttrTenantID:                models.AttributeDescriptor{Kind: common.ValueTypeString, Description: "The Azure ActiveDirectory identifier for the service principal", Immutable: true},
	AttrZone:                    models.AttributeDescriptor{Kind: common.ValueTypeString, Description: "The Azure zone", Optional: true, Immutable: true},
	AttrResourceGroupName:       models.AttributeDescriptor{Kind: common.ValueTypeString, Description: "The resource group used to track Azure resources", Immutable: true},
	AttrStorageAccountName:      models.AttributeDescriptor{Kind: common.ValueTypeString, Description: "The Azure storage account used for blob storage", Immutable: true},
	AttrPStoreBlobContainerName: models.AttributeDescriptor{Kind: common.ValueTypeString, Description: "The name of the protection store blob container", Optional: true, Immutable: true},
}

var credentialAttributesNames = []string{AttrClientID, AttrClientSecret, AttrTenantID}
var domainAttributesNames = []string{AttrSubscriptionID, AttrLocation, AttrZone, AttrResourceGroupName, AttrStorageAccountName, AttrPStoreBlobContainerName}
var allAttributeNames = util.StringKeys(azureAttributes)

// Attributes satisfies CloudServiceProvider
func (c *CSP) Attributes() map[string]models.AttributeDescriptor {
	return azureAttributes
}

// CredentialAttributes satisfies CloudServiceProvider
func (c *CSP) CredentialAttributes() map[string]models.AttributeDescriptor {
	credAttributes := make(map[string]models.AttributeDescriptor, len(credentialAttributesNames))
	for _, attr := range credentialAttributesNames {
		credAttributes[attr] = azureAttributes[attr]
	}
	return credAttributes
}

// DomainAttributes satisfies CloudServiceProvider
func (c *CSP) DomainAttributes() map[string]models.AttributeDescriptor {
	domAttributes := make(map[string]models.AttributeDescriptor, len(domainAttributesNames))
	for _, attr := range domainAttributesNames {
		domAttributes[attr] = azureAttributes[attr]
	}
	return domAttributes
}

// SupportedCspStorageTypes satisfies CloudServiceProvider
func (c *CSP) SupportedCspStorageTypes() []*models.CSPStorageType {
	return azureCspStorageTypes
}

// SupportedStorageFormulas returns the storage formulas for Azure
func (c *CSP) SupportedStorageFormulas() []*models.StorageFormula {
	return azureStorageFormulas
}

// SanitizedAttributes takes input CSPDomain attributes, sanitizes and returns them.
func (c *CSP) SanitizedAttributes(cspDomainAttrs map[string]models.ValueType) (map[string]models.ValueType, error) {
	if val, ok := cspDomainAttrs[AttrPStoreBlobContainerName]; ok && val.Value != "" {
		if val.Kind != "STRING" {
			return nil, fmt.Errorf("Persistent Store Bucket [BLOB Container] name must be a string")
		}
	} else {
		cspDomainAttrs[AttrPStoreBlobContainerName] = models.ValueType{Kind: "STRING", Value: fmt.Sprintf(protectionStoreNameFormat, uuidGenerator())}
	}
	if !CompatibleBucketName(cspDomainAttrs[AttrPStoreBlobContainerName].Value) {
		return nil, fmt.Errorf(azureInvalidBucketNameMsg)
	}

	return cspDomainAttrs, nil
}

var reDomain = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)

// CompatibleBucketName returns true if the bucket name is DNS compatible.
// Buckets created outside of the classic region MUST be DNS compatible.
// Based of similar pattern found at https://github.com/aws/aws-sdk-go/blob/master/service/s3/host_style_bucket.go
func CompatibleBucketName(bucket string) bool {
	return reDomain.MatchString(bucket)
}

// Indirection for UUID generation to support UT
type uuidGen func() string

var uuidGenerator uuidGen = func() string { return uuid.NewV4().String() }

// ProtectionStoreUploadTransferRate satisfies CloudServiceProvider
func (c *CSP) ProtectionStoreUploadTransferRate() int32 {
	return BLOBUploadTransferRate
}

// GetDeviceTypeByCspStorageType returns device type (SSD or HDD) for the given CspStorageType
func (c *CSP) GetDeviceTypeByCspStorageType(storageType models.CspStorageType) (string, error) {
	for _, st := range azureCspStorageTypes {
		if storageType == st.Name {
			return st.DeviceType, nil
		}
	}
	return "", fmt.Errorf("incorrect storageType %s", storageType)
}

// LocalInstanceDeviceName satisfies the LocalInstanceDiscover interface
func (c *CSP) LocalInstanceDeviceName(VolumeIdentifier, AttachedNodeDevice string) (string, error) {
	// AttachedNodeDevice is of the form
	//       /dev/disk/azure/scsi1/lunN
	// on a linux host where N is the LUN. The value is a symlink of the form "../../../sdc".
	// Currently the AKS Linux OS is Ubuntu 16.04; in this version blkid does not work on symlinks.
	// So we will return the expanded symlink (/dev/sdc).
	// See https://docs.microsoft.com/en-us/azure/virtual-machines/troubleshooting/troubleshoot-device-names-problems
	return filepath.EvalSymlinks(AttachedNodeDevice)
}

func newResourceName(rType string) string {
	return fmt.Sprintf(newResourceNameFormat, rType, uuidGenerator())
}
