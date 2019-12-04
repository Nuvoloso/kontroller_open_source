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

	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

var secretFetchFakeRes = map[string]string{
	"/api/v1/namespaces/default/secrets/mysecret": `{
		"kind": "Secret",
		"apiVersion": "v1",
		"metadata": {
		  "name": "mysecret",
		  "namespace": "default",
		  "selfLink": "/api/v1/namespaces/default/secrets/mysecret",
		  "uid": "58f39715-e86c-11e8-b102-02308b559f88",
		  "resourceVersion": "26772",
		  "creationTimestamp": "2018-11-15T00:21:08Z"
		},
		"data": {
		  "pass": "c2VjcmV0cGFzcw==",
		  "username": "c2lyaXNo",
		  "nuvoloso-secret": "c2VjcmV0cGFzcw=="
		},
		"type": "Opaque"
	  }`,
}
var secretFetchFakeResMdJSON = map[string]string{ // map data
	"/api/v1/namespaces/default/secrets/mysecret": `{
		"kind": "Secret",
		"apiVersion": "v1",
		"metadata": {
		  "name": "mysecret",
		  "namespace": "default",
		  "selfLink": "/api/v1/namespaces/default/secrets/mysecret",
		  "uid": "58f39715-e86c-11e8-b102-02308b559f88",
		  "resourceVersion": "26772",
		  "creationTimestamp": "2018-11-15T00:21:08Z"
		},
		"data": {
			"nuvoloso-secret": "eyJ0ZXN0IjoiZGF0YSIsInRlc3QyIjoiZGF0YTIifQ=="
		},
		"type": "Opaque"
	  }`,
}
var secretFetchFakeResCdJSON = map[string]string{ // customization data
	"/api/v1/namespaces/default/secrets/mysecret": `{
		"kind": "Secret",
		"apiVersion": "v1",
		"metadata": {
		  "name": "mysecret",
		  "namespace": "default",
		  "selfLink": "/api/v1/namespaces/default/secrets/mysecret",
		  "uid": "58f39715-e86c-11e8-b102-02308b559f88",
		  "resourceVersion": "26772",
		  "creationTimestamp": "2018-11-15T00:21:08Z"
		},
		"data": {
			"nuvoloso-secret": "eyJhY2NvdW50U2VjcmV0IjoiYWNjb3VudC1zZWNyZXQiLCJjb25zaXN0ZW5jeUdyb3VwTmFtZSI6ImNnLW5hbWUiLCJjb25zaXN0ZW5jeUdyb3VwRGVzY3JpcHRpb24iOiJjZy1kZXNjIiwiY29uc2lzdGVuY3lHcm91cFRhZ3MiOlsiY2ctdDEiLCJjZy10MiJdLCJ2b2x1bWVUYWdzIjpbInZvbC10MSIsInZvbC10MiIsInZvbC10MyJdfQ=="
		},
		"type": "Opaque"
	  }`,
}
var secretFetchFakeResJSONInvalid = map[string]string{
	"/api/v1/namespaces/default/secrets/mysecret": `{
		"kind": "Secret",
		"apiVersion": "v1",
		"metadata": {
		  "name": "mysecret",
		  "namespace": "default",
		  "selfLink": "/api/v1/namespaces/default/secrets/mysecret",
		  "uid": "58f39715-e86c-11e8-b102-02308b559f88",
		  "resourceVersion": "26772",
		  "creationTimestamp": "2018-11-15T00:21:08Z"
		},
		"data": {
			"nuvoloso-secret": "sdfasd=="
		},
		"type": "Opaque"
	  }`,
}
var secretCreateFakeRes = map[string]string{
	"/api/v1/namespaces/fakeNamespace/secrets": `{
		"kind": "Secret",
		"apiVersion": "v1",
		"metadata": {
		  "name": "mysecret",
		  "namespace": "default",
		  "selfLink": "/api/v1/namespaces/default/secrets/mysecret",
		  "uid": "58f39715-e86c-11e8-b102-02308b559f88",
		  "resourceVersion": "26772",
		  "creationTimestamp": "2018-11-15T00:21:08Z"
		},
		"data": {
		  "pass": "c2VjcmV0cGFzcw==",
		  "username": "c2lyaXNo"
		},
		"type": "Opaque"
	  }`,
}
var secretCreateFakeResJSON = map[string]string{
	"/api/v1/namespaces/fakeNamespace/secrets": `{
		"kind": "Secret",
		"apiVersion": "v1",
		"metadata": {
		  "name": "mysecret",
		  "namespace": "default",
		  "selfLink": "/api/v1/namespaces/default/secrets/mysecret",
		  "uid": "58f39715-e86c-11e8-b102-02308b559f88",
		  "resourceVersion": "26772",
		  "creationTimestamp": "2018-11-15T00:21:08Z"
		},
		"data": {
		  "nuvoloso-secret": "eyJ0ZXN0IjoiZGF0YSIsInRlc3QyIjoiZGF0YTIifQ=="
		},
		"type": "Opaque"
	  }`,
}

