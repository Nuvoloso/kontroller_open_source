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
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

var k8sPVWatchSample = `{"type":"ADDED","object":{"kind":"PersistentVolume","apiVersion":"v1","metadata":{"name":"nuvoloso-volume-6be07df1-e4c2-49c4-a691-fe29c85f6793","selfLink":"/api/v1/persistentvolumes/nuvoloso-volume-6be07df1-e4c2-49c4-a691-fe29c85f6793","uid":"ee8d1d18-b93b-11e9-8159-028cf555dd6a","resourceVersion":"110830","creationTimestamp":"2019-08-07T17:51:06Z","labels":{"accountId":"1c943f84-b641-4f1e-8f3e-ba706e9fef63","driverType":"csi","fsType":"ext4","sizeBytes":"2147483648","systemId":"b9bed48c-ae04-4d95-b1f8-85716fa1eca0","type":"nuvoloso-volume","volumeId":"6be07df1-e4c2-49c4-a691-fe29c85f6793"}},"spec":{"capacity":{"storage":"2Gi"},"csi":{"driver":"csi.nuvoloso.com","volumeHandle":"6be07df1-e4c2-49c4-a691-fe29c85f6793"},"accessModes":["ReadWriteOnce"],"persistentVolumeReclaimPolicy":"Retain","storageClassName":"nuvoloso-general","volumeMode":"Filesystem"},"status":{"phase":"Pending"}}}
{"type":"MODIFIED","object":{"kind":"PersistentVolume","apiVersion":"v1","metadata":{"name":"nuvoloso-volume-6be07df1-e4c2-49c4-a691-fe29c85f6793","selfLink":"/api/v1/persistentvolumes/nuvoloso-volume-6be07df1-e4c2-49c4-a691-fe29c85f6793","uid":"ee8d1d18-b93b-11e9-8159-028cf555dd6a","resourceVersion":"110831","creationTimestamp":"2019-08-07T17:51:06Z","labels":{"accountId":"1c943f84-b641-4f1e-8f3e-ba706e9fef63","driverType":"csi","fsType":"ext4","sizeBytes":"2147483648","systemId":"b9bed48c-ae04-4d95-b1f8-85716fa1eca0","type":"nuvoloso-volume","volumeId":"6be07df1-e4c2-49c4-a691-fe29c85f6793"}},"spec":{"capacity":{"storage":"2Gi"},"csi":{"driver":"csi.nuvoloso.com","volumeHandle":"6be07df1-e4c2-49c4-a691-fe29c85f6793"},"accessModes":["ReadWriteOnce"],"persistentVolumeReclaimPolicy":"Retain","storageClassName":"nuvoloso-general","volumeMode":"Filesystem"},"status":{"phase":"Available"}}}
{"type":"DELETED","object":{"kind":"PersistentVolume","apiVersion":"v1","metadata":{"name":"nuvoloso-volume-d6e97364-6358-4985-a086-112930fde889","selfLink":"/api/v1/persistentvolumes/nuvoloso-volume-d6e97364-6358-4985-a086-112930fde889","uid":"d4a88093-ba1a-11e9-8159-028cf555dd6a","resourceVersion":"338602","creationTimestamp":"2019-08-08T20:26:41Z","deletionTimestamp":"2019-08-09T18:29:21Z","deletionGracePeriodSeconds":0,"labels":{"accountId":"1c943f84-b641-4f1e-8f3e-ba706e9fef63","driverType":"csi","fsType":"ext4","sizeBytes":"1073741824","systemId":"b9bed48c-ae04-4d95-b1f8-85716fa1eca0","type":"nuvoloso-volume","volumeId":"d6e97364-6358-4985-a086-112930fde889"},"finalizers":["kubernetes.io/pv-protection"]},"spec":{"capacity":{"storage":"1Gi"},"csi":{"driver":"csi.nuvoloso.com","volumeHandle":"d6e97364-6358-4985-a086-112930fde889"},"accessModes":["ReadWriteOnce"],"persistentVolumeReclaimPolicy":"Retain","storageClassName":"nuvoloso-general","volumeMode":"Filesystem"},"status":{"phase":"Available"}}}`

