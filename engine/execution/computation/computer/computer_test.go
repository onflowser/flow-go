package computer_test

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"github.com/onflow/cadence"
	"github.com/onflow/cadence/encoding/json"
	"github.com/onflow/cadence/runtime"
	"github.com/onflow/cadence/runtime/common"
	"github.com/onflow/cadence/runtime/interpreter"
	"github.com/onflow/cadence/runtime/sema"
	"github.com/onflow/cadence/runtime/stdlib"

	"github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/onflow/flow-go/engine/execution"
	"github.com/onflow/flow-go/engine/execution/computation/committer"
	"github.com/onflow/flow-go/engine/execution/computation/computer"
	computermock "github.com/onflow/flow-go/engine/execution/computation/computer/mock"
	"github.com/onflow/flow-go/engine/execution/state/delta"
	"github.com/onflow/flow-go/engine/execution/testutil"
	"github.com/onflow/flow-go/fvm"
	"github.com/onflow/flow-go/fvm/derived"
	"github.com/onflow/flow-go/fvm/environment"
	fvmErrors "github.com/onflow/flow-go/fvm/errors"
	reusableRuntime "github.com/onflow/flow-go/fvm/runtime"
	"github.com/onflow/flow-go/fvm/state"
	"github.com/onflow/flow-go/fvm/systemcontracts"
	"github.com/onflow/flow-go/ledger"
	"github.com/onflow/flow-go/model/convert/fixtures"
	"github.com/onflow/flow-go/model/flow"
	"github.com/onflow/flow-go/module"
	"github.com/onflow/flow-go/module/epochs"
	"github.com/onflow/flow-go/module/executiondatasync/execution_data"
	"github.com/onflow/flow-go/module/executiondatasync/provider"
	mocktracker "github.com/onflow/flow-go/module/executiondatasync/tracker/mock"
	"github.com/onflow/flow-go/module/mempool/entity"
	"github.com/onflow/flow-go/module/metrics"
	modulemock "github.com/onflow/flow-go/module/mock"
	requesterunit "github.com/onflow/flow-go/module/state_synchronization/requester/unittest"
	"github.com/onflow/flow-go/module/trace"
	"github.com/onflow/flow-go/utils/unittest"
)

func incStateCommitment(startState flow.StateCommitment) flow.StateCommitment {
	endState := flow.StateCommitment(startState)
	endState[0] += 1
	return endState
}

type fakeCommitter struct {
	callCount int
}

func (committer *fakeCommitter) CommitView(
	view state.View,
	startState flow.StateCommitment,
) (
	flow.StateCommitment,
	[]byte,
	*ledger.TrieUpdate,
	error,
) {
	committer.callCount++

	endState := incStateCommitment(startState)

	trieUpdate := &ledger.TrieUpdate{}
	trieUpdate.RootHash[0] = byte(committer.callCount)
	return endState,
		[]byte{byte(committer.callCount)},
		trieUpdate,
		nil
}

