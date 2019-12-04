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


package gc

import (
	"context"
	"errors"
	"io/ioutil"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	com "github.com/Nuvoloso/kontroller/pkg/common"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/Nuvoloso/kontroller/pkg/gcsdk"
	"github.com/Nuvoloso/kontroller/pkg/gcsdk/mock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/option"
)

type gcFakeClientInitializer struct {
	err error
}

func (fci *gcFakeClientInitializer) clientInit(cl *Client) error {
	return fci.err
}

func TestClientInit(t *testing.T) {
	assert := assert.New(t)

	csp, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(csp)
	gcCSP, ok := csp.(*CSP)
	assert.True(ok)
	assert.Equal(gcCSP, gcCSP.clInit) // self-ref

	gcCl := &Client{
		csp: gcCSP,
	}

	err = gcCSP.clientInit(gcCl)
	assert.Regexp("required domain attributes missing", err)

	gcCredsFileContent, _ := ioutil.ReadFile("./gc_creds.json")
	gcCredAttrs := map[string]models.ValueType{
		AttrCred: models.ValueType{Kind: com.ValueTypeSecret, Value: string(gcCredsFileContent)},
		AttrZone: models.ValueType{Kind: com.ValueTypeString, Value: "my_zone"},
	}

	// real Client calls
	dObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID: "CSP-DOMAIN-1",
			},
			CspDomainType:       models.CspDomainTypeMutable(CSPDomainType),
			CspDomainAttributes: gcCredAttrs,
		},
		CSPDomainMutable: models.CSPDomainMutable{},
	}
	dc, err := gcCSP.Client(dObj)
	assert.NoError(err)
	gcCl, ok = dc.(*Client)
	assert.True(ok)
	assert.Equal(gcCSP, gcCl.csp)
	assert.Equal(time.Second*clientDefaultTimeoutSecs, gcCl.Timeout)
	assert.Equal(dObj.CspDomainAttributes, gcCl.attrs)
	assert.Equal(dObj.Meta.ID, dc.ID())
	assert.Equal(dObj.CspDomainType, dc.Type())
	assert.Equal("client-test-project-12345", gcCl.projectID)

	// timeout can be set
	dc.SetTimeout(clientDefaultTimeoutSecs + 1)
	assert.EqualValues((clientDefaultTimeoutSecs+1)*time.Second, gcCl.Timeout)
	dc.SetTimeout(-1)
	assert.EqualValues(clientDefaultTimeoutSecs*time.Second, gcCl.Timeout)

	// invalid domain obj
	dObj.CspDomainType = ""
	dc, err = gcCSP.Client(dObj)
	assert.Error(err)
	assert.Regexp("invalid CSPDomain", err)
	assert.Nil(dc)

	// bad JSON
	gcCredsFileContent, _ = ioutil.ReadFile("./gc_creds_bad.json")
	gcCredAttrs = map[string]models.ValueType{
		AttrCred: models.ValueType{Kind: com.ValueTypeSecret, Value: string(gcCredsFileContent)},
		AttrZone: models.ValueType{Kind: com.ValueTypeString, Value: "my_zone"},
	}
	dObj.CspDomainType = models.CspDomainTypeMutable(CSPDomainType)
	dObj.CspDomainAttributes = gcCredAttrs
	dc, err = gcCSP.Client(dObj)
	assert.Error(err)
	assert.Regexp("could not parse gc_cred", err)
}

func TestCreateProtectionStore(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()

	csp, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(csp)
	gcCSP, ok := csp.(*CSP)
	assert.True(ok)
	assert.NotNil(gcCSP)

	gcCl := &Client{
		csp:       gcCSP,
		api:       gcsdk.New(),
		attrs:     map[string]models.ValueType{},
		projectID: "test-proj-1",
	}

	// invalid AttrCred
	attrs, err := gcCl.CreateProtectionStore(ctx)
	assert.Regexp("unexpected end of JSON input", err)
	assert.Error(errors.Unwrap(err))
	assert.Nil(attrs)

	// (cl.Timeout is zero), minimal serviceAccount
	ctx, cancelFn := context.WithTimeout(ctx, 0*time.Second)
	defer cancelFn()
	serviceAccount := `{ "type": "service_account", "project_id": "test-proj-1" }`
	gcCl.attrs[AttrCred] = models.ValueType{Kind: "SECRET", Value: serviceAccount}
	gcCl.attrs[AttrPStoreBucketName] = models.ValueType{Kind: "STRING", Value: "nuvoloso-1-2-3-4"}
	attrs, err = gcCl.CreateProtectionStore(ctx)
	assert.Regexp("protection store creation failed: context deadline exceeded", err)
	assert.Error(errors.Unwrap(err))

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	cl := mock.NewMockStorageClient(mockCtrl)
	api.EXPECT().NewStorageClient(ctx, option.WithCredentialsJSON([]byte(serviceAccount))).Return(cl, nil)
	bucket := mock.NewMockBucketHandle(mockCtrl)
	cl.EXPECT().Bucket("nuvoloso-1-2-3-4").Return(bucket)
	bucketAttrs := &storage.BucketAttrs{BucketPolicyOnly: storage.BucketPolicyOnly{Enabled: true}}
	bucket.EXPECT().Create(ctx, "test-proj-1", bucketAttrs).Return(nil)
	_, err = gcCl.CreateProtectionStore(ctx)
	assert.NoError(err)
}

