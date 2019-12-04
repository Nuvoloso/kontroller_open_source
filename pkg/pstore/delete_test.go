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

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/csp/aws"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestSnapshotDelete(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ctx := context.Background()

	ca := &ControllerArgs{Log: tl.Logger()}
	ops, err := NewController(ca)
	assert.NoError(err)
	assert.NotNil(ops)

	psd := &ProtectionStoreDescriptor{
		CspDomainType: aws.CSPDomainType,
		CspDomainAttributes: map[string]models.ValueType{
			aws.AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: "bn"},
			aws.AttrRegion:           models.ValueType{Kind: "STRING", Value: "rg"},
			aws.AttrAccessKeyID:      models.ValueType{Kind: "STRING", Value: "ak"},
			aws.AttrSecretAccessKey:  models.ValueType{Kind: "SECRET", Value: "sk"},
		},
	}

	args := &SnapshotDeleteArgs{}
	assert.False(args.Validate(), "case: args invalid SnapIdentifier")
	args.SnapIdentifier = "snapId"
	assert.False(args.Validate(), "case: args invalid ID")
	args.ID = "id"
	args.PStore = &ProtectionStoreDescriptor{}
	assert.False(args.Validate(), "case: args invalid PStore")
	args.PStore = psd
	assert.False(args.Validate(), "case: args invalid SourceSnapshot")

	res, err := ops.SnapshotDelete(ctx, args)
	assert.Equal(ErrInvalidArguments, err, "case: invoked bad args")
	assert.Nil(res)

	args.SourceSnapshot = "SS"
	assert.True(args.Validate(), "case: args ok")

	// not implemented but does not fail
	res, err = ops.SnapshotDelete(ctx, args)
	assert.NoError(err)
	assert.NotNil(res)
}
