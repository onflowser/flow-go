package protocol

import (
	"github.com/onflow/flow-go/model/flow"
)

// InitialProtocolState returns constant data for given epoch.
// This interface can be only obtained for epochs that have progressed to epoch commit event.
type InitialProtocolState interface {
	// Epoch returns counter of epoch.
	Epoch() uint64
	// Clustering returns initial clustering from epoch setup.
	// No errors are expected during normal operations.
	Clustering() (flow.ClusterList, error)
	// EpochSetup returns original epoch setup event that was used to initialize the protocol state.
	EpochSetup() *flow.EpochSetup
	// EpochCommit returns original epoch commit event that was used to update the protocol state.
	EpochCommit() *flow.EpochCommit
	// DKG returns information about DKG that was obtained from EpochCommit event.
	// No errors are expected during normal operations.
	DKG() (DKG, error)
	// Entry Returns low-level protocol state entry that was used to initialize this object.
	// It shouldn't be used by high-level logic, it is useful for some cases such as bootstrapping.
	// Prefer using other methods to access protocol state.
	Entry() *flow.RichProtocolStateEntry
}

// DynamicProtocolState extends the InitialProtocolState with data that can change from block to block.
// It can be used to access the identity table at given block.
type DynamicProtocolState interface {
	InitialProtocolState

	// InvalidEpochTransitionAttempted denotes whether an invalid epoch state transition was attempted
	// on the fork ending this block. Once the first block where this flag is true is finalized, epoch
	// fallback mode is triggered.
	// TODO for 'leaving Epoch Fallback via special service event': at the moment, this is a one-way transition and requires a spork to recover - need to revisit for sporkless EFM recovery
	InvalidEpochTransitionAttempted() bool
	// PreviousEpochExists returns true if a previous epoch exists. This is true for all epoch
	// except those immediately following a spork.
	PreviousEpochExists() bool
	// EpochPhase returns the epoch phase for the current epoch.
	EpochPhase() flow.EpochPhase

	// Identities returns identities (in canonical ordering) that can participate in the current or previous
	// or next epochs. Let P be the set of identities in the previous epoch, C be the set of identities in
	// the current epoch, and N be the set of identities in the next epoch.
	// The set of authorized identities this function returns is different depending on epoch state:
	// EpochStaking phase:
	//   - nodes in C with status `flow.EpochParticipationStatusActive`
	//   - nodes in P-C with status `flow.EpochParticipationStatusLeaving`
	// EpochSetup/EpochCommitted phase:
	//   - nodes in C with status `flow.EpochParticipationStatusActive`
	//   - nodes in N-C with status `flow.EpochParticipationStatusJoining`
	Identities() flow.IdentityList
	// GlobalParams returns params that are same for all nodes in the network.
	GlobalParams() GlobalParams
}

// ProtocolState is the read-only interface for protocol state, it allows to query information
// on a per-block and per-epoch basis.
type ProtocolState interface {
	// ByEpoch returns an object with static protocol state information by epoch number.
	// To be able to use this interface we need to observe both epoch setup and commit events.
	// Not available for next epoch unless we have observed an EpochCommit event.
	// No errors are expected during normal operations.
	// TODO(yuraolex): check return types
	// TODO(yuraolex): decide if we really need this approach. It's unclear if it's useful to query
	//  by epoch counter. To implement it we need an additional index by epoch counter. Alternatively we need a way to map
	//  epoch counter -> block ID. It gets worse if we consider that we need a way to get the epoch counter itself at caller side.
	//ByEpoch(epoch uint64) (InitialProtocolState, error)

	// AtBlockID returns epoch protocol state at block ID.
	// The resulting epoch protocol state is returned AFTER applying updates that are contained in block.
	// Can be queried for any block that has been added to the block tree.
	// Returns:
	// - (DynamicProtocolState, nil) - if there is an epoch protocol state associated with given block ID.
	// - (nil, storage.ErrNotFound) - if there is no epoch protocol state associated with given block ID.
	// - (nil, exception) - any other error should be treated as exception.
	AtBlockID(blockID flow.Identifier) (DynamicProtocolState, error)

	// KVStoreAtBlockID returns protocol state at block ID.
	// The resulting protocol state is returned AFTER applying updates that are contained in block.
	// Can be queried for any block that has been added to the block tree.
	// Returns:
	// - (KVStoreReader, nil) - if there is a protocol state associated with given block ID.
	// - (nil, storage.ErrNotFound) - if there is no protocol state associated with given block ID.
	// - (nil, exception) - any other error should be treated as exception.
	KVStoreAtBlockID(blockID flow.Identifier) (KVStoreReader, error)

	// GlobalParams returns params that are the same for all nodes in the network.
	GlobalParams() GlobalParams
}

// MutableProtocolState is the read-write interface for protocol state. It allows evolving the protocol state by
// creating a StateMutator for each block and applying state-changing service events.
type MutableProtocolState interface {
	ProtocolState

	// Mutator instantiates a `StateMutator` based on the previous protocol state.
	// Has to be called for each block to evolve the protocol state.
	// Expected errors during normal operations:
	//  - `storage.ErrNotFound` if no protocol state for parent block is known.
	Mutator(candidateView uint64, parentID flow.Identifier) (StateMutator, error)
}

