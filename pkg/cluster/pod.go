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

// Pod is an interface for interacting with a Container Orchestrator pod (group of co-located containers) object
type Pod interface {
	PodFetch(ctx context.Context, pfa *PodFetchArgs) (*PodObj, error)
	PodList(ctx context.Context, pla *PodListArgs) ([]*PodObj, error)
}

// PodFetchArgs contains the parameters required to fetch a pod.
// Name - the name of the pod
// Namespace - the namespace containing the pod
type PodFetchArgs struct {
	Name      string
	Namespace string
}

// Validate makes sure the input parameters for a Pod fetch are valid
func (pfa *PodFetchArgs) Validate() error {
	if pfa.Name == "" || pfa.Namespace == "" {
		return fmt.Errorf("invalid values for Pod fetch")
	}
	return nil
}

// PodListArgs contains the parameters required to list pods.
// Namespace - the namespace containing the pod
// NodeName - the node where the pod is executing (optional), the valid values may vary with the Orchestrator but generally must be a valid DNS name
// Labels - a list of pod labels that must match. The acceptable values vary with the Orchestrator
type PodListArgs struct {
	Namespace string
	NodeName  string
	Labels    []string
}

// Validate makes sure the input parameters for a Pod list are valid
func (pla *PodListArgs) Validate() error {
	if pla.Namespace == "" {
		return fmt.Errorf("invalid values for Pod list")
	}
	return nil
}

// PodObj is a container orchestrator neutral structure to return pod (group of co-located containers) information
type PodObj struct {
	Name           string
	Namespace      string
	Labels         map[string]string
	Annotations    map[string]string
	ControllerName string
	ControllerKind string
	ControllerID   string
	Raw            interface{}
}
