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


package gc

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/op/go-logging"
	uuid "github.com/satori/go.uuid"
)

// Google Cloud constants
const (
	AttrCred             = "gc_cred"
	AttrPStoreBucketName = "gc_protection_store_bucket_name"
	AttrZone             = "gc_zone"
	ProjectID            = "project_id"
	PAVolumeType         = "gc_gce_volume_type"
	PAIopsGB             = "gc_iops_per_GB"
	PAIopsMin            = "gc_min_iops_per_device"
	PAIopsMax            = "gc_max_iops_per_device"
	PAService            = "gc_service"
	ServiceGCE           = "gce"
	ServiceGCS           = "gcs" // Google Cloud Storage

	CSPDomainType        = "GCP"
	PSUploadTransferRate = 1048576 // TBD, we have no idea what the transfer rate is, so start more pessimistic

	clientDefaultTimeoutSecs = 30
	imdDefaultTimeoutSecs    = 30
	imdQuery                 = "http://metadata.google.internal/computeMetadata/v1/instance/?recursive=true&alt=json"

	diskDir                   = "/dev/disk/by-id"
	diskPathFormat            = diskDir + "/google-%s"
	diskSourceURL             = "https://www.googleapis.com/compute/v1/projects/%s/zones/%s/disks/%s" // "projects/vs-test-project-254718/zones/us-west1-a/disks/nuvoloso-disk-2"
	nuvoNamePrefix            = "nuvoloso-"
	protectionStoreNameFormat = "nuvoloso-%s"
	volTypeURL                = "https://www.googleapis.com/compute/v1/projects/%s/zones/%s/diskTypes/%s" // "https://www.googleapis.com/compute/v1/projects/vs-test-project-254718/zones/us-west1-a/diskTypes/pd-standard"

	invalidBucketNameMsg = `- Bucket names must be unique across all existing bucket names in Cloud Storage namespace.
	- Bucket names must contain only lowercase letters, numbers, dashes (-), underscores (_), and dots (.). Names containing dots require verification.
	- Bucket names must start and end with a number or letter.
	- Bucket names must contain 3 to 63 characters. Names containing dots can contain up to 222 characters, but each dot-separated component can be no longer than 63 characters.
	- Bucket names cannot be represented as an IP address in dotted-decimal notation (for example, 192.168.5.4).
	- Bucket names cannot begin with the "goog" prefix.
	- Bucket names cannot contain "google" or close misspellings, such as "g00gle".
	- Bucket names must comply with DNS naming conventions. For DNS compliance and future compatibility, you should not use underscores (_) or have a period adjacent to another period or dash. For example, ".." or "-." or ".-" are not valid in DNS names.`
)

// CSP is the Google Cloud package cloud service provider abstraction
type CSP struct {
	Log        *logging.Logger
	IMDTimeout time.Duration
	imdClient  *http.Client
	clInit     clientInitializer
	exec       util.Exec
	mux        sync.Mutex
}

// InstanceMetaData represents the instance metadata of interest to us
type InstanceMetaData struct {
	Attributes        map[string]string
	Hostname          string // internal hostname
	ID                int64  // just a number
	MachineType       string // projects/[NUMERIC_PROJECT_ID]/machineTypes/[MACHINE_TYPE]
	Name              string // same as kubernetes node name
	NetworkInterfaces []NetworkInterface
	Zone              string // projects/[NUMERIC_PROJECT_ID]/zones/[ZONE]
}

// NetworkInterface represents the network interfaces in the instance metadata
type NetworkInterface struct {
	AccessConfigs []AccessConfig
	IP            string
}

// AccessConfig represents the network access configuration in the instance metadata
type AccessConfig struct {
	ExternalIP string
}

type clientInitializer interface {
	clientInit(cl *Client) error
}

func init() {
	gcCSP := &CSP{exec: util.NewExec(), imdClient: &http.Client{}}
	gcCSP.clInit = gcCSP // self-reference
	csp.RegisterCSP(gcCSP)
}

// Type satisfies CloudServiceProvider
func (c *CSP) Type() models.CspDomainTypeMutable {
	return CSPDomainType
}

// SetDebugLogger satisfies CloudServiceProvider
func (c *CSP) SetDebugLogger(log *logging.Logger) {
	c.Log = log
}

func (c *CSP) dbgF(fmt string, args ...interface{}) {
	if c.Log != nil {
		c.Log.Debugf(fmt, args...)
	}
}

