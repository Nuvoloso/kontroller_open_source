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
	"net/http"
	"testing"

	"github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mockaws "github.com/Nuvoloso/kontroller/pkg/awssdk/mock"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	"github.com/alecthomas/units"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/go-openapi/swag"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestAwsEC2VolumeConverter(t *testing.T) {
	assert := assert.New(t)

	volID := "vol-myVol"
	expVol := &csp.Volume{
		CSPDomainType:     CSPDomainType,
		StorageTypeName:   EC2VolTypeToCSPStorageType("gp2"),
		Identifier:        VolumeIdentifierCreate(ServiceEC2, volID),
		Type:              "gp2",
		SizeBytes:         20 * int64(units.GiB),
		ProvisioningState: csp.VolumeProvisioningProvisioned,
		Tags:              []string{"n1:v1", "n2:v2"},
		Attachments: []csp.VolumeAttachment{
			{
				NodeIdentifier: "node1",
				Device:         "/dev/block",
				State:          csp.VolumeAttachmentAttached,
			},
		},
	}
	ec2vol := &ec2.Volume{
		VolumeId:   &volID,
		VolumeType: &expVol.Type,
		Size:       aws.Int64(20),
		State:      aws.String(ec2.VolumeStateInUse),
		Attachments: []*ec2.VolumeAttachment{
			&ec2.VolumeAttachment{
				InstanceId: &expVol.Attachments[0].NodeIdentifier,
				Device:     &expVol.Attachments[0].Device,
				State:      aws.String(ec2.AttachmentStatusAttached),
			},
		},
		Tags: []*ec2.Tag{
			&ec2.Tag{
				Key:   aws.String("n1"),
				Value: aws.String("v1"),
			},
			&ec2.Tag{
				Key:   aws.String("n2"),
				Value: aws.String("v2"),
			},
		},
	}

	vol := awsEC2VolumeToVolume(ec2vol)
	expVol.Raw = ec2vol
	assert.Equal(expVol, vol)

	// vary the State
	ec2States := []struct {
		ec2State string
		vps      csp.VolumeProvisioningState
	}{
		{ec2.VolumeStateCreating, csp.VolumeProvisioningProvisioning},
		{ec2.VolumeStateAvailable, csp.VolumeProvisioningProvisioned},
		{ec2.VolumeStateInUse, csp.VolumeProvisioningProvisioned},
		{ec2.VolumeStateDeleting, csp.VolumeProvisioningUnprovisioning},
		{ec2.VolumeStateDeleted, csp.VolumeProvisioningUnprovisioned},
	}
	for _, tc := range ec2States {
		ec2vol.State = aws.String(tc.ec2State)
		expVol.ProvisioningState = tc.vps
		vol := awsEC2VolumeToVolume(ec2vol)
		expVol.Raw = ec2vol
		assert.Equal(expVol, vol)
	}

	// vary the Attachment status
	ec2AttStatus := []struct {
		ec2AttState string
		vas         csp.VolumeAttachmentState
	}{
		{ec2.AttachmentStatusAttaching, csp.VolumeAttachmentAttaching},
		{ec2.AttachmentStatusAttached, csp.VolumeAttachmentAttached},
		{ec2.AttachmentStatusDetaching, csp.VolumeAttachmentDetaching},
		{ec2.AttachmentStatusDetached, csp.VolumeAttachmentDetached},
	}
	for _, tc := range ec2AttStatus {
		ec2vol.Attachments[0].State = aws.String(tc.ec2AttState)
		expVol.Attachments[0].State = tc.vas
		vol := awsEC2VolumeToVolume(ec2vol)
		expVol.Raw = ec2vol
		assert.Equal(expVol, vol)
	}
}

func TestAwsIdentifierParser(t *testing.T) {
	assert := assert.New(t)

	// test the identifer parser
	s0, s1, err := VolumeIdentifierParse(ServiceEC2 + ":something")
	assert.NoError(err)
	assert.Equal(ServiceEC2, s0)
	assert.Equal("something", s1)
	s0, s1, err = VolumeIdentifierParse(ServiceS3 + ":else:")
	assert.NoError(err)
	assert.Equal(ServiceS3, s0)
	assert.Equal("else:", s1)
	s0, s1, err = VolumeIdentifierParse("foobar")
	assert.Regexp("invalid volume identifier", err)
	s0, s1, err = VolumeIdentifierParse(ServiceEC2)
	assert.NoError(err)
	assert.Equal(ServiceEC2, s0)
	assert.Equal("", s1)
}

func TestAwsEC2TagsConverter(t *testing.T) {
	assert := assert.New(t)

	// case: typical usage
	tags := []string{
		"n1:v1",
		"some/tag/key:tag:value",
		"tag/noValue:",
	}
	ec2Tags := []*ec2.Tag{
		&ec2.Tag{
			Key:   aws.String("n1"),
			Value: aws.String("v1"),
		},
		&ec2.Tag{
			Key:   aws.String("some/tag/key"),
			Value: aws.String("tag:value"),
		},
		&ec2.Tag{
			Key:   aws.String("tag/noValue"),
			Value: aws.String(""),
		},
	}
	et := awsEC2TagsFromModel(tags)
	assert.NotNil(et)
	assert.EqualValues(ec2Tags, et)

	mt := awsEC2TagsToModel(ec2Tags)
	assert.NotNil(mt)
	assert.EqualValues(tags, mt)

	// case: specifying model tag keys with no values (typically for deletion)
	tagKeyOnly := []string{"tagKeyOnly", "nuvoloso-storage-id"}
	ec2TagKeyOnly := []*ec2.Tag{
		&ec2.Tag{
			Key: aws.String("tagKeyOnly"),
		},
		&ec2.Tag{
			Key: aws.String("nuvoloso-storage-id"),
		},
	}
	et = awsEC2TagsFromModel(tagKeyOnly)
	assert.NotNil(et)
	assert.EqualValues(ec2TagKeyOnly, et)

	// case: well known storage id tag is converted, but also causes a Name tag to be added
	tags = []string{
		"nuvoloso-storage-id:1-2-3",
	}
	ec2Tags = []*ec2.Tag{
		&ec2.Tag{
			Key:   aws.String("nuvoloso-storage-id"),
			Value: aws.String("1-2-3"),
		},
		&ec2.Tag{
			Key:   aws.String("Name"),
			Value: aws.String("nuvoloso-1-2-3"),
		},
	}
	et = awsEC2TagsFromModel(tags)
	assert.NotNil(et)
	assert.Equal(ec2Tags, et)

	// case: if Name tag is present, it is not changed
	tags = []string{
		"nuvoloso-storage-id:1-2-3",
		"Name:mine",
	}
	ec2Tags = []*ec2.Tag{
		&ec2.Tag{
			Key:   aws.String("nuvoloso-storage-id"),
			Value: aws.String("1-2-3"),
		},
		&ec2.Tag{
			Key:   aws.String("Name"),
			Value: aws.String("mine"),
		},
	}
	et = awsEC2TagsFromModel(tags)
	assert.NotNil(et)
	assert.Equal(ec2Tags, et)

	// case: EC2 tags with no value (typical importing externally set tags)
	ec2TagNoValue := []*ec2.Tag{
		&ec2.Tag{
			Key: aws.String("tagWithNoValue"),
		},
		&ec2.Tag{
			Key: aws.String("tagWithNoValue:ButWithColon"),
		},
		&ec2.Tag{
			Key:   aws.String("tagWithEmptyString"),
			Value: aws.String(""),
		},
	}
	tagNoValue := []string{
		"tagWithNoValue",
		"tagWithNoValue:ButWithColon", // won't survive a write back
		"tagWithEmptyString:",
	}
	mt = awsEC2TagsToModel(ec2TagNoValue)
	assert.NotNil(mt)
	assert.EqualValues(tagNoValue, mt)
}

