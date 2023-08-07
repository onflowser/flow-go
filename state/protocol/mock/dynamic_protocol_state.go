// Code generated by mockery v2.21.4. DO NOT EDIT.

package mock

import (
	flow "github.com/onflow/flow-go/model/flow"
	mock "github.com/stretchr/testify/mock"

	protocol "github.com/onflow/flow-go/state/protocol"
)

// DynamicProtocolState is an autogenerated mock type for the DynamicProtocolState type
type DynamicProtocolState struct {
	mock.Mock
}

// Clustering provides a mock function with given fields:
func (_m *DynamicProtocolState) Clustering() (flow.ClusterList, error) {
	ret := _m.Called()

	var r0 flow.ClusterList
	var r1 error
	if rf, ok := ret.Get(0).(func() (flow.ClusterList, error)); ok {
		return rf()
	}
	if rf, ok := ret.Get(0).(func() flow.ClusterList); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(flow.ClusterList)
		}
	}

	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// DKG provides a mock function with given fields:
func (_m *DynamicProtocolState) DKG() (protocol.DKG, error) {
	ret := _m.Called()

	var r0 protocol.DKG
	var r1 error
	if rf, ok := ret.Get(0).(func() (protocol.DKG, error)); ok {
		return rf()
	}
	if rf, ok := ret.Get(0).(func() protocol.DKG); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(protocol.DKG)
		}
	}

	if rf, ok := ret.Get(1).(func() error); ok {
		r1 = rf()
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Entry provides a mock function with given fields:
func (_m *DynamicProtocolState) Entry() *flow.ProtocolStateEntry {
	ret := _m.Called()

	var r0 *flow.ProtocolStateEntry
	if rf, ok := ret.Get(0).(func() *flow.ProtocolStateEntry); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*flow.ProtocolStateEntry)
		}
	}

	return r0
}

// Epoch provides a mock function with given fields:
func (_m *DynamicProtocolState) Epoch() uint64 {
	ret := _m.Called()

	var r0 uint64
	if rf, ok := ret.Get(0).(func() uint64); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(uint64)
	}

	return r0
}

// EpochCommit provides a mock function with given fields:
func (_m *DynamicProtocolState) EpochCommit() *flow.EpochCommit {
	ret := _m.Called()

	var r0 *flow.EpochCommit
	if rf, ok := ret.Get(0).(func() *flow.EpochCommit); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*flow.EpochCommit)
		}
	}

	return r0
}

// EpochSetup provides a mock function with given fields:
func (_m *DynamicProtocolState) EpochSetup() *flow.EpochSetup {
	ret := _m.Called()

	var r0 *flow.EpochSetup
	if rf, ok := ret.Get(0).(func() *flow.EpochSetup); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*flow.EpochSetup)
		}
	}

	return r0
}

// EpochStatus provides a mock function with given fields:
func (_m *DynamicProtocolState) EpochStatus() *flow.EpochStatus {
	ret := _m.Called()

	var r0 *flow.EpochStatus
	if rf, ok := ret.Get(0).(func() *flow.EpochStatus); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*flow.EpochStatus)
		}
	}

	return r0
}

// GlobalParams provides a mock function with given fields:
func (_m *DynamicProtocolState) GlobalParams() protocol.GlobalParams {
	ret := _m.Called()

	var r0 protocol.GlobalParams
	if rf, ok := ret.Get(0).(func() protocol.GlobalParams); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(protocol.GlobalParams)
		}
	}

	return r0
}

// Identities provides a mock function with given fields:
func (_m *DynamicProtocolState) Identities() flow.IdentityList {
	ret := _m.Called()

	var r0 flow.IdentityList
	if rf, ok := ret.Get(0).(func() flow.IdentityList); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(flow.IdentityList)
		}
	}

	return r0
}

type mockConstructorTestingTNewDynamicProtocolState interface {
	mock.TestingT
	Cleanup(func())
}

// NewDynamicProtocolState creates a new instance of DynamicProtocolState. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewDynamicProtocolState(t mockConstructorTestingTNewDynamicProtocolState) *DynamicProtocolState {
	mock := &DynamicProtocolState{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
