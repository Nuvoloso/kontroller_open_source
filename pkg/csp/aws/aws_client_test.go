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


package aws

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	"github.com/Nuvoloso/kontroller/pkg/awssdk"
	mockaws "github.com/Nuvoloso/kontroller/pkg/awssdk/mock"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

type awsFakeClientInitializer struct {
	err error
}

func (fci *awsFakeClientInitializer) clientInit(cl *Client) error {
	return fci.err
}

func TestClientInit(t *testing.T) {
	assert := assert.New(t)

	csp, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(csp)
	awsCSP, ok := csp.(*CSP)
	assert.True(ok)
	assert.Equal(awsCSP, awsCSP.clInit) // self-ref

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cl := mockaws.NewMockAWSClient(mockCtrl)
	awsCl := &Client{
		csp: awsCSP,
	}
	awsCl.client = cl

	// no attrs set
	err = awsCSP.clientInit(awsCl)
	assert.NotNil(err)
	assert.Regexp("required domain attributes missing", err)

	attrs := map[string]models.ValueType{
		AttrAccessKeyID:      models.ValueType{Kind: "STRING", Value: "key"},
		AttrSecretAccessKey:  models.ValueType{Kind: "SECRET", Value: "secret"},
		AttrRegion:           models.ValueType{Kind: "STRING", Value: "region"},
		AttrAvailabilityZone: models.ValueType{Kind: "STRING", Value: "zone"},
	}
	awsCl.attrs = attrs

	// Try invalid attribute types
	for k, a := range attrs {
		var nKind string
		switch a.Kind {
		case "STRING":
			nKind = "INT"
		case "SECRET":
			nKind = "STRING"
		default:
			nKind = "STRING"
		}
		p := attrs[k]
		attrs[k] = models.ValueType{Kind: nKind, Value: p.Value}
		err = awsCSP.clientInit(awsCl)
		assert.NotNil(err)
		assert.Regexp("required domain attributes missing or invalid", err)
		attrs[k] = p
	}

	// session creation failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	awsCl.client = cl
	awsCl.session = nil
	cfgM := newAWSClientMatcher(t, mockAWSClientConfig).SetAttrs(attrs)
	cl.EXPECT().NewSession(cfgM).Return(nil, fmt.Errorf("session failure"))
	err = awsCSP.clientInit(awsCl)
	assert.NotNil(err)
	assert.Regexp("session failure", err)

	// success
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	awsCl.client = cl
	awsCl.session = nil
	fakeSession := &session.Session{}
	cl.EXPECT().NewSession(cfgM).Return(fakeSession, nil)
	err = awsCSP.clientInit(awsCl)
	assert.Nil(err)
	assert.Equal(fakeSession, awsCl.session)

	// real Client calls
	dObj := &models.CSPDomain{
		CSPDomainAllOf0: models.CSPDomainAllOf0{
			Meta: &models.ObjMeta{
				ID: "CSP-DOMAIN-1",
			},
			CspDomainType: models.CspDomainTypeMutable(CSPDomainType),
			CspDomainAttributes: map[string]models.ValueType{
				AttrAccessKeyID:      models.ValueType{Kind: "STRING", Value: "key"},
				AttrSecretAccessKey:  models.ValueType{Kind: "SECRET", Value: "secret"},
				AttrRegion:           models.ValueType{Kind: "STRING", Value: "region"},
				AttrAvailabilityZone: models.ValueType{Kind: "STRING", Value: "zone"},
			},
		},
		CSPDomainMutable: models.CSPDomainMutable{},
	}

	// success
	fci := &awsFakeClientInitializer{}
	awsCSP.clInit = fci
	dc, err := awsCSP.Client(dObj)
	assert.Nil(err)
	awsCl, ok = dc.(*Client)
	assert.True(ok)
	assert.Equal(awsCSP, awsCl.csp)
	assert.Equal(time.Second*awsClientDefaultTimeoutSecs, awsCl.Timeout)
	assert.Equal(dObj.CspDomainAttributes, map[string]models.ValueType(awsCl.attrs))
	assert.Equal(dObj.Meta.ID, dc.ID())
	assert.Equal(dObj.CspDomainType, dc.Type())

	// success for object with no meta-data (e.g. called before object is created)
	meta := dObj.Meta
	dObj.Meta = nil
	dc, err = awsCSP.Client(dObj)
	assert.Nil(err)
	awsCl, ok = dc.(*Client)
	assert.True(ok)
	assert.Equal(awsCSP, awsCl.csp)
	assert.Equal(time.Second*awsClientDefaultTimeoutSecs, awsCl.Timeout)
	assert.Equal(dObj.CspDomainAttributes, map[string]models.ValueType(awsCl.attrs))
	assert.Equal(models.ObjID(""), dc.ID())
	assert.Equal(dObj.CspDomainType, dc.Type())
	dObj.Meta = meta

	// timeout can be set
	dc.SetTimeout(awsClientDefaultTimeoutSecs + 1)
	assert.EqualValues((awsClientDefaultTimeoutSecs+1)*time.Second, awsCl.Timeout)
	dc.SetTimeout(-1)
	assert.EqualValues(awsClientDefaultTimeoutSecs*time.Second, awsCl.Timeout)

	// init failed
	fci.err = fmt.Errorf("fci error")
	dc, err = awsCSP.Client(dObj)
	assert.Error(err)
	assert.Regexp("fci error", err)
	assert.Nil(dc)
	fci.err = nil

	// invalid domain obj
	dObj.CspDomainType = ""
	dc, err = awsCSP.Client(dObj)
	assert.Error(err)
	assert.Regexp("invalid CSPDomain", err)
	assert.Nil(dc)
}

