package cache

import (
	"fmt"

	"github.com/rs/zerolog"

	"github.com/onflow/flow-go/model/flow"
	"github.com/onflow/flow-go/module"
)

// ClusterPrefixedMessagesReceivedTracker struct that keeps track of the amount of cluster prefixed control messages received by a peer.
type ClusterPrefixedMessagesReceivedTracker struct {
	cache *RecordCache
}

// NewClusterPrefixedMessagesReceivedTracker returns a new *ClusterPrefixedMessagesReceivedTracker.
func NewClusterPrefixedMessagesReceivedTracker(logger zerolog.Logger, sizeLimit uint32, clusterPrefixedCacheCollector module.HeroCacheMetrics, decay float64) (*ClusterPrefixedMessagesReceivedTracker, error) {
	config := &RecordCacheConfig{
		sizeLimit:   sizeLimit,
		logger:      logger,
		collector:   clusterPrefixedCacheCollector,
		recordDecay: decay,
	}
	recordCache, err := NewRecordCache(config, NewRecordEntity)
	if err != nil {
		return nil, fmt.Errorf("failed to create new record cahe: %w", err)
	}
	return &ClusterPrefixedMessagesReceivedTracker{cache: recordCache}, nil
}

// Inc increments the cluster prefixed control messages received Counter for the peer.
func (c *ClusterPrefixedMessagesReceivedTracker) Inc(nodeID flow.Identifier) (float64, error) {
	count, err := c.cache.Update(nodeID)
	if err != nil {
		return 0, fmt.Errorf("failed to increment cluster prefixed received tracker Counter for peer %s: %w", nodeID, err)
	}
	return count, nil
}

// Load loads the current number of cluster prefixed control messages received by a peer.
func (c *ClusterPrefixedMessagesReceivedTracker) Load(nodeID flow.Identifier) float64 {
	count, _, _ := c.cache.Get(nodeID)
	return count
}

// StoreActiveClusterIds stores the active cluster Ids in the underlying record cache.
func (c *ClusterPrefixedMessagesReceivedTracker) StoreActiveClusterIds(clusterIdList flow.ChainIDList) {
	c.cache.storeActiveClusterIds(clusterIdList)
}

// GetActiveClusterIds gets the active cluster Ids from the underlying record cache.
func (c *ClusterPrefixedMessagesReceivedTracker) GetActiveClusterIds() flow.ChainIDList {
	return c.cache.getActiveClusterIds()
}
