// Code generated by mockery v2.21.4. DO NOT EDIT.

package mock

import mock "github.com/stretchr/testify/mock"

// FinalizedExecutionState is an autogenerated mock type for the FinalizedExecutionState type
type FinalizedExecutionState struct {
	mock.Mock
}

// GetHighestFinalizedExecuted provides a mock function with given fields:
func (_m *FinalizedExecutionState) GetHighestFinalizedExecuted() (uint64, error) {
	ret := _m.Called()

	var r0 uint64
	var r1 error
	if rf, ok := ret.Get(0).(func() (uint64, error)); ok {
		return rf()
	}
	if rf, ok := ret.Get(0).(func() uint64); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(uint64)
	}

	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

type mockConstructorTestingTNewFinalizedExecutionState interface {
	mock.TestingT
	Cleanup(func())
}

// NewFinalizedExecutionState creates a new instance of FinalizedExecutionState. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewFinalizedExecutionState(t mockConstructorTestingTNewFinalizedExecutionState) *FinalizedExecutionState {
	mock := &FinalizedExecutionState{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