func TestBlockExecutor_ExecuteBlock(t *testing.T) {

	rag := &RandomAddressGenerator{}

	me := new(modulemock.Local)
	me.On("SignFunc", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, nil)

	t.Run("single collection", func(t *testing.T) {

		execCtx := fvm.NewContext(
			fvm.WithDerivedBlockData(derived.NewEmptyDerivedBlockData()),
		)

		vm := new(computermock.VirtualMachine)
		vm.On("Run", mock.Anything, mock.Anything, mock.Anything).
			Return(nil).
			Run(func(args mock.Arguments) {
				ctx := args[0].(fvm.Context)
				tx := args[1].(*fvm.TransactionProcedure)

				tx.Events = generateEvents(1, tx.TxIndex)

				getSetAProgram(t, ctx.DerivedBlockData)
			}).
			Times(2 + 1) // 2 txs in collection + system chunk

		committer := &fakeCommitter{
			callCount: 0,
		}

		exemetrics := new(modulemock.ExecutionMetrics)
		exemetrics.On("ExecutionCollectionExecuted",
			mock.Anything,  // duration
			mock.Anything). // stats
			Return(nil).
			Times(2) // 1 collection + system collection

		exemetrics.On("ExecutionTransactionExecuted",
			mock.Anything, // duration
			mock.Anything, // computation used
			mock.Anything, // memory used
			mock.Anything, // actual memory used
			mock.Anything, // number of events
			mock.Anything, // size of events
			false).        // no failure
			Return(nil).
			Times(2 + 1) // 2 txs in collection + system chunk tx

		exemetrics.On(
			"ExecutionChunkDataPackGenerated",
			mock.Anything,
			mock.Anything).
			Return(nil).
			Times(2) // 1 collection + system collection

		expectedProgramsInCache := 1 // we set one program in the cache
		exemetrics.On(
			"ExecutionBlockCachedPrograms",
			expectedProgramsInCache).
			Return(nil).
			Times(1) // 1 block

		bservice := requesterunit.MockBlobService(blockstore.NewBlockstore(dssync.MutexWrap(datastore.NewMapDatastore())))
		trackerStorage := mocktracker.NewMockStorage()

		prov := provider.NewProvider(
			zerolog.Nop(),
			metrics.NewNoopCollector(),
			execution_data.DefaultSerializer,
			bservice,
			trackerStorage,
		)

		exe, err := computer.NewBlockComputer(
			vm,
			execCtx,
			exemetrics,
			trace.NewNoopTracer(),
			zerolog.Nop(),
			committer,
			me,
			prov)
		require.NoError(t, err)

		// create a block with 1 collection with 2 transactions
		block := generateBlock(1, 2, rag)

		view := delta.NewDeltaView(nil)

		parentBlockExecutionResultID := unittest.IdentifierFixture()
		result, err := exe.ExecuteBlock(
			context.Background(),
			parentBlockExecutionResultID,
			block,
			view,
			derived.NewEmptyDerivedBlockData())
		assert.NoError(t, err)
		assert.Len(t, result.StateSnapshots, 1+1)      // +1 system chunk
		assert.Len(t, result.Chunks, 1+1)              // +1 system chunk
		assert.Len(t, result.ChunkDataPacks, 1+1)      // +1 system chunk
		assert.Len(t, result.ChunkExecutionDatas, 1+1) // +1 system chunk

		require.Equal(t, 2, committer.callCount)

		assert.Equal(t, block.ID(), result.BlockExecutionData.BlockID)

		// regular collection chunk
		chunk1 := result.Chunks[0]

		assert.Equal(t, block.ID(), chunk1.BlockID)
		assert.Equal(t, uint(0), chunk1.CollectionIndex)
		assert.Equal(t, uint64(2), chunk1.NumberOfTransactions)
		assert.Equal(t, result.EventsHashes[0], chunk1.EventCollection)

		assert.Equal(t, *block.StartState, chunk1.StartState)

		expectedChunk1EndState := incStateCommitment(*block.StartState)

		assert.NotEqual(t, *block.StartState, chunk1.EndState)
		assert.NotEqual(t, flow.DummyStateCommitment, chunk1.EndState)
		assert.Equal(t, expectedChunk1EndState, chunk1.EndState)

		chunkDataPack1 := result.ChunkDataPacks[0]

		assert.Equal(t, chunk1.ID(), chunkDataPack1.ChunkID)
		assert.Equal(t, *block.StartState, chunkDataPack1.StartState)
		assert.Equal(t, []byte{1}, chunkDataPack1.Proof)
		assert.NotNil(t, chunkDataPack1.Collection)

		chunkExecutionData1 := result.ChunkExecutionDatas[0]
		assert.Equal(
			t,
			chunkDataPack1.Collection,
			chunkExecutionData1.Collection)
		assert.NotNil(t, chunkExecutionData1.TrieUpdate)
		assert.Equal(t, byte(1), chunkExecutionData1.TrieUpdate.RootHash[0])

		// system chunk is special case, but currently also 1 tx
		chunk2 := result.Chunks[1]
		assert.Equal(t, block.ID(), chunk2.BlockID)
		assert.Equal(t, uint(1), chunk2.CollectionIndex)
		assert.Equal(t, uint64(1), chunk2.NumberOfTransactions)
		assert.Equal(t, result.EventsHashes[1], chunk2.EventCollection)

		assert.Equal(t, expectedChunk1EndState, chunk2.StartState)

		expectedChunk2EndState := incStateCommitment(expectedChunk1EndState)

		assert.NotEqual(t, *block.StartState, chunk2.EndState)
		assert.NotEqual(t, flow.DummyStateCommitment, chunk2.EndState)
		assert.NotEqual(t, expectedChunk1EndState, chunk2.EndState)
		assert.Equal(t, expectedChunk2EndState, chunk2.EndState)

		chunkDataPack2 := result.ChunkDataPacks[1]

		assert.Equal(t, chunk2.ID(), chunkDataPack2.ChunkID)
		assert.Equal(t, chunk2.StartState, chunkDataPack2.StartState)
		assert.Equal(t, []byte{2}, chunkDataPack2.Proof)
		assert.Nil(t, chunkDataPack2.Collection)

		chunkExecutionData2 := result.ChunkExecutionDatas[1]
		assert.NotNil(t, chunkExecutionData2.Collection)
		assert.NotNil(t, chunkExecutionData2.TrieUpdate)
		assert.Equal(t, byte(2), chunkExecutionData2.TrieUpdate.RootHash[0])

		assert.Equal(t, expectedChunk2EndState, result.EndState)

		assert.Equal(
			t,
			parentBlockExecutionResultID,
			result.ExecutionResult.PreviousResultID)

		assertEventHashesMatch(t, 1+1, result)

		assert.NotEqual(t, flow.ZeroID, result.ExecutionDataID)

		vm.AssertExpectations(t)
	})

	t.Run("empty block still computes system chunk", func(t *testing.T) {

		execCtx := fvm.NewContext()

		vm := new(computermock.VirtualMachine)
		committer := new(computermock.ViewCommitter)

		bservice := requesterunit.MockBlobService(blockstore.NewBlockstore(dssync.MutexWrap(datastore.NewMapDatastore())))
		trackerStorage := mocktracker.NewMockStorage()

		prov := provider.NewProvider(
			zerolog.Nop(),
			metrics.NewNoopCollector(),
			execution_data.DefaultSerializer,
			bservice,
			trackerStorage,
		)

		exe, err := computer.NewBlockComputer(
			vm,
			execCtx,
			metrics.NewNoopCollector(),
			trace.NewNoopTracer(),
			zerolog.Nop(),
			committer,
			me,
			prov)
		require.NoError(t, err)

		// create an empty block
		block := generateBlock(0, 0, rag)
		derivedBlockData := derived.NewEmptyDerivedBlockData()

		vm.On("Run", mock.Anything, mock.Anything, mock.Anything).
			Return(nil).
			Once() // just system chunk

		committer.On("CommitView", mock.Anything, mock.Anything).
			Return(nil, nil, nil, nil).
			Once() // just system chunk

		view := delta.NewDeltaView(nil)

		result, err := exe.ExecuteBlock(
			context.Background(),
			unittest.IdentifierFixture(),
			block,
			view,
			derivedBlockData)
		assert.NoError(t, err)
		assert.Len(t, result.StateSnapshots, 1)
		assert.Len(t, result.TransactionResults, 1)
		assert.Len(t, result.ChunkExecutionDatas, 1)

		assertEventHashesMatch(t, 1, result)

		vm.AssertExpectations(t)
	})

	t.Run("system chunk transaction should not fail", func(t *testing.T) {

		// include all fees. System chunk should ignore them
		contextOptions := []fvm.Option{
			fvm.WithTransactionFeesEnabled(true),
			fvm.WithAccountStorageLimit(true),
			fvm.WithBlocks(&environment.NoopBlockFinder{}),
		}
		// set 0 clusters to pass n_collectors >= n_clusters check
		epochConfig := epochs.DefaultEpochConfig()
		epochConfig.NumCollectorClusters = 0
		bootstrapOptions := []fvm.BootstrapProcedureOption{
			fvm.WithTransactionFee(fvm.DefaultTransactionFees),
			fvm.WithAccountCreationFee(fvm.DefaultAccountCreationFee),
			fvm.WithMinimumStorageReservation(fvm.DefaultMinimumStorageReservation),
			fvm.WithStorageMBPerFLOW(fvm.DefaultStorageMBPerFLOW),
			fvm.WithEpochConfig(epochConfig),
		}

		chain := flow.Localnet.Chain()
		vm := fvm.NewVirtualMachine()
		derivedBlockData := derived.NewEmptyDerivedBlockData()
		baseOpts := []fvm.Option{
			fvm.WithChain(chain),
			fvm.WithDerivedBlockData(derivedBlockData),
		}

		opts := append(baseOpts, contextOptions...)
		ctx := fvm.NewContext(opts...)
		view := delta.NewDeltaView(nil)

		baseBootstrapOpts := []fvm.BootstrapProcedureOption{
			fvm.WithInitialTokenSupply(unittest.GenesisTokenSupply),
		}
		bootstrapOpts := append(baseBootstrapOpts, bootstrapOptions...)
		err := vm.Run(ctx, fvm.Bootstrap(unittest.ServiceAccountPublicKey, bootstrapOpts...), view)
		require.NoError(t, err)

		comm := new(computermock.ViewCommitter)

		bservice := requesterunit.MockBlobService(blockstore.NewBlockstore(dssync.MutexWrap(datastore.NewMapDatastore())))
		trackerStorage := mocktracker.NewMockStorage()

		prov := provider.NewProvider(
			zerolog.Nop(),
			metrics.NewNoopCollector(),
			execution_data.DefaultSerializer,
			bservice,
			trackerStorage,
		)

		exe, err := computer.NewBlockComputer(
			vm,
			ctx,
			metrics.NewNoopCollector(),
			trace.NewNoopTracer(),
			zerolog.Nop(),
			comm,
			me,
			prov)
		require.NoError(t, err)

		// create an empty block
		block := generateBlock(0, 0, rag)

		comm.On("CommitView", mock.Anything, mock.Anything).
			Return(nil, nil, nil, nil).
			Once() // just system chunk

		result, err := exe.ExecuteBlock(
			context.Background(),
			unittest.IdentifierFixture(),
			block,
			view,
			derivedBlockData)
		assert.NoError(t, err)
		assert.Len(t, result.StateSnapshots, 1)
		assert.Len(t, result.TransactionResults, 1)
		assert.Len(t, result.ChunkExecutionDatas, 1)

		assert.Empty(t, result.TransactionResults[0].ErrorMessage)
	})

	t.Run("multiple collections", func(t *testing.T) {
		execCtx := fvm.NewContext()

		vm := new(computermock.VirtualMachine)
		committer := new(computermock.ViewCommitter)

		bservice := requesterunit.MockBlobService(blockstore.NewBlockstore(dssync.MutexWrap(datastore.NewMapDatastore())))
		trackerStorage := mocktracker.NewMockStorage()

		prov := provider.NewProvider(
			zerolog.Nop(),
			metrics.NewNoopCollector(),
			execution_data.DefaultSerializer,
			bservice,
			trackerStorage,
		)

		exe, err := computer.NewBlockComputer(
			vm,
			execCtx,
			metrics.NewNoopCollector(),
			trace.NewNoopTracer(),
			zerolog.Nop(),
			committer,
			me,
			prov)
		require.NoError(t, err)

		collectionCount := 2
		transactionsPerCollection := 2
		eventsPerTransaction := 2
		eventsPerCollection := eventsPerTransaction * transactionsPerCollection
		totalTransactionCount := (collectionCount * transactionsPerCollection) + 1 // +1 for system chunk
		// totalEventCount := eventsPerTransaction * totalTransactionCount

		// create a block with 2 collections with 2 transactions each
		block := generateBlock(collectionCount, transactionsPerCollection, rag)
		derivedBlockData := derived.NewEmptyDerivedBlockData()

		vm.On("Run", mock.Anything, mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) {
				tx := args[1].(*fvm.TransactionProcedure)

				tx.Err = fvmErrors.NewInvalidAddressErrorf(
					flow.EmptyAddress,
					"no payer address provided")
				// create dummy events
				tx.Events = generateEvents(eventsPerTransaction, tx.TxIndex)
			}).
			Return(nil).
			Times(totalTransactionCount)

		committer.On("CommitView", mock.Anything, mock.Anything).
			Return(nil, nil, nil, nil).
			Times(collectionCount + 1)

		view := delta.NewDeltaView(nil)

		result, err := exe.ExecuteBlock(
			context.Background(),
			unittest.IdentifierFixture(),
			block,
			view,
			derivedBlockData)
		assert.NoError(t, err)

		// chunk count should match collection count
		assert.Len(t, result.StateSnapshots, collectionCount+1) // system chunk

		// all events should have been collected
		assert.Len(t, result.Events, collectionCount+1)

		for i := 0; i < collectionCount; i++ {
			assert.Len(t, result.Events[i], eventsPerCollection)
		}

		assert.Len(t, result.Events[len(result.Events)-1], eventsPerTransaction)

		// events should have been indexed by transaction and event
		k := 0
		for expectedTxIndex := 0; expectedTxIndex < totalTransactionCount; expectedTxIndex++ {
			for expectedEventIndex := 0; expectedEventIndex < eventsPerTransaction; expectedEventIndex++ {

				chunkIndex := k / eventsPerCollection
				eventIndex := k % eventsPerCollection

				e := result.Events[chunkIndex][eventIndex]
				assert.EqualValues(t, expectedEventIndex, int(e.EventIndex))
				assert.EqualValues(t, expectedTxIndex, e.TransactionIndex)
				k++
			}
		}

		expectedResults := make([]flow.TransactionResult, 0)
		for _, c := range block.CompleteCollections {
			for _, t := range c.Transactions {
				txResult := flow.TransactionResult{
					TransactionID: t.ID(),
					ErrorMessage: fvmErrors.NewInvalidAddressErrorf(
						flow.EmptyAddress,
						"no payer address provided").Error(),
				}
				expectedResults = append(expectedResults, txResult)
			}
		}
		assert.ElementsMatch(t, expectedResults, result.TransactionResults[0:len(result.TransactionResults)-1]) // strip system chunk

		assertEventHashesMatch(t, collectionCount+1, result)

		vm.AssertExpectations(t)
	})

	t.Run("service events are emitted", func(t *testing.T) {
		execCtx := fvm.NewContext(
			fvm.WithServiceEventCollectionEnabled(),
			fvm.WithAuthorizationChecksEnabled(false),
			fvm.WithSequenceNumberCheckAndIncrementEnabled(false),
		)

		collectionCount := 2
		transactionsPerCollection := 2

		totalTransactionCount := (collectionCount * transactionsPerCollection) + 1 // +1 for system chunk

		// create a block with 2 collections with 2 transactions each
		block := generateBlock(collectionCount, transactionsPerCollection, rag)

		ordinaryEvent := cadence.Event{
			EventType: &cadence.EventType{
				Location:            stdlib.FlowLocation{},
				QualifiedIdentifier: "what.ever",
			},
		}

		serviceEvents, err := systemcontracts.ServiceEventsForChain(execCtx.Chain.ChainID())
		require.NoError(t, err)

		payload, err := json.Decode(nil, []byte(fixtures.EpochSetupFixtureJSON))
		require.NoError(t, err)

		serviceEventA, ok := payload.(cadence.Event)
		require.True(t, ok)

		serviceEventA.EventType.Location = common.AddressLocation{
			Address: common.Address(serviceEvents.EpochSetup.Address),
		}
		serviceEventA.EventType.QualifiedIdentifier = serviceEvents.EpochSetup.QualifiedIdentifier()

		payload, err = json.Decode(nil, []byte(fixtures.EpochCommitFixtureJSON))
		require.NoError(t, err)

		serviceEventB, ok := payload.(cadence.Event)
		require.True(t, ok)

		serviceEventB.EventType.Location = common.AddressLocation{
			Address: common.Address(serviceEvents.EpochCommit.Address),
		}
		serviceEventB.EventType.QualifiedIdentifier = serviceEvents.EpochCommit.QualifiedIdentifier()

		// events to emit for each iteration/transaction
		events := make([][]cadence.Event, totalTransactionCount)
		events[0] = nil
		events[1] = []cadence.Event{serviceEventA, ordinaryEvent}
		events[2] = []cadence.Event{ordinaryEvent}
		events[3] = nil
		events[4] = []cadence.Event{serviceEventB}

		emittingRuntime := &testRuntime{
			executeTransaction: func(script runtime.Script, context runtime.Context) error {
				for _, e := range events[0] {
					err := context.Interface.EmitEvent(e)
					if err != nil {
						return err
					}
				}
				events = events[1:]
				return nil
			},
			readStored: func(address common.Address, path cadence.Path, r runtime.Context) (cadence.Value, error) {
				return nil, nil
			},
		}

		execCtx = fvm.NewContextFromParent(
			execCtx,
			fvm.WithReusableCadenceRuntimePool(
				reusableRuntime.NewCustomReusableCadenceRuntimePool(
					0,
					func(_ runtime.Config) runtime.Runtime {
						return emittingRuntime
					})))

		vm := fvm.NewVirtualMachine()

		bservice := requesterunit.MockBlobService(blockstore.NewBlockstore(dssync.MutexWrap(datastore.NewMapDatastore())))
		trackerStorage := mocktracker.NewMockStorage()

		prov := provider.NewProvider(
			zerolog.Nop(),
			metrics.NewNoopCollector(),
			execution_data.DefaultSerializer,
			bservice,
			trackerStorage,
		)

		exe, err := computer.NewBlockComputer(
			vm,
			execCtx,
			metrics.NewNoopCollector(),
			trace.NewNoopTracer(),
			zerolog.Nop(),
			committer.NewNoopViewCommitter(),
			me,
			prov)
		require.NoError(t, err)

		view := delta.NewDeltaView(nil)

		result, err := exe.ExecuteBlock(
			context.Background(),
			unittest.IdentifierFixture(),
			block,
			view,
			derived.NewEmptyDerivedBlockData())
		require.NoError(t, err)

		// make sure event index sequence are valid
		for _, eventsList := range result.Events {
			unittest.EnsureEventsIndexSeq(t, eventsList, execCtx.Chain.ChainID())
		}

		// all events should have been collected
		require.Len(t, result.ServiceEvents, 2)

		// events are ordered
		require.Equal(t, serviceEventA.EventType.ID(), string(result.ServiceEvents[0].Type))
		require.Equal(t, serviceEventB.EventType.ID(), string(result.ServiceEvents[1].Type))

		assertEventHashesMatch(t, collectionCount+1, result)
	})

	t.Run("succeeding transactions store programs", func(t *testing.T) {

		execCtx := fvm.NewContext()

		address := common.Address{0x1}
		contractLocation := common.AddressLocation{
			Address: address,
			Name:    "Test",
		}

		contractProgram := &interpreter.Program{}

		rt := &testRuntime{
			executeTransaction: func(script runtime.Script, r runtime.Context) error {

				_, err := r.Interface.GetAndSetProgram(
					contractLocation,
					func() (*interpreter.Program, error) {
						return contractProgram, nil
					},
				)
				require.NoError(t, err)

				return nil
			},
			readStored: func(address common.Address, path cadence.Path, r runtime.Context) (cadence.Value, error) {
				return nil, nil
			},
		}

		execCtx = fvm.NewContextFromParent(
			execCtx,
			fvm.WithReusableCadenceRuntimePool(
				reusableRuntime.NewCustomReusableCadenceRuntimePool(
					0,
					func(_ runtime.Config) runtime.Runtime {
						return rt
					})))

		vm := fvm.NewVirtualMachine()

		bservice := requesterunit.MockBlobService(blockstore.NewBlockstore(dssync.MutexWrap(datastore.NewMapDatastore())))
		trackerStorage := mocktracker.NewMockStorage()

		prov := provider.NewProvider(
			zerolog.Nop(),
			metrics.NewNoopCollector(),
			execution_data.DefaultSerializer,
			bservice,
			trackerStorage,
		)

		exe, err := computer.NewBlockComputer(
			vm,
			execCtx,
			metrics.NewNoopCollector(),
			trace.NewNoopTracer(),
			zerolog.Nop(),
			committer.NewNoopViewCommitter(),
			me,
			prov)
		require.NoError(t, err)

		const collectionCount = 2
		const transactionCount = 2
		block := generateBlock(collectionCount, transactionCount, rag)

		view := delta.NewDeltaView(nil)

		err = view.Set(
			flow.AccountStatusRegisterID(flow.BytesToAddress(address.Bytes())),
			environment.NewAccountStatus().ToBytes())
		require.NoError(t, err)

		result, err := exe.ExecuteBlock(
			context.Background(),
			unittest.IdentifierFixture(),
			block,
			view,
			derived.NewEmptyDerivedBlockData())
		assert.NoError(t, err)
		assert.Len(t, result.StateSnapshots, collectionCount+1) // +1 system chunk
	})

	t.Run("failing transactions do not store programs", func(t *testing.T) {
		execCtx := fvm.NewContext(
			fvm.WithAuthorizationChecksEnabled(false),
			fvm.WithSequenceNumberCheckAndIncrementEnabled(false),
		)

		address := common.Address{0x1}

		contractLocation := common.AddressLocation{
			Address: address,
			Name:    "Test",
		}

		contractProgram := &interpreter.Program{}

		const collectionCount = 2
		const transactionCount = 2

		var executionCalls int

		rt := &testRuntime{
			executeTransaction: func(script runtime.Script, r runtime.Context) error {

				executionCalls++

				// NOTE: set a program and revert all transactions but the system chunk transaction
				_, err := r.Interface.GetAndSetProgram(
					contractLocation,
					func() (*interpreter.Program, error) {
						return contractProgram, nil
					},
				)
				require.NoError(t, err)

				if executionCalls > collectionCount*transactionCount {
					return nil
				}

				return runtime.Error{
					Err: fmt.Errorf("TX reverted"),
				}
			},
			readStored: func(address common.Address, path cadence.Path, r runtime.Context) (cadence.Value, error) {
				return nil, nil
			},
		}

		execCtx = fvm.NewContextFromParent(
			execCtx,
			fvm.WithReusableCadenceRuntimePool(
				reusableRuntime.NewCustomReusableCadenceRuntimePool(
					0,
					func(_ runtime.Config) runtime.Runtime {
						return rt
					})))

		vm := fvm.NewVirtualMachine()

		bservice := requesterunit.MockBlobService(blockstore.NewBlockstore(dssync.MutexWrap(datastore.NewMapDatastore())))
		trackerStorage := mocktracker.NewMockStorage()

		prov := provider.NewProvider(
			zerolog.Nop(),
			metrics.NewNoopCollector(),
			execution_data.DefaultSerializer,
			bservice,
			trackerStorage,
		)

		exe, err := computer.NewBlockComputer(
			vm,
			execCtx,
			metrics.NewNoopCollector(),
			trace.NewNoopTracer(),
			zerolog.Nop(),
			committer.NewNoopViewCommitter(),
			me,
			prov)
		require.NoError(t, err)

		block := generateBlock(collectionCount, transactionCount, rag)

		view := delta.NewDeltaView(nil)

		err = view.Set(
			flow.AccountStatusRegisterID(flow.BytesToAddress(address.Bytes())),
			environment.NewAccountStatus().ToBytes())
		require.NoError(t, err)

		result, err := exe.ExecuteBlock(
			context.Background(),
			unittest.IdentifierFixture(),
			block,
			view,
			derived.NewEmptyDerivedBlockData())
		require.NoError(t, err)
		assert.Len(t, result.StateSnapshots, collectionCount+1) // +1 system chunk
	})
}

