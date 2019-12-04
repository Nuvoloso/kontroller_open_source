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


package mock

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/stretchr/testify/assert"
)

// DiskMatcher is a gomock.Matcher
type DiskMatcher struct {
	t *testing.T
	d compute.Disk
}

// NewDiskMatcher returns a gomock.Matcher
func NewDiskMatcher(t *testing.T, d compute.Disk) *DiskMatcher {
	return &DiskMatcher{t: t, d: d}
}

// Matches is from gomock.Matcher
func (o *DiskMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	d, _ := x.(compute.Disk)
	return assert.IsType(compute.Disk{}, x) &&
		assert.Equal(o.d, d)
}

// String is from gomock.Matcher
func (o *DiskMatcher) String() string {
	return "DiskMatcher matches"
}

// VirtualMachineMatcher is a gomock.Matcher
type VirtualMachineMatcher struct {
	t *testing.T
	d compute.VirtualMachine
}

// NewVirtualMachineMatcher returns a gomock.Matcher
func NewVirtualMachineMatcher(t *testing.T, d compute.VirtualMachine) *VirtualMachineMatcher {
	return &VirtualMachineMatcher{t: t, d: d}
}

// Matches is from gomock.Matcher
func (o *VirtualMachineMatcher) Matches(x interface{}) bool {
	assert := assert.New(o.t)
	d, _ := x.(compute.VirtualMachine)
	return assert.IsType(compute.VirtualMachine{}, x) &&
		assert.Equal(o.d, d)
}

// String is from gomock.Matcher
func (o *VirtualMachineMatcher) String() string {
	return "VirtualMachineMatcher matches"
}