var secretCreateFakeResJSON2 = map[string]string{
	"/api/v1/namespaces/fakeNamespace/secrets": `{
		"kind": "Secret",
		"apiVersion": "v1",
		"metadata": {
		  "name": "mysecret",
		  "namespace": "default",
		  "selfLink": "/api/v1/namespaces/default/secrets/mysecret",
		  "uid": "58f39715-e86c-11e8-b102-02308b559f88",
		  "resourceVersion": "26772",
		  "creationTimestamp": "2018-11-15T00:21:08Z"
		},
		"data": {
		  "nuvoloso-secret": "sdfasd=="
		},
		"type": "Opaque"
	  }`,
}

func TestSecretFetch(t *testing.T) {
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
		data:       secretFetchFakeRes,
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
	sfa := &SecretFetchArgs{
		Name:      "mysecret",
		Namespace: "default",
	}
	secretRes, err := c.SecretFetch(ctx, sfa)
	assert.Nil(err)
	assert.NotNil(secretRes)

	// Missing required fields
	sfa = &SecretFetchArgs{
		Namespace: "default",
	}
	secretRes, err = c.SecretFetch(ctx, sfa)
	assert.NotNil(err)
	assert.Regexp("Required field\\(s\\) Name, Namespace are missing", err.Error())
	assert.Nil(secretRes)

	// Incorrect required fields
	sfa = &SecretFetchArgs{
		Name:      "badsecretname",
		Namespace: "default",
	}
	secretRes, err = c.SecretFetch(ctx, sfa)
	assert.NotNil(err)
	assert.Nil(secretRes)

	// Note: decoding MV JSON with SV decoder succeeds!
	mvJSONCases := []map[string]string{
		secretFetchFakeResMdJSON,
		secretFetchFakeResCdJSON,
	}
	for i, tc := range mvJSONCases {
		ft.data = tc
		sfa = &SecretFetchArgs{
			Name:      "mysecret",
			Namespace: "default",
		}
		secretRes, err = c.SecretFetch(ctx, sfa)
		assert.NoError(err, "mvJSON %d", i)
		assert.NotNil(secretRes, "mvJSON %d", i)
		assert.Equal("{", secretRes.Data[0:1], "mvJSON %d", i) // surprise: its the JSON string!
	}

	// fetchJSON (SecretIntentGeneral: map data)
	ft.data = secretFetchFakeResMdJSON
	sfaJ := &SecretFetchArgs{
		Name:      "mysecret",
		Namespace: "default",
	}
	secJ, err := c.SecretFetchMV(ctx, sfaJ)
	assert.Nil(err)
	assert.NotNil(secJ)
	assert.Equal("mysecret", secJ.Name)
	assert.Equal("default", secJ.Namespace)
	assert.Equal(SecretIntentGeneral, secJ.Intent)
	assert.True(len(secJ.Data) > 0)
	assert.Empty(secJ.CustomizationData.AccountSecret)

	// fetchJSON (SecretIntentDynamicVolumeCustomization: customization data)
	ft.data = secretFetchFakeResCdJSON
	sfaJ = &SecretFetchArgs{
		Name:      "mysecret",
		Namespace: "default",
	}
	secJ, err = c.SecretFetchMV(ctx, sfaJ)
	tl.Flush()
	assert.Nil(err)
	assert.NotNil(secJ)
	assert.Equal("mysecret", secJ.Name)
	assert.Equal("default", secJ.Namespace)
	assert.Equal(SecretIntentDynamicVolumeCustomization, secJ.Intent)
	assert.Empty(secJ.Data)
	assert.NotEmpty(secJ.CustomizationData.AccountSecret)

	// more multi-value encode/decode variants
	mvCases := []SecretCreateArgsMV{
		{
			Name:      "general",
			Namespace: "ns-g",
			Data:      map[string]string{"foo": "bar"},
		},
		{
			Name:      "all-customization-fields",
			Namespace: "ns-acf",
			Intent:    SecretIntentDynamicVolumeCustomization,
			CustomizationData: AccountVolumeData{
				AccountSecret:               "accountSecret",
				ApplicationGroupName:        "cg-name",
				ApplicationGroupDescription: "cg-d",
				ApplicationGroupTags:        []string{"ag-t1", "ag-t2"},
				ConsistencyGroupName:        "cg-name",
				ConsistencyGroupDescription: "cg-d",
				ConsistencyGroupTags:        []string{"cg-t1", "cg-t2"},
				VolumeTags:                  []string{"v-t1"},
			},
		},
		{
			Name:      "all-cg-name",
			Namespace: "ns-cgn",
			Intent:    SecretIntentDynamicVolumeCustomization,
			CustomizationData: AccountVolumeData{
				AccountSecret:        "accountSecret",
				ConsistencyGroupName: "cg-name",
			},
		},
		{
			Name:      "all-vol-tags",
			Namespace: "ns-vt",
			Intent:    SecretIntentDynamicVolumeCustomization,
			CustomizationData: AccountVolumeData{
				AccountSecret: "accountSecret",
				VolumeTags:    []string{"v-t1"},
			},
		},
		{
			Name:      "account-only",
			Namespace: "ns-vt",
			Intent:    SecretIntentAccountIdentity,
			CustomizationData: AccountVolumeData{
				AccountSecret: "accountSecret",
			},
		},
	}
	for i, tc := range mvCases {
		sca := k8s.secretEncodeMV(&tc)
		sObj := &SecretObj{SecretCreateArgs: SecretCreateArgs{Name: sca.Name, Namespace: sca.Namespace, Data: sca.Data}}
		secJ, err := k8s.secretDecodeMV(sObj)
		assert.NoError(err, "mvCase %d", i)
		assert.NotNil(secJ, "mvCase %d", i)
		assert.Equal(tc.Name, secJ.Name, "mvCase %d", i)
		assert.Equal(tc.Namespace, secJ.Namespace, "mvCase %d", i)
		assert.Equal(tc.Intent, secJ.Intent, "mvCase %d", i)
		assert.Equal(tc.Data, secJ.Data, "mvCase %d", i)
		assert.Equal(tc.CustomizationData, secJ.CustomizationData, "mvCase %d", i)
	}

	// MV decoder cannot decode SV secret
	ft.data = secretFetchFakeRes
	sfaJ = &SecretFetchArgs{
		Name:      "mysecret",
		Namespace: "default",
	}
	secJ, err = c.SecretFetchMV(ctx, sfaJ)
	assert.NotNil(err)
	assert.Nil(secJ)

	// invalid json unable to unmarshal
	ft.data = secretFetchFakeResJSONInvalid
	sfaJ = &SecretFetchArgs{
		Name:      "mysecret",
		Namespace: "default",
	}
	secJ, err = c.SecretFetchMV(ctx, sfaJ)
	assert.NotNil(err)
	assert.Nil(secJ)

	// Request error
	k8s.httpClient = nil
	k8s.env.Host = ""
	k8s.shp = ""
	secretRes, err = c.SecretFetch(ctx, sfa)
	assert.NotNil(err)
	assert.Nil(secretRes)

	// Request error
	k8s.httpClient = nil
	k8s.env.Host = ""
	k8s.shp = ""
	secJ, err = c.SecretFetchMV(ctx, sfa)
	assert.NotNil(err)
	assert.Nil(secJ)

}