func assertEventHashesMatch(t *testing.T, expectedNoOfChunks int, result *execution.ComputationResult) {

	require.Len(t, result.Events, expectedNoOfChunks)
	require.Len(t, result.EventsHashes, expectedNoOfChunks)

	for i := 0; i < expectedNoOfChunks; i++ {
		calculatedHash, err := flow.EventsMerkleRootHash(result.Events[i])
		require.NoError(t, err)

		require.Equal(t, calculatedHash, result.EventsHashes[i])
	}
}

type testTransactionExecutor struct {
	executeTransaction func(runtime.Script, runtime.Context) error

	script  runtime.Script
	context runtime.Context
}

func (executor *testTransactionExecutor) Preprocess() error {
	// Do nothing.
	return nil
}

func (executor *testTransactionExecutor) Execute() error {
	return executor.executeTransaction(executor.script, executor.context)
}

func (executor *testTransactionExecutor) Result() (cadence.Value, error) {
	panic("Result not expected")
}

type testRuntime struct {
	executeScript      func(runtime.Script, runtime.Context) (cadence.Value, error)
	executeTransaction func(runtime.Script, runtime.Context) error
	readStored         func(common.Address, cadence.Path, runtime.Context) (cadence.Value, error)
}

var _ runtime.Runtime = &testRuntime{}

