package block_queue

import (
	"fmt"
	"sync"

	"github.com/onflow/flow-go/model/flow"
	"github.com/onflow/flow-go/module/mempool/entity"
	"github.com/rs/zerolog"
)

// BlockQueue keeps track of state of blocks and determines which blocks are executable
// A block becomes executable when all the following conditions are met:
// 1. the block has been validated by consensus algorithm
// 2. the block's parent has been executed
// 3. all the collections included in the block have been received
type BlockQueue struct {
	sync.Mutex
	log zerolog.Logger

	// when receiving a new block, adding it to the map, and add missing collections to the map
	blocks map[flow.Identifier]*entity.ExecutableBlock
	// a collection could be included in multiple blocks,
	// when a missing block is received, it might trigger multiple blocks to be executable, which
	// can be looked up by the map
	// when a block is executed, its collections should be removed from this map unless a collection
	// is still referenced by other blocks, which will eventually be removed when those blocks are
	// executed.
	collections map[flow.Identifier]*collectionInfo

	// blockIDsByHeight is used to find next executable block.
	// when a block is executed, the next executable block must be a block with height = current block height + 1
	// the following map allows us to find the next executable block by height
	blockIDsByHeight map[uint64]map[flow.Identifier]*entity.ExecutableBlock // for finding next executable block
}

type MissingCollection struct {
	BlockID   flow.Identifier
	Height    uint64
	Guarantee *flow.CollectionGuarantee
}

func (m *MissingCollection) ID() flow.Identifier {
	return m.Guarantee.ID()
}

// collectionInfo is an internal struct used to keep track of the state of a collection,
// and the blocks that include the collection
type collectionInfo struct {
	Collection *entity.CompleteCollection
	IncludedIn map[flow.Identifier]*entity.ExecutableBlock
}

func NewBlockQueue(logger zerolog.Logger) *BlockQueue {
	log := logger.With().Str("module", "block_queue").Logger()

	return &BlockQueue{
		log:              log,
		blocks:           make(map[flow.Identifier]*entity.ExecutableBlock),
		collections:      make(map[flow.Identifier]*collectionInfo),
		blockIDsByHeight: make(map[uint64]map[flow.Identifier]*entity.ExecutableBlock),
	}
}

