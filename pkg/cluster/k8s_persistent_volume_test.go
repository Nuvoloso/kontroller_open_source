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
	"encoding/json"
	"net/http"
	"testing"

	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

var k8sFakePVList = map[string]string{
	"/api/v1/persistentvolumes": `{
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
					"name": "nuvoloso-volume-d9837354-b889-4add-8c76-92eca54f36ca",
					"namespace": "",
					"resourceVersion": "1482616",
					"selfLink": "/api/v1/persistentvolumes/nuvoloso-volume-d9837354-b889-4add-8c76-92eca54f36ca",
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
	}`,
	"/api/v1/persistentvolumes?labelSelector=type%3Dnuvoloso-volume": `{
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
					"name": "nuvoloso-volume-d9837354-b889-4add-8c76-92eca54f36ca",
					"namespace": "",
					"resourceVersion": "1482616",
					"selfLink": "/api/v1/persistentvolumes/nuvoloso-volume-d9837354-b889-4add-8c76-92eca54f36ca",
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
	}`,
}

var k8sFakePV = map[string]string{
	"/api/v1/persistentvolumes": `{
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
					"name": "nuvoloso-volume-d9837354-b889-4add-8c76-92eca54f36ca",
					"namespace": "",
					"resourceVersion": "1482616",
					"selfLink": "/api/v1/persistentvolumes/nuvoloso-volume-d9837354-b889-4add-8c76-92eca54f36ca",
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
			}`,
}

var k8sFakePVDelete = map[string]string{
	"/api/v1/persistentvolumes/nuvoloso-volume-fakeid": `{
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
					"name": "nuvoloso-volume-d9837354-b889-4add-8c76-92eca54f36ca",
					"namespace": "",
					"resourceVersion": "1482616",
					"selfLink": "/api/v1/persistentvolumes/nuvoloso-volume-d9837354-b889-4add-8c76-92eca54f36ca",
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
					"phase": "Available"
				}
			}`,
	"/api/v1/persistentvolumes/nuvoloso-volume-fakeid2": `{
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
					"name": "nuvoloso-volume-d9837354-b889-4add-8c76-92eca54f36ca",
					"namespace": "",
					"resourceVersion": "1482616",
					"selfLink": "/api/v1/persistentvolumes/nuvoloso-volume-d9837354-b889-4add-8c76-92eca54f36ca",
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
			}`,
}

func TestPersistentVolumeList(t *testing.T) {
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
		data:       k8sFakePVList,
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

	pvs, err := c.PersistentVolumeList(ctx)
	assert.Nil(err)
	assert.NotNil(pvs)
	assert.Equal(2, len(pvs))

	pvs, err = c.PublishedPersistentVolumeList(ctx)
	assert.Nil(err)
	assert.NotNil(pvs)
	assert.Equal(2, len(pvs))

	// error
	k8s.httpClient = nil
	k8s.env.Host = ""
	k8s.shp = ""
	pvs, err = c.PersistentVolumeList(ctx)
	tl.Logger().Info(err)
	assert.NotNil(err)
	assert.Nil(pvs)

	pvs, err = c.PublishedPersistentVolumeList(ctx)
	tl.Logger().Info(err)
	assert.NotNil(err)
	assert.Nil(pvs)
}

