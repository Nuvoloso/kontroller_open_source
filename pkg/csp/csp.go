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


package csp

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/op/go-logging"
)

// CloudServiceProvider abstraction provides operations on a type of CSP
type CloudServiceProvider interface {
	Type() models.CspDomainTypeMutable
	SetDebugLogger(log *logging.Logger)
	// Attributes returns a description of all possible CSP attributes
	Attributes() map[string]models.AttributeDescriptor
	// CredentialAttributes returns a description of all possible attributes specific to CSP credential only
	CredentialAttributes() map[string]models.AttributeDescriptor
	// DomainAttributes returns a description of all possible attributes specific to CSP domain only
	DomainAttributes() map[string]models.AttributeDescriptor
	SupportedCspStorageTypes() []*models.CSPStorageType
	// InDomain checks if an instance is in the domain described by the CSPDomain attributes set by the user.
	// It does so by comparing the CSPDomain attributes with the instance meta-data attributes.
	InDomain(cspDomainAttrs map[string]models.ValueType, imd map[string]string) error
	LocalInstanceDiscoverer
	// Client returns a client for the services available in a given CSPDomain instance.
	// Logging is inherited from the CSP
	Client(cspDomain *models.CSPDomain) (DomainClient, error)
	// ValidateCredential determines if the credentials are valid
	ValidateCredential(domType models.CspDomainTypeMutable, attrs map[string]models.ValueType) error
	// Returns a list of storage formulas for the CSP
	SupportedStorageFormulas() []*models.StorageFormula
	// Returns sanitized CSPDomain attributes
	SanitizedAttributes(cspDomainAttrs map[string]models.ValueType) (map[string]models.ValueType, error)
	// Provides an estimate of the data transfer rate for protection store upload
	ProtectionStoreUploadTransferRate() int32
	// GetDeviceTypeByCspStorageType returns device type (SSD or HDD) for the given CspStorageType
	GetDeviceTypeByCspStorageType(storageType models.CspStorageType) (string, error)
}

// DomainClient abstraction provides a client to invoke services tied to a CSPDomain.
type DomainClient interface {
	Type() models.CspDomainTypeMutable
	ID() models.ObjID
	SetTimeout(secs int)
	Validate(ctx context.Context) error
	CreateProtectionStore(ctx context.Context) (map[string]models.ValueType, error)
	EphemeralDiscoverer
	VolumeProvisioner
}

// EphemeralDiscoverer returns information regarding ephemeral devices on the executing instance.
// It may need to perform CSPDomain-scope operations to complete the discovery.
type EphemeralDiscoverer interface {
	LocalEphemeralDevices() ([]*EphemeralDevice, error)
}

// Constants related to ephemeral storage
const (
	// Well-known CSP Storage Type Attribute key for the ephemeral type
	CSPEphemeralStorageType = "cspEphemeralStorageType"

	// This is the currently supported ephemeral device types
	EphemeralTypeSSD = "SSD"
	EphemeralTypeHDD = "HDD"
)

// EphemeralDevice returns information about a single, locally-available device
type EphemeralDevice struct {
	Path        string // e.g. /dev/xvdb
	Type        string // device type
	Initialized bool   // If true, device is performance-optimized
	Usable      bool   // If true, device can be used by Nuvoloso (eg for cache)
	SizeBytes   int64  // raw, unformatted size
}

// This is a minimal list of properties that could be provided in instance meta-data.
// Individual CSP types could provide more - for example there could be multiple public IPS.
const (
	IMDHostname     = "Hostname"
	IMDInstanceName = "InstanceName"
	IMDLocalIP      = "LocalIP"
	IMDZone         = "Zone"
)

// IMDProvisioningPrefix is the prefix for IMD properties that should be provided during volume provisioning requests
// Such attributes convey values discoverable only at runtime in the target cluster
// to centrald where provisioning is performed.
// These attributes must be hoisted into the Cluster/Node object attributes if present.
const IMDProvisioningPrefix = "Provisioning-"

// LocalInstanceDiscoverer returns information on the executing instance
type LocalInstanceDiscoverer interface {
	LocalInstanceMetadata() (map[string]string, error)
	LocalInstanceMetadataSetTimeout(timeSec int)
	// LocalInstanceDeviceName takes the CSP volume identifier and the previous AttachedNodeDevice, return the current AttachedNodeDevice
	// The empty string can be returned, meaning the CSP volume is no longer attached
	LocalInstanceDeviceName(VolumeIdentifier, AttachedNodeDevice string) (string, error)
}

