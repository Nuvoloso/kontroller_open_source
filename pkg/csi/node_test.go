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


package csi

import (
	"context"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/Nuvoloso/kontroller/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestNodeHandlerMethods(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	// invalid args
	invalidTCs := []NodeHandlerArgs{
		NodeHandlerArgs{},
		NodeHandlerArgs{Node: &models.Node{}},
		NodeHandlerArgs{Node: &models.Node{}, Cluster: &models.Cluster{}},
		NodeHandlerArgs{Node: &models.Node{}, Cluster: &models.Cluster{}, Ops: &fakeNHO{}},
		NodeHandlerArgs{Node: &models.Node{}, Cluster: &models.Cluster{}, Ops: &fakeNHO{}, HandlerArgs: HandlerArgs{Log: tl.Logger(), Socket: "foo", Version: ""}},
		NodeHandlerArgs{Node: &models.Node{}, Cluster: &models.Cluster{}, Ops: &fakeNHO{}, HandlerArgs: HandlerArgs{Socket: "foo", Version: "0.2.3"}},
		NodeHandlerArgs{Node: &models.Node{}, Cluster: &models.Cluster{}, Ops: &fakeNHO{}, HandlerArgs: HandlerArgs{Log: tl.Logger(), Version: "0.2.3"}},
	}
	for i, tc := range invalidTCs {
		assert.Error(tc.Validate(), "case %d", i)
		nh, err := NewNodeHandler(&tc)
		assert.Error(err, "case %d", i)
		assert.Nil(nh, "case %d", i)
	}

	// valid args, invalid path
	nha := &NodeHandlerArgs{
		HandlerArgs: HandlerArgs{
			Socket:  "./foo",
			Log:     tl.Logger(),
			Version: "1.0.1",
		},
		Node: &models.Node{
			NodeAllOf0: models.NodeAllOf0{
				Meta: &models.ObjMeta{
					ID: "id",
				},
			},
		},
		Cluster: &models.Cluster{
			ClusterAllOf0: models.ClusterAllOf0{
				Meta: &models.ObjMeta{
					ID: "id",
				},
			},
		},
		Ops: &fakeNHO{},
	}
	assert.NoError(nha.Validate())

	// create succeeds
	nh, err := NewNodeHandler(nha)
	assert.NoError(err)
	assert.NotNil(nh)

	h, ok := nh.(*nodeHandler)
	assert.True(ok)
	assert.NotNil(h)
	assert.Equal(h.csiService.NodeOps, h.Ops)
	assert.Equal(nha.Node, h.csiService.Node)
	assert.Equal(nha.Cluster, h.csiService.Cluster)

	// Start is currently a no-op
	nh.Start()

	// Stop is currently a no-op
	nh.Stop()
}

func TestValidateMethods(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	// ******** MountArgs
	// valid args
	validAccount := &models.Account{
		AccountAllOf0: models.AccountAllOf0{
			Meta: &models.ObjMeta{
				ID: "someid",
			},
		},
	}
	invalidAccount := &models.Account{
		AccountAllOf0: models.AccountAllOf0{},
	}

	mArgs := &MountArgs{
		VolumeID:   "vID",
		TargetPath: "tID",
		NodeID:     "nID",
		Account:    validAccount,
	}
	err := mArgs.Validate(nil)
	assert.Nil(err)
	// invalid cases
	invalidTCs := []MountArgs{
		MountArgs{VolumeID: "", TargetPath: "", NodeID: "", Account: invalidAccount},
		MountArgs{VolumeID: "", TargetPath: "", NodeID: "a", Account: invalidAccount},
		MountArgs{VolumeID: "", TargetPath: "a", NodeID: "", Account: invalidAccount},
		MountArgs{VolumeID: "a", TargetPath: "", NodeID: "", Account: invalidAccount},
		MountArgs{VolumeID: "a", TargetPath: "a", NodeID: "", Account: invalidAccount},
		MountArgs{VolumeID: "a", TargetPath: "", NodeID: "a", Account: invalidAccount},
		MountArgs{VolumeID: "", TargetPath: "a", NodeID: "a", Account: invalidAccount},
		MountArgs{VolumeID: "a", TargetPath: "a", NodeID: "a", Account: invalidAccount},
		MountArgs{VolumeID: "a", TargetPath: "a", NodeID: "a", Account: nil},
		MountArgs{VolumeID: "", TargetPath: "", NodeID: "", Account: validAccount},
		MountArgs{VolumeID: "", TargetPath: "", NodeID: "a", Account: validAccount},
		MountArgs{VolumeID: "", TargetPath: "a", NodeID: "", Account: validAccount},
		MountArgs{VolumeID: "a", TargetPath: "", NodeID: "", Account: validAccount},
		MountArgs{VolumeID: "a", TargetPath: "a", NodeID: "", Account: validAccount},
		MountArgs{VolumeID: "a", TargetPath: "", NodeID: "a", Account: validAccount},
		MountArgs{VolumeID: "", TargetPath: "a", NodeID: "a", Account: validAccount},
	}
	for _, tc := range invalidTCs {
		err := tc.Validate(nil)
		assert.NotNil(err)
	}

	// valid dynamic args
	mArgs = &MountArgs{
		VolumeID:    "vID",
		TargetPath:  "tID",
		NodeID:      "nID",
		Account:     validAccount,
		Dynamic:     true,
		Name:        "name",
		ServicePlan: "servicePlanid",
	}
	err = mArgs.Validate(nil)
	assert.Nil(err)
	assert.EqualValues(util.BytesInGiB, mArgs.SizeBytes)

	invalidTCs = []MountArgs{
		MountArgs{VolumeID: "a", TargetPath: "a", NodeID: "a", Account: validAccount, Dynamic: true, Name: "", ServicePlan: ""},
		MountArgs{VolumeID: "a", TargetPath: "a", NodeID: "a", Account: validAccount, Dynamic: true, Name: "a", ServicePlan: ""},
		MountArgs{VolumeID: "a", TargetPath: "a", NodeID: "a", Account: validAccount, Dynamic: true, Name: "", ServicePlan: "a"},
	}
	for _, tc := range invalidTCs {
		err := tc.Validate(nil)
		assert.NotNil(err)
	}

	// ******** UnmountArgs
	// valid args
	uArgs := &UnmountArgs{
		VolumeID:   "vID",
		TargetPath: "tID",
	}
	err = uArgs.Validate(nil)
	assert.Nil(err)
	// invalid cases
	invalidTCs2 := []UnmountArgs{
		UnmountArgs{VolumeID: "", TargetPath: ""},
		UnmountArgs{VolumeID: "a", TargetPath: ""},
		UnmountArgs{VolumeID: "", TargetPath: "a"},
	}
	for _, tc := range invalidTCs2 {
		err := tc.Validate(nil)
		assert.NotNil(err)
	}

	// ******** AccountFetchArgs
	// valid args
	afArgs := &AccountFetchArgs{
		Secret:      "ssshhhh",
		ClusterID:   "somecluster",
		CSPDomainID: "someDomain",
	}
	err = afArgs.Validate(nil)
	assert.Nil(err)
	// invalid cases
	invalidTCs3 := []AccountFetchArgs{
		AccountFetchArgs{Secret: "", ClusterID: "", CSPDomainID: ""},
		AccountFetchArgs{Secret: "a", ClusterID: "", CSPDomainID: ""},
		AccountFetchArgs{Secret: "", ClusterID: "a", CSPDomainID: ""},
		AccountFetchArgs{Secret: "", ClusterID: "", CSPDomainID: "a"},
		AccountFetchArgs{Secret: "a", ClusterID: "", CSPDomainID: "a"},
		AccountFetchArgs{Secret: "a", ClusterID: "a", CSPDomainID: ""},
		AccountFetchArgs{Secret: "", ClusterID: "a", CSPDomainID: "a"},
	}
	for _, tc := range invalidTCs3 {
		err := tc.Validate(nil)
		assert.NotNil(err)
	}
}

type fakeNHO struct {
	InMountArgs *MountArgs
	RetMountErr error

	InUnmountArgs *UnmountArgs
	RetUnmountErr error

	InGasArgs *AccountFetchArgs
	RetGas    *models.Account
	RetGasErr error

	RetIsReady bool
}

func (h *fakeNHO) MountVolume(ctx context.Context, args *MountArgs) error {
	h.InMountArgs = args
	return h.RetMountErr
}

func (h *fakeNHO) UnmountVolume(ctx context.Context, args *UnmountArgs) error {
	h.InUnmountArgs = args
	return h.RetUnmountErr
}

func (h *fakeNHO) GetAccountFromSecret(ctx context.Context, args *AccountFetchArgs) (*models.Account, error) {
	h.InGasArgs = args
	return h.RetGas, h.RetGasErr
}

func (h *fakeNHO) IsReady() bool {
	return h.RetIsReady
}
