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
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

var testCert []byte

func init() {
	testCert, _ = ioutil.ReadFile("./testdata/ca.crt")
	os.Unsetenv(K8sEnvServerHost)
	os.Unsetenv(K8sEnvServerPort)
	os.Unsetenv(NuvoEnvPodIP)
	os.Unsetenv(NuvoEnvPodName)
}

type k8sFakeTransport struct {
	t                 *testing.T
	authToken         string
	authTokenMatched  bool
	acceptJSONMatched bool
	gotHostPort       string
	gotScheme         string
	data              map[string]string
	statusCode        map[string]int
}

func (tr *k8sFakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if tr.authToken != "" {
		tr.authTokenMatched = tr.authToken == req.Header.Get("Authorization")
	}
	tr.acceptJSONMatched = req.Header.Get("Accept") == "application/json"
	tr.gotHostPort = req.URL.Host
	tr.gotScheme = req.URL.Scheme
	path := req.URL.Path
	if req.URL.RawQuery != "" {
		path = path + "?" + req.URL.RawQuery
	}
	resp := &http.Response{
		Header:     make(http.Header),
		Request:    req,
		StatusCode: tr.statusCode[path], //http.StatusOK,
	}
	body, ok := tr.data[path]
	if !ok {
		resp.StatusCode = http.StatusBadGateway
		return resp, fmt.Errorf("bad path")
	}
	tr.t.Log("path:", path, body)
	resp.Body = ioutil.NopCloser(strings.NewReader(body))
	return resp, nil
}

var k8sFakeCluster = map[string]string{
	"/version": `{
		"major": "1",
		"minor": "7",
		"gitVersion": "v1.7.8",
		"gitCommit": "bc6162cc70b4a39a7f39391564e0dd0be60b39e9",
		"gitTreeState": "clean",
		"buildDate": "2017-10-05T06:35:40Z",
		"goVersion": "go1.8.3",
		"compiler": "gc",
		"platform": "linux/amd64"
	  }`,
	"/api/v1/namespaces/default/services/kubernetes": `{
		"kind": "Service",
		"apiVersion": "v1",
		"metadata": {
		  "name": "kubernetes",
		  "namespace": "default",
		  "selfLink": "/api/v1/namespaces/default/services/kubernetes",
		  "uid": "c981db76-bda0-11e7-b2ce-02a3152c8208",
		  "resourceVersion": "8",
		  "creationTimestamp": "2017-10-30T18:33:13Z",
		  "labels": {
			"component": "apiserver",
			"provider": "kubernetes"
		  }
		},
		"spec": {
		  "ports": [
			{
			  "name": "https",
			  "protocol": "TCP",
			  "port": 443,
			  "targetPort": 443
			}
		  ],
		  "clusterIP": "100.64.0.1",
		  "type": "ClusterIP",
		  "sessionAffinity": "ClientIP"
		},
		"status": {
		  "loadBalancer": {}
		}
	  }`,
	"/api/v1/namespaces/nuvoloso-management/services/nuvo-https": `{
		"apiVersion": "v1",
		"kind": "Service",
		"metadata": {
			"creationTimestamp": "2019-05-22T22:03:20Z",
			"name": "nuvo-https",
			"namespace": "nuvoloso-management",
			"resourceVersion": "2010",
			"selfLink": "/api/v1/namespaces/nuvoloso-management/services/nuvo-https",
			"uid": "6919e403-7cdd-11e9-94f9-0208099e0bf4"
		},
		"spec": {
			"clusterIP": "100.66.108.10",
			"externalTrafficPolicy": "Cluster",
			"ports": [
				{
					"nodePort": 30981,
					"port": 443,
					"protocol": "TCP",
					"targetPort": 443
				}
			],
			"selector": {
				"app": "nuvoloso"
			},
			"sessionAffinity": "None",
			"type": "LoadBalancer"
		},
		"status": {
			"loadBalancer": {
				"ingress": [
					{
						"hostname": "a6919e4037cdd11e994f90208099e0bf-1863363181.us-west-2.elb.amazonaws.com"
					}
				]
			}
		}
	}`,
	"/api/v1/namespaces/nuvoloso-management/services/nuvo-https-both": `{
		"apiVersion": "v1",
		"kind": "Service",
		"metadata": {
			"creationTimestamp": "2019-05-22T22:03:20Z",
			"name": "nuvo-https-both",
			"namespace": "nuvoloso-management",
			"resourceVersion": "2010",
			"selfLink": "/api/v1/namespaces/nuvoloso-management/services/nuvo-https-both",
			"uid": "6919e403-7cdd-11e9-94f9-0208099e0bf4"
		},
		"spec": {
			"clusterIP": "100.66.108.10",
			"externalTrafficPolicy": "Cluster",
			"ports": [
				{
					"nodePort": 30981,
					"port": 443,
					"protocol": "TCP",
					"targetPort": 443
				}
			],
			"selector": {
				"app": "nuvoloso"
			},
			"sessionAffinity": "None",
			"type": "LoadBalancer"
		},
		"status": {
			"loadBalancer": {
				"ingress": [
					{
						"hostname": "a6919e4037cdd11e994f90208099e0bf-1863363181.us-west-2.elb.amazonaws.com",
						"ip": "1.2.3.4"
					}
				]
			}
		}
	}`,
	"/api/v1/namespaces/nuvoloso-management/services/nuvo-https-ip": `{
		"apiVersion": "v1",
		"kind": "Service",
		"metadata": {
			"creationTimestamp": "2019-05-22T22:03:20Z",
			"name": "nuvo-https-ip",
			"namespace": "nuvoloso-management",
			"resourceVersion": "2010",
			"selfLink": "/api/v1/namespaces/nuvoloso-management/services/nuvo-https-ip",
			"uid": "6919e403-7cdd-11e9-94f9-0208099e0bf4"
		},
		"spec": {
			"clusterIP": "100.66.108.10",
			"externalTrafficPolicy": "Cluster",
			"ports": [
				{
					"nodePort": 30981,
					"port": 443,
					"protocol": "TCP",
					"targetPort": 443
				}
			],
			"selector": {
				"app": "nuvoloso"
			},
			"sessionAffinity": "None",
			"type": "LoadBalancer"
		},
		"status": {
			"loadBalancer": {
				"ingress": [
					{
						"ip": "1.2.3.4"
					}
				]
			}
		}
	}`,
	"/api/v1/namespaces/nuvoloso-management/services/badServicetype": `{
		"spec": {
			"type": "NOtLoadBalancer"
		},
		"status": {
			"loadBalancer": {
				"ingress": [
					{
						"hostname": "a6919e4037cdd11e994f90208099e0bf-1863363181.us-west-2.elb.amazonaws.com"
					}
				]
			}
		}
	}`,
	"/api/v1/namespaces/nuvoloso-management/services/badServiceHostname": `{
		"spec": {
			"type": "LoadBalancer"
		},
		"status": {
			"loadBalancer": {
				"ingress": [
					{
					}
				]
			}
		}
	}`,
}

