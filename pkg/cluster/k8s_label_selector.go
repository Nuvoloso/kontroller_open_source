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
	"fmt"
	"strings"

	"github.com/Nuvoloso/kontroller/pkg/util"
)

// List returns the label selector in string list form. The values are not URL encoded
func (s *K8sLabelSelector) List() []string {
	if s == nil {
		return nil
	}
	lenLabels := len(s.MatchLabels)
	lenMatchers := len(s.MatchExpressions)
	if lenLabels == 0 && lenMatchers == 0 {
		return nil
	}
	res := make([]string, lenLabels+lenMatchers)

	for i, k := range util.SortedStringKeys(s.MatchLabels) {
		v := s.MatchLabels[k]
		label := fmt.Sprintf("%s=%s", k, v)
		res[i] = label
	}
	for i, m := range s.MatchExpressions {
		var label strings.Builder
		if m.Operator == K8sLabelSelectorOpDoesNotExist {
			label.WriteByte('!')
		}
		label.WriteString(m.Key)
		if m.Operator == K8sLabelSelectorOpIn || m.Operator == K8sLabelSelectorOpNotIn {
			label.WriteByte(' ')
			if m.Operator == K8sLabelSelectorOpNotIn {
				label.WriteString("not")
			}
			label.WriteString("in (")
			for i, v := range m.Values {
				if i > 0 {
					label.WriteByte(',')
				}
				label.WriteString(v)
			}
			label.WriteByte(')')
		}
		res[lenLabels+i] = label.String()
	}
	return res
}
