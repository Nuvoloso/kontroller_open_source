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
	"errors"

	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/csp/aws"
	"github.com/Nuvoloso/kontroller/pkg/csp/azure"
	"github.com/Nuvoloso/kontroller/pkg/csp/gc"
	"github.com/Nuvoloso/kontroller/pkg/endpoint"
)

// SnapshotCatalogGetArgs contains arguments to the SnapshotCatalogGet method
type SnapshotCatalogGetArgs struct {
	// The protection store in which the snapshot catalog is maintained
	PStore *ProtectionStoreDescriptor
	// The name of the encryption algorithm ("NONE" if not encrypted)
	EncryptionAlgorithm string
	// The passphrase needed to generate the key (empty if not encrypted)
	Passphrase string
	// The protection domain object id
	ProtectionDomainID string
	// Snapshot Identifier to retrieve from the catalog
	SnapID string
}

// Validate checks the arguments for correctness
func (a *SnapshotCatalogGetArgs) Validate() bool {
	if a.PStore == nil || !a.PStore.Validate() || a.EncryptionAlgorithm == "" || a.SnapID == "" ||
		(a.EncryptionAlgorithm != common.EncryptionNone && a.Passphrase == "") || a.ProtectionDomainID == "" {
		return false
	}
	return true
}

// SnapshotCatalogGet returns a snapshot catalog entry from the catalog
func (c *Controller) SnapshotCatalogGet(ctx context.Context, args *SnapshotCatalogGetArgs) (*SnapshotCatalogEntry, error) {
	if !args.Validate() {
		return nil, ErrInvalidArguments
	}
	// Build the endpoint arguments
	epArgs := endpoint.Arg{Purpose: "Manipulation"}

	switch args.PStore.CspDomainType {
	case aws.CSPDomainType:
		epArgs.Type = endpoint.TypeAWS
		epArgs.Args.S3.BucketName = args.PStore.CspDomainAttributes[aws.AttrPStoreBucketName].Value
		epArgs.Args.S3.Domain = args.ProtectionDomainID
		epArgs.Args.S3.PassPhrase = args.Passphrase
		// AWS Unique
		epArgs.Args.S3.Region = args.PStore.CspDomainAttributes[aws.AttrRegion].Value
		epArgs.Args.S3.AccessKeyID = args.PStore.CspDomainAttributes[aws.AttrAccessKeyID].Value
		epArgs.Args.S3.SecretAccessKey = args.PStore.CspDomainAttributes[aws.AttrSecretAccessKey].Value
	case gc.CSPDomainType:
		epArgs.Type = endpoint.TypeGoogle
		epArgs.Args.Google.BucketName = args.PStore.CspDomainAttributes[gc.AttrPStoreBucketName].Value
		epArgs.Args.Google.Domain = args.ProtectionDomainID
		epArgs.Args.Google.PassPhrase = args.Passphrase
		// Google Unique
		epArgs.Args.Google.Cred = args.PStore.CspDomainAttributes[gc.AttrCred].Value
	case azure.CSPDomainType:
		epArgs.Type = endpoint.TypeAzure
		epArgs.Args.Azure.BucketName = "Azure bucket: catalog_get.go"
		epArgs.Args.Azure.Domain = args.ProtectionDomainID
		epArgs.Args.Azure.PassPhrase = args.Passphrase
		// Azure Unique
		epArgs.Args.Azure.StorageAccount = "StorageAccount: catalog.go"
		epArgs.Args.Azure.StorageAccessKey = "StorageAccessKey: catalog.go"
	}

	// Setup the endpoint
	ep, err := setupEndpoint(&epArgs, nil, 0)
	if err != nil {
		return nil, errors.New("SetupEndpoint: " + err.Error())
	}

	si := &SnapshotCatalogEntry{}
	err = endpoint.GetJSONCatalog(ep, args.SnapID, si)
	if err != nil {
		return nil, errors.New("Error: Retreiving snapshot information: " + err.Error())
	}

	return si, nil
}
