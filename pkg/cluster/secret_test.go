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


package cluster

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecretDatastructures(t *testing.T) {
	assert := assert.New(t)

	badSecretCreateArgs := []SecretCreateArgs{
		{},
		{Name: "name"},
		{Name: "name", Data: ""},
	}
	for i, tc := range badSecretCreateArgs {
		assert.Error((&tc).ValidateForFormat(), "badSCA %d", i)
	}

	badSecretCreateArgsMV := []SecretCreateArgsMV{
		{},
		{Name: "name"},
		{Name: "name", Intent: -1},
		{Name: "name", Intent: SecretIntentAccountIdentity},
		{Name: "name", Intent: SecretIntentAccountIdentity, CustomizationData: AccountVolumeData{AccountSecret: "foo", ConsistencyGroupDescription: "foo"}},
		{Name: "name", Intent: SecretIntentDynamicVolumeCustomization},
		{Name: "name", Intent: SecretIntentDynamicVolumeCustomization, CustomizationData: AccountVolumeData{AccountSecret: "foo"}},
		{Name: "name", Intent: SecretIntentDynamicVolumeCustomization, CustomizationData: AccountVolumeData{AccountSecret: "foo", ApplicationGroupTags: []string{}}},
		{Name: "name", Intent: SecretIntentDynamicVolumeCustomization, CustomizationData: AccountVolumeData{AccountSecret: "foo", ConsistencyGroupTags: []string{}}},
		{Name: "name", Intent: SecretIntentDynamicVolumeCustomization, CustomizationData: AccountVolumeData{AccountSecret: "foo", VolumeTags: []string{}}},
	}
	for i, tc := range badSecretCreateArgsMV {
		assert.Error((&tc).ValidateForFormat(), "badScaMV %d", i)
	}

	validSecretCreateArgsMV := []SecretCreateArgsMV{
		{Name: "name", Data: map[string]string{"foo": "bar"}},
		{Name: "name", Intent: SecretIntentAccountIdentity, CustomizationData: AccountVolumeData{AccountSecret: "foo"}},
		{Name: "name", Intent: SecretIntentDynamicVolumeCustomization, CustomizationData: AccountVolumeData{AccountSecret: "foo", ConsistencyGroupName: "cg"}},
		{Name: "name", Intent: SecretIntentDynamicVolumeCustomization, CustomizationData: AccountVolumeData{AccountSecret: "foo", ConsistencyGroupDescription: "foo"}},
		{Name: "name", Intent: SecretIntentDynamicVolumeCustomization, CustomizationData: AccountVolumeData{AccountSecret: "foo", ApplicationGroupTags: []string{"agt1"}}},
		{Name: "name", Intent: SecretIntentDynamicVolumeCustomization, CustomizationData: AccountVolumeData{AccountSecret: "foo", ConsistencyGroupTags: []string{"cgt1"}}},
		{Name: "name", Intent: SecretIntentDynamicVolumeCustomization, CustomizationData: AccountVolumeData{AccountSecret: "foo", VolumeTags: []string{"vt1"}}},
		{Name: "name", Intent: SecretIntentDynamicVolumeCustomization, CustomizationData: AccountVolumeData{AccountSecret: "foo", ConsistencyGroupName: "cg", ConsistencyGroupDescription: "foo", ConsistencyGroupTags: []string{"cgt1"}, VolumeTags: []string{"vt1"}}},
	}
	for i, tc := range validSecretCreateArgsMV {
		assert.NoError((&tc).ValidateForFormat(), "validScaMV %d", i)
	}
}
