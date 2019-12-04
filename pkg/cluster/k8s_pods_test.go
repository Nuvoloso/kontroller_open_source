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
	"context"
	"log"
	"net/http"
	"os"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

var podFetchFakeRes = map[string]string{
	"/api/v1/namespaces/default/pods/my-pod": `{
		"apiVersion": "v1",
		"kind": "Pod",
		"metadata": {
			"creationTimestamp": "2019-05-03T17:42:34Z",
			"generateName": "sql-wordpress-20190503104231-6bb648bb45-",
			"labels": {
				"app": "wordpress",
				"pod-template-hash": "6bb648bb45",
				"tier": "mysql"
			},
			"annotations": {
				"ann1": "annotation1",
				"ann2": "annotation2"
			},
			"name": "my-pod",
			"namespace": "default",
			"ownerReferences": [
				{
					"apiVersion": "apps/v1",
					"blockOwnerDeletion": true,
					"controller": true,
					"kind": "ReplicaSet",
					"name": "podOwner",
					"uid": "ownerID"
				}
			],
			"resourceVersion": "3895",
			"selfLink": "/api/v1/namespaces/one/pods/sql-wordpress-20190503104231-6bb648bb45-hh9kh",
			"uid": "d583f4be-6dca-11e9-9669-021528f68bd4"
		}
	}`,
}

const podListResValue = `{
	"apiVersion": "v1",
	"items": [
	  {
		"apiVersion": "v1",
		"kind": "Pod",
		"metadata": {
		  "creationTimestamp": "2019-05-03T17:42:34Z",
		  "generateName": "sql-wordpress-20190503104231-6bb648bb45-",
		  "labels": {
			"app": "wordpress",
			"controller-revision-hash": "65b8d79b58",
			"pod-template-generation": "1"
		  },
		  "annotations": {
			"ann1": "annotation1",
			"ann2": "annotation2"
		  },
		  "name": "my-pod",
		  "namespace": "default",
		  "ownerReferences": [
			{
			  "apiVersion": "apps/v1",
			  "blockOwnerDeletion": true,
			  "controller": true,
			  "kind": "DaemonSet",
			  "name": "podOwner",
			  "uid": "ownerID"
			}
		  ],
		  "resourceVersion": "3895",
		  "selfLink": "/api/v1/namespaces/one/pods/sql-wordpress-20190503104231-6bb648bb45-hh9kh",
		  "uid": "d583f4be-6dca-11e9-9669-021528f68bd4"
		},
		"spec": {},
		"status": {}
	  }
	]
}`

var podListFakeRes = map[string]string{
	"/api/v1/namespaces/default/pods":                                                                                        podListResValue,
	"/api/v1/namespaces/default/pods?fieldSelector=spec.nodeName%3Dmy-node.local":                                            podListResValue,
	"/api/v1/namespaces/default/pods?labelSelector=a%3Db,env+in+%28prod%2Cqa%29":                                             podListResValue,
	"/api/v1/namespaces/default/pods?fieldSelector=spec.nodeName%3Dmy-node.local&labelSelector=a%3Db,env+in+%28prod%2Cqa%29": podListResValue,
}