func (e *testRuntime) Config() runtime.Config {
	panic("Config not expected")
}

func (e *testRuntime) NewScriptExecutor(script runtime.Script, c runtime.Context) runtime.Executor {
	panic("NewScriptExecutor not expected")
}

func (e *testRuntime) NewTransactionExecutor(script runtime.Script, c runtime.Context) runtime.Executor {
	return &testTransactionExecutor{
		executeTransaction: e.executeTransaction,
		script:             script,
		context:            c,
	}
}

func (e *testRuntime) NewContractFunctionExecutor(contractLocation common.AddressLocation, functionName string, arguments []cadence.Value, argumentTypes []sema.Type, context runtime.Context) runtime.Executor {
	panic("NewContractFunctionExecutor not expected")
}

func (e *testRuntime) SetInvalidatedResourceValidationEnabled(_ bool) {
	panic("SetInvalidatedResourceValidationEnabled not expected")
}

func (e *testRuntime) SetTracingEnabled(_ bool) {
	panic("SetTracingEnabled not expected")
}

func (e *testRuntime) SetResourceOwnerChangeHandlerEnabled(_ bool) {
	panic("SetResourceOwnerChangeHandlerEnabled not expected")
}

func (e *testRuntime) InvokeContractFunction(_ common.AddressLocation, _ string, _ []cadence.Value, _ []sema.Type, _ runtime.Context) (cadence.Value, error) {
	panic("InvokeContractFunction not expected")
}

