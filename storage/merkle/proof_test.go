package merkle

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProofWithASingleKey tests proof generation and verification
// when trie includes only a single value
func TestProofWithASingleKey(t *testing.T) {

	keyLength := 32
	tree1, err := NewTree(keyLength)
	assert.NoError(t, err)

	key, val := randomKeyValuePair(32, 128)

	replaced, err := tree1.Put(key, val)
	assert.NoError(t, err)
	require.False(t, replaced)

	// work for an existing key
	proof, existed := tree1.Prove(key)
	require.True(t, existed)

	err = proof.Verify(tree1.Hash())
	assert.NoError(t, err)

	// fail for non-existing key
	key2, _ := randomKeyValuePair(32, 128)

	proof, existed = tree1.Prove(key2)
	require.False(t, existed)
	require.Nil(t, proof)
}

// TestValidateFormat tests cases a proof can not be valid
func TestValidateFormat(t *testing.T) {

	// construct a valid proof
	keyLength := 32
	key := make([]byte, keyLength)
	key[0] = uint8(5)
	value := make([]byte, 128)
	value[0] = uint8(6)

	key2 := make([]byte, keyLength)
	key2[0] = uint8(4)
	value2 := make([]byte, 128)

	tree1, err := NewTree(keyLength)
	assert.NoError(t, err)
	replaced, err := tree1.Put(key, value)
	assert.NoError(t, err)
	require.False(t, replaced)
	replaced, err = tree1.Put(key2, value2)
	assert.NoError(t, err)
	require.False(t, replaced)
	proof, existed := tree1.Prove(key)
	require.True(t, existed)

	// invalid key size
	proof.Key = make([]byte, 0)
	err = proof.validateFormat()
	assert.Error(t, err)
	require.Equal(t, err.Error(), "malformed proof, key is empty")

	// invalid key size (too large)
	proof.Key = make([]byte, maxKeyLength+1)
	err = proof.validateFormat()
	assert.Error(t, err)
	require.Equal(t, err.Error(), "malformed proof, key length is larger than max key length allowed (8193 > 8192)")

	// issue with the key size not matching the rest of the proof
	proof.Key = make([]byte, 64)
	err = proof.validateFormat()
	assert.Error(t, err)
	require.Equal(t, err.Error(), "malformed proof, key length doesn't match the length of ShortPathLengths and SiblingHashes")

	// reset the key back to its original value
	proof.Key = key

	// empty InterimNodeTypes
	InterimNodeTypesBackup := proof.InterimNodeTypes
	proof.InterimNodeTypes = make([]byte, 0)
	err = proof.validateFormat()
	assert.Error(t, err)
	require.Equal(t, err.Error(), "malformed proof, InterimNodeTypes is empty")

	// too many interim nodes
	proof.InterimNodeTypes = make([]byte, maxKeyLength+1)
	err = proof.validateFormat()
	assert.Error(t, err)
	require.Equal(t, err.Error(), "malformed proof, InterimNodeTypes is larger than max key length allowed (8193 > 8192)")

	// issue with the size of InterimNodeTypes
	proof.InterimNodeTypes = append(InterimNodeTypesBackup, byte(0))
	err = proof.validateFormat()
	assert.Error(t, err)
	require.Equal(t, err.Error(), "malformed proof, the length of InterimNodeTypes doesn't match the length of ShortPathLengths and SiblingHashes")

	proof.InterimNodeTypes = InterimNodeTypesBackup

	// issue with a short count
	backupShortPathLengths := proof.ShortPathLengths
	proof.ShortPathLengths[0] = uint16(10)
	err = proof.validateFormat()
	assert.Error(t, err)
	require.Equal(t, err.Error(), "malformed proof, key length doesn't match the length of ShortPathLengths and SiblingHashes")

	// drop a shortpathlength - index out of bound
	proof.ShortPathLengths = proof.ShortPathLengths[:1]
	proof.ShortPathLengths[0] = uint16(255)
	err = proof.validateFormat()
	assert.Error(t, err)
	require.Equal(t, err.Error(), "malformed proof, not enough short path lengths are provided")
	proof.ShortPathLengths = backupShortPathLengths
}

// TestProofsWithRandomKeys tests proof generation and verification
// when trie includes many random keys. (only a random subset of keys are checked for proofs)
func TestProofsWithRandomKeys(t *testing.T) {
	// initialize random generator, two trees and zero hash
	rand.Seed(time.Now().UnixNano())
	keyLength := 32
	numberOfInsertions := 10000
	numberOfProofsToVerify := 100
	tree1, err := NewTree(keyLength)
	assert.NoError(t, err)

	// generate the desired number of keys and map a value to each key
	keys := make([][]byte, 0, numberOfInsertions)
	vals := make(map[string][]byte)
	for i := 0; i < numberOfInsertions; i++ {
		key, val := randomKeyValuePair(32, 128)
		keys = append(keys, key)
		vals[string(key)] = val
	}

	// insert all key-value paris into the first tree
	for _, key := range keys {
		val := vals[string(key)]
		replaced, err := tree1.Put(key, val)
		assert.NoError(t, err)
		require.False(t, replaced)
	}

	// shuffle the keys and insert them with random order into the second tree
	rand.Shuffle(len(keys), func(i int, j int) {
		keys[i], keys[j] = keys[j], keys[i]
	})

	// get proofs for keys and verify for a subset of keys
	for _, key := range keys[:numberOfProofsToVerify] {
		proof, existed := tree1.Prove(key)
		require.True(t, existed)
		err := proof.Verify(tree1.Hash())
		assert.NoError(t, err)
	}
}
