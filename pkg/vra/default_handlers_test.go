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

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultHandlers(t *testing.T) {
	assert := assert.New(t)

	// they should all just set RetryLater
	a := &Animator{}
	rhs := &RequestHandlerState{}
	expected := &RequestHandlerState{RetryLater: true}

	a.AllocateCapacity(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.AttachFs(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.Bind(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.CGSnapshotCreate(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.ChangeCapacity(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.ChooseNode(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.Configure(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.Create(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.CreateFromSnapshot(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.Export(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.NodeDelete(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.Place(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.Publish(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.PublishServicePlan(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.ReallocateCache(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.Rename(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.ResizeCache(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.Size(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.VolDetach(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.VolSnapshotCreate(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.VolSnapshotRestore(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	// and the undo handlers
	a.UndoAllocateCapacity(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.UndoAttachFs(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.UndoBind(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.UndoCGSnapshotCreate(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.UndoChangeCapacity(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.UndoConfigure(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.UndoCreate(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.UndoCreateFromSnapshot(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.UndoExport(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.UndoPlace(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.UndoSize(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.UndoPublish(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.UndoReallocateCache(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.UndoRename(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.UndoResizeCache(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.UndoVolSnapshotCreate(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false

	a.UndoVolSnapshotRestore(nil, rhs)
	assert.Equal(expected, rhs)
	rhs.RetryLater = false
}
