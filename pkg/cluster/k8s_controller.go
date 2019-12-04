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

	"github.com/go-openapi/swag"
)

// ControllerFetch fetches a Kubernetes Controller given the args (name, namespace)
func (c *K8s) ControllerFetch(ctx context.Context, cfa *ControllerFetchArgs) (*ControllerObj, error) {
	if err := cfa.Validate(); err != nil {
		return nil, err
	}
	switch K8sControllerKind(cfa.Kind) {
	case DaemonSet:
		return c.daemonSetFetch(ctx, cfa)
	case StatefulSet:
		return c.statefulSetFetch(ctx, cfa)
	}
	return nil, fmt.Errorf("unsupported controller kind %s", cfa.Kind)
}

func (c *K8s) daemonSetFetch(ctx context.Context, cfa *ControllerFetchArgs) (*ControllerObj, error) {
	path := fmt.Sprintf("/apis/apps/v1/namespaces/%s/daemonsets/%s", cfa.Namespace, cfa.Name)
	var ds *K8sDaemonSet
	if err := c.K8sClientGetJSON(ctx, path, &ds); err != nil {
		return nil, err
	}
	return ds.ToModel(), nil
}

// ToModel converts a K8sDaemonSet object to a ControllerObj
func (ds *K8sDaemonSet) ToModel() *ControllerObj {
	cObj := &ControllerObj{
		Kind:          ds.Kind,
		Name:          ds.Name,
		Namespace:     ds.Namespace,
		Selectors:     ds.Spec.Selector.List(),
		Replicas:      ds.Status.DesiredNumberScheduled,
		ReadyReplicas: ds.Status.NumberReady,
		Annotations:   ds.Annotations,
		UID:           ds.UID,
		Raw:           ds,
	}
	return cObj
}

func (c *K8s) statefulSetFetch(ctx context.Context, cfa *ControllerFetchArgs) (*ControllerObj, error) {
	path := fmt.Sprintf("/apis/apps/v1/namespaces/%s/statefulsets/%s", cfa.Namespace, cfa.Name)
	var ss *K8sStatefulSet
	if err := c.K8sClientGetJSON(ctx, path, &ss); err != nil {
		return nil, err
	}
	return ss.ToModel(), nil
}

// ToModel converts a K8sStatefulSet object to a ControllerObj
func (ss *K8sStatefulSet) ToModel() *ControllerObj {
	cObj := &ControllerObj{
		Kind:          ss.Kind,
		Name:          ss.Name,
		Namespace:     ss.Namespace,
		ServiceName:   ss.Spec.ServiceName,
		Selectors:     ss.Spec.Selector.List(),
		Replicas:      swag.Int32Value(ss.Spec.Replicas),
		ReadyReplicas: ss.Status.ReadyReplicas,
		Annotations:   ss.Annotations,
		UID:           ss.UID,
		Raw:           ss,
	}
	return cObj
}
