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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"text/template"

	com "github.com/Nuvoloso/kontroller/pkg/common"
)

// SecretKey is the key value used when storing nuvoloso secret data in kubernetes secret.Datamap

// SecretFetch fetches a Kubernetes secret given the args (name, namespace)
func (c *K8s) SecretFetch(ctx context.Context, sfa *SecretFetchArgs) (*SecretObj, error) {
	if sfa.Name == "" || sfa.Namespace == "" {
		return nil, fmt.Errorf("Required field(s) Name, Namespace are missing")
	}
	path := fmt.Sprintf("/api/v1/namespaces/%s/secrets/%s", sfa.Namespace, sfa.Name)
	var secret *K8sSecret
	err := c.K8sClientGetJSON(ctx, path, &secret)
	if err != nil {
		return nil, err
	}
	var res *SecretObj
	res = secret.ToModel()
	return res, nil
}

// SecretFetchMV fetches the secret data, tries to decode it as json and return the value
func (c *K8s) SecretFetchMV(ctx context.Context, sfa *SecretFetchArgs) (*SecretObjMV, error) {
	sObj, err := c.SecretFetch(ctx, sfa)
	if err != nil {
		return nil, err
	}
	return c.secretDecodeMV(sObj)
}

func (c *K8s) secretDecodeMV(sObj *SecretObj) (*SecretObjMV, error) {
	retS := &SecretObjMV{}
	retS.Name = sObj.Name
	retS.Namespace = sObj.Namespace
	retS.Raw = sObj.Raw
	if err := retS.Unmarshal([]byte(sObj.Data)); err != nil {
		return nil, err
	}
	return retS, nil
}

// SecretCreate performs a POST operation to create a Secret
func (c *K8s) SecretCreate(ctx context.Context, sca *SecretCreateArgs) (*SecretObj, error) {
	var secretBody secretPostBody
	secretBody.Init(sca)
	var secret *K8sSecret
	path := fmt.Sprintf("/api/v1/namespaces/%s/secrets", sca.Namespace)
	err := c.K8sClientPostJSON(ctx, path, secretBody.Marshal(), &secret)
	if err != nil {
		return nil, err
	}
	var res *SecretObj
	res = secret.ToModel()
	return res, nil
}

// SecretCreateMV takes a secret that is a map, json encodes it, and creates it in kubernetes
func (c *K8s) SecretCreateMV(ctx context.Context, scaj *SecretCreateArgsMV) (*SecretObjMV, error) {
	secretBytes, _ := json.Marshal(scaj.Data)
	sca := &SecretCreateArgs{
		Name:      scaj.Name,
		Namespace: scaj.Namespace,
		Data:      string(secretBytes),
	}
	createRes, err := c.SecretCreate(ctx, sca)
	if err != nil {
		return nil, err
	}
	return c.secretDecodeMV(createRes)
}

// SecretFormat takes the secret create args and returns a yaml string representation
func (c *K8s) SecretFormat(ctx context.Context, sca *SecretCreateArgs) (string, error) {
	if err := sca.ValidateForFormat(); err != nil {
		return "", err
	}
	return c.encodeSecret(sca, SecretIntentGeneral), nil
}

func (c *K8s) encodeSecret(sca *SecretCreateArgs, intent SecretIntent) string {
	ns := sca.Namespace
	if ns == "" {
		ns = "default"
	}
	expectedLen := base64.StdEncoding.EncodedLen(len(sca.Data))
	encodedData := make([]byte, expectedLen)
	base64.StdEncoding.Encode(encodedData, []byte(sca.Data))
	m := map[string]string{
		"Name":                   sca.Name,
		"Namespace":              ns,
		"SecretKey":              com.K8sSecretKey,
		"SecretData":             string(encodedData),
		"PvcSecretAnnotationKey": com.K8sPvcSecretAnnotationKey,
	}
	t, _ := template.New("").Parse(k8sSecretTemplates[intent])
	b := &bytes.Buffer{}
	_ = t.Execute(b, m)
	return b.String()
}

