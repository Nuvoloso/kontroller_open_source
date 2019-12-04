// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/Nuvoloso/kontroller/pkg/awssdk (interfaces: AWSClient,EC2,STS,S3,Waiter)

// Package mock is a generated GoMock package.
package mock

import (
	context "context"
	awssdk "github.com/Nuvoloso/kontroller/pkg/awssdk"
	aws "github.com/aws/aws-sdk-go/aws"
	client "github.com/aws/aws-sdk-go/aws/client"
	request "github.com/aws/aws-sdk-go/aws/request"
	session "github.com/aws/aws-sdk-go/aws/session"
	ec2 "github.com/aws/aws-sdk-go/service/ec2"
	s3 "github.com/aws/aws-sdk-go/service/s3"
	sts "github.com/aws/aws-sdk-go/service/sts"
	gomock "github.com/golang/mock/gomock"
	reflect "reflect"
)

// MockAWSClient is a mock of AWSClient interface
type MockAWSClient struct {
	ctrl     *gomock.Controller
	recorder *MockAWSClientMockRecorder
}

// MockAWSClientMockRecorder is the mock recorder for MockAWSClient
type MockAWSClientMockRecorder struct {
	mock *MockAWSClient
}

// NewMockAWSClient creates a new mock instance
func NewMockAWSClient(ctrl *gomock.Controller) *MockAWSClient {
	mock := &MockAWSClient{ctrl: ctrl}
	mock.recorder = &MockAWSClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockAWSClient) EXPECT() *MockAWSClientMockRecorder {
	return m.recorder
}

// NewEC2 mocks base method
func (m *MockAWSClient) NewEC2(arg0 client.ConfigProvider, arg1 ...*aws.Config) awssdk.EC2 {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0}
	for _, a := range arg1 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "NewEC2", varargs...)
	ret0, _ := ret[0].(awssdk.EC2)
	return ret0
}

// NewEC2 indicates an expected call of NewEC2
func (mr *MockAWSClientMockRecorder) NewEC2(arg0 interface{}, arg1 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0}, arg1...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewEC2", reflect.TypeOf((*MockAWSClient)(nil).NewEC2), varargs...)
}

// NewS3 mocks base method
func (m *MockAWSClient) NewS3(arg0 client.ConfigProvider, arg1 ...*aws.Config) awssdk.S3 {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0}
	for _, a := range arg1 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "NewS3", varargs...)
	ret0, _ := ret[0].(awssdk.S3)
	return ret0
}

// NewS3 indicates an expected call of NewS3
func (mr *MockAWSClientMockRecorder) NewS3(arg0 interface{}, arg1 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0}, arg1...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewS3", reflect.TypeOf((*MockAWSClient)(nil).NewS3), varargs...)
}

// NewSTS mocks base method
func (m *MockAWSClient) NewSTS(arg0 client.ConfigProvider, arg1 ...*aws.Config) awssdk.STS {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0}
	for _, a := range arg1 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "NewSTS", varargs...)
	ret0, _ := ret[0].(awssdk.STS)
	return ret0
}

// NewSTS indicates an expected call of NewSTS
func (mr *MockAWSClientMockRecorder) NewSTS(arg0 interface{}, arg1 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0}, arg1...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewSTS", reflect.TypeOf((*MockAWSClient)(nil).NewSTS), varargs...)
}

// NewSession mocks base method
func (m *MockAWSClient) NewSession(arg0 ...*aws.Config) (*session.Session, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{}
	for _, a := range arg0 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "NewSession", varargs...)
	ret0, _ := ret[0].(*session.Session)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// NewSession indicates an expected call of NewSession
func (mr *MockAWSClientMockRecorder) NewSession(arg0 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewSession", reflect.TypeOf((*MockAWSClient)(nil).NewSession), arg0...)
}

// NewWaiter mocks base method
func (m *MockAWSClient) NewWaiter(arg0 *request.Waiter) awssdk.Waiter {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NewWaiter", arg0)
	ret0, _ := ret[0].(awssdk.Waiter)
	return ret0
}

// NewWaiter indicates an expected call of NewWaiter
func (mr *MockAWSClientMockRecorder) NewWaiter(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NewWaiter", reflect.TypeOf((*MockAWSClient)(nil).NewWaiter), arg0)
}

