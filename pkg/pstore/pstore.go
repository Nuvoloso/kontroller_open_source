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


package pstore

import (
	"context"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/csp/aws"
	"github.com/Nuvoloso/kontroller/pkg/csp/azure"
	"github.com/Nuvoloso/kontroller/pkg/csp/gc"
	"github.com/Nuvoloso/kontroller/pkg/endpoint"
	"github.com/Nuvoloso/kontroller/pkg/nuvoapi"
	"github.com/Nuvoloso/kontroller/pkg/rei"
	"github.com/Nuvoloso/kontroller/pkg/util"
	logging "github.com/op/go-logging"
)

const (
	numBackupThreads  = uint(1)
	numRestoreThreads = uint(20)
)

// The location of the copy executable
var copyLocation = "/opt/nuvoloso/bin/copy"

// ProtectionStoreDescriptor describes a protection store
type ProtectionStoreDescriptor struct {
	// CspDomainType is the type of cloud service provider.
	CspDomainType string
	// CspDomainAttributes contains connectivity and identification properties specific to the type of CSP.
	CspDomainAttributes map[string]models.ValueType
}

// Validate checks the arguments for correctness for use in snapshot operations
func (psd *ProtectionStoreDescriptor) Validate() bool {
	attrs := psd.CspDomainAttributes
	switch psd.CspDomainType {
	case aws.CSPDomainType:
		bn := util.ParseValueType(attrs[aws.AttrPStoreBucketName], common.ValueTypeString, "")
		rg := util.ParseValueType(attrs[aws.AttrRegion], common.ValueTypeString, "")
		ak := util.ParseValueType(attrs[aws.AttrAccessKeyID], common.ValueTypeString, "")
		sk := util.ParseValueType(attrs[aws.AttrSecretAccessKey], common.ValueTypeSecret, "")
		if bn == "" || rg == "" || ak == "" || sk == "" {
			return false
		}
	case azure.CSPDomainType:
		// Pass for now to allow tests to pass
		// TODO Azure
		return true
	case gc.CSPDomainType:
		cred := util.ParseValueType(attrs[gc.AttrCred], common.ValueTypeSecret, "")
		bn := util.ParseValueType(attrs[gc.AttrPStoreBucketName], common.ValueTypeString, "")
		if bn == "" || cred == "" {
			return false
		}
	default:
		return false
	}
	return true
}

// ValidateForSnapshotCatalogEntry validates a ProtectionStoreDescriptor for use in snapshot catalog entries
func (psd *ProtectionStoreDescriptor) ValidateForSnapshotCatalogEntry() bool {
	attrs := psd.CspDomainAttributes
	switch psd.CspDomainType {
	case aws.CSPDomainType:
		bn := util.ParseValueType(attrs[aws.AttrPStoreBucketName], common.ValueTypeString, "")
		rg := util.ParseValueType(attrs[aws.AttrRegion], common.ValueTypeString, "")
		if bn == "" || rg == "" {
			return false
		}
		ak := util.ParseValueType(attrs[aws.AttrAccessKeyID], common.ValueTypeString, "")
		sk := util.ParseValueType(attrs[aws.AttrSecretAccessKey], common.ValueTypeSecret, "")
		if ak != "" || sk != "" {
			return false // no credential information
		}
	case azure.CSPDomainType:
		// TODO Azure pass until implemented
		return true
	case gc.CSPDomainType:
		cred := util.ParseValueType(attrs[gc.AttrCred], common.ValueTypeSecret, "")
		bn := util.ParseValueType(attrs[gc.AttrPStoreBucketName], common.ValueTypeString, "")
		if cred == "" || bn == "" {
			return false
		}
	default:
		return false
	}
	return true
}

// Initialize pulls the desired properties from a CSPDomain object
func (psd *ProtectionStoreDescriptor) Initialize(cspObj *models.CSPDomain) {
	psd.CspDomainType = string(cspObj.CspDomainType)
	attrs := cspObj.CspDomainAttributes
	switch cspObj.CspDomainType {
	case aws.CSPDomainType:
		psd.CspDomainAttributes = make(map[string]models.ValueType)
		psd.CspDomainAttributes[aws.AttrAccessKeyID] = attrs[aws.AttrAccessKeyID]
		psd.CspDomainAttributes[aws.AttrSecretAccessKey] = attrs[aws.AttrSecretAccessKey]
		psd.CspDomainAttributes[aws.AttrPStoreBucketName] = attrs[aws.AttrPStoreBucketName]
		psd.CspDomainAttributes[aws.AttrRegion] = attrs[aws.AttrRegion]
	case azure.CSPDomainType:
		// TODO Azure
	case gc.CSPDomainType:
		psd.CspDomainAttributes = make(map[string]models.ValueType)
		psd.CspDomainAttributes[gc.AttrCred] = attrs[gc.AttrCred]
		psd.CspDomainAttributes[gc.AttrPStoreBucketName] = attrs[gc.AttrPStoreBucketName]
	}
}

