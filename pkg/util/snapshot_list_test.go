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


package util

import (
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/go-openapi/strfmt"
	"github.com/stretchr/testify/assert"
)

func TestSnapshotMapToList(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()

	sMap := map[string]models.SnapshotData{
		"snap1": models.SnapshotData{
			SnapTime:        strfmt.DateTime(now.Add(-3 * time.Hour)), // oldest snap time
			DeleteAfterTime: strfmt.DateTime(now.Add(3 * time.Hour)),  // youngest delete time
		},
		"snap2": models.SnapshotData{
			SnapTime:        strfmt.DateTime(now.Add(-2 * time.Hour)),
			DeleteAfterTime: strfmt.DateTime(now.Add(2 * time.Hour)),
		},
		"snap3": models.SnapshotData{
			SnapTime:        strfmt.DateTime(now.Add(-1 * time.Hour)), // youngest snap time
			DeleteAfterTime: strfmt.DateTime(now.Add(1 * time.Hour)),  // oldest delete time
		},
	}

	t.Log("Case: sort by SnapTime")
	sl := SnapshotMapToListSorted(sMap, SnapshotSortOnSnapTime)
	assert.Equal(len(sMap), len(sl))
	for i, s := range sl {
		t.Logf("%d %s SnapTime:%v\n", i, s.SnapIdentifier, s.SnapTime)
	}
	assert.Equal("snap3", sl[0].SnapIdentifier) // youngest snap time
	assert.Equal("snap2", sl[1].SnapIdentifier)
	assert.Equal("snap1", sl[2].SnapIdentifier) // oldest snap time

	t.Log("Case: sort by DeleteAfterTime")
	sl = SnapshotMapToListSorted(sMap, SnapshotSortOnDeleteAfterTime)
	assert.Equal(len(sMap), len(sl))
	for i, s := range sl {
		t.Logf("%d %s DeleteAfterTime:%v\n", i, s.SnapIdentifier, s.DeleteAfterTime)
	}
	assert.Equal("snap1", sl[0].SnapIdentifier) // youngest delete time
	assert.Equal("snap2", sl[1].SnapIdentifier)
	assert.Equal("snap3", sl[2].SnapIdentifier) // oldest delete time

	// empty map
	sl = SnapshotMapToListSorted(map[string]models.SnapshotData{}, SnapshotSortOnSnapTime)
	assert.NotNil(sl)
	assert.Empty(sl)
}

func TestSnapshotListToSorted(t *testing.T) {
	assert := assert.New(t)

	now := time.Now()
	snapshots := []*models.Snapshot{
		&models.Snapshot{
			SnapshotAllOf0: models.SnapshotAllOf0{
				Meta: &models.ObjMeta{
					ID: "id1",
				},
				SnapTime:       strfmt.DateTime(now.Add(-3 * time.Hour)), // oldest snap time
				SnapIdentifier: "snap1",
			},
			SnapshotMutable: models.SnapshotMutable{
				Locations:       map[string]models.SnapshotLocation{},
				DeleteAfterTime: strfmt.DateTime(now.Add(3 * time.Hour)), // youngest delete time
			},
		},
		&models.Snapshot{
			SnapshotAllOf0: models.SnapshotAllOf0{
				Meta: &models.ObjMeta{
					ID: "id2",
				},
				SnapTime:       strfmt.DateTime(now.Add(-2 * time.Hour)),
				SnapIdentifier: "snap2",
			},
			SnapshotMutable: models.SnapshotMutable{
				Locations:       map[string]models.SnapshotLocation{},
				DeleteAfterTime: strfmt.DateTime(now.Add(2 * time.Hour)),
			},
		},
		&models.Snapshot{
			SnapshotAllOf0: models.SnapshotAllOf0{
				Meta: &models.ObjMeta{
					ID: "id3",
				},
				SnapTime:       strfmt.DateTime(now.Add(-1 * time.Hour)), // youngest snap time
				SnapIdentifier: "snap3",
			},
			SnapshotMutable: models.SnapshotMutable{
				Locations:       map[string]models.SnapshotLocation{},
				DeleteAfterTime: strfmt.DateTime(now.Add(1 * time.Hour)), // oldest delete time
			},
		},
	}

	t.Log("Case: sort by SnapTime")
	sl := SnapshotListToSorted(snapshots, SnapshotSortOnSnapTime)
	assert.Equal(len(snapshots), len(sl))
	for i, s := range sl {
		t.Logf("%d %s SnapTime:%v\n", i, s.SnapIdentifier, s.SnapTime)
	}
	assert.Equal("snap3", sl[0].SnapIdentifier) // youngest snap time
	assert.Equal("snap2", sl[1].SnapIdentifier)
	assert.Equal("snap1", sl[2].SnapIdentifier) // oldest snap time

	t.Log("Case: sort by DeleteAfterTime")
	sl = SnapshotListToSorted(snapshots, SnapshotSortOnDeleteAfterTime)
	assert.Equal(len(snapshots), len(sl))
	for i, s := range sl {
		t.Logf("%d %s DeleteAfterTime:%v\n", i, s.SnapIdentifier, s.DeleteAfterTime)
	}
	assert.Equal("snap1", sl[0].SnapIdentifier) // youngest delete time
	assert.Equal("snap2", sl[1].SnapIdentifier)
	assert.Equal("snap3", sl[2].SnapIdentifier) // oldest delete time

	// empty map
	sl = SnapshotListToSorted([]*models.Snapshot{}, SnapshotSortOnSnapTime)
	assert.NotNil(sl)
	assert.Empty(sl)
}
