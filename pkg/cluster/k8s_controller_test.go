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
	"net/http"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

var controllerFetchFakeRes = map[string]string{
	"/apis/apps/v1/namespaces/default/daemonsets/fake": `{
		"apiVersion": "apps/v1",
		"kind": "DaemonSet",
		"metadata": {
		  "annotations": {
			"ann0": "annotation0",
			"ann1": "annotation1"
		  },
		  "creationTimestamp": "2019-08-20T21:26:35Z",
		  "generation": 1,
		  "name": "fake",
		  "namespace": "default",
		  "resourceVersion": "8757736",
		  "selfLink": "/apis/apps/v1/namespaces/default/daemonsets/fake",
		  "uid": "3039d761-c391-11e9-ad38-066d8cc99636"
		},
		"spec": {
		  "revisionHistoryLimit": 10,
		  "selector": {
			"matchLabels": {
			  "app": "fake"
			}
		  },
		  "template": {
			"metadata": {
			  "creationTimestamp": null,
			  "labels": {
				"app": "fake"
			  }
			},
			"spec": {
			  "containers": [],
        	  "dnsPolicy": "ClusterFirst",
			  "restartPolicy": "Always",
        	  "schedulerName": "default-scheduler",
        	  "securityContext": {},
        	  "serviceAccount": "nuvoloso-agent-account",
        	  "serviceAccountName": "nuvoloso-agent-account",
        	  "terminationGracePeriodSeconds": 30,
        	  "volumes": []
			}
		  },
		  "updateStrategy": {
			"rollingUpdate": {
			  "maxUnavailable": 1
			},
			"type": "RollingUpdate"
		  }
		},
		"status": {
		  "currentNumberScheduled": 1,
		  "desiredNumberScheduled": 3,
		  "numberAvailable": 1,
		  "numberMisscheduled": 0,
		  "numberReady": 2,
		  "observedGeneration": 1,
		  "updatedNumberScheduled": 1
		}
	}`,
	"/apis/apps/v1/namespaces/default/statefulsets/fake": `{
		"apiVersion": "apps/v1",
		"kind": "StatefulSet",
		"metadata": {
		  "annotations": {
			"ann1": "annotation1",
			"ann2": "annotation2"
		  },
		  "creationTimestamp": "2019-08-01T15:20:05Z",
		  "generation": 1,
		  "name": "fake",
		  "namespace": "default",
		  "resourceVersion": "6545847",
		  "selfLink": "/apis/apps/v1/namespaces/default/statefulsets/fake",
		  "uid": "d76641fc-b46f-11e9-ad38-066d8cc99636"
		},
		"spec": {
		  "podManagementPolicy": "OrderedReady",
		  "replicas": 1,
		  "revisionHistoryLimit": 10,
		  "selector": {
			"matchLabels": {
			  "app": "nuvoloso-fake"
			}
		  },
		  "serviceName": "fake",
		  "template": {
			"metadata": {
			  "creationTimestamp": null,
			  "labels": {
				"app": "nuvoloso-fake"
			  }
			},
			"spec": {
			  "containers": [
				{
				  "args": [
					"--sslMode",
					"requireSSL",
					"--sslPEMKeyFile",
					"/fakePEM",
					"--sslCAFile",
					"/etc/nuvoloso/tls/caCert",
					"--sslDisabledProtocols",
					"TLS1_0,TLS1_1"
				  ],
				  "env": [
					{
					  "name": "FAKE_CRT",
					  "value": "/etc/nuvoloso/tls/fakeCert"
					},
					{
					  "name": "FAKE_KEY",
					  "value": "/etc/nuvoloso/tls/fakeKey"
					},
					{
					  "name": "FAKE_PEM",
					  "value": "/fakePEM"
					}
				  ],
				  "image": "407798037446.dkr.ecr.us-west-2.amazonaws.com/nuvoloso/fake:dlc",
				  "imagePullPolicy": "Always",
				  "name": "mongo",
				  "ports": [
					{
					  "containerPort": 27017,
					  "protocol": "TCP"
					}
				  ],
				  "resources": {},
				  "terminationMessagePath": "/dev/termination-log",
				  "terminationMessagePolicy": "File",
				  "volumeMounts": [
					{
					  "mountPath": "/data/db",
					  "name": "fakestorage",
					  "subPath": "fake"
					},
					{
					  "mountPath": "/etc/nuvoloso/tls",
					  "name": "tls",
					  "readOnly": true
					}
				  ]
				}
			  ],
			  "dnsPolicy": "ClusterFirst",
			  "restartPolicy": "Always",
			  "schedulerName": "default-scheduler",
			  "securityContext": {},
			  "terminationGracePeriodSeconds": 30,
			  "volumes": [
				{
				  "name": "tls",
				  "secret": {
					"defaultMode": 420,
					"secretName": "nuvoloso-tls"
				  }
				}
			  ]
			}
		  },
		  "updateStrategy": {
			"rollingUpdate": {
			  "partition": 0
			},
			"type": "RollingUpdate"
		  },
		  "volumeClaimTemplates": [
			{
			  "metadata": {
				"creationTimestamp": null,
				"name": "fakestorage"
			  },
			  "spec": {
				"accessModes": [
				  "ReadWriteOnce"
				],
				"dataSource": null,
				"resources": {
				  "requests": {
					"storage": "3Gi"
				  }
				},
				"storageClassName": "gp2",
				"volumeMode": "Filesystem"
			  },
			  "status": {
				"phase": "Pending"
			  }
			}
		  ]
		},
		"status": {
		  "collisionCount": 0,
		  "currentReplicas": 1,
		  "currentRevision": "fake-584d7bdfb8",
		  "observedGeneration": 1,
		  "readyReplicas": 1,
		  "replicas": 1,
		  "updateRevision": "fake-584d7bdfb8",
		  "updatedReplicas": 1
		}
	}`,
}