func TestAwsEC2VolumeFetch(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()

	volID := "vol-myVol"
	expVol := &csp.Volume{
		CSPDomainType:     CSPDomainType,
		StorageTypeName:   EC2VolTypeToCSPStorageType("gp2"),
		Identifier:        VolumeIdentifierCreate(ServiceEC2, volID),
		Type:              "gp2",
		SizeBytes:         20 * int64(units.GiB),
		ProvisioningState: csp.VolumeProvisioningProvisioned,
		Tags:              []string{"n:v"},
		Attachments: []csp.VolumeAttachment{
			{
				NodeIdentifier: "node1",
				Device:         "/dev/block",
				State:          csp.VolumeAttachmentAttached,
			},
		},
	}

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cl := mockaws.NewMockAWSClient(mockCtrl)
	fakeSession := &session.Session{}
	awsCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(awsCSP)
	caws, ok := awsCSP.(*CSP)
	assert.True(ok)
	awsCl := &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
	}
	mEC2 := mockaws.NewMockEC2(mockCtrl)
	cl.EXPECT().NewEC2(fakeSession).Return(mEC2)
	dvi := &ec2.DescribeVolumesInput{
		VolumeIds: []*string{&volID},
	}
	dvo := &ec2.DescribeVolumesOutput{
		Volumes: []*ec2.Volume{
			&ec2.Volume{
				VolumeId:   &volID,
				VolumeType: &expVol.Type,
				Size:       aws.Int64(20),
				State:      aws.String(ec2.VolumeStateInUse),
				Attachments: []*ec2.VolumeAttachment{
					&ec2.VolumeAttachment{
						InstanceId: &expVol.Attachments[0].NodeIdentifier,
						Device:     &expVol.Attachments[0].Device,
						State:      aws.String(ec2.AttachmentStatusAttached),
					},
				},
				Tags: []*ec2.Tag{
					&ec2.Tag{
						Key:   aws.String("n"),
						Value: aws.String("v"),
					},
				},
			},
		},
	}
	expVol.Raw = dvo.Volumes[0]
	mEC2.EXPECT().DescribeVolumesWithContext(ctx, dvi).Return(dvo, nil)
	assert.Nil(awsCl.ec2)
	vfa := &csp.VolumeFetchArgs{}
	vfa.VolumeIdentifier = expVol.Identifier
	vol, err := awsCl.VolumeFetch(ctx, vfa)
	assert.NoError(err)
	assert.Equal(expVol, vol)
	assert.Equal(awsCl.ec2, mEC2)

	// repeat call should use the cached client; pass a nil context
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	mEC2.EXPECT().DescribeVolumesWithContext(gomock.Not(gomock.Nil()), dvi).Return(dvo, nil)
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	vol, err = awsCl.VolumeFetch(nil, vfa)
	assert.NoError(err)
	assert.Equal(expVol, vol)
	assert.Equal(awsCl.ec2, mEC2)

	// DescribeVolumes error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	mEC2.EXPECT().DescribeVolumesWithContext(ctx, dvi).Return(nil, fmt.Errorf("describeVolumes error"))
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	vol, err = awsCl.VolumeFetch(ctx, vfa)
	assert.Regexp("describeVolumes error", err)
	assert.Nil(vol)

	// invalid service
	vfa.VolumeIdentifier = "foo:id"
	vol, err = awsCl.VolumeFetch(ctx, vfa)
	assert.Regexp("invalid volumeIdentifier", err)
	assert.Nil(vol)
}

func TestAwsEC2VolumeList(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()

	volID1 := "vol-myVol1"
	volID2 := "vol-myVol2"
	resVolumes := []*csp.Volume{
		&csp.Volume{
			CSPDomainType:     CSPDomainType,
			StorageTypeName:   EC2VolTypeToCSPStorageType("gp2"),
			Identifier:        VolumeIdentifierCreate(ServiceEC2, volID1),
			Type:              "gp2",
			SizeBytes:         20 * int64(units.GiB),
			ProvisioningState: csp.VolumeProvisioningProvisioned,
			Tags:              []string{"n1:v1"},
			Attachments: []csp.VolumeAttachment{
				{
					NodeIdentifier: "node1",
					Device:         "/dev/block1",
					State:          csp.VolumeAttachmentAttached,
				},
			},
		},
		&csp.Volume{
			CSPDomainType:     CSPDomainType,
			StorageTypeName:   EC2VolTypeToCSPStorageType("gp2"),
			Identifier:        VolumeIdentifierCreate(ServiceEC2, volID2),
			Type:              "gp2",
			SizeBytes:         10 * int64(units.GiB),
			ProvisioningState: csp.VolumeProvisioningProvisioned,
			Tags:              []string{"n1:v2"},
			Attachments: []csp.VolumeAttachment{
				{
					NodeIdentifier: "node1",
					Device:         "/dev/block2",
					State:          csp.VolumeAttachmentAttached,
				},
			},
		},
	}

	// success case with filter arguments
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cl := mockaws.NewMockAWSClient(mockCtrl)
	fakeSession := &session.Session{}
	awsCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(awsCSP)
	caws, ok := awsCSP.(*CSP)
	assert.True(ok)
	awsCl := &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
	}
	mEC2 := mockaws.NewMockEC2(mockCtrl)
	cl.EXPECT().NewEC2(fakeSession).Return(mEC2)
	vla := &csp.VolumeListArgs{
		StorageTypeName: EC2VolTypeToCSPStorageType("gp2"),
		Tags:            []string{"n1:has:in-value", "n1"},
	}
	dvi := &ec2.DescribeVolumesInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name:   aws.String("volume-type"),
				Values: []*string{aws.String(string("gp2"))},
			},
			&ec2.Filter{
				Name:   aws.String("tag:n1"),
				Values: []*string{aws.String("has:in-value")},
			},
			&ec2.Filter{
				Name:   aws.String("tag-key"),
				Values: []*string{aws.String("n1")},
			},
		},
	}
	dvo := &ec2.DescribeVolumesOutput{
		Volumes: []*ec2.Volume{
			&ec2.Volume{
				VolumeId:   &volID1,
				VolumeType: &resVolumes[0].Type,
				Size:       aws.Int64(20),
				State:      aws.String(ec2.VolumeStateInUse),
				Attachments: []*ec2.VolumeAttachment{
					&ec2.VolumeAttachment{
						InstanceId: &resVolumes[0].Attachments[0].NodeIdentifier,
						Device:     &resVolumes[0].Attachments[0].Device,
						State:      aws.String(ec2.AttachmentStatusAttached),
					},
				},
				Tags: []*ec2.Tag{
					&ec2.Tag{
						Key:   aws.String("n1"),
						Value: aws.String("v1"),
					},
				},
			},
			&ec2.Volume{
				VolumeId:   &volID2,
				VolumeType: &resVolumes[1].Type,
				Size:       aws.Int64(10),
				State:      aws.String(ec2.VolumeStateInUse),
				Attachments: []*ec2.VolumeAttachment{
					&ec2.VolumeAttachment{
						InstanceId: &resVolumes[1].Attachments[0].NodeIdentifier,
						Device:     &resVolumes[1].Attachments[0].Device,
						State:      aws.String(ec2.AttachmentStatusAttached),
					},
				},
				Tags: []*ec2.Tag{
					&ec2.Tag{
						Key:   aws.String("n1"),
						Value: aws.String("v2"),
					},
				},
			}},
	}
	resVolumes[0].Raw = dvo.Volumes[0]
	resVolumes[1].Raw = dvo.Volumes[1]
	mEC2.EXPECT().DescribeVolumesWithContext(ctx, gomock.Eq(dvi)).Return(dvo, nil)
	assert.Nil(awsCl.ec2)
	vols, err := awsCl.VolumeList(ctx, vla)
	assert.NoError(err)
	assert.Equal(resVolumes, vols)
	assert.Equal(awsCl.ec2, mEC2)

	// repeat call should use the cached client; pass a nil context
	// no filters, api error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	vla = &csp.VolumeListArgs{}
	dvi = &ec2.DescribeVolumesInput{}
	mEC2.EXPECT().DescribeVolumesWithContext(gomock.Not(gomock.Nil()), gomock.Eq(dvi)).Return(nil, fmt.Errorf("api error"))
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	vols, err = awsCl.VolumeList(nil, vla)
	assert.Nil(vols)
	assert.Regexp("api error", err)

	// DescribeVolumes error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	vla = &csp.VolumeListArgs{}
	dvi = &ec2.DescribeVolumesInput{}
	mEC2.EXPECT().DescribeVolumesWithContext(ctx, gomock.Eq(dvi)).Return(nil, fmt.Errorf("describeVolumes error"))
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	vols, err = awsCl.VolumeList(ctx, vla)
	assert.Regexp("describeVolumes error", err)
	assert.Nil(vols)

	// invalid storage type in filter
	vla.StorageTypeName = "invalid storage type"
	svc, st, obj := StorageTypeToServiceVolumeType(vla.StorageTypeName)
	assert.Empty(svc)
	assert.Empty(st)
	assert.Nil(obj)
	vols, err = awsCl.VolumeList(ctx, vla)
	assert.Regexp("invalid storage type", err)
	assert.Nil(vols)

	// valid storage type, but wrong service
	vla.StorageTypeName = "Amazon SSD Instance Store"
	svc, st, obj = StorageTypeToServiceVolumeType(vla.StorageTypeName)
	assert.Empty(svc)
	assert.Empty(st)
	assert.NotNil(obj)
	vols, err = awsCl.VolumeList(ctx, vla)
	assert.Regexp("invalid storage type", err)
	assert.Nil(vols)
}