// StateMutator is a stateful object to evolve the protocol state. It is instantiated from the parent block's protocol state.
// State-changing operations can be iteratively applied and the StateMutator will internally evolve its in-memory state.
// While the StateMutator does not modify the database, it internally tracks the necessary database updates to persist its
// dependencies (specifically EpochSetup and EpochCommit events). Upon calling `Build` the StateMutator returns the updated
// protocol state, its ID and all database updates necessary for persisting the updated protocol state.
//
// The StateMutator is used by a replica's compliance layer to update protocol state when observing state-changing service in
// blocks. It is used by the primary in the block building process to obtain the correct protocol state for a proposal.
// Specifically, the leader may include state-changing service events in the block payload. The flow protocol prescribes that
// the proposal needs to include the ID of the protocol state, _after_ processing the payload incl. all state-changing events.
// Therefore, the leader instantiates a StateMutator, applies the service events to it and builds the updated protocol state ID.
//
// Not safe for concurrent use.
//
// TODO: merge methods `EvolveState` and `Build` into one, as they must be always called in this succession (improves API's safety & clarity)
type StateMutator interface {

	// Build assembles the resulting Protocol State, *after* evolving it based on information from the
	// candidate block (potentially still under construction) via the `EvolveState(..)` method call.
	// Information that may change the state is
	//   - the candidate block's view (already provided at construction time of `StateMutator`)
	//   - Service Events from execution results sealed by the candidate block
	//
	// During the execution of `EvolveState(..)` and the subsequent `Build()` step, we collect deferred
	// database updates for persisting the resulting Protocol State, including all of its dependencies
	// and respective indices. Though, the resulting batch of deferred database updates still depends
	// on the candidate block's ID, which is still unknown at the time of block construction.
	// Executing the deferred database updates is the responsibility of the calling code.
	//
	// CAUTION:
	//  - For Consensus Participants that are replicas, the calling code must check that the returned `stateID` matches the
	//    commitment in the block proposal! If they don't match, the proposal is byzantine and should be slashed.
	//  - `EvolveState(..)` must have been called before
	//
	// Return values
	//  - stateID: the hash commitment to the updated Protocol State Snapshot
	//  - dbUpdates: database updates necessary for persisting the State Snapshot itself including all data structures
	//    that the Snapshot references. In addition, `dbUpdates` also populates the `ProtocolKVStore.ByBlockID`.
	//    Therefore, even if there are no changes of the Protocol State, `dbUpdates` still contains deferred storage writes
	//    that must be executed to populate the `ByBlockID` index.
	//  - err: All error returns indicate potential state corruption and should therefore be treated as fatal.
	Build() (stateID flow.Identifier, dbUpdates *DeferredBlockPersist, err error)

	// EvolveState updates the overall Protocol State based on information from the candidate block
	// (potentially still under construction). Information that may change the state is:
	//   - the candidate block's view (already provided at construction time)
	//   - Service Events sealed in the candidate block
	//
	// We only mutate the `StateMutator`'s internal in-memory copy of the protocol state, without
	// changing the parent state (i.e. the state we started from).
	//
	// SAFETY REQUIREMENT:
	// The StateMutator assumes that the proposal has passed the following correctness checks!
	//   - The seals in the payload continuously follow the ancestry of this fork. Specifically,
	//     there are no gaps in the seals.
	//   - The seals guarantee correctness of the sealed execution result, including the contained
	//     service events. This is actively checked by the verification node, whose aggregated
	//     approvals in the form of a seal attest to the correctness of the sealed execution result
	//     (specifically the Service Events contained in the result and their order).
	//   - `EvolveState` must be called before `Build`
	//
	// Consensus nodes actively verify protocol compliance for any block proposal they receive,
	// including integrity of each seal individually as well as the seals continuously following
	// the fork. Light clients only process certified blocks, which guarantees that consensus nodes
	// already ran those checks and found the proposal to be valid.
	//
	// Details on SERVICE EVENTS:
	// Consider a chain where a service event is emitted during execution of block A. Block B contains
	// an execution receipt `RA` for A. Block C contains a seal `SA` for A's execution result.
	//
	//	A <- .. <- B(RA) <- .. <- C(SA)
	//
	// Service Events are included within execution results, which are stored opaquely as part of the
	// block payload (block B in our example). We only validate, process and persist the typed service
	// event to storage once we process C, the block containing the seal for block A. This is because
	// we rely on the sealing subsystem to validate correctness of the service event before processing
	// it. Consequently, any change to the protocol state introduced by a service event emitted during
	// execution of block A would only become visible when querying C or its descendants.
	//
	// Error returns:
	// [TLDR] All error returns indicate potential state corruption and should therefore be treated as fatal.
	//   - Per convention, the input seals from the block payload have already been confirmed to be protocol compliant.
	//     Hence, the service events in the sealed execution results represent the honest execution path.
	//     Therefore, the sealed service events should encode a valid evolution of the protocol state -- provided
	//     the system smart contracts are correct.
	//   - As we can rule out byzantine attacks as the source of failures, the only remaining sources of problems
	//     can be (a) bugs in the system smart contracts or (b) bugs in the node implementation. A service event
	//     not representing a valid state transition despite all consistency checks passing is interpreted as
	//     case (a) and _should be_ handled internally by the respective state machine. Otherwise, any bug or
	//     unforeseen edge cases in the system smart contracts would in consensus halt, due to errors while
	//     evolving the protocol state.
	//   - A consistency or sanity check failing within the StateMutator is likely the symptom of an internal bug
	//     in the node software or state corruption, i.e. case (b). This is the only scenario where the error return
	//     of this function is not nil. If such an exception is returned, continuing is not an option.
	EvolveState(seals []*flow.Seal) error
}
