package flow

import (
	"fmt"

	"github.com/onflow/flow-go/consensus/hotstuff/model"
)

// QuorumCertificate represents a quorum certificate for a block proposal as defined in the HotStuff algorithm.
// A quorum certificate is a collection of votes for a particular block proposal. Valid quorum certificates contain
// signatures from a super-majority of consensus committee members.
type QuorumCertificate struct {
	View    uint64
	BlockID Identifier

	// SignerIndices encodes the HotStuff participants whose vote is included in this QC.
	// For `n` authorized consensus nodes, `SignerIndices` is an n-bit vector (padded with tailing
	// zeros to reach full bytes). We list the nodes in their canonical order, as defined by the protocol.
	SignerIndices []byte

	// For consensus cluster, the SigData is a serialization of the following fields
	// - SigType []byte, bit-vector indicating the type of sig produced by the signer.
	// - AggregatedStakingSig []byte
	// - AggregatedRandomBeaconSig []byte
	// - ReconstructedRandomBeaconSig crypto.Signature
	// For collector cluster HotStuff, SigData is simply the aggregated staking signatures
	// from all signers.
	SigData []byte
}

// ID returns the QuorumCertificate's identifier
func (qc *QuorumCertificate) ID() Identifier {
	if qc == nil {
		return ZeroID
	}
	return MakeID(qc)
}

// BeaconSignature extracts the source of randomness from the QC sigData.
//
// The sigData is an RLP encoded structure that is part of QuorumCertificate.
func (qc *QuorumCertificate) BeaconSignature() ([]byte, error) {
	// unpack sig data to extract random beacon signature
	randomBeaconSig, err := model.UnpackRandomBeaconSig(qc.SigData)
	if err != nil {
		return nil, fmt.Errorf("could not unpack block signature: %w", err)
	}
	return randomBeaconSig, nil
}

// QuorumCertificateWithSignerIDs is a QuorumCertificate, where the signing nodes are
// identified via their `flow.Identifier`s instead of indices. Working with IDs as opposed to
// indices is less efficient, but simpler, because we don't require a canonical node order.
// It is used for bootstrapping new Epochs, because the FlowEpoch smart contract has no
// notion of node ordering.
type QuorumCertificateWithSignerIDs struct {
	View      uint64
	BlockID   Identifier
	SignerIDs []Identifier
	SigData   []byte
}