func TestAwsVolumeCreate(t *testing.T) {
	assert := assert.New(t)

	// insert fake storage types to force failure
	numST := len(awsCspStorageTypes)
	cspSTFooService := models.CspStorageType("Aws Fake ST")
	fooServiceST := &models.CSPStorageType{
		CspDomainType:          CSPDomainType,
		Description:            "Fake ST",
		Name:                   cspSTFooService,
		MinAllocationSizeBytes: swag.Int64(int64(4 * units.Gibibyte)),
		MaxAllocationSizeBytes: swag.Int64(int64(16 * units.Tebibyte)),
		AccessibilityScope:     "CSPDOMAIN",
		CspStorageTypeAttributes: map[string]models.ValueType{
			PAService:       models.ValueType{Kind: "STRING", Value: "FooService"},
			PAEC2VolumeType: models.ValueType{Kind: "STRING", Value: "blah"},
		},
	}
	awsCspStorageTypes = append(awsCspStorageTypes, fooServiceST)
	cspSTNoService := models.CspStorageType("Aws no svc ST")
	noServiceST := &models.CSPStorageType{
		CspDomainType:          CSPDomainType,
		Description:            "not dynamically provisionable storage",
		Name:                   cspSTNoService,
		MinAllocationSizeBytes: swag.Int64(int64(4 * units.Gibibyte)),
		MaxAllocationSizeBytes: swag.Int64(int64(16 * units.Tebibyte)),
		AccessibilityScope:     "CSPDOMAIN",
		CspStorageTypeAttributes: map[string]models.ValueType{
			PAEC2VolumeType: models.ValueType{Kind: "STRING", Value: "blah"},
		},
	}
	awsCspStorageTypes = append(awsCspStorageTypes, noServiceST)
	defer func() {
		awsCspStorageTypes = awsCspStorageTypes[:numST]
		awsCreateSTMap()
	}()
	awsCreateSTMap()

	// unsupported storage type
	vca := &csp.VolumeCreateArgs{
		StorageTypeName: cspSTFooService,
	}
	awsCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(awsCSP)
	_, ok := awsCSP.(*CSP)
	assert.True(ok)
	caws, ok := awsCSP.(*CSP)
	assert.True(ok)
	awsCl := &Client{
		csp: caws,
	}
	vol, err := awsCl.VolumeCreate(nil, vca)
	assert.Regexp("storage type currently unsupported", err)
	assert.Nil(vol)

	// not dynamically provisionable storage type
	vca.StorageTypeName = cspSTNoService
	vol, err = awsCl.VolumeCreate(nil, vca)
	assert.Regexp("storage type is not dynamically provisionable", err)
	assert.Nil(vol)

	// invalid storage type
	vca.StorageTypeName = "fooBarType"
	vol, err = awsCl.VolumeCreate(nil, vca)
	assert.Regexp("invalid storage type for CSPDomain", err)
	assert.Nil(vol)
}

func TestAwsVolumeTagSet(t *testing.T) {
	assert := assert.New(t)

	awsCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(awsCSP)
	_, ok := awsCSP.(*CSP)
	assert.True(ok)
	caws, ok := awsCSP.(*CSP)
	assert.True(ok)
	awsCl := &Client{
		csp: caws,
	}

	// invoke with missing tags
	vta := &csp.VolumeTagArgs{}
	assert.True(vta.Tags == nil)
	vol, err := awsCl.VolumeTagsSet(nil, vta)
	assert.Regexp("no tags specified", err)
	assert.Nil(vol)

	// invoke with empty tags
	vta.Tags = []string{}
	assert.True(vta.Tags != nil)
	assert.Len(vta.Tags, 0)
	vol, err = awsCl.VolumeTagsSet(nil, vta)
	assert.Regexp("no tags specified", err)
	assert.Nil(vol)

	// invalid volume Id
	vta.Tags = []string{"foo:bar"}
	vta.VolumeIdentifier = "foo:vol-id"
	vol, err = awsCl.VolumeTagsSet(nil, vta)
	assert.Regexp("invalid volume identifier", err)
	assert.Nil(vol)
}

func TestAwsVolumeTagDelete(t *testing.T) {
	assert := assert.New(t)

	awsCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(awsCSP)
	_, ok := awsCSP.(*CSP)
	assert.True(ok)
	caws, ok := awsCSP.(*CSP)
	assert.True(ok)
	awsCl := &Client{
		csp: caws,
	}

	// invoke with missing tags
	vta := &csp.VolumeTagArgs{}
	assert.True(vta.Tags == nil)
	vol, err := awsCl.VolumeTagsDelete(nil, vta)
	assert.Regexp("no tags specified", err)
	assert.Nil(vol)

	// invoke with empty tags
	vta.Tags = []string{}
	assert.True(vta.Tags != nil)
	assert.Len(vta.Tags, 0)
	vol, err = awsCl.VolumeTagsDelete(nil, vta)
	assert.Regexp("no tags specified", err)
	assert.Nil(vol)

	// invalid volume Id
	vta.Tags = []string{"foo:bar"}
	vta.VolumeIdentifier = "foo:vol-id"
	vol, err = awsCl.VolumeTagsDelete(nil, vta)
	assert.Regexp("invalid volume identifier", err)
	assert.Nil(vol)
}

func TestAwsVolumeSize(t *testing.T) {
	assert := assert.New(t)

	// insert fake storage types to force failure
	numST := len(awsCspStorageTypes)
	cspSTFooService := models.CspStorageType("Aws Fake ST")
	fooServiceST := &models.CSPStorageType{
		CspDomainType:          CSPDomainType,
		Description:            "Fake ST",
		Name:                   cspSTFooService,
		MinAllocationSizeBytes: swag.Int64(int64(4 * units.Gibibyte)),
		MaxAllocationSizeBytes: swag.Int64(int64(16 * units.Tebibyte)),
		AccessibilityScope:     "CSPDOMAIN",
		CspStorageTypeAttributes: map[string]models.ValueType{
			PAService:       models.ValueType{Kind: "STRING", Value: "FooService"},
			PAEC2VolumeType: models.ValueType{Kind: "STRING", Value: "blah"},
		},
	}
	awsCspStorageTypes = append(awsCspStorageTypes, fooServiceST)
	cspSTNoService := models.CspStorageType("Aws no svc ST")
	noServiceST := &models.CSPStorageType{
		CspDomainType:          CSPDomainType,
		Description:            "not dynamically provisionable storage",
		Name:                   cspSTNoService,
		MinAllocationSizeBytes: swag.Int64(int64(4 * units.Gibibyte)),
		MaxAllocationSizeBytes: swag.Int64(int64(16 * units.Tebibyte)),
		AccessibilityScope:     "CSPDOMAIN",
		CspStorageTypeAttributes: map[string]models.ValueType{
			PAEC2VolumeType: models.ValueType{Kind: "STRING", Value: "blah"},
		},
	}
	awsCspStorageTypes = append(awsCspStorageTypes, noServiceST)
	defer func() {
		awsCspStorageTypes = awsCspStorageTypes[:numST]
		awsCreateSTMap()
	}()
	awsCreateSTMap()

	awsCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(awsCSP)
	_, ok := awsCSP.(*CSP)
	assert.True(ok)
	caws, ok := awsCSP.(*CSP)
	assert.True(ok)
	awsCl := &Client{
		csp: caws,
	}

	// unsupported storage type
	sizeBytes, err := awsCl.VolumeSize(nil, cspSTFooService, 1)
	assert.Regexp("storage type currently unsupported", err)
	assert.Zero(sizeBytes)

	// not dynamically provisionable storage type
	sizeBytes, err = awsCl.VolumeSize(nil, cspSTNoService, 1)
	assert.Regexp("storage type is not dynamically provisionable", err)
	assert.Zero(sizeBytes)

	// invalid storage type
	sizeBytes, err = awsCl.VolumeSize(nil, "fooBarType", 1)
	assert.Regexp("invalid storage type for CSPDomain", err)
	assert.Zero(sizeBytes)

	// invalid sizes
	sizeBytes, err = awsCl.VolumeSize(nil, "foo", -1)
	assert.Regexp("invalid allocation size", err)
	assert.Zero(sizeBytes)
	sizeBytes, err = awsCl.VolumeSize(nil, fooServiceST.Name, *fooServiceST.MaxAllocationSizeBytes+1)
	assert.Regexp("requested size exceeds the storage type maximum", err)
	assert.Zero(sizeBytes)
}

