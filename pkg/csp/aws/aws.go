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

// Instance meta-data support adapted from https://github.com/travisjeffery/go-ec2-metadata, MIT license

package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/op/go-logging"
	uuid "github.com/satori/go.uuid"
)

// AWS constants
const (
	AttrAccessKeyID      = "aws_access_key_id"
	AttrAvailabilityZone = "aws_availability_zone"
	AttrRegion           = "aws_region"
	AttrSecretAccessKey  = "aws_secret_access_key"
	AttrPStoreBucketName = "aws_protection_store_bucket_name"
	CSPDomainType        = "AWS"
	NameTag              = "Name" // well known tag for all objects to give the object a name
	PAEC2VolumeType      = "aws_ec2_volume_type"
	PAIopsGB             = "aws_iops_per_GB"
	PAIopsMin            = "aws_min_iops_per_device"
	PAIopsMax            = "aws_max_iops_per_device"
	PAService            = "aws_service"
	ServiceEC2           = "ec2"
	ServiceS3            = "S3"
	S3UploadTransferRate = 1048576 // For now, we're not coming close to the published transfer rate, so start more pessimistic
	//	S3UploadTransferRate     = 7340032 // https://www.rightscale.com/blog/cloud-industry-insights/network-performance-within-amazon-ec2-and-amazon-s3
	awsBasePath                 = "/latest/meta-data/"
	awsClientDefaultTimeoutSecs = 30
	awsCredsProviderName        = "NuvoAwsCredsProvider"
	awsIMDDefaultTimeoutSecs    = 30
	awsIPProto                  = "http://169.254.169.254"
	awsNuvoNamePrefix           = "nuvoloso-"
	protectionStoreNameFormat   = "nuvoloso.%s"

	awsInvalidBucketNameMsg = `- Bucket names must be unique across all existing bucket names in Amazon S3.
	- Bucket names must comply with DNS naming conventions.
	- Bucket names must be at least 3 and no more than 63 characters long.
	- Bucket names must not contain uppercase characters or underscores.
	- Bucket names must start with a lowercase letter or number.
	- Bucket names must be a series of one or more labels. Adjacent labels are separated by a single period (.). Bucket names can contain lowercase letters, numbers, and hyphens. Each label must start and end with a lowercase letter or a number.
	- Bucket names must not be formatted as an IP address (for example, 192.168.5.4).`

	awsNVMeEBS       = "Amazon Elastic Block Store              "
	awsNVMeEphemeral = "Amazon EC2 NVMe Instance Storage        "
)

var awsIMDProps = []string{
	"public-ipv4",
	"local-hostname",
	"local-ipv4",
	"instance-id",
	"placement/availability-zone",
	"instance-type",
	"public-hostname",
}

var awsIMDRenamedProps = map[string]string{
	"placement/availability-zone": csp.IMDZone,
	"instance-id":                 csp.IMDInstanceName,
	"local-ipv4":                  csp.IMDLocalIP,
	"local-hostname":              csp.IMDHostname,
}

// CSP is the AWS package cloud service provider abstraction
type CSP struct {
	Log        *logging.Logger
	IMDTimeout time.Duration
	clInit     awsClientInitializer
	exec       util.Exec
	devCache   map[string]*InstanceDevice // key is device name, eg "nvme0n1"
	mux        sync.Mutex
}

// InstanceDevice includes details about a storage device on an instance
type InstanceDevice struct {
	EphemeralDevice  bool
	VolumeIdentifier string
}

type awsClientInitializer interface {
	clientInit(cl *Client) error
}

// cspMutex is available for internal use in this package
var cspMutex sync.Mutex

func init() {
	awsCSP := &CSP{exec: util.NewExec()}
	awsCSP.clInit = awsCSP // self-reference
	csp.RegisterCSP(awsCSP)
}

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

var awsAttributes = map[string]models.AttributeDescriptor{
	AttrAccessKeyID:      models.AttributeDescriptor{Kind: "STRING", Description: "An AWS account access key"},
	AttrSecretAccessKey:  models.AttributeDescriptor{Kind: "SECRET", Description: "An AWS account secret key"},
	AttrRegion:           models.AttributeDescriptor{Kind: "STRING", Description: "The AWS region", Immutable: true},
	AttrAvailabilityZone: models.AttributeDescriptor{Kind: "STRING", Description: "The AWS availability zone", Immutable: true},
	AttrPStoreBucketName: models.AttributeDescriptor{Kind: "STRING", Description: "The AWS bucket name used for persistent storage", Optional: true, Immutable: true},
}