func (e *testRuntime) ExecuteScript(script runtime.Script, context runtime.Context) (cadence.Value, error) {
	return e.executeScript(script, context)
}

func (e *testRuntime) ExecuteTransaction(script runtime.Script, context runtime.Context) error {
	return e.executeTransaction(script, context)
}

func (*testRuntime) ParseAndCheckProgram(_ []byte, _ runtime.Context) (*interpreter.Program, error) {
	panic("ParseAndCheckProgram not expected")
}

func (*testRuntime) SetCoverageReport(_ *runtime.CoverageReport) {
	panic("SetCoverageReport not expected")
}

func (*testRuntime) SetContractUpdateValidationEnabled(_ bool) {
	panic("SetContractUpdateValidationEnabled not expected")
}

func (*testRuntime) SetAtreeValidationEnabled(_ bool) {
	panic("SetAtreeValidationEnabled not expected")
}

func (e *testRuntime) ReadStored(a common.Address, p cadence.Path, c runtime.Context) (cadence.Value, error) {
	return e.readStored(a, p, c)
}

func (*testRuntime) ReadLinked(_ common.Address, _ cadence.Path, _ runtime.Context) (cadence.Value, error) {
	panic("ReadLinked not expected")
}

func (*testRuntime) SetDebugger(_ *interpreter.Debugger) {
	panic("SetDebugger not expected")
}