func TestAwsEC2VolumeAttach(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()

	nid := "n-1"
	vid := "v-1"
	prefix := "/dev/xvd"
	devicePath := prefix + "ba"
	vaa := &csp.VolumeAttachArgs{
		VolumeIdentifier: VolumeIdentifierCreate(ServiceEC2, vid),
		NodeIdentifier:   nid,
	}
	avi := &ec2.AttachVolumeInput{
		Device:     aws.String(devicePath),
		InstanceId: aws.String(nid),
		VolumeId:   aws.String(vid),
	}
	dii := &ec2.DescribeInstancesInput{InstanceIds: []*string{&nid}}
	iObj := &ec2.Instance{
		InstanceId:          &nid,
		BlockDeviceMappings: []*ec2.InstanceBlockDeviceMapping{},
	}
	dio := &ec2.DescribeInstancesOutput{
		Reservations: []*ec2.Reservation{
			{Instances: []*ec2.Instance{iObj}},
		},
	}
	dvi := &ec2.DescribeVolumesInput{VolumeIds: []*string{&vid}}
	vObj := &csp.Volume{
		CSPDomainType:     CSPDomainType,
		StorageTypeName:   EC2VolTypeToCSPStorageType("gp2"),
		Identifier:        VolumeIdentifierCreate(ServiceEC2, vid),
		Type:              "gp2",
		SizeBytes:         20 * int64(units.GiB),
		ProvisioningState: csp.VolumeProvisioningProvisioned,
		Tags:              []string{"n:v"},
		Attachments: []csp.VolumeAttachment{
			{
				NodeIdentifier: nid,
				Device:         devicePath,
				State:          csp.VolumeAttachmentAttached,
			},
		},
	}
	evObj := &ec2.Volume{
		VolumeId:   &vid,
		VolumeType: &vObj.Type,
		Size:       aws.Int64(20),
		State:      aws.String(ec2.VolumeStateInUse),
		Attachments: []*ec2.VolumeAttachment{
			&ec2.VolumeAttachment{
				InstanceId: &vObj.Attachments[0].NodeIdentifier,
				Device:     &vObj.Attachments[0].Device,
				State:      aws.String(ec2.AttachmentStatusAttached),
			},
		},
		Tags: []*ec2.Tag{
			&ec2.Tag{
				Key:   aws.String("n"),
				Value: aws.String("v"),
			},
		},
	}
	vObj.Raw = evObj
	dvo := &ec2.DescribeVolumesOutput{Volumes: []*ec2.Volume{evObj}}
	awsCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(awsCSP)
	caws, ok := awsCSP.(*CSP)
	assert.True(ok)

	// success with context, new attachment
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cl := mockaws.NewMockAWSClient(mockCtrl)
	fakeSession := &session.Session{}
	mEC2 := mockaws.NewMockEC2(mockCtrl)
	awsCl := &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	mEC2.EXPECT().DescribeInstancesWithContext(ctx, dii).Return(dio, nil)
	mEC2.EXPECT().AttachVolumeWithContext(ctx, avi).Return(nil, nil)
	rwM := newWaiterMatcher(t).WithExpected(ec2.AttachmentStatusAttached)
	mW := mockaws.NewMockWaiter(mockCtrl)
	cl.EXPECT().NewWaiter(rwM).Return(mW)
	mW.EXPECT().WaitWithContext(ctx).Return(nil)
	mEC2.EXPECT().DescribeVolumesWithContext(ctx, dvi).Return(dvo, nil)
	vol, err := awsCl.VolumeAttach(ctx, vaa)
	assert.Equal(vObj, vol)
	assert.NoError(err)

	// success, already attaching
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	devicePath = prefix + "bx"
	iObj.BlockDeviceMappings = []*ec2.InstanceBlockDeviceMapping{
		{
			DeviceName: aws.String(devicePath),
			Ebs: &ec2.EbsInstanceBlockDevice{
				VolumeId: aws.String(vid),
			},
		},
	}
	vObj.Attachments[0].Device = devicePath
	cl = mockaws.NewMockAWSClient(mockCtrl)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	mEC2.EXPECT().DescribeInstancesWithContext(ctx, dii).Return(dio, nil)
	mW = mockaws.NewMockWaiter(mockCtrl)
	cl.EXPECT().NewWaiter(rwM).Return(mW)
	mW.EXPECT().WaitWithContext(ctx).Return(nil)
	mEC2.EXPECT().DescribeVolumesWithContext(ctx, dvi).Return(dvo, nil)
	vol, err = awsCl.VolumeAttach(ctx, vaa)
	assert.Equal(vObj, vol)
	assert.NoError(err)
	assert.Empty(attachingNodeDevices)

	// fail looking up instance for device path
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	devicePath = prefix + "ba"
	iObj.BlockDeviceMappings = []*ec2.InstanceBlockDeviceMapping{}
	vObj.Attachments[0].Device = devicePath
	cl = mockaws.NewMockAWSClient(mockCtrl)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	expErr := fmt.Errorf("instance lookup failure")
	mEC2.EXPECT().DescribeInstancesWithContext(ctx, dii).Return(nil, expErr)
	vol, err = awsCl.awsEC2VolumeAttach(ctx, vaa, vid)
	assert.Nil(vol)
	assert.Equal(expErr, err)

	// fail during attach
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	mEC2.EXPECT().DescribeInstancesWithContext(ctx, dii).Return(dio, nil)
	expErr = fmt.Errorf("Attach failure")
	mEC2.EXPECT().AttachVolumeWithContext(ctx, avi).Return(nil, expErr)
	vol, err = awsCl.awsEC2VolumeAttach(ctx, vaa, vid)
	assert.Nil(vol)
	assert.Equal(expErr, err)
	assert.Empty(attachingNodeDevices)

	// fail during wait
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	mEC2.EXPECT().DescribeInstancesWithContext(ctx, dii).Return(dio, nil)
	mEC2.EXPECT().AttachVolumeWithContext(ctx, avi).Return(nil, nil)
	expErr = fmt.Errorf("WaitWithContext failure")
	mW = mockaws.NewMockWaiter(mockCtrl)
	cl.EXPECT().NewWaiter(rwM).Return(mW)
	mW.EXPECT().WaitWithContext(ctx).Return(expErr)
	vol, err = awsCl.awsEC2VolumeAttach(ctx, vaa, vid)
	assert.Nil(vol)
	assert.Equal(expErr, err)
	assert.Empty(attachingNodeDevices)

	// api error, pass no context
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	mEC2.EXPECT().DescribeInstancesWithContext(gomock.Not(gomock.Nil()), dii).Return(dio, nil)
	mEC2.EXPECT().AttachVolumeWithContext(gomock.Not(gomock.Nil()), avi).Return(nil, fmt.Errorf("api error"))
	vol, err = awsCl.awsEC2VolumeAttach(nil, vaa, vid)
	assert.Nil(vol)
	assert.Regexp("api error", err)

	// invalid storage type
	vaa.VolumeIdentifier = "not the volume identifier you are looking for"
	vol, err = awsCl.VolumeAttach(ctx, vaa)
	assert.Nil(vol)
	assert.Regexp("storage type currently unsupported", err)
}

