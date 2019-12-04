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

func TestControllerDataStructures(t *testing.T) {
	assert := assert.New(t)

	badControllerFetchArgs := []ControllerFetchArgs{
		{},
		{Kind: "kind"},
		{Name: "name"},
		{Namespace: "namespace"},
		{Name: "name", Kind: "any"},
		{Kind: "any", Namespace: "ns"},
		{Name: "name", Namespace: "ns"},
	}
	for i, tc := range badControllerFetchArgs {
		assert.Error((&tc).Validate(), "bad ControllerFetchArgs %d", i)
	}

	validControllerFetchArgs := &ControllerFetchArgs{
		Kind:      "anyKind",
		Name:      "name",
		Namespace: "namespace",
	}
	assert.NoError(validControllerFetchArgs.Validate())
}