type RandomAddressGenerator struct{}

func (r *RandomAddressGenerator) NextAddress() (flow.Address, error) {
	return flow.HexToAddress(fmt.Sprintf("0%d", rand.Intn(1000))), nil
}

func (r *RandomAddressGenerator) CurrentAddress() flow.Address {
	return flow.HexToAddress(fmt.Sprintf("0%d", rand.Intn(1000)))
}

func (r *RandomAddressGenerator) Bytes() []byte {
	panic("not implemented")
}

func (r *RandomAddressGenerator) AddressCount() uint64 {
	panic("not implemented")
}

func (testRuntime) Storage(runtime.Context) (*runtime.Storage, *interpreter.Interpreter, error) {
	panic("Storage not expected")
}

type FixedAddressGenerator struct {
	Address flow.Address
}

func (f *FixedAddressGenerator) NextAddress() (flow.Address, error) {
	return f.Address, nil
}

func (f *FixedAddressGenerator) CurrentAddress() flow.Address {
	return f.Address
}

func (f *FixedAddressGenerator) Bytes() []byte {
	panic("not implemented")
}

func (f *FixedAddressGenerator) AddressCount() uint64 {
	panic("not implemented")
}

func Test_AccountStatusRegistersAreIncluded(t *testing.T) {

	address := flow.HexToAddress("1234")
	fag := &FixedAddressGenerator{Address: address}

	vm := fvm.NewVirtualMachine()
	execCtx := fvm.NewContext()

	ledger := testutil.RootBootstrappedLedger(vm, execCtx)

	key, err := unittest.AccountKeyDefaultFixture()
	require.NoError(t, err)

	view := delta.NewDeltaView(ledger)
	txnState := state.NewTransactionState(view, state.DefaultParameters())
	accounts := environment.NewAccounts(txnState)

	// account creation, signing of transaction and bootstrapping ledger should not be required for this test
	// as freeze check should happen before a transaction signature is checked
	// but we currently discard all the touches if it fails and any point
	err = accounts.Create([]flow.AccountPublicKey{key.PublicKey(1000)}, address)
	require.NoError(t, err)

	bservice := requesterunit.MockBlobService(blockstore.NewBlockstore(dssync.MutexWrap(datastore.NewMapDatastore())))
	trackerStorage := mocktracker.NewMockStorage()

	prov := provider.NewProvider(
		zerolog.Nop(),
		metrics.NewNoopCollector(),
		execution_data.DefaultSerializer,
		bservice,
		trackerStorage,
	)

	me := new(modulemock.Local)
	me.On("SignFunc", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, nil)

	exe, err := computer.NewBlockComputer(
		vm,
		execCtx,
		metrics.NewNoopCollector(),
		trace.NewNoopTracer(),
		zerolog.Nop(),
		committer.NewNoopViewCommitter(),
		me,
		prov)
	require.NoError(t, err)

	block := generateBlockWithVisitor(1, 1, fag, func(txBody *flow.TransactionBody) {
		err := testutil.SignTransaction(txBody, txBody.Payer, *key, 0)
		require.NoError(t, err)
	})

	_, err = exe.ExecuteBlock(
		context.Background(),
		unittest.IdentifierFixture(),
		block,
		view,
		derived.NewEmptyDerivedBlockData())
	assert.NoError(t, err)

	registerTouches := view.Interactions().RegisterTouches()

	// make sure check for account status has been registered
	id := flow.AccountStatusRegisterID(address)

	require.Contains(t, registerTouches, id)
}