// InitializeForSnapshotCatalogEntry pulls the desired properties from a CSPDomain object
func (psd *ProtectionStoreDescriptor) InitializeForSnapshotCatalogEntry(cspObj *models.CSPDomain) {
	psd.CspDomainType = string(cspObj.CspDomainType)
	attrs := cspObj.CspDomainAttributes
	switch cspObj.CspDomainType {
	case aws.CSPDomainType:
		psd.CspDomainAttributes = make(map[string]models.ValueType)
		psd.CspDomainAttributes[aws.AttrPStoreBucketName] = attrs[aws.AttrPStoreBucketName]
		psd.CspDomainAttributes[aws.AttrRegion] = attrs[aws.AttrRegion]
	case azure.CSPDomainType:
		// TODO Azure
	case gc.CSPDomainType:
		psd.CspDomainAttributes = make(map[string]models.ValueType)
		psd.CspDomainAttributes[gc.AttrCred] = attrs[gc.AttrCred]
		psd.CspDomainAttributes[gc.AttrPStoreBucketName] = attrs[gc.AttrPStoreBucketName]
	}
}

// SnapshotCatalogEntry contains metadata about a snapshot
// There is sufficient data in this record to recover the snapshot data from its
// protection store using the SnapshotRestore() operation, provided the invoker
// also knows the passphrase of the snapshot protection domain and has a credential
// to the snapshot's protection store.
type SnapshotCatalogEntry struct {
	SnapIdentifier       string // unique identifier for the entry
	SnapTime             time.Time
	SizeBytes            int64
	AccountID            string
	AccountName          string
	TenantAccountName    string
	VolumeSeriesID       string
	VolumeSeriesName     string
	ProtectionDomainID   string
	ProtectionDomainName string
	EncryptionAlgorithm  string
	ProtectionStores     []ProtectionStoreDescriptor // no credential information in the descriptors
	// additional useful properties valid at the time the record is inserted or replaced
	ConsistencyGroupID   string
	ConsistencyGroupName string
	VolumeSeriesTags     []string
	SnapshotTags         []string
}

// Validate checks the data structure for correctness during upsert
func (sce *SnapshotCatalogEntry) Validate() bool {
	if sce.SnapIdentifier == "" || sce.SizeBytes < 0 || sce.SnapTime.IsZero() ||
		sce.AccountID == "" || sce.AccountName == "" ||
		sce.VolumeSeriesID == "" || sce.VolumeSeriesName == "" ||
		sce.ProtectionDomainID == "" || sce.ProtectionDomainName == "" ||
		sce.EncryptionAlgorithm == "" || len(sce.ProtectionStores) == 0 ||
		sce.ConsistencyGroupID == "" || sce.ConsistencyGroupName == "" {
		return false
	}
	for _, ps := range sce.ProtectionStores {
		if !ps.ValidateForSnapshotCatalogEntry() {
			return false
		}
	}
	return true
}

// SnapshotCatalogEntrySourceObjects contains objects that source the data in the record
type SnapshotCatalogEntrySourceObjects struct {
	Snapshot         *models.Snapshot
	Account          *models.Account
	TenantAccount    *models.Account // optional
	VolumeSeries     *models.VolumeSeries
	ProtectionDomain *models.ProtectionDomain
	ProtectionStores []*models.CSPDomain
	ConsistencyGroup *models.ConsistencyGroup
}

// Initialize copies properties from the source objects
func (sce *SnapshotCatalogEntry) Initialize(src *SnapshotCatalogEntrySourceObjects) {
	sce.SnapIdentifier = src.Snapshot.SnapIdentifier
	sce.SnapTime = time.Time(src.Snapshot.SnapTime)
	sce.SizeBytes = src.Snapshot.SizeBytes
	sce.AccountID = string(src.Account.Meta.ID)
	sce.AccountName = string(src.Account.Name)
	if src.TenantAccount != nil {
		sce.TenantAccountName = string(src.TenantAccount.Name)
	}
	sce.VolumeSeriesID = string(src.VolumeSeries.Meta.ID)
	sce.VolumeSeriesName = string(src.VolumeSeries.Name)
	sce.ProtectionDomainID = string(src.ProtectionDomain.Meta.ID)
	sce.ProtectionDomainName = string(src.ProtectionDomain.Name)
	sce.EncryptionAlgorithm = src.ProtectionDomain.EncryptionAlgorithm
	sce.ProtectionStores = make([]ProtectionStoreDescriptor, len(src.ProtectionStores))
	for i, ps := range src.ProtectionStores {
		sce.ProtectionStores[i].InitializeForSnapshotCatalogEntry(ps)
	}
	sce.ConsistencyGroupID = string(src.ConsistencyGroup.Meta.ID)
	sce.ConsistencyGroupName = string(src.ConsistencyGroup.Name)
	sce.VolumeSeriesTags = make([]string, len(src.VolumeSeries.Tags))
	for i, tag := range src.VolumeSeries.Tags {
		sce.VolumeSeriesTags[i] = tag
	}
	sce.SnapshotTags = make([]string, len(src.Snapshot.Tags))
	for i, tag := range src.Snapshot.Tags {
		sce.SnapshotTags[i] = tag
	}
}