func TestEc2Client(t *testing.T) {
	assert := assert.New(t)

	csp, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(csp)
	caws, ok := csp.(*CSP)
	assert.True(ok)
	assert.NotNil(caws)

	// allocates a new EC2 client on 1st call
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cl := mockaws.NewMockAWSClient(mockCtrl)
	fakeSession := &session.Session{}
	awsCl := &Client{
		csp: caws,
	}
	awsCl.client = cl
	awsCl.session = fakeSession
	awsCl.ec2 = nil
	mEC2 := mockaws.NewMockEC2(mockCtrl)
	cl.EXPECT().NewEC2(fakeSession).Return(mEC2)
	ec2client := awsCl.ec2Client()
	assert.Equal(mEC2, ec2client)
	assert.Equal(mEC2, awsCl.ec2)

	// 2nd call returns previously allocated client
	ec2client = awsCl.ec2Client()
	assert.Equal(mEC2, ec2client)
}

func TestValidate(t *testing.T) {
	assert := assert.New(t)
	zones := &ec2.DescribeAvailabilityZonesOutput{
		AvailabilityZones: []*ec2.AvailabilityZone{
			{ZoneName: aws.String("us-west-2a")},
			{ZoneName: aws.String("us-west-2b")},
			{ZoneName: aws.String("us-west-2c")},
		},
	}
	ctx := context.Background()

	csp, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(csp)
	caws, ok := csp.(*CSP)
	assert.True(ok)
	assert.NotNil(caws)
	awsCl := &Client{
		csp: caws,
	}
	awsCl.attrs = make(map[string]models.ValueType)
	awsCl.attrs[AttrRegion] = models.ValueType{Kind: "STRING", Value: endpoints.UsWest2RegionID}
	awsCl.attrs[AttrAvailabilityZone] = models.ValueType{Kind: "STRING", Value: "us-west-2b"}

	// success with context
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cl := mockaws.NewMockAWSClient(mockCtrl)
	fakeSession := &session.Session{}
	awsCl.client = cl
	awsCl.session = fakeSession
	mEC2 := mockaws.NewMockEC2(mockCtrl)
	awsCl.ec2 = mEC2
	mSTS := mockaws.NewMockSTS(mockCtrl)
	cl.EXPECT().NewSTS(fakeSession).Return(mSTS)
	mSTS.EXPECT().GetCallerIdentityWithContext(ctx, &sts.GetCallerIdentityInput{}).Return(nil, nil)
	mEC2.EXPECT().DescribeAvailabilityZonesWithContext(ctx, &ec2.DescribeAvailabilityZonesInput{}).Return(zones, nil)
	err = awsCl.Validate(ctx)
	assert.NoError(err)

	// identity fails (3 cases, generic, access_key, secret)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	awsCl.client = cl
	awsCl.session = fakeSession
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	awsCl.ec2 = mEC2
	mSTS = mockaws.NewMockSTS(mockCtrl)
	cl.EXPECT().NewSTS(fakeSession).Return(mSTS).Times(3)
	mSTS.EXPECT().GetCallerIdentityWithContext(ctx, &sts.GetCallerIdentityInput{}).Return(nil, fmt.Errorf("foo fail"))
	err = awsCl.Validate(ctx)
	assert.Regexp("identity validation failed.*foo fail", err)
	ae := awserr.New("InvalidClientTokenId", "long message", nil)
	rf := awserr.NewRequestFailure(ae, 403, "id1")
	mSTS.EXPECT().GetCallerIdentityWithContext(ctx, &sts.GetCallerIdentityInput{}).Return(nil, rf)
	err = awsCl.Validate(ctx)
	assert.Regexp("identity validation failed: .*aws_access_key_id$", err)
	ae = awserr.New("SignatureDoesNotMatch", "long message", nil)
	rf = awserr.NewRequestFailure(ae, 403, "id2")
	mSTS.EXPECT().GetCallerIdentityWithContext(ctx, &sts.GetCallerIdentityInput{}).Return(nil, rf)
	err = awsCl.Validate(ctx)
	assert.Regexp("identity validation failed: .*aws_secret_access_key$", err)

	// describe zones fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	awsCl.client = cl
	awsCl.session = fakeSession
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	awsCl.ec2 = mEC2
	mSTS = mockaws.NewMockSTS(mockCtrl)
	cl.EXPECT().NewSTS(fakeSession).Return(mSTS)
	mSTS.EXPECT().GetCallerIdentityWithContext(ctx, &sts.GetCallerIdentityInput{}).Return(nil, nil)
	mEC2.EXPECT().DescribeAvailabilityZonesWithContext(ctx, &ec2.DescribeAvailabilityZonesInput{}).Return(nil, fmt.Errorf("fail"))
	err = awsCl.Validate(ctx)
	assert.Regexp("list .*availability_zone.*failed", err)

	// invalid zone (pass nil context)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	awsCl.client = cl
	awsCl.session = fakeSession
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	awsCl.ec2 = mEC2
	mSTS = mockaws.NewMockSTS(mockCtrl)
	cl.EXPECT().NewSTS(fakeSession).Return(mSTS)
	mSTS.EXPECT().GetCallerIdentityWithContext(gomock.Not(gomock.Nil()), &sts.GetCallerIdentityInput{}).Return(nil, nil)
	mEC2.EXPECT().DescribeAvailabilityZonesWithContext(gomock.Not(gomock.Nil()), &ec2.DescribeAvailabilityZonesInput{}).Return(zones, nil)
	awsCl.attrs[AttrAvailabilityZone] = models.ValueType{Kind: "STRING", Value: "xyz"}
	err = awsCl.Validate(nil)
	assert.Regexp("invalid .*availability_zone", err)

	// invalid region
	awsCl.attrs[AttrRegion] = models.ValueType{Kind: "STRING", Value: "xyz"}
	err = awsCl.Validate(ctx)
	assert.Regexp("invalid aws_region", err)
}

