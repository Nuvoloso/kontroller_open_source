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

	mockAWS "github.com/Nuvoloso/kontroller/pkg/awssdk/mock"
	"github.com/Nuvoloso/kontroller/pkg/csp"
	awsSession "github.com/aws/aws-sdk-go/aws/session"
	awsEC2 "github.com/aws/aws-sdk-go/service/ec2"
	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestAwsEC2InstanceFetch(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()

	instanceID := "i-0"
	obj := &awsEC2.Instance{
		InstanceId: &instanceID,
	}
	req := &awsEC2.DescribeInstancesInput{InstanceIds: []*string{&instanceID}}
	dio := &awsEC2.DescribeInstancesOutput{
		Reservations: []*awsEC2.Reservation{
			{
				Instances: []*awsEC2.Instance{
					{
						InstanceId: &instanceID,
					},
				},
			},
		},
	}

	awsCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(awsCSP)
	caws, ok := awsCSP.(*CSP)
	assert.True(ok)
	assert.NotNil(caws)

	// success with context
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cl := mockAWS.NewMockAWSClient(mockCtrl)
	fakeSession := &awsSession.Session{}
	awsCl := &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
	}
	mEC2 := mockAWS.NewMockEC2(mockCtrl)
	cl.EXPECT().NewEC2(fakeSession).Return(mEC2)
	mEC2.EXPECT().DescribeInstancesWithContext(ctx, req).Return(dio, nil)
	result, err := awsCl.awsEC2InstanceFetch(ctx, instanceID)
	assert.NoError(err)
	assert.NotNil(result)
	assert.Equal(*obj, *result)

	// not found, should use cached client
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockAWS.NewMockAWSClient(mockCtrl)
	mEC2 = mockAWS.NewMockEC2(mockCtrl)
	awsCl = &Client{
		csp:    caws,
		client: cl,
		ec2:    mEC2,
	}
	dio.Reservations = []*awsEC2.Reservation{}
	mEC2.EXPECT().DescribeInstancesWithContext(ctx, req).Return(dio, nil)
	result, err = awsCl.awsEC2InstanceFetch(ctx, instanceID)
	assert.Regexp("not found", err)

	// awsEC2FetchInstances fails, also cover the no-context path
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockAWS.NewMockAWSClient(mockCtrl)
	mEC2 = mockAWS.NewMockEC2(mockCtrl)
	awsCl = &Client{
		csp:    caws,
		client: cl,
		ec2:    mEC2,
	}
	expErrStr := "describeInstances error"
	mEC2.EXPECT().DescribeInstancesWithContext(gomock.Not(gomock.Nil()), req).Return(nil, fmt.Errorf(expErrStr))
	result, err = awsCl.awsEC2InstanceFetch(nil, instanceID)
	assert.Regexp(expErrStr, err)
}

func TestAwsEC2InstancesFetch(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()

	instanceID1 := "i-1"
	instanceID2 := "i-2"
	instanceID3 := "i-3"
	instanceID4 := "i-4"
	instanceID5 := "i-5"
	obj1 := &awsEC2.Instance{InstanceId: &instanceID1}
	obj2 := &awsEC2.Instance{InstanceId: &instanceID2}
	obj3 := &awsEC2.Instance{InstanceId: &instanceID3}
	obj4 := &awsEC2.Instance{InstanceId: &instanceID4}
	obj5 := &awsEC2.Instance{InstanceId: &instanceID5}

	nextToken := "nextToken"
	req1 := &awsEC2.DescribeInstancesInput{InstanceIds: []*string{&instanceID1, &instanceID2, &instanceID3, &instanceID4, &instanceID5}}
	req2 := &awsEC2.DescribeInstancesInput{InstanceIds: req1.InstanceIds, NextToken: &nextToken}
	dio1 := &awsEC2.DescribeInstancesOutput{
		NextToken: &nextToken,
		Reservations: []*awsEC2.Reservation{
			{
				Instances: []*awsEC2.Instance{obj1, obj2},
			}, {
				Instances: []*awsEC2.Instance{obj3},
			},
			{
				Instances: []*awsEC2.Instance{obj4},
			},
		},
	}
	dio2 := &awsEC2.DescribeInstancesOutput{
		Reservations: []*awsEC2.Reservation{
			{
				Instances: []*awsEC2.Instance{obj5},
			},
		},
	}

	awsCSP, err := csp.NewCloudServiceProvider(CSPDomainType)
	assert.Nil(err)
	assert.NotNil(awsCSP)
	caws, ok := awsCSP.(*CSP)
	assert.True(ok)
	assert.NotNil(caws)

	// success with NextToken and context
	mockCtrl := gomock.NewController(t)
	defer func() { mockCtrl.Finish() }()
	cl := mockAWS.NewMockAWSClient(mockCtrl)
	fakeSession := &awsSession.Session{}
	awsCl := &Client{
		csp:     caws,
		client:  cl,
		session: fakeSession,
	}
	mEC2 := mockAWS.NewMockEC2(mockCtrl)
	cl.EXPECT().NewEC2(fakeSession).Return(mEC2)
	mEC2.EXPECT().DescribeInstancesWithContext(ctx, req1).Return(dio1, nil)
	mEC2.EXPECT().DescribeInstancesWithContext(ctx, req2).Return(dio2, nil)
	result, err := awsCl.awsEC2InstancesFetch(ctx, req1.InstanceIds)
	assert.NoError(err)
	assert.NotNil(result)
	assert.Equal([]*awsEC2.Instance{obj1, obj2, obj3, obj4, obj5}, result)

	// success without NextToken, nil InstanceIds are filtered, and no context, client is cached
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockAWS.NewMockAWSClient(mockCtrl)
	mEC2 = mockAWS.NewMockEC2(mockCtrl)
	awsCl = &Client{
		csp:    caws,
		client: cl,
		ec2:    mEC2,
	}
	dio1.NextToken = nil
	obj3.InstanceId = nil
	mEC2.EXPECT().DescribeInstancesWithContext(gomock.Not(gomock.Nil()), req1).Return(dio1, nil)
	result, err = awsCl.awsEC2InstancesFetch(nil, req1.InstanceIds)
	assert.NoError(err)
	assert.NotNil(result)
	assert.Equal([]*awsEC2.Instance{obj1, obj2, obj4}, result)

	// success with empty list
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockAWS.NewMockAWSClient(mockCtrl)
	awsCl = &Client{
		csp:    caws,
		client: cl,
		ec2:    mEC2,
	}
	result, err = awsCl.awsEC2InstancesFetch(nil, nil)
	assert.NoError(err)
	assert.NotNil(result)

	// DescribeInstancesWithContext fails
	mockCtrl.Finish()
	mockCtrl = gomock.NewController(t)
	cl = mockAWS.NewMockAWSClient(mockCtrl)
	mEC2 = mockAWS.NewMockEC2(mockCtrl)
	awsCl = &Client{
		csp:    caws,
		client: cl,
		ec2:    mEC2,
	}
	expErrStr := "DescribeInstancesWithContext failure"
	mEC2.EXPECT().DescribeInstancesWithContext(ctx, req1).Return(nil, fmt.Errorf(expErrStr))
	result, err = awsCl.awsEC2InstancesFetch(ctx, req1.InstanceIds)
	assert.Nil(result)
	assert.Error(err)
	assert.Regexp(expErrStr, err)
}
