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
	"encoding/json"
	"log"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/util"

	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

var k8sFakeEvent = map[string]string{
	"/api/v1/namespaces/testNamespace/events": `{
		"apiVersion": "v1",
		"count": 6281,
		"eventTime": null,
		"firstTimestamp": "2018-11-28T00:23:22Z",
		"involvedObject": {
			"apiVersion": "v1",
			"fieldPath": "spec.containers{agentd}",
			"kind": "Pod",
			"name": "nuvoloso-node-6np84",
			"namespace": "testNamespace",
			"resourceVersion": "118372",
			"uid": "be23d874-f2a3-11e8-bee0-023793e6aaa4"
		},
		"kind": "Event",
		"lastTimestamp": "2018-11-28T23:12:53Z",
		"message": "Back-off restarting failed container",
		"metadata": {
			"creationTimestamp": "2018-11-28T23:12:53Z",
			"name": "nuvoloso-node-6np84.156b21edb44fc327",
			"namespace": "testNamespace",
			"resourceVersion": "991",
			"selfLink": "/api/v1/namespaces/nuvoloso-cluster/events/nuvoloso-node-6np84.156b21edb44fc327",
			"uid": "21ebf374-f363-11e8-bee0-023793e6aaa4"
		},
		"reason": "BackOff",
		"reportingComponent": "",
		"reportingInstance": "",
		"source": {
			"component": "kubelet",
			"host": "ip-172-20-49-82.us-west-2.compute.internal"
		},
		"type": "Warning"
	}`,
	"/api/v1/namespaces/testNamespace/events/testPodName.ServiceReady": `{
		"apiVersion": "v1",
		"count": 0,
		"eventTime": null,
		"firstTimestamp": "2018-11-28T00:23:22Z",
		"involvedObject": {
			"apiVersion": "v1",
			"fieldPath": "spec.containers{agentd}",
			"kind": "Pod",
			"name": "nuvoloso-node-6np84",
			"namespace": "testNamespace",
			"resourceVersion": "118372",
			"uid": "be23d874-f2a3-11e8-bee0-023793e6aaa4"
		},
		"kind": "Event",
		"lastTimestamp": "2018-11-28T23:12:53Z",
		"message": "Back-off restarting failed container",
		"metadata": {
			"creationTimestamp": "2018-11-28T23:12:53Z",
			"name": "nuvoloso-node-6np84.156b21edb44fc327",
			"namespace": "testNamespace",
			"resourceVersion": "991",
			"selfLink": "/api/v1/namespaces/nuvoloso-cluster/events/nuvoloso-node-6np84.156b21edb44fc327",
			"uid": "21ebf374-f363-11e8-bee0-023793e6aaa4"
		},
		"reason": "BackOff",
		"reportingComponent": "",
		"reportingInstance": "",
		"source": {
			"component": "kubelet",
			"host": "ip-172-20-49-82.us-west-2.compute.internal"
		},
		"type": "Warning"
	}`,
}

var k8sEventCountRes = map[string]string{
	"/api/v1/namespaces/testNamespace/events/testPodName.ServiceReady": `{
		"apiVersion": "v1",
		"count": 10,
		"eventTime": null,
		"firstTimestamp": "2018-11-28T00:23:22Z",
		"involvedObject": {
			"apiVersion": "v1",
			"fieldPath": "spec.containers{agentd}",
			"kind": "Pod",
			"name": "nuvoloso-node-6np84",
			"namespace": "testNamespace",
			"resourceVersion": "118372",
			"uid": "be23d874-f2a3-11e8-bee0-023793e6aaa4"
		},
		"kind": "Event",
		"lastTimestamp": "2018-11-28T23:12:53Z",
		"message": "Back-off restarting failed container",
		"metadata": {
			"creationTimestamp": "2018-11-28T23:12:53Z",
			"name": "nuvoloso-node-6np84.156b21edb44fc327",
			"namespace": "testNamespace",
			"resourceVersion": "991",
			"selfLink": "/api/v1/namespaces/nuvoloso-cluster/events/nuvoloso-node-6np84.156b21edb44fc327",
			"uid": "21ebf374-f363-11e8-bee0-023793e6aaa4"
		},
		"reason": "BackOff",
		"reportingComponent": "",
		"reportingInstance": "",
		"source": {
			"component": "kubelet",
			"host": "ip-172-20-49-82.us-west-2.compute.internal"
		},
		"type": "Warning"
	}`,
	"/api/v1/namespaces/testNamespaceErr/events/testName": `{
		"badres" : "ponce",
	}`,
}

