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
	"bytes"
	"text/template"

	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/util"
)

// PvcSpecTemplateStatic4Flex is basic template for a PersistentVolumeClaim Yaml
// Created for backward compatibility.
var PvcSpecTemplateStatic4Flex = `apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  # A PVC must be created with a namespace specific name - adjust to meet your needs:
  namespace: default
  name: my-pvc
spec:
  accessModes:
    - {{.AccessMode}}
  resources:
    requests:
      storage: {{.Capacity}}
  # explicit empty storageClassName required in case a default storageClass exists
  storageClassName: ""
  selector:
    matchLabels:
      type: {{.LabelType}}
      volumeId: {{.VolumeID}}
`

// PvcSpecTemplateStatic is basic template for a PersistentVolumeClaim Yaml
var PvcSpecTemplateStatic = `---
# This sample PersistentVolumeClaim illustrates how to claim the
# PersistentVolume identified by its Nuvoloso volume identifier.
# Adjust the name and namespace to meet your needs and ensure that the
# '{{.AccountSecret}}' secret exists in this namespace.
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  namespace: default
  name: pvc-for-a-pre-provisioned-volume
spec:
  accessModes:
    - {{.AccessMode}}
  resources:
    requests:
      storage: {{.Capacity}}
  storageClassName: ""
  selector:
    matchLabels:
      type: {{.LabelType}}
      volumeId: {{.VolumeID}}
`

// PvcSpecTemplateDynamic is basic template for a PersistentVolumeClaim Yaml
var PvcSpecTemplateDynamic = `---
# This sample PersistentVolumeClaim illustrates how to dynamically provision
# a PersistentVolume for the storage class '{{.StorageClassName}}'
# which corresponds to the Nuvoloso '{{.NuvoServicePlanName}}' service plan.
# Adjust the name, namespace and capacity to meet your needs and ensure that
# the secret object named by the '{{.PvcSecretAnnotationKey}}' annotation
# key exists in this namespace.
# The '{{.AccountSecret}}' secret referenced in this example is the same as that
# used to mount pre-provisioned PersistentVolumes.  Some customization of the
# dynamically provisioned volume is possible by creating a customization secret
# explicitly and referencing that secret instead.
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  namespace: default
  name: pvc-for-a-dynamically-provisioned-volume
  annotations:
    {{.PvcSecretAnnotationKey}}: {{.AccountSecret}}
spec:
  accessModes:
    - {{.AccessMode}}
  resources:
    requests:
      storage: {{.Capacity}}
  storageClassName: {{.StorageClassName}}
`

// K8sPVCSpecArgs contains the properties required to create a PVC yaml
type K8sPVCSpecArgs struct {
	Capacity        int64
	VolumeID        string
	ServicePlanName string
}

// GetStaticYaml creates a yaml definition of a PersistentVolumeClaim for Pre-Provisioned Volumes
func (p *K8sPVCSpecArgs) GetStaticYaml() string {
	m := map[string]string{
		"AccessMode":    string(ReadWriteOnce),
		"AccountSecret": com.AccountSecretClusterObjectName,
		"Capacity":      util.K8sSizeBytes(p.Capacity).String(),
		"LabelType":     K8sPvLabelType,
		"VolumeID":      p.VolumeID,
	}
	t, _ := template.New("").Parse(PvcSpecTemplateStatic)
	b := &bytes.Buffer{}
	_ = t.Execute(b, m)
	return b.String()
}

// GetDynamicYaml creates a yaml definition of a PersistentVolumeClaim for Dynamically-Provisioned volumes
func (p *K8sPVCSpecArgs) GetDynamicYaml() string {
	m := map[string]string{
		"AccessMode":             string(ReadWriteOnce),
		"AccountSecret":          com.AccountSecretClusterObjectName,
		"Capacity":               "1Gi",
		"NuvoServicePlanName":    p.ServicePlanName,
		"PvcSecretAnnotationKey": com.K8sPvcSecretAnnotationKey,
		"StorageClassName":       K8sStorageClassName(p.ServicePlanName),
	}
	t, _ := template.New("").Parse(PvcSpecTemplateDynamic)
	b := &bytes.Buffer{}
	_ = t.Execute(b, m)
	return b.String()
}

// GetStaticYaml4Flex creates a yaml definition of a PersistentVolumeClaim for Pre-Provisioned Volumes. for flex we dont set storage class
// Created for backward compatibility.
func (p *K8sPVCSpecArgs) GetStaticYaml4Flex() string {
	m := map[string]string{
		"AccessMode": string(ReadWriteOnce),
		"Capacity":   util.K8sSizeBytes(p.Capacity).String(),
		"VolumeID":   p.VolumeID,
		"LabelType":  K8sTypeLabelFlex,
	}
	t, _ := template.New("").Parse(PvcSpecTemplateStatic4Flex)
	b := &bytes.Buffer{}
	_ = t.Execute(b, m)
	return b.String()
}