var pvFetchResp = `{
	"apiVersion": "v1",
	"items": [
		{
			"apiVersion": "v1",
			"kind": "PersistentVolume",
			"metadata": {
				"annotations": {
					"pv.kubernetes.io/bound-by-controller": "yes"
				},
				"creationTimestamp": "2018-11-02T21:52:57Z",
				"finalizers": [
					"kubernetes.io/pv-protection"
				],
				"labels": {
					"matcher": "csi-matcher",
					"type": "csi-vol"
				},
				"name": "my-manually-created-pv",
				"namespace": "",
				"resourceVersion": "1163063",
				"selfLink": "/api/v1/persistentvolumes/my-manually-created-pv",
				"uid": "a8b30b76-dee9-11e8-ba47-02e2883b527e"
			},
			"spec": {
				"accessModes": [
					"ReadWriteOnce"
				],
				"capacity": {
					"storage": "5Gi"
				},
				"claimRef": {
					"apiVersion": "v1",
					"kind": "PersistentVolumeClaim",
					"name": "pg-pv-claim",
					"namespace": "randns",
					"resourceVersion": "1163060",
					"uid": "b7c44663-dee9-11e8-ba47-02e2883b527e"
				},
				"csi": {
					"driver": "csi.nuvoloso.com",
					"nodePublishSecretRef": {
						"name": "mysecret",
						"namespace": "randns"
					},
					"volumeHandle": "e7ac80e4-85e5-4152-9f87-c3494c6d8648"
				},
				"persistentVolumeReclaimPolicy": "Retain"
			},
			"status": {
				"phase": "Bound"
			}
		},
		{
			"apiVersion": "v1",
			"kind": "PersistentVolume",
			"metadata": {
				"annotations": {
					"pv.kubernetes.io/bound-by-controller": "yes"
				},
				"creationTimestamp": "2018-11-05T23:28:47Z",
				"finalizers": [
					"kubernetes.io/pv-protection"
				],
				"labels": {
					"account": "212216c6-e661-4f8a-a8d6-773f63f975ab",
					"nuvomatcher": "storage-ext4-d9837354-b889-4add-8c76-92eca54f36ca",
					"type": "nuvoloso-volume"
				},
				"name": "nuvoloso-volume-for-d9837354-b889-4add-8c76-92eca54f36ca",
				"namespace": "",
				"resourceVersion": "1482616",
				"selfLink": "/api/v1/persistentvolumes/nuvoloso-volume-for-d9837354-b889-4add-8c76-92eca54f36ca",
				"uid": "8b4637c3-e152-11e8-ba47-02e2883b527e"
			},
			"spec": {
				"accessModes": [
					"ReadWriteOnce"
				],
				"capacity": {
					"storage": "49Gi"
				},
				"claimRef": {
					"apiVersion": "v1",
					"kind": "PersistentVolumeClaim",
					"name": "mysql-pv-claim",
					"namespace": "default",
					"resourceVersion": "1482613",
					"uid": "3d3c7bd5-e153-11e8-ba47-02e2883b527e"
				},
				"flexVolume": {
					"driver": "nuvoloso.com/nuvo",
					"fsType": "ext4",
					"options": {
						"nuvoSystemId": "05371bd2-9244-4e9d-9d4e-4501ba23c41b",
						"nuvoVolumeId": "d9837354-b889-4add-8c76-92eca54f36ca"
					}
				},
				"persistentVolumeReclaimPolicy": "Retain"
			},
			"status": {
				"phase": "Bound"
			}
		}
	],
	"kind": "List",
	"metadata": {
		"resourceVersion": "",
		"selfLink": ""
	}
}`

