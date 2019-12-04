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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CreateVolumeID generates a unique ID for a volume
func (c *csiComp) CreateVolumeID(ctx context.Context) (string, error) {
	volID, err := c.app.OCrud.VolumeSeriesNewID(ctx)
	if err != nil {
		return "", status.Errorf(codes.Internal, "unable to create Volume ID: %s", err.Error())
	}
	return volID.Value, nil
}
