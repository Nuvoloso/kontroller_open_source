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

// Controller is an interface for interacting with a Container Orchestrator controller object
type Controller interface {
	ControllerFetch(ctx context.Context, cfa *ControllerFetchArgs) (*ControllerObj, error)
}

// ControllerFetchArgs contains the parameters required to fetch a controller.
// Kind - the kind of the controller
// Name - the name of the controller
// Namespace - the namespace containing the controller
type ControllerFetchArgs struct {
	Kind      string
	Name      string
	Namespace string
}

// Validate makes sure the input parameters for a Controller fetch are valid
func (cfa *ControllerFetchArgs) Validate() error {
	if cfa.Kind == "" || cfa.Name == "" || cfa.Namespace == "" {
		return fmt.Errorf("invalid values for Controller fetch")
	}
	return nil
}

// ControllerObj is a container orchestrator neutral structure to return controller information
type ControllerObj struct {
	Kind          string
	Name          string
	Namespace     string
	ServiceName   string
	Annotations   map[string]string
	Selectors     []string
	Replicas      int32
	ReadyReplicas int32
	UID           string
	Raw           interface{}
}
