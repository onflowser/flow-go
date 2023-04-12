package scoring

import (
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/rs/zerolog"

	"github.com/onflow/flow-go/module"
	"github.com/onflow/flow-go/module/metrics"
	"github.com/onflow/flow-go/network/channels"
	"github.com/onflow/flow-go/network/p2p"
	netcache "github.com/onflow/flow-go/network/p2p/cache"
	"github.com/onflow/flow-go/utils/logging"
)

const (
	DefaultAppSpecificScoreWeight = 1
	MaxAppSpecificPenalty         = -100
	MinAppSpecificPenalty         = -1
	MaxAppSpecificReward          = float64(100)

	// DefaultGossipThreshold when a peer's penalty drops below this threshold,
	// no gossip is emitted towards that peer and gossip from that peer is ignored.
	//
	// Validation Constraint: GossipThreshold >= PublishThreshold && GossipThreshold < 0
	//
	// How we use it:
	// As current max penalty is -100, we set the threshold to -99 so that all gossips
	// to and from peers with penalty -100 are ignored.
	DefaultGossipThreshold = -99

	// DefaultPublishThreshold when a peer's penalty drops below this threshold,
	// self-published messages are not propagated towards this peer.
	//
	// Validation Constraint:
	// PublishThreshold >= GraylistThreshold && PublishThreshold <= GossipThreshold && PublishThreshold < 0.
	//
	// How we use it:
	// As current max penalty is -100, we set the threshold to -99 so that all penalized peers are deprived of
	// receiving any published messages.
	DefaultPublishThreshold = -99

	// DefaultGraylistThreshold when a peer's penalty drops below this threshold, the peer is graylisted, i.e.,
	// incoming RPCs from the peer are ignored.
	//
	// Validation Constraint:
	// GraylistThreshold =< PublishThreshold && GraylistThreshold =< GossipThreshold && GraylistThreshold < 0
	//
	// How we use it:
	// As current max penalty is -100, we set the threshold to -99 so that all penalized peers are graylisted.
	DefaultGraylistThreshold = -99

	// DefaultAcceptPXThreshold when a peer sends us PX information with a prune, we only accept it and connect to the supplied
	// peers if the originating peer's penalty exceeds this threshold.
	//
	// Validation Constraint: must be non-negative.
	//
	// How we use it:
	// As current max reward is 100, we set the threshold to 99 so that we only receive supplied peers from
	// well-behaved peers.
	DefaultAcceptPXThreshold = 99

	// DefaultOpportunisticGraftThreshold when the median peer penalty in the mesh drops below this value,
	// the peer may select more peers with penalty above the median to opportunistically graft on the mesh.
	//
	// Validation Constraint: must be non-negative.
	//
	// How we use it:
	// We set it to the MaxAppSpecificReward + 1 so that we only opportunistically graft peers that are not access nodes (i.e., with MinAppSpecificPenalty),
	// or penalized peers (i.e., with MaxAppSpecificPenalty).
	DefaultOpportunisticGraftThreshold = MaxAppSpecificReward + 1

	// MaxDebugLogs sets the max number of debug/trace log events per second. Logs emitted above
	// this threshold are dropped.
	MaxDebugLogs = 50

	// defaultScoreCacheSize is the default size of the cache used to store the app specific penalty of peers.
	defaultScoreCacheSize = 1000
)

// ScoreOption is a functional option for configuring the peer scoring system.
type ScoreOption struct {
	logger zerolog.Logger

	peerScoreParams     *pubsub.PeerScoreParams
	peerThresholdParams *pubsub.PeerScoreThresholds
	validator           p2p.SubscriptionValidator
	appScoreFunc        func(peer.ID) float64
}

type ScoreOptionConfig struct {
	logger       zerolog.Logger
	provider     module.IdentityProvider
	cacheSize    uint32
	cacheMetrics module.HeroCacheMetrics
	appScoreFunc func(peer.ID) float64
	topicParams  []func(map[string]*pubsub.TopicScoreParams)
}

func NewScoreOptionConfig(logger zerolog.Logger) *ScoreOptionConfig {
	return &ScoreOptionConfig{
		logger:       logger,
		cacheSize:    defaultScoreCacheSize,
		cacheMetrics: metrics.NewNoopCollector(), // no metrics by default
		topicParams:  make([]func(map[string]*pubsub.TopicScoreParams), 0),
	}
}

// SetProvider sets the identity provider for the penalty option.
// It is used to retrieve the identity of a peer when calculating the app specific penalty.
// If the provider is not set, the penalty registry will crash. This is a required field.
// It is safe to call this method multiple times, the last call will be used.
func (c *ScoreOptionConfig) SetProvider(provider module.IdentityProvider) {
	c.provider = provider
}

// SetCacheSize sets the size of the cache used to store the app specific penalty of peers.
// If the cache size is not set, the default value will be used.
// It is safe to call this method multiple times, the last call will be used.
func (c *ScoreOptionConfig) SetCacheSize(size uint32) {
	c.cacheSize = size
}

// SetCacheMetrics sets the cache metrics collector for the penalty option.
// It is used to collect metrics for the app specific penalty cache. If the cache metrics collector is not set,
// a no-op collector will be used.
// It is safe to call this method multiple times, the last call will be used.
func (c *ScoreOptionConfig) SetCacheMetrics(metrics module.HeroCacheMetrics) {
	c.cacheMetrics = metrics
}

// SetAppSpecificScoreFunction sets the app specific penalty function for the penalty option.
// It is used to calculate the app specific penalty of a peer.
// If the app specific penalty function is not set, the default one is used.
// Note that it is always safer to use the default one, unless you know what you are doing.
// It is safe to call this method multiple times, the last call will be used.
func (c *ScoreOptionConfig) SetAppSpecificScoreFunction(appSpecificScoreFunction func(peer.ID) float64) {
	c.appScoreFunc = appSpecificScoreFunction
}