func TestCreateProtectionStore(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()
	csp, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(csp)
	caws, ok := csp.(*CSP)
	assert.True(ok)
	assert.NotNil(caws)

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cl := mockaws.NewMockAWSClient(mockCtrl)
	fakeSession := &session.Session{}
	awsCl := &Client{
		csp: caws,
	}
	awsCl.client = cl
	awsCl.session = fakeSession
	awsCl.attrs = make(map[string]models.ValueType)
	awsCl.attrs[AttrRegion] = models.ValueType{Kind: "STRING", Value: endpoints.UsWest2RegionID}
	awsCl.attrs[AttrPStoreBucketName] = models.ValueType{Kind: "STRING", Value: "bucketname"}
	createBucketInput := &s3.CreateBucketInput{
		Bucket: aws.String("bucketname"),
		CreateBucketConfiguration: &s3.CreateBucketConfiguration{
			LocationConstraint: aws.String(endpoints.UsWest2RegionID),
		},
	}
	createBucketOutput := &s3.CreateBucketOutput{}
	awsCl.session = fakeSession
	mS3 := mockaws.NewMockS3(mockCtrl)
	cl.EXPECT().NewS3(fakeSession).Return(mS3)
	mS3.EXPECT().CreateBucketWithContext(ctx, createBucketInput).Return(createBucketOutput, nil)
	accessBlockInput := &s3.PutPublicAccessBlockInput{
		Bucket: aws.String("bucketname"),
		PublicAccessBlockConfiguration: &s3.PublicAccessBlockConfiguration{
			BlockPublicAcls:       aws.Bool(true),
			BlockPublicPolicy:     aws.Bool(true),
			IgnorePublicAcls:      aws.Bool(true),
			RestrictPublicBuckets: aws.Bool(true),
		},
	}
	accessBlockOutput := &s3.PutPublicAccessBlockOutput{}
	mS3.EXPECT().PutPublicAccessBlockWithContext(ctx, accessBlockInput).Return(accessBlockOutput, nil)
	attrMap, err := awsCl.CreateProtectionStore(ctx)
	assert.NoError(err)
	assert.Equal(awsCl.attrs, attrMap)

	// CreateBucket error cases, cover us-east-1 special case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	awsCl.client = cl
	awsCl.session = fakeSession
	awsCl.attrs[AttrRegion] = models.ValueType{Kind: "STRING", Value: endpoints.UsEast1RegionID}
	cbcSaved := createBucketInput.CreateBucketConfiguration
	createBucketInput.CreateBucketConfiguration = nil
	mS3 = mockaws.NewMockS3(mockCtrl)
	cl.EXPECT().NewS3(fakeSession).Return(mS3).Times(3)
	mS3.EXPECT().CreateBucketWithContext(ctx, createBucketInput).Return(nil, fmt.Errorf("foo fail"))
	attrMap, err = awsCl.CreateProtectionStore(ctx)
	assert.Regexp("protection store creation failed.*foo fail", err)
	assert.Nil(attrMap)
	ae := awserr.New("BucketAlreadyExists", "long message", nil)
	rf := awserr.NewRequestFailure(ae, 403, "id1")
	mS3.EXPECT().CreateBucketWithContext(ctx, createBucketInput).Return(nil, rf)
	attrMap, err = awsCl.CreateProtectionStore(ctx)
	assert.Regexp("protection store creation failed.*bucketname", err)
	assert.Nil(attrMap)
	ae = awserr.New("BucketAlreadyOwnedByYou", "long message", nil)
	rf = awserr.NewRequestFailure(ae, 403, "id1")
	mS3.EXPECT().CreateBucketWithContext(ctx, createBucketInput).Return(nil, rf)
	attrMap, err = awsCl.CreateProtectionStore(ctx)
	assert.Regexp("protection store creation failed.*bucketname", err)
	assert.Nil(attrMap)
	awsCl.attrs[AttrRegion] = models.ValueType{Kind: "STRING", Value: endpoints.UsWest2RegionID} // reset
	createBucketInput.CreateBucketConfiguration = cbcSaved

	// PublicAccessBlock error case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	awsCl.client = cl
	awsCl.session = fakeSession
	mS3 = mockaws.NewMockS3(mockCtrl)
	cl.EXPECT().NewS3(fakeSession).Return(mS3)
	mS3.EXPECT().CreateBucketWithContext(ctx, createBucketInput).Return(createBucketOutput, nil)
	ae = awserr.New("NoSuchBucket", "long message", nil)
	rf = awserr.NewRequestFailure(ae, 404, "id1")
	mS3.EXPECT().PutPublicAccessBlockWithContext(ctx, accessBlockInput).Return(nil, rf)
	attrMap, err = awsCl.CreateProtectionStore(ctx)
	assert.Regexp("protection store put public access policy failed: NoSuchBucket", err)
}