func TestK8sLoadEnv(t *testing.T) {
	assert := assert.New(t)
	defer func() {
		os.Unsetenv(K8sEnvServerHost)
		os.Unsetenv(K8sEnvServerPort)
		os.Unsetenv(NuvoEnvPodIP)
		os.Unsetenv(NuvoEnvPodName)
	}()

	_, err := NewClient("not" + K8sClusterType)
	assert.NotNil(err)
	assert.Regexp("invalid cluster type", err.Error())

	c, err := NewClient(K8sClusterType)
	assert.Nil(err)
	k8s, ok := c.(*K8s)
	assert.True(ok)
	assert.Nil(k8s.env)
	defer func() { k8s.env = nil }()

	err = k8s.loadEnvData()
	assert.NotNil(err)
	assert.Regexp(K8sEnvServerHost+".*"+K8sEnvServerPort+".*"+NuvoEnvPodIP+".*"+NuvoEnvPodName, err.Error())
	assert.Nil(k8s.env)

	h := "host"
	os.Setenv(K8sEnvServerHost, h)
	err = k8s.loadEnvData()
	assert.NotNil(err)
	assert.Regexp(K8sEnvServerHost+".*"+K8sEnvServerPort+".*"+NuvoEnvPodIP+".*"+NuvoEnvPodName, err.Error())
	assert.Nil(k8s.env)

	os.Setenv(K8sEnvServerPort, "NotANumber")
	err = k8s.loadEnvData()
	assert.NotNil(err)
	assert.Regexp(K8sEnvServerHost+".*"+K8sEnvServerPort+".*"+NuvoEnvPodIP+".*"+NuvoEnvPodName, err.Error())
	assert.Nil(k8s.env)

	os.Setenv(NuvoEnvPodIP, "1.2.3.4")
	err = k8s.loadEnvData()
	assert.NotNil(err)
	assert.Regexp(K8sEnvServerHost+".*"+K8sEnvServerPort+".*"+NuvoEnvPodIP+".*"+NuvoEnvPodName, err.Error())
	assert.Nil(k8s.env)

	os.Setenv(NuvoEnvPodName, "pod-name")
	err = k8s.loadEnvData()
	assert.NotNil(err)
	assert.Regexp("invalid "+K8sEnvServerPort, err.Error())
	assert.Nil(k8s.env)

	os.Setenv(K8sEnvServerPort, "0")
	err = k8s.loadEnvData()
	assert.NotNil(err)
	assert.Regexp("invalid "+K8sEnvServerPort, err.Error())
	assert.Nil(k8s.env)

	p := 443
	os.Setenv(K8sEnvServerPort, fmt.Sprintf("%d", p))
	savedSASecretPath := k8sSASecretPath
	defer func() {
		k8sSASecretPath = savedSASecretPath
	}()
	k8sSASecretPath = "/foo"
	err = k8s.loadEnvData()
	assert.NotNil(err)
	assert.Regexp(".*no such file", err.Error())
	assert.Nil(k8s.env)

	k8sSASecretPath = "./testdata"
	assert.NotNil(testCert)
	err = k8s.loadEnvData()
	assert.Nil(err)
	assert.NotNil(k8s.env)
	expEnv := &K8sEnv{
		Host:      h,
		Port:      p,
		Token:     "Bearer fake token",
		CaCert:    testCert,
		Namespace: "nuvoloso-cluster",
		PodIP:     "1.2.3.4",
		PodName:   "pod-name",
	}
	assert.Equal(expEnv, k8s.env)
}

func TestK8sMakeClient(t *testing.T) {
	assert := assert.New(t)

	c, err := NewClient(K8sClusterType)
	assert.Nil(err)
	k8s, ok := c.(*K8s)
	assert.True(ok)
	assert.Nil(k8s.env)
	assert.Nil(k8s.httpClient)
	defer func() {
		k8s.env = nil
		k8s.transport = nil
		k8s.shp = ""
	}()

	// error case: missing env on client call
	hc, err := k8s.makeClient(0)
	assert.NotNil(err)
	assert.Nil(hc)
	assert.Regexp(K8sEnvServerHost+".*"+K8sEnvServerPort, err.Error())

	// error case: missing env on shp
	_, err = k8s.K8sGetSchemeHostPort()
	assert.NotNil(err)
	assert.Regexp(K8sEnvServerHost+".*"+K8sEnvServerPort, err.Error())

	k8s.env = &K8sEnv{
		CaCert: testCert,
	}
	hc, err = k8s.makeClient(0)
	assert.Nil(err)
	subjects := k8s.transport.TLSClientConfig.RootCAs.Subjects()
	s := ""
	for _, b := range subjects {
		s += string(b)
	}
	t.Log(s)
	assert.Regexp("Nuvoloso", s)
	assert.NotNil(hc)

}

func TestK8sClientDo(t *testing.T) {
	assert := assert.New(t)

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
	ft.data["/"] = "foo"
	ft.statusCode["/"] = 200
	c, err := NewClient(K8sClusterType)
	assert.Nil(err)
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
	k8s.env = &K8sEnv{Token: ft.authToken}
	k8s.httpClient = http.DefaultClient

	ctx := context.Background()

	// service call has Authorization and Accept headers
	req, err := http.NewRequest("GET", "/", nil)
	assert.Nil(err)
	res, err := k8s.K8sClientDo(ctx, req)
	assert.Nil(err)
	assert.True(ft.authTokenMatched)
	assert.True(ft.acceptJSONMatched)
	body, err := ioutil.ReadAll(res.Body)
	assert.Nil(err)
	assert.Equal("foo", string(body))

	// service call sets Accept explicitly to non-JSON
	req, err = http.NewRequest("GET", "/", nil)
	assert.Nil(err)
	req.Header.Set("Accept", "text")
	res, err = k8s.K8sClientDo(ctx, req)
	assert.Nil(err)
	assert.True(ft.authTokenMatched)
	assert.False(ft.acceptJSONMatched)
	body, err = ioutil.ReadAll(res.Body)
	assert.Nil(err)
	assert.Equal("foo", string(body))

	// error: bad path
	req, err = http.NewRequest("GET", "/badPath", nil)
	assert.Nil(err)
	res, err = k8s.K8sClientDo(ctx, req)
	assert.NotNil(err)
	assert.Regexp("bad path", err.Error())

	// error: bad auth
	ft.authToken = "different"
	req, err = http.NewRequest("GET", "/", nil)
	assert.Nil(err)
	res, err = k8s.K8sClientDo(ctx, req)
	assert.Nil(err)
	assert.False(ft.authTokenMatched)

	k8s.httpClient = nil
	k8s.timeout = 0
	req, err = http.NewRequest("GET", "/badPath", nil)
	assert.Nil(err)
	res, err = k8s.K8sClientDo(ctx, req)
	assert.NotNil(err)
	assert.NotNil(k8s.httpClient)
	dur, err := time.ParseDuration(fmt.Sprintf("%ds", K8sDefaultTimeoutSecs))
	assert.Nil(err)
	assert.Equal(dur, k8s.timeout)
}