func TestPersistentVolumeCreate(t *testing.T) {
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
	for k, v := range k8sFakePV {
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

	pvca := &PersistentVolumeCreateArgs{
		SizeBytes:       10737418240, //10Gi
		VolumeID:        "vol-1",
		FsType:          "ext4",
		AccountID:       "account1",
		SystemID:        "System1",
		DriverType:      "",
		ServicePlanName: "somename",
	}
	pv, err := c.PersistentVolumeCreate(ctx, pvca)
	assert.Nil(err)
	assert.NotNil(pv)
	assert.NotEqual("", pvca.DriverType)

	pvca = &PersistentVolumeCreateArgs{
		SizeBytes:       10737418240, //10Gi
		VolumeID:        "vol-1",
		FsType:          "ext4",
		AccountID:       "account1",
		SystemID:        "System1",
		DriverType:      "csi",
		ServicePlanName: "somename",
	}
	pv, err = c.PersistentVolumeCreate(ctx, pvca)
	assert.Nil(err)
	assert.NotNil(pv)

	// already exists error skipped from K8s
	ft.statusCode["/api/v1/persistentvolumes"] = 409
	ft.data["/api/v1/persistentvolumes"] = `{
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
	ft.statusCode["/api/v1/persistentvolumes/nuvoloso-volume-vol-1"] = 200
	ft.data["/api/v1/persistentvolumes/nuvoloso-volume-vol-1"] = `{
		"apiVersion": "v1",
		"kind": "PersistentVolume",
		"metadata": {
			"labels": {
				"type": "nuvoloso-volume",
				"volumeId":   "vol-1",
				"accountId":  "account1",
				"fsType":     "ext4",
				"driverType": "csi",
				"systemId":   "System1",
				"sizeBytes":  "10737418240"
			}
		}
	}`
	pv, err = c.PersistentVolumeCreate(ctx, pvca)
	assert.Nil(err)
	assert.Nil(pv)

	// already exists error not skipped because labels differ
	ft.statusCode["/api/v1/persistentvolumes"] = 409
	ft.data["/api/v1/persistentvolumes"] = `{
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
	ft.statusCode["/api/v1/persistentvolumes/nuvoloso-volume-vol-1"] = 200
	ft.data["/api/v1/persistentvolumes/nuvoloso-volume-vol-1"] = `{
			"apiVersion": "v1",
			"kind": "PersistentVolume",
			"metadata": {
				"labels": {
					"type": "nuvoloso-volume",
					"volumeId":   "vol-1",
					"accountId":  " wrongaccount1",
					"fsType":     "ext4",
					"driverType": "csi",
					"systemId":   "System1",
					"sizeBytes":  "10737418240"
				}
			}
		}`
	pv, err = c.PersistentVolumeCreate(ctx, pvca)
	assert.NotNil(err)
	assert.Regexp("value incorrect", err.Error())
	assert.Nil(pv)

	// already exists error not skipped because labels are missing
	ft.statusCode["/api/v1/persistentvolumes"] = 409
	ft.data["/api/v1/persistentvolumes"] = `{
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
	ft.statusCode["/api/v1/persistentvolumes/nuvoloso-volume-vol-1"] = 200
	ft.data["/api/v1/persistentvolumes/nuvoloso-volume-vol-1"] = `{
				"apiVersion": "v1",
				"kind": "PersistentVolume",
				"metadata": {
					"labels": {
						"type": "nuvoloso-volume",
						"volumeId":   "vol-1"
					}
				}
			}`
	pv, err = c.PersistentVolumeCreate(ctx, pvca)
	assert.NotNil(err)
	assert.Regexp("missing", err.Error())
	assert.Nil(pv)

	// already exists error fetch causes error
	ft.statusCode["/api/v1/persistentvolumes"] = 409
	ft.data["/api/v1/persistentvolumes"] = `{
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

	ft.statusCode["/api/v1/persistentvolumes/nuvoloso-volume-vol-1"] = 400
	pv, err = c.PersistentVolumeCreate(ctx, pvca)
	assert.NotNil(err)
	assert.Nil(pv)

	// error returned from K8s
	ft.statusCode["/api/v1/persistentvolumes"] = 404
	ft.data["/api/v1/persistentvolumes"] = `{
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
	pv, err = c.PersistentVolumeCreate(ctx, pvca)
	assert.NotNil(err)
	assert.Nil(pv)
	assert.Regexp("Some Error Message", err.Error())

	missingTCs := []PersistentVolumeCreateArgs{
		PersistentVolumeCreateArgs{SizeBytes: 0, VolumeID: "Someid", FsType: "ext4", AccountID: "account1", SystemID: "System1", DriverType: "csi", ServicePlanName: "somename"},
		PersistentVolumeCreateArgs{SizeBytes: 10737418240, VolumeID: "", FsType: "ext4", AccountID: "account1", SystemID: "System1", DriverType: "csi", ServicePlanName: "somename"},
		PersistentVolumeCreateArgs{SizeBytes: 10737418240, VolumeID: "Someid", FsType: "", AccountID: "account1", SystemID: "System1", DriverType: "csi", ServicePlanName: "somename"},
		PersistentVolumeCreateArgs{SizeBytes: 10737418240, VolumeID: "Someid", FsType: "ext4", AccountID: "", SystemID: "System1", DriverType: "csi", ServicePlanName: "somename"},
		PersistentVolumeCreateArgs{SizeBytes: 10737418240, VolumeID: "Someid", FsType: "ext4", AccountID: "account1", SystemID: "", DriverType: "csi", ServicePlanName: "somename"},
		PersistentVolumeCreateArgs{SizeBytes: 10737418240, VolumeID: "Someid", FsType: "ext4", AccountID: "account1", SystemID: "System1", DriverType: "baddriver", ServicePlanName: "somename"},
		PersistentVolumeCreateArgs{SizeBytes: 10737418240, VolumeID: "Someid", FsType: "invalid", AccountID: "account1", SystemID: "System1", DriverType: "csi", ServicePlanName: "somename"},
		PersistentVolumeCreateArgs{SizeBytes: 10737418240, VolumeID: "Someid", FsType: "ext4", AccountID: "account1", SystemID: "System1", DriverType: "csi", ServicePlanName: ""},
	}
	for _, tc := range missingTCs {
		pv, err = c.PersistentVolumeCreate(ctx, &tc)
		assert.NotNil(err)
		assert.Nil(pv)
	}

	// error
	k8s.httpClient = nil
	k8s.env.Host = ""
	k8s.shp = ""
	pv, err = c.PersistentVolumeCreate(ctx, pvca)
	tl.Logger().Info(err)
	assert.NotNil(err)
	assert.Nil(pv)
}

func TestPersistentVolumeDelete(t *testing.T) {
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
		data:       k8sFakePVDelete,
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

	pvda := &PersistentVolumeDeleteArgs{
		VolumeID: "fakeid",
	}
	pv, err := c.PersistentVolumeDelete(ctx, pvda)
	assert.Nil(err)
	assert.NotNil(pv)

	pvda.VolumeID = "fakeid2"
	pv, err = c.PersistentVolumeDelete(ctx, pvda)
	assert.NotNil(err)
	assert.Nil(pv)
	k8sErr, ok := err.(*k8sError)
	assert.True(ok)
	assert.True(k8sErr.PVIsBound())

	// error returned from K8s
	pvda.VolumeID = "fakeid"
	ft.statusCode["/api/v1/persistentvolumes/nuvoloso-volume-fakeid"] = 404
	ft.data["/api/v1/persistentvolumes/nuvoloso-volume-fakeid"] = `{
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
	pv, err = c.PersistentVolumeDelete(ctx, pvda)
	assert.NotNil(err)
	assert.Nil(pv)
	assert.Regexp("Some Error Message", err.Error())

	// error
	k8s.httpClient = nil
	k8s.env.Host = ""
	k8s.shp = ""
	pv, err = c.PersistentVolumeDelete(ctx, pvda)
	tl.Logger().Info(err)
	assert.NotNil(err)
	assert.Nil(pv)
}

func TestPVCreateArgsToPostBody(t *testing.T) {
	assert := assert.New(t)
	tl := testutils.NewTestLogger(t)
	defer tl.Flush()
	c, err := NewClient(K8sClusterType)
	assert.Nil(err)
	c.SetDebugLogger(tl.Logger())
	k8s, ok := c.(*K8s)
	assert.True(ok)
	sca := &PersistentVolumeCreateArgs{
		SizeBytes:       10737418240, //10Gi
		VolumeID:        "vol-1",
		FsType:          "ext4",
		AccountID:       "account1",
		SystemID:        "System1",
		DriverType:      "FLEX",
		ServicePlanName: "somename",
	}

	var postBody pvPostBody
	postBody.Init(k8s, sca)
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
	_, exists = keyMap["spec"]
	assert.True(exists)
	assert.Contains(keyMap["spec"], "flexVolume")

	keyMap2 := keyMap["metadata"].(map[string]interface{})
	keyMap3, exists := keyMap2["labels"].(map[string]interface{})
	assert.True(exists)
	label, exists := keyMap3["type"]
	assert.True(exists)
	assert.Equal("nuvo-vol", label)

	keyMap2 = keyMap["spec"].(map[string]interface{})
	_, exists = keyMap2["storageClassName"]
	assert.False(exists)

	// ***************** CSI
	sca = &PersistentVolumeCreateArgs{
		SizeBytes:       10737418240, //10Gi
		VolumeID:        "vol-1",
		FsType:          "ext4",
		AccountID:       "account1",
		SystemID:        "System1",
		DriverType:      "csi",
		ServicePlanName: "somename",
	}

	postBody.Init(k8s, sca)
	jsonString, err = json.Marshal(postBody)
	assert.Nil(err)
	keyMap = make(map[string]interface{})
	err = json.Unmarshal(jsonString, &keyMap)
	_, exists = keyMap["spec"]
	assert.True(exists)
	assert.Contains(keyMap["spec"], "csi")
}

func TestPersistentVolumePublish(t *testing.T) {
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
	for k, v := range k8sFakePV {
		ft.data[k] = v
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

	pvca := &PersistentVolumeCreateArgs{
		SizeBytes:       10737418240, //10Gi
		VolumeID:        "vol-1",
		FsType:          "ext4",
		AccountID:       "account1",
		SystemID:        "System1",
		DriverType:      "flex",
		ServicePlanName: "somename",
	}
	cd, err := c.PersistentVolumePublish(ctx, pvca)
	assert.Nil(err)
	assert.NotNil(cd)
	assert.Contains(cd, com.K8sPvcYaml)
	assert.NotContains(cd[com.K8sPvcYaml].Value, "nuvoloso-somename")
	assert.Contains(cd, com.ClusterDescriptorPVName)
	assert.Equal("nuvoloso-volume-d9837354-b889-4add-8c76-92eca54f36ca", cd[com.ClusterDescriptorPVName].Value) // from k8sFakePV

	// csi
	pvca = &PersistentVolumeCreateArgs{
		SizeBytes:       10737418240, //10Gi
		VolumeID:        "vol-1",
		FsType:          "ext4",
		AccountID:       "account1",
		SystemID:        "System1",
		DriverType:      "csi",
		ServicePlanName: "somename",
	}
	cd, err = c.PersistentVolumePublish(ctx, pvca)
	assert.Nil(err)
	assert.NotNil(cd)
	assert.Contains(cd, com.K8sPvcYaml)
	assert.Contains(cd, com.ClusterDescriptorPVName)
	assert.Equal("nuvoloso-volume-d9837354-b889-4add-8c76-92eca54f36ca", cd[com.ClusterDescriptorPVName].Value) // from k8sFakePV

	pvca = &PersistentVolumeCreateArgs{
		SizeBytes:       10737418240, //10Gi
		VolumeID:        "",
		FsType:          "ext4",
		AccountID:       "account1",
		SystemID:        "System1",
		DriverType:      "flex",
		ServicePlanName: "somename",
	}
	cd, err = c.PersistentVolumePublish(ctx, pvca)
	assert.NotNil(err)
	assert.Nil(cd)

	// error
	k8s.httpClient = nil
	k8s.env.Host = ""
	k8s.shp = ""
	cd, err = c.PersistentVolumePublish(ctx, pvca)
	tl.Logger().Info(err)
	assert.NotNil(err)
	assert.Nil(cd)
}

var fakePVListRes = `{
    "kind": "PersistentVolumeList",
    "apiVersion": "v1",
    "metadata": {
        "selfLink": "/api/v1/persistentvolumes",
        "resourceVersion": "1"
    },
    "items": [
        {
            "metadata": {
                "name": "nuvoloso-volume-2fac26a6-90d5-42f6-bdad-04fb052dad88",
                "selfLink": "/api/v1/persistentvolumes/nuvoloso-volume-2fac26a6-90d5-42f6-bdad-04fb052dad88",
                "uid": "5aa347bb-be0a-11e9-a15e-02b73a78e4f4",
                "resourceVersion": "116581",
                "creationTimestamp": "2019-08-13T20:38:49Z",
                "labels": {
                    "accountId": "d86a476d-03a1-4cc2-b00a-be9ffe0afc32",
                    "driverType": "csi",
                    "fsType": "ext4",
                    "sizeBytes": "1073741824",
                    "systemId": "ae40e342-88cd-439d-8c8e-3a5f5bc13bcb",
                    "type": "nuvoloso-volume",
                    "volumeId": "2fac26a6-90d5-42f6-bdad-04fb052dad88"
                },
                "finalizers": [
                    "kubernetes.io/pv-protection"
                ]
            },
            "spec": {
                "capacity": {
                    "storage": "1Gi"
                },
                "csi": {
                    "driver": "csi.nuvoloso.com",
                    "volumeHandle": "2fac26a6-90d5-42f6-bdad-04fb052dad88"
                },
                "accessModes": [
                    "ReadWriteOnce"
                ],
                "persistentVolumeReclaimPolicy": "Retain",
                "storageClassName": "nuvoloso-general",
                "volumeMode": "Filesystem"
            },
            "status": {
                "phase": "Available"
            }
        },
        {
            "metadata": {
                "name": "nuvoloso-volume-f5eff2c5-e717-44fa-af1a-7f60067a5dea",
                "selfLink": "/api/v1/persistentvolumes/nuvoloso-volume-f5eff2c5-e717-44fa-af1a-7f60067a5dea",
                "uid": "60b79325-be0a-11e9-a15e-02b73a78e4f4",
                "resourceVersion": "116597",
                "creationTimestamp": "2019-08-13T20:38:59Z",
                "labels": {
                    "accountId": "d86a476d-03a1-4cc2-b00a-be9ffe0afc32",
                    "driverType": "csi",
                    "fsType": "ext4",
                    "sizeBytes": "1073741824",
                    "systemId": "ae40e342-88cd-439d-8c8e-3a5f5bc13bcb",
                    "type": "nuvoloso-volume",
                    "volumeId": "f5eff2c5-e717-44fa-af1a-7f60067a5dea"
                },
                "finalizers": [
                    "kubernetes.io/pv-protection"
                ]
            },
            "spec": {
                "capacity": {
                    "storage": "1Gi"
                },
                "csi": {
                    "driver": "csi.nuvoloso.com",
                    "volumeHandle": "f5eff2c5-e717-44fa-af1a-7f60067a5dea"
                },
                "accessModes": [
                    "ReadWriteOnce"
                ],
                "persistentVolumeReclaimPolicy": "Retain",
                "storageClassName": "nuvoloso-general",
                "volumeMode": "Filesystem"
            },
            "status": {
                "phase": "Available"
            }
        }
    ]
}`

var pvWatcherModEvent = `{"type":"MODIFIED","object":{"kind":"PersistentVolume","apiVersion":"v1","metadata":{"name":"nuvoloso-volume-6be07df1-e4c2-49c4-a691-fe29c85f6793","selfLink":"/api/v1/persistentvolumes/nuvoloso-volume-6be07df1-e4c2-49c4-a691-fe29c85f6793","uid":"ee8d1d18-b93b-11e9-8159-028cf555dd6a","resourceVersion":"110831","creationTimestamp":"2019-08-07T17:51:06Z","labels":{"accountId":"1c943f84-b641-4f1e-8f3e-ba706e9fef63","driverType":"csi","fsType":"ext4","sizeBytes":"2147483648","systemId":"b9bed48c-ae04-4d95-b1f8-85716fa1eca0","type":"nuvoloso-volume","volumeId":"6be07df1-e4c2-49c4-a691-fe29c85f6793"}},"spec":{"capacity":{"storage":"2Gi"},"csi":{"driver":"csi.nuvoloso.com","volumeHandle":"6be07df1-e4c2-49c4-a691-fe29c85f6793"},"accessModes":["ReadWriteOnce"],"persistentVolumeReclaimPolicy":"Retain","storageClassName":"nuvoloso-general","volumeMode":"Filesystem"},"status":{"phase":"Available"}}}`
var pvWatcherDelEvent = `{"type":"DELETED","object":{"kind":"PersistentVolume","apiVersion":"v1","metadata":{"name":"nuvoloso-volume-d6e97364-6358-4985-a086-112930fde889","selfLink":"/api/v1/persistentvolumes/nuvoloso-volume-d6e97364-6358-4985-a086-112930fde889","uid":"d4a88093-ba1a-11e9-8159-028cf555dd6a","resourceVersion":"338602","creationTimestamp":"2019-08-08T20:26:41Z","deletionTimestamp":"2019-08-09T18:29:21Z","deletionGracePeriodSeconds":0,"labels":{"accountId":"1c943f84-b641-4f1e-8f3e-ba706e9fef63","driverType":"csi","fsType":"ext4","sizeBytes":"1073741824","systemId":"b9bed48c-ae04-4d95-b1f8-85716fa1eca0","type":"nuvoloso-volume","volumeId":"d6e97364-6358-4985-a086-112930fde889"},"finalizers":["kubernetes.io/pv-protection"]},"spec":{"capacity":{"storage":"1Gi"},"csi":{"driver":"csi.nuvoloso.com","volumeHandle":"d6e97364-6358-4985-a086-112930fde889"},"accessModes":["ReadWriteOnce"],"persistentVolumeReclaimPolicy":"Retain","storageClassName":"nuvoloso-general","volumeMode":"Filesystem"},"status":{"phase":"Available"}}}`

func TestPersistentVolumeWatcher(t *testing.T) {
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
	ft.data["/api/v1/persistentvolumes?labelSelector=type%3Dnuvoloso-volume"] = fakePVListRes // resource version is "1"
	ft.statusCode["/api/v1/persistentvolumes?labelSelector=type%3Dnuvoloso-volume"] = 200
	ft.data["/api/v1/persistentvolumes?labelSelector=type%3Dnuvoloso-volume&watch=1&resourceVersion=1"] = k8sPVWatchSample // resource version is "1"
	ft.statusCode["/api/v1/persistentvolumes?labelSelector=type%3Dnuvoloso-volume&watch=1&resourceVersion=1"] = 200
	http.DefaultClient.Transport = ft
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

	// success
	fakePVWOps := &fakePVWatcherOps{}
	args := &PVWatcherArgs{
		Timeout: 0,
		Ops:     fakePVWOps,
	}
	watcher, res, err := c.CreatePublishedPersistentVolumeWatcher(ctx, args)
	assert.Nil(err)
	assert.NotNil(watcher)
	wtchr, ok := watcher.(*K8sPVWatcher)
	assert.True(ok)
	assert.Equal("/api/v1/persistentvolumes?labelSelector=type%3Dnuvoloso-volume", wtchr.url)
	assert.Equal(c, wtchr.c)
	assert.Equal("1", wtchr.resourceVersion)
	assert.Equal(fakePVWOps, wtchr.pvOps)
	assert.Equal(tl.Logger(), wtchr.log)
	assert.Equal(2, len(res))
	assert.False(wtchr.isOpen)
	assert.NotNil(wtchr.client)

	// test WatchEvent and WatchObject using above watcher
	eventObj := wtchr.WatchObject()
	e, ok := eventObj.(*K8sPVWatchEvent)
	assert.True(ok)
	json.Unmarshal([]byte(pvWatcherModEvent), &e)
	wtchr.WatchEvent(ctx, e)
	assert.Nil(fakePVWOps.pvDIn)

	json.Unmarshal([]byte(pvWatcherDelEvent), &e)
	wtchr.WatchEvent(ctx, e)
	assert.Equal(e.Object.ToModel(), fakePVWOps.pvDIn)
	tl.Flush()

	// k8s list error
	fakePVWOps = &fakePVWatcherOps{}
	args = &PVWatcherArgs{
		Timeout: 0,
		Ops:     fakePVWOps,
	}
	ft.data["/api/v1/persistentvolumes?labelSelector=type%3Dnuvoloso-volume"] = "someerror"
	ft.statusCode["/api/v1/persistentvolumes?labelSelector=type%3Dnuvoloso-volume"] = 400
	watcher, res, err = c.CreatePublishedPersistentVolumeWatcher(ctx, args)
	assert.NotNil(err)
	assert.Regexp("someerror", err.Error())
	assert.Nil(watcher)
	assert.Nil(res)

	// error: client failure
	k8s.httpClient = nil
	k8s.env = nil
	watcher, res, err = c.CreatePublishedPersistentVolumeWatcher(ctx, args)
	assert.NotNil(err)
	assert.Regexp(K8sEnvServerHost+".*"+K8sEnvServerPort, err.Error())
	assert.Nil(res)

	// validate args
	badArgs := []*PVWatcherArgs{
		&PVWatcherArgs{},
		&PVWatcherArgs{Timeout: 0},
		&PVWatcherArgs{Ops: fakePVWOps},
		&PVWatcherArgs{Ops: fakePVWOps, Timeout: -1},
	}
	for _, args := range badArgs {
		_, _, err = c.CreatePublishedPersistentVolumeWatcher(ctx, args)
		assert.Error(err)
	}
}