func Test_ExecutingSystemCollection(t *testing.T) {

	execCtx := fvm.NewContext(
		fvm.WithChain(flow.Localnet.Chain()),
		fvm.WithBlocks(&environment.NoopBlockFinder{}),
	)

	vm := fvm.NewVirtualMachine()

	rag := &RandomAddressGenerator{}

	ledger := testutil.RootBootstrappedLedger(vm, execCtx)

	committer := new(computermock.ViewCommitter)
	committer.On("CommitView", mock.Anything, mock.Anything).
		Return(nil, nil, nil, nil).
		Times(1) // only system chunk

	noopCollector := metrics.NewNoopCollector()

	expectedNumberOfEvents := 2
	expectedEventSize := 911
	// bootstrapping does not cache programs
	expectedCachedPrograms := 0

	metrics := new(modulemock.ExecutionMetrics)
	metrics.On("ExecutionCollectionExecuted",
		mock.Anything,  // duration
		mock.Anything). // stats
		Return(nil).
		Times(1) // system collection

	metrics.On("ExecutionTransactionExecuted",
		mock.Anything, // duration
		mock.Anything, // computation used
		mock.Anything, // memory used
		mock.Anything, // actual memory used
		expectedNumberOfEvents,
		expectedEventSize,
		false).
		Return(nil).
		Times(1) // system chunk tx

	metrics.On(
		"ExecutionChunkDataPackGenerated",
		mock.Anything,
		mock.Anything).
		Return(nil).
		Times(1) // system collection

	metrics.On(
		"ExecutionBlockCachedPrograms",
		expectedCachedPrograms).
		Return(nil).
		Times(1) // block

	bservice := requesterunit.MockBlobService(blockstore.NewBlockstore(dssync.MutexWrap(datastore.NewMapDatastore())))
	trackerStorage := mocktracker.NewMockStorage()

	prov := provider.NewProvider(
		zerolog.Nop(),
		noopCollector,
		execution_data.DefaultSerializer,
		bservice,
		trackerStorage,
	)

	me := new(modulemock.Local)
	me.On("SignFunc", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, nil)

	exe, err := computer.NewBlockComputer(
		vm,
		execCtx,
		metrics,
		trace.NewNoopTracer(),
		zerolog.Nop(),
		committer,
		me,
		prov)
	require.NoError(t, err)

	// create empty block, it will have system collection attached while executing
	block := generateBlock(0, 0, rag)

	view := delta.NewDeltaView(ledger)

	result, err := exe.ExecuteBlock(
		context.Background(),
		unittest.IdentifierFixture(),
		block,
		view,
		derived.NewEmptyDerivedBlockData())
	assert.NoError(t, err)
	assert.Len(t, result.StateSnapshots, 1) // +1 system chunk
	assert.Len(t, result.TransactionResults, 1)

	assert.Empty(t, result.TransactionResults[0].ErrorMessage)

	stats := result.CollectionStats(0)
	// ignore computation and memory used
	stats.ComputationUsed = 0
	stats.MemoryUsed = 0

	assert.Equal(
		t,
		module.ExecutionResultStats{
			EventCounts:                     expectedNumberOfEvents,
			EventSize:                       expectedEventSize,
			NumberOfRegistersTouched:        66,
			NumberOfBytesWrittenToRegisters: 4214,
			NumberOfCollections:             1,
			NumberOfTransactions:            1,
		},
		stats)

	committer.AssertExpectations(t)
}

