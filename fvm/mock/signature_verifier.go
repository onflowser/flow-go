// Code generated by mockery v1.0.0. DO NOT EDIT.

package mock

import (
	crypto "github.com/dapperlabs/flow-go/crypto"

	hash "github.com/dapperlabs/flow-go/crypto/hash"

	mock "github.com/stretchr/testify/mock"
)

// SignatureVerifier is an autogenerated mock type for the SignatureVerifier type
type SignatureVerifier struct {
	mock.Mock
}

// Verify provides a mock function with given fields: signature, tag, message, publicKey, hashAlgo
func (_m *SignatureVerifier) Verify(signature []byte, tag []byte, message []byte, publicKey crypto.PublicKey, hashAlgo hash.HashingAlgorithm) (bool, error) {
	ret := _m.Called(signature, tag, message, publicKey, hashAlgo)

	var r0 bool
	if rf, ok := ret.Get(0).(func([]byte, []byte, []byte, crypto.PublicKey, hash.HashingAlgorithm) bool); ok {
		r0 = rf(signature, tag, message, publicKey, hashAlgo)
	} else {
		r0 = ret.Get(0).(bool)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func([]byte, []byte, []byte, crypto.PublicKey, hash.HashingAlgorithm) error); ok {
		r1 = rf(signature, tag, message, publicKey, hashAlgo)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
