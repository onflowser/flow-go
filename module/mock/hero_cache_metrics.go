// Code generated by mockery v2.12.1. DO NOT EDIT.

package mock

import (
	mock "github.com/stretchr/testify/mock"

	testing "testing"
)

// HeroCacheMetrics is an autogenerated mock type for the HeroCacheMetrics type
type HeroCacheMetrics struct {
	mock.Mock
}

// BucketAvailableSlots provides a mock function with given fields: _a0, _a1
func (_m *HeroCacheMetrics) BucketAvailableSlots(_a0 uint64, _a1 uint64) {
	_m.Called(_a0, _a1)
}

// OnEntityEjectionDueToEmergency provides a mock function with given fields:
func (_m *HeroCacheMetrics) OnEntityEjectionDueToEmergency() {
	_m.Called()
}

// OnEntityEjectionDueToFullCapacity provides a mock function with given fields:
func (_m *HeroCacheMetrics) OnEntityEjectionDueToFullCapacity() {
	_m.Called()
}

// OnKeyGetFailure provides a mock function with given fields:
func (_m *HeroCacheMetrics) OnKeyGetFailure() {
	_m.Called()
}

// OnKeyGetSuccess provides a mock function with given fields:
func (_m *HeroCacheMetrics) OnKeyGetSuccess() {
	_m.Called()
}

// OnKeyPutFailure provides a mock function with given fields:
func (_m *HeroCacheMetrics) OnKeyPutFailure() {
	_m.Called()
}

// OnKeyPutSuccess provides a mock function with given fields:
func (_m *HeroCacheMetrics) OnKeyPutSuccess() {
	_m.Called()
}

// NewHeroCacheMetrics creates a new instance of HeroCacheMetrics. It also registers the testing.TB interface on the mock and a cleanup function to assert the mocks expectations.
func NewHeroCacheMetrics(t testing.TB) *HeroCacheMetrics {
	mock := &HeroCacheMetrics{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