// Error extends error and is returned by methods of the Operations interface
type Error interface {
	error
	IsRetryable() bool
}

// errorDesc is used for errors returned by methods of the Operations interface
type errorDesc struct {
	description string
	retryable   bool
}

var _ = Error(&errorDesc{})

// Error implements the error interface
func (e *errorDesc) Error() string {
	return e.description
}

// IsRetryable indicates that the failed operation may succeed if retried with the same parameters
func (e *errorDesc) IsRetryable() bool {
	return e.retryable
}

// NewFatalError returns an error for an Error with IsRetryable() false
func NewFatalError(msg string) error {
	return &errorDesc{description: msg}
}

// NewRetryableError returns an error for an Error with IsRetryable() true
func NewRetryableError(msg string) error {
	return &errorDesc{description: msg, retryable: true}
}

// ErrInvalidArguments returned on invalid arguments
var ErrInvalidArguments = &errorDesc{description: "invalid arguments"}

// Operations is an interface providing external services involving protection stores
type Operations interface {
	// Snapshot operations
	SnapshotBackup(ctx context.Context, args *SnapshotBackupArgs) (*SnapshotBackupResult, error)
	SnapshotDelete(ctx context.Context, args *SnapshotDeleteArgs) (*SnapshotDeleteResult, error)
	SnapshotRestore(ctx context.Context, args *SnapshotRestoreArgs) (*SnapshotRestoreResult, error)
	// Catalog operations
	SnapshotCatalogList(ctx context.Context, args *SnapshotCatalogListArgs) (SnapshotCatalogIterator, error)
	SnapshotCatalogUpsert(ctx context.Context, args *SnapshotCatalogUpsertArgs) (*SnapshotUpsertResult, error)
	SnapshotCatalogGet(ctx context.Context, args *SnapshotCatalogGetArgs) (*SnapshotCatalogEntry, error)
	// TBD management methods
}

// InternalOps is an interface for internal operations
type InternalOps interface {
	ReadResult(resFileName string, res interface{}) error
	SpawnCopy(ctx context.Context, ID string, args *endpoint.CopyArgs, res interface{}) error
}

// CopyProgressReporter is an interface to report progress
type CopyProgressReporter interface {
	CopyProgress(ctx context.Context, cpr CopyProgressReport)
}

// CopyProgressReport provides progress information for copy operations
type CopyProgressReport struct {
	// a numeric representation of the total work involved
	TotalBytes int64
	// the number of reads done on the source of the copy
	SrcReadCount int64
	// a numeric representation of the current position in the pending work, with value in the range [0, TotalBytes]
	OffsetBytes int64
	// a numeric representation of the cumulative effort so far - it could serve as a "liveness" indicator
	TransferredBytes int64
	// percentage complete if computable
	PercentComplete float64
}

// progressHolder a interface for something that can generate a progress report
type progressHolder interface {
	progressReport() *CopyProgressReport
}

// ControllerArgs contains the properties required to create a Controller
type ControllerArgs struct {
	// The nuvo api (only if the data management layer is present locally)
	NuvoAPI nuvoapi.NuvoVM
	// Logging interface
	Log *logging.Logger
	// Enable runtime error injection
	DebugREI bool
}

// Validate checks the arguments for correctness
func (ca *ControllerArgs) Validate() bool {
	if ca.Log == nil {
		return false
	}
	return true
}

// Controller implements the Operations interface
type Controller struct {
	ControllerArgs
	// internal attributes
	intOps InternalOps
	rei    *rei.EphemeralPropertyManager
}

// NewController returns an Operations interface
func NewController(args *ControllerArgs) (Operations, error) {
	if !args.Validate() {
		return nil, ErrInvalidArguments
	}
	c := &Controller{
		ControllerArgs: *args,
	}
	c.intOps = c // self-reference
	c.rei = rei.NewEPM("pstore")
	c.rei.Enabled = c.DebugREI
	return c, nil
}