func generateBlock(collectionCount, transactionCount int, addressGenerator flow.AddressGenerator) *entity.ExecutableBlock {
	return generateBlockWithVisitor(collectionCount, transactionCount, addressGenerator, nil)
}

func generateBlockWithVisitor(collectionCount, transactionCount int, addressGenerator flow.AddressGenerator, visitor func(body *flow.TransactionBody)) *entity.ExecutableBlock {
	collections := make([]*entity.CompleteCollection, collectionCount)
	guarantees := make([]*flow.CollectionGuarantee, collectionCount)
	completeCollections := make(map[flow.Identifier]*entity.CompleteCollection)

	for i := 0; i < collectionCount; i++ {
		collection := generateCollection(transactionCount, addressGenerator, visitor)
		collections[i] = collection
		guarantees[i] = collection.Guarantee
		completeCollections[collection.Guarantee.ID()] = collection
	}

	block := flow.Block{
		Header: &flow.Header{
			Timestamp: flow.GenesisTime,
			Height:    42,
			View:      42,
		},
		Payload: &flow.Payload{
			Guarantees: guarantees,
		},
	}

	return &entity.ExecutableBlock{
		Block:               &block,
		CompleteCollections: completeCollections,
		StartState:          unittest.StateCommitmentPointerFixture(),
	}
}

func generateCollection(transactionCount int, addressGenerator flow.AddressGenerator, visitor func(body *flow.TransactionBody)) *entity.CompleteCollection {
	transactions := make([]*flow.TransactionBody, transactionCount)

	for i := 0; i < transactionCount; i++ {
		nextAddress, err := addressGenerator.NextAddress()
		if err != nil {
			panic(fmt.Errorf("cannot generate next address in test: %w", err))
		}
		txBody := &flow.TransactionBody{
			Payer:  nextAddress, // a unique payer for each tx to generate a unique id
			Script: []byte("transaction { execute {} }"),
		}
		if visitor != nil {
			visitor(txBody)
		}
		transactions[i] = txBody
	}

	collection := flow.Collection{Transactions: transactions}

	guarantee := &flow.CollectionGuarantee{CollectionID: collection.ID()}

	return &entity.CompleteCollection{
		Guarantee:    guarantee,
		Transactions: transactions,
	}
}

func generateEvents(eventCount int, txIndex uint32) []flow.Event {
	events := make([]flow.Event, eventCount)
	for i := 0; i < eventCount; i++ {
		// creating some dummy event
		event := flow.Event{Type: "whatever", EventIndex: uint32(i), TransactionIndex: txIndex}
		events[i] = event
	}
	return events
}

func getSetAProgram(t *testing.T, derivedBlockData *derived.DerivedBlockData) {

	derivedTxnData, err := derivedBlockData.NewDerivedTransactionData(
		0,
		0)
	require.NoError(t, err)

	loc := common.AddressLocation{
		Name:    "SomeContract",
		Address: common.MustBytesToAddress([]byte{0x1}),
	}
	_, _, got := derivedTxnData.GetProgram(
		loc,
	)
	if got {
		return
	}

	derivedTxnData.SetProgram(
		loc,
		&derived.Program{},
		&state.State{},
	)
	err = derivedTxnData.Commit()
	require.NoError(t, err)
}
