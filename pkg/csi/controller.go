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

import "context"

// ControllerHandler provides the CSI interface for dynamic provisioning and
// Volume lifecycle management (including snapshots and deletion).

// ControllerHandler provides methods to manage the ControllerHandler.
type ControllerHandler interface {
	Handler
}

// ControllerHandlerOps contains consumer provided methods that are invoked by the ControllerHandler
type ControllerHandlerOps interface {
	// CreateVolume is a blocking call to create a volume and return its ID
	// Currently it just generates an ID that will be used to create a Volume Series
	CreateVolumeID(ctx context.Context) (string, error)
	// DeleteVolume is a blocking call to delete a volume
	DeleteVolume(ctx context.Context, volumeID string) error
	// IsReady returns true if the controller is ready
	IsReady() bool
}

// ControllerHandlerArgs contains arguments to create a ControllerHandler
type ControllerHandlerArgs struct {
	HandlerArgs
	Ops ControllerHandlerOps
}

// Validate checks correctness
func (ha *ControllerHandlerArgs) Validate() error {
	if ha.Version == "" || ha.Ops == nil {
		return ErrInvalidArgs
	}
	return ha.HandlerArgs.Validate()
}

// controllerHandler implements the ControllerHandler interface
type controllerHandler struct {
	ControllerHandlerArgs
	csiService
}

var _ = ControllerHandler(&controllerHandler{})

// NewControllerHandler returns a ControllerHandler
func NewControllerHandler(args *ControllerHandlerArgs) (ControllerHandler, error) {
	if err := args.Validate(); err != nil {
		return nil, err
	}
	c := &controllerHandler{ControllerHandlerArgs: *args}
	c.csiService.HandlerArgs = args.HandlerArgs
	c.csiService.ControllerOps = c.Ops
	return c, nil
}
