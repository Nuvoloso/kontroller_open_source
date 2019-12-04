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
	"net/url"
	"strings"

	"github.com/go-openapi/swag"
)

// PodFetch fetches a Kubernetes Pod given the args (name, namespace)
func (c *K8s) PodFetch(ctx context.Context, pfa *PodFetchArgs) (*PodObj, error) {
	if err := pfa.Validate(); err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s", pfa.Namespace, pfa.Name)
	var pod *K8sPod
	err := c.K8sClientGetJSON(ctx, path, &pod)
	if err != nil {
		return nil, err
	}
	return pod.ToModel(), nil
}

// PodList fetches Kubernetes Pods given the args (nodeName, namespace, labels)
func (c *K8s) PodList(ctx context.Context, pla *PodListArgs) ([]*PodObj, error) {
	if err := pla.Validate(); err != nil {
		return nil, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "/api/v1/namespaces/%s/pods", pla.Namespace)
	if pla.NodeName != "" || len(pla.Labels) != 0 {
		b.WriteByte('?')
		if pla.NodeName != "" {
			b.WriteString("fieldSelector=spec.nodeName%3D")
			b.WriteString(url.QueryEscape(pla.NodeName))
		}
		if len(pla.Labels) > 0 {
			if pla.NodeName != "" {
				b.WriteByte('&')
			}
			b.WriteString("labelSelector=")
			for i, label := range pla.Labels {
				if i > 0 {
					b.WriteByte(',')
				}
				b.WriteString(url.QueryEscape(label))
			}
		}
	}
	var pods *K8sPodListRes
	if err := c.K8sClientGetJSON(ctx, b.String(), &pods); err != nil {
		return nil, err
	}
	res := make([]*PodObj, len(pods.Items))
	for i, pod := range pods.Items {
		res[i] = pod.ToModel()
	}
	return res, nil
}

// ToModel converts a K8sPod object to a PodObj
func (p *K8sPod) ToModel() *PodObj {
	pObj := &PodObj{
		Name:        p.Name,
		Namespace:   p.Namespace,
		Labels:      p.Labels,
		Annotations: p.Annotations,
		Raw:         p,
	}
	for _, owner := range p.OwnerReferences {
		if swag.BoolValue(owner.Controller) == true {
			pObj.ControllerID = owner.UID
			pObj.ControllerKind = owner.Kind
			pObj.ControllerName = owner.Name
		}
	}
	return pObj
}