func TestSecretCreate(t *testing.T) {
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
		data:       secretCreateFakeRes,
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

	sca := &SecretCreateArgs{
		Name:      "fakeSecret",
		Namespace: "fakeNamespace",
		Data:      "fakeSecretData",
	}
	secret, err := c.SecretCreate(ctx, sca)
	assert.Nil(err)
	assert.NotNil(secret)

	ft.data = secretCreateFakeResJSON
	scaj := &SecretCreateArgsMV{
		Name:      "fakeSecret",
		Namespace: "fakeNamespace",
		Data: map[string]string{ //   eyJ0ZXN0IjoiZGF0YSIsInRlc3QyIjoiZGF0YTIifQ==
			"test":  "data",
			"test2": "data2",
		},
	}
	secretJSON, err := c.SecretCreateMV(ctx, scaj)
	assert.Nil(err)
	assert.NotNil(secretJSON)

	ft.data = secretCreateFakeResJSON2
	scaj = &SecretCreateArgsMV{
		Name:      "fakeSecret",
		Namespace: "fakeNamespace",
		Data: map[string]string{ //   eyJ0ZXN0IjoiZGF0YSIsInRlc3QyIjoiZGF0YTIifQ==
			"test":  "data",
			"test2": "data2",
		},
	}
	secretJSON, err = c.SecretCreateMV(ctx, scaj)
	assert.NotNil(err)
	assert.Nil(secretJSON)

	// Request error
	k8s.httpClient = nil
	k8s.env.Host = ""
	k8s.shp = ""
	secret, err = c.SecretCreate(ctx, sca)
	assert.NotNil(err)
	assert.Nil(secret)

	// Request error
	k8s.httpClient = nil
	k8s.env.Host = ""
	k8s.shp = ""
	secretJSON, err = c.SecretCreateMV(ctx, scaj)
	assert.NotNil(err)
	assert.Nil(secretJSON)
}