// SetTopicScoreParams adds the topic penalty parameters to the peer penalty parameters.
// It is used to configure the topic penalty parameters for the pubsub system.
// If there is already a topic penalty parameter for the given topic, the last call will be used.
func (c *ScoreOptionConfig) SetTopicScoreParams(topic channels.Topic, topicScoreParams *pubsub.TopicScoreParams) {
	c.topicParams = append(c.topicParams, func(topics map[string]*pubsub.TopicScoreParams) {
		topics[topic.String()] = topicScoreParams
	})
}

// NewScoreOption creates a new penalty option with the given configuration.
func NewScoreOption(cfg *ScoreOptionConfig) *ScoreOption {
	throttledSampler := logging.BurstSampler(MaxDebugLogs, time.Second)
	logger := cfg.logger.With().
		Str("module", "pubsub_score_option").
		Logger().
		Sample(zerolog.LevelSampler{
			TraceSampler: throttledSampler,
			DebugSampler: throttledSampler,
		})
	validator := NewSubscriptionValidator()
	scoreRegistry := NewGossipSubAppSpecificScoreRegistry(&GossipSubAppSpecificScoreRegistryConfig{
		Logger:        logger,
		DecayFunction: DefaultDecayFunction(),
		Penalty:       DefaultGossipSubCtrlMsgPenaltyValue(),
		Validator:     validator,
		Init:          InitAppScoreRecordState,
		CacheFactory: func() p2p.GossipSubSpamRecordCache {
			return netcache.NewGossipSubSpamRecordCache(cfg.cacheSize, cfg.logger, cfg.cacheMetrics, DefaultDecayFunction())
		},
	})
	s := &ScoreOption{
		logger:          logger,
		peerScoreParams: defaultPeerScoreParams(),
	}

	// set the app specific penalty function for the penalty option
	// if the app specific penalty function is not set, use the default one
	if cfg.appScoreFunc == nil {
		s.appScoreFunc = scoreRegistry.AppSpecificScoreFunc()
	} else {
		s.appScoreFunc = cfg.appScoreFunc
	}

	s.peerScoreParams.AppSpecificScore = s.appScoreFunc

	// apply the topic penalty parameters if any.
	for _, topicParams := range cfg.topicParams {
		topicParams(s.peerScoreParams.Topics)
	}

	return s
}

func (s *ScoreOption) SetSubscriptionProvider(provider *SubscriptionProvider) error {
	return s.validator.RegisterSubscriptionProvider(provider)
}

func (s *ScoreOption) BuildFlowPubSubScoreOption() pubsub.Option {
	s.preparePeerScoreThresholds()

	s.logger.Info().
		Float64("gossip_threshold", s.peerThresholdParams.GossipThreshold).
		Float64("publish_threshold", s.peerThresholdParams.PublishThreshold).
		Float64("graylist_threshold", s.peerThresholdParams.GraylistThreshold).
		Float64("accept_px_threshold", s.peerThresholdParams.AcceptPXThreshold).
		Float64("opportunistic_graft_threshold", s.peerThresholdParams.OpportunisticGraftThreshold).
		Msg("peer penalty thresholds configured")

	return pubsub.WithPeerScore(
		s.peerScoreParams,
		s.peerThresholdParams,
	)
}

func (s *ScoreOption) preparePeerScoreThresholds() {
	s.peerThresholdParams = &pubsub.PeerScoreThresholds{
		GossipThreshold:             DefaultGossipThreshold,
		PublishThreshold:            DefaultPublishThreshold,
		GraylistThreshold:           DefaultGraylistThreshold,
		AcceptPXThreshold:           DefaultAcceptPXThreshold,
		OpportunisticGraftThreshold: DefaultOpportunisticGraftThreshold,
	}
}

func defaultPeerScoreParams() *pubsub.PeerScoreParams {
	return &pubsub.PeerScoreParams{
		Topics: make(map[string]*pubsub.TopicScoreParams),
		// we don't set all the parameters, so we skip the atomic validation.
		// atomic validation fails initialization if any parameter is not set.
		SkipAtomicValidation: true,
		// DecayInterval is the interval over which we decay the effect of past behavior. So that
		// a good or bad behavior will not have a permanent effect on the penalty.
		DecayInterval: time.Hour,
		// DecayToZero defines the maximum value below which a peer scoring counter is reset to zero.
		// This is to prevent the counter from decaying to a very small value.
		// The default value is 0.01, which means that a counter will be reset to zero if it decays to 0.01.
		// When a counter hits the DecayToZero threshold, it means that the peer did not exhibit the behavior
		// for a long time, and we can reset the counter.
		DecayToZero: 0.01,
		// AppSpecificWeight is the weight of the application specific penalty.
		AppSpecificWeight: DefaultAppSpecificScoreWeight,
	}
}

func (s *ScoreOption) BuildGossipSubScoreOption() pubsub.Option {
	s.preparePeerScoreThresholds()

	s.logger.Info().
		Float64("gossip_threshold", s.peerThresholdParams.GossipThreshold).
		Float64("publish_threshold", s.peerThresholdParams.PublishThreshold).
		Float64("graylist_threshold", s.peerThresholdParams.GraylistThreshold).
		Float64("accept_px_threshold", s.peerThresholdParams.AcceptPXThreshold).
		Float64("opportunistic_graft_threshold", s.peerThresholdParams.OpportunisticGraftThreshold).
		Msg("peer penalty thresholds configured")

	return pubsub.WithPeerScore(
		s.peerScoreParams,
		s.peerThresholdParams,
	)
}
