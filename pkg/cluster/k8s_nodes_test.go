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

var nodeFetchFakeRes = map[string]string{
	"/api/v1/nodes/my-node": `{
		"apiVersion": "v1",
		"kind": "Node",
		"metadata": {
			"creationTimestamp": "2019-05-03T17:42:34Z",
			"labels": {
                "failure-domain.beta.kubernetes.io/region": "us-west-2",
                "failure-domain.beta.kubernetes.io/zone": "us-west-2a",
                "kubernetes.io/role": "node"
			},
			"annotations": {
				"ann1": "annotation1",
				"ann2": "annotation2"
			},
			"name": "my-node",
			"resourceVersion": "3895",
			"selfLink": "/api/v1/nodes/my-node",
			"uid": "d583f4be-6dca-11e9-9669-021528f68bd4"
		}
	}`,
}

func TestNodeFetch(t *testing.T) {
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
		data:       nodeFetchFakeRes,
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
	nfa := &NodeFetchArgs{
		Name: "my-node",
	}
	nodeRes, err := c.NodeFetch(ctx, nfa)
	// labels to match those in the response example
	labels := map[string]string{
		"failure-domain.beta.kubernetes.io/region": "us-west-2",
		"failure-domain.beta.kubernetes.io/zone":   "us-west-2a",
		"kubernetes.io/role":                       "node",
	}
	annotations := map[string]string{
		"ann1": "annotation1",
		"ann2": "annotation2",
	}
	assert.Nil(err)
	assert.NotNil(nodeRes)
	assert.Equal("my-node", nodeRes.Name)
	assert.Equal("d583f4be-6dca-11e9-9669-021528f68bd4", nodeRes.UID)
	assert.EqualValues(labels, nodeRes.Labels)
	assert.EqualValues(annotations, nodeRes.Annotations)

	// Missing require fields
	nfa = &NodeFetchArgs{}
	nodeRes, err = c.NodeFetch(ctx, nfa)
	assert.NotNil(err)
	assert.Nil(nodeRes)

	// Incorrect required fields, fetch returns error
	nfa = &NodeFetchArgs{
		Name: "badNodeName",
	}
	nodeRes, err = c.NodeFetch(ctx, nfa)
	assert.NotNil(err)
	assert.Nil(nodeRes)
}