// VolumeProvisioner is an abstraction of volume management interfaces.
// It requires a call to ClientInit()
type VolumeProvisioner interface {
	VolumeAttach(ctx context.Context, vaa *VolumeAttachArgs) (*Volume, error)
	VolumeCreate(ctx context.Context, vca *VolumeCreateArgs) (*Volume, error)
	VolumeDelete(ctx context.Context, vda *VolumeDeleteArgs) error
	VolumeDetach(ctx context.Context, vda *VolumeDetachArgs) (*Volume, error)
	VolumeFetch(ctx context.Context, vfa *VolumeFetchArgs) (*Volume, error)
	VolumeList(ctx context.Context, vla *VolumeListArgs) ([]*Volume, error)
	VolumeTagsDelete(ctx context.Context, vta *VolumeTagArgs) (*Volume, error)
	VolumeTagsSet(ctx context.Context, vta *VolumeTagArgs) (*Volume, error)
	VolumeSize(ctx context.Context, stName models.CspStorageType, requestedSizeBytes int64) (int64, error)
}

// VolumeAttachArgs contains the parameters for VolumeAttach
type VolumeAttachArgs struct {
	// VolumeIdentifier contains the CSP domain identifier of the volume to be attached.
	VolumeIdentifier string
	// NodeIdentifier contains the CSP domain identifier of a node to which a volume is to be attached.
	NodeIdentifier string
	// ProvisioningAttributes supplement the domain (client) attributes
	ProvisioningAttributes map[string]models.ValueType
}

// VolumeCreateArgs contains the parameters for VolumeCreate
type VolumeCreateArgs struct {
	StorageTypeName models.CspStorageType
	SizeBytes       int64
	// Creation time volume tags of the form "key:value".
	// They are set atomically with the creation if supported by the domain.
	Tags []string
	// ProvisioningAttributes supplement the domain (client) attributes
	ProvisioningAttributes map[string]models.ValueType
}

// VolumeDeleteArgs contains input parameters for VolumeDelete
type VolumeDeleteArgs struct {
	VolumeIdentifier string
	// ProvisioningAttributes supplement the domain (client) attributes
	ProvisioningAttributes map[string]models.ValueType
}

// VolumeDetachArgs contains the parameters for VolumeDetach
type VolumeDetachArgs struct {
	// VolumeIdentifier contains the CSP domain identifier of the volume to be detached.
	VolumeIdentifier string
	// NodeIdentifier contains the CSP domain identifier of a node from which the volume is to be detached.
	NodeIdentifier string
	// NodeDevice is the OS specific name for the device through which the storage is accessed on the Node.
	NodeDevice string
	// Force lets you force a detach
	Force bool
	// ProvisioningAttributes supplement the domain (client) attributes
	ProvisioningAttributes map[string]models.ValueType
}

// VolumeFetchArgs contains input parameters for VolumeFetch
type VolumeFetchArgs struct {
	VolumeIdentifier string
	// ProvisioningAttributes supplement the domain (client) attributes
	ProvisioningAttributes map[string]models.ValueType
}

// VolumeListArgs contains search parameters for VolumeList.
type VolumeListArgs struct {
	// StorageTypeName specifies the type of storage. Optional.
	// If unspecified all node-attachable storage types will be considered.
	StorageTypeName models.CspStorageType
	// Tags specify a set of tags to match in volumes. Optional.
	// Tags are expressed in the form "key:value".
	//  - When specified here as just "key" then the existence of a tag with that key will be matched.
	//  - When specified here as "key:value" then both the key and value will be matched.
	Tags []string
	// ProvisioningAttributes supplement the domain (client) attributes
	ProvisioningAttributes map[string]models.ValueType
}

// VolumeTagArgs contains input parameters for VolumeTagsSet and VolumeTagsDelete
type VolumeTagArgs struct {
	VolumeIdentifier string
	// Tags specify a set of tags to set on (meaning add or update) or delete from a Volume. They are expressed in the form "key:value".
	Tags []string
	// ProvisioningAttributes supplement the domain (client) attributes
	ProvisioningAttributes map[string]models.ValueType
}

// Volume is a vendor neutral structure to return vendor specific volume data
type Volume struct {
	CSPDomainType     models.CspDomainTypeMutable
	StorageTypeName   models.CspStorageType
	Identifier        string // volume identifier
	Type              string // volume type
	SizeBytes         int64
	ProvisioningState VolumeProvisioningState
	Tags              []string // volume tags
	Attachments       []VolumeAttachment
	Raw               interface{} // vendor specific data
}

// VolumeProvisioningState enum type
type VolumeProvisioningState int

// VolumeProvisioningState values
const (
	VolumeProvisioningError VolumeProvisioningState = iota
	VolumeProvisioningUnprovisioned
	VolumeProvisioningProvisioning
	VolumeProvisioningProvisioned
	VolumeProvisioningUnprovisioning
)

var cspVPSMap = map[VolumeProvisioningState]string{
	VolumeProvisioningError:          "ERROR",
	VolumeProvisioningUnprovisioned:  "UNPROVISIONED",
	VolumeProvisioningProvisioning:   "PROVISIONING",
	VolumeProvisioningProvisioned:    "PROVISIONED",
	VolumeProvisioningUnprovisioning: "UNPROVISIONING",
}