func TestRecordIncident(t *testing.T) {
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
		data:       k8sFakeEvent,
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

	// Success
	savedPodNamespace := os.Getenv(NuvoEnvPodNamespace)
	savedPodName := os.Getenv(NuvoEnvPodName)
	savedPodUID := os.Getenv(NuvoEnvPodUID)
	os.Setenv(NuvoEnvPodNamespace, "testNamespace")
	os.Setenv(NuvoEnvPodName, "testPodName")
	os.Setenv(NuvoEnvPodUID, "testPodUID")
	defer func() {
		os.Setenv(NuvoEnvPodNamespace, savedPodNamespace)
		os.Setenv(NuvoEnvPodName, savedPodName)
		os.Setenv(NuvoEnvPodUID, savedPodUID)
	}()
	inc := &Incident{
		Severity: 1,
		Message:  "Some warning message",
	}
	res, err := c.RecordIncident(ctx, inc)
	assert.Nil(err)
	assert.NotNil(res)

	// Invalid input parameters
	inc = &Incident{
		Severity: 5, // doesnt exist
		Message:  "Some warning message",
	}
	res, err = c.RecordIncident(ctx, inc)
	assert.NotNil(err)
	assert.Nil(res)

	inc = &Incident{
		Severity: 1, // doesnt exist
		Message:  "",
	}
	res, err = c.RecordIncident(ctx, inc)
	assert.NotNil(err)
	assert.Nil(res)

	// Error
	inc = &Incident{
		Severity: 1,
		Message:  "Some warning message",
	}
	k8s.httpClient = nil
	k8s.env.Host = ""
	k8s.shp = ""
	res, err = c.RecordIncident(ctx, inc)
	tl.Logger().Info(err)
	assert.NotNil(err)
	assert.Nil(res)

}

func TestRecordCondition(t *testing.T) {
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
		data:       k8sFakeEvent,
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

	// check that all conditions are covered
	for i := ServiceReady; i < MaxCondition; i++ {
		o, found := k8sConditionMap[i]
		assert.True(found)
		assert.NotEmpty(o.Name)
		assert.True(util.Contains([]string{"Info", "Failure"}, o.Type))
		assert.NotEmpty(o.Reason)
		assert.NotEmpty(o.Message)
	}

	// Success
	savedPodNamespace := os.Getenv(NuvoEnvPodNamespace)
	savedPodName := os.Getenv(NuvoEnvPodName)
	savedPodUID := os.Getenv(NuvoEnvPodUID)
	os.Setenv(NuvoEnvPodNamespace, "testNamespace")
	os.Setenv(NuvoEnvPodName, "testPodName")
	os.Setenv(NuvoEnvPodUID, "testPodUID")
	defer func() {
		os.Setenv(NuvoEnvPodNamespace, savedPodNamespace)
		os.Setenv(NuvoEnvPodName, savedPodName)
		os.Setenv(NuvoEnvPodUID, savedPodUID)
	}()
	con := &Condition{
		Status: 1,
	}
	res, err := c.RecordCondition(ctx, con)
	assert.Nil(err)
	assert.NotNil(res)

	// Invalid input parameters
	con = &Condition{
		Status: MaxCondition, // invalid message
	}
	res, err = c.RecordCondition(ctx, con)
	assert.NotNil(err)
	assert.Nil(res)

	// Invalid input parameters
	con = &Condition{
		Status: 4, // Update error
	}
	res, err = c.RecordCondition(ctx, con)
	assert.NotNil(err)
	assert.Nil(res)

	// skip update because it was done recently
	k8s.conditionTimeMap[ServiceReady] = time.Now()
	con = &Condition{
		Status: 1,
	}
	res, err = c.RecordCondition(ctx, con)
	assert.Nil(err)
	assert.Nil(res)

	// Event update error
	os.Setenv(NuvoEnvPodNamespace, "testNamespaceErr")
	eca := &eventCreateArgs{
		Name: "testName",
	}
	_, err = k8s.eventUpdate(ctx, eca)
	assert.NotNil(err)

	// Error
	con = &Condition{
		Status: 0,
	}
	k8s.httpClient = nil
	k8s.env.Host = ""
	k8s.shp = ""
	res, err = c.RecordCondition(ctx, con)
	tl.Logger().Info(err)
	assert.NotNil(err)
	assert.Nil(res)

}

