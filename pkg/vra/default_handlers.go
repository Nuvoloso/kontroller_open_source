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


package vra

import "context"

// AllocateCapacity implements the default SPA capacity handler, which simply sets RetryLater
func (c *Animator) AllocateCapacity(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// AttachFs implements the default attach filesystem handler, which simply sets RetryLater
func (c *Animator) AttachFs(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// Bind implements the default volume series binding handler, which simply sets RetryLater
func (c *Animator) Bind(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// CGSnapshotCreate implements the default handler of this name, which simply sets RetryLater
func (c *Animator) CGSnapshotCreate(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// ChangeCapacity implements the default volume series ChangeCapacity handler, which simply sets RetryLater
func (c *Animator) ChangeCapacity(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// ChooseNode implements the default volume series ChooseNode handler, which simply sets RetryLater
func (c *Animator) ChooseNode(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// Configure implements the default volume series configuration handler, which simply sets RetryLater
func (c *Animator) Configure(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// Create implements the default volume series creating handler, which simply sets RetryLater
func (c *Animator) Create(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// CreateFromSnapshot implements the default handler of this name, which simply sets RetryLater
func (c *Animator) CreateFromSnapshot(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// Export implements the default volume series export handler, which simply sets RetryLater
func (c *Animator) Export(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// NodeDelete implements the default handler of this name, which simply sets RetryLater
func (c *Animator) NodeDelete(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// Place implements the default volume series placement handler, which simply sets RetryLater
func (c *Animator) Place(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// Publish implements the default volume series publishing handler, which simply sets RetryLater
func (c *Animator) Publish(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// PublishServicePlan implements the default service plan publishing handler, which simply sets RetryLater
func (c *Animator) PublishServicePlan(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// ReallocateCache implements the default volume series ReallocateCache handler, which simply sets RetryLater
func (c *Animator) ReallocateCache(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// Rename implements the default volume series renaming handler, which simply sets RetryLater
func (c *Animator) Rename(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// ResizeCache implements the default volume series ResizeCache handler, which simply sets RetryLater
func (c *Animator) ResizeCache(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// Size implements the default volume series sizing handler, which simply sets RetryLater
func (c *Animator) Size(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// VolDetach implements the default handler of this name, which simply sets RetryLater
func (c *Animator) VolDetach(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// VolSnapshotCreate implements the default handler of this name, which simply sets RetryLater
func (c *Animator) VolSnapshotCreate(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// VolSnapshotRestore implements the default handler of this name, which simply sets RetryLater
func (c *Animator) VolSnapshotRestore(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// UndoAllocateCapacity implements the default SPA capacity undo handler, which simply sets RetryLater
func (c *Animator) UndoAllocateCapacity(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// UndoAttachFs implements the default attach filesystem handler, which simply sets RetryLater
func (c *Animator) UndoAttachFs(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// UndoBind implements the default volume series binding undo handler, which simply sets RetryLater
func (c *Animator) UndoBind(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// UndoCGSnapshotCreate implements the default handler of this name, which simply sets RetryLater
func (c *Animator) UndoCGSnapshotCreate(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// UndoChangeCapacity implements the default volume series UndoChangeCapacity handler, which simply sets RetryLater
func (c *Animator) UndoChangeCapacity(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// UndoConfigure implements the default volume series configuration undo handler, which simply sets RetryLater
func (c *Animator) UndoConfigure(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// UndoCreate implements the default volume series creating undo handler, which simply sets RetryLater
func (c *Animator) UndoCreate(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// UndoCreateFromSnapshot implements the default undo handler of this name, which simply sets RetryLater
func (c *Animator) UndoCreateFromSnapshot(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// UndoExport implements the default volume series export undo handler, which simply sets RetryLater
func (c *Animator) UndoExport(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// UndoPlace implements the default volume series placement undo handler, which simply sets RetryLater
func (c *Animator) UndoPlace(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// UndoPublish implements the default volume series publishing undo handler, which simply sets RetryLater
func (c *Animator) UndoPublish(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// UndoReallocateCache implements the default volume series UndoReallocateCache handler, which simply sets RetryLater
func (c *Animator) UndoReallocateCache(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// UndoRename implements the default volume series renaming undo handler, which simply sets RetryLater
func (c *Animator) UndoRename(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// UndoResizeCache implements the default volume series ResizeCache handler, which simply sets RetryLater
func (c *Animator) UndoResizeCache(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// UndoSize implements the default volume series sizing undo handler, which simply sets RetryLater
func (c *Animator) UndoSize(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// UndoVolSnapshotCreate implements the default handler of this name, which simply sets RetryLater
func (c *Animator) UndoVolSnapshotCreate(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}

// UndoVolSnapshotRestore implements the default handler of this name, which simply sets RetryLater
func (c *Animator) UndoVolSnapshotRestore(ctx context.Context, rhs *RequestHandlerState) {
	rhs.RetryLater = true
}
