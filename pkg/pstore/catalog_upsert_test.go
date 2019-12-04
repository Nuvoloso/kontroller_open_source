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
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/csp/aws"
	"github.com/Nuvoloso/kontroller/pkg/csp/azure"
	"github.com/Nuvoloso/kontroller/pkg/csp/gc"
	fakeep "github.com/Nuvoloso/kontroller/pkg/endpoint/fake"
	"github.com/stretchr/testify/assert"
)

func TestSnapshotCatalogUpsertArgs(t *testing.T) {
	assert := assert.New(t)

	psd := ProtectionStoreDescriptor{
		CspDomainType: aws.CSPDomainType,
		CspDomainAttributes: map[string]models.ValueType{
			aws.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: "bn"},
			aws.AttrRegion:           models.ValueType{Kind: "STRING", Value: "rg"},
		},
	}
	sce := SnapshotCatalogEntry{
		SnapIdentifier:       "snapId",
		SnapTime:             time.Now(),
		SizeBytes:            10,
		AccountID:            "accountID",
		AccountName:          "accountName",
		VolumeSeriesID:       "vsID",
		VolumeSeriesName:     "vsName",
		ProtectionDomainID:   "pdID",
		ProtectionDomainName: "pdName",
		EncryptionAlgorithm:  "NONE",
		ProtectionStores:     []ProtectionStoreDescriptor{psd},
		ConsistencyGroupID:   "cgID",
		ConsistencyGroupName: "cgName",
	}
	assert.True(sce.Validate())

	args := &SnapshotCatalogUpsertArgs{
		Entry: sce,
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
		"PStore", "nil PStore", "Entry",
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
		case "Entry":
			args.Entry.SnapIdentifier = ""
			assert.False(args.Entry.Validate())
		}
		assert.False(args.Validate(), "case: Args failure", f)

		c := &Controller{}
		res, err := c.SnapshotCatalogUpsert(context.Background(), args)
		assert.Error(err)
		e, ok := err.(Error)
		assert.True(ok)
		assert.Equal(ErrInvalidArguments, e)
		assert.Nil(res)
	}
}

func TestSnapshotCatalogUpsert(t *testing.T) {
	assert := assert.New(t)

	psd := ProtectionStoreDescriptor{
		CspDomainType: aws.CSPDomainType,
		CspDomainAttributes: map[string]models.ValueType{
			aws.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: "bn"},
			aws.AttrRegion:           models.ValueType{Kind: "STRING", Value: "rg"},
		},
	}
	sce := SnapshotCatalogEntry{
		SnapIdentifier:       "snapId",
		SnapTime:             time.Now(),
		SizeBytes:            10,
		AccountID:            "accountID",
		AccountName:          "accountName",
		VolumeSeriesID:       "vsID",
		VolumeSeriesName:     "vsName",
		ProtectionDomainID:   "pdID",
		ProtectionDomainName: "pdName",
		EncryptionAlgorithm:  "NONE",
		ProtectionStores:     []ProtectionStoreDescriptor{psd},
		ConsistencyGroupID:   "cgID",
		ConsistencyGroupName: "cgName",
	}
	args := &SnapshotCatalogUpsertArgs{
		Entry: sce,
		PStore: &ProtectionStoreDescriptor{
			CspDomainType: aws.CSPDomainType,
			CspDomainAttributes: map[string]models.ValueType{
				aws.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: "bn"},
				aws.AttrRegion:           models.ValueType{Kind: "STRING", Value: "rg"},
				aws.AttrAccessKeyID:      models.ValueType{Kind: "STRING", Value: "ak"},
				aws.AttrSecretAccessKey:  models.ValueType{Kind: "SECRET", Value: "sk"},
				gc.AttrPStoreBucketName:  models.ValueType{Kind: "STRING", Value: "bn"},
				gc.AttrCred:              models.ValueType{Kind: "SECRET", Value: "cred"},
			},
		},
		EncryptionAlgorithm: common.EncryptionAES256,
		Passphrase:          "a secret",
		ProtectionDomainID:  "pdID",
	}
	assert.True(args.Validate())

	c := &Controller{}

	setupEndpoint = fakeep.EndpointFactory

	CSPtypes := []string{gc.CSPDomainType, azure.CSPDomainType, aws.CSPDomainType}
	for _, args.PStore.CspDomainType = range CSPtypes {
		res, err := c.SnapshotCatalogUpsert(context.Background(), args)
		assert.NoError(err) // TBD
		assert.NotNil(res)  // TBD
	}

	fakeep.FailSetup = true
	res, err := c.SnapshotCatalogUpsert(context.Background(), args)
	assert.Error(err)
	assert.Nil(res)
	fakeep.FailSetup = false

	fakeep.FailStore = true
	res, err = c.SnapshotCatalogUpsert(context.Background(), args)
	assert.Error(err)
	assert.Nil(res)
	fakeep.FailStore = false

	fakeep.FailDone = true
	res, err = c.SnapshotCatalogUpsert(context.Background(), args)
	assert.Error(err)
	assert.Nil(res)
}

func TestSnapshotCatalogUpsertInitialize(t *testing.T) {
	assert := assert.New(t)

	sceSrc, expSCE := sceSourceData()
	expUA := &SnapshotCatalogUpsertArgs{
		Entry: *expSCE,
		PStore: &ProtectionStoreDescriptor{
			CspDomainType: aws.CSPDomainType,
			CspDomainAttributes: map[string]models.ValueType{
				aws.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: "cat-bucket"},
				aws.AttrRegion:           models.ValueType{Kind: "STRING", Value: "cat-region"},
				aws.AttrAccessKeyID:      models.ValueType{Kind: "STRING", Value: "cat-access-key"},
				aws.AttrSecretAccessKey:  models.ValueType{Kind: "SECRET", Value: "cat-secret-access-key"},
			},
		},
		EncryptionAlgorithm: "catEa",
		Passphrase:          "cat secret",
		ProtectionDomainID:  "catPdId",
	}

	// Any domain type will do here
	cspObj := &models.CSPDomain{}
	cspObj.Meta = &models.ObjMeta{ID: "domId"}
	cspObj.CspDomainType = models.CspDomainTypeMutable(aws.CSPDomainType)
	da := make(map[string]models.ValueType)
	da[aws.AttrPStoreBucketName] = models.ValueType{Value: "cat-bucket", Kind: common.ValueTypeString}
	da[aws.AttrRegion] = models.ValueType{Value: "cat-region", Kind: common.ValueTypeString}
	da[aws.AttrAccessKeyID] = models.ValueType{Value: "cat-access-key", Kind: common.ValueTypeString}
	da[aws.AttrSecretAccessKey] = models.ValueType{Value: "cat-secret-access-key", Kind: common.ValueTypeSecret}
	cspObj.CspDomainAttributes = da

	pdObj := &models.ProtectionDomain{}
	pdObj.Meta = &models.ObjMeta{ID: "catPdId"}
	pdObj.Name = "catPdName"
	pdObj.EncryptionAlgorithm = "catEa"
	pdObj.EncryptionPassphrase = &models.ValueType{Kind: common.ValueTypeSecret, Value: "cat secret"}

	uaO := &SnapshotCatalogUpsertArgsSourceObjects{
		EntryObjects:     *sceSrc,
		ProtectionStore:  cspObj,
		ProtectionDomain: pdObj,
	}
	ua := &SnapshotCatalogUpsertArgs{}
	ua.Initialize(uaO)
	assert.Equal(expUA, ua)
}
