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

func TestPodFetchValidate(t *testing.T) {
	assert := assert.New(t)

	badPodFetchArgs := []PodFetchArgs{
		{},
		{Name: ""},
		{Name: "name"},
		{Name: "name", Namespace: ""},
	}
	for i, tc := range badPodFetchArgs {
		assert.Error((&tc).Validate(), "badPFA %d", i)
	}

	validPodFetchArgs := &PodFetchArgs{
		Name:      "name",
		Namespace: "namespace",
	}
	assert.NoError(validPodFetchArgs.Validate())
}

func TestPodListValidate(t *testing.T) {
	assert := assert.New(t)

	badPodListArgs := &PodListArgs{}
	assert.Error((badPodListArgs).Validate())

	validPodListArgs := &PodListArgs{
		Namespace: "namespace",
	}
	assert.NoError(validPodListArgs.Validate())
}