func TestValidateCredential(t *testing.T) {
	assert := assert.New(t)

	csp, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(csp)
	awsCSP, ok := csp.(*CSP)
	assert.True(ok)

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()

	// invalid DomainType
	err = awsCSP.ValidateCredential("BadDomainType", map[string]models.ValueType{})
	assert.NotNil(err)
	assert.Regexp("invalid DomainType", err)

	// no attrs
	err = awsCSP.ValidateCredential(CSPDomainType, map[string]models.ValueType{})
	assert.NotNil(err)
	assert.Regexp("required domain attributes missing or invalid", err)

	attrs := map[string]models.ValueType{
		AttrAccessKeyID:     models.ValueType{Kind: "STRING", Value: "key"},
		AttrSecretAccessKey: models.ValueType{Kind: "SECRET", Value: "secret"},
	}

	cl := mockaws.NewMockAWSClient(mockCtrl)

	// real session creation, failure in identity validation
	err = awsCSP.ValidateCredential(CSPDomainType, attrs)
	assert.NotNil(err)
	assert.Regexp("identity validation failed: incorrect aws_access_key_id", err)

	origGetNewClientHook := getNewClientHook
	getNewClientHook = func() awssdk.AWSClient {
		return cl
	}
	defer func() {
		getNewClientHook = origGetNewClientHook
	}()

	// session creation failure
	cfgM := newAWSClientMatcher(t, mockAWSClientConfig).SetAttrs(attrs)
	cl.EXPECT().NewSession(cfgM).Return(nil, fmt.Errorf("session failure"))
	err = awsCSP.ValidateCredential(CSPDomainType, attrs)
	assert.NotNil(err)
	assert.Regexp("session failure", err)

	// failure in GetCallerIdentityWithContext
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	fakeSession := &session.Session{}
	cfgM = newAWSClientMatcher(t, mockAWSClientConfig).SetAttrs(attrs)
	cl.EXPECT().NewSession(cfgM).Return(fakeSession, nil).Times(3)
	mSTS := mockaws.NewMockSTS(mockCtrl)
	cl.EXPECT().NewSTS(fakeSession).Return(mSTS).Times(3)
	mSTS.EXPECT().GetCallerIdentityWithContext(gomock.Any(), &sts.GetCallerIdentityInput{}).Return(nil, fmt.Errorf("foo fail"))
	err = awsCSP.ValidateCredential(CSPDomainType, attrs)
	assert.NotNil(err)
	assert.Regexp("identity validation failed: foo fail", err)

	ae := awserr.New("InvalidClientTokenId", "long message", nil)
	rf := awserr.NewRequestFailure(ae, 403, "id1")
	mSTS.EXPECT().GetCallerIdentityWithContext(gomock.Any(), &sts.GetCallerIdentityInput{}).Return(nil, rf)
	err = awsCSP.ValidateCredential(CSPDomainType, attrs)
	assert.NotNil(err)
	assert.Regexp("identity validation failed: .*aws_access_key_id$", err)

	ae = awserr.New("SignatureDoesNotMatch", "long message", nil)
	rf = awserr.NewRequestFailure(ae, 403, "id2")
	mSTS.EXPECT().GetCallerIdentityWithContext(gomock.Any(), &sts.GetCallerIdentityInput{}).Return(nil, rf)
	err = awsCSP.ValidateCredential(CSPDomainType, attrs)
	assert.NotNil(err)
	assert.Regexp("identity validation failed: .*aws_secret_access_key$", err)

	// success
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	cfgM = newAWSClientMatcher(t, mockAWSClientConfig).SetAttrs(attrs)
	cl.EXPECT().NewSession(cfgM).Return(fakeSession, nil).Times(1)
	mSTS = mockaws.NewMockSTS(mockCtrl)
	cl.EXPECT().NewSTS(fakeSession).Return(mSTS).Times(1)
	mSTS.EXPECT().GetCallerIdentityWithContext(gomock.Any(), &sts.GetCallerIdentityInput{}).Return(nil, nil)
	err = awsCSP.ValidateCredential(CSPDomainType, attrs)
	assert.Nil(err)
}

