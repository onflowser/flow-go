package factories

import (
	"github.com/rs/zerolog"

	"github.com/onflow/flow-go/model/flow"
	"github.com/onflow/flow-go/module/chainsync"
	"github.com/onflow/flow-go/module/metrics"
)

type SyncCoreFactory struct {
	log  zerolog.Logger
	conf chainsync.Config
}

func NewSyncCoreFactory(
	log zerolog.Logger,
	conf chainsync.Config,
) (*SyncCoreFactory, error) {
	factory := &SyncCoreFactory{
		log:  log,
		conf: conf,
	}
	return factory, nil
}

func (f *SyncCoreFactory) Create(chainID flow.ChainID) (*chainsync.Core, error) {
	core, err := chainsync.New(f.log, f.conf, metrics.NewChainSyncCollector(chainID))
	if err != nil {
		return nil, err
	}
	return core, nil
}