func TestK8sGetJson(t *testing.T) {
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
	http.DefaultClient.Transport = ft
	ft.data["/foo"] = `{
		"fieldOneInt": 1,
		"fieldTwoString": "field 2 string",
		"field3Struct": {
			"field3aInt": 3,
			"field3bString": "field 3b string",
			"field3cSomething": "3c else"
		},
		"field4Something": "4 else"
	}`
	ft.statusCode["/foo"] = 200
	type tFooS struct {
		Field3aInt int
		AString    string `json:"field3bString"`
	}
	type tFoo struct {
		FieldOneInt    int
		FieldTwoString string
		Field3Struct   tFooS
	}
	var v tFoo
	expV := tFoo{
		FieldOneInt:    1,
		FieldTwoString: "field 2 string",
		Field3Struct: tFooS{
			Field3aInt: 3,
			AString:    "field 3b string",
		},
	}
	ctx := context.Background()

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
		Port:  6999,
	}
	k8s.httpClient = http.DefaultClient
	expHP := "host:6999"
	expScheme := "https"

	// success case
	err = k8s.K8sClientGetJSON(ctx, "/foo", &v)
	assert.Nil(err)
	assert.True(ft.authTokenMatched)
	assert.True(ft.acceptJSONMatched)
	assert.Equal(expV, v)
	assert.Equal(expHP, ft.gotHostPort)
	assert.Equal(expScheme, ft.gotScheme)
	assert.Equal(expScheme+"://"+expHP, k8s.shp)
	tl.Flush()

	// error: invalid json
	ft.statusCode["/wrongType"] = 200
	ft.data["/wrongType"] = `{
		"fieldOneInt": "not an int"
	}`
	err = k8s.K8sClientGetJSON(ctx, "/wrongType", &v)
	assert.NotNil(err)
	assert.Regexp("cannot unmarshal.*FieldOneInt", err.Error())
	tl.Flush()

	ft.statusCode["/pvfetch"] = 200
	ft.data["/pvfetch"] = pvFetchResp
	var pvs *K8sPVlistRes
	err = k8s.K8sClientGetJSON(ctx, "/pvfetch", &pvs)
	assert.Nil(err)

	// error: invalid status code
	ft.statusCode["/foo"] = 400
	err = k8s.K8sClientGetJSON(ctx, "/foo", &v)
	assert.NotNil(err)
	tl.Flush()

	// error: Kubernetes Error
	ft.data["/k8serror"] = `{
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
	ft.statusCode["/k8serror"] = 409
	err = k8s.K8sClientGetJSON(ctx, "/k8serror", &v)
	assert.NotNil(err)
	e, ok := err.(*k8sError)
	if assert.True(ok) && assert.NotNil(e) {
		assert.Equal("Some Error Message", e.Message)
		assert.Equal("error", string(e.Reason))
		assert.Equal(409, e.Code)
	}
	tl.Flush()

	// error: Kubernetes Error unable to unmarshal
	ft.data["/k8serror"] = `{"unable to marshal"}`
	ft.statusCode["/k8serror"] = 409
	err = k8s.K8sClientGetJSON(ctx, "/k8serror", &v)
	assert.NotNil(err)
	e, ok = err.(*k8sError)
	if assert.True(ok) && assert.NotNil(e) {
		assert.Regexp("unable to marshal", e.Message)
		assert.Equal(StatusReasonUnknown, e.Reason)
		assert.Equal(409, e.Code)
	}
	tl.Flush()

	// error: client failure
	k8s.httpClient = nil
	k8s.env = nil
	err = k8s.K8sClientGetJSON(ctx, "/foo", &v)
	tl.Logger().Info(err)
	assert.NotNil(err)
	assert.Regexp(K8sEnvServerHost+".*"+K8sEnvServerPort, err.Error())
}

func TestK8sPostJson(t *testing.T) {
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
	http.DefaultClient.Transport = ft
	ft.data["/foo"] = `{
		"fieldOneInt": 1,
		"fieldTwoString": "field 2 string",
		"field3Struct": {
			"field3aInt": 3,
			"field3bString": "field 3b string",
			"field3cSomething": "3c else"
		},
		"field4Something": "4 else"
	}`
	ft.statusCode["/foo"] = 200
	type tFooS struct {
		Field3aInt int
		AString    string `json:"field3bString"`
	}
	type tFoo struct {
		FieldOneInt    int
		FieldTwoString string
		Field3Struct   tFooS
	}
	var v tFoo
	expV := tFoo{
		FieldOneInt:    1,
		FieldTwoString: "field 2 string",
		Field3Struct: tFooS{
			Field3aInt: 3,
			AString:    "field 3b string",
		},
	}
	ctx := context.Background()

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
		Port:  6999,
	}
	k8s.httpClient = http.DefaultClient
	expHP := "host:6999"
	expScheme := "https"

	// success case
	err = k8s.K8sClientPostJSON(ctx, "/foo", []byte{}, &v)
	assert.Nil(err)
	assert.True(ft.authTokenMatched)
	assert.True(ft.acceptJSONMatched)
	assert.Equal(expV, v)
	assert.Equal(expHP, ft.gotHostPort)
	assert.Equal(expScheme, ft.gotScheme)
	assert.Equal(expScheme+"://"+expHP, k8s.shp)
	tl.Flush()

	// error: invalid json
	ft.statusCode["/wrongType"] = 200
	ft.data["/wrongType"] = `{
		"fieldOneInt": "not an int"
	}`
	err = k8s.K8sClientPostJSON(ctx, "/wrongType", []byte{}, &v)
	assert.NotNil(err)
	assert.Regexp("cannot unmarshal.*FieldOneInt", err.Error())
	tl.Flush()

	// error: invalid status code
	ft.statusCode["/foo"] = 400
	err = k8s.K8sClientPostJSON(ctx, "/foo", []byte{}, &v)
	assert.NotNil(err)
	ft.statusCode["/foo"] = 200
	tl.Flush()

	// error: Kubernetes Error
	ft.data["/k8serror"] = `{
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
	ft.statusCode["/k8serror"] = 409
	err = k8s.K8sClientPostJSON(ctx, "/k8serror", []byte{}, &v)
	assert.NotNil(err)
	e, ok := err.(*k8sError)
	if assert.True(ok) && assert.NotNil(e) {
		assert.Equal("Some Error Message", e.Message)
		assert.Equal("error", string(e.Reason))
		assert.Equal(409, e.Code)
	}
	ft.statusCode["/k8serror"] = 200
	tl.Flush()

	// error: Kubernetes Error unable to unmarshal
	ft.data["/k8serror"] = `{"unable to marshal"}`
	ft.statusCode["/k8serror"] = 409
	err = k8s.K8sClientPostJSON(ctx, "/k8serror", []byte{}, &v)
	assert.NotNil(err)
	e, ok = err.(*k8sError)
	if assert.True(ok) && assert.NotNil(e) {
		assert.Regexp("unable to marshal", e.Message)
		assert.Equal(StatusReasonUnknown, e.Reason)
		assert.Equal(409, e.Code)
	}
	ft.statusCode["/k8serror"] = 200
	tl.Flush()

	// error: client failure
	k8s.httpClient = nil
	k8s.env = nil
	err = k8s.K8sClientPostJSON(ctx, "/foo", []byte{}, &v)
	tl.Logger().Info(err)
	assert.NotNil(err)
	assert.Regexp(K8sEnvServerHost+".*"+K8sEnvServerPort, err.Error())
}