func (vps VolumeProvisioningState) String() string {
	s, ok := cspVPSMap[vps]
	if ok {
		return s
	}
	s, _ = cspVPSMap[VolumeProvisioningError]
	return s
}

// VolumeAttachment returns the device to which a volume is attached
type VolumeAttachment struct {
	NodeIdentifier string // CSP domain identifier of the node (assume local if "")
	Device         string // OS device name
	State          VolumeAttachmentState
}

// VolumeAttachmentState enum type
type VolumeAttachmentState int

// VolumeAttachmentState values
const (
	VolumeAttachmentError VolumeAttachmentState = iota
	VolumeAttachmentAttaching
	VolumeAttachmentAttached
	VolumeAttachmentDetaching
	VolumeAttachmentDetached
)

var cspVASMap = map[VolumeAttachmentState]string{
	VolumeAttachmentError:     "ERROR",
	VolumeAttachmentAttaching: "ATTACHING",
	VolumeAttachmentAttached:  "ATTACHED",
	VolumeAttachmentDetaching: "DETACHING",
	VolumeAttachmentDetached:  "DETACHED",
}

func (vas VolumeAttachmentState) String() string {
	s, ok := cspVASMap[vas]
	if ok {
		return s
	}
	s, _ = cspVASMap[VolumeAttachmentError]
	return s
}

// ErrorVolumeNotAttached is returned when attempting to detach an already detached volume
var ErrorVolumeNotAttached = fmt.Errorf("VolumeNotAttached")

// ErrorVolumeNotFound is returned when a volume is not found
var ErrorVolumeNotFound = fmt.Errorf("VolumeNotFound")

// cspRegistry is a mapping of CSP domain type name to CloudServiceProvider
var cspRegistry = make(map[models.CspDomainTypeMutable]CloudServiceProvider)

// cspMutex is available for internal use in this package
var cspMutex sync.Mutex

// RegisterCSP registers a cloud service provider interface
func RegisterCSP(csp CloudServiceProvider) {
	cspMutex.Lock()
	defer cspMutex.Unlock()
	cspRegistry[csp.Type()] = csp
}

// NewCloudServiceProvider returns a CSP interface to a concrete type
func NewCloudServiceProvider(cspDomainType models.CspDomainTypeMutable) (CloudServiceProvider, error) {
	p, ok := cspRegistry[cspDomainType]
	if !ok {
		return nil, fmt.Errorf("unsupported CSPDomainType")
	}
	return p, nil
}

// SupportedCspDomainTypes returns a list of supported CSPDomain types
func SupportedCspDomainTypes() []models.CspDomainTypeMutable {
	dt := make([]models.CspDomainTypeMutable, len(cspRegistry))
	i := 0
	for k := range cspRegistry {
		dt[i] = k
		i++
	}
	return dt
}

// AttributeDescriptionMap is a collection of attribute descriptors that can be used for validation.
type AttributeDescriptionMap map[string]models.AttributeDescriptor

// ValidateExpected validates that the specified attributes are present and of a proper kind.
// Missing optional attributes are defaulted to the empty string if desired.
// In case of error it returns an error with the cause and the list of expected values.
func (desc AttributeDescriptionMap) ValidateExpected(expAttrs []string, attrs map[string]models.ValueType, supplyMissingOptional, ignoreExtra bool) error {
	numFound := 0
	numSpecified := len(attrs)
	var err error
	for _, n := range expAttrs {
		ad, ok := desc[n]
		if !ok {
			panic(fmt.Sprintf("attribute descriptor '%s' not found", n))
		}
		vt, ok := attrs[n]
		if !ok {
			if !ad.Optional {
				err = fmt.Errorf("missing required attribute '%s'", n)
				break
			}
			if supplyMissingOptional {
				attrs[n] = models.ValueType{Kind: ad.Kind} // default
			}
			continue
		}
		if vt.Kind != ad.Kind {
			err = fmt.Errorf("invalid Kind for '%s'", n)
			break
		}
		numFound++
	}
	if err == nil && numFound != numSpecified && !ignoreExtra {
		err = fmt.Errorf("extra attributes specified")
	}
	if err != nil {
		var rB, oB bytes.Buffer
		sort.Strings(expAttrs)
		for _, n := range expAttrs {
			ad := desc[n]
			b := &rB
			if ad.Optional {
				b = &oB
			}
			fmt.Fprintf(b, "%s[%s], ", n, common.ValueTypeKindAbbrev[ad.Kind])
		}
		var eB bytes.Buffer
		fmt.Fprintf(&eB, "%s:", err.Error())
		if rB.Len() > 0 {
			rB.Truncate((rB.Len() - 2))
			fmt.Fprintf(&eB, " Required: %s", rB.String())
		}
		if oB.Len() > 0 {
			oB.Truncate((oB.Len() - 2))
			fmt.Fprintf(&eB, " Optional: %s", oB.String())
		}
		err = fmt.Errorf("%s", eB.String())
	}
	return err
}