func TestSecretCreateArgstoPostBody(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	sca := &SecretCreateArgs{
		Name:      "test",
		Namespace: "nstest",
		Data:      "fakeSecretData",
	}

	var postBody secretPostBody
	postBody.Init(sca)
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
	data, exists := keyMap["stringData"]
	assert.True(exists)
	assert.Contains(data.(map[string]interface{})[com.K8sSecretKey], sca.Data)
}

func TestSecretFormat(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()

	c, err := NewClient(K8sClusterType)
	assert.Nil(err)
	c.SetDebugLogger(tl.Logger())

	sca := &SecretCreateArgs{}
	res, err := c.SecretFormat(nil, sca)
	assert.Error(err)
	assert.Empty(res)

	sca.Name = "testname"
	res, err = c.SecretFormat(nil, sca)
	assert.Error(err)
	assert.Empty(res)

	sca.Data = "somedata" // encoded value - c29tZWRhdGE
	res, err = c.SecretFormat(nil, sca)
	assert.Nil(err)
	assert.NotNil(res)
	assert.Equal(secretFakeFormat, res)

	// General
	scaj := &SecretCreateArgsMV{
		Name: "testname",
		Data: map[string]string{ //   eyJ0ZXN0IjoiZGF0YSIsInRlc3QyIjoiZGF0YTIifQ==
			"test":  "data",
			"test2": "data2",
		},
	}
	res, err = c.SecretFormatMV(nil, scaj)
	assert.Nil(err)
	assert.NotNil(res)
	assert.Equal(secretFakeFormatMv, res)

	// AccountIdentity
	scaj.Intent = SecretIntentAccountIdentity
	scaj.CustomizationData = AccountVolumeData{
		AccountSecret: "account-secret",
	}
	res, err = c.SecretFormatMV(nil, scaj)
	assert.Nil(err)
	assert.NotNil(res)
	assert.Equal(secretFakeFormatMvAI, res)

	// DynamicVolumeCustomization
	scaj = &SecretCreateArgsMV{
		Name:      "dvc",
		Namespace: "aNamespace",
		Intent:    SecretIntentDynamicVolumeCustomization,
	}
	res, err = c.SecretFormatMV(nil, scaj)
	assert.Error(err)
	assert.Empty(res)
	scaj.CustomizationData = AccountVolumeData{
		AccountSecret:               "account-secret",
		ApplicationGroupName:        "cg-name",
		ApplicationGroupDescription: "cg-d",
		ApplicationGroupTags:        []string{"ag-t1"},
		ConsistencyGroupName:        "cg-name",
		ConsistencyGroupDescription: "cg-desc",
		ConsistencyGroupTags:        []string{"cg-t1", "cg-t2"},
		VolumeTags:                  []string{"vol-t1", "vol-t2", "vol-t3"},
	}
	res, err = c.SecretFormatMV(nil, scaj)
	assert.Nil(err)
	assert.NotNil(res)
	assert.Equal(secretFakeFormatMvDVC, res)
}