var gcAttributes = map[string]models.AttributeDescriptor{
	AttrCred:             models.AttributeDescriptor{Kind: com.ValueTypeSecret, Description: "The Google Cloud Platform service account credential JSON"},
	AttrZone:             models.AttributeDescriptor{Kind: com.ValueTypeString, Description: "The Google Cloud Platform project zone", Immutable: true},
	AttrPStoreBucketName: models.AttributeDescriptor{Kind: com.ValueTypeString, Description: "The Google Cloud Platform project bucket name used for persistent storage", Optional: true, Immutable: true},
}

var credentialAttributesNames = []string{AttrCred}
var domainAttributesNames = []string{AttrZone, AttrPStoreBucketName}

// Attributes satisfies CloudServiceProvider
func (c *CSP) Attributes() map[string]models.AttributeDescriptor {
	return gcAttributes
}

// CredentialAttributes satisfies CloudServiceProvider
func (c *CSP) CredentialAttributes() map[string]models.AttributeDescriptor {
	credAttributes := make(map[string]models.AttributeDescriptor, len(credentialAttributesNames))
	for _, attr := range credentialAttributesNames {
		credAttributes[attr] = gcAttributes[attr]
	}
	return credAttributes
}

// DomainAttributes satisfies CloudServiceProvider
func (c *CSP) DomainAttributes() map[string]models.AttributeDescriptor {
	domAttributes := make(map[string]models.AttributeDescriptor, len(domainAttributesNames))
	for _, attr := range domainAttributesNames {
		domAttributes[attr] = gcAttributes[attr]
	}
	return domAttributes
}

// SupportedCspStorageTypes satisfies CloudServiceProvider
func (c *CSP) SupportedCspStorageTypes() []*models.CSPStorageType {
	return gcCspStorageTypes
}

// SupportedStorageFormulas returns the storage formulas for Google Cloud
func (c *CSP) SupportedStorageFormulas() []*models.StorageFormula {
	return gcStorageFormulas
}

// SanitizedAttributes takes input CSPDomain attributes, sanitizes and returns them.
func (c *CSP) SanitizedAttributes(cspDomainAttrs map[string]models.ValueType) (map[string]models.ValueType, error) {
	// TBD: more might be needed
	if val, ok := cspDomainAttrs[AttrPStoreBucketName]; ok && val.Value != "" {
		if val.Kind != com.ValueTypeString {
			return nil, errors.New("persistent Store Bucket name must be a string")
		}
	} else {
		cspDomainAttrs[AttrPStoreBucketName] = models.ValueType{Kind: com.ValueTypeString, Value: fmt.Sprintf(protectionStoreNameFormat, uuidGenerator())}
	}
	if CompatibleBucketName(cspDomainAttrs[AttrPStoreBucketName].Value) {
		return cspDomainAttrs, nil
	}
	return nil, errors.New(invalidBucketNameMsg)
}

// TBD improve reDomain. The full rules are complex: https://cloud.google.com/storage/docs/domain-name-verification
var reDomain = regexp.MustCompile(`^[a-z0-9][a-z0-9\.\-]{1,61}[a-z0-9]$`)
var reIPAddress = regexp.MustCompile(`^(\d+\.){3}\d+$`)
var reGoogle = regexp.MustCompile(`g[o|0]{2}gl?e?`)

// CompatibleBucketName returns true if the bucket name is DNS compatible: https://cloud.google.com/storage/docs/naming
func CompatibleBucketName(bucket string) bool {
	return reDomain.MatchString(bucket) &&
		!reIPAddress.MatchString(bucket) &&
		!reGoogle.MatchString(bucket) &&
		!strings.Contains(bucket, "..") &&
		!strings.Contains(bucket, ".-") &&
		!strings.Contains(bucket, "-.")
}

// ProtectionStoreUploadTransferRate satisfies CloudServiceProvider
func (c *CSP) ProtectionStoreUploadTransferRate() int32 {
	return PSUploadTransferRate
}

// GetDeviceTypeByCspStorageType returns device type (SSD or HDD) for the given CspStorageType
func (c *CSP) GetDeviceTypeByCspStorageType(storageType models.CspStorageType) (string, error) {
	for _, st := range gcCspStorageTypes {
		if storageType == st.Name {
			return st.DeviceType, nil
		}
	}
	return "", fmt.Errorf("incorrect storageType %s", storageType)
}