func TestAwsEC2VolumeCreate(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()

	// the tests below use a fake type which defines the AwsPAIopsGB property to exercise the related code path
	numST := len(awsCspStorageTypes)
	volType := "fakeVT"
	fakeST := &models.CSPStorageType{
		CspDomainType:          CSPDomainType,
		Description:            "Fake ST with IOPS",
		Name:                   "Aws Fake ST",
		MinAllocationSizeBytes: swag.Int64(int64(1 * units.Gibibyte)),
		MaxAllocationSizeBytes: swag.Int64(int64(16 * units.Tebibyte)),
		AccessibilityScope:     "CSPDOMAIN",
		CspStorageTypeAttributes: map[string]models.ValueType{
			PAService:       models.ValueType{Kind: "STRING", Value: ServiceEC2},
			PAEC2VolumeType: models.ValueType{Kind: "STRING", Value: volType},
			PAIopsGB:        models.ValueType{Kind: "INT", Value: "2"},
			PAIopsMin:       models.ValueType{Kind: "INT", Value: "50"},
			PAIopsMax:       models.ValueType{Kind: "INT", Value: "32000"},
		},
	}
	awsCspStorageTypes = append(awsCspStorageTypes, fakeST)
	defer func() {
		awsCspStorageTypes = awsCspStorageTypes[:numST]
		awsCreateSTMap()
	}()
	awsCreateSTMap()
	cspST := EC2VolTypeToCSPStorageType(volType)
	assert.NotEqual("", cspST)
	assert.EqualValues("Aws Fake ST", cspST)
	svc, awsVolT, stObj := StorageTypeToServiceVolumeType(cspST)
	assert.Equal(ServiceEC2, svc)
	assert.EqualValues(volType, awsVolT)
	assert.Equal(fakeST, stObj)

	volID := "vol-myVol"
	expVol := &csp.Volume{
		CSPDomainType:     CSPDomainType,
		StorageTypeName:   EC2VolTypeToCSPStorageType(volType),
		Identifier:        VolumeIdentifierCreate(ServiceEC2, volID),
		Type:              volType,
		SizeBytes:         20 * int64(units.GiB),
		ProvisioningState: csp.VolumeProvisioningProvisioned,
		Tags:              []string{"n:v"},
		Attachments:       []csp.VolumeAttachment{},
	}

	// success, wait
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cl := mockaws.NewMockAWSClient(mockCtrl)
	fakeSession := &session.Session{}
	awsCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(awsCSP)
	caws, ok := awsCSP.(*CSP)
	assert.True(ok)
	awsAttrs := make(map[string]models.ValueType)
	zone := "zone"
	awsAttrs[AttrAvailabilityZone] = models.ValueType{Kind: "STRING", Value: zone}
	awsCl := &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		attrs:   awsAttrs,
	}
	mEC2 := mockaws.NewMockEC2(mockCtrl)
	cl.EXPECT().NewEC2(fakeSession).Return(mEC2)
	vca := &csp.VolumeCreateArgs{
		StorageTypeName: expVol.StorageTypeName,
		SizeBytes:       expVol.SizeBytes,
		Tags:            expVol.Tags,
	}
	cvi := &ec2.CreateVolumeInput{
		VolumeType:       aws.String(expVol.Type),
		AvailabilityZone: aws.String(zone),
		Encrypted:        aws.Bool(true),
		Size:             aws.Int64(20),
		Iops:             aws.Int64(50),
		TagSpecifications: []*ec2.TagSpecification{
			&ec2.TagSpecification{
				ResourceType: aws.String("volume"),
				Tags: []*ec2.Tag{
					&ec2.Tag{
						Key:   aws.String("n"),
						Value: aws.String("v"),
					},
				},
			},
		},
	}
	cvo := &ec2.Volume{
		VolumeId:   &volID,
		VolumeType: &expVol.Type,
		Encrypted:  aws.Bool(true),
		Size:       aws.Int64(20),
		State:      aws.String(ec2.VolumeStateCreating),
		Tags: []*ec2.Tag{
			&ec2.Tag{
				Key:   aws.String("n"),
				Value: aws.String("v"),
			},
		},
	}
	dvi := &ec2.DescribeVolumesInput{
		VolumeIds: []*string{aws.String(volID)},
	}
	dvo := &ec2.DescribeVolumesOutput{
		Volumes: []*ec2.Volume{
			&ec2.Volume{},
		},
	}
	*dvo.Volumes[0] = *cvo // shallow copy
	dvo.Volumes[0].State = aws.String(ec2.VolumeStateAvailable)
	expVol.Raw = dvo.Volumes[0]
	mEC2.EXPECT().CreateVolumeWithContext(ctx, cvi).Return(cvo, nil)
	mEC2.EXPECT().WaitUntilVolumeAvailableWithContext(ctx, dvi).Return(nil)
	mEC2.EXPECT().DescribeVolumesWithContext(ctx, dvi).Return(dvo, nil)
	assert.Nil(awsCl.ec2)
	vol, err := awsCl.VolumeCreate(ctx, vca)
	assert.NoError(err)
	assert.Equal(expVol, vol)
	assert.Equal(awsCl.ec2, mEC2)

	// wait failure
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	fakeSession = &session.Session{}
	awsCSP, err = csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(awsCSP)
	caws, ok = awsCSP.(*CSP)
	assert.True(ok)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		attrs:   awsAttrs,
		ec2:     mEC2,
	}
	mEC2.EXPECT().CreateVolumeWithContext(ctx, cvi).Return(cvo, nil)
	mEC2.EXPECT().WaitUntilVolumeAvailableWithContext(ctx, dvi).Return(fmt.Errorf("wait error"))
	vol, err = awsCl.VolumeCreate(ctx, vca)
	assert.Regexp("wait error", err)
	assert.Nil(vol)

	// success, no wait, size round up
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	vca.SizeBytes = 1 * int64(units.GB) // AWS sizes are in multiples of GiB
	cvi.Size = aws.Int64(1)
	cvo.Size = aws.Int64(1)
	cvo.State = aws.String(ec2.VolumeStateAvailable)
	expVol.SizeBytes = 1 * int64(units.GiB)
	expVol.Raw = cvo
	cl = mockaws.NewMockAWSClient(mockCtrl)
	fakeSession = &session.Session{}
	awsCSP, err = csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(awsCSP)
	caws, ok = awsCSP.(*CSP)
	assert.True(ok)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		attrs:   awsAttrs,
		ec2:     mEC2,
	}
	mEC2.EXPECT().CreateVolumeWithContext(ctx, cvi).Return(cvo, nil)
	vol, err = awsCl.VolumeCreate(ctx, vca)
	assert.NoError(err)
	assert.Equal(expVol, vol)

	// api error, nil ctx, no tags
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	vca.Tags = []string{}
	cvi.TagSpecifications[0].Tags = nil
	cl = mockaws.NewMockAWSClient(mockCtrl)
	fakeSession = &session.Session{}
	awsCSP, err = csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(awsCSP)
	caws, ok = awsCSP.(*CSP)
	assert.True(ok)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		attrs:   awsAttrs,
		ec2:     mEC2,
	}
	mEC2.EXPECT().CreateVolumeWithContext(gomock.Not(gomock.Nil()), cvi).Return(nil, fmt.Errorf("api error"))
	vol, err = awsCl.VolumeCreate(nil, vca)
	assert.Regexp("api error", err)
	assert.Nil(vol)

	// validate allocation sizes
	awsCl = &Client{
		csp: caws,
	}
	oneGib := int64(units.GiB)
	oneGB := int64(units.GB)
	sizeTCs := []struct{ req, actual int64 }{
		{0, oneGib},
		{oneGB, oneGib},
		{oneGB + 1, oneGib},
		{oneGB + (oneGib - oneGB - 1), oneGib},
		{oneGib, oneGib},
		{oneGib + 1, 2 * oneGib},
	}
	for _, tc := range sizeTCs {
		sizeBytes, err := awsCl.VolumeSize(ctx, cspST, tc.req)
		assert.NoError(err)
		assert.Equal(tc.actual, sizeBytes)
	}
}