func TestPodFetch(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	b := &bytes.Buffer{}
	log.SetOutput(b) // http client logs in this case, capture it so UT does not log
	oldTr := http.DefaultClient.Transport
	defer func() {
		http.DefaultClient.Transport = oldTr
		log.SetOutput(os.Stderr)
	}()
	ft := &k8sFakeTransport{
		t:          t,
		authToken:  "fakeAuthToken",
		data:       podFetchFakeRes,
		statusCode: make(map[string]int),
	}
	for k := range ft.data {
		ft.statusCode[k] = 200
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

	// success
	pfa := &PodFetchArgs{
		Name:      "my-pod",
		Namespace: "default",
	}
	podRes, err := c.PodFetch(ctx, pfa)
	// labels to match those in the response example
	labels := map[string]string{
		"app":               "wordpress",
		"pod-template-hash": "6bb648bb45",
		"tier":              "mysql",
	}
	annotations := map[string]string{
		"ann1": "annotation1",
		"ann2": "annotation2",
	}
	assert.Nil(err)
	assert.NotNil(podRes)
	assert.Equal("podOwner", podRes.ControllerName)
	assert.Equal("ReplicaSet", podRes.ControllerKind)
	assert.Equal("ownerID", podRes.ControllerID)
	assert.EqualValues(labels, podRes.Labels)
	assert.EqualValues(annotations, podRes.Annotations)

	// Missing require fields
	pfa = &PodFetchArgs{
		Namespace: "default",
	}
	podRes, err = c.PodFetch(ctx, pfa)
	assert.NotNil(err)
	assert.Nil(podRes)

	// Incorrect required fields, fetch returns error
	pfa = &PodFetchArgs{
		Name:      "badPodName",
		Namespace: "default",
	}
	podRes, err = c.PodFetch(ctx, pfa)
	assert.NotNil(err)
	assert.Nil(podRes)
}

func TestPodList(t *testing.T) {
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
		data:       podListFakeRes,
		statusCode: make(map[string]int),
	}
	for k := range ft.data {
		ft.statusCode[k] = 200
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
	k8s.httpClient = http.DefaultClient

	// invalid args
	ctx := context.Background()
	pla := &PodListArgs{}
	res, err := c.PodList(ctx, pla)
	assert.Nil(res)
	assert.Regexp("invalid values", err)

	// K8sClientGetJSON, bad env
	pla = &PodListArgs{
		Namespace: "default",
	}
	res, err = c.PodList(ctx, pla)
	assert.Nil(res)
	assert.Regexp("not set", err)

	k8s.env = &K8sEnv{
		Token: ft.authToken,
		Host:  "host",
		Port:  443,
		PodIP: "8.7.6.5",
	}

	// success, only namespace
	labels := map[string]string{
		"app":                      "wordpress",
		"controller-revision-hash": "65b8d79b58",
		"pod-template-generation":  "1",
	}
	annotations := map[string]string{
		"ann1": "annotation1",
		"ann2": "annotation2",
	}
	exp := &PodObj{
		Name:           "my-pod",
		Namespace:      "default",
		Labels:         labels,
		Annotations:    annotations,
		ControllerName: "podOwner",
		ControllerKind: "DaemonSet",
		ControllerID:   "ownerID",
	}
	res, err = c.PodList(ctx, pla)
	assert.NoError(err)
	if len(res) == 1 {
		podRes := res[0]
		_, ok = podRes.Raw.(*K8sPod)
		assert.True(ok)
		exp.Raw = podRes.Raw
		assert.Equal(exp, podRes)
	}

	// success, namespace and node
	pla.NodeName = "my-node.local"
	res, err = c.PodList(ctx, pla)
	assert.NoError(err)
	if len(res) == 1 {
		podRes := res[0]
		_, ok = podRes.Raw.(*K8sPod)
		assert.True(ok)
		exp.Raw = podRes.Raw
		assert.Equal(exp, podRes)
	}

	// success, namespace and labels
	pla.NodeName = ""
	pla.Labels = []string{"a=b", "env in (prod,qa)"}
	res, err = c.PodList(ctx, pla)
	assert.NoError(err)
	if len(res) == 1 {
		podRes := res[0]
		_, ok = podRes.Raw.(*K8sPod)
		assert.True(ok)
		exp.Raw = podRes.Raw
		assert.Equal(exp, podRes)
	}

	// success, all params
	pla.NodeName = "my-node.local"
	pla.Labels = []string{"a=b", "env in (prod,qa)"}
	res, err = c.PodList(ctx, pla)
	assert.NoError(err)
	if len(res) == 1 {
		podRes := res[0]
		_, ok = podRes.Raw.(*K8sPod)
		assert.True(ok)
		exp.Raw = podRes.Raw
		assert.Equal(exp, podRes)
	}
}