func TestStartWatcher(t *testing.T) {
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
		data:       make(map[string]string),
		statusCode: make(map[string]int),
	}
	http.DefaultClient.Transport = ft
	ft.data["/foo&watch=1&resourceVersion="] = `{
		"fieldOneInt": 1,
		"fieldTwoString": "field 2 string",
		"field3Struct": {
			"field3aInt": 3,
			"field3bString": "field 3b string",
			"field3cSomething": "3c else"
		},
		"field4Something": "4 else"
	}`
	ft.statusCode["/foo&watch=1&resourceVersion="] = 200

	ctx := context.Background()
	c, err := NewClient(K8sClusterType)
	assert.Nil(err)
	c.SetDebugLogger(tl.Logger())
	k8s, ok := c.(*K8s)
	assert.True(ok)
	assert.Nil(k8s.env)
	defer func() {
		k8s.env = nil
		k8s.transport = nil
		k8s.shp = ""
	}()
	k8s.env = &K8sEnv{
		Token: ft.authToken,
		Host:  "host",
		Port:  6999,
	}
	expHP := "host:6999"
	expScheme := "https"

	// success
	fakeOps := &fakeK8sWatcherOps{}
	fakeOps.watchObjectWaitForSignal = true
	fakeOps.watchObjectSignalChan = make(chan struct{})
	fakeOps.watchObjectGenerator = func() interface{} { return &fakeK8sWatcherObject{} }
	w := &K8sWatcher{}
	w.c = k8s
	w.url = "/foo"
	w.resourceVersion = ""
	w.ops = fakeOps
	w.client = http.DefaultClient
	w.log = tl.Logger()
	assert.Nil(w.stoppedChan)
	assert.False(w.isOpen)
	assert.Nil(w.ioReadCloser)
	err = w.Start(ctx)
	assert.Nil(err)
	assert.True(ft.authTokenMatched)
	assert.True(ft.acceptJSONMatched)
	assert.Equal(expHP, ft.gotHostPort)
	assert.Equal(expScheme, ft.gotScheme)
	assert.Equal(expScheme+"://"+expHP, k8s.shp)
	assert.NotNil(w.stoppedChan)
	assert.True(w.isOpen)
	assert.NotNil(w.ioReadCloser)
	close(fakeOps.watchObjectSignalChan)
	fakeOps.watchObjectWaitForSignal = false
	w.Stop()
	assert.False(w.isOpen)
	tl.Flush()

	// error: invalid status code
	ft.statusCode["/foo&watch=1&resourceVersion="] = 400
	ft.data["/foo&watch=1&resourceVersion="] = "someerror"
	w = &K8sWatcher{}
	w.client = http.DefaultClient
	w.c = k8s
	w.url = "/foo"
	w.resourceVersion = ""
	w.log = tl.Logger()
	err = w.Start(ctx)
	assert.NotNil(err)
	assert.False(w.isOpen)
	tl.Flush()

	// error: Kubernetes Error
	ft.data["/k8serror&watch=1&resourceVersion="] = `{
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
		"code": 409
	}`
	ft.statusCode["/k8serror&watch=1&resourceVersion="] = 409
	w = &K8sWatcher{}
	w.c = k8s
	w.client = http.DefaultClient
	w.url = "/k8serror"
	w.resourceVersion = ""
	w.log = tl.Logger()
	err = w.Start(ctx)
	assert.Error(err)
	e, ok := err.(*k8sError)
	if assert.True(ok) && assert.NotNil(e) {
		assert.Equal("Some Error Message", e.Message)
		assert.Equal("error", string(e.Reason))
		assert.Equal(409, e.Code)
	}
	assert.False(w.isOpen)

	// error: client failure
	w.client = nil
	k8s.env = nil
	err = w.Start(ctx)
	assert.NotNil(err)
	assert.Regexp("invalid client", err.Error())
	assert.False(w.isOpen)
}

func TestMonitorBody(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	ctx := context.Background()

	fakeOps := &fakeK8sWatcherOps{}
	fakeOps.watchObjectGenerator = func() interface{} { return &K8sPVWatchEvent{} }
	fakeOps.watchEventCnt = 0
	w := &K8sWatcher{}
	w.ops = fakeOps
	w.ioReadCloser = ioutil.NopCloser(strings.NewReader(k8sPVWatchSample))
	w.isOpen = true
	w.stoppedChan = make(chan struct{})
	w.log = tl.Logger()
	w.monitorBody(ctx)
	assert.Equal(3, fakeOps.watchEventCnt)
	assert.False(w.isOpen)
	assert.Equal(1, tl.CountPattern("starting k8s watcher"))
	assert.Equal(1, tl.CountPattern("stopping k8s watcher"))
	tl.Flush()

	fakeOps = &fakeK8sWatcherOps{}
	fakeOps.watchObjectGenerator = func() interface{} { return &K8sPVWatchEvent{} }
	fakeOps.watchEventCnt = 0
	w = &K8sWatcher{}
	w.ops = fakeOps
	w.ioReadCloser = ioutil.NopCloser(strings.NewReader("Garbage"))
	w.isOpen = true
	w.stoppedChan = make(chan struct{})
	w.log = tl.Logger()
	w.monitorBody(ctx)
	assert.Equal(0, fakeOps.watchEventCnt)
	assert.False(w.isOpen)
	assert.Equal(1, tl.CountPattern("starting k8s watcher"))
	assert.Equal(1, tl.CountPattern("unable to decode k8s watcher event"))
	assert.Equal(1, tl.CountPattern("stopping k8s watcher"))
	tl.Flush()
}

type fakeK8sWatcherObject struct{}

type fakeK8sWatcherOps struct {
	watchEventInObj interface{}
	watchEventInCtx context.Context
	watchEventCnt   int

	watchObjectWaitForSignal bool
	watchObjectSignalChan    chan struct{}
	watchObjectGenerator     func() interface{}
}

func (fake *fakeK8sWatcherOps) WatchEvent(ctx context.Context, obj interface{}) {
	fake.watchEventInCtx = ctx
	fake.watchEventInObj = obj
	fake.watchEventCnt++
}

func (fake *fakeK8sWatcherOps) WatchObject() interface{} {
	if fake.watchObjectWaitForSignal {
		<-fake.watchObjectSignalChan
	}
	return fake.watchObjectGenerator()
}
