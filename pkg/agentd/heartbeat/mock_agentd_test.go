// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/Nuvoloso/kontroller/pkg/agentd (interfaces: AppServant)

package heartbeat

import (
	context "context"
	agentd "github.com/Nuvoloso/kontroller/pkg/agentd"
	models "github.com/Nuvoloso/kontroller/pkg/autogen/models"
	mgmtclient "github.com/Nuvoloso/kontroller/pkg/mgmtclient"
	gomock "github.com/golang/mock/gomock"
	reflect "reflect"
)

// MockAppServant is a mock of AppServant interface
type MockAppServant struct {
	ctrl     *gomock.Controller
	recorder *MockAppServantMockRecorder
}

// MockAppServantMockRecorder is the mock recorder for MockAppServant
type MockAppServantMockRecorder struct {
	mock *MockAppServant
}

// NewMockAppServant creates a new mock instance
func NewMockAppServant(ctrl *gomock.Controller) *MockAppServant {
	mock := &MockAppServant{ctrl: ctrl}
	mock.recorder = &MockAppServantMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockAppServant) EXPECT() *MockAppServantMockRecorder {
	return m.recorder
}

// AddLUN mocks base method
func (m *MockAppServant) AddLUN(arg0 *agentd.LUN) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "AddLUN", arg0)
}

// AddLUN indicates an expected call of AddLUN
func (mr *MockAppServantMockRecorder) AddLUN(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AddLUN", reflect.TypeOf((*MockAppServant)(nil).AddLUN), arg0)
}

// AddStorage mocks base method
func (m *MockAppServant) AddStorage(arg0 *agentd.Storage) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "AddStorage", arg0)
}

// AddStorage indicates an expected call of AddStorage
func (mr *MockAppServantMockRecorder) AddStorage(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AddStorage", reflect.TypeOf((*MockAppServant)(nil).AddStorage), arg0)
}

// FatalError mocks base method
func (m *MockAppServant) FatalError(arg0 error) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "FatalError", arg0)
}

// FatalError indicates an expected call of FatalError
func (mr *MockAppServantMockRecorder) FatalError(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FatalError", reflect.TypeOf((*MockAppServant)(nil).FatalError), arg0)
}

// FindLUN mocks base method
func (m *MockAppServant) FindLUN(arg0, arg1 string) *agentd.LUN {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FindLUN", arg0, arg1)
	ret0, _ := ret[0].(*agentd.LUN)
	return ret0
}

// FindLUN indicates an expected call of FindLUN
func (mr *MockAppServantMockRecorder) FindLUN(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FindLUN", reflect.TypeOf((*MockAppServant)(nil).FindLUN), arg0, arg1)
}

// FindStorage mocks base method
func (m *MockAppServant) FindStorage(arg0 string) *agentd.Storage {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FindStorage", arg0)
	ret0, _ := ret[0].(*agentd.Storage)
	return ret0
}

// FindStorage indicates an expected call of FindStorage
func (mr *MockAppServantMockRecorder) FindStorage(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FindStorage", reflect.TypeOf((*MockAppServant)(nil).FindStorage), arg0)
}

// GetCentraldAPIArgs mocks base method
func (m *MockAppServant) GetCentraldAPIArgs() *mgmtclient.APIArgs {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetCentraldAPIArgs")
	ret0, _ := ret[0].(*mgmtclient.APIArgs)
	return ret0
}

// GetCentraldAPIArgs indicates an expected call of GetCentraldAPIArgs
func (mr *MockAppServantMockRecorder) GetCentraldAPIArgs() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetCentraldAPIArgs", reflect.TypeOf((*MockAppServant)(nil).GetCentraldAPIArgs))
}

// GetClusterdAPI mocks base method
func (m *MockAppServant) GetClusterdAPI() (mgmtclient.API, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetClusterdAPI")
	ret0, _ := ret[0].(mgmtclient.API)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetClusterdAPI indicates an expected call of GetClusterdAPI
func (mr *MockAppServantMockRecorder) GetClusterdAPI() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetClusterdAPI", reflect.TypeOf((*MockAppServant)(nil).GetClusterdAPI))
}