// gomock.Matcher for AWSClient
type mockAWSClientMatcherEnum int

const (
	mockAWSClientInvalid mockAWSClientMatcherEnum = iota
	mockAWSClientConfig
)

type mockAWSClientMatcher struct {
	t     *testing.T
	ctx   mockAWSClientMatcherEnum
	attrs map[string]models.ValueType
}

var _ = gomock.Matcher(&mockAWSClientMatcher{})

func newAWSClientMatcher(t *testing.T, ctx mockAWSClientMatcherEnum) *mockAWSClientMatcher {
	return &mockAWSClientMatcher{t: t, ctx: ctx}
}

func (m *mockAWSClientMatcher) SetAttrs(attrs map[string]models.ValueType) *mockAWSClientMatcher {
	m.attrs = attrs
	return m
}

func (m *mockAWSClientMatcher) Matches(x interface{}) bool {
	assert := assert.New(m.t)
	switch m.ctx {
	case mockAWSClientConfig:
		cfg, ok := x.(*aws.Config)
		assert.True(ok)
		cV, err := cfg.Credentials.Get()
		rc := assert.Nil(err) &&
			assert.False(cfg.Credentials.IsExpired()) &&
			assert.Equal(m.attrs[AttrAccessKeyID].Value, cV.AccessKeyID) &&
			assert.Equal(m.attrs[AttrSecretAccessKey].Value, cV.SecretAccessKey) &&
			assert.Equal(awsCredsProviderName, cV.ProviderName) &&
			assert.Equal(m.attrs[AttrRegion].Value, aws.StringValue(cfg.Region))
		return rc
	}
	return false
}

func (m *mockAWSClientMatcher) String() string {
	switch m.ctx {
	case mockAWSClientConfig:
		return "matches aws.Config"
	}
	return "unknown context"
}