func TestValidate(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()

	csp, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(csp)
	gcCSP, ok := csp.(*CSP)
	assert.True(ok)
	assert.NotNil(gcCSP)

	gcCl := &Client{
		csp:       gcCSP,
		api:       gcsdk.New(),
		attrs:     map[string]models.ValueType{},
		projectID: "test-proj-1",
	}

	// invalid AttrCred
	err = gcCl.Validate(ctx)
	assert.Regexp("unexpected end of JSON input", err)
	assert.Error(errors.Unwrap(err))

	// again with nil context (cl.Timeout is zero), minimal serviceAccount
	serviceAccount := `{ "type": "service_account", "project_id": "test-proj-1" }`
	gcCl.attrs[AttrCred] = models.ValueType{Kind: "SECRET", Value: serviceAccount}
	gcCl.attrs[AttrZone] = models.ValueType{Kind: "STRING", Value: "test-zone"}
	err = gcCl.Validate(nil)
	assert.Regexp("context deadline exceeded", err)
	assert.NoError(errors.Unwrap(err))

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	api := mock.NewMockAPI(mockCtrl)
	gcCl.api = api
	svc := mock.NewMockComputeService(mockCtrl)
	api.EXPECT().NewComputeService(ctx, option.WithCredentialsJSON([]byte(serviceAccount))).Return(svc, nil)
	zones := mock.NewMockZonesService(mockCtrl)
	svc.EXPECT().Zones().Return(zones)
	get := mock.NewMockZonesGetCall(mockCtrl)
	zones.EXPECT().Get("test-proj-1", "test-zone").Return(get)
	get.EXPECT().Context(ctx).Return(get)
	get.EXPECT().Do().Return(nil, nil) // only err is inspected
	assert.NoError(gcCl.Validate(ctx))
}

func TestValidateCredential(t *testing.T) {
	assert := assert.New(t)

	csp, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(csp)
	gcCSP, ok := csp.(*CSP)
	assert.True(ok)
	assert.NotNil(gcCSP)

	// invalid DomainType
	err = gcCSP.ValidateCredential("DomainType", map[string]models.ValueType{})
	assert.Regexp("invalid DomainType", err)

	// no attrs
	err = gcCSP.ValidateCredential(CSPDomainType, map[string]models.ValueType{})
	assert.Regexp("required domain attributes missing or invalid", err)

	attrs := map[string]models.ValueType{
		AttrPStoreBucketName: models.ValueType{Kind: "STRING", Value: "my_bucket_name"},
	}

	// missing attrs
	err = gcCSP.ValidateCredential(CSPDomainType, attrs)
	assert.Regexp("required domain attributes missing or invalid", err)

	// invalid attrs
	attrs[AttrCred] = models.ValueType{Kind: "STRING", Value: "{}"}
	attrs[AttrZone] = models.ValueType{Kind: "STRING", Value: "my_zone"}
	err = gcCSP.ValidateCredential(CSPDomainType, attrs)
	assert.Regexp("required domain attributes missing or invalid", err)

	// invalid cred
	attrs[AttrCred] = models.ValueType{Kind: "SECRET", Value: "{}"}
	err = gcCSP.ValidateCredential(CSPDomainType, attrs)
	assert.Regexp("missing 'type' field in credentials", err)

	// bad JSON
	attrs[AttrCred] = models.ValueType{Kind: "SECRET", Value: "This is not JSON"}
	err = gcCSP.ValidateCredential(CSPDomainType, attrs)
	assert.Regexp("could not parse gc_cred", err)

	// success
	serviceAccount := `{ "type": "service_account", "project_id": "test-proj-1" }`
	attrs[AttrCred] = models.ValueType{Kind: "SECRET", Value: serviceAccount}
	err = gcCSP.ValidateCredential(CSPDomainType, attrs)
	assert.NoError(err)
}