var credentialAttributesNames = []string{AttrAccessKeyID, AttrSecretAccessKey}
var domainAttributesNames = []string{AttrRegion, AttrAvailabilityZone, AttrPStoreBucketName}

// Attributes satisfies CloudServiceProvider
func (c *CSP) Attributes() map[string]models.AttributeDescriptor {
	return awsAttributes
}

// CredentialAttributes satisfies CloudServiceProvider
func (c *CSP) CredentialAttributes() map[string]models.AttributeDescriptor {
	credAttributes := make(map[string]models.AttributeDescriptor, len(credentialAttributesNames))
	for _, attr := range credentialAttributesNames {
		credAttributes[attr] = awsAttributes[attr]
	}
	return credAttributes
}

// DomainAttributes satisfies CloudServiceProvider
func (c *CSP) DomainAttributes() map[string]models.AttributeDescriptor {
	domAttributes := make(map[string]models.AttributeDescriptor, len(domainAttributesNames))
	for _, attr := range domainAttributesNames {
		domAttributes[attr] = awsAttributes[attr]
	}
	return domAttributes
}

// SupportedCspStorageTypes satisfies CloudServiceProvider
func (c *CSP) SupportedCspStorageTypes() []*models.CSPStorageType {
	return awsCspStorageTypes
}

// GetDeviceTypeByCspStorageType returns device type (SSD or HDD) for the given CspStorageType
func (c *CSP) GetDeviceTypeByCspStorageType(storageType models.CspStorageType) (string, error) {
	for _, st := range awsCspStorageTypes {
		if storageType == st.Name {
			return st.DeviceType, nil
		}
	}
	return "", fmt.Errorf("incorrect storageType %s", storageType)
}

// ProtectionStoreUploadTransferRate satisfies CloudServiceProvider
func (c *CSP) ProtectionStoreUploadTransferRate() int32 {
	return S3UploadTransferRate
}

// LocalInstanceMetadata satisfies the LocalInstanceDiscover interface
func (c *CSP) LocalInstanceMetadata() (map[string]string, error) {
	if c.IMDTimeout == 0 {
		c.LocalInstanceMetadataSetTimeout(awsIMDDefaultTimeoutSecs)
	}
	res := make(map[string]string)
	for _, p := range awsIMDProps {
		v, err := c.imdFetch(p)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch AWS instance meta-data: %w", err)
		}
		if n, ok := awsIMDRenamedProps[p]; ok {
			c.dbgF("%s(%s): %v\n", n, p, v)
			res[n] = v
		} else {
			c.dbgF("%s: %v\n", p, v)
			res[p] = v
		}
	}
	return res, nil
}

// LocalInstanceMetadataSetTimeout satisfies the LocalInstanceDiscover interface
func (c *CSP) LocalInstanceMetadataSetTimeout(secs int) {
	c.IMDTimeout, _ = time.ParseDuration(fmt.Sprintf("%ds", secs))
}