func TestEvCreateArgsToPostBody(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	eca := &eventCreateArgs{
		Name:           "fakename",
		NamePrefix:     "fakenamepre",
		Type:           "fakewarning",
		Message:        "fakemessage",
		Reason:         "fakereason",
		Count:          1,
		FirstTimeStamp: time.Now(),
		LastTimeStamp:  time.Now(),
	}

	var postBody eventPostBody
	postBody.Init(eca)
	jsonString, err := json.Marshal(postBody)
	assert.Nil(err)
	keyMap := make(map[string]interface{})
	err = json.Unmarshal(jsonString, &keyMap)
	_, exists := keyMap["apiVersion"]
	assert.True(exists)
	_, exists = keyMap["kind"]
	assert.True(exists)
	_, exists = keyMap["metadata"]
	assert.True(exists)
	_, exists = keyMap["involvedObject"]
	assert.True(exists)
	_, exists = keyMap["reason"]
	assert.True(exists)
	_, exists = keyMap["message"]
	assert.True(exists)
	_, exists = keyMap["firstTimestamp"]
	assert.True(exists)
	_, exists = keyMap["lastTimestamp"]
	assert.True(exists)
	_, exists = keyMap["count"]
	assert.True(exists)
	_, exists = keyMap["type"]
	assert.True(exists)
	_, exists = keyMap["source"]
	assert.True(exists)

}

func TestGetEventCount(t *testing.T) {
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
		data:       k8sEventCountRes,
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

	savedPodNamespace := os.Getenv(NuvoEnvPodNamespace)
	savedPodName := os.Getenv(NuvoEnvPodName)
	os.Setenv(NuvoEnvPodNamespace, "testNamespace")
	os.Setenv(NuvoEnvPodName, "testPodName")
	defer func() {
		os.Setenv(NuvoEnvPodNamespace, savedPodNamespace)
		os.Setenv(NuvoEnvPodName, savedPodName)
	}()
	con := &Condition{
		Status: 1,
	}
	count, _ := k8s.getEventCount(ctx, con)
	assert.Equal(int32(10), count)

	con.Status = 0
	count, err = k8s.getEventCount(ctx, con)
	assert.Equal(int32(0), count)
	assert.NotNil(err)
}

func TestConditionUpdateRequired(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c, err := NewClient(K8sClusterType)
	assert.Nil(err)
	c.SetDebugLogger(tl.Logger())
	k8s, ok := c.(*K8s)
	assert.True(ok)
	savedPodName := os.Getenv(NuvoEnvPodName)
	os.Setenv(NuvoEnvPodName, "testPodName")
	defer func() {
		os.Setenv(NuvoEnvPodName, savedPodName)
	}()

	// Cache ServiceReady and update
	k8s.conditionTimeMap = nil
	assert.Nil(k8s.conditionTimeMap)
	con := &Condition{
		Status: 1,
	}
	bT := time.Now()
	retVal := k8s.conditionUpdateRequired(con)
	aT := time.Now()
	assert.True(retVal)
	conTime, ok := k8s.conditionTimeMap[ServiceReady]
	assert.True(ok)
	assert.WithinDuration(aT, conTime, aT.Sub(bT))

	// Cache ServiceNotReady and remove ServiceReady
	con = &Condition{
		Status: 2,
	}
	bT = time.Now()
	retVal = k8s.conditionUpdateRequired(con)
	aT = time.Now()
	assert.True(retVal)
	_, ok = k8s.conditionTimeMap[ServiceReady]
	assert.False(ok)
	conTime, ok = k8s.conditionTimeMap[ServiceNotReady]
	assert.True(ok)
	assert.WithinDuration(aT, conTime, aT.Sub(bT))

	// ServiceNotReady exists and is within EventUpdatePeriod
	con = &Condition{
		Status: 2,
	}
	retVal = k8s.conditionUpdateRequired(con)
	for key, val := range k8s.conditionTimeMap {
		t.Log(key, val)
	}
	assert.False(retVal)

	// ServiceNotReady exists but is past EventUpdatePeriod
	con = &Condition{
		Status: 2,
	}
	k8s.conditionTimeMap[ServiceNotReady] = time.Time{}
	bT = time.Now()
	retVal = k8s.conditionUpdateRequired(con)
	aT = time.Now()
	conTime, ok = k8s.conditionTimeMap[ServiceNotReady]
	assert.True(retVal)
	assert.WithinDuration(aT, conTime, aT.Sub(bT))
}