// MockEC2 is a mock of EC2 interface
type MockEC2 struct {
	ctrl     *gomock.Controller
	recorder *MockEC2MockRecorder
}

// MockEC2MockRecorder is the mock recorder for MockEC2
type MockEC2MockRecorder struct {
	mock *MockEC2
}

// NewMockEC2 creates a new mock instance
func NewMockEC2(ctrl *gomock.Controller) *MockEC2 {
	mock := &MockEC2{ctrl: ctrl}
	mock.recorder = &MockEC2MockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockEC2) EXPECT() *MockEC2MockRecorder {
	return m.recorder
}

// AttachVolumeWithContext mocks base method
func (m *MockEC2) AttachVolumeWithContext(arg0 context.Context, arg1 *ec2.AttachVolumeInput, arg2 ...request.Option) (*ec2.VolumeAttachment, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "AttachVolumeWithContext", varargs...)
	ret0, _ := ret[0].(*ec2.VolumeAttachment)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// AttachVolumeWithContext indicates an expected call of AttachVolumeWithContext
func (mr *MockEC2MockRecorder) AttachVolumeWithContext(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AttachVolumeWithContext", reflect.TypeOf((*MockEC2)(nil).AttachVolumeWithContext), varargs...)
}

// CreateTagsWithContext mocks base method
func (m *MockEC2) CreateTagsWithContext(arg0 context.Context, arg1 *ec2.CreateTagsInput, arg2 ...request.Option) (*ec2.CreateTagsOutput, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "CreateTagsWithContext", varargs...)
	ret0, _ := ret[0].(*ec2.CreateTagsOutput)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateTagsWithContext indicates an expected call of CreateTagsWithContext
func (mr *MockEC2MockRecorder) CreateTagsWithContext(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateTagsWithContext", reflect.TypeOf((*MockEC2)(nil).CreateTagsWithContext), varargs...)
}

// CreateVolumeWithContext mocks base method
func (m *MockEC2) CreateVolumeWithContext(arg0 context.Context, arg1 *ec2.CreateVolumeInput, arg2 ...request.Option) (*ec2.Volume, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "CreateVolumeWithContext", varargs...)
	ret0, _ := ret[0].(*ec2.Volume)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateVolumeWithContext indicates an expected call of CreateVolumeWithContext
func (mr *MockEC2MockRecorder) CreateVolumeWithContext(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateVolumeWithContext", reflect.TypeOf((*MockEC2)(nil).CreateVolumeWithContext), varargs...)
}

// DeleteTagsWithContext mocks base method
func (m *MockEC2) DeleteTagsWithContext(arg0 context.Context, arg1 *ec2.DeleteTagsInput, arg2 ...request.Option) (*ec2.DeleteTagsOutput, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "DeleteTagsWithContext", varargs...)
	ret0, _ := ret[0].(*ec2.DeleteTagsOutput)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DeleteTagsWithContext indicates an expected call of DeleteTagsWithContext
func (mr *MockEC2MockRecorder) DeleteTagsWithContext(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteTagsWithContext", reflect.TypeOf((*MockEC2)(nil).DeleteTagsWithContext), varargs...)
}

// DeleteVolumeWithContext mocks base method
func (m *MockEC2) DeleteVolumeWithContext(arg0 context.Context, arg1 *ec2.DeleteVolumeInput, arg2 ...request.Option) (*ec2.DeleteVolumeOutput, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "DeleteVolumeWithContext", varargs...)
	ret0, _ := ret[0].(*ec2.DeleteVolumeOutput)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DeleteVolumeWithContext indicates an expected call of DeleteVolumeWithContext
func (mr *MockEC2MockRecorder) DeleteVolumeWithContext(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteVolumeWithContext", reflect.TypeOf((*MockEC2)(nil).DeleteVolumeWithContext), varargs...)
}

// DescribeAvailabilityZonesWithContext mocks base method
func (m *MockEC2) DescribeAvailabilityZonesWithContext(arg0 context.Context, arg1 *ec2.DescribeAvailabilityZonesInput, arg2 ...request.Option) (*ec2.DescribeAvailabilityZonesOutput, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "DescribeAvailabilityZonesWithContext", varargs...)
	ret0, _ := ret[0].(*ec2.DescribeAvailabilityZonesOutput)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DescribeAvailabilityZonesWithContext indicates an expected call of DescribeAvailabilityZonesWithContext