func TestK8sPutJson(t *testing.T) {
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
	http.DefaultClient.Transport = ft
	ft.data["/foo"] = `{
		"fieldOneInt": 1,
		"fieldTwoString": "field 2 string",
		"field3Struct": {
			"field3aInt": 3,
			"field3bString": "field 3b string",
			"field3cSomething": "3c else"
		},
		"field4Something": "4 else"
	}`
	ft.statusCode["/foo"] = 200
	type tFooS struct {
		Field3aInt int
		AString    string `json:"field3bString"`
	}
	type tFoo struct {
		FieldOneInt    int
		FieldTwoString string
		Field3Struct   tFooS
	}
	var v tFoo
	expV := tFoo{
		FieldOneInt:    1,
		FieldTwoString: "field 2 string",
		Field3Struct: tFooS{
			Field3aInt: 3,
			AString:    "field 3b string",
		},
	}
	ctx := context.Background()

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
		Port:  6999,
	}
	k8s.httpClient = http.DefaultClient
	expHP := "host:6999"
	expScheme := "https"

	// success case
	err = k8s.K8sClientPutJSON(ctx, "/foo", []byte{}, &v)
	assert.Nil(err)
	assert.True(ft.authTokenMatched)
	assert.True(ft.acceptJSONMatched)
	assert.Equal(expV, v)
	assert.Equal(expHP, ft.gotHostPort)
	assert.Equal(expScheme, ft.gotScheme)
	assert.Equal(expScheme+"://"+expHP, k8s.shp)
	tl.Flush()

	// error: invalid json
	ft.statusCode["/wrongType"] = 200
	ft.data["/wrongType"] = `{
		"fieldOneInt": "not an int"
	}`
	err = k8s.K8sClientPutJSON(ctx, "/wrongType", []byte{}, &v)
	assert.NotNil(err)
	assert.Regexp("cannot unmarshal.*FieldOneInt", err.Error())
	tl.Flush()

	// error: invalid status code
	ft.statusCode["/foo"] = 400
	err = k8s.K8sClientPutJSON(ctx, "/foo", []byte{}, &v)
	assert.NotNil(err)
	ft.statusCode["/foo"] = 200
	tl.Flush()

	// error: Kubernetes Error
	ft.data["/k8serror"] = `{
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
	ft.statusCode["/k8serror"] = 409
	err = k8s.K8sClientPutJSON(ctx, "/k8serror", []byte{}, &v)
	assert.NotNil(err)
	e, ok := err.(*k8sError)
	if assert.True(ok) && assert.NotNil(e) {
		assert.Equal("Some Error Message", e.Message)
		assert.Equal("error", string(e.Reason))
		assert.Equal(409, e.Code)
	}
	ft.statusCode["/k8serror"] = 200
	tl.Flush()

	// error: Kubernetes Error unable to unmarshal
	ft.data["/k8serror"] = `{"unable to marshal"}`
	ft.statusCode["/k8serror"] = 409
	err = k8s.K8sClientPutJSON(ctx, "/k8serror", []byte{}, &v)
	assert.NotNil(err)
	e, ok = err.(*k8sError)
	if assert.True(ok) && assert.NotNil(e) {
		assert.Regexp("unable to marshal", e.Message)
		assert.Equal(StatusReasonUnknown, e.Reason)
		assert.Equal(409, e.Code)
	}
	ft.statusCode["/k8serror"] = 200
	tl.Flush()

	// error: client failure
	k8s.httpClient = nil
	k8s.env = nil
	err = k8s.K8sClientPutJSON(ctx, "/foo", []byte{}, &v)
	tl.Logger().Info(err)
	assert.NotNil(err)
	assert.Regexp(K8sEnvServerHost+".*"+K8sEnvServerPort, err.Error())
}

func TestK8sDeleteJson(t *testing.T) {
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
	http.DefaultClient.Transport = ft
	ft.data["/foo"] = `{
		"fieldOneInt": 1,
		"fieldTwoString": "field 2 string",
		"field3Struct": {
			"field3aInt": 3,
			"field3bString": "field 3b string",
			"field3cSomething": "3c else"
		},
		"field4Something": "4 else"
	}`
	ft.statusCode["/foo"] = 200
	type tFooS struct {
		Field3aInt int
		AString    string `json:"field3bString"`
	}
	type tFoo struct {
		FieldOneInt    int
		FieldTwoString string
		Field3Struct   tFooS
	}
	var v tFoo
	expV := tFoo{
		FieldOneInt:    1,
		FieldTwoString: "field 2 string",
		Field3Struct: tFooS{
			Field3aInt: 3,
			AString:    "field 3b string",
		},
	}
	ctx := context.Background()

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
		Port:  6999,
	}
	k8s.httpClient = http.DefaultClient
	expHP := "host:6999"
	expScheme := "https"

	// success case
	err = k8s.K8sClientDeleteJSON(ctx, "/foo", &v)
	assert.Nil(err)
	assert.True(ft.authTokenMatched)
	assert.True(ft.acceptJSONMatched)
	assert.Equal(expV, v)
	assert.Equal(expHP, ft.gotHostPort)
	assert.Equal(expScheme, ft.gotScheme)
	assert.Equal(expScheme+"://"+expHP, k8s.shp)
	tl.Flush()

	// error: invalid json
	ft.statusCode["/wrongType"] = 200
	ft.data["/wrongType"] = `{
		"fieldOneInt": "not an int"
	}`
	err = k8s.K8sClientDeleteJSON(ctx, "/wrongType", &v)
	assert.NotNil(err)
	assert.Regexp("cannot unmarshal.*FieldOneInt", err.Error())
	tl.Flush()

	// error: invalid status code
	ft.statusCode["/foo"] = 400
	err = k8s.K8sClientDeleteJSON(ctx, "/foo", &v)
	assert.NotNil(err)
	ft.statusCode["/foo"] = 200
	tl.Flush()

	// error: Kubernetes Error
	ft.data["/k8serror"] = `{
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
	ft.statusCode["/k8serror"] = 409
	err = k8s.K8sClientDeleteJSON(ctx, "/k8serror", &v)
	assert.NotNil(err)
	e, ok := err.(*k8sError)
	if assert.True(ok) && assert.NotNil(e) {
		assert.Equal("Some Error Message", e.Message)
		assert.Equal("error", string(e.Reason))
		assert.Equal(409, e.Code)
	}
	ft.statusCode["/k8serror"] = 200
	tl.Flush()

	// error: Kubernetes Error unable to unmarshal
	ft.data["/k8serror"] = `{"unable to marshal"}`
	ft.statusCode["/k8serror"] = 409
	err = k8s.K8sClientDeleteJSON(ctx, "/k8serror", &v)
	assert.NotNil(err)
	e, ok = err.(*k8sError)
	if assert.True(ok) && assert.NotNil(e) {
		assert.Regexp("unable to marshal", e.Message)
		assert.Equal(StatusReasonUnknown, e.Reason)
		assert.Equal(409, e.Code)
	}
	ft.statusCode["/k8serror"] = 200
	tl.Flush()

	// error: client failure
	k8s.httpClient = nil
	k8s.env = nil
	err = k8s.K8sClientDeleteJSON(ctx, "/foo", &v)
	tl.Logger().Info(err)
	assert.NotNil(err)
	assert.Regexp(K8sEnvServerHost+".*"+K8sEnvServerPort, err.Error())
}