func TestAwsEC2VolumeIops(t *testing.T) {
	assert := assert.New(t)

	// the tests below use a fake type which defines the AwsPAIopsGB property to exercise the related code path
	volType := "fakeVT"
	fakeST := &models.CSPStorageType{
		CspDomainType:          CSPDomainType,
		Description:            "Fake ST with IOPS",
		Name:                   "Aws Fake ST",
		MinAllocationSizeBytes: swag.Int64(int64(1 * units.Gibibyte)),
		MaxAllocationSizeBytes: swag.Int64(int64(16 * units.Tebibyte)),
		AccessibilityScope:     "CSPDOMAIN",
		CspStorageTypeAttributes: map[string]models.ValueType{
			PAService:       models.ValueType{Kind: "STRING", Value: ServiceEC2},
			PAEC2VolumeType: models.ValueType{Kind: "STRING", Value: volType},
			PAIopsGB:        models.ValueType{Kind: "INT", Value: "50"},
			PAIopsMin:       models.ValueType{Kind: "INT", Value: "100"},
			PAIopsMax:       models.ValueType{Kind: "INT", Value: "32000"},
		},
	}

	// use the IopsMin
	assert.Equal(awsEC2VolumeIops(fakeST, int64(1)), int64(100))

	// use the IopsGB
	assert.Equal(awsEC2VolumeIops(fakeST, int64(100)), int64(5000))

	// use the IopsMax
	assert.Equal(awsEC2VolumeIops(fakeST, int64(10000)), int64(32000))
}

func TestAwsEC2VolumeDelete(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()

	volID := "vol-myVol"

	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cl := mockaws.NewMockAWSClient(mockCtrl)
	fakeSession := &session.Session{}
	awsCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(awsCSP)
	caws, ok := awsCSP.(*CSP)
	assert.True(ok)
	mEC2 := mockaws.NewMockEC2(mockCtrl)
	cl.EXPECT().NewEC2(fakeSession).Return(mEC2)
	vda := &csp.VolumeDeleteArgs{
		VolumeIdentifier: VolumeIdentifierCreate(ServiceEC2, volID),
	}
	deleteVolumeInput := &ec2.DeleteVolumeInput{
		VolumeId: aws.String(volID),
	}
	dvo := &ec2.DeleteVolumeOutput{}
	mEC2.EXPECT().DeleteVolumeWithContext(ctx, deleteVolumeInput).Return(dvo, nil)
	dvi := &ec2.DescribeVolumesInput{
		VolumeIds: []*string{aws.String(volID)},
	}
	mEC2.EXPECT().WaitUntilVolumeDeletedWithContext(ctx, dvi).Return(nil)
	awsCl := &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
	}
	assert.Nil(awsCl.ec2)
	err = awsCl.VolumeDelete(ctx, vda)
	assert.NoError(err)
	assert.Equal(awsCl.ec2, mEC2)

	// delete, not found error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	cl.EXPECT().NewEC2(fakeSession).Return(mEC2)
	notFoundErr := fmt.Errorf("InvalidVolume.NotFound: The volume 'vol-myVol' does not exist")
	mEC2.EXPECT().DeleteVolumeWithContext(ctx, deleteVolumeInput).Return(dvo, notFoundErr)
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
	}
	assert.Nil(awsCl.ec2)
	err = awsCl.VolumeDelete(ctx, vda)
	assert.Equal(csp.ErrorVolumeNotFound, err)

	// delete, wait error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	cl.EXPECT().NewEC2(fakeSession).Return(mEC2)
	mEC2.EXPECT().DeleteVolumeWithContext(ctx, deleteVolumeInput).Return(dvo, nil)
	mEC2.EXPECT().WaitUntilVolumeDeletedWithContext(ctx, dvi).Return(fmt.Errorf("wait error"))
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
	}
	assert.Nil(awsCl.ec2)
	err = awsCl.VolumeDelete(ctx, vda)
	assert.Regexp("wait error", err)

	// delete, no context, api error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	vda = &csp.VolumeDeleteArgs{
		VolumeIdentifier: VolumeIdentifierCreate(ServiceEC2, volID),
	}
	deleteVolumeInput = &ec2.DeleteVolumeInput{
		VolumeId: aws.String(volID),
	}
	mEC2.EXPECT().DeleteVolumeWithContext(gomock.Not(gomock.Nil()), deleteVolumeInput).Return(nil, fmt.Errorf("api error"))
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	err = awsCl.VolumeDelete(nil, vda)
	assert.Regexp("api error", err)

	// invalid volume id
	vda = &csp.VolumeDeleteArgs{
		VolumeIdentifier: "invalidVolumeIdentifier",
	}
	awsCl.ec2 = nil
	err = awsCl.VolumeDelete(nil, vda)
	assert.Regexp("invalid volume identifier", err)
}

func TestAwsEC2VolumeDetach(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()

	nid := "n-1"
	vid := "v-1"
	prefix := "/dev/xvd"
	devicePath := prefix + "ba"
	vda := &csp.VolumeDetachArgs{
		VolumeIdentifier: VolumeIdentifierCreate(ServiceEC2, vid),
		NodeIdentifier:   nid,
		NodeDevice:       devicePath,
	}
	dvr := &ec2.DetachVolumeInput{
		Device:     aws.String(devicePath),
		InstanceId: aws.String(nid),
		VolumeId:   aws.String(vid),
	}
	dvi := &ec2.DescribeVolumesInput{VolumeIds: []*string{&vid}}
	vObj := &csp.Volume{
		CSPDomainType:     CSPDomainType,
		StorageTypeName:   EC2VolTypeToCSPStorageType("gp2"),
		Identifier:        VolumeIdentifierCreate(ServiceEC2, vid),
		Type:              "gp2",
		SizeBytes:         20 * int64(units.GiB),
		ProvisioningState: csp.VolumeProvisioningProvisioned,
		Tags:              []string{"n:v"},
		Attachments:       []csp.VolumeAttachment{},
	}
	evObj := &ec2.Volume{
		VolumeId:    &vid,
		VolumeType:  &vObj.Type,
		Size:        aws.Int64(20),
		State:       aws.String(ec2.VolumeStateInUse),
		Attachments: []*ec2.VolumeAttachment{},
		Tags: []*ec2.Tag{
			&ec2.Tag{
				Key:   aws.String("n"),
				Value: aws.String("v"),
			},
		},
	}
	vObj.Raw = evObj
	dvo := &ec2.DescribeVolumesOutput{Volumes: []*ec2.Volume{evObj}}
	awsCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(awsCSP)
	caws, ok := awsCSP.(*CSP)
	assert.True(ok)

	// success with context, new attachment
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cl := mockaws.NewMockAWSClient(mockCtrl)
	fakeSession := &session.Session{}
	mEC2 := mockaws.NewMockEC2(mockCtrl)
	dvr.Force = aws.Bool(false)
	mEC2.EXPECT().DetachVolumeWithContext(ctx, dvr).Return(nil, nil)
	mEC2.EXPECT().WaitUntilVolumeAvailableWithContext(ctx, dvi).Return(nil)
	mEC2.EXPECT().DescribeVolumesWithContext(ctx, dvi).Return(dvo, nil)
	awsCl := &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	vol, err := awsCl.VolumeDetach(ctx, vda)
	assert.Equal(vObj, vol)
	assert.NoError(err)

	// fail to detach
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	expErr := fmt.Errorf("detach failure")
	mEC2.EXPECT().DetachVolumeWithContext(ctx, dvr).Return(nil, expErr)
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	vol, err = awsCl.awsEC2VolumeDetach(ctx, vda, vid)
	assert.Nil(vol)
	assert.Equal(expErr, err)

	// already detached
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	expErr = fmt.Errorf("IncorrectState: Volume 'vol-0529d29a7dfa05ab1'is in the 'available' state")
	mEC2.EXPECT().DetachVolumeWithContext(ctx, dvr).Return(nil, expErr)
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	vol, err = awsCl.awsEC2VolumeDetach(ctx, vda, vid)
	assert.Nil(vol)
	assert.Equal(csp.ErrorVolumeNotAttached, err)

	// fail during wait
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	mEC2.EXPECT().DetachVolumeWithContext(ctx, dvr).Return(nil, nil)
	expErr = fmt.Errorf("WaitWithContext failure")
	mEC2.EXPECT().WaitUntilVolumeAvailableWithContext(ctx, dvi).Return(expErr)
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	vol, err = awsCl.awsEC2VolumeDetach(ctx, vda, vid)
	assert.Nil(vol)
	assert.Equal(expErr, err)

	// no context, api error, force
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	vda.Force = true
	dvr.Force = aws.Bool(true)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	mEC2.EXPECT().DetachVolumeWithContext(gomock.Not(gomock.Nil()), dvr).Return(nil, fmt.Errorf("api error"))
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	vol, err = awsCl.awsEC2VolumeDetach(nil, vda, vid)
	assert.Nil(vol)
	assert.Regexp("api error", err)

	// invalid storage type
	vda.VolumeIdentifier = "not the volume identifier you are looking for"
	vol, err = awsCl.VolumeDetach(ctx, vda)
	assert.Nil(vol)
	assert.Regexp("storage type currently unsupported", err)
}

