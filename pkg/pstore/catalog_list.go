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
	"github.com/Nuvoloso/kontroller/pkg/endpoint"
)

// SnapshotCatalogListArgs contains arguments to the SnapshotCatalogList method
type SnapshotCatalogListArgs struct {
	// The protection store in which the snapshot catalog is maintained
	PStore *ProtectionStoreDescriptor
	// The name of the encryption algorithm ("NONE" if not encrypted)
	EncryptionAlgorithm string
	// The passphrase needed to generate the key (empty if not encrypted)
	Passphrase string
	// The protection domain object id
	ProtectionDomainID string
}

// Validate checks the arguments for correctness
func (a *SnapshotCatalogListArgs) Validate() bool {
	if a.PStore == nil || !a.PStore.Validate() || a.EncryptionAlgorithm == "" ||
		(a.EncryptionAlgorithm != common.EncryptionNone && a.Passphrase == "") || a.ProtectionDomainID == "" {
		return false
	}
	return true
}

// SnapshotCatalogIterator is used to fetch snapshot catalog entries
type SnapshotCatalogIterator interface {
	// Next returns the next catalog entry or an error on failure
	// If the iterator is exhausted or closed it returns (nil, nil)
	Next(ctx context.Context) (*SnapshotCatalogEntry, error)
	// Close is used to prematurely terminate the iterator
	Close()
}

// SnapshotCatalogLister is implements a catalog lister intance
type SnapshotCatalogLister struct {
	ep    endpoint.EndPoint
	token interface{}
}

// SnapshotCatalogList returns an iterator through which to retrieve the snapshot catalog entries in
// the specified protection store and protection domain
func (c *Controller) SnapshotCatalogList(ctx context.Context, args *SnapshotCatalogListArgs) (SnapshotCatalogIterator, error) {
	if !args.Validate() {
		return nil, ErrInvalidArguments
	}
	// Build the endpoint arguments
	epArgs := endpoint.Arg{Purpose: "Manipulation"}

	switch args.PStore.CspDomainType {
	case aws.CSPDomainType:
		epArgs.Type = "S3"
		epArgs.Args.S3.BucketName = args.PStore.CspDomainAttributes[aws.AttrPStoreBucketName].Value
		epArgs.Args.S3.Region = args.PStore.CspDomainAttributes[aws.AttrRegion].Value
		epArgs.Args.S3.AccessKeyID = args.PStore.CspDomainAttributes[aws.AttrAccessKeyID].Value
		epArgs.Args.S3.SecretAccessKey = args.PStore.CspDomainAttributes[aws.AttrSecretAccessKey].Value
		epArgs.Args.S3.PassPhrase = args.Passphrase
		epArgs.Args.S3.Domain = args.ProtectionDomainID
	}

	// Setup the endpoint
	ep, err := setupEndpoint(&epArgs, nil, 0)
	if err != nil {
		return nil, errors.New("SetupEndpoint: " + err.Error())
	}

	iter := &SnapshotCatalogLister{ep: ep, token: nil}

	return iter, nil

}

// Next is the function to get the next item from the iterator
func (sci *SnapshotCatalogLister) Next(ctx context.Context) (*SnapshotCatalogEntry, error) {
	name, token, err := sci.ep.GetListNext(sci.token)
	if err != nil {
		return nil, errors.New("Error: endpoint iterator: " + err.Error())
	}

	sci.token = token

	if name == "" {
		return nil, nil
	}
	si := &SnapshotCatalogEntry{}
	err = endpoint.GetJSONCatalog(sci.ep, name, si)
	if err != nil {
		return nil, errors.New("Error: Retreiving snapshot information: " + err.Error())
	}

	return si, nil
}

// Close tells the iterator you are done
func (sci *SnapshotCatalogLister) Close() {
	// Nothing needed
}
