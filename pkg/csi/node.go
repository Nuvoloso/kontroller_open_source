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

// Node handler provides the CSI interface to mount and unmount a volume

import (
	"context"
	"fmt"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/cluster"
	"github.com/Nuvoloso/kontroller/pkg/util"
)

// NodeHandler provides methods to manage the NodeHandler.
type NodeHandler interface {
	Handler
}

// NodeHandlerOps contains consumer provided methods that are invoked by the NodeHandler
type NodeHandlerOps interface {
	// MountVolume is a blocking call to mount a volume
	MountVolume(ctx context.Context, args *MountArgs) error
	// UnmountVolume is a blocking call to unmount a volume
	UnmountVolume(ctx context.Context, args *UnmountArgs) error
	// GetAccountFromSecret gets the account obj from a given secret(/cluster/domain) combo
	GetAccountFromSecret(ctx context.Context, args *AccountFetchArgs) (*models.Account, error)
	// IsReady returns true if the node is ready
	IsReady() bool
}

// MountArgs describe a mount request
type MountArgs struct {
	Name         string // volume name
	ServicePlan  string
	SizeBytes    int64
	VolumeID     string
	NodeID       string
	TargetPath   string
	FsType       string
	Account      *models.Account
	ReadOnly     bool
	Dynamic      bool
	AVD          *cluster.AccountVolumeData
	PodName      string
	PodNamespace string
}

// Validate validates MountArgs
func (m *MountArgs) Validate(ctx context.Context) error {
	if m.VolumeID == "" || m.TargetPath == "" || m.NodeID == "" || m.Account == nil || m.Account.Meta == nil ||
		(m.Dynamic && (m.Name == "" || m.ServicePlan == "")) {
		return fmt.Errorf("mount args invalid or missing")
	}
	if m.SizeBytes == 0 && m.Dynamic {
		m.SizeBytes = util.BytesInGiB
	}
	return nil
}

// UnmountArgs describe an unmount request
type UnmountArgs struct {
	VolumeID   string
	TargetPath string
}

// Validate validates UnmountArgs
func (m *UnmountArgs) Validate(ctx context.Context) error {
	if m.VolumeID == "" || m.TargetPath == "" {
		return fmt.Errorf("unmount args invalid or missing")
	}
	return nil
}

// AccountFetchArgs describes the required values to fetch an account
type AccountFetchArgs struct {
	Secret      string
	ClusterID   string
	CSPDomainID string
}

// Validate validates AccountFetchArgs
func (a *AccountFetchArgs) Validate(ctx context.Context) error {
	if a.Secret == "" || a.ClusterID == "" || a.CSPDomainID == "" {
		return fmt.Errorf("Account Fetch args invalid or missing")
	}
	return nil
}

// NodeHandlerArgs contains arguments to create a NodeHandler for a node
type NodeHandlerArgs struct {
	HandlerArgs
	Node    *models.Node
	Cluster *models.Cluster
	Ops     NodeHandlerOps
}

// Validate checks correctness
func (ha *NodeHandlerArgs) Validate() error {
	if ha.Node == nil || ha.Ops == nil || ha.Version == "" {
		return ErrInvalidArgs
	}
	return ha.HandlerArgs.Validate()
}

// nodeHandler implements the NodeHandler interface
type nodeHandler struct {
	NodeHandlerArgs
	csiService
}

var _ = NodeHandler(&nodeHandler{})

// NewNodeHandler returns a NodeHandler
func NewNodeHandler(args *NodeHandlerArgs) (NodeHandler, error) {
	if err := args.Validate(); err != nil {
		return nil, err
	}
	c := &nodeHandler{NodeHandlerArgs: *args}
	c.csiService.HandlerArgs = args.HandlerArgs
	c.csiService.NodeOps = c.Ops
	c.csiService.Node = args.Node
	c.csiService.Cluster = args.Cluster
	return c, nil
}
