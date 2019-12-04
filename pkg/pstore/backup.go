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

	"github.com/Nuvoloso/kontroller/pkg/csp/aws"
	"github.com/Nuvoloso/kontroller/pkg/csp/azure"
	"github.com/Nuvoloso/kontroller/pkg/csp/gc"
	"github.com/Nuvoloso/kontroller/pkg/endpoint"
)

// SnapshotBackupArgs contains arguments to the SnapshotBackup method.
type SnapshotBackupArgs struct {
	// The snapshot identifier
	SnapIdentifier string
	// An identifier for the invocation (only)
	ID string
	// The protection store to which the snapshot will be copied.
	PStore *ProtectionStoreDescriptor
	// Nuvo Socket Path Name
	NuvoAPISocketPathName string
	// Pathname to the exported PiT LUN to be backed up.
	SourceFile string
	// UUID for the volume series object
	VsID string
	// The UUID of the nuvoloso volume
	NuvoVolumeIdentifier string
	// Previous SnapIdentifier against which to compute the difference, may not be set if a full back is desired.
	BaseSnapIdentifier string
	// Optional, previous PiT identifier against which to compute the difference, may not be set if a full back is desired.
	BasePiT string
	// Latest PiT identifier. This will become the snapshot identifier.
	IncrPiT string
	// The name of the encryption algorithm ("NONE" if not encrypted)
	EncryptionAlgorithm string
	// The passphrase needed to generate the key (empty if not encrypted)
	Passphrase string
	// The protection domain object id
	ProtectionDomainID string
	// Optional progress reporter callback interface.
	Reporter CopyProgressReporter
}

// Validate checks the arguments for correctness
func (sba *SnapshotBackupArgs) Validate() bool {
	if sba.SnapIdentifier == "" || sba.ID == "" || sba.VsID == "" || sba.NuvoVolumeIdentifier == "" || sba.PStore == nil || !sba.PStore.Validate() || sba.SourceFile == "" || sba.IncrPiT == "" || sba.ProtectionDomainID == "" {
		return false
	}
	if (sba.BaseSnapIdentifier == "" && sba.BasePiT != "") || (sba.BaseSnapIdentifier != "" && sba.BasePiT == "") { // both BaseSnapIdentifier and BasePiT are set together or not
		return false
	}
	return true
}

// SnapshotBackupResult returns the result of the SnapshotBackup method.
// Through the magic of json, these values get automatically sucked out
// of the results JSON which are all named in endpoint.statName*.
// SnapshotBackupResult is also used for periodic progress generation
type SnapshotBackupResult struct {
	Stats CopyStats
	Error CopyError
}

// progressReport constructs a progress from the result
func (sbr *SnapshotBackupResult) progressReport() *CopyProgressReport {
	return sbr.Stats.progressReport()
}

// SnapshotBackup copies a snapshot difference to a protection store.
// It is a synchronous call that will periodically invoke the optional CopyProgressReporter interface if specified.
func (c *Controller) SnapshotBackup(ctx context.Context, args *SnapshotBackupArgs) (*SnapshotBackupResult, error) {
	if !args.Validate() {
		return nil, ErrInvalidArguments
	}
	src := endpoint.AllArgs{}

	src.Nuvo.FileName = args.SourceFile
	src.Nuvo.VolumeUUID = args.NuvoVolumeIdentifier
	src.Nuvo.BaseSnapUUID = args.BasePiT
	src.Nuvo.IncrSnapUUID = args.IncrPiT
	src.Nuvo.NuvoSocket = args.NuvoAPISocketPathName

	dst := endpoint.AllArgs{}
	var dstType string
	switch args.PStore.CspDomainType {
	case aws.CSPDomainType:
		dstType = endpoint.TypeAWS
		dst.S3.BucketName = args.PStore.CspDomainAttributes[aws.AttrPStoreBucketName].Value
		dst.S3.Base = args.BaseSnapIdentifier
		dst.S3.Incr = args.SnapIdentifier
		dst.S3.Domain = args.ProtectionDomainID
		dst.S3.PassPhrase = args.Passphrase
		// AWS Unique
		dst.S3.Region = args.PStore.CspDomainAttributes[aws.AttrRegion].Value
		dst.S3.AccessKeyID = args.PStore.CspDomainAttributes[aws.AttrAccessKeyID].Value
		dst.S3.SecretAccessKey = args.PStore.CspDomainAttributes[aws.AttrSecretAccessKey].Value
	case gc.CSPDomainType:
		dstType = endpoint.TypeGoogle
		dst.Google.BucketName = args.PStore.CspDomainAttributes[gc.AttrPStoreBucketName].Value
		dst.Google.Base = args.BaseSnapIdentifier
		dst.Google.Incr = args.SnapIdentifier
		dst.Google.Domain = args.ProtectionDomainID
		dst.Google.PassPhrase = args.Passphrase
		// Google Unique
		dst.Google.Cred = args.PStore.CspDomainAttributes[gc.AttrCred].Value
	case azure.CSPDomainType:
		dstType = endpoint.TypeAzure
		dst.Azure.BucketName = "AzureBucket: backup.go"
		dst.Azure.Base = args.BaseSnapIdentifier
		dst.Azure.Incr = args.SnapIdentifier
		dst.Azure.Domain = args.ProtectionDomainID
		dst.Azure.PassPhrase = args.Passphrase
		// Azure Unique
		dst.Azure.StorageAccount = "StorageAccount: backup.go"
		dst.Azure.StorageAccessKey = "StorageAccessKey: backup.go"
	}

	copyArgs := endpoint.CopyArgs{
		SrcType: endpoint.TypeNuvo, SrcArgs: src,
		DstType: dstType, DstArgs: dst,
		ProgressFileName: "/tmp/backup_progress_" + args.ID,
		NumThreads:       numBackupThreads,
	}

	if args.Reporter == nil {
		args.Reporter = progressLogger{
			logger: c.Log,
			id:     args.ID,
		}
	}

	res := &SnapshotBackupResult{}

	progressTimer := c.progressStart(ctx, res, args.Reporter, copyArgs.ProgressFileName)
	defer progressTimer.Stop()

	if err := c.intOps.SpawnCopy(ctx, args.ID, &copyArgs, res); err != nil {
		return nil, err
	}
	return res, nil
}