// LocalInstanceMetadata satisfies the LocalInstanceDiscover interface
func (c *CSP) LocalInstanceMetadata() (map[string]string, error) {
	imd, err := c.imdFetch()
	if err != nil {
		return nil, err
	}
	res := make(map[string]string)
	res[csp.IMDHostname] = imd.Name // csp.IMDHostname is not set to the DNS name. For GKS the imd.Name is the same as the kubernetes name
	res[csp.IMDInstanceName] = imd.Name
	res["id"] = imd.Name // used as NodeIdentifier. Use Name so it matches the csp.Volume.Attachments.NodeIdentifier
	res["local-hostname"] = imd.Hostname
	if parts := strings.Split(imd.Zone, "/"); len(parts) == 4 {
		res["project-number"] = parts[1]
		res[csp.IMDZone] = parts[3]
	}
	if parts := strings.Split(imd.MachineType, "/"); len(parts) == 4 {
		res["machine-type"] = parts[3]
	}
	if clName, ok := imd.Attributes["cluster-name"]; ok { // set for instance in a GKS cluster
		res["cluster-name"] = clName
	}
out:
	for _, iF := range imd.NetworkInterfaces {
		res[csp.IMDLocalIP] = iF.IP
		for _, cfg := range iF.AccessConfigs {
			if cfg.ExternalIP != "" {
				res["public-ipv4"] = cfg.ExternalIP
				break out
			}
		}
	}
	return res, nil
}

// LocalInstanceMetadataSetTimeout satisfies the LocalInstanceDiscover interface
func (c *CSP) LocalInstanceMetadataSetTimeout(secs int) {
	c.IMDTimeout, _ = time.ParseDuration(fmt.Sprintf("%ds", secs))
}

// see the documentation at https://cloud.google.com/compute/docs/storing-retrieving-metadata
func (c *CSP) imdFetch() (*InstanceMetaData, error) {
	if c.IMDTimeout == 0 {
		c.LocalInstanceMetadataSetTimeout(imdDefaultTimeoutSecs)
	}
	c.imdClient.Timeout = c.IMDTimeout
	req, _ := http.NewRequest("GET", imdQuery, nil)
	req.Header.Add("Metadata-Flavor", "Google")
	res, err := c.imdClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error getting instance metadata: %w", err)
	}
	defer res.Body.Close()
	imd := &InstanceMetaData{}
	dec := json.NewDecoder(res.Body)
	if err = dec.Decode(imd); err != nil {
		return nil, err
	}
	return imd, nil
}

// InDomain checks that the availability zone matches.
func (c *CSP) InDomain(cspDomainAttrs map[string]models.ValueType, imd map[string]string) error {
	var cspDomainZone, imdZone string
	attrsOK := false
	if vt, ok := cspDomainAttrs[AttrZone]; ok && vt.Kind == com.ValueTypeString {
		cspDomainZone = vt.Value
		if s, ok := imd[csp.IMDZone]; ok {
			imdZone = s
			attrsOK = true
		}
	}
	if !attrsOK {
		return fmt.Errorf("missing or invalid domain attribute %s or instance attribute %s", AttrZone, csp.IMDZone)
	}
	if cspDomainZone != imdZone {
		return fmt.Errorf("zone does not match (domain %s=%s, instance %s=%s)", AttrZone, cspDomainZone, csp.IMDZone, imdZone)
	}
	return nil
}

// LocalInstanceDeviceName satisfies the LocalInstanceDiscover interface
func (c *CSP) LocalInstanceDeviceName(VolumeIdentifier, AttachedNodeDevice string) (string, error) {
	switch svc, _, _ := VolumeIdentifierParse(VolumeIdentifier); svc {
	case ServiceGCE:
		// thanks to udev rules and the way we attach disks, the device path does not change
		// However, when GKE updates an instance (includes a reboot), disks can be auto-detached. So, if the path no longer exists, return empty string
		if _, err := os.Lstat(AttachedNodeDevice); err != nil && os.IsNotExist(err) {
			return "", nil
		}
		return AttachedNodeDevice, nil
	}
	return "", errors.New("storage type currently unsupported")
}

// Indirection for UUID generation to support UT
type uuidGen func() string

var uuidGenerator uuidGen = func() string { return uuid.NewV4().String() }
