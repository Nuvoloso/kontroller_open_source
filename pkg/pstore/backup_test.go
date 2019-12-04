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
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/csp/aws"
	"github.com/Nuvoloso/kontroller/pkg/csp/azure"
	"github.com/Nuvoloso/kontroller/pkg/csp/gc"
	"github.com/Nuvoloso/kontroller/pkg/endpoint"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestSnapshotBackup(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ctx := context.Background()

	ca := &ControllerArgs{Log: tl.Logger()}
	ops, err := NewController(ca)
	assert.NoError(err)
	assert.NotNil(ops)
	c, ok := ops.(*Controller)
	assert.True(ok)
	fio := &fakeInternalOps{}
	c.intOps = fio

	psd := &ProtectionStoreDescriptor{
		CspDomainType: aws.CSPDomainType,
		CspDomainAttributes: map[string]models.ValueType{
			aws.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: "bn"},
			gc.AttrPStoreBucketName:  models.ValueType{Kind: "STRING", Value: "bn"},
			aws.AttrRegion:           models.ValueType{Kind: "STRING", Value: "rg"},
			aws.AttrAccessKeyID:      models.ValueType{Kind: "STRING", Value: "ak"},
			aws.AttrSecretAccessKey:  models.ValueType{Kind: "SECRET", Value: "sk"},
			gc.AttrCred:              models.ValueType{Kind: "SECRET", Value: "cred"},
		},
	}

	args := &SnapshotBackupArgs{}
	assert.False(args.Validate(), "case: args invalid SnapIdentifier")
	args.SnapIdentifier = "snapId"
	assert.False(args.Validate(), "case: args invalid ID")
	args.ID = "id"
	assert.False(args.Validate(), "case: args invalid VsID")
	args.VsID = "vsid"
	assert.False(args.Validate(), "case: args invalid NuvoVolumeIdentifier")
	args.NuvoVolumeIdentifier = "nuvo-vol-id"
	args.PStore = &ProtectionStoreDescriptor{}
	assert.False(args.Validate(), "case: args invalid PStore")
	args.PStore = psd
	assert.False(args.Validate(), "case: args invalid SourceFile")
	args.SourceFile = "sf"
	assert.False(args.Validate(), "case: args invalid Passphrase")
	args.Passphrase = "pw"
	assert.False(args.Validate(), "case: args invalid ProtectionDomainID")
	args.ProtectionDomainID = "pdid"

	res, err := ops.SnapshotBackup(ctx, args)
	assert.Equal(ErrInvalidArguments, err, "case: invoked bad args")
	assert.Nil(res)

	args.IncrPiT = "ip"
	assert.True(args.Validate(), "case: args ok, no BasePiT or BaseSnapIdentifier")
	args.BasePiT = "bp"
	assert.False(args.Validate(), "case: args ok, with BasePiT but no BaseSnapIdentifier")
	args.BaseSnapIdentifier = "bsn"
	assert.True(args.Validate(), "case: args ok, with BasePiT and BaseSnapIdentifier")

	// args ok, force copy failure
	fio.RetSCerr = fmt.Errorf("spawn-copy-error")
	res, err = ops.SnapshotBackup(ctx, args)
	assert.Error(err)
	assert.Regexp("spawn-copy-error", err)
	assert.Equal(args.ID, fio.InSCid)
	assert.Nil(res)

	// copy success
	fio.RetSCerr = nil

	CSPtypes := []string{gc.CSPDomainType, azure.CSPDomainType, aws.CSPDomainType}
	for _, psd.CspDomainType = range CSPtypes {
		res, err = ops.SnapshotBackup(ctx, args)
		assert.NoError(err)
		assert.NotNil(res)
	}

	// validate the copy arguments
	expArgs := &endpoint.CopyArgs{
		SrcType: endpoint.TypeNuvo,
		SrcArgs: endpoint.AllArgs{
			Nuvo: endpoint.NuvoArgs{
				VolumeUUID:   "nuvo-vol-id",
				FileName:     "sf",
				BaseSnapUUID: "bp",
				IncrSnapUUID: "ip",
			},
		},
		DstType: endpoint.TypeAWS,
		DstArgs: endpoint.AllArgs{
			S3: endpoint.S3Args{
				BucketName:      "bn",
				Region:          "rg",
				AccessKeyID:     "ak",
				SecretAccessKey: "sk",
				Domain:          "pdid",
				Base:            "bsn",
				Incr:            "snapId",
				PassPhrase:      "pw",
			},
		},
		ProgressFileName: "/tmp/backup_progress_id",
		NumThreads:       numBackupThreads,
	}
	assert.Equal(expArgs, fio.InSCargs)

	// validate backup result parsing
	expRes := &SnapshotBackupResult{
		Stats: CopyStats{
			BytesTransferred: 1000,
			BytesChanged:     30,
			BytesUnchanged:   970,
		},
	}
	tf := "./backupResFile.json"
	defer os.Remove(tf)
	b, err := json.Marshal(expRes)
	assert.NoError(err)
	assert.NoError(ioutil.WriteFile(tf, b, 0600), "WriteFile")
	res = &SnapshotBackupResult{}
	err = c.ReadResult(tf, res)
	assert.NoError(err)
	assert.Equal(expRes, res)
}