func (c *CSP) imdFetch(path string) (string, error) {
	http.DefaultClient.Timeout = c.IMDTimeout
	res, err := http.Get(awsIPProto + awsBasePath + path)
	if err != nil {
		return "", err
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// LocalInstanceDeviceName satisfies the LocalInstanceDiscover interface
func (c *CSP) LocalInstanceDeviceName(VolumeIdentifier, AttachedNodeDevice string) (string, error) {
	switch svc, vid, _ := VolumeIdentifierParse(VolumeIdentifier); svc {
	case ServiceEC2:
		if name, err := c.findLocalNVMeDeviceByID(vid); err != nil {
			return "", err
		} else if name != "" {
			return filepath.Join("/dev", name), nil
		} else if !c.hasNVMeStorage() {
			if strings.HasPrefix(AttachedNodeDevice, "/dev/sd") {
				// documented remapping on HVM instances
				AttachedNodeDevice = "/dev/xv" + AttachedNodeDevice[6:]
			}
			return AttachedNodeDevice, nil
		}
		return "", fmt.Errorf("not found: %s", VolumeIdentifier)
	}
	return "", fmt.Errorf("storage type currently unsupported")
}

// loadLocalDevices primes the local device cache, if necessary.
// On instances without NVMe storage, this creates an empty cache.
func (c *CSP) loadLocalDevices(reloadEBS bool) error {
	c.mux.Lock()
	defer c.mux.Unlock()
	return c.loadLocalDevicesLocked(reloadEBS)
}

type readDirFn func(dirName string) ([]os.FileInfo, error)

// readDirHook can be replaced during UT
var readDirHook readDirFn = ioutil.ReadDir

// loadLocalDevicesLocked must be called with mux locked
func (c *CSP) loadLocalDevicesLocked(reloadEBS bool) error {
	if c.devCache != nil && len(c.devCache) == 0 {
		// an empty cache means this was called previously and discovered no NVMe devices
		return nil
	}
	if c.devCache == nil {
		c.devCache = map[string]*InstanceDevice{}
	}
	if reloadEBS {
		for _, name := range util.StringKeys(c.devCache) {
			if !c.devCache[name].EphemeralDevice {
				delete(c.devCache, name)
			}
		}
	}
	devList, err := readDirHook("/dev")
	if err == nil {
		// on EC2, all storage, including the AMI is either NVMe or not, there no combination of the two
		for _, info := range devList {
			if name := info.Name(); strings.HasPrefix(name, "nvme") {
				if strings.HasSuffix(name, "n1") {
					if dev, ok := c.devCache[name]; !ok {
						if dev, err = c.identifyNVMeDevice(name); err != nil {
							c.dbgF("Failed to identify device %s: %v", name, err)
							if len(c.devCache) == 0 { // reset to nil to signify cache is not loaded
								c.devCache = nil
							}
							return err
						}
						c.devCache[name] = dev
					}
				}
			}
		}
		if len(c.devCache) > 0 {
			c.dbgF("Found NVMe devices: %v", util.SortedStringKeys(c.devCache))
		} else {
			c.dbg("Instance has no NVMe devices")
		}
		return nil
	}
	c.dbgF("Failed to list device names: %v", err)
	if len(c.devCache) == 0 { // reset to nil to signify cache is not loaded
		c.devCache = nil
	}
	return err
}

// NVMeDeviceIdentify represents the required subset of the output of nvme id-ctrl -o json
type NVMeDeviceIdentify struct {
	SerialNumber string `json:"sn"`
	ModelName    string `json:"mn"`
}

// identifyNVMeDevice determines device identity for a single NVMe device
func (c *CSP) identifyNVMeDevice(devName string) (*InstanceDevice, error) {
	// "nvme id-ctrl devPath -o json" to identify the device
	ctx := context.Background()
	output, err := c.exec.CommandContext(ctx, "nvme", "id-ctrl", filepath.Join("/dev", devName), "-o", "json").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("nvme id-ctrl %s failed: %w", devName, err)
	}
	res := &NVMeDeviceIdentify{}
	if err = json.Unmarshal(output, res); err != nil {
		return nil, fmt.Errorf("nvme id-ctrl %s output error: %w", devName, err)
	}
	if res.ModelName == awsNVMeEBS && strings.HasPrefix(res.SerialNumber, "vol") {
		// Actual AWS Volume ID has a "-" after the "vol" prefix, but the value in the SerialNumber does not: add the dash so we can compare
		return &InstanceDevice{EphemeralDevice: false, VolumeIdentifier: res.SerialNumber[:3] + "-" + res.SerialNumber[3:]}, nil
	} else if res.ModelName == awsNVMeEphemeral {
		return &InstanceDevice{EphemeralDevice: true}, nil
	}
	return nil, fmt.Errorf("nvme id-ctrl %s: unsupported mn[%s] sn[%s]", devName, res.ModelName, res.SerialNumber)
}

// Find device by AWS EBS volume identifier, returns the device name, empty if not found.
// Always returns empty string when hasNVMeStorage() returns false.
func (c *CSP) findLocalNVMeDeviceByID(VolumeIdentifier string) (string, error) {
	c.mux.Lock()
	defer c.mux.Unlock()
	for pass := 1; pass <= 3; pass++ {
		for name, dev := range c.devCache {
			if dev.VolumeIdentifier == VolumeIdentifier {
				return name, nil
			}
		}
		switch pass {
		case 1: // add missing devices to the cache
			if err := c.loadLocalDevicesLocked(false); err != nil {
				return "", err
			}
		case 2: // assume cache is no longer valid, reload
			if err := c.loadLocalDevicesLocked(true); err != nil {
				return "", err
			}
		}
	}
	return "", nil
}

// hasNVMeStorage tests if NVMe storage is present, only valid after loadLocalDevices called
func (c *CSP) hasNVMeStorage() bool {
	c.mux.Lock()
	defer c.mux.Unlock()
	return len(c.devCache) > 0
}

// isLocalDeviceEphemeral tests if the given device (simple name, no /dev/ prefix) is an ephemeral device.
// Only valid after loadLocalDevices called.
func (c *CSP) isLocalNVMeDeviceEphemeral(devName string) bool {
	c.mux.Lock()
	defer c.mux.Unlock()
	dev, ok := c.devCache[devName]
	return ok && dev.EphemeralDevice
}

// remove device (simple name, no /dev/ prefix) from the cache, eg on DETACH
func (c *CSP) removeLocalDevice(devName string) {
	c.mux.Lock()
	defer c.mux.Unlock()
	delete(c.devCache, devName)
}

// InDomain checks that the availability zone matches.
func (c *CSP) InDomain(cspDomainAttrs map[string]models.ValueType, imd map[string]string) error {
	var cspDomainZone, imdZone string
	attrsOK := false
	if vt, ok := cspDomainAttrs[AttrAvailabilityZone]; ok && vt.Kind == "STRING" {
		cspDomainZone = vt.Value
		if s, ok := imd[csp.IMDZone]; ok {
			imdZone = s
			attrsOK = true
		}
	}
	if !attrsOK {
		return fmt.Errorf("missing or invalid domain attribute %s or instance attribute %s", AttrAvailabilityZone, csp.IMDZone)
	}
	if cspDomainZone != imdZone {
		return fmt.Errorf("availability zone does not match (domain %s=%s, instance %s=%s)", AttrAvailabilityZone, cspDomainZone, csp.IMDZone, imdZone)
	}
	return nil
}

// SupportedStorageFormulas returns the storage formulas for AWS
func (c *CSP) SupportedStorageFormulas() []*models.StorageFormula {
	return awsStorageFormulas
}

// SanitizedAttributes takes input CSPDomain attributes, sanitizes and returns them.
func (c *CSP) SanitizedAttributes(cspDomainAttrs map[string]models.ValueType) (map[string]models.ValueType, error) {
	if val, ok := cspDomainAttrs[AttrPStoreBucketName]; ok && val.Value != "" {
		if val.Kind != "STRING" {
			return nil, fmt.Errorf("persistent store bucket name must be a string")
		}
	} else {
		cspDomainAttrs[AttrPStoreBucketName] = models.ValueType{Kind: "STRING", Value: fmt.Sprintf(protectionStoreNameFormat, uuidGenerator())}
	}
	if CompatibleBucketName(cspDomainAttrs[AttrPStoreBucketName].Value) {
		return cspDomainAttrs, nil
	}
	return nil, fmt.Errorf(awsInvalidBucketNameMsg)
}

var reDomain = regexp.MustCompile(`^[a-z0-9][a-z0-9\.\-]{1,61}[a-z0-9]$`)
var reIPAddress = regexp.MustCompile(`^(\d+\.){3}\d+$`)

// CompatibleBucketName returns true if the bucket name is DNS compatible.
// Buckets created outside of the classic region MUST be DNS compatible.
// Based of similar pattern found at https://github.com/aws/aws-sdk-go/blob/master/service/s3/host_style_bucket.go
func CompatibleBucketName(bucket string) bool {
	return reDomain.MatchString(bucket) &&
		!reIPAddress.MatchString(bucket) &&
		!strings.Contains(bucket, "..") &&
		!strings.Contains(bucket, ".-") &&
		!strings.Contains(bucket, "-.")
}

// Indirection for UUID generation to support UT
type uuidGen func() string

var uuidGenerator uuidGen = func() string { return uuid.NewV4().String() }