// OnBlock is called when a new block is received, and its parent is not executed.
// It returns a list of missing collections and a list of executable blocks
// Note: caller must ensure when OnBlock is called with a block,
// if its parent is not executed, then the parent must be added to the queue first.
// if its parent is executed, then the parent's finalState must be passed in.
func (q *BlockQueue) OnBlock(block *flow.Block, parentFinalState *flow.StateCommitment) (
	[]*MissingCollection, // missing collections
	[]*entity.ExecutableBlock, // blocks ready to execute
	error, // exceptions
) {
	q.Lock()
	defer q.Unlock()

	// check if the block already exists
	blockID := block.ID()
	executable, ok := q.blocks[blockID]
	// handle the case where the block has seen before
	if ok {
		// we have already received this block, and its parent still has not been executed yet
		if executable.StartState == nil && parentFinalState == nil {
			return nil, nil, nil
		}

		// this is an edge case where parentFinalState is provided, and its parent block exists
		// in the queue but has not been marked as executed yet (OnBlockExecuted(parent) is not called),
		// in this case, we will internally call OnBlockExecuted(parentBlockID, parentFinalState).
		// there is no need to create the executable block again, since it's already created.
		if executable.StartState == nil && parentFinalState != nil {
			executables, err := q.onBlockExecuted(block.Header.ParentID, *parentFinalState)
			if err != nil {
				return nil, nil, fmt.Errorf("receiving block %v with parent commitment %v, but parent block %v already exists with no commitment, fail to call mark parent as executed: %w",
					blockID, *parentFinalState, block.Header.ParentID, err)
			}

			// we already have this block, its collection must have been fetched, so we only return the
			// executables from marking its parent as executed.
			return nil, executables, nil
		}

		// this is an edge case could be ignored
		if executable.StartState != nil && parentFinalState == nil {
			q.log.Warn().
				Str("blockID", blockID.String()).
				Hex("parentID", block.Header.ParentID[:]).
				Msg("edge case: receiving block with no parent commitment, but its parent block actually has been executed")
			return nil, nil, nil
		}

		// this is an exception that should not happen
		if *executable.StartState != *parentFinalState {
			return nil, nil,
				fmt.Errorf("block %s has already been executed with a different parent final state, %v != %v",
					blockID, *executable.StartState, parentFinalState)
		}

		return nil, nil, nil
	}

	// if parentFinalState is not provided, then its parent block must exists in the queue
	// otherwise it's an exception
	if parentFinalState == nil {
		_, parentExists := q.blocks[block.Header.ParentID]
		if !parentExists {
			return nil, nil, fmt.Errorf("parent block %s of block %s is not in the queue",
				block.Header.ParentID, blockID)
		}
	}

	executable = &entity.ExecutableBlock{
		Block:      block,
		StartState: parentFinalState,
	}

	// add block to blocks
	q.blocks[blockID] = executable

	// update collection
	colls := make(map[flow.Identifier]*entity.CompleteCollection, len(block.Payload.Guarantees))
	executable.CompleteCollections = colls

	// find missing collections and update collection index
	missingCollections := make([]*MissingCollection, 0, len(block.Payload.Guarantees))

	for _, guarantee := range block.Payload.Guarantees {
		colID := guarantee.ID()
		colInfo, ok := q.collections[colID]
		if ok {
			// some other block also includes this collection
			colInfo.IncludedIn[blockID] = executable
			colls[colID] = colInfo.Collection
		} else {
			col := &entity.CompleteCollection{
				Guarantee: guarantee,
			}
			colls[colID] = col

			// add new collection to collections
			q.collections[colID] = &collectionInfo{
				Collection: col,
				IncludedIn: map[flow.Identifier]*entity.ExecutableBlock{
					blockID: executable,
				},
			}

			missingCollections = append(missingCollections, missingCollectionForBlock(executable, guarantee))
		}
	}

	// index height
	blocksAtSameHeight, ok := q.blockIDsByHeight[block.Header.Height]
	if !ok {
		blocksAtSameHeight = make(map[flow.Identifier]*entity.ExecutableBlock)
		q.blockIDsByHeight[block.Header.Height] = blocksAtSameHeight
	}
	blocksAtSameHeight[blockID] = executable

	// check if the block is executable
	var executables []*entity.ExecutableBlock
	if executable.IsComplete() {
		// executables might contain other siblings, but won't contain "executable",
		// which is the block itself, that's because executables are created
		// from OnBlockExecuted(
		executables = []*entity.ExecutableBlock{executable}
	}

	return missingCollections, executables, nil
}

// OnCollection is called when a new collection is received
// It returns a list of executable blocks that contains the collection
func (q *BlockQueue) OnCollection(collection *flow.Collection) ([]*entity.ExecutableBlock, error) {
	q.Lock()
	defer q.Unlock()
	// when a collection is received, we find the blocks the collection is included in,
	// and check if the blocks become executable.
	// Note a collection could be included in multiple blocks, so receiving a collection
	// might trigger multiple blocks to be executable.

	// check if the collection is for any block in the queue
	colID := collection.ID()
	colInfo, ok := q.collections[colID]
	if !ok {
		// no block in the queue includes this collection
		return nil, nil
	}

	if colInfo.Collection.IsCompleted() {
		// the collection is already received, no action needed because an action must
		// have been returned when the collection is first received.
		return nil, nil
	}

	// update collection
	colInfo.Collection.Transactions = collection.Transactions

	// check if any block, which includes this collection, become executable
	executables := make([]*entity.ExecutableBlock, 0, len(colInfo.IncludedIn))
	for _, block := range colInfo.IncludedIn {
		if !block.IsComplete() {
			continue
		}
		executables = append(executables, block)
	}

	if len(executables) == 0 {
		return nil, nil
	}

	return executables, nil
}