func TestK8sMetaData(t *testing.T) {
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
		data:       k8sFakeCluster,
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

	md, err := c.MetaData(ctx)
	assert.Nil(err)
	assert.NotNil(md)
	assert.Contains(md, CMDVersion)
	assert.Equal(md[CMDVersion], "1.7")
	assert.Contains(md, CMDIdentifier)
	assert.Equal("8.7.6.5", md[CMDServiceIP])
	assert.Equal("k8s-svc-uid:"+"c981db76-bda0-11e7-b2ce-02a3152c8208", md[CMDIdentifier])
	assert.Equal("v1.7.8", md["GitVersion"])
	assert.Equal("linux/amd64", md["Platform"])
	assert.Equal("2017-10-30T18:33:13Z", md["CreationTimestamp"])

	// error
	k8s.httpClient = nil
	k8s.env.Host = ""
	k8s.shp = ""
	md, err = c.MetaData(ctx)
	tl.Logger().Info(err)
	assert.NotNil(err)
	assert.Nil(md)
}

func TestPVSpec(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	savedpvTProc := pvTProc
	savedpvSpecTemplateDir := pvSpecTemplateDir
	defer func() {
		pvTProc = savedpvTProc
		pvSpecTemplateDir = savedpvSpecTemplateDir
	}()
	ctx := context.Background()

	// point to the local test data
	pvSpecTemplateDir = "./testdata"
	pvTProc = nil

	c, err := NewClient(K8sClusterType)
	assert.Nil(err)
	assert.NotNil(c)
	k8s, ok := c.(*K8s)
	assert.True(ok)
	assert.NotNil(k8s)

	c.SetDebugLogger(tl.Logger())

	d, err := c.GetPVSpec(ctx, nil)
	assert.NotNil(err)
	assert.Nil(d)
	assert.Regexp("invalid arguments", err.Error())

	d, err = c.GetPVSpec(ctx, &PVSpecArgs{})
	assert.NotNil(err)
	assert.Nil(d)
	assert.Regexp("invalid arguments", err.Error())

	tA := &PVSpecArgs{
		AccountID: "account1",
		VolumeID:  "volume1",
		SystemID:  "system1",
		Capacity:  "10Gi",
	}
	k8s.pvSpecFillDefaultArgs(ctx, tA)
	assert.Equal(PVSpecDefaultVersion, tA.ClusterVersion)
	assert.Equal(PVSpecDefaultFsType, tA.FsType)
	assert.Equal("account1", tA.AccountID)
	assert.Equal("volume1", tA.VolumeID)
	assert.Equal("system1", tA.SystemID)
	assert.Equal("10Gi", tA.Capacity)

	tA = &PVSpecArgs{
		AccountID:      "account1",
		VolumeID:       "volume1",
		SystemID:       "system1",
		Capacity:       "10Gi",
		ClusterVersion: "v1",
		FsType:         "ext4",
	}
	k8s.pvSpecFillDefaultArgs(ctx, tA)
	assert.Equal("v1", tA.ClusterVersion)
	assert.Equal("ext4", tA.FsType)
	assert.Equal("account1", tA.AccountID)
	assert.Equal("volume1", tA.VolumeID)
	assert.Equal("system1", tA.SystemID)
	assert.Equal("10Gi", tA.Capacity)

	d, err = c.GetPVSpec(ctx, nil)
	assert.NotNil(err)
	assert.Nil(d)
	assert.Regexp("invalid arguments", err.Error())

	pvSpecTemplateDir = "./invaliddir"
	pvTProc = nil
	tA = &PVSpecArgs{
		AccountID: "account1",
		VolumeID:  "volume1",
		SystemID:  "system1",
		Capacity:  "10Gi",
	}
	k8s.pvSpecFillDefaultArgs(ctx, tA)
	d, err = c.GetPVSpec(ctx, tA)
	assert.NotNil(err)
	assert.Nil(d)
	assert.Regexp("failed to create k8s pv template processor", err.Error())
}