// SecretFormatMV takes the secret data, json encodes it, and returns a yaml string representation
// that depends on the intent.
func (c *K8s) SecretFormatMV(ctx context.Context, scaj *SecretCreateArgsMV) (string, error) {
	if err := scaj.ValidateForFormat(); err != nil {
		return "", err
	}
	return c.encodeSecret(c.secretEncodeMV(scaj), scaj.Intent), nil
}

func (c *K8s) secretEncodeMV(scaj *SecretCreateArgsMV) *SecretCreateArgs {
	mvO := &SecretObjMV{}
	mvO.Intent = scaj.Intent
	mvO.Data = scaj.Data
	mvO.CustomizationData = scaj.CustomizationData
	return &SecretCreateArgs{
		Name:      scaj.Name,
		Namespace: scaj.Namespace,
		Data:      string(mvO.Marshal()),
	}
}

// ToModel converts a K8sSecret object to a SecretObj
func (s *K8sSecret) ToModel() *SecretObj {
	var secretData string
	if s.Data != nil {
		if val, ok := s.Data[com.K8sSecretKey]; ok {
			secretData = string(val)
		}
	}
	sObj := &SecretObj{
		SecretCreateArgs: SecretCreateArgs{
			Name:      s.Name,
			Namespace: s.Namespace,
			Data:      secretData,
		},
		Raw: s,
	}
	return sObj
}

// secretPostBody
type secretPostBody struct {
	APIVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Metadata   *ObjectMeta       `json:"metadata"`
	StringData map[string]string `json:"stringData"`
}

func (spb *secretPostBody) Init(sca *SecretCreateArgs) {
	spb.APIVersion = "v1"
	spb.Kind = "Secret"
	spb.Metadata = &ObjectMeta{
		Name:      sca.Name,
		Namespace: sca.Namespace,
	}
	spb.StringData = map[string]string{com.K8sSecretKey: sca.Data}
}

func (spb *secretPostBody) Marshal() []byte {
	body, _ := json.Marshal(spb) // strict input type so Marshal won't fail
	return body
}

// K8sSecretTemplate is a template to create an arbitrary secret in Kubernetes
var K8sSecretTemplate = `apiVersion: v1
kind: Secret
metadata:
  name: {{.Name}}
  # Adjust the namespace as required.
  namespace: default
type: Opaque
data:
  {{.SecretKey}}: {{.SecretData}}
`

// K8sAccountSecretTemplate is a template to create account identity secrets in kubernetes
var K8sAccountSecretTemplate = `---
# This secret is required to use pre-provisioned PersistentVolumes
# of a Nuvoloso account. It must be present in every namespace
# where the corresponding PersistentVolumeClaim objects are created.
# Although it contains no customization metadata it may also be used
# by PersistentVolumeClaim objects that are used to request dynamically
# provisioned PersistentVolumes of the same account.
apiVersion: v1
kind: Secret
metadata:
  name: {{.Name}}
  namespace: {{.Namespace}}
type: Opaque
data:
  {{.SecretKey}}: {{.SecretData}}
`

// K8sCustomizationSecretTemplate is a template to dynamic volume customization secrets in kubernetes
var K8sCustomizationSecretTemplate = `---
# This secret is used to create customized dynamically provisioned
# PersistentVolumes for a Nuvoloso account. It must be deployed in
# the same namespace as the PersistentVolumeClaim object used for
# dynamic provisioning, which should reference this customization
# secret in a metadata annotation as follows:
#    metadata:
#      annotations:
#        {{.PvcSecretAnnotationKey}}: {{.Name}}
# Multiple PVC objects may reference the same customization secret object.
apiVersion: v1
kind: Secret
metadata:
  name: {{.Name}}
  namespace: {{.Namespace}}
type: Opaque
data:
  {{.SecretKey}}: {{.SecretData}}
`

var k8sSecretTemplates = map[SecretIntent]string{
	SecretIntentGeneral:                    K8sSecretTemplate,
	SecretIntentAccountIdentity:            K8sAccountSecretTemplate,
	SecretIntentDynamicVolumeCustomization: K8sCustomizationSecretTemplate,
}
