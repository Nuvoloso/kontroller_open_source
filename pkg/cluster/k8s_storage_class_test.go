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
	"net/http"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"

	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

var k8sFakeSC = map[string]string{
	"/apis/storage.k8s.io/v1/storageclasses": `{
		"apiVersion": "storage.k8s.io/v1",
		"kind": "StorageClass",
		"metadata": {
			"creationTimestamp": "2019-03-28T18:50:39Z",
			"name": "nuvoloso-general",
			"resourceVersion": "11175",
			"selfLink": "/apis/storage.k8s.io/v1/storageclasses/nuvoloso-general",
			"uid": "617fe848-518a-11e9-b1f9-022fbed1729c"
		},
		"parameters": {
			"csi.storage.k8s.io/node-publish-secret-name": "nuvo-account",
			"csi.storage.k8s.io/node-publish-secret-namespace": "${pvc.namespace}"
		},
		"provisioner": "csi.nuvoloso.com",
		"reclaimPolicy": "Retain",
		"volumeBindingMode": "Immediate"
	}`,
}

func TestServicePlanPublish(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	oldTr := http.DefaultClient.Transport
	defer func() {
		http.DefaultClient.Transport = oldTr

	}()
	ft := &k8sFakeTransport{
		t:          t,
		authToken:  "fakeAuthToken",
		data:       make(map[string]string),
		statusCode: make(map[string]int),
	}
	for k, v := range k8sFakeSC {
		ft.statusCode[k] = 200
		ft.data[k] = v
	}
	http.DefaultClient.Transport = ft
	c, err := NewClient(K8sClusterType)
	assert.Nil(err)
	c.SetDebugLogger(tl.Logger())
	k8s, ok := c.(*K8s)
	assert.True(ok)
	assert.Nil(k8s.env)
	assert.Nil(k8s.httpClient)
	defer func() {
		k8s.env = nil
		k8s.transport = nil
		k8s.httpClient = nil
		k8s.shp = ""
	}()
	k8s.env = &K8sEnv{
		Token: ft.authToken,
		Host:  "host",
		Port:  443,
		PodIP: "8.7.6.5",
	}
	k8s.httpClient = http.DefaultClient
	ctx := context.Background()

	sp := &models.ServicePlan{
		ServicePlanAllOf0: models.ServicePlanAllOf0{
			Meta: &models.ObjMeta{
				ID: "someid",
			},
		},
		ServicePlanMutable: models.ServicePlanMutable{
			Name: "general",
		},
	}
	cd, err := c.ServicePlanPublish(ctx, sp)
	assert.NotNil(cd)
	assert.Nil(err)

	// already exists error skipped from K8s
	ft.statusCode["/apis/storage.k8s.io/v1/storageclasses"] = 409
	ft.data["/apis/storage.k8s.io/v1/storageclasses"] = `{
		"kind": "Status",
		"apiVersion": "v1",
		"metadata": {},
		"status": "Failure",
		"message": "Some Error Message",
		"reason": "AlreadyExists",
		"details": {
			"name": "pv2",
			"kind": "persistentvolumes"
		},
		"code": 409
	}`
	cd, err = c.ServicePlanPublish(ctx, sp)
	assert.NotNil(cd)
	assert.Nil(err)

	// error returned from K8s
	ft.statusCode["/apis/storage.k8s.io/v1/storageclasses"] = 404
	ft.data["/apis/storage.k8s.io/v1/storageclasses"] = `{
		"kind": "Status",
		"apiVersion": "v1",
		"metadata": {},
		"status": "Failure",
		"message": "Some Error Message",
		"reason": "error",
		"details": {
			"name": "pv2",
			"kind": "persistentvolumes"
		},
		"code": 404
	}`
	cd, err = c.ServicePlanPublish(ctx, sp)
	assert.Nil(cd)
	assert.NotNil(err)
	assert.Regexp("Some Error Message", err.Error())

	sp.Name = ""
	cd, err = c.ServicePlanPublish(ctx, sp)
	assert.Nil(cd)
	assert.NotNil(err)
	assert.Regexp("invalid arguments", err.Error())

	// error
	sp.Name = "someName"
	k8s.httpClient = nil
	k8s.env.Host = ""
	k8s.shp = ""
	cd, err = c.ServicePlanPublish(ctx, sp)
	tl.Logger().Info(err)
	assert.NotNil(err)
	assert.Nil(cd)

	// Test scPostBody
	pb := &scPostBody{}
	scArgs := &storageClassCreateArgs{
		name:            K8sStorageClassName("spName"),
		secretName:      com.AccountSecretClusterObjectName,
		servicePlanName: string("spName"),
		servicePlanID:   string("spID"),
	}
	pb.Init(scArgs)
	expPB := &scPostBody{
		APIVersion: "storage.k8s.io/v1",
		Kind:       "StorageClass",
		Metadata: &ObjectMeta{
			Name: scArgs.name,
		},
		Provisioner: com.CSIDriverName,
		Parameters: &K8sScParameters{
			NodePublishSecretName:      fmt.Sprintf("${pvc.annotations['%s']}", com.K8sPvcSecretAnnotationKey),
			NodePublishSecretNamespace: "${pvc.namespace}",
			ServicePlanID:              scArgs.servicePlanID,
			ServicePlanName:            scArgs.servicePlanName,
		},
	}
	assert.Equal(expPB, pb)
}
