// Code generated by mockery v1.0.0. DO NOT EDIT.

package mock

import flow "github.com/dapperlabs/flow-go/model/flow"
import mock "github.com/stretchr/testify/mock"

// Finalizer is an autogenerated mock type for the Finalizer type
type Finalizer struct {
	mock.Mock
}

// MakeFinal provides a mock function with given fields: blockID
func (_m *Finalizer) MakeFinal(blockID flow.Identifier) error {
	ret := _m.Called(blockID)

	var r0 error
	if rf, ok := ret.Get(0).(func(flow.Identifier) error); ok {
		r0 = rf(blockID)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// MakePending provides a mock function with given fields: blockID
func (_m *Finalizer) MakePending(blockID flow.Identifier) error {
	ret := _m.Called(blockID)

	var r0 error
	if rf, ok := ret.Get(0).(func(flow.Identifier) error); ok {
		r0 = rf(blockID)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}
