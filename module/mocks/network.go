// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/dapperlabs/flow-go/module (interfaces: Network,Local)

// Package mocks is a generated GoMock package.
package mocks

import (
	flow "github.com/dapperlabs/flow-go/model/flow"
	network "github.com/dapperlabs/flow-go/network"
	gomock "github.com/golang/mock/gomock"
	reflect "reflect"
)

// MockNetwork is a mock of Network interface
type MockNetwork struct {
	ctrl     *gomock.Controller
	recorder *MockNetworkMockRecorder
}

// MockNetworkMockRecorder is the mock recorder for MockNetwork
type MockNetworkMockRecorder struct {
	mock *MockNetwork
}

// NewMockNetwork creates a new mock instance
func NewMockNetwork(ctrl *gomock.Controller) *MockNetwork {
	mock := &MockNetwork{ctrl: ctrl}
	mock.recorder = &MockNetworkMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockNetwork) EXPECT() *MockNetworkMockRecorder {
	return m.recorder
}

// Register mocks base method
func (m *MockNetwork) Register(arg0 byte, arg1 network.Engine) (network.Conduit, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Register", arg0, arg1)
	ret0, _ := ret[0].(network.Conduit)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Register indicates an expected call of Register
func (mr *MockNetworkMockRecorder) Register(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Register", reflect.TypeOf((*MockNetwork)(nil).Register), arg0, arg1)
}

// MockLocal is a mock of Local interface
type MockLocal struct {
	ctrl     *gomock.Controller
	recorder *MockLocalMockRecorder
}

// MockLocalMockRecorder is the mock recorder for MockLocal
type MockLocalMockRecorder struct {
	mock *MockLocal
}

// NewMockLocal creates a new mock instance
func NewMockLocal(ctrl *gomock.Controller) *MockLocal {
	mock := &MockLocal{ctrl: ctrl}
	mock.recorder = &MockLocalMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockLocal) EXPECT() *MockLocalMockRecorder {
	return m.recorder
}

// Address mocks base method
func (m *MockLocal) Address() string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Address")
	ret0, _ := ret[0].(string)
	return ret0
}

// Address indicates an expected call of Address
func (mr *MockLocalMockRecorder) Address() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Address", reflect.TypeOf((*MockLocal)(nil).Address))
}

// NodeID mocks base method
func (m *MockLocal) NodeID() flow.Identifier {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NodeID")
	ret0, _ := ret[0].(flow.Identifier)
	return ret0
}

// NodeID indicates an expected call of NodeID
func (mr *MockLocalMockRecorder) NodeID() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NodeID", reflect.TypeOf((*MockLocal)(nil).NodeID))
}
