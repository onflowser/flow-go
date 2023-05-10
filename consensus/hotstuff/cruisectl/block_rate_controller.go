// Package cruisectl implements a "cruise control" system for Flow by adjusting
// nodes' block rate delay in response to changes in the measured block rate.
//
// It uses a PID controller with the block rate as the process variable and
// the set-point computed using the current view and epoch length config.
package cruisectl

import (
	"time"

	"github.com/rs/zerolog"
	"go.uber.org/atomic"

	"github.com/onflow/flow-go/model/flow"
	"github.com/onflow/flow-go/module/component"
	"github.com/onflow/flow-go/module/irrecoverable"
	"github.com/onflow/flow-go/state/protocol"
)

// measurement represents one measurement of block rate and error.
// A measurement is taken each time the view changes for any reason.
// Each measurement measures the instantaneous and exponentially weighted
// moving average (EWMA) block rates, computes the target block rate,
// and computes the error terms.
type measurement struct {
	view            uint64    // v       - the current view
	time            time.Time // t[v]    - when we entered view v
	blockRate       float64   // r[v]    - measured instantaneous block rate at view v
	aveBlockRate    float64   // r_N[v]  - EWMA block rate over past views [v-N, v]
	targetBlockRate float64   // r_SP[v] - computed target block rate at view v
	proportionalErr float64   // e_N[v]  - proportional error at view v
	integralErr     float64   // E_N[v]  - integral of error at view v
	derivativeErr   float64   // ∆_N[v]  - derivative of error at view v
}

// epochInfo stores data about the current and next epoch. It is updated when we enter
// the first view of a new epoch, or the EpochSetup phase of the current epoch.
type epochInfo struct {
	curEpochFinalView        uint64
	curEpochTargetSwitchover time.Time
	nextEpochFinalView       *uint64
	epochFallbackTriggered   *atomic.Bool
}

// BlockRateController dynamically adjusts the block rate delay of this node,
// based on the measured block rate of the consensus committee as a whole, in
// order to achieve a target overall block rate.
type BlockRateController struct {
	cm     *component.ComponentManager
	config *Config
	state  protocol.State
	log    zerolog.Logger

	lastMeasurement *measurement    // the most recently taken measurement
	blockRateDelay  *atomic.Float64 // the block rate delay value to use when proposing a block
	epochInfo

	viewChanges chan uint64            // OnViewChange events
	epochSetups chan protocol.Snapshot // EpochSetupPhaseStarted events
}

// NewBlockRateController returns a new BlockRateController.
func NewBlockRateController(log zerolog.Logger, config *Config, state protocol.State) (*BlockRateController, error) {

	ctl := &BlockRateController{
		config:      config,
		log:         log,
		state:       state,
		viewChanges: make(chan uint64),
		epochSetups: make(chan protocol.Snapshot),
	}

	ctl.cm = component.NewComponentManagerBuilder().
		AddWorker(ctl.processEventsWorker).
		Build()

	// TODO initialize last measurement
	// TODO initialize epoch info

	return ctl, nil
}

// processEventsWorker is a worker routine which processes events received from other components.
func (ctl *BlockRateController) processEventsWorker(ctx irrecoverable.SignalerContext, ready component.ReadyFunc) {
	ready()

	done := ctx.Done()

	for {
		select {
		case <-done:
			return
		case enteredView := <-ctl.viewChanges:
			err := ctl.handleOnViewChange(enteredView)
			if err != nil {
				ctl.log.Err(err).Msgf("fatal error handling OnViewChange event")
				ctx.Throw(err)
			}
		case snapshot := <-ctl.epochSetups:
			err := ctl.handleEpochSetupPhaseStarted(snapshot)
			if err != nil {
				ctl.log.Err(err).Msgf("fatal error handling EpochSetupPhaseStarted event")
				ctx.Throw(err)
			}
		}
	}
}

// handleOnViewChange processes OnViewChange events from HotStuff.
// Whenever the view changes, we:
//   - take a new measurement for instantaneous and EWMA block rate
//   - compute a new target block rate (set-point)
//   - compute error terms, compensation function output, and new block rate delay
//   - updates epoch info, if this is the first observed view of a new epoch
func (ctl *BlockRateController) handleOnViewChange(view uint64) error {
	// TODO
	return nil
}

// handleEpochSetupPhaseStarted processes EpochSetupPhaseStarted events from the protocol state.
// Whenever we enter the EpochSetup phase, we:
//   - store the next epoch's final view
//     -
func (ctl *BlockRateController) handleEpochSetupPhaseStarted(snapshot protocol.Snapshot) error {
	// TODO
	return nil
}

// OnViewChange responds to a view-change notification from HotStuff.
// The event is queued for async processing by the worker.
func (ctl *BlockRateController) OnViewChange(oldView, newView uint64) {
	// TODO
}

// EpochSetupPhaseStarted responds to the EpochSetup phase starting for the current epoch.
// The event is queued for async processing by the worker.
func (ctl *BlockRateController) EpochSetupPhaseStarted(currentEpochCounter uint64, first *flow.Header) {
	// TODO
}

// EpochEmergencyFallbackTriggered responds to epoch fallback mode being triggered.
func (ctl *BlockRateController) EpochEmergencyFallbackTriggered() {
	ctl.epochFallbackTriggered.Store(true)
}
