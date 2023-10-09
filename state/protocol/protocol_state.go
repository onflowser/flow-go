package protocol

import (
	"github.com/onflow/flow-go/model/flow"
	"github.com/onflow/flow-go/storage/badger/transaction"
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
	// EpochStatus returns the status of current epoch at given block based on the internal state of protocol.
	EpochStatus() *flow.EpochStatus
	// Identities returns identities that can participate in current and next epochs.
	// Set of Authorized identities are different depending on epoch state:
	// staking phase - identities for current epoch + identities from previous epoch (with 0 weight)
	// setup & commit phase - identities for current epoch + identities from next epoch (with 0 weight)
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

	// AtBlockID returns protocol state at block ID.
	// The resulting protocol state is returned AFTER applying updates that are contained in block.
	// Can be queried for any block that has been added to the block tree.
	// Returns:
	// - (DynamicProtocolState, nil) - if there is a protocol state associated with given block ID.
	// - (nil, storage.ErrNotFound) - if there is no protocol state associated with given block ID.
	// - (nil, exception) - any other error should be treated as exception.
	AtBlockID(blockID flow.Identifier) (DynamicProtocolState, error)
	// GlobalParams returns params that are the same for all nodes in the network.
	GlobalParams() GlobalParams
}

// StateUpdater is a dedicated interface for updating protocol state.
// It is used by the compliance layer to update protocol state when certain events that are stored in blocks are observed.
// CAUTION: The compliance layer is responsible for validating events before passing them to StateUpdater.
type StateUpdater interface {
	// Build returns updated protocol state entry, state ID and a flag indicating if there were any changes.
	Build() (updatedState *flow.ProtocolStateEntry, stateID flow.Identifier, hasChanges bool)
	// ProcessEpochSetup updates current protocol state with data from epoch setup event.
	// Processing epoch setup event also affects identity table for current epoch.
	// Observing an epoch setup event, transitions protocol state from staking to setup phase, we stop returning
	// identities from previous+current epochs and start returning identities from current+next epochs.
	// As a result of this operation protocol state for the next epoch will be created.
	// CAUTION: Caller must validate input event.
	// No errors are expected during normal operations.
	ProcessEpochSetup(epochSetup *flow.EpochSetup) error
	// ProcessEpochCommit updates current protocol state with data from epoch commit event.
	// Observing an epoch setup commit, transitions protocol state from setup to commit phase, at this point we have
	// finished construction of the next epoch.
	// As a result of this operation protocol state for next epoch will be committed.
	// CAUTION: Caller must validate input event.
	// No errors are expected during normal operations.
	ProcessEpochCommit(epochCommit *flow.EpochCommit) error
	// UpdateIdentity updates identity table with new identity entry.
	// Should pass identity which is already present in the table, otherwise an exception will be raised.
	// TODO: This function currently modifies both current+next identities based on input.
	//       This is incompatible with the design doc, and needs to be updated to modify current/next epoch separately
	// No errors are expected during normal operations.
	UpdateIdentity(updated *flow.DynamicIdentityEntry) error
	// SetInvalidStateTransitionAttempted sets a flag indicating that invalid state transition was attempted.
	// Such transition can be detected by compliance layer.
	SetInvalidStateTransitionAttempted()
	// TransitionToNextEpoch discards current protocol state and transitions to the next epoch.
	// Epoch transition is only allowed when:
	// - next epoch has been set up,
	// - next epoch has been committed,
	// - candidate block is in the next epoch.
	// No errors are expected during normal operations.
	TransitionToNextEpoch() error
	// View returns the view that is associated with this state updater.
	// StateUpdater is created for a view where protocol state changes will be applied.
	View() uint64
	// ParentState returns parent protocol state that is associated with this state updater.
	ParentState() *flow.RichProtocolStateEntry
}

// StateMutator is an interface for creating protocol state updaters and committing protocol state to the database.
// It is used by the compliance layer to update protocol state when certain events that are stored in blocks are observed.
// It has to be used for each block that is added to the block tree to maintain a correct protocol state on a block-by-block basis.
// TODO: this should be a stand-alone interface to support evolving the protocol state in the compliance layer (already possible) as well as during block construction (complex with the current implementation).
type StateMutator interface {
	// CreateUpdater creates a protocol state updater based on previous protocol state.
	// Has to be called for each block to correctly index the protocol state.
	// Expected errors during normal operations:
	//  * `storage.ErrNotFound` if no protocol state for parent block is known.
	CreateUpdater(candidateView uint64, parentID flow.Identifier) (StateUpdater, error)
	// CommitProtocolState commits the protocol state to the database.
	// Has to be called for each block to correctly index the protocol state.
	// No errors are expected during normal operations.
	CommitProtocolState(blockID flow.Identifier, updater StateUpdater) (func(tx *transaction.Tx) error, flow.Identifier)

	ApplyServiceEvents(updater StateUpdater, seals []*flow.Seal) (dbUpdates []func(*transaction.Tx) error, err error)
}