func TestAwsEC2VolumeDevicePath(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()

	nid := "i-1"
	vid0 := "v-0"
	vid1 := "v-1"
	prefix := "/dev/xvd"
	req := &ec2.DescribeInstancesInput{InstanceIds: []*string{&nid}}
	obj1 := &ec2.Instance{
		InstanceId: &nid,
		BlockDeviceMappings: []*ec2.InstanceBlockDeviceMapping{
			{
				DeviceName: aws.String(prefix + "ba"),
				Ebs: &ec2.EbsInstanceBlockDevice{
					VolumeId: aws.String(vid0),
				},
			},
		},
	}
	dio := &ec2.DescribeInstancesOutput{
		Reservations: []*ec2.Reservation{
			{
				Instances: []*ec2.Instance{obj1},
			},
		},
	}
	awsCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(awsCSP)
	caws, ok := awsCSP.(*CSP)
	assert.True(ok)

	// success, allocates a new device
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cl := mockaws.NewMockAWSClient(mockCtrl)
	fakeSession := &session.Session{}
	mEC2 := mockaws.NewMockEC2(mockCtrl)
	mEC2.EXPECT().DescribeInstancesWithContext(ctx, req).Return(dio, nil)
	awsCl := &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	path1, added, err := awsCl.awsEC2VolumeDevicePath(ctx, vid1, nid)
	assert.Equal(prefix+"bb", path1)
	assert.True(added)
	assert.NoError(err)
	assert.Len(attachingNodeDevices, 1)
	_, ok = attachingNodeDevices[nid+":"+path1]
	assert.True(ok)

	// success, already mapped
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	mEC2.EXPECT().DescribeInstancesWithContext(ctx, req).Return(dio, nil)
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	path2, added, err := awsCl.awsEC2VolumeDevicePath(ctx, vid0, nid)
	assert.Equal(prefix+"ba", path2)
	assert.False(added)
	assert.NoError(err)

	// success with reserved (from 1st case)
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	mEC2.EXPECT().DescribeInstancesWithContext(ctx, req).Return(dio, nil)
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	path2, added, err = awsCl.awsEC2VolumeDevicePath(ctx, vid1, nid)
	assert.Equal(prefix+"bc", path2)
	assert.True(added)
	assert.NoError(err)
	assert.Len(attachingNodeDevices, 2)
	_, ok = attachingNodeDevices[nid+":"+path1]
	assert.True(ok)
	_, ok = attachingNodeDevices[nid+":"+path2]
	assert.True(ok)

	// release reservations
	awsCl.awsEC2VolumeDevicePathRelease(nid, path1)
	awsCl.awsEC2VolumeDevicePathRelease(nid, path2)
	assert.Empty(attachingNodeDevices)

	// failure, all devices in use
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	for first := 'b'; first <= 'c'; first++ {
		for second := 'a'; second <= 'z'; second++ {
			attachingNodeDevices[nid+":"+prefix+string([]rune{first, second})] = struct{}{}
		}
	}
	assert.Len(attachingNodeDevices, 52)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	mEC2.EXPECT().DescribeInstancesWithContext(ctx, req).Return(dio, nil)
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	path2, added, err = awsCl.awsEC2VolumeDevicePath(ctx, vid1, nid)
	assert.False(added)
	assert.Regexp("all device names are in use", err)
	attachingNodeDevices = make(map[string]struct{})
}

func TestAwsEC2VolumeTagsSet(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()

	volID := "vol-myVol"
	expVol := &csp.Volume{
		CSPDomainType:     CSPDomainType,
		StorageTypeName:   EC2VolTypeToCSPStorageType("gp2"),
		Identifier:        VolumeIdentifierCreate(ServiceEC2, volID),
		Type:              "gp2",
		SizeBytes:         20 * int64(units.GiB),
		ProvisioningState: csp.VolumeProvisioningProvisioned,
		Tags:              []string{"n:v"},
		Attachments: []csp.VolumeAttachment{
			{
				NodeIdentifier: "node1",
				Device:         "/dev/block",
				State:          csp.VolumeAttachmentAttached,
			},
		},
	}
	dvi := &ec2.DescribeVolumesInput{
		VolumeIds: []*string{&volID},
	}
	dvo := &ec2.DescribeVolumesOutput{
		Volumes: []*ec2.Volume{
			&ec2.Volume{
				VolumeId:   &volID,
				VolumeType: &expVol.Type,
				Size:       aws.Int64(20),
				State:      aws.String(ec2.VolumeStateInUse),
				Attachments: []*ec2.VolumeAttachment{
					&ec2.VolumeAttachment{
						InstanceId: &expVol.Attachments[0].NodeIdentifier,
						Device:     &expVol.Attachments[0].Device,
						State:      aws.String(ec2.AttachmentStatusAttached),
					},
				},
				Tags: []*ec2.Tag{
					&ec2.Tag{
						Key:   aws.String("n"),
						Value: aws.String("v"),
					},
				},
			},
		},
	}
	expVol.Raw = dvo.Volumes[0]

	// successful tag add/modify, with ctx
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cl := mockaws.NewMockAWSClient(mockCtrl)
	fakeSession := &session.Session{}
	awsCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(awsCSP)
	caws, ok := awsCSP.(*CSP)
	assert.True(ok)
	awsCl := &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
	}
	mEC2 := mockaws.NewMockEC2(mockCtrl)
	cl.EXPECT().NewEC2(fakeSession).Return(mEC2)
	vta := &csp.VolumeTagArgs{
		VolumeIdentifier: expVol.Identifier,
		Tags:             []string{"new:value"},
	}
	cti := &ec2.CreateTagsInput{
		Resources: []*string{aws.String(volID)},
		Tags: []*ec2.Tag{
			&ec2.Tag{
				Key:   aws.String("new"),
				Value: aws.String("value"),
			},
		},
	}
	cto := &ec2.CreateTagsOutput{}
	mEC2.EXPECT().CreateTagsWithContext(ctx, cti).Return(cto, nil)
	mEC2.EXPECT().DescribeVolumesWithContext(ctx, dvi).Return(dvo, nil) // embedded fetch call
	assert.Nil(awsCl.ec2)
	vol, err := awsCl.VolumeTagsSet(ctx, vta)
	assert.NoError(err)
	assert.Equal(expVol, vol)
	assert.Equal(awsCl.ec2, mEC2)

	// Repeat, no ctx, cached ec2 client
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	mEC2.EXPECT().CreateTagsWithContext(gomock.Not(gomock.Nil()), cti).Return(cto, nil)
	mEC2.EXPECT().DescribeVolumesWithContext(gomock.Not(gomock.Nil()), dvi).Return(dvo, nil)
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	vol, err = awsCl.VolumeTagsSet(nil, vta)
	assert.NoError(err)
	assert.Equal(expVol, vol)

	// API error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	mEC2.EXPECT().CreateTagsWithContext(ctx, cti).Return(nil, fmt.Errorf("create tags error"))
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	vol, err = awsCl.VolumeTagsSet(ctx, vta)
	assert.Regexp("create tags error", err)
	assert.Nil(vol)
}

