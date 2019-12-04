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

	"github.com/aws/aws-sdk-go/aws"
	awsEC2 "github.com/aws/aws-sdk-go/service/ec2"
)

// There are currently no exposed functions for instances, only awsEC2 functions

func (cl *Client) awsEC2InstanceFetch(ctx context.Context, instanceID string) (*awsEC2.Instance, error) {
	instances, err := cl.awsEC2InstancesFetch(ctx, []*string{&instanceID})
	if err != nil {
		return nil, err
	}
	if len(instances) == 0 {
		return nil, fmt.Errorf("not found")
	}
	return instances[0], nil
}

func (cl *Client) awsEC2InstancesFetch(ctx context.Context, instanceIDs []*string) ([]*awsEC2.Instance, error) {
	result := []*awsEC2.Instance{}
	if len(instanceIDs) == 0 {
		return result, nil
	}
	ec2client := cl.ec2Client()
	req := &awsEC2.DescribeInstancesInput{InstanceIds: instanceIDs}
	if ctx == nil {
		var cancelFn func()
		ctx = context.Background()
		ctx, cancelFn = context.WithTimeout(ctx, cl.Timeout)
		defer cancelFn()
	}
	for {
		dio, err := ec2client.DescribeInstancesWithContext(ctx, req)
		if err != nil {
			return nil, err
		}
		for _, reservation := range dio.Reservations {
			for _, instance := range reservation.Instances {
				if instanceID := aws.StringValue(instance.InstanceId); instanceID != "" {
					result = append(result, instance)
				}
			}
		}
		if aws.StringValue(dio.NextToken) == "" {
			break
		}
		req.NextToken = dio.NextToken
	}
	return result, nil
}
