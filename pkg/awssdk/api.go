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


package awssdk

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/sts"
)

// AWSClient is an interface over the AWS SDK
type AWSClient interface {
	// Add factory methods as needed
	NewSession(cfgs ...*aws.Config) (*session.Session, error)
	NewEC2(p client.ConfigProvider, cfgs ...*aws.Config) EC2
	NewSTS(p client.ConfigProvider, cfgs ...*aws.Config) STS
	NewS3(p client.ConfigProvider, cfgs ...*aws.Config) S3
	NewWaiter(adoptee *request.Waiter) Waiter
}

// EC2 is an interface for the EC2 service client methods.
// Note that the AWS SDK provides interface EC2API in package ec2/ec2iface but it would
// generate a huge mock client. Instead, copy method signatures from there - the package
// variables used are identical and the file is a lot easier to search.
type EC2 interface {
	// Add required methods as needed. They are organized here by object and not verb.

	// AvailabilityZones
	DescribeAvailabilityZonesWithContext(aws.Context, *ec2.DescribeAvailabilityZonesInput, ...request.Option) (*ec2.DescribeAvailabilityZonesOutput, error)

	// Instances related
	DescribeInstancesWithContext(ctx aws.Context, input *ec2.DescribeInstancesInput, opts ...request.Option) (*ec2.DescribeInstancesOutput, error)

	// Tags related (applies to all resources)
	CreateTagsWithContext(aws.Context, *ec2.CreateTagsInput, ...request.Option) (*ec2.CreateTagsOutput, error)
	DeleteTagsWithContext(aws.Context, *ec2.DeleteTagsInput, ...request.Option) (*ec2.DeleteTagsOutput, error)

	// Volume related
	AttachVolumeWithContext(aws.Context, *ec2.AttachVolumeInput, ...request.Option) (*ec2.VolumeAttachment, error)
	DescribeVolumesRequest(input *ec2.DescribeVolumesInput) (req *request.Request, output *ec2.DescribeVolumesOutput)
	DescribeVolumesWithContext(aws.Context, *ec2.DescribeVolumesInput, ...request.Option) (*ec2.DescribeVolumesOutput, error)
	DetachVolumeWithContext(aws.Context, *ec2.DetachVolumeInput, ...request.Option) (*ec2.VolumeAttachment, error)
	CreateVolumeWithContext(aws.Context, *ec2.CreateVolumeInput, ...request.Option) (*ec2.Volume, error)
	DeleteVolumeWithContext(aws.Context, *ec2.DeleteVolumeInput, ...request.Option) (*ec2.DeleteVolumeOutput, error)
	WaitUntilVolumeAvailableWithContext(ctx aws.Context, input *ec2.DescribeVolumesInput, opts ...request.WaiterOption) error
	WaitUntilVolumeDeletedWithContext(ctx aws.Context, input *ec2.DescribeVolumesInput, opts ...request.WaiterOption) error
}

// STS is an interface for the sts service client methods.
type STS interface {
	GetCallerIdentityWithContext(aws.Context, *sts.GetCallerIdentityInput, ...request.Option) (*sts.GetCallerIdentityOutput, error)
}

// S3 is an interface for the s3 service client methods
type S3 interface {
	CreateBucketWithContext(aws.Context, *s3.CreateBucketInput, ...request.Option) (*s3.CreateBucketOutput, error)
	PutPublicAccessBlockWithContext(aws.Context, *s3.PutPublicAccessBlockInput, ...request.Option) (*s3.PutPublicAccessBlockOutput, error)
}

// Waiter is an interface for request.Waiter methods.
type Waiter interface {
	WaitWithContext(ctx aws.Context) error
}

// SDK satisfies AWSClient.API
type SDK struct{}

var _ = AWSClient(&SDK{})

// New returns the real implementation of API over the AWS SDK
func New() AWSClient {
	return &SDK{}
}

// NewSession wraps session.NewSession
func (c *SDK) NewSession(cfgs ...*aws.Config) (*session.Session, error) {
	return session.NewSession(cfgs...)
}

// NewEC2 wraps ec2.New
func (c *SDK) NewEC2(p client.ConfigProvider, cfgs ...*aws.Config) EC2 {
	return ec2.New(p, cfgs...)
}

// NewSTS wraps sts.New
func (c *SDK) NewSTS(p client.ConfigProvider, cfgs ...*aws.Config) STS {
	return sts.New(p, cfgs...)
}

// NewS3 wraps s3.New
func (c *SDK) NewS3(p client.ConfigProvider, cfgs ...*aws.Config) S3 {
	return s3.New(p, cfgs...)
}

// NewWaiter adopts a previously populated Waiter struct
func (c *SDK) NewWaiter(adoptee *request.Waiter) Waiter {
	return adoptee
}