func (mr *MockEC2MockRecorder) DescribeAvailabilityZonesWithContext(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DescribeAvailabilityZonesWithContext", reflect.TypeOf((*MockEC2)(nil).DescribeAvailabilityZonesWithContext), varargs...)
}

// DescribeInstancesWithContext mocks base method
func (m *MockEC2) DescribeInstancesWithContext(arg0 context.Context, arg1 *ec2.DescribeInstancesInput, arg2 ...request.Option) (*ec2.DescribeInstancesOutput, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "DescribeInstancesWithContext", varargs...)
	ret0, _ := ret[0].(*ec2.DescribeInstancesOutput)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DescribeInstancesWithContext indicates an expected call of DescribeInstancesWithContext
func (mr *MockEC2MockRecorder) DescribeInstancesWithContext(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DescribeInstancesWithContext", reflect.TypeOf((*MockEC2)(nil).DescribeInstancesWithContext), varargs...)
}

// DescribeVolumesRequest mocks base method
func (m *MockEC2) DescribeVolumesRequest(arg0 *ec2.DescribeVolumesInput) (*request.Request, *ec2.DescribeVolumesOutput) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DescribeVolumesRequest", arg0)
	ret0, _ := ret[0].(*request.Request)
	ret1, _ := ret[1].(*ec2.DescribeVolumesOutput)
	return ret0, ret1
}

// DescribeVolumesRequest indicates an expected call of DescribeVolumesRequest
func (mr *MockEC2MockRecorder) DescribeVolumesRequest(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DescribeVolumesRequest", reflect.TypeOf((*MockEC2)(nil).DescribeVolumesRequest), arg0)
}

// DescribeVolumesWithContext mocks base method
func (m *MockEC2) DescribeVolumesWithContext(arg0 context.Context, arg1 *ec2.DescribeVolumesInput, arg2 ...request.Option) (*ec2.DescribeVolumesOutput, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "DescribeVolumesWithContext", varargs...)
	ret0, _ := ret[0].(*ec2.DescribeVolumesOutput)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DescribeVolumesWithContext indicates an expected call of DescribeVolumesWithContext
func (mr *MockEC2MockRecorder) DescribeVolumesWithContext(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DescribeVolumesWithContext", reflect.TypeOf((*MockEC2)(nil).DescribeVolumesWithContext), varargs...)
}

// DetachVolumeWithContext mocks base method
func (m *MockEC2) DetachVolumeWithContext(arg0 context.Context, arg1 *ec2.DetachVolumeInput, arg2 ...request.Option) (*ec2.VolumeAttachment, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "DetachVolumeWithContext", varargs...)
	ret0, _ := ret[0].(*ec2.VolumeAttachment)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DetachVolumeWithContext indicates an expected call of DetachVolumeWithContext
func (mr *MockEC2MockRecorder) DetachVolumeWithContext(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DetachVolumeWithContext", reflect.TypeOf((*MockEC2)(nil).DetachVolumeWithContext), varargs...)
}

// WaitUntilVolumeAvailableWithContext mocks base method
func (m *MockEC2) WaitUntilVolumeAvailableWithContext(arg0 context.Context, arg1 *ec2.DescribeVolumesInput, arg2 ...request.WaiterOption) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "WaitUntilVolumeAvailableWithContext", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// WaitUntilVolumeAvailableWithContext indicates an expected call of WaitUntilVolumeAvailableWithContext
func (mr *MockEC2MockRecorder) WaitUntilVolumeAvailableWithContext(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "WaitUntilVolumeAvailableWithContext", reflect.TypeOf((*MockEC2)(nil).WaitUntilVolumeAvailableWithContext), varargs...)
}

// WaitUntilVolumeDeletedWithContext mocks base method
func (m *MockEC2) WaitUntilVolumeDeletedWithContext(arg0 context.Context, arg1 *ec2.DescribeVolumesInput, arg2 ...request.WaiterOption) error {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "WaitUntilVolumeDeletedWithContext", varargs...)
	ret0, _ := ret[0].(error)
	return ret0
}

// WaitUntilVolumeDeletedWithContext indicates an expected call of WaitUntilVolumeDeletedWithContext
func (mr *MockEC2MockRecorder) WaitUntilVolumeDeletedWithContext(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "WaitUntilVolumeDeletedWithContext", reflect.TypeOf((*MockEC2)(nil).WaitUntilVolumeDeletedWithContext), varargs...)
}

