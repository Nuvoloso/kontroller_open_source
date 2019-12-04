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
	"sort"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
)

// SnapshotSortField is an enum type to identify the sort field
type SnapshotSortField int

// SnapshotSortField enum values
const (
	SnapshotSortOnSnapTime SnapshotSortField = iota
	SnapshotSortOnDeleteAfterTime
)

// SnapshotSorter can sort a list of snapshots
type SnapshotSorter struct {
	Snapshots []*models.SnapshotData
	sortField SnapshotSortField
}

// Len satisfies the sort interface
func (ss *SnapshotSorter) Len() int {
	return len(ss.Snapshots)
}

// Less satisfies the sort interface
func (ss *SnapshotSorter) Less(i, j int) bool {
	var ti, tj time.Time
	switch ss.sortField {
	default:
		ti = time.Time(ss.Snapshots[i].SnapTime)
		tj = time.Time(ss.Snapshots[j].SnapTime)
	case SnapshotSortOnDeleteAfterTime:
		ti = time.Time(ss.Snapshots[i].DeleteAfterTime)
		tj = time.Time(ss.Snapshots[j].DeleteAfterTime)
	}
	return ti.After(tj)
}

// Swap satisfies the sort interface
func (ss *SnapshotSorter) Swap(i, j int) {
	ss.Snapshots[j], ss.Snapshots[i] = ss.Snapshots[i], ss.Snapshots[j]
}

// Sort will sort the list as per the sort field
func (ss *SnapshotSorter) Sort(sortField SnapshotSortField) {
	ss.sortField = sortField
	sort.Sort(ss)
}

// SnapshotMapToList takes a map of snapshots keyed on their snap identifier,
// such as that defined in a VolumeSeries object, and returns an unordered list of snapshots.
// It ensures that the snapIdentifier is set in the snapshot object from the map key.
func SnapshotMapToList(sMap map[string]models.SnapshotData) []*models.SnapshotData {
	list := make([]*models.SnapshotData, 0, len(sMap))
	for k, v := range sMap {
		s := &models.SnapshotData{}
		*s = v
		s.SnapIdentifier = k // enforce
		list = append(list, s)
	}
	return list
}

// SnapshotMapToListSorted is a sorting variant of SnapshotMapToList, sorting in descending time (i.e. latest first)
// over the chosen sort field.  An invalid sortField results in a sort by snapTime.
func SnapshotMapToListSorted(sMap map[string]models.SnapshotData, sortField SnapshotSortField) []*models.SnapshotData {
	ss := &SnapshotSorter{Snapshots: SnapshotMapToList(sMap)}
	ss.Sort(sortField)
	return ss.Snapshots
}

// SnapshotListSorter can sort a list of snapshots
type SnapshotListSorter struct {
	Snapshots []*models.Snapshot
	sortField SnapshotSortField
}

// Len satisfies the sort interface
func (ss *SnapshotListSorter) Len() int {
	return len(ss.Snapshots)
}

// Less satisfies the sort interface
func (ss *SnapshotListSorter) Less(i, j int) bool {
	var ti, tj time.Time
	switch ss.sortField {
	default:
		ti = time.Time(ss.Snapshots[i].SnapTime)
		tj = time.Time(ss.Snapshots[j].SnapTime)
	case SnapshotSortOnDeleteAfterTime:
		ti = time.Time(ss.Snapshots[i].DeleteAfterTime)
		tj = time.Time(ss.Snapshots[j].DeleteAfterTime)
	}
	return ti.After(tj)
}

// Swap satisfies the sort interface
func (ss *SnapshotListSorter) Swap(i, j int) {
	ss.Snapshots[j], ss.Snapshots[i] = ss.Snapshots[i], ss.Snapshots[j]
}

// Sort will sort the list as per the sort field
func (ss *SnapshotListSorter) Sort(sortField SnapshotSortField) {
	ss.sortField = sortField
	sort.Sort(ss)
}

// SnapshotListToSorted is a sorting variant of SnapshotMapToList, sorting in descending time (i.e. latest first)
// over the chosen sort field.  An invalid sortField results in a sort by snapTime.
func SnapshotListToSorted(sList []*models.Snapshot, sortField SnapshotSortField) []*models.Snapshot {
	ss := &SnapshotListSorter{Snapshots: sList}
	ss.Sort(sortField)
	return ss.Snapshots
}
