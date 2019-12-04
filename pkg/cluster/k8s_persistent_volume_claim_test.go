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

var resultPVCpp4flex = `apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  # A PVC must be created with a namespace specific name - adjust to meet your needs:
  namespace: default
  name: my-pvc
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10
  # explicit empty storageClassName required in case a default storageClass exists
  storageClassName: ""
  selector:
    matchLabels:
      type: nuvo-vol
      volumeId: someID
`

var resultPVCpp = `---
# This sample PersistentVolumeClaim illustrates how to claim the
# PersistentVolume identified by its Nuvoloso volume identifier.
# Adjust the name and namespace to meet your needs and ensure that the
# 'nuvoloso-account' secret exists in this namespace.
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  namespace: default
  name: pvc-for-a-pre-provisioned-volume
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10
  storageClassName: ""
  selector:
    matchLabels:
      type: nuvoloso-volume
      volumeId: 02274068-015f-4827-b427-c806ef4e0ead
`

var resultPVCd = `---
# This sample PersistentVolumeClaim illustrates how to dynamically provision
# a PersistentVolume for the storage class 'nuvoloso-a-serviceplan'
# which corresponds to the Nuvoloso 'A ServicePlan' service plan.
# Adjust the name, namespace and capacity to meet your needs and ensure that
# the secret object named by the 'nuvoloso.com/provisioning-secret' annotation
# key exists in this namespace.
# The 'nuvoloso-account' secret referenced in this example is the same as that
# used to mount pre-provisioned PersistentVolumes.  Some customization of the
# dynamically provisioned volume is possible by creating a customization secret
# explicitly and referencing that secret instead.
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  namespace: default
  name: pvc-for-a-dynamically-provisioned-volume
  annotations:
    nuvoloso.com/provisioning-secret: nuvoloso-account
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: nuvoloso-a-serviceplan
`

func TestK8sPVCSpecArgsGetStaticYaml(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	retVal := resultPVCpp
	pvcArgs := K8sPVCSpecArgs{
		Capacity:        10,
		VolumeID:        "02274068-015f-4827-b427-c806ef4e0ead",
		ServicePlanName: "somename",
	}
	pvc := pvcArgs.GetStaticYaml()
	assert.Equal(retVal, pvc)
}

func TestK8sPVCSpecArgsGetDynamicYaml(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	retVal := resultPVCd
	pvcArgs := K8sPVCSpecArgs{
		ServicePlanName: "A ServicePlan",
	}
	pvc := pvcArgs.GetDynamicYaml()
	assert.Equal(retVal, pvc)
}

func TestK8sPVCSpecArgsGetStaticYaml4FLEX(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	retVal := resultPVCpp4flex
	pvcArgs := K8sPVCSpecArgs{
		Capacity: 10,
		VolumeID: "someID",
	}
	pvc := pvcArgs.GetStaticYaml4Flex()
	assert.Equal(retVal, pvc)
}