// MockSTS is a mock of STS interface
type MockSTS struct {
	ctrl     *gomock.Controller
	recorder *MockSTSMockRecorder
}

// MockSTSMockRecorder is the mock recorder for MockSTS
type MockSTSMockRecorder struct {
	mock *MockSTS
}

// NewMockSTS creates a new mock instance
func NewMockSTS(ctrl *gomock.Controller) *MockSTS {
	mock := &MockSTS{ctrl: ctrl}
	mock.recorder = &MockSTSMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockSTS) EXPECT() *MockSTSMockRecorder {
	return m.recorder
}

// GetCallerIdentityWithContext mocks base method
func (m *MockSTS) GetCallerIdentityWithContext(arg0 context.Context, arg1 *sts.GetCallerIdentityInput, arg2 ...request.Option) (*sts.GetCallerIdentityOutput, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "GetCallerIdentityWithContext", varargs...)
	ret0, _ := ret[0].(*sts.GetCallerIdentityOutput)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetCallerIdentityWithContext indicates an expected call of GetCallerIdentityWithContext
func (mr *MockSTSMockRecorder) GetCallerIdentityWithContext(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetCallerIdentityWithContext", reflect.TypeOf((*MockSTS)(nil).GetCallerIdentityWithContext), varargs...)
}

// MockS3 is a mock of S3 interface
type MockS3 struct {
	ctrl     *gomock.Controller
	recorder *MockS3MockRecorder
}

// MockS3MockRecorder is the mock recorder for MockS3
type MockS3MockRecorder struct {
	mock *MockS3
}

// NewMockS3 creates a new mock instance
func NewMockS3(ctrl *gomock.Controller) *MockS3 {
	mock := &MockS3{ctrl: ctrl}
	mock.recorder = &MockS3MockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockS3) EXPECT() *MockS3MockRecorder {
	return m.recorder
}

// CreateBucketWithContext mocks base method
func (m *MockS3) CreateBucketWithContext(arg0 context.Context, arg1 *s3.CreateBucketInput, arg2 ...request.Option) (*s3.CreateBucketOutput, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "CreateBucketWithContext", varargs...)
	ret0, _ := ret[0].(*s3.CreateBucketOutput)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CreateBucketWithContext indicates an expected call of CreateBucketWithContext
func (mr *MockS3MockRecorder) CreateBucketWithContext(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateBucketWithContext", reflect.TypeOf((*MockS3)(nil).CreateBucketWithContext), varargs...)
}

// PutPublicAccessBlockWithContext mocks base method
func (m *MockS3) PutPublicAccessBlockWithContext(arg0 context.Context, arg1 *s3.PutPublicAccessBlockInput, arg2 ...request.Option) (*s3.PutPublicAccessBlockOutput, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{arg0, arg1}
	for _, a := range arg2 {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "PutPublicAccessBlockWithContext", varargs...)
	ret0, _ := ret[0].(*s3.PutPublicAccessBlockOutput)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// PutPublicAccessBlockWithContext indicates an expected call of PutPublicAccessBlockWithContext
func (mr *MockS3MockRecorder) PutPublicAccessBlockWithContext(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{arg0, arg1}, arg2...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PutPublicAccessBlockWithContext", reflect.TypeOf((*MockS3)(nil).PutPublicAccessBlockWithContext), varargs...)
}

// MockWaiter is a mock of Waiter interface
type MockWaiter struct {
	ctrl     *gomock.Controller
	recorder *MockWaiterMockRecorder
}

// MockWaiterMockRecorder is the mock recorder for MockWaiter
type MockWaiterMockRecorder struct {
	mock *MockWaiter
}

// NewMockWaiter creates a new mock instance
func NewMockWaiter(ctrl *gomock.Controller) *MockWaiter {
	mock := &MockWaiter{ctrl: ctrl}
	mock.recorder = &MockWaiterMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockWaiter) EXPECT() *MockWaiterMockRecorder {
	return m.recorder
}

// WaitWithContext mocks base method
func (m *MockWaiter) WaitWithContext(arg0 context.Context) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "WaitWithContext", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// WaitWithContext indicates an expected call of WaitWithContext
func (mr *MockWaiterMockRecorder) WaitWithContext(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "WaitWithContext", reflect.TypeOf((*MockWaiter)(nil).WaitWithContext), arg0)
}