func TestControllerFetch(t *testing.T) {
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
		data:       controllerFetchFakeRes,
		statusCode: make(map[string]int),
	}
	for k := range ft.data {
		ft.statusCode[k] = 200
	}
	http.DefaultClient.Transport = ft

	c, err := NewClient(K8sClusterType)
	assert.NoError(err)
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

	// K8sClientGetJSON, bad env
	ctx := context.Background()
	cfa := &ControllerFetchArgs{
		Kind:      "DaemonSet",
		Name:      "fake",
		Namespace: "default",
	}
	res, err := c.ControllerFetch(ctx, cfa)
	assert.Nil(res)
	assert.Error(err)

	cfa = &ControllerFetchArgs{
		Kind:      "StatefulSet",
		Name:      "fake",
		Namespace: "default",
	}
	res, err = c.ControllerFetch(ctx, cfa)
	assert.Nil(res)
	assert.Error(err)

	k8s.env = &K8sEnv{
		Token: ft.authToken,
		Host:  "host",
		Port:  443,
		PodIP: "8.7.6.5",
	}

	// success, StatefulSet
	res, err = c.ControllerFetch(ctx, cfa)
	annotations := map[string]string{
		"ann1": "annotation1",
		"ann2": "annotation2",
	}
	assert.NoError(err)
	if assert.NotNil(res) {
		assert.Equal("StatefulSet", res.Kind)
		assert.Equal("fake", res.Name)
		assert.Equal("fake", res.ServiceName)
		assert.Equal("default", res.Namespace)
		assert.Equal([]string{"app=nuvoloso-fake"}, res.Selectors)
		assert.EqualValues(1, res.Replicas)
		assert.EqualValues(1, res.ReadyReplicas)
		assert.Equal(annotations, res.Annotations)
		assert.Equal("d76641fc-b46f-11e9-ad38-066d8cc99636", res.UID)
		_, ok = res.Raw.(*K8sStatefulSet)
		assert.True(ok)
	}

	// success, DaemonSet
	cfa = &ControllerFetchArgs{
		Kind:      "DaemonSet",
		Name:      "fake",
		Namespace: "default",
	}
	res, err = c.ControllerFetch(ctx, cfa)
	annotations = map[string]string{
		"ann0": "annotation0",
		"ann1": "annotation1",
	}
	assert.NoError(err)
	if assert.NotNil(res) {
		assert.Equal("DaemonSet", res.Kind)
		assert.Equal("fake", res.Name)
		assert.Empty(res.ServiceName)
		assert.Equal("default", res.Namespace)
		assert.Equal([]string{"app=fake"}, res.Selectors)
		assert.EqualValues(3, res.Replicas)
		assert.EqualValues(2, res.ReadyReplicas)
		assert.Equal(annotations, res.Annotations)
		assert.Equal("3039d761-c391-11e9-ad38-066d8cc99636", res.UID)
		_, ok = res.Raw.(*K8sDaemonSet)
		assert.True(ok)
	}

	// Missing require fields
	cfa = &ControllerFetchArgs{
		Namespace: "default",
	}
	res, err = c.ControllerFetch(ctx, cfa)
	assert.Error(err)
	assert.Nil(res)

	// bad kind
	cfa = &ControllerFetchArgs{
		Kind:      "Pod",
		Name:      "fake",
		Namespace: "default",
	}
	res, err = c.ControllerFetch(ctx, cfa)
	assert.Error(err)
	assert.Nil(res)

	// Incorrect required fields, fetch returns error
	cfa = &ControllerFetchArgs{
		Name:      "badControllerName",
		Namespace: "default",
	}
	res, err = c.ControllerFetch(ctx, cfa)
	assert.Error(err)
	assert.Nil(res)
}
