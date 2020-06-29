// Code generated by mockery v1.0.0. DO NOT EDIT.

package mock

import (
	fvm "github.com/dapperlabs/flow-go/fvm"
	mock "github.com/stretchr/testify/mock"
)

// Invokable is an autogenerated mock type for the Invokable type
type Invokable struct {
	mock.Mock
}

// Invoke provides a mock function with given fields: ctx, ledger
func (_m *Invokable) Invoke(ctx fvm.Context, ledger fvm.Ledger) (*fvm.InvocationResult, error) {
	ret := _m.Called(ctx, ledger)

	var r0 *fvm.InvocationResult
	if rf, ok := ret.Get(0).(func(fvm.Context, fvm.Ledger) *fvm.InvocationResult); ok {
		r0 = rf(ctx, ledger)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*fvm.InvocationResult)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(fvm.Context, fvm.Ledger) error); ok {
		r1 = rf(ctx, ledger)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Parse provides a mock function with given fields: ctx, ledger
func (_m *Invokable) Parse(ctx fvm.Context, ledger fvm.Ledger) (fvm.Invokable, error) {
	ret := _m.Called(ctx, ledger)

	var r0 fvm.Invokable
	if rf, ok := ret.Get(0).(func(fvm.Context, fvm.Ledger) fvm.Invokable); ok {
		r0 = rf(ctx, ledger)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(fvm.Invokable)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(fvm.Context, fvm.Ledger) error); ok {
		r1 = rf(ctx, ledger)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
