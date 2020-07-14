package ingestion

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/dapperlabs/flow-go/consensus/hotstuff/model"
	"github.com/dapperlabs/flow-go/engine"
	"github.com/dapperlabs/flow-go/engine/access/rpc"
	"github.com/dapperlabs/flow-go/model/flow"
	"github.com/dapperlabs/flow-go/model/messages"

	module "github.com/dapperlabs/flow-go/module/mock"
	network "github.com/dapperlabs/flow-go/network/mock"
	protocol "github.com/dapperlabs/flow-go/state/protocol/mock"
	realstore "github.com/dapperlabs/flow-go/storage"
	storage "github.com/dapperlabs/flow-go/storage/mock"
	"github.com/dapperlabs/flow-go/utils/unittest"
)

type Suite struct {
	suite.Suite

	// protocol state
	proto struct {
		state    *protocol.State
		snapshot *protocol.Snapshot
		mutator  *protocol.Mutator
	}

	me           *module.Local
	net          *module.Network
	provider     *network.Engine
	blocks       *storage.Blocks
	headers      *storage.Headers
	collections  *storage.Collections
	transactions *storage.Transactions
	eng          *Engine

	// mock conduit for requesting/receiving collections
	collectionsConduit *network.Conduit
}

func TestIngestEngine(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (suite *Suite) SetupTest() {
	log := zerolog.New(os.Stderr)

	obsIdentity := unittest.IdentityFixture(unittest.WithRole(flow.RoleAccess))

	// mock out protocol state
	suite.proto.state = new(protocol.State)
	suite.proto.snapshot = new(protocol.Snapshot)
	suite.proto.state.On("Identity").Return(obsIdentity, nil)
	suite.proto.state.On("Final").Return(suite.proto.snapshot, nil)

	suite.me = new(module.Local)
	suite.me.On("NodeID").Return(obsIdentity.NodeID)

	suite.net = new(module.Network)
	suite.collectionsConduit = &network.Conduit{}
	suite.net.On("Register", uint8(engine.CollectionProvider), mock.Anything).
		Return(suite.collectionsConduit, nil).
		Once()

	suite.provider = new(network.Engine)
	suite.blocks = new(storage.Blocks)
	suite.headers = new(storage.Headers)
	suite.collections = new(storage.Collections)
	suite.transactions = new(storage.Transactions)

	rpcEng := rpc.New(log, suite.proto.state, rpc.Config{}, nil, nil, suite.blocks, suite.headers, suite.collections, suite.transactions, flow.Testnet)

	eng, err := New(log, suite.net, suite.proto.state, suite.me, suite.blocks, suite.headers, suite.collections, suite.transactions, rpcEng)
	require.NoError(suite.T(), err)
	suite.eng = eng

}

// TestHandleBlock checks that when a block is received, a request for each individual collection is made
func (suite *Suite) TestHandleBlock() {

	block := unittest.BlockFixture()

	cNodeIdentities := unittest.IdentityListFixture(1, unittest.WithRole(flow.RoleCollection))
	suite.proto.snapshot.On("Identities", mock.Anything).Return(cNodeIdentities, nil).Once()

	suite.blocks.On("ByID", block.ID()).Return(&block, nil).Once()

	// expect that the block storage is indexed with each of the collection guarantee
	suite.blocks.On("IndexBlockForCollections", block.ID(), flow.GetIDs(block.Payload.Guarantees)).Return(nil).Once()

	// expect that the collection is requested
	suite.collectionsConduit.On("Submit", mock.Anything, mock.Anything).Return(nil).Times(len(block.Payload.Guarantees))

	// create a model.Block with the block ID (other fields of model.block are not needed)
	modelBlock := model.Block{
		BlockID: block.ID(),
	}

	// simulate the follower engine calling the ingest engine with the model block
	suite.eng.OnFinalizedBlock(&modelBlock)

	done := suite.eng.unit.Done()
	assert.Eventually(suite.T(), func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)

	suite.proto.snapshot.AssertExpectations(suite.T())
	suite.headers.AssertExpectations(suite.T())
	suite.collectionsConduit.AssertExpectations(suite.T())
}

// TestHandleCollection checks that when a Collection is received, it is persisted
func (suite *Suite) TestHandleCollection() {
	originID := unittest.IdentifierFixture()
	collection := unittest.CollectionFixture(5)
	light := collection.Light()

	suite.collections.On("StoreLightAndIndexByTransaction", &light).Return(nil).Once()
	suite.transactions.On("Store", mock.Anything).Return(nil).Times(len(collection.Transactions))

	cr := messages.CollectionResponse{Collection: collection}
	err := suite.eng.Process(originID, &cr)

	require.NoError(suite.T(), err)
	suite.collections.AssertExpectations(suite.T())
	suite.transactions.AssertExpectations(suite.T())
}

// TestHandleDuplicateCollection checks that when a duplicate Collection is received, it is ignored
func (suite *Suite) TestHandleDuplicateCollection() {
	originID := unittest.IdentifierFixture()
	collection := unittest.CollectionFixture(5)
	light := collection.Light()

	error := fmt.Errorf("extra text: %w", realstore.ErrAlreadyExists)
	suite.collections.On("StoreLightAndIndexByTransaction", &light).Return(error).Once()

	cr := messages.CollectionResponse{Collection: collection}
	err := suite.eng.Process(originID, &cr)

	require.NoError(suite.T(), err)
	suite.collections.AssertExpectations(suite.T())
}
