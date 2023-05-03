// Code generated by mockery v2.21.4. DO NOT EDIT.

package mock

import (
	chainsync "github.com/onflow/flow-go/model/chainsync"
	flow "github.com/onflow/flow-go/model/flow"

	mock "github.com/stretchr/testify/mock"
)

// SyncCore is an autogenerated mock type for the SyncCore type
type SyncCore struct {
	mock.Mock
}

// BatchRequested provides a mock function with given fields: batch
func (_m *SyncCore) BatchRequested(batch chainsync.Batch) {
	_m.Called(batch)
}

// HandleBlock provides a mock function with given fields: header
func (_m *SyncCore) HandleBlock(header *flow.Header) bool {
	ret := _m.Called(header)

	var r0 bool
	if rf, ok := ret.Get(0).(func(*flow.Header) bool); ok {
		r0 = rf(header)
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// HandleHeight provides a mock function with given fields: final, height
func (_m *SyncCore) HandleHeight(final *flow.Header, height uint64) {
	_m.Called(final, height)
}

// RangeRequested provides a mock function with given fields: ran
func (_m *SyncCore) RangeRequested(ran chainsync.Range) {
	_m.Called(ran)
}

// ScanPending provides a mock function with given fields: final
func (_m *SyncCore) ScanPending(final *flow.Header) ([]chainsync.Range, []chainsync.Batch) {
	ret := _m.Called(final)

	var r0 []chainsync.Range
	var r1 []chainsync.Batch
	if rf, ok := ret.Get(0).(func(*flow.Header) ([]chainsync.Range, []chainsync.Batch)); ok {
		return rf(final)
	}
	if rf, ok := ret.Get(0).(func(*flow.Header) []chainsync.Range); ok {
		r0 = rf(final)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]chainsync.Range)
		}
	}

	if rf, ok := ret.Get(1).(func(*flow.Header) []chainsync.Batch); ok {
		r1 = rf(final)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).([]chainsync.Batch)
		}
	}

	return r0, r1
}

// WithinTolerance provides a mock function with given fields: final, height
func (_m *SyncCore) WithinTolerance(final *flow.Header, height uint64) bool {
	ret := _m.Called(final, height)

	var r0 bool
	if rf, ok := ret.Get(0).(func(*flow.Header, uint64) bool); ok {
		r0 = rf(final, height)
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

type mockConstructorTestingTNewSyncCore interface {
	mock.TestingT
	Cleanup(func())
}

// NewSyncCore creates a new instance of SyncCore. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewSyncCore(t mockConstructorTestingTNewSyncCore) *SyncCore {
	mock := &SyncCore{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
