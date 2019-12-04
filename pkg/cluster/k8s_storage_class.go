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
	"encoding/json"
	"fmt"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
)

// ServicePlanPublish performs a post to create a Storage Class in K8s and returns a PVC clusterDescriptor.
func (c *K8s) ServicePlanPublish(ctx context.Context, sp *models.ServicePlan) (models.ClusterDescriptor, error) {
	scArgs := &storageClassCreateArgs{
		name:            K8sStorageClassName(string(sp.Name)),
		secretName:      com.AccountSecretClusterObjectName,
		servicePlanName: string(sp.Name),
		servicePlanID:   string(sp.Meta.ID),
	}
	if err := c.storageClassCreate(ctx, scArgs); err != nil {
		return nil, err
	}
	pvcSpec := K8sPVCSpecArgs{
		ServicePlanName: string(sp.Name),
	}
	cd := models.ClusterDescriptor{
		com.K8sPvcYaml: models.ValueType{Kind: "STRING", Value: pvcSpec.GetDynamicYaml()},
	}
	return cd, nil
}

type storageClassCreateArgs struct {
	name            string
	secretName      string
	servicePlanName string
	servicePlanID   string
}

func (scArgs *storageClassCreateArgs) Validate() error {
	if scArgs.secretName == "" || scArgs.name == "" {
		return fmt.Errorf("invalid arguments")
	}
	return nil
}

func (c *K8s) storageClassCreate(ctx context.Context, scArgs *storageClassCreateArgs) error {
	if err := scArgs.Validate(); err != nil {
		return err
	}
	var scBody scPostBody
	scBody.Init(scArgs)
	var sc *K8sStorageClass
	if err := c.K8sClientPostJSON(ctx, "/apis/storage.k8s.io/v1/storageclasses", scBody.Marshal(), &sc); err == nil {
		return nil
	} else if e, ok := err.(*k8sError); ok {
		if e.ObjectExists() {
			return nil
		}
		return e
	} else {
		return err
	}
}

// K8sStorageClassName creates a valid K8s storage class name from a Nuvo service plan name
func K8sStorageClassName(servicePlanName string) string {
	if servicePlanName == "" {
		return ""
	}
	return K8sMakeValidName(K8sScNamePrefix + servicePlanName)
}

// scPostBody describes a body for post operations
type scPostBody struct {
	APIVersion  string           `json:"apiVersion"`
	Kind        string           `json:"kind"`
	Metadata    *ObjectMeta      `json:"metadata"`
	Provisioner string           `json:"provisioner"`
	Parameters  *K8sScParameters `json:"parameters"`
}

func (scpb *scPostBody) Init(scArgs *storageClassCreateArgs) {
	scpb.APIVersion = "storage.k8s.io/v1"
	scpb.Kind = "StorageClass"
	scpb.Metadata = &ObjectMeta{
		Name: scArgs.name,
	}
	scpb.Provisioner = com.CSIDriverName
	scpb.Parameters = &K8sScParameters{
		NodePublishSecretName:      fmt.Sprintf("${pvc.annotations['%s']}", com.K8sPvcSecretAnnotationKey),
		NodePublishSecretNamespace: "${pvc.namespace}",
		ServicePlanName:            scArgs.servicePlanName,
		ServicePlanID:              scArgs.servicePlanID,
	}
}

func (scpb *scPostBody) Marshal() []byte {
	body, _ := json.Marshal(scpb)
	return body
}
