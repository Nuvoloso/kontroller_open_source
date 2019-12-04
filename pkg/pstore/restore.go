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

// SnapshotRestoreArgs contain arguments to the SnapshotRestore method.
type SnapshotRestoreArgs struct {
	// The snapshot identifier
	SnapIdentifier string
	// An identifier for the invocation (only)
	ID string
	// The protection store containing the snapshot to be restored.
	PStore *ProtectionStoreDescriptor
	// Snapshot identifier to restore.
	SourceSnapshot string
	// Pathname to the mounted volume being recreated/restored.
	DestFile string
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
func (sbr *SnapshotRestoreArgs) Validate() bool {
	if sbr.SnapIdentifier == "" || sbr.ID == "" || sbr.PStore == nil || !sbr.PStore.Validate() || sbr.SourceSnapshot == "" || sbr.DestFile == "" || sbr.ProtectionDomainID == "" {
		return false
	}
	return true
}

// SnapshotRestoreResult returns the result of the SnapshotRestore method
// and is also used for periodic progress generation.
type SnapshotRestoreResult struct {
	Stats CopyStats
	Error CopyError
}

// progressReport constructs a progress from the result
func (srr *SnapshotRestoreResult) progressReport() *CopyProgressReport {
	return srr.Stats.progressReport()
}

// SnapshotRestore restores a snapshot in a volume
// It is a synchronous call that will periodically invoke the optional CopyProgressReporter interface if specified.
func (c *Controller) SnapshotRestore(ctx context.Context, args *SnapshotRestoreArgs) (*SnapshotRestoreResult, error) {
	if !args.Validate() {
		return nil, ErrInvalidArguments
	}
	dst := endpoint.AllArgs{}
	dst.File.FileName = args.DestFile
	dst.File.DestPreZeroed = true
	src := endpoint.AllArgs{}
	var srcType string

	switch args.PStore.CspDomainType {
	case aws.CSPDomainType:
		srcType = endpoint.TypeAWS
		src.S3.BucketName = args.PStore.CspDomainAttributes[aws.AttrPStoreBucketName].Value
		src.S3.Base = ""
		src.S3.Incr = args.SnapIdentifier
		src.S3.Domain = args.ProtectionDomainID
		src.S3.PassPhrase = args.Passphrase
		// AWS Unique
		src.S3.Region = args.PStore.CspDomainAttributes[aws.AttrRegion].Value
		src.S3.AccessKeyID = args.PStore.CspDomainAttributes[aws.AttrAccessKeyID].Value
		src.S3.SecretAccessKey = args.PStore.CspDomainAttributes[aws.AttrSecretAccessKey].Value
	case gc.CSPDomainType:
		srcType = endpoint.TypeGoogle
		src.Google.BucketName = args.PStore.CspDomainAttributes[gc.AttrPStoreBucketName].Value
		src.Google.Base = ""
		src.Google.Incr = args.SnapIdentifier
		src.Google.Domain = args.ProtectionDomainID
		src.Google.PassPhrase = args.Passphrase
		// Google Unique
		src.Google.Cred = args.PStore.CspDomainAttributes[gc.AttrCred].Value
	case azure.CSPDomainType:
		srcType = endpoint.TypeAzure
		src.Azure.BucketName = "Azure bucket: restore.go"
		src.Azure.Base = ""
		src.Azure.Incr = args.SnapIdentifier
		src.Azure.Domain = args.ProtectionDomainID
		src.Azure.PassPhrase = args.Passphrase
		// Azure Unique
		src.Azure.StorageAccount = "StorageAccount: restore.go"
		src.Azure.StorageAccessKey = "StorageAccessKey: restore.go"
	}

	copyArgs := endpoint.CopyArgs{
		SrcType: srcType, SrcArgs: src,
		DstType: endpoint.TypeFile, DstArgs: dst,
		ProgressFileName: "/tmp/restore_progress_" + args.ID,
		NumThreads:       numRestoreThreads,
	}

	if args.Reporter == nil {
		args.Reporter = progressLogger{
			logger: c.Log,
			id:     args.ID,
		}
	}

	res := &SnapshotRestoreResult{}
	progressTimer := c.progressStart(ctx, res, args.Reporter, copyArgs.ProgressFileName)
	defer progressTimer.Stop()

	if err := c.intOps.SpawnCopy(ctx, args.ID, &copyArgs, res); err != nil {
		return nil, err
	}
	return res, nil
}
