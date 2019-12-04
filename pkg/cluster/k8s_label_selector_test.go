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

	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestK8sLabelSelectorList(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	// nil selector
	var s *K8sLabelSelector
	assert.Nil(s.List())

	// empty selector
	s = &K8sLabelSelector{}
	assert.Nil(s.List())

	// one label
	s.MatchLabels = map[string]string{"a": "b"}
	assert.Equal([]string{"a=b"}, s.List())

	// multiple labels
	s.MatchLabels = map[string]string{"a": "b", "c": "d"}
	assert.Equal([]string{"a=b", "c=d"}, s.List())

	// one matcher
	s = &K8sLabelSelector{}
	s.MatchExpressions = []K8sLabelSelectorRequirement{
		{Key: "a", Operator: K8sLabelSelectorOpExists, Values: []string{"ignored"}},
	}
	assert.Equal([]string{"a"}, s.List())

	// multiple matchers
	s.MatchExpressions = []K8sLabelSelectorRequirement{
		{Key: "env", Operator: K8sLabelSelectorOpIn, Values: []string{"prod", "qa"}},
		{Key: "a", Operator: K8sLabelSelectorOpExists, Values: []string{"ignored"}},
	}
	assert.Equal([]string{"env in (prod,qa)", "a"}, s.List())

	// multiple both
	s.MatchLabels = map[string]string{"a": "b", "c": "d"}
	s.MatchExpressions = []K8sLabelSelectorRequirement{
		{Key: "env", Operator: K8sLabelSelectorOpNotIn, Values: []string{"qa", "prod"}},
		{Key: "x", Operator: K8sLabelSelectorOpDoesNotExist},
	}
	assert.Equal([]string{"a=b", "c=d", "env notin (qa,prod)", "!x"}, s.List())
}
