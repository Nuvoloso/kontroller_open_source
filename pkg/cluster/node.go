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

// Node is an interface for interacting with a Container Orchestrator node object
type Node interface {
	NodeFetch(ctx context.Context, nfa *NodeFetchArgs) (*NodeObj, error)
}

// NodeFetchArgs contains the parameters required to fetch a node.
// Name - the name of the node
type NodeFetchArgs struct {
	Name string
}

// Validate makes sure the input parameters for a Node fetch are valid
func (nfa *NodeFetchArgs) Validate() error {
	if nfa.Name == "" {
		return fmt.Errorf("invalid values for Node fetch")
	}
	return nil
}

// NodeObj is a container orchestrator neutral structure to return node information
type NodeObj struct {
	Name        string
	Labels      map[string]string
	Annotations map[string]string
	UID         string
	Raw         interface{}
}
