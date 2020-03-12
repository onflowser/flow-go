// (c) 2019 Dapper Labs - ALL RIGHTS RESERVED

package main

import (
	"fmt"
	"time"

	"github.com/spf13/pflag"

	"github.com/dapperlabs/flow-go/cmd"
	"github.com/dapperlabs/flow-go/consensus/coldstuff"
	"github.com/dapperlabs/flow-go/engine/consensus/consensus"
	"github.com/dapperlabs/flow-go/engine/consensus/ingestion"
	"github.com/dapperlabs/flow-go/engine/consensus/matching"
	"github.com/dapperlabs/flow-go/engine/consensus/propagation"
	"github.com/dapperlabs/flow-go/engine/consensus/provider"
	"github.com/dapperlabs/flow-go/module"
	"github.com/dapperlabs/flow-go/module/buffer"
	builder "github.com/dapperlabs/flow-go/module/builder/consensus"
	finalizer "github.com/dapperlabs/flow-go/module/finalizer/consensus"
	"github.com/dapperlabs/flow-go/module/mempool"
	"github.com/dapperlabs/flow-go/module/mempool/stdmap"
	storage "github.com/dapperlabs/flow-go/storage/badger"
)

func main() {

	var (
		guaranteeLimit uint
		receiptLimit   uint
		approvalLimit  uint
		sealLimit      uint
		err            error
		chainID        string
		guarantees     mempool.Guarantees
		receipts       mempool.Receipts
		approvals      mempool.Approvals
		seals          mempool.Seals
		prop           *propagation.Engine
		prov           *provider.Engine
	)

	cmd.FlowNode("consensus").
		ExtraFlags(func(flags *pflag.FlagSet) {
			flags.UintVar(&guaranteeLimit, "guarantee-limit", 100000, "maximum number of guarantees in the memory pool")
			flags.UintVar(&receiptLimit, "receipt-limit", 100000, "maximum number of execution receipts in the memory pool")
			flags.UintVar(&approvalLimit, "approval-limit", 100000, "maximum number of result approvals in the memory pool")
			flags.UintVar(&sealLimit, "seal-limit", 100000, "maximum number of block seals in the memory pool")
			flags.StringVarP(&chainID, "chain-id", "C", "flow", "the chain ID for the protocol chain")
		}).
		Module("collection guarantees mempool", func(node *cmd.FlowNodeBuilder) error {
			guarantees, err = stdmap.NewGuarantees(guaranteeLimit)
			return err
		}).
		Module("execution receipts mempool", func(node *cmd.FlowNodeBuilder) error {
			receipts, err = stdmap.NewReceipts(receiptLimit)
			return err
		}).
		Module("result approvals mempool", func(node *cmd.FlowNodeBuilder) error {
			approvals, err = stdmap.NewApprovals(approvalLimit)
			return err
		}).
		Module("block seals mempool", func(node *cmd.FlowNodeBuilder) error {
			seals, err = stdmap.NewSeals(sealLimit)
			return err
		}).
		Component("matching engine", func(node *cmd.FlowNodeBuilder) (module.ReadyDoneAware, error) {
			results := storage.NewExecutionResults(node.DB)
			return matching.New(node.Logger, node.Network, node.State, node.Me, results, receipts, approvals, seals)
		}).
		Component("provider engine", func(node *cmd.FlowNodeBuilder) (module.ReadyDoneAware, error) {
			prov, err = provider.New(node.Logger, node.Network, node.State, node.Me)
			return prov, err
		}).
		Component("propagation engine", func(node *cmd.FlowNodeBuilder) (module.ReadyDoneAware, error) {
			prop, err = propagation.New(node.Logger, node.Network, node.State, node.Me, guarantees)
			return prop, err
		}).
		Component("ingestion engine", func(node *cmd.FlowNodeBuilder) (module.ReadyDoneAware, error) {
			ing, err := ingestion.New(node.Logger, node.Network, prop, node.State, node.Me)
			return ing, err
		}).
		Component("consensus engine", func(node *cmd.FlowNodeBuilder) (module.ReadyDoneAware, error) {
			headersDB := storage.NewHeaders(node.DB)
			payloadsDB := storage.NewPayloads(node.DB)
			cache := buffer.NewPendingBlocks()
			con, err := consensus.New(node.Logger, node.Network, node.Me, node.State, headersDB, payloadsDB, cache)
			if err != nil {
				return nil, fmt.Errorf("could not initialize engine: %w", err)
			}
			build := builder.NewBuilder(node.DB, guarantees, seals, chainID)
			final := finalizer.NewFinalizer(node.DB, guarantees, seals, prov)
			cold, err := coldstuff.New(node.Logger, node.State, node.Me, con, build, final, 3*time.Second, 6*time.Second)
			if err != nil {
				return nil, fmt.Errorf("could not initialize algorithm: %w", err)
			}
			con.WithConsensus(cold)
			return con, nil
		}).
		Run()
}
