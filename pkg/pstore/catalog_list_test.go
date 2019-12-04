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
	"encoding/json"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/csp/aws"
	fakeep "github.com/Nuvoloso/kontroller/pkg/endpoint/fake"
	"github.com/stretchr/testify/assert"
)

func TestSnapshotCatalogListArgs(t *testing.T) {
	assert := assert.New(t)

	args := &SnapshotCatalogListArgs{
		PStore: &ProtectionStoreDescriptor{
			CspDomainType: aws.CSPDomainType,
			CspDomainAttributes: map[string]models.ValueType{
				aws.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: "bn"},
				aws.AttrRegion:           models.ValueType{Kind: "STRING", Value: "rg"},
				aws.AttrAccessKeyID:      models.ValueType{Kind: "STRING", Value: "ak"},
				aws.AttrSecretAccessKey:  models.ValueType{Kind: "SECRET", Value: "sk"},
			},
		},
		EncryptionAlgorithm: common.EncryptionAES256,
		Passphrase:          "a secret",
		ProtectionDomainID:  "pdID",
	}
	assert.True(args.Validate())

	tcs := []string{
		"ProtectionDomainID", "Passphrase", "EncryptionAlgorithm",
		"PStore", "nil PStore",
	}
	for _, f := range tcs {
		switch f {
		case "ProtectionDomainID":
			args.ProtectionDomainID = ""
		case "Passphrase":
			args.Passphrase = ""
		case "EncryptionAlgorithm":
			args.EncryptionAlgorithm = ""
		case "PStore":
			args.PStore.CspDomainType = ""
			assert.False(args.PStore.Validate())
		case "nil PStore":
			args.PStore = nil
		}
		assert.False(args.Validate(), "case: Args failure", f)

		setupEndpoint = fakeep.EndpointFactory

		c := &Controller{}
		res, err := c.SnapshotCatalogList(context.Background(), args)
		assert.Error(err)
		e, ok := err.(Error)
		assert.True(ok)
		assert.Equal(ErrInvalidArguments, e)
		assert.Nil(res)
	}
}

func TestSnapshotCatalogList(t *testing.T) {
	assert := assert.New(t)

	args := &SnapshotCatalogListArgs{
		PStore: &ProtectionStoreDescriptor{
			CspDomainType: aws.CSPDomainType,
			CspDomainAttributes: map[string]models.ValueType{
				aws.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: "bn"},
				aws.AttrRegion:           models.ValueType{Kind: "STRING", Value: "rg"},
				aws.AttrAccessKeyID:      models.ValueType{Kind: "STRING", Value: "ak"},
				aws.AttrSecretAccessKey:  models.ValueType{Kind: "SECRET", Value: "sk"},
			},
		},
		EncryptionAlgorithm: common.EncryptionAES256,
		Passphrase:          "a secret",
		ProtectionDomainID:  "pdID",
	}
	assert.True(args.Validate())
	setupEndpoint = fakeep.EndpointFactory
	fakeep.ObjectList = append(fakeep.ObjectList, "Object1")

	c := &Controller{}
	iter, err := c.SnapshotCatalogList(context.Background(), args)

	ctx := context.Background()
	for si, err := iter.Next(ctx); si != nil; si, err = iter.Next(ctx) {
		assert.NoError(err) // TBD
	}
	iter.Close()

	fakeep.FailSetup = true
	iter, err = c.SnapshotCatalogList(context.Background(), args)
	assert.Error(err)
	fakeep.FailSetup = false

	entry := SnapshotCatalogEntry{}
	b, _ := json.Marshal(entry)
	fakeep.DataToRetrieve = string(b)

	fakeep.ObjectList = append(fakeep.ObjectList, "Object1")

	fakeep.FailListNext = true
	iter, err = c.SnapshotCatalogList(context.Background(), args)
	_, err = iter.Next(ctx)
	assert.Error(err)
	fakeep.FailListNext = false

	fakeep.FailRetreive = true
	iter, err = c.SnapshotCatalogList(context.Background(), args)
	_, err = iter.Next(ctx)
	assert.Error(err)
	fakeep.FailRetreive = false
}
