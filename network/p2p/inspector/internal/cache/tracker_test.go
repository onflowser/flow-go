package cache

import (
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/onflow/flow-go/model/flow"
	"github.com/onflow/flow-go/module/metrics"
	"github.com/onflow/flow-go/utils/unittest"
)

// TestClusterPrefixedMessagesReceivedTracker_Inc ensures cluster prefixed received tracker increments a counter correctly.
func TestClusterPrefixedMessagesReceivedTracker_Inc(t *testing.T) {
	tracker := mockTracker(t)
	id := unittest.IdentifierFixture()
	n := float64(5)
	for i := float64(1); i <= n; i++ {
		j, err := tracker.Inc(id)
		require.NoError(t, err)
		require.Equal(t, i, j)
	}
}

// TestClusterPrefixedMessagesReceivedTracker_IncConcurrent ensures cluster prefixed received tracker increments a counter correctly concurrently.
func TestClusterPrefixedMessagesReceivedTracker_IncConcurrent(t *testing.T) {
	tracker := mockTracker(t)
	n := float64(5)
	id := unittest.IdentifierFixture()
	var wg sync.WaitGroup
	wg.Add(5)
	for i := float64(0); i < n; i++ {
		go func() {
			defer wg.Done()
			_, err := tracker.Inc(id)
			require.NoError(t, err)
		}()
	}
	unittest.RequireReturnsBefore(t, wg.Wait, 100*time.Millisecond, "timed out waiting for goroutines to finish")
	require.Equal(t, n, tracker.Load(id))
}

// TestClusterPrefixedMessagesReceivedTracker_ConcurrentIncAndLoad ensures cluster prefixed received tracker increments/loads a counter correctly concurrently.
func TestClusterPrefixedMessagesReceivedTracker_ConcurrentIncAndLoad(t *testing.T) {
	tracker := mockTracker(t)
	n := float64(5)
	id := unittest.IdentifierFixture()
	var wg sync.WaitGroup
	wg.Add(10)
	go func() {
		for i := float64(0); i < n; i++ {
			go func() {
				defer wg.Done()
				_, err := tracker.Inc(id)
				require.NoError(t, err)
			}()
		}
	}()
	go func() {
		for i := float64(0); i < n; i++ {
			go func() {
				defer wg.Done()
				j := tracker.Load(id)
				require.NotNil(t, j)
			}()
		}
	}()
	unittest.RequireReturnsBefore(t, wg.Wait, 100*time.Millisecond, "timed out waiting for goroutines to finish")
	require.Equal(t, float64(5), tracker.Load(id))
}

func TestClusterPrefixedMessagesReceivedTracker_StoreAndGetActiveClusterIds(t *testing.T) {
	tracker := mockTracker(t)
	activeClusterIds := []flow.ChainIDList{chainIDListFixture(), chainIDListFixture(), chainIDListFixture()}
	for _, chainIDList := range activeClusterIds {
		tracker.StoreActiveClusterIds(chainIDList)
		actualChainIdList := tracker.GetActiveClusterIds()
		require.Equal(t, chainIDList, actualChainIdList)
	}
}

func TestClusterPrefixedMessagesReceivedTracker_StoreAndGetActiveClusterIdsConcurrent(t *testing.T) {
	tracker := mockTracker(t)
	activeClusterIds := []flow.ChainIDList{chainIDListFixture(), chainIDListFixture(), chainIDListFixture()}
	expectedLen := len(activeClusterIds[0])
	var wg sync.WaitGroup
	wg.Add(len(activeClusterIds))
	for _, chainIDList := range activeClusterIds {
		go func(ids flow.ChainIDList) {
			defer wg.Done()
			tracker.StoreActiveClusterIds(ids)
			actualChainIdList := tracker.GetActiveClusterIds()
			require.NotNil(t, actualChainIdList)
			require.Equal(t, expectedLen, len(actualChainIdList)) // each fixture is of the same len
		}(chainIDList)
	}

	unittest.RequireReturnsBefore(t, wg.Wait, 100*time.Millisecond, "timed out waiting for goroutines to finish")

	actualChainIdList := tracker.GetActiveClusterIds()
	require.NotNil(t, actualChainIdList)
	require.Equal(t, expectedLen, len(actualChainIdList)) // each fixture is of the same len
}

func mockTracker(t *testing.T) *ClusterPrefixedMessagesReceivedTracker {
	logger := zerolog.Nop()
	sizeLimit := uint32(100)
	collector := metrics.NewNoopCollector()
	decay := float64(0)
	tracker, err := NewClusterPrefixedMessagesReceivedTracker(logger, sizeLimit, collector, decay)
	require.NoError(t, err)
	return tracker
}

func chainIDListFixture() flow.ChainIDList {
	return flow.ChainIDList{
		flow.ChainID(unittest.IdentifierFixture().String()),
		flow.ChainID(unittest.IdentifierFixture().String()),
		flow.ChainID(unittest.IdentifierFixture().String()),
		flow.ChainID(unittest.IdentifierFixture().String()),
	}
}