func TestError(t *testing.T) {
	assert := assert.New(t)

	var err error

	err = NewK8sError("new-error", "reason")
	assert.Equal("new-error", err.Error())
	ed, ok := err.(*k8sError)
	assert.True(ok)

	err = &k8sError{Message: "error-message", Reason: StatusReasonUnknown, Code: 409}
	assert.Equal("error-message", err.Error())
	ed, ok = err.(*k8sError)
	assert.True(ok)
	assert.False(ed.ObjectExists())
	assert.False(ed.PVIsBound())

	err = &k8sError{Message: "error-message", Reason: StatusReasonAlreadyExists, Code: 409}
	assert.Equal("error-message", err.Error())
	ed, ok = err.(*k8sError)
	assert.True(ok)
	assert.True(ed.ObjectExists())
	assert.False(ed.PVIsBound())

	err = ErrPvIsBoundOnDelete
	assert.NotNil(err.Error())
	ed, ok = err.(*k8sError)
	assert.False(ed.ObjectExists())
	assert.True(ed.PVIsBound())

	err = &k8sError{Message: "error-message", Reason: StatusReasonNotFound, Code: 409}
	assert.NotNil(err.Error())
	ed, ok = err.(*k8sError)
	assert.True(ed.NotFound())

}

func TestPVNameSwap(t *testing.T) {
	assert := assert.New(t)

	pvName := VolID2K8sPvName("volID")
	assert.Equal(K8sPvNamePrefix+"volID", pvName)

	volID := K8sPvName2VolID(pvName)
	assert.Equal("volID", volID)

	volID = K8sPvName2VolID("bad pv name")
	assert.Equal("", volID)
}

func TestK8sMakeValidName(t *testing.T) {
	assert := assert.New(t)

	tcs := map[string]string{
		"Backup":                 "backup",
		"DB Log":                 "db-log",
		"DSS Data":               "dss-data",
		"General - Premier":      "general-premier",
		"General":                "general",
		"OLTP Data - Premier":    "oltp-data-premier",
		"OLTP Data":              "oltp-data",
		"Online Archive":         "online-archive",
		"Streaming Analytics":    "streaming-analytics",
		"Technical Applications": "technical-applications",
		"other     test":         "other-test",
		"shouldn't-change":       "shouldn't-change", // but is an invalid name
	}

	for i, v := range tcs {
		res := K8sMakeValidName(i)
		assert.Equal(v, res)
	}
}

func TestK8sGetHostname(t *testing.T) {
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
		data:       k8sFakeCluster,
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

	service, err := c.GetService(ctx, "nuvo-https", "nuvoloso-management")
	assert.Nil(err)
	assert.Equal("a6919e4037cdd11e994f90208099e0bf-1863363181.us-west-2.elb.amazonaws.com", service.Hostname)

	service, err = c.GetService(ctx, "nuvo-https-both", "nuvoloso-management")
	assert.Nil(err)
	assert.Equal("a6919e4037cdd11e994f90208099e0bf-1863363181.us-west-2.elb.amazonaws.com", service.Hostname)

	service, err = c.GetService(ctx, "nuvo-https-ip", "nuvoloso-management")
	assert.Nil(err)
	assert.Equal("1.2.3.4", service.Hostname)

	service, err = c.GetService(ctx, "badServicetype", "nuvoloso-management")
	assert.Nil(err)
	assert.Equal("", service.Hostname)

	service, err = c.GetService(ctx, "badServiceHostname", "nuvoloso-management")
	assert.Nil(err)
	assert.Equal("", service.Hostname)

	// error
	k8s.httpClient = nil
	k8s.env.Host = ""
	k8s.shp = ""
	service, err = c.GetService(ctx, "nuvo-https", "nuvoloso-management")
	tl.Logger().Info(err)
	assert.NotNil(err)
	assert.Nil(service)
}