var secretFakeFormat = `apiVersion: v1
kind: Secret
metadata:
  name: testname
  # Adjust the namespace as required.
  namespace: default
type: Opaque
data:
  nuvoloso-secret: c29tZWRhdGE=
`

var secretFakeFormatAI = `---
# This secret is required to use pre-provisioned PersistentVolumes
# of a Nuvoloso account. It must be present in every namespace
# where the corresponding PersistentVolumeClaim objects are created.
# Although it contains no customization metadata it may also be used
# by PersistentVolumeClaim objects that are used to request dynamically
# provisioned PersistentVolumes, with the following metadata annotation:
#    metadata:
#      annotations:
#        nuvoloso.com/provisioning-secret: testname
apiVersion: v1
kind: Secret
metadata:
  name: testname
  namespace: default
type: Opaque
data:
  nuvoloso-secret: c29tZWRhdGE=
`

var secretFakeFormatMv = `apiVersion: v1
kind: Secret
metadata:
  name: testname
  # Adjust the namespace as required.
  namespace: default
type: Opaque
data:
  nuvoloso-secret: eyJ0ZXN0IjoiZGF0YSIsInRlc3QyIjoiZGF0YTIifQ==
`

var secretFakeFormatMvAI = `---
# This secret is required to use pre-provisioned PersistentVolumes
# of a Nuvoloso account. It must be present in every namespace
# where the corresponding PersistentVolumeClaim objects are created.
# Although it contains no customization metadata it may also be used
# by PersistentVolumeClaim objects that are used to request dynamically
# provisioned PersistentVolumes of the same account.
apiVersion: v1
kind: Secret
metadata:
  name: testname
  namespace: default
type: Opaque
data:
  nuvoloso-secret: eyJhY2NvdW50U2VjcmV0IjoiYWNjb3VudC1zZWNyZXQifQ==
`

var secretFakeFormatMvDVC = `---
# This secret is used to create customized dynamically provisioned
# PersistentVolumes for a Nuvoloso account. It must be deployed in
# the same namespace as the PersistentVolumeClaim object used for
# dynamic provisioning, which should reference this customization
# secret in a metadata annotation as follows:
#    metadata:
#      annotations:
#        nuvoloso.com/provisioning-secret: dvc
# Multiple PVC objects may reference the same customization secret object.
apiVersion: v1
kind: Secret
metadata:
  name: dvc
  namespace: aNamespace
type: Opaque
data:
  nuvoloso-secret: eyJhY2NvdW50U2VjcmV0IjoiYWNjb3VudC1zZWNyZXQiLCJhcHBsaWNhdGlvbkdyb3VwTmFtZSI6ImNnLW5hbWUiLCJhcHBsaWNhdGlvbkdyb3VwRGVzY3JpcHRpb24iOiJjZy1kIiwiYXBwbGljYXRpb25Hcm91cFRhZ3MiOlsiYWctdDEiXSwiY29uc2lzdGVuY3lHcm91cE5hbWUiOiJjZy1uYW1lIiwiY29uc2lzdGVuY3lHcm91cERlc2NyaXB0aW9uIjoiY2ctZGVzYyIsImNvbnNpc3RlbmN5R3JvdXBUYWdzIjpbImNnLXQxIiwiY2ctdDIiXSwidm9sdW1lVGFncyI6WyJ2b2wtdDEiLCJ2b2wtdDIiLCJ2b2wtdDMiXX0=
`
