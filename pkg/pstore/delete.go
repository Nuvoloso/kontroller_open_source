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

import "context"

// SnapshotDeleteArgs contain arguments to the SnapshotDelete method.
type SnapshotDeleteArgs struct {
	// The snapshot identifier
	SnapIdentifier string
	// An identifier for the invocation (only)
	ID string
	// The protection store containing the snapshot to be deleted.
	PStore *ProtectionStoreDescriptor
	// Snapshot identifier to delete.
	SourceSnapshot string
}

// Validate checks the arguments for correctness
func (sbd *SnapshotDeleteArgs) Validate() bool {
	if sbd.SnapIdentifier == "" || sbd.ID == "" || sbd.PStore == nil || !sbd.PStore.Validate() || sbd.SourceSnapshot == "" {
		return false
	}
	return true
}

// SnapshotDeleteResult returns the result of the SnapshotDelete method.
type SnapshotDeleteResult struct{}

// SnapshotDelete restores a snapshot in a volume
// TBD: Not yet implemented
func (c *Controller) SnapshotDelete(ctx context.Context, args *SnapshotDeleteArgs) (*SnapshotDeleteResult, error) {
	if !args.Validate() {
		return nil, ErrInvalidArguments
	}
	return &SnapshotDeleteResult{}, nil
}