func TestK8sGetClusterIdentity(t *testing.T) {
	jsonDataStr := `{
		"kind": "Service",
		"apiVersion": "v1",
		"metadata": {
		  "name": "kubernetes",
		  "namespace": "default",
		  "selfLink": "/api/v1/namespaces/default/services/kubernetes",
		  "uid": "c981db76-bda0-11e7-b2ce-02a3152c8208",
		  "resourceVersion": "8",
		  "creationTimestamp": "2017-10-30T18:33:13Z",
		  "labels": {
			"component": "apiserver",
			"provider": "kubernetes"
		  }
		},
		"spec": {
		  "ports": [
			{
			  "name": "https",
			  "protocol": "TCP",
			  "port": 443,
			  "targetPort": 443
			}
		  ],
		  "clusterIP": "100.64.0.1",
		  "type": "ClusterIP",
		  "sessionAffinity": "ClientIP"
		},
		"status": {
		  "loadBalancer": {}
		}
	  }`
	yamlDataStr := `
apiVersion: v1
kind: Service
metadata:
  creationTimestamp: "2019-05-21T23:21:19Z"
  name: kubernetes
  namespace: default
  resourceVersion: "33"
  selfLink: /api/v1/namespaces/default/services/kubernetes
  uid: c981db76-bda0-11e7-b2ce-02a3152c8208
  labels:
    component: apiserver
    provider: kubernetes
spec:
  clusterIP: 100.64.0.1
  ports:
  - name: https
    port: 443
    protocol: TCP
    targetPort: 443
  sessionAffinity: None
  type: ClusterIP
status:
  loadBalancer: {}
`
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	c, err := NewClient(K8sClusterType)
	assert.Nil(err)
	ctx := context.Background()

	// json, success
	uid, err := c.ValidateClusterObjID(ctx, jsonDataStr, "json")
	assert.Nil(err)
	assert.Equal(K8sClusterIdentifierPrefix+"c981db76-bda0-11e7-b2ce-02a3152c8208", uid)

	// missing uid
	jsonDataStr = `{
		"kind": "Service",
		"apiVersion": "v1",
		"metadata": {
		  "name": "kubernetes",
		  "uid": ""
		}
	  }`
	uid, err = c.ValidateClusterObjID(ctx, jsonDataStr, "json")
	assert.NotNil(err)
	assert.Regexp("uid not found", err.Error())

	// json parsing error
	jsonDataStr = `{
		"kind": "Service",
		"apiVersion": "v1",
		"metadata": {
		  "name": 123
		}
	}`
	uid, err = c.ValidateClusterObjID(ctx, jsonDataStr, "json")
	assert.NotNil(err)
	assert.Regexp("cannot unmarshal number into Go struct field", err.Error())

	// yaml, success
	uid, err = c.ValidateClusterObjID(ctx, yamlDataStr, "YAml")
	assert.Nil(err)
	assert.Equal(K8sClusterIdentifierPrefix+"c981db76-bda0-11e7-b2ce-02a3152c8208", uid)

	// yaml parsing error
	yamlDataStr = `
apiVersion: v1
kind: Service
metadata:
  creationTimestamp: "some_ivalid_date"
  name: kubernetes
`
	uid, err = c.ValidateClusterObjID(ctx, yamlDataStr, "YAml")
	assert.NotNil(err)
	assert.Regexp("cannot parse", err.Error())

	// unsupported format type
	uid, err = c.ValidateClusterObjID(ctx, jsonDataStr, "bad_format")
	assert.NotNil(err)
	assert.Regexp("unsupported format type", err.Error())

	// wrong service kind
	yamlDataStr = `
apiVersion: v1
kind: BAD_SERVICE
metadata:
  creationTimestamp: "2019-05-21T23:21:19Z"
  name: kubernetes
  namespace: default
  resourceVersion: "33"
  selfLink: /api/v1/namespaces/default/services/kubernetes
  uid: c981db76-bda0-11e7-b2ce-02a3152c8208
  labels:
    component: apiserver
    provider: kubernetes
spec:
  clusterIP: 100.64.0.1
  ports:
  - name: https
    port: 443
    protocol: TCP
    targetPort: 443
  sessionAffinity: None
  type: ClusterIP
status:
  loadBalancer: {}
`
	uid, err = c.ValidateClusterObjID(ctx, yamlDataStr, "YAml")
	assert.NotNil(err)
	assert.Regexp("invalid identity object", err.Error())

	// wrong service kind
	yamlDataStr = `
apiVersion: v1
kind: SERVICE
metadata:
  creationTimestamp: "2019-05-21T23:21:19Z"
  name: kubernetes
  namespace: default
  resourceVersion: "33"
  selfLink: /api/v1/namespaces/BAD_NAMESPACE/services/kubernetes
  uid: c981db76-bda0-11e7-b2ce-02a3152c8208
  labels:
    component: apiserver
    provider: kubernetes
spec:
  clusterIP: 100.64.0.1
  ports:
  - name: https
    port: 443
    protocol: TCP
    targetPort: 443
  sessionAffinity: None
  type: ClusterIP
status:
  loadBalancer: {}
`
	uid, err = c.ValidateClusterObjID(ctx, yamlDataStr, "YAml")
	assert.NotNil(err)
	assert.Regexp("invalid identity object", err.Error())
}