// GetClusterdAPIArgs mocks base method
func (m *MockAppServant) GetClusterdAPIArgs() *mgmtclient.APIArgs {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetClusterdAPIArgs")
	ret0, _ := ret[0].(*mgmtclient.APIArgs)
	return ret0
}

// GetClusterdAPIArgs indicates an expected call of GetClusterdAPIArgs
func (mr *MockAppServantMockRecorder) GetClusterdAPIArgs() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetClusterdAPIArgs", reflect.TypeOf((*MockAppServant)(nil).GetClusterdAPIArgs))
}

// GetLUNs mocks base method
func (m *MockAppServant) GetLUNs() []*agentd.LUN {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetLUNs")
	ret0, _ := ret[0].([]*agentd.LUN)
	return ret0
}

// GetLUNs indicates an expected call of GetLUNs
func (mr *MockAppServantMockRecorder) GetLUNs() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetLUNs", reflect.TypeOf((*MockAppServant)(nil).GetLUNs))
}

// GetServicePlan mocks base method
func (m *MockAppServant) GetServicePlan(arg0 context.Context, arg1 string) (*models.ServicePlan, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetServicePlan", arg0, arg1)
	ret0, _ := ret[0].(*models.ServicePlan)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetServicePlan indicates an expected call of GetServicePlan
func (mr *MockAppServantMockRecorder) GetServicePlan(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetServicePlan", reflect.TypeOf((*MockAppServant)(nil).GetServicePlan), arg0, arg1)
}

// GetStorage mocks base method
func (m *MockAppServant) GetStorage() []*agentd.Storage {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetStorage")
	ret0, _ := ret[0].([]*agentd.Storage)
	return ret0
}

// GetStorage indicates an expected call of GetStorage
func (mr *MockAppServantMockRecorder) GetStorage() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetStorage", reflect.TypeOf((*MockAppServant)(nil).GetStorage))
}

// InitializeCSPClient mocks base method
func (m *MockAppServant) InitializeCSPClient(arg0 *models.CSPDomain) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "InitializeCSPClient", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// InitializeCSPClient indicates an expected call of InitializeCSPClient
func (mr *MockAppServantMockRecorder) InitializeCSPClient(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "InitializeCSPClient", reflect.TypeOf((*MockAppServant)(nil).InitializeCSPClient), arg0)
}

// InitializeNuvo mocks base method
func (m *MockAppServant) InitializeNuvo(arg0 context.Context) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "InitializeNuvo", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// InitializeNuvo indicates an expected call of InitializeNuvo
func (mr *MockAppServantMockRecorder) InitializeNuvo(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "InitializeNuvo", reflect.TypeOf((*MockAppServant)(nil).InitializeNuvo), arg0)
}

// IsReady mocks base method
func (m *MockAppServant) IsReady() bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IsReady")
	ret0, _ := ret[0].(bool)
	return ret0
}

// IsReady indicates an expected call of IsReady
func (mr *MockAppServantMockRecorder) IsReady() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsReady", reflect.TypeOf((*MockAppServant)(nil).IsReady))
}

// RemoveLUN mocks base method
func (m *MockAppServant) RemoveLUN(arg0, arg1 string) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "RemoveLUN", arg0, arg1)
}

// RemoveLUN indicates an expected call of RemoveLUN
func (mr *MockAppServantMockRecorder) RemoveLUN(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RemoveLUN", reflect.TypeOf((*MockAppServant)(nil).RemoveLUN), arg0, arg1)
}

// RemoveStorage mocks base method
func (m *MockAppServant) RemoveStorage(arg0 string) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "RemoveStorage", arg0)
}

// RemoveStorage indicates an expected call of RemoveStorage
func (mr *MockAppServantMockRecorder) RemoveStorage(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RemoveStorage", reflect.TypeOf((*MockAppServant)(nil).RemoveStorage), arg0)
}