func TestAwsEC2VolumeTagsDelete(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()

	volID := "vol-myVol"
	expVol := &csp.Volume{
		CSPDomainType:     CSPDomainType,
		StorageTypeName:   EC2VolTypeToCSPStorageType("gp2"),
		Identifier:        VolumeIdentifierCreate(ServiceEC2, volID),
		Type:              "gp2",
		SizeBytes:         20 * int64(units.GiB),
		ProvisioningState: csp.VolumeProvisioningProvisioned,
		Tags:              []string{"n:v"},
		Attachments: []csp.VolumeAttachment{
			{
				NodeIdentifier: "node1",
				Device:         "/dev/block",
				State:          csp.VolumeAttachmentAttached,
			},
		},
	}
	dvi := &ec2.DescribeVolumesInput{
		VolumeIds: []*string{&volID},
	}
	dvo := &ec2.DescribeVolumesOutput{
		Volumes: []*ec2.Volume{
			&ec2.Volume{
				VolumeId:   &volID,
				VolumeType: &expVol.Type,
				Size:       aws.Int64(20),
				State:      aws.String(ec2.VolumeStateInUse),
				Attachments: []*ec2.VolumeAttachment{
					&ec2.VolumeAttachment{
						InstanceId: &expVol.Attachments[0].NodeIdentifier,
						Device:     &expVol.Attachments[0].Device,
						State:      aws.String(ec2.AttachmentStatusAttached),
					},
				},
				Tags: []*ec2.Tag{
					&ec2.Tag{
						Key:   aws.String("n"),
						Value: aws.String("v"),
					},
				},
			},
		},
	}
	expVol.Raw = dvo.Volumes[0]

	// successful tag delete with ctx
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cl := mockaws.NewMockAWSClient(mockCtrl)
	fakeSession := &session.Session{}
	awsCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(awsCSP)
	caws, ok := awsCSP.(*CSP)
	assert.True(ok)
	awsCl := &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
	}
	mEC2 := mockaws.NewMockEC2(mockCtrl)
	cl.EXPECT().NewEC2(fakeSession).Return(mEC2)
	vta := &csp.VolumeTagArgs{
		VolumeIdentifier: expVol.Identifier,
		Tags:             []string{"new:value"},
	}
	dti := &ec2.DeleteTagsInput{
		Resources: []*string{aws.String(volID)},
		Tags: []*ec2.Tag{
			&ec2.Tag{
				Key:   aws.String("new"),
				Value: aws.String("value"),
			},
		},
	}
	dto := &ec2.DeleteTagsOutput{}
	mEC2.EXPECT().DeleteTagsWithContext(ctx, dti).Return(dto, nil)
	mEC2.EXPECT().DescribeVolumesWithContext(ctx, dvi).Return(dvo, nil) // embedded fetch call
	assert.Nil(awsCl.ec2)
	vol, err := awsCl.VolumeTagsDelete(ctx, vta)
	assert.NoError(err)
	assert.Equal(expVol, vol)
	assert.Equal(awsCl.ec2, mEC2)

	// Repeat, no ctx, cached ec2 client
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	mEC2.EXPECT().DeleteTagsWithContext(gomock.Not(gomock.Nil()), dti).Return(dto, nil)
	mEC2.EXPECT().DescribeVolumesWithContext(gomock.Not(gomock.Nil()), dvi).Return(dvo, nil)
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	vol, err = awsCl.VolumeTagsDelete(nil, vta)
	assert.NoError(err)
	assert.Equal(expVol, vol)
	assert.Equal(awsCl.ec2, mEC2)

	// API error
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	mEC2.EXPECT().DeleteTagsWithContext(ctx, dti).Return(nil, fmt.Errorf("create tags error"))
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	vol, err = awsCl.VolumeTagsDelete(ctx, vta)
	assert.Regexp("create tags error", err)
	assert.Nil(vol)
}

func TestAwsEC2VolumeWaitForAttachmentState(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()

	vid := "v-1"
	state := ec2.VolumeAttachmentStateAttached
	oReq := &request.Request{HTTPRequest: &http.Request{}}
	awsCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.NoError(err)
	assert.NotNil(awsCSP)
	caws, ok := awsCSP.(*CSP)
	assert.True(ok)

	// success
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cl := mockaws.NewMockAWSClient(mockCtrl)
	fakeSession := &session.Session{}
	awsCl := &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
	}
	mEC2 := mockaws.NewMockEC2(mockCtrl)
	cl.EXPECT().NewEC2(fakeSession).Return(mEC2)
	mEC2.EXPECT().DescribeVolumesRequest(&ec2.DescribeVolumesInput{VolumeIds: []*string{&vid}}).Return(oReq, nil)
	rwM := newWaiterMatcher(t).WithExpected(state)
	mW := mockaws.NewMockWaiter(mockCtrl)
	cl.EXPECT().NewWaiter(rwM).Return(mW)
	mW.EXPECT().WaitWithContext(ctx).Return(nil)
	err = awsCl.awsEC2VolumeWaitForAttachmentState(ctx, vid, state)
	assert.NoError(err)
	if assert.NotNil(rwM.newRequest) {
		req, err := rwM.newRequest([]request.Option{})
		assert.NotNil(req)
		assert.NoError(err)
		assert.Equal(ctx, req.Context())
	}

	// fail on Wait, also cover no context case
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockaws.NewMockAWSClient(mockCtrl)
	mEC2 = mockaws.NewMockEC2(mockCtrl)
	rwM.newRequest = nil
	mW = mockaws.NewMockWaiter(mockCtrl)
	cl.EXPECT().NewWaiter(rwM).Return(mW)
	mW.EXPECT().WaitWithContext(gomock.Not(gomock.Nil())).Return(fmt.Errorf("WaitWithContext error"))
	mEC2.EXPECT().DescribeVolumesRequest(&ec2.DescribeVolumesInput{VolumeIds: []*string{&vid}}).Return(oReq, nil)
	awsCl = &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
		ec2:     mEC2,
	}
	err = awsCl.awsEC2VolumeWaitForAttachmentState(nil, vid, state)
	assert.Regexp("WaitWithContext error", err)
	if assert.NotNil(rwM.newRequest) {
		req, err := rwM.newRequest([]request.Option{})
		assert.NotNil(req)
		assert.NoError(err)
		assert.NotNil(req.Context())
	}
}

// gomock.Matcher for Waiter
type mockWaiterMatcher struct {
	t          *testing.T
	expected   string
	newRequest func([]request.Option) (*request.Request, error) // result
}

var _ = gomock.Matcher(&mockWaiterMatcher{})

func newWaiterMatcher(t *testing.T) *mockWaiterMatcher {
	return &mockWaiterMatcher{t: t}
}

func (m *mockWaiterMatcher) WithExpected(expected string) *mockWaiterMatcher {
	m.expected = expected
	return m
}

func (m *mockWaiterMatcher) Matches(x interface{}) bool {
	assert := assert.New(m.t)
	w, ok := x.(*request.Waiter)
	assert.True(ok)
	if ok {
		ok = assert.NotZero(w.MaxAttempts) &&
			assert.NotNil(w.Delay) &&
			assert.NotNil(w.NewRequest) &&
			assert.NotZero(w.Delay(1)) &&
			assert.Len(w.Acceptors, 2) &&
			assert.Equal(request.SuccessWaiterState, w.Acceptors[0].State) &&
			assert.Equal(m.expected, w.Acceptors[0].Expected) &&
			assert.Equal(request.FailureWaiterState, w.Acceptors[1].State) &&
			assert.Equal("deleted", w.Acceptors[1].Expected)
		m.newRequest = w.NewRequest
	}
	return ok
}

func (m *mockWaiterMatcher) String() string {
	return "matches request.Waiter"
}
