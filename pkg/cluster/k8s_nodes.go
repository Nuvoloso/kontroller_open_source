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
	"context"
	"fmt"
)

// NodeFetch fetches a Kubernetes Node given the args (name, namespace)
func (c *K8s) NodeFetch(ctx context.Context, nfa *NodeFetchArgs) (*NodeObj, error) {
	if err := nfa.Validate(); err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/api/v1/nodes/%s", nfa.Name)
	var node *K8sNode
	err := c.K8sClientGetJSON(ctx, path, &node)
	if err != nil {
		return nil, err
	}
	return node.ToModel(), nil
}

// ToModel converts a K8sNode object to a NodeObj
func (n *K8sNode) ToModel() *NodeObj {
	obj := &NodeObj{
		Name:        n.Name,
		Labels:      n.Labels,
		Annotations: n.Annotations,
		UID:         n.UID,
		Raw:         n,
	}
	return obj
}
