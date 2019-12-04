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


package state

import (
	"context"
	"fmt"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockcsp "github.com/Nuvoloso/kontroller/pkg/csp/mock"
	"github.com/Nuvoloso/kontroller/pkg/mgmtclient/fake"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/alecthomas/units"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestGetPoolStorageType(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	fc := &fake.Client{}
	clObj := &models.Cluster{
		ClusterAllOf0: models.ClusterAllOf0{
			Meta: &models.ObjMeta{
				ID: "CLUSTER-1",
			},
		},
	}

	ctx := context.Background()

	sts := []*models.CSPStorageType{
		&models.CSPStorageType{
			Name:                             "Amazon gp2",
			MinAllocationSizeBytes:           swag.Int64(int64(units.Gibibyte)),
			MaxAllocationSizeBytes:           swag.Int64(int64(16 * units.Tebibyte)),
			PreferredAllocationSizeBytes:     swag.Int64(int64(500 * units.Gibibyte)),
			PreferredAllocationUnitSizeBytes: swag.Int64(int64(100 * units.Gibibyte)),
			ParcelSizeBytes:                  swag.Int64(int64(units.Gibibyte)),
		},
	}
	spObj := &models.Pool{
		PoolAllOf0: models.PoolAllOf0{
			Meta: &models.ObjMeta{
				ID: "SP-1",
			},
		},
		PoolCreateOnce: models.PoolCreateOnce{
			CspStorageType: "Amazon gp2",
		},
	}

	// sp is cached
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cSP := mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cSP.EXPECT().SupportedCspStorageTypes().Return(sts)
	args := &ClusterStateArgs{
		Log:     tl.Logger(),
		OCrud:   fc,
		Cluster: clObj,
		CSP:     cSP,
	}
	cs := NewClusterState(args)
	assert.NotNil(cs)
	fc.RetPoolFetchObj = spObj
	assert.Empty(cs.pCache)
	cst, err := cs.GetPoolStorageType(ctx, "SP-1")
	assert.NoError(err)
	assert.NotNil(cst)
	assert.Equal(sts[0], cst)
	assert.NotEmpty(cs.pCache)
	assert.Len(cs.pCache, 1)

	// cache is used if available
	fc.RetPoolFetchErr = fmt.Errorf("Invalid SPid")
	cst, err = cs.GetPoolStorageType(ctx, "SP-1")
	assert.NoError(err)
	assert.NotNil(cst)
	assert.Equal(sts[0], cst)

	// errors returned
	fc.RetPoolFetchErr = fmt.Errorf("Invalid SPid")
	cst, err = cs.GetPoolStorageType(ctx, "SP-2")
	assert.Error(err)
	assert.Regexp("Invalid SPid", err)

	// unsupported type
	fc.RetPoolFetchErr = nil
	fc.RetPoolFetchObj = spObj
	spObj.CspStorageType = "fooType"
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cSP = mockcsp.NewMockCloudServiceProvider(mockCtrl)
	cSP.EXPECT().SupportedCspStorageTypes().Return(sts)
	cs.CSP = cSP
	cst, err = cs.GetPoolStorageType(ctx, "SP-2")
	assert.Error(err)
	assert.Regexp("unsupported storage type", err)
	assert.Len(cs.pCache, 1)
}