// OnBlockExecuted is called when a block is executed
// It returns a list of executable blocks (usually its child blocks)
func (q *BlockQueue) OnBlockExecuted(
	blockID flow.Identifier,
	commit flow.StateCommitment,
) ([]*entity.ExecutableBlock, error) {
	q.Lock()
	defer q.Unlock()

	return q.onBlockExecuted(blockID, commit)
}

func (q *BlockQueue) onBlockExecuted(
	blockID flow.Identifier,
	commit flow.StateCommitment,
) ([]*entity.ExecutableBlock, error) {
	// when a block is executed, the child block might become executable
	// we also remove it from all the indexes

	// remove block
	block, ok := q.blocks[blockID]
	if !ok {
		return nil, nil
	}

	delete(q.blocks, blockID)

	// remove height index
	height := block.Block.Header.Height
	delete(q.blockIDsByHeight[height], blockID)
	if len(q.blockIDsByHeight[height]) == 0 {
		delete(q.blockIDsByHeight, height)
	}

	// remove colections if no other blocks include it
	for colID := range block.CompleteCollections {
		colInfo, ok := q.collections[colID]
		if !ok {
			return nil, fmt.Errorf("collection %s not found", colID)
		}

		delete(colInfo.IncludedIn, blockID)
		if len(colInfo.IncludedIn) == 0 {
			// no other blocks includes this collection,
			// so this collection can be removed from the index
			delete(q.collections, colID)
		}
	}

	return q.checkIfChildBlockBecomeExecutable(block, commit)
}

func (q *BlockQueue) checkIfChildBlockBecomeExecutable(
	block *entity.ExecutableBlock,
	commit flow.StateCommitment,
) ([]*entity.ExecutableBlock, error) {
	childHeight := block.Block.Header.Height + 1
	blocksAtNextHeight, ok := q.blockIDsByHeight[childHeight]
	if !ok {
		// no block at next height
		return nil, nil
	}

	// find children and update their start state
	children := make([]*entity.ExecutableBlock, 0, len(blocksAtNextHeight))
	for _, childBlock := range blocksAtNextHeight {
		// a child block at the next height must have the same parent ID
		// as the current block
		isChild := childBlock.Block.Header.ParentID == block.ID()
		if !isChild {
			continue
		}

		// update child block's start state with current block's end state
		childBlock.StartState = &commit
		children = append(children, childBlock)
	}

	if len(children) == 0 {
		return nil, nil
	}

	// check if children are executable
	executables := make([]*entity.ExecutableBlock, 0, len(children))
	for _, child := range children {
		if child.IsComplete() {
			executables = append(executables, child)
		}
	}

	return executables, nil
}

// GetMissingCollections returns the missing collections and the start state
// It returns an error if the block is not found
func (q *BlockQueue) GetMissingCollections(blockID flow.Identifier) (
	[]*MissingCollection, *flow.StateCommitment, error) {
	q.Lock()
	defer q.Unlock()
	block, ok := q.blocks[blockID]
	if !ok {
		return nil, nil, fmt.Errorf("block %s not found", blockID)
	}

	missingCollections := make([]*MissingCollection, 0, len(block.Block.Payload.Guarantees))
	for _, col := range block.CompleteCollections {
		// check if the collection is already received
		if col.IsCompleted() {
			continue
		}
		missingCollections = append(missingCollections, missingCollectionForBlock(block, col.Guarantee))
	}

	return missingCollections, block.StartState, nil
}

func missingCollectionForBlock(block *entity.ExecutableBlock, guarantee *flow.CollectionGuarantee) *MissingCollection {
	return &MissingCollection{
		BlockID:   block.ID(),
		Height:    block.Block.Header.Height,
		Guarantee: guarantee,
	}
}